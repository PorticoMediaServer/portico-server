#!/usr/bin/env bash
set -euo pipefail
if [[ $# -ne 5 ]]; then echo "usage: $0 VERSION GOARCH FILE_ARCH FFMPEG_ROOT BUILD_NUMBER" >&2; exit 2; fi
VERSION="$1"; GOARCH_VALUE="$2"; FILE_ARCH="$3"; FFMPEG_ROOT="$4"; BUILD_NUMBER="$5"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STAGE="$ROOT/dist/stage-windows-$GOARCH_VALUE"
OUT="$ROOT/dist"
"$ROOT/scripts/build-release-payload.sh" "$VERSION" windows "$GOARCH_VALUE" "$FFMPEG_ROOT" "$STAGE" "$BUILD_NUMBER"
STAGE_WIN="$(cygpath -w "$STAGE")"
PORTABLE_WIN="$(cygpath -w "$OUT/Portico-Media-Server-Windows-${FILE_ARCH}-Portable.zip")"
SETUP_WIN="$(cygpath -w "$OUT/Portico-Media-Server-Windows-${FILE_ARCH}-Setup.exe")"
INSTALLER_WIN="$(cygpath -w "$ROOT/packaging/windows/installer.nsi")"
export PORTICO_STAGE_WIN="$STAGE_WIN" PORTICO_PORTABLE_WIN="$PORTABLE_WIN"
powershell.exe -NoLogo -NoProfile -Command \
  '$ErrorActionPreference = "Stop"; Compress-Archive -Path (Join-Path $env:PORTICO_STAGE_WIN "*") -DestinationPath $env:PORTICO_PORTABLE_WIN -Force'
makensis.exe "/DOUTPUT_FILE=$SETUP_WIN" "/DSTAGE_DIR=$STAGE_WIN" "/DPRODUCT_VERSION=$VERSION" "$INSTALLER_WIN"
