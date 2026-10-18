# Outcome

Nine independent findings: **0 blocker, 2 high, 4 medium, 3 low; all nine fixed**. Rotated/unknown picture conversions obey decoder bounds; copied ten-bit H.264 and nonstandard channel counts remain usable; SDR matrices are retained; fMP4-only clients receive fMP4 audio renditions; sought bitmap burns retain both audio and subtitle timing; malformed MP4 headers fail safely; unsupported H.264 constraint guesses are omitted; the Dolby Vision fixture exercises a qualified copy route; orphan checkpoints enumerate bounded pages.

Reviewed and changed only `server-review-playback-pipeline` and this report. Starting HEAD: `1097d54`. Execution host: personal Ubuntu mini-1 (`ehler-homelab-mini-1`). No commit, push, deployment, helper or subagent. Snapshot 001 records the first compiling fixes. Final cumulative snapshot 002 closes the work after this report and final validation; no worktree changes follow it.

# Decisions I made

- Read the programme/lane briefs and all five prerequisite reports including merge/startup notes; consulted their briefs, relevant original audit/BUG-REGISTER/FIX-LOG entries, and the canonical Portico/project/verification guidance. Context-sync could not write its read-only lock; existing canonical guidance was read directly.
- A target describes copied streams as well as encoded streams. Bounded source metadata admits depths 8–16 and up to 64 audio channels, matching the device document. Actual video encoding still admits only the supported 8/10-bit outputs (H.264 eight-bit SDR); audio encoding still admits only its supported 1/2/6/8-channel layouts. No new encoder, protocol, filename, bitrate, duration or sample-rate authority was added.
- Decoder size/level limits apply to display geometry after rotation and square-pixel normalization. Missing geometry uses a conservative 1920×1080 bound; missing cadence is capped at 60 fps, narrowed by device/viewer limits. Known lower source cadence remains unchanged.
- Scaling SDR does not transform its primaries/transfer/matrix. Inherit its decoder description instead of labelling arbitrary SDR samples BT.709. HDR and actual tone-map outputs retain explicit output tags.
- Renditions inherit their containing plan's admitted transport. The derived packaging field is private (`json:"-"`) and restored after plan/target loading: no second stored/wire capability decision or compatibility route.
- Restore the bitmap subtitle demuxer origin with input-local `itsoffset`, measured from the sealed capsule. Removing global `copyts` alone was tried and rejected because it lost PGS visibility. The final correction passes cue visibility and copied/converted audio timing together. The origin probe has a 20-second deadline, 8 MiB probe/allocation bound and 1 MiB output bound; it reads only the sealed input and generated VobSub companion through the existing media executor.
- Omit optional complete CODECS when non-High/unknown H.264 profile constraint bytes cannot be described reliably; do not hard-code software encoder flags for hardware encoders. Init/elementary headers still identify the actual stream.
- Orphan maintenance consumes at most 128 directory entries per checkpoint, including recent/irrelevant entries. One serialized directory stream survives between checkpoints and closes at EOF/error or maintenance/shutdown retirement. Aging and live session/artifact/reservation/producer custody are retained. This does not claim a new library-scale capacity result or redesign the existing whole-cache budget measurement.

# Bugs

This is the pipeline review's original-bug reconciliation, not a new claim that every historical cross-client audit entry was re-qualified on physical devices.

- B-03.1 — Already fixed: forced-IDR vendor controls remain; all-vendor argument checks pass, GPU execution remains external.
- B-03.2 — Already fixed: SDR hardware scale qualification does not require a tone-map filter.
- B-03.3 — Already fixed: hardware tone-map probes use the HDR fixture; actual VAAPI/QSV qualification remains external.
- B-03.4 — Already fixed: CPU tone mapping and graph-derived GPU requirements retain the existing executor grants.
- B-03.5 — Already fixed: private bridge jobs carry hardware requirements/diagnostics; no grants widened here.
- B-03.6 — Already fixed: hardware circuit attribution requires successful uninterrupted software retry; focused attribution tests pass.
- B-03.7 — Not present as the claimed bundled-FFmpeg keyframe burst; relative live keyframe control remains. The independent global-copyts audio defect is F6, now fixed.
- B-03.8 — Already fixed: owner session cap and played-output reclamation remain; budget regression passes.
- B-03.9 — Already fixed for bounded indexable sources: packet-timeline fallback exists. Slow/unindexable sources still convert within the retained indexing deadline.
- B-03.10 — Already fixed: startup consumes scratch root, and prepared conversion admission consumes the background cap; no new setting.
- B-03.11 — Already fixed: independent bounded conversion history survives generated cleanup; regression passes.
- B-03.12 — Already fixed in main copied-audio argument construction; F6 additionally removes bitmap burns' doubled sought audio clock. Actual Apple/Cast audible acceptance remains external.
- B-03.13 — Already fixed for even/square output; F1 additionally fixes rotated/missing-fact decoder bounds.
- B-03.14 — Already fixed: prepared cadence is capped rather than forced to 30 fps.
- B-03.15 — Already fixed: supported codec/device rates govern lossy conversion and qualified lossless targets keep supported higher rates; rate regression passes.
- B-03.16 — Fixed narrower remaining gaps in F1/F7/F8: bounded output geometry/level, non-invented H.264 constraints, qualified copy-signalling fixture. Real Main/Baseline SPS, H.264/HEVC init/media and Dolby brand tests pass.
- B-03.17 — The prior ordinary-video assertion was refuted/narrowed in V03; F6 fixes a real bitmap-global-copyts clock regression introduced subsequently.
- B-03.18 — Fixed remaining claimed checkpoint gap in F9: directory enumeration/stat work is now bounded, with resumed aged cleanup and owned/recent preservation tests.
- B-03.19 — Already fixed configuration: linear decode uses two threads. Real 4K live throughput remains unverified.
- B-04.13 — Already fixed geometry: cropped/HDR bitmap shape tests pass. F6 fixes sought audio timing alongside these burns; real-disc/device placement remains external.
- B-04.14 — Already fixed implementation: generated attachment names, bounded font extraction, script fallback pack and fontsdir are present. Universal glyph/face fidelity is not claimed.
- B-04.28 — Not fixed here: analysis still uses `-xerror`; analysis stage tolerance is a scanning/derived-media handoff, outside the selected playback pipeline.
- B-04.38 — Not fixed here: the acknowledged HDR/anamorphic still-image analysis handoff is outside stream production. Do not treat playback HDR tests as thumbnail acceptance.
- B-09.9 — Already fixed with B-03.10: configured scratch root is consumed through its dedicated Portico child.
- B-02.12 — Already fixed: current/stored presentation ceilings remain applied in fallback; no ceiling policy changed.
- B-02.14 — Fixed remaining rendition transport gap F5. Real ordinary H.264/HEVC fMP4-only sessions and both AAC renditions pass; live fMP4 remains a separate publisher gap.
- B-11.15 — Already fixed shared live planning/copy admission; actual live publication/device acceptance is separate.
- B-11.16 — Already fixed: unknown live rates do not become fictitious ceiling compliance.
- B-14.4 — Already fixed current recipe/menu inventory filtering; richer persisted prepared outputs remain an explicit handoff.

# API changes

No route, URL, request/response type, key scope or schema field was added/removed. HLS payloads now advertise/serve admitted rendition packaging and omit unsupported optional CODECS guesses. Existing string-valued copied `channelLayout` can retain bounded source layouts such as quad; the client-core parser accepts those on copy actions, rejects filter punctuation and retains strict encoded layout admission. No screen change is needed for these fixes. Registry/restriction wiring is unchanged; production route-construction/pattern guards pass.

# Schema changes

None. Existing generation-fenced targets and plan JSON are used. Rendition packaging is derived from the persisted containing plan after readback; it is not another column/JSON wire field. No schema-sync or API generation was necessary because neither schema nor route contracts changed.

# Owner or viewer settings added or wired

None. Existing player/quality/owner conversion, scratch, background and storage settings keep their defaults and enforcement.

# Needed from others

- Physical decoder/display/receiver qualification on Apple, Android/Fire TV, Vega and browsers; genuine GPU/sandbox acceptance under each vendor's production executor.
- Existing declared handoffs remain: live fMP4 publisher and two-engine live presentation; rich prepared/download recipe identities/packaging; source mastering-display/MaxCLL capture; analysis decode tolerance and HDR/anamorphic previews. These are not secretly implemented by this review.
- Documentation owners should retain the distinction between source/copy facts and encoder limits, input-local subtitle origin restoration, optional omitted codec declarations, and software tests versus device qualification. Correct outdated universal H.264/SDR/stereo or global-copyts claims. Private architecture documents lie outside this writable lane.
- No client screen, route registration, database merge or string-catalogue task is required for the nine fixes.

# Test notes for other environments

Passed on Ubuntu with `GOFLAGS=-p=2`, without full suites:

- New planner regressions: rotated/anamorphic display bounds, explicit bounds with unknown source facts, audio conversion preserving ten-bit H.264, picture conversion copying quad AAC, fMP4-only audio renditions.
- New decoder regressions: bounded copied source facts versus strict encoder admission; rejection of layout filter punctuation; a real SMPTE170M FFV1→H.264 encode retains its matrix/range; a real sought PGS burn keeps copied and encoded AAC aligned. FFV1 did not retain the fixture's primaries/transfer tags, so the colour test claims only the matrix/range actually observed.
- New MP4/header, real Main/Baseline SPS and orphan-allocation regressions. Empty mdhd now returns an error instead of panic. The old Main/Baseline mismatch was 4d00/4200 versus actual 4d40/42c0. The old recent-directory checkpoint made 10,017 allocations for 2,000 entries; the bounded regression now passes its 2,000-object budget. A 300-orphan fixture resumes across checkpoints, retains an old owned directory/recent directory and closes the scan at shutdown.
- Strengthened `TestPlannerFragmentedConversionsReachTheRealProducer` passes for both H.264 and HEVC with **two AAC tracks**: generation-fenced target/transport reload, video/audio playlists and maps, stable init, independently decoded media, 12-second seek restart, prior-fragment retention and replacement-target retention.
- Focused existing inventory/device-source matrix, codec ladders, lossless-rate/passthrough, qualified Dolby Vision brand/range, audio sample rate, AC3/EAC3 seek channels, fragment clocks, budget/history, hardware fallback attribution and sweep/live-output preservation checks pass. The stale DV fixture failed with unchanged HEAD picture-bound functions too; it now supplies stripping qualification and explicitly asserts copy before retaining its original brand/range checks.
- Focused decoder software H.264/HEVC/AV1 and audio output checks, SDR/HDR metadata, vendor target arguments, ASS/PGS title-clock visibility, cropped/HDR bitmap plane tests pass.
- HTTP subtitle burn/original range/revocation, credential/library boundary, production-router construction and route-pattern compatibility checks pass (7.571 s).
- `go vet ./internal/playback ./internal/decoder` and `go build ./...` pass. Windows amd64 and Darwin arm64 touched-package builds pass on the final tree. These are compilation, not OS/device execution.
- Client-core delivery test file passes all seven tests; `npx --no-install tsc --noEmit` passes. `git diff --check` is clean.

Cold acceptance from `server/`, with the qualified FFmpeg/ffprobe pair and real production confinement:

```
PORTICO_REQUIRE_QUALIFIED_FFMPEG=1 GOFLAGS=-p=2 go test ./internal/decoder -run 'Test(SDRConversionRetainsSourceColourDescription|BitmapBurnSeekKeepsAudioAndPictureOnOneClock|BurnGraphDrawsOnlyAtTheTitleClock)$' -count=1 -v
PORTICO_REQUIRE_QUALIFIED_FFMPEG=1 GOFLAGS=-p=2 go test ./internal/playback -run 'Test(PlannerFragmentedConversionsReachTheRealProducer|H264ManifestDoesNotInventConstraintFlags|OrphanSweep.*)$' -count=1 -v
```

Both codec producer subtests must run; a missing HEVC skip is not acceptance. Repeat on Windows/macOS; use subtitle/temp paths with spaces, commas, apostrophes and drive colons. For real PGS/VobSub/DVB, seek 0→12→6 with positive/negative offsets and sources having nonzero container origins: pass means authored cue placement and uninterrupted synchronized sound. Test fMP4-only profiles with AAC/AC3/EAC3 alternate tracks: every offered track has .m4s/init and plays. Test ten-bit H.264/quad AAC on a device declaring support: copy the supported signal instead of downconverting/refusing. Verify rotated/anamorphic outputs and actual levels stay within declared dimensions; inspect SDR DVD/full-range colour samples against a trusted player.

Vendor acceptance: NVENC (Linux/Windows), QSV (Linux/Windows), VAAPI (Linux), VideoToolbox (macOS), AMF (Windows). Use HEVC Main8/Main10 HDR10/HLG, multichannel/lossless audio, ASS/image subtitles, fMP4-only capabilities and Original/fixed qualities. Seek/re-plan repeatedly; inject a hardware failure after init publication. Pass means correct codec/depth/range/layout, independently decodable cuts, retained target, replacement generation/grant when init changes, old grants unable to read new init, and no circuit charge for a source failing both encoders. Qualified optional GPU filters must operate with the unchanged source/device grants.

This host denies unprivileged user namespaces. Mediaexec used its existing reported baseline; no confinement grant or policy was weakened. GPU, real receiver/display, tuner, Windows/macOS execution and large/NAS capacity acceptance did not run. No whole test suite, native/device build, Docker build or deployment was performed.

# Not finished

No independent finding remains open. The nine lane fixes and their regressions are complete. Physical/GPU/genuine-confinement acceptance and the explicit cross-lane publisher/prepared/analysis/mastering handoffs above remain; this report does not claim their completion or whole-programme release readiness. Whole-cache budget measurement remains the existing background accounting pass; F9 bounds orphan enumeration, not all cache accounting.

# Findings (recorded before the corresponding production changes)

The following locations/evidence describe the pre-fix code and failing runs. F8 is a test-fixture finding; the others concern production behavior/robustness.

## F1 — Rotated conversions exceed the device's picture bounds
Severity: high. `server/internal/playback/selected_target.go:153` (`boundConvertedPicture`).
The height/level bounding loop uses coded source width and height while FFmpeg autorotates and normalizes pixel aspect. A 1920x1080 source rotated 90 degrees is planned at 1080x1920 for a decoder whose maximum height is 1080. The final target has Height=0. `TestConversionFitsDisplayedPictureWithinDecoderBounds/rotated` fails with those exact dimensions. Smallest fix: apply both decoder bounds to display geometry, then calculate the level from that geometry. Retain cadence and explicit viewer ceilings.

## F2 — Encoder restrictions reject streams the pipeline only copies
Severity: medium. `server/internal/decoder/targets.go:113,145` and `server/internal/playback/selected_target.go:71,110`.
Target normalization unconditionally limits H.264 to eight bits and audio to 1/2/6/8 channels, even when these streams are copied. The planner admits H.264 ten-bit plus unsupported TrueHD as audio-only conversion, but normalization rejects the final plan. Similarly VC-1 with device-supported quad AAC cannot start picture conversion. The two repository regressions above fail before fixes. Smallest fix: keep source/copy facts bounded and intact; enforce producible depths/layouts specifically at video/audio encoding admission. Preserve source layout metadata in selected targets.

## F3 — SDR conversion relabels colour samples as BT.709
Severity: medium. `server/internal/decoder/targets.go:227` (`videoColorArgs`), `hardware.go:907`.
Every SDR conversion forces BT.709 colour primaries, transfer, matrix and limited range. The SDR filter chain only scales/changes pixel format; it does not transform source primaries/transfer/matrix into BT.709. A tiny SMPTE170M FFV1 sample converted by the actual builder fails `TestSDRConversionRetainsSourceColourDescription`: output matrix is bt709. This changes the intended interpretation of SD/DVD colours and can also mislabel full-range material. Smallest fix: retain decoder colour metadata for untone-mapped SDR; explicit output tags remain for HDR and real HDR-to-SDR transformations.

## F4 — Malformed fragmented MP4 headers panic during publication
Severity: low. `server/internal/playback/mp4_boxes.go:95` (`mp4Timescales`).
The parser reads the version byte of tkhd/mdhd before checking the full-box payload exists. `TestMP4TimescalesRejectEmptyHeaders/mdhd` panics with index out of range [56] with length 56. Converter output can be truncated/malformed; supervision limits the blast radius, but the parser should return its bounded publication error. Smallest fix: validate full-box header size/version before indexing it.

## F5 — Fragmented-MP4-only clients receive MPEG-TS audio renditions
Severity: medium. `server/internal/playback/hls_budget.go:88` (`renditionPackaging`), `hls_renditions.go:57,406,548`.
Audio rendition planning validates codecs against the plan's admitted fMP4 transport, but publication selects its container solely by codec. AAC/AC3/EAC3 are then written/advertised as TS even if the client has no TS support. With two AAC tracks and fMP4-only capabilities, the regression expects .m4s but receives audio-0-000000.ts. Smallest fix: persist the admitted transport for each audio rendition and use it consistently for manifests, arguments, init/fragment names and validation.

## F6 — Bitmap burn-in puts sought audio on the wrong clock
Severity: high. `server/internal/decoder/burn_in.go:68`, `server/internal/playback/hls.go:442,518`.
Bitmap inputs add global `-copyts`. The video filter rebases only the picture, while copied/converted audio retains the sought source time; output_ts_offset adds the seek position again. The new real-media regression seeks four seconds into an FFV1/AAC sample with PGS: a two-second output contains no audio at all, for either copy or conversion, because its timestamps begin beyond the output duration. Existing bitmap clock tests map only video and miss this. Smallest fix: preserve subtitle timing without changing the main input's audio clock, verified against actual subtitle visibility and copied/converted audio after seeking.

## F7 — Main/Baseline H.264 manifests invent constraint bytes
Severity: low. `server/internal/playback/hls_renditions.go:193` (`videoCodecString`).
The codec string always inserts 00 as its H.264 constraint byte. The real SPS test fails: Main is advertised avc1.4d001f but libx264 writes avc1.4d401f; Baseline is advertised avc1.42001f but writes avc1.42c01f. Thus the claim that every converted H.264 profile's declaration matches the encoded stream is not true. Smallest sound fix: omit the optional complete CODECS declaration when these uncaptured constraint bits cannot be described reliably, as already done for unqualified AV1/DTS-HD fields. Do not guess per-vendor bytes from the software fixture.

## F8 — Dolby Vision signalling test never qualifies the copy route it asserts
Severity: low. `server/internal/playback/hls_renditions_test.go:153,173`.
The test expects the copied source's HEVC level 153 after removing native DV support, but supplies no dovi_rpu/filter_units qualification. The current planner correctly rejects base copy and converts to HEVC level 150, so the assertion fails before these changes too. Re-running with the unchanged HEAD picture-bound functions reproduces the identical hvc1.2.4.L150.B0 manifest. Smallest fix: qualify the required stripping filters in this test and explicitly assert the copy strategy, preserving all brand/range/codec assertions.

## F9 — The claimed bounded orphan checkpoint reads and stats the whole directory
Severity: medium. `server/internal/playback/hls_orphans.go:20,29`.
`os.ReadDir(h.root)` allocates the entire listing before its 128-old-directory check. Recent/irrelevant entries do not consume the counter, so a large recent backlog also triggers unbounded stat work per orphan checkpoint. `TestOrphanSweepDoesNotAllocateForTheWholeDirectory` fails with 10,017 allocations for just 2,000 recent directories (no database lookups). Smallest fix: keep a serialized directory stream between maintenance checkpoints and read at most 128 entries per call, counting all entries. Close/reset at EOF, errors and maintenance/shutdown retirement. Keep live row/reservation/producer checks and two-minute aging unchanged.

# Fixes

| Finding | Result and verification |
|---|---|
| F1 (high) | Fixed. Display geometry governs decoder width/height/level; conservative geometry/cadence covers missing facts. Rotated/unknown-bound regressions and device/source/ladders pass. |
| F2 (medium) | Fixed. Copy metadata retains depth/channel/layout; supported encoding checks stay separate. Ten-bit/quad planner, strict negative encoder/token and client parser regressions pass. |
| F3 (medium) | Fixed. Untone-mapped SDR inherits decoder colour descriptions. Actual SMPTE170M matrix/range encode passes; HDR/tone-map explicit metadata tests pass. |
| F4 (low) | Fixed. Check MP4 full-box header/version before indexing. Empty-header regression returns errors without panic. |
| F5 (medium) | Fixed. Every rendition inherits the admitted plan transport on planning/load. Both real codec producer tests pass with two independently decoded fMP4 AAC tracks. |
| F6 (high) | Fixed. Bounded sealed-capsule origin probe plus input-local itsoffset replaces global copyts. Real copied/converted audio clock and subtitle visibility/HDR plane tests pass. |
| F7 (low) | Fixed. Omit non-High/unknown H.264 declarations whose constraint bits are uncaptured. Real Main/Baseline SPS regression and manifest checks pass. |
| F8 (low) | Fixed. DV fixture explicitly qualifies stripping and asserts copy. Original brand/range/level assertions pass; the pre-fix failure was reproduced with unchanged HEAD bound functions. |
| F9 (medium) | Fixed. Stream 128 directory entries per orphan checkpoint, resume and close on retirement. Allocation, aged-cursor/live/recent preservation, shutdown and existing budget/sweep tests pass. |

# Snapshots

- 001 — Keep converted pictures within device limits and preserve copied streams.
- 002 (final cumulative for the original lane) — Keep playback audio and subtitle clocks aligned and bound orphan cleanup. Contains all nine fixes and their tests; taken after the original report/checks. The separately requested base merge below follows this snapshot.

# Merge with the base

- Read the networking, identity, API platform, providers/matching, music/audiobooks, web-app, web-player and naming reports.
- `server/internal/playback/hls_renditions_test.go`: resolved both conflicts using the base's shared `matrixToolchain` fixture, which qualifies Dolby Vision stripping. Kept the base's HDR10 delivery-strategy assertion and this lane's explicit HDR10/HLG video-copy assertions, with all codec, range and supplementary-signalling checks retained. No lane-owned routes were added; the merged registry's scope requirements apply unchanged.
- Ran `GOFLAGS=-p=2 go run ./cmd/apigen` from `server/` and `GOFLAGS=-p=2 python3 scripts/schema-sync.py` from the root. Generated contracts/OpenAPI and persistence lists already matched the merged sources; no hand edits or additional generated diff. The schema checks pass.
- All requested verification passes: `GOFLAGS=-p=2 go build ./...` on Linux, with `GOOS=windows GOARCH=amd64`, and with `GOOS=darwin GOARCH=arm64`; `go vet ./internal/playback ./internal/decoder`; all eight tests in the conflicted file plus the lane's picture-bound, copied-stream, transport, real producer, H.264 signalling and orphan regressions; decoder copied-target, SDR colour, bitmap-burn audio-clock and owner-unmapped regressions; HTTP production-router, route-pattern and credential-scope tests; `npx --no-install tsc --noEmit` in `web/`; `git diff --check`. All Go checks use `GOFLAGS=-p=2`.
- Both `rg` and recursive `grep` found no conflict markers in project files (Git metadata and dependency directories excluded). Cross-builds are compilation checks, not native runtime acceptance.
- No staging, commit or additional snapshot was performed; the index intentionally remains unmerged until Justin stages and finishes the merge commit.
