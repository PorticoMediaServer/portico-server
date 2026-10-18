#!/usr/bin/env bash
# Private fixture-driven Classic/Cinema review on the existing Ubuntu AVDs.
set -euo pipefail
here=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
export ANDROID_HOME=${ANDROID_HOME:-/home/justin/Android/Sdk}
export JAVA_HOME=${PORTICO_JAVA_HOME:-/usr/lib/jvm/java-17-openjdk-amd64}
export PATH="$JAVA_HOME/bin:$ANDROID_HOME/platform-tools:$PATH"

usage() {
  cat <<'EOF'
Usage: scripts/test-experiments.sh COMMAND
  status
  start phone|tv
  build classic|cinema|all
  install phone|tv classic|cinema|all
  open phone|tv classic|cinema
  screen phone|tv classic|cinema SCREEN_ID
  key phone|tv up|down|left|right|select|back|home|playpause [...]
  shot phone|tv classic|cinema [OUTPUT.png]

Builds standalone x86_64 testing APKs; no Metro or backend needed.
The existing main Portico installation and its data are preserved.
EOF
}

device() {
  case "${1:-}" in
    phone) serial=emulator-5554; avd=Portico_Phone_API36 ;;
    tv) serial=emulator-5556; avd=Portico_TV_API36 ;;
    *) echo 'Choose phone or tv.' >&2; exit 2 ;;
  esac
}

verify_device() {
  local current
  current=$(adb -s "$serial" emu avd name | tr -d '\r' | head -1)
  [[ "$current" == "$avd" ]] || { echo "$serial is $current; expected $avd. Refusing to alter it." >&2; exit 1; }
  [[ $(adb -s "$serial" shell getprop sys.boot_completed | tr -d '\r') == 1 ]] || {
    echo "$avd has not finished booting." >&2; exit 1;
  }
}

experiment() {
  case "${1:-}" in
    classic|cinema) variant=$1 ;;
    *) echo 'Choose classic or cinema.' >&2; exit 2 ;;
  esac
  app_id="tv.getportico.experiment.$variant"
  activity="com.portico${variant}.MainActivity"
  project="$here/experiments/$variant"
  apk="$project/android/app/build/outputs/apk/release/app-release.apk"
}

build_one() {
  experiment "$1"
  if [[ ! -d "$project/node_modules" ]]; then
    (cd "$project" && npm ci --legacy-peer-deps --no-audit --no-fund)
  fi
  (cd "$project/android" && bash ./gradlew --no-daemon --max-workers=1 \
    '-Dorg.gradle.jvmargs=-Xmx1536m -XX:MaxMetaspaceSize=512m' \
    :app:assembleRelease "-PreactNativeArchitectures=${PORTICO_EXPERIMENT_ABIS:-x86_64}")
  echo "$apk"
}

install_one() {
  experiment "$1"
  [[ -f "$apk" ]] || { echo "Missing $apk; build $variant first." >&2; exit 1; }
  local built_id
  built_id=$("$ANDROID_HOME/build-tools/36.0.0/aapt" dump badging "$apk" | sed -n "s/^package: name='\([^']*\)'.*/\1/p")
  [[ "$built_id" == "$app_id" ]] || {
    echo "APK identifies as $built_id; expected $app_id. Refusing to install it." >&2; exit 1;
  }
  adb -s "$serial" install -r "$apk"
}

case "${1:-help}" in
  status)
    adb devices -l
    for target in phone tv; do
      device "$target"
      if adb -s "$serial" get-state >/dev/null 2>&1; then
        echo "$target ($serial):"
        adb -s "$serial" emu avd name | tr -d '\r'
        adb -s "$serial" shell pm list packages | rg '^package:tv\.getportico\.(portico|experiment\.(classic|cinema))$' || true
      fi
    done
    ;;
  start)
    device "${2:-}"
    exec "$here/scripts/test-clients.sh" start "$2"
    ;;
  build)
    case "${2:-}" in all) build_one classic; build_one cinema ;; *) build_one "${2:-}" ;; esac
    ;;
  install)
    device "${2:-}"; verify_device
    case "${3:-}" in all) install_one classic; install_one cinema ;; *) install_one "${3:-}" ;; esac
    ;;
  open)
    device "${2:-}"; verify_device; experiment "${3:-}"
    adb -s "$serial" shell am start -W -n "$app_id/$activity"
    ;;
  screen)
    device "${2:-}"; verify_device; experiment "${3:-}"
    screen_id=${4:-}
    [[ -n "$screen_id" && "$screen_id" != *[!a-zA-Z0-9/_-]* ]] || { echo 'Provide a screen ID from the experiment screen index.' >&2; exit 2; }
    adb -s "$serial" shell am start -W -n "$app_id/$activity" \
      -a android.intent.action.VIEW -d "portico-$variant://screen/$screen_id"
    ;;
  key)
    device "${2:-}"; verify_device; shift 2
    [[ $# -gt 0 ]] || { usage; exit 2; }
    for key in "$@"; do
      case "$key" in
        up) code=19 ;; down) code=20 ;; left) code=21 ;; right) code=22 ;;
        select) code=23 ;; back) code=4 ;; home) code=3 ;; playpause) code=85 ;;
        *) echo "Unknown key: $key" >&2; exit 2 ;;
      esac
      adb -s "$serial" shell input keyevent "$code"
    done
    ;;
  shot)
    device "${2:-}"; verify_device; experiment "${3:-}"
    output=${4:-"$here/.scratch/experiment-review/$variant-$2.png"}
    mkdir -p -- "$(dirname -- "$output")"
    adb -s "$serial" exec-out screencap -p > "$output"
    echo "$output"
    ;;
  help|--help|-h) usage ;;
  *) usage >&2; exit 2 ;;
esac
