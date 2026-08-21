#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

echo "==> Running scheduled 1,000,000-item catalogue / 100-active-viewer performance gate"
PORTICO_PERFORMANCE_TIER=deep go test ./internal/app \
  -run 'Test(LargeLibraryEndpointPerformanceBudgets|MixedUserBrowsingLoadSmoke|MillionItemMostlyUnchangedScannerCheckpointGate)$' \
  -count=1 \
  -timeout 45m

echo "==> Deep performance gate passed"
