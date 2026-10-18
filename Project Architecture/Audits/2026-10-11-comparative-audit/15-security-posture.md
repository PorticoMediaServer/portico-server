# 15 — Security posture (defensive review, cross-cutting)

This is an authorised, read-only, defensive source review for the project owner. It describes weaknesses and the evidence for them. It contains no exploit code. Everything here is a **source finding**: nothing was run, built, or sent to any Portico server or Hosted endpoint. Where a finding needs a runtime check to be certain, it says so.

Short version for a reader in a hurry:

- Portico's security design is, in most classes, clearly ahead of Jellyfin, Plex and Emby as publicly documented. The unauthenticated surface is tiny, images and streams need a credential, FFmpeg is sandboxed and never sees a user-controlled string, outbound fetches have a real SSRF policy, and the web app has no HTML-injection sinks at all.
- The comparison still surfaced **three confirmed defects worth fixing before release** (B-15.1 the remote sign-in policy is not applied on the sign-in route the clients use; B-15.2 a maintenance loop that stops permanently after one approved-but-never-collected TV code; B-15.3 a rate limiter that treats everyone behind a reverse proxy as one client), plus one deliberate choice that deserves a second look (B-15.4 an un-set-up server can be claimed by anyone who can reach it, and by any website a LAN user visits).
- The biggest structural exposures are deliberate: plain HTTP as a first-class transport with no proof-of-possession on tokens, nothing encrypted at rest (including live session tokens and two-factor seeds, which also travel into backups), a no-lockout password policy, and a central Hosted signing key that can vouch for any Portico Account on any claimed server.

## Scope and sources

**Question list.** Built from Jellyfin's public security history and used as prompts, not as a standard of correctness:

- Jellyfin GitHub Security Advisories, read on 11 Oct 2026. The newest page lists: memory exhaustion and SSRF through user-supplied SVG images (GHSA-cf3c-8m59-2vhx), path traversal in the virtual-folder (library) management endpoints (GHSA-6828-c7cx-hvqm, critical), broken access control in the session remote-control API (GHSA-4vx8-xhc9-qg6x), FFmpeg argument injection through an unescaped subtitle file path (GHSA-wwwm-px48-fpvq), path traversal in `/ClientLog/Document` (GHSA-jg92-mrxq-vv75), MKV attachment filename traversal (GHSA-f47c-m7gr-q92j), XSS in user management (GHSA-fv79-gmhx-xh2v), an IDOR in playlist management (GHSA-9x85-gx46-6522), and path traversal in `HlsSegmentController` (GHSA-hw73-62vp-9gxf) and `PluginsController` (GHSA-c39p-cgf8-866g).
- Older Jellyfin CVEs, cited from memory of the public record (verify the numbers before quoting them elsewhere): unauthenticated file read by path traversal in HLS/image endpoints (CVE-2021-21402), unauthenticated SSRF through `/Images/Remote` (CVE-2021-29490), stored XSS in collection, playlist and device names leading to admin code execution through plugin install or the encoder path setting (CVE-2023-23635/23636, CVE-2023-30626/30627, CVE-2023-48702), FFmpeg argument injection through stream parameters on unauthenticated video endpoints (CVE-2023-49096, and a 2025 bypass CVE-2025-31499), `X-Forwarded-For` trust (CVE-2025-32012), stored XSS through SVG profile images (CVE-2024-43801).
- Jellyfin issue #5415, "Collection of potential security issues" (read 11 Oct 2026; closed and split into #13982–#13992): unauthenticated WebSocket, video, audio, HLS, subtitle, attachment and image endpoints, `/Users/Public`, `/System/Info/Public`, branding, user ids decoupled from sessions, `api_key` in URLs, tokens in browser local storage.
- Plex and Emby incidents, from public reporting: the 2022 Plex account breach, old-version remote code execution through an admin token (CVE-2020-5741), SSDP amplification, token leakage through `X-Plex-Token` in URLs, the `plex.direct` certificate model; Emby's 2023 botnet (passwordless admin on the "local network", reached by spoofed proxy headers, followed by a malicious plugin install).

**Portico code read (properly, end to end unless marked sampled).**

- Server HTTP layer: `apps/portico-server/server/internal/httpapi/router.go` (all), `rate.go`, `request_locality.go`, `onboarding.go`, `identity_routes.go`, `direct_identity.go`, `access_admission.go`, `owner_authority.go`, `access_area.go` (parts), `secure_connections.go`, `installation.go`, `administration.go` (parts), `admin_pprof.go`, `admin_diagnostics.go` (parts), `backup_routes.go` (route table), `topshelf.go`, `guide_images.go`, `trickplay.go` (parts), `fairness.go` (parts), `principal_cache.go` (constants), `dev_e2e.go` (build tags), `restriction_matrix_test.go` and `hostile_input_test.go` (as evidence of what is tested). Every one of the 329 `mux.Handle…` registrations was passed through a script that looks for an authentication call in the handler's first lines; every handler the script could not classify was opened by hand.
- API toolkit: `apps/portico-server/apikit/registry.go` (access classes and admission).
- Identity: `internal/identity/service.go`, `credential_security.go`, `hash_cost.go`, `direct_accounts.go` (first half), `refresh.go`, `families.go`, `quick_connect.go`, `onboarding.go`, `two_factor.go` (parts), `direct_profiles.go` (PIN path), `browser_accounts.go`, `issuing_device.go`, `recover_owner.go` (first half), `migrate_sealed.go` (head).
- Hosted trust on the server: `internal/hostedtrust/trust.go`, `internal/hosted/trust.go`, `internal/hosted/identity_assertion.go`.
- Media processes: `internal/mediaexec/mediaexec.go`, `internal/decoder/burn_in.go`, `linear.go`, `linear_hls.go` (parts), `hardware.go` (filter assembly), `internal/playback/hls.go` (argument builder and artifact names), `finite_hls.go` (file resolution); every `-protocol_whitelist`/filter site was listed by search.
- Outbound fetching: `internal/remotemedia/policy.go` and `client.go`, `internal/metadata/artwork_worker.go` (allowlist and dialer), `internal/administration/update_feed.go`; every HTTP client construction site was listed by search.
- Files and parsing: `internal/administration/filesystem.go`, `internal/backup/backup.go` (head, delete), `stateperms.go`, `internal/servicelog/bundle.go` and `clientlogs.go` (redaction), all XML decoder and image decode sites by search.
- Networking: `internal/networking/route_identity.go` (parts), `certificate_listener.go` (TLS config), `cmd/server/main.go` (listener, origins, provider keys).
- Web: `web/src/bridge/server-connections.ts`, `account.ts`, `protectedArtwork.ts`, `packages/client-core/src/credential-format.ts`; a search of all of `web/src` and `packages/*/src` for HTML sinks and browser storage.
- Native: `apps/native/ios/Portico/Info.plist`, `tvos/Portico/Info.plist`, `android/app/src/main/AndroidManifest.xml`, `network_security_config.xml`, `data_extraction_rules.xml`, `Native/PorticoNetwork.swift` (key handling), `PorticoUrlRoutes.kt` (head), `PorticoRecordStore.kt` (head), `PorticoTopShelf.kt` (content provider), Keychain attribute sites by search.
- Supply chain: `SECURITY.md`, `.github/workflows/security.yml`, `server-trust.yml`, `release.yml` and `ci.yml` (pins and signing steps), `Dockerfile`, `.gitleaks.toml`, `server/internal/metadata/project.go` and `tvdb_project.go`.
- Docs: Product Contracts `identity/contract.md` and `networking/contract.md` (both in full), Project Architecture `01`, `02`, `07`, `08`, `Backlog.md`, the summary of the 25 Sep code audit.

**Comparison code read.** `jellyfin/Jellyfin.Server.Implementations/Users/UserManager.cs` (lockout), `Emby.Server.Implementations/Cryptography/CryptographyProvider.cs` and `MediaBrowser.Model/Cryptography/Constants.cs` (hashing), `Emby.Server.Implementations/QuickConnect/QuickConnectManager.cs`, the `[Authorize]` attributes of `ImageController`, `VideosController`, `AudioController`, `UserController`, `SystemController`, `StartupController`, `EnvironmentController`, a search of the server for security response headers, `Jellyfin.Server/Extensions/ApiServiceCollectionExtensions.cs` (forwarded headers). Plezy: `lib/services/storage_service.dart`, `lib/database/tables.dart`, platform manifests. Token storage in `jellyfin-android`, `swiftfin` and `streamyfin` by search only.

**Not reached.** Hosted services source (`portico-internal/hosted-services`) beyond `ACCOUNT_SECURITY.md` and the deployed CSP in `deployment/hosted/Caddyfile`; the DNS service; `internal/social` and the Cast receiver protocol (domain 13 owns them; the Jellyfin "session remote-control" advisory is the question to ask there); `internal/downloads` receipts (domain 14); rclone remote configuration and `internal/mounts`; `internal/subtitles` parsers line by line; the Vega and Roku apps; the native JavaScript layer's handling of `portico://` links after the native grammar check; dependency versions against current advisories.

## How Portico does it today

**Who can call what.** The server registers routes in two styles. Newer routes go through the apikit registry, where every route declares an access class (`public`, `device`, `viewer_library`, `viewer_item_restrictions`, `owner`, `admin_scope`) and the registry refuses to define a non-public route without an authoriser (`apikit/registry.go:24-31`, `:150-166`, `:190-196`). Older routes are plain `mux.HandleFunc` handlers that each call a helper first: `d.principal(r)` for a signed-in viewer, `d.owner(r)` / `d.ownerContext` for the owner, `d.administrator(r)` for the admin tier, `directBearer(r)` passed into an identity service method for account-level routes (`httpapi/router.go:601-664`, `owner_authority.go:44-75`, `access_area.go:71-83`).

The only routes that answer without a credential are: health and readiness, `GET /v1/system`, `GET /v1/server`, `GET /v1/auth/capabilities`, `GET /v1/attribution`, first-run setup (`POST /v1/setup`, `/v1/setup/browser`, `/v1/setup/resume`), sign-in (`POST /v1/sessions`, `/v1/direct/sign-in`, `/v1/direct/portico-challenge`, `/v1/auth/two-factor/challenge`, `/v1/auth/refresh`, `/v1/auth/register`), `GET /v1/auth/remembered-accounts`, TV-code start and polling (`POST /v1/quick-connect`, `/token`, `/cancel`), invitation preview and acceptance, the signed Hosted wake and the server identity proof, the Cast receiver page and the static web app. Media bytes are served on capability URLs (below).

**Sessions.** A sign-in creates a *session family*: a 32-byte random access token valid 15 minutes and a 32-byte random refresh token, both stored only as SHA-256 digests (`identity/service.go:103-113`, `families.go:104-158`). Refresh rotates both; presenting an already-used refresh token outside an exact replay of the same request revokes the whole family (`refresh.go:36-55`). A refresh also needs the *installation id*, a random string the client generated and sends as `X-Portico-Installation-Id` (`refresh.go:22-24`, `:60-63`). Every request re-checks the account, profile, epoch and device approval (`families.go:236-250`); a 15-second cache is used only on lanes where that lag is acceptable (`router.go:615-626`, `principal_cache.go:40`). There are no cookies on the home server; credentials travel only in the `Authorization` header.

**Passwords.** bcrypt cost 10, 8 to 72 characters, no composition rule (`identity/hash_cost.go:8-11`, `direct_accounts.go:87-89`). An unknown username costs the same bcrypt work and gets the same cool-off progression as a real one (`direct_accounts.go:26`, `:117-121`, `credential_security.go:118-146`). Wrong guesses earn a cool-off that grows to at most 60 seconds per source network and per account; nothing ever locks an account (`credential_security.go:74-96`, `:275-321`). An in-memory limiter allows 20 authentication requests a minute per address and path (`rate.go:38-43`, `router.go:1298-1304`), and a database-backed limiter counts sign-in attempts per peer per ten minutes (`quick_connect.go:117-132`). Two-factor (TOTP, single-use steps, recovery codes) is available for direct accounts (`two_factor.go`).

**TV codes.** A device asks for an 8-character code (about 37 bits) valid ten minutes; a signed-in member approves it; the device then collects a session as the approver, with the approver's role (`quick_connect.go:22`, `:81-95`, `:209-286`, `:540-588`).

**First-run setup.** A fresh server writes a setup token to a file. `POST /v1/setup/browser` hands that token to any browser whose `Origin` header matches the request's `Host` (`httpapi/onboarding.go:132-164`); the first owner created wins (`identity/onboarding.go:55-141`). The owner later recovers a lost password only from the server's command line (`identity/recover_owner.go:14-27`).

**Media URLs.** Starting playback mints a *grant*: a random token embedded in the URL path (`/v1/media/{grant}/…`). The grant is stored hashed, expires, belongs to one playback session, and is re-checked against the viewer's current access on every request and roughly every 100 ms while bytes flow (`router.go:1123-1233`, `:1355-1369`). HLS file names must match a fixed pattern (`playback/hls.go:815`). Images need the normal bearer token; the web app fetches them with the header and turns them into blob URLs (`web/src/bridge/protectedArtwork.ts:26-50`).

**Media processes.** FFmpeg and ffprobe run only through `internal/mediaexec`: sandboxed with bubblewrap on Linux and `sandbox-exec` on macOS (no network, only the tool, its libraries, the declared input and private output folders visible), inputs passed as open file descriptors or a private loopback bridge rather than paths, a protocol whitelist and format whitelist on every invocation, a minimal environment, file-size and CPU limits (`mediaexec/mediaexec.go:1-30`, `:269-285`). Arguments are assembled from numbers and fixed vocabulary; the one file path that enters a filter graph is a server-written file and is escaped at both levels (`decoder/burn_in.go:19-31`).

**Outbound requests.** Anything fetched because of library content or an owner-configured source (`.strm` targets, IPTV playlists and guides, WebDAV remotes, downloaded subtitles) goes through `internal/remotemedia`: http/https only, no credentials in URLs, cloud metadata addresses always refused, private addresses refused until the owner confirms that exact origin, the resolved address pinned at dial time, redirects re-validated and stripped of credentials (`remotemedia/policy.go:60-113`, `:218-250`, `client.go:22-80`). Artwork is fetched only from named provider hosts with a public-address-only dialer and manual redirect handling (`metadata/artwork_worker.go:40-90`).

**Browser.** Every response carries a Content-Security-Policy (`script-src 'self' https://www.gstatic.com; style-src 'self'; frame-ancestors 'none'; object-src 'none'; base-uri 'none'`), `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`, and HSTS when the connection is TLS (`router.go:1240-1247`). A request with an `Origin` header is refused unless the origin is the server itself or on the allow list, which is `https://web.getportico.tv`, `https://cast.getportico.tv` and anything in `PORTICO_ALLOWED_ORIGINS` (`router.go:1248-1271`, `cmd/server/main.go:551-556`). The web app stores the home-server session in IndexedDB, encrypted with a non-extractable WebCrypto key when the page is a secure context, and as plain JSON when it is not (plain HTTP on the LAN) (`web/src/bridge/server-connections.ts:6-10`, `:36-39`).

**Hosted.** Release builds compile in an offline Ed25519 root public key and refuse to start without it (`hostedtrust/trust.go:27-29`, `:66-72`). Hosted signs with online keys certified by that root for at most 400 days; servers remember the highest certificate revision and every key id it revoked (`hosted/trust.go:59-108`). Hosted never issues a server session. It signs "this is Portico Account A, speaking to server S, answering challenge C"; the server checks that statement offline, spends the challenge and nonce, and decides membership from its own tables (`hosted/identity_assertion.go:14-30`, `:105-176`).

**At rest.** Nothing is encrypted. Token digests are hashed, but two-factor seeds, the last refresh response of every family (which contains the family's current access and refresh tokens) and playback renewal responses are stored as plain values so a device whose answer was lost can retry (`identity/quick_connect.go:106-116`, `refresh.go:117-130`, `families.go:376-379`). The server checks that the state folder is not readable by other users and offers to fix it (`backup/stateperms.go:13-40`). Backups are plain copies of the database and key files inside the state folder (`backup/backup.go:1-10`).

## How the others do it

**Jellyfin.** ASP.NET controllers; an endpoint is authenticated only if it or its controller carries `[Authorize]`, optionally with a policy such as `RequiresElevation` (administrator) or `FirstTimeSetupOrElevated`. In the October 2026 clone the video stream (`jellyfin/Jellyfin.Api/Controllers/VideosController.cs:314`), audio stream (`AudioController.cs:88`) and image read endpoints (`ImageController.cs`, where only uploads and deletes are annotated) still carry no `[Authorize]`: knowing an item id is enough. `/System/Info/Public` and `/Users/Public` are public by design. Sessions are one long-lived opaque token per device, accepted in a header or as an `api_key`/`ApiKey` query parameter (needed by players that cannot set headers). Passwords are PBKDF2-SHA512 with 210,000 iterations (`MediaBrowser.Model/Cryptography/Constants.cs:21`); empty passwords are allowed. A per-user counter disables the account after a configured number of wrong passwords (`UserManager.cs:680-692`). There is no built-in second factor. Quick Connect uses a 6-digit code (`QuickConnectManager.cs:26`). The server sets no CSP, frame or sniffing headers of its own (none found by search). FFmpeg is launched unsandboxed with a command line built by string concatenation that includes file paths and, historically, request parameters. Administrators can install plugins (arbitrary code), set the FFmpeg path, and browse the filesystem; the setup wizard and the directory browser are open until setup completes. Proxy headers are honoured only from `KnownProxies`.

**Jellyfin web.** A large vanilla-JS and React hybrid that builds HTML by string concatenation in many places; this is where its stored-XSS advisories came from. Tokens live in `localStorage`.

**Plezy.** A client only. Tokens are kept in its local SQLite (Drift) database in a config payload column (`plezy/lib/database/tables.dart:198`), with a redaction pass for logs. iOS, tvOS and macOS builds set `NSAllowsArbitraryLoads`; Android sets `usesCleartextTraffic="true"`. It has a small relay server (`plezy/server/`) with its own OAuth and client-IP code, not reviewed here.

**Other clients.** `jellyfin-android` stores the access token in a Room table (`data/entity/UserEntity.kt:31`); Swiftfin uses the Keychain; Streamyfin has a secure-credentials helper.

**Plex and Emby (public documentation only).** Plex authenticates everything through plex.tv accounts and a long-lived `X-Plex-Token` that frequently appears in URLs; servers get a `*.plex.direct` certificate; "Secure connections" can be Required, Preferred or Disabled; a list of networks can be "allowed without auth". Emby has per-user passwords, optional passwordless local-network sign-in, and plugins.

## Architectural differences

Assessment words: **Portico ahead**, **Portico behind**, **Trade-off (subjective)**, **Unclear**. "Deliberate" means the choice is written down; the citation says where.

### Authentication and the request surface

### A-15.1 How a route becomes authenticated
- **Portico:** Two styles. Registry routes declare an access class and cannot be defined without an authoriser (`apikit/registry.go:190-196`). Roughly 330 older `mux.HandleFunc` routes each call an auth helper by hand as their first statement.
- **Jellyfin:** A declarative `[Authorize]` attribute per controller or action, with named policies. Forgetting it leaves the endpoint open, which is exactly how issue #5415's list came about.
- **Why it matters:** Both hand-written styles fail open when a developer forgets. The registry style fails closed.
- **Assessment:** **Portico ahead in outcome, level in structure.** I opened every raw handler the script could not classify and found none missing a check. But nothing in the test suite asserts "every route answers 401 to an anonymous caller unless it is on a named public list". The restriction matrix test already walks the complete route table (`restriction_matrix_test.go:57-80`), so adding that assertion is small (see F-15.1). Finishing the move of raw routes onto the registry is medium.

### A-15.2 Size of the unauthenticated surface
- **Portico:** About two dozen routes, all listed in "How Portico does it today". None returns library content.
- **Jellyfin:** Streams, images, subtitles, attachments, public user list, public system info, branding (issue #5415; confirmed for streams and images in the clone).
- **Why it matters:** On Jellyfin, anyone who can reach the port and guess or learn an item id can pull video. On Portico they get a server name and a version.
- **Assessment:** **Portico ahead.** Keep it that way.

### A-15.3 Images need a credential
- **Portico:** Artwork, avatars, trickplay tiles, chapter images and guide images all require the bearer token (`router.go:1058-1090`, `identity_routes.go:79-118`, `guide_images.go:19-22`). The web app fetches them with a header and enforces same-origin, no redirects, an image content type and a size cap before attaching the token (`protectedArtwork.ts:27-45`).
- **Jellyfin:** Image reads are public; clients use plain `<img src>`.
- **Why it matters:** Private posters (home videos, adult libraries) do not leak. The cost is that every image is a scripted fetch and a blob URL rather than a browser-native image load, which matters for memory and caching (domain 06/09 territory).
- **Assessment:** **Portico ahead on security; a real performance trade-off.** Deliberate.

### A-15.4 Media URLs are per-playback capabilities
- **Portico:** `/v1/media/{grant}/…` where the grant is random, hashed at rest, tied to one playback session and re-authorised continuously (`router.go:1123-1233`).
- **Jellyfin / Plex:** Stream by item id, with the account-wide token as a query parameter when the player cannot send headers (`api_key`, `X-Plex-Token`).
- **Why it matters:** A leaked Portico media URL (proxy log, shared screenshot, cast device) exposes one title until that session ends. A leaked Jellyfin or Plex URL exposes the account.
- **Assessment:** **Portico ahead.**

### A-15.5 Credentials in URLs
- **Portico:** One case: the tvOS Top Shelf artwork URL carries a narrow, item-bound, read-only grant as `?grant=` (`topshelf.go:136-137`), because a system extension cannot send headers. The installation id (not a credential, but a second factor for refresh) also appears in a query string on `GET /v1/auth/remembered-accounts` (`identity_routes.go:170-174`).
- **Jellyfin / Plex:** Account tokens in URLs are routine; the WebSocket takes `api_key` in its URL.
- **Assessment:** **Portico ahead.** The contract's absolute wording ("secrets never appear in URLs") is not literally true; see Doc/code disagreements.

### A-15.6 Session model
- **Portico:** 15-minute access token, rotating refresh token with reuse detection, session families that end together, a 90-day idle window renewed by use (`families.go:169`), per-request re-check of account state.
- **Jellyfin:** One opaque token per device, valid until revoked. **Plex:** Long-lived account token.
- **Why it matters:** A stolen Portico access token dies in minutes. A stolen refresh token is caught when the real device next refreshes, and both lose the session. On Jellyfin a stolen token works until someone notices.
- **Assessment:** **Portico ahead.** The price is client complexity (refresh races, replay receipts), which the Backlog's "two Apple sign-ins ended without an obvious cause" item shows is not free.

### A-15.7 "Device-bound" means a random id, not a key
- **Portico:** A refresh needs the refresh token plus the installation id (`refresh.go:22-24`, `:60-63`). The installation id is sent in a header on every request. There is no per-device key pair and no signature over requests on the home server (search of `internal/identity` for key or signature verification finds none).
- **Others:** Jellyfin and Plex have nothing comparable either. Proof-of-possession tokens (DPoP, mutual TLS) are the technique that would actually bind a token to a device.
- **Why it matters:** `02 Server.md` says "a device registers a key". If that were true, a token sniffed off plain HTTP would be useless to the thief. As built, it is fully usable. See B-15.7.
- **Assessment:** **Portico level with peers, behind its own documentation.** Adding real proof-of-possession is large (every client, every request path) but is the one measure that would make "plain HTTP is first-class" defensible against a passive listener.

### A-15.8 Password hashing
- **Portico:** bcrypt cost 10, 72-byte maximum, longer inputs refused rather than silently truncated (`hash_cost.go:8-11`, `direct_accounts.go:103`). Recorded as Justin's decision of 23 Sep in the file.
- **Jellyfin:** PBKDF2-HMAC-SHA512, 210,000 iterations.
- **Why it matters:** Both sit at current OWASP minimums. bcrypt is somewhat more resistant to GPU cracking per unit of server time; PBKDF2 has no length cap. Neither is memory-hard (Argon2id, scrypt).
- **Assessment:** **Trade-off (subjective).** Deliberate. Cost 10 is the floor, chosen so weak NAS boxes stay responsive; I would not lower it and would revisit when the hardware floor rises.

### A-15.9 Password rules
- **Portico:** At least 8 characters, nothing else (`direct_accounts.go:83-89`).
- **Jellyfin:** No minimum; empty passwords allowed and common. **Emby:** same, which fed the 2023 incident.
- **Assessment:** **Portico ahead.** Note the Identity contract still specifies upper/lower/digit rules that the code deliberately does not enforce.

### A-15.10 Wrong-password handling
- **Portico:** Never locks. Cool-off per source network (/24 or /64) capped at 60 s, an account-wide budget that delays only sources that have guessed wrong, a separate bucket for a device that has signed in to this account before, identical behaviour for unknown usernames (`credential_security.go:74-146`, `:275-333`). Deliberate: "a correct username and password always signs you in" (`01 Overview.md`, principles).
- **Jellyfin:** Counts failures per user and disables the account at a threshold; an administrator must re-enable it.
- **Why it matters:** Jellyfin's rule stops online guessing cold but lets anyone lock anyone out by typing wrong passwords. Portico's rule cannot be used to lock people out, but an attacker with many source networks gets roughly one guess per network per minute against a known username, indefinitely.
- **Assessment:** **Trade-off (subjective).** For a household server I think Portico's side is the better default. It makes password strength and the optional second factor carry more weight, and it argues for an owner-visible sign-in history (F-15.5) and an optional stricter mode for internet-exposed servers (F-15.4).

### A-15.11 Username enumeration
- **Portico:** Unknown and known names take the same bcrypt work and show the same cool-off progression (`direct_accounts.go:117-131`; test `TestUnknownAndKnownNamesHaveTheSameSignInCooloff`). No public user list.
- **Jellyfin:** `/Users/Public` lists users (configurable per user).
- **Assessment:** **Portico ahead.** One small leak remains in `remembered-accounts` (B-15.14).

### A-15.12 Second factor
- **Portico:** TOTP with one-time steps and hashed recovery codes for direct accounts, and at Hosted for Portico Accounts.
- **Jellyfin:** None in core. **Plex:** At plex.tv.
- **Assessment:** **Portico ahead.** See B-15.9 for how the no-lockout rule limits what the second factor buys.

### A-15.13 TV-code sign-in
- **Portico:** 8 characters from a 31-letter alphabet, ten-minute life, approval only by a signed-in member, limited to 10 lookups per 10 minutes per address and per account, security notice to the approver (`quick_connect.go:80-95`, `:224-309`; `httpapi/onboarding.go:259-270`).
- **Jellyfin:** 6 digits.
- **Why it matters:** Jellyfin's code space is a million; Portico's is about 2 × 10^11.
- **Assessment:** **Portico ahead.** Both share the inherent weakness of the pattern: whoever gets you to approve *their* code becomes you. On Portico an owner's approval yields an owner session (B-15.13).

### A-15.14 First-run setup
- **Portico:** Open to any browser that can reach the server, from anywhere, with no code. Recorded decision: "Any browser may start first-run setup (Justin, 23 Sep: no setup code, even remotely)" (`httpapi/onboarding.go:141-144`).
- **Jellyfin:** The wizard is likewise open until completed. **Plex:** Claiming needs a plex.tv sign-in from the same machine or network.
- **Assessment:** **Trade-off (subjective), leaning behind Plex.** The window is short for a careful owner but indefinite for a server left un-set-up, and the present code also lets a web page drive it (B-15.4).

### A-15.15 Owner recovery
- **Portico:** Only from the server's own command line; issues a temporary password that must be changed, revokes every session, optionally resets two-factor (`recover_owner.go:14-60`). A separate "recovery owner" account on Hosted-mode servers signs in by password only from a private address unless the owner turned remote recovery on (`identity/onboarding.go:185-200`).
- **Jellyfin:** "Forgot password" writes a PIN file on disk, usable from the local network.
- **Assessment:** **Portico ahead.**

### A-15.16 What "local network" is trusted for
- **Portico:** Nothing passwordless. A private peer address only (a) lets the recovery owner use its password, and (b) decides whether the remote sign-in policy, remote bitrate limit and upload budget apply (`request_locality.go:48-101`, `httpapi/onboarding.go:18-45`). A request carrying `X-Forwarded-For` while no proxy is configured is never private.
- **Emby / Plex:** Offer passwordless or tokenless access for "local" clients. Emby's 2023 incident was this feature plus spoofable proxy headers.
- **Assessment:** **Portico ahead.** The residual risk is B-15.11.

### A-15.17 Proxy header handling
- **Portico:** Forwarded headers count only from configured proxy networks (`PORTICO_TRUSTED_PROXY_CIDRS` plus an owner setting). But three separate functions interpret them, with different rules (`rate.go:clientAddress`, `request_locality.go:forwardedClient`, `httpapi/onboarding.go:privateSetupPeer`), and a fourth, `setupPeerSubject`, ignores them.
- **Jellyfin:** One ASP.NET forwarded-headers middleware fed by `KnownProxies`; it has had its own CVE here.
- **Assessment:** **Trade-off; Portico's principle is right, its implementation is fragmented.** See B-15.3 and B-15.6. One shared "client address of this request" function would be a small, high-value consolidation.

### Browser-facing

### A-15.18 Security response headers
- **Portico:** CSP, `nosniff`, frame denial, `no-referrer`, HSTS on TLS, `Cache-Control: no-store` by default (`router.go:1240-1247`).
- **Jellyfin:** None set by the server.
- **Assessment:** **Portico ahead.** Small tightening available (B-15.16).

### A-15.19 Cross-origin requests
- **Portico:** Any request with an unlisted `Origin` is refused before routing; allowed origins are echoed exactly; no `Access-Control-Allow-Credentials` (`router.go:1248-1271`). Hosted's web app and the Cast receiver host are allowed by default because a packaged server has no environment to configure.
- **Jellyfin:** CORS hosts configurable, default permissive.
- **Why it matters:** Because the home server uses bearer headers and never cookies, a hostile page cannot ride a signed-in browser's authority anyway (no CSRF). The origin check is a second layer.
- **Assessment:** **Portico ahead.** The built-in trust of `https://web.getportico.tv` is the mechanism behind A-15.33.

### A-15.20 Where the browser keeps the session
- **Portico:** IndexedDB. On HTTPS, sealed with a non-extractable AES-GCM key so a copy of the profile directory is ciphertext; on plain HTTP, plain JSON (`server-connections.ts:6-10`, `:36-39`). The Portico Account's refresh credential is an HttpOnly cookie at Hosted and never reaches script (`credential-format.ts:33-35`).
- **Jellyfin web:** Access token in `localStorage`.
- **Why it matters:** Neither design survives script injection in the page: injected script can ask WebCrypto to decrypt, or simply call the API. The sealing only protects against offline copies of browser storage.
- **Assessment:** **Portico modestly ahead.** The Identity contract promises HttpOnly cookies for the home server too; the code does not do that (Doc/code disagreements).

### A-15.21 HTML injection sinks
- **Portico:** A search of `web/src` and the shared packages finds no `dangerouslySetInnerHTML`, `innerHTML`, `outerHTML`, `insertAdjacentHTML`, `document.write`, `eval` or `new Function`. Metadata, lyrics and subtitles are rendered as React text. Two places put a server-supplied URL into an `href` (`web/src/screens/library/Entity.tsx:230`, `web/src/screens/server/Maintenance.tsx:134`); React 19 and the CSP both stop `javascript:` URLs.
- **Jellyfin web:** Many string-built HTML fragments; repeated stored-XSS advisories from item, collection, device and user names.
- **Why it matters:** On Jellyfin, stored XSS in an administrator's browser became server code execution through plugin install. Portico has neither the sink nor the plugin installer.
- **Assessment:** **Portico ahead.** Worth a lint rule so it stays true (F-15.16).

### Media and files

### A-15.22 Running FFmpeg
- **Portico:** One package may spawn media tools, enforced by a spawn-site inventory test. Sandbox with no network and a minimal filesystem view; inputs by file descriptor or loopback bridge; protocol and format whitelists; stripped environment; size and CPU limits; owner can require the sandbox (`mediaexec/mediaexec.go:1-30`).
- **Jellyfin / Plex / Emby:** The transcoder runs as the server user with full filesystem and network access.
- **Why it matters:** A malicious media file that exploits a demuxer bug gets the server's whole authority on the others. On Portico with the sandbox active it gets one job's input and scratch folder.
- **Assessment:** **Portico ahead, and this is the single largest security difference.** Caveat: where the sandbox is unavailable (Docker default profiles, Windows) the baseline is closer to the others, and the Dockerfile says so honestly (`Dockerfile:22-28`).

### A-15.23 Building FFmpeg arguments
- **Portico:** Arguments are numbers and fixed tokens chosen by the server's own plan; the client sends capabilities, not codec strings. The only path in a filter graph is a server-written subtitle file, validated and escaped for both the option parser and the filter-graph parser (`decoder/burn_in.go:19-31`). Inputs are `/dev/fd/N` or a loopback URL, never a library file name.
- **Jellyfin:** Command line assembled as a string including file paths and, historically, request parameters; three argument-injection advisories (2023, 2025, June 2026 via subtitle file path).
- **Assessment:** **Portico ahead.** I searched every filter and protocol site and read the main builders; I did not read every line of every builder.

### A-15.24 No owner-to-code-execution features
- **Portico:** No plugins, no configurable transcoder path in the API (tool paths come from the environment), no post-processing scripts, API keys switched off. In the routes I read, an owner session can delete media and read the filesystem tree but cannot make the server run a program of the attacker's choosing.
- **Jellyfin / Emby / Plex:** Plugin install, custom FFmpeg path, DVR post-processing scripts; each has been the last step of a real attack chain.
- **Assessment:** **Portico ahead.** Not verified for rclone remote configuration (`internal/remotesources`, `internal/mounts`), where option strings can be dangerous; that deserves its own look.

### A-15.25 Server-side request forgery
- **Portico:** One policy package for everything library content or a source can make the server fetch; cloud metadata addresses hard-blocked; private addresses need the owner's confirmation of that exact origin; address pinned at dial; redirects re-checked (`remotemedia/policy.go`, `client.go`). Artwork only from named hosts. Hosted has a matching egress confinement (`07 Hosted Services.md`).
- **Jellyfin:** Unauthenticated SSRF in 2021; SVG-driven SSRF in September 2026; `.strm` targets handed to FFmpeg.
- **Assessment:** **Portico ahead.** One owner-only exception: the release feed URL uses a plain client (B-15.17).

### A-15.26 Image handling
- **Portico:** Pure-Go JPEG, PNG and WebP decoders; dimensions and pixel count checked from the header before decoding (`identity/avatars.go:45`, `:162-163`; every `image.Decode` site has a `DecodeConfig` beside it); a concurrency gate; no SVG.
- **Jellyfin:** Native Skia; SVG accepted (stored XSS in 2024, memory exhaustion and SSRF in 2026).
- **Assessment:** **Portico ahead.** The cost is no SVG channel logos.

### A-15.27 XML and archives
- **Portico:** Go's `encoding/xml` for NFO and XMLTV, which does not fetch external entities or expand custom ones; inputs size-bounded. No archive is ever unpacked: backups are plain folders, so there is no "zip-slip" path (`backup/backup.go:1-10`). No MKV attachment extraction.
- **Jellyfin:** MKV attachment filename traversal advisory (June 2026).
- **Assessment:** **Portico ahead.** (Whether skipping embedded fonts hurts subtitle rendering is domain 04's question.)

### A-15.28 Paths that come from requests
- **Portico:** Static files through `os.OpenRoot`, which confines symlinks (`installation.go:56-62`); HLS artifacts by a fixed name pattern (`playback/hls.go:815`); backup ids validated; client logs stored as database rows, not files.
- **Jellyfin:** Path traversal advisories in 2021, 2023 and four more in 2026 (HLS segments, client log, plugins, virtual folders).
- **Assessment:** **Portico ahead.**

### A-15.29 Server-side folder picker
- **Portico:** Owner only, re-checked after the work; honours a configured-roots policy; refuses `/proc`, `/sys`, `/dev`, `/run`; hides dotfiles; bounded listing (`administration/filesystem.go:64-140`).
- **Jellyfin:** Open during first-time setup, administrator afterwards.
- **Assessment:** **Portico ahead.** Small ordering nit in B-15.18.

### Authorisation of content

### A-15.30 Restrictions are part of the query, and every route is tested
- **Portico:** Rating ceilings, blocked labels and library access are applied inside the catalogue's read paths. A test walks the complete route table and fails if a new GET route has no restriction classification or if any probe returns a hidden title to a restricted profile (`restriction_matrix_test.go:38-80`).
- **Jellyfin:** Filters applied per controller; parental-control leaks through side endpoints have been a recurring bug class; the September 2026 playlist IDOR is the same family.
- **Assessment:** **Portico ahead.** This test is worth advertising.

### A-15.31 Who the request is "as"
- **Portico:** The viewer is derived from the token. No route takes a user id to act as.
- **Jellyfin:** Many routes take `userId` and historically did not check it against the session (issue #5415, "authentication and user IDs are decoupled").
- **Assessment:** **Portico ahead.**

### A-15.32 API keys
- **Portico:** Designed with scopes (`access/apikeys.go:14-21`) but creation answers `api_keys_disabled` and authentication accepts only session families (`admin_access.go:243-248`, `access_area.go:85-90`).
- **Jellyfin:** Administrator-equivalent API keys, usable in URLs.
- **Assessment:** **Trade-off.** Safer today; it also means no third-party integrations (F-15.11).

### Hosted, trust and transport

### A-15.33 A central account service exists at all
- **Portico:** Optional. A server can run with direct sign-in only and never talk to Hosted. With Portico Accounts, Hosted proves identity; the server decides membership.
- **Jellyfin:** No central service. **Plex:** Mandatory, and plex.tv tokens are what servers accept.
- **Why it matters:** A central service is one place to attack for many homes. Portico's blast radius if Hosted is compromised: (1) with an online signing key, an attacker can assert any Portico Account id to any claimed server and is admitted with that member's rights, including an owner who signs in by Portico Account (B-15.12); (2) with control of the web host or its CDN, hostile script on `web.getportico.tv` runs in the one origin every home server trusts for cross-origin calls, and can use the session of each visiting user on the server they have selected; (3) with DNS, an attacker can point a server's `direct.getportico.tv` name elsewhere and, with the CA account, obtain a certificate for it. What Hosted cannot do: read media or libraries in normal operation, mint a server session directly, or reach a direct-sign-in-only server at all.
- **Assessment:** **Trade-off (subjective).** Better contained than Plex, more exposed than Jellyfin. The offline root, the per-server membership decision and the direct-only mode are the right ingredients.

### A-15.34 Trust root and key rotation
- **Portico:** Offline Ed25519 root compiled into release builds; online keys certified for a year, each certificate carrying a revision number and a revocation list the server remembers permanently; a CI job fails if a development root could reach a default build (`server-trust.yml`, `hostedtrust/trust.go:46-72`).
- **Others:** Not public for Plex. Jellyfin has no equivalent need.
- **Assessment:** **Portico ahead of anything comparable I can see.** The gap is how fast a server that Hosted cannot reach learns of a revocation (B-15.12).

### A-15.35 TLS
- **Portico:** Optional. Claimed servers get a publicly trusted wildcard certificate for their own random name through DNS-01, with the private key generated and kept on the server (Networking contract §8). TLS 1.2 minimum (`networking/certificate_listener.go:76`). Owners can supply their own certificate.
- **Plex:** Same idea (`plex.direct`). **Jellyfin:** Bring your own certificate or reverse proxy.
- **Assessment:** **Portico ahead of Jellyfin, level with Plex.**

### A-15.36 Plain HTTP as a first-class transport
- **Portico:** Deliberate principle ("Plain HTTP stays first-class", `01 Overview.md`). The server listens on `0.0.0.0:32500` for HTTP always. "Secure connections: required" refuses `/v1` over HTTP only when the server is claimed and has a working certificate, never for loopback, and never for the static web app (`secure_connections.go:122-192`). Native clients are stricter than the server: Apple builds allow cleartext only for local networking, and the Networking contract (§4) allows HTTP only to local addresses.
- **Plex:** "Required / Preferred / Disabled". **Jellyfin:** HTTP by default.
- **Why it matters:** Over HTTP, anyone on the path sees the password at sign-in, every access and refresh token, the installation id, and every media grant. Rotation and reuse detection limit how long a stolen refresh token lasts only if the real device comes back; nothing else mitigates it (A-15.7).
- **Assessment:** **Trade-off (subjective), deliberate.** On a home LAN this is the industry norm. Over the internet it is the weakest point of the whole design, and the server does nothing to discourage a browser from signing in at `http://<public-ip>:32500`.

### A-15.37 Server identity pinning
- **Portico:** Clients pin the server's Ed25519 key and verify a signed, nonce-fresh identity proof before sending credentials (`networking/route_identity.go:20-33`), including over HTTP.
- **Others:** None; they rely on TLS or nothing.
- **Why it matters:** This stops a different machine that has taken over the address from collecting credentials. It does not stop someone in the middle who relays the proof and reads the traffic.
- **Assessment:** **Portico ahead**, with the limits stated.

### A-15.38 Encryption at rest
- **Portico:** None, by principle ("Open data", `01 Overview.md`). An earlier file-key design was removed; `identity/migrate_sealed.go` converts old sealed rows to plain. Folder permissions are checked and can be fixed from the console.
- **Jellyfin:** Also none; access tokens sit in the database in the clear. **Navidrome** encrypts stored passwords reversibly with a configurable key.
- **Assessment:** **Trade-off (subjective), deliberate.** Portico hashes its long-lived tokens, which is better than Jellyfin, then stores the current ones in replay receipts, which gives most of that back (B-15.5).

### A-15.39 Backups
- **Portico:** Plain copy of the database and identity files; no download route; restore stages a backup already on disk and applies it at restart.
- **Jellyfin:** Backup archives via API since 10.11.
- **Assessment:** **Portico ahead on attack surface** (no upload, no archive parsing). The contents are everything needed to become the server and its users (B-15.5).

### Observability and operations

### A-15.40 Logging discipline
- **Portico:** Request logging uses the registered route pattern, never the URL (`router.go:146-161`); 5xx causes are bounded to one sanitised line (`router.go:181-195`); uploaded client logs are scrubbed of bearer tokens, media grants and query secrets (`servicelog/clientlogs.go:30-40`); the diagnostics bundle is owner-only and built from a settings document with secret fields removed (`admin_diagnostics.go:206-221`).
- **Jellyfin:** Request URLs including `api_key` reach logs; the documentation warns about it.
- **Assessment:** **Portico ahead.**

### A-15.41 Rate limiting and overload
- **Portico:** In-memory per-address and per-network limiter on authentication paths, database-backed counters on setup, sign-in, invitation and TV-code actions, credential cool-off, and an admission layer with per-client fairness lanes. Body limit 64 KB on JSON routes (`router.go:570`), header limit 32 KB, read-header timeout 5 s (`cmd/server/main.go:959`).
- **Jellyfin:** None built in; operators add fail2ban.
- **Assessment:** **Portico ahead**, with the bugs in B-15.3 and B-15.6.

### A-15.42 Error messages
- **Portico:** Fixed public messages; internal causes only in the log; a test asserts raw errors are not disclosed (`phase_zero_test.go:10`).
- **Jellyfin:** Exception text reaches clients in places.
- **Assessment:** **Portico ahead.**

### A-15.43 Hostile-input testing
- **Portico:** Every classified route is called with hostile paths, queries and bodies, signed in and anonymous, and must not answer 5xx or panic (`hostile_input_test.go:181-221`); fuzz tests exist.
- **Jellyfin:** No equivalent sweep.
- **Assessment:** **Portico ahead.** The Backlog already notes the rotation weakness.

### A-15.44 Runtime profiling endpoint
- **Portico:** Go's pprof behind owner authority, time-clamped (`admin_pprof.go:27-55`). Heap profiles can contain secrets held in memory.
- **Jellyfin:** None.
- **Assessment:** **Trade-off.** Reasonable; worth excluding from any future scoped API key.

### Clients

### A-15.45 Native credential storage
- **Portico:** Keychain items marked this-device-only (`Native/PorticoNetwork.swift:28`, `PorticoAccountAuth.m:63`); on Android, files sealed by an Android Keystore key in the no-backup directory, backups and device transfer disabled (`PorticoRecordStore.kt:22-30`, `AndroidManifest.xml:24-26`, `data_extraction_rules.xml`).
- **Others:** jellyfin-android and Plezy keep tokens in plain app databases; Swiftfin uses the Keychain.
- **Assessment:** **Portico ahead.**

### A-15.46 Native transport rules
- **Portico:** Apple: App Transport Security on, with only the local-networking exception (`ios/Portico/Info.plist:42-48`). Android: cleartext permitted globally, system certificate authorities only, no user-installed ones (`network_security_config.xml`).
- **Plezy:** Arbitrary loads allowed on every platform.
- **Assessment:** **Portico ahead.** The Android config is broader than the contract's "HTTP only to local addresses"; the restriction there lives in JavaScript address normalisation, which I did not trace.

### A-15.47 Deep links, WebViews, exported components
- **Portico:** `portico://` and app links pass a strict grammar before reaching JavaScript (`PorticoUrlRoutes.kt:14-45`); no WebView anywhere; the one exported content provider serves only files named by a 128-bit hex pattern (`PorticoTopShelf.kt:301-319`).
- **jellyfin-android:** A WebView around the web client.
- **Assessment:** **Portico ahead.**

### Supply chain

### A-15.48 Build and release integrity
- **Portico:** Actions pinned by commit in the release and security workflows; gitleaks over history; CodeQL for Go and TypeScript; `npm ci --ignore-scripts`; reproducibility-minded Go flags; Ed25519-signed release manifest; SBOM and build-provenance attestations; a verified FFmpeg bundle; a patched hls.js vendored as a tarball. Packages are not yet OS-signed and there is no automatic update (`release.yml:234`).
- **Jellyfin:** Signed package repositories; plugin repositories as a further trust channel.
- **Assessment:** **Portico ahead on pipeline hygiene, behind on signed installers.** Loose ends in B-15.19.

### A-15.49 Credentials shipped in the binary
- **Portico:** A TMDB read token and a TheTVDB project key are compiled in on purpose and allow-listed for the secret scanner (`server/internal/metadata/project.go`, `tvdb_project.go`, `.gitleaks.toml`, `THIRD-PARTY-NOTICES.md:25-27`). Owners can override TMDB's by environment.
- **Jellyfin:** Also ships a TMDB key.
- **Assessment:** **Level; deliberate.** The risk is operational, not confidentiality (B-15.20).

### A-15.50 Network discovery protocols
- **Portico:** mDNS advertisement only (can be turned off); no DLNA, no SSDP responder (DLNA is on the Backlog as planned, off by default).
- **Plex / Jellyfin:** SSDP and DLNA; Plex's was abused for amplification.
- **Assessment:** **Portico ahead for now.** When DLNA arrives it brings unauthenticated media by design; the Backlog's own note ("exposes the whole server when on") is the right warning to keep.

## Missing features and capabilities to consider

### F-15.1 A test that every route refuses an anonymous caller unless it is on a named public list
- **What:** One table-driven test over `routeLanes` (the complete route table the restriction and admission tests already use) that calls each route with no credential and expects 401, except routes on an explicit, reviewed allow list.
- **Who has it:** Jellyfin does not, which is why its unauthenticated list grew. ASP.NET projects that are strict use a global "require authenticated user" fallback policy.
- **Value:** Converts "I read 330 handlers and found none open" into something CI proves on every change. **Size:** small. **Conflicts:** none.
- **Opinion:** Do this first. It is the cheapest structural improvement in this report.

### F-15.2 Real device binding (proof of possession)
- **What:** Each installation holds a private key in the Keychain / Keystore / non-extractable WebCrypto and signs each refresh (or each request) with it; the server stores the public key with the device.
- **Who has it:** OAuth DPoP is the standard form. No media server does this.
- **Value:** A token copied off the wire or out of a backup stops being enough. This is what would make the plain-HTTP principle hold against passive listeners. **Size:** large (server, client-core, three native platforms; WebCrypto is unavailable on plain-HTTP pages, which is exactly where it is most needed, so the web app would need a fallback).
- **Opinion:** At minimum, change the documentation to match the code (see Doc/code disagreements). Consider binding only the refresh step first; that is medium-sized and covers the long-lived credential.

### F-15.3 Passkeys (WebAuthn)
- **What:** Phishing-resistant sign-in with platform authenticators. **Who:** Plex and Emby do not; Authentik/Authelia front-ends give it to Jellyfin users.
- **Value:** Removes password guessing and TV-code-style phishing for owners. **Size:** large; needs a secure context, so only HTTPS routes. **Conflicts:** none with principles, but HTTP-only servers cannot use it.
- **Opinion:** Post-release, for Hosted accounts first.

### F-15.4 An optional stricter sign-in mode for internet-facing servers
- **What:** Owner setting to add a hard cap (for example a 15-minute lock after N failures per account from unfamiliar devices), and an allow/deny list of networks. **Who:** Jellyfin's per-user lockout; Plex's "require authentication on local networks"; fail2ban users everywhere.
- **Value:** Owners who port-forward get a real brake on guessing. **Size:** small to medium; the per-device bucket (`credential_security.go:107-123`) already separates familiar devices, so a lock on *unfamiliar* sources would not break "a correct password from your own phone always works".
- **Conflicts:** The "never a lockout" principle, unless scoped to unfamiliar sources and opt-in.
- **Opinion:** Worth offering, off by default.

### F-15.5 An owner-visible sign-in and security history
- **What:** A page listing sign-ins, failed attempts by source network, new devices, code approvals, password and two-factor changes. **Who:** Jellyfin's Activity log shows failed logins; Plex emails about new devices.
- **Value:** With no lockout and no email, this is how an owner would ever learn someone is guessing. Today a notice is raised at the fifth failure (`credential_security.go:342-344`) and for new devices; I did not find a browsable record of failures by source. **Size:** medium. **Conflicts:** none.
- **Opinion:** High value given the no-lockout choice.

### F-15.6 Setup protection options
- **What:** Any of: allow setup only from a private address by default with an explicit override; print a short code to the server console that remote setup must supply; close the setup route after N minutes of uptime until restart. **Who:** Plex's claim flow.
- **Value:** Closes B-15.4. **Size:** small (the helpers `localSetupHost` and `sameMachineSetup` still exist unused in `httpapi/onboarding.go:61-90`). **Conflicts:** Justin's 23 Sep decision.
- **Opinion:** At least restore the Host check so a web page cannot drive setup; that does not add any ceremony for a person.

### F-15.7 A real "HTTPS only" mode, and a warning on public HTTP
- **What:** When the owner chooses Required: redirect or refuse all HTTP including the web app, and say clearly in settings when the mode is not actually in force (unclaimed, no certificate). Separately, have the bundled web app show a one-line notice when it is loaded over HTTP from a non-private address. **Who:** Plex's Required mode.
- **Value:** Makes the setting mean what it says; nudges the one unsafe way to use Portico. **Size:** small. **Conflicts:** "plain HTTP stays first-class" for the notice; none for making Required honest.

### F-15.8 Keep fewer live secrets at rest
- **What:** Without adding encryption: do not copy replay receipts, renewal responses, TV-grant envelopes and the setup receipt into backups; delete each receipt as soon as its successor token is used (already done for refresh receipts on the next rotation, not for idle devices); on restore, revoke every session family so tokens in an old backup are dead.
- **Who:** Navidrome and Audiobookshelf also store secrets plainly; nobody does this well.
- **Value:** A leaked backup stops being a set of working sessions. **Size:** small to medium. **Conflicts:** none with "open data"; a restore would sign everyone out once.

### F-15.9 Optional passphrase for backups written outside the state folder
- **What:** When `PORTICO_BACKUP_DIR` points elsewhere (a NAS share), offer encryption with an owner passphrase. **Size:** medium. **Conflicts:** "Backups are plain SQLite copies, openable in any viewer" (`02 Server.md`). **Opinion:** Probably not worth the principle; F-15.8 gets most of the benefit.

### F-15.10 First-class reverse-proxy support
- **What:** Documentation for nginx/Caddy/Traefik; a "public address" setting; use the trusted proxy's `X-Forwarded-Proto` when comparing origins; one shared client-address function. **Who:** Jellyfin documents this at length (`jellyfin-docs`), with a "known proxies" setting.
- **Value:** Many self-hosters run everything behind a proxy; today that silently breaks limits and origin checks (B-15.3, B-15.6, B-15.8). **Size:** small to medium.

### F-15.11 Scoped API keys
- **What:** Turn on the designed-but-disabled keys with the four scopes in `access/apikeys.go:14-21`, header-only, hashed at rest, never accepted in a URL. **Who:** Jellyfin (unscoped), Navidrome/Subsonic, Audiobookshelf.
- **Value:** Integrations (dashboards, automation, Home Assistant) without sharing a password. **Size:** medium. **Opinion:** Worth doing after release, read-only scope first.

### F-15.12 Step-up confirmation for destructive owner actions
- **What:** Require the password (or a fresh Portico assertion) again for deleting media from disk, restoring a backup, changing library paths, turning off device approval. Ownership transfer already does this (`direct_identity.go:282-311`).
- **Value:** A stolen 15-minute owner token cannot empty the library. **Size:** small to medium. **Who:** none of the peers.

### F-15.13 Make "owner approves new devices" the suggested default for owner-tier sign-ins
- **What:** The policy exists (`identity/devices.go:31-34`); default is automatic. Consider applying approval only to owner and admin tiers by default, or prompting during setup.
- **Value:** It is the one control that also blunts a Hosted key compromise and TV-code phishing, because a new device does not get an owner session without a second, local act. **Size:** small. **Conflicts:** "low ceremony".

### F-15.14 Second-factor hardening
- **What:** A notice to the account when a correct password is followed by a wrong code; a per-account budget for factor failures that grows beyond 60 s for unfamiliar sources. **Value:** see B-15.9. **Size:** small.

### F-15.15 Signed installers and a signed update feed
- **What:** OS code signing and notarisation; when automatic update arrives, verify the signed manifest the release pipeline already produces. **Who:** Plex auto-updates; Jellyfin uses signed repositories. **Size:** medium; mostly process.

### F-15.16 Guard rails in CI
- **What:** A lint rule banning HTML sinks in web and native code; `govulncheck` and `npm audit` jobs; Docker base images pinned by digest; `require-trusted-types-for 'script'` in the CSP. **Size:** small each.

### F-15.17 Single sign-on for self-hosters (OIDC / LDAP)
- **What:** Let a server delegate sign-in to the owner's own identity provider. **Who:** Jellyfin via plugins (its LDAP plugin has had its own advisory), Audiobookshelf natively (OIDC).
- **Conflicts:** The Identity contract lists enterprise federation as a non-goal. **Opinion:** Recorded for completeness; I would not build it.

### F-15.18 A faster path for signing-key revocation
- **What:** On revocation, Hosted wakes every reachable server, and every Hosted answer (directory, heartbeat) carries the current certificate so unreachable servers learn at their next contact; publish the current key set at a well-known URL servers check daily. **Size:** small to medium. See B-15.12.

### F-15.19 Sandbox coverage
- **What:** Ship a Docker seccomp/AppArmor profile (or documented flags) under which bubblewrap works; a Windows job-object/AppContainer equivalent. **Value:** The headline protection is currently absent on the two most common non-technical installs. **Size:** medium (Docker), large (Windows).

### F-15.20 Control over version disclosure
- **What:** Drop `version`, `buildId`, `sourceDigest` and `builtAt` from the unauthenticated `GET /v1/system` answer, keeping only what a client needs before sign-in (API level is already in `GET /v1/server`). **Size:** small, if no client depends on them pre-sign-in.

### F-15.21 A published threat model and `security.txt`
- **What:** A short public page saying what Portico protects against and what it does not (plain HTTP, at-rest), plus `/.well-known/security.txt` on the web host. **Value:** Sets expectations before the first advisory; `SECURITY.md` already has the reporting process. **Size:** small.

## Possible bugs and risks in Portico

Each entry: what goes wrong, evidence, how the comparison surfaced it, **Confidence**, **Severity**, Backlog status, and a short remediation direction. Entries are grouped: confirmed weaknesses first, then design risks and deliberate trade-offs, then hardening items. The numbers are stable labels, not a ranking, and are not in numeric order across the groups. None of these is in `Backlog.md` unless stated.

### Confirmed weaknesses

### B-15.1 The remote sign-in policy is enforced on only one of the sign-in routes
- **What goes wrong:** The owner setting "Who may sign in from outside this network" (`allow` / `owner-only` / `off`; `operations/settings.go:96`) is applied by `admitRemoteSignIn`, which is called from exactly one place: `POST /v1/sessions` (`httpapi/router.go:856`; a search for the function name finds no other caller). The first-party web app signs in through `POST /v1/direct/sign-in` (`packages/client-core/src/profile-management.ts:77-78`, used at `web/src/app/session.tsx:681-726`), which applies only the access schedule (`httpapi/direct_identity.go:100-109`). Portico Account sign-in on the same route, two-factor completion (`identity_routes.go:258-281`), profile selection (`direct_identity.go:157-169`), self-registration (`identity_routes.go:237-257`), TV-code collection (`httpapi/onboarding.go:315-345`) and invitation acceptance (`admin_access.go:139`) also never call it.
- **Consequence:** With the policy set to `off` or `owner-only`, a member (or an attacker with a member's password) outside the LAN still signs in normally through the route the apps actually use. The setting looks active in the console and in `GET /v1/admin/connectivity/status`.
- **How the comparison surfaced it:** Emby and Plex both have "local network" sign-in rules that failed in practice; the question was "where exactly is Portico's enforced?".
- **Confidence:** Confirmed by reading (no test exercises enforcement; `admin_access_test.go:521-551` only checks the setting round-trips).
- **Severity:** High. A security control the owner believes is on is off for the common path.
- **Remediation direction:** Apply the policy inside the one function that issues a viewing session (or in a wrapper every issuing route must use), not per route; add a test that tries every session-issuing route from a remote address under each policy value.

### B-15.2 One approved-but-uncollected TV code permanently stops identity maintenance
- **What goes wrong:** `PruneSetup` selects `token_hash` for rows with `status='approved'` past expiry and scans it into a Go `string` (`identity/quick_connect.go:438-449`). Since approval stopped issuing the session (the session is now issued when the TV collects, going straight from `approved` to `consumed`; `:299`, `:586`), an `approved` row always has `token_hash` NULL (the column is nullable: `persistence/migrations/0001_baseline.sql:2743`). Scanning NULL into `string` is an error in `database/sql`, so the function returns before doing anything else, on every run, for as long as the row exists. The row is never removed, because the only delete excludes `status='approved'` (`:477`). The caller discards the error (`:498`).
- **When:** Someone approves a code and the TV never collects it within ten minutes (TV switched off, app closed, network drop). Any member can also cause it on purpose.
- **Consequence:** From then on nothing in that transaction runs: delivered TV-grant envelopes (which contain a plain access and refresh token) are never cleared after their five-minute recovery window (`:470`); the first-run setup receipt holding the owner's initial tokens is never cleared (`:471`); expired TV-code rows and rate-limit rows are never deleted, and both can be created by unauthenticated requests, so two tables grow without bound.
- **How the comparison surfaced it:** Jellyfin's Quick Connect keeps codes in memory with a sweep; the question was "what cleans up Portico's durable ones, and what does it leave behind?".
- **Confidence:** Confirmed by reading; a one-line runtime check (approve a code, do not collect, wait) would settle it.
- **Severity:** Medium (silent; grows into disk and secret-retention problems rather than an immediate breach).
- **Remediation direction:** Scan into a nullable type and treat NULL as "nothing to revoke", expire such rows, and log maintenance errors instead of discarding them. A test for "approved, never collected, then pruned" would have caught it.

### B-15.3 Behind a reverse proxy, the database-backed limiter treats all clients as one
- **What goes wrong:** `setupPeerSubject` returns the socket peer's address and never consults trusted proxies (`httpapi/onboarding.go:98-104`). It is the subject for the sign-in counter (20 per 10 minutes, `router.go:845`), Portico Account sign-in (30, `direct_identity.go:86`), setup, resume, TV-code creation (10, `onboarding.go:236`) and invitation preview/acceptance (10, `admin_access.go:152`, `foundation.go:313`). Behind nginx, Caddy, Traefik or a tunnel, every client's peer address is the proxy's.
- **Consequence:** A household of normal size can hit "rate_limited" for ten minutes on its own, and anyone on the internet can keep sign-in, TV-code creation and invitation acceptance refused for everyone by sending a few dozen junk requests every ten minutes.
- **How the comparison surfaced it:** Jellyfin's CVE-2025-32012 was about trusting forwarded headers too much; this is the mirror image, trusting them too little in one place while the sibling limiter (`rate.go:63-67`) handles it correctly.
- **Confidence:** Confirmed by reading.
- **Severity:** Medium (availability of sign-in on proxied deployments; no bypass).
- **Remediation direction:** Derive the subject from the same client-address function the in-memory limiter uses.

### B-15.4 An un-set-up server can be claimed by anyone who can reach it, and by a web page
- **What goes wrong:** `POST /v1/setup/browser` returns the setup token when the `Origin` header equals `scheme://Host` of the request (`httpapi/onboarding.go:132-164`). Two things follow. (1) By recorded decision there is no local-only restriction and no code, so until an owner exists, whoever reaches the port first becomes owner; a non-browser client simply sends a matching `Origin`. (2) There is no check that `Host` is a local name. A web page on a domain whose DNS answer is switched to the server's private address ("DNS rebinding") is then same-origin with the server from the browser's point of view, passes this check, and can complete setup from the browser of anyone on the LAN who has that page open. The helper that would prevent this, `localSetupHost`, with the comment "Host validation prevents a public DNS name rebound to a private peer from acquiring the token", is still in the file but no longer called (`httpapi/onboarding.go:77-90`; a search finds no caller).
- **Consequence:** Owner takeover of a fresh install. The owner can then add libraries pointing anywhere the server's user can read.
- **How the comparison surfaced it:** Emby 2023 (administrators without passwords) and the old Plex "unclaimed server" problem.
- **Confidence:** Confirmed by reading for the open window (deliberate). Probable for the rebinding path: the logic is plain, but browser behaviour (private-network access rules in current Chromium) may block some variants and was not tested.
- **Severity:** Medium overall; High for a server that is port-forwarded or left running before setup.
- **Remediation direction:** Keep "no code" if wanted, but require the `Host` to be an IP address, `localhost`, `.local`, or the server's own `direct.getportico.tv` name; consider closing remote setup after a period of uptime (F-15.6). Update the stale comments.

### B-15.5 Live credentials and two-factor seeds are stored and backed up in the clear
- **What goes wrong:** By decision ("folder permissions are the protection, as in Plex", `identity/quick_connect.go:106-110`), these are plain database values: the last refresh response of every session family, containing that family's current access and refresh tokens, kept until the next rotation or the 90-day horizon (`identity/refresh.go:117-130`); playback renewal responses containing a live access token (`identity/families.go:376-382`); delivered TV grants and the setup receipt (until pruned, see B-15.2); TOTP seeds (`identity/two_factor.go:123`). Backups copy the whole database (`backup/backup.go:1-10`), and can be directed to another path (`PORTICO_BACKUP_DIR`).
- **Consequence:** The care taken to store only SHA-256 digests of tokens is undone for whichever tokens are current. Anyone who can read the database file or one recent backup holds working sessions for every device (subject to the installation id, which is in the same database, `identity_devices.installation_id`) and can generate second-factor codes forever.
- **How the comparison surfaced it:** Jellyfin stores access tokens plainly too; the question was whether Portico's hashing actually changes the outcome. It does for old tokens, not for current ones.
- **Confidence:** Confirmed by reading. **Severity:** Medium (needs file access; the same access already yields the password hashes and the server identity key). Deliberate.
- **Remediation direction:** F-15.8: leave these out of backups, revoke sessions on restore, and consider storing receipts so that they are useless without something only the device has (for example, seal each receipt with a key derived from the predecessor refresh token, which the server no longer holds in the clear and the retrying device still does). That last idea keeps "no server key" and "open data" intact.

### B-15.6 Three different readings of `X-Forwarded-For`
- **What goes wrong:** The in-memory limiter accepts the header only when it is exactly one address and otherwise falls back to the proxy's own address (`rate.go:194-237`). Locality reads it right to left, skipping trusted proxies (`request_locality.go:119-141`). The private-peer check accepts one address and consults only the environment's proxy list, not the owner's "trusted proxies" setting (`httpapi/onboarding.go:18-45` against `request_locality.go:38-46`).
- **Consequence:** With two proxies in front (a CDN plus nginx), or one proxy that appends rather than replaces the header, every client lands in the proxy's single limiter bucket: 20 authentication requests a minute for the whole world. A proxy trusted only through the settings page is honoured for locality but not for the recovery-owner rule.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (availability and consistency; I found no way to use the differences to appear local).
- **Remediation direction:** One function, used everywhere, with the right-to-left rule.

### B-15.7 Tokens are not bound to a device key, and the documentation says they are
- **What goes wrong:** See A-15.7. A refresh needs only values that travel in every request (`identity/refresh.go:22-24`, `:60-63`). Over plain HTTP a passive listener collects the access token, the next refresh token and the installation id.
- **Consequence:** Until the real device next refreshes, the listener has a full session; if the listener refreshes first, the real device's next attempt revokes the family for both. A patient listener who captures the password at the next sign-in has everything.
- **Confidence:** Confirmed by reading. **Severity:** Medium on HTTP over untrusted networks; Low on a home LAN. The transport choice is deliberate; the gap between `02 Server.md` ("a device registers a key") and the code is not.
- **Remediation direction:** Correct the document now; decide on F-15.2.

### B-15.8 The server's own web app is refused behind a TLS-terminating proxy unless an environment variable is set
- **What goes wrong:** `sameRequestOrigin` builds the expected origin from `r.TLS` (`httpapi/onboarding.go:91-97`), not from the trusted proxy's `X-Forwarded-Proto` that `secureTransport` already understands (`secure_connections.go:78-84`). A browser at `https://media.example.com` sends `Origin: https://media.example.com`; the server computes `http://media.example.com`; the request is answered 403 `origin_not_allowed` (`router.go:1248-1259`). Browsers attach `Origin` to same-origin POST, PUT, PATCH and DELETE, so sign-in fails.
- **Confidence:** Probable (depends on the proxy forwarding `Host` unchanged, which is the usual configuration). **Severity:** Low (fails closed; works once the origin is added to `PORTICO_ALLOWED_ORIGINS`, which no document mentions).
- **Remediation direction:** Use `secureTransport(r)` for the scheme; document reverse proxies (F-15.10).

### Design risks and deliberate trade-offs

### B-15.9 The second factor can be worn down by an attacker who already has the password
- **What:** Each password-step success yields a challenge good for five code attempts (`identity/two_factor.go:328-329`, `:378`). The cool-off is checked at the password step but not at the code step (`:381-383` records a probe only). Failures add at most a 60-second delay per source network. That is about five guesses a minute per source against three valid codes in a million: weeks from one address, about a day from fifty networks.
- **Confidence:** Confirmed by reading (arithmetic, not a test). **Severity:** Low to Medium. Follows directly from the no-lockout principle.
- **Remediation direction:** F-15.14.

### B-15.10 Two hashing slots serve every password check
- **What:** bcrypt work is limited to two at a time, and a third caller is told "busy" immediately rather than queued (`identity/service.go:125`; `direct_accounts.go:122-126`). The same two slots serve sign-in, password change, PIN checks and setup.
- **Consequence:** On a slow box, a distributed trickle of sign-in attempts (each address staying under 20 a minute) keeps both slots occupied and honest sign-ins receive 503 `authentication_busy`. It protects the server's CPU, which is the intent, at the cost of sign-in availability.
- **Confidence:** Confirmed by reading; the threshold depends on hardware. **Severity:** Low to Medium for internet-facing servers.
- **Remediation direction:** A short bounded wait instead of immediate refusal, and priority for requests from familiar devices (the device bucket already identifies them).

### B-15.11 "Private peer" can be wrong when something in front hides the client address
- **What:** If the server sits behind anything that does not add `X-Forwarded-For` (a TCP-level proxy, an SSH or VPN tunnel endpoint, Docker Desktop or rootless Docker port publishing, which present the gateway address), every remote client's peer address is private or loopback. Then `privateSetupPeer` and `requestIsRemote` say "local" (`httpapi/onboarding.go:18-45`, `request_locality.go:86-101`), and `loopbackClient` exempts the request from the HTTPS requirement (`secure_connections.go:97-103`, `:178-180`).
- **Consequence:** The recovery owner's password works from the internet; remote sign-in policy, remote bitrate cap and upload budget are not applied; "HTTPS required" is skipped. Nothing becomes passwordless, which is what separates this from Emby's incident.
- **Confidence:** Confirmed by reading for the logic; which deployments trigger it is a runtime fact. **Severity:** Medium for affected deployments.
- **Remediation direction:** Let the owner state "this server is behind NAT or a proxy that hides addresses: treat everyone as remote"; show in the connectivity page what address the server sees for the current request so the owner can tell.

### B-15.12 A stolen Hosted signing key vouches for any Portico Account until each server hears otherwise
- **What:** An identity assertion is accepted if it is signed by a key whose certificate chains to the root, is in date (up to 400 days, `hostedtrust/trust.go:22`), and is not on the server's remembered revocation list (`hosted/trust.go:59-108`). A server adds to that list only when it happens to verify an envelope carrying a newer certificate revision. The challenge and nonce stop replay of real assertions, not forging new ones with a stolen key.
- **Consequence:** Whoever holds an online "documents" key can sign in to any claimed server as any Portico-Account member, including an owner who uses a Portico Account, until that server next sees a legitimate envelope signed after the revocation. A server Hosted cannot wake (no port forward) learns only at its own next contact.
- **Confidence:** Confirmed by reading for the verification rule; how quickly revocations propagate depends on Hosted behaviour I did not read. **Severity:** High impact, low likelihood.
- **Remediation direction:** F-15.18 and F-15.13. Shorter certificate lifetimes would also shrink the window, against the "one renewal a year" decision recorded in the file.

### B-15.13 Approving a code makes the other device *you*, and codes are advertised on the LAN
- **What:** A device collects a session with the approver's own role; an owner's approval yields an owner session (`identity/quick_connect.go:540-588`). The preview the approver sees is the device's self-chosen name, platform and version (`:209-223`). TVs broadcast their current code over mDNS so phones can offer "nearby TV" (`Native/PorticoNetwork.swift:advertiseTV`).
- **Consequence:** Anyone who persuades an owner to enter or tap a code that belongs to the attacker's device gets an owner session. A device on the same network can advertise a convincing "Living Room TV". The Networking contract (§5) labels nearby results as untrusted, and a security notice is raised on approval.
- **Confidence:** Confirmed by reading; inherent to device-code sign-in (Jellyfin's Quick Connect is the same). **Severity:** Medium.
- **Remediation direction:** Offer "sign this device in as a viewer" as the default when an owner approves, with full role as an explicit choice; F-15.13.

### B-15.14 Remembered-account lookup is unauthenticated and keyed by a value sent in every request
- **What:** `GET /v1/auth/remembered-accounts?installationId=…` returns up to ten usernames, display names and last-used times for that installation (`identity_routes.go:170-181`, `identity/browser_accounts.go:106-123`). The installation id is a query parameter here and a header on every other request. The comment calls it "a 256-bit value"; the validator accepts any 32-plus-character identifier and the web app uses a UUID (`identity/devices.go:42`, `web/src/bridge/profile-trust.ts:6`).
- **Consequence:** Someone who has seen a browser's traffic (plain HTTP) or a proxy's access log learns who uses that browser. No credential is exposed.
- **Confidence:** Confirmed by reading. **Severity:** Low. Deliberate (stated in the handler).
- **Remediation direction:** Send the id in a header or body; fix the comment.

### B-15.15 Version and build fingerprint are public
- **What:** `GET /v1/system` returns `version`, `buildId`, `sourceDigest` and `builtAt` to anyone (`router.go:816-818`). The comment on the health routes says public endpoints "contain no account, build, route or dependency details" (`installation.go:17-18`), which this one contradicts.
- **Consequence:** Internet scanners can find servers on a version with a known flaw. This is exactly Jellyfin's `/System/Info/Public` complaint.
- **Confidence:** Confirmed by reading. **Severity:** Low. **Remediation:** F-15.20.

### B-15.21 Profile PIN attempts are counted per device, and a new sign-in is a new device
- **What:** The counter key is (profile, device) (`identity/direct_profiles.go:327`, `:368`). Signing in again without the installation header creates a fresh device record (`identity/issuing_device.go:46-51`), so someone who knows the account password can reset their own PIN delays at will; 10,000 PINs is a few hours at the sign-in rate limit.
- **Confidence:** Confirmed by reading. **Severity:** Low; the contract calls PINs "convenience locks, not account authentication". Worth knowing if PINs are presented to parents as the way to keep a child out of an adult profile on a shared account.
- **Remediation direction:** Also count per profile across devices, with its own cap.

### B-15.22 "Secure connections: required" has quiet exceptions
- **What:** It applies only when the server is claimed and its certificate is ready; otherwise HTTP continues with an owner alert (`secure_connections.go:122-175`). It never covers the static web app or loopback peers (`:178-180`). There is no redirect; an HTTP request is refused after its body, including a password, has already crossed the network.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (expectation gap). Deliberate ("must not lock the owner out").
- **Remediation direction:** F-15.7; have the web app refuse to submit credentials when it knows the mode is Required and the page is HTTP.

### Hardening items

### B-15.16 The main CSP allows scripts from all of `www.gstatic.com`
- **What:** `script-src 'self' https://www.gstatic.com` (`router.go:1242`, and the same in `deployment/hosted/Caddyfile:93`) exists for the Cast sender. That host also serves script libraries that CSP auditors list as bypass gadgets. The Cast receiver page already uses the narrower `https://www.gstatic.com/cast/` (`social_routes.go:966`).
- **Consequence:** None today, because there is no HTML injection point. It weakens the second line of defence.
- **Confidence:** Confirmed by reading. **Severity:** Low. **Remediation:** Restrict to the sender SDK's paths; add `form-action 'self'` to the server's policy (Hosted's has `form-action 'none'`).

### B-15.17 The release-feed fetch has no network policy
- **What:** `FetchReleaseFeed` uses a default HTTP client on an owner-set URL, follows up to three redirects to any http/https host, and has no private-address or metadata-service guard (`administration/update_feed.go:23-58`). The feed's `url` field is then rendered as a link in the owner console (`web/src/screens/server/Maintenance.tsx:134`).
- **Consequence:** Owner-only blind request to internal addresses; inconsistent with the care taken elsewhere. **Confidence:** Confirmed by reading. **Severity:** Low. **Remediation:** Route it through the `remotemedia` policy and validate the link scheme server-side.

### B-15.18 The folder picker checks the path before resolving symlinks, not after
- **What:** `systemPath` and `policy.CheckBrowse` run on the cleaned path; `EvalSymlinks` then replaces it and the listing proceeds without re-checking (`administration/filesystem.go:137-149`).
- **Consequence:** With configured media roots, a symlink inside a root that points outside it (or into `/proc`) can be listed. Owner-only. **Confidence:** Probable. **Severity:** Low. **Remediation:** Re-run both checks on the resolved path.

### B-15.19 Supply-chain loose ends
- **What:** `server-trust.yml` and the first job of `ci.yml` use `actions/checkout@v4` / `setup-go@v5` by tag while every other workflow pins commits (`server-trust.yml:10-11`, `ci.yml:14`); Docker base images are tags, not digests (`Dockerfile:30`, `:51`, `:63`); CI has secret scanning and CodeQL but no `govulncheck` or `npm audit` step (Dependabot is configured); installers are unsigned (`release.yml:234`). `build-server-release.py:35-43` creates a signing key if none exists, which is right for development and worth a guard in release mode (the workflow does check that the key is configured, `release.yml:53`).
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-15.20 The built-in provider credentials are a shared dependency
- **What:** Every server uses the same TMDB token and TheTVDB key (`server/internal/metadata/project.go:4`, `tvdb_project.go:5`). If either provider revokes or throttles it, metadata stops fleet-wide until a new release ships; anyone can extract and misuse them, which makes that more likely.
- **Confidence:** Confirmed by reading. **Severity:** Low (availability). Deliberate. **Remediation:** Expose both overrides in the owner console, not only by environment; plan how a replacement key reaches servers without a release.

### B-15.23 TV-code and invitation routes sit outside the cheap limiter
- **What:** The in-memory limiter covers `/v1/direct/`, `/v1/auth/`, `/v1/sessions`, `/v1/setup` and two Cast paths (`router.go:1298`). `POST /v1/quick-connect…` and `/v1/access/invitations/…` rely only on the database counter, which costs a top-priority write transaction per request before it can refuse (`identity/quick_connect.go:117-132`).
- **Consequence:** Anonymous floods on those paths put load on the single writer. **Confidence:** Confirmed by reading; the impact needs measuring. **Severity:** Low. **Remediation:** Add both prefixes to the limiter's list.

### B-15.24 Small output-encoding nits
- **What:** `Content-Disposition` is built by concatenating a file name inside quotes without escaping (`httpapi/downloads_routes.go:223`); Go removes line breaks from header values, so this is at worst a malformed header if a name contains a quote. The play-history CSV export (`telemetry_routes.go:99`, unused by current clients per the Backlog) writes titles and names that a spreadsheet may interpret as formulas.
- **Confidence:** Speculative (neither traced to an input an attacker controls). **Severity:** Low.

## Where Portico is stronger

Worth keeping, and worth saying publicly once the confirmed items are fixed:

1. **Nothing in the library is reachable without a credential.** No public streams, images, subtitles or user list (A-15.2 to A-15.5).
2. **The transcoder is sandboxed and never handed a user-controlled string or a library path** (A-15.22, A-15.23). No other server in this comparison does either.
3. **No plugins, no configurable tool paths, no scripts.** An administrator's browser being tricked does not become code on the server (A-15.24).
4. **A single outbound-fetch policy** with metadata-service blocking, owner-confirmed LAN targets and pinned addresses (A-15.25).
5. **A web app with zero HTML-injection sinks**, a strict CSP, and credentialed image fetches that refuse cross-origin and redirects (A-15.18, A-15.21).
6. **Short-lived tokens with rotation and family revocation**, re-checked while bytes are flowing (A-15.6).
7. **Restriction enforcement proven route by route** by a test that fails when a new route is unclassified (A-15.30).
8. **Enumeration-resistant sign-in** and owner recovery only from the server's console (A-15.11, A-15.15).
9. **Hosted cannot mint server sessions**, and its signing keys hang off an offline root that release builds must contain (A-15.33, A-15.34).
10. **Logs carry route patterns, not URLs**, and uploaded client logs are scrubbed (A-15.40).
11. **Native apps use the platform secret stores**, forbid backup of credentials, validate deep links by grammar and contain no WebView (A-15.45 to A-15.47).
12. **A release pipeline with pinned actions, secret scanning, static analysis, SBOM, provenance and a signed manifest** (A-15.48).

## Doc/code disagreements

| Where the document says | What the code does |
|---|---|
| `02 Server.md`, Identity and access: "a device registers a key" | No device key exists on the home server; binding is the refresh token plus a random installation id (`identity/refresh.go:22-24`). |
| `android/…/data_extraction_rules.xml` comment: sign-ins are "bound to this installation and its Keystore key" | The Keystore key encrypts local storage only (`PorticoRecordStore.kt:22-30`); the server never sees it. |
| Identity contract §4.1: password needs upper, lower and a digit or symbol | Only length 8 to 72 is enforced, by stated decision (`identity/direct_accounts.go:83-89`). |
| Identity contract §9: profile PINs use "bcrypt cost 10" | Cost 8 (`identity/hash_cost.go:10`). |
| Identity contract §5: browser sessions use "Secure, HttpOnly, host-scoped renewal cookies"; bearer credentials must not persist in localStorage | True for Hosted. The home server sets no cookies; the web app persists the refresh token in IndexedDB, sealed on HTTPS and plain on HTTP (`web/src/bridge/server-connections.ts:6-10`). |
| Identity contract §9: refresh, setup and bootstrap secrets "never appear in URLs" | Top Shelf artwork carries a grant in the query string (`httpapi/topshelf.go:137`). Narrow and deliberate, but not "never". |
| Identity contract §9: PIN "attempt limits, delay, and lockout" | Delays exist but are per device and reset by a new sign-in (B-15.21). |
| Setting text: "Who may sign in from outside this network" (`operations/settings.go:96`) | Enforced on one legacy route only (B-15.1). |
| `httpapi/onboarding.go:77-78` and `identity/onboarding.go` (`BrowserSetupToken` comment: "only exposed by the local, origin-checked first-run route") | The route is no longer local-only and the Host check is unused (B-15.4). `sameMachineSetup` and `localSetupHost` are dead code. |
| `httpapi/installation.go:17-18`: public endpoints "contain no account, build, route or dependency details" | `GET /v1/system` returns version, build id and source digest (B-15.15). |
| `identity/quick_connect.go:298` comment: "Approval, family issuance and recoverable encrypted delivery are ONE commit" | Issuance moved to collection, and nothing is encrypted; `sealSetup` returns plain JSON (`:106-110`). The stale assumption is the root of B-15.2. `PruneAuthorizationRenewals`'s comment also still says "encrypted responses". |
| `identity/browser_accounts.go` comment: an installation id is "a 256-bit value" | Any identifier of 32 or more characters is accepted; the web app uses a UUID. |
| Networking contract §9: a 45-minute presence lease renewed about every 30 minutes | `01 Overview.md` and `07 Hosted Services.md` say "no periodic chatter". The two documents disagree with each other; I did not check which the code follows. This matters for B-15.12. |
| Networking contract §13: "Forwarded headers are trusted only from configured proxy CIDRs" | True, but one limiter ignores them entirely and three parsers disagree (B-15.3, B-15.6). |
| `01 Overview.md` diagram: "Hosted (api.getportico.tv)" | The Networking contract (§2) calls that name a legacy alias nothing uses. |

## Open questions for Justin

1. **Remote sign-in policy (B-15.1):** Should `off` and `owner-only` also block Portico Account sign-ins, TV-code collection and invitation acceptance from outside, or only password sign-in? The fix depends on the answer.
2. **First-run setup (B-15.4):** Is "any browser, even remotely" still wanted now that it also means "any web page a LAN user has open"? Would a Host check plus a time limit on remote setup be acceptable?
3. **Plain HTTP over the internet (A-15.36, B-15.7):** Is "first-class" meant to cover a browser at a public IP address, or only the LAN? If only the LAN, the server could say so to the person signing in.
4. **Device keys (F-15.2):** Do you want real proof of possession, at least on refresh, or should the documents simply stop claiming it?
5. **Secrets at rest (B-15.5):** Are you comfortable with backups containing working sessions and two-factor seeds? Is "a restore signs everyone out" acceptable as the price of fixing that without encryption?
6. **No lockout (A-15.10, B-15.9, B-15.10):** Would an opt-in stricter mode that only ever delays *unfamiliar* devices fit the principle?
7. **TV codes (B-15.13):** Should an owner approving a TV create an owner session on that TV by default (decided 2 Oct per the Backlog), or a viewer session unless the owner asks for more?
8. **Device approval (F-15.13):** Should owner and admin sign-ins on a new device need a second, local approval by default?
9. **Hosted key lifetime (B-15.12):** Is one renewal a year still the right balance once there are real users, given what a stolen key allows?
10. **Reverse proxies (F-15.10):** Is running behind a proxy a supported configuration for release? If yes, B-15.3, B-15.6 and B-15.8 become release blockers for that configuration.
11. **Version disclosure (B-15.15):** Does any client need `version` or `buildId` before sign-in?
12. **Sandbox on Docker (F-15.19):** Should the image documentation tell owners the flags under which the sandbox works, or should the default image ship a profile?
13. **Automatic port mapping:** `internal/networking/mapping_upnp.go` can open a router port. I did not determine whether it is on by default; if it is, a fresh server may be internet-reachable over HTTP before setup, which raises B-15.4.

## Evidence boundary

- **Nothing was executed.** No server, test, build, fuzzer or exploit was run; no request was sent to any Portico host. Every finding is a reading of source and configuration in the working tree as of 11 October 2026.
- **Route authentication:** every raw route registration was script-classified and the unclassified ones read by hand, but this is not a proof. F-15.1 is the proof.
- **Runtime-dependent claims** are marked in each entry: browser behaviour for DNS rebinding and `Origin` headers (B-15.4, B-15.8), which deployments hide client addresses (B-15.11), hashing throughput on slow hardware (B-15.10), the `database/sql` NULL scan (B-15.2, which I am confident of but did not run), revocation propagation from Hosted (B-15.12), whether Apple's local-networking exception admits public IP literals over HTTP (A-15.46).
- **Not reviewed:** Hosted services and DNS source; Watch Together, remote control and Cast (the Jellyfin "session remote-control" advisory should be asked of domain 13); download receipts; rclone and mount configuration; subtitle parsers line by line; Vega and Roku clients; JavaScript-side deep-link handling; third-party dependency versions against current advisories; the FFmpeg bundle's build flags; deployed infrastructure (Cloudflare, Caddy beyond the checked-in CSP, firewall).
- **Jellyfin CVE numbers before 2026** are from memory of the public record and should be checked before being quoted. The 2026 advisory ids and the summary of issue #5415 were read from GitHub on the audit date. Jellyfin code facts are from the October 2026 clone. Plex and Emby statements are from public documentation and reporting, not code.
- **Comparisons are not endorsements.** Where Jellyfin does something differently (per-user lockout, PBKDF2, public system info) it is recorded as a difference, not as the correct answer.

## Outside my domain

- **Images as scripted fetches (A-15.3)** have performance and memory consequences for large grids on web and TV; domains 06 and 09 should weigh that against the privacy gain.
- **No MKV attachment extraction (A-15.27)** is safe, but may mean embedded subtitle fonts are not used; domain 04.
- **The Backlog item "Two Apple sign-ins ended without an obvious cause"** logs `reason=explicit_signout`. That reason is also written by code paths that are not a person signing out: `retireUnreturnedLogin` on the legacy `POST /v1/sessions` route (`identity/service.go:187-200`), TV-code cancellation and the TV-code pruner (`identity/quick_connect.go:421-429`), and Cast family retirement (`identity/direct_accounts.go:394-399`). Whoever investigates should look at which of those ran, rather than assume a client called sign-out.


