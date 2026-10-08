#!/bin/sh
# Run the Hosted test suite against a throwaway PostgreSQL database.
#
#   dev/test-db.sh [host] [go test args...]
#
# host defaults to /tmp (a local unix socket). Anything after it is passed on to
# go test, e.g. dev/test-db.sh /tmp -run TestWake -v
#
# The database is created before the run and dropped after it, pass or fail.
set -eu

host=${1:-/tmp}
[ $# -gt 0 ] && shift

dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
name="portico_hosted_test_$$"
work=$(mktemp -d)
url="$work/url"

cleanup() {
	status=$?
	dropdb -h "$host" --if-exists "$name" 2>/dev/null || true
	rm -rf "$work"
	exit $status
}
trap cleanup EXIT INT TERM

umask 077
createdb -h "$host" "$name"
printf '%s\n' "postgres:///$name?host=$host&sslmode=disable" >"$url"

echo "Hosted tests against $name on $host"
PORTICO_TEST_DATABASE_URL_FILE="$url" go test -C "$dir" ./... -count=1 "$@"
