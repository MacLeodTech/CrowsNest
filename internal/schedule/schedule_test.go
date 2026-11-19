package schedule

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/macleodtech/crowsnest/internal/backup"
	"github.com/macleodtech/crowsnest/internal/games"
)

type fakeCtl struct{ starts, stops []string }

func (f *fakeCtl) RequestStart(_ context.Context, id string) error {
	f.starts = append(f.starts, id)
	return nil
}
func (f *fakeCtl) RequestStop(_ context.Context, id string) error {
	f.stops = append(f.stops, id)
	return nil
}

func setup(t *testing.T, list []Entry) (*Runner, *fakeCtl, *time.Time) {
	t.Helper()
	reg, err := games.NewRegistry([]games.GameDef{{ID: "alpha", DisplayName: "Alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := Open(filepath.Join(t.TempDir(), "s.json"))
	if err := st.Replace(list, func(id string) bool { _, ok := reg.Get(id); return ok }); err != nil {
		t.Fatal(err)
	}
	ctl := &fakeCtl{}
	// Friday 2026-11-13 18:00 UTC.
	now := time.Date(2026, 11, 13, 18, 0, 5, 0, time.UTC)
	return &Runner{Store: st, Registry: reg, Control: ctl, Loc: time.UTC, Now: func() time.Time { return now }}, ctl, &now
}

func TestFiresOncePerMinute(t *testing.T) {
	r, ctl, now := setup(t, []Entry{
		{ID: "fri", Game: "alpha", Action: "start", Days: []int{5}, Time: "18:00", Enabled: true},
		{ID: "off", Game: "alpha", Action: "stop", Days: []int{5}, Time: "18:00", Enabled: false},
		{ID: "sat", Game: "alpha", Action: "stop", Days: []int{6}, Time: "18:00", Enabled: true},
	})
	r.Tick(context.Background())
	*now = now.Add(20 * time.Second)
	r.Tick(context.Background())
	if len(ctl.starts) != 1 || len(ctl.stops) != 0 {
		t.Fatalf("starts=%v stops=%v", ctl.starts, ctl.stops)
	}
	*now = now.Add(time.Minute)
	r.Tick(context.Background())
	if len(ctl.starts) != 1 {
		t.Errorf("fired again outside its minute: %v", ctl.starts)
	}
}

func TestValidate(t *testing.T) {
	known := func(id string) bool { return id == "alpha" }
	good := Entry{ID: "a", Game: "alpha", Action: "stop", Days: []int{0, 6}, Time: "07:30"}
	if err := good.Validate(known); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*Entry){
		"game":   func(e *Entry) { e.Game = "x" },
		"action": func(e *Entry) { e.Action = "restart" },
		"time":   func(e *Entry) { e.Time = "7:30" },
		"hour":   func(e *Entry) { e.Time = "24:00" },
		"days":   func(e *Entry) { e.Days = nil },
		"day":    func(e *Entry) { e.Days = []int{7} },
		"dup":    func(e *Entry) { e.Days = []int{1, 1} },
		"id":     func(e *Entry) { e.ID = "A B" },
	} {
		e := good
		mut(&e)
		if e.Validate(known) == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestReloadFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	st, _ := Open(path)
	if err := st.Replace([]Entry{{ID: "a", Game: "alpha", Action: "start", Days: []int{3, 1}, Time: "09:00", Enabled: true}}, func(string) bool { return true }); err != nil {
		t.Fatal(err)
	}
	again, err := Open(path)
	if err != nil || len(again.List()) != 1 || again.List()[0].Days[0] != 1 {
		t.Fatalf("reload = %+v %v", again.List(), err)
	}
}

type fakeBackups struct {
	got []string
	err error
}

func (f *fakeBackups) Request(_ context.Context, d games.GameDef, actor string) error {
	f.got = append(f.got, d.ID+" by "+actor)
	return f.err
}

func TestBackupAction(t *testing.T) {
	r, ctl, _ := setup(t, []Entry{{ID: "nightly", Game: "alpha", Action: "backup", Days: []int{5}, Time: "18:00", Enabled: true}})
	fb := &fakeBackups{}
	r.Backups = fb
	var told []string
	r.Notify = func(_ context.Context, m string) { told = append(told, m) }
	r.Tick(context.Background())
	if len(fb.got) != 1 || fb.got[0] != "alpha by schedule" || len(ctl.starts)+len(ctl.stops) != 0 {
		t.Fatalf("backups %v starts %v stops %v", fb.got, ctl.starts, ctl.stops)
	}
	if len(told) != 0 {
		t.Errorf("a normal backup told someone it failed: %v", told)
	}

	// a game that is down, or already being backed up, is skipped without a fuss
	for _, err := range []error{backup.ErrNotRunning, backup.ErrBusy} {
		r, _, _ := setup(t, []Entry{{ID: "nightly", Game: "alpha", Action: "backup", Days: []int{5}, Time: "18:00", Enabled: true}})
		r.Backups = &fakeBackups{err: err}
		told = nil
		r.Notify = func(_ context.Context, m string) { told = append(told, m) }
		r.Tick(context.Background())
		if len(told) != 0 {
			t.Errorf("%v was announced: %v", err, told)
		}
	}
	// a real failure is announced; so is a missing backup manager
	r2, _, _ := setup(t, []Entry{{ID: "nightly", Game: "alpha", Action: "backup", Days: []int{5}, Time: "18:00", Enabled: true}})
	r2.Backups = &fakeBackups{err: errors.New("disk full")}
	told = nil
	r2.Notify = func(_ context.Context, m string) { told = append(told, m) }
	r2.Tick(context.Background())
	if len(told) != 1 {
		t.Errorf("a failed backup was not announced: %v", told)
	}
}
