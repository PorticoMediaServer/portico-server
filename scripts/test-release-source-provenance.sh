#!/usr/bin/env bash
set -euo pipefail

SCRIPTS="$(cd "$(dirname "$0")" && pwd)"
WORK="$(mktemp -d)"
ROOT="$WORK/server"
FFMPEG="$WORK/ffmpeg"
BIN="$WORK/bin"
cleanup() { rm -rf "$WORK"; }
trap cleanup EXIT INT TERM

mkdir -p "$ROOT/scripts" "$ROOT/cmd/porticod" "$ROOT/web/dist" "$ROOT/media-toolchain" "$FFMPEG" "$BIN"
cp "$SCRIPTS/build-release-payload.sh" "$SCRIPTS/source-tree-revision.py" "$ROOT/scripts/"
chmod 0700 "$ROOT/scripts/build-release-payload.sh" "$ROOT/scripts/source-tree-revision.py"
printf 'package main\n' > "$ROOT/cmd/porticod/main.go"
printf '<!doctype html><title>Portico</title>\n' > "$ROOT/web/dist/index.html"
printf 'license\n' > "$ROOT/LICENSE"
printf 'notices\n' > "$ROOT/THIRD-PARTY-NOTICES.md"
printf 'media notice\n' > "$ROOT/media-toolchain/NOTICE.md"
printf '#!/bin/sh\nexit 0\n' > "$FFMPEG/ffmpeg"
printf '#!/bin/sh\nexit 0\n' > "$FFMPEG/ffprobe"
chmod 0700 "$FFMPEG/ffmpeg" "$FFMPEG/ffprobe"

cat > "$ROOT/.gitignore" <<'EOF'
/dist/
/web/dist/
EOF

cat > "$BIN/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
output=""
previous=""
for argument in "$@"; do
  if [[ "$previous" == "-o" ]]; then output="$argument"; fi
  previous="$argument"
done
[[ -n "$output" ]]
printf '#!/bin/sh\nexit 0\n' > "$output"
EOF
chmod 0700 "$BIN/go"

git -C "$ROOT" init -q
git -C "$ROOT" config user.name 'Portico Release Test'
git -C "$ROOT" config user.email 'release-test@portico.invalid'
git -C "$ROOT" add .
git -C "$ROOT" commit -qm 'fixture'

head_revision="$(git -C "$ROOT" rev-parse HEAD)"
PATH="$BIN:$PATH" "$ROOT/scripts/build-release-payload.sh" 1.0.0 linux amd64 "$FFMPEG" "$ROOT/dist/clean" 1
grep -Fq "\"commit\": \"$head_revision\"" "$ROOT/dist/clean/release.json"

printf 'dirty source\n' >> "$ROOT/cmd/porticod/main.go"
if PATH="$BIN:$PATH" "$ROOT/scripts/build-release-payload.sh" 1.0.0 linux amd64 "$FFMPEG" "$ROOT/dist/unreviewed" 2 >/dev/null 2>&1; then
  echo "unreviewed dirty Server source was accepted" >&2
  exit 1
fi

source_revision="$("$ROOT/scripts/source-tree-revision.py" "$ROOT")"
wrong_revision=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
if PORTICO_BUILD_SOURCE_REVISION="$wrong_revision" PATH="$BIN:$PATH" \
  "$ROOT/scripts/build-release-payload.sh" 1.0.0 linux amd64 "$FFMPEG" "$ROOT/dist/mismatched" 3 >/dev/null 2>&1; then
  echo "mismatched dirty Server source revision was accepted" >&2
  exit 1
fi

PORTICO_BUILD_SOURCE_REVISION="$source_revision" PORTICO_BUILD_TIMESTAMP=2026-08-30T12:00:00Z PATH="$BIN:$PATH" \
  "$ROOT/scripts/build-release-payload.sh" 1.0.0 linux amd64 "$FFMPEG" "$ROOT/dist/reviewed" 4
grep -Fq "\"commit\": \"$source_revision\"" "$ROOT/dist/reviewed/release.json"
grep -Fq '"builtAt": "2026-08-30T12:00:00Z"' "$ROOT/dist/reviewed/release.json"

printf 'new untracked source\n' > "$ROOT/new-source.go"
if PORTICO_BUILD_SOURCE_REVISION="$source_revision" PATH="$BIN:$PATH" \
  "$ROOT/scripts/build-release-payload.sh" 1.0.0 linux amd64 "$FFMPEG" "$ROOT/dist/drifted" 5 >/dev/null 2>&1; then
  echo "source drift after review was accepted" >&2
  exit 1
fi
new_revision="$("$ROOT/scripts/source-tree-revision.py" "$ROOT")"
[[ "$new_revision" != "$source_revision" ]]

rm "$ROOT/new-source.go"
ln -s cmd/porticod/main.go "$ROOT/source-link"
if "$ROOT/scripts/source-tree-revision.py" "$ROOT" >/dev/null 2>&1; then
  echo "source revision accepted a symbolic link" >&2
  exit 1
fi

echo "Server release source provenance tests passed"
