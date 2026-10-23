#!/bin/bash
# One owned foreground Docker job. All source/artifacts are per-job; no host installs.
set -eu
cd "$(dirname "$0")/../../.."
job="desktop-r29-$(date +%s)-$$"
image="portico/$job"
cleanup(){ docker image rm "$image" >/dev/null 2>&1 || true; }
trap cleanup EXIT
# Root's remote wrapper holds the machine slot for this entire sequential check.
docker build --network=host --label portico=desktop-r29 --tag "$image" --file apps/desktop/test/native-linux.Dockerfile .
docker image inspect "$image" --format '{{.Id}} {{json .RepoDigests}} {{json .Config.Labels}}'
# Tauri generates permission schemas beside its manifest. Only this exact per-job
# archive is writable, with the runner uid so its final cleanup can remove every file.
mkdir -p "$TMPDIR/native-work/home"
scope="${1:-engine}"
case "$scope" in shell|engine) ;; *) exit 64 ;; esac
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
python3 apps/desktop/test/native-fixtures.py
if [ "$PORTICO_NATIVE_CHECK" = engine ]; then
  cargo test --locked --manifest-path apps/desktop/src-tauri/engine/Cargo.toml --lib -- --test-threads=1
  cargo test --locked --manifest-path apps/desktop/src-tauri/engine/Cargo.toml --lib mpv::local_source_tests -- --ignored --test-threads=1
else
  cargo test --locked --manifest-path apps/desktop/src-tauri/Cargo.toml --features shell --lib -- --test-threads=1
  cargo test --locked --manifest-path apps/desktop/src-tauri/Cargo.toml --features shell --lib mpv::local_source_tests -- --ignored --test-threads=1
fi
'
