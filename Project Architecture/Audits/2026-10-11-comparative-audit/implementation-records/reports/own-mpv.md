# Lane report: own-mpv (our own LGPL 2.1 media engine for Android and Apple)

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/native-own-mpv` (branch `audit-implementation/own-mpv`, from main-native `8c8b973`). Commits: see section 10. Nothing pushed.

## 1. Outcome

- **The mpv engine is now built by us, from pinned source, with one recipe for both platforms, as separate dynamic libraries under LGPL 2.1 or permissive licences.** The package is `third_party/media-engine/` in the native repo. FFmpeg is the server's exact pin (8.1.2, same URL and SHA-256; `check-ffmpeg-pin.py` enforces it).
- **Android is built and verified for real, on this host, for all four ABIs** (`arm64-v8a armeabi-v7a x86 x86_64`) with the app's NDK 27.1.12297006: twenty `.so` files per ABI (libavcodec.so … libssl.so, libcrypto.so, libass.so, libmpv.so …), plain SONAMEs, 16 KB page alignment, stripped. `verify.py android` passes every check (licences, linking, contents, recorded dependency graph, notices and source bundle).
- **The Android app consumes it.** `portico-mpv` no longer downloads media-kit; it packages our libraries and the licence notices (`assets/media-engine/THIRD-PARTY-NOTICES.txt`) and stops with build instructions when they are missing. `./gradlew :portico-mpv:assembleRelease` and `./gradlew :app:assembleGoogleDebug` (all four ABIs) **built successfully** here.
- **It plays on Android**: on the Android 16 x86_64 emulator, a small libmpv program (`smoke/android-smoke.sh`) loaded the whole engine from its plain `.so` names and played to the end: H.264/AAC with ASS subtitles (MKV), HEVC Main 10 (MP4), AV1 through dav1d with Opus, VP9, MPEG-2/AC-3 (TS), E-AC-3, FLAC, HLS, and a Windows-1251 SRT that came out as correct UTF-8 Cyrillic (uchardet + libiconv). OpenSSL refused an HTTPS server without trusted roots ("certificate verify failed") and accepted it with roots.
- **Apple is written and reviewed, not run** (no Mac here): `build-apple.sh` builds iOS device, iOS simulator (arm64 + x86_64), tvOS device and tvOS simulator (arm64) slices, one dynamic framework per library with `@rpath` install names, Info.plist (MinimumOSVersion read from both pbxproj files, 15.1), a generated `PrivacyInfo.xcprivacy` and licence notice, combined into one `.xcframework` per library. Both Xcode projects now depend on a **local** Swift package (`third_party/media-engine/apple/Package.swift`, product `PorticoMediaEngine`) whose `Libmpv` framework keeps the module name, so the `#if canImport(Libmpv)` Swift code is unchanged. The Apple playbook is in section 7.
- **Certificate trust**: mpv never connected to the server itself before and does not now (the in-app relay fetches with OkHttp / URLSession, i.e. exactly the app's own trust); mpv is now additionally told `tls-verify=yes` on both platforms, so if it is ever handed an HTTPS address it verifies instead of trusting blindly (mpv's default). Plain HTTP keeps working.
- Native `npm test` **665/665** pass (660 + 5 new), Vega **87/87**, `tsc --noEmit` clean for native, native tests and Vega.

## 2. Components

| Component | Version | Licence | Platforms | Why needed |
|---|---|---|---|---|
| FFmpeg | 8.1.2 (= server pin, sha256 464beb5e…) | LGPL-2.1-or-later as configured | both | Demux/decode/parse, network, HTTPS. All decoders, demuxers, parsers, bitstream filters; **no encoders, muxers, devices**; 21 filters (the ones mpv inserts: bwdif, yadif, w3fdif, rotate, transpose, flips, crop, pad, format, scale, aresample, hwdownload/upload/map, …); protocols file/http/https/tcp/tls/crypto/data/async/cache/subfile/httpproxy; MediaCodec (Android), VideoToolbox (Apple). `--disable-autodetect`, so nothing on a build machine can sneak in. |
| OpenSSL | 3.5.9 (LTS) | Apache-2.0 | both | TLS for FFmpeg (accepted without `version3` since GPL is off). Client subset: no engines/modules/legacy provider, no national/obsolete ciphers, SRP, SCTP, QUIC, CMS, TS, CT, SLH-DSA. TLS 1.2/1.3 incl. hybrid ML-KEM stays. DTLS-SRTP kept (FFmpeg's TLS code references it). |
| mpv | 0.41.0 | LGPL-2.1-or-later (`-Dgpl=false`) | both | Player core. Android: `vo=gpu` + `gpu-context=android` (OpenGL ES) and `mediacodec_embed`, as the engine uses. Apple: `vo=gpu-next` + Vulkan/MoltenVK, `hwdec=videotoolbox`. |
| libass | 0.17.5 | ISC | both | Styled subtitles (CoreText fonts on Apple). |
| FreeType | 2.15.0 | FTL | both | Glyphs for libass. |
| HarfBuzz | 14.6.0 | MIT | both | Shaping for libass (built without libstdc++). |
| FriBidi | 1.0.17 | LGPL-2.1-or-later | both | RTL text. |
| libunibreak | 8.0 | Zlib | both | libass line breaking (optional in libass; kept: CJK subtitles wrap correctly). |
| dav1d | 1.5.4 | BSD-2-Clause | both | AV1 software decode. |
| libplacebo | 7.360.1 (+ pinned submodules glad, jinja, markupsafe, fast_float, Vulkan-Headers) | LGPL-2.1-or-later | both | A required dependency of mpv 0.41 (vo=gpu uses it too). Android: OpenGL only; Apple: Vulkan + glslang. |
| Little CMS | 2.19.1 | MIT | both | ICC colour management (its GPL-3 plugins `fastfloat`/`threaded` explicitly off). |
| libdovi | 3.4.0 | MIT (Rust deps MIT/Apache, vendored) | both | Dolby Vision metadata for libplacebo's gpu-next. On Android it is linked but only gpu-next would use it; kept so the recipe is one and Android can move to gpu-next. Built with cargo-c. |
| uchardet | 0.0.8 | LGPL-2.1-or-later (chosen from MPL/GPL/LGPL) | both | Subtitle charset detection. |
| **libiconv** (added) | 1.19 | LGPL-2.1-or-later (library only) | Android | **Needed**: mpv refuses uchardet without iconv, and Android's iconv exists only from API 28; the app's minSdk is 24. Apple has iconv in libSystem. |
| MoltenVK | 1.4.2 | Apache-2.0 | Apple | Vulkan on Metal. Khronos's **release build** (dynamic frameworks, min OS 15.0), pinned by SHA-256; not rebuilt from source (permissive licence, and a source build needs Xcode plus four more repositories). |
| glslang | 16.6.0 | BSD-3/BSD-2/MIT/Apache-2.0 (+ GPL-3 with Bison exception for its generated parser) | Apple | GLSL→SPIR-V compiler libplacebo's Vulkan path needs. **Chosen over shaderc** (one repository, no SPIRV-Tools). Built static, no optimizer, no HLSL, linked into Libplacebo (permissive code inside an LGPL library is fine). |

Tools (all pinned in the lock, user-level installs into the work directory): meson 1.12.1, ninja 1.13.0, cmake 3.31.10 (pip venv), nasm 3.02 (from source, sha256), Rust 1.99.0 (must match; Android targets added with `rustup target add`), cargo-c 0.10.26 (`cargo install --locked`), Android NDK 27.1.12297006, Xcode ≥ 16.0. zlib is the platform's own; the C++ runtime on Android is React Native's `libc++_shared.so` (libplacebo and uchardet need it; the engine ships no second copy).

## 3. Dropped, and why

- **GnuTLS, Nettle, GMP** (MPVKit) and **mbedTLS** (media-kit): OpenSSL does TLS. GMP was also what forced MPVKit's FFmpeg to LGPLv3.
- **libbluray, libsmbclient, libxml2 (DASH), uavs3d, Lua/LuaJIT, JavaScript, libarchive, DVD/CD, rubberband, zimg, vapoursynth**: the app plays files and HLS from its own server; never discs, SMB shares, DASH, AVS3 or scripts.
- **Every encoder and muxer** (MPVKit kept a few): a player encodes nothing; the engines decode sound to PCM (no S/PDIF passthrough), and Portico never asks mpv for screenshots or recording.
- **Most FFmpeg filters**: only those mpv inserts itself are kept (saves ≈3.5 MB per ABI). The engines set no user filters.
- **shaderc**: glslang alone is enough for libplacebo.
- `verify.py` fails any build containing GnuTLS, Nettle, GMP, mbedTLS, x264, x265, libbluray, libsmbclient or fdk-aac.

## 4. Decisions I made

1. **All FFmpeg decoders/demuxers/parsers, not a curated list.** Superset of both old engines (media-kit "full" had all; MPVKit a curated list) and of the server's `requirements.v1.json` decode features. `verify.py` additionally requires a named list the app relies on (H.264, HEVC, AV1+libdav1d, VP8/9, MPEG-1/2/4, MS-MPEG4v3, VC-1/WMV3, FLV, AAC, AC-3, E-AC-3, TrueHD/MLP, DTS, FLAC, Opus, Vorbis, MP2/3, ALAC, PCM incl. Blu-ray/DVD, WMA2/Pro, SubRip, ASS/SSA, WebVTT, mov_text, PGS, DVD, DVB, CEA-608; MediaCodec decoders on Android; VideoToolbox hwaccels on Apple; the containers and parsers to match). No `--enable-small`: it builds with -Os, which slows software HEVC/AV1 decode on weak TV boxes.
2. **Size**: the engine is bigger than media-kit's single libmpv.so (section 6). The price is separate libraries, OpenSSL instead of mbedTLS, libplacebo/libdovi/lcms2/libiconv, and -O2/-O3 FFmpeg. If download size matters more, `--enable-small` and a curated decoder list are the two levers (a decision for Justin, section 9).
3. **Plain `libssl.so` / `libcrypto.so` names** (as the brief lists). Safe from API 24 (linker namespaces keep the app's copies apart from the system's BoringSSL ones); Qt-style `_3` suffixes were considered and not needed.
4. **tvOS simulator is arm64 only.** Rust (libdovi) has no x86_64 tvOS simulator target (it is tier 3, not in rustup). iOS simulator keeps x86_64. The Apple TV simulator therefore needs an Apple-silicon Mac.
5. **MoltenVK from Khronos's release, not source** (see table).
6. **mpv patch for iOS/tvOS** (`patches/mpv/0001-…`): upstream mpv 0.41 has no Vulkan context for an embedder-owned `CAMetalLayer`; the Swift engine uses `gpu-context=moltenvk` (as MPVKit's patched mpv provided). I wrote our own small context (`video/out/vulkan/context_moltenvk.m`, LGPL-2.1+) plus a meson option, and stopped mpv's build from adding the Mac's `-L/usr/local/lib` when cross-compiling. MPVKit's patch was read for reference only, not copied.
7. **libdovi patch**: drops its benchmark-only dev-dependency (criterion) and the 61 lock entries only it used, so the vendored crates in the source bundle are 19 MB instead of 190 MB. Every remaining crate keeps upstream's pinned version and checksum.
8. **The bridge compiles against the engine's own `mpv/client.h`** (0.41, API 2.5); the vendored 2.2 copy was deleted so headers and library can never drift.
9. **Gradle packaging**: libmpv.so stays an IMPORTED CMake library (AGP packages it as before); the other nineteen are staged by a `Sync` task into `build/media-engine/jniLibs` (no duplicate-file conflicts, verified in the AAR and APK). `-PporticoLibmpvDir` now names the engine's `out/android` folder (`lib/<abi>/*.so`, `include/`, `notices/`).
10. **No Mozilla root bundle on Apple, and no CA export on Android.** mpv is never allowed to speak TLS (its protocol list is `file,http,tcp,crypto`); the relay does all server traffic with the app's own trust. Shipping `cacert.pem` would create a second, unmaintained trust store that ignores Apple's and Google's revocations — the opposite of "consistent with the app". With `tls-verify=yes` and no roots, a future direct-HTTPS path fails closed (shown on the emulator) until someone deliberately adds a trust source.
11. **Apple privacy manifests are generated from each binary's imports** (file-timestamp APIs → C617.1, `mach_absolute_time` → 35F9.1); a disk-space API would stop the build for a human decision rather than a guessed reason. Every framework gets one (OpenSSL included, as Apple lists it).
12. **`manifest/apple.json` is provisional**, derived from the Android graph (no libiconv; libmpv and libplacebo load MoltenVK). The playbook says how to confirm it on the Mac.
13. **Notices in the apps**: Android ships `assets/media-engine/THIRD-PARTY-NOTICES.txt`; Apple puts each component's `NOTICE.txt` in its framework and the full file in `Libmpv.framework`. No in-app screen shows them yet (section 11).

## 5. Android build and verify results (what I ran)

Host: personal Ubuntu, `nice -n 10`, `-j8`. Work and output under `_program/scratch/own-mpv/` (`work/`, `out/`).

```
MEDIA_ENGINE_WORK_DIR=…/scratch/own-mpv/work MEDIA_ENGINE_OUT_DIR=…/scratch/own-mpv/out \
  nice -n 10 third_party/media-engine/build-android.sh
…
[media-engine] Notices: …/out/android/notices/THIRD-PARTY-NOTICES.txt
[media-engine] Source bundle: …/out/android/source/portico-media-engine-android-source-r1.tar
Wrote …/out/android/manifest.json
4625 checks passed, 0 failed.
[media-engine] Done: …/out/android
```

Timing on this 12-core host: all four ABIs from scratch ≈ 20 minutes (plus ≈ 6 minutes once for cargo-c and nasm); the last run, after the libdovi patch, rebuilt libdovi → mpv for four ABIs in 7 minutes. Work directory ≈ 2.2 GB, output 242 MB (of which the source bundle 133 MB). `python3 verify.py android --out …` run again on the final output: **4625 checks passed, 0 failed**. `./check-ffmpeg-pin.py main-server/third_party/ffmpeg/sources.lock.json` → "FFmpeg 8.1.2 matches the server's pin".

FFmpeg configure summary (arm64): `External libraries: jni mediacodec zlib libdav1d openssl`; config.h `FFMPEG_LICENSE "LGPL version 2.1 or later"`. mpv features (`FULLCONFIG`): `android android-media-ndk audiotrack egl-android ffmpeg gl glibc-thread-name iconv lcms2 libass libdl libplacebo linux-fstatfs memrchr posix ppoll pthread-condattr-setclock uchardet vector vt.h zlib` (no `gpl`).

Dependency graph (identical on all ABIs; system libraries only libc/libm/libdl/liblog/libz/libandroid/libmediandk/libEGL/libc++_shared):
- libmpv → libass, libavcodec, libavfilter, libavformat, libavutil, libiconv, liblcms2, libplacebo, libswresample, libswscale, libuchardet
- libavformat → libavcodec, libavutil, **libssl, libcrypto**; libavcodec → libavutil, libdav1d, libswresample
- libass → libfreetype, libfribidi, libharfbuzz, libiconv, libunibreak; libplacebo → libdovi, liblcms2 (+ libc++_shared)
- libssl → libcrypto; everything else depends only on system libraries.

Gradle (with `-PporticoLibmpvDir=…/scratch/own-mpv/out/android`, `-Dorg.gradle.jvmargs=-Xmx3g`, a private copy of the main checkout's node_modules in the worktree, git-ignored):
- `./gradlew :portico-mpv:assembleRelease` → BUILD SUCCESSFUL (1 m 6 s). AAR: 21 `.so` per ABI (our 20 + libporticompv.so) and the notices asset; libporticompv.so NEEDED libmpv.so, 16 KB LOAD alignment.
- `./gradlew :portico-mpv:checkMediaEngine` without the property → fails with the "not built … run third_party/media-engine/build-android.sh …" message (default path resolves to the worktree's `third_party/media-engine/out/android`).
- `./gradlew :app:assembleGoogleDebug` → BUILD SUCCESSFUL (8 m 44 s). APK has every engine library for all four ABIs, React Native's libc++_shared.so, the notices asset; libraries stored uncompressed (as AGP does for minSdk ≥ 23).

Emulator (`Portico_Phone_API36`, Android 16 x86_64, started headless with `-read-only -no-snapshot`, stopped afterwards):
```
PATH=$ANDROID_HOME/platform-tools:$PATH ANDROID_SERIAL=emulator-5580 third_party/media-engine/smoke/android-smoke.sh --abi x86_64
Device: sdk_gphone64_x86_64, Android 16, ABI x86_64
PASS h264-aac-ass        (H.264 + AAC; ASS line "Portico styled line" seen)
PASS hevc-main10         (H.265 / HEVC in MP4)
PASS av1-dav1d-opus      ("decoder":"libdav1d", "decoder":"opus")
PASS vp9-opus
PASS mpeg2-ac3-ts        (MPEG-2 video, ATSC A/52A (AC-3), mpegts)
PASS eac3
PASS flac
PASS hls                 (format hls)
PASS cp1251-subtitles    (sub-text "Привет, мир! Это субтитры в кодировке Windows-1251.")
PASS tls-verify-refuses  (tls: error:0A000086:SSL routines::certificate verify failed)
PASS tls-verify-accepts  (with this host's CA bundle: TLS succeeds, "unrecognized file format")
11 passed, 0 failed.
```
**In the real app:** `./gradlew :app:assembleGoogleRelease -PreactNativeArchitectures=x86_64` (BUILD SUCCESSFUL, 8 m 2 s), installed on the same emulator and launched (no server, so nothing was played). logcat: `nativeloader: Load …base.apk!/lib/x86_64/libmpv.so using class loader ns clns-9 …: ok` then `… libporticompv.so …: ok`, and no "The mpv libraries could not be loaded" warning: the app's own loader resolved libmpv and its nineteen dependencies (and React Native's libc++_shared.so) straight from the APK, so the Player setting is offered. The app was uninstalled and the emulator stopped afterwards (it ran with `-read-only`, so the AVD is unchanged).

Native tests: `npm test` (apps/native) 665 pass / 0 fail; `npm test` (apps/vega) 87 / 0; `npx tsc --noEmit` and `npx tsc -p test/tsconfig.json --noEmit` (native) and `npx tsc --noEmit` (vega) clean.

## 6. Sizes (stripped, uncompressed; and zipped −9 for a download estimate)

| ABI | Ours (20 libraries) | media-kit v1.1.11 libmpv.so | Ours zipped | media-kit jar |
|---|---|---|---|---|
| arm64-v8a | 29.2 MB | 16.1 MB | 13.7 MB | 7.8 MB |
| armeabi-v7a | 20.9 MB | 15.3 MB | 12.1 MB | 7.6 MB |
| x86 | 33.0 MB | 16.9 MB | 14.6 MB | 7.9 MB |
| x86_64 | 35.5 MB | 19.8 MB | 14.9 MB | 8.7 MB |

Biggest pieces (arm64): libavcodec 11.3 MB (all decoders, -O3), libcrypto 4.3 MB, libmpv 2.0 MB, libavformat 1.8 MB, libharfbuzz 1.3 MB, libswscale 1.2 MB, libiconv 1.1 MB. Play delivers per-ABI splits compressed, so the download cost is roughly the zipped column for one ABI.

## 7. Apple playbook (for the internal Apple test plan)

1. **Prerequisites.** Xcode 16+ selected (`xcode-select -p`), iOS and tvOS platforms installed (Xcode > Settings > Components), `rustup` (then `rustup toolchain install 1.99.0` if `rustc --version` differs), `brew install pkgconf`. About 15 GB free.
2. **Build the engine** from the repository root: `third_party/media-engine/build-apple.sh` (about an hour the first time). It stops early and says why if Xcode, an SDK, a tool, a deployment target or a checksum is wrong, and ends by running `verify.py apple`.
3. **First verify run.** `manifest/apple.json` was derived from the Android build. If verify reports differences in that file only, read them (each framework should load only other engine frameworks and Apple system libraries); if they are right, run `python3 third_party/media-engine/verify.py apple --update-expected` and commit the file. Any other failure is a real problem: report it.
4. **Pods**: `cd apps/native/ios && pod install`, and the same in `apps/native/tvos`.
5. **Open** `apps/native/ios/Portico.xcworkspace`. `PorticoMediaEngine` appears under Package Dependencies as a local package ("missing binary target" means step 2 did not finish). Build for an iOS simulator, then a device; then `apps/native/tvos` (tvOS simulator needs an Apple-silicon Mac).
6. **Test first, on a device:** Settings > Player = mpv; play H.264 MP4, HEVC 10-bit HDR MKV, AV1 (software), and an HLS stream from the server; styled ASS subtitles, a PGS track, a non-UTF-8 SRT (Cyrillic/Chinese) that must display correctly; a Dolby Vision profile 8 (and 5 if available) title on HDR and SDR screens; seek, background pause, audio-track switch, 5.1 over HDMI/AirPlay; no `dyld` "Library not loaded" in the console at launch; archive a Release build and run "Validate App" (bundle ids, minimum OS, privacy manifests of every embedded framework).
7. Report results and any `verify.py` failures.

## 8. Certificate trust: before and after

**Before.** Both engines gave mpv only `http://127.0.0.1:<port>/…` (the relay, `PorticoMpvRelay.kt` / `.swift`), whose fetches use OkHttp / URLSession: the system trust store, the app's network security config (Android: system anchors only, cleartext allowed) and ATS. mpv's own protocols were limited to `file,http,tcp,crypto` and `access-references=no` (HLS per load only after the relay rewrote every address). `tls-verify` was not set (mpv default: **no**), and the FFmpeg TLS backends (media-kit's mbedTLS, MPVKit's GnuTLS) could not see the system's roots — harmless only because mpv never spoke TLS.

**After.** Same relay design (it is the only way mpv's traffic gets exactly the app's trust, including owner certificates for the owner's domain, which chain to public CAs as the native app requires). Added `tls-verify=yes` on both platforms. FFmpeg's TLS is now OpenSSL; no roots are given to mpv, so a direct HTTPS load fails closed. Plain HTTP unchanged. Test: `test/mpv-network-trust.test.ts` pins `tls-verify=yes`, the protocol list (no https/tls) and that the relays use the app's clients with no custom trust and no redirects.

**Owner certificates.** The native app (not just mpv) trusts only publicly trusted certificates (Android: `network_security_config.xml`, system anchors, no user CAs; Apple: system trust). An owner-supplied certificate for their domain therefore works everywhere; a **self-signed** one is refused by the whole app, mpv included. If Justin wants self-signed or private-CA certificates accepted, that is an app-wide networking change (for example pinning the TLS key to the server identity the app already verifies); the relay would inherit it automatically.

Device test cases: (a) server on HTTPS with a public certificate (owner's domain): mpv plays through the relay; (b) server with a self-signed certificate: connecting/sign-in itself fails as before; a media URL would be refused by the relay with a connection-security failure, reported through the engine's existing error classification; (c) plain HTTP on any address: plays; (d) on Android also `smoke/android-smoke.sh` shows mpv's own TLS refusing without roots.

## 9. Untested, not finished, and for Justin

**Untested here:** `build-apple.sh`, the Swift package, the pbxproj change (`objectVersion` 54 → 60 for `XCLocalSwiftPackageReference`; CocoaPods 1.17 supports it), the mpv MoltenVK patch, OpenSSL/glslang/MoltenVK on Apple, everything on Apple devices. On Android: real devices (arm64, armeabi-v7a), MediaCodec hardware decoding and the JNI JavaVM hand-off with shared FFmpeg (`av_jni_set_java_vm` reached through libmpv's dependencies; the bridge already handled this name), the app's own playback with the new engine, 16 KB-page devices (alignment is verified statically).

**Decisions for Justin:**
1. **Size vs. completeness**: keep all decoders at -O3 (current), or add `--enable-small` and/or a curated decoder list to get close to media-kit's size.
2. **Publishing the corresponding source**: each release must publish `out/<platform>/source/portico-media-engine-<platform>-source-r<rev>.tar` (≈ 133 MB for Android) beside the app, e.g. as a release asset; the in-app notice says it is published with each release.
3. **Build paths in binaries**: FFmpeg's configuration string and mpv's verbose log embed the build machine's work path (e.g. `/home/justin/…`). Release builds could use a neutral work directory (`MEDIA_ENGINE_WORK_DIR=/tmp/pme`).
4. Whether self-signed owner certificates should be trusted app-wide (section 8).

## 10. Commits (branch `audit-implementation/own-mpv`, Justin's identity, not pushed)

```
f9cde38 Describe libplacebo as a requirement of mpv 0.41 in the engine README
0bf6bfc State the engine's real build time in the build instructions
128ea15 Print log lines mpv sends after a file ends in the engine smoke test
2e14d50 Describe the media engine in the native README
e12b1bd Have mpv verify TLS certificates should it ever connect by itself
d9f300e Use the media engine's local Swift package in the iPhone and Apple TV projects
12bf7d5 Package our own media engine in the Android mpv module
4ee0af1 Build the mpv media engine from pinned sources as separate LGPL 2.1 libraries
```

Files: `third_party/media-engine/**` (new); `apps/native/android/portico-mpv/build.gradle`, `src/main/cpp/CMakeLists.txt`, deleted `src/main/cpp/include/mpv/client.h`; `apps/native/{ios,tvos}/Portico.xcodeproj/project.pbxproj`; `apps/native/Native/PorticoMpvController.swift`, `PorticoMpvCore.swift` (comment); `apps/native/android/app/src/main/java/tv/getportico/portico/native/PorticoMpvController.kt`, `PorticoMpvPlaybackModule.kt` (comment); `apps/native/src/bridge/mpv-profile.ts` (comment); `apps/native/test/mpv-network-trust.test.ts` (new); `apps/native/README.md`.

Kept on disk (scratch, ≈ 250 MB): `_program/scratch/own-mpv/out/android/` (the four ABIs, headers, meta, notices, source bundle, manifest) and `_program/scratch/own-mpv/logs/` (build, Gradle, emulator and smoke logs; per-component build logs under `logs/components/`). Work files, tools and the worktree's Gradle build folders were deleted. `apps/native/node_modules` and `apps/vega/node_modules` are git-ignored symlinks to the main checkout's (as other lanes use); the Gradle runs used a private copy so nothing was written into the main checkout.

## 11. Needed from others

- **Client lanes**: an "Open-source licences" screen (Settings) that shows `assets/media-engine/THIRD-PARTY-NOTICES.txt` (Android) and `THIRD-PARTY-NOTICES.txt` inside `Libmpv.framework` (Apple). Strings via the catalogue.
- **Release/ops**: publish the source bundle with each app release; run `build-apple.sh` before Xcode builds (CI too, if any).
- **Android playback lane (optional follow-ups)**: the engine now has FFmpeg's bwdif/yadif, so software deinterlacing could be offered (`softwareDeinterlace` in `PorticoMpvPlaybackModule.kt` and `mpv-profile.ts`) after testing on a TV box; Dolby Vision profile 5 would need `vo=gpu-next` (libplacebo and libdovi are already in the build).
- **Internal docs**: the Apple test plan should take the playbook in section 7; documents that describe MPVKit/media-kit (if any) are out of date.

## 12. Standard sections

- Bugs: none assigned to this lane.
- API changes: none. Schema changes: none. Settings: none.
