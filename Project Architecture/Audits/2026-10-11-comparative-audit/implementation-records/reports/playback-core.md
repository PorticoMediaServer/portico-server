# Lane report: playback-core

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/server-playback-core`, branch `audit-implementation/playback-core` (nine commits on top of `0e03d99`, head `feca5c4`; nothing uncommitted). Server, the cast receiver inside the server repository, `packages/client-core`, `packages/i18n`, generated `packages/contracts` and `server/api` documents were changed. No screen was changed; in `web/` only one bridge file and one test were touched so the web app still type-checks and its unit tests pass.

## 1. Outcome

- **One capability document.** A device publishes one document: the device itself (display, audio output, the viewer's player setting and three overrides) and one profile per player engine (`native`, and `mpv` where the app carries it). It is stored exactly as published, per device, and it is what planning reads. The second document, the translation between the two and their routes are gone.
- **The plan names the engine.** Every presentation and every plan preview says `engine: "native" | "mpv"`. On Automatic the native player keeps every title it can play as it is or with a plain repackaging; mpv is chosen only when the native player would need a converted picture, a lossy audio conversion or burned-in subtitles and mpv can play the original file; otherwise the title is converted for the native player. Native and mpv settings are honoured (the other engine is used only when the chosen one cannot play the title at all, and the trace says so). A re-plan caused by a subtitle burn-in or the conversion fallback stays in the engine that is playing.
- **Planner fixes.** Passthrough works (it is a list beside the decoders and is used while the output route carries bitstreams). A device that tone-maps HDR itself says so and is given HDR on an SDR screen. Per-container limits, interlaced support and "can switch audio tracks inside a file" reach the planner because nothing is translated any more. Codec synonyms fold to one name. HEVC and HDR are repackaged into fragmented MP4 first. An `hev1` HEVC file is repackaged for an engine that only takes `hvc1`. Losslessly coded surround goes to Dolby Digital Plus or Dolby Digital at full rate where the device takes them, before AAC. A device with fragmented-MP4 HLS and no MPEG-TS can now be converted for. Repackaging still works with conversion turned off. A file of unknown bitrate no longer passes every ceiling.
- **The planner chooses conversion targets from a table.** What a conversion produces (picture codec, bit depth, dynamic range kept or tone-mapped; audio codec, channels, lossless or not) is chosen from `playback.ConversionOutputs` against the capability document. Today's table is what the converter can do today (H.264 8-bit SDR; AAC 2.0/5.1; AC-3/E-AC-3 5.1), so behaviour is unchanged until the converter lane registers more with `playback.SetConversionOutputs`. Tests prove HEVC 10-bit HDR-preserving, tone-mapped HEVC and lossless FLAC are picked correctly when the table offers them.
- **Quality.** The default is the original. Fixed qualities are the ladder steps and are ceilings, not forced conversions. Automatic starts at the original and steps down or up when the client asks; each step re-plans the same session at the player's position. A viewer has two saved defaults, `quality.home` and `quality.remote`; a start that names no quality gets the one for where it comes from. Owner ceilings, member caps and the upload budget always apply. A client may send its throughput estimate: Automatic then starts on the step that fits, and any other quality the connection plainly cannot carry is refused with the quality that would fit.
- **One home/remote rule.** Whether a request is "remote" is decided once, from the owner's home networks setting, for quality defaults, owner ceilings and member caps alike. The private-address guess and the per-network lanes (local/wifi/cellular/unknown) are gone.
- **One re-plan operation that keeps the queue.** `PATCH /v1/playback/sessions/{id}` changes quality, version, audio track or subtitle track, or reports a player failure; when the bytes change the same session gets a new generation at the current position. The session keeps its id, its queue and its place in it. A reported player failure is remembered per file, per device and per engine, and the session is re-planned the next way up in the same call.
- **Legacy playback routes removed.** Starting without an `Idempotency-Key`, the old session read/stop, `/progress`, `client-profile` and `route-failures` no longer exist. The cast receiver uses the new routes.
- **Remembered track choices.** The audio language and the subtitle choice (a language, or off) a profile last picked for a show (all its episodes) or a film are applied at the next start that names none, and shown as the preferred tracks in playback options.
- **Capability overrides.** Force stereo, disable passthrough and disable HDR travel in the capability document and narrow what the device is planned as; the decision trace notes each.
- **Shared client code.** `packages/client-core` builds, validates and publishes the new document (declared tables, browser/Apple/Android merges, an mpv engine built beside the native one), parses the new session and options fields, and has the in-place re-plan plumbing (`V1QueuePlayer.changePlayback`, `PlaybackSessionController.stepQuality/playerFailed`, `V1PlaybackApi.change/reportPlayerFailure`, `AutomaticQuality`). `PlaybackService.engineFailed` now recovers any server session in place, including queue successors.

## 2. Decisions I made

1. **Wire format = the planner's own structure, version 2, on the device route.** `PUT/GET /v1/me/devices/current/capabilities` survives (typed registry route, keyed by device, survives token rotation); `PUT/GET /v1/playback/client-profile` is deleted. The server refuses any other `version`.
2. **Passthrough is a separate list, not a flag on a decoder.** `audio[]` is what the engine decodes; `audioPassthrough[]` is what it can hand on undecoded. A codec can be in both. Passthrough is used only while `audioOutput.route` is `hdmi`, `arc`, `earc`, `optical` or `spdif`. An unknown or empty route does not pass through (the safe direction).
3. **HDR is three facts.** A decoder's `dynamicRanges` is what it decodes; `display.dynamicRanges` is what the screen shows; the engine's `toneMapsHdr` bridges the two. An empty display list means "not stated" and rejects nothing.
4. **Dolby Vision** counts as native only when the engine has the profile and can present it; otherwise the base layer is judged, as before.
5. **A fixed quality is a ceiling.** A 720p file at "720p" plays as it is. The old "rung forces a conversion" behaviour is gone.
6. **Automatic never moves by itself on the server.** The client measures and asks (`step: "down" | "up"`); the server only chooses the step and re-plans. With a throughput estimate at start it lands on the widest step that needs no more than 80 % of the estimate.
7. **Throughput refusal threshold:** a non-automatic start or quality change is refused (`connection_too_slow`) only when the estimate is below 80 % of what the plan needs. A client can retry without the estimate ("play anyway").
8. **Saved quality defaults are per device class** (`profile-device-class` scope, like the keys they replace) and are the only quality preferences: the 20 per-lane keys (`quality.<lane>.mode|maxVideoBitrateMbps|maxAudioBitrateKbps|maxVideoHeight|allowHDR`) are removed. "Allow HDR" became the `disableHdr` override in the capability document.
9. **The transport hint header (`X-Portico-Transport-Class`) decides nothing** any more; it is kept in the stored policy for diagnostics.
10. **Engine choice for music and remote (.strm) sources is always native** (music is rendered by the client's own audio engine).
11. **Route-failure memory:** a failure that names a stream fault (`decode_error`, `source_not_supported`, `audio_decode_error`, `manifest_incompatible`) closes that way of delivering the file for 7 days, as before; any other code (`engine_error`, `stalled_without_data`…) for 6 hours. Failures are per engine.
12. **`ended` far from the end is not "watched".** A timeline `ended` before 80 % of the duration ends the session but keeps the resume point instead of marking the title watched.
13. **Unknown bitrate under a ceiling** is taken as an ordinary file of its picture size (40/20/12/6/3 Mb/s for 2160/1440/1080/720/480p). This applies only to a file whose streams were analysed; a file not analysed yet is left as before (its picture is itself a guess and it may be a song).
14. **Lossless stereo still converts to 192 kb/s AAC** when it must be converted; only surround got a better target order. (A lossless target is used automatically once the converter offers one.)
15. **HEVC `codecTags` for Apple** (`hvc1`, `dvh1`) is declared in the shared Apple table, so `hev1` files are repackaged instead of failing once. The audit left the device behaviour unproven; a repackage is cheap either way.
16. **Version choice** when none is named: least done to the file (as it is, repackaged, audio converted, fully converted) under the quality asked for, then tallest, then highest bitrate; at most 8 versions are compared.
17. **Device-drawn styled/image subtitles** are chosen only when the engine lists the kind and the track is inside the original file the engine is playing. A flag (`SubtitleBesideStream`) exists for the subtitles lane to widen this once raw tracks can be delivered beside a stream.
18. **`GET /v1/items/{id}/playback-offers` stays.** It is the session-bound read for the subtitle plan, markers and prepared versions that the players use; it is not a start route. Its `qualities` rung list is superseded by `playback-options.qualities`.
19. **Internal Go entry points `playback.Service.Create/CreateContext/CreateReplacingContext`, `Get`, `Stop`, `Progress` remain** as the presentation-level seam fixtures across several lanes' tests use. They go through the same planning as a v1 start (a named quality becomes that quality's ceilings); no HTTP route reaches them.
20. **Every change to a session is planned from the quality the session asked for under the ceilings that hold at that moment.** A member cap that was lowered applies to the next change; one the owner lifted no longer applies to it. Before, a subtitle change through the session kept the old presentation's limits even after the cap was lifted; `TestSubtitleReplanRoutesHoldToTheMembersCurrentCeiling` was updated for its last step only (all "cap holds" steps are unchanged and pass). The separate subtitle route (`PUT /v1/items/{id}/playback/{session}/subtitles`) still keeps the stricter of the old and the current limits.
21. **A viewer's quality the server cannot produce is refused, not silently played as the original** (`422 unsupported_media`, or `transcode_not_allowed` where the owner's switch is what stops it). This is the behaviour the `limit` quality already had; it now also applies to a saved default such as "720p". Clients should offer the original when they get it. A softer rule (fall back to the original and say so in `presentation.quality`) is a product choice I left for the owner.
22. **Owner's conversion switch:** it is enforced on the plan (no converting route is admissible while it is off), no longer on "any HLS stream", so a copy-only repackaging plays with conversion off. Repackaging keeps its own owner switch.
23. I did not use the optional helper model; all work in this lane is mine.

## 3. Bugs

| ID | Result |
|---|---|
| B-02.1 | Fixed. No translation; a device's own per-transport lists are used, and HEVC/HDR prefer fragmented MP4. |
| B-02.2 = B-20.1 | Fixed. Passthrough list plus output route in the one document. |
| B-02.3 | Fixed. `embeddedAudioSwitching` is an engine field. |
| B-02.4 = B-20.2 | Fixed. `toneMapsHdr` is declared; `appleProfile` sets it with hardware HEVC. |
| B-02.5 | Fixed (per-container direct lists kept, interlaced kept, codec names folded, subtitle kinds are `subtitleRendering`). |
| B-02.6 = B-17.1 | Server side and shared-client plumbing fixed (one PATCH; `V1QueuePlayer.changePlayback`). The player screens still have to call it: see section 7. |
| B-02.7 | Fixed. Legacy start removed; no key is `428 idempotency_key_required`. |
| B-02.8 | Not fixed: deliberate (contract says a sustained re-watch clears watched). |
| B-02.9 | Fixed. One classifier (`requestIsRemote`, the owner's home networks). |
| B-02.10 | Fixed. No per-lane HDR switch; HDR is refused only by the owner or by the viewer's `disableHdr`. |
| B-02.11 | Fixed (best version for this device). |
| B-02.12 | Already fixed; kept, and simplified (every presentation stores its ceilings; one without is refused). |
| B-02.13 | Fixed: a timeline `error` with an engine-failure code is recorded for the file; the re-plan itself is `PATCH … failure`. |
| B-02.14 | Fixed in the planner (H.264 conversion into fragmented MP4 when the device has no MPEG-TS). Not run against a real producer: see section 8. |
| B-02.15 = B-20.9 | Fixed by declaration (`codecTags`); device behaviour unverified. |
| B-02.16 | Narrowed: uncertain failures lapse after 6 hours; failures are per engine. Still not visible or clearable by the owner. |
| B-02.17 | Already fixed. |
| B-02.18 | Fixed. |
| B-02.19 | Fixed. |
| B-02.20 | Fixed (80 % guard at the v1 timeline). |
| B-02.21 | Fixed (`positionMs` on PATCH; the shared client sends it automatically). |
| B-02.22 | Not changed (latent, off by default, belongs to the conversion pipeline). |
| N-V02.1 | Fixed in `packages/client-core` (`engineFailed` no longer depends on the last explicit play) and on the server (in-place re-plan). |
| B-20.8 | Already fixed; names now also fold on the server. |
| B-20.10 | Fixed in `androidProfile` (no stated channel limit is not stereo). |
| B-20.11 | Fixed in the declared table (Android declares `text` only until it has a real renderer). |
| B-20.14 | Fixed in `androidProfile` (software-only decoders are held to 1080p). Needs the native module to report `hardware`. |
| B-20.15 | Fixed (the `features` claims no longer exist). |
| B-20.20 | Half: the merge keeps decoders beside passthrough and carries the route; the Android native probe must report the active sink (section 7). |
| B-17.2 | Shared-client part fixed (in-place change and recovery); the web player still has to stop restarting the queue (section 7). |
| Other B-17.x, B-20.x | Not in this lane (player and device behaviour). |

## 4. API changes

All routes below are on the `apikit` registry unless marked. Generated artefacts are refreshed: `server/api/portico.openapi.json`, `packages/contracts/src/*`, `server/testdata/contracts/*`. Requests on registry routes refuse unknown fields (400 `invalid_request`).

### 4.1 Removed routes

| Route | Replacement |
|---|---|
| `POST /v1/playback/sessions` without `Idempotency-Key` (body `{itemId, quality, requestId}`) | The same path with `Idempotency-Key` (Playback Protocol v1). Without the header: `428 idempotency_key_required`. |
| `GET /v1/playback/sessions/{id}` and `DELETE …` for non-v1 ids | v1 only. |
| `POST /v1/playback/sessions/{id}/progress` | `POST /v1/playback/sessions/{id}/timeline`. |
| `PUT /v1/playback/client-profile`, `GET /v1/playback/client-profile` | `PUT/GET /v1/me/devices/current/capabilities`. |
| `POST /v1/playback/route-failures` | `PATCH /v1/playback/sessions/{id}` with `failure`. |

### 4.2 `PUT /v1/me/devices/current/capabilities`

Replaces the device's whole capability document. Body limit 64 KiB. Optional `If-Match: "<revision>"`. Answers `204` with `ETag: "<revision>"`; an unchanged document keeps its revision. `412 revision_mismatch` carries `error.current = {revision, capabilities}`. `400 invalid_request` names the first bad field in `error.fields[0].path` (for example `engines.native.video`, `engineSetting`, `version`).

Old body: `{form, network, containers[], streaming{hls{fmp4,ts},progressive}, video[], display{hdr[]}, audio[], audioDecode[], subtitles[{format,render}], features{}}`. New body (every field; "optional" means it may be omitted, zero and empty list mean "not stated"):

| Field | Type | Meaning |
|---|---|---|
| `version` | integer, required, must be `2` | |
| `client.family` | token, required | `web`, `apple`, `android`, `androidtv`, `firetv`, `vega`, `cast`… (`^[a-z0-9][a-z0-9_.+-]{0,31}$`) |
| `client.form` | optional token | `tv`, `phone`, `tablet`, `desktop`, `browser`, `cast`, `speaker` |
| `client.platform`, `platformVersion`, `os`, `osVersion`, `model`, `app`, `appVersion` | optional strings ≤ 64 | diagnostic |
| `engineSetting` | `automatic` (default when omitted) \| `native` \| `mpv` | the viewer's player setting on this device |
| `overrides.forceStereo`, `overrides.disablePassthrough`, `overrides.disableHdr` | booleans, default false | the viewer's overrides |
| `display.width`, `display.height` | integers | pixels |
| `display.dynamicRanges` | list of `sdr` `hdr10` `hdr10plus` `hlg` `dolby_vision` | what the screen shows now; empty = not stated |
| `display.maxFrameRate` | number | |
| `audioOutput.route` | token | `hdmi`, `arc`, `earc`, `optical`, `spdif` carry bitstreams; `speaker`, `headphones`, `bluetooth`, `airplay`, `usb`… do not |
| `audioOutput.maxChannels` | integer 0–64 | channels the output plays |
| `audioOutput.spatial` | boolean | |
| `maxBitrateBps` | integer | the device's own ceiling; 0 none |
| `engines.native` | object, required | the platform's own player |
| `engines.mpv` | object, optional | present only where the app carries mpv |
| `audioDecode[]` | optional, ≤ 32 | unchanged entries `{codec, containers[], maxSampleRate?, sampleRates?[], maxChannels?, maxBitDepth?, via?}` for the client's own music engine |

Each engine object:

| Field | Type | Meaning |
|---|---|---|
| `name`, `version` | optional strings | the engine's own name (`avplayer`, `exoplayer`, `hls.js`, `mpv`) |
| `evidence` | `declared` \| `probed` \| `mixed`, required | |
| `video[]` (≤ 32) | `{codec, profiles[], maxLevel, bitDepths[], maxWidth, maxHeight, maxFrameRate, maxBitrateBps, dynamicRanges[], dolbyVisionProfiles[], interlaced, codecTags[]?, evidence?}` | decoders. `dynamicRanges` is what the decoder decodes. `codecTags`, when present, are the only sample entries played from an original file. Levels use FFmpeg numbering (H.264 4.1 = 41, HEVC 5.1 = 153). |
| `audio[]` (≤ 48) | `{codec, maxChannels, maxSampleRate, objectAudio[] ("atmos","dtsx"), evidence?}` | codecs the engine decodes itself |
| `audioPassthrough[]` (≤ 24) | `{codec, maxChannels, objectAudio[], evidence?}` | codecs the engine can hand to the output undecoded |
| `transports[]` (≤ 16) | `{transport: "direct"\|"hls_ts"\|"hls_fmp4", containers[], video[], audio[]}` | what travels how; one entry per container group for `direct` |
| `subtitleRendering[]` | subset of `text`, `ass`, `pgs`, `vobsub`, `dvb` | kinds the engine draws itself |
| `toneMapsHdr` | boolean | the engine shows HDR it decodes acceptably on a non-HDR display |
| `embeddedAudioSwitching` | boolean | the engine can choose between audio tracks inside an original file |

Codec names are FFmpeg's (`h264`, `hevc`, `av1`, `vp9`, `mpeg2video`, `mpeg4`, `vc1`, `aac`, `mp3`, `ac3`, `eac3`, `dts`, `truehd`, `flac`, `alac`, `opus`, `vorbis`, `pcm`); the server folds `h265`→`hevc`, `avc`→`h264`, `mpeg2`→`mpeg2video`, `ec-3`→`eac3`, `ac-3`→`ac3`, `dca`→`dts`, `mlp`→`truehd`, `pcm_*`→`pcm`.

Example (an Apple TV with mpv; the full generated example for every platform is `server/internal/playback/testdata/client-profiles.json`, key `appletv_mpv`):

```json
{
  "version": 2,
  "client": {"family": "apple", "form": "tv", "platform": "tvos", "os": "tvos", "app": "portico-apple"},
  "engineSetting": "automatic",
  "overrides": {"forceStereo": false, "disablePassthrough": false, "disableHdr": false},
  "display": {"width": 3840, "height": 2160, "maxFrameRate": 60, "dynamicRanges": ["sdr", "hdr10", "hlg", "dolby_vision"]},
  "audioOutput": {"route": "hdmi", "maxChannels": 8, "spatial": true},
  "engines": {
    "native": {
      "name": "avplayer", "evidence": "mixed",
      "video": [
        {"codec": "h264", "bitDepths": [8], "maxWidth": 4096, "maxHeight": 2304, "maxFrameRate": 60, "dynamicRanges": ["sdr"], "evidence": "declared"},
        {"codec": "hevc", "bitDepths": [8, 10], "maxWidth": 4096, "maxHeight": 2304, "maxFrameRate": 60, "dynamicRanges": ["sdr", "hdr10", "hlg", "dolby_vision"], "dolbyVisionProfiles": [5, 8], "codecTags": ["hvc1", "dvh1"], "evidence": "probed"}
      ],
      "audio": [
        {"codec": "aac", "maxChannels": 8}, {"codec": "mp3", "maxChannels": 2}, {"codec": "ac3", "maxChannels": 6},
        {"codec": "eac3", "maxChannels": 8, "objectAudio": ["atmos"]}, {"codec": "alac", "maxChannels": 8}, {"codec": "flac", "maxChannels": 8}, {"codec": "pcm", "maxChannels": 8}
      ],
      "audioPassthrough": [],
      "transports": [
        {"transport": "direct", "containers": ["mp4", "m4v", "mov"], "video": ["h264", "hevc"], "audio": ["aac", "mp3", "ac3", "eac3", "alac", "flac"]},
        {"transport": "direct", "containers": ["mp3", "m4a", "m4b", "aac", "flac", "wav", "aiff"], "video": [], "audio": ["aac", "mp3", "alac", "flac", "pcm"]},
        {"transport": "hls_ts", "containers": ["mpegts"], "video": ["h264"], "audio": ["aac", "mp3", "ac3", "eac3"]},
        {"transport": "hls_fmp4", "containers": ["fmp4"], "video": ["h264", "hevc"], "audio": ["aac", "mp3", "ac3", "eac3", "alac", "flac"]}
      ],
      "subtitleRendering": ["text"],
      "toneMapsHdr": true,
      "embeddedAudioSwitching": true
    },
    "mpv": {
      "name": "mpv", "evidence": "declared",
      "video": [
        {"codec": "h264", "bitDepths": [8, 10], "maxWidth": 4096, "maxHeight": 2304, "maxFrameRate": 60, "dynamicRanges": ["sdr"], "interlaced": true},
        {"codec": "hevc", "bitDepths": [8, 10], "maxWidth": 4096, "maxHeight": 2304, "maxFrameRate": 60, "dynamicRanges": ["sdr", "hdr10", "hlg"], "interlaced": true},
        {"codec": "vp9", "bitDepths": [8], "maxWidth": 1920, "maxHeight": 1080, "maxFrameRate": 60, "dynamicRanges": ["sdr"], "interlaced": true},
        {"codec": "mpeg2video", "bitDepths": [8], "maxWidth": 1920, "maxHeight": 1080, "dynamicRanges": ["sdr"], "interlaced": true}
      ],
      "audio": [{"codec": "aac", "maxChannels": 8}, {"codec": "ac3", "maxChannels": 6}, {"codec": "eac3", "maxChannels": 8}, {"codec": "dts", "maxChannels": 8}, {"codec": "truehd", "maxChannels": 8}, {"codec": "flac", "maxChannels": 8}, {"codec": "opus", "maxChannels": 8}],
      "audioPassthrough": [],
      "transports": [
        {"transport": "direct", "containers": ["mkv", "mp4", "m4v", "mov", "webm", "avi", "ts", "m2ts"], "video": ["h264", "hevc", "vp9", "mpeg2video"], "audio": ["aac", "ac3", "eac3", "dts", "truehd", "flac", "opus"]},
        {"transport": "hls_ts", "containers": ["mpegts"], "video": ["h264", "hevc", "mpeg2video"], "audio": ["aac", "ac3", "eac3"]},
        {"transport": "hls_fmp4", "containers": ["fmp4"], "video": ["h264", "hevc", "vp9"], "audio": ["aac", "ac3", "eac3", "flac", "opus"]}
      ],
      "subtitleRendering": ["text", "ass", "pgs", "vobsub", "dvb"],
      "toneMapsHdr": true,
      "embeddedAudioSwitching": true
    }
  }
}
```

### 4.3 `GET /v1/me/devices/current/capabilities`

`200 {"revision": <integer>, "capabilities": <the stored document, normalised>}` with `ETag`; `404 not_found` when the device has published none. (Old shape: `{revision, capabilities}` with the old v1 body.)

### 4.4 `POST /v1/playback/sessions` (header `Idempotency-Key` required)

Request, changed or new fields only (all other fields unchanged: `itemId`, `channelId`, `channelGeneration`, `queue`, `transferId`, `versionId`, `partIndex`, `startPositionMs`, `startFrom`, `state`, `audio`, `subtitles`, `network`, `replacesSessionId`):

| Field | Old | New |
|---|---|---|
| `quality` | `{mode: "original"}` or `{mode: "limit", maxVideoBitrateKbps?, maxHeight?, maxAudioBitrateKbps?}`; default original | one of `{mode:"original"}`, `{mode:"fixed", id}`, `{mode:"automatic", id?}`, `{mode:"limit", maxVideoBitrateKbps?, maxHeight?, maxAudioBitrateKbps?}`. `id` is a ladder id: `2160p` `1440p` `1080p` `720p` `480p` `360p`. **Omitted: the viewer's saved default for this connection** (`quality.home` / `quality.remote`). `step` is refused on a start. |
| `throughputKbps` | — | optional integer 0–10,000,000: the client's estimate of its connection to this server |

New refusal: `422 connection_too_slow` (retry never); `error.reason` is the widest fixed quality id that fits (may be empty). Sent only when the request carried `throughputKbps`, the quality is not automatic, and the estimate is below 80 % of what the plan needs.

Response `presentation`, new fields (the rest unchanged):

| Field | Meaning |
|---|---|
| `engine` | `"native"` or `"mpv"`: the engine the device must play this presentation with |
| `quality` | the quality in force: `{mode, id?, maxVideoBitrateKbps?, maxHeight?, maxAudioBitrateKbps?}`; for automatic, `id` is the step it is on (absent on the original). Never contains `step`. |
| `audioTrackId` | the audio track being played, for example `"a1"` |
| `subtitle` | absent when subtitles are off; else `{trackId, kind: "text"\|"ass"\|"pgs"\|"vobsub"\|"dvb", delivery: "sidecar"\|"device"\|"burn"}`. `sidecar`: a WebVTT file in `subtitles[]`, drawn by the app. `device`: the engine draws the track itself from the file it is playing. `burn`: in the picture. |

Example (a start that was re-planned one step down in Automatic):

```json
{
  "id": "ps_Tjqh94zOfO7-zEATOfJo", "revision": "2", "kind": "vod", "role": "local", "state": "playing",
  "itemId": "N0AfeetlQ23lEjcxkLSl1w", "versionId": "teBUakQ-1zlO_JJsKRtyLkixUX3v4XsTdhqozSP-Q9c",
  "queue": {"queueId": "q_…", "entryId": "e_…"},
  "lease": {"expiresAt": "2026-10-13T04:49:32.497Z", "reportEveryMs": 10000},
  "presentation": {
    "generation": 2, "mode": "stream", "url": "/v1/media/<grant>/master.m3u8", "engine": "native",
    "startPositionMs": 61000, "subtitles": [],
    "decision": {"video": {"action": "transcode", "to": "h264", "reasons": ["bitrate_above_request"]}, "audio": {"action": "copy", "reasons": []}},
    "bitrateKbps": 8000, "quality": {"mode": "automatic", "id": "1080p"}, "audioTrackId": "a1", "container": "mpegts_hls"
  }
}
```

### 4.5 `PATCH /v1/playback/sessions/{id}` (header `If-Match` required): the one re-plan operation

Request, new or changed fields (unchanged: `state`, `versionId`, `partIndex`, `audio`, `subtitles`, `seek`, `subtitleOffsetMs`):

| Field | Meaning |
|---|---|
| `quality` | as in 4.4, plus for automatic `step: "down" \| "up"` (one ladder step from where the session is). `{mode:"automatic"}` with neither `id` nor `step` on a session already on automatic changes nothing. |
| `positionMs` | where the player is now; a change that starts a new presentation starts there (without it: the last reported position) |
| `throughputKbps` | the current estimate; checked (as in 4.4) against a `quality` sent in the same request; also stored for Now Playing |
| `failure` | `{code, detail?}`; `code` matches `^[a-z0-9_]{1,48}$` (`decode_error`, `source_not_supported`, `audio_decode_error`, `manifest_incompatible`, `stalled_without_data`, `engine_error`), `detail` ≤ 512 chars. The server records the failure for this file, device and engine and re-plans the same session the next way up at `positionMs`. `422 unsupported_media` when there is nothing left to try; the session is left as it is. |

Behaviour: a version, audio, quality or part change, a `failure`, and a subtitle change that involves a styled/image track or leaves a device-drawn or burned-in one, produce a new presentation: `presentation.generation` + 1, a new `url`, possibly another `engine`, `startPositionMs` at the position. The session id, `queue` and its place are unchanged. A text-subtitle switch or an offset change keeps the generation. Errors added: `connection_too_slow`. Response: the session, as in 4.4.

### 4.6 `POST /v1/playback/sessions/{id}/timeline`

Shape unchanged. Behaviour: `state: "error"` with `error.code` one of the engine-failure codes records that failure for the file (no re-plan; use PATCH `failure` for that). `state: "ended"` reported before 80 % of the title's duration ends the session without marking the title watched.

### 4.7 `GET /v1/items/{itemId}/playback-options`

Query: new `qualityId` (with `quality=fixed` or `quality=automatic`). `quality` omitted = the saved default for this connection. Response, new or changed fields:

| Field | Meaning |
|---|---|
| `qualities[]` | the quality menu for the preferred/previewed version: `{mode:"original"}`, `{mode:"automatic"}`, then `{mode:"fixed", id, maxHeight, maxVideoBitrateKbps, maxAudioBitrateKbps}` for every ladder step not taller than the picture. Music: only original. |
| `preferred.versionId` | now the version this device plays best (was: the first) |
| `preferred.audioId`, `preferred.subtitleId` | include this profile's remembered choice |
| `preferred.subtitlesOff` | true when the profile last turned subtitles off for this show or film |
| `preferred.reason` | `last_used`, `profile_language` or `default` (was always `default`) |
| `preferred.quality` | the saved default quality for this connection, `{mode, id?}` |
| `plan.engine` | `native` or `mpv` |
| `plan.quality` | the previewed quality (now includes `id`) |
| `plan.streams[].action` | adds `device` for a styled/image subtitle the engine would draw itself (besides `direct`, `copy`, `transcode`, `burn`, `sidecar`) |

### 4.8 `GET /v1/playback/delivery-policy` (not on the registry; unchanged path)

`policy.networkClass`: was `local|wifi|cellular|remote|unknown`, now `local|remote|unknown`. `policy.preferenceLane`: was `local|wifi|cellular|unknown`, now `home|remote`. `policy.qualityMode`: now the saved default quality for this class (`original`, `automatic` or a ladder id). `policy.maxVideoBitrateBps`: the owner's remote ceiling when remote, 0 at home. `policy.allowHDR`: false only when the owner disallows HDR. New top-level `savedQualities`: `["original","automatic","2160p","1440p","1080p","720p","480p","360p"]`. `ladder` unchanged.

### 4.9 Other reads that carry plan or policy objects

- `GET /v1/admin/sessions`: each item gains `engine` and `quality`.
- `GET /v1/items/{id}/playback-offers` (unchanged path, not on the registry): `current.qualityId` is now `original`, a ladder id, `limit`, `automatic` or `automatic:<id>` (was `auto`/`original`/a rung id); `deliveryPolicy` as in 4.8; `deliveryPlan` gains `engine`, `outputBitDepth`, `outputRange`, `audioLossless`, `audioPassthrough`; `deliveryPlan.trace.client` gains `engine` (native/mpv) and `engineName`; `trace.audio.chosenBy` may be `remembered`.
- Trace vocabulary added: rejections `video_sample_entry_unsupported`, `dynamic_range_not_shown_by_display`, `audio_passthrough_unavailable`; notes `engine_chosen_automatically`, `engine_setting_unavailable_other_engine_used`, `device_tone_maps_hdr`, `audio_passed_through_to_receiver`, `viewer_override_force_stereo`, `viewer_override_disable_passthrough`, `viewer_override_disable_hdr`, `source_bitrate_unobserved_assumed_from_picture_size`.
- `GET/PATCH /v1/preferences`: registry revision `p40.Q2`; keys `quality.home`, `quality.remote` added; the 20 `quality.<lane>.*` keys removed (a PATCH naming one is refused as an unknown key).

### 4.10 `packages/client-core` exports that changed (for client lanes)

- `client-profile.ts`: types `CapabilityDocument`, `EngineCapabilities`, `ClientAudioPassthrough`, `CapabilityOverrides`, `EngineId`, `EngineSetting`, `SubtitleRendering`, `StoredCapabilities`, `MpvFacts`, `PlayerFailureCode`, `ProbeIdentity`; functions `capabilityDocumentProblem`, `withViewerSettings`, `engineRendersSubtitle`, `mpvEngine`, `withMpvEngine`; the declared tables and `probeWebProfile` / `appleProfile` / `androidProfile` now return a `CapabilityDocument`. Removed: `ClientProfile`, `clientProfileProblem`, `ClientProfilePublisher`, `reportRouteFailure`, `RouteFailureCode`, `RouteFailureResult`, `clientProfileVersion`.
- `playback-v1`: `CapabilityPublisher(http, scope?)` now publishes a `CapabilityDocument` (`publish`, `read`, `reset`); removed `capabilityProfileFromClient`, `networkClassOf`, `CapabilityProfile`, `qualityDecision`, `qualityLane`, `qualityPreferences`, `automaticRung`, types `QualityLane`, `QualityMode`, `QualityPreferences`, `QualityDecision`. Added `AutomaticQuality`, `qualityRequest`, `qualityOptionRequest`, `qualityOptionSelected`, `savedQualities`, `qualityLadder` (new values), types `QualityId`, `SavedQuality`, `QualityOption`, `QualityInForce`, `PlaybackEngine`, `PresentationSubtitle`. `QualityRequest` has the four modes. `Presentation` gains `engine`, `quality`, `audioTrackId`, `subtitle`, `container`; `PlaybackOptions` gains `qualities`, richer `preferred`; `PlaybackPlan` gains `engine`, `quality`.
- `SessionsClient` / `PlaybackSessionController`: `StartOptions.throughputKbps`; `SessionChange.positionMs|throughputKbps|failure`; controller methods `setQuality(quality, throughputKbps?)`, `stepQuality('down'|'up')`, `playerFailed(code, detail?)`; every re-planning change carries the player's position.
- `V1PlaybackApi`: `change(session, change)`, `reportPlayerFailure(sessionId, code, detail, positionSeconds)`, `onSessionChanged(listener)`. `PlaybackApi.reportRouteFailure` is replaced by optional `reportPlayerFailure`. `HttpLocalApi` no longer has `createPlayback`, `stopPlayback`, `progressPlayback`, `reportRouteFailure` and no longer implements `PlaybackApi`.
- `V1QueuePlayer`: `currentSession()`, `changePlayback(change)`, `setQuality`, `stepQuality`, `setVersion`, `setAudioTrack`, `setSubtitleTrack`.
- `delivery.ts`: `NetworkClass` is `local|remote|unknown`; `ResolvedDeliveryPolicy.preferenceLane` is `home|remote`. `presentation/quality-model.ts`: `QualityNetwork` is `home|away|null`.
- `presentation/settings-structure.ts`: see section 6.

## 5. Schema changes (`0001_baseline.sql`, synced with `scripts/schema-sync.py`)

- Dropped table `playback_client_profiles` and its index `playback_client_profiles_age`.
- `playback_device_capabilities`: column `planner_profile` removed; `document` now holds the capability document, `CHECK(length(document)<=131072)`.
- `playback_route_failures`: new column `engine TEXT NOT NULL DEFAULT 'native' CHECK(engine IN('native','mpv'))`; primary key is now `(session_hash, asset_id, engine, route)`.
- New table `playback_track_memory(profile_id TEXT, item_id INTEGER → catalog_entities ON DELETE CASCADE, audio_language TEXT, subtitle_mode TEXT IN('','off','on'), subtitle_language TEXT, updated_ms INTEGER, PRIMARY KEY(profile_id,item_id)) WITHOUT ROWID`. `item_id` is the show for an episode, else the title itself; `profile_id` is the personal owner key used by `progress`.
- No change to `playback_delivery_plans`: the engine and the new output facts live in its existing `policy_json`.

## 6. Owner or viewer settings added or wired

| Name | Type | Default | Where it takes effect | Screen |
|---|---|---|---|---|
| `quality.home` (preference registry, device-class scope) | `original` \| `automatic` \| `2160p` \| `1440p` \| `1080p` \| `720p` \| `480p` \| `360p` | `original` | the quality of a start that names none, inside the owner's home networks | Row exists in `settings-structure.ts` (Quality page, "At home"); renders on web through the generic preference row; native lane must check |
| `quality.remote` | same | `original` | same, outside the home networks | same ("Away from home") |
| Player (`engineSetting` in the capability document) | `automatic` \| `native` \| `mpv` | `automatic` | engine choice in planning | Row `playerEngine` declared (storage `device`, shown only when `capabilities.mpvPlayer`); **no screen wiring yet** |
| Force stereo (`overrides.forceStereo`) | switch | off | planning: every title two channels, no passthrough | Row `forceStereo` declared; no wiring yet |
| Don't pass sound through (`overrides.disablePassthrough`) | switch | off | planning: passthrough list ignored | Row `disablePassthrough` declared (shown when `capabilities.audioPassthrough`); no wiring yet |
| Turn off HDR (`overrides.disableHdr`) | switch | off | planning: device treated as SDR only | Row `disableHdr` declared (hidden when `capabilities.hdr === false`); no wiring yet |

Removed viewer settings: the per-network quality rows (`quality.cellular` composite and the lane keys) and the registry-backed `hdr` composite (replaced by the override). Owner settings are unchanged; the owner's remote bitrate limit, audio ceiling, height ceiling, HDR switch, remux switch, conversion switch and planning policy all still apply (the conversion switch no longer blocks copy-only repackaging).

New strings (`packages/i18n/src/catalog/en-US/settings.ts`, `preferences.ts`): `settings.row.playerEngine`, `settings.help.playerEngine`, `settings.playerEngine.automatic|native|mpv`, `settings.section.playbackFixes`, `settings.section.playbackFixesHelp`, `settings.row.forceStereo`, `settings.help.forceStereo`, `settings.row.disablePassthrough`, `settings.help.disablePassthrough`, `settings.row.disableHdr`, `settings.help.disableHdr`, `pref.label.quality.home|remote`, `pref.help.quality.home|remote`; reworded `settings.row.qualityHome`, `settings.help.qualityHome`, `settings.help.qualityAway`, `settings.section.qualityLimits`, `pref.group.quality.description`. Removed: `settings.row.qualityCellular`, `settings.row.hdr`, `settings.help.hdr`, `settings.hdr.*`, `pref.qualityMode.*`, `pref.network.*`, `pref.label.allowHDR`. The server's fallback message for `connection_too_slow` is in the error catalogue; `V1QueuePlayer` has the same sentence hard-coded beside its existing ones.

## 7. Needed from others

**Every client (web, iPhone/iPad/Apple TV, Android/Android TV/Fire TV, Vega):**
1. Publish the capability document with `CapabilityPublisher` (`PUT /v1/me/devices/current/capabilities`): build it with the platform function (`probeWebProfile`, `appleProfile`, `androidProfile`, `vegaDeclaredProfile`), add mpv with `withMpvEngine(doc, mpvFacts)` where the app carries it, stamp the viewer's device settings with `withViewerSettings(doc, {engine, overrides})`, and republish when the display, the audio route or a setting changes. Delete any use of the two old publishers.
2. Play each presentation with `session.presentation.engine`. A re-plan can change the engine mid-title.
3. Quality menu: render `options.qualities`, mark `presentation.quality`, send `setQuality(qualityOptionRequest(option))`. Stop using the offers rungs for it. For Automatic, feed `AutomaticQuality.decide(presentation.quality, {throughputKbps, currentKbps, stalled, bufferedMs})` from the player and call `stepQuality`. Send `throughputKbps` at start when known; on `connection_too_slow` offer `error.reason` (a quality id) or Automatic.
4. Version, audio and subtitle menus: call `V1QueuePlayer.setVersion / setAudioTrack('a<N>') / setSubtitleTrack(id | null)` (or the controller equivalents). Never restart the queue for these.
5. Subtitles: follow `presentation.subtitle.delivery` (`sidecar` → load the WebVTT in `presentation.subtitles`; `device` → enable that track in the engine; `burn` → nothing to draw).
6. Player failure: keep calling `PlaybackService.engineFailed`; it now re-plans in place. A native listening owner or a custom controller calls `playerFailed`.
7. Settings: store `playerEngine`, `forceStereo`, `disablePassthrough`, `disableHdr` on the device and feed them to step 1; pass `mpvPlayer`, `audioPassthrough`, `hdr` in `SettingsCapabilities`.
8. Show preferred tracks from `options.preferred` (`reason: "last_used"`).

**Web (web-playback-2):** steps 1–8; the bridge (`web/src/bridge/client-profile.ts`) already publishes the probed document but does not yet stamp overrides. Add `ass`, `pgs`, `vobsub` to `subtitleRendering` only when the WebAssembly renderers really draw them. `Player.tsx` `chooseVersion` / quality rows / `selectAudioTrack` are the callers to move.

**Native (native-playback, apple-mpv, android-mpv):** steps 1–8, after `scripts/shared-packages.mjs --write`. The Android native module must report: `decoders[].hardware`; `audioDecoders[].maxChannels` (0 or absent = no limit); `passthrough` for the **active** sink only; `audio.route` as one of `hdmi`, `arc`, `earc`, `optical`, `speaker`, `headphones`, `bluetooth`, `usb`; `subtitleRendering` only for renderers that exist. The mpv builds report `MpvFacts` (version, missing decoders, libass, image subtitles, passthrough, Dolby Vision profiles, software ceiling). The Apple route name is folded by `appleProfile`.

**conversion-outputs / planner-wiring:** call `playback.SetConversionOutputs` at start-up with what the converter can really produce, in the server's order of preference; extend `DeliveryPlan.conversionGraph` (`delivery_plan.go`) to pass `OutputVideoCodec`, `OutputBitDepth`, `OutputRange`, 8-channel and lossless audio to `decoder.ConversionRequest`; extend `serverConversionRejections` for non-H.264 encoders. Until then the default table keeps today's outputs. Verify H.264 conversion into fragmented MP4 in the producer.

**subtitles-text:** when a styled or image track can be handed to the device beside a stream, set `choice.SubtitleBesideStream` in `playbackv1/sessions.go` (`present`) and list the file in `presentation.subtitles`. The kind names are `playback.SubtitleText|ASS|PGS|VobSub|DVB`; `playbackv1.subtitleKindOf(format)` maps formats.

**livetv / planner-wiring:** `playback.ClientProfile` is now one engine of the document (no `Passthrough` flag on audio entries; `AudioPassthrough` list; `SubtitleRendering`). `playbackruntime/linear_plan.go` still compiles and plans from the native engine; live copy rules can now use passthrough and the display facts.

**console:** nothing required. Optional: show `engine` and `quality` in Now Playing (`GET /v1/admin/sessions`).

**documents:** `server/api/guide/04-playback-v1.md` still cites the removed start and progress operations of `openapi.yaml` (its example checker, `server/api/guide/check-examples.mjs`, already failed on that guide's `/v2` mentions before this lane; it now also reports these). `server/api/README.md` still describes the old single-session model and a `/v2` control plane; `Spec — Playback Protocol v1` §3 (capability profile), §5.2 (quality, Automatic), §5.4 (last used), §5.5 (subtitle delivery), §6 (timeline error), §17 (removed routes); `Project Architecture/04 Playback and Media.md`; `server/api/preferences.md` if it lists the lane keys. Section 1 of the programme brief wins over all of them.

## 8. Test notes for other environments

What was run here (Linux, no devices): `go build ./...`; `go vet` on every package that imports the changed ones; `go test` for the whole of `internal/playback`, `internal/playbackv1`, `internal/playbackruntime`, `internal/operations`, `internal/apispec`, `internal/httpapi`, `internal/persistence`, `internal/backup`, `internal/social`, `internal/downloads`, `internal/preparedmedia`, `internal/producerinput`, `internal/eventfeed` (all pass except item 0 below); `go run ./cmd/apigen`; the cast receiver's Node tests (18 pass); the `packages/client-core` test files (1,494 pass), `packages/contracts` (26 pass) and the `web` unit tests (431 pass); `tsc --noEmit` for `packages/client-core` and `web`. Not run: `go test ./...`, any native build or native test, any browser or device, Docker.

0. `TestListeningHTTPPreferenceEnvelopeAndCAS` (`internal/httpapi`) fails here with "explicit reviewed helper binary required"; it needs the storage helper binary and is unrelated to this lane.
1. **Passthrough (Android TV / Fire TV with an AV receiver).** Play an MKV with TrueHD Atmos and one with DTS-HD MA over HDMI. Pass: the receiver shows the bitstream format; `GET /v1/playback/sessions/{id}` shows `decision.audio.action: "direct"`; the trace note `audio_passed_through_to_receiver`. Switch the box to its own speakers or Bluetooth and replay: pass is a conversion (`audio.action: "transcode"`). Turn on "Don't send sound to my receiver untouched": conversion.
2. **Device tone mapping (Apple TV on an SDR television, or HDR output off).** Play an HDR10 HEVC MP4. Pass: `presentation.mode: "direct"`, no server conversion, picture not grey. Turn on "Turn off HDR": the server converts and tone-maps.
3. **HEVC MKV on Apple.** Pass: `presentation.container: "fmp4_hls"`, plays, HDR kept.
4. **`hev1` MP4 on Apple.** Pass: repackaged (`mode: "stream"`, video `copy`) rather than a failed first play. If AVPlayer does play `hev1` directly on the device under test, remove `codecTags` from `appleDeclaredProfile`.
5. **mpv, Automatic.** With mpv present play: an H.264/AAC MP4 (pass: `engine: "native"`), an MKV with DTS (pass: `engine: "mpv"`, `mode: "direct"`), an MP4 with a PGS track selected (pass: `engine: "mpv"`, `subtitle.delivery: "device"`), then switch the PGS track off (pass: a new generation, back to `native`).
6. **Quality.** Remote connection, saved default Original: start; choose 720p in the player (pass: same session id, new generation, playback continues from the same second, queue intact); choose Automatic and throttle the link (pass: `stepQuality('down')` requests, generations climb, no restart from zero).
7. **Connection estimate.** Start a 20 Mb/s title with `throughputKbps: 5000`: pass is `422 connection_too_slow` with `error.reason: "480p"`; with `quality: {mode:"automatic"}` the start succeeds on `automatic`/`480p`.
8. **Player failure.** Force a decode failure (a capability claim the device cannot meet): pass is one `PATCH … failure`, the same session and queue entry, a new generation by another route; after three the viewer sees the error.
9. **Remembered tracks.** Pick Japanese audio and English subtitles on episode 1; start episode 2 with no choice: pass is the same languages. Turn subtitles off; next episode starts without them.
10. **fMP4-only device** (a third-party client with `hls_fmp4` and no `hls_ts`): play a VP9 file. Pass: a converted H.264 stream in `fmp4_hls` that plays. Fail: the producer errors; then restrict the H.264 entry of `DefaultConversionOutputs` to `hls_ts`.
11. **Cast.** Pair a Chromecast and play: pass is `PUT /v1/me/devices/current/capabilities` 204 in the server log and playback as before; on a Google TV with a receiver, AC-3 passes through.
12. **Home versus away.** With a VPN range listed in the owner's home networks, a request from it must get `quality.home` and no remote bitrate limit; from outside, `quality.remote` and the limit.

## 9. Not finished

- **No player or settings screen is adapted** (by design: client lanes). Until they are, the web and native players still show their old quality and track menus, and the three override rows and the player row are declared but not wired to any storage or to the published document.
- **The native apps will not publish capabilities or type-check against the new shared package until adapted** (they called the removed publishers and types). The web app type-checks; its player still restarts instead of calling the new change methods.
- **HEVC, HDR-preserving, lossless and 7.1 conversion outputs are not produced**: the planner chooses them only when the table offers them, and nothing registers them yet.
- **Converted H.264 in fragmented MP4** (devices without MPEG-TS HLS) is planned but was never run against a real producer.
- **Styled and image subtitles are drawn by the device only for tracks inside an original file being played directly.** Beside a stream they are still burned in.
- **Automatic quality is not wired into any player**; the decision helper and the server operation exist. There is no server bandwidth-test endpoint: the estimate is the client's own.
- **Route-failure memory is still invisible to the owner** and cannot be cleared from the console.
- **Remembered choices** have no "forget" control and are not shown in settings.
- **Not run:** the full Go suite (`go test ./...`), native builds and tests, browsers, devices.
- **Documents in `portico-internal`, `server/api/README.md` and the developer guide page `server/api/guide/04-playback-v1.md` are not updated** (out of this lane's reach or scope; listed in section 7).
- Internal Go seams `playback.Service.Create*`, `Get`, `Stop`, `Progress` and the quality-rung helpers used by `playback-offers` remain for fixtures and that read.

## 10. Merge with the base (added after the lane report above)

`audit-implementation/main` was merged into this branch seven times as it moved (last at `0301eed`). Where this section and sections 1–9 differ, this section is current.

**What changed in this lane's own work because of the merge**

- **Key scopes.** `start_playback_session`, `change_playback_session`, `put_device_capabilities`: `ScopePlayback`; `get_device_capabilities`, `get_playback_options`: `ScopeRead`. The scope-table entries of the removed routes (legacy start/read/stop, `/progress`, `client-profile`, `route-failures`) are deleted, and the scope test no longer knows "shared patterns".
- **Home or away** is `Dependencies.requestIsRemote` (the networking lane's one test) everywhere in playback: delivery context, saved-quality lane, owner and member limits.
- **Version choice.** A start that names no version plays the one this device plays with the least done to it; among equals the catalogue's best version (`compactcatalog.BestVersionOrder`), which is also the order `playback-options.versions` lists them in. (Replaces decision 16's "tallest, then highest bitrate".)
- **Automatic can name the original.** `quality: {mode: "automatic", id: "original"}` (start or PATCH) puts an Automatic session back on no step. This is for a client that chooses the step itself, as the web player does. `{mode: "automatic"}` with neither `id` nor `step` still leaves a stepped session where it is.
- **Styled and image subtitles a device draws itself** now come with the original file (the subtitles lane's delivery): when the engine playing the presentation draws the kind (this lane's decision from the capability document) or the client asks with `delivery: "embeddedClient"`, the track is selected with the lane's client rendering, `presentation.subtitles[]` lists the original (`format`, `url`, `companionUrl?`, `fontsUrl?`) and `presentation.subtitle.delivery` is `"device"`. Where no original can be delivered the engine reads the track from the file it is playing, as before. The WebVTT sidecar URL ends in `.vtt` (subtitles lane). Planning still treats a styled or image track beside a converted stream as a burn-in (`SubtitleBesideStream` is not set yet): see "Still open".
- **`packages/client-core`, reconciled with the web player lane:**
  - `playback-v1/choices.ts` (web player lane) is the player-facing model and stays: `QualityChoice`, `fixedQualities`, `offeredQualities`, its `AutomaticQuality` (measures, names the step). It now speaks the server's values: `qualityRequest('720')` is `{mode:'fixed', id:'720p'}`, Automatic is `{mode:'automatic', id:'<step>p' | 'original'}`; `fixedQualities` is the server's ladder one for one (adds 1440; 20/12/8/4/2/1 Mb/s); new `qualityChoiceOf(quality)`; `fixedQualityOf` also reads the quality in force. `PlaybackChoices` gained `subtitleTrackId` and `throughputKbps`.
  - `playback-v1/quality.ts` is the server vocabulary only. Removed from it: this lane's own `AutomaticQuality`/`AutomaticInput` (section 4.10) and the name `qualityRequest`; added `savedQualityRequest(saved)` and `requestInForce(inForce)`.
  - One change path: `V1QueuePlayer.changePlayback(choices: PlaybackChoices)` (the web player lane's), with this lane's `setQuality`, `stepQuality`, `setVersion`, `setAudioTrack`, `setSubtitleTrack`, `currentSession` on top of it. After a quality change the next queue entry starts with the quality the server put in force (so the step Automatic is on carries over).
  - `QueueSessionTransport.change(id, change, durationSeconds)` is the signature (`V1PlaybackApi.change` follows it; `durationSeconds` optional there). It reports the position first and sends `positionMs` with the change. New optional `onSessionChanged`.
  - `PlaybackService.engineFailed` re-plans the same session in place (`reportPlayerFailure` → `installPresentation`); it no longer starts the title again.
  - Settings: the Player row is the web-app lane's (`playerEngine`, strings `settings.row.player`, `settings.help.player`, `settings.player.*`, shown when `SettingsCapabilities.playerEngines`); this lane's duplicate strings and its `mpvPlayer` flag are gone. The three fixes rows stay. `quality.home` and `quality.remote` are preference rows with scope `deviceClass`; the `quality.away`, `quality.cellular` and `hdr` composites and `settings.row.hdr`/`settings.hdr.*` are gone.
- **Web (minimal adaptation, in `web/`):** `playback-v1/web-playback.ts` reports an engine failure as `PATCH … {failure, positionMs}` through the session controller and keeps the session and queue (it called the removed `route-failures`); `player/Player.tsx` ticks the quality in force on the session (`presentation.quality`, which is the saved default when the viewing named none) instead of reading the removed per-network composites; tests updated.

**Still open after the merge**

- **The planner does not yet hand the conversion lane's richer targets to the producer.** Both seams are in the tree and neither is wired to the other: this lane chooses the output from `playback.ConversionOutputs` (default: today's H.264/AAC/AC-3/E-AC-3, so nothing changes) and records it on the plan (`OutputVideoCodec`, `OutputBitDepth`, `OutputRange`, `AudioLossless`); the conversion lane produces HEVC, HDR, FLAC and 7.1 from `DeliveryPlan.ConversionTarget` stored with `playback.StoreConversionTarget`. The join is: register the real outputs with `SetConversionOutputs`, build a `decoder.ConversionTarget` from the chosen output in `planDelivery`'s full-conversion route, and store it in the transaction that persists the plan. Until then every conversion is the legacy one.
- Three `internal/playback` tests fail on the base itself since the conversion work landed (`TestDecisionMatrixAcrossDevices/surround…`, `TestDeliveryRouteSelectionAndExecutableCodecPolicy` (their expected FFmpeg arguments are the old form), `TestMPEGTSSeekRestartsOnIndexedPacket` (sandbox path)). `playbackruntime`'s `TestLinearAudioConversionKeepsCopiedHDR` now states that the test device's screen shows HDR10 (a decoder alone no longer implies the screen does).

- With originals now deliverable beside a stream, the planner could stop choosing a burn-in (and, on Automatic, mpv) for an engine that draws the kind: set `choice.SubtitleBesideStream` in `playbackv1/sessions.go` once the subtitles lane confirms an original exists for every such track. Not done here because it changes which titles convert.

- The web publishes the capability document but does not stamp the three overrides into it (no storage for them on web yet); it sends no `throughputKbps`; its quality menu is still built from `offeredQualities`, not from `playback-options.qualities`; a burn-in subtitle pick still goes through the subtitle route, not `PATCH {subtitles}`.
- Tests that fail on `audit-implementation/main` itself and are not this lane's (checked on an unmerged checkout): Go `TestLargeContainersListInIndexOrder` (`playbackv1`); in `httpapi`, 18 tests outside playback (the load-fixture ones that stop at "setup 401", the route lane, OpenAPI coverage and restriction matrices for other lanes' new routes, `TestAdministrationTierBoundaries`, `TestAdministrationOwnerBoundaryAndLibrarySettingsRoundTrip`, `TestFoundationContractAndReplay`, and `TestCastReceiverCallsRegisteredRoutes`, whose inventory lacks the receiver's new `/v1/cast/devices/` call; this list is from the merge at `e4a0ad1`, and the whole package was not run again after the later merges, only its playback, subtitle, lane and scope tests, which pass); every `TestPlaybackV1*`, delivery, capability and scope test passes; client-core `group-session-coverage` (3), `receiver-events` (1), `remote-onboarding` (1), `stream-options` (1). `web` `artwork-budget` timed out under load here.
