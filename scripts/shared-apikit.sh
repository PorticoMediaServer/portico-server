#!/bin/sh
# apikit (the Go API toolkit Hosted and the server share) is owned by portico-server.
# hosted-services/apikit is an identical copy, so this repository builds on its own.
#   scripts/shared-apikit.sh          # fail if the copy differs from portico-server's
#   scripts/shared-apikit.sh --write  # replace the copy with portico-server's
# PORTICO_SERVER_REPO points at a portico-server Git checkout.
# Read origin/main's Git objects so a dirty checkout cannot become a source input.
# PORTICO_SERVER_REF may pin an older commit already merged into origin/main.
set -eu
here=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
server=${PORTICO_SERVER_REPO:-"$here/../apps/portico-server"}
exec python3 - "$here" "$server" "${PORTICO_SERVER_REF:-origin/main}" "$@" <<'PY'
import os
from pathlib import Path
import subprocess
import sys

root, server = map(Path, sys.argv[1:3])
ref = sys.argv[3]
args = sys.argv[4:]
if args not in ([], ["--write"]):
    sys.exit("usage: scripts/shared-apikit.sh [--write]")

def git(*args):
    return subprocess.check_output(["git", "-C", str(server), *args])

commit = git("rev-parse", "--verify", ref + "^{commit}").decode().strip()
main = git("rev-parse", "--verify", "origin/main^{commit}").decode().strip()
if subprocess.run(["git", "-C", str(server), "merge-base", "--is-ancestor", commit, main]).returncode:
    sys.exit("shared-apikit: source must be merged into server origin/main")

files = {}
for row in git("ls-tree", "-rz", commit, "--", "apikit").split(b"\0"):
    if not row:
        continue
    metadata, name = row.split(b"\t", 1)
    mode, kind, oid = metadata.split()
    relative = Path(os.fsdecode(name)).relative_to("apikit")
    if kind != b"blob" or mode not in (b"100644", b"100755"):
        sys.exit("shared-apikit: unsupported canonical entry: " + str(relative))
    files[relative] = (git("cat-file", "blob", oid.decode()), int(mode, 8) & 0o777)
if Path("go.mod") not in files:
    sys.exit("shared-apikit: canonical source has no apikit/go.mod")

destination = root / "hosted-services/apikit"
if destination.is_symlink() or destination.parent.is_symlink():
    sys.exit("shared-apikit: destination must not be a symlink")
actual = {p.relative_to(destination): p for p in destination.rglob("*") if p.is_file() or p.is_symlink()}
if any(p.is_symlink() for p in destination.rglob("*")):
    sys.exit("shared-apikit: destination contains a symlink")
different = sorted(relative for relative, (data, _) in files.items()
                   if relative not in actual or actual[relative].read_bytes() != data)
extra = sorted(set(actual) - set(files))
if args == ["--write"]:
    destination.mkdir(parents=True, exist_ok=True)
    for relative, (data, mode) in files.items():
        target = destination / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_bytes(data)
        target.chmod(mode)
    for relative in extra:
        actual[relative].unlink()
elif different or extra:
    for relative in different:
        print("shared-apikit: differs: " + str(relative), file=sys.stderr)
    for relative in extra:
        print("shared-apikit: extra: " + str(relative), file=sys.stderr)
    sys.exit(1)
print("shared-apikit: identical to server main input " + commit)
PY
