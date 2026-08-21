#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "usage: $0 TARGET [DESTINATION]" >&2
  exit 2
fi

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TARGET="$1"
DESTINATION="${2:-$ROOT/.toolchain/qualified-ffmpeg}"
REPOSITORY="${GITHUB_REPOSITORY:-PorticoMediaServer/portico-server}"
TAG="$(awk -F'"' '/"releaseTag"/ { print $4; exit }' "$ROOT/media-toolchain/sources.lock.json")"

[[ -n "$TAG" ]] || { echo "qualified FFmpeg release tag is missing" >&2; exit 1; }
command -v gh >/dev/null || { echo "GitHub CLI is required" >&2; exit 1; }

archive="portico-ffmpeg-${TAG}-${TARGET}.tar.xz"
mkdir -p "$DESTINATION"
gh release download "$TAG" --repo "$REPOSITORY" --pattern "$archive" --dir "$DESTINATION"
tar -xJf "$DESTINATION/$archive" -C "$DESTINATION"
rm "$DESTINATION/$archive"
"$ROOT/scripts/verify-ffmpeg-bundle.sh" "$DESTINATION" full

if [[ -n "${GITHUB_PATH:-}" ]]; then
  printf '%s\n' "$DESTINATION" >> "$GITHUB_PATH"
fi

printf '%s\n' "$DESTINATION"
