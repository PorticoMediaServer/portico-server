# 02 — Playback negotiation: device profiles, the delivery decision, the session protocol, progress and resume

Audit date: 11 October 2026. Read-only source review of the working tree as it stands. Nothing here was run: every statement about behaviour is a **source finding**, not a reproduced one.

Path shorthand used throughout (all relative to `/home/justin/projects/Portico`):

| Short | Full path |
|---|---|
| `S/` | `apps/portico-server/server/internal/` |
| `CC/` | `apps/portico-server/packages/client-core/src/` |
| `W/` | `apps/portico-server/web/src/` |
| `N/` | `apps/portico-react-native/apps/native/src/` |

Comparison paths are written `jellyfin/…`, `jellyfin-web/…`, `plezy/…` and so on.

Terms used:

- **Direct play**: the client reads the original file by byte range.
- **Remux / direct stream / "copy"**: the server repackages the original audio and video into another container (here HLS) without re-encoding.
- **Conversion / transcode**: the server re-encodes audio, video or both.
- **Capability profile / device profile**: the document a client sends to say what it can decode and display.
- **Presentation**: Portico's name for the concrete bytes one session is being served; each has a **generation** number, and new bytes mean a new generation and a new URL.
- **Lease**: a session stays alive only while the client keeps reporting; when the lease lapses the server ends it.

## Scope and sources

**Compared:** who makes the play decision and from what inputs; how devices declare capabilities; the ladder of outcomes; quality and bitrate ceilings; version, audio and subtitle selection; runtime fallback; the session lifecycle (start, change, timeline, stop, lease, cleanup); progress, resume and watched rules; stream limits and admission.

**Portico files read in full or nearly so**

- Docs: `Project Architecture/04 Playback and Media.md`, `Specs/Spec — Playback Protocol v1.md` (all 441 lines), `Backlog.md`, `server/api/playback-admission.md`.
- Server decision: `S/playback/delivery_decision.go`, `delivery_plan.go`, `client_profile.go`, `delivery_source.go`, `delivery_policy.go`, `delivery_settings.go`, `delivery_context.go`, `quality_ladder.go`, `v1.go`, `v1_session_helpers.go`, `service.go`, `offers.go`, `offers_types.go`, `route_failures.go`, `personal_progress.go`, `subtitle_plan.go`, `subtitle_delivery.go`, the first 140 lines of `hls_renditions.go`, `hls_timeline.go` (constants), `audio_plan.go` (reservation), `detail_refresh.go` (part), `remote.go` (part).
- Server protocol: `S/playbackv1/sessions.go`, `sessions_change.go`, `capabilities.go`, `devices.go`, `options.go`, `subtitles.go`, `admission_gate.go`, `audio.go`, `skips.go`, `restore.go`, function index of the rest.
- Related server code: `S/catalog/personal_activity.go` (progress acceptance), `S/catalog/continue_policy.go`, `S/operations/preferences.go` (playback and quality preferences), `S/access/enforcement.go` and `limits.go` (member limits), `S/subtitles/autoselect.go` and `presentation.go` (part), `S/assets/stream_detail.go` (probe facts), `S/httpapi/playback_v1_routes.go` (part), `playback_sessions.go`, `playback_v1.go` (dispatch), `delivery_routes.go`, `request_locality.go`, `cmd/server/main.go` (HLS wiring).
- Clients: `CC/client-profile.ts` (all), `CC/playback-v1/capability-profile.ts`, `quality.ts`, `timeline.ts`, parts of `sessions.ts`, `v1-queue-player.ts`, `legacy-bridge.ts`, `options.ts`; `CC/index.ts` (failure escalation, audio selection, play); `CC/pending-track-choice.ts` (head); `N/bridge/audio-decode.ts`, `device-profile.ts`, `client-profile.ts`, `capability-publisher.ts`; `W/player/Player.tsx` (quality handlers), `W/player/engine.tsx` (play path).

**Comparison files read**

- Jellyfin server: `MediaBrowser.Model/Dlna/StreamBuilder.cs` (BuildVideoItem, BuildStreamVideoItem, GetVideoDirectPlayProfile, source sorting, bitrate test), `ProfileConditionValue.cs`, `Session/TranscodeReason.cs`, `Jellyfin.Api/Helpers/MediaInfoHelper.cs` (policy, remote bitrate, sorting), `Emby.Server.Implementations/Library/UserDataManager.cs` (UpdatePlayState), `MediaStreamSelector.cs` (all), `MediaSourceManager.cs` (default stream selection, by grep), `Session/SessionManager.cs` (timers, progress, stop, by grep), `PlaystateController.cs` (routes), `ServerConfiguration.cs`, `UserPolicy.cs`, `TranscodeManager.cs` (ping timer).
- Jellyfin clients: `jellyfin-web/src/scripts/browserDeviceProfile.js` (every special-case comment and the code around the ones cited), `src/utils/bitrateTest.ts`, `src/components/playback/playbackmanager.js` (error retry, cinema mode, by grep); `jellyfin-androidtv/.../util/profile/KnownDefects.kt`, `preference/UserPreferences.kt`.
- Plezy: `lib/services/jellyfin_playback_bundle.dart`, header of `playback_progress_tracker.dart`, grep of `jellyfin_client/parts/playback.dart`.
- Listed only (names and layout): Swiftfin `Shared/Objects/PlaybackDeviceProfile.swift` and custom device profile screens, Streamyfin `utils/profiles/*`.

**Not reached:** Jellyfin's `ConditionProcessor.cs` and `StreamingHelpers.cs` line by line; Swiftfin, Streamyfin, Wholphin and jellyfin-android profile builders in detail; Plezy's Plex decision mapper; Portico's linear (Live TV) session code beyond its entry points; the music `audio_plan_v2.go` (auditor 10); the HLS producers (auditor 03); the subtitle service internals (auditor 04); group, transfer and device-command code. Plex is closed source: statements about Plex are from memory of its public documentation and are marked as such.

## How Portico does it today

### 1. The server decides, from a stored device profile

The client never chooses a play method. It asks for a session (`POST /v1/playback/sessions`), and the server answers with one presentation: a URL, a mode (`direct` or `stream`) and a per-stream decision with reasons (`S/playbackv1/sessions.go:146-163`, `options.go:694-768`).

The decision is a pure function, `planDelivery` (`S/playback/delivery_plan.go:224`), of:

- **the file's facts** read from the scanner's stream rows (`delivery_source.go:95-163`): container, video codec, profile, level, bit depth, size, frame rate, interlacing, rotation, dynamic range, Dolby Vision profile/level/compatibility id, codec tag, bitrates; every audio track's codec, channels, sample rate, language, default and commentary flags;
- **the device profile** stored on the server (below);
- **the resolved policy**: the network class the server worked out, the viewer's per-device-class delivery preferences, the owner's switches and ceilings (`delivery_policy.go:191-242`, `delivery_settings.go`);
- **the quality target**: the client's request (`original` or `limit` with ceilings), narrowed by owner and member ceilings (`v1.go:58-72`);
- **routes this device already reported broken for this file** (`route_failures.go`);
- **what this server's FFmpeg can do** (decoders, encoders, tone-map filter, fMP4 muxer) and the hardware encoder probe.

### 2. Two capability documents, and which one wins

There are two documents a device can publish:

1. **The planner profile** (`PUT /v1/playback/client-profile`; `S/playback/client_profile.go:47-140`, mirrored in `CC/client-profile.ts`). It lists video codecs (profiles, max level, bit depths, max size, frame rate, bitrate, dynamic ranges, Dolby Vision profiles, interlaced), audio codecs (max channels, sample rate, object audio, passthrough), the audio output (route such as `hdmi`, max channels, spatial), **transports** (`direct`, `hls_ts`, `hls_fmp4`, each with its own container, video and audio lists), subtitle formats it draws, a device bitrate ceiling and whether it can switch audio tracks inside an original file. Each codec entry carries `evidence`: `declared` (the app's own table) or `probed` (asked of the device at run time).
2. **The v1 capability profile** (`PUT /v1/me/devices/current/capabilities`; spec §3; `S/playbackv1/capabilities.go`). A different shape: `form`, `network`, `containers`, `streaming.hls.{fmp4,ts}`, `video[]`, `display`, `audio[]`, `audioDecode[]`, `subtitles[]`, `features{}`.

The server **translates** the v1 document into a planner profile when it is stored (`capabilities.go:213-316`, `devices.go:58-116`). When planning, the translated copy is read **first**, and the directly published planner profile only if the device has no v1 document (`client_profile.go:459-486`, first query at line 466).

Who publishes what:

- The **web app** publishes only the planner profile, built by probing the browser (`CC/client-profile.ts` `probeWebProfile`, `W/bridge/client-profile.ts:71`).
- The **native app** (Apple, Android, Fire TV) and **Vega** publish **both**: the planner profile (`N/bridge/client-profile.ts:21-34`) and a v1 profile derived from it (`N/bridge/capability-publisher.ts:20-35`, `N/bridge/audio-decode.ts:25-72`, `CC/playback-v1/capability-profile.ts:34-74`). So for native devices the **translated v1 copy is the one planning uses**. The translation is lossy; see B-02.1 to B-02.5.

A device that has published nothing is planned against a built-in baseline: MP4 with H.264 and AAC/MP3 plays directly, MPEG-TS HLS carries H.264 with stereo AAC, nothing else (`client_profile.go:369-392`).

Per-family declared tables exist for web, Apple (AVPlayer), Android (ExoPlayer), Fire TV, Vega, webOS, Tizen, Roku and six Cast models (`CC/client-profile.ts`, declared tables from line 99; probes from line 337). A golden copy lives at `S/playback/testdata/client-profiles.json`.

### 3. The ladder: four routes, cheapest admissible one wins

`planDelivery` evaluates four routes and records, for each, every rule that closed it (`delivery_plan.go:347-508`):

| # | Route | What it is | Typical reasons it closes |
|---|---|---|---|
| 1 | `original` | Direct play of the file | container not in a `direct` transport; any video or audio rejection; wanted audio track is not the first and the device cannot switch tracks in a file; a quality ceiling; burn-in subtitle; viewer preference "never" |
| 2 | `copy_remux` | Video and audio copied into HLS | no HLS transport carries both streams; server cannot copy that codec into that HLS container; owner turned remux off; a ceiling or burn-in |
| 3 | `audio_conversion` | Video copied, audio re-encoded | video itself rejected; transcoding disabled; server lacks decoder/encoder |
| 4 | `video_conversion` | Full conversion | transcoding disabled; device has no MPEG-TS HLS transport with H.264 and AAC |

Checks on video (`delivery_decision.go:160-207`): codec known to the device and listed on the transport; profile; level; bit depth; width/height; frame rate; bitrate; interlacing; rotation (copy routes only); dynamic range, including a Dolby Vision fallback rule (a device without the file's DV profile is judged on the base layer: HDR10 for profile 7 and compatibility ids 1/6, HLG for 4, SDR for 2, none for profile 5); and a rule that a DV sample entry (`dvh1`/`dvhe`) in an original file is refused by non-DV decoders.

Checks on audio (`delivery_decision.go:209-225`): codec; passthrough codecs need a passthrough output route (`hdmi`, `arc`, `optical`); channels; sample rate.

A fact the file never had probed does not reject a route (zero or empty means "not stated").

Ranking (`delivery_plan.go:537-568`): the viewer's `prefer` settings first, then the owner's **planning policy** (`maximum_fidelity` and `minimize_server_work` pick the cheapest route; `maximum_compatibility` picks the most converted one). If nothing is admissible the start is **refused** with a typed reason (`delivery_plan.go:525-535`), except that an original whose only problem is "wanted audio track unreachable" plays with its first track and says so (`:514-524`).

Conversion output is fixed: **H.264, 8-bit, SDR, in MPEG-TS HLS**, with AAC stereo or 5.1 (or AC-3/E-AC-3 5.1 to a passthrough route) (`delivery_plan.go:621-681`, `delivery_decision.go:347-372`, `delivery_plan.go:829`). HDR sources are tone-mapped unless the owner turned tone mapping off. Audio the device can already decode is copied even under a converted picture (`:623-631`).

### 4. Quality and ceilings

- The client asks for `{mode: original}` or `{mode: limit, maxVideoBitrateKbps, maxHeight, maxAudioBitrateKbps}`. "Automatic" is meant to be a client-side choice (spec §5.2).
- A ceiling closes the picture-preserving routes when the file exceeds it. Bitrate has a 10 % tolerance and compares the larger of the file's average and its declared peak (`delivery_plan.go:210-214`, `:307`).
- Owner ceilings (`delivery_settings.go`): max video bitrate (applied to remote connections only, `v1.go:62-66`), max audio bitrate, max height, HDR allowed, remux enabled, max simultaneous conversions (total, hardware, software).
- Member ceilings (`S/access/limits.go:95-103`): max streams, remote bitrate, content rating, schedule, channel and label policy.
- Viewer preferences per device class (`S/operations/preferences.go:188-190`, `:123-137`): `delivery.directPlay|directStream|transcode` each `allow|prefer|never|require`; per-network-lane quality presets and `allowHDR`.
- A fixed ladder (2160p 20 Mbps … 360p 1 Mbps) sets conversion budgets (`quality_ladder.go:46-53`).

### 5. Network class

Two separate classifiers exist:

- `requestIsRemote` (`S/httpapi/request_locality.go:64-104`): honours owner-configured LAN networks, trusted proxies (right-most forwarded address) and "treat my public address as LAN". It sets `Caller.Remote`, which gates the member's remote bitrate cap.
- `playback.RequestLocality` (`S/playback/delivery_policy.go:57-80`): private, loopback and link-local addresses are local, everything else remote; no owner configuration. Combined with the client's `X-Portico-Transport-Class` header it yields `local | wifi | cellular | remote | unknown`, which selects the viewer's preference lane.

### 6. Audio, subtitles and versions

- **Audio track** (`delivery_decision.go:231-266`): the viewer's explicit choice, else the first track in a preferred language (non-commentary first), else the default-flagged track, else the first non-commentary track.
- **Subtitles**: chosen by the subtitle service. Automatic selection (`S/subtitles/autoselect.go:75-112`) has three modes, `off`, `forced` (default) and `always`, plus a "foreign audio" rule. Delivery is decided by the **track's format**: text becomes a WebVTT sidecar, image and styled tracks are burned in (which forces a full conversion) unless the client asks for `embeddedClient` (`S/playbackv1/subtitles.go:35-79`).
- **Versions**: an item's files are grouped into versions and parts (`S/playbackv1/options.go:326-364`). Without a `versionId` the first available file in `part_index, token` order plays (`S/playback/v1.go:89-94`).

### 7. The session protocol

- **Start** (`S/playbackv1/sessions.go:525-764`): requires an `Idempotency-Key`; a replay returns the same session, a different body with the same key is refused. Admission (library access, restrictions, schedule, device approval, stream limits) runs under a per-account gate so two overlapping starts cannot both slip under a limit (`admission_gate.go`). If the stream is not ready within 1.5 s the answer is `202` with `retryAfterMs` and the client replays the same request.
- **Change** (`PATCH`, `sessions_change.go:51-202`): needs `If-Match` with the session revision. Version, audio track, quality or part changes create a new presentation (generation + 1) and re-run admission. Subtitle changes keep the generation unless burn-in toggles. A seek is a server matter only when something is being converted.
- **Timeline** (`sessions_change.go:263-378`): one call is progress, lease renewal and observation. It carries `seq` (must rise) and `generation`; stale reports renew the lease but change nothing. Cadence 10 s playing, 30 s paused, returned in `Report-Every-Ms`. Rate-limited to 10/s per device (`server/api/playback-admission.md`).
- **Stop** (`DELETE`, `:206-220`): idempotent, records a final position, fences the media URL.
- **Lease**: 120 s (`sessions.go:23-30`). A sweeper that runs only while sessions exist ends lapsed ones (`sessions_change.go:390-483`). An optional owner setting ends video sessions paused too long (`:492-516`).
- **Media URLs** are capability URLs: `/v1/media/{grant}/…`, unguessable, valid for one presentation, re-checked on every request against the session state, the sign-in, and the file's size and modified time (`S/playback/service.go:696-780`).
- **After a database restore** every live session is ended and every grant replaced (`S/playbackv1/restore.go`).

### 8. Progress, resume and watched

`AcceptPlaybackProgress` (`S/catalog/personal_activity.go:27-150`) runs inside each accepted timeline report:

- **Started**: position ≥ min(30 s, `startedThresholdPercent` of duration; default 5 %). Below that no resume point is kept.
- **Completed**: the client reported `ended`, or position ≥ the played threshold (library default 90 % for movies and episodes, viewer preference 95 % otherwise or when the viewer set it), or position reached the first **approved** credits marker (library mode `earliest`, `threshold` or `credits`).
- On completion: watched = true, resume = 0. On a started-but-not-complete report: resume = position **and watched = false** (`:117-119`).
- `privacy.pauseWatchHistory` stops the viewer's own history, watched state and resume from changing; the owner's play history is still written.
- At start, a stored resume point within 3 s of the end is ignored (`S/playback/service.go:464-466`).
- A "claim" and "fence" mechanism (`S/playback/personal_progress.go`) orders progress writers across devices, so an older session cannot undo a newer play or a manual "mark watched".

### 9. Failure at run time

When the player engine rejects a stream, the client calls `POST /v1/playback/route-failures` (`CC/index.ts:1453-1468`). The server records "this route failed for this file on this sign-in" for 7 days (`S/playback/route_failures.go:23`, `:63-90`) and the next plan skips it. The client retries from the same position, up to three times per play.

### 10. Remote sources, disc images

`.strm` files are admitted only from owner-approved network roots, and only when the remote file is MP4/MOV with H.264 and AAC/MP3 (`S/playback/remote.go:140`, `:168`); they always play as `direct`. `.iso` and `.img` files are inventoried as `unsupported_disc_image` (`S/catalog/inventory.go:42-47`). There is no BDMV or VIDEO_TS folder handling and no 3D flag.

## How the others do it

### Jellyfin

**The server computes, the client chooses.** A client posts its whole `DeviceProfile` with every `PlaybackInfo` request. The server returns every media source (version) of the item, each marked `SupportsDirectPlay`, `SupportsDirectStream`, `SupportsTranscoding`, with a ready `TranscodingUrl` that carries the transcode reasons. The client picks a source and a method and builds the stream URL itself (`jellyfin/Jellyfin.Api/Helpers/MediaInfoHelper.cs:180-330`).

**The profile is a small rule language.** `DirectPlayProfiles` (container + codec lists), `TranscodingProfiles` (what to convert to: container, protocol, codecs, max channels), `CodecProfiles` and `ContainerProfiles` with conditions over 26 values (`jellyfin/MediaBrowser.Model/Dlna/ProfileConditionValue.cs`): audio channels, bitrate, profile, sample rate, bit depth; video width, height, bit depth, bitrate, frame rate, level, profile, range type, codec tag, rotation; is-anamorphic, is-interlaced, is-AVC, ref frames, number of streams, is-secondary-audio, and more. `SubtitleProfiles` state, per format, whether the client takes it embedded, external, in HLS, or needs it burned in.

**The decision** (`StreamBuilder.cs:648-846`): try direct play against each direct-play profile; if not, direct stream (remux, possibly with audio re-encoded); if not, transcode using the first matching transcoding profile, then apply codec conditions to shape the output (level, bit depth, resolution). Special cases found:

- It will **switch to another audio track of the same language** (or among default tracks) if that one direct-plays (`:668-706`, `:1006`).
- DVD and Blu-ray folder items never direct play (`:716-720`); HLS manifests as sources never direct play (`:722-728`).
- A file of unknown bitrate is assumed to be 40 Mbps when a cap is set (`:1655-1680`).
- A subtitle that needs burning rules out direct play (`:1321-1331`).
- Reasons are a flag set of 28 values (`Session/TranscodeReason.cs`), surfaced in the dashboard and stream URL.
- Versions are sorted: a direct-playable local file first, then direct-streamable, then by fit to the bitrate cap, while keeping the version the user asked for first (`MediaInfoHelper.cs:367-412`).

**Server policy:** per-user `EnableVideoPlaybackTranscoding`, `EnableAudioPlaybackTranscoding`, `EnablePlaybackRemuxing`, `ForceRemoteSourceTranscoding`, `RemoteClientBitrateLimit` (per user, falling back to a server-wide one; applied only outside the configured LAN, `MediaInfoHelper.cs:518-540`), `MaxActiveSessions` (sign-ins, not streams).

**Clients carry the device knowledge.** `jellyfin-web/src/scripts/browserDeviceProfile.js` is 1,654 lines of probes and scars: Safari takes HEVC only with `hvc1`/`dvh1` tags (`:1464-1470`) and not in MPEG-TS HLS (`:379`); Firefox's MKV and fMP4 problems (`:217-219`, `:877-878`); Tizen and webOS DTS removal by model year (`:117`); Tizen's 32-stream limit (`:976`); HE-AAC handling (`:994`); FLAC and Opus channel limits per platform (`:1065`, `:1107`); which browsers tone-map (`:254-262`); Dolby Vision fallback layers that TVs play although they do not report DV (`:1250-1289`); anamorphic handling per platform (`:308-314`). User-facing overrides exist: allow DTS, allow TrueHD, max audio channels, prefer fMP4, limit resolution to the screen. Android TV keeps a per-model defect list (`jellyfin-androidtv/.../util/profile/KnownDefects.kt`) and preferences for max bitrate, audio behaviour (direct stream or downmix), refresh-rate switching, PGS and ASS direct play, resume rewind, cinema mode and "still watching" (`preference/UserPreferences.kt:57-304`). Swiftfin lets the user define custom device profiles.

**Auto quality:** the web client measures download speed against a server test endpoint in stages, takes 70 % of the result, and asks for that as its max bitrate (`jellyfin-web/src/utils/bitrateTest.ts:111-160`). There is no mid-stream adaptive switching.

**Failure at run time:** the client retries by asking for playback info again with direct play disabled, then with stream copy disabled (`jellyfin-web/src/components/playback/playbackmanager.js:3496-3521`). Nothing is remembered.

**Sessions and reporting:** a session is a signed-in device, not a playback. Clients post `Sessions/Playing`, `…/Progress`, `…/Stopped`, and ping a transcode by `PlaySessionId` (`PlaystateController.cs:201-249`). There is no sequence number, no generation, no idempotency. Playback that stops checking in is closed after a 5-minute idle check (`SessionManager.cs:609`, `:639-666`); a transcode dies 10 s or 60 s after its last ping (`TranscodeManager.cs:153-160`); an optional setting stops sessions paused too long (`SessionManager.cs:676-685`).

**Resume and watched** (`UserDataManager.cs:455-529`, `ServerConfiguration.cs:133-157`): under `MinResumePct` (5 %) no position is kept; over `MaxResumePct` (90 %), or within 1 s of the end, the item is played; items shorter than `MinResumeDurationSeconds` (300 s) are simply marked played; audiobooks use minutes (5 from the start, 5 from the end); unknown runtime means played. `Played` is never cleared by progress. Play count rises at start (`SessionManager.cs:848`).

**Default tracks:** audio by the user's language (or "original language"), with a "play default audio track regardless of language" switch, and a remembered per-item choice (`MediaSourceManager.cs:531-586`, `SessionManager.cs:1009-1037`). Subtitles by five modes, `Default`, `Always`, `OnlyForced`, `None`, `Smart`, over all subtitle types, with forced tracks of undefined language accepted (`MediaStreamSelector.cs`).

**Other:** pre-rolls ("cinema mode") through an intros API (`playbackmanager.js:265-272`); `.strm` to any URL with probing and optional forced transcoding; disc folders and ISO; a 3D format field.

### Plezy

Plezy is a client, so it shows what a good client needs from a server. Its player is mpv-based and plays almost anything directly; it asks Jellyfin for a transcode only when the user picks a capped quality preset (`plezy/lib/services/jellyfin_client/parts/playback.dart:332-398`). Notable habits: it always pins the chosen `MediaSourceId` because the server's default version can differ from the one shown (`jellyfin_playback_bundle.dart`); it does not report progress until the first frame has rendered; it queues progress locally when offline and syncs later (`offline_watch_sync_service.dart`); it flips watched locally at the threshold and only sends an explicit "mark played" if the server never saw the crossing; it marks sibling episodes of a multi-episode file; it pings the Plex transcoder while paused (`playback_progress_tracker.dart:19-81`).

### Plex (from public documentation, not verified here)

The server decides (a "decision" request), using client profiles shipped with the server plus per-request overrides. It offers per-user "limit remote stream bitrate", a server-wide remote limit, an "automatically adjust quality" option that changes quality mid-stream, automatic version choice, and a "terminate sessions paused for longer than" setting (which Portico's paused timeout cites).

## Architectural differences

Each entry ends with an assessment. "Deliberate" means the choice is written down; I still give a view.

### A. Who decides, and from what

### A-02.1 One server-made decision versus a menu the client chooses from
- **Portico:** the server returns one presentation with its URL and decision. The client cannot ask for "direct play this anyway" (`S/playbackv1/sessions.go:432-497`).
- **Jellyfin:** the server marks every version as direct-playable, direct-streamable or transcodable and hands back URLs; the client picks and may force (`MediaInfoHelper.cs:180-330`; `StreamBuilder.cs:1298-1306` `ForceDirectPlay`).
- **Why it matters:** Portico clients stay thin and identical; a wrong server decision cannot be overridden on the device, so the fallback path (A-02.21) and the profile's accuracy carry all the weight. Jellyfin clients can work around server mistakes but each client re-implements the choice differently.
- **Assessment: Portico ahead** for consistency. Deliberate (`04 Playback and Media.md` opening paragraph; "Thin clients" principle). What would change my view: evidence that device quirks need per-user overrides the server cannot know (see F-02.14).

### A-02.2 Profile stored per device versus sent with every request
- **Portico:** the device publishes once and on change; the server stores it keyed by device and reads it for every plan (`client_profile.go:459-486`). Republishing an unchanged profile costs one indexed read.
- **Jellyfin:** the full profile travels in every `PlaybackInfo` body.
- **Why it matters:** Portico's requests are small and every route (start, queue advance, transfer, preview) plans from the same facts. The cost is staleness: a profile can describe a previous HDMI sink or display mode until the client republishes. Native clients republish on a capabilities event with a 1.5 s settle (`N/bridge/capability-publisher.ts:30-33`); the web publishes at sign-in.
- **Assessment: Trade-off (subjective).** Deliberate (spec §3). The race to watch: play starts before a changed profile lands. `keepCapabilitiesPublished` exposes `ready` so the first play can wait; later changes are not awaited.

### A-02.3 Codec tables plus transports versus a condition language
- **Portico:** fixed fields per codec and per transport (`client_profile.go:70-101`). To express a new kind of limit the wire shape and the planner must both grow.
- **Jellyfin:** `CodecProfile`/`ContainerProfile` conditions over 26 values with `Equals`, `NotEquals`, `LessThanEqual`, `GreaterThanEqual`, `EqualsAny` (`ProfileConditionValue.cs`). A client can state "H.264 in TS on iOS 12 only up to level 4.2" without a server change.
- **Why it matters:** Portico's shape is easier to validate, hash and explain. It cannot express: ref-frame limits, anamorphic support, AVC vs Annex-B, codec-tag rules (`hev1` vs `hvc1`), audio profile (HE-AAC, DTS-HD vs DTS core), max stream count, per-container codec limits within one transport family (Opus only in MKV/WebM on old Tizen; FLAC max 2 channels on webOS; Opus max 2 channels in Safari). Portico gathers `Anamorphic`, `RefFrames`, `CodecTag`, `PixelFormat` and `HDR10Plus` at scan (`S/assets/stream_detail.go:28-56`) but the planner reads only `CodecTag`, and only for Dolby Vision.
- **Assessment: Trade-off (subjective), leaning Portico behind on coverage.** A per-codec-per-transport override (channels, level, tag list) would recover most of it without a rule language. Medium: wire shape, planner, each declared table.

### A-02.4 Two capability documents with a lossy translation between them
- **Portico:** the planner profile and the v1 profile coexist; the v1 one is translated and then preferred (see "How Portico does it" §2). The translation drops the per-transport codec lists, the audio output route and channel count, embedded audio switching, the interlaced flag, the "device tone-maps HDR itself" fact and bitmap subtitle rendering.
- **Jellyfin / others:** one profile document.
- **Why it matters:** the richer document (the one the comments and COMPAT fixes were written against) is not the one native devices are planned with. See B-02.1 to B-02.5.
- **Assessment: Portico behind.** Looks accidental (a migration in progress: `client_profile.go:134-136` and `S/playbackv1/audio.go:28-32` both say "until it publishes v1 capabilities"). Fix: pick one document. Either make v1 carry everything the planner profile does, or have the v1 route accept the planner shape. Medium.

### A-02.5 Evidence tags on every capability
- **Portico:** each codec entry says `declared` or `probed`, and the decision trace stores the profile's revision and evidence (`client_profile.go:29-38`, `:412-426`).
- **Jellyfin:** none.
- **Assessment: Portico ahead.** Small, useful for support. Keep.

### A-02.6 A conservative baseline for unknown clients
- **Portico:** an unpublished device gets H.264/AAC/MP3 in MP4 or MPEG-TS HLS only (`client_profile.go:369-392`), and the trace notes `client_published_no_profile`.
- **Jellyfin:** I did not verify what a request without a profile gets.
- **Why it matters:** a third-party client that forgets to publish converts nearly everything, silently. The trace says why, but only to someone who reads it.
- **Assessment: Unclear.** Safe default. Consider surfacing "this device sent no profile" in Now Playing.

### A-02.7 Tolerant of unknown names, strict about version
- **Portico:** unknown codec names are kept and never match; an unknown profile `version` is refused; unknown fields are ignored in the planner profile but **refused** in the v1 document ("unknown fields are refused, like every v1 request", `capabilities.go:18-20`), although spec §3 says "Extra fields… are ignored".
- **Assessment:** see Doc/code disagreements. The planner-profile behaviour is the better one for forward compatibility.

### B. The ladder and what conversion produces

### A-02.8 Four named routes with a recorded reason for every closed route
- **Portico:** `original`, `copy_remux`, `audio_conversion`, `video_conversion`; a `DecisionTrace` lists each route, whether it was admissible and each rejection with detail (`delivery_decision.go:48-102`), stored per session (`delivery_plan.go:909-914`, capped at 32 KiB).
- **Jellyfin:** DirectPlay, DirectStream, Transcode, with a flag set of reasons and debug logs.
- **Why it matters:** Portico can answer "why not the cheaper thing" for a past session without logs. But the v1 session resource shows only coarse reasons: every video problem (profile, level, bit depth, resolution, frame rate, interlacing, rotation, range) collapses to `video_codec_unsupported` (`delivery_plan.go:487-489`, `S/playbackv1/options.go:674-735`). Jellyfin's flags are finer at the surface.
- **Assessment: Portico ahead underneath, behind at the surface.** Small: map the trace's rejection codes into `decision.reasons`.

### A-02.9 Conversion output is fixed; Jellyfin's is negotiated
- **Portico:** always H.264 8-bit SDR, MPEG-TS HLS, AAC 2.0/5.1 (or AC-3/E-AC-3 5.1). No HEVC or AV1 output, no HDR-preserving conversion, no fMP4 conversion output, no 7.1 (`delivery_plan.go:462-470`, `:621-663`, `:829`).
- **Jellyfin:** the client's `TranscodingProfiles` choose container (TS or fMP4), video codec list (H.264, HEVC, AV1) and audio codec list; codec conditions shape level and bit depth; HDR survives an HEVC transcode when the client supports the range.
- **Why it matters:** any ceiling on a 4K HDR title in Portico yields 8-bit SDR H.264. A viewer on a 20 Mbps remote link with an HDR TV loses HDR and gets H.264 at a bitrate where HEVC would look much better. A device that supports only fMP4 HLS cannot receive a conversion at all (B-02.14).
- **Assessment: Portico behind.** Partly deliberate (one qualified conversion path is simpler to guarantee). Large: encoder graphs, hardware paths, HLS codec strings, per-device targets. A first step with most of the value: HEVC Main10 HDR10 output in fMP4 for devices that declare it.

### A-02.10 MPEG-TS is tried before fMP4
- **Portico:** `hlsTransports` lists TS first "the older, more forgiving container" (`delivery_decision.go:305-321`).
- **Jellyfin:** client preference (`preferFmp4HlsContainer`); Safari must use fMP4 for HEVC (`browserDeviceProfile.js:379`).
- **Why it matters:** correctness then depends on each device's TS codec list being exact. See B-02.1.
- **Assessment: Trade-off (subjective).** For copy routes carrying HEVC, AV1, FLAC, Opus or Dolby Vision, fMP4 is the only correct container on Apple devices; preferring fMP4 whenever the device declares it would remove a class of error.

### A-02.11 An owner "planning policy"
- **Portico:** `maximum_fidelity`, `maximum_compatibility`, `minimize_server_work` (`delivery_settings.go:44-49`). In the code the first and third rank identically (`delivery_plan.go:551-559`).
- **Jellyfin:** none.
- **Assessment: Unclear.** Two of three options do the same thing today; `maximum_compatibility` converts everything it may. Worth either giving `minimize_server_work` a distinct meaning or removing it.

### A-02.12 Viewer delivery preferences with `require` and `never`
- **Portico:** per device class, `delivery.directPlay|directStream|transcode` ∈ `allow|prefer|never|require` (`S/operations/preferences.go:188-190`), leading to typed refusals such as `direct_play_required_but_unavailable`.
- **Jellyfin:** `EnableDirectPlay/EnableDirectStream/EnableTranscoding` request flags (used mainly for retries) and per-user admin permissions.
- **Assessment: Portico ahead** in expressiveness; it is a power-user feature. Check the clients expose it; I did not trace the settings UI.

### A-02.13 Refusal is a first-class outcome
- **Portico:** `ErrDeliveryRefused` with a code (`delivery_plan.go:67-91`); `PlanV1` lets the title page preview it.
- **Jellyfin:** returns sources with nothing supported and an error code such as `NoCompatibleStream`.
- **Assessment: Portico ahead.** Keep.

### A-02.14 Jellyfin re-selects an audio track to keep direct play; Portico does not
- **Portico:** picks one track, then plans around it. If that track needs conversion while a sibling track in the same language would direct-play, Portico converts.
- **Jellyfin:** builds candidate streams (same language, or default-flagged) and uses the first that fits (`StreamBuilder.cs:668-706`, `:1006`).
- **Why it matters:** common on discs ripped with TrueHD first and AC-3 second in the same language.
- **Assessment: Portico behind.** Small to medium: try same-language non-commentary siblings before leaving the cheaper route, and say so in the trace. See F-02.10.

### A-02.15 Dolby Vision and HDR modelling
- **Portico:** a range list per codec, a DV profile list, and a fallback rule keyed on the file's compatibility id; profile 5 without DV support converts and is flagged "colors approximate" unless the tone-mapper can reshape (`delivery_decision.go:126-155`, `delivery_plan.go:649-663`). DV profile matching is by major number only: the client sends `dolbyvision:8.1` and `8.4`, the server keeps `8` (`S/playbackv1/capabilities.go:193-209`).
- **Jellyfin:** a `VideoRangeType` enum of a dozen values (`DOVIWithHDR10`, `DOVIWithHLG`, `DOVIWithSDR`, `DOVIWithEL`, `DOVIWithHDR10Plus`, `DOVIInvalid`, …) that clients list as supported.
- **Why it matters:** both get the main cases. Jellyfin separates "DV with enhancement layer" (profile 7 FEL/MEL) and "invalid" DV; Portico has `EnhancementLayer` in its facts but the planner does not read it.
- **Assessment: roughly level.** Portico's fallback rule is cleaner. Open risk: profile 7 with a full enhancement layer copied to a DV-profile-7-declaring device that only handles MEL.

### A-02.16 Passthrough is route-aware
- **Portico:** a codec marked `passthrough` is usable only while the output route is `hdmi`, `arc` or `optical` (`delivery_decision.go:215-217`, `:374-376`).
- **Jellyfin:** the client just lists the codec; route changes are the client's problem.
- **Assessment: Portico ahead in design**, but the fact is lost for v1-publishing devices (B-02.2).

### A-02.17 Audio-track switching inside an original file is a declared capability
- **Portico:** `embeddedAudioSwitching`; without it a non-first track closes direct play (`delivery_plan.go:366-368`).
- **Jellyfin:** `IsSecondaryAudio` condition per codec profile (`browserDeviceProfile.js:494-500`).
- **Assessment: level** in intent; lost in translation for native (B-02.3).

### C. Quality, bitrate and network

### A-02.18 Quality changes only when a person asks
- **Portico:** binding rule (spec header and §5.2). The server never lowers quality for load.
- **Jellyfin:** same in practice (no adaptive ladder). **Plex** (docs): optional automatic mid-stream adjustment.
- **Assessment: Trade-off (subjective).** Deliberate. The consequence is that on a link that cannot carry the chosen quality the viewer buffers until they change it; the client-side "Automatic" was meant to cover that and is not wired (A-02.19).

### A-02.19 "Automatic" quality is specified as client-side, built, and unused
- **Portico:** `CC/playback-v1/quality.ts` implements a ladder, 80 % of the estimate, 25 % hysteresis and `qualityDecision`; I found no caller in the web or native apps. The queue player starts every session with `{state:'playing'}` and no quality (`CC/playback-v1/v1-queue-player.ts:332`). No bandwidth test endpoint exists.
- **Jellyfin:** staged download test, 70 % of the result, applied before play (`bitrateTest.ts:112-160`).
- **Why it matters:** today every start is `original`. A remote viewer without an owner cap gets the full file or a full-bitrate remux.
- **Assessment: Portico behind.** See F-02.4 and B-02.6.

### A-02.20 How a ceiling is compared with a file
- **Portico:** max(average, declared peak) against the ceiling with 10 % tolerance; unknown bitrate never exceeds (`delivery_plan.go:210-214`, `:307`). The ceiling is a **video** bitrate ceiling compared against the **whole file's** rate; conversion output is video ceiling plus audio budget on top.
- **Jellyfin:** container bitrate against total max, no tolerance; unknown is assumed 40 Mbps; audio bitrate is subtracted from the video budget (`StreamBuilder.cs:1655-1680`, `:1129-1145`).
- **Why it matters:** Portico's tolerance avoids pointless conversions of files a hair over a round number. Using the peak makes it stricter for variable-bitrate files. Under a member's "remote bitrate" cap the delivered stream can exceed the cap by the audio rate (up to 640 kbps).
- **Assessment: Trade-off (subjective).** Suggest defining the member cap as total bitrate, since that is what an owner means by it.

### A-02.21 Two different "is this remote?" answers
- **Portico:** member caps use the configurable classifier; the viewer's lane and the server-wide cap also consult the unconfigurable one (`S/playback/v1.go:74-76` ORs both). See B-02.9.
- **Jellyfin:** one `IsInLocalNetwork` with configured LAN subnets.
- **Assessment: Portico behind** (accidental duplication). Small: derive the network class from `requestIsRemote`.

### A-02.22 A client-declared transport class
- **Portico:** `X-Portico-Transport-Class: wifi|cellular|ethernet` shapes the lane; "a client that says it is on cellular is believed" (`delivery_policy.go:82-100`), and per-lane defaults differ (cellular: no HDR, 8 Mbps preset).
- **Jellyfin:** separate client-side settings for "home" and "internet" quality, plus mobile clients' own cellular settings.
- **Assessment: Portico ahead** in having one model. Note the surprise in B-02.10 (cellular default `allowHDR=false` forces conversion).

### D. Failure, sessions and reporting

### A-02.23 The server remembers a failed route
- **Portico:** 7 days per sign-in family and file (`route_failures.go:23`, `:93-114`); any engine failure code closes the route in use; a failed full conversion is never escalated.
- **Jellyfin:** client-side retry per attempt, nothing remembered.
- **Why it matters:** Portico pays one failure per file per device and then plays without a visible error next time. But a profile mistake affecting a whole codec is learned one file at a time, and a transient failure blocks direct play of that file for a week with nothing in the UI to reset it.
- **Assessment: Portico ahead**, with a refinement to consider: count failures per (device, codec/container shape) and tell the owner "this device keeps failing HEVC direct play".

### A-02.24 A playback session is its own resource
- **Portico:** id, revision, generation, lease, idempotent start, `If-Match` on change, typed end reason readable after the fact (`S/playbackv1/sessions.go:113-141`).
- **Jellyfin:** a session is a device; "now playing" is a field on it; `PlaySessionId` ties a transcode to reports.
- **Why it matters:** Portico can fence stale writers (a late report from an old generation, a duplicated start, two tabs). Jellyfin relies on timing.
- **Assessment: Portico ahead.** Deliberate (spec §2, §16). The cost is protocol weight for third-party clients: keys, revisions, generations, 202 replays.

### A-02.25 One timeline call versus start / progress / stopped / ping
- **Portico:** a single report with `seq` and `generation`; the server answers with the cadence; rate-limited; duplicate reports coalesce lease writes (`sessions_change.go:291-303`).
- **Jellyfin:** four endpoints, no ordering guarantees.
- **Assessment: Portico ahead.**

### A-02.26 Lease and cleanup timing
- **Portico:** 120 s lease, event-driven sweeper (sleeps until the earliest lease), paused-too-long limit for video only.
- **Jellyfin:** 5-minute idle scan (so up to about 10 minutes), transcode kill after 10–60 s without a ping, optional paused limit.
- **Why it matters:** a crashed Portico client frees its stream slot and conversion within 2 minutes. A client suspended by its OS for more than 2 minutes (phone call, tab discarded) loses the session and must start a replacement at the last position (`CC/playback-v1/sessions.ts:237-242` does this for a lapsed lease only).
- **Assessment: Portico ahead**, deliberate (spec §5.8).

### A-02.27 Start can answer "being prepared"
- **Portico:** 202 with `retryAfterMs: 500` after waiting 1.5 s; the client replays the identical request (`sessions.go:23-30`, `:699-710`).
- **Jellyfin:** returns a URL at once; the player waits on the first segment.
- **Assessment: Trade-off (subjective).** Portico's is honest and lets the UI show "preparing"; it is one more thing a third-party client must implement.

### A-02.28 Capability URLs instead of tokens in query strings
- **Portico:** `/v1/media/{grant}/…` per presentation, revoked with the session, re-validated on each request against sign-in state and the file's size and modified time (`S/playback/service.go:696-780`).
- **Jellyfin:** the user's API key or token in the stream URL.
- **Assessment: Portico ahead** (a leaked URL dies with the session). Detailed review belongs to the security auditor.

### A-02.29 A plan is pinned to the exact file
- **Portico:** each session pins size and modified time; a changed file ends the grant (`service.go:414-424`, `:759-764`).
- **Jellyfin:** none.
- **Assessment: Portico ahead** for correctness; it means replacing a file mid-play stops playback, which is the right failure.

### A-02.30 Seeking
- **Portico:** HLS manifests cover the whole title on a fixed 6 s grid; seeking in direct and copy modes is a plain media request; only conversions need `PATCH seek`, which restarts production and bumps the generation (`hls_timeline.go:14-21`, `sessions_change.go:161-166`).
- **Jellyfin:** similar for HLS (segment requests restart the encoder); progressive transcodes need a new start time.
- **Assessment: level.** The 7-day manifest bound (`hls_timeline.go:19-21`) is generous.

### A-02.31 Stream limits and conversion caps
- **Portico:** member `maxStreams`, owner per-account cap, owner server-wide cap, and caps on simultaneous conversions (total, hardware, software), all checked at admission, with replacement sessions not double-counted (`S/access/enforcement.go:105-122`, `S/playback/v1_session_helpers.go:21-51`, `audio_plan.go:45-62`).
- **Jellyfin:** `MaxActiveSessions` counts sign-ins; stream and transcode limits need plugins.
- **Assessment: Portico ahead.** Note the tension with "no refusal because of server load": a conversion cap is an owner-set refusal, which is consistent with the rule as written.

### A-02.32 Transcoding permission is global, not per member
- **Portico:** one owner switch (`playback_owner_policy.transcoding_enabled`); when off, **all** HLS is refused, including remux (`v1_session_helpers.go:28-30`).
- **Jellyfin:** per user: video transcoding, audio transcoding, remuxing, force-transcode remote sources.
- **Assessment: Portico behind** and at odds with spec §5.3 (`allowTranscode` per account/profile). Also, "transcoding off" blocking copy-only remux is harsher than the name implies; `RemuxEnabled` is a separate owner setting that then has no effect. See F-02.9.

### E. Progress, resume, watched

### A-02.33 Thresholds
| Rule | Portico | Jellyfin |
|---|---|---|
| Resume kept after | min(30 s, 5 % of duration); 1 s for audiobook parts | 5 % of duration |
| Played at | 90 % (library default, movies/episodes) or viewer's 95 %; or `ended`; or first approved credits marker | 90 %, or within 1 s of end |
| Short items | no special rule | under 300 s: played, no resume |
| Audiobooks | book-relative position, own resume table | minutes from start/end |
| Unknown duration | never completes by threshold | marked played |
| Settings scope | per library (owner) and per viewer | server-wide |

- **Why it matters:** Portico's 30 s cap means a two-hour film enters Continue Watching after half a minute; Jellyfin needs six minutes. Portico's credits-marker completion is a real improvement for shows with long credits. A 40-second clip in Portico gets a resume point from 2 s (5 %).
- **Assessment: Trade-off (subjective).** Portico is more configurable. Consider a minimum-duration rule (F-02.12).

### A-02.34 Re-watching clears "watched"
- **Portico:** any started-but-incomplete report writes watched = false (`S/catalog/personal_activity.go:117-119`).
- **Jellyfin:** progress never clears `Played`. **Plex** (from memory): a re-watch shows progress but keeps the view count.
- **Why it matters:** re-watching episode 3 of a finished season to the halfway point turns it unwatched, which can change "next episode" and unwatched counts.
- **Assessment: Unclear whether deliberate.** Listed as B-02.8.

### A-02.35 Two histories and a privacy switch
- **Portico:** the owner's play history records every play; the viewer's own history, watched state and resume stop while `privacy.pauseWatchHistory` is on (`personal_activity.go:76-84`).
- **Jellyfin:** one user-data record; no pause.
- **Assessment: Portico ahead.**

### A-02.36 Cross-device ordering of progress writers
- **Portico:** claims and fences give each accepted play an ordinal; an older session or an automatic next-episode cannot overwrite a newer explicit play or a manual mark (`S/playback/personal_progress.go`).
- **Jellyfin:** last write wins.
- **Assessment: Portico ahead.** It is intricate; a short written explanation of the rules would help future work.

### A-02.37 When a play is counted
- **Portico:** a play-history row is created once the play passes the started threshold; the **play count** rises only the first time that play becomes completed (`S/catalog/play_history.go:26-48`, called from `personal_activity.go:76`).
- **Jellyfin:** play count rises at start, and again at stop if no position was reported (`SessionManager.cs:848`, `:1196`).
- **Why it matters:** Portico's counts mean "finished"; Jellyfin's mean "started". Portico never counts an accidental start; it also never counts a film abandoned at 80 %.
- **Assessment: Portico ahead** for most uses (recommendations, "most played").

### F. Selection defaults

### A-02.38 Default audio track
- **Portico:** explicit → preferred languages in order → default flag → first non-commentary. No memory of the last choice; no "original language" option.
- **Jellyfin:** remembered per item, user language or the item's original language, optional "prefer the default track".
- **Assessment: Portico behind** on memory and original language; ahead on skipping commentary tracks. Spec §5.4 promises "then last-used"; the code has none. See F-02.6, F-02.7.

### A-02.39 Default subtitles
- **Portico:** `off`, `forced`, `always`, plus an implicit rule: if the audio language is not among the viewer's preferred audio languages, pick a subtitle in a preferred subtitle language. Candidates are **only stored text tracks** (`autoselect.go:134`); a forced track must match the audio language exactly; forced tracks with undefined language are never chosen.
- **Jellyfin:** five modes including `Default` (follow the file's default/forced flags) over all subtitle types; forced tracks of undefined language accepted.
- **Why it matters:** a foreign film whose only subtitles are PGS gets none automatically in Portico. A forced track tagged `und` (common) is skipped.
- **Assessment: Portico behind** on coverage, deliberate on cost ("never extracts, converts or burns anything in", `autoselect.go:9-15`). See F-02.8.

### A-02.40 Subtitle delivery is decided by the track, not the device
- **Portico:** image and styled tracks burn in unless the client says `embeddedClient`; the device's declared subtitle lists are stored and never read by decision code (`S/playbackv1/capabilities.go:301-314` writes them; nothing reads `Subtitles.Text/Styled/Bitmap`).
- **Jellyfin:** `SubtitleProfiles` per format and method drive Embed / External / Hls / Encode / Drop.
- **Assessment: Portico behind**, and a doc/code disagreement with spec §5.5. Detail belongs to auditor 04; noted here because burn-in forces route 4.

### A-02.41 Which version plays by default
- **Portico:** the first file in `part_index, token` order (`S/playback/v1.go:93`), effectively arbitrary. `preferred.reason` is always `"default"`.
- **Jellyfin:** sorted by direct-playability and bitrate fit, keeping the requested one first. **Plex** (docs): automatic best version.
- **Assessment: Portico behind.** Today the backlog notes that same-title files are not yet merged into versions, so this is latent; it becomes visible the day they are. See F-02.1, B-02.11.

### G. Sources and extras

### A-02.42 Remote URL sources
- **Portico:** owner-approved roots only; MP4/MOV with H.264 and AAC/MP3 only; always direct; cannot be converted.
- **Jellyfin:** any URL, probed, direct or transcoded, with a per-user force-transcode switch.
- **Assessment: Trade-off (subjective).** Portico's is far safer (no server-side request to arbitrary hosts) and far narrower.

### A-02.43 Disc folders, ISO, 3D
- **Portico:** none; ISO is flagged unsupported.
- **Jellyfin:** BDMV and VIDEO_TS folders and ISO play (always via remux or transcode); a 3D format field exists.
- **Assessment: Portico behind**, likely deliberate scope. See F-02.13.

### A-02.44 Pre-rolls
- **Portico:** none. **Jellyfin:** intros API and client "cinema mode". **Plex** (docs): pre-roll video setting.
- **Assessment: Portico behind**, low priority. See F-02.15.

### A-02.45 Next episode
- **Portico:** the server-owned queue carries `next` and `postPlay {autoplay, countdownSeconds, passoutCheckDue}` from viewer preferences (spec §18.5; `S/operations/preferences.go:166-169`).
- **Jellyfin:** clients compute Next Up and show their own prompts; Android TV has a "still watching" setting.
- **Assessment: Portico ahead** (same behaviour on every client).

### A-02.46 Live TV and VOD share the session resource
- **Portico:** a channel is a v1 session with a `linear` clock; the timeline is the channel's lease (spec §18.6).
- **Jellyfin:** separate live-stream open/close calls around the normal reporting.
- **Assessment: Portico ahead** in uniformity. Not reviewed in depth here.

### A-02.47 Multi-part titles
- **Portico:** versions have parts; a session plays one part and carries `partIndex`; chapters and markers are offset onto one timeline in the options read (`S/playbackv1/options.go:550-584`).
- **Jellyfin:** additional parts are separate playable items queued after the first.
- **Assessment: Portico ahead** on paper. I did not verify that clients move from part to part seamlessly or that resume across parts is stored (`resume.partIndex` is always 0 in `options.go:525-527`).

### A-02.48 Stream facts are refreshed at first play
- **Portico:** a file probed before detailed facts existed is re-probed at start, bounded, at most two at a time (`S/playback/detail_refresh.go:32-60`); otherwise the plan notes `source_detail_unobserved` and lets unknown facts pass.
- **Jellyfin:** probes at scan; a missing probe means unknown streams and a transcode reason.
- **Assessment: Portico ahead**, with the small consequence that a preview made before the first play can differ from the plan made at start.

## Missing features and capabilities to consider

Sizes are rough: small (days), medium (a week or two), large (more).

### F-02.1 Choose the best version automatically
- **What:** when a title has several files, default to the one this device can direct play, then the one that fits the quality request, then the highest quality; keep an explicit choice sticky.
- **Who has it:** Jellyfin (`MediaInfoHelper.cs:367-412`, `StreamBuilder.cs:262-305`); Plex (docs).
- **Value:** owners keep a 4K remux and a 1080p encode precisely so weak devices and remote viewers get the small one without thinking.
- **Size:** small to medium. Run `planDelivery` per version in the options read and rank by route cost, then bitrate fit. Depends on the backlog item that merges same-title files into versions.
- **Principles:** fits "the server composes". **Opinion:** do it together with the version merge; otherwise the first merged library will play arbitrary versions.

### F-02.2 HEVC (and later AV1) conversion output, keeping HDR
- **What:** convert to HEVC Main10 with HDR10/HLG kept when the device and display take it; H.264 SDR stays the fallback.
- **Who has it:** Jellyfin (client transcoding profiles list codecs; server keeps HDR on HEVC), Plex (docs: HEVC transcoding option).
- **Value:** remote viewers under a bitrate cap keep HDR and get roughly twice the quality per bit.
- **Size:** large (touches auditor 03's pipeline). **Opinion:** the biggest quality gap versus Jellyfin for remote 4K viewing.

### F-02.3 fMP4 as a conversion container
- **What:** let route 4 use fragmented MP4 when the device has no MPEG-TS transport, and for any future HEVC output.
- **Who has it:** Jellyfin. **Size:** medium. **Opinion:** needed by F-02.2 and by B-02.14.

### F-02.4 A bandwidth measurement and a working "Automatic"
- **What:** a small server endpoint that returns N bytes for timing (or reuse of the first media segments), and wiring `CC/playback-v1/quality.ts` into the players so remote starts ask for a sensible limit and re-ask (`PATCH`) when the estimate moves.
- **Who has it:** Jellyfin (`bitrateTest.ts`), Plex.
- **Value:** remote playback that starts without buffering on slow links, with no owner setup.
- **Size:** medium (the decision helper exists; the estimate source, the start wiring and the mid-play `PATCH` do not). **Principles:** consistent with "quality changes only when a person asks" only if Automatic is the person's standing request, which is how spec §5.2 frames it. **Opinion:** high value; depends on fixing B-02.6 first.

### F-02.5 Show the precise reason on the session
- **What:** expose the trace's rejection codes (`video_level_exceeds_device`, `interlaced_unsupported`, `audio_channels_exceed_device`, `dolby_vision_profile_unsupported`, …) in `presentation.decision.*.reasons` and in Now Playing.
- **Who has it:** Jellyfin's 28 transcode reasons. **Size:** small. **Opinion:** cheap, and it makes every future support question shorter.

### F-02.6 Remember the last audio and subtitle choice
- **What:** per item (and ideally per show: "I chose Japanese audio with English subtitles for this series").
- **Who has it:** Jellyfin per item (`SessionManager.cs:1009-1037`); Plex per show (docs).
- **Size:** small to medium. The timeline already carries `audioTrackId` and `subtitleTrackId`. **Opinion:** spec §5.4 already promises "then last-used"; anime viewers will ask on day one.

### F-02.7 "Original language" as an audio preference
- **Who has it:** Jellyfin (`MediaSourceManager.cs:545-578`). **Size:** small if the catalogue stores original language. **Opinion:** worth having beside the ordered language list.

### F-02.8 Wider automatic subtitle rules
- **What:** (a) a mode that follows the file's own default/forced flags; (b) consider image tracks when no text track matches (accepting the burn-in or client rendering that follows); (c) accept forced tracks with undefined language.
- **Who has it:** Jellyfin (`MediaStreamSelector.cs`). **Size:** small for (a) and (c), medium for (b). **Conflict:** (b) can start a conversion the viewer did not ask for; make it a preference.

### F-02.9 Per-member conversion permissions
- **What:** allow video conversion, allow audio conversion, allow remux, force conversion of remote sources, each per member.
- **Who has it:** Jellyfin user policy. **Size:** small to medium (limits document, admission, planner input). **Opinion:** spec §5.3 lists `allowTranscode`; owners with weak hardware want "family may convert, friends may not".

### F-02.10 Swap to a sibling audio track to stay on the cheaper route
- See A-02.14. **Size:** small. **Opinion:** do it; announce it in the decision ("played the AC-3 track of the same language").

### F-02.11 Rewind a little on resume
- **What:** start a few seconds before the stored position (fixed, or scaled by how long ago playback stopped).
- **Who has it:** jellyfin-androidtv (`UserPreferences.kt:78` `resumeSubtractDuration`), Swiftfin (resume offset setting), audiobook players generally.
- **Size:** small; can be server-side in `startPositionMs`. **Opinion:** especially valuable for audiobooks.

### F-02.12 A short-item rule and per-kind resume rules
- **What:** items under N minutes are marked played rather than given resume points; extras and trailers never enter Continue Watching.
- **Who has it:** Jellyfin (`MinResumeDurationSeconds`). **Size:** small. **Opinion:** check how trailers and extras behave today before deciding.

### F-02.13 Disc folders, ISO and 3D
- **Who has it:** Jellyfin, Plex (folders), Kodi. **Size:** large for folders (title selection, multi-file timelines). **Opinion:** low priority; say plainly in the product that discs must be remuxed.

### F-02.14 Per-device overrides and a known-defect table
- **What:** user switches such as "this TV's receiver takes DTS", "limit to stereo", "never send Dolby Vision"; and a model-keyed list of known lies (Fire TV sticks with broken DV+HDR10+, Shield's unreported DV profile 7).
- **Who has it:** jellyfin-web settings (`appSettings.js:177-195`), jellyfin-androidtv `KnownDefects.kt`, Swiftfin custom profiles.
- **Value:** the escape hatch when probing is wrong. Portico's route-failure memory helps per file; this helps per device.
- **Size:** medium. **Opinion:** a server-side, updatable defect table fits Portico's "server decides" better than client switches; a handful of viewer overrides are still worth it.

### F-02.15 Pre-rolls
- **Who has it:** Jellyfin (intros API, `playbackmanager.js:265-272`), Plex. **Size:** small with the queue model (prepend entries). **Opinion:** low priority; some owners love it.

### F-02.16 Wider remote sources
- **What:** remote HLS and non-MP4 files, optionally converted. **Who has it:** Jellyfin, Plex (limited). **Conflict:** server-side fetches to arbitrary hosts; Portico's allowlist exists for that reason. **Opinion:** keep the allowlist; consider HLS pass-through for IPTV-style VOD only if asked.

### F-02.17 A few more capability fields
- **What:** allowed codec tags (`hvc1` only), max ref frames, anamorphic support, audio profiles (HE-AAC, DTS-HD vs core), max channels per codec **per transport**, max total streams.
- **Who has it:** Jellyfin conditions; each has a real device behind it (`browserDeviceProfile.js:976`, `:994`, `:1065`, `:1107`, `:1464-1470`).
- **Size:** small each. **Opinion:** add codec tags and per-transport channel limits first.

### F-02.18 Better multichannel conversion targets
- **What:** 7.1 output, and Dolby Digital (Plus) output for devices that decode AC-3/E-AC-3 but not multichannel AAC, without requiring a passthrough route.
- **Who has it:** Jellyfin (transcoding profile audio codec order and `MaxAudioChannels`).
- **Size:** medium. **Opinion:** spec §5.4 already describes "E-AC-3 5.1 (7.1 when the route supports it)" for Apple TV; the code only reaches E-AC-3 through a passthrough route (`delivery_decision.go:362-369`).

### F-02.19 A device profile report
- **What:** a screen (client) and an owner view (server) showing what the device declared, what was probed, and the revision the server holds.
- **Who has it:** jellyfin-androidtv (`deviceProfileReport.kt`). Portico has `GET /v1/playback/client-profile` for the caller's own sign-in.
- **Size:** small. **Opinion:** pairs well with A-02.5.

### F-02.20 Act on client error reports
- **What:** the timeline's `error` field recorded and answered (an event offering a fallback, the session shown as failed in Now Playing).
- **Who has it:** Jellyfin shows a failed stop (`PlaybackStopInfo.Failed`). **Size:** small. See B-02.13.

### F-02.21 Do not count progress before the first frame
- **What:** Plezy withholds progress reports until a frame has rendered, so a native clock cannot create progress for a stream that never showed (`playback_progress_tracker.dart:79-81`).
- **Portico today:** reports in `preparing` are ignored server-side; after that the client is trusted. **Size:** small, client-side. **Opinion:** worth a check on the native players.

### F-02.22 A way to clear "this route failed" memory
- **What:** owner or viewer action, or automatic clearing when the device republishes a changed profile or the app version changes.
- **Size:** small. See B-02.16.

### F-02.23 Offline progress upload and session transfer
- Spec §13 names `POST /v1/playback/offline-reports` and §9.2 names transfers started with `{transferId}`. I found no such route, and `transferId` has no start handler (`S/playbackv1/sessions.go:555-569`; only `channelId` and `queue` are registered). The native app keeps an offline journal (`N/bridge/useNativePlayback.ts:40-41`); I did not trace how it syncs. Both belong to other auditors; listed so they are not lost.

## Possible bugs and risks in Portico

None of these is in `Backlog.md`. All are from reading code; none was reproduced.

### B-02.1 Native devices are told HEVC may travel in MPEG-TS HLS
- **What goes wrong:** for any device that publishes a v1 capability profile, the server builds the MPEG-TS transport's video list as "every declared codec that is h264, mpeg2 or hevc" (`S/playbackv1/capabilities.go:289-297`). The Apple declared table deliberately lists **H.264 only** for MPEG-TS (`CC/client-profile.ts:134`) and puts HEVC under fMP4. Because the translated copy wins (`S/playback/client_profile.go:466`) and MPEG-TS is tried first (`delivery_decision.go:308-321`), an HEVC file that needs repackaging (any MKV on Apple) plans as HEVC copied into MPEG-TS.
- **Expected effect on device:** Apple's HLS rules require fMP4 for HEVC, and Jellyfin's web profile records "safari doesn't support hevc in TS-HLS" (`jellyfin-web/src/scripts/browserDeviceProfile.js:379`). If AVPlayer refuses it, the client reports a route failure, the next plan (audio conversion, video still copied into MPEG-TS) fails the same way, and the third plan converts to H.264 SDR. Even if a given OS version plays it, HDR10 and Dolby Vision signalling in MPEG-TS is not something to rely on.
- **How the comparison surfaced it:** Jellyfin's explicit Safari special case.
- **Confidence:** the planning path is **confirmed by reading**; the device failure is **probable** and needs one run on an Apple TV with an HEVC MKV (check `presentation.container`: it should say `fmp4_hls`).
- **Severity:** High (Apple is the reference client; HEVC MKV is the most common 4K format).

### B-02.2 Passthrough codecs are always rejected for v1-publishing devices
- **What goes wrong:** the planner refuses a passthrough codec unless `audioOutput.route` is `hdmi`, `arc` or `optical` (`delivery_decision.go:215-217`). The v1 document has no route field, and the translation sets `AudioOutput` to `{MaxChannels: <largest codec channel count>}` with no route (`S/playbackv1/capabilities.go:277`). The client-side builder also drops the route (`CC/playback-v1/capability-profile.ts:49-55`). So AC-3, E-AC-3, DTS or TrueHD that an Android TV or Fire TV reports as receiver passthrough (`CC/client-profile.ts:499-503`) is rejected with "passthrough output unavailable" and converted to AAC.
- **Second effect:** `AudioOutput.MaxChannels` becomes the largest decoder channel count rather than the real output (the declared tables say 2), so a stereo device can be sent 5.1 AAC (`delivery_decision.go:353-361`). Harmless on most players, wasteful.
- **Confidence:** Confirmed by reading. **Severity:** Medium to High for home-theatre users on Android TV / Fire TV (surround and Atmos lost, needless conversions). Apple is unaffected (no passthrough entries in its table).

### B-02.3 "Can switch audio tracks inside a file" is lost for native devices
- **What goes wrong:** the translation reads `features.embeddedAudioSwitching` (`capabilities.go:220`); the native builders send `features: {trickplay, chapters, gapless, crossfade, pip}` only (`N/bridge/audio-decode.ts:31`, `:70`), although both declared tables set `embeddedAudioSwitching: true` (`CC/client-profile.ts:139`, `:164`). The planner then closes direct play whenever the wanted track is not the first (`delivery_plan.go:366-368`).
- **Effect:** on Android (which plays MKV directly) every multi-audio file whose chosen track is second or later is remuxed to HLS instead of played directly; on Apple the same for MP4/MOV files. Anime with Japanese first and a preferred English track second is the typical case.
- **Confidence:** Confirmed by reading. **Severity:** Medium (server work and loss of in-file extras, not a failure).

### B-02.4 "The device tone-maps HDR itself" is never sent
- **What goes wrong:** the Apple profile lists HDR10 and HLG for HEVC even when the screen is SDR, because AVPlayer tone-maps well (the COMPAT-02 comment, `CC/client-profile.ts:452-456`). The v1 translation keeps a range only if the display lists it or the codec has `toneMapsToSdr` (`capabilities.go:250-256`). `capabilityProfileFromClient` never emits `toneMapsToSdr` (`CC/playback-v1/capability-profile.ts:38-48`).
- **Effect:** an Apple TV on an SDR television (or with HDR output off), and Apple devices with SDR screens, get HDR titles converted to 8-bit H.264 with server tone mapping, undoing COMPAT-02.
- **Confidence:** Confirmed by reading. **Severity:** Medium.

### B-02.5 Other facts lost or bent in the v1 translation
- **Direct transport is flattened** to "all direct containers × all video codecs × all audio codecs" (`capabilities.go:278-288`), so container-specific limits in the declared tables (Cast: VP9 only in WebM; web: Opus/Vorbis only in WebM) disappear for any device that publishes v1.
- **Interlaced support is dropped** (no v1 field), so every interlaced source is converted even on devices that declared it.
- **MPEG-2 naming:** the MPEG-TS filter tests for `"mpeg2"` (`capabilities.go:292`) while the planner's and the declared tables' name is `mpeg2video` (`delivery_decision.go:299`, `CC/client-profile.ts:193`); a device declaring `mpeg2video` never gets it on MPEG-TS, and one declaring `mpeg2` never matches a source.
- **Bitmap subtitles:** the client marks bitmap formats `render: 'native'` (`capability-profile.ts:59`); the server keeps bitmap formats only when `render == "client"` (`capabilities.go:311`). Currently without effect, because nothing reads the subtitle lists (A-02.40).
- **Confidence:** Confirmed by reading. **Severity:** Low individually; together with B-02.1 to B-02.4 they say the translation should not be the source of truth.

### B-02.6 The player's Quality menu, version choice and audio re-plan never reach the server
- **What goes wrong:** every play goes through `PlaybackService.play(itemId, position, prepared, quality, audioStream)` → `queue.playItem(…)` (`CC/index.ts:1092-1095`). The only implementation, `V1QueuePlayer.playItem`, ignores `_prepared`, `_quality` and `_audioStream` (`CC/playback-v1/v1-queue-player.ts:275-277`) and starts the queue with `{startPlayback: {state: 'playing'}}` (`:332`). Nothing else calls `setQuality`, `setVersion` or `setAudio` on a session (they exist at `CC/playback-v1/sessions.ts:327-329`).
- **Effects, by reading:**
  - Web: choosing "720p" calls `engine.play(…, id)` (`W/player/Player.tsx:347-352`), which restarts the same stream at the same quality while the menu shows the new label.
  - Choosing a prepared version (`chooseVersion`, `Player.tsx:358-364`) likewise restarts the original.
  - `selectAudioTrack` (`CC/index.ts:858-864`), used when a stream carries one audio track at a time, restarts with the default track; `installAudioTracks` then marks the asked-for track as selected (`:851-856`), so the menu can show a track that is not the one playing.
  - Failure escalation (`engineFailed`) is unaffected, since it needs no parameters.
  - Without a queue, `createPlayback` posts the legacy body with `quality: "auto"` fixed (`CC/index.ts:456-461`).
- **How the comparison surfaced it:** looking for Portico's equivalent of Jellyfin's bitrate selection.
- **Confidence:** the dropped parameters are **confirmed by reading**; the visible symptoms are **probable** (needs one click in the web player while watching the network tab). No test covers it.
- **Severity:** High (user-visible controls that do nothing; also means no client can lower quality on a slow link).

### B-02.7 A start without an `Idempotency-Key` takes the legacy path and skips caps
- **What goes wrong:** `POST /v1/playback/sessions` is routed to the v1 handler only when the header is present (`S/httpapi/playback_v1.go:332-349`). Otherwise the deprecated handler runs (`S/httpapi/playback_sessions.go:13-51`). It runs admission but **discards the member's remote bitrate cap** (`:37`, the `_`), plans with no v1 choice, and accepts `quality: "original"`, whose target zeroes every ceiling including the owner's (`S/playback/quality_ladder.go:176-177`).
- **Further effects:** the session has no v1 row, so it has no 120 s lease and no paused timeout, is not listed in Now Playing (`S/playbackv1/admin.go:60`) and cannot be terminated there; its row expires after 12 hours (`S/playback/service.go:409`) and counts against stream limits until then unless stopped.
- **Confidence:** Confirmed by reading. **Severity:** Medium to High (any signed-in member can bypass a remote bitrate cap and owner ceilings by omitting a header; the owner cannot see it happening).
- **Suggestion:** remove the legacy start, or make it apply the same choice and create a v1 row.

### B-02.8 Re-watching a watched title marks it unwatched
- **What goes wrong:** once a re-watch passes the started threshold (30 s at most), `AcceptPlaybackProgress` writes watched = false (`S/catalog/personal_activity.go:117-124`). It becomes watched again only on completion.
- **Effect:** half-re-watching an old episode un-watches it; depending on how "next episode" is computed, the show's next-up can jump back. Jellyfin never clears played from progress (`UserDataManager.cs:465-490`).
- **Confidence:** Confirmed by reading; whether it is intended is unknown (the comment at `:121` shows the opposite care was taken for brief replays). **Severity:** Medium.

### B-02.9 Two "remote" classifiers disagree
- **What goes wrong:** `V1Choice.remote` is true if `Caller.Remote` (configurable LAN networks, hairpin handling) **or** the simple network class says remote or cellular (`S/playback/v1.go:74-76`). The simple one (`delivery_policy.go:57-80`) knows no owner configuration. An owner who lists a VPN range (Tailscale's 100.64.0.0/10 is not a private range to Go's `IsPrivate`) or relies on "treat my public address as LAN" gets `Caller.Remote = false` (no member cap) but the server-wide remote limit still applies and the viewer is put on the "unknown" lane.
- **Also:** behind a trusted proxy the simple classifier reads the **left-most** forwarded address (`:60-63`), which the client controls; the configurable one reads from the right. This cannot lift a cap (the OR keeps it), but it can select the "local" preference lane.
- **Confidence:** Confirmed by reading. **Severity:** Medium for VPN users (LAN-quality expectations, remote-limit behaviour).

### B-02.10 On the cellular lane, HDR titles are converted even at "original" quality
- **What goes wrong:** `allowHDR` comes from the viewer's lane preferences, default **false** for cellular (`S/operations/preferences.go:135`). The v1 target copies it (`v1.go:59`), and a disallowed HDR picture closes routes 1 to 3 (`delivery_decision.go:194-195`). So `{mode: original}` on cellular yields 8-bit SDR H.264 for any HDR file.
- **Why it is listed:** the comment on `V1Choice` says the per-network presets are "not consulted" (`v1.go:13-16`) and spec §5.2 says only the request and admin ceilings decide. The delivery modes (`delivery.directPlay` etc.) are consulted as well.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (surprising, expensive, and invisible unless the reason is read).

### B-02.11 The default version is arbitrary
- **What goes wrong:** without a `versionId` the first file by `part_index, token` plays (`S/playback/v1.go:93`); tokens are not ordered by quality. `options.preferred.versionId` is likewise the first group (`S/playbackv1/options.go:586`, `:626`).
- **Confidence:** Confirmed by reading. **Severity:** Low today (the backlog says same-title files are not yet merged into versions), Medium once they are.

### B-02.12 Turning on a burned-in subtitle re-plans without the session's ceilings
- **What goes wrong:** `ChangeSubtitleDeliveryTx` rebuilds the plan from the stored `QualityID` through the legacy rung table (`S/playback/subtitle_delivery.go:39-44`). A v1 session's `QualityID` is `"original"` or `"limit"` (`v1.go:59-62`).
  - `"original"` resolves to the Original rung, whose target has no ceilings: the member's remote cap, the server-wide remote limit and the owner's height and audio ceilings are all absent from the burn-in conversion.
  - `"limit"` is not a rung, so `SelectedRung` fails with `ErrQualityUnavailable`, which the v1 layer reports as an invalid `subtitles.trackId` (the error branch after `Subtitles.Select`, `S/playbackv1/subtitles.go:67-72`).
- **Confidence:** Confirmed by reading. **Severity:** Medium (cap bypass on burn-in today; burn-in broken under a quality limit as soon as limits are wired).

### B-02.13 The timeline's `error` is accepted and ignored
- **What goes wrong:** `Report.Error` is parsed (`S/playbackv1/sessions_change.go:238`, `:250-253`) and never read. State `error` leaves the session's state unchanged (`:304-308`), so Now Playing shows it as playing until the client stops it or the lease lapses. Spec §6 says the server records it and may offer a fallback.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-02.14 A device without MPEG-TS HLS can never receive a conversion
- **What goes wrong:** route 4 requires an `hls_ts` transport listing H.264 (for video) and AAC (`delivery_plan.go:461-470`). The spec allows `streaming.hls: {fmp4: true, ts: false}`. Such a device gets `no_admissible_delivery_route` for anything it cannot direct play or copy.
- **Confidence:** Confirmed by reading. **Severity:** Low for first-party clients (all declare TS), Medium for third parties.

### B-02.15 `hev1`-tagged HEVC in MP4 is direct-played to AVPlayer
- **What goes wrong:** AVFoundation plays HEVC in MP4 only with the `hvc1` sample entry; Jellyfin encodes that as a codec-tag condition (`browserDeviceProfile.js:1464-1470`). Portico checks codec tags only for Dolby Vision entries (`delivery_decision.go:200-204`); no code mentions `hev1`.
- **Effect:** such a file fails once per device, the route-failure memory then repackages it. Self-healing, but the first play shows a recovery, and the memory lapses after 7 days.
- **Confidence:** Probable (device behaviour is well documented; not run). **Severity:** Low.

### B-02.16 Route-failure memory: risks of the design
- Any engine failure code, including the generic `engine_error` (the web sends it for fatal hls.js errors that are neither network nor media errors; media errors are retried locally twice first, `W/playback-v1/web-media-player.ts:160-172`), closes the route for that file for 7 days (`route_failures.go:23`, `:79-90`). A one-off glitch therefore costs a week of remuxing or converting that file on that device, with nothing in the UI to show or reset it.
- A failed **full conversion** is never recorded or escalated (`:82-86`), so the client retries the same thing (bounded by its three escalations).
- The memory is per sign-in family, so signing out and in forgets it, and two profiles on one device learn separately.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (design risk, not a defect).

### B-02.17 Two language tables have drifted apart
- **What goes wrong:** the audio chooser's ISO 639 table (`S/playback/delivery_decision.go:286-294`) has 58 three-letter codes; the subtitle chooser's copy (`S/subtitles/autoselect.go:57-62`) has 36. A viewer preferring Bulgarian, Croatian, Persian, Malay, Catalan, Tamil and others gets audio matched (`bul` = `bg`) but subtitles not.
- **Confidence:** Confirmed by reading. **Severity:** Low. One shared table (ideally a complete ISO 639-2 map) fixes it.

### B-02.18 A file of unknown bitrate passes every bitrate ceiling
- **What goes wrong:** `bitrateOver` is false when the actual rate is 0 (`delivery_plan.go:213`), and the rate is derived from size ÷ duration (`delivery_source.go:106-108`). A file with no recorded duration direct-plays under any cap. Jellyfin assumes 40 Mbps in that case.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-02.19 "Transcoding off" also blocks copy-only remux
- **What goes wrong:** with the owner switch off, every HLS session is refused (`S/playback/v1_session_helpers.go:28-30`), although a copy remux converts nothing and has its own owner switch (`RemuxEnabled`). The plan preview does not apply this check (`v1.go:144-146` only refuses plans that convert), so a title page can say "plays, repackaged" and the start then fails with `transcoding_disabled`.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (an owner who disables conversion on a weak box loses MKV playback on Apple devices and browsers entirely, and the preview disagrees with the start).

### B-02.20 `ended` marks a title watched at any position
- **What goes wrong:** `completed := state == "ended" || …` (`S/catalog/personal_activity.go:66`). A player that reports `ended` early (a truncated file, a stream that stops at a failed segment and fires the element's end event) marks the title watched and clears the resume point.
- **Confidence:** Speculative; depends on each engine never reporting `ended` short of the real end. **Severity:** Low to Medium. A guard such as "`ended` counts only past the started threshold or within N % of the duration" would be cheap.

### B-02.21 A re-plan resumes from the last reported position unless the client sends one
- **What goes wrong:** a `PATCH` that changes bytes starts the new presentation at `r.position`, the last timeline report (`S/playbackv1/sessions_change.go:108-113`), which can be up to 10 s old, unless the same `PATCH` carries `seek`.
- **Confidence:** Speculative (I did not trace whether clients include `seek` with track changes; today no client sends such a `PATCH` at all, see B-02.6). **Severity:** Low.

### B-02.22 Latent: the experimental "finite VOD" switch converts everything
- With `PORTICO_FINITE_VOD=1` every HLS plan is rewritten to a full H.264 conversion (`S/playback/service.go:384-391`, `v1.go:140-143`), and `vod.IntervalCount` returns 0 above 7,200 s (`S/playback/vod/encoder.go:35-37`), which reads as a two-hour limit. Off by default (`cmd/server/main.go:670-674`). Listed so nobody enables it casually. Detail belongs to auditor 03.
- **Confidence:** Confirmed by reading that the switch exists and what it rewrites; the 2-hour consequence is Probable. **Severity:** Low (off by default).

## Where Portico is stronger

- **One decision, explained.** A pure planning function with a stored trace of every route and every rejection per session; Jellyfin has flags and logs.
- **Fencing everywhere.** Idempotent start, `If-Match` on change, monotonic `seq`, generation numbers, and "404/410 means ended" make duplicate and late requests harmless. Jellyfin's reporting has none of this.
- **Fast, cheap cleanup.** A 120 s lease and an event-driven sweeper free slots and conversions quickly; nothing polls while idle.
- **Capability URLs** bound to one presentation and to the exact file (size and modified time), re-checked on every request, revoked on stop, restore and sign-out.
- **Stream and conversion limits** at three levels with a race-free admission gate; Jellyfin needs plugins.
- **Route-aware passthrough and output channels** in the planner profile, evidence-tagged capabilities, and probing on web, Apple and Android.
- **Dolby Vision fallback logic** keyed on the file's compatibility id, with an honest "colors approximate" flag for profile 5.
- **Server-side memory of failed routes**, so a wrong capability claim costs one retry instead of one per play.
- **Watched rules** that can use an approved credits marker, per-library thresholds, a viewer privacy switch, an owner history kept separately, and ordering of progress writers across devices.
- **Play counts that mean "finished"**, not "started".
- **Next-episode behaviour decided by the server** (autoplay, countdown, pass-out check) so every client behaves the same.
- **Commentary tracks are never chosen automatically**; Jellyfin has no such rule.
- **Typed refusals and a no-side-effect plan preview** (`playback-options` with `plan`) for labelling choices before play.
- **Source facts refreshed at first play** rather than failing on an unprobed file.
- **10 % bitrate tolerance**, which avoids converting files a hair over a round ceiling.

## Doc/code disagreements

| # | Doc says | Code does |
|---|---|---|
| D1 | Spec §3: one capability profile; "Extra fields… are ignored" | Two documents; the v1 one is translated lossily and preferred (`client_profile.go:466`); the v1 route's comment says unknown fields are refused (`capabilities.go:18-20`; decoder not tested here) |
| D2 | Spec §5.2 and `v1.go:13-16`: only the request and admin ceilings decide; per-network presets not consulted | Lane `allowHDR` and the three `delivery.*` modes are consulted (`v1.go:59`, `delivery_policy.go:208-227`) |
| D3 | Spec §5.2: client "Auto" picks a limit and may `PATCH` it | Helper exists (`CC/playback-v1/quality.ts`), no caller; no client sends a quality (B-02.6) |
| D4 | Spec §5.3: per account/profile `allowTranscode`, `allowDownloads`, `maxBitrateKbps` local and remote | Member limits have `maxStreams` and `remoteBitrateKbps` only (`S/access/limits.go:95-103`); conversion is one global switch; no local cap per member |
| D5 | Spec §5.4: audio default "profile languages, then last-used, then default flag" | No last-used (`delivery_decision.go:231-266`) |
| D6 | Spec §5.4: DTS/TrueHD to Apple TV → E-AC-3 5.1 (7.1 when supported) | AAC 2.0/5.1; E-AC-3 only via a passthrough route, never 7.1 (`delivery_decision.go:347-372`) |
| D7 | Spec §5.5: subtitles drawn natively when the profile says `render: native|client`, including PGS when `client` | Profile subtitle lists are never read; format decides; the client must ask for `embeddedClient` (`S/playbackv1/subtitles.go:35-79`) |
| D8 | Spec §5.6: profile 7/8.1 fall back to HDR10 "via copy"; `hdr` block in the presentation (`source`, `delivered`, `toneMapped`) | Fallback logic exists; the presentation has no `hdr` field (`sessions.go:146-163`); the output range is only in the trace |
| D9 | Spec §4.1: `preferred.reason` is `profile_language|last_used|default`; `preferred.subtitleId`; `resume.partIndex`, `updatedAt` | Reason always `default`; subtitle id never set; part index always 0; no `updatedAt` (`options.go:524-527`, `:586`) |
| D10 | Spec §4.1 lists `trickplay` in options | Not in the `Options` struct (`options.go:20-29`); may be served elsewhere (auditor 04) |
| D11 | Spec §6: `error` reports are recorded and may trigger a fallback event | Ignored (B-02.13); failures go through a separate `POST /v1/playback/route-failures`, which the spec does not describe |
| D12 | Spec §5.1: session `revision`, `kind`, … and decision reasons such as `bitrate_above_request`, `admin_cap_remote_bitrate` | Present; but all video-compatibility causes collapse to `video_codec_unsupported` (A-02.8) |
| D13 | Spec §9.2/§13: transfers by `transferId`; offline reports route | No start handler for `transferId`; no offline-reports route found (F-02.23) |
| D14 | `playback-admission.md`: after a failed report "retry the same report sequence and body… do not rewrite an attempted sequence" | The reporter sends a new, higher `seq` built from the latest observation on every attempt (`CC/playback-v1/timeline.ts:189-195`). Harmless to the server, but the doc and the first-party client disagree |
| D15 | `04 Playback and Media.md`: "offers, one per way to play… The client takes an offer" | v1 returns one presentation; the older offers read still exists (`S/playback/offers.go`) and the players' quality menus still read it |
| D16 | Spec §17 ("Removed"): the old `/v1/playback/sessions*` shapes are removed | The legacy start and read handlers are still mounted (B-02.7) and `createPlayback` still posts the old body (`CC/index.ts:456-461`) |
| D17 | Owner setting "planning policy" has three values | Two behave identically (A-02.11) |

## Open questions for Justin

1. **One capability document or two?** Should the planner profile shape become the v1 wire shape, or should v1 grow the missing fields (per-transport codec lists, output route and channels, embedded audio switching, interlaced, tone-maps-to-SDR)? Until then, should the server prefer the directly published planner profile?
2. **Should MPEG-TS stay the first-choice HLS container** for copy routes, or should fMP4 be preferred whenever the device declares it?
3. **Is "re-watching un-watches" intended** (B-02.8)? If not, should a re-watch keep watched and show progress, as Plex does?
4. **Is the 30-second "started" cap what you want** for long films, or should it be a percentage with a floor?
5. **Should `ended` be trusted at any position** (B-02.20)?
6. **Automatic quality:** build a bandwidth test and wire it, or leave quality fully manual for the first release? If manual, what should a remote start default to?
7. **Member caps:** should a remote bitrate cap mean total bitrate (video plus audio)? Do you want per-member "may convert" and a per-member local cap, as spec §5.3 says?
8. **Should "conversion off" still allow copy-only remux** (B-02.19)?
9. **Conversion output:** is H.264 SDR only acceptable for release, with HEVC/HDR output as a known later item?
10. **Version default:** when versions exist, what is the rule (first that direct plays, then best fit to the quality request, then highest)?
11. **Remember track choices:** per item, per show, or not at all?
12. **Subtitles by default:** should image-only subtitles be auto-selected for foreign-language audio even though that means burn-in on some devices?
13. **Legacy start route:** can it be removed now, given first-party clients use the queue path?
14. **Route-failure memory:** is 7 days right, and should the owner see and clear it?
15. **Cellular lane default `allowHDR = false`:** should an explicit "original" request override it?
16. **VPN users:** should the owner's LAN network list alone decide local versus remote for every purpose (B-02.9)?

## Evidence boundary

- Nothing was executed. No server, client, test, build or device was run. Every finding is from reading source in the working tree on 11 October 2026.
- **Device behaviour is inferred**, not observed: whether AVPlayer plays HEVC in MPEG-TS HLS on current tvOS/iOS (B-02.1), how it treats `hev1` (B-02.15), whether any engine reports `ended` early (B-02.20).
- **Client paths were traced partly.** B-02.6 rests on `playItem` being the only path from the players' menus to a session start or change; I found no other caller, but the web and native player code is large and I did not read all of it.
- **Which document a device is planned with** rests on the SQL join in `client_profile.go:466` matching rows the native app writes; I did not inspect a database.
- **Jellyfin** was read at the files and functions listed; `ConditionProcessor.cs` and the streaming helpers were not read line by line, and Jellyfin's behaviour for a request with no device profile was not checked.
- **Plex** statements are from memory of public documentation.
- **Performance** (planning cost, admission gate contention, sweeper behaviour under hundreds of sessions) was not assessed.
- **Live TV sessions, groups, transfers, device commands, music audio plans, the HLS producers and subtitle internals** were touched only where the decision depends on them.

## Outside my domain

- **Transfers and offline reports** named in the spec appear unimplemented (F-02.23); for the remote-control and downloads auditors.
- **`owns()`** lets any profile on the same device read and control a session by id (`S/playbackv1/sessions.go:338-340`, first clause); for the security auditor to judge on shared TVs.
- **Subtitle capability lists are never read** (A-02.40); for auditor 04.
- **Finite VOD's two-hour bound** (B-02.22); for auditor 03.
