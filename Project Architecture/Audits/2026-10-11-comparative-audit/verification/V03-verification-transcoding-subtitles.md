# V03 — Verification of transcoding, subtitles, tracks, markers and trickplay

## Summary

All **25 Tier 1** and **20 Tier 2** entries were reached. Tier 1: **1 Confirmed, 19 Confirmed, narrower, 1 Refuted, 4 Unsettled — needs runtime**. Tier 2: **7 Confirmed, 13 Confirmed, narrower**, including three promoted detailed checks. Most mechanisms in the first pass are present, but several broad consequences are conditional, and explicit capacity/fidelity/selection policies should be treated as trade-offs rather than accidental defects. This is independent static verification, not runtime reproduction.

The most consequential confirmations are the software-HDR/GPU sandbox mismatch (B-03.4), the native-filter capability false negatives (B-03.2/B-03.3), the sidecar resource gap and scan import coupling (B-04.1/B-04.3/B-04.4), the stale Windows embedded-extraction gate (B-04.31), and the false WebVTT presentation contract (B-04.36). A newly traced **Medium defect, N-V03.1**, disables even small bitmap sidecars through mismatched reader limits. Existing first-pass B-14.4 already covers the four unusable prepared quality rungs encountered while verifying B-03.14; it is not counted as new.

**B-03.7 is refuted for the pinned FFmpeg 8.1.2 bundle**: expression `t` is relative to the first encoded frame, so the duplicate keyframe option does not produce the claimed mid-file burst. B-03.8’s 8 Mb/s/one-hour storage example is also invalid, although the high-bitrate retention conflict remains. B-03.5’s reachable remote conversion is specifically requested burn-in; ordinary STRM delivery takes an early return. Unapproved heuristic markers reaching manual prompts are intentional and tested (B-04.21); automatic skipping remains fenced. The promised cross-episode detector is nevertheless a genuine owner-facing documentation error (B-04.40).

## Verdict table

First-pass confidence/severity below is taken from the supplied worklist; ranges are preserved. Size: **S = hours, M = days, L = a week or more**. Severity assigned to a trade-off or unresolved risk describes its affected-user consequence, not a recommendation to abandon the deliberate policy. “No” means no direct item in the current Backlog; related work is called out explicitly.

| ID | Claim (short) | First pass (confidence / severity) | Verdict | Class | Your severity | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-03.1 | NVIDIA, Quick Sync and AMD encodes probably do not cut on the six-second grid | Probable / High | Confirmed, narrower | Defect | High | M | No |
| B-03.3 | The VA-API "native" probe very likely fails on every host | Probable / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-03.4 | Software HDR conversions fail inside the Linux sandbox when Vulkan (or OpenCL) was detected | Probable / High | Confirmed, narrower | Defect | High | M | No |
| B-03.5 | Remote-source conversions get no GPU access and no error text | Probable / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-03.7 | Live/Library-Channel conversions: the safe keyframe expression is overridden by the unsafe one | Confirmed by reading / Medium | Refuted | Design risk | Low | S | No |
| B-03.8 | The fixed 4 GiB per-session ceiling fights the owner's retention setting | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-03.9 | MPEG-TS and other non-indexed sources can probably never be direct-streamed | Probable / Medium | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-03.12 | Converted video with copied audio: audio from before the seek point is included | Speculative / Low–Medium | Unsettled — needs runtime | Design risk | Low | M | No |
| B-03.15 | Converted audio keeps a 96 kHz (or 88.2 kHz) sample rate | Confirmed by reading / Low–Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-03.19 | Performance risk: 4K live channels decoded on one thread | Speculative / Low–Medium | Unsettled — needs runtime | Design risk | Medium | S | No |
| B-04.1 | Sidecar subtitle files are never imported automatically, so they are never auto-selected | Confirmed by reading / High | Confirmed, narrower | Unfinished feature | High | M | No (related Cast gap only) |
| B-04.4 | The 90-second budget covers up to eight full reads of the file | Probable / High | Confirmed, narrower | Defect | High | M | No |
| B-04.9 | Non-Western legacy encodings are shown as garbage, silently | Confirmed by reading / Medium | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-04.10 | One odd line makes a whole ASS script unusable | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-04.11 | Image subtitle tracks over 32 MiB cannot be used | Probable / Medium–High | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-04.12 | First use of an image track must read the whole file inside a 110-second request | Probable / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-04.13 | Burned image subtitles may be squashed or clipped when the video is not the subtitle's shape | Speculative / Medium | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-04.14 | Burned-in styled subtitles can lose their glyphs or fonts | Probable / Medium | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-04.20 | The OpenSubtitles sign-in is a static environment variable | Probable / Low–Medium | Confirmed, narrower | Unfinished feature | Low | M | No |
| B-04.23 | One imperfect chapter discards all chapters | Confirmed by reading / Low–Medium | Confirmed, narrower | Deliberate trade-off | Low | S | No |
| B-04.27 | Flat folders with more than 4,096 entries lose all sidecars, and discovery re-lists the folder for every video | Confirmed by reading / Medium | Confirmed, narrower | Design risk | Medium | M | Related metadata cache item only |
| B-04.28 | Any decode error aborts an analysis stage | Probable / Low–Medium | Confirmed, narrower | Deliberate trade-off | Low | S | No |
| B-04.31 | On a Windows server no embedded subtitle can be used | Confirmed by reading / High | Confirmed, narrower | Defect | High | S | No |
| B-04.38 | HDR and anamorphic sources produce poor preview images | Probable / Low–Medium | Confirmed, narrower | Defect | Low | M | No |
| B-04.40 | The owner-facing description of segment detection does not match what runs | Not stated / Not stated | Confirmed | Documentation error | Medium | S | No |
| B-03.2 | The NVIDIA "native" path can never be enabled with Portico's own FFmpeg | Confirmed by reading / Medium | Confirmed | Unfinished feature | Medium | M | No |
| B-03.6 | The hardware circuit breaker cannot tell a bad GPU from a bad file | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-03.10 | The temporaryDirectory and maxBackgroundSessions owner settings do nothing | Confirmed by reading / Medium | Confirmed | Unfinished feature | Medium | M | No |
| B-03.11 | A failed conversion's diagnostic is deleted almost immediately | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-03.14 | Prepared versions are forced to 30 fps | Confirmed by reading / Medium | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-04.2 | The scan imports an arbitrary eight text tracks, not the useful ones | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-04.3 | One bad or empty text track blocks the rest of the file's subtitles and all deep analysis | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-04.5 | A file with more than 32 subtitle tracks gets none | Confirmed by reading / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-04.6 | Common sidecar names are disabled as "ambiguous" | Confirmed by reading / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-04.7 | A second video with the same base name disables every sidecar for both | Confirmed by reading / Low–Medium | Confirmed | Deliberate trade-off | Low | S | Related version-grouping item only |
| B-04.15 | Audio language preferences fail for languages outside a 57-entry table | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-04.16 | Forced-track matching uses a second, smaller table | Confirmed by reading / Low–Medium | Confirmed | Defect | Medium | S | No |
| B-04.18 | Forced tracks with no language tag are never auto-selected; file "default" flags are ignored | Confirmed by reading / Low–Medium | Confirmed, narrower | Deliberate trade-off | Low | S | No |
| B-04.19 | Online search for an episode sends only the episode's title | Confirmed by reading / Medium | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-04.21 | Unreviewed luminance guesses are shown to viewers as Skip buttons, and the intro guess is often backwards | Confirmed by reading / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-04.24 | A title-page subtitle choice can silently do nothing | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-04.32 | Subtitle reads and a viewer's subtitle selection queue for the single write slot at background priority | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-04.35 | "Complete" scanning is far heavier than its label suggests (design risk) | Confirmed by reading / Medium | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-04.36 | The v1 session says "webvtt" but the URL returns JSON | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-04.37 | HLS subtitle renditions carry no default, auto-select or forced signalling (design risk) | Confirmed by reading / Low–Medium | Confirmed, narrower | Unfinished feature | Medium | M | No |

## Tier 1 — detailed verification

### B-03.1 — NVIDIA, Quick Sync and AMD encodes probably do not cut on the six-second grid
- **Claim:** Hardware encoders are forced to make intra frames on the six-second grid without explicitly requesting IDR frames.
- **What I traced:** apps/portico-server/server/cmd/server/main.go:664–670 wires ordinary HLS by default. apps/portico-server/server/internal/playback/delivery_plan.go:795–837 calls BuildConversion; apps/portico-server/server/internal/decoder/hardware.go:722–731 sets only the generic expression; apps/portico-server/server/internal/playback/hls.go:476–480 uses the keyframe-gated HLS muxer; apps/portico-server/server/internal/playback/hls_timeline.go:65–81 publishes a fixed grid. I read the pinned upstream FFmpeg n8.1.2 sources, not only Jellyfin: nvenc.c:2786–2790 and :3177–3179; qsvenc.c:2481–2484 and :2668–2674; amfenc.c:260–265 and :505–512; hlsenc.c:2459–2460 and :2485–2487.
- **Verdict:** **Confirmed, narrower.** The missing IDR controls and muxer mismatch are supported by upstream source. NVENC/AMF force plain I pictures by default and only mark IDR output as key; QSV also requests IDR when the input frame already has its key flag, so some source-keyframe coincidences can work. The first pass overstates a universal 250-frame GOP and a simple one-GOP-per-segment count: defaults/drivers vary and HLS waits until its duration threshold as well as a key packet. The timing risk remains real; no GPU encode was run.
- **Class:** Defect.
- **Severity:** **High.** Affected NVENC, QSV and AMF conversions can publish incorrect VOD segment timing and break seeking; the device-visible failure has not been reproduced.
- **Fix direction:** Add encoder-specific IDR policy in apps/portico-server/server/internal/decoder/hardware.go, then qualify grid boundaries and independent decodability with a 19-second non-grid-source clip per available backend. Touch decoder probes/tests and playback HLS timeline validation. (**Size:** M; main packages/files are named above.)
- **Related:** B-03.17 shares the expression fragility; B-03.7 is its separate preserved-clock case. No other lane claims this specific encoder defect.
- **Backlog:** No.

### B-03.3 — The VA-API "native" probe very likely fails on every host
- **Claim:** The VA-API native-filter capability probe cannot validate its tone mapper with the synthetic SDR input it supplies.
- **What I traced:** apps/portico-server/server/internal/decoder/hardware.go:148–153 defines tonemap_vaapi; :284–300 constructs testsrc2 and appends upload AFTER the provided native filter chain; :369–373 gates NativeFilters on that command. BuildConversion at :606–633 then withholds native decoding. Pinned FFmpeg vf_tonemap_vaapi.c:62–67, :124–126 rejects missing mastering-display metadata.
- **Verdict:** **Confirmed, narrower.** The probe is intrinsically unsuitable: the tone mapper gets neither an HDR mastering-data frame nor the correctly ordered uploaded input. The latter is an independent reason this exact chain cannot validate VA-API. This proves the VA-API false-negative, but does not establish the separate QSV-on-SDR conjecture. Software decode/filter plus VA-API encode remains the fallback.
- **Class:** Defect.
- **Severity:** **Medium.** VA-API owners lose native decode/filter acceleration even for SDR playback; encoding itself remains available.
- **Fix direction:** Split SDR decode/scale qualification from HDR tone-map qualification, upload before hardware filters, and supply a tiny HDR fixture with mastering metadata. Touch decoder/hardware.go and probe fixtures. (**Size:** M; main packages/files are named above.)
- **Related:** B-03.2 shares the all-or-nothing native-filter gate; B-03.6 concerns counting later content failures as hardware failures.
- **Backlog:** No.

### B-03.4 — Software HDR conversions fail inside the Linux sandbox when Vulkan (or OpenCL) was detected
- **Claim:** A software encoder can choose a GPU tone mapper but be launched without the GPU sandbox grants.
- **What I traced:** apps/portico-server/server/internal/decoder/toolchain.go:148–165 probes Vulkan/OpenCL with Hardware:true; apps/portico-server/server/internal/decoder/hardware.go:684–691 chooses them for a software backend. apps/portico-server/server/internal/playback/delivery_plan.go:800 uses CurrentToolchain, while apps/portico-server/server/internal/playback/hls.go:532 derives Hardware only from the encoder backend. apps/portico-server/server/internal/mediaexec/sandbox_linux.go:128–144 grants driver paths/devices only for Hardware. hls.go:566–601 retries hardware once as software, but has no second fallback for a software graph.
- **Verdict:** **Confirmed, narrower.** The probe/production permission mismatch is confirmed on the wired path. Scope is Linux with working confinement and GPU-dependent tone mapping selected. Lavapipe availability alone does not prove an actual failure or slowness on every installation: exact mounted dependencies matter. On a genuine GPU the devices are absent, so the promised software fallback can also fail. Physical execution remains untested.
- **Class:** Defect.
- **Severity:** **High.** HDR conversions can fail outright on Linux with an active sandbox, a successful Vulkan/OpenCL probe and a software encode plan, including software fallback.
- **Fix direction:** Derive job GPU requirements from ConversionGraph stages/device initialization, including the fallback graph. Retain CPU tone mapping as a compatible retry if GPU filtering fails. Touch decoder graph metadata, playback/hls.go and mediaexec qualification. (**Size:** M; main packages/files are named above.)
- **Related:** B-03.5 is the parallel missing-grant problem in the remote executor; Q-24.12 asks for hardware recovery acceptance. These agree on the untested fallback boundary.
- **Backlog:** No.

### B-03.5 — Remote-source conversions get no GPU access and no error text
- **Claim:** STRM playback windows lack a hardware grant flag and return no FFmpeg diagnostic to HLS.
- **What I traced:** apps/portico-server/server/internal/playback/hls_burn.go:55–64 selects WindowInputs only for SourceContainer=strm; apps/portico-server/server/internal/subtitlevideo/window_input.go:34–47 calls decoder.RunHLSWindow; apps/portico-server/server/internal/decoder/hls_window.go:28–41 builds confinedHLSCommand. apps/portico-server/server/internal/decoder/command.go:82–102 defines decoderJob/mediaJob without Hardware. apps/portico-server/server/internal/playback/hls.go:504–513 explicitly returns an empty diagnostic; :566–587 still counts hardware fallback. Crucial caller guard: apps/portico-server/server/internal/playback/delivery_plan.go:285–291 returns original remote delivery unless BurnIn; apps/portico-server/server/internal/playback/subtitle_delivery.go:44–60 supplies BurnIn plus selected hardware on subtitle replacement.
- **Verdict:** **Confirmed, narrower.** The executor mismatch and empty diagnostic are definite, but ordinary STRM playback is not converted: planDelivery returns remote delivery early unless burn-in was requested (:285–291). The reachable affected path is an explicitly selected burn-in subtitle on a STRM source, with hardware selected and the Linux sandbox active. Mounted/network files using ordinary PlaybackInput do not take it; unsandboxed execution may work. Only failing requests on this narrower path count toward the breaker.
- **Class:** Defect.
- **Severity:** **Medium.** Viewers explicitly choosing burn-in on a STRM source can lose GPU execution and useful diagnostics on sandboxed Linux; ordinary remote playback returns before conversion planning.
- **Fix direction:** Carry graph hardware requirements and bounded stderr through WindowInput.Run/RunHLSWindow, using the same job metadata as local HLS. Touch subtitles WindowInput interface, subtitlevideo/window_input.go, decoder/command.go and hls_window.go, playback/hls.go. (**Size:** M; main packages/files are named above.)
- **Related:** B-03.4 and B-03.6 agree on the executor/attribution defect. B-02.12 concerns burn-in re-planning policy on the same caller. Merge grant propagation with B-03.4; retain remote diagnostic capture as its own acceptance case.
- **Backlog:** No.

### B-03.7 — Live/Library-Channel conversions: the safe keyframe expression is overridden by the unsafe one
- **Claim:** The live conversion builder overwrites its preserved-clock-safe keyframe expression with the generic zero-origin expression.
- **What I traced:** apps/portico-server/server/internal/playbackruntime/linear.go:757, :837 calls RunLinearHLS. apps/portico-server/server/internal/decoder/linear_hls.go:82–99 sets copyts/start_at_zero and seek, :112 adds prev_forced_t, :113 appends graph.Video; apps/portico-server/server/internal/decoder/hardware.go:731 supplies the later n_forced expression. However, pinned upstream FFmpeg n8.1.2 fftools/ffmpeg_enc.c:739–742 initializes ref_pts from the first encoded frame and computes pts_time=(frame->pts-ref_pts)*time_base; :750 feeds that relative value into expression t. The source version is fixed by apps/portico-server/third_party/ffmpeg/sources.lock.json:5–8.
- **Verdict:** **Refuted.** The duplicate option is real, but the claimed preserved-source-clock failure is false for the qualified bundle. FFmpeg rebases expression t to the first frame independently of copyts, so joining 1800 seconds in begins with expression t=0 and does not force 450 consecutive keyframes. An explicitly configured older/custom FFmpeg is a separate compatibility question; I did not qualify every historical implementation. This is not a playback blocker in the bundled path. Changing to prev_forced_t remains a maintenance choice, not evidence of a required fix.
- **Class:** Design risk.
- **Severity:** **Low.** Only duplicate argument maintenance and custom-version compatibility remain; the stated mid-file playback failure is refuted.
- **Fix direction:** Remove the duplicate option for clarity and qualify alternate FFmpeg versions before relying on their expression semantics. Touch decoder/linear_hls.go; no urgent behavioral repair is justified. (**Size:** S; main packages/files are named above.)
- **Related:** B-03.17 repeats the same clock assumption as a Low latent warning; it is listed as not re-checked per the worklist, not used as corroboration. B-03.19 remains a separate throughput question.
- **Backlog:** No.

### B-03.8 — The fixed 4 GiB per-session ceiling fights the owner's retention setting
- **Claim:** Per-session storage enforcement cancels production without reclaiming bytes when a valid retention window exceeds 4 GiB.
- **What I traced:** apps/portico-server/server/internal/operations/settings.go:81–82 permits 0–3600 seconds retention; apps/portico-server/server/internal/playback/hls.go:940–958 prunes using the configured value; hls_timeline.go:131–147 keeps all segments at zero. hls_sweep.go:59–65 sets the ceiling, :169–207 measures and cancels active producers. :343–360 protects recently served sessions from global eviction.
- **Verdict:** **Confirmed, narrower.** The cap/retention conflict is real, but the 8 Mb/s one-hour example is wrong: one hour plus a 60-second buffer is about 3.66 GB, below 4 GiB, and ordinary pruning keeps it there. At one-hour retention the approximate threshold is 9.39 Mb/s including that buffer (before overhead); at default 180+60 seconds it is about 143 Mb/s. A 25 Mb/s remux or zero-retention long session can exceed the cap. The 30-second repeated-cancel cycle is possible while production is active; exact viewer stalls were not measured.
- **Class:** Defect.
- **Severity:** **Medium.** Long-retention/high-bitrate HLS sessions can suffer repeated producer cancellation; ordinary 8 Mb/s playback with a one-hour retention does not reach 4 GiB.
- **Fix direction:** Enforce disk limits with oldest-played-segment eviction and explicit retention semantics; cancel only if safe reclamation cannot restore the limit. Touch playback/hls_sweep.go, hls_timeline.go, copy-timeline pruning and settings descriptions. (**Size:** M; main packages/files are named above.)
- **Related:** No exact duplicate; this is a source/storage policy inconsistency, not evidence that ordinary default playback stalls.
- **Backlog:** No.

### B-03.9 — MPEG-TS and other non-indexed sources can probably never be direct-streamed
- **Claim:** The one-packet seek probe may reject non-indexed TS sources and force an unnecessary conversion.
- **What I traced:** apps/portico-server/server/internal/playback/hls_copy_timeline.go:138–186 seeks at each grid point and requires a first packet with a PTS and K flag; :93–102 rejects any failed point. apps/portico-server/server/internal/playback/hls.go:338–345 downgrades copy on error. Pinned FFmpeg mpegts.c:3674–3681 uses read_timestamp=mpegts_get_dts and seek.c:289–346 implements generic binary/index seeking; there is no TS-specific keyframe-seek guarantee.
- **Verdict:** **Unsettled — needs runtime.** The strict requirement and conversion fallback are confirmed, but “can never” is too broad. Existing probe-built indexes, GOP alignment and packet parsing can change the result. Source reading cannot establish how a representative DVR recording answers every read interval. Cheapest experiment: outside all checkouts, use a tiny generated H.264/AAC TS with off-grid GOPs and nonzero TS origin; issue the exact ffprobe CSV interval command from :169 and pass observations through the copied timeline validator. A non-K/missing row proves that fixture is needlessly converted; all valid rows refute the blanket TS claim. No experiment was run.
- **Class:** Design risk.
- **Severity:** **Medium.** Viewers who need HLS for TS recordings may pay an avoidable video encode; whether a representative file actually fails indexing remains unproved.
- **Fix direction:** If the experiment confirms common failure, persist source keyframe evidence from a bounded sequential background scan rather than guessing a fixed remux timeline. Touch playback/hls_copy_timeline.go and scan-owned keyframe facts. (**Size:** M; main packages/files are named above.)
- **Related:** No duplicate found; general direct-stream policy findings do not settle this demuxer behavior.
- **Backlog:** No.

### B-03.12 — Converted video with copied audio: audio from before the seek point is included
- **Claim:** Input-seek video conversion with copied muxed audio can retain audio preroll, potentially upsetting segment/seek behavior.
- **What I traced:** apps/portico-server/server/internal/playback/delivery_plan.go:623–630 allows copied audio beside converted video; apps/portico-server/server/internal/playback/hls.go:399–445 seeks before input and has no output seek. At :439 alternate-audio renditions remove audio from the video process; hls_renditions.go:419 has an output-side -ss 0 for those separate audio windows. Pinned FFmpeg ffmpeg_mux_init.c:1482 defaults copy_prior_start=-1; ffmpeg_mux.c:476–484 gates discarded preroll on that setting/output seek.
- **Verdict:** **Unsettled — needs runtime.** The suspect command shape exists, but it excludes the separate rendition path and no player-visible error is established. Timestamp offset, muxer start behavior and player discard of preroll all matter. Cheapest experiment: create a ten-second-GOP file with an audible timecode outside the checkouts, copy the exact converted-video/copied-audio argument recipe, restart at 36 seconds, inspect first audio/video PTS and HLS cut times, then play that boundary on one physical Apple player. Early audio alone confirms preroll, not an audible repeat; a repeat or non-grid cut establishes the defect.
- **Class:** Design risk.
- **Severity:** **Low.** Only seeks/restarts with converted video and muxed copied audio are implicated; a user-visible repeat or seek error is not established.
- **Fix direction:** If observed, add an output-side trim/start gate for copied audio without re-encoding it; verify timeline continuity. Touch playback/hls.go and HLS seek fixtures. (**Size:** M; main packages/files are named above.)
- **Related:** B-03.1 is another segment-grid mechanism, not a duplicate; no other worklist entry proves this player effect.
- **Backlog:** No.

### B-03.15 — Converted audio keeps a 96 kHz (or 88.2 kHz) sample rate
- **Claim:** Video-session audio conversion does not specify or negotiate an output sample rate.
- **What I traced:** apps/portico-server/server/internal/decoder/hardware.go:751–770 sets codec, bitrate and channels only; DownmixFilter at :781–785 does not set a rate. apps/portico-server/server/internal/playback/delivery_plan.go:799–810 has no output sample-rate field. hls_renditions.go:420 uses the same graph; decoder/linear_hls.go:123 correctly sets 48000 for live audio.
- **Verdict:** **Confirmed, narrower.** For AAC, a supported high source rate can survive conversion; there is no output-rate adaptation to the client. This is not true for every codec: AC-3/E-AC-3 may select a codec-supported rate. The affected devices and probability of silence were not qualified. Separate rendered music/audio uses its own declared-rate plan and is outside this finding.
- **Class:** Defect.
- **Severity:** **Medium.** Video viewers with a high-rate source and an AAC decoder limited to lower rates can receive an incompatible converted audio stream.
- **Fix direction:** Add negotiated output sample rate to delivery and audio-rendition plans and set -ar for conversions; select a supported rate from capabilities rather than blindly clamping all audio. Touch playback delivery_plan/delivery_decision, rendition models and decoder.ConversionRequest. (**Size:** M; main packages/files are named above.)
- **Related:** No exact duplicate. Keep this separate from the rendered-audio fidelity/capability findings in document 10.
- **Backlog:** No.

### B-03.19 — Performance risk: 4K live channels decoded on one thread
- **Claim:** Live conversion limits source decoding to one thread, risking failure to keep up with demanding channels.
- **What I traced:** apps/portico-server/server/internal/playbackruntime/linear_plan.go:181 requires conversion for HEVC/interlaced/incompatible sources; :240–250 enforces owner and quality permission. linear.go:757 and :837 call the live executor. apps/portico-server/server/internal/decoder/linear_hls.go:82 sets input threads=1, :112 sets encoder threads:v=2; :93 builds a software conversion graph.
- **Verdict:** **Unsettled — needs runtime.** The thread limits are confirmed. It is not possible to conclude that every 4K live channel misses real time: host speed, codec and source bitrate are decisive, and direct/copied H.264 sources avoid this cost. Cheapest experiment: run the copied live conversion command on a 30-second representative 4K HEVC clip to local files under /tmp, compare encoded media duration with wall time on the intended weakest supported host. Slower than real time establishes the throughput failure for that tuple.
- **Class:** Design risk.
- **Severity:** **Medium.** High-resolution live conversion on CPU-limited hosts is at risk; a one-thread option alone does not establish inability to sustain real time.
- **Fix direction:** Qualify decoder threading and use an appropriate bounded interactive thread budget; do not infer lower quality from load. Touch decoder/linear_hls.go and live acceptance profiles. (**Size:** S; main packages/files are named above.)
- **Related:** B-03.7 shares this builder but concerns correctness, not throughput. No exact duplicate.
- **Backlog:** No.

### B-04.1 — Sidecar subtitle files are never imported automatically, so they are never auto-selected
- **Claim:** Scan-time sidecar discovery does not produce a selectable resource for automatic subtitle choice or HLS renditions.
- **What I traced:** apps/portico-server/server/cmd/server/main.go:498, :541–544 installs the service/extractor/scanner. apps/portico-server/server/internal/ingestion/analysis.go:396–418 discovers embedded and sidecars; :454 calls ImportScanText. apps/portico-server/server/internal/subtitles/scan_text.go:46 imports embedded only. autoselect.go:134 chooses subtitle_resources, manifest.go:18–23 pins resources, playbackv1/options.go:500–506 lists resources. apps/portico-server/packages/client-core/src/subtitles.ts:186–187 imports discovery as personal on explicit in-player choice.
- **Verdict:** **Confirmed, narrower.** The resource gap is confirmed; “never auto-selected” needs the qualifier “until imported.” A previous personal import remains available to that viewer, and an owner can publish a shared resource. The cheap stored-text-only auto-selection rule is deliberate (autoselect.go:9–15); failure to populate sidecar text in advance is unfinished wiring against Spec — Playback Protocol v1 §4.1/§5.5 and P06, not a reason to add expensive work to playback start. Cast additionally has a separately recorded sidecar presentation gap.
- **Class:** Unfinished feature.
- **Severity:** **High.** Viewers relying on matching sidecar text lose automatic captions and standard HLS subtitle delivery until someone imports the track.
- **Fix direction:** Import bounded valid sidecar text as shared resources in scan work, retaining source revision fences and no start-time extraction; include discoveries in options or provide an explicit availability state. Touch subtitles/scan_text.go, ingestion analysis, playbackv1/options.go. (**Size:** M; main packages/files are named above.)
- **Related:** B-04.24 and B-21.9 concern pre-play controls; B-04.2 concerns resource ordering. They agree on the imported/discovered split. Backlog lists the Cast presentation gap, not automatic sidecar import.
- **Backlog:** No (related Cast gap only).

### B-04.4 — The 90-second budget covers up to eight full reads of the file
- **Claim:** One scan-import deadline spans queueing and up to eight sequential whole-track extraction runs.
- **What I traced:** apps/portico-server/server/internal/subtitles/scan_text.go:25–32 starts a 90-second deadline before the one global slot; :44–55 imports up to eight tracks sequentially. acquisition.go:84–99 releases the write gate before calling the extractor. subtitlevideo/runtime.go:204–231 opens a fresh bridge and extractor per track. decoder/subtitle.go:65 maps one subtitle stream and reads to EOF. ingestion/analysis.go:480–484 withholds completion on failure. scan_text_test.go:70–101 explicitly verifies extraction does not hold the writer.
- **Verdict:** **Confirmed, narrower.** The finite budget and repeated demux passes are definite; the assertion that every 40 GB file must read every byte from disk each time is too strong because container access, page cache and storage throughput matter. A single 40 GB sequential read in 90 seconds already needs about 444 MB/s before overhead. No size-proportional budget or multi-track extraction is supplied. The exact timeout frequency is unmeasured, but the mismatch is real and unrelated to writer locking.
- **Class:** Defect.
- **Severity:** **High.** Large files with embedded text tracks can repeatedly fail the basic import and block media-derived deep features on HDD/NAS libraries.
- **Fix direction:** Acquire the slot before starting an appropriately bounded batch budget, extract multiple text tracks in one demux pass, and publish per-track outcomes without blocking independent analysis. Touch subtitles/scan_text.go, Extractor interface, subtitlevideo/runtime.go, decoder/subtitle.go and ingestion completion state. (**Size:** M; main packages/files are named above.)
- **Related:** B-04.3 is the downstream consequence; B-04.12 is the separate interactive first-use budget. B-04.35 covers optional deep-stage cost.
- **Backlog:** No.

### B-04.9 — Non-Western legacy encodings are shown as garbage, silently
- **Claim:** Legacy SRT/VTT bytes that are not valid Unicode are interpreted as Windows-1252 without a charset choice.
- **What I traced:** apps/portico-server/server/internal/subtitles/tolerant.go:50–68 explicitly chooses Windows-1252 as the Western fallback; document.go:83–118 admits only valid Unicode in DecodeText. render_asset.go:72–75 and :106–113 uses strict DecodeText for IDX and ASS before any text simplification. CanonicalAsset at :114–123 sends SRT/VTT through canonicalPlain/ParseTolerant.
- **Verdict:** **Confirmed, narrower.** The asymmetric encoding behavior is confirmed. It is an explicit fallback policy, not an accidental decoder path. Valid timestamp ASCII with CP1251/GBK text can pass as wrong words; malformed UTF-16 with a BOM is refused instead of guessed. Existing document.go no-locale-guessing intent and tolerant.go Western fallback rationale should be reconciled. No source evidence justifies calling every non-Western file affected: Unicode files work.
- **Class:** Deliberate trade-off.
- **Severity:** **Medium.** Viewers of non-Unicode legacy text in other code pages can receive silently corrupted dialogue; existing Unicode and Western-1252 files work.
- **Fix direction:** Add an explicit charset override and provenance/warning when a fallback is used; optionally bounded charset detection if product policy permits. Preserve Unicode-first deterministic parsing. Touch subtitles/document.go, tolerant.go, import/upload schemas and caption UI. (**Size:** M; main packages/files are named above.)
- **Related:** No exact duplicate; B-04.10 is separate validation behavior.
- **Backlog:** No.

### B-04.10 — One odd line makes a whole ASS script unusable
- **Claim:** Strict ASS admission can reject a complete script before the tolerant text projection can salvage valid dialogue.
- **What I traced:** apps/portico-server/server/internal/subtitles/render_asset.go:106–113 calls validateASS; :175–176 refuses font/graphics sections; :215–230 returns on one bad/zero-length event; :244 fixes centisecond syntax. acquisition.go:148–154 and subtitlevideo/runtime.go:229 canonicalize before renderChoice. tolerant.go:263–266 would skip a bad dialogue line, but renderChoice at :319–324 sees only admitted assets. render_asset_test.go:53–59 tests malformed-time rejection.
- **Verdict:** **Confirmed, narrower.** Whole-script rejection is confirmed, but the duration-overrun check does not run for embedded extraction: runtime.go:229 supplies duration=0. Embedded FFmpeg ASS re-encoding can also normalize some unusual input syntax before Portico sees it. The [Fonts]/[Graphics] rejection is a deliberate closed rich-asset policy and should not be “fixed” by accepting unsafe content blindly. The defect is applying full styled-asset admission to requested plain-text output before its tolerant projector can salvage ordinary lines.
- **Class:** Defect.
- **Severity:** **Medium.** An anomalous ASS event can remove all usable dialogue from that script, with further scan coupling through B-04.3.
- **Fix direction:** Separate safe text extraction from strict styled-asset admission; skip individually invalid text events with an explicit warning while retaining structural/resource bounds. Touch subtitles CanonicalAsset/validateASS, ParseASSAsText/renderChoice and extraction result handling. (**Size:** M; main packages/files are named above.)
- **Related:** B-04.3 agrees on scan fallout; B-04.14 is a different font-availability issue.
- **Backlog:** No.

### B-04.11 — Image subtitle tracks over 32 MiB cannot be used
- **Claim:** The bitmap import/extraction pipeline caps a single track at 32 MiB of subtitle data.
- **What I traced:** apps/portico-server/server/internal/decoder/subtitle.go:12, :71–75 bounds output; apps/portico-server/server/internal/subtitles/render_asset.go:18–19 bounds binary/canonical bytes and :42–46 bounds sidecar data plus companion. inventory.go:129–140 requests the binary bound for SUP/VobSub; acquisition.go:95–101 imports the whole extracted result; playback/hls_burn.go:21–26 requires a selected imported render asset. No streaming/windowed bitmap import alternative is wired.
- **Verdict:** **Confirmed, narrower.** The bound and inability to use larger tracks are confirmed, but prevalence/30–60 MB estimates were not measured. A finite memory/output bound is intentional; raising it is a product capacity choice, not itself a correctness fix. Sidecar behavior is worse for a separate reason: the storage reader refuses a requested 32 MiB bound even for a tiny file (N-V03.1). That new defect must not be confused with this intentional large-track ceiling.
- **Class:** Deliberate trade-off.
- **Severity:** **Medium.** Viewers requiring a bitmap track beyond the advertised internal bound have no supported import/burn path for that track.
- **Fix direction:** Document a consistent supported-size envelope or replace whole-track JSON capsules with bounded on-disk/windowed subtitle assets; validate size-proportional extraction. Touch decoder/subtitle.go, subtitles render assets/object publication, subtitlevideo extraction and burn input. (**Size:** M; main packages/files are named above.)
- **Related:** B-04.12 shares whole-track first use. N-V03.1 is an actual mismatched lower reader bound, not a duplicate of the 32 MiB policy.
- **Backlog:** No.

### B-04.12 — First use of an image track must read the whole file inside a 110-second request
- **Claim:** Interactive embedded bitmap import scans the track synchronously under the client’s 110-second request deadline.
- **What I traced:** apps/portico-server/packages/client-core/src/subtitles.ts:152–154 gives non-GET requests 110000 ms; :186–187 waits for /import before selecting. apps/portico-server/server/internal/httpapi/subtitle_routes.go:158–163 forwards r.Context to Service.Import; apps/portico-server/server/internal/subtitles/acquisition.go:84–101 commits preflight then invokes Extract synchronously; subtitlevideo/runtime.go:204–219 opens a fresh source and one-track extraction. decoder/subtitle.go:65 has no seek/duration/window limiter.
- **Verdict:** **Confirmed, narrower.** The synchronous first-import risk is confirmed. “Picking any image track” is too broad: a previously imported resource skips extraction, and sidecars use their own small-file reader instead of the whole media demux. Retry reuses the operation ID; after successful publication it can replay a receipt, but an aborted extraction has no partial-progress checkpoint. No actual 110-second failure was timed. Cheapest runtime acceptance: import one unprepared bitmap track from a representative large remux over the intended NAS, observe request/extractor completion time and retry receipt behavior.
- **Class:** Design risk.
- **Severity:** **Medium.** First-time embedded bitmap users on large/slow sources can time out; already imported assets and small sidecars avoid the scan.
- **Fix direction:** Offer a durable asynchronous import with progress or prepare bitmap assets in bounded background work, preserving exact source/operation fencing. Touch subtitle acquisition/routes, subtitlevideo extraction and client-core subtitle import status. (**Size:** M; main packages/files are named above.)
- **Related:** B-04.4 shares repeated extraction cost; B-04.11 is an independent byte ceiling.
- **Backlog:** No.

### B-04.13 — Burned image subtitles may be squashed or clipped when the video is not the subtitle's shape
- **Claim:** Bitmap burn-in fixes the subtitle canvas and final subtitle scale to source video dimensions, which may distort a different authored canvas.
- **What I traced:** apps/portico-server/server/internal/playback/hls_burn.go:48–50 reads dimensions from the delivery trace; apps/portico-server/server/internal/decoder/burn_in.go:46 sets canvas_size and :47 scales the subtitle frames to exactly that width/height before overlay. hardware.go:696–709 burns before final output scaling. decoder/burn_in_test.go:18–36, :64 uses matching 160x90 video and PGS canvas, so it does not disprove the cropped-canvas issue.
- **Verdict:** **Unsettled — needs runtime.** The potentially unsafe geometry is confirmed, but FFmpeg bitmap decoding/sub2video canvas growth, rect origin and clipping determine the actual result. No cropped-canvas playback or screenshot was produced. Cheapest experiment: adapt the existing synthetic PGS fixture outside the checkouts to a 1920x1080 canvas with a bottom caption, burn through the copied graph into 1920x800 video, and inspect a still plus caption bounding box. Distorted aspect or missing bottom pixels confirms this trigger; correct geometry refutes it.
- **Class:** Design risk.
- **Severity:** **Medium.** If reproduced, cropped-video viewers lose readable or correctly placed bitmap captions; same-canvas fixtures do not exercise this case.
- **Fix direction:** If reproduced, retain subtitle-authored canvas dimensions and apply explicit aspect-preserving crop/pad/scale rules. Touch decoder/burn_in.go, RenderAsset canvas metadata and playback/hls_burn.go. (**Size:** M; main packages/files are named above.)
- **Related:** No exact duplicate. Same-canvas timing tests only establish their own fixture shape.
- **Backlog:** No.

### B-04.14 — Burned-in styled subtitles can lose their glyphs or fonts
- **Claim:** Styled burn-in does not carry embedded font attachments and only guarantees a limited fallback face.
- **What I traced:** apps/portico-server/server/internal/decoder/burn_in.go:30 renders a standalone ASS file without fontsdir; decoder/subtitle.go:65 maps only the subtitle stream and excludes data; playback/hls_burn.go:36–46 writes only the script/bitmap files. apps/portico-server/server/internal/mediaexec/fonts.go:15–33 deliberately bundles DejaVu Sans and exposes existing host fonts; sandbox_linux.go:145–151 mounts that pack with Fonts:true from hls.go:532.
- **Verdict:** **Confirmed, narrower.** Attachment fonts are not transported, so exact authored typography is not preserved unless the host supplies the same face. Missing fontsdir by itself is not proof of missing text: fontconfig has a real fallback and host-font discovery. On minimal hosts, scripts using glyphs outside that pack’s coverage have no guaranteed usable glyph. The default ASS import flattens to external text, so this specifically affects the explicit styled/burn route, not all anime/CJK text subtitles.
- **Class:** Unfinished feature.
- **Severity:** **Medium.** Viewers burning styled text on minimal hosts can lose unsupported glyphs, while authored attachment-only fonts are substituted.
- **Fix direction:** Extract/revision-fence permitted font attachments into a per-asset font set, pass fontsdir to libass, and establish fallback script coverage for the release bundle. Touch subtitlevideo extraction, render assets, playback/hls_burn.go, decoder/burn_in.go and mediaexec/fonts.go. (**Size:** M; main packages/files are named above.)
- **Related:** B-04.10’s [Fonts] section admission is distinct. B-20.12 concerns overlay appearance preferences, not burn-in font custody.
- **Backlog:** No.

### B-04.20 — The OpenSubtitles sign-in is a static environment variable
- **Claim:** OpenSubtitles authentication is an immutable startup bearer token with no login or renewal mechanism.
- **What I traced:** apps/portico-server/server/cmd/server/main.go:498 calls ProviderFromEnvironment once; apps/portico-server/server/internal/subtitles/provider.go:55–69 reads the API key/token; :108–109 attaches the same token; :116–126 handles rate limiting but treats all non-200 statuses as unavailable. Search at :137–150 and download at :173–190 use that same request helper. No login/refresh caller was found in the package.
- **Verdict:** **Confirmed, narrower.** The static auth implementation is confirmed. The quoted “about a day” token lifetime and anonymous quota were not independently established from current official provider documentation and are not treated as facts here. Whenever a configured token expires or is revoked, the process has no recovery except owner configuration/restart. A provider-less server does not encounter this path. Normative Spec — Playback Protocol v1 §4.3 expects owner-configurable provider behavior; this is incomplete auth/configuration support.
- **Class:** Unfinished feature.
- **Severity:** **Low.** Owners using authenticated online downloads must manually supply a renewed token when the provider stops accepting it.
- **Fix direction:** Implement owner-held credentials/login lifecycle, renewal on an authenticated expiry response and actionable provider status, keeping tokens private. Touch subtitles/provider.go, owner provider configuration and operation/UI error mapping. (**Size:** M; main packages/files are named above.)
- **Related:** No exact duplicate; B-04.19 concerns search identity, independent of token validity.
- **Backlog:** No.

### B-04.23 — One imperfect chapter discards all chapters
- **Claim:** Chapter admission is all-or-nothing if any timing violates the validator.
- **What I traced:** apps/portico-server/server/internal/assets/assets.go:190–192 records container OriginUS; :233–252 compares raw chapter starts/ends with Duration and rejects overlap, nonpositive length, invalid numbers or overrun beyond 0.1 seconds by clearing the list. That condition runs in the production probe; chapters then feed playback options/mediaanalysis from persisted facts.
- **Verdict:** **Confirmed, narrower.** The fail-closed chapter rule is confirmed. The extra nonzero-container-origin scenario remains a hypothesis because chapter timestamp conventions vary by demuxer; OriginUS is not subtracted in this loop. Rejecting genuinely incoherent chapters is deliberate validation, not evidence of broken media playback. The useful compatibility question is whether safe start-only chapters can be retained while individual bad entries are flagged.
- **Class:** Deliberate trade-off.
- **Severity:** **Low.** A single anomalous chapter hides the whole chapter list for that file; media playback itself remains available.
- **Fix direction:** Preserve valid ordered chapter starts with explicit per-entry warnings and normalize the chapter time domain using demuxer evidence; avoid inventing chronology for malformed chapters. Touch assets chapter parser, catalog chapter publication and chapter fixtures. (**Size:** S; main packages/files are named above.)
- **Related:** B-10.16 is an exact duplicate and agrees on whole-list rejection; merge under one chapter-admission issue.
- **Backlog:** No.

### B-04.27 — Flat folders with more than 4,096 entries lose all sidecars, and discovery re-lists the folder for every video
- **Claim:** Large flat directories exceed sidecar discovery capacity and local discovery repeats directory enumeration per video.
- **What I traced:** apps/portico-server/server/internal/ingestion/analysis.go:410–418 splits managed remote and local paths. apps/portico-server/server/internal/subtitles/inventory.go:37–40 enumerates the whole local parent; :57–60 caps at 4096 entries; :81–89 returns embedded tracks with capacity/unavailable before appending any sidecars. managed_inventory.go:37–67 instead reuses durable per-scan listing rows, with LIMIT 4097. storage/client.go:124–129 invokes a new helper; :306–330 opens and reads the directory, with no listing cache for this operation. ingestion/analysis.go:487–497 warns on incomplete discovery.
- **Verdict:** **Confirmed, narrower.** The sidecar-loss cap is confirmed for both paths, but external remote relisting per video is refuted: managed sources reuse persisted entries. Local directories are freshly enumerated per video (OS caching may reduce physical I/O, not the loop/stat cost). Embedded tracks are retained, so “all sidecars” does not mean “all subtitles.” The fail-closed capacity bound is intentional, with a poorly scaling association design; about 1400 movies plus NFO/subtitle peers can cross it.
- **Class:** Design risk.
- **Severity:** **Medium.** Owners of large flat folders lose sidecar inventory for the affected files and pay repeated directory traversal work.
- **Fix direction:** Index exact-basename sidecar associations once per admitted directory revision and query candidates per video; retain bounded candidate limits without failing on unrelated folder entries. Touch subtitles/inventory.go/managed_inventory.go, ingestion directory publication and storage inventory. (**Size:** M; main packages/files are named above.)
- **Related:** B-05.22 agrees on repeated local sidecar process/I/O cost but concerns metadata lookups, not this subtitle directory cap. Backlog “Missing sidecars aren’t cached” is likewise related metadata work. Managed remote listings are durably reused.
- **Backlog:** Related metadata cache item only.

### B-04.28 — Any decode error aborts an analysis stage
- **Claim:** Analysis recipes ask FFmpeg to treat a reported fatal decode error as stage failure.
- **What I traced:** apps/portico-server/server/internal/decoder/analysis.go:39 sets -xerror for every analysis recipe. mediaanalysis/work.go:193–202 publishes nothing on decoder failure; :240–262 retries ordinary errors up to three times, and :115–118 later skips failed stages so other stages can proceed. The pinned FFmpeg n8.1.2 flag behavior was read, not exercised on a damaged file.
- **Verdict:** **Confirmed, narrower.** Fail-on-error is an explicit data-integrity policy. The claim “any decode error” overstates the guarantee: concealed/recoverable corruption that does not produce a fatal reported error is not necessarily fatal. A retry exists, but a deterministic fault will repeat. Successful unrelated stages are not discarded and later stages can still run. It is an optional-analysis robustness trade-off rather than a universal file-playback defect.
- **Class:** Deliberate trade-off.
- **Severity:** **Low.** One reported fatal decoding error prevents publishing that optional analysis stage, even when most of the file could be usable.
- **Fix direction:** If partial optional previews are desired, define a tolerable corruption budget and publish sampled/partial provenance; keep exact audio facts and integrity checks strict. Touch decoder/analysis.go and mediaanalysis stage policy. (**Size:** S; main packages/files are named above.)
- **Related:** B-04.3 is different: basic import failure blocks media-derived deep entry; this finding fails one deep stage with retry.
- **Backlog:** No.

### B-04.31 — On a Windows server no embedded subtitle can be used
- **Claim:** Windows disables all new embedded subtitle imports through a stale Unix capability gate.
- **What I traced:** apps/portico-server/server/internal/subtitles/service.go:63–68 clears ffmpeg on Windows; :333–336 then disables every embedded discovery. acquisition.go:72–73 rejects disabled tracks before calling Extractor. apps/portico-server/server/cmd/server/main.go:505, :541 installs a separate Runtime with the actual FFmpeg tool; subtitlevideo/runtime.go:214–219 feeds that tool a loopback bridge; decoder/subtitle.go:65 uses http,tcp, not /dev/fd. apps/portico-server/INSTALLING.md:10–13 and BUILDING.md:47 explicitly include Windows server packages.
- **Verdict:** **Confirmed, narrower.** The gate is definite and Windows is in documented release scope; the stale /dev/fd explanation no longer matches the wired extractor. Already imported resources retained from another environment, uploads, provider tracks and sidecars can still work, so “no embedded subtitle can ever be used” is too absolute. Newly inventoried embedded tracks are unusable on Windows regardless of format.
- **Class:** Defect.
- **Severity:** **High.** Windows-server viewers cannot import any newly discovered embedded track despite the current extraction transport being portable.
- **Fix direction:** Replace the GOOS blanket gate with the actual extractor/toolchain capability; qualify a text and bitmap import on a packaged Windows server. Touch subtitles/service.go and Windows extraction acceptance fixtures. (**Size:** S; main packages/files are named above.)
- **Related:** B-04.34 is a different format whitelist limitation; Q-24.30 asks for packaged platform acceptance. These do not refute the gate.
- **Backlog:** No.

### B-04.38 — HDR and anamorphic sources produce poor preview images
- **Claim:** Navigation-image recipes omit HDR tone mapping and flatten sample aspect ratio after pixel-based scaling.
- **What I traced:** apps/portico-server/server/internal/decoder/analysis.go:79 generates chapter JPEGs with scale/pad/setsar=1; :91–99 generates trickplay RGB frames and scene grayscale similarly, with no HDR-specific graph. mediaanalysis/produce.go:156–168 calls image decoding, :193–219 creates trickplay geometry, :286–307 uses the same scene recipe. No source HDR/SAR adaptation is passed into AnalysisSpec.
- **Verdict:** **Confirmed, narrower.** The missing color/geometry adaptation is confirmed; exact washed-out or Profile-5 color appearance has not been visually qualified. Anamorphic input’s display aspect can be lost because setsar=1 removes the scale filter’s SAR compensation. Scope is navigation imagery/derived luminance, not main playback. Ordinary square-pixel SDR previews are unaffected.
- **Class:** Defect.
- **Severity:** **Low.** HDR and anamorphic titles can have misleading navigation images, without changing the main playback picture.
- **Fix direction:** Pass source color/HDR and display aspect facts to preview construction; tone-map HDR and scale using display geometry before setting SAR to one. Touch decoder/analysis.go, mediaanalysis binding/recipes and preview fixtures. (**Size:** M; main packages/files are named above.)
- **Related:** No exact duplicate. B-04.21 uses these scene samples but its semantic false positives do not depend on HDR.
- **Backlog:** No.

### B-04.40 — The owner-facing description of segment detection does not match what runs
- **Claim:** The owner control promises comparisons across a season, while the detector analyzes one file’s chapters and luminance.
- **What I traced:** apps/portico-server/server/internal/administration/library_settings.go:69 describes “Compares episodes within a season”; apps/portico-server/server/internal/catalog/library_sources.go:106 includes segment_detection. mediaanalysis/produce.go:279–313 operates on b.AssetID and one decoded source, invokes chapterCandidates/detectSegments, and labels itself chapter-label-and-luminance-v1. detectors.go:24–49, :53–102 never loads a second episode or compares fingerprints.
- **Verdict:** **Confirmed.** The owner-facing promise is false on the actual configured operation. The stored fingerprint stage has no role in this detector. A user decision to enable expensive work is being made from the wrong description. No normative product decision found authorizes describing this heuristic as repeated-opening comparison.
- **Class:** Documentation error.
- **Severity:** **Medium.** Owners enabling segment detection are promised cross-episode matching but receive a single-file heuristic.
- **Fix direction:** Change the operation description/help to state exactly what single-file heuristics do and their review requirement; if cross-episode detection is intended, plan that as a separate large feature. Touch administration/library_settings.go and related owner help/contract text. (**Size:** S; main packages/files are named above.)
- **Related:** B-04.21 covers exposing unreviewed results and agrees on the detector; merge documentation acceptance with that issue without hiding their different causes.
- **Backlog:** No.

## Tier 2 — quick checks

B-04.3, B-04.18 and B-04.21 are promoted below because the correction affects scope or classification. The remaining entries are concise checks; verdict, severity and size are also in the table.

**B-03.2 — The NVIDIA "native" path can never be enabled with Portico's own FFmpeg.** **Confirmed**; Unfinished feature; **Medium**, **M**. Bundled upstream FFmpeg has no tonemap_cuda, yet one mandatory native probe includes it, so NVIDIA native filtering/decoding cannot qualify; GPU encoding can still work. Runtime throughput remains unmeasured. Evidence: apps/portico-server/server/internal/decoder/hardware.go:168, :369–372, :606–610; apps/portico-server/third_party/ffmpeg/sources.lock.json:5–8 and upstream n8.1.2 libavfilter/allfilters.c:511–513.

Fix: Qualify SDR CUDA decode/scale separately from HDR tone mapping; touch decoder/hardware.go and GPU probe fixtures. Related: B-03.3 has a different native-probe failure; common split-capability fix. Backlog: No.

**B-03.6 — The hardware circuit breaker cannot tell a bad GPU from a bad file.** **Confirmed, narrower**; Defect; **Medium**, **S**. Any non-throttled hardware-plan failure counts BEFORE the software attempt proves source validity. Explicit caller cancellation is excluded; three counted events in ten minutes open the circuit. Already running other hardware sessions are not automatically rewritten. Evidence: apps/portico-server/server/internal/playback/hls.go:566–587; hls_produce_support.go:121–127; apps/portico-server/server/internal/decoder/toolchain.go:201–227.

Fix: Count backend faults only after classifying the error or proving same-source software success; touch playback/hls.go and hardware failure recording. Related: B-03.4/B-03.5 can feed the breaker; no claim that all active viewers immediately switch follows from this code. Backlog: No.

**B-03.10 — The temporaryDirectory and maxBackgroundSessions owner settings do nothing.** **Confirmed**; Unfinished feature; **Medium**, **M**. Production Go references to these settings are limited to validation, console and telemetry; HLS/prepared roots remain state-relative and background producers do not read the cap. The unused-directory readiness warning can therefore be false. Evidence: apps/portico-server/server/internal/operations/settings.go:83, :87; telemetry/capacity.go:189–203; telemetry/attention.go:175–181; apps/portico-server/server/cmd/server/main.go:523, :664.

Fix: Wire the working-directory policy to every conversion owner and the background cap to actual preparation admission, then make telemetry report those real consumers; touch main, preparedmedia, operations and telemetry. Related: No exact duplicate found. Backlog: No.

**B-03.11 — A failed conversion's diagnostic is deleted almost immediately.** **Confirmed, narrower**; Defect; **Medium**, **S**. Diagnostics live only in playback_artifacts and are deleted with failed terminal output on maintenance. Cleanup waits for active producers and works in bounded batches, so “almost immediately” has no fixed measured duration. Evidence: apps/portico-server/server/internal/playback/hls_produce_support.go:109–115; hls.go:975, :990, :1015–1045; delivery_diagnostics.go:99–101.

Fix: Retain a bounded failure-history row independently of output cleanup; touch playback failure persistence, diagnostics and maintenance. Related: No exact duplicate found. Backlog: No.

**B-03.14 — Prepared versions are forced to 30 fps.** **Confirmed, narrower**; Deliberate trade-off; **Medium**, **M**. Prepared video recipes force 30 fps AND output qualification rejects rates above 30.1; this is an explicit compatibility envelope. Only converted prepared video is affected: original-file offline downloads and audio-only preparation are not. Film/PAL cadence conversion is a fidelity cost. Evidence: apps/portico-server/server/internal/decoder/prepared.go:37; apps/portico-server/server/internal/preparedmedia/worker.go:427, :701.

Fix: Preserve source cadence below an explicit ceiling in a NEW immutable recipe version, updating output validation; touch preparedmedia profile identity/worker and decoder/prepared.go. Related: B-14.4 independently confirms that four offered preparation rungs are rejected by this same fixed-recipe builder. That is already in the first pass, so it is not a new V03 finding. This cadence risk is separate; update both recipe and output qualification if policy changes. Backlog: No.

**B-04.2 — The scan imports an arbitrary eight text tracks, not the useful ones.** **Confirmed**; Defect; **Medium**, **S**. Discovery is sorted by hashed stable ID, then the first eight eligible embedded text tracks are imported; neither default flag nor viewer language ranks that batch. Manifest pinning again limits by resource ID. Evidence: apps/portico-server/server/internal/subtitles/service.go:310; persist.go:45–46; scan_text.go:44–55; manifest.go:18–23.

Fix: Rank stable candidates by a documented default/language/track policy before capping; touch scan_text and manifest pinning. Related: B-04.1 and B-04.5 share resource coverage but have different triggers. Backlog: No.

### B-04.3 — One bad or empty text track blocks the rest of the file's subtitles and all deep analysis
- **Claim:** One failed scan-time embedded text import aborts remaining imports and prevents the basic revision from completing.
- **What I traced:** apps/portico-server/server/internal/subtitles/scan_text.go:53–55 returns the first import error; ingestion/analysis.go:480–484 only completes successful batches; catalog/inventory.go:523–528 stores an empty basic analysis_revision while text is pending; catalog/inventory_text_import.go:35 completes it only later. mediaanalysis/work.go:141–144 requires this revision for every MEDIA-DERIVED stage, but explicitly exempts checksum.
- **Verdict:** **Confirmed, narrower.** The normal Complete-scan wrapper aborts before calling analyzeDeep (ingestion/analysis.go:483–502), so it blocks all new deep work for that attempt. There are two scope limits: earlier successful imports/results remain published, and the deep worker itself explicitly permits checksum-only Custom work without BasicRevision (:141–144); that exemption does not rescue the ordinary Complete wrapper. The extractor test verifies no long-held writer, not resolution of this durable completion gate. A malformed/empty result or shared deadline failure can repeatedly stop remaining subtitle imports and the file’s analysis.
- **Class:** Defect.
- **Severity:** **Medium.** One failed basic subtitle import can repeatedly block trickplay, chapter images and marker production for that file; already published resources survive.
- **Fix direction:** Publish per-track failure state and continue independent imports; mark basic technical facts complete independently of optional subtitle-resource preparation. Touch scan_text.go, ingestion/analysis.go, inventory text completion and mediaanalysis prerequisite logic. (**Size:** M; main packages/files are named above.)
- **Related:** B-04.4 and B-04.10 are causes; this is the downstream coupling issue to merge with them. Do not interpret the checksum-worker exemption as proof that an ordinary Complete scan continues past a failed import.
- **Backlog:** No.

**B-04.5 — A file with more than 32 subtitle tracks gets none.** **Confirmed, narrower**; Deliberate trade-off; **Medium**, **S**. More than 32 embedded tracks clears discovery; exceeding the combined embedded-plus-sidecar cap keeps embedded tracks but drops all newly found sidecars. Previously imported resources are separate and are not deleted by this function. Evidence: apps/portico-server/server/internal/subtitles/inventory.go:14, :44–47, :76–89; service.go:310–337.

Fix: Keep bounded inventory/pagination or choose explicit ranked capacity behavior with a visible explanation; touch subtitle inventory and catalog UI. Related: B-04.2 caps import at eight; B-04.27 caps directory entries separately. Backlog: No.

**B-04.6 — Common sidecar names are disabled as "ambiguous".** **Confirmed, narrower**; Deliberate trade-off; **Medium**, **S**. Only forced/sdh/cc have special suffix semantics; a second other token marks ambiguous_association. The first other token is fed to BCP-47 parsing, so aliases like “English” do not reliably become the intended language. Exact basename ownership is deliberately strict. Evidence: apps/portico-server/server/internal/subtitles/inventory.go:100–120; service.go:321–336; document.go:58–69.

Fix: Add accepted conventional suffix aliases and treat harmless descriptive tokens as a title with explicit association policy; touch inventory parser/fixtures. Related: B-04.7 is a separate exact-basename ownership rule. Backlog: No.

**B-04.7 — A second video with the same base name disables every sidecar for both.** **Confirmed**; Deliberate trade-off; **Low**, **S**. The code deliberately disables matching sidecars when a second supported media extension shares the exact stem. Different stems remain unaffected. Evidence: apps/portico-server/server/internal/subtitles/inventory.go:34–36, :66–68, :118–120.

Fix: If product policy changes, attach to all exact-stem versions or ask for explicit association; touch inventory and fixtures. Related: Backlog second-file/version grouping is related product work, not this specific association policy. Backlog: Related version-grouping item only.

**B-04.15 — Audio language preferences fail for languages outside a 57-entry table.** **Confirmed**; Defect; **Medium**, **S**. Audio language remains the raw probe tag; the small ISO table fails to equate omitted three-letter tags with a two-letter preference. This can miss preferred audio and mark same-language audio foreign for automatic subtitles. Evidence: apps/portico-server/server/internal/playback/delivery_source.go:117, :136; delivery_decision.go:242–251, :270–293; delivery_context.go:160–169.

Fix: Use one canonical BCP-47/ISO normalizer in both audio and subtitle matching; touch playback languageKey and subtitles/autoselect. Related: B-04.16 is the same normalization family with a smaller table; merge the repair. Backlog: No.

**B-04.16 — Forced-track matching uses a second, smaller table.** **Confirmed**; Defect; **Medium**, **S**. Forced matching uses its own smaller table against raw selected-audio language. For a stored canonical “bg” track and audio tagged “bul,” a known-language forced track is missed. Evidence: apps/portico-server/server/internal/subtitles/autoselect.go:43–62, :103–109; apps/portico-server/server/internal/playback/delivery_context.go:160.

Fix: Reuse the common canonical language matcher for forced selection; touch subtitles/autoselect and playback language matching. Related: B-04.15 agrees; one shared-normalizer implementation can fix both. Backlog: No.

### B-04.18 — Forced tracks with no language tag are never auto-selected; file "default" flags are ignored
- **Claim:** Forced subtitle matching requires a known matching audio language; default flags do not independently enable subtitles.
- **What I traced:** apps/portico-server/server/internal/subtitles/autoselect.go:22–24 documents explicit off; :74–101 selects requested languages and excludes forced fragments from full dialogue; :103–110 matches forced tracks only to audio language; :134 orders default candidates FIRST. autoselect_test.go:12–43 exercises viewer on/off/forced policy.
- **Verdict:** **Confirmed, narrower.** The untagged forced-track gap is confirmed. “Default flags are ignored” is wrong in its blanket form: default changes ordering and therefore can select the winner among matching candidates. It does not override explicit off or absent language/preference. That is a deliberate viewer-policy choice, not a missed default control. No source evidence establishes a safe automatic language assignment for an untagged forced track.
- **Class:** Deliberate trade-off.
- **Severity:** **Low.** Viewers with untagged forced tracks need a manual choice; a file default cannot turn captions on against the explicit policy.
- **Fix direction:** If wanted, define an explicit policy for untagged forced tracks and file defaults, preserving viewer off. Do not auto-select all undefined-language tracks indiscriminately. Touch ChooseAutomatic and preference descriptions/fixtures. (**Size:** S; main packages/files are named above.)
- **Related:** B-04.16 is a genuine failed match for KNOWN languages; B-04.15 shares its normalization cause. Keep undefined-language policy separate.
- **Backlog:** No.

**B-04.19 — Online search for an episode sends only the episode's title.** **Confirmed, narrower**; Unfinished feature; **Medium**, **M**. Default episode search uses only catalog_entities.title and a base-language query; the web requests its UI locale. A caller-supplied query can work around the missing series/episode identifiers, so “mostly unusable” is not established. Evidence: apps/portico-server/server/internal/subtitles/provider.go:137–150, :255–264; apps/portico-server/web/src/player/Player.tsx:551.

Fix: Supply provider-supported series/season/episode/identifier fields and the viewer subtitle language; touch provider candidate model and player search. Related: B-04.20 concerns credentials, separate from search quality. Backlog: No.

### B-04.21 — Unreviewed luminance guesses are shown to viewers as Skip buttons, and the intro guess is often backwards
- **Claim:** Low-confidence single-file hypotheses reach manual skip prompts, with potentially incorrect semantic labels.
- **What I traced:** apps/portico-server/server/internal/mediaanalysis/detectors.go:51–52 explicitly calls results hypotheses; :73–79 emits intro/recap from the first eligible black gap; :82–98 emits commercial/credits hypotheses. viewer_markers.go:90–109 projects undeleted unapproved results with AutomaticSafe false. viewer_markers_test.go:44–45, :73–84 explicitly EXPECTS weak/unapproved markers in the viewer set. apps/portico-server/web/src/player/SkipPrompt.tsx:23, :32–39 presents manual prompts but checks AutomaticSafe before unattended skip. Spec — Playback Protocol v1:100–101 distinguishes manual and automatic skips.
- **Verdict:** **Confirmed, narrower.** Projection is intentional and tested, so this is not an unenforced automatic-skip safeguard. The “owner review required” stage summary is misleading if interpreted as a visibility gate. A cold open ending at the first black gap is deterministically mislabeled as intro by this heuristic; how often that occurs was not measured. The concern is exposing unreliable semantics as a confident manual affordance, while unattended skipping remains correctly fenced.
- **Class:** Design risk.
- **Severity:** **Medium.** Viewers offered low-confidence manual skip prompts may skip a cold open or an ordinary scene; these heuristic markers cannot skip automatically.
- **Fix direction:** Choose a product policy: hide ambiguous low-confidence detections until reviewed, or present their uncertainty before manual skip; improve opening semantics separately. Touch viewer marker projection, detectors and skip presentation, keeping AutomaticSafe enforcement. (**Size:** M; main packages/files are named above.)
- **Related:** B-04.40 is a confirmed false owner description; B-04.22’s duplicate intro/recap label is listed as Low/not re-checked. B-12.9 uses approved credits for completion and does not prove this prompt authorizes automatic skipping.
- **Backlog:** No.

**B-04.24 — A title-page subtitle choice can silently do nothing.** **Confirmed, narrower**; Defect; **Medium**, **M**. The native title control lists raw embedded subtitle languages; the pending rule only searches imported resources and consumes an unmatched choice silently. Sidecars generally are absent from this raw-stream title control in the first place, rather than being offered then ignored. Evidence: apps/portico-react-native/apps/native/src/screens/title/PlaybackChoices.tsx:42–46; apps/portico-server/packages/client-core/src/pending-track-choice.ts:82–88; apps/portico-react-native/apps/native/src/player/usePlayer.ts:281–286.

Fix: Use actual selectable-resource/discovery identity, import when needed or expose a visible unavailable result; touch PlaybackChoices and shared pending choice handling. Related: B-04.1 and B-21.9 agree on different facets of pre-play selection. Backlog: No.

**B-04.32 — Subtitle reads and a viewer's subtitle selection queue for the single write slot at background priority.** **Confirmed, narrower**; Defect; **Medium**, **M**. List/Select/document checks still acquire ClassBackgroundMedia writer transactions, including two checks per body chunk; Plan uses a read snapshot. Long extraction releases the writer (scan_text_test.go:70–101), and no measured subtitle timeout under scan load is established. Evidence: apps/portico-server/server/internal/subtitles/service.go:97–102, :237–248; session.go:71, :190, :337, :376; manifest.go:32; httpapi/subtitle_routes.go:353–360; dbwork/write.go:74.

Fix: Move pure authorization reads to snapshots, retain revision/pin writes with appropriate playback priority; touch subtitles session/manifest/service and dbwork callers. Related: B-17.20/B-20.13 cover subtitle blanking after failed rechecks; they do not prove this queue causes it. Backlog: No.

**B-04.35 — "Complete" scanning is far heavier than its label suggests (design risk).** **Confirmed, narrower**; Deliberate trade-off; **Medium**, **M**. Complete explicitly opts into full-file reads and substantial generated data; separate checksum/audio/video passes are real. Calling its sustained cost inherently mislabeled ignores the contract’s required warning. Cost varies with streams/chapters and stages skip unsupported inputs; no “weeks” estimate was measured. Evidence: apps/portico-server/server/internal/catalog/library_sources.go:105–129; apps/portico-server/server/internal/mediaanalysis/produce.go:99–111, :156–168, :193–219, :279–307; decoder/analysis.go:39–99; portico-internal/Product Contracts/libraries-and-scanning/contract.md:68–75, :109–123.

Fix: Share compatible decodes and avoid producing unconsumed data; audit that the owner sees the required warning. Touch mediaanalysis production and scan-operation policy. Related: B-04.4 is BASIC import cost, a stronger unexpected-cost issue; B-04.40 is a provably false detector description. Backlog: No.

**B-04.36 — The v1 session says "webvtt" but the URL returns JSON.** **Confirmed**; Defect; **Medium**, **M**. A presentation advertises webvtt with a cue-document JSON URL. The .vtt handler requires a manifest pin only established on HLS sessions, so direct-play third-party clients cannot use the promised WebVTT URL. Evidence: apps/portico-server/server/internal/playbackv1/subtitles.go:75–76; httpapi/subtitle_routes.go:272, :296, :347; subtitles/manifest.go:45–47; playback/service.go:429–433.

Fix: Publish an authorized real WebVTT sidecar for any selected text resource, and label JSON separately if exposed; touch playbackv1 subtitles, subtitle routes and pinning. Related: B-04.37 affects standard HLS selection; both conflict with P05 third-party playback but are separate fixes. Backlog: No.

**B-04.37 — HLS subtitle renditions carry no default, auto-select or forced signalling (design risk).** **Confirmed, narrower**; Unfinished feature; **Medium**, **M**. Subtitle renditions explicitly signal DEFAULT=NO and AUTOSELECT=NO, and omit FORCED; positive session-selected/default/forced metadata is lost. Thus this is a missing-selection implementation, not total absence of HLS fields. Evidence: apps/portico-server/server/internal/playback/hls_renditions.go:188–190; subtitles/manifest.go:18–23.

Fix: Carry selected/default/forced metadata into TextRenditions while enforcing HLS constraints and viewer choice; touch manifest pinning, delivery text models and hlsMasterManifest. Related: B-04.36 agrees on the plain-player contract gap; native/first-party overlay controls mask this metadata loss. Backlog: No.

## Not re-checked (Low)

These 14 entries are copied from the worklist without an independent Low-entry verification or endorsement.

- B-03.13 — Odd-sized sources fail to convert when no size limit applies
- B-03.16 — Declared codec string does not describe the encoded stream
- B-03.17 — Latent trap: the keyframe expression depends on t starting at zero
- B-03.18 — Latent trap: folders with no database row are only removed under disk pressure
- B-04.8 — The client has no message for the server's "ambiguous" reason
- B-04.17 — The hearing-impaired flag from the file is ignored
- B-04.22 — The same stretch is emitted as both "intro" and "recap"
- B-04.25 — Curly-brace text is deleted from SRT and WebVTT
- B-04.26 — Overlapping cues share italic and top placement on the native overlay
- B-04.29 — Every trickplay frame is stored twice
- B-04.30 — Preview images are re-downloaded every five minutes
- B-04.33 — A vestigial second subtitle-plan system still writes rows, and several functions are dead
- B-04.34 — Embedded subtitles in some containers cannot be extracted
- B-04.39 — The web renderer rebuilds every cue whenever the HLS mapping changes

## New findings

### N-V03.1 — Bitmap sidecar discovery requests a limit the storage reader rejects
- **Claim:** Even a tiny SUP sidecar, or a valid paired VobSub IDX/SUB sidecar, is marked unavailable because discovery requests a 32 MiB read bound from a reader that permits at most 8 MiB.
- **What I traced:** `apps/portico-server/server/internal/ingestion/analysis.go:308–310` installs the rooted scan context and `:415–418` invokes subtitle discovery. `apps/portico-server/server/internal/subtitles/inventory.go:129–133` passes `MaxBinaryBytes` for every SUP; `:136–140` passes it for the SUB companion of every IDX. `apps/portico-server/server/internal/subtitles/render_asset.go:18` sets that constant to 32 MiB. `apps/portico-server/server/internal/storage/client.go:377–384` forwards the requested limit unchanged, choosing rooted `inventory-blob` in scanning. `apps/portico-server/server/internal/storage/scan_content.go:149–151` rejects requested limits above 8 MiB before opening the file; the unrooted helper also rejects at `client.go:220–222`, and managed remote readers reject at `remote_read.go:12–14`. Discovery translates the failure to `source_unavailable` at `inventory.go:149–150`, and `subtitles/acquisition.go:72–73` refuses the disabled discovery.
- **Verdict:** **Confirmed.** This is a parameter-contract mismatch, not a file-size limitation: a 1 KiB valid SUP still requests 32 MiB and is refused. A VobSub IDX can be read, but its mandatory SUB companion makes the same oversized-bound request. The ordinary rooted scanner, explicit local refresh, and managed-source small-file reader all have the same rejecting bound. No file or decoder was executed.
- **Class:** **Defect.** The storage guard works, but its caller promises a supported bitmap sidecar path that can never satisfy the reader contract.
- **Severity:** **Medium.** All viewers depending on discovered SUP or paired VobSub sidecars lose those tracks regardless of actual size. Embedded tracks and explicitly uploaded resources are independent paths and can work.
- **Fix direction:** Define one agreed binary sidecar read contract across inventory and storage. Either provide a dedicated bounded binary reader with the supported 32 MiB envelope, or use an honestly documented smaller envelope at discovery. Retain rooted source fencing and reject actual oversize files. **Size: S** for a coordinated limit repair with meaningful small-SUP/paired-SUB fixtures; touches subtitle inventory/render bounds, storage local/rooted/remote small readers.
- **Related:** B-04.11 discusses actual tracks over 32 MiB, whereas this defect disables every size; B-04.1 separately prevents automatic selection before import. No duplicate found in B-index.txt, including the other sidecar entries. **Backlog:** No.


## Disagreements worth Justin's attention

- **Do not plan B-03.7 as a bundled-FFmpeg playback blocker.** The two options exist, but upstream 8.1.2 rebases expression time and disproves the claimed hundreds-of-keyframes burst. The proposed option cleanup can remain small maintenance work.
- **Narrow B-03.5 to STRM burn-in conversion.** The ordinary remote planner exits with original delivery before selecting conversion; a general remote-source GPU failure claim overstates reachable scope.
- **Correct B-03.8's capacity arithmetic.** At 8 Mb/s, one-hour retention plus a 60-second buffer is roughly 3.66 GB, below 4 GiB. The retained-window conflict starts around 9.39 Mb/s for those settings before overhead, and zero retention means no ordinary pruning. The real policy conflict still merits a fix.
- **Treat explicit policies as decisions, not automatic bug confirmations.** Western-1252 fallback (B-04.9), actual 32 MiB bitmap limits (B-04.11), strict chapter admission (B-04.23), fail-on-error analysis (B-04.28), 30 fps prepared qualification (B-03.14), strict basename/suffix association (B-04.6/B-04.7), and undefined-language/default subtitle policy (B-04.18) have deliberate code/contract evidence. Their compatibility costs remain; lower-ceremony fixes include clear warnings, explicit overrides and better bounded processing.
- **B-04.21 has a tested manual/automatic distinction.** The weak markers are intentionally visible but cannot auto-skip. “Owner review required” does not act as a visibility gate. Decide whether uncertain semantic guesses should be offered as confident manual prompts; do not describe this as automatic skipping without approval. B-04.40's promised season comparison is independently false.
- **B-04.27's remote repeated-listing allegation is overstated.** Managed remote discovery reuses durable listing rows. Local directories still re-enumerate per video, and the capacity failure drops sidecars while preserving embedded tracks.
- **Do not lower the priority of N-V03.1 by calling it only a large-track limit.** It disables valid tiny bitmap sidecars because the requested limit itself is invalid.
- **Scan completion scope matters.** The ordinary Complete wrapper aborts before any new deep work after an import error. Checksum-only Custom work is exempt from the deep worker's Basic prerequisite, but that exemption does not rescue the ordinary failed Complete attempt. Already published tracks/results are retained.

Severity decisions compared with the worklist: B-03.7 **Medium → Low**; B-03.12 **Low–Medium → Low**; B-03.15 and B-03.19 **Low–Medium → Medium**; B-04.11 **Medium–High → Medium**; B-04.20, B-04.23, B-04.28, B-04.38, B-04.7 and B-04.18 **Low–Medium → Low**; B-04.16 and B-04.37 **Low–Medium → Medium**. B-04.40 had no first-pass confidence/severity; I assign **Confirmed / Medium / Documentation error / S**. Range resolutions are planning choices, not evidence that affected-device prevalence was measured.

## Evidence boundary

- Reviewed the live working trees on **ehler-homelab-mini-1** on 11 October 2026. Server checkout HEAD: `82aaa54f89efdc2e029b635746627cd8c8b71960`; native checkout HEAD: `a3d8e7cd14c16229715a07c67f43cb8364bae743`. These identify checkout ancestry, not a claim that the working trees were pristine or frozen. Paths are relative to `/home/justin/projects/Portico`; abbreviated same-file references refer to the full path named immediately before them.
- Read the verification brief and original brief in full, both lane documents' B entries, the worklist, B-index, current Backlog, playback architecture, playback contract, relevant normative playback/scanning sections, and applicable decision/architecture documents. No direct Backlog entry for a verified defect was found; the listed metadata/version/Cast items are related rather than duplicates.
- Read production startup, planning, callers and extractors, plus relevant subtitle/probe/burn/marker/retention fixtures. Tests were **read, not run**. Notably: scan-text tests show extraction releases the writer; viewer-marker tests deliberately project unapproved hypotheses; burn tests only use matching bitmap/video dimensions; prepared output validation enforces the fixed cadence envelope.
- Fetched only public upstream **FFmpeg n8.1.2 source text** into `/tmp/v03-ffmpeg-source`; executed none of it. Primary references: [expression time rebasing](https://github.com/FFmpeg/FFmpeg/blob/n8.1.2/fftools/ffmpeg_enc.c#L734), [NVENC IDR/output flags](https://github.com/FFmpeg/FFmpeg/blob/n8.1.2/libavcodec/nvenc.c), [QSV frame forcing/output flags](https://github.com/FFmpeg/FFmpeg/blob/n8.1.2/libavcodec/qsvenc.c), [AMF forcing/output flags](https://github.com/FFmpeg/FFmpeg/blob/n8.1.2/libavcodec/amfenc.c), [VA-API tone-map metadata admission](https://github.com/FFmpeg/FFmpeg/blob/n8.1.2/libavfilter/vf_tonemap_vaapi.c), [HLS key-packet split rule](https://github.com/FFmpeg/FFmpeg/blob/n8.1.2/libavformat/hlsenc.c), [TS demux timestamp seeking](https://github.com/FFmpeg/FFmpeg/blob/n8.1.2/libavformat/mpegts.c), and [stream-copy preroll handling](https://github.com/FFmpeg/FFmpeg/blob/n8.1.2/fftools/ffmpeg_mux.c). The tag matches the locked bundle version; custom owner-supplied FFmpeg builds/versions were not qualified.
- No Portico server, build, test suite, formatter, install or comparison code was executed. No Portico endpoint, demo, `.private`, browser or computer-use tool was accessed. No scratch behavior experiment was run; only read-only searches/text parsing and report-generation scripts ran. The proposed cheapest experiments under unsettled entries are future acceptance work outside the checkouts, not claimed reproductions.
- **Runtime-dependent effects remain unestablished:** GPU driver/backend output, real segment boundaries, throughput, disk/NAS read time, Apple/Cast preroll behavior, bitmap canvas clipping, font/glyph fidelity, preview appearance and write-gate-induced subtitle timeouts. B-03.9/B-03.12/B-03.19/B-04.13 explicitly remain unsettled; other confirmed entries distinguish the source defect/policy from these effects. Current OpenSubtitles token lifetime/quota was not independently verified and is not asserted.
- The context-sync command was attempted as instructed but failed to write its lock outside this read-only sandbox. Existing project and verification guidance was read. Only this single output file was changed under Portico; scratch reports/scripts/public-source copies are under `/tmp`.
- **Coverage:** all 25 Tier 1 entries and all 20 Tier 2 entries completed; all 14 Low entries listed without re-check. No requested ID was left unreached. One new finding recorded, N-V03.1.
