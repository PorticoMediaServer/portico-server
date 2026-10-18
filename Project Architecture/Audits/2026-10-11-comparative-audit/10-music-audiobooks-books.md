# 10 — Music, audiobooks and books

Comparative audit, 11 October 2026. Read-only source review. Nothing here was run: every statement about behaviour is what the code says, not something observed on a device.

## Scope and sources

**Domain.** Music, audiobooks and books end to end: the library model, tag reading and metadata, how audio reaches a device (direct play, conversion, gapless, loudness), queues, playlists, lyrics, listening history, and the listening experience in the web and native clients. Portico's stated bar is Plex plus Plexamp.

**Portico files actually read (paths relative to `/home/justin/projects/Portico`).**

- Docs: `portico-internal/Project Architecture/03 Catalogue and Metadata.md`, `04 Playback and Media.md`, `Backlog.md`, `Specs/Spec — Page Content by Media Type.md`, `Specs/Spec — Playback Protocol v1.md` (§1, §7–§13, §18 in full).
- Server, read in full: `apps/portico-server/server/internal/playback/audio_plan.go`, `audio_plan_v2.go`; `internal/audiofacts/facts.go`, `store.go`, `facts_test.go`; `internal/playbackv1/audio.go`, `shuffle.go`; `internal/httpapi/audio_media.go`; `internal/subtitlevideo/audio_facts.go`, `audio_convert.go`; `internal/decoder/analysis.go` (audio recipes); `internal/localmetadata/audio.go`, `evidence.go`, `audio_evidence_ingestion.go`; `internal/assets/assets.go`, `audio_tags.go`; `internal/catalog/audio_repository.go`, `audio_evidence.go`, `play_history.go` (first half), `personal_activity.go` (progress acceptance); `internal/metadata/music_inputs.go`, `music_policy.go`, `music_scoring.go`, `musicbrainz.go`, `music_acquisition.go`, `music_details.go`; `internal/lyrics/document.go`, `lrclib.go`, `acquisition.go`; `internal/storage/lyrics.go` (sidecar and embedded lyric helper); `internal/playback/personal_progress.go`.
- Server, read in part: `internal/playbackv1/queues.go`, `queue_commands.go` (create, add, remove, move, update, advance), `internal/catalog/listening_selection.go` (ordered and mix), `internal/lyrics/service.go`, `bulk.go`, `internal/metadata/music_artist.go`, `internal/catalog/playlists.go`, `internal/operations/preferences.go` (music and listening keys), `internal/catalog/library_sources.go` (scan tiers), `internal/mediaexec` (priority and limits).
- Shared client logic: `apps/portico-server/packages/client-core/src/queue-controller.ts`, `queues.ts`, `audio-effects.ts`, `music-preferences.ts`, `playback-v1/audio-render.ts`, `listening.ts` (first 120 lines), `lyrics.ts` (acquisition flow only), `playback-v1/v1-queue-player.ts` (grep-level).
- Web: `apps/portico-server/web/src/bridge/DecodeAudioRender.ts` and `web/src/bridge/audio/` (`track.ts`, `mp3.ts`, `mp4.ts`, `decoder.ts`, `demux.ts`, `engine.ts`, `resampler.ts`, `capabilities.ts`, start of `stretch.ts`), `bridge/useSystemListening.ts`, `player/Lyrics.tsx` (grep-level).
- Native: `apps/portico-react-native/apps/native/src/bridge/audio-decode.ts`; `Native/PorticoAudioRender.m` (cache and decoder sections, lines 1–330); `android/.../native/PorticoAudioPlan.kt`; grep-level reads of `PorticoPlaybackController.kt`, `PorticoListeningOwner.kt`, `PorticoSystemMedia.m/.kt`, `PorticoListeningSession.kt`, `AndroidManifest.xml`, `build.gradle`.
- Fixtures: `apps/portico-server/fixtures/audio-gapless/` (listing and two sidecars), `fixtures/library-layout/Music` and `Audiobooks` (listing).

**Comparison code read.** Jellyfin: `MediaBrowser.Providers/MediaInfo/AudioFileProber.cs` (most of it), `Emby.Naming/Common/NamingOptions.cs` (audio extensions, album stacking prefixes), `Emby.Naming/Audio/AlbumParser.cs`, `Emby.Server.Implementations/ScheduledTasks/Tasks/AudioNormalizationTask.cs`, `MediaBrowser.Providers/Lyric/LrcLyricParser.cs`, `Emby.Server.Implementations/Library/MusicManager.cs`, `MediaBrowser.Model/Configuration/ServerConfiguration.cs` (resume settings); jellyfin-web `plugins/htmlAudioPlayer/plugin.js`, `scripts/settings/userSettings.js`. Finamp: `lib/models/finamp_models.dart` (defaults and enums), `lib/services/queue_service.dart` (structure). Navidrome: `conf/configuration.go` (all defaults), `consts/consts.go` (identity rules), `resources/mappings.yaml` (tag mapping and split rules). Audiobookshelf: `server/scanner/AudioFileScanner.js`, `server/models/MediaProgress.js`, `server/models/Library.js`, router and provider listings. Plezy: `lib/services/music/*` (headers and structure). FFmpeg source (public, fetched to check two facts): `libavformat/demux.c`, `libavformat/mp3dec.c`.

**Not reached, or only skimmed.** Most of `PorticoListeningOwner.m` and `.kt` (about 1,450 lines each: the native queue owner and background policy); `PorticoAudioRender.m` after line 330 (deck scheduling); `v1-queue-player.ts` in full; `playbackv1/transitions.go` (prepare-next/commit-next server side); `queue_build.go`; the web and native music screens and mini player (file names only); `internal/compactcatalog` book context (domain 30); `internal/downloads` for music; the Vega app; Jellyfin's `PlaylistsController`, `UniversalAudioController`, the Bookshelf plugin; Swiftfin and jellyfin-androidtv music; Navidrome's scanner internals and Subsonic API; Audiobookshelf's client and podcast code. Plex and Plexamp are closed: comparisons with them are from public knowledge of the products, not from code, and are marked as such.

## How Portico does it today

### Library model

A music library holds **artists**, **albums** and **songs**; an audiobook library holds **authors**, **books** and **book files** (parts), with chapters attached to each file. All are rows in `catalog_entities` with small side tables (`catalog_songs`, `catalog_albums`, `catalog_book_files`, …).

One function, `commitAudio` (`apps/portico-server/server/internal/catalog/audio_repository.go:221`), turns a scanned file into those rows:

- **Song title** is the `title` tag, else the file name.
- **Artist** is the `artist` tag, else "Unknown artist". An artist is found or created by its **name** (lower-cased, whitespace collapsed), per library (`audio_repository.go:112`, `:212`).
- **Album artist** is the `album_artist` tag, else the track artist, else "Various Artists" when a `compilation` tag is 1/true (`:326–331`).
- **Album** is identified by a key made of: the **folder**, the album title, the album artist, the **year**, and an "edition" (edition tag, MusicBrainz release id, barcode, catalogue number) (`:414`). A file with the same title/artist/year in the same folder joins an existing album when its edition evidence is compatible; albums are never merged afterwards ("Two albums are never merged by content", `:176`).
- A parent folder named `disc N`, `disk N` or `cd N` (exactly) is treated as a disc of the folder above (`:92`, `:254`).
- **Track and disc numbers** come from tags, else a leading number in the file name (`:93`, `:270`).
- **Book**: the `album` tag is the book title (else the folder name; a loose file in the library root is its own book). The key is folder, title, author, narrator, ISBN, ASIN, edition (`:505`). Author is the `author` tag, else `artist`; narrator is the `narrator` tag only (`:466–467`).

### Tag reading

Tags are read by **ffprobe's JSON output** (`internal/assets/assets.go:96`, format and first-audio-stream tags), not by a tag library. Only a fixed whitelist of about 55 keys is kept (`internal/assets/audio_tags.go:18–70`): title, album, artist, album artist, track/disc, date/year, genre, composer, author, narrator, compilation, MusicBrainz ids, ISRC, barcode, label, ReplayGain and R128 gains, series, ISBN/ASIN, description and a few more. Everything else is dropped. Sort-name tags, comments, grouping, work/movement, BPM, mood and lyrics are not in the list.

Sidecars add to this: `album.nfo` or `<file>.nfo`, `<file>.portico.json`, and for audiobooks `metadata.opf` / `<file>.opf` / `book.nfo` (`internal/localmetadata/audio_evidence_ingestion.go:27–31`). Every value is kept with its source ("evidence"), so the server can show where a field came from and re-decide later.

Only nine extensions are scanned in audio libraries: `.mp3 .m4a .m4b .flac .ogg .opus .wav .aac .aiff` (`assets.go:285–295`).

**Cover art**: `cover.jpg`, `folder.jpg`, `cover.png`, `folder.png` in the file's own folder win; otherwise the first embedded picture is extracted and scaled to at most 1024 px (`localmetadata/audio.go:55–78`). All local art lives in one folder, `<state>/local-artwork`, which stops accepting new images at 128 MiB in total (`:136`).

### Online metadata

Music uses **MusicBrainz**. Each album and each song has a durable job. With a MusicBrainz id in the tags the job looks that id up; otherwise it searches by title and artist and scores candidates (exact title, exact artist, duration within 2 s, ISRC, barcode, year, track count), publishing automatically only above 0.85 confidence with a clear margin and at least two strong signals; everything else waits for the owner's review (`internal/metadata/music_scoring.go`). AcoustID is consulted only when a file already carries an `acoustid_fingerprint` tag and the operator set `PORTICO_ACOUSTID_CLIENT_KEY`; the server does not compute fingerprints (`internal/metadataprovider/fingerprint.go:12`, `cmd/server/main.go:714`). Matched artists get a biography and portrait through MusicBrainz → Wikidata → Wikipedia → Wikimedia Commons (`internal/metadata/music_artist.go`).

A library setting, `local_mode` = prefer / supplement / off, decides whether local tags or the matched MusicBrainz title and artist are shown (`catalog/audio_evidence.go:275`).

Audiobooks have no online provider: tags and sidecars only.

### Audio delivery ("the audio plan")

For a song or audiobook file, the playback session carries an **audio render plan** (`internal/playback/audio_plan_v2.go:140`). The idea is Plexamp's: the device downloads the original file and decodes it itself; the server supplies what a decoder cannot know.

- **Facts.** One background pass per file runs ffprobe with every packet decoded (`-show_packets -show_frames`, "skip_manual" so no priming is hidden) and records: container, codec, sample rate, channels, bit depth, the exact number of decoded frames, how many frames to skip at the start (encoder delay) and end (padding), and the decoder configuration bytes (`internal/audiofacts/facts.go:53–145`). An `iTunSMPB` tag, when consistent, overrides the measured trim. A play of a file not yet measured tries once, within 5 seconds (`internal/playbackv1/audio.go:39–43`).
- **Mode.** `direct` when the device declared that (container, codec) pair within its sample-rate, channel and bit-depth limits and the file's bitrate is under any limit the viewer set. Otherwise `converted`: the server streams **FLAC** (16 or 24 bit, source rate capped to the device's) or **Ogg Opus** (48 kHz, default 128 kb/s stereo) made from exactly the trimmed frames. Otherwise `unavailable`, and the client plays the ordinary stream with no gapless, crossfade or normalisation.
- **Gain.** ReplayGain tags first (R128 tags converted to the same −18 LUFS reference), else the server's own EBU R128 loudness analysis (a deep-scan operation). Album gain comes from tags, or from analysis once every track of the album is measured. Peaks travel with the gains so the client can avoid clipping (`audio_plan_v2.go:303–349`).
- **Next track.** The queue has two extra commands. `:prepare-next` hands the client a private plan for the next entry, allowed to fetch only its first few megabytes; `:commit-next`, sent about two seconds before the join, atomically ends the old session and starts the new one (spec §18.2–18.3).

Each platform has its own engine:

| Platform | Decode | Gapless join |
|---|---|---|
| Web | WebCodecs `AudioDecoder` per codec, a JavaScript FLAC decoder as fallback, own demuxers for MP3/MP4/Ogg/FLAC/WAV (`web/src/bridge/audio/`) | Two "decks" scheduled on the Web Audio sample clock; trim applied from the plan (`track.ts`, `engine.ts`) |
| iPhone, iPad, Apple TV | Core Audio `ExtAudioFile` over a sparse on-disk byte cache (`Native/PorticoAudioRender.m`) | Next track appended to the same player node; a rule decides how much of the plan's trim Core Audio already applied (`:279–288`) |
| Android, Android TV, Fire TV | Media3 ExoPlayer (`PorticoAudioPlan.kt`) | Two plans played as one ExoPlayer playlist; ExoPlayer's own gapless handling, not the plan's trim |

Listening preferences are per profile: gapless (on), crossfade 0–12 s (off), normalisation off/track/album (off) (`internal/operations/preferences.go:194–196`). Consecutive tracks of one album are always joined, never crossfaded.

### Queues

The queue lives on the **server**, one per device (`internal/playbackv1/queues.go`, `queue_commands.go`). It is a list of **segments**, each a frozen snapshot of a selector (an album, artist, book, playlist, or up to 500 item ids). Clients read a window of at most 200 entries. Every change carries the queue's revision (`If-Match`), so two devices cannot silently overwrite each other.

**Shuffle is never materialised**: a seeded Feistel permutation (a small keyed cipher over positions) maps play order to source order in constant time, so a ten-million-song shuffle costs nothing to create (`shuffle.go`). Repeat is off/one/all. "Play next" and "Add to queue" add a segment; remove and move write a tombstone.

Queues can be saved as playlists. Playlists belong to a profile and can be shared with other profiles as viewer or editor (`internal/catalog/playlists.go`).

An "instant mix" selection exists on the server (`internal/catalog/listening_selection.go:226–290`) but no screen calls it.

### Lyrics

A lyric is a **versioned resource** attached to a song, with a scope (the whole library, or one profile), a language, an offset and a provenance record (`internal/lyrics/service.go`). Sources: a sidecar (`.lrc`, `.lyrics`, `.txt` beside the file), an embedded tag (`lyrics`, `unsyncedlyrics`, `lyrics-xx`), an upload, or LRCLIB (off until the owner enables it). Lyrics are **acquired on request**: someone runs a search for a song, gets candidates, and publishes one; the owner can also run a library-wide LRCLIB fetch from the console (`internal/lyrics/bulk.go`). Nothing is picked up automatically at scan time. Synced lyrics are line-level; word timestamps are removed (`document.go:177`).

### History and progress

Each timeline report passes through `AcceptPlaybackProgress` (`internal/catalog/personal_activity.go:27`). A play is "completed" when the client reports `ended` or the position passes the played threshold (default 95%, `preferences.go:178`). Completion adds one to the item's play count (server-wide and per profile) and writes the owner's play history and, unless paused, the viewer's own history. Songs never keep a resume point. For audiobooks, `book_resume` holds one row per profile and book: which file, and where in it.

Speed is one of 0.5, 0.75, 1, 1.25, 1.5, 1.75, 2, stored per profile separately for music and for books. Sleep timer choices: 15/30/45/60/90 minutes, end of chapter, end of this item, end of the book. A "passout" check (30–120 minutes without interaction) and an autoplay switch govern automatic advancing. The native apps enforce these while JavaScript is suspended.

### System integration

Web: the Media Session API (lock-screen and hardware keys). Apple: `MPRemoteCommandCenter` and Now Playing. Android: a Media3 `MediaSessionService` with a foreground service. There is no CarPlay, no Android Auto browse tree, no widget, no watch app. Chromecast goes through Portico's Cast receiver; AirPlay is the system picker.

## How the others do it

### Jellyfin (server and web)

- **Model.** A folder that contains audio is an album; an artist is a name. Multi-disc folders are recognised by a prefix list (`cd`, `disc`, `disk`, `vol`, `volume`, `part`, `act`, `digital media`) after stripping punctuation (`jellyfin/Emby.Naming/Common/NamingOptions.cs:182`, `Emby.Naming/Audio/AlbumParser.cs`). About 100 audio extensions are accepted (`NamingOptions.cs:212`).
- **Tags.** The ATL tag library reads tags, with ffprobe as fallback (`jellyfin/MediaBrowser.Providers/MediaInfo/AudioFileProber.cs:177`). Multi-value tags are split; a per-library option adds custom delimiters with a whitelist of names that must not be split (for example "AC/DC"); another option prefers the non-standard `ARTISTS` / `ALBUMARTISTS` tags. MusicBrainz ids, composer and ReplayGain track and album gain are read. Embedded lyrics are extracted at scan.
- **Audiobooks** (now a first-class `AudioBook` type on master): album artist is the author, the **composer tag is the narrator** ("Audiobookshelf and other tools use Composer for narrator", `AudioFileProber.cs:269`), description/comment is the overview, publisher becomes a studio. An audiobook is finished when less than 5 minutes remain, and resumable after 5 minutes (`MinAudiobookResume`, `MaxAudiobookResume`, `ServerConfiguration.cs:151–157`).
- **Loudness.** A scheduled task measures LUFS with FFmpeg's `ebur128` for libraries that opted in; album loudness is measured over the album's tracks concatenated (`AudioNormalizationTask.cs:113–120`). Only a gain is stored, no peak.
- **Delivery.** Direct play of the file, or a transcode chosen from the client's device profile (many codecs; HLS or progressive). No server-side gapless facts.
- **Lyrics.** `.lrc`/`.elrc` and `.txt` sidecars found at scan; embedded lyrics; word-level cues from enhanced LRC (`LrcLyricParser.cs`); remote providers through plugins with a scheduled download task.
- **Instant mix.** Up to 200 random songs sharing the seed's genres (`MusicManager.cs:71–95`).
- **Web player.** An HTML audio element with a gain node for normalisation (default mode `TrackGain`, `userSettings.js:223`); no gapless and no crossfade; the queue lives in the browser.

### Plezy

A client only. Music plays through mpv; the next track is "armed" with `setNext` so mpv joins gaplessly; the queue is client-side with a materialised shuffle order that survives un-shuffling; the last session (up to 1,000 tracks) is persisted per profile and restored (`plezy/lib/services/music/music_playback_service_impl.dart:60–81`, `music_queue_controller.dart`, `music_session_store.dart:38`). It has hardware media keys, a companion remote, lyrics, and pauses for vehicle driving restrictions.

### Finamp (Jellyfin music client)

Offline-first: downloads with optional transcoding, automatic offline mode, re-sync on start. Gapless through `just_audio` (ExoPlayer on Android, AVQueuePlayer on Apple). Normalisation is on by default in **hybrid** mode (album gain when playing an album, track gain otherwise), with Android's loudness-enhancer effect so quiet tracks can be boosted and a separate "iOS base gain" setting (default value 6 dB; I did not confirm its sign) because iOS volume cannot amplify (`finamp/lib/models/finamp_models.dart:125–129`, `:318`; `lib/services/music_player_background_task.dart:315`, `:435`). The queue has a separate **Next Up** lane kept in place under shuffle by a custom shuffle order (`queue_service.dart:1597`), previous tracks are kept, queues are saved and restored. **Radio** modes extend the queue automatically: similar, continuous, album mix, reshuffle, random (`finamp_models.dart:3791`). Also: Android Auto and CarPlay helpers, sleep timer, speed and pitch, lyrics, fade in/out settings, Discord presence, playback history that survives offline.

### Navidrome (Go + SQLite music server)

Tags are read with TagLib through a declarative mapping file listing every alias of every field (`navidrome/resources/mappings.yaml`). Artists are split on `" / "`, `" feat. "`, `" ft. "`, `"; "` unless the multi-valued `ARTISTS` tag exists; genres on `; / ,`; all configurable, with split exceptions. Album identity is a configurable rule, by default "MusicBrainz album id, else album artist + album + version + release date" (`consts/consts.go:157`): the folder is not part of it. It models **participants** (composer, conductor, lyricist, performer with instrument…), sort tags and ignored articles, disc subtitles and disc art, cover/artist/lyrics source priority lists (`conf/configuration.go`: `coverartpriority`, `lyricspriority` = `.ttml,.yaml,.yml,.elrc,.lrc,.srt,.txt,embedded`). It has smart playlists (`.nsp` rule files), M3U import/export with auto-import, public playlists, shares, Last.fm/ListenBrainz scrobbling and a local scrobble history, external "agents" (Deezer, Last.fm, ListenBrainz) for biographies, images and similar artists, a transcoding cache, a jukebox mode, the Subsonic/OpenSubsonic API, and (new) a Jellyfin-compatible API. ReplayGain comes from tags only and is applied by clients.

### Audiobookshelf

A book is a **folder**. Metadata comes from several sources in a per-library precedence order: folder structure (Author/Series/Title), audio tags, NFO, text files, OPF, its own metadata file (`audiobookshelf/server/models/Library.js:84`). In tags, composer is the narrator, description/comment the description, `series` or `grouping` ("Series Name #1") the series (`server/scanner/AudioFileScanner.js:219–270`). Chapters come from OverDrive markers, else embedded chapters, else one chapter per file (`:496–553`); they can be edited and looked up from Audible data. Providers: Audible, Audnexus, Google Books, Open Library, iTunes, FantLab, custom. Progress is one book-relative time; a book is finished when under 10 seconds remain, or at a percentage the library sets (`server/models/MediaProgress.js:222–235`). Bookmarks, per-user playback sessions and listening statistics, a year in review, podcasts with RSS download, an ebook and comic reader, send-to-device, RSS feed publishing, collections and playlists, tools to merge files into an M4B and embed metadata. The mobile apps add Android Auto/CarPlay, sleep timer with end-of-chapter and shake-to-reset, auto-rewind.

### Plex and Plexamp (public knowledge only)

Tag-driven albums with album-artist grouping; sonic analysis producing "sonically similar" artists, albums and tracks and powering radios and mixes; loudness levelling from server analysis; "sweet fades" (crossfades timed from the analysis), gapless, lyrics (synced, from a partner), downloads, CarPlay/Android Auto, a server-side play queue shared between players, guest DJ modes.

## Architectural differences

### Library model and tagging

### A-10.1 Tags are read by ffprobe and a whitelist, not by a tag library
- **Portico:** One ffprobe JSON call per file; format tags plus the first audio stream's tags; about 55 whitelisted keys kept, at most 128 embedded values, each 4 KB (`internal/assets/assets.go:96–210`, `audio_tags.go:18–118`).
- **Jellyfin / Navidrome / Audiobookshelf:** Jellyfin uses the ATL library with ffprobe as fallback; Navidrome uses TagLib with a mapping file of aliases per container (ID3 frame ids, MP4 atoms, WM/ names, RIFF ids); Audiobookshelf uses ffprobe like Portico.
- **Why it matters:** ffprobe flattens what each container stores. It joins duplicate Vorbis comments with ";", gives one string for an ID3v2.4 multi-value frame, does not read APEv2 tags on MP3 files at all (checked in FFmpeg's `mp3dec.c`), and exposes ID3 `TXXX` frames only by their free-text description. A tag library sees the real structure: several values, frame languages, picture types, sort frames. Portico's one process per file also reuses the sandboxed probe it already trusts, which is a real security and simplicity benefit.
- **Assessment:** **Trade-off (subjective), leaning Portico behind for music.** It looks deliberate (one confined tool for every media read; `04 Playback and Media.md` "Decoder confinement"). The cost shows up as A-10.5, A-10.6, B-10.14 and B-10.18. Adopting a pure-Go tag reader for audio (no subprocess, bounded input) alongside ffprobe would be medium work touching `assets` and `localmetadata`; it would also remove one ffprobe launch per file from scans.

### A-10.2 Nine audio extensions are scanned
- **Portico:** `.mp3 .m4a .m4b .flac .ogg .opus .wav .aac .aiff` (`assets.go:285–295`). The ffprobe format whitelist beside it already allows APE, WavPack, TTA, ASF/WMA, AMR and Matroska (`assets.go:89`), but files with those extensions never reach it.
- **Jellyfin / others:** Jellyfin lists about 100, including `.aif`, `.oga`, `.mka`, `.wma`, `.ape`, `.wv`, `.dsf`, `.mpc`, `.mp4`, `.m4b`, `.aa/.aax`, `.cue` (`NamingOptions.cs:212`). Navidrome goes by MIME type. Audiobookshelf accepts `.mp4`, `.m4b`, `.mka`, `.wma`, `.awb`, `.caf` and more.
- **Why it matters:** A file with an unlisted extension is simply absent from the library. `.aif` (the common short form of AIFF), `.oga`, `.wma`, `.ape`, `.wv`, `.dsf` and `.mp4` audiobooks are all found in real collections.
- **Assessment:** **Portico behind.** Small to add the extensions whose codecs FFmpeg already decodes (they would play through the FLAC conversion path); DSD needs a decision about conversion rate. See B-10.29.

### A-10.3 Album identity includes the folder and the year, and albums are never merged
- **Portico:** Key = folder + title + album artist + year + edition (`audio_repository.go:414`). MusicBrainz release ids take part only as "edition" evidence.
- **Jellyfin:** The folder is the album; tags name it. **Navidrome:** MusicBrainz album id if present, else album artist + album + version + release date; the folder is ignored; configurable. **Plex (public):** album artist + album title, with splitting and merging tools in the UI.
- **Why it matters:** Portico's rule protects against the classic wrong merge (two different "Greatest Hits" by one artist in one year in different folders stay apart). It causes the opposite problem in three common layouts: a multi-disc album whose disc folders are not named exactly `Disc N` becomes two albums; an album whose tracks carry different years becomes one album per year; the same album stored in two places is two albums. None of these can be fixed in Portico today because there is no merge.
- **Assessment:** **Trade-off (subjective).** Deliberate (comments at `audio_repository.go:176`, `:377`). I would keep the folder as a tie-breaker but let an identical MusicBrainz release id, or identical album artist + title + total-discs evidence under one parent folder, join discs; and take the year from the album's most common value rather than from each track. Medium; touches `commitAudio` and needs a merge path. See B-10.17.

### A-10.4 Multi-disc folders are recognised by one strict pattern
- **Portico:** `^(disc|disk|cd)[ ._-]*([0-9]{1,3})$` on the folder name (`audio_repository.go:92`).
- **Jellyfin:** prefixes `cd, digital media, disc, disk, vol, volume, part, act`, after replacing `- . ( )` and spaces, so "Disc 1 - Live" and "CD01" both count.
- **Why it matters:** "Disc 1 - The Early Years", "CD1 (Bonus)", "Volume 2" are everyday names. In Portico they change the album's folder and so split the album (A-10.3).
- **Assessment:** **Portico behind.** Small: widen the pattern to prefix + number + optional suffix.

### A-10.5 An artist is a name; there are no sort names and no MusicBrainz artist identity at scan
- **Portico:** Artist key = lower-cased, whitespace-collapsed name per library (`audio_repository.go:112`, `:212`). `artistsort`/`albumartistsort`/`titlesort` tags are not in the whitelist. `musicbrainz_artistid` is kept as evidence but does not identify the artist.
- **Jellyfin:** also name-based, with MusicBrainz artist ids stored as provider ids. **Navidrome:** artist id from MusicBrainz artist id when present; sort tags honoured (`prefersorttags`), a configurable ignored-articles list ("The El La Los Las Le Les Os As O A").
- **Why it matters:** "Beyoncé" and "Beyonce", or "Sigur Rós" in two spellings, are two artists. Japanese, Korean or classical names tagged with a Latin sort name cannot sort where the owner expects. Two different artists who share a name are one artist.
- **Assessment:** **Portico behind.** Reading sort tags is small. Using the MusicBrainz artist id as identity when present is medium (it changes entity keys and needs a migration rule).

### A-10.6 Multi-artist tracks are not split
- **Portico:** The credited artists of a song come from an `artists` value only when it is a JSON list, which only a `.portico.json` or NFO sidecar produces; an embedded `ARTISTS` tag arrives from ffprobe as plain text and is ignored, and the single `artist` string is used whole (`catalog/audio_evidence.go:138–153`, `audio_repository.go:435`). "A feat. B", "A; B" and "A / B" each become one new artist.
- **Jellyfin:** splits multi-value tags, optional custom delimiters with a do-not-split whitelist, optional preference for `ARTISTS`. **Navidrome:** splits on `" / "`, `" feat. "`, `" ft. "`, `"; "` by default, prefers `ARTISTS` and the id lists, with exceptions configurable. **Plex (public):** does not split track artists.
- **Why it matters:** The artist list fills with one-off "X feat. Y" entries and an artist's page misses their guest appearances. After a MusicBrainz match the shown artist string is replaced by MusicBrainz's credit (`audio_evidence.go:296–308`), but the artist entities were already created from the tag.
- **Assessment:** **Portico behind** Jellyfin and Navidrome, level with Plex. Medium: honour embedded multi-value `ARTISTS` first (unambiguous), then decide whether to split free text. Splitting free text is where "AC/DC" and "Earth, Wind & Fire" go wrong, so a do-not-split list is part of the work.

### A-10.7 Album artist fallback and compilations
- **Portico:** No album artist tag → the track artist is the album artist; `compilation=1` → "Various Artists". There is no detection of a compilation from the folder's contents.
- **Jellyfin:** album artist falls back to the track artists, but the album is the folder, so it does not split. **Navidrome:** same fallback as Portico, with a compilation flag making it "Various Artists".
- **Why it matters:** A folder of 15 tracks by 15 artists with no album-artist tag and no compilation flag becomes 15 one-track albums in Portico. Plex behaves the same way and this is one of its most-reported music complaints.
- **Assessment:** **Trade-off (subjective).** A cheap, safe improvement: when the same folder and album title would produce several albums differing only in album artist, and none has an album-artist tag, group them as Various Artists. Small to medium.

### A-10.8 Genres split on fixed characters
- **Portico:** `,` `;` `/`, at most 16 genres of 64 characters (`audio_repository.go:618–637`).
- **Navidrome:** the same three by default, configurable. **Jellyfin:** multi-values plus optional custom delimiters and a whitelist.
- **Why it matters:** "Singer/Songwriter" becomes two genres; "R&B/Soul" probably should. Owners with odd genres have no way to adjust.
- **Assessment:** **Trade-off**, minor. Matches Navidrome's default. A per-library "don't split these" list would be small.

### A-10.9 Evidence with provenance, and a three-way local/online policy
- **Portico:** Every tag value is stored with its source (embedded, NFO, OPF, Portico JSON) in `audio_local_evidence`, the chosen value in `audio_tag_evidence`, and a per-library `local_mode` (prefer/supplement/off) decides what is shown; MusicBrainz ids always count (`catalog/audio_evidence.go:18–120`, `:275–312`).
- **Others:** Jellyfin has per-field locks and "replace all metadata"; Audiobookshelf has an ordered precedence list of sources; Navidrome is tags-only.
- **Why it matters:** Portico can explain a value and change its mind without rescanning.
- **Assessment:** **Portico ahead** in principle. Audiobookshelf's ordered, owner-editable precedence is easier for an owner to understand than three modes; worth a look when the audiobook agent is revisited.

### A-10.10 Sidecar formats
- **Portico:** Kodi-style `album.nfo` / per-file `.nfo`, its own `.portico.json`, OPF for books; 64 KB limit; strict XML budget (`localmetadata/evidence.go:21`).
- **Jellyfin:** NFO for albums and artists. **Audiobookshelf:** OPF, NFO, `desc.txt`, `reader.txt`, its own `metadata.json`. **Navidrome:** none (tags), plus `.nsp` playlists.
- **Assessment:** **Roughly level.** Missing for books: `desc.txt` / `reader.txt` and Audiobookshelf's `metadata.json`, which matter to anyone migrating from it (small).

### A-10.11 Cover art discovery and storage
- **Portico:** Four exact names in the file's own folder, 4 MiB each, at most 16 megapixels, JPEG or PNG; the sidecar wins over embedded art for every file in that folder; embedded art is the first picture stream; one store capped at 128 MiB (`localmetadata/audio.go:55–141`).
- **Navidrome:** priority list `cover.*, folder.*, front.*, embedded, external`, plus disc art (`disc*.*, cd*.*`) and artist art (`artist.*`), any image type, a size-bounded resize cache separate from the originals. **Jellyfin:** many names (`folder`, `cover`, `front`, `poster`, `default`, `albumart`…), looks in the album folder for disc subfolders, picks the embedded picture marked as front cover.
- **Why it matters:** `front.jpg`, `Folder.jpg` (Windows Media Player's capitalisation, on a case-sensitive Linux disk), `cover.webp`, a cover in the album folder above `CD1/`, and scans larger than 4 MiB are all missed; a folder of loose singles shares one `cover.jpg`. The 128 MiB cap is a hard functional limit, see B-10.2.
- **Assessment:** **Portico behind.** Small for names and case; the store limit needs a design decision.

### A-10.12 Online matching is evidence-scored and conservative
- **Portico:** Durable per-album and per-song MusicBrainz jobs, scored evidence, automatic only when confident, otherwise an owner review queue; a 30-day evidence cache; one request per second; the library can be local-only (`metadata/musicbrainz.go`, `music_scoring.go`).
- **Jellyfin:** MusicBrainz and TheAudioDB plugins match by name with little scoring. **Navidrome:** no matching; it trusts ids already in the tags and asks agents for extras. **Plex (public):** its own music agent with fingerprinting.
- **Why it matters:** Portico will not silently attach the wrong release. The price is throughput: with no ids in the tags, every album and many songs need a search at one request a second, so a 100,000-song library takes on the order of a day or more and a multi-million-song library weeks; and many albums end in "needs review".
- **Assessment:** **Portico ahead on correctness, behind on coverage and speed.** Deliberate. Worth deciding how the owner experiences a large first import (see Open questions).

### A-10.13 Artist biographies and images come from Wikidata, Wikipedia and Commons only
- **Portico:** `metadata/music_artist.go`; no key, no extra provider; needs a MusicBrainz-matched artist with a Wikidata link. Local `artist.jpg` first.
- **Jellyfin:** TheAudioDB and fanart plugins. **Navidrome:** Deezer, Last.fm, ListenBrainz agents by default, with local `artist.*` first.
- **Why it matters:** Wikipedia has portraits for well-known artists and nothing for most others; Commons images are often poor crops. Deezer-style artist images cover far more of a typical library.
- **Assessment:** **Trade-off.** Deliberate ("No Fanart.tv (no extra sources or keys, per Justin)", Page Content spec §8). Expect many initial-letter circles.

### A-10.14 No roles beyond artist, and nothing for classical music
- **Portico:** `composer` is whitelisted and stored as evidence but there is no composer entity, no conductor/performer/work/movement.
- **Navidrome:** "participants" by role, including performer with instrument. **Jellyfin:** composer as a person. **Plex (public):** limited.
- **Assessment:** **Portico behind** Navidrome; level with most. Large if done properly; a composer filter and a "Composer" line on the song would be small.

### Audio delivery

### A-10.15 Gapless by server-measured facts and on-device decoding
- **Portico:** The server measures each file's exact frame count and trim; clients decode the original and output exactly those frames (spec §18.1).
- **Jellyfin web:** no gapless. **Finamp / Plezy:** hand the next file to a native player (ExoPlayer, AVQueuePlayer, mpv) that reads LAME/iTunSMPB/edit lists itself. **Navidrome:** leaves it to Subsonic clients. **Plexamp (public):** its own decoder; server analysis for loudness and fades.
- **Why it matters:** Portico is the only one here that can be sample-exact in a **browser**, and the only one whose answer is the same on every platform by construction. The cost is a full decode of every file in the library once (A-10.17), three custom engines to keep correct (A-10.16), and a dependence on the client's decoder producing exactly the frames FFmpeg's does. Failure cases: a file not yet measured (plays without gapless), a decoder that drops or adds priming differently from the table (`web/src/bridge/audio/track.ts:28–33`), HE-AAC (B-10.10), edit-list-only AAC (B-10.23), end padding longer than a frame (B-10.9), different sample rates (A-10.23).
- **Assessment:** **Portico ahead in ambition and, where the fixtures cover it, in result.** Deliberate (spec §18.1 "Plex and Plexamp are the standard to match").

### A-10.16 Three engines with three different sources of truth for trim
- **Portico:** Web applies the plan's trim to raw decoder output with a per-browser table of what the decoder already dropped. Apple compares Core Audio's own frame count to the plan's and skips "plan start minus packet-table priming" otherwise (`PorticoAudioRender.m:279–288`). Android ignores the plan's trim and trusts ExoPlayer's gapless parsing (`PorticoAudioPlan.kt:9–11`).
- **Others:** one engine per client, the platform's.
- **Why it matters:** A file can be exact on one platform and off on another, and the conformance fixtures are the only guard. The fixtures cover LAME MP3, iTunSMPB AAC, Opus, FLAC, ALAC, WAV (spec §18.8) but not: AAC with only an edit list, HE-AAC, MP3 without a LAME header, MP3 with end padding over one frame, Vorbis, mixed sample rates.
- **Assessment:** **Trade-off.** Unavoidable given the goal; the gap is fixture coverage (B-10.28).

### A-10.17 Every audio file is fully decoded once to measure it
- **Portico:** `audiofacts.Args` asks ffprobe for every packet and every decoded frame (`facts.go:53–59`); a background backfill walks all listening assets one at a time (`store.go:171–246`). The spec describes a packet-only read with decoding as the fallback (§18.7); the code always decodes.
- **Others:** nothing comparable; Jellyfin's optional LUFS scan is a similar cost.
- **Why it matters:** Decoding is typically several hundred times faster than real time per core, single-threaded here. A 100,000-track library (about 7,000 hours) is on the order of a day of one core; a 20-hour audiobook file is minutes, far past the 5-second play-time budget (B-10.12). Loudness analysis is a second full decode of the same files.
- **Assessment:** **Trade-off.** For FLAC, WAV, ALAC and Opus the exact answer is in the headers and a decode adds nothing; for MP3 and AAC a packet count plus the header evidence is exact in almost every case. Doing what the spec says (packets first, decode only on inconsistency) and sharing one decode between facts and loudness would cut the cost several-fold. Medium.

### A-10.18 Conversion targets are FLAC and Opus only, made fresh for every request
- **Portico:** Lossless FLAC or 48 kHz Opus, never PCM, never AAC or MP3 (`audio_plan_v2.go:189–228`); each request runs FFmpeg from the start of the file; a seek is a new request with `?fromFrame=N` that decodes from the beginning and discards up to N (`httpapi/audio_media.go:147–185`, `decoder/analysis.go:62`); nothing is cached.
- **Jellyfin:** transcodes to whatever the device profile lists (AAC, MP3, Opus, FLAC…) over HLS or progressive, seeking with `-ss`. **Navidrome:** named transcoding profiles (MP3, Opus, AAC), a transcoding cache (100 MB default), per-player bitrate caps. **Finamp:** asks Jellyfin for AAC in fragmented MP4 by default.
- **Why it matters:** FLAC-for-everything is simple and exact, and it never loses quality. It also multiplies bandwidth when a lossy file must be converted (a 128 kb/s Vorbis file becomes roughly 700–900 kb/s of FLAC), and on Apple devices a viewer's bitrate cap cannot be honoured at all because those devices do not declare Opus (B-10.11). Seeking deep into a converted audiobook re-decodes hours of audio first.
- **Assessment:** **Trade-off (subjective).** Deliberate (spec §18.1). I would add AAC-in-MP4 as a bitrate-limited target for Apple (exact trim is possible with an edit list or iTunSMPB the server writes itself), and use input seeking for long conversions. Medium.

### A-10.19 The audio capability model has no codec profile
- **Portico:** A device declares (codec, containers, max sample rate or exact rates, max channels, max bit depth) (`audio_plan_v2.go:19–26`). The measured facts carry no profile either (`facts.go:25–36`).
- **Jellyfin:** device profiles carry codec profile conditions (for example excluding HE-AAC, limiting channels per codec).
- **Why it matters:** "AAC" covers AAC-LC, HE-AAC (v1, v2) and xHE-AAC, which need different decoder configurations and produce a different number of frames per packet. See B-10.10.
- **Assessment:** **Portico behind.** Small to add `profile` to facts and capabilities; then test what each engine really does.

### A-10.20 Loudness: tags, then server analysis, with peaks; off by default
- **Portico:** Track and album gain against −18 LUFS, R128 tags converted, peaks included, client limits gain so peak ≤ full scale (`audio_plan_v2.go:303–349`; `client-core/src/playback-v1/audio-render.ts:109–118`). Album gain from analysis is an energy average of track loudness weighted by duration. Modes off / track / album; default **off**; no pre-amp setting in preferences (the function accepts one).
- **Jellyfin:** tag gain or measured LUFS; gain only, no peak; web default `TrackGain`. **Finamp:** on by default, hybrid mode. **Navidrome:** passes tags to clients. **Plexamp (public):** loudness levelling on analysed libraries.
- **Why it matters:** Portico's is the most complete model here (peak-aware, tags and analysis, R128 handled). Being off by default, and analysis needing the Complete scan tier, means most installs will not hear it. With "album" chosen, a queue of unrelated songs is still album-normalised; Finamp's hybrid mode avoids that by looking at what is playing.
- **Assessment:** **Portico ahead in mechanism, behind in defaults and modes.** Small: an "auto" mode (album gain when the neighbours are the same album, which the queue already knows via `albumId`), and consider on by default.

### A-10.21 Normalisation can only turn tracks down on Android
- **Portico:** Android gain is clamped to at most 1.0 because it is applied as player volume (`PorticoAudioPlan.kt:77–88`); web and Apple can boost up to the peak limit.
- **Finamp:** uses Android's loudness-enhancer effect for positive gain, and has a separate base-gain setting for iOS, where volume cannot amplify.
- **Why it matters:** Quiet tracks (classical, older masters) stay quiet on Android while they are raised elsewhere.
- **Assessment:** **Portico behind** on Android. Small: either a base attenuation like Finamp's, or an audio processor that can amplify.

### A-10.22 Crossfade
- **Portico:** 0–12 s equal-power crossfade, shortened to half the shorter track, skipped between consecutive tracks of an album (`web/src/bridge/audio/engine.ts:195–215`; Android `PorticoPlaybackController.kt:671–719`).
- **jellyfin-web:** none. **Finamp:** fade in/out durations, not a true overlap as far as the settings show. **Plexamp (public):** "sweet fades" timed from analysed track endings.
- **Assessment:** **Portico ahead** of the open-source field; behind Plexamp's content-aware fades, which need the loudness-over-time data Portico already has the tooling to produce (large).

### A-10.23 Sample-rate changes between tracks
- **Portico (web):** The `AudioContext` is created at the first track's rate and kept (`DecodeAudioRender.ts:64`, `:283–286`). A later track at another rate goes through a per-track windowed-sinc resampler that starts and ends with silence as its history (`resampler.ts:54`, `:108–112`). **Apple:** "a format change uses a second node scheduled on the output sample clock" (`PorticoAudioRender.m:25`). **Android:** ExoPlayer reconfigures its audio track.
- **Others:** native players reopen or resample in the OS mixer; none promise sample-exact joins across a rate change.
- **Why it matters:** An album is normally one rate, so joins stay exact. But on the web, if the session started on a 48 kHz track and then plays a 44.1 kHz album, every join of that album is two independently resampled edges (B-10.20).
- **Assessment:** **Trade-off.** Recreating the context when the queue moves to a different rate and nothing is joined to the old one would restore exactness; small to medium.

### A-10.24 A protocol for the next track (prepare, then commit)
- **Portico:** Private next-track plans with a byte budget, a 60-second token, an atomic commit that moves progress, history and Now Playing at the audio boundary (spec §18.2–18.5).
- **Others:** clients simply open the next URL early; the server learns about it from a later progress report.
- **Why it matters:** Portico's play counts, Now Playing and resume points change at the moment the audio does, and a prepared-but-never-played track leaves no trace. It is also a lot of machinery: many cancel conditions, and a fallback path to ordinary advance.
- **Assessment:** **Portico ahead** on accounting accuracy; a complexity risk worth runtime testing (see Evidence boundary).

### A-10.25 Seeking inside large files
- **Portico (web):** MP4 seeks through the sample table held in memory; MP3 and FLAC walk packet headers from the start, downloading everything before the target (`track.ts:149–169`; Backlog "Web audio"). Converted streams restart FFmpeg from the file start.
- **Others:** native players use Xing/VBRI tables, FLAC seek tables or bisection; browsers' own `<audio>` does the same.
- **Assessment:** **Portico behind** for MP3/FLAC on the web, and it is already in the Backlog. It matters most for multi-hour MP3 audiobooks.

### A-10.26 Hi-res, bit-perfect, exclusive output, DSD
- **Portico:** Declared limits per device (Apple and Android: 96 kHz/24-bit/2 channels; web: up to 384 kHz for FLAC and PCM, `capabilities.ts:36–39`); above them the server converts down. Output always goes through the platform mixer. DSD files are not scanned.
- **Plezy:** mpv, which can use exclusive output. **Plexamp (public):** exclusive mode and sample-rate matching on desktop. **Finamp / jellyfin-web:** mixer only.
- **Assessment:** **Level with most; behind Plexamp** for the audiophile case. The spec's "bit-perfect when the route allows" (§12) is not something the current engines attempt. Large and platform-specific; probably a desktop-app topic.

### A-10.27 Speed
- **Portico:** Seven fixed steps from 0.5 to 2.0 (`preferences.go:120`), one setting for music and one for books per profile, pitch kept (WSOLA time-stretch on web `stretch.ts`, `AVAudioUnitTimePitch` on Apple, ExoPlayer on Android).
- **Audiobookshelf:** finer steps and higher top speeds; some apps remember speed per book. **Finamp:** speed and pitch separately.
- **Assessment:** **Level on mechanism, behind on range.** The spec promised 0.5–3.0 (§12). Small.

### Queues and playlists

### A-10.28 The queue is a server resource
- **Portico:** Server-owned, revisioned, windowed, built from selectors, one per device (spec §8).
- **Jellyfin web, Finamp, Plezy:** the queue is a list in the client; Finamp can optionally report it to the server. **Plex (public):** server-side play queues, the closest relative.
- **Why it matters:** A Portico queue costs the same for 10 songs or 10 million, survives a reload or a crash, can be driven from another device, and is what makes atomic gapless accounting possible. It also means every edit is a round trip, and offline playback needs a separate local story.
- **Assessment:** **Portico ahead** for scale and multi-device; deliberate (thin clients). The consequences for shuffle are the next entry.

### A-10.29 Shuffle is a formula, so edits behave differently
- **Portico:** Play order is computed from (seed, lap, size, first). Adding while shuffled always appends after the shuffled body, even for "Play next" (`queue_commands.go:744–748`); removing or moving any entry recomputes the shuffle over the whole queue with the current entry first (`:838`, `:844`, `:939`).
- **Finamp, Plezy, jellyfin-web:** keep the shuffled order as a list and insert into it; Finamp keeps "Next Up" directly after the current track under shuffle.
- **Why it matters:** See B-10.3 and B-10.4: "Play next" does not play next, and one edit brings already-played tracks back.
- **Assessment:** **Portico behind in behaviour**, for a scale property that only matters above a few hundred thousand entries. A hybrid would keep both: keep the formula for the untouched body, and keep a small explicit "up next" segment that sits between the current entry and the shuffled remainder, plus a "played so far" boundary that edits do not reset. Medium, inside `playbackv1`.

### A-10.30 One queue, no separate "Next Up" lane, no previous-queue history
- **Portico:** "Play replaces this device's current queue" (`queue_commands.go:309`).
- **Finamp:** previous tracks / current / Next Up / queue as four parts; saved queues can be restored; **Plezy:** restores the last session.
- **Assessment:** **Trade-off.** Portico's durable server queue covers "restore after restart" better than either; it has no way back to the queue you just replaced by accident (small to add: keep the last N queues per device for a while).

### A-10.31 Advancing policy lives on the server and in native code
- **Portico:** Autoplay switch, post-play countdown, "passout" check after N minutes without interaction, sleep timers enforced natively while JavaScript sleeps (`listening.ts:11–21`; `QueueView.postPlay`).
- **Others:** client-side sleep timers; no passout check for music.
- **Assessment:** **Portico ahead**; unusual and sensible for audiobooks. For music a passout check that stops an album may surprise people; worth confirming the default.

### A-10.32 Instant mix exists on the server but is not offered
- **Portico:** Up to 640 candidates (the seed, its album, its artists' songs, and the 192 lowest-id songs of the library), scored 8/4/2/1 and sorted by score then a seeded hash (`listening_selection.go:226–290`); `client-core` has `journey(…, 'mix')`; no screen calls it; the path it uses is capped at 1,000 entries (`listening.ts:49`).
- **Jellyfin:** 200 random songs sharing genres. **Finamp:** five radio modes that keep extending the queue. **Navidrome:** similar songs from external agents. **Plexamp (public):** sonic similarity.
- **Assessment:** **Portico behind** (Backlog: "music radio and mixes (a listening-similarity source is still to be chosen)"). See B-10.25 for why the current mix would feel repetitive.

### A-10.33 Playlists
- **Portico:** Owned by a profile; shared with named profiles as viewer or editor; idempotent mutations with receipts; rank-key ordering; any playable item; no size cap (`catalog/playlists.go`).
- **Jellyfin:** per-user, optional public, per-user edit rights, stored as files. **Navidrome:** owner + public flag, M3U import/export and folder auto-import, smart playlists from rule files. **Audiobookshelf:** private playlists and shared collections.
- **Assessment:** **Portico ahead on collaboration and scale, behind on smart playlists and M3U** (F-10.7, F-10.8).

### Lyrics

### A-10.34 Lyrics are curated resources, not discovered files
- **Portico:** Explicit search, candidates, publish; scopes, revisions, provenance, rights text; per-viewer selection (`lyrics/service.go`, `acquisition.go`).
- **Jellyfin / Navidrome:** a sidecar or embedded lyric is simply there when the song plays; remote lyrics by scheduled task or plugin.
- **Why it matters:** An owner with `.lrc` files beside every track sees "No lyrics available for this track" until someone runs a local search and publishes, per song (`web/src/player/Lyrics.tsx:20`; there is no bulk run for local sources, only for LRCLIB, `bulk.go:92–99`).
- **Assessment:** **Portico behind in the default experience**, ahead in control. Small: on first view of a song with no library-scope lyric, run the local search and auto-publish the best local candidate (sidecar over embedded, synced over plain). See F-10.1.

### A-10.35 Strict LRC parsing, line-level only
- **Portico:** Rejects the whole document on an untimed line, an unknown `[xx:…]` header, seconds ≥ 60 or a `[mm:ss:xx]` stamp; strips word timestamps (`document.go:50–51`, `:144–177`); plain text and LRC only.
- **Jellyfin:** tolerant parser library, falls back to plain text, keeps word-level cues. **Navidrome:** TTML, its own YAML format, enhanced LRC, LRC, SRT, plain.
- **Assessment:** **Portico behind.** See B-10.8 and F-10.2.

### A-10.36 Lyrics are pinned to one exact version of the file
- **Portico:** A lyric's key includes a digest of the file's size, modified time and duration (`service.go:155`, `:277`).
- **Others:** keyed by item.
- **Why it matters:** Retagging a file hides its lyrics (B-10.7). The pin protects synced timing against a replaced recording, which is a real concern, but a tag edit does not change timing.
- **Assessment:** **Trade-off**, currently with a bug attached. Pin synced lyrics to the audio's frame count instead (already measured), which survives retagging.

### History and statistics

### A-10.37 "Played" is one percentage for everything that is not video
- **Portico:** 95% by default, 75–100% (`preferences.go:178`); the same rule completes a song and an audiobook file (`personal_activity.go:44–69`).
- **Jellyfin:** music is played at the end; audiobooks finish with under 5 minutes left. **Audiobookshelf:** under 10 seconds left, or a library percentage. **Last.fm rule (used by Navidrome and scrobblers):** half the track or four minutes.
- **Why it matters:** For songs the difference is taste. For audiobooks it loses the listener's place (B-10.1).
- **Assessment:** **Portico behind** for audiobooks.

### A-10.38 Two histories and maintained play counts; no scrobbling, no statistics
- **Portico:** Owner's play history (who, what, device, converted or not) and the viewer's own history that they can pause; `play_counts` and `personal_play_counts` (`play_history.go`).
- **Navidrome:** scrobbles to Last.fm/ListenBrainz and keeps its own scrobble history. **Audiobookshelf:** listening sessions, per-day statistics, year in review. **Finamp:** offline play history replayed to the server.
- **Assessment:** **Portico ahead** on privacy design; **behind** on scrobbling and on showing people their listening (F-10.3, F-10.4).

### Audiobooks and books

### A-10.39 A book is identified by tags, with the folder as a tie-breaker
- **Portico:** `album` tag = book; folder + title + author + narrator + ids form the key (`audio_repository.go:505`).
- **Audiobookshelf:** the folder is the book, always; tags fill in. **Jellyfin:** folder-based audiobook resolver with file-name parsing for parts and chapters.
- **Why it matters:** Multi-file books whose files carry per-part album tags ("Book — Part 1") split into several books in Portico; in Audiobookshelf they cannot. Conversely Portico copes with several books loose in one folder.
- **Assessment:** **Trade-off.** For audiobook libraries I think folder authority is the safer default, because audiobook tagging is far messier than music tagging. Small to medium (an audiobook-library rule in `commitAudio`).

### A-10.40 Audiobook metadata is local only
- **Portico:** Tags, OPF, NFO, Portico JSON. The "audiobooks agent" (Open Library, Wikipedia) was built but never merged (Backlog).
- **Audiobookshelf:** Audible, Audnexus, Google Books, Open Library, iTunes, FantLab, custom providers; cover search. **Jellyfin:** Bookshelf plugin (Google Books, Open Library, ComicVine).
- **Assessment:** **Portico behind.** Audible-derived data (Audnexus) is what gives audiobook libraries narrators, series order and chapters; Open Library has little of that.

### A-10.41 Progress is "which file, and where", ordered by intent
- **Portico:** `book_resume(profile, book) → (file, position)`, protected by per-play ordinals so an old device cannot overwrite a newer choice (`playback/personal_progress.go`).
- **Audiobookshelf:** one book-relative time and a last-update timestamp; latest wins.
- **Assessment:** **Portico ahead** on multi-device correctness. The spec's "position is book-relative (`positionMs` across parts)" (§12) is not how it is stored, which is fine, but it means "X hours left in the book" has to be computed.

### A-10.42 Chapters come only from the file, and validation is all-or-nothing
- **Portico:** Embedded chapters per file; any overlap, an end beyond the duration, or more than 4,096 chapters discards them all (`assets.go:234–251`). Files without chapters get none; the book page lists parts.
- **Audiobookshelf:** OverDrive markers, embedded, or one per file; an editor; online lookup. **Jellyfin:** embedded chapters for audiobooks.
- **Assessment:** **Portico behind.** See B-10.16, F-10.21.

### A-10.43 Sleep timer and automatic stop
- **Portico:** Minutes, end of chapter, end of item, end of book; enforced natively; passout check.
- **Audiobookshelf (apps):** minutes, end of chapter, shake to reset, automatic timer in set hours, rewind after the timer fires. **Finamp:** minutes.
- **Assessment:** **Portico level or ahead** on the essentials; missing the small comforts (F-10.18, F-10.37).

### A-10.44 No podcasts, ebooks or comics
- **Portico:** None. The catalogue has `book` and `audiobook_file` kinds only for audio.
- **Audiobookshelf:** podcasts with RSS and auto-download; EPUB/PDF/CBZ reader. **Jellyfin:** books and comics via the Bookshelf plugin and web readers.
- **Assessment:** **Absent by scope.** A decision for Justin (Open questions).

### System integration and reach

### A-10.45 Lock screen yes; car, watch, widgets and speakers no
- **Portico:** Media Session (web), Now Playing and remote commands (Apple), Media3 session and foreground service (Android). No CarPlay, no Android Auto browsing (a `MediaSessionService`, not a `MediaLibraryService`, `AndroidManifest.xml:74–79`), no widgets, no watch app, no Sonos/DLNA (DLNA is planned).
- **Finamp:** Android Auto and CarPlay. **Audiobookshelf apps:** both. **Plezy:** media keys, companion remote, car driving restrictions. **Plexamp (public):** CarPlay, Android Auto, Sonos, casting.
- **Assessment:** **Portico behind** for a product whose bar is Plexamp: the car is where much music and nearly all audiobook listening happens. Medium per platform.

### A-10.46 No third-party client ecosystem
- **Portico:** Its own API and first-party clients only.
- **Navidrome:** Subsonic/OpenSubsonic (dozens of clients) and now a Jellyfin-compatible API. **Jellyfin:** many clients.
- **Why it matters:** Portico must itself build every surface in A-10.45. Navidrome gets CarPlay, Sonos bridges, desktop players and hardware streamers for free.
- **Assessment:** **Trade-off**, deliberate (no external contributions). An OpenSubsonic facade over the music catalogue is a bounded piece of work (large, but self-contained) that would buy the whole ecosystem; it would bypass the audio plan, so those clients would get ordinary playback.

### A-10.47 Offline music
- **Portico:** Downloads by album, book or playlist selector with offline progress reports (spec §13; `client-core/src/download-requests.ts`). Not reviewed in depth here.
- **Finamp:** the app is built around offline: automatic offline mode, favourites and playlists kept in sync, optional transcoded downloads, play history replayed later.
- **Assessment:** **Unclear** (outside what I read). Worth comparing against Finamp's sync rules in the downloads audit.

## Missing features and capabilities to consider

Sizes are rough: small = days, medium = a week or two, large = more. "Conflict" means a clash with a stated Portico principle.

### Lyrics

- **F-10.1 Local lyrics that just appear.** Sidecar and embedded lyrics shown without any action. Jellyfin finds sidecars at scan and extracts embedded lyrics (`AudioFileProber.cs:562–573`); Navidrome reads them on demand by a priority list. Value: high for anyone who already curated lyrics. Small (the local reader exists: `storage/lyrics.go`). No conflict. Opinion: do it; it is the single cheapest improvement in this domain.
- **F-10.2 Word-level and richer lyric formats.** Enhanced LRC word timing (Jellyfin keeps cues, `LrcLyricParser.cs`), TTML and SRT (Navidrome). Value: karaoke-style highlighting, which Plexamp and Apple Music have made expected. Small to keep the word stamps Portico already parses and discards; medium for TTML. Opinion: keep the data now even if no client draws it yet.
- **F-10.3 Scheduled or on-add lyric fetching.** Jellyfin has a scheduled lyric download task; Portico's bulk run is manual. Small once B-10.5 and B-10.6 are fixed. Mild conflict with "no periodic chatter" (that principle is about Hosted, not LRCLIB); run it on library change, not on a timer.

### Discovery, radio and statistics

- **F-10.4 Radio / instant mix / keep playing when the queue ends.** Finamp's five radio modes; Jellyfin's instant mix; Navidrome's similar songs; Plexamp's radios. Value: high; it is most of what people mean by "Plexamp". Medium to wire the existing server mix into a `mix` selector and an "autoplay similar" queue tail; the quality of the source is the open question (Backlog).
- **F-10.5 A similarity source.** Options seen: shared genres only (Jellyfin); external services (Navidrome: Last.fm, ListenBrainz, Deezer; conflicts with "no keys, no extra sources"); sonic analysis on the server (Plex, closed). Portico already decodes every file twice (facts, loudness); a compact audio-feature vector from the same pass is the Portico-shaped answer. Large.
- **F-10.6 Scrobbling to Last.fm and ListenBrainz.** Navidrome (per user, with a local scrobble history and a "first artist only" option); Jellyfin by plugin. Value: high for music people; it is also the cheapest backup of listening history. Medium (per-profile account linking, a durable outbox, offline replays). No conflict if per-profile and off by default.
- **F-10.7 Smart playlists.** Navidrome `.nsp` rule files with a refresh delay; Plex smart playlists. Portico already has a browse filter tree and saved views; a playlist whose content is a saved query is the natural form (the spec lists `query` and `savedView` selectors as "not in the first release"). Medium.
- **F-10.8 M3U import and export.** Navidrome imports `.m3u` found in the library and exports any playlist; Jellyfin stores playlists as files. Value: migration from every other player. Small to medium (path matching against assets).
- **F-10.9 Listening statistics and a year in review.** Audiobookshelf (minutes per day, streaks, year in review); Navidrome's scrobble history. Portico has the play history and counts to build it. Medium.
- **F-10.10 Sort by plays, most played, recently played, never played, top rated.** Finamp's genre and artist "curated" sections (most played, latest releases, favourites). Backlog already has "Sort Songs by plays". Small to medium each.

### Tagging and library model

- **F-10.11 Sort tags and ignored articles.** Navidrome (`prefersorttags`, `ignoredarticles`). Small.
- **F-10.12 Multi-valued artists and MusicBrainz artist identity.** See A-10.5 and A-10.6. Medium.
- **F-10.13 More audio formats.** See A-10.2. Small for FFmpeg-decodable formats; DSD is its own decision.
- **F-10.14 Cue sheets.** One FLAC/APE/WAV per album with a `.cue` is common in ripped collections; Jellyfin lists `.cue` among audio extensions; many players split by it. Portico's asset links already support a start and end inside a file (`catalog_asset_links.start_seconds/end_seconds`), so the model is ready. The spec also names cue sheets as a chapter source for audiobooks (§12). Medium.
- **F-10.15 Roles (composer, conductor, performer) and classical support.** Navidrome participants. Large; a composer filter is small.
- **F-10.16 Disc subtitles and disc art.** Navidrome (`discartpriority`, disc subtitle tags). Small.
- **F-10.17 Album versions grouped as one album** (several releases of one release group). Navidrome `scanner.groupalbumreleases`. Portico's edition key keeps them apart; a "Versions" control on the album page would mirror what the Backlog asks for movies. Medium.
- **F-10.18 Merge and split albums by hand.** Plex has it; Portico has Fix Match on the server but no way to say "these two albums are one" or "move these tracks out". Medium. Needed while A-10.3 stands.
- **F-10.19 Music videos in a music library.** The Page Content spec lists them; audio libraries reject video files (`assets.go:292–295`). Medium.
- **F-10.20 Ratings on albums and artists; "Want to listen" for books.** Already in the Backlog.

### Playback

- **F-10.21 Automatic normalisation mode and a pre-amp.** Finamp's hybrid mode; a pre-amp is standard in ReplayGain players. Small (A-10.20).
- **F-10.22 "Play next" that works under shuffle, and a visible Next Up lane.** Finamp. Medium (A-10.29).
- **F-10.23 Fallback when the device cannot play a direct plan.** jellyfin-web retries a failed direct play as a transcode. Portico fails the playback (B-10.34). Small to medium: a "this plan failed on this device" signal that makes the server answer `converted` for that asset and device.
- **F-10.24 A lossy conversion target Apple can play, and a conversion cache.** Navidrome's transcoding cache; Jellyfin's AAC. Medium (A-10.18).
- **F-10.25 Fade on pause, resume and seek; ducking for interruptions.** Finamp (`audioFadeOutDuration`, `duckOnAudioInterruption`). Small.
- **F-10.26 Equaliser, output device choice, exclusive or bit-perfect output.** Plexamp; mpv-based clients. Large; desktop-app territory.
- **F-10.27 Speed up to 3× in finer steps; per-book speed.** Audiobookshelf. Small.
- **F-10.28 Skip silence and voice boost for spoken word.** Common in podcast and audiobook apps; ExoPlayer has skip-silence built in. Small on Android, medium elsewhere.

### Audiobooks

- **F-10.29 Bookmarks with notes.** Audiobookshelf (`/me/item/:id/bookmark`); promised in Portico's spec (§12). Small to medium.
- **F-10.30 Rewind a little on resume.** Audiobookshelf apps and most audiobook players rewind a few seconds (more after a long pause). Small, client-core.
- **F-10.31 Chapters from more places, and an editor.** One chapter per file when files have none; OverDrive MediaMarkers; lookup by ASIN (Audnexus); manual editing (Audiobookshelf). Medium.
- **F-10.32 An audiobook metadata agent with narrators, series and covers.** Audible/Audnexus-derived data; Open Library; Google Books. The unmerged "audiobooks agent" is the start. Medium to large. Reading narrator from the composer tag (B-10.14) is the small first step.
- **F-10.33 A book-level position and time left.** "6 h 12 m left" on the book page and lock screen, computed across files. Small.
- **F-10.34 Podcasts.** Audiobookshelf (RSS subscribe, auto-download, episode progress). Large. A scope decision.
- **F-10.35 Ebooks and comics.** Audiobookshelf reader; Jellyfin Bookshelf. Large. A scope decision.
- **F-10.36 Merge to M4B / embed metadata tools.** Audiobookshelf. Conflicts with Portico never modifying the owner's files; I would not.
- **F-10.37 Sleep-timer comforts.** Shake or tap to extend, fade out at the end, an automatic timer during set hours, rewind after the timer stops playback (Audiobookshelf apps). Small each.
- **F-10.38 Kids' audiobooks.** Nothing found in the others beyond per-user library access and content ratings, which Portico's profile restrictions already cover; audiobook tags rarely carry ratings, so restrictions by rating will not see most books. Worth a check in the restrictions audit.

### Reach

- **F-10.39 CarPlay and Android Auto.** Finamp, Audiobookshelf, Plexamp. Android needs a `MediaLibraryService` and a browse tree; CarPlay needs the audio entitlement and templates. Medium each. Opinion: the highest-value missing surface for both music and audiobooks.
- **F-10.40 Home-screen widget, watch controls, Siri and Assistant ("play … in Portico").** Platform intents. Small to medium each.
- **F-10.41 Sonos, DLNA renderers, multi-room.** Plexamp (Sonos), Jellyfin (DLNA plugin). DLNA is planned. Large.
- **F-10.42 An OpenSubsonic-compatible API.** See A-10.46. Large but self-contained.
- **F-10.43 Public share link for a track, album or playlist.** Navidrome shares with expiry and optional download. `client-core` has `public-links.ts`; I did not check whether music is covered.
- **F-10.44 Offline-first conveniences.** Automatic offline mode, favourites kept offline, transcoded downloads, history replay (Finamp). To be judged in the downloads audit.

## Possible bugs and risks in Portico

All of these come from reading source. None was reproduced. "Confirmed by reading" means the code path is unambiguous; whether it bites depends on real files and devices.

### B-10.1 An audiobook file is marked finished, and its resume point reset to zero, at 95%
- **What goes wrong.** For any item that is not a film or episode, "completed" is `position ≥ duration × playedThreshold` with a default of 95 (`internal/catalog/personal_activity.go:44–48`, `:57`, `:69`; `internal/operations/preferences.go:178`). When completed, the item becomes watched and its progress is set to 0 (`:113–116`), and the book's resume row is written with that 0 (`:141`). Every later report in the last 5% does the same. So stopping in the final 5% of an audiobook file loses the place: a single-file 12-hour book cannot be resumed in its last 36 minutes, and is shown as finished; in a multi-file book, pausing near the end of a part restarts that part from its beginning.
- **How the comparison surfaced it.** Jellyfin has separate audiobook rules (finished with under 5 minutes left, `ServerConfiguration.cs:151–157`); Audiobookshelf finishes with under 10 seconds left (`MediaProgress.js:232`).
- **Confidence:** Confirmed by reading. **Severity:** High for audiobooks. **Backlog:** not listed.
- **Suggested direction.** For book files, complete on `ended` or on a small time remaining, and never zero the resume position before the next file has started.

### B-10.2 All local artwork shares one 128 MiB folder; after that, covers are silently dropped
- **What goes wrong.** `localmetadata.Service.store` sums the size of every file in `<state>/local-artwork` and refuses a new image with "local artwork cache full" once the total would pass 128 MiB (`internal/localmetadata/audio.go:126–138`; the folder is set at `cmd/server/main.go:435`). It is the permanent home of the image (the catalogue stores `local:<key>` and `Open` reads from it, `:156–171`), not a cache. Sidecar covers, embedded covers, movie posters and show art all go there. At 100–300 KB per cover that is roughly 400–1,300 images for the whole server. Later albums get no local cover; the failure sets no visible issue for sidecars (`:59–64` only flags an invalid image). Each store also lists the whole folder first.
- **How the comparison surfaced it.** Navidrome and Jellyfin keep originals in place and bound only a resize cache.
- **Confidence:** Confirmed by reading (I found no pruning of this folder other than the owner's maintenance action). **Severity:** High; it contradicts the 5–10 million item goal. **Backlog:** not listed.

### B-10.3 "Play next" on a shuffled queue adds to the very end
- **What goes wrong.** `if l.q.shuffle { where = "end" }` (`internal/playbackv1/queue_commands.go:744–748`). With a shuffled 3,000-song queue, the track chosen with "Play next" plays after the other 2,999.
- **Evidence of intent.** The comment cites spec §17.10, so it is deliberate; the client still offers the action as play-next (`v1-queue-player.ts:511`).
- **How the comparison surfaced it.** Finamp's `NextUpShuffleOrder` exists precisely to keep Next Up after the current track under shuffle.
- **Confidence:** Confirmed by reading. **Severity:** Medium (a visible everyday surprise). **Backlog:** not listed.

### B-10.4 Removing or moving one entry in a shuffled queue reshuffles everything and forgets what was played
- **What goes wrong.** After a remove or move while shuffled, the server sets the shuffle domain to the whole queue, makes the current entry position 0, and derives a new order (`queue_commands.go:838`, `:844`, `:939`). Tracks already heard are back in the upcoming list; "Previous" has nothing behind it; entries added earlier as the unshuffled tail are folded into the shuffle; and a dragged entry does not land where it was dropped, because the drop position is in source order and the play order is recomputed.
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-10.5 A library-wide lyrics run that is rate-limited once races through the rest of the library as "missing"
- **What goes wrong.** After a 429 or 503 the LRCLIB client refuses further calls until the provider's deadline, returning `ErrUnavailable` immediately when the wait would exceed 5 seconds (`internal/lyrics/lrclib.go:40–48`, `:68–82`). The bulk run treats any search error as "missing" and moves its cursor on (`internal/lyrics/bulk.go:307–311`, `:215–236`). With a 60-second pause in force, every remaining song is counted missing within moments and the run ends as "succeeded".
- **Confidence:** Confirmed by reading. **Severity:** Medium (the feature quietly does nothing for large libraries; no data is damaged and a rerun retries). **Backlog:** not listed.

### B-10.6 The bulk lyrics run publishes the first search hit without checking it is the same song
- **What goes wrong.** The query is "title + first artist" sent to LRCLIB's free-text search (`bulk.go:298–306`, `lrclib.go:130`). The first candidate whose timestamps fit inside the track is published for the whole library (`bulk.go:312–326`); the result's title, artist, album and duration are not compared with the song (duration is not even read from the response, `lrclib.go:155–163`). Plain lyrics have no timestamps, so any plain result "fits". Common titles ("Intro", "Home", live versions, covers) will get another song's words.
- **How the comparison surfaced it.** LRCLIB's own exact-match endpoint takes track, artist, album and duration; Jellyfin's LRCLIB plugin and Navidrome's lookups use signatures like that.
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-10.7 Retagging a file hides its lyrics, and the bulk run will not fetch them again
- **What goes wrong.** A lyric resource is stored against a "source version" that hashes the file's size and modified time (`internal/lyrics/service.go:155`); the view only returns resources with the current version (`:277`). Any change to the file (new tags, ReplayGain written, cover embedded) orphans every lyric for it. The bulk run's "has no lyrics" test ignores the version (`bulk.go:80–82`), so it skips the song for ever. Nothing rebinds or deletes the orphan (no `UPDATE lyric_resources SET source_version` exists).
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-10.8 One unexpected line makes a whole LRC file invalid
- **What goes wrong.** In LRC mode a line with no timestamp that is not one of twelve known headers rejects the document (`internal/lyrics/document.go:51`, `:144`, `:174–176`). So does a timestamp with seconds ≥ 60 (`:155`), a second `[offset:]` line (`:131`), any line over 8 KB, or `[mm:ss:xx]` stamps (they do not match `:49`). Real `.lrc` files often start with a blank-tagged title line, a `[id:…]`, `[encoding:…]`, `[total:…]` or a credit line. Separately, any synced line later than the track's duration plus one second rejects the lyric at acquisition (`:222–235`), which will hit lyrics timed for a longer edit.
- **How the comparison surfaced it.** Jellyfin's parser skips what it cannot read and falls back to plain text.
- **Confidence:** Confirmed by reading; how often it bites is Probable. **Severity:** Medium. **Backlog:** not listed.

### B-10.9 End padding longer than one frame is under-counted
- **What goes wrong.** The facts parser keeps only the **last** packet's `discard_padding` (`internal/audiofacts/facts.go:79–88`, `:137`). FFmpeg sets it on every packet that overlaps the discard region (checked in FFmpeg `libavformat/demux.c`: `discard_padding = FFMIN(end_sample - first_discard_sample, duration)` per packet). For MP3, FFmpeg's discard length is the LAME end padding minus 529; LAME writes paddings from 576 to 1,727, so when it exceeds 1,681 the region spans two packets and the earlier packet's share (1–46 frames, about a millisecond) is lost: the track is that much too long and a continuous album gets that much silence at the join. Encoders that pad more (the field allows 4,095) lose more. The parser's own consistency check cannot notice, because it derives the duration from the same numbers.
- **Confidence:** Probable (the Portico side is confirmed; the FFmpeg side is from its source, not from a run). **Severity:** Low. **Backlog:** not listed. The fixtures do not include such a file (`facts_test.go:63–71`).

### B-10.10 HE-AAC is planned as plain "AAC" and will not decode on the web engine
- **What goes wrong.** Facts hold codec `aac` and FFmpeg's output sample rate; no profile (`facts.go:95–103`). The plan chooses `direct` for any device that declared AAC (`audio_plan_v2.go:99–102`), and the web declares AAC after testing only AAC-LC (`web/src/bridge/audio/capabilities.ts:44–47`). The web demuxer takes the sample rate from the first index in the AudioSpecificConfig, which for HE-AAC is the **core** rate (half the output rate), and then refuses the file because "The file's sample rate isn't the plan's" (`mp4.ts:89–95`, `track.ts:85`). Low-bitrate audiobooks, podcasts and radio rips are often HE-AAC. What Core Audio and ExoPlayer do with the plan's frame counts for such files is untested by the fixtures.
- **Confidence:** Probable. **Severity:** Medium (with B-10.34 it is a playback failure, not a quality loss). **Backlog:** not listed.

### B-10.11 A bitrate limit on an Apple device turns the audio plan off
- **What goes wrong.** When the viewer's limit is below the file's bitrate, the plan wants Opus; the FLAC branch is excluded while limited (`audio_plan_v2.go:191`, `:205`). Apple devices do not declare Opus (`apps/portico-react-native/apps/native/src/bridge/audio-decode.ts:14–20`), so the result is `unavailable` with the text "This device can't decode this track's format or the converted formats (FLAC, Opus)" (`:223–227`), which is untrue for FLAC, and the track plays without gapless, crossfade or normalisation. Every FLAC or ALAC track is over a typical mobile limit.
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-10.12 Long files cannot be measured in the 5-second play-time budget, and each attempt is thrown away
- **What goes wrong.** A play of an unmeasured file runs the full-decode measurement with a 5-second limit (`internal/playbackv1/audio.go:39–43`). A multi-hour audiobook file needs far longer; the attempt is cancelled, nothing is recorded (`audiofacts/store.go:82–90`), the plan is `unavailable`, and the next play repeats the wasted 5 seconds until the background pass reaches that file. The pass measures one file at a time in token order across the whole server (`store.go:171–246`), so on a new large library that can be days.
- **Confidence:** Probable (decode speed is an estimate). **Severity:** Medium on first import, Low afterwards. **Backlog:** not listed. Related: spec §18.7 says the play-time measurement is "packet read only".

### B-10.13 A listener's conversion runs as background work and restarts from the file's beginning on every seek
- **What goes wrong.** `ConvertAudio` reuses the analysis runner, which hard-codes `background: true` (`internal/decoder/analysis.go:127`; `internal/subtitlevideo/audio_convert.go:24`), so a stream someone is listening to runs at the lowest priority (`internal/mediaexec/shim_unix.go:47–49`) on one thread, competing equally with scans and analysis. A seek sends `?fromFrame=N`; FFmpeg decodes from frame 0 and trims (`analysis.go:62`), so seeking to hour 9 of a converted audiobook decodes nine hours before the first byte.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (weak boxes, long files). **Backlog:** not listed. The architecture doc says interactive conversions are not background priority.

### B-10.14 Narrators are read only from a `narrator` tag
- **What goes wrong.** `narrator := firstTag(f.Tags, "narrator")` (`audio_repository.go:467`). Most audiobook tools, Audiobookshelf's own embedder included, write the narrator to the **composer** field; MP4 has no standard narrator atom. Such books show no narrator unless an OPF sidecar exists. The same files usually carry the description in `comment` (not whitelisted) and the series in `grouping` or `mvnm` (not whitelisted).
- **How the comparison surfaced it.** Jellyfin (`AudioFileProber.cs:269`) and Audiobookshelf (`AudioFileScanner.js:221–224`) both map composer → narrator.
- **Confidence:** Confirmed by reading. **Severity:** Medium for audiobook libraries. **Backlog:** related to the unmerged audiobooks agent.

### B-10.15 Audiobook parts without a track tag or a leading number play in catalogue order, not file-name order
- **What goes wrong.** The part number is the `track` tag or digits at the very **start** of the file name (`audio_repository.go:93`, `:270–275`). "Chapter 01.mp3", "Book Title - 03.mp3", "Part 2 of 12.mp3" get none. Ordering is then disc, part, entity id (`internal/playbackv1/selectors.go:155`; `catalog/audio_queries.go:243`), and entity ids follow the order files were first catalogued. A file added later, or a scan that visited files out of order, plays out of sequence permanently.
- **How the comparison surfaced it.** Audiobookshelf and Jellyfin parse numbers anywhere in the name and fall back to a natural sort of file names.
- **Confidence:** Probable (I did not confirm the scan's visiting order). **Severity:** Medium. **Backlog:** not listed.

### B-10.16 One bad chapter discards all chapters
- **What goes wrong.** A chapter that starts before the previous one ends, or ends more than 0.1 s past the duration, or a file with more than 4,096 chapters, sets the status to invalid and drops every chapter (`internal/assets/assets.go:234–251`). M4B files made by merging tools commonly have a last chapter that overruns slightly or millisecond overlaps.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium. **Backlog:** not listed. Clamping and skipping the bad entry would keep the rest.

### B-10.17 Common album layouts split into several albums
- **What goes wrong.** Three separate causes, all from the album key (`audio_repository.go:341–344`, `:377–414`):
  1. **Year per track.** The year is the first four characters of each file's `date`/`year`. Compilations and remasters often tag each track with its original year; each year becomes its own album.
  2. **No album-artist tag.** Each distinct track artist becomes an album artist, so a soundtrack or a "feat." track splits off (A-10.7).
  3. **Disc folders with a suffix** ("Disc 1 - Live"), or the same album in two folders (A-10.4).
- **Confidence:** Confirmed by reading. **Severity:** Medium (the most common complaint about tag-keyed music servers). **Backlog:** only "Untagged albums in the main Albums grid".

### B-10.18 ReplayGain stored in APEv2 tags on MP3 files is not seen
- **What goes wrong.** Portico reads ReplayGain from ffprobe's tag dictionary (`audio_plan_v2.go:303–322`). FFmpeg's MP3 demuxer reads ID3v2 and ID3v1 only (checked in `libavformat/mp3dec.c`: no APE tag parsing). mp3gain and some foobar2000 setups write ReplayGain as APEv2. Those files fall back to server analysis, or to no normalisation when the library is not on the Complete scan tier.
- **Confidence:** Probable. **Severity:** Low. **Backlog:** not listed.

### B-10.19 Covers with other names, capitalisation, size or location are missed
- **What goes wrong.** Only `cover.jpg`, `folder.jpg`, `cover.png`, `folder.png`, read by exact path (`localmetadata/audio.go:55–56`), so `Folder.jpg`, `Cover.JPG`, `front.jpg`, `album.jpg`, `cover.jpeg`, `cover.webp` are not found on a case-sensitive file system. Images over 4 MiB or 16 megapixels are rejected (`:112–116`). For `Album/CD1/track.flac` a cover in `Album/` is not looked for. The folder cover is applied to every file in the folder even when they belong to different albums. Embedded art is always the first picture stream (`:78`), which is not always the front cover.
- **Confidence:** Confirmed by reading for names and limits; Probable for case sensitivity (I saw no case folding in the read path). **Severity:** Medium. **Backlog:** not listed.

### B-10.20 On the web, a gapless join between tracks that need resampling is two filtered edges
- **What goes wrong.** Each deck owns a resampler whose history starts as silence and whose tail is flushed against silence (`web/src/bridge/audio/resampler.ts:54`, `:108–112`; `engine.ts:69`). When the context rate differs from the album's rate (the context is created once at the first track's rate, `DecodeAudioRender.ts:64`), the last ~0.7 ms of one track and the first ~0.7 ms of the next are each filtered as if the music stopped, instead of across the join.
- **Confidence:** Confirmed by reading; audibility is Speculative (needs a listening test or the album fixture run at a mismatched rate). **Severity:** Low. **Backlog:** not listed.

### B-10.21 In Firefox every lossy track is converted to FLAC
- **What goes wrong.** The per-browser table lists no WebCodecs codecs for Gecko (`web/src/bridge/audio/track.ts:28–33`), so only FLAC (JavaScript decoder) and WAV play directly. MP3, AAC, Opus and Vorbis become FLAC conversions (several times the bandwidth, one FFmpeg per listener), or `unavailable` if the owner forbids conversion. Vorbis is converted in every browser, since no web capability declares it (`capabilities.ts`).
- **Confidence:** Confirmed by reading. **Severity:** Medium as a design risk. **Backlog:** not listed.

### B-10.22 Android cannot raise quiet tracks
- See A-10.21 (`PorticoAudioPlan.kt:77–88`). **Confidence:** Confirmed by reading. **Severity:** Low.

### B-10.23 AAC with only an edit list: the end is not trimmed, and Apple may cut the start
- **What goes wrong.** Without `iTunSMPB`, the start comes from the MP4 edit list (via FFmpeg) but the end padding inside the last AAC frame is not removed; the test file says as much ("FFmpeg's AAC in MP4: an edit list for the priming, no end padding", `facts_test.go:86–88`). The plan still labels the trim `mp4-editlist`, so up to 1,023 padded frames (about 23 ms) play at each join. On Apple, if Core Audio assumes its usual 2,112-frame AAC priming for a file whose edit list says 1,024 (FFmpeg-encoded AAC), `MAX(0, start − primed)` is 0 and the first 1,088 real frames are skipped (`PorticoAudioRender.m:281–288`).
- **Confidence:** Speculative for the Apple part (depends on Core Audio); Probable for the end padding. **Severity:** Low. **Backlog:** not listed. No fixture covers edit-list-only AAC.

### B-10.24 Embedded cover art inflates the "bitrate" used for the limit test
- **What goes wrong.** `sourceBPS` is file size × 8 ÷ duration (`audio_plan_v2.go:158–162`). A 3-minute 192 kb/s MP3 with a 5 MB embedded cover computes as about 410 kb/s and is converted to Opus under a 320 kb/s limit although the audio is under it.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-10.25 The instant mix pads from the same 192 songs every time
- **What goes wrong.** The candidate pool's "library" branch is the 192 lowest-id songs of the library (`listening_selection.go:236`), and results are ordered strictly by score tier (seed, same artist, same album, same genre string) before the random hash, so a mix is "everything by this artist, then the album, then the same early-scanned songs". Genre matching compares the raw tag text, so "Rock; Pop" does not match "Rock" (`:241`).
- **Confidence:** Confirmed by reading. **Severity:** Low today (no screen calls it); it matters when radio is built on it.

### B-10.26 Tracks with no disc number sort before disc 1
- **What goes wrong.** Album order is `disc_number, track_number, id` with missing treated as 0 (`selectors.go:149`; `audio_queries.go:186`). When only some files of an album carry a disc tag, the untagged ones play first.
- **Confidence:** Confirmed by reading. **Severity:** Low. Navidrome treats a missing disc as 1.

### B-10.27 Normalisation comes and goes while analysis catches up
- **What goes wrong (design risk).** With no tags and no analysis yet, the plan carries no gain and the client plays that track at full level (`audio_plan_v2.go:327–331`; `audio-render.ts:111–113`). Album gain appears only once every track of the album is measured (`:339–349`), at which point "album" mode switches from track gains to the album gain. During a first import, levels jump between and within albums.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-10.28 The gapless fixtures leave whole classes of file untested (design risk)
- Missing from `fixtures/audio-gapless/` (spec §18.8): AAC with an edit list and no `iTunSMPB`; HE-AAC; MP3 without a LAME header; MP3 with LAME padding above 1,681; Vorbis; an album whose rate differs from the output rate; a multi-hour file. Each maps to an entry above. **Severity:** Medium as a process gap.

### B-10.29 Files with unlisted extensions vanish without a word
- **What goes wrong.** `.aif`, `.oga`, `.mka`, `.wma`, `.ape`, `.wv`, `.dsf`, `.mp4` and others are not "supported for kind" (`assets.go:285–295`), so an audio library ignores them. The FFmpeg whitelists show the intent to read several of them (`assets.go:89`; `audiofacts/facts.go:48`). I did not find a place that tells the owner files were skipped.
- **Confidence:** Confirmed by reading for the gate; Probable for the lack of notice. **Severity:** Medium.

### B-10.30 The web MP3 reader gives up if audio does not start within 8 KB after the ID3 tag
- **What goes wrong.** After skipping one ID3v2 tag the demuxer searches 8,192 bytes for two agreeing frame headers (`web/src/bridge/audio/mp3.ts:57–71`). Files with padding, a second tag, or junk before the first frame fail with "No MPEG audio frames were found", although FFmpeg measured them. With B-10.34 that is a failed play.
- **Confidence:** Probable. **Severity:** Low.

### B-10.31 Any `<track name>.txt` is offered as lyrics
- `storage/lyrics.go:105` lists `stem + ".txt"` as a plain-lyrics sidecar. Rip logs and notes that share the track's name would be offered. **Confidence:** Confirmed by reading. **Severity:** Low (it is a candidate, not auto-published).

### B-10.32 The lock-screen entry for an audiobook has no author
- The web Media Session fills artist and album only from `song` (`web/src/bridge/useSystemListening.ts:25–26`). **Confidence:** Confirmed by reading. **Severity:** Low.

### B-10.33 Small queue edge cases
- "Previous" at the first entry with repeat-all stays put rather than wrapping (`queue_commands.go:1013–1014`). With shuffle and repeat-all, a new lap uses a new permutation, so the last track of one lap can be the first of the next (`:1019–1023`). After a lap wraps there is no way back into the previous lap. **Confidence:** Confirmed by reading. **Severity:** Low.

### B-10.34 When the device's engine cannot play a direct plan, playback fails instead of falling back
- **What goes wrong.** Any demux or decode error in the web engine ends in `audioRenderFailed`, which calls `playback.fail` (`web/src/bridge/DecodeAudioRender.ts:162`, `:174`, `:245`; `client-core/src/playback-v1/v1-queue-player.ts:724–728`). I found no retry as `converted` and no switch to the ordinary stream; "Retry playback to reconnect" will fail the same way. The server chose `direct` from coarse facts (codec name and container), so the device is the only party that knows, and it cannot say so.
- **How the comparison surfaced it.** jellyfin-web answers a playback error by retrying with direct play disabled.
- **Confidence:** Probable (I did not trace what `fail` leads to in every client). **Severity:** Medium; it turns each decode gap (B-10.10, B-10.30, damaged files) into "this song will not play".

## Where Portico is stronger

- **Sample-exact gapless in a browser.** Nobody else here attempts it. The plan, the fixtures with known answers, and the per-engine conformance table are a serious piece of engineering.
- **One loudness model with peaks.** Tags first, R128 converted correctly to the ReplayGain reference, server analysis as fallback, peak-limited gain on the client. Jellyfin stores a gain without a peak; Navidrome only forwards tags.
- **A queue that scales and is shared.** Server-owned, revisioned, windowed, with a constant-time shuffle: a "shuffle the whole library" of ten million songs is one request, and another device can drive it.
- **Accounting at the audio boundary.** Prepare/commit means play counts, history and Now Playing change exactly when the sound does, and a pre-buffered track that never played leaves nothing behind.
- **Progress that survives several devices.** The intent-ordinal scheme stops an old phone overwriting a newer resume point; the others are last-write-wins.
- **Two histories.** The viewer can pause their own history; the owner's play history is separate and says whether a play was converted.
- **Conservative matching with evidence.** Every tag keeps its source; MusicBrainz matches publish only on strong evidence; there is a review path; the library can be local-only.
- **Confinement.** Every probe, measurement and conversion runs in the sandboxed FFmpeg with a private loopback input; lyric sidecars are read through `os.Root` with change detection; LRCLIB requests refuse private addresses and redirects.
- **Lyrics with provenance and scopes.** A viewer's private lyric and the library's lyric are distinct, with revisions and a rights note.
- **Listening policy that outlives JavaScript.** Sleep timers, passout checks and auto-advance are enforced by the native owners while the app is suspended.
- **Collaborative playlists** with roles and idempotent edits, with no size cap.
- **Speed with pitch correction on every platform**, including the web's own time-stretcher.

## Doc/code disagreements

| # | Document says | Code does |
|---|---|---|
| D-10.1 | Spec §18.7: audio facts are a "single-pass packet read, no decode", decoding only when headers are missing or inconsistent; at play time "one file, packet read only". | Always decodes every frame (`audiofacts/facts.go:53–59`, package comment `:1–9`). |
| D-10.2 | Spec §12: "transcode to AAC or Opus at the requested bitrate"; playback options carry `gapless: {encoderDelaySamples, paddingSamples}`. | FLAC or Opus only; the fields are `trim.startFrames/endFrames` (§18.1 supersedes §12 but §12 was not updated). |
| D-10.3 | Spec §12: speed 0.5–3.0. | Seven steps, 0.5–2.0 (`operations/preferences.go:120`; `client-core/src/listening.ts:11`). |
| D-10.4 | Spec §12: bookmarks, `POST /v1/items/{id}/bookmarks {positionMs, note}`. | No bookmark code on the server (no match for "bookmark" outside an unrelated client flag). |
| D-10.5 | Spec §12: chapters "from tags, cue sheets or files"; position is book-relative across parts. | Chapters from embedded tags only; no cue-sheet reader; progress is per file. |
| D-10.6 | Spec §12: `GET /v1/items/{id}/lyrics` returns synced or plain lyrics. | Lyrics are selected resources with explicit acquisition (`lyrics/service.go`); nothing is returned for a song until someone publishes a lyric. |
| D-10.7 | `03 Catalogue and Metadata.md`: music uses "MusicBrainz, with AcoustID fingerprints for matching"; "Keys ship with the server … the owner configures nothing". | AcoustID needs `PORTICO_ACOUSTID_CLIENT_KEY` (`cmd/server/main.go:714`) and a fingerprint already embedded in the file; the server "does not generate fingerprints" (`metadataprovider/fingerprint.go:12`). |
| D-10.8 | Page Content spec §8: audiobooks agent uses "the existing audiobook metadata, then Open Library, then Wikipedia"; §3: "Music videos if the library has them". | No online audiobook source (Backlog confirms unmerged); audio libraries reject video files (`assets.go:292–295`). |
| D-10.9 | Backlog: "`musicArtistStep` groups every album on every step". | The step walks a 64-artist window with a cursor (`metadata/music_artist.go:42–47`); this item looks already done. |
| D-10.10 | `04 Playback and Media.md`: "Background conversions run at the lowest OS priority; interactive ones don't." | Converted audio streams for a listener run with `background: true` (`decoder/analysis.go:127`). |
| D-10.11 | `04 Playback and Media.md`: web uses WebCodecs, "the Apple app uses native modules". | Android has a third engine (ExoPlayer) that does not apply the plan's trim (`PorticoAudioPlan.kt:9–11`); the doc predates the port. |
| D-10.12 | Spec §1 parity table: "play next, add to queue" at parity. | Under shuffle, play next is add-to-end (spec §17.10 and `queue_commands.go:744–748`). |
| D-10.13 | `client-core/src/playback-v1/audio-render.ts:9`: the web engine is `WebAudioRender`. | It is `DecodeAudioRender`. Trivial. |
| D-10.14 | Spec §18.1: mode `converted` when facts "can't be measured in time". | An unmeasured file gets `unavailable` (`audio_plan_v2.go:149–152`). |

## Open questions for Justin

1. **Audiobook completion.** Should a book file finish on time remaining (for example under 30 seconds) rather than at 95%? And should the resume point ever be reset before the next part starts? (B-10.1)
2. **Album identity.** Keep "folder + year in the key, never merge", or let an identical MusicBrainz release id / matching disc evidence join discs, take the year from the album as a whole, and add a manual merge and split? (A-10.3, B-10.17)
3. **Missing album artist.** Group a folder's differently-credited tracks as Various Artists when none has an album-artist tag? (A-10.7)
4. **Multi-artist credits.** Honour the embedded `ARTISTS` tag? Split free-text artists ("feat.", ";", " / ") with a do-not-split list, or stay with Plex's "never split"? (A-10.6)
5. **Shuffle and "Play next".** Is "added entries go after the shuffled body" still what you want, given Finamp and Plexamp put them next? Should editing a shuffled queue keep what was already played? (B-10.3, B-10.4)
6. **Local lyrics.** Should sidecar and embedded lyrics appear automatically, with the curated model kept for uploads and LRCLIB? (A-10.34)
7. **Lyrics and file changes.** Should a lyric follow the song when the file is retagged? (B-10.7)
8. **Normalisation default.** Leave it off, or on with an automatic album/track mode? Is loudness analysis meant to be in the default scan for music libraries?
9. **Bitrate limits on Apple devices.** Add an AAC target, let Apple take Opus through another container, or accept ordinary playback when limited? (B-10.11)
10. **Firefox.** Accept FLAC conversion for all lossy music there, or test and enable its WebCodecs decoders? (B-10.21)
11. **Local artwork limit.** What should bound local art: nothing (it is the owner's data, content-addressed), or a much larger, visible budget? (B-10.2)
12. **Formats.** Which extra extensions should audio libraries accept, and what should happen to DSD?
13. **Audiobook authority.** In audiobook libraries, should the folder define the book as in Audiobookshelf? Should composer be read as narrator? (A-10.39, B-10.14)
14. **Audiobook metadata.** Is an Audible-derived source (Audnexus) acceptable inside "Portico Audiobooks", or only Open Library and Wikipedia?
15. **Scope.** Are podcasts, ebooks and comics out for good, later, or wanted?
16. **Car.** Are CarPlay and Android Auto in scope before release, given the Plexamp bar?
17. **Scrobbling.** Is per-profile Last.fm/ListenBrainz scrobbling acceptable (it sends listening data to a third party, by the listener's choice)?
18. **Radio.** Which similarity source: genres and co-listening from the server's own history, server-side audio features, or an external service?
19. **A Subsonic-compatible API.** Worth having for reach, knowing those clients would not use the audio plan?
20. **Measuring cost.** Is a full decode of every track (twice, with loudness) acceptable for a first import of a very large library, or should facts follow the spec's packets-first design?
21. **Speed.** Extend to 3× and finer steps? Remember speed per book?
22. **Played rule for songs.** Keep 95%, or follow the scrobbling convention (half the track or four minutes)?

## Evidence boundary

- **Nothing was executed.** No server, client, test, FFmpeg or ffprobe run. Every behaviour above is read from source.
- **Decoder behaviour is asserted from code comments and FFmpeg source, not measured.** What Chrome, Safari, Firefox, Core Audio and ExoPlayer actually output for HE-AAC, edit-list-only AAC, MP3 without a LAME header, Vorbis and long files is unknown to this review. B-10.9, B-10.10, B-10.20 and B-10.23 need the fixture harness (`web/audio-conformance.html`, the Apple exit tests) run on new fixtures.
- **Timing claims are estimates.** Decode speeds, MusicBrainz import durations and the artwork count that fits in 128 MiB are arithmetic from typical values.
- **Native owners were skimmed.** `PorticoListeningOwner.m/.kt` (background queue ownership, sleep policy, prepare/commit on native) and the deck scheduler in `PorticoAudioRender.m` were not read line by line; queue-state races there are not assessed. The server side of prepare-next/commit-next (`playbackv1/transitions.go`) was not read.
- **Screens were not reviewed.** Whether the web and native apps disable drag-reorder while shuffled, how lyrics search is surfaced, what the mini player and book page show, were not checked beyond grep.
- **Downloads and offline listening** were not reviewed.
- **Closed products.** Plex and Plexamp statements are from public knowledge, not code or current documentation fetched for this audit.
- **Comparison depth varies.** Jellyfin's tag reader, Navidrome's configuration and mappings, Audiobookshelf's scanner and progress model, Finamp's settings and queue structure, and Plezy's music service headers were read; their full behaviour was not traced.
- **Case sensitivity of sidecar reads** (B-10.19) and **the scan's file-visiting order** (B-10.15) were inferred, not traced to the storage helper.

## Outside my domain

- `internal/localmetadata/audio.go:126–138`: the 128 MiB local-artwork limit (B-10.2) applies to movie and TV sidecar art as well; the metadata and artwork auditors should see it.
- `internal/catalog/personal_activity.go:44–48`: the played-threshold rule for non-video items is shared code; any future listening kind inherits it.
- `internal/decoder/analysis.go:127`: every user of `RunAnalysis` is treated as background work, whatever asked for it.
