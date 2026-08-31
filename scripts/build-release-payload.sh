#!/usr/bin/env bash
set -euo pipefail
export COPYFILE_DISABLE=1

reject_macos_metadata() {
  local root="$1"
  local found
  found="$(find "$root" -type f \( -name '._*' -o -name '.DS_Store' \) -print -quit)"
  if [[ -n "$found" ]]; then
    echo "release payload contains forbidden macOS metadata: $found" >&2
    exit 1
  fi
}

if [[ $# -ne 6 ]]; then
  echo "usage: $0 VERSION GOOS GOARCH FFMPEG_ROOT OUTPUT_ROOT BUILD_NUMBER" >&2
  exit 2
fi

VERSION="$1"
TARGET_OS="$2"
TARGET_ARCH="$3"
FFMPEG_ROOT="$4"
OUTPUT_ROOT="$5"
BUILD_NUMBER="$6"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

case "$TARGET_OS" in
  windows) executable="portico-media-server.exe"; ffmpeg_name="ffmpeg.exe"; ffprobe_name="ffprobe.exe" ;;
  linux|darwin) executable="portico-media-server"; ffmpeg_name="ffmpeg"; ffprobe_name="ffprobe" ;;
  *) echo "unsupported target OS: $TARGET_OS" >&2; exit 2 ;;
esac

for required in "$FFMPEG_ROOT/$ffmpeg_name" "$FFMPEG_ROOT/$ffprobe_name"; do
  [[ -f "$required" ]] || { echo "missing qualified media tool: $required" >&2; exit 1; }
done

COMMIT="${PORTICO_BUILD_SOURCE_REVISION:-}"
if [[ -z "$COMMIT" ]]; then
  if [[ -n "$(git -C "$ROOT" status --porcelain=v1 --untracked-files=normal --ignore-submodules=none)" ]]; then
    echo "dirty Server source requires reviewed PORTICO_BUILD_SOURCE_REVISION" >&2
    exit 1
  fi
  COMMIT="$(git -C "$ROOT" rev-parse HEAD)"
else
  [[ "$COMMIT" =~ ^[a-f0-9]{64}$ ]] || {
    echo "reviewed dirty-source revision must be a 64-character SHA-256 digest" >&2
    exit 1
  }
  actual_source_revision="$("$ROOT/scripts/source-tree-revision.py" "$ROOT")"
  [[ "$COMMIT" == "$actual_source_revision" ]] || {
    echo "reviewed Server source revision does not match the build context" >&2
    exit 1
  }
fi
[[ "$COMMIT" =~ ^([a-f0-9]{40}|[a-f0-9]{64})$ ]] || {
  echo "invalid Server source revision" >&2
  exit 1
}
BUILT_AT="${PORTICO_BUILD_TIMESTAMP:-$(git -C "$ROOT" show -s --format=%cI HEAD)}"
[[ "$BUILT_AT" =~ ^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$ ]] || {
  echo "invalid Server build timestamp" >&2
  exit 1
}
rm -rf "$OUTPUT_ROOT"
mkdir -p "$OUTPUT_ROOT/bin" "$OUTPUT_ROOT/web" "$OUTPUT_ROOT/licenses"

GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" CGO_ENABLED=0 go build -C "$ROOT" -trimpath \
  -ldflags="-s -w -X main.version=$VERSION -X main.buildNumber=$BUILD_NUMBER -X main.channel=stable -X main.commit=$COMMIT -X main.builtAt=$BUILT_AT -X main.releaseSafetyClass=protected" \
  -o "$OUTPUT_ROOT/$executable" ./cmd/porticod

if [[ "$TARGET_OS" == "windows" || "$TARGET_OS" == "linux" ]]; then
  companion="portico-desktop"
  companion_ldflags="-s -w -X main.version=$VERSION -X main.buildNumber=$BUILD_NUMBER"
  if [[ "$TARGET_OS" == "windows" ]]; then
    companion="portico-desktop.exe"
    companion_ldflags="$companion_ldflags -H=windowsgui"
  fi
  GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" CGO_ENABLED=0 go build -C "$ROOT" -trimpath \
    -ldflags="$companion_ldflags" -o "$OUTPUT_ROOT/$companion" ./cmd/portico-desktop
fi

cp "$FFMPEG_ROOT/$ffmpeg_name" "$OUTPUT_ROOT/bin/$ffmpeg_name"
cp "$FFMPEG_ROOT/$ffprobe_name" "$OUTPUT_ROOT/bin/$ffprobe_name"
chmod +x "$OUTPUT_ROOT/$executable" "$OUTPUT_ROOT/bin/$ffmpeg_name" "$OUTPUT_ROOT/bin/$ffprobe_name" 2>/dev/null || true
cp -R "$ROOT/web/dist/." "$OUTPUT_ROOT/web/"
cp "$ROOT/LICENSE" "$OUTPUT_ROOT/LICENSE"
cp "$ROOT/THIRD-PARTY-NOTICES.md" "$OUTPUT_ROOT/THIRD-PARTY-NOTICES.md"
cp "$ROOT/media-toolchain/NOTICE.md" "$OUTPUT_ROOT/licenses/FFMPEG-NOTICE.md"
if [[ -d "$FFMPEG_ROOT/LICENSES" ]]; then cp -R "$FFMPEG_ROOT/LICENSES/." "$OUTPUT_ROOT/licenses/"; fi
if [[ -f "$FFMPEG_ROOT/build-info.json" ]]; then cp "$FFMPEG_ROOT/build-info.json" "$OUTPUT_ROOT/licenses/ffmpeg-build-info.json"; fi

# Finder provenance and quarantine metadata are host state, never release
# identity. Strip them before both the manifest and archive boundary.
if command -v xattr >/dev/null 2>&1; then
  xattr -cr "$OUTPUT_ROOT"
fi

cat > "$OUTPUT_ROOT/release.json" <<EOF
{
  "schemaVersion": 1,
  "product": "Portico Media Server",
  "version": "$VERSION",
  "buildNumber": "$BUILD_NUMBER",
  "commit": "$COMMIT",
  "builtAt": "$BUILT_AT",
  "platform": "$TARGET_OS-$TARGET_ARCH",
  "signed": false,
  "channel": "stable",
  "releaseSafetyClass": "protected"
}
EOF

reject_macos_metadata "$OUTPUT_ROOT"
