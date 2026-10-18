# Lane brief: own-mpv — our own LGPL 2.1 media engine build for Android and Apple

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/native-own-mpv` (portico-react-native, branch `audit-implementation/own-mpv`, from main-native `8c8b973`). Report: `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/own-mpv.md`.

## Rules (non-negotiable)

- **Do not spawn subagents or helpers of any kind; do the work yourself.** (The Opencode helper in `_program/helper.sh` is allowed for bounded look-ups only, and you must verify what it says.)
- Commit in your worktree with Justin's normal Git identity (already configured). No AI authors, co-authors, bylines or tool names in commit messages. Never push. Do not touch Justin's main checkouts or other worktrees. Do not edit main-native directly; the orchestrator merges your branch.
- Keep secrets out of Git. No build outputs or source tarballs in Git (gitignore them).
- Disk: keep all build scratch under `_program/scratch/own-mpv/` (≤ 25 GB peak). Check `df -h /` before big steps; stop and report if free space drops under 40 GB. Delete intermediates at the end; keep only the final Android outputs and logs.
- Host is shared: run builds with `nice -n 10` and at most `-j8`.
- No sudo is available. Install tools at user level only (pip in a venv under the build package's tool directory, `rustup target add`, `cargo install`, building a pinned tool from source). Pin every tool version you install.

## Why (Justin's decision, 14 Oct 2026)

Portico ships under GPLv3. Justin owns all its copyright, so his own code can go on the app stores, but third-party GPL code cannot, and LGPL libraries must be LGPL-friendly in practice: LGPL **2.1**, separate **dynamic** libraries the user could replace, notices and corresponding source shipped. Today the mpv engine comes from two outside prebuilt projects:

- Apple: MPVKit `1.1.0-n9.0.2` (pinned in both `apps/native/{ios,tvos}/Portico.xcodeproj/project.pbxproj`). Its FFmpeg is configured `--enable-version3` (LGPLv3, forced by `--enable-gmp`), **statically linked**, and it bundles GnuTLS + Nettle + GMP, libbluray, uavs3d. Build script reviewed at `https://github.com/mpvkit/MPVKit/blob/1.1.0-n9.0.2/Sources/BuildScripts/XCFrameworkBuild/main.swift`.
- Android: media-kit `libmpv-android-video-build` v1.1.11 "full" (`apps/native/android/portico-mpv/build.gradle`): FFmpeg **6.0** `--disable-gpl --enable-version3` with mbedTLS, everything static inside one `libmpv.so`.

Decision: **build the device media engine ourselves from pinned source, one recipe for both platforms**, as separate dynamic components:

| Component | Configuration |
|---|---|
| FFmpeg | **Same version as the server's pin**: read `main-server/third_party/ffmpeg/sources.lock.json` (currently n8.1.2, with sha256). **LGPL 2.1**: no `--enable-gpl`, no `--enable-version3`, no `--enable-nonfree`. Decode/demux/parse focused (players do not encode), network on, TLS via **OpenSSL 3** (FFmpeg's configure accepts OpenSSL ≥ 3 without version3 when gpl is off — verified in configure). Not Secure Transport (deprecated, TLS 1.2 max). No GMP, GnuTLS, Nettle, mbedTLS, libbluray, uavs3d, libsmbclient. Keep every decoder/demuxer/parser the current engines rely on (compare the current media-kit "full" and MPVKit LGPL decoder lists; also the server's `requirements.v1.json` decode features). Hardware decode: MediaCodec on Android, VideoToolbox on Apple. |
| OpenSSL 3 | Latest 3.x LTS/stable, Apache-2.0, its own dynamic library (libssl + libcrypto). |
| mpv | Latest stable release compatible with that FFmpeg (0.41.x expected), `-Dgpl=false`, libmpv shared. Android: the render path the current engine uses (check `PorticoMpv*.kt` / `portico_mpv_jni.c`); Apple: what `Native/` Swift engine uses (gpu-next via MoltenVK as with MPVKit). |
| libass + FreeType + HarfBuzz + FriBidi (+ libunibreak if libass wants it) | Styled subtitles. FriBidi is LGPL-2.1-or-later — fine. |
| dav1d | AV1 software decode. |
| libplacebo, lcms2, libdovi | mpv gpu-next, colour management, Dolby Vision. libdovi is Rust (cargo-c). If libdovi proves impractical on a platform, say so and justify. |
| uchardet | Subtitle charset detection. |
| Apple only: MoltenVK + shader compiler (shaderc or glslang, whichever libplacebo/mpv needs) | mpv's Vulkan-on-Metal rendering. |

Drop anything not in this table. If you find something in the table is unnecessary for what our engines actually use, drop it too and say why; if something not in the table is genuinely required, add it with a one-line justification and its licence (must be LGPL-2.1-or-later compatible or permissive; never GPL, never LGPLv3-only, never Apache-2.0 *inside* FFmpeg's own configure licence check unless configure accepts it without version3).

## Deliverables

1. **A standard build package** in the native repo, e.g. `third_party/media-engine/` (name it sensibly), containing:
   - `sources.lock.json`: every source URL + sha256 + version, and every pinned tool version. FFmpeg's entry must match the server pin; add a check script that compares it with a given path to the server's `third_party/ffmpeg/sources.lock.json` and fails on mismatch.
   - Patches folder (only if needed; each patch explained).
   - `build-android.sh`: builds all four ABIs (`arm64-v8a armeabi-v7a x86 x86_64`, matching `reactNativeArchitectures`) with the installed NDK (`$ANDROID_HOME/ndk/27.1.12297006`), outputting one directory per ABI with one `.so` per library (libavcodec.so, libavformat.so, …, libssl.so, libcrypto.so, libass.so, libmpv.so, …) plus `libc++_shared.so` handling consistent with the React Native app. Android dynamic loader requirements: correct SONAMEs (no versioned `.so.N` names — Android packaging needs plain `libX.so`), 16 KB page alignment for arm64/x86_64 (Play requirement), stripped.
   - `build-apple.sh`: **written and reviewed carefully but not run** (no Mac here). Builds iOS device (arm64), iOS simulator (arm64 + x86_64), tvOS device (arm64), tvOS simulator (arm64 + x86_64); each library as a **dynamic framework** (Info.plist with bundle id/version, `@rpath` install names, `MinimumOSVersion` matching the apps' deployment targets — read them from the pbxproj), combined into one `.xcframework` per library. Include a `PrivacyInfo.xcprivacy` where Apple requires one for these libraries (OpenSSL is on Apple's commonly-used SDK list). Make it fail loudly and early (tool checks, Xcode version, checksums).
   - `verify.py`: runs after a build on either platform. Checks: FFmpeg's licence string (`avutil_license()` / `FFMPEG_LICENSE` in config.h) is "LGPL version 2.1 or later"; mpv built gpl=false; no forbidden libraries linked or embedded (scan `NEEDED`/load commands and symbol tables for gnutls, nettle, gmp, mbedtls, x264, x265, bluray, smbclient, fdk); each component is its own dynamic library; required decoders/demuxers/parsers present (list from your comparison); outputs match a recorded manifest; notices and source archive present.
   - `collect-notices.sh` (or part of the build): gathers every component's licence text and produces the **corresponding source bundle** (pinned source archives + patches + recipe) that a release must ship or link.
   - `README.md`: what is built, why (licensing in plain words), how to run on Ubuntu (Android) and on the Mac (Apple), how to update a version, how to verify.
2. **Android integration**: change `apps/native/android/portico-mpv/build.gradle` so it consumes our build output instead of downloading media-kit (keep a `-PporticoLibmpvDir`-style override; fail with a clear message telling the developer to run the build script if outputs are missing). Package all the `.so` files. Update the JNI bridge / Kotlin only as far as needed (e.g. loading order, `System.loadLibrary` for dependencies if required).
3. **Apple integration**: replace the MPVKit package reference in both Xcode projects with a **local** Swift package (or framework references) that wraps our xcframeworks, keeping the module name the Swift engine imports (`Libmpv` — check) so `#if canImport(Libmpv)` code is unchanged. You cannot compile it here: keep the change minimal and reviewable, and describe exactly what to run on the Mac. The first step of the Apple playbook becomes "run `build-apple.sh`" (like `pod install`).
4. **Certificate trust on the mpv path** (this matters with OpenSSL):
   - Find out what the engines do today: mpv's `tls-verify` defaults to **no**; check whether our engines set `tls-verify`, `tls-ca-file`, etc. on Android and Apple, and how the native app handles owner/self-signed certificates for its other requests (the owner's own-certificate decision, D-07 in the audit; look in the native networking code and `PROGRAM-BRIEF.md`).
   - Make mpv verify certificates by default, consistently with how the rest of the native app trusts the server (including owner-trusted certificates). Android: OpenSSL can read the system store (`/system/etc/security/cacerts` is in OpenSSL hashed format; consider `SSL_CERT_DIR` or FFmpeg's `ca_file`; also user-added CAs if the app's network security config trusts them). Apple: OpenSSL cannot read Apple's trust store — supply a pinned root bundle (e.g. Mozilla's via curl's `cacert.pem`, record its licence and an update step) plus owner-trusted certificates through mpv options. Plain HTTP must keep working (Justin's decision: HTTP anywhere).
   - Add tests at the JS/Kotlin level where feasible; document device test cases (public cert OK, owner-trusted self-signed OK, untrusted refused with a clear error).
5. **Build and verify Android for real here**: run `build-android.sh` for all four ABIs, run `verify.py`, then build the Android module/app as far as this host allows (`./gradlew :portico-mpv:assembleRelease` at least; the app `assembleDebug` if it works in reasonable time). If an Android emulator/system image happens to be installed, try loading libmpv and playing a short generated file; if not, say so.
6. **Native tests**: native `npm test` (660) and Vega (87) still pass; native type-checks clean.
7. **Docs and notices**: update `apps/native/README.md` mpv sections (both platforms), the native third-party notices, and write a short **Apple playbook section** (numbered steps for the Mac: prerequisites, `build-apple.sh`, verify, open Xcode, what to test first) in the package README. Also put that playbook text in your report so the orchestrator can add it to the internal Apple test plan.

## Report

Write `_program/reports/own-mpv.md` with: outcome (plain words first), components table (version, licence, why needed), what was dropped and why, decisions you made, Android build/verify results with exact commands and output excerpts, sizes (per ABI, compared with media-kit's libmpv.so), what is untested (Apple build, devices), the Apple playbook, certificate-trust behaviour before/after, and follow-ups. Keep it factual: say what you ran and what you did not.
