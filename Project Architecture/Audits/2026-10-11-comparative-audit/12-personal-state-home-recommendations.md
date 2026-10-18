# 12 — Personal state, Home and recommendations

> Comparative audit, 11 October 2026. Read-only static review. Nothing here was run. Every statement about behaviour is a reading of source code or public documentation, not a reproduction.

Path shorthand used throughout (all relative to `/home/justin/projects/Portico`):

| Prefix | Means |
|---|---|
| `S/` | `apps/portico-server/server/internal/` |
| `CC/` | `apps/portico-server/packages/client-core/src/` |
| `W/` | `apps/portico-server/web/src/` |
| `N/` | `apps/portico-react-native/apps/native/src/` |
| `PA/` | `portico-internal/Project Architecture/` |
| `PC/` | `portico-internal/Product Contracts/` |

Comparison paths are written `jellyfin/…`, `jellyfin-web/…`, `plezy/…`, `streamyfin/…`, `wholphin/…`, `audiobookshelf/…`, `navidrome/…`.

## Scope and sources

**Domain.** What a person's own activity does to the product: started / resumable / watched rules, Continue Watching and Next Up, favourites, My List, ratings, history and play counts, playlists and collections from the viewer's side, how Home is composed and customised, recommendations, Discover, search, and how preferences travel between devices.

**Portico, read in full or nearly so**

- Docs: `PA/03 Catalogue and Metadata.md`, `PA/05 Recommendations.md`, `PA/Backlog.md`, `PC/saved-state-and-downloads/contract.md`, `PC/decisions.md`, `apps/portico-server/server/api/home.md`, `api/preferences.md`.
- Server, watched and progress: `S/personalstate/watched.go`, `S/catalog/personal.go`, `personal_activity.go`, `container_personal.go`, `continue_policy.go`, `continue_count.go`, `play_history.go`, `personal_batch.go` (head), `S/playback/personal_progress.go`, `S/playback/service.go` (session start and progress), `S/playbackv1/sessions_change.go` (report path), `S/downloads/progress.go` (offline sync).
- Server, Home and Next Up: `S/catalog/home_rows.go`, `home.go` (the older projection), `home_continue_source.go`, `home_show_continuation.go`, `show_workspace_next.go`, `show_workspace.go` (season choice around Next Up), `home_recent_source.go`, `home_views.go`, `discovery.go` (Continue section).
- Server, recommendations: `S/catalog/rec_engine.go`, `recommendation_engine.go`, `rec_generators.go` (generators and daily choice), `rec_title.go` (Viewers also watched), `S/compactcatalog/rec_profile.go` (signal weights), and the recommendation triggers in `S/persistence/migrations/0001_baseline.sql`.
- Server, saved things and search: `S/catalog/saved.go`, `resources.go`, `playlists.go`, `playlist_types.go` (types and limits), `search.go` (groups, normalisation, scoring), `search_history.go`, `S/compactcatalog/derive_search.go`, `S/operations/preferences.go` (the preference registry).
- Schema: `0001_baseline.sql` (personal tables and triggers), `0003_drop_reaction.sql`, `0007_play_history.sql`, `0011_recommendation_positive_seeds.sql`.
- Clients: `CC/container-state.ts`, `continue-watching.ts`, `home.ts`, `search.ts`, `presentation/search-model.ts`, `presentation/title-page.ts`, `playback-v1/timeline.ts` (report cadence); `W/screens/detail/Show.tsx`, `W/screens/search/Search.tsx`; `N/screens/Show.tsx`, `N/screens/Search.tsx`.

**Comparison projects, read**

- Jellyfin server (master, Oct 2026): `Emby.Server.Implementations/Library/UserDataManager.cs` (`UpdatePlayState`), `Session/SessionManager.cs` (start / progress / stopped handlers, additional users), `TV/TVSeriesManager.cs`, `Jellyfin.Server.Implementations/Item/NextUpService.cs`, `Item/ItemPersistenceService.cs` (user data kept when an item is deleted), `MediaBrowser.Controller/Entities/BaseItem.cs`, `Folder.cs`, `Video.cs`, `TV/Episode.cs`, `TV/Series.cs` (mark played, user-data keys), `Library/SimilarItems/*` (similar items and movie recommendations), `Jellyfin.Api/Controllers/MoviesController.cs`, `MediaBrowser.Model/Configuration/UserConfiguration.cs` and `ServerConfiguration.cs`.
- jellyfin-web: `src/components/homesections/**`, `src/constants/homeSectionType.ts`, `src/scripts/settings/userSettings.js`, `src/strings/en-us.json`.
- Plezy: `lib/services/data_aggregation_service.dart`, `local_playback_history.dart`, `offline_watch_sync_service.dart`, `lib/services/trackers/*` (Trakt, Simkl, MDBList, MAL, AniList), `lib/services/catalog/*` (Explore sources, Seerr), `jellyfin_client/parts/watch_state.dart`, `plex_client.dart` (remove from Continue Watching, ratings).
- Streamyfin `utils/atoms/settings.ts`, `components/home/*`; Wholphin `data/model/HomeRowConfig.kt`, `WholphinDataStore.proto`, services list; jellyfin-androidtv `constant/HomeSectionType.kt`; Audiobookshelf `server/models/MediaProgress.js`, `Library.js`, `LibraryItem.js`; Navidrome `core/scrobbler`.
- Plex, Emby, Infuse, Kodi: from public documentation as I remember it. I did not re-fetch those pages today, so treat Plex/Infuse statements as "public docs, not re-verified on 11 Oct".

**Not reached**

- Portico: `rec_browse.go` (For you sort), `rec_composition.go`, `rec_hydration.go`, `derive_rec_facets.go`, `internal/enrichment`, `recommendation_pool.go`, most of `show_workspace*.go`, `collections.go`, `resource_pins.go`, `library_pins.go` bodies, `browse_personal.go`, bulk jobs beyond the container tests, native `Home.tsx` / `HomeCustomise.tsx` / `Saved.tsx` and web `Home.tsx` / `Customise.tsx` / `Saved.tsx` bodies, the Vega app, Top Shelf code.
- Jellyfin: `PlaylistsController`, `CollectionController`, `DisplayPreferencesController`, `UserViewsController`, `SuggestionsController`, the Trakt and Playback Reporting plugins (not in the clones), Swiftfin's home, jellyfin-vue.
- Plezy: the home screen widgets, the Explore and Discover screens' UI code, Plex watchlist calls.

## How Portico does it today

### The data

Personal state is kept per **profile** (a "personal key" built from authority, account and profile) and per **catalogue entity** (the integer id of a film, episode, song or audiobook file).

| Table | What it holds |
|---|---|
| `personal_items` | One row per profile and title: My List flag (`watchlisted`), Favorite, Not interested, star rating (0.5–5 in half steps), a cached watched bit, `last_played_at`, and a revision number |
| `personal_watched_intents` | The profile's explicit word on one item: watched or not, and when it was authored |
| `container_personal_state` | A watched **default** for a whole show, season, album or book: watched or not, a `watermark` time, and `cleared_through` |
| `progress` | The resume position in milliseconds, and which playback session owns it |
| `progress_activity` | When the item was last active and in what state (`playing`, `paused`, `ended`) |
| `profile_show_activity` | Per profile and show: the last activity time and the last active episode. Kept by triggers; drives Next Up |
| `continue_dismissals`, `continue_show_dismissals` | "Remove from Continue Watching", per item and per show |
| `personal_history` | The viewer's own History list (they can pause and clear it) |
| `play_history`, `play_counts`, `personal_play_counts` | The owner's permanent record of plays, and counts of plays to the end (server-wide and per profile) |
| `rec_profile_signals`, `rec_profile_taste`, `rec_profile_jobs` | The recommendation engine's per-profile model |

### Watched is computed, not stored

"Is this item watched?" is one SQL expression (`S/personalstate/watched.go:29-53`):

1. If the profile has an explicit intent for the item (and it is newer than any "mark unwatched" on a container above it), that intent wins.
2. Otherwise the newest container default applies (season beats show on a tie), **but only if the item had been added to the library by the time the default was set**. So "Mark show watched" does not mark episodes that arrive later.
3. Otherwise not watched.

This makes "Mark show watched" a single small write regardless of how many episodes the show has (`S/catalog/container_personal.go:121-179`). "Mark show unwatched" is also one write, plus a background job that clears the per-episode intents in batches (`:198-295`).

### Started, resumable, watched: the rules

The **server** decides, on every progress report from a player (`S/catalog/personal_activity.go:27-150`). Clients report position and state about every 10 seconds while playing and every 30 while paused (`CC/playback-v1/timeline.ts:102`).

| Rule | Value | Where set |
|---|---|---|
| Started (a resume point is kept) | Position ≥ the smaller of 30 seconds and `startedThresholdPercent` of the duration (default 5%). An audiobook file: 1 second | `personal_activity.go:69-75`; preference per profile, 1–25% (`S/operations/preferences.go:177`) |
| Watched by percentage | Films and episodes: the **library's** "video played threshold", default 90% (50–100). Songs and audiobook files: the **profile's** `playedThresholdPercent`, default 95% (75–100). If a profile has changed its own value, that replaces the library's for video too | `personal_activity.go:45-48,57`; `continue_policy.go:19-22,43-47`; `preferences.go:178` |
| Watched by credits | The first **owner-approved** credits marker. Library setting `videoCompletion`: `earliest` (default: whichever of percentage or credits comes first), `threshold` (percentage only), `credits` (credits only) | `personal_activity.go:58-66`; `continue_policy.go:74-88` |
| Watched by reaching the end | A report with state `ended` completes the item at any position | `personal_activity.go:66` |
| Below "started" | Resume position is set to zero; a previous watched mark is left alone | `:120-122` |
| Between started and watched | Not watched, resume = position. This also **clears** a previous watched mark (a rewatch makes the title unwatched until it completes again) | `:117-119,123-127` |
| Songs | Never keep a resume point | `:128-130` |

Only the "canonical" session writes resume and watched state: the playback that most recently claimed the item for this profile (`S/playback/personal_progress.go:43-76`). An explicit Mark watched / unwatched "fences" older sessions so their later reports cannot undo it (`S/catalog/personal.go:161-168`, checked at `personal_activity.go:103-112`).

A profile can **pause its watch history**. While paused, nothing of the viewer's own is recorded (no resume, no watched, no History entry, no personal play count), but the owner's play history still records the play (`personal_activity.go:76-84`; `play_history.go:16-19`).

### Explicit changes

`PUT /v1/items/{id}/personal-state` sets exactly one field per call (My List, Favorite, Not interested, rating, watched, Continue Watching dismissal) with an idempotency key and the revision the client last saw (`personal.go:329-556`). There is a designed path for changes made **offline on two devices**: conflicting changes from a shared starting point are recorded and the viewer chooses (`:434-497`). A separate batch route sets one field on up to 200 items (`personal_batch.go`).

Offline **playback** progress from downloads goes through a different, simpler path: last write wins by the device's clock (`S/downloads/progress.go:66-198`).

### Continue Watching and Next Up: one row

Home has one row, `continue`, that holds both unfinished titles and next episodes (`home_continue_source.go:17-68`):

- **Films**: in progress, not dismissed, last active within the library's window.
- **Shows**: at most **one episode per started show**. In order of preference: the last active episode if it is still unfinished; else the newest unfinished episode of the show; else the first unwatched, unstarted episode **after the last active episode** in season/episode order (or absolute order) (`home_show_continuation.go:57-73`). A special is offered only when the last active episode was itself a special (`:68`).
- **Window**: the library's "weeks" setting, default 16 (1–104). A show older than that is still shown if the offered episode is episode 1 of a later season ("include season premieres", default on) (`:79-84`).
- **Size**: the smallest "maximum items" among the viewer's libraries, default 40 (1–200).
- **Order**: most recent activity first.
- **Remove**: per item, with Undo; removing an episode hides the show until the show has newer activity (`:77-78`).

`Continue Listening` is a second row for audiobooks in progress (songs keep no resume point), capped at 40 (`home_rows.go:267`).

The show page asks the same question for one show (`show_workspace_next.go:13-57`). A show never started offers its first unwatched episode, specials last.

### Home

`GET /v1/home` returns an ordered list of rows the **server** composes (`home_rows.go:785-881`). Default order, by priority: Continue Watching (10), Continue Listening (20), Recently added in *each library* (30, in the owner's library order), Recommended for you (60), the personal rows family (65), Trending now (70), Popular on this server (80), then any saved views the profile placed (90+) (`:351-383`).

- **Hero**: the first entry of Continue Watching or Continue Listening, whichever is newer. No hero if neither row has entries (`:889-902`).
- **Customising**: per profile, a row order and a hidden set, stored as two synced preferences (`home.rowOrder`, `home.hiddenRowIds`; `preferences.go:208-209`), at most 64 rows. Continue rows cannot be hidden. The personal rows are arranged as one family.
- **Recently added** is per library, one card per *work* (a show, an album, a book, or a film across its editions), newest 100 (`home_recent_source.go`). It does not hide watched titles.
- **Freshness**: every row and cursor carries the catalogue revision and the viewer revision; any personal change bumps the viewer revision by trigger, so the next read is recomposed.

### Recommendations

Described well in `PA/05 Recommendations.md`. In one paragraph: each work has typed features (genre, people, studio, franchise, decade, dataset tags…); each profile has a taste vector at two time horizons, updated exactly as the profile acts; a request reads the heads of the best-first lists for the profile's strongest features plus similar-title links of recent favourites, rescores a few hundred candidates, removes what the profile has engaged with or may not see, and diversifies page by page. Rows rotate daily. "Not interested" with Undo and "Reset recommendations" are the feedback controls. "Popular on this server" and "Viewers also watched" use other members' history only when the owner allows it and the member has opted in.

### Saved, playlists, views

Saved has My List, Favorites and Playlists tabs (`saved.go:34-36`). My List and Favorites accept items and whole titles (show, album, book, artist for Favorite). Playlists are ordered, allow duplicates, and can be shared with named profiles as viewer or editor (up to 100 shares). Viewer-owned "saved collections" and "saved views" (a stored browse query, pin-able to Home) are the other saved resources (`resources.go`).

### Search

Title search over an SQLite FTS5 index (`unicode61`, diacritics removed, prefix indexes), all tokens must match as prefixes, 2–128 characters, up to 8 tokens (`search.go:114-135`). Nine groups: movies, shows, episodes, artists, albums, songs, books, people, live TV, each with its own 600 ms budget and paging. Clients debounce typing, show type chips, a "Top result" when exactly one title matches strongly, and recent searches held in memory.

### Preferences

One registry (`S/operations/preferences.go`). Two scopes: **profile-server** (follows the profile to every device) and **profile-device-class** (appearance, quality and delivery choices differ by class of device). Language and subtitle defaults, skip behaviour, thresholds, privacy switches and the Home layout are all profile-server.

## How the others do it

### Jellyfin

**User data.** One `UserData` row per user and item: `Played`, `PlaybackPositionTicks`, `PlayCount`, `LastPlayedDate`, `IsFavorite`, `Likes` (a nullable thumbs up/down), `Rating`, and the remembered audio and subtitle stream. Rows are matched by a **user-data key**, not only by item id: a film's key is its IMDb or TMDB id, a series' its TVDB/IMDb id, an episode's the series key plus season and episode number (`jellyfin/MediaBrowser.Controller/Entities/Video.cs:293-320`, `TV/Series.cs:183-201`, `TV/Episode.cs:162-180`). When an item is deleted its user data is **kept**, parked on a placeholder item with a retention date, and re-attached when an item with the same key reappears (`jellyfin/Jellyfin.Server.Implementations/Item/ItemPersistenceService.cs:93-131,296`; a scheduled task cleans up old detached rows). Watched state therefore survives file renames, quality upgrades and re-created libraries.

**Thresholds** are server-wide and admin-only (`jellyfin/MediaBrowser.Model/Configuration/ServerConfiguration.cs:133-157`): resume from 5% (`MinResumePct`), played at 90% (`MaxResumePct`) or within one second of the end, and items shorter than 300 seconds (`MinResumeDurationSeconds`) are never resumable (they become played once past 5%). Audiobooks use minutes instead: resume after 5 minutes in, finished with 5 minutes left. An item with **no known runtime** is assumed fully played when stopped. A failed playback changes nothing. (`jellyfin/Emby.Server.Implementations/Library/UserDataManager.cs:455-529`; `Session/SessionManager.cs:1176-1215`.) There is no credits-marker rule in the server.

**Play count and last played.** `PlayCount` goes up when playback **starts**; marking played by hand ensures it is at least 1. `LastPlayedDate` is not moved by merely starting an already-played item (a code comment explains that otherwise "rewatch Next Up moves on from an episode that was barely started") (`SessionManager.cs:844-863`). Rewatching does **not** clear `Played`.

**Versions.** Each version of a film is its own item with its own resume point; completing one marks every version played and clears their resume points (`SessionManager.cs:1205-1212`). Next Up tries to offer the next episode in the same named version the viewer has been watching (`TV/TVSeriesManager.cs:269-300`).

**Watching together.** A session can carry **additional users**; start, progress and stop update user data for all of them (`SessionManager.cs:582-600,786,926,1139,1647-1692`).

**Mark played cascade.** Marking a series or season played walks every episode and writes each one (skipping unaired placeholders), with an optional date played (`Entities/Folder.cs:1956-2013`, `BaseItem.cs:2207-2253`).

**Resume and Next Up are two rows.** "Continue Watching" (resumable items) and "Next Up" are separate home sections. Next Up (`TVSeriesManager.cs:36-200`, `NextUpService.cs`): per series, find the most recently played episode, offer the first unplayed episode after it; ordered by when the series was last watched. Options: a date cut-off the client supplies (web setting "Max days in Next Up", default 365), **rewatching** (also offer the episode after the last played one even if played), specials slotted into seasons by their "airs before/after" data (server switch `DisplaySpecialsWithinSeasons`), and whether to include a next episode that is already partly watched. Libraries can be excluded per user. There is **no** "remove from Continue Watching / Next Up" in Jellyfin 10.11 (Plezy's adapter notes the route exists on Emby only: `plezy/lib/services/jellyfin_client/parts/watch_state.dart:54-66`).

**Home is authored by each client.** jellyfin-web has ten ordered slots chosen from a fixed list: library tiles, library buttons, Continue Watching (video), Continue Listening, Continue Reading, Live TV, Next Up, Latest media (one row per library), active recordings, none (`jellyfin-web/src/constants/homeSectionType.ts`). The choice is stored in the user's display preferences under the web client's name, so the TV app, Swiftfin and third-party apps each have their own home and their own settings. Per-user server options: library order, grouped folders, libraries hidden from "My Media", libraries excluded from Latest, and "hide played in Latest" (default on) (`jellyfin/MediaBrowser.Model/Configuration/UserConfiguration.cs`). No hero in the official web client.

**Recommendations.** Only for films, on a library's Suggestions tab: a handful of categories built from the seven most recently played films and ten random favourites — "because you watched X", "because you liked X", "directed by", "starring" (`jellyfin/Emby.Server.Implementations/Library/SimilarItems/SimilarItemsManager.cs:304-425`). "More like this" is a weighted overlap count (director 50, actor 15, genre 10, tag 5, studio 5) optionally merged with a metadata provider's similar list, cached on disk with an expiry (`MovieSimilarItemsProvider.cs:29-50`; `SimilarItemsManager.cs:176-231`). No taste model, no negative feedback, no kids-specific logic beyond normal access filtering.

**Other** (partly from prior knowledge of Jellyfin 10.9+; the playlist, collection and search controllers were not read today). Favourites exist for every item type; star ratings and likes exist in the API but the official web client only shows the heart. Playlists (audio or video) can be public or shared with named users who may edit; collections ("box sets") are server-wide. The admin activity log records who played what (30 days by default); the Playback Reporting plugin keeps long-term statistics. Trakt sync is a plugin. Search is substring matching on names.

### Plezy

Plezy is a client, so it shows what a careful client has to add on top of Plex / Jellyfin / Emby:

- **Cross-server merging.** Home hubs, Continue Watching / On Deck, search and the library list fan out to every connected server and are merged and de-duplicated; a slow server gets a deadline so it cannot hold back the rest; failures, cancellations and "could not ask" are kept apart so absence is never inferred from an error (`plezy/lib/services/data_aggregation_service.dart:22-210`).
- **A device-local play log** exists only to decide which duplicate copy the user actually played, because servers do not say (`plezy/lib/services/local_playback_history.dart:7-14`).
- **Offline watch sync**: queued progress and manual marks are replayed when online; a terminal mark purges older queued progress for the same item, after a bug where the stale progress "pins the item to Continue Watching for good" (`plezy/lib/services/offline_watch_sync_service.dart:113-125`).
- **Trackers**: Trakt, Simkl and MDBList scrobble in real time; MyAnimeList and AniList are told once when the watched threshold is crossed; manual and container marks are batched; failed writes are queued and retried; ratings sync both ways; an anime id mapping is downloaded only if an anime tracker is active (`plezy/lib/services/trackers/tracker.dart`, `tracker_coordinator.dart:26-44`).
- **Explore tab**: pluggable external catalogues (Plex Discover, Trakt, Simkl, MDBList, MAL, AniList, Seerr) with trending rows, a watchlist that can hold titles not in any library, matching of catalogue titles to library items, and **requesting** a missing title through Seerr (`plezy/lib/services/catalog/catalog_source.dart`, `seerr_catalog_source.dart`).
- **Remove from Continue Watching** where the server supports it (Plex, Emby); hidden where it does not (Jellyfin).
- Watched threshold follows the server (Plex's preference, 90% for Jellyfin).

### Plex (public documentation, not re-verified today)

- Server-wide library settings that Portico has copied almost one for one: weeks to consider for Continue Watching (16), maximum items (40), include season premieres, video played threshold (90%), and completion behaviour (threshold, first credits marker, final credits marker, earliest of threshold and first credits marker).
- One merged Continue Watching row (On Deck was folded in), with "Remove from Continue Watching".
- Star ratings, no separate favourite for video. Watchlist is account-level and lives in Plex's cloud ("Discover"), so it holds titles that are on no server, shows where they can be watched, and follows the account to every server. Optional sync of watched state and ratings across servers through the account.
- Home is server-composed "hubs". The owner can manage a library's recommended rows and promote collections (including smart, filter-based collections) to everyone's Home; each user can pin and reorder libraries.
- Smart playlists and smart collections (saved filters that stay live).
- The owner sees every user's history; managed users cannot hide from the owner.

### Others, briefly

- **Emby**: per-item "hide from resume"; otherwise close to Jellyfin.
- **Infuse**: Up Next list combining in-progress and next episodes; iCloud sync of watched state across Apple devices; Trakt sync both ways.
- **Kodi**: per-file thresholds in advanced settings (ignore the first 180 seconds, played at 90%, ignore the last 8%).
- **Streamyfin**: client-authored home with an optional setting to merge Next Up into Continue Watching; a hero carousel whose sources can be chosen; a companion server plugin lets the admin push settings to all clients and **lock** them; optional recommendation and search back-ends (Streamystats, Marlin); Seerr requests (`streamyfin/utils/atoms/settings.ts:400-560`, `components/home/Home.tsx:298-327`).
- **Wholphin** (Android TV): fully user-built home rows (continue, next up, combined, recently added, recently released, genre rows, studio rows, favourites by type, live TV), stored in the server's display preferences; rewatching and max-days options; publishes Next Up to the Android TV system "Watch Next" row; Seerr (`wholphin/app/src/main/java/com/github/damontecres/wholphin/data/model/HomeRowConfig.kt`, `WholphinDataStore.proto:100-106`).
- **Audiobookshelf**: finished when under 10 seconds remain, or at a percentage, **per library** (`audiobookshelf/server/models/Library.js:15-16`); Continue Listening, Continue Series, Listen Again and Discover shelves; "hide from Continue Listening" that resets when progress changes (`MediaProgress.js:214-217`).
- **Navidrome**: stars and 1–5 ratings, play counts, scrobbling to Last.fm and ListenBrainz with a buffered retry queue, smart playlists as rule files.

## Architectural differences

Sizes are rough: **small** = one file or one query, **medium** = a few files plus clients, **large** = schema or cross-cutting.

**Group: State model and authority**

### A-12.1 Watched is computed from intents and container defaults, not stored per item
- **Portico:** One SQL expression answers "watched?" from an item intent, else the newest show/season/album/book default with a watermark (`S/personalstate/watched.go:29-53`). Marking a 5,000-episode show is one row.
- **Jellyfin / Plezy / other:** Jellyfin stores a `Played` flag per item and writes every episode when a series is marked (`jellyfin/MediaBrowser.Controller/Entities/Folder.cs:1956-1992`). Plex stores per-item view counts.
- **Why it matters:** Portico's write is constant-time and cannot time out on a huge show; the price is that every read of watched state runs a correlated sub-query, and that the precedence rules (A-12.2, A-12.3) are subtle and user-visible.
- **Assessment:** **Portico ahead** on scale, deliberate (scale principle; tests `S/catalog/bulk_jobs_test.go:149-199, 285-310`). The precedence choice is the risk, not the mechanism.

### A-12.2 An item's own intent always beats a later "mark show watched"
- **Portico:** If an episode has any explicit intent (it was started, or once marked unwatched by hand), a later "Mark season/show watched" does not change it. Only "mark **un**watched" on a container overrides older item intents (`watched.go:49-52`; `container_personal.go:155-161`). The test expects this (`bulk_jobs_test.go:152-163`).
- **Jellyfin / Plex:** Mark played on a series sets every episode played and clears every resume point.
- **Why it matters:** The viewer presses "Mark season as watched" and the episode they were half-way through stays unwatched, keeps its resume point and stays in Continue Watching. For an audiobook, "mark book finished" while mid-book leaves the current file unfinished. See B-12.1.
- **Assessment:** **Portico behind.** Deliberate in code, but I could not find the product decision that asks for it. Fix is small: when a container is marked watched, set `cleared_through` as the unwatched path does (so older item intents stop applying) and zero the members' resume points lazily or in the existing background job.

### A-12.3 Titles added after a container mark stay unwatched; a title with no "added" time never inherits
- **Portico:** A default applies only to items whose `added_ms` is at or before the watermark; an item with unknown `added_ms` never inherits (`watched.go:16-18,51`).
- **Jellyfin / Plex:** Same visible result for new episodes (they simply have no user data yet).
- **Why it matters:** Correct for new episodes. But an episode whose file is replaced or re-imported with a newer added time may flip back to unwatched if its watched state came only from the container default. I did not trace whether `added_ms` changes on replacement.
- **Assessment:** **Trade-off (subjective).** Worth a test that a re-scanned or replaced episode keeps its inherited watched state.

### A-12.4 Personal state follows the catalogue entity; Jellyfin's follows the title's provider identity
- **Portico:** All personal tables reference `catalog_entities(id)` with `ON DELETE CASCADE` (`0001_baseline.sql:345, 1277, 1994-1996, 3551`). A film's entity identity is its **path relative to the library root** (`S/compactcatalog/write.go:85-94`); an episode's is show + numbering + season + number (`:100`). A removed entity is hard-deleted on purge (`S/catalog/purge.go:59`, `write.go:580-600`); the identity ledger gives the same *public id* back if the same path returns, but the personal rows are gone.
- **Jellyfin:** User data is keyed by IMDb/TMDB/TVDB id (plus season and episode number), kept when the item is deleted and re-attached later (see "How the others do it").
- **Why it matters:** In Portico, renaming a film file, replacing it with a better copy under a new name, or moving it to another folder appears to create a new title with no watched state, rating, favourite or My List entry. Episodes are safe from renames because their identity is numeric. Removing and re-adding a library from a different root loses everything.
- **Assessment:** **Portico behind** (probable; the scanner's move handling is outside my domain — see B-12.18). Adopting provider-keyed personal state is **large**; a cheaper step is to carry personal rows across when a new entity gets the same provider id as a just-retired one in the same library.

### A-12.5 Thresholds are per library and per profile; Jellyfin's are server-wide
- **Portico:** Video completion rules belong to the library's owner settings (`S/catalog/continue_policy.go:11-22`); started and played percentages are also per-profile preferences (`S/operations/preferences.go:177-178`), and a profile's own played value overrides the library's (`personal_activity.go:45-48`).
- **Jellyfin:** Four server-wide numbers, admin only. **Plex:** server-wide. **Audiobookshelf:** per library.
- **Why it matters:** More flexible (a kids' cartoon library can complete at credits, a concert library at 98%), but there are three places a threshold can come from and the one shown to the viewer is not always the one applied (B-12.10).
- **Assessment:** **Trade-off (subjective).** Consider making the profile value "follow the library" by default in the UI rather than showing 95%.

### A-12.6 "Started" is a time floor capped by a percentage; Jellyfin's is a pure percentage plus a minimum length
- **Portico:** Started at `min(30 s, startedPercent × duration)`; 1 s for an audiobook file (`personal_activity.go:69-75`).
- **Jellyfin:** Started at 5% of runtime, and items under 5 minutes are never resumable. **Kodi:** 180 seconds.
- **Why it matters:** Portico keeps a resume point after 30 seconds of a three-hour film where Jellyfin needs nine minutes. Good for "I just started it"; noisier Continue Watching for people who sample. Clips of a few seconds can become "in progress". The percentage preference has almost no effect above ten minutes (B-12.11).
- **Assessment:** **Trade-off (subjective).** I prefer Portico's floor, but the preference should be redefined or removed.

### A-12.7 Completion by credits marker
- **Portico:** First owner-approved credits marker completes the title; three modes (`continue_policy.go:74-88`; `personal_activity.go:58-66`). Detector guesses that the owner has not approved do not count.
- **Jellyfin:** Not in the server. **Plex:** four modes including *final* credits marker.
- **Why it matters:** Correctly ends long-credit films without lowering the percentage for everything. Requiring approval is safe but means the default `earliest` mode behaves as plain 90% on a library where nobody approves markers. `credits` mode has a trap (B-12.9).
- **Assessment:** **Portico ahead** of Jellyfin, level with Plex. Open question whether unapproved high-confidence markers should count.

### A-12.8 Unknown duration
- **Portico:** No percentage completion (`personal_activity.go:57`), no place in Continue Watching (the comparison against a NULL duration fails, `home_rows.go:291`), and a new session always starts from zero (`S/playback/service.go:464`, `resume >= duration-3` is true when duration is 0).
- **Jellyfin:** Assumes the item was fully played when stopped (`UserDataManager.cs:508-513`).
- **Why it matters:** Rare for files the server probed, but possible for remote or STRM sources. Portico silently never resumes them; Jellyfin silently marks them watched. Neither is right.
- **Assessment:** **Unclear.** See B-12.13.

### A-12.9 Very short videos
- **Portico:** No minimum length. A 40-second clip is resumable from 2 seconds and watched at 36.
- **Jellyfin:** Under 300 seconds: never resumable; counts as played once past 5%.
- **Why it matters:** Home-video and extras libraries fill Continue Watching with clips in Portico. (Extras, kind 11, are excluded from the row, `home_rows.go:270`, but short films and home videos are not.)
- **Assessment:** **Portico behind**, small. A per-library "do not resume under N minutes" would match.

### A-12.10 Audiobook rules
- **Portico:** Resume after 1 second; finished at the profile's 95% of the **file** (`personal_activity.go:46-48,73-75`). A book's resume is "which file, what position" (`book_resume`).
- **Jellyfin:** Resume after 5 minutes, finished with under 5 minutes left. **Audiobookshelf:** finished with under 10 seconds left, or a percentage, per library.
- **Why it matters:** 95% of a 12-hour single-file audiobook is the last 36 minutes: stopping there marks it finished and clears the position. Time-remaining rules suit long files better than percentages.
- **Assessment:** **Portico behind**, small: add a "time remaining" rule for listening kinds.

### A-12.11 Rewatching clears the watched mark
- **Portico:** Once a replay passes the started threshold the title becomes unwatched and in progress (`personal_activity.go:117-127`; required by `PC/saved-state-and-downloads/contract.md` §4.2).
- **Jellyfin / Plex:** The title stays played and also has a resume point.
- **Why it matters:** In Portico an abandoned rewatch leaves a previously watched film looking unwatched, drops it from "watched" filters and counts, and (for an episode) can pull a finished show back into Continue Watching. In exchange the model stays a clean four-state machine.
- **Assessment:** **Trade-off (subjective)**, deliberate. Play counts (A-12.12) preserve "I have seen this"; consider showing "Watched before" from the count.

### A-12.12 Play count means "played to the end", counted once per playback
- **Portico:** `personal_play_counts` and `play_counts` increase when a playback first completes (`play_history.go:46-50`; `personal_activity.go:85-89`). Marking watched by hand counts nothing. "Reset viewing activity" deletes the profile's counts (`personal_activity.go:349`). Counts are only surfaced for songs today (`play_history.go:323-362`).
- **Jellyfin:** `PlayCount` increases at playback **start**, and manual mark sets it to at least 1.
- **Why it matters:** Portico's number is the more honest one. It is not shown for films or episodes, cannot be sorted on (Backlog), and is not exposed as "times watched".
- **Assessment:** **Portico ahead** in meaning, **behind** in use.

### A-12.13 "Last played" moves on every accepted report, including while paused
- **Portico:** `last_played_at` and `progress_activity.updated_at` are set to now on every report (`personal_activity.go:102,138`), paused or playing, even below the started threshold; `last_played_at` is written for reports from any session of the profile, not only the one that owns the resume point.
- **Jellyfin:** Only when the position is past the resume threshold or the item completed; not at the start of a replay of a played item.
- **Why it matters:** A paused player left overnight keeps the title at the front of Continue Watching and keeps bumping the viewer revision. A five-second accidental play of a finished episode becomes the show's "last active episode" and changes Next Up (the exact problem Jellyfin's comment describes).
- **Assessment:** **Portico behind**, small: update activity time only when position ≥ started threshold or completed, and not on paused reports.

### A-12.14 One canonical writer per item, with fences, instead of last write wins
- **Portico:** The most recently accepted explicit play owns resume and watched state; other sessions only add history. Manual marks fence older sessions (`S/playback/personal_progress.go:43-76`; `personal.go:161-168`).
- **Jellyfin:** Whichever report arrives last wins.
- **Why it matters:** Prevents a stale device from dragging the resume point back, and prevents a stopped session from undoing "mark watched". It also means a completion by a non-owning session is not recorded as watched (B-12.14).
- **Assessment:** **Portico ahead**, deliberate (contract §4.3). Completion should probably be accepted from any session.

### A-12.15 Explicit changes are idempotent, revision-checked and have an offline-conflict protocol
- **Portico:** Operation id, expected revision, 30-day receipts, a permanent "retired operation" fence, per-device sequence numbers, and a "needs resolution" state with choices (`personal.go:329-556`).
- **Jellyfin:** Plain POST/DELETE; last one wins.
- **Why it matters:** Retries and double taps are safe; two phones offline cannot silently fight. The cost is real complexity for a rarely hit case, and a client must hold the item's revision before it can favourite it (the dismissal helper has to read the title first: `CC/continue-watching.ts:34-58`).
- **Assessment:** **Portico ahead** on correctness, deliberate (P15). The container flags skip all of it (last write wins, `container_personal.go:16-18`), which shows the lighter form is acceptable for booleans; consider it for item-level My List and Favorite too.

### A-12.16 Two histories: the viewer's and the owner's
- **Portico:** `personal_history` is the viewer's (pause, clear); `play_history` is the owner's, permanent unless `playHistoryDays` is set, with names copied in so rows survive deletion (`0007_play_history.sql`; `play_history.go:16-19`).
- **Jellyfin:** An admin activity log (30 days by default) and plugins for statistics; users have no history page. **Plex:** owner sees all history.
- **Why it matters:** Portico gives the viewer a real History page and the owner a real audit trail. The viewer's "pause history" does not hide plays from the owner, which matches Plex and Jellyfin and is recorded as decided (Backlog, 2 Oct).
- **Assessment:** **Portico ahead.** Say plainly in the viewer's Privacy screen that the owner still sees plays.

### A-12.17 Pause watch history (incognito)
- **Portico:** A profile switch that stops recording resume, watched, History and taste (`personal_activity.go:80-84`).
- **Jellyfin / Plex:** None.
- **Why it matters:** Useful ("don't let this ruin my recommendations"). Side effect: while paused, nothing resumes where it was left and nothing gets marked watched.
- **Assessment:** **Portico ahead.** Consider a narrower variant: keep resume, skip History and taste.

### A-12.18 No "watched by several people on one screen"
- **Portico:** Watch Together is separate devices; each participant has their own evidence (contract §4.3). Nothing marks a title for a second profile sitting on the same sofa.
- **Jellyfin:** Additional users on a session; all are updated.
- **Why it matters:** Couples and families on one TV is the most common household case.
- **Assessment:** **Portico behind.** See F-12.3.

### A-12.19 Versions share one resume point
- **Portico:** Progress is per entity; a title's versions are sources of one entity, so position and watched are shared. (Backlog notes that a second file of the same film currently becomes a second title, which then has separate state.)
- **Jellyfin:** Per-version resume, completion propagated to all versions, Next Up continues in the same named version.
- **Why it matters:** Portico's is simpler and right when versions have the same length. A theatrical and an extended cut resume at the same timestamp, which is wrong by minutes.
- **Assessment:** **Trade-off (subjective).** Remembering the *version last played* per profile (for Next Up and resume) is the valuable part and is small.

**Group: Continue Watching and Next Up**

### A-12.20 One merged row, no option to split
- **Portico:** Unfinished titles and next episodes share the `continue` row (`home_continue_source.go:17-68`).
- **Jellyfin:** Two sections. **Plex:** one. **Streamyfin, Wholphin:** the user chooses.
- **Why it matters:** One row is simpler and matches Plex and the streaming services. People who use Next Up as a to-do list of shows lose it among films.
- **Assessment:** **Trade-off (subjective).** Deliberate ("Plex's shape").

### A-12.21 Next Up is anchored on the last *active* episode, not the last *watched* one
- **Portico:** `profile_show_activity.last_item_id` is whatever episode last had accepted activity; the offer is the first unwatched, unstarted episode strictly after it (`home_show_continuation.go:60-73`).
- **Jellyfin:** The most recently *played* (completed) episode, then the first unplayed after it.
- **Why it matters:** Portico follows where the viewer actually is (jump to S3 and Next Up follows). But a brief accidental play moves the anchor (A-12.13), and marking the anchor episode unwatched makes Next Up skip it (B-12.4).
- **Assessment:** **Trade-off (subjective).**

### A-12.22 Staleness window is the owner's, per library, and also applies to unfinished films
- **Portico:** 16 weeks by default, set by the owner per library, applied to films and episodes alike (`continue_policy.go:90-121`; `home_continue_source.go:21-25`).
- **Jellyfin:** The window is a per-user client setting (365 days by default) and applies to Next Up only; Continue Watching has no age limit.
- **Why it matters:** A viewer cannot keep a long-paused show on their own Home, nor shorten the window for themselves. The same window is reused on the show page, where it hides the resume target (B-12.2).
- **Assessment:** **Portico behind** on who controls it. Making it a profile preference with the library value as default is small.

### A-12.23 Season premieres are exempt from the window
- **Portico:** If the offered episode is episode 1 of a later season, the show is listed however old the last activity is (`home_show_continuation.go:79-84`).
- **Jellyfin:** No equivalent. **Plex:** same idea.
- **Why it matters:** A returning show reappears when its new season lands. Because the test is "episode 1 of a later season exists", not "a season arrived recently", shows the viewer walked away from at a finale stay eligible for ever (B-12.15).
- **Assessment:** **Portico ahead** in intent; the rule needs a recency bound.

### A-12.24 Specials
- **Portico:** A special (season 0) is offered only after another special; otherwise specials are skipped. A show never started offers specials last (`home_show_continuation.go:68`; `show_workspace_next.go:37`).
- **Jellyfin:** Specials with "airs before/after" data are placed inside the season order and offered in sequence when the server switch is on (`TVSeriesManager.cs:175-208`).
- **Why it matters:** Portico never interrupts a season with a special, which is the safe default; it also never offers the special that belongs between two seasons.
- **Assessment:** **Trade-off (subjective).** Jellyfin's needs metadata Portico may not store.

### A-12.25 No rewatch mode
- **Portico:** Next Up only ever offers unwatched episodes. Rewatching a finished show produces no Next Up on Home or on the show page.
- **Jellyfin, Wholphin:** "Enable rewatching in Next Up".
- **Assessment:** **Portico behind.** See F-12.2.

### A-12.26 Remove from Continue Watching, with Undo and show-level scope
- **Portico:** Per-item dismissal tied to the playback identity; the next real play brings it back; dismissing an episode hides the show until newer activity (`personal.go:501-510`; triggers `0001_baseline.sql` "continue_show_dismiss_*"; `home_show_continuation.go:77-78`).
- **Jellyfin:** None. **Emby, Plex, Audiobookshelf:** yes.
- **Why it matters:** A long-requested Jellyfin feature. The same dismissal also blanks the show page's Next Up (B-12.2).
- **Assessment:** **Portico ahead.**

### A-12.27 Marking episodes watched by hand does not "start" a show
- **Portico:** Only playback (or offline sync) writes `last_played_at` / `progress_activity`, which feed `profile_show_activity`. A show whose first three episodes were ticked by hand is not on Home; its own page does offer episode 4.
- **Jellyfin:** Manual mark sets `LastPlayedDate`, so the series enters Next Up.
- **Why it matters:** People migrating from another server or catching up after watching elsewhere tick episodes and expect the next one on Home.
- **Assessment:** **Portico behind**, small, but it interacts with "a manual action is not a play" (`personal.go:186-187`). Open question for Justin.

### A-12.28 Row size is the owner's, and the smallest library setting wins
- **Portico:** Maximum items default 40; with several libraries the **minimum** applies to the merged row (`continue_policy.go:92,111-113`).
- **Jellyfin:** The client asks for 12–24.
- **Why it matters:** Setting one small library to 5 shrinks everyone's merged row to 5.
- **Assessment:** **Trade-off (subjective)**; the min-across-libraries rule is surprising.

### A-12.29 Continue Listening is audiobooks only; music has no "recently played"
- **Portico:** Songs keep no position, so only audiobook files qualify (`personal_activity.go:128-130`; `home_rows.go:275-280`). No recently played / most played rows for music on Home or Discover.
- **Jellyfin:** Music library "Recently played" and "Frequently played". **Plex, Navidrome:** both.
- **Assessment:** **Portico behind.** See F-12.9.

**Group: Favourites, lists, ratings**

### A-12.30 Four separate personal signals
- **Portico:** My List (intent to watch), Favorite (heart), star rating (half stars), Not interested. Like/dislike was removed on purpose (`0003_drop_reaction.sql`).
- **Jellyfin:** Favourite, plus likes and a numeric rating that the official UI does not expose. **Plex:** stars and a cloud watchlist; no heart for video.
- **Why it matters:** Portico's set is the fullest and each feeds the taste model with its own weight (`S/compactcatalog/rec_profile.go:105-133`). Four controls is also the most to explain.
- **Assessment:** **Portico ahead.**

### A-12.31 My List is local to one server and holds only what is in the library
- **Portico:** Stored on the selected server; the contract forbids Hosted from receiving titles (`PC/saved-state-and-downloads/contract.md` §2).
- **Plex:** Account-level, includes titles nobody owns, shows availability. **Plezy:** external watchlists from Trakt, Simkl, MAL, AniList, Plex.
- **Why it matters:** "I want to watch this when it arrives" cannot be expressed in Portico. Privacy is the reason and it is deliberate.
- **Assessment:** **Trade-off (subjective).** See F-12.6 for a server-local wish list.

### A-12.32 Which kinds carry which flag
- **Portico:** My List: films, episodes, songs, files, shows, books. Favorite: those plus albums and artists. Star rating: playable items only. Watched default: show, season, album, book (`container_personal.go:54-66`; `personal.go:317-327`).
- **Jellyfin:** Favourite and played on everything, including seasons, collections, people and playlists.
- **Why it matters:** No star rating on a show, album or artist (Backlog), no favourite season, person or collection.
- **Assessment:** **Portico behind**, known.

### A-12.33 My List has no "when I saved it"
- **Portico:** Sorts are title, date added to the library, last played, year, duration, progress; default title (`saved.go:15-22,68-70`). No timestamp is stored for the save.
- **Plex / Netflix-style lists:** newest saved first.
- **Assessment:** **Portico behind**, small (one column).

### A-12.34 Playlists
- **Portico:** Stable entry ids, duplicates allowed, compare-and-swap reordering, named-profile shares with viewer/editor roles (100 max), 200 entries per write, any playable kind can be mixed (`playlists.go:21-22,43-53`).
- **Jellyfin:** Audio or video playlists, public or shared-with-edit, stored as files; imports `.m3u` playlists found in libraries. **Plex / Navidrome:** smart playlists.
- **Why it matters:** Portico's editing model is the safest. Missing: smart playlists (saved views are queries but not play-in-order lists), import/export, a public-to-server switch.
- **Assessment:** **Portico ahead** on integrity, **behind** on smart and import.

### A-12.35 Collections: two kinds
- **Portico:** Library collections (owner-made or from metadata) and viewer-owned saved collections that can be made visible to the whole server (`resources.go:106-110`).
- **Jellyfin:** One kind, server-wide, edited by users with a permission.
- **Assessment:** **Portico ahead.** Neither kind can become a Home row (F-12.7).

**Group: Home**

### A-12.36 The server composes Home; clients only draw it
- **Portico:** Rows, order, titles, totals, paging and the hero come from `GET /v1/home` (`home_rows.go:785-881`).
- **Jellyfin:** Each client assembles its own home from several calls and its own settings; clients differ in which sections exist.
- **Why it matters:** Every Portico client shows the same Home with one implementation; a new row type ships without client releases. Home's cost is concentrated in one request (measured 259 ms cold at 10k films, `PA/05`), where Jellyfin's rows load independently and progressively.
- **Assessment:** **Portico ahead**, deliberate (thin clients). Consider letting clients fetch below-the-fold rows lazily (the per-row endpoint exists).

### A-12.37 Home layout is per profile and identical on every device
- **Portico:** `home.rowOrder` and `home.hiddenRowIds` are profile-server scope (`preferences.go:208-209`).
- **Jellyfin:** Per client app, so the TV and the phone differ (by accident of design). **Wholphin:** per user, synced among Wholphin devices.
- **Why it matters:** One arrangement to maintain. A TV in the living room cannot have a different Home from a phone (for example no music rows on the TV).
- **Assessment:** **Trade-off (subjective).** The registry already has a device-class scope; allowing the two layout keys in it is small.

### A-12.38 The hero is "what you were last in the middle of"
- **Portico:** First Continue entry, newer of watching and listening; no hero otherwise (`home_rows.go:889-902`). Decided 2 Oct 2026.
- **Others:** Jellyfin web has none; Streamyfin and several TV clients use a carousel of continue / next up / recently added, with filters.
- **Why it matters:** Always personal, never an advert. A new profile, or one that finished everything, has no hero at all; a household that hides Continue rows cannot (they are required).
- **Assessment:** **Trade-off (subjective)**, deliberate. A fallback (newest addition, or top recommendation) for the empty case is small.

### A-12.39 No library tiles on Home
- **Portico:** Libraries live in navigation, pinned and ordered per profile (`S/catalog/library_pins.go`).
- **Jellyfin:** "My Media" tiles are the first home section by default, and forced on TV.
- **Assessment:** **Trade-off (subjective).**

### A-12.40 Recently added: per library, one card per work, watched titles included
- **Portico:** One shelf per library, folded to works, newest 100 (`home_recent_source.go:13-116`).
- **Jellyfin:** Latest per library, episodes grouped by series, **played items hidden by default** (`HidePlayedInLatest`), libraries excludable per user.
- **Why it matters:** Portico's shelves keep showing what the viewer already watched the night it arrived.
- **Assessment:** **Portico behind**, small: a profile preference "hide watched in Recently added".

### A-12.41 The owner cannot arrange or add rows for everyone
- **Portico:** Rows are fixed types plus each profile's own saved views. The owner can only switch community activity on or off.
- **Plex:** Owner-managed recommended rows; collections promoted to all users' Home. **Streamyfin plugin:** admin-defined home sections and locked settings.
- **Assessment:** **Portico behind.** See F-12.7.

### A-12.42 No merging across servers
- **Portico:** One server at a time; the contract excludes cross-server aggregation (contract §1).
- **Plezy:** Merged Continue Watching, hubs and search across Plex, Jellyfin and Emby servers.
- **Assessment:** **Trade-off (subjective)**, deliberate. Hosted already knows a person's servers, so a client-side merged Continue Watching is feasible later without Hosted seeing titles.

### A-12.43 Freshness by revision, never by time
- **Portico:** Rows and cursors carry catalogue and viewer revisions; a changed revision answers `stale_continuation`; memos are keyed by every revision read (`home_rows.go:436-469`; `rec_engine.go:459,498-510`).
- **Jellyfin:** Client query caches with time-based staleness; similar-items lists cached on disk for a provider-defined period.
- **Why it matters:** A watch shows on the next Home load, always. The cost is that every 10-second progress report invalidates the profile's Home and recommendation memos (B-12.20). Each row still publishes a `cacheTtlSeconds` that no client reads (`home_rows.go:53`; `CC/home.ts:16`).
- **Assessment:** **Portico ahead**, deliberate. Remove the unused TTL field or decide what it means.

### A-12.44 Exact totals and resumable paging on every row
- **Portico:** Every row has an exact total, a signed cursor and an anchor for restoring position (`home_rows.go:478-558`).
- **Jellyfin:** Fixed-size rows; "see all" is a different library query.
- **Assessment:** **Portico ahead.**

**Group: Recommendations**

### A-12.45 A maintained taste model versus per-request category lists
- **Portico:** Typed features per work, best-first posting lists, a two-horizon taste vector per profile updated exactly on each action, TF-IDF rescoring with quality and similar-title evidence (`rec_engine.go:15-36, 525-662, 735-779`; `rec_profile.go:72-136`).
- **Jellyfin:** At request time, take 7 recently played films and 10 random favourites and list similar films for each; similarity is an overlap count (see "How the others do it").
- **Why it matters:** Portico can rank a whole library for a person, explain rows by genre or person, and respond to dislikes; Jellyfin's results change randomly between loads and only exist for films.
- **Assessment:** **Portico ahead**, by a wide margin. The cost is the amount of machinery to keep correct (triggers, jobs, overlays, memos).

### A-12.46 Coverage: every media family, on Home, in libraries, on title pages and as a sort
- **Portico:** Films, shows, albums, books; Home rows, per-library Discover, title-page rows, and a "For you" browse sort (`PA/05`).
- **Jellyfin:** Films only, one tab. "More like this" exists for films, series, albums, artists.
- **Assessment:** **Portico ahead.**

### A-12.47 Explanation of why something is recommended
- **Portico:** Personal rows are named for their reason ("*Genre* for you", a person's name, "Next in the series"). The main "Recommended for you" row has no per-title reason. The row shape has an `explanation` field that nothing fills and no client shows (`home_rows.go:41,175`).
- **Jellyfin:** Every category is "Because you watched *X*" / "Because you liked *X*" / "Directed by" / "Starring".
- **Why it matters:** "Because you watched X" is the most trusted form of recommendation label and is cheap for Portico: the similar-title seeds and strongest matching feature are already in hand during ranking.
- **Assessment:** **Portico behind** on this one visible point. See F-12.8.

### A-12.48 Feedback controls
- **Portico:** Not interested (with Undo) on every recommendation card, on works as well as items; Reset recommendations; low star ratings count against; abandonment (started, under 25%, untouched 21 days) counts mildly against (`rec_engine.go:288-334`; `rec_profile.go:119-130`).
- **Jellyfin / Plex:** None of these.
- **Assessment:** **Portico ahead.** Not interested hides one title; there is no "less like this" for a genre or person.

### A-12.49 Cold start
- **Portico:** With no taste, ranks by a vote-weighted quality score per library (`rec_engine.go:619-626`); Trending (provider feeds) needs the owner's metadata consent.
- **Jellyfin:** Nothing until the user has played or favourited films.
- **Assessment:** **Portico ahead.** No onboarding "pick a few you like" step (F-12.14).

### A-12.50 Restricted (kids) profiles
- **Portico:** Restrictions are joined into candidate eligibility, re-applied on every read of a memoised list, and part of the memo key (`rec_engine.go:787-806`; `recommendation_engine.go:355-362`; `rec_engine.go:459`). Other members' activity only counts for titles the viewer may see.
- **Jellyfin:** Normal access filtering of results.
- **Why it matters:** I looked for a path where a restricted profile could be shown, or could infer, a blocked title through recommendations and did not find one. A restricted profile's *own* taste is learned normally.
- **Assessment:** **Portico ahead.**

### A-12.51 Community signals are opt-in twice and need at least two people
- **Portico:** "Popular on this server" and "Viewers also watched" need the owner's switch and each member's `privacy.showActivityToMembers`, exclude members who paused history, and require two distinct viewers (`recommendation_engine.go:25-26,186-216,385-431`; `rec_title.go:137-206`).
- **Jellyfin:** No such rows. **Plex:** friends' activity through the cloud account.
- **Why it matters:** A threshold of two is weak in a two-person household (B-12.21).
- **Assessment:** **Portico ahead**; raise the minimum or hide the row for the people who form the minimum.

### A-12.52 Daily rotation and exploration
- **Portico:** The choice among close personal rows, tie-breaks and one exploratory card in eight are seeded by profile and **UTC date** (`rec_engine.go:489,659`; `rec_generators.go:353-358`).
- **Jellyfin:** Random on every request.
- **Why it matters:** Stable through the day and identical across a person's devices. The day changes at 00:00 UTC, not at the viewer's midnight (B-12.16).
- **Assessment:** **Portico ahead**; use the server's local day.

### A-12.53 Cost model
- **Portico:** Library-side work when the library changes, profile-side work when the profile acts, bounded reads per request, memo by exact revisions (256 entries, `rec_engine.go:982`).
- **Jellyfin:** Queries per request plus a disk cache for provider similar lists.
- **Assessment:** **Portico ahead** at scale; unmeasured on weak hardware (see Evidence boundary).

**Group: Discover, search**

### A-12.54 "Discover" means inside the library
- **Portico:** A library's first tab: continue, recommended, trending-in-your-library, recently added, personal rows (`discovery.go:303-462`). Nothing about titles the server does not have.
- **Plex:** Discover is the whole world of film and TV, with a universal watchlist. **Plezy, Streamyfin, Wholphin:** external catalogues and Seerr requests.
- **Assessment:** **Trade-off (subjective)**, consistent with "Hosted stays cheap" and privacy. See F-12.5, F-12.6.

### A-12.55 Search engine: token-prefix full-text on one title
- **Portico:** FTS5 over the entity's single display title; every token must match as a word prefix; ranked by BM25 with exact-title and title-prefix boosts; diacritics folded (`search.go:114-135,154-159`; `derive_search.go:17-23`; `0001_baseline.sql:326-328`).
- **Jellyfin:** Substring match on the cleaned name (and original title). **Streamyfin + Marlin:** an external typo-tolerant engine.
- **Why it matters:** Portico is fast and bounded at millions of titles and gives sensible ranking. It does not find a title by its original or alternative name, by a word inside a word ("spider" will not find "Superspider"), or with a typo. Jellyfin has no typo tolerance either.
- **Assessment:** **Portico ahead** on scale and ranking, **behind** on recall. See F-12.10.

### A-12.56 Search groups and budgets
- **Portico:** Nine typed groups, each paged and sortable, each with its own 600 ms budget so one slow group cannot fail the response; people are a first-class group; restrictions applied in the query (`search.go:22-47`).
- **Jellyfin:** Search hints across types including genres, studios, playlists, collections, programmes.
- **Why it matters:** Portico has no collections, playlists, genres or authors groups; a show cannot be sorted by year in search (`search.go:165-167`).
- **Assessment:** **Portico ahead** on structure, **behind** on breadth.

### A-12.57 Recent searches
- **Portico:** The server has a per-profile search history (20 entries), a preference to turn it off, and read/clear routes (`S/catalog/search_history.go`; `S/httpapi/search.go:162-180`). No client uses them: web and native each keep recents in memory until the app or tab closes (`W/screens/search/Search.tsx:33-40,110-120`; `N/screens/Search.tsx:26-33`).
- **Jellyfin:** No recent searches in the official web client.
- **Assessment:** **Unclear**; two half-features. See B-12.17.

**Group: Preferences and sync**

### A-12.58 One registry, two scopes, server-stored
- **Portico:** Every preference is declared once with type, range, default and scope; values follow the profile, with device-class overrides for appearance, quality and delivery (`preferences.go:22-29,150-215`).
- **Jellyfin:** Three stores: server-side user configuration (languages, subtitle mode, library order), per-client display preferences (home sections, per-view sorts), and each app's local storage.
- **Why it matters:** In Portico a new phone has the person's skip settings and Home at first sign-in; in Jellyfin each app starts from defaults.
- **Assessment:** **Portico ahead.**

### A-12.59 The owner cannot set or lock viewers' preferences
- **Portico:** Preferences are the profile's. The owner sets library rules and server policy, not a profile's defaults.
- **Streamyfin plugin:** Admin pushes settings to all clients, with per-setting locks. **Jellyfin:** admin can edit each user's server-side configuration.
- **Assessment:** **Portico behind** for families (set a child's subtitle language, lock autoplay off). Medium.

### A-12.60 Per-profile language and subtitle defaults
- **Portico:** Ordered lists of preferred audio and subtitle languages, subtitle mode off / forced / always, size and background, all per profile and synced (`preferences.go:179-185`).
- **Jellyfin:** One preferred audio and one subtitle language, modes default / always / only forced / none / smart, and "remember my last choice per item".
- **Why it matters:** Lists are better than single values. Portico has no "smart" mode (subtitles only when the audio is not in my language). Domain 04 covers track selection; noted here only for the per-profile angle.
- **Assessment:** **Trade-off (subjective).**

### A-12.61 No third-party watch-state sync
- **Portico:** None (no Trakt, Simkl, Letterboxd, AniList, MAL, Last.fm, ListenBrainz code anywhere in the server or clients).
- **Jellyfin:** Trakt plugin and others. **Plezy:** five trackers built in. **Navidrome:** Last.fm and ListenBrainz. **Infuse:** Trakt.
- **Assessment:** **Portico behind.** See F-12.1.

### A-12.62 No import or export of watch state
- **Portico:** No route to export a profile's watched/ratings/lists, or import from Plex, Jellyfin, Trakt or a file. Open data is the answer for export (plain SQLite), but the tables are not self-describing (integer ids, computed watched).
- **Jellyfin:** Community tools and the Trakt plugin serve as the bridge; mark-played accepts a date.
- **Assessment:** **Portico behind.** See F-12.4.

### A-12.63 Offline progress is reconciled by the server with last-write-wins
- **Portico:** A dedicated endpoint takes a batch of offline observations and applies each if it is newer than the last offline observation and the last online activity (`S/downloads/progress.go:66-198`). The client states watched; the server does not apply its thresholds on this path.
- **Jellyfin clients / Plezy:** The client replays ordinary progress and mark-played calls.
- **Why it matters:** A purpose-built endpoint with receipts is cleaner. It bypasses the fences that protect explicit marks (B-12.3) and writes neither history (B-12.5).
- **Assessment:** **Trade-off**; the endpoint needs to join the same rules as online evidence.

### A-12.64 Operating-system home integrations
- **Portico:** tvOS Top Shelf and an Android TV equivalent exist (`apps/portico-react-native/apps/native/tvos/TopShelf/ContentProvider.swift`, `android/.../PorticoTopShelf.kt`); not reviewed here.
- **Others:** Streamyfin Top Shelf; Wholphin and Plezy publish to Android TV "Watch Next".
- **Assessment:** **Unclear** (not read). Parity appears to exist.

## Missing features and capabilities to consider

### F-12.1 Sync watched state, ratings and lists with outside trackers
- **What:** Send plays and marks to Trakt / Simkl / MDBList (film and TV), AniList / MyAnimeList (anime), Last.fm / ListenBrainz (music); optionally read history and watchlists back.
- **Who has it:** Plezy, in the client, with a write queue, batching for whole-show marks and rate-limit care (`plezy/lib/services/trackers/tracker_coordinator.dart:26-44`). Jellyfin via the Trakt plugin. Navidrome on the server with a buffered scrobbler (`navidrome/core/scrobbler/buffered_scrobbler.go`). Infuse (Trakt).
- **Value:** High for enthusiasts; it is also the practical import/export path between products.
- **Size:** Large. Server-side per-profile connectors fit Portico best (one implementation, works from every client, respects pause-history). The play history and intent tables already provide the events.
- **Conflicts:** None with Hosted (the server would talk to the tracker directly). Needs an outbound-policy and privacy statement per profile.
- **Opinion:** Worth planning the seam now (an event feed of "completed", "marked", "rated") even if no connector ships at release.

### F-12.2 Rewatch mode for Next Up
- **What:** When the viewer is rewatching a finished show, offer the episode after the one just played even though it is already watched.
- **Who:** Jellyfin (`EnableRewatching`, `jellyfin/Emby.Server.Implementations/TV/TVSeriesManager.cs:115-166`), Wholphin.
- **Value:** Medium; without it a rewatch has no Continue Watching entry and no show-page Next Up between episodes.
- **Size:** Small to medium: in `home_show_continuation.go`, when no unwatched successor exists and the last active episode was completed recently, offer the next episode regardless of watched state.
- **Opinion:** Do it; it is also the cleanest fix for the "show page falls back to S1E1" problem in rewatch cases.

### F-12.3 Mark a title for other people watching on the same screen
- **What:** "Also watching: Sam" on the player or after playback, so progress and watched apply to a second profile.
- **Who:** Jellyfin (additional users on a session, `SessionManager.cs:1647-1692`).
- **Value:** High in households; it is the usual reason two profiles' Continue Watching disagree.
- **Size:** Medium. Portico's evidence model can take a second profile as another consumer of the same playback evidence; consent and PIN rules for the added profile need a decision.
- **Opinion:** One of the most valuable gaps in this domain.

### F-12.4 Import and export of a profile's state
- **What:** Export watched, progress, ratings, My List, favourites and playlists to a documented file keyed by provider ids; import from that file, from Plex or Jellyfin, or from a Trakt export. Accept a "watched on" date.
- **Who:** Jellyfin's mark-played takes a date; migration tools rely on provider-id keys.
- **Value:** High at launch: people arriving from Plex or Jellyfin will not re-tick years of history.
- **Size:** Medium for the file format and import by provider id; each source adapter is extra.
- **Conflicts:** None. It supports the open-data principle.
- **Opinion:** Strongly worth having before release.

### F-12.5 Request a missing title (Seerr integration)
- **What:** A link to an Overseerr/Jellyseerr/Seerr instance: search it from Portico, request a title, show request status.
- **Who:** Plezy, Streamyfin, Wholphin (client-side).
- **Value:** Medium-high for shared servers.
- **Size:** Medium as an optional owner-configured link; large if Portico builds its own request queue.
- **Opinion:** Record as an option. A minimal form is an owner-set URL shown in empty search results.

### F-12.6 A wish list for titles the server does not have
- **What:** My List entries for titles by provider id that light up when the title arrives.
- **Who:** Plex (cloud watchlist), Plezy (tracker watchlists).
- **Value:** Medium; pairs with F-12.5.
- **Size:** Medium (a server-local table of provider ids per profile; matching already exists for the dataset by provider id).
- **Conflicts:** Fine for privacy if it stays on the server.

### F-12.7 Owner-curated Home rows
- **What:** The owner promotes a collection, a saved view or a library's recent shelf to every profile's Home (or to chosen profiles), and sets the default Home order for new profiles.
- **Who:** Plex (managed recommendations, collections visible on Home), Streamyfin plugin (admin home sections).
- **Value:** Medium-high: "Family movie night picks", "Christmas", a kids' shelf.
- **Size:** Medium. Server-visible saved collections and view rows already exist; what is missing is a server-level list of row ids merged into `homeSpecs`.
- **Opinion:** Fits the server-authored Home model very well.

### F-12.8 "Because you watched X" labels
- **What:** A reason on recommended cards or rows, from the strongest contributing seed or feature.
- **Who:** Jellyfin (category names), every streaming service.
- **Size:** Small to medium: the ranking already knows each candidate's similar-title seeds and matching features; the row shape already has an unused `explanation` field.
- **Opinion:** High value for trust, low cost.

### F-12.9 Music: Recently played, Most played, Jump back in
- **What:** Rows of albums, artists and playlists by recent and frequent play; sorting songs by plays.
- **Who:** Jellyfin music library, Plex, Navidrome, Finamp.
- **Size:** Small to medium: per-profile play counts exist for songs; an album-level "last played" projection is needed.
- **Opinion:** Music Home is thin without it; Continue Listening is audiobooks only.

### F-12.10 Search recall
- **What:** (a) Index original and alternative titles and sort titles; (b) typo tolerance (for example a trigram or edit-distance pass over a small vocabulary of title words when the exact search finds little); (c) match inside words; (d) "show s02e03" style queries; (e) groups for collections, playlists, genres and authors; (f) search within the current library.
- **Who:** Jellyfin (substring, original title), Marlin for Streamyfin (typo tolerant), Plex (broad).
- **Size:** (a) small, (e) small each, (b) medium, (c) medium (FTS5 trigram tokenizer has an index-size cost at millions of rows).
- **Conflicts:** Must stay O(visible); a vocabulary-based correction keeps it bounded.
- **Opinion:** (a) is a clear gap for anime and foreign films and should not wait.

### F-12.11 Hide watched titles from Recently added; exclude a library from rows
- **Who:** Jellyfin (`HidePlayedInLatest`, `LatestItemsExcludes`, `MyMediaExcludes`).
- **Size:** Small each. Portico can already hide a library's recent shelf; it cannot keep a library out of Continue Watching, Recommended or search for one profile without removing access.

### F-12.12 A different Home per kind of device
- **What:** Allow the Home layout keys in the device-class scope.
- **Who:** Jellyfin (per client, by accident), Wholphin.
- **Size:** Small on the server; a Customize Home toggle "only on this kind of device" on clients.

### F-12.13 Viewer control over Continue Watching
- **What:** Split Next Up from Continue Watching; per-profile window (weeks); "never show this show here".
- **Who:** Jellyfin (max days, per user), Streamyfin and Wholphin (merge/split).
- **Size:** Small to medium.

### F-12.14 Teaching the recommender directly
- **What:** A first-run "pick a few you like"; "less like this" for a genre or person; "I've seen this" on a recommendation card (marks watched without leaving).
- **Who:** Streaming services; not Jellyfin or Plex.
- **Size:** Small each on top of the existing signals.

### F-12.15 Smart playlists, and playlist import/export
- **What:** Play a saved view in order as a live playlist; import and export M3U.
- **Who:** Plex and Navidrome (smart), Jellyfin (M3U import).
- **Size:** Medium.

### F-12.16 Saved-list polish
- **What:** Sort My List by when saved; star rating on shows, albums and artists (Backlog); favourite people and collections; favourite seasons.
- **Size:** Small each.

### F-12.17 History polish
- **What:** Remove one History entry; "times watched" and "last watched" on a title page from the existing counts; for the owner, "who watched this" (Backlog) and a personal yearly summary.
- **Who:** Plex (history entries), Playback Reporting / Jellystat for Jellyfin.
- **Size:** Small each.

### F-12.18 Remember the version a person watches
- **What:** Next Up and resume open the same named version (4K, extended, dubbed) the profile last chose for that show.
- **Who:** Jellyfin (`TVSeriesManager.cs:269-300`).
- **Size:** Small once versions are merged into one title (Backlog).

### F-12.19 Specials in sequence
- **What:** Use "airs before/after" data to place specials in Next Up and season lists.
- **Who:** Jellyfin (`DisplaySpecialsWithinSeasons`).
- **Size:** Medium; depends on metadata.

### F-12.20 Completion rules by time remaining, and a minimum length for resume
- **What:** "Finished when under N minutes remain" for audiobooks and long files; "do not keep resume points for items under N minutes".
- **Who:** Jellyfin (audiobooks, 300-second minimum), Audiobookshelf (10 seconds remaining, per library).
- **Size:** Small.

### F-12.21 A hero when nothing is in progress
- **What:** Fall back to the top recommendation or newest addition; optionally a short carousel.
- **Who:** Streamyfin (configurable carousel), most TV clients.
- **Size:** Small. Deliberately not done today (2 Oct decision: no Featured row).

### F-12.22 Owner-set defaults and locks for a profile's preferences
- **Who:** Streamyfin plugin, Jellyfin admin user settings.
- **Size:** Medium (a third scope above profile in the registry).

### F-12.23 "Mark everything before this as watched"
- **What:** On an episode: mark all earlier episodes watched in one action.
- **Who:** Trakt-style apps, Infuse.
- **Size:** Small with the container machinery (a watermark by position rather than by time would be new).

### F-12.24 Upcoming episodes
- **What:** A row or tab of episodes airing soon for shows the profile watches.
- **Who:** Jellyfin ("Upcoming" in TV libraries), Trakt-driven clients.
- **Size:** Medium; needs future air dates from the provider. Portico has "Recently aired" only.

### F-12.25 Merged Continue Watching across a person's servers
- **Who:** Plezy.
- **Size:** Medium, client-side only. Out of scope by contract today.

## Possible bugs and risks in Portico

None of these was run. "Confirmed by reading" means the code path is unambiguous on the page; it still needs a runtime check before anyone acts on it. None is listed in `PA/Backlog.md` unless stated.

### B-12.1 "Mark season / show / album / book as watched" leaves in-progress members unwatched
- **What goes wrong:** Any member with its own intent is immune to a later container "watched". A member gets an intent of *not watched* the moment playback passes the started threshold (`S/catalog/personal_activity.go:117-127`), or when it was once marked unwatched by hand. After "Mark season watched", that episode still reads unwatched, keeps its resume point (the container path never touches `progress`) and stays in Continue Watching. For an audiobook being listened to, "mark book finished" leaves the current file unfinished and the book still resumes.
- **Evidence:** `S/personalstate/watched.go:49-52` (item intent is consulted first; only `cleared_through`, set on *un*watch, can retire it); `S/catalog/container_personal.go:154-174` (watched=true writes the default and nothing else); the test encodes it (`S/catalog/bulk_jobs_test.go:152-163`, expected `[false, true, true]`).
- **How the comparison surfaced it:** Jellyfin's cascade writes every child and resets positions (`jellyfin/MediaBrowser.Controller/Entities/Folder.cs:1956-1992`).
- **Confidence:** Confirmed by reading (it is also tested as intended behaviour). **Severity:** High as a user-visible wrong result; the audiobook case is the worst.

### B-12.2 A show's page loses its "Next up" when the show is old, dismissed, or being rewatched; clients then offer the wrong episode
- **What goes wrong:** The show page takes Next Up from the same query as Home's row, including the library's staleness window and the "remove from Continue Watching" dismissal (`S/catalog/show_workspace_next.go:13-22`; window at `home_show_continuation.go:79`; dismissal at `:77-78`). For a show that has been started, the "first unwatched episode" fallback is skipped (`show_workspace_next.go:36`). So for a show last watched 17 weeks ago at S3E5, or one the viewer removed from Continue Watching, or one where every later episode is already watched, the server sends no `nextUp`. The web client then applies an older local rule to the first page of whichever season the server opened (in progress, else first unwatched, else **the first playable episode**) (`W/screens/detail/Show.tsx:141-151,168`), and native does the same (`N/screens/Show.tsx:145-156`, `N/screens/title/facts.ts:34-36`). The main button can read "Play S1 E1" for someone in the middle of season 3.
- **How surfaced:** Jellyfin's cut-off is passed only by the home section, not by the series page.
- **Confidence:** Confirmed by reading for the server; Probable for the exact button a user sees (depends on which season opens). **Severity:** Medium.
- **Fix direction:** The show page should ignore the window and the dismissal, and fall back to the first unwatched episode after the last active one.

### B-12.3 A stale offline observation can undo a newer explicit "Mark watched / unwatched"
- **What goes wrong:** Offline progress sync accepts an entry unless a newer *offline* observation or a newer *online activity time* exists (`S/downloads/progress.go:136-170`). An explicit mark does not move the online activity time (`personal.go:191` updates `state` only) and the sync does not consult the explicit-mark fences. So: play a download offline on Sunday to minute 30; on Monday mark the film watched from the web; on Tuesday the phone syncs. The Sunday entry is applied: position 30 minutes, state paused, and, if the client sent `watched:false`, a new intent stamped *now* (`progress.go:201-228`, `personalstate.Write`). Either way the film is back in Continue Watching (the film query does not test watched, `home_rows.go:286-291`); with `watched:false` it is also unwatched again.
- **Related:** an offline `watched:true` is also stamped now, so it beats a "mark show unwatched" done after the offline viewing.
- **How surfaced:** Plezy fixed the same class of bug in its client queue (`plezy/lib/services/offline_watch_sync_service.dart:113-125`). The contract requires the opposite ("Explicit Mark Watched/Unwatched … take precedence over older playback evidence", `PC/saved-state-and-downloads/contract.md` §4.4).
- **Confidence:** Confirmed by reading. **Severity:** Medium (needs downloads plus a second device).

### B-12.4 Marking the last-played episode unwatched makes Next Up skip it
- **What goes wrong:** Next Up is the first unwatched episode *strictly after* the show's last active episode (`home_show_continuation.go:65-67`). If the viewer marks that same episode unwatched (to see it again), it has no resume point, so the "unfinished" branches do not pick it, and the "next" branch starts after it. Home and the show page offer the following episode.
- **How surfaced:** Jellyfin anchors on the last *played* episode, so un-marking moves the anchor back.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-12.5 Plays of downloads while offline never reach either history or the play counts
- **What goes wrong:** `applyProgress` writes the resume point, activity, watched intent and `personal_items`, and nothing else (`S/downloads/progress.go:201-228`). No `personal_history` row (the viewer's History page), no `play_history` row (the owner's record: "the owner always sees every play"), no `play_counts` / `personal_play_counts`. The recommender still learns the finish through the intent, but "watched again" and album weights that use completion counts do not.
- **Confidence:** Confirmed by reading (`grep` finds no other writer of these tables outside `S/catalog/play_history.go` and `personal_activity.go`). **Severity:** Medium for the owner's audit trail; Low otherwise.

### B-12.6 Container "watched" marks are invisible to the recommender, and season-level marks do not stop a show being recommended
- **What goes wrong:** A work's signal counts items finished by history, by `personal_items.watched` or by item intents (`S/compactcatalog/rec_profile.go:80-83`); it never reads `container_personal_state`, and no trigger queues a taste job when a container is marked (triggers at `0001_baseline.sql` "rec_profile_*", none on that table). Eligibility excludes a work only when the **work itself** (show, album, book) carries a watched default (`S/catalog/rec_engine.go:805`). So (a) marking a show watched hides it from recommendations but teaches the taste model nothing, and (b) marking each **season** watched leaves the show unengaged and still recommended.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (matters most right after someone ticks off what they have already seen, which is exactly when first impressions of recommendations form).

### B-12.7 The offline path lets the client decide "watched", and writes times in a different text layout
- **What goes wrong:** Online, the server applies the library's threshold and credits rule. Offline, the client's `watched` is stored as given (`progress.go:216-228`), so a client with a different rule (or none) produces different results for the same viewing. Separately, this path writes `progress_activity.updated_at` and `personal_items.last_played_at` as RFC 3339 without milliseconds (`progress.go:212`), while the online path writes a fixed millisecond layout (`personal_activity.go:51`). These columns are compared and sorted as text (Continue Watching order and window, `profile_show_activity` trigger comparisons, cursors). Two values in the same second sort wrongly (`…:05Z` sorts after `…:05.900Z`).
- **Confidence:** Confirmed by reading. **Severity:** Low (ordering error is under one second); a latent trap for any future exact comparison.

### B-12.8 A report of state `ended` marks the title watched at any position
- **What goes wrong:** `completed := state == "ended" || …` (`personal_activity.go:66`). If a player reaches "ended" early (a truncated file, a stream that stops, a wrong duration), the title is watched and its resume point cleared, whatever the position.
- **How surfaced:** Jellyfin's stop report carries the position and applies the percentage; a failed playback changes nothing (`SessionManager.cs:1176-1215`).
- **Confidence:** Speculative — needs a runtime check of when each client reports `ended`. **Severity:** Low to Medium.

### B-12.9 `credits` completion mode never completes a title that has no approved credits marker, short of playing to the very end
- **What goes wrong:** In `credits` mode the percentage is ignored for video (`personal_activity.go:66`), and only an owner-approved marker counts (`continue_policy.go:74-88`). A title with no approved marker stopped at 99% stays "in progress" with a resume point at 99%.
- **How surfaced:** Plex's equivalent modes are documented as marker-based with the percentage still configured alongside.
- **Confidence:** Confirmed by reading. **Severity:** Medium for an owner who selects the mode (not the default).

### B-12.10 The played threshold a viewer sees is not the one applied to video
- **What goes wrong:** The profile preference `playback.playedThresholdPercent` defaults to 95 (`S/operations/preferences.go:178`). For films and episodes the server uses the **library's** value (default 90) unless the profile has stored its own value (the preference's source is not `default`, `personal_activity.go:45-48`). A settings screen that shows the effective preference shows 95% while video completes at 90%. And once a profile sets any value, the owner's per-library choice is silently ignored for that profile.
- **Confidence:** Confirmed by reading (server); I did not check how the clients label the control. **Severity:** Low.

### B-12.11 The "started" percentage preference does almost nothing
- **What goes wrong:** Started = `min(30 s, percent × duration)` (`personal_activity.go:69-72`). At the default 5% the percentage only matters for items under 10 minutes; at the maximum 25%, under 2 minutes. Raising it never delays the resume point of a normal film or episode.
- **Confidence:** Confirmed by reading. **Severity:** Low (a control that does not do what its name says).

### B-12.12 A paused or momentary play moves "last active", which reorders Continue Watching and re-anchors Next Up
- **What goes wrong:** Every accepted report sets `last_played_at` and the activity time (`personal_activity.go:102,138`), and triggers copy the newest into `profile_show_activity` with that episode as the anchor. Opening a finished episode for five seconds makes it the show's anchor; a player paused for hours keeps moving to the front (reports every 30 seconds while paused).
- **How surfaced:** Jellyfin's comment at `SessionManager.cs:850-855`.
- **Confidence:** Confirmed by reading for the writes; Probable for the visible effect. **Severity:** Low to Medium.

### B-12.13 Titles with unknown duration cannot resume or complete normally
- **What goes wrong:** With duration 0: a new session always starts from zero (`S/playback/service.go:464`), the title never appears in Continue Watching (`home_rows.go:291` compares against NULL), and it completes only by `ended` or a credits marker (`personal_activity.go:57`).
- **Confidence:** Confirmed by reading; how often a playable asset lacks a duration is unknown to me. **Severity:** Low.

### B-12.14 A completion by a session that is not the "canonical" one does not mark the title watched
- **What goes wrong:** Only the session that most recently claimed the item writes watched state (`personal_activity.go:112`). Two devices on one profile playing the same film: the one started later owns the state. If it is stopped after two minutes and the first device plays to the end, History and the play count record the completion but the title stays unwatched with a two-minute resume point.
- **Confidence:** Confirmed by reading. **Severity:** Low (unusual), but completion is the one piece of evidence that is safe to accept from any session.

### B-12.15 The season-premiere exemption has no "recently arrived" test
- **What goes wrong:** A show outside the window is still listed whenever the offered episode is episode number 1 of a later season (`home_show_continuation.go:79-84`). A show abandoned at a season finale three years ago, with the next season long on disk, stays in Continue Watching for ever (at the tail, until 40 newer entries push it out). Conversely a premiere not numbered 1 is not recognised.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-12.16 "Today" for recommendations is the UTC date
- **What goes wrong:** Row choice, tie-breaks, exploration and the memo key use `now.UTC()` (`rec_engine.go:489,659`; `rec_generators.go:353-358`; `recommendation_engine.go:261,353`). Rows reshuffle at 00:00 UTC: 5 pm in Vancouver, 8 pm in New York, 10 am in Sydney. A viewer paging "Recommended" across that moment gets `stale_continuation`.
- **Confidence:** Confirmed by reading. **Severity:** Low. Deterministic across devices, which is the important property; only the boundary is odd.

### B-12.17 Search history is built on the server and not used; the web client ignores the "remember searches" preference and puts the query in the address
- **What goes wrong:** `GET/DELETE /v1/search/history`, `record=1` and `SearchHistoryLimit` exist (`S/catalog/search_history.go:12-83`; `S/httpapi/search.go:162-180`), but neither client calls them. Web keeps recents in a module variable regardless of `search.rememberHistory` (`W/screens/search/Search.tsx:33-40,110-120`; no reference to the preference in the file); native honours it (`N/screens/Search.tsx:64-68`). Web also writes `?q=` into the URL (`Search.tsx:70-74`), which decision R14 says not to do (`PC/decisions.md` R14).
- **Confidence:** Confirmed by reading. **Severity:** Low (a privacy preference that one client does not honour).

### B-12.18 Watched state, ratings and lists appear to be lost when a film's file is renamed, moved or replaced under a new name
- **What goes wrong:** See A-12.4. A film entity's identity is `file:<path relative to root>` (`S/compactcatalog/write.go:85-94`); personal rows cascade away when the old entity is purged (`S/catalog/purge.go:59`).
- **Confidence:** Probable — I did not trace the scanner for move detection or a retire-versus-purge grace period. **Severity:** High if it holds, because automated upgraders rename files routinely. Belongs to the libraries-and-scanning domain; recorded here because the loss is entirely personal state.

### B-12.19 A show's Next Up anchor is a bare id with no foreign key
- **What goes wrong:** `profile_show_activity.last_item_id` is text with no reference (`0001_baseline.sql:3079-3085`). If that episode entity is purged, the "next episode" branch joins to nothing (`home_show_continuation.go:60-64`) and the show drops out of Continue Watching and loses its page Next Up until the viewer plays something from it again.
- **Confidence:** Probable (depends on when episodes are purged). **Severity:** Low.

### B-12.20 Each progress report rewrites the profile's personal row and invalidates its Home (design risk)
- **What happens:** Every 10 seconds per playing client (30 paused): a `personal_history` upsert, a `personal_items` update with revision +1, one new `personal_snapshots` row holding the state as JSON, a `progress_activity` upsert, and through triggers a viewer-revision bump and a personal-sort job (`personal_activity.go:91-149`; triggers `0001_baseline.sql` "personal_revision_update", "catalog_personal_sort_source_update"). A two-hour film leaves about 720 snapshot rows (pruned after 30 days, 1,000 per sweep, `personal.go:558-568`). The viewer-revision bump changes the Home validator and every recommendation memo key for that profile, so a second device browsing Home recomposes rows every 10 seconds while the first plays.
- **Confidence:** Confirmed by reading; cost not measured. **Severity:** Low at household size; worth a look against the "couple of hundred concurrent browsers" target.
- **Fix direction:** Bump revisions and snapshot only when the visible state changes (started, completed, or position moved by more than some step).

### B-12.21 "Popular on this server" needs only two viewers
- **What happens:** A title appears when two opted-in profiles played it in seven days (`recommendation_engine.go:25-26,210`). In a two-person household, a title I watched that shows as popular tells me the other person watched it too.
- **Confidence:** Confirmed by reading. **Severity:** Low (both opted in).

### B-12.22 Server-wide play counts ignore "pause history" and the activity opt-out
- **What happens:** `play_counts` is incremented for every completed play before the pause check and without reading `privacy.showActivityToMembers` (`play_history.go:46-50`; `personal_activity.go:76-84`). When the owner shares activity, an artist's "Popular" songs use those counts (`S/catalog/listening_content.go:482`). A member who opted out still shapes what others see as popular.
- **Confidence:** Confirmed by reading. **Severity:** Low (aggregate only).

### B-12.23 Multi-episode files: "unfinished" is tested against the whole file's length
- **What happens:** The in-progress tests compare an episode's position with `max(duration)` of the linked asset (`home_show_continuation.go:47`; `home_rows.go:291`), while other code uses the episode's own range (`personal.go:514`, `continue_policy.go:80-86`). Harmless when completion is recorded normally; a latent inconsistency otherwise.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-12.24 One library's small "maximum items" shrinks everyone's merged row
- **What happens:** The merged Continue Watching row uses the minimum of the viewer's libraries' maximums (`continue_policy.go:92,111-113`).
- **Confidence:** Confirmed by reading. **Severity:** Low (surprising, not wrong).

### B-12.25 An explicit resume position on a never-played title does not appear in Continue Watching
- **What happens:** "Mark unwatched with a resume position" inserts into `progress` but only *updates* `progress_activity` (`personal.go:182-192`); with no activity row the Continue queries, which start from that table, never see it.
- **Confidence:** Confirmed by reading. **Severity:** Low (the request shape is allowed by the contract but probably unused by clients).

### B-12.26 "Mark unwatched" is not an undo for an accidental "Mark show watched": it erases the episodes' real state
- **What goes wrong:** Marking a container unwatched sets `cleared_through` to now, which immediately stops every older item intent from applying, and a background job then deletes those intents (`S/catalog/container_personal.go:155-174, 254-262`; `S/personalstate/watched.go:50`). So after an accidental "Mark show watched", the natural correction ("Mark unwatched") turns **every** episode unwatched, including the ones the viewer had really watched before the slip. The watermark design could support a true undo (remove the default that was just written, leaving intents alone), but there is no such operation. Resume points are not cleared by either container action.
- **How surfaced:** Jellyfin has the same loss (its cascade overwrites each episode); Portico's model is the one that could avoid it.
- **Confidence:** Confirmed by reading; I did not check whether clients offer an Undo toast for the container action. **Severity:** Low to Medium (one mis-tap on a long-running show).

## Where Portico is stronger

- **Marking a whole show is constant-time** and survives restarts and concurrent edits (A-12.1), where Jellyfin writes every episode.
- **Remove from Continue Watching with Undo**, at item and show level, which Jellyfin still lacks (A-12.26).
- **Explicit changes are safe to retry and safe offline** (A-12.15), and manual marks cannot be undone by a lagging player (A-12.14).
- **A real recommendation engine** covering every media family, with negative feedback, cold start, daily stability and bounded cost (A-12.45 to A-12.53).
- **Restrictions are applied inside the queries** for Home, Next Up, recommendations and search, including when lists are memoised (A-12.50). I found no leak path.
- **Home is one server-composed document** with exact totals and resumable paging; every client shows the same thing (A-12.36, A-12.44).
- **Freshness by revision**: no stale Continue Watching after a watch (A-12.43).
- **Two honest histories** and a viewer-controlled pause (A-12.16, A-12.17).
- **Credits-aware completion** and per-library rules (A-12.5, A-12.7).
- **Preferences follow the person** to a new device, with device-class overrides where they make sense (A-12.58).
- **Playlists with stable entry ids and compare-and-swap edits**, shared by role (A-12.34).
- **Search with per-group budgets and proper ranking** that stays bounded on very large libraries (A-12.55, A-12.56).
- **Privacy posture**: nothing about what anyone watches leaves the server; community rows are opt-in at two levels.

## Doc/code disagreements

1. **Favorites row on Home.** `PA/05 Recommendations.md` ("Default Home order … Favorites …") and `PA/03 Catalogue and Metadata.md` (Home: "…Recently added, Favorites, Recommended…") list a Favorites row; `PA/Backlog.md` mentions "Home's Watchlist/Favorites rows". The code removed both rows on 2–3 Oct and drops them from saved layouts (`S/catalog/home_rows.go:1065-1078`).
2. **Like / dislike.** `PC/saved-state-and-downloads/contract.md` §2 and §4.1 still define `reaction`; migration `0003_drop_reaction.sql` removed it.
3. **"Every personal-state mutation MUST carry an idempotency key and expected state revision"** (contract §4.4). Container My List / Favorite writes carry neither and are last-write-wins by design (`S/catalog/container_personal.go:16-18,108`); container watched carries a revision but no operation id.
4. **Offline evidence.** Contract §10.3 describes offline observations ordered by a device event sequence with a needs-resolution flow; the download progress sync is last-write-wins by the device's clock (`S/downloads/progress.go:61-65`).
5. **Search state.** `PC/decisions.md` R14: "In-memory scoped search only; no ordinary route URL…/persisted restoration". The web client writes the query to the URL and the server has a persisted search-history store.
6. **The hero and music.** `PA/03` says the hero shows "the artist or author for something listened to". Songs never keep a resume point, so only audiobooks can be the hero.
7. **`api/home.md`** is a ten-line note on dismissal and still speaks of "Continue Watching and Up Next" as two rows; `/v1/home` itself (rows, hero, layout, paging) is not described there.
8. **"New for you".** `PA/05` says there is no "New for you" row, then lists it among rows that have a See all.
9. **No time-based caches.** Home rows publish `cacheTtlSeconds` (45 / 120 / 300, `home_rows.go:353-360,377`), which no client reads; rows also carry an `explanation` field that is never set.
10. **Thresholds.** The contract (§4.2) speaks of "the active viewer's approved progress thresholds"; for video the library owner's value governs unless the viewer changed theirs (B-12.10). Neither doc states the 30-second floor.

## Open questions for Justin

1. Should "Mark season / show / book as watched" also finish members that are in progress or were individually marked unwatched (B-12.1)? I think yes for in-progress members; your call on hand-unmarked ones.
2. Should the show page's Next Up ignore the Continue Watching window and dismissals (B-12.2)? And should "Mark show watched" have a real Undo that restores the previous per-episode state (B-12.26)?
3. Should ticking episodes watched by hand put the show into Continue Watching (A-12.27), or is "only real playback starts a show" the intended rule?
4. Is "a rewatch makes the title unwatched until it completes" (A-12.11) still what you want now that play counts exist? And do you want a rewatch mode for Next Up (F-12.2)?
5. Who should own the Continue Watching window: the owner per library (today) or each profile (A-12.22)?
6. Should the played threshold be the library's only, the profile's only, or both as now (B-12.10)? And what should "started" mean (B-12.11)?
7. Should unapproved credits markers ever complete a title, and should `credits` mode fall back to the percentage when no marker exists (B-12.9)?
8. Must downloads played offline appear in the owner's play history and the viewer's History (B-12.5)?
9. Do you want watched state to survive a film being renamed, upgraded or re-added (A-12.4)? If so, by provider id, by file fingerprint, or by a grace period on retired titles?
10. Is co-viewing on one screen ("also watching") in scope for release (F-12.3)?
11. Is outside tracker sync (Trakt and friends) wanted, and if so on the server or in clients (F-12.1)? Is a one-time import from Plex / Jellyfin wanted for launch (F-12.4)?
12. Should the owner be able to place rows on everyone's Home and set a default layout (F-12.7)? Should a profile be able to have a different Home on a TV (F-12.12)?
13. With no hero when nothing is in progress (A-12.38), is an empty top of Home acceptable for new profiles?
14. Should recently added hide what the profile has watched (A-12.40)?
15. Search: are original and alternative titles, and typo tolerance, wanted for release (F-12.10)? Should recent searches be server-stored (synced, clearable) or device-only, and should the web URL carry the query (B-12.17)?
16. Should recommendation "days" follow the server's local time (B-12.16)?
17. Should "Popular on this server" require three viewers, or hide titles from the viewers who make up the minimum (B-12.21)?
18. Is a Seerr link or a wish list for titles not in the library something you would consider (F-12.5, F-12.6)?

## Evidence boundary

- **Static reading only.** No server was started, no test was run, no client was opened. Every "what goes wrong" above is a trace through source, not an observation.
- **Not established:** how often clients send `ended`, what the clients' settings screens show for thresholds, which season the show page opens in each case, what the native Home and Customize Home screens do, how `added_ms` behaves on file replacement, whether the scanner detects moved films, how often assets lack a duration.
- **Performance:** none measured. Statements about cost (per-report writes, Home recomposition) are inferences from the code and from the numbers in `PA/05`.
- **Recommendation quality:** not evaluated; only mechanisms were compared.
- **Plex, Emby, Infuse, Kodi:** from memory of public documentation; not re-fetched today. Plex's exact semantics for season premieres and credits modes should be checked before copying further.
- **Jellyfin:** read at master (Oct 2026), which includes a newer similar-items system and version handling than the 10.11 release line; behaviour of released versions may differ. Plugins (Trakt, Playback Reporting) were not in the clones.
- **Not read:** see "Not reached" under Scope.

## Outside my domain

- **Entity identity for films is the file path** (B-12.18). The libraries-and-scanning auditor should confirm what happens on rename, move, upgrade and library re-creation; the consequence lands on personal state.
- **A second file of the same film becomes a second title** (already in Backlog) also splits watched state and resume between the two.
- **Session start resets `progress.position` to the chosen resume point before any playback happens** (`S/playback/personal_progress.go:66-69`), including while "pause watch history" is on. If a client starts "from the beginning" under pause, the old resume point is overwritten although nothing else is recorded. Playback-protocol domain.
- **Trending needs the metadata consent switch and a provider with a feed**; TheTVDB libraries have none (Backlog). Metadata domain.
