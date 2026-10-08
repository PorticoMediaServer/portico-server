# Playback and media

Playback matches or exceeds Plex. The server decides how a file reaches a device: played as-is, remuxed, or converted. The player on each client is a thin engine that follows that decision. Quality changes only when a person asks; server load never lowers it.

## Playback Protocol v1

`internal/playbackv1` is the resource layer clients use (API guide: `server/api/guide/04-playback-v1.md`):

1. A client opens a **playback session** for an item, declaring its device profile: codecs, containers, HDR, audio channels and subtitle support. Declared profiles for known devices live in `internal/playback/testdata/client-profiles.json`.
2. The server answers with **offers**, one per way to play: direct play of the original, a direct stream (remux to HLS without re-encoding), or a conversion. Each offer states its audio and subtitle renditions.
3. The client takes an offer and plays its URL. Progress, seeks, rendition switches and the end are reported on the session. An end is reported once per session generation, and a `404`/`410` from the server counts as a confirmed end.

`internal/playback` holds the delivery decision (`delivery_decision.go`, `delivery_plan.go`), per-owner delivery settings, and diagnostics that explain why a decision was made.

## Delivery

- **HLS for video.** Finite (VOD) HLS is produced on demand in segments and closed segments are immutable (`hls_*.go`). Seeks restart production at the nearest keyframe, and a bad network gets bounded retries.
- **Conversions** run FFmpeg through one executor (`internal/mediaexec`). Background conversions run at the lowest OS priority; interactive ones don't.
- **Hardware acceleration** is detected and reported to the owner. A failed hardware encode falls back to software, never to lower quality.
- **Subtitles** (`internal/subtitles`, `internal/subtitlevideo`): text subtitles are delivered as WebVTT renditions where the client supports them. Image subtitles, and styled subtitles a client can't render, are burned in.
- **Chapters, markers and lyrics:** chapter thumbnails, detected intro/credits markers (`segmentmarkers`) and lyric documents (`lyrics`) are served alongside playback.
- **Prepared media** (`internal/preparedmedia`): owner-requested full conversions and offline-download preparations are stored as closed artifacts. `artifactlease` pins an artifact while any reader uses it.

## Music and audiobooks

Plex and Plexamp are the standard. Gapless is a hybrid:

- Clients direct-play the original file and decode it on the device. The server supplies the encoder delay, padding and gain metadata the client needs to trim exactly.
- The server converts only formats a client can't decode, to FLAC or Opus, with exact trim. It never streams raw PCM.
- The audio plan (`playback/audio_plan*.go`) chooses per track. The web app decodes with WebCodecs (`web/src/bridge/DecodeAudioRender.ts`); the Apple app uses native modules (`PorticoAudioRender`).
- Queues, shuffle, repeat and listening preferences are shared client-core logic. The server records listening sessions for history and recommendations.

## Decoder confinement

Every FFmpeg and ffprobe the server runs goes through `internal/mediaexec` and `internal/decoder`:

- **Always:** the input is passed as an open descriptor or a private loopback bridge (`privateoutput`); the protocol whitelist, minimal environment, bounded output size, no core dumps, own process group, parent-death signal and an orphan ledger.
- **On Linux, additionally a bubblewrap sandbox** (user, PID, network, IPC and UTS namespaces, read-only mounts of FFmpeg's exact dependencies) where the host allows unprivileged user namespaces. The server tries it at startup and reports the outcome in the console.
- `PORTICO_DECODER_SANDBOX=required` makes media jobs fail instead of running unsandboxed; `=off` skips the sandbox.
- **FFmpeg itself** is a qualified bundle (`third_party/ffmpeg`, built by `scripts/build-ffmpeg.sh` and verified against its manifest), or explicitly configured binaries (`PORTICO_FFMPEG`, `PORTICO_FFPROBE`).

## Live TV, DVR and Library Channels

- **Sources:** HDHomeRun tuners, M3U playlists and Xtream accounts (`internal/livechannels`), with an XMLTV or provider guide. Clients never show source-type names.
- **Live playback** shares one input per channel across viewers (`linearinput`); `linearbuffer` holds closed HLS segments so late joiners and rewinds are cheap.
- **DVR:** recordings are scheduled from the guide, written to the recordings folder, and become library items (`recordingaccess`, `recordingmedia`).
- **Library Channels:** owner-scheduled linear channels built from library queries (any browse filter, text match, rating thresholds, boolean groups, ordering). They're always shared, with no personal filters.

## Casting and remote playback

- **Chromecast:** one shared Cast receiver application serves every server. Its page is compiled into each server at `/receiver/cast/` (`server/receiver/cast`) and published at cast.getportico.tv (`portico-internal/cast-receiver`).
- **AirPlay:** the system picker, where the platform has one.
- **Play on:** signed-in Portico devices found by local-network discovery. Watch Together groups (`internal/social`) sync playback across devices.
- tvOS shows no casting UI.
