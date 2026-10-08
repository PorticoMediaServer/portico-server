#!/bin/sh
# apikit (the Go API toolkit Hosted and the server share) is owned by portico-server.
# hosted-services/apikit is an identical copy, so this repository builds on its own.
#   scripts/shared-apikit.sh          # fail if the copy differs from portico-server's
#   scripts/shared-apikit.sh --write  # replace the copy with portico-server's
# PORTICO_SERVER_REPO points at the portico-server checkout (default: ../apps/portico-server).
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
server=${PORTICO_SERVER_REPO:-"$here/../apps/portico-server"}
[ -d "$server/apikit" ] || { echo "shared-apikit: no portico-server checkout at $server" >&2; exit 2; }
if [ "${1:-}" = --write ]; then
	rm -rf "${here:?}/hosted-services/apikit"
	cp -R "$server/apikit" "$here/hosted-services/apikit"
	exit 0
fi
diff -r "$server/apikit" "$here/hosted-services/apikit" && echo 'shared-apikit: identical to portico-server'
