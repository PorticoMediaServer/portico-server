This is an unqualified packet-level libdovi-backed profile 7 to 8.1 BSF draft.
It is a separate post-series patch for Jellyfin FFmpeg v8.1.3-1 after all 100 upstream patches. No upstream manifest is modified.

Dependency: quietvoid/dovi_tool commit 26ed61ff36f946313889a09c0d2da202e9ceb97a, dolby_vision crate 3.4.0, MIT. Exact archive and licence hashes are in portico-libdovi-source.json. The C ABI is produced by cargo-c 0.10.20 with capi, pkg-config dovi, header libdovi/rpu_parser.h. Intended build: cargo cinstall --locked --manifest-path dolby_vision/Cargo.toml --features capi --release --library-type staticlib --prefix PREFIX. This exact C ABI installation and the FFmpeg patch remain under qualification.

Procedural metadata fixtures in fixtures/video/dovi-metadata cover profiles 5, 8.1, 8.4, profile 7 MEL and profile 7 FEL metadata, each with CM 2.9 and 4.0. All serialized RPUs reparse with CRC/profile/CM/enhancement checks; P7 metadata converts through the Rust library to P8 without residual dependency. A corrupt-CRC negative is rejected. Byte-identical locked repeats passed. This does not qualify the C ABI or FFmpeg BSF, a BL/EL video join, mastered FEL imagery, or any target binary.

The filter deliberately discards enhancement-layer NALs; FEL residual reconstruction is not provided. Server target/option qualification integration remains incomplete. Do not consume this WIP patch as a checked runtime feature contract. Normal bounded qualification is active; the former heavy-work pause is retired.
