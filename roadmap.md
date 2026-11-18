# CrowsNest — Roadmap

Single source of truth for work that is **still to do**. Finished work lives in
[`CHANGELOG.md`](CHANGELOG.md); the design detail behind the modular-games rework is in
[`docs/modularization-plan.md`](docs/modularization-plan.md). When an item lands, delete it here and add it to
`CHANGELOG.md` with the date.

Origin: a full-repo review (2026-09-12), the modular-games design review, the accounts/settings review (2026-11-08/09)
and a full review at `432053b` (2026-11-18; `go vet`, `go test ./...` and `govulncheck` clean, no exploitable issue
found). The roadmap was audited against the code on 2026-11-18: items already done or already designed away were
removed, and the former tracks A, B and C were merged into the sections below so each item appears once.

Phases 0, 1, 3, 4, 5 and 6 of the modular-games rework are done (see `CHANGELOG.md`).

## Priority order (updated 2026-11-18)

### Now: live checks on prod (one play session each)

Palworld, Valheim and Windrose passed their restored-world play-tests on 2026-11-18 (see `CHANGELOG.md`). Left:

- [ ] **Windrose join from outside the LAN** by invite code `TwoManSquad`, on the macvlan setup (`dockervlan`
  10.0.1.16, games net 192.168.0.50; the Windrose+ dashboard answers on :8780). Check the firewall for `:5001` and the
  game ports.
- [ ] **Windrose Files-tab edit** (the world-settings form) on a running server with a real save, then a restart to
  apply it (the image's updater applies it; not tried yet).
- [ ] **Valheim crossplay** join code and player names with `CROSSPLAY=1` (the log patterns come from the known
  format, not a capture).
- [ ] **Idle and night auto-stop with real Discord notices.** Set `DISCORD_WEBHOOK_URL` on prod first (the channel
  only accepts discord.com addresses). Notices are verified on the test stack (start, stop, backup, restore, crash
  loop, offsite) but not on Discord itself. A game gets a fixed one-hour grace until a player has been seen, so this
  is slow; the earlier test stack was torn down on 2026-11-17, so run it on prod during normal play. Also check a
  crash notice (crash detection itself passed on 2026-11-18).

### Next: data safety and ops

- [ ] **Valheim stop backup is taken before the shutdown save.** Valheim has no on-demand save, so the panel's stop
  backup runs first and holds the previous autosave (seen 2026-11-18: backup had `_main.3` at 07:10:15, the shutdown
  save wrote `_main.4` at 07:10:18). The world is safe, but a restore loses up to one autosave interval. Confirmed
  still open: modules only have `pre_stop` steps, nothing runs after the container exits. Likely fix: for games with
  no `pre_stop` save, take a second backup after exit. Check Palworld's stop backup too (it autosaves every 30
  seconds, so the gap should be small).
- [ ] **Set up the offsite backups sidecar on prod.** Today the only copies of the worlds are on the same disk
  (`game-backups`, `game-backups-temp`).
- [ ] **Restore drill on prod Palworld** from a panel backup (`restore`, `restore_binds` and the `palworld-restore`
  scope are in place; never run there). See `docs/backups.md`. Restore is verified on a real Minecraft world
  (2026-11-16) and on a config-file game on the test stack; Valheim, Source, 7 Days to Die and Don't Starve Together
  are unchecked on real worlds. Config-file games back up inside their own data and would need the bind added by hand.
- [ ] **Backup verification:** a periodic job that opens the newest archive (`tar -tzf`) and records the result in
  `backup/health`, so a bad backup is found before a restore is needed.
- [ ] **Commit the CrowsNest parts of `~/docker`** (compose, config, policy, README). They are uncommitted and mixed
  with unrelated edits, so commit selectively.
- [ ] **Passkeys on a phone and a security key** (Windows Hello is done).
- [ ] **Clean up** `~/docker/storage/retired-prod` and `game-backups-temp` once offsite backups and the restore drill
  are confirmed. Also note the Valheim config `backups` folder was not restored.

### Then: hardening and small fixes

- [ ] **Security re-review at 0.16.x** of what changed since v0.13.0: the Engine API, file-editing forms, macvlan
  networking, generated secrets shown on the card. This review covered auth, the web surface, the guard create rules
  and `gamefiles`; still to audit are the guard's remaining paths (exec, networks, volumes), `managed` request
  handling, `exclusivity` and the dashboard JS (`app.js`, `games.js`, `settings.js`), which were only skimmed.
- [ ] **Guard memory ceilings.** `Memory`, `MemorySwap` and `ShmSize` accept any size in the guard
  (`dockerguard/rules.go`); memory admission lives only in the panel, which the guard assumes may be compromised. Add
  a per-scope maximum in `policy.yaml`. Denial-of-service risk, not an escape.
- [ ] **Cap `Limiter.Hit`** (`auth/ratelimit.go`): it appends on every failed sign-in even when the key is already
  over the limit, so one IP can grow its list without bound inside the window. Keep at most `max` entries.
- [ ] **File-edit history keeps raw secrets.** `gamefiles` stores earlier versions with redacted values un-masked in
  `HistoryDir` (0600, admin-only). Encrypt them with `vault`, or redact before keeping.
- [ ] **Request deadlines.** The main server sets `ReadHeaderTimeout` only, so a slow-body client can hold a
  connection. Add `http.TimeoutHandler` (or per-request deadlines through `ResponseController`) for non-streaming
  routes.
- [ ] **Symlink check for bind sources.** `bindOK` matches exact paths, but a game with write access to its data
  folder could plant a symlink that a later bind follows on the host. Resolve with `EvalSymlinks` at create time, or
  have `doctor` warn.
- [ ] **Update diff shows permission changes only**, so a changed env value shows "nothing" (found in R5; still the
  case).
- [ ] **A game that failed for a missing module only recovers after a restart** (found in R5).
- [ ] **Static asset delivery:** gzip and content-hashed `Cache-Control` for `/static/` (`app.js` is ~1.7k lines,
  unminified). Low value on a LAN or Tailscale, more behind Cloudflare.

### Features to consider

- [ ] **"Server is ready" notice:** a Discord ping through the existing notifier, using startup-progress detection,
  for a friend waiting on a slow Palworld or Windrose start.
- [ ] **Per-game play history and stats page:** hours played, peak players and crashes, from the data `history` and
  `playerwatch` already collect.
- [ ] **Metrics dashboard and alerts:** a ready-made Grafana JSON and alert rules on top of `/metrics` (crash loop,
  offsite stale, disk below 10%).
- [ ] **`files:` entries for the Source, Don't Starve Together and 7 Days to Die modules.** Parked by the owner on
  2026-11-17. The modules live in the modules repository; confirm each config path against the real image first. Also
  a stopped-game edit for games set up in `config.yaml` (low value).
- [ ] **Discord slash commands** (`/start`, `/stop`, `/status`). Parked by the owner on 2026-11-12. Needs a Discord
  application and its public key before it can start.
- [ ] **Tier 2 adapter-sidecar contract** (modular games, Phase 2), only when a module needs it.

### Operating notes

- A stop or start of a game done outside the panel (plain `docker`) skips its backup, so test starts on prod should go
  through the panel.

## Considered, not doing

- Go `plugin` package or in-process plugins: they would run inside the panel's trust boundary. WASM (wazero) stays an
  option once the adapter contract is stable.
- Re-reading the registry on a change notice instead of every 5 seconds: the poll is an in-memory read and the hub
  already has a reconcile signal, so there is nothing to save.
- Installing a module whose image the policy does not list from the dashboard: by design the dashboard cannot grant
  it; the card shows the exact block to add to `policy.yaml` and the guard picks it up within seconds.
- A one-shot stats call: `Stats()` already takes one sample and diffs against the cached previous one.
