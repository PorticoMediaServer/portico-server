# 22 — Client discovery, connectivity and resilience

## Scope and sources

Read-only static comparative audit, 11 October 2026. This domain covers finding a server, deciding which address to use, proving that it is the intended server, maintaining authentication, handling interrupted requests and playback reports, and distinguishing a user-server outage from a Hosted outage. No code was changed, built, installed, executed or tested. No browser, computer-use or MCP tools were used. No request was made to Portico infrastructure or the demo server, and `.private` was not accessed.

The execution host was `ehler-homelab-mini-1`, the active personal Ubuntu workspace. Required context-sync was attempted but could not write its lock outside the writable sandbox. Existing project and verification guidance was read. The shared audit brief was read in full. The audited working trees had no tracked modifications reported by the read-only status commands: server `82aaa54`, React Native `a3d8e7c`, internal `19cc2a7`. These are source snapshots, not deployed-version claims.

Portico documentation read: `Project Architecture/02 Server.md`, `06 Clients.md`, `07 Hosted Services.md`, `Specs/Spec — Hosted at Scale.md`, the networking, identity and onboarding product contracts, and relevant Backlog entries. Contracts contain older policy-lease language superseded by the Hosted-at-Scale membership model; that distinction matters throughout this report.

Primary Portico source areas read:

- Shared core: `apps/portico-server/packages/client-core/src/route-identity.ts`, `route-connection.ts`, `server-connections.ts`, `known-servers.ts`, `portico-servers.ts`, `installation.ts`, `local-session.ts`, `session-refresh.ts`, `session-selection.ts`, `viewer-selection.ts`, `session-end.ts`, `credentials.ts`, `console-transport.ts`, `bounded-json.ts`, `sse.ts`, `hosted-gate.ts`, `remote-access.ts`, `certificates.ts`, `setup-code.ts`, `setup-qr.ts`, `workspace-links.ts`, and the HTTP implementation in `index.ts`.
- Playback transport/recovery: `playback-v1/http.ts`, `local-http.ts`, `sessions.ts`, `timeline.ts`, `legacy-bridge.ts`, `events.ts`; guide deduplication; native Home reads and offline progress sync. Relevant networking/refresh test source was inspected as evidence of intended behavior, never run.
- Web: `web/src/bridge/server-connections.ts`, account/restore/transport adapters, `web/src/app/session.tsx`, `polyfills.ts`, `sign-in-errors.ts`, and Watch Together event wiring.
- Native: `apps/portico-react-native/apps/native/src/app/session.tsx`, `direct-connect.ts`, connection/event/session hooks, server and profile bridges, `useNativePlayback.ts`, `event-stream.ts`, `offline-downloads.ts`; Apple `Native/PorticoNetwork.swift`, its bridge surface and transport in `Native/PorticoPlayer.m`; Android `PorticoNetwork.kt`, `PorticoRuntime.kt`, `PorticoTransport.kt`. Vega `src/bridge/network.ts` and `transport.ts` were also read.
- Server: `server/internal/networking/discovery.go`, `route_identity.go`, `topology.go`, `remote_manager.go`, `control_retry.go`; connectivity discovery/status; Hosted schedule/membership paths; identity refresh/families; httpapi admission, route lanes, secure transport, auth response, CORS and overload handling; host limits and work-policy intent.
- Hosted: `portico-internal/hosted-services/hosted/internal/directory/routes.go`, `postgres/routes.go`, `directroute/service.go`, `wake/worker.go`, and direct DNS codec/table/answer logic.

Read-only recent Git history/diffs were inspected for the October 9 client, server and Roku resilience changes. Roku was used for the explicitly requested report-ordering intent only, not audited as a fourth complete client. The most consequential changes are retained-report ordering, bounded client work, separate server resource lanes and response budgets. Current code, including subsequent fixes, takes precedence over a commit's initial implementation.

Comparison snapshots: Jellyfin `a45d141`, web `319dbfc`, Plezy `ce361d9`, Swiftfin `2229d6f`, Streamyfin `805f1a5`, Finamp `673b568`, Android TV `2246041`. Source read includes Jellyfin's current `src/Jellyfin.Networking/AutoDiscoveryHost.cs`—the older UdpServer starting-point file no longer exists here—network URL overrides, SystemController, web ConnectionManager/fetch; Android TV auth/server repository; Swiftfin connection models/manager; Plezy connection models, endpoint races, failover, retry, caches, offline/aggregation/diagnostics; Streamyfin network provider/query defaults/local-network settings; and Finamp network automation and failed/offline-listen persistence.

Additional public sources were fetched as text with Python's standard HTTP client; no source or private data was sent externally. Jellyfin Kotlin SDK **v1.8.12**, the Android TV dependency version, was read directly from its official repository for address expansion, recommendation scoring and local UDP discovery. Plex and Firecore support documentation was read for secure connections, DNS rebinding and relay behavior. OkHttp's official source was read to verify its implicit retry default. The full external JavaScript SDK websocket implementation was not obtained; this report does not invent its keepalive constants. Plezy's cache and offline capabilities are described at the actual service boundary, not attributed wholesale to `api_cache.dart`.

For compact evidence references below, **Core** means `apps/portico-server/packages/client-core/src/`, **Web** means `apps/portico-server/web/src/`, **Native** means `apps/portico-react-native/apps/native/`, **Vega** means `apps/portico-react-native/apps/vega/`, **Server** means `apps/portico-server/server/`, and **Hosted** means `portico-internal/hosted-services/hosted/`, and **Android** means `apps/portico-react-native/apps/native/android/app/src/main/java/tv/getportico/portico/native/`. They are exact path prefixes relative to `/home/justin/projects/Portico`, not separate repositories. Comparison references start with the clone's name.

## How Portico does it today

### Finding and verifying a server

A native client can browse `_portico._tcp` using Bonjour on Apple or NSD (Android's network service discovery). The server runs its own bounded multicast DNS responder on UDP 5353. It advertises a version, server ID, public-key fingerprint, port, root path and display name, with A/AAAA addresses. It probes names before announcing, sends goodbye records on withdrawal, limits packets and records, rejects off-link traffic and restarts observation on topology changes. Discovery only supplies untrusted address hints. Browser clients do not get a Bonjour port from their environment; Vega explicitly returns an empty discovery result because its SDK has no service-discovery API. Manual address entry and setup-code approval remain alternatives. Evidence: Server `internal/networking/discovery.go:24,132–154,240–260,376–474`; Native `Native/PorticoNetwork.swift:33–85`; Android `PorticoNetwork.kt:248–312`; Vega `src/bridge/network.ts:16–18,98–103`.

Manual input receives a scheme guess: home-looking input gets HTTP, other input HTTPS. It removes surrounding whitespace and trailing slashes, but does **not** insert port 32500. A second, strict origin parser rejects credentials, paths, queries, fragments, odd IPv4 encodings and public HTTP. This creates mismatches between what the forgiving UI accepts and what the transport accepts. Origin-only URLs deliberately exclude reverse-proxy subpaths. Evidence: Native `src/app/direct-connect.ts:21–27`; Web `app/session.tsx:166–172`; Core `route-identity.ts:42–54`.

Before credentials are sent through the selected route, `probeRoute` POSTs a random 32-byte nonce and exact base URL to `/v1/networking/identity-proof`, with a 3-second deadline. The answer is signed by the server's Ed25519 identity key and binds the nonce, origin, server ID, public key, fingerprint and a 30-second validity window. The client recomputes both server ID and fingerprint from the key and verifies private-key possession. A remembered DHCP address answering as a different server therefore cannot pass this initial probe merely by copying an ID string. First manual pairing obtains a key from the contacted endpoint and uses it as a pin; this is trust on first use, not out-of-band authentication. Evidence: Core `route-identity.ts:56–60,96–108`; `server-connections.ts:67–80`; Server `internal/networking/route_identity.go:67–135`.

Hosted supplies account-scoped signed route documents. Documents carry current/previous connection evidence, endpoint/claim/credential generations, key identity, a 45-minute validity period and candidates classified as LAN/public/manual. Candidate quality is only `reachable` or `probe_required`, not authorization. Every winner still proves the pinned server identity. The client rejects an entire document if any candidate is invalid or duplicate. There is an important current mismatch: Hosted converts secure LAN literals into a LAN direct DNS name that this validator refuses (B-22.1–2). Evidence: Hosted `internal/directory/routes.go:37–54,132–151`; `internal/postgres/routes.go:122–179`; Core `route-identity.ts:74–84`.

TV setup uses an eight-character alphabet excluding ambiguous characters, accepts spaces/hyphens at entry, and displays a verification URL/QR whose code is in a fragment rather than a credential-bearing query. SetupCodeService durably records create, current grant and retired/cancelled operations; it bounds each request to 15 seconds, keeps at most eight retired operations, preserves a still-valid code during replacement failure and reconciles late responses instead of inventing a new sign-in. Native Hosted activation binds an encrypted X25519 grant to the TV's request/key; local Quick Connect uses the verified server route. Neither finding a nearby TV nor typing its code is server discovery authority. Evidence: Core `setup-code.ts:6–11,48–63,95–125,154–184,201–220`; `setup-qr.ts:1–7`; Native `src/bridge/setup-codes.ts:25–33,51–58`.

Clock policy is operation-specific. A server accepts a Hosted identity assertion up to five minutes of issuance/expiry skew, with a maximum ten-minute assertion lifetime and a five-minute challenge. This is separate from the client's much tighter route proof check: two minutes of future issuance tolerance but no expiry skew allowance for a thirty-second proof (B-22.16). Access refresh starts proactively two minutes before client-perceived expiry, with sixty-second request-time freshness threshold; there is no single shared calibrated time offset across these seams. Evidence: Server `internal/hosted/identity_assertion.go:44–56,121–133`; Core `session-refresh.ts:76–84`; `route-identity.ts:70–72`.

### Choosing and keeping a route

`RouteConnection` keeps a stable **logical origin** for ownership of commands and journals, while changing the physical transport origin. It races remembered candidates and native discovery, uses at most three probes with at most two LAN probes, gives public candidates a 120 ms delay where remembered LAN candidates exist, and cancels losers. Last-good priority precedes LAN priority. Each probe has 3 seconds; the race has 8.5 seconds overall. Fresh Hosted discovery starts after **3 seconds**, or earlier once remembered candidates conclusively fail. Fresh requests have a five-minute suppression interval. The contract's 500 ms hedge is not the current default. Evidence: Core `route-connection.ts:35,107–132`.

Network-change notifications invalidate a ready origin and trigger a new identity race. Web observes online/offline and the optional Network Information API; native observes Apple NWPathMonitor or Android network events. Foreground activity accelerates an already failed connection, but a ready connection is not reoptimized on foreground alone. Failed routes retry with exponential backoff starting around 5 seconds and ±10% jitter, capped around 5 minutes foreground or 15 minutes background. The browser's outer launch-restore loop stops scheduling while hidden; native's outer restore loop has its own fixed quick retries and two-minute slow retry. These are distinct loops with different timing, not one universal retry policy. Evidence: Core `route-connection.ts:81–105`; Web `bridge/server-connections.ts:42–56`, `app/session.tsx:233–314`; Native `src/bridge/server-connections.ts:151–171`, `src/app/session.tsx:448–514`.

A transport exception or non-JSON proxy 502/503/504 initiates same-server recovery. GET/HEAD replay once; mutations do not automatically replay on route loss. Portico JSON overload answers stay application responses. An older request failure cannot invalidate a newer route generation. A ready route is otherwise reused until an observed network transition or request failure; it is not bound cryptographically to every subsequent HTTP socket. Evidence: Core `route-connection.ts:137–167`.

### Request policy and credentials

`HttpLocalApi` adds selected credentials, installation and delivery headers; it proactively asks its refresh owner for a live token and retries a request once after 401 with a different token. Identity exchanges never recursively refresh themselves. This assumes 401 occurs before the operation acts. It does not install a universal deadline, byte limit, concurrent-request scheduler, GET retry ladder, cache or circuit breaker. Specialized surfaces supply their own policies. Evidence: Core `index.ts:338–415`, `installation.ts:39–80`.

Apple and Android bounded transports share connections, omit cookies/cache, reject redirects, validate TLS through platform trust, permit 12 requests active and 64 waiting, allow 1 MiB request bodies and 1 MiB responses (8 MiB for general API GETs), and set 15-second network deadlines. Apple's 15-second task deadline begins after its application queue admits the task; queue residence is additional. Android leaves OkHttp's implicit connection retry enabled. Browser global fetch has no equivalent default cap or deadline. Vega approximates the native limits but detects a redirect only after its SDK has followed it. Evidence: Native `Native/PorticoPlayer.m:92–172,235–253`; Android `PorticoTransport.kt:23–56,88–155`; Vega `src/bridge/transport.ts:8–10,66–91`.

Selected-server credentials, raw pinned key and route envelopes are saved together. Web uses encrypted IndexedDB in secure contexts and plain IndexedDB JSON for first-class LAN HTTP, plus Web Locks. On ordinary LAN HTTP, a polyfill substitutes **tab-local** locks; it does not coordinate separate tabs sharing the database. Native uses its protected record store. There is one selected-server record; the bounded known-server list holds up to 20 remembered direct-server labels/addresses without credentials. Switching servers is a selection handoff, not a merged catalogue. Evidence: Web `bridge/server-connections.ts:6–39`, `app/polyfills.ts:39–77`; Core `server-connections.ts:7–19`; `known-servers.ts:1–8,28,83–100`; `viewer-selection.ts:37–91`.

Server access tokens last 15 minutes; sign-in renewal rolls a 90-day local window. Refresh rotates credentials under a per-server credential lock, rereads storage, durably records a request ID before sending, and bounds the call to 15 seconds. Exact server receipts survive a restart and remain usable until successor use or family horizon; a different replay of a known predecessor revokes the family. The client retains credentials on transport, overload and invalid-result failure; 401/403 refusal ends it. A storage read exception is currently incorrectly collapsed into a missing sign-in (B-22.4). Evidence: Core `server-connections.ts:225–274`; Server `internal/identity/refresh.go:17–53,118–129,145–175`; `internal/identity/families.go:73–78,164–169`.

### Reporting and events during outages

The current UI playback adapter queues bounded **facts**—state transitions, seeks, markers and ends—with at most 64 reports per session and only one outstanding. Retries preserve the same sequence/body, back off exponentially with positive jitter, honor numeric server retry guidance and wait up to 20 seconds per report. Stop waits behind accepted facts. This is in-memory, per-session report ordering; it is distinct from the PlaybackService recovery journal and the durable downloaded-playback progress journal. The older generic TimelineReporter coalesces latest state and has a simpler fixed backoff; one should not assume its behavior describes the live UI adapter. Evidence: Core `playback-v1/legacy-bridge.ts:31–40,102–118,175–229`; `playback-v1/timeline.ts:86–95,196–259`.

Downloaded playback progress is durably grouped by the viewer whose receipt authorized it, coalesced per item, sent in batches of 100, and retained after failure. Receipts decide offline eligibility independently of a cached name or account login. Progress from another viewer never submits under the active viewer. Ordinary online timeline facts are not automatically converted into that download journal. Evidence: Native `src/bridge/useNativePlayback.ts:40–57`; `src/bridge/offline-downloads.ts:513,546–592`.

The device event owner supports SSE (server-sent events) and long polling. It resumes with cursors, deduplicates 512 recent IDs, broadcasts resync when the server ring cannot cover the gap, handles an explicit close/reconnect delay and changes to polling after repeated SSE failure. Quiet EOF is a failure with backoff, not a tight reconnect loop. Native's unified owner deliberately uses **10-second long polls** to fit its 15-second transport; it does not blindly use core's 25-second default. Separate native Watch Together/receiver XHR feeds still exist and bypass route remapping (B-22.18). Evidence: Core `playback-v1/events.ts:97–104,125–204`; Native `src/bridge/useNativePlayback.ts:98–102`; `src/app/together.tsx:19–23`, `receiver.tsx:57`.

### Hosted and server outages

Hosted is a control plane, not a media relay. In the current membership model, a Portico Account assertion admits the user into a **server-local** family. Thereafter profile selection, playback and refresh run against the server even with Hosted down. A brand-new account-linked connection still needs Hosted assertion/directory work; a new direct username/password connection does not. A server outage does not end the Hosted account. A healthy cached route avoids fresh Hosted discovery. The shared HostedGate defers automatic work, shares keyed calls and adds exponential positive jitter; interactive taps bypass it. It is process-local and forgotten at relaunch. Evidence: Core `portico-servers.ts:1–13,45–83`; `hosted-gate.ts:18–55`; `server-connections.ts:51–65,96–142`; Hosted-at-Scale spec §§1–7.

Server-list version checks are foreground-only, nominally six hours ±one hour, with a lightweight credential-free watch token and no PostgreSQL read for unchanged versions. Server check-ins, membership pushes, DNS feeds and signed wakes are separately paced. The wake worker retries with jitter and parses both Retry-After forms; callbacks are hints, not acknowledgements or authority. Evidence: Core `portico-servers.ts:143–215`; Hosted `internal/wake/worker.go:64–74,138–175`; Server `internal/networking/control_retry.go:33–50`; `internal/hosted/current_schedule.go:72–80`.

Under load, the server separates authentication, browsing, playback reports, media bodies, realtime and maintenance. Browsing gets 24 active slots and a 5-second work budget, authentication 64/5 seconds, established playback 32/10 seconds, with separate bounded waiting and fairness ledgers. A refusal returns JSON `503 server_busy` and `Retry-After: 1`; auth-rate refusal may request 60 seconds. The resource classes prioritize established playback and defer background work. Client native FIFO admission can still place those important requests behind browsing before they reach the server. Evidence: Server `internal/httpapi/admission.go:69–81,338–347,516–525`; `route_lanes.go:20,45`; `router.go:1298–1302`.

### Failure-message inventory

This inventory records the source strings or source message families, not proof that every device displays every string in every transition. Sign-in presenters replace many raw transport errors with localized, step-specific copy.

| Failure | Current source wording or behavior | Evidence |
|---|---|---|
| Route not found | “Your server is unavailable. Your sign-in is saved, and Portico will keep trying to connect.” | Core `route-connection.ts:36` |
| Verifying/network change | “Finding a verified connection to your server.” / “The network changed. Verifying the same server before reconnecting.” | Core `route-connection.ts:96,101` |
| Different identity | “An address answered with a different server identity. No credentials were sent.” | Core `route-connection.ts:102` |
| Wrong pinned endpoint | “This address belongs to a different server. No credentials were sent.” | Core `route-identity.ts:104` |
| Transient renewal | “The server couldn’t renew your sign-in right now.” | Core `server-connections.ts:262` |
| Refused renewal | “Your sign-in has ended. Sign in again.” | Core `server-connections.ts:257` |
| Access refusal | “This profile no longer has access to this server.” | Core `server-connections.ts:124` |
| Web launch cannot restore | “Your server is not responding. Portico keeps trying to reconnect.”; shell banner retains record | Web `app/session.tsx:285–292` |
| Native direct unreachable | “Portico couldn’t reach that server. Check the address and try again.” | Native `src/app/direct-connect.ts:51` |
| Native server unconfigured | “This server hasn’t finished setup. Complete setup in a web browser first.” | Native `src/app/direct-connect.ts:60` |
| Local permission denial | “Local Network access is denied. Manual addresses and TV codes still work.” | Native `src/bridge/server-connections.ts:109–118` |
| Gate open | “Your Portico Account service is temporarily unavailable. Portico will try again shortly.” | Core `hosted-gate.ts:51` |
| Server overload | “The server is busy handling other requests. Try again shortly.” | Server `internal/httpapi/admission.go:525` |
| Native queue full/deadline | “Too many queue requests.” / “The queue request could not be confirmed.” | Android `PorticoRuntime.kt:123`, `PorticoTransport.kt:102` |
| Console failure | Selected access ended; conflict/read current revision; capacity/retry later; uncertain result/reconcile same action | Core `console-transport.ts:18–27` |
| Lease expired/restart recovery | “The stream disconnected. Play again to continue.” / backup restore message | Core `session-end.ts:9–11` |
| Certificate degraded | Previous valid certificate remains during renewal; expiry makes remote TLS unavailable; recover from local owner access | Core `certificates.ts:64–81` |
| Remote setup failure | Explicit CGNAT/upstream forwarding/IPv6/reverse-proxy guidance; no bypass promise | Core `remote-access.ts:30–32` |
| Sign-in-specific failures | Separate server/account unavailable, timeout, incorrect credentials, access refusal, clock skew, identity, old server and storage messages | Web `app/sign-in-errors.ts:47–149`; native has its own presenter |

There is no dedicated DNS-rebinding, hairpin-NAT or captive-portal diagnostic in these paths. `secure_connection_required` is also not listed in the web sign-in code map; a 403 falls into generic forbidden wording. Missing messages can turn a recoverable network configuration into an apparent account problem.

## How the others do it

### Jellyfin

Jellyfin is principally a direct-server system: discovery and login work without a centralized account directory. Its current server listens on UDP **7359**, recognizes `who is JellyfinServer?` case-insensitively, and replies to the sender with JSON containing smart API URL, ID and name. URL selection can depend on the requesting subnet and owner-defined published URLs. The current host listens on IPv4 Any; this is not Portico's Bonjour/NSD service protocol. Evidence: `jellyfin/src/Jellyfin.Networking/AutoDiscoveryHost.cs:25,60–78,98–113`; `Manager/NetworkManager.cs:467–585,877`.

The web connection manager retains local/manual/remote addresses and races public system-info requests with 0/100/200 ms launch offsets and 20-second individual timeouts. It checks minimum server version and an expected ID before authenticated validation. This is an ID-string match, not proof of a private key. Timeout rejects a promise without aborting the fetch; losers already running continue. Schemeless entry tries HTTPS then HTTP. Its authentication-validation failure callback clears the stored token regardless of whether the error was a network failure—something Portico should **not** imitate. Evidence: `jellyfin-web/src/lib/jellyfin-apiclient/connectionManager.js:215–242,440–558,660–691`; `src/utils/fetch.js:46–68`.

Android TV uses SDK recommendation scoring and stores the chosen server address. The SDK expands schemes and common ports (8096/8920), probes at most three at a time with 3.5-second connect/request/socket timeouts and redirects disabled, and distinguishes product mismatch, TLS failure, slow response, absent version, outdated version and unsupported version. Android TV accepts a GREAT result or falls back to GOOD, surfaces issues otherwise, and has minimum/recommended/upcoming minimum version information. Discovery uses a 1 KiB UDP reply buffer, a 500 ms socket timeout and duplicate-ID suppression. Evidence: `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/auth/repository/ServerRepository.kt:54–59,101–175`; official SDK v1.8.12 `discovery/AddressCandidateHelper.kt`, `RecommendedServerDiscovery.kt`, `LocalServerDiscovery.kt`.

Jellyfin's event channel is websocket-based in its SDK clients. The web imports subscriptions and periodic listeners; precise reconnect/keepalive policy is dependency-owned in this snapshot. Portico's cursor-resumable SSE/long-poll feed is a materially different application protocol, not merely another socket library.

### Plex through Plezy

Plezy distinguishes a Plex account connection, containing cached resource servers and per-server access tokens, from a Jellyfin/Emby server-user connection with several base URLs and a long-lived access token. Plex resources include local/remote/relay and IPv4/IPv6 candidates, including `plex.direct` HTTPS names. Plezy synthesizes HTTP fallbacks only for eligible LAN HTTPS connections; it explicitly removed older synthesized WAN cleartext fallbacks. Plex probes carry `X-Plex-Token` immediately, a weaker precredential identity boundary than Portico's nonce proof. Evidence: `plezy/lib/connection/connection.dart:38–69,155–195`; `services/plex_auth_service.dart:390–417,904–917`.

Discovery has two phases: use the first viable candidate promptly, then measure candidates and promote a better route. Plex fallback tiers hold IPv6 until IPv4 direct has failed and relay until all direct candidates have failed. Later selection prefers local, direct, IPv4 and HTTPS before latency within the remaining class. The manager does not persist a relay as preferred and has relay-escape timers, generation-fenced profile changes and coalesced network-change reconnect sweeps. These are policies with costs, not universal best practice: stronger preference can postpone a usable fallback, and latency sweeps spend more network work. Evidence: `plezy/lib/utils/endpoint_race.dart:24–44,89–199`; `services/plex_auth_service.dart:473–484,840–894`; `services/multi_server_manager.dart:206–210,517–538,1100–1205`.

Plezy's shared failover client retries GET and explicitly replay-safe POST only. A current endpoint is rechecked after a connection failure; if it still answers after a timeout, it surfaces the timeout instead of rerunning an expensive query on another route. There is one authenticated retry per cascade, generation stamping prevents duplicate cascades, and persistence follows a successful retry. Separate retries allow two immediate connection-error retries within one total deadline, but do not retry timeouts. Evidence: `plezy/lib/utils/failover_http_client.dart:9–41,138–209`; `media_server_retry.dart:7–41,42–85`.

The manager tracks online, auth-rejected and access-refused servers separately. Aggregation fans out under per-server deadlines, preserves successful legs and reports failed/unqueried servers, so one failure does not erase other servers' results. Offline mode depends on real reachability of visible servers, not the platform's generic “internet connected” flag. Cache rows are server/user-scoped, support durable offline pinning and fresh-age checks. Offline watch actions have a database queue and reconciliation of superseded progress versus watched/unwatched changes. Evidence: `plezy/lib/services/data_aggregation_service.dart:132–220`; `providers/offline_mode_provider.dart:11–20,66–76`; `services/api_cache.dart:117–180`; `services/offline_watch_sync_service.dart:37–46,113–157,312–374`.

Plex's public secure-connection documentation explicitly covers routers and DNS providers that block private answers beneath public names. Firecore's Infuse documentation warns that Plex relay has a low bandwidth ceiling and that local traffic incorrectly using it can cause playback trouble. Portico's no-relay decision avoids that bandwidth dependency but loses relay's ability to get through difficult remote networks. Sources: [Plex secure server connections](https://support.plex.tv/articles/206225077-how-to-use-secure-server-connections/), [Infuse server integration](https://support.firecore.com/hc/en-us/articles/360006462093-Streaming-from-Plex-Emby-and-Jellyfin).

### Swiftfin, Streamyfin and Finamp

Swiftfin stores multiple named connections with priorities, interface restrictions and optional Wi-Fi names. It evaluates matching URLs **sequentially**, tests expected server ID with 8-second request/12-second resource deadlines, and switches the server's active connection. The probe may include an access token even though it asks public system info; its ID check is not Portico's cryptographic proof. NWPathMonitor triggers resolution; a foreground-resolution hook is commented out as potentially too greedy. Evidence: `swiftfin/Shared/Objects/ServerConnection.swift:11–79,95–102`; `Shared/Services/ServerConnectionManager.swift:69–148,156–176`.

Streamyfin configures React Query for stale-on-mount, 24-hour retention, offline-first reads, reconnect refetch, and up to three retries while online. It persists query data while excluding mutations. Its server reachability check uses public system info with a 10-second abort, specifically avoiding HEAD/root checks that fail behind subpath proxies. Its optional local-URL feature uses home Wi-Fi names and a permission-aware editor. Startup renders cached identity without waiting for an unbounded SDK validation request; definitive 401 clears the session centrally. Evidence: `streamyfin/app/_layout.tsx:281–322`; `providers/NetworkStatusProvider.tsx:31–48,74–102`; `components/settings/LocalNetworkSettings.tsx:95–102,161–206`; `providers/JellyfinProvider.tsx:293–309,1012–1092`.

Finamp supports user-controlled and automatic offline mode based on disconnected/network/unreachable choices, waits seven seconds and rechecks to avoid flapping, and can prefer a local address after pinging it. Failed or offline listens are stored in Hive and exported to a file. I did **not** establish automatic replay of every such listen to Jellyfin; persistence/export must not be described as a verified general progress-replay queue. Evidence: `finamp/lib/services/network_manager.dart:101–176`; `offline_listen_helper.dart:15–16,34–84`; `playback_history_service.dart:365–465`.

## Architectural differences

### A-22.1 A direct data plane with an optional account control plane

- **Portico:** Hosted discovers/admits users; server-local sessions own everyday traffic and refresh. Hosted-at-Scale §§1–7; Core `portico-servers.ts:1–13`.
- **Others:** Jellyfin has direct server identity without Hosted; Plex resources/account flows provide central discovery and relay options.
- **Why it matters:** Portico must keep two outage states separate, but can give central-account convenience without routing media through its infrastructure.
- **Assessment:** **Trade-off (subjective)**, deliberate. Keep the separation. Removing Hosted would be large and lose account discovery; making it part of everyday authorization would undermine the stated outage/cheap-Hosted goals.

### A-22.2 Bonjour/NSD versus a product-specific UDP discovery request

- **Portico:** Standard DNS-SD `_portico._tcp`, platform-native browsing and fingerprints. Server `internal/networking/discovery.go:24,249`.
- **Jellyfin/Plezy:** UDP 7359 request/reply JSON; Plezy sends twice 350 ms apart. `jellyfin/src/Jellyfin.Networking/AutoDiscoveryHost.cs:25,76`; `plezy/lib/services/jellyfin_lan_discovery_service.dart:22–45`.
- **Why it matters:** Platform service integration and IPv6 records favor Portico; VLAN/firewall/permission and responder correctness differ substantially.
- **Assessment:** **Trade-off (subjective)**, deliberate networking contract §5. Medium to add another protocol; do so only for demonstrated discovery failures, retaining the same identity proof.

### A-22.3 Discovery is an identity hint, never a credential destination by itself

- **Portico:** Fingerprint group validation, TTL checks, conflict refusal, then private-key proof. Core `route-connection.ts:20–25`, `route-identity.ts:96–108`.
- **Others:** Jellyfin/Plezy parse address/name/ID; no comparable signed nonce in reviewed discovery. Plex probes themselves already carry a token.
- **Why it matters:** Portico detects a remembered IP reassigned to another innocent server before first routed credential dispatch.
- **Assessment:** **Portico ahead**, deliberate. Preserve it; small to expose better user explanation, medium to close ongoing socket/restore bypasses described below.

### A-22.4 Persistent server identity is key-derived

- **Portico:** Server ID is a domain-separated hash of a 32-byte Ed25519 key, and fingerprint is its SHA-256 digest. Core `route-identity.ts:56–60`.
- **Others:** Compared Jellyfin clients check a reported machine ID, not proof of private-key possession.
- **Why it matters:** Copying an ID string is insufficient; server-key loss/restore becomes an identity lifecycle event.
- **Assessment:** **Portico ahead** for remembered-route authentication. Deliberate; replacing it would be large and undesirable without an equally strong trust model.

### A-22.5 Manual pairing is trust on first use

- **Portico:** An unpinned endpoint supplies and proves its own identity key during inspection. Core `server-connections.ts:67–73`.
- **Others:** Manual Jellyfin URLs likewise establish a first contacted server; TLS authenticates a trusted hostname where available.
- **Why it matters:** A successful nonce proof cannot tell a new user that a malicious server is the server they intended. An out-of-band QR/fingerprint would strengthen that first encounter.
- **Assessment:** **Trade-off (subjective)**. Medium to introduce a signed/address-independent pairing QR; preserve low ceremony, but verify UI confirmation actually communicates this boundary.

### A-22.6 Signed account-scoped route documents

- **Portico:** 45-minute signed evidence with audience, identity and generations; rejects invalid/duplicate candidates as a unit. Core `route-identity.ts:74–84`.
- **Others:** Plex resources arrive through the account API; Jellyfin URLs are local store/server/public-discovery configuration.
- **Why it matters:** Portico can reason about rollback and stale topology independently of bearer lifetime, at the cost of strict producer/consumer coordination.
- **Assessment:** **Portico ahead** on evidence structure, deliberate; B-22.1 demonstrates why complete wire-boundary fixtures matter. Medium to improve compatibility and canonicalization.

### A-22.7 Bounded current/previous route evidence

- **Portico:** Keeps current and eligible previous sets, up to eight paired origins and last-good; an omitted obsolete signed candidate cannot be resurrected by cache. Core `route-connection.ts:55–79,107–112`.
- **Others:** Plezy retains several user-entered historical URLs, removes proven machine mismatches but keeps inconclusive endpoints for later retry. `jellyfin_endpoint_discovery.dart:51–68`.
- **Why it matters:** Tight history limits stale recovery and memory; retaining more alternatives helps complex homes/VPNs.
- **Assessment:** **Trade-off (subjective)**, deliberate. Small/medium to make manual alternatives editable without relaxing signed revocation rules.

### A-22.8 LAN gets a short head start, not a full serial timeout

- **Portico:** Public probes wait 120 ms; three active probes, two LAN maximum. Core `route-connection.ts:122–126`.
- **Jellyfin:** Web starts local/manual/remote at 0/100/200 ms. Swiftfin evaluates ordered connections serially.
- **Why it matters:** Portico avoids long dead-LAN stalls while still biasing local traffic.
- **Assessment:** **Portico ahead** over sequential fallback; similar to Jellyfin web but with stronger cancellation. Deliberate. Small to tune after physical-network measurements; do not change constants from intuition alone.

### A-22.9 First verified winner versus two-phase optimization

- **Portico:** Cancels losers immediately and retains the winner until failure/change. Core `route-connection.ts:115–117,126`.
- **Plezy:** Emits first winner, then measures and promotes a better candidate. `utils/endpoint_race.dart:158–199`.
- **Why it matters:** Portico uses less work; Plezy is better at escaping a usable but inferior remote/relay route.
- **Assessment:** **Trade-off (subjective)**. Medium to add local-only promotion on foreground/network evidence; avoid periodic Hosted chatter and mid-playback churn.

### A-22.10 Last-good precedes LAN quality in ordering

- **Portico:** Sorts last-good first, then LAN, then quality; class scheduling still imposes the public delay. Core `route-connection.ts:113,123`.
- **Plezy:** Explicit local/remote/relay and IPv4/IPv6 tiers, with HTTPS ranking and latency measurement.
- **Why it matters:** A historically remote route can remain preferred; a speedy success is not always the cheapest or most desirable path.
- **Assessment:** **Trade-off (subjective)**. Small/medium to define preference versus readiness separately. Current simple ordering is defensible if users rarely see route-quality problems.

### A-22.11 Fixed per-origin proof deadlines versus phase-specific transport budgets

- **Portico:** 3-second identity proof and 8.5-second race; native general network call 15 seconds. Core `route-identity.ts:98`; `route-connection.ts:115`.
- **Plezy:** Separates connect 10 seconds, headers 30 seconds and body 120 seconds, plus surface deadlines. `utils/media_server_timeouts.dart:5–36`.
- **Why it matters:** Portico fails promptly; slow disks/proxies/mobile networks may need a different treatment from a dead endpoint.
- **Assessment:** **Trade-off (subjective)**. Medium to formalize per-phase or per-operation budgets. Keep upper bounds and cancellation; make acceptance evidence decide generosity.

### A-22.12 Fresh directory discovery is hedged and rate-limited

- **Portico:** Default hedge 3 seconds; suppresses fresh Hosted requests for five minutes per connection. Core `route-connection.ts:35,119,132`.
- **Others:** Jellyfin has no equivalent Hosted recovery; Plezy re-races available resources and coordinates reconnect sweeps.
- **Why it matters:** Cheap Hosted and low chatter improve; a newly changed address can remain undiscovered longer after a failed fetch.
- **Assessment:** **Trade-off (subjective)**, deliberate implementation comment, but contradicts 500 ms contract. Small to reconcile the contract; medium to make fresh suppression depend on failure class and topology generation.

### A-22.13 Platform network changes are hints, not proof of server loss

- **Portico:** Network event starts fresh same-identity probing; LAN use does not require generic internet availability. Core `route-connection.ts:95`; native network bridge.
- **Plezy:** Explicitly re-probes even when connectivity reports `none`; unknown plugin state assumes online. `offline_mode_provider.dart:66–76`; `connectivity_probe.dart:30–63`.
- **Why it matters:** WAN outages, local-only Wi-Fi and loopback should remain useful.
- **Assessment:** **Portico ahead** relative to a global internet gate, and broadly aligned with Plezy. Deliberate contract §11. Small to improve explicit path-state messages; runtime permission/VPN cases remain unverified.

### A-22.14 Ready connections are not re-evaluated on foreground alone

- **Portico:** `foregroundActivity` returns immediately when ready. Core `route-connection.ts:86–88`.
- **Others:** Plezy has optimization/reconnect orchestration; Swiftfin's similar foreground hook is currently commented out.
- **Why it matters:** A resumed app can keep a remote route even when a better LAN route now works without a reported interface change.
- **Assessment:** **Trade-off (subjective)**. Medium for bounded foreground preference checks; it should not interrupt healthy playback or force fresh Hosted calls.

### A-22.15 HTTP remains first-class, public HTTP is refused

- **Portico:** Local HTTP accepted; public addresses require HTTPS; HTTP/HTTPS issue the same server credentials. Core `route-identity.ts:42–54`; Server `internal/httpapi/auth_response.go:5–11`.
- **Plezy/Plex:** Eligible LAN cleartext fallbacks exist; public cleartext twins of HTTPS are intentionally excluded. Jellyfin web tries HTTPS then HTTP for schemeless input.
- **Why it matters:** Low-ceremony homes retain access, but local-route trust and channel confidentiality must be described precisely.
- **Assessment:** **Trade-off (subjective)**, explicitly deliberate. Medium to add stronger HTTP proof-of-possession or secure local names; do not remove HTTP merely because competitors prefer TLS.

### A-22.16 A secure-LAN DNS alias replaces the raw IP

- **Portico:** Hosted rewrites HTTPS LAN literals to its LAN direct name; public wildcard certificate covers that name. Hosted `internal/postgres/routes.go:164–172`; direct DNS `server.go:441–445`.
- **Plex:** `plex.direct` serves trusted names for private IPs; Plezy can fall back to raw LAN HTTP.
- **Why it matters:** A certificate for a DNS name does not authenticate a raw IP literal. Private-answer DNS protection can break this useful secure path.
- **Assessment:** **Trade-off (subjective)** as a design, presently **behind** in implementation because the client rejects it. Medium to repair the wire model and diagnostics; retain ordinary public PKI validation.

### A-22.17 Private-address classification excludes Tailscale's IPv4 range

- **Portico:** Core permits RFC1918, loopback and link-local, but not 100.64/10 HTTP; server public-address logic excludes CGNAT. Core `route-identity.ts:42–46`; Server `internal/networking/topology.go:96–104`.
- **Plezy:** Its Plex local classification includes 100.64/10 and `.ts.net`; routed VPN use is acknowledged. `plex_auth_service.dart:956–979`.
- **Why it matters:** `http://100.x.y.z:32500` is a common private overlay address, yet Portico calls it public/insecure.
- **Assessment:** **Trade-off (subjective)**, security classification is deliberate but overlay support is incomplete. Small/medium for explicit owner-approved overlay policy; avoid broadly allowing public HTTP.

### A-22.18 Reverse-proxy origins supported, subpaths refused

- **Portico:** A custom HTTPS origin is a manual candidate; `/media` and other path prefixes are rejected. Core `route-identity.ts:49–54`; Server remote access URL seam.
- **Jellyfin/Streamyfin/Plezy:** Base URLs can include subpaths; Jellyfin warns if published path and BaseUrl disagree, Streamyfin probes the API instead of root HEAD.
- **Why it matters:** Dedicated hostnames work well; owners sharing one hostname among services need extra proxy setup.
- **Assessment:** **Trade-off (subjective)**, deliberate origin-only contract. Medium/large to add a rigorously canonical base-path model across API, playback, links, probes and capabilities.

### A-22.19 No relay is an architectural and product boundary

- **Portico:** Port forwarding, public IPv6, reverse proxy or VPN; no hosted media relay. Networking contract §1 and brief product decisions.
- **Plex:** Relay fallback can connect otherwise unreachable servers; Infuse documents its low-bandwidth limitations.
- **Why it matters:** Portico avoids bandwidth bills and a media dependency on Hosted, but CGNAT/simple remote setup is harder.
- **Assessment:** **Trade-off (subjective)**, deliberate. Large and a product/pricing decision to adopt; recommend owner-friendly diagnosis before a relay project.

### A-22.20 Remote reachability is not inferred from a certificate or router lease

- **Portico:** Publishes/records topology and uses identity-pinned external proof; IPv6 requires pinhole/open-firewall evidence or owner confirmation. Server `internal/networking/remote_manager.go:423–443,639–655`.
- **Others:** Plex resources list address classes; Jellyfin publishes configured/smart URLs without this Hosted proof system.
- **Why it matters:** UI can honestly distinguish “TLS ready” from “reachable from elsewhere.”
- **Assessment:** **Portico ahead**, deliberate. Keep it; medium to extend evidence to richer user-facing failure reasons without implying external proof covers every client's path.

### A-22.21 DNS names are stateful current addresses with membership-label rotation

- **Portico:** Current `c-<label>.<namespace>` and LAN aliases, previous-label grace and DNS table feed. Hosted `internal/directroute/service.go:1–10,139–144`.
- **Plex/old Portico contract:** Plex names can encode an address; Portico contract §8 still describes stateless address-encoded names.
- **Why it matters:** Current DNS can follow WAN changes without changing a remembered URL, but needs functioning authoritative nodes/feed and cache convergence.
- **Assessment:** **Trade-off (subjective)**, current implementation deliberate. Medium to improve documentation and test DNS-cache/outage transitions. This is not stateless DNS today.

### A-22.22 One active server instead of simultaneous catalogue aggregation

- **Portico:** One selected record and fenced viewer handoff; known-server labels carry no credentials. Core `server-connections.ts:7–19`, `viewer-selection.ts:37–91`.
- **Plezy:** Simultaneous per-server clients and merged library/Home/search results with partial success.
- **Why it matters:** Portico's scope and permission boundaries are simpler; multiple households/servers cost more switching.
- **Assessment:** **Trade-off (subjective)**, deliberate thin-client/server-authored direction. Large to aggregate and maintain exact server provenance, paging and restrictions; a better switcher is smaller.

### A-22.23 Per-server failure isolation has different meaning

- **Portico:** Unavailable selected server leaves account/chooser operational; there is no cross-server request fanout to isolate. Networking contract §6.
- **Plezy:** Each failed aggregation leg is recorded while others succeed. `data_aggregation_service.dart:158–220`.
- **Why it matters:** A Portico user's page depends on one server; Plezy can show partial rows with explicit completeness semantics.
- **Assessment:** **Trade-off (subjective)**. Medium for richer chooser status; large for merged partial-result semantics. Do not call this a bug unless merged browsing becomes required.

### A-22.24 Durable rotated sessions versus long-lived access tokens

- **Portico:** Fifteen-minute access token plus rotating refresh and device-bound installation. Server `internal/identity/families.go:73–78,158–169`.
- **Plezy/Jellyfin:** Stored long-lived access token, no analogous refresh cycle in reviewed connection model. `connection.dart:190–195`.
- **Why it matters:** Portico improves revocation/device lifecycle but adds storage races, clock dependence and recovery work.
- **Assessment:** **Trade-off (subjective)**. Retain the model if its recovery guarantees are enforced end to end; medium to harden refresh rather than remove it.

### A-22.25 Refresh has a persistent request ID and restart-safe receipt

- **Portico:** Exact rotation request is persisted before dispatch and replays after a lost answer; receipt lasts until successor use/horizon. Core `server-connections.ts:237–246`; Server `internal/identity/refresh.go:118–129`.
- **Others:** Long-lived-token clients do not need rotation receipts; standard failure retry is not equivalent.
- **Why it matters:** A restart between commit and response need not strand the user or count as stolen refresh reuse.
- **Assessment:** **Portico ahead** for its chosen auth model, deliberate. Keep it and remove stale five-minute comments. Small for documentation, medium for storage/coordination failure fixtures.

### A-22.26 Refusal is distinct from transient failure

- **Portico:** Server refresh ends only on 401/403; request/transport/5xx retain credentials. Core `server-connections.ts:247–266`.
- **Jellyfin web:** Public connection succeeds, then any authentication-validation rejection clears access/user fields. `connectionManager.js:233–242`.
- **Why it matters:** Restart/overload should not force a password prompt.
- **Assessment:** **Portico ahead** in the intended path. Keep it; B-22.4–5 show exceptions elsewhere. Small/medium to make classification consistent rather than copied at every bridge.

### A-22.27 Single-flight is layered across runtime and storage

- **Portico:** Refresher shares a flight; credential lock rereads another tab's rotation; selected record updates have ownership guards. Core `session-refresh.ts:108–125`; `server-connections.ts:225–240`.
- **Others:** Plezy generation-fences client/profile swaps and coalesces network sweeps; it avoids token rotation complexity for Jellyfin.
- **Why it matters:** Sharing a promise only within one React hook is insufficient when the credential store is shared across contexts.
- **Assessment:** **Portico ahead** in secure contexts, **behind** on ordinary HTTP multi-tab fallback. Medium to supply true cross-tab coordination or explicitly isolate credentials by tab.

### A-22.28 General transport policies remain surface-owned

- **Portico:** General `request` delegates deadlines/retries/bounds to callers; console, lyrics, diagnostics, Home and playback differ. Core `index.ts:282–415`.
- **Plezy:** Shared connect/header/body budgets plus narrower surface deadlines; Streamyfin query defaults centralize retry/cache policy.
- **Why it matters:** Feature authors can inadvertently create indefinite calls or inconsistent overload handling.
- **Assessment:** **Portico behind** for policy consistency. Medium for one operation-classed transport that permits explicit long-poll/media exceptions and avoids mutation replay.

### A-22.29 Native transport admission is bounded FIFO

- **Portico:** 12 active/64 queued, shared cookie-less transport; no priority reservation for renewal/timeline. Native transport citations above.
- **Plezy:** Its HTTP layer handles abort/deadline/failover; compared manager/source does not establish the same application-level FIFO cap.
- **Why it matters:** Portico bounds client memory and sockets, but important work can wait behind browsing.
- **Assessment:** **Trade-off (subjective)**. Keep the bounds; medium to reserve or prioritize small control work without starving user reads.

### A-22.30 Browser and native response memory boundaries differ

- **Portico:** Native bytes capped before JS response construction; general browser response JSON/text is not capped. Specialized browser readers incrementally cap streams. Core `bounded-json.ts:4–11`, `index.ts:376–414`.
- **Others:** Kotlin recommendation discovery tightly caps time; Plezy and Streamyfin generally use their HTTP/SDK object parsers rather than Portico's exact per-document bounds.
- **Why it matters:** Validating a parsed shape does not bound the bytes allocated to parse it.
- **Assessment:** **Portico ahead** on specialized/native bounds, **behind** on shared consistency. Medium to generalize bounded reads, preserving large permitted GET documents.

### A-22.31 Mutation recovery belongs to the operation owner

- **Portico:** Route layer never blind-replays mutations; personal/queue/start operations supply operation IDs/keys and revision reconciliation. Core `route-connection.ts:135–160`; `detail.ts:259–316`; `playback-v1/http.ts:61–64`.
- **Plezy:** GET failover by default; POST only opt-in replay-safe work. `failover_http_client.dart:12–17`.
- **Why it matters:** A disconnected response may follow a committed operation; retrying a POST is not safe just because the transport failed.
- **Assessment:** **Portico ahead** in explicit receipt-oriented design, deliberate. Small/medium to audit hidden platform retry and ensure every replayable path retains its key.

### A-22.32 401 retries apply to all methods once

- **Portico:** Exchange renews then resends once, including mutations, based on the pre-effect 401 assumption. Core `index.ts:365–369,393`.
- **Others:** Jellyfin long-lived tokens generally return to auth; Plezy separates auth failure from endpoint exhaustion.
- **Why it matters:** Correct server middleware ordering makes this safe; a proxy-origin 401 or post-effect custom route would violate the assumption.
- **Assessment:** **Trade-off (subjective)**. Medium to document/test pre-effect refusal guarantees across all mutable routes; no confirmed double-application was found in this path.

### A-22.33 No universal selected-server circuit breaker

- **Portico:** Route single-flight and per-surface backoff, but shared gate is for Hosted only. Core `hosted-gate.ts:1–12`; general `index.ts:387–415`.
- **Others:** Plezy manager tracks per-server endpoint health; Streamyfin gates retries by online status rather than a Portico-style circuit.
- **Why it matters:** Many features may discover one overloaded server independently and issue repeated reads.
- **Assessment:** **Portico behind** for coordinated load recovery. Medium; use server guidance and work classes, not a blanket gate that delays playback or interactive recovery unnecessarily.

### A-22.34 Server overload is typed and resource-classed

- **Portico:** Bounded auth/browse/playback/realtime lanes and JSON retry guidance. Server `internal/httpapi/admission.go:69–81,524–525`.
- **Others:** Reviewed Jellyfin networking/discovery paths do not expose an analogous comprehensive client-facing lane contract.
- **Why it matters:** A busy server can remain responsive to playback reports and renewal while background work yields.
- **Assessment:** **Portico ahead**, deliberate scale principles. Medium to align client priorities and prove physical capacity; static structure is not a 200-browser performance result.

### A-22.35 Retry-After is several policies, not one

- **Portico:** General API supports seconds/date; playback and list-check accept numbers only; browser account wrapper drops it; server wake supports both. Core `index.ts:134–141`; `playback-v1/http.ts:42–45`; `portico-servers.ts:191–192`; Web `bridge/account.ts:103–105`.
- **Others:** Plezy uses operation deadlines/failure types; it is not proof of correct header support everywhere either.
- **Why it matters:** A reverse proxy can legally send an HTTP date, so otherwise correct recovery can disregard its floor.
- **Assessment:** **Portico behind** in consistency. Small to share parsing and preserve guidance across error translation; medium to verify every operation's scheduling behavior.

### A-22.36 Jitter exists for route, event and timeline recovery but not every auth loop

- **Portico:** Route ±10%, events ±25%, legacy timeline positive 0–20%; refresh retry fixed 5/15/30/60/120 seconds. Core route/events/legacy/refresher citations.
- **Others:** Plezy coalesces reconnects and debounces network flaps; Finamp delays/rechecks offline transitions.
- **Why it matters:** Synchronized sign-ins can synchronize token expiry and hammer a restarting server even while event/report loops spread themselves.
- **Assessment:** **Portico behind** on refresh/outer-native-restore jitter. Small to add proportional jitter and common retry floors.

### A-22.37 Guide read reuse is bounded but time-based

- **Portico:** Guide reads deduplicate per authenticated API object, reuse completed results for 10 seconds and retain up to 64 entries. Core `guide/reads.ts:5–6,37–63`.
- **Others:** Plezy explicit fresh-age cache; Streamyfin stale/retention policies are query-layer time based.
- **Why it matters:** Portico avoids overlap and bounds memory, but this is an exception to “no time-based caches of derived results.”
- **Assessment:** **Trade-off (subjective)**, recent intentional resilience work. Small to document whether it is an accepted transport memo exception; medium for revision-keyed invalidation if stronger freshness is required.

### A-22.38 Subscriber-aware deduplication cancels only orphaned reads

- **Portico:** One guide transport for several subscribers; last subscriber abort cancels it, one departing subscriber does not. Core `guide/reads.ts:31–36,45–91`.
- **Others:** Query frameworks deduplicate keyed queries; Plezy has explicit abort controllers and manager flights.
- **Why it matters:** Scrolling or navigation should stop useless server work without cancelling another visible viewport's request.
- **Assessment:** **Portico ahead** in this narrowly implemented surface. Medium to generalize with scope/revision-aware keys instead of global endpoint caching.

### A-22.39 ETags and revisions are application-owned

- **Portico:** Playback/resources expose ETag/If-Match; Hosted watch versions support 304; general HttpLocalApi does not transparently maintain a conditional-response cache. Core `index.ts:376–385`; `portico-servers.ts:184–198`.
- **Others:** Plezy API cache stores JSON by server/endpoint; Streamyfin retains query results and invalidates/refetches rather than requiring Portico revisions.
- **Why it matters:** Exact revision models improve consistency, but APIs cannot assume every caller automatically sends `If-None-Match`.
- **Assessment:** **Trade-off (subjective)**, deliberate fresh-data direction. Medium to add scoped exact-revision read reuse where documents expose sufficient evidence.

### A-22.40 Online report facts are ordered and bounded

- **Portico:** At most 64 pending facts, one outstanding, same sequence/body replay, stop waits behind facts. Core `playback-v1/legacy-bridge.ts:102–118,175–229`.
- **Others:** Plezy offline watch DB queue has semantic reconciliation; Finamp retains offline/failed listens.
- **Why it matters:** The server cannot receive end before prior accepted marker/seek facts solely due to parallel HTTP completion order.
- **Assessment:** **Portico ahead** on live sequence discipline. Deliberate October 9 work. Medium to clarify crash durability and user experience at a full fact queue.

### A-22.41 Online reporting and offline downloaded-progress sync are separate

- **Portico:** Online facts stay in a session queue; downloaded progress is owner-scoped durable per-item state. Native `src/bridge/offline-downloads.ts:546–592`.
- **Plezy:** A shared offline-watch queue handles progress/watched/unwatched actions across supported backends.
- **Why it matters:** Portico has clear authorization boundaries, but online playback through a crash/outage is not automatically recovered by the download journal.
- **Assessment:** **Trade-off (subjective)**. Medium to persist selected report facts or final online observations without duplicating historical actions or crossing viewer boundaries.

### A-22.42 One cursor-resumable device feed with explicit resync

- **Portico:** Cursor replay, duplicate suppression, resync broadcast, close hints and polling fallback. Core `playback-v1/events.ts:1–12,97–104,158–204`.
- **Jellyfin:** Websocket subscriptions/periodic listeners, no equivalent cursor guarantee established from reviewed caller source.
- **Why it matters:** Clients can distinguish a recoverable event gap from complete delivery and refetch after ring loss.
- **Assessment:** **Portico ahead** in protocol explicitness, deliberate. Medium to verify subscriptions honor resync and native auxiliary feeds use the same route/auth boundary.

### A-22.43 Native uses short polling rather than core's default SSE

- **Portico:** Unified native events explicitly use 10-second long polls, no SSE opener. Native `src/bridge/useNativePlayback.ts:98–102`.
- **Web:** May stream; Core fallback defaults to a 25-second long poll. Jellyfin commonly uses websockets.
- **Why it matters:** Native avoids buffered-fetch/SSE limitations and known 15-second quiet-poll failure, but makes about six idle polls a minute per active device.
- **Assessment:** **Trade-off (subjective)**, intentional current fix. Medium to support authenticated bounded native streaming; budget must preserve cheap user-server operation, not just cheap Hosted.

### A-22.44 Server-list checks are version probes, not presence polling

- **Portico:** Six-hour ±one-hour foreground-only watch checks; no background list polling. Core `portico-servers.ts:143–215`.
- **Others:** Plezy manages active per-server health/optimization; Jellyfin direct clients remember their own servers.
- **Why it matters:** Hosted stays cheap and an offline server is not inferred from the account directory, but new memberships may take hours to appear without an explicit event/refresh.
- **Assessment:** **Portico ahead** on control-plane cost, deliberate Hosted-at-Scale. Small/medium to make explicit refresh and change signals reliable without adding heartbeat traffic.

### A-22.45 Server-side Hosted retries persist and spread fleet recovery

- **Portico:** Persisted schedule/membership outbox plus exponential proportional positive jitter and server-provided floor. Server `internal/networking/control_retry.go:33–50`; `internal/hosted/membership_push.go:360`.
- **Jellyfin:** No comparable centralized server directory to recover; Plex closed server implementation was not audited.
- **Why it matters:** Relaunching a server need not reset fleet backoff or lose membership updates.
- **Assessment:** **Portico ahead**, deliberate. Keep it; medium to validate crash/restart and queue-drain ordering at scale.

### A-22.46 Wakes are signed hints with durable acknowledgement tracking

- **Portico:** Hosted calls verified public endpoints, signs short-lived hints, retries even after successful delivery until exact convergence. Hosted `internal/wake/worker.go:35–38,138–175`.
- **Others:** Jellyfin direct networking needs no central targeted wake; Plex implementation not established here.
- **Why it matters:** A callback's 200 cannot be mistaken for completed account cleanup or directory convergence.
- **Assessment:** **Portico ahead** in distributed-work discipline, deliberate. Small for clearer docs; medium for missed wake/private-proxy fallback acceptance.

### A-22.47 Offline launch prioritizes shell/download access differently by platform

- **Portico:** Web enters shell with saved-server unavailable banner; native gives saved-server/recovery state after a bounded launch wait, retaining downloads under receipt rules. Web session and native resume citations above.
- **Streamyfin/Plezy:** Cache/identity can render immediately and offline mode follows server reachability; broader cached browsing can survive.
- **Why it matters:** Saving credentials is not the same as having useful offline Home/detail content.
- **Assessment:** **Portico behind** on broad offline browsing, but intentional freshness/privacy trade-offs are reasonable. Medium/large for clearly labelled cached snapshots; downloads remain a separate, stronger authority boundary.

### A-22.48 Explicit URL/interface preferences versus automatic route hints

- **Portico:** Automatic route classes and last-good, with owner access URLs; no per-device named interface/SSID priority model in reviewed stores.
- **Swiftfin/Streamyfin/Finamp:** Ordered named connections, home Wi-Fi rules or a local-address preference.
- **Why it matters:** Users with split-horizon DNS, VPNs, multiple homes and hairpin limitations can supply better intent than a blind race.
- **Assessment:** **Trade-off (subjective)**. Medium to add a compact advanced preference editor, with cryptographic probes still mandatory and no SSID-as-authentication shortcut.

### A-22.49 Compatibility is document/feature validation, not a system version gate

- **Portico:** Identity proof version 1 and bounded parsers reject incompatible shapes; public system version is read but not compared in connect paths. Missing challenge nonce produces `client_update_required`. Core route identity; Native `src/app/direct-connect.ts:58–60`; Server `internal/httpapi/direct_identity.go:38–42`.
- **Jellyfin SDK/web:** Explicit minimum/recommended version gates and diagnostic scoring.
- **Why it matters:** Pre-release coordinated deployment works; after release an older client may fail far into login/playback with a generic incompatible response.
- **Assessment:** **Portico behind** for release-era negotiation. Medium to define stable minimum capabilities/protocol versions and precise update messages; avoid semver gates that reject otherwise usable features unnecessarily.

### A-22.50 Connection diagnostics remain less specific than failure evidence allows

- **Portico:** Strong identity/network state machine, but DNS/TLS/permission/proxy/clock errors often collapse to unreachable, forbidden or identity wording.
- **Jellyfin SDK/Plezy:** Recommendation issues distinguish TLS, product, version and latency; Plezy also diagnoses Android background/data-saver/OEM restrictions.
- **Why it matters:** A router blocking LAN DNS, a captive portal and a revoked account require different user actions.
- **Assessment:** **Portico behind** for actionable diagnostics. Medium to expose classified, non-sensitive causes while retaining simple primary UI.

### A-22.51 Discovery responder recovery and hard bounds are unusually explicit

- **Portico:** Packet size, query count, compression-pointer hop checks, interface caps, hop-limit checks, rate-limited responses, conflict withdrawal and topology retry. Server `internal/networking/discovery.go:56–99,127–154,273–350,433–473`.
- **Jellyfin:** Simple UTF-8 request substring/JSON response, bind failure logged without in-process restart. `AutoDiscoveryHost.cs:74–95`.
- **Why it matters:** Portico limits hostile LAN input and handles network lifecycle more deliberately, but maintaining a custom mDNS implementation carries protocol-edge risk.
- **Assessment:** **Portico ahead** on bounds/recovery; **Trade-off** on custom protocol maintenance. Medium to add standards/network fixtures; switching to a library is not automatically better.

### A-22.52 Vega cannot enforce the same redirect contract

- **Portico:** Apple/Android refuse redirects before following; Vega detects final URL only on completed response. Vega `src/bridge/transport.ts:8–10,87–91`.
- **Others:** Jellyfin Kotlin recommendation probes explicitly disable redirects; Plezy validates effective redirects and disallows cross-host/downgrade changes.
- **Why it matters:** Rejecting a response after a 307/308 follow cannot prevent credential-bearing body transmission.
- **Assessment:** **Portico behind**, documented SDK limitation, not an acceptable claim of equivalence. Medium/large for a safe platform transport or a fail-closed credential exchange feature gate.

### A-22.53 TV activation is a durable authorization operation

- **Portico:** Saved create/redeem/cancel state, preserved current code through replacement failure, explicit late-result quarantine, bounded retired operations and TV-bound encrypted Hosted grant. Core `setup-code.ts:60–105,154–184,201–220`; Native `src/bridge/setup-codes.ts:25–33`.
- **Others:** Jellyfin Quick Connect and Plex PIN account login serve related convenience goals, but the reviewed discovery/connection models do not establish Portico's same durable cancellation/encrypted-grant contract.
- **Why it matters:** Restarting or leaving a setup screen must not orphan a valid credential or let a stale approval publish into a newer selection. QR/code convenience must remain independent of LAN permission.
- **Assessment:** **Portico ahead** in explicit operation lifecycle, deliberate onboarding contract. Keep it; medium to coordinate failure backoff with HostedGate and verify platform grant installation/storage boundaries.

### A-22.54 Clock tolerance differs between server admission and client route proof

- **Portico:** Hosted assertion verification permits five-minute skew, while a route proof expires after thirty seconds with no client-ahead expiry grace. Server `internal/hosted/identity_assertion.go:44–56,121–133`; Core `route-identity.ts:70–72`.
- **Others:** Jellyfin public-info/ID probes have no signed short-expiry clock requirement in reviewed code. Plex token/SDK clock behavior was not independently established.
- **Why it matters:** A clock tolerated by server login can still prevent the client from ever getting to login; device clock and server-versus-Hosted clock are different failure boundaries.
- **Assessment:** **Trade-off (subjective)**. Short proof validity improves freshness, but nonce uniqueness already prevents replay for this exchange. Medium to define a coherent bounded clock policy and distinct actionable messages.

## Missing features and capabilities to consider

### F-22.1 A user-editable set of addresses for one pinned server

Swiftfin exposes named ordered connections; Plezy's Jellyfin connection stores several base URLs. Portico has paired origins internally but its known-server list exposes only one remembered address and one selected credential record. Give owners/users a way to add, remove and prioritize LAN, VPN and reverse-proxy alternatives for the **same pinned identity**, without signing in to each as a separate realm. Value: moving between homes/VPNs and repairing DHCP changes. **Size: medium**, connection store, chooser and settings. **Opinion:** high value; keep automatic selection as the default and cryptographic verification mandatory. No principle conflict.

### F-22.2 A common operation-classed request policy

Plezy centralizes phase budgets and failover rules; Streamyfin centralizes query retry defaults. Portico should have shared defaults for small reads, large bounded reads, auth, mutations, timeline and long-poll, including deadlines, byte limits, cancellation and retry classification. **Size: medium/large**, core and bridges. **Opinion:** highest implementation value; avoid a generic retry-on-any-error wrapper. Fits thin shared clients and bounded work.

### F-22.3 Shared Retry-After parsing and preservation everywhere

Portico already has a correct seconds/date helper; several paths bypass it or discard its result. Use it across playback, list watch, Hosted account, console error translation and refresh. **Size: small/medium**. **Opinion:** do this before additional automatic retries. No principle conflict; the server/Hosted recovery contract already demands it.

### F-22.4 Small control requests reserved ahead of browsing

Server work lanes prioritize established playback; native client admission currently has one FIFO. Add reserved slots or a fair priority scheduler for token renewal, timeline/end, identity probes and cancellation/reconciliation. **Size: medium**, native bridges plus shared operation labels. **Opinion:** useful on slow networks/large artwork/Home bursts; measure starvation rather than reducing all concurrency blindly. Fits playback-first resource discipline.

### F-22.5 A bounded “return to LAN” preference check

Plezy's second phase/relay-escape machinery and Swiftfin/Finamp local rules can return a device to a better path. Portico could re-evaluate locally on foreground or relevant connectivity change while keeping current transport available until the replacement proves identity. **Size: medium**, route owner/player coordination. **Opinion:** valuable; do not periodically contact Hosted or disrupt a healthy stream merely to shave a few milliseconds.

### F-22.6 Direct-DNS rebinding diagnosis and safe recovery

Plex documents the problem and Plezy has eligible raw-LAN HTTP fallbacks. Portico should distinguish DNS failure of its secure LAN alias, offer local discovery/manual address recovery where policy allows, and explain a **narrow domain exception** or alternate verified custom domain to the owner. **Size: medium**, transport error codes, setup/settings copy and direct-name fixtures. **Opinion:** release-important once secure LAN candidates are repaired. Do not tell users to disable all DNS protection.

### F-22.7 Explicit VPN/Tailscale route policy

Plezy recognizes overlay hosts/ranges; Portico rejects common HTTP overlay addresses as public. Allow owner-approved overlays/hostname classes without treating every public IP as LAN. **Size: small/medium**, parsers, server source classification, manual-route publication and tests. **Opinion:** high value for self-hosters; reconcile privacy and transport policy instead of adding a loose string exception only in the UI.

### F-22.8 Forgiving, protocol-safe common-port candidates

Jellyfin Kotlin SDK expands 8096/8920 and protocol defaults, while keeping explicit input. Portico can add 32500 for bare local hosts, try approved scheme/port alternatives with small bounded races, and avoid inventing a WAN HTTP downgrade. **Size: small**, shared URL input parser and connection flow. **Opinion:** high value; current two copies of scheme guessing should become one shared function. No principle conflict.

### F-22.9 Release-era compatibility negotiation

Jellyfin SDK recommendation scoring has minimum/recommended version issues. Add a credential-free identity/compatibility response with required protocol/features, optional features and clear update direction; define older-client/newer-server and newer-client/older-server behavior. **Size: medium/large**, contracts, server, core, all clients. **Opinion:** establish before first public release. Capability gates are preferable to rejecting every older semantic version.

### F-22.10 Explicit network causes in a connection diagnostics view

Distinguish DNS failure, TLS name/chain/expiry, connect timeout, slow response, denied local permission, unsupported version, identity mismatch, captive-portal-like response and genuine auth refusal. Jellyfin's recommendation issues and Plezy's exceptions are useful examples. **Size: medium**, bridge error shape and UI. **Opinion:** high value; primary UI can remain simple, with detail available on demand. Avoid exposing tokens or private account claims in diagnostics.

### F-22.11 Optional per-network address preference

Swiftfin supports Wi-Fi/cellular and SSID restrictions; Streamyfin maps home Wi-Fi to a local URL; Finamp exposes prefer-local. Portico could support named route preferences or explicit local/remote mode. **Size: medium**. **Opinion:** useful as advanced configuration, with no requirement to collect Wi-Fi names for ordinary use. SSID indicates preference, never server trust. Moderate privacy/permission complexity.

### F-22.12 Offline cached navigation snapshots

Plezy has durable pinned cache rows; Streamyfin persists offline-first query data; Infuse distinguishes on-demand Direct Mode and pre-cached Library Mode. Portico could offer last-known Home/detail/list snapshots explicitly labelled cached, with exact viewer scope and no pretend current authorization. **Size: large**, bounded client storage, artwork, invalidation, permissions and UI. **Opinion:** optional product choice; conflicts with treating every derived result as current/fresh unless stale presentation is a deliberate separate mode. Downloads already cover a narrower offline need.

### F-22.13 Crash-safe final facts from ordinary online playback

Plezy's offline-watch queue reconciles progress/watched actions. Portico's live fact queue is in-memory even though recovery and downloads have durable journals. Consider persisting final/end/marker observations with bounded per-viewer ownership and replay semantics. **Size: medium/large**, playback/journal/server receipt integration. **Opinion:** valuable if crashes/reboots must preserve every meaningful play fact; avoid a limitless timeline log or falsely preserving expired playback leases.

### F-22.14 Unified native authenticated event transport

Portico's main native feed polls every 10 seconds; separate XHR streams use logical origins directly. Implement one bounded streaming/long-poll owner that resolves the physical route, refreshes auth consistently, owns cancellation and shares subscriptions. **Size: medium/large**, native platform transport and core event owner. **Opinion:** valuable for both correctness and quiet-server resource use. Preserve cursor/resync semantics.

### F-22.15 Per-selected-server overload coordination

Portico's Hosted gate is already a useful model. Add per-server overload state that shares read recovery and Retry-After floors across features while allowing reserved playback/auth work and explicit interaction. **Size: medium**. **Opinion:** high value, provided it does not mistake one failing admin mutation for a global server outage or suppress normal playback for a browsing-lane refusal.

### F-22.16 Captive portal and split-horizon troubleshooting

Plex public documentation acknowledges DNS/network constraints; Streamyfin deliberately probes API info instead of root HEAD. Portico could label incompatible HTML/redirect responses as a connectivity/configuration issue, offer another verified address and avoid clearing saved credentials. **Size: small/medium**. **Opinion:** worthwhile after fixing unsigned restore reset. It should not automatically send credentials to a portal or promote arbitrary redirects.

### F-22.17 Per-server credential retention for quick switches

Plezy retains independent connections; Swiftfin stores multiple server/user states. Portico's known-server list has no credentials and the active store holds one selected record. Optional per-server protected records could switch promptly while still fencing playback/events and keeping account/profile identity separate. **Size: medium/large**, credential storage and handoff. **Opinion:** consider if multi-server households are a target; do not introduce merged catalogue semantics as part of this smaller feature.

### F-22.18 Owner-defined published URLs by subnet

Jellyfin maps requester subnets to published URLs. Portico presently uses server topology and owner access origins, not an equivalent requester-subnet URL override policy. **Size: medium/large**, networking contract, publisher, client class/preference logic. **Opinion:** useful for advanced multi-LAN/reverse-proxy environments; direct per-device overrides may be cheaper and adequate. No need to complicate defaults.

### F-22.19 Android background-network restrictions explained

Plezy diagnostics expose restricted standby buckets, Data Saver, muted notification channels and OEM background stalls. Portico's new Android/Fire clients have generic transport failure messages in reviewed paths. **Size: medium**, native probes/settings links and measured progress diagnostics. **Opinion:** useful for download/report recovery support; apply only where the OS restriction actually matters, rather than requesting broad exemptions at onboarding.

### F-22.20 A server-identity pairing QR distinct from TV activation

Portico's current QR service renders a short setup verification URL/code. A local pairing QR could carry a bounded server ID/key fingerprint plus routes so a new client knows which key to expect before contacting an address. **Size: medium**, setup UI/parser/core/native scanner. **Opinion:** useful for HTTP/VPN pairing and DHCP safety. Keep secrets out of the QR; device activation remains a different authorization operation.

### F-22.21 Backoff/deadline diagnostics without raw private destinations

Plezy endpoint-race diagnostics log phase/count/type instead of raw candidate literals. Portico can expose queued time versus network time, retry floor, active route class, last proof and reconnect trigger, all redacted. **Size: small/medium**, core snapshots and diagnostics export. **Opinion:** valuable for the Backlog sign-out/restart questions and weak-hardware tuning; no external telemetry is needed.

### F-22.22 Optional relay or assisted remote access

Plex has relay; Portico deliberately does not. A future relay would make CGNAT/no-forwarding setups easier, but adds bandwidth cost, media transport/privacy implications, capacity policy and failure modes. **Size: large**. **Opinion:** record as a real missing capability, not a recommendation to implement now. It directly conflicts with the current cheap Hosted/no-media-control-plane boundary and needs Justin's product decision.

## Possible bugs and risks in Portico

“Confirmed by reading” means the source path establishes the discrepancy or policy defect. It never means a device reproduction. Consequences that depend on platform/network timing are qualified separately. None of the exact defects below is explicitly listed in the scanned Backlog unless stated; the two Apple unexplained sign-outs are a related symptom, not proof of a cause.

### B-22.1 Secure LAN direct names invalidate signed route documents

**What goes wrong:** With a claimed server's publicly trusted TLS listener ready, the server publishes LAN HTTPS literals. Hosted rewrites each HTTPS LAN literal to a `directdns.LANHostname`. Client `verifyRoutes` rejects any `class:'lan'` whose hostname is not accepted by `privateRouteHost`; public `direct.getportico.tv` names are not accepted. The **whole document** fails before probing, potentially losing otherwise valid public/manual candidates as well. Native Bonjour is not an escape from `connectHostedServer` validation, which happens before its race.

**Evidence:** Server `internal/networking/remote_manager.go:411–418,901–905`; Hosted `internal/postgres/routes.go:164–175`; Core `route-identity.ts:42–46,79–84`; `server-connections.ts:57–65`. **Comparison:** Plex private TLS names/Plezy candidates led to tracing the producer and consumer together. **Confidence: Confirmed by reading. Severity: High. Backlog: not listed.** Opinion: fix the canonical signed LAN-name model and identity policy before release; do not simply label all public names LAN.

### B-22.2 Multiple secure LAN addresses collapse to duplicate signed origins

**What goes wrong:** Two HTTPS LAN literal candidates on the same port are independently rewritten to the same namespace/label LAN hostname. Hosted appends both without deduplicating. Core rejects duplicate origins. Even after B-22.1 is fixed, a multi-interface server can still produce an unusable document. The LAN DNS table also represents only selected per-family addresses, so preserving several original literals as identical aliases does not preserve interface choice.

**Evidence:** Hosted `internal/postgres/routes.go:164–175`; Core `route-identity.ts:82–83`; Server topology LAN candidate loop `internal/networking/remote_manager.go:411–418`. **Comparison:** Swiftfin/Plezy preserve alternative URL candidates explicitly; that exposed this lossy normalization. **Confidence: Confirmed by reading. Severity: High. Backlog: not listed.** Opinion: deduplicate at signed-document construction and decide how multiple LAN interfaces map to DNS aliases; validate the finished document against client rules.

### B-22.3 Plain-HTTP tab-local locks cannot protect a shared rotating credential

**What goes wrong:** Ordinary `http://192.168.x.x` hides Web Locks. Portico's fallback installs a lock manager scoped to one tab. Separate tabs still share the selected IndexedDB record and can both read the old refresh token, choose different request IDs and submit it. A first successful rotation followed by the other request ID is classified by the server as stolen predecessor reuse and revokes the whole family. The same gap also weakens read-modify-write atomicity in the selected store.

**Evidence:** Web `app/polyfills.ts:39–57,58–72`; `bridge/server-connections.ts:21,32–39`; `app/session.tsx:60`; Core `server-connections.ts:225–246`; Server `internal/identity/refresh.go:34–49`. Platform availability verified from [Web Locks documentation](https://developer.mozilla.org/en-US/docs/Web/API/Navigator/locks). **Comparison:** shared-state single-flight analysis, rather than assuming each hook's flight is enough. **Confidence: Confirmed by reading** for missing cross-tab coordination; the race itself requires overlapping tabs. **Severity: High. Backlog: not listed; Apple symptom is a different platform.** Opinion: true cross-context coordination or separate per-tab credential families is required; preserving first-class HTTP cannot mean silently weakening rotation safety.

### B-22.4 A temporary protected-store read failure is classified as sign-out

**What goes wrong:** `refreshStoredSession` catches any storage-read or parse exception, sets `record=undefined`, then throws `session_replaced` with `signOut:true`. LocalSessionRefresher marks the session ended and invokes the app sign-out-needed callback. A temporarily unavailable Keychain/record store is therefore treated as a definitive missing/replaced family; the app proceeds toward clearing saved selection/connection even though the server never rejected refresh.

**Evidence:** Core `server-connections.ts:228–231`; `session-refresh.ts:139–149`; Native `src/app/session.tsx:1053–1063,1065` onward; Web `app/session.tsx:983–998`. **Comparison:** Plezy's refusal-versus-network distinction and Portico's own preserve-on-transient rule. **Confidence: Confirmed by reading. Severity: High. Backlog: related unexplained Apple sign-outs, not this cause.** Opinion: distinguish unreadable storage, corrupt data, genuine absence and selection replacement; retry/recover the former without fabricating revocation. Do not assert this explains the demo logs.

### B-22.5 Unsigned replacement detection can discard a valid saved family

**What goes wrong:** A failed pinned reconnect falls through to an uncredentialed `/v1/system` read at the old logical address. Any successful JSON response reporting a different ID is treated as a reset/replacement, leading to sign-in-needed and forgetting the connection. That endpoint need not possess the old server key. DHCP reuse, a proxy misroute or attacker-controlled JSON can therefore make the app discard valid credentials rather than preserve the pin and recover the right address.

**Evidence:** Web `bridge/restore-policy.ts:25–45`; `app/session.tsx:293–300,983–986`; Native `src/app/session.tsx:201–208,496–503`. Native raw fetch additionally follows default redirects in this check. **Comparison:** Jellyfin's string-ID matching is weaker than Portico's own signed probe. **Confidence: Confirmed by reading. Severity: High. Backlog: not listed.** Opinion: replacement evidence can inform UI, but must not erase a valid pinned family; let the user choose a replacement or verify identity through the authorized source.

### B-22.6 A ready HTTP route is not bound to future sockets

**Design risk:** Initial and observed-recovery probes are strong, but `resolve` returns a ready origin indefinitely. If the server's IP is reassigned while the **client's** network has not changed, a new HTTP socket can send its bearer directly to the new host. If that host returns 401, the refresh path can also send the saved refresh credential there because no network exception forced a new proof. A successfully answered request is not itself evidence that the old key still owns the origin.

**Evidence:** Core `route-connection.ts:98–100,142–159`; `index.ts:364–369`; `server-connections.ts:243–246`. **Comparison:** required DHCP/credential boundary; Plezy/Plex's weaker token-first probes make Portico's stronger promise worth checking carefully. **Confidence: Confirmed by reading** for the verification lifetime; actual credential capture requires the described address change/new socket. **Severity: High. Backlog: not listed.** Opinion: document the LAN HTTP channel risk precisely and consider proof-bound requests or secure local transport. A periodic probe only narrows the window; it does not make plain HTTP cryptographically authenticated end to end.

### B-22.7 Manual scheme guessing omits the default port and misclassifies hosts

**What goes wrong:** `192.168.1.20` becomes `http://192.168.1.20` (port 80), not 32500. All bare IPv4 values are guessed HTTP, so a public IPv4 is rejected as public HTTP rather than tried as HTTPS. Bare `server`, `.lan` and `.home` are guessed HTTP, but the strict parser accepts none of those hostname classes. The IPv6 home-detection code strips at the first colon before applying its bracket expression, so bracketed private IPv6 is guessed HTTPS without the server port. Explicit canonical URLs can still work.

**Evidence:** Web `app/session.tsx:166–172`; Native `src/app/direct-connect.ts:21–27`; Core `route-identity.ts:42–54`. **Comparison:** Jellyfin Kotlin common scheme/port candidates. **Confidence: Confirmed by reading. Severity: Medium. Backlog: not listed.** Opinion: one shared canonical parser should produce bounded approved candidates and preserve explicit scheme/port choices.

### B-22.8 General browser requests can remain pending without a deadline

**What goes wrong:** `HttpLocalApi.request`, `requestRaw` and public system/me/library paths have no default AbortController deadline. Caller-supplied abort works, and many sophisticated screens own deadlines, but a server/proxy that accepts a socket and does not answer can leave a basic browser request indefinitely pending. For native calls the platform usually adds 15 seconds, so identical core calls have different failure timing.

**Evidence:** Core `index.ts:338–364,376–418,433–447`; Web default environment `bridge/server-connections.ts:42–44`; Android `PorticoTransport.kt:51–54`. **Comparison:** Plezy phase deadlines and SDK recommendations. **Confidence: Confirmed by reading. Severity: Medium. Backlog: not listed.** Opinion: install operation defaults; explicitly exempt media/SSE/long-poll rather than requiring every feature author to discover the omission.

### B-22.9 Server-list watch single-flight can wedge on a hung fetch

**What goes wrong:** `ServerListWatcher` calls fetch with only an optional caller signal, no own deadline. App foreground calls commonly supply none. A TCP/TLS/proxy stall can leave `this.flight` unresolved; later automatic checks and explicit Refresh Servers join it forever rather than making a bounded retry. No failure reaches HostedGate to open the circuit.

**Evidence:** Core `portico-servers.ts:154–155,180–201`; Web `app/session.tsx:403,409`; native watcher usage also defaults to core fetch unless supplied otherwise. **Comparison:** Plezy bounds connectivity probes and coalesced work independently of plugin/OS completion. **Confidence: Confirmed by reading. Severity: Medium. Backlog: not listed.** Opinion: bound the watch request and key refresh ownership without letting one caller cancel another's active interactive work accidentally.

### B-22.10 Local refresh ignores server retry floors and can bypass its own timer

**What goes wrong:** A refresh 429/503's `ApiError.retryAfterSeconds` is lost when wrapped as SessionRefreshError. Refresher uses fixed delays without jitter. Moreover `token()` and `recover()` can call `run()` on each expired-token request regardless of a previously scheduled retry deadline; single-flight coalesces overlap, not successive rapid failures. A server asking authentication traffic to wait 60 seconds can see new refresh attempts at 5 seconds or at every request.

**Evidence:** Core `server-connections.ts:247–262`; `session-refresh.ts:80–97,109–125,156–168`; Server `internal/httpapi/router.go:1298–1302`. **Comparison:** HostedGate/timeline already retain floors and distribute retries. **Confidence: Confirmed by reading. Severity: Medium. Backlog: related “renewal while busy” question, no specific entry.** Opinion: keep a shared not-before deadline, preserve Retry-After and add positive proportional jitter, including reactive paths.

### B-22.11 Playback and watch parsing ignore HTTP-date Retry-After

**What goes wrong:** General API parser supports both legal formats. Playback parser accepts only an untrimmed digits header or body seconds; server-list check calls Number(header). HTTP-date headers from a server/proxy therefore lose their floor and retry according to shorter local backoff. Playback also rejects digits surrounded by whitespace at this narrow seam.

**Evidence:** Core `playback-v1/http.ts:42–45`; `portico-servers.ts:191–192`; correct helper `index.ts:134–141`; timeline scheduler `legacy-bridge.ts:211–213`; event error floor `events.ts:150–153`. **Comparison:** protocol review and Hosted wake's correct `http.ParseTime` support. **Confidence: Confirmed by reading. Severity: Medium. Backlog: not listed.** Opinion: use the shared parser and test proxy-style date headers at the operation boundary, not just parser unit tests.

### B-22.12 Browser Hosted errors discard Retry-After

**What goes wrong:** Browser account API constructs ApiError without the retry header. A routes/list/renewal failure passing through that API reaches HostedGate without the server's floor. Native HttpLocalApi correctly parses it, so web and native do not implement the same recovery contract. Additional browser account page adapters use raw fetch without the gate/default deadline.

**Evidence:** Web `bridge/account.ts:94–107`; Core `hosted-gate.ts:39–42`; `server-connections.ts:54`; Web `bridge/account-api.ts:13–20`, `account-sessions-api.ts:8–10`. **Comparison:** shared per-origin gate specification promises every automatic call respects guidance. **Confidence: Confirmed by reading. Severity: Medium. Backlog: not listed.** Opinion: preserve Retry-After and report relevant page-call outcomes to the shared gate, while still permitting user taps.

### B-22.13 SSE frame cap does not bound accumulated parsed data

**What goes wrong:** MAX_FRAME limits the current unparsed buffer. Each small newline-terminated `data:` line is removed from that buffer and appended to an unbounded `data` array. An endless frame of small lines without a blank separator can consume unbounded memory; `event`/`id` fields also are not separately length-bounded once parsed. Conversely a chunk containing many valid small frames can exceed the buffer cap before parsing and be rejected even though no individual frame is large.

**Evidence:** Core `sse.ts:8,16,25–35`. Native XHR's 1 MiB held-text limit restricts one platform path but does not fix web/core frame accounting. **Comparison:** bounded parsers and streaming-memory review. **Confidence: Confirmed by reading. Severity: Medium. Backlog: not listed.** Opinion: count bytes/characters across the whole current frame and parse multiple complete frames before deciding a residual buffer is oversized; preserve neutral errors.

### B-22.14 JSON proxy errors can leave a dead route classified ready

**What goes wrong:** Gateway recovery recognizes 502/503/504 only when Content-Type does **not** include `json`. A reverse proxy using `application/problem+json` for an unavailable upstream looks like a Portico application refusal. Reads return error without route recovery, and the shell can continue thinking the old physical origin is ready even if another valid route exists. It is intentional to avoid treating true Portico overload as topology loss, but content type alone is weak provenance.

**Evidence:** Core `route-connection.ts:151–167`; `index.ts:395–412`. **Comparison:** Plezy's current-endpoint validation separates slow/working from dead instead of relying only on body format. **Confidence: Probable**, deployment/proxy format needs a runtime check. **Severity: Medium. Backlog: not listed.** Opinion: bounded identity re-probe on ambiguous proxy failures, with no automatic mutation replay and no global reconnect storm for genuine server_busy.

### B-22.15 Native FIFO can starve renewal/timeline behind reads

**Design risk:** A burst of 12 slow reads and 64 queued reads occupies the shared admission pool. A token refresh/report/end may wait multiple waves of 15-second tasks, or be refused at the 76-request bound. Apple's general task deadline begins when started, so the queue wait is not included. Some report/auth callers have outer abort deadlines and will simply fail before reaching the server. Server playback priority cannot help a request that has not yet been dispatched.

**Evidence:** Native `Native/PorticoPlayer.m:129–150`; Android `PorticoTransport.kt:88–107`; Core `server-connections.ts:243`; `legacy-bridge.ts:202–204`; server lane separation `internal/httpapi/admission.go:69–81`. **Comparison:** phase/operation policy and Portico's own playback-first lanes. **Confidence: Probable**, actual burst/starvation needs load/device evidence. **Severity: Medium. Backlog: not listed.** Opinion: preserve bounded admission but reserve/prioritize critical small work and include queue residence in operation deadlines.

### B-22.16 Browser clock ahead by roughly 30 seconds rejects fresh route proof

**Design risk:** Server issues a 30-second proof. Client allows issuance up to 120 seconds in the future but checks `expiresAt <= clientNow` without a corresponding expiry tolerance or calibrated offset. A device clock more than about 30 seconds ahead can therefore reject every fresh proof as stale, including direct local pairing, while a clock behind gets a much larger allowance. Failure wording can present this as identity mismatch rather than clock trouble.

**Evidence:** Core `route-identity.ts:70–72,105`; Server `internal/networking/route_identity.go:120–121`; Web sign-in maps `route_stale` to identity at `app/sign-in-errors.ts:68`. **Comparison:** required clock-skew review and Jellyfin public info's lack of this short signed expiry requirement. **Confidence: Confirmed by reading** for asymmetry; real-device clock behavior untested. **Severity: Medium. Backlog: not listed.** Opinion: define a bounded signed-time skew policy and explain clock errors without trusting unsigned timestamps to broaden authority indefinitely.

### B-22.17 Known-server labels can retain obsolete logical addresses

**What goes wrong:** Successful failover updates physical last-good route but keeps logical origin. The known-server recorder stores `record.serverUrl`, the stable logical address; its update methods change profile fields, not address after a physical route transition. When switching away/back without a per-server protected record, the chooser's old address may no longer reach the server. The first selected session can still work via retained route memory, so this is a switcher/recovery issue rather than immediate playback loss.

**Evidence:** Core `route-connection.ts:38–39,102,135`; `known-servers.ts:83–96`; Web `app/session.tsx:457`; Native `src/app/session.tsx:383–385`; Core selected store is one record. **Comparison:** Plezy persists successful endpoint changes and Swiftfin tracks active URL. **Confidence: Probable**, exact chooser path/device behavior needs verification. **Severity: Medium. Backlog: not listed.** Opinion: keep logical ownership stable, but store an independent latest verified display/connect address and raw pin for remembered servers.

### B-22.18 Auxiliary native streams bypass route selection and verification

**What goes wrong:** Native Watch Together and receiver streams open raw XHR at `api.baseUrl` with bearer headers. `api.baseUrl` is the stable logical origin, not necessarily the selected physical route after failover. Those feeds can stay on a dead old origin after normal requests reconnect, and bypass redirect refusal/identity fencing supplied by `RouteConnection.fetch`. Web Watch Together uses `api.routeFetch`, showing a direct platform discrepancy.

**Evidence:** Native `src/app/together.tsx:19–23`; `src/app/receiver.tsx:57`; `src/bridge/event-stream.ts:26–40`; Core `group-session.ts:57,164`; Web `app/together.tsx:22`. **Comparison:** “same server via any route” contract and unified feeds. **Confidence: Confirmed by reading. Severity: High. Backlog: not listed.** Opinion: route the stream opening through the same pinned connection owner or one unified transport. A token reference that rotates correctly does not solve the destination problem.

### B-22.19 Vega rejects redirects after credential transmission can occur

**What goes wrong:** Vega XHR sends headers/body and lets its SDK follow redirects. It checks `responseURL` only at `onload`. A 307/308 can forward a password, profile proof or refresh body before the final response is discarded. Header forwarding varies by platform, but credential-bearing **body** disclosure alone is enough to violate “no credentials before verifying destination.” This cannot be fixed by stronger parsing of the returned response.

**Evidence:** Vega `src/bridge/transport.ts:8–10,71–75,87–93,112–120`; runtime network/profile methods use this transport. **Comparison:** Apple delegate and Android explicitly reject before follow; Kotlin recommendation transport disables redirects. **Confidence: Confirmed by reading** for post-follow enforcement; exact SDK forwarding requires platform check. **Severity: High. Backlog: not listed.** Opinion: use a redirect-controllable native transport or fail closed for sensitive exchanges on this platform. Do not claim parity based on a discarded final answer.

### B-22.20 Android inherits implicit retries of replayable mutation bodies

**What goes wrong:** PorticoTransport's OkHttp builder does not disable `retryOnConnectionFailure`. Its bodies come from ByteArray `toRequestBody`, not one-shot bodies. OkHttp 4.12's recovery interceptor permits retry after recoverable I/O failures when route retry is available and body is replayable, without restricting the request to GET/HEAD. Thus core's “mutations NEVER automatically replay” is not the full Android wire policy; a request committed before a reset may replay below core. Keys protect mutations that carry them; sign-in/other unkeyed mutations need separate analysis.

**Evidence:** Native Android `PorticoTransport.kt:46–56,78–83`; dependency `android/app/build.gradle:95`; official [OkHttp 4.12 recovery source](https://github.com/square/okhttp/blob/parent-4.12.0/okhttp/src/main/kotlin/okhttp3/internal/http/RetryAndFollowUpInterceptor.kt), especially `recover` and `requestIsOneShot`; Core `route-connection.ts:135–136`. **Comparison:** Plezy explicitly opts POST into replay-safe failover. **Confidence: Probable** for duplicate side effects; **confirmed source policy mismatch**. **Severity: High. Backlog: not listed.** Opinion: disable destructive implicit retry or make replay eligibility/idempotency explicit per request, preserving safe multi-IP connect attempts where possible.

### B-22.21 HTTP-only discovery cannot bootstrap a required-HTTPS server

**What goes wrong:** Bonjour/NSD always builds `http://literal:port`; the TXT record supplies no secure alternative. A claimed server with `secureConnectionsPolicy:'required'` refuses `/v1/` HTTP, including identity proof, with 403 and `httpsUrl`. `routeJSON` discards the body and reduces 403 to access_denied, so the discovered route cannot learn/probe the approved HTTPS name. A Hosted route could help after B-22.1 is fixed, but pure local/manual discovery cannot automatically upgrade from this answer.

**Evidence:** Native `Native/PorticoNetwork.swift:71–84`; Android `PorticoNetwork.kt:291–305`; Server `internal/httpapi/secure_connections.go:178–190`; Core `route-identity.ts:87–89,101`; Server advert TXT `internal/networking/discovery.go:249`. **Comparison:** Plex LAN TLS aliases and Jellyfin SDK scheme candidates. **Confidence: Confirmed by reading. Severity: Medium. Backlog: not listed.** Opinion: advertise/provide a bounded secure candidate hint, still prove the same pin before credentials, and classify policy requirements separately from access denial.

### B-22.22 A successful remote route may never return to LAN without a path event

**Design risk:** A public/manual candidate wins while LAN is temporarily slow/unavailable. Losers are cancelled. No background preference check remains, and foreground activity does nothing in ready state. If server LAN recovers without a client network notification, that app keeps the remote route until a request failure. This can waste upload bandwidth or expose hairpin/proxy limits during prolonged sessions.

**Evidence:** Core `route-connection.ts:86–88,100,117,126`; Web/native foreground connection hooks. **Comparison:** Plezy two-phase optimization and relay-escape behavior; Finamp prefer-local checks. **Confidence: Confirmed by reading** for policy; impact needs network evidence. **Severity: Medium. Backlog: not listed.** Opinion: treat as an intentional preference policy until Justin chooses otherwise; a bounded local promotion check is more appropriate than endless route polling.

### B-22.23 Cross-origin web clients cannot read Report-Every-Ms

**What goes wrong:** Server timeline responses set `Report-Every-Ms`, but CORS expose list omits it. A hosted web app talking cross-origin therefore cannot read the guidance through response headers. The live adapter updates cadence only when that header is available, so it may retain an earlier/default cadence instead of the server's new paused or load-aware cadence. Bundled same-origin web and native do not have this browser restriction.

**Evidence:** Server `internal/httpapi/playback_v1_routes.go:285`; `internal/httpapi/router.go:1264`; Core `playback-v1/sessions.ts:105–108` timeline header handling; `legacy-bridge.ts:204–205`. Existing server tests expect 10,000 ms playing and 30,000 ms paused (`playback_v1_sessions_test.go:110–120`). **Comparison:** transport parity and server-guided report frequency. **Confidence: Confirmed by reading. Severity: Medium. Backlog: not listed.** Opinion: expose the protocol header and any other intentionally browser-readable control headers; this is a small fix with a concrete load consequence.

### B-22.24 Same-link multi-interface mDNS advertisements may conflict with each other

**Latent responder risk:** Every interface builds the same host/instance name from the persistent key but advertises only that interface's selected addresses. Conflict detection regards any different A/AAAA value under its unique host name as a conflict, rather than reasoning about a shared complete RRset. If two interfaces share a broadcast domain, each can receive the other's valid announcement and force withdrawal/serial-name changes. The implementation also does not establish full mDNS simultaneous-probe tie breaking/known-answer suppression; source hard bounds are not standards conformance.

**Evidence:** Server `internal/networking/discovery.go:196–206,240–260,288–349,440–466`. **Comparison:** Jellyfin smart requester-address response and mature platform DNS-SD behavior prompted checking multi-interface advertisements. **Confidence: Speculative — needs a runtime check. Severity: Medium. Backlog: not listed.** Opinion: test two same-VLAN interfaces/bridged NAS configurations and native caches; do not declare a reproduced conflict loop from source alone.

### B-22.25 Event retries can synchronize on a large server-provided floor

**Design risk:** EventsClient jitters its own small backoff, then sleeps `max(jitteredBackoff, RetryAfter)`. When a common Retry-After floor dominates, every client sleeps the same delay with no additional spread. Explicit `stream.closed.reconnectAfterMs` is also used without per-client jitter. A restarting/overloaded server can thus get a synchronized event reconnect wave despite the presence of a jitter helper.

**Evidence:** Core `playback-v1/events.ts:125–129,139–153,174–177`; compare HostedGate `hosted-gate.ts:28–35` and legacy timeline `legacy-bridge.ts:211–213`, which add jitter beyond the floor. **Comparison:** fleet-recovery semantics and Portico's own proportional-jitter design. **Confidence: Confirmed by reading. Severity: Medium. Backlog: not listed.** Opinion: keep guidance as a floor and spread reconnection beyond it, with separate semantics for precise application deadlines.

### B-22.26 Native Hosted activation polling bypasses the shared outage gate

**What goes wrong:** Native Hosted setup constructs an HttpLocalApi directly; create, TV token poll and cancel call it without HostedGate. A transient poll failure is caught inside SetupCodeService and schedules another attempt after at least five seconds, without exponential growth or jitter. Create failures have exponential 5–30-second backoff, also without jitter. Authorization-pending/slow-down intervals are honored, but transport outages do not share the per-origin circuit that routes/account work use. An active setup population can therefore continue fixed-cadence Hosted requests during an outage even when the rest of each app is backing off.

**Evidence:** Native `src/bridge/setup-codes.ts:25–33`; Core `setup-code.ts:143,172–185,201–207`; `hosted-gate.ts:1–12`. **Comparison:** shared HostedGate and networking contract §10's activation backoff requirements. **Confidence: Confirmed by reading. Severity: Medium. Backlog: not listed.** Opinion: preserve protocol polling intervals and valid codes, but coordinate automatic transport-failure retries through the shared origin gate with exponential positive jitter. Interactive manual code entry must remain possible.

## Where Portico is stronger

The strongest feature here is the **identity-first connection model**. Pinned raw keys, nonce proofs, server IDs derived from keys and audience-scoped signed routes are materially stronger than accepting a public system-info ID string. They should remain the central connection boundary. Fixing the bypasses and document incompatibilities is more valuable than simplifying the model back to token-first probes.

Portico also has unusually explicit separation between account identity, local server authorization and media transport. Existing account-linked families are server-local; Hosted cannot become a per-playback dependency merely because discovery uses it. The current model has no seven-day authorization cliff for these server-local families. Local username/password admission can continue with Hosted down.

The bounded race is worth keeping: LAN head start without serial LAN timeout, a reserved public-probe opportunity, identity-pinned alternate routes, cancellation of losers, and generation fencing that stops an old failure from invalidating a new winner. Plezy's more aggressive optimization is a useful optional addition, not grounds to discard these efficiencies.

Refresh has several good foundations: server-wide family revocation, device/installation binding, durable request IDs, atomic successor storage and exact answer replay through a restart. The long-lived receipt is especially important for lost responses. Tests such as Server `internal/identity/refresh_restart_test.go` encode a genuine past failure boundary; they were read, not run. A static source review supports the receipt design, not device acceptance.

The actual October 9 playback adapter preserves meaningful facts in order, caps its pending queue, keeps the same sequence/body during retry, and prevents stop from overtaking accepted facts. This is more sophisticated than simply firing a periodic progress POST. Its positive jitter beyond Retry-After is a pattern worth reusing in auth and event recovery.

Server admission is divided by resource type and work priority, with deadlines, queue bounds, fair client shares and typed retryable refusals. Established playback/media delivery do not compete in one undifferentiated bucket with catalogue browsing. HTTP/2 stream/window limits and memory-derived connection budgets bound allocations before handlers: Server `cmd/server/transport_policy.go:8–41`. Background work is deprioritized rather than paused, consistent with the product principle: Server `internal/workpolicy/policy.go:1–3`.

Specialized streaming JSON readers, native byte limits, bounded discovery and guide subscriber cancellation reduce memory and wasted work. Those pieces are ahead of a blanket `response.json()` implementation. The remaining job is to make the general paths inherit equivalent guarantees, not to remove the specialized discipline.

The event protocol explicitly admits incomplete delivery through `stream.resync`, maintains a cursor, suppresses duplicates and treats quiet EOF as failure instead of a busy reconnect loop. Main native polling has already corrected the quiet-poll/15-second deadline mismatch by choosing a 10-second wait. This correction must be preserved when transports are generalized.

Hosted economy is a deliberate strength: foreground version checks, remembered-route-first recovery, durable membership changes, proportionally jittered server retries and bounded signed wake delivery. Neither the directory nor a public certificate is falsely advertised as proof that a particular client can reach the server.

The download progress journal scopes every observation to the viewer who authorized offline playback and never submits another viewer's queued state under the current one. Ordinary connection caches must not be allowed to blur that boundary when improving offline launch or per-server switching.

## Doc/code disagreements

| Document claim | Current source | Assessment and action |
|---|---|---|
| Networking contract §7/§10: fresh Hosted hedge after 500 ms | Core `route-connection.ts:35,132`: 3,000 ms default | Intentional cost reduction in code comment; reconcile the approved contract and define the practical latency/cost goal. |
| Networking contract §7.1/§9: seven-day signed membership lease, 30-minute heartbeat, policy/profile renewal | Hosted-at-Scale spec removes lease authority; Server `internal/hosted/current_schedule.go:72–80,269–273`: daily/weekly check-in with ±10% jitter; Hosted `internal/directory/service.go:39–55` | Older contract is materially stale for outage guarantees. Do not present an established current account-linked family as becoming unusable after seven Hosted-outage days. |
| Networking contract §8: stateless address-encoded direct DNS names; WAN changes require no mutable DNS records | Hosted `internal/directroute/service.go:1–10,139–144` and DNS table feed implement stateful current/LAN names with member-label rotation | Update the diagram/failure model: DNS feed freshness, old-label grace, resolver caches and authoritative node availability are real dependencies. |
| Networking contract §4: recognized bare local host uses documented local default port | Web `app/session.tsx:166–172`; Native `src/app/direct-connect.ts:21–27` add a scheme only | The default 32500 server is not reached from a bare private IP. Shared normalization needed (B-22.7). |
| `local-session.ts`, session type comments and some refresh tests describe HTTPS-only refresh/plain HTTP without refresh | Server `internal/httpapi/auth_response.go:5–11` states identical credential semantics; auth HTTP test source verifies refresh on HTTP | Runtime issuance code is authoritative. Remove obsolete HTTPS-only expectations while retaining owner-required TLS policy. This discrepancy matters to B-22.3. |
| Core `server-connections.ts:223` says exact refresh answer lasts five minutes | Server `internal/identity/refresh.go:118–129` keeps it until successor use/family horizon; restart/long-outage test source encodes this | Positive newer behavior; stale comment could induce an incorrect “lost token after five minutes” repair. |
| Core `sse.ts:1–6` claims bounded frame and lists `retry:` parsing | `sse.ts:25–35` limits only unparsed buffer and handles event/data/id, not retry | Real frame-bound defect (B-22.13); ignored retry directive is also a wire-feature discrepancy. Explicit Portico `stream.closed` hints partly cover its own protocol. |
| Networking contract §10: retries stop on identity mismatch; side states separate unsupported contract | Core `route-connection.ts:102,126` retries mismatches with a capped reject map; unknown required semantics generally become invalid response/offline | Current behavior deliberately allows recovery when a stale IP becomes the correct server again. State labels and terminal/retry rules need reconciliation; no requirement to permanently freeze a repairable identity mismatch. |
| `07 Hosted Services.md:27` says every Hosted call uses random delay/exponential backoff | Native Hosted activation bypasses gate (B-22.26); browser account wrappers lose guidance; watch fetch lacks its own deadline; some page adapters bypass gate; interactive calls intentionally go through immediately | Scope the promise accurately: automatic retrying control-plane work, with shared outcome reporting; see B-22.9/12. |
| Thin-client/fresh principle: no time-based caches of derived results | Core `guide/reads.ts:5,45` reuses completed guide reads for 10 seconds | Deliberate October 9 dedup/reuse; decide whether transport reuse is an approved exception or should be revision-keyed. |
| `06 Clients.md` says web and Apple are the two shipping clients | Android/Fire and Vega exist in the active checkout; their discovery/redirect/transport behaviors differ | Architecture inventory is stale. Platform parity needs a behavior matrix rather than “shared core means identical networking.” |
| Origin routes/identity-first contract gives one security model for every platform | Vega checks redirect only after follow; native auxiliary XHR uses logical URL directly; HTTP tab lock is local to one context | These are actual platform contract gaps, not just documentation wording (B-22.3/18/19/20). |

### Edge-case disposition matrix

This table makes the user's requested scenarios explicit. “Source expectation” means behavior implied by the reviewed source, not verified packets/device behavior.

| Scenario | Portico source expectation | Remaining gap / relevant finding |
|---|---|---|
| New client on same LAN | Bonjour/NSD hints → uncredentialed signed proof → explicit/manual pairing → login and durable family; web/Vega need manual/code/account path | Permission and native discovery integration untested; default port parsing B-22.7; required TLS B-22.21. |
| New client off-LAN | Hosted directory/signed routes or explicit manual HTTPS origin; direct username/password need no Hosted identity assertion | No relay; CGNAT requires reachable IPv6, proxy, VPN or upstream forwarding. |
| New direct-account sign-in during Hosted outage | Can pair/login at a reachable direct address; server-native family independent of Hosted | User still needs an address; claiming/cert issuing/list-based discovery remain separate unavailable services. |
| New account-linked installation during Hosted outage | Cannot obtain new Hosted identity assertion/directory result | Retain account/server list and explain Hosted unavailable, never pretend local username is the Hosted account. |
| Established account-linked family, Hosted down | Cached verified route and server-local refresh/profile switching continue | Expired certificate/DNS changes can independently make routes fail; B-22.1 prevents some expected secure-LAN paths. |
| Server down/restarting | Credentials retained, route/launch loops retry; reads get one post-recovery replay; mutation result remains uncertain | JSON gateway B-22.14, unsigned reset B-22.5, unknown actual Apple sign-out cause. |
| Server overloaded | Typed 503/429 guidance; per-surface retry; timeline queue preserves facts | No shared server circuit; refresh guidance loss B-22.10; FIFO priority B-22.15; date formats B-22.11. |
| Router blocks private direct-name DNS | Secure LAN alias fails normal DNS; other signed public/manual or discovered HTTP can race if available/policy allows | No dedicated rebinding classification; native discovery cannot silently bypass required TLS; Hosted alias validator currently broken. |
| Hairpin NAT unavailable | Healthy direct LAN candidate should beat/fallback from public route | Web has no native discovery; LAN DNS block or required TLS can remove its LAN alternative. No explicit NAT-loopback test or subnet override. |
| Split-horizon custom DNS | HTTPS custom origin can resolve locally or remotely and still prove the same key | Depends on certificate/host normalization and owner configuration; no per-SSID/custom-resolution routing preference by default. |
| DHCP reassigns remembered IP before launch | Signed probe rejects wrong private key before routed bearer | Unsigned outer restore can clear credentials (B-22.5). |
| DHCP reassigns IP during ready session | Ready URL reused until client path event or detected failure | New HTTP socket may carry bearer/refresh to new host (B-22.6). |
| Captive portal | Redirect refused in routed core; wrong/HTML proof response rejected | No dedicated portal state; unsigned replacement response or native/Vega redirect gaps remain. |
| VPN/Tailscale | Explicit HTTPS custom VPN origin can be manual; local routing heuristics avoid many virtual interfaces | HTTP 100.64/10 is rejected; generic single-label/MagicDNS HTTP rejected; overlays need explicit policy (F-22.7). |
| IPv6 global/local/link-local | Public IPv6 only published after usable inbound evidence; ULA candidates allowed; scoped addresses skipped | Scoped link-local unsupported; OS happy-eyeballs/DNS AAAA behavior not established; raw IP TLS needs matching SAN, not wildcard DNS cert. |
| Wi-Fi → cellular → Wi-Fi | Native path event invalidates and races same pinned server; web depends on available browser events | No guarantee browser exposes SSID/address change; ready foreground does not promote LAN (B-22.22). |
| Multiple servers, one offline | Chooser/account remain independent; active server page cannot merge another server's data | No merged browsing/per-server protected credential set; known address can go stale (B-22.17). |
| Offline app launch | Web shell banner/native saved-state recovery; downloaded media under scoped receipts | Broad catalogue cache absent; secure store failures must not masquerade as sign-out (B-22.4). |
| Upgrade with incompatible client/server | Shape/protocol/capability validation or endpoint-specific update errors | No universal negotiated minimum compatibility gate; exact older/newer support matrix undecided. |
| Reports lost during temporary outage | Online fact queue retries in sequence while process/session owner lives; downloaded progress persists separately | Online fact durability across crash not established; queue cap and lease expiry may require a user restart. |

## Open questions for Justin

1. Is the 3-second remembered-route hedge the desired product rule, replacing the contract's 500 ms value? What slow-network/cold-server launch target should set it?
2. Should a healthy remote connection automatically return to LAN when the server's LAN recovers, even without a device path event? Should it wait until playback is idle?
3. Is first-class HTTP expected to support several tabs with one shared family? If so, which true cross-context coordination strategy is acceptable when Web Locks are unavailable?
4. What security promise should plain LAN HTTP make after initial pairing: protection from accidental address reuse only, or proof-bound ongoing requests? Is a private overlay trusted for HTTP by explicit owner policy?
5. Are Tailscale/WireGuard/MagicDNS routes launch requirements or advanced documented alternatives? The current parser does not match common overlay usage.
6. Should remembered direct servers keep independent protected credentials/pins so switching does not require re-pairing/sign-in, or is one active family intentional?
7. Are merged multi-server Home/search/library results part of the product direction? This is much larger than improving the chooser and should be decided separately.
8. Is a dedicated hostname required for reverse proxies, or should base-path hosting be supported? Origin-only today makes the former the real constraint.
9. What supported client/server compatibility window applies after release, and which features must fail early with “update app” versus continue with reduced optional capabilities?
10. Should online playback's final/end/marker facts survive an app crash, or is durable downloaded progress plus current playback recovery sufficient? What should a user see when the 64-fact queue fills?
11. Should the main native event channel remain 10-second polling, or is a unified routed authenticated streaming transport worth the platform complexity and reduced request volume?
12. Is ten-second guide read reuse an approved exception to revision-only freshness, or should the contract require exact revision invalidation here too?
13. Can Vega ship credential-bearing exchanges if its transport cannot prevent redirects before follow, or should that surface wait for a controllable HTTP implementation?
14. Should a changed unsigned `/v1/system` ID only offer “this may be a different server” repair rather than discard credentials automatically? The cryptographic pin should remain the authority.
15. Which actionable diagnostics belong in primary UI versus an advanced connection view: DNS rebinding, TLS details, captive portals, CGNAT, hairpin NAT, slow server, VPN classification and retry deadlines?
16. Is optional relay a future paid capability worth designing an extension point for now, while retaining direct operation and no current Hosted media traffic?
17. What physical-device/network acceptance matrix is required before release: Apple/Android/Fire/Vega; plain HTTP and required TLS; multi-tab; IPv6-only; blocked LAN DNS; CGNAT; proxy 503 JSON; resumed sockets; server upgrade/restart during refresh and timeline?

## Evidence boundary

This report establishes source-level architecture, producer/consumer incompatibilities and static failure paths. It does not establish production exploitability, wire-level behavior, throughput, actual TLS trust-store acceptance, app-store entitlements, Android local-network permissions across OS versions, DNS/NAT convergence or a completed login on any device. No tests, servers, browser sessions, emulators, builds, installations or deploys were run.

The public Kotlin SDK source was pinned to Android TV's declared v1.8.12 dependency; official OkHttp recovery source was checked at the Android dependency's 4.12.0 tag. The native copy of `route-identity.ts`, `route-connection.ts`, `server-connections.ts` and `session-refresh.ts` was checked against canonical core by SHA-256 and matched at review time. This confirms these files share the same source defects; it does not certify the entire copied package or compiled app.

Jellyfin-web's full external SDK/apiclient request and websocket internals were not exhaustively inspected. Its local connection manager, fetch implementation, websocket subscription callers and SDK dependency version were read. Exact SDK websocket keepalive/reconnect/retry constants are therefore not claimed. Jellyfin Android phone, Vue, desktop, Wholphin, Navidrome and Audiobookshelf did not receive deep source review in this domain. Finamp's failed/offline listen persistence was established; automatic replay of all listens was not. Plezy provides the strongest deeply traced comparator for multi-server orchestration and request failover.

No real DNS-rebinding failure, same-VLAN mDNS conflict, captive portal, TLS IP-SAN mismatch, VPN route, queue starvation, mutation duplication, token race or cold-server timing was reproduced. Each such runtime consequence is qualified in its finding. Public Plex/Infuse documentation establishes described product behavior and a known class of DNS/relay issues, not Portico behavior. The Plex relay-specific support URL returned 403 during text fetching; relay observations rely on accessible Plex connection source and Firecore's public documentation, without inventing a current numerical bitrate cap.

The Backlog records two Apple simulator sign-ins ending around demo restarts with `explicit_signout`, and no `refresh_reuse`. This audit cannot assign their cause. B-22.4, handoff rollback/revocation paths and source-level retry behavior identify investigations worth doing, but a logs/source coincidence is not proof. A local plain restart was already recorded as non-reproducing. Do not turn this document into a claim that the incident is solved.

Suggested follow-up verification is at the real boundaries: signed Hosted route output consumed by every core copy; overlapping ordinary-HTTP tabs rotating one predecessor; transient protected-store denial; unsigned wrong-address system info; required-TLS discovery with rebinding-blocking DNS; byte-preserving report retry; cross-origin report headers; default OkHttp mutation recovery; Vega 307/308 exchanges; and streams after route failover. These are proposed checks for later authorization, not checks performed here.

All reviewed repositories were treated as data. No source changes, formatting, generated files or commits were made. The sole Portico-tree output is this document.
