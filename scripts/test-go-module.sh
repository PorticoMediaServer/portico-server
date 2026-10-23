#!/bin/sh
# Give the CPU-heavy catalogue its own phase without skipping any module tests.
set -eu
case "${1:-}" in
apikit) exec go test ./... ;;
server) ;;
*) echo 'expected module: apikit or server' >&2; exit 2 ;;
esac

catalog=portico.local/server/internal/catalog
packages=$(go list ./...)
if ! printf '%s\n' "$packages" | grep -Fx "$catalog" >/dev/null; then
  echo 'catalog package missing from module test inventory' >&2
  exit 2
fi
remaining=$(printf '%s\n' "$packages" | awk -v catalog="$catalog" '$0 != catalog')

status=0
echo 'Testing catalogue alone with the unchanged default test timeout'
go test "$catalog" || status=$?
# A failed catalogue phase must not hide failures in the rest of the module.
# Go import paths contain no shell whitespace; catalogue subpackages stay here.
echo 'Testing every remaining module package'
# shellcheck disable=SC2086
go test $remaining || status=$?
exit "$status"
