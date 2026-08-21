#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WEB_DIR="$ROOT_DIR/web"
CLIENT_CORE_DIR="$ROOT_DIR/packages/portico-client-core"

cd "$ROOT_DIR"

echo "==> Running server tests"
go test ./...

echo "==> Running production-shaped 100k catalogue / 100-viewer gate"
PORTICO_PERFORMANCE_TIER=release go test ./internal/app -run 'Test(LargeLibraryEndpointPerformanceBudgets|MixedUserBrowsingLoadSmoke)$' -count=1

echo "==> Building reusable client core"
(cd "$CLIENT_CORE_DIR" && npm run build)

echo "==> Building web app"
(cd "$WEB_DIR" && npm run build)
(cd "$WEB_DIR" && npm run verify:bundle)

echo "==> Checking web performance bundle guardrails"
(cd "$WEB_DIR" && npm run performance:bundle)

DB_PATH="${PORTICO_DATABASE_PATH:-${PORTICO_APP_DATA:-$ROOT_DIR/var}/portico.db}"
if [[ -f "$DB_PATH" ]]; then
  echo "==> Checking active job smoke guardrails"
  go run ./scripts/check-active-jobs.go "$DB_PATH"
else
  echo "==> Skipping active job smoke guardrails; database not found at $DB_PATH"
fi

echo "==> Performance release check passed"
