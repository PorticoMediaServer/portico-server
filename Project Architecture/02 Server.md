# Server

`apps/portico-server/server` is one Go module (`portico.local/server`) that builds one binary, `portico-server`. The binary is also its own helper: it re-executes itself as the confined decoder wrapper (`--portico-media-exec`, `--portico-decoder-linux`), so a deployment installs nothing else except FFmpeg.

## Process

`cmd/server/main.go` opens the state folder, migrates the database, wires the services, and starts the HTTP listeners and background loops.

- **Every goroutine starts through `supervise.Go`** (`internal/supervise`). A panic in a background loop is contained, logged and reported, instead of taking the process down. A guard test rejects a bare `go` statement.
- **Background loops wait, work, wait** (`internal/worker`): each loop sleeps until a database change or a timer wakes it (`dbwork.WakeOnTables`), so an idle server does no polling work.
- **Shutdown** is `signal.NotifyContext`: listeners drain, loops stop, the database closes. Exit code 75 asks the service manager to start the server again (used after a staged restore).

## State folder

`PORTICO_STATE_DIR` holds everything the server owns. It can be copied to another machine and started there.

- `server.sqlite`: the database (below).
- Artwork, generated media (HLS segments, conversions, thumbnails), analysis artifacts, subtitles, downloads, logs and backups, all in their own subfolders, all referenced relative to the folder.
- Optional separate paths for cache, recordings and backups (`PORTICO_CACHE_DIR`, `PORTICO_RECORDINGS_DIR`, `PORTICO_BACKUP_DIR`).

Backups are plain SQLite copies plus the files they reference, openable in any SQLite viewer.

## Database

One SQLite database in WAL mode (`internal/persistence`), with one baseline migration, `migrations/0001_baseline.sql`. [`persistence/README.md`](../../apps/portico-server/server/internal/persistence/README.md) has the migration rules: additive only, no startup rewrites of large tables, derived data rebuildable, permanent enum numbers, kinds as data.

**Concurrency** (`internal/dbwork`):

- Reads run on a pool of read connections. Request reads use one read snapshot per request (`WithReadSnapshot`), so a page is internally consistent.
- Writes go through one application-level write gate that orders writers by work class. Foreground writes (a watch, a rating, a settings change) go ahead of background batches, and background transactions are short. This is how browsing and playback win against a scan without anything being paused.
- BUSY/LOCKED retries are typed and bounded. A background-pressure signal lets bulk loops yield.
- Statement guards in tests prepare every SQL statement against the schema and reject full scans on large tables outside an allowlist.

## HTTP API

`internal/httpapi` serves the API under `/v1` (and `/v2` for a few newer surfaces). Routes are declared in a registry built with **apikit** (`apps/portico-server/apikit`): each route states its method, path, request and response schemas, errors and limits. `cmd/apigen` generates the OpenAPI document (`server/api/portico.openapi.json`) and the TypeScript contract package (`packages/contracts`) from it, and CI fails if either is stale. Hand-written OpenAPI files in `server/api/` document surfaces not yet on the registry; `internal/apispec` checks real answers against them.

Conventions every route follows:

- **Cursors, not offsets.** Lists page with opaque, signed cursors that carry the revision they were read at. A cursor from an older revision answers `409 stale_continuation`, and the client restarts.
- **Revisions.** Documents carry the catalogue and viewer revisions they were built from; clients use them for conditional reads (`ETag`/`If-None-Match`) and to discard stale pages.
- **Idempotency.** Mutations accept an idempotency key; a replay returns the recorded receipt.
- **Bounded everything.** Every request and response field has a documented limit, enforced on both sides.

The reference is [`server/api/README.md`](../../apps/portico-server/server/api/README.md) and its guide (`server/api/guide/`).

## Identity and access

`internal/identity` and `internal/access` own people:

- **Profiles** belong to the server. A profile may be linked to a Portico Account (Hosted) or be local only. The owner is the admin tier; members are invited.
- **Sessions** are device-bound: a device registers a key, and access tokens are short-lived and renewed with a refresh token bound to that device and installation. Session families let one sign-out end every token issued from a sign-in.
- **Sign-in methods:** direct (username and password, with optional TOTP 2FA), Portico Account through Hosted, and a TV code approved from a signed-in phone or computer (TV only). A device linked with a code is signed in **as whoever approved it**, with that person's role: an owner's Apple TV is an owner's session, a member's a member's. A session that is itself viewer-only links a viewer-only device. A correct password always signs you in; failed attempts get a cool-off, never a lockout or email code.
- **Restrictions** (Kids profiles, content ratings, library access) are applied in the catalogue's read paths, not by filtering results afterwards.

## Networking

`internal/networking` and `internal/connectivity`:

- The server listens on port 32500 (HTTP, and HTTPS when it has a certificate).
- **Remote access** is port forwarding, or none. The server learns its public address and publishes access URLs to Hosted when they change.
- **Certificates:** a claimed server gets a name under `direct.getportico.tv` and a certificate for it through Hosted (ACME DNS-01, Google Public CA). Owners can also supply a custom certificate. HTTP stays available unless the owner requires HTTPS.
- **Claim** connects a server to a Portico Account: the server prepares an unbound claim, the owner approves it in the browser (`claimweb`), and the server then holds a signed binding. `internal/hostedtrust` verifies Hosted's signatures against the official root compiled into release builds.
- Trusted proxies, "treat WAN as LAN", upload speed and an advertised interface are owner settings.

## Other services

| Package | Role |
|---|---|
| `storage`, `mounts`, `remotesources`, `sourceaccess` | Library folders on local disks, network mounts and rclone remotes; the folder picker; physical root registration |
| `ingestion`, `scanevents` | Scanning: discovery, identity of files, the inventory contract (`ingestion/INVENTORY_CONTRACT.md`) |
| `downloads` | Offline downloads for the apps: requests, preparation, bounded storage |
| `livechannels`, `linearinput`, `linearbuffer`, `recordingaccess`, `recordingmedia` | Live TV (HDHomeRun, M3U, Xtream), the guide, DVR recordings, and Library Channels (owner-scheduled channels from the library) |
| `social` | Watch Together groups and remote playback |
| `catalog` (`play_history.go`) | The server's **play history**: every play (who, what, which device, when, how far), written in the playback transaction once a play has really started. It is the owner's record, kept in full unless `playHistoryDays` is set (Storage & backups › What Portico keeps), and read at `GET /v1/admin/play-history` with filters for person, library and time. A viewer's own History is separate (`personal_history`): theirs to pause and clear. Play counts per title (`play_counts`) and per viewer (`personal_play_counts`) are kept from completed plays and outlive pruned history. **The owner always sees every play** (2 Oct 2026): a viewer pausing or clearing their own History never removes a play from the owner's record, and nothing deletes it unless the owner sets a retention (default: forever). The Dashboard's Viewing figures are totals of this table (`GET /v1/admin/play-history/summary?period=24h|7d|30d|all`), so they go back as far as the history does; each play records whether the server converted it (`converted`). |
| `notify`, `servicelog` | In-app notifications (no push, no email from the server) and the server's message log |
| `operations`, `telemetry`, `diagnostics`, `capabilityreport` | The owner console: host, storage, network and GPU facts, logs and a bounded support export |
| `backup` | Scheduled plain backups and restore |
| `enrichment` | The title enrichment dataset client ([Recommendations](05%20Recommendations.md)) |

## Tests

`go test ./...` from `server/`, on the runner: `scripts/runner-test.sh` ([Operations](08%20Operations.md)). Tests are tiered with `PORTICO_TEST_TIER`. `default` is the everyday suite. `media` adds real-FFmpeg, Live TV, Library Channels and real-time tests. `PORTICO_PERFORMANCE_TIER` runs the scale proofs. `internal/catalogtest` builds deterministic catalogues, including large synthetic ones, for tests and scale runs.
