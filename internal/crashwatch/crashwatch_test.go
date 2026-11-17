package crashwatch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/macleodtech/crowsnest/internal/dockerctl"
	"github.com/macleodtech/crowsnest/internal/games"
)

type fakeDocker struct{ st dockerctl.ContainerState }

func (f *fakeDocker) State(context.Context, string) (dockerctl.ContainerState, error) {
	return f.st, nil
}

type events struct{ got []string }

func (e *events) Record(kind, game, actor, detail string) {
	e.got = append(e.got, kind+":"+game+":"+detail)
}

type notes struct{ got []string }

func (n *notes) Notify(_ context.Context, m string) { n.got = append(n.got, m) }

func setup(t *testing.T) (*Watcher, *fakeDocker, *events, *notes, *time.Time) {
	t.Helper()
	reg, err := games.NewRegistry([]games.GameDef{{ID: "pal", DisplayName: "Palworld", ContainerName: "pal"}})
	if err != nil {
		t.Fatal(err)
	}
	d := &fakeDocker{}
	w := New(reg, d)
	clock := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	w.Now = func() time.Time { return clock }
	ev, no := &events{}, &notes{}
	w.History, w.Notifier = ev, no
	return w, d, ev, no, &clock
}

func TestCrashed(t *testing.T) {
	for _, c := range []struct {
		code int
		oom  bool
		want bool
	}{{0, false, false}, {143, false, false}, {137, false, false}, {137, true, true}, {139, false, true}, {1, false, true}} {
		if got := Crashed(c.code, c.oom); got != c.want {
			t.Errorf("Crashed(%d, %v) = %v, want %v", c.code, c.oom, got, c.want)
		}
	}
}

func TestLoopAlertsOnceAndClears(t *testing.T) {
	w, d, ev, no, clock := setup(t)
	ctx := context.Background()
	crash := func(restarts int, exit int) {
		*clock = clock.Add(2 * time.Minute)
		d.st = dockerctl.ContainerState{Running: true, ExitCode: exit, RestartCount: restarts, FinishedAt: *clock}
		w.Tick(ctx)
	}
	// An old crash from before CrowsNest started is not counted.
	d.st = dockerctl.ContainerState{Running: true, ExitCode: 139, FinishedAt: clock.Add(-time.Hour)}
	w.Tick(ctx)
	if w.Total("pal") != 0 {
		t.Fatal("counted a crash from before startup")
	}
	crash(1, 139)
	crash(2, 139)
	if w.Looping("pal") || len(no.got) != 0 {
		t.Fatal("alerted before the threshold")
	}
	crash(3, 139)
	if !w.Looping("pal") || len(no.got) != 1 || !strings.Contains(no.got[0], "Palworld") || w.Total("pal") != 3 {
		t.Fatalf("looping=%v notes=%v total=%d", w.Looping("pal"), no.got, w.Total("pal"))
	}
	crash(4, 139) // still looping: no second alert
	if len(no.got) != 1 {
		t.Fatalf("alerted again: %v", no.got)
	}
	// A clean stop (exit 0) is not a crash, and once the window passes it clears.
	*clock = clock.Add(time.Hour)
	d.st = dockerctl.ContainerState{ExitCode: 0, RestartCount: 4, FinishedAt: *clock}
	w.Tick(ctx)
	if w.Looping("pal") || w.Total("pal") != 4 {
		t.Fatalf("looping=%v total=%d after a clean stop", w.Looping("pal"), w.Total("pal"))
	}
	var crashes, loops int
	for _, e := range ev.got {
		if strings.HasPrefix(e, "crash:") {
			crashes++
		}
		if strings.HasPrefix(e, "crash_loop:") {
			loops++
		}
	}
	if crashes != 4 || loops != 1 {
		t.Fatalf("events = %v", ev.got)
	}
}

func TestRestartPolicyCyclesBetweenPollsAreCounted(t *testing.T) {
	w, d, _, _, clock := setup(t)
	ctx := context.Background()
	d.st = dockerctl.ContainerState{Running: true, FinishedAt: *clock}
	w.Tick(ctx)
	*clock = clock.Add(time.Minute)
	d.st = dockerctl.ContainerState{Running: true, ExitCode: 1, RestartCount: 3, FinishedAt: *clock}
	w.Tick(ctx)
	if w.Total("pal") != 3 || !w.Looping("pal") {
		t.Fatalf("total=%d looping=%v, want 3 crashes between two polls to loop", w.Total("pal"), w.Looping("pal"))
	}
}

// Docker 29 resets the exit code to 0 once a restarted container is running, so only the restart count shows that
// the game died. Seen on a real Minecraft killed with SIGSEGV (docker's die event said 255; inspect said 0).
func TestRestartedByDockerWithForgottenExitCodeCounts(t *testing.T) {
	w, d, ev, _, clock := setup(t)
	ctx := context.Background()
	d.st = dockerctl.ContainerState{Running: true, FinishedAt: *clock}
	w.Tick(ctx)
	for i := 1; i <= 3; i++ {
		*clock = clock.Add(time.Minute)
		d.st = dockerctl.ContainerState{Running: true, ExitCode: 0, RestartCount: i, FinishedAt: *clock}
		w.Tick(ctx)
	}
	if w.Total("pal") != 3 || !w.Looping("pal") {
		t.Fatalf("total=%d looping=%v, want 3 crashes and a loop", w.Total("pal"), w.Looping("pal"))
	}
	if len(ev.got) == 0 || !strings.Contains(ev.got[0], "restarted by docker") {
		t.Fatalf("events = %v", ev.got)
	}
}

func TestStopsAreNotCrashes(t *testing.T) {
	w, d, _, _, clock := setup(t)
	ctx := context.Background()
	w.Tick(ctx)
	for i, code := range []int{0, 143, 137} {
		*clock = clock.Add(time.Minute)
		d.st = dockerctl.ContainerState{ExitCode: code, FinishedAt: *clock}
		w.Tick(ctx)
		if w.Total("pal") != 0 {
			t.Fatalf("exit %d (#%d) counted as a crash", code, i)
		}
	}
}
