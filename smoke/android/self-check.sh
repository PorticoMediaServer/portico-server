#!/usr/bin/env bash
# Offline self-check for the Android pass harness.
#
#   bash smoke/android/self-check.sh
#
# Needs no device, no emulator and no credentials, so it can run on a VPS as the item gate for a
# smoke-harness change. It proves the scripts parse, their CLIs load, and the guards that keep a
# second emulator instance out of the way are still present.
set -uo pipefail
SELF="$(readlink -f "${BASH_SOURCE[0]}")"
ROOT="$(cd "$(dirname "$SELF")/../.." && pwd)"
fail=0
ok() { printf 'PASS %s\n' "$1"; }
bad() { printf 'FAIL %s\n' "$1"; fail=1; }

bash -n "$ROOT/smoke/android/m0-pass.sh" && ok "m0-pass.sh parses" || bad "m0-pass.sh does not parse"

python3 - "$ROOT" <<'PY' || fail=1
import ast, sys
root = sys.argv[1]
for name in ("flows.py", "settings-capture.py"):
    path = f"{root}/smoke/android/{name}"
    ast.parse(open(path).read(), path)
    print(f"PASS {name} parses")
PY

python3 "$ROOT/smoke/android/flows.py" --help >/dev/null && ok "flows.py CLI loads" || bad "flows.py CLI does not load"
python3 "$ROOT/smoke/android/settings-capture.py" --help >/dev/null && ok "settings-capture.py CLI loads" || bad "settings-capture.py CLI does not load"

grep -q 'readlink -f "${BASH_SOURCE\[0\]}"' "$ROOT/smoke/android/m0-pass.sh" \
  && ok "the pass resolves its own root" || bad "the pass resolves its root from the caller"
grep -q 'a device or emulator is already attached' "$ROOT/smoke/android/m0-pass.sh" \
  && ok "the pass refuses an attached device" || bad "the pass may start a second instance"
grep -q 'qemu-system.\*-avd' "$ROOT/smoke/android/m0-pass.sh" \
  && ok "the pass refuses a live AVD process" || bad "the pass may start a second instance"
grep -q 'locks=' "$ROOT/smoke/android/m0-pass.sh" \
  && ok "the pass proves the wipe with device and lock counts" || bad "the wipe has no proof"
grep -q -- '--drawn-size' "$ROOT/smoke/android/m0-pass.sh" \
  && ok "the display override is opt-in" || bad "the display override is not selectable"
grep -q 'home_wait_stages' "$ROOT/smoke/android/flows.py" \
  && ok "a Home stall keeps staged evidence" || bad "a Home stall keeps no evidence"
grep -q 'def actual_viewport' "$ROOT/smoke/android/settings-capture.py" \
  && ok "the capture records the real viewport" || bad "the capture records a constant viewport"

printf 'self-check exit=%s\n' "$fail"
exit "$fail"
