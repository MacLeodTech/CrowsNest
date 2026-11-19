package games

import (
	"log"
	"os"

	"github.com/macleodtech/crowsnest/internal/config"
	"github.com/macleodtech/crowsnest/internal/dockerctl"
	"github.com/macleodtech/crowsnest/internal/logging"
)

// FromConfig builds one GameDef per configured game, wired with the Phase 3
// baseline strategies (docker-ps-only status, always-undetermined player
// count). Real per-game packages (Phase 10) construct richer GameDefs
// directly; this is the fallback that makes every configured game
// start/stop-able before that integration work lands.
func FromConfig(cfgs []config.GameConfig, docker *dockerctl.Client) []GameDef {
	defs := make([]GameDef, 0, len(cfgs))
	for _, g := range cfgs {
		defs = append(defs, GameDef{
			ID:             g.ID,
			DisplayName:    g.DisplayName,
			ContainerName:  g.ContainerName,
			ConnectAddress: g.ConnectAddress,
			ConnectNote:    g.ConnectNote,
			ConnectSecrets: connectSecrets(g),
			NightShutdown:  g.NightShutdown == nil || *g.NightShutdown,
			Backup:         backupSpec(g.Backup),
			Status:         &DockerStatusChecker{Docker: docker},
			Players:        UnknownPlayerCounter{},
		})
	}
	return defs
}

func backupSpec(b *config.BackupConfig) *BackupSpec {
	if b == nil {
		return nil
	}
	return &BackupSpec{Exec: b.Exec, ListDir: b.ExecListDir(), LocalDir: b.LocalDir, BeforeStop: b.BeforeStop, Keep: b.Keep,
		CanRestore: len(b.Restore) > 0 && (b.LocalDir != "" || len(b.RestoreBinds) > 0),
		Dir:        b.ExecListDir(), RestoreCmd: b.Restore, RestoreBinds: b.RestoreBinds}
}

// DockerTarget converts a GameDef's container identity into a
// dockerctl.Target for Start/Stop calls.
func (d GameDef) DockerTarget() dockerctl.Target {
	return dockerctl.Target{ContainerName: d.ContainerName}
}

// connectSecrets resolves a game's connect_secrets from the environment.
// A variable that is unset or empty is skipped with a log line naming the
// variable (never a value), so a typo doesn't silently show nothing.
func connectSecrets(g config.GameConfig) []ConnectSecret {
	var out []ConnectSecret
	for _, s := range g.ConnectSecrets {
		v := os.Getenv(s.Env)
		if v == "" {
			log.Printf("games: %s: %s is unset or empty; %q not shown on the card", logging.Clean(g.ID), logging.Clean(s.Env), s.Label)
			continue
		}
		out = append(out, ConnectSecret{Label: s.Label, Value: v})
	}
	return out
}
