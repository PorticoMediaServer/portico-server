# 05 — Libraries, scanning and file interpretation

Comparative audit, 11 October 2026. Read-only: nothing in the Portico checkouts was built, run or changed. Everything about Portico below is a **source finding** (what the code says it will do), not observed runtime behaviour, unless a line says otherwise.

Portico paths are relative to `/home/justin/projects/Portico`. To keep citations short, `server/…` means `apps/portico-server/server/internal/…`. Comparison paths are written `jellyfin/…`, `navidrome/…`, `audiobookshelf/…`, `plezy/…`, `jellyfin-docs/…`.

## Scope and sources

**Domain.** Library types and settings; how folders and files are discovered; how file and folder names are read; how Portico decides "this is the same file" and notices changes; real-time monitoring; versions, editions and multi-part files; extras; ignore rules; storage back-ends (local disks, network mounts, rclone, WebDAV). Provider matching and artwork belong to auditor 06 and the database shape to auditor 07; where a finding crosses into those I say so and keep it short.

**Portico, read in full or nearly so**

- Docs: `portico-internal/Project Architecture/03 Catalogue and Metadata.md`, `02 Server.md` (the one row on scanning), `Backlog.md`; `portico-internal/Product Contracts/libraries-and-scanning/contract.md`; `apps/portico-server/server/internal/ingestion/INVENTORY_CONTRACT.md`; `apps/portico-server/server/api/admin-libraries.md`.
- Scanner: `server/ingestion/*` (service, inventory, incremental, analysis, legacy_inventory, remote, recovery, operation_group); `server/worker/worker.go` (the loop that paces it); `server/scanevents/scanevents.go`.
- Storage: `server/storage/client.go`, `inventory_page.go`, `inventory_page_linux.go`, `inventory_page_stream.go`, `inventory_changes_linux.go`, `local_observation_*.go`, `mount_identity_unix.go`, `source_policy.go`; skimmed `server/remotesources/*` and `server/mounts/*` (rclone and WebDAV listing, mount flags).
- File interpretation: `server/catalog/inventory.go`, `inventory_admin.go`, `inventory_maintenance.go`, `inventory_purge.go`, `library_sources.go`, `catalog.go` (library create, `FilenameTitle`, movie commit), `episodic.go`, `episodic_reparse.go`, `episode_repository.go`, `extras.go`, `audio_repository.go`; `server/assets/assets.go` (supported extensions); `server/localmetadata/sidecars.go`, `video.go`, `video_parse.go`, `evidence.go` (sidecars, NFO, OPF); `server/metadata/screen_match.go` and `screen_base.go` (only the part that reads names, years and id hints out of file names).
- Admin: `server/administration/library_settings.go`, `file_operations.go`, `deletion.go` (skim); `server/persistence/migrations/0001_baseline.sql` (library, source, inventory tables and defaults), `0005_library_order.sql`.
- Clients: `apps/portico-server/packages/client-core/src/library-management.ts`, `library-inventory.ts`, `remote-sources.ts`, `storage-management.ts`; `apps/portico-server/web/src/screens/server/Libraries.tsx` (source dialog and defaults).
- Fixtures and tests as a statement of what is covered: `apps/portico-server/fixtures/library-layout/**`, `server/catalog/episodic_test.go`, `server/storage/inventory_page_test.go`.

**Comparison projects, read**

- Jellyfin server: `Emby.Naming/Common/NamingOptions.cs` (whole file), `Emby.Naming/Video/VideoListResolver.cs`, `Emby.Naming/TV/SeasonPathParser.cs`; all of `tests/Jellyfin.Naming.Tests` as a checklist of real file names; `Emby.Server.Implementations/Library/IgnorePatterns.cs`, `CoreResolutionIgnoreRule.cs`, `DotIgnoreIgnoreRule.cs`, `LibraryManager.cs` (item ids), `Resolvers/Movies/MovieResolver.cs`, `Resolvers/TV/SeriesResolver.cs`, `Resolvers/BaseVideoResolver.cs`; `Emby.Server.Implementations/IO/LibraryMonitor.cs`, `FileRefresher.cs`; `MediaBrowser.Controller/Entities/Folder.cs` (`ValidateChildren`), `Video.cs`, `TV/Series.cs`, `TV/Episode.cs` (user-data keys); `Jellyfin.Server.Implementations/Item/ItemPersistenceService.cs` (what happens to watch state when an item is deleted); `MediaBrowser.Providers/MediaInfo/ProbeProvider.cs` (change detection); `MediaBrowser.Model/Configuration/LibraryOptions.cs`, `ServerConfiguration.cs`; `Jellyfin.Data/Enums/CollectionType.cs`.
- Jellyfin docs: `docs/general/server/libraries.md`, `media/excluding-directory.md`, `media/placeholders.md`, `media/_video-multiversion.md`, `_video-multipart.md`, `_video-external-extras.md`, `_video-theme-media.md`, `media/mixed-movies-and-shows.md`.
- Navidrome: `scanner/README.md`, `scanner/walk_dir_tree.go`, `scanner/folder_entry.go`, `scanner/phase_2_missing_tracks.go`, `conf/configuration.go`, `consts/consts.go`.
- Audiobookshelf: `server/Watcher.js`, `server/scanner/LibraryScanner.js` (inode matching).
- Plezy: `lib/widgets/library_management_sheet.dart`, `lib/services/library_events/library_event_socket.dart` (it is a client; it has no scanner of its own).
- Plex: from memory of its public naming guide and settings only (closed source). Marked "Plex (public docs)" where used.

**A note on method for file names.** To find names Portico would misread without running Portico, I wrote a line-for-line Python copy of Portico's five small name readers (`ParseEpisode`, `FilenameTitle`, `classifyExtra`, `screenFilename` and their regular expressions) in my scratchpad and fed it every file name in Jellyfin's naming tests plus about 200 names of my own. Results quoted as "port:" come from that copy. It is a faithful copy of short code, but it is not the Go code; treat each quoted result as "confirmed by reading, illustrated by the port".

**Not covered.** Windows and macOS runtime behaviour; the rclone and WebDAV adapters beyond a skim (`remotesources`, `mounts`, `remotemedia`); DVR private libraries; the deep-analysis stages (trickplay, loudness and so on — auditor 04); the metadata matcher beyond name reading (auditor 06); Kodi and Emby source; Plex beyond public documentation. I did not measure anything.

## How Portico does it today

### Libraries and folders

A **library** has a name, one of five kinds — `movie`, `tv`, `anime`, `music`, `audiobook` (`server/catalog/catalog.go:139`) — and one or more **sources**. A source is one folder: a local path, a network mount the operating system has already mounted, or a "managed remote" (an rclone remote or a WebDAV address that Portico lists itself without mounting). A library may have up to 64 sources (`server/catalog/library_sources.go:314`). Two sources of one library may not sit inside one another, and two libraries may not use the same root folder (`libraries.root` is `UNIQUE`, `0001_baseline.sql:1268`).

Each source carries its own settings (`server/catalog/library_sources.go:44`): display name, path, **classification** (`local` or `network`), **follow symlinks** (default off), **missing grace** (how long a vanished file is kept; default 24 hours, at least 1 hour for network sources), and **rescan interval** (default 0, meaning "only when asked"; the web form offers 15 minutes, 1 hour, 6 hours, daily or only when asked — `apps/portico-server/web/src/screens/server/Libraries.tsx:385`).

Each library also has a **scan policy** (`library_scan_policies`): one of four tiers that promise how much of each file Portico will read. *File List Only* reads directory listings and nothing else; *Basic* (the default) also probes each file with FFprobe and reads small local files beside it (NFO, artwork, subtitles); *Complete* adds full-file work (trickplay, loudness, fingerprints…); *Custom* lets the owner pick operations. A second, overlapping settings document (`server/administration/library_settings.go:150`) holds "allow deleting media" (off by default), trash retention (30 days), Continue Watching rules, the metadata provider, and navigation artefacts.

Libraries have an owner-set order (`libraries.position`, migration 0005), each viewer can pin and arrange libraries in their own navigation rail (`server/catalog/library_pins.go`), and a profile can be restricted to a list of libraries. There is no route to delete a library yet (a comment in `Libraries.tsx:305` says it is waiting on `DELETE /v1/admin/libraries/{id}`), and a library's kind cannot be changed.

### What starts a scan

Three things (`grep` of every caller of the queue functions): the owner pressing Scan (`POST /v1/libraries/{id}/scans`, or per source), a source's own rescan interval coming due (`server/ingestion/inventory.go:393`, with ±5% jitter), and creating a library (its first scan is queued in the same transaction). Changing the scan policy queues analysis only. **There is no file-system watcher**: the only use of inotify in the server is inside a single directory read, to notice a change during that read (`server/storage/inventory_changes_linux.go`). Because a new source's interval defaults to 0 and nothing watches the disk, a freshly created library does not pick up new files until someone presses Scan or sets an interval.

### How a scan runs

A scan is a **durable state machine**, one job per source, stored in SQLite (`jobs`, `inventory_runs`, `inventory_directories`). It moves through phases: `inventory` → `legacy_inventory` (first scans after an upgrade) → `verifying` → `reconciling` → `analysis_enqueue` → `basic_pending`/`basic_running` → `complete` or `complete_with_warnings` (`server/ingestion/service.go:456`).

It works in small steps called **quanta**. One quantum does exactly one bounded thing: list one page of at most 32 directory entries (`server/ingestion/inventory.go:133`), or check whether one known directory is unchanged, or re-check 8 directories, or mark up to 128 files missing, or queue 32 files for analysis, or analyse one file. Each page is committed in one database transaction together with a cursor, so a crash or restart resumes exactly where it stopped. Up to four sources run at once (`service.go:362`).

Every touch of the file system happens in a **separate helper process**: the server re-executes its own binary with `--portico-storage-helper` (`server/storage/client.go:91`, `inventory_page.go:73`). The reason given in the code is that a hung network mount can then be killed without freezing the server or holding a database transaction.

The scan loop is woken by database writes and then paced: it makes at most one pass every 300 ms while anything is in flight, and each pass starts at most one quantum per source (`service.go:331-394`, `server/worker/worker.go:104-170`). See B-05.1 for what that means for speed.

There are four **scan modes** (`server/ingestion/incremental.go:24`): `first-import`; `incremental` (the normal one — a directory whose identity, size, modified time and change time are the same as last time is not listed again, only its known sub-folders are visited); `integrity` (lists everything; chosen automatically when the last full listing is 7 days old); and `full` (owner-requested).

Before a listing is trusted, the `verifying` phase re-reads the identity and timestamps of every directory it listed. If any directory changed during the scan, or the root folder is no longer the same physical folder, the whole job **fails** rather than guessing (`inventory.go:208`, `INVENTORY_CONTRACT.md` "Changed revisions fail the run conservatively"). Files already committed stay in the catalogue. A failed scan raises a notice to the server's administrators (`server/notify/producers.go:145`).

### How Portico decides "this is the same file"

For a local file Portico records the **device number and inode number** the operating system reports (`dev:<n>:ino:<n>`, `server/storage/local_observation_linux.go:16`), plus size, modified time and change time. An inode is the file system's own serial number for a file; it stays the same when a file is renamed or moved within one file system.

There are three layers (`INVENTORY_CONTRACT.md`, `server/catalog/inventory.go:174`):

- a **location** (`inventory_objects`): "source X has something at relative path P";
- an **asset** (`catalog_assets`): one physical file's worth of bytes; and
- a **catalogue entity**: the movie, episode, song or book part people see.

When a path shows a file whose device+inode, size and modified time match exactly one known asset, the asset is reused; if that asset had a single location that has not been seen in this scan, the location is simply moved (a *relocation*). So renaming or moving a file inside one file system keeps everything. If the same path now holds a different inode, the old location is retired and a new asset is created (a *replacement*). Nothing is ever matched by file name similarity, and no content hash is computed in the Basic tier.

The root folder has the same kind of identity. If the root's device+inode differs from the recorded one, scans stop with "source root changed" until the owner accepts the replacement, which starts a new "incarnation" of the source (`inventory.go:48-54`, `library_sources.go:334-346`). This is how Portico tells an unmounted mount point (an empty folder on another disk) from the real thing.

The **entity** a file belongs to is chosen once, when the asset is first seen, from its path and name (`inventory.go:310`, `:353`):

| Kind | Entity key (what makes two files "the same title") | Where |
|---|---|---|
| Movie | the file's path relative to the library root | `catalog.go:511`, `compactcatalog/write.go:85` |
| Show | lower-cased show title + `:` + year | `episode_repository.go:123` |
| Episode | show key + numbering style + season + number | `episode_repository.go:177` |
| Song, audiobook part | the file's relative path | `audio_repository.go:287` |
| Album | folder + album tag + album artist + year + edition tags | `audio_repository.go:414` |
| Audiobook | folder + album tag + author + narrator + ISBN/ASIN/edition | `audio_repository.go:505` |

A ledger (`catalog_identities`) remembers the public id ever given to a key, so a title that disappears and comes back at the same path gets the same id.

### Missing files

A file that is not seen is not removed. It becomes a "missing candidate"; only after **two completed, trusted scans** have failed to see it **and** the grace period has passed is it marked `missing` and hidden as unavailable (`inventory.go:310`). Timeouts, permission errors, an offline source or a changed root never count as "not seen". The owner can then move missing rows to a catalogue-only trash and "forget" them (`inventory_admin.go:218`); neither touches files on disk. A separate, off-by-default feature lets the owner delete media files from the UI through a three-step preview/confirm/delete, moving the bytes into `<state>/trash/` for the retention period (`api/admin-libraries.md`, `administration/deletion.go:301`).

### Reading names

Portico's name readers are deliberately small and strict.

- **Movies** (`catalog.go:354-365`): the title is the file name up to the first four-digit year (1900–2099) that follows a space, dot, underscore, `[` or `(`; dots and underscores become spaces. The folder name is not used. One file is one movie.
- **TV and anime** (`episodic.go:37`): the file name must contain exactly one `S01E02`-style or `1x02`-style marker, or a `2024-03-05`-style date, or (anime libraries only) ` - 12` after the title. The show title is the text before the marker. If the file sits in a folder called `Season N` or `Specials`, the folder above it is treated as the show folder and must agree with the title in the file name; otherwise the folder is ignored. Anything the reader is not sure about is **not guessed**: the file is recorded with an issue code (for example `folder_and_filename_series_conflict`) and listed for the owner to assign by hand (`GET /v1/libraries/{id}/episode-issues`, `POST …/episode-assignments`).
- **Music and audiobooks** (`audio_repository.go:221`): embedded tags decide title, album, artist, track and disc. Without tags the album is the folder name, the artist is "Unknown artist", and a leading number in the file name is the track number. A parent folder named `CD1`, `Disc 2` and so on is treated as a disc of the album above it.
- **Extras** (`extras.go`): a file under a folder named `Extras`, `Featurettes`, `Trailers`, `Deleted Scenes`, `Behind The Scenes`, `Interviews`, `Scenes`, `Shorts`, `Other`, `Bonus`… (28 names) anywhere in its path, or whose name ends in `-trailer`, `-featurette`, `-deleted`, `-behindthescenes`, `-interview`, `-scene`, `-short` or `-other`, becomes an extra attached to the folder that owns it.
- **Supported files** (`assets/assets.go:259-296`): video libraries accept `.mp4 .m4v .mov .mkv .webm .avi .ts .m2ts .mts .vob .mpg .mpeg .strm` (and list `.iso`/`.img` as "unsupported disc image"); music and audiobook libraries accept `.mp3 .m4a .m4b .flac .ogg .opus .wav .aac .aiff`. There is no other filter: no hidden-file rule, no ignore file, no sample rule.

### Progress and errors

Viewers get a path-free status per library (`GET /v1/libraries/{id}/inventory-status`: health, phase, files discovered, analysed, warnings) and a `library.scan.updated` event at most every 10 seconds (`scanevents.go:18`). Owners also get a pageable inventory of every location with its state, and a numbered change feed per source (`inventory_changes`: created, relocated, replaced, missing, analysis completed…). A scan that stops is reported with a generic reason code, not the path that caused it.

### Remote storage

For rclone, Portico runs the owner's own rclone binary as `rclone lsjson <remote> --max-depth=1` one directory at a time (`server/mounts/native.go:115`) and stores each listing privately before paging it into the catalogue; reads use `rclone cat --offset`. For WebDAV it issues `PROPFIND Depth: 1` itself and can use sync tokens for deltas. An optional read-only rclone mount exists for compatibility but is never used as scan truth. `.strm` files (a text file holding a URL) are catalogued by name only; the URL is read at playback time and never stored.

## How the others do it

### Jellyfin

**Shape.** A scan is an in-process, recursive walk. For each folder it lists the children, runs them through a chain of *resolvers* (movie, series, season, episode, audio, album, artist, book, photo, playlist…) that turn paths into items, compares the result with the children it had before, and immediately creates new items and deletes vanished ones (`jellyfin/MediaBrowser.Controller/Entities/Folder.cs:417-620`). It walks everything every time; what it skips is re-probing a file whose modified time has not changed (`jellyfin/MediaBrowser.Providers/MediaInfo/ProbeProvider.cs:127`).

**Triggers.** A scheduled task every 12 hours by default (`RefreshMediaLibraryTask.cs:52`); an optional per-library **real-time monitor** that puts an OS watcher on every library folder, collects events, waits 60 seconds of quiet (`LibraryMonitorDelay`, `ServerConfiguration.cs:172`) and refreshes just the affected item or its nearest surviving parent (`LibraryMonitor.cs`, `FileRefresher.cs`); and an HTTP call other tools use to say "these paths changed" (`POST /Library/Media/Updated`, `Jellyfin.Api/Controllers/LibraryController.cs:648`), which is what Sonarr and Radarr call.

**Identity.** An item's id is the MD5 hash of its type name plus its path (`LibraryManager.cs:797-819`). Move or rename a file and it is a brand-new item. Watch state survives anyway because it is stored under a *user-data key* derived from provider ids — TMDB/IMDb id for a movie, the series' key plus season and episode number for an episode (`Video.cs:293`, `Episode.cs:162`) — and when an item is deleted its user data is parked on a placeholder with a retention date and re-attached when an item with the same key appears (`ItemPersistenceService.cs:94-132`, `:236`).

**Offline protection.** Only at the top: a library folder that is "inaccessible or empty" is skipped (`Folder.cs:407-411`), and an I/O error while listing a folder leaves that folder's items alone (`Folder.cs:434-453`). Below that, a missing file is deleted from the library at once.

**Naming.** `NamingOptions.cs` is a long list of regular expressions collected from real libraries: 27 episode patterns tried in order (Kodi's standard ones, `S01E02`, `1x02`, `Season 3 Episode 9`, `Episode 16`, bare `02 - title`, anime `[Group] Name - 12`, `[Name][21]`, dates in two orders, parts), ten multi-episode patterns, clean-up patterns that strip release tags, stacking rules for multi-part files, 3D flags, stub types, 15 extras folder names, 18 extras suffixes and 3 bare file names, and 53 video and 81 audio extensions. Series and movie names normally come from the **folder**, provider id hints (`[tmdbid-…]`, `{tmdb-…}`, `[imdbid-…]`, `[tvdbid-…]` and more) are read from folder or file name, and several files in one movie folder whose names start with the folder name become **versions** of one movie.

**Ignoring.** A built-in list of 74 glob patterns (samples, Synology/QNAP system folders, `lost+found`, `.snapshots`, every dot-file) in `IgnorePatterns.cs`, plus `.ignore` files in gitignore format (`DotIgnoreIgnoreRule.cs`, `jellyfin-docs/…/excluding-directory.md`).

**Library types.** Movies, shows, music, music videos, home videos and photos, books, mixed movies-and-shows, box sets, playlists (`CollectionType.cs`). 42 per-library options (`LibraryOptions.cs`).

### Plezy

Plezy is a client for Plex, Jellyfin and Emby and has no scanner. What it shows in this domain is how a client exposes library management: a sheet where each person reorders and hides libraries locally, admin actions per library (refresh metadata on every back-end; scan, analyse and empty trash on Plex only), and a socket that listens for the server's library-change events and throttles them to at most one refresh per window because one Plex scan was measured sending 156 frames (`plezy/lib/widgets/library_management_sheet.dart:136-176`, `plezy/lib/services/library_events/library_event_socket.dart:180`).

### Navidrome (Go + SQLite, music only)

Four phases: walk folders, match missing tracks, refresh albums, import playlists (`navidrome/scanner/README.md`). It decides whether a folder needs re-reading from a hash of the folder's modified time **and every audio file's name, size and modified time** (`scanner/folder_entry.go:106-130`), so an in-place overwrite is seen without relying on the folder's own timestamp. Tracks and albums have **persistent ids** computed from tags (`musicbrainz_trackid|albumid,discnumber,tracknumber,title`, `consts/consts.go:157-158`); phase 2 uses them to recognise a moved or renamed file and carry its play counts and ratings over. It follows symlinks by default, skips unreadable folders with a warning (`walk_dir_tree.go:301-306`), ignores dot-files and a list of NAS system folders (`:315-324`), honours `.ndignore`, has a watcher with a 5-second settle time, scans on start-up, and by default never purges missing tracks (`conf/configuration.go:1062-1073`).

### Audiobookshelf

A watcher that waits for a new file's modified time to stop changing (polled every 3 seconds, up to 10 minutes) before scanning it (`audiobookshelf/server/Watcher.js:239-257`), and scans that fall back to **inode matching** when a library item's path is gone, to recognise renamed or moved folders and files (`server/scanner/LibraryScanner.js:175-187`, `:642-700`). Books are folders (author/series/title inferred from the folder path), single-file or multi-file, with podcasts as a second library type.

### Plex (public docs)

Folder-per-movie or loose files; `{tmdb-…}`, `{imdb-…}`, `{tvdb-…}` hints and `{edition-…}`; versions by matching titles; `- pt1` stacking; local extras folders and suffixes; `.plexignore`; partial scan on change with "run a partial scan when changes are detected", periodic scans, and a per-folder scan URL other tools call; missing files are shown as unavailable and removed when "empty trash automatically after every scan" is on.

## The naming grammar, side by side

"Yes" means the file becomes the right title without help. Portico results marked *port* were produced by my Python copy of the readers (see Scope). Jellyfin results come from `NamingOptions.cs`, its naming tests and its docs.

### Movies

| Pattern | Portico | Jellyfin | Notes |
|---|---|---|---|
| `Title (2009)/Title (2009).mkv` | Yes (title and year read from the **file** name) | Yes (name from the **folder**) | |
| Loose `Title (2009).mkv` in the library root | Yes | Yes | |
| `Title (2009)/Title.mkv` | Title "Title", **no year** (folder ignored) | Yes | port |
| `Title (2009)/movie.mkv` | Title **"movie"** | Yes (single video in a folder takes the folder's name, `MovieResolver.cs:488-491`) | port |
| Year as `(2009)`, `[2009]`, `.2009.`, ` 2009`, `Title(2009)` | Yes | Yes | |
| A year inside the title: `Blade Runner 2049 (2017)`, `Wonder Woman 1984 (2020)`, `Death Race 2000 (1975)` | **Wrong**: title cut at the first year → "Blade Runner", year 2049 | Yes (takes the last year; tests `300 2001 (2006)`) | port; B-05.7 |
| No year, release tags: `Title.1080p.BluRay.x264-GRP.mkv` | Catalogue title "Title 1080p BluRay x264-GRP"; the matcher strips a short tag list for its search | Yes (long clean-up list) | port |
| Dots in real titles: `E.T. the Extra-Terrestrial (1982)` | "E T  the Extra-Terrestrial" (double space) until a provider match replaces it | "E.T. the Extra-Terrestrial" | port; B-05.17 |
| Id hint `[tmdbid-19995]` / `{tmdb-19995}` | Only when it is in the **file** name; folder hint ignored | Folder or file | `screen_base.go:221` |
| Id hint `[imdbid-tt0499549]` / `{imdb-tt0499549}` | **Not recognised** (only tmdb, tvdb, anilist with numbers, `screen_match.go:21`) | Yes | Plex: yes |
| `{edition-Director's Cut}` | Seen by a regex, value thrown away (`screen_match.go:105`); if it comes before the year it stays in the title | Not a concept (use a version label) | Plex: yes |
| Versions: `Title (2009) - 1080p.mkv` + `Title (2009) - 4K.mkv` | **Two movies** | One movie with versions (prefix must equal the folder name) | Backlog; B-05.8 |
| Multi-part: `Title-cd1.mkv`, `Title-cd2.mkv` | Two movies | One stacked movie (`cd`, `dvd`, `part`, `pt`, `disc`, `disk` + number or a–d) | |
| 3D flags (`.3D.HSBS.`), `.disc` placeholders | No | Yes | |
| Blu-ray/DVD folders (`BDMV/STREAM/*.m2ts`, `VIDEO_TS/*.VOB`) | **Every stream file becomes a movie** ("00001", "VTS 01 1") | One item per disc folder | port; B-05.15 |
| `.iso` | Listed as "unsupported disc image" | Item | |
| `.strm` | Yes; content never read during scans | Yes | Portico stricter on privacy |

### TV and anime

| Pattern | Portico | Jellyfin | Notes |
|---|---|---|---|
| `Show (2010)/Season 01/Show - S01E02 - Title.mkv` | Yes | Yes | |
| `Show (2010)/Season 01/S01E02.mkv` (no show name in file) | Yes (show from folder) | Yes | |
| `Show (2010)/S01E02.mkv` (no season folder) | **Rejected** `series_identity_missing` | Yes | port; B-05.10 |
| `Show (2010)/Show - S01E02.mkv` (no season folder) | Parsed, but the folder and its **year are ignored** → show "Show" with no year | Yes | port; B-05.10 |
| Scene name in a tidy folder: `The Office (US)/Season 1/The.Office.US.S01E01.mkv`; `Mr. Robot/Season 1/Mr.Robot.S01E01.mkv` | **Rejected** `folder_and_filename_series_conflict` | Yes (folder wins) | port; B-05.11 |
| `1x02`, `S1E2`, `s01e02` | Yes | Yes | |
| `S01 E02`, `s01.e02`, `S01xE02`, `Season 3 Episode 9`, `Episode 16`, `02 - Title.mkv`, `02.mkv` | No (`unrecognized_episode_name`) | Yes | port |
| Multi-episode `S01E01E02`, `S01E01-E02`, `S01E01-02`, `1x01x02` | Yes; the file is linked to each episode (up to 8), boundary marked unknown | Yes (one item with a start and end number) | |
| `S01E23-E24-E26`, `1x01-1x02`, `02x03 - 02x04` | Rejected (`ambiguous_episode_range` / `conflicting_episode_markers`) | Yes | port |
| `1x01-02` | Episode 1 only (the `-02` is silently dropped) | Episodes 1–2 | port |
| Specials: `Season 00/…S00E01`, `Specials/…S00E01` | Yes | Yes | |
| `Specials/The Christmas Invasion.mkv` | No | No number either, but item is kept | |
| Season folder `Season 1`, `Season01`, `season_1` | Yes | Yes | |
| Season folder `S01`, `Series 1`, `Staffel 1`, `Saison 1`, `Temporada 1`, `Season 1 (2022)`, `Season 01 - Name` | Not recognised as a season folder; the file still parses if it names the show, but the show folder's year is lost | Yes (about 20 languages, `SeasonPathParser.cs`) | port |
| Daily shows `Show 2024-03-05`, `2024.03.05`, `2024_03_05` | Yes → season = year, episode = day of year | Yes (matched by air date) | |
| Daily show in a numbered season folder: `Jeopardy/Season 40/Jeopardy - 2024-01-15.mkv` | **Rejected** `folder_and_filename_season_conflict` | Yes | port; B-05.13 |
| `Show (2024-03-05)`, `Show 05.03.2024` | No | Second form yes | port |
| Absolute `Show - 12.mkv`, `[Group] Show - 12 [1080p].mkv`, `- 12v2` | **Anime libraries only** | Any show library | |
| `Show 12`, `Show E12`, `[Group][Show][12]`, `[Group]_Show_-_12_`, `- 12.5` | No | First four yes | port |
| Fansub resolution tag: `[Group] Show - 05 [1920x1080].mkv` | **Misread** as season 1920 episode 1080 of a show called "Show - 05 [" (anime), or rejected as two markers (TV) | Yes | port; B-05.12 |
| Episode title ending "Part 1": `… - S01E19 - The Siege of the North, Part 1.mkv` | Rejected `multipart_episode_requires_assignment` | Yes | port; B-05.13 |
| Episode 0: `S01E00` | Rejected (`episode_number_out_of_range`) | Yes | port |
| Four-digit seasons `S2017E368` | Yes | Yes | |

### Extras, samples, hidden files

| Pattern | Portico | Jellyfin | Notes |
|---|---|---|---|
| Extras folders | 28 names, matched on **any** folder in the path | 15 names, only directly inside a movie, show or season folder | B-05.14 |
| Extras suffixes | 8: `-trailer -featurette -deleted -behindthescenes -interview -scene -short -other` | 18, incl. `.trailer`, `_trailer`, `-clip`, `-deletedscene`, `-extra`, `-sample` | |
| Bare `trailer.mkv`, `sample.mkv`, `Sample/` folder, `movie-sample.mkv` | **Become movies** called "trailer", "sample" | Trailer / ignored | port; B-05.16 |
| Theme music and video: `theme.mp3`, `theme-music/`, `backdrops/` | No | Yes | |
| Dot-files, macOS `._Name.mkv`, `@eaDir`, `#recycle`, `.Trash-1000`, `$RECYCLE.BIN`, `lost+found` | **Scanned like anything else** | Ignored | B-05.6, B-05.4 |
| `.ignore` / `.plexignore` / `.ndignore` | None | `.ignore` (gitignore syntax) | |

### Music and audiobooks

| Pattern | Portico | Jellyfin | Navidrome / Audiobookshelf |
|---|---|---|---|
| Tagged files anywhere | Yes, tags decide | Yes | Yes |
| Untagged `Artist/Album/01 Track.flac` | Album = folder name, artist = "Unknown artist", track = leading number | Artist and album from folders | Navidrome: "[Unknown Artist]" |
| Disc folders | `CD1`, `Disc 2`, `Disk-3` exactly | `cd/disc/disk/vol/volume/part/act` + number | Navidrome: disc tag only |
| `Disc 1 - Early Years/` | Not a disc folder → a **separate album** even with identical album tags (folder is part of the album key) | Not a disc folder either | Navidrome: one album (tags) |
| One `.m4b` with chapters | Yes, chapters read from the file | Limited | ABS: yes |
| Many files in a book folder | Parts ordered by track tag or leading number | Chapter/part patterns in names (`ch 01`, `part 02`…) | ABS: yes |
| `Author/Series/Book/…` folders without tags | Book title = folder; no author or series | — | ABS: author, series, title from folders |
| Extensions | 9 audio | 81 audio | |

## Architectural differences

### A-05.1 A durable, paged state machine instead of an in-process walk
- **Portico:** a scan is a row in the database that advances one bounded step at a time, with a cursor committed alongside each page (`server/ingestion/service.go:409-475`, `INVENTORY_CONTRACT.md`). It survives restarts, can be paused, resumed and cancelled, and never holds a database write open while touching the disk.
- **Jellyfin / Navidrome:** an in-memory recursive walk; a restart loses the scan and it starts again. Navidrome keeps a per-folder record so a restarted scan is cheap.
- **Why it matters:** Portico's design is what makes "inventory is visible while analysis continues", restart-safety and a blocked mount not blocking others possible. Its cost is bookkeeping per step (several small transactions and a process start per page).
- **Assessment:** **Portico ahead** on robustness by design; deliberate (`contract.md` §3). The cost side is the subject of A-05.3 and B-05.1.

### A-05.2 Every file-system call runs in a child process
- **Portico:** listing a page, stat-ing a file, reading a sidecar and probing all start a fresh helper process (`server/storage/client.go:91`, `:390`, `inventory_page.go:73`).
- **Jellyfin / Navidrome / Audiobookshelf:** direct system calls in the server process.
- **Why it matters:** a hung NFS or SMB mount cannot freeze the Portico server, and the helper can be killed. But a process start costs milliseconds where a system call costs microseconds, and a directory with no sidecars costs dozens of starts per file (B-05.22).
- **Assessment:** **Trade-off (subjective).** Deliberate. A long-lived helper per source that answers many requests would keep the isolation and remove most of the cost (medium: `storage/client.go`, the helper protocol; the non-Linux path already keeps one helper alive per directory, `inventory_page_stream.go`).

### A-05.3 Scan speed is set by a clock, not by the disk
- **Portico:** the scan loop makes one pass per 300 ms and starts one quantum per source per pass (`service.go:389`, `worker.go:104-170`). One quantum is one page of ≤32 entries, or one directory check, or one file analysed.
- **Jellyfin:** as fast as the disk and CPU allow, with configurable parallelism (`LibraryScanFanoutConcurrency`). **Navidrome:** a pipeline with parallel stages.
- **Why it matters:** see B-05.1 — by reading, the ceiling is about 3 directories per second per source however fast the storage is.
- **Assessment:** **Portico behind**, and I think accidental: the 300 ms was meant as "come back soon", but the shared-signal floor in `worker.go` turns it into a fixed cadence. Tests drive the scanner through a different function (`service.go:400`, used by `incremental_test.go:53`) that has no such pacing, so they would not show it.

### A-05.4 Change detection: directory fingerprints with a weekly safety net
- **Portico:** in incremental mode a directory is re-listed only if its own identity, size, modified time or change time differs (`incremental.go:47-80`). Files inside an unchanged directory are not looked at. Once a week a scan lists everything (`incremental.go:38`).
- **Jellyfin:** lists every directory and stats every file on every scan; re-probes when a file's modified time changed. **Navidrome:** lists every directory and hashes each file's name, size and modified time into the folder's fingerprint (`folder_entry.go:106`).
- **Why it matters:** Portico's way does the least disk work on a quiet library, which is right for cloud remotes and sleeping disks. But a directory's timestamp does not change when a file inside it is overwritten, appended to, or (on some file systems) even added — B-05.3 and B-05.19.
- **Assessment:** **Trade-off (subjective)**; deliberate (`api/admin-libraries.md`, last section). I would keep the directory skip for remote sources and add a cheap per-file size+mtime check for local ones (small: `skipUnchangedDirectory` would list names and stats instead of skipping outright).

### A-05.5 What triggers a scan
- **Portico:** manual, per-source interval (default "only when asked"), library creation. No watcher, no "these paths changed" call, no scan at start-up.
- **Jellyfin:** 12-hourly by default, optional real-time monitor with a 60 s settle, `POST /Library/Media/Updated`. **Navidrome:** watcher (5 s settle), scan on start-up, optional cron. **Audiobookshelf:** watcher by default. **Plex:** watcher, periodic, per-path scan URL.
- **Why it matters:** with defaults, a file dropped into a Portico library never appears on its own. The owner must learn to set an interval per folder. Sonarr/Radarr-style tools have nothing to call.
- **Assessment:** **Portico behind.** The contract already says "Native change notifications are acceleration hints only" (§4.1), so watchers are allowed, just not built. See F-05.1 to F-05.3.

### A-05.6 A scan must see a still tree, or it fails
- **Portico:** if any directory changes between being listed and the end-of-scan verification, or while it is being paged, the whole job fails and analysis for that job never starts (`inventory.go:166-208`, `service.go:472`).
- **Jellyfin / Navidrome:** best effort; what changed mid-scan is picked up next time.
- **Why it matters:** this is what lets Portico say a completed scan is *authoritative* (safe to use as evidence that a file is gone). But the stricter rule is applied to the whole job, not just to the deletion decision. See B-05.2.
- **Assessment:** **Trade-off**, deliberate (`INVENTORY_CONTRACT.md`), but I think the blast radius is wider than intended: a changed directory need only make the run *non-authoritative* (no absence decisions) while everything else carries on.

### A-05.7 File identity: device + inode + size + time
- **Portico:** `dev:ino` plus size and modified time identify an asset (`inventory.go:207`); path is only a locator.
- **Jellyfin:** identity is the path (MD5 of type + path). **Audiobookshelf:** path first, inode as a fallback for moves. **Navidrome:** path, with a tag-derived persistent id used to re-link moved tracks. **Plex (public docs):** path plus a partial content hash for matching.
- **Why it matters:** Portico recognises a rename or move within one file system exactly, with no reading of file content, and never confuses two different files. It does not recognise a copy, a move across file systems, or a re-download. And device numbers are not stable everywhere (B-05.18).
- **Assessment:** **Portico ahead** for same-disk renames, **behind** for cross-disk moves and replacements. Deliberate (`contract.md` §1.3). A content fingerprint (first/last megabyte + size, as Plex and OpenSubtitles do) or tag ids for music would cover the gap (medium).

### A-05.8 What a "title" is keyed on
- **Portico:** movies, songs and book parts are keyed on their **relative path**; shows on **title + year**; episodes on **show + season + number** (table in "How Portico does it").
- **Jellyfin:** every item is keyed on its path; grouping across folders happens later through provider ids (`PresentationUniqueKey`, "automatic series grouping", versions by name).
- **Why it matters:** in Portico a second file for the same movie is a second movie (Backlog), while a second file for the same episode number attaches to the same episode. A show split across two folders or two sources merges automatically if the names and years agree, and splits if they do not (B-05.27).
- **Assessment:** **Trade-off.** Episode keying is better than Jellyfin's. Movie keying by path is the root of the Backlog item "a second file of the same title becomes a second title".

### A-05.9 What survives a move, rename, replace or re-download
| Event | Portico | Jellyfin |
|---|---|---|
| Rename or move inside one file system | Same asset, same entity, everything kept. The old name's interpretation is kept too (B-05.9) | New item; watch state re-attaches by provider id |
| Overwrite at the same path (new inode) | New asset on the same entity; watch state kept; analysis redone | Same item, re-probed |
| Replace with a differently named file (Radarr upgrade `…WEBDL-720p.mkv` → `…Bluray-1080p.mkv`) | **New movie**; the old one lingers until grace ends; watch state, favourites, collections membership and manual match stay on the old one | New item; watch state re-attaches by TMDB/IMDb id |
| Move to another disk or library | New movie/song (new path, new inode); episodes re-attach by number | New item; re-attaches by provider id |
| Delete, re-add months later at the same path | Same public id (ledger) | New item; user data re-attached if still retained |
- **Assessment:** **Portico behind** for the common "upgrade replaces the file under a new name" case for movies and music; **ahead** for exact renames. See F-05.17.

### A-05.10 Protecting the library when a drive disappears
- **Portico:** the root folder's device+inode is recorded; a different one stops scanning until the owner accepts it (`inventory.go:48-54`). Absence needs two completed scans plus a grace period. Errors, timeouts and offline sources never count as absence.
- **Jellyfin:** a top-level library folder that is empty or unreadable is skipped (`Folder.cs:407`); an I/O error while listing keeps existing items. Otherwise deletion is immediate. **Navidrome:** marks tracks missing and by default never purges.
- **Why it matters:** Portico cannot wipe a library because a mount vanished, even one level down (Jellyfin can, if a sub-folder is itself a mount point). The price is that genuinely deleted files stay "available" for a day (A-05.11) and that some harmless events look like a replaced root (B-05.18).
- **Assessment:** **Portico ahead** on safety; deliberate (`contract.md` §10).

### A-05.11 What happens to a file that is really gone
- **Portico:** stays visible and marked available until two scans and the grace period (24 h local, ≥1 h network) have passed, then becomes unavailable; it is never removed from the catalogue unless the owner trashes and forgets it (`inventory.go:310`, `inventory_admin.go:218`). A maintenance task can move missing rows to the catalogue trash but does not forget them (`inventory_maintenance.go`).
- **Jellyfin:** removed at the next scan. **Plex:** shown with a trash mark, removed by "empty trash". **Navidrome:** flagged missing; purge policy `never`/`always`/`full`.
- **Why it matters:** after an owner deletes a film on disk, Portico goes on offering it for at least a day and the Play button fails. With "only when asked" scans it stays until two manual scans a day apart.
- **Assessment:** **Trade-off (subjective).** Deliberate. An obvious refinement: when the file's *parent directory was listed successfully and the file is not in it*, that is strong evidence on a local disk; the grace could apply only to whole-directory or whole-source disappearance (small: `reconcilePage`).

### A-05.12 Library types
- **Portico:** movie, tv, anime, music, audiobook. Anime is a first-class kind with its own naming rule (absolute numbering).
- **Jellyfin:** movies, shows, music, music videos, home videos & photos, books, mixed, plus box sets and playlists as views. **Audiobookshelf:** books, podcasts. **Plex:** movies, shows, music, photos, other videos.
- **Why it matters:** home videos, concerts, YouTube archives, photos, e-books, comics and podcasts have no home in Portico. The contract allows a mixed library "only as an explicitly supported generic-file experience" (§1.1); none exists.
- **Assessment:** **Portico behind** in breadth; a separate anime kind is **ahead** of Jellyfin, which has no such thing. See F-05.15.

### A-05.13 Libraries and folders
- **Portico:** up to 64 folders per library, each with its own schedule, grace, symlink rule and health; folders in one library may not nest; a folder may be the root of only one library.
- **Jellyfin:** many paths per library, no per-path settings; the same path may be in several libraries.
- **Why it matters:** per-folder health and schedule is finer than anything Jellyfin has. The uniqueness rule blocks setups like a "Kids" library and a "Movies" library over the same folder with different visibility.
- **Assessment:** **Portico ahead** on per-source control; the uniqueness rule is a **trade-off** (it is a database constraint on a legacy column, `libraries.root UNIQUE`, so possibly accidental).

### A-05.14 Movie titles come from the file name, never the folder
- **Portico:** `FilenameTitle` reads only the base name (`catalog.go:356`); the matcher also searches with base names only (`screen_base.go:221`).
- **Jellyfin / Plex / Kodi:** the folder name is the primary source for a movie in its own folder.
- **Why it matters:** `Avatar (2009)/Avatar.mkv` loses its year; `Avatar (2009)/movie.mkv` is "movie"; an id hint on the folder (where both the Plex and Jellyfin guides put it) is not seen.
- **Assessment:** **Portico behind.** Small change: when a file is the only video in its folder (or its name starts with the folder name), prefer the folder name and read hints from both.

### A-05.15 A strict episode reader that refuses, versus a permissive one that guesses
- **Portico:** exactly one marker allowed; any doubt becomes an issue code and a row in a to-do list for the owner (`episodic.go`, 11 issue codes). Nothing is guessed.
- **Jellyfin:** 27 patterns tried in order, several marked "optimistic"; the first that matches wins. Wrong guesses happen and are fixed by renaming or "Identify".
- **Why it matters:** Portico never files an episode in the wrong place silently — except when its own single pattern misfires (B-05.12). But on a real, messy library a large share of files end up in the to-do list: of the distinct names in Jellyfin's TV naming tests, my port accepted 57 and rejected 117 (some of those rejections are artefacts of the folder names I had to invent; the breakdown is in B-05.10 to B-05.13).
- **Assessment:** **Trade-off (subjective).** The "never guess, show a list" idea is good and worth keeping; the grammar underneath it is too narrow for release. The folder-must-agree rule in particular rejects more correct files than wrong ones (B-05.11).

### A-05.16 Show titles come from the file name, and the folder only checks them
- **Portico:** title = text before the marker; the show folder is consulted only if the file is inside a `Season N`/`Specials` folder, and then it must match exactly (case-insensitively) or the file is rejected (`episodic.go:44-50`, `:133-142`).
- **Jellyfin / Plex / Sonarr:** the show folder names the show; the file name supplies only season and episode.
- **Why it matters:** two real-world habits break: tidy folders holding untouched scene releases, and flat show folders with `S01E02 - Title.mkv` files.
- **Assessment:** **Portico behind.** Making the nearest non-season ancestor folder the authority (as everyone else does) and using the file-name title only when there is no show folder would fix B-05.10 and B-05.11 together (small to medium: `ParseEpisode` only).

### A-05.17 Absolute numbering depends on the library kind
- **Portico:** ` - 12` is read only in `anime` libraries (`episodic.go:117`).
- **Jellyfin:** absolute numbers are accepted in any show library.
- **Why it matters:** an anime show kept in a TV library, or a long-running western show numbered absolutely, is rejected. In return, a TV library never mistakes "Show - 24" for episode 24.
- **Assessment:** **Trade-off**; deliberate (anime is a kind). Reasonable.

### A-05.18 Daily shows become "season = year, episode = day of the year"
- **Portico:** a dated file is stored with numbering style `date`, season = the year, number = 1–366 (`episodic.go:111`).
- **Jellyfin:** stores the air date and asks the provider which season and episode aired that day.
- **Why it matters:** Portico's episode key is stable and needs no provider. But providers number daily shows by season (TheTVDB "Season 29"), and Sonarr puts dated files in those numbered season folders, which Portico then rejects because 29 ≠ 2024 (B-05.13).
- **Assessment:** **Trade-off**; the folder check should not apply to dated files.

### A-05.19 Season folders are English only
- **Portico:** `^season[ ._-]*N$` or `specials` (`episodic.go:21`, `:47`).
- **Jellyfin:** about 20 languages, `S01`, `Season 1 (2016)`, `Show Season 1`, numeric folders.
- **Why it matters:** Portico is being localised (the `i18n` package); owners with `Staffel 1` or `Saison 1` folders lose folder-derived show names and years.
- **Assessment:** **Portico behind**; small.

### A-05.20 Multi-episode files
- **Portico:** the file is linked to each episode it names (2–8), each link marked "boundary unknown" (`episode_repository.go:207-212`); every episode is its own catalogue entry that happens to play the same file.
- **Jellyfin:** one item with `IndexNumber` and `IndexNumberEnd`.
- **Why it matters:** Portico's model lets each episode have its own watch state and metadata, and leaves room to record where one episode ends inside the file. Jellyfin's is simpler but the second episode has no page of its own.
- **Assessment:** **Portico ahead** in model. Worth checking with auditor 03/04 what "watched" means when two episodes share one file.

### A-05.21 Versions and editions
- **Portico:** no automatic grouping of files into versions of one movie; the owner can attach a file to an existing title by hand (`AssociateInventoryVersion`, `inventory_admin.go:311`). Episodes group by number automatically (two files for S01E01 are two versions of one episode). Editions are read only from an NFO `<edition>` tag.
- **Jellyfin:** versions by file-name prefix for movies and by episode number for episodes, with a label after ` - `, sorted with resolutions first. **Plex:** `{edition-…}` plus automatic version merging of the same matched title.
- **Why it matters:** owners who keep a 4K and a 1080p copy see two posters. Already in `Backlog.md`.
- **Assessment:** **Portico behind.** The data model already supports several assets on one entity; what is missing is the rule that decides it (see F-05.5).

### A-05.22 Multi-part files
- **Portico:** movies: `…-cd1.mkv` and `…-cd2.mkv` are two movies with the same title. Episodes: a trailing `part/pt/cd/disc/disk N` makes the file an issue for manual assignment (`episodic.go:31`, `:61`).
- **Jellyfin:** stacks parts into one item played back to back.
- **Why it matters:** rare in new libraries, common in old DVD-era ones. Portico's episode rule also catches real titles that end in "Part 1" (B-05.13).
- **Assessment:** **Portico behind**, low priority. The asset link already has a `part_index`.

### A-05.23 Extras
- **Portico:** an extra is an ordinary playable entity of kind `extra` plus a row naming the **folder** that owns it (`item_extras.parent_path`); a title shows the extras whose owning folder equals the folder of any of its files (`extras.go:199-259`). Eight types.
- **Jellyfin:** extras are owned items of a specific movie, series or season (`OwnerId`), a dozen types including sample, clip, theme song and theme video.
- **Why it matters:** Portico's folder join survives re-matching and needs no parent lookup at scan time. But it breaks for loose movies in one shared folder (every movie in the folder would show every `-trailer` file in it), and matching folder names anywhere in the path over-reaches (B-05.14).
- **Assessment:** **Trade-off.** Deliberate ("Sidecar extras are recognised from the owning media folder", `extras.go:12`).

### A-05.24 Ignore rules
- **Portico:** none. The only filter is the file extension.
- **Jellyfin:** 74 built-in patterns and `.ignore` files. **Navidrome:** dot-files, eight NAS system folders, `.ndignore`. **Plex:** `.plexignore`.
- **Why it matters:** NAS recycle bins, Synology `@eaDir` thumbnails, macOS `._` resource forks, samples and `lost+found` all enter the catalogue or break the scan (B-05.4, B-05.6, B-05.16). An owner has no way to say "skip this folder".
- **Assessment:** **Portico behind.** Small to add (a name filter in `CommitInventoryPage`), and it must be added carefully: an ignored file that was previously catalogued should go through the normal missing path.

### A-05.25 Which files count as media
- **Portico:** 13 video and 9 audio extensions, chosen per library kind; an audio file in a movie library is not media and vice versa (`assets.go:285`).
- **Jellyfin:** 53 video, 81 audio.
- **Why it matters:** `.wmv`, `.flv`, `.3gp`, `.ogv`, `.divx`, `.rmvb`, `.mk3d`, `.m2v`, `.wtv`, `.dvr-ms`, `.asf` video and `.wma`, `.ape`, `.wv`, `.dsf`, `.mka`, `.oga`, `.aif`, `.mpc`, `.ac3`, `.dts`, `.aa/.aax` audio are silently invisible. FFmpeg can read almost all of them.
- **Assessment:** **Portico behind** on breadth; possibly deliberate caution (only formats the playback path has been checked against). An "unsupported" row in the inventory, as already done for `.iso`, would at least make them visible.

### A-05.26 Disc folders and images
- **Portico:** no notion of a disc folder. `.m2ts` and `.vob` are "ordinary extracted media files" (`INVENTORY_CONTRACT.md`), so a ripped disc becomes one title per stream file. `.iso`/`.img` are listed as unsupported.
- **Jellyfin:** `BDMV` and `VIDEO_TS` folders and ISO files resolve to one item.
- **Why it matters:** B-05.15. The contract's wording is "MAY appear as one logical item with a truthful playback limitation" (§4.3).
- **Assessment:** **Portico behind**; at minimum the two folder names should be collapsed to one unsupported item or ignored.

### A-05.27 Symbolic links
- **Portico:** off by default; when on, a link must resolve inside the same source root; a link that escapes, or cannot be resolved, is an error for the whole page (`inventory_page.go:171-187`).
- **Jellyfin:** always follows links, anywhere. **Navidrome:** follows by default (`scanner.followsymlinks`).
- **Why it matters:** symlink farms are a common way to build libraries (debrid mounts, hard-link-less setups, "curated" views). They are invisible in Portico until the owner finds the per-folder switch, and then one dangling link stops the scan (B-05.5). Portico's containment rule is a real security property the others lack.
- **Assessment:** **Trade-off.** Deliberate (`contract.md` §4.2). I would keep the default and the containment rule, but skip a bad link with a warning rather than fail.

### A-05.28 One bad entry versus the whole scan
- **Portico:** any error from a directory read — permission denied, a link that cannot be resolved, a file that vanishes between listing and stat — fails the job (`service.go:472`).
- **Jellyfin:** logs and carries on, keeping what it had for that folder. **Navidrome:** "Skipping unreadable directory" and continues.
- **Why it matters:** B-05.4. The contract asks for per-object recording: "Permission errors are recorded per root/object and do not silently authorize deletion" (§4.2).
- **Assessment:** **Portico behind**; the safe middle is "record the error on that directory, treat its known contents as *not observed* (so nothing under it goes missing), continue, and end `complete_with_warnings` and non-authoritative for that subtree".

### A-05.29 Provider id hints in names
- **Portico:** `[tmdbid-N]`, `{tmdb-N}`, `[tvdbid-N]`, `[anilistid-N]` with `-`, `=`, `:` or space; numeric only; read from movie **file** names (`screen_match.go:21`, `screen_base.go:221`). Not read for shows at all, because the matcher is given the already-cleaned show title.
- **Jellyfin:** folder or file; IMDb, TMDB, TVDB for movies; plus TVmaze, AniDB, AniList, AniSearch for series.
- **Why it matters:** the most widely used Radarr/Sonarr naming presets put `{imdb-tt…}` or `[tvdbid-…]` on the **folder**. Portico reads none of those. (Matching itself is auditor 06's; the grammar gap is here.)
- **Assessment:** **Portico behind**; small.

### A-05.30 Which year wins
- **Portico:** the first plausible year after a separator (`catalog.go:354`), and separately in the matcher (`screen_match.go:16`, which also accepts a year at the very start and 18xx).
- **Jellyfin:** the last one (greedy match), with a rule that a date like `2013.12.09` is not a year.
- **Assessment:** **Portico behind** (B-05.7). Two readers with different rules for the same job is also a smell: `FilenameTitle` and `screenFilename` disagree on `1917 (2019)` and on `Movie.2048x1080`.

### A-05.31 Music grouping
- **Portico:** tags first. An album is keyed on **folder** + album + album artist + year + edition tags, so the same album tags in two folders are two albums; files in one folder with different album tags are different albums (`audio_repository.go:380-423`). A file keeps its album across a folder rename when its tags still describe it (`:296-309`). Compilations use "Various Artists" when the compilation tag is set.
- **Jellyfin:** folders define albums (a folder with audio files is an album); tags fill in. **Navidrome:** tags only; folders are irrelevant.
- **Why it matters:** Portico sits between the two. Including the folder protects against merging two unrelated "Greatest Hits" by the same artist and year, but splits multi-disc sets whose disc folders are not named exactly `CD1`/`Disc 1` (B-05.31).
- **Assessment:** **Trade-off (subjective).**

### A-05.32 Audiobooks
- **Portico:** a book is folder + album tag + author + narrator (+ ISBN/ASIN/edition); parts are ordered by track tag or a leading number; chapters come from the file; OPF, `book.nfo` and a Portico JSON sidecar are read (`localmetadata/audio_evidence_ingestion.go:30`).
- **Jellyfin:** audiobooks are a thin variant of music/books with name patterns for chapter and part. **Audiobookshelf:** the folder path is the model (`Author/Series/Title`), with rich sidecar and tag reading and podcasts.
- **Assessment:** **Portico ahead of Jellyfin, behind Audiobookshelf** on untagged collections (no author/series from folders). Deliberate ("Books and audiobooks: embedded tags and local files", `03 Catalogue and Metadata.md`).

### A-05.33 Scan tiers as promises about disk reads
- **Portico:** four named tiers with a hard rule that File List Only opens no file at all, enforced at read time, with dependencies between operations validated (`library_sources.go:119-153`, `contract.md` §2).
- **Jellyfin:** separate per-library switches (chapter images, trickplay, LUFS scan, real-time monitor…) with no "never read content" mode.
- **Why it matters:** for cloud storage where every read costs money or quota, Portico can list a remote library without downloading anything. Nobody else in this comparison offers that guarantee.
- **Assessment:** **Portico ahead.** Deliberate.

### A-05.34 Analysis stops while anything plays from the same source
- **Portico:** before analysing a file the scanner checks for any playing or buffering session on that source and, if there is one, records `pause_reason='playback'` and does nothing; a probe already running is cancelled (`analysis.go:137`, `:191-197`, `:256-275`).
- **Jellyfin:** scans and playback run together; the scan runs at lower process priority if configured.
- **Why it matters:** in a household where something is usually playing from the main media disk, first-time analysis (which is also what fills in music tags) may make little progress.
- **Assessment:** **Trade-off**, and it sits awkwardly with two stated rules: "background work is deprioritized, never paused" and `02 Server.md` line 30 ("playback win[s] against a scan without anything being paused"). The scan contract does allow it (§11 "analysis was paused for playback and will resume"). Worth a decision.

### A-05.35 Remote storage is a first-class back-end
- **Portico:** rclone remotes and WebDAV are listed and read natively, with credentials kept server-side, generations fencing every read, and an optional supervised read-only mount (`remotesources`, `mounts`).
- **Jellyfin, Navidrome, Audiobookshelf:** local paths only; remote storage means an OS or rclone mount the user manages, with all the usual watcher and timeout caveats.
- **Assessment:** **Portico ahead** — the largest architectural lead in this domain. Deliberate (`contract.md` §5–7).

### A-05.36 How a remote is listed
- **Portico:** one `rclone lsjson --max-depth=1 --disable=ListR` per directory (`mounts/native.go:115`), one directory per scan step.
- **Common rclone practice:** recursive or `--fast-list` listings for bucket-style and Drive-style remotes, which return a whole tree in a few API calls.
- **Why it matters:** a remote with 50,000 folders needs 50,000 rclone process starts and at least as many API calls, on top of the pacing in B-05.1. The contract asks for exactly this shape ("SHOULD list one directory/page at a time", §5.3) for bounded memory.
- **Assessment:** **Trade-off.** A recursive listing streamed into the same private table would keep memory bounded and cut the time by orders of magnitude on back-ends that support it.

### A-05.37 `.strm` descriptors
- **Portico:** catalogued by name; contents never read during a scan, never stored, re-validated against SSRF rules at every playback; analysing the target is a separate opt-in (`analysis.go:83-108`, `contract.md` §8).
- **Jellyfin:** reads the URL into the item at scan time.
- **Assessment:** **Portico ahead** on privacy and safety. Deliberate.

### A-05.38 Reporting progress and problems
- **Portico:** factual counts (discovered, analysed, warnings), a viewer-safe status, a per-source inventory with states (`available`, `missing_candidate`, `missing`, `trashed`, `replaced`, `unsupported`, `source_unavailable`), an episode to-do list with reason codes, and an admin notice when a scan stops. No percentage (the total is unknown by design).
- **Jellyfin:** a percentage bar per library and a log file; unparsed files are simply absent or misfiled.
- **Why it matters:** Portico tells the owner *which files it could not place and why*. It does not tell them which folder made a scan fail (`fail()` stores only a code, `inventory.go:345-388`).
- **Assessment:** **Portico ahead**, with that one gap.

### A-05.39 Deleting media from the UI
- **Portico:** off per library until enabled; preview → typed confirmation → delete, fenced by the library's revision; files go to `<state>/trash/` for 30 days and can be restored; files shared with another title are kept (`api/admin-libraries.md`).
- **Jellyfin:** a per-user "allow media deletion" permission; delete removes the file at once.
- **Assessment:** **Portico ahead** on safety. One trap: the trash lives in the server's state folder, so deleting from another volume copies the whole file there (B-05.25).

### A-05.40 Order, grouping and visibility of libraries
- **Portico:** owner-set order (`libraries.position`), per-viewer pinned libraries in the rail (up to 100), per-profile allowed libraries.
- **Jellyfin:** per-user library order and "group my media" views, per-user library access, per-library "enabled" switch. **Plezy:** client-side reorder and hide.
- **Assessment:** roughly level. Missing in Portico: disabling a library without removing it, and removing a library at all (F-05.16).

### A-05.41 Unicode and letter case
- **Portico:** paths are compared byte for byte; show and artist keys are lower-cased with whitespace collapsed but **not Unicode-normalised**; the folder-versus-file title check is a simple case-insensitive compare (`episodic.go:137`); sidecar names are exact lower-case (`poster.jpg`, not `Poster.JPG`, `localmetadata/sidecars.go:58`); extension, season-folder and extras-folder matching ignore case.
- **Jellyfin:** item ids are case-insensitive by default; image and metadata file names are matched without regard to case. I found no Unicode normalisation in the Jellyfin or Navidrome scan paths I read either.
- **Why it matters:** "é" can be stored as one character or as "e" plus an accent mark. macOS tools tend to write the second form. A folder `Pokémon` written one way and files written the other would fail Portico's folder check, and the same artist tagged both ways is two artists. Windows-ripped `Folder.jpg` is not picked up on a case-sensitive disk.
- **Assessment:** **Unclear** (nobody does this well). Normalising to NFC when building keys and comparing titles is cheap (`golang.org/x/text/unicode/norm` is already imported in `metadata/screen_match.go`).

### A-05.42 Per-library options
- **Portico:** kind, provider (+language/region), scan tier and operations, trickplay geometry, chapter-image mode, preview clips, allow deletion, trash retention, Continue Watching rules; per source: classification, symlinks, grace, interval.
- **Jellyfin:** 42 options (`LibraryOptions.cs`), including real-time monitor, embedded titles, embedded episode info, automatic series grouping, automatically add to collection, season-zero display name, metadata savers (write NFO), save artwork/subtitles/lyrics/trickplay beside media, subtitle download rules, custom tag delimiters and whitelist, prefer non-standard artists tag, metadata refresh interval, per-type image options.
- **Assessment:** **Trade-off (subjective)** — Portico's "owner choice, low ceremony" argues for fewer switches. The Jellyfin options that correspond to real needs Portico does not yet meet are listed in F-05.22.

### A-05.43 Reading local sidecars
- **Portico:** strict XML parsing with hard budgets: an NFO over 256 KiB, with more than 100 actors, 64 genres, a `<!DOCTYPE>`, or any text after the closing tag is rejected whole (`localmetadata/video_parse.go:50-110`, `:152`, `:214`); OPF text nodes over 4,096 characters reject the whole OPF (`evidence.go:70`). Only TMDB/TVDB/AniList ids are taken from an NFO.
- **Jellyfin:** lenient: reads what it can, and parses a bare IMDb/TMDB URL out of a non-XML `.nfo`.
- **Why it matters:** Kodi-exported NFOs for large-cast shows and "XML followed by a URL" NFOs are common; in Portico they are dropped entirely (B-05.29). Details belong to auditor 06.
- **Assessment:** **Trade-off**; truncating instead of rejecting would keep the safety budgets without losing the file.

### A-05.44 A file is interpreted once
- **Portico:** the name is read when the asset is first linked (`inventory.go:310`). A later rename moves the location but does not re-read the name; only files that still carry an issue are re-read (they have no link yet).
- **Jellyfin:** a renamed file is a new item, so it is always re-read.
- **Why it matters:** B-05.9 — renaming a misnamed file to fix it does nothing.
- **Assessment:** **Portico behind** here, as a side effect of doing identity better.

### A-05.45 Re-reading old problems when the reader improves
- **Portico:** a version number on the episode reader; when it is raised, every file that still has a naming issue is read again once in the background (`episodic_reparse.go`).
- **Jellyfin:** nothing equivalent; a full rescan re-resolves everything anyway.
- **Assessment:** **Portico ahead** — a neat mechanism, and exactly what will be needed as the grammar grows. Note it covers only files *with issues*; files the old reader accepted wrongly are not revisited.

### A-05.46 A numbered change feed
- **Portico:** `inventory_changes` gives each source a monotonically numbered list of events that other parts of the server (or a future integration) can page through (`inventory_admin.go:144`).
- **Jellyfin:** in-memory events (`ItemAdded`, `ItemRemoved`) and a web-socket `LibraryChanged` message.
- **Assessment:** **Portico ahead** in durability. Nothing prunes it (B-05.20).

### A-05.47 Hard links and the same file in two places
- **Portico:** two paths to one inode are one asset with two locations, and one catalogue entity (`inventory.go:207-246`); the same file reachable from two libraries is one asset linked in each.
- **Jellyfin:** two paths are two items.
- **Assessment:** **Portico ahead** — avoids duplicates when a download folder and a library folder share hard links, and avoids probing the same bytes twice.

### A-05.48 What kinds of re-scan an owner can ask for
- **Portico:** incremental (default), `?mode=full`, `?mode=integrity`; per library or per source; pause, resume, cancel, retry.
- **Jellyfin:** "scan for new and updated files", "search for missing metadata", "replace all metadata" (with or without images); per library, per item, or everything.
- **Assessment:** level in spirit. Missing in Portico: scanning one folder or one title (F-05.2).

### A-05.49 Files of the wrong kind inside a library
- **Portico:** the library's kind decides everything. In a movie library every video file is a movie, including a stray episode; in a TV library every video file must parse as an episode or it becomes an issue.
- **Jellyfin:** resolvers look at each folder's contents; a mixed library type exists but is discouraged in its own docs.
- **Assessment:** **Trade-off**; Portico's rule is simpler and matches the contract ("MUST NOT silently mix kinds", §1.1).

## Missing features and capabilities to consider

Sizes are rough: **small** = one function or file; **medium** = one package plus API and a little UI; **large** = new subsystem.

### F-05.1 Real-time monitoring of library folders
- **What:** notice new, changed and removed files within a minute, without a full scan.
- **Who has it:** Jellyfin (per-library switch; one recursive OS watcher per library path, 64 KB event buffer, events collapsed to the affected item or its nearest surviving parent, 60 s quiet period — `LibraryMonitor.cs`, `FileRefresher.cs`, `LibraryMonitorDelay`). Navidrome (5 s settle). Audiobookshelf (10 s batch, plus waiting for a growing file to stop changing). Plex.
- **Value:** the single most visible difference to an owner: "I copied a film and it appeared."
- **Size:** medium. Portico already has the pieces: a durable job per source and a directory-level work list. A watcher would only need to insert `inventory_directories` rows for the changed folders into a small job — a *partial* scan — after a quiet period.
- **Principles:** fits "Fresh". The contract already frames watchers as "acceleration hints only" backed by scheduled reconciliation (§4.1), which is the right stance: watchers do not work on NFS/SMB mounts (the server is not told about changes made elsewhere), overflow under bulk copies, and on Linux need one watch per directory against a kernel limit (`fs.inotify.max_user_watches`).
- **Opinion:** worth doing for local sources; keep interval scans for network and remote ones. Do the "wait until the file stops growing" step (F-05.19) at the same time.

### F-05.2 "These paths changed" API and scanning one folder or title
- **What:** an authenticated call that says "rescan `/media/tv/Show/Season 2`", and a "scan this folder / refresh this title" action in the UI.
- **Who has it:** Jellyfin `POST /Library/Media/Updated` (what Sonarr, Radarr and Lidarr's "Connect" feature calls), plus per-item refresh. Plex per-section `refresh?path=`. Navidrome selective scan (`scanner_selective_test.go`).
- **Value:** instant, exact updates from download managers; cheap fix-ups after a manual rename.
- **Size:** small to medium, same mechanism as F-05.1.
- **Opinion:** high value for the power users Portico targets; it is the reliable alternative to watchers on network storage.

### F-05.3 A default schedule and a scan at start-up
- **What:** new libraries rescan on their own (Jellyfin: every 12 hours; Navidrome: on start-up and by watcher).
- **Portico today:** interval 0 = only when asked (`0001_baseline.sql:1856`).
- **Size:** small (a default in `CreateAuthorizedThen` or the schema).
- **Opinion:** pick a default (daily for local, as Jellyfin's 12 h shows nobody minds) once B-05.1 makes an idle rescan cheap. This is "Owner choice, low ceremony": a correct default the owner can change.

### F-05.4 Ignore rules
- **What:** (a) a built-in list of names never scanned; (b) a per-folder ignore file; (c) hidden files skipped.
- **Who has it:** Jellyfin `IgnorePatterns.cs` (samples, `@eaDir`, `#recycle`, `@Recycle`, `$RECYCLE.BIN`, `System Volume Information`, `lost+found`, `.snapshots`, `.zfs`, `extrafanart`, `subs`, `*.trickplay`, every `.*`) and `.ignore` in gitignore syntax (empty file = ignore the folder). Navidrome `.ndignore`. Plex `.plexignore`.
- **Value:** fixes B-05.4, B-05.6 and B-05.16 and gives owners an escape hatch.
- **Size:** small for the built-in list; small to medium for an ignore file (reading it is a "content read", so it needs a decision about File List Only — the name `.ignore` existing could be enough for the empty-file case).
- **Opinion:** the built-in list should ship before release. Honour `.ignore` and `.plexignore` so migrants' existing files work.

### F-05.5 Automatic versions of one title
- **What:** `Movie (2021) - 2160p.mkv` and `Movie (2021) - 1080p.mkv` in one folder are one movie with a version picker.
- **Who has it:** Jellyfin (by name prefix), Plex (by matched identity), Emby.
- **Size:** medium. Already in `Backlog.md`. The rule could be either Jellyfin's (same folder, name starts with the folder name) or identity-based (two movie entities in one library that publish the same provider id merge). The second also fixes the Radarr-upgrade case in A-05.9.
- **Opinion:** do the identity-based merge; it needs coordination with auditor 06's matcher.

### F-05.6 Editions as a named concept
- **What:** `{edition-Director's Cut}` (Plex) shown as a label and kept as separate, selectable editions of one title.
- **Portico today:** the pattern is matched and then discarded.
- **Size:** small once F-05.5 exists (an edition is a labelled version).

### F-05.7 Multi-part stacking
- **What:** `-cd1/-cd2`, `-part1/-part2`, `-pt1`, `-disc1` played as one item. Jellyfin and Plex both do it for movies; Jellyfin also for episodes.
- **Size:** medium (playback must chain parts — check with the playback auditor). Low priority.

### F-05.8 Use the folder name; read id hints from folders; accept IMDb ids
- **What:** A-05.14 and A-05.29. Folder-derived movie titles and years; `[imdbid-tt…]`/`{imdb-tt…}`; hints on show folders.
- **Size:** small.
- **Opinion:** cheap and removes a whole class of first-run mismatches.

### F-05.9 A wider episode grammar
- **What Jellyfin accepts that Portico refuses** (each line is a test in `tests/Jellyfin.Naming.Tests/TV`): `S01 E02`, `s01.e02`, `S01xE02`; `Season 3 Episode 9`; `Episode 16`, `Episode 16 - Title`; `02 - Title`, `02.mkv`, `2. Title` inside a season folder; `Show E07`; `S01E23-E24-E26`, `02x03 - 02x04 - 02x15`; `[Group][Show][21][1080p]`; `Show 101` (absolute, any library); `dd.mm.yyyy` dates; `(yyyy-mm-dd)`.
- **Size:** small each; the hard part is keeping Portico's "do not guess" stance. A reasonable rule: accept the bare-number forms only inside a recognised season folder under a show folder.

### F-05.10 Season folders in other languages and shapes
- `S01`, `Series 1`, `Staffel`, `Saison`, `Temporada`, `Stagione`, `Сезон`, `シーズン`, `Season 1 (2016)`, `Show Season 1`, `3.Staffel`, release-style `Show.S04.1080p…` (`SeasonPathParserTests.cs`). **Size:** small.

### F-05.11 More kinds of extras
- Samples (ignored or typed), clips, theme songs (`theme.mp3`, `theme-music/`), theme videos (`backdrops/`), bare `trailer.ext`, `.trailer`/`_trailer`/` - trailer` suffixes, `-deletedscene`, `-clip`, `-extra`; extras on a **season**, not only a show or movie. **Size:** small for names; theme playback is a client feature.

### F-05.12 More file types
- See A-05.25. **Size:** small per type, but each needs a playback check. At least list them as "unsupported" so owners can see why a file is absent.

### F-05.13 Disc folders as one title
- Collapse `BDMV` and `VIDEO_TS` trees into one catalogue item (playable or flagged unsupported). **Size:** small for the collapse.

### F-05.14 Placeholder files
- Jellyfin's `.disc` stubs represent a title the owner has on a shelf (`placeholders.md`). **Size:** small. Niche; mention only.

### F-05.15 More library kinds
- **Home videos / other videos** (no provider, folder = album, date from file): Jellyfin, Plex. **Music videos**: Jellyfin. **Photos**: Jellyfin, Plex. **Books and comics** (epub, pdf, cbz): Jellyfin, Kavita-class tools. **Podcasts**: Audiobookshelf. **Mixed**: Jellyfin (discouraged).
- **Value:** "other videos" is the one most owners hit first (concerts, lectures, YouTube archives, family footage).
- **Size:** medium for a provider-less video kind, since kinds are data in Portico (`catalog_kinds`); large for photos or e-books (new clients surfaces).
- **Opinion:** a plain "Videos" kind (title = file name, grouped by folder, no matching) is the cheapest big win.

### F-05.16 Remove a library; disable a library; change a library's kind
- **Portico today:** rename and scan only (`ManagedLibrary.actions` in `client-core/src/library-management.ts`; note in `Libraries.tsx:305`). Not in `Backlog.md`.
- **Size:** medium (cascade through catalogue, user data, artwork, analysis artefacts in background batches).
- **Opinion:** required before release; an owner who mis-picks "movie" for a TV folder has no way out.

### F-05.17 Personal state that follows the title, not the file
- **What:** watch state, ratings and favourites survive replace-under-a-new-name, moves across disks, and delete-then-re-add.
- **Who has it:** Jellyfin (user-data keys from provider ids; on delete the rows are parked on a placeholder item with a retention date and re-attached later — `ItemPersistenceService.cs:94-132`). Navidrome (annotations moved when a missing track is matched by persistent id — `phase_2_missing_tracks.go`). Plex (by GUID).
- **Size:** medium; falls out of F-05.5 if entities merge by provider identity.
- **Opinion:** important. Upgrading a file is the most common library change there is.

### F-05.18 Recognising a moved file across file systems
- **What:** a cheap content fingerprint (size plus hashes of the first and last blocks) or, for music, tag-derived ids, so a copy to a new disk is recognised as the same file.
- **Who has it:** Navidrome (tags), Plex (partial hash). The Portico contract already anticipates "exact content evidence" (§1.3).
- **Size:** medium; costs two small reads per new file, so not in File List Only.

### F-05.19 Waiting for a file to finish arriving
- **What:** do not catalogue or probe a file that is still being written.
- **Who has it:** Audiobookshelf polls the modified time every 3 s until it stops changing (`Watcher.js:239`); Jellyfin's 60 s quiet period does the same job crudely.
- **Portico today:** nothing; see B-05.3.
- **Size:** small: skip (and do not fingerprint the directory as clean) any file modified within the last N seconds.

### F-05.20 Per-folder error tolerance with a per-path error list
- A-05.28. **Size:** small to medium.

### F-05.21 Pruning of scan history
- Keep the last N runs per source; delete their `inventory_directories`, `inventory_analysis_queue` and old `inventory_changes`. **Size:** small. See B-05.20.

### F-05.22 Per-library options others have that map to real needs
- Write NFO and artwork beside the media (Jellyfin "metadata savers", "save artwork into media folders") — the way owners keep their curation portable; arguably part of "Open data".
- "Automatically add to collection" from provider collections, and automatic series grouping across folders.
- Custom tag delimiters and a whitelist for multi-artist tags (`CustomTagDelimiters`, `DelimiterWhitelist` — the "AC/DC" problem); Navidrome has `artistsplitexceptions`.
- Prefer embedded titles over file names (home videos).
- Season-zero display name.
- Enable/disable a library.
- **Size:** each small to medium. **Opinion:** NFO/artwork export and tag delimiters are the two I would not skip.

### F-05.23 Inferring artist, author and series from folders when tags are missing
- Jellyfin uses `Artist/Album`; Audiobookshelf uses `Author/Series/Title` and `Title - Subtitle`, volume numbers in folder names. Portico yields "Unknown artist". **Size:** small. Fits the "File List Only" tier especially, where tags are never read.

### F-05.24 Disc-folder names beyond `CD1`
- `Disc 1 - Title`, `Vol 2`, `Volume 2`, `Part 1`, `Act 1`, `Digital Media 1` (Jellyfin `AlbumStackingPrefixes`), or simply trusting equal album tags across sibling folders. **Size:** small. See B-05.31.

### F-05.25 Automatic clean-up of long-missing items
- Plex "empty trash automatically after every scan"; Navidrome `purgemissing`. Portico's maintenance task trashes but never forgets (`inventory_maintenance.go`), so missing titles stay in the catalogue for ever unless handled one by one. **Size:** small. Needs a decision about how long personal state for a forgotten title is kept (F-05.17).

### F-05.26 Recursive or fast listing for remotes
- A-05.36. **Size:** medium.

### F-05.27 Library actions and live library updates in every client
- Plezy offers scan / refresh / analyse / empty trash from the phone and TV apps, and refreshes lists on library events with throttling. Portico's server already emits `library.scan.updated`; source configuration is "owner web UI only" (`INVENTORY_CONTRACT.md`). **Size:** small per client. Low priority.

### F-05.28 Object identity on Windows
- If the server is meant to run on Windows, file identity (volume serial + file id) is needed there; today it is unimplemented (B-05.28). **Size:** small.

### F-05.29 3D flags, and an "extras-only" trailers library
- Jellyfin reads `.3D.HSBS.` style flags and has a `trailers` collection type. Niche; listed for completeness.

## Possible bugs and risks in Portico

Confidence scale: **Confirmed by reading** (the code path is unambiguous; not run), **Probable**, **Speculative — needs a runtime check**. Nothing here was reproduced on a running server.

### The Backlog's scan items, re-checked

| Backlog item | Still holds? | Notes |
|---|---|---|
| "Missing sidecars aren't cached" | **Yes** | `server/localmetadata/sidecars.go:29`, `:112-135` caches hits only. It is costlier than the Backlog says: each miss is a helper process start plus a guard stat. See B-05.22. |
| "A second file of the same title becomes a second title" | **Yes, by construction** | A movie's key is its relative path (`catalog.go:511`). See B-05.8. |
| "A replaced file is not re-read by an incremental scan" | **Yes, by design of incremental mode** | The directory's fingerprint does not change when a file inside it is overwritten (`incremental.go:47-80`); only the weekly integrity scan or `?mode=full` sees it. Consequences in B-05.3. |
| "Library add form: local metadata only and `en` fails" | **Yes** | `server/httpapi/router.go:934-942` refuses any non-empty language unless `LibraryLanguageOffered` is true, and that function returns false for every agent except the online one (`server/metadata/library_agent.go:78-80`). So a create request carrying the local agent *and* a language is always refused. (Auditor 06's area; noted because it blocks creating a library.) |

### B-05.1 Scan speed has a hard ceiling of about three steps per second per folder source
- **What goes wrong:** the scan loop passes at most once every 300 ms while work is in flight, and each pass starts at most one step per source. The loop is driven through `worker.Run`, which treats its wake-up signal as "shared" and floors it at the step's own requested delay (`server/worker/worker.go:108-110`, `:150-170`); the step asks for 300 ms (`server/ingestion/service.go:388-390`); a source that is already running a step is skipped (`service.go:362`). One step is one page of at most 32 entries (`inventory.go:133`), one unchanged-directory check (`incremental.go:47`), 8 directory re-checks (`inventory.go:455`), 32 files queued (`analysis.go:25`) or **one** file analysed (`analysis.go:162`).
- **What that implies** (arithmetic, not measurement):

| Library shape | Directories | Minimum time just to walk it (first scan **or** an idle rescan) |
|---|---|---|
| 1,000 movies, one folder each | 1,001 | about 5 minutes |
| 10,000 movies, one folder each | 10,001 | about 50 minutes |
| 5,000 albums in `Artist/Album` | about 6,500 | about 33 minutes |
| 200,000 directories | 200,000 | about 17 hours |
| 1 million directories | 1,000,000 | about 3.5 days |

  The verify phase adds about an eighth again; queueing adds one step per 32 files; Basic analysis then runs at no more than 3.3 files per second per source (100,000 songs: at least 8 hours before their tags are read, during which they are "Unknown artist"). A directory holding 100,000 files takes about 16 minutes to list.
- **Why tests would not show it:** the tests call `Service.process` (`service.go:400`), which loops without the 300 ms pacing; production calls `Service.Run`.
- **How the comparison surfaced it:** Jellyfin and Navidrome scans are limited by disk and CPU; I went looking for Portico's limiter.
- **Confidence:** Confirmed by reading (the cadence); the table is derived. A stopwatch on the demo server's next scan would settle it in a minute.
- **Severity:** High — it collides with the "5–10 million items" target and makes "rescan every 15 minutes" (an option in the UI) continuous work on a modest library.
- **Backlog:** not listed.
- **Fix direction:** let a job run steps back to back until it has used a time slice (say 100 ms) or a foreground request is waiting, and give the scan loop an *urgent* signal for its own continuation. Small change in `Run`/`processQuantum`.

### B-05.2 Any folder changing during a scan fails the whole scan — and then nothing gets analysed
- **What goes wrong:** if a directory's modified or change time differs between two pages of its own listing, or between its listing and the end-of-scan verification, the step returns `ErrInventoryChanged` and the job is marked failed (`inventory.go:166-208`; `storage/inventory_page.go:146-148`, `:200-212`; `service.go:472-474`). A failed job never reaches the `analysis_enqueue` and `basic` phases, so files it inventoried are not probed, their NFO and artwork are not read and their tags are not read, until some later scan completes cleanly.
- **When:** whenever something writes into the library during the scan — a download finishing, Sonarr importing, a subtitle tool dropping an `.srt`, a tagger saving, another NFO manager. With the pacing in B-05.1 a scan of a 10,000-folder library is exposed for about an hour. Each failure also raises a "Library scan stopped" notice to administrators.
- **Evidence the strictness is intended:** `INVENTORY_CONTRACT.md` ("Changed revisions fail the run conservatively; explicit retry starts a new inventory"). What I doubt is intended is that *analysis* is lost with it.
- **Confidence:** Confirmed by reading. **Severity:** High on busy libraries. **Backlog:** not listed.
- **Fix direction:** on a changed directory, re-list that directory in the same job and mark the run non-authoritative (no files may be declared missing from it); carry on to analysis.

### B-05.3 A file that changes after it was first seen stays stale — and probably unplayable — for up to a week
- **What goes wrong:** incremental scans skip a directory whose own fingerprint is unchanged (`incremental.go:47-80`). Appending to, or overwriting, a file does not change its directory's timestamps. So the stored size and modified time stay as first seen. Playback then compares the file on disk with the stored size and time and refuses on a mismatch (`server/playback/source_preparation.go:107-109`). Analysis also refuses ("source_revision_changed", `analysis.go:297-303`) and the source shows warnings.
- **The common trigger is a file caught mid-copy:** a scheduled scan lists a folder while a 40 GB file is still being copied in under its final name (plain `cp`, an SMB copy from a desktop, a torrent client writing in place, `rsync --inplace`). The copy finishes; nothing tells Portico.
- **Other triggers:** in-place edits (`mkvpropedit`, a tagger that rewrites in place), replacing a file by writing over it.
- **Recovery:** the automatic integrity scan (when the last full listing is 7 days old, `incremental.go:38`) or an owner-requested `?mode=full`.
- **Confidence:** Confirmed by reading for the stale record; **Probable** for "unplayable" (I did not trace every playback path for a fallback that re-observes the file). **Severity:** High. **Backlog:** the stale-facts half is listed; the mid-copy trigger and the playback consequence are not.
- **Fix direction:** F-05.19 (do not accept a file modified in the last N seconds, and do not record its directory as clean) plus a per-file size/mtime check for local sources (A-05.4). On a playback mismatch, re-observe that one file and retry.

### B-05.4 One unreadable sub-folder stops every scan of that source
- **What goes wrong:** a directory the server cannot open (the classic is `lost+found`, mode 700 and owned by root, at the top of every ext4 volume; also another user's private folder on a shared NAS) makes its listing step return an error, which fails the job (`storage/inventory_page.go:135-138` → `service.go:472`). Every later scan fails at the same place. Directories are processed in byte order of their relative path (`inventory.go:96`), so folders that sort after the bad one are never listed at all; and because no scan completes, nothing is analysed (as in B-05.2) and nothing can ever be declared missing.
- **How the comparison surfaced it:** `lost+found` is in both Jellyfin's ignore list and Navidrome's; Navidrome also logs "Skipping unreadable directory" and continues (`navidrome/scanner/walk_dir_tree.go:301`).
- **Confidence:** Confirmed by reading. **Severity:** High when it applies (a library whose root is a mount point). **Backlog:** not listed.

### B-05.5 With "follow symlinks" on, one dangling or out-of-root link stops the scan
- **What goes wrong:** `filepath.EvalSymlinks` failing (dangling link) or resolving outside the root returns an error for the whole page (`storage/inventory_page.go:175-182`), which fails the job. The escape case is asserted by a test (`storage/inventory_page_test.go:137`), so failing is intended there; a dangling link is treated the same.
- **Confidence:** Confirmed by reading. **Severity:** Medium (only when the option is on, but symlink-built libraries are exactly where dangling links occur). **Backlog:** not listed.

### B-05.6 Hidden and system files are catalogued as media
- **What goes wrong:** the only filter between the directory listing and the catalogue is the file extension (`catalog/inventory.go:42-48`, `:141-144`). So:
  - macOS resource-fork files `._Movie (2009).mkv` (a few kilobytes, created whenever a Mac writes to a non-Apple share) become a second "Movie (2009)" (port: `FilenameTitle` strips the `._`), whose probe fails and leaves a permanent warning;
  - Synology `@eaDir/…/SYNOVIDEO_VIDEO_SCREENSHOT.mp4`, `#recycle/…`, QNAP `@Recycle`, Linux `.Trash-1000/files/…`, Windows `$RECYCLE.BIN/…` and any dot-folder are scanned, so deleted files reappear as titles.
- **Confidence:** Confirmed by reading. **Severity:** High (any library on a NAS share used from a Mac). **Backlog:** not listed.

### B-05.7 A year inside a title is taken as the release year
- **What goes wrong:** `yearPattern` takes the **first** 1900–2099 number after a separator and cuts the title there (`catalog.go:354-365`). The matcher's own reader does the same (`metadata/screen_match.go:16`, `:55-58`), and the search year comes from the catalogue year first (`:73`).
- **Examples (port):** `Blade Runner 2049 (2017).mkv` → "Blade Runner", 2049. `Wonder Woman 1984 (2020)` → "Wonder Woman", 1984. `Death Race 2000 (1975)` → "Death Race", 2000. `The Wolf of Wall Street 2001 (2013)` (a Jellyfin test) → 2001. Shows: `Cyberpunk 2077 Edgerunners` → "Cyberpunk", 2077; `Space 1999` → "Space", 1999. Also `My Movie 20131209` → 2013 and `Movie.Title.2048x1080` → 2048, because the pattern does not require a boundary after the year.
- **Effect:** wrong title and year on the card, and a search for the wrong film; I would expect a wrong match or none (auditor 06 to confirm).
- **Confidence:** Confirmed by reading. **Severity:** Medium-High (well-known titles). **Backlog:** not listed.
- **Fix direction:** prefer a year in brackets, else the last year, and require a non-digit after it — Jellyfin's rule.

### B-05.8 A second file for a movie is a second movie; an upgraded file loses the title's personal state
- **What goes wrong:** the movie key is the relative path (`catalog.go:511`). Two files in one folder are two movies (Backlog). The same rule means a file **replaced under a new name** — what Radarr does on every quality upgrade when the name includes the quality — creates a new movie; the old one stays "available" through the grace period and then sits unavailable with the watch state, rating, favourite, collection membership and any manual match the owner made. Songs and audiobook parts are keyed the same way (`audio_repository.go:287`).
- **Confidence:** Confirmed by reading. **Severity:** High. **Backlog:** first half listed.

### B-05.9 Renaming a file to fix it does not change how Portico read it
- **What goes wrong:** a renamed file is recognised by inode as the same asset, and because the asset already has a title the name is not read again (`catalog/inventory.go:306-314`). A movie keeps the title from its old name; an episode misnamed `S01E02` and corrected to `S01E03` stays episode 2. No code consumes the `relocated` event.
- **Confidence:** Confirmed by reading. **Severity:** Medium (it is the first thing an owner tries). **Backlog:** not listed.
- **Fix direction:** on relocation, re-run the name reader when the entity is not manually assigned or locked, and move the link if the reading changed.

### B-05.10 Flat show folders, and the show folder's year, are only understood inside a `Season N` folder
- **What goes wrong:** `ParseEpisode` looks at the folder above a file only when the file's own folder is `Season N` or `Specials` (`catalog/episodic.go:44-50`).
  - `Severance (2022)/S01E01.mkv` → rejected, `series_identity_missing` (port).
  - `Doctor Who (2005)/Doctor Who - S01E01.mkv` → show "Doctor Who" with **no year**, while `Doctor Who (2005)/Season 2/Doctor Who - S02E01.mkv` → "Doctor Who", 2005. The show key is title + year (`episode_repository.go:123`), so one folder yields **two shows**. The same split happens with season folders Portico does not recognise (`S01`, `Series 1`, `Staffel 1`, `Season 1 (2022)`, `Season 1/Disc 1`).
- **Confidence:** Confirmed by reading (illustrated by the port). **Severity:** High — flat folders are common for miniseries and anime. **Backlog:** not listed.

### B-05.11 The folder-must-match rule rejects correctly organised shows
- **What goes wrong:** inside a season folder, the title from the file name must equal the title from the show folder, compared case-insensitively and nothing more (`episodic.go:133-142`). Rejected with `folder_and_filename_series_conflict` (port):
  - `The Office (US)/Season 01/The.Office.US.S01E01.720p.mkv` (brackets);
  - `Mr. Robot/Season 1/Mr.Robot.S01E01.mkv` ("Mr  Robot" with two spaces versus "Mr Robot": dots become spaces without collapsing);
  - `Marvel's Agents of S.H.I.E.L.D/…/Marvels.Agents.of.S.H.I.E.L.D.S01E01.mkv` (apostrophe);
  - `Severance {tvdb-371980}/Season 01/Severance - S01E01.mkv` and `Severance [tvdbid-371980]/…` (an id hint without a year stays in the folder title);
  - `Blake's 7/Season 1/Blakes 7 - S01E01.mkv`;
  - anime folders in English holding files in romaji: `Frieren/Season 1/Sousou no Frieren - 01.mkv`.
- **Effect:** whole shows land in the to-do list; the library is permanently "with warnings" (B-05.26).
- **Confidence:** Confirmed by reading. **Severity:** High. **Backlog:** not listed.
- **Fix direction:** A-05.16 — let the folder name the show.

### B-05.12 Resolution tags such as `1920x1080` are read as season × episode
- **What goes wrong:** the marker pattern `[0-9]{1,4}x[0-9]{1,4}` has no boundary rules (`episodic.go:22`). Port results:
  - anime: `[Group] Show - 05 [1920x1080].mkv` → show **"Show - 05 ["**, season **1920**, episode **1080**, no issue raised — a confident wrong filing. `[CASO&Sumisora][Oda_Nobuna_no_Yabou][04][BDRIP][1920x1080]…` (a Jellyfin test) likewise.
  - TV: `Show - S01E01 - 1920x1080.mkv`, `Show.S01E01.1080p.DD5.1x264-GRP.mkv` → rejected as `conflicting_episode_markers`.
  - The `s…e…` alternative also matches inside words: `Shows1e1.mkv` → show "Show".
- **Confidence:** Confirmed by reading. **Severity:** Medium (fansub releases often carry resolution tags). **Backlog:** not listed.

### B-05.13 Other correct names the episode reader refuses or shortens
All port results; each is a real-world shape:
- Daily shows in provider-numbered season folders — the layout Sonarr produces: `Jeopardy/Season 40/Jeopardy - 2024-01-15.mkv` → `folder_and_filename_season_conflict`, because a dated file's season is forced to the year (`episodic.go:111-115`).
- Episode titles that end in "Part N": `… - S01E19 - The Siege of the North, Part 1.mkv` → `multipart_episode_requires_assignment` (`episodic.go:31`, `:61`). `…(Part 1)` and `…Part 1 [1080p]` pass.
- A second marker in the episode title: `Show - S01E01 - 2x4 Lumber.mkv`, `… - The S01E02 Paradox.mkv` → `conflicting_episode_markers`. Jellyfin's tests carry scars for the same family ("S03E21 - E2", "S01E01 1-23-45").
- `S01E00` → `episode_number_out_of_range` (some shows and many anime releases number a prologue or pilot as episode 0).
- `S01E23-E24-E26`, `S01E01-E09` (more than 8) → `ambiguous_episode_range`.
- `1x01-02` → episode 1 only; `02x03 - x04 - x15` → episode 3 only. The extra episodes are dropped without an issue.
- Anime: `Show - 2019 [1080p].mkv` → absolute episode 2019; `Show - 2020 - 01.mkv` → absolute 2020.
- **Confidence:** Confirmed by reading. **Severity:** Medium (daily shows), Low (the rest). **Backlog:** not listed.

### B-05.14 An extras folder name anywhere in the path swallows everything under it
- **What goes wrong:** `classifyExtra` tests **every** folder in the relative path against 28 names (`catalog/extras.go:130-144`) and runs before any library-kind logic (`inventory.go:360-362`).
  - TV library, the series *Extras*: `Extras/Season 1/Extras - S01E01.mkv` → an "other" extra, never a show (port).
  - A movie root organised with a `Shorts/`, `Other/`, `Trailers/` or `Bonus/` top-level folder: every film under it becomes an extra of the library root and disappears from the grid (port: `Shorts/Pixar - Bao (2018).mkv` → extra "short").
- **How the comparison surfaced it:** Jellyfin only treats these names as extras directly inside a movie, show or season folder, and explicitly does not at the top level (`CoreResolutionIgnoreRule.cs:44-61`).
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-05.15 A ripped Blu-ray or DVD folder becomes dozens of titles
- **What goes wrong:** `.m2ts` and `.vob` are ordinary media (`assets.go:261`). `Movie (2009)/BDMV/STREAM/00001.m2ts` … `00213.m2ts` become movies "00001" … "00213"; `VIDEO_TS/VTS_01_1.VOB` becomes "VTS 01 1" (port). The contract says such a structure "MAY appear as one logical item with a truthful playback limitation" (§4.3); `INVENTORY_CONTRACT.md` says they "use the normal file path".
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-05.16 Samples and bare trailers become titles
- `Movie (2020)/Sample/sample.mkv` → movie "sample"; `movie-sample.mkv` → "movie-sample"; `Movie (2020)/trailer.mkv` → movie "trailer"; `Movie (2020).trailer.mkv` → a second "Movie (2020)"; `backdrops/loop.mp4` → movie "loop" (port). In a TV library each becomes an unassignable issue instead.
- **Confidence:** Confirmed by reading. **Severity:** Medium (scene releases ship samples). **Backlog:** not listed.

### B-05.17 Titles are left mangled until (and unless) a provider match replaces them
- Dots become spaces one for one: "E T  the Extra-Terrestrial", "Mr  Robot", "St  Vincent", "Kill Bill Vol  1" (`catalog.go:363`).
- With no year, release tags stay: "Avatar 1080p BluRay x264-GROUP".
- A trailing separator stays: `Maximum Ride - 2016 - …` → "Maximum Ride -".
- A hint or edition before the year, or with no year, stays: "Avatar {tmdb-19995}", "Movie {edition-Extended}".
- **Effect:** cosmetic in an online library once matched; permanent in a **local-metadata-only** library without NFOs, and in the show key (B-05.11).
- **Confidence:** Confirmed by reading. **Severity:** Low-Medium. **Backlog:** not listed.

### B-05.18 File identity includes the device number, which is not stable across reboots on many setups
- **What goes wrong:** identity is `dev:<st_dev>:ino:<st_ino>` (`storage/local_observation_linux.go:16`), for files and for the source root. On Linux, `st_dev` for NFS, SMB, FUSE (rclone mount, mergerfs), btrfs subvolumes, ZFS datasets and overlay mounts is an anonymous number handed out in mount order; for USB and device-mapper disks it follows detection order. After a reboot or remount it can be different.
- **Consequence chain (by reading):** root identity differs → `ErrSourceChanged` (`ingestion/inventory.go:52`) → job fails with `scan_root_changed`, source health `root_changed`, notice raised. The owner must edit the folder and tick "accept replacement" (`catalog/library_sources.go:334-346`), which starts a new incarnation; every file is then treated as replaced (`catalog/inventory.go:193`), gets a new asset, and all analysis (probe, and under Complete: trickplay, loudness…) is redone. Titles and watch state survive because keys are paths and numbers.
- **Also:** file systems that do not give stable inode numbers at all (some FUSE and SMB configurations) would look like "every file replaced" on every remount.
- **Why it is this way:** the root check is how Portico tells an empty mount point from the mounted disk (`INVENTORY_CONTRACT.md`, "An unmounted backing directory is not the previously admitted mount") — a real benefit.
- **Confidence:** Probable (the mechanism is confirmed by reading; that `st_dev` shifts is general Unix behaviour I did not test on the hosts). **Severity:** High where it applies. **Backlog:** not listed.
- **Fix direction:** use the file-system UUID/fsid where the OS offers one (the code already reads `Fsid` in `storage/mount_identity_unix.go:32-35`), or on a root mismatch compare a sample of known child inodes+sizes and auto-accept when they agree.

### B-05.19 A directory's timestamp is not always evidence that nothing was added
The incremental skip trusts the directory's modified/change time. Three cases where that fails, leaving new files unseen until the weekly integrity scan:
- **Union file systems.** mergerfs reports a directory's times from one branch by default; a file added on another branch does not change them (its documentation recommends a different `getattr` policy for media servers for this reason — from memory, not re-checked here).
- **Mounts that do not maintain directory times** (several cloud back-ends under an rclone mount; some SMB servers).
- **Coarse timestamps.** If a file is added in the same clock tick as an earlier change that the listing already saw, the directory's time does not move. On Linux local disks the tick is milliseconds; on NFSv3, FAT-family and some SMB shares it is one to two seconds. The Linux fast path has no guard for this (the inotify guard in `storage/inventory_changes_linux.go` is used only by the streamed listing, `inventory_page.go:151-157`).
- **Confidence:** Probable (mechanism confirmed; file-system behaviour from general knowledge). **Severity:** Medium-High for mergerfs and cloud-mount users, who are a large share of the target audience. **Backlog:** not listed.
- **Fix direction:** never record a directory as clean if its modified time is within a few seconds of when it was listed (git's "racy timestamp" rule); offer a per-source "always list every folder" switch, defaulting on for `network` classification.

### B-05.20 Scan bookkeeping is never pruned
- **What goes wrong:** every scan writes one `inventory_directories` row per directory (`0001_baseline.sql:1883`), one `inventory_analysis_queue` row per file (`ingestion/analysis.go:123`), an `inventory_runs` row, a `jobs` row and `inventory_changes` events. I found no statement that deletes any of them except when a whole asset is purged (`catalog/purge.go:71`); `INVENTORY_CONTRACT.md` says of the change feed "There is no automatic event-log retention/purge in P04A".
- **Scale:** 20,000 directories and 60,000 files scanned hourly is about 480,000 + 1,440,000 rows a day.
- **Confidence:** Confirmed by reading (absence of a delete, by search). **Severity:** Medium-High over months; it works against "open data in one state folder" staying small and against backup time. **Backlog:** not listed.

### B-05.21 Every scan re-queues every file
- **What goes wrong:** after reconciliation, `enqueueAnalysisPage` walks **all** available files of the source, 32 per step, and writes a queue row for each, even when nothing changed (`ingestion/analysis.go:27-133`). With B-05.1 that is one step per 32 files: 100,000 files ≈ 16 more minutes per scan and 100,000 writes.
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.
- **Fix direction:** the page-commit path already queues changed files (`catalog/inventory.go:336`); the full pass is only needed after a policy change.

### B-05.22 Looking for sidecars that are not there costs dozens of process starts per file
- **What goes wrong:** for a TV episode `ReadSidecars` tries poster (4 names), backdrop (4), logo (1) and season poster (4–5) in the file's folder and its parent — up to 28 reads when none exist (`localmetadata/sidecars.go:57-75`). Each read goes through `storage.ReadSmall`, which starts a helper process (`storage/client.go:363-402`), after a read guard that itself stats the path through another helper (`ingestion/analysis.go:308-324`). NFO lookup adds up to three more. Hits are cached; misses are not.
- **Confidence:** Confirmed by reading. **Severity:** Medium (it multiplies the first-import time of large TV and music libraries). **Backlog:** listed (the cost is understated there).
- **Fix direction:** the scanner already has each directory's listing in hand at inventory time; pass the set of names along (or cache "directory D contains these names" per scan) and never ask for a file that is not listed.

### B-05.23 A new library never rescans by default
- A library's first folder is created by a database trigger that leaves `interval_seconds` at its default of 0, "only when asked" (`0001_baseline.sql:1856`, `:4122`); there is no watcher and no start-up scan. A folder *added later* through the dialog starts from a draft of one hour (`apps/portico-server/packages/client-core/src/server-admin/panel-logic.ts:151`). So the two kinds of folder behave differently, and the main one never rescans.
- **Confidence:** Confirmed by reading (not checked in the running web form). **Severity:** Medium — a first-run impression problem. **Backlog:** not listed.

### B-05.24 Deleted files stay "available" for a day and are never removed on their own
- By design (A-05.11): two completed scans plus 24 h before a vanished file is even marked unavailable; nothing ever forgets it. With B-05.2/B-05.4 preventing completed scans, vanished files stay available indefinitely.
- **Confidence:** Confirmed by reading. **Severity:** Medium (design risk; deliberate). **Backlog:** not listed.

### B-05.25 Deleting media on another volume copies it into the server's state folder
- **What goes wrong:** delete-to-trash moves the file to `<state>/trash/<id>/` (`administration/deletion.go:301-309`); across file systems the move is a full copy then remove (`administration/file_operations.go:346-420`). Deleting a season from a NAS library copies it onto the (usually small) disk holding the database, for 30 days. I saw no free-space check on that path.
- **Confidence:** Probable (read the move code; did not trace every caller). **Severity:** Medium — a full state disk is a server outage. **Backlog:** not listed.
- **Fix direction:** a trash folder at the root of each source (as desktop systems do), falling back to the state folder only for read-only sources.

### B-05.26 One unplaceable file makes a TV library permanently "degraded"
- `finishScanTx` ends every scan as `complete_with_warnings` and the source as `degraded` while any episode issue exists in the library (`ingestion/analysis.go:553-566`). The only way to clear an issue is to assign the file to an episode; there is no "ignore this file" (`catalog/episode_repository.go` has assignment only). A `Specials/Behind the Scenes.mkv` is enough.
- **Confidence:** Confirmed by reading. **Severity:** Low-Medium (alarm fatigue). **Backlog:** not listed.

### B-05.27 Show identity is "title + year", which both splits and merges wrongly
- Splits: B-05.10; also the same show under two sources where only one folder carries the year.
- Merges: two different shows with the same title and no year in their names (`Kingdom/…`, `The Office/…` UK and US in separate folders both named "The Office") become one show, their episodes interleaved by number.
- A file-name year silently beats a different folder year (`Show (2010)/Season 1/Show (2011) - S01E01.mkv` → 2011, port).
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.
- **Fix direction:** key a show on its folder (per source), merge shows across folders only by matched provider identity — which is what Jellyfin's "automatic series grouping" does.

### B-05.28 On Windows the server cannot add a library
- `observedObjectBinding` returns an empty identity on anything that is not Linux or macOS (`storage/local_observation_other.go:7`), and library creation, folder saving and every scan step reject an empty root identity (`catalog/catalog.go:147-149`, `catalog/library_sources.go:266-269`, `ingestion/inventory.go:49-51`).
- **Confidence:** Probable (by reading; build tags followed, nothing run). **Severity:** Critical *if* a Windows server is intended — the tree has Windows build files and the folder picker documents Windows drive letters, but the architecture docs do not name supported server platforms. **Backlog:** not listed.

### B-05.29 Local metadata files are rejected whole when they exceed a budget
- A `tvshow.nfo` or `movie.nfo` with more than 100 `<actor>` entries, more than 64 genres, a `<!DOCTYPE>` line, an encoding other than UTF-8 (Go's XML decoder needs a charset reader for those), or any text after the closing tag — the Kodi "NFO plus a URL" form — is discarded entirely (`localmetadata/video_parse.go:50-110`, `:152`, `:214`). An OPF whose description exceeds 4,096 characters is discarded entirely (`localmetadata/evidence.go:70`).
- **Confidence:** Confirmed by reading for the limits; Probable for the encoding case. **Severity:** Medium for local-only libraries. **Backlog:** not listed. Detail belongs to auditor 06.

### B-05.30 A stopped scan does not say where it stopped
- `fail()` stores one of four codes and the owner-facing text is "Scan needs attention. Existing inventory was retained." (`ingestion/inventory.go:345-388`, `ingestion/service.go:164-166`). The failing directory is known at that moment and thrown away. For B-05.2, B-05.4 and B-05.5 the owner has nothing to act on.
- **Confidence:** Confirmed by reading. **Severity:** Low-Medium. **Backlog:** not listed.

### B-05.31 Multi-disc albums split when disc folders are not named exactly `CD1` / `Disc 1`
- The album key includes the folder (`catalog/audio_repository.go:380`, `:414`), and only `^(disc|disk|cd)[ ._-]*N$` folds a sub-folder into its parent (`:92`). `Album/Disc 1 - Studio/…` and `Album/Disc 2 - Live/…` with identical album tags become two albums.
- **Confidence:** Confirmed by reading. **Severity:** Low-Medium. **Backlog:** not listed.

### B-05.32 Accented names written two ways may not be recognised as equal
- Keys and the folder check do not normalise Unicode (A-05.41). A show folder created by a Mac tool and files named by a Linux tool can differ invisibly and hit `folder_and_filename_series_conflict`; an artist tagged both ways is two artists.
- **Confidence:** Speculative — needs a runtime check with real NFD and NFC names. **Severity:** Low-Medium. **Backlog:** not listed.

### B-05.33 With "follow symlinks" on, a folder alias inside the root may make every incremental scan fail on a database constraint
- **What I traced:** one scan may hold only one row per directory identity (`CREATE UNIQUE INDEX inventory_directories_visited ON inventory_directories(job_id,identity) WHERE identity!=''`, `0001_baseline.sql:3717`). That is the loop protection, and on a full listing it works because child rows are added with `INSERT OR IGNORE` (`catalog/inventory.go:136`). But both the real folder and its symlink alias are written to the directory cache (`catalog/inventory.go:133`), and on an incremental scan the skipped parent re-adds **both** as rows with an empty identity (`ingestion/incremental.go:109`). When each is then processed, its identity is filled in by a plain `UPDATE` (`incremental.go:113` for the skipped one, `catalog/inventory.go:115` for the listed one); the second update collides with the first, the statement errors, and the job fails.
- **Confidence:** Probable (traced by reading; depends on nothing unusual, but not run). **Severity:** Medium (needs the symlink option and a directory alias such as `Movies/4K -> ../UHD`). **Backlog:** not listed.

## Where Portico is stronger

- **It cannot lose a library to a missing disk.** Root identity, "errors are never absence", two observations and a grace period (A-05.10). Jellyfin protects only the top-level folder; Navidrome's answer is to never purge.
- **Scans are durable.** Pause, resume, cancel, restart-safe, one job per folder with its own health (A-05.1, A-05.13).
- **A hung mount cannot hang the server** (A-05.2).
- **Exact rename and move tracking with no content reads**, and hard links collapse to one asset (A-05.7, A-05.47).
- **Episodes are keyed on show + number**, so a replaced episode file keeps its page and its watch state, and two files for one episode are already two versions (A-05.8).
- **It says what it could not place and why**, lets the owner assign it, and re-reads old problems when the reader improves (A-05.15, A-05.45). No other project here has this.
- **Scan tiers that are real promises**, including a mode that opens no file (A-05.33) — a genuine advantage on metered cloud storage.
- **rclone and WebDAV as first-class sources**, credentials kept server-side; `.strm` URLs never stored (A-05.35, A-05.37).
- **Deleting media is off by default, previewed, typed-to-confirm and reversible** (A-05.39).
- **A durable change feed and an administrator notice when a scan stops** (A-05.46, A-05.38).
- **Anime as its own library kind** with its own numbering rule (A-05.12).
- **Symlinks cannot be used to read outside a library** (A-05.27).
- **Each episode in a multi-episode file is its own entry** (A-05.20).
- **Stable public ids**: a title that leaves and returns at the same path keeps its id (`catalog_identities`).
- **Bounded memory on huge directories** (paged listing by kernel cookie, `storage/inventory_page_linux.go`).

## Doc/code disagreements

1. **Permission errors.** `contract.md` §4.2: "Permission errors are recorded per root/object and do not silently authorize deletion." Code: a permission error fails the whole job with a generic code and records no path (B-05.4, B-05.30).
2. **"Never paused".** `02 Server.md` line 30 says playback wins against a scan "without anything being paused", and `server/workpolicy/policy.go:2` says "Background work is never paused for a window or foreground activity." Code: analysis is skipped with `pause_reason='playback'` while anything plays from the same source (`ingestion/analysis.go:191-197`). The scan contract itself (§11) does describe "analysis was paused for playback".
3. **Missing files the contract points to.** `contract.md` §13 names `scan-profile.schema.json` and `conformance.json`, and its last section names `plex-jellyfin-precedent-review.md`. None of the three exists under `portico-internal/Product Contracts/` (searched the whole tree).
4. **Source kinds.** Contract §1.2: `local`, `rclone`, `webdav`. Code: `library_sources.kind` is `local` or `remote` (`ingestion/inventory.go:20`); rclone versus WebDAV lives in a separate `remote_sources` table.
5. **States.** Contract §11 lists scan states `catalog_published`, `complete_pending`, `complete_running`; the code has none of them and has `legacy_inventory`, `verifying`, `analysis_enqueue` and job status `paused` instead (`ingestion/service.go:456-471`). Source health `root_changed` is used in code (`inventory.go:349`) and absent from the contract's list.
6. **What File List Only infers.** Contract §2.1: "infer media kind, hierarchy, title, year, season/episode, **version**, and **disc/descriptor kind** from names and structure". Code infers no version and no disc kind.
7. **Disc structures.** Contract §4.3: BDMV/VIDEO_TS "MAY appear as one logical item with a truthful playback limitation". `INVENTORY_CONTRACT.md` and code: each stream file is an ordinary title (B-05.15). (A "MAY", so not a breach — but the two documents read differently.)
8. **Analysis operations.** `api/admin-libraries.md` lists 16 operations (including `artwork_extraction`, `chapter_read`, `deep_stream_analysis`, `subtitle_extraction`, `video_preview`, `credits_detection`, `subtitle_download`, `lyrics`). The server publishes and accepts only the ten that map to a scan-policy operation (`administration/library_runtime.go:11`, `library_settings.go:75-81`). Belongs to auditor 04; noted because it is the same document.
9. **Scanning has no architecture chapter.** `03 Catalogue and Metadata.md` does not describe scanning; `02 Server.md` gives it one table row. The only description is `ingestion/INVENTORY_CONTRACT.md`, written as an implementation hand-off ("P04A", "P04B") rather than as the current design. Given how much of this audit's risk sits here, it deserves its own chapter.
10. **`Backlog.md` omissions.** "Remove library" is tracked only in a code comment (`web/src/screens/server/Libraries.tsx:305`).

## Open questions for Justin

1. **Who names a show or a movie — the folder or the file?** Plex, Jellyfin, Kodi and Sonarr/Radarr all assume the folder. Portico uses the file and, for shows, rejects files that disagree with the folder. Should the folder become the authority, with the file name as fallback?
2. **Keep "never guess"?** Should bare forms like `02 - Title.mkv` and `Episode 3.mkv` be accepted inside a recognised `Show/Season N` folder, or stay on the to-do list?
3. **Should personal state follow the title rather than the file?** That is, should two movie files that match the same provider record merge into one title (versions), so an upgraded file keeps its watch state?
4. **Real-time monitoring:** build it for local folders? And a "these paths changed" call for Sonarr/Radarr-style tools?
5. **What should a new library's default rescan be?** Today: never, for the first folder; hourly, for folders added later.
6. **How long should a deleted file linger?** Is 24 hours right for a file missing from a folder that was itself listed successfully on a local disk?
7. **Should missing titles ever be forgotten automatically**, and if so what happens to their watch history?
8. **May a scan that saw a folder change finish as "not authoritative" instead of failing?** (B-05.2)
9. **Is pausing analysis during playback from the same source intended**, given "deprioritized, never paused"?
10. **Is a process start per file-system call the intended long-term shape**, or was a long-lived helper always the plan?
11. **Which operating systems must the server run on?** If Windows: file identity is missing there. On Linux: is the device number meant to be part of identity, given it can change across reboots for network and FUSE mounts?
12. **Should symlinks be followed by default for local folders** (as Jellyfin and Navidrome do), keeping the stay-inside-the-root rule?
13. **Ignore rules:** ship a built-in list and hide dot-files by default? Honour `.ignore` and `.plexignore`?
14. **More library kinds before release?** A provider-less "Videos" kind is the cheap one; photos, books and podcasts are large.
15. **Disc folders (`BDMV`, `VIDEO_TS`):** one unsupported item, or ignored?
16. **Is an album meant to be tied to its folder?** (A-05.31, B-05.31)
17. **Anime-only absolute numbering:** keep it tied to the library kind?
18. **Daily shows:** keep "season = year, episode = day of year", or adopt provider season numbers?
19. **Where should deleted media wait** — the server's state folder (today) or a trash folder on the media volume?
20. **Unknown media extensions:** widen the list, or show them in the inventory as "unsupported" so owners can see why a file is missing?

## Evidence boundary

- **Nothing was run.** No Portico server, test, build or scan was executed. Every Portico statement is from reading source at the working-tree state on 11 October 2026.
- **The Python port.** File-name results marked "port" come from my own copy of five small functions and their regular expressions (kept in the session scratchpad, not in the repository). Go's and Python's regular-expression engines agree on these simple patterns as far as I can tell, and I forced ASCII classes to match Go's; still, each example should be confirmed with a Go test before being treated as a reproduction.
- **Speeds are arithmetic.** B-05.1's table is derived from the loop's cadence and page size. No timing was taken. If steps routinely take longer than 300 ms the real numbers are worse; if I have misread the loop, they are better.
- **File-system behaviour** (device numbers changing across reboots, mergerfs directory times, timestamp granularity, inode stability on FUSE and SMB) is general knowledge, not tested on Justin's hosts.
- **Playback consequences** (B-05.3) were traced only as far as the source-preparation check; another auditor owns playback.
- **Jellyfin, Navidrome, Audiobookshelf** behaviour is from reading their source and docs at the cloned revisions, not from running them. **Plex** is from memory of public documentation.
- **Not examined:** Windows and macOS code paths beyond build tags; the rclone and WebDAV adapters beyond listing shape; DVR libraries; how each client surfaces scan state; the demo server's actual data.

## Outside my domain

- **Auditor 06 (metadata):** `LibraryLanguageOffered` makes "local agent + any language" an invalid create request (`httpapi/router.go:934-942`); the wrong-year reading feeds the matcher (B-05.7); IMDb id hints and folder hints are not read (A-05.29); NFO and OPF all-or-nothing budgets (B-05.29); the positive sidecar cache is keyed by path with no check that the file changed (`localmetadata/sidecars.go:112-135`).
- **Auditor 04 / playback:** playback refuses a file whose size or time differs from the catalogue (`playback/source_preparation.go:107`) with no on-demand re-observation that I found; analysis is at most one file per pass and pauses during playback.
- **Auditor 07 (database):** unbounded growth of `inventory_directories`, `inventory_analysis_queue`, `inventory_changes`, `inventory_runs`, `jobs` (B-05.20).
- **Settings/operations:** the media trash lives in the state folder (B-05.25).
