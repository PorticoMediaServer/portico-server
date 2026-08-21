#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
npm --prefix "$ROOT/packages/portico-client-core" ci --no-audit --no-fund
npm --prefix "$ROOT/packages/portico-client-core" run build
npm --prefix "$ROOT/web" ci --no-audit --no-fund
npm --prefix "$ROOT/web" run build
npm --prefix "$ROOT/web" run verify:bundle
