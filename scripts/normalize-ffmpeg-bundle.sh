#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 4 ]]; then
  echo "usage: $0 EXTRACTED_ROOT TARGET OUTPUT_ROOT SOURCE_LABEL" >&2
  exit 2
fi

INPUT_ROOT="$1"
TARGET="$2"
OUTPUT_ROOT="$3"
SOURCE_LABEL="$4"
case "$TARGET" in
  windows-*) ffmpeg_name="ffmpeg.exe"; ffprobe_name="ffprobe.exe" ;;
  *) ffmpeg_name="ffmpeg"; ffprobe_name="ffprobe" ;;
esac

ffmpeg_path="$(find "$INPUT_ROOT" -type f -name "$ffmpeg_name" -perm -u+x -print -quit 2>/dev/null || true)"
ffprobe_path="$(find "$INPUT_ROOT" -type f -name "$ffprobe_name" -perm -u+x -print -quit 2>/dev/null || true)"
if [[ -z "$ffmpeg_path" ]]; then ffmpeg_path="$(find "$INPUT_ROOT" -type f -name "$ffmpeg_name" -print -quit)"; fi
if [[ -z "$ffprobe_path" ]]; then ffprobe_path="$(find "$INPUT_ROOT" -type f -name "$ffprobe_name" -print -quit)"; fi
[[ -n "$ffmpeg_path" && -n "$ffprobe_path" ]] || { echo "FFmpeg executables were not found in $INPUT_ROOT" >&2; exit 1; }

rm -rf "$OUTPUT_ROOT"
mkdir -p "$OUTPUT_ROOT/LICENSES"
cp "$ffmpeg_path" "$OUTPUT_ROOT/$ffmpeg_name"
cp "$ffprobe_path" "$OUTPUT_ROOT/$ffprobe_name"
chmod +x "$OUTPUT_ROOT/$ffmpeg_name" "$OUTPUT_ROOT/$ffprobe_name" 2>/dev/null || true

while IFS= read -r license; do
  name="$(basename "$license")"
  cp "$license" "$OUTPUT_ROOT/LICENSES/${name}"
done < <(find "$INPUT_ROOT" -type f \( -iname 'license*' -o -iname 'copying*' \) -print)

ffmpeg_sha="$(shasum -a 256 "$OUTPUT_ROOT/$ffmpeg_name" | awk '{print $1}')"
ffprobe_sha="$(shasum -a 256 "$OUTPUT_ROOT/$ffprobe_name" | awk '{print $1}')"
cat > "$OUTPUT_ROOT/build-info.json" <<EOF
{
  "schemaVersion": 1,
  "target": "$TARGET",
  "sourceRecipe": "$SOURCE_LABEL",
  "ffmpegSha256": "$ffmpeg_sha",
  "ffprobeSha256": "$ffprobe_sha",
  "licensePolicy": "GPL-3.0-compatible; nonfree prohibited"
}
EOF
