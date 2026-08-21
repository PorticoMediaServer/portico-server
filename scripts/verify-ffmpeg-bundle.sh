#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "usage: $0 FFMPEG_ROOT [limited]" >&2
  exit 2
fi

ROOT="$1"
PROFILE="${2:-full}"
if [[ -f "$ROOT/ffmpeg.exe" ]]; then FFMPEG="$ROOT/ffmpeg.exe"; FFPROBE="$ROOT/ffprobe.exe"; else FFMPEG="$ROOT/ffmpeg"; FFPROBE="$ROOT/ffprobe"; fi
[[ -x "$FFMPEG" || -f "$FFMPEG" ]] || { echo "missing ffmpeg" >&2; exit 1; }
[[ -x "$FFPROBE" || -f "$FFPROBE" ]] || { echo "missing ffprobe" >&2; exit 1; }

version="$($FFMPEG -hide_banner -version 2>&1)"
buildconf="$($FFMPEG -hide_banner -buildconf 2>&1)"
license="$($FFMPEG -hide_banner -L 2>&1)"
decoders="$($FFMPEG -hide_banner -decoders 2>&1)"
encoders="$($FFMPEG -hide_banner -encoders 2>&1)"
filters="$($FFMPEG -hide_banner -filters 2>&1)"

grep -q -- '--enable-gpl' <<<"$buildconf"
grep -q -- '--enable-version3' <<<"$buildconf"
if grep -q -- '--enable-nonfree' <<<"$buildconf"; then echo "nonfree FFmpeg build is prohibited" >&2; exit 1; fi
license_one_line="$(tr '\n' ' ' <<<"$license")"
grep -Eqi 'GPL version 3|GPLv3|GNU General Public License.*version 3' <<<"$license_one_line"
for decoder in h264 hevc av1 vp9; do grep -Eq "[[:space:]]${decoder}[[:space:]]" <<<"$decoders" || { echo "missing decoder: $decoder" >&2; exit 1; }; done
for filter in scale tonemap; do grep -Eq "[[:space:]]${filter}[[:space:]]" <<<"$filters" || { echo "missing filter: $filter" >&2; exit 1; }; done
if [[ "$PROFILE" == "full" ]]; then
  for encoder in libx264 libx265; do grep -Eq "[[:space:]]${encoder}[[:space:]]" <<<"$encoders" || { echo "missing encoder: $encoder" >&2; exit 1; }; done
fi
$FFPROBE -hide_banner -version >/dev/null
printf '%s\n' "$version"
