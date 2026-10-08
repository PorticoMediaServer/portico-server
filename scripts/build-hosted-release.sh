#!/bin/sh
# Build only; never deploy. The root is public, but its absence is a release gate.
# Development builds of Hosted with the official root; full releases use build-hosted-release.py.
set -eu
repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
root_file=${PORTICO_ROOT_PUBLIC_KEY_FILE:-"$repo/deployment/hosted/root-public-key.pem"}
[ -f "$root_file" ] || { echo 'Official root public key file required for release' >&2; exit 1; }
root_key=$(go run "$repo/scripts/inspect-hosted-root.go" "$root_file")
case "$root_key" in *[!A-Za-z0-9_-]*|'') echo 'Invalid root encoding' >&2; exit 1;; esac
output=${1:-"$repo/.scratch/portico-hosted-release"}
cd "$repo/hosted-services/hosted"
go build -tags release -ldflags "-X portico.local/hosted/trust.OfficialRoot=$root_key" -o "$output" ./cmd/hosted
