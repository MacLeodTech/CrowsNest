# Live checks

Things the test suite cannot prove, to run by hand against real servers after an upgrade. Each step says what you should
see. If a step fails, note the exact message and the time, then look at `docker compose logs crowsnest` (and
`dockerproxy-crowsnest` for anything the guard refused) around that time.

## 0. Before you start

Run `docker compose exec crowsnest crowsnest doctor` first. It checks Docker access, that each game's container
exists, and which notification channels are on (a webhook variable that is named in `config.yaml` but empty in the
container is the usual reason a notice never arrives). Fix what it flags before going further. The list below is what it
does not cover.

- **Version.** `docker compose images crowsnest` shows the tag the container runs. Compare it with the latest release.
- **Containers exist.** CrowsNest only starts containers that already exist; it never creates them, and doctor flags a
  missing one. A game that is missing fails Start with
  `container "<name>" does not exist: create it on the host, or add the game from the dashboard`.
- **Guard and exec rules.** `docker compose ps` shows the guard and execguard healthy. Backups and file reads run
  through exec, so a game whose backup is refused usually means the exec rule or the guard scope is missing.
- **Memory.** Free RAM is enough for the game; the dashboard refuses a start that would not fit and says why.

## 1. Start, stop, backup (through the guard)

Use the game you run for real, for example Palworld.

1. **Start.** Press Start. The card goes to *starting*, then *online* once the game answers (Palworld: its REST API).
   Player count and stats appear.
2. **Backup.** Open Backups and take one. The new file appears in the list with a size and time. For a game with a
   backup folder, check it on the host as well.
3. **Stop.** Press Stop. The card goes to *stopping*, then *offline*. If `before_stop` is set, a backup is taken first.
4. **Cooldown.** Press Start again straight away. It is refused with a wait time (the per-game "restart wait"), and
   works once that has passed.

What a failure means:

| You see | Likely cause |
|---|---|
| `does not exist` on Start | the container is missing from the host; see [troubleshooting](troubleshooting.md#start-fails-container-does-not-exist) |
| Start refused with a memory message | not enough free RAM for the declared size |
| Backup fails with a refused command | the guard scope lacks `"exec": true`, or execguard has no rule for it |
| Stays *starting* forever | the game's status probe cannot reach it (REST port, password) |

Config-file editing exists only for module games, not for games set up in `config.yaml`, so there is no file-edit step
for Palworld.

## 2. Idle auto-stop and the Discord notice

Needs the webhook variable set (doctor's Notifications section says whether Discord is on; the panel itself does not
show it).

1. In Settings, set the idle timeout low (for example 2 minutes) for the game you are testing.
2. Start the game and leave it empty.
3. After the timeout it stops by itself, and the channel gets a notice saying so. The activity log has the entry.
4. Put the timeout back afterwards.

A game whose player count cannot be read is never stopped automatically; if nothing happens, check the player count
on the card first.

## 3. Passkeys

Needs `passkeys` set in `config.yaml` (see [accounts](accounts.md#passkeys)); the values are fixed, so do not change
them once passkeys exist.

1. Account page: add a passkey (Windows Hello, a phone, or a security key). Name it.
2. Sign out, then sign in with the passkey alone. It counts as both steps.
3. Repeat from a phone, and with a security key if you have one.
4. Remove a passkey you no longer want and confirm it can no longer sign in.

## 4. Start-only players

1. Users tab: edit a player and tick a game under "start but not stop". (Or `crowsnest set-start-only <user> <game>`.)
2. Sign in as that player. The game's card has Start but no Stop button.
3. As that player, start the game: it works. Switching away from it or stopping it is refused with
   "You can start this game but not stop it."
4. The idle auto-stop still stops it, since it is not a person's action. Untick the box to lift the limit.

## 5. Not yet covered

Windrose invite-code join, Valheim crossplay join code, and crash-loop detection against a container that really
crashes need those servers; they stay on the [roadmap](../roadmap.md).
