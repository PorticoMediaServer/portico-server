This is an unqualified WIP packet-level libdovi-backed profile 7 to 8.1 BSF.
It is a separate post-series patch for Jellyfin FFmpeg v8.1.3-1 after all 100 upstream patches. No upstream manifest is modified.

Dependency: quietvoid/dovi_tool commit 26ed61ff36f946313889a09c0d2da202e9ceb97a, dolby_vision crate 3.4.0, MIT. The C ABI is produced by cargo-c with the capi feature, pkg-config dovi, header libdovi/rpu_parser.h. Intended build: cargo cinstall --manifest-path dolby_vision/Cargo.toml --features capi --release --library-type staticlib --prefix PREFIX; FFmpeg --enable-libdovi. Exact cargo-c installation flags remain to be qualified.

The Rust static library compiled locally. The FFmpeg patch has NOT been compiled or packet-tested. Do not consume this checkpoint as a checked feature contract. The filter deliberately discards enhancement-layer NALs; FEL residual reconstruction is not provided. Server target/option qualification integration remains incomplete.

Heavy builds stopped per Justin/Claude resource steer. Logs and source preserved in assigned video-server-r29 evidence. Resume only after Root confirms admission thresholds.
