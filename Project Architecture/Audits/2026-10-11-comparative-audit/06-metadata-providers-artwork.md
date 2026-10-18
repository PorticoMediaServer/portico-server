# 06 — Metadata: providers, matching, refresh, locks, local metadata, people, collections, ratings and the artwork pipeline

> Comparative audit, 11 October 2026. Read-only source review. Nothing in this file was reproduced at runtime; every statement about behaviour is a source finding unless it says otherwise. Portico paths are relative to `/home/justin/projects/Portico`. Comparison paths are written `jellyfin/…`, `plezy/…` and so on.

## Scope and sources

**What I compared.** How a title gets its descriptive facts and its pictures: which online services are asked, how a file is matched to a record, when it is refreshed, what the owner can edit and lock, what is read from local files (NFO, tags, sidecar images), how people, collections and ratings are stored, and the whole image path from provider to pixels on a client.

**Portico files read in full or nearly so**

- Docs: `portico-internal/Project Architecture/03 Catalogue and Metadata.md`, `Backlog.md`, `portico-internal/Product Contracts/metadata/contract.md`.
- Provider transports (`apps/portico-server/server/internal/metadataprovider`): `http.go`, `conditional.go`, `screen.go`, `screen_cache.go`, `tmdb.go`, `tvdb.go`, `tvdb_screen.go`, `tvdb_artwork.go`, `tvdb_page.go`, `anilist.go`, `trending.go`.
- Matching and publishing (`…/internal/metadata`): `service.go`, `screen_base.go`, `screen_match.go`, `screen_worker.go`, `screen_publish.go`, `screen_review.go`, `screen_repair.go`, `screen_freshness.go`, `freshness.go`, `details.go`, `library_language.go`, `administration_policy.go`, `repair.go`, `repair_schema.go`.
- Artwork (`…/internal/metadata`): `artwork.go`, `artwork_discovery.go`, `artwork_integration.go`, `artwork_state.go`, `artwork_worker.go`, `artwork_encoding.go`, `artwork_upload.go` (first half), `page_artwork.go`, `tvdb_artwork.go`; `…/internal/imagework/gate.go`, `scale.go` (head); `…/internal/httpapi/artwork_response.go`, the image route in `metadata_repair.go`.
- Local metadata: `…/internal/localmetadata/video_parse.go`, `video.go`, `sidecars.go`, `audio.go`; `…/internal/assets/video_metadata.go`, `audio_tags.go`; `…/internal/catalog/local_artwork.go`, `catalog.go` (`FilenameTitle`), `collections.go` (head).
- Supporting: `…/internal/worker/worker.go`, `…/internal/entityid/entityid.go`, the relevant tables and triggers in `…/internal/persistence/migrations/0001_baseline.sql`, `0009`, `0010`.
- Clients: `apps/portico-server/packages/client-core/src/artwork/{buckets,scheduler,index}.ts`, `metadata-review.ts` (head), `apps/portico-server/web/src/ui/artwork-store.ts`, `artwork-path.ts`, `apps/portico-react-native/apps/native/src/ui/ArtworkPipeline.ts`.

**Portico files only skimmed (signatures, constants, greps)**: the music lane (`mb_*.go`, `music_*.go`, `musicbrainz.go`, `acoustid.go`, `music_artist.go`), the TVDB episode publication files (`tvdb_publication*.go`), `repair_fields.go`, `repair_bulk.go`, `repair_relationships.go`, `discovery.go`, `screen_links.go`, `enrichment/enrichment.go`, `catalog/people.go`, the web and native `MetadataEditor` screens, `media-presentation/contract.md`.

**Comparison code read**

- Jellyfin server: `Emby.Naming/Common/NamingOptions.cs` (date cleaning), `tests/Jellyfin.Naming.Tests/Video/CleanDateTimeTests.cs`, `MediaBrowser.Providers/Plugins/Tmdb/*` (`TmdbUtils.cs`, `TmdbClientManager.cs`, `Configuration/PluginConfiguration.cs`, `Movies/TmdbMovieProvider.cs`, the TV provider list), `MediaBrowser.Providers/Manager/MetadataService.cs` (merge and refresh decisions), `MediaBrowser.Model/Configuration/LibraryOptions.cs` and `TypeOptions.cs`, `MediaBrowser.Model/Entities/MetadataField.cs`, `MediaBrowser.XbmcMetadata/Parsers/BaseNfoParser.cs` and the parser/saver file lists, `MediaBrowser.LocalMetadata/Images/LocalImageProvider.cs`, `Jellyfin.Api/Controllers/{ImageController,ItemLookupController,RemoteImageController}.cs`, `src/Jellyfin.Drawing/ImageProcessor.cs`, `src/Jellyfin.Drawing.Skia/SkiaEncoder.cs`, `Emby.Server.Implementations/Library/Validators/*`, the built-in provider folders (Omdb, MusicBrainz, AudioDb, ListenBrainz, StudioImages, Books).
- jellyfin-web: component list (`metadataEditor`, `itemidentifier`, `imageeditor`, `imageDownloader`, `imageUploader`, `refreshdialog`). jellyfin-docs: `docs/general/server/plugins/index.mdx`.
- Plezy: `lib/metadata_edit/*`, `lib/services/image_cache_service.dart`, `lib/utils/media_image_helper.dart`, `lib/utils/tone_mapped_logo_image.dart` (heads and constants).
- Navidrome: `core/artwork/*` (file list, `resize.go`), `conf/configuration.go` (agents default). Audiobookshelf: `server/providers/*` (file list).

**Not reached**: Jellyfin's MusicBrainz/AudioDb/OMDb provider internals, its TVDB plugin (a separate repository, not cloned), Swiftfin/Android TV image loading, Portico's music matcher internals line by line, Portico's bulk-edit path, and the two `MetadataEditor` screens beyond confirming they exist. Closed products (Plex, Emby, Infuse, Kodi) are referenced from memory of public documentation only and are marked as such.

## How Portico does it today

### 1. Who is asked

A movie, TV or anime library has **one online provider** (the service that supplies titles, plots, cast and pictures). It is TMDB by default, for anime too; the owner may pick TheTVDB instead, or AniList for an anime library (`metadata/screen_review.go:153-155`, migrations `0009`, `0010`). The setting is stored as a list but the server refuses more than one entry (`screen_review.go:238-240`). A library can also be **local only**, which never goes online (`metadata/library_agent.go`).

Music uses MusicBrainz, with AcoustID audio fingerprints as an opt-in aid, Cover Art Archive for covers and Wikidata/Wikipedia/Wikimedia Commons for artist text and portraits. Books and audiobooks use only what is in the files.

Keys ship with the server: a TMDB read token (`metadata/project.go`, overridable only by the environment variable `PORTICO_TMDB_READ_ACCESS_TOKEN`, `cmd/server/main.go:712`) and a TheTVDB project key (`metadata/tvdb_project.go`, no override). Nothing online is asked until the owner confirms a one-time disclosure (`screen_review.go:16-17`, `screen_worker.go:103-104`).

### 2. The work queue

Every movie, show and episode has one row in `screen_metadata_work` holding its match state. Database triggers set a row back to `pending` when its file, its NFO, its library settings or its episodes change (`0001_baseline.sql:4197-4205`, `4517-4528`, `5562-5590`). One background loop (`metadata/service.go:109-161`) claims **one row per pass** and a pass runs at most once a second (`worker/worker.go:104`, `116-199`). Each provider has one transport that sends one request at a time, one second apart (`metadataprovider/http.go:43-81`).

### 3. Matching a title

The scanner stores a title and year parsed from the file name (`catalog/catalog.go:354-365`). The matcher builds up to four search strings from the NFO titles, that seed and up to four file names (`metadata/screen_match.go:68-122`), and collects **ID hints**: `[tmdb-123]`, `{tvdb=456}`, `[anilistid-7]` in a file name, or `<uniqueid type="tmdb">` / `<tmdbid>` in an NFO.

- An ID hint is fetched directly and accepted outright.
- Otherwise up to three searches are sent, the top four results are fetched in full (to get alternative titles), and each is scored (`screen_match.go:166-208`): an exact title or alias match is worth 0.64, a near match (edit-distance similarity of 0.92 or more) 0.58; an equal year adds 0.28; a different year is a **contradiction**.
- A title is accepted automatically only if it was fetched in full, has no contradiction, has **two strong signals** (title and year), scores at least **0.85**, and beats every different candidate by **0.12** (`screen_match.go:225-245`). Anything else is left as `needs_selection` (candidates stored for the owner) or `unmatched`.

### 4. Publishing what was found

The accepted provider document is stored whole (`screen_metadata_publications`), and each fact is written to `screen_metadata_fields` with where it came from, how confident, in which language and when (`screen_publish.go:406-422`). Facts the catalogue needs (title, year, overview, tagline, studio, network, content rating, genres, credits, tags, rating) are then written through the catalogue write API. Rules worth knowing:

- A field that came from an **NFO** is never replaced by a provider (`screen_publish.go:412`). For credits, the NFO keeps the departments it lists and the provider fills the rest (`:604-646`).
- A provider never clears a field: empty values are simply not written (`:306-370`).
- The library's refresh mode is `replace_unlocked` (default) or `fill_missing` (`0001_baseline.sql:2075`).
- A **different identity** (Fix Match, or a provider change) retires the old provider's facts first (`:186-236`).
- The content rating written is the one for the library's region **exactly**; no region, or no rating for that region, means none (`:375-386`).

### 5. Episodes

- **TMDB**: each episode is its own work row and its own request (`/tv/{id}/season/{s}/episode/{e}`, `metadataprovider/tmdb.go:364-429`). TMDB "episode groups" (alternative orderings) can be chosen per show. Episodes whose files are numbered absolutely (no season) stop at `needs_order` (`screen_publish.go:976-979`).
- **TheTVDB**: the show's whole episode list is fetched page by page in the chosen order (official, DVD, absolute, alternate, regional) and applied as one publication (`tvdb_publication*.go`).
- **AniList** names no episodes; an episode is only linked to the work, and a multi-season show needs the owner to map each local season to an AniList work (`screen_review.go:643-715`).

### 6. Refresh

There is **no timer**. A matched title is looked at again only when something wakes its row. When it is, the stored document is reused unless it is older than its **freshness window** (30 days for a film or episode, 14 for a show, 36 hours for a show still airing, 24 hours for anything added in the last week; `metadata/freshness.go:36-45`), there is **budget** left (240 refreshes an hour and 2,000 a day per provider; `:93-96`), and then the request is conditional (ETag), so an unchanged document costs no body (`metadataprovider/conditional.go`). A person pressing Refresh on a **movie** clears its freshness record (`metadata/details.go:30-54`).

Failures back off per row (30 s doubling), a provider's `Retry-After` is honoured and remembered in `metadata_provider_cooldowns`, and after six failed tries the row is parked as `unavailable` (`screen_worker.go:177-233`).

### 7. Owner editing

The editor's field list is published by the server per kind (`metadata/repair_schema.go:58-171`). Every field can be locked individually; credits, genres, the match itself and each artwork role have their own locks; there is lock-all and unlock-all. Every command carries the revision it was based on and is refused if the item changed meanwhile. The last 30 changes per item are kept and any of them can be undone (`metadata/repair.go:385-604`). Fix Match is "search (title, year, provider) → candidates → select" and is **queued** for the same background worker (`screen_repair.go:130-162`).

### 8. Local metadata

- **NFO**: `<file>.nfo`, `movie.nfo`, `tvshow.nfo` (file's folder and its parent). Parsed strictly as XML with size and count limits; read fields are title, original title, sort title, plot/outline, tagline, premiered/aired, year, edition, mpaa, runtime, season/episode/absolute number, genres, studios, the first `<set>`, TMDB/TVDB/AniList ids and actors (name and role) (`localmetadata/video_parse.go:49-229`, `video.go:22-39`).
- **Sidecar images**: movie `<name>-poster.jpg|png`, `poster.jpg|png`, `logo.png`; show `poster|folder`, `fanart|backdrop`, `logo.png`, `seasonNN.jpg`; music `cover|folder`, `artist`, `fanart|backdrop`; embedded cover art extracted with ffmpeg (`localmetadata/sidecars.go:30-92`, `audio.go:55-66`, `175-189`). Each is **copied** into `state/local-artwork`.
- **Embedded tags** are read for music and audiobooks (`assets/audio_tags.go`); a video container's title tag is deliberately ignored (contract §6).
- Nothing is ever written beside the owner's media (contract §6).

### 9. Artwork

1. **Candidates.** For each entity and role (poster, backdrop, logo, still, cover, portrait, banner, square, thumbnail) the server records candidate images: the poster/backdrop/still named in the provider document, local sidecars, uploads, and a provider image list (TMDB for **movies**: up to 30 posters, 20 backdrops, 10 logos and up to 100 cast/crew portraits; TheTVDB for **shows**: up to 80 images plus season posters and cast photos; Cover Art Archive for albums) (`metadata/artwork_discovery.go:28-188`, `413-555`, `page_artwork.go`).
2. **Download.** One job at a time per lane fetches the chosen image from an allow-listed host over HTTPS, with private addresses refused at connect time and an 8 MB limit (`artwork_worker.go:30-155`).
3. **Normalise.** JPEG and PNG only (uploads may be WebP). At most 10,000 px a side and 24 megapixels. The image is re-encoded to at most 1,920 px on its long edge and at most 500 KB (JPEG quality 90 → 82 → 75, then smaller; PNG kept for transparency), plus 800 px and 400 px copies. **The provider's original is not kept** (`artwork_worker.go:207-251`, `artwork_encoding.go`). One decode runs at a time server-wide (`imagework/gate.go:15-28`).
4. **Store.** Files are named by their SHA-256 in one folder; a table maps entity + role to the chosen file, with a lock flag and a revision (`artwork_selections`). Unreferenced files are collected after a day (`artwork_worker.go:631-742`).
5. **Serve.** `GET /v1/metadata/{kind}/{id}/art/{role}?w=400|800|1920&v=<digest>`; the caller must be signed in and allowed to see the entity; with `v` the answer is cacheable for a year and immutable (`httpapi/metadata_repair.go:102-190`, `artwork_response.go`).
6. **Clients.** A shared scheduler orders requests by screen position, limits concurrency, retries, and picks one of the three sizes from the rendered size × pixel density (`client-core/src/artwork`). The web fetches bytes with the bearer token and shows them as object URLs (`web/src/ui/artwork-store.ts`); the native app uses React Native's `<Image>` with an Authorization header (`native/src/ui/ArtworkPipeline.ts`). Placeholders are a tint derived from the title's id; the server publishes no dominant colour or blur preview yet (Backlog).

### 10. People, collections, ratings and extras

- **People** are keyed `provider:id` when a provider named them, otherwise `name:<folded name>`; a provider record keeps at most 128 credits with key crew reserved (`tmdb.go:499-562`). A person's portrait is an artwork selection **on a title** (`subject = tmdb:<person id>`), not on the person. The `catalog_people` table has biography and birth/death columns but nothing fills them for film and TV people.
- **Collections** are created by the owner, in movie libraries (`catalog/collections.go:57-88`). TMDB's "belongs to collection" and an NFO `<set>` are stored as evidence but create nothing.
- **Ratings**: the provider's community score and vote count (TMDB out of 10, AniList out of 100). No critic or IMDb score.
- **Genres, tags, studios, networks** are interned vocabulary terms per source; studio and network are one text value each on a title.
- Portico-only extras: provider "similar" lists and external ids feeding recommendations (`screen_links.go`), daily trending feeds (`discovery.go`), and a signed, downloadable Portico title dataset matched by provider id (`enrichment/enrichment.go`).

## How the others do it

### Jellyfin

**Providers are plug-ins in ordered chains.** Each library has, per item type (movie, series, season, episode, album…), an ordered list of metadata fetchers and a separate ordered list of image fetchers, each of which can be switched off (`jellyfin/MediaBrowser.Model/Configuration/TypeOptions.cs`: `MetadataFetchers`, `MetadataFetcherOrder`, `ImageFetchers`, `ImageFetcherOrder`, `ImageOptions`). Built in: TMDB, OMDb (IMDb ratings and Rotten Tomatoes critic score), MusicBrainz, TheAudioDB, ListenBrainz (similar artists), Studio Images, and on current master Google Books, ComicVine, ISBN and OPF for books (`jellyfin/MediaBrowser.Providers/Plugins/*`, `…/Books/*`). Official add-on plug-ins: TheTVDB, AniList, AniDB, AniSearch, Kitsu, Fanart, Bookshelf, TMDb Box Sets, Trakt (`jellyfin-docs/docs/general/server/plugins/index.mdx:44-226`); many third-party ones.

**Merging.** Local metadata (NFO) is read first, then each remote provider in order fills what is still empty; a "replace all" refresh lets remote values overwrite (`jellyfin/MediaBrowser.Providers/Manager/MetadataService.cs:873-917`). Nine field groups can be locked (Cast, Genres, ProductionLocations, Studios, Tags, Name, Overview, Runtime, OfficialRating; `jellyfin/MediaBrowser.Model/Entities/MetadataField.cs`), plus an all-or-nothing item lock.

**Matching is "best result wins".** The file name is cleaned and split into title and year with regular expressions that take the **last** year-looking number and require a title before it (`jellyfin/Emby.Naming/Common/NamingOptions.cs:146-150`; cases like `300 2001 (2006)` are tested in `jellyfin/tests/Jellyfin.Naming.Tests/Video/CleanDateTimeTests.cs:13-52`). `[imdbid-tt…]`, `[tmdbid-…]`, `[tvdbid-…]` in a name are used directly. Otherwise TMDB is searched and the result with the best small score wins (exact title 8, title-prefix 4, same year 2, year ±1 1); with no score at all, **the first result is taken** (`jellyfin/MediaBrowser.Providers/Plugins/Tmdb/TmdbUtils.cs:42-45`, `149-262`). An IMDb id is resolved through TMDB's "find" call (`…/Movies/TmdbMovieProvider.cs:150-185`). There is no "unmatched, please choose" state: a title either has provider ids or it does not.

**Identify is interactive.** The Identify dialog posts a name, year and any provider ids and gets results in the same request; applying one replaces the item's metadata (`jellyfin/Jellyfin.Api/Controllers/ItemLookupController.cs:88-245`).

**Refresh.** Per library, "refresh metadata automatically every N days" (`AutomaticRefreshIntervalDays`, default off; `MetadataService.cs:95-104`). On demand, for any item, folder or library: "scan for new and updated files", "search for missing metadata", or "replace all metadata", with a separate "replace all images" tick (`jellyfin-web/src/components/refreshdialog/refreshdialog.js:26-89`). TMDB calls are cached in memory for one hour and are **not throttled** by Jellyfin (`…/Tmdb/TmdbClientManager.cs:26`); MusicBrainz is throttled to its one-a-second rule (`…/MusicBrainz/Plugin.cs:46`).

**Keys.** A TMDB key is compiled into the server (`TmdbUtils.cs`, `ApiKey`) and the owner may enter their own (`TmdbClientManager.cs:44-49`).

**Language and country.** Per library and per item: preferred metadata language and country. Images are requested in "my language, no language, English" order (`TmdbUtils.cs:403-423`). A content rating missing for the chosen country falls back to the US one (`TmdbMovieProvider.cs:196-212`). Non-US ratings are stored with a country prefix (`TmdbUtils.cs:519`), and a table of rating systems maps them to ages.

**Local metadata.** NFO reading for movies, series, seasons, episodes, artists, albums and music videos, covering some sixty tags including ratings, critic rating, director, writer, countries, tags, trailers, actor thumbnails, stream details, lock flags and watched state (`jellyfin/MediaBrowser.XbmcMetadata/Parsers/BaseNfoParser.cs:271-966`). It tolerates a URL after the closing XML tag, which Kodi allows (`:123-175`). It can also **write** NFO files and save artwork into the media folders (`…/Savers/*`, `LibraryOptions.SaveLocalMetadata`, `MetadataSavers`). Local images are found under many names: poster, folder, cover, default, fanart, backdrop, art, extrafanart, logo, clearlogo, clearart, banner, landscape, thumb, disc, `seasonNN-poster` (`jellyfin/MediaBrowser.LocalMetadata/Images/LocalImageProvider.cs:24-60`, `198-248`, `332-344`, `403-432`). Options exist to use titles embedded in files and episode info embedded in files (`EnableEmbeddedTitles`, `EnableEmbeddedEpisodeInfos`).

**Images.** Originals are kept (in the server's metadata folder or beside the media). The image endpoint resizes on request to any size, with quality, format (WebP, JPEG, PNG; SVG passes through), blur, background colour, played-percentage and unplayed-count overlays, and caches each result on disk (`jellyfin/Jellyfin.Api/Controllers/ImageController.cs`; `jellyfin/src/Jellyfin.Drawing/ImageProcessor.cs:228-248`; `jellyfin/src/Jellyfin.Drawing.Skia/SkiaEncoder.cs:100-101`). Encoding uses the native Skia library with a configurable number of parallel encodes (`ImageProcessor.cs:68-74`). Every stored image gets a **BlurHash** (a ~30-character string a client can turn into a blurred preview) (`ImageProcessor.cs:408-431`). Image GETs need no sign-in: knowing the item id is enough; a `tag` in the URL makes the response immutable (`ImageController.cs:2008-2058`). An item may have several backdrops. The owner can browse every provider's images for an item, filter by type and language, download one, upload one, delete and reorder (`jellyfin/Jellyfin.Api/Controllers/RemoteImageController.cs:62-151`; jellyfin-web `imageeditor`, `imageDownloader`, `imageUploader`).

**People, collections, the rest.** People are items with their own refresh: biography, birth and death, birthplace, images (`…/Tmdb/People/TmdbPersonProvider.cs`), cleaned up by a scheduled task (`jellyfin/Emby.Server.Implementations/Library/Validators/PeopleValidator.cs`). TMDB cast is capped at 15 and crew at 15 by default (`…/Tmdb/Configuration/PluginConfiguration.cs`). A library option adds movies to a collection named by TMDB automatically (`…/Validators/CollectionPostScanTask.cs:53`). TMDB on current master also creates placeholder items for missing and upcoming episodes (`…/Tmdb/TV/TmdbMissingEpisodeProvider.cs`, `TmdbUpcomingEpisodesTask.cs`). Specials can be placed before or after a season or episode (`jellyfin/MediaBrowser.Controller/Entities/TV/Episode.cs:37-59`).

### Plezy

Plezy is a client, so it owns no providers. Two things are relevant:

- **A metadata editor that fronts Plex's and Jellyfin's editing APIs** behind one adapter interface, with text, date, list, choice and artwork fields and an artwork picker grid (`plezy/lib/metadata_edit/*`, `plezy/lib/screens/metadata_edit_screen.dart`).
- **A careful image loader**: requested sizes are rounded up to 40 px steps between 160 and 1,920 px and scaled by the device's pixel density and a performance tier (`plezy/lib/utils/media_image_helper.dart:44-52`, `107-124`); a disk cache of 3,000 objects with a 14-day staleness period (`plezy/lib/services/image_cache_service.dart:51-52`); a request limiter (`:237-306`); images decoded at display size, not full size (`media_image_helper.dart:370-373`); and logos recoloured when a white logo would vanish on a light background (`plezy/lib/utils/tone_mapped_logo_image.dart`).

### Navidrome (Go + SQLite music server)

Tags in the files are the truth; online "agents" only add artist biographies, images, similar artists and top songs, in a configured order (default `deezer,lastfm,listenbrainz`; `navidrome/conf/configuration.go:1084`). Its artwork package has cover priority lists (`CoverArtPriority`, `ArtistArtPriority`), optional WebP output with a quality setting (`navidrome/core/artwork/resize.go:120-125`), a sized on-disk image cache, optional pre-caching, and BlurHash, ThumbHash and dominant-colour placeholders (`navidrome/core/artwork/{blurhash,thumbhash,dominant}`).

### Audiobookshelf

Book and audiobook matching against Audible, Audnexus, Google Books, Open Library, iTunes, FantLab, MusicBrainz and owner-defined custom providers (`audiobookshelf/server/providers/*`), with a "quick match" per item and per library.

### Plex and Kodi (public documentation, from memory; not verified here)

Plex moved from agent chains to one agent per library, has an "Unmatched" filter, Fix Match with a title/year/agent search, per-field lock icons, periodic library metadata refresh, automatic collections with a minimum size, and an image transcoder with BlurHash-style colours. Kodi defined the NFO format and the local artwork names most libraries on disk follow.

## Architectural differences

### A-06.1 One provider per library, versus ordered chains with per-field merge
- **Portico:** exactly one online provider for a movie/TV/anime library (`metadata/screen_review.go:238-240`). Changing it sends every title back to be matched again (`:165-183`).
- **Jellyfin:** an ordered chain per item type, metadata and images separately; later providers fill what earlier ones left empty.
- **Why it matters:** one provider means one identity, one language policy, one attribution and no "which source said this?" puzzles. It also means no gap-filling: a title TMDB lacks stays bare; ratings from a second source (IMDb, Rotten Tomatoes) and artwork from a second source (Fanart) have nowhere to plug in; and what a provider lacks as a whole is lacking for the whole library (see B-06.7: TMDB TV art, TheTVDB movie art).
- **Assessment:** **Trade-off (subjective)**, deliberate (`03 Catalogue and Metadata.md`, 2 Oct 2026; note the contract still says "TheTVDB fallback", see Doc/code). I think one *identity* provider is right, and that the cost is paid on *supplementary* data. A narrower rule would keep the benefit: one provider decides identity and text; separate, typed supplement slots (critic ratings, extra artwork, person biographies) may be filled by other sources through the provider-id crosswalk Portico already stores. Medium: touches `screen_publish.go`, the artwork discovery table and the settings page. I would change my view if Justin wants zero second-source data on principle.

### A-06.2 Conservative automatic matching, versus "best result wins"
- **Portico:** two strong signals, 0.85, 0.12 margin, no contradiction (`metadata/screen_match.go:225-245`). **A title with no year can never match automatically** (the test `TestScreenConfidenceRequiresVerifiedDetailsSignalsAndMargin` asserts it, `screen_match_test.go:52-56`). A year that differs by one is a contradiction.
- **Jellyfin:** takes the best-scoring search result, or the first result (`TmdbUtils.cs:193-226`); year ±1 still scores.
- **Why it matters:** Portico will almost never show the wrong film. It will also leave far more titles for the owner. Most TV folders are named `Show Name` with no year, so on a typical TV library the default outcome is "needs selection" for most shows (and no episode data until the show is chosen, since episodes wait on the parent). Jellyfin's failure mode is the opposite: a confident wrong match the owner has to notice.
- **Assessment:** **Trade-off (subjective)**, deliberate (contract §3.2). In my opinion the rule is too strict *for shows* and for the year-off-by-one case, and it becomes a real product problem combined with A-06.51 (there is no list of what needs attention). Options, smallest first: treat "exact title + the only result with that exact title" as sufficient for shows; accept year ±1 as a weaker signal instead of a contradiction; add an owner setting ("match automatically when unsure: never / when there is one clear result / always take the top result"). Small to medium, all inside `screen_match.go`. See B-06.2.

### A-06.3 Matching and Fix Match are queued jobs, versus an interactive request
- **Portico:** a search is written to the work row and performed by the background worker; the client polls for candidates (`metadata/screen_repair.go:130-162`). Provider calls never happen inside a web request.
- **Jellyfin:** the Identify dialog calls the providers during the request and shows results at once.
- **Why it matters:** Portico's requests can never hang on a provider, and every search leaves durable evidence. But the owner's search waits its turn behind whatever the single worker is doing (B-06.13).
- **Assessment:** **Trade-off**; Portico's separation is sound engineering but needs a fast lane for a person who is waiting. Small: order `searching` rows first in `claimScreen` (`screen_worker.go:32`).

### A-06.4 One request a second, one title a second, versus unthrottled provider calls
- **Portico:** each provider transport sends one request at a time, one second apart (`metadataprovider/http.go:43-81`), and the metadata loop handles one work row per pass with at least a second between passes (`worker/worker.go:104`). A new movie costs up to three searches and four detail fetches (`screen_worker.go:448-515`), then an image-list fetch and the image downloads, which run in their own lanes at one job per pass.
- **Jellyfin:** no pacing for TMDB (TMDB's published guidance is roughly 50 requests a second per IP address; from public documentation, not re-verified today); library scans refresh several items in parallel.
- **Why it matters:** see B-06.11 for arithmetic. In short, a 2,000-film library takes hours and a 100,000-episode library takes days before it has titles and pictures, and Portico's stated scale target (millions of items) is out of reach for first import at this pace. Portico's own comment says so (`metadata/freshness.go:14-17`).
- **Assessment:** **Portico behind** on throughput, **ahead** on politeness and predictability. The one-a-second rule is MusicBrainz's rule applied to everyone. A per-provider pace (TMDB perhaps 10–20 a second with a few connections; TheTVDB a few; AniList and MusicBrainz as now) and several rows per pass would be a small change in `http.go`/`service.go`, with the durable back-off already in place to absorb a 429.

### A-06.5 A durable evidence model, versus overwrite in place
- **Portico:** candidates, accepted publications (with the previous one linked), per-field provenance, per-row attempts and errors are all tables (`0001_baseline.sql:2076-2110`). Writers are fenced by a lease, a generation counter and a digest of everything they read (`screen_worker.go:128-149`), so a slow provider answer can never overwrite a newer owner decision.
- **Jellyfin:** the item row is the only record; a refresh overwrites it. Concurrent refreshes of one item are serialised by a lock, not fenced.
- **Why it matters:** Portico can say why a title shows what it shows, can undo, and survives restarts mid-match. The cost is complexity (several thousand lines) and more database writes per title.
- **Assessment:** **Portico ahead.** Keep.

### A-06.6 Freshness windows, a refresh budget and conditional requests
- **Portico:** described in "Refresh" above (`metadata/freshness.go`, `screen_freshness.go`, `metadataprovider/conditional.go`).
- **Jellyfin:** a one-hour in-memory cache; no conditional requests; no budget.
- **Why it matters:** on a large library a settings change or rescan wakes every row; without this Portico would re-download everything. With it, almost nothing is asked.
- **Assessment:** **Portico ahead** in design. Two side effects are bugs today: a language change is not a reason to refetch (B-06.9), and a person's Refresh on a show cannot get past the window (B-06.10). The freshness key should include the language, and "a person asked" should always bypass it.

### A-06.7 No scheduled refresh
- **Portico:** nothing re-reads a matched title on a clock. A row is woken only by a local change or an owner action.
- **Jellyfin:** optional per-library interval; a scheduled library scan; scheduled people refresh.
- **Why it matters:** community ratings, corrected plots, new artwork and a show's status ("Ended") stay as first fetched, indefinitely, unless a file changes. An airing show is re-read only when a new episode file arrives (which is when it matters most, so this is mostly fine).
- **Assessment:** **Trade-off**, consistent with "no time-based caches" and the budget design. I would add a slow background trickle that spends *unused* refresh budget on the stalest documents (the budget table already bounds it). Small.

### A-06.8 A consent gate before the first provider call
- **Portico:** no title leaves the server until the owner confirms a disclosure; consent is revisioned and can be withdrawn (`screen_review.go:16-17`, `254-260`).
- **Jellyfin / Plex:** providers run as soon as a library exists.
- **Assessment:** **Portico ahead** on privacy; costs one click. Deliberate (contract §13).

### A-06.9 Shipped provider credentials
- **Portico:** a TMDB read token and a TheTVDB project key are compiled in; TMDB's can be replaced only by an environment variable; TheTVDB's cannot be replaced.
- **Jellyfin:** a TMDB key is compiled in too, with a settings field to replace it. TheTVDB is a separate plug-in.
- **Why it matters:** (1) *Rate limits*: as far as I know TMDB limits by IP address, not by key, so thousands of installs on one key do not share an allowance. (2) *Revocation*: one abusive install, or a terms dispute, can get the shared key disabled for every install at once; with no in-product override the only fix is a new server release. (3) *Terms*: TMDB's and TheTVDB's free tiers are for non-commercial use with attribution; a paid relay later may change how they classify Portico. TheTVDB in particular licenses per project and has a subscriber-PIN model for end users. (4) AniList's terms and its rate limit (30–90 requests a minute) also apply to the shared User-Agent.
- **Assessment:** **Same approach as Jellyfin**, slightly more exposed because there is no owner override in settings. Suggest: an owner-entered key per provider in Server settings (small), and a written terms check before release (contract §10.2 already requires a provider-policy record). All the terms statements here are from memory and must be checked against the providers' current pages.

### A-06.10 Year parsing takes the first year-looking number
- **Portico:** both the scanner (`catalog/catalog.go:354-365`) and the matcher (`metadata/screen_match.go:16`, `55-58`) cut the name at the **first** `18xx/19xx/20xx` token. The matcher's pattern also matches at the very start of the name.
- **Jellyfin:** greedy match, so the **last** year wins, and a title must precede it.
- **Why it matters:** films and shows whose titles contain or are a year. See B-06.1.
- **Assessment:** **Portico behind** (a bug, not a choice).

### A-06.11 Which ID hints are understood
- **Portico:** `tmdb`, `tvdb`, `anilist` in `[]` or `{}` in file names (`screen_match.go:21`), the same three in NFO `<uniqueid>`/`<…id>`. **IMDb ids are ignored** everywhere (`localmetadata/video_parse.go:177-179`). For a movie only the file's own name is inspected, not its folder (`screen_base.go:221-227`). For a show the folder name is the seed, but the scanner cuts it at the year, so `Show (2008) [tvdbid-123]` loses the id before the matcher sees it (`catalog/catalog.go:354-365`).
- **Jellyfin:** `imdbid`, `tmdbid`, `tvdbid` in file or folder names; NFO ids of every registered provider; a trailing IMDb/TMDB URL in an NFO.
- **Why it matters:** `{imdb-tt…}` is the default folder naming of Radarr and is common in NFOs written by older tools. Those libraries would match perfectly by id and instead go through name matching.
- **Assessment:** **Portico behind.** Small: TMDB's `/find/{imdb id}` gives the TMDB id; TheTVDB has a remote-id search. See F-06.1.

### A-06.12 A different year is a hard contradiction
- **Portico:** any year difference blocks automatic acceptance (`screen_match.go:194-202`). If the scanner's seed year, an NFO year and a file-name year disagree with each other, **every** candidate is marked contradictory (`:260-277`, `screen_worker.go:516-521`).
- **Jellyfin:** ±1 year still scores; Plex (from memory) also tolerates a year either side.
- **Why it matters:** festival year versus release year, and regional release years, differ by one fairly often.
- **Assessment:** **Trade-off**; I would soften to "±1 is a weak signal, not a contradiction, provided no other candidate matches the exact year".

### A-06.13 Title comparison
- **Portico:** lower-cases, strips accents and punctuation, then compares by edit distance against the title, original title and every alternative title (`screen_match.go:24-37`, `140-165`, `176-181`). No handling of `&`/`and`, roman numerals, or a trailing article ("Office, The").
- **Jellyfin:** trusts TMDB's own search ranking, then exact or whole-word prefix on title and original title only.
- **Assessment:** **Portico ahead** on alias coverage (it fetches alternative titles for the top four), **behind** on a few common normalisations. Small additions.

### A-06.14 Episodes: one work row and one request per episode (TMDB)
- **Portico:** TMDB episodes are fetched one by one (`tmdb.go:364-429`); with an episode-group order it is three calls per episode (show details, group, episode) softened by a 64-entry, five-minute response cache (`screen_cache.go`). TheTVDB episodes come in pages of the whole list.
- **Jellyfin:** also one call per episode for TMDB, cached for an hour, unthrottled.
- **Why it matters:** at one request a second, a 10,000-episode library is about three hours of episode calls alone. TMDB's season endpoint returns every episode of a season in one response.
- **Assessment:** **Portico behind** only because of pacing. Fetching by season (one request per season, fan the episodes out from it) would cut requests by roughly 10–20× and fits the existing "pending children" step. Medium.

### A-06.15 Episode order is an explicit, per-show choice
- **Portico:** the chosen order is stored on the show (`episode_order`), must be one the provider document lists, and changing it re-dispatches the episodes (`screen_review.go:534-548`, `screen_worker.go:381-389`). TMDB episode groups, TheTVDB's six season types and AniList absolute/seasonal are all supported. Local episode numbers are never renumbered by a provider.
- **Jellyfin:** a free-text `DisplayOrder` on the series that providers interpret.
- **Assessment:** **Portico ahead** in rigour. Not yet a library-level default (Backlog: "Per-show settings not built").

### A-06.16 Absolute-numbered files need TheTVDB or AniList
- **Portico:** with TMDB, an episode whose local numbering is absolute ends at `needs_order` (`screen_publish.go:976-979`).
- **Jellyfin:** similar limits with TMDB alone; anime users install AniDB/AniList/TVDB plug-ins and mapping lists.
- **Why it matters:** TMDB became the default for anime libraries on 2 Oct, and absolute numbering (`[Group] Show - 137.mkv`) is the norm for anime releases. See B-06.21.
- **Assessment:** **Portico behind** for its own default. TMDB's "absolute" episode group, or simply counting through the official seasons in order, would map most cases. Medium.

### A-06.17 Specials placement
- **Portico:** season 0 is a season; nothing says where a special sits among regular episodes.
- **Jellyfin:** `AirsBeforeSeasonNumber`, `AirsAfterSeasonNumber`, `AirsBeforeEpisodeNumber`, read from NFO and TheTVDB, used for ordering and Next Up.
- **Assessment:** **Portico behind**, small feature (F-06.20).

### A-06.18 Language and region
- **Portico:** one language tag and one optional region per library (`0001_baseline.sql:2074`: default `en-US`, region empty). TMDB is asked in that language and returns its own fallbacks; TheTVDB text is replaced only by an exact language match, otherwise the original-language text stays (`tvdb_screen.go:171-183`); AniList offers English, romaji or native (`anilist.go:45-59`). Content rating: exact region or nothing. Image candidates: language recorded, not used.
- **Jellyfin:** language and country per library and per item; image language order "mine, none, English"; US rating fallback; region-specific language quirks handled (`TmdbUtils.cs:432-470`).
- **Why it matters:** with the default empty region no title gets a content rating at all (B-06.14). The contract's six-step fallback order (§5.1) is not implemented.
- **Assessment:** **Portico behind.** Refusing to show a foreign rating as local is defensible (contract §5.2), but then the default region should be derived from the language tag or asked at library creation, and a clearly labelled fallback ("US: PG-13") would serve parental controls better than nothing.

### A-06.19 Precedence between local files and the provider
- **Portico:** per field, an NFO value wins outright and the provider value is not even recorded for that field (`screen_publish.go:412`); for credits the split is by department. There is no `off`/`supplement`/`prefer` switch for video libraries (the contract's §6 modes exist only for music, `metadata/music_policy.go`).
- **Jellyfin:** local readers first, remote fills gaps; "replace all" overrides; local readers can be disabled or reordered per library.
- **Assessment:** **Trade-off.** Portico's rule is simple and predictable; the missing piece is the owner's choice (some people keep stale NFOs they do not want to win). Small to medium.

### A-06.20 Locks, history and undo
- **Portico:** every editable field has its own lock; credits, genres, identity and each artwork role too; 30-step history with undo, each undo a new revision (`metadata/repair.go:411-560`).
- **Jellyfin:** nine lockable field groups and a whole-item lock; no history.
- **Assessment:** **Portico ahead.** Worth advertising.

### A-06.21 Optimistic concurrency on edits
- **Portico:** every edit carries the 64-character revision it saw and is refused if anything changed (`repair.go:398-408`).
- **Jellyfin:** last write wins.
- **Assessment:** **Portico ahead.**

### A-06.22 The editor's field list comes from the server and is narrower
- **Portico:** fields per kind from `repair_schema.go:58-171`: title, sort title, metadata language, original title, edition, tagline, description, year, release date, content rating, one studio, country, tags, labels (plus network for shows, numbers for episodes and songs, author/narrator/series for books).
- **Jellyfin:** hard-coded form with many more: community and critic rating, end date, status, air days and time, runtime, parental rating and custom rating, multiple studios, people with types and roles, external ids for every provider, display order, preferred language and country per item, 3D format, aspect ratio.
- **Assessment:** Server-published schema is **Portico ahead** (new fields need no client release). Coverage is **Portico behind** (F-06.28).

### A-06.23 NFO reading is strict and narrow; no NFO writing
- **Portico:** strict XML; any stray text, one bad number, or more than 100 actors rejects the whole file (`localmetadata/video_parse.go:54-65`, `104-106`, `39-48`, `214-216`). About twenty tags read. Never written (contract §6).
- **Jellyfin:** tolerant reader, ~60 tags, URL-after-XML, and savers.
- **Why it matters:** people arrive from Kodi, Jellyfin, Emby and tinyMediaManager with years of curated NFOs. A reader that drops a file for one odd field loses that curation silently (B-06.16), and ratings, directors, writers, tags and countries are not read even from good files.
- **Assessment:** **Portico behind** on reading; not writing is a **deliberate trade-off** that I think is right as a default (an export is listed in the contract as a future explicit operation).

### A-06.24 NFO file names looked for
- **Portico:** `<file>.nfo`, `movie.nfo`, `tvshow.nfo` (two levels).
- **Jellyfin:** also `<folder name>.nfo`, `VIDEO_TS.nfo`, `season.nfo`, `artist.nfo`, `album.nfo`, music-video NFOs.
- **Assessment:** **Portico behind**, small.

### A-06.25 Local artwork names recognised
- **Portico:** the short list in "Local metadata" above. Movies have no local backdrop name at all; episodes have no local still (`-thumb.jpg`); season posters only as `seasonNN.jpg`/`.png`; only JPEG and PNG.
- **Jellyfin / Kodi convention:** `fanart.jpg`, `<name>-fanart.jpg`, `backdrop.jpg`, `<episode>-thumb.jpg`, `seasonNN-poster.jpg`, `season-all-poster.jpg`, a `poster.jpg` inside the season folder, `banner`, `clearlogo`, `clearart`, `landscape`, `disc`, `extrafanart/`, WebP and more.
- **Assessment:** **Portico behind**; also see B-06.5 (a season folder's `poster.jpg` is taken for the show).

### A-06.26 Local artwork is copied into a capped cache
- **Portico:** each sidecar or embedded cover is copied to `state/local-artwork/<sha256>.img`; the folder has a hard 128 MB total limit and is never pruned (`localmetadata/audio.go:111-150`). The artwork worker then makes its own normalised copies in `state/artwork`.
- **Jellyfin:** local images are used in place by path; only resized variants are cached.
- **Assessment:** **Portico behind**: the copy is reasonable for remote storage, but the cap is a bug at any real size (B-06.4), and each local image is stored up to four times (local copy, 1920, 800, 400).

### A-06.27 Embedded information in video files
- **Portico:** a video file's embedded title is ignored (deliberate, contract §6); no use of embedded cover attachments in MKV; I found no "grab a frame" fallback for a film or episode with no picture (trickplay tiles are a different feature).
- **Jellyfin:** optional embedded titles and episode info; embedded image provider; a screenshot provider that makes a still from the video when nothing else exists.
- **Assessment:** **Portico behind** on the frame-grab fallback, which matters most for local-only libraries and home videos (F-06.26).

### A-06.28 People
- **Portico:** identity `provider:id` or `name:<folded>`; no merging of a TMDB person with a TheTVDB person or an NFO name; up to 128 credits a title; portraits attached to titles; no biography, dates or birthplace for film/TV people; for a TMDB show the cast is TMDB's plain `credits` (the most recent season), and the aggregate-credits reader exists but is no longer called (`tmdb.go:435-497`).
- **Jellyfin:** people are first-class items with their own provider refresh and images; 15 cast + 15 crew by default (configurable); aggregate credits for series.
- **Assessment:** Mixed. Portico keeps far more credits and never merges on name alone (good, contract §9). It is **behind** on person pages (F-06.6) and on show cast (B-06.8).

### A-06.29 Collections
- **Portico:** manual only, movie libraries only.
- **Jellyfin:** manual, plus automatic from TMDB collection, plus NFO `<set>`; collections have their own metadata and images (TMDB Box Sets).
- **Assessment:** **Portico behind** (F-06.5). The evidence is already captured in every TMDB record.

### A-06.30 Genres, tags, studios, networks
- **Portico:** interned terms per source, so a filter is an indexed join; keywords kept separate from genres; AniList spoiler and adult tags dropped at the door (`anilist.go:183-206`). Studio and network are single text columns: only the first production company and first network are kept as facts (`screen_publish.go:346-355`, `1111-1121`). No cross-provider genre mapping ("Sci-Fi & Fantasy" vs "Science Fiction").
- **Jellyfin:** plain string lists; all studios; genres and studios are browsable items with images.
- **Assessment:** Storage **Portico ahead**; "one studio" **Portico behind** (co-productions lose all but the first company, so "A24" finds only the films where it is listed first).

### A-06.31 Ratings
- **Portico:** provider community score with vote count and scale; nothing else.
- **Jellyfin:** community rating plus critic rating (Rotten Tomatoes via OMDb), IMDb rating via OMDb.
- **Assessment:** **Portico behind** if Justin wants critic or IMDb scores (F-06.17); note that OMDb needs a key and has a 1,000-a-day free limit.

### A-06.32 Trailers, theme music, extras
- **Portico:** TMDB's video list is not requested (`tmdb.go:205-210`), so there are no remote trailers; local extras belong to another audit.
- **Jellyfin:** remote trailer URLs from TMDB, local trailers and extras, theme songs and videos.
- **Assessment:** **Portico behind**, optional (F-06.18).

### A-06.33 Originals are discarded; one normalised master is kept
- **Portico:** at most 1,920 px and 500 KB per stored image; provider originals never kept (`artwork_worker.go:238-239`).
- **Jellyfin:** keeps the original file; resizes from it on demand.
- **Why it matters:** Portico's artwork folder is small and bounded (roughly 0.3–0.7 MB per image including the two smaller copies). On a 4K television a full-screen backdrop is 1,920 px stretched to 3,840 px, and 500 KB at 1,920×1,080 is moderate JPEG quality; a later change of mind (larger masters, a new format) means downloading everything again.
- **Assessment:** **Trade-off**, deliberate. I would raise the master for backdrops to 3,840 px with a larger byte ceiling, at least as a setting, since 4K TVs are the reference client's main screen. Small change, larger disk use.

### A-06.34 Three fixed sizes, versus any size on demand
- **Portico:** 400, 800 and 1,920 px on the long edge; 800 and 400 are made at download time, anything missing is made on first request and stored (`artwork.go:122-169`, `222-266`). JPEG or PNG out.
- **Jellyfin:** any width/height/fill, quality, WebP, blur, overlays; each distinct request cached.
- **Why it matters:** three sizes keep the cache small and make browser caches hit across screens. But the smallest is 400 px, so a 60-px-wide list thumbnail still downloads a 400-px image; and JPEG-only costs roughly 25–35% more bytes than WebP at the same quality.
- **Assessment:** **Trade-off.** I would add one smaller size (about 200 px) and WebP output where the client says it accepts it (F-06.9, F-06.10).

### A-06.35 Pure-Go image code with hard limits, versus a native library
- **Portico:** Go's standard decoders, a custom memory-bounded resampler (`imagework/scale.go`), one decode at a time for the whole server, 256 waiters then "busy" (`imagework/gate.go:15-58`), 24 MP and 10,000 px limits checked on the header before decoding (`artwork_worker.go:216-222`), 8 MB download and 10 MB upload limits, formats sniffed from bytes not names (`artwork_upload.go:27-37`), remote fetches restricted to named hosts with private addresses refused at connect time and redirects re-checked (`artwork_worker.go:35-155`).
- **Jellyfin:** Skia (native, fast, more formats including SVG with a safety validator), configurable parallel encodes.
- **Why it matters:** Portico is safe against decompression bombs and server-side request forgery, and safe on weak hardware because memory use is bounded. It is slow: one image at a time, in a pure-Go scaler, and both the artwork worker and an on-demand resize for a waiting viewer queue for the same single slot.
- **Assessment:** **Portico ahead on safety, behind on speed.** A small pool sized to CPU count (two to four) with the same pixel limits would help without giving up the bounds; and on-demand resizes for viewers should not wait behind background downloads.

### A-06.36 Image URLs and who may fetch them
- **Portico:** `/v1/metadata/{kind}/{id}/art/{role}`, sign-in required, library access and content restrictions checked per request, `v=<digest>` for immutable caching, a stale `v` answers "gone" (`httpapi/metadata_repair.go:102-190`).
- **Jellyfin:** images are public by item id.
- **Why it matters:** a restricted profile cannot fetch a poster it may not see, and a URL cannot be shared. The cost is several database reads per image and the need to send a token, which is why the web app downloads images with `fetch` and shows object URLs instead of plain `<img src>` (no native lazy loading, bytes held in script memory, capped at 48 MB).
- **Assessment:** **Portico ahead** on privacy; watch the per-image database cost on a grid of 60 posters. A short-lived signed image token or a cookie scoped to the image path would allow plain `<img>` and browser-managed memory. Medium.

### A-06.37 Placeholders
- **Portico:** a tint from the id (Backlog lists "a title's dominant colour").
- **Jellyfin:** BlurHash for every image, delivered with the item. **Navidrome:** BlurHash, ThumbHash and dominant colour.
- **Assessment:** **Portico behind** (F-06.8). The image is already decoded at ingest, so computing a colour or hash there is nearly free.

### A-06.38 One image per role, chosen through a candidate/selection model
- **Portico:** candidates → preview → select (which also locks) → download job; upload; per-role lock; restore through undo. One selection per entity + role (+ subject for portraits). Up to 200 candidates listed (`artwork_state.go:118`).
- **Jellyfin:** several backdrops per item with an index; browse all remote images with type/language filters; delete and reorder.
- **Assessment:** Model **Portico ahead** (fenced, undoable, auditable); breadth **Portico behind** (no multiple backdrops, no delete-to-none, no language filter).

### A-06.39 Image language is recorded but not used to choose
- **Portico:** the poster and backdrop named in the provider document come in the library language because the document request carries it. Everything from the image list (logos, alternates) is taken in the provider's order; the first logo wins whatever its language (`artwork_discovery.go:362-397`).
- **Jellyfin:** language-ordered request and sort.
- **Assessment:** **Portico behind** (B-06.19).

### A-06.40 Always fetching TMDB "original" size
- **Portico:** every TMDB image, cast portraits included, is fetched at `original` size and then shrunk (`metadata/service.go:103-108`, `artwork_discovery.go:464`).
- **Jellyfin:** configurable size per image type (default original for saved images, `w500`/`w780` for previews).
- **Why it matters:** originals are several megabytes for posters and backdrops; TMDB offers `w780`, `w1280`, `h632` and similar that are already near Portico's targets. Portraits end up at 400 px on a card.
- **Assessment:** **Portico behind** on bandwidth and TMDB load: fetch `w1280`/`original` for backdrops, `w780` for posters, `h632` for portraits. Small.

### A-06.41 Artwork sources
- **Portico:** TMDB image list for movies only; TheTVDB image list for shows only; Cover Art Archive; Wikimedia Commons; local; upload.
- **Jellyfin:** TMDB for everything it has (series, seasons, episodes, people, collections), TheTVDB, Fanart, AudioDB, Studio Images, embedded, screenshots.
- **Assessment:** **Portico behind**; the TMDB TV gap is a bug given TMDB is the TV default (B-06.7).

### A-06.42 Client image loading
- **Portico:** shared scheduler with visual ordering, row-by-row fill, a priority lane for the hero image, retry with back-off, drop of off-screen work (`client-core/src/artwork/scheduler.ts`); web keeps 600 object URLs / 48 MB (150 / 16 MB on low-memory devices); native relies on React Native's own cache and adds a cache-buster on retries.
- **Plezy:** 40-px size steps with a device-tier budget, a managed disk cache (3,000 objects, 14 days), decode-at-display-size, logo tone mapping.
- **Jellyfin web:** lazy loader with BlurHash canvases.
- **Assessment:** Scheduling **Portico ahead** of everything I read. **Behind** on a managed on-device disk cache for native (behaviour is whatever the platform defaults are, which I could not establish from source) and on placeholders.

### A-06.43 Portraits belong to titles, not people
- **Portico:** the same actor's photo is a separate candidate, job and selection on every title they appear in; the bytes are stored once because files are named by content.
- **Jellyfin:** one image per person item.
- **Why it matters:** download count grows with credits, not with people (B-06.20), and a person with only NFO credits has no photo (Backlog).
- **Assessment:** **Portico behind.** A person-level selection keyed by `provider:id`, filled once, is the natural fix. Medium (touches `artwork_discovery.go`, `catalog/people.go`, the portrait route).

### A-06.44 Music
- **Portico:** MusicBrainz with the same confidence/margin model as video (0.85 / 0.12, `metadata/music_scoring.go:14-15`), AcoustID as opt-in evidence, typed release/recording/artist-credit evidence tables, embedded tags preferred by default (`local_mode = prefer`), Cover Art Archive covers, artist text from Wikipedia (English only, Backlog) and portraits from Commons.
- **Jellyfin:** MusicBrainz + TheAudioDB (artist/album text and images) + ListenBrainz (similar artists). **Navidrome:** tags are the truth; Last.fm, Deezer and ListenBrainz for artist extras.
- **Assessment:** Identity handling **Portico ahead** (fingerprints, typed credits, no silent overwrite). Artist images and biographies **Portico behind** in coverage: Commons has photos for far fewer artists than Deezer/Last.fm/AudioDB/Fanart, and only English text.

### A-06.45 Books and audiobooks
- **Portico:** local evidence only (deliberate, contract §11.2). Backlog notes an unmerged "Audiobooks agent".
- **Audiobookshelf:** Audible/Audnexus, Google Books, Open Library, iTunes, custom providers. **Jellyfin:** Google Books, ComicVine, OPF.
- **Assessment:** **Portico behind** for audiobook users, who expect covers, series order, narrators and descriptions from Audible-derived sources (F-06.24).

### A-06.46 Trending feeds, provider "similar" lists and the Portico title dataset
- **Portico:** has all three, matched to the library by provider id, never by title (`metadata/discovery.go`, `screen_links.go`, `enrichment/enrichment.go`).
- **Jellyfin:** similar-items providers appeared recently (`TmdbMovieSimilarProvider.cs`, `SimilarItemProviders` in `TypeOptions`); no trending, no dataset.
- **Assessment:** **Portico ahead.**

### A-06.47 Attribution is a product requirement
- **Portico:** every candidate, artwork choice and publication carries an attribution string (`metadataprovider/screen.go:276-286`, `artwork_discovery.go:189-203`); the contract requires visible TMDB and TheTVDB attribution.
- **Jellyfin:** provider names in the UI; no enforced notice.
- **Assessment:** **Portico ahead** (and safer with providers' terms).

### A-06.48 Provider failures are durable state
- **Portico:** attempts, next attempt and a sanitised error code per row; a per-provider cooldown table shared by all rows; non-retryable codes park the row (`screen_worker.go:177-233`, `649-657`).
- **Jellyfin:** logs and moves on; the next refresh tries again.
- **Assessment:** **Portico ahead** in visibility. The parking rule is too eager for long outages (B-06.12).

### A-06.49 Provider responses are validated hard
- **Portico:** every record is checked for sizes, counts, control characters and id shapes; a search with more than 25 results, a result of the wrong type or one invalid record fails the whole search or record (`metadataprovider/screen.go:182-267`, `tmdb.go:187-196`).
- **Jellyfin:** trusts the client library.
- **Assessment:** **Portico ahead** on safety; the "fail the whole record" choice for list sizes is a latent trap where truncating would be kinder (B-06.17).

### A-06.50 What leaves the server
- **Portico:** cleaned title, year, typed ids, episode numbers, language and region only; a query that looks like a path or URL is refused (`screen_match.go:247-257`); Hosted services never see any of it (contract §2.4); errors are logged as codes, not queries.
- **Jellyfin:** similar data, no path guard, no consent.
- **Assessment:** **Portico ahead.**

### A-06.51 Finding what did not match
- **Portico:** music has an owner-only "Unmatched" tab. For movies and shows the match status is on each title's Matching tab only; the browse `match` field covers albums (`catalog/browse_contract.go:99-100`). I found no list, filter or count of `needs_selection`/`unmatched`/`unavailable` titles.
- **Jellyfin:** no explicit state, but unmatched titles are visibly bare, and the Reports plug-in lists items missing ids. **Plex** (from memory): an "Unmatched" filter in every library.
- **Assessment:** **Portico behind**, and it matters more for Portico than for anyone because A-06.2 leaves more titles unmatched (F-06.3).

### A-06.52 Changing provider re-matches the library
- **Portico:** every title matched with the old provider is cleared and searched again by name with the new one; identity locks from the old provider are released (`screen_review.go:165-183`).
- **Jellyfin:** reordering providers changes nothing until a refresh, and existing ids for the new provider are reused.
- **Why it matters:** Portico stores the crosswalk (a TMDB record lists its TheTVDB and IMDb ids, and vice versa), but the re-match does not use it; it starts from the name, under the strict rule of A-06.2, so a provider switch can turn a fully matched library into one with many "needs selection" titles.
- **Assessment:** **Portico behind** on this path: use the stored crosswalk id as an exact ID hint for the new provider first. Small (`screen_base.go` already reads `Existing` ids; the last publication's `crosswalk` is one more read).

### A-06.53 "Local metadata only" is a library-level agent
- **Portico:** one switch; a local library never contacts any provider for anything (`metadata/library_agent.go:13-22`).
- **Jellyfin:** untick every fetcher per type.
- **Assessment:** **Portico ahead** (clearer promise). Backlog notes the web add-library form failed with local-only + `en` on 25 Sep.

## Missing features and capabilities to consider

Sizes: **small** = days, one package; **medium** = a week or two, server and clients; **large** = multi-week.

### F-06.1 IMDb ids as match hints, and ids on folders
`{imdb-tt…}`/`[imdbid-tt…]` in file and folder names and `<uniqueid type="imdb">`/`<imdbid>`/`<id>tt…</id>` in NFOs, resolved through TMDB's find call or TheTVDB's remote-id search. Also read ids from the movie's folder name and from a show folder after the year. Jellyfin has all of this. Value: instant, certain matches for libraries managed by Radarr/Sonarr or exported from Kodi. Small. No conflict with principles (an exact typed id is the strongest evidence in the contract, §3.1). I would do this first; it removes most of the pain of A-06.2 for organised libraries.

### F-06.2 Fix Match by pasted id or URL
Let the owner type `tmdb:603`, `tt0133093` or paste a TMDB/TheTVDB/IMDb URL. Today a custom query is only a title search (`metadata/screen_match.go:88-91`). Jellyfin's Identify dialog has an id box per provider. Small.

### F-06.3 A "Needs attention" view for movies and shows
A filter, tab or dashboard count of titles that are `needs_selection`, `unmatched`, `unavailable`, `needs_order`, `needs_season_mapping` or `identity_conflict`, with one-click candidate selection from the list. Plex has an Unmatched filter; Portico has the tab for music only. The states already exist in `screen_metadata_work.status` (an index on `(library_id, status)` keeps it O(visible)). Small to medium. I consider this required before release if the strict matcher stays.

### F-06.4 A matching-strictness setting and year tolerance
See A-06.2 and A-06.12. Small.

### F-06.5 Automatic collections
Create or extend a collection from TMDB's `belongs_to_collection` (already in every stored record as a `collection` relation, `metadataprovider/tmdb.go:322-324`) and from an NFO `<set>` (already stored as `collectionName`). Options seen elsewhere: a library switch (Jellyfin `AutomaticallyAddToCollection`), a minimum number of owned films (Plex), collection poster/backdrop/overview from TMDB's collection endpoint. Medium (a background step keyed by the collection's TMDB id; fits "library-wide work in background batches"). High user value for film libraries.

### F-06.6 Person details and person-level portraits
Biography, birth/death dates and birthplace from TMDB's person endpoint, fetched lazily when a person page is first opened *by the background worker* or for the top-billed N per title; one portrait per person (A-06.43). The `catalog_people` columns already exist. Medium. The cost question is real at one request a second, so tie it to F-06.21's spare budget.

### F-06.7 Full TMDB artwork for TV
Season posters, show logos, alternative posters/backdrops and cast photos for TMDB-matched shows (TMDB returns all of these with `append_to_response=images,aggregate_credits` on the show and `images` on each season). Small to medium; closes B-06.7 and B-06.8.

### F-06.8 Image placeholders from the server
A dominant colour and/or a BlurHash/ThumbHash per stored image, computed at ingest when the pixels are already decoded, published with every artwork URL. Jellyfin and Navidrome both do it. Small on the server; small per client. Already on the Backlog as "dominant colour".

### F-06.9 WebP (or AVIF) output
Encode the three sizes as WebP when the client's `Accept` header allows. Navidrome does this in Go with a WebAssembly/native encoder (`navidrome/core/artwork/resize.go:15-29`); Jellyfin through Skia. Roughly a quarter to a third fewer bytes per image. Medium (a new dependency; the pure-Go rule for the image path would need a decision).

### F-06.10 One smaller and one larger size
About 200 px for list rows and phone grids; 3,840 px masters for backdrops on 4K screens (A-06.33, A-06.34). Small.

### F-06.11 Several backdrops per title
Jellyfin keeps a list and clients rotate them. Small in the selection table (the `subject` column could carry an index), small per client.

### F-06.12 Image language preference
Choose logos and posters in "library language, then no-language, then English" order, and show the language in the artwork picker. Small. Closes B-06.19.

### F-06.13 Fanart.tv as an artwork supplement
Clear logos, clear art, disc art, banners and 16:9 thumbs with the title on them, keyed by TMDB/TheTVDB/MusicBrainz id. Jellyfin has it as an official plug-in. Needs a project key and its own terms review. Medium. Conflicts with "one provider" only if supplements are ruled out (A-06.1).

### F-06.14 Wider, more tolerant NFO reading
Tolerate a trailing URL and stray text; skip a bad field instead of the file; read `<ratings>`/`<rating>`/`<votes>`, `<director>`, `<credits>` (writers), `<country>`, `<tag>`, `<trailer>`, actor `<thumb>` and `<order>`, `<status>`, `<displayseason>`/`<displayepisode>`, `<dateadded>`, `<set><overview>`, `<lockdata>`; more file names (A-06.24). Small to medium. High value for people migrating.

### F-06.15 Optional export of NFO and artwork beside the media
Listed in the contract (§6) as a future explicit operation. Jellyfin's savers are the precedent. Medium; keep it off by default.

### F-06.16 Wider local artwork names and formats
See A-06.25. Small. Include WebP.

### F-06.17 Critic and IMDb ratings
Via OMDb (owner key), or from the Portico title dataset, which could carry them offline with no per-title request. Small if it rides the dataset; medium as a live provider.

### F-06.18 Trailers
TMDB's `videos` list (YouTube keys) for a "Trailer" action; local `*-trailer.*` files. Medium (playback of a YouTube URL is a client question; Plezy and Jellyfin clients open the YouTube app or an embedded player).

### F-06.19 Missing and upcoming episodes
Placeholder rows for episodes the provider lists but the library lacks, and a calendar of upcoming ones. Jellyfin gained this in its TMDB plug-in (`TmdbMissingEpisodeProvider.cs`, 687 lines; settings `ImportMissingEpisodes`, `ImportUnairedEpisodes`, `ImportSpecials`). TheTVDB libraries in Portico already download the full episode list, so the data is half there. Medium to large. Product decision.

### F-06.20 Specials placement
Store "airs before/after season X episode Y" (TheTVDB supplies it; NFO `displayseason`/`displayepisode`) and use it in season lists and Next Up. Small to medium.

### F-06.21 A background refresh trickle, and Refresh for every level
Spend unused refresh budget on the stalest documents; make the owner's Refresh work on shows, seasons, episodes and whole libraries, with the two modes the policy already has. Small. Closes B-06.10.

### F-06.22 Content-rating fallback and mapping
Default the region from the language tag; optionally show a labelled foreign rating; map ratings to ages across systems so a restriction set in one system still applies to titles rated in another (Jellyfin ships such a table). Small to medium; interacts with the restrictions domain.

### F-06.23 Anime-specific help
An absolute-number mapping for TMDB (B-06.21); community id maps (anime-lists) to turn an AniList/AniDB id into TMDB/TheTVDB season and episode offsets; AniList voice-actor credits (the current query asks only for staff, `metadataprovider/anilist.go:31`). Medium.

### F-06.24 A book and audiobook provider
Audnexus/Audible-style data (cover, description, series and position, narrators, chapters) and Open Library/Google Books for ebooks. The contract (§11.2) requires a contract change first. Medium. Audiobookshelf is the reference.

### F-06.25 Broader artist images and biographies
Deezer, Last.fm, TheAudioDB or Fanart for artist photos; Wikipedia in the library's language (Backlog notes English-only). Small to medium each; each has its own key and terms.

### F-06.26 A frame from the video when there is no picture
For local-only libraries, home videos and episodes without a still. Jellyfin's screenshot provider is the precedent. Small (ffmpeg is already used for embedded covers and trickplay).

### F-06.27 Pictures and pages for studios, networks and genres
Jellyfin's Studio Images repository supplies studio/network logos. Small to medium; purely presentational.

### F-06.28 More editable fields
Community/critic rating, status and end year (also on the Backlog as missing show facts), runtime, several studios, country list, external ids (which is also F-06.2 by another door), people with role and order. Small each, thanks to the server-published schema.

### F-06.29 Owner-entered provider keys
A settings field per provider that replaces the shipped credential after validation (contract §2.2 allows it). Small. Insurance against a revoked shared key.

### F-06.30 Library-wide "identify" tools
"Match all unmatched by taking the top candidate", "re-match this folder", "refresh missing only". The bulk-job machinery exists (`httpapi/bulk_command_hooks.go`), but its `refresh` command accepts movies only. Small to medium.

### F-06.31 Remove an image, or set one from a URL
Selecting "no image" for a role, and pasting an image URL (fetched through the same guarded downloader with a broader allow-list decision). Small.

### F-06.32 Date-named episodes
Daily shows named `Show 2026-10-09`. Portico's episode identity is season/episode or absolute only (`0001_baseline.sql:1476`); matching by air date against the provider's list would need a third mode. Medium.

### F-06.33 Logo tone mapping on light surfaces
Plezy recolours white-on-transparent logos when they would sit on a light background (`plezy/lib/utils/tone_mapped_logo_image.dart`). Only relevant if Portico shows logos on light themes. Small, client-side.

## Possible bugs and risks in Portico

Confidence scale: **Confirmed by reading** (the code path is unambiguous), **Probable** (the path is clear but one link is inferred), **Speculative** (needs a runtime check). None of these was run.

### B-06.1 Titles that are or contain a year are parsed wrongly and end up unmatched
- **What goes wrong.** (a) The matcher's cleaner cuts a name at the first year-looking token, *including one at the very start*, so `1917`, `2012`, `1941`, `2046`, `1883`, `1923` and `2001 A Space Odyssey` become an empty search string. With no search string there are no candidates and the title is `unmatched`. (b) The scanner's parser takes the first year after a separator, so `Blade Runner 2049 (2017).mkv` is catalogued as "Blade Runner", year 2049, and `Wonder Woman 1984 (2020)` as "Wonder Woman", 1984; the search is then sent with the wrong year and finds nothing, and the wrong title and year are what the library shows meanwhile.
- **Evidence.** `apps/portico-server/server/internal/metadata/screen_match.go:16` (pattern allows `^`), `:55-58` (cuts at `m[0]`), `:74-81` (empty string dropped), `:104-110`; `apps/portico-server/server/internal/catalog/catalog.go:354-365`; `catalog/episodic.go:35` (show folders use the same function). Worked example for `1917 (2019).mkv`: seed title `1917`, year 2019; `screenFilename("1917")` → `""`; `screenFilename("1917 (2019)")` → first match is `1917` at position 0 → `""`; no queries.
- **How the comparison surfaced it.** Jellyfin's regex is greedy on purpose and has tests for `300 2001 (2006)` (`jellyfin/tests/Jellyfin.Naming.Tests/Video/CleanDateTimeTests.cs:16-21`).
- **Confidence:** Confirmed by reading. **Severity:** High (well-known titles; silent). **Backlog:** not listed. A manual Fix Match search works, because a custom query skips the cleaner.

### B-06.2 A title without a year never matches on its own, and nothing lists the leftovers
- **What goes wrong.** One strong signal is not enough (A-06.2). Typical TV folders (`Breaking Bad/Season 01/…`) carry no year, so most shows on a typical library stop at `needs_selection`, their episodes at `needs_parent_match`, and the owner must open each show's Matching tab to choose. There is no view that lists them (A-06.51).
- **Evidence.** `metadata/screen_match.go:194-205`, `:236`; `metadata/screen_match_test.go:52-56` asserts it; `metadata/screen_publish.go:962-964` (episodes wait for the parent).
- **Confidence:** Confirmed by reading (the rule); the size of the effect depends on how people name folders. **Severity:** High as a first-run experience. Deliberate per contract §3.2, so this is a design risk, not a coding error. **Backlog:** not listed.

### B-06.3 The "is this slot already taken?" check for provider posters never matches
- **What goes wrong.** When artwork is seeded from the accepted provider document, the code asks whether the poster/backdrop/still slot is already held (locked, local art, or already chosen under the current state). The query compares the integer `entity_id` column with the entity's **public text id**, so it is always false. The provider's poster and backdrop are therefore queued for download again **every time** the title is re-seeded, which is every time its work row is republished (any wake). Two consequences: (1) repeated downloads and re-encodes of images already held, per title per wake; (2) a title whose current poster is a **local sidecar** has it replaced by the provider's on the second seeding, because the later local check sees "a local image is already selected" and does not re-queue it, leaving the provider job to run and overwrite. Owner-locked selections are safe (a second guard in `queueArtworkCandidate` stops them).
- **Evidence.** `apps/portico-server/server/internal/metadata/artwork_integration.go:73` binds `t.ID` (a 22-character public id) to `a.entity_id=?` and `entity_id=?`; compare the correct form in `page_artwork.go:37-38` and `artwork_discovery.go:111`, which bind the resolved integer. Schema: `artwork_selections.entity_id INTEGER` (`0001_baseline.sql:1525-1530`). No worker-side precedence check: `artwork_worker.go:473-531`. Lock guard: `artwork_state.go:262-264`. Re-seed trigger: `0001_baseline.sql:4209` and `screen_publish.go:300`.
- **Confidence:** Confirmed by reading for the dead guard; **Probable** for the local-art overwrite (traced through the job upsert order, not run). **Severity:** Medium to High (owner's local posters silently replaced; wasted provider traffic on every library-wide wake). **Backlog:** not listed.

### B-06.4 Local artwork stops being accepted after 128 MB in total
- **What goes wrong.** Every sidecar image and extracted embedded cover is copied into `state/local-artwork`. Before each new file the code sums the folder and refuses the image if the total would pass 128 MB. Nothing ever removes files from that folder. A music library with embedded covers (extracted at up to 1,024 px, perhaps 100–200 KB each) reaches the limit after roughly 600–1,300 albums; after that, new local covers, posters, logos and artist photos are silently not recorded. Each new image also lists and stats the whole folder under a lock.
- **Evidence.** `apps/portico-server/server/internal/localmetadata/audio.go:111-150` (limit at `:136-137`); no delete in the package (`grep` for `os.Remove` finds only the temp-file cleanup at `:143`); folder set in `cmd/server/main.go:435`.
- **How the comparison surfaced it.** Jellyfin and Navidrome reference local images in place; Navidrome's image cache is sized and pruned (`navidrome/core/artwork/prune.go`).
- **Confidence:** Confirmed by reading. **Severity:** High for music and for any library relying on local art. **Backlog:** not listed (the Backlog's sidecar item is about negative caching).

### B-06.5 A season folder's `poster.jpg` becomes the show's poster; the usual season-poster name is not recognised
- **What goes wrong.** For an episode in `Show/Season 01/`, show-level images are looked for in the episode's own folder first and the parent second. Kodi and Jellyfin put the *season's* poster at `Show/Season 01/poster.jpg` (or `folder.jpg`), so Portico records a season poster as the **show** poster; with several seasons the last episode scanned decides. The same applies to `fanart.jpg`. Season posters are looked for only as `seasonNN.jpg|png` (and `season-specials-poster.jpg`), not the common `seasonNN-poster.jpg`. A removed sidecar is never noticed (the code says so).
- **Evidence.** `apps/portico-server/server/internal/localmetadata/sidecars.go:38-50` (`dirs = [dir, parent]`, first hit wins), `:57-75`; `catalog/local_artwork.go:41-56` (upsert; "a file's removal is not actively detected"), `:89-116`.
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-06.6 TheTVDB titles get no studio and no network
- **What goes wrong.** The TheTVDB adapter labels every company relation `company`; the publisher only turns `production_company`, `studio` and `network` relations into the studio and network facts. So a TheTVDB-matched show or film has neither, and cannot be filtered by them.
- **Evidence.** `apps/portico-server/server/internal/metadataprovider/tvdb_screen.go:210-214`; `metadata/screen_publish.go:391-405`. No other writer of those columns in the TheTVDB publication files.
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-06.7 Whole categories of artwork are missing depending on the provider
- **What goes wrong.** The provider image list is requested only for **TMDB movies** and **TheTVDB shows**. Therefore: a show on TMDB (the default for TV and anime since 2 Oct) gets its one poster, one backdrop and episode stills, but **no season posters, no logo, no cast photos of its own and no alternatives to choose from** (a cast member shows a photo only if the same person is also credited on a movie the library holds, since portraits are served from whichever title has one); a movie library on TheTVDB gets **no poster or backdrop at all** (the TheTVDB record carries no image path and no list is fetched).
- **Evidence.** `apps/portico-server/server/internal/metadata/artwork_discovery.go:160-164` (the only two cases), `:448` (`/movie/` only); `artwork_integration.go:57-66`; `metadataprovider/tvdb_screen.go:157` (no poster path set); `metadataprovider/tvdb_artwork.go:70` (`/series/` only).
- **Confidence:** Confirmed by reading. **Severity:** High (default configuration for TV). **Backlog:** not listed.

### B-06.8 A TMDB show's cast is the latest season only
- **What goes wrong.** The show document is requested with TMDB's `credits`, which for a series is the most recent season's cast, not everyone who was ever a regular. The aggregate-credits reader is still in the code but nothing calls it since the 2 Oct change.
- **Evidence.** `metadataprovider/tmdb.go:205-208`; `ScreenShowAggregateCredits` at `:435-497` has no caller (`grep` in `internal/metadata`). Jellyfin requests `CreditsAggregate` (`jellyfin/MediaBrowser.Providers/Plugins/Tmdb/TmdbClientManager.cs:151`).
- **Confidence:** Probable (relies on TMDB's documented meaning of `credits` for TV). **Severity:** Medium. **Backlog:** not listed.

### B-06.9 Changing a library's language or region does not refetch, and old text is relabelled as the new language
- **What goes wrong.** A language change sets every row to `pending`. For a matched title the worker then republishes the **stored** document while it is inside its freshness window (up to 30 days), because the freshness key is provider + kind + id with no language in it. The fields are rewritten with the new language label and the catalogue's `metadata_language` is set to the new language, though the text is still the old one. After the window, refetches are limited to 240 an hour, so a 20,000-title library takes days more.
- **Evidence.** `metadata/library_language.go:20-38` and `metadata/administration_policy.go:109` (wake only); `metadata/screen_freshness.go:86-104` (fresh → remembered record); `metadata/freshness.go:106-112` (key); `screen_publish.go:420` (stamps `b.Language`), `:438-443`. The only place freshness is cleared: `metadata/details.go:49-50`.
- **Confidence:** Confirmed by reading. **Severity:** Medium to High (the setting appears not to work; violates contract §5.1 "retains the actual locale returned"). **Backlog:** not listed.

### B-06.10 Only a movie can be force-refreshed
- **What goes wrong.** `POST /v1/items/{id}/metadata/refresh` and the bulk `refresh` command refuse anything that is not a movie. For a show or episode the editor's "Try again" re-queues the row, but an accepted identity inside its freshness window is republished from the stored document without asking the provider. So an owner cannot pull a corrected episode title or a new show poster for up to 14–30 days (36 hours for an airing show).
- **Evidence.** `metadata/details.go:36-42` (`kind != 1` → error); `metadata/screen_repair.go:158-161` (no freshness reset); `screen_freshness.go:94-104`.
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-06.11 First import of a large library takes days to weeks
- **What goes wrong.** One provider request a second and one work row a second (A-06.4). Rough arithmetic, provider calls only: a film needs 2–7 TMDB calls → 2,000 films ≈ 1–4 hours, 20,000 ≈ 11–39 hours; episodes need one call each → 100,000 episodes ≈ 28 hours, 1 million ≈ 12 days, 5 million ≈ 2 months. Artwork runs beside it at about one image a second per lane, and a film asks for up to ~100 portraits (B-06.20).
- **Evidence.** `metadataprovider/http.go:49`, `:61-80`; `worker/worker.go:104`; `metadata/service.go:132`, `:155-158`; `screen_worker.go:448-515`; the comment at `metadata/freshness.go:14-17`.
- **Confidence:** Confirmed by reading (the limits); the timings are estimates, not measurements. **Severity:** High against the stated scale principle. **Backlog:** not listed.

### B-06.12 A provider or internet outage of about a quarter of an hour parks titles permanently
- **What goes wrong.** Each failure doubles a row's delay from 30 seconds; on the sixth failure the row becomes `unavailable`, even for retryable errors such as "no network". Nothing re-arms `unavailable` rows; only a file change, a settings change or the owner's "Try again" does. Rows that are merely *waiting out a provider's cooldown* also burn attempts (`provider_backoff` is returned as an error and counted).
- **Evidence.** `metadata/screen_worker.go:197-228` (attempt limit at `:222-224`), `:259-267` (cooldown returned as an error), claim query `:32` (only `pending`, `searching`, `pending_children`).
- **Confidence:** Confirmed by reading. **Severity:** Medium to High (a router reboot during first import leaves a scattering of bare titles with no list to find them). **Backlog:** not listed.

### B-06.13 An owner's Fix Match search waits behind the background backlog
- **What goes wrong.** There is one worker and it orders work as: shows fanning out to episodes, then shows, then items, each by next-attempt time and then by id. A `searching` row has no priority of its own. During a long import a search on a movie with a high id waits behind every pending movie with a lower id.
- **Evidence.** `metadata/screen_worker.go:32`; `metadata/screen_repair.go:157`.
- **Confidence:** Confirmed by reading (the ordering); **Probable** for the user-visible wait. **Severity:** Medium. **Backlog:** not listed.

### B-06.14 With the default settings no title has a content rating
- **What goes wrong.** A library's region defaults to empty, and the rating is published only for an exact region match. The web's library settings offer an optional country box. So unless the owner fills it, `content_rating` is blank for the whole library, and anything that depends on it (rating badges, rating-based restrictions) has nothing to work with.
- **Evidence.** `0001_baseline.sql:2074` (`region … DEFAULT ''`); `metadata/screen_publish.go:375-386`; `apps/portico-server/web/src/screens/server/LibrarySettings.tsx:172`.
- **Confidence:** Confirmed by reading for the metadata side; how restricted profiles treat unrated titles belongs to another audit. **Severity:** Medium (High if unrated titles are shown to restricted profiles). **Backlog:** not listed.

### B-06.15 A field once set by an NFO stays NFO-owned after the NFO is gone
- **What goes wrong.** Provider values never replace a field whose stored source is `nfo`. When the NFO is deleted or loses a tag, the worker rewrites only the fields the remaining NFOs supply; I found no step that removes or releases `nfo`-sourced field rows that no longer have a source. The old NFO title, plot or cast then blocks the provider indefinitely.
- **Evidence.** `metadata/screen_publish.go:412`; `:853-950` (writes present values only); all field deletions in the package keep `source_kind IN('nfo','filename')` rows (`:233`, `:729`, `:815`; `screen_repair.go:185`).
- **Confidence:** Probable (I did not trace every path that could clear the row). **Severity:** Low to Medium. **Backlog:** not listed.

### B-06.16 One odd detail makes the whole NFO unreadable
- **What goes wrong.** The parser rejects the entire file for: any text after the XML (Kodi's documented "XML plus a URL" form, and URL-only files), invalid XML such as an unescaped `&`, a non-numeric or negative `<year>`, `<runtime>`, `<season>` or `<episode>` (older files write `<runtime>148 min</runtime>`), more than 100 `<actor>` entries, more than 64 genres or studios, more than 8 root elements, or a size over 256 KB. The title then shows the issue code `invalid_video_nfo` on its Matching tab and the NFO's ids and facts are lost.
- **Evidence.** `localmetadata/video_parse.go:50-52`, `:54`, `:62-65`, `:84-86`, `:104-106`, `:39-48` with `:129-147`, `:151-162`, `:214-216`; `localmetadata/video.go:61-64`.
- **How the comparison surfaced it.** `jellyfin/MediaBrowser.XbmcMetadata/Parsers/BaseNfoParser.cs:123-175` exists precisely for the trailing-URL case.
- **Confidence:** Confirmed by reading; how common each trigger is in real libraries is not measured. **Severity:** Medium. **Backlog:** not listed.

### B-06.17 A provider record with one over-long list is rejected whole
- **What goes wrong.** More than 256 alternative titles, 64 genres, 256 relations, 100 certifications or 64 orders fails validation, and a non-retryable validation failure parks the row. Keywords are trimmed before validation, so the author knew the pattern; the other lists are not. One invalid result also fails an entire search. TheTVDB records add every name translation to the alias list.
- **Evidence.** `metadataprovider/screen.go:187-189`; `metadataprovider/tmdb.go:187-196`, `:275-298` (keywords capped); `tvdb_screen.go:173-177`; `screen_worker.go:212-214`.
- **Confidence:** Speculative (needs a title that actually exceeds a limit; the biggest worldwide releases are the candidates). **Severity:** Low, but it would hit famous titles. **Backlog:** not listed.

### B-06.18 TheTVDB artwork keeps the first 80 images in the provider's order
- **What goes wrong.** A popular series has hundreds or thousands of images. The adapter keeps the first 80 it meets, not the best 80 or a spread across roles and languages, and the first of each role is then selected. The show may get a foreign-language poster or no logo at all.
- **Evidence.** `metadataprovider/tvdb_artwork.go:77-84`; `metadata/artwork_discovery.go:362-397`.
- **Confidence:** Probable (depends on TheTVDB's response order). **Severity:** Low to Medium. **Backlog:** not listed.

### B-06.19 Logos and alternative images are chosen without regard to language
- **What goes wrong.** For TMDB movies the image list is requested with no language filter and the first logo in the response is queued. TMDB orders by community vote, across all languages, so an English library can get a logo in another script.
- **Evidence.** `metadata/artwork_discovery.go:448` (no `language`/`include_image_language`), `:481-486`, `:362-397` (first per role).
- **Confidence:** Probable. **Severity:** Medium (visible on hero banners). **Backlog:** not listed (the Backlog notes only that movie `logo.png` sidecars are not exposed).

### B-06.20 Cast photos are downloaded per title, at original size
- **What goes wrong.** Up to 80 cast and 20 crew portraits are queued for every movie, each fetched at TMDB `original` size and re-encoded into three sizes, one job at a time. An actor in 40 of the owner's films is downloaded 40 times (stored once). For a 10,000-film library that is on the order of a million portrait downloads.
- **Evidence.** `metadata/artwork_discovery.go:487-505`, `:464`; candidate id includes the title (`artwork_state.go:236`).
- **Confidence:** Confirmed by reading; volume is an estimate. **Severity:** Medium (time, bandwidth, load on TMDB's image CDN from the shared User-Agent). **Backlog:** related item "NFO cast has no photos" mentions per-title portraits but not the cost.

### B-06.21 Absolute-numbered anime gets no episode data on the default provider
- **What goes wrong.** An anime library now defaults to TMDB. Files numbered absolutely are catalogued with `numbering = absolute`, and the TMDB episode step stops at `needs_order` for anything not seasonal. The show matches; every episode stays untitled and without a still.
- **Evidence.** `metadata/screen_publish.go:976-979`; migration `0010_tmdb_for_anime_rematch.sql`.
- **Confidence:** Confirmed by reading. **Severity:** High for anime libraries. **Backlog:** not listed (the Backlog says the demo's anime library moved to TMDB).

### B-06.22 Every TMDB search includes adult titles
- **What goes wrong.** `include_adult=true` is hard-coded, so adult entries appear as candidates and can be matched automatically if title and year agree. Jellyfin defaults this off with a setting.
- **Evidence.** `metadataprovider/tmdb.go:167`.
- **Confidence:** Confirmed by reading; whether it is intended is unknown. **Severity:** Low. **Backlog:** not listed.

### B-06.23 Small costs that add up on weak hardware (design notes)
- Each in-flight provider request starts a watcher that queries the database every 250 ms (`metadata/screen_worker.go:270-291`); each artwork download does the same every 300 ms with a full snapshot transaction and the multi-table fence computation (`metadata/artwork_integration.go:134-160`).
- Every image request performs authentication, an access check and two or three selection queries; a 60-poster grid is a few hundred small reads.
- Garbage collection of artwork reads the entire owner-history table into one string to look for digests (`metadata/artwork_worker.go:773-789`) every pass that has candidates.
- **Confidence:** Confirmed by reading; impact unmeasured. **Severity:** Low.

### B-06.24 Adding one episode re-fetches every episode of the show; the freshness rules do not cover episodes
- **What goes wrong.** When a show's row is woken (a new episode file, a changed file, an NFO change, a library setting), the show is republished and marked `pending_children`, which sets **every** episode of the show back to `pending` (owner-chosen ones excepted). For a TMDB show each episode step then calls TMDB for that episode with no freshness check, no budget and no conditional request: those three protections live only in the path for films and shows. So one new episode of a 900-episode series costs about 900 TMDB requests (a quarter of an hour at the current pace) and 900 episode publications, every time. Each publication also marks the episode's artwork dirty, and with B-06.3 that re-downloads its still. A library-wide wake does this for every episode in the library. The `freshnessEpisode` constant (30 days) is defined but nothing uses it for episodes.
- **Evidence.** `metadata/screen_publish.go:72-77` (any accepted show publication → `pending_children`); `metadata/screen_worker.go:555-610` (all episodes set pending, 64 per pass); `screen_publish.go:976-984` (direct `ScreenEpisode` call); `screenRefreshRecord` has one caller, `screen_worker.go:374`, which episodes never reach (`:304-306`); wake triggers `0001_baseline.sql:5566-5573`, `5562-5565`. The file header of `screen_freshness.go:14-18` names "a show fanning out to its episodes" as a case it was written to stop.
- **Confidence:** Confirmed by reading for the unguarded episode calls; **Probable** for the still re-download (it depends on B-06.3). **Severity:** High (it is the common weekly event for any airing show, and it multiplies B-06.11). **Backlog:** not listed (the Backlog's "re-walks finished episodes" item is about `tvdb_episode_credits.go`).

## Where Portico is stronger

- **It can explain itself.** Every fact has a source, a confidence, a language and a time; every match has stored candidates with reasons; every owner change is in a 30-step history with undo. Jellyfin has none of this.
- **Nothing overwrites a newer decision.** Leases, generation counters and input digests fence every background write; edits use revision checks. A slow provider answer cannot undo a Fix Match made meanwhile.
- **Locks are per field, per relationship class, per artwork role and for the match itself.** Jellyfin has nine field groups.
- **Wrong matches are rare by construction**, and an exact id always beats a name.
- **Provider traffic is disciplined**: consent first, minimal queries, path-like text refused, Retry-After honoured and remembered across restarts, conditional requests, a durable refresh budget, bounded response sizes, no redirects followed blindly.
- **The image path is hardened**: allow-listed hosts, private addresses refused at connect time, byte and pixel limits checked before decoding, formats sniffed from bytes, one bounded-memory decode at a time, content-addressed files installed atomically, reference-aware cleanup.
- **Images respect access control.** A restricted profile cannot fetch art for a title it may not see.
- **Bounded, predictable storage** for artwork, with immutable versioned URLs.
- **The editor's fields are data from the server**, so adding a field needs no client release.
- **Episode order is an explicit, validated choice**, including TMDB episode groups and all six TheTVDB orders, and local numbering is never silently renumbered.
- **AniList spoiler and adult tags are filtered at the adapter.**
- **Local-only libraries are a real promise**: one switch, checked on every provider path.
- **Trending feeds, provider similar-title lists and the signed Portico dataset** feed recommendations without sending the library anywhere.
- **Music identity** uses typed MusicBrainz evidence and optional fingerprints with the same conservative acceptance rule.
- **A shared client artwork scheduler** that loads in visual order, keeps a lane for the hero image and drops work nobody is waiting for.

## Doc/code disagreements

1. **Provider chain.** The contract still describes "TMDB primary, TheTVDB default fallback", an owner-chosen provider *order*, and AniList as the anime primary (`Product Contracts/metadata/contract.md` Scope, §2.1, §2.3, §14). The code and `03 Catalogue and Metadata.md` say one provider per library, TMDB by default for anime too. The 2 Oct decision is recorded in the architecture doc but the contract was not revised.
2. **Language fallback.** Contract §5.1 specifies a six-step fallback and "retains the actual locale returned". The code passes one language to the provider and stamps the *requested* language on every field (`metadata/screen_publish.go:420`, `:438-443`).
3. **Local evidence modes.** Contract §6 defines `off` / `supplement` / `prefer` generally. Only music has the setting (`0001_baseline.sql:1393`); for video the NFO always wins.
4. **Refresh intents.** Contract §8 lists six intents and a "scheduled age policy" trigger. The library policy has two modes (`replace_unlocked`, `fill_missing`), cascades accept `repair_assets` and `refresh_unlocked`, and there is no schedule.
5. **"Refreshes are conditional where the provider supports it"** (`03 Catalogue and Metadata.md`). Only TMDB film and show documents use conditional requests (`metadataprovider/tmdb.go:570-595` is the only implementation); TMDB episodes, TheTVDB and AniList do not, and episodes skip the freshness rules altogether (B-06.24).
6. **Authentication failure.** Contract §12: an authentication or licensing failure "disables new provider work and alerts the owner". I found only per-row parking (`request_rejected`, not retryable) and a start-up log line (`metadata/provider_state.go`); no provider-wide disable or owner alert.
7. **Owner override of the TMDB credential.** Contract §2.2 allows "an advanced override"; the only override is an environment variable (`cmd/server/main.go:712`), and there is none for TheTVDB.
8. **"Local art wins over online art unless the owner picks otherwise"** (comment, `localmetadata/sidecars.go:11-16`). Not reliably true because of B-06.3.
9. **Fix Match UI.** `Backlog.md` lists "Fix Match UI (the server side exists)" under planned features; the web editor already has a Matching tab with search, candidates and Try again (`apps/portico-server/web/src/screens/shared/MetadataEditor.tsx:420-479`), and the native app has a `MetadataEditor` screen.
10. **Stale client comments about image sizes.** `client-core/src/artwork/buckets.ts:5-7` ("400 and 1920 today; lane B adds 800") and `native/src/ui/ArtworkPipeline.ts` ("today's server knows only `size=thumbnail` and ignores `w`") predate the server's `w=400|800|1920` support (`httpapi/artwork_response.go:12-27`). The native app still appends `size=thumbnail`, harmlessly.
11. **Dead code.** `ScreenShowAggregateCredits` (`metadataprovider/tmdb.go:435-497`) and the `freshnessEpisode` window (`metadata/freshness.go:39`) are defined and unused; the legacy `Movie` type and `metadata_movie_selections`/`metadata_movie_candidates` reads remain in `repair.go:197-208`, `:320-322`, `:649-660` although the comment at `:637-638` says the legacy queue has no worker.

## Open questions for Justin

1. **How strict should automatic matching be?** Keep "two signals or ask" for everything, relax it for shows, or make it a setting? (A-06.2, B-06.2)
2. **Is a second source acceptable for supplements** (critic ratings, extra artwork, person biographies) when the *identity* provider stays single? (A-06.1)
3. **What first-import time is acceptable** for 2,000 films? For 100,000 episodes? That decides the provider pace. (A-06.4, B-06.11)
4. **Should IMDb ids be honoured** as match hints even though IMDb is not a provider? (F-06.1)
5. **Should Portico ever write beside the media** (NFO, artwork export), as an explicit owner action? (F-06.15)
6. **Content ratings:** when the library's country has no rating for a title, show nothing, or show another country's rating clearly labelled? And should the country default from the language? (B-06.14)
7. **Are automatic collections wanted**, and with what minimum size? (F-06.5)
8. **Should the server keep larger masters** (3,840 px backdrops) for 4K televisions, and is WebP worth a non-Go dependency? (A-06.33, F-06.9)
9. **People pages:** are biographies and dates wanted, given the extra provider calls? (F-06.6)
10. **Missing and upcoming episodes:** in scope for a personal media server, or not Portico's job? (F-06.19)
11. **Audiobook and book providers:** stay local-only, or add one? (F-06.24)
12. **Provider terms:** has anyone confirmed TMDB's, TheTVDB's and AniList's current terms for a free product that may later sell a relay, and for shipping keys in a GPL repository? Should owners be able to enter their own keys in settings? (A-06.9)
13. **Adult titles in search results:** intended? (B-06.22)
14. **Should a library-wide "Refresh" exist**, and should a person's Refresh always bypass the freshness window? (B-06.10)

## Evidence boundary

- Everything here is from reading source. I did not run the server, a scan, a provider call or a client.
- Provider behaviour (TMDB's rate limit and meaning of `credits` for TV, the order of TMDB and TheTVDB image lists, ETag behaviour across languages, TheTVDB and AniList terms) is from memory of public documentation and was not re-verified today.
- Timings in B-06.11 and volumes in B-06.4 and B-06.20 are arithmetic on the code's limits, not measurements.
- The consequence chain in B-06.3 (local art overwritten) and the still re-download in B-06.24 were traced by hand through the job upsert; they need one runtime check each.
- How real-world NFO files trip the strict parser (B-06.16) and how often real provider records exceed list limits (B-06.17) are unknown without a corpus.
- I did not establish what React Native's default image caches do on each platform, so the native app's on-device image caching is unassessed.
- The music matcher, the TheTVDB episode publication and the bulk-edit path were skimmed, not audited line by line. Jellyfin's TheTVDB plug-in was not available to read.

## Outside my domain

- **Scanner title/year parsing** (`catalog/catalog.go:354-365`) is the root of half of B-06.1 and belongs to the libraries-and-scanning audit as well.
- **Restricted profiles and unrated titles**: B-06.14 leaves most titles unrated by default; what a rating-restricted profile then sees is for the identity/restrictions audit.
- **`state/local-artwork` in backups and storage care**: the folder is listed under the "Artwork" storage category as not cleanable (`administration/maintenance.go:36`), which matters once B-06.4 is fixed.

