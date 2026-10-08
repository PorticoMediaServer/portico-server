#!/bin/sh
PORTICO_ROOT="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
export GOCACHE="$PORTICO_ROOT/.cache/go-build"
export GOMODCACHE="$PORTICO_ROOT/.cache/go-mod"
export TMPDIR="$PORTICO_ROOT/.cache/tmp"
export npm_config_cache="$PORTICO_ROOT/.cache/npm"
export CP_HOME_DIR="$PORTICO_ROOT/.cache/cocoapods"
export DEVELOPER_DIR=/Applications/Xcode-beta.app/Contents/Developer
exec "$@"
