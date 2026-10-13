# Third-party API acceptance

## Purpose and evidence

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Private local candidate server/web, owner/member/restricted accounts and scoped keys, private curl configs, recording webhook/redirect listener, actual MCP desktop client, optional isolated Sonarr/Radarr and format21 Hosted+OS callback receiver. Linux first then Windows/macOS/network paths. Large-library fixture for cursor/index cost and real tuner for recording event wiring.

## Media and accounts

For ordinary playback, make synthetic samples with the candidate's qualified FFmpeg/ffprobe pair. Run in a new writable fixture folder (never over existing media):

```sh
ffmpeg -f lavfi -i testsrc2=size=1280x720:rate=24 -f lavfi -i sine=frequency=440:sample_rate=48000 -t 19 -c:v libx264 -g 96 -keyint_min 96 -sc_threshold 0 -c:a aac -movflags +faststart 'Fixture (2026).mp4'
ffmpeg -i 'Fixture (2026).mp4' -c copy 'Fixture (2026).mkv'
ffmpeg -f lavfi -i sine=frequency=440:sample_rate=44100 -t 120 -c:a flac -metadata artist='Fixture Artist' -metadata album='Fixture Album' -metadata title='Track 01' -metadata track=1 'Track 01.flac'
```

Make three TV copies named `Fixture Show/Season 1/Fixture Show - S01E01.mkv` through `S01E03.mkv`; make a second version/edition copy and a two-part film. Write a UTF-8 sidecar `Fixture (2026).en.srt` containing `1`, `00:00:02,000 --> 00:00:05,000`, and `Fixture caption`, separated by newlines and followed by a blank line. For long buffering/background/download cases generate a 20-minute/2+GB synthetic file by raising duration/bitrate and verify its hash before testing; a 19-second clip cannot establish those cases.

For HDR10/HLG/Dolby Vision 5/7/8, TrueHD/DTS-HD/Atmos, PGS/VobSub/DVB, HE-AAC/SBR and attached-font ASS use legally held test clips/disc or broadcast extracts. Log provenance, ffprobe facts and a correctly rendered reference frame/audio/channel map. SDR bars with HDR tags are not HDR-fidelity evidence. The server checkout has `fixtures/audio-gapless/` and `fixtures/audio-gapless/generate-portable.py`; preserve its original corpus and generate extras with `python3 fixtures/audio-gapless/generate-portable.py --out /tmp/portico-audio-extra`. Obtain measured HE-AAC/PS fixtures separately. Missing real formats block their cases. Make bad-input copies separately: truncate a copy, corrupt a metadata row, or insert a malformed subtitle cue between good cues. Record which data changed.

## Build or install

Install the verified candidate local server with the same production web and a private fixture library. First sign in as direct owner and open Settings > Server > Integrations > API keys. Create separate Read, Library, Playback and Admin keys plus a member personal token; copy each secret once into a mode0600 private curl config, never into shell history/shared logs. Example private config has `header = "Authorization: Bearer ACTUAL_SECRET"`; subsequent `curl --config /private/read.curl URL` hides the secret from command-line arguments. Origin is the server’s actual reachable HTTP/HTTPS address. Use current server/api/portico.openapi.json operation request/response schemas and server/api/guide/07-keys-tokens-and-scopes.md, 08-for-tools.md, 09-webhooks.md, 10-mcp.md, 02-authentication.md and generated routes; no stable legacy API compatibility. Inspect response status, ETag/revision, cookies, MIME/body and readback, not HTTP receipt alone.

For Sonarr/Radarr use isolated instances, lawful imported synthetic fixture and Custom Script, not their webhook payload. Use private script from server/api/guide/08-for-tools.md with sonarr_series_path/radarr_movie_path mapped to the Portico-visible absolute path and a Library key file. For webhook receiver run a private standard-library HTTP server that records raw body bytes/headers to a private directory and returns configured2xx/503/redirect; do not use a one-shot nc listener as retry acceptance. Verify raw HMAC of timestamp + '.' + original bytes with server/api/guide/09-webhooks.md’s Python/JavaScript verifier (≤300s timestamp tolerance and duplicate/malformed-header rejection). Receiver deduplicates Portico-Event-Id. Register actual receiver IP/port privately; allow only selected test traffic.

For MCP use a desktop Streamable HTTP client with origin/v1/mcp and private bearer config. POST JSON-RPC initialize with protocolVersion2025-06-18, capabilities{}, clientInfo{name:'Acceptance',version:'1'}, then tools/list and tools/call; GET is405, notifications accepted202, signed-in app sessions refused. A missing key, insufficient_scope or session_required is a contract/authority result; do not promote scopes globally to hide it. Code-flow sign-in additionally needs the isolated format21 Hosted instance and an OS callback receiver; absent real callback/provider/MFA is Blocked.

## Smoke subset

Run cases 1, 2, 4, 6, 7 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. Scoped keys, member tokens, revocation and password independence — smoke

1. POST /v1/admin/api-keys as owner with {"operationId":"accept-key-0001","name":"Acceptance Read","scopes":["read"]}; require201 once-only pk. secret. With private Read config GET /v1/libraries, POST /v1/libraries/ID/scans and GET /v1/admin/api-keys.
2. Use owner revoke key then repeat read; on separate valid key change owner password/sign-out-everywhere. Create member pat., disable/delete member and revoke/alter library/role while requests active.
3. Set apiRequestsPerMinute30 and issue31 quick requests; compare unaffected human owner session. Use once, idle>minute/list/restart and inspect last-used.

**Expected:** Read200, scan403 insufficient_scope, key-list403 session_required, revoked401. Password/session sign-out does not revoke owner integration key; disabled/deleted/live-restricted member authority ends correctly. 31st429/Retry-After, owner console unaffected, last-used durable.

**Failure:** Scope bypass, API key administered by itself, owner key tied to browser sign-out, stale member authority, missing Retry-After or lost last-used.

**Route to:** api-platform, review-api-platform, console, server-followups.

### 2. Paths changed, immediate missing and real manager import — smoke

1. Add file in fixture film folder then POST /v1/libraries/paths-changed with {"paths":[{"path":"ABSOLUTE_SERVER_FILE","change":"added"}]} using Library key; read202 jobs then scanner completion/title.
2. Delete a season folder/file in unchanged readable root and send removed. Repeat while scan running, unavailable/retargeted/unreadable root, unknown/outside path/private recording/network share; inspect limited inventory directories.
3. Configure actual Sonarr/Radarr Settings > Connect > Custom Script for Test/import/upgrade/delete, map host/container paths and import/delete fixture. Windows backslash paths as server sees them, macOS mount paths separately.

**Expected:** Status queued/expected folder, one bounded incremental job/only targeted parent; proven named deletion missing before scan and counted once, unproven root not missing. Actual managers send accepted mapped paths, Test handles its special event without unintended scan; no full-library rescan.

**Failure:** Wrong-machine path scans all, duplicate jobs/missing count, outside/private source touched, manager webhook payload mistaken for paths contract or accepted202 never completes.

**Route to:** api-platform, review-api-platform, scan-loop, identity, server-followups.

### 3. Bounded title query, cursors and privacy

1. With Read key GET /v1/titles?library=ID&kind=movie&sort=title&order=asc&limit=100; follow nextCursor until absent, repeat title/year/added both directions and equal-key ties, provider=tmdb&providerId=603 lookup.
2. During scan/metadata edit page late/deep results by stable IDs; use restricted token and another profile’s cursor. Send repeated/unknown query, limit0/101/fractional and range on wrong sort.
3. On large library capture actual query plan and per-page latency/SQL slots, compare early/late cursor pages without materializing entire library.

**Expected:** Indexed bounded seek/page, no unchanged-key duplicates/skips, restrictions before paging, query/caller cursor fence and expiry30min. Unknown/malformed400, actual source matches emitted types/UTC dates/millis.

**Failure:** Whole catalogue scan for late cursor, hidden titles/context, cursor crosses profile/query, nondeterministic equal-key paging or invalid bounds accepted.

**Route to:** api-platform, review-api-platform, search-sort, grouping-sorts, registry.

### 4. Signed webhooks, retry ordering and independent receivers — smoke

1. Create webhook for playback.started/paused/resumed/stopped, scan.finished/member.signed_in; start/pause/resume/quality/stop/scan/sign-in and verify raw signature/IDs/attempt headers/payload secrecy.
2. Make receiverA fail with65+ backlog, B healthy; restoreA after first retry and inspect same ID/order, start before stop. Return redirect and capture target; tamper body/header/duplicate timestamp/stale timestamp/wrong secret.
3. Disable/rotate/remove webhook and fill1000 waiting events via large first import/title.added subscription; inspect dropped/log retention and scan cost.

**Expected:** Local server delivers expected events (quality no stop/start), valid HMAC/≤64KiB body/no secrets; B independent, A retries stable ID in order, redirect never followed. Signature checker refuses tamper/stale/ambiguous headers, disable abandons regular queue, bounded1000 queue/dropped and200 finished log.

**Failure:** Unsigned/re-serialized body accepted, healthy receiver starved, stop overtakes failed start, new ID each retry, credential redirect or unbounded backlog.

**Route to:** api-platform, review-api-platform, social, server-docs.

### 5. Webhook event wiring beyond playback

1. Subscribe recording.finished/failed, storage.low, backup.succeeded/failed, update.available, server.started. Make actual tuner recording success/failure, fill only disposable volume below warning level, run backup/failure, restart.
2. Use controlled valid current/newer release feed, allow first check~2min and six-hour schedule or focused timer fixture; inspect once per new version. Duplicate event receiver handling uses Event-Id.

**Expected:** Wiring produces correct event/reason without private recording title.* leaks; update notification only, no auto-install; retry/dedup event identity stable and storage warning honest.

**Failure:** Handler tests offered as actual event proof, unbounded repeated update, no recording signal or secrets/user private recording exposed.

**Route to:** api-platform, livetv, ops-packaging, limits-settings.

### 6. MCP actual client and role-filtered tools — smoke

1. Initialize/list using Read key, call list_libraries/search; confirm start_library_scan absent. Repeat Library/Admin/Playback scopes/member tokens and app-session rejection.
2. Use tv_guide({}), start-only/end-only valid window and explicit invalid window; call read_logs(category=playback,limit=10), owner metadata-attention/status/download-usage tools; member cannot read owner data.
3. Try fractional limits/unknown tool params/body bounds; issue administrator playback control while preparing/paused and inspect player update/receipt no URLs; test own-profile remote view restrictions.

**Expected:** Real client works with exact scope vocabulary, bounded validated tools/routes, guide published defaults and owner boundary. Controls reach actual player, paused load silent; receipt hides media grant.

**Failure:** Tool list leaks action authority, parameter bypass, incorrect guide default or admin command only changes stored document.

**Route to:** api-platform, review-api-platform, server-followups, registry.

### 7. Outside app code-flow sign-in — smoke

1. On isolated Hosted issue local signed server challenge, generate app nonce/state and PKCE-S256 verifier/challenge in callback receiver, open returned authorization URL. Use exact registered HTTPS/loopback/custom-scheme redirect.
2. Sign in/password/provider/MFA, review Allow or Cancel then receive only code/state, exchange once and submit assertion to local server membership/profile admission.
3. Change state/verifier/URI/account/family after review, replay/expire code, race12 exchanges, click/hold Continue during503/date-valued Retry-After.

**Expected:** Code flow preserves exact callback/state/challenge/PKCE and explicit consent; one exchange winner, wrong verifier/URI no consume; only identity assertion, local membership/MFA/role remains local. Cancel denial only, retry floor honored.

**Failure:** Bearer in callback, Hosted bypasses local access, automatic held-key consent, code replay/redirect mismatch or unqualified OS handler claimed pass.

**Route to:** hosted, review-hosted, hosted-followups, accounts-security.

### 8. Capabilities and playback for external client

1. Publish actual v2 native/mpv or fMP4-only/noTS capabilities via PUT /v1/me/devices/current/capabilities, GET readback then start session with idempotency key and current contract using guide04 sample.
2. Play VP9 converted H264/HEVC fMP4 with alternate AAC/AC3/EAC3 tracks, ten-bitH264/quadAAC supported-copy case and HEVC/HDR/lossless96k24bit7.1 profile. PATCH quality/audio/subtitle with If-Match at12s.
3. Send invalid track/stale revision/decode-failure once; test playing remote mpv controlled by home/native browser with upload/conversion ceiling and one burn-in slot.

**Expected:** One capability document/no translation, executable target matches supported codec/depth/range/audio/transport; every fMP4 rendition map/m4s decodes. Same session/queue/generation/position on valid replan, invalid preserves accepted media, playing device engine/locality/current ceiling controls.

**Failure:** Producer cannot serve admitted profile, TS-only alternate audio, silent stereo/SDR default, controller changes away-player engine/ceiling or rejected plan destroys old progress.

**Route to:** playback-core, review-playback-core, planner-wiring, review-playback-pipeline.

### 9. Media/subtitle grants, events and hostile input

1. From active session privately exercise original/subtitle/font/art GET/HEAD/Range/416/If-None-Match; verify container MIME for mkv/webm/mov/avi/ts and exact authored ASS/VobSub companions.
2. Stop session/replace source/revoke access then retry conditional/read plus another-profile/source/resource address. Consume /v1/events with opening cursor/Last-Event-ID, overlap409/reconnect and long poll; expire cursor retention.
3. Run only current scope/lane/router/generated-example guards, authenticated hostile-input canary and small controlled bogus-key burst; measure SQL acquisitions/console latency.

**Expected:** Grant/source/auth scope checked even304/active chunks; mapped MIME/timing preserved. Opening feed position not lost, overlap recovery/cursor reset stable, private audiences isolated. Raw credentials cheaply bounded before SQL and human console remains reachable.

**Failure:** 304 escapes revoked grant, wrong MIME breaks direct play, event gap/private leak, hostile test silently only401 after sign-out or bogus keys occupy unbounded SQL.

**Route to:** subtitles-text, registry, freshness-server, review-api-platform, leftovers-server-b.

### 10. Recoverable integration authority after backup

1. Create scoped integration key/webhook, backup and restore whole directory on isolated server, reauthenticate owner, inspect restored keys/secrets privately and review/revoke.
2. Retry old download grant/receipt/exact progress occurrence after restore/library split/merge and revoked member/library access.

**Expected:** People signed out once, integration identity/custody preserved per backed-up configuration and explicitly reviewable; revoked scopes/library authority refuse, durable receipts no duplicate personal state.

**Failure:** All keys silently replaced, live person sessions restored, saved grant reads wrong post-merge source or receipt bypasses withdrawal.

**Route to:** ops-packaging, accounts-security, downloads, server-followups.

### 11. Generated paths, nullable unions and analysis job replies

1. After native package sync call current route builders with raw.., encoded%2e%2e, pre-encoded delimiters/control/malformed percent; also valid Unicode/slash/percent/punctuation intended as a SINGLE segment. Inspect dispatch count.
2. Decode device subtitle command string/null and profile library policy list/null; reject number/object/wrong-list types while retaining additive/optional response tolerance.
3. On actual candidate GET /v1/items/ID/analysis with advertised sourceId/sourceRevision/mappingRevision/sessionId/generation/positionUS, POST analysis/jobs, control pause/resume/cancel/retry; library lyrics fetch. Compare served status/body with generated union schemas and owner/member/restricted/absent boundaries.
4. During actual startup without DB request readiness; require503 {ready:false,status:starting}, Retry-After2 then200 when ready.

**Expected:** Unsafe path rejected before transport; valid raw segment exactly once encoded, current grant-bound subtitle/font routes still accepted. Meaningful null retained, branch validation exact; queue/retry202job and pause/resume/cancel200status, owner mutation authority unchanged; lyrics202scheduler rejects undeclared query parameters.

**Failure:** Destination-changing builder sends request, null dropped/coerced, wrong job/status branch loses fields, member mutates analysis or readiness503 forced into ordinary error parser.

**Route to:** review-registry, registry, native-health.

### 12. Natural credential expiry, guide images and diagnostics

1. Warm credential cache under short valid test expiry then let it expire WITHOUT any DB write; use Read/Playback keys against admin listing/control/terminate and administrative MCP, plus own allowed controls/events.
2. Refresh guide with shared source/one allowed and one over-ceiling programme; request projected programme-bound images, withdraw ratings/source access then switch active generation and retry known URLs.
3. Send test error/report with FAKE short named passwords/factor seeds/auth-cookie headers/signed URL params; inspect downloaded report/rotating logs/service stderr and readable ordinary title/status information.

**Expected:** Calls reject exactly at natural expiry, scope-admin403/hidden tools while personal allowed controls remain; read events carry no owner outbox. Allowed image loads, hidden/stale generation/withdrawn URLs fail, no raw external URL/retired digest. Marked fake secrets redact in every diagnostic channel.

**Failure:** Cache survives expiry, playback/read gains admin access, image URL leaks forbidden programme or short secret survives downloaded/stderr log.

**Route to:** security-sweep, api-platform, accounts-security, livetv, console.
