# Foundation: ownership, authorization and resources

Contract ID: PC-FOUNDATION. Revision: portico.october.1. Current intended behavior; implementation evidence remains separate.

## Stable owner versus current permission

A durable viewer owner is `(authority, accountId, profileId, serverId)`. Authority is `hosted` or `local`; identical identifier strings under different authorities are distinct. In a database belonging to exactly one server, server ID may be implicit in that database boundary. An internal stable viewer ID is permitted only with a unique, immutable mapping to the full tuple. Raw externally issued profile ID alone is insufficient.

Hosted account-only state is `(hosted, accountId[, profileId])` with no invented server. Authenticated account session review and profile management must work before a viewing profile is selected. Local identity is always server-bound. Account settings/session families are account-owned; media viewing state is profile-owned. Device IDs are metadata or proof-key bindings, never a substitute for an account principal.

Authorization evidence is separate: Hosted policy revision/expiry, server-local restriction revision, profile/PIN revision, credential/security incarnation and client selection generation each fence their relevant work. They are not interchangeable counters or part of durable personal-data identity. An authorization revision change invalidates old requests/grants/cache eligibility and re-evaluates access; it does not erase or fork playlists, recordings, watched state or coherent offline bytes. Actual revocation/erasure follows the owning contract. Cache keys and request envelopes include both the stable owner and relevant version evidence.

Every repository read, write, cursor, share, event and idempotency receipt must bind the correct stable owner/resource and verify applicable current authority. A role field copied into an old session is not permanent authority. Server-only audit and catalog facts have their own explicit owner; they must not be made profile-private or globally public by accident.

## Permission intersection

Hosted owns global identity, server ownership association, memberships and synchronized permissions. The local server owns its durable data and is the final enforcer. Effective permission is the intersection of current membership/ownership, profile rules, server-local restrictions, library/media/resource access and the operation's explicit policy. Unknown permissions deny; clients cannot create permission meaning from labels.

Local owner restrictions commit with local revision and a durable sync operation before acknowledgment and apply immediately. A later broader Hosted snapshot cannot clear an unacknowledged restriction. Relaxation requires an online revision-checked Hosted commit and matching signed local application. Restrictive tombstones survive retries and restart. Seven-day policy expiry and same-revision daily renewal follow Networking. Restored old policy cannot use normal outage continuity to override restore quarantine.

Account stream caps apply across all that account's profiles/devices on this server. Default limits are unlimited. Optional owner-set per-account and total caps, transcoding toggle, replacement-slot transfer and forward-looking cap changes are exactly document 32. Security revocation and real source failures remain enforceable; predicted CPU, RAM, disk, bandwidth or decoder pressure must not become an implicit playback start veto or eviction policy.

## One scheduling order

| Priority | Work class | Owner and purpose |
|---|---|---|
| 1 | security-fence | Current authorization, revocation, restore fencing and explicit emergency stop |
| 2 | protected-capture | Accepted recording reservations and writes under the explicit DVR policy |
| 3 | established-playback | Authorized active media delivery and continuity |
| 4 | playback-start | Accepted starts, explicit player commands and foreground preparation |
| 5 | interactive | Browse, search, ordinary settings and personal mutations |
| 6 | foreground-transfer | Download preparation and transfer |
| 7 | background-media | Inventory, matching, analysis, artwork acquisition and optimization |
| 8 | maintenance | Routine backup, compaction, pruning and cleanup |

This is an execution order, not eight global mutexes or a resource-prediction admission gate. Use separately bounded control/DB/read/process resources, short transactions, cooperative quanta, per-source fairness and durable queues. A hung low-priority filesystem/helper request must not hold the only path used to revoke authority or serve a healthy source. Observe starvation and let background jobs progress when higher-priority work permits; do not promise impossible latency under failed hardware or unlimited real demand.

Owner-configured tuner reservations and actual exclusive hardware claims are distinct from predictive overload. Preserve the original explicit DVR reservation/countdown policy. A known finite tuner cannot serve two incompatible tunes simultaneously. An unknown device count is capability uncertainty, not evidence that an ordinary media server should refuse all additional playback. See Live/DVR for discovery and owner override.

Background analysis and optional downloads may wait for execution/storage budgets. Existing authorized playback is not killed to make space for those jobs. Actual failed reads, allocation failures or full storage are reported honestly; no fabricated successful session or endless hidden retry.

## Server-first product

The selected server owns browse/search/filter/sort/facets, recommendation membership/ranking, media hierarchy, playback, personal state, downloads, guide/schedule, rooms, settings and application events. Clients own presentation and pending user intent. Offline clients may use validated local artifacts and cached server projections under their own contracts; they do not invent fresh server recommendations.

Hosted receives only the minimum global identity/directory/permission, opaque wake and network coordination facts. No titles, progress, queues, rooms, library structures, source credentials or media delivery are sent to Hosted for convenience. Opaque library restriction IDs needed for membership enforcement are allowed; names and catalog contents are not. Global account management remains Hosted even when opened from a selected-server UI.

No new per-playback Hosted dependency, metadata broker, mandatory managed database, distributed application platform or paid feature is introduced. Initial Hosted remains self-managed PostgreSQL on one VPS, public DNS separate, Hosted behind Cloudflare, future relay separate infrastructure.

## Cross-domain references

Durable mechanics are in [architecture.md](../architecture.md). PC-PLAYBACK document 32 owns playback identifiers/timelines/leases; other features consume them and do not redefine them. [decisions.md](../decisions.md) records preserved product choices and reconciled implementation assumptions. [conformance.md](../conformance.md) supplies cross-boundary failure scenarios in addition to the preserved domain cases.

## October scope and evidence

Separate progress, personal-command, browse-order and directory counters govern freshness. Scoped keys/tokens and local webhooks/MCP are implemented. Background work has Gentle/Normal/Fast pacing and an explicit one-hour owner pause. Do not equate all changes with one authorisation revision, promise device possession keys or require encryption at rest.

Sources: [freshness-server](../../../_program/reports/freshness-server.md), [api-platform](../../../_program/reports/api-platform.md), [limits-settings](../../../_program/reports/limits-settings.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
