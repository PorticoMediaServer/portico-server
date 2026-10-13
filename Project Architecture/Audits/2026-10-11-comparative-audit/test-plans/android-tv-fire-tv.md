# Android TV and Fire TV acceptance

## Purpose and evidence

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Ubuntu build host, physical AndroidTV12+ HDR box, 32-bit FireTV Stick4K/FireOS6–7 and newer FireOS8+, HDR television, receiver5.1/7.1/Bluetooth, remote, TalkBack on supported Google model. Flavor on a Google emulator is not Fire OS evidence. API24 Google and25 Fire minimum; ABIs must match each actual device.

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

Run cases 1, 2, 5, 17 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. Release launch and capability truth — smoke

1. Install the actual candidate Release and launch signed out with Metro stopped, then direct sign-in and server/device code sign-in. Open Settings > Quality > Player and select all three choices.
2. Inspect GET /v1/me/devices/current/capabilities through a private debug log or owner device view; record version 2/native/mpv/engineSetting/actual decoder facts, not bearer. Turn force stereo on/off and inspect updated overrides.
3. Check PiP, Cast and AirPlay only on models/engines reporting support; TV and forced mpv hide handheld PiP.

**Expected:** Real modules/views load, Automatic default and Original quality; Player only if available, capability engine/depth/rate/channel/transport facts accurate and changes within about two seconds. No impossible feature shown.

**Failure:** Missing module/row, unresolved library, Release depends on Metro, duplicate capability translation or declared feature cannot run.

**Route to:** native-playback, native-app, playback-core, apple-mpv-review, android-native-review.

### 2. Native/mpv handoff preserves the real queue — smoke

1. Play episode 2 in three-item queue, seek to 60s, switch native→mpv→native and Automatic repeatedly while playing, paused, loading and closing. Include rapid double preference changes.
2. Use MP4 eligible for native and MKV/DTS or PGS/ASS requiring mpv under Automatic; change quality 720p/Original and audio track; inspect session/generation/queue/current entry.
3. Close engine and allow another app’s audio to resume; repeat bridge reload/title switches with thread/memory instruments.

**Expected:** Native for direct/plain-remux eligible files; mpv where avoiding conversion/lossy audio/burn. One audible engine, queue/entry preserved, position within 0.5s and pause intent retained; last setting wins, lease ends after teardown.

**Failure:** Black picture with sound, double engine, old callback replaces current, empty Up Next/new queue, lost sound or growing render threads.

**Route to:** native-playback, playback-core, review-playback-core, apple-mpv-review, android-native-review.

### 3. Seek, EOF and genuine failures

1. Resume cueless MKV/TS, repeated forward/back paused/playing seeks, deep and beyond-end positions, seek backward after EOF; use speed 1.5x and two audio tracks.
2. Use empty/corrupt/unsupported codec fixture and a truncated file with longer catalogue duration. Inject segment 404/403/410/503, reset/TLS failure and unspecified I/O via private proxy; Android also reclaimed codec/AudioTrack and behind-live-window.
3. Send invalid track/stale If-Match with existing session via focused API fixture; compare old media/progress and prepared-next after refusal.

**Expected:** Acknowledged seek settles once; supported real source/decode failure gets bounded fallback, output/resource/network failures never teach file/engine incompatibility. Early/unknown-duration unexpected EOF preserves resume rather than watched. Rejected command leaves old accepted playback/progress usable.

**Failure:** Pending forever, progress rewinds after ack, false complete/watch, network remembered as codec failure or old player retired on rejected replan.

**Route to:** native-playback, apple-mpv-review, android-native-review, review-playback-core, personal-state; fix-pass native-platform.

### 4. Quality, throughput, memory and receiver policy

1. Start Original, explicitly Automatic on 20 Mbps title then throttle ~3 Mbps/restore; compare throughputKbps facts/down/up generation changes and retained queue.
2. Play 4K HEVC 20min on oldest supported device plus software VC1/VP9/AV1/Hi10P and interlaced MPEG2; record frames/thermal/memory.
3. Switch HDMI 5.1/7.1 receiver→speakers/Bluetooth with TrueHD/DTS-HD/AC3/EAC3; toggle stereo/disable HDR/passthrough overrides where shown.

**Expected:** Original default; opt-in Automatic adjusts ~15s down/~minute up without reset. Declared decoder bounds match sustainable capability or honest conversion; correct channel order/PCM or actual allowed bitstream, passthrough facts withdraw with route, no impossible codec claim.

**Failure:** Automatic default, missing throughput prevents adaptation, stutter/heat termination, false software AV1 support or silent stereo/unupdated receiver facts.

**Route to:** native-playback, playback-core, planner-wiring, apple-mpv, android-mpv.

### 5. Captions, fonts, offsets and late responses — smoke

1. Play SRT/VTT/server cues, embedded/external ASS with fonts, PGS/SUP/VobSub companions/DVB. Change Portico/system size/foreground/background while paused; test italic/top/no-background then authored ASS.
2. Switch track/off while font/subtitle download delayed, set ±0.5s timing, rotate/resize where supported, use Arabic/Hebrew/Indic/Thai/CJK and mixed/untagged attachments.
3. Use private proxy: valid cues at t0, fail recheck ~25s with timeout/reset/503/truncated200, then success200/304; repeat 401/403/410/redirect/wrong MIME/oversized/malformed.

**Expected:** Latest choice only, immediate plain appearance update, authored ASS retains layout/fonts, correctly timed images and companions; bad/excess font sizes skipped within total budgets. Transient recheck preserves granted cues until ~40s then clears; valid refresh restores, authority/malformed failures clear immediately without deadline extension.

**Failure:** Stale track reappears, burn when drawing supported, unreadable declared scripts, auth loss extends cues or transient recheck clears immediately.

**Route to:** native-playback, apple-native-review, android-native-review, subtitles-text; fix-pass native-platform.

### 6. HTTP, TLS, HLS relay and credential scope

1. Connect to trusted/untrusted HTTPS plus private/Tailscale/global HTTP, IPv6 bare/bracketed/scoped; choose text and embedded-language subtitles.
2. Use a private controlled HLS source with relative/absolute variant/audio/map/segment/key references and redirect/out-of-origin/out-of-grant URIs. Inspect server/proxy request paths and zero capture-origin requests.
3. On the mpv local relay open >16 connections, idle10s, fragmented request head >16KiB and manifest >2MiB/invalid URI; concurrently switch title during old failed fetch.

**Expected:** Allowed HTTP works everywhere with normal authorization, untrusted TLS refuses; relay uses system validated client without credential escape. Excess/idle/oversize exchanges close without UI/memory growth; old failure cannot classify new title.

**Failure:** Public HTTP blocked, invalid TLS accepted, bearer capture, arbitrary localhost relay path accepted or UI hang from bounded hostile input.

**Route to:** native-sync, leftovers-native, apple-mpv-review, android-native-review, networking.

### 7. Lifecycle, focus, media session and remote queue

1. Start queued video, press Home/screen-off then return; repeat native PiP/AirPlay only if supported and music listening in background.
2. Test call/Siri/transient/permanent audio focus/ducking, headphone removal, media-services reset/activity recreation and play/pause/seek from headset/lock screen/TV remote.
3. Next/Previous in video queue (Previous within 3s prior entry, after3s restart); chapters when no queue, music/book regression, Live channel controls and no system track-next for live.

**Expected:** Video pauses when background without declared external playback, button/intent agrees and one press resumes picture; listening continues where allowed. Correct resume-only-if-was-playing, ducking/session metadata/art/clock and one active engine/session; queue ownership correct.

**Failure:** Sound behind launcher, black return/two presses, focus exception crashes, duplicate media session or video Next hits music owner.

**Route to:** apple-mpv, apple-native-review, android-mpv, android-native-review, native-playback; fix-pass native.

### 8. Freshness, renewal and deep-list focus

1. Keep Home/library/title/Saved/History open >15min through renewal with a download if supported; from second device mark/play/scan/delete and edit untouched owner setting.
2. Scroll ~2000th library item, focus fifth card/300th audio track, hold Right during updates then leave remote still >1/3s. Push title over Home, Back twice, switch root tabs and background2min.
3. Stop server >15s, renew/reconnect/feed cursor and withdraw access; test old cursor after retention maintenance.

**Expected:** One native authenticated /v1/events stream per install on supported transport (no fallback polls masquerading as success), private profiles isolated, updates ≤30s. Stable focused card/scroll/current screen/unsaved edit, covered/hidden roots quiet, token renewal no visible reload; resume cursor/authority fences correct.

**Failure:** Focus jump, retained-root rerender storm, old stream/callback survives, blank screen on renewal or private progress wakes other profile.

**Route to:** native-app, freshness-clients, freshness-server, native-sync, leftovers-native.

### 9. Saved lists, remembered views, search and sorting

1. Use 300 History/500 playlist entries and a short first page; traverse all Saved tabs/pages, reorder/remove via Menu/long press/accessibility, Clear history and refresh.
2. Play/Shuffle a saved view of120 without scrolling; repeat600, disconnect or leave mid-read. Set tab/sort/filter/late-letter jump, rail/See all/group link/profile switch/relaunch and delayed library capability read.
3. Browse every genre/series/category/collection/person/tab for video/music/books using same broad options as main library, 205+ groups and restricted/private/shared fixture. Test natural/Unicode/numeric/CJK search and half-star ratings.

**Expected:** Paged smooth lists/one title each with correct headings, short page still extends; view fetches all120, >500 warns/caps500, canceled read plays nothing later. Per-profile memory/link isolation and server-advertised options, no stale capability flash, exact counts/no skips/restriction leaks.

**Failure:** Whole-list memory climb, view first page only, wrong list after renewal, group title-only sorting or unsupported old options retained.

**Route to:** native-app, native-health, native-sync, search-sort, grouping-sorts.

### 10. Resource revisions, dates, personal state and Undo

1. Use private proxy to return one 428 revision_required to setting/container/queue mutation, then real reread; test repeated428,403/malformed reread/repeated cursor/abort.
2. Rate five stars, play/seek120s, sync offline progress if supported; inspect rating10, position120000ms and UTC timestamps. Mark mixed-progress show/season/album/book, Undo <30s and retry after newer playback/membership/expiry.
3. Compare Next Up after dismiss/replay/missing file, History pause/clear/restrictions and exact See all destinations across two profiles.

**Expected:** Silent retry current body/header revision+original operation ID, ≤three mutation attempts/two rereads and none after invalid reread. Exact units/personal state; Undo atomic or genuine refusal, older evidence never wins newer intent, no private history leaks.

**Failure:** Wrong revision resource, replay changed key/body, seconds/millis drift, partial Undo or container follow-up guessed client-side.

**Route to:** native-health, personal-state, review-freshness, registry, downloads.

### 11. Accessibility, unavailable titles and account controls

1. Use VoiceOver/TalkBack arrival/back/refresh/covered page; read watched/42% cards, Actions rotor/menu for More/reorder, empty list and sheet title; largest text long book late-letter jump.
2. Play missing/restricted/nonconverting-unsupported file and transient network/expired paused lease; on TV verify Close focus/remote play disabled only for permanent failure.
3. Change device name/account details as Portico owner, allowed-hours exclusion/recovery, identity-mismatched address, member role/PIN changes and approve device.

**Expected:** Names/state once, no covered/provisional announcement, actions accessible; permanent unplayable has useful reason/Close and no retry loop, temporary failure recovers without sign-out. Live account/role rules, retained auth for hours/identity mismatch.

**Failure:** Double speech, inaccessible actions, terminal card for transient network, profile data leaks or device approval bypass.

**Route to:** native-app, native-sync, server-followups, accounts-security.

### 12. Together and Play on

1. Join two profiles, one converted member taking1–2/8/19s seeks; host seek/pause/title change while member seek pending including final30s, log120s revisions/landings/readiness.
2. Background host/guest; test takeover at30s denied/2min only fresh-ready member and10min departure, activity-private viewer and restricted item with retained position clock.
3. Phone sends Play on to TV; reject wrong-device approval/readiness, approve correct target, control pause/resume/start-over/seek/stop, kill/reopen phone and drop route.

**Expected:** Group hold≤5s/paused loads silent, no repeated pending correction, converges<750ms/end protection; hidden title sentence rather than Nothing chosen, eligibility/privacy proper. Source continues until target playback real, phone bar reconnects and target remote state shows≤5s.

**Failure:** Seek oscillation, hidden item leak/false empty room, early takeover, source stops prematurely or remote controls act only in UI.

**Route to:** social, leftovers-server-b, native-sync, native-app; fix-pass social-downloads, web-client-core.

### 13. TV activation outage, feedback and sign-out

1. On code screen block Hosted with503/Retry-After120 and then date-valued delay, compare two devices’ attempt times. Require device approval and approve from phone; show actual receiving device name/platform.
2. Send report via Settings > About, force a debug fatal JS error/relaunch and send again; hit1MiB daily account budget. Check redacted reports in owner console.
3. With local downloads if supported sign out once keeping files then deleting files; compare disk sizes, server device list and second profile access.

**Expected:** Code stays/no automatic attempt before Retry-After; normal outage backoff~5/10/20 then24–30s jitter. Pending owner approval correct, feedback last-fatal/footer clears after accepted report and limit useful. Sign-out choice/privacy/files/device authority correct.

**Failure:** Code cleared, hammer retry, wrong Apple TV name on Fire/Vega, leaked tokens/fatal log or another viewer sees retained private downloads.

**Route to:** native-sync, native-app, leftovers-native, console; fix-pass web-client-core.

### 14. Gapless, gain, audiobook rewind and compressed targets

1. Generate audio extras, preserve original measured corpus; run apps/native/test/audio-gain-tests.kt with production gain source, Media3 1.10.1 dependencies, Android headers, Kotlin stdlib/Guava (main tv.getportico.portico.native.Audio_gain_testsKt); then physical +6dB/0.25peak and peak-limited playback.
2. Use LC/HE-AAC/edit-list/iTunSMPB/AAC conversion/ALAC/FLAC/MP3, exact joins/deep seeks at first/packet/EOF with reference PCM; 44.1/48/96k output, speaker/Bluetooth/HDMI/AirPlay where available. Pause book at120s with native threads running/JS suspended, wait5min/1h and headset Play; repeat explicit paused seek/timer/new session.
3. Force optional renderer failure, crossfade music then video; lower mobile bitrate below FLAC/ALAC input and inspect qualified compressed plan. Check >100 chapters/credits, embedded WEBP/local lyrics/no-lyrics and provider retry behavior.

**Expected:** Declared conformance exact frames/lag0/tolerance and seamless joins; supported PCM gain/no clipping, rewind~105/90s once, explicit seek wins without sound. Renderer fallback same session/position, video policy restored. Compressed mobile output preserves normalization/gapless/bitrate and prompt seek; absent qualified plan marks unresolved compressed-target case.

**Failure:** Priming/tail/phase error, gain bypasses PCM, stuck paused seek, double rewind, leftover music buffer/player or ordinary fallback claimed as compressed-plan pass.

**Route to:** audio-engines, music-audiobooks, native-playback, apple-native-review, android-native-review.

### 15. Keystore loss and ambiguous failures

1. Use disposable Android7–11/FireOS install and Android12+ separately. Sign in/save queue/preferences/profile/downloads; force-stop and remove only tv.getportico.portico.records.v1 with an in-process debug instrumentation helper, retain encrypted files.
2. On older provider read ambiguity require Reset this device/Try again; restore key before confirmed Reset as a negative control. On confirmed permanent loss/newer recovery relaunch/sign in/relaunch/token renewal15min. Corrupt one ciphertext and temporarily lock/unavailable Keystore separately.
3. Interrupt cleanup after pending marker/during deletion and repeat bridge reload; inspect no plaintext credential files. Without the instrumentation helper mark injected variants Blocked; run secure-store regression file for source coverage, not physical proof.

**Expected:** Older ambiguous missing-key state requires explicit reset UI, Retry does not erase; restored key leaves original bytes/sign-in. Confirmed loss finishes cleanup before new writes/notice once and establishes new installation; generic locked/corrupt-one-record failures preserve originals. Retained download files private, no old credentials revived.

**Failure:** Automatic destructive reset on ambiguous Keystore error, stuck blank activation, old bridge invalidates new, cleanup interruption restores auth or plaintext secrets.

**Route to:** native-app, android-native-review; fix-pass native-platform.

### 16. Native event routes and no mutation replay

1. Use private recording proxy with two proven routes. Record an unkeyed POST/PATCH/PUT/DELETE application then reset connection before reply; repeat408/503 Retry-After0, body/no body. GET/HEAD connect-before-send failures separately.
2. Join group/enable receiver, make logical route fail but proven B available; open/read/background/cancel/reload/EOF and redirect301/302/307/308 capture. Split Unicode bytes/oversize stream, hold read>60s, rotate healthy feed.
3. Start on remote route, restore LAN without path event, foreground/read; repeat LAN down interactions.

**Expected:** Unkeyed mutation at most one application/wire attempt; safe reads recover, explicit keyed retry same bytes/key one application. Feeds use proven route, cursor group only, nonce proves new server before bearer, zero redirect captures, bounded ≤four auxiliary entries/cancel/deadline no leak. LAN promotion bounded≤once/min on failure, no Hosted polling.

**Failure:** OkHttp implicit replay, bearer forwarded, stuck open/read promise, stale stream cancellation or repeated promotion disrupts playback.

**Route to:** android-native-review, leftovers-native; fix-pass native-platform.

### 17. Frame-rate/HDR late surface and Fire OS variants — smoke

1. Set Android Match frame rate Always/Seamless only/Never and start23.976fps SDR/nativeHDR10/mpvHDR10/HLG; qualification view-mount delay2s and unknown FPS.
2. On FireOS6–7 confirm no unsupported vote/switch; on FireOS8 observe actual behavior/wait. Toggle PGS/ASS off/on/off while mpvHDR direct then pause/seek/Home/return.
3. Try DV8 base and DV5 on Automatic; compare SDR TV tone-map and decoder MIME MPEG2/VC1 capability facts to MediaCodecList.

**Expected:** Opening seconds held until surface/display, readiness bounded≤10s; correct HDR→SDR rich-subtitle→HDR transitions, no DV5 Automatic mpv admission, declared decoder vocabulary mpeg2video/vc1 and bounds actual. Uncertain direct-HDR output fails qualification per model if black/frozen/wrong colour; do not silently change claim.

**Failure:** First seconds lost, stuck startup, recurring10s wait, HDR black/swapchain stretch, undeclared DV5 green output or false MIME support.

**Route to:** android-mpv, android-native-review, native-playback; fix-pass native-platform.

### 18. Remote Menu, television detection and density

1. Use actual Fire flavor or FEATURE_LEANBACK even when UI mode is not TV. Focus Play/Sign out then Menu; focus poster then Menu/hold Menu. Debug developer menu must stay closed.
2. Navigate missing-title/report/Become host/playlist reorder/long Saved lists with remote; rotate projector/change font scale and measure TV density/focus against reference.

**Expected:** TV layout/focus ring/design density, Menu on actionable card once on release and never unrelated controls, current focus retained across quiet changes. Any RN density-reset gap explicitly fails its variant.

**Failure:** Phone layout, hold repeats menu, developer menu opens, unreachable focus target or density/focus jumps.

**Route to:** native-app, leftovers-native, freshness-clients.
