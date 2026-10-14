# Vega acceptance

## Purpose and evidence

Updated for the evening implementation and owner decisions. New cases remain qualification work; known missing candidate implementation is Fail, while an unavailable test prerequisite is Blocked. Native-owner recorded 655 native/87 Vega before the mirror; the later running log confirms 655/87 again after the mirror with clean type checks and equal packages. Physical acceptance remains unrun.

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Vega SDK0.24/CLI on PATH and Node22+, Vega OS1.2 physical Fire TV in developer mode (virtual device only build/navigation evidence). Matching candidate native repository and private server/phone; remote, HDR/source fixtures and optional debug trace sink. Authenticated SDK transport is restored with exact-origin credential binding and observed-redirect containment. The owner accepts pre-follow disclosure and invisible return-chain risk. These device cases are required; there has been no physical Vega acceptance.

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

On a qualification copy check synchronized packages from native root, then `cd apps/vega`, `npm ci --legacy-peer-deps` (use the committed lockfile), `npx tsc --noEmit`, and `node scripts/vega.mjs run --release`. If no lockfile in a specifically reviewed candidate, use its recorded npm install --legacy-peer-deps procedure and record resolved inputs. Packages appear under build/{aarch64,armv7,x86_64}-release/*.vpkg; device list must select the actual target architecture. Use `node scripts/vega.mjs device status`, `logs 200`, `key left select` and `shot /private/evidence.png`. Path-space problems require PORTICO_VEGA_STAGE pointing at a writable path without spaces; stage/mirror through the wrapper. Missing SDK/adaptor/package/ABI is a build prerequisite, not a transport fix. For physical renderer traces only set debugConsole=true/debugSink privately in app.json and `node scripts/debug-sink.mjs`; virtual screenshots may stop updating once video draws. Restore production flags before packaging.

The bridge checks observed response addresses before admitting headers/body/chunks, retires exposed families, clears playback/captions, signs out and attempts issuer-only revocation. The SDK cannot refuse redirects before transmission. Use synthetic credentials at owned capture origins; do not require zero captured secrets or claim an invisible chain is detected. Ordinary offline failures retain sign-in. Opaque direct media is removed; supported MP4/MKV uses observable HLS, with plain remux where possible. TV downloads/mpv/speed/gapless/crossfade/leveling/HDR/surround controls stay hidden.

## Smoke subset

Run cases 1 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. First build and restored authentication — smoke

1. Build/install Release with SDK 0.24 on actual Vega OS 1.2 Fire TV; cold/warm launch without Metro. Sign in by password over trusted HTTPS and remote plain HTTP; select a PIN profile, renew/refresh and restore the remembered session after restart.
2. Use local Quick Connect and Hosted TV approval separately, inspect actual receiving name/platform/role and receive events. A fresh signed same-server alternate-route proof may authorize that exact origin; a different server at a remembered address receives no secret.
3. Exercise ordinary offline timeout and invalid TLS. Read the single quiet About line; a normal outage preserves sign-in rather than showing credential exposure.

**Expected:** Real sign-in/control/refresh/streams work with existing identity/TLS/bounds; HTTP is permitted remotely. Physical package/SDK results recorded separately from the 87 bridge tests.

**Failure:** Credentialed requests still blanket-refused, wrong remembered identity admitted, untrusted TLS accepted or ordinary outage retires session.

**Route to:** vega-transport, vega, native-sync.

### 2. Code, profile and origin-gate behavior

1. Open TV code, approve from phone; require device approval and view waiting states. Compare local Quick Connect and Hosted activation.
2. Open the Hosted origin gate for 120 seconds from a sibling service. At 30/60/90 seconds verify zero automatic activation requests. Keep valid code until local expiry; explicit actions stay interactive and consumed grant receipts survive restart.
3. Immediately sign in after claim installation/acknowledgment without owner-link wait or manual wake; retain pre-existing owner links.

**Expected:** Correct identity/role, prompt admission and origin-wide backoff; code and durable redemption/cancellation semantics survive outages.

**Failure:** Wrong role/name, delayed admission or automatic gate bypass.

**Route to:** claim-and-gate, vega-transport.

### 3. Observable HLS, quality and lifecycle

1. Play supported HTTPS MP4 and HTTP MKV, unsupported codecs, music and live channels. Inspect plan: no opaque direct media; supported picture codec survives plain HLS remux where possible, SDR/stereo declared truthfully.
2. Start Original; choose fixed/Automatic, seek across buffered and unbuffered windows, background/resume and hold/cancel opening streams. Delayed/clamped seek acknowledges actual landed position; retired bindings/rate commands cannot move current play.
3. Change route/title/profile while responses are delayed. Play next episode and verify no duplicate/unbounded segment fetch; playlists are at most 1 MiB and segments 32 MiB.

**Expected:** Current-source observable playback matches declared limits, finite seek/cancel settlement, state/privacy fence and same-queue quality change.

**Failure:** Opaque direct opening, false HDR/surround claim, stuck seek/open, retired command wins or bytes exceed bounds.

**Route to:** vega-transport, review-native-js, vega.

### 4. Subtitles, Together and Play on

1. Use SRT/offset/Off, ASS/PGS supported burn-in; fetch subtitle grants with 304 and failure recovery. Redirect a playlist/segment/caption: none of that response’s bytes may render.
2. Load into a paused Together room before first output, then seek and hand off to an actual second device. Play on controls compare to receiving position/state and server-listed authority.
3. Tune live across programme boundaries and the retained window. Check truthful hidden unsupported settings and remote focus.

**Expected:** Usable supported captions/live/Together/remote control with no redirected bytes rendered; initial paused load silent.

**Failure:** Caption leak, first-frame sound, live freeze, wrong target control or unsupported option shown.

**Route to:** native-screens, vega-transport, social, vega.

### 5. Redirect exposure and durable containment

1. With synthetic credentials and two owned recording origins, return 301/302/303/307/308 for password, refresh, profile proof, pairing, control, events, playlist/segment and subtitle. Repeat cross-origin final 401/500 and same-origin redirected path. Record precisely what the SDK forwards; do not use real credentials.
2. Require response rejection before caller headers/chunks; affected queued/in-flight work cancels, media/captions clear, live viewer retires, saved viewer is removed/quarantined and a five-second/64-KiB issuer-only no-cookie revocation attempt begins.
3. Make revocation unavailable/redirected; hold saved restore and persistence through exposure; restart immediately. A failed attempt is never called successful and no late credential/restore republishes the viewer. Unissued password exchanges have no session family to revoke.
4. Inject a cross-origin URI into an otherwise valid playlist: no app request to it. Try a redirect chain that ends at the original origin: document the SDK’s invisible-chain limitation; do not assert detection or zero disclosure. Password exposure cannot be undone by token revocation.

**Expected:** Accepted fallback containment: no redirected answer/bytes/credential adoption, prompt sign-out and bounded revocation attempt with durable retirement. Captured pre-follow synthetic secrets and invisible-return risk are explicitly accepted limits, not strict no-follow success.

**Failure:** Redirected response rendered/adopted, stale viewer restored, containment hangs, failed revocation called success or test claims SDK guaranteed zero transmitted secrets.

**Route to:** vega-transport.

### 6. Wide sort/filter on every grouping

1. Prepare two libraries with at least 250 genres/series/groups and 650 playable titles; include authored playlists with duplicate entries, personal collections, artist albums/songs, author books, seasons, people with cast/crew credits, categories and Home recommendation rows. Sign in as a restricted member as well as the owner.
2. For every library tab and each named grouping, compare Sort/Filter to that scope’s server advertisement. Change Recently aired from Latest episode to Title, then duration/rating/last-played where advertised. Choose Random, page, navigate away and Back: the seed/order stays for that open selection; choosing Random again reshuffles.
3. Combine two member filters, clear one and then all. Every request must retain the grouping membership and current library/profile fence. Follow Genres/Series past 200 and sorted lists through the final page; check counts and restricted titles. A short/empty page with a next cursor must continue.
4. For a cross-library person/playlist/My List/Favorites, switch library chips or the native library control and test each library separately. Check own order before a filter, then the explicit server sort while filtered; removing the filter restores group scope. Cast/Crew does not pretend to be a supported person operand.
5. Record Home See all, personal collection, book parts/discs, native History/My List/Favorites and music Playlists separately. Current missing fields/controls are known parity gaps, not waived tests. Native role controls hide during browse; web offers the toolbar under All. Check phone/tablet and remote focus without moving the hero.

**Expected:** Every supported grouping offers all applicable advertised choices, membership and privacy survive sort/filter/paging and Recently aired is changeable. Authored own order is available unfiltered; filtered own-order preservation remains a recorded server gap. No grouping-wide parity pass while the named missing surfaces remain.

**Failure:** Missing required toolbar, a filter leaves its group, first-200 truncation, leaked denied titles, duplicate/lost pages, title-only subgroup or a false claim of own-order preservation under a filter.

**Route to:** web-viewer, native-screens, native-health-2, grouping-sorts, scale-check-2.

### 7. Read-only owner TV Dashboard

1. Sign in as owner on this actual television and open Settings > Server > Dashboard. Inspect server name/version/uptime/health, active viewers/devices/direct or converted play, background work, free/Portico/database space, alerts and work failed in the last day.
2. Press Select/Menu/long press on every row. There must be no Stop playback, Acknowledge, pause/cancel task or other write action and no broad Integrations/Metadata/People/Live TV administration. Owner file actions are absent on title pages too.
3. Start/stop playback and change a task/alert from a phone/web owner while focus sits on a Dashboard row. Observe quiet reread without blanking or focus movement. Disconnect/reconnect for 20 seconds and preserve prior content while retrying. Sign in as a member: no Server heading.

**Expected:** Useful read-only data, stable focus, current sessions/tasks and no mutation action. Removing the former TV actions is intentional under the owner decision; broad phone panels stay on phones/tablets.

**Failure:** Any TV write button/press sends a write, member sees owner Dashboard, broad admin pages appear or refresh moves focus/blanks the screen.

**Route to:** native-owner.

### 8. Whole-view Play and Shuffle beyond loaded pages

1. Prepare a saved view and a genre/person/collection/playlist selection with at least 650 playable permitted items; include hidden/missing titles and a second profile. Load only the first page, choose a non-title sort/filter and press Play, then Shuffle with a recorded seed.
2. Compare the queue with the complete authorized browse order/membership, including items after 500; advance through the boundary. Inspect POST /v1/queues: query/saved/container identity should be forwarded, not only loaded ContentEntry IDs. Repeat from phone, tablet and each TV client that exposes the page.
3. Edit/add/remove/restrict items while capture grows, switch profile before a late answer and restart the server during capture. Unchanged source resumes; changed source that lost its snapshot preserves the accepted prefix and fails that segment. Start a new selection to capture new membership.
4. Start two long captures and saturate the background read pool while browsing/owner controls continue. A third capture can refuse retryably; request deadlines settle, and resources release. Explicit item arrays remain capped at 500; that bound is distinct from query capture.

**Expected:** A first playback window arrives promptly and later capture reaches the entire original authorized selection without duplicates/loss or 500 truncation. Already accepted occurrences stay stable under seeded shuffle. Current screen forwarding is unfinished: record a UI truncation as Fail while separately recording server-selector success.

**Failure:** A first page or first 500 is treated as the whole view, a reordered/duplicate playlist starts the wrong occurrence, old profile starts playback, membership mixes after restart or control requests hang.

**Route to:** queue-and-retention, web-viewer, native-screens.
