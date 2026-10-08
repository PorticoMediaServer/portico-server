#!/bin/sh
# Rejects built executables and large files in Git (A77). Media fixtures and
# vendored third-party archives are the only large files allowed.
#   scripts/check-repo-binaries.sh            # every tracked file
#   scripts/check-repo-binaries.sh --staged   # files staged for commit (pre-commit hook)
set -eu
repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$repo"
limit=1048576
if [ "${1:-}" = --staged ]; then
	list="git diff --cached --name-only --diff-filter=AM -z"
else
	list="git ls-files -z"
fi
status=0
$list | xargs -0 -I{} sh -c '
	f=$1; limit=$2
	[ -f "$f" ] || exit 0
	case "$f" in fixtures/*|third_party/*) allowed=1 ;; *) allowed=0 ;; esac
	size=$(wc -c <"$f" | tr -d " ")
	magic=$(head -c 4 "$f" | od -An -tx1 | tr -d " \n")
	case "$magic" in
	7f454c46|cffaedfe|cefaedfe|feedfacf|feedface|cafebabe|4d5a*) echo "built executable in Git: $f" >&2; exit 1 ;;
	esac
	if [ "$allowed" = 0 ] && [ "$size" -gt "$limit" ]; then echo "file over 1 MB in Git: $f ($size bytes)" >&2; exit 1; fi
' _ {} "$limit" || status=1
exit $status
