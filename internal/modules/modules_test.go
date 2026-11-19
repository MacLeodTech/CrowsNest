package modules

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/macleodtech/crowsnest/internal/config"
	"github.com/macleodtech/crowsnest/internal/dockerctl"
	"github.com/macleodtech/crowsnest/internal/games"
	"github.com/macleodtech/crowsnest/internal/probes"
)

type fakeRunner struct{}

func (fakeRunner) Run(context.Context) {}

func TestBuildWiresRegisteredAndKeepsBaselineForUnknown(t *testing.T) {
	s := NewSet()
	s.Register("known", func(def *games.GameDef, env Env) []Runner {
		def.RestartCooldown = 5
		if env.Config.ID != "known" {
			t.Errorf("env.Config.ID = %q, want known", env.Config.ID)
		}
		return []Runner{fakeRunner{}}
	})
	cfg := &config.Config{Games: []config.GameConfig{{ID: "other"}, {ID: "known"}}}
	defs, runners := s.Build(cfg, &dockerctl.Client{})

	if len(defs) != 2 || defs[0].ID != "other" || defs[1].ID != "known" {
		t.Fatalf("defs out of config order: %+v", defs)
	}
	if defs[1].RestartCooldown != 5 || defs[0].RestartCooldown != 0 {
		t.Errorf("cooldowns = %v/%v, want 0/5", defs[0].RestartCooldown, defs[1].RestartCooldown)
	}
	if defs[0].Status == nil || defs[0].Players == nil {
		t.Error("a game with no module should keep the baseline status and player counter")
	}
	if len(runners) != 1 {
		t.Errorf("runners = %d, want 1", len(runners))
	}
}

func TestRegisterRejectsDuplicateAndEmpty(t *testing.T) {
	noop := func(*games.GameDef, Env) []Runner { return nil }
	for name, fn := range map[string]func(){
		"duplicate": func() { s := NewSet(); s.Register("a", noop); s.Register("a", noop) },
		"empty id":  func() { NewSet().Register("", noop) },
		"nil func":  func() { NewSet().Register("a", nil) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: want panic", name)
				}
			}()
			fn()
		}()
	}
}

func TestBundledIDs(t *testing.T) {
	// What ships in the binary; the tests also register copies of the modules published from crowsnest-modules.
	want := []string{"palworld", "valheim", "windrose"}
	s := NewSet()
	sub, err := fs.Sub(bundledFS, "bundled")
	if err != nil {
		t.Fatal(err)
	}
	registerFS(s, sub)
	if got := s.IDs(); !reflect.DeepEqual(got, want) {
		t.Errorf("embedded IDs = %v, want %v", got, want)
	}
}

func TestForConfig_LocalModuleDrivesConfiguredGame(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "terraria"), 0o755); err != nil {
		t.Fatal(err)
	}
	y := "api_version: 1\nid: terraria\nname: Terraria\nruntime: {restart_cooldown: 20s}\nstartup:\n  - {match: Done, pct: 100, label: Ready}\n"
	if err := os.WriteFile(filepath.Join(dir, "terraria", "module.yaml"), []byte(y), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "orphan"), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "orphan", "module.yaml"), []byte("api_version: 1\nid: orphan\nname: Orphan\n"), 0o644)

	cfg := &config.Config{ModulesDir: dir, Games: []config.GameConfig{
		{ID: "terraria", DisplayName: "Terraria", ContainerName: "terraria"},
		{ID: "valheim", DisplayName: "Valheim", ContainerName: "valheim"},
	}}
	var warned []string
	set, err := ForConfig(cfg, func(f string, a ...any) { warned = append(warned, fmt.Sprintf(f, a...)) })
	if err != nil {
		t.Fatal(err)
	}
	if len(warned) != 1 || !strings.Contains(warned[0], "orphan") {
		t.Fatalf("warnings %v", warned)
	}
	defs, _ := set.Build(cfg, &dockerctl.Client{})
	if defs[0].RestartCooldown != 20*time.Second || defs[0].Startup == nil {
		t.Fatalf("terraria not wired from module.yaml: %+v", defs[0])
	}
	if defs[1].Players == nil { // legacy bundled path still wires valheim
		t.Fatal("valheim lost its bundled wiring")
	}

	// A local module may not shadow a bundled one.
	os.MkdirAll(filepath.Join(dir, "valheim"), 0o755)
	os.WriteFile(filepath.Join(dir, "valheim", "module.yaml"), []byte("api_version: 1\nid: valheim\nname: V\n"), 0o644)
	if _, err := ForConfig(cfg, nil); err == nil {
		t.Fatal("expected error for shadowing a bundled module")
	}
}

func TestForConfig_NoDirIsBundledOnly(t *testing.T) {
	set, err := ForConfig(&config.Config{}, nil)
	if err != nil || len(set.IDs()) != 6 {
		t.Fatalf("%v %v", set.IDs(), err)
	}
}

func TestRAMEstimate(t *testing.T) {
	m := &Manifest{Runtime: Runtime{RAM: "6g", Memory: "10g"}}
	if got := ramEstimate(m, nil); got != 6<<30 {
		t.Fatalf("runtime.ram should win over the limit, got %d", got)
	}
	if got := ramEstimate(&Manifest{Runtime: Runtime{Memory: "10g"}}, nil); got != 10<<30 {
		t.Fatalf("the limit is the fallback, got %d", got)
	}
	if got := ramEstimate(m, map[string]string{"ram": "512m"}); got != 512<<20 {
		t.Fatalf("the instance's ram option should win, got %d", got)
	}
	if got := ramEstimate(&Manifest{}, nil); got != 0 {
		t.Fatalf("no figure means no check, got %d", got)
	}
	if got := ramEstimate(m, map[string]string{"ram": "x"}); got != 0 {
		t.Fatalf("a bad instance value skips the check, got %d", got)
	}
}

func TestPolicyDefaults(t *testing.T) {
	d := PolicyDefaults(Platform{})
	if d.Network != DefaultNetwork {
		t.Errorf("network %q", d.Network)
	}
	if !slices.Contains(d.Images, "lloesche/valheim-server") {
		t.Errorf("the valheim module's image is missing: %v", d.Images)
	}
	if d := PolicyDefaults(Platform{Network: "mine"}); d.Network != "mine" {
		t.Errorf("configured network ignored: %q", d.Network)
	}
}

func TestModulesCannotReadThePanelsOwnSecrets(t *testing.T) {
	t.Setenv("CROWSNEST_MASTER_KEY", "master")
	t.Setenv("MY_DISCORD_HOOK", "https://discord.com/api/webhooks/1/x")
	t.Setenv("WINDROSE_RCON_PASSWORD", "game-secret")
	cfg := &config.Config{}
	cfg.Server.MetricsTokenEnv = "MY_METRICS"
	cfg.Notifications = &config.NotificationsConfig{DiscordWebhookURLEnv: "MY_DISCORD_HOOK"}
	reservePanelEnv(cfg)
	e := Env{}
	for _, name := range []string{"CROWSNEST_MASTER_KEY", "CROWSNEST_ANYTHING", "GUARD_SCOPES", "MY_DISCORD_HOOK"} {
		if got := e.Secret(name); got != "" {
			t.Errorf("a module read %s", name)
		}
	}
	if got := e.Secret("WINDROSE_RCON_PASSWORD"); got != "game-secret" {
		t.Errorf("a game's own variable = %q", got)
	}
	// a game set up from the dashboard has its own store, which is not the environment
	if got := (Env{Lookup: func(string) string { return "stored" }}).Secret("CROWSNEST_X"); got != "stored" {
		t.Errorf("store lookup = %q", got)
	}
}

func TestReviewShowsWhatActionsAndPreStopSend(t *testing.T) {
	m := &Manifest{ID: "x"}
	m.Runtime.Image = "img"
	m.Actions = []probes.Action{{ID: "kick", RCON: "kick {player}"}, {ID: "save", Request: probes.Request{Method: "POST", Path: "/save", Body: map[string]string{"b": "2", "a": "1"}}}}
	m.PreStop = []probes.Step{{Request: probes.Request{Method: "POST", Path: "/shutdown"}}}
	before := m.Review()
	for _, want := range []string{`RCON command "kick {player}"`, "POST /save with a=1, b=2", "POST /shutdown"} {
		if !strings.Contains(strings.Join(before, "|"), want) {
			t.Errorf("review lacks %q: %v", want, before)
		}
	}
	m.Actions[0].RCON = "ban {player}" // an update that swaps the command must show in the diff
	added, removed := DiffLines(before, m.Review())
	if len(added) != 1 || len(removed) != 1 || !strings.Contains(added[0], "ban {player}") {
		t.Errorf("diff added %v removed %v", added, removed)
	}
}
