# Outcome

The ordinary HLS producer can now select H.264 8-bit, HEVC 8/10-bit and available software AV1; retain HDR10 or HLG; tone map on the CPU; encode multichannel AAC, AC-3, E-AC-3, Opus and FLAC; and copy receiver-compatible audio. HEVC/AV1 and lossless/Opus audio use fragmented MP4 with stable init segments and absolute fragment clocks across seeks. ASS/SSA burn-in uses embedded fonts; bitmap overlays preserve glyph shape; HDR burn-in uses a separate subtitle plane. Owner working-directory, storage-budget, background-cap and FFmpeg overrides are wired.

This is the producer implementation and selection seam. The playback-core lane still needs to choose these targets from the single capability document. It does not automatically change every existing planner decision to HEVC/HDR/FLAC.

Work is confined to the named worktree and this report. No commit, push or deployment. Snapshots 001–004 record compiling milestones. Snapshot 005 is the final cumulative snapshot; no tree changes follow it. Execution host: personal Ubuntu mini-1. Context-sync could not acquire its read-only state lock; canonical relevant guidance was read directly.

# Decisions I made

- Q-8: retain the existing confinement grants. CPU zscale/tonemap is the dependable HDR-to-SDR route. GPU filters require real qualification with the same grants as production. Detect SDR hardware scaling separately from HDR tone mapping, so missing upstream CUDA tone mapping does not disable CUDA scaling.
- HEVC hardware Main/Main10 is separately qualified on VAAPI, QSV, NVENC, VideoToolbox and AMF. Each quality mode earns a separate probe. Missing, stale or unsupported codec/depth/quality qualification selects software before launch. A failed hardware encode retries in software; a changed init retires the generation/grant while keeping the selected codec/range/layout and rechecking current ceilings.
- Dolby Vision profiles 7 and compatible 8 use their HDR10/HLG/SDR base. Base-layer copy requires the qualified RPU stripping filter and strips profile 7 enhancement units. No-base DV requires qualified libplacebo reshaping; otherwise refuse conversion rather than show a wrong picture. Existing native DV copy paths remain available.
- Original conversion can use quality mode: default quality 20 for H.264/HEVC, 28 for AV1. Software accepts a bitrate ceiling; capped hardware quality uses software. Cadence ceilings use the lesser of source rate and the requested ceiling, preserving lower-rate sources. Fixed H.264 ladders are 1.5/3/8/14/35 Mbps through 480/720/1080/1440/2160p; HEVC uses 70%, AV1 60%. These are producer defaults, not automatic changes to viewer choices.
- Uncapped quality manifests use a conservative peak bandwidth declaration. AV1 sequence level and DTS-HD subtype are not inferred from missing facts: omit the complete CODECS attribute when it cannot be described reliably. Converted H.264/HEVC profiles and levels are explicitly set and declared consistently.
- Upstream AC-3/E-AC-3 encoding supports up to 5.1 here. Preserve 7.1 E-AC-3 by copy when supported; use AAC, FLAC or Opus for other eight-channel targets. FLAC has no lossy bitrate and supports an explicit integer sample depth/rate. Unknown converted audio rate becomes 48 kHz; supported known rates remain explicit.
- Configured scratch roots contain Portico's dedicated `portico-hls` child, so cleanup cannot erase arbitrary owner files. Budget pressure first removes closed, already played segments; current/future segments and init files stay. A still-over-budget producer is stopped. The existing global disk-pressure policy remains. Init reads are bounded to 4 MiB and individual fragmented-media reads to 128 MiB; raising the session budget does not remove those bounds.
- Retain the newest 200 sanitized conversion failures/fallbacks for at most 30 days, independently of artifact cleanup. No source paths or raw command lines are retained.
- Add unmodified SIL OFL Noto CJK, Thai and Devanagari fallback fonts, with embedded licenses, alongside DejaVu. Embedded source attachment names never become paths: at most 32 generated names, 8 MiB allocation/file bound, 64 MiB accepted total, 20-second extraction deadline. Other scripts can use embedded/host fonts.
- A rejected custom FFmpeg leaves its requested setting visible and falls back to the default tools, retaining console access. It is never launched as an accepted override without qualification. The default path, protocol/source custody and process restrictions are unchanged.

# Bugs

- B-03.1 — already fixed on base; forced-IDR controls retained and extended to HEVC. Argument checks pass; vendor runtime acceptance remains.
- B-03.2 — fixed: SDR scale qualification is independent of unavailable CUDA tone mapping.
- B-03.3 — already fixed on base; real PQ/mastering fixture retained and used for native HDR filter qualification.
- B-03.4 — already fixed on base; CPU tone mapping remains first software choice. No grants widened.
- B-03.5 — already fixed on base; private remote bridge GPU requirement and sanitized error propagation retained.
- B-03.6 — already fixed on base; only a demonstrated successful software retry counts toward the hardware circuit. Source failures do not.
- B-03.7 — not present as claimed: V03 refutes the source-clock burst claim. Removed redundant conversion options; retain one final relative `prev_forced_t` cadence expression.
- B-03.8 — fixed: owner-adjustable default 4 GiB session cap, scaled file limit, played-segment reclamation before stopping.
- B-03.9 — fixed for bounded unindexed sources: streaming packet index fallback retains source keyframes. Tiny 19-second MPEG-TS indexing and repeated copied seeks pass, including decoded source-picture hash equality. A source that cannot be indexed inside the existing 20-second bound still converts.
- B-03.10 — fixed: working folder and background conversion limit are consumed; restart paths distinguish requested/effective values.
- B-03.11 — fixed: bounded history outlives conversion artifact/session cleanup; successful hardware fallback is recorded too.
- B-03.12 — fixed in command construction: discard copied-audio packets before the rebased seek origin. Apple/Cast audible preroll acceptance still requires devices.
- B-03.13 — fixed: uncapped software output also scales to even dimensions and square pixels; rotation/anamorphic sources use software decode/filtering.
- B-03.14 — fixed: prepared recipes preserve source cadence with a 60 fps ceiling instead of forcing 30; validation accepts it. All shipped recipe dimensions/rates are accepted.
- B-03.15 — already fixed on base for high-rate AAC; extended with explicit supported rates and an explicit unknown-source fallback.
- B-03.16 — fixed for converted H.264/HEVC: matching forced profile/level, display geometry and HDR range. Omit unsupported sequence/subtype guesses.
- B-03.17 — not reproduced: ordinary FFmpeg rebases decoded seek timestamps. Relative expression also remains on the source-clock linear producer. Separately fixed a real fMP4 issue: muxer offsets alone did not preserve fragment clocks; closed fragments are now shifted before publication.
- B-03.18 — fixed: bounded, cursor-based aged-orphan cleanup also runs below disk pressure, protecting live session/artifact/reservation rows and producers.
- B-03.19 — fixed configuration: linear video decode uses two threads instead of one. Actual 4K live throughput requires tuner/device acceptance.
- B-04.13 — fixed overlay geometry: aspect-preserving fit and transparent padding. Tiny cropped PGS glyph test passes; real VobSub/DVB placement remains a manual acceptance item.
- B-04.14 — fixed narrower font gap: embedded attachment extraction, fallback script coverage, portable path escaping and HDR planes. Fonts outside bundled coverage need usable attachments/host fonts.
- B-09.9 — fixed with B-03.10.
- B-02.12 — already fixed on base; existing presentation-ceiling fallback checks still pass.
- B-02.14 — producer fMP4 conversion is implemented; planner admission for a device with only fMP4 belongs to playback-core and is not yet wired here.

# API changes

- GET `/v1/console/registry`, GET and PATCH `/v1/admin/console/settings` now live on the typed apikit registry. Removed their former legacy handlers and duplicate hand-maintained OpenAPI paths/schemas; URLs and successful `{scope:{serverId,viewerFence},data}` envelope stay the same.
- Settings values add required `ffmpegPath` and `sessionStorageMiB`. PATCH is a complete, exact-name document: reject omitted/null non-pointer settings, unknown fields and duplicate JSON keys; retain owner authority, revision/idempotency and bounded admission checks.
- These routes use standard typed errors, including `invalid_request`, `revision_mismatch` with `current.revision`, `not_permitted`, `rate_limited` and `internal`. Shared console transport preserves bounded field paths/revision hints without showing server error text. Other console endpoints retain their existing contracts.
- Shared RuntimeSettings types/parser cover the complete current owner document so saving it retains all existing settings. DecisionTrace.video adds optional rotation and sampleAspectRatio; shared delivery parser/types updated. No new playback-session route or capability document.
- Internal seam: `decoder.ConversionTarget`, `ConversionRequest.Target`, `DeliveryPlan.ConversionTarget/ConversionProbe`, and `playback.StoreConversionTarget`.

# Schema changes

Edited the baseline directly and ran schema-sync; no compatibility migration.

- `playback_conversion_targets`: one bounded target/probe JSON snapshot per delivery plan, generation-fenced to playback_sessions; cascading deletion when the plan is replaced. Stores codec/depth/range, resolution/cadence, bitrate/quality, audio layout/rate/depth, packaging, DV base facts and optional mastering metadata. The seam mirrors selected output/backend fields into the delivery plan.
- `playback_conversion_failures`: independent bounded sanitized failure/fallback history. Session ID is evidence, not a cascading foreign key; cleanup cannot immediately delete the diagnostic.

# Owner or viewer settings added or wired

| Setting | Type/default | Takes effect | Screen |
|---|---|---|---|
| ffmpegPath | absolute path / empty uses defaults | restart; qualify before use, report rejected override as pending | new field; client lanes must add control |
| sessionStorageMiB | integer / 4096; 64–1048576 | next budget measurement/operation | new field; client lanes must add control |
| temporaryDirectory | absolute directory / empty uses state/hls | restart; dedicated portico-hls child when configured | existing control |
| maxBackgroundSessions | integer / 0 means no added owner cap | future prepared/background media processes; cancellable queued admission | existing control |
| hdrToneMapping | boolean / true | existing delivery planning | existing control; fresh default now agrees with registry |

New setting labels/help are in the string catalogue. Existing throttle/retention, hardware backend/device, tone curve and software preset remain wired. No viewer switch added: playback-core selects the output from the viewer's quality and capability choices.

# Needed from others

- **Playback-core:** in the same transaction that persists the plan/current generation, call `StoreConversionTarget(ctx, tx, sessionID, target, optionalQualifiedProbe)` before publishing. Choose actions/source stream/BurnInSpec on DeliveryPlan and provide accurate TraceVideo. Honor capability codec/profile/level, bit depth, HDR range, packaging, channel layout, sample rate and receiver path. Supply source DV facts and optional validated mastering-display/MaxCLL strings. A later quality/subtitle/source replan must deliberately store the new target; stale generations cannot reuse it.
- Target matrix: H.264 requires 8-bit SDR; HEVC/AV1 accept 8/10-bit SDR and 10-bit HDR10/HLG. HEVC/AV1 require fMP4. AAC/AC3/EAC3/Opus/FLAC conversion accepts mono/stereo/5.1/7.1 as encoder permits; AC3/EAC3 eight-channel conversion is refused. Copy supports AC3/EAC3/DTS/DTS-HD/TrueHD; FLAC/Opus/TrueHD require fMP4. Do not copy an unsupported receiver stream merely because FFmpeg can mux it. Preserve source rate/depth for lossless selections.
- **Playback-core:** replace the old approximate-DV warning/admission decision with the actual qualified reshape or compatible-base decision. The producer already refuses a wrong picture. Wire fMP4-only admission and Original quality mode. Rich targets currently belong to ordinary HLS; the experimental finite engine is still a separate path.
- **Client lanes:** render the two owner fields using registry bounds/application mode and the catalogue strings; preserve complete requested documents when saving, show effective/restart state, and consume typed error hints. Keep platform-specific unsupported output hidden through the single capability document.
- **Downloads/prepared lane:** current shipped prepared profiles remain SDR H.264. Cadence is fixed here, but introducing persisted HEVC/HDR/lossless prepared recipes belongs with their profile identities and download packaging.
- **Subtitles lane:** supply canonical ASS/SSA or bounded bitmap capsules and pinned source custody; DVB currently enters as a Matroska capsule. Device rendering remains preferred when possible.
- **Live TV lane:** adopt the target contract for per-viewer conversions while sharing tuner input. The linear builder's bounded decode/cadence correction is ready.
- **Documents lane:** correct architecture/product contracts and audit assumptions that all conversion is H.264 SDR/TS, that prepared output must be 30 fps, that seven-channel-plus audio always becomes 5.1 AAC, or that E-AC-3 7.1 encoding is available in upstream FFmpeg. Record D-17/D-20/D-22/D-54 and Q-8 choices above; do not promise universal DV reshaping or glyph coverage.

# Test notes for other environments

Verification here: allowed whole-server Go builds, touched-package focused tests and vet; web TypeScript no-emit; 29 console/delivery client tests. Tiny real software tests cover H.264, HEVC SDR8/HDR10/HLG10, available AV1, multichannel AAC/AC3/EAC3/Opus/FLAC, CPU HDR fixture tone mapping, ASS HDR plane, cropped PGS and HDR PGS. fMP4 init equality, independently decoded fragments and absolute seek clocks pass for H.264/HEVC/AV1/FLAC/Opus. MPEG-TS indexing/copied seeks, target readback/generation replacement, budget reclamation, history cleanup, generated font names, restart state, lazy custom-tool resolution and typed owner contracts pass. Windows/macOS cross-compilation passes; runtime acceptance is separate.

The host denies unprivileged user namespaces, so mediaexec reports its existing baseline mode. Actual bubblewrap/GPU confinement was not established here and no grants were changed. An earlier broader test selection hit `TestStorageOwnerProjectionReceiptsAndClock` failing with HTTP 400 on mount-start, outside edited mounts code; no clean-base reproduction was run. Focused owner/settings/OpenAPI tests pass. No full test suite, deployment, player session or tuner run.

For acceptance, use the same packaged/qualified FFmpeg build and production mediaexec executor, a private test library, and a 19-second clip with keyframes every four seconds (not aligned with HLS's six-second grid). Test software and each vendor with HEVC Main8, Main10 HDR10 and HLG, both bitrate and quality modes. Seek forward/back repeatedly across windows, concatenate served init+each segment, decode each independently and inspect codec/pixel format/transfer/primaries/level, timestamps and six-second cut spacing. Reject black/green/purple pictures, missing IDRs, init churn within a generation, reset timestamps, holes or audio drift.

- **NVENC / Linux and Windows:** current NVIDIA driver, supported HEVC/Main10 GPU. Confirm forced-IDR, CUDA scale qualification independently of tone mapping, local and pinned STRM source custody. Stock upstream without tonemap_cuda must still hardware-scale SDR and CPU-tone-map HDR. Repeat under real bubblewrap; inspect grants to confirm no source directories/secrets/new `/proc` authority.
- **QSV / Linux and Windows:** test one older Intel generation and one current generation. Confirm both encoder/depth probe verdicts and VPP graph, forced_idr, software fallback when Main10/quality is unsupported. Repeat interlaced, rotated and anamorphic sources; system-frame filters must not consume hardware surfaces accidentally.
- **VAAPI / Linux:** Intel and AMD render nodes, explicit owner device. Confirm HDR mastering fixture native probe, P010 HEVC10 surfaces and CPU HLG fallback without mastering metadata. An unreadable device must produce software output with accurate stages.
- **VideoToolbox / macOS:** Intel where available and Apple Silicon. Check Main10/profile metadata, bitrate and independently qualified quality mode, software ASS/bitmap upload path, native player HDR/HLG presentation and init replacement after encoder failure.
- **AMF / Windows:** supported AMD GPU/current driver, HEVC8/10 and cqp qualification. Verify forced_idr, system-frame upload, restart segments and software fallback. Windows paths with spaces, commas, apostrophes and drive colons must work for ASS/fonts/temp roots.
- **Failure circuit:** a corrupt source failing in hardware and software must not charge the GPU circuit. Three true hardware failures with successful software output in ten minutes should open it; after cooldown, qualification can recover. Repeat failure after a published fMP4 init: grant/generation changes, target range/layout stays, no old grant sees a new init.
- **Audio/receivers:** independent channel impulses for 5.1/7.1; verify mapping, clipping and downmix loudness. Compare decoded integer PCM against the source for FLAC at preserved depth/rate. Test copied AC3/EAC3/DTS/DTS-HD/TrueHD on each actual player/receiver; check Atmos preservation only for copy. Include unknown rate, 44.1/48/96 kHz, encoder priming and seek preroll on Apple/Cast. Decode ability alone does not prove receiver passthrough.
- **Subtitles:** a minimal Linux container with no host fonts, Japanese/Chinese/Korean/Thai/Devanagari text, SSA/ASS with a deliberately different embedded face, cropped PGS, VobSub and DVB, source offsets and repeated seeks. Verify line positioning and glyph shape. On an HDR display check subtitle white around 100 nits and unchanged picture highlights/colour. Repeat Windows CoreText/DirectWrite/font-provider paths and macOS; bundled fallback faces must be copied into fontsdir even if fontconfig is unused.
- **Settings/lifecycle:** save a writable SSD/RAM-backed temp root, restart, confirm only its portico-hls child is used; inspect requested/effective/restart state before/after. Lower the session cap during playback, confirm played segments reclaim first, then raise it. Restart with a qualified custom build plus sibling ffprobe; confirm scanning, extraction and conversion all use it. An invalid/missing custom build must leave the console reachable, requested path intact and effective default visible. Kill a producer/server and verify aged-orphan reclamation without pressure and without deleting live rows or source files.

Font provenance: Noto CJK repository revision `f8d157532fbfaeda587e826d4cd5b21a49186f7c` (Sans/OTF/Japanese/NotoSansCJKjp-Regular.otf, Sans/LICENSE); Google Fonts revision `66928797f271fed5ee428544e7229685e2d90127` (ofl/notosansthai and ofl/notosansdevanagari variable fonts/OFL.txt). CJK SHA256 `68a3fc98800b2a27b371f2fb79991daf3633bd89309d4ffaa6946fd587f375b5`; Thai `5a1c559bb539583c8a1fd99d1c5b9491e5e14478c9cd2bd0970d5c3096cc9ef8`; Devanagari `14ec4af41f27482216d1c2229f417ff9b1425e1babb014e57d1d40d03229853e`. Assets/licenses are embedded unmodified; no new runtime package dependency.

# Not finished

The planner/capability selection and new owner screens remain explicit handoffs, not completed user-visible features on every first-party client. GPU, real sandbox, Windows/macOS runtime, HDR-display, tuner and player/receiver acceptance have not run. Dolby Vision without a compatible base requires an available qualified reshape route; otherwise conversion is refused. Upstream E-AC-3 7.1 encoding is unavailable; copy or another output is required. Prepared/download HDR and the experimental finite delivery engine are not wired to rich targets here. Font coverage beyond the bundled scripts depends on attachments or host fonts. The unrelated storage mount test failure remains uninvestigated outside this lane.

# Merge with the base

- `console.ts`: kept complete conversion/owner settings checks and the web lane's tolerant row readers; adopted missing-title retention, connection limit/external address and API credential settings. Retired trash, mandatory-HTTPS and WAN-as-LAN fields stay removed.
- `main.go`: retained qualified custom FFmpeg/background admission and namespaced owner scratch roots; default HLS and its capacity report now use the base's cache directory. Kept the applied-directory console projection and all new identity/provider/music/API wiring.
- `httpapi/console.go`: retained the base's other console behavior; removed duplicate settings/registry handlers. `connection_settings.go` remains the owner settings registration hook, delegating to the single typed conversion-aware implementation. Settings writes refresh `connectionPolicy`, not the removed HTTPS policy.
- `httpapi/foundation.go`: kept credential/platform/webhook/MCP registrations and the conversion registry. All three conversion-owned typed routes require `ScopeAdmin`, consistent with the base scope table; the registry read is also classified in the admission-lane table.
- `operations/settings.go`: kept HDR/default storage budget, FFmpeg validation and requested/effective restart paths; adopted home-network defaults, release feed, missing-title retention and API settings. Applied-directory projection preserves the FFmpeg restart field.
- Baseline SQL: kept both conversion tables and all incoming queue/audio/API/webhook tables/triggers. Generated contracts/OpenAPI and schema files were regenerated by apigen/schema-sync, never hand-merged.

Merge checks passed: no conflict markers remain in the worktree; whole-server Linux, Windows amd64 and macOS arm64 builds; vet on cmd/server, httpapi, operations and persistence; focused settings/restart, registry/scope/admission, MCP/key/webhook, schema and startup-wiring tests; web TypeScript; and all 25 console client tests. Apigen and schema-sync completed successfully. No staging, commit or snapshot was performed; the index deliberately remains unmerged until the owner stages and commits. Context refresh still cannot write its lock; canonical guidance and all eight requested lane reports were read.
