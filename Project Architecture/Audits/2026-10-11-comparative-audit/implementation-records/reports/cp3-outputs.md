# cp3-outputs — media outputs

Lane worktrees: `server-cp3-outputs` and `native-cp3-outputs`, branch `audit-implementation/cp3-outputs`. Both have `audit-implementation/main` merged in (server `13c406d9`, native `7adc5f3`). Contracts were regenerated after the merge, the shared packages were mirrored again and the lock was committed. Nothing was pushed. No subagents or helpers were used. GOCACHE was deleted at the end.

## 1 Outcome

- **A19, device capability profiles with operating points.** Each video codec in the capability document can now list `limits`. Every entry is one decoder operating point: the largest picture at a frame rate, and optionally only for some bit depths. That lets a device say "4K at 30 but 1080p at 60", or that 10-bit H.264 is limited to 1080p30 while 8-bit runs on hardware at 4K.
  - The server applies these points when it decides whether a file can be copied. A conversion takes the point that keeps the source frame rate, then the largest picture. A 4K60 source on a device limited to 4K30 or 1080p60 therefore becomes 1080p60. Points also apply to Live TV.
  - Android reports real per-size rates from `MediaCodec` (`getSupportedFrameRatesFor`) for both the Media3 engine and mpv.
  - The mpv profiles on native gave 8-bit and 10-bit one shared limit. That limit is now split: the hardware point covers 8-bit and a software point covers 10-bit.
  - Web and Vega ask the browser or device about 4K60 and 1080p60 through the shared probe.
- **A18, HDR mastering metadata.** The scanner now reads the SMPTE ST 2086 mastering display and MaxCLL/MaxFALL from stream side data (Matroska colour elements, MP4 `mdcv`/`clli`). Analysis reads them from the first frame for HEVC elementary or MPEG-TS sources. They are stored in the stream detail; the detail version is bumped, so the first play of each title refreshes it.
  - A conversion that keeps HDR10 carries the values to libx265 (`master-display`, `max-cll`) and to SVT-AV1 (`mastering-display`, `content-light`). The new HEVC prepared recipes carry them too.
- **A16, rich optimized downloads.** There are four new prepared recipes, `hevc-720/1080/1440/2160-v1`. They keep HDR10 or HLG at 10 bits with the mastering description, keep any 10-bit source at 10 bits, keep surround sound as 5.1 AAC (mono and stereo stay as they are), tag the picture `hvc1`, and remove the Dolby Vision RPU and EL units (base layer only).
  - The download menu picks the HEVC recipe for a quality step when two things are true. It must keep something the H.264 recipe would lose: HDR, 10-bit, surround, or an HEVC or AV1 original. And the asking device's own planner must say it plays the result.
  - The choice is stored on the download request, so the worker builds what the device was offered.
  - The H.264 recipes no longer refuse HDR sources. They tone-map them on the CPU, or refuse when `zscale`/`tonemap` are missing. Output validation was updated for both families.
- **A17, Live TV beyond MPEG-TS.** Live inputs can now be published as fragmented MP4: an init segment per producer, `.m4s` fragments dated from their `tfdt`/`trun` clock, and `EXT-X-MAP` in the playlist (version 7). The live planner chooses fMP4 when it keeps the signal: FLAC or Opus, or HEVC for a player that takes it only in MP4. One buffer keeps one packaging. Library channels stay on MPEG-TS.
  - Channel sessions now store the device's mpv profile when that mpv plays channels, together with the viewer's Player setting. The producer then plans for the engine that keeps the most: Automatic uses native unless native would convert the picture or make lossless audio lossy and mpv would not. The chosen engine is pinned per buffer and reported as `presentation.engine`.
  - Channel presentations now carry clock epochs (`linear.clocks`). The channel playlist accepts `?at=<ms>` to start at a position.
  - On native, mpv can now play channels in both the Kotlin and Swift controllers. It maps media time to channel time through the clock epochs, seeks within what it has read, and when it cannot seek further it reopens the playlist at the target. The JS coordinator and the engine choice are wired. **This is switched off** (`LIVE_CHANNELS` / `liveChannels = false`) until it has been played on a device, so today channels still play only in the native engine and the server never chooses mpv for them.

## 2 Decisions I made

- **Operating-point choice for conversions:** keep motion first, then size. A juddering 4K30 is worse than 1080p60. When no point keeps the rate, the point that keeps the most pixels per second wins.
- **HEVC download recipe only when it gains something.** It is used only if the source has something the H.264 copy would lose (HDR, 10-bit, surround, or an HEVC/AV1 original) and the device plays HEVC. A plain SDR H.264 stereo source still gets H.264: it is quicker to make, plays everywhere, and HEVC would gain only size. The download menu is still one option per quality step; the server picks the recipe ("server decides, clients render").
- **Surround in downloads is AAC 5.1 (384 kbps).** Mono and stereo are kept as they are, and wider layouts become 5.1. Every first-party player decodes AAC 5.1. No E-AC-3 encode (upstream 5.1 only) and no FLAC in downloads (size).
- **HEVC download preset is `fast` at CRF 22, capped at 70% of the H.264 rate.** x265 `medium` at 4K on two threads is impractically slow for background work.
- **Tone mapping for H.264 copies of HDR sources** uses the CPU `hable` curve. It does not read the owner's tone-curve setting: the prepared worker has no access to playback settings, and the default matches.
- **Dolby Vision profile 5 is refused for every prepared recipe,** because there is no base layer to show. Its quality steps can still appear in the download menu and the job fails. This is pre-existing behaviour, kept.
- **Library channels stay MPEG-TS.** Their timeline is built on the source's own TS clock, and fMP4 `tfdt` from the muxer is not guaranteed to keep that absolute clock.
- **One buffer, one packaging and one engine.** A playlist cannot switch between TS and fMP4, and one player plays one playlist. A later producer of the same buffer is planned within that packaging and engine.
- **mpv live is gated off.** It cannot be checked here, and if it failed, channels that play today (after conversion) would be stranded on a broken engine. To test it, flip `PorticoMpvController.LIVE_CHANNELS` (Kotlin) and `PorticoMpvPlaybackModule.liveChannels` (Swift). The capability document, features, server choice and coordinator then all switch on together.
- **mpv route resume on a channel** accepts the position within 8 s (one segment) instead of 0.5 s, because a reopened live playlist lands on a segment boundary.

## 3 Items closed

- **A19:** closed on server, client-core, Android (Media3 and mpv), mpv profiles, web and Vega (shared probe). Apple declared tables are unchanged: the Apple TV 4K plays 4K60, and the Apple TV HD's H.264 1080p limit already holds. Tests: `video_limits_test.go` (3), client-core `client-profile.test.ts` (new test), native `mpv-profile*.test.ts` (updated).
- **A18:** closed on server. Tests: `assets/mastering_test.go`, `decoder/conversion_outputs_hdr_meta_test.go`, `playback/mastering_target_test.go`.
- **A16:** closed on server. Clients needed no change: menus render server options, the profile list grows from 7 to 11 (the parser allows 16), and offline play goes through the existing planner check. Tests: `decoder/prepared_rich_test.go`, `downloads/rich_recipe_test.go`, existing download and prepared suites (updated for the new signatures and recipe family).
- **A17:** fMP4 live and the two-engine choice are closed on the server. mpv channel playback is written and gated off (see section 9). Tests: `linearbuffer` `TestFFmpegFragmentedLiveOutput` (real ffmpeg: FLAC plus H.264 into fMP4 HLS through the publisher), `TestClocksMapMediaTimeAndPlaylistsStartWhereAsked`, `playbackruntime` `TestLinearPlanUsesFragmentedMP4WhereItKeepsTheSignal` and `TestLinearEngineChoiceFollowsThePlayerSetting`.
- **Earlier handoffs now closed:**
  - planner-wiring "Live publisher: linearbuffer rejects EXT-X-MAP"
  - "Live presentation: probe-time Automatic/mpv switching" (server side)
  - "Prepared/download owner: rich recipes"
  - "Source facts: mastering-display/MaxCLL"
  - android-mpv §7 item 4
  - apple-mpv item 6

## 4 API changes

- **Capability document** (`PUT /v1/me/devices/current/capabilities`, and the stored and parsed types):
  - `engines.*.video[].limits?: [{bitDepths?, maxWidth, maxHeight, maxFrameRate, maxLevel?}]`, at most 16 entries. Each entry must bound something.
  - `engines.*.liveChannels?: bool`. The native engine is always treated as true.
- **Channel session presentation** (`presentation` of a channel v1 session):
  - `engine` is now always set (`native` or `mpv`).
  - New `linear.clocks?: [{startMs, endMs, offsetMs}]`.
- **Channel state projection** (`LinearMedia`): new `engine?` and `clocks?: [{startUs, endUs, offsetUs}]`.
- **Live media route** `GET /v1/media/linear/{id}/{gen}/{token}/{name}`:
  - `index.m3u8?at=<ms>` (1–13 digits) starts the playlist at the segment holding that position. Any other query is still refused.
  - New object names: `s<N>.m4s` (`video/iso.segment`) and `i<producer>.mp4` (`video/mp4`) next to `s<N>.ts`.
- Generated contracts were refreshed with apigen and mirrored to native.

## 5 Schema changes

All are edits to the baseline, followed by schema-sync.

- `download_preparations.recipe TEXT NOT NULL DEFAULT ''`: the prepared recipe chosen for the device.
- `playback_channel_sessions.mpv_profile_json TEXT NOT NULL DEFAULT ''` and `engine_setting TEXT NOT NULL DEFAULT 'automatic'` (checked).
- Stream detail JSON (no column change) gains `masterDisplay` and `maxCll`. `StreamDetailVersion` goes from 1 to 2.

## 6 Settings added

None. The new behaviour follows the existing Player setting (Automatic/Native/mpv), quality choices and owner transcoding switch.

## 7 Tests run

- `go build ./...`, and `go vet` on assets, decoder, downloads, preparedmedia, playback, playbackv1, playbackruntime and linearbuffer: pass, before and after the merge.
- `go test` focused runs, all pass after the merge:
  - linearbuffer (whole package, including real-ffmpeg fMP4)
  - playbackruntime (whole package)
  - assets (whole package)
  - downloads (whole package)
  - preparedmedia (whole package)
  - decoder `-run 'Prepared|HDR10Metadata'`
  - playback `-run 'OperatingPoints|ConversionKeeps|KeptHDR10|Capability|Linear|Planner|Declared'`
  - httpapi `-run 'Channel|OpenAPI|EveryRouteDeclares|Download'`
- `go run ./cmd/apigen` and `scripts/schema-sync.py` (persistence test): pass.
- client-core: `npx tsc --noEmit` passes, and `client-profile.test.ts` passes 14 of 14.
- native: `npx tsc --noEmit` passes. `mpv-profile`, `mpv-profile-android`, `capability-profile` and `engine-choice` pass 25 of 25. `npm run lint:hooks` and `lint:strings` are clean (only the existing pending warnings).
- Vega: `npx tsc --noEmit` passes.
- Web: `npm run lint:hooks` and `npx tsc --noEmit` pass.
- Kotlin and Swift were **not compiled** (no Gradle or Xcode here).

## 8 Device test notes

1. **Android operating points.** On a phone whose H.264 decoder does 4K30 only, read the stored capability document. `engines.native.video[h264].limits` should list 4K@30 and 1080p@60 (and the same under `engines.mpv` for hardware codecs).
   - Play a 1080p60 H.264 file: it should direct-play.
   - Play a 4K60 file: it should convert to 1080p60, not 4K30.
   - On a device with only an 8-bit HEVC chip, 10-bit HEVC must stay within the software point (1080p30 on a standard device).
2. **Browser operating points.** In Chrome on a weak GPU, the probe should state a limit only when 4K60 is not smooth.
3. **HDR10 mastering.** Use an HDR10 MKV that has mastering metadata.
   - Force a conversion (a quality cap with an HEVC Main10 client). `ffprobe -show_frames` on a fragment should show "Mastering display metadata" and "Content light level" matching the source.
   - Repeat with NVENC, QSV and VAAPI. Hardware encoders rely on FFmpeg passing frame side data through; the explicit values only reach libx265 and SVT-AV1.
4. **HEVC downloads.**
   - Apple TV 4K or iPhone, with an HDR10 7.1 TrueHD film: the 1080p option should prepare as `hevc-1080-v1`. Check that it is HEVC Main10 `hvc1`, PQ/BT.2020 with mastering SEI, and AAC 5.1. It should play offline in HDR.
   - Browser or Android phone without HEVC: the same film gets `portable-1080-v3`. Check that it is a tone-mapped picture (not grey or washed out) with stereo AAC.
   - A Dolby Vision 8.1 source should give an HDR10 HEVC copy with no DV signalling in `ffprobe`.
5. **fMP4 live.** Use a DVB-T2 HEVC channel, or an IPTV source with FLAC or Opus audio, on Apple TV or Android TV.
   - The playlist (`index.m3u8`) should show `EXT-X-MAP:URI="i1.mp4"` and `.m4s` fragments.
   - Playback should start, timeshift seeking should work, and program dates should be correct.
   - A programme boundary (new producer) should add a new `EXT-X-MAP` after `EXT-X-DISCONTINUITY`.
   - An H.264/AAC channel must stay MPEG-TS.
6. **mpv live** (after flipping the two switches).
   - On Android TV and Apple TV with Player set to mpv, open a channel. The timeline position should match what AVPlayer or Media3 shows (within about 0.2 s).
   - Pause or seek back 30 s within the cache: it should be instant.
   - Seek back 10 minutes: the playlist should reopen (`?at=` in the relay log) and land within 0.5 s of the target.
   - Go Live should work. A programme boundary should keep the position continuous.
   - With Automatic, an MPEG-2 or AC-3 interlaced channel on a device whose native player cannot take it should open in mpv (`presentation.engine == "mpv"`).
   - Fail case: anything stuck in "seeking" for more than 5 s.

## 9 Not finished

- **mpv live is switched off** until it has been played on a device. The code has not been compiled (Kotlin, Swift) or run. Expect tail-end bugs in the reopen-at-position path and in the media-time offset for fMP4 (decode time versus presentation time, about one frame).
- **Hardware encoders and mastering metadata:** explicit values are passed only to libx265 and SVT-AV1. NVENC, QSV, VAAPI, VideoToolbox and AMF depend on FFmpeg passing frame side data through, which has not been verified.
- **Library channels:** they cannot use fMP4 (TS only). Their engine is chosen and pinned like a live input's.
- **Download menu and Dolby Vision profile 5:** steps are still offered for a DV profile 5 source, and the preparation then fails, as before.
- **Documentation:** architecture and contract documents (capability document `limits`/`liveChannels`, live fMP4, clock epochs, rich recipes) are not updated in portico-internal. This is for the docs lane or the owner report.
- **Not run:** no cross-compile (Windows/darwin), no full suites, no drivers.

## Commits

**Server (`server-cp3-outputs`):**
- `62b9df54` operating points
- `41cc4c32` mastering metadata
- `3623a74b` rich downloads
- `5d6ac87d` fMP4 live
- `43c43c1a` live engine choice
- `dacb97b8` clock epochs and playlist start
- `d788b5aa` merge main
- one follow-up commit: library-channel producers pin their engine

**Native (`native-cp3-outputs`):**
- `f78ad7f` operating points (Android and mpv)
- `e59dd68` mpv channel playback (gated)
- `1af0a5a` engine contract
- `2d468a2` merge main
