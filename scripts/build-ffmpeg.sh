#!/usr/bin/env bash
set -euo pipefail
# Native macOS builds, or BtbN's container cross builds. Never publishes.
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TARGET="${1:?usage: build-ffmpeg.sh TARGET [WORK_DIRECTORY]}"
WORK="${2:-$ROOT/runtime/ffmpeg-build/$TARGET}"
mkdir -p "$WORK"
WORK="$(cd "$WORK" && pwd)"
readlock() { python3 -c 'import json,sys;v=json.load(open(sys.argv[1]));print(v[sys.argv[2]][sys.argv[3]])' "$ROOT/third_party/ffmpeg/sources.lock.json" "$1" "$2"; }
VERSION="$(readlock ffmpeg version)"
SOURCE="$WORK/ffmpeg-$VERSION.tar.xz"
curl --fail --location --retry 3 "$(readlock ffmpeg source)" -o "$SOURCE"
python3 - "$SOURCE" "$(readlock ffmpeg sha256)" <<'PY'
import hashlib,pathlib,sys
if hashlib.sha256(pathlib.Path(sys.argv[1]).read_bytes()).hexdigest()!=sys.argv[2]:raise SystemExit('FFmpeg source checksum mismatch')
PY
case "$TARGET" in
 macos-arm64|macos-x64)
  [[ "$(uname -s)" == Darwin ]]
  expected=arm64; [[ "$TARGET" == macos-x64 ]] && expected=x86_64
  [[ "$(uname -m)" == "$expected" ]] || { echo 'Use a native runner for this architecture' >&2;exit 1; }
  if [[ ! -d "$WORK/recipe/.git" && -d "$WORK/recipe/.git.bak" ]]; then mv "$WORK/recipe/.git.bak" "$WORK/recipe/.git"; fi
  [[ -d "$WORK/recipe/.git" ]] || git clone "$(readlock macosRecipe repository)" "$WORK/recipe"
  git -C "$WORK/recipe" checkout --detach "$(readlock macosRecipe commit)"
  for patch in homebridge-sourceforge-download.patch homebridge-gpl-only.patch homebridge-arm64-vpx.patch homebridge-private-python.patch homebridge-zix-build.patch homebridge-private-pkgconfig.patch; do
    if git -C "$WORK/recipe" apply --check "$ROOT/third_party/ffmpeg/patches/$patch"; then git -C "$WORK/recipe" apply "$ROOT/third_party/ffmpeg/patches/$patch"; else git -C "$WORK/recipe" apply --reverse --check "$ROOT/third_party/ffmpeg/patches/$patch"; fi
  done
  # Build tools and downloads are private to this job; never upgrade system Python.
  python3 -m venv "$WORK/python"
  "$WORK/python/bin/pip" install --disable-pip-version-check 'meson==1.7.0' 'ninja==1.11.1.4' 'setuptools==75.8.0'
  export PATH="$WORK/python/bin:$PATH"
  # Seed the exact verified source consumed by the pinned recipe.
  mkdir -p "$WORK/recipe/packages";cp "$SOURCE" "$WORK/recipe/packages/ffmpeg-$VERSION.tar.xz"
  (cd "$WORK/recipe" && FFMPEG_VERSION="$VERSION" SKIPINSTALL=yes SKIPRAV1E=yes ./build-ffmpeg --build --enable-gpl)
  INPUT="$WORK/recipe/workspace"
  mkdir -p "$INPUT/LICENSES"
  python3 - "$WORK/recipe/packages" "$INPUT/LICENSES" <<'PYLICENSE'
import pathlib,shutil,sys
for i,f in enumerate(sorted(pathlib.Path(sys.argv[1]).rglob("*"))):
 if f.is_file() and f.name.lower().startswith(("license","copying")):
  shutil.copy2(f,pathlib.Path(sys.argv[2])/(str(i)+"-"+f.name))
PYLICENSE
  ;;
 linux-x64|linux-arm64|windows-x64|windows-arm64)
  case "$TARGET" in linux-x64) native=linux64;;linux-arm64) native=linuxarm64;;windows-x64) native=win64;;windows-arm64) native=winarm64;;esac
  [[ -d "$WORK/recipe/.git" ]] || git clone "$(readlock btbn repository)" "$WORK/recipe"
  git -C "$WORK/recipe" checkout --detach "$(readlock btbn commit)"
  # These generated recipe files are owned by this build. Restore only the
  # patched files so a resumed job cannot accumulate overlapping edits.
  git -C "$WORK/recipe" restore --source="$(readlock btbn commit)" -- build.sh images/base-linux64/ct-ng-config images/base-linuxarm64/ct-ng-config
  for patch in btbn-gnu-ncurses-mirror.patch btbn-verified-source.patch; do
    if git -C "$WORK/recipe" apply --check "$ROOT/third_party/ffmpeg/patches/$patch"; then git -C "$WORK/recipe" apply "$ROOT/third_party/ffmpeg/patches/$patch"; else git -C "$WORK/recipe" apply --reverse --check "$ROOT/third_party/ffmpeg/patches/$patch"; fi
  done
  if ! grep -q 'std::system_error in the Windows Graphics Capture' "$WORK/recipe/build.sh"; then "$ROOT/scripts/patch-btbn-winarm64-system-error.sh" "$WORK/recipe/build.sh"; fi
  cp "$SOURCE" "$WORK/recipe/portico-ffmpeg-source.tar.xz"
  (cd "$WORK/recipe" && export GIT_BRANCH_OVERRIDE="n$VERSION" && ./makeimage.sh "$native" gpl 8.1 && ./build.sh "$native" gpl 8.1)
  mkdir -p "$WORK/extracted"
  python3 - "$WORK/recipe/artifacts" "$WORK/extracted" <<'PY'
import pathlib,shutil,sys
files=[p for p in pathlib.Path(sys.argv[1]).rglob('*') if p.name.endswith(('.tar.xz','.zip'))]
if len(files)!=1:raise SystemExit('Expected exactly one build artifact')
shutil.unpack_archive(str(files[0]),sys.argv[2])
PY
  INPUT="$WORK/extracted"
  ;;
 *) echo 'Unknown target' >&2;exit 2;;
esac
mkdir -p "$WORK/provenance"
mkdir -p "$WORK/provenance/contract" "$WORK/provenance/scripts"
cp "$ROOT/third_party/ffmpeg/"*.json "$ROOT/third_party/ffmpeg/NOTICE.md" "$WORK/provenance/contract/"
cp -R "$ROOT/third_party/ffmpeg/patches" "$WORK/provenance/contract/"
cp "$ROOT/scripts/build-ffmpeg.sh" "$ROOT/scripts/normalize-ffmpeg-bundle.py" "$ROOT/scripts/patch-btbn-winarm64-system-error.sh" "$WORK/provenance/scripts/"
cp "$SOURCE" "$WORK/provenance/"
# Include dependency sources and exact recipe, excluding only output binaries and git metadata.
tar -C "$WORK" --exclude=recipe/.git --exclude=recipe/workspace --exclude=recipe/artifacts --exclude=recipe/.cache/images -cJf "$WORK/corresponding-source.tar.xz" recipe provenance
python3 "$ROOT/scripts/normalize-ffmpeg-bundle.py" "$INPUT" "$TARGET" "$WORK/bundle" "$WORK/corresponding-source.tar.xz"
tar -C "$WORK/bundle" -cJf "$WORK/portico-ffmpeg-$TARGET.tar.xz" .
echo "$WORK/portico-ffmpeg-$TARGET.tar.xz"
