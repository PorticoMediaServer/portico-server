# 09 — Running the server: install, setup, configuration, networking, the owner console, backups, updates and resources

Audit date: 11 October 2026. Static source review only: nothing here was run, built or reproduced. Portico paths are relative to `/home/justin/projects/Portico`; comparison paths are written `jellyfin/…`, `jellyfin-docs/…`, `plezy/…`, `navidrome/…`.

## Scope and sources

**What this covers.** Everything an owner touches to run a Portico server rather than to watch something on it: how it is installed and packaged on each operating system, first-run setup, where configuration lives, how the server is reached on the LAN and from outside (ports, TLS, proxies, IPv6, discovery), the owner console (dashboard, activity, logs, tasks, storage), backup and restore, updates and migrations, resource controls, and behaviour when things go wrong (disk full, unclean shutdown, a second instance).

**Portico, read in full or nearly so**

- Docs: `portico-internal/Project Architecture/02 Server.md`, `07 Hosted Services.md`, `08 Operations.md`, `Backlog.md`, `Specs/Spec — Backups (Plex model).md`; Product Contracts `settings-and-operations/contract.md`, `networking/contract.md`, `onboarding/contract.md` (first half), `decisions.md`.
- Packaging and release: `apps/portico-server/README.md`, `INSTALLING.md`, `BUILDING.md`, `RELEASING.md`, `Dockerfile`, `Makefile`, `packaging/linux/*`, `packaging/macos/*`, `packaging/windows/installer.nsi`, `scripts/package-release.sh`, `scripts/package-macos.sh`, `scripts/write-public-update-manifest.mjs`, `scripts/container-release.mjs`, `.github/workflows/release.yml`.
- Process: all of `server/cmd/server/` except tests (`main.go`, `startup.go`, `drain.go`, `instance_lock*.go`, `health_check.go`, `transport_policy.go`, `connection_cap.go`, `console.go`, `maintenance.go`, `networking_initializer.go`, `administration.go`, `recover_owner.go`, `analysis_initialization.go`, `media_sources.go`).
- Packages read in full: `internal/backup` (all nine files), `internal/servicelog`, `internal/hostlimits/hostlimits.go`, `internal/diskspace`, `internal/workpolicy`, `internal/supervise/supervise.go`, `internal/connectivity/discovery.go`, `internal/persistence/{schema,migrations,inspect,rebase}.go` and its README.
- Packages read in part (the paths that matter here): `internal/networking` (`mapping*.go`, `wan.go`, `topology.go`, `remote_manager.go` first 520 lines, `remote_store.go`, `certificate_listener.go`, `certificate_serving.go`, `certificate_crypto.go` renewal, `custom_certificate.go`, `discovery.go` head), `internal/hosted` (`events.go`, `current_schedule.go`, `trust.go`), `internal/operations` (`settings.go`, `jobs.go` schedules and scheduler loop, `schedule_wake.go`, `notifications.go` maintenance), `internal/administration` (`update_feed.go`, `maintenance.go` updates, `deletion.go` trash), `internal/httpapi` (`admission.go`, `route_lanes.go`, `backup_routes.go`, `request_locality.go`, `onboarding.go`, `secure_connections.go`, `rate.go`, `installation.go`, `admin_concurrency.go`, `write_deadline.go`, `compression.go`), `internal/dbwork` (`policy.go`, `wal.go`), `internal/mediatools/resolve.go`, `apikit/registry.go`.
- Clients: `packages/client-core/src/presentation/settings-structure.ts` (the authored server-settings structure), `web/src/screens/server/MaintenanceWindows.tsx`, the en-US i18n catalogue for the console copy.

**Comparison sources read**

- Jellyfin server: `Jellyfin.Server/Program.cs`, `StartupOptions.cs`, `Migrations/JellyfinMigrationService.cs` (backup-before-migrate), `Jellyfin.Server.Implementations/FullSystemBackup/BackupService.cs`, `StorageHelpers/StorageHelper.cs`, `MediaBrowser.Common/Net/NetworkConfiguration.cs`, `MediaBrowser.Model/Configuration/ServerConfiguration.cs`, controllers `Startup`, `System`, `Backup`, `Configuration`, `ScheduledTasks`, `ActivityLog`, `Branding`, `ApiKey`, `Devices`, `ClientLog` (route lists), `Extensions/ApiServiceCollectionExtensions.cs` (forwarded headers), `Startup.cs` (metrics, health), middleware list, scheduled task list.
- Jellyfin web: `src/apps/dashboard/routes/**` and `features/**` (page inventory).
- Jellyfin docs: `docs/general/installation/**` (container, Windows, Linux), `post-install/networking/index.md` plus the reverse-proxy, Tailscale and advanced file lists, `administration/configuration.md`, `backup-and-restore.md`.
- Plezy: `plezy/server/` (the small Go relay: `main.go` serve/shutdown, `client_ip.go`, `Dockerfile`, `docker-compose.yml`). Plezy is a client, so it has little to say here beyond how it deploys its own relay.
- Navidrome: `conf/configuration.go` defaults, `db/backup.go`. Audiobookshelf: environment variables and backup settings in `server/objects/settings/ServerSettings.js`.
- Plex: from memory of its public support articles only (remote access, relay, `plex.direct`, scheduled tasks). Not re-verified on the web in this pass; treat as background.

**Not reached.** `internal/networking` claim and certificate order state machines beyond the parts listed (about 9,000 lines), `internal/telemetry` collectors, `internal/capabilityreport`, `internal/notify` producers beyond the two used here, `internal/claimweb`, most of `internal/administration` (libraries, live sources), the web console screens other than the one named, the React Native apps, and the Hosted service code. Jellyfin's Windows tray installer and macOS app live in separate repositories that were not cloned.

## How Portico does it today

**One binary, one folder.** The server is one statically linked Go program (`cmd/server/main.go`). The same file is also its own helper: it re-runs itself for the sandboxed FFmpeg wrapper, the rclone helpers, a `recover-owner` command, `--version` and `--health-check` (`main.go:71-131`). Everything the server owns lives in one state folder named by `PORTICO_STATE_DIR`: `server.sqlite` (SQLite in WAL mode), `logs/`, `backups/`, `hls/` (conversions), `artwork/`, `subtitles/`, `recordings/`, `mounts/`, `trash/`, `downloads/`, `supervisor/authority-incarnation.v1`, `networking-keys`, `networking-tls.pem`, and a `server.lock` file.

**Startup order** (`main.go:200-1067`): raise the open-file limit and set a Go memory limit from host or cgroup memory (`hostlimits.Apply`) → bind the TCP listener first, serving an honest "starting" answer with named phases (`startup.go`) → take an advisory lock on `server.lock` so two servers never share a folder (`instance_lock*.go`) → apply any staged restore → open and migrate the database → wire about forty services and background loops → swap in the real router → serve. Every goroutine is started through `supervise.Go`, which contains panics and restarts database-backed loops with backoff.

**Shutdown** is `SIGINT`/`SIGTERM` → cancel the root context → `http.Server.Shutdown` with a 10 s budget in a side goroutine → one "drain barrier" that stops every loop under a shared 15 s budget and names any that did not finish (`drain.go`) → close the database. Exit code 75 means "start me again" and is used after a restore is staged.

**Configuration** has two surfaces and no config file:

1. About forty `PORTICO_*` environment variables read at start (listed in A-09.10). The ones an owner might touch are `PORTICO_STATE_DIR`, `PORTICO_BIND` (default `0.0.0.0:32500`), `PORTICO_WEB_DIR`, `PORTICO_RECORDINGS_DIR`, `PORTICO_TRUSTED_PROXY_CIDRS`, `PORTICO_DISCOVERY`, `PORTICO_FFMPEG`/`PORTICO_FFPROBE`, `PORTICO_DECODER_SANDBOX`, `PORTICO_INTEGRITY_CHECK`, `PORTICO_ALLOWED_ORIGINS`.
2. One revisioned settings document in SQLite, edited in the web console, published through a field registry that states each field's type, default, range and whether it applies hot, on the next operation or after a restart (`operations/settings.go:65-113`). Saves carry an expected revision and an idempotency key.

**Packaging** (`packaging/`, `scripts/package-release.sh`):

| Target | What is installed | How it runs |
|---|---|---|
| Linux `.deb`/`.rpm` (x64, ARM64) | `/usr/lib/portico-media-server` (binary, FFmpeg bundle, web app, signed manifest); state in `/var/lib/portico-media-server` | systemd unit as user `portico`, `ProtectSystem=strict`, `Restart=on-failure` |
| Linux `.tar.gz` | The same payload, nothing else | Whatever the owner arranges |
| Windows `Setup.exe` (x64, ARM64) | `Program Files\Portico Media Server`; state in `%ProgramData%\Portico Media Server` | A Scheduled Task at boot, as `SYSTEM`, running `start-portico.cmd` |
| Windows portable `.zip` | The payload | Whatever the owner arranges |
| macOS `.dmg` (Apple Silicon) | `Portico Media Server.app`; state in `~/Library/Application Support/Portico Media Server` | Opening the app installs a per-user launchd agent and opens the browser |
| Docker (amd64, arm64) | `debian:bookworm-slim`, user `portico`, `tini`, bubblewrap, bundled FFmpeg; one volume `/var/lib/portico` | `HEALTHCHECK` calls the binary's own `--health-check` |

Releases are built from a sealed source snapshot, each binary carries a signed manifest, and the release publishes checksums, an SBOM and build provenance. The OS packages themselves are unsigned.

**First run.** The server prints the LAN address to open. Any browser that can reach it may start setup (a recorded decision, `httpapi/onboarding.go:141-144`): the page asks the server for a setup token (same-origin check and a rate limit), then the owner picks a server name, a username and password, and one of two sign-in modes: a Portico Account (the server is then "claimed" through Hosted) or direct sign-in with no Hosted dependency.

**Networking.**

- One port, 32500, carries both HTTPS and plain HTTP. `networking.DirectListener` looks at the first byte of each connection: a TLS hello is handed to TLS; anything else is accepted as plain HTTP **only** when the peer address is loopback, private (RFC 1918 / IPv6 ULA) or link-local; otherwise the socket is closed (`certificate_listener.go:155-184`).
- A claimed server gets a wildcard certificate for a name under `direct.getportico.tv`, issued through Hosted by ACME DNS-01, renewed at a per-order random point between 58% and 76% of its life (`certificate_crypto.go:143-153`). An owner can instead (or also) point at their own PEM certificate and key for one custom domain; it is hot-reloaded.
- Remote access, once switched on for a claimed server, is automatic where the router allows it: the server tries PCP, NAT-PMP and UPnP IGD port mappings and, for IPv6, IGDv2 firewall "pinholes" (`mapping*.go`), learns its WAN address from the router or else from public STUN and "what is my IP" services (`wan.go`), and publishes route changes to Hosted only when they change. A manual port forward and extra "access URLs" (a reverse proxy or VPN name) are also supported.
- LAN discovery is a hand-written mDNS/DNS-SD responder for `_portico._tcp` (`networking/discovery.go`).
- Requests are classed local or remote from the socket peer, or from `X-Forwarded-For` when the peer is a configured trusted proxy (`httpapi/request_locality.go`). Remote requests can be limited: who may sign in remotely, a remote bitrate ceiling, and an upload-capacity budget.

**The owner console** is the web app's Server section (structure authored in `client-core/src/presentation/settings-structure.ts`): Dashboard (now playing, alerts, health, activity, statistics), Play history, Libraries, Live TV, People (accounts, invitations, API keys), Playback (transcoding status, settings and limits), Remote access, Storage (disk usage, retention, remote sources, backups, deleted titles), Schedule (jobs, maintenance windows, background priority), Troubleshooting (detail window, logs, records, capabilities, support export, feedback, state folder) and General (name, updates, alert thresholds). About 150 owner routes sit behind it.

**Logs** are the standard library logger teed to stderr and to `servicelog.Recorder`: an in-memory ring of 5,000 records for the console and live tail, plus `logs/messages.log` rotated at 4 MiB with four older files kept (`servicelog.go`).

**Backups** follow the "Plex model" spec: a nightly default window creates `backups/<UTC timestamp>/` holding `portico.db` (SQLite's online backup API on a pinned read snapshot, then `quick_check`), a few identity and config files, and a `manifest.json`; the newest seven are kept. Restore validates and copies the chosen backup into `restore-staged/`, then the server exits with code 75; on the next start the current database is moved into a `…-pre-restore` backup, the staged one moves in, migrations run, and any failure rolls back automatically through a five-state marker file (`backup/apply.go`).

**Migrations** are embedded SQL files applied at open, each in one transaction, each recorded with a digest that is re-checked on every start. The rules are additive-only and "never rewrite a large table at startup" (`persistence/README.md`). A database newer than the binary is refused with a plain message.

**Updates** are notify-only: the console can read a release feed the owner configures and say whether a newer build exists. Nothing installs itself.

**Resource controls.** HTTP requests pass through "lanes" with capacities and per-request time budgets (`httpapi/admission.go:70-81`); writes pass through one write gate ordered by work class; accepted connections are capped from host memory; background FFmpeg runs at lower priority (`nice 10`); producers of new files decline to start below 2 GiB free (`diskspace`).

## How the others do it

### Jellyfin

**Process.** A .NET application (`Jellyfin.Server/Program.cs`). It starts a small "setup server" first that shows an HTML startup page with the current activity and, to local viewers, the log, while the real host is built and migrations run; if startup fails, that page stays up for ten minutes with the error (`Program.cs:273-283`). Restart is **in-process**: `StartApp` loops `StartServer` while `_restartOnShutdown` is set (`Program.cs:145-163`), so a restart or a restore needs no service manager. `POST /System/Restart` and `/System/Shutdown` exist and the dashboard has buttons for both.

**Configuration** is files plus flags: `system.xml` (`ServerConfiguration`), `network.xml` (`NetworkConfiguration`), `encoding.xml`, `logging.json` for log levels, command-line flags (`--datadir`, `--configdir`, `--cachedir`, `--logdir`, `--webdir`, `--ffmpeg`, `--published-server-url`, `--restore-archive`, `--nowebclient`) and matching `JELLYFIN_*` environment variables (`jellyfin-docs/docs/general/administration/configuration.md`). The dashboard edits the XML through `POST /System/Configuration`. Network changes generally need a restart.

**Directories** are separate and follow the XDG convention: data, config, cache, log, web, plus configurable transcode and metadata paths. Docker uses two volumes, `/config` and `/cache`.

**Install.** Signed apt repository and install script for Debian/Ubuntu, community packages elsewhere, a Windows installer that sets up a per-user tray app (installing as a service is offered but "not recommended"), a macOS app, the official Docker image (runs as root unless `user: uid:gid` is given; LinuxServer.io's image adds `PUID`/`PGID`), Unraid template in-repo (`jellyfin/deployment/unraid`), and docs for Synology, TrueNAS, Kubernetes and Podman. Hardware acceleration has long per-vendor guides and the Docker image ships Jellyfin's own FFmpeg build with the GPU user-space drivers.

**First run** is a wizard reachable by anyone who can reach the port until it is completed (`StartupController`): display language → first user and password → add libraries → metadata language and country → "allow remote connections" → done.

**Networking.** HTTP on 8096 and, optionally, HTTPS on a second port 8920 with an owner-supplied PKCS#12 certificate; bind address lists; IPv4 and IPv6 toggles (IPv6 off by default); LAN subnets; known proxies (addresses, subnets or host names) that switch on `X-Forwarded-For/-Proto/-Host` handling; a **base URL** for hosting under a sub-path such as `/jellyfin`; "published server URL" overrides per subnet; a remote IP allow-list or block-list; a global and per-user "allow remote access" switch; a remote bitrate limit; optional Unix-socket listening. UPnP port mapping was removed: the `EnableUPnP` field remains in `NetworkConfiguration.cs:114` but nothing in the tree implements it. LAN discovery is a UDP broadcast on port 7359 (`src/Jellyfin.Networking/AutoDiscoveryHost.cs`). Remote access is "bring your own": the docs steer owners to a reverse proxy (guides for nginx, Caddy, Apache, HAProxy, Traefik), a VPN or Tailscale, Let's Encrypt and fail2ban.

**Dashboard pages** (`jellyfin-web/src/apps/dashboard/routes`): Dashboard (server info, restart/shutdown, active sessions with send-message and remote control, alerts, activity, storage paths), Settings (general), Branding (login disclaimer, custom CSS, splash screen), Users (add, edit with per-user access, parental control and remote-access/bitrate limits), Libraries (list, display, metadata, NFO), Playback (transcoding, resume, streaming, trickplay), Devices, Activity, Networking, API keys, Backups, Logs (list of log files and a file viewer), Scheduled tasks (each with editable triggers), Plugins (installed, catalogue, repositories), Live TV (tuners, guide providers, recordings).

**Scheduled tasks** (`*Task.cs`): scan libraries, refresh people, chapter images, keyframe extraction, media segment scan, audio normalisation, trickplay, clean activity log, clean cache, clean logs, clean transcodes, clean user data, optimise database, update plugins. Triggers are daily, weekly, interval or at startup, with an optional time limit.

**Backups** (new in 10.11, `BackupService.cs`): a zip written on request from the dashboard containing every database table exported as JSON, the config files, and optionally metadata, subtitles and trickplay. It refuses to run during a library scan and needs 5 GB free. There is no schedule and no retention. Restore replaces the tables from the archive and restarts in-process; `--restore-archive` does the same from the command line. Separately, the migration runner **backs up the database automatically before applying migrations** and optimises it afterwards (`JellyfinMigrationService.cs:417-492`).

**Observability.** Serilog rolling log files configured by `logging.json`, a separate FFmpeg log per transcode, `GET /health` with a database check, opt-in Prometheus metrics at `/metrics` (`Startup.cs:242-256`), a "slow response" warning threshold, an activity log kept 30 days, and client log upload.

**Resources.** Settings for scan fan-out concurrency, metadata refresh concurrency, parallel image encoding, transcoding thread count and throttling. Jellyfin refuses to start when its data, cache or program-data volume has under 2 GB free (`StorageHelper.cs`).

### Plezy

Plezy is a client. Its only server piece is a small Go relay (`plezy/server/`) for watch-together rooms, log upload and OAuth hand-offs. It is worth a glance as a deployment reference: a `scratch` image with `GOMEMLIMIT=64MiB`, a compose file with `mem_limit`, `stop_grace_period`, bounded Docker log files and images pinned by digest, a required `TRUSTED_PROXY_CIDRS` (the service binds to 127.0.0.1 behind a proxy), and a `main` that waits for `http.Server.Shutdown` to return before flushing state and exiting (`plezy/server/main.go:2682-2704`).

### Navidrome and Audiobookshelf (brief)

- **Navidrome** (Go + SQLite, the closest technical match): one binary with the web UI embedded; a TOML config file with every key also settable as `ND_*` environment variables; `BaseURL` for sub-path hosting; built-in TLS from a cert and key; Unix-socket listening; opt-in Prometheus; reverse-proxy header authentication; scheduled backups by cron expression to a configurable path with a keep count, and `backup create/prune/restore` commands (`navidrome/db/backup.go`); an anonymous "insights" collector that is on unless disabled (`conf/configuration.go`: `enableinsightscollector = true`).
- **Audiobookshelf**: environment variables for paths, port and `ROUTER_BASE_PATH`; backups as zip files on a cron schedule with a keep count (default 2) and a size cap, to a configurable `BACKUP_PATH`.

### Plex (from public documentation, not re-checked here)

Per-user desktop app with a tray/menu-bar icon on Windows and macOS, packages for Linux and most NAS vendors; an unclaimed server can only be set up from the same machine or subnet; remote access by automatic UPnP/NAT-PMP mapping or manual forward, with a bandwidth-limited relay as a fallback; `*.plex.direct` certificates issued per server; LAN networks, "treat WAN IP as LAN bandwidth", upload-speed and per-stream remote limits; scheduled database backups every three days inside a nightly maintenance window, kept beside the database; update channels with in-app download.

## Architectural differences

Sizes are rough: **small** = a day or two in one package; **medium** = about a week across a few packages; **large** = multi-week or a new subsystem.

### Installing and packaging

### A-09.1 One self-contained binary that is also its own helper
- **Portico:** One static Go binary (CGO off) re-executes itself as the FFmpeg sandbox wrapper, the rclone guardian, the storage and subtitle helpers, the owner-recovery tool and the container health check (`server/cmd/server/main.go:71-123`). A deployment installs the binary, the web folder and the FFmpeg bundle, and nothing else.
- **Jellyfin / others:** Jellyfin needs the .NET runtime (bundled in its packages) plus a separate `jellyfin-ffmpeg` package. Navidrome is also one Go binary, with the UI embedded.
- **Why it matters:** Fewer moving parts to install, sign, sandbox and upgrade; a broken install has fewer ways to be half-installed.
- **Assessment:** **Portico ahead.** Deliberate (`02 Server.md`, opening paragraph). Keep it.

### A-09.2 The web app is a folder on disk, not part of the binary
- **Portico:** The web UI is served from `PORTICO_WEB_DIR` through `os.Root` (`server/internal/httpapi/installation.go:33-90`). If the variable is unset there is no fallback: `/` answers 404. FFmpeg, by contrast, is found beside the executable automatically (`internal/mediatools/resolve.go:178-203`).
- **Jellyfin / others:** Jellyfin defaults the web folder to `<binary dir>/jellyfin-web` and has `--nowebclient` to turn hosting off on purpose. Navidrome embeds the UI with `go:embed`.
- **Why it matters:** The packaged installs set the variable, so they work. The portable `.tar.gz` and Windows `.zip` do not, and someone who unpacks one and runs the binary gets a server with no interface (see B-09.27).
- **Assessment:** **Portico behind**, slightly. Either embed the built web app or default to `<binary dir>/web`. Small. Embedding also removes one class of "the web folder and the binary are from different builds" problems; the cost is a larger binary and a slower dev loop, which a build tag solves.

### A-09.3 Windows: a boot-time Scheduled Task running as SYSTEM
- **Portico:** The NSIS installer writes `start-portico.cmd` and registers it with `schtasks /SC ONSTART /RU SYSTEM /RL HIGHEST` (`packaging/windows/installer.nsi:36-45`). There is no Windows service, no tray icon, no firewall rule and no visible "running/stopped" control. A desktop companion app is on the backlog ("Planned features").
- **Jellyfin / Plex:** Both install a per-user tray application by default; Jellyfin offers a service as a non-recommended option. Running as the signed-in user is what lets them read mapped drives and password-protected network shares.
- **Why it matters:** Windows is where the least technical owners are. SYSTEM cannot see the user's mapped drives or use their saved share credentials, FFmpeg decoding untrusted files runs with the highest local privilege, and a Scheduled Task has surprising defaults (B-09.4, B-09.5).
- **Assessment:** **Portico behind.** The current shape reads as a placeholder until the companion app exists. Until then, a real Windows service (Go has `golang.org/x/sys/windows/svc`) running as a low-privilege account with restart-on-failure would remove most of the sharp edges. Medium.

### A-09.4 macOS: a launcher app that installs a per-user background agent
- **Portico:** The `.app` is a shell script (`packaging/macos/launcher.sh`) marked `LSUIElement` (no Dock icon). Opening it writes a LaunchAgent with `KeepAlive`, starts it, waits two seconds and opens `http://127.0.0.1:32500/`. Apple Silicon only, macOS 13+, unsigned unless a signing identity is supplied.
- **Jellyfin / Plex:** Menu-bar apps with start/stop/quit and "open at login"; Jellyfin ships Intel and Apple Silicon builds.
- **Why it matters:** The agent runs only while that user is logged in (same as Plex), there is no visible way to stop or uninstall it, and deleting the app leaves the agent behind (B-09.31).
- **Assessment:** **Trade-off (subjective)** now, **behind** on polish. Covered by the planned companion app. Whether Intel Macs matter is Justin's call; old Intel Mac minis are common home servers.

### A-09.5 Linux: packages with a locked-down systemd unit, but no repository
- **Portico:** `nfpm` builds a `.deb` and `.rpm` whose unit runs as `portico` with `NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=strict`, `ProtectHome=read-only` and a single writable path (`packaging/linux/portico-media-server.service`). Packages are downloaded from GitHub Releases; there is no apt or dnf repository, no `EnvironmentFile`, and the package declares no dependencies.
- **Jellyfin:** A signed apt repository (so `apt upgrade` updates it), a looser unit, `/etc/default/jellyfin` for options, and the post-install adds the service user to the `render` and `video` groups for GPU access.
- **Why it matters:** Portico's unit is safer by default, which is good. The same strictness makes several product features fail unless the owner edits the unit (B-09.14), and without a repository every update is a manual download.
- **Assessment:** **Trade-off.** The hardening is worth keeping, but it has to agree with what the product lets an owner do (delete media, record elsewhere, read a certificate). Adding an `EnvironmentFile=-/etc/default/portico-media-server` line and documented drop-ins is small; a signed repository is medium and belongs with package signing.

### A-09.6 Docker: non-root, an init process, and a built-in health check
- **Portico:** The image creates a system user, runs under `tini` so orphaned FFmpeg grandchildren are reaped, and the `HEALTHCHECK` is the server binary asking its own readiness route, so no `curl` is needed (`Dockerfile`, `cmd/server/health_check.go`). One volume. No `PUID`/`PGID`, no compose example, no GPU notes.
- **Jellyfin:** Root by default with an optional `user:`; two volumes (`/config`, `/cache`); documented compose file with `restart: unless-stopped`, the discovery UDP port, fonts and GPU device/group examples. LinuxServer.io's image is where the `PUID`/`PGID` convention comes from.
- **Why it matters:** Portico's image is the more careful one. What is missing is everything around it that self-hosters copy and paste: a compose file, how to match file ownership on a bind mount, how to pass a GPU through, and the restart policy that restore depends on (B-09.2).
- **Assessment:** **Portico ahead** on the image, **behind** on documentation and conventions. See F-09.1.

### A-09.7 Platform coverage
- **Portico:** Linux x64/ARM64 (deb, rpm, tar), Windows x64/ARM64, macOS ARM64, Docker amd64/arm64 (`.github/workflows/release.yml:83-88`).
- **Jellyfin / Plex:** Add 32-bit ARM (older Raspberry Pi), Intel macOS, and first- or third-party packages for Synology, QNAP, Unraid and TrueNAS. Plex also ships FreeBSD and many NAS-native builds.
- **Why it matters:** NAS boxes are where a large share of home media servers actually run. Most can run Docker, but an Unraid template or a Synology package is what makes a product show up in the place those owners look.
- **Assessment:** **Portico behind** on reach; reasonable for a pre-release. An Unraid template is a small XML file; the rest is medium to large each.

### A-09.8 FFmpeg is bundled, pinned and verified
- **Portico:** Releases carry a specific FFmpeg build with a manifest; the server verifies the bundle before using it and qualifies the toolchain at start, joining that check before any playback service is built (`main.go:309-431`, `mediatools/resolve.go`). FFmpeg is released on its own cadence (`RELEASING.md`).
- **Jellyfin:** Ships its own patched `jellyfin-ffmpeg` as a separate package; owners can point at another FFmpeg.
- **Why it matters:** "Works on my FFmpeg" problems disappear, and a tampered or mismatched FFmpeg is caught at start.
- **Assessment:** **Portico ahead** on integrity. One gap to note: the Docker image carries the FFmpeg binaries but no GPU user-space drivers (B-09.26).

### A-09.9 Release integrity: strong provenance, unsigned installers
- **Portico:** Sealed source snapshot, Ed25519-signed build manifest inside every package, SHA-256 list, SBOM and GitHub build attestation (`release.yml`). The installers themselves are not Authenticode-signed, notarised or GPG-signed, and `INSTALLING.md` says so.
- **Jellyfin:** GPG-signed apt repository; Windows and macOS builds are not fully signed either.
- **Assessment:** **Portico ahead** on provenance, level on signing. Deliberate and documented ("signing … before unattended updates").

### Configuration and data

### A-09.10 Configuration: environment variables plus one database document, no file
- **Portico:** Non-test code reads these variables: `PORTICO_STATE_DIR`, `PORTICO_BIND`, `PORTICO_WEB_DIR`, `PORTICO_RECORDINGS_DIR`, `PORTICO_MANAGED_MOUNT_ROOT`, `PORTICO_MEDIA_ROOTS`, `PORTICO_TRUSTED_PROXY_CIDRS`, `PORTICO_ALLOWED_ORIGINS`, `PORTICO_DISCOVERY`, `PORTICO_FFMPEG`, `PORTICO_FFPROBE`, `PORTICO_REQUIRE_QUALIFIED_FFMPEG`, `PORTICO_DECODER_SANDBOX`, `PORTICO_PREPARED_SANDBOX`, `PORTICO_DECODER_LIBRARIES`, `PORTICO_LINEAR_DECODER_LIBRARIES`, `PORTICO_FINITE_VOD`, `PORTICO_INTEGRITY_CHECK`, `PORTICO_STARTUP_TRACE`, `PORTICO_DBWORK_TRACE`, `PORTICO_DBWORK_STRICT`, `PORTICO_ANALYSIS_MAX_INPUT_BYTES`, `…_MAX_GENERATED_BYTES`, `…_MAX_CACHE_BYTES`, `…_MAX_PREVIEW_FRAMES`, `…_RETAIN_DAYS`, `PORTICO_TMDB_READ_ACCESS_TOKEN`, `PORTICO_ACOUSTID_CLIENT_KEY`, `PORTICO_OPENSUBTITLES_API_KEY`, `…_TOKEN`, `…_USER_AGENT`, `PORTICO_LYRICS_PROVIDER_ORIGIN`, `PORTICO_DATASET_MANIFEST`, `PORTICO_DATASET_PUBLIC_KEY`, `PORTICO_HOSTED_ORIGIN`, `PORTICO_HOSTED_PUBLIC_KEY`, `PORTICO_HOSTED_KEY_ID`, `PORTICO_CERTIFICATE_ENVIRONMENT`, `PORTICO_CERTIFICATE_STAGING_ROOTS`, and three development-only ones (`PORTICO_DEV_LOOPBACK_ROUTE`, `PORTICO_DEV_HOSTED_CA_FILE`, `PORTICO_CHAOS_PANIC`), plus Go's `GOMEMLIMIT`. Everything else is the settings document in SQLite.
- **Jellyfin / Navidrome:** Jellyfin: XML files an owner can read and edit by hand, flags and environment variables. Navidrome: a TOML file where every key is also an environment variable.
- **Why it matters:** Portico's settings live in one place with revisions and validation, which is cleaner than Jellyfin's XML. The cost is that the handful of things that are environment-only (bind address and port, state folder, recordings folder, the first set of trusted proxies) are awkward on every packaged install: a systemd drop-in on Linux, editing a `.cmd` file in Program Files on Windows, editing a plist on macOS that the launcher rewrites when it differs. There is also no documented list of the variables anywhere in the repository.
- **Assessment:** **Trade-off.** The split is sound; the missing piece is one small, documented, per-install file for the environment half (F-09.15) and a reference page.

### A-09.11 Settings are a typed, revisioned registry
- **Portico:** Every server setting is declared once with type, default, bounds, options, application mode and consumer (`operations/settings.go:65-113`); writes carry `expectedRevision` and an idempotency key and return the whole document; unknown fields are rejected.
- **Jellyfin:** `POST /System/Configuration` replaces the configuration object; last write wins; validation is ad hoc.
- **Why it matters:** Two browser tabs cannot silently overwrite each other; a client can render the right control from the registry; an invalid value never lands.
- **Assessment:** **Portico ahead.** Deliberate (settings contract §2.2, §5.4). Three places where the registry and reality disagree are listed as B-09.9, B-09.10 and B-09.28.

### A-09.12 What needs a restart
- **Portico:** Almost everything applies hot or on the next operation. Only `temporaryDirectory` is declared `restart`, and the listen address and port are not settings at all. The document's `restartFields` is always empty (`settings.go:245`).
- **Jellyfin:** Network settings (ports, bind addresses, HTTPS, base URL) apply after a restart, and the dashboard offers the restart.
- **Why it matters:** Portico avoids "restart to apply" almost entirely, which is the better experience. The flip side is that the port cannot be changed from the console at all.
- **Assessment:** **Portico ahead** on hot application; **behind** on being able to change the port or bind address without touching the service definition (F-09.8).

### A-09.13 One state folder versus separate config, data, cache and log directories
- **Portico:** One folder holds the database, identity keys, logs, caches, conversions, artwork, trash, backups, mounts and (by default) recordings. Only recordings and the managed mount root can be moved by environment variable. `02 Server.md` also names `PORTICO_CACHE_DIR` and `PORTICO_BACKUP_DIR`; neither exists in code (see Doc/code disagreements).
- **Jellyfin:** Data, config, cache, log, transcode and metadata paths are each configurable; Docker separates `/config` from `/cache`.
- **Why it matters:** One folder is simple to explain, copy and back up, and matches the "open data" principle. But it puts things with opposite needs on one volume: the database and identity (small, precious, wants a reliable disk and a backup) beside conversions, artwork, trash and recordings (large, disposable or bulky, often wanted on a scratch disk or RAM disk). A full cache then becomes a full database volume. Owners with a small SSD for the system and a large array for everything else are the common case this bites.
- **Assessment:** **Trade-off (subjective)**, leaning behind for larger installs. Deliberate as a default (Spec — Backups §3, §6). Keeping the single folder as the default while letting cache-class subfolders (`hls`, `artwork`, `prepared-media`, `trash`, `backups`) be redirected is medium: each already takes its directory as a constructor argument in `main.go`.

### A-09.14 Moving a server: absolute paths rewritten at start
- **Portico:** Paths are stored absolute. On start, if the recorded state folder differs from the current one, sixteen named columns are rewritten from the old prefix to the new in one transaction (`persistence/rebase.go:26-104`). Media library paths outside the state folder are left alone.
- **Jellyfin:** Moving an install is a documented manual procedure with known path pitfalls (`jellyfin-docs/…/administration/migrate.md`).
- **Why it matters:** "Copy the folder, start it" works on the same operating system, which is better than Jellyfin. The rewrite uses the platform's path separator, so Windows↔Linux moves are not handled (the spec calls that an aspiration). On a multi-million-row `assets` table the `LIKE … ESCAPE` update is a full scan in one write transaction before the server finishes starting; that happens once per move.
- **Assessment:** **Portico ahead** in outcome. The spec says "relative to the state folder, never by absolute path"; the code chose absolute-plus-rebase instead (Doc/code disagreements). Either is defensible; the documents should say which.

### A-09.15 Nothing encrypted at rest, with a permissions warning instead
- **Portico:** By decision, no sealed data and no key files; the server warns in the console when the state folder is readable by other users and offers a fix (`backup/stateperms.go`, Spec — Backups §6).
- **Jellyfin:** Also plain files; no permission check.
- **Assessment:** **Portico ahead** in intent. The check itself has platform problems (B-09.15).

### First run and recovery

### A-09.16 What first-run setup asks
- **Portico:** Server name, owner username and password, and a choice between a Portico Account and direct sign-in. Libraries, language and remote access come afterwards in the console (a checklist is referenced in `Backlog.md`).
- **Jellyfin:** A five-step wizard that also covers display language, first libraries, metadata language/country and whether to allow remote connections.
- **Why it matters:** Portico's first screen is shorter. Jellyfin's leaves the owner with a working library at the end of the wizard.
- **Assessment:** **Trade-off (subjective).** Worth deciding whether "add your first library" belongs in the first-run flow (F-09.29).

### A-09.17 Who can reach an unconfigured server
- **Portico:** Any browser that can connect may run setup (recorded decision, `httpapi/onboarding.go:141-144`). What limits it is the transport: a peer that is not loopback, private or link-local must speak TLS, and an unclaimed server has no certificate, so in a direct deployment only the LAN can connect. The setup token is handed out behind a same-origin check and a rate limit.
- **Jellyfin / Plex:** Jellyfin's wizard is open to anyone who can reach the port, with no transport restriction. Plex only lets an unclaimed server be set up from the same machine or subnet.
- **Why it matters:** Portico sits between the two. Its protection depends on the socket peer address being honest, which stops being true behind Docker's userland proxy, a TCP tunnel, or a reverse proxy that does not add `X-Forwarded-For` (B-09.24).
- **Assessment:** **Portico ahead** of Jellyfin, roughly level with Plex in direct deployments. Deliberate.

### A-09.18 What a browser sees while the server is starting
- **Portico:** The socket is bound before anything slow happens and every request gets `503` JSON with a list of named startup phases and `Retry-After: 2` (`cmd/server/startup.go:90-101`). First-party clients can show a "starting" state. A plain browser shows raw JSON.
- **Jellyfin:** An HTML startup page with the current activity ("Running migration 3 of 7"), and the log for local viewers; it stays up for ten minutes after a failed start so the error can be read in the browser.
- **Why it matters:** Binding first is the important part and Portico does it. The difference is presentation and what happens on failure: Portico logs to stderr and exits, so a failed start is a connection refusal and a journal entry.
- **Assessment:** **Level** on mechanism, **behind** on presentation (F-09.9, F-09.10). Small.

### A-09.19 Getting back in when the owner is locked out
- **Portico:** `portico-server recover-owner --state <dir> [--reset-mfa]` prints a one-time password and revokes sessions (`cmd/server/recover_owner.go`); a claimed server also has a separate local recovery owner (onboarding contract §4.4).
- **Jellyfin:** "Forgot password" writes a PIN file into the config directory that the admin must read from disk.
- **Assessment:** **Portico ahead.** One small risk in B-09.32.

### Networking and remote access

### A-09.20 One port for HTTP and HTTPS
- **Portico:** Port 32500 serves both, decided by the first byte of each connection (`networking/certificate_listener.go:147-170`).
- **Jellyfin:** Separate ports (8096 HTTP, 8920 HTTPS).
- **Why it matters:** One port to forward, one to remember, and the same address works before and after a certificate exists. The cost is a custom listener in front of `net/http` with its own limits (B-09.7) and a two-second window to send the first byte.
- **Assessment:** **Portico ahead** for owners. Deliberate (decision P21).

### A-09.21 Plain HTTP is refused to non-private peers at the socket
- **Portico:** A plain-HTTP connection is accepted only from loopback, RFC 1918/ULA or link-local peers; any other peer is disconnected before a request is read (`certificate_listener.go:176-184`).
- **Jellyfin / Plex:** Jellyfin answers HTTP to anyone unless "require HTTPS" is set. Plex has "secure connections: required / preferred / disabled".
- **Why it matters:** A port forwarded to the internet can never be used in clear text by accident, which is a strong default. It also hard-codes the definition of "local": carrier-grade NAT and Tailscale addresses (100.64.0.0/10) and global IPv6 addresses on the home LAN are not "private" and are refused, with no setting that changes it (B-09.8).
- **Assessment:** **Portico ahead** on safety, **behind** on flexibility. "HTTP stays first-class" is a stated principle, so the classifier should honour the owner's own LAN networks setting.

### A-09.22 Listening: one explicit IP, IPv4 by default
- **Portico:** `PORTICO_BIND` must be a literal IP and port (`main.go:225-233`); the default and every packaged install use `0.0.0.0:32500`, which in Go is IPv4 only. Dual stack needs `[::]:32500`. One address only.
- **Jellyfin:** Bind address lists and IPv4/IPv6 switches in the dashboard (IPv6 off by default too); the port is a setting.
- **Why it matters:** Portico already has the hard part of IPv6 remote access (router pinholes, an `ipv6Open` switch), but the listener that would receive those connections is IPv4-only unless the owner edits the service definition. For households behind carrier-grade NAT, IPv6 is the only direct path in.
- **Assessment:** **Portico behind.** Listening on `[::]` by default where the OS supports dual stack is small; see B-09.21 for what else has to agree.

### A-09.23 Automatic port mapping
- **Portico:** PCP, NAT-PMP, UPnP IGD v1/v2 and IGDv2 IPv6 firewall pinholes, with lease renewal at half-life, epoch-reset detection, durable intent written before each router call, and cleanup on shutdown (`networking/mapping.go`, `mapping_upnp.go`, `mapping_udp.go`, `mapping_pinhole.go`, `remote_manager.go:255-281,707-1050`).
- **Jellyfin / Plex:** Jellyfin removed UPnP. Plex does UPnP and NAT-PMP.
- **Why it matters:** For a typical home router, switching on remote access is the whole job. This is the most complete implementation of the three.
- **Assessment:** **Portico ahead.** The architecture and brief describe remote access as "port forwarding, or none", which undersells it. Gateway detection exists for Linux and macOS only (`topology.go:111-166`), so on Windows the owner must type the router address into the Advanced section before mapping can work.

### A-09.24 Finding the public address
- **Portico:** Asks the router first; otherwise STUN to `stun.cloudflare.com` and `stun.l.google.com`, then HTTPS to `checkip.amazonaws.com` and `icanhazip.com`, at most every five minutes while remote access is on (`networking/wan.go:37-95`). The service list is compiled in.
- **Jellyfin / Plex:** Jellyfin does not discover it (the owner supplies a published URL). Plex asks plex.tv.
- **Why it matters:** It works without Hosted traffic. It also means a server with remote access on contacts four third-party hosts that the owner was not told about and cannot change.
- **Assessment:** **Portico ahead** functionally. Worth one line in the console and privacy notes, and worth deciding whether Portico's own DNS nodes should offer the reflection instead (see Open questions).

### A-09.25 Names and certificates through Hosted
- **Portico:** Each claimed server gets a wildcard certificate for its own random namespace under `direct.getportico.tv` by ACME DNS-01; the private key never leaves the server; renewal is jittered per order; failures back off from 15 s to about half an hour with status in the console (`certificate_manager.go:313-345`).
- **Jellyfin / Plex:** Jellyfin: bring your own certificate or a reverse proxy. Plex: `plex.direct`, the model Portico follows.
- **Assessment:** **Portico ahead** of Jellyfin, level with Plex. The backlog records that a fresh issuance and a renewal have not been run end to end since the move to systemd.

### A-09.26 Bringing your own certificate
- **Portico:** PEM chain and key paths plus one domain in settings, reloaded when the files change, checked at most every 30 s, last good certificate kept on error (`networking/custom_certificate.go`). The domain is published to members as an extra address.
- **Jellyfin:** One PKCS#12 file and password, applied after a restart.
- **Assessment:** **Portico ahead.** One domain only; and on the Linux package the service user usually cannot read `/etc/letsencrypt/live/…`, which is the example the console shows (B-09.14).

### A-09.27 Reverse proxies and sub-path hosting
- **Portico:** Trusted proxies come from an environment variable and from a setting; only `X-Forwarded-For` and `X-Forwarded-Proto` are understood; the presence of a `Forwarded` header from an untrusted peer counts against the request. The app must live at the root of a host: there is no base path. The repository has no reverse-proxy or VPN guidance.
- **Jellyfin / Navidrome:** Known proxies (addresses, networks or host names) enable the standard forwarded-headers middleware including `X-Forwarded-Host`; a base URL lets the server live under `/jellyfin`; guides for five proxies, Tailscale, Let's Encrypt and fail2ban. Navidrome has `BaseURL` too.
- **Why it matters:** A reverse proxy is the single most common way self-hosters publish a media server. Portico's built-in names and certificates make a proxy unnecessary for many owners, but those who already run one for everything else will expect it to work and to be documented.
- **Assessment:** **Portico behind** on proxy support and documentation. Sub-path hosting is medium to large (every client and the web router must carry the prefix) and may reasonably be declined; forwarded-header consistency is small (B-09.24); guides are small.

### A-09.28 Deciding whether a request is local or remote
- **Portico:** One function: the socket peer, or the right-most untrusted `X-Forwarded-For` hop behind a trusted proxy; then the owner's LAN networks if any are set, otherwise private/loopback/link-local; optionally the server's own public address counts as local for hairpin NAT (`httpapi/request_locality.go:47-98`). Unknown means remote.
- **Jellyfin:** LAN subnets setting; everything else is remote; a remote IP allow-list or block-list; per-user "allow remote connections".
- **Why it matters:** Portico's rule set is clear and fails safe. It is not the only implementation, though: setup, sign-in rate limiting and the socket classifier each decide locality their own way (B-09.24).
- **Assessment:** **Level**, with Portico's hairpin handling a nice touch and Jellyfin's IP filter a feature Portico lacks (F-09.17).

### A-09.29 Controls for remote use
- **Portico:** Remote sign-in policy (everyone / owner only / nobody), a remote video bitrate ceiling, an upload-capacity figure from which new remote streams share about 80%, a paused-session timeout, and "require HTTPS" (`operations/settings.go:96-105`).
- **Jellyfin / Plex:** Jellyfin: global and per-user remote bitrate limits, per-user remote access. Plex: upload speed and per-stream limits, much like Portico's.
- **Assessment:** **Level.** Portico's upload budget is closer to Plex than to Jellyfin.

### A-09.30 Finding servers on the LAN
- **Portico:** mDNS/DNS-SD service `_portico._tcp`, written from scratch, carrying the server's key fingerprint; clients treat it as an untrusted hint and verify the identity before sending credentials (`networking/discovery.go`, networking contract §5).
- **Jellyfin:** A UDP broadcast on port 7359 answered with the server name and address; no identity proof.
- **Why it matters:** mDNS is what Apple and Android discovery APIs speak natively, and the fingerprint makes spoofing useless. Multicast does not pass through Docker's default bridge network, so discovery needs host networking there; Jellyfin's broadcast can be published as a UDP port.
- **Assessment:** **Portico ahead** on design; one Docker caveat to document (B-09.26).

### A-09.31 Remote access needs a claimed server
- **Portico:** Port mapping, WAN detection and route publication run only when the server is claimed to a Portico Account and has its Hosted certificate (`remote_manager.go:352,423,486-493`). A direct-sign-in server reports `claim_required` and is left with manual forwarding plus its own certificate or proxy.
- **Jellyfin:** No account exists; everything is manual for everyone.
- **Why it matters:** The owner who chose "no Hosted dependency" also loses automatic port mapping, although the router protocols themselves need nothing from Hosted.
- **Assessment:** **Trade-off (subjective).** Mapping without a name or certificate has limited use, so the coupling is understandable; an owner with their own domain and certificate could still benefit. Small to medium to decouple.

### A-09.32 HTTP versions and sockets
- **Portico:** HTTP/1.1 in clear text; HTTP/2 over TLS with explicit limits (32 streams per connection, bounded receive windows, a 15 s write-progress timeout) (`cmd/server/transport_policy.go`). No HTTP/3, no Unix-socket listener, no clear-text HTTP/2.
- **Jellyfin / Navidrome:** Kestrel HTTP/1.1 and HTTP/2; optional Unix socket. Navidrome also offers a Unix socket.
- **Assessment:** **Level.** A Unix socket is a small addition some proxy setups like (F-09.16). HTTP/3 is not worth it yet.

### A-09.33 Overload protection is designed in
- **Portico:** Accepted connections are capped from host memory; each route belongs to a lane with a capacity, a queue wait, a per-request time budget and a database work class; a refused request gets `503 server_busy` with `Retry-After` (`httpapi/admission.go:70-81,446-527`).
- **Jellyfin:** Kestrel defaults; no admission model.
- **Why it matters:** A weak box under load degrades by refusing honestly rather than by falling over. The budgets are also a sharp tool: an owner operation that legitimately takes longer than its lane's budget fails (B-09.1), and a second, fixed cap hides behind the first (B-09.7).
- **Assessment:** **Portico ahead** in design. Deliberate.

### A-09.34 Talking to the vendor's service
- **Portico:** Event-driven. The control loop sleeps until something is due, with a six-hour safety interval and a check-in bound of days; a network change is "a reason to look, not a reason to call Hosted" (`hosted/current_schedule.go:22-35,70-81`). Events sent are a rename, "storage nearly full" and a shutdown notice (`hosted/events.go`).
- **Jellyfin:** Has no vendor service. Plex servers talk to plex.tv continuously.
- **Assessment:** **Portico ahead** of Plex on restraint. The networking contract still describes a 30-minute heartbeat that the code no longer has (Doc/code disagreements).

### The owner console

### A-09.35 Settings and Console are separate ideas; the dashboard is server-composed panels
- **Portico:** "Settings" is desired future state; "Console" is what is happening now (settings contract §3). The page structure is authored once in `client-core` and each panel loads and fails independently. Roughly 150 owner routes back it.
- **Jellyfin:** One dashboard application with these pages: Dashboard, General, Branding, Users, Libraries (four tabs), Playback (transcoding, resume, streaming, trickplay), Devices, Activity, Networking, API keys, Backups, Logs, Scheduled tasks, Plugins (installed, catalogue, repositories), Live TV.
- **What Portico has that Jellyfin's dashboard does not:** play history with totals and filters, per-library storage breakdown with cleanup actions, a deleted-titles trash, maintenance windows, a time-boxed debug window, a support export, a capability report, remote-access status with mapping and certificate state, viewer feedback reports, a state-folder permission check.
- **What Jellyfin's dashboard has that Portico's does not:** restart and shutdown buttons, send a message to one session, remote-control a session, a plugin catalogue, branding (login disclaimer, custom CSS, splash), a list of log files with a viewer, editable triggers per scheduled task, a devices page with per-device options, per-task run history with duration.
- **Assessment:** **Trade-off.** Different emphasis more than different quality; the gaps are itemised under Missing features.

### A-09.36 Acting on a live session
- **Portico:** Stop one playback (`POST /v1/admin/playback-sessions/{id}/stop`), end a sign-in session (`POST /v1/admin/sessions/{id}:terminate`), broadcast a notice (`POST /v1/admin/notifications/broadcast`).
- **Jellyfin:** Stop, pause, seek and other remote-control commands per session, and a text message shown on that one client.
- **Assessment:** **Level** on stopping, **behind** on a message to one viewer (F-09.21). Whether the owner should be able to drive someone else's player is a product question.

### A-09.37 Activity, history and audit
- **Portico:** Three separate records: the owner's permanent play history (who, what, device, when, how far, converted or not), an operational activity feed, and a hash-chained audit table for administrative actions (`operations/store.go:198-`, table `console_audit`).
- **Jellyfin:** One activity log (sign-ins, failures, playback start/stop, installs, task results) kept 30 days by default; play statistics need the Playback Reporting plugin.
- **Assessment:** **Portico ahead.** A tamper-evident audit trail and built-in play history are both beyond stock Jellyfin.

### A-09.38 Scheduled work
- **Portico:** Two mechanisms. (1) "Console schedules": a job kind, a daily start time, a window length, a time zone and a catch-up flag (`operations/jobs.go:794-893`); none are created by default and the console has no "add" control, only edit for ones that exist. (2) "Maintenance windows": named weekly windows listing task classes; one nightly window is seeded (03:00, four hours, all days; `migrations/0001_baseline.sql:4401-4422`). In code only backups consult windows; `workpolicy.Allowed` returns true for everything (`workpolicy/policy.go:145-147`).
- **Jellyfin:** Every background task is a named scheduled task with one or more triggers (daily at a time, weekly, every N hours, at startup) and an optional time limit, all editable in the dashboard, each with "run now", progress and last result.
- **Why it matters:** Portico's deliberate rule is that background work is deprioritised, never paused, so windows cannot gate work. That leaves the Schedule page describing windows as "when the server does its heavier background work" while most of that work ignores them (B-09.28), and leaves housekeeping jobs with no default trigger at all (B-09.12).
- **Assessment:** **Portico behind** on clarity and completeness. One mechanism, with defaults seeded for every housekeeping job and an interval option, would be medium.

### A-09.39 Logs
- **Portico:** About 120 `log.Printf` call sites in the whole server, nearly all at the default level; two use a `warn:` prefix and none use `debug:`. Lines go to stderr and to a 5,000-record ring plus five 4 MiB files (about 20 MiB in total). The console reads the ring only; a live tail streams new lines; a "debug window" raises the level for a bounded time (`servicelog/servicelog.go`).
- **Jellyfin:** Serilog writes a log file per day, kept a configurable number of days, with levels per component set in `logging.json`; every transcode writes its own FFmpeg log with the full command line; the dashboard lists log files and opens them.
- **Why it matters:** Portico's log is quiet and bounded, which suits a home appliance. For support it is thin: there is almost nothing to turn up, FFmpeg's own output is not kept as a log an owner can hand over, and the console shows nothing from before the last restart (B-09.23).
- **Assessment:** **Trade-off**, leaning behind for troubleshooting. Deliberate in spirit (bounded, redacted, time-boxed debug per settings contract §10).

### A-09.40 Diagnostics beyond logs
- **Portico:** A bounded support bundle (16 MiB cap, settings with secrets removed, system report, connectivity report, recent messages, rotated logs) (`servicelog/bundle.go`), a capability report, per-lane concurrency diagnostics, owner-only pprof profiles, an on-demand database integrity check and orphan repair, panic and loop-restart counters (`supervise`).
- **Jellyfin:** Logs and the activity page.
- **Assessment:** **Portico ahead**, clearly.

### A-09.41 Alerts and notifications
- **Portico:** Deduplicated in-app alerts (`scan-failed`, `job-failed-*`, `retention-cleanup-failed`, `schedule-admission-failed`, `secure-connections-required-unavailable`, `state-permissions`, `upload-budget-exhausted`) and an in-app notification inbox; by decision no email and no push.
- **Jellyfin:** Dashboard alerts from the activity log; webhooks, email and push through plugins.
- **Assessment:** **Trade-off (subjective).** Deliberate ("no push notifications or server email"). An owner who never opens the console learns nothing, so a single outbound webhook is the smallest thing that would change that (F-09.26). Two conditions the contract names as alerts (backup failure, low storage) are not reliably raised (B-09.12, B-09.17).

### A-09.42 Metrics for external monitoring
- **Portico:** None. Host and transcode telemetry is available to the owner as JSON routes only.
- **Jellyfin / Navidrome:** Opt-in Prometheus endpoint.
- **Assessment:** **Portico behind** for the homelab audience. Small, off by default (F-09.7).

### A-09.43 Health endpoints
- **Portico:** `/health/live`, `/health/ready` (database ping), `/v1/readiness` (also reports `degraded` and `corrupt` from the database watchdog, and startup phases while starting), and `portico-server --health-check` for containers.
- **Jellyfin:** `/health` with a database-context check, available on the startup server too.
- **Assessment:** **Portico ahead.**

### A-09.44 Plugins and branding
- **Portico:** No plugin system (no external code), no branding options.
- **Jellyfin:** Plugin catalogue with repositories and an auto-update task; branding page.
- **Assessment:** **Trade-off (subjective).** No plugins is deliberate and removes a whole class of stability and security problems. A login-page message is the one branding item with real use for an owner who shares with family (F-09.24).

### A-09.45 API keys
- **Portico:** Keys are stored hashed, show a hint and last-used time, and carry one of four scopes: `read-only`, `playback`, `library-management`, `full` (`server/internal/access/apikeys.go:14-43`). The settings contract (§1.2) says a key never counts as an interactive owner session for restore or other high-risk actions.
- **Jellyfin:** API keys are all-powerful administrator tokens with a name and creation date.
- **Assessment:** **Portico ahead.** I did not trace whether every high-risk route actually refuses a `full` key; that belongs to the identity audit.

### Backups, restore, migrations and updates

### A-09.46 What a backup is
- **Portico:** A folder with a byte-for-byte consistent SQLite copy made by the online backup API on a pinned snapshot, followed by `quick_check`, plus identity keys, the TLS key, rclone configs, channel logos and the subtitles folder, and a manifest (`backup/backup.go`, `copy.go`, `files.go:21-36`). Any SQLite viewer opens it.
- **Jellyfin:** A zip with each table exported to JSON plus config and optional metadata, subtitles and trickplay folders; a logical export that is independent of the database engine.
- **Why it matters:** Portico's is faster, verifiable and trivially inspectable; Jellyfin's survives a change of database engine and is one file to download. Jellyfin lets the owner choose components; Portico's set is fixed, and does not include artwork (rebuildable) or anything outside the state folder.
- **Assessment:** **Portico ahead** for its design goals. Deliberate (Spec — Backups).

### A-09.47 Scheduling, retention and location
- **Portico:** On by default: nightly inside the seeded window, newest seven kept (`backup/schedule.go`, `DefaultKeepCount`). Always under `<state>/backups`, on the same volume as the database.
- **Jellyfin:** Manual only, kept until deleted, in the data folder. **Navidrome / Audiobookshelf:** cron schedule, keep count and a configurable destination path.
- **Assessment:** **Portico ahead** of Jellyfin on having a schedule at all; **behind** Navidrome and Audiobookshelf on being able to put backups somewhere else (B-09.18, F-09.31).

### A-09.48 Restore
- **Portico:** Validate (file is SQLite, `integrity_check`, is a Portico database, schema not newer), copy into a staging folder, restart; on the next start move the live database into a pre-restore backup, move the staged copy in, migrate, verify, and on any failure move everything back, with each step resumable after a crash through a marker file (`backup/stage.go`, `apply.go:66-150`). The source can be a listed backup, a backup folder elsewhere, or a bare `.db` file.
- **Jellyfin:** Deletes the current rows and re-inserts from the archive inside the running process, then restarts itself; no automatic safety copy or rollback.
- **Assessment:** **Portico ahead**, by a wide margin, in design. Three implementation problems undercut it today (B-09.1, B-09.2, B-09.16).

### A-09.49 How the server restarts itself
- **Portico:** It does not. It exits with code 75 and expects a service manager to start it again (`main.go:124-128`). There is no restart or shutdown route and no console button.
- **Jellyfin:** An in-process loop rebuilds the host, so restart works identically in a terminal, a tray app, Docker without a restart policy, or a service.
- **Why it matters:** Portico's approach is simple and correct under systemd and launchd, and wrong wherever nothing is watching: the Windows install as packaged, a default `docker run`, a portable build in a terminal.
- **Assessment:** **Portico behind.** An outer loop in `main` that re-runs `run()` when it returns the restart sentinel is small (the function already returns cleanly through its defers). See B-09.2 and F-09.2.

### A-09.50 Schema migrations
- **Portico:** Embedded SQL files, one transaction each, a ledger with a digest per migration re-verified on every open, numbers that may arrive out of order, additive-only rules and background backfills so cost does not grow with the library (`persistence/schema.go`, `README.md`). No backup is taken before migrating.
- **Jellyfin:** EF Core migrations plus code "routines" in stages; the runner backs up the database (and, when a routine asks, metadata/subtitles/trickplay) before migrating, shows per-migration progress on the startup page, and optimises afterwards.
- **Why it matters:** Portico's rules make a failed migration far less likely and far cheaper, and each step rolls back on error. Jellyfin assumes migrations can go badly and keeps a way back. With nightly backups on by default Portico usually has a recent copy, but "usually" is not "always": a new install upgraded on day one has none.
- **Assessment:** **Portico ahead** on migration discipline, **behind** on a guaranteed way back (F-09.3). A pre-migration backup is small: the backup service already has a synchronous `Create`.

### A-09.51 Updates
- **Portico:** Notify-only, and only if the owner pastes a feed URL into a setting that is empty by default; channels `stable` and `beta`; the feed is fetched in the background at most every six hours (`administration/update_feed.go`). Releases publish `portico-update-manifest.json`, marked unsigned with automatic installation disallowed.
- **Jellyfin / Plex:** Jellyfin updates through the OS package manager or by pulling a new image; plugins auto-update. Plex checks itself and offers in-app download with channels.
- **Assessment:** **Portico behind** today, because the check is off by default and the published manifest is not the shape the server parses (B-09.11). The policy ("notify and manual install", decision P19) is deliberate and sensible.

### A-09.52 Downgrades
- **Portico:** A binary older than the database refuses to open it and says to upgrade or restore an earlier backup (`schema.go:107-122`); restore refuses a newer-schema backup.
- **Jellyfin:** Same stance; documented as "no downgrade mechanism".
- **Assessment:** **Level.** Portico's message is clearer; the automatic pre-upgrade backup that would make "restore an earlier backup" always possible is the missing half.

### Resources and failure behaviour

### A-09.53 Resource controls
- **Portico:** Mostly automatic: open-file limit raised to the hard limit; Go memory limit set to effective memory minus a quarter (cgroup-aware) unless `GOMEMLIMIT` is set (`hostlimits/hostlimits.go:25-88`); connection cap from memory; database pool of six connections with one writer and a class-ordered write gate; background FFmpeg at `nice 10` with CPU-time and file-size limits; a "background task priority" switch (lower/normal); owner limits on total, hardware, software and background conversions.
- **Jellyfin:** Owner-set numbers: scan fan-out, metadata refresh concurrency, parallel image encodes, transcoding threads, throttling, an in-memory cache size.
- **Why it matters:** Portico asks the owner almost nothing and adapts to the box, which fits "weak boxes to a couple of hundred browsers". There is no I/O priority (`ionice`) and no memory ceiling on FFmpeg children.
- **Assessment:** **Portico ahead** in approach. The numbers have not been validated on hardware (the brief says performance targets are not yet established).

### A-09.54 When the disk fills
- **Portico:** Never refuses to start. Producers of new files (prepared media, artwork, download snapshots) decline below 2 GiB free; the conversion sweeper frees space; `SQLITE_FULL` is a typed error; one event tells Hosted the disk is nearly full (`diskspace`, `hosted/events.go:150-175`).
- **Jellyfin:** Refuses to start when the data, cache or program-data volume has under 2 GB free; backups need 5 GB.
- **Why it matters:** Jellyfin's hard stop is blunt and unpopular but impossible to miss. Portico keeps serving, which is kinder, but the owner has to notice. The proactive notice does not currently fire by default (B-09.12), and the trash and backups sit on the same volume (B-09.13, B-09.18).
- **Assessment:** **Portico ahead** in philosophy, with gaps in telling the owner.

### A-09.55 One server per state folder
- **Portico:** An advisory lock on `server.lock`, taken after the listener binds and before the database opens; a filesystem that cannot lock produces a warning rather than a refusal (`cmd/server/instance_lock*.go`).
- **Jellyfin:** I found no equivalent lock in the files read; a second instance normally fails on the port.
- **Assessment:** **Portico ahead.**

### A-09.56 Crash containment
- **Portico:** A panic in any background goroutine is contained, logged with its stack, counted, and for database-backed loops restarted with backoff from 250 ms to 30 s (`supervise/supervise.go`); a test forbids a bare `go` statement.
- **Jellyfin:** Unhandled exceptions on background threads are logged; behaviour depends on where they occur.
- **Assessment:** **Portico ahead.**

### A-09.57 Looking after the database
- **Portico:** A watchdog that can reopen the handle; WAL checkpoints on a timer that escalate to truncation past 64 MiB when quiet and past 192 MiB regardless (`dbwork/wal.go:135-165`); planner statistics refreshed off the write path; an opt-in structural check (`PORTICO_INTEGRITY_CHECK=startup|hourly|daily|weekly|<duration>`); an on-demand check and a repair for orphaned rows.
- **Jellyfin:** An "Optimize database" scheduled task (vacuum/analyse) and optimisation after migrations.
- **Assessment:** **Portico ahead.** The scheduled integrity check is environment-only; it would be more discoverable as a setting. Portico has no vacuum, so a database that shrinks never gives space back (F-09.34).

### A-09.58 Deleting media
- **Portico:** "Delete" moves files into `<state>/trash/<entry>/` (copying when the library is on another volume) with a retention figure, restorable from the console; emptying is an explicit owner action (`administration/deletion.go:298-310,416-470,717-`).
- **Jellyfin / Plex:** Delete the file immediately.
- **Why it matters:** A recoverable delete is a real safety feature. Where the trash lives and the fact that nothing empties it automatically make it a disk-space trap (B-09.13).
- **Assessment:** **Portico ahead** in intent, with a design risk.

### A-09.59 Telemetry and privacy posture
- **Portico:** No usage analytics. Outbound traffic an owner may not expect: Hosted (claim, certificates, route changes, three event kinds), STUN and address services when remote access is on, metadata providers, LRCLIB for lyrics, the update feed if set, and the title dataset once published. Logs and exports are bounded and settings exports strip secrets.
- **Jellyfin / Navidrome / Plex:** Jellyfin: none beyond metadata and plugin repositories. Navidrome: an anonymous insights collector on by default. Plex: account-bound telemetry.
- **Assessment:** **Portico ahead** of Plex and Navidrome, level with Jellyfin in spirit. A single "what this server contacts and why" page would turn this into something to advertise.

## Missing features and capabilities to consider

Each entry: what it is, who has it, the value, a rough size, any conflict with Portico's principles, and my opinion.

### Installing and running

### F-09.1 A published compose file and container guide
- **What:** An official `docker-compose.yml` and a short page covering the volume, file ownership on bind mounts (`user: uid:gid`, or a `PUID`/`PGID` entrypoint), `restart: unless-stopped`, `TZ`, host networking for discovery, and GPU passthrough (`/dev/dri`, the `render` group, NVIDIA runtime).
- **Who:** Jellyfin (`jellyfin-docs/docs/general/installation/_container-docker-compose.md`), LinuxServer.io images, Plezy's relay (`plezy/server/docker-compose.yml`).
- **Value:** Docker is the most common install route for this audience; today the only instruction is "pull the image". Restore depends on a restart policy the owner is never told to set (B-09.2).
- **Size:** Small (docs), small to medium for a `PUID`/`PGID` entrypoint. **Conflict:** none.
- **Opinion:** Do it before release. It also forces the GPU-in-Docker story to be tested (B-09.26).

### F-09.2 Restart and shut down from the console, and restart without a service manager
- **What:** Owner buttons for Restart and Shut down, and an in-process restart so the server comes back wherever it is running.
- **Who:** Jellyfin (`POST /System/Restart`, `/System/Shutdown`; the restart loop in `Program.cs:145-163`).
- **Value:** Needed after a restore everywhere; useful after changing an environment-level setting; removes the Windows and plain-Docker failure in B-09.2.
- **Size:** Small. **Conflict:** the settings contract lists shutdown as a high-risk action needing confirmation; nothing forbids it.
- **Opinion:** Worth doing; the in-process loop is the important half.

### F-09.3 An automatic backup before a schema migration
- **What:** When a start finds migrations to apply, take a `pre-upgrade` backup first and name it in the log and console.
- **Who:** Jellyfin (`JellyfinMigrationService.PrepareSystemForMigration`).
- **Value:** Makes "restore an earlier backup" (the advice in Portico's own downgrade error) always possible, including for the owner who upgraded before the first nightly backup.
- **Size:** Small; `backup.Service.Create` exists and can run before `persistence.Open` migrates if the copy is taken with a plain file copy after a checkpoint, as `apply.go` already does for pre-restore. **Conflict:** startup time grows with database size on upgrade starts only; the "no startup work that grows with the library" rule would need this one named exception.
- **Opinion:** Yes. Keep one or two, pruned like pre-restore copies.

### F-09.4 Hosting under a sub-path (base URL)
- **What:** Serve the whole app and API under a prefix such as `/portico`.
- **Who:** Jellyfin (`NetworkConfiguration.BaseUrl`), Navidrome (`BaseURL`), Audiobookshelf (`ROUTER_BASE_PATH`).
- **Value:** Owners who publish many services on one host name behind a proxy.
- **Size:** Medium to large: router, web app asset paths, every client's URL builder, the Cast receiver, signed route documents. **Conflict:** the networking contract's address rules reject "unexpected paths" (§4), so this would be a contract change.
- **Opinion:** Decline for release and say so in the docs; a sub-domain works for almost everyone and Portico's own names remove the need for many.

### F-09.5 Reverse-proxy, VPN and Tailscale guides
- **What:** Short, tested recipes for Caddy, nginx and Traefik (which headers to pass, WebSocket/SSE buffering off, large-range streaming, what to put in Trusted proxies), plus Tailscale and WireGuard.
- **Who:** Jellyfin (`jellyfin-docs/…/8_reverse-proxy/*`, `2_tailscale.md`, `9_advanced/*`).
- **Value:** Prevents the misconfigurations in B-09.24 and tells Tailscale users what currently does not work (B-09.8).
- **Size:** Small. **Conflict:** none.
- **Opinion:** Do it; writing them will surface real bugs.

### F-09.6 Let the owner say which networks may use plain HTTP
- **What:** Make the socket-level "local peer" test honour the existing LAN networks setting (and offer a one-click "include Tailscale/CGNAT range").
- **Who:** Jellyfin treats whatever is listed under LAN networks as local; its Tailscale page tells owners to add `100.64.0.0/10`.
- **Value:** Tailscale and other overlay VPNs, IPv6 LANs, campus networks with public addressing.
- **Size:** Small: `localHTTPConnection` needs the cached locality policy. **Conflict:** it widens where clear-text HTTP is accepted, by explicit owner choice, which fits "owner choice, low ceremony".
- **Opinion:** Yes. See B-09.8.

### F-09.7 A Prometheus metrics endpoint
- **What:** Opt-in `/metrics` with request, lane, stream, conversion, database and host gauges, protected by an API key or loopback-only.
- **Who:** Jellyfin (`EnableMetrics`), Navidrome (`Prometheus.Enabled`).
- **Value:** The homelab audience graphs everything; the numbers already exist in the telemetry and concurrency routes.
- **Size:** Small to medium. **Conflict:** the contract wants public health endpoints free of operational detail, so it must be off by default and authenticated.
- **Opinion:** Worth having after release; low priority.

### F-09.8 Change the port and listen on IPv6 from the console
- **What:** Port and bind address(es) as settings with "applies after restart", dual-stack by default, more than one bind address.
- **Who:** Jellyfin (ports, bind lists, IPv4/IPv6 switches, with a restart prompt).
- **Value:** Port conflicts are common (32500 is unusual enough to be safe, but not always); IPv6 is the only inbound path behind carrier-grade NAT.
- **Size:** Medium. The listener is created before the database opens, so the setting would have to be read from a small file or the database opened earlier. **Conflict:** none.
- **Opinion:** Dual-stack default first (small); the rest after release.

### F-09.9 A human-readable page while starting or migrating
- **What:** For a browser asking for HTML during startup, a small static page that polls readiness and shows the phase, instead of JSON.
- **Who:** Jellyfin's setup server (`ServerSetupApp/index.mstemplate.html`).
- **Size:** Small; the reporter already has the phases. **Opinion:** Cheap polish worth doing.

### F-09.10 Keep answering after a failed start
- **What:** When startup fails after the listener is bound (database newer than the binary, migration error, unreadable state folder), stay up for a while serving the reason to local viewers rather than exiting immediately.
- **Who:** Jellyfin holds its startup page for ten minutes with the error.
- **Value:** Turns "the site can't be reached" into a sentence the owner can act on, on platforms where the log is hard to find (Windows, macOS).
- **Size:** Small. **Conflict:** a process that stays up in a failed state must not look healthy; readiness already answers 503.
- **Opinion:** Yes, for the handful of known owner-fixable failures.

### F-09.11 A real Windows service or tray app, with a firewall rule
- **What:** See A-09.3. Run under a service control manager entry with restart-on-failure, as a low-privilege or user account, add an inbound firewall rule for the port at install, and give the owner a visible status and "open Portico" entry point.
- **Who:** Jellyfin (tray app), Plex (tray app, firewall rule).
- **Size:** Medium. **Opinion:** Needed before Windows is recommended to non-technical owners.

### F-09.12 macOS menu-bar presence, an uninstall path and Intel builds
- **What:** A menu-bar item with status, open, stop and "start at login"; removal of the LaunchAgent when the app is deleted or via an Uninstall item; optionally a universal binary.
- **Who:** Jellyfin, Plex. **Size:** Medium (companion app, already planned). **Opinion:** Fine to ship after release if the current launcher's rough edges are documented.

### F-09.13 NAS and appliance packaging
- **What:** An Unraid template, a TrueNAS app definition, Synology/QNAP packages or documented container recipes for each; 32-bit ARM if older Raspberry Pis matter.
- **Who:** Jellyfin (`jellyfin/deployment/unraid`, docs for Synology and TrueNAS), Plex.
- **Size:** Small each for templates, large for native packages. **Opinion:** Unraid and TrueNAS templates are cheap reach.

### F-09.14 A signed package repository
- **What:** apt and dnf repositories so the OS updates Portico.
- **Who:** Jellyfin. **Size:** Medium, tied to package signing. **Conflict:** "no unattended installation of preview builds" until signing exists, which `INSTALLING.md` already says.
- **Opinion:** The natural way to deliver updates on Linux once signing is in place; simpler than building an updater.

### F-09.15 A per-install environment file and a configuration reference
- **What:** `/etc/default/portico-media-server` read by the unit, an equivalent on Windows and macOS that the installers leave alone, and one page listing every `PORTICO_*` variable.
- **Who:** Jellyfin (`/etc/default/jellyfin`, configuration docs), Navidrome (TOML file).
- **Size:** Small. **Opinion:** Yes; today changing the port on Windows means editing a generated `.cmd` file that the next install overwrites.

### F-09.16 Listening on a Unix socket
- **What:** Optional socket path for reverse proxies on the same host.
- **Who:** Jellyfin, Navidrome. **Size:** Small. **Opinion:** Low priority; the peer-address rules would need a defined answer for socket peers.

### Networking

### F-09.17 A remote address allow-list or block-list, and proxy-friendly failure logs
- **What:** Owner lists of networks that may or may not connect remotely, and a stable log line for failed sign-ins that tools like fail2ban or CrowdSec can match.
- **Who:** Jellyfin (`RemoteIPFilter`, `IsRemoteIPFilterBlacklist`, `9_advanced/fail2ban.md`).
- **Value:** Country- or network-level blocking is a common ask; Portico already has cool-offs on failed attempts, so the block-list is the smaller half.
- **Size:** Small. **Conflict:** none. **Opinion:** Optional; the documented log line is the cheap, useful part.

### F-09.18 HTTP/3
- **Who:** None of the comparison servers by default. **Opinion:** Not now; record as considered.

### F-09.19 A relay for servers that cannot accept inbound connections
- **What:** Already planned as the one paid feature. **Who:** Plex Relay. **Value:** Carrier-grade NAT without IPv6 leaves an owner with no direct path; today the only answers are a VPN or a third-party tunnel, neither documented.
- **Opinion:** Until it exists, a documented Tailscale/Cloudflare Tunnel recipe covers the gap (F-09.5).

### F-09.20 DLNA
- Planned and recorded in the backlog (off by default). No further comment from this domain except that it adds SSDP on UDP 1900 and another reason host networking matters in Docker.

### The console

### F-09.21 Message one viewer
- **What:** Send a short text to a single active session ("pausing the server in five minutes").
- **Who:** Jellyfin (`Sessions/{id}/Message`). Portico has a broadcast to everyone.
- **Size:** Small; the notification inbox exists. **Opinion:** Nice to have.

### F-09.22 Richer schedules
- **What:** Interval and weekly triggers, a "run at startup" trigger, a per-task time limit, a "run now" button and last-run duration for every housekeeping job, and a way to add a schedule in the console.
- **Who:** Jellyfin scheduled tasks.
- **Size:** Medium. **Conflict:** none; "never paused" is about priority, not about when a job is asked to start.
- **Opinion:** At minimum seed defaults and show every job; see B-09.12 and B-09.28.

### F-09.23 Log files in the console and FFmpeg logs
- **What:** List and download the rotated log files, read history older than the in-memory ring, and keep the FFmpeg command line and stderr of recent conversions (bounded) where an owner can export them.
- **Who:** Jellyfin (log file list and viewer; a log per transcode).
- **Value:** "It won't play" is the most common support question and the FFmpeg log answers most of them.
- **Size:** Small for file listing; medium for bounded FFmpeg logs. **Conflict:** the redaction rules in the settings contract §10 (paths and titles) apply; a bounded, owner-only record is compatible.
- **Opinion:** High value for support.

### F-09.24 A sign-in page message
- **What:** One owner-written line on the sign-in page. **Who:** Jellyfin branding (login disclaimer); custom CSS and splash are the rest of that page.
- **Size:** Small. **Opinion:** The message is worth having; custom CSS is not (it fights the "thin clients" design).

### F-09.25 Plugins
- **Who:** Jellyfin, Emby, Plex (historically). **Conflict:** direct conflict with "no external code contributions" and with the bounded, typed API. **Opinion:** Decline; record the decision so the question stops coming up. Webhooks (F-09.26) cover the most common plugin use.

### F-09.26 An outbound webhook for owner alerts
- **What:** One URL that receives a small JSON post when an owner alert opens or resolves (backup failed, disk low, certificate expiring, scan failed).
- **Who:** Jellyfin via its webhook plugin; many self-hosted tools natively.
- **Value:** The owner's existing notifier (ntfy, Home Assistant, Discord, email relay) does the delivery, so Portico still sends no email and no push.
- **Size:** Small. **Conflict:** "no push notifications or server email" is about Portico delivering; a webhook hands delivery to the owner. Still, it is a new outbound channel and Justin's call.
- **Opinion:** The cheapest way to make alerts reach an owner who is not looking at the console.

### F-09.27 Security events in the activity view
- **What:** Failed sign-ins, new devices, password and role changes as readable activity entries.
- **Who:** Jellyfin's activity log. Portico records an audit chain; I did not verify how much of it the console shows.
- **Opinion:** Check against the identity audit; if the data exists, surfacing it is small.

### F-09.28 Free space for every place that matters
- **What:** Show free space for each library path, the recordings folder and the conversion folder, not only the state volume.
- **Who:** Jellyfin (`GET /System/Info/Storage` lists every library folder and system path with free and used space).
- **Size:** Small; `telemetry.VolumeUsage` exists. **Opinion:** Yes; recordings filling a media disk is as common as the state disk filling.

### F-09.29 First library in first-run setup
- **What:** Offer "add your first library" (and language) before landing in an empty app.
- **Who:** Jellyfin's wizard. **Size:** Small to medium. **Opinion:** Justin's call; see A-09.16.

### F-09.30 Time zone handling for containers
- **What:** Document `TZ`, show the server's time zone beside schedule times, and warn when it is UTC on a machine whose owner is clearly elsewhere.
- **Value:** In Docker the default zone is UTC, so the seeded 03:00 window lands in the evening for the Americas.
- **Size:** Small. **Opinion:** Yes.

### Backups and updates

### F-09.31 Backups somewhere else, as one file, on demand from a shell
- **What:** (a) A backup destination outside the state folder, by setting. (b) "Download backup" as a single archive. (c) `portico-server backup` and `portico-server restore <path>` commands for scripts and for a server that will not start.
- **Who:** Navidrome (path, schedule, count, CLI), Audiobookshelf (`BACKUP_PATH`), Jellyfin (`--restore-archive`, zip).
- **Value:** A backup on the same disk as the database does not survive the failure most people back up for. The spec's "copy the folder" works but needs shell access.
- **Size:** Small each. **Conflict:** the spec says "no archive format" for the stored backup; a download archive is a transport, not the stored form.
- **Opinion:** (a) is the important one; the architecture doc already claims it exists.

### F-09.32 Backup health the owner can see
- **What:** An alert when a scheduled backup fails or none has succeeded for N days; a visible note that backups share the database's disk; an occasional "restore drill" that opens the newest backup read-only and checks it.
- **Who:** The settings contract asks for all three (§12.1, §9 "last restore drill"). None of the comparison servers does a drill.
- **Size:** Small. **Opinion:** The failure alert is a must; see B-09.17.

### F-09.33 A working update check, on by default
- **What:** A default feed, a notification when a newer release exists, and release notes in the console.
- **Who:** Plex; Jellyfin relies on package managers.
- **Size:** Small once B-09.11 is fixed. **Conflict:** "Hosted stays cheap" suggests the feed stays a static file on GitHub, fetched a few times a day with jitter, which is what the code already does.
- **Opinion:** Yes; an owner on an old pre-release build should be told.

### F-09.34 Reclaim database space
- **What:** An occasional `VACUUM` (or incremental auto-vacuum) offered as an owner action with a clear "needs this much free space, takes about this long" note.
- **Who:** Jellyfin's Optimize database task.
- **Size:** Small. **Conflict:** a full vacuum is library-sized work under the write gate; it must be explicit and never automatic.
- **Opinion:** Low priority; useful after removing a large library.

### Kept for the record

### F-09.35 Deployment hardening statements
- **What:** State and test whether the container runs with a read-only root filesystem (it should: everything is written under the state volume, plus `PrivateTmp`/`TMPDIR` needs), under rootless Docker and Podman, and in Kubernetes (probes map directly onto `/health/live` and `/health/ready`).
- **Who:** Jellyfin documents Podman and Kubernetes. **Size:** Small (testing and a paragraph). **Opinion:** Cheap credibility with the audience that asks.

## Possible bugs and risks in Portico

All of these come from reading source. None was reproduced. "Confirmed by reading" means the code path is unambiguous; it does not mean the behaviour was observed. `Backlog.md` lists none of these unless the entry says so.

### B-09.1 Restore, and the on-demand integrity check, run under a 10-second request budget
- **What goes wrong:** `POST /v1/admin/backups/restore` is on the `adminHeavy` lane (`server/internal/httpapi/route_lanes.go:308`), whose budget is 10 s (`admission.go:80`); the middleware wraps the request context in that timeout (`admission.go:484-487`). The handler calls `Backups.Stage(ctx, …)` synchronously (`backup_routes.go:149`). `Stage` hashes the state files, copies the whole database into `restore-staged/` (no context, so it runs to completion however long it takes) and then calls `persistence.InspectCandidate(ctx, …)`, which runs `PRAGMA integrity_check` on the copy with that same context (`persistence/inspect.go:46-64`). Once the 10 s have passed, the query fails or is interrupted, the function returns `ErrCandidateIntegrity`, and the API answers `restore_integrity_failed`.
- **When:** Any database whose copy plus full integrity check takes longer than ten seconds. On a spinning disk that is a few hundred megabytes; on an SSD perhaps one or two gigabytes. Portico's target libraries are far larger. The owner is told a healthy backup "failed its integrity check".
- **Same pattern:** `POST /v1/admin/diagnostics/integrity` and `…/integrity/repair` run `quick_check`, `foreign_key_check` and the orphan repair with `r.Context()` on the same lane (`httpapi/admin_concurrency.go:196-224`), so the on-demand check cannot complete on a large database either. `POST /v1/admin/backups` avoids this on purpose by detaching its context (`backup/backup.go:309`), which shows the hazard was known for backup creation.
- **How the comparison surfaced it:** Jellyfin's restore is a restart-time operation with no request deadline; asking "how long may Portico's restore take?" led to the lane table.
- **Confidence:** Confirmed by reading. **Severity:** High (restore is unusable at the scale Portico is designed for, with a misleading error).
- **Fix direction:** Make staging a detached job with progress, like backup creation; give long owner operations their own lane without a budget.

### B-09.2 After a restore the server exits and nothing may start it again
- **What goes wrong:** Staging a restore calls the restart hook, which cancels the root context after two seconds; `main` then exits with status 75 "so the service manager restarts it" (`cmd/server/main.go:124-128,917-924`). That works under systemd (`Restart=on-failure`) and launchd (`KeepAlive`). It does not work where nothing is watching:
  - **Windows as packaged:** the Scheduled Task runs `start-portico.cmd`, which runs the server once and ends (`packaging/windows/installer.nsi:37-46`). The task has an at-boot trigger only. After a restore the server stays down until the machine reboots or someone runs the task by hand.
  - **Docker without a restart policy:** the container exits. Neither the Dockerfile nor `INSTALLING.md` mentions a restart policy.
  - **Portable builds run from a terminal:** the process simply ends.
- **How the comparison surfaced it:** Jellyfin restarts in-process (`jellyfin/Jellyfin.Server/Program.cs:145-163`).
- **Confidence:** Confirmed by reading (the packaging has no restart path). **Severity:** High on Windows, Medium elsewhere.

### B-09.3 Linux packages: a fresh install is not started, and an upgrade leaves the service stopped (deb) or stopped and disabled (rpm)
- **What goes wrong:** `packaging/linux/postinstall.sh` runs `systemctl enable` but never `start`. `preremove.sh` runs `systemctl stop` and `disable` unconditionally, without looking at the argument that distinguishes an upgrade from a removal.
  - **deb upgrade:** the old package's pre-removal script runs (stop, disable), then the new post-install runs (enable). The server is left enabled but not running until a reboot or a manual start.
  - **rpm upgrade:** the new post-install runs first (enable), then the old package's pre-uninstall script (stop, disable). The server is left stopped **and disabled**, so it does not come back after a reboot either.
  - **fresh install (both):** enabled, not started. Nothing prints the address to open.
- **How the comparison surfaced it:** Jellyfin's packages start and restart the service; reading Portico's two scripts next to that made the missing branches obvious.
- **Confidence:** Confirmed by reading the scripts; the script ordering is standard package-manager behaviour and was not exercised here. **Severity:** High for anyone updating a packaged install.
- **Fix direction:** `systemctl enable --now` on first install, `try-restart` on upgrade, and stop/disable only when the argument says removal (`remove` for deb, `$1 = 0` for rpm).

### B-09.4 Windows: Scheduled Task defaults will stop the server after three days and on battery
- **What goes wrong:** The task is created with `schtasks /Create … /SC ONSTART /RU SYSTEM /RL HIGHEST` and no XML (`installer.nsi:45`). Tasks created that way inherit Task Scheduler's defaults, which to my knowledge include "stop the task if it runs longer than 3 days", "start only on AC power" and "stop if the computer switches to battery power". A server is exactly the kind of task that runs longer than three days. Because the only trigger is at boot, a stopped task stays stopped.
- **Also:** stopping or replacing the task ends the process tree abruptly, so on Windows every stop is an unclean one (no drain, no final checkpoint, no shutdown notice). SQLite's WAL makes that safe for the data, but it is not the path the shutdown code was written for.
- **Confidence:** Probable; needs a check on a Windows machine (the defaults are from memory, the missing settings are confirmed by reading). **Severity:** High on Windows.
- **Fix direction:** Register from an XML definition with no execution time limit and battery conditions off, or use a real service (F-09.11).

### B-09.5 Windows: running as SYSTEM breaks network shares and has no firewall rule
- **What goes wrong:** (a) `SYSTEM` has no access to the signed-in user's mapped drive letters or saved share credentials, so a library on a NAS share that works in Explorer is invisible to the server. (b) FFmpeg and the helpers parse untrusted media as the most privileged local account. (c) The installer adds no inbound firewall rule; a process in the services session cannot raise the "allow access?" prompt, so other devices on the LAN are blocked until the owner adds a rule by hand. (d) `%ProgramData%\Portico Media Server` is created with inherited permissions, which normally let every local user read it, and the permission check that would warn about that cannot work on Windows (B-09.15).
- **Confidence:** Probable (standard Windows behaviour, not tested). **Severity:** High for first-run success on Windows.

### B-09.6 Two log files that grow without limit on Windows and macOS
- **What goes wrong:** The server writes every log line to stderr as well as to its own rotated files (`cmd/server/administration.go:27-28`). The Windows wrapper appends stdout and stderr to `%ProgramData%\Portico Media Server\server.log` forever (`installer.nsi:42`). The macOS agent sends them to `logs/server.log` and `logs/server-error.log` in the state folder (`packaging/macos/launcher.sh:21-22`). Nothing rotates or truncates either, and the support bundle only collects `messages*.log`.
- **Confidence:** Confirmed by reading. **Severity:** Medium (slow, but it is the state volume that fills).
- **Fix direction:** Point both at the null device once the recorder is installed, or have the server skip the stderr tee when it is not attached to a terminal or a journal.

### B-09.7 A fixed 256-connection ceiling sits under the memory-based one, and it drops rather than queues
- **What goes wrong:** `main` computes a connection budget from memory (16 to 2,048; `transport_policy.go:30-42`) and logs "Connection ceiling set to N". Whenever the bind address is not loopback, which is every real install, the listener is wrapped in `networking.DirectListener`, which has its own hard-coded `connections: make(chan struct{}, 256)` (`networking/certificate_listener.go:57`). In its accept loop, when that channel is full the new connection is closed immediately (`certificate_listener.go:116-121`). So on any host with more than about 4 GiB of memory (where the computed budget exceeds 256) the real ceiling is 256, and past it clients get a reset instead of waiting in the kernel backlog, which is the behaviour `connection_cap.go:17-21` says was chosen on purpose.
- **Why it matters for the stated target:** Plain HTTP is first-class and is HTTP/1.1, where a browser opens up to six connections per host and holds idle ones for the 60 s idle timeout; each viewer also holds an event stream. A few dozen active browsers on the LAN can reach 256. The memory formula has its own tension with the target: 1 GiB gives 64 connections and 4 GiB gives 256.
- **Why tests would not catch it:** the soak and idle tests bind to 127.0.0.1, where the direct listener is not used (`main.go:996`).
- **Confidence:** Confirmed by reading for the cap and the close; the user-visible impact needs a load test. **Severity:** High against the "couple of hundred concurrent browsers" goal.

### B-09.8 Plain HTTP is refused to Tailscale, carrier-grade-NAT and global-IPv6 peers, and the console disagrees with itself about them
- **What goes wrong:** `localHTTPConnection` accepts clear-text HTTP only from loopback, `IsPrivate()` and link-local peers (`certificate_listener.go:176-184`). Go's `IsPrivate` does not include 100.64.0.0/10, which is the range Tailscale and carrier-grade NAT use; the test table asserts that `100.64.0.1` is not local (`certificate_listener_test.go:24`). It also excludes every global IPv6 address, which is what LAN devices have on an IPv6 home network. No setting changes this: the owner's "home networks" list feeds `requestIsRemote` but not the socket classifier.
- **Consequences:** A phone on the owner's tailnet opening `http://100.x.y.z:32500` gets a closed connection. HTTPS to that address fails name validation because the certificate is for the `direct.getportico.tv` name. The same applies to a reverse proxy reaching the server over a tailnet. Meanwhile the connectivity report classifies 100.64.0.0/10 as "lan" (`connectivity/status.go:246`), so the console can show such an address as a LAN address while the listener rejects it.
- **How the comparison surfaced it:** Jellyfin has a Tailscale guide that tells owners to add that range to LAN networks.
- **Confidence:** Confirmed by reading. **Severity:** Medium to High for the self-hosting audience (Tailscale is a default answer to "how do I reach it remotely?").

### B-09.9 The "Folder for converted video" setting is saved, validated and reported, but never used
- **What goes wrong:** `temporaryDirectory` is a registry field marked `restart` (`operations/settings.go:83`), is shown in Playback settings (`client-core/src/presentation/settings-structure.ts:361`) and is checked for writability by the capacity report and the attention list (`telemetry/capacity.go:191,302-`, `telemetry/attention.go:175-180`). The converter is constructed with a fixed path: `playback.NewHLS(ctx, db, filepath.Join(state, "hls"), ffmpegTool)` (`cmd/server/main.go:664`). No other code reads the setting.
- **Effect:** An owner who points conversions at a fast disk or RAM disk sees the console confirm that folder is ready while conversions keep writing under the state folder. If the chosen folder is unwritable the console warns that "every conversion will fail", which is also untrue.
- **Confidence:** Confirmed by reading. **Severity:** Medium. The settings contract forbids exactly this ("a visible writable field without a named runtime consumer", §2.2).

### B-09.10 The console offers a secure-connections choice the server rejects
- **What goes wrong:** The settings structure offers three values for `secureConnectionsPolicy`: `required`, `preferred`, `lan-plain-allowed` ("Required away from home only") (`settings-structure.ts:392`, `packages/i18n/src/catalog/en-US/settings.ts:231`). The server accepts two (`operations/settings.go:52`) and reports any other as an invalid field (`settings_administration.go:27-28`). I found no mapping layer.
- **Effect:** Choosing the third option fails to save. In behaviour, `preferred` already is "required away from home" because of the socket classifier, so the third option is redundant as well as broken.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium.

### B-09.11 The update check cannot work with the manifest the release pipeline publishes
- **What goes wrong:** The server parses a feed shaped `{"channels": {"stable": [{"version", "url", …}]}}` (`administration/maintenance.go:490-492,525-531`). The release workflow publishes `portico-update-manifest.json` shaped `{"schemaVersion", "channel": "stable", "version", "tag", "assets": {…}}` (`scripts/write-public-update-manifest.mjs:22-30`). Pointing the setting at that file yields "The release feed was not in a shape this server understands." The feed URL setting is empty by default and nothing in the repository says what to put there, so out of the box the Updates panel reports only the running build.
- **Confidence:** Confirmed by reading both files. **Severity:** Medium (owners are never told a newer build exists).

### B-09.12 Low-storage and certificate-expiry notifications have no default trigger
- **What goes wrong:** Both notices are raised only inside `Store.NotificationMaintenance` (`operations/notifications.go:926-`), which is reachable only as the job kind `notification-maintenance` (`cmd/server/console.go:85-91`). Maintenance job kinds run when a job is posted or a console schedule exists. The baseline seeds no console schedules, the web console can edit schedules but has no control to add one (`web/src/screens/server/MaintenanceWindows.tsx:80-81`), and no client code mentions the kind. The General page nevertheless shows three thresholds for these alerts ("When the server puts a warning in your notifications").
- **Effect:** On a default install the storage warning, the storage critical warning and the certificate-expiry warning are never sent, and the thresholds do nothing. The dashboard's health panel computes storage attention on demand, so an owner looking at it still sees a problem.
- **Confidence:** Probable (I found no trigger in the server, the web app, client-core or the native apps; a runtime check would settle it). **Severity:** Medium; a silently filling disk is the classic way these servers break.

### B-09.13 Deleted media is moved into the state folder and never leaves on its own
- **What goes wrong:** Deleting a title moves its files to `<state>/trash/<entry>/`; when the library is on another volume this is a copy onto the state volume (`administration/deletion.go:298-314,416-470`). Each entry records an expiry from the retention setting, but expiry is only a flag in the listing; the only code that removes held entries is the owner-invoked `EmptyTrash` (`deletion.go:717-`, called only from `httpapi/administration.go:238`). The catalogue-side `catalog-trash-cleanup` job is a different thing and is likewise unscheduled by default.
- **Effect:** Deleting a 60 GB file from a 12 TB array writes 60 GB onto whatever small disk holds the database. Thirty days later it is still there. The setting text says "how long deleted files stay recoverable", which reads as a promise that they go afterwards.
- **How the comparison surfaced it:** Jellyfin and Plex delete in place, so they have no such volume.
- **Confidence:** Confirmed by reading. **Severity:** High as a design risk (it converts a routine action into a path to a full database volume).
- **Fix direction:** Keep the trash on the same filesystem as the file (a hidden folder at the library root) so a delete is a rename, and purge expired entries on a schedule that exists by default.

### B-09.14 The systemd sandbox blocks things the console lets the owner do
- **What goes wrong:** The unit sets `ProtectSystem=strict` with only `/var/lib/portico-media-server` writable (`packaging/linux/portico-media-server.service:19-23`). Under that setting every other path is read-only for the service regardless of file permissions. So, on the deb/rpm install:
  - deleting media: the copy into the trash succeeds, removing the original fails (or the whole operation fails), for any library outside the state folder;
  - `PORTICO_RECORDINGS_DIR` pointing at the media disk cannot be written;
  - restoring from, or (if added) backing up to, a path outside the state folder is read-only at best;
  - `ProtectHome=read-only` plus the `portico` user means libraries under `/home` are readable only if permissions allow, and never writable;
  - the custom-certificate example the console shows, `/etc/letsencrypt/live/…`, is not readable by a non-root user, so the certificate fails to load;
  - the `portico` user is not in the `render` or `video` group, so VAAPI/QSV device nodes are usually not accessible.
- **Confidence:** Confirmed by reading the unit; each consequence follows from documented systemd behaviour and was not tested. **Severity:** Medium to High (features appear broken on the most "official" Linux install).

### B-09.15 The state-folder permission check walks everything, misfires on Windows and Docker, and its fix can touch media
- **What goes wrong:** At every start `backup.CheckPermissions` walks the whole state folder until it finds one file or folder with any group/other permission bit (`backup/stateperms.go:16-41`, called from `main.go:829-840`).
  - **Cost:** on a healthy install it visits every artwork file, conversion segment, recording and backup, which grows with the library, against the "bounded work" principle (it runs in the background, so it delays nothing).
  - **Mounts and recordings:** `<state>/mounts` holds rclone mounts and `<state>/recordings` holds recordings by default. The walk descends into both, and "Fix permissions" (`FixPermissions`, same file) will `chmod` remote-mount entries and recorded media, although the spec says the fix "tightens only Portico's own files".
  - **Windows:** Go reports synthetic modes there (0666/0777), so the check always says "exposed" and the fix cannot change that; the file has no platform guard. The warning would be permanent.
  - **Docker:** the image creates the state folder as the user's home with default mode and skeleton files (`Dockerfile`: `useradd --create-home --home-dir /var/lib/portico`), and bind mounts are commonly 0755, so a new container is likely to show the warning about "other accounts on this computer", which means little inside a container.
- **Confidence:** Confirmed by reading for the walk and the missing guard; Probable for the Windows and Docker outcomes. **Severity:** Medium.

### B-09.16 Undoing a restore loads the whole database into memory
- **What goes wrong:** A pre-restore backup's manifest lists the database itself with a hash (`backup/apply.go:209-217`, `hashTree`). Restoring from any backup folder first runs `checkManifestHashes`, which calls `os.ReadFile` on every listed file (`backup/files.go:242-256`). For a pre-restore backup that is the full database read into one byte slice. A 20 GB database on an 8 GB machine will be killed for memory before the restore is staged. Ordinary backups are unaffected because their manifests list only the small state files (and, as a side effect, carry no hash for the database at all).
- **Confidence:** Confirmed by reading. **Severity:** Medium (it breaks the "put it back" path exactly when an owner needs it). Fix: hash by streaming, as `hashTree` already does.

### B-09.17 Scheduled backups can fail quietly, and several small robustness gaps
- A failed scheduled backup is logged and retried every five minutes for the rest of the window (`backup/schedule.go:62-66`); no alert or notification is raised, although the settings contract lists backup failure as an owner notification (§11.2).
- The free-space check is skipped on Windows (`backup/disk_other.go`), although `internal/diskspace` has a Windows implementation.
- `manifest.json` and the final rename are not synced to disk (`files.go:150-160`, `backup.go:420`). After a power cut a backup folder can exist with an empty manifest; `List` skips it and `Prune` never removes it, so it sits there unseen, holding space.
- Manual backups count toward the keep count (`backup.go:489-516`), so an owner's deliberate "before I change something" copy is deleted by the nightly prune after seven nights.
- The scheduler creates its backup directly rather than through `Start`, so the console's "running" indicator does not show it and a manual backup can run at the same moment.
- A window whose start time does not exist on the spring-forward day admits nothing (`workpolicy/policy.go:178-183`), so that night's backup is skipped; with the seeded 03:00 start this needs a zone that skips 03:00, which is rare.
- **Confidence:** Confirmed by reading. **Severity:** Low individually; Medium together because backups are the safety net.

### B-09.18 Backups can only live on the database's own volume
- **What goes wrong:** Every path in `internal/backup` is under `<state>/backups`. `02 Server.md` says `PORTICO_BACKUP_DIR` (and `PORTICO_CACHE_DIR`) exist; no code reads either. The settings contract requires the console to say that a same-disk backup is not an independent copy (§12.1); I found no such text in the English catalogue.
- **Confidence:** Confirmed by reading. **Severity:** Medium (the disk failure that destroys the database destroys every backup).

### B-09.19 A restore brings back old sessions, API keys and passwords
- **What happens:** The backup includes `supervisor/authority-incarnation.v1` and the whole database (`backup/files.go:21-36`); after a restore, verification retires playback state only (`apply.go:27-59`). Sessions and API keys revoked after the backup was taken, a member removed since, or a password changed since, all return to their state at backup time.
- **Why it is listed:** The settings contract (§12.2 items 7-8, and decision R11) requires a fresh "security incarnation" kept outside the database and the invalidation of every restored credential, plus quarantine of restored Hosted membership. The later backup spec replaced that with a deliberately simple model ("existing sign-ins from before the backup may no longer be valid") and marked the related tracker item won't-fix. The code follows the spec, and neither document acknowledges the other.
- **Confidence:** Confirmed by reading. **Severity:** Medium as a risk an owner should be told about (restore after a suspected compromise does not lock anyone out); Jellyfin's restore has the same property.

### B-09.20 On a filesystem without advisory locks, a restore restarts the server and then does nothing
- **What goes wrong:** If the instance lock cannot be taken because the filesystem does not support it, startup deliberately skips applying a staged restore and logs one line (`main.go:271-277`). The restore route does not know this: it stages, answers success and triggers the restart. The server comes back unchanged, with the staged copy still waiting, and the console shows no failed result because none was recorded.
- **When:** State folders on some NFS, SMB or container bind mounts, which the lock code itself calls out as realistic.
- **Confidence:** Confirmed by reading. **Severity:** Low (narrow), but confusing when hit.

### B-09.21 The server listens on IPv4 only unless the service definition is edited
- **What goes wrong:** The default, the Dockerfile, the systemd unit, the launchd plist and the Windows wrapper all use `0.0.0.0:32500`. `observeTopology` then discards IPv6 addresses ("an IPv4 wildcard does not accept IPv6", `networking/topology.go:251-255`), so the IPv6 pinhole and "IPv6 is open" features have nothing to publish. The console offers an IPv6 switch on the Remote page with no hint that the listener must be changed first. `PORTICO_BIND` also takes exactly one address.
- **Confidence:** Confirmed by reading. **Severity:** Medium (it silently disables a remote-access path the code otherwise supports, and it is the path that matters behind carrier-grade NAT).

### B-09.22 Shutdown does not wait for in-flight HTTP requests
- **What goes wrong:** `server.Shutdown` is called in a side goroutine (`main.go:976-985`). In Go, `Serve` returns as soon as `Shutdown` is called, not when it finishes, so `run()` returns at once and its deferred cleanup starts while requests are still being served. Nothing joins the side goroutine. On an idle server the deferred work is quick, the process exits, and the Hosted shutdown notice (2 s budget, started in that same goroutine) and any response still being written are cut off. The comment on `restartRequested` ("read when the HTTP server has drained", `main.go:154-156`) describes behaviour the code does not have. Plezy's relay shows the usual pattern: call `Shutdown` on the main path and continue only when it returns (`plezy/server/main.go:2696-2700`).
- **Confidence:** Confirmed by reading. **Severity:** Low (no data risk; clients retry), but it makes "graceful restart during playback" less graceful than intended and the shutdown notice unreliable.

### B-09.23 The log an owner can read is thinner than the design suggests
- **Early lines are missing:** the recorder is installed during the "administration" phase (`cmd/server/administration.go:27-28`, reached at `main.go:862`). Everything before that, including a failed or rolled-back restore, orphan cleanup, the memory and file-limit lines, toolchain problems and any fatal startup error, reaches stderr only, never the console or the rotated files.
- **Nothing before the last restart:** the console and the live tail read the in-memory ring (`servicelog.go:419-456`); the files are only reachable through the support bundle. After a crash the Logs page is empty of the evidence.
- **Retention in days applies to the ring, not the files** (`servicelog.go:376-395`); the files are bounded by size only (about 20 MiB), so the contract's "30 days plus bounded size" is really "bounded size".
- **Levels barely exist:** of about 120 log call sites, two use a level prefix and none log at debug. Opening a debug window or lowering the level therefore changes almost nothing.
- **No redaction step:** lines are stored as written. Current call sites look careful, but nothing enforces the contract's "redaction occurs before persistence" (§10).
- **Confidence:** Confirmed by reading. **Severity:** Medium for supportability.

### B-09.24 Four different answers to "who is the client behind this proxy?"
- **What goes wrong:** Locality for playback and remote policy uses `forwardedClient`: right-most untrusted hop, multi-hop aware, proxies from the environment **and** the settings document (`httpapi/request_locality.go:119-142`). Setup and sign-in use `privateSetupPeer`: proxies from the environment variable **only**, and the `X-Forwarded-For` header must hold exactly one address (`httpapi/onboarding.go:18-44`). The rate limiter's `clientAddress` also requires a single value (`rate.go:194-239`). The socket classifier uses the raw peer and no proxy knowledge at all.
- **Consequences:**
  - A proxy listed only in the console's Trusted proxies setting (with the environment variable empty) is honoured for playback but not for sign-in: any request carrying `X-Forwarded-For` is treated as "not private" during sign-in, which is safe but means owner-recovery sign-in "from the LAN" fails through that proxy. If the environment variable lists a different proxy, the same request is judged by the settings-only proxy's own private address instead and counts as private.
  - Behind two proxies (for example Cloudflare in front of nginx) the header has two addresses; sign-in treats everyone as remote and the limiter puts every user in the proxy's bucket, so one person's failed attempts slow everyone's sign-in.
  - A proxy or tunnel that adds no header at all (nginx's default `proxy_pass`, TCP-mode proxies, SSH or frp tunnels, Docker's userland proxy and rootless/Desktop Docker networking) makes every internet client look like a private or loopback peer. That client is then local for remote policy, may use plain HTTP, and may run first-run setup on an unconfigured server.
  - The `Forwarded` header (RFC 7239), `X-Real-IP` and `X-Forwarded-Host` are not understood.
- **Dead code:** `sameMachineSetup` and `localSetupHost` in `onboarding.go` are no longer called since setup was opened to any browser.
- **Confidence:** Confirmed by reading. **Severity:** Medium. The header-less-proxy case is inherent to trusting peer addresses; the inconsistencies are not.

### B-09.25 Unfriendly failures on two environment variables
- `PORTICO_TRUSTED_PROXY_CIDRS=127.0.0.1` (a bare address, the very example the console shows for the equivalent setting) fails to parse as a prefix and stops startup with "trusted proxy CIDRs must be explicit non-global networks" (`httpapi/rate.go:176-193`, `main.go:817-820`). The settings field accepts bare addresses.
- `PORTICO_BIND=:32500` or `localhost:32500` stops startup with "server requires an explicit IP bind", which does not name the variable (`main.go:230-233`).
- Both failures happen before the log recorder exists, so they are visible only on stderr.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-09.26 The container image: likely no hardware transcoding, no discovery on the default network, UTC schedules
- The runtime image installs `bubblewrap ca-certificates python3 tini` and the FFmpeg bundle, and nothing else (`Dockerfile`). VAAPI and Quick Sync need the vendor's user-space driver libraries at run time, which a slim Debian image does not have; the `portico` user is also not in a group that can open `/dev/dri` nodes. Unless the FFmpeg bundle carries those drivers itself, hardware conversion in Docker will fall back to software.
- mDNS multicast does not cross Docker's default bridge, so LAN discovery works only with host networking; nothing says so, and the image exposes only TCP 32500.
- The container's time zone is UTC unless `TZ` is set, so the seeded 03:00 maintenance window (backups) runs at 03:00 UTC.
- `python3` stays in the runtime image though it is only needed to verify the FFmpeg bundle at build time: extra size and attack surface.
- Docker's default bubblewrap restriction means the decoder sandbox usually cannot start in a container; the Dockerfile comment acknowledges this and the server falls back to baseline restrictions.
- **Confidence:** Probable for hardware transcoding (I did not inspect the bundle's contents); Confirmed by reading for the rest. **Severity:** Medium.

### B-09.27 The portable archives do not run as unpacked
- **What goes wrong:** The `.tar.gz` and Windows `.zip` contain the binary, FFmpeg, the web folder and the manifest (`scripts/package-release.sh:21-25`), with no launcher, unit file or readme. Run as-is, `PORTICO_WEB_DIR` is unset so there is no web interface (`httpapi/installation.go:33-35`), and `PORTICO_STATE_DIR` defaults to the development path `../../runtime/server` relative to the current directory (`main.go:214`), so state lands two folders above wherever the owner happened to be.
- **Confidence:** Confirmed by reading. **Severity:** Medium for those artifacts.

### B-09.28 The Schedule page says jobs run in maintenance windows; only backups do
- **What goes wrong:** The page text is "When the server does its heavier background work", jobs are labelled "Runs in {windows}" or "Not scheduled: no enabled window includes it" (`packages/i18n/src/catalog/en-US/settings.ts:252-257`), and a window lists tasks `library-scan`, `metadata-refresh`, `analysis`, `trickplay`, `backup`. In the server, the only consumer of windows is the backup scheduler (`backup/schedule.go:51`); `workpolicy.Allowed` returns true unconditionally and nothing starts scans, analysis or trickplay because a window opened.
- **Effect:** Moving "Library scan" out of every window changes the label to "Not scheduled" and changes nothing else. This follows from the deliberate "never paused" rule, but the page was not brought into line with it.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (truthfulness of a settings surface; the contract forbids stored-only settings).

### B-09.29 A failed start is invisible outside the service log
- **What goes wrong:** Any error in `run()` reaches `log.Fatal` (`main.go:124-130`): the listener closes and the process exits. Under systemd it is restarted up to five times in five minutes and then left failed. The reason (database newer than the binary, a digest mismatch, a missing authority file, a bad environment value) is in the journal or the redirected stderr file only. The startup reporter does record a failed phase with its error text, but the process is gone before anyone can ask.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium; see F-09.10.

### B-09.30 A long backup holds back the write-ahead log
- **Suspicion:** The backup pins one read snapshot for the whole copy (`backup/copy.go:36-44`) so the copy is consistent and never restarts. While it is pinned, checkpoints cannot move past it, so on a server that is writing heavily (a first import) the WAL grows for as long as the copy takes; the housekeeping loop's truncation needs readers to drain. The copy advances 16 pages per step with no pause, so it should be quick relative to the database size.
- **Confidence:** Speculative; needs a runtime check with a large database under a scan. **Severity:** Low.

### B-09.31 macOS launcher leftovers
- Deleting the app leaves `~/Library/LaunchAgents/tv.getportico.server.service.plist` pointing at a missing binary with `KeepAlive`, so launchd keeps retrying.
- Each time the app is opened and the generated plist differs from the installed one (the app was moved or updated), the launcher boots the agent out and back in (`launcher.sh:24-27`), restarting the server under anyone who is watching.
- The launcher always opens `http://127.0.0.1:32500/` after a fixed two-second sleep, whatever the configured port.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-09.32 `recover-owner` opens the live database without the instance lock
- **What goes wrong:** The command calls `persistence.Open` directly (`cmd/server/recover_owner.go:46`), which applies any migrations the binary carries, and does not take `server.lock`. Run with a newer binary against a folder an older server is still using, it would migrate the schema underneath the running server.
- **Confidence:** Confirmed by reading; the scenario needs mismatched binaries. **Severity:** Low.

### B-09.33 Health check versus "require HTTPS" on a specific bind address
- **Suspicion:** `--health-check` dials the bind address over plain HTTP, substituting loopback only for wildcard binds (`cmd/server/health_check.go:26-32`). If the server is bound to a specific LAN address and the owner sets secure connections to required on a claimed server, `/v1/readiness` falls under `enforceSecureConnections` (it applies to every `/v1/` path from a non-loopback, non-TLS client; `httpapi/secure_connections.go:178-192`) and answers 403, so the container would report unhealthy.
- **Confidence:** Speculative. **Severity:** Low.

### B-09.34 Public endpoints say a little more than the contract allows
- `GET /v1/system` returns the version, build id, source digest and build time to anyone (`httpapi/router.go:816-818`). Jellyfin's public info does the same and it is how scanners fingerprint vulnerable versions.
- During startup every request, authenticated or not, receives the phase list including raw error strings, which can contain filesystem paths (`cmd/server/startup.go:47-59,90-101`).
- The settings contract says public liveness and readiness endpoints must be "free of operational, identity, library, route and dependency detail" (§17).
- **Confidence:** Confirmed by reading. **Severity:** Low. Clients do need a compatibility signal before sign-in; a coarse API level would serve without naming the build.

### B-09.35 Moving only the database file does not work, and a mismatched restore is not rolled back
- **What goes wrong:** Startup refuses when `server.sqlite` exists but `supervisor/authority-incarnation.v1` does not: "existing server state has no current authority incarnation" (`cmd/server/networking_initializer.go:39-42`). An owner who copies just the database to a new machine, a natural reading of "it's plain SQLite", gets that message and no hint that the whole folder is needed.
- **Related suspicion:** Restore accepts a bare `.db` file and does not compare its server identity with the running server's. Verification after the swap checks only that the database opens, migrates and passes `quick_check` (`backup/apply.go:27-59`). The identity, claim-key and authority checks run later in startup (`main.go:303-379`); if they reject the combination of a foreign database with this server's key files, the restore has already been recorded as "restored", the rollback window has closed, and the server exits on every start.
- **Confidence:** Confirmed by reading for the first part; Speculative for the second (I did not trace which mismatches those later checks reject). **Severity:** Medium if the second part is real.

## Where Portico is stronger

Things worth keeping, and in several cases worth saying out loud in the product's own description.

1. **Remote access that mostly sets itself up.** PCP, NAT-PMP, UPnP and IPv6 pinholes, WAN detection without the vendor service, a per-server name and a publicly trusted wildcard certificate whose key never leaves the box. Jellyfin offers none of this; Plex offers most of it but through constant contact with its cloud.
2. **A safe default for a forwarded port.** Clear-text HTTP is simply not accepted from the internet, on the same single port that serves HTTPS.
3. **Backups that exist by default and a restore that can be undone.** Nightly, verified with `quick_check`, kept seven deep, readable in any SQLite tool; restore keeps a pre-restore copy and rolls back automatically, resumably. Jellyfin has no schedule and no rollback.
4. **Migration discipline.** Additive-only, digest-verified, no startup work that grows with the library, and an explicit refusal for newer databases.
5. **Starts honestly, stops within a budget.** The port answers with named phases before the database is open; shutdown is bounded and names what did not finish; one lock per state folder; a staged restore is never applied without that lock.
6. **Built to stay up.** Panic containment and supervised loops, a database watchdog, lanes with budgets and a busy answer instead of a pile-up, memory- and cgroup-aware limits set without asking the owner anything.
7. **Careful containment of FFmpeg.** A bundled, verified toolchain, a sandbox where the host allows it, lower priority and CPU/file-size limits for background work, orphan cleanup after a crash, children that die with the server on Linux and Windows.
8. **A settings model that cannot silently lose a change.** Typed registry, revisions, idempotency keys, apply modes, almost nothing needing a restart.
9. **Real diagnostics.** A bounded support bundle with secrets removed, a capability report, on-demand integrity check and repair, concurrency diagnostics, a hash-chained audit trail, and the owner's permanent play history.
10. **Release provenance.** Sealed source snapshots, a signed manifest in every package, checksums, SBOM and build attestation.
11. **Restraint.** No usage analytics; Hosted is contacted when something changes rather than on a clock; a container health check that needs neither `curl` nor credentials.
12. **A hardened Linux unit and a non-root container** as the starting point rather than as an afterthought (with the adjustments in B-09.14 and B-09.26).

## Doc/code disagreements

| # | The document says | The code does |
|---|---|---|
| D1 | `02 Server.md` (State folder): "Optional separate paths for cache, recordings and backups (`PORTICO_CACHE_DIR`, `PORTICO_RECORDINGS_DIR`, `PORTICO_BACKUP_DIR`)." | Only `PORTICO_RECORDINGS_DIR` is read (`cmd/server/main.go:573`). No code mentions the other two. |
| D2 | `02 Server.md`: generated files are "all referenced relative to the folder"; Spec — Backups §3: "referenced relative to the state folder, never by absolute path". | Paths are stored absolute and rewritten on start when the folder has moved (`persistence/rebase.go`; the comment at `main.go:334-335` says "storage stays absolute and the move repairs it"). |
| D3 | `02 Server.md` and `07 Hosted Services.md`: "Remote access is port forwarding, or none." | The server also performs automatic mapping over PCP, NAT-PMP and UPnP and opens IPv6 pinholes (`internal/networking/mapping*.go`). True in the sense that there is no relay; misleading about how much is automated. |
| D4 | `02 Server.md`: "one baseline migration". `persistence/README.md`: "Until the first release, a schema change edits `0001_baseline.sql` directly." | Eleven migrations exist (`0001`…`0011`) and `schemaVersion = 11` (`persistence/schema.go:24`). |
| D5 | Networking contract §7.1, §9 and decision P05: seven-day signed membership policy, daily renewal, a 30-minute heartbeat and a 45-minute presence lease. | "Hosted is contacted when something changed, not on a clock… There is no membership policy to renew" (`hosted/current_schedule.go:70-74`); the safety interval is six hours and the check-in bound eight days. The code matches the "Hosted stays cheap" principle; the contract was not updated. |
| D6 | Settings contract §12.2: restore requires interactive re-authentication, an exact-target confirmation, a fresh security incarnation outside the database, invalidation of all restored credentials and quarantine of Hosted projections; §12.3 defines a recovery mode. | Spec — Backups replaced this with "no owner proof… a plain confirmation" and the code follows the spec (B-09.19). The incarnation file is part of the backup (`backup/files.go:28`). There is no recovery mode; a restore that cannot finish makes the server refuse to start (`main.go:283-288`). |
| D7 | Settings contract §12.1: every backup has a checksum and a `restoreReady` result, and "Backups on the same disk do not satisfy the recommendation… Portico MUST make that limitation visible." Spec §1: the manifest lists files with hashes. | Ordinary backups carry no hash for the database, only for the small state files (`backup/backup.go:405-416`); there is no restore-ready field; I found no same-disk notice in the English catalogue. |
| D8 | Settings contract §10: default 30 days of server log history "plus bounded rotating size"; redaction before persistence; support export needs an owner-selected time window and components and "expires the private artifact". | Files are bounded by size only (five files of 4 MiB); day-based retention prunes the in-memory ring; lines are stored as written; the bundle is a fixed set streamed on request (`servicelog/bundle.go`). |
| D9 | Settings contract §2.2: "A visible writable field without a named runtime consumer and outcome test is prohibited." | `temporaryDirectory` (B-09.9), the maintenance-window task lists (B-09.28) and the alert thresholds (B-09.12) are visible and have no effect on a default install. |
| D10 | Settings contract §13: release state distinguishes `current`, `available`, `downloaded`, `ready`, `installing`, `restart-required`, `verifying`, `failed`, `rollback-available`, `manual-action-required`. | The report has `current`, `available`, `unconfigured`, `unavailable` and a few status strings (`administration/maintenance.go:500-558`). Reasonable for notify-only; the contract describes an updater that does not exist. |
| D11 | Settings contract §11.2: owner notifications include backup failure, low storage and certificate-expiry risk. | Backup failure raises nothing (B-09.17); the other two have no default trigger (B-09.12). |
| D12 | `cmd/server/main.go:139-141` (comment on `defaultBind`): "Setup still accepts only private peers". | `httpapi/onboarding.go:141-144`: "Any browser may start first-run setup (Justin, 23 Sep: no setup code, even remotely)". The helpers that enforced the older rule (`sameMachineSetup`, `localSetupHost`) are now unused. |
| D13 | `cmd/server/connection_cap.go:17-21`: past the cap "the kernel's own listen backlog absorbs the burst". `main.go:1047` logs the memory-derived ceiling. | With the direct listener in place, a fixed 256 cap closes excess connections (B-09.7). |
| D14 | `connectivity/status.go:246` treats 100.64.0.0/10 as "lan". | `networking/certificate_listener.go:183` and `httpapi/request_locality.go:100` do not (B-09.8). |
| D15 | `RELEASING.md`: "a small update manifest" is the "stable machine-facing location" for updates. | The server's update reader expects a different document shape (B-09.11). |
| D16 | `INSTALLING.md` lists six install routes with one line each. | Nothing in the repository documents environment variables, Docker options, GPU access, reverse proxies, moving a server, or `recover-owner`. The README points to getportico.tv for documentation, which I did not review. |

## Open questions for Justin

1. **State folder shape.** Should the single state folder stay strictly single, or should cache-class data (conversions, artwork, prepared media, trash) and backups be redirectable to other disks? The architecture document already promises two of those variables.
2. **Where the trash lives.** Is a recoverable delete worth copying large files onto the state volume, or should the trash sit beside the media it came from? And should "retention" mean files are removed automatically when it runs out?
3. **Tailscale and other overlay networks.** Should the owner's "home networks" list decide who may use plain HTTP, so a tailnet can be treated as home? Today nothing can.
4. **IPv6.** Listen on both families by default? This is the only direct route for owners behind carrier-grade NAT until a relay exists.
5. **Restore and credentials.** The contract says a restore must invalidate every old session and key; the spec says keep it simple. Which is the product's answer, and should the confirm dialog tell the owner that revoked access comes back?
6. **Restart ownership.** Should the server be able to restart itself in-process (and offer Restart/Shut down in the console), or is relying on the service manager the intended design everywhere, in which case Windows and the Docker instructions need to provide one?
7. **Windows shape before the companion app.** Ship the Scheduled Task as it is, switch to a proper service, or hold Windows back until the tray app exists?
8. **Reverse proxies.** Is "behind my own proxy on a sub-domain" a supported, documented configuration for release? Is sub-path hosting explicitly out?
9. **Update checks.** Should a default feed be built in so every server can say "a newer version exists", given that it is one static file fetched a few times a day?
10. **Third-party address services.** Is it acceptable that a server with remote access on asks Cloudflare, Google, Amazon and icanhazip for its public address, or should Portico's own DNS nodes answer that question?
11. **Schedules.** One mechanism or two? If windows cannot gate work by design, should the Schedule page stop presenting them as when work happens and become simply "when the nightly backup and clean-up run"?
12. **Alerts leaving the server.** With no email and no push, is a single owner-configured webhook acceptable so that "disk nearly full" or "backup failed" can reach someone who is not looking at the console?
13. **Public version disclosure.** Should `/v1/system` keep returning the exact version and build identity to unauthenticated callers?
14. **Pre-upgrade backup.** May a start that has migrations to apply spend time proportional to the database size to copy it first? That is one deliberate exception to "no library-sized startup work".
15. **Logs.** Is the very quiet log deliberate for the long term, or should conversions and scans gain debug-level detail that the debug window can reveal?
16. **Platform reach for release.** Which of these are in scope: Intel macOS, 32-bit ARM, an Unraid template, a TrueNAS app, Synology?

## Evidence boundary

- **Nothing was executed.** No server was started, no package was installed, no installer was run, no container was built. Every finding is a reading of source and packaging files as they stood in the working tree on 11 October 2026.
- **Windows behaviour is inferred.** The Scheduled Task defaults (B-09.4), SYSTEM's access to shares, the firewall prompt and the permission-bit behaviour of Go on Windows come from general platform knowledge, not from a Windows machine.
- **Package-manager script ordering** (B-09.3) is standard dpkg and rpm behaviour applied to the two scripts as written; no package was built or upgraded.
- **Capacity and timing claims are unmeasured.** How quickly 256 connections are reached, how long an integrity check takes per gigabyte, WAL growth during a backup, and the cost of the permission walk all need measurement on real hardware.
- **The FFmpeg bundle's contents were not inspected**, so whether GPU user-space drivers are included (B-09.26) is unknown.
- **Hosted, DNS and certificate issuance were not examined end to end.** The certificate order state machine and claim flow were read only where they touch the listener, renewal timing and retries.
- **The web console was read through its authored structure and one screen**, not screen by screen; a control I describe as missing may exist in a component I did not open. The React Native apps were only searched, not read.
- **Jellyfin's Windows and macOS packaging** live in repositories that were not cloned; statements about them come from Jellyfin's documentation. **Plex** statements come from memory of its public documentation and were not re-verified.
- **Line numbers** were checked against the working tree after writing; ranges are indicative of the function or block, not exact statement boundaries.

## Outside my domain

Noticed in passing; not investigated.

- **Static asset caching.** Only files named `assets/index-*` get an immutable cache header; other hashed chunks are served `no-cache` and revalidated on every load (`server/internal/httpapi/installation.go:84-87`). Belongs to the web client audit.
- **Owner routes authorise twice.** The route registry checks owner access before the handler, and many handlers then call `ownerContext` again after doing the work (for example every route in `httpapi/backup_routes.go`). It is redundant rather than unsafe, costs a second token lookup and database snapshot per request, and reads oddly because the second check comes after the side effect. Belongs to the identity audit.
- **`GET /v1/admin/play-history` and the unused session-history route** are already in `Backlog.md`.
- **Hosted learns three things about a server** beyond routes: its name when it changes, "storage nearly full", and "shutting down" (`server/internal/hosted/events.go`). Whether the storage event should exist at all, given that there are no alert emails, is a Hosted-domain question.
