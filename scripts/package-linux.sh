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
export PORTICO_PACKAGE_VERSION="$VERSION" PORTICO_PACKAGE_ARCH="$PACKAGE_ARCH" PORTICO_PACKAGE_ROOT="$STAGE"
nfpm package --config "$ROOT/packaging/linux/nfpm.yaml" --packager deb --target "$OUT/portico-media-server-linux-${FILE_ARCH}.deb"
nfpm package --config "$ROOT/packaging/linux/nfpm.yaml" --packager rpm --target "$OUT/portico-media-server-linux-${FILE_ARCH}.rpm"
