#!/usr/bin/env bash
# Sourced by the private pinned homebridge recipe. Build additions from source;
# never brew-install a replacement FFmpeg or a binary dependency bundle.
set -euo pipefail
portico_lock="${CWD}/portico-macos-extras.lock.json"
portico_clone() {
 local name="$1" repo revision
 repo=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))[sys.argv[2]]["repository"])' "$portico_lock" "$name")
 revision=$(python3 -c 'import json,sys;print(json.load(open(sys.argv[1]))[sys.argv[2]]["commit"])' "$portico_lock" "$name")
 if [ ! -d "$PACKAGES/portico-$name/.git" ]; then
  git init "$PACKAGES/portico-$name"
  git -C "$PACKAGES/portico-$name" remote add origin "$repo"
  git -C "$PACKAGES/portico-$name" fetch --depth=1 origin "$revision"
  git -C "$PACKAGES/portico-$name" checkout --detach FETCH_HEAD
 fi
 [ "$(git -C "$PACKAGES/portico-$name" rev-parse HEAD)" = "$revision" ]
 cd "$PACKAGES/portico-$name"
}
portico_cmake() {
 cmake -S . -B portico-build -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$WORKSPACE" -DCMAKE_INSTALL_LIBDIR=lib -DBUILD_SHARED_LIBS=OFF "$@"
 cmake --build portico-build --parallel "${PORTICO_BUILD_JOBS:-4}"
 cmake --install portico-build
}
portico_meson() {
 meson setup portico-build --prefix="$WORKSPACE" --libdir=lib --buildtype=release --default-library=static "$@"
 ninja -C portico-build -j "${PORTICO_BUILD_JOBS:-4}"
 ninja -C portico-build install
}
export PKG_CONFIG_PATH="$WORKSPACE/lib/pkgconfig:${PKG_CONFIG_PATH:-}"
portico_clone soxr
portico_cmake -DWITH_OPENMP=OFF -DBUILD_TESTS=OFF -DBUILD_EXAMPLES=OFF -DCMAKE_POLICY_VERSION_MINIMUM=3.5
portico_clone fontconfig
portico_meson -Dtests=disabled -Dtools=disabled -Ddoc=disabled -Dxml-backend=libxml2
portico_clone dvdread
portico_meson -Ddefault_library=static -Dlibdvdcss=disabled
portico_clone dvdnav
portico_meson -Ddefault_library=static
portico_clone udfread
portico_meson -Denable_examples=false
portico_clone bluray
portico_meson -Denable_docs=false -Denable_tools=false -Denable_examples=false -Dbdj_jar=disabled -Dfontconfig=enabled -Dfreetype=enabled -Dlibxml2=disabled -Dlibudfread=enabled
portico_clone vulkanHeaders
portico_cmake -DVULKAN_HEADERS_ENABLE_TESTS=OFF
portico_clone vulkanLoader
portico_cmake -DBUILD_TESTS=OFF -DBUILD_WSI_XCB_SUPPORT=OFF -DBUILD_WSI_XLIB_SUPPORT=OFF -DBUILD_WSI_WAYLAND_SUPPORT=OFF -DUPDATE_DEPS=OFF -DBUILD_SHARED_LIBS=ON
portico_clone moltenVK
git submodule update --init --recursive
./fetchDependencies --macos
make -j "${PORTICO_BUILD_JOBS:-4}" macos
mkdir -p "$WORKSPACE/lib" "$WORKSPACE/share/vulkan/icd.d"
cp Package/Release/MoltenVK/dylib/macOS/libMoltenVK.dylib "$WORKSPACE/lib/"
cp Package/Release/MoltenVK/dylib/macOS/MoltenVK_icd.json "$WORKSPACE/share/vulkan/icd.d/"
portico_clone shaderc
python3 utils/git-sync-deps
portico_cmake -DSHADERC_SKIP_TESTS=ON -DSHADERC_SKIP_EXAMPLES=ON -DSHADERC_SKIP_COPYRIGHT_CHECK=ON
portico_clone placebo
git submodule update --init --recursive
portico_meson -Dvulkan=enabled -Dshaderc=enabled -Dglslang=disabled -Ddemos=false -Dtests=false -Dbench=false
# Every collected native source and build command is archived with the bundle.
