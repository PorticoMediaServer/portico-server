#!/bin/bash
# Headless native persistence check. No GTK, audio device, WebView or media SDK.
set -eu
cd "$(dirname "$0")/../../.."
mkdir -p "$TMPDIR/storage-work"
docker run --rm --network=host --user "$(id -u):$(id -g)" --label portico=desktop-r29 --cpus=1 --memory=1536m --memory-swap=1536m \
  --env CARGO_HOME=/work/cargo --env CARGO_TARGET_DIR=/work/target --env CARGO_BUILD_JOBS=1 --env TMPDIR=/work/tmp \
  --mount "type=bind,src=$PWD,dst=/source,readonly" --mount "type=bind,src=$TMPDIR/storage-work,dst=/work" \
  rust:1.99.0-slim-bookworm@sha256:2c3a22f0a5533ea2dd5a16627bc841228151faa2d4de2644ac9987e4a2f1f2fa \
  bash -c 'set -eu; mkdir -p "$TMPDIR"; cd /source; rustc --version; cargo test --locked --manifest-path apps/desktop/src-tauri/storage/Cargo.toml --lib -- --test-threads=1'
