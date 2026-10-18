# Android native review

## Outcome

Completed the source review of the Android changes since `a3d8e7c`: main sources, both Google and Fire TV flavours, the libmpv Java/JNI bridge, Gradle/CMake wiring, manifests and resources. Changes stay in this lane's worktree. No packages were edited, no subagents or helpers were used, and nothing was committed, pushed or deployed.

Fixed download cancellation/replacement races and integrity bounds, stale bridge/event/relay cleanup, native playback validation, television startup ordering, caption updates, media-session artwork and actual audio teardown at engine handoff. The standalone Kotlin compiler found no compilation errors in the supplied base. Final standalone Kotlin checks passed for Google and Fire TV with Media3 1.10.1 selected explicitly. No Kotlin compilation repair was needed; the fixes address behavior, ownership and build-input validation.

The Java bridge compiles. JNI compiles **and links against the actual pinned libmpv** for all four ABIs. All four download checksums match, all four binaries have 16 KiB LOAD alignment, and none depends on the archive's helper library or on libc++_shared. This establishes source/link checks, not APK packaging or playback.

Milestones are cumulative snapshots `001` (replacement/lifetime fixes), `002` (bounds, captions, relay and build checks) and `003` (audio teardown). The final cumulative snapshot is recorded at the end of this report. Git metadata is read-only, as expected.

## Decisions I made

- Use the cached Kotlin 2.1.20 compiler, Android 36 headers, actual pinned Media3 1.10.1 AAR classes and existing React Native/dependency/generated declarations. No Gradle execution. Cached generated `BuildConfig`/package/resource declarations prove signatures, not current resource generation or flavour packaging.
- Release now closes the actual player. mpv's synchronous `stop` command only sets `PT_STOP`; its completion does not establish audio teardown. Terminate mpv on its core queue before resolving release. Media3 views detach first, then `ExoPlayer.release()` runs on its required application thread. Remove the predecessor's idle mpv handle reuse. Device timing, including Media3's bounded release timeout, must still be qualified.
- Serialize file workers per download without occupying worker threads waiting for predecessors; unrelated downloads can still run together. Keep the partial file's actual digest beside its bytes, independent of the manifest's latest desired request. A partial without a valid identity marker is discarded safely once; ready downloads are retained. No compatibility path for unverifiable partials was added.
- Require the supplied artifact's valid SHA-256 and positive whole byte count. An HTTP response may never raise that count. Both count and digest must agree at completion.
- Removal renames a download folder aside immediately and deletes that detached folder after its old worker closes. A new request with the same identifier gets its own folder. Interrupted detached-folder deletion is retried at launch.
- Quiet caption changes apply to a paused picture too. Plain mpv text follows system foreground/background overrides; ASS keeps its supplied styling. The overlay honours system typefaces; arbitrary system typeface selection remains unavailable in this older mpv font backend.
- Keep dependency versions and archive pins. Unsupported ABI lists fail explicitly; local overrides are checked on every task run. Do not add duplicate `jniLibs` packaging or a helper .so without evidence from a real APK build.
- Execution host: personal Ubuntu mini-1, `ehler-homelab-mini-1`. Context refresh failed because its lock directory is read-only; existing project, standards and verification guidance was read directly. Fetched evidence and temporary harnesses are under `_program/scratch/android-native-review/`.

## Bugs

Android Kotlin paths below are relative to `apps/native/android/app/src/main/java/tv/getportico/portico/native/`, unless otherwise stated. Line numbers identify the final implementation region.

### Confirmed source defects fixed here

| File:line | What was wrong | Change |
|---|---|---|
| `PorticoDownloadCenter.kt:189`, new `PorticoDownloadWork.kt:7` | Cancelled, paused or superseded workers could still write the same partial file as their replacement. | Per-download ordered file jobs; cancelled hashing stops promptly; the next worker waits for actual file closure. |
| `PorticoDownloadWork.kt:30` | The manifest's newest digest did not prove which file the partial bytes belonged to, especially after an intermediate queued attempt was cancelled or the process died. | Atomic, synced `partial.sha256` identity beside `partial.part`; mismatches discard only the partial. |
| `PorticoDownloadCenter.kt:163`, `:444`, `:515` | Missing SHA-256 disabled verification. Response length could replace the expected artifact size and fill storage beyond it. | Validate required digest/count; freeze them per transfer; refuse excess headers/streamed bytes and require exact final size/hash. |
| `PorticoDownloadCenter.kt:260` | Deferred removal could delete a new download reusing the identifier, and recursive deletion could block bookkeeping. | Rename the old directory aside and queue deletion of only that detached directory. |
| `PorticoDownloadCenter.kt:289` | `setMetas` changed earlier rows before rejecting a later invalid value. | Validate the complete batch before mutation. |
| `PorticoDownloadCenter.kt:336` | Extra fetches shared a temporary name; stale replies could overwrite new extras or recursively erase a replacement download. | Unique temporary files, entry/attempt identity at commit, narrow cleanup and a 60-second request deadline. |
| `PorticoDownloads.kt:39`, `:49` | Old module initialization/invalidation could replace or clear the new module's listener. | Invalidated initialization is ignored; cleanup compares listener identity. |
| `PorticoEventStream.kt:126` | An old stream's delayed failure or read cleanup could cancel a replacement with the same identifier. | Cleanup requires the original entry identity, including owner invalidation. |
| `PorticoPlaybackController.kt:275` | Invalid route values could reach `number(route)!!` and crash the command handler. | Nullable validation rejects stale/malformed transitions before any state change. |
| `PorticoPlaybackController.kt:377` | Native HTTP sources did not enforce the contract's host/no-credentials rule. | Share the validated source rule with mpv; retain resolved Downloads confinement. |
| `PorticoPlaybackController.kt:635`, `:827` | Video could run before readiness/matching; READY before the surface marked the item matched without ever voting for its rate. | Video waits for readiness; television surface arrival is awaited before matching, with one ten-second total deadline. |
| `PorticoMpvDevice.kt:166`, native controller acquire | Installing visibility after lifecycle start could leave the wrong visibility state; new native leases assumed foreground. | Reconcile at install, resume and destruction; new leases inherit current visibility. PiP remains started/visible. |
| `PorticoPlaybackModule.kt:72` | A retired module invalidated whichever controllers occupied global shared slots, potentially destroying replacements. | Teardown uses only this module's initialized instances and clears shared slots by identity. |
| `PorticoSystemMedia.kt:117`, `:146`, `:189` | Initial publication cancelled the artwork request just started. Unknown-length responses were buffered before checking the 4 MiB limit. | Preserve artwork through session replacement; enforce the byte limit while reading. |
| `PorticoPlaybackController.kt:1383` | Reclaimed decoders and AudioTrack output failures were classed as media decode failures and could create remembered incompatibility. | Treat resource/output failures as uncertain; reserve decode/parser classes for actual format/decoder evidence. The media client also refuses redirects. |
| `PorticoMpvSupport.kt:303`, `PorticoMpvController.kt:1260`, `PorticoPlayerView.kt:104` | mpv plain-text colours ignored system overrides; paused overlays did not update on caption style changes. | Shared colour selection and live size/style/enabled listeners in both engines. |
| `PorticoMpvController.kt:682` | A terminal EOF before playback restart could leave startup without any terminal fact. | Report an unplayable-file error when a file ends before producing playback. |
| `PorticoPlayerView.kt:53` | A surface supplier retained a detached stage and its activity through the controller. | Clear the supplier by identity when its stage releases the player. |
| `PorticoMpvRelay.kt:83`, `:311` | Old exchanges could publish failures against a new title; a call could be installed after unbind had already cancelled the exchange. | Failure recording/call installation require a live exchange; binding clears old failure evidence atomically. |
| `PorticoMpvRelay.kt:157` | Bridge retirement closed sockets but retained its relay executors. | Terminal destroy shuts down its executors; ordinary unbind remains reusable. |
| `PorticoMpvCore.kt:394`, `PorticoMpvController.kt:1561` | HDR parameters could trail first restart. HDR matching was skipped without FPS or below Android 11. | Deliver current video parameters before restart; HDR televisions are held even without frame-rate voting, within one deadline. |
| `PorticoMpvCore.kt:190`, both controllers release | Dispatching `stop` was mistaken for finishing audio teardown. | mpv termination completes off-main before the release promise; Media3 detaches and releases before returning. Delete the obsolete dispatch-only flush and idle teardown timer. |
| `android/portico-mpv/build.gradle:31`, fetch task | Unsupported ABIs disappeared silently; local override validation could be skipped as up to date. | Reject unsupported ABI lists; always revalidate local overrides. |
| `android/portico-mpv/src/main/java/.../PorticoMpvNative.java:65` | Missing libraries silently hid the engine. | Log the loading failure and its cause under `PorticoMpv`. |

Added `apps/native/test/download-work-tests.kt`: independent work, ordered cancellation/replacement/removal, reentrant enqueue and durable partial identity. A temporary harness also exercises the actual native route validator and stream identity cleanup; it is not shipped as application code.

### Audit cross-reference

Read the B entries in domain documents 19 and 20, their BUG-REGISTER rows, the relevant fix log, and predecessor reports. These statuses distinguish this Android source review from the programme's other lanes. No Apple, server, shared-package or ordinary screen repair is claimed here.

| ID | Disposition |
|---|---|
| B-19.1 | Already fixed in supplied launch-link flow; native allowlist/warm intent delivery preserved. Device cold-launch acceptance remains. |
| B-19.2 | Already fixed in supplied stable viewer scope; JavaScript ownership, untouched. |
| B-19.3 | Already fixed: Android Menu stays its own key; no synthetic Select. Pinned React Native source agrees; remote acceptance remains. |
| B-19.4 | Already fixed: confirmed reset with durable markers, manual reset for ambiguous older Keystore absence, no replacement behind ciphertext. Reviewed; device failure injection remains. |
| B-19.5 | Already fixed in supplied artwork retry flow. Also fixed native media-session artwork cancellation/bounds here. |
| B-19.6 | Already fixed by supplied diagnostic journal/report flow; outside Kotlin changes. |
| B-19.7 | Not fixed here: Apple upload/orientation validation is another platform's scope. |
| B-19.8 | Not fixed: the RN television density/rotation internal override still needs a rotating-display device; no speculative replacement of its design-space contract. |
| B-19.9 | Not present on Android: runtime storage has its own executor. Apple queue ownership is outside this lane. |
| B-19.10 | Not fixed: generic boxes that do not report television mode can still get touch layout; a device-layout override needs the client/settings owner. |
| B-19.11 | Not fixed here: Vega private plaintext storage is an explicit platform limitation, with no Kotlin/JNI/Gradle changes in Vega. |
| B-19.12 | Not present on Android; Apple reinstall/Keychain lifecycle belongs to the Apple review. |
| B-19.13 | Already fixed for caption row height on supplied base; giant-offset float precision remains a JS grid/device qualification issue, not changed here. |
| B-19.14 | Not fixed: mounted screen memory lifetime remains with the JS lane. Covered-screen network work was already reduced. |
| B-19.15 | Improved: added and ran standalone JVM regressions; actual APK/native test infrastructure and React hook harness migration remain. |
| B-19.16 | Not fixed here: remaining catalogue work is owned by the JS/shared lane. Added diagnostics are not viewer-facing text; engine rejection messages retain the contract. |
| B-19.17 | Not fixed: actual Hermes locale/plural availability requires runtime verification; no locale feature claim from Kotlin compilation. |
| B-19.18 | Not fixed here: Apple scenes/SDK launch acceptance is outside Android. |
| B-19.19 | Native leftovers retained: search keyboard remains unused by screens and manifest adjustResize remains; PiP capability is refined by the native view. Wider cleanup belongs to the client lane. |
| B-20.1 | Already fixed in supplied version-2 document/output route and separate passthrough/decode lists. Android/Fire TV HDMI qualification remains. |
| B-20.2 | Not present in this Android source; Apple/planner fix reported by other lanes. |
| B-20.3 | Already fixed: cue readers accept same-origin/grant HTTP on any host; network security configuration permits it. |
| B-20.4 | Fixed further: background starts inherit actual visibility, including installation after lifecycle start. Existing pause/systemPaused preserved. |
| B-20.5 | Fixed further for Android matching: late surface, startup readiness, single deadline and early HDR parameters. Original Apple finding is outside this lane. |
| B-20.6 | Fixed further: resource/audio-output failures no longer blame media; HTTP/network classification preserved; stale relay evidence fenced. Live behind-window orchestration remains below. |
| B-20.7 | Already fixed: systemPaused/system-media facts and JS intent updates; visibility edges improved here. |
| B-20.8 | Already fixed: MPEG-2 is `mpeg2video`, VC-1 is `vc1`; source codec vocabulary preserved. |
| B-20.9 | Not present in Android; Apple HEVC tag qualification belongs to the other review. |
| B-20.10 | Already fixed: unspecified digital output counts imply multichannel, other outputs stereo. |
| B-20.11 | Already fixed: native declares text only, mpv declares its own rich renderer; native text remains disabled in Media3 and drawn from cues. |
| B-20.12 | Fixed further: live system caption colours/style updates in both Android engines. Arbitrary system typeface in mpv-drawn text remains unavailable. |
| B-20.13 | Already fixed: transient rechecks retain held cues only until the existing authorization deadline and retry after five seconds. |
| B-20.14 | Already fixed in supplied hardware flag/device-class limits. Throughput limits remain estimates needing devices. |
| B-20.15 | Already fixed in supplied chapter/trickplay screen flow; native features accurately distinguish embedded chapter reporting. |
| B-20.16 | Already fixed in supplied video queue/remote command flow. |
| B-20.17 | Already fixed in supplied preferred-speed flow. |
| B-20.18 | Already fixed in supplied bounded pre-play flow. |
| B-20.19 | Already fixed in code: unseekable streamed files report source_not_supported for repackage recovery; TS timestamp search widened. Cueless-file device acceptance remains. |
| B-20.20 | Already fixed: active output governs passthrough; decode remains beside passthrough; Media3 adds Fire OS HDMI/surround evidence. |
| B-20.21 | Already fixed: common low-memory builder, PCM gain, and fresh player after listening retirement; release now also closes the player. |
| B-20.22 | Original Apple issue outside lane; analogous Android teardown corrected here for both engines. |
| B-20.23 | Original Apple observer issue outside lane; Android native callback/resource ownership reviewed and tightened. |
| B-14.2 | Fixed further: resumed partial writers are serialized and actual byte identity is durable. Unidentified old partials restart safely once. |
| B-14.12 | Signed-in common player already chooses mpv; signed-out launch fallback still uses VideoView and lacks mpv (below). |
| B-14.18 | Original iOS background verification outside lane; Android verification now requires exact immutable size/hash and respects cancellation. |
| B-14.19 | Already fixed launch task/state reconciliation; strengthened worker/listener replacement safety here. |
| B-14.20 | Already fixed foreground timeout/restart-on-next-launch policy; preserved. No restart while process is dead. |
| B-10.22 | Already fixed PCM gain/amplification helper; real Media3 API signatures reviewed. |
| B-10.34 | Already fixed optional listening-renderer fallback; network failures still do not trigger it. |
| B-22.4 | Already fixed unreadable-versus-damaged storage handling and explicit reset; preserved. |
| B-22.9 | Already fixed in shared server-list deadline; outside native changes. |
| B-22.11 | Already fixed shared Retry-After parsing; outside native changes. |
| B-22.13 | Already fixed bounded shared SSE parser; native stream remains chunk/time bounded. |
| B-22.15 | Not fixed: reserved control admission needs an operation class from the shared request path before Kotlin can reserve capacity. |
| B-22.18 | Already fixed verified-route streams; old stream/exchange cancellation identity tightened here. |
| B-22.19 | Not fixed: Vega XHR redirect interception needs its JS/runtime owner, no Android source equivalent. |
| B-22.20 | Already fixed: one-shot request bodies and no implicit write retry; verified against OkHttp 4.12 APIs. |
| B-22.23 | Already fixed server header exposure; outside Android. |

## API changes

No public route, registry, payload or schema shape changes. Native module/view/event names and JavaScript argument/fact fields remain the existing engine contract. `release` now resolves after actual player teardown. `PorticoDownloads.start` rejects missing/invalid required artifact SHA-256 or byte count with the existing `invalid_download` code; current JavaScript supplies both from the grant. Internal `partial.sha256` and detached removal directories are device storage only.

## Schema changes

None. No server schema or copied package edits.

## Owner or viewer settings added or wired

No new setting or screen. Existing Player (Automatic default), Wi-Fi/mobile-data preference, caption size (medium/default scale), caption background (translucent), and system caption settings remain. System caption colours and paused style updates now take effect in both Android engines. ASS keeps its own appearance. Existing normalization, gapless, crossfade and audiobook comfort policy were reviewed and preserved.

## Needed from others

- **Actual Android build/device qualification:** complete the checklist below. Groovy syntax and JNI linking do not establish task graph evaluation, AAR/APK packaging, resource linking, Hermes/New Architecture interop or hardware rendering.
- **Native JS/launch owner:** `Downloads.tsx` falls back to `offlineDownloads.play` when no `playLocal` is supplied; that invokes `PorticoOfflinePlayerActivity` in `PorticoDownloads.kt`. It remains VideoView with basic controls, without mpv, bundle subtitles/chapters/markers. Route launch-mode downloads through the common player so engine choice and the bundled extras work when signed out. This requires ordinary screen/lifecycle work outside this lane; no second offline engine/session contract was invented.
- **Shared request owner:** operation classes/control reservation for B-22.15. Current bounded Kotlin transport cannot distinguish bulk reads from control actions from its existing arguments.
- **Live playback owner:** behind-live-window facts are delivery failures, not media incompatibility. Confirm the shared live window recovery re-prepares at an authorized window/edge rather than leaving the viewer with a failed channel.
- **Documents:** `Native/ENGINE-CONTRACT.md` section 10 still describes Apple television HDR limitations as universal. Android conditional direct HDR has different declared limitations. Section 7's cue example also describes the earlier path; Kotlin uses `/v1/media/subtitles/<grant>/<resource>/<revision>`. State the actual paths and Android caption/typeface limitation. Correct predecessor reports' suggestion to package the archive helper or libc++ dependency for this pinned libmpv: ELF inspection shows neither is required.
- **Distribution:** libmpv/FFmpeg/libass and other transitive notices/source availability remain necessary in the licence/distribution work. No dependency upgrade or licence policy change here.

## Test notes for other environments

### Checks performed here

- Initial cached standalone Kotlin checks passed both flavours. Final explicitly pinned checks selected ten Media3 1.10.1 AAR `classes.jar` files before other cached declarations; Google and Fire TV both passed (compiler exit 0). A JVM perfdata lock warning during concurrent checks was environmental, not a Kotlin diagnostic.
- Java bridge: `javac --release 17` against Android 36 headers; passed.
- JNI: NDK 27.1 clang, API 24, C11, `-Wall -Wextra -Werror`, shared link with `--no-undefined` and 16 KiB alignment; **all four ABIs passed**, linked against the actual pinned binaries. C API header matches the exact source header byte for byte. No native app build or JNI execution.
- Four Gradle files parsed and converted to Groovy AST with Groovy 4.0.27; checksum byte formatting also checked. No Gradle DSL/task evaluation.
- All 13 Android manifest/resource XML files parsed. Static service/activity/provider/source-set names match source files; Google Cast metadata is confined to Google flavour, Fire TV has no Cast module.
- Native and Vega `npx tsc --noEmit`; passed. Vega contains no Kotlin/Java/JNI/Gradle changes to review.
- Standalone `download-work-tests.kt` JVM regression passed. Existing tests were not broadened into a whole suite.
- Temporary reflection harness against the compiled native classes passed nine assertions: malformed/missing route revisions are rejected without mutation, stale stream cancellation preserves its replacement, and current cancellation closes it. A scratch `SystemClock` stub supports the stream entry constructor; controller constructors/UI/native runtime are intentionally not exercised.
- `git diff --check`; clean.

Scratch scripts/evidence: `_program/scratch/android-native-review/ktcheck-exact.sh`, `download-tests.sh`, `native-regression.sh`, `parse-gradle.groovy`, downloaded sources, four release archives/libraries and four linked JNI outputs. None is added to the product worktree. Run the JVM download test with `bash .../download-tests.sh "$PWD/apps/native"` from this worktree. Its shipped test entry point is `tv.getportico.portico.native.Download_work_testsKt`; it needs only Kotlin stdlib and `PorticoDownloadWork.kt`.

### Primary API evidence

- [Pinned media-kit dependency revisions](https://github.com/media-kit/libmpv-android-video-build/blob/v1.1.11/buildscripts/include/depinfo.sh), [mpv build](https://github.com/media-kit/libmpv-android-video-build/blob/v1.1.11/buildscripts/scripts/mpv.sh) and [full FFmpeg flavour](https://github.com/media-kit/libmpv-android-video-build/blob/v1.1.11/buildscripts/flavors/full.sh): mpv `78d43740f52db817d98bcf24fb30a76ab6fa13ff`, FFmpeg 6.0, libass 0.17.1. Binary version string agrees (`v0.36.0-549-g78d43740f5-dirty`).
- [Exact mpv client API](https://github.com/mpv-player/mpv/blob/78d43740f52db817d98bcf24fb30a76ab6fa13ff/libmpv/client.h), [client implementation](https://github.com/mpv-player/mpv/blob/78d43740f52db817d98bcf24fb30a76ab6fa13ff/player/client.c), [command implementation](https://github.com/mpv-player/mpv/blob/78d43740f52db817d98bcf24fb30a76ab6fa13ff/player/command.c), [options](https://github.com/mpv-player/mpv/blob/78d43740f52db817d98bcf24fb30a76ab6fa13ff/DOCS/man/options.rst) and [properties/commands](https://github.com/mpv-player/mpv/blob/78d43740f52db817d98bcf24fb30a76ab6fa13ff/DOCS/man/input.rst). Names, formats, tokens, event ids/end reasons, NUL-terminated async argument copying and termination semantics match the bridge. All used symbols link. `stop` requests stopping; terminate_destroy is the actual teardown fence.
- Exact Android [surface ownership](https://github.com/mpv-player/mpv/blob/78d43740f52db817d98bcf24fb30a76ab6fa13ff/video/out/android_common.c) and [AudioTrack output](https://github.com/mpv-player/mpv/blob/78d43740f52db817d98bcf24fb30a76ab6fa13ff/audio/out/ao_audiotrack.c): the window is a global Java Surface reference, retained through VO lifetime; the VM setter is exported. Surface changes are the intentional synchronous exception to the core queue. JNI event data is copied before the next wait; global/local references and event-thread joining were reviewed.
- [Media3 1.10.1 exceptions](https://github.com/androidx/media/blob/1.10.1/libraries/common/src/main/java/androidx/media3/common/PlaybackException.java), [SimpleBasePlayer](https://github.com/androidx/media/blob/1.10.1/libraries/common/src/main/java/androidx/media3/common/SimpleBasePlayer.java), [renderers](https://github.com/androidx/media/blob/1.10.1/libraries/exoplayer/src/main/java/androidx/media3/exoplayer/DefaultRenderersFactory.java), [audio sink](https://github.com/androidx/media/blob/1.10.1/libraries/exoplayer/src/main/java/androidx/media3/exoplayer/audio/DefaultAudioSink.java), [audio capabilities](https://github.com/androidx/media/blob/1.10.1/libraries/exoplayer/src/main/java/androidx/media3/exoplayer/audio/AudioCapabilities.java) and [OkHttp data source](https://github.com/androidx/media/blob/1.10.1/libraries/datasource_okhttp/src/main/java/androidx/media3/datasource/okhttp/OkHttpDataSource.java) fetched/reviewed with their actual classes. Reclaimed codecs are explicitly a higher-priority resource failure.
- React Native tvOS 0.83.10-1 and its local bridge/host/view sources were checked for exported names, nullable/platform signatures, application-thread dispatch and reload/module lifetimes. OkHttp 4.12 headers/methods, one-shot bodies and cancellation match the installed API. Android version guards preserve API 24 Google / API 25 Fire TV floors for AudioFocusRequest, PiP, Surface frame-rate voting, display preferences and service timeout callbacks.

### Cold first build: phone/tablet, Android TV, Fire TV

Use an integrated checkout containing the final snapshot on Ubuntu with JDK 17, Android SDK/platform/build tools 36, NDK `27.1.12297006`, CMake 3.22.1 and network access. No system Gradle is needed: use the checked-in Gradle 9 wrapper. These are instructions for the person with build authorization/tools; none was run here.

```sh
cd apps/native
npm ci --legacy-peer-deps
cd android
./gradlew :portico-mpv:fetchLibmpv
./gradlew :app:assembleGoogleDebug -PreactNativeArchitectures=x86_64
./gradlew :app:assembleFiretvDebug -PreactNativeArchitectures=x86_64
./gradlew :app:assembleGoogleRelease :app:assembleFiretvRelease \
  -PreactNativeArchitectures=armeabi-v7a,arm64-v8a,x86,x86_64
```

Google is one APK for phones/tablets and Android TV; television mode chooses its layout. Fire TV is its separate flavour, minimum API 25. Release currently uses the project's debug signing configuration and embeds JavaScript; these are test artifacts, not store-signed releases.

Inspect both release APKs, usually `app/build/outputs/apk/{google,firetv}/release/app-<flavour>-release.apk`:

```sh
unzip -l app/build/outputs/apk/google/release/app-google-release.apk
unzip -l app/build/outputs/apk/firetv/release/app-firetv-release.apk
```

Pass: `lib/<abi>/libmpv.so` and `lib/<abi>/libporticompv.so` once each for every selected ABI, complete React Native/Hermes libraries, and an embedded release JS bundle. Inspect the merged manifest: expected launcher, listening/download services, Google-only Cast options metadata and unrestricted cleartext with system TLS trust.

Most likely first-build failures and precise response:

1. **Fetch failure/checksum mismatch:** inspect the exact release URL, archive name and actual digest. All four pins were verified here. Never accept a new digest automatically. For an intentional locally built LGPL libmpv, use `-PporticoLibmpvDir=/absolute/path` with `<abi>/libmpv.so` for every selected ABI; the task validates presence every time.
2. **CMake missing libmpv:** confirm `fetchLibmpv` runs before configure/link tasks, the imported directory resolves as expected and the ABI is supported. The task dependencies were read and Groovy-parsed, but not evaluated. Keep the imported `mpv` target linked to `porticompv`.
3. **Missing/duplicate packaged libmpv:** inspect the library AAR and APK. Modern AGP packages a linked imported CMake library. If this task graph does not, add exactly one explicit jniLibs source directory for the imported binaries; remove duplicate packaging if the import already supplies them. Do not blindly add both.
4. **Unresolved JNI/mpv symbols:** use the exact pinned binary and client header. All four link here with no undefined strong symbols. `PorticoMpvNative` and Listener names are kept by consumer rules; both Surface and event method signatures agree. Do not add the archive's `libmediakitandroidhelper.so` or libc++ as a guessed fix: neither is a dependency of these binaries.
5. **Java/Kotlin task target mismatch or generated symbol/resource error:** confirm Kotlin 2.1.20/Java 17, the React Native Gradle plugin from npm install, current generated `BuildConfig`/PackageList/R and correct Google/Fire TV source sets. Standalone checks used cached generated declarations; they do not generate resources/autolinking.
6. **mpv setting absent at runtime:** inspect `adb logcat -s PorticoMpv` for the actual loading cause; check package ABI/library presence. Successful APK compilation alone does not establish `available=true` or a working view manager.
7. **Startup option error:** inspect the option named in the existing errorDetail. Keep the exact pinned manual/backend. Only appearance/performance/absent scripting switches are optional; do not silently skip scope, protocol, output or critical initialization failures.
8. **Release handoff freezes/delays:** inspect Media3's release timeout/error and mpv event-thread termination. Surface detachment must remain synchronous, mpv destruction off-main. Do not restore the dispatch-only stop fence merely to hide a teardown delay.

### Device acceptance (pass/fail)

Use the integrated server and at least: API 24 Google phone/tablet, Android TV 12+ with HDR television and receiver, 32-bit Fire TV/Fire OS 6–7 and a newer Fire TV. Emulator flavour tests do not establish real Fire OS behavior.

1. **Launch/interop:** both flavours launch signed out and signed in; native and mpv modules/views appear where supported, Cast only on eligible Google handhelds. Debug needs Metro on 19424; Release must launch with Metro absent. Fail: missing module/row, crash, unresolved .so or bundle.
2. **Engine handoff:** play a queued title, seek, switch native→mpv→native repeatedly, including paused and while loading. Pass: one audible engine, same queue/current entry, retained position within 0.5 s and paused state retained. Close playback and resume another app's sound. Watch UI latency, Media3 release timeout, mpv teardown and thread/memory growth through repeated bridge reloads.
3. **Late surface/display:** delay the player view mount by two seconds, start 23.976fps SDR and HDR. Pass: opening seconds stay held, ready follows display matching and total matching wait is bounded by ten seconds. Repeat system Always/Seamless/Never, HDR with unknown FPS and older Fire OS without frame-rate voting. Fail: first seconds lost into black, stuck startup or recurring ten-second hold.
4. **HDR/surface:** HDR10 and HLG, subtitles off/on/off, pause/seek, Home/return, rotation/resize. Pass: correct colours and visible picture; mpv direct HDR only where declared, rich subtitles tone-map as declared. Dolby Vision 5 must not be selected for mpv Automatic. If direct output fails, qualify disabling `HDR_DIRECT_OUTPUT` rather than claiming HDR works.
5. **Captions:** local VTT, server cues, embedded/external ASS with fonts, PGS and VobSub companions. Change Portico size/background and system size/foreground/background while paused; unpause and change timing ±0.5s. Pass: current choice only, immediate style update, correct timing. Test Arabic/Indic/CJK and mixed/untagged fonts; record remaining missing glyphs/typeface limits.
6. **Scope/network:** HTTP at private and public hostnames, trusted/untrusted HTTPS, HLS relative and absolute references, grant-escaping resources and redirects. Pass: allowed HTTP works, untrusted TLS/redirect/out-of-grant references fail without credential propagation. Rapidly replace titles during a failing old relay request: old failure never classifies the new title.
7. **Seek/error:** resume cueless MKV/TS, beyond-window Live TV, corrupt container, unsupported codec, simulated reclaimed codec and AudioTrack output failure. Pass: repackage or permitted recovery for real source errors; output/resource/network failures never produce remembered file incompatibility. Empty/truncated mpv startup must terminate with a fact. Unknown-duration early EOF needs particular attention.
8. **Download race:** large file on a slow link; Pause/Continue repeatedly, renew URL during transfer, cancel a queued intermediate attempt, Remove then immediately request the same id. Kill/relaunch during each phase. Pass: exact final hash/count, continued identified partials, one writer per id, no old extras replacing/deleting new files. Unknown-length/oversized/short upstream responses must remain bounded and never become ready.
9. **Download extras/foreground:** race subtitle/artwork fetch against removal/re-add and another fetch of the same name. Pass: only latest extras committed, no orphan partial overwrite. Switch Wi-Fi/mobile data, exhaust Android dataSync allowance and reopen. Pass: bytes retained, system timeout stops cleanly, next foreground run continues. Signed-out launcher MKV/extras remains the known gap below.
10. **Audio/music:** +6dB measured gain, saturation, crossfade, album gapless, 5-minute/1-hour audiobook comfort pause, explicit paused seek, renderer failure fallback, then video. Pass: correct gain/transition/rewind/fallback; new video has normal output/buffer policy and no leftover music player.
11. **System/lifecycle:** calls, permanent/transient focus loss, ducking, headphone unplug, headset/remote media keys, PiP, Home/screen off and activity recreation. Pass: video pauses and intent agrees, PiP continues where permitted, listening continues in foreground service, one active media session. First title's lock-screen artwork loads; oversized unknown-length artwork is refused.
12. **Storage/events:** disposable device only: remove/invalidate Keystore key, inject temporary Keystore failure and corrupt one record; reset/reload repeatedly. Pass: confirmed loss resets correctly, ambiguous failures retain originals, replacement bridges survive old invalidation. Keep Home/group event streams through cancellation/reconnect and route changes; no duplicate stream, old callback cancellation or request-body replay.

## Not finished

- **No Gradle build, CMake task-graph/package evaluation, APK installation, Android lint, device/emulator playback, GPU/HDR/audio/thermal or minimum-OS acceptance occurred.** Native application behavior remains unestablished despite compiler/link/harness checks.
- Signed-out launcher downloads still use `PorticoOfflinePlayerActivity`/VideoView, without mpv or bundle extras. Common-player launch wiring is an ordinary JS screen/lifecycle dependency outside this review.
- mpv direct HDR/output switching, subtitle/fonts, seek restart ordering, PiP/lifecycle, exact HDMI layouts/passthrough and frame-rate matching need the physical matrix above. Arbitrary system caption typeface cannot be applied to mpv-drawn text with this font backend; mixed-script/untagged fallback remains limited.
- Unknown-duration early EOF may still resemble normal completion. No supported stream-completion proof exists here to distinguish it reliably; known-duration relay failure checks remain.
- Media3 release uses its required application thread and bounded release mechanism; qualification must catch pathological timeout/latency. No zero-latency teardown guarantee is claimed.
- B-19.8/10/14/17, shared control reservation B-22.15, Vega redirect behavior and live-window orchestration remain their listed device/client dependencies. No Apple/shared/server feature completion is claimed.

Final Kotlin checks, Java compilation, download-work regression and whitespace checks passed after all source changes. The temporary route/event regression also passed against the final classes (nine assertions).

Final snapshot: `snapshots/android-native-review/004` — “Complete Android native playback and build review”, cumulative against `1696b6f`. The final patch and untracked inventory match the working tree byte for byte. Fourteen tracked files and two new files are included. No product worktree file changed after this snapshot; only this programme report was updated to record completion.
