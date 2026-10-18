# 08 — The HTTP API as a product: conventions, contracts, real-time channels and extensibility

Audit date: 11 October 2026. Static source review only (nothing was built, run or called).

**Path shorthand used throughout.** Portico paths are relative to `/home/justin/projects/Portico`. To keep citations readable:

- `server/…` means `apps/portico-server/server/…`
- `httpapi/…` means `apps/portico-server/server/internal/httpapi/…`
- `apikit/…` means `apps/portico-server/apikit/…`
- `client-core/…` means `apps/portico-server/packages/client-core/src/…`
- Comparison repos are cited as `jellyfin/…`, `plezy/…`, `navidrome/…`, `streamyfin/…`.

**Jargon, defined once.**

- *Route / operation*: one method + path the server answers (for example `GET /v1/items/{id}`).
- *Registry*: Portico's typed route table (`apikit`). A route declared there states its access rule, limits and schemas, and the OpenAPI file and TypeScript types are generated from it.
- *Legacy handler*: a Portico route registered straight on Go's `http.ServeMux` with a hand-written handler, documented in a hand-written OpenAPI YAML file.
- *SSE* (server-sent events): one long HTTP response the server keeps writing small text frames to. One direction only (server to client).
- *Long-poll*: the client asks "anything new since X?", the server holds the request open until there is something (or a timeout), answers, and the client asks again.
- *WebSocket*: one long-lived two-way connection.
- *Envelope*: the fixed outer JSON shape of an answer (for example `{"error":{…}}`).
- *ETag / conditional request*: the server labels an answer with a tag; the client sends the tag back (`If-None-Match`) and gets a bodyless `304 Not Modified` if nothing changed.
- *Idempotency key*: a client-chosen id on a write so that a retried request is applied once.
- *Cursor*: an opaque "continue from here" token for paging.
- *Fan-out*: the work the server does to tell N connected clients about one change.

## Scope and sources

**What I compared.** How each project designs its HTTP API as something other people build on: resource shapes, contracts and generated code, versioning, errors, paging, limits, auth header design, CORS, caching; the real-time channels (what is pushed, over what, with what resume and keep-alive behaviour, at what cost); and extensibility (plugins, webhooks, scheduled tasks, API keys, compatibility protocols, third-party ecosystem).

**Portico files actually read (not just listed).**

- Docs: `portico-internal/Project Architecture/02 Server.md`, `Backlog.md`, `Specs/Spec — API v1 Style Guide.md`, `Specs/Spec — Link Contract.md`; `server/api/README.md`; `server/api/guide/01-overview.md`, `06-events.md`, `README.md` (and grep of 02/04); headers of `server/api/openapi.yaml`; `server/api/admin-access.md` (API keys section).
- Toolkit: all of `apikit/registry.go`, `apikit/apierror/error.go`, `apikit/cursor/cursor.go`, `apikit/cursor/keyring.go`, `apikit/idempotency/store.go`, `apikit/precondition/precondition.go`, `apikit/contract/fixture.go`, `apikit/templates/*`; `server/cmd/apigen/main.go`.
- Router and middleware: all of `httpapi/router.go`, `foundation.go`, `playback_v1.go`, `events_routes.go`, `admission.go` (most), `rate.go`, `compression.go`, `cache_headers.go`, `grid_etag.go`, `revision_presence.go`, `saved_revision.go`, `connection_context.go`, `write_deadline.go`, `installation.go`, `media_original.go`, `request_locality.go` (most), `openapi_coverage_test.go`; large parts of `notifications_routes.go`, `administration.go`, `route_lanes.go`, `secure_connections.go`, `onboarding.go`, `social_routes.go` (group stream), `admin_access.go` (API keys), `admin_diagnostics.go` (client logs).
- Events and background: all of `server/internal/eventfeed/feed.go`, `apievents/events.go`, `scanevents/scanevents.go`, `worker/worker.go`, `worker/broadcast.go`; parts of `playbackv1/events.go`, `sessions.go`, `sessions_change.go`; `operations/jobs.go` (job and schedule model); `cmd/server/console.go` (job kinds), `cmd/server/main.go` (origins, HTTP server settings), `cmd/server/transport_policy.go`; `notify/producers.go` (categories); `access/apikeys.go` (head); `catalog/discovery.go` (cursor signing).
- Clients: `client-core/sse.ts`, `console-transport.ts`, `bounded-json.ts`, `attention.ts`, `public-links.ts`, `playback-v1/events.ts`, the request layer in `client-core/index.ts` (lines 125–440), heads of `notifications.ts`/`inbox.ts`; `apps/portico-server/web/src/app/device-events.ts`, `library-scan.ts`, `player/engine.tsx` (events wiring); `apps/portico-react-native/apps/native/src/bridge/useNativePlayback.ts` (events wiring).
- I also ran two read-only scripts (kept in the session scratchpad, not in the repo) that list every route registration in `httpapi` and `networking`, and compare the hand-written OpenAPI operations with the code.

**Comparison files actually read.**

- Jellyfin: directory survey of `Jellyfin.Api/**`; `Jellyfin.Server.Implementations/Security/AuthorizationContext.cs`; `Jellyfin.Server/Extensions/ApiServiceCollectionExtensions.cs` (policies, CORS, forwarded headers, Swagger setup); `Jellyfin.Server/Configuration/CorsPolicyProvider.cs`; `MediaBrowser.Model/Session/SessionMessageType.cs`; `Emby.Server.Implementations/Session/SessionWebSocketListener.cs` (keep-alive constants); `Emby.Server.Implementations/EntryPoints/LibraryChangedNotifier.cs` and `UserDataChangeNotifier.cs` (batching); `Emby.Server.Implementations/Plugins/PluginManager.cs` and `Updates/InstallationManager.cs` (grep-level); `Jellyfin.Api/Middleware/ExceptionMiddleware.cs`, `ResponseTimeMiddleware.cs`; route attributes of `ScheduledTasksController`, `PluginsController`, `PackageController`, `ApiKeyController`, `ClientLogController`; `ItemsController.GetItems` parameter count; `MediaBrowser.Model/Configuration/ServerConfiguration.cs` (CORS and legacy-auth defaults). Beyond that I rely on prior knowledge of Jellyfin's API and say so where it matters.
- Plezy: `lib/media/media_server_client.dart` (interface, first ~250 lines), `lib/media/server_capabilities.dart`, `lib/media/library_change_event.dart`, `lib/services/api_cache.dart` (head).
- Navidrome: `server/events/sse.go` (constants and structure), `server/subsonic/opensubsonic.go`, and the **Jellyfin-compatible API layer** `server/jellyfin/` (README, `api.go` route table, `socket.go`).
- Streamyfin: `package.json` (uses `@jellyfin/sdk`), presence of `providers/WebSocketProvider.tsx`.

**What I did not get to.** The Hosted API (same style guide, different codebase). Portico's `social` package internals (group ledger, handoff state machine) beyond the stream handler. The `apispec` validator's coverage (which routes' answers are actually checked against the hand-written schemas). The `packages/contracts` tests. Jellyfin's `SessionManager` send path and `WebSocketConnection` in detail, the Kotlin/TypeScript SDK generators, jellyfin-web's plugin page loader, Finamp, Audiobookshelf's Socket.IO layer, and the closed products (Plex, Emby, Infuse) beyond general knowledge. No runtime behaviour of any kind.

## How Portico does it today

### The shape of the API

Portico's server answers roughly **566 operations**, almost all under `/v1`. They come in two generations that live side by side:

| Generation | How many | How it is declared | How it is documented |
|---|---|---|---|
| Typed registry (`apikit`) | 66 operations | One Go struct per route: id, method, path, access rule, lane, cost class, body limit, allowed query keys, error codes, typed request and response (`apikit/registry.go:52-69`) | Generated: `server/api/portico.openapi.json` (OpenAPI 3.1) and `packages/contracts` TypeScript (`server/cmd/apigen/main.go`) |
| Legacy handlers | about 500 operations | `mux.HandleFunc("METHOD /path", func…)` in ~90 files of `httpapi` | Fifteen hand-written YAML files in `server/api/` (532 operations, of which 32 duplicate registry routes) |

A test (`httpapi/openapi_coverage_test.go`) scans the Go source for route strings and fails if a route has no entry in a YAML file, so the *list of routes* cannot drift. It does not check schemas.

The API is **screen-shaped**. The server composes whole documents for a screen: `GET /v1/bootstrap` (server, capabilities, viewer, libraries, preferences, home layout in one answer, `httpapi/foundation.go:41-48`), `/v1/home`, `/v1/libraries/{id}/content`, title workspaces, the guide directory. Rows carry their own presentation (`poster|square|landscape|circle`), localisable titles as `{code, params, fallback}`, and artwork objects. Clients are told not to filter, sort or pick layouts themselves (Style Guide amendments 5–7).

### Conventions (as written, and as built)

- **Strict requests.** Registry routes reject unknown JSON fields, duplicate keys, wrong-case keys, trailing data, wrong content type and any query key not on the route's allow-list (`apikit/registry.go:169-184`, `426-448`). Path ids must be `[A-Za-z0-9_-]{1,128}` (`registry.go:269-277`).
- **No `null` in responses.** The registry walks every response by reflection and refuses to send one that would contain a `null` (a nil list, map or interface) — it answers `500 internal` instead (`registry.go:412-425`, `580-674`). Responses over 512 KiB are also refused with a 500 (`registry.go:421`).
- **Errors.** There are several envelopes in service today (see A-08.9): the registry's `{"error":{code,message,retry,requestId,fields?,current?,reason?,retryAfterSeconds?}}` (`apikit/apierror/error.go:37-49`) and the older `{"error":{code,message,retryable}}` written by `failure()` (`httpapi/router.go:234-561`), plus per-family variations.
- **Paging.** Cursors are HMAC-signed, scoped to the viewer and query, expire after 30 minutes and carry the catalogue and viewer revision they were issued at; a revision change answers `409 stale_continuation` (`server/internal/catalog/discovery.go:93-162`).
- **Concurrency.** Writes carry `expectedRevision` in the body (legacy) or `If-Match` (registry); a missing revision is `428 revision_required` (`httpapi/revision_presence.go`, `apikit/precondition/precondition.go`).
- **Idempotency.** Each family has its own key and replay store: `Idempotency-Key` header on Playback v1, `operationId`/`requestId`/`idempotencyKey` body fields elsewhere.
- **Limits.** Every request passes an admission gate that sorts it into one of twelve *lanes* (auth, browsing, expensive, playback, media, media-body, bulk-transfer, realtime, admin, admin-heavy, security-fence, default), each with a concurrency cap, a bounded queue, a per-client fair share and a per-request time budget (`httpapi/admission.go:27-83`). A full lane answers `503 server_busy` with `Retry-After: 1` (`admission.go:512-531`). The lane of every route is listed in one table that a test keeps exhaustive (`httpapi/route_lanes.go:3-11`).
- **Caching.** Everything defaults to `Cache-Control: no-store` (`router.go:1247`). Registry `GET`s get an ETag that is the SHA-256 of the body and `private, no-cache` (`registry.go:391-405`). Grids use a tag built from a process-wide write counter (`httpapi/grid_etag.go:41-43`). Home and title detail use exact revisions (`httpapi/detail.go:92`, `home_routes.go:133`).
- **Auth on the wire.** `Authorization: Bearer <access token>`; the device describes itself in `X-Portico-Installation-Id`, `-Device-Name`, `-Device-Platform`, `-App`, `-App-Version` headers (`router.go:1283-1297`). Media bytes use *capability URLs* (`/v1/media/{grant}/…`): the unguessable grant in the path is the authority, and it is re-checked every 100 ms while bytes flow (`router.go:1355-1369`).
- **CORS.** A request with an `Origin` header is refused (`403 origin_not_allowed`) unless the origin equals the request's own scheme+host, is `https://web.getportico.tv` or `https://cast.getportico.tv`, or is listed in `PORTICO_ALLOWED_ORIGINS` (`router.go:1248-1259`, `server/cmd/server/main.go:551-556`).
- **Security headers** on every response: CSP, `X-Frame-Options: DENY`, `nosniff`, `Referrer-Policy`, HSTS on TLS (`router.go:1240-1246`).
- **Capabilities.** `GET /v1/capabilities` returns per-viewer feature states (`enabled | disabled_by_owner | not_permitted | unavailable`) and a handful of numeric limits; `GET /v1/server` returns `api: {major: 1, level: 1}` (`foundation.go:149-229`, `298-302`).

### Real-time channels

There are five long-lived transports, all admitted through the *realtime* lane (512 concurrent, no queue, no time budget, never compressed):

| Channel | Transport | Carries | Resume |
|---|---|---|---|
| `GET /v1/events` | SSE, or long-poll with `?after=&waitSeconds=` (max 25 s) | The unified feed: `notification.changed`, `operation.updated`, `library.scan.updated`, `job.updated`, `session.updated`, `queue.updated`, `admin.sessions`, `stream.resync` | Numeric event id; `Last-Event-ID` or `after`; a durable ring of 20,000 rows in SQLite (`apievents/events.go:12`) |
| `GET /v1/notifications/events` and `/wait` | SSE and long-poll (max 60 s) | Inbox changes with data, plus `receiver.inbox_changed` hints | Inbox revision |
| `GET /v1/groups/{id}/events` | SSE | Watch Together group timeline | Durable ordinal, `resume-gap` |
| `GET /v1/receivers/{id}/inbox/wait` | long-poll | Remote-playback commands for a receiver device | — |
| `GET /v1/admin/logs/events` | SSE | Server log tail | `recorderNonce:sequence` |

How `/v1/events` works end to end:

1. A domain write appends a row to `api_events` in its own transaction, addressed to an *audience*: `profile:…`, `device:…`, `account:…`, `library:…`, `group:…` or `admin` (`apievents/events.go:14-65`). Every 64th insert trims the ring.
2. A single loop (`eventfeed.Hub.Run`) is woken by commits that touch `api_events`, `notification_inbox` or `console_operations`, at most once a second (`eventfeed/feed.go:76-86`, `worker/worker.go:104`), and nudges **every** connected reader (`feed.go:88-97`).
3. Each reader re-proves its sign-in against live state, reads its page of events from SQLite (filtered by audience, and by library visibility for `library:` rows), and writes them (`httpapi/events_routes.go:186-245`, `feed.go:137-226`).
4. Events are *invalidations*: "this resource is now at revision N". The client refetches.
5. A stream sends a comment heartbeat every 25 s (re-checking authority each time), bounds each frame write to 20 s, ends itself after 30 minutes with `stream.closed`, and allows one stream per account+installation (`events_routes.go:17-18`, `209-231`; `feed.go:113-135`).

**Important finding:** no first-party client uses the SSE form of `/v1/events`. The web app builds `new EventsClient({http})` with no stream opener (`apps/portico-server/web/src/player/engine.tsx:120`), and the native app builds it with `waitSeconds: 10` and no opener (`apps/portico-react-native/apps/native/src/bridge/useNativePlayback.ts:100`); without an opener the client only long-polls (`client-core/playback-v1/events.ts:134`). So today every browser holds a 25-second long-poll and every native app a 10-second one.

### Background work, notifications, credentials

- **Workers** sleep until a database commit or a timer wakes them; a five-minute safety tick backs up missed wakes (`server/internal/worker/worker.go:26-30`).
- **Scheduler.** `operations.Scheduler` runs registered job kinds (library scan, trash cleanup, retention cleanup, notification maintenance, expired-playback cleanup, receipt cleanup, lyrics fetch, generated-playback cleanup, prepared versions, bulk personal-state jobs) in resource lanes, and owners can attach *schedules*: a daily start minute, a window length, a timezone and a catch-up flag (`server/internal/operations/jobs.go:20-45`, `760-800`; `server/cmd/server/console.go`). The web app shows these as Maintenance windows.
- **Notifications** are an in-app inbox only: categories such as `download.finished`, `dvr.conflict`, `security.new-device`, `scan.failed`, `networking.certificate-expiring`, `storage.low`, `feedback.received` (`server/internal/notify/producers.go`), with dedupe keys, read/archive state, allow-listed actions and 30-day retention.
- **Client log upload** exists: `POST /v1/diagnostics/client-logs`, bounded, listed for admins (`httpapi/admin_diagnostics.go:148-205`).
- **API keys** have a table, a store and scopes (`read-only`, `playback`, `library-management`, `full`), but creating one answers `403 api_keys_disabled` and nothing authenticates with one (`httpapi/admin_access.go:243-249`; `access.AuthenticateKey` has no caller). The Style Guide records this as deliberate (amendment 11).
- **No plugins, no webhooks, no outbound integrations, no compatibility protocol.** DLNA is listed as planned in `Backlog.md`.

## How the others do it

### Jellyfin

- **One generic item API.** About 60 controllers and roughly 400 route attributes (`jellyfin/Jellyfin.Api/Controllers`). The centre is `GET /Items` with about 88 query parameters (`ItemsController.GetItems`): filters, sort, `StartIndex`/`Limit`, `Fields` (a list from ~40 optional field names) to choose how much of each item to return. The same `BaseItemDto` shape describes a movie, a season, a channel, a folder. The client assembles screens from several such queries.
- **Contract from code.** Every controller action carries attributes; Swashbuckle generates one OpenAPI document at run time, served at `/api-docs/openapi.json` with Swagger UI and ReDoc (`jellyfin/Jellyfin.Server/Extensions/ApiServiceCollectionExtensions.cs:197-262`). The official TypeScript, Kotlin and other SDKs are generated from that document, and third-party apps consume those SDKs (Streamyfin depends on `@jellyfin/sdk`, `streamyfin/package.json:43`).
- **No URL versioning.** Paths are unversioned. Compatibility is kept by never removing things: old route spellings stay as extra attributes, obsolete parameters are marked, and a server setting `EnableLegacyAuthorization` gates the oldest header and query spellings (`jellyfin/Jellyfin.Server.Implementations/Security/AuthorizationContext.cs:93-110`).
- **Auth header.** `Authorization: MediaBrowser Client="…", Device="…", DeviceId="…", Version="…", Token="…"`, or `?ApiKey=` in the query (`AuthorizationContext.cs:84-106`). Tokens do not expire until revoked. Admins can mint API keys that act with full admin power (`ApiKeyController`).
- **Authorization** is declared per action with named policies (`RequiresElevation`, `Download`, `LiveTvAccess`, `FirstTimeSetupOrElevated`, …; `ApiServiceCollectionExtensions.cs:70-86`).
- **Errors** are plain text with a status code for exceptions (`jellyfin/Jellyfin.Api/Middleware/ExceptionMiddleware.cs:53-60`, mapping at `84-96`) and ASP.NET problem details for validation. No error codes.
- **No rate limiting, no idempotency, no optimistic concurrency, no JSON ETags** in the core API.
- **CORS** defaults to any origin (`CorsHosts = ["*"]`, `jellyfin/MediaBrowser.Model/Configuration/ServerConfiguration.cs:236`; `Jellyfin.Server/Configuration/CorsPolicyProvider.cs:33-38`).
- **One WebSocket** (`/socket`, token in the query string). Server-to-client message types (`jellyfin/MediaBrowser.Model/Session/SessionMessageType.cs`): `UserDataChanged`, `LibraryChanged`, `Sessions`, `Play`, `Playstate`, `GeneralCommand`, `SyncPlayCommand`, `SyncPlayGroupUpdate`, `RefreshProgress`, `ScheduledTasksInfo`, `ScheduledTaskEnded`, `ActivityLogEntry`, `Package*`, `Timer*`, `ServerRestarting`, `ServerShuttingDown`, `RestartRequired`, `UserUpdated`, `UserDeleted`. Messages **carry data** (the changed user-data records; lists of added, updated and removed item ids). Library changes are batched for `LibraryUpdateDuration` seconds and user-data changes for 500 ms (`LibraryChangedNotifier.cs:195`, `UserDataChangeNotifier.cs:20`). There is no event id, no replay and no resume: a client that reconnects refetches.
- **Keep-alive** is a two-way application message: the server tells the client the timeout (`ForceKeepAlive` with 60 s), the client must send `KeepAlive`, and a socket silent for 60 s is dropped (`SessionWebSocketListener.cs:26-36`, `218-221`).
- **Remote control** is part of the session model: any permitted session can `POST /Sessions/{id}/Playing`, `/Playing/{command}`, `/Command`, `/Message`, and the server relays it over the target's WebSocket.
- **Scheduled tasks** are a user-visible list. Each `IScheduledTask` has a name, category, default triggers (daily, weekly, interval, startup), progress and last result; admins edit triggers and press Run (`ScheduledTasksController`, `jellyfin/Emby.Server.Implementations/ScheduledTasks/Triggers/*`).
- **Plugins** are .NET assemblies loaded into the server process (`PluginManager.cs:140`), able to register services, API controllers, scheduled tasks, metadata providers, auth providers and configuration pages that the web client embeds (`IHasWebPages`). They install from repository manifests with an MD5 checksum (`InstallationManager.cs:586`) and are tracked with statuses (Active, Restart, Disabled, NotSupported, Malfunctioned). DLNA, webhooks, Trakt, Kodi sync, intro detection and many more are plugins.
- **Other small surfaces:** client log upload (`ClientLogController`), branding (login disclaimer, custom CSS, splash screen), per-client display preferences, activity log, Quick Connect, `X-Response-Time-ms` and a slow-response warning log (`ResponseTimeMiddleware.cs:17`, `45-46`), forwarded-header handling with known proxies including `X-Forwarded-Proto` and `X-Forwarded-Host` (`ApiServiceCollectionExtensions.cs:179`), and a configurable base URL path.

### Plezy

Plezy is a client, but it is the best evidence of what a third-party client needs from a server API, because it puts Plex, Jellyfin and Emby behind one interface (`plezy/lib/media/media_server_client.dart:121`):

- Neutral reads: libraries, paged library content with a neutral query (offset + limit), filter categories with values, sort options, a single item, an item with its "on deck" episode, children of a parent, hubs (home rows).
- A **capabilities** record so the UI can hide what a backend cannot do (`plezy/lib/media/server_capabilities.dart`: live TV, DVR, transcoding, rich hubs, numeric rating, favourites, continue-watching removal, subtitle search, metadata edit).
- A **library change channel** per server, coalesced into "this library changed, refetch — and here are the exact removed ids" (`plezy/lib/media/library_change_event.dart:5-38`).
- Its own **response cache** keyed by server and endpoint so the app works offline (`plezy/lib/services/api_cache.dart`).
- Notes in the interface record where Jellyfin's generic API costs extra round trips compared with Plex's composed answers (for example item + on-deck: one Plex call, several Jellyfin calls; `media_server_client.dart:219-237`), and where Jellyfin has no endpoint at all (sort options are hard-coded in the client; `:196-200`).

### Navidrome (Go + SQLite, the closest technical match)

- Its own native API plus **two adopted ecosystem protocols**: Subsonic/OpenSubsonic (`navidrome/server/subsonic`, with declared OpenSubsonic extensions such as `transcodeOffset`, `songLyrics`, `apiKeyAuthentication`; `opensubsonic.go:12-23`) and, new, a **Jellyfin-compatible API subset** (`navidrome/server/jellyfin`, about 10,000 lines, off by default, mounted at `/jellyfin`, with Quick Connect, UDP auto-discovery and a minimal WebSocket that only answers keep-alives; `README.md`, `api.go:75-194`, `socket.go`). Its README states the goal plainly: let existing Jellyfin music clients (Finamp, jftui) use a Navidrome library without a Jellyfin server.
- Its own real-time channel is SSE with a 15-second keep-alive and a write timeout a little longer (`navidrome/server/events/sse.go:26-28`), an in-memory broker and no replay.

## Architectural differences

### A-08.1 Screen documents composed by the server, versus one generic item query

- **Portico:** The server composes a document per screen (bootstrap, home, library content, title workspace, guide directory). Rows declare their own presentation and titles. Clients render (`httpapi/foundation.go:41-48`; Style Guide amendments 5–7).
- **Jellyfin / Plezy / other:** Jellyfin exposes one generic query (`GET /Items`, ~88 parameters, `Fields` selection) and clients compose screens from several calls. Plex sits closer to Portico (hubs, on-deck bundled); Plezy's interface notes the extra Jellyfin round trips (`plezy/lib/media/media_server_client.dart:219-237`).
- **Why it matters:** Portico's first-party clients stay thin and consistent, and a page is a bounded amount of server work. The cost falls on outsiders: a third-party client that wants a *different* screen (a different home, a "random unwatched" tile, a custom grid) has no general-purpose query to build it from and gets Portico's opinion about layout along with the data. Jellyfin's ecosystem (Streamyfin, Wholphin, Finamp, Jellyseerr, Jellystat) exists because the generic query lets each build its own UX.
- **Assessment:** **Trade-off (subjective).** Deliberate (the "Thin clients" principle; Style Guide). I think it is right for Portico's own apps. What would change my view: if a third-party ecosystem is a goal, the structured browse route (`POST /v1/libraries/{id}/browse`) plus item detail would need to be promoted and documented as the stable "data" layer, separate from the "screen" layer. Medium effort (documentation and stability promises more than code).

### A-08.2 Size and granularity of the surface

- **Portico:** about 566 operations for a pre-release product; many are task-specific (`POST /v1/admin/library-channels/{id}/regenerate`, `POST /v1/downloads/receipts/revalidate`, four routes per metadata provider per entity kind).
- **Jellyfin:** about 60 controllers and roughly 400 route attributes after fifteen years (including legacy duplicates).
- **Why it matters:** Every route is a promise once the API is called stable. A wide surface is expensive to keep compatible, to document and to secure. It also makes the first hour harder for an outside developer.
- **Assessment:** **Portico behind** on economy, **ahead** on explicitness. Not written down as a goal either way. Before declaring `/v1` stable it is worth marking which routes are *public contract* and which are *first-party internal* (console panels, onboarding steps, receipts), so only the first group carries a compatibility promise. Small to do as a label; large if routes are actually consolidated.

### A-08.3 Where the contract comes from

- **Portico:** Two sources. 66 routes are generated from the typed registry into OpenAPI 3.1 and TypeScript; about 500 are hand-written YAML checked only for route presence (`httpapi/openapi_coverage_test.go:172-199`). 32 routes appear in both and can disagree. `internal/apispec` validates some real answers against the hand-written schemas.
- **Jellyfin:** One source. 100% of routes are described by attributes on the code that serves them; the document cannot list a route that does not exist or miss one that does. Schema accuracy is as good as the C# types.
- **Why it matters:** Generated clients are only as good as the contract. For 88% of Portico's routes the request and response schemas are prose that someone has to keep true by hand; `client-core` still hand-parses most answers (Backlog: "Client-core: move onto the generated `packages/contracts` types").
- **Assessment:** **Portico behind today, ahead in design.** The registry is stricter than Jellyfin's approach (access rule, limits, error list, cost class are mandatory), but it covers 12% of the surface. Finishing the migration is large (hundreds of handlers) and is the single biggest lever on "API as a product". See also B-08.11.

### A-08.4 Is the contract served by the running server?

- **Portico:** No. The OpenAPI files live in the repository. A running server does not publish its own contract, and there is no interactive explorer.
- **Jellyfin:** Yes: `/api-docs/openapi.json`, Swagger UI and ReDoc on every server, matching that exact build.
- **Why it matters:** Outsiders develop against the server they have. A served contract answers "what does *this* build accept?" and lets SDK generators and tools such as Postman point at a URL.
- **Assessment:** **Portico behind.** Small once there is one document (embed the generated JSON and serve it at, say, `GET /v1/openapi.json`); the blocker is A-08.3.

### A-08.5 Versioning scheme

- **Portico:** A path version (`/v1`) plus an `api: {major, level}` pair in `GET /v1/server` and `/v1/capabilities` (`httpapi/foundation.go:23-32`, `154`). The Style Guide says "all routes are `/v1`". The level is hard-coded to 1 and nothing reads it yet.
- **Jellyfin:** No path version. The server version is the API version; SDKs are released per server version; clients branch on `ServerVersion`.
- **Why it matters:** A path version gives a clean way to ship a breaking redesign beside the old one. A `level` counter gives clients a cheap feature test without parsing version strings — if it is actually incremented.
- **Assessment:** **Portico ahead in design**, deliberate. To make it real: define what bumps `level` (any additive change clients may rely on), and prefer the per-feature strings in `/v1/capabilities` for anything optional. Small.

### A-08.6 Compatibility and deprecation policy

- **Portico:** Pre-release: "routes are reshaped and deleted rather than deprecated" (`server/api/README.md:47-49`). One mechanism exists for the future: `Deprecation: true` plus a `Link … rel="successor-version"` header, used on the legacy playback session routes (`httpapi/playback_v1.go:343-347`). The Style Guide mentions breaking-change detection in CI "active at release".
- **Jellyfin:** Never breaks if it can avoid it; carries legacy route spellings, obsolete parameters and old auth headers for years, now behind `EnableLegacyAuthorization`.
- **Why it matters:** Portico also ships apps through app stores, so old app builds will meet new servers and new apps will meet old servers (owners update servers on their own schedule). That is a compatibility matrix even with zero third parties.
- **Assessment:** **Unclear — a decision is needed before release.** The policy for the day after release is not written: how long a route lives after `Deprecation: true`, the oldest app a server must serve, the oldest server an app must work with. Small to write; it shapes everything else.

### A-08.7 Strict request parsing

- **Portico:** Unknown body fields, duplicate keys, wrong-case keys, and unknown query parameters are `400` (`apikit/registry.go:169-184`, `426-448`). Legacy handlers also use `DisallowUnknownFields` (`router.go:575-577`).
- **Jellyfin:** Permissive model binding: unknown fields and parameters are ignored, names are case-insensitive, lists accept comma or pipe separators through custom binders (`jellyfin/Jellyfin.Api/ModelBinders`).
- **Why it matters:** Strictness catches client bugs early ("a typo is a refusal rather than a silently ignored filter"). It also removes the usual forward-compatibility cushion: a newer client that sends a new optional field to an older server gets a 400 instead of being ignored. With app-store clients and owner-updated servers that case will happen.
- **Assessment:** **Trade-off (subjective).** Deliberate (Style Guide amendment 3: "the server stays strict on requests"). I would keep strictness but pair it with a rule that clients gate every new field on `/v1/capabilities` or `api.level`, and test "new client against previous server" in CI. What would change my view: evidence that clients already do this everywhere.

### A-08.8 No `null` in responses, tolerant readers

- **Portico:** Responses never contain `null`; absent means default; clients must ignore unknown fields and map unknown enum values to a fallback (Style Guide amendments 3–4). The registry enforces no-null at run time by failing the response (`registry.go:417-419`).
- **Jellyfin:** DTOs are full of nullable fields; clients null-check everything. No guidance on unknown enum values (new enum members have broken generated SDK clients in the past, from general knowledge).
- **Why it matters:** Cleaner client code and fewer "undefined is not an object" crashes. The enforcement method matters, though: see B-08.9.
- **Assessment:** **Portico ahead** as a contract rule. The run-time enforcement is a risk worth softening.

### A-08.9 Error model

- **Portico:** Coded errors with stable `code`, safe `message`, and retry guidance. But there are at least five shapes on the wire today:
  1. Registry: `{"error":{code,message,retry:"never|same_request|after_refresh|after_reauth",requestId,…}}` (`apikit/apierror/error.go:37-49`).
  2. Legacy: `{"error":{code,message,retryable:true|false}}` plus sibling fields (`router.go:560`).
  3. Administration: success is wrapped as `{"protocolVersion":"1.0","serverId":…,"result":…}` (`httpapi/administration.go:156`), errors use shape 2 with their own code family (`administration_failed`, `forbidden` where others say `not_permitted`).
  4. Notifications and console: success is `{"scope":{serverId,viewerFence},"data":…}` (`httpapi/notifications_routes.go:186`).
  5. Setup and Quick Connect: `message` is just the code repeated (`httpapi/onboarding.go:120`).
  One route even mixes them: a legacy-shaped body with `"retry":"never"` (`httpapi/audio_media.go`, the `conversion_unavailable` answer).
- **Jellyfin:** No error codes at all: plain-text exception messages with a status, or problem-details JSON for validation.
- **Why it matters:** Portico's coded errors are far better for clients and localisation than Jellyfin's text. The cost of the mixture is that every client needs several parsers; `client-core` has `ApiError`, `ConsoleTransportError`, `BoundedReadError` and the v1 playback error class, and `ApiError` reads only `retryable` (`client-core/index.ts:398-406`), so a registry error with `retry: "same_request"` on a 409 or 429 is treated as not retryable.
- **Assessment:** **Portico ahead of Jellyfin, behind its own Style Guide.** The single envelope is the documented target. The migration rides on A-08.3. In the meantime a one-page table of "which family uses which envelope" in the API guide would save outsiders real time (the guide currently says "every JSON error is the same envelope", which is not true of registry routes).

### A-08.10 Paging: signed, revision-fenced cursors versus offsets

- **Portico:** Opaque HMAC-signed cursors bound to viewer, query and the catalogue/viewer revision; `409 stale_continuation` when the revision moves; 30-minute expiry; random access via `start` + `positionIndex` on big grids (`server/internal/catalog/discovery.go:106-162`).
- **Jellyfin:** `StartIndex` + `Limit` + optional `TotalRecordCount`. No snapshot: a walk during a scan can skip or repeat items, but it always finishes.
- **Why it matters:** Portico never shows a page stitched from two states, and paging cost does not grow with depth. The price: any change to the library or to the viewer's own state restarts the walk. For a person scrolling that is a re-read of the visible window; for a tool exporting a whole library it can mean never finishing while a scan is running (see B-08.14).
- **Assessment:** **Portico ahead** for first-party UX and scale; **Trade-off** for integrators. Deliberate ("Correctness over limits", "Scale"). A snapshot-tolerant export route (keyset order by id, no revision fence, documented as "may miss concurrent changes") would serve sync tools. Small to medium.

### A-08.11 Optimistic concurrency

- **Portico:** Every change of an existing thing names the revision it was planned against; missing is `428`, mismatch is `409`/`412`, often with the current state attached so no second read is needed.
- **Jellyfin:** Last write wins.
- **Why it matters:** Two devices editing the same playlist, queue or setting cannot silently overwrite each other.
- **Assessment:** **Portico ahead.** Note the two spellings (`expectedRevision` in the body with `409 *_conflict`; `If-Match` header with `412 revision_mismatch`) — one more thing an outside developer has to learn per family.

### A-08.12 Idempotency

- **Portico:** Writes take a client key and replay the recorded answer. Five spellings exist (`Idempotency-Key` header, `operationId`, `requestId`, `idempotencyKey`, `registrationRequestId`), each with its own table, window (seconds to 30 days) and conflict code. The shared toolkit store (`apikit/idempotency`) is not used by any route (B-08.11).
- **Jellyfin:** None. Clients must tolerate double-applied writes (most are naturally idempotent "set" operations).
- **Why it matters:** On flaky mobile links a retried "add to playlist" or "start recording" is applied once.
- **Assessment:** **Portico ahead**, with the same "many dialects" caveat.

### A-08.13 Conditional requests and cache headers

- **Portico:** Default `no-store`; three different ETag schemes (body hash on registry GETs; process-wide write counter + token hash + five-minute bucket on grids; exact revisions on home and detail) all with `private, no-cache` so nothing is ever served stale (`httpapi/cache_headers.go:10-25`, `grid_etag.go:14-43`). Artwork revalidates on every use instead of using a max-age.
- **Jellyfin:** No ETags on JSON. Images are cacheable for long periods and keyed by a `tag` query value that changes when the image changes, so clients fetch a poster once and never ask again.
- **Why it matters:** "Fresh" is honoured strictly. The price is a request per poster per render (a cheap 304, but still a round trip, authentication and a lane slot each), where Jellyfin and Plex pay zero requests for an unchanged poster. The Style Guide actually specifies the Jellyfin-style answer ("URLs are versioned and immutable", amendment 5); the code comment in `cache_headers.go` argues for revalidation instead.
- **Assessment:** **Trade-off / doc-code disagreement.** Versioned immutable artwork URLs (`…?v=<revision>` with `Cache-Control: private, max-age=31536000, immutable`) are both fresh *and* request-free, because an edit changes the URL. `httpapi/router.go:1074` already passes a `v` parameter into the artwork lookup. This looks like the best of both; medium effort (every card must carry the versioned URL, and the server must refuse a stale `v`). The grid ETag has its own limitation (B-08.8).

### A-08.14 Auth header design

- **Portico:** Standard `Authorization: Bearer`. Device identity travels in separate `X-Portico-*` headers and is bound into the session server-side.
- **Jellyfin:** A bespoke scheme: `Authorization: MediaBrowser Client="…", Device="…", DeviceId="…", Version="…", Token="…"`, with the token also accepted as `?ApiKey=`.
- **Why it matters:** Standard bearer works with every HTTP tool, proxy and API gateway without custom parsing. Jellyfin's query-string token leaks into access logs and browser history (it is needed there because WebSocket and `<img>`/`<video>` cannot set headers). Portico avoids that by using fetch-based SSE (header auth) and capability URLs for media.
- **Assessment:** **Portico ahead.**

### A-08.15 Credential lifetime and third-party credentials

- **Portico:** 15-minute access tokens with rotating, device-bound refresh tokens for everyone; API keys built but disabled; OAuth 2.1 and personal access tokens named as the future third-party path (Style Guide amendment 11).
- **Jellyfin:** Device tokens that last until revoked; admin API keys with full power; Quick Connect.
- **Why it matters:** Portico's model is safer by default. It also means there is **no credential a script, a Home Assistant integration, a stats dashboard or a request manager can hold today** without implementing the full rotating-refresh flow with a stable installation id.
- **Assessment:** **Trade-off now, gap at release.** Deliberate and recorded. See F-08.1.

### A-08.16 How authorization is expressed

- **Portico:** In handlers and wrappers. Owner/admin routes authorise before the work, re-check inside the write transaction, and re-check again after composing the answer (`httpapi/administration.go:128-158`). Registry routes also carry a mandatory `Access` label, but the shared authoriser treats `device`, `viewer_library` and `viewer_item_restrictions` the same except for one path parameter name (B-08.13).
- **Jellyfin:** Declarative policies on controller actions; no re-check during or after the work.
- **Why it matters:** Portico's revocation is immediate even mid-request and mid-stream; Jellyfin's is checked once at the door. Jellyfin's is easier to audit by reading attributes.
- **Assessment:** **Portico ahead** on strength. For auditability, the registry label should be the thing that is enforced (so the generated contract's `x-access` is a fact, not a comment).

### A-08.17 Media URL authority

- **Portico:** Capability URLs with a grant in the path, re-authorised every 100 ms during the transfer (`router.go:1355-1369`), bounded write stalls (`httpapi/write_deadline.go`).
- **Jellyfin:** The session token or API key on the stream URL, checked when the request starts.
- **Why it matters:** A revoked viewer stops mid-file in Portico. A capability URL can be handed to a player (AVPlayer, Chromecast) without giving it the account token.
- **Assessment:** **Portico ahead.** (Detail belongs to the playback audit.)

### A-08.18 CORS posture

- **Portico:** Allow-list: same origin, the two Hosted origins, plus an environment variable. Everything else with an `Origin` header is refused before routing (`router.go:1248-1259`).
- **Jellyfin:** Any origin by default; an optional host list.
- **Why it matters:** Portico is closed to browser-based third-party tools unless the owner edits the environment. It also breaks its own web app behind a TLS-terminating reverse proxy (B-08.1). The Style Guide says the opposite ("bearer routes allow any origin without credentials", amendment 12).
- **Assessment:** **Doc/code disagreement; I think the Style Guide is right.** With bearer tokens and no cookies, allowing any origin on bearer routes adds no meaningful risk (a hostile page cannot read the token), and keeping setup/sign-in routes first-party-only preserves the protection that matters. Small change in one function.

### A-08.19 Admission control and back-pressure

- **Portico:** Twelve lanes with caps, queues, per-client fair shares and per-request budgets; `503 server_busy` + `Retry-After`; explicit `429` codes per subsystem; body caps per route; a per-request database statement counter recorded per route pattern (`admission.go:489-494`); HTTP/2 stream and connection ceilings sized from host memory (`server/cmd/server/transport_policy.go`).
- **Jellyfin:** None of this. The web server's defaults apply; a heavy request simply competes.
- **Why it matters:** This is what lets a weak box serve 200 browsers predictably and lets background work yield to people. It is also a lot of machinery whose numbers (24 browsing slots, 512 realtime, 5 s budgets) have not yet met a physical-device acceptance test (brief: performance targets not established).
- **Assessment:** **Portico ahead**, by a wide margin. Keep, advertise, and publish the limits (F-08.14).

### A-08.20 Sign-in throttling

- **Portico:** 20 attempts a minute per address and 1,280 per network (/24 or /64), with eviction that never refuses honest clients (`httpapi/rate.go:38-43`), plus identity-level `SetupLimit` budgets and per-account cool-off.
- **Jellyfin:** A per-user failed-attempt counter that locks the account (`LoginAttemptsBeforeLockout`); no per-address limit.
- **Why it matters:** Portico resists password spraying without letting an attacker lock a user out. The limiter's client-address logic has a gap behind reverse proxies (B-08.7).
- **Assessment:** **Portico ahead.**

### A-08.21 Real-time transport: SSE and long-poll versus one WebSocket

- **Portico:** One-way channels over plain HTTP: SSE or long-poll, several of them (unified feed, notifications, group, receiver inbox, log tail).
- **Jellyfin:** One two-way WebSocket for everything; Navidrome: one SSE stream.
- **Why it matters:** SSE and long-poll pass through every proxy, need no upgrade handshake, authenticate with the normal header and reconnect with a standard cursor. A WebSocket gives lower latency in both directions and one connection for everything, but needs proxy configuration (the most common Jellyfin reverse-proxy support question) and a token in the URL.
- **Assessment:** **Portico ahead** for a self-hosted product. Deliberate (Style Guide amendment 8). The remaining untidiness is the number of channels: the Style Guide says "one `/v1/events` endpoint", the code has five long-lived routes, and a receiver device keeps two open (`apps/portico-react-native/apps/native/src/app/receiver.tsx:57` plus the unified feed).

### A-08.22 Events as invalidations versus events as data

- **Portico:** An event says which resource changed and its new revision; the client refetches (`server/api/guide/06-events.md:5-6`). A few carry a small `data` object (scan state and count, job progress, session state).
- **Jellyfin:** Messages carry the data: full user-data records, id lists of added/updated/removed items, the whole session list.
- **Why it matters:** Invalidation keeps one source of truth and one permission check (the refetch), and makes event loss harmless after a resync. Data-carrying events save the round trip and let a UI update in place, at the cost of a second code path that must apply the same visibility rules.
- **Assessment:** **Portico ahead** on correctness; deliberate. Fine as long as the refetches are cheap (they are conditional and bounded).

### A-08.23 Durable, resumable event log versus ephemeral push

- **Portico:** Events are rows in SQLite (ring of 20,000, `apievents/events.go:12`), ids are monotonic (`AUTOINCREMENT`), a reconnect resumes exactly, and a cursor older than the ring gets `stream.resync`.
- **Jellyfin / Navidrome:** In-memory only; on reconnect the client refetches everything it shows.
- **Why it matters:** Portico clients can sleep, roam networks and miss nothing, and a long-poll client gets the same guarantees as a streaming one. The cost is a database write per event and a read per reader per wake (A-08.25).
- **Assessment:** **Portico ahead.** Two edge cases in the current implementation are listed as B-08.4 and B-08.5.

### A-08.24 What is actually pushed

| Change | Portico (`/v1/events` unless noted) | Jellyfin WebSocket |
|---|---|---|
| Watched/progress/favourite changed on another device | **Not pushed** | `UserDataChanged` with the records (500 ms batch) |
| Items added/updated/removed in a library | **Not pushed** (only scan started/progress/finished) | `LibraryChanged` with id lists (batched, default 30 s) |
| Scan progress | `library.scan.updated` (≤ 1 per 10 s per library, to everyone who can see the library) | `RefreshProgress` (admins) |
| Background task progress | `operation.updated`, `job.updated` | `ScheduledTasksInfo` (periodic while subscribed), `ScheduledTaskEnded` |
| Sessions / now playing | `session.updated` (own device and profile), `admin.sessions` ("refetch") | `Sessions` (full list, periodic while subscribed) |
| Play queue changed | `queue.updated` | — (client-side queues) |
| Remote control commands | Receiver inbox + `receiver.inbox_changed` on the notifications stream | `Play`, `Playstate`, `GeneralCommand` |
| Watch together | Group SSE stream | `SyncPlayCommand`, `SyncPlayGroupUpdate` |
| Notifications | `notification.changed` | — (no inbox; activity log entries) |
| Server restarting / shutting down | **Not pushed** | `ServerRestarting`, `ServerShuttingDown`, `RestartRequired` |
| User deleted / policy changed | Not pushed as an event (the next request or heartbeat fails authority) | `UserDeleted`, `UserUpdated` |
| Plugin install progress | n/a | `Package*` |
| Live TV timers | Not pushed (DVR detail has its own `after` poll) | `TimerCreated`/`Cancelled` |

- **Why it matters:** The two rows in bold are the ones people notice: finishing an episode on the TV does not update Continue Watching on the phone until the phone refetches, and a newly added film does not appear in an open grid until a scan event or navigation. Plezy treats exactly these two signals as the essential ones (`plezy/lib/media/library_change_event.dart`).
- **Assessment:** **Portico behind on coverage** (ahead on mechanism). Adding `personal.changed` (profile audience, with the viewer revision) and `library.changed` (library audience, with the catalogue revision, coalesced like scan events) fits the existing design exactly and is small to medium. The Style Guide's own topic list (`device.command`, `playback.transfer_offered`, `group.timeline`, `server.notice`) is also not built. See F-08.6.

### A-08.25 Fan-out mechanism

- **Portico:** One shared wake nudges every parked reader, at most once a second. Each reader then does a live authentication read (two for owners) and an event query, whether or not the event was for it (`eventfeed/feed.go:88-97`; `httpapi/events_routes.go:106-128`, `232-244`).
- **Jellyfin:** The server holds each session's socket in memory and writes only to the sessions concerned.
- **Why it matters:** At 200 connected browsers one event costs Jellyfin a handful of socket writes and costs Portico several hundred small SQLite read transactions. That is probably fine on a modern box and is bounded to once a second, but it is work proportional to *connected clients*, not to *interested clients*, on hardware that "spans weak boxes". See B-08.6.
- **Assessment:** **Portico behind on cost**, ahead on correctness (no in-memory state to lose, revocation re-proved on every delivery). A small in-memory index (audience → waiting readers, filled from the committed rows) would keep the durable log and wake only the readers concerned. Medium.

### A-08.26 Keep-alive and stream lifetime

- **Portico:** Server-driven. Comment heartbeat every 25 s on `/v1/events` (20 s notifications, 15 s groups); each frame write bounded to 20 s; streams end themselves after 30 minutes and the client reconnects with its cursor; `X-Accel-Buffering: no`; SSE is never compressed (`events_routes.go:17-18`, `166-167`; `httpapi/compression.go:34-38`).
- **Jellyfin:** Client-driven application keep-alive with a 60 s lost-socket timeout. Navidrome: 15 s keep-alive.
- **Why it matters:** 25 s sits under the common 60 s idle timeouts of nginx, HAProxy and cloud load balancers but leaves little margin for 30 s defaults (some corporate proxies, some mobile carriers' NATs). A re-check of authority on every heartbeat means a revoked session loses its stream within one beat.
- **Assessment:** **Portico ahead.** Small tidy-ups: use one heartbeat interval across the three streams (15 s is the safest), and state it in the guide (`06-events.md` currently says heartbeat interval is "not specified").

### A-08.27 One stream per installation

- **Portico:** A second `/v1/events` stream for the same account and installation cancels the first and is itself refused with `409 event_stream_exists` (`eventfeed/feed.go:113-123`).
- **Jellyfin:** Any number of sockets per session.
- **Why it matters:** It bounds server resources per device. It also makes two browser tabs, or a reconnect that arrives before the server has noticed the old connection died, a refusal (B-08.3).
- **Assessment:** **Trade-off.** "Newest wins, old one is closed" (admit the new stream, cancel the old) gives the same bound without refusing the client that is actually alive.

### A-08.28 Subscriptions and topics

- **Portico:** No subscription message: the server decides what a viewer hears from who they are (profile, device, owner, visible libraries). A `topics` query parameter is reserved and rejected (`events_routes.go:57-60`).
- **Jellyfin:** Clients opt in to the expensive periodic feeds with start/stop messages (`SessionsStart "0,1500"`, `ScheduledTasksInfoStart`, `ActivityLogEntryStart`).
- **Why it matters:** Implicit topics are simple and safe. Opt-in would let a TV app skip admin and scan chatter it never shows.
- **Assessment:** **Portico ahead** for simplicity. Revisit only if event volume grows.

### A-08.29 Remote control of another device

- **Portico:** A formal model: a device registers as a *receiver*, controllers obtain *grants*, and a *handoff* moves playback with a two-phase commit (readiness, commit, rollback); the receiver learns of commands by long-polling its inbox or by a hint on the notifications stream (`httpapi/social_routes.go:597-836`). Admins can end a session with a message (`POST /v1/admin/sessions/{id}:terminate`).
- **Jellyfin:** Any session that reports `SupportsRemoteControl` can be sent `Play`, `Playstate` (pause, seek, next) and ~40 `GeneralCommand`s (volume, subtitles, navigation, `DisplayMessage`) by any user allowed to control it; delivery is best-effort over the WebSocket.
- **Why it matters:** Jellyfin's is the "phone as remote for the TV" feature people expect, and the reason third-party remotes and Home Assistant integrations exist. Portico's handoff is more robust for *moving* playback, and a receiver accepts a five-word command vocabulary — `load`, `play`, `pause`, `seek`, `stop` (`server/internal/social/receivers.go:30`). What Jellyfin has beyond that: next/previous, volume and mute, audio and subtitle track selection, navigation keys, and "show this message on that screen" for ordinary users. I did not read the receiver code past the vocabulary.
- **Assessment:** **Trade-off.** Portico's is safer (explicit grants, durable inbox, two-phase move) and narrower. Widening the vocabulary is small; see F-08.7.

### A-08.30 Background work that people can see

- **Portico:** Wake-on-change workers that need no schedule, plus a job scheduler with owner-set daily maintenance windows for a fixed set of job kinds; progress through `operation.updated` and console job lists.
- **Jellyfin:** Every recurring job is a named scheduled task with editable triggers (daily at, weekly on, every N hours, at startup), a Run button, live progress, and last-run result; plugins add their own.
- **Why it matters:** Portico's server is idle when idle and catches up by itself, which suits weak hardware. Jellyfin's list answers the owner's question "what is the server doing, when did it last do it, and can I make it do it now?" in one place and gives plugins a home.
- **Assessment:** **Trade-off (subjective).** Portico's engine is the better design; Jellyfin's presentation is the better owner experience. Portico's `GET /v1/admin/console/job-kinds`, `jobs` and `schedules` already hold most of the data; a "Tasks" view with last run, next run and Run now is mostly UI. Interval and weekly triggers are the missing schedule shapes (F-08.12).

### A-08.31 Extensibility by code

- **Portico:** None, and by decision no external code contributions. Everything is in the binary.
- **Jellyfin:** In-process plugins with full trust (they run as the server, can read every file the server can, add unauthenticated routes, and have caused crashes and security advisories); the catalogue covers metadata sources, subtitles, auth (LDAP, SSO), notifications/webhooks, DLNA, intro detection, Trakt/AniList sync, reports, themes.
- **Why it matters:** Plugins are how Jellyfin covers the long tail without the core team building it, and also its largest source of instability ("Malfunctioned" is a first-class plugin status; `jellyfin/MediaBrowser.Model/Plugins/PluginStatus.cs`) and upgrade breakage (`targetAbi` gating).
- **Assessment:** **Trade-off; I think Portico's choice is right for its principles** (one binary, open data, low ceremony, security). The long tail still has to go somewhere: the safe substitute is *out-of-process* extension — webhooks out, a stable API and scoped tokens in (F-08.1, F-08.2). What would change my view: a need for third-party metadata providers or auth providers, which cannot be done over webhooks.

### A-08.32 Notifications

- **Portico:** A durable per-profile and per-owner inbox with severity, categories, dedupe keys, allow-listed actions, read/archive, retention, broadcast by the owner; delivered in-app only (decision: no push, no server email).
- **Jellyfin:** Removed its built-in notification system; owners use the Webhook plugin (Discord, ntfy, Gotify, Slack, generic) and the activity log.
- **Why it matters:** Portico's inbox is a real product feature Jellyfin lacks. An owner who wants "tell my phone when the disk is full" while not looking at Portico has no path.
- **Assessment:** **Portico ahead in-app, behind off-app.** A generic outbound webhook per notification category would respect "no push, no email from the server" while letting owners wire their own (F-08.2).

### A-08.33 Client log upload

- **Portico:** `POST /v1/diagnostics/client-logs` for any signed-in device, stored and listed for admins, included in a bounded support bundle.
- **Jellyfin:** `POST /ClientLog/Document` (1 MB, can be disabled), written to the server log folder.
- **Assessment:** **Portico ahead** (structured, bounded, surfaced in the console).

### A-08.34 Response compression

- **Portico:** gzip only, decided by lane, minimum 1 KiB, never for media, bulk or streams (`httpapi/compression.go`).
- **Jellyfin:** ASP.NET response compression (gzip and Brotli).
- **Assessment:** **Roughly equal.** Brotli or zstd would shrink the 300 KB home document further; trivial priority.

### A-08.35 Reverse proxies and forwarded headers

- **Portico:** Forwarded headers are trusted only from configured proxy networks. Scheme from `X-Forwarded-Proto` is honoured for the "require HTTPS" policy (`httpapi/secure_connections.go:76-84`) but not for the CORS same-origin test; there are two different `X-Forwarded-For` parsers; `X-Forwarded-Host` is not used; no base-path support (the server must sit at `/`).
- **Jellyfin:** Known-proxies list feeding the framework's forwarded-headers handling (`For`, `Proto`, `Host`), and a configurable base URL so it can live at `/jellyfin`.
- **Why it matters:** Reverse proxy with TLS termination is the most common way self-hosters publish a media server. Portico's first-class path is port forwarding with its own certificates, which avoids the problem for people who follow it, but owners with an existing proxy will try that first.
- **Assessment:** **Portico behind** for this deployment style; see B-08.1 and B-08.7. Deciding whether reverse proxies are *supported* or *tolerated* is an open question.

### A-08.36 Health and readiness

- **Portico:** `/health/live`, `/health/ready` (database ping, 1 s), and `/v1/readiness` (no database work; reports `starting | ready | degraded | corrupt`) (`httpapi/installation.go:19-32`, `admin_concurrency.go:226-246`).
- **Jellyfin:** `/health`.
- **Assessment:** **Portico ahead.** Three endpoints for one question is one or two too many for outsiders.

### A-08.37 Capability negotiation

- **Portico:** `/v1/capabilities` (feature states per viewer, numeric limits), `/v1/auth/capabilities` (sign-in methods), `/v1/bootstrap`.
- **Jellyfin:** `/System/Info` and version sniffing. Plezy had to build its own capabilities record per backend.
- **Assessment:** **Portico ahead.** Make it trustworthy: one advertised limit is already wrong (B-08.5), and `limits.ts` in the generated contract is two lines long.

### A-08.38 Server-supplied strings

- **Portico:** Errors localised by `code` on the client; row titles as `{code, params, fallback}`.
- **Jellyfin:** The server localises strings itself (`LocalizationController`, per-user language).
- **Assessment:** **Portico ahead** for multi-user servers with mixed languages (the server never has to know the viewer's language).

### A-08.39 Observability of the API itself

- **Portico:** Per-route database statement counts, lane occupancy and refusal reasons, rate-limited 5xx logging with a request id in the body (`apierror/error.go:129-140`; `router.go:197-233`), owner diagnostics panels.
- **Jellyfin:** `X-Response-Time-ms` on every response and a slow-request warning in the log; optional Prometheus metrics.
- **Assessment:** **Portico ahead** internally; **behind** on standard outward signals (no response-time header, no Prometheus endpoint; F-08.15). Legacy handlers do not return a request id.

### A-08.40 Route matching inside the registry

- **Portico:** The registry's own dispatcher loops over all its routes and splits both strings for each (`apikit/registry.go:226-247`), after `http.ServeMux` has already matched once.
- **Jellyfin:** Framework routing.
- **Why it matters:** Negligible at 66 routes; worth replacing before the registry holds 500.
- **Assessment:** **Trivial**, noted for completeness.

### A-08.41 Identifier and unit conventions

- **Portico:** Opaque string ids restricted to `[A-Za-z0-9_-]`; RFC 3339 timestamps on catalogue shapes but epoch milliseconds on notifications and jobs; durations as float seconds on legacy shapes and integer milliseconds on v1 playback (the guide README admits the integer-ms rule is "a target, not the current wire format").
- **Jellyfin:** GUIDs; .NET ticks (100 ns) for durations; ISO timestamps.
- **Assessment:** **Portico ahead** on ids, **mixed** on units. One rule for time and duration before `/v1` is frozen would spare every client a table of exceptions.

### A-08.42 Ecosystem protocols

- **Portico:** None. Only Portico apps can talk to a Portico server.
- **Others:** Jellyfin speaks its own API (which Infuse, Kodi add-ons, VLC, Plezy, Streamyfin, Finamp and dozens more implement) and DLNA via plugin. Navidrome speaks Subsonic/OpenSubsonic and now a Jellyfin subset. Audiobookshelf and Kavita-style servers speak OPDS for e-readers.
- **Why it matters:** On day one a Portico owner can use only the platforms Portico ships for. An owner with a Sonos, an old Android head unit, a Kodi box or a preference for Infuse has no route in.
- **Assessment:** **Portico behind on reach**; a strategic choice rather than an oversight. See F-08.8.

### A-08.43 What a multi-backend client needs from a server (lessons from Plezy)

- **Portico:** Provides, in one form or another: libraries, paged content, filters with values, sort options, item detail with next-up, children, home rows, capabilities. Does not provide: a coarse "library changed" push, offset paging, or a way to reuse a cached answer offline (answers are `no-store` or revalidate-only).
- **Plezy:** Its neutral interface (`plezy/lib/media/media_server_client.dart:121-243`) is a fair checklist of the minimum a general-purpose client asks of any server, and it caches raw answers by endpoint for offline use (`plezy/lib/services/api_cache.dart`).
- **Why it matters:** If someone wrote a Portico backend for Plezy tomorrow, the reads would map well (Portico is closer to Plex's composed style than Jellyfin's), `/v1/capabilities` would map directly, paging would need an adapter from cursors to offsets (possible only by walking), the change channel would have nothing to listen to outside scans, and the offline cache would be fighting `no-store`.
- **Assessment:** **Portico mostly ahead**, with the two concrete gaps already listed (F-08.6, A-08.13). Useful as an outside-in test of the API: "could a competent stranger build a client from the published contract?" Today the answer depends on the stale guide (see Doc/code disagreements).

### A-08.44 Security headers on API responses

- **Portico:** CSP, frame denial, `nosniff`, referrer policy and HSTS on every response, API included; a narrower CSP for the Cast receiver page (`httpapi/router.go:1240-1246`, `social_routes.go:957-964`).
- **Jellyfin:** None by default on API responses; owners add them at the reverse proxy.
- **Assessment:** **Portico ahead.** One side effect to keep in mind: the fixed `connect-src` list in the CSP names Portico's own hosts, so an owner-hosted copy of the web app on another origin needs both `PORTICO_ALLOWED_ORIGINS` and a CSP that the server does not let them change.

### A-08.45 Unknown routes and methods

- **Portico:** An unmatched `/v1/…` request gets the JSON error envelope, and `405` with an `Allow` header when the path exists for other methods (`httpapi/router.go:1307-1347`). Unknown query parameters are a 400 on registry routes and on many legacy ones.
- **Jellyfin:** Framework defaults (empty 404/405).
- **Assessment:** **Portico ahead** (small, but it is the kind of thing API consumers notice).

### A-08.46 Slow or stalled stream consumers

- **Portico:** Every SSE frame write has a 20-second deadline; media writes have a rolling 60-second stall deadline; HTTP/2 has a 15-second write-progress timeout (`httpapi/write_deadline.go:33-43`; `server/cmd/server/transport_policy.go:21`). A stalled reader is disconnected and resumes from its cursor.
- **Navidrome:** A one-message buffer per client and a write timeout of keep-alive + 5 s (`navidrome/server/events/sse.go:26-29`); a slow client loses messages.
- **Jellyfin:** Sends are awaited per socket; a stuck socket is dropped by the keep-alive timeout.
- **Assessment:** **Portico ahead**: nothing is lost because the log is durable.

### A-08.47 Web app delivery

- **Portico:** The server serves the built web app from a directory through a confined root (no symlink escape), with SPA fallback only for HTML requests, `no-cache` for the shell and `immutable` for the entry bundle (`httpapi/installation.go:38-88`). A Hosted copy at `web.getportico.tv` can drive any server cross-origin.
- **Jellyfin:** Serves jellyfin-web from the server; plugin configuration pages are injected into it; third-party web clients (jellyfin-vue) can be hosted anywhere because CORS is open.
- **Assessment:** **Roughly equal**; Portico's Hosted web app is a nice extra that Jellyfin has no equivalent of. See B-08.12 for the chunk-caching nit.

## Missing features and capabilities to consider

### F-08.1 A credential for things that are not Portico apps

- **What:** Long-lived, scoped, revocable tokens a person or tool can paste into a script, Home Assistant, a stats dashboard, a request manager or a third-party player. Portico has the table, scopes and admin UI routes for API keys but creation is switched off and nothing authenticates with one (`httpapi/admin_access.go:243-249`).
- **Who has it:** Jellyfin (admin API keys, full power, never expire; device tokens that never expire). Navidrome (Subsonic token/salt, and OpenSubsonic `apiKeyAuthentication`). Audiobookshelf (API tokens). Plex (the `X-Plex-Token`).
- **User value:** Every integration starts here. Without it the only way in is a username and password plus Portico's rotating-refresh protocol.
- **Size:** Medium. The scope predicate exists (`server/internal/access/apikeys.go:32-38`); the work is enforcing scope per route — which the registry's `Access`/`Scope` metadata was designed for, but only 12% of routes are on it.
- **Conflicts:** None with the principles. The Style Guide prefers member-approvable OAuth 2.1 and personal access tokens over server-wide keys.
- **Opinion:** Ship **personal access tokens** first (a token acts as one profile, with a scope ceiling, shown once, revocable from the account page). That covers scripts and dashboards without the OAuth machinery, and a read-only scope can be enforced coarsely (GET-only plus an allow-list) before the full registry migration.

### F-08.2 Outbound webhooks

- **What:** The owner registers a URL; the server POSTs a small JSON document when chosen things happen (playback started/stopped, item added, scan finished/failed, new device signed in, storage low, recording finished).
- **Who has it:** Jellyfin (Webhook plugin, the most-installed plugin after the metadata ones; templates for Discord, ntfy, Gotify, Slack, generic). Plex (webhooks, Plex Pass). Audiobookshelf and Navidrome expose event streams instead.
- **User value:** This one feature unlocks "notify my phone" (ntfy, Gotify, Pushover), Discord announcements, Home Assistant automations ("dim the lights when a film starts") and scrobbling, all without Portico building or hosting any of it.
- **Size:** Small to medium. Portico already has the durable event log and the notification producers; a webhook sender is one more wake-on-change worker with retry/backoff and a per-hook failure counter. Needs SSRF care (owner-only configuration; the existing `remotemedia` network policy code is a model).
- **Conflicts:** Consistent with "no push notifications or server email" — the server sends nothing unless the owner points it somewhere — and with "Hosted stays cheap".
- **Opinion:** High value for the size. It is the safe replacement for most of what Jellyfin's plugin system is used for.

### F-08.3 One complete, generated, served contract (and SDKs)

- **What:** A single OpenAPI document covering every public route, generated from code, served by the running server, with an interactive explorer, and official generated clients (TypeScript first; Kotlin and Swift follow for free).
- **Who has it:** Jellyfin (`/api-docs`, `@jellyfin/sdk`, `jellyfin-sdk-kotlin`, community Swift/Python/Dart SDKs).
- **User value:** Outside developers can start in minutes; first-party clients lose their hand-written parsers; contract drift becomes a build failure rather than a runtime surprise.
- **Size:** Large (it is the registry migration). A cheaper interim step: merge the fifteen YAML files and the generated JSON into one published document and serve it.
- **Opinion:** This is the core of "API as a product". Portico's registry design is better than Jellyfin's attribute approach; it just needs to cover the surface.

### F-08.4 Scrobbling and watch-state sync to outside services

- **What:** Send plays to Trakt, Last.fm, ListenBrainz, Simkl, AniList/MyAnimeList; optionally import history and ratings back.
- **Who has it:** Jellyfin (Trakt, AniList, Simkl, ListenBrainz, Last.fm plugins). Navidrome (Last.fm and ListenBrainz built in, per user). Plex (Trakt via webhooks; Last.fm built in for music).
- **User value:** People keep years of history in Trakt and Last.fm and treat a server that cannot scrobble as a regression.
- **Size:** Medium per service if built in; near zero for Portico if F-08.2 exists and the community bridges it (webhook → Trakt bridges already exist for Plex and Jellyfin).
- **Conflicts:** Outbound calls with a user's third-party token; per-user opt-in. Does not touch Hosted.
- **Opinion:** Decide between "built-in for the two or three that matter" (ListenBrainz and Last.fm are simple; Trakt has rate limits and an approval process) and "webhooks plus a documented payload". I would do webhooks first.

### F-08.5 Import of watch state and ratings from Plex and Jellyfin

- **What:** A one-time import so a household moving to Portico keeps watched flags, resume points, ratings and playlists. Either direct (read the other server's API with a token the owner provides) or from an export file.
- **Who has it:** The Jellyfin community (Plex → Jellyfin migration scripts; watch-state sync tools such as JellyPlex-Watched), Trakt as a neutral intermediary.
- **User value:** The biggest practical barrier to switching servers.
- **Size:** Medium. Matching titles by provider ids (TMDB/TVDB/IMDb) is the hard part and Portico stores those.
- **Opinion:** Probably the highest-leverage adoption feature in this whole list for a brand-new server. A public bulk personal-state route already exists (`POST /v1/jobs`), so the importer could even be an external tool if F-08.1 lands.

### F-08.6 Push events for personal state and catalogue changes

- **What:** `personal.changed` (this profile's watched/progress/favourites moved; refetch Continue Watching and visible cards) and `library.changed` (titles added/removed/updated in a library you can see), as invalidation events on `/v1/events`. Possibly `server.notice` (restarting/shutting down) so clients show "server restarting" instead of an error.
- **Who has it:** Jellyfin (`UserDataChanged`, `LibraryChanged`, `ServerRestarting`). Plex (timeline and activity notifications). Plezy consumes both and treats them as the minimum useful set.
- **User value:** Home and grids stay current across devices without navigating away and back.
- **Size:** Small to medium: append an event in the personal-state write transaction and in catalogue publication (coalesced like `scanevents`, which is a good template).
- **Conflicts:** None; it is the "Fresh" principle applied to open screens. Mind the fan-out cost (B-08.6) since personal state changes often.

### F-08.7 Simple remote control and "send a message to that screen"

- **What:** Portico receivers already take `load`, `play`, `pause`, `seek`, `stop`. The gap is the rest of a remote: next/previous, volume and mute, audio/subtitle selection, and showing a short message on another screen.
- **Who has it:** Jellyfin (Sessions API; used by its web dashboard, mobile apps and Home Assistant). Plex (companion protocol).
- **User value:** "Use my phone as the remote", parental "bedtime" message, owner "restarting in 5 minutes".
- **Size:** Small if built on the existing receiver inbox; the event name `device.command` is already reserved in the Style Guide.
- **Opinion:** Worth doing for parity with what people call "phone as a remote". Admin terminate-with-message exists already.

### F-08.8 A compatibility protocol so existing apps work

Recorded as feature *options*, as asked. They are independent.

| Option | What works afterwards | Precedent | Rough size | My view |
|---|---|---|---|---|
| **Jellyfin-API subset** | Infuse, Kodi (Jellyfin add-on), VLC, Plezy, Streamyfin, Wholphin, Finamp, Swiftfin — depending on how much is implemented | Navidrome built a music-only subset in ~10,000 lines of Go (`navidrome/server/jellyfin`), off by default, with Quick Connect and UDP discovery, routes matched case-insensitively | Large for video (items query with dozens of parameters, `PlaybackInfo` with device profiles, HLS URL shapes, progress reporting, images, the WebSocket keep-alive) | The highest reach per unit of work of any option, and the most strategic: it makes every platform Portico has not shipped an app for usable on day one. It also imports Jellyfin's permissive model (long-lived tokens, generic queries) beside Portico's strict one, so it must be off by default and clearly second-class. |
| **Subsonic / OpenSubsonic** (music only) | Symfonium, DSub, Amperfy, play:Sub, Feishin, Sonos via bonob, car head units | Navidrome, Gonic, Ampache; a small, stable, well-documented protocol | Medium | Good fit if music is a first-class Portico library type; the music-client ecosystem is large and loyal. |
| **DLNA / UPnP** | Smart TVs, AV receivers, consoles | Jellyfin plugin, Plex | Medium; already on the Backlog as planned, off by default | Agree with the Backlog note that it exposes the whole server to the LAN. |
| **OPDS** (books/audiobooks) | E-reader apps (KOReader, Moon+, Panels) | Kavita, Komga, Audiobookshelf (partial) | Small | Only if ebooks become a library type. |
| **Plex-API compatibility** | Plex clients | Nobody credible; clients are tied to plex.tv sign-in | Very large, legally awkward | Not worth it. |

- **Conflicts:** "No external code contributions" is unaffected. "Thin clients / server composes" does not apply to foreign clients. Security: each protocol is a second front door with weaker credentials than Portico's own; keep each behind an owner switch reported by `/v1/capabilities`.
- **What would change my view on Jellyfin-API:** if Portico's own apps will cover Android TV, Fire TV, Roku, Vega and the web well at launch, the remaining gap is mostly Infuse, Kodi and smart-TV-native apps, and the case weakens.

### F-08.9 Targeted "this path changed" scan endpoint for download managers

- **What:** `POST` a list of changed file paths (or a folder) so the server scans just those, now.
- **Who has it:** Jellyfin (`POST /Library/Media/Updated`), Plex (`/library/sections/{id}/refresh?path=`), used by Sonarr/Radarr/Lidarr "Connect" and by Autoscan.
- **User value:** A new download appears within seconds without a full library scan or filesystem watching on network mounts (where change notifications often do not work).
- **Size:** Small to medium. `POST /v1/libraries/{id}/scans?mode=incremental|full|integrity` exists today and is library-wide (`httpapi/router.go:995-1007`); the ingestion call takes a library and a source, not a path (`server/internal/ingestion/incremental.go:18`).
- **Opinion:** High value for the arr-using audience; needs F-08.1 to be callable.

### F-08.10 Request-manager and statistics ecosystem hooks

- **What:** The read surfaces that Jellyseerr/Overseerr (what is already in the library, by TMDB/TVDB id; who the users are), Jellystat/Tautulli (play history export) and similar tools need.
- **Who has it:** Jellyfin and Plex by virtue of generic queries plus tokens.
- **Size:** Portico already has the data (`GET /v1/admin/play-history`, provider ids). What is missing is a documented lookup "do you have TMDB id X?" and a credential. Small after F-08.1.

### F-08.11 Public share links

- **What:** A link that plays one title (or a playlist) for someone with no account, with an expiry.
- **Who has it:** Navidrome (`server/public`, shares), Audiobookshelf (share links), Plex (Watch Together invites require accounts; no anonymous shares).
- **Opinion:** Sharing domain rather than mine; recorded because `client-core/public-links.ts` is only terms/privacy links and the Link Contract has no share row.

### F-08.12 Richer schedule shapes and a visible task list

- **What:** Interval ("every 6 hours"), weekly ("Sundays at 03:00") and at-startup triggers; a Tasks page with last run, duration, result, next run, Run now.
- **Who has it:** Jellyfin (`DailyTrigger`, `WeeklyTrigger`, `IntervalTrigger`, `StartupTrigger`).
- **Size:** Small (the schedule row is `start_minute` + `window_minutes` + timezone today, `server/internal/operations/jobs.go:768`).
- **Opinion:** Mostly a presentation gap; Portico's wake-on-change design needs schedules far less than Jellyfin does.

### F-08.13 Owner branding of the sign-in screen

- **What:** A login message/disclaimer, a server picture or splash, optionally custom CSS.
- **Who has it:** Jellyfin (`BrandingController`: login disclaimer, custom CSS, generated splash screen).
- **Opinion:** A login message is cheap and popular with owners who share with family. Custom CSS conflicts with Portico's design consistency and CSP; I would skip it.

### F-08.14 Published limits and rate-limit headers

- **What:** A complete limits table in the contract (page sizes, body sizes, write rates, stream counts, long-poll maximum, cursor lifetime) and standard `RateLimit`/`RateLimit-Policy` headers on limited routes.
- **Who has it:** Neither Jellyfin nor Plex; this is where Portico could simply be better than the field.
- **State today:** The CORS expose list already names `RateLimit`, `RateLimit-Policy` and `Idempotent-Replayed` (`httpapi/router.go:1264`) but no handler sends them; the generated `limits.ts` is two lines; the guide says limit tables are "not specified".
- **Size:** Small.

### F-08.15 Standard metrics and timing signals

- **What:** A Prometheus-format metrics endpoint (owner-only or loopback-only) and a `Server-Timing` or response-time header.
- **Who has it:** Jellyfin (optional `/metrics`, `X-Response-Time-ms`), Navidrome (Prometheus).
- **Opinion:** Portico already measures more than either (lane and per-route statement costs); exposing it in the format monitoring tools read is small and serves the homelab audience.

### F-08.16 A stable "data" layer for third-party UIs

- **What:** Declare a small generic set as the supported way to build a different UI: structured browse with field selection, multi-get by ids, children-of, search, personal state. Today these exist (for example `POST /v1/libraries/{id}/browse`) but are presented beside screen documents with no statement of which is stable.
- **Who has it:** Jellyfin (`/Items` with `Fields`, `Ids=`), Plex.
- **Size:** Mostly documentation and a promise; possibly a `fields` selector.

### F-08.17 Hosting under a path prefix

- **What:** Serve Portico at `https://example.com/portico/` behind an existing reverse proxy.
- **Who has it:** Jellyfin (`BaseUrl`), Navidrome (`BaseURL`), Audiobookshelf.
- **Size:** Medium (every absolute `/v1/…` and asset path, CSP, cookies if any).
- **Opinion:** Only if reverse proxies become a supported path (A-08.35).

### F-08.18 Activity log of who did what

- **What:** A chronological owner-visible log: sign-ins and failures, playback start/stop, settings changes, library changes, who deleted what.
- **Who has it:** Jellyfin (`ActivityLog`, pushed live to the dashboard).
- **State today:** Portico has the full play history, the message log and security notifications; I did not find one merged "activity" list and did not read `operations` audit code closely. Recorded as a question.

## Possible bugs and risks in Portico

All of these are findings from reading source. None was reproduced.

### B-08.1 Behind a TLS-terminating reverse proxy, the bundled web app's writes are refused as "origin not allowed"

- **What goes wrong:** Browsers send an `Origin` header on every same-origin `POST`, `PUT`, `PATCH` and `DELETE`. Portico accepts an origin only if it equals `scheme://Host` of the request, where the scheme is `https` only when the Go server itself terminated TLS (`httpapi/onboarding.go:91-97`). Behind nginx, Caddy or Traefik terminating TLS, the browser sends `Origin: https://media.example.com` and the server computes `http://media.example.com` (or `http://<backend address>` if the proxy rewrites `Host`). The check fails and the request is answered `403 origin_not_allowed` before routing (`httpapi/router.go:1248-1259`). Page loads (GET) work; sign-in and every other write do not.
- **Why nothing rescues it:** The trusted-proxy `X-Forwarded-Proto` logic exists but is used only by the require-HTTPS policy (`httpapi/secure_connections.go:76-84`). The owner's own `accessUrls` setting ("extra URLs clients may use, such as a reverse proxy address", `server/api/admin-access.md:186`) is not consulted. The only fix is the `PORTICO_ALLOWED_ORIGINS` environment variable (`server/cmd/server/main.go:551-556`), which is documented nowhere in `INSTALLING.md` or the API docs (no match for "reverse proxy" or the variable name).
- **How the comparison surfaced it:** Jellyfin's CORS default is any origin, and it honours `X-Forwarded-Proto`/`Host` from known proxies; reverse proxy is its most common deployment.
- **Confidence:** Confirmed by reading (static). **Severity:** High for anyone using a TLS-terminating proxy (they cannot sign in on the web); nil for the port-forwarding path Portico recommends. **Backlog:** not listed.
- **Also:** the code contradicts Style Guide amendment 12 (A-08.18).

### B-08.2 Any error a legacy handler does not recognise becomes `400 invalid_request`, unlogged

- **What goes wrong:** `failure()` starts from `status, code := 400, "invalid_request"` and only overrides it for errors it knows (`httpapi/router.go:247`). An unexpected internal error that is not a path error or a SQLite error (a wrapped `fmt.Errorf`, a decode failure in a service, a nil-dependency error such as `errors.New("artwork unavailable")` at `router.go:1071`) is therefore reported to the client as the client's own mistake, with `retryable: false`, and is not logged because only statuses ≥ 500 are (`router.go:556-559`).
- **Consequence:** Server faults in ~500 legacy routes can be invisible in the log and unactionable in the client ("Check the request and try again"). The administration family and the registry both default the other way (500, logged; `httpapi/administration.go:31-33`, `apikit/apierror/error.go:119-140`).
- **Confidence:** Confirmed by reading. **Severity:** Medium (diagnosability; wrong retry behaviour). **Backlog:** not listed.

### B-08.3 A reconnect that arrives before the server notices the old stream is dead demotes the client to long-polling for good

- **What goes wrong:** `/v1/events` allows one SSE stream per account+installation. A second one cancels the first **and is itself refused** with `409 event_stream_exists` (`server/internal/eventfeed/feed.go:113-123`). The server's error text says "reconnect using your last event ID", but the shared client treats a 409 as "this server refuses streaming" and sets `streamFailures = Infinity`, which switches to long-poll until the client object is restarted (`client-core/playback-v1/events.ts:134`, `147`, `152`). A phone that changes network reconnects within a second or two; the server learns the old socket is dead only at the next heartbeat write (up to 25 s + 20 s). The same happens with two browser tabs.
- **Why it is latent today:** no first-party client passes a stream opener, so they never stream (see "How Portico does it today"). It will bite the first client that does, and any third-party client following the docs.
- **Confidence:** Confirmed by reading both sides. **Severity:** Low today, Medium once SSE is used. **Backlog:** not listed.
- **Related observation:** the SSE branch of `/v1/events` (exclusivity, `stream.closed`, heartbeats) is exercised only by tests.

### B-08.4 An SSE client that has not yet received an event has no cursor, so a drop loses whatever happened in the gap, silently

- **What goes wrong:** Opening the stream without `Last-Event-ID` returns nothing and never tells the client the current position (`eventfeed/feed.go:149-152`; `httpapi/events_routes.go:181-185` sends only `retry:`). The client learns a cursor only from the `id:` of the first real event (`client-core/playback-v1/events.ts:102`). On a quiet server that can be many minutes. If the connection drops in that time — including the planned 30-minute `stream.closed` — the client reconnects with no cursor and starts from "now"; events committed in between are skipped with no `stream.resync`.
- **Not affected:** long-poll, because every answer carries `nextAfter` (`events.ts:203`). The web scan tracker also re-reads on every reconnect (`apps/portico-server/web/src/app/library-scan.ts:53-57`), which masks it for that one consumer.
- **Fix shape:** send an opening frame with `id: <current max>` (the group stream already does this with `group.snapshot`, `httpapi/social_routes.go:458-466`).
- **Confidence:** Confirmed by reading. **Severity:** Low today (SSE unused), Medium for streaming clients. **Backlog:** not listed.

### B-08.5 The advertised long-poll maximum is 55 seconds; the server accepts 25

- **Evidence:** `/v1/capabilities` publishes `longPollSecondsMax: 55` (`httpapi/foundation.go:155`); the Style Guide says "≤ 55 s"; the client clamps to 55 (`client-core/playback-v1/events.ts:196`); the handler rejects anything above 25 with `400 invalid_request` (`httpapi/events_routes.go:77-81`).
- **Consequence:** A client that trusts the advertised limit fails every poll. First-party clients use 25 and 10, so they are unaffected.
- **Confidence:** Confirmed by reading. **Severity:** Low. **Backlog:** not listed.

### B-08.6 Every event wakes every connected client into several database reads (design risk at 200 browsers)

- **What happens:** The hub's wake reaches all parked readers (`eventfeed/feed.go:88-97`). Each long-poll then runs a strict (uncached) authentication — token, recovery route and library authority in one snapshot, and a second resolution for owners (`httpapi/events_routes.go:37-53`, `114-123`) — then an event read that opens another snapshot and runs three queries (`feed.go:139-175`), and one more authentication before answering (`events_routes.go:131`). Readers the event was not for go back to sleep after all that.
- **Scale:** With 200 browsers that is on the order of 600–800 short read transactions per wake, at most one wake per second (`worker/worker.go:104`), against a small read pool (a code comment mentions eight connections, `router.go:596`). Wakes come from any commit touching `api_events`, `notification_inbox` or `console_operations` — playback state changes, queue edits, notification writes, console operations, scan progress.
- **Baseline cost without any events:** every browser re-issues its long-poll every 25 s and every native app every 10 s (`useNativePlayback.ts:100`), each with the same authentication reads; a five-minute safety tick also wakes everyone.
- **How the comparison surfaced it:** Jellyfin writes to the sockets of the sessions concerned from memory; Navidrome filters per client in memory.
- **Confidence:** Confirmed by reading that the work happens; **whether it matters is speculative — needs a runtime check** on weak hardware. **Severity:** Medium as a design risk. **Backlog:** not listed.

### B-08.7 Behind a reverse proxy, sign-in and device-code limits are shared by everyone

- **What goes wrong:** Three different pieces of code decide "who is the client":
  1. Locality (LAN vs remote) walks `X-Forwarded-For` from the right past trusted proxies (`httpapi/request_locality.go:118-141`) — correct.
  2. The sign-in rate limiter and lane fairness accept a forwarded address only if the header is a **single** address (`httpapi/rate.go:231-238`). Any list — two proxies in a chain, or a client that sent its own `X-Forwarded-For` which nginx then appended to — falls back to the proxy's address, so all users share one 20-per-minute bucket.
  3. The identity budgets use the socket peer with no proxy handling at all (`httpapi/onboarding.go:98-104`): `login` 20, `setup` 10, `quick-create` 10, `invitation` 10 per peer (`router.go:825`, `845`; `onboarding.go:236`; `foundation.go:313`). Behind any reverse proxy the peer is the proxy for everyone.
- **Consequence:** A household of TVs requesting Quick Connect codes, or many people signing in after a restart, can exhaust a budget that was meant to be per device; conversely one abusive client can lock out sign-in for all.
- **Confidence:** Confirmed by reading; the budget windows inside `identity.SetupLimit` were not read. **Severity:** Medium for proxy deployments, nil otherwise. **Backlog:** not listed.

### B-08.8 Grid ETags are invalidated by any write anywhere and by token renewal

- **Evidence:** The validator for grids and rows hashes the process-wide count of committed write transactions, the access-token hash, and a five-minute time bucket (`httpapi/grid_etag.go:41-43`; counter defined in `server/internal/dbwork/write.go:25-37`). A playback progress report from anyone, an event row, a lease refresh, or the 15-minute token rotation changes every tag.
- **Consequence:** On a server where anyone is playing, grid revalidation practically always answers 200 with a full recomposition; the 304 path only helps an idle server. The code comment says so ("conservative"), but `02 Server.md` describes conditional reads as revision-based. Home and detail do use exact revisions (`home_routes.go:133`, `detail.go:92`).
- **Also:** no client sends `If-None-Match` for JSON explicitly (only subtitles do, `client-core/subtitles.ts:112`), so the benefit depends on each platform's HTTP cache; React Native on Android has none by default.
- **Confidence:** Confirmed by reading. **Severity:** Low (lost efficiency, never staleness). **Backlog:** not listed.

### B-08.9 Registry safety checks fail whole responses at run time

- **What goes wrong:** After a handler succeeds, the registry rejects the response with `500 internal` if any reachable slice, map or interface is nil, or if the JSON is over 512 KiB (`apikit/registry.go:412-425`, `580-674`). A single un-initialised list in a rarely hit branch (an item with no tracks, an empty failure list) turns a working endpoint into a 500 for exactly those inputs. For a write, the mutation has already committed when the response is refused, so the client sees a failure for a change that happened.
- **How the comparison surfaced it:** Jellyfin serialises whatever it has (nulls included).
- **Confidence:** Confirmed by reading the mechanism; no failing handler was identified. **Severity:** Medium as a latent trap. **Backlog:** not listed.
- **Softer alternative:** enforce in tests and in development builds; in release, log loudly and send `[]`/`{}`.

### B-08.10 The mismatch between error envelopes changes client retry behaviour

- **Evidence:** `ApiError` computes `retryable` from `error.retryable`, defaulting to `status >= 500` (`client-core/index.ts:398-406`). Registry errors do not send `retryable`; they send `retry` (`apikit/apierror/error.go:40`). So `409 request_in_progress`, `429 rate_limited` and `409 cursor_expired` from registry routes read as not retryable through the general request path. The v1 playback client has its own parser, which limits the damage to registry routes called through `request()` (devices, bulk jobs, download requests, backups, bootstrap).
- **Also:** `/v1/events` is a registry route whose handler writes legacy-shaped errors (`httpapi/events_routes.go:20-24`), so the generated contract's error schema does not describe what that route returns.
- **Confidence:** Confirmed by reading; which screens are affected was not traced. **Severity:** Low to Medium. **Backlog:** related item "Client-core: move onto the generated contracts".

### B-08.11 Toolkit pieces the documents describe are not what the server runs

- **Evidence:**
  - `apikit/cursor` (signer with a key file and rotation) has no caller in the server; catalogue cursors are signed with a key stored in the `configuration` table, loaded once and never rotated (`server/internal/catalog/discovery.go:93-105`).
  - `apikit/idempotency.Store.Execute` has no caller; only its `Sweep` runs, against an `api_idempotency` table nothing writes (`server/cmd/server/maintenance.go:82`, `182-184`).
  - `apikit/precondition` has no caller in `server/internal`.
  - `access.AuthenticateKey` has no caller, while `server/api/admin-access.md:113-117` describes API keys as authenticating.
- **Consequence:** No runtime fault. The risk is that reviewers and outside readers reason about guarantees (key rotation, uniform replay semantics) that the running code does not provide, and that expired catalogue cursors answer `400 invalid_cursor` rather than the toolkit's `409 cursor_expired`, so a client cannot tell "expired, restart" from "malformed".
- **Confidence:** Confirmed by search (no call sites outside tests). **Severity:** Low. **Backlog:** not listed.

### B-08.12 `If-None-Match`, ETag and encoding nits

Small HTTP-correctness items, recorded briefly:

- The registry's ETag is strong and identical for gzip and identity encodings (`apikit/registry.go:392-398`; compression applied later in `admission.go:500-507`). Proxies that care will weaken it; Portico's own matcher ignores the weak prefix, so it works.
- `acceptsGzip` ignores `q=0` (`httpapi/compression.go:198-206`).
- The 304 is produced after the full response has been built and hashed, so it saves bytes but not work.
- Only `assets/index-*` gets `immutable` caching; other hashed chunks under `assets/` are revalidated on every load (`httpapi/installation.go:83-86`).
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-08.13 The registry's access label is not what is enforced

- **Evidence:** `v1Authorize` handles `Owner`, `Admin` and "everything else" (`httpapi/playback_v1.go:221-248`). `Device`, `ViewerLibrary` and `ViewerItem` all resolve the principal the same way; the only extra check is for `ViewerItem`, and only when the path has a parameter literally named `itemId` (`:257-262`). An `Admin` route's `Scope` string is not checked beyond the special case `direct_account` (any other scope means "admin tier"). In the generated contract 43 of 66 routes are `device`, one is `viewer_item_restrictions`, none is `viewer_library`.
- **Why it matters:** The Style Guide says the mandatory access rule "is how content restrictions … become impossible to forget". As built, item and library restrictions are still enforced by each handler (I saw them doing so), and the label is documentation. A future route that takes an item id in `{id}` or in the body and is labelled `viewer_item_restrictions` would get no check from the registry.
- **Confidence:** Confirmed by reading. I did **not** find an unprotected route: every handler I sampled from the ~350 registrations authenticates, through a wrapper or directly, and the intentionally public ones are listed under "Outside my domain". **Severity:** Medium as a latent trap. **Backlog:** not listed.

### B-08.14 A full-library walk may not finish while the library is changing (needs a runtime check)

- **Evidence:** A cursor is valid only while the library's catalogue revision and the viewer's revision are unchanged; otherwise `409 stale_continuation` and the client restarts from the first page (`server/internal/catalog/discovery.go:159-161`).
- **Risk:** A tool exporting a multi-million-item library in pages of 100 needs tens of thousands of requests with no revision change in between. During a scan or metadata refresh that may never happen. Whether the viewer revision moves on the viewer's own playback progress I did not check; if it does, a person's own TV can restart their phone's paging.
- **Confidence:** Speculative — depends on how often revisions move. **Severity:** Medium for integrators, Low for first-party screens (they re-read a window). **Backlog:** not listed.

### B-08.15 Original files are always labelled `video/mp4`

- **Evidence:** `serveOriginal` sets `Content-Type: video/mp4` for anything with a video codec, whatever the container (`httpapi/media_original.go:47-53`), and every response carries `X-Content-Type-Options: nosniff` (`router.go:1240`). The stored container is used only for the download filename.
- **Consequence:** A direct-played Matroska, WebM, MPEG-TS or AVI file is declared as MP4. Players that probe bytes (mpv, ExoPlayer, VLC) cope; players and browsers that trust the declared type, and any third-party client, may refuse it. Jellyfin maps container to MIME type.
- **Confidence:** Confirmed by reading; impact on real players not tested. **Severity:** Low to Medium. Belongs with the playback audit; recorded here because it is an HTTP-contract issue.

### B-08.16 Unauthenticated build fingerprint

- **Evidence:** `GET /v1/system` returns `version`, `buildId`, `sourceDigest` and `builtAt` to anyone (`httpapi/router.go:816-818`).
- **Risk:** Lets an internet scanner identify exactly which build a port-forwarded server runs. Jellyfin exposes its version publicly too, and it has been used to target unpatched servers.
- **Confidence:** Confirmed by reading. **Severity:** Low. A sign-in page needs the name and `setupRequired`, not the source digest.

### B-08.17 The realtime lane is one pool for every long-lived connection

- **Evidence:** 512 slots, no queue (`httpapi/admission.go:79`); used by the unified feed, notification streams and waits, group streams, receiver waits, the log tail and claim approval waits (`httpapi/route_lanes.go:24-38`, `153`, `246-252`). A receiver device holds two (feed long-poll plus notifications SSE).
- **Risk:** 200 browsers with two tabs each, plus native devices, approaches the ceiling; the 513th is refused with `503 server_busy`. Per-client fair shares soften abuse but not honest volume.
- **Confidence:** Confirmed limits; reaching them is speculative. **Severity:** Low. The notification-stream comment in `notifications_routes.go:38-48` records that this ceiling has already been raised once from 64.

### B-08.18 Slow uploads other than avatars and artwork are cut at 15 seconds

- **Evidence:** The HTTP server sets `ReadTimeout: 15s` (`server/cmd/server/main.go:959`). Only avatar and artwork uploads extend the read deadline (`httpapi/identity_routes.go:49-52`, `metadata_editor.go:124-127`). Large JSON bodies elsewhere — Live TV playlist uploads up to twice `MaxUploadBytes` (32 MiB; `httpapi/live_channels.go:239`, `server/internal/livechannels/model.go:20`), client log uploads — must arrive within 15 s.
- **Confidence:** Probable (Go applies `ReadTimeout` to the whole body); body sizes and real upload speeds not checked. **Severity:** Low (remote owners on slow uplinks).

## Where Portico is stronger

Worth keeping, and worth saying out loud when the API is presented to outsiders:

1. **A durable, resumable event log.** Reconnect and miss nothing; long-poll and streaming clients get identical guarantees; a too-old cursor is told to resync. Jellyfin, Plex and Navidrome have nothing like it (A-08.23).
2. **Admission lanes with honest refusals.** Bounded concurrency, fair shares per client, per-request budgets, `Retry-After` on every refusal, background work that yields to people (A-08.19). No comparison project has an equivalent.
3. **Revocation that actually revokes.** Authority is re-proved inside write transactions, after composing answers, on every stream heartbeat and every 100 ms of a media transfer (A-08.16, A-08.17).
4. **Optimistic concurrency and idempotency on every write** (A-08.11, A-08.12).
5. **Coded errors with retry guidance and a request id** (registry routes), and a lint that keeps implementation jargon out of public messages (`apikit/apierror/error.go:61-73`).
6. **Standard bearer auth and capability URLs** instead of a bespoke header and tokens in query strings (A-08.14).
7. **Mandatory route metadata.** A registry route cannot be registered without an access rule, lane, cost class, body limit and error list; the lane table is exhaustive by test; the route list cannot drift from the documents by test.
8. **Strictness that catches client bugs**: unknown fields, unknown query keys, duplicate JSON keys and wrong-case keys are refused (A-08.7).
9. **Capabilities and bootstrap** documents, so clients ask what the server offers instead of guessing from a version string (A-08.37).
10. **Idle means idle.** Wake-on-change workers with a five-minute safety tick; scan events bounded to one per ten seconds per library (`server/internal/scanevents/scanevents.go:18-19`).
11. **A real notification inbox** with dedupe, actions and retention (A-08.32), and structured client log upload (A-08.33).
12. **Sign-in throttling that cannot be turned into a lockout of other people** (`httpapi/rate.go:14-37`).
13. **Security headers on everything** and enveloped 404/405 answers (A-08.44, A-08.45).
14. **No in-process third-party code.** Jellyfin's plugin model is its biggest source of crashes, upgrade breakage and security advisories (A-08.31).
15. **Stalled consumers are bounded on every kind of response** (A-08.46).

## Doc/code disagreements

| # | The document says | The code does |
|---|---|---|
| D1 | `server/api/README.md` (Versioning, "The controller and lane model", recipe 3) and guide sections 01, 02 and 04 describe a `/v2` playback control plane (controllers, lanes, occurrences, `/v2/queues`) and files `playback-v2.openapi.json`, `playback-v2-protocol.md`. `02 Server.md` says "(and `/v2` for a few newer surfaces)". | There is no `/v2` route anywhere in the server, and those files do not exist in `server/api/`. `openapi.yaml`'s own header says "There is no `/v2`". Queues and sessions are `/v1/queues` and `/v1/playback/sessions`. The README's main playback recipe cannot work. |
| D2 | Style Guide amendment 1: "all routes are `/v1`" and the registry enforces it (`apikit/registry.go:291`). | Consistent with code; inconsistent with D1's documents. Listed so the stale ones are fixed, not the code. |
| D3 | Style Guide amendment 12: bearer routes allow any origin. | Allow-list only; other origins get 403 (`httpapi/router.go:1248-1259`). |
| D4 | Style Guide amendment 8: one `/v1/events` endpoint, long-poll ≤ 55 s, topics including `device.command`, `playback.transfer_offered`, `group.timeline`, `server.notice`. | Five long-lived routes; long-poll ≤ 25 s; none of those four topics exists; `topics` is rejected (`httpapi/events_routes.go:57-60`, `77-81`). |
| D5 | `/v1/capabilities` advertises `longPollSecondsMax: 55`. | 25 (B-08.5). |
| D6 | Style Guide: "Access rule is mandatory per route… the registry refuses to start if a route lacks one… This is how content restrictions become impossible to forget." | True for 66 routes; ~500 routes are not on the registry; and the label is not what is enforced (B-08.13). The six rules in the guide (`public | device | viewer(library) | viewer(item, restrictions) | owner | admin(scope)`) collapse to three behaviours in `v1Authorize`. |
| D7 | `02 Server.md`: "Lists page with opaque, signed cursors"; apikit provides the cursor signer with rotation. | Catalogue cursors are signed, but by `catalog` with a never-rotated key in the database; `apikit/cursor` is unused (B-08.11). |
| D8 | `02 Server.md`: "Idempotency. Mutations accept an idempotency key; a replay returns the recorded receipt" (one mechanism implied); apikit provides the store. | Five per-family mechanisms; `apikit/idempotency.Store.Execute` is unused; its table is swept but never written (B-08.11). |
| D9 | `02 Server.md`: "Revisions… clients use them for conditional reads (`ETag`/`If-None-Match`)". Guide 01 says "There is no `If-Match` / `If-None-Match` / `ETag`-on-JSON contract". | Both partly wrong: registry GETs, grids, home and detail do send ETags and honour `If-None-Match` (A-08.13); registry writes do use `If-Match`. Grid tags are not revision-based (B-08.8). |
| D10 | Style Guide amendment 5: artwork "URLs are versioned and immutable". | Artwork is `private, no-cache` and revalidated on every use (`httpapi/cache_headers.go:10-25`). |
| D11 | `server/api/admin-access.md:113-117`: an API key "authenticates like a bearer token… resolved in `httpapi.bearerPrincipal`". | Key creation answers `403 api_keys_disabled`; `access.AuthenticateKey` has no caller. (The Style Guide does say creation is disabled.) |
| D12 | `server/api/README.md` and `openapi.yaml`: "Every JSON error is the same envelope `{code, message, retryable}`". | Registry routes use `{code, message, retry, requestId, …}`; several families wrap successes differently (A-08.9). |
| D13 | Guide 06: registered event types are `notification.changed`, `operation.updated`, `library.scan.updated`, `stream.resync`. | Also `job.updated`, `session.updated`, `queue.updated`, `admin.sessions`, and `stream.closed` (search of `apievents.Append` callers; `httpapi/events_routes.go:219`). |
| D14 | `server/api/README.md`: sign-in limited to "20 attempts per minute per peer address". | Also a per-network ceiling and separate identity budgets; and "peer" is the proxy behind a reverse proxy (B-08.7). |
| D15 | `openapi.yaml` `license: Proprietary`. | The project decision recorded in the brief is GPL-3.0-or-later. |
| D16 | `server/api/guide/README.md`: every citation "can be checked mechanically" with `check-examples.mjs`. | The guide cites operationIds that no longer exist (D1), so either the check is not run or it does not check citations. I did not run it (read-only audit). |
| D17 | CORS `Access-Control-Expose-Headers` lists `Idempotent-Replayed`, `RateLimit`, `RateLimit-Policy` (`router.go:1264`). | No handler sets any of them. |
| D18 | `server/api/README.md`: "Media URLs… `/v2/media/linear/…`". | Linear media is `/v1/media/linear/{playbackId}/{generation}/{token}/{name}` (`httpapi/linear_media.go:83`). |

## Open questions for Justin

1. **Is a third-party ecosystem a goal for 1.0, a goal for later, or a non-goal?** Almost every "behind" in this document follows from that one answer (credentials, served contract, webhooks, stable data layer, compatibility protocols).
2. **If it is a goal: personal access tokens first, or wait for OAuth 2.1?** (F-08.1)
3. **Do you want a Jellyfin-API compatibility layer as an option** (off by default), so Infuse, Kodi, VLC, Plezy, Streamyfin and others work with a Portico server? Navidrome has just done the music-only version of exactly this. And separately: Subsonic for music, OPDS for books? (F-08.8)
4. **Are reverse proxies with TLS termination a supported way to run Portico?** If yes, B-08.1 and B-08.7 are release blockers and a base path may be wanted; if no, the installation guide should say so plainly and name `PORTICO_ALLOWED_ORIGINS`.
5. **Should bearer routes accept any browser origin**, as the Style Guide says, or stay on an allow-list, as the code does? (A-08.18)
6. **Outbound webhooks: yes or no?** They fit "no push, no email from the server" as I read it, but that is your call. (F-08.2)
7. **What is the compatibility promise the day after release?** How long does a deprecated route live; what is the oldest app build a server must serve and the oldest server an app must tolerate? (A-08.6)
8. **Which routes are public contract and which are first-party internal?** (A-08.2)
9. **Strict unknown-field rejection versus rolling upgrades:** keep strict and rely on capability gating, or let the server ignore unknown *optional* fields from newer clients? (A-08.7)
10. **Artwork caching:** revalidate every time (current code) or versioned immutable URLs (current Style Guide)? (A-08.13)
11. **Should watch-state and library changes be pushed to open screens?** (F-08.6) It is the visible difference from Jellyfin and Plex on a second device.
12. **Should clients stream (`SSE`) or keep long-polling `/v1/events`?** The server has both; the apps only use long-poll, at 25 s (web) and 10 s (native). If long-poll is the intended steady state, the SSE-only rules (one stream per installation, 30-minute life) can go; if SSE is intended, B-08.3 and B-08.4 need fixing first.
13. **Is the 10-second native long-poll acceptable as permanent idle chatter** (six requests a minute per open app, each with database reads), given the "idle server does no polling work" goal? It exists because the native transport ends requests at 15 s.
14. **Run-time response guards (no-null, 512 KiB): fail the request, or log and repair?** (B-08.9)
15. **Is Jellyfin-style "phone as a full remote" wanted**, beyond load/play/pause/seek/stop? (F-08.7)
16. **Watch-state import from Plex and Jellyfin: built in, or left to an external tool once tokens exist?** (F-08.5)
17. **Should `GET /v1/system` keep returning build identifiers to unauthenticated callers?** (B-08.16)
18. **Do you want a single activity log** in the owner console, or are play history + message log + notifications enough? (F-08.18)
19. **One time/duration convention before `/v1` freezes** (RFC 3339 vs epoch ms; seconds vs ms)? (A-08.41)

## Evidence boundary

- **Nothing was executed.** Every statement about behaviour is an inference from source. In particular: the CORS failure behind a proxy (B-08.1), the long-poll demotion (B-08.3), the cursor gap (B-08.4) and the fan-out cost (B-08.6) were traced in code but not observed.
- **Performance** numbers (hundreds of read transactions per wake, lane sizing, whether 512 realtime slots is enough) are arithmetic from constants, not measurements. No device, network or proxy was involved.
- **Route audit depth.** I enumerated all ~350 route registrations in `httpapi` and `networking` by script and inspected every one my script could not classify (about 150), family by family, until each resolved to an authentication wrapper or an intentionally public route. I did not read every handler body in full, and I did not audit what each wrapper's authority function checks inside `identity`, `access` or `operations` — that belongs to the identity audit.
- **Hand-written schemas.** I compared route *lists* between the YAML files and the code (they match), not request/response schemas.
- **Not read:** the Hosted API; `social` beyond the stream handler and command vocabulary; `apispec` coverage; `packages/contracts` tests; `identity.SetupLimit` windows; most of `operations` (console, panels, audit); the DVR `after` polling contract; whether the viewer revision moves on playback progress (B-08.14).
- **Jellyfin** claims about SDK generation, plugin capabilities, the webhook plugin's popularity, proxy-related support load, and the session remote-control routes rest partly on prior knowledge rather than on files read today; the file citations given are the ones I opened. **Plex, Emby, Infuse, Kodi** statements are from general knowledge only.
- **Third-party client behaviour** (which players trust `Content-Type`, which apps a Jellyfin-compatible subset would satisfy) is not tested.

## Outside my domain

Short notes on things noticed in passing; not chased.

- **Intentionally unauthenticated routes** (for the identity auditor to confirm each is meant): `/health/live`, `/health/ready`, `/v1/readiness`, `/v1/system`, `/v1/server`, `/v1/attribution`, `/v1/auth/capabilities`, `/v1/auth/remembered-accounts`, `/v1/setup*`, `/v1/sessions` (POST), `/v1/direct/sign-in`, `/v1/direct/portico-challenge`, `/v1/auth/refresh`, `/v1/auth/register`, `/v1/auth/two-factor/challenge`, `/v1/quick-connect*`, `/v1/access/invitations/preview` and `/accept`, `/v1/hosted/wake`, `/v1/cast/redeem` and `/reconnect`, the capability URLs (`/v1/media/{grant}…`, `/v1/media/linear/…`, `/v1/downloads/artifacts/{grant}`, `/v1/topshelf/art/{id}?grant=`), `/receiver/cast/`, the static web app, and the networking claim routes.
- **`GET /v1/auth/remembered-accounts?installationId=…`** returns usernames and display names for an installation id to an unauthenticated caller, and the id travels in the query string, where proxies log it (`httpapi/identity_routes.go:170-181`). Whether an installation id is secret enough for that is an identity question.
- **Catalogue cursor key in the database** (`configuration.catalog_cursor_key`): it travels with backups and state copies. Forged cursors seem low-risk (scope is re-checked), but it is a secret in plain SQLite by design ("open data").
- **`closeUnreadUpload` sets the read deadline to "now"** on refused HTTP/1 uploads (`httpapi/identity_routes.go:124-130`), which is the mechanism that later makes requests arrive with a cancelled context and is then patched at the edge by `detachCanceledConnection` (`httpapi/connection_context.go:24-31`). It works; it is a self-inflicted wound with a bandage, and each occurrence forces a connection close.
- **Sign-outs after a demo restart** (Backlog, "Two Apple sign-ins… ended without an obvious cause"): unrelated to anything I traced, but note that long-polls parked during a restart fail, and what the client does with a failed poll during token renewal is exactly the kind of interaction that entry asks about.
