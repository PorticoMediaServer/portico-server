# 11 — Live TV, guide, DVR and Library Channels

Static source review, 11 October 2026. Nothing here was run. Every finding is a reading of source code, not an observation of a running server or device.

Shorthand used for paths:

- `S/` = `apps/portico-server/server/internal/`
- `W/` = `apps/portico-server/web/src/`
- `C/` = `apps/portico-server/packages/client-core/src/`
- `N/` = `apps/portico-react-native/apps/native/`
- Comparison paths are relative to the clone folder (`jellyfin/…`, `jellyfin-web/…`, `plezy/…`).

Words used throughout:

- **Tuner / input**: one connection to a TV source (a physical HDHomeRun tuner, or one HTTP connection to an IPTV provider).
- **XMLTV**: the common XML file format for programme guides. **M3U**: the common text playlist format for channel lists. **Xtream**: a widely used IPTV provider API (`player_api.php`).
- **Generation**: Portico's name for one complete imported copy of a source's channels and guide.
- **Padding**: extra time recorded before and after a programme.
- **Timeshift**: pausing and rewinding live TV.
- **Retune**: stopping the stream for a channel and starting it again.

## Scope and sources

**Portico, read in full unless noted**

- Docs: `Project Architecture/04 Playback and Media.md` (Live TV section), `Specs/Spec — Channels and Guide.md`, `Specs/Spec — Custom Channels Builder.md`, `Product Contracts/live-tv-and-dvr/contract.md`, `Product Contracts/library-channels.md`, the Live TV section of `architecture-amendments.md`, `Backlog.md` (it has no open Live TV, DVR or Library Channels item apart from a fixed planner-statistics note that mentions the guide table).
- Server: `S/livechannels/` (`model.go`, `parse.go`, `remote.go`, `adapters.go`, `store.go`, `directory.go`, `allocator.go`, `physical*.go`, `occurrence.go`, `conflicts.go`, `programme_facts.go`, `generation_retention.go`, `preferences.go`), `S/livechannels/dvr/` (all non-test files), `S/linearinput/`, `S/linearbuffer/`, `S/recordingmedia/`, `S/decoder/linear.go` and `linear_hls.go`, `S/playbackruntime/linear.go`, `linear_plan.go`, `linear_resolver.go`, `linear_direct.go`, `S/playback/channel_sessions.go`, `S/playbackv1/channels.go`, `S/httpapi/live_channels.go`, `channel_tuners.go`, route lists of `dvr.go`, `guide_directory.go`, `library_channels.go`, `administration.go`, `S/administration/live_sources.go`, `dvr_settings.go` (types and validation).
- Server, skimmed: `S/livechannels/library/` (model, generator core, maintenance; not the candidate SQL or templates), `S/recordingaccess/`, `S/livechannels/dvr/query.go` and `presentation.go` (not read).
- Clients: `C/guide/legacy-source.ts`, `C/guide/time.ts`, `C/linear-api.ts`; feature greps over `W/screens/channels/Channels.tsx`, `W/screens/server/LiveTV.tsx`, `LiveSourceExtras.tsx`, `N/src/screens/NOTES-live.md`, `N/android/.../PorticoCapabilities.kt`. I did not read the guide grid components, the DVR screens, the mini-guide or the native guide line by line.

**Jellyfin**

- `jellyfin/src/Jellyfin.LiveTv/`: `TunerHosts/M3uParser.cs`, `M3UTunerHost.cs`, `SharedHttpStream.cs`, `LiveStream.cs`, `HdHomerun/HdHomerunHost.cs` (grep), `Listings/XmlTvListingsProvider.cs`, `ListingsManager.cs` (grep), `Guide/GuideManager.cs` (grep), `Timers/TimerManager.cs`, `Recordings/RecordingsManager.cs` (record flow, paths, retry), `DefaultLiveTvService.cs` (stream sharing, series timers), `IO/EncodedRecorder.cs` (grep).
- `jellyfin/Jellyfin.Api/Controllers/LiveTvController.cs` (endpoint list), `MediaBrowser.Model/LiveTv/*.cs` (option classes), `MediaBrowser.Controller/LiveTv/SeriesTimerInfo.cs`.
- `jellyfin-web/src/components/guide/guide.js` (grep).

**Plezy**: file inventory of live TV code, `lib/services/live_seek_accumulator.dart`, `lib/screens/video_player/live_stream_retry.dart`, `lib/models/livetv_capture_buffer.dart`, header of `lib/media/live_tv_support.dart`.

**Public documentation**: the XMLTV DTD (fetched today, for date format, optional `stop`, and the DOCTYPE line). ErsatzTV's IPTV endpoints (searched today). Statements about Plex DVR, Channels DVR, Emby, Tvheadend, Tunarr and dizqueTV are from my general knowledge of their public documentation and were not re-verified today; treat them as pointers.

**Not reached**: Jellyfin's Schedules Direct client, `HdHomerunUdpStream`/`HdHomerunManager` internals, jellyfin-androidtv, Swiftfin, Wholphin and Streamyfin live TV screens, Plezy's guide tab, Portico's Roku and Vega clients, the cast receiver.

## How Portico does it today

### Sources

An owner adds a live source in the web console (`W/screens/server/LiveTV.tsx`). There are three kinds (`S/livechannels/remote.go:99`):

- **M3U playlist URL**, with an optional XMLTV guide URL.
- **Xtream account**: the server calls `player_api.php?action=get_live_streams`, builds a playlist from the answer, and by default uses the provider's `xmltv.php` as the guide (`S/livechannels/adapters.go:74-128`).
- **HDHomeRun**: the server reads `discover.json` (for the tuner count) and `lineup.json` (for the channels) over HTTP (`adapters.go:129-175`). The web form does not offer a guide URL for this kind (`LiveTV.tsx:117`).

All three are turned into the same internal shape: an M3U text plus an optional XMLTV text, at most 16 MiB together, at most 2,000 channels and 100,000 programmes (`S/livechannels/model.go:20-22`). One parser handles both (`S/livechannels/parse.go`).

Adding a source is two steps: **preview** (fetch and parse, nothing published, the fetched data is parked for 20 minutes) and **save** (publish). Fetches go through `S/remotemedia`, which pins DNS, refuses private addresses unless the owner has confirmed that exact local address ("LAN confirmation"), and bounds sizes and timeouts. Source URLs and passwords are stored as plain columns (`S/livechannels/store.go:88-92`), which matches the "open data" principle.

A background worker refreshes each remote source on its own interval (1, 6, 12 or 24 hours in the UI; default 6 hours, `C/linear-api.ts:11`), with a lease so two workers cannot refresh the same source, and exponential back-off on failure (`remote.go:391-481`).

### Guide ingest and data model

Each save or refresh creates a new **generation**: a full copy of every channel row and every programme row, written in batches of 128 rows per transaction (`store.go:282-341`). When the copy is complete and its row counts match, one transaction switches the source's `active_generation` pointer. Readers only ever follow that pointer, so nobody sees a half-imported guide, and a failed refresh leaves the old guide in place. Old generations are deleted in small batches 24 hours after they are replaced (`generation_retention.go`).

Identity:

- A channel's id is a hash of the source id and the playlist's `tvg-id` (`store.go:298`).
- A programme's id is a hash of the source, the channel and the programme's start time (`parse.go:279`, `occurrence.go`). XMLTV has no standard programme id, so in practice this is always the rule.
- A programme's series id comes from a `<series-id>` element (not part of standard XMLTV) or the first ten characters of a `dd_progid` episode number (`parse.go:252-265`).

The parser validates strictly and rejects the **whole document** on the first problem it finds (see B-11.2 to B-11.7). It keeps title, description, sub-title, season and episode, categories, rating, year, star rating, the live, premiere, new and repeat flags, and one image URL per programme.

### Guide queries and clients

- `GET /v1/guide/sources` summarises sources (channel count, groups, favourites, guide range).
- `GET /v1/guide/channels` pages the channel list with server-side sort (by playlist order, by channel number, or by name with proper collation) and filters (source, group, favourites, hidden) (`S/livechannels/directory.go`).
- `GET /v1/guide` returns programmes for up to 50 named channels and a window of at most 24 hours, in one indexed query (`store.go:521-781`).

The clients hold only the visible channels and hours ("tiles" of 50 channels by 3 hours, at most 24 tiles in memory) and lay cells out in real UTC milliseconds (`C/guide/`). Favourite and hidden state is stored per profile on the server (`S/livechannels/preferences.go`).

### Live playback

Watching a channel is an ordinary Playback Protocol v1 session started with a `channelId` (`S/playbackv1/channels.go`). For each session the server:

1. takes a **tuner allocation** for that session (`S/playbackruntime/linear.go:658`),
2. opens its own connection to the source through a private loopback gateway (`S/linearinput/gateway.go`). The gateway accepts only a raw MPEG-TS stream or an HLS playlist, and for HLS it rewrites every URL so FFmpeg can only fetch through the gateway,
3. probes the stream, decides copy or convert (`S/playbackruntime/linear_plan.go`), and runs one sandboxed FFmpeg that writes 4-second HLS segments,
4. stores those segments in a per-session directory, the **timeshift buffer** (`S/linearbuffer/buffer.go`), 30 minutes or 1 GiB by default, whichever comes first (`linear.go:150-155`),
5. serves a sliding HLS playlist with wall-clock timestamps to the client. Pause, seek inside the window and "go live" are session operations.

So one viewer equals one source connection, one FFmpeg and one buffer. Two people watching the same channel use two of each.

The copy-or-convert rule is narrow: the stream is copied only if it is progressive H.264 with stereo AAC-LC inside the client's limits. Anything else (MPEG-2, HEVC, interlaced video, AC-3, 5.1 audio) is converted to H.264 and 192 kb/s stereo AAC. Only the first video and first audio track are used and subtitles are dropped (`S/decoder/linear_hls.go:105-125`).

### Tuner capacity

A source's capacity is "known" when the owner set a limit or accepted the HDHomeRun's discovered tuner count; otherwise it is unknown and nothing is refused (`S/livechannels/allocator.go:30-59`). Allocations are rows in SQLite with a 20-second lease that the running job renews, plus a file lock that the FFmpeg child inherits, so a crashed server cannot free a tuner that a surviving child still holds (`S/livechannels/physical.go`). Scheduled recordings write **reservations**; a live session may not start, or is given an end time, where a reservation will need the tuner (`allocator.go:171-177`).

### DVR

- A viewer records one programme, or creates a **series rule** for a series id. Rules can ask for new episodes only, allow or block channels, require or block keywords, and set padding, priority, retention days and an episode cap (`S/livechannels/dvr/model.go:94-160`).
- Recordings and rules belong to one profile. The recording id is a hash of the profile, source and programme (`dvr/model.go:44-46`).
- A worker sleeps until the next start time, claims the recording, checks permissions, the guide, storage and conflicts, takes a tuner allocation, and captures (`dvr/worker.go`).
- Capture is a stream copy to MPEG-TS through the same gateway, written into a content-addressed private store (`S/recordingmedia/driver.go`). Progress is checkpointed about once a second.
- At the end the file is validated and probed and published as a "movie" item in a private per-profile library called **Recorded TV** (`dvr/publication.go:59,82-88`). A recording is `completed` only if the capture reached the planned end, had no gaps and its duration matches; otherwise it is `incomplete-playable` with a note in its description.
- Retention deletes by age and by per-series episode cap, never a recording marked Keep (`dvr/retention.go`). A free-space floor and an optional size cap are checked before and during every write (`recordingmedia/storage.go`).

### Library Channels

Owner-built linear channels (up to 64) made from library movies and episodes. Each channel has rules (a browse filter, text match, pinned titles, an ordering and an episode mode) and optional weekly time blocks. A background generator produces a durable seven-day schedule per channel from a fixed seed, extends it as time passes, and publishes it atomically like a guide generation (`S/livechannels/library/generator.go`). Items play back to back at their real lengths; when nothing is eligible a 30-minute "slate" entry is scheduled.

Tuning a Library Channel uses the same per-session runtime as live TV: FFmpeg reads the scheduled file from the current offset at real-time speed into the session's buffer (`S/playbackruntime/linear.go:778-853`). A title the viewer's profile may not see shows as "Not available on this profile" and cannot be tuned.

### Permissions

- Each source is "owner only" or "server members".
- Each profile has a Live TV switch; it is rechecked on every start, every 500 ms while playing and on every media request (`S/playbackruntime/linear_resolver.go:143-152`).
- Members can have channel allow or deny lists (`admitChannel` in `playbackv1`).
- Recording needs an explicit per-account grant (`S/recordingaccess/grants.go`).
- With content restrictions active, guide programmes above the profile's rating are shown as an untitled slot (`S/httpapi/live_channels.go:377-381`).

### A second settings layer

`S/administration/live_sources.go` and `dvr_settings.go` hold owner "documents" for live sources and DVR: stream buffer seconds, retry window, user agent, guide days, channel filters, renumbering, attaching another source's guide, a per-channel map (number override, guide channel id, hidden), DVR default padding and keep policy, folder templates, conversion and sidecar files. The web console edits several of these (`W/screens/server/LiveSourceExtras.tsx`). Most of them are not read by the code that actually fetches, lists or plays channels (see B-11.9).

## How the others do it

### Jellyfin

- **Tuners** are plug-in "tuner hosts". Built in: M3U (a URL or a local file; `http`, `https`, `rtsp`, `rtp` and `udp` stream URLs) and HDHomeRun (HTTP, plus the old UDP protocol for legacy devices, with discovery). Plugins add Tvheadend, NextPVR and others.
- Each tuner host has options that are scars from real providers: a custom user agent (a browser-like one by default), a simultaneous-stream limit, "allow stream sharing", "ignore DTS", "read at native frame rate", "enable stream looping", a fallback bitrate, and whether hardware transcoding and fMP4 are allowed (`jellyfin/MediaBrowser.Model/LiveTv/TunerHostInfo.cs`).
- **Shared streams.** When sharing is allowed and the stream is plain MPEG-TS over HTTP, the first consumer opens one upstream connection that is copied to a temp file; later viewers and recordings of the same channel attach to it and a consumer counter is incremented (`DefaultLiveTvService.cs:462-476`, `TunerHosts/SharedHttpStream.cs`). The stream limit counts open streams, not viewers (`M3UTunerHost.cs:84-92`).
- **Delivery** then goes through the normal playback decision: direct play (the client is given the provider URL when there is no stream limit), remux, or transcode with hardware acceleration, with the client's audio and subtitle selection.
- **Guide.** "Listings providers" are separate from tuners: XMLTV (a URL or a local file, gzip by file extension, cached on disk, falls back to the last good file) and Schedules Direct. Channels are matched to guide channels automatically by id, tuner channel id, number and normalised name, and there is a mapping screen (`Listings/ListingsManager.cs:390-470`, `LiveTvController` `ChannelMappingOptions`/`ChannelMappings`).
- Programmes are ordinary library items; a daily task updates them incrementally, using a per-programme etag to skip unchanged rows (`Guide/GuideManager.cs`). Guide length is 1 to 14 days.
- XMLTV categories are mapped to movie, sports, news and kids flags by configurable category lists. For XMLTV, a programme with an episode number gets a series id derived from its **title**, and a "show id" used to detect the same episode airing twice (`XmlTvListingsProvider.cs:288-317`).
- **DVR.** Timers are server-wide (not per user) and stored in JSON files; each pending timer is an in-process timer (`Timers/TimerManager.cs`). Series timers match by series id or, when there is none, by programme name, with options for new episodes only, any channel, any time, days of week, keep-up-to N, and skip episodes already in the library (`DefaultLiveTvService.cs:644-668, 806-834`).
- There is no conflict planning. A failed or interrupted recording is retried up to ten times at 60-second intervals while the programme is still on (`Recordings/RecordingsManager.cs:403-412`).
- Recordings are written under readable folders (`Series/Name (Year)/Season N/`, `Movies/`, `Kids/`, `Sports/`), with NFO and images, appear in normal libraries while still recording, and can be handed to an owner-defined post-processing program (`RecordingsManager.cs:476-585, 803-838`).
- **Permissions** are two user flags: Live TV access and Live TV management.
- **Web guide.** Loads up to 500 channels and a whole day of programmes per page and renders them all, with filters for movies, series, sports, kids, news and favourites, and optional colour coding by category (`jellyfin-web/src/components/guide/guide.js`).

### Plezy

A client only. For Plex and Jellyfin servers it offers a guide tab, a programme search sheet, a "show schedule" screen listing every airing of a show, a recordings tab, locally stored favourite channels, deep links to channels, and a live timeline bar. Its notable engineering is defensive: rapid skip presses on live TV are collected into one seek (`live_seek_accumulator.dart`), live stream recovery is guarded against stale attempts (`live_stream_retry.dart`), and the seekable window is read from the server's capture buffer rather than invented (`livetv_capture_buffer.dart`).

### Other products (public documentation, from memory)

- **Plex DVR**: HDHomeRun and USB tuners; its own cloud guide by postcode, or XMLTV; recordings go into the owner's normal TV and movie libraries with full metadata; series options include new airings only, preferred resolution, replace lower quality, padding and keep limits; a draggable priority list; commercial detection as skip markers or destructive removal; pause and rewind live TV; watch while recording.
- **Channels DVR**: HDHomeRun, TV Everywhere and custom M3U sources; commercial detection with automatic skip; passes including team passes and rule-based passes; "Virtual Channels" built from the library and recordings; tuner sharing.
- **Emby**: similar to Jellyfin, plus automatic conversion of recordings and a guide-data service.
- **Tvheadend / NextPVR**: back ends for DVB, ATSC, SAT>IP and IPTV with their own EPG grabbers; Tvheadend's auto-recordings match by title pattern with time windows, several duplicate-detection modes and a priority per rule.
- **ErsatzTV, Tunarr, dizqueTV** (the products Library Channels competes with): schedules built from collections with fixed-time or flowing items; **filler** (pre-roll, mid-roll, post-roll, fallback) and padding to round start times; watermarks; one FFmpeg per channel shared by all viewers, started on demand; and, importantly, they publish the channels as an **M3U playlist, an XMLTV guide and an emulated HDHomeRun** so Plex, Jellyfin, Channels or any IPTV app can consume them. Tunarr and dizqueTV also offer "on-demand" channels that pause when nobody is watching.

## Architectural differences

### A-11.1 One input per viewer, not per channel
- **Portico:** each playback session takes its own tuner allocation, opens its own upstream connection, runs its own FFmpeg and writes its own buffer (`S/playbackruntime/linear.go:226-266, 658, 717`). A recording opens another (`S/recordingmedia/driver.go:174`). `04 Playback and Media.md:45` says the input is shared per channel; the code does not do that.
- **Jellyfin / others:** one upstream connection per channel with a consumer count, shared by viewers and recordings (`jellyfin/src/Jellyfin.LiveTv/DefaultLiveTvService.cs:462-476`). ErsatzTV runs one FFmpeg per channel. Channels DVR and Plex share tuners between viewers of the same channel.
- **Why it matters:** with a two-tuner HDHomeRun, two people cannot watch the same channel while anything else records. An IPTV subscription with one allowed connection cannot be watched and recorded at once, even on the same channel. CPU and disk cost grow with viewers, which matters for the "couple of hundred browsers" target.
- **Assessment:** **Portico behind.** It looks accidental (the doc describes sharing). Adopting sharing is **large**: the allocation key changes from session to channel, the gateway needs reference counting, and the buffer and the copy-or-convert decision have to split into a shared copy stage and per-viewer conversion stages. A cheaper first step is to share the raw input (one gateway and allocation per channel) while keeping per-session FFmpeg.

### A-11.2 Always re-segmented HLS, never direct
- **Portico:** every live stream passes through server FFmpeg into 4-second MPEG-TS HLS segments on disk, even when nothing is converted.
- **Jellyfin:** may hand the provider URL to the client (direct play), remux, or transcode. Plezy plays whatever the server negotiates.
- **Why it matters:** Portico's way gives uniform behaviour, timeshift everywhere, no provider URL or credentials on clients, and one code path. It costs an FFmpeg and disk writes per viewer and several seconds of start-up (probe, then at least one 4-second segment).
- **Assessment:** **Trade-off (subjective).** Deliberate and consistent with thin clients and privacy. I would keep it. Shorter first segments and skipping the separate probe when the source was probed recently would cut channel-change time; both are **small to medium**.

### A-11.3 Server-side timeshift buffer per session
- **Portico:** a real seekable window with honest bounds published to the client (`S/linearbuffer/buffer.go:107-160`), default 30 minutes or 1 GiB per session. At 15 to 19 Mb/s (a copied broadcast stream) 1 GiB is about 7 to 9 minutes, so the byte cap usually wins.
- **Jellyfin:** no real timeshift; the shared stream is an ever-growing temp file and clients can rewind only as far as their own buffer. Plex has a capture buffer per tuner session that Plezy reads.
- **Why it matters:** pause and rewind work on every client, including browsers.
- **Assessment:** **Portico ahead** of Jellyfin, level with Plex. The depth and size are configuration values with no owner control I could find (see F-11.24).

### A-11.4 Narrow copy rule on live: H.264 with stereo AAC only
- **Portico:** copies only progressive 8-bit H.264 with AAC-LC stereo. MPEG-2, HEVC, interlaced H.264, AC-3 and 5.1 are always converted; audio always becomes 192 kb/s stereo; first video and first audio only; subtitles dropped (`S/playbackruntime/linear_plan.go:67-93, 181, 228`; `S/decoder/linear_hls.go:105-125`). The session API refuses audio and subtitle choices for channels (`S/playbackv1/channels.go:131-138`).
- **Jellyfin:** the normal device-profile decision applies, so AC-3 and HEVC are passed through to devices that support them, the viewer picks audio and subtitle tracks, and hardware encoders are used.
- **Why it matters:** North American over-the-air TV is MPEG-2 with AC-3 5.1; most European DVB is interlaced H.264 or HEVC. On Portico every such channel is a full video conversion per viewer and loses surround sound, second audio (descriptive audio, other languages) and DVB subtitles. In-band closed captions inside the video should survive, but I did not verify that.
- **Assessment:** **Portico behind.** Partly deliberate (one safe output tuple). Widening the rule to "copy what this client declared it can play in HLS-TS" (AC-3, E-AC-3, HEVC, multi-channel AAC) is **medium** and mostly confined to `linear_plan.go`. Track selection is **medium to large** because the buffer currently holds one muxed rendition.

### A-11.5 Guide stored as full-copy generations
- **Portico:** every refresh writes every channel and programme again under a new generation id, then flips a pointer; the old copy is deleted a day later.
- **Jellyfin:** programmes are library items updated in place; unchanged rows are skipped by etag; stale rows are deleted.
- **Why it matters:** Portico gets atomic publication, a guide that never goes blank, and trivially consistent paging. It pays in write volume: a 100,000-programme guide refreshed hourly writes 2.4 million programme rows a day and keeps up to 24 extra copies for the grace period.
- **Assessment:** **Trade-off (subjective)**, leaning Portico ahead for correctness. Skipping the publish when the fetched bytes are identical to the last ones (a hash compare) would remove most of the cost and is **small**. It would also avoid B-11.1 in the common case.

### A-11.6 All-or-nothing validation of provider data
- **Portico:** any malformed row rejects the whole playlist or guide (`S/livechannels/parse.go`). The owner sees a generic "invalid" or "source refresh failed".
- **Jellyfin:** skips the bad channel or programme, logs it and keeps going; its XMLTV reader accepts the date variants the format allows.
- **Why it matters:** provider files are messy. One bad line among thousands makes the source unusable on Portico, with no hint which line.
- **Assessment:** **Portico behind.** The contract asks for rows to be "rejected or quarantined with owner diagnostics" (`contract.md` §4.2.3); the code only rejects the document. Changing to per-row quarantine with counts and a few examples is **medium** and is the single most valuable robustness change in this domain. Details in B-11.2 to B-11.7.

### A-11.7 Hard size limits on sources
- **Portico:** 16 MiB for playlist plus guide, uncompressed; 2,000 channels; 100,000 programmes; 64 sources. The whole text is held in memory and parsed several times, and the previewed text is stored in SQLite.
- **Jellyfin:** streams the XMLTV file from a disk cache, no size limit; channel count unbounded.
- **Why it matters:** IPTV line-ups of 5,000 to 30,000 channels with 50 to 300 MB guides are common. A seven-day guide for 500 channels is already around 100,000 programmes.
- **Assessment:** **Portico behind**, and in tension with the product's own scale principle. A streaming parser that writes rows as it reads, with limits per channel rather than per document, is **medium to large**.

### A-11.8 Guide sources
- **Portico:** an XMLTV URL attached to an M3U or Xtream source. No local file path, no Schedules Direct, no guide for HDHomeRun in the UI.
- **Jellyfin:** XMLTV URL or file, and Schedules Direct, each attachable to any tuner or all tuners. Plex and Channels ship a commercial guide.
- **Why it matters:** an antenna user's usual choices are Schedules Direct or a locally generated XMLTV file. Neither works today.
- **Assessment:** **Portico behind.** See F-11.2 and F-11.3.

### A-11.9 Channel-to-guide mapping
- **Portico:** a channel gets guide data only when its `tvg-id` equals the XMLTV channel id (or the Xtream `epg_channel_id`). The parser supports explicit mappings (`parse.go:124-138`) but no client sends them. The XMLTV `<channel>` elements (names, icons) are skipped (`parse.go:177-182`). The web console's channel map dialog writes a different table that nothing reads (B-11.9).
- **Jellyfin:** automatic matching by id, number and normalised name, plus a mapping screen.
- **Why it matters:** many playlists have ids that differ slightly from the guide's. Those channels silently have no guide, and with no guide there is no recording.
- **Assessment:** **Portico behind.** Wiring the existing dialog to the existing mapping input, and proposing matches by name and number, is **medium**.

### A-11.10 Channel identity from `tvg-id`
- **Portico:** channel id = hash(source, `tvg-id`). Stable when the stream URL or token changes. Requires every channel to have a unique, non-empty `tvg-id`.
- **Jellyfin:** channel id = hash of the stream URL. Survives missing ids; breaks favourites and timers when a provider rotates tokens in URLs.
- **Why it matters:** Portico's choice keeps favourites, rules and recordings attached across token changes, which is the better default for IPTV. But it turns a missing or repeated `tvg-id` into a fatal error instead of a degraded channel.
- **Assessment:** **Trade-off (subjective).** Keep the idea; add a fallback identity (name plus URL path, or position) when `tvg-id` is empty or repeated. **Small to medium.**

### A-11.11 Programme identity from start time
- **Portico:** programme id = hash(source, channel, start time). Recordings and rules hold that id and re-find it in each new generation. A corrected start time is a different programme.
- **Jellyfin:** programme id is also channel plus start, but a timer keeps its own start and end once created.
- **Why it matters:** see B-11.13: a one-time recording is lost when the provider moves a programme by a few minutes.
- **Assessment:** **Portico behind in outcome**, though the rule "never retarget by title" is deliberate (`contract.md` §4.3, `architecture-amendments.md:83`). The contract's own wording asks for stable occurrence lineage across time corrections, which interval-only identity cannot give.

### A-11.12 Series identity
- **Portico:** a series rule needs a `seriesId`, which exists only if the guide has `<series-id>` or a `dd_progid` (`parse.go:252-265`; `dvr/rules.go:85`). There is no title-based rule.
- **Jellyfin:** falls back to the programme name, and for XMLTV derives the series id from the title whenever an episode number is present. Tvheadend matches titles by pattern.
- **Why it matters:** most IPTV guides have neither field, so "Record series" never appears for them.
- **Assessment:** **Portico behind.** Deliberate caution, but the result is that the feature is absent for most sources. An explicit, clearly labelled "record every programme with this exact title on this channel" rule would keep the honesty and restore the function. **Medium.**

### A-11.13 Source and stream types
- **Portico:** HTTP and HTTPS only, and the stream itself must be MPEG-TS or HLS (`S/linearinput/gateway.go:149-155`). HLS is limited to clear or AES-128 streams without low-latency tags (`linearinput/manifest.go:55-81`).
- **Jellyfin:** also `rtsp`, `rtp`, `udp`, local files and anything FFmpeg can open; legacy HDHomeRun over UDP; back ends through plugins.
- **Why it matters:** multicast IPTV from an ISP, SAT>IP boxes, and cameras need the other protocols.
- **Assessment:** **Trade-off (subjective).** Portico's restriction is what makes the sandbox and the SSRF policy enforceable. I would keep HTTP-only and suggest that owners bridge other protocols with a proxy, but say so in the product.

### A-11.14 Hostile-input posture
- **Portico:** provider data is treated as hostile throughout: DNS-pinned fetches, private-address confirmation, no XML directives, HLS URL rewriting so FFmpeg never sees a provider URL, FFmpeg sandboxed with a protocol and format whitelist, credentials never in any client response.
- **Jellyfin:** passes the provider URL to FFmpeg and, for direct play, to the client.
- **Why it matters:** a malicious playlist or guide cannot make Portico's server fetch internal addresses or read files.
- **Assessment:** **Portico ahead.** Worth advertising. The cost is the strictness discussed in A-11.6; the security goal does not require rejecting whole documents.

### A-11.15 Fixed user agent, no per-source request options
- **Portico:** every provider request sends `User-Agent: Portico/1` (`S/remotemedia/client.go:98`). A per-source user agent setting exists in the admin document but is not used.
- **Jellyfin:** a browser-like default and a per-tuner override.
- **Why it matters:** some IPTV providers block or throttle unknown agents, or require a specific one.
- **Assessment:** **Portico behind**, by omission. Wiring the stored setting through is **small**.

### A-11.16 Durable tuner allocations with reservations
- **Portico:** allocations and reservations live in SQLite with leases, fencing tokens and inherited file locks; a crash cannot leave a phantom tuner in use, and a restarted server sees a child still using one.
- **Jellyfin:** counts open streams in memory per tuner host.
- **Why it matters:** correctness after crashes and with several processes.
- **Assessment:** **Portico ahead** in rigour. The machinery is sound; what it counts (sessions rather than channels, A-11.1) is the weakness.

### A-11.17 Conflict policy
- **Portico:** deterministic: higher priority wins, then earlier start, then id. The loser of any overlapping interval fails outright at its start time (`dvr/worker.go:341-347`). Unknown capacity means no conflicts at all.
- **Jellyfin:** no planning; the recording that cannot get a tuner fails and is retried each minute. Plex shows conflicts and lets the owner reorder. Tvheadend drops padding before dropping a recording.
- **Why it matters:** determinism and an explanation ("lost to N higher-priority recordings between 20:59 and 21:03") are valuable. Failing a whole programme for a few minutes of padding overlap is not (B-11.10).
- **Assessment:** **Trade-off (subjective)** on the policy, **behind** on the padding consequence.

### A-11.18 DVR belongs to a profile
- **Portico:** rules, recordings and the Recorded TV library are per profile and private. Two profiles recording the same programme make two captures on two tuners.
- **Jellyfin:** timers and recordings are server-wide; anyone with the management flag sees and edits them.
- **Why it matters:** privacy between household members versus the normal household expectation that "the DVR" is shared, and double tuner and disk use.
- **Assessment:** **Trade-off (subjective).** Deliberate (`contract.md` §2). A middle path is to keep ownership per profile but store one capture when two profiles record the same occurrence, and to let an owner mark a recording or rule as shared. **Medium to large.**

### A-11.19 Scheduling engine
- **Portico:** one database-driven worker that sleeps until the next due time and wakes on any relevant commit; every step is restartable and fenced.
- **Jellyfin:** one in-process timer object per pending recording, reloaded from JSON at start.
- **Why it matters:** Portico's scales to thousands of scheduled items and survives restarts by construction.
- **Assessment:** **Portico ahead.**

### A-11.20 A recording follows the guide rather than a clock time
- **Portico:** until capture starts, a scheduled recording's window is recomputed from the current guide on every refresh; if the programme id is no longer in the guide the recording waits and then fails.
- **Jellyfin:** a timer has fixed times; guide changes update it when the programme is found, otherwise it records the original times.
- **Why it matters:** Portico follows a programme that is extended before it starts. It records nothing when identity is lost (B-11.13) and does not extend a recording already in progress.
- **Assessment:** **Trade-off (subjective).** Falling back to the last accepted window when the programme disappears would combine both strengths. **Small.**

### A-11.21 No retry or resume during a recording
- **Portico:** the first upstream error ends the capture; what exists is published as incomplete. After a server restart the partial is published and the remaining airtime is not captured.
- **Jellyfin:** up to ten restarts a minute apart until the programme ends, each writing a new file; after a restart the timer fires again.
- **Why it matters:** IPTV streams hiccup. See B-11.11 and B-11.12.
- **Assessment:** **Portico behind.** The amendment "do not restart an expired programme as if it could recover missed live bytes" (`architecture-amendments.md:89`) is about honesty, and Portico's completeness accounting already provides that. Continuing the capture as a further part of the same recording is compatible with it. **Medium.**

### A-11.22 Recording storage is content-addressed and private
- **Portico:** `<recordings root>/<profile hash>/captures/<recording id>/objects/<sha-256>`, read-only, raw MPEG-TS, no sidecar files.
- **Jellyfin / Plex / Channels:** `Series/Show (Year)/Season 02/Show S02E05 Title.ts`, with NFO files, in folders the owner chooses.
- **Why it matters:** an owner cannot browse recordings in a file manager, copy one to a laptop, or point another program at them. If Portico's database is lost the files are anonymous. This sits oddly beside the "open data" principle.
- **Assessment:** **Portico behind** for owner usability, ahead for integrity. The DVR settings document already defines folder templates and tokens but refuses any value (`S/administration/dvr_settings.go`, `validateDVRDefaults`), so the intent exists. See F-11.14.

### A-11.23 Recordings appear as "movies" with only the programme title
- **Portico:** each recording is a movie-kind item titled with the programme title, in a private "Recorded TV" library; the guide's sub-title, season and episode are known but not used (`dvr/publication.go:59, 82-88`). No metadata match, no artwork.
- **Jellyfin / Plex:** recordings are filed as series episodes or movies in normal libraries and matched to metadata providers.
- **Why it matters:** ten recordings of one show are ten identical tiles in the library. The DVR screen groups by series id, but Home, search and "continue watching" see the flat list.
- **Assessment:** **Portico behind.** The code comment says this is deliberate, to avoid inventing season and episode numbers. Using the sub-title and the "S2 E5" text in the display title when the guide provides them invents nothing. **Small.** Full episode filing is **large**.

### A-11.24 Honest completeness accounting
- **Portico:** records the first and last captured instant, gap count and whether the planned end was reached; distinguishes `completed`, `incomplete-playable` and `failed`.
- **Jellyfin:** a recording is "completed" if a file exists.
- **Why it matters:** the viewer knows a recording is truncated before sitting down to watch it.
- **Assessment:** **Portico ahead.** One calibration issue in B-11.20.

### A-11.25 Recording format and post-processing
- **Portico:** stream copy to MPEG-TS only. Conversion, remux and sidecars are declared in the settings vocabulary and refused.
- **Jellyfin:** raw copy for TS-over-HTTP, otherwise FFmpeg copy with optional audio re-encode; then an owner-defined post-processor. Emby converts automatically. Plex and Channels detect commercials.
- **Assessment:** **Portico behind** on features (F-11.15); the copy-only default is right.

### A-11.26 No watching a recording in progress
- **Portico:** the item is published only after capture ends. The spec already describes the design for this (`Spec — Channels and Guide.md` §8.3).
- **Jellyfin, Plex, Channels:** playable from the first minute.
- **Assessment:** **Portico behind**, known and planned. **Medium to large**, as the spec says.

### A-11.27 Cancelling an in-progress recording discards it
- **Portico:** Cancel marks the recording cancelled and a cleanup job deletes the captured bytes (`dvr/cleanup.go`).
- **Jellyfin:** stopping keeps the file.
- **Assessment:** **Portico behind**, small. A "Stop and keep" action that publishes the partial as incomplete reuses the existing recovery path. **Small.**

### A-11.28 Retention and disk guards
- **Portico:** per-rule retention days and episode cap, a global policy where the more protective value wins, Keep, deletion that waits for active readers, a free-space floor and an optional size cap checked on every write, and a 24-hour storage forecast.
- **Jellyfin:** "keep up to N" per series timer; no free-space guard.
- **Assessment:** **Portico ahead.**

### A-11.29 Series rule options
- **Portico:** new or all, allowed and blocked channels, required and blocked keywords, padding, priority, retention days, episode cap, a preview with match counts.
- **Jellyfin:** new only, any channel or one channel, any time or the original time slot, days of week, keep up to N, skip episodes already in the library.
- **Why it matters:** each side has options the other lacks. Portico lacks time-slot and weekday limits and duplicate skipping; Jellyfin lacks keywords, priorities and a preview.
- **Assessment:** **Trade-off (subjective).** Duplicate skipping is the one that matters (B-11.25).

### A-11.30 No recording without a programme
- **Portico:** a recording is always bound to a guide programme (`dvr/model.go:64-73`).
- **Jellyfin, Tvheadend, NextPVR:** a timer can be a channel plus a start and end time.
- **Why it matters:** channels with no guide (all HDHomeRun channels today, many IPTV channels) cannot be recorded at all.
- **Assessment:** **Portico behind.** See F-11.12.

### A-11.31 Guide API shape
- **Portico:** purpose-built windowed reads with a page revision; at most 50 channels and 24 hours per call; channel list separate from programmes; server-side sort with numeric channel-number ordering and proper collation.
- **Jellyfin:** a general programme query with many filters (is movie, is sports, has aired, genres, series timer id) and a channel list that carries the current programme.
- **Why it matters:** Portico's is bounded and fast for the grid. Jellyfin's supports discovery screens ("movies on tonight") from the same endpoint.
- **Assessment:** **Portico ahead** for the grid, **behind** for discovery (F-11.22).

### A-11.32 Guide grid rendering
- **Portico:** virtualised in both axes with a bounded cache, real-time layout so daylight-saving days are 23 or 25 hours wide, honest gap cells.
- **Jellyfin web:** renders 500 channels by 24 hours as DOM; pages of 500 with previous and next buttons.
- **Assessment:** **Portico ahead.**

### A-11.33 Guide filters and colour coding
- **Portico:** filters by source, group and favourites; the spec's category highlight is not in the web guide as far as my grep shows; no programme search.
- **Jellyfin:** filters for movies, series, sports, kids, news, favourites; colour-coded cells; indicators for new, repeat, live and HD. Plezy has programme search.
- **Assessment:** **Portico behind** on discovery inside the guide. **Small to medium** each.

### A-11.34 Categories kept as raw strings
- **Portico:** up to 16 category strings per programme, as the provider wrote them.
- **Jellyfin:** maps provider categories to four flags using owner-editable lists per listings provider.
- **Why it matters:** "Movies", "Film", "Spielfilm" and "Movie/Drama" are one concept. Any category filter or colour needs a mapping.
- **Assessment:** **Portico behind** if category features are wanted. **Small.**

### A-11.35 Favourite and hidden channels
- **Portico:** per profile on the server, with revisions and idempotent writes. The web shows "Hide channel" to owners only, but the stored state is the owner's own profile (B-11.29). There is no server-wide way to remove unwanted channels.
- **Jellyfin:** favourites per user; channels can be disabled per tuner by re-importing "favourites only" on HDHomeRun. Plezy keeps favourites locally.
- **Assessment:** **Trade-off (subjective)** for favourites; **behind** for line-up curation (F-11.23).

### A-11.36 Images
- **Portico:** channel logos come from `tvg-logo` through a background importer and are served by the server; XMLTV channel icons and Xtream `stream_icon` are ignored; programme images are imported and proxied, capped at 5,000 distinct URLs per generation.
- **Jellyfin:** logos from the tuner or the listings provider; programme images cached ahead for the next days.
- **Assessment:** roughly level; Portico misses two easy logo sources. **Small.**

### A-11.37 Parental controls on live content
- **Portico:** a per-profile Live TV switch, per-member channel lists, and guide masking by programme rating. Tuning is not blocked by the rating of what is on now (B-11.30). Recordings inherit the guide rating and are filtered like library items.
- **Jellyfin:** a Live TV access flag and "block unrated" categories that include live channels and programmes.
- **Assessment:** **Portico ahead** in granularity, with one gap.

### A-11.38 Guide refresh scheduling
- **Portico:** per source, 1 to 24 hours (the server accepts down to 5 minutes), leased, with back-off from 30 seconds to 32 minutes, event-driven wake-ups, manual "refresh now".
- **Jellyfin:** one daily task for everything; falls back to the cached XMLTV file on download failure.
- **Assessment:** **Portico ahead.**

### A-11.39 A new guide generation is treated as a new source by playback
- **Portico:** the active generation id is part of the playing session's identity, so a refresh retunes viewers (B-11.1).
- **Jellyfin:** a guide refresh never touches open streams.
- **Assessment:** **Portico behind.** Accidental coupling; the channel's stream address is what matters, not the guide copy it came from.

### A-11.40 Credentials at rest
- **Portico:** plain text in SQLite, by decision (`S/livechannels/store.go:88-92`). The contract still says "encrypted at rest" (`contract.md` §3.3).
- **Jellyfin:** plain text in XML configuration.
- **Assessment:** level. Recorded under doc/code disagreements.

### A-11.41 Xtream as a first-class source
- **Portico:** built in, but thin: it reads stream id, name, number and guide id only.
- **Jellyfin:** none built in (owners use the M3U export).
- **Assessment:** **Portico ahead** for having it; see F-11.20 for what is left on the table.

### A-11.42 HDHomeRun support depth
- **Portico:** HTTP line-up and tuner count; DRM channels skipped; discovery by UDP broadcast and SSDP that returns only an address (B-11.28); no guide; no legacy devices; no "favourites only"; no transcode profile for devices that have one.
- **Jellyfin:** all of those.
- **Assessment:** **Portico behind.** The guide gap is the serious one.

### A-11.43 Live session failure handling
- **Portico:** the session stays alive, reports a "recoverable" state with a code, and restarts its producer with back-off up to five times (B-11.17). The buffered window remains playable meanwhile.
- **Jellyfin:** the transcode dies and the client must reopen the stream (Plezy has its own retry for that).
- **Assessment:** **Portico ahead** in design, with a counter bug.

### A-11.44 Library Channels: schedule model
- **Portico:** rules plus weekly blocks, a seeded and reproducible generator, a rolling seven-day durable schedule published atomically, a preview that runs the real generator, templates, show rotation and marathon modes, dedup windows.
- **ErsatzTV / Tunarr:** comparable scheduling plus filler, padding to round times, and several schedule styles.
- **Assessment:** **Trade-off (subjective).** Portico's builder is tied to the library's own filter language, which is a real advantage. It lacks filler and time alignment (F-11.25).

### A-11.45 Library Channels: one FFmpeg per viewer
- **Portico:** each viewer gets a private FFmpeg reading the file in real time. Two viewers of the same channel are independent and may be converted differently for their devices.
- **ErsatzTV / Tunarr:** one process per channel, shared; normalised to one output format.
- **Why it matters:** Portico can copy the original stream when the device supports it (no forced normalisation), but only within the narrow rule in A-11.4, and cost grows per viewer.
- **Assessment:** **Trade-off (subjective).** Since Library Channel items are ordinary files, serving them through the normal on-demand path at an offset (as the spec's wording "VOD sessions for Library Channels" suggests) would give full track selection, subtitles and direct play. That is a design question for Justin.

### A-11.46 Library Channels are closed to other players
- **Portico:** playable only in Portico clients.
- **ErsatzTV, Tunarr, dizqueTV:** publish M3U, XMLTV and an emulated HDHomeRun.
- **Assessment:** **Trade-off (subjective).** Export conflicts with per-profile restrictions and with authenticated media. See F-11.25.

### A-11.47 Library Channels respect profile restrictions
- **Portico:** a restricted title is a slate for that viewer and cannot be tuned; library visibility is checked on every segment.
- **ErsatzTV / Tunarr:** no viewer identity at all.
- **Assessment:** **Portico ahead.**

### A-11.48 Settings that exist only as documents
- **Portico:** a parallel layer of live and DVR settings is stored, versioned and editable but mostly not consumed (B-11.9).
- **Jellyfin:** every tuner option is read by the tuner code.
- **Assessment:** **Portico behind** until wired. This is scaffolding ahead of implementation; the risk is that owners believe the settings work.

## Missing features and capabilities to consider

Sizes are rough: **small** = days, **medium** = one to three weeks, **large** = more.

### F-11.1 Share one input per channel
One upstream connection per channel, counted once against the tuner limit, used by every viewer and every recording of that channel. Jellyfin does it with a consumer counter and a shared temp file; ErsatzTV with one FFmpeg per channel. Value: tuner limits mean channels rather than people; "watch what is being recorded" costs nothing; back-to-back recordings on one channel never conflict. Size: large for full sharing, medium for sharing only the raw input. No conflict with principles. Opinion: the most important structural change in this domain, and the architecture doc already promises it.

### F-11.2 A guide for HDHomeRun, and a working mapping screen
Let an HDHomeRun source (and any source) take an XMLTV URL, and let the owner map each channel to a guide channel, with matches proposed by number and name. Jellyfin: `ChannelMappingOptions` and automatic matching. Value: without it antenna users have no guide and no DVR at all (B-11.8). Size: medium; the parser already accepts mappings and the web already has a dialog. Opinion: required before Live TV can be called supported for HDHomeRun.

### F-11.3 More guide sources
Schedules Direct (the standard paid guide for North American antenna and cable users), and a local XMLTV file path for owners who run their own grabber. Jellyfin has both. Size: Schedules Direct is medium to large (account, line-ups, daily deltas, image rules); a local file is small. Opinion: local file first; it unlocks every external grabber at once.

### F-11.4 Tolerant parsing with owner diagnostics
Skip bad rows, keep good ones, and report "3 channels skipped (no stream address), 41 programmes skipped (bad time)" with examples. Accept the date forms the XMLTV format allows, a missing `stop` (use the next programme's start), a DOCTYPE line, a byte-order mark, common legacy encodings, and overlapping programmes (trim, as the client already does when drawing). Jellyfin and every IPTV player do this. Size: medium. Principle check: fits "correctness over limits"; the security rules (no entity expansion, bounded sizes) stay. Opinion: highest value for effort in this audit.

### F-11.5 Large guides and gzip
Streaming parse to disk, per-channel limits, gzip detection by content. Size: medium to large. Opinion: needed for the stated scale principle; see A-11.7.

### F-11.6 Title-based series rules
"Record every programme titled exactly X on this channel (or any channel)", clearly labelled as title matching, with the existing preview. Jellyfin falls back to this automatically; Tvheadend uses patterns. Size: medium. Tension: the contract forbids fuzzy title matching for **reconciling** an existing recording; an explicit rule the viewer chose is a different thing, but Justin should decide.

### F-11.7 Skip episodes already recorded or already in the library
Use the episode id (`dd_progid`, or season and episode plus series) to avoid recording the same episode twice, and optionally skip episodes the library already has. Jellyfin: `SkipEpisodesInLibrary` and duplicate show-id handling (with a scar: generic ids ending `0000` are excluded, `DefaultLiveTvService.cs:699-703`). Size: small to medium. Opinion: needed for "all episodes" rules on channels that repeat.

### F-11.8 Reconnect during a recording, and resume after a restart
Retry the upstream with back-off until the planned end and append to the same recording as a further part with the gap accounted for. Jellyfin retries ten times. Size: medium. Opinion: essential for DVR trust.

### F-11.9 Soft padding
When padding (not the programme itself) is what overlaps, shorten the padding instead of failing a recording. Tvheadend and Plex behave this way. Size: small once conflicts are computed on programme time first. Largely unnecessary on a shared channel if F-11.1 exists.

### F-11.10 Watch a recording while it records
Already designed in `Spec — Channels and Guide.md` §8.3. Size: medium to large.

### F-11.11 Stop a recording and keep what was captured
Size: small (A-11.27).

### F-11.12 Manual and repeating timed recordings
Channel plus start and end time, optionally weekly, for channels without a guide. Jellyfin's timer model and every PVR back end allow it. Size: medium. Opinion: also the simplest safety net when guide identity fails.

### F-11.13 Record from the buffer
"Record this programme" while watching could include what the timeshift buffer already holds. Plex and Channels do this in effect because their capture is the buffer. Size: medium to large; natural after F-11.1.

### F-11.14 Readable recording files
Publish or hard-link each finished recording as `Recorded TV/Show (Year)/Season 02/Show - S02E05 - Title.ts`, or offer "export". The folder-template vocabulary already exists in `S/administration/dvr_settings.go`. Size: medium. Principle: supports "open data". Opinion: owners will expect it.

### F-11.15 Post-processing
In order of value: (a) commercial detection that produces skip markers, reusing the intro and credits marker system (Plex, Channels and Emby all have it); (b) remux to MKV or MP4; (c) an owner-defined command after each recording (Jellyfin `RecordingPostProcessor`). Size: (a) medium to large, (b) small to medium, (c) small. (c) conflicts mildly with decoder confinement and deserves a decision.

### F-11.16 Real metadata for recordings, and shared recordings
File recordings as episodes or movies with artwork and matching, and optionally into a library the whole household sees. Plex records straight into normal libraries. Size: large. Tension: per-profile privacy (A-11.18).

### F-11.17 Audio, subtitle and caption choice on live TV, and surround pass-through
Second audio programmes, descriptive audio, DVB subtitles (burned in on request, as Plex does and as Plezy exposes), AC-3 and E-AC-3 copy for devices that declare them. Size: medium to large.

### F-11.18 Start over
"Watch from start" for a live programme (from the buffer when it reaches back far enough, or from provider catch-up) and for a Library Channel programme. The spec defines `canStartOver` and a `startOver` start mode for Library Channels (`Spec — Channels and Guide.md` §5); the session API accepts only `live` for channels (`S/playbackv1/channels.go:131-135`). Size: small for Library Channels (the file is on disk), medium for live.

### F-11.19 IPTV catch-up
Xtream `tv_archive` and the M3U `catchup` attributes let a viewer play programmes from the past days. Common in IPTV players (TiviMate and similar). Size: medium. Opinion: optional; it also offers a way to "record" a missed programme.

### F-11.20 A fuller Xtream adapter
Read categories and use them as groups; read `stream_icon` as the logo; read the account status, expiry date and `max_connections` and offer the last as the stream limit; warn before expiry. The code already parses `category_id` and discards it (`S/livechannels/adapters.go:92`). Size: small. Opinion: cheap and makes Xtream sources look finished (today they have no groups and no logos).

### F-11.21 Per-source request options
Custom user agent (stored but unused today), referrer and extra headers, as carried by `#EXTVLCOPT` and `http-user-agent` attributes in playlists. Jellyfin has the user agent. Size: small.

### F-11.22 Discovery beyond the grid
Programme search across the guide; "On now" and "Starting soon" rows; movies, sports and kids on tonight; every airing of a show (Plezy's show schedule screen; Jellyfin's `Programs/Recommended`). Size: medium on the server (an index on title and category), small on clients. Principle: server-composed rows fit "thin clients".

### F-11.23 Line-up curation that applies to everyone
Owner-level hide, rename, renumber, regroup and custom logo per channel, and include or exclude by group, applied in the directory and guide queries. The data model and UI exist (B-11.9); the read side does not. Size: medium. Opinion: necessary for IPTV line-ups, where most owners want 50 of 5,000 channels.

### F-11.24 Owner control of timeshift depth and place
Buffer minutes, size cap and directory are runtime configuration only (`S/playbackruntime/linear.go:150-156`). A setting, plus a per-server total cap, would let owners with small disks or many viewers choose. Size: small.

### F-11.25 Library Channels extras
- Filler and bumpers between programmes, and rounding start times to :00 or :30 (ErsatzTV, Tunarr, dizqueTV). Size: medium.
- More item kinds (music videos, concerts, trailers as filler). Size: small to medium.
- Export as M3U, XMLTV and an emulated HDHomeRun so other apps can show the channels. Size: medium. Tension: it would need a token-in-URL stream that bypasses per-profile restrictions; a per-export "plays as this profile" token is one answer.
- One shared stream per channel when several people watch. Covered by F-11.1.
Opinion: filler and alignment first; export only if Justin wants Portico to be a channel source for other players.

### F-11.26 Tuner and stream status for the owner
Who is watching or recording what on which tuner, signal quality for HDHomeRun (`status.json`), and a "reset tuner" action (Jellyfin `Tuners/{id}/Reset`). An endpoint `GET /v1/admin/dvr/tuners` exists; I did not read what it returns. Size: small.

### F-11.27 The reservation countdown
The contract's "a recording needs this tuner in 60 seconds" event and on-screen countdown (`contract.md` §6.1). The server computes the boundary but does not publish it (B-11.27). Size: small to medium.

### F-11.28 Guide history and guide length settings
A setting for how many days of guide to keep and how far back, applied at import. `GuideDays` is stored and unused. Size: small.

### F-11.29 Recording rules that are not series
Team passes, "any programme whose title contains X", "all new movies in genre Y" (Channels DVR passes, Tvheadend auto-recordings). Portico's keyword fields exist only inside a series rule. Size: medium.

## Possible bugs and risks in Portico

None of these is listed in `Backlog.md`. "Confirmed by reading" means the code path is unambiguous; it does not mean I ran it.

### B-11.1 Every guide refresh retunes every live viewer of that source
- **What happens:** a refresh always publishes a new generation, even when nothing changed. The running session's check compares the generation it started with to the source's current one and reports "source changed"; the monitor stops the producer and a new one is started (new tuner allocation, new upstream connection, new probe, new FFmpeg). The viewer's stream stalls for the time that takes and continues after a discontinuity.
- **When:** at each refresh interval (default 6 hours, selectable down to 1 hour) and on every manual "refresh now", for all viewers of that source at once.
- **Evidence:** `S/playback/channel_sessions.go:288` and `:320`; `S/playbackruntime/linear_resolver.go:115` (the selection always carries the current generation); `S/playbackruntime/linear.go:590-613` ("stopped for a retune"); `S/livechannels/store.go:252-259` (new generation per save); `C/linear-api.ts:11`.
- **How the comparison surfaced it:** Jellyfin's guide refresh never touches open streams.
- **Confidence:** Confirmed by reading (the retune). The visible length of the stall needs a runtime check.
- **Severity:** High.

### B-11.2 A standard XMLTV file with a DOCTYPE line is rejected
- **What happens:** the parser returns "invalid" for any XML directive. `<!DOCTYPE tv SYSTEM "xmltv.dtd">` is a directive, and it is the second line of the XMLTV format's own example and of the output of the xmltv.org grabbers. The intent was to block entity tricks (the test uses a DOCTYPE with an ENTITY), but the rule also blocks the harmless form.
- **Evidence:** `S/livechannels/parse.go:160-161` and `:371-372`; `S/livechannels/store_test.go:179`. XMLTV DTD (fetched today) shows the DOCTYPE line in its example.
- **Confidence:** Confirmed by reading. How many of Justin's intended providers include the line is unknown; I believe Xtream's `xmltv.php` output commonly does, which would make Xtream sources fail to add, but I did not verify that.
- **Severity:** High.

### B-11.3 Valid XMLTV time forms, a missing `stop`, or a non-UTF-8 encoding reject the whole guide
- **What happens:** times must be exactly `YYYYMMDDhhmmss ±hhmm`. The format allows shorter forms and no time zone (UTC is then assumed), and `stop` is optional. Any such programme anywhere fails the document. Separately, Go's XML decoder is used without a charset reader, so a file declared as `ISO-8859-1` or `windows-1252` fails at the first token.
- **Evidence:** `S/livechannels/parse.go:222-225`; no `CharsetReader` anywhere in `S/livechannels` (grep). XMLTV DTD for the format.
- **Confidence:** Confirmed by reading.
- **Severity:** High.

### B-11.4 One odd programme rejects the whole guide
- **What happens:** the document is rejected if any programme overlaps the previous one on its channel, has an empty title or a title containing a line break or tab, lasts more than seven days, or carries both `<new/>` and `<previously-shown>`. Overlaps of a minute or two are routine in provider guides; the client layout code already trims them when drawing.
- **Evidence:** `S/livechannels/parse.go:224` (title, length), `:266`, `:320-325`.
- **Confidence:** Confirmed by reading.
- **Severity:** High for real-world guides.

### B-11.5 Size limits fail the whole source
- **What happens:** playlist plus guide over 16 MiB, more than 2,000 channels, or more than 100,000 programmes returns "invalid" or "unavailable". There is no partial import. About 500 channels with seven days of guide reaches the programme limit.
- **Evidence:** `S/livechannels/model.go:20-22`; `parse.go:33, 116, 295`; `adapters.go:34, 53, 96, 159, 180-185`.
- **Confidence:** Confirmed by reading.
- **Severity:** High for IPTV line-ups, low for an antenna.

### B-11.6 Gzip guide files are not decompressed
- **What happens:** requests send `Accept-Encoding: identity` and only a `Content-Encoding: gzip` response header triggers decompression. A URL such as `…/guide.xml.gz`, served as a gzip file with no content encoding (the normal case), is passed to the XML parser as binary and rejected.
- **Evidence:** `S/remotemedia/client.go:97`; `S/livechannels/adapters.go:39-51`.
- **How surfaced:** Jellyfin unzips by file extension (`XmlTvListingsProvider.cs`, `UnzipIfNeededAndCopy`).
- **Confidence:** Confirmed by reading.
- **Severity:** Medium (most public guide feeds are `.gz`).

### B-11.7 One imperfect playlist entry rejects the whole playlist
- **What happens:** the playlist is rejected if any channel has no `tvg-id`, if two channels share a `tvg-id` with any other field different (HD and SD variants often do), if any stream address is not `http` or `https`, contains `user:password@` or a `#fragment`, if a line exceeds 16 KiB, or if the file starts with a byte-order mark. For Xtream, one channel name containing a double quote fails the source.
- **Evidence:** `S/livechannels/parse.go:21-27, 37, 49, 87, 96, 101-112`; `adapters.go:113`.
- **How surfaced:** Jellyfin skips a bad entry and logs it, and falls back to `tvg-name` and the URL for identity (`jellyfin/src/Jellyfin.LiveTv/TunerHosts/M3uParser.cs:94-110, 279-320`).
- **Confidence:** Confirmed by reading.
- **Severity:** High.

### B-11.8 HDHomeRun sources cannot have a guide, so they cannot record
- **What happens:** the web form hides the guide address for HDHomeRun. Even through the API, HDHomeRun channels are keyed `hdhr-<number>`, which no XMLTV file uses, and no client can send mappings. With no programmes there is nothing to record, because a recording must reference a programme.
- **Evidence:** `W/screens/server/LiveTV.tsx:117`; `S/livechannels/adapters.go:173`; `C/linear-api.ts:11` (`mappings: []`, never edited: grep of `W/` and `N/src`); `S/livechannels/dvr/model.go:64-73`.
- **Confidence:** Confirmed by reading.
- **Severity:** High (the most common home tuner gets channel names only).

### B-11.9 Owner settings that are saved but do nothing
- **What happens:** the web console lets the owner edit, per source, the stream buffer, retry window, user agent, category, country and keyword filters, sequential renumbering, "use another source's guide", and a per-channel map with number override, guide channel id and hidden. On save it says "Changes to filters and numbering show after the next guide refresh." None of these values is read by the guide, directory, fetch or playback code. Of the server-wide defaults, logo import and tuner discovery are honoured (`S/administration/guide_image_worker.go:135`, `library_channels.go:668`, `live_sources.go:563-590`); guide days, stream buffer, retry window and user agent are not.
- **Evidence:** definitions in `S/administration/live_sources.go:17-91, 338-470`; UI in `W/screens/server/LiveSourceExtras.tsx:60-110, 143-201`; a grep for `admin_channel_numbers`, `StreamBufferSeconds`, `RetryWindowSeconds`, `GuideSourceID`, `Numbering` and `Filters` outside that file finds only the schema list. `S/remotemedia/client.go:98` always sends `Portico/1`.
- **Confidence:** Confirmed by reading.
- **Severity:** Medium (misleading; and it hides the absence of the mapping feature in B-11.8).

### B-11.10 A few minutes of padding overlap fails an entire recording
- **What happens:** with a known tuner limit, conflicts are computed over padded windows. At a recording's start, if any interval of its window is "lost" to higher-ranked reservations, the whole recording is marked failed with `tuner-conflict`. Default padding is 60 seconds before and 180 after. Example with one tuner or a one-connection IPTV account: programme A 20:00 to 21:00 and programme B 21:00 to 22:00. A's window ends 21:03, B's starts 20:59, A started earlier so A ranks higher, and B is failed at 20:59. The same happens for two consecutive episodes on the **same** channel, because recordings do not share an input.
- **Evidence:** `S/livechannels/dvr/worker.go:341-347`; `dvr/conflicts.go:13-61`; ranking in `S/livechannels/conflicts.go` (`reservationBefore`); defaults `dvr/store.go:39`.
- **How surfaced:** Jellyfin retries a recording that cannot get a tuner; Tvheadend and Plex treat padding as soft.
- **Confidence:** Confirmed by reading.
- **Severity:** High where a limit is set (every HDHomeRun using the discovered count, every IPTV source with an owner limit).

### B-11.11 The first upstream error ends a recording for good
- **What happens:** when the source connection drops or stalls, the gateway marks the input finished, FFmpeg sees end of stream, and the capture returns. The partial is published as `incomplete-playable` and nothing reconnects, however much of the programme remains.
- **Evidence:** `S/linearinput/gateway.go:200-235` (pump ends on any read error); `S/recordingmedia/driver.go:215-317`; `S/livechannels/dvr/worker.go:456-476` and `publication.go` (terminal state).
- **How surfaced:** `jellyfin/src/Jellyfin.LiveTv/Recordings/RecordingsManager.cs:403-412` retries ten times.
- **Confidence:** Confirmed by reading.
- **Severity:** High.

### B-11.12 A server restart during a recording loses the rest of the programme
- **What happens:** on start, a recording found in a capturing state goes to recovery, which validates and publishes the bytes already on disk and stops. The remaining airtime is not recorded.
- **Evidence:** `S/livechannels/dvr/worker.go:253-272, 451-452`; `S/recordingmedia/driver.go:319-355`.
- **Confidence:** Confirmed by reading. The amendment text suggests this is deliberate; the user-visible result is still a lost half-programme after any update or crash.
- **Severity:** Medium.

### B-11.13 A one-time recording is lost if the provider corrects the start time
- **What happens:** the programme id includes the start time. If a later guide refresh moves the programme (sport overruns, schedule corrections), the old id no longer exists. The recording becomes `waiting-guide`, the claim retries every three seconds, and at the old end time it is marked failed with `capture-window-missed`. Nothing is recorded, not even the originally accepted window. Series rules recover by discovering the moved programme as a new one; one-time recordings do not.
- **Evidence:** `S/livechannels/parse.go:279`; `occurrence.go` (`programmeIdentityTx`); `S/livechannels/dvr/scheduled.go:71-72, 143-190`; `dvr/worker.go:298-302`.
- **How surfaced:** Jellyfin timers keep their own times.
- **Confidence:** Confirmed by reading.
- **Severity:** High.

### B-11.14 HLS sources with several quality variants: recordings take all of them, live takes the first listed
- **What happens:** a recording maps every video and audio stream of the input. For an HLS master playlist FFmpeg exposes every variant, so the recording would download and store all qualities at once (and may exceed the gateway's limit of four concurrent requests). Live playback maps the first video stream, which is the first variant in the provider's list, not the best one.
- **Evidence:** `S/decoder/linear.go:36` (`-map 0:v? -map 0:a? -map 0:s?`); `S/decoder/linear_hls.go:105-110` (`-map 0:v:0`, `-map 0:a:0`); `S/linearinput/gateway.go:309-313`.
- **Confidence:** Probable (it rests on FFmpeg's stream mapping for HLS, which I did not run).
- **Severity:** High for recordings of such channels (several times the disk and bandwidth), Medium for live quality.

### B-11.15 On Android and Fire TV, live channels are probably always converted
- **What happens:** the Android app reports each decoder's maximum bitrate. When a device declares one, the live plan forces a conversion whenever the source's bitrate is unknown, and for a live MPEG-TS or HLS probe it normally is unknown. The target bitrate is then the device maximum (often 100 Mb/s or more); if a device reports more than 200 Mb/s the FFmpeg argument builder rejects the plan and the channel cannot start.
- **Evidence:** `N/android/app/src/main/java/tv/getportico/portico/native/PorticoCapabilities.kt:146, 162`; `C/client-profile.ts:495`; `S/playbackruntime/linear_plan.go:185-194`; `S/decoder/linear_hls.go:73`.
- **Confidence:** Probable — needs a runtime check on an Android device.
- **Severity:** Medium to High on those platforms (CPU per viewer, large segments, short timeshift).

### B-11.16 An unknown frame rate forces a conversion
- **What happens:** if the probe reports no average frame rate (`0/0`, which happens with live streams), the plan sets a frame-rate cap, and a set cap is one of the conditions for converting video. A compatible H.264 stream is then converted instead of copied.
- **Evidence:** `S/playbackruntime/linear_plan.go:143-151, 181`.
- **Confidence:** Speculative — needs a runtime check with real sources.
- **Severity:** Medium.

### B-11.17 Live recovery stops after five failures and the counter does not reset
- **What happens:** each producer failure increments `attempts`. At five, the loop only waits. The counter is reset by a viewer's retry, by a programme change on a Library Channel, or by a new guide generation; it is not reset after a long healthy period. A viewer who leaves a flaky channel on will, after the fifth hiccup within one guide interval, sit on an error until someone presses retry.
- **Evidence:** `S/playbackruntime/linear.go:330-373, 416`.
- **Confidence:** Confirmed by reading.
- **Severity:** Medium.

### B-11.18 The segment writer has no tolerance for a slow database
- **What happens:** every segment FFmpeg uploads to the buffer runs an authority check that reads the database. Any error there answers FFmpeg with "gone", which ends the producer. The 500 ms monitor was given patience for exactly this case after a demo incident (comment at `linear.go:626-630`); the upload path was not.
- **Evidence:** `S/linearbuffer/buffer.go:354`; `S/playbackruntime/linear.go:737`; `S/playback/channel_sessions.go:279-293`.
- **Confidence:** Speculative — needs a load test.
- **Severity:** Medium.

### B-11.19 The HLS gateway refuses playlists with common extra tags
- **What happens:** a provider playlist is rejected if it contains low-latency tags (`EXT-X-PART`, `EXT-X-PRELOAD-HINT`, and others) or any unrecognised tag whose text contains `URI=` or `URL=`. Ad-marker tags on free ad-supported channels (for example `EXT-X-DATERANGE` with an asset URL) match the second rule, so the channel would fail whenever an ad break is announced.
- **Evidence:** `S/linearinput/manifest.go:55-56, 98-103`.
- **Confidence:** Probable for the rule; which providers trigger it needs real playlists.
- **Severity:** Medium. Ignoring such tags (dropping the line) keeps the security goal.

### B-11.20 "Complete" is judged against the padded window with a four-second tolerance
- **What happens:** a recording is complete only if the first byte arrived within four seconds of the padded start and the duration is within four seconds of the padded length. The worker wakes at the padded start and then has to claim, connect, and wait for the source to deliver. A slow provider, an HDHomeRun that takes a few seconds to lock, or a live viewer being pre-empted (ten-second grace) makes the recording `incomplete-playable` and adds "the full programme was not captured" to its description, although the programme itself, inside the padding, is all there.
- **Evidence:** `S/recordingmedia/driver.go:445-453`; `S/livechannels/allocator.go:143-162`.
- **Confidence:** Probable — needs timing on real sources.
- **Severity:** Low to Medium (trust in the label).

### B-11.21 Measuring storage stalls every running capture
- **What happens:** `Measure` walks the entire recordings folder while holding the same lock every capture takes for each write. It runs when the owner opens the storage page and after every deletion. With thousands of recordings on a slow or sleeping disk the walk can take seconds; all captures pause, and an HDHomeRun, which sends in real time, drops data.
- **Evidence:** `S/recordingmedia/storage.go:31-95` (walk at `:45`), `:126-159` (write under the same lock); callers `driver.go:480`, `S/livechannels/dvr/storage.go` (`StorageStatus`).
- **Confidence:** Confirmed by reading (the locking); the size of the stall is a runtime question.
- **Severity:** Medium, latent.

### B-11.22 A full disk at start time fails the recording before retention can help
- **What happens:** the free-space check runs before the claim and a failure is terminal (`storage-floor`). Retention and deletion run later in the same pass, so space that was about to be freed does not save the recording.
- **Evidence:** `S/livechannels/dvr/worker.go:181-184, 326-334, 115-121`.
- **Confidence:** Confirmed by reading.
- **Severity:** Low to Medium.

### B-11.23 Each finished recording is hashed twice, one result discarded
- **What happens:** `validateTransport` reads the whole file, checks packet framing and computes a SHA-256 that is thrown away; sealing hashes it again. For a three-hour broadcast recording that is tens of gigabytes of extra reading while other captures run.
- **Evidence:** `S/recordingmedia/driver.go:495-531` (discard at `:529`).
- **Confidence:** Confirmed by reading.
- **Severity:** Low.

### B-11.24 `dd_progid` gives movies and one-off programmes a "series"
- **What happens:** the series id is the first ten characters of any `dd_progid`, including `MV…` (movies), `SP…` (sports) and `SH…` (shows without episodes). "Record series" is then offered for a film, and a rule on it matches only reruns of that film. Generic episode ids ending `0000` are treated as real episodes.
- **Evidence:** `S/livechannels/parse.go:258-264`.
- **How surfaced:** the scar at `jellyfin/src/Jellyfin.LiveTv/DefaultLiveTvService.cs:699-703`.
- **Confidence:** Confirmed by reading.
- **Severity:** Low.

### B-11.25 An "all episodes" rule records every airing of every episode
- **What happens:** one recording is created per matching programme. Nothing compares episode ids, so a channel that shows the same episode three times a day yields three recordings, each on its own tuner, until the episode cap deletes the oldest.
- **Evidence:** `S/livechannels/dvr/reconcile.go:144-195`; `dvr/model.go:127-160`.
- **Confidence:** Confirmed by reading.
- **Severity:** Medium.

### B-11.26 Two profiles recording the same programme use two tuners and store it twice
- **What happens:** recordings are per profile and each opens its own input.
- **Evidence:** `S/livechannels/dvr/model.go:44-46`; `dvr/worker.go:352`.
- **Confidence:** Confirmed by reading. A consequence of two deliberate choices (A-11.1, A-11.18).
- **Severity:** Medium on a tuner-limited source.

### B-11.27 The pre-emption countdown is not delivered
- **What happens:** the server works out when a live session must end for a recording and shortens its lease, but that time never leaves the allocator. No event, no field in the session view. The viewer's stream simply fails with `tuner_reservation_changed`.
- **Evidence:** `PreemptAt` and `preempt_at_ms` appear only in `S/livechannels/allocator.go`; no client references a reservation or countdown (grep of `C/`, `W/`, `N/src`).
- **Confidence:** Confirmed by reading.
- **Severity:** Medium (the contract makes the countdown a condition of pre-emption).

### B-11.28 HDHomeRun discovery returns addresses only, and "already configured" is never true
- **What happens:** the discovery result has fields for model, name, tuner count and firmware, and the comment says each responder's `discover.json` is read, but only the address is filled. `Configured` looks up the device id (which is set to the IP address) in a set of source ids (48-character hex), so it is always false.
- **Evidence:** `S/administration/live_sources.go:559-613` (lookup at `:608`), `:678`.
- **Confidence:** Confirmed by reading.
- **Severity:** Low.

### B-11.29 "Hide channel" is offered to owners but hides only for the owner's own profile
- **What happens:** hidden state is per profile on the server. The web shows Hide to owners only, which reads as "remove this channel for everyone". Other profiles still see it, and members have no way to hide channels for themselves.
- **Evidence:** `S/livechannels/preferences.go`; `W/screens/channels/Channels.tsx:239, 287`.
- **Confidence:** Confirmed by reading.
- **Severity:** Low to Medium.

### B-11.30 A programme the profile may not see can still be watched
- **What happens:** with content restrictions, the guide replaces a too-mature programme's title with "Not available on this profile", but starting the channel checks only the Live TV switch, source access and channel lists, not the rating of what is on. Library Channels do block the tune.
- **Evidence:** `S/httpapi/live_channels.go:377-381`; `S/playbackruntime/linear_resolver.go:92-135` (no rating check), compared with `:174` for Library Channels.
- **Confidence:** Confirmed by reading.
- **Severity:** Medium for a parental-control promise; arguably intended, since ratings in guides are unreliable and channel lists exist. Worth a decision.

### B-11.31 The channel list sorts the whole line-up for every page
- **What happens:** each page request builds the full filtered channel set, counts it, hashes all of the profile's preferences for the revision, and sorts with a Go collation callback before applying limit and offset. Fine at today's cap of 2,000 channels per source; it is O(all channels) per page, not O(visible), and would matter if the caps are raised to IPTV sizes. The guide's search filter is an unindexed substring scan for the same reason.
- **Evidence:** `S/livechannels/directory.go:125-139, 242-315, 379-389`; `store.go:592`.
- **Confidence:** Confirmed by reading; performance not measured.
- **Severity:** Low now, a design trap for A-11.7.

### B-11.32 Hourly refresh keeps up to a day of full guide copies
- **What happens:** replaced generations are kept 24 hours "for requests that resolved them just before the swap". At an hourly refresh that is about 24 full copies of every programme row in the main database; at the server's minimum of five minutes it would be 288.
- **Evidence:** `S/livechannels/generation_retention.go:19-27`; `remote.go:96` (300 to 86,400 seconds).
- **Confidence:** Confirmed by reading.
- **Severity:** Low to Medium (database size and backup time). A grace of a few minutes would do, since DVR keeps its own copy of what it schedules.

### B-11.33 A slow import can outlive its refresh lease
- **What happens:** a refresh takes a two-minute lease, may spend 75 seconds fetching, then parses and writes the generation in hundreds of small background-priority transactions, each of which rechecks that the lease is still valid. On a busy or slow server a large guide can pass two minutes, the publish is refused, the source is marked degraded and the next attempt repeats the work.
- **Evidence:** `S/livechannels/remote.go:413, 427, 439`; `store.go:313-341`.
- **Confidence:** Speculative — needs a timing test with a guide near the limits.
- **Severity:** Low to Medium.

### B-11.34 Time ruler labels in zones offset by 45 minutes
- **What happens:** ruler ticks are placed on UTC half-hours and labelled "hour" or "day" only when they fall on a local whole hour or midnight. In zones such as Nepal (+5:45) or the Chatham Islands (+12:45) no tick ever does, so there are no emphasised hours and no day-boundary labels.
- **Evidence:** `C/guide/time.ts` (`rulerTicks`).
- **Confidence:** Confirmed by reading.
- **Severity:** Low.

### B-11.35 The default timeshift window is shorter than it sounds at broadcast bitrates
- **What happens:** 30 minutes or 1 GiB, whichever comes first. A copied 15 to 19 Mb/s stream fills 1 GiB in 7 to 9 minutes. The client is told the true window, so nothing is wrong on screen, but the practical rewind depth depends on bitrate.
- **Evidence:** `S/playbackruntime/linear.go:150-155`; `S/linearbuffer/buffer.go:284-294`.
- **Confidence:** Confirmed by arithmetic.
- **Severity:** Low (a tuning decision).

## Where Portico is stronger

- **The guide never goes blank or half-loaded.** Atomic generations, a last-good guide on failure, and a separate "guide is stale" state.
- **Provider data is treated as hostile.** DNS-pinned fetches, confirmation for private addresses, no provider URL or credential ever reaches a client or FFmpeg, FFmpeg sandboxed with protocol and format whitelists.
- **A guide that costs what is on screen.** Windowed channel and programme reads, bounded client caches, real-time layout that is correct on daylight-saving days. Jellyfin's web guide loads 500 channels by 24 hours.
- **Real timeshift on every client**, with bounds the server states and the client obeys.
- **Crash-safe tuner accounting.** Leases with fencing tokens plus file locks held by the FFmpeg child.
- **A restartable, database-driven DVR.** No in-process timers; workers sleep until the next due time and wake on commits; every mutation is idempotent with a request id and an expected revision.
- **Honest recordings.** Per-second checkpoints, captured-interval and gap accounting, and a clear difference between complete, incomplete and failed.
- **Disk protection.** A free-space floor and size cap enforced on every write, retention that waits for active viewers, Keep.
- **Deterministic, explainable conflicts**, and a rule preview that shows what a rule would match before saving.
- **Per-profile privacy** for favourites, rules and recordings, a per-profile Live TV switch enforced on every segment, and per-member channel lists.
- **Xtream built in.**
- **Library Channels inside the server**, built with the library's own filter language, reproducible schedules, a true preview, templates, and per-profile content restrictions that no stand-alone channel tool has.
- **Sources named by the owner, never by technology**, and Live TV and Library Channels in one guide.

## Doc/code disagreements

1. `04 Playback and Media.md:45` says live playback "shares one input per channel across viewers (`linearinput`)". The code opens one input per session and one per recording (A-11.1).
2. `contract.md` §3.3 says source secrets are "encrypted at rest". They are plain text by a later decision (`S/livechannels/store.go:88-92`, `apps/portico-server/server/cmd/server/live_channels.go` comment). The contract is stale.
3. `contract.md` §4.2.3 says invalid rows are "rejected or quarantined with owner diagnostics". The code rejects the whole document and records only `source-refresh-failed`.
4. `contract.md` §4.1 says "owner channel mapping explicitly connects tuner channel to guide channel". The only mapping screen writes a table nothing reads (B-11.9).
5. `contract.md` §6.1 and §16 require a typed `reservation-required` event and a visible countdown before a live session is pre-empted. Not implemented (B-11.27).
6. `contract.md` §9 lists conflict resolution actions the UI must offer (prefer this, lower another, edit padding). I found the conflict intervals in the API but did not verify the screens; flagging for a check.
7. `Spec — Channels and Guide.md` §1.6 says Library Channels play as "VOD sessions". They play as channel sessions through the linear runtime.
8. The same spec, §5 and §8.1, defines `allowRestart`, `canStartOver` and `GET /v1/channels/{id}/now`. None exists; channel starts accept only `live` (`S/playbackv1/channels.go:131-135`). The session code lists `startOver` as a start mode for other targets only.
9. The same spec, §8.1, names target endpoints `/v1/channel-sources`, `/v1/channels` and `/v1/guide/programs`. The built ones are `/v1/guide/sources`, `/v1/guide/channels` and `/v1/guide?channels=…`. The spec still describes the older adapter as the stopgap.
10. The same spec, §2.2: a "Showing: Everything" category menu, and "Hide channel: owner and admin only". No category control in the web guide (grep), and hide is per profile (B-11.29).
11. `S/administration/live_sources.go:559-561` says discovery is "followed by a read of each responder's discover.json". It is not (B-11.28).
12. `N/src/screens/NOTES-live.md` describes files (`LiveTV.tsx`, `Channels.tsx`), a stubbed tune and missing recording actions that no longer match `N/src/screens/channels/`. Stale.
13. `S/administration/dvr_settings.go` publishes folder-template tokens, conversion modes and sidecar options as vocabulary and then refuses every non-default value. Honest in the API, but the documents suggest features that do not exist.
14. `contract.md` §5 requires guide search. The server has a channel-name search parameter; the shared guide adapter always sends an empty search (`C/guide/legacy-source.ts:153, 212`).

## Open questions for Justin

1. Should a tuner be used per **channel** (shared by everyone watching or recording it) rather than per viewer? This decides A-11.1, B-11.10, B-11.26 and much of the DVR's reliability on small tuner counts.
2. Is the DVR meant to be private per profile, or is "the household DVR" (shared recordings, one capture) the expected product? Or private by default with an owner option to share?
3. When a provider file has bad rows, do you want Portico to import the good rows and report the bad ones? (I recommend yes.)
4. What line-up size must work: an antenna (under 100 channels), or IPTV accounts with 10,000 or more channels and 100 MB guides? The current limits assume the first.
5. For recordings whose programme vanishes or moves in the guide: record the originally accepted time window anyway, or keep the current "never guess" behaviour and lose the recording?
6. Do you accept a clearly labelled title-based series rule for guides without series ids?
7. Should recordings be readable files in folders an owner can browse (and survive losing the database), or stay as private content-addressed objects?
8. Should recordings be filed as episodes with metadata in normal libraries, or remain a separate "Recorded TV" area?
9. How far should live TV go on audio: is stereo-only acceptable, or are AC-3 pass-through and a second-audio choice required for release?
10. Should a restricted profile be prevented from tuning a channel while a too-mature programme is on, or is the guide masking plus channel lists the intended control?
11. Library Channels: are they Portico-only by design, or should other apps be able to consume them (M3U, XMLTV, HDHomeRun emulation)? And do you want filler and round start times?
12. Should Library Channels play through the normal on-demand path at an offset (full track choice, subtitles, direct play, start over) instead of the live pipeline?
13. The inert settings (B-11.9): wire them up, or remove them from the console until they work?
14. Is Schedules Direct in scope, or is "bring your own XMLTV" the position?
15. Commercial detection for recordings: wanted, and if so as skip markers only?

## Evidence boundary

- Nothing was executed. I did not run the server, FFmpeg, any client, or any test.
- FFmpeg behaviour that findings depend on (stream mapping for HLS master playlists, what a live probe reports for bitrate and frame rate, how the HLS muxer reacts to a refused upload) is from knowledge of FFmpeg, not observation.
- I have no real provider files. Statements about what "typical" playlists and guides contain come from the XMLTV DTD (fetched today) and general knowledge. Testing three or four of Justin's real sources against B-11.2 to B-11.7 would settle their practical weight quickly.
- No device behaviour was observed: how long a retune stalls a player, whether captions survive, how Android reports decoder bitrates in practice.
- Performance was not measured: import time for large guides, directory query time, the storage walk, timeshift disk use.
- Not read: `dvr/query.go` and `presentation.go` (conflict presentation), most of `livechannels/library` candidate selection and templates, the web and native guide grid, DVR and mini-guide components, the Roku and Vega clients, the cast receiver, and what `GET /v1/admin/dvr/tuners` returns.
- One unverified suspicion, not listed as a bug: deletion checks for active viewers by reading the `playback_sessions` table (`S/livechannels/dvr/retention.go`, `readersTx`); I did not confirm that Playback Protocol v1 sessions always have a row there. If they do not, a recording could be deleted under a viewer, with only the artifact lease as a backstop.
- Comparison coverage is uneven: Jellyfin's server and web guide were read in the areas cited; jellyfin-androidtv, Swiftfin, Wholphin and Streamyfin live TV were not opened; Plezy was sampled; closed products are from memory of public documentation.

Sources fetched today: [XMLTV DTD](https://raw.githubusercontent.com/XMLTV/xmltv/master/xmltv.dtd); search results on ErsatzTV's IPTV and HDHomeRun emulation, including [ErsatzTV IPTV docs mirror](https://www.mintlify.com/ErsatzTV/ErsatzTV/streaming/iptv) and [Make Tech Easier on ErsatzTV](https://maketecheasier.com/ersatztv-turn-media-library-into-tv-channels).
