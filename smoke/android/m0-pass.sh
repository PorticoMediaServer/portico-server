#!/usr/bin/env bash
# One authorised M0 Android core pass on the pinned 119 artifact.
#
#   smoke/android/m0-pass.sh --out /path/scratch/phone-119 [--serial emulator-5554]
#
# Run ONLY after the named Portico_Phone_API36 owner has confirmed stopped and wiped. The script
# boots the named AVD itself, installs the pinned APK over cleared data, runs the core flows and the
# owner Settings capture, then stops the emulator, wipes its data and proves adb is empty. It never
# creates a second instance and never touches CertTest or any PIN.
set -uo pipefail

ARTIFACT=/home/justin/projects/Portico/.artifacts/release-r40/native-119112b3/portico-google-debug-x86_64-119112b3.apk
PIN=119112b3
EXPECTED_SHA=c0efe442aadebab6b8be938ca0cb950f06e3a44ea0a0ca603f5427b61d4c31ea
AVD=Portico_Phone_API36
SERIAL=emulator-5554
OUT=""
CREDS=/home/justin/projects/Portico/.private/demo-testing/active-credentials.json
SERVER=https://demo.getportico.tv

while [ $# -gt 0 ]; do
  case "$1" in
    --out) OUT="$2"; shift 2 ;;
    --serial) SERIAL="$2"; shift 2 ;;
    --creds) CREDS="$2"; shift 2 ;;
    --server) SERVER="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[ -n "$OUT" ] || { echo "usage: $0 --out DIR [--serial emulator-5554]" >&2; exit 2; }
mkdir -p "$OUT"
LOG="$OUT/pass.log"
say() { printf '%s %s\n' "$1" "$2" | tee -a "$LOG"; }

# 0. The artifact must be the pinned one.
[ -f "$ARTIFACT" ] || { say FAIL "artifact missing: $ARTIFACT"; exit 1; }
ACTUAL_SHA=$(sha256sum "$ARTIFACT" | cut -d' ' -f1)
[ "$ACTUAL_SHA" = "$EXPECTED_SHA" ] || { say FAIL "artifact hash $ACTUAL_SHA does not match the pin"; exit 1; }
say PASS "artifact $PIN verified ($(stat -c %s "$ARTIFACT") bytes)"

# 1. Never start a second instance.
if adb devices | awk 'NR>1 && $2=="device"' | grep -q .; then
  say FAIL "a device is already running; this script only boots the named AVD from stopped"
  exit 75
fi

say INFO "booting $AVD"
nohup "$HOME/Android/Sdk/emulator/emulator" -avd "$AVD" -no-window -no-audio -no-boot-anim \
  -gpu swiftshader_indirect -no-snapshot -memory 2048 -cores 4 > "$OUT/emulator.log" 2>&1 &
adb wait-for-device >/dev/null 2>&1
for _ in $(seq 1 60); do
  [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ] && break
  sleep 5
done
[ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ] || { say FAIL "the AVD did not finish booting"; exit 1; }
adb shell wm size 393x852 >/dev/null 2>&1
adb shell wm density 160 >/dev/null 2>&1
say PASS "booted at 393x852, density 160"

SHELL_DIR="$(dirname "$0")/.."
cleanup() {
  adb shell wm size reset >/dev/null 2>&1 || true
  adb shell wm density reset >/dev/null 2>&1 || true
  adb emu kill >/dev/null 2>&1 || true
  sleep 8
  rm -f "$HOME/.android/avd/$AVD.avd/userdata-qemu.img" "$HOME/.android/avd/$AVD.avd/userdata-qemu.img.qcow2" \
        "$HOME/.android/avd/$AVD.avd/snapshots/"* 2>/dev/null || true
  local devices
  devices=$(adb devices | awk 'NR>1 && $2=="device"' | wc -l)
  say INFO "stopped and wiped; adb devices=$devices; AVD size=$(du -sh "$HOME/.android/avd/$AVD.avd" 2>/dev/null | cut -f1)"
}
trap cleanup EXIT

# 2. Core flows.
python3 "$SHELL_DIR/smoke/android/flows.py" --serial "$SERIAL" --apk "$ARTIFACT" --mode phone \
  --out "$OUT/core" --creds "$CREDS" --server "$SERVER" --build "$PIN" >> "$LOG" 2>&1
CORE=$?
say "$([ $CORE -eq 0 ] && echo PASS || echo FAIL)" "core flows exit=$CORE"

# 3. Owner Settings evidence: row order, Troubleshooting pair, Storage and Integrations when drawn.
python3 "$SHELL_DIR/smoke/android/settings-capture.py" --serial "$SERIAL" --apk "$ARTIFACT" \
  --out "$OUT/settings" --creds "$CREDS" --server "$SERVER" --build "$PIN" >> "$LOG" 2>&1
SETTINGS=$?
say "$([ $SETTINGS -eq 0 ] && echo PASS || echo FAIL)" "settings capture exit=$SETTINGS"

say INFO "evidence under $OUT (core, settings, pass.log)"
exit $(( CORE != 0 || SETTINGS != 0 ))
