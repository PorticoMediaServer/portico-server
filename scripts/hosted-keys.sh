#!/bin/sh
# Portico Hosted trust keys: one-time setup and yearly renewal.
# See deployment/hosted/KEYS.md. Runs only locally; it never contacts anything.
#
#   scripts/hosted-keys.sh setup --private-out <path> [--staging <new folder>]
#   scripts/hosted-keys.sh renew --root <path> --current-certificate <file> [--staging <new folder>]
set -eu
umask 077
repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
usage() {
	sed -n 2,7p "$0" | sed 's/^# \{0,1\}//' >&2
	exit 2
}
[ $# -ge 1 ] || usage
command=$1
shift
case "$command" in setup | renew) ;; *) usage ;; esac
staging="$PWD/hosted-keys-$(date -u +%Y%m%d-%H%M%S)"
args=""
while [ $# -gt 0 ]; do
	case "$1" in
	--staging)
		[ $# -ge 2 ] || usage
		staging=$2
		shift 2
		;;
	--private-out | --root | --current-certificate | --days)
		[ $# -ge 2 ] || usage
		args="$args $1 $(printf '%s' "$2" | sed "s/'/'\\\\''/g; s/^/'/; s/\$/'/")"
		shift 2
		;;
	*) usage ;;
	esac
done
work=$(mktemp -d)
trap 'rm -rf "${work:?}"' EXIT HUP INT TERM
(cd "$repo/hosted-services/hosted" && GOWORK=off go build -o "$work/hosted-keytool" ./cmd/hosted)
if [ "$command" = setup ]; then
	eval "set -- $args"
	"$work/hosted-keytool" keys setup "$@" --public-out "$repo/deployment/hosted/root-public-key.pem" --staging "$staging"
	# The public key is public; the release builders and Git read it.
	chmod 644 "$repo/deployment/hosted/root-public-key.pem"
else
	eval "set -- $args"
	"$work/hosted-keytool" keys renew "$@" --staging "$staging"
fi
