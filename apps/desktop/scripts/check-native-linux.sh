#!/bin/bash
# One owned foreground Docker job. All source/artifacts are per-job; no host installs.
set -eu
cd "$(dirname "$0")/../../.."
job="desktop-r29-$(date +%s)-$$"
image="portico/$job"
scope="${1:-engine}"
case "$scope" in shell|api|startup|engine) ;; *) exit 64 ;; esac
cleanup(){
  if [ "$scope" = startup ] && [ -n "${PORTICO_RUNNER_OUTPUT:-}" ]; then
    if [ -f "$TMPDIR/native-work/startup-report.native.log" ]; then
      cp "$TMPDIR/native-work/startup-report.native.log" "$PORTICO_RUNNER_OUTPUT/native-startup.log"
    else printf '%s\n' 'Native startup did not reach a recorded stage.' > "$PORTICO_RUNNER_OUTPUT/native-startup.log"; fi
  fi
  docker image rm "$image" >/dev/null 2>&1 || true
}
trap cleanup EXIT
# Root's remote wrapper holds the machine slot for this entire sequential check.
docker build --network=host --label portico=desktop-r29 --tag "$image" --file apps/desktop/test/native-linux.Dockerfile .
docker image inspect "$image" --format '{{.Id}} {{json .RepoDigests}} {{json .Config.Labels}}'
# Tauri generates permission schemas beside its manifest. Only this exact per-job
# archive is writable, with the runner uid so its final cleanup can remove every file.
mkdir -p "$TMPDIR/native-work/home"
docker run --rm --network=host --user "$(id -u):$(id -g)" --label portico=desktop-r29 --cpus=2 --memory=3g --memory-swap=3g \
  --env HOME=/work/home --env CARGO_HOME=/work/cargo --env RUSTUP_TOOLCHAIN=1.99.0 \
  --env "PORTICO_NATIVE_CHECK=$scope" \
  --mount "type=bind,src=$PWD,dst=/source" --mount "type=bind,src=$TMPDIR/native-work,dst=/work" "$image" bash -c '
set -eu
export TMPDIR=/work/tmp CARGO_TARGET_DIR=/work/cargo-target CARGO_BUILD_JOBS=1
mkdir -p "$TMPDIR"
rustc --version
cargo --version
pkg-config --modversion gtk+-3.0 webkit2gtk-4.1 mpv
ffmpeg -hide_banner -version
dpkg-query -W libmpv2 libmpv-dev libwebkit2gtk-4.1-dev
export PORTICO_TEST_MPV_LIBRARY=/usr/lib/x86_64-linux-gnu/libmpv.so.2
export PORTICO_TEST_PCM_FIXTURE=/work/owned-48000-stereo.wav PORTICO_TEST_VIDEO_FIXTURE=/work/owned-vfr-6.mkv
if [ "$PORTICO_NATIVE_CHECK" != startup ]; then python3 apps/desktop/test/native-fixtures.py; fi
if [ "$PORTICO_NATIVE_CHECK" = engine ]; then
  cargo test --locked --manifest-path apps/desktop/src-tauri/engine/Cargo.toml --lib -- --test-threads=1
  cargo test --locked --manifest-path apps/desktop/src-tauri/engine/Cargo.toml --lib mpv::local_source_tests -- --ignored --test-threads=1
else
  if [ "$PORTICO_NATIVE_CHECK" != startup ]; then
  cargo test --locked --manifest-path apps/desktop/src-tauri/Cargo.toml --features custom-protocol --lib -- --test-threads=1
  if [ "$PORTICO_NATIVE_CHECK" = shell ]; then
    cargo test --locked --manifest-path apps/desktop/src-tauri/Cargo.toml --features custom-protocol --lib mpv::local_source_tests -- --ignored --test-threads=1
  fi
  cargo test --locked --manifest-path apps/desktop/src-tauri/Cargo.toml --features custom-protocol --lib shell::tests -- --ignored --test-threads=1
  fi
  cargo build --locked --manifest-path apps/desktop/src-tauri/Cargo.toml --features custom-protocol --bin portico-desktop
  if [ "$PORTICO_NATIVE_CHECK" = startup ]; then
    # Deliberately absent engine: the app must preserve native identity/host startup,
    # fail capability publication closed and retire an unattached WebView cleanly.
    export PORTICO_SPIKE_DATA=/work/startup-data PORTICO_SPIKE_REPORT=/work/startup-report.json PORTICO_SPIKE_CLOSE=1
    export PORTICO_MPV_LIBRARY=/work/no-native-library.so
    export LIBGL_ALWAYS_SOFTWARE=1 WEBKIT_DISABLE_DMABUF_RENDERER=1
    timeout 45 xvfb-run -a -s "-screen 0 1440x900x24" dbus-run-session -- gdb --batch --return-child-result \
      -ex "set print frame-arguments none" -ex run -ex "python gdb.execute(\"backtrace 64\") if gdb.selected_inferior().threads() else None" \
      --args /work/cargo-target/debug/portico-desktop
    python3 apps/desktop/test/native-startup.py
  fi
fi
'
