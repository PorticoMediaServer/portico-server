# Development conformance only; this distribution mpv is not the release FFmpeg8.1.3 bundle.
# The checked package lock/toolchain receipt and physical-output qualification remain separate.
FROM rust:1.99.0-slim-bookworm AS rust-toolchain
FROM ubuntu:26.04
LABEL portico="desktop-r29"
ENV DEBIAN_FRONTEND=noninteractive \
    CARGO_HOME=/opt/portico/cargo RUSTUP_HOME=/opt/portico/rustup \
    PATH=/opt/portico/cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
COPY --from=rust-toolchain /usr/local/cargo /opt/portico/cargo
COPY --from=rust-toolchain /usr/local/rustup /opt/portico/rustup
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential pkg-config ca-certificates python3 curl \
    libwebkit2gtk-4.1-dev libgtk-3-dev libmpv-dev ffmpeg \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /source
