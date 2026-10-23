#!/usr/bin/env bash
set -euo pipefail
[[ $# -ge 1 && $# -le 2 ]] || { echo 'usage: verify-ffmpeg-bundle.sh BUNDLE [full|limited]' >&2; exit 2; }
# limited is retained for existing callers; target requirements always apply.
[[ ${2:-full} == full || ${2:-full} == limited ]] || exit 2
python3 "$(dirname "$0")/verify-ffmpeg-manifest.py" "$1"
python3 "$(dirname "$0")/verify-ffmpeg-features.py" "$1" --report "$1/../feature-report-$(basename "$1").json"
