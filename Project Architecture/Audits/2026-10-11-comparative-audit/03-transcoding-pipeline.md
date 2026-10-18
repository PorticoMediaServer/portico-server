# 03 — The transcoding and stream-production pipeline

Auditor domain: FFmpeg command construction, HLS production and serving, hardware acceleration, HDR and tone mapping, throttling, segment storage and clean-up, and process confinement. The decision of *whether* to convert belongs to auditor 02 and subtitles to auditor 04; they appear here only where the pipeline depends on them.

Everything below is a **static source review**. Nothing was built, run or played. Where a finding depends on how FFmpeg or a GPU driver behaves at run time, it says so and carries a confidence rating.

Jargon used throughout, defined once:

- **Segment**: one small media file (here about six seconds) of an HLS stream. **Playlist** / **manifest**: the text file listing the segments. **Master playlist**: the top-level playlist that lists the video stream and its alternate audio and subtitle tracks ("renditions").
- **Keyframe** (IDR frame): a picture that can be decoded on its own. A segment must start on one. **GOP**: the run of frames between two keyframes.
- **Remux / copy / direct stream**: repackaging the existing compressed video into HLS without re-encoding it. **Convert / transcode**: decoding and re-encoding.
- **MPEG-TS** and **fMP4**: the two container formats HLS segments can use. fMP4 needs an extra "init" file but can carry more codecs (HEVC in browsers, AV1, FLAC, Opus).
- **Tone mapping**: turning an HDR picture into an ordinary SDR one without it looking grey.
- **Zero-copy**: decode, filter and encode all on the GPU so frames never travel through main memory.
- **Window** (Portico's word): one run of FFmpeg that produces a contiguous stretch of segments for one session.

## Scope and sources

### Portico files read

Read in full unless noted. Paths are relative to `/home/justin/projects/Portico`.

| Area | Files |
|---|---|
| Docs | `portico-internal/Project Architecture/04 Playback and Media.md`, `Backlog.md` |
| On-demand HLS engine | `apps/portico-server/server/internal/playback/hls.go`, `hls_timeline.go`, `hls_copy_timeline.go`, `hls_restart.go`, `hls_bad_network.go`, `hls_sweep.go`, `hls_renditions.go`, `hls_burn.go`, `hls_text.go`, `hls_fragmented_failure.go`, `hls_produce_support.go`, `mp4_boxes.go` (outline and key functions), `delivery_plan.go`, `delivery_decision.go` (pipeline-relevant half), `delivery_settings.go`, `quality_ladder.go`, `audio_plan.go` (reservation part), `service.go` (HLS wiring only) |
| Second, opt-in engine | `internal/playback/finite_hls.go` and `internal/playback/vod/encoder.go` (outline only; it is off unless `PORTICO_FINITE_VOD=1`) |
| Argument builders | `internal/decoder/hardware.go`, `burn_in.go`, `command.go`, `analysis.go`, `linear.go`, `linear_hls.go`, `prepared.go`, `toolchain.go`, `subtitle.go`, `run.go`, `hls_window.go` |
| Process execution | `internal/mediaexec/mediaexec.go`, `sandbox_linux.go`, `sandbox_darwin.go`, `posture.go`, `shim.go` (first half), `internal/childprocess/*` |
| Live TV | `internal/playbackruntime/linear_plan.go` (conversion rules only), `internal/decoder/linear_hls.go` |
| Prepared media | `internal/decoder/prepared.go`, `internal/preparedmedia/worker.go` (probe and admission rules only) |
| Host limits, policy | `internal/hostlimits/hostlimits.go`, `internal/workpolicy/policy.go` (head) |
| FFmpeg bundle | `apps/portico-server/third_party/ffmpeg/requirements.v1.json`, `sources.lock.json`, `NOTICE.md`, `scripts/build-ffmpeg.sh`, `.github/workflows/ffmpeg-toolchain.yml` |
| Startup wiring | `server/cmd/server/main.go` (toolchain probe and HLS construction) |

### Comparison files read

| Project | Files |
|---|---|
| Jellyfin server | Read by targeted section, not line by line: `MediaBrowser.Controller/MediaEncoding/EncodingHelper.cs` (keyframe, seek, bitstream-filter, bitrate, audio-filter and input-flag sections read; the per-vendor filter chains of its 8,093 lines were scanned by keyword and otherwise described from prior knowledge of that file), `Jellyfin.Api/Controllers/DynamicHlsController.cs` (segment request, command line and video-argument functions), `Jellyfin.Api/Helpers/DynamicHlsHelper.cs` (master playlist), `src/Jellyfin.MediaEncoding.Hls/Playlist/DynamicHlsPlaylistGenerator.cs` (in full), `MediaBrowser.Model/Configuration/EncodingOptions.cs` (all defaults), `MediaBrowser.Controller/MediaEncoding/DownMixAlgorithmsHelper.cs`. Scanned by keyword only: `MediaBrowser.MediaEncoding/Transcoding/TranscodeManager.cs`, `TranscodingThrottler.cs`, `TranscodingSegmentCleaner.cs`. Not opened: `EncoderValidator.cs`, `EncodingJobInfo.cs` beyond one property, the keyframe extractors. |
| Jellyfin docs | `jellyfin-docs/docs/general/post-install/transcoding/hardware-acceleration/known-issues.md` (read); the per-vendor pages (`intel.md`, `nvidia.md`, `amd.md`, `apple.md`, `rockchip.md`) and `downmix.md` were listed but not read. |
| Others | Plezy, Streamyfin, Swiftfin and the other clients do not produce streams; they were checked only for what stream shapes they request. Navidrome and Audiobookshelf were only spot-checked (Navidrome's transcoding-cache setting, Audiobookshelf's `server/objects/Stream.js`); what is said about them is otherwise general knowledge. Plex and Emby only from general public knowledge. Public web: FFmpeg's filter documentation (to confirm which tone-map filters exist upstream) and one search on jellyfin-ffmpeg's patches. |

### Not covered

- The opt-in "finite" engine (`internal/playback/vod`) was only outlined. It is disabled by default (`server/cmd/server/main.go:670`).
- `internal/producerinput`, `internal/remotemedia`, `internal/mediaartifact`, `internal/artifactlease`, `internal/privateoutput`, `internal/mediatimeline`: only their role was established, not their internals.
- The patched `third_party/hls.js` was not diffed.
- Music conversion (FLAC/Opus with exact trim) is only touched in passing.

## How Portico does it today

### One engine, three kinds of work

All on-demand video goes through `playback.HLS` (`internal/playback/hls.go`). A session's stored **delivery plan** (`playback_delivery_plans`) says what to do with each stream: `video_action` is `copy`, `convert` or `none`; `audio_action` likewise. The plan is fixed when the session is created and FFmpeg arguments are a pure function of it (`delivery_plan.go:795` → `decoder.BuildConversion`, `decoder/hardware.go:561`).

1. **Converted video** (`video_action = convert`). Output is always **H.264, 8-bit, BT.709, in MPEG-TS** (`delivery_plan.go:462-470`, `:622`). The server writes the *whole* playlist itself before FFmpeg starts: every segment is declared as exactly six seconds on a fixed grid (`hls_timeline.go:17`, `:65`). FFmpeg is told to put a keyframe on every grid line (`-force_key_frames expr:gte(t,n_forced*6)`, `hardware.go:731`) and to write segments with FFmpeg's HLS muxer starting at the right number (`hls.go:476-480`).
2. **Copied video** (`video_action = copy`, with audio copied or converted). A copied picture can only be cut where the file already has keyframes. Before the first segment, the server asks `ffprobe` for "the keyframe at or before each six-second line", one seek per line (`hls_copy_timeline.go:141`), builds a **cut list**, caches it per file (`playback_copy_timelines`), and publishes a playlist with each segment's real length. FFmpeg's *segment* muxer (not the HLS muxer) is given the explicit cut list (`hls_copy_timeline.go:305`). Output goes to a staging folder and each finished segment is renamed into place. If the keyframes cannot be indexed within 20 seconds, or any gap between keyframes exceeds 30 seconds, the session is **silently re-planned as a full conversion** (`hls.go:338-346`, `hls_produce_support.go:134`).
3. **Audio-only and alternate audio.** When a file has two or more audio tracks, each track becomes its own audio rendition with its own playlist and its own FFmpeg process, started only when a player actually asks for that track (`hls_renditions.go:320`).

### Seeking is "ask for a different segment"

Because the playlist lists the whole title up front, a seek is just a request for segment *N*. `FileContext` (`hls.go:826`) serves the file if it exists. If not, it calls `start(id, N)`: any running FFmpeg for that session is cancelled and a new one is started with `-ss N×6` *before* the input and `-output_ts_offset N×6`, so the new segments carry the same absolute timestamps the old ones would have (`hls.go:399-409`, `:480`). The request then waits up to 20 seconds for the file; many requests for the same segment share one wait (`hls_bad_network.go:134`). Restarts are limited to 12 per 10 seconds per session.

### Throttle, retention, abandonment

A watcher goroutine ticks every 100–500 ms per window (`hls.go:643`). When produced segments are 60 seconds (default; owner setting 10–3600) ahead of the last segment the player fetched, FFmpeg is **killed**, not paused. The next request past the produced edge starts a new window there. Segments more than 300 seconds (default) behind the player are deleted as playback advances (`hls_timeline.go:131`). A session nobody has fetched from for two minutes loses its FFmpeg but keeps its files (`hls_bad_network.go:35`). A whole-server budget of 24 GiB and a per-session ceiling of 4 GiB are enforced every 30 seconds (`hls_sweep.go:37`, `:65`), and no window starts with under 2 GiB free on the volume.

### Hardware

At start-up (off the request path) each backend — VideoToolbox, NVENC, Quick Sync, VA-API, AMF — is tested with a real half-second encode of a synthetic clip, run exactly as a real job would run including the sandbox (`decoder/hardware.go:319`). The first that works wins. A second probe tests the backend's own tone-map and scale filters; only if that also passes does the plan use GPU decode and GPU filters ("native"). Otherwise frames are decoded and filtered on the CPU and uploaded just for the encode. If a hardware encode fails on a real file, the same window is immediately re-run in software and the stored plan is changed (`hls.go:566-599`); three failures in ten minutes open a circuit breaker for that backend (`decoder/toolchain.go:201-228`).

### HDR

HDR input is detected from the transfer function only (PQ or HLG; `hardware.go:799`). Converted output is always SDR. Tone mapping uses, in order of preference: the GPU backend's own filter; libplacebo on Vulkan (the only route that applies Dolby Vision Profile 5 reshaping); `tonemap_opencl`; or the CPU `zscale`+`tonemap` chain with the owner's chosen curve (default hable). HDR is only *kept* on copy routes, where the master playlist carries `VIDEO-RANGE=PQ|HLG` and, for Dolby Vision 5/8, a `dvh1` sample entry and `SUPPLEMENTAL-CODECS`.

### Audio

Converted audio is AAC stereo at 192 kb/s, AAC 5.1 at 384 kb/s when the device takes it, or E-AC-3/AC-3 5.1 at 640 kb/s for a passthrough output (`delivery_decision.go:347`). Downmixing uses one fixed recipe: FFmpeg's ITU matrix without LFE, normalised, +3 dB, then a limiter (`hardware.go:781`). Copy into MPEG-TS is allowed for AAC, MP3, MP2, AC-3, E-AC-3; into fMP4 also FLAC, Opus, ALAC. DTS and TrueHD are never copied into HLS.

### Process confinement

Every FFmpeg/ffprobe goes through `mediaexec` (`internal/mediaexec/mediaexec.go`). The input is an inherited file descriptor (`/dev/fd/3`) or a private loopback bridge, never a library path. On Linux the process runs inside bubblewrap with no network, an empty read-only root, only FFmpeg's exact libraries, and write access only to the session's output folder; a "hardware" job additionally sees `/dev/dri`, `/dev/nvidia*`, `/sys` and the system library folders (`sandbox_linux.go:22`, `:134-144`). macOS uses `sandbox-exec`. Windows has no sandbox. A small shim applies `RLIMIT_FSIZE` (4 GiB per file), no core dumps, low priority for background jobs, and records the process group in a ledger so the next server start can reap orphans.

### FFmpeg itself

Portico ships its own pinned FFmpeg 8.1.2: BtbN's GPL static build recipe on Linux and Windows, the Homebridge recipe on macOS (`third_party/ffmpeg/sources.lock.json`, `scripts/build-ffmpeg.sh`). It is **unpatched upstream FFmpeg**. A JSON contract lists the codecs, filters and hardware interfaces a build must have.

### Live TV and prepared versions

Live TV uses a separate argument builder (`decoder/linear_hls.go:74`): four-second segments, a 12-segment sliding playlist, `-copyts -start_at_zero`, output PUT to a private gateway. Video is converted (libx264 `veryfast`, two threads, **software only**) whenever the source is not H.264 8-bit 4:2:0 progressive; audio is converted to AAC stereo whenever it is not AAC-LC stereo (`playbackruntime/linear_plan.go:181`, `:228`). Prepared versions (owner "optimize" and offline downloads) use two fixed recipes — 720p at 2.5 Mb/s or 1080p at 5 Mb/s, libx264 `medium`, CRF 21, forced 30 fps, AAC stereo, fragmented MP4 (`decoder/prepared.go:22`).

Path shorthand used below: `server/…` means `apps/portico-server/server/…`.

## How the others do it

### Jellyfin

**One long command builder.** `jellyfin/MediaBrowser.Controller/MediaEncoding/EncodingHelper.cs` (8,093 lines) turns a request plus the server's `EncodingOptions` into one FFmpeg command string. It has a separate filter-chain function for each hardware family — NVIDIA, AMD on Windows (AMF/D3D11), Intel Quick Sync on Windows and Linux, VA-API (with different paths for Intel and AMD drivers), Apple VideoToolbox, Rockchip, and software — and each covers decode, deinterlace, rotate, tone map, scale and subtitle overlay on the GPU where possible.

**Its own FFmpeg.** Jellyfin ships `jellyfin-ffmpeg`, a patched fork. The patches matter: they add `tonemap_cuda`, `tonemap_videotoolbox`, a SIMD software tone mapper `tonemapx`, Dolby Vision handling in the OpenCL/CUDA tone mappers, fixes to VA-API/QSV filters, and a "pause" key used for throttling. The Linux packages also bundle the GPU user-space drivers (Intel media driver, Mesa VA drivers, OpenCL runtime) so hardware acceleration does not depend on what the host has installed.

**HLS shape.** The server writes the playlist itself, like Portico (`jellyfin/src/Jellyfin.MediaEncoding.Hls/Playlist/DynamicHlsPlaylistGenerator.cs`). For conversions every segment is declared equal length: **3 s** by default for a conversion, 6 s for a remux, or whatever the client requests (`jellyfin/MediaBrowser.Controller/Streaming/StreamState.cs:74-105`). For remuxes it uses real keyframe positions *if* it has them — read from the Matroska index on demand for `.mkv` only by default, or pre-computed by a scheduled task for other containers — and otherwise falls back to equal-length segments. FFmpeg's HLS muxer writes the segments with `-copyts -avoid_negative_ts disabled -start_number N` (`jellyfin/Jellyfin.Api/Controllers/DynamicHlsController.cs:1640`). Segments can be MPEG-TS or fMP4 as the client asks.

**Seeking.** A request for a segment that does not exist restarts FFmpeg at that segment only if it is behind the current position or more than 24 seconds (eight 3-second segments) ahead (`DynamicHlsController.cs:1472-1494`); otherwise the request waits.

**Master playlist.** One main variant plus, where useful: a spec-compliant Dolby Vision variant, SDR fallback variants (H.264, and HEVC/AV1 when enabled) for HDR remuxes, a "Level 5.0" entry for older Apple chips, two lower-bitrate variants for remote viewers (a crude adaptive ladder), subtitle renditions, and trickplay image playlists (`jellyfin/Jellyfin.Api/Helpers/DynamicHlsHelper.cs:212-332`, `:714`). It does **not** publish alternate audio renditions; changing audio track restarts the stream.

**Encoders and quality.** Output can be H.264 (default), HEVC or AV1 (both opt-in). Software H.264 uses CRF 23 with a bitrate cap, preset `veryfast` by default, an explicit profile and level, and tuned `x264opts` (`EncodingHelper.cs:1723-1760`, `:2392`). Each hardware encoder gets its own rate-control recipe (`:1652-1723`). Hardware encoders that cannot be forced to key-frame on a timestamp get a fixed GOP instead (`:2060-2074`).

**Throttling and clean-up.** Both are **off by default**. When on, throttling *pauses* FFmpeg by writing a key to its standard input once it is 180 s ahead of the player (`TranscodingThrottler.cs:118-138`); segment deletion removes segments more than 720 s behind (`TranscodingSegmentCleaner.cs:96-109`). A job nobody has pinged for 60 s (HLS) is killed (`TranscodeManager.cs:153-160`). The transcode folder is configurable and commonly pointed at a RAM disk.

**Logs.** Every transcode writes its full command line and all FFmpeg output to its own log file, listed in the dashboard (`TranscodeManager.cs:451-471`).

**No sandbox.** FFmpeg runs as the server user with the library path as an argument.

### Plezy, Streamyfin, Swiftfin, Wholphin and the other clients

None of them produce streams. They ask Jellyfin/Plex/Emby for a stream and play it (Plezy and jellyfin-desktop through mpv, which direct-plays nearly everything and so rarely triggers a transcode at all). They are relevant here only as evidence of which stream shapes real clients request: mostly direct play, then HLS with fMP4 for HEVC, with the device profile carrying a maximum bitrate.

### Navidrome and Audiobookshelf

Navidrome (Go) pipes one FFmpeg per stream to the HTTP response with an owner-editable command template and a size-bounded on-disk transcode cache keyed by track and format. Audiobookshelf produces HLS for audiobooks with a simple fixed command and a per-session folder. Neither has throttling, hardware paths or a sandbox. They are useful mainly as a reminder that a **cache of finished transcodes keyed by content** is a common, cheap feature (see F-03.14).

### Plex (public knowledge only)

Plex's transcoder is also a patched FFmpeg. It produces DASH or HLS, offers HEVC output, hardware tone mapping on Intel/NVIDIA (Linux, Windows), a configurable temp directory, a "transcoder default throttle buffer" (60 s default — the number Portico also uses), and background "Optimize" versions.

## Architectural differences

### A-03.1 Throttle by killing FFmpeg versus pausing it
- **Portico:** When output is 60 s ahead of the last segment fetched, the FFmpeg process is killed; the next request past the edge starts a new process at that point (`server/internal/playback/hls.go:692-707`). A copy window also stops after 900 segments.
- **Jellyfin:** Off by default. When enabled, FFmpeg is paused in place (a key written to stdin) at 180 s ahead and resumed on demand.
- **Why it matters:** Killing frees CPU, GPU session slots (consumer NVIDIA cards allow only a few) and memory completely, and survives server restarts trivially. But every minute of viewing costs a fresh process start, sandbox set-up, input seek and decode-from-previous-keyframe, and the request that triggers the restart blocks until the first new segment exists. On a slow software encode or a slow disk that request can take several seconds; the player normally has ~30 s buffered so it is invisible, but it shrinks the margin on weak boxes.
- **Assessment:** **Trade-off (subjective), leaning Portico ahead.** Deliberate (comment at `hls.go:639-642`). It only works because of the fixed grid (A-03.2), and it is on by default, which Jellyfin's is not. What would change my view: a measured restart latency on a Raspberry-Pi-class box above ~10 s.

### A-03.2 Absolute timestamps via `-output_ts_offset` versus `-copyts`
- **Portico:** Each window seeks with `-ss` before the input, lets FFmpeg reset timestamps to zero, then adds the window's start back with `-output_ts_offset` (`hls.go:399-409`, `:480`). Keyframes are forced with `gte(t,n_forced*6)` where `t` starts at zero in each window.
- **Jellyfin:** `-copyts -avoid_negative_ts disabled` (plus `-start_at_zero` in some cases) so FFmpeg keeps the source clock throughout (`DynamicHlsController.cs:1640`, `:1865`, `:1888-1897`).
- **Why it matters:** Both aim for segments that share one continuous clock so no discontinuity tags are needed. Portico's way keeps `t` small, which makes the `n_forced` keyframe expression safe (see B-03.7 for what happens when that assumption is broken in Live TV). Jellyfin's way is closer to the source but has a long history of edge cases (negative timestamps, files whose first timestamp is not zero, subtitle timing), visible in its special cases.
- **Assessment:** **Trade-off.** Portico's is simpler to reason about. One residual difference: the first window (offset 0) can have slightly negative audio timestamps from AAC encoder priming, which FFmpeg shifts for that window only, so a restarted window is ~21 ms off from window zero. Harmless in practice; noted for completeness.

### A-03.3 Copied video: keyframe "at or before" the grid line, cut by the segment muxer
- **Portico:** One `ffprobe` seek per six-second line asks for the keyframe at or before it; the cut list is cached per file; FFmpeg's *segment* muxer is given explicit cut times; segments are staged and renamed (`hls_copy_timeline.go:141`, `:305`, `:339`).
- **Jellyfin:** Reads all keyframe times (Matroska index, or a full `ffprobe` packet scan), then cuts on the first keyframe *at or after* each target, which is what FFmpeg's HLS muxer does on its own (`DynamicHlsPlaylistGenerator.cs`, `ComputeSegments`). FFmpeg's HLS muxer does the cutting.
- **Why it matters:** Portico's playlist is exact for any container the demuxer can seek in, and the same cut list is forced on every restart, so restarted windows cannot disagree. Jellyfin's is exact only when keyframe data exists; for MP4 and others without the scheduled task it publishes equal-length segments that the remuxed file does not follow, a long-standing source of seek glitches there.
- **Assessment:** **Portico ahead** on correctness. The cost is in A-03.4.

### A-03.4 A source that cannot be indexed is converted instead of remuxed
- **Portico:** If the keyframe probe fails, times out (20 s for the whole title), finds a gap over 30 s, or returns anything odd, the plan is rewritten to a full video conversion (`hls.go:338-346`, `hls_copy_timeline.go:69-84`, `hls_produce_support.go:134`). With conversion disabled by the owner, the session fails.
- **Jellyfin:** Falls back to a remux with an approximate playlist.
- **Why it matters:** A two-hour film is ~1,200 seeks. On a spun-down disk, a network share, or a file whose index is at the end, 20 s may not be enough the first time; the viewer then silently gets an 8 Mb/s H.264 re-encode of a file that could have been copied, and the server pays a full transcode. The result is not cached as "failed", so each new session retries the probe (good), but each retry costs up to 20 s of start-up.
- **Assessment:** **Trade-off, with a risk** (B-03.9). Portico's "never publish a wrong timeline" is defensible. Options: index in the background at scan time (as Jellyfin's task does), read Matroska cues directly (cheap, no seeks), or lengthen the budget when the file is on slow storage. Medium size.

### A-03.5 Segment length and grid
- **Portico:** Fixed 6 s grid, not configurable (`hls_timeline.go:17`). Maximum title length seven days.
- **Jellyfin:** Client may request the segment length; otherwise 3 s when the video is converted and 6 s when it is copied (`StreamState.cs:74-105`); minimum-segments parameter.
- **Why it matters:** The first segment after a start or seek cannot be served until all of it is encoded. On an encoder running near real time, a 6 s segment adds about three seconds to start-up and to every seek compared with a 3 s one. Longer segments are slightly more efficient and mean fewer requests.
- **Assessment:** **Trade-off.** Portico's fixed value removes a class of bugs, and Apple recommends 6 s. Worth measuring start and seek latency on a weak box before freezing it; a 4 s grid for conversions (what Portico's Live TV already uses) would be a middle path. Changing it later is easy only if no client assumes the number.

### A-03.6 Converted output is always H.264 8-bit SDR in MPEG-TS
- **Portico:** The conversion route requires the MPEG-TS transport and H.264 (`delivery_plan.go:462-470`, `:622`). fMP4 is used only for copy routes. The FFmpeg contract requires `libx265` but nothing uses it for delivery.
- **Jellyfin:** H.264 by default; HEVC and AV1 output are owner opt-ins and travel in fMP4; 10-bit HDR can be preserved when re-encoding to HEVC on capable hardware.
- **Why it matters:** For the same picture quality HEVC needs roughly 40–50% fewer bits and AV1 fewer still. That matters most for remote viewing on limited uplinks — exactly when a conversion is happening. It also means any conversion of an HDR title (for bitrate, burn-in or a quality rung) becomes SDR, even on an HDR TV.
- **Assessment:** **Portico behind** (feature F-03.1, F-03.2). Deliberately simple for now, but not recorded as a decision that I found. Large: new encoder argument sets per backend, fMP4 conversion path, client profile checks.

### A-03.7 Bitrate policy: fixed average bitrate versus quality target with a cap
- **Portico:** `-b:v X -maxrate X -bufsize 2X` for every encoder (`server/internal/decoder/hardware.go:726-730`). `X` is the quality rung's bitrate, the policy ceiling, or — when nothing constrains it — the ladder value for the source height: 8 Mb/s for 1080p, 20 Mb/s for 4K (`delivery_plan.go:758-777`). The source's own bitrate is not considered.
- **Jellyfin:** Software H.264 uses CRF 23 capped by `-maxrate`; the cap is the lower of the client's limit and a scaled version of the source bitrate; each hardware encoder gets its own VBR/CBR recipe.
- **Why it matters:** A 1.5 Mb/s 1080p HEVC web file that needs converting only for codec reasons becomes an 8 Mb/s H.264 stream in Portico — five times the bytes, no quality gain, and it may now exceed a remote viewer's link. Conversely a fixed average bitrate wastes bits on easy scenes and starves hard ones compared with CRF-plus-cap.
- **Assessment:** **Portico behind.** Small to medium: use `-crf` with `-maxrate` for libx264, and cap the target at something like the source bitrate times a codec factor.

### A-03.8 No profile, level, GOP or scene-cut control on the on-demand encoder
- **Portico:** On-demand conversion passes only preset, pixel format, bitrate and the keyframe expression (`hardware.go:722-731`). No `-profile:v`, `-level`, `-g`, `-keyint_min`, `-sc_threshold`, `-bf`, `-refs`. (Live TV and prepared versions do set profile/level.)
- **Jellyfin:** Sets profile, level, GOP, `-sc_threshold 0`, and tuned x264 options; per-encoder B-frame and look-ahead settings.
- **Why it matters:** The master playlist *declares* `avc1.640028` (High, level 4.0) or level 5.1 (`hls_renditions.go:114-131`) regardless of what the encoder actually produced. A 1080p60 encode is level 4.2; NVENC defaults to a different profile. Most players ignore the mismatch; strict ones (some smart-TV HLS stacks) check the declared level against their limits. Extra scene-cut keyframes are harmless to the HLS muxer in the normal case but interact badly with early audio (B-03.12).
- **Assessment:** **Portico behind**, small. Set profile/level explicitly from the plan and derive the `CODECS` string from the same values.

### A-03.9 Forcing keyframes on hardware encoders
- **Portico:** The same `-force_key_frames` expression for every encoder; nothing else (`hardware.go:731`).
- **Jellyfin:** Has an explicit list of encoders for which "forcing keyframes is not enough" and sets a fixed GOP instead: `h264_qsv`, `h264_nvenc`, `h264_amf` and their HEVC/AV1 siblings (`EncodingHelper.cs:2060-2074`).
- **Why it matters:** This is the difference behind B-03.1, the most serious finding in this report.
- **Assessment:** **Portico behind.** See B-03.1.

### A-03.10 Hardware selection: probe everything automatically versus owner picks
- **Portico:** Tests each backend with a real encode at start-up through the real sandbox and picks the first that works; owner can override; result shown in diagnostics (`hardware.go:319-390`, `:464-489`).
- **Jellyfin:** The owner chooses the acceleration type and device in settings; Jellyfin validates encoder/decoder/filter names at start-up (`EncoderValidator.cs`) but does not test-encode. Wrong settings surface as failed playback.
- **Assessment:** **Portico ahead.** "Works without configuration and tells you why not" is a real advantage. Keep it.

### A-03.11 GPU decode and GPU filters are all-or-nothing behind one probe
- **Portico:** GPU decode, GPU scaling and GPU tone mapping are enabled together only if a probe of *tone-map + scale* on a synthetic SDR clip succeeds (`hardware.go:369-374`, `:606-634`). Otherwise decode and all filters run on the CPU and frames are uploaded for the encode only. VideoToolbox decodes on the GPU only when there are no filters at all. AMF never does.
- **Jellyfin:** Decides decode, each filter and encode separately per vendor, with documented fallbacks (e.g. GPU decode → OpenCL tone map → GPU encode on AMD).
- **Why it matters:** For the common case — an SDR 4K HEVC file scaled to 1080p — the heavy work is the decode and the scale. If the tone-map probe fails for a reason unrelated to this file, Portico does both on the CPU. See B-03.2 and B-03.3: on Portico's own FFmpeg build the probe very likely fails for both NVIDIA and VA-API, so the "native" path may never be taken on Linux.
- **Assessment:** **Portico behind.** Medium: probe decode, scale and tone map independently, and probe tone mapping with an HDR-tagged synthetic input.

### A-03.12 Stock FFmpeg versus a patched fork with bundled drivers
- **Portico:** Unpatched FFmpeg 8.1.2 from BtbN's static recipe; GPU drivers come from the host (`third_party/ffmpeg/sources.lock.json`, `scripts/build-ffmpeg.sh`). Clear licence posture (GPL v3, no non-free), reproducible, verified against a feature contract.
- **Jellyfin:** `jellyfin-ffmpeg` with dozens of patches and bundled Intel/Mesa/OpenCL user-space drivers.
- **Why it matters:** Portico avoids maintaining a fork — a real saving for one maintainer. The price: no `tonemap_cuda`, no `tonemap_videotoolbox`, no fast software tone mapper, no Dolby Vision reshaping outside libplacebo, and hardware acceleration only works if the owner's distribution has matching VA-API/QSV drivers where a static binary can find them. Jellyfin's "install and it works" hardware story on Linux comes largely from the bundled drivers.
- **Assessment:** **Trade-off (subjective).** An owner can already point `PORTICO_FFMPEG` at jellyfin-ffmpeg (GPL-compatible) and Portico would then find `tonemap_cuda`. Worth documenting as a supported option, and worth a runtime check of whether the static build finds VA-API drivers on stock Ubuntu/Debian/Fedora without `LIBVA_DRIVERS_PATH` (see Evidence boundary).

### A-03.13 Sandbox versus none
- **Portico:** bubblewrap on Linux, `sandbox-exec` on macOS, baseline limits everywhere; input by file descriptor; no network; output only to the session folder (`server/internal/mediaexec/sandbox_linux.go:109-178`).
- **Jellyfin:** None. FFmpeg has the server's full file and network access.
- **Why it matters:** FFmpeg parses hostile input for a living; Portico's containment is a genuine security advantage nobody else in this comparison has. The cost is compatibility: GPU access inside the sandbox must be granted explicitly and has gaps (B-03.4, B-03.5), fonts and drivers must be mapped in, and Docker's default profile disables the sandbox entirely (the server then reports it and continues).
- **Assessment:** **Portico ahead**, deliberate (`04 Playback and Media.md`, "Decoder confinement"). Keep; fix the GPU gaps.

### A-03.14 Tone-mapping choices
- **Portico:** Order: backend-native filter → libplacebo/Vulkan (BT.2390 curve, fixed) → `tonemap_opencl` (hable, fixed) → CPU `zscale`+`tonemap` with the owner's curve (`hardware.go:207-212`, `:683-694`). One owner control: the curve, which only affects the CPU path and `tonemap_cuda`. Tone mapping is **on by default**.
- **Jellyfin:** Tone mapping **off by default**; many controls (algorithm, mode, range, desaturation, peak, parameter, VPP brightness/contrast); default curve BT.2390; software path uses its SIMD `tonemapx`.
- **Why it matters:** On-by-default is right — a grey washed-out picture is a bug to a normal person. But the owner's "algorithm" setting silently does nothing on the Vulkan, OpenCL, VA-API and QSV paths, and the CPU `zscale` chain on 4K is slow (often below real time on mid-range CPUs), which is why Jellyfin wrote `tonemapx`.
- **Assessment:** **Portico ahead on defaults, behind on speed and control.** Either apply the chosen curve everywhere or say in the setting's description that it applies to software tone mapping only.

### A-03.15 Dolby Vision
- **Portico:** Profile 5/8 copied with a `dvh1` sample entry and `SUPPLEMENTAL-CODECS` for 8.1/8.4 (`hls_renditions.go:132-135`, `:221-232`, `:545`). Profile 7 copy is treated as HDR10. Conversion of Profile 5 without libplacebo is flagged to clients as "colours approximate" before play (`delivery_plan.go:652-658`).
- **Jellyfin:** Publishes both a `dvh1` variant and an `hvc1` fallback variant in one master playlist so the player chooses; strips Dolby Vision or HDR10+ metadata with bitstream filters when the client cannot take it (`EncodingHelper.cs:1524-1559`); tone maps Profile 5 correctly with its patched OpenCL/CUDA/VideoToolbox filters.
- **Assessment:** **Mixed.** Portico's "warn before play" is more honest than anything Jellyfin does. Jellyfin's metadata stripping (so a Profile 8 file can be remuxed as clean HDR10 for a TV that chokes on the RPU) is a real gap — see F-03.6.

### A-03.16 HDR detection
- **Portico:** Only the transfer function (PQ or HLG) means HDR; BT.2020 primaries alone do not (`hardware.go:795-815`, with reasoning).
- **Jellyfin:** Derives a `VideoRangeType` from transfer, primaries, bit depth and Dolby Vision side data.
- **Assessment:** **Portico's rule is sound** and well argued. One inconsistency: the prepared-media probe *does* treat BT.2020 primaries alone as HDR and refuses the file (`server/internal/preparedmedia/worker.go:635`). See Doc/code disagreements.

### A-03.17 Deinterlacing
- **Portico:** `bwdif` (or `yadif`) on the CPU, single rate, only frames flagged interlaced, always before any GPU upload; using it disables GPU decode for that session (`hardware.go:622`, `:639-649`).
- **Jellyfin:** Owner picks yadif or bwdif and single or double rate; uses `deinterlace_vaapi`, `deinterlace_qsv`, `yadif_cuda`/`bwdif_cuda`, VideoToolbox variants on the GPU.
- **Why it matters:** Interlaced sources are mostly live TV and DVD rips. Single rate turns 50/60 fields per second into 25/30 frames, which looks noticeably less smooth for sport. Portico's Live TV path cannot use hardware at all (A-03.27), so this is all CPU.
- **Assessment:** **Portico behind**, modest. Double-rate as an option is small; GPU deinterlace is medium.

### A-03.18 Scaling and pixel aspect
- **Portico:** Scales only when the plan has a width or height limit; keeps the stored pixel shape (non-square pixels stay non-square) and rounds to even numbers (`hardware.go:819-837`). With no limit there is no scale filter at all.
- **Jellyfin:** Always computes an output size; converts anamorphic sources to square pixels; forces even dimensions.
- **Why it matters:** Two edge cases. (1) DVD-style anamorphic sources reach the player with a non-square pixel flag; browsers honour it, some TV players do not and show a squashed picture. (2) With no limit and an odd-sized source, libx264 refuses to start (B-03.13).
- **Assessment:** **Portico behind**, small: always emit a scale-to-even, square-pixel step in the software path.

### A-03.19 Rotation
- **Portico:** A rotated source is never copied into HLS (`delivery_decision.go`, `RejectRotation`); conversion relies on FFmpeg's automatic rotation. The master playlist's `RESOLUTION` uses the stored width and height, so a portrait phone clip is declared landscape.
- **Jellyfin:** Passes `-noautorotate` with GPU decode and applies `transpose_*` GPU filters; swaps dimensions in its size maths.
- **Assessment:** **Roughly equal in software; Portico behind on GPU paths**, where automatic rotation cannot run on GPU frames and the session falls back to software (and counts as a hardware failure, B-03.6). Small.

### A-03.20 Audio conversion targets
- **Portico:** AAC stereo 192 kb/s; AAC 5.1 384 kb/s; E-AC-3/AC-3 5.1 640 kb/s for passthrough outputs. Output is 2 or 6 channels only. No sample-rate argument (`hardware.go:751-771`, `delivery_decision.go:347-370`).
- **Jellyfin:** Any of AAC, AC-3, E-AC-3, MP3, Opus, FLAC, DTS, TrueHD, ALAC, Vorbis as the client requests; optional VBR modes; channel count and sample rate from the client profile; per-channel bitrate scaling.
- **Why it matters:** Portico's fixed targets are sensible and cover nearly everyone. Missing: 7.1 is always folded to 5.1; a 96 kHz source is encoded as 96 kHz AAC because nothing resamples to 48 kHz, and many TV and Cast decoders reject AAC above 48 kHz (B-03.15).
- **Assessment:** **Trade-off**, Portico simpler. Add `-ar 48000` when converting (Live TV already does: `linear_hls.go:123`).

### A-03.21 Downmix
- **Portico:** One recipe: FFmpeg's ITU matrix (no LFE), normalised so it cannot clip, +3 dB, limiter (`hardware.go:775-786`).
- **Jellyfin:** Five selectable algorithms (none, Dave750, Night-mode dialogue, RFC 7845, AC-4) and a boost multiplier, default ×2 (`DownMixAlgorithmsHelper.cs:17-30`, `EncodingOptions.cs`).
- **Why it matters:** "Dialogue too quiet in the stereo downmix" is among the most common media-server complaints. Portico's recipe is a good default (normalise, then make-up gain, then limit is more careful than Jellyfin's plain boost). A dialogue-boost option is something users ask for by name.
- **Assessment:** **Portico ahead on the default, behind on choice.** See F-03.9.

### A-03.22 Audio copy into HLS
- **Portico:** MPEG-TS: AAC, MP3, MP2, AC-3, E-AC-3. fMP4: those plus FLAC, Opus, ALAC. DTS and TrueHD are always converted (`delivery_decision.go`, `tsCopyAudio`/`fmp4CopyAudio`).
- **Jellyfin:** Will copy DTS and TrueHD into fMP4 (with `-strict -2`) when the client says it can take them.
- **Assessment:** **Trade-off.** Few HLS players accept DTS/TrueHD; the people who care (AVR passthrough from an Android TV box) mostly direct-play. Trivial.

### A-03.23 Alternate audio as real HLS renditions
- **Portico:** Up to 16 audio tracks published as `EXT-X-MEDIA` renditions, each produced by its own FFmpeg only when requested; switching language is a player-side switch with no restart (`hls_renditions.go:40-93`, `:182-273`, `:320`).
- **Jellyfin:** One muxed audio track; switching restarts the transcode at the current position.
- **Why it matters:** Instant audio switching and correct behaviour for players (AVPlayer, Cast) that only know HLS-native track switching.
- **Assessment:** **Portico ahead.** One detail to verify at run time: audio renditions are always MPEG-TS on the fixed 6 s grid even when the video is fMP4 on a keyframe-derived timeline (`hls_renditions.go:294`, `:426`). The HLS specification permits renditions with different segment boundaries and formats; hls.js and AVPlayer handle it in my experience, but it is an unusual combination worth a device pass.

### A-03.24 Master playlist: single variant, no fallbacks
- **Portico:** Exactly one `EXT-X-STREAM-INF` with `BANDWIDTH`, `CODECS`, `RESOLUTION`, `FRAME-RATE`, `VIDEO-RANGE`; no `AVERAGE-BANDWIDTH`; no alternative variants (`hls_renditions.go:182-273`). Fallback is a server re-plan after the client reports a route failure.
- **Jellyfin:** Several variants so the player itself can fall back (SDR alternatives, Dolby Vision plus plain HEVC, lower bitrates for remote).
- **Why it matters:** Portico's model ("the server decides; the client reports failure; the server re-plans") fits its thin-client principle and "quality changes only when a person asks". Jellyfin's lets a player recover without a round trip but takes the decision away from the server.
- **Assessment:** **Trade-off (subjective); Portico's is deliberate** (principles in the brief). No change suggested, except adding `AVERAGE-BANDWIDTH` (Apple's validator asks for it) — trivial.

### A-03.25 No adaptive bitrate
- **Portico:** One rendition per session. Quality changes are explicit rung selections that discard output and restart (`hls_restart.go:16`).
- **Jellyfin:** Optional two extra lower-bitrate variants for non-LAN clients, each started on demand.
- **Plex:** "Auto quality" adjusts by measured throughput.
- **Assessment:** **Deliberate** ("quality changes only when a person asks; server load never lowers it"). Recorded as a difference; the principle forbids load-driven changes, not network-driven ones a viewer has opted into — see Open questions.

### A-03.26 Restart rule on seek
- **Portico:** Restart unless the requested segment is between the window start and two segments (12 s) past the produced edge (`hls.go:105-107`). At most 12 restarts per 10 s per session; concurrent requests for one segment share one wait (`hls_bad_network.go:40-41`, `:134`).
- **Jellyfin:** Restart if behind the current position or more than 24 s ahead. A per-playlist lock serialises requests; no restart budget.
- **Assessment:** **Portico ahead** on robustness (budget, shared waits, abandoned-conversion reclaim). The 12 s versus 24 s threshold is a tuning choice.

### A-03.27 Live TV conversion is software-only and converts more than necessary
- **Portico:** The live builder calls `BuildConversion` with no hardware probe, so it always uses libx264 with two encoder threads and one decoder thread (`server/internal/decoder/linear_hls.go:82`, `:93`, `:112`). Video is converted whenever it is not H.264 8-bit 4:2:0 progressive; audio whenever it is not AAC-LC mono/stereo (`server/internal/playbackruntime/linear_plan.go:181`, `:228`).
- **Jellyfin:** Live TV uses the same hardware paths as files, copies AC-3 when the client takes it, and can copy MPEG-2/HEVC to capable clients.
- **Why it matters:** North-American and much European broadcast is MPEG-2 or H.264 **interlaced** with AC-3 5.1. In Portico every such channel is a CPU deinterlace plus a software encode, and 5.1 is reduced to stereo, even on a server with an idle GPU and a client that plays AC-3. On a low-power box one or two live channels may be the ceiling. A 4K HEVC IPTV channel decoded on one thread will not reach real time.
- **Assessment:** **Portico behind.** Medium: reuse the on-demand plan (hardware probe, audio copy rules) for live. The decision side belongs to the Live TV auditor; the pipeline limitation is recorded here.

### A-03.28 Live output through an HTTP PUT gateway
- **Portico:** Live FFmpeg PUTs segments to a private loopback endpoint; the server holds closed segments in memory/buffer for all viewers of a channel (`linear_hls.go:125`). One FFmpeg per channel regardless of viewer count.
- **Jellyfin:** One FFmpeg per viewer writing to the transcode folder; a shared tuner stream underneath.
- **Assessment:** **Portico ahead** — sharing one conversion across viewers is the right design for live.

### A-03.29 Temporary storage location
- **Portico:** Always `<state>/hls` (`server/cmd/server/main.go:664`). An owner setting `temporaryDirectory` exists and is validated and health-checked, but the engine never reads it (B-03.10).
- **Jellyfin / Plex:** Configurable transcode directory; RAM disks are a documented, popular choice.
- **Assessment:** **Portico behind** until the setting is wired. Small.

### A-03.30 Disk budgets
- **Portico:** Per-session ceiling 4 GiB, whole-server budget 24 GiB with least-recently-watched eviction, 2 GiB free-space floor before starting, per-file write limit 4 GiB via `RLIMIT_FSIZE` (`hls_sweep.go:37-65`, `mediaexec.go:69`). None are configurable.
- **Jellyfin:** No budgets. Optional trailing deletion. A full disk fails transcodes and, if it is the system disk, the server.
- **Assessment:** **Portico ahead** in intent. The fixed per-session ceiling conflicts with the owner's own retention setting — B-03.8.

### A-03.31 Segment retention default
- **Portico:** Deletes played segments 180 s behind (registry default; 300 s in the engine's own default) — on by default.
- **Jellyfin:** Keeps everything until the session ends unless the owner enables deletion (720 s).
- **Why it matters:** Portico uses far less disk; a rewind of more than three minutes re-converts.
- **Assessment:** **Trade-off**, Portico's default is the more disk-friendly one.

### A-03.32 Idle and end-of-session handling
- **Portico:** FFmpeg is cancelled within about a second of a session becoming stopped/ended/failed/expired (sweep), and after two minutes with no segment fetched; files survive the latter. The process group is killed as a unit; a ledger lets the next start-up reap orphans; shutdown waits for converters (`hls_sweep.go:169-187`, `hls_bad_network.go:35`, `hls.go:261`, `server/internal/childprocess`).
- **Jellyfin:** A kill timer fires 60 s after the last ping/segment request; asks FFmpeg to quit, then kills; deletes the session's files; wipes the transcode folder at start-up.
- **Assessment:** **Portico ahead** — more precise, and the orphan ledger and parent-death signal cover crashes Jellyfin leaves to luck.

### A-03.33 Concurrency limits and queueing
- **Portico:** Optional owner caps on total, hardware and software conversions, counted per *session with a conversion plan*; over the cap the new session is refused with a typed error. No queue. Default unlimited (`audio_plan.go:45-62`).
- **Jellyfin:** No limit on interactive transcodes at all (only a per-user stream limit and parallel image-encoding limit).
- **Plex:** A "maximum simultaneous transcodes" setting.
- **Assessment:** **Portico ahead.** Note the count is of sessions, not running processes, so a paused or throttled viewer still holds a slot — reasonable, but worth knowing.

### A-03.34 Failure diagnostics
- **Portico:** Keeps the last 4 KiB of FFmpeg's error output, strips paths, classifies it into a small set of codes, and stores up to 2,000 characters on the session's artifact row (`hls_produce_support.go:38-106`). FFmpeg runs at `-loglevel error`. The command line is not recorded.
- **Jellyfin:** A complete log file per transcode: the exact command line, the stream mapping, every warning and progress line.
- **Why it matters:** When a conversion misbehaves without failing — wrong colours, no audio, slow — there is nothing to look at in Portico. When it does fail, the stored record is deleted almost immediately (B-03.11).
- **Assessment:** **Portico behind.** Small to medium: keep a bounded ring of recent conversion records (arguments with paths redacted, exit status, error tail, speed) independent of the session's lifetime.

### A-03.35 Hardware failure handling
- **Portico:** Any failure of a hardware-backed window re-runs it in software, rewrites the plan, and counts toward a per-backend circuit breaker: three failures in ten minutes disable the backend until a fresh probe passes (`hls.go:566-599`, `server/internal/decoder/toolchain.go:201-241`).
- **Jellyfin:** No automatic fallback. The transcode fails; the client may retry with different parameters.
- **Assessment:** **Portico ahead** — invisible fallback is what a normal person wants. The breaker cannot tell a bad GPU from a bad file (B-03.6).

### A-03.36 Input handling for odd sources
- **Portico:** No input flags beyond the protocol whitelist and `-ss`; default probe size; `-max_muxing_queue_size 1024`.
- **Jellyfin:** `-fflags +genpts` when copying video, optional `+igndts`, `+discardcorrupt`, `+fastseek`; configurable `-analyzeduration`/`-probesize`; `-max_muxing_queue_size 2048`; `-max_delay 5000000`; removes metadata and chapters from output (`EncodingHelper.cs:7445-7472`, `DynamicHlsController.cs:1640`).
- **Why it matters:** These flags exist because of AVI/WMV/old MPEG files without presentation timestamps and broadcast captures with corrupt packets. Portico's copy path will fail on such a file and rely on the client-reported route failure to re-plan as a conversion; its conversion path mostly copes because decoding regenerates timestamps.
- **Assessment:** **Portico behind**, small. `+genpts` on copy routes and `+discardcorrupt` for MPEG-TS inputs are cheap.

### A-03.37 Stream-copied audio next to converted video
- **Portico:** When the picture is converted but the device can take the original audio, audio is copied (`delivery_plan.go:623-629`).
- **Jellyfin:** Same, but with a recent fix: after a seek, copied audio starts at the previous keyframe — possibly seconds before the picture — so Jellyfin drops those packets with a bitstream filter, and offers an owner switch to convert the audio instead (`EncodingHelper.cs:1593-1622`, `EncodingOptions.HlsAudioSeekStrategy`).
- **Assessment:** **Unclear** — Portico has the same mechanism and no trim. See B-03.12.

### A-03.38 fMP4 handled by the server, not just FFmpeg
- **Portico:** For copied fMP4, each staged file is split into init and media parts, timestamps are rewritten in the `tfdt` boxes, and a fixed one-second edit list is installed so every window's init is byte-identical (`mp4_boxes.go:114-390`). If a later window's init differs, the whole presentation is retired and re-planned as a conversion (`hls_fragmented_failure.go`).
- **Jellyfin:** Lets FFmpeg's HLS muxer write init and segments with `movflags=+frag_discont+skip_sidx`.
- **Why it matters:** Portico's approach gives a stable init across restarts — something Jellyfin gets wrong in places — at the cost of owning a small MP4 rewriter, and of converting files whose parameter sets change mid-stream (some broadcast recordings and concatenated files).
- **Assessment:** **Trade-off**, Portico more correct and more code to own.

### A-03.39 Session-private output, no reuse
- **Portico and Jellyfin:** Both write to a per-session folder; nothing is shared between two people watching the same title at the same settings, and nothing survives the session.
- **Plex:** Same for live transcodes; "Optimized versions" are the reuse mechanism. Portico's prepared media is the equivalent.
- **Assessment:** **Equal.**

### A-03.40 Prepared versions: two fixed recipes
- **Portico:** 720p/2.5 Mb/s or 1080p/5 Mb/s, libx264 `medium`, CRF 21 capped, **forced 30 fps**, AAC stereo, software only, two threads; HDR sources and files with more than one video stream are refused (`server/internal/decoder/prepared.go:22-52`, `server/internal/preparedmedia/worker.go:662`).
- **Jellyfin:** No server-side optimised versions (downloads transcode on the fly). **Plex:** Optimize with device presets and custom quality.
- **Assessment:** **Portico ahead of Jellyfin for having it; the recipe has problems** — B-03.14 (30 fps) and F-03.11 (HDR titles cannot be prepared).

### A-03.41 Trickplay and still extraction
- **Portico:** Full software decode of every frame on one thread, `fps` filter, raw RGB to a pipe; no tone mapping (`server/internal/decoder/analysis.go:39`, `:79`, `:98`).
- **Jellyfin:** Options for hardware decode, key-frames-only extraction (`-skip_frame nokey`, an order of magnitude faster), hardware MJPEG encode, thread count, process priority, and tone mapping of HDR sources.
- **Assessment:** **Portico behind** on speed and on HDR thumbnails (they will look grey). Detailed comparison belongs to the trickplay auditor; listed here because it shares the pipeline.

### A-03.42 Threads
- **Portico:** On-demand conversion sets no thread count (FFmpeg uses all cores). Live uses 1 decode + 2 encode threads. Background jobs use 1–2 and low priority.
- **Jellyfin:** `EncodingThreadCount` owner setting (default auto).
- **Assessment:** **Trivial**, but an owner-visible thread cap is a common request on shared boxes.

### A-03.43 A second, dormant engine
- **Portico:** `internal/playback/vod` ("finite", policy `finite_cfr_sdr_grid_v4`) encodes each six-second interval independently, limited to two-hour titles, and when enabled **forces every HLS session to full conversion** (`server/internal/playback/service.go:384-391`). It is off unless `PORTICO_FINITE_VOD=1`.
- **Others:** No equivalent.
- **Assessment:** **Unclear.** It is ~650 lines plus `finite_hls.go` (471 lines) of unexercised code on the hot path's switch points (`hls.go:772-781`, `service.go:851`). Decide whether it is the future or should be removed before the architecture is frozen (Open questions).

### A-03.44 Seek position for a copied stream
- **Portico:** Knows the exact keyframe time from its cut list and seeks 2 ms past it so the demuxer cannot land on the keyframe before (`server/internal/playback/hls.go:400-408`).
- **Jellyfin:** Adds a fixed half second to the requested time when remuxing, "to get the exact keyframe on most videos" (`jellyfin/MediaBrowser.Controller/MediaEncoding/EncodingHelper.cs:3107-3112`), and clamps seeks to five seconds before the end so the muxer always gets packets.
- **Assessment:** **Portico ahead** — exact rather than "most videos". Jellyfin's end-of-file clamp is worth copying as a guard: Portico refuses out-of-range segment numbers, which covers the same case differently.

### A-03.45 Remote sources reach FFmpeg through a private bridge
- **Portico:** Local files are passed as an open descriptor; remote objects are served to FFmpeg over a loopback HTTP bridge with a one-time token path, and the protocol whitelist is narrowed to `http,tcp` for that job (`hls.go:394-397`, `server/internal/decoder/command.go:30`, `:58-72`). FFmpeg never sees a provider URL, credential or library path.
- **Jellyfin:** Gives FFmpeg the file path or the remote URL (with user-agent/referer flags) directly. Cloud storage is normally an rclone mount, i.e. a path.
- **Why it matters:** Portico's approach keeps secrets and network reach away from the decoder. The cost is that every remote byte is proxied by the server, each FFmpeg seek is a new ranged request through the bridge, and this job type currently lacks GPU access and error text (B-03.5).
- **Assessment:** **Portico ahead on safety, behind on completeness** until B-03.5 is fixed.

### A-03.46 Rate limiting a remux
- **Portico:** No read-rate limit on on-demand copies; the watcher (100 ms tick) kills the process once it is 60 s ahead, and a copy window is capped at 900 segments.
- **Jellyfin:** When segment deletion is on, limits remux input to 10× real time (`-readrate 10`) "to prevent ffmpeg from exiting prematurely (due to fast drive)" (`EncodingHelper.cs:7424-7441`).
- **Why it matters:** On a fast SSD a remux can write minutes of video between two watcher ticks. Portico tolerates that (segments are staged and promoted, and the throttle catches up), at the price of a burst of disk writes larger than the configured buffer.
- **Assessment:** **Trivial.** Portico's Library Channel path already uses `-readrate 1` (`linear_hls.go:88`); adding a modest `-readrate` to on-demand copies would make the buffer setting exact.

### A-03.47 Consumer NVIDIA session limits
- **Portico:** Killing FFmpeg on throttle and on abandonment releases NVENC sessions promptly. An owner cap `maxHardwareSessions` exists (default unlimited). If a session is refused by the driver, the window falls back to software — and counts toward the circuit breaker (B-03.6).
- **Jellyfin:** Documents the limit (3 to 12 concurrent sessions depending on driver version; `jellyfin-docs/…/hardware-acceleration/known-issues.md`) and leaves it to the owner; paused transcodes keep their session.
- **Assessment:** **Portico ahead in mechanism**, but the breaker interaction means the fourth viewer on an older driver can switch hardware off for the first three. Fixing B-03.6 resolves it.

### A-03.48 Dependence on host GPU drivers
- **Portico:** Uses whatever VA-API/Quick Sync/NVIDIA user-space the host provides (A-03.12).
- **Jellyfin:** Its documentation lists what that costs in practice: Fedora, openSUSE and Manjaro ship Mesa with H.264/HEVC VA-API disabled; old LTS kernels lack firmware or have i915 bugs for recent Intel chips; specific Intel compute-runtime and Windows driver versions break tone mapping. Jellyfin's answer is "use our Docker image", which carries its own drivers.
- **Why it matters:** Portico's start-up probe turns each of these into "hardware unavailable, using software" rather than failed playback — good — but the owner is not told *why* beyond a probe code. On Fedora-family hosts AMD hardware encoding will simply never appear.
- **Assessment:** **Trade-off.** A short "why hardware encoding is unavailable" explainer keyed to the probe's error text (driver missing, codec disabled, permission on `/dev/dri`, sandbox) would cover most support questions. Small.

### A-03.49 Platform coverage
- **Portico:** Bundled FFmpeg for Linux x64/arm64, Windows x64/arm64, macOS arm64/x64. Hardware: VideoToolbox (macOS), VA-API and Quick Sync (Linux), Quick Sync/NVENC/AMF (Windows), NVENC (Linux). Sandbox on Linux and macOS only. Raspberry Pi and other ARM boards: software only.
- **Jellyfin:** Same families plus Rockchip, D3D11-based decode/filter paths on Windows, and legacy V4L2 (deprecated; the Pi 5 has no hardware encoder at all). Packages for most distributions, Synology/QNAP via Docker.
- **Assessment:** **Close.** The practical gaps are Rockchip (F-03.4) and Windows GPU decode/filter (A-03.11).

### A-03.50 Process priority
- **Portico:** Background media jobs (analysis, trickplay, prepared versions) run at the lowest OS priority via the limits shim; interactive conversions at normal priority (`server/internal/mediaexec/shim.go`, `Job.Background`).
- **Jellyfin:** No priority management for transcodes; trickplay has a process-priority option.
- **Assessment:** **Portico ahead**, and consistent with its "deprioritised, never paused" principle.

## Missing features and capabilities to consider

### F-03.1 HEVC (and later AV1) as a conversion output
- **What:** Encode to HEVC when the device plays it, delivered as fMP4 HLS.
- **Who has it:** Jellyfin (owner opt-in `AllowHevcEncoding`/`AllowAv1Encoding`, all hardware families plus libx265/SVT-AV1), Plex, Emby.
- **Value:** Roughly half the bitrate for the same picture — the single biggest lever for remote viewing over a home uplink. Almost every phone, TV box and Safari plays HEVC.
- **Size:** Large. Per-backend encoder arguments, fMP4 on the conversion path, `hvc1` tagging, client profile checks, new rung bitrates.
- **Conflicts:** None with principles. Software HEVC encoding is too slow for weak boxes, so it should be hardware-only by default.
- **Opinion:** The most valuable pipeline feature Portico lacks. Design the plan and argument builder now so the output codec is a variable, even if only H.264 ships at release.

### F-03.2 Keeping HDR when a conversion is needed
- **What:** HDR10/HLG in, 10-bit HEVC HDR out (resolution or bitrate reduced, colour untouched).
- **Who has it:** Jellyfin on QSV/NVENC/VA-API/VideoToolbox when the client supports the range; Plex on some platforms.
- **Value:** Today, choosing "1080p" on a 4K HDR title, or watching it remotely on a capped link, turns it into SDR on an HDR TV.
- **Size:** Medium once F-03.1 exists.
- **Opinion:** Follows naturally from F-03.1.

### F-03.3 GPU decode and GPU scaling independent of tone mapping
- **What:** Use the GPU for decode and scale whenever those work, whatever the tone-map probe says.
- **Who has it:** Jellyfin, Plex.
- **Value:** Without it a GPU "hardware" session still burns a CPU core or more on decode and scaling; 4K sources may not reach real time.
- **Size:** Medium. See A-03.11, B-03.2, B-03.3.
- **Opinion:** Needed for the "weak box" end of Portico's stated hardware range.

### F-03.4 More hardware families
- **What:** Rockchip (RK3588 boards are now a popular low-power server), Intel/AMD on Windows via D3D11 decode with AMF/QSV filters, VideoToolbox scaling and tone mapping.
- **Who has it:** Jellyfin (RKMPP path with RGA scaler; `tonemap_videotoolbox`, `scale_vt` via patches).
- **Value:** Raspberry Pi 5 has no hardware encoder, so "ARM box" increasingly means Rockchip. On Windows, AMD cards get encode only.
- **Size:** Medium each; Rockchip needs FFmpeg patches or a different build.
- **Opinion:** Lower priority than making the existing five backends fully work.

### F-03.5 Background keyframe indexing
- **What:** Extract keyframe positions for remux at scan time or in a scheduled background job, instead of at first play.
- **Who has it:** Jellyfin ("Keyframe Extractor" task; Matroska cues read directly).
- **Value:** Removes the up-to-20-second first-play probe and the silent fall-back to conversion on slow storage (A-03.4).
- **Size:** Small to medium. The table (`playback_copy_timelines`) and probe exist; it needs a background step and, ideally, a Matroska cues reader.
- **Conflicts:** Fits "library-wide work happens in background batches".
- **Opinion:** Worth doing.

### F-03.6 Stripping Dolby Vision / HDR10+ metadata on remux
- **What:** When a device takes HDR10 but mishandles Dolby Vision RPUs or HDR10+ SEI, remove them with a bitstream filter while copying.
- **Who has it:** Jellyfin (`hevc_metadata=remove_dovi=1` / `dovi_rpu=strip=1`, `remove_hdr10plus=1`; `EncodingHelper.cs:1524-1559`).
- **Value:** Fixes green/purple or black-screen playback on certain TVs without a transcode.
- **Size:** Small (one `-bsf:v` on the copy path, driven by the client profile).
- **Opinion:** Cheap and effective.

### F-03.7 A per-conversion log the owner can read
- **What:** The exact FFmpeg arguments (paths redacted), exit code, error tail, achieved speed and which stages ran on the GPU, kept for the last N conversions regardless of session lifetime.
- **Who has it:** Jellyfin (a file per transcode), Plex (transcoder logs).
- **Value:** The first thing anyone asks for when helping an owner with playback. Portico already records stages (`playback_hls_stages`) and a failure tail; both die with the session.
- **Size:** Small.
- **Opinion:** Do it before release; it will pay for itself in the first week of support.

### F-03.8 Conversion speed and progress reporting
- **What:** Live "this conversion is running at 1.4× / 38 fps", shown on Now Playing, and a warning when it is below 1×.
- **Who has it:** Jellyfin (parses FFmpeg progress; shows fps and percentage), Plex (speed number and "throttled" state), Tautulli.
- **Value:** Tells an owner at a glance whether the box is keeping up. Portico cannot currently say "this stream is buffering because the encoder is too slow".
- **Size:** Small: `-progress pipe:` or compare produced segments with wall time (the watcher already knows `produced` every tick).
- **Opinion:** Worth doing with F-03.7.

### F-03.9 Downmix choices and dialogue boost
- **What:** An owner (or per-device) choice of stereo downmix: standard, dialogue-forward "night mode", with-LFE; and a boost amount.
- **Who has it:** Jellyfin (five algorithms plus boost), Plex (per-client "boost"), Emby.
- **Value:** Addresses "can't hear dialogue". Portico's fixed recipe is good but not adjustable.
- **Size:** Small.
- **Opinion:** A single "Boost dialogue in stereo" switch would cover most of the demand.

### F-03.10 Double-rate and GPU deinterlacing
- **What:** 50/60 fps output from interlaced sport; GPU deinterlacers.
- **Who has it:** Jellyfin.
- **Size:** Small (option) / medium (GPU filters).
- **Opinion:** Matters for Live TV, which Portico promotes.

### F-03.11 Prepared versions for HDR titles, with more recipes
- **What:** Allow "optimize"/download preparation of HDR sources (tone mapped, or kept as HEVC HDR), and an "original quality, compatible container" remux recipe.
- **Who has it:** Plex Optimize; Infuse/Plex downloads.
- **Value:** Today a 4K HDR library — the common case for enthusiasts — cannot be prepared for offline at all (`preparedmedia/worker.go:662`).
- **Size:** Medium.
- **Opinion:** A visible gap for the downloads feature.

### F-03.12 Faster trickplay generation
- **What:** Key-frames-only extraction, GPU decode, tone mapping of HDR thumbnails.
- **Who has it:** Jellyfin (all three as options).
- **Value:** Turns hours per 4K title into minutes on first import; correct-looking thumbnails for HDR.
- **Size:** Small for `-skip_frame nokey` (with the accuracy trade-off), medium for GPU.
- **Opinion:** Key-frames-only should be the default for a multi-million-item target.

### F-03.13 Configurable transcode directory, including RAM disk
- **What:** Honour the existing `temporaryDirectory` setting.
- **Who has it:** Jellyfin, Plex, Emby.
- **Value:** SSD wear, speed on NAS boxes whose state folder is on spinning disks, and keeping bulky temp files off the volume that holds the database.
- **Size:** Small — the setting, validation and health check already exist.
- **Opinion:** Do it (see B-03.10).

### F-03.14 Reuse of a finished conversion
- **What:** Keep a completed session's output keyed by (file identity, plan) so a second viewer, or the same viewer tomorrow, does not re-encode.
- **Who has it:** Navidrome (transcode cache with size bound), Plex (optimized versions as the manual form).
- **Value:** Popular titles on a shared server; resume next day.
- **Size:** Medium. Segments are already immutable and plan-deterministic; identity and eviction (`hls_sweep.go`) exist per session, not per content.
- **Conflicts:** "No time-based caches of derived results" — this would be keyed by exact revision, so it fits.
- **Opinion:** Not urgent; note that the design already makes it possible.

### F-03.15 Trickplay images in the HLS master playlist
- **What:** `EXT-X-IMAGE-STREAM-INF` so HLS-native players show scrub thumbnails without a custom API.
- **Who has it:** Jellyfin (`DynamicHlsHelper.cs:714-722`).
- **Value:** Mostly for third-party or Roku-style players. Portico's first-party clients use its own route.
- **Size:** Small.
- **Opinion:** Low priority.

### F-03.16 Owner controls Jellyfin exposes that Portico does not
Each is a real-world need someone had; listed for completeness with my view in brackets.
- Encoder thread count (useful on shared hosts).
- Hardware *decode* per codec on/off, and 10-bit/12-bit toggles (needed when a specific GPU mis-decodes one codec; Portico's answer is automatic fallback, which is better if B-03.6 is fixed).
- Intel low-power encoder mode (required on some recent Intel chips for HEVC; relevant with F-03.1).
- Tone-map peak, desaturation, range (enthusiast; skip).
- CRF values per codec (skip; expose a three-step quality choice instead if anything).
- Audio VBR (skip).
- Fallback font folder for burned-in subtitles (auditor 04).
- "Allow on-demand keyframe extraction for these extensions" (Portico needs no such switch).
- Max muxing queue size (keep internal).

### F-03.17 Adaptive bitrate for remote viewers
- **What:** Several bitrates of the same title so the player adapts to the network.
- **Who has it:** Jellyfin (crude, two extra variants, non-LAN only), Plex (automatic quality).
- **Conflicts:** Portico's recorded principle is that quality changes only when a person asks.
- **Opinion:** Not recommended as a default. If ever wanted, it should be a viewer-chosen "Automatic (adapts to connection)" rung, and it multiplies encoder load.

### F-03.18 Graceful stop of FFmpeg
- **What:** Ask FFmpeg to finish (`q`) before killing it.
- **Who has it:** Jellyfin.
- **Value:** For HLS with `temp_file` and staged renames it changes nothing; for a future single-file output it would matter.
- **Opinion:** Not needed now.

### F-03.19 DVD and Blu-ray folder / ISO sources
- **What:** Play `VIDEO_TS` and `BDMV` folders and disc images by feeding FFmpeg a concatenated or `bluray:` input.
- **Who has it:** Jellyfin (concat of VOB/M2TS files; ISO via libbluray/libdvdnav), Kodi, Plex (partially).
- **Value:** Collectors with untouched disc rips.
- **Size:** Medium. Portico's FFmpeg contract already lists `libbluray`/`libdvdnav` as optional, but the protocol whitelist (`file,pipe`) and descriptor-only input rule this out today; it would need a deliberate exception or a server-side concatenating bridge.
- **Opinion:** Niche; note it so the input model (one descriptor per job) is a conscious choice. Mostly a library-scanning question for another auditor.

## Possible bugs and risks in Portico

Severity is my estimate of user impact if the behaviour is as read. "Backlog" says whether `Backlog.md` already lists it — none of these are listed.

### B-03.1 NVIDIA, Quick Sync and AMD encodes probably do not cut on the six-second grid
- **What goes wrong:** Portico relies on `-force_key_frames expr:gte(t,n_forced*6)` to make every encoder start a new independently decodable frame exactly on each grid line (`server/internal/decoder/hardware.go:731`). In FFmpeg, `h264_nvenc`, `h264_qsv` and `h264_amf` turn a forced keyframe into a plain intra frame, **not** an IDR frame, unless their `forced-idr`/`forced_idr` option is set — and Portico never sets it (no occurrence of that option, `-g` or `-idr_interval` anywhere in the server). FFmpeg's HLS muxer only starts a new segment on a packet flagged as a keyframe, which for these encoders means the encoder's own periodic IDR (every 250 frames by default: 10.4 s at 24 fps, 8.3 s at 30 fps, 4.2 s at 60 fps).
- **Consequence if so:** Segment files no longer match the published playlist. Each file holds one encoder GOP rather than six seconds, so a two-hour film declared as 1,200 six-second segments is written as roughly 690 ten-second files with the same numbering. Players that trust the playlist (AVPlayer on Apple devices, Cast, smart TVs) will seek to the wrong place and may stall or loop near the end; a seek restarts the encoder at `N × 6 s` and writes file `N` with content that overlaps or skips relative to its neighbours. hls.js hides much of this by re-timing from the timestamps, which may be why it has not been noticed.
- **Evidence:** `hardware.go:722-731` (identical arguments for all backends); `hls.go:476-480` (HLS muxer, no `split_by_time`); `hls_timeline.go:65-83` (every segment declared 6.000000 s). Jellyfin's explicit special case: "Unable to force key frames using these encoders, set key frames by GOP" (`jellyfin/MediaBrowser.Controller/MediaEncoding/EncodingHelper.cs:2060-2074`).
- **How found:** Jellyfin lists exactly these encoders as exceptions.
- **Not affected:** libx264, `h264_vaapi` and `h264_videotoolbox` honour forced keyframes as IDR. Portico's development hosts are a Mac and Ubuntu minis (likely VideoToolbox / VA-API / QSV), so NVENC and AMF may never have been exercised. Quick Sync is first in the Linux probe order ahead of VA-API on Intel, so Intel hosts may be affected.
- **Likely fix:** add `-forced-idr 1` (NVENC), `-forced_idr 1` (QSV, AMF), and also set `-g`/`-keyint_min` as a belt-and-braces bound; add a start-up self-test that encodes 13 s through each available backend and checks that three segments come out.
- **Confidence:** Probable — from FFmpeg encoder behaviour I know well plus Jellyfin's corroborating special case; needs one run on each of NVENC, QSV and AMF. **Severity:** High. **Backlog:** no.

### B-03.2 The NVIDIA "native" path can never be enabled with Portico's own FFmpeg
- **What goes wrong:** GPU decode and GPU scaling for NVENC are enabled only if a probe of `tonemap_cuda` + `scale_cuda` succeeds (`hardware.go:168`, `:369-374`, `:606-610`). `tonemap_cuda` does not exist in upstream FFmpeg — it is a jellyfin-ffmpeg patch (FFmpeg's published filter list has `scale_cuda`, `yadif_cuda`, `bwdif_cuda`, `overlay_cuda`, `colorspace_cuda`, etc., and only `tonemap`, `tonemap_opencl`, `tonemap_vaapi`). Portico bundles unpatched FFmpeg (`third_party/ffmpeg/sources.lock.json`).
- **Consequence:** On every NVIDIA host using the bundled FFmpeg, `NativeFilters` is false: all decoding, scaling and tone mapping run on the CPU and only the final encode uses the GPU. HDR tone mapping falls to the CPU `zscale` chain because the Vulkan/OpenCL alternatives are skipped for backends that need an upload (`hardware.go:684-693`, `spec.upload == ""` condition).
- **Evidence:** as cited; requirement contract `third_party/ffmpeg/requirements.v1.json` lists `hw.nvidia` as `cuda`/`nvenc` only.
- **Confidence:** Confirmed by reading (and against FFmpeg's public filter documentation); the performance impact is a runtime matter. **Severity:** Medium (works, but a GPU owner gets a fraction of the benefit; 4K HDR conversions may not reach real time). **Backlog:** no.

### B-03.3 The VA-API "native" probe very likely fails on every host
- **What goes wrong:** The same probe for VA-API runs `tonemap_vaapi` on a synthetic **SDR** test pattern (`hardware.go:152`, `:284-302`, `:369-374`). FFmpeg's `tonemap_vaapi` returns an error when the input frame has no HDR mastering-display metadata ("No mastering display data from input"). A test pattern has none.
- **Consequence:** `NativeFilters` false on Intel and AMD VA-API hosts too: CPU decode and CPU scaling for every session, GPU used for encode only. The same doubt applies to `vpp_qsv=tonemap=1` on SDR input (not verified either way).
- **Second consequence at run time:** real HDR files without mastering-display metadata (all HLG, many phone clips, some Dolby Vision Profile 5 files) would make `tonemap_vaapi` fail even if the probe passed; Portico would then fall back to software for that session (good) and count a hardware failure (see B-03.6).
- **Evidence:** as cited; FFmpeg `libavfilter/vf_tonemap_vaapi.c`.
- **Confidence:** Probable (FFmpeg behaviour from source; Portico's probe from reading). **Severity:** Medium. **Backlog:** no.

### B-03.4 Software HDR conversions fail inside the Linux sandbox when Vulkan (or OpenCL) was detected
- **What goes wrong:** At start-up the server checks whether libplacebo-on-Vulkan works, running that check **with** GPU access (`server/internal/decoder/toolchain.go:148-162`, `Hardware: true`). If it works, every *software-backend* HDR conversion is built with `-init_hw_device vulkan=tone … libplacebo …` (`hardware.go:684-687`). But the conversion job is given GPU devices and driver libraries only when the plan's backend is a hardware encoder (`server/internal/playback/hls.go:532`). A software plan therefore runs in a sandbox with no `/dev/dri`, no `/usr/share/vulkan`, no driver libraries (`server/internal/mediaexec/sandbox_linux.go:134-144` are all under `if job.Hardware`), so Vulkan initialisation fails and FFmpeg exits. There is no retry for a software plan, so the session fails with `converter_failed`.
- **When:** Linux with bubblewrap active, a working Vulkan driver on the host (any Mesa GPU driver, or Mesa's CPU "lavapipe"), an HDR source needing conversion, and the software backend in use. The software backend is used when no hardware encoder probes clean (Raspberry Pi 4/5 and many ARM boards have Vulkan but no supported encoder; older NVIDIA cards without NVENC; VMs with lavapipe), when the owner selects "software", during the first seconds after start while probes are "warming" (`hardware.go:434-437`), **and after a hardware failure fallback** (`hls.go:566-572` switches the plan to software — so the fallback itself fails for HDR titles).
- **Also:** where only lavapipe is present and the sandbox is off, Portico will prefer CPU-emulated Vulkan over the CPU `zscale` chain, which is likely much slower.
- **Evidence:** as cited. The same applies to the `tonemap_opencl` branch (`hardware.go:688-691`).
- **Confidence:** Probable (straightforward from reading; needs one run to confirm the failure mode). **Severity:** High where the conditions hold. **Backlog:** no.
- **Likely fix:** set `Hardware` on the job whenever the built graph initialises any hardware device, not only when the encoder is hardware; and prefer `zscale` over lavapipe.

### B-03.5 Remote-source conversions get no GPU access and no error text
- **What goes wrong:** Sources whose container is `strm` (remote objects) run through `decoder.RunHLSWindow` instead of the normal executor (`server/internal/subtitlevideo/window_input.go:43`, `server/internal/decoder/hls_window.go`). That job type has no hardware flag at all (`server/internal/decoder/command.go:95-103`), and its Linux confinement mounts no GPU devices. A plan that selected a hardware backend therefore fails on first run, is retried in software, and records a hardware failure. The run function returns an empty diagnostic for this path (`hls.go:504-513`), so a real failure is stored with no FFmpeg text.
- **Consequence:** every remote-source conversion pays a failed start; three in ten minutes open the circuit breaker and switch hardware encoding off for local files as well (B-03.6).
- **Confidence:** Probable. **Severity:** Medium (only for owners using remote sources on a sandboxed Linux GPU host). **Backlog:** no.

### B-03.6 The hardware circuit breaker cannot tell a bad GPU from a bad file
- **What goes wrong:** Any non-zero FFmpeg exit on a hardware plan is treated as a hardware failure: the window is re-run in software and `RecordHardwareFailure` is called (`hls.go:566-571`, `hls_produce_support.go:121-128`). Three in ten minutes disable the backend for everyone until a re-probe passes (`decoder/toolchain.go:201-228`).
- **Consequence:** Three corrupt files, three HLG clips on VA-API (B-03.3), three rotated clips on a GPU-decode path, three remote sources (B-03.5), three viewers beyond a consumer NVIDIA card's concurrent-session limit, or one viewer retrying one broken file three times, turn off hardware encoding server-wide. On a box sized for hardware encoding that can push every active viewer to software at once.
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** no.
- **Likely fix:** count a failure against the backend only when the software re-run of the same window *succeeds* (which proves the file was fine).

### B-03.7 Live/Library-Channel conversions: the safe keyframe expression is overridden by the unsafe one
- **What goes wrong:** The live builder first adds `-force_key_frames expr:if(isnan(prev_forced_t),1,gte(t,prev_forced_t+4))` — the form that is correct when timestamps do not start at zero — and then appends the generic conversion arguments, which contain a second `-force_key_frames expr:gte(t,n_forced*4)` (`server/internal/decoder/linear_hls.go:112-113`; the second comes from `hardware.go:731` via `BuildConversion` at `linear_hls.go:93`). FFmpeg uses the last one. This builder runs with `-copyts`, so `t` is the source clock. For a Library Channel joined part-way through a file (`-ss SeekUS`, `linear_hls.go:88`), `t` starts at the seek position while `n_forced` starts at zero, so **every frame is forced to be a keyframe** until `n_forced × 4` catches up: joining 30 minutes in means about 450 consecutive keyframes (15–19 s of all-intra video squeezed under the bitrate cap — very blocky), and an hour in, twice that. A timestamp jump in a live broadcast has the same effect.
- **Evidence:** as cited. The duplicated `-c:v libx264 -preset … -pix_fmt` are harmless; the duplicated keyframe option is not.
- **Confidence:** Confirmed by reading for the argument order; Probable for the visible effect (standard FFmpeg behaviour). **Severity:** Medium. **Backlog:** no.

### B-03.8 The fixed 4 GiB per-session ceiling fights the owner's retention setting
- **What goes wrong:** Every 30 s a live session's folder is measured; over 4 GiB (or 4,096 files) its FFmpeg is cancelled (`server/internal/playback/hls_sweep.go:65`, `:191-208`). Nothing is deleted, so the next measurement cancels it again. Between cancellations the next segment request restarts it, so playback limps on with a forced restart every 30 s.
- **When:** whenever (retention + throttle buffer) × bitrate exceeds 4 GiB. The owner may set retention up to 3,600 s and the buffer up to 600 s (`server/internal/operations/settings.go:81-82`). Examples: an 8 Mb/s conversion with one-hour retention crosses 4 GiB after about 68 minutes of viewing; a 25 Mb/s Blu-ray remux being direct-streamed crosses it at about 21 minutes; with retention `0` — which the engine treats as "never prune" (`hls_timeline.go:132-134`) — an 80 Mb/s UHD remux crosses it in under 7 minutes. Even at defaults (180 s + 60 s) a UHD remux peaking above ~140 Mb/s would cross it.
- **Confidence:** Confirmed by reading for the mechanism; the user-visible effect (periodic stalls) is Probable. **Severity:** Medium. **Backlog:** no.
- **Likely fix:** when over the ceiling, prune the oldest played segments first and only cancel if that does not help; or derive the ceiling from the settings.

### B-03.9 MPEG-TS and other non-indexed sources can probably never be direct-streamed
- **What goes wrong:** The copy cut list requires that, after seeking to each grid line, the first video packet is flagged as a keyframe (`hls_copy_timeline.go:177-183`, `:100-102`). FFmpeg's MPEG-TS demuxer seeks by timestamp to an arbitrary packet, not to a keyframe, so most answers will not be keyframes and the whole timeline is refused; the session is then converted (A-03.4). AVI/WMV-style files without presentation timestamps fail the same way.
- **Consequence:** `.ts`/`.m2ts` files — including, it appears, Portico's own DVR recordings, which are written as MPEG-TS (`server/internal/decoder/linear.go:36`) — are fully transcoded whenever they are not direct-playable (for example in any browser), even when both streams are H.264/AAC and a remux would do. That is a large, avoidable CPU cost for the DVR feature.
- **How found:** Jellyfin limits on-demand keyframe extraction to `.mkv` and otherwise remuxes on an approximate timeline; asking "what does Portico do for the others?" led here.
- **Confidence:** Probable (demuxer behaviour from knowledge of FFmpeg; needs one run with a `.ts` recording). **Severity:** Medium. **Backlog:** no.
- **Likely fix:** for containers without a keyframe index, build the cut list from one sequential packet scan in the background (`ffprobe -show_entries packet=pts_time,flags -select_streams v`), as Jellyfin's task does.

### B-03.10 The `temporaryDirectory` and `maxBackgroundSessions` owner settings do nothing
- **What goes wrong:** `temporaryDirectory` is defined, validated, reported in the capacity report, and raises an attention item when not writable ("Every conversion will fail", `server/internal/telemetry/attention.go:175-181`). But the HLS engine is always constructed on `<state>/hls` (`server/cmd/server/main.go:664`) and no code outside settings, telemetry and the console route reads the value. `maxBackgroundSessions` is likewise only reported (`server/internal/telemetry/capacity.go:189`).
- **Consequence:** an owner who points conversions at a RAM disk or SSD is told it is healthy while files keep going to the state folder; an unwritable *unused* directory raises a false "every conversion will fail" alarm.
- **Confidence:** Confirmed by reading (searched every non-test Go file for both names). **Severity:** Medium (misleading setting; disk-wear and capacity surprises). **Backlog:** no.

### B-03.11 A failed conversion's diagnostic is deleted almost immediately
- **What goes wrong:** The failure code and FFmpeg's last words are stored on the session's `playback_artifacts` row (`hls_produce_support.go:109-116`). `CleanupGenerated` runs on every maintenance wake-up, selects sessions in a terminal state — including `failed` — removes their folder and **deletes that row** (`hls.go:990`, `:1045`). The owner's "recent failures" list reads only that table (`server/internal/playback/delivery_diagnostics.go:99-101`; its comment acknowledges "the rows are reclaimed with the session directory").
- **Consequence:** by the time an owner opens diagnostics, an outright failure has vanished. Only hardware-fallback notes on still-running sessions are visible.
- **Confidence:** Confirmed by reading. **Severity:** Medium (supportability). **Backlog:** no. Fix together with F-03.7.

### B-03.12 Converted video with copied audio: audio from before the seek point is included
- **What goes wrong:** With `-ss` before the input, FFmpeg trims *decoded* streams to the exact position but passes *copied* packets from wherever the demuxer landed — the previous video keyframe, up to a GOP earlier. Portico uses exactly this combination (`delivery_plan.go:623-629`, `hls.go:399-409`). The window's first segment then starts with up to several seconds of audio that belongs to earlier segments, and FFmpeg's HLS muxer measures its six-second cuts from that earlier audio timestamp. Combined with the absence of `-sc_threshold 0` (A-03.8), a scene-change keyframe can then split a segment early.
- **Consequence:** probably benign on hls.js (timestamps are absolute, so duplicate audio overwrites itself). On stricter players it may show as a brief audio repeat or a slightly misplaced seek. Jellyfin added a dedicated fix for this in its current code (`EncodingHelper.cs:1593-1622`), which is the reason to check.
- **Confidence:** Speculative — needs a runtime check on Apple and Cast players. **Severity:** Low to Medium. **Backlog:** no.

### B-03.13 Odd-sized sources fail to convert when no size limit applies
- **What goes wrong:** The scale filter (which also rounds to even dimensions) is added only when the plan has a width or height limit (`hardware.go:660`, `:708`). libx264 with `yuv420p` refuses odd widths or heights. The built-in baseline device profile has no maximum width (`server/internal/playback/client_profile.go:375`), and "Automatic" quality with no policy cap sets no height.
- **When:** an unknown/baseline client, automatic quality, and a source such as 853×480 or 1917×1080 that needs conversion. Known device profiles all declare a maximum width, so they are safe.
- **Confidence:** Confirmed by reading for the missing filter; Probable for the failure. **Severity:** Low. **Backlog:** no.

### B-03.14 Prepared versions are forced to 30 fps
- **What goes wrong:** The prepared-media recipe includes `-r 30` (`server/internal/decoder/prepared.go:37`). Film (23.976/24 fps) and PAL (25 fps) sources are converted by duplicating frames, which produces visible judder on pans; 50/60 fps sport is halved.
- **Confidence:** Confirmed by reading. **Severity:** Medium (every offline download and optimized version of a film is affected). **Backlog:** no.
- **Likely fix:** drop `-r 30`; cap at 60 with the same `fps` filter the on-demand path uses.

### B-03.15 Converted audio keeps a 96 kHz (or 88.2 kHz) sample rate
- **What goes wrong:** On-demand audio conversion passes codec, bitrate and channel count but no sample rate (`hardware.go:770`; the same in `hls_renditions.go:420`). FFmpeg's AAC encoder accepts 96 kHz, so a 96 kHz TrueHD/DTS-HD/FLAC/LPCM track becomes 96 kHz AAC at 192 kb/s. Many TV, Cast and set-top AAC decoders accept at most 48 kHz. The Live TV builder does set `-ar 48000` (`linear_hls.go:123`).
- **Confidence:** Confirmed by reading; device impact Probable. **Severity:** Low to Medium (rare sources, but silent audio when it hits). **Backlog:** no.

### B-03.16 Declared codec string does not describe the encoded stream
- **What goes wrong:** For conversions the master playlist always declares H.264 High profile at level 4.0 (or 5.1 above 1080p) (`hls_renditions.go:114-131`) while the encoder is given neither. 1080p60 from libx264 is level 4.2; NVENC's default profile differs. `BANDWIDTH` is the target average, not the peak, and there is no `AVERAGE-BANDWIDTH`. `RESOLUTION` ignores rotation and pixel aspect.
- **Confidence:** Confirmed by reading. **Severity:** Low. **Backlog:** no.

### B-03.17 Latent trap: the keyframe expression depends on `t` starting at zero
- **Design risk, not a current on-demand bug.** `gte(t,n_forced*6)` is only correct because on-demand windows do not use `-copyts`. Anyone who later adds `-copyts` to the on-demand path (for subtitle timing, say — Jellyfin does) will reproduce B-03.7 there. A comment at `hardware.go:731`, or switching everywhere to the `prev_forced_t` form, would remove the trap.
- **Confidence:** Confirmed by reading. **Severity:** Low (latent).

### B-03.18 Latent trap: folders with no database row are only removed under disk pressure
- **Design risk.** `CleanupGenerated` works from database rows (`hls.go:990`). A session folder created just before a crash, with no `playback_artifacts` row yet (`hls.go:301-305`), is only ever removed by the 24 GiB budget sweep (`hls_sweep.go:251`). Jellyfin simply empties its transcode folder at start-up.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-03.19 Performance risk: 4K live channels decoded on one thread
- **Design risk.** The live builder passes `-threads 1` before the input (`linear_hls.go:82`), limiting the decoder to one thread, and gives the encoder two. A 4K HEVC IPTV channel that needs conversion is unlikely to decode in real time on one core. Interactive Live TV is not "background work", so the usual "deprioritised, never paused" reasoning does not apply.
- **Confidence:** Speculative on the numbers. **Severity:** Low to Medium.

## Where Portico is stronger

- **Confinement.** A real sandbox, descriptor-only input, no network, private output folder, file-size limit, no secrets in the environment. No compared project has anything like it.
- **Exact timelines for remux.** A copied stream's playlist states each segment's true length, and every restarted window is forced onto the same cut list. Jellyfin still publishes approximate timelines for non-Matroska remuxes.
- **Hardware works without configuration** and is tested with a real encode through the real sandbox; a failure falls back to software invisibly instead of failing playback.
- **Alternate audio as HLS renditions** with on-demand production — instant track switching, and no encoder spent on tracks nobody selected.
- **Throttling, retention and abandoned-conversion reclaim are on by default**, with disk budgets and a free-space floor. Jellyfin's equivalents are off by default and it has no budgets.
- **Bad-network hardening:** restart budget, shared segment waits, typed retryable errors.
- **Process lifecycle:** process-group kill, parent-death signal, orphan ledger, shutdown drain.
- **Immutable, explained plans.** Every session stores what was decided and why; FFmpeg arguments are a pure function of the plan. Jellyfin rebuilds its command from request parameters on every restart.
- **Honest Dolby Vision handling:** the viewer is warned before a Profile 5 conversion with approximate colours.
- **Tone mapping on by default**, and HDR detected only from the transfer function.
- **One live conversion shared by all viewers of a channel.**
- **A pinned, verified, licence-clean FFmpeg** with a written feature contract.
- **Downmix that cannot clip** (normalise → make-up gain → limiter).

## Doc/code disagreements

1. **`04 Playback and Media.md` says "A failed hardware encode falls back to software, never to lower quality."** True for the encode. But a hardware failure on an HDR title falls back to a software plan that may itself fail in the sandbox (B-03.4), and repeated fallbacks disable hardware for everyone (B-03.6).
2. **Same doc: "Seeks restart production at the nearest keyframe."** For conversions, production restarts on the grid line (keyframes are created there); for copies, at the cut-list keyframe at or before the grid line. "Nearest" is loose but not wrong.
3. **Same doc: "Hardware acceleration is detected and reported to the owner."** Accurate, but it does not say that GPU decode/filtering is separate from GPU encode and, on the bundled FFmpeg, likely never active on Linux (B-03.2, B-03.3).
4. **Owner setting `temporaryDirectory`** is documented in its own description as "Absolute path for conversion working files"; the engine ignores it (B-03.10).
5. **Owner setting `maxBackgroundSessions`** ("Background conversions … allowed at once") is not enforced anywhere I could find.
6. **Two different defaults for played-segment retention:** 180 s in the settings registry (`operations/settings.go:82`) and 300 s in the engine (`delivery_settings.go:48`). The registry wins in a running server; the engine constant is what tests and an unwired server use.
7. **Retention `0`:** the setting allows 0 and reads as "keep nothing"; the engine treats 0 as "never delete" (`hls_timeline.go:132-134`).
8. **Throttle description says a conversion "pauses";** it is stopped and restarted (A-03.1). Harmless wording, but an owner watching processes will see FFmpeg come and go.
9. **`hdrToneMappingAlgorithm` is described as "Tone curve used when HDR tone mapping runs";** it is used only by the CPU chain and `tonemap_cuda` (A-03.14).
10. **HDR definition differs between on-demand playback and prepared media** (transfer only versus transfer or BT.2020 primaries; A-03.16).
11. **`mediaexec.DefaultOutputBytes` comment:** "HLS sessions are budgeted at 4 GiB in total, so one file never needs more" — the same 4 GiB is used as both the per-file limit and the per-session ceiling; see B-03.8 for why the session ceiling is questionable.
12. **FFmpeg contract requires `libx265` and `loudnorm`** (`requirements.v1.json`), neither of which the server's delivery path uses. Not wrong, but it constrains builds for unused features.

## Open questions for Justin

1. **Output codec.** Is H.264-only conversion a decision for release, or should the plan/argument builder be shaped now so HEVC (and HDR-preserving HEVC) can be added without re-architecture? (A-03.6, F-03.1, F-03.2)
2. **FFmpeg strategy.** Stay on unpatched upstream and accept the gaps (no CUDA/VideoToolbox tone mapping, slow software tone mapping, host-supplied drivers), carry a small patch set, or officially support jellyfin-ffmpeg as an alternative binary? (A-03.12)
3. **When a file cannot be keyframe-indexed quickly,** do you prefer today's silent conversion, an approximate remux like Jellyfin's, or a background index so the question rarely arises? (A-03.4, B-03.9)
4. **Bitrate policy.** Should a conversion with no ceiling target the ladder bitrate (today) or be capped near the source's own bitrate? Should software encodes use a quality target (CRF) with a cap? (A-03.7)
5. **The "finite" engine** (`internal/playback/vod`): future direction or removable? (A-03.43)
6. **Live TV:** should it share the on-demand hardware and audio-copy rules, and is stereo-only AAC for broadcast 5.1 intended? (A-03.27)
7. **Disk ceilings:** should the 4 GiB per-session and 24 GiB server budgets be owner settings, or derived from the retention and buffer settings? (B-03.8)
8. **Adaptive bitrate:** is a viewer-chosen "adapt to my connection" rung acceptable under the "quality changes only when a person asks" principle, or is it ruled out entirely? (A-03.25, F-03.17)
9. **Diagnostics retention:** how many recent conversion records should the server keep, and may they include the (path-redacted) FFmpeg command line? (F-03.7, B-03.11)
10. **Tone-mapping control:** one curve applied on every path, or remove the setting and choose per path? (A-03.14)
11. **Prepared versions for HDR sources:** tone map to SDR, or wait for HEVC HDR output? (F-03.11)

## Evidence boundary

This review read source only. It could not establish:

- **Any FFmpeg run-time behaviour.** B-03.1, B-03.3, B-03.4, B-03.7, B-03.9 and B-03.12 rest on how FFmpeg 8.1 and specific encoders/demuxers behave. Each names the one run that would settle it. I did not execute the bundled FFmpeg or any binary in the checkout.
- **Whether the bundled static FFmpeg finds GPU drivers on common Linux distributions** (VA-API driver search path, libvpl runtime for Quick Sync, NVIDIA libraries) with and without the sandbox, and inside Docker.
- **Whether group-owned `/dev/dri` nodes remain accessible inside the bubblewrap user namespace** on distributions that restrict render nodes to a `render` group.
- **Player behaviour:** AVPlayer, Cast, ExoPlayer and hls.js handling of fMP4 video with MPEG-TS audio renditions, of segment lengths that differ from the playlist, of a declared codec level that differs from the stream, and of early copied audio.
- **Performance:** restart latency after a throttle kill, software tone-mapping speed, single-thread live decode, copy-timeline probe time on slow or remote storage. No targets exist yet (per the brief).
- **Windows.** No sandbox there; the job-object child handling is marked unverified in the code itself (`server/internal/childprocess/childprocess_windows.go`).
- **Timestamp continuity across restarted windows** for copied MPEG-TS and fMP4 output (B-frame delay, `avoid_negative_ts` shifts). The design is careful and has tests I did not run; I found no defect by reading but cannot vouch for it.
- **Jellyfin's `EncodingHelper.cs` was read selectively.** Its per-vendor filter chains contain more special cases than are listed here; the ones that prompted a Portico question are recorded.
- **jellyfin-ffmpeg's patch list** was taken from general knowledge and one public search, not from reading the patch repository.

## Outside my domain

- **Decision side of Live TV** (convert whenever not H.264 8-bit progressive; AC-3 → AAC stereo): `server/internal/playbackruntime/linear_plan.go:181`, `:228`. For the Live TV / decision auditors.
- **DVR recordings are MPEG-TS**, which interacts badly with B-03.9. For the DVR auditor.
- **Trickplay generation cost and HDR thumbnails** (A-03.41). For the trickplay auditor.
- **Subtitle burn-in graph** (`server/internal/decoder/burn_in.go`) looked sound on a read-through; burn-in disables all GPU filtering for the session, which auditor 04 may want to weigh against Jellyfin's GPU overlay filters.
