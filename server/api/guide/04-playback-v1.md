# 4. Playback v1

Two playback models are registered side by side. `/v1` is the original
single-session model (open a session, read its stream URL, report progress).
`/v2` is the renderer-independent control plane (controllers, lanes,
occurrences, queues). This section covers the `/v1` path a simple player
needs and points at the `/v2` routes it graduates to; it does not re-specify
the `/v2` protocol state machine (see `playback-v2-protocol.md`).

## Sessions

`POST /v1/playback/sessions (openapi.yaml, operationId createPlaybackSession)`
opens one session for one item:

<!-- example: playback-create-request file=openapi.yaml operationId=createPlaybackSession direction=request schema=@request /v1/playback/sessions post -->
```json
{
  "itemId": "item_0001"
}
```

The response is the ready session with its grant-bound `streamUrl`:

<!-- example: playback-session-response file=openapi.yaml operationId=createPlaybackSession direction=response status=201 schema=#/components/schemas/PlaybackSession -->
```json
{
  "id": "sess_play_1",
  "generation": 1,
  "streamUrl": "/v1/media/grant_01J6X7QZ9MAB0CDEF123456789",
  "mode": "direct",
  "duration": 6540.0,
  "resumeSeconds": 320.5
}
```

`GET /v1/playback/sessions/{id} (openapi.yaml, operationId getPlaybackSession)`
re-reads the session (generation increments on renewal — re-read before
sending progress); `DELETE /v1/playback/sessions/{id} (openapi.yaml,
operationId stopPlaybackSession)` stops it, releases delivery resources, and
invalidates the grant immediately (in-flight bytes stop mid-stream).
`GET /v1/playback/sessions/{id}/chapters (openapi.yaml, operationId
getSessionChapters)` reads chapters. Before choosing a source or quality, read
`GET /v1/items/{id}/playback-offers (openapi.yaml, operationId
getPlaybackOffers)` (`sessionId` ties offers to a session, `revision` fences
them; a newer projection answers `stale_playback_offer`):

<!-- example: playback-offers-response file=openapi.yaml operationId=getPlaybackOffers direction=response status=200 schema=#/components/schemas/PlaybackOffers -->
```json
{
  "scope": {},
  "revision": "rev_7",
  "sources": [],
  "controls": [],
  "preparedVersions": [],
  "preparedOffersRevision": "rev_7"
}
```

Session resources with `ETag`/`If-Match`, `Idempotency-Key` headers, and PATCH
semantics are **not specified**; the body `requestId` on creation is the
idempotency key.

## Queues and selectors

`/v1` has no queue resource. Queues live under `/v2`: `PUT /v2/queues/{id}
(openapi.yaml, operationId createQueue)` creates or replaces a queue, `GET
/v2/queues/{id} (openapi.yaml, operationId getQueue)` reads it, `POST
/v2/queues/{id}/commands (openapi.yaml, operationId mutateQueue)` mutates it,
`POST /v2/queues/{id}/playback (openapi.yaml, operationId startQueuePlayback)`
starts an entry (200 ready, 202 preparing — then poll `GET
/v2/queues/{id}/playback/requests/{request} (openapi.yaml, operationId
getQueuePlaybackRequest)`), and `GET /v2/queues/{id}/playback (openapi.yaml,
operationId getQueuePlayback)` returns the combined queue/occurrence/next/media
view. Every queue read is bounded and answers `429 queue_busy` past 8 command
slots (24 separate event-poll slots).

Selectors as a tagged union (`container` | `query` | `items`) exist for bulk
jobs (`POST /v1/jobs (jobs.openapi.yaml, operationId create_job)`, schema
`Selector`), not for queues — queue entries address items directly. The job
selector shape is:

<!-- example: playback-selector-items file=jobs.openapi.yaml operationId=create_job direction=request schema=#/components/schemas/Selector -->
```json
{
  "items": {
    "ids": ["item_0001", "item_0002"]
  }
}
```

Queue segments-from-selectors, seeded shuffle orders, and play-from-here
anchors are **not specified** in the registered queue routes.

## Timeline

The renderer reports position and transport state to `POST
/v1/playback/sessions/{id}/progress (openapi.yaml, operationId
reportPlaybackProgress)` (204 on acceptance). `generation` must equal the
session's current generation and `sequence` must increase monotonically, so a
delayed or duplicated report is discarded instead of rewinding resume state.
That report is what advances watched state, resume positions, and history:

<!-- example: playback-progress-request file=openapi.yaml operationId=reportPlaybackProgress direction=request schema=#/components/schemas/PlaybackProgress -->
```json
{
  "generation": 1,
  "sequence": 120,
  "positionSeconds": 321.5,
  "state": "playing"
}
```

Marker skips are recorded evidence, not lane commands: `POST
/v1/playback/occurrences/{playbackId}/marker-skip (openapi.yaml, operationId
recordMarkerSkip)`. The `/v2` occurrence path uses `POST
/v2/playback/occurrences/{playbackId}/intent (openapi.yaml, operationId
setPlaybackIntent)` (control) versus `POST
/v2/playback/occurrences/{playbackId}/observations (openapi.yaml, operationId
observePlayback)` (renderer evidence); channel occurrences use their own
channel-intents/channel-observations routes and never enter personal state.
Microsecond clocks and `seq`-ordered worklets are **not specified** for `/v1`.

## Media grants

`GET /v1/media/{grant} (openapi.yaml, operationId getMediaStream)` reads the
media body authorized by the grant in the path — no bearer token. The grant
resolves to a principal and item, access is re-checked around every read, and
range requests are supported (`206` for ranges). There is no JSON body to show:
success is `video/mp4`, `audio/mp4`, `audio/mpeg`, or `audio/aac` bytes.
Subtitle documents, lyric revisions, audio renders, analysis artifacts,
trickplay tiles, and download artifacts follow the same grant-in-path pattern
with their own routes. Short-lived signed artwork URLs for third parties are
**not specified** beyond the registered grant routes.

## Events

Queue followers poll `GET /v2/queues/{id}/events (openapi.yaml, operationId
pollQueueEvents)` (up to two seconds against an opaque cursor), and any viewer
can resume the unified feed at `GET /v1/events (openapi.yaml, operationId
getEvents)` (SSE, or long-poll with `after`+`waitSeconds` — see section 6).
Occurrence lifecycle (`POST /v2/playback/occurrences (openapi.yaml, operationId
createPlaybackOccurrence)`, `GET /v2/playback/occurrences/{playbackId}
(openapi.yaml, operationId getPlaybackOccurrence)`), lanes (`POST
/v2/playback/lanes (openapi.yaml, operationId establishCommandLane)`), and
controllers (`POST /v2/playback/controllers (openapi.yaml, operationId
registerPlaybackController)`, every `/v2` call also carrying
`X-Playback-Controller-Token`) are control-plane setup, not per-play calls.
Typed invalidation topics beyond the registered `notification.changed`,
`operation.updated`, and `stream.resync` event types are **not specified**.

## Subtitle delivery

The generated registry contract in `portico.openapi.json` describes the subtitle
routes. `GET /v1/items/{id}/subtitles` includes all admitted discoveries as
selectable resources. Discovery is enough to choose or automatically choose a
track: clients do not import a track before playback. Its bytes are an internal
cache, filled on first delivery. A disconnected first request does not discard
that work; a retry joins it. A cold embedded track can still require a full demux
pass before its bytes are available.

Each resource and playback-option track exposes `deliveries`. Formats are `text`,
`ass`, `pgs`, `vobsub`, and `dvb`. A selected track includes URLs under the current
playback grant:

| Delivery | Payload | Additional files |
| --- | --- | --- |
| `text` | `{base}.vtt`, UTF-8 WebVTT | None |
| `ass` | `{base}/original.ass` or `original.ssa`, authored script | `{base}/fonts` lists font names and grant URLs |
| `pgs` | `{base}/original.sup`, PGS packets | None |
| `vobsub` | `{base}/original.idx` | `companionUrl` supplies `original.sub`; preserve the matching basename locally |
| `dvb` | `{base}/original.mks`, a single-track Matroska DVB packet capsule | None |

Here `{base}` is `/v1/media/{grant}/subtitles/{resource}/{revision}`. Original and
font files accept GET, HEAD, byte ranges, and conditional reads. WebVTT also
supports byte ranges. ASS/SSA originals preserve their source encoding; the
response content type declares its detected charset. Decode that encoding before
passing script text to a renderer that requires Unicode. Font filenames are
labels; download through each returned URL, not a filename-derived path.

The separate `{base}` route returns a version 1 JSON cue document:
`{version:1,timeDomain:"source-relative",cues:[{startUs,endUs,text,italic?,top?}],skippedCues?}`.
Times are decimal microsecond strings. The selected `documentUrl` names this JSON
form; a presentation labelled `webvtt` names the `.vtt` form. Plain-text clients
apply the selected offset locally to JSON cues. WebVTT includes the offset.

Subtitle selection accepts `rendering: "text"|"ass"|"pgs"|"vobsub"|"dvb"` when a
device chooses a supported delivery itself. `renderer: "client"` means an
original is drawn on the device, without changing the video. The v1
`embeddedClient` selection now uses this path and returns the original URLs.
A client should choose it only for formats its player declares it can render;
otherwise the server's ordinary burn-in path remains available subject to policy.
Styled resources can also be explicitly selected with `rendering: "text"`.

Grants, live session pins, viewer visibility, exact source bytes, and remote
representation evidence authorize delivery. Stopping a session or changing its
grant ends access. Re-read the presentation after renewal. JSON cue clients can
retain the existing validator-based authorization checks; byte ranges and
conditional responses never bypass grant checks.

Admission remains bounded: 32 discovered tracks per file, 4 MiB source text,
32 MiB bitmap data, 20,000 cues, 8 KiB per plain cue, and a 24-hour timeline.
Inventory capacity retains usable tracks and a warning. Font extraction admits
up to 16 attempts, 8 MiB per font, 16 MiB total; a font list can report
`skippedCount`. Malformed cues are skipped; `skippedCues` reports the count where
available. Two cache fills run at once, with at most 64 in flight. Each track
allows two minutes without input progress, bounded by two hours overall.

Subtitle search uses the profile's preferred subtitle language when `language`
is empty, falling back to English. It keeps regional Portuguese/Chinese tags and
uses title/provider identities or series and episode identity. OpenSubtitles
configuration uses `PORTICO_OPENSUBTITLES_API_KEY`, optional
`PORTICO_OPENSUBTITLES_USERNAME` and `PORTICO_OPENSUBTITLES_PASSWORD`, and optional
`PORTICO_OPENSUBTITLES_USER_AGENT`. Username/password enable login and renewal;
credentials and tokens never appear in subtitle responses. The former immutable
`PORTICO_OPENSUBTITLES_TOKEN` input is removed.
