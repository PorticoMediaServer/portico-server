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

The Portico application release downloads that exact component tag. It never
uses an unpinned FFmpeg found on a runner. Windows ARM64 is intentionally a
limited profile: it must provide the core decode/filter surface, but hardware
transcoding parity with x64 is not a release requirement.

FFprobe is built and shipped beside FFmpeg. It is part of the same upstream
project and license configuration; including it does not change Portico's
license or remove functionality.
