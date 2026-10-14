# Web browsers acceptance

## Purpose and evidence

Updated for the evening implementation and owner decisions. New cases remain qualification work; known missing candidate implementation is Fail, while an unavailable test prerequisite is Blocked. Native-owner recorded 655 native/87 Vega before the mirror; the later running log confirms 655/87 again after the mirror with clean type checks and equal packages. Physical acceptance remains unrun.

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
4. Delay a session GET/PATCH in private proxy, change titles twice, release old replies then close/reload; repeat paused and lease recovery. Current title/queue remains current and old session retires within bounded lease/request failure fallback.

**Expected:** Same session/queue/current entry, replacement generation and position; one bounded decode fallback, old stream/progress survives rejected replan, replay preserves next entry. Permanent refusal shows specific plain reason; expired lease/network use recovery.

**Failure:** New queue/empty Up Next, resets to zero, progress writer stops on rejected command, remembered network incompatibility or endless fallback.

**Route to:** web-player, playback-core, review-playback-core, planner-wiring, review-web-logic.

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
4. Use the review-web-logic bounded DVB fixture containing256 large region definitions; retained valid content renders without memory growth. Rapid seeks/rates/track changes must release retired worker.

**Expected:** Within a frame or two, proper letterbox/authored positions, colour/fades/both interlace rows, no memory climb/stale cues; one worker maximum, none after Off. Repackaged/converted resume retains subtitle origin.

**Failure:** Stretched/shifted cues, pink/green pixels, alternating missing rows, timing runs ahead while stalled or old track over next title.

**Route to:** web-subtitles, subtitles-text, conversion-outputs, review-playback-pipeline, review-web-logic.

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
4. Keep tabA visible, hide tabB>30s until its feed stops, change metadata/watchedA then revealB; exactly one quiet refetch, repeat HTTP long-poll.

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
4. Type search without Enter: address remains /search; Enter/reload records Recent searches; Clear/reload removes, disabled-recents setting stores none. Owner title menu offers Delete/Play next/Add to queue after playback and Download only if enabled; test rights for member.

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
4. Edit Name inA and Connection limit/Home networks inB, saveB thenA: both survive; edit same field: genuine conflict retains draft. Drop response after rebased save and Retry with exact same operation key/body; one receipt. Forget choices after slow initial storage, restart and verify forget persists.

**Expected:** All controls persist/take effect as described, monitoring off prevents automatic scan; background work pauses between chunks. Custom tool qualification refuses bad path and changes only after restart. Usage/retention/copy operations exact; edits kept while quiet refresh updates untouched rows; no automatic update installation.

**Failure:** Dead saved control, setting takes unintended instant effect, old stream switches tools, unsaved edit lost, inaccurate usage or update silently installs.

**Route to:** console, limits-settings, networking, ops-packaging, server-followups, review-web-logic.

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
4. Submit downloads for versions A/B; make only A unavailable and retry A. Use the existing collision regression with a pending playlist request whose ID equals a merged show ID; read target/membership/claim after merge.

**Expected:** Human names/output filenames, persisted whole-season request/quality list and selected version; deleted-title note, retained-authorized copy renews but withdrawn access conceals/refuses. At most three list polls per round, old row settles ≤two polls; shared prepared bytes count once, originals zero. Retry stays on requested version/same-source successor; unrelated playlist target/membership survives coincident entity ID.

**Failure:** IDs exposed as names, wrong version, per-row polling, revoked grant served or ordinary expiration deletes offline copy.

**Route to:** downloads, review-downloads, server-followups, console, review-server-late.

### 22. Audio conformance, listening and lyrics

1. On private Vite conformance host set AUDIO_REPORT_DIR and open /audio-conformance.html in each browser. Use original/extras HE-AAC/PS/Vorbis/MP3/FLAC corpus; compare frame count/lag/tolerance and supported qualifiedCodecs.
2. Play gapless tone at 44.1→48k output, deep-seek one/two-hour FLAC/MP3 on slow link, >100 chapters, 2.5x/3x audiobook and pause 5min/1h comfort rewind. Test optional renderer failure and queue shuffle/play-next/remove/move/Previous/repeat-all.
3. On iPhone Safari silent switch/lock/call/media keys. Scan embedded WEBP, mixed-case Cover.WEBP, local LRC/TXT/embedded lyrics; retag/duration-change and wrong LRCLIB/429 Retry-After fixture.
4. On fault proxy ignore Range or return wrong Content-Range; own engine falls back cleanly with bounded memory. Deliver chunks for>30s then pause/seek during prepared-track load: progressing read lives, cancellation settles.

**Expected:** Admitted codec exact trim/no audible joins, prompt bounded deep seeks (no whole prefix), pitch and 15/30s rewind correct with explicit paused seek winning; ordinary fallback retains session. Local useful tags/art/lyrics survive, wrong provider lyrics not auto-published, no-lyrics/private access honored.

**Failure:** Extra priming/tail/drift, clicks/prefix download, stuck paused seek, wrong gain/credits/chapters, playback broken by optional decoder or private lyrics leaked.

**Route to:** audio-engines, music-audiobooks, web-player, web-subtitles, review-web-logic.

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

### 25. Autoplay, credits and continuous-play protection

1. Enable video continuous-play protection with count2 then3, play consecutive short episodes to that count and Continue. Disable and repeat; separately autoplay off/countdown variants.
2. Use episode with credits marker/next entry: Play now, Watch credits and credits auto-skip; film with empty queue should offer no next card. Repeat audio queue without inheriting video protection.

**Expected:** Question appears at selected count only when on; Continue counts correctly, autoplay/countdown obey preference. Authored/high-confidence chapter markers work, weak luminance marker never offered before approval; credits/card/skip behavior correct, listening keeps its own policy.

**Failure:** Dead settings, protection every title/never, weak unapproved marker shown, next card with no queue or video protection interrupts album.

**Route to:** limits-settings, web-subtitles, subtitles-text, native-playback.

### 26. Wide sort/filter on every grouping

1. Prepare two libraries with at least 250 genres/series/groups and 650 playable titles; include authored playlists with duplicate entries, personal collections, artist albums/songs, author books, seasons, people with cast/crew credits, categories and Home recommendation rows. Sign in as a restricted member as well as the owner.
2. For every library tab and each named grouping, compare Sort/Filter to that scope’s server advertisement. Change Recently aired from Latest episode to Title, then duration/rating/last-played where advertised. Choose Random, page, navigate away and Back: the seed/order stays for that open selection; choosing Random again reshuffles.
3. Combine two member filters, clear one and then all. Every request must retain the grouping membership and current library/profile fence. Follow Genres/Series past 200 and sorted lists through the final page; check counts and restricted titles. A short/empty page with a next cursor must continue.
4. For a cross-library person/playlist/My List/Favorites, switch library chips or the native library control and test each library separately. Check own order before a filter, then the explicit server sort while filtered; removing the filter restores group scope. Cast/Crew does not pretend to be a supported person operand.
5. Record Home See all, personal collection, book parts/discs, native History/My List/Favorites and music Playlists separately. Current missing fields/controls are known parity gaps, not waived tests. Native role controls hide during browse; web offers the toolbar under All. Check phone/tablet and remote focus without moving the hero.

**Expected:** Every supported grouping offers all applicable advertised choices, membership and privacy survive sort/filter/paging and Recently aired is changeable. Authored own order is available unfiltered; filtered own-order preservation remains a recorded server gap. No grouping-wide parity pass while the named missing surfaces remain.

**Failure:** Missing required toolbar, a filter leaves its group, first-200 truncation, leaked denied titles, duplicate/lost pages, title-only subgroup or a false claim of own-order preservation under a filter.

**Route to:** web-viewer, native-screens, native-health-2, grouping-sorts, scale-check-2.

### 27. Receipt Undo, paused start and real remote controls

1. Mark a partially played show/season/album/book from the title page and from a card menu. Press the one Undo toast within 30 seconds; compare watched intent/resume on every member. Repeat after another device plays/edits membership, after expiry and with a double press.
2. Load a title in an already-paused Watch Together room and measure its first audible frame, including same-title reload. It must stay paused before preparation/first output, not play then pause.
3. Play on a second actual device and open Play on. Compare its reported position/duration, pause/resume, Back/Forward and Start over against the target, not the controller device. Stop elsewhere and use a retained menu; hidden actions must not be sent.
4. On a native companion device during playback change Automatic/Native/mpv; this engine selector is not a web or Vega feature. Record session ID, queue occurrence, position, pause and track/quality choices before/after the capability publication and replan. Offline files/live channels follow their supported engine behavior rather than a forced unrelated restart.

**Expected:** Undo submits its receipt and restores exactly the accepted snapshot or says why it cannot; there is one toast. Paused start is silent. Remote controls change the actual receiving player, refresh readback and use server-listed authority. Online engine changes preserve the same session/queue/position/choices.

**Failure:** Inverse mark used as Undo, two notices, expired receipt clears newer intent, first-frame audio, controls affect the phone instead of target or engine setting creates a new queue/session unexpectedly.

**Route to:** native-screens, web-viewer, web-owner, social, native-playback.

### 28. Quiet web lists, field-safe retries and owner transport

1. Use two owners A/B. In A keep keys/tokens/webhooks/devices/invitations/reports/backups/storage/source/tuner/recordings/channels and viewer downloads/guide open in turn. Mutate in B; within about 30 seconds visible lists reread without blanking. Hide A: automatic clock reads stop. Fail one read then restore; no immediate panel error. Expensive disk usage is not polled.
2. In A edit one setting; B changes a different setting. A’s untouched value updates and its draft survives. Force one 428 revision_required or 412 revision_mismatch; read current state and retry with the original operation identity. Both owners edit the same field differently: conflict retains the draft and asks for a deliberate next Save. Repeated refusal, denied/malformed reread or abort must terminate boundedly.
3. Play a film as a member on an actual second device. Web Dashboard > Watching now: Pause/Resume, Back/Forward 30 seconds, Start over, Details; observe target position/audio. Live sessions have no relative seek. End the target and use a retained menu. Member/read/playback credentials cannot administer another viewer.
4. Repeat own-profile remote seek from Settings > This device. No admin scrubber/duration is invented. Record Library Channel builder and mount-command conflicts separately; these do not currently use the silent retry wrapper.

**Expected:** Visible lists update calmly; drafts rebase only unchanged fields; one stale response recovers silently and real conflicting edits remain. Actual target obeys transport and dead session has a useful message. Known missing server hint events are measured clock behavior, not event delivery.

**Failure:** Blank list, hidden-tab polling, draft overwritten, retry after denied/invalid reread, endless 428 loop, controls only change UI or mutation scope bypass.

**Route to:** web-owner, web-viewer, review-web-logic, freshness-clients, security-sweep.

### 29. Playback problems, schedules and episode/source controls

1. Cause a definite disposable file/player failure. Server > Streaming shows remembered playback problems grouped by source. Forget removes that file’s failures and the next attempt retries it. With no rows the group is absent. Current rows have no title link; native has no panel.
2. People > an account: add Weekdays viewing hours, save/reload and verify real outside-hours sign-in/playback refusal without credential retirement. Conflicting edits keep the form. Test narrow width, next-day hours and removal to Any time.
3. Using a real provider candidate or faithful provider fixture, Edit metadata > Match selects alternate/absolute order and a signed episode offset, then verify episodes against the chosen provider coordinates. The local-only browser fixture does not establish this acceptance.
4. HDHomeRun source add displays guide URL and useful cause-specific failures. Check actual mapping/programmes; verify confirmation patterns, phone-width group headings and indistinguishable version/subtitle labels, including Download picker.

**Expected:** Controls save/read back and change actual remembered failures, admission/matching/source results; each populated state is exercised. No claim that blank browser pages or pure rules tested these rendered panels.

**Failure:** Missing rows/control, failure clear does nothing, wrong episode mapping, blocked guide field, generic error replacing actual cause or duplicate version/track labels.

**Route to:** web-owner, web-viewer, leftovers-server-a, livetv.

### 30. Whole-view Play and Shuffle beyond loaded pages

1. Prepare a saved view and a genre/person/collection/playlist selection with at least 650 playable permitted items; include hidden/missing titles and a second profile. Load only the first page, choose a non-title sort/filter and press Play, then Shuffle with a recorded seed.
2. Compare the queue with the complete authorized browse order/membership, including items after 500; advance through the boundary. Inspect POST /v1/queues: query/saved/container identity should be forwarded, not only loaded ContentEntry IDs. Repeat from phone, tablet and each TV client that exposes the page.
3. Edit/add/remove/restrict items while capture grows, switch profile before a late answer and restart the server during capture. Unchanged source resumes; changed source that lost its snapshot preserves the accepted prefix and fails that segment. Start a new selection to capture new membership.
4. Start two long captures and saturate the background read pool while browsing/owner controls continue. A third capture can refuse retryably; request deadlines settle, and resources release. Explicit item arrays remain capped at 500; that bound is distinct from query capture.

**Expected:** A first playback window arrives promptly and later capture reaches the entire original authorized selection without duplicates/loss or 500 truncation. Already accepted occurrences stay stable under seeded shuffle. Current screen forwarding is unfinished: record a UI truncation as Fail while separately recording server-selector success.

**Failure:** A first page or first 500 is treated as the whole view, a reordered/duplicate playlist starts the wrong occurrence, old profile starts playback, membership mixes after restart or control requests hang.

**Route to:** queue-and-retention, web-viewer, native-screens.

### 31. Deferred Home and large mixed-library qualification

1. Use an integrated candidate that includes scale-check-2’s final snapshot; record that prerequisite. Test 10k, 200k and one-million mixed video/music/books with versions, genres, credits and three restricted profiles. A full two-million fixture is not required under the recorded 8 GB scratch budget; structural tests are separate evidence.
2. Hold one Home preview response. The row manifest arrives first; two row slots work independently, a stalled/failing row times out and other rows appear. Quiet refresh keeps cards. Switch viewer while responses are held: old hero/row/pagination cannot cross the fence.
3. Compare every advertised sort/direction/operator, exact totals and subgroup membership on first/deep pages during empty and version-bump rebuilds; browse/search keep responding with prior content and live restrictions. Exercise broad search prefixes, group missing/personal/play-state predicates and mark/Undo 500/5,000.
4. Record actual latency/refusals/RSS/WAL/disk, writer holds and first visibility after add/remove. Treat one observation as one sample, not p95. Monitor space and preserve logs before deleting each owned database. Test authenticated HTTP/concurrent players separately from catalogue-process benchmarks.

**Expected:** No unavailable/preparing browse reply, independent visible Home rows, exact restricted results and reproducible measured resource outcomes. Capacity claims require explicit targets/sample distribution; pending 200k/one-million runs or structural two-million proof are never called mixed-scale acceptance.

**Failure:** Stalled shelf blocks Home, old viewer cards publish, totals/order/privacy differ, large operation kills control reachability or disk budget exhausted without preserved evidence.

**Route to:** scale-check-2, scale-check, freshness-server, queue-and-retention.
