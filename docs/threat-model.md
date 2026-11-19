# Threat model

What CrowsNest defends, against whom, and where each defence lives. It is written for the operator deciding how far
to trust it, and for anyone reviewing a change. It is a description of the design, not a proof: the guard has had a
review and fuzzing (2026-11-10) and the rest was read, not audited. Report a gap through [SECURITY.md](../SECURITY.md).

## What is being protected

1. **The host.** The panel's whole purpose is to run containers, so the worst outcome is a path from the browser
   or a game container to the host's Docker socket.
2. **Secrets:** the session secret, the vault key (`master.key` or `CROWSNEST_MASTER_KEY`), sign-in client secrets,
   game passwords, webhook URLs, passphrase hashes.
3. **Availability of the game server** for the people who play on it, and the data in it (worlds and backups).
4. **The record of who did what** (the activity log and the audit file).

## Trust boundaries

```
 browser ──HTTPS──▶ reverse proxy ──▶ panel ──(internal network)──▶ guard ──▶ /var/run/docker.sock
                                        │                                         │
                                        └── modules (module.yaml) ──▶ game containers (own networks)
```

| Boundary | Who is on each side | What enforces it |
|---|---|---|
| Browser → panel | Anyone who can reach the port vs. a signed-in admin or player | Sign-in (passphrase, Google or OIDC, optional TOTP); a signed session cookie (`HttpOnly`, `SameSite=Lax`, `__Host-` and `Secure` when `secure_cookies` is on) plus a server-side session list; CSRF token on every state change; per-address and per-user sign-in throttling; CSP with a nonce and the other security headers; two roles, with admin-only routes behind `RequireAdmin`; sessions are renewed only up to an absolute limit |
| Panel → guard | A web-facing process that may be compromised vs. the Docker API | The panel never mounts the Docker socket. It reaches the guard over an `internal` network (set `GUARD_ALLOW_FROM` so only the panel may connect), and the guard allows only the containers, images, networks, binds, ports and security options its scope lists (named container, no privileged mode, no host binds outside the game folder) |
| Guard → Docker | A filter vs. the daemon | The guard is the only container with the socket. `policy.yaml` is mounted read-only into it and **not writable by the panel**, so what the dashboard can create is bounded by something the dashboard cannot edit. A managed scope that does not fit the policy is dropped |
| Panel → game containers | The panel vs. code it did not write | Game containers get their own networks; `exec` is refused unless the policy turns it on and the command is one a game's module declares for backups or for reading and writing its declared config files (the exec allowlist runs in the guard's process) |
| Module sources → panel | Third-party game definitions vs. the engine | A module is data (`module.yaml`), not code. A URL install must be signed by a key in `managed.trusted_keys`, which lives in `config.yaml` and not in anything the dashboard edits; the command-line install is pinned to a hash. A module cannot widen the policy: it can only ask for what the policy already admits |
| Panel → outside | Webhook receivers, OAuth providers, the module host | Outbound only. Webhook URLs come from the environment, never from `config.yaml`, and are redacted from logs and errors |
| Disk → attacker | Files at rest | Secrets in the settings and game files are sealed by the vault (a key that is not on the same disk as the data if you set `CROWSNEST_MASTER_KEY`); the state snapshot never contains the vault key or the session secret |

## Threats by STRIDE category

Reviewed 2026-11-19 by reading the guard, exec guard, sign-in, sessions, middleware, setup, OIDC validation, managed
policy, `gamefiles`, `restore` and the module fetch client, and by searching the dashboard JavaScript. Not read in
depth yet: `exclusivity`, `rcon`, `probes`, `backup/offsite`, `passkey`, `mfa`, `vault`, the OIDC token exchange.

### Spoofing

| Threat | Answer | Residual risk |
|---|---|---|
| Guessed or stolen password | bcrypt (cost 12, upgraded at sign-in), throttling per address and per user, optional TOTP or passkey | A password alone is still enough unless the admin enrols a second factor; `crowsnest doctor` names admins without one |
| Someone locks a user out on purpose | The per-user throttle does not apply to an address that user signed in from in the last 30 days (kept in `known-addrs.json` next to the settings file) | From an address that user has not signed in from, a flood of wrong passwords against one name can block that name's password sign-in for the window; passkey and Google sign-in are unaffected |
| Forged session cookie | HMAC signature plus a server-side session list: a cookie whose id is not in the list is refused, so even a leaked `session_secret` mints nothing | |
| Forged identity from Google or OIDC | PKCE (S256), state and nonce cookies, signature, issuer, audience and nonce checks, a verified email, first sign-in bound to the provider's `sub` | The token exchange has not been read line by line |
| Spoofed client address to dodge throttling | `X-Forwarded-For` is honoured only from `trusted_proxies`, read right to left | An empty `trusted_proxies` behind a proxy makes everyone share one address; `doctor` warns |
| Cross-site login (signing a victim into the attacker's account) | `/login` and `/login/mfa` require `Content-Type: application/json`, which a cross-site form cannot send | |
| First-run takeover | Setup code (96 bits, 0600 file, rate limited, deleted after use) | Anyone who can read the container log can take over a panel that has no users yet |

### Tampering

| Threat | Answer | Residual risk |
|---|---|---|
| Compromised panel creates a privileged container | The guard checks every field of a create body, refuses unknown ones, rebuilds the request, resolves container references once; fuzzed | See "planting files on the host" below |
| Panel widens its own policy | `policy.yaml` is mounted read-only. The optional dashboard edit can only narrow (every list entry must already be in the file, compared exactly), and cannot touch exceptions, trusted keys or `max_memory` | `policy_editable` needs a writable policy folder, which a compromised panel could write directly: leave it off |
| Planting files on the host | A scope leaves `User`, `Entrypoint` and `Cmd` free, so a compromised panel can run as root in an allowed image against an rw bind and write a setuid-root file into the games folder | Mount the games and backups disks `nosuid,nodev` (`doctor` checks what it can see). Do not run anything from those folders on the host |
| Malicious module | Data only, signed or hash-pinned, cannot exceed the policy; exec rules come from installed modules, never from the state file the panel writes | A signed module may ask for anything the policy already admits, including listing an admin password under `connect:`, which every signed-in player sees |
| Upstream image changes | `pin_digests: true` makes a managed create name its image by digest | Off by default; `doctor` warns |
| Bad archive restored | The restore runs in a one-shot container with no network and only that game's folders | A bad archive can corrupt that game's own data |
| Path tricks in the file editor | Files are chosen by id; paths come from the module and are validated; edits run inside the game's container | |
| Request smuggling at the guard | Strict parser: no chunked bodies, no `%`, `;` or `#` in paths, no repeated query keys | |

### Repudiation

| Threat | Answer | Residual risk |
|---|---|---|
| Denying an admin action | Activity log and a hash-chained audit file; a step-up password check on user, sign-in, module and policy changes; a notice for a sign-in from a new device | |
| Rewriting the audit file after a break-in | The hash chain, and an **audit anchor** (entry number and hash) posted daily and at start through the notification channel, where the panel cannot reach it; `crowsnest audit verify --anchor N:HASH` checks it | Without a notification channel the whole file can be rewritten; entries since the last anchor can be cut off |

### Information disclosure

| Threat | Answer | Residual risk |
|---|---|---|
| Secrets in game logs | Logs are admin-only; webhook URLs are redacted | |
| Secrets at rest | Sealed by the vault; the state snapshot leaves out the keys | Set `CROWSNEST_MASTER_KEY` so the key is not on the same disk |
| Earlier versions of edited files | Stored un-masked in the history folder (admin only, 0600) | Roadmap |
| Game passwords on the dashboard | A module's `connect:` entries (join password, invite code) are shown to every signed-in player on purpose. Admin and RCON passwords are not: the generated ones appear only in the admin's games list | A module author decides what `connect:` lists; read it before installing |
| Script injection in the dashboard | CSP with a nonce, `frame-ancestors 'none'`, `HttpOnly` cookie, and none of the dashboard's 4,500 lines of JavaScript use `innerHTML`, `insertAdjacentHTML`, `outerHTML`, `eval` or `document.write` | |
| Server-side request forgery from module fetches | A dial-time check refuses private, loopback, link-local and carrier-grade addresses, no proxy, redirects re-checked | |
| The same from OIDC discovery | Refuses odd addresses only; LAN and loopback are allowed on purpose, so an internal identity provider works | An admin can aim the panel at a LAN address; an admin can already do more than that |
| Guessing the `/metrics` token | Constant-time compare; wrong tokens are throttled per address; `doctor` warns on a short token | |

### Denial of service

| Threat | Answer | Residual risk |
|---|---|---|
| A container with no memory limit | `max_memory` in the policy: the guard refuses a create with no limit or one over the ceiling, and bounds swap, shm and (through the limit) tmpfs | Opt-in. The panel's own admission check still applies first |
| Growing the sign-in throttle list | A key keeps at most `max` entries | |
| Slow request bodies on the panel | `ReadHeaderTimeout` only | Roadmap: per-request deadlines |
| Anything else on the guard's network | `GUARD_ALLOW_FROM` closes other clients | Unset by default |
| Notification channel failure hides an incident | Retries, then a dashboard banner | |

### Elevation of privilege

| Threat | Answer | Residual risk |
|---|---|---|
| A player acts as admin | Server-side role checks on every route (admin GETs and writes both); none was missing from the route table | |
| Panel to host | No socket in the panel; the guard confines what it may ask Docker; exec runs only exact commands | Planting files on the host (above) |
| Game container to host | Capabilities dropped, no-new-privileges, per-game networks, no bind outside the games folder | A kernel or runtime escape is out of scope; a game that can write a symlink in its own data folder is on the roadmap |
| Supply chain (CrowsNest's own image) | CI builds, scans (Trivy), SBOM and provenance, cosign signature, Actions pinned by commit | Verify the signature yourself (`cosign verify`, see [publishing](publishing.md)) |

## Out of scope

These are assumptions, the same as in [SECURITY.md](../SECURITY.md):

- An attacker who already controls the host, the Docker socket or `policy.yaml` has won.
- Running on the open internet without HTTPS, or with the raw Docker socket mounted into the panel (`crowsnest
  doctor` warns about both).
- Vulnerabilities inside a game server image.
- Players' own devices and the game's own protocol security (see [connecting](connecting.md) for why the panel
  does not proxy game traffic).
