# Portico FFmpeg toolchain

Portico ships FFmpeg and FFprobe with every server package so users do not
need to install or troubleshoot them separately. These binaries are component
artifacts, built only when Justin explicitly approves a stable FFmpeg version.
They do not automatically change during an ordinary Portico release.

The pinned version, upstream recipes, and Portico revision are recorded in
`sources.lock.json`. The manual **Qualified FFmpeg toolchain** GitHub workflow:

1. verifies that its requested version exactly matches the lock;
2. builds Linux x64/ARM64 and Windows x64/ARM64 with the pinned BtbN recipe;
3. builds Apple Silicon macOS with the pinned Homebridge recipe and Portico's
   GPL-only patch, including the pinned FreeType, FriBidi, HarfBuzz, and libass
   stack required for server-side subtitle rendering;
4. rejects `--enable-nonfree` and `--enable-libfdk-aac`, requires GPL version 3,
   and checks FFprobe plus the codec, subtitle, HDR, container, protocol, and
   playback/transcoding features in `scripts/verify-ffmpeg-bundle.sh`;
5. publishes binaries, license files, build identity, and corresponding source
   archives in a versioned FFmpeg prerelease.

Dependency compilation is cached durably because approved versions normally
remain pinned for a long time. Linux and Windows use target-specific builder
images in Portico's GitHub Container Registry; macOS uses a checksum-verified
dependency-prefix archive held in a non-public draft release. Cache keys cover
the immutable upstream recipe, Portico patches, target, FFmpeg release line,
and the explicit dependency-cache revision in `sources.lock.json`. A patch or
recipe change naturally produces a new cache key; increment the cache revision
when a clean dependency refresh is required without either changing. The
workflow still rebuilds FFmpeg itself and runs the complete qualification
checks every time. Its **force dependency rebuild** input bypasses every cache
for a clean-room security or upgrade run.

The Portico application release downloads that exact component tag. It never
uses an unpinned FFmpeg found on a runner. Windows ARM64 is intentionally a
limited profile: it must provide the core decode/filter surface, but hardware
transcoding parity with x64 is not a release requirement.

FFmpeg 8.1.2 also requires a narrowly scoped Windows ARM64 source correction:
its Graphics Capture filter uses `std::system_error` without directly including
the standard `<system_error>` header. The pinned BtbN recipe patch adds that
header; it does not disable the filter or change Portico's licensing profile.

FFprobe is built and shipped beside FFmpeg. It is part of the same upstream
project and license configuration; including it does not change Portico's
license or remove functionality.
