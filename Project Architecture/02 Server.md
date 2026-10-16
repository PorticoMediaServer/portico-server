# Server

The local server is one Go module and binary. It owns catalogue, media, access and background work. It re-executes confined helper roles; FFmpeg provides analysis and conversion. A restart parent now supervises the worker, including portable and Docker launches. [Operations packaging](../../_program/reports/ops-packaging.md) records shutdown and cross-platform evidence.

## State and database

The state folder contains SQLite and Portico-owned files. Media roots are separate. `PORTICO_CACHE_DIR` and `PORTICO_BACKUP_DIR` select separate cache and regular-backup destinations. Pre-restore safety copies and pre-migration copies remain beside the database. Backups contain no active sign-ins; ordinary state is not encrypted at rest.

SQLite uses WAL, request read snapshots and short serialised writes with foreground priority. Catalogue workers publish bounded checkpoints. New databases use incremental reclamation rather than a whole-database vacuum. The pre-release baseline is edited directly and synchronised with the schema script. There is no promise to migrate an older programme database or preserve a retired API.

## API and integration

Routes declared on apikit have typed contracts, access rules, limits and credential scopes. The generated `server/api/portico.openapi.json` is the wire reference; shared services/parsers are in client-core. Registry migration is still in progress, so the toolkit's presence does not prove every historical route already uses it. Existing `/v1` spelling is provisional until release.

[API platform](../../_program/reports/api-platform.md) reports owner keys (`pk.`), personal tokens (`pat.`), digest-only secret storage and four scopes: read, playback, library and admin. Role, libraries and profile restrictions remain live checks. Credential-management and sign-in actions require sessions; integrations cannot mint more credentials. Keys have independent lifetimes and survive password changes and ordinary sign-out. Restoring a backup can restore a previously revoked key; the owner must review credentials afterwards.

Browse totals are exact up to 10,000. Past that, a posted filter's page says `pageInfo.totalIsLowerBound: true` with `total: 10000`, and grouping entries carry `totalAtLeast`; cursors and direct ranges keep paging past the threshold, so counting never grows with a huge matching set (Justin, 14 Oct; [perf-reads](../../_program/reports/perf-reads.md)). Lists that span libraries (`POST /v1/browse`: a person's titles, playlists, personal collections, Home rows, My List, Favorites, History) read their bounded group first, at most 10,000 members ([cp-browse-parity](../../_program/reports/cp-browse-parity.md)).

`GET /v1/titles` supplies a bounded supplemental query. `POST /v1/libraries/paths-changed` requests narrow scans. Local webhooks use HMAC-SHA256 over timestamp and body, persistent retries and delivery logs. `/v1/mcp` exposes scoped tools through the same API authority. [API review](../../_program/reports/review-api-platform.md) and [server follow-ups](../../_program/reports/server-followups.md) distinguish open scheduler/query/admission findings from later attention tools. These surfaces do not establish full release API coverage.

## Identity and access

Accounts and profile restrictions gate reads and mutations. Remote sign-in policy applies at session creation, using the owner's home networks. Session families bind to a device record and a random installation ID. Access tokens are short-lived bearer credentials; rotating refresh tokens also require the installation ID. No device owns a possession key. Captured bearer credentials and installation ID can be used by their holder until revocation or refresh-reuse detection; plain HTTP offers no confidentiality.

TV code sign-in carries the approver's role. A Cast target instead receives a playback-only member session. Permission changes apply at server admission. Hosted signing-key provenance follows admitted session descendants; learned revocation retires affected families while direct/recovery families remain independent. The heartbeat consumer is present in `internal/hosted/current_schedule.go`; fleet convergence still needs real acceptance.

The owner keeps a separate permanent play history. Pausing or clearing a viewer's personal history does not erase that record. Deleting a member retains owner plays while removing names. Backups scrub sessions; restore signs people out. API-key restore behaviour is the separate exception above. [Accounts/security](../../_program/reports/accounts-security.md) and its [review](../../_program/reports/review-accounts-security.md) record remaining restricted-profile and failed-attempt-retention gaps. Country rating tables, a per-library unrated or allow-list kids mode and unified profile/account label rules are not planned (dropped by Justin, 15 Oct).

## Freshness and work

One authenticated `/v1/events` feed carries scoped catalogue, personal, progress, settings, receiver and notification hints, and `list.changed` hints (resource `list:<name>`) for every owner list: keys, tokens, webhooks and their deliveries, members, invitations, devices and the rest. Administrators also receive `admin`-audience hints, so owner screens refresh from hints rather than a clock ([cp-owner-parity](../../_program/reports/cp-owner-parity.md)). Browse generations and search FTS slots retain readable content during rebuild. Ordering, directory, personal command and position counters have separate meanings. Continuations recover quietly where possible. Unpublished restriction classes may return protected empty content rather than disclose stale private data. Owners can read bounded derivation failures. See [freshness-server](../../_program/reports/freshness-server.md).

Gentle, Normal and Fast control background pacing. The owner can explicitly pause background work for one hour or resume it. Playback causes pacing, not indefinite cancellation. Backup timing is separate from general background work. Some pause boundaries and the deep-analysis outer time cap remain follow-ups in [limits/settings](../../_program/reports/limits-settings.md).

## Connectivity

HTTP and TLS share the listening port, default 32500, on IPv4/IPv6. HTTP is accepted from any peer. `lanNetworks` is the complete home-network list; an empty list makes all clients remote. Trusted-proxy configuration alone authorises forwarded address/scheme/host evidence. Owner certificates reload while retaining the previous valid pair after an invalid replacement. Manual owner addresses coexist with direct names; only TLS candidates reach Hosted. There is a bounded connection reserve for sign-in and required console reads. This is measured admission protection, not a guarantee against host exhaustion. See [networking](../../_program/reports/networking.md) and the completed [networking review fixes](../../_program/reports/review-networking.md).

## Verification

Use focused package tests and the report's cold acceptance procedures. No full suite, deployment, native package qualification, GPU/tuner proof or large-load guarantee is claimed by this document.
