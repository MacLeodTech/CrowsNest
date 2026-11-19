# Troubleshooting

Start with `docker compose exec crowsnest crowsnest doctor` (Docker access, permissions, policy, each game's
container) and `docker compose logs crowsnest`. Dashboard toasts are short; the log line has the full reason.

## Start fails: container does not exist

The dashboard says `<Game> failed to start (see the CrowsNest log for details)` and the log has:

```
exclusivity: start <id> failed: dockerctl: container "<id>" does not exist: create it on the host, or add the game from the dashboard
```

CrowsNest only starts and stops containers that already exist. It does not create one on Start. The container went
missing while the game still shows on the Games page as `ready` (its data folder is untouched). Typical causes: a
`docker rm`, `docker compose down`, or `docker system prune` / `docker container prune` on the host, or a game that was
added in `config.yaml` and never created.

**Games added from the dashboard:** on the Games page, press **Update image** on the game's card. With no container
present it skips the "nothing new" shortcut and creates the container from the saved settings and pinned image. The
data folder and backups are kept. Then press Start. If it fails, the card shows the error. For a policy refusal, fix
`policy.yaml` and restart the guard, then press **Retry install** (the same button when the install failed).

**Games defined in `config.yaml`:** create the container on the host without starting it:
`docker compose --profile <game> up --no-start`.

To stop it happening: do not prune stopped containers on a host that runs CrowsNest games. They are stopped most of
the time by design, so a prune removes them.

## Other common messages

| You see | Likely cause |
|---|---|
| Start refused with a memory message | not enough free RAM for the game's declared or measured size; see Memory in [operating](operating.md) |
| Start refused, wait time shown | the per-game restart wait has not passed |
| Start refused, another game is running | `max_active_games`; use Switch or raise it |
| Stays *starting* forever | the game's status probe cannot reach it (REST port, password) |
| Backup fails with a refused command | the guard scope lacks `"exec": true`, or execguard has no rule for it |

## Adding, editing and updating games

| You see | Cause and fix |
|---|---|
| `outside the operator's policy (add it to policy.yaml to allow it)` | The module asks for something `policy.yaml` does not allow (a port, network, address, `exec`, an unlisted image). Add it to `policy.yaml`, restart the guard, then **Retry install**. `crowsnest doctor` reads the policy. |
| Install `failed` on the card | Fix the cause (usually the line above, or a failed image pull), then **Retry install**. The card shows the last part of the error. |
| `stop the game before changing it` / `before disabling it` | Settings, Update image, Roll back and Disable need the game stopped. |
| `the game is starting or stopping; wait for it to finish` | An action is still in flight. Wait, then try again. |
| `address <ip> on network <net> is already used by <game>` | Two games want the same fixed address. Pick another. |
| `<game> uses that folder: choose another data folder` | Two games share a data folder. Set a different one under More settings. |
| `option "<key>" is required` | A module option with no default was left empty. |
| `id must be 2-31 lowercase letters...` / `id may not end in -clean / -files / -restore` | Those suffixes name the helper containers, so a game cannot use them. |
| `there is no previous image to roll back to` | Roll back needs an earlier digest. It appears after an Update image that brought a new image. |
| `... has no registry digest for ... (built locally?)` | A locally built image cannot be pinned, which `pin_digests: true` requires. Push it to a registry. |
| Update image says "already up to date" | Nothing new upstream, so the container was left alone. This is correct. |
| `this module is not signed by a key you trust` | The dashboard installs only signed modules. Use a signed address, or the command line (`crowsnest module install`), which can install an unsigned one. |
| `module "<id>" is not installed` after an upgrade | The module file is missing from the modules folder. Reinstall it, then restart CrowsNest. A game whose module failed at start only recovers after a restart. |

## Players cannot connect

The panel being online does not mean the game is reachable. Check, in order:

1. The card shows *online*, not *starting*. Windrose and Palworld take minutes after the container starts.
2. The address on the card is the one players use. For the access options (Tailscale, invite codes, why an HTTP tunnel
   cannot carry game traffic) see [connecting](connecting.md).
3. The game's port is reachable for them. On Tailscale, friends should have only the game's port allowed in the
   access policy (see [connecting](connecting.md)); a port the policy does not allow is not published at all.
4. Windrose joins by invite code. The LAN join is tested; a join from outside the LAN is still open in the
   [roadmap](../roadmap.md).

## Backups and restore

| You see | Cause and fix |
|---|---|
| Banner: the backups folder is refused | The folder lost its marker, usually an unmounted drive that Docker replaced with an empty directory. Mount the drive, restart Docker and CrowsNest. To accept a different drive on purpose, delete `backups-root` next to `settings.json` and restart. |
| A new folder is refused on first use | Set `platform.backups_confirm_new: true` once you have checked the right drive is mounted, then restart. |
| `the backups folder is almost full` | Under 1 GiB free. Prune or move backups; `doctor` warns under 5 GiB. |
| `permission denied` in a backup | The game's user cannot write its backup folder. Chown it to that user, run the game as uid 1000, or set `backups_dir_mode: "0777"`. |
| `the backup command finished but no new file appeared` | The command exited 0 but wrote nothing. Check its path and the game's backup folder mount. Old backups are not pruned when this happens. |
| Backups vanish after recreating the panel | `backups_mount` is not a bind mount, so the folder lived inside the container. `doctor` warns about this. |
| Restore refused: it would delete its own file | With `keep: N`, the safety backup would push that file out. Restore a newer one, or raise `keep`. |
| Restore left the game stopped | It failed part way or the restart was refused (for example the active-game limit). Restore the newest file (the safety copy) again, then start the game. |
| Restore asks "Restore without a backup" | The game was stopped, so there was nothing to take a safety copy from. Confirm only if you do not need the current world. |
| The offsite copy is not updating | See the offsite section of [backups](backups.md): it copies without syncing and refuses a folder with no marker, so a missing drive stops it. |

## The file editor

- **Save refused with a 409:** the file changed since you opened it (the game rewrote it, or another admin saved).
  Reload and make the change again.
- **Save refused with a syntax error:** `.json`, `.yaml`, `.yml` and `.xml` files are checked, and the message names
  the line and column.
- **A change did not stick:** some games rewrite their files on shutdown. Edit while the game is stopped, or use
  **Save and restart**.
- **Windrose's preset flipped to Custom:** the game does that on its next start when a value differs from the
  preset. It is expected.
- **Went back to an older version:** the last 10 versions are in `file-history/`. Load one and Save. A hidden secret
  keeps its current value, not the old one.

## Signing in

| You see | Cause and fix |
|---|---|
| Locked out of password sign-in | Run `docker compose exec crowsnest crowsnest set-password <user>`. It applies within seconds. |
| Locked out after turning password sign-in off | The switch warned who would be locked out. Sign in another way (Google or a passkey) and turn it back on in Settings. |
| A user lost their authenticator phone | An admin resets their app from Users. That signs them out. They can also use a one-time recovery code. |
| `five wrong codes` lock | The account is locked for 15 minutes. |
| `the client secret ... can't be decrypted (master key changed?)` | The vault key changed or is missing. Enter the secret again in Settings. The key is not in a state export, by design. |
| Signed out unexpectedly | A session ends after 3 hours without activity (`server.session_idle_minutes`), and an admin revoking a user's sessions ends them at once. |
| `no setup code` on first run | Read it from `docker compose logs crowsnest`, or from `setup-token` next to the session secret. The page closes once an account exists. |

## Crashes, idle stops and the host

- **The card says the game is crash-looping:** Docker keeps restarting it. Read the game's own logs
  (`docker logs <id>`) and check the data folder permissions and free disk space.
- **A game stopped by itself:** idle auto-stop (nobody on for the timeout), the overnight window, or a Switch to
  another game. The activity log says which.
- **Start refused, not enough memory:** the host's free RAM minus the reserve (`server.memory_reserve_mb`) is less than
  the game's measured or declared size. Stop other containers, or lower the reserve. Negative turns the check off.
- **Start refused, another game is running:** `max_active_games` is 1 by default. Use Switch, or raise it.
- **Metrics or `crowsnest doctor` report a problem you cannot place:** run `doctor` first; it names the file or
  setting to fix.

## Still stuck

Collect `docker compose logs crowsnest`, the guard's logs (`docker compose logs <guard service>` shows `DENY` lines
with the reason), and `crowsnest doctor` output. Report security problems through [SECURITY.md](../SECURITY.md).

More checks to run after an upgrade: [live-checks](live-checks.md).
