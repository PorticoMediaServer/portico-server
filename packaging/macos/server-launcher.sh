#!/bin/sh
# Keep early startup failures, replacing them on each launch. Runtime messages
# go to the server's bounded message recorder once it has been installed.
set -eu
server_dir="$(cd -P -- "$(dirname -- "$0")/../server" && pwd)"
umask 077
export PORTICO_LOG_STDERR=off
exec "$server_dir/portico-server" > "$PORTICO_STATE_DIR/logs/server.log" 2> "$PORTICO_STATE_DIR/logs/server-error.log"
