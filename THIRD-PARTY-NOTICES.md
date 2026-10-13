# Third-party notices

Portico Media Server is GPL-3.0-or-later. Components below retain their own
copyrights and licences. Licence names here are an inventory, not replacement
licence texts. Preserve the upstream notices and corresponding source/build
materials when distributing a binary. Versioned source files and lockfiles,
not a moving upstream default branch, identify an update.

## Go server

`server/go.mod` and `server/go.sum` pin the modules. `apikit` is Portico's own
module. Go itself and copied Go image-scaling code use BSD-3-Clause terms.

| Module | Version | Licence |
| --- | --- | --- |
| golang.org/x/crypto | v0.56.0 | BSD-3-Clause |
| golang.org/x/image | v0.46.0 | BSD-3-Clause |
| golang.org/x/sys | v0.48.0 | BSD-3-Clause |
| golang.org/x/text | v0.42.0 | BSD-3-Clause |
| modernc.org/sqlite | v1.58.0 | BSD-3-Clause; SQLite engine public domain |
| modernc.org/libc | v1.75.6 | BSD-3-Clause; generated/ported code retains its source notices |
| modernc.org/mathutil | v1.7.1 | BSD-3-Clause |
| modernc.org/memory | v1.12.1 | BSD-3-Clause |
| github.com/dustin/go-humanize | v1.0.1 | MIT |
| github.com/google/uuid | v1.6.0 | BSD-3-Clause |
| github.com/mattn/go-isatty | v0.0.24 | MIT |
| github.com/ncruces/go-strftime | v1.0.0 | MIT |
| github.com/remyoudompheng/bigfft | v0.0.0-20230129092748-24d4a6f8daec | BSD-3-Clause |
| gopkg.in/yaml.v3 | v3.0.1 | MIT and Apache-2.0, per file |

The maintained SQLite driver's copied files retain upstream notices in
[server/internal/thirdparty/sqlite/LICENSE](server/internal/thirdparty/sqlite/LICENSE)
and provenance in [PORTICO.md](server/internal/thirdparty/sqlite/PORTICO.md).
The image scaler's BSD notice is
[server/internal/imagework/SCALE_LICENSE](server/internal/imagework/SCALE_LICENSE).

## Browser and build packages

The [npm dependency inventory](docs/dependency-licenses.md) lists every package
version and declared licence in the root and standalone web lockfiles, including
transitive, development and platform-optional packages. React/React DOM,
TanStack Router, Radix UI, QRCode and Noble cryptography are MIT; hls.js is
Apache-2.0. Build dependencies include TypeScript (Apache-2.0), Vite (MIT), and
platform-dependent bundler binaries under their recorded licences.

The patched hls.js build is `1.7.2-portico.1`, based on upstream `1.7.2` at
`dd669ec53b601aa9fab7a2a78c913e25018cdf18`. Its archive, patch, build recipe,
[licence](third_party/hls.js/LICENSE) and
[provenance](third_party/hls.js/provenance.json) are retained in `third_party/hls.js`.

## Browser subtitle renderer and fonts

The unmodified `@jellyfin/libass-wasm` 4.2.4 archive and served files are in
`third_party/libass-wasm`. Source is
[JavascriptSubtitlesOctopus at the pinned revision](https://github.com/jellyfin/JavascriptSubtitlesOctopus/tree/2d4ec9b16a639f23733cd32982e09e0eb757f092).
The worker is WebAssembly; the JavaScript wrapper and worker glue are MIT.
The worker contains:

| Component | Licence |
| --- | --- |
| libass | ISC |
| FreeType | FTL or GPL-2.0-or-later; the FTL option is recorded in provenance |
| HarfBuzz | MIT-style |
| FriBidi | LGPL-2.1-or-later |
| Fontconfig | MIT-style and NTP |
| Expat, Brotli | MIT/Expat |
| zlib | Zlib |
| Small parts listed in COPYRIGHT | BSD-2-Clause, BSL-1.0, NTP and public domain |

Full per-file copyrights and licence texts are in
[COPYRIGHT](third_party/libass-wasm/COPYRIGHT), with wrapper
[LICENSE](third_party/libass-wasm/LICENSE) and font
[OFL-1.1.txt](third_party/libass-wasm/OFL-1.1.txt). The web build copies these
beside the served renderer. `python3 -I third_party/libass-wasm/verify.py` checks
its bytes against the stored archive. The corresponding source must include
its pinned library submodules and build recipe; an upstream top-level archive
alone is not a complete source bundle.

| Distributed font/art | Licence and local notice |
| --- | --- |
| Manrope web interface font | OFL-1.1, `web/src/ui/fonts/OFL.txt` |
| Liberation Sans 2.00.5 subtitle default | OFL-1.1, `third_party/libass-wasm/OFL-1.1.txt` |
| Noto Sans Arabic, Hebrew, Thai, Devanagari, Bengali, Tamil | OFL-1.1, `third_party/subtitle-fonts/OFL-1.1.txt` |
| Droid Sans Fallback, Chinese/Japanese glyphs | Apache-2.0, `third_party/subtitle-fonts/Apache-2.0.txt` |
| Decoder DejaVu fonts | Bitstream Vera/Arev font terms, DejaVu changes public domain; `server/internal/mediaexec/fonts/LICENSE` |
| Lucide icons | ISC, `packages/design/LICENSE-LUCIDE` |

Font file origins and digests are in each folder's provenance file. Droid Sans
Fallback does not supply Korean Hangul; the current browser fallback set is
not a claim of coverage for every script.

## Server FFmpeg and FFprobe

The server invokes these as separate processes. The qualified server build is
FFmpeg **8.1.2**, component release `ffmpeg-8.1.2-portico.1`, pinned in
[third_party/ffmpeg/sources.lock.json](third_party/ffmpeg/sources.lock.json).
The Linux/Windows recipe is BtbN revision
`48576f197ad1c2afb2e0b8efe204919a1afbff54`; macOS is the Homebridge recipe at
`4920fab9d6ac56999ce7e961b240659b4da078a5`, with the recorded Portico patches.
Configuration requires `--enable-gpl --enable-version3` and forbids `nonfree`
and libfdk-aac. The resulting FFmpeg build is GPL-3.0-or-later. Its compiled
codec/filter dependencies retain their individual notices.

The qualified bundle's `licenses` directory and corresponding source archive
are the inventory for the actual platform build, including external libraries
chosen by that recipe. Package creation carries this bundle intact. Consult
[third_party/ffmpeg/NOTICE.md](third_party/ffmpeg/NOTICE.md). This checkout has
recipes rather than the completed platform bundles; it cannot establish the
complete dependency list of a release artifact that has not been inspected.
Do not substitute a PATH binary and describe it as the qualified build.

## Native mpv builds (separate client repository)

These are fetched by the native clients, not embedded in the Go server. They
use different FFmpeg builds from the server's qualified toolchain.

**Apple:** both Xcode projects pin
[MPVKit 1.1.0-n9.0.2](https://github.com/mpvkit/MPVKit/tree/1.1.0-n9.0.2), selecting
`MPVKit`, not its separate `MPVKit-GPL` product. The package's own licence is
LGPL-3.0. Its recipe names mpv 0.41.0 and FFmpeg n9.0.2 with version-3 terms.
The selected product's framework dependencies are:

| Framework/library | Licence |
| --- | --- |
| libmpv | LGPL-2.1-or-later for the LGPL build |
| libavcodec, libavdevice, libavfilter, libavformat, libavutil, libswresample, libswscale | FFmpeg LGPL-3.0-or-later in this version-3, non-GPL build |
| OpenSSL libssl/libcrypto 3.3.5 | Apache-2.0 |
| libass | ISC |
| FreeType | FTL or GPL-2.0-or-later |
| FriBidi | LGPL-2.1-or-later |
| HarfBuzz, libunibreak | MIT-style / Zlib respectively |
| MoltenVK, shaderc and its SPIR-V/glslang parts | Apache-2.0, with BSD-3-Clause notices in glslang |
| Little CMS 2 | MIT |
| libplacebo | LGPL-2.1-or-later |
| libdovi | MIT |
| GMP | LGPL-3.0-or-later or GPL-2.0-or-later |
| Nettle/Hogweed | LGPL-3.0-or-later or GPL-2.0-or-later |
| GnuTLS | LGPL-2.1-or-later for the library; retain its per-file notices |
| dav1d, uavs3d | BSD-2-Clause / BSD-3-Clause respectively |
| uchardet | MPL-1.1 or GPL-2.0-or-later or LGPL-2.1-or-later |
| libbluray | LGPL-2.1-or-later |

The selected Apple product also links system frameworks/libraries, including
AVFoundation, VideoToolbox, AudioToolbox, Metal, zlib, bzip2, iconv, Expat and
libxml2; these are supplied by the operating system rather than redistributed
framework archives. The package declares a macOS-only LuaJIT dependency (MIT);
that dependency is not selected for the current iOS/tvOS apps. GPL libsmbclient
belongs to the unselected GPL product.

**Android/Android TV/Fire TV:**
[media-kit libmpv-android-video-build v1.1.11](https://github.com/media-kit/libmpv-android-video-build/releases/tag/v1.1.11),
`full` flavour. `apps/native/android/portico-mpv/build.gradle` pins SHA-256 for
`full-arm64-v8a.jar`, `full-armeabi-v7a.jar`, `full-x86.jar`, `full-x86_64.jar`
and takes only `libmpv.so` from each. The upstream full-flavour recipe disables
GPL/nonfree and enables FFmpeg version 3; its dependency declarations identify:

| Component | Licence |
| --- | --- |
| Build scripts | MIT for default/full |
| mpv (revision `78d43740f52db817d98bcf24fb30a76ab6fa13ff`) | LGPL-2.1-or-later for `gpl=false` |
| FFmpeg 6.0 | LGPL-3.0-or-later for this configuration |
| libass 0.17.1 | ISC |
| FreeType 2.13.0 | FTL or GPL-2.0-or-later |
| HarfBuzz 7.2.0 | MIT-style |
| FriBidi 1.0.12 | LGPL-2.1-or-later |
| dav1d 1.2.0 | BSD-2-Clause |
| mbedTLS 3.4.0 | Apache-2.0 or GPL-2.0-or-later |
| libxml2 2.10.3 | MIT |

The native pin has artifact hashes but no matching source commit for that release.
The recipe inventory above was checked against upstream source available at the
release date; it does not prove the archives were built from that exact commit.
Native distribution must retain full library notices and the matching source,
patches and build instructions; this server notice does not prove that a native
app's licence screen or source delivery is complete.

## Container and release tools

The Dockerfile fetches Debian packages for bubblewrap (LGPL-2.0-or-later), tini
(MIT), tzdata (public-domain/BSD notices), CA certificates (Mozilla certificates
plus Debian packaging notices), libva (MIT), Mesa VA drivers (MIT and per-file
permissive notices), and on amd64 Intel media/i965 drivers (MIT and their
bundled notices). Runtime Debian libraries and their transitive dependencies
retain notices under `/usr/share/doc/<package>/copyright`; the image SBOM is the
artifact inventory. Node build images include Node.js (MIT plus bundled notices)
and npm (Artistic-2.0 plus dependencies); Go build images carry Go (BSD-3-Clause).
They are build stages, not copied wholesale into the runtime image.

Release tools include Python cryptography 49.0.0 (Apache-2.0 or BSD-3-Clause,
with bundled dependencies), nFPM 2.43.4 (MIT), NSIS (zlib/libpng licence for core,
with component notices), Meson 1.7.0 (Apache-2.0), Ninja (Apache-2.0), and
setuptools (MIT). Compiler, OS packaging tools and GitHub workflow actions are
build infrastructure; their upstream distributions retain their notices.
Their use is not a claim that their code is bundled into the Portico server.

## External services

TMDB and TheTVDB application credentials identify Portico and are intentionally
distributed, not server-owner secrets. The UI carries provider attribution;
these services' data/API terms are separate from software licences. Owners can
supply their own server-wide credentials. MusicBrainz, AniList and configured
subtitle services have their own service/data terms.

The Cast receiver loads Google's hosted Cast Application Framework script at
runtime under Google's SDK terms. It is not copied into this repository and is
not GPL licensed by Portico. Service access and Apple store/distribution rights
must be checked separately from this software inventory.
