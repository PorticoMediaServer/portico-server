# GPU conversion acceptance

## Purpose and evidence

Updated for the evening implementation and owner decisions. New cases remain qualification work; known missing candidate implementation is Fail, while an unavailable test prerequisite is Blocked. Native-owner recorded 655 native/87 Vega before the mirror; the later running log confirms 655/87 again after the mirror with clean type checks and equal packages. Physical acceptance remains unrun.

These are qualification instructions, not evidence that the candidate passed. Use disposable state, copied media and test accounts. Record candidate commit IDs, artifact SHA-256, OS/device/driver/browser versions, start/end times, redacted server/client logs and the numbered case result: Pass, Fail or Blocked. A missing prerequisite is Blocked, never Pass. Save raw credentials, grant URLs, cookies, private keys and unredacted evidence outside Git.

The source candidates are the integrated `audit-implementation/main` checkouts at `/home/justin/projects/Portico/.worktrees/audit-implementation/main-server` and `main-native`; internal Hosted work is delivered separately. On the qualification machine use local copies of those exact integrated revisions and record `git rev-parse HEAD` plus whether the tree is dirty. Do not test an individual lane branch without its dependencies. No stable pre-release API/database compatibility is promised: fresh fixtures use the candidate baseline; upgrade fixtures must have the same baseline digest. Preserve the entire old state separately before any restore/upgrade.

Use a reachable private test origin, normally `http://<host>:32500`, and a trusted HTTPS origin for TLS cases. Mini-1 previews use `100.120.50.24` and the actual listening port; use Tailscale for access/transfers. Never substitute a production/demo server for destructive, conversion or load cases. Open `/`, complete first-run setup with a disposable direct owner, then create two member accounts, two profiles on one account and one profile restricted to a separate library. Make Movies, TV, Anime, Music and Audiobooks libraries in separate writable fixture roots. Owner settings are under Settings > Server; viewer settings are under Settings. Keep a second signed-in device/browser to observe changes. Unless a case chooses otherwise, quality is Original and player is Automatic.

## What is needed

Run the common cases for software plus each available vendor. Intel: Linux/Windows QSV and Linux VAAPI. AMD: Linux VAAPI and Windows AMF. NVIDIA: Linux/Windows NVENC/CUDA. Apple: macOS VideoToolbox. Record unavailable vendors as Blocked. Hardware, HDR display and real surround receiver are required; argument/unit tests do not qualify them.

## Media and accounts

For ordinary playback, make synthetic samples with the candidate's qualified FFmpeg/ffprobe pair. Run in a new writable fixture folder (never over existing media):

```sh
ffmpeg -f lavfi -i testsrc2=size=1280x720:rate=24 -f lavfi -i sine=frequency=440:sample_rate=48000 -t 19 -c:v libx264 -g 96 -keyint_min 96 -sc_threshold 0 -c:a aac -movflags +faststart 'Fixture (2026).mp4'
ffmpeg -i 'Fixture (2026).mp4' -c copy 'Fixture (2026).mkv'
ffmpeg -f lavfi -i sine=frequency=440:sample_rate=44100 -t 120 -c:a flac -metadata artist='Fixture Artist' -metadata album='Fixture Album' -metadata title='Track 01' -metadata track=1 'Track 01.flac'
```

Make three TV copies named `Fixture Show/Season 1/Fixture Show - S01E01.mkv` through `S01E03.mkv`; make a second version/edition copy and a two-part film. Write a UTF-8 sidecar `Fixture (2026).en.srt` containing `1`, `00:00:02,000 --> 00:00:05,000`, and `Fixture caption`, separated by newlines and followed by a blank line. For long buffering/background/download cases generate a 20-minute/2+GB synthetic file by raising duration/bitrate and verify its hash before testing; a 19-second clip cannot establish those cases.

For HDR10/HLG/Dolby Vision 5/7/8, TrueHD/DTS-HD/Atmos, PGS/VobSub/DVB, HE-AAC/SBR and attached-font ASS use legally held test clips/disc or broadcast extracts. Log provenance, ffprobe facts and a correctly rendered reference frame/audio/channel map. SDR bars with HDR tags are not HDR-fidelity evidence. The server checkout has `fixtures/audio-gapless/` and `fixtures/audio-gapless/generate-portable.py`; preserve its original corpus and generate extras with `python3 fixtures/audio-gapless/generate-portable.py --out /tmp/portico-audio-extra`. Obtain measured HE-AAC/PS fixtures separately. Missing real formats block their cases. Make bad-input copies separately: truncate a copy, corrupt a metadata row, or insert a malformed subtitle cue between good cues. Record which data changed.

## Build or install

Install the actual candidate package/image on the target OS, or use an isolated candidate server build with PORTICO_STATE_DIR and PORTICO_WEB_DIR pointing at disposable state and its matching web. Use the bundled qualified FFmpeg/ffprobe through the production media executor. In Server > Playback select the hardware backend/device, restart when requested and save the hardware/probe report. An installed driver or an encoder name alone is insufficient. On Linux use real bubblewrap namespaces and test PORTICO_DECODER_SANDBOX=required; baseline fallback is recorded separately. Permission failures need narrow render/video/device access. Missing/failed capability probes must fall back honestly; never remove confinement/signature checks. Keep conversions off the demo VPS.

Make a 19-second source with four-second GOP using the sample recipe above and another with ten-second source GOP (`-g 240 -keyint_min 240` for 24 fps). Use actual HEVC Main8, Main10 HDR10/HLG, interlaced MPEG-2/AC-3, rotated/anamorphic SDR/DVD/full-range and 96 kHz/24-bit 7.1 lossless clips. Use a drawing and non-drawing subtitle client; a fMP4-only test profile is exercised through third-party-api.md. Save master/media playlists, init and four media fragments privately; grant URLs must be redacted in shared evidence.

## Smoke subset

Run cases 1, 2, 7, 8, 9, 10 first, in order. Stop a destructive case if isolation is lost; keep running independent cases. Smoke success establishes a starting point; all remaining cases still need results.

## Test cases

### 1. Qualify the production executor before comparing output — smoke

1. Record GPU/driver/tool bundle identity and server hardware report. Start an ordinary software and a forced 720p hardware playback through the normal API/player.
2. Repeat under genuinely required confinement with a local source and a pinned HTTP STRM source requiring ASS/PGS burn-in. Inspect sandbox/process grants and hardware stages.

**Expected:** Actual hardware output, correct backend/device and explicit encode/decode/filter probe facts; permitted remote burns succeed. Normal net/input/output/device grants remain bounded, CPU-only graphs have no GPU authority.

**Failure:** Reported availability without executed hardware, source directories/secrets exposed, --unshare-net missing for bridge jobs or required confinement bypassed.

**Route to:** conversion-outputs, planner-wiring; fix-pass hardware-conversion.

### 2. Segment boundaries, clocks and fMP4 generations — smoke

1. Play HEVC8/10 in Original then 1080p; seek 0→12→6→18, choose 720p then Original. Do both quality and bitrate modes and two AAC audio tracks on fMP4-only capabilities.
2. For TS inspect first video packet with `ffprobe -v error -select_streams v:0 -show_packets -show_entries packet=pts_time,flags -of csv=p=0 SEGMENT`; decode each alone with `ffmpeg -v error -i SEGMENT -f null -`. For fMP4 concatenate its init plus each individual .m4s and decode each combined file.
3. Record codec/pixel format/primaries/transfer/level/range/audio/timestamps using ffprobe. Fail the selected hardware after its first published init, allow software fallback and retry the old grant.

**Expected:** Six-second cuts near 0/6/12/18 within a frame (allow measured mux offset), key packets/independent decode; absolute seek clocks, stable init in one generation and all audio renditions have the admitted transport. Changed init gets replacement generation/grant; old grant cannot see new init; selected range/audio/layout survives fallback.

**Failure:** Missing IDR, hole/reset timestamps, reference decode errors, .ts audio on fMP4-only plan, init churn or old grant reads new bytes.

**Route to:** conversion-outputs, planner-wiring, review-playback-pipeline; fix-pass hardware-conversion.

### 3. HDR, SDR colour, geometry and bitmap timing

1. Compare HDR10/HLG output on HDR and SDR displays; disable tone mapping once in a separate owner-choice run. Compare rotated/anamorphic/DVD/full-range output against a trusted player.
2. Burn ASS with fonts and PGS/VobSub/DVB on nonzero-container-origin media; seek 0→12→6 with timing ±0.5 seconds and both copied/encoded audio.
3. Try DV5/7/8 with and without a qualified reshape/base-copy route; distinguish discarded enhancement-layer fidelity from correct base colour.

**Expected:** Supported HEVC10/HDR metadata retained, SDR tone mapping sensible, output bounds/level fit displayed square-pixel geometry; cue placement/audio remain synchronized after seeks. Owner tone-mapping-off may show washed-out colours but plays. Unsupported DV route refuses honestly without permission wording or guessed colour claims.

**Failure:** Green/purple/black frames, relabeled untransformed SDR, lost subtitle rows/audio after seek, decoder dimensions exceeded or enhancement-layer fidelity claimed without proof.

**Route to:** conversion-outputs, planner-wiring, review-playback-pipeline, subtitles-text, leftovers-server-a, base-health.

### 4. Audio layout, rate, depth and real receivers

1. Use independent speaker impulses for 5.1/7.1. Play copied AC3/EAC3/DTS/DTS-HD/TrueHD into the actual receiver; switch to stereo/Bluetooth and force-stereo/disable-passthrough controls.
2. Convert 44.1/48/96 kHz and unknown-rate sources; inspect each output. Compare decoded integer PCM for lossless output with preserved source rate/depth. Seek and listen for preroll/priming, especially Apple/Cast.
3. Check Atmos only with a copied stream; test unsupported E-AC3 7.1 encoding with available alternatives.

**Expected:** Correct channel order/no clipping, declared receiver bitstream or documented PCM, supported lossless rates/depth retained; lossy output uses supported target rates (normally 48k for high-rate AAC, 44.1k remains when supported). No invented E-AC3 7.1 encoder; copy/other target or honest refusal.

**Failure:** Silent stereo fallback, wrong centre/rear channels, invalid high-rate AAC or Atmos claim on converted audio.

**Route to:** playback-core, conversion-outputs, planner-wiring, audio-engines; fix-pass catalogue-playback.

### 5. Hardware failure attribution and cooldown

1. Use three corrupt but probe-admitted sources that fail both hardware and software; record both attempts and play a valid control clip afterward.
2. On three valid short clips make driver unavailable after qualification using a qualification-only wrapper rejecting that hardware encoder and delegating software unchanged; let software finish. Repeat one canceled retry and a long throttled/canceled retry.
3. After the ten-minute cooldown restore hardware and request re-probe. Capture artifact diagnostics from a deliberate unavailable filter, redacting paths/tokens.

**Expected:** Bad sources and canceled/unhealthy retries do not charge the circuit. Three true hardware failures with healthy software recovery within the window select software afterward; cooldown/requalification can restore hardware. Diagnostic phrase/class survives without private paths/bridge tokens.

**Failure:** Source errors blacklist GPU, canceled retry counts, recovery never re-probes, filter error swallowed or credentials in diagnostics.

**Route to:** conversion-outputs, planner-wiring, review-playback-pipeline; fix-pass hardware-conversion.

### 6. Finite resources and orphan cleanup

1. Fill conversion slots/cache within owner limits while browsing and opening console. Cancel/replan/sweep and keep a live output active.
2. Seed many recent/old output folders only in disposable cache; trigger normal maintenance and monitor work/hold durations.

**Expected:** Budget includes whole cache through accounting, orphan enumeration advances in bounded checkpoints, active/current/recent outputs retained and ordinary UI stays reachable.

**Failure:** Stop-the-world enumeration/reclamation, live output deleted, conversion limits ignored or foreground stalls.

**Route to:** conversion-outputs, limits-settings, review-playback-pipeline.

## Intel

### 7. Intel — QSV and VAAPI — smoke

1. Select QSV then Linux VAAPI on the explicit render node; repeat common cases with HEVC8/10, quality/bitrate, interlacing, rotation and anamorphic samples.
2. Inspect hevc_qsv forced-IDR/global_quality, P010/VPP and hevc_vaapi qp/metadata. Try unsupported Main10/quality and remove device access; test HLG without mastering metadata.

**Expected:** Qualified native hardware filters operate on proper surfaces; unsupported depth/quality/native HDR filters use accurate encode-only/software fallback. No software-frame filter accidentally consumes hardware surfaces.

**Failure:** Device mis-selected, fake nativeFilters, broken VPP graph, wrong mastering assumptions or failed unsupported-quality recovery.

**Route to:** conversion-outputs, planner-wiring, review-playback-pipeline; fix-pass hardware-conversion.

## AMD

### 8. AMD — VAAPI and AMF — smoke

1. On Linux select explicit AMD VAAPI and repeat native HDR/SDR/filter cases; remove device read access.
2. On Windows select AMF; repeat common seeks/segments/subtitle paths with spaces, commas, apostrophes and drive colons. Inspect hevc_amf qp_i/forced_idr/upload and cqp qualification.

**Expected:** Qualified VAAPI/AMF depth/quality/IDRs and native filter verdicts agree with actual output; unreadable/unqualified devices fall back and subtitle upload remains correct.

**Failure:** Invented AMF/VAAPI availability, missing IDRs, P010 mismatch, escaped Windows filter path or black burn-in.

**Route to:** conversion-outputs, planner-wiring, review-playback-pipeline; fix-pass hardware-conversion.

## NVIDIA

### 9. NVIDIA — NVENC and CUDA — smoke

1. Select NVENC and repeat common output/seek/failure cases; inspect hevc_nvenc cq/forced_idr and separately qualify CUDA scale.
2. Use stock build without tonemap_cuda: SDR hardware scale must still work and HDR must use qualified alternative/CPU tone-map. Try capped quality where hardware cannot enforce it.
3. With software encoder selected test Vulkan/libplacebo and OpenCL-only tone map under required confinement, including unavailable GPU filters and DV5.

**Expected:** NVENC works without extra source/proc authority; scaling does not imply tone-map qualification; unsupported capped quality uses software. GPU-assisted software graphs receive only normal necessary grants; inaccessible filters use CPU or honest DV refusal/warning.

**Failure:** CUDA scale mistaken for HDR support, device initialization failure instead of bounded fallback, new /proc/source grants or DV colour falsely accepted.

**Route to:** conversion-outputs, planner-wiring, review-playback-pipeline; fix-pass hardware-conversion.

## Apple

### 10. Apple — VideoToolbox — smoke

1. Select VideoToolbox and repeat HEVC8/10 bitrate plus independently qualified quality modes, HDR10/HLG and subtitles with software-plane upload.
2. Inspect hevc_videotoolbox profile/tags, actual native HDR display and fMP4 init after hardware fails post-publication.

**Expected:** Backend probe and native device presentation agree; bitrate/quality and Main10 metadata correct; independent segments and fallback preserve target/generation rules.

**Failure:** Quality declared without qualification, wrong HEVC tag/profile, lost HDR or stale init reused after fallback.

**Route to:** conversion-outputs, planner-wiring, review-playback-pipeline; fix-pass hardware-conversion.
