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

### Priority order (review of 2026-11-12)

A full review of the platform and repository at `35f22b6` (v0.13.0). `go vet` and `go test ./...` passed and
`govulncheck` found nothing reachable. The findings come from reading the code, not from live exploitation;
`dockerguard`'s rules, `managed.go`, `exclusivity` and the dashboard JS were skimmed, not audited (the guard had
its own review and fuzzing on 2026-11-10). IDs are stable so commits can refer to them. Work the tiers in order;
inside a tier, top to bottom. Items that already live in track A, B or C are named here and stay tracked there.

#### P0: do now (small, and each closes a real gap)

- [x] **R4 (ops). Upgrade production from 0.11.0 to 0.16.2.** Done 2026-11-16: template-check reconciled, game
  containers created (`up --no-start`), master key added, panel, execguard and guard recreated and healthy.
  Prod now runs v0.16.19 (checked 2026-11-17). Two things the upgrade needed that the templates don't say yet: `users.env` and the session secret must live in
  the writable data folder (`users_file: /app/data/users.env`, `session_secret_file: /app/data/.session_secret`),
  because 0.16 rewrites them through a temp file in the same directory and a single-file bind mount under a
  read-only root fails (the panel crash-looped on "read-only file system"); and execguard now needs the
  `policy` folder mounted too.
- [x] **R5 (ops). Phase 5 live checks on the test stack:** done 2026-11-16 in a real browser: signed install of
  Minecraft from the public crowsnest-modules URL, the module-update check and apply, "Make a copy" (valheim-copy
  on ports 2466/2467), the offsite banner (failed shows, ok hides), and Start→Switch between two real games.
  The update diff panel lists permission changes only, so a changed env value shows "nothing"; a game that was
  failing for a missing module only recovers after a restart.

#### P1: next (security design and data safety)

- [x] **Fix the templates for the bind-mount failure (#44):** put `users.env` and the session secret in the data folder in
  `compose.yaml.example`, `config.yaml.example` and `quickstart/`, mount `policy` into execguard, or make the writes
  fall back to in-place when the file is a mount.
- [x] **Try the signed-module Allow switch on a real guard.** Done 2026-11-16 on the test stack in a browser: installed
  Minecraft from the public URL, switched it on, added its image to `policy.yaml` on the host, added the game, started
  and stopped it (healthy on the chosen port). Rough edge found: a game whose first image pull the guard refused
  stays `failed`; Update image does re-run it, so the card now says **Retry install** (#47).
- [x] **Try passkeys in a real browser.** Windows Hello worked (2026-11-17). A phone and a security key are untried.
- [ ] **Track C checks against real servers:** idle and night auto-stop with Discord notices, the Windrose
  invite-code join and the Palworld crash re-test (all in C below). Done: crash-loop detection against a container
  that really exits (one alert, to a webhook), and Minecraft backup and restore on a real world (2026-11-16).

#### P2: reliability, tests and release hygiene

Done (R15 to R23); see `CHANGELOG.md`.

#### P3: features, in build order

Done (passkeys, the Allow switch); see `CHANGELOG.md`. What is left of them is the two real-world checks in P1.

#### P4: larger design work

- [ ] **Tier 2 adapter-sidecar contract** (track A, Phase 2), when a module needs it.

### A. Modular games (open)

- [ ] **Phase 2 leftovers:** verify the Windrose log markers on a graceful quit; the Tier 2 adapter-sidecar
  contract is deferred until a module needs it.
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

- [ ] **Re-test crash detection on a real Palworld.** Its init script exits 0 when the server dies, so the Docker 29
  fix may now catch it (crash-loop detection itself was checked on a real Minecraft, 2026-11-17). Prod `palworld` is
  stopped on purpose until someone wants to play, so this needs it started first.
- [ ] **Windrose invite-code join** from the LAN and from outside, on the macvlan setup.
- [ ] **Valheim crossplay join code and player names** on a real server with `CROSSPLAY=1` (the log patterns are from
  the known format, not a capture).
- [ ] **Idle/night auto-stop and Discord notifications** (backlog 2026-11-17: the real Discord webhook check) against real servers. Webhook notices are verified on the
  test stack (start, stop, backup, restore, crash loop, offsite) but the Discord channel itself is not: it only
  accepts discord.com addresses, so it needs a real webhook. A game gets a fixed one-hour grace until a player has
  been seen, which makes this slow to test; the test stack that held the earlier idle-stop attempt was torn down on
  2026-11-17, so run it again. Check the firewall for `:5001` and game ports.

### Backlog (parked, come back to it later)

- [ ] **`files:` entries for the Source, Don't Starve Together and 7 Days to Die modules.** Parked by the owner on
  2026-11-17. The modules live in the modules repository; confirm each config path against the real image first.
- [ ] **Discord slash commands** (track B). Parked by the owner on 2026-11-12. Needs a Discord application and its
  public key before it can start.

### Considered, not doing

- Go `plugin` package or in-process plugins: they would run inside the panel's trust boundary. WASM (wazero)
  stays an option once the adapter contract is stable.
