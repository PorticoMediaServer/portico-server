#!/usr/bin/env bash
# Reuse only archives with exact current source, recipe, patch and file hashes.
set -euo pipefail
[[ $# == 3 ]] || { echo "usage: $0 TARGET OUTPUT_DIR VALIDATION_RUN_OR_EMPTY" >&2; exit 2; }
TARGET="$1"; OUTPUT="$2"; PREVIOUS_RUN="$3"
case "$TARGET" in linux-x64|linux-arm64|windows-x64|windows-arm64|macos-arm64) ;; *) exit 2 ;; esac
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORK="$ROOT/.toolchain/$TARGET"
mkdir -p "$WORK" "$OUTPUT"
if [[ -n "$PREVIOUS_RUN" ]]; then
  [[ "$PREVIOUS_RUN" =~ ^[0-9]+$ ]]
  gh run download "$PREVIOUS_RUN" --name "validation-ffmpeg-$TARGET" --dir "$WORK"
else
  TAG="$(python3 -c "import json;print(json.load(open('third_party/ffmpeg/sources.lock.json'))['releaseTag'])")"
  gh release download "$TAG" --pattern "portico-ffmpeg-${TAG}-${TARGET}.tar.xz" --pattern checksums.sha256 --dir "$WORK"
  python3 - "$WORK" "portico-ffmpeg-${TAG}-${TARGET}.tar.xz" <<'DIGEST'
import hashlib,pathlib,sys
folder,name=pathlib.Path(sys.argv[1]),sys.argv[2]
entries=[line.split() for line in (folder/'checksums.sha256').read_text().splitlines()]
expected=[row[0] for row in entries if len(row)==2 and pathlib.PurePosixPath(row[1]).name==name]
if len(expected)!=1 or hashlib.file_digest((folder/name).open('rb'),'sha256').hexdigest()!=expected[0]:
    raise SystemExit('Qualified toolchain release checksum mismatch')
DIGEST
fi
archives=("$WORK"/*.tar.xz)
[[ ${#archives[@]} == 1 && -s "${archives[0]}" ]]
tar -xJf "${archives[0]}" -C "$OUTPUT"
python3 "$ROOT/scripts/verify-ffmpeg-manifest.py" "$OUTPUT"
python3 - "$OUTPUT/toolchain-manifest.json" "$TARGET" <<'CHECK'
import json,sys
assert json.load(open(sys.argv[1]))['target']==sys.argv[2]
CHECK
cmp "$ROOT/third_party/ffmpeg/sources.lock.json" "$OUTPUT/sources.lock.json"
cmp "$ROOT/third_party/ffmpeg/requirements.v1.json" "$OUTPUT/requirements.v1.json"
mkdir -p "$WORK/recipe-check"
tar -xJf "$OUTPUT/corresponding-source.tar.xz" -C "$WORK/recipe-check" provenance
for script in build-ffmpeg.sh normalize-ffmpeg-bundle.py patch-btbn-winarm64-system-error.sh; do
  cmp "$ROOT/scripts/$script" "$WORK/recipe-check/provenance/scripts/$script"
done
diff -r "$ROOT/third_party/ffmpeg/patches" "$WORK/recipe-check/provenance/contract/patches"
