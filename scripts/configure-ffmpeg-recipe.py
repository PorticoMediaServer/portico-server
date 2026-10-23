#!/usr/bin/env python3
"""Apply Portico's target build requirements to pinned dependency recipes."""
from pathlib import Path
import json
import sys

recipe, source, target = Path(sys.argv[1]), Path(sys.argv[2]), sys.argv[3]
root = Path(__file__).resolve().parent.parent
common = '--enable-gpl --enable-version3 --enable-libdav1d --enable-libsvtav1 --enable-libx264 --enable-libx265 --enable-libzimg --enable-libass --enable-fontconfig --enable-libsoxr --enable-libbluray --enable-libdvdnav --enable-libdvdread --enable-libplacebo --enable-vulkan --disable-nonfree --disable-libfdk-aac'
flags = common
if target.startswith('linux'):
    flags += ' --enable-vaapi --enable-libdrm --enable-opencl'
if target in ('linux-x64', 'windows-x64', 'windows-arm64'):
    flags += ' --enable-libvpl --enable-ffnvcodec --enable-cuda-llvm --enable-nvenc --enable-nvdec --enable-amf'
if target.startswith('windows'):
    flags += ' --enable-d3d11va --enable-d3d12va --enable-opencl'
if target == 'linux-arm64':
    flags += ' --enable-rkmpp --enable-rkrga --enable-v4l2-m2m'
if target.startswith('macos'):
    flags += ' --enable-videotoolbox --enable-metal'
    # Extras are built from the pinned source recipes into this private prefix.
    build = recipe / 'build-ffmpeg'
    text = build.read_text()
    marker = 'build "ffmpeg" "${FFMPEG_VERSION}"'
    addition = 'source "${CWD}/portico-macos-extras.sh"\nCONFIGURE_OPTIONS+=(' + ' '.join('"' + x + '"' for x in flags.split()) + ')\n'
    if marker not in text:
        raise ValueError('macOS recipe changed')
    text = text.replace(marker, addition + marker, 1)
    build.write_text(text)
    import shutil
    shutil.copy2(root / 'scripts/build-macos-ffmpeg-extras.sh', recipe / 'portico-macos-extras.sh')
    shutil.copy2(root / 'third_party/ffmpeg/macos-extras.lock.json', recipe / 'portico-macos-extras.lock.json')
    sys.exit(0)

# BtbN disables Linux ARM VAAPI by default, although libva supports it.
path = recipe / 'scripts.d/50-vaapi/50-libva.sh'
text = path.read_text().replace('    [[ $TARGET == linuxarm64 ]] && return -1\n', '')
text = text.replace('elif [[ $TARGET == win* ]]; then', '''elif [[ $TARGET == linuxarm64 ]]; then
        myconf+=(--cross-file=/cross.meson --default-library=shared --sysconfdir="/etc"
                 -Ddriverdir="/usr/lib/aarch64-linux-gnu/dri" -Ddisable_drm=false
                 -Dwith_x11=yes -Dwith_glx=no -Dwith_wayland=no)
    elif [[ $TARGET == win* ]]; then''')
path.write_text(text)
# The ARM Windows build must attempt these headers/dispatchers and pass the
# feature verifier; a dependency failure is a build failure, never a downgrade.
for name in ['50-onevpl.sh', '50-ffnvcodec.sh']:
    path = recipe / 'scripts.d' / name
    text = path.read_text()
    text = text.replace('    [[ $TARGET == *arm64 ]] && return -1', '    [[ $TARGET == linuxarm64 ]] && return -1')
    text = text.replace('    [[ $TARGET == winarm64 ]] && (( $(ffbuild_ffver) <= 801 )) && return -1\n', '')
    path.write_text(text)
# Reuse the exact Rockchip dependency commits in the checked Jellyfin source.
for name, directory in [('50-rkmpp.sh', 'rkmpp'), ('50-rkrga.sh', 'rkrga')]:
    text = (source / 'builder/scripts.d' / name).read_text()
    start = text.index('    git-mini-clone ')
    end = text.index('\n', text.index('    cd ' + directory, start))
    text = text[:start] + text[end:]
    # BtbN stages sources before invoking ffbuild_dockerbuild, and installs into
    # a stage DESTDIR which is copied into the final static dependency prefix.
    text = text.replace('ninja install', 'DESTDIR="$FFBUILD_DESTDIR" ninja install')
    text = text.replace('>> "$FFBUILD_PREFIX"/lib/pkgconfig/', '>> "$FFBUILD_DESTPREFIX"/lib/pkgconfig/')
    (recipe / 'scripts.d' / name).write_text(text)
# Final command-line flags override recipe defaults without hiding omissions.
path = recipe / 'build.sh'
text = path.read_text()
needle = '--extra-version="\\$(date +%Y%m%d)"'
if needle not in text:
    raise ValueError('BtbN configure invocation changed')
text = text.replace(needle, '--extra-version="Portico-Jellyfin" ' + flags)
text = text.replace('make -j\\$(nproc) V=1', 'make -j' + str(max(1, int(__import__('os').environ.get('PORTICO_BUILD_JOBS', '4')))) + ' V=1')
path.write_text(text)
print('Configured source recipe for ' + target)
