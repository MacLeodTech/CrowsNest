package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/macleodtech/crowsnest/internal/auth"
	"github.com/macleodtech/crowsnest/internal/config"
	"github.com/macleodtech/crowsnest/internal/dockerguard"
	"github.com/macleodtech/crowsnest/internal/mfa"
)

const minMetricsTokenLen = 24

// doctorSecurity holds the checks that come from the threat model (docs/threat-model.md): settings that are valid
// but leave a door open.
func doctorSecurity(r *doctorReport, cfg *config.Config, users auth.Users) {
	if env := cfg.Server.MetricsTokenEnv; env != "" {
		if tok := envValue(env); tok != "" && len(tok) < minMetricsTokenLen {
			r.add(docWarn, fmt.Sprintf("the /metrics token (%s) is only %d characters", env, len(tok)),
				fmt.Sprintf("use at least %d random characters (`openssl rand -hex 24`); wrong guesses are throttled but a short token is still guessable", minMetricsTokenLen))
		}
	}
	if cfg.Server.SecureCookies && len(cfg.Server.TrustedProxies) == 0 {
		r.add(docWarn, "secure_cookies is on, which means a reverse proxy, but trusted_proxies is empty: every visitor looks like the proxy, so one person's wrong passwords throttle everyone",
			"list the proxy's address or subnet under server.trusted_proxies")
	}
	doctorSecondFactor(r, cfg, users)
}

// doctorSecondFactor warns about admins who sign in with a password alone. It reads the enrolment files the way
// the panel does, without the vault (only whether a record exists and is enabled, never a secret).
func doctorSecondFactor(r *doctorReport, cfg *config.Config, users auth.Users) {
	dir := filepath.Dir(cfg.Server.SettingsFile)
	mf, err := mfa.NewStore(filepath.Join(dir, "mfa.json"), nil)
	if err != nil {
		r.add(docWarn, "could not read the two-factor file: "+err.Error(), "")
		return
	}
	pk, err := auth.NewPasskeyStore(filepath.Join(dir, "passkeys.json"))
	if err != nil {
		r.add(docWarn, "could not read the passkeys file: "+err.Error(), "")
		return
	}
	var bare []string
	for name, u := range users {
		if u.EffectiveRole() != auth.RoleAdmin || u.Hash == "" {
			continue
		}
		if !mf.Enabled(name) && pk.Count(name) == 0 {
			bare = append(bare, name)
		}
	}
	if len(bare) == 0 {
		return
	}
	sort.Strings(bare)
	r.add(docWarn, "admin(s) who sign in with a password alone: "+strings.Join(bare, ", "),
		"turn on an authenticator app or add a passkey on each one's Account page")
}

// doctorSuid warns when a folder the games can write is mounted without nosuid. A compromised panel can run as
// root in an allowed game image against its bind folder, and a setuid-root file planted there is a way to root on
// the host for anyone or anything that later runs it. path is as this process sees it; what is the plain-words name.
func doctorSuid(r *doctorReport, what, path, mountinfo string) {
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		// a host path the panel does not mount: its own mountinfo says nothing about it
		r.add(docInfo, fmt.Sprintf("%s (%s) is not visible from here, so its mount options were not checked", what, path),
			"on the host run: findmnt -T "+path+" -o OPTIONS (it should list nosuid,nodev)")
		return
	}
	opts, known := mountOptions(path, mountinfo)
	switch {
	case !known:
		r.add(docInfo, fmt.Sprintf("could not read the mount options of %s", what),
			"check on the host: findmnt -T "+path+" -o OPTIONS (it should list nosuid,nodev)")
	case !hasOpt(opts, "nosuid"):
		r.add(docWarn, fmt.Sprintf("%s (%s) is not mounted nosuid: a setuid file planted there by a game container would stay setuid on the host", what, path),
			"mount that disk with nosuid,nodev (an /etc/fstab options change) and remount it")
	default:
		r.add(docOK, fmt.Sprintf("%s is mounted nosuid", what), "")
	}
}

func hasOpt(opts []string, want string) bool {
	for _, o := range opts {
		if o == want {
			return true
		}
	}
	return false
}

// mountOptions are the mount options of the mount holding path, read from a mountinfo file: the entry with the
// longest mount point that is path or a folder above it. known is false when the file cannot be read or nothing
// matches.
func mountOptions(path, mountinfo string) (opts []string, known bool) {
	b, err := os.ReadFile(mountinfo) // #nosec G304 -- a fixed /proc path, or a test file
	if err != nil {
		return nil, false
	}
	path = filepath.Clean(path)
	best := -1
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		mp := filepath.Clean(strings.ReplaceAll(f[4], `\040`, " "))
		if path != mp && !strings.HasPrefix(path, strings.TrimSuffix(mp, "/")+"/") {
			continue
		}
		if len(mp) > best {
			best = len(mp)
			opts, known = strings.Split(f[5], ","), true
		}
	}
	return opts, known
}

// doctorPolicyHardening notes policy settings that are valid but leave out a limit.
func doctorPolicyHardening(r *doctorReport, pol *dockerguard.Policy) {
	if !pol.PinDigests {
		r.add(docWarn, "the policy does not set pin_digests: a dashboard game runs whatever its image tag points at when it is created, so an upstream push changes the code in your game",
			"add `pin_digests: true` to policy.yaml")
	}
	if pol.MaxMemoryBytes() == 0 {
		r.add(docInfo, "the policy has no max_memory, so the guard puts no ceiling on a container's memory (the panel's own check is the only one)",
			"add `max_memory: 16g` (your largest game) to policy.yaml; every create must then carry a Memory limit")
	}
}

// doctorSymlinks warns about a symlink under root (a few levels down) that points outside it. A bind is matched by
// its path, but the host follows a link in that path: a game that can write inside its own folder could replace a
// folder it is bound to with a link to somewhere else, and the next start would then bind that. Links that stay
// inside root are fine. Folders the process cannot see are skipped (their check is the host's).
func doctorSymlinks(r *doctorReport, what, root string) {
	const maxDepth, maxEntries, maxShown = 4, 50000, 5
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		return
	}
	var bad []string
	seen := 0
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() && rel != "." && strings.Count(rel, string(filepath.Separator)) >= maxDepth-1 {
			return filepath.SkipDir
		}
		if seen++; seen > maxEntries {
			return filepath.SkipAll
		}
		if d.Type()&os.ModeSymlink == 0 {
			return nil
		}
		target, err := filepath.EvalSymlinks(p)
		if err != nil || target != real && !strings.HasPrefix(target, real+string(filepath.Separator)) {
			bad = append(bad, p)
		}
		return nil
	})
	if len(bad) == 0 {
		r.add(docOK, fmt.Sprintf("%s holds no link that leads outside it", what), "")
		return
	}
	sort.Strings(bad)
	more := ""
	if len(bad) > maxShown {
		more = fmt.Sprintf(" (and %d more)", len(bad)-maxShown)
		bad = bad[:maxShown]
	}
	r.add(docWarn, fmt.Sprintf("%s holds a symlink that leads outside it: %s%s", what, strings.Join(bad, ", "), more),
		"a game that can write here could use it to make a later start bind another part of the host: remove the link, or keep what it points to inside the folder")
}
