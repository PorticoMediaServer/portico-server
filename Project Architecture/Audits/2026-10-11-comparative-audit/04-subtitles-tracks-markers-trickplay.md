# 04 — Subtitles, track selection, chapters, skip markers, trickplay and media analysis

Auditor domain 04 of the 11 October 2026 comparative audit. Read-only source review; nothing here was run. Portico paths are relative to `/home/justin/projects/Portico`; comparison paths are written `jellyfin/…`, `plezy/…` and so on.

Jargon used below, defined once:

- **Sidecar** — a subtitle file sitting beside the video (`Movie.en.srt`).
- **Embedded track** — a subtitle or audio stream inside the video container.
- **Text subtitles** — SRT, WebVTT, ASS/SSA: words plus timing (ASS also carries fonts, positions, colours and animation).
- **Image (bitmap) subtitles** — PGS (Blu-ray), VobSub (DVD), DVB: pictures of text. They can only be shown by drawing the picture, or after **OCR** (optical character recognition: software that reads the pictures back into text).
- **Burn-in** — the server draws the subtitle into the video frames while re-encoding. It forces a full video transcode.
- **Forced track** — a short track that only translates foreign-language lines and signs.
- **SDH / CC** — subtitles for the deaf and hard of hearing, including sound descriptions.
- **Trickplay** — the small preview pictures shown while scrubbing the seek bar.
- **Marker / segment** — a time range such as intro, recap or credits that a player can offer to skip.
- **Absolute vs per-type stream index** — FFmpeg numbers every stream in a file 0,1,2… (absolute). "The second subtitle track" is a per-type index. Mixing the two is a classic source of wrong-track bugs.

## Scope and sources

**Portico, read in full or nearly so**

- Server: `apps/portico-server/server/internal/subtitles/*` (all non-test files), `internal/subtitlevideo/*`, `internal/segmentmarkers/markers.go`, `internal/mediaanalysis/{detectors,markers,produce,trickplay,service,viewer_markers,work}.go`, `internal/probefacts/*`, `internal/mediatimeline/interval.go`, `internal/decoder/{subtitle,burn_in,analysis}.go`, `internal/mediaexec/fonts.go`, `internal/assets/{assets,chapters}.go` (probe and chapter parsing), `internal/ingestion/analysis.go` (scan-time subtitle work), `internal/playback/{subtitle_plan,subtitle_delivery,hls_burn,hls_text,hls_renditions,chapters,delivery_context,delivery_decision}.go`, `internal/playbackv1/{subtitles,options}.go`, `internal/httpapi/{subtitle_routes,trickplay,playback_chapters}.go`, `internal/administration/library_settings.go`, `internal/catalog/library_sources.go`, `internal/operations/preferences.go`.
- Client core: `packages/client-core/src/{subtitles,segment-markers,pending-track-choice,player-chapters,trickplay}.ts` (and skimmed `audio-selection.ts`, `media-analysis.ts`).
- Web: `web/src/player/{Player.tsx,options.ts,SkipPrompt.tsx,LiveCaptions.tsx}`, `web/src/bridge/useWebPlaybackAdapter.ts` (subtitle timeline part).
- Native: `apps/portico-react-native/apps/native/Native/{PorticoSubtitleSelection.m,PorticoSubtitleFiles.m}`, the Android `PorticoSubtitleSelection.kt` header, `src/player/{VideoPlayer,SkipPrompt}.tsx` (grep-level), preference usage.
- Docs: `Project Architecture/04 Playback and Media.md`, `Backlog.md`, `Specs/Spec — Playback Protocol v1.md` (§3–§6, §4.2–4.4, §5.4–5.5), `Product Contracts/playback/contract.md`, `accessibility-and-language/contract.md` (A11Y-10), `media-presentation/contract.md` (player sections), `decisions.md`.

**Comparison projects** — listed under "How the others do it".

**Not reached** — see "Evidence boundary" at the end.

## How Portico does it today

### 1. Two kinds of subtitle record

Portico keeps two separate lists per video file ("source"):

1. **Discovered tracks** (`asset_subtitles`): what the scan found — embedded streams from the probe and sidecar files beside the video. These are only facts ("stream 4 is PGS, English"). They cannot be played directly.
2. **Subtitle resources** (`subtitle_resources` + immutable `subtitle_revisions`): tracks the server has actually **imported** into its own content-addressed store (`mediaartifact`). Only resources can be selected in the player. A resource is either `shared` (owner-made, everyone sees it) or `personal` (one viewer). Each resource has an origin (`embedded`, `sidecar`, `upload`, `provider`), a renderer (`external_text` or `burn_in`), a stored per-track timing offset, default/forced flags, and revision history. (`internal/subtitles/service.go:156-190`, `:237-345`)

To turn a discovered track into a resource, something calls **Import** (`internal/subtitles/acquisition.go:34`). That happens two ways:

- **At scan time, automatically**, for the first eight *embedded text* tracks of a file (`internal/subtitles/scan_text.go:21-56`, called from `internal/ingestion/analysis.go:454`). Sidecar files and image tracks are not auto-imported.
- **On demand**, when a viewer picks a not-yet-imported track in the player; the client imports it as a *personal* resource and then selects it (`packages/client-core/src/subtitles.ts:186-188`, `web/src/player/Player.tsx:545`).

### 2. Discovery rules

- **Embedded**: the scan probe (`internal/assets/assets.go:150-232`) records each stream's absolute FFmpeg index, codec, language tag, title, and the `default`, `forced`, `hearing_impaired` and `comment` dispositions. Codec mapping is in `subtitles.CodecFormat` (`internal/subtitles/inventory.go:187-207`): `subrip`/`text`/`mov_text`→srt, `webvtt`→vtt, `ass`, `ssa`, `hdmv_pgs_subtitle`→pgs, `dvd_subtitle`→vobsub, `dvb_subtitle`→dvb. Anything else is listed with reason `unsupported_format`.
- **Sidecar** (`internal/subtitles/inventory.go:39-173`): only the video's own folder is listed (max 4,096 directory entries). A file matches when its name without extension equals the video's name, or starts with the video's name plus a dot. Extensions accepted: `.srt .vtt .ass .ssa .sup .idx` (`.sub` is only ever read as the partner of an `.idx`). Tokens after the video name are parsed: `forced` → forced flag; `sdh`/`cc` → the title "SDH"/"CC"; the first other token → language; a second other token → the track is disabled as `ambiguous_association`. If another *video* with the same base name exists in the folder (`Movie.mkv` + `Movie.mp4`), every sidecar is disabled as ambiguous.
- **Limits**: at most 32 tracks per file (embedded + sidecar). Over that, the whole inventory is replaced by status `capacity` with **no tracks** (`inventory.go:44-48`, `:77-79`).
- Each sidecar is read and validated during the scan (4 MiB limit for text, 32 MiB for `.sup`/`.sub`), and fingerprinted.

### 3. Text handling and encodings

- SRT and WebVTT go through a **tolerant parser** (`internal/subtitles/tolerant.go`): it accepts UTF-8, UTF-16 with a byte-order mark, and otherwise assumes **Windows-1252**; strips HTML-like tags and ASS override blocks; keeps two presentation facts per cue — *whole cue italic* and *placed at top*; sorts cues; drops cues past the end of the film (+2 s); caps at 20,000 cues, 8 KiB per cue.
- ASS/SSA goes through a **strict validator** first (`internal/subtitles/render_asset.go:150-218`): UTF-8/UTF-16-with-BOM only; rejects scripts containing `[Fonts]` or `[Graphics]` sections; rejects the whole script if any one Dialogue line has a malformed time, zero or negative length, or ends more than 2 s after the film. If it passes, the default ("text" render) converts it to plain cues with `ParseASSAsText` (drops drawings, collapses duplicate layered lines). "styled" render keeps the script for burn-in.
- The stored form of a text track is a small JSON **cue document** (`{version, timeDomain:"source-relative", cues:[{startUs,endUs,text,italic?,top?}]}`), not SRT or VTT (`internal/subtitles/document.go:28-43`, `:129-143`).

### 4. Extraction

Embedded tracks are extracted **one stream per FFmpeg process** through a private loopback HTTP bridge to the file (`internal/subtitlevideo/runtime.go:206-230`, `internal/decoder/subtitle.go:43-74`): `-copyts -start_at_zero -map 0:<absolute index> -c:s srt|webvtt|ass` for text (4 MiB output cap), or `-c:s copy -f matroska` for PGS/VobSub/DVB (32 MiB cap) — image tracks are kept as a little Matroska "capsule". Containers allowed for extraction: mov/mp4, Matroska, AVI, MPEG-TS. Extraction is unavailable on Windows (`service.go:64-68`).

The scan-time import of all text tracks of one file shares a single 90-second budget and a single global slot (`scan_text.go:25-32`).

### 5. Delivery to players

Three paths exist:

1. **Cue document over a grant URL** — `GET /v1/media/{grant}/subtitles/{resource}/{revision}` returns the JSON cue document. First-party clients fetch it, re-fetch every ~25 s purely to stay authorised (the server answers `304` when unchanged), and draw the text themselves: the web app feeds `VTTCue`s into a programmatic text track (`web/src/player/options.ts:102-147`); iOS/tvOS and Android draw an overlay from native modules (`Native/PorticoSubtitleSelection.m`, `…/PorticoSubtitleSelection.kt`). Cues go dark 40 s after the last good authorisation.
2. **HLS WebVTT renditions** — when a session is HLS, up to eight imported text tracks are pinned into the master playlist as `#EXT-X-MEDIA:TYPE=SUBTITLES` (`internal/playback/hls_renditions.go:189-191`, `internal/subtitles/manifest.go:18-30`). Each rendition is a playlist of 60-second WebVTT windows generated on the fly from the cue document (`internal/playback/hls_text.go:35-51`, `manifest.go:75-125`). This is what AVPlayer (so PiP and AirPlay) and Cast can use natively.
3. **Burn-in** — for image tracks and for ASS kept as "styled". Selecting one re-plans the session as a video transcode, bumps the session generation, issues a new grant and restarts HLS production at the current position (`internal/subtitles/presentation.go:98-125`, `session.go:267-305`, `internal/playback/subtitle_delivery.go:16-86`). The filter is libass `ass=` for text and `overlay` of the bitmap stream for images (`internal/decoder/burn_in.go:17-50`). Burn-in is refused when the owner has turned transcoding off. The web client warns once before burning image subtitles on 4K/HDR sources and silently prefers a text track of the same language when one exists (`web/src/player/Player.tsx:292-309`).

There is no client-side renderer for ASS styling or for image subtitles on any Portico client.

### 6. Timing offset

Each selection carries `offsetUs` (±10 minutes). For the cue-document path it is applied by the client; for HLS renditions it is baked into the generated WebVTT; for burn-in it shifts the filter clock. Changing the offset on an HLS or burned session creates a new presentation (new grant, restart). The web UI offers −0.5 s / +0.5 s buttons (`Player.tsx:553`).

### 7. Default track selection

- **Audio** (`internal/playback/delivery_decision.go:227-262`): explicit choice → first track in a preferred language (non-commentary first) → container default (non-commentary) → first non-commentary → first.
- **Subtitles** (`internal/subtitles/autoselect.go:76-112`, `internal/playback/delivery_context.go:155-183`): profile preference `playback.subtitleMode` = `off` | `forced` (default) | `always`, plus an ordered `playback.preferredSubtitleLanguages` list. "Always": first preferred language that has a track, non-SDH before SDH, never a forced track. "Forced": a forced track whose language equals the playing audio's language. Additionally, if the playing audio language is not in the viewer's preferred *audio* languages ("foreign audio"), full subtitles in a preferred subtitle language are chosen even in "forced" mode. The choice is made once, when the session's subtitle state is first created, and only among **already imported plain-text resources**.
- There is **no memory** of what a viewer last picked for a title or a series. A title-page "pick before play" choice (`client-core/src/pending-track-choice.ts`) lasts exactly one play.

### 8. Online subtitles and uploads

- One provider, OpenSubtitles.com REST, enabled only by environment variables (`PORTICO_OPENSUBTITLES_API_KEY`, optional `_TOKEN`, `_USER_AGENT`) (`internal/subtitles/provider.go:55-70`). Search sends the item's title text and a language; download asks for SRT. Results are stored as resources (personal by default, shared if the owner chooses).
- Uploads: `POST /v1/items/{id}/subtitles` with text content or base64 data (+ `.sub` companion). iPhone/iPad have a document picker (`Native/PorticoSubtitleFiles.m`); tvOS does not.
- Nothing is ever written next to the media. Subtitles live in the state folder's object store.

### 9. Chapters

Read from the container at scan (`internal/assets/assets.go:233-253`), stored in `asset_chapters` with start and end. If any chapter is out of order, overlapping, zero-length or ends more than 0.1 s past the duration, **all** chapters for that file are discarded and the file is flagged `invalid_chapter_timeline`. Players read chapters per session (`GET /v1/playback/sessions/{id}/chapters`, paged, revision-fenced). Chapter thumbnails (320×180 JPEG, letterboxed) are an optional analysis product served at `/v1/items/{id}/chapters/{source:index}/image`. Chapters are withheld for prepared (optimised) versions.

### 10. Markers (intro / recap / credits / commercial)

- Stored per file in `analysis_markers` with kind, start/end (µs), confidence, provenance, approved, edited, deleted.
- Detection (`internal/mediaanalysis/detectors.go`), run by the optional "segment_detection" analysis: (a) **chapter titles** containing words like *intro*, *opening*, *recap*, *previously*, *credit*, *outro*, *commercial*; (b) a **luminance heuristic** on a 32×18 greyscale frame per second — "intro" = from 0 to the first ≥1 s black gap that starts between 10 s and 180 s; "commercial" = between two black gaps; "credits" = a sustained dark-with-some-bright-text run in the last fifth.
- One rule decides unattended skipping (`internal/segmentmarkers/markers.go:63-71`): `automaticSafe` is true only for owner-approved markers, or detector markers with confidence ≥ 0.85 — and no current detector reaches that (chapter labels are 0.85 but excluded by provenance; luminance guesses are 0.2–0.35). So auto-skip only ever fires on markers an owner approved or created.
- Owner can create, edit, approve and dismiss markers through `POST /v1/items/{id}/analysis/markers`.
- Viewer preferences: `playback.introSkip`, `creditsSkip`, `recapSkip` = `ask` | `auto` | `off`. Clients show a Skip button while inside any non-dismissed marker (even an unapproved low-confidence one) and report skips back to the server.

### 11. Trickplay

Optional "trickplay" analysis per library (off by default). One FFmpeg pass decodes the **whole video** single-threaded in software with `fps=1/interval,scale,pad` and pipes raw RGB frames to Go, which JPEG-encodes them (quality 75) into 10×10 sprite sheets (defaults: 320×180 tiles, 10 s interval, at most 720 frames per file by operator budget, so long films get a longer interval) (`internal/decoder/analysis.go:81-99`, `internal/mediaanalysis/produce.go:186-260`, `trickplay.go:84-103`). Every frame is stored twice: once as an individual JPEG artifact and once inside a sheet. Storage is the analysis artifact store in the state folder, with a 32 GiB default cache cap and retention sweeps. Delivery: `GET /v1/items/{id}/trickplay` (descriptor list), `…/tiles/{n}.jpg`, and a WebVTT thumbnail track `…/thumbnails.vtt` with `#xywh` fragments; all under library authorisation, cached privately for five minutes.

### 12. Other analysis stages

`checksum` (SHA-256 of the file), `waveform`, `fingerprint` (a home-grown 32-bit spectral signature roughly once a second), `loudness` (EBU R128), `chapter_images`, `trickplay`, `segment_detection`. Each is a separate pass over the file; one analysis job runs at a time (`work.go:38-44`), at background priority. The deep stages require the basic scan (including the scan-time subtitle text import) to have completed for that exact file revision (`work.go:137-139`).

### 13. Client coverage today

| Capability | Web | iPhone/iPad/Apple TV | Android/Fire TV | Cast receiver |
|---|---|---|---|---|
| Text subtitles (cue document overlay) | yes (`VTTCue`s) | yes (app overlay; AVPlayer-native when the track is in the HLS manifest or matches an embedded option) | yes (app overlay) | in-stream only (Backlog) |
| Import a discovered track from the player | yes | yes (`src/player/PlayerOptions.tsx:256`) | same RN screen | no |
| Image / styled subtitles | burn-in only | burn-in only | burn-in only | burn-in only |
| Timing offset control | ±0.5 s buttons | yes (`PlayerOptions.tsx:267`) | same RN screen | no |
| Appearance settings | size, background | none found | none found | no |
| Subtitle upload | **no screen calls it** (server API exists) | a native picker module exists (`PorticoSubtitleFiles.m`) but **no JS caller found** | none | no |
| Online search | yes (uses UI locale as language) | yes (`PlayerOptions.tsx:264`) | same RN screen | no |
| Skip prompt (ask/auto/off) | yes | yes | yes | no |
| Chapter list with thumbnails | yes | **no usage of the chapter service found** | none found | no |
| Trickplay scrub previews | yes | **no usage found** | **no usage found** | no |

## How the others do it

### Jellyfin server (read: `MediaBrowser.MediaEncoding/Subtitles/*`, `Emby.Naming/ExternalFiles/ExternalPathParser.cs`, `Emby.Naming/Common/NamingOptions.cs`, `MediaBrowser.Providers/MediaInfo/MediaInfoResolver.cs`, `Emby.Server.Implementations/Library/{MediaStreamSelector,MediaSourceManager}.cs`, `Jellyfin.Api/Controllers/SubtitleController.cs`, `MediaBrowser.Model/Entities/MediaStream.cs`, `MediaBrowser.Model/Dlna/{StreamBuilder,SubtitleDeliveryMethod}.cs`, `MediaBrowser.Controller/MediaEncoding/EncodingHelper.cs` (subtitle parts), `MediaBrowser.MediaEncoding/Encoder/MediaEncoder.cs` (image/trickplay extraction), `Jellyfin.Server.Implementations/Trickplay/TrickplayManager.cs`, `MediaBrowser.Model/Configuration/{TrickplayOptions,LibraryOptions,EncodingOptions}.cs`, `Jellyfin.Server.Implementations/MediaSegments/MediaSegmentManager.cs`)

- **One list of streams.** Every embedded and external subtitle is a `MediaStream` with one integer `Index`. External files get indexes continuing after the embedded ones (`MediaInfoResolver.cs:83-150`). Any stream can be requested directly: `GET /Videos/{item}/{source}/Subtitles/{index}/Stream.{format}`.
- **External files** (`MediaInfoResolver.cs:180-260`, `ExternalPathParser.cs`, `NamingOptions.cs:162-318`): the video's folder *and* the item's internal metadata folder are searched. Name must start with the video name followed by `.`. Extensions: `.ass .mks .sami .smi .srt .ssa .sub .sup .vtt` plus `.idx`. Tokens are read from the end: `default`; `forced`/`foreign`; `cc`/`hi`/`sdh` (with a special case so `hi` can still mean Hindi); a language by 2-letter code, 3-letter code or name; **anything unrecognised becomes the track title** instead of disabling the file. Each external file is probed with ffprobe, so a multi-track `.mks` works. External **audio** files are handled by the same resolver.
- **Encoding**: external text files go through a character-set detector (`SubtitleEncoder.cs:167-200`, `:1006-1030`) and are re-encoded to UTF-8; for burn-in the detected charset is passed to libass (`EncodingHelper.cs:1962-1978`).
- **Parsing/conversion**: the SubtitleEdit library (`SubtitleEditParser.cs`) parses a very wide range of formats; writers exist for SRT, WebVTT, ASS, SSA, TTML and a JSON cue format (`SubtitleEncoder.cs:269-313`). ASS can be handed out untouched for clients that render it.
- **Extraction**: on demand, cached on disk under a subtitle cache path keyed by source and stream index, with a `.meta` freshness file (`size:mtime`) (`SubtitleEncoder.cs:355-420`). When one text track of a file is needed, **all** extractable tracks of that file are extracted in **one** FFmpeg pass (`ExtractAllExtractableSubtitles`, `:515-566`). Timeout is configurable, default 30 minutes (`EncodingOptions.cs:62`).
- **Delivery method per client** (`SubtitleDeliveryMethod`): `External` (client fetches a file), `Hls` (WebVTT rendition playlist, `SubtitleController.cs:337-410`, segment length is a query parameter), `Embed` (kept inside the stream), `Encode` (burn-in), `Drop`. The client's device profile lists which formats it can take by which method; the web client adds user choices "burn: auto / only image formats / all complex formats / all" and "always burn in when transcoding".
- **Burn-in** maps the subtitle stream straight from the source file. Graphical subtitles use the subtitle's own canvas size and are scaled preserving aspect, padded or cropped to the video (`EncodingHelper.cs:984-1008`, `:3431-3478`). Text burn-in passes a fonts directory holding the file's extracted font attachments and optional fallback fonts (`:1955-1961`; `SubtitleController.cs:536-615`).
- **Default tracks** (`MediaStreamSelector.cs`, `MediaSourceManager.cs:493-570`): a remembered per-item audio/subtitle index (user settings `RememberAudioSelections`, `RememberSubtitleSelections`, default on) wins. Otherwise audio: optional "original language" preference, preferred language, default flag. Subtitles: mode `Default` (external/default/forced flags), `Always`, `OnlyForced` (forced track in a preferred subtitle language *or with an undefined language*), `None`, `Smart` (subtitles only when the audio language is not a preferred subtitle language, else behave like OnlyForced).
- **Online subtitles**: provider plugins (OpenSubtitles is the common one), remote search/download endpoints, upload and delete, plus a scheduled "download missing subtitles" task driven by per-library options: languages to fetch, skip if embedded subtitles exist, skip if the audio already matches, require perfect (hash) match, and **save next to media** (default on) (`LibraryOptions.cs:16-136`).
- **Chapters**: read from the file; "dummy" chapters are generated for files with none; chapter images are optional per library, extracted during scan or by a scheduled task, using FFmpeg's `thumbnail` filter to avoid black frames and software tone-mapping for HDR (`MediaEncoder.cs:695-745`).
- **Media segments**: core Jellyfin stores segments of type Intro, Outro, Recap, Preview, Commercial and exposes them to clients; detection comes from provider plugins, ordered and enabled per library; a scheduled task runs them.
- **Trickplay** (`TrickplayOptions.cs`): per library on/off and "extract during scan"; options for hardware decode, hardware MJPEG encode, **key-frame-only extraction**, blocking vs non-blocking scan, process priority, thread count, interval (10 s), one or more widths (320), 10×10 tiles, JPEG quality 90; stored in the metadata folder or **next to the media**; served as numbered JPEG sheets plus an HLS image playlist; existing folders are re-adopted.

### intro-skipper plugin (read: `IntroSkipper/Configuration/PluginConfiguration.cs`, analyzer file list, `ChromaprintAnalyzer.cs` header, FFmpeg parser)

The de-facto Jellyfin detector. Methods, combined: **Chromaprint audio fingerprints** compared across the episodes of a season (the first 25 % / at most 10 minutes of each episode) to find the shared opening; **chapter-name regexes** (configurable per type, plus SponsorBlock-style chapter names); **black-frame analysis** and a **key-frame visual credits detector** for end credits (including non-black credits); **silence detection** and **key-frame snapping** to refine boundaries; optional **subtitle-text** detection of recaps and previews ("previously on…", "next time…"). Dozens of knobs: min/max duration per type, skip-first-episode, offsets, parallelism (2), process priority (below normal), per-file timeout (300 s), auto-skip, button hide delay.

### jellyfin-web (read: `src/plugins/htmlVideoPlayer/plugin.js` subtitle parts, `src/components/subtitlesettings/*`, `playbackmanager.js` track carry)

- Renders ASS/SSA in the browser with **libass compiled to WebAssembly** (`@jellyfin/libass-wasm`), loading the file's font attachments and server fallback fonts (`plugin.js:1395-1470`).
- Renders **PGS** in the browser with `libbitsub` (`plugin.js:1474-1540`), so image subtitles do not force a transcode.
- Custom subtitle element for text with appearance settings: size, weight, font, drop shadow, text colour, background, vertical position, and a styling mode (auto / custom / native).
- Subtitle offset slider; **secondary subtitles** (two tracks at once); burn-in preference; carries the previous episode's audio/subtitle choice to the next (`autoSetNextTracks`).
- Per-segment-type action: none / ask to skip / skip.

### Plezy (read: `lib/services/{track_selection_service,player_sync_offsets,subtitle_preference,bif_thumbnail_service,jellyfin_trickplay_service}.dart` headers, `lib/services/settings_service.dart` preference list, `lib/utils/subtitle_forced_semantics.dart`, mpv player property use)

- mpv plays the original file, so ASS (with fonts), PGS, VobSub and every text format are rendered **on the device**. The server never burns in.
- Sync: `audio-delay` and `sub-delay`, saved per scope the user chooses (don't save / global / library / title).
- Styling: font size, colour, border size and colour, background colour and opacity, bold, italic, ASS-override mode, render resolution, margins, anchor to screen. Secondary subtitle track.
- Track choice: `rememberTrackSelections` and `followServerTrackSelections`; a 1,300-line service matches a carried "intent" (language, title, forced-ness, codec, channels) to the next episode's tracks and **declines rather than guesses** when ambiguous. Its comments record real bugs from matching by index or position.
- Markers: skip mode per type (off / button / auto) with an **auto-skip countdown** (default 5 s); a client-side fallback that derives intro/credits from chapter names by regex when the server has no markers.
- Thumbnails: Plex BIF files and Jellyfin trickplay tiles. Subtitle search through the server's API.

### Others, lighter

- **jellyfin-androidtv**: optional libass rendering (`libass_enabled`, off by default), PGS direct play (on by default), subtitle text size, background colour, stroke colour, vertical offset, per-segment-type actions (`preference/UserPreferences.kt:223-304`).
- **Swiftfin**: VLCKit player with subtitle and audio offset buttons (`Shared/Views/VideoPlayer/Components/Toolbar/ActionButtons/*OffsetActionButton.swift`), trickplay.
- **Streamyfin** (React Native, closest to Portico's native app): bundles an **mpv player module** (`modules/mpv-player`), so ASS/PGS render on device; settings include subtitle mode, remember audio/subtitle selections, subtitle size, skip-intro mode, and a **client-side OpenSubtitles** key (`utils/atoms/settings.ts:425-548`).
- **Plex / Emby / Infuse** (public knowledge, not code): Plex keeps per-show audio and subtitle language overrides, has on-demand subtitle search with hash matching, intro/credits detection on its own schedule, and BIF "video preview thumbnails". Infuse renders everything on device.

## Architectural differences

### A-04.1 Import-then-play versus "every stream is playable"
- **Portico:** two lists per file — discovered tracks (facts) and imported resources (playable). A track must be imported into the server's object store before it can be selected (`subtitles/service.go:237-345`, `acquisition.go:34`, `playbackv1/subtitles.go:50-60`).
- **Jellyfin / Plezy:** one list. Any embedded or external stream is addressable by index and delivered or converted on request. mpv clients just read the file.
- **Why it matters:** Portico gains immutability, revisions, per-viewer tracks and a clean authorisation story. It pays with a second state machine, an extra user step for anything not pre-imported, and several edge cases where a track exists in the file but is not offered (see B-04.1, B-04.2, B-04.24).
- **Assessment:** **Trade-off (subjective).** Looks deliberate (the whole `subtitles` package is built around it). I think the model is defensible but the *boundary is in the wrong place*: discovery should be enough to list and auto-select a text track, with import as a lazy cache fill. Closing that gap is medium work (server: let selection of a discovery trigger import transparently; let auto-select and playback options see discoveries).

### A-04.2 When text is extracted: at scan, per stream, versus on demand, per file
- **Portico:** the scan imports up to eight embedded text tracks, one FFmpeg run per track (each a full read of the file through a loopback bridge), all inside one 90-second budget (`scan_text.go:25-56`, `subtitlevideo/runtime.go:206-230`).
- **Jellyfin:** nothing at scan by default. On first use it extracts *all* text tracks in one pass and caches them; 30-minute configurable timeout.
- **Why it matters:** Portico's first play of an imported track is instant and needs no FFmpeg. But scans of large files read each file up to eight extra times, the work is classed "metadata-only" in the owner's cost table (`administration/library_settings.go:59`), and the 90 s budget is easy to exceed on remuxes or network storage (B-04.4).
- **Assessment:** **Portico behind** on cost and robustness; ahead on first-play latency. Adopting "one pass, all text streams" is small-to-medium (one FFmpeg command with several outputs, then publish each). Making the time budget proportional to file size is small.

### A-04.3 Stored form: a JSON cue document versus the original file
- **Portico:** text tracks are reduced to `{start,end,text,italic?,top?}` cues (`document.go:28-43`). Colours, fonts, partial italics, positions other than "top", karaoke and voice tags are dropped by design (`tolerant.go:17-28`).
- **Jellyfin:** keeps the original; converts per request to SRT/VTT/ASS/TTML/JSON.
- **Why it matters:** Portico's document is tiny, safe to render (no markup injection) and identical on every client. But the conversion is one-way: a later client that could render ASS or coloured SRT has nothing to render from unless the track is re-imported as "styled" (which today means burn-in only).
- **Assessment:** **Trade-off (subjective).** Deliberate. I would keep the cue document as the default but also retain the original bytes in the object store (they are already digested as `original_digest`, just not kept for text). That keeps the door open for client ASS rendering (F-04.1). Small.

### A-04.4 Subtitles never written beside the media
- **Portico:** uploads, downloads and extractions live only in the state folder (`subtitles/service.go:531-547`).
- **Jellyfin:** `SaveSubtitlesWithMedia` defaults to on; downloaded subtitles become ordinary sidecars other tools can see.
- **Why it matters:** Portico never needs write access to libraries, which is safer. But a downloaded subtitle is invisible to Kodi, Plex, Infuse or a file copy, and is lost if the state folder is.
- **Assessment:** **Trade-off (subjective).** Consistent with "one state folder" but in tension with the "open data" principle. An opt-in per-library "also save beside the media" would be medium (needs a write path in the storage client).

### A-04.5 Sidecar matching grammar
- **Portico:** exact folder; `.srt .vtt .ass .ssa .sup .idx`; tokens `forced`, `sdh`, `cc`, one language; a second unknown token **disables** the file; a same-named second video disables all sidecars (`inventory.go:66-121`).
- **Jellyfin:** also `.smi .sami .sub .mks`; flags `default`, `foreign`, `hi`; language by name; unknown tokens become the title; also scans its metadata folder.
- **Why it matters:** common real-world names fail in Portico: `Movie.en.hi.srt` (Bazarr's default for hearing-impaired), `Movie.en.default.srt`, `Movie.English.srt` (language lost), `Movie.en.Signs.srt`. See B-04.6.
- **Assessment:** **Portico behind.** The strictness reads as deliberate caution ("ambiguous association"), but refusing is the worst outcome for a viewer. Small: recognise `hi`, `default`, `foreign`; treat leftovers as a title; map language names.

### A-04.6 Character encodings
- **Portico:** SRT/VTT: UTF-8, UTF-16 with BOM, else assume Windows-1252 (`tolerant.go:49-69`). ASS and `.idx`: Unicode only, "no locale-dependent guessing" (`document.go:84-116`).
- **Jellyfin:** statistical charset detection for every external text file.
- **Why it matters:** a Windows-1251 (Cyrillic), Windows-1250 (Central European), GBK/Big5, Shift-JIS, EUC-KR or ISO-8859-7 SRT is accepted by Portico and shown as mojibake, with no warning. A legacy-encoded ASS is rejected outright. See B-04.9.
- **Assessment:** **Portico behind** for non-Western libraries. Small-to-medium: add a detector (Go has `golang.org/x/net/html/charset` and `saintfish/chardet`-style libraries), using the track's language as a hint.

### A-04.7 Own parsers versus a subtitle library
- **Portico:** hand-written tolerant SRT/VTT parser, strict ASS validator, hand-written VTT→ASS converter (`render_asset.go:246-618` and `cue_css.go`; `textASS` has no caller), plus a strict `Parse` that nothing calls.
- **Jellyfin:** SubtitleEdit's library.
- **Why it matters:** Portico controls every byte it accepts (good for a hostile-input stance) but supports few formats and carries dead code.
- **Assessment:** **Trade-off (subjective).** Keep the approach; delete the dead paths (see B-04.33).

### A-04.8 Format coverage

| Format | Portico in | Portico out | Jellyfin in | Jellyfin out |
|---|---|---|---|---|
| SRT | embedded, sidecar, upload, provider | cue JSON / HLS WebVTT | yes | SRT, VTT, ASS, TTML, JSON |
| WebVTT | embedded, sidecar, upload | same | yes | same |
| ASS/SSA | embedded, sidecar, upload | flattened text, or burn-in | yes (kept) | ASS untouched to clients; converted; burn-in |
| MOV text (`mov_text`, tx3g) | embedded (as SRT) | text | yes | text |
| PGS | embedded, `.sup` | burn-in only | yes | client render (web, Android TV), burn-in |
| VobSub | embedded, `.idx`+`.sub` | burn-in only | yes | burn-in (some clients render) |
| DVB bitmap | embedded | burn-in only | yes | burn-in |
| DVB teletext, ARIB | no | — | via ffmpeg where supported | burn-in |
| CEA-608/708 in video | no (not a stream) | web shows hls.js caption track on live only | partial | partial |
| TTML/DFXP, SMI/SAMI, MicroDVD `.sub`, `.mks` | no | — | yes | converted |
| External audio (`.mka` etc.) | no | — | yes | — |

- **Assessment:** **Portico behind** on breadth; adequate for mainstream Western libraries. Each extra text format is small once a general parser exists.

### A-04.9 Styled ASS: flatten or burn, never render on the client
- **Portico:** default is plain text; "styled" means burn-in; no client has libass (`acquisition.go:24-31`, `tolerant.go:311-332`). The burn-in filter gets no fonts directory, and scripts with an embedded `[Fonts]` section are rejected (`decoder/burn_in.go:30`, `render_asset.go:175`).
- **Jellyfin web / Android TV / mpv clients:** render ASS on the device with the file's own fonts.
- **Why it matters:** anime and fansub typesetting is either lost (plain) or costs a full video transcode (styled) — and even then with substitute fonts. This is the single biggest quality gap for anime libraries, which Portico lists as first-class.
- **Assessment:** **Portico behind.** The spec already anticipates `render: client` for ASS (§5.5). Web: medium (JASSUB + a fonts endpoint + keep original ASS + attachments extraction). Native: large unless an mpv/libass component is added.

### A-04.10 Image subtitles: burn-in only
- **Portico:** PGS/VobSub/DVB always burn in; refused when the owner disables transcoding (`presentation.go:98-110`).
- **Jellyfin web, Android TV, mpv clients:** draw the bitmaps on the device.
- **Why it matters:** turning on Blu-ray subtitles on a 4K HDR remux forces a 4K transcode (and tone-mapping decisions) on the server. On weak hardware that is the difference between playing and not playing. The spec says clients render PGS/VobSub when they declare `client` (§5.5, "per D-FEAT-3") — no Portico client does.
- **Assessment:** **Portico behind.** Web PGS rendering is medium (a WASM or JS PGS renderer plus a route for the raw `.sup`). Android can use Media3's PGS decoder (medium). AVPlayer cannot; tvOS/iOS would stay on burn-in unless OCR-to-text (F-04.20) or an alternative engine is added.

### A-04.11 Burn-in needs a pre-extracted copy of the track
- **Portico:** the image track must first be extracted in full into a Matroska capsule (≤32 MiB, stored base64 inside JSON) and is then fed to FFmpeg as a second input (`subtitlevideo/runtime.go:226-227`, `playback/hls_burn.go:36-53`).
- **Jellyfin:** maps the subtitle stream from the same input; no preparation.
- **Why it matters:** first selection of a PGS track in Portico costs a full read of the file before the transcode can even start, with a 110 s client timeout (`client-core/src/subtitles.ts:153`) and a size cap that real Blu-ray tracks exceed (B-04.11, B-04.12).
- **Assessment:** **Portico behind.** The capsule exists because the confined decoder gets one bridged input and because an immutable asset is easy to fence. Medium to change: let the burn graph map `0:s:<index>` from the already-open source when the selected resource is "embedded image".

### A-04.12 Burn-in geometry
- **Portico:** forces the subtitle canvas to the video's pixel size and scales the subtitle picture to exactly that size; overlays before any output scaling (`decoder/burn_in.go:46-48`).
- **Jellyfin:** uses the subtitle's own canvas, scales by height preserving aspect, pads/crops to the output size; overlays at output resolution.
- **Why it matters:** when the video's shape differs from the subtitle canvas (a cropped 1920×800 encode with 1920×1080 PGS), Portico's approach risks squashed or clipped subtitles (B-04.13). Overlaying at source resolution is more accurate but costs more on 4K→1080p conversions.
- **Assessment:** **Portico behind** on the mismatch case (needs a runtime check); trade-off on where the overlay happens. Small fix once confirmed.

### A-04.13 Fonts for burn-in
- **Portico:** a generated fontconfig naming host font folders plus one bundled face, DejaVu Sans (`mediaexec/fonts.go:15-66`).
- **Jellyfin:** fonts directory of the file's attachments, optional admin fallback font folder, charset hint.
- **Why it matters:** on a minimal container host, styled CJK, Thai, Devanagari or Arabic subtitles burn as empty boxes in Portico (DejaVu has no CJK). See B-04.14.
- **Assessment:** **Portico behind.** Small: bundle a Noto fallback set or let the owner name a fonts folder; medium: extract and use attachments.

### A-04.14 Who decides to burn
- **Portico:** the server decides from the resource's renderer; the owner has one switch (transcoding on/off); the web client asks once for 4K/HDR image tracks and silently swaps to a same-language text track when one exists (`Player.tsx:292-309`).
- **Jellyfin:** per-device user settings: burn auto / only image formats / all complex formats / all, and "always burn in when transcoding".
- **Why it matters:** Portico is simpler and honest about cost. It gives no way to *force* burn-in of text (useful for devices with poor text rendering or for exact styling) other than re-importing as "styled".
- **Assessment:** **Trade-off (subjective).** Portico's default is the better default. The silent swap from the picked bitmap track to "the" text track of that language can pick a different track than the viewer meant (SDH vs non-SDH); it is announced with a notice, which is reasonable.

### A-04.15 Delivery by authorised polling
- **Portico:** clients re-fetch the cue document every ~25 s; the server re-authorises every read and every chunk; cues go dark 40 s after the last success (`httpapi/subtitle_routes.go:325-370`, `PorticoSubtitleSelection.m:89`, `:116`).
- **Jellyfin:** a subtitle is a static file URL fetched once.
- **Why it matters:** Portico revokes subtitles with the session, which nobody else does. The price: a subtitle that vanishes mid-film on a flaky connection or a busy server (each read takes two background-class write transactions, see B-04.32), and a protocol a third party must learn.
- **Assessment:** **Trade-off (subjective).** I think the revocation benefit is very small (the viewer already has the whole document in memory; hiding it protects nothing) and the failure mode is visible. I would keep the grant-scoped URL but drop the "go dark after 40 s" rule. What would change my view: a concrete threat this closes.

### A-04.16 HLS subtitle renditions
- **Portico:** up to eight imported text tracks, each `DEFAULT=NO, AUTOSELECT=NO`, no `FORCED`, no SDH `CHARACTERISTICS`, 60-second windows against 6-second video segments, offset baked in (`hls_renditions.go:189-191`, `hls_text.go:35-51`).
- **Jellyfin:** one WebVTT playlist per requested track, segment length chosen by the client.
- **Why it matters:** this path is what makes PiP, AirPlay and Cast show subtitles. Because nothing is marked default/auto/forced, a plain HLS player (the spec's stated target, P05) never shows the auto-selected or forced track by itself, and system accessibility settings (prefer SDH) cannot match.
- **Assessment:** **Portico ahead** in having multi-track renditions at all; **behind** on attributes. Small: emit `DEFAULT/AUTOSELECT=YES` for the session's selected track, `FORCED=YES` for forced tracks, and `public.accessibility.*` characteristics for SDH.

### A-04.17 Timing offset is server state
- **Portico:** the offset is part of the session's subtitle selection (revisioned, idempotent), and each resource revision also has its own stored offset, so an owner can permanently fix a badly timed shared track (`service.go:349-367`, `session.go:306`). Changing it on HLS/burn restarts the presentation.
- **Jellyfin web / Plezy:** a client-side slider or mpv `sub-delay`; Plezy can persist per title/library.
- **Why it matters:** Portico's permanent per-track correction is something Jellyfin lacks. The live adjustment is coarser (0.5 s steps in the web UI) and heavier (a server round trip; a new grant on HLS).
- **Assessment:** **Portico ahead** on persistence, **behind** on immediacy. Small: apply the live offset locally for overlay rendering and send it on a debounce.

### A-04.18 No audio delay control
- **Portico:** none found.
- **Plezy, Swiftfin, Kodi, VLC:** audio delay, often remembered per title or per output device.
- **Why it matters:** lip-sync correction for Bluetooth speakers and AVRs is a common need on TV devices.
- **Assessment:** **Portico behind.** AVPlayer has no direct audio delay; Android ExoPlayer and the web can approximate. Medium, platform-dependent.

### A-04.19 Subtitle modes and the meaning of "forced"
- **Portico:** `off` / `forced` / `always`, plus an implicit "foreign audio ⇒ full subtitles" rule that applies even in `forced` mode whenever the viewer has set preferred audio languages (`delivery_context.go:157-170`, `autoselect.go:80-101`). A forced track is chosen when its language equals the **audio** language. Forced tracks with an undefined language are never chosen. The container's `default` flag is ignored.
- **Jellyfin:** `Default` (honour file flags) / `Always` / `OnlyForced` / `None` / `Smart`. Forced tracks are matched against the viewer's **subtitle** languages, and undefined-language forced tracks are accepted.
- **Why it matters:** the two forced rules diverge for dubbed content and for files that tag the forced track in the viewer's language rather than the audio's. Portico has no way to say "do what the file's author flagged".
- **Assessment:** **Trade-off (subjective).** Portico's rule is well reasoned in its comments. I would add: accept `und` forced tracks; fall back to a forced track in a preferred subtitle language; consider a "follow the file" mode. Small.

### A-04.20 Auto-selection only sees imported plain-text tracks
- **Portico:** candidates are imported `external_text` resources (`autoselect.go:134`). Sidecars, image tracks and embedded text beyond the first eight are invisible to it.
- **Jellyfin:** all streams are candidates (text preferred by score).
- **Assessment:** **Portico behind** — see B-04.1 and B-04.2. The "never extract or burn at start" intent (comment at `autoselect.go:8-14`) is sound; importing sidecars at scan would satisfy it.

### A-04.21 SDH detection
- **Portico:** by words in the title (`sdh`, `hearing`, `[cc]`, `closed caption`) (`autoselect.go:64-67`, `playbackv1/options.go:504`). The probe reads the `hearing_impaired` disposition (`assets/assets.go:161`) but the subtitle code never receives it.
- **Jellyfin:** disposition flag or filename flag.
- **Assessment:** **Portico behind.** Small (carry the flag through `subtitles.Track`).

### A-04.22 Audio default and language matching
- **Portico:** preferred languages → default flag → first, never a commentary while an ordinary track exists; commentary/descriptive detection partly by title text; languages compared through a hand-written 57-entry ISO table (`delivery_decision.go:227-287`).
- **Jellyfin:** remembered choice → "original language" option → preferred → default.
- **Why it matters:** see B-04.15 for the table's gaps. Portico's commentary avoidance is a nice touch Jellyfin lacks.
- **Assessment:** **Trade-off**, with a real bug in the table. No "original language" preference in Portico (F-04.6).

### A-04.23 No remembered track choice
- **Portico:** none; the spec promises "last-used" (§5.4) but nothing stores it. A title-page pick lasts one play.
- **Jellyfin:** per-item memory on the server, and the web client carries the choice to the next episode. **Plezy:** carries an "intent" across episodes with careful matching. **Plex:** per-show language overrides.
- **Why it matters:** a viewer who switches a show to Japanese audio with English subtitles must redo it every episode unless their global languages happen to express it.
- **Assessment:** **Portico behind.** Medium: store `{audio language+title, subtitle language+forced/SDH+title | off}` per viewer per show (and per movie), apply after global preferences. Plezy's matcher is the model to copy: match by meaning, decline on ambiguity.

### A-04.24 Track identity
- **Portico:** audio = `a<absolute stream index>`; subtitle = opaque resource id, with the embedded stream's absolute index used only inside the server for extraction. I found no place that mixes per-type and absolute indexes.
- **Jellyfin:** one integer index space where externals follow embedded streams — so adding or removing a sidecar renumbers remembered choices. Plezy's matcher exists largely to survive this.
- **Assessment:** **Portico ahead.** Worth keeping.

### A-04.25 Switching audio
- **Portico:** HLS sessions list every audio track as a rendition and switch in place; direct play of a non-default track starts a new session (`pending-track-choice.ts:62-76`).
- **Jellyfin:** changing audio restarts the stream (or switches natively in mpv/VLC clients).
- **Assessment:** **Portico ahead** on HLS; equal otherwise.

### A-04.26 Personal versus shared subtitles, with history
- **Portico:** resources are shared or personal, revisioned, idempotent by operation id, with attribution and a "rights" note (`service.go:349-648`).
- **Jellyfin:** one global set per item; upload/delete need a permission.
- **Assessment:** **Portico ahead.** No competitor has per-viewer subtitles.

### A-04.27 Online subtitles
- **Portico:** OpenSubtitles only, enabled by environment variable, viewer-initiated only, query by title text, 40 candidates, 10-minute candidate lifetime (`provider.go`).
- **Jellyfin:** plugins, scheduled automatic download with per-library rules, hash and ID matching.
- **Assessment:** **Portico behind** (and see B-04.19, B-04.20). The spec's "owner-configurable provider" is not built.

### A-04.28 Who may manage subtitles
- **Portico:** any viewer with library access may search, download, upload and import **personal** tracks; only the local owner may create shared ones. The spec's `subtitles.download` profile permission is not implemented.
- **Jellyfin:** a `SubtitleManagement` permission per user; delete needs elevation.
- **Assessment:** **Trade-off.** Portico's split is elegant. Note the cost exposure: each viewer download uses the owner's provider quota.

### A-04.29 Chapters
- **Portico:** strictly validated; one bad chapter discards all (`assets/assets.go:233-253`); served per playback session with paging and a revision fence; withheld for prepared versions.
- **Jellyfin:** chapters ride on the item; start times only; dummy chapters every five minutes when a file has none.
- **Assessment:** **Trade-off.** Portico's fence is rigorous; the all-or-nothing discard is harsh (B-04.23). Chapters being session-scoped means a title page cannot show them before play.

### A-04.30 Chapter thumbnails
- **Portico:** exact chapter start, 320×180 letterboxed, no HDR tone-mapping (`decoder/analysis.go:78-79`).
- **Jellyfin:** `thumbnail` filter to dodge black frames, tone-maps HDR, keeps aspect.
- **Assessment:** **Portico behind** on picture quality (chapter starts are very often black or mid-fade). Small.

### A-04.31 Marker model
- **Portico:** confidence, provenance, approval, history; the server alone decides `automaticSafe`; clients are forbidden from inferring it (`segmentmarkers/markers.go`).
- **Jellyfin:** plain segments; trust is whatever the provider wrote.
- **Assessment:** **Portico ahead** in design. It is the right shape for mixing detectors of different quality.

### A-04.32 Detection method
- **Portico:** chapter labels plus single-file luminance guesses with confidences of 0.2–0.35; nothing can be auto-skipped without owner approval (`mediaanalysis/detectors.go`, `produce.go:313`).
- **intro-skipper:** cross-episode audio fingerprinting plus black-frame, silence, key-frame and chapter analysis.
- **Why it matters:** the industry-standard way to find an intro is "the audio that repeats across episodes". Portico's luminance rule labels *everything before the first black gap* as intro, which for a cold-open show is the story itself (B-04.21). In practice Portico today has no usable automatic intro detection.
- **Assessment:** **Portico behind**, by a wide margin. The analysis framework (stages, artifacts, a `fingerprint` stage that already decodes audio) is a good base. Large: a season-level comparison job is a new kind of work (it needs several files at once, not one).

### A-04.33 Marker types and preferences
- **Portico:** intro, recap, credits, commercial, outro (owner-only `chapter`); viewer prefs for intro, credits, recap; commercial is hard-coded to "ask" (`Player.tsx:148`); no `preview`.
- **Jellyfin:** Intro, Outro, Recap, Preview, Commercial with a per-type action on each client.
- **Assessment:** minor **behind** (no preview, no commercial preference).

### A-04.34 Skip experience
- **Portico:** button while inside a marker; `auto` skips instantly but only for safe markers; no countdown; Dismiss; a rewind does not re-skip (`SkipPrompt.tsx`).
- **Plezy:** auto-skip after a visible countdown (default 5 s) so it can be cancelled. **Jellyfin web/androidtv:** ask or skip.
- **Assessment:** **Trade-off.** Portico's instant skip on approved markers is fine; a short cancellable countdown is friendlier.

### A-04.35 Skips are reported to the server
- **Portico:** the client reports `{markerId, mode, position}`; the server records once per session and ignores an automatic skip it did not authorise (spec §4.2).
- **Others:** nothing equivalent.
- **Assessment:** **Portico ahead** — this is the raw material for measuring marker quality.

### A-04.36 Trickplay pipeline
- **Portico:** full software decode, one thread, every frame through an `fps` filter, raw RGB to Go, Go encodes JPEG (`decoder/analysis.go:39`, `:98-99`; `produce.go:214-260`).
- **Jellyfin:** FFmpeg writes MJPEG itself; optional hardware decode/encode; optional key-frame-only mode; configurable threads and priority.
- **Why it matters:** key-frame-only extraction is typically an order of magnitude cheaper than decoding every frame. On the "weak box" end of Portico's target hardware, a 4K HEVC library may take weeks.
- **Assessment:** **Portico behind** on cost. Small: add `-skip_frame nokey` mode (with `-an -sn`) as the default for long-GOP-tolerant thumbnails; medium: hardware decode inside the sandbox.

### A-04.37 Trickplay geometry
- **Portico:** tiles are always 16:9 with letterbox bars; one width; a per-file frame budget (720 by default) stretches the interval for long files (`trickplay.go:84-103`, `produce.go:197`).
- **Jellyfin:** tile height follows the video's aspect; several widths; fixed interval.
- **Assessment:** **Trade-off.** Portico's bounded storage per file is sensible. Letterboxing wastes pixels on 2.39:1 films.

### A-04.38 Trickplay storage
- **Portico:** content-addressed artifacts in the state folder under a 32 GiB default cap with retention; each frame stored both singly and in a sheet (B-04.29).
- **Jellyfin:** a folder of sheets in the metadata folder or beside the media; re-adopted on rescan or migration.
- **Why it matters:** beside-the-media storage survives a server rebuild and can be shared between servers. Portico's survive only with the state folder, and a cap can evict them.
- **Assessment:** **Trade-off (subjective).** Reading existing Jellyfin `*.trickplay` folders (F-04.17) would spare owners the regeneration.

### A-04.39 Trickplay delivery
- **Portico:** JSON descriptor + sheets + a WebVTT thumbnails track, item-scoped, `private, max-age=300`, an `ETag` that is sent but never honoured for `If-None-Match` (`httpapi/trickplay.go:55-59`).
- **Jellyfin:** sheets by index + an HLS image playlist; **Plex:** BIF.
- **Assessment:** **Portico ahead** on formats (the WebVTT thumbnail track is a de-facto standard many web players read). Caching is weaker than it could be (B-04.30).

### A-04.40 Trickplay on clients
- **Portico:** only the web app uses it.
- **Jellyfin clients, Plezy, Swiftfin, Streamyfin:** scrub thumbnails on TV and phone.
- **Assessment:** **Portico behind**; TV remotes benefit most from scrub previews. Medium per platform (client-core already resolves frames).

### A-04.41 How analysis is scheduled
- **Portico:** part of the library scan job as "deep" operations chosen by tier (basic / complete / custom); one stage quantum at a time, one job at a time; maintenance windows are hints only (`catalog/library_sources.go:105-135`, `mediaanalysis/work.go:38-44`, `workpolicy/policy.go:1-2`).
- **Jellyfin:** separate scheduled tasks per product (trickplay, chapter images, segments, subtitles), each with triggers, plus "during scan" switches.
- **Why it matters:** "Complete" in Portico turns on **seven** deep stages at once — a checksum read, three audio decodes and two full video decodes per file (`DeepScanOperations`).
- **Assessment:** **Trade-off.** Portico's single queue is simpler and never starves playback by design. The "Complete" bundle is a trap for weak hardware (B-04.35).

### A-04.42 Deep analysis depends on subtitle import
- **Portico:** trickplay, chapter images and markers will not start for a file until the scan-time subtitle text import has succeeded for that exact revision (`catalog/inventory.go:523-528`, `work.go:137-139`).
- **Jellyfin:** independent tasks.
- **Assessment:** **Portico behind** — an unrelated subtitle problem blocks previews (B-04.3).

### A-04.43 Analysis products nobody reads
- **Portico:** `fingerprint` output has no reader anywhere in the server; `checksum` and `waveform` for video have none that I found.
- **Assessment:** **Unclear** — possibly groundwork. Worth removing from "Complete" until something consumes them.

### A-04.44 Confined decoders for every subtitle and analysis job
- **Portico:** every FFmpeg/ffprobe run gets the file through a private loopback bridge with a protocol and format whitelist, inside a sandbox where available; sidecars are read by a helper process with `OpenRoot` confinement (`decoder/subtitle.go:65`, `acquisition.go:176-262`).
- **Jellyfin:** FFmpeg opens library paths directly.
- **Why it matters:** subtitle and container parsers are a classic attack surface; Portico's posture is far stronger. The cost is throughput (1 MiB extents with validation around each) and some format restrictions (B-04.34).
- **Assessment:** **Portico ahead.**

### A-04.45 Live TV captions
- **Portico:** web shows a toggle when hls.js exposes a caption track; first track only (`LiveCaptions.tsx`).
- **Assessment:** adequate start; no language/service choice (CC1/CC2), nothing on recordings played as VOD (captions inside the video stream are not discovered as tracks).

### A-04.46 Prepared versions, downloads and offline
- **Portico:** a session on a prepared (optimised) version refuses the subtitle plan and chapters entirely (`session.go:56-63`, `playback/chapters.go:74-90`); the downloads package has no subtitle handling.
- **Jellyfin / Plezy:** downloads carry external subtitles; Plezy downloads subtitle files alongside.
- **Assessment:** **Portico behind.** Since text resources are source-relative cue documents, serving them for a full-length derivative mostly needs a decision that the clocks match. Medium.

### A-04.47 Limits

| Limit | Portico | Note |
|---|---|---|
| Tracks per file | 32 (over → none) | Jellyfin: none |
| Scan-imported text tracks | 8, arbitrary order | — |
| HLS text renditions | 8 | — |
| Resources per item per viewer | 128 | — |
| Cues per track | 20,000; 8 KiB each | a 3-hour film with dense SDH is ~3,000–4,000 cues: fine |
| Text input | 4 MiB | fine |
| Image track | 32 MiB | too small for many Blu-ray tracks (B-04.11) |
| Media length | 24 h | fine |
| Offset | ±10 min | fine |
| Directory entries scanned | 4,096 | flat folders exceed it (B-04.27) |
| Chapters | 4,096 | fine |
| Markers per file | 512 | fine |
| Trickplay frames | 720 default budget | interval stretches |

### A-04.48 Windows servers
- **Portico:** the subtitle service disables embedded extraction on Windows (`service.go:64-68`), so every embedded track reads `extraction_unavailable` there.
- **Jellyfin:** same behaviour on all platforms.
- **Assessment:** **Portico behind** if Windows is a server target (see B-04.31 and the open questions).

## Missing features and capabilities to consider

Sizes are rough: **small** = days, **medium** = a week or two, **large** = a month or more or a new subsystem.

### F-04.1 Render styled ASS on the device
- **What:** draw ASS/SSA with its fonts, positions and effects in the client instead of flattening or burning.
- **Who:** jellyfin-web (`@jellyfin/libass-wasm`, `plugin.js:1395-1470`), jellyfin-androidtv (`ass-media` for Media3, opt-in), every mpv/VLC client (Plezy, Streamyfin, Swiftfin, Infuse).
- **Value:** anime and typeset fansubs look right with no transcode. This is the feature anime users judge a server by.
- **Size:** web medium (WASM renderer, keep original script, extract MKV font attachments, fonts route). Android medium. Apple large (AVPlayer has no hook; needs an overlay renderer fed by libass, or an alternate player engine).
- **Principles:** fits "quality changes only when a person asks"; no conflict.
- **Opinion:** do the web first. It also fixes the need for server fonts.

### F-04.2 Render image subtitles (PGS/VobSub) on the device
- **Who:** jellyfin-web (`libbitsub`), jellyfin-androidtv (Media3 PGS, default on), mpv clients.
- **Value:** Blu-ray subtitles without a 4K transcode. Large effect on weak servers.
- **Size:** web medium; Android medium; Apple large.
- **Opinion:** high value; the spec already promises it (§5.5).

### F-04.3 Character-set detection for legacy subtitles
- **Who:** Jellyfin (`SubtitleEncoder.cs:1006-1030`), mpv (`sub-codepage` auto).
- **Value:** Cyrillic, Greek, Central European, Chinese, Japanese, Korean, Hebrew and Arabic sidecars display correctly instead of as garbage.
- **Size:** small-to-medium. Use the language tag as a prior; let the owner override per track.
- **Opinion:** needed before release for non-Western users.

### F-04.4 Wider sidecar support
- **What:** `.smi/.sami`, MicroDVD `.sub`, `.ttml/.dfxp`, `.mks`; flags `hi`, `default`, `foreign`; language names (`English`), common codes (`chs`, `cht`, `pob`); unknown tokens kept as the title; optional `Subs/` and `Subtitles/` subfolders (Plex and Kodi read these; scene releases ship them).
- **Size:** small each. Subfolder search is small but needs care with the "exact folder" inventory model.
- **Opinion:** the flags are a must; formats are nice-to-have.

### F-04.5 Remember the viewer's track choice
- **What:** per viewer, per show (and per movie): last chosen audio (language + title) and subtitle (language + forced/SDH + title, or off), applied to the next episode and the next visit.
- **Who:** Jellyfin (server per-item + web carry-over), Plezy (intent matcher), Plex (per-show language overrides), Streamyfin.
- **Value:** probably the most-noticed missing behaviour for anyone watching dubbed/subbed series.
- **Size:** medium. Store on the server so every device agrees ("thin clients").
- **Opinion:** build it; copy Plezy's "match by meaning, decline on ambiguity" rule. The spec already lists `last_used`.

### F-04.6 More selection modes
- **What:** "follow the file's default/forced flags"; "original language" audio preference (play the title's original language when available); accept undefined-language forced tracks; fall back to forced in a preferred subtitle language.
- **Who:** Jellyfin (`MediaStreamSelector.cs`, `MediaSourceManager.cs:545-570`).
- **Size:** small (original-language needs the catalogue's original-language fact).
- **Opinion:** "original language" is popular with anime and world-cinema viewers.

### F-04.7 Subtitle appearance on every client
- **What:** size, colour, outline/shadow, background and opacity, font, vertical position; on native as well as web; honour the OS caption style where the platform has one (the accessibility contract A11Y-10.5 says "respects platform/user preferences").
- **Who:** jellyfin-web, androidtv, Plezy, Streamyfin.
- **Size:** web small (more `::cue` variables, or a custom overlay for position); native small-to-medium (the overlay is already Portico's own view).
- **Opinion:** the native apps currently have no appearance control at all; at least size and background should match web.

### F-04.8 Audio delay
- **Who:** Plezy (`audio-delay`, scoped persistence), Swiftfin, Kodi.
- **Size:** medium, per platform; remember per output route if possible.
- **Opinion:** worth having on TV platforms.

### F-04.9 Two subtitle tracks at once
- **Who:** jellyfin-web (`SecondarySubtitles`), Plezy (`secondary-sid`).
- **Value:** language learners; households mixing languages.
- **Size:** small for overlay clients (two cue documents), since Portico draws its own text.
- **Opinion:** cheap differentiator given the cue-document design.

### F-04.10 Automatic subtitle download
- **What:** at scan or on a schedule, fetch missing subtitles for the languages an owner lists, with rules: skip if an embedded/sidecar track exists, skip if audio already matches, prefer hash matches; providers beyond OpenSubtitles; provider sign-in from the web console rather than environment variables; search by IMDb/TMDB id, season/episode and file hash; show hearing-impaired and match quality in results.
- **Who:** Jellyfin (library options + plugins), Plex, Bazarr.
- **Size:** medium. The "subtitle_download" operation is already named in the analysis vocabulary but has no runtime (`administration/library_runtime.go:11`).
- **Principles:** network calls must respect library "local only" (already honoured in search, `provider.go:246-251`).
- **Opinion:** fix the search quality first (B-04.19); automatic download can wait.

### F-04.11 Optionally save subtitles beside the media
- See A-04.4. Medium. Helps "open data" and multi-app households.

### F-04.12 Hide or delete unwanted tracks
- **What:** an owner hides a discovered track (junk, duplicates) for everyone, or deletes a sidecar file.
- **Who:** Jellyfin deletes external subtitle files (`SubtitleController.cs:91-115`).
- **Size:** small for hide (a per-discovery flag); delete needs library write access.

### F-04.13 Cross-episode intro detection
- **What:** compare audio fingerprints of the first minutes of each episode in a season; the shared run is the intro. Refine edges with silence and key frames.
- **Who:** intro-skipper (Chromaprint), Plex.
- **Value:** the feature people mean by "Skip Intro".
- **Size:** large. Needs a season-scoped analysis job, a fingerprint store, and a matcher. FFmpeg's `chromaprint` muxer needs libchromaprint in the bundled FFmpeg; Portico's own `fingerprint` stage would need validating for this purpose (one window per second may be too sparse).
- **Principles:** background, bounded per season; fine.
- **Opinion:** the most valuable missing feature in this domain. Results from a measured detector can legitimately clear the 0.85 auto-skip bar the design already has.

### F-04.14 Credits detection that works
- **What:** black-frame search near the end, refined by a "text card" visual detector for non-black credits; cap by min/max duration; treat movies and episodes differently.
- **Who:** intro-skipper (`BlackFrameAnalyzer`, `Analyzers/Credits/*`), Plex.
- **Size:** medium on top of the existing scene sampler, if restricted to the last N minutes rather than a full decode.
- **Opinion:** also lets "Next episode" appear at the right time.

### F-04.15 Other marker sources
- **What:** configurable chapter-name patterns per type and language; EDL/comskip files beside recordings (commercials for DVR); recap/preview from subtitle text; "preview" type.
- **Who:** intro-skipper, Kodi (EDL), Plezy (client-side chapter regex fallback).
- **Size:** small each. EDL import is especially relevant because Portico has a DVR.

### F-04.16 Skip polish
- **What:** cancellable auto-skip countdown; hide the button after N seconds; "don't skip the first episode's intro"; a viewer preference for commercials; credits skip that goes to the next episode.
- **Size:** small.

### F-04.17 Cheaper and reusable trickplay
- **What:** key-frame-only extraction; hardware decode; several widths; optional storage beside media; **import existing Jellyfin `.trickplay` folders and Plex BIF files** instead of regenerating.
- **Who:** Jellyfin (`TrickplayOptions.cs`), Plezy reads both formats.
- **Size:** key-frame mode small; import medium; hardware decode medium.
- **Opinion:** importing existing tiles is a strong migration sweetener ("your previews come with you").

### F-04.18 Trickplay and chapters on the native apps
- Scrub previews on Apple TV / Android TV / phones; a chapter list in the native player. Medium. The media-presentation contract already makes the web player's capability set the minimum for native.

### F-04.19 Better chapter handling
- Dummy chapters for files with none (Jellyfin); pick a representative, non-black frame for the thumbnail; tolerate one bad chapter. Small.

### F-04.20 OCR image subtitles to text (optional)
- **What:** convert PGS/VobSub to text once on the server so they become ordinary text tracks.
- **Who:** not built into Jellyfin or Plex; done by add-on tools.
- **Value:** would remove burn-in for Apple devices, where client bitmap rendering is hard.
- **Size:** large (an OCR engine and language data in the sandbox); accuracy is imperfect.
- **Opinion:** only if Apple bitmap rendering proves impractical. Record as an option, not a recommendation.

### F-04.21 Automatic subtitle re-timing
- Align a mistimed subtitle to the audio (tools like ffsubsync do this from voice activity). Portico already stores a per-track offset; a computed offset would fit that slot. Medium. Low priority.

### F-04.22 Standards-level HLS subtitle signalling
- `FORCED=YES`, `AUTOSELECT/DEFAULT=YES` for the chosen track, `CHARACTERISTICS="public.accessibility.transcribes-spoken-dialog,public.accessibility.describes-music-and-sound"` for SDH, and `CLOSED-CAPTIONS=` for 608/708 in live and recorded TV. Small. Makes third-party and system players behave correctly.

### F-04.23 External audio tracks
- A commentary or dub in a `.mka`/`.ac3` beside the video (Jellyfin supports this). Medium (the delivery planner assumes one input). Low priority.

### F-04.24 Subtitles for downloads and prepared versions
- Ship selected-language cue documents with an offline download and allow them on full-length prepared versions. Medium. The protocol spec's download request already has `subtitles: [langs]`.

### F-04.25 Owner controls for extraction
- Choose "extract at scan" vs "on first use"; a timeout proportional to file size; which languages to pre-extract (for example the server's preferred languages first). Small. Jellyfin exposes a timeout; Portico hard-codes 90 s and "first eight".

### F-04.26 Consistent, localised track labels
- Jellyfin builds a display title such as "English – SDH – Default – SRT – External". Portico clients show the stored title or language. A server-composed label (thin-client principle) that distinguishes forced / SDH / commentary / format / source would help pickers with many similar tracks. Small.

### F-04.27 Subtitle upload UI
- The server API and an iOS file picker module exist, but no web or native screen calls them. Small to finish.

## Possible bugs and risks in Portico

All of these come from reading source. None was reproduced. "Confirmed by reading" means the code path is unambiguous; how often it bites in real libraries still needs a runtime check. Unless stated, **`Backlog.md` does not list the item** (its only entry in this domain is "Cast receiver: sidecar subtitles aren't offered as CAF text tracks").

### B-04.1 Sidecar subtitle files are never imported automatically, so they are never auto-selected
- **What happens:** the scan-time import skips every track whose origin is not `embedded` (`subtitles/scan_text.go:46`). Auto-selection, the HLS manifest and playback options all read imported resources only (`autoselect.go:134`, `manifest.go:18-30`, `playbackv1/options.go:500-506`). So a plain `Movie.en.srt` beside the film — the most common subtitle arrangement there is — is not turned on by "always show subtitles: English", is not in the HLS manifest (so not in PiP/AirPlay/Cast), and is not listed in playback options or on the title page. It appears only in the in-player list as an un-imported track; picking it imports it for that one viewer.
- **How the comparison surfaced it:** Jellyfin ranks external text subtitles *first* in its default order.
- **Confidence:** Confirmed by reading. **Severity:** High (core expectation of a media server).

### B-04.2 The scan imports an arbitrary eight text tracks, not the useful ones
- **What happens:** the import loop walks discovered tracks in `ORDER BY t.asset_id,t.id` (`service.go:310`) where `t.id` is a SHA-256 of asset, origin and stream index (`subtitles/persist.go:45`), and stops after eight (`scan_text.go:49`). For a file with more than eight text tracks (common for streaming-service rips), which eight are imported is effectively random with respect to language, default flag or stream order. English can be left out. The HLS manifest then takes eight in resource-id order, also random (`manifest.go:23`).
- **Confidence:** Confirmed by reading. **Severity:** Medium.

### B-04.3 One bad or empty text track blocks the rest of the file's subtitles and all deep analysis
- **What happens:** `ImportScanText` returns on the first failed import (`scan_text.go:53-56`). A track with no cues (empty forced tracks are common), an ASS track the strict validator rejects (B-04.10), or a timeout (B-04.4) makes the scan record `subtitle_text_import_failed` (`ingestion/analysis.go:483-485`). Because the file's `analysis_revision` is only set after the import completes (`catalog/inventory.go:523-528`, `inventory_text_import.go:35`), the deep stages then refuse the file (`mediaanalysis/work.go:137-139`): no trickplay, chapter thumbnails or markers for it, with nothing connecting the two in the owner's view.
- **Confidence:** Confirmed by reading (logic). **Severity:** Medium.

### B-04.4 The 90-second budget covers up to eight full reads of the file
- **What happens:** the budget starts before the single global slot is acquired and spans every track (`scan_text.go:25-32`). Each track is its own FFmpeg run over the whole file (`subtitlevideo/runtime.go:206-230`). A 40 GB remux on a hard disk or NAS cannot be read once in 90 s, let alone several times. Result: B-04.3's consequences for exactly the large, high-value files.
- **How surfaced:** Jellyfin extracts all tracks in one pass with a 30-minute default timeout.
- **Confidence:** Probable (arithmetic on typical disk speeds; needs a timed run). **Severity:** High for remux libraries.

### B-04.5 A file with more than 32 subtitle tracks gets none
- **What happens:** `discover` replaces the inventory with status `capacity` and `Tracks=nil` when embedded tracks exceed 32 (`inventory.go:44-48`). If embedded ≤ 32 but embedded + sidecars > 32, all sidecars are dropped and the scan warns (`inventory.go:77-79`, `:83-92`). Apple TV+ and some Disney+/Netflix rips carry 35–45 subtitle tracks.
- **Confidence:** Confirmed by reading. **Severity:** Medium.

### B-04.6 Common sidecar names are disabled as "ambiguous"
- **What happens:** after the language, any further token other than `forced`, `sdh`, `cc` marks the file `ambiguous_association` and it cannot be used (`inventory.go:103-116`). That catches `Movie.en.hi.srt` (the hearing-impaired suffix Bazarr writes by default), `Movie.en.default.srt` and `Movie.en.foreign.srt` (Jellyfin/Plex conventions), and any descriptive name like `Movie.en.Commentary.srt`. A language *name* (`Movie.English.srt`) is kept but loses its language (`und`), as do `chs`, `cht`, `pob`, `jp`. A lone `Movie.forced.srt` is fine; `Movie.foreign.srt` becomes an un-forced `und` track.
- **Confidence:** Confirmed by reading. **Severity:** Medium.

### B-04.7 A second video with the same base name disables every sidecar for both
- **What happens:** `Movie.mkv` + `Movie.mp4` (or a `.strm` twin) in one folder sets `ambiguous` and every matching sidecar is disabled (`inventory.go:66-68`, `:118-120`).
- **Note:** deliberate (comment at `inventory.go:36-38`). Jellyfin attaches the sidecar to both.
- **Confidence:** Confirmed by reading. **Severity:** Low–Medium.

### B-04.8 The client has no message for the server's "ambiguous" reason
- **What happens:** the server sends `ambiguous_association`; the client's table knows `ambiguous_sidecar` (`client-core/src/subtitles.ts:128`), so the viewer gets the generic "This subtitle is unavailable…" text.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-04.9 Non-Western legacy encodings are shown as garbage, silently
- **What happens:** any SRT/VTT that is not valid UTF-8/UTF-16 is decoded as Windows-1252 (`tolerant.go:54-69`). Windows-1252 accepts every byte, so a Windows-1251 Russian file or a GBK Chinese file "succeeds" and is stored as nonsense text; nothing flags it. ASS, SSA and VobSub `.idx` files in a legacy encoding are rejected instead (`document.go:84-116` via `render_asset.go:139`).
- **Confidence:** Confirmed by reading. **Severity:** Medium (High for affected languages).

### B-04.10 One odd line makes a whole ASS script unusable
- **What happens:** the validator returns an error for the entire script if any Dialogue line has end ≤ start (zero-length lines are used as markers by some typesetters), a time not in exactly `H:MM:SS.cc` form (some tools write three decimals), or ends more than 2 s after the film; or if the script has a `[Fonts]`/`[Graphics]` section (`render_asset.go:171-232`, `:244`). This runs before the plain-text conversion, so even the "text" rendering is lost. For embedded tracks it feeds B-04.3.
- **How surfaced:** the SRT path was deliberately made tolerant (`tolerant.go:17-28`); the ASS path was not.
- **Confidence:** Confirmed by reading; prevalence Probable. **Severity:** Medium.

### B-04.11 Image subtitle tracks over 32 MiB cannot be used
- **What happens:** extraction output is capped at 32 MiB (`decoder/subtitle.go:12`), `.sup` sidecars at 32 MiB (`inventory.go:127-130`), and the asset at 32 MiB of data (`render_asset.go:17`, `:44-48`). Full-length Blu-ray PGS tracks are frequently 30–60 MB (more for SDH). Such a track fails to import, and since burn-in needs the imported asset, it cannot be shown at all.
- **Confidence:** Probable (size expectation from typical discs; the cap itself is confirmed). **Severity:** Medium–High for Blu-ray remux libraries.

### B-04.12 First use of an image track must read the whole file inside a 110-second request
- **What happens:** picking an un-imported PGS track calls Import, which extracts the track by reading the entire file (`acquisition.go:101-110`), while the client aborts non-GET subtitle requests after 110 s (`client-core/src/subtitles.ts:153`). On a large remux the request is cancelled before extraction finishes; the retry starts from zero.
- **Confidence:** Probable. **Severity:** Medium.

### B-04.13 Burned image subtitles may be squashed or clipped when the video is not the subtitle's shape
- **What happens:** the filter forces `-canvas_size` to the video's width×height and scales the subtitle stream to exactly that (`decoder/burn_in.go:46-47`, sizes from the source video at `playback/hls_burn.go:59-61`). A 1920×800 cropped encode with 1920×1080 PGS would have its subtitles compressed vertically or positioned off the picture.
- **How surfaced:** Jellyfin special-cases exactly this (own canvas, aspect-preserving scale, pad/crop) (`jellyfin/MediaBrowser.Controller/MediaEncoding/EncodingHelper.cs:984-1008`, `:3431-3478`).
- **Confidence:** Speculative — depends on how FFmpeg's bitmap pipeline treats `-canvas_size` for PGS; needs a runtime check with a cropped encode. **Severity:** Medium if real.

### B-04.14 Burned-in styled subtitles can lose their glyphs or fonts
- **What happens:** `ass=filename=…` is used with no fonts directory (`decoder/burn_in.go:30`), embedded font attachments are never extracted, and the only guaranteed font is DejaVu Sans (`mediaexec/fonts.go:22-23`), which has no CJK, Thai or Indic coverage. On a host without system fonts (containers), such text burns as boxes; everywhere, typeset fonts are replaced.
- **Confidence:** Probable. **Severity:** Medium for anime/CJK.

### B-04.15 Audio language preferences fail for languages outside a 57-entry table
- **What happens:** `languageKey` maps three-letter codes through a hand-written table (`playback/delivery_decision.go:263-287`). A file tagged `mal`, `kan`, `mar`, `pan`, `guj`, `afr`, `bos`, `mkd`, `sqi/alb`, `kat/geo`, `hye/arm`, `eus/baq`, `glg`, `cym/wel`, `gle`, `khm`, `lao`, `mya/bur`, `nep`, `sin`, `swa`, `amh`, `kaz`, `uzb`, `aze`, `mon`, `yue`, `cmn`… never equals the viewer's two-letter preference. Two effects: the preferred audio track is not chosen; and `automaticSubtitles` then treats the audio as **foreign** and turns subtitles on (`delivery_context.go:160-169`).
- **Note:** `golang.org/x/text/language` (already used by `subtitles.Language`) canonicalises all of these.
- **Confidence:** Confirmed by reading. **Severity:** Medium for those languages.

### B-04.16 Forced-track matching uses a second, smaller table
- **What happens:** `subtitleLanguageKey` has its own 35-entry table (`subtitles/autoselect.go:57-62`). Stored subtitle languages are canonical two-letter tags, but the audio language arrives raw from the probe. For `bul`, `cat`, `hrv`, `srp`, `slk/slo`, `slv`, `est`, `lav`, `lit`, `isl/ice`, `fas/per`, `ben`, `tam`, `tel`, `urd`, `fil/tgl`, `msa/may` (all in the other table but not this one) the forced track never matches the audio.
- **Confidence:** Confirmed by reading. **Severity:** Low–Medium.

### B-04.17 The hearing-impaired flag from the file is ignored
- **What happens:** the probe reads `disposition.hearing_impaired` (`assets/assets.go:161`, `:217`) but `subtitles.Track` has no such field (`inventory.go:19-25`, `ingestion/analysis.go:408`). SDH is inferred only from title words (`autoselect.go:64-67`). An SDH track titled just "English" is treated as ordinary and may be auto-selected ahead of the real one.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-04.18 Forced tracks with no language tag are never auto-selected; file "default" flags are ignored
- **What happens:** the forced rule needs `language == audio language` and returns nothing when either is undefined (`autoselect.go:102-112`). Many rips leave the forced track untagged. Separately, `Default` is only used to order candidates (`autoselect.go:134`), never to choose.
- **Confidence:** Confirmed by reading (design gap). **Severity:** Low–Medium.

### B-04.19 Online search for an episode sends only the episode's title
- **What happens:** with no typed query, the server searches OpenSubtitles for the item's own `title` (`provider.go:254-259`) — for an episode that is "Pilot" or "Episode 3" — with no series name, season/episode numbers, year, IMDb/TMDB id or file hash (`provider.go:138`). The language is cut to its first subtag, so `pt-BR` and `zh-TW` become `pt` and `zh`. The web client passes the **UI locale** as the language, not the viewer's subtitle language (`web/src/player/Player.tsx:551`).
- **Confidence:** Confirmed by reading. **Severity:** Medium (feature mostly unusable for TV).

### B-04.20 The OpenSubtitles sign-in is a static environment variable
- **What happens:** `PORTICO_OPENSUBTITLES_TOKEN` is read once at start (`provider.go:69`). The provider's user tokens are short-lived (about a day, per its public API documentation), and anonymous use has a very small daily download quota. There is no login or refresh, and a `429` only delays the next call.
- **Confidence:** Probable (relies on the provider's documented behaviour, not verified here). **Severity:** Low–Medium.

### B-04.21 Unreviewed luminance guesses are shown to viewers as Skip buttons, and the intro guess is often backwards
- **What happens:** `ViewerMarkersTx` projects every non-dismissed marker regardless of approval or confidence (`mediaanalysis/viewer_markers.go:90-110`); clients show a Skip button for any marker whose mode is not `off` (`web/src/player/SkipPrompt.tsx:23`). The "intro" candidate is *start of file → first black gap between 10 s and 180 s* (`detectors.go:73-79`): for a show with a cold open that is the opening scene, not the titles. "Commercial" is any stretch between two black gaps 15 s–4 min apart in the middle of the programme (`:81-87`); "credits" is any ≥10 s dark-with-highlights run in the last fifth, extended to the end (`:88-100`). The stage's own summary says "owner review required" (`produce.go:313`), but nothing enforces review on the viewer path.
- **Mitigation present:** these are never auto-skipped (`segmentmarkers/markers.go:63-71`).
- **Confidence:** Confirmed by reading. **Severity:** Medium (owners who enable "Complete" will see wrong Skip prompts across the library).

### B-04.22 The same stretch is emitted as both "intro" and "recap"
- **What happens:** when the first black gap is within 90 s, two markers with identical ranges are stored (`detectors.go:75-78`). The prompt shows whichever sorts first by id (`viewer_markers.go:91`), so the label is arbitrary, and the two follow different viewer preferences.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-04.23 One imperfect chapter discards all chapters
- **What happens:** any chapter that overlaps the previous one, has zero length, or ends more than 0.1 s after the container duration sets the whole list to "invalid" (`assets/assets.go:237-242`). Chapter times are compared with the *duration*, not with start+duration, so a container whose `start_time` is not zero may fail on its last chapter. The file is also flagged with a local-metadata issue.
- **How surfaced:** Jellyfin keeps start times and ignores ends.
- **Confidence:** Confirmed by reading (rule); the non-zero-start case is Speculative. **Severity:** Low–Medium.

### B-04.24 A title-page subtitle choice can silently do nothing
- **What happens:** the title page offers subtitle languages from the file's raw streams (`native/src/screens/title/PlaybackChoices.tsx:42`), but the pending-choice rule looks for an *imported* resource in that language and clears the choice whether or not it found one (`client-core/src/pending-track-choice.ts:84-88`). Picking a language that only exists as an image track, a sidecar, or an un-imported text track starts playback with no subtitles and no message.
- **Confidence:** Confirmed by reading. **Severity:** Medium (looks like a broken control).

### B-04.25 Curly-brace text is deleted from SRT and WebVTT
- **What happens:** the tolerant parser removes every `{…}` block, not just ASS override tags beginning with a backslash (`tolerant.go:44`, `:113`). Subtitles that write sound descriptions as `{LAUGHS}` lose them; a cue consisting only of that is dropped.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-04.26 Overlapping cues share italic and top placement on the native overlay
- **What happens:** the Apple renderer joins all active cues into one string and ORs their flags (`Native/PorticoSubtitleSelection.m:131-138`). If a top-placed sign overlaps normal dialogue, both jump to the top (covering what "top" was meant to avoid); if one is italic, all are. The web path keeps cues separate.
- **Confidence:** Confirmed by reading for Apple; Android header suggests the same single `Display` model (`PorticoSubtitleSelection.kt:32`). **Severity:** Low.

### B-04.27 Flat folders with more than 4,096 entries lose all sidecars, and discovery re-lists the folder for every video
- **What happens:** enumeration stops with `capacity` after 4,096 directory entries (`inventory.go:57-60`); the status is not `known`, so the scan warns `sidecar_analysis_incomplete` for every video in that folder (`ingestion/analysis.go:487-498`). A single flat "Movies" folder of ~1,400 films with one subtitle and one NFO each reaches the limit. Separately, each video lists its whole folder (`inventory.go:40-42`), which is quadratic work in flat folders (the Backlog notes the same pattern for metadata sidecars only).
- **Confidence:** Confirmed by reading (cap); Probable (cost — depends on whether the storage client caches listings). **Severity:** Medium for flat libraries.

### B-04.28 Any decode error aborts an analysis stage
- **What happens:** analysis FFmpeg runs with `-xerror` (`decoder/analysis.go:39`), which exits on the first decoding error. Real-world files with a single damaged frame are common and otherwise play fine; for those, trickplay, scene sampling and loudness fail as a whole stage.
- **Confidence:** Probable. **Severity:** Low–Medium.

### B-04.29 Every trickplay frame is stored twice
- **What happens:** each frame is saved as its own JPEG artifact and again inside a sprite sheet (`mediaanalysis/produce.go:249`, `:253-257`, `:201-213`). Only sheets are served (`trickplay.go:356-368`). Roughly doubles trickplay storage against the 32 GiB cap.
- **Confidence:** Confirmed by reading (unless an owner-facing view uses the single frames — not found). **Severity:** Low.

### B-04.30 Preview images are re-downloaded every five minutes
- **What happens:** tiles and chapter images are immutable by digest but served `private, max-age=300`; an `ETag` is sent but `If-None-Match` is never checked, so there is no `304` (`httpapi/trickplay.go:55-59`). A 3200×1800 sheet is re-fetched in full during a long film.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-04.31 On a Windows server no embedded subtitle can be used
- **What happens:** `subtitles.New` clears its FFmpeg path on Windows (`service.go:64-68`), and every embedded discovery is then reported `extraction_unavailable` (`service.go:333-335`). The comment says extraction needs `/dev/fd`, but the current extractor uses a loopback HTTP bridge (`subtitlevideo/runtime.go:214-219`), so the gate looks stale.
- **Confidence:** Confirmed by reading. **Severity:** High if Windows servers are in scope, otherwise none.

### B-04.32 Subtitle reads and a viewer's subtitle selection queue for the single write slot at background priority
- **What happens:** `Service.tx` always calls `dbwork.Begin(…, ClassBackgroundMedia)` (`service.go:97-103`). It is used by `List`, `Select`, `OpenDocument`, `CheckDocument`, `OpenManifestDocument`, `CheckManifestDocument`, `Import` and `Search`. So: the every-25-seconds document re-authorisation, **each 32 KiB chunk** of a document body (two checks per chunk, `httpapi/subtitle_routes.go:353-369`), every HLS WebVTT window, and the viewer's own "turn subtitles on" all wait for the one database write gate in the same class as a library scan (only age-promotion lifts them). `Plan` alone was moved to a read snapshot ("NEW-38", `session.go:67-77`).
- **Consequence:** during a heavy scan, subtitle selection can lag and the periodic re-authorisation can time out (15 s client timeout), after which cues go dark at 40 s.
- **Confidence:** Confirmed by reading (classes and call sites); runtime effect needs measurement. **Severity:** Medium.

### B-04.33 A vestigial second subtitle-plan system still writes rows, and several functions are dead
- **What happens:** every HLS session start runs `persistSubtitlePlan`, inserting into `playback_subtitle_plans`/`playback_subtitle_tracks` with its own nine-track limit and `capacity` state (`playback/subtitle_plan.go:47-124`, called at `playback/service.go:437`). Nothing reads them: `loadSubtitlePlan` and `subtitlePinValid` have no callers. In the subtitles package `Parse` (strict), `WebVTT`, `ClockCollector`, `MuxOffset`, `textASS`, `styledText`, `positionASS`, `parseCueCSS` have no production callers.
- **Risk:** wasted writes per session; a future reader may trust the stale plan; ~700 lines that look load-bearing but are not.
- **Confidence:** Confirmed by reading (grep for callers across `internal/` and `cmd/`). **Severity:** Low.

### B-04.34 Embedded subtitles in some containers cannot be extracted
- **What happens:** extraction whitelists only `mov,matroska,avi,mpegts` demuxers (`decoder/subtitle.go:65`), while the scanner accepts `.vob`, `.mpg`, `.mpeg` too (`assets/assets.go:262`). DVD subtitle streams in a `.vob`/`.mpg` are discovered and then fail to import.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-04.35 "Complete" scanning is far heavier than its label suggests (design risk)
- **What happens:** the Complete tier enables all seven deep stages (`catalog/library_sources.go:105-106`, `:128-129`): a checksum read, three separate audio decodes (waveform, loudness, fingerprint), two full single-threaded video decodes (trickplay, scene sampling) and per-chapter seeks — per file, one at a time. Two of the outputs (fingerprint; waveform/checksum for video) have no consumer I could find. The trickplay and scene passes could share one decode.
- **Confidence:** Confirmed by reading. **Severity:** Medium on weak hardware (weeks of background load for products mostly unused).

### B-04.36 The v1 session says "webvtt" but the URL returns JSON
- **What happens:** a session's `presentation.subtitles[]` entry is built as `{format:"webvtt", url: plan.DocumentURL}` (`playbackv1/subtitles.go:76`), but that URL serves the JSON cue document (`application/json`) unless the path ends in `.vtt` (`httpapi/subtitle_routes.go:258-298`, `:347`), and the `.vtt` form only works for tracks pinned into an HLS manifest (`subtitles/manifest.go:43-46`; pinning happens only when `mode == "hls"`, `playback/service.go:429-433`). A third-party client following the spec on a direct-play session gets JSON labelled WebVTT and has no WebVTT route.
- **Confidence:** Confirmed by reading. **Severity:** Medium for the "implementable by a third party" promise; none for first-party clients.

### B-04.37 HLS subtitle renditions carry no default, auto-select or forced signalling (design risk)
- See A-04.16. A plain HLS player never shows the session's selected or forced track on its own. **Confidence:** Confirmed by reading (`playback/hls_renditions.go:190`). **Severity:** Low–Medium.

### B-04.38 HDR and anamorphic sources produce poor preview images
- **What happens:** trickplay, scene and chapter-image filters convert straight to RGB/JPEG with no tone-mapping (`decoder/analysis.go:79`, `:98`), so HDR10/HLG previews look grey and washed out and Dolby Vision profile 5 previews have wrong colours. Scaling uses pixel dimensions and then forces `setsar=1`, so anamorphic DVDs are squeezed.
- **How surfaced:** Jellyfin tone-maps still images (`jellyfin/MediaBrowser.MediaEncoding/Encoder/MediaEncoder.cs:729-745`) and corrects height from the display aspect for trickplay (`:880-893`); its software trickplay path also skips tone-mapping.
- **Confidence:** Probable. **Severity:** Low–Medium.

### B-04.39 The web renderer rebuilds every cue whenever the HLS mapping changes
- **What happens:** on each `FRAG_CHANGED`, if `startPTS − start` differs as a string from the last value, all cues are removed and re-added (`web/src/bridge/useWebPlaybackAdapter.ts:228-239`, `web/src/player/options.ts:117-146`). Floating-point noise between fragments could make that every few seconds, briefly clearing the on-screen line.
- **Confidence:** Speculative — needs observation in a browser. **Severity:** Low.

### B-04.40 The owner-facing description of segment detection does not match what runs
- Listed under Doc/code disagreements; noted here because an owner enabling it expects cross-episode intro detection and gets B-04.21.

## Where Portico is stronger

- **Security posture.** Every subtitle and analysis decode is confined (sandbox, loopback bridge, protocol and demuxer whitelists, bounded output); sidecars are read by a confined helper with replacement fencing. Subtitle text reaches clients as data, never as markup. No competitor does this.
- **Source fencing.** Every resource, pin and document read is tied to the exact file size and modification time; a replaced file can never show the old file's subtitles, chapters, markers or tiles.
- **Per-viewer and shared subtitles with revisions.** Personal uploads and downloads, owner-shared tracks, idempotent operations, attribution and a rights note, 24-hour grace for superseded revisions still pinned by a session.
- **Permanent per-track timing correction.** An owner can fix a badly timed track once for everyone.
- **Tolerant SRT/WebVTT reading with a clear rationale** (`tolerant.go:17-28`): ordinary subtitles never trigger a transcode.
- **Marker trust model.** One server rule decides unattended skipping; clients cannot infer it; owner approve/dismiss/edit with history; skip reports flow back. A better foundation than Jellyfin's plain segment table.
- **Automatic choice that never costs anything at start.** Selection reads only what is already stored, so it cannot delay or fail a start.
- **Audio selection avoids commentary tracks** unless nothing else exists.
- **In-place audio switching on HLS** through audio renditions.
- **Multi-track HLS subtitle renditions**, so PiP, AirPlay and Cast can show text tracks natively.
- **Stable track identity** — no renumbering when sidecars come and go.
- **Trickplay as a standard WebVTT thumbnail track** plus a JSON descriptor; bounded frames per file; fenced to the source revision with an explicit `stale` flag.
- **Background analysis never competes with playback by design** (one job, background priority, admission hook).
- **The text-preferred swap and the 4K/HDR burn warning** in the web player are thoughtful touches.
- **Subtitle search respects "local only" libraries.**

## Doc/code disagreements

| # | Document says | Code does |
|---|---|---|
| 1 | Spec §4.1: playback options list every subtitle stream (`embedded|sidecar|downloaded`, any format such as `pgs`) | Only imported resources are listed (`playbackv1/options.go:500-506`); discovered tracks are absent |
| 2 | Spec §4.1: `chapters[].thumbnail`, `markers[].confidence: detected|manual|embedded`, a `trickplay` object in playback options | Chapters carry only `startMs`,`title` (`options.go:88-91`); markers carry `automaticSafe`, and `confidence` is hard-coded to `"detected"` even for owner-made markers (`httpapi/playback_v1_routes.go:384`); `outro` is renamed `credits`; trickplay is a separate route (`/v1/items/{id}/trickplay`) |
| 3 | Spec §4.1/§5.4: `preferred.subtitleId` and reason `profile_language|last_used|default`; audio default "then last-used" | `Preferred.SubtitleID` is never set (`options.go:586-620`); nothing stores a last-used track |
| 4 | Spec §4.2: `GET /v1/items/{id}/segment-markers`; marker types include `preview` | No such route; markers arrive inside detail, offers and playback options. Kinds are intro, recap, credits, commercial, outro, (owner) chapter — no preview |
| 5 | Spec §4.3: `GET …/subtitles:search`, `POST …/subtitles:download {candidateId}` → Operation; candidates have `format, hearingImpaired, score`; "owner-configurable provider"; members need `subtitles.download` | `POST /v1/items/{id}/subtitles/search` and `/apply` (synchronous); candidates have `id, language, title, attribution`; provider is environment-variable only; no permission check beyond library access |
| 6 | Spec §4.4: tile sheets "JPEG/WebP" | JPEG only |
| 7 | Spec §5.5: clients declaring `render: client` get ASS, PGS and VobSub in original form (D-FEAT-3) | Never delivered in original form; `embeddedClient` is a no-op that selects nothing (`playbackv1/subtitles.go:63-66`). D-FEAT-3 and D-MEDIA-7 are cited but not found in `decisions.md` |
| 8 | Spec §5.1/§5.5: `presentation.subtitles[]` is a WebVTT sidecar URL | The URL returns the JSON cue document (B-04.36) |
| 9 | Spec §5.5: "`subtitleOffsetMs` is client-side for sidecars" | The offset is server state per session; clients send it and read it back |
| 10 | `04 Playback and Media.md`: "text subtitles are delivered as WebVTT renditions where the client supports them" | First-party clients fetch a JSON cue document and draw it; WebVTT exists only as HLS renditions |
| 11 | `04 Playback and Media.md`: "detected intro/credits markers (`segmentmarkers`)" | `segmentmarkers` is only the shape and the skip rule; detection is in `mediaanalysis` and is a low-confidence heuristic |
| 12 | Owner UI text for *segment_detection*: "Compares episodes within a season to find repeated openings" (`administration/library_settings.go:67`) | Chapter-title matching plus single-file luminance; no cross-episode comparison |
| 13 | Owner UI lists *subtitle_discovery* as "metadata-only… one small read per file" | It also triggers up to eight full-file extraction passes (`ingestion/analysis.go:439-486`) |
| 14 | Owner UI text for *audio_fingerprint*: "used to match recordings that tags cannot identify", music only | The stage runs for any file with audio under "Complete" and has no consumer |
| 15 | API guide (`server/api/guide/04-playback-v1.md`) is meant to let a third party implement playback | Subtitles are mentioned once in passing; the cue-document format, 25 s re-authorisation, import step and selection route are undocumented |
| 16 | Contract A11Y-10.5: caption appearance respects platform/user preferences | Web has size and background; native apps have no appearance settings and do not read OS caption styles |
| 17 | Media-presentation contract §9: the web player's capability set is the minimum for handheld and TV | Native apps lack trickplay previews, a chapter list and subtitle appearance |
| 18 | Contract A11Y-10.1: track choices expose kind (`CC`, `SDH`, `forced`), commentary/descriptive purpose | SDH/commentary/descriptive are guessed from title words; the file's own flags are dropped for subtitles (B-04.17) |
| 19 | Subtitle error text: "SRT, WebVTT, ASS/SSA, PGS and paired VobSub are supported within the documented limits" (`subtitle_routes.go:66`) | The limits are not documented anywhere I found |

## Open questions for Justin

1. **Import model.** Should a discovered text track (especially a sidecar) be directly selectable and auto-selectable, with import as an invisible cache step? (A-04.1, B-04.1)
2. **Scan-time extraction.** Keep it, make it one pass per file with a size-proportional budget, or move to first use? Should owners choose? (A-04.2)
3. **Client rendering.** Is on-device rendering of ASS and of PGS in scope before release — at least on web — as the spec says, or is "flatten or burn" the accepted v1 position? (A-04.9, A-04.10)
4. **Apple strategy for image subtitles.** Burn-in forever, an alternate player engine, or OCR-to-text?
5. **Encodings.** Is automatic charset detection acceptable, given the code's explicit "no locale-dependent guessing" stance? (A-04.6)
6. **Sidecar strictness.** May unknown filename tokens become a title instead of disabling the file? Should a same-named second video stop sidecars from attaching? (B-04.6, B-04.7)
7. **Remembered tracks.** Per show, per title, or both? Stored on the server per profile? Should it override global language preferences? (A-04.23)
8. **Subtitles beside the media.** Should Portico ever write a downloaded subtitle next to the file (opt-in), given "open data"? (A-04.4)
9. **The 40-second "go dark" rule.** Is revoking already-fetched subtitle text worth the failure mode? (A-04.15)
10. **Intro detection.** Is cross-episode audio fingerprinting wanted for v1? If not, should luminance guesses be hidden from viewers until an owner approves them? (A-04.32, B-04.21)
11. **Marker defaults.** Should chapter-labelled markers (an explicit "Intro" chapter) be auto-skippable? Today they are deliberately not.
12. **"Complete" tier.** Should it include checksum, waveform and fingerprint for video, given nothing reads them? (B-04.35)
13. **Trickplay.** Key-frame-only by default? Import existing Jellyfin/Plex previews? Allow storage beside media? Which native clients get scrub previews first?
14. **Windows.** Is a Windows server a supported target? If yes, B-04.31 is a release blocker.
15. **Limits.** Are 32 tracks per file, 8 pre-imported tracks, 32 MiB image tracks and 4,096 directory entries intended product limits or placeholders?
16. **Online subtitles.** Provider configuration in the web console with sign-in? A per-profile permission, as the spec says? Automatic download?
17. **Third-party contract.** Should plain players get a real WebVTT/SRT URL on direct-play sessions? (B-04.36)
18. **Appearance.** Which settings are wanted, and should they follow the OS caption style on Apple and Android?

## Evidence boundary

- **Nothing was executed.** No server, build, test, FFmpeg or client was run. Every finding is from reading source in the working tree as it stood on 11 October 2026.
- **Runtime-dependent claims** are marked Probable or Speculative: extraction timing (B-04.4, B-04.12), PGS sizes (B-04.11), FFmpeg bitmap canvas behaviour (B-04.13), font fallback (B-04.14), `-xerror` prevalence (B-04.28), write-gate latency (B-04.32), preview colour (B-04.38), cue flicker (B-04.39), OpenSubtitles token lifetime (B-04.20).
- **Not read or only skimmed in Portico:** `subtitlevideo/remote_storage.go`, `orphans.go`, `window_input.go`; `mediaanalysis/{view,target,retention,publisher,capture,source_evidence,verify_source,artifact_custody}.go`; `internal/audiofacts`; `client-core/src/{audio-selection,media-analysis}.ts`; most of `web/src/player/Player.tsx` and `engine.tsx` beyond subtitle/marker/trickplay lines; the Android subtitle module beyond its header; the Vega app's `bridge/subtitles.ts`; the Cast receiver; the native `VideoPlayer.tsx`/`PlayerOptions.tsx` beyond grep hits; test files (which may encode intent I have not seen); the HLS production command builder as a whole (so burn-in timestamp interplay with `-copyts` is not established).
- **Not established:** behaviour for STRM/remote sources; multi-part items' subtitle handling across parts; Watch Together subtitle interplay; how `workAdmission.Admit` actually schedules analysis; whether the storage client caches directory listings; physical-device rendering on any platform.
- **Comparison side:** Jellyfin, intro-skipper, jellyfin-web and Plezy were read at the file and function level cited; androidtv, Swiftfin, Streamyfin, Wholphin, jellyfin-vue, jellyfin-android and jellyfin-desktop only by targeted search. Plex, Emby, Infuse and Bazarr statements are from general public knowledge, not code, and were not re-verified online.
- **Client coverage table:** "none found" means a search of the source tree found no caller; it does not prove absence on a device.

## Outside my domain

- `dbwork` gives every `subtitles.Service` call background-media priority (B-04.32). Other packages may have the same pattern of read paths taking the write gate; worth a sweep by the server/database auditor.
- A subtitle-import failure blocking unrelated deep analysis (B-04.3) is a symptom of `analysis_revision` being overloaded as "basic scan finished"; the scanning auditor may want to look at that coupling.
- The Backlog item "A replaced file is not re-read by an incremental scan" would interact badly with the strict size/mtime fencing here: subtitles, chapters and tiles would all report `source_changed` until a full rescan.
