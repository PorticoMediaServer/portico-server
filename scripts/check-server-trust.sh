#!/bin/sh
# CI guard: a default build must contain no embedded development root.
set -eu
repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "${tmp:?}"' EXIT HUP INT TERM
# A39: the server's copy of the trust contract must be the Hosted package byte
# for byte: every source, test and test vector, with no file missing on either
# side.
hosted_trust="$repo/services/hosted/trust"
server_trust="$repo/services/server/internal/hostedtrust"
(cd "$hosted_trust" && find . -type f | sort) > "$tmp/hosted-files"
(cd "$server_trust" && find . -type f | sort) > "$tmp/server-files"
if ! cmp -s "$tmp/hosted-files" "$tmp/server-files"; then
  diff "$tmp/hosted-files" "$tmp/server-files" >&2 || true
  echo 'trust copies have different files' >&2; exit 1
fi
while IFS= read -r file; do
  cmp "$hosted_trust/$file" "$server_trust/$file"
done < "$tmp/hosted-files"
# The release root inspector repeats the development-root digest; it must match.
digest=$(sed -n 's/.*hex.EncodeToString(sum\[:\]) == "\([0-9a-f]\{64\}\)".*/\1/p' "$hosted_trust/trust.go")
[ -n "$digest" ] && grep -q "$digest" "$repo/scripts/inspect-hosted-root.go" || { echo 'inspect-hosted-root.go development-root digest differs from trust.go' >&2; exit 1; }
(cd "$repo/services/server" && GOWORK=off go build -o "$tmp/server" ./cmd/server)
root=$(sed -n 's/.*DevelopmentRoot = "\([^"]*\)".*/\1/p' "$repo/services/hosted/trust/development_root.go")
[ -n "$root" ] || exit 1
if grep -a -q "$root" "$tmp/server"; then echo 'untagged server embeds development root' >&2; exit 1; fi
(cd "$repo/services/server" && GOWORK=off go test ./internal/hostedtrust ./internal/hosted -run 'TestUntaggedTrust' -count=1)
