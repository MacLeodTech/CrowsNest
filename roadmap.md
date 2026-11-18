# CrowsNest — Roadmap

Single source of truth for work that is **still to do**. Finished work lives in
[`CHANGELOG.md`](CHANGELOG.md); the design detail behind the modular-games rework is in
[`docs/modularization-plan.md`](docs/modularization-plan.md). When an item lands, delete it here and add it to
`CHANGELOG.md` with the date.

Origin: a full-repo review (2026-09-12), the modular-games design review and
the accounts/settings review (2026-11-08/09).

## Active roadmap

Three tracks that share one path: **A. Modular games** (the phased rework),
**B. Accounts and settings** (largely rides on the modules), and **C. Verify
and harden** (open checks against real servers). Phase numbers match
`docs/modularization-plan.md`. Phases 0, 1, 3, 4, 5 and 6 are done (see `CHANGELOG.md`).

### Priority order (updated 2026-11-18)

Origin: the full review at `35f22b6` (v0.13.0; `go vet`, `go test ./...` and `govulncheck` clean; the guard rules,
`managed.go`, `exclusivity` and the dashboard JS were skimmed, not audited). All of its P0 to P3 items are done
(R4, R5, #44, the Allow switch, passkeys, R15 to R23); see `CHANGELOG.md`. What is left, in order:

#### Now: finish the live checks (one play session each)

- [ ] **Windrose join from outside the LAN:** a friend joins by invite code `TwoManSquad`. The LAN join, save, panel
  stop backup, graceful shutdown and restart-and-rejoin passed on 2026-11-18 (see track C).
- [ ] **Windrose Files-tab edit** (the world-settings form) on a running server, then a restart to apply it.
- [ ] **Valheim crossplay** join code and player names with `CROSSPLAY=1`.

#### Next: data safety and ops

- [ ] **Valheim stop backup is taken before the shutdown save.** Valheim has no on-demand save, so the panel's
  stop backup runs first and holds the previous autosave (seen 2026-11-18: backup had `_main.3` at 07:10:15, the
  shutdown save wrote `_main.4` at 07:10:18). The world is safe, but a restore loses up to one autosave interval.
  Likely fix: for games with no `pre_stop` save, take a second backup after the container exits. Check
  Palworld's stop backup too (it autosaves every 30 seconds, so the gap should be small).
- [ ] **Set up the offsite backups sidecar on prod.** Today the only copies of the worlds are on the same disk
  (`game-backups`, `game-backups-temp`).
- [ ] **Commit the CrowsNest parts of `~/docker`** (compose, config, policy, README). They are uncommitted and mixed
  with unrelated edits, so commit selectively.
- [ ] **Set `DISCORD_WEBHOOK_URL` on prod**, then check idle and night auto-stop and the Discord notices during
  normal play (see C).
- [ ] **Restore drill on prod Palworld** from a panel backup (`restore`, `restore_binds` and the `palworld-restore`
  scope are in place; never run there). See `docs/backups.md`.
- [ ] **Passkeys on a phone and a security key** (Windows Hello is done).

#### Then: hardening and small fixes

- [ ] **Security re-review at 0.16.x** of what changed since v0.13.0: the Engine API, file-editing forms, macvlan
  networking, generated secrets shown on the card. Include `dockerguard` rules, `managed.go`, `exclusivity` and the
  dashboard JS, which the last review only skimmed.
- [ ] **Update diff shows permission changes only**, so a changed env value shows "nothing" (found in R5).
- [ ] **A game that failed for a missing module only recovers after a restart** (found in R5).
- [ ] **Clean up** `~/docker/storage/retired-prod` and `game-backups-temp` once the play-tests and offsite backups
  are confirmed.

#### Larger design work

- [ ] **Tier 2 adapter-sidecar contract** (track A, Phase 2), when a module needs it.

### A. Modular games (open)

- [ ] **Phase 2 leftovers:** the Tier 2
  adapter-sidecar contract is deferred until a module needs it.
- [ ] **Phase 5 leftovers:** installing a module whose image the
  policy does not list (shown as "needs policy lines"). The extra port range is edited in `policy.yaml`.

### B. Accounts and settings (open)

- [ ] **Backups:** the restore is verified on a real Minecraft world (2026-11-16) and on a config-file game on the
  test stack. Left: the config-file restore on prod Palworld (the `restore` and `restore_binds` settings and the
  `palworld-restore` guard scope are in place on prod, but a restore has not been run there yet; see
  `docs/backups.md`), and Valheim, Source, 7 Days to
  Die and Don't Starve Together on real worlds. Config-file games still back up inside their own data and would need
  the bind added by hand.
- [ ] **File editing, next steps:** Palworld's form and the Windrose `WorldDescription.json` file and form are done
  (see `CHANGELOG.md`). Left: check a Windrose edit on a running server with a real save (restart applies it via the
  image's updater; not tried yet), and a stopped-game edit for games set up in `config.yaml` (low value).

### C. Verify and harden (open, against real servers)

- [ ] **Play-test the restored worlds on prod.** The fresh layout is live (games under
  `~/docker/storage/game-servers`, backups under `game-backups`). **Palworld passed on 2026-11-18:** a player joined
  save `A826860D…`, it saved, a panel stop took a backup, and a restart and rejoin kept the changes. **Valheim passed on 2026-11-18:**
  world `Midgard` saved on shutdown and kept the changes after a restart; its stop backup lags one save behind (see
  Next). **Windrose passed on 2026-11-18** (old world `320FFB65…`): joined by invite code from the LAN, saved, a panel stop
  took a backup (`world-20261118-072027.tar.gz`) with a graceful shutdown, and a restart and rejoin kept the changes.
  Left: the join from outside the LAN and a Files-tab edit (see Now).
- [ ] **Windrose invite-code join from outside the LAN** (the LAN join passed), on the macvlan setup. The server is added
  and enabled on prod (`dockervlan` 10.0.1.16, games net 192.168.0.50, Windrose+ dashboard answers on :8780).
- [x] **Check the Windrose startup bar on the next start** (checked 2026-11-18: it showed progress) (v0.16.23 adds the download percentage and first-run
  steps from the real log; the module must update on prod first). The "ready for owner to connect" line did appear.
- [ ] **Valheim crossplay join code and player names** on a real server with `CROSSPLAY=1` (the log patterns are from
  the known format, not a capture).
- [ ] **Idle/night auto-stop and Discord notifications** (backlog 2026-11-17: the real Discord webhook check) against real servers. Webhook notices are verified on the
  test stack (start, stop, backup, restore, crash loop, offsite) but the Discord channel itself is not: it only
  accepts discord.com addresses, so it needs a real webhook. A game gets a fixed one-hour grace until a player has
  been seen, which makes this slow to test; the test stack that held the earlier idle-stop attempt was torn down on
  2026-11-17, so run it again. Check the firewall for `:5001` and game ports.

### Backlog (parked, come back to it later)

- [ ] **Prod loose ends (2026-11-17):** a stop or start of a game done outside the panel (plain `docker`) skips its
  backup, so test starts should go through the panel. The Valheim config `backups` folder was not restored. (Offsite
  backups, the `~/docker` commit, the Discord webhook and the cleanup of `retired-prod` and `game-backups-temp` are
  in "Next" above.)

- [ ] **`files:` entries for the Source, Don't Starve Together and 7 Days to Die modules.** Parked by the owner on
  2026-11-17. The modules live in the modules repository; confirm each config path against the real image first.
- [ ] **Discord slash commands** (track B). Parked by the owner on 2026-11-12. Needs a Discord application and its
  public key before it can start.

### Considered, not doing

- Go `plugin` package or in-process plugins: they would run inside the panel's trust boundary. WASM (wazero)
  stays an option once the adapter contract is stable.
