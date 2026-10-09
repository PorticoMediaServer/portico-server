#!/usr/bin/env bash
# Personal Ubuntu host: release builds and visible testing devices, using the demo VPS.
set -euo pipefail
here=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
native=${PORTICO_NATIVE_REPO:-"$here/../apps/portico-react-native"}
export ANDROID_HOME=${ANDROID_HOME:-/home/justin/Android/Sdk}
export JAVA_HOME=${PORTICO_JAVA_HOME:-/usr/lib/jvm/java-17-openjdk-amd64}
export PATH="$JAVA_HOME/bin:$ANDROID_HOME/platform-tools:$HOME/.local/bin:$PATH"
logs="$native/.scratch/interactive-testing"
mkdir -p "$logs"
desktop() {
  while IFS='=' read -r name value; do
    case "$name" in DISPLAY|WAYLAND_DISPLAY|XDG_RUNTIME_DIR|XAUTHORITY|DBUS_SESSION_BUS_ADDRESS) export "$name=$value" ;; esac
  done < <(systemctl --user show-environment)
  test -n "${DISPLAY:-}" || { echo 'No active Ubuntu desktop session.' >&2; exit 1; }
}
device() {
  case "${1:-}" in phone) serial=emulator-5554; avd=Portico_Phone_API36; port=5554 ;; tv) serial=emulator-5556; avd=Portico_TV_API36; port=5556 ;; *) echo 'Choose phone or tv.' >&2; exit 2 ;; esac
}
verify_device() {
  current=$(adb -s "$serial" emu avd name | tr -d '\r' | head -1)
  test "$current" = "$avd" || { echo "$serial is $current; expected $avd. Refusing to alter it." >&2; exit 1; }
}
case "${1:-help}" in
  build)
    (cd "$native/apps/native" && npm ci --no-audit --no-fund --legacy-peer-deps)
    (cd "$native/apps/native/android" && ./gradlew --no-daemon --max-workers=3 :app:assembleGoogleRelease :app:assembleFiretvRelease "-PreactNativeArchitectures=${PORTICO_ANDROID_ABIS:-x86_64,arm64-v8a}")
    (cd "$native/apps/vega" && npm ci --no-audit --no-fund --legacy-peer-deps && node scripts/vega.mjs build --release)
    ;;
  start)
    desktop
    if [ "${2:-}" = vega ]; then
      # Keep the SDK's virtual-device descendants outside this terminal's group.
      # Otherwise terminal cleanup can kill a successfully booted virtual device.
      (cd "$native/apps/vega" && PORTICO_VEGA_GUI=1 nohup setsid node scripts/vega.mjs device start > "$logs/vega-device.log" 2>&1 < /dev/null &)
      adaptor=$(vega which vda | tail -1)
      test -x "$adaptor" || { echo 'Vega device adaptor was not found.' >&2; exit 1; }
      vega_serial="emulator-$(( ${PORTICO_VEGA_DEVICE_PORT:-5701} - 1 ))"
      ready=0
      for ((attempt=0; attempt<180; attempt++)); do
        if (cd "$native/apps/vega" && node scripts/vega.mjs device status 2>/dev/null) | rg -q '"running":\s*true' &&
          [ "$(ANDROID_ADB_SERVER_PORT=${PORTICO_VEGA_ADB_PORT:-5039} timeout 3 "$adaptor" -s "$vega_serial" get-state 2>/dev/null | tr -d '\r')" = device ]; then
          ready=1; break
        fi
        sleep 1
      done
      test "$ready" = 1 || { echo "Vega did not boot; see $logs/vega-device.log" >&2; exit 1; }
      echo "Vega ready; log $logs/vega-device.log"
    else
      device "${2:-}"
      if adb -s "$serial" get-state >/dev/null 2>&1; then
        verify_device; echo "$avd already running"
      else
        nohup setsid "$ANDROID_HOME/emulator/emulator" -avd "$avd" -port "$port" -cores 4 -memory 3072 -no-snapshot-load -gpu software > "$logs/$avd.log" 2>&1 < /dev/null &
        echo "Starting $avd on $serial; log $logs/$avd.log"
      fi
      ready=0
      for ((attempt=0; attempt<180; attempt++)); do
        if [ "$(adb -s "$serial" shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = 1 ]; then ready=1; break; fi
        sleep 1
      done
      test "$ready" = 1 || { echo "$avd did not boot within 180 seconds; see its log." >&2; exit 1; }
      verify_device
      echo "$avd ready for installation"
    fi
    ;;
  install)
    if [ "${2:-}" = vega ]; then
      (cd "$native/apps/vega" && node scripts/vega.mjs install --release && node scripts/vega.mjs launch)
    else
      device "${2:-}"; verify_device
      flavor=${3:-google}
      case "$flavor" in google|firetv) ;; *) echo 'Choose google or firetv.' >&2; exit 2 ;; esac
      adb -s "$serial" install -r "$native/apps/native/android/app/build/outputs/apk/$flavor/release/app-$flavor-release.apk"
      adb -s "$serial" shell am force-stop tv.getportico.portico
      adb -s "$serial" shell am start -n tv.getportico.portico/.MainActivity
    fi
    ;;
  stop)
    if [ "${2:-}" = vega ]; then (cd "$native/apps/vega" && node scripts/vega.mjs device stop)
    else
      device "${2:-}"; verify_device; adb -s "$serial" emu kill
      stopped=0
      for ((attempt=0; attempt<30; attempt++)); do
        if ! adb -s "$serial" get-state >/dev/null 2>&1 && ! (: < "/dev/tcp/127.0.0.1/$port") 2>/dev/null; then stopped=1; break; fi
        sleep 1
      done
      test "$stopped" = 1 || { echo "$avd is still stopping; wait before restarting it." >&2; exit 1; }
    fi
    ;;
  status)
    adb devices -l
    (cd "$native/apps/vega" && node scripts/vega.mjs device status)
    curl -fsS https://demo.getportico.tv/portico-build.json
    ;;
  help|--help|-h)
    echo 'Usage: scripts/test-clients.sh build | start phone|tv|vega | install phone|tv [google|firetv] | install vega | stop phone|tv|vega | status'
    ;;
  *) echo 'Unknown command; use --help.' >&2; exit 2 ;;
esac
