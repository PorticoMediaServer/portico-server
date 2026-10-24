#!/usr/bin/env bash
# Remote runner entry for the DS web settings lane. Run from the repository root
# on the external runner with an exact pushed commit. Modes:
#   capture  build the genuine screen and its approved mock into $PORTICO_RUNNER_OUTPUT
#   gate     bounded types, affected Settings tests and mandatory security tests
set -euo pipefail
mode=${1:-capture}
required=$(cat .node-version)
if [ "$(node -p process.versions.node)" != "$required" ]; then
  node_dir="$TMPDIR/node-exact"
  mkdir -p "$node_dir"
  archive="node-v$required-linux-x64.tar.xz"
  curl -fsSL "https://nodejs.org/dist/v$required/SHASUMS256.txt" -o "$node_dir/sums"
  curl -fsSL "https://nodejs.org/dist/v$required/$archive" -o "$node_dir/$archive"
  (cd "$node_dir"; awk -v file="$archive" '$2 == file' sums | sha256sum -c -)
  tar -xf "$node_dir/$archive" -C "$node_dir"
  export PATH="$node_dir/node-v$required-linux-x64/bin:$PATH"
fi
export NODE_OPTIONS=--max-old-space-size=1536
npm ci --ignore-scripts --no-audit --no-fund --quiet
npm exec -w web-next -- tsc --noEmit
if [ "$mode" = gate ]; then
  npm run lint:design --workspace web-next
  npm run lint:hooks --workspace web-next
fi
if [ "$mode" = capture ]; then
  npm exec -w web-next -- vite build --logLevel warn
  mkdir -p dist
  (cd server && GOTOOLCHAIN=go1.26.10 CGO_ENABLED=0 go build -trimpath -o "$OLDPWD/dist/portico-server" ./cmd/server)
  PORTICO_SERVER_BIN="$PWD/dist/portico-server" node web-next/src/screens/settings/ds-web-settings-r46-capture.mjs
  exit 0
fi
cd web-next
node --experimental-transform-types --import ./test/register.mjs --test --test-concurrency=1 \
  test/settings-configuration-binding.test.ts test/settings-effects.test.ts test/settings-search.test.ts \
  test/settings-platform.test.ts test/settings-owner-switch.test.ts test/settings-visibility.test.ts \
  test/settings-spec.test.ts test/settings-condition.test.ts test/shell-home-settings.test.ts \
  test/account-entry-authority.test.ts test/viewer-storage.test.ts \
  ../packages/client-core/test/account-sessions.test.ts ../packages/client-core/test/session-refresh.test.ts \
  ../packages/client-core/test/identity-restrictions.test.ts ../packages/client-core/test/profiles-membership.test.ts \
  ../packages/client-core/test/generated-routes.test.ts ../packages/client-core/test/credential-scoped-logout.test.ts
