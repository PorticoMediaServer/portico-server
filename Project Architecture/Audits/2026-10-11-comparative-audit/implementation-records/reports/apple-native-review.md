# Apple native review

## Outcome
Completed the source review and fixes in `native-apple-native-review`, based on HEAD `1696b6f5767da94740e0ac279af5497c13c82245` and the Apple changes since `a3d8e7c`. The working tree contains the fixes; cumulative snapshots preserve each milestone. No commits, pushes, deployments, Xcode/Gradle builds or full test suites were attempted.

The mpv promise callback and existing Swift regression literals are corrected. AVPlayer rejects malformed numeric commands and invalid network sources, and its item callbacks stay on the main queue. PIN/Keychain work no longer queues ahead of API submission. Downloads preserve Wi-Fi policy when resuming, validate metadata batches atomically, and require an exact task identity. Both Apple players share caption appearance; ASS keeps its authored style. AAC input advertises its real channel count and paused optional-renderer seeks can settle without sound. Cast channel ownership is explicit. The television shelf has bounded, redirect-free reads, viewer-specific caches, its own HTTP policy and privacy manifest. The extension's framework/product references no longer depend on a particular SDK installation.

**Verified here:** both permitted app TypeScript checks passed; the single native-playback test file passed all 10 tests; both Xcode projects parsed and all 37 iOS / 36 tvOS source/resource references resolve without phase duplicates; Apple plists, privacy manifests, entitlements, schemes and workspace XML parse; final whitespace check passed. **Not verified:** Swift/Objective-C compilation, Apple SDK linking, native tests or any runtime/device behavior. The standalone Swift grammar printed no errors for 14 files, but its WebAssembly parser crashed at process teardown; that is supplementary reading evidence, not a clean native check.

## Decisions I made
- Kept React Native tvOS 0.83.10-1, Google Cast 4.8.4, the LGPL MPVKit product at exact 1.1.0-n9.0.2 (libmpv 0.41.0), Swift 5 mode, and the 15.1 deployment floors. No package-copy edits or dependency upgrades.
- Resume data cannot have its original request rewritten through URLSession's public API. Reuse it only when the recorded cellular policy matches; a policy change may restart a partial file. Unknown policy and unknown task identity are refused rather than supported as old manifests.
- Local profile operations accept the owner's HTTP route like other local-server operations. Hosted account-auth origin validation, certificate verification, redirect refusal and all transport/body/concurrency bounds remain in place.
- Used one shared Objective-C caption helper, already part of both application targets, with explicit Swift names. Plain mpv text follows system colour/background/typeface overrides; authored ASS does not inherit the plain-caption scale.
- Shelf cache directories are keyed by a SHA-256 of server origin and viewer scope. A retired response cannot refill another viewer's directory even if configuration changes between a check and a write. Files are cache data, not credentials.
- The existing app-group Keychain access group is valid: Apple's documentation says registered App Groups also act as Keychain groups. Did not invent a different prefixed group. Signed entitlement verification remains a device task.
- Did not introduce first-install Keychain deletion or a scene-life-cycle redesign without the protected-data/SDK evidence those changes need. These exact gaps are recorded below.
- Context sync could not write its read-only lock path. Read the existing canonical Portico, project standards and verification guidance; execution host is personal Ubuntu mini-1.

## Bugs
The findings below are source defects, not runtime reproductions. File paths are relative to `apps/native`; the discovery record at the end of this section preserves milestone detail. Final references for moved code:

| File and line | Defect and change |
|---|---|
| `Native/PorticoMpvPlaybackModule.swift:92` | Asynchronous release captured a nonescaping resolver. Add `@escaping`. |
| `test/PorticoMpvTests.swift:51` | Ordinary quoted strings contained literal newlines. Use escaped newlines; add native rejection/atomicity regression. |
| `Native/PorticoPlaybackController.m:28,230,240,256` | Booleans were accepted as numbers; malformed/nonfinite route fields could pass or crash later. Use finite non-boolean integer/number checks before mutation. |
| `Native/PorticoPlaybackController.m:34,377` | HTTP media without a host or with credentials was accepted. Apply the engine contract's source rule and preserve the symlink-resolved Downloads sandbox. |
| `Native/PorticoPlaybackController.m:487,605,606,621` | KVO and item terminal/stall callbacks lacked main-queue fences. Marshal before state access, then check current item. |
| `Native/PorticoPlayer.m:283,324` | Keychain/PIN work shared the API submission queue. Dispatch all 13 storage entry points to one separate serial queue. |
| `Native/PorticoPlayer.m:390` | Local profile operations refused non-loopback HTTP. Accept the selected HTTP origin; preserve redirect/security bounds and require nonempty request IDs/bounded URLs. |
| `Native/PorticoDownloads.m:69` | Bridge methods and download delegate state used different queues. Use the download serial method queue. |
| `Native/PorticoDownloads.m:454` | Resume data retained a permissive cellular request after Wi-Fi-only was enabled. Require matching persisted request policy. |
| `Native/PorticoDownloads.m:539` | Invalid later metadata in a batch left earlier entries changed. Validate the complete batch before mutation. |
| `Native/PorticoDownloads.m:256,273,284,369,395` | Missing task descriptions could reach dictionary subscripting; hash-buffer allocation was unchecked. Refuse unsafe callbacks and fail hashing on allocation failure. |
| `Native/PorticoDownloadAttempt.h:8`, `Native/PorticoDownloads.m:175` | Old-manifest fallback accepted callbacks with no current task identity. Remove it; reconcile cancels unowned tasks. Extend the Foundation guard regression. |
| `Native/PorticoAudioRender.m:271,314` | AAC input claimed zero channels while returning compressed bytes. Advertise the converter input channel count. |
| `Native/PorticoAudioRender.m:442` | Malformed gain objects reached nested dictionary indexing. Refuse the optional plan instead of raising. |
| `Native/PorticoAudioRender.m:562` | Paused decoded seeks waited for Resume before acknowledgement. Acknowledge once the decoded head is scheduled, without starting sound. |
| `Native/PorticoSubtitleSelection.h:6`, `.m:33,80`; `Native/PorticoMpvPlayerView.swift:129`; `Native/PorticoMpvController.swift:1085,1190` | mpv ignored system caption colours/background/typeface and refreshed only size live. Share appearance with AVPlayer; reapply all options on changes. |
| `Native/PorticoMpvController.swift:428` | mpv's default ASS override mode scaled authored ASS. Set `sub-ass-override=no`; pinned source confirms converted plain text still gets its scale. |
| `Native/PorticoCast.m:46,60` | Resume could register another channel without retiring the first; registration failure was ignored. Track session ownership, reuse/detach channels, reject stale terminal callbacks and clean up on main. |
| `Native/PorticoMpvCore.swift:381` | Warning-tail diagnostics could include media-grant URLs. Redact HTTP(S) URLs and bare grant paths before retaining the bounded tail. |
| `tvos/TopShelf/ContentProvider.swift:10,126,190` | Credentialed reads followed redirects and buffered before checking size; poster validation checked only host. Bound reads during receipt, refuse redirects and enforce full same-origin URL constraints. |
| `tvos/TopShelf/ContentProvider.swift:93`; `Native/PorticoTopShelf.m:40` | Old responses could repopulate caches after viewer/server change. Scope cache folders and responses to captured identity; clear the new cache root on changes. |
| `tvos/TopShelf/Info.plist:23` | Extension lacked the app's HTTP exception. Add its explicit ATS policy. |
| `tvos/Portico.xcodeproj/project.pbxproj:92,94` | Framework reference hard-coded AppleTVOS26.0; extension product omitted `.appex`/type. Use SDKROOT and the proper extension product reference. |
| Both application `PrivacyInfo.xcprivacy` files; new `tvos/TopShelf/PrivacyInfo.xcprivacy` | Missing disk-space/shared-defaults reasons and an inappropriate SDK-only reason. Declare actual uses; add extension manifest to its Resources phase. |

Audit IDs reviewed from documents 19, 20 and 14, BUG-REGISTER and the relevant FIX-LOG entries. “Already fixed” for other areas means the preceding lane's supplied source/report boundary, not independent server/device acceptance.

- B-19.1 — Already fixed on supplied base by native-app pending-link ownership; native URL allowlist preserved.
- B-19.2 — Already fixed by native-app stable viewer scope; outside native source.
- B-19.3 — Not fixed here — Android input; outside this lane.
- B-19.4 — Not present on Apple — Keychain has no separate app sealing key; Android recovery belongs to its lane.
- B-19.5 — Not fixed here — JavaScript artwork retry; outside this lane.
- B-19.6 — Not fixed here — global JavaScript error journaling; outside this lane.
- B-19.7 — Already fixed — all four iPad orientations are present; upload validation pending.
- B-19.8 — Not fixed here — Android density; outside this lane.
- B-19.9 — Fixed here — separate serial storage queue; API submission no longer waits for PIN work.
- B-19.10 — Not fixed here — device form-factor/UI override; outside this lane.
- B-19.11 — Not fixed here — Vega storage, deliberate programme trade-off.
- B-19.12 — Not fixed — reinstall can retain Keychain credentials/quarantine-sensitive state; needs a protected-data-aware first-install reset design/device proof.
- B-19.13 — Already fixed for row height by native-app; very-large-offset precision still needs device testing.
- B-19.14 — Not fixed here — mounted React screen lifetime/focus; outside native source.
- B-19.15 — Not fixed end to end — repaired/extended native test sources; no native test target/execution here.
- B-19.16 — Not fixed here — shared string catalogue/JavaScript literals are assigned elsewhere; no new viewer flow added.
- B-19.17 — Not fixed here — Hermes plural support needs actual runtime; package copies are read-only.
- B-19.18 — Not fixed — classic app delegate remains; scene enforcement must be tested with the chosen Xcode SDK.
- B-19.19 — Not fixed here — unused search keyboard/JS PiP capability leftovers; current engine feature names match their consumers.

- B-20.1 — Not fixed here — Android/Vega passthrough and server planning; Apple mpv declares PCM-only.
- B-20.2 — Already fixed at the supplied version-2 profile boundary; native and mpv HDR output/tone mapping still need devices.
- B-20.3 — Already fixed — cue/original/font addresses accept same-origin HTTP; extension HTTP policy corrected here.
- B-20.4 — Not fixed here — Android lifecycle; outside this lane.
- B-20.5 — Already fixed in supplied source — AVPlayer/mpv hold for display matching; late-window/native matching and HDMI timing need device proof.
- B-20.6 — Already fixed — Apple separates delivery/network/decoder failures; preserve classification and fence item callbacks.
- B-20.7 — Already fixed — AVPlayer foreground/systemPaused and mpv background facts; actual background/PiP/interruption acceptance pending.
- B-20.8 — Not fixed here — Android source codec names; preceding lane reports fixed.
- B-20.9 — Already fixed at supplied profile boundary — hvc1/dvh1 conditions; server and black-picture acceptance outside this lane.
- B-20.10 — Not fixed here — Android channel-count probe; preceding lane reports fixed.
- B-20.11 — Already fixed at supplied profile boundary — native text only, mpv separate rich subtitle features.
- B-20.12 — Fixed here for remaining Apple gap — full shared appearance, live updates, authored ASS preservation.
- B-20.13 — Already fixed — transient cue recheck retains authorization until deadline and retries; preserved.
- B-20.14 — Not fixed here — Android probe/throughput; Apple mpv software limits remain estimates.
- B-20.15 — Already fixed for mpv chapter facts/native system-media consumer; screen/trickplay work is another lane.
- B-20.16 — Already fixed in supplied JavaScript; 10 native-playback tests pass; no screen edits here.
- B-20.17 — Already fixed in supplied JavaScript profile-speed application; native supported-rate contract preserved.
- B-20.18 — Already fixed in supplied bounded Dolby Vision preflight; no native redesign.
- B-20.19 — Not fixed here — Android unseekable-file recovery; Apple paused optional-renderer seek settlement improved here.
- B-20.20 — Not fixed here — Android active-output passthrough; outside this lane.
- B-20.21 — Not fixed here — Android crossfade-to-video engine reuse; preceding lane reports fixed.
- B-20.22 — Already fixed — AVPlayer delayed deactivation and mpv teardown completion; release callback compile form corrected here.
- B-20.23 — Already fixed — observed-item storage/removal; main-queue handling strengthened here.

- B-14.1 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.2 — Already fixed for unchanged file/grant on supplied source; extended here to preserve Wi-Fi policy. Address or policy changes still restart iOS partials.
- B-14.3 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.4 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.5 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.6 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.7 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.8 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.9 — Already fixed in supplied downloads handshake/batching; corrected native batch atomicity here.
- B-14.10 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.11 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.12 — Already fixed for the main native player selecting mpv for unsupported originals; signed-out system-player fallback still needs the JS lane/device review.
- B-14.13 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.14 — Partly already fixed by download/native lanes; removal screens/settings ownership are JavaScript scope.
- B-14.15 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.16 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.17 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.18 — Already fixed in supplied source — incoming files survive for verification at launch. Large-file background completion remains untested.
- B-14.19 — Already fixed — launch reconciliation exists; strengthened exact-task fencing here.
- B-14.20 — Not fixed here — Android transfer lifetime.
- B-14.21 — Not fixed here — receipt/progress clock policy is shared JavaScript/server scope.
- B-14.22 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.23 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.24 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.25 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.26 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.
- B-14.27 — Not fixed in this lane — server or JavaScript download behavior; see downloads/native-sync reports for supplied implementation and remaining gaps.

- B-10.10 — Already implemented by audio-engines; corrected AAC input channel metadata here. HE-AAC decoder clock/trim remains unqualified.
- B-10.11 — Not fixed here — server AAC converted target dependency; no server writes in this lane.
- B-10.23 — Already implemented raw AAC trim by audio-engines; frame-perfect priming/deep seeks require Apple conformance.
- B-10.27 — Already implemented gain/peak policy; malformed gain envelopes now fail safely.
- B-10.28 — Not fixed end to end — real HE-AAC and physical codec/output matrix still required.
- B-10.34 — Already implemented first optional-renderer failure fallback; paused seek completion corrected here.

### First milestone findings
- `Native/PorticoMpvPlaybackModule.swift:92`: release promise captured by an escaping asynchronous teardown completion; added `@escaping` to its resolver parameter. The pinned React Native header defines it as a block typedef.
- `Native/PorticoPlaybackController.m:28,230,377`: native accepted JSON booleans as numeric revisions/rate/seek, malformed route numbers, and network sources lacking a host or carrying credentials. Added finite, non-boolean numeric checks and the same network-source constraints as mpv. Retained existing error messages and file sandbox.
- `Native/PorticoPlaybackController.m:487`: status KVO processing had no main-queue fence. Marshal off-main observations before inspecting/mutating controller state and retain the current-item check.
- `Native/PorticoDownloads.m:70,435,541`: downloads bridge methods ran on React Native's separate method queue while delegate updates used `_queue`; batch metadata mutated earlier entries before discovering an invalid later value; resumed requests retained the old cellular permission. Use the downloads serial method queue, validate complete metadata batches before mutation, and reuse resume data only under the same recorded network policy. A policy change may restart a partial transfer because URLSession resume requests cannot be rewritten.
- `Native/PorticoDownloads.m:255,283,289,385,411`: ignore background callbacks without safe task descriptions before dictionary subscripting; refuse hashing if buffer allocation fails.

### Second milestone findings
- `test/PorticoMpvTests.swift:51–66`: ordinary quoted strings contained literal newlines, so the predecessor XCTest file could not compile. Replaced them with escaped newlines; all Swift files parse with the standalone grammar (not an SDK type-check).
- `Native/PorticoPlayer.m:334,387–540`: B-19.9 still present: Keychain and 200,000-round PIN work blocked the runtime's API submission method queue. Dispatch all 13 durable storage methods onto one separate serial storage queue shared across bridge instances. Preserve Keychain readback, quarantine, PIN charging, crypto and existing class locking.
- `Native/PorticoPlayer.m:459`: local profile management and selection still refused HTTP except loopback even though sign-in and the rest of the API accepted the owner's HTTP route. Allow HTTP on any valid host under the programme decision, retain no redirects, no credentials/fragments, body/concurrency bounds, and add a URL-length and nonempty request-id bound. Hosted's separate account-auth origin validator remains TLS-only.

### Third milestone findings
- `Native/PorticoAudioRender.m:275,324`: the new AAC input callback advertised zero interleaved channels while returning compressed audio bytes. Preserve the converter input description's channel count in the buffer list. Core Audio packet/priming fidelity still requires the conformance run.
- `Native/PorticoAudioRender.m:442`: malformed `gain` (array/string/null) reached nested dictionary subscripting during plan validation and could raise instead of refusing the optional render plan. Require a dictionary before looking inside.
- `tvos/TopShelf/Info.plist`: the extension did not inherit the app's ATS exception; HTTP feeds/posters were still blocked. Add the same explicit arbitrary-load policy to the extension.
- `tvos/TopShelf/ContentProvider.swift`: feed and poster requests followed redirects with the feed bearer, buffered entire bodies before checking size, and checked only poster host (not scheme/port/credentials). Use bounded delegate readers (1 MiB feed, 8 MiB poster, 20-second resource lifetime), refuse redirects, and enforce complete same-origin poster URLs. Preserve existing image shrinking and four-poster concurrency.
- `tvos/TopShelf/ContentProvider.swift` and `Native/PorticoTopShelf.m`: an old feed or artwork response could repopulate caches after a profile/server change. Capture server/profile identity, reject obsolete responses, and keep feed/art in hashed identity-specific cache folders; clear the new cache root on viewer changes. Old in-flight writes can no longer populate the next viewer's folder.
- `test/PorticoMpvTests.swift`: added a native-controller regression that proves malformed numeric/route/seek commands do not replace a previously accepted command. Apple unit target execution is pending.

### Fourth milestone findings
- `Native/PorticoMpvController.swift:1086` and `Native/PorticoMpvPlayerView.swift:127`: mpv used the system's caption size but ignored the viewer's foreground colour, opacity, background and typeface overrides, despite the current engine contract and the previous lane's completion claim. Extract the existing AVPlayer appearance code into `PorticoSubtitleSelection.{h,m}` and use it from both overlay views; derive mpv plain-text options from the same result. Explicit `NS_SWIFT_NAME` declarations keep the new Swift/Objective-C method names stable. Styled ASS keeps its normal mpv style handling; no forced ASS override was added.

### Fifth milestone findings
- `Native/PorticoMpvController.swift:399`: the pinned mpv default for `sub-ass-override` is `scale`, so the caption scale changed styled ASS placement/size despite the contract saying ASS keeps its own look. Set `sub-ass-override=no`; the pinned `sd_ass.c` still applies plain-text scale because that path is converted text.
- `Native/PorticoDownloadAttempt.h:9` and `Native/PorticoDownloads.m:182`: the old-manifest compatibility fallback accepted callbacks without a current task identity. Remove it under the pre-release rule; reconcile cancels unowned/restored obsolete tasks and retries incomplete entries. Extract the network-policy resume guard beside the existing attempt guard and extend the standalone Foundation regression to cover tightening, unchanged policy, missing policy and malformed policy.

### Sixth milestone findings
- `tvos/Portico.xcodeproj/project.pbxproj:92`: Foundation referenced an absolute Xcode SDK-relative AppleTVOS26.0 path, so another installed SDK could not resolve it. Use `SDKROOT/System/Library/Frameworks/Foundation.framework`.
- `tvos/Portico.xcodeproj/project.pbxproj:94`: the extension product reference named `TopShelf` without its `.appex` suffix/type, risking a missing product at the Embed App Extensions phase. Name the built product `TopShelf.appex` and declare `wrapper.app-extension`.
- `ios/Portico/PrivacyInfo.xcprivacy`, `tvos/Portico/PrivacyInfo.xcprivacy`: own UserDefaults use incorrectly declared the SDK-only wrapper reason C56D.1 (iOS), while shared App Group access was missing on tvOS. Remove C56D.1, add shared-group reason 1C8F.1 on tvOS. Both apps use free-space APIs for bounded audio caching; iOS also displays download space. Declare E174.1 in both and 85F4.1 on iOS, checked against Apple's actual reason definitions.
- `tvos/TopShelf/PrivacyInfo.xcprivacy` (new) and the extension Resources phase: add its own shared-defaults reason 1C8F.1; an app's manifest does not describe an extension's bundle by itself. The new resource is explicitly in the TopShelf target.

### Seventh milestone findings
- `Native/PorticoCast.m:61`: each session resume installed a new generic channel without removing the one retained by the Cast session, and ignored `addChannel` failure. Track the owning session, reuse the current session's channel, detach it on replacement/end/invalidation, check registration success, and ignore an old session's terminal event when another session owns the channel. Marshal invalidation cleanup to the main queue as the pinned Google Cast SDK requires.
- `Native/PorticoMpvController.swift:1200`: a live Accessibility change refreshed only size for mpv-rendered text. Reapply all shared caption options on the system appearance notification.

### Eighth milestone findings
- `Native/PorticoAudioRender.m:586`: a paused optional-renderer seek never settled because `startPrimary` returned before the seek acknowledgement unless the node was allowed to play. Once its decoded head has been scheduled, acknowledge the seek without starting sound. Keep the normal play/start readiness path and buffer threshold. Device acceptance must verify paused audiobook scrubbing stays silent and clears the owner's pending seek.


### Final findings after milestone 8
- `Native/PorticoMpvCore.swift:381`: redact media-grant diagnostics before retaining a warning tail.
- `Native/PorticoPlaybackController.m:605,606,621`: main-queue fences for stall, failure-to-end and end notifications.

## API changes
No server routes, contracts or JavaScript module/method/event/argument shapes changed. `PorticoRuntime.profileRequest` accepts ordinary HTTP origins as directed. Internal Objective-C caption helper methods have explicit Swift names; they are not exported to JavaScript. Existing engine validation/rejection text is retained.

## Schema changes
No server schema changes. Local download manifest adds `cellularAllowed` (boolean) per transfer to qualify resume data. Shelf caches now live under `Library/Caches/topshelf/<SHA-256 of origin and viewer>/`; no old cache compatibility path. Unknown task/policy data is not trusted.

## Owner or viewer settings added or wired
No new settings or screens. Existing Wi-Fi-only download preference is now applied to resumed requests as well as fresh ones. Existing profile caption size/background and system Accessibility caption overrides now take effect consistently in both Apple text rendering paths. HTTP remains the owner's route choice.

## Needed from others
- Integrate the final cumulative snapshot into the programme's native checkout; do not replay cumulative patches as independent additive diffs.
- Perform the Mac build and signed device checks below. Add a native XCTest target to run the existing Swift file; there is still no target on this base.
- Native JavaScript lane: inspect signed-out offline playback through `PorticoDownloads.play`, which uses AVPlayerViewController rather than the two-engine coordinator. It may still fail unsupported MKV originals and does not draw bundle extras itself. This lane did not change screens/player ownership.
- Correct `ENGINE-CONTRACT.md` section 7's obsolete cue-document address example to `/v1/media/subtitles/<grant>/<resource>/<revision>`; code and current consumers already use that shape. Documentation is outside this lane's source/bridge scope.
- Update earlier native-playback completion claims: mpv's full system appearance was incomplete until this patch. No server/schema/shared-package work is included here.

## Test notes for other environments

### Checks run here
- `cd apps/native && npx --no-install tsc --noEmit` — exit 0.
- `cd apps/vega && npx --no-install tsc --noEmit` — exit 0.
- `cd apps/native && node --import ./test/register.mjs --test ./test/native-playback.test.ts` — 10/10 passed. Exercises the shared session/generation/engine/quality path against the existing fake server, not native playback.
- Parsed both `.pbxproj` files with the existing xcode parser; resolved parent-group paths for every source/resource phase entry. 37 iOS and 36 tvOS entries exist; no duplicates within a phase. New extension manifest is in TopShelf Resources. Header-only caption changes reuse an already-compiled file in both targets.
- Python plist/XML parsing of all Apple plists, entitlements, privacy manifests, shared schemes and workspaces — passed. Final `git diff --check` — passed.
- Supplementary tree-sitter Swift parsing printed no errors across 14 source/test files; the parser then hit a V8 teardown OOM. Objective-C grammar cannot interpret React macros/nullability/preprocessor branches reliably. Neither tool establishes type correctness.
- No Swift compiler, clang binary with Apple SDK, Xcode or simulator is available here. No native language test ran. The earlier Kotlin standalone toolchain provides no Apple substitute.

### API evidence fetched/inspected
Downloads are under `_program/scratch/apple-native-review/`, outside the worktree.
- [Pinned MPVKit manifest](https://github.com/mpvkit/MPVKit/blob/1.1.0-n9.0.2/Package.swift), freshly fetched; exact package minimum iOS/tvOS 15 and Swift tools 5.9. Inspected real release module/header artifacts preserved by the earlier review; module is Libmpv, product is MPVKit. Fresh [mpv 0.41 client header](https://github.com/mpv-player/mpv/blob/v0.41.0/include/mpv/client.h) confirms command pointer/event/nullability ownership. Pinned option documentation and `sd_ass.c` confirm ASS override behavior; commands/options/property names remain valid.
- [Actual Cast 4.8.4 SDK archive](https://dl.google.com/dl/chromecast/sdk/ios/GoogleCastSDK-ios-4.8.4_static.zip), fetched from its exact CocoaPods spec. Extracted Cast options/context/UI category/session/session-manager/generic-channel headers. `presentCastDialog` belongs to `GCKCastContext+UI.h`, included by the umbrella; session listener selectors match; channel add/remove return BOOL; options are present. No blind current-version substitution.
- Installed React Native tvOS 0.83.10-1 headers define the promise block typedef and methodQueue/requiresMainQueueSetup API; Pod helper declares minimum Xcode **16.1**, deployment **15.1**.
- Apple's [Network listener property](https://developer.apple.com/documentation/network/nwparameters/acceptlocalonly), [AudioConverter callback](https://developer.apple.com/documentation/audiotoolbox/audioconverterfillcomplexbuffer(_:_:_:_:_:_:)), [wrapped-file lifetime](https://developer.apple.com/documentation/audiotoolbox/extaudiofilewrapaudiofileid(_:_:_:)), [required-reason definitions](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype), and [App Groups Keychain behavior](https://developer.apple.com/documentation/BundleResources/Entitlements/com.apple.security.application-groups). Documentation data downloaded where useful; inspected earlier SDK AVDisplayCriteria/UIScreen headers. Network property is available below the project floor; AVDisplayCriteria initializer is correctly tvOS-17-gated; EDR is iOS-16-gated; AV1 is 17-gated. ExtAudioFile wrapping does not own/close the underlying AudioFileID, so existing explicit teardown is retained.

### First build, from a Mac checkout with this integrated patch
Use Xcode 16.1 or newer with iOS/tvOS SDKs and installed simulator runtimes, Node/npm and CocoaPods. Retain the lockfiles and exact package revision. Use separate derived-data folders per platform. No build below was run here.

```sh
cd apps/native
npm ci --legacy-peer-deps
cd ios
pod install
xcodebuild -resolvePackageDependencies -workspace Portico.xcworkspace -scheme Portico
xcodebuild -workspace Portico.xcworkspace -scheme Portico -configuration Debug -destination 'generic/platform=iOS Simulator' -derivedDataPath /tmp/portico-review-ios CODE_SIGNING_ALLOWED=NO build
```

From `apps/native`:

```sh
cd tvos
pod install
xcodebuild -resolvePackageDependencies -workspace Portico.xcworkspace -scheme Portico
xcodebuild -workspace Portico.xcworkspace -scheme Portico -configuration Debug -destination 'generic/platform=tvOS Simulator' -derivedDataPath /tmp/portico-review-tvos CODE_SIGNING_ALLOWED=NO build
```

Then repeat each workspace with configuration **Release** and destinations `generic/platform=iOS` / `generic/platform=tvOS`, different derived-data directories and `CODE_SIGNING_ALLOWED=NO`. Release exercises conditional code and bundling; Debug exercises native harnesses. Confirm `Portico.app/PlugIns/TopShelf.appex` contains `PrivacyInfo.xcprivacy`. For signed device builds set Justin's team/profiles, the existing App Group and Sign in with Apple capabilities; register the matching Cast receiver id where testing Cast.

The compiler/linker error checklist:

| Error/symptom | First action |
|---|---|
| `No such module React` / failed bridging header | Run pod install for that platform, build the workspace, preserve inherited CocoaPods header/search settings. The app bridging header imports clock, subtitle and playback controller declarations. |
| `No such module Libmpv` or mpv exported `available=false` | Resolve exact MPVKit package and ensure full MPVKit product remains in app Frameworks/package dependencies. A successful native-only build is insufficient: canImport can exclude the implementation. |
| Unresolved FFmpeg/libass/Vulkan/MoltenVK symbols | Link full MPVKit and its transitive products, not Libmpv alone; inspect selected simulator/device slices. Do not bypass package checksums or change to GPL product. |
| Swift helper import/name/type error | Inspect the Clang import of the two explicit `NS_SWIFT_NAME` methods and dictionary generic; `applyCaptionAppearance(to:baseSize:style:italic:)` takes UILabel/CGFloat/optional dictionary/Bool, `mpvCaptionOptions(style:)` returns `[String:String]`. Keep callback nullability; do not force-cast unknown cue values. |
| Swift pointer error on mpv command calls | Compare the selected actual header: array is mutable outer storage of nullable const C string pointers, including null terminator. Current implementation uses that shape. |
| Swift actor/Sendable errors | Verify SWIFT_VERSION remains 5.0. A Swift 6 migration needs explicit ownership/actor design; do not add unchecked sendability broadly. |
| Foundation or extension embedding missing path | Confirm SDKROOT Foundation reference and `TopShelf.appex` product reference survived integration; rebuild the tvOS workspace. |
| Google Cast property/selector error | Compare the extracted 4.8.4 headers, including its UI category; reinstall the locked 4.8.4 pod. The APIs used here exist in those headers. |
| CocoaPods deployment/SDK diagnostics | Keep minimum 15.1 on app/pods and SDK APIs guarded as above. Do not bump the floor to silence an individual availability error without reading its actual header. |
| New SDK scene launch warning/failure | Build and launch both app targets with the chosen SDK; classic lifecycle remains and B-19.18 is unresolved. If adoption is enforced, migrate both AppDelegates with launch-link/background-session handling tested. |
| Privacy-manifest validation | Inspect actual app and extension bundle manifests and package/pod manifests. New own-code reasons are tied to source uses; archive validation still must run. |
| Native Swift test can't see ObjC controller | Unit target needs the existing bridging header/header search paths, test host Portico and Debug testability. Match the explicit helper names to the imported interfaces; test source has not been compiler checked. |

Add an iOS and tvOS Unit Testing Bundle `PorticoNativeTests`, host application Portico, to each shared scheme's Test action. Add `test/PorticoMpvTests.swift`; use the app bridging header, inherited CocoaPods search paths and MPVKit product access. Run on an installed simulator:

```sh
xcodebuild -workspace Portico.xcworkspace -scheme Portico -configuration Debug -destination 'platform=iOS Simulator,name=<installed simulator>' -only-testing:PorticoNativeTests/PorticoMpvTests test
```

Use analogous tvOS destination/workspace. Expect **8** test methods when Libmpv is imported (7 otherwise); absence of the conditional engine test is not full coverage. Standalone Foundation guard regression on Mac, from `apps/native`:

```sh
xcrun --sdk macosx clang -fobjc-arc -framework Foundation test/native-download-attempt-tests.m -o /tmp/portico-download-attempt-tests
/tmp/portico-download-attempt-tests
```

Pass: reports attempt fencing and resume-policy assertions without exceptions. Fail: old attempt accepted, old cellular policy trusted, or malformed/absent policy treated as known.

### Device acceptance with explicit pass/fail
1. **Native/mpv engines:** Automatic uses native for a directly playable MP4 and mpv for an otherwise-converted unsupported file. Alternate engines rapidly while playing/paused and closing during load. Pass: one engine/sound, same queue and retained position, lease release resolves after teardown. Fail: unavailable module, audio lost after handoff, stale picture/fact or crash. Test iPhone, iPad, Apple TV HD and 4K.
2. **Command rejection:** run the native XCTest regression. Booleans/NaN/infinite/fractional route/revision/seek fields must be rejected without replacing the accepted command. Invalid network host/credentials must produce the existing media-address error without network access. Symlink/file escapes remain refused.
3. **Storage responsiveness:** unlock a PIN repeatedly while opening a page and submitting playback controls. Pass: request submission proceeds while PIN derivation is busy, storage ordering/readback/attempt charging persists. Also test locked device before first unlock: unreadable records remain unreadable, never silently reset.
4. **Downloads:** begin under mobile-data permission, switch Wi-Fi-only on mid-transfer, resume while still cellular. Pass: transfer does not move until allowed connectivity; unchanged policy+grant resumes; changed policy may restart at zero. Pause/remove/restart and inject old callback delivery; no late callback can change a new attempt or completed file. Submit a batch with one valid and one invalid metadata value; rejected batch changes neither. Force-kill during a 20+GB hash, relaunch: incoming file is verified/reconciled rather than lost. These are physical background-session tests.
5. **Captions:** change profile size/background and Accessibility foreground/font/background during native cue, mpv cue and mpv plain SRT playback. Pass: both overlays/plain paths update, including italic/top cues and no-background edge; authored ASS positioning/font/size stays authored. Test external VobSub and its companion, ASS fonts, PGS/DVB; rapidly switch/off while files load. Test downloaded WebVTT offline.
6. **Audio:** run `PORTICO_AUDIO_CONFORMANCE=<fixture folder>` and `PORTICO_AUDIO_CONFORMANCE_OUT=<temporary JSON>` in a Debug app. Declared fixtures must have exact frame count, lag zero and agreed tolerance. Check LC-AAC, HE-AAC/SBR, iTunSMPB/edit lists, converted AAC, ALAC/FLAC/MP3. Seek paused audiobook while effects enabled: pending seek clears without sound. Seek before/inside packet/deep/near EOF; reconnect/audio-route reset and crossfade. Fail: channel rejection, clipped syllable, extra priming/tail, stuck paused seek, drift or wrong frame count. Remaining HE-AAC fixtures/converted recipe dependency are in audio-engines report.
7. **Display/lifecycle:** tvOS Match Content on, 23.976fps title from 60Hz UI; delay mounting the picture, test HDR/native and SDR/mpv. Pass: opening seconds retained, switch settles/bounds out and prior mode restored. Background without PiP pauses and sends intent fact; PiP/AirPlay retain their declared behavior. Test interrupts/headphones/media-services reset, not merely simulator focus.
8. **Shelf:** HTTP and HTTPS server, trusted/untrusted TLS, chunked oversized feed/poster, redirects, same host on different port/scheme, profile/server switch while requests run. Pass: no redirect bearer forwarding, at most bounded accepted body, invalid references refused, cached shelf works offline and old responses cannot show the next viewer's history/art. Inspect signed app-group Keychain and extension manifest/ATS. Unauthorized feed clears its scoped cache.
9. **Cast:** configured receiver, start → background/resume → repeated reconnect → receiver switch → bridge reload. Pass: one registered channel per session, send/replies continue, retired session cannot clear current channel. No configured receiver: feature remains unavailable. Requires real receiver and Local Network permission.
10. **TLS/relay:** HTTPS and HTTP file/HLS variants, absolute/relative media/audio/subtitle/key/map URLs, rejects out-of-grant references, >2MiB manifests, >16KiB heads, idle and >16 connections. Check retained-route seek within 0.5s/ten-second bound and throughput/cache facts; confirm redacted error diagnostics contain no grant URL. Unknown-duration early EOF remains a doubt below.

## Not finished
- **No Apple compile/link/native test/device run.** This is completed source work with a concrete first-build checklist, not a claim of a clean first build or correct physical playback. XCTest target wiring remains a Mac task.
- **B-19.12:** app deletion/reinstall can retain Keychain sessions. No install-time deletion was added; protected-data-aware reset/readback and a device reinstall test remain necessary.
- **B-19.18:** no scene manifest/lifecycle migration. The actual chosen SDK's launch/scene enforcement must be established. This could be a first-build/launch blocker on a future enforced SDK.
- **AAC/music accuracy:** converter input clock, HE-AAC raw priming/output rates, deep packet seek warm-up, converted-stream priming and sample-exact joins remain unqualified. A paused seek to exactly EOF/no scheduled head still needs explicit end-boundary testing. Server AAC conversion recipe and real HE-AAC corpus remain other-lane dependencies.
- **mpv/device doubts:** MoltenVK resizing/Metal validation, EDR/HDR/Dolby Vision fidelity, multichannel output and AudioUnit `ao-reload`, seek reply/restart ordering under real load, font fallback, exact live-route retention and software decode limits. Apple TV mpv remains SDR/tone-mapped, without PiP/AirPlay video/live channels/music/passthrough, as declared.
- **Unknown-duration early EOF:** existing mpv heuristic needs a known duration to prove premature completion. No new server completion signal was invented here.
- **AVPlayer late-window display matching:** current ready handler can use the app key window, but a case where no window exists yet is not proven by source checks. Delay-window acceptance remains required.
- **Offline signed-out flow:** the separate system-player path may still fail original MKV and omit bundle extras; native JavaScript/owner routing work is needed, outside this lane.
- **Platform report/manifest acceptance:** no signed archive/upload validation, shared App Group provisioning test, physical Cast test or background-download force-kill test. The user-facing HTTP choice is implemented; TLS identity is still verified by system APIs.
- Other JavaScript/server/Android/Vega bug IDs above are outside this lane and remain with their named owners. No package copies or unrelated client screens were edited.

Final cumulative snapshot: `_program/snapshots/apple-native-review/009`, subject “Finish Apple player and extension source fixes”, against HEAD `1696b6f5767da94740e0ac279af5497c13c82245`. Tracked patch: 91,941 bytes; one new file (the extension privacy manifest) is in its untracked archive. Verified the current tracked diff and untracked list match this snapshot. No worktree changes were made afterward. Snapshots 001–008 are milestones, not additive patches; integrate the final cumulative result once.
