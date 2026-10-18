# apple-mpv-review

## 1. Outcome

Completed the source review and corrections in the named worktree on ehler-homelab-mini-1 (personal Ubuntu host). No commit, push, deployment, full build or full test suite was run. Snapshots are cumulative against this worktree's HEAD; use the final snapshot as the complete patch.

The Apple mpv implementation now retains its rendering surface until C teardown ends, initializes mpv away from the main thread, waits for audio teardown before releasing an engine lease, handles either ordering of seek completion events, removes obsolete subtitle additions, bounds font transfers, and keeps HLS references on the system-validated relay. Apple TV startup waits for its view before matching the display. Capability declarations no longer give Apple TV HD a 4K H.264 decoder or assume HEVC hardware after a failed probe.

Engine handoffs now wait for old leases and outstanding acquisitions. Changing the Player setting uses the existing current-entry recovery operation, keeping Up Next and the position; it also preserves pause intent. Capability publications are serialized, and only the matching preference revision wakes a re-plan.

**Evidence boundary:** JavaScript checks pass. Both Xcode project files parse and their wiring checks pass. Native code and the new Swift tests have not been compiled or run: there is no Swift compiler, Apple SDK or Xcode on this host.

Milestones saved: snapshots 001–005; final cumulative snapshot 006 records the completed tree.

## 2. Decisions I made

- Preserve the engine contract's command, event and field shapes. The only semantic exception is allowing external subtitles/fonts over HTTP on any host, as the shared programme explicitly requires. The contract's private-address restriction needs correcting by the orchestrator.
- Destroy mpv at lease release instead of retaining an idle handle for the next title. Upstream audiounit uninit deactivates AVAudioSession; allowing it to happen after the replacement engine starts can silence that engine. The release promise now finishes after C destruction. Startup costs may increase slightly; correctness wins here.
- Keep MPVKit exactly at **1.1.0-n9.0.2**, LGPL product **MPVKit**. Its binary module is **Libmpv**, not MPVKit. No dependency or project-file change was needed.
- Keep the Metal/Vulkan output path. The pinned package really patches in the moltenvk context and treats wid as an unretained CAMetalLayer pointer. The libmpv render-context API has OpenGL/software backends; this implementation does not use it.
- Keep raw structured-property JSON reads. An early suspicion that these strings were display text was disproved by mpv 0.41 options/m_option.c: print_node calls json_write. I corrected the progress update and retained the valid code. Added a 2 MiB parse bound.
- Ignore only option-not-found for osc, ytdl and load-scripts. The pinned build disables Lua on iOS/tvOS, and these options are conditionally compiled. Every other failed startup option aborts startup with its option name in the technical detail.
- HLS is allowed to follow references only for a remote .m3u8 source, with the HLS demuxer forced and the relay rewriting every URI. Direct/local files cannot open referenced resources. The FFmpeg protocol lists are limited to file/http/tcp/crypto; HTTPS happens in URLSession. Local playlists are deliberately not granted unrestricted access to other app files.
- Relay bounds: 16 simultaneous exchanges, 16 KiB request heads, 10 seconds to finish an idle request head, 2 MiB manifests, and existing 4 MiB/1 MiB streaming watermarks. Non-grant addresses grant the source itself, not its parent folder.
- Fonts stream to temporary files rather than buffering all data in memory. The budget is 24 MiB total across at most 16 fonts, with the existing 10/20 second request/resource timeouts. Cancellation is asynchronous, so an in-flight network chunk can exceed the transfer threshold; stored fonts stay within the total budget.
- A single H.264 codec entry cannot express separate 8-bit hardware and 10-bit software limits. When both depths are offered, use the software limit for both. This is conservative for 8-bit H.264 (1080p30 on the standard tier, 4K30 on the strong tier); per-depth limits belong in the shared capability document. Failed probes use the light baseline with no invented HEVC hardware.
- Use V1QueuePlayer.check(true) to re-open the same entry instead of PlaybackService.play(), which calls playItem() and creates a new single-title queue. A dedicated settings/re-plan API would still be better for preserving all delivery options.
- Context refresh failed because its lock directory is read-only. Existing project standards, project notes and verification guidance were loaded directly. Both lane/programme briefs were read completely.

## 3. Bugs

### Defects found and corrected

Line numbers refer to the final worktree. Paths below are relative to the repository.

| File and line | What was wrong | Correction |
| --- | --- | --- |
| apps/native/Native/PorticoMpvCore.swift:60 | C held an unretained layer pointer while destruction was asynchronous; controller destruction could free the layer first. | The core retains the layer through termination. |
| PorticoMpvCore.swift:84 | mpv creation/options/initialization ran on the main thread; rejected options were ignored. | Startup runs on the serial core queue; failures reach the load error path, with the failing option identified. Conditional scripting options are handled explicitly. |
| PorticoMpvCore.swift:181,246 | C command pointer conversion was implicit; the headers require const char **, whose outer pointer is mutable in Swift. | Use a null-terminated array through withUnsafeMutableBufferPointer. This is checked against the actual pinned headers; compiler validation remains outstanding. |
| PorticoMpvCore.swift:204; PorticoMpvController.swift:164; PorticoMpvPlaybackModule.swift release method | Release resolved before audiounit uninit could deactivate the shared audio session. | Retire/destroy the core; resolve release only after termination; refuse acquisition during that teardown. Clear queued event delivery immediately. |
| PorticoMpvCore.swift:119 | Every wakeup could enqueue another drain even when one was already queued. | Coalesce wakeups with a short lock; the callback never calls mpv or UI APIs. |
| PorticoMpvCore.swift:285 | Structured-property parsing had no document-size bound. | Refuse JSON over 2 MiB. This bounds application parsing, not mpv's own property allocation. |
| PorticoMpvController.swift:748 | A restart arriving before its successful asynchronous seek reply could leave the seek pending forever. | Record both acceptance and restart; finish when both have arrived and seeking is false, regardless of ordering. |
| PorticoMpvController.swift:620 | EOF before the first restart was ignored and might never be announced again. | Recheck eof-reached on the periodic tick, preserving the existing single-ended guard and transport-failure classification. |
| PorticoMpvController.swift:407 | Default audio layout negotiation could downmix, and audiounit could override playback with MixWithOthers. | Request original accepted layouts (audio-channels=auto) and audio-exclusive=yes; the latter prevents audiounit's MixWithOthers category override. |
| PorticoMpvController.swift:413,518; PorticoMpvRelay.swift:421; PorticoMpvSupport.swift:73 | Absolute HLS segment/key/rendition references could bypass URLSession and its system TLS validation. | Buffer bounded manifests; resolve, scope-check and rewrite all URI attributes and media lines to loopback. Refuse malformed/variable/out-of-scope references. Block references in non-HLS files and bound FFmpeg protocols. Fetch known manifests without Range headers and also handle 206 manifest answers, so range requests cannot bypass rewriting. |
| PorticoMpvRelay.swift:136,273 | A non-media source granted its whole folder, potentially the entire origin; the constructed URL was not independently rechecked. | Exact-source scope outside media grants; validate the resolved scheme, host, port, credentials, fragment and path again. |
| PorticoMpvRelay.swift:226,250 | Connections could accumulate; a complete oversized header was routed before its size check. | Cap exchanges and idle header time; enforce the head limit before routing, including the final received chunk. |
| PorticoMpvSupport.swift:59 | New rich subtitle/font fetches rejected HTTP to a public hostname despite programme policy. | Accept HTTP/HTTPS while preserving origin, grant, credential and traversal checks. |
| PorticoMpvSupport.swift:230,276,284,298 | Font data tasks buffered unbounded responses before checking size; up to 16 individual 24 MiB fonts could be accepted; stale callbacks could write after cancellation. | Streaming download delegates, a total budget, identity checks on every callback, and delegate invalidation when finished. |
| PorticoMpvController.swift:1125 | A late font callback could mutate a newer subtitle selection's font directory. | Fence it by subtitle token and target; clear the font directory when the old external subtitle is removed. |
| PorticoMpvController.swift:1058 | Embedded title and font identity were omitted from the subtitle selection key. | Include them, allowing a more specific match or changed font list to be applied. |
| PorticoMpvController.swift:1035,1058 | An old sub-add could finish after replacement/off, select itself, and remain attached. | Remove stale external tracks on track-list changes and restore the current embedded/external selection. Failed adds now clean up before falling back. |
| PorticoMpvController.swift:1106 | A local VobSub idx was checked, but its automatically opened sub companion could resolve outside Downloads. | Validate the companion separately after symlink resolution. |
| PorticoMpvController.swift:1213 | A fast load without an attached view announced ready before television matching; a late match did not immediately pause. | Hold readiness for attachment (bounded), then match; pause immediately when matching starts; fence delayed work by load epoch. |
| PorticoMpvController.swift:1267; PorticoMpvSupport.swift imports | CFString dictionary keys are a likely Swift Hashable compile failure, and CF type functions depended on transitive imports. | Use String keys bridged to CFDictionary; import CoreFoundation explicitly. |
| apps/native/src/bridge/NativePlaybackCoordinator.ts:42,110,155,307 | New engines could acquire before the old release or a stale acquisition finished, including stop/start cycles. | Await release/acquisition barriers; rejected release prevents replacement acquisition. |
| apps/native/src/player/usePlayer.ts:354; engine-replan.ts:7 | Player-setting re-plan called play(), replacing Up Next with one title; it could also run after a later setting/title change and resume a paused title. | Reopen the current queue entry; preserve pause immediately after starting the new intent; reject stale setting/session/phase callbacks. |
| apps/native/src/bridge/capability-publisher.ts:30; player/engine-setting-store.ts:28 | Concurrent capability PUTs could overwrite the newer setting; any earlier publication woke all waiters; expired waiters remained retained. | Serialize writes; fence probes and settlements by preference revision; include queued initial preference writes in readiness; remove timed-out waiters. |
| apps/native/src/bridge/mpv-profile.ts:63,104 | Apple TV HD claimed 4K H.264; failed probes invented HEVC hardware; software Hi10P inherited hardware limits. | Conservative light fallback and depth-safe limits; label combined availability/estimated-limit evidence mixed. |

### Existing B entries

Read the B-20 entries, BUG-REGISTER and the relevant FIX-LOG entries. These are source confirmations, not device reproductions. B-19 is included because the predecessor report names it; native application work remains outside this lane.

| Bug | Status |
| --- | --- |
| B-20.1 | Not fixed here — planner/Android passthrough ownership. mpv explicitly declares no passthrough. |
| B-20.2 | Already fixed on this base at the client boundary: version-2 native/mpv profiles carry tone mapping. Server acceptance is another lane's. |
| B-20.3 | Already fixed for LAN cue documents; extended mpv rich subtitles/fonts to public HTTP in this lane. The shared cue adapter's private-host restriction remains another lane's responsibility. |
| B-20.4 | Not fixed — Android lifecycle is outside this lane. |
| B-20.5 | Fixed for mpv's late-view path in this lane. AVPlayer still applies criteria after readiness; not changed here. |
| B-20.6 | Already fixed on the base for AVPlayer; mpv retains equivalent HTTP/network/uncertain classification. Relay TLS bypass corrected here. |
| B-20.7 | Already fixed on the base: both engines emit systemPaused and the player changes intent. Device acceptance remains. |
| B-20.8 | Already fixed on the base according to FIX-LOG; Android code not modified here. mpv uses mpeg2video/vc1. |
| B-20.9 | Already fixed at the client boundary: native HEVC codecTags are hvc1/dvh1. Server planner ownership remains. |
| B-20.10 | Not fixed — Android output probe is outside this lane. |
| B-20.11 | Already fixed at the new profile boundary: native rendering says text only, mpv has its own rich rendering list. Android implementation is another lane's. |
| B-20.12 | Not fully fixed — predecessor reads system caption size for mpv. Colour/background/other appearance settings and AVPlayer overlay styling are outside this review. |
| B-20.13 | Already fixed on the base: transient cue refresh failure keeps loaded cues and retries. |
| B-20.14 | Not fixed for Android. Corrected the analogous mpv Hi10P/unknown-probe/Apple TV HD overclaims here. Throughput still needs measurement. |
| B-20.15 | Partly already fixed: chapter facts reach system media; false old feature flags are absent from the new document. Timeline chapter marks/thumbnails remain outside this lane. |
| B-20.16 | Already fixed on the base: video next/previous use queue navigation. Queue preservation during engine preference changes corrected here. |
| B-20.17 | Already fixed on the base: profile default speed is applied. |
| B-20.18 | Already fixed on the base: pre-play Dolby Vision check is bounded to 1.5 seconds. |
| B-20.19 | Not fixed — Android extractor behavior is outside this lane. mpv keeps the agreed exact-seek rule; cue-less files need device testing. |
| B-20.20 | Not fixed — Android active-route passthrough probe is outside this lane. mpv does not claim bitstream output. |
| B-20.21 | Not fixed — Android music crossfade engine ownership is outside this lane. |
| B-20.22 | Already fixed on the base for video session release, including AVPlayer's check of PorticoEngineClock.active. Corrected the separate mpv AudioUnit teardown race here. |
| B-20.23 | Already fixed on the base: AVPlayer removes observers from _observedItem. |
| B-19.1 | Not fixed in this lane — startup links belong to native-app; FIX-LOG records earlier fixes and remaining gaps. |
| B-19.2 | Not fixed — native-app service lifetime. |
| B-19.3 | Not fixed — Android remote input. |
| B-19.4 | Not fixed — Android Keystore; FIX-LOG records partial earlier repair. |
| B-19.5 | Not fixed — artwork retry behavior. |
| B-19.6 | Not fixed — global JavaScript error handling. |
| B-19.7 | Not fixed — iPad orientation/upload validation. |
| B-19.8 | Not fixed — Android TV density. |
| B-19.9 | Not fixed — unrelated Apple storage/network queue ownership. |
| B-19.10 | Not fixed — device form detection. |
| B-19.11 | Not fixed — Vega storage. |
| B-19.12 | Not fixed — app deletion/sign-in lifecycle. |
| B-19.13 | Not fixed — windowed grid layout. |
| B-19.14 | Not fixed — mounted-screen lifetime. |
| B-19.15 | Not fixed — broader native test harness; this lane supplies a Swift test file but does not claim a native run. |
| B-19.16 | Not fixed — broader string catalogue and predecessor English-only Player stand-ins need shared-package integration. This lane adds no viewer text. |
| B-19.17 | Not fixed — Hermes plural behavior. |
| B-19.18 | Not fixed — scene manifest/SDK launch acceptance. |
| B-19.19 | Not fixed — unrelated native-app contract leftovers. |

## 4. API changes

No server route, payload shape, command/event shape or route removal. Existing version-2 capability PUTs are serialized. Release still resolves true and ignores a non-current lease; a current mpv lease resolves only once C teardown has finished.

Player-setting re-plan now uses the existing queue current-entry recovery/start path with replacesSessionId. It does not create a new queue. No packages/* files were edited.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

No new setting or screen. Existing device Player setting (Automatic default, Native, mpv) now safely publishes and applies mid-title without discarding Up Next or silently resuming a paused title. The existing settings/player controls remain those supplied by the predecessor.

## 7. Needed from others

1. Shared client-core must preserve PlaybackSession.engine from the server presentation; until that is integrated, Automatic's full conversion/lossy-audio/burn-in choice cannot use the server's named engine. The local download/subtitle fallback remains.
2. Merge version-2 capability types and subtitle deliveries/renderer/choose(rendering) into the canonical shared packages, then replace the stand-in readers/cast. Do not edit these copied packages in this worktree.
3. Supply the dedicated current-session re-plan operation. The existing current-entry recovery keeps queue and position, but its current shared-package implementation does not carry all per-session quality/version/audio selection controls. The old play() path also discarded these queue arguments. Wire them in the canonical package and migrate engine-replan.ts.
4. Extend capability limits by bit depth/profile if 8-bit hardware H.264 and software Hi10P should have different ceilings. Until then both advertised depths share the safer limit.
5. Expose per-engine output ranges to the planner if it should distinguish Apple TV mpv SDR from native HDR. The predecessor's single display document cannot express that distinction.
6. Replace the predecessor's English-only EngineSettingRow/engine-strings with shared settings and i18n catalogue entries.
7. Correct ENGINE-CONTRACT.md sections 7/9: external subtitle/font HTTP is accepted on every host under the same-origin/grant rules. Source remains restricted; redirects remain refused. The original-source cue adapter still enforces the older private-host rule and needs the responsible native-playback lane's update.
8. Apple iOS/tvOS Info.plist still has NSAllowsArbitraryLoads=false plus NSAllowsLocalNetworking=true. The networking/native-app lane must wire public-hostname HTTP under programme policy; the mpv URLSession relay inherits ATS. When enabling arbitrary HTTP, account for Apple's fine-grained ATS keys overriding the broad key.
9. AVPlayer display-start gating, complete caption appearance, timeline marks/thumbnails, Android lifecycle/probes and native download integration remain their respective lanes.
10. Architecture 04 Playback and Media / 06 Clients and the PiP/AirPlay product documents need the predecessor's two-engine correction. MPV Metal video does not offer PiP or AirPlay video.
11. The new Swift XCTest file is ready to add to an Apple unit-test target. No existing native unit-test target was available, so target creation/test execution belongs to the Xcode verification pass.

## 8. Test notes for other environments

### Checked here

- npx tsc --noEmit in apps/native: passed.
- npx tsc -p test/tsconfig.json --noEmit: passed.
- Six individual test files, **36 tests passed**: engine-choice (6), engine-switch (10), mpv-profile (7), engine-media (7), engine-replan (3), engine-setting-store (3). No whole test suite.
- npm run lint:strings: passed; zero hard-coded viewer strings; only pre-existing logging/diagnostic warnings.
- git diff --check: clean.
- Both project.pbxproj files parsed with the xcode OpenStep parser installed only under /tmp. All eight engine source references exist and occur once in each Portico target; target package product, exact pin, Debug/Release Swift 5 language setting and bridging-header path agree.
- A supplementary tree-sitter Swift scan reported zero ERROR/missing nodes for all six engine Swift files and the Swift test file. Its Node process subsequently failed with a V8 Zone out-of-memory error on two attempts; this is recorded as an incomplete tool check, not a passing compiler check. No core dump appeared in the worktree.
- No native build, Swift test run or device playback occurred.

Run an individual JavaScript file from apps/native:
node --experimental-strip-types --import ./test/register.mjs --test test/engine-switch.test.ts

### API and dependency evidence

Downloaded evidence is under /tmp/portico-apple-mpv-review (not shipped in the worktree).

- [Pinned MPVKit manifest](https://github.com/mpvkit/MPVKit/blob/1.1.0-n9.0.2/Package.swift): MPVKit product targets _MPVKit, depending on Libmpv and the complete LGPL codec/render dependencies; iOS/tvOS minimum 15, Swift tools 5.9.
- [Pinned build source](https://github.com/mpvkit/MPVKit/blob/1.1.0-n9.0.2/Sources/BuildScripts/XCFrameworkBuild/main.swift): mpv v0.41.0, mobile Lua disabled, VideoToolbox/libplacebo, audiounit and avfoundation enabled.
- [MoltenVK patch](https://github.com/mpvkit/MPVKit/blob/1.1.0-n9.0.2/Sources/BuildScripts/patch/libmpv/0001-player-add-moltenvk-context.patch): wid bridges to an unretained CAMetalLayer; GPU context name moltenvk.
- Actual [Libmpv binary release](https://github.com/mpvkit/MPVKit/releases/download/1.1.0-n9.0.2/Libmpv.xcframework.zip) checksum: 1d8651bebfe467bd5408751647de692cdacb1d19ba825e11785d770bc88c3225, exactly matching the manifest. Module map exports Libmpv. Device and simulator slices exist for iOS and tvOS.
- The release client.h/render.h are byte-identical to [mpv v0.41 client.h](https://github.com/mpv-player/mpv/blob/v0.41.0/include/mpv/client.h) and [render.h](https://github.com/mpv-player/mpv/blob/v0.41.0/include/mpv/render.h).
- Checked all used functions: create, initialize, set_option/string, request_log_messages, observe_property, set_wakeup_callback, wait_event, get_property/string, error_string, command/command_async, free, terminate_destroy. Formats, event constants, end reasons, playlist_entry_id, error, reply_userdata, and callback signature match those headers. Command strings are copied by the async API; temporary argument storage is released after the call. Event C pointers are read/copied before the next wait_event.
- Wake callbacks may run on arbitrary threads and must not call mpv: ours only locks briefly and queues a drain. All C calls now use the core queue; controller/UI/facts use main; relay state uses its serial queue and its URLSession delegate queue; font delegates use main. Core destruction retains the C callback context until termination.
- [Input/property documentation](https://github.com/mpv-player/mpv/blob/v0.41.0/DOCS/man/input.rst) confirms loadfile's replace/-1/options arguments, exact seek flags, sub-add/sub-remove, track-list fields, playlist id and the observed property names. ao-reload exists but is explicitly experimental/internal upstream.
- [Option documentation](https://github.com/mpv-player/mpv/blob/v0.41.0/DOCS/man/options.rst), options/options.c, stream/stream_lavf.c, video/out/vo_gpu_next.c and builtin.conf were checked for the configured options and fast profile. deinterlace=auto is valid in 0.41; it was not changed. Percent-length escaping for the comma-containing protocol whitelist matches the key/value parser.
- [audiounit source](https://github.com/mpv-player/mpv/blob/v0.41.0/audio/out/ao_audiounit.m) confirms default MixWithOthers unless exclusive, requested source channel layout, and AVAudioSession deactivation at uninit.
- Apple [AVDisplayCriteria initializer](https://developer.apple.com/documentation/avfoundation/avdisplaycriteria/init(refreshrate:formatdescription:)) was verified against the Apple documentation data and SDK header: Float refresh rate, CMFormatDescription, tvOS 17. UIScreen EDR/refresh-rate signatures were checked against the SDK header. CoreMedia dimensions are Int32 and the output description is optional.
- React Native's installed RCTTurboModuleManager dispatches invalidation to methodQueue; this module specifies main. Project targets use Swift 5, with no complete Swift 6 sendability setting found.

### First build, on the Mac

Use a Mac checkout containing this final snapshot and the integrated native programme changes. Use Xcode 16 or newer with both iOS and tvOS SDKs/runtimes installed; do not use Linux to substitute for Apple compilation.

From that checkout:

    cd apps/native
    npm ci --legacy-peer-deps
    cd ios
    pod install
    xcodebuild -resolvePackageDependencies -workspace Portico.xcworkspace -scheme Portico
    xcodebuild -workspace Portico.xcworkspace -scheme Portico -configuration Debug -destination 'generic/platform=iOS Simulator' -derivedDataPath /tmp/portico-mpv-ios CODE_SIGNING_ALLOWED=NO build

Then from apps/native:

    cd tvos
    pod install
    xcodebuild -resolvePackageDependencies -workspace Portico.xcworkspace -scheme Portico
    xcodebuild -workspace Portico.xcworkspace -scheme Portico -configuration Debug -destination 'generic/platform=tvOS Simulator' -derivedDataPath /tmp/portico-mpv-tvos CODE_SIGNING_ALLOWED=NO build

After simulator compilation, compile the same two schemes with destinations generic/platform=iOS and generic/platform=tvOS (separate derived-data folders, CODE_SIGNING_ALLOWED=NO), then run signed builds on an iPhone/iPad and Apple TV. MPVKit downloads substantial binary dependencies on resolution.

Likely failure points and concrete next actions:

- Libmpv unavailable/import skipped: confirm the **MPVKit** product is linked to the Portico app target and every pinned artifact downloaded. The source imports **Libmpv**, correctly. canImport intentionally hides the implementation when missing, so a successful build alone is insufficient: check the exported available constant and that the Player row appears.
- Pointer conversion in mpv_command/command_async: use the actual selected framework header; its outer argument is mutable const char **. The source now uses UnsafeMutableBufferPointer<UnsafePointer<CChar>?>. The array always includes a null terminator; do not replace it with a pointer to the Swift Array object.
- CoreMedia dictionary conversion: keys are now String, bridged as CFDictionary, with Int32 dimensions. If the local SDK reports a different import, adjust only that typed bridge against its header.
- Objective-C callback types: the app bridging header must import PorticoEngineClock.h, PorticoSubtitleSelection.h and PorticoPlaybackController.h. If onText imports NSDictionary rather than the Swift dictionary bridge, explicitly bridge its cue before assigning overlayCue; keep the nullable cue/status semantics.
- Swift 6 actor/sendability errors: verify the app target is still Swift 5 in both configurations, as this snapshot specifies. Enabling Swift 6 is a separate migration: main-owned controller/module/view state and nonisolated C callback forwarding need explicit actor annotations, not broad unchecked-sendable declarations.
- Unresolved Vulkan/FFmpeg/libass symbols: link the full MPVKit product, including its transitive package targets, rather than manually adding Libmpv alone. Keep inherited linker flags and the package's framework/library settings.
- Runtime startup option failure: inspect errorDetail for the option name. Confirm the exact 0.41 package/backend; only absent scripting switches are intentionally skipped. Do not silently skip GPU, audio or scope-related failures.
- GPU validation diagnostics on HDR: compare with Metal API Validation disabled in the Run scheme, as the predecessor recommends; this does not establish HDR correctness.

To run the new Swift tests: in each Xcode project add an iOS/tvOS Unit Testing Bundle named PorticoMpvTests, Host Application Portico, Debug testability enabled; add test/PorticoMpvTests.swift to that target. Give it the app's CocoaPods module search paths and MPVKit package product so canImport(Libmpv) also enables the controller test. The file has six helper tests and one conditional controller fencing test. Select an installed simulator and run:

    xcodebuild -workspace Portico.xcworkspace -scheme Portico -configuration Debug -destination 'platform=iOS Simulator,name=<installed iPhone simulator>' -only-testing:PorticoMpvTests test

Use the analogous tvOS destination in the tvos workspace. Add the test bundle to the Portico scheme's Test action first. Passing means seven tests run with mpv linked; six tests alone means the C-module condition excluded the controller test.

### Device acceptance checklist

1. **Startup/build:** both targets compile and expose available=true; MP4 remains native on Automatic; MKV starts in mpv without a video conversion. Fail: missing row/module, startup option error, or sound over black.
2. **Actual queue preservation:** start a series queue with at least three entries, seek to a known position, switch Player native → mpv → native. Pass: same queue/current entry, remaining Up Next entries unchanged, position within 0.5 seconds. Repeat while paused: it stays paused. Change preference twice quickly: only the last setting wins. Inspect server queue ids and capability engineSetting.
3. **Teardown:** alternate engines/titles rapidly, including closing while loading and bridge reload. Pass: no crash, leaked renderer or silent replacement; one audible engine. Run Xcode memory graph and Address Sanitizer where supported. After closing, another app's audio may resume.
4. **Seek/EOF:** resume, repeated forward/back seeks while paused/playing, seek near EOF, seek backwards after EOF. Pass: seek settles once, no indefinite pending state, ended once at true completion, and progress does not rewind after the acknowledgement. Check cue-less MKV/TS and beyond-end seeks.
5. **HLS/TLS/scope:** HTTPS direct and TS/fMP4 HLS with relative AND absolute variant/audio/map/segment/key URLs all play. Use a trusted certificate. Repeat with an untrusted certificate: refusal, never successful playback. Test HTTP at both a private IP and a public hostname (integrated ATS settings required). Out-of-origin/grant HLS references and redirects must be refused, and server/proxy logs should show system relay fetches.
6. **Relay bounds:** send a fragmented head over 16 KiB, stall an empty connection, and open more than 16 connections. Pass: excess/idle requests close without UI stalls. Serve a manifest over 2 MiB and an invalid URI attribute: refusal, no external fetch. MPV cache suspension should not grow relay memory indefinitely.
7. **Subtitles:** embedded/external ASS with attached fonts, SRT, PGS, VobSub/DVB; test positive timing offset. Switch repeatedly while fonts/subtitles are still downloading, then turn off. Pass: only the latest choice is visible; no stale track reappears. Missing/broken rich subtitles use cues when available. Font transfers over the total budget are canceled/skipped without large memory growth.
8. **Offline VobSub:** idx and sub inside Downloads work; a companion symlink outside Downloads is refused. Original local video/subtitle symlinks must remain confined.
9. **Apple TV matching:** tvOS 17+, Match Frame Rate on, 23.976 fps title from 60 Hz UI. Pass: playback remains held until the view/display is ready, including a deliberately delayed view mount; no opening seconds disappear into the HDMI switch; old mode restores after close. Retained-route recovery still has its existing ten-second timeout.
10. **Display/graphics:** iPhone/iPad rotate and resize during playback; tvOS app/window attachment changes. Pass: correctly sized picture, no stretched/stale swapchain or render crash. The pinned moltenvk patch only reconfigures size on VO reconfig, so this needs particular attention.
11. **HDR:** HDR10/HLG and Dolby Vision 5/7/8 on HDR iPhone/iPad; Apple TV expected SDR tone mapping. Pass: correct colours/highlights without green/purple output. Test EDR changes/Low Power Mode/external display. Dolby Vision profile 7 enhancement-layer fidelity is not established.
12. **Audio/system:** DTS-HD/TrueHD 5.1/7.1 to a receiver, then headphones/Bluetooth; speed changes; Siri/call interruption; headphone removal; Home/foreground; lock-screen controls and chapters. Pass: expected PCM channels, no unexpected mixing, correct pause/resume intent, clock and remote commands. ao-reload behavior needs real output changes.
13. **Loss/recovery:** lose the network during playback and reconnect on another route. Pass: retained position and queue, no false completed/watch mark, no format downgrade for an HTTP/TLS/network error. Include unknown-duration HLS, which is not covered by the current early-EOF duration heuristic, plus IPv6 HTTP/HTTPS server origins.
14. **Performance:** older supported devices, interlaced MPEG-2/VC-1, software VP9/AV1/Hi10P. Pass: sustainable frame delivery, no combing/heat termination or runaway memory. Capability size/frame-rate limits are estimates, not benchmarks.

## 9. Not finished

- Native compilation, Swift test target wiring/execution, signed device playback, GPU/display/audio/thermal acceptance and upload/privacy-manifest validation remain unperformed.
- All first-build and device doubts above remain until checked, particularly MoltenVK resizing, EDR/HDR/Dolby Vision fidelity, AudioUnit channel layouts, ao-reload, paused seek event order and URLSession HLS rewriting with real streams. The first-frame decode-failure heuristic (hasVideo plus vid=no) also needs corrupt-codec/foreground-restore tests; track selection alone does not establish visible output.
- Unknown-duration early EOF can still look like genuine completion; the existing transport heuristic only rejects an early EOF when a known duration proves it. A robust stream-completion signal needs runtime evidence/server integration.
- The font transfer limit allows a small in-flight cancellation overshoot; it bounds accepted/stored data and avoids in-memory font buffering.
- A capability probe tells codec availability, not measured maximum throughput. The remaining device-class limits need measurement; combined H.264 entries currently constrain 8-bit as well as software 10-bit.
- The two-engine capability/session/subtitle/settings/i18n package integrations listed above remain outside this lane. A compiler-clean build without these integrations does not establish the full Automatic planning rule.
- Current-entry recovery cannot yet preserve every per-session quality/version/audio option across a Player-setting re-plan; the canonical dedicated re-plan API is still needed.
- No Apple TV HDR output, mpv PiP/AirPlay video, live channels, music sessions or bitstream passthrough were added; these remain the declared engine limitations.
