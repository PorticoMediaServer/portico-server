# Portico playback protocol v1 — specification

Status: **normative**. It replaces contract 32 §3–§5 *mechanisms* (controllers, epochs, lanes, sequences, receipts, candidate grants, the dual-written legacy session) while keeping its product invariants P01–P06. It follows `Spec — API v1 Style Guide.md` for envelopes, errors, paging, `If-Match`, `Idempotency-Key` and events.

**Bar:** everything Plex and Jellyfin clients can do, for **every media type as a first-class citizen**: movies, TV episodes, anime (including absolute order), extras and trailers, music (tracks, albums, artists, playlists, music videos), audiobooks (single-file and multi-file, chapters), Live TV, DVR recordings (including while recording) and Library Channels. It must be implementable by a third party from this document plus the generated reference, with plain HLS/progressive players (AVPlayer, ExoPlayer, hls.js, Chromecast, VLC, mpv).

**Binding product rules:** quality is decided **only** by the client's request and admin/profile ceilings, never by server load (no automatic degradation). Content restrictions, profile feature switches and member limits apply to every session, queue and download (one admission function). US English messages via error codes.

---

## 1. Parity checklist (what this protocol must support)

| Capability | Plex | Jellyfin | Portico v1 |
|---|---|---|---|
| Device capability negotiation (codecs, containers, HDR, channels, subtitles, bitrate) | client profiles | DeviceProfile | §3 capability profile |
| Multiple versions/editions per title, multi-part (stacked) files | yes | yes | §4 versions → parts |
| Direct play (original file, byte ranges) | yes | yes | `delivery.mode = direct` |
| Direct stream / remux (container change, stream copy) | yes | yes | `stream` with per-stream `copy` |
| Transcode video and/or audio independently | yes | yes | per-stream `transcode` |
| User-selected quality (Original, bitrate/resolution caps), local vs remote defaults | yes | yes | §5.2 quality |
| Admin per-user caps (remote bitrate, transcode permission) | yes | yes | §5.3 ceilings |
| Audio track choice, language defaults, commentary, channel downmix, passthrough (AC-3/E-AC-3/TrueHD/DTS) | yes | yes | §5.4 |
| Subtitles: embedded text/image, external sidecar, forced, SDH, burn-in, styled ASS, offset | yes | yes | §5.5 |
| Online subtitle search and download | OpenSubtitles | plugins | §4.3 |
| HDR10/HDR10+/Dolby Vision/HLG passthrough and tone-mapping | yes | yes | §5.6 |
| Resume point, mark played at threshold, scrobble | yes | yes | §6 timeline |
| Chapters, intro/credits/recap/preview/commercial markers, skip | yes | yes (plugins) | §4.2, §6.3 |
| Trickplay/scrub thumbnails | BIF | trickplay | §4.4 |
| Play queues from any source, shuffle, repeat, play next, add to queue | play queues | playlists/queue | §8 queues |
| Remote control of another client (companion) | yes | session commands | §9 device commands |
| Transfer playback between devices | partial | yes | §9.2 transfer |
| Cast/AirPlay/Chromecast | yes | yes | §9.3 |
| Group watch sync | Watch Together | SyncPlay | §10 groups |
| Live TV (channels, timeshift, start over, record) | yes | yes | §11 |
| Music: gapless, crossfade, loudness normalisation, lyrics (synced) | Plexamp | yes | §12 |
| Audiobooks: chapters across files, speed, per-book resume, bookmarks | limited | yes | §12 |
| Offline downloads with sync | yes | yes | §13 |
| Now Playing admin dashboard with decisions, bandwidth, terminate | yes | yes | §14 |
| Playback speed, sleep timer | client | client | client-side; reported in timeline |

## 2. Model

- **Device**: an authenticated installation (web tab = device). Bearer tokens are device-bound. Each device publishes a **capability profile** (§3).
- **Item** → **versions** (a version is one media file set: e.g. 4K HDR remux, 1080p encode, director's cut edition) → **parts** (multi-part/stacked files or multi-file audiobooks; played seamlessly as one timeline) → **streams** (video, audio, subtitle, attachment).
- **Playback session**: one independent playback on one device of one item (or channel). It owns the lease, the grant family and the counted slot for stream caps. Kinds: `vod`, `audio`, `live`, `channel`. Roles: `local`, `receiver` (playing for another device's command), `group_member`.
- **Presentation**: the concrete bytes being delivered for a session, identified by `generation`. A change that alters bytes (track needing transcode, quality, version, burn-in) creates a new generation with new URLs. Seeking within a presentation doesn't.
- **Delivery decision**: per stream `direct | copy | transcode | burn | drop`, with machine reason codes (e.g. `video_codec_unsupported`, `bitrate_above_request`, `admin_cap_remote_bitrate`, `subtitle_image_needs_burn`, `container_unsupported`) and human messages from the client catalogue.
- **Queue** (§8), **group** (§10), **transfer** and **device command** (§9).

## 3. Device capability profile

`PUT /v1/me/devices/current/capabilities` (idempotent; sent at sign-in and whenever it changes: display HDR mode, audio route/receiver, network class). Body (all lists ordered by preference):

```json
{
  "form": "tv|phone|tablet|desktop|browser|cast|speaker",
  "network": {"class": "local|remote|cellular", "maxBitrateKbps": 0},
  "containers": [{"container": "mp4|mkv|webm|mpegts|mov|flac|mp3|ogg|m4a|m4b|wav", "direct": true}],
  "streaming": {"hls": {"fmp4": true, "ts": true, "maxSegmentMs": 6000}, "progressive": true},
  "video": [{"codec": "h264|hevc|av1|vp9|mpeg2|vc1", "profiles": ["main","main10"], "maxLevel": 51, "maxBitDepth": 10,
             "maxWidth": 3840, "maxHeight": 2160, "maxFps": 60, "maxBitrateKbps": 80000,
             "hdr": ["hdr10","hdr10plus","dolbyvision:5","dolbyvision:8.1","hlg"], "toneMapsToSdr": true}],
  "display": {"hdr": ["hdr10","hlg"], "maxWidth": 3840, "maxHeight": 2160},
  "audio": [{"codec": "aac|ac3|eac3|truehd|dts|dtshd|flac|alac|opus|mp3|pcm", "maxChannels": 8, "passthrough": false, "atmos": false, "maxSampleRate": 192000, "maxBitDepth": 24}],
  "audioDecode": [{"codec": "flac|alac|mp3|aac|opus|vorbis|pcm", "containers": ["flac","mp4","ogg","mp3","adts","wav","aiff"], "maxSampleRate": 192000, "sampleRates": [44100, 48000, 96000], "maxChannels": 8, "maxBitDepth": 24}],
  "subtitles": [{"format": "webvtt|srt|ass|ssa|pgs|vobsub|dvbsub|cea608|ttml", "render": "native|client|none"}],
  "features": {"trickplay": true, "chapters": true, "gapless": true, "crossfade": true, "pip": true}
}
```

`audioDecode` lists what the client's **own audio engine** decodes. A client that doesn't publish this profile yet (web) may send the same `audioDecode` list in its playback client profile (`PUT /v1/playback/client-profile`); the server uses it when the device profile has none. `sampleRates`, when present, is the exact set that decodes, and it overrides `maxSampleRate`. Extra fields (a client's own `via`, for example) are ignored (the §18.1 render path: Web Audio/WebCodecs, AVAudioFile/AudioConverter). It is not what the platform player plays: that is `audio` plus `containers`. The server stores it per device (`playback_client_profiles` rekeyed by device) and uses it for every decision. Unknown values are ignored (tolerant reader); the generated types enumerate known ones.

## 4. Reads

### 4.1 Playback options
`GET /v1/items/{itemId}/playback-options?versionId&partId` (no side effects) →

```json
{
  "itemId": "…", "kind": "movie|episode|track|audiobook|musicVideo|extra|recording",
  "resume": {"positionMs": 1234000, "partIndex": 0, "updatedAt": "…"},
  "versions": [{
    "id": "…", "label": "4K HDR · Remux", "edition": "Director's Cut",
    "container": "mkv", "sizeBytes": 0, "durationMs": 0, "bitrateKbps": 0,
    "parts": [{"id": "…", "index": 0, "durationMs": 0, "sizeBytes": 0}],
    "video": [{"id": "v0", "codec": "hevc", "profile": "main10", "level": 153, "bitDepth": 10, "width": 3840, "height": 2160, "fps": 23.976, "hdr": "dolbyvision:8.1", "bitrateKbps": 0}],
    "audio": [{"id": "a1", "codec": "truehd", "channels": 8, "layout": "7.1", "atmos": true, "language": "en", "title": "English", "default": true, "commentary": false, "visualImpaired": false}],
    "subtitles": [{"id": "s3", "format": "pgs", "source": "embedded|sidecar|downloaded", "language": "en", "title": "English SDH", "forced": false, "sdh": true, "default": false}]
  }],
  "chapters": [{"startMs": 0, "title": "Chapter 1", "thumbnail": "<artwork url>"}],
  "markers": [{"type": "intro|recap|credits|preview|commercial", "startMs": 0, "endMs": 0, "confidence": "detected|manual|embedded"}],
  "trickplay": {"url": "<versioned capability url>", "intervalMs": 10000, "tileWidth": 320, "tileHeight": 180, "columns": 10, "rows": 10},
  "preferred": {"versionId": "…", "audioId": "a1", "subtitleId": null, "reason": "profile_language|last_used|default"},
  "plan": {"versionId": "…", "mode": "direct|stream", "streams": [{"id": "v0", "action": "copy", "reasons": []}], "estimatedBitrateKbps": 0, "quality": {"mode": "original"}}
}
```

`plan` is what `POST /v1/playback/sessions` would choose for **this device** and the given query (`quality`, `audioId`, `subtitleId`, `versionId` query parameters may be supplied to preview alternatives). Clients use it to label choices ("Plays as original", "Converts audio"); the COMPAT findings' user-visible explanations come from `reasons`.

### 4.2 Markers and chapters
Included above. Clients show Skip Intro/Credits per the viewer's `playback.introSkip`/`creditsSkip`/`recapSkip` preference (`ask|auto|off`). A skip is an ordinary seek; the client reports `skipped: {markerId, mode, positionMs}` in the next timeline report (§6) so skip analytics and marker confidence work (fixes CD-16). `markerId` is the segment marker's id (`GET /v1/items/{id}/segment-markers`), `mode` is `automatic` (the viewer's `auto` preference, only for a marker the server marks safe for unattended skipping) or `manual`, and `positionMs` is where the skip left from. The server records it once per session and marker, and ignores an automatic skip of a marker it did not authorize (B8b).

### 4.3 Subtitle search and download
`GET /v1/items/{itemId}/subtitles:search?language=en&versionId` → candidates `{id, provider, language, title, format, hearingImpaired, score}`; `POST /v1/items/{itemId}/subtitles:download {candidateId}` → Operation → a new `downloaded` subtitle stream appears in playback options (event `item.updated`). Owner-configurable provider; members may download only if `subtitles.download` is allowed for their profile.

### 4.4 Trickplay
Versioned immutable tile sheets (JPEG/WebP), one descriptor per version. Generated as an analysis product with a budget. Absent → the field is omitted.

## 5. Sessions

### 5.1 Start
`POST /v1/playback/sessions` with `Idempotency-Key`:

```json
{
  "itemId": "…" | "channelId": "…" | "queue": {"queueId": "…", "entryId": "…"} | "transferId": "…",
  "versionId": "…", "partIndex": 0,
  "startPositionMs": 0 | "startFrom": "resume|beginning|live|startOver",
  "state": "playing|paused",
  "quality": {"mode": "original"} | {"mode": "limit", "maxVideoBitrateKbps": 8000, "maxHeight": 1080, "maxAudioBitrateKbps": 320},
  "audio": {"trackId": "a1", "channels": 2},
  "subtitles": {"trackId": "s3", "delivery": "auto|sidecar|embeddedClient|burn"} | {"trackId": null},
  "network": "local|remote|cellular",
  "replacesSessionId": "…"
}
```

→ `201` (or `202` while preparing, with `retryAfterMs`) `{session}`:

```json
{
  "id": "…", "revision": "…", "kind": "vod", "role": "local", "state": "playing",
  "itemId": "…", "versionId": "…", "queue": {"queueId": "…", "entryId": "…"} ,
  "lease": {"expiresAt": "…", "reportEveryMs": 10000},
  "presentation": {
    "generation": 3, "mode": "direct|stream",
    "url": "/v1/media/{grant}/{generation}/master.m3u8" ,
    "startPositionMs": 0,
    "subtitles": [{"trackId": "s1", "format": "webvtt", "url": "…/subs/s1.vtt"}],
    "decision": {"video": {"action": "copy", "reasons": []}, "audio": {"action": "transcode", "to": "eac3", "channels": 6, "reasons": ["audio_codec_unsupported"]}, "subtitles": {"action": "sidecar"}},
    "bitrateKbps": 0,
    "hdr": {"source": "dolbyvision:8.1", "delivered": "hdr10", "toneMapped": false}
  }
}
```

- `mode: direct` → `url` is the original file (single part) or a manifest stitching parts; supports `Range`. `mode: stream` → HLS (CMAF fMP4 preferred, MPEG-TS when the profile needs it) with a single rendition matching the requested quality.
- Admission (one function): library access, content restrictions, profile feature switches, member stream limits, owner stream/transcode caps (admin-set). Refusals are typed errors (`restricted_content`, `stream_limit_reached`, `transcode_not_allowed`, `source_unavailable`, `unsupported_media`). **No refusal and no quality change because of server load.**
- `replacesSessionId` (same device) atomically ends the old session and takes its slot (queue advance, version switch without a new slot).

### 5.2 Quality
Only the client's `quality` and admin/profile ceilings decide. `original` means direct play or direct stream when the profile allows; otherwise the minimum transcode needed for compatibility at source bitrate. `limit` sets ceilings. The client's "Auto" is a **client-side** choice: it picks a `limit` from its own bandwidth estimate and preference (e.g. per network class), and may change it later with `PATCH`. The server never picks a lower quality on its own.

### 5.3 Admin/profile ceilings
Per account and per profile: `maxBitrateKbps` for local and for remote, `allowTranscode`, `allowDownloads`, `maxConcurrentStreams`. Ceilings clamp the request and appear in `decision.reasons` (`admin_cap_remote_bitrate`), so users see why.

### 5.4 Audio
Track selection per request; default from profile preferred languages, then last-used, then default flag. Channel handling: passthrough when the device declares `passthrough` for that codec (AVR); otherwise decode/transcode. Defaults: DTS/TrueHD to Apple TV → E-AC-3 5.1 (7.1 when the route supports it); PCM multichannel when local and the profile allows. Stereo downmix only when the device's `maxChannels` is 2. Atmos is preserved when E-AC-3 JOC passthrough is available.

### 5.5 Subtitles
Delivery precedence: (1) native/client rendering of the original format when the profile says `render: native|client` (WebVTT/SRT/TTML; ASS/SSA when `client`; **PGS/VobSub when `client`**: the client renders image subtitles, per D-FEAT-3); (2) server-converted WebVTT sidecar for text formats (ASS → plain WebVTT by default, per D-MEDIA-7); (3) burn-in only when the format is image-based and the client can't render it, or the user asks for styled ASS without client support. Burn-in forces a video transcode and the `decision` says so; clients warn before switching to it. Offsets: `subtitleOffsetMs` is client-side for sidecars; for burn-in it's a `PATCH` field.

### 5.6 HDR
Deliver HDR as-is when the display supports it; when the display is SDR and the device declares `toneMapsToSdr` for that format, deliver as-is; otherwise tone-map on the server (video transcode). Dolby Vision profile 5 needs DV-capable devices or a transcode; profiles 7/8.1 fall back to their HDR10 base layer via `copy` when the device lacks DV.

### 5.7 Change
`PATCH /v1/playback/sessions/{id}` with `If-Match` — any subset of `{state, versionId, partIndex, audio, subtitles, quality, seek:{positionMs, id}, subtitleOffsetMs}` → `200 {session}`, with a new `presentation.generation` when bytes change (the client switches URLs at `startPositionMs`), else the same generation. A seek within a direct or already-produced range is a plain media request; `seek` in `PATCH` is only for transcoded presentations where the server must restart production.

### 5.8 Stop
`DELETE /v1/playback/sessions/{id}` with optional body `{positionMs}`. Idempotent. Fences media grants within 1 s. Lease expiry (120 s with no timeline or media request) ends a session automatically.

## 6. Timeline (progress, liveness, observations)
`POST /v1/playback/sessions/{id}/timeline` → `204` with `Report-Every-Ms`. Body:

```json
{"seq": 42, "generation": 3, "state": "playing|paused|buffering|ended|error", "positionMs": 0, "partIndex": 0,
 "rate": 1.0, "bufferedMs": 0, "bandwidthKbps": 0, "droppedFrames": 0, "volume": 1.0, "muted": false,
 "skipped": {"markerId": "…", "mode": "automatic|manual", "positionMs": 0}, "error": {"code": "decoder_error", "detail": "…"}}
```

- The single call is progress, lease renewal and observation. `seq` is monotonic per session; older reports are ignored; reports for an old `generation` never change state.
- Default cadence: 10 s playing, 30 s paused, immediately on state change and on seek completion.
- The server writes resume position and **played** state (threshold `playback.playedThresholdPercent`), respects `privacy.pauseWatchHistory`, feeds Continue Watching and recommendations incrementally, and ends the session on `ended`.
- `error` reports a client-side failure; the server records it (route failures) and may offer a fallback (e.g. a stream-mode presentation) via an event. It never lowers quality unasked.

## 7. Media URLs
Capability URLs `/v1/media/{grant}/{generation}/…`: unguessable, short-lived, revocable per session and generation, usable by plain players without headers. HLS playlists reference segments under the same prefix. Sidecar subtitles and trickplay are capability URLs too. Direct play supports `Range` and `HEAD`.

## 8. Queues
Queues are server-owned, windowed and revisioned:
- Routes: `POST /v1/queues`, `GET /v1/queues/{id}`, `GET /v1/queues/{id}/window`, `GET /v1/queues/{id}/entries`, `POST /v1/queues/{id}/segments`, `DELETE /v1/queues/{id}/entries/{entryId}`, `POST /v1/queues/{id}/entries/{entryId}:move`, `PATCH /v1/queues/{id}`, `POST /v1/queues/{id}:advance`, `POST /v1/queues/{id}:save-as-playlist`, `DELETE /v1/queues/{id}`.
- **Selectors** use the style guide's tagged union `{container:{kind,id}} | {query:{…browse body…}} | {items:{ids≤500}}`. Container kinds: `album, artist, disc, book, playlist, collection, savedView, show, season`, plus `mix` (recommendation-produced).
- **First release:** `query` selectors and the `savedView` and `mix` container kinds are **not in the first release**. The server answers `unsupported_selector` and clients use `container` or `items`. `:save-as-playlist` **is** in the first release, capped by the playlist size limit.
- Snapshot at Play; seeded Feistel shuffle; windows ≤ 200; no entry cap (a resource bound of 10 M keys per segment); specials policy per D-MEDIA-3; play-from-here anchors; Resume a show via `anchor: "resume"`.
- `startPlayback` in `POST /v1/queues` returns the first session, so **Play/Shuffle is one request**.
- Playlists lose the 1,000 cap and gain `:move`; group queues use the same model.

## 9. Remote control, transfer and casting

### 9.1 Device commands (companion parity)
`POST /v1/devices/{deviceId}/commands` `{type, …}` with `Idempotency-Key`, where `type ∈ play (item|queue|selector, startFrom) | pause | resume | stop | seek {positionMs} | next | previous | setTracks {audio?, subtitles?} | setQuality | setVolume {level} | setRate {rate} | playNext {selector} | addToQueue {selector}`. Allowed when the target is the caller's own device, or a device signed in to the same account/profile, or a receiver the caller paired with. Delivered on the target's event stream (`device.command`); the target acts and its timeline reflects the result. `GET /v1/me/devices?controllable=true` lists targets with `nowPlaying` summaries.

### 9.2 Transfer ("Play on" another device, handoff)
`POST /v1/playback/sessions/{id}/transfers {targetDeviceId}` → `{transferId}`. The target receives `playback.transfer_offered`, then starts with `POST /v1/playback/sessions {transferId}` (same queue, position, tracks, quality request re-evaluated for the target's profile). Committed when the target's first timeline reports `playing` (or `paused`); then the source ends. Failure before commit leaves the source untouched.

### 9.3 Cast, AirPlay and Portico TVs
- **Portico devices** (tvOS, and future Android TV/Fire TV/Roku apps, plus other signed-in Portico players): **discovered on the local network** (Bonjour/mDNS advertising only the device id, kind and a friendly name, and only while signed in). Control always goes **through the server** via §9.1/§9.2 (commands and transfers); there is no LAN control or media channel. Play on lists only these locally discovered devices plus Chromecasts (plus the system AirPlay picker); there's no code entry and no remote/cloud device list (Justin, 22 Sep). tvOS never shows casting UI.
- **Google Cast:** the receiver app is a Portico device with a receiver-scoped session obtained by pairing (bound to the sender that paired; re-pair on controller change, D-FEAT-10). It plays via §5 like any client and takes commands via §9.1 from its paired sender. Casting carries the queue.
- **AirPlay:** system-level; the Apple client keeps its session and reports timeline (external playback).

## 10. Groups (Watch Together / SyncPlay parity)
A group owns a queue (`owner_kind: group`) and a **group timeline** `{state, positionMs, rate, atServerTimeMs, revision}`. Members run their own sessions with `role: group_member` bound to the group. Commands `POST /v1/groups/{id}:play|pause|seek|next|setRate` are revision-fenced; the server broadcasts `group.timeline` events. Clients align using server time with RTT compensation (NTP-style, four timestamps; fixes BE-MEDIA-14), correcting drift by rate nudges (±5 %) under 500 ms and a seek above it. Readiness: members report `buffering` in timeline; the group waits (host-configurable) up to a timeout. Membership is re-checked on every event delivery. Restrictions: entries a member can't access are skipped for that member with a placeholder.

## 11. Live TV, DVR and Library Channels
- Live: `POST /v1/playback/sessions {channelId, startFrom: "live"|"startOver"}` → kind `live`, presentation HLS with a sliding window (timeshift buffer, default 60 min, admin-configurable). Viewers of one channel share one tuner stream. Pause live and rewind within the buffer; "start over" to the programme start when buffered. Tuner exhaustion → typed error `no_tuner_available` with the conflicting sessions and recordings listed for the owner.
- Recordings are VOD items; in-progress recordings are playable (growing presentation; `live: true` flag until finished).
- Library Channels: scheduled VOD. `POST /v1/playback/sessions {channelId}` resolves "what's on now" to an item and offset and returns an ordinary VOD session; channel surfing is `replacesSessionId`.
- DVR operations: `POST /v1/dvr/recordings {programme}`, series rules, conflicts and storage per the DVR contract; they aren't playback, but they are the missing write side that FEAT-02/WEB-LIVE-01 need, so the server implements them in `/v1` too.

## 12. Music and audiobooks
- Tracks: direct play of FLAC/ALAC/MP3/AAC/Opus when supported (bit-perfect when the route allows); transcode to AAC or Opus at the requested bitrate. Playback options include `gapless: {encoderDelaySamples, paddingSamples}`, `loudness: {trackGainDb, trackPeak, albumGainDb, albumPeak}` (EBU R128 measured by analysis) so the **client** engine does gapless, crossfade and normalisation (X-02). `GET /v1/items/{id}/lyrics` returns synced (LRC-style `{startMs, text}` lines) or plain lyrics.
- Music videos are video items linked to artists/tracks, first-class in artist pages and queues.
- Audiobooks: a book is one item with **parts** (files) and **chapters** (from tags, cue sheets or files); position is book-relative (`positionMs` across parts), so resume, speed (0.5–3.0, client-side, reported in `rate`), sleep timer (client), and bookmarks (`POST /v1/items/{id}/bookmarks {positionMs, note}`) work across files. Series order via container selectors.

## 13. Downloads and offline
`POST /v1/downloads {selector, quality: {mode, maxVideoBitrateKbps?, maxHeight?}, subtitles: [langs], audio: [langs]}` → an Operation producing download items (originals when allowed and the device supports them; otherwise converted renditions, generated without owner preparation when the server is idle, per D-MEDIA-8). Selectors let a device subscribe to a whole show/season/playlist ("keep next 5 unwatched"). Each download has a signed offline receipt (existing design, keeping APP-01's ownership model). Offline timeline reports queue on the device and upload later via `POST /v1/playback/offline-reports` (respecting pause-history, BE-MEDIA-11). No server-side copy of originals.

## 14. Administration (Now Playing)
- `GET /v1/admin/sessions` → active sessions `{user, profile, device, item, kind, state, positionMs, decision (direct/copy/transcode per stream with reasons), bitrateKbps, bandwidthKbps (from timeline), transcode: {speed, hardware, throttled}, location: local|remote, startedAt}`; paged; updated via `admin.sessions` events.
- `POST /v1/admin/sessions/{id}:terminate {message}` shows the message to the viewer (typed event) and ends the session.
- History: `GET /v1/admin/playback-history` (paged, filterable).

## 15. Events used
`/v1/events` topics (API style guide): `session.updated` (own device), `playback.transfer_offered`, `device.command`, `queue.updated`, `group.timeline`, `group.membership`, `item.updated`, `download.updated`, `admin.sessions`, `live.guide`, `operation.updated`. No playback feature polls.

## 16. Invariants (must be tests)
1. An idempotent start replay returns the same session; the same key with a different body → 422 `idempotency_key_reused`.
2. A `PATCH` with a stale `If-Match` → 412 with the current session; a stale change never overwrites newer intent.
3. A stale-generation or lower-`seq` timeline report is ignored.
4. `DELETE` fences media URLs within 1 s; lease expiry releases the slot.
5. Admission is identical for sessions, queues, downloads, groups and transfers (restricted content never plays by ID).
6. Server load never changes the decision; the same request plus the same profile always produces the same plan.
7. Shuffle is a bijection with a computable inverse for N ∈ [1, 2²⁰] (property test).
8. Windows never exceed 200 entries; no request or response size grows with queue length.
9. Transfer failure before commit leaves the source session playing.
10. Group commands are revision-fenced and membership is re-checked per delivery.

## 17. Removed
`/v2/playback/*`, `/v2/queues/*`, the old `/v1/playback/sessions*` shapes, controllers, epochs, lanes, sequences, receipts, candidate grants, cancellation tombstones, the client command journal, `playback_occurrences` and the dual-written legacy session, `/v1/handoffs` and the receiver inbox polling, the channel-intent/observation plane, and the listening-selection paging route (now a selector kind).

## 17. Clarifications
1. **Idempotency-Key reuse:** the same key with the same body replays the original response. The same key with a different body is `422 idempotency_key_reused`, the one API-wide code apikit uses (the style guide wins over the earlier 409).
2. **202 preparation:** the body is `{retryAfterMs}`, and `Retry-After` is also set. The client replays the identical request with the same key until 201/200.
3. **412 precondition:** the body carries `error.current` (the full current resource). `ETag` is the revision as a quoted strong ETag (`"12"`); clients compare the revision number, and `If-Match` accepts either form.
4. **Server-side seek** only happens when something is transcoded or burned: PATCH `{seek:{positionMs, id}}`. Direct and copy streams seek locally.
5. **Play/pause** are timeline facts (reports). A PATCH of `state` exists only for remote control.
6. **Timeline reports include `subtitleTrackId` and `audioTrackId`**, so local sidecar switches are recorded for resume and admin views.
7. **Timeline response:** 204 with `Report-Every-Ms`; 410 or 404 means the session ended.
8. **Queue commands** return `{queue, session?}`. Entry ids are opaque. Advancing past the end is `409 queue_ended`.
9. **Queue entries and window** use collection envelopes with `page.total`, `start` and `revision`.
10. **Queue create:** `{selector, startPlayback?, shuffle?:{seed:"<decimal string>"}, repeat?}`. When shuffled, the current entry stays first; entries added later form an unshuffled tail after the shuffled body.
11. **Events:** SSE `id:`/`event:`/`data:`. Long-poll is `GET /v1/events?after=&waitSeconds≤55`, returning `{events, nextAfter}`. `stream.resync` tells clients to refetch. `stream.closed` carries `reconnectAfterMs`. A second SSE stream for the same device gets `409 stream_exists`, and the client falls back to long-poll.
12. **`device.command`** payload: `{commandId, fromDeviceId, targetDeviceId, command, args}`. Sending returns `202 {commandId}`.
13. **Transfer:** offer creates `playback.transfer_offered`; the target starts with `{transferId}`; the transfer commits (and the source session ends) at the target's first `playing` timeline report.
14. **Admin sessions:** collection envelope. Terminate is `204` and takes an optional `{message}`. The `admin.sessions` event means "the list changed; refetch".
15. **Options preview query names** follow the client core.
16. **A lapsed lease** makes the client start a new session at the last reported position, with `replacesSessionId` set.
17. **Capabilities PUT** replaces the device's whole profile. The server keeps the revision, and a 412 applies as in item 3.

## 18. Queue transitions for rendered audio

Music and audiobooks keep v2's gapless, crossfade and normalization. The client renders the server's decoded audio (`audioRender`, §12), and the queue prepares the next entry's audio before the current one ends, then commits it at the audio boundary. "Next" belongs to the queue, so both are queue commands. A server offers them when `GET /v1/capabilities` has `features.queueTransitions: "enabled"`. Without that flag, clients advance with `:advance` (§8) and play ordinary, non-gapless audio.

### 18.1 `Presentation.audioRender` (version 2: the client decodes; "Plex and Plexamp are the standard to match")
The client decodes the audio itself (Plexamp's model). It direct-plays the original file with the server's exact gapless trim and gain, and the server converts only when the client can't decode the source. Converted audio is FLAC or Opus, never raw PCM. Version 1 (server-decoded `pcm_f32le` windows at 48 kHz) is withdrawn and retires with /v2.

A presentation of an `audio` session carries `audioRender` (absent for other kinds; the API carries no nulls):
```json
"audioRender": {"version": 2, "mode": "direct", "id": "<opaque media id>",
  "url": "/v1/media/{grant}/audio", "container": "flac", "codec": "flac", "decoderConfig": "<base64>",
  "sampleRate": 96000, "channels": 2, "bitDepth": 24, "bytes": 187654321, "prefetchBytes": 8388608,
  "durationFrames": 27095040,
  "trim": {"startFrames": 0, "endFrames": 0, "source": "flac"},
  "gain": {"trackDb": -6.2, "albumDb": -5.8, "trackPeak": 0.98, "albumPeak": 0.99, "source": "tags"}}
```
- **`mode`:**
  - `direct` — `url` serves the original file's bytes with `Range` (206; `Accept-Ranges: bytes`), exactly as stored. `bytes` is its size.
  - `converted` — the client can't decode the source, or the request's quality limits it. `url` streams FLAC (`container: "flac"`) or Ogg Opus (`container: "ogg"`, `codec: "opus"`). `reason` says why, for people ("This device can't decode ALAC, so Portico converts it to FLAC.").
  - `unavailable` — neither is possible (the owner forbids converting audio, and the client can't decode the source). `reason` says why. The client plays `presentation.url` ordinarily, without gapless, crossfade or normalization, and shows the reason quietly (X-02).
- **`id`** identifies this presentation's media (opaque). It is not the v1 session id or `presentation.generation`. A PATCH that changes bytes gives a new plan.
- **`durationFrames`** is the exact number of frames the track plays, at `sampleRate`, after trim. Gapless means the client outputs exactly these frames and nothing else.
- **`trim`** is relative to the codec's **raw decoded output**: every frame of every packet, before any container- or codec-signaled skip.
  - The track is raw frames `[startFrames, startFrames + durationFrames)`, and `endFrames` is what follows.
  - `source` names the evidence: `lame` (the LAME/Xing header), `itunsmpb`, `mp4-editlist`, `opus-preskip`, `flac` (none needed), `measured` (the server decoded the file, §18.7), or `none` (no delay: PCM, WAV, AIFF).
  - An engine whose platform decoder already applies part of the skip (for example, a packet table honored by Core Audio) must not trim twice. The conformance fixtures (§18.8) decide this per platform.
- **`gain`** (dB, ReplayGain 2.0 reference, −18 LUFS). Each field is absent when unknown.
  - `source` is `tags` (ReplayGain, or R128 converted: R128 gain/256 + 5 dB) or `analysis` (the server's EBU R128 measurement: gain = −18 − integrated loudness).
  - Album values come from tags, or from analysis once every track of the album is measured: the duration-weighted energy mean of the tracks' loudness, with peak = max.
  - Peaks are linear, true peak where measured. Tags win over analysis. This deliberately differs from v1/v2 for R128-only files, which took the R128 value against −23 LUFS without converting.
- **`decoderConfig`** (optional, base64) is the codec extradata as FFmpeg reports it: AAC's AudioSpecificConfig, Opus's OpusHead, FLAC's 34-byte STREAMINFO body (a WebCodecs FLAC description wants `fLaC` plus a STREAMINFO block header before it), and the ALAC magic cookie. It's for engines that feed a packet decoder (WebCodecs).
- **Choosing the mode.** The client declares what its own engine decodes in its capability profile's `audioDecode` (§3). The server chooses:
  1. `direct` when the (container, codec) pair is declared, and sample rate, channels and bit depth are within the declared limits, and the quality request allows the source bitrate.
  2. Otherwise `converted`, if the owner allows converting audio: **FLAC** for a lossless source or a lossy source with no bitrate limit (at the source sample rate and depth, capped to the client's limits; downmixed only past `maxChannels`, and then `downmixed: true`), or **Opus** at the requested bitrate when a bitrate limit applies.
  3. Otherwise `unavailable`.
- **Converted streams are exact.** They are encoded from the trimmed PCM:
  - FLAC's `trim` is `{0, 0}`;
  - Opus's `trim.startFrames` is its pre-skip (312 at 48 kHz), and `endFrames` its final-page padding;
  - `durationFrames` is at the output rate (Opus is 48 kHz).
  - A seek is `GET {url}?fromFrame=N` (in output frames). It returns a stream whose first output frame after its own `trim.startFrames` is exactly frame N, and its trim is repeated in `X-Audio-Trim-Start`/`X-Audio-Trim-End`.
  - Direct seeks use byte ranges and the container's own index (FLAC seek table, Xing TOC, MP4 sample table, Ogg bisection). Sample accuracy matters at boundaries, not at seeks.
- **Before commit** (a prepared, private presentation, §18.2): the grant serves at most `prefetchBytes` bytes in total, in any ranges. That covers the start plus a trailing index, such as an MP4 `moov` at the end. Beyond that it answers `403 prepared_limit`. A converted stream serves only `fromFrame=0` before commit.

### 18.2 `POST /v1/queues/{id}:prepare-next`
Headers: `If-Match: "<queue revision>"` (required) and `Idempotency-Key` (required; §17.1).
```json
{"sessionId": "ps_…", "sessionGeneration": 3}
```
- **When it's accepted.** The next entry is the one `:advance {reason: "completion"}` would play (repeat and shuffle included), and it starts at 0. The caller must be the device playing `sessionId`, which is the queue's current session. That session must be `playing`, `paused` or `starting` at `sessionGeneration`, with `audioRender.mode` `direct` or `converted`. The next entry must be playable by the caller (the fence, SEC-02).
- **200:**
  ```json
  {"token": "pn_…", "expiresAt": "2026-09-24T12:00:60Z", "entryId": "…", "itemId": "…", "presentation": {…with audioRender…}}
  ```
  The presentation is **private until commit**:
  - Its grant serves the `audioRender.url` within `prefetchBytes` (a converted stream from frame 0 only; §18.1), and the ordinary media URL is withheld.
  - It holds no lease and no stream slot of its own (it shares the current session's).
  - It creates no session resource, and it isn't listed in Now Playing.
- **Replay.** The same key replays the same answer while the token lives. A new `:prepare-next` for the same queue replaces (cancels) the previous token.
- **Expiry.** Hard, 60 s after acceptance. The server removes an expired preparation without being asked.
- **Cancellation.** Any other queue command, any PATCH or DELETE of the current session, a timeline report with a different state or rate, or a seek cancels an outstanding token.
- **Errors:**
  - `412 revision_mismatch` (with `error.current`);
  - `409 queue_ended` (nothing next);
  - `409 prepare_not_allowed` (`error.reason`: `session_not_current` | `session_changed` | `not_rendered`);
  - `503 queue_building` with `Retry-After`;
  - `404 not_found` (queue, or a withheld next entry);
  - `422 idempotency_key_reused`.

### 18.3 `POST /v1/queues/{id}:commit-next`
```json
{"token": "pn_…"}
```
- **200:** `{"queue": QueueView, "session": SessionView}`. `session` is a **new session resource** for the next entry. It inherits the previous session's state (`playing` or `paused`), `rate`, quality request and audio-track choice, starts at position 0, and has a fresh lease. Its presentation is the prepared one, now public: the whole `audioRender.url` and the media URL.
- **What happens atomically,** in one transaction:
  - the queue's current entry and session move (the queue revision increments);
  - the previous session ends with reason `completed`;
  - the previous grant stops serving **new** requests, while audio already fetched keeps playing (the client drains it);
  - the viewer's progress ordinal moves to the new session.
- **Idempotent by token.** A replay returns the same new session.
- **Fences.** All of these are re-checked at commit, and any failure cancels the token:
  - the token is unexpired;
  - the queue revision is unchanged since prepare;
  - the previous session is unchanged since prepare: not ended, same generation, same state and rate;
  - its lease is valid;
  - converting is still allowed (owner policy);
  - the next entry's source is unchanged (size and mtime pin);
  - the caller may still play it.
- **Errors:** `410 prepared_expired`, `409 prepared_canceled` (`error.reason`: `replaced` | `session_changed` | `queue_changed` | `source_changed` | `policy_changed` | `access_changed`), `404 not_found` (unknown token).
- **Client fallback.** On any error the client finishes the current track normally and advances with `:advance {reason: "completion"}`. Audio never plays twice, and nothing is skipped.

### 18.4 Events
- `queue.updated` (revision) after a commit.
- `session.updated` for the previous session (`state: "ended"`, `reason: "completed"`) and for the new one.
- A preparation's cancellation or expiry is **not** an event. The client learns it at commit (410/409) or from its own cancelling action.
- Admin Now Playing (`admin.sessions`) changes only at commit.

### 18.5 Advancing (applies without rendering too)
1. `:advance {reason: "completion"}` after the current session reported `ended` still starts the next entry's session. The one-request auto-advance does not depend on the old session being live.
2. The session `:advance` starts inherits state, rate, quality request and audio-track choice, as in 18.3.
3. Queue entries (window and entries pages) carry `albumId`, `disc` and `track` for a song on its album (a book file in its book), absent when unknown. A client joins consecutive tracks of one album gaplessly rather than crossfading them.
4. `QueueView` gains:
   - `next: {entryId?, available: bool, reason: "ready"|"end"|"unavailable"}`, computed through the viewer's fence (`entryId` is absent at the end; the API carries no nulls);
   - `postPlay: {autoplay: bool, countdownSeconds, passoutCheckDue: bool, automaticAdvances}`, from the viewer's listening preferences.

### 18.6 Channels
§11's `POST /v1/playback/sessions {channelId, startFrom}` is implemented as specified there, reusing today's linear producers:
- kinds `live` and `channel`;
- surfing with `replacesSessionId`;
- the linear clock (retained window, live edge, current programme) in the presentation;
- seek and go-live by PATCH;
- `409 no_tuner_available`.

**Implementation .** A channel session is an ordinary v1 session. The linear runtime's work selection, media authority, status and stage, retune, restart and intent move onto it (`playback_channel_sessions`, one row per v1 channel session).
- The session lease and the timeline are the channel's lease and observations.
- PATCH `state` and `seek` (`{"seek": {"positionMs": n}}` within the retained window, `{"seek": {"live": true}}` for the live edge) are the channel intent.
- DELETE and end stop the producer.
- No v2 occurrence exists for a channel at any point. A facade over a hidden occurrence (option A) was withdrawn.
- Plan — Client Playback Migration §10 has the commit sequence.

**The wire (B4, 23 Sep):**
- **Start.** `POST /v1/playback/sessions {channelId, channelGeneration?, state?, startFrom?: "live", replacesSessionId?}`.
  - `channelId` is `live:<sourceId>:<channelId>` (a Live TV channel, the key the server already uses for live channels) or `library:<channelId>` (a Library Channel).
  - `channelGeneration` is the guide generation the viewer tuned from, a fence against a lineup that changed since. Absent means the current one.
  - A start refused because every tuner of the source is in use is `409 no_tuner_available`: the claims are tuners held plus live channel sessions not yet holding one, less the session being replaced.
  - A channel the viewer can't see is `404 not_found` (SEC-02).
  - Kinds are `live` and `channel`. Surfing is `replacesSessionId`: the old session ends `replaced`, one slot.
- **Presentation.** `mode: "stream"`, `url` (the channel's HLS), and `generation` (the timeshift buffer's; a new buffer is new bytes, and the client reloads). `linear` carries:
  - `name`, `programme` and `next` (`{id, itemId?, title, startMs, endMs}`);
  - `originMs`, and `windowStartMs`, `windowEndMs`, `liveEdgeMs` (milliseconds on the channel timeline);
  - `state` (`preparing` | `active` | `recoverable`) and `errorCode?`;
  - `seek?` (`{id, live?, positionMs?, acknowledged, error?}`) and `confirmedPositionMs?`.
- **PATCH.**
  - `state` plays or pauses. `playing` on a channel whose source failed (`linear.errorCode`) is a retry.
  - `seek: {id?, positionMs}` goes to a position in the retained window. `seek: {id?, live: true}` goes to the live edge.
  - Nothing else applies.
  - Errors: `409 seek_unavailable` (outside the window, a gap, a stale buffer) and `503 channel_unavailable`.
- **Timeline.** It renews the lease and is the channel's observation: `generation` is the buffer's and `positionMs` is on the channel timeline. A report within 1 s of a pending seek's target acknowledges it. A report for an older buffer renews the lease and changes nothing.
- **DELETE and end.** A stop, `replaced`, terminate or a lapsed lease ends the channel's work, and its media stops at once.
- **Now Playing** lists the session once, as `kind: "channel"`, titled with the channel's name.

Retiring `/v2` (occurrences, controllers, lanes, receipts, the command journal, queue playback, prepared-audio staging and the preparation stack, their routes and tables, and all three clients' v2 code) comes last, after both apps ship every kind on v1.

### 18.7 Audio facts (scan and analysis)
One row per audio asset (`audio_render_facts`), kept by the scan and analysis pipeline and pinned to the source's size and mtime. A changed file is re-measured before its facts are used again.
- **Probe (at scan):** container, codec, sample rate, channels, bit depth, and the tags (ReplayGain, R128, `iTunSMPB`, encoder delay and padding).
- **`audio_facts` operation** (deep scan, on by default for music and audiobook libraries; single-pass packet read, no decode). The server reads the audio packets (`ffprobe -show_packets`) for:
  - the raw frame total;
  - `skip_samples` on the first packet and `discard_padding` on the last, which is how FFmpeg surfaces the LAME header, MP4 edit lists and `iTunSMPB`, and Opus pre-skip;
  - `decoderConfig`.

  Header evidence is cross-checked against the packet total. When it is missing or inconsistent (a trim larger than the stream, an `iTunSMPB` total beyond the packets), the file is **decoded** with the container's own trimming, and the decoded count is authoritative (`trim.source: "measured"`).
- **`loudness` operation** (existing): integrated loudness, true peak and loudness range. It feeds `gain` when there are no tags, and the album values once the album is measured.
- **At play time**, facts that are still missing are measured for that asset on demand (bounded: one file, packet read only). The plan is never guessed. If they can't be measured in time, the mode is `converted` (exact by construction) or `unavailable`.

### 18.8 Conformance fixtures
`fixtures/audio-gapless/` holds generated tracks with known answers, used by the server tests and by every client engine's exit tests:
- LAME MP3 (LAME header);
- AAC in M4A (edit list and `iTunSMPB`);
- Ogg Opus (pre-skip);
- FLAC at 44.1/16 and 96/24;
- ALAC in M4A;
- WAV;
- a 12-track continuous sine album, cut at non-frame-aligned points, so any gap or overlap is measurable to the sample.

Each fixture has a JSON sidecar with the expected `durationFrames`, `trim` and the first and last sample values after trim. An engine passes when, on its platform:
- every fixture yields exactly `durationFrames` frames;
- the output has a best-fit lag of 0 against the signal;
- every sample is within `tolerance`, except within `edgeFrames` of either end, where `edgeTolerance` applies (lossy decoders ring at a cut that isn't frame-aligned);
- the 12-track album joins with a boundary error of at most 1 frame (≤1 ms is the exit bar; 1 frame is the target).

Core Audio note (F-title, 23 Sep): ExtAudioFile honors the LAME header and `iTunSMPB` itself. Its MP3 packet-table priming is 576 (the encoder delay), where FFmpeg's `trim.startFrames` is 1105 (the encoder delay plus the 529-frame decoder delay). The rule: if ExtAudioFile's frame count equals `durationFrames`, skip nothing; otherwise skip `trim.startFrames` minus the packet-table priming.

