#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "usage: $0 FFMPEG_ROOT [full|limited]" >&2
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
demuxers="$($FFMPEG -hide_banner -demuxers 2>&1)"
muxers="$($FFMPEG -hide_banner -muxers 2>&1)"
protocols="$($FFMPEG -hide_banner -protocols 2>&1)"

require_entry() {
  local inventory="$1"
  local entry="$2"
  local kind="$3"
  grep -Eq "[[:space:],]${entry}([[:space:],]|$)" <<<"$inventory" || {
    echo "missing ${kind}: ${entry}" >&2
    exit 1
  }
}

grep -q -- '--enable-gpl' <<<"$buildconf"
grep -q -- '--enable-version3' <<<"$buildconf"
if grep -q -- '--enable-nonfree' <<<"$buildconf"; then echo "nonfree FFmpeg build is prohibited" >&2; exit 1; fi
if grep -q -- '--enable-libfdk-aac' <<<"$buildconf"; then echo "libfdk-aac is prohibited by the redistributable build policy" >&2; exit 1; fi
license_one_line="$(tr '\n' ' ' <<<"$license")"
grep -Eqi 'GPL version 3|GPLv3|GNU General Public License.*version 3' <<<"$license_one_line"
for decoder in h264 hevc av1 vp9 mpeg2video vc1 flv aac ac3 eac3 truehd dca flac opus pcm_s16le alac vorbis subrip webvtt ass pgssub dvdsub; do require_entry "$decoders" "$decoder" decoder; done
for filter in scale format zscale tonemap bwdif yadif subtitles overlay hwupload hwdownload aresample pan loudnorm; do require_entry "$filters" "$filter" filter; done
for demuxer in mov matroska mpegts hls flv vobsub; do require_entry "$demuxers" "$demuxer" demuxer; done
for muxer in hls mp4 mpegts; do require_entry "$muxers" "$muxer" muxer; done
for protocol in file http https tcp udp; do require_entry "$protocols" "$protocol" protocol; done
for library in libass libfreetype libfribidi libharfbuzz; do grep -q -- "--enable-${library}" <<<"$buildconf" || { echo "missing configured library: $library" >&2; exit 1; }; done
if [[ "$PROFILE" == "full" ]]; then
  for encoder in libx264 libx265; do require_entry "$encoders" "$encoder" encoder; done
fi
$FFPROBE -hide_banner -version >/dev/null
printf '%s\n' "$version"
