#!/usr/bin/env bash
set -euo pipefail
if [[ $# -ne 6 ]]; then echo "usage: $0 VERSION GOARCH PACKAGE_ARCH FILE_ARCH FFMPEG_ROOT BUILD_NUMBER" >&2; exit 2; fi
VERSION="$1"; GOARCH_VALUE="$2"; PACKAGE_ARCH="$3"; FILE_ARCH="$4"; FFMPEG_ROOT="$5"; BUILD_NUMBER="$6"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
STAGE="$ROOT/dist/stage-linux-$GOARCH_VALUE"
OUT="$ROOT/dist"
mkdir -p "$OUT"
"$ROOT/scripts/build-release-payload.sh" "$VERSION" linux "$GOARCH_VALUE" "$FFMPEG_ROOT" "$STAGE" "$BUILD_NUMBER"
tar --sort=name --mtime='UTC 2020-01-01' --owner=0 --group=0 --numeric-owner -C "$STAGE" -czf "$OUT/portico-media-server-linux-${FILE_ARCH}.tar.gz" .

escape_sed_replacement() {
  local value="$1"
  value="${value//\\/\\\\}"
  value="${value//&/\\&}"
  value="${value//|/\\|}"
  printf '%s' "$value"
}

PACKAGE_CONFIG="$OUT/nfpm-${GOARCH_VALUE}.yaml"
sed \
  -e "s|\${PORTICO_PACKAGE_VERSION}|$(escape_sed_replacement "$VERSION")|g" \
  -e "s|\${PORTICO_PACKAGE_ARCH}|$(escape_sed_replacement "$PACKAGE_ARCH")|g" \
  -e "s|\${PORTICO_PACKAGE_ROOT}|$(escape_sed_replacement "$STAGE")|g" \
  "$ROOT/packaging/linux/nfpm.yaml" > "$PACKAGE_CONFIG"

if grep -q '\${PORTICO_PACKAGE_' "$PACKAGE_CONFIG"; then
  echo "unresolved Portico package variable in $PACKAGE_CONFIG" >&2
  exit 1
fi

nfpm package --config "$PACKAGE_CONFIG" --packager deb --target "$OUT/portico-media-server-linux-${FILE_ARCH}.deb"
nfpm package --config "$PACKAGE_CONFIG" --packager rpm --target "$OUT/portico-media-server-linux-${FILE_ARCH}.rpm"
