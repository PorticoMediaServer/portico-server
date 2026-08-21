#!/usr/bin/env bash
set -euo pipefail
if [[ $# -ne 5 ]]; then echo "usage: $0 VERSION GOARCH FILE_ARCH FFMPEG_ROOT BUILD_NUMBER" >&2; exit 2; fi
VERSION="$1"; GOARCH_VALUE="$2"; FILE_ARCH="$3"; FFMPEG_ROOT="$4"; BUILD_NUMBER="$5"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STAGE="$ROOT/dist/stage-windows-$GOARCH_VALUE"
OUT="$ROOT/dist"
"$ROOT/scripts/build-release-payload.sh" "$VERSION" windows "$GOARCH_VALUE" "$FFMPEG_ROOT" "$STAGE" "$BUILD_NUMBER"
powershell.exe -NoLogo -NoProfile -Command "Compress-Archive -Path '$STAGE\\*' -DestinationPath '$OUT\\Portico-Media-Server-Windows-${FILE_ARCH}-Portable.zip' -Force"
makensis.exe /DOUTPUT_FILE="$OUT\\Portico-Media-Server-Windows-${FILE_ARCH}-Setup.exe" /DSTAGE_DIR="$STAGE" /DPRODUCT_VERSION="$VERSION" "$ROOT/packaging/windows/installer.nsi"
