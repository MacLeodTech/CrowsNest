package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/macleodtech/crowsnest/internal/execguard"
	"github.com/macleodtech/crowsnest/internal/modules"
)

// A module copied into modules_dir after the exec guard started takes effect without restarting it: the file
// commands of a game that uses it are allowed once the folder is read again.
func TestWatchManagedRulesPicksUpANewModule(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusCreated) }))
	defer up.Close()
	g, err := execguard.New(up.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	managedDir, modsDir := t.TempDir(), t.TempDir()
	policy := filepath.Join(t.TempDir(), "policy.yaml")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(managedDir, "games.json"), []byte(`{"games":[{"id":"late","module":"late","state":"ready"}]}`), 0o600))
	must(os.WriteFile(policy, []byte("games_root: /srv/games\nnames: [late]\nimages: [\"@modules\"]\nnetworks: [n]\nports: [\"25565/tcp\"]\nexec: true\n"), 0o600))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	load := func() (*modules.Set, error) {
		s := modules.NewSet()
		return s, s.LoadDir(modsDir)
	}
	go watchManagedRules(ctx, g, nil, managedDir, policy, load, false)

	spec := filepath.Join(modsDir, "late")
	must(os.MkdirAll(spec, 0o755))
	must(os.WriteFile(filepath.Join(spec, "module.yaml"), []byte(`api_version: 1
id: late
name: Late
runtime:
  image: example/late
  data:
    - {path: /data}
files:
  - id: cfg
    label: Config
    path: /data/*/settings.json
`), 0o600))

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(500 * time.Millisecond)
		if g.RuleCount() > 0 {
			break
		}
	}
	if g.RuleCount() == 0 {
		t.Fatalf("no commands allowed after the module appeared")
	}
}
