#!/bin/sh
set -eu
payload_dir="$(cd -P -- "$(dirname -- "$0")" && pwd)"
umask 077
PORTICO_WEB_DIR="${PORTICO_WEB_DIR:-$payload_dir/web}"
PORTICO_STATE_DIR="${PORTICO_STATE_DIR:-$payload_dir/state}"
export PORTICO_WEB_DIR PORTICO_STATE_DIR
exec "$payload_dir/portico-server" "$@"
