# Android phone and tablet acceptance

## Purpose and evidence

Updated for the evening implementation and owner decisions. New cases remain qualification work; known missing candidate implementation is Fail, while an unavailable test prerequisite is Blocked. Native-owner recorded 655 native/87 Vega before the mirror; the later running log confirms 655/87 again after the mirror with clean type checks and equal packages. Physical acceptance remains unrun.

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Ubuntu build host, Google API24 phone/tablet and current Android15+, HDR-capable and older devices, Fire tablet for download-background variant, adb developer access and isolated app data. Cellular/Wi-Fi, Bluetooth, TalkBack, PiP-capable API26+ model and Google handheld Cast receiver; signed testing APKs are not store-signing qualification.

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

On Ubuntu use JDK 17, Android SDK/platform/build-tools 36, NDK 27.1.12297006, CMake 3.22.1, network/cached dependencies, adb and checked-in Gradle 9 wrapper. Check copied packages from native root with `PORTICO_SERVER_REPO=/absolute/candidate-server node scripts/shared-packages.mjs --check`. From the native repo:

```sh
cd apps/native
npm ci --legacy-peer-deps
cd android
./gradlew :portico-mpv:fetchLibmpv
./gradlew :app:assembleGoogleDebug -PreactNativeArchitectures=x86_64
./gradlew :app:assembleFiretvDebug -PreactNativeArchitectures=x86_64
./gradlew :app:assembleGoogleRelease :app:assembleFiretvRelease -PreactNativeArchitectures=armeabi-v7a,arm64-v8a,x86,x86_64
```

Google APK serves phones/tablets and Android TV; Fire TV is its own flavor/minimum API 25 (Google minimum 24). These Release builds currently use debug signing and embedded JS, suitable for testing, not store-signing evidence. Select the actual device serial with `adb -s SERIAL install -r app/build/outputs/apk/google/release/app-google-release.apk` or corresponding firetv path. Both flavors share application ID, so use separate device state/snapshots for comparison. Debug requires Metro on 19424 (`npm start` in apps/native; use a verified private route/adb reverse); Release must launch without it. Inspect APK with unzip -l: lib/<abi>/libmpv.so and libporticompv.so exactly once for each selected ABI, RN/Hermes and embedded release bundle. Inspect merged launcher/services, Google-handheld-only Cast metadata and cleartext/system-TLS policy.

| First error | Response |
| --- | --- |
| Fetch/checksum mismatch | Inspect exact locked archive/digest; do not accept replacement digest automatically. Intentional local LGPL build uses -PporticoLibmpvDir=/absolute/path containing each selected <abi>/libmpv.so. |
| CMake imported lib absent | Confirm fetch dependency precedes configure/link and correct ABI directory; imported mpv target links porticompv. |
| Missing/duplicate packaged .so | Inspect AAR/APK; add exactly one jniLibs directory only if import packaging absent, remove duplication if both supply it. |
| Unresolved JNI/mpv | Match pinned header/binary, kept PorticoMpvNative/Listener JNI symbols and Surface/event signatures; do not guess extra helper/libc++ dependencies (later review found none for these binaries). |
| Kotlin/Java/generated BuildConfig/PackageList/R | Use Kotlin 2.1.20/Java 17 and npm-provided RN Gradle plugin/current generated sources/flavor, not cached stub declarations. |
| Player absent/startup option failure | adb logcat -s PorticoMpv reveals ABI/module/option cause; successful compilation does not establish available=true. Scope/protocol/output initialization errors stay mandatory. |
| Engine handoff freezes | Inspect Media3 release timeout and mpv event-thread termination; surface detachment stays synchronous, destruction off-main. Do not revert the physical stop fence. |

No Gradle command was run by this documentation lane. These are commands for the later platform qualification runner.

## Smoke subset

Run cases 1, 2, 6, 9 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. Release launch and capability truth — smoke

1. Install the actual candidate Release and launch signed out with Metro stopped, then direct sign-in and server/device code sign-in. Open Settings > Quality > Player and select all three choices.
2. Inspect GET /v1/me/devices/current/capabilities through a private debug log or owner device view; record version 2/native/mpv/engineSetting/actual decoder facts, not bearer. Turn force stereo on/off and inspect updated overrides.
3. Check PiP, Cast and AirPlay only on models/engines reporting support; TV and forced mpv hide handheld PiP.

**Expected:** Real modules/views load, Automatic default and Original quality; Player only if available, capability engine/depth/rate/channel/transport facts accurate and changes within about two seconds. No impossible feature shown.

**Failure:** Missing module/row, unresolved library, Release depends on Metro, duplicate capability translation or declared feature cannot run.

**Route to:** native-playback, native-app, playback-core, apple-mpv-review, android-native-review.

### 2. Download pause, background, verification and races — smoke

1. Download >2GB MKV and season on Wi-Fi; interrupt2min, Pause15min/Continue, force-kill/reopen, queue season/lock until completion then Airplane Mode BEFORE first reopen.
2. Toggle Wi-Fi-only during mobile-enabled transfer, continue while cellular, wait30min closed for Wi-Fi; also wait>24h before reconnecting. Repeated Pause/Continue, URL renewal, Remove/immediate same-ID re-add and delayed extras/old callbacks; kill at each phase.
3. Force-kill at100% during large hash (20+GB qualification sample when space permits); compare final hash/count, partial custody and latest artwork/subtitle extras. Unknown-length/oversized/short/malformed metadata batch uses existing native downloader regression; physical fault injection needs qualification harness.
4. On source regression/private instrumented device hold grant renewal, switch viewer, then release; change grant while downloading must update actual native URL only for current owner. Use same-millisecond newer offline observation and incomplete receipt: only exactly acknowledged submitted play/item may be cleared. Test transient container enrollment retry, backlog bound across requests and same-ID request on another viewer/server.

**Expected:** Identified allowed partials resume, no concurrent writer/stale callback overwrites new files; ready only after exact verification and local durable extras/receipt, no online finalization needed to play cold offline. Policy changes stop disallowed bytes (changed policy may restart0), known >24h grant expiry restart gap recorded. Hash survives process death/reconciles. Invalid batch atomic, hostile response never ready. No latest progress lost by same-timestamp ack, incomplete receipt retains unacknowledged journal; enrollment retries transient failures without unbounded queue or viewer collision.

**Failure:** Restarts0 unnecessarily, final file lost at hash, old extras delete new file, wrong policy resumes cellular or season requires initial online opening.

**Route to:** downloads, review-downloads, apple-native-review, android-native-review, review-native-js.

### 3. Offline bundle, expiry, authorization and progress

1. Cold signed-out Watch downloads/Airplane Mode for MP4/SRT and MKV/embedded/external ASS/VobSub: inspect show poster/episode order, chapter/intro marker/art/subtitles and private local paths.
2. Set server claim retention1day on isolated clock, expire or delete source title then reconnect and go offline again; separately disable profile downloads/withdraw library access and replay old claim/receipt operations.
3. Play one minute offline then sync, including per-part book/multi-episode occurrences, A→newerB→still-newerA, skewed future/backward device clock and a newer manual watched mark. Sign out/switch profiles and explicitly reclaim other-viewer bytes.
4. Submit downloads for versions A/B; make only A unavailable and retry A. Use the existing collision regression with a pending playlist request whose ID equals a merged show ID; read target/membership/claim after merge.

**Expected:** Self-contained copies survive ordinary expiry/source tidying with unavailable note; actual revocation removes/conceals online authority. Offline subtitle/media symlink escape refuses. Original watch time/one occurrence/count and newest authorized state preserved, private copies never named to another profile. Signed-out Android now uses the common capable player; test MKV/extras directly and never substitute signed-in results. Retry stays on requested version/same-source successor; unrelated playlist target/membership survives coincident entity ID.

**Failure:** Ordinary tidying deletes device copy, auth revoked yet transfer/receipt served, wrong profile art/name, per-part progress missing or skewed old evidence overrides newer state.

**Route to:** downloads, review-downloads, personal-state, server-followups, native-playback, review-server-late.

### 4. Keystore loss and ambiguous failures

1. Use disposable Android7–11/FireOS install and Android12+ separately. Sign in/save queue/preferences/profile and downloads where supported; force-stop and remove only tv.getportico.portico.records.v1 with an in-process debug instrumentation helper, retain encrypted files.
2. On older provider read ambiguity require Reset this device/Try again; restore key before confirmed Reset as a negative control. On confirmed permanent loss/newer recovery relaunch/sign in/relaunch/token renewal15min. Corrupt one ciphertext and temporarily lock/unavailable Keystore separately.
3. Interrupt cleanup after pending marker/during deletion and repeat bridge reload; inspect no plaintext credential files. Without the instrumentation helper mark injected variants Blocked; run secure-store regression file for source coverage, not physical proof.

**Expected:** Older ambiguous missing-key state requires explicit reset UI, Retry does not erase; restored key leaves original bytes/sign-in. Confirmed loss finishes cleanup before new writes/notice once and establishes new installation; generic locked/corrupt-one-record failures preserve originals. Retained download files private, no old credentials revived.

**Failure:** Automatic destructive reset on ambiguous Keystore error, stuck blank activation, old bridge invalidates new, cleanup interruption restores auth or plaintext secrets.

**Route to:** native-app, android-native-review; fix-pass native-platform.

### 5. Pending cold links, viewer fence and damaged saved connection

1. Set saved sign-in on a server taking>2s to restore; force-quit and cold-launch TopShelf/Spotlight/Siri/universal or Android/Fire WatchNext card and `portico://item/ID`/play as applicable. During restore send A thenB using xcrun simctl openurl booted URL (simulator) or adb shell am start -a android.intent.action.VIEW -d URL (Android).
2. After delivery switch profile/server/sign-out/in and ensure no replay. With server unreachable choose different viewer then reconnect/switch back; repeat offline11min and5min, signed-out sign-in<10min, running-connected immediate link and handheld portico://device.
3. On disposable saved connection use existing storage regression/instrumentation to corrupt a connection record or wrong encryption key and delay storage temporarily; sign in again/relaunch. Do not edit native encrypted files by guess.
4. Hold saved-session restore/profile/QuickConnect credential lookup then Sign out/Use another/profile/server switch; release old answer. Same-family renewed token restore must send no current-session DELETE. Hold old account-directory lookup through account switch: no request uses replacement account bearer under retired watcher.

**Expected:** Destination opens exactly once after restore, only latest pending link accepted, no replay/cross-viewer link. Ten-minute item-link fence separate from30min invitation fence. Confirmed damaged connection ends only affected sign-in and can save anew; ambiguous storage preserves originals. Late restore/selection never revives signed-out viewer; directory/extra-title caches scoped to current API/server; same-family renewal cannot revoke itself.

**Failure:** Cold link lost/duplicates, old viewer link opens after switch/expiry, endless Cannot reach for damaged connection or wrong record erased on transient storage error.

**Route to:** native-app, leftovers-native; fix-pass native, web-client-core, review-native-js.

### 6. Native/mpv handoff preserves the real queue — smoke

1. Play episode 2 in three-item queue, seek to 60s, switch native→mpv→native and Automatic repeatedly while playing, paused, loading and closing. Include rapid double preference changes.
2. Use MP4 eligible for native and MKV/DTS or PGS/ASS requiring mpv under Automatic; change quality 720p/Original and audio track; inspect session/generation/queue/current entry.
3. Close engine and allow another app’s audio to resume; repeat bridge reload/title switches with thread/memory instruments.
4. Delay play preflight/localMedia lookup and queue-start response, then offline play/live tune/new title/leave; release old success or failure and stale Dolby confirmation. Only latest intent remains; no uncertain queue response triggers a second unkeyed start.

**Expected:** Native for direct/plain-remux eligible files; mpv where avoiding conversion/lossy audio/burn. One audible engine, queue/entry preserved, position within 0.5s and pause intent retained; last setting wins, lease ends after teardown.

**Failure:** Black picture with sound, double engine, old callback replaces current, empty Up Next/new queue, lost sound or growing render threads.

**Route to:** native-playback, playback-core, review-playback-core, apple-mpv-review, android-native-review, review-native-js.

### 7. Seek, EOF and genuine failures

1. Resume cueless MKV/TS, repeated forward/back paused/playing seeks, deep and beyond-end positions, seek backward after EOF; use speed 1.5x and two audio tracks.
2. Use empty/corrupt/unsupported codec fixture and a truncated file with longer catalogue duration. Inject segment 404/403/410/503, reset/TLS failure and unspecified I/O via private proxy; Android also reclaimed codec/AudioTrack and behind-live-window.
3. Send invalid track/stale If-Match with existing session via focused API fixture; compare old media/progress and prepared-next after refusal.

**Expected:** Acknowledged seek settles once; supported real source/decode failure gets bounded fallback, output/resource/network failures never teach file/engine incompatibility. Early/unknown-duration unexpected EOF preserves resume rather than watched. Rejected command leaves old accepted playback/progress usable.

**Failure:** Pending forever, progress rewinds after ack, false complete/watch, network remembered as codec failure or old player retired on rejected replan.

**Route to:** native-playback, apple-mpv-review, android-native-review, review-playback-core, personal-state; fix-pass native-platform.

### 8. Quality, throughput, memory and receiver policy

1. Start Original, explicitly Automatic on 20 Mbps title then throttle ~3 Mbps/restore; compare throughputKbps facts/down/up generation changes and retained queue.
2. Play 4K HEVC 20min on oldest supported device plus software VC1/VP9/AV1/Hi10P and interlaced MPEG2; record frames/thermal/memory.
3. Switch HDMI 5.1/7.1 receiver→speakers/Bluetooth with TrueHD/DTS-HD/AC3/EAC3; toggle stereo/disable HDR/passthrough overrides where shown.

**Expected:** Original default; opt-in Automatic adjusts ~15s down/~minute up without reset. Declared decoder bounds match sustainable capability or honest conversion; correct channel order/PCM or actual allowed bitstream, passthrough facts withdraw with route, no impossible codec claim.

**Failure:** Automatic default, missing throughput prevents adaptation, stutter/heat termination, false software AV1 support or silent stereo/unupdated receiver facts.

**Route to:** native-playback, playback-core, planner-wiring, apple-mpv, android-mpv.

### 9. Captions, fonts, offsets and late responses — smoke

1. Play SRT/VTT/server cues, embedded/external ASS with fonts, PGS/SUP/VobSub companions/DVB. Change Portico/system size/foreground/background while paused; test italic/top/no-background then authored ASS.
2. Switch track/off while font/subtitle download delayed, set ±0.5s timing, rotate/resize where supported, use Arabic/Hebrew/Indic/Thai/CJK and mixed/untagged attachments.
3. Use private proxy: valid cues at t0, fail recheck ~25s with timeout/reset/503/truncated200, then success200/304; repeat 401/403/410/redirect/wrong MIME/oversized/malformed.

**Expected:** Latest choice only, immediate plain appearance update, authored ASS retains layout/fonts, correctly timed images and companions; bad/excess font sizes skipped within total budgets. Transient recheck preserves granted cues until ~40s then clears; valid refresh restores, authority/malformed failures clear immediately without deadline extension.

**Failure:** Stale track reappears, burn when drawing supported, unreadable declared scripts, auth loss extends cues or transient recheck clears immediately.

**Route to:** native-playback, apple-native-review, android-native-review, subtitles-text; fix-pass native-platform.

### 10. HTTP, TLS, HLS relay and credential scope

1. Connect to trusted/untrusted HTTPS plus private/Tailscale/global HTTP, IPv6 bare/bracketed/scoped; choose text and embedded-language subtitles.
2. Use a private controlled HLS source with relative/absolute variant/audio/map/segment/key references and redirect/out-of-origin/out-of-grant URIs. Inspect server/proxy request paths and zero capture-origin requests.
3. On the mpv local relay open >16 connections, idle10s, fragmented request head >16KiB and manifest >2MiB/invalid URI; concurrently switch title during old failed fetch.

**Expected:** Allowed HTTP works everywhere with normal authorization, untrusted TLS refuses; relay uses system validated client without credential escape. Excess/idle/oversize exchanges close without UI/memory growth; old failure cannot classify new title.

**Failure:** Public HTTP blocked, invalid TLS accepted, bearer capture, arbitrary localhost relay path accepted or UI hang from bounded hostile input.

**Route to:** native-sync, leftovers-native, apple-mpv-review, android-native-review, networking.

### 11. Lifecycle, focus, media session and remote queue

1. Start queued video, press Home/screen-off then return; repeat native PiP/AirPlay only if supported and music listening in background.
2. Test call/Siri/transient/permanent audio focus/ducking, headphone removal, media-services reset/activity recreation and play/pause/seek from headset/lock screen/TV remote.
3. Next/Previous in video queue (Previous within 3s prior entry, after3s restart); chapters when no queue, music/book regression, Live channel controls and no system track-next for live.

**Expected:** Video pauses when background without declared external playback, button/intent agrees and one press resumes picture; listening continues where allowed. Correct resume-only-if-was-playing, ducking/session metadata/art/clock and one active engine/session; queue ownership correct.

**Failure:** Sound behind launcher, black return/two presses, focus exception crashes, duplicate media session or video Next hits music owner.

**Route to:** apple-mpv, apple-native-review, android-mpv, android-native-review, native-playback; fix-pass native.

### 12. Freshness, renewal and deep-list focus

1. Keep Home/library/title/Saved/History open >15min through renewal with a download if supported; from second device mark/play/scan/delete and edit untouched owner setting.
2. Scroll ~2000th library item, focus fifth card/300th audio track, hold Right during updates then leave remote still >1/3s. Push title over Home, Back twice, switch root tabs and background2min.
3. Stop server >15s, renew/reconnect/feed cursor and withdraw access; test old cursor after retention maintenance.

**Expected:** One native authenticated /v1/events stream per install on supported transport (no fallback polls masquerading as success), private profiles isolated, updates ≤30s. Stable focused card/scroll/current screen/unsaved edit, covered/hidden roots quiet, token renewal no visible reload; resume cursor/authority fences correct.

**Failure:** Focus jump, retained-root rerender storm, old stream/callback survives, blank screen on renewal or private progress wakes other profile.

**Route to:** native-app, freshness-clients, freshness-server, native-sync, leftovers-native.

### 13. Saved lists, remembered views, search and sorting

1. Use 300 History/500 playlist entries and a short first page; traverse all Saved tabs/pages, reorder/remove via Menu/long press/accessibility, Clear history and refresh.
2. Play/Shuffle a saved view of120 without scrolling; repeat600, disconnect or leave mid-read. Set tab/sort/filter/late-letter jump, rail/See all/group link/profile switch/relaunch and delayed library capability read.
3. Browse every genre/series/category/collection/person/tab for video/music/books using same broad options as main library, 205+ groups and restricted/private/shared fixture. Test natural/Unicode/numeric/CJK search and half-star ratings.

**Expected:** Paged smooth lists/one title each with correct headings, short page still extends; view fetches all 120; a view beyond 500 must reach the whole selection through query capture once its screen is wired; current truncation is a known Fail. A canceled read plays nothing later. Per-profile memory/link isolation and server-advertised options, no stale capability flash, exact counts/no skips/restriction leaks.

**Failure:** Whole-list memory climb, view first page only, wrong list after renewal, group title-only sorting or unsupported old options retained.

**Route to:** native-app, native-health, native-sync, search-sort, grouping-sorts.

### 14. Resource revisions, dates, personal state and Undo

1. Use private proxy to return one 428 revision_required to setting/container/queue mutation, then real reread; test repeated428,403/malformed reread/repeated cursor/abort.
2. Rate five stars, play/seek120s, sync offline progress if supported; inspect rating10, position120000ms and UTC timestamps. Mark mixed-progress show/season/album/book, Undo <30s and retry after newer playback/membership/expiry.
3. Compare Next Up after dismiss/replay/missing file, History pause/clear/restrictions and exact See all destinations across two profiles.

**Expected:** Silent retry current body/header revision+original operation ID, ≤three mutation attempts/two rereads and none after invalid reread. Exact units/personal state; Undo atomic or genuine refusal, older evidence never wins newer intent, no private history leaks.

**Failure:** Wrong revision resource, replay changed key/body, seconds/millis drift, partial Undo or container follow-up guessed client-side.

**Route to:** native-health, personal-state, review-freshness, registry, downloads.

### 15. Accessibility, unavailable titles and account controls

1. Use VoiceOver/TalkBack arrival/back/refresh/covered page; read watched/42% cards, Actions rotor/menu for More/reorder, empty list and sheet title; largest text long book late-letter jump.
2. Play missing/restricted/nonconverting-unsupported file and transient network/expired paused lease; on TV verify Close focus/remote play disabled only for permanent failure.
3. Change device name/account details as Portico owner, allowed-hours exclusion/recovery, identity-mismatched address, member role/PIN changes and approve device.

**Expected:** Names/state once, no covered/provisional announcement, actions accessible; permanent unplayable has useful reason/Close and no retry loop, temporary failure recovers without sign-out. Live account/role rules, retained auth for hours/identity mismatch.

**Failure:** Double speech, inaccessible actions, terminal card for transient network, profile data leaks or device approval bypass.

**Route to:** native-app, native-sync, server-followups, accounts-security.

### 16. Together and Play on

1. Join two profiles, one converted member taking1–2/8/19s seeks; host seek/pause/title change while member seek pending including final30s, log120s revisions/landings/readiness.
2. Background host/guest; test takeover at30s denied/2min only fresh-ready member and10min departure, activity-private viewer and restricted item with retained position clock.
3. Phone sends Play on to TV; reject wrong-device approval/readiness, approve correct target, control pause/resume/start-over/seek/stop, kill/reopen phone and drop route.

**Expected:** Group hold≤5s/paused loads silent, no repeated pending correction, converges<750ms/end protection; hidden title sentence rather than Nothing chosen, eligibility/privacy proper. Source continues until target playback real, phone bar reconnects and target remote state shows≤5s.

**Failure:** Seek oscillation, hidden item leak/false empty room, early takeover, source stops prematurely or remote controls act only in UI.

**Route to:** social, leftovers-server-b, native-sync, native-app; fix-pass social-downloads, web-client-core.

### 17. TV activation outage, feedback and sign-out

1. On code screen block Hosted with503/Retry-After120 and then date-valued delay, compare two devices’ attempt times. Require device approval and approve from phone; show actual receiving device name/platform.
2. Send report via Settings > About, force a debug fatal JS error/relaunch and send again; hit1MiB daily account budget. Check redacted reports in owner console.
3. With local downloads if supported sign out once keeping files then deleting files; compare disk sizes, server device list and second profile access.

**Expected:** Code stays/no automatic attempt before Retry-After; ordinary protocol retry/backoff remains bounded, but a 120-second origin outage is never shortened to 30 seconds. Pending owner approval correct, feedback last-fatal/footer clears after accepted report and limit useful. Sign-out choice/privacy/files/device authority correct.

**Failure:** Code cleared, hammer retry, wrong Apple TV name on Fire/Vega, leaked tokens/fatal log or another viewer sees retained private downloads.

**Route to:** native-sync, native-app, leftovers-native, console; fix-pass web-client-core.

### 18. Gapless, gain, audiobook rewind and compressed targets

1. Generate audio extras, preserve original measured corpus; run apps/native/test/audio-gain-tests.kt with production gain source, Media3 1.10.1 dependencies, Android headers, Kotlin stdlib/Guava (main tv.getportico.portico.native.Audio_gain_testsKt); then physical +6dB/0.25peak and peak-limited playback.
2. Use LC/HE-AAC/edit-list/iTunSMPB/AAC conversion/ALAC/FLAC/MP3, exact joins/deep seeks at first/packet/EOF with reference PCM; 44.1/48/96k output, speaker/Bluetooth/HDMI/AirPlay where available. Pause book at120s with native threads running/JS suspended, wait5min/1h and headset Play; repeat explicit paused seek/timer/new session.
3. Force optional renderer failure, crossfade music then video; lower mobile bitrate below FLAC/ALAC input and inspect qualified compressed plan. Check >100 chapters/credits, embedded WEBP/local lyrics/no-lyrics and provider retry behavior.

**Expected:** Declared conformance exact frames/lag0/tolerance and seamless joins; supported PCM gain/no clipping, rewind~105/90s once, explicit seek wins without sound. Renderer fallback same session/position, video policy restored. Compressed mobile output preserves normalization/gapless/bitrate and prompt seek; absent qualified plan marks unresolved compressed-target case.

**Failure:** Priming/tail/phase error, gain bypasses PCM, stuck paused seek, double rewind, leftover music buffer/player or ordinary fallback claimed as compressed-plan pass.

**Route to:** audio-engines, music-audiobooks, native-playback, apple-native-review, android-native-review.

### 19. Native event routes and no mutation replay

1. Use private recording proxy with two proven routes. Record an unkeyed POST/PATCH/PUT/DELETE application then reset connection before reply; repeat408/503 Retry-After0, body/no body. GET/HEAD connect-before-send failures separately.
2. Join group/enable receiver, make logical route fail but proven B available; open/read/background/cancel/reload/EOF and redirect301/302/307/308 capture. Split Unicode bytes/oversize stream, hold read>60s, rotate healthy feed.
3. Start on remote route, restore LAN without path event, foreground/read; repeat LAN down interactions.

**Expected:** Unkeyed mutation at most one application/wire attempt; safe reads recover, explicit keyed retry same bytes/key one application. Feeds use proven route, cursor group only, nonce proves new server before bearer, zero redirect captures, bounded ≤four auxiliary entries/cancel/deadline no leak. LAN promotion bounded≤once/min on failure, no Hosted polling.

**Failure:** OkHttp implicit replay, bearer forwarded, stuck open/read promise, stale stream cancellation or repeated promotion disrupts playback.

**Route to:** android-native-review, leftovers-native; fix-pass native-platform.

### 20. Downloader service and notification constraints

1. Download large file on Wi-Fi-only, switch mobile/return Wi-Fi in background, pull network30s then restore; inspect partial.part under no_backup/Downloads/ID and notification.
2. On Android15 run background long enough for dataSync allowance timeout then foreground; on actual Fire tablet repeat with Fire OS background restrictions.
3. Force-stop/reopen and race extras with remove/re-add; perform signed-out offline launch separately from normal signed-in download playback.

**Expected:** Notification/foreground service appropriate, waits correctly, partial bytes kept, timeout cleanly pauses and reopening resumes without ANR. Device extras committed only by newest attempt. Signed-out offline gap tracked if unresolved.

**Failure:** ANR/kill loop, row stuck downloading, partial deleted, background Fire download absent or signed-out MKV black picture.

**Route to:** downloads, android-native-review, native-app.

### 21. Handheld layout, PiP and rotation

1. Rotate phone/tablet, change font scale, deep-scroll with largest type. On wide tablet enter Downloads then narrow layout; play native video and enter PiP only if FEATURE_PICTURE_IN_PICTURE/API26+.
2. Choose mpv and unsupported/PiP-less model; background video/music then recreate activity with actual device rotation/process pressure.

**Expected:** Touch layout/density correct, selection/scroll stable; native supported PiP continues, mpv/unsupported PiP hidden. No duplicate session, lost music service or black return.

**Failure:** TV layout on phone, unsupported PiP button, density reset or activity recreation restarts queue.

**Route to:** native-app, leftovers-native, android-native-review.

### 22. Autoplay, credits and continuous-play protection

1. Enable video continuous-play protection with count2 then3, play consecutive short episodes to that count and Continue. Disable and repeat; separately autoplay off/countdown variants.
2. Use episode with credits marker/next entry: Play now, Watch credits and credits auto-skip; film with empty queue should offer no next card. Repeat audio queue without inheriting video protection.

**Expected:** Question appears at selected count only when on; Continue counts correctly, autoplay/countdown obey preference. Authored/high-confidence chapter markers work, weak luminance marker never offered before approval; credits/card/skip behavior correct, listening keeps its own policy.

**Failure:** Dead settings, protection every title/never, weak unapproved marker shown, next card with no queue or video protection interrupts album.

**Route to:** limits-settings, web-subtitles, subtitles-text, native-playback.

### 23. Google handheld Cast and receiver autonomy

1. On eligible Google handheld with registered receiver/Cast ID start film then lock/turn off Wi-Fi/force-kill>10min. Reopen, receiver switch and bridge reload; repeat failed controller storage/identical friendly names.
2. Disconnect receiver near EOF/final report, revoke receiver pairing unattended and return controller; remove Cast configuration. Fire/tablet/TV or non-Google build must hide unsupported Cast.
3. Check real receiver token cannot GET /v1/libraries (403) but selected media/art/subtitle routes continue to work; paused/seek controls from sender change television.

**Expected:** One controller channel, same playback on reconnect/no re-pair or restart, receiver autonomy until actual end/revocation; final-report failure does not prevent idle. Cast target retains narrow authority, unsupported feature hidden.

**Failure:** Phone departure stops TV, old session cancels replacement, receiver browser gains library access or Cast promises unavailable platform.

**Route to:** social, native-sync, accounts-security, android-native-review; fix-pass web-client-core.

### 24. Wide sort/filter on every grouping

1. Prepare two libraries with at least 250 genres/series/groups and 650 playable titles; include authored playlists with duplicate entries, personal collections, artist albums/songs, author books, seasons, people with cast/crew credits, categories and Home recommendation rows. Sign in as a restricted member as well as the owner.
2. For every library tab and each named grouping, compare Sort/Filter to that scope’s server advertisement. Change Recently aired from Latest episode to Title, then duration/rating/last-played where advertised. Choose Random, page, navigate away and Back: the seed/order stays for that open selection; choosing Random again reshuffles.
3. Combine two member filters, clear one and then all. Every request must retain the grouping membership and current library/profile fence. Follow Genres/Series past 200 and sorted lists through the final page; check counts and restricted titles. A short/empty page with a next cursor must continue.
4. For a cross-library person/playlist/My List/Favorites, switch library chips or the native library control and test each library separately. Check own order before a filter, then the explicit server sort while filtered; removing the filter restores group scope. Cast/Crew does not pretend to be a supported person operand.
5. Record Home See all, personal collection, book parts/discs, native History/My List/Favorites and music Playlists separately. Current missing fields/controls are known parity gaps, not waived tests. Native role controls hide during browse; web offers the toolbar under All. Check phone/tablet and remote focus without moving the hero.

**Expected:** Every supported grouping offers all applicable advertised choices, membership and privacy survive sort/filter/paging and Recently aired is changeable. Authored own order is available unfiltered; filtered own-order preservation remains a recorded server gap. No grouping-wide parity pass while the named missing surfaces remain.

**Failure:** Missing required toolbar, a filter leaves its group, first-200 truncation, leaked denied titles, duplicate/lost pages, title-only subgroup or a false claim of own-order preservation under a filter.

**Route to:** web-viewer, native-screens, native-health-2, grouping-sorts, scale-check-2.

### 25. Receipt Undo, paused start and real remote controls

1. Mark a partially played show/season/album/book from the title page and from a card menu. Press the one Undo toast within 30 seconds; compare watched intent/resume on every member. Repeat after another device plays/edits membership, after expiry and with a double press.
2. Load a title in an already-paused Watch Together room and measure its first audible frame, including same-title reload. It must stay paused before preparation/first output, not play then pause.
3. Play on a second actual device and open Play on. Compare its reported position/duration, pause/resume, Back/Forward and Start over against the target, not the controller device. Stop elsewhere and use a retained menu; hidden actions must not be sent.
4. On a native companion device during playback change Automatic/Native/mpv; this engine selector is not a web or Vega feature. Record session ID, queue occurrence, position, pause and track/quality choices before/after the capability publication and replan. Offline files/live channels follow their supported engine behavior rather than a forced unrelated restart.

**Expected:** Undo submits its receipt and restores exactly the accepted snapshot or says why it cannot; there is one toast. Paused start is silent. Remote controls change the actual receiving player, refresh readback and use server-listed authority. Online engine changes preserve the same session/queue/position/choices.

**Failure:** Inverse mark used as Undo, two notices, expired receipt clears newer intent, first-frame audio, controls affect the phone instead of target or engine setting creates a new queue/session unexpectedly.

**Route to:** native-screens, web-viewer, web-owner, social, native-playback.

### 26. Phone/tablet integrations and owner permissions

1. Run once on a phone and once on a tablet. Owner Settings > Server has Metadata and Integrations plus the extended Libraries, People, Live TV, Storage, Troubleshooting and General. A member has no Server heading and has Profile > Personal access tokens; a secondary profile cannot gain account management.
2. Create a Read key with 30-day expiry, share/copy the one-time secret privately, reopen and prove it cannot be read again, then revoke. Create a member token, see/revoke as owner, disallow new member tokens and verify existing tokens remain revocable.
3. Create a webhook to an isolated receiver, inspect the one-time secret, send a test and watch pending/delivered log changes. Edit Off, save, rotate its secret, remove it. Inspect MCP’s local address/scopes; no Hosted account or enable switch should be needed.
4. Use the API to revoke/expire a warm cached credential without another database mutation. Read/playback keys must refuse administrative session controls/listing and owner events; an admin key can perform permitted admin operations. Same-profile personal controls remain usable.

**Expected:** Owner panels/one-time disclosures/scopes and member-token policy match readback on both device sizes; members see only personal token actions. Secrets do not reappear after reopening; legitimate local integrations work without Hosted.

**Failure:** Missing owner panel, a member sees it, repeat secret disclosure, playback key controls another member or stale expiry accepts.

**Route to:** native-owner, api-platform, security-sweep.

### 27. Phone/tablet people, metadata and file tools

1. Edit a member rating system and Weekdays 16:00–20:00 hours, reopen and check persistence. Try 0830/20.15/banana; invalid time changes nothing. Test next-day end. Outside-hours refusal preserves saved credentials. Change different fields concurrently then the same field: unrelated changes retry silently; genuine conflict retains the draft.
2. Delete a disposable member via preview. Change that member on another device before confirm; require refreshed confirmation, not automatic destructive retry. Confirm and verify exact profiles/devices removed, retained owner history names blanked.
3. Open Needs attention from a library’s exact count. Accept, manual search, provider ID and dismiss; search settles without another tap. Edit adult search/artwork limit/provider keys; secrets are never read back and invalid limits refuse. Remove a library with typed name while scanning; media files remain.
4. Split one present film/track/book file with another version, then merge a film/show/episode by searching a second title and choosing which survives. Preview/delete one version with the server’s typed token. Check member/TV cannot reach owner file actions. Native song/book-file/extra merge remains absent, record it separately.

**Expected:** Exact authorization, preview/readback, hours and rating choices; attention counts settle; typed library removal never deletes media; supported file tools preserve title/history and touch only approved files. Native merge exclusions remain documented gaps.

**Failure:** Destructive stale confirmation retried, secret readback, incorrect retained history, disappeared disk files after library removal, whole title deleted when one version chosen or false complete merge parity.

**Route to:** native-owner, accounts-security, identity, providers-matching.

### 28. Phone/tablet source mapping, runtime and reports

1. Add HDHomeRun with XMLTV URL; Check, edit the URL and require Check again. Save, refresh and compare mapped programmes by ID/number/name. Select source groups/channels using >200 choices, change 250 map rows and verify batched number/guide-ID/hidden saves.
2. Inspect skipped-import explanations, guide-day and import limits. Show actual usage/locations/upgrade copies/download space. With more profile rows than one page, record the current download-usage paging gap rather than claiming all users are shown.
3. Pause background work for an hour, resume, inspect FFmpeg requested/effective/restart status, restart with confirmation and wait for return. Verify playback/DVR/auth stay reachable during background pause.
4. Open titles set aside and client reports; read/share/delete a disposable redacted report. Compare labels and footers on phone/tablet, nested sheets, VoiceOver/TalkBack and keyboard. Check native keys/tokens/reports/attention refresh from a second owner: the current pull-to-refresh-only behavior is a freshness gap.

**Expected:** Mapped sources and batch readback match owner selection, stale check is invalidated; runtime rows actually render; reports share safely and supported actions recover. Missing quiet paged refresh/all-profile usage are recorded as Fail against the promised scope.

**Failure:** Guide URL silently omitted, more-than-200 edits lost, inert pause/status row, restart never settles, report credentials exposed or missing pages silently accepted.

**Route to:** native-owner, livetv, limits-settings, console.

### 29. Whole-view Play and Shuffle beyond loaded pages

1. Prepare a saved view and a genre/person/collection/playlist selection with at least 650 playable permitted items; include hidden/missing titles and a second profile. Load only the first page, choose a non-title sort/filter and press Play, then Shuffle with a recorded seed.
2. Compare the queue with the complete authorized browse order/membership, including items after 500; advance through the boundary. Inspect POST /v1/queues: query/saved/container identity should be forwarded, not only loaded ContentEntry IDs. Repeat from phone, tablet and each TV client that exposes the page.
3. Edit/add/remove/restrict items while capture grows, switch profile before a late answer and restart the server during capture. Unchanged source resumes; changed source that lost its snapshot preserves the accepted prefix and fails that segment. Start a new selection to capture new membership.
4. Start two long captures and saturate the background read pool while browsing/owner controls continue. A third capture can refuse retryably; request deadlines settle, and resources release. Explicit item arrays remain capped at 500; that bound is distinct from query capture.

**Expected:** A first playback window arrives promptly and later capture reaches the entire original authorized selection without duplicates/loss or 500 truncation. Already accepted occurrences stay stable under seeded shuffle. Current screen forwarding is unfinished: record a UI truncation as Fail while separately recording server-selector success.

**Failure:** A first page or first 500 is treated as the whole view, a reordered/duplicate playlist starts the wrong occurrence, old profile starts playback, membership mixes after restart or control requests hang.

**Route to:** queue-and-retention, web-viewer, native-screens.

### 30. Common signed-out player and local subtitle companions

1. Download ASS with unique attached fonts, PGS, DVB and VobSub index plus .sub. Interrupt transfer before companions complete, finish it and sign out while keeping files. Launch from the signed-out download picker using the common PlayerHost.
2. Confirm .idx is paired with .sub, incomplete VobSub is not offered and stored script/font paths remain inside the owned download folder. Change engine, seek and use chapters. Compare actual font glyphs/styles to a reference: local fonts are currently ignored by both mpv engines and must be recorded as a gap.
3. Switch to another profile/server and test receipt denial/revocation/expiry; then signed-out offline launch. Existing no-offline-next-episode queue and trusted-wall-clock gaps are separate results.

**Expected:** The common player opens supported offline formats under the saved receipt and exact account root; complete companions render where supported. Missing local font consumption is a current Fail, not a reason to hide the stored asset.

**Failure:** System-player fallback loses MKV/extras, VobSub lacks pictures, fonts wrong, wrong account files appear or receipt authority bypassed.

**Route to:** native-screens, downloads, apple-mpv-review, android-native-review.
