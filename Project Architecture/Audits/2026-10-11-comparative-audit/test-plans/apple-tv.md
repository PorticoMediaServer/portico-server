# Apple TV acceptance

## Purpose and evidence

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Mac with tvOS SDK/runtime, physical Apple TV HD and oldest/current Apple TV4K, tvOS15.1 floor and tvOS17+ Match Content, 60Hz SDR UI plus HDR TV/HDMI receiver5.1/7.1, Siri Remote, VoiceOver. Approved signed app group/keychain for Top Shelf and real receiver for Cast. The current native downloader explicitly excludes TV; Downloads controls must be hidden and handheld download acceptance belongs to iphone-ipad.md. Do not require TV background downloads.

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

On a Mac use Node/npm, CocoaPods and Xcode 16.1+ with iOS/tvOS SDKs and installed runtimes; also archive/launch with the currently chosen shipping SDK (the later review named Xcode 27). Record the actual SDK. From the native repository check copied packages: `PORTICO_SERVER_REPO=/absolute/candidate-server node scripts/shared-packages.mjs --check`. Drift needs the integrated copied packages reviewed/synchronized on a qualification copy, not an arbitrary old server dependency. From apps/native:

```sh
npm ci --legacy-peer-deps
cd PLATFORM
pod install
xcodebuild -resolvePackageDependencies -workspace Portico.xcworkspace -scheme Portico
xcodebuild -workspace Portico.xcworkspace -scheme Portico -configuration Debug -destination 'generic/platform=SIMULATOR' -derivedDataPath /tmp/portico-accept-debug CODE_SIGNING_ALLOWED=NO build
xcodebuild -workspace Portico.xcworkspace -scheme Portico -configuration Release -destination 'generic/platform=DEVICE' -derivedDataPath /tmp/portico-accept-release CODE_SIGNING_ALLOWED=NO build
```

Replace PLATFORM with ios or tvos, SIMULATOR with iOS Simulator or tvOS Simulator, DEVICE with iOS or tvOS. Use separate derived-data paths for each platform. Resolve exact pinned MPVKit 1.1.0-n9.0.2, import Libmpv and link the full MPVKit product/transitive frameworks. Then open the workspace, set the approved team/profiles and existing App Group/Sign in with Apple capabilities, build/archive signed Release and install on the actual device. Debug needs Metro (`npm start` in apps/native, port 19424); Release must run with Metro absent. Simulator compilation is a gate before physical compilation, never playback qualification.

First-build failure checklist:

| Error or symptom | Concrete response |
| --- | --- |
| No React module / bridging-header failure | Run that platform’s pod install, open the workspace, keep inherited search/link settings and clock/subtitle/playback imports. |
| Libmpv missing or Player row absent | Resolve pinned artifacts, inspect correct simulator/device slices and full MPVKit target linkage; canImport may hide implementation despite a clean build. Require available=true at runtime. |
| FFmpeg/libass/Vulkan/MoltenVK unresolved | Link full package transitive products; do not substitute a differently licensed product or ignore checksum. |
| Swift pointer/enum/CFDictionary/callback import mismatch | Compare selected Libmpv/SDK header: command uses mutable outer nullable const-char pointers with null terminator; dictionary keys String/Int32 dimensions and nullable cue/status bridges. Inspect explicit NS_SWIFT_NAME subtitle helpers; retain semantics. |
| Swift actor/Sendable errors | Confirm Swift 5.0 target setting; a separate Swift 6 ownership migration cannot be replaced with blanket unchecked Sendable. |
| Cast selector mismatch | Reinstall locked Cast 4.8.4 pod and inspect actual headers/UI category. |
| Availability/scene launch/privacy error | Retain iOS/tvOS 15.1 floor and guarded SDK APIs; inspect current scene manifest/AppDelegate integration, app and TopShelf privacy bundles and real archive validation. Later source contains scene manifests; do not assume the earlier unresolved note still describes it. |
| mpv option startup error | Inspect named errorDetail and exact pinned backend; scope/protocol/GPU/audio errors cannot be silently skipped. Compare Metal API Validation off only as diagnostics, never HDR proof. |

The test source apps/native/test/PorticoMpvTests.swift currently has eight test methods including a Libmpv-conditional controller test. If schemes lack a test bundle, add a qualification-only iOS/tvOS Unit Testing Bundle PorticoNativeTests, host Portico, Debug testability, app bridging/search paths and full MPVKit, add that source and the bundle to scheme Test. Run `xcodebuild -workspace Portico.xcworkspace -scheme Portico -configuration Debug -destination 'platform=SIMULATOR,name=INSTALLED_DEVICE' -only-testing:PorticoNativeTests/PorticoMpvTests test`. Replace destination with an installed simulator. All eight must run; seven means the engine test was excluded. Standalone Mac guard: from apps/native run `xcrun --sdk macosx clang -fobjc-arc -framework Foundation test/native-download-attempt-tests.m -o /tmp/portico-download-attempt-tests`, then that executable. The three Foundation/Security contract commands are in .github/workflows/ci.yml’s apple-contracts job; use their exact includes. This target wiring is qualification setup, not a claim the app shipped a runnable test scheme.

From native repository root, run the three Mac-only contract helpers (outputs stay private):

```sh
swiftc -O -o /tmp/portico-url-routes apps/native/tvos/Portico/PorticoURLRoutes.swift apps/native/test/porticourlroutes-tests.swift
/tmp/portico-url-routes
clang -fobjc-arc -framework Foundation -o /tmp/portico-download-attempts apps/native/test/native-download-attempt-tests.m
/tmp/portico-download-attempts
clang -fobjc-arc -framework Foundation -framework Security -o /tmp/portico-installation-storage apps/native/test/native-installation-storage-tests.m
/tmp/portico-installation-storage
```

## Smoke subset

Run cases 1, 5, 6, 10 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. Release launch and capability truth — smoke

1. Install the actual candidate Release and launch signed out with Metro stopped, then direct sign-in and server/device code sign-in. Open Settings > Quality > Player and select all three choices.
2. Inspect GET /v1/me/devices/current/capabilities through a private debug log or owner device view; record version 2/native/mpv/engineSetting/actual decoder facts, not bearer. Turn force stereo on/off and inspect updated overrides.
3. Check PiP, Cast and AirPlay only on models/engines reporting support; TV and forced mpv hide handheld PiP.

**Expected:** Real modules/views load, Automatic default and Original quality; Player only if available, capability engine/depth/rate/channel/transport facts accurate and changes within about two seconds. No impossible feature shown.

**Failure:** Missing module/row, unresolved library, Release depends on Metro, duplicate capability translation or declared feature cannot run.

**Route to:** native-playback, native-app, playback-core, apple-mpv-review, android-native-review.

### 2. Installation marker, Keychain and transport reserve

1. Sign in/update without uninstall, then remove/reinstall on disposable device. Reboot and background-launch before first unlock; unlock/retry. Lock established install during playback.
2. Run exact apple-contracts CI helper commands on Mac; hold PIN hash/Keychain work while submitting API controls, then occupy10 reads+56waiting and issue timeline/renew/end write; cancel/deadline/reload/reused identifiers.
3. Use qualification harness for locked Keychain and cleanup interruption; compare marker/quarantine/service cleanup on retry and prepared installation. If no hook exists record physical injection Blocked and run the corresponding standalone helper only.

**Expected:** Update keeps sign-in; reinstall exposes no old auth/ID/PIN vault; unreadable locked records never wiped/marked prepared and exact service cleanup resumes before new writes. Transport control reserve independent of serial storage/read load, cancellation/callback fence settles once within original deadline.

**Failure:** Old auth after reinstall, wipe before unlock, false prepared marker, transport stalls behind PIN or stale callback cancels replacement.

**Route to:** apple-native-review, leftovers-native, native-app.

### 3. Cold links, scene lifecycle and archive validation

1. Archive/install candidate on shipping SDK; cold/warm launch permitted portico item/play/account/device/invitation links plus universal/Spotlight/Handoff/Siri where supported. Deliver same item twice<5s, then after10s/from existing title.
2. Signed out invitation then Hosted sign-in within30min and after31min; provider universal-link cold return; complete background URL session launch and verify completion handler.
3. Validate archive manifests/app groups/entitlements and iPad ITMS-90474; rejected malformed/other-server URL must not navigate.

**Expected:** One root/destination for receiving viewer, duplicate suppression correct, invite expiry/OS return and background handler preserved; both current scene targets launch cleanly and privacy/signing validation passes.

**Failure:** Blank/duplicate root, old viewer destination, OS callback lost, pending background task never completed or scene/manifest archive rejection.

**Route to:** native-app, apple-native-review, leftovers-native; fix-pass native.

### 4. Pending cold links, viewer fence and damaged saved connection

1. Set saved sign-in on a server taking>2s to restore; force-quit and cold-launch TopShelf/Spotlight/Siri/universal or Android/Fire WatchNext card and `portico://item/ID`/play as applicable. During restore send A thenB using xcrun simctl openurl booted URL (simulator) or adb shell am start -a android.intent.action.VIEW -d URL (Android).
2. After delivery switch profile/server/sign-out/in and ensure no replay. With server unreachable choose different viewer then reconnect/switch back; repeat offline11min and5min, signed-out sign-in<10min, running-connected immediate link and handheld portico://device.
3. On disposable saved connection use existing storage regression/instrumentation to corrupt a connection record or wrong encryption key and delay storage temporarily; sign in again/relaunch. Do not edit native encrypted files by guess.
4. Hold saved-session restore/profile/QuickConnect credential lookup then Sign out/Use another/profile/server switch; release old answer. Same-family renewed token restore must send no current-session DELETE. Hold old account-directory lookup through account switch: no request uses replacement account bearer under retired watcher.

**Expected:** Destination opens exactly once after restore, only latest pending link accepted, no replay/cross-viewer link. Ten-minute item-link fence separate from30min invitation fence. Confirmed damaged connection ends only affected sign-in and can save anew; ambiguous storage preserves originals. Late restore/selection never revives signed-out viewer; directory/extra-title caches scoped to current API/server; same-family renewal cannot revoke itself.

**Failure:** Cold link lost/duplicates, old viewer link opens after switch/expiry, endless Cannot reach for damaged connection or wrong record erased on transient storage error.

**Route to:** native-app, leftovers-native; fix-pass native, web-client-core, review-native-js.

### 5. Native/mpv handoff preserves the real queue — smoke

1. Play episode 2 in three-item queue, seek to 60s, switch native→mpv→native and Automatic repeatedly while playing, paused, loading and closing. Include rapid double preference changes.
2. Use MP4 eligible for native and MKV/DTS or PGS/ASS requiring mpv under Automatic; change quality 720p/Original and audio track; inspect session/generation/queue/current entry.
3. Close engine and allow another app’s audio to resume; repeat bridge reload/title switches with thread/memory instruments.
4. Delay play preflight/localMedia lookup and queue-start response, then offline play/live tune/new title/leave; release old success or failure and stale Dolby confirmation. Only latest intent remains; no uncertain queue response triggers a second unkeyed start.

**Expected:** Native for direct/plain-remux eligible files; mpv where avoiding conversion/lossy audio/burn. One audible engine, queue/entry preserved, position within 0.5s and pause intent retained; last setting wins, lease ends after teardown.

**Failure:** Black picture with sound, double engine, old callback replaces current, empty Up Next/new queue, lost sound or growing render threads.

**Route to:** native-playback, playback-core, review-playback-core, apple-mpv-review, android-native-review, review-native-js.

### 6. Display mode readiness and SDR mpv — smoke

1. Enable Match Frame Rate/Dynamic Range, start23.976fps native HDR then mpv SDR from60Hz menu; qualification harness delays view mount2s. Repeat both settings off and unknown FPS.
2. Observe held clock/opening seconds/readiness/display notifications/ten-second timeout and mode restore after close. Resize/app-window attachment and retained-route recovery; actual pipeline display trace must be captured.
3. Compare native device tone-map on SDR TV and explicit Turn off HDR server conversion; Apple TV mpv currently presents SDR with correct tone-map.

**Expected:** Playback held until picture/display ready, opening seconds retained, wait bounded≤10s and prior UI mode restored; off settings no unnecessary hold. Honest native HDR versus mpv SDR, not HDR badge assumed from source tags.

**Failure:** Opening content lost during HDMI switch, repeat full10s waits, mode never restores, black/stretched swapchain or incorrect HDR claim.

**Route to:** native-playback, apple-mpv-review, apple-native-review, playback-core.

### 7. Top Shelf authority, cache and privacy

1. With signed app group open Top Shelf from HTTP/trustedHTTPS and cold-launch card. While request active switch server/profile; offline repeat scoped cached shelf.
2. Private feed fixture sends chunked oversized feed/poster, redirect, invalid reference and same host different port/scheme, untrustedTLS and401. Inspect extension manifest/ATS/Keychain scope.
3. Validate TopShelf.appex PrivacyInfo.xcprivacy in archived app and compare cached ownership before/after auth refusal.

**Expected:** Bounded response/no redirect bearer, current viewer’s authorized art only; offline scoped cache works, unauthorized feed clears it; port/scheme changes require valid authority, one cold destination and proper manifest.

**Failure:** Prior viewer shelf leaks, oversized body memory climb, untrustedTLS accepted, redirect capture or missing extension privacy bundle.

**Route to:** apple-native-review, native-app, leftovers-native.

### 8. Seek, EOF and genuine failures

1. Resume cueless MKV/TS, repeated forward/back paused/playing seeks, deep and beyond-end positions, seek backward after EOF; use speed 1.5x and two audio tracks.
2. Use empty/corrupt/unsupported codec fixture and a truncated file with longer catalogue duration. Inject segment 404/403/410/503, reset/TLS failure and unspecified I/O via private proxy; Android also reclaimed codec/AudioTrack and behind-live-window.
3. Send invalid track/stale If-Match with existing session via focused API fixture; compare old media/progress and prepared-next after refusal.

**Expected:** Acknowledged seek settles once; supported real source/decode failure gets bounded fallback, output/resource/network failures never teach file/engine incompatibility. Early/unknown-duration unexpected EOF preserves resume rather than watched. Rejected command leaves old accepted playback/progress usable.

**Failure:** Pending forever, progress rewinds after ack, false complete/watch, network remembered as codec failure or old player retired on rejected replan.

**Route to:** native-playback, apple-mpv-review, android-native-review, review-playback-core, personal-state; fix-pass native-platform.

### 9. Quality, throughput, memory and receiver policy

1. Start Original, explicitly Automatic on 20 Mbps title then throttle ~3 Mbps/restore; compare throughputKbps facts/down/up generation changes and retained queue.
2. Play 4K HEVC 20min on oldest supported device plus software VC1/VP9/AV1/Hi10P and interlaced MPEG2; record frames/thermal/memory.
3. Switch HDMI 5.1/7.1 receiver→speakers/Bluetooth with TrueHD/DTS-HD/AC3/EAC3; toggle stereo/disable HDR/passthrough overrides where shown.

**Expected:** Original default; opt-in Automatic adjusts ~15s down/~minute up without reset. Declared decoder bounds match sustainable capability or honest conversion; correct channel order/PCM or actual allowed bitstream, passthrough facts withdraw with route, no impossible codec claim.

**Failure:** Automatic default, missing throughput prevents adaptation, stutter/heat termination, false software AV1 support or silent stereo/unupdated receiver facts.

**Route to:** native-playback, playback-core, planner-wiring, apple-mpv, android-mpv.

### 10. Captions, fonts, offsets and late responses — smoke

1. Play SRT/VTT/server cues, embedded/external ASS with fonts, PGS/SUP/VobSub companions/DVB. Change Portico/system size/foreground/background while paused; test italic/top/no-background then authored ASS.
2. Switch track/off while font/subtitle download delayed, set ±0.5s timing, rotate/resize where supported, use Arabic/Hebrew/Indic/Thai/CJK and mixed/untagged attachments.
3. Use private proxy: valid cues at t0, fail recheck ~25s with timeout/reset/503/truncated200, then success200/304; repeat 401/403/410/redirect/wrong MIME/oversized/malformed.

**Expected:** Latest choice only, immediate plain appearance update, authored ASS retains layout/fonts, correctly timed images and companions; bad/excess font sizes skipped within total budgets. Transient recheck preserves granted cues until ~40s then clears; valid refresh restores, authority/malformed failures clear immediately without deadline extension.

**Failure:** Stale track reappears, burn when drawing supported, unreadable declared scripts, auth loss extends cues or transient recheck clears immediately.

**Route to:** native-playback, apple-native-review, android-native-review, subtitles-text; fix-pass native-platform.

### 11. HTTP, TLS, HLS relay and credential scope

1. Connect to trusted/untrusted HTTPS plus private/Tailscale/global HTTP, IPv6 bare/bracketed/scoped; choose text and embedded-language subtitles.
2. Use a private controlled HLS source with relative/absolute variant/audio/map/segment/key references and redirect/out-of-origin/out-of-grant URIs. Inspect server/proxy request paths and zero capture-origin requests.
3. On the mpv local relay open >16 connections, idle10s, fragmented request head >16KiB and manifest >2MiB/invalid URI; concurrently switch title during old failed fetch.

**Expected:** Allowed HTTP works everywhere with normal authorization, untrusted TLS refuses; relay uses system validated client without credential escape. Excess/idle/oversize exchanges close without UI/memory growth; old failure cannot classify new title.

**Failure:** Public HTTP blocked, invalid TLS accepted, bearer capture, arbitrary localhost relay path accepted or UI hang from bounded hostile input.

**Route to:** native-sync, leftovers-native, apple-mpv-review, android-native-review, networking.

### 12. Lifecycle, focus, media session and remote queue

1. Start queued video, press Home/screen-off then return; repeat native PiP/AirPlay only if supported and music listening in background.
2. Test call/Siri/transient/permanent audio focus/ducking, headphone removal, media-services reset/activity recreation and play/pause/seek from headset/lock screen/TV remote.
3. Next/Previous in video queue (Previous within 3s prior entry, after3s restart); chapters when no queue, music/book regression, Live channel controls and no system track-next for live.

**Expected:** Video pauses when background without declared external playback, button/intent agrees and one press resumes picture; listening continues where allowed. Correct resume-only-if-was-playing, ducking/session metadata/art/clock and one active engine/session; queue ownership correct.

**Failure:** Sound behind launcher, black return/two presses, focus exception crashes, duplicate media session or video Next hits music owner.

**Route to:** apple-mpv, apple-native-review, android-mpv, android-native-review, native-playback; fix-pass native.

### 13. Freshness, renewal and deep-list focus

1. Keep Home/library/title/Saved/History open >15min through renewal with a download if supported; from second device mark/play/scan/delete and edit untouched owner setting.
2. Scroll ~2000th library item, focus fifth card/300th audio track, hold Right during updates then leave remote still >1/3s. Push title over Home, Back twice, switch root tabs and background2min.
3. Stop server >15s, renew/reconnect/feed cursor and withdraw access; test old cursor after retention maintenance.

**Expected:** One native authenticated /v1/events stream per install on supported transport (no fallback polls masquerading as success), private profiles isolated, updates ≤30s. Stable focused card/scroll/current screen/unsaved edit, covered/hidden roots quiet, token renewal no visible reload; resume cursor/authority fences correct.

**Failure:** Focus jump, retained-root rerender storm, old stream/callback survives, blank screen on renewal or private progress wakes other profile.

**Route to:** native-app, freshness-clients, freshness-server, native-sync, leftovers-native.

### 14. Saved lists, remembered views, search and sorting

1. Use 300 History/500 playlist entries and a short first page; traverse all Saved tabs/pages, reorder/remove via Menu/long press/accessibility, Clear history and refresh.
2. Play/Shuffle a saved view of120 without scrolling; repeat600, disconnect or leave mid-read. Set tab/sort/filter/late-letter jump, rail/See all/group link/profile switch/relaunch and delayed library capability read.
3. Browse every genre/series/category/collection/person/tab for video/music/books using same broad options as main library, 205+ groups and restricted/private/shared fixture. Test natural/Unicode/numeric/CJK search and half-star ratings.

**Expected:** Paged smooth lists/one title each with correct headings, short page still extends; view fetches all120, >500 warns/caps500, canceled read plays nothing later. Per-profile memory/link isolation and server-advertised options, no stale capability flash, exact counts/no skips/restriction leaks.

**Failure:** Whole-list memory climb, view first page only, wrong list after renewal, group title-only sorting or unsupported old options retained.

**Route to:** native-app, native-health, native-sync, search-sort, grouping-sorts.

### 15. Resource revisions, dates, personal state and Undo

1. Use private proxy to return one 428 revision_required to setting/container/queue mutation, then real reread; test repeated428,403/malformed reread/repeated cursor/abort.
2. Rate five stars, play/seek120s, sync offline progress if supported; inspect rating10, position120000ms and UTC timestamps. Mark mixed-progress show/season/album/book, Undo <30s and retry after newer playback/membership/expiry.
3. Compare Next Up after dismiss/replay/missing file, History pause/clear/restrictions and exact See all destinations across two profiles.

**Expected:** Silent retry current body/header revision+original operation ID, ≤three mutation attempts/two rereads and none after invalid reread. Exact units/personal state; Undo atomic or genuine refusal, older evidence never wins newer intent, no private history leaks.

**Failure:** Wrong revision resource, replay changed key/body, seconds/millis drift, partial Undo or container follow-up guessed client-side.

**Route to:** native-health, personal-state, review-freshness, registry, downloads.

### 16. Accessibility, unavailable titles and account controls

1. Use VoiceOver/TalkBack arrival/back/refresh/covered page; read watched/42% cards, Actions rotor/menu for More/reorder, empty list and sheet title; largest text long book late-letter jump.
2. Play missing/restricted/nonconverting-unsupported file and transient network/expired paused lease; on TV verify Close focus/remote play disabled only for permanent failure.
3. Change device name/account details as Portico owner, allowed-hours exclusion/recovery, identity-mismatched address, member role/PIN changes and approve device.

**Expected:** Names/state once, no covered/provisional announcement, actions accessible; permanent unplayable has useful reason/Close and no retry loop, temporary failure recovers without sign-out. Live account/role rules, retained auth for hours/identity mismatch.

**Failure:** Double speech, inaccessible actions, terminal card for transient network, profile data leaks or device approval bypass.

**Route to:** native-app, native-sync, server-followups, accounts-security.

### 17. Together and Play on

1. Join two profiles, one converted member taking1–2/8/19s seeks; host seek/pause/title change while member seek pending including final30s, log120s revisions/landings/readiness.
2. Background host/guest; test takeover at30s denied/2min only fresh-ready member and10min departure, activity-private viewer and restricted item with retained position clock.
3. Phone sends Play on to TV; reject wrong-device approval/readiness, approve correct target, control pause/resume/start-over/seek/stop, kill/reopen phone and drop route.

**Expected:** Group hold≤5s/paused loads silent, no repeated pending correction, converges<750ms/end protection; hidden title sentence rather than Nothing chosen, eligibility/privacy proper. Source continues until target playback real, phone bar reconnects and target remote state shows≤5s.

**Failure:** Seek oscillation, hidden item leak/false empty room, early takeover, source stops prematurely or remote controls act only in UI.

**Route to:** social, leftovers-server-b, native-sync, native-app; fix-pass social-downloads, web-client-core.

### 18. TV activation outage, feedback and sign-out

1. On code screen block Hosted with503/Retry-After120 and then date-valued delay, compare two devices’ attempt times. Require device approval and approve from phone; show actual receiving device name/platform.
2. Send report via Settings > About, force a debug fatal JS error/relaunch and send again; hit1MiB daily account budget. Check redacted reports in owner console.
3. With local downloads if supported sign out once keeping files then deleting files; compare disk sizes, server device list and second profile access.

**Expected:** Code stays/no automatic attempt before Retry-After; normal outage backoff~5/10/20 then24–30s jitter. Pending owner approval correct, feedback last-fatal/footer clears after accepted report and limit useful. Sign-out choice/privacy/files/device authority correct.

**Failure:** Code cleared, hammer retry, wrong Apple TV name on Fire/Vega, leaked tokens/fatal log or another viewer sees retained private downloads.

**Route to:** native-sync, native-app, leftovers-native, console; fix-pass web-client-core.

### 19. Gapless, gain, audiobook rewind and compressed targets

1. Generate audio extras, preserve original measured corpus; set PORTICO_AUDIO_CONFORMANCE to its absolute folder and PORTICO_AUDIO_CONFORMANCE_OUT to private JSON in Debug scheme and run each declared fixture.
2. Use LC/HE-AAC/edit-list/iTunSMPB/AAC conversion/ALAC/FLAC/MP3, exact joins/deep seeks at first/packet/EOF with reference PCM; 44.1/48/96k output, speaker/Bluetooth/HDMI/AirPlay where available. Pause book at120s with native threads running/JS suspended, wait5min/1h and headset Play; repeat explicit paused seek/timer/new session.
3. Force optional renderer failure, crossfade music then video; lower mobile bitrate below FLAC/ALAC input and inspect qualified compressed plan. Check >100 chapters/credits, embedded WEBP/local lyrics/no-lyrics and provider retry behavior.

**Expected:** Declared conformance exact frames/lag0/tolerance and seamless joins; supported PCM gain/no clipping, rewind~105/90s once, explicit seek wins without sound. Renderer fallback same session/position, video policy restored. Compressed mobile output preserves normalization/gapless/bitrate and prompt seek; absent qualified plan marks unresolved compressed-target case.

**Failure:** Priming/tail/phase error, gain bypasses PCM, stuck paused seek, double rewind, leftover music buffer/player or ordinary fallback claimed as compressed-plan pass.

**Route to:** audio-engines, music-audiobooks, native-playback, apple-native-review, android-native-review.

### 20. Cast receiver channel and controller recovery

1. With locked Cast4.8.4/registered receiver and Local Network permission, cast film then background/resume/reconnect/receiver switch/bridge reload. Kill phone>10min and return; repeat storage refusal/identical TV names.
2. Deny Local Network and use Play on > Link a TV while source plays; remove configured receiver ID and inspect feature unavailable. Pairing revoke/EOF while final report network fails.

**Expected:** One channel per current session; retired callback cannot clear it, stored reconnect same playback without re-pair/start; failed adoption leaves TV playing. Link TV works despite denied local discovery and unset Cast hides feature. Receiver stops on end/revocation.

**Failure:** Duplicate channels/new playback on reconnect, old session clears replacement, unsupported feature exposed or unattended TV stops when phone leaves.

**Route to:** apple-native-review, native-sync, social, accounts-security.

### 21. Autoplay, credits and continuous-play protection

1. Enable video continuous-play protection with count2 then3, play consecutive short episodes to that count and Continue. Disable and repeat; separately autoplay off/countdown variants.
2. Use episode with credits marker/next entry: Play now, Watch credits and credits auto-skip; film with empty queue should offer no next card. Repeat audio queue without inheriting video protection.

**Expected:** Question appears at selected count only when on; Continue counts correctly, autoplay/countdown obey preference. Authored/high-confidence chapter markers work, weak luminance marker never offered before approval; credits/card/skip behavior correct, listening keeps its own policy.

**Failure:** Dead settings, protection every title/never, weak unapproved marker shown, next card with no queue or video protection interrupts album.

**Route to:** limits-settings, web-subtitles, subtitles-text, native-playback.
