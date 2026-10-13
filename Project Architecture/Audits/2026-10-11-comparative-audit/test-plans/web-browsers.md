# Web browsers acceptance

## Purpose and evidence

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Installed Chrome, Firefox, Safari and Edge; macOS/Windows for keyboard/screen-reader variants, iPhone/iPad/Android for touch. Integrated production server/web, conversions-capable private host, real HDR samples and Cast receiver for dependent cases; private network/fault proxy. All cases repeat at desktop and 375px where UI exists.

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

Use the candidate package’s matched production web, or on a private qualification copy run `npm ci --ignore-scripts` at server root, then `npm run build --workspace portico-web`. Point PORTICO_WEB_DIR at that absolute web/dist and start the isolated candidate server. A development-only check uses `npm run dev:web -- --host 100.120.50.24` on mini-1 (verify the actual port in its output); a production build is required for stale-chunk/cache/vendor cases. These full web builds are later qualification steps, not commands executed by this lane. Keep lockfiles and verify `third_party/libass-wasm/verify.py`; do not install into existing programme node_modules symlinks. Missing CSS/catalogue/vendor files are asset/build failures; a WebAssembly CSP error needs the shipped policy/assets inspected without broad unsafe script changes.

Run each applicable case in current installed Chrome, Firefox, Edge on Windows/Linux and Safari on macOS; record exact versions. Repeat touch/fullscreen cases in Safari on iPhone/iPad and Chrome on Android, desktop ≥1280 px and phone 375 px (also under 720 px). Use regular private testing accounts/profiles rather than production state. For accessibility use keyboard plus NVDA/VoiceOver and platform reduced-motion/largest text settings. For fault cases a private intercepting proxy must alter only the named test endpoint/status, leave successful replies untouched and record attempts without bearer values.

## Smoke subset

Run cases 1, 2, 3, 6 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. Data actions and stale previews in the console — smoke

1. At desktop and 375 px open a two-version film’s Files, preview Delete this version, type its exact title and remove; repeat after replacing the file and with deletion off. Do episode version deletion and last-version Missing.
2. Split a version via Not this title > Split it off, cancel while finding; merge exactly two films keeping the first, then remove library using typed exact name; rescan its still-present files.
3. Make a season member non-removable and delete season/show, then inspect count/partial reasons.

**Expected:** Only intended unchanged files deleted, changed/disallowed files retained, title identity/personal state retained; split/merge preserve correct versions and combined state, library removal keeps files and re-add restores watched. Small screens have reachable confirmations.

**Failure:** Unexpected disk deletion, wrong version IDs, lost state, absent partial failure or destructive confirmation unreachable.

**Route to:** console, identity, server-followups, review-identity.

### 2. Restore and restart progress screens — smoke

1. Create backup and make a visible change. Restore from Storage & backups, watch Copying/Checking progress beyond ten seconds then restart wait/result.
2. Restart from General, inspect spinner/readiness and new worker; send a client report, Read/Download/Delete under Troubleshooting.
3. Use a disposable corrupt state fixture and open / and readiness; repair/start.

**Expected:** Progress/restore result matches durable state, sessions sign out once, restart feedback settles; reports have sender/time/size, JSON download and confirmation. Recovery guidance reachable with 503 and no secret leak.

**Failure:** Timeout mistaken for integrity failure, perpetual restart spinner, falsely successful restore, secret report or blank recovery page.

**Route to:** console, ops-packaging, review-ops, accounts-security, limits-settings.

### 3. Start, resume and in-place quality/audio/engine recovery — smoke

1. Resume a half-watched remux; inspect first segment at resume rather than seg0. From episode 2 change to 720p, audio track 2, Original and force one actual decode failure.
2. Check network/session/queue after each action; replay at episode 3 end. Send invalid track/stale If-Match via isolated proxy and retry actual mutation.
3. On a nonconverting profile play unsupported media; separately delete/miss a file and retry a paused expired lease.

**Expected:** Same session/queue/current entry, replacement generation and position; one bounded decode fallback, old stream/progress survives rejected replan, replay preserves next entry. Permanent refusal shows specific plain reason; expired lease/network use recovery.

**Failure:** New queue/empty Up Next, resets to zero, progress writer stops on rejected command, remembered network incompatibility or endless fallback.

**Route to:** web-player, playback-core, review-playback-core, planner-wiring.

### 4. Original buffering, Automatic and sleep/restart

1. Throttle 20 Mbps title to 1.5 Mbps on Original for minutes, then explicitly choose Automatic; unthrottle after a downshift.
2. Restart server mid-title and sleep laptop five minutes; reopen. Reload/close tab with one-stream account and start again.
3. Play 80 Mbps 4K remux for ten minutes on capable Chrome, inspect media-internals quota/frame/stall evidence.

**Expected:** Original buffers without stopping; Automatic starts Original, lowers around 15s and raises gradually around one minute after recovery. Server returns within 90s to same position/pause state; old closed session retires promptly. High-rate run has no recurring stalls/quota errors.

**Failure:** Automatic default, stop after ordinary buffering, lost position, stale occupied stream slot or buffer quota loop.

**Route to:** web-player, playback-core, limits-settings.

### 5. Touch, keyboard, PiP and AirPlay

1. Try tap/show/hide, double-tap seek and system fullscreen on phone/tablet; exercise each ? shortcut, Caps Lock, slider focus and modified Ctrl/Cmd+L/F, Alt+Left.
2. Pause by media key/browser hub/PiP; seek/book/queue Next/Previous. In Safari choose direct MP4 AirPlay then hls.js remux; test native-HLS fallback on a supported DV5 fMP4 source.
3. In Watch Together pause by media key.

**Expected:** Controls update/pause intent stays; ordinary tap never pauses; modified browser shortcuts work. Direct/native Safari AirPlay and fallback follow capabilities; hls.js-only route hides AirPlay. Group receives pause, no route failure learned from native fallback.

**Failure:** Media pause resumes itself, browser shortcut seeks/fullscreens, touch subtitles vanish in fullscreen or group bypass.

**Route to:** web-player, web-subtitles; fix-pass web-client-core.

### 6. Subtitle renderer loads lazily and preserves styles — smoke

1. Play SRT and inspect no vendor load; choose attached-font anime ASS, inline [Fonts] sidecar and legacy Windows-1251 ASS.
2. Compare a reference frame for signs/karaoke/typeface; test Japanese/Chinese/Arabic/Hebrew/Thai/Hindi and Korean with no attachments, noting declared bundled-font gaps.
3. Inspect one worker/script/WASM/default font request and only matching script-font requests; block WASM then original PGS on separate runs.

**Expected:** No video conversion/new stream for supported drawing path; authored styles/scripts retained. WASM failure falls back within 45s to useful cues, bitmap failure burns or honestly refuses when conversion off. Missing Korean coverage recorded as a gap, never accepted glyph fidelity.

**Failure:** CSP/WASM failure loop, all ASS flattened without fallback notice, duplicate workers, boxes for supported script or unsupported fidelity claimed.

**Route to:** web-subtitles, subtitles-text.

### 7. Subtitle timing, canvas and formats

1. Choose ASS then PGS; pause/seek, play 0.5x/2x five minutes, frame-step and starve buffer. G/H offsets must apply/reload persist.
2. Resize/fullscreen/fill-fit/density-screen/rotate iPad. Test 1920x800 image with 1920x1080 subtitle canvas and 4:3 VobSub.
3. Play embedded/sidecar SUP, idx/sub and real DVB TS with forced/multi-position/fade cues; seek for ten minutes and inspect memory. Switch ASS→PGS→SRT→Off→next episode.

**Expected:** Within a frame or two, proper letterbox/authored positions, colour/fades/both interlace rows, no memory climb/stale cues; one worker maximum, none after Off. Repackaged/converted resume retains subtitle origin.

**Failure:** Stretched/shifted cues, pink/green pixels, alternating missing rows, timing runs ahead while stalled or old track over next title.

**Route to:** web-subtitles, subtitles-text, conversion-outputs, review-playback-pipeline.

### 8. Caption appearance and device limits

1. Change Size/Background and drawn Position Up/Down/Reset, reload and compare. Keep buffered text subtitles through 60s network loss; Firefox lines stay above controls.
2. On iPhone use ASS, PGS and system fullscreen; use drawn subtitles then PiP on desktop. Force Native/non-drawing fallback and keep burn lease >2 minutes.
3. Try malformed/UTF16/legacy text, literal {LAUGHS} and one bad cue between good cues.

**Expected:** Appearance/position remembered, useful cues retained/no markup execution. iPhone ASS cue/PGS burn and fullscreen subtitles work; desktop drawn subtitles remain only in page during PiP (platform limit), no false floating-window claim. Burn keeps lease.

**Failure:** No subtitles in native fullscreen, unwanted markup, lost cues after transient network or burn lease dies.

**Route to:** web-subtitles, web-player, subtitles-text.

### 9. Freshness, scroll, focus and offline edits

1. Open Home and deep library page (~2000th title); mark watched/play/scan/remove on second device. Keep an unsaved owner field while dropping/restoring network.
2. Back through Home/library/title; hide tab/background two minutes, edit elsewhere, foreground. Over HTTPS open two tabs then close stream leader; repeat HTTP long-poll.
3. Stop server >15s, visit an uncached library, restart; trim retained events then reconnect from old cursor.

**Expected:** One shared healthy HTTPS feed, updates ≤30s (watched normally ~5s, Continue ~20s), at most one Home read/15s during ticks; covered/hidden screens quiet, mounted page/edit/scroll retained. Background feed closes ~30s, cursor resumes, failure line delayed and no focus; stale cursor resyncs.

**Failure:** Flash/skeleton/scroll reset, duplicate SSE, frequent polling/idle reads, lost unsaved value or uncached error prevents recovery.

**Route to:** freshness-clients, freshness-server, review-freshness, web-app.

### 10. Real revision changes and resource retries

1. On two devices add My List then favorite within a second, rename playlist then stale add; start/pause scan during progress. Separately concurrently reorder playlist.
2. Use proxy to return one 428 revision_required to a setting/container/queue change, then allow reread/retry. Repeat repeated 428, malformed/403 reread and abort.
3. Rate 0/1/9/10 and clear null; play 120s and inspect 120000ms wire plus UTC dates.

**Expected:** Unrelated actions stick silently, real ordering conflict is surfaced. Retry uses current exact resource revision and original operation ID, max three mutations/two rereads; invalid/aborted reread sends none. Half-star/rating/progress/date units accurate, fractions/out-of-range refused.

**Failure:** Catch-all conflicts, mutation replay after invalid reread, changed key/body or seconds on millisecond wire.

**Route to:** freshness-clients, native-health, registry, search-sort.

### 11. Browse memory, sorting, filtering and large paging

1. In each video/music/book library and every genre/series/category/collection/person/tab choose each advertised sort/filter both directions; use 205+ grouping values, stable random seed and restricted/private/shared lists.
2. Set library tab/sort/filter, go Home/rail/back; open See all and genre link, then rail; switch profile/reload. Rename artist/album/subtitle evidence during page.
3. Use Rocky 2/10, articles/punctuation/accents/CJK/Cyrillic and # jumps; search spiderman, wall e, MASH, X, middle CJK substrings, casts/directors/authors/narrators and alternative titles.

**Expected:** Same broad options across scopes, server-advertised pivot vocabulary honored; no duplicate/skip/hidden context, count/filter/sort exact and stable. Rail remembers per-profile library choice; scoped link does not overwrite it; Back preserves position. Exact titles precede context.

**Failure:** Title-only group view, option leakage from prior library, client whole-library scan, wrong natural/Unicode order or hidden/private results.

**Route to:** web-app, search-sort, grouping-sorts, native-health, freshness-server.

### 12. Accessibility, page announcements and phone navigation

1. From address bar Tab once and use Skip to content; count main landmarks. Visit Home/library/title/show/settings/search/sign-in with NVDA/VoiceOver; test slow provisional page/profile switch.
2. Read unwatched/watched/half-watched cards, list row positions/totals and far-scrolled focused card. Check grid/list wrappers at three card sizes on library/Saved/person/episodes/sections.
3. At 375px open avatar Switch profile/server, feedback/help, all bottom sheets and episode ⋯ by hover/focus/touch/right-click. Enable reduced motion and largest text.

**Expected:** Visible skip moves into one main, stable title/history, real page name spoken once (no old/provisional profile title), watch state once, position/total; list focus continues visible controls. Layout spacing unchanged, menus accessible without watched badge obstruction. Known identical-name announcement limitation recorded.

**Failure:** Double announcements, missing state/landmark, focus loss, spacing regressions or unreachable sheets.

**Route to:** web-app; fix-pass web-client-core.

### 13. Session renewal, vault loss and restrictions

1. Sign in on HTTP and HTTPS two tabs >15 minutes; switch A→B in one tab and reload both. Open Saved during renewal.
2. In disposable DevTools IndexedDB portico.server-connections.v1 > vault delete key, replace selected with broken version-2 plain JSON, then use wrong-algorithm key on separate snapshots; reload/sign in/reload. Briefly block IndexedDB via open version-change transaction.
3. Promote/demote member, reset secondary profile PIN and block library/ratings after plays. Return gateway HTML 403, 429 Retry-After and connection error.

**Expected:** Renewal remains signed in, profile switch invalidates old active viewer on next request while reload chooses B. Confirmed vault loss goes to sign-in and can save anew; ambiguous blocked storage preserves originals/session. Live role updates ≤30s; hidden/absent answers concealed, transient gateway errors never sign out.

**Failure:** Infinite Cannot reach after vault loss, overwrite on ambiguous error, wrong profile data, stale admin authority or 403/429 sign-out.

**Route to:** web-app, accounts-security, server-followups, base-health-2; fix-pass web-client-core.

### 14. Personal state, Undo, recommendations and discovery

1. Mark mixed-progress show/season/album/book watched then Undo within 30s and restart inside window. Repeat expiry/new playback/new member/changed season overlap; read every member.
2. Finish/replay/partially play unknown-duration/shared multi-episode/missing titles; dismiss show then reopen Next Up, mark anchor unwatched and add a later-season premiere. Pause hours and inspect Home order/count.
3. Pause/clear History, sync older offline plays after newer mark, change restrictions and inspect History/Home/suggestions/discovery/See all.

**Expected:** Undo restores exact progress/history/flags/ratings or declines atomically when changed/expired. Stable title state, meaningful Continue, one completion per occurrence, hours-paused no reorder; next available episode correct. Older/skewed evidence cannot overwrite newer action; private/restricted titles absent before paging/counts.

**Failure:** Partial Undo, duplicate completion, stale missing copy offered, leaked historical art/ID or wrong destination reconstructed by client.

**Route to:** personal-state, review-freshness, downloads, music-audiobooks.

### 15. API key and personal-token console

1. Open Server > Integrations > API keys; create disabled until name/scope, Everything locks scope checks. Create Read expiring 30 days, inspect once-only pk. dialog, Copy and explicit copied dismissal.
2. Use key for libraries, Refresh last-used, wait idle >minute/restart; revoke and Show revoked. Set 30 requests/min then clear to default 600.
3. As member Profile & account create once-only pat.; owner lists/revokes it; disable Members may make tokens and verify create disabled, existing revoke usable and owner profile has no personal-token section.

**Expected:** Secrets only once, proper scope/name/expiry/hint/last-used and revoked status, limit affects keys not human sessions; membership/tokens boundaries correct at desktop/375px.

**Failure:** Secret shown in list, dialog dismissed losing secret, wrong token authority, last-used lost or owner UI limited by script traffic.

**Route to:** console, api-platform, review-api-platform.

### 16. Webhook and assistant console

1. Add valid name/address/events, invalid ftp/credentials URL, Playback All and empty/custom secret; inspect once-only response. Send test and watch live delivery log.
2. Use closed-port receiver then restore; switch off, rotate secret, remove/log deletion and add to server maximum. Open Assistants, copy /v1/mcp and follow Create key.
3. Use actual scoped MCP client for initialize/list tools; verify wrong scope tool absent.

**Expected:** Validation/plain sentences, async waiting→delivery/error/next retry without Refresh, limits and secret handling correct; assistant address/copy works and role-filtered tools load. Test disabled in off UI even if protocol supports explicit test.

**Failure:** Hidden retry/failure, duplicate secrets, unreachable event checklist/mobile action or off webhook silently sends regular events.

**Route to:** console, api-platform, review-api-platform.

### 17. Metadata attention and provider settings

1. Import misspelled/no-confidence fixtures, >one review page; compare exact badge/total. Use suggestion, search, full TMDB movie URL then bare ID/picker, Dismiss then rename/rescan. Episode only Open show/Dismiss; local-only only Dismiss; no audio attention tab.
2. Set own provider key then Use Portico key, verify field empties/status; save artwork limit using abc then 1,5/1.5; inspect used/limit, provider navigation from library.
3. Change language/country and refresh provider, rename/edit title while scan/rebuild runs; create deterministic poison title with the source regression fixture then inspect Titles set aside and retry.

**Expected:** Exact live counts, correct row actions, async search suggestions, provider roles/locale/art quota and locks preserved; invalid number blocked, comma/period normalize; poison isolated/reason persists/retry clears. No broad catalogue stall.

**Failure:** Page length used as total, key displayed/persisted in field, local-only network requests or poison blocks all rebuild work.

**Route to:** console, providers-matching, freshness-server, server-followups.

### 18. People, privacy and device approvals

1. Preview/delete disposable member with multiple profiles/devices/plays; mutate member in second tab before Delete; inspect re-preview. Owner cannot delete self; turn member off; change rating system/age and save.
2. Invite a specific Portico username and try different account. Require device approval, sign member into new device and approve/block; owner new device not held. View member privacy sentence.
3. Queued admin trust change: demote/sign out actor before commit using the focused server authority regression, then inspect device unchanged.

**Expected:** Counts/consequence clear, stale preview rereads, retained history says removed person; new rating choice resets value correctly, invite account strict, device pending/approved/blocked correct and owner survives. Commit uses current authority.

**Failure:** Wrong person removed, stale preview silently accepted, self-deletion, wrong invite admitted or queued demoted actor changes device.

**Route to:** console, accounts-security, review-accounts-security.

### 19. Owner settings round-trip and effects

1. Change speed Normal→Fast; pause background hour/resume; disable Watch for library changes and add sample.mkv/broken symlink then scan, inspect skip sentences and Last scan live counts.
2. Set working space 4→8GB, nonexistent/incompatible/qualified FFmpeg path; observe requested/effective/inUse before/after restart. Configure home ranges/external address/cert/connectionLimit and inspect immediate versus restart effects.
3. Inspect storage three folders/space/recovery-copy delete, streams/connections, prepared download usage by profile; remove one profile’s downloads. Turn update checks off/on, use controlled current/newer public feed. Edit setting elsewhere while this page has one unsaved field.

**Expected:** All controls persist/take effect as described, monitoring off prevents automatic scan; background work pauses between chunks. Custom tool qualification refuses bad path and changes only after restart. Usage/retention/copy operations exact; edits kept while quiet refresh updates untouched rows; no automatic update installation.

**Failure:** Dead saved control, setting takes unintended instant effect, old stream switches tools, unsaved edit lost, inaccurate usage or update silently installs.

**Route to:** console, limits-settings, networking, ops-packaging, server-followups.

### 20. Live guide and recording console

1. Add source preview with bad EXTINF/missing address, change Most channels/guide row/expanded-size limits; choose source groups with search/counts, bring all/choose two and refresh.
2. While viewer plus recording occupy distinct channels inspect actual tuner occupancy; tune/pause/go-live/channel arrows/G guide/captions. Schedule/cancel, Back out then confirm; check currently recorded-so-far wording and injected failure inside confirmation.
3. Check en-GB Favourites, 24-hour guide ruler, metadata/console dates; source guide refresh/programme boundary.

**Expected:** Useful skips/counts and configured selections, actual tuner occupancy (shared same channel counts once), controls work, cancel confirmation safe/focus Cancel, partial consequence honest. Times/locale consistent; guide refresh does not retune.

**Failure:** One bad row blocks source, stale usage, cancel bypass, US/date remnants or boundary freeze.

**Route to:** console, livetv, review-livetv, web-player, web-app; fix-pass native.

### 21. Downloads page and receipt visibility

1. Claim film/large MKV/whole season with external subtitles and multiple versions; reload and Save. Delete title then inspect claim; expire claim while phone retained copy.
2. Withdraw library access and replay old operation IDs; poll 350 claims and finish an older row. Inspect preparation request count.
3. Select lower version then add better copy, split/merge title and download; compare bytes/hash and profile usage.

**Expected:** Human names/output filenames, persisted whole-season request/quality list and selected version; deleted-title note, retained-authorized copy renews but withdrawn access conceals/refuses. At most three list polls per round, old row settles ≤two polls; shared prepared bytes count once, originals zero.

**Failure:** IDs exposed as names, wrong version, per-row polling, revoked grant served or ordinary expiration deletes offline copy.

**Route to:** downloads, review-downloads, server-followups, console.

### 22. Audio conformance, listening and lyrics

1. On private Vite conformance host set AUDIO_REPORT_DIR and open /audio-conformance.html in each browser. Use original/extras HE-AAC/PS/Vorbis/MP3/FLAC corpus; compare frame count/lag/tolerance and supported qualifiedCodecs.
2. Play gapless tone at 44.1→48k output, deep-seek one/two-hour FLAC/MP3 on slow link, >100 chapters, 2.5x/3x audiobook and pause 5min/1h comfort rewind. Test optional renderer failure and queue shuffle/play-next/remove/move/Previous/repeat-all.
3. On iPhone Safari silent switch/lock/call/media keys. Scan embedded WEBP, mixed-case Cover.WEBP, local LRC/TXT/embedded lyrics; retag/duration-change and wrong LRCLIB/429 Retry-After fixture.

**Expected:** Admitted codec exact trim/no audible joins, prompt bounded deep seeks (no whole prefix), pitch and 15/30s rewind correct with explicit paused seek winning; ordinary fallback retains session. Local useful tags/art/lyrics survive, wrong provider lyrics not auto-published, no-lyrics/private access honored.

**Failure:** Extra priming/tail/drift, clicks/prefix download, stuck paused seek, wrong gain/credits/chapters, playback broken by optional decoder or private lyrics leaked.

**Route to:** audio-engines, music-audiobooks, web-player, web-subtitles.

### 23. Together, Play on and Cast autonomy

1. Join two profiles/devices, one converted member with 1–2/8/19s seeks. Host seek/pause/title change during pending seek, including near last 30s; log readiness/revision/landing/drift 120s.
2. Background host/guest, wait 30s/2min/10min; test privacy/restriction-hidden item, eligible takeover and phone→TV approval/wrong device, then remote controls/reconnect.
3. Cast to registered HTTPS receiver, inspect CSP load, close/kill phone >10min then reconnect; drop receiver network near EOF, revoke pairing, deny takeover and race stale cleanup against replacement.

**Expected:** Bounded group hold (≤5s), no seek while pending, at most few corrective seeks converging <750ms, no early group end. Host controls only conflict on real changes, stale/takeover eligibility times correct; paused loads silent. Destination begins before source stops; Cast keeps playing unattended and stops on end/revocation even final report fails, one current channel.

**Failure:** Periodic seek loop, private title leak, early takeover, source stopped before ready, phone disconnect stops TV or old cleanup revokes replacement.

**Route to:** social, leftovers-server-b, native-sync; fix-pass social-downloads, web-client-core.

### 24. Production assets and stale chunk recovery

1. Serve candidate production web, open Home, replace assets with another candidate whose hashed chunks differ, then navigate Settings without reloading and press Try again.
2. Reload and inspect Cache-Control/conditional requests for hashed assets and vendor resources; inspect dist assets for Gallery/WindowedLab/ChannelsLab/BrowseLab/Features.
3. Use private response override with unknown enum/extra server row kind while browsing.

**Expected:** Try again reloads into requested Settings, immutable hashed assets avoid redundant conditional requests; no experiment chunk shipped. Unknown supported-extension rows are ignored and rest page stays usable.

**Failure:** Same stale-chunk loop, test pages shipped, broken vendor assets or whole page crashes on unknown row.

**Route to:** web-app, web-subtitles; fix-pass web-client-core, catalogue-playback.
