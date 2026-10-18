# Lane: conversion-outputs — what the server can produce: HEVC, HDR, tone mapping, multichannel audio, burn-in

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-conversion-outputs` (branch `audit-implementation/conversion-outputs`). This is your working directory and, with `/home/justin/projects/Portico/.worktrees/audit-implementation/_program`, the only place you can write.
**You cannot commit** (the sandbox keeps git metadata read-only). At each milestone, when the tree compiles, run from the worktree root:
`bash /home/justin/projects/Portico/.worktrees/audit-implementation/_program/snap.sh conversion-outputs "<one-line commit subject in plain language>"`
Snapshots are cumulative; each becomes one commit. Take one at least every hour of work and always one at the very end, after which you must not change the tree. Go environment: `GOCACHE=/home/justin/projects/Portico/.worktrees/audit-implementation/_program/gocache` and `GOFLAGS=-p=2` are already set.
Audit documents: 03 (all), 02 and 20 (output entries: A-02.9, A-03.6, A-03.7, A-20.9), 04 (burn-in entries); verification V03, V02. Decision register D-17, D-54, D-20, D-22. Open question Q-8 in `FIX-LOG-2026-10-12.md` and `fix-pass/lane-reports/report-hardware-conversion.md`.

Today every conversion is H.264 8-bit SDR in MPEG-TS at a fixed ladder bitrate, and DTS-HD or TrueHD usually becomes 384 kbps AAC. Justin wants Plex-level "it just plays", keeping quality wherever the device can take it.

## What to deliver
1. **Video targets**: H.264 (8-bit), HEVC 8-bit and 10-bit, AV1 where an encoder exists. **HDR preserved** (HDR10 and HLG with correct colour metadata) when the device supports it; **tone mapped to SDR** when it does not, on the CPU always and on the GPU where it is sound inside the sandbox (settle Q-8). Dolby Vision sources: keep what can be kept (the HDR10 base layer of compatible profiles), tone map the rest, never produce a wrong picture. Sensible bitrate ladders per codec and resolution, and a quality-targeted mode for "Original quality but different codec".
2. **Packaging**: HEVC and AV1 in HLS need fragmented MP4 (an init segment and `.m4s` parts); keep MPEG-TS only where it is the right choice. Playlists, segment naming, and serving for both.
3. **Audio targets**: stereo and 5.1 AAC, AC-3, E-AC-3 (5.1 and 7.1 where possible), Opus, and **lossless** (FLAC) for devices that take it; **copy** (passthrough) of AC-3, E-AC-3, DTS, DTS-HD and TrueHD in a remux when the device or its receiver accepts it. Correct channel layouts and downmix. An unknown source rate must still give a rate the device takes.
4. **Hardware**: HEVC encoders for VAAPI, QSV, NVENC, VideoToolbox and AMF alongside the H.264 ones, with the hardware keyframe and probe problems fixed (B-03.1, B-03.3) and a clean fall back to software. All untested on a GPU here: write exact test notes per vendor.
5. **Burn-in at high quality**: ASS and SSA with their embedded fonts through libass, PGS, VobSub and DVB as an overlay, correct for HDR sources, positioned and scaled properly. It must work on Windows too.
6. **A target description rich enough for all of this**: extend the type the planner hands you (video codec, bit depth, dynamic range and tone-map flag, resolution, bitrate or quality, audio codec, channels, container/packaging, burn-in track). Make the pipeline honour every field.
7. **Owner settings wired**: the conversion temp folder saves and does nothing (B-03.10 / B-09.9), wire it; the 4 GiB per-session cap (B-03.8) becomes a default the owner can raise; the owner may point Portico at another FFmpeg build (D-54), validated before use and still run inside the sandbox.
8. Every other confirmed bug in document 03 that is still present.

## Boundaries
- You own FFmpeg command construction, encoder and hardware selection, the conversion session's files, HLS packaging and burn-in. The **playback-core** lane owns the capability document, the planner's decision and session routes, and is reworking them now; do not edit those beyond the minimum to compile. A later lane wires the planner to your new targets, so make them selectable through the target description alone and document each one.
- The **subtitles-text** lane owns subtitle parsing, extraction and delivery to clients. The **livetv** lane owns live input and recordings.
- FFmpeg always runs inside the existing sandbox. Do not relax it; if a filter needs something the sandbox denies, say so.
- Test command builders with argument-level unit tests. Run real FFmpeg only on tiny generated samples and only for the software paths.
