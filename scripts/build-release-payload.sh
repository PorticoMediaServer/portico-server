#!/usr/bin/env bash
set -euo pipefail

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

COMMIT="$(git -C "$ROOT" rev-parse HEAD)"
BUILT_AT="$(git -C "$ROOT" show -s --format=%cI "$COMMIT")"
rm -rf "$OUTPUT_ROOT"
mkdir -p "$OUTPUT_ROOT/bin" "$OUTPUT_ROOT/web" "$OUTPUT_ROOT/licenses"

GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" CGO_ENABLED=0 go build -C "$ROOT" -trimpath \
  -ldflags="-s -w -X main.version=$VERSION -X main.buildNumber=$BUILD_NUMBER -X main.channel=stable -X main.commit=$COMMIT -X main.builtAt=$BUILT_AT -X main.releaseSafetyClass=protected" \
  -o "$OUTPUT_ROOT/$executable" ./cmd/porticod

cp "$FFMPEG_ROOT/$ffmpeg_name" "$OUTPUT_ROOT/bin/$ffmpeg_name"
cp "$FFMPEG_ROOT/$ffprobe_name" "$OUTPUT_ROOT/bin/$ffprobe_name"
chmod +x "$OUTPUT_ROOT/$executable" "$OUTPUT_ROOT/bin/$ffmpeg_name" "$OUTPUT_ROOT/bin/$ffprobe_name" 2>/dev/null || true
cp -R "$ROOT/web/dist/." "$OUTPUT_ROOT/web/"
cp "$ROOT/LICENSE" "$OUTPUT_ROOT/LICENSE"
cp "$ROOT/THIRD-PARTY-NOTICES.md" "$OUTPUT_ROOT/THIRD-PARTY-NOTICES.md"
cp "$ROOT/media-toolchain/NOTICE.md" "$OUTPUT_ROOT/licenses/FFMPEG-NOTICE.md"
if [[ -d "$FFMPEG_ROOT/LICENSES" ]]; then cp -R "$FFMPEG_ROOT/LICENSES/." "$OUTPUT_ROOT/licenses/"; fi
if [[ -f "$FFMPEG_ROOT/build-info.json" ]]; then cp "$FFMPEG_ROOT/build-info.json" "$OUTPUT_ROOT/licenses/ffmpeg-build-info.json"; fi

cat > "$OUTPUT_ROOT/release.json" <<EOF
{
  "schemaVersion": 1,
  "product": "Portico Media Server",
  "version": "$VERSION",
  "commit": "$COMMIT",
  "builtAt": "$BUILT_AT",
  "platform": "$TARGET_OS-$TARGET_ARCH",
  "signed": false
}
EOF
