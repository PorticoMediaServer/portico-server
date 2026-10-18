# 18 — Web app user experience and feature parity, screen by screen

Audit date: 11 October 2026. Read-only review from source. Nothing here was run, opened in a browser or reproduced; every statement about behaviour is a reading of the code (see "Evidence boundary").

Paths: Portico paths are relative to `/home/justin/projects/Portico`. To keep citations short, `web/…` means `apps/portico-server/web/…`, `core/…` means `apps/portico-server/packages/client-core/src/…`, `server/…` means `apps/portico-server/server/internal/…`, and `i18n/…` means `apps/portico-server/packages/i18n/src/…`. A bare `screens/…`, `app/…`, `shell/…` or `ui/…` is under `web/src/`. Comparison paths are written `jellyfin-web/…`, `plezy/…` and so on (jellyfin-web paths are under its `src/`).

Terms used below:

- **Owner tools**: anything only the server's owner sees (Server settings, metadata editor, delete, scan).
- **Card menu**: the ⋯ menu on a poster card (also opened by right-click, long-press or the `M` key).
- **Detail page**: the page for one title (a movie, a show, an album…).
- **Pivot / tab**: one view of a library (Movies, Collections, Albums, Songs…).
- **Jellyfin "legacy" and "modern"**: jellyfin-web now ships two layouts in one bundle. `apps/legacy` is the long-standing default (what the docs call "stable"); `apps/modern` is the newer React layout (formerly "experimental"). `apps/dashboard` is the admin console used by both.

## Scope and sources

**What was compared.** Every screen, control, menu action and setting in Portico's web app (viewer side and owner side, excluding the inside of the player) against Jellyfin's web client and dashboard, with lighter reference to jellyfin-vue, Plezy, Navidrome, Audiobookshelf, and Plex/Emby from public knowledge.

**Portico, read in full or nearly so**

- Docs: `Project Architecture/06 Clients.md`, `Backlog.md`, `Specs/Spec — Title Pages.md`, `Spec — Page Content by Media Type.md`, `Spec — Link Contract.md`; Product Contracts `navigation-and-input` (NAV-5 to NAV-8), `media-presentation` (§6, §14, §15), outlines of `visual`.
- Web: `web/src/app/router.tsx`, `shell/Shell.tsx`, `screens/home/*`, `screens/library/*`, `screens/detail/*`, `screens/people/*`, `screens/search/Search.tsx`, `screens/saved/*`, `screens/shared/*` (EntryActions, SelectionBar, Sections, MetadataEditor), `screens/settings/*` (Settings, SettingsRows, AccountPanels, parts of Profiles and Security), `screens/server/` (LibrarySettings, Libraries, People, Activity in full; the rest by header comments, strings and targeted greps), `app/` (open, detail, preferences, i18n, delete-media, downloads, metadata-editor, selection, server-pages), `ui/Card.tsx`, `ui/viewport.ts`.
- client-core: `presentation/settings-structure.ts`, `library-tabs.ts`, `browse-toolbar.ts`, `detail-actions.ts`, `entry-actions.ts`, `title-page.ts`, `search-model.ts`, `profile-actions.ts`; `preferences.ts`; parts of `show-workspace.ts`.
- Server (only to answer UI questions): `operations/preferences.go`, `operations/preference_consumers.go`, `catalog/browse_contract.go`, `catalog/browse_engine.go` (aggregate pivots), `catalog/search_history.go`, `httpapi/search.go`, `httpapi/detail.go`, `httpapi/owner_authority.go`, `metadata/details.go`, `metadata/repair_schema.go`, `identity/access_tier.go`, the route table `httpapi/route_lanes.go`.
- i18n catalogue (`i18n/catalog/en-US.ts` and `en-US/*.ts`, 4,570 keys) used as an index of UI strings; a small throw-away script counted keys no client references.

**Comparison projects read**

- jellyfin-web (commit `319dbfc`, 11 Oct 2026): `src/apps/legacy/routes/**`, `apps/legacy/controllers/**` (file inventory; `itemDetails/index.js` and `.html` function and element inventory), `apps/modern/routes/**`, `apps/modern/features/libraries/**` (views, sort, filters, view settings), `apps/dashboard/routes/**` and `features/**` (every page's label keys), `apps/wizard/**`, `components/itemContextMenu.js` (every command and its condition), `multiSelect`, `metadataEditor`, `refreshdialog`, `filterdialog`, `imageeditor`, `itemidentifier`, `subtitleeditor`, `libraryoptionseditor`, `displaySettings`, `homeScreenSettings`, `playbackSettings`, `subtitlesettings`, `scripts/keyboardNavigation.js`, `scripts/libraryMenu.js`, `themes/`.
- jellyfin-vue: page list only (`packages/frontend/src/pages`).
- Plezy: screen and widget inventory, `media_context_menu.dart` and appearance settings string keys.
- Navidrome (`ui/src`) and Audiobookshelf (`client/pages`): page inventories.
- Plex and Emby: from public documentation and general knowledge only. Nothing about them was verified in this session; treat those rows as prompts, not facts.

**Not covered.** The player's internals (auditor 17). Web engineering architecture (auditor 16). The Channels guide and DVR screens were only skimmed (their header comments and menus); the live-TV auditor owns them. The Portico Account pages (`screens/account/*`) and sign-in flows were skimmed for inventory, not traced. The React Native apps were opened only to compare one hook. No runtime check of any kind.

## How Portico does it today

### The frame

One authenticated frame (`web/src/shell/Shell.tsx`). On a wide screen it is a **left rail**: Home, Search, Saved, (Watch Together, only while you are in a group), then a **Libraries** heading listing every library by name, then a **Channels** heading (All channels, each source, Recordings) when the server has live sources, and at the foot a notifications bell, Settings, and a profile button that opens a menu (Switch profile, Switch server, Account, Downloads, Send feedback, Help, Sign out). The rail collapses to icons; at tablet width (≤1080 px) it starts collapsed. On a phone (≤720 px) the rail is replaced by a top bar (wordmark, Search, bell, avatar) and a bottom tab bar (Home, Library, Channels, Saved, Downloads) (`Shell.tsx:452-503`).

Every destination has a durable URL (`web/src/app/router.tsx`): `/`, `/home/rows/<row>`, `/library/<id>?view=&sort=&direction=&filters=&category=&row=`, `/library/<id>/<view>/<entity>`, `/media/<item>`, `/show/<show>?season=`, `/show/<show>/episode/<episode>`, `/person/<id>`, `/search?q=&type=`, `/saved?view=…`, `/saved/<kind>/<id>`, `/channels[/<entry>]`, `/downloads`, `/together`, `/settings[/<page>]`, `/account`. Old `/live` and `/server/*` addresses redirect. Cards are real links, so middle-click and "open in new tab" work.

### Home

`screens/home/Home.tsx`. The server composes the rows; the client draws them in the viewer's order. A hero (the newer of the two Continue rows' first entries) with Play and Details. A **Customize** dialog reorders and hides rows (drag or arrows), adds a saved view as a row, and resets to default (`screens/home/Customise.tsx`). Row "See all" goes to the row's library, to the row's own paged page, or to the saved view's page. "Remove from Continue Watching" and "Not interested" act at once with Undo.

### A library

`screens/library/Library.tsx`. The header carries the library's name, a library switcher, grid/list, card size (Small, Medium, Large), and for the owner a ⋯ with "Scan library" and "Manage library". Tabs are authored in client-core (`core/presentation/library-tabs.ts`):

| Library kind | Tabs |
|---|---|
| Movies | Discover · Movies · Collections |
| TV and Anime | Discover · Shows · Recently aired · Collections |
| Music | Discover · Artists · Albums · Songs · Genres · Playlists (+ Unmatched, owner only) |
| Audiobooks | Discover · Books · Authors · Series |

Discover is rows (recommendations and personal rows). The other tabs read one **browse engine**: a windowed grid (only the rows on screen exist), a toolbar built from what the server says it supports (`core/presentation/browse-toolbar.ts`): one Sort control, two or three quick toggles (Unwatched, In progress, Favorites), up to four promoted filter menus per kind (Genre, Year, Resolution, Rating for movies), "+ Filter" with every other field in three groups (about the title, your activity, about the file), the applied filters as removable chips, a count, and **Save view**. Ranges are presets ("Under 90 min", "8 or higher"), never two number boxes. A letter rail jumps by server-computed positions when sorted by title and unfiltered. The list view is a sortable table (title, year, runtime, rating, resolution, added; songs: title, artist, album, duration, plays, added). Everything is in the URL.

### Title pages

One shared hero (`ui/TitleHero.tsx`) for every type: backdrop, logo or title, a facts line, tagline, synopsis, credit lines, a **pick-before-play** row (Version, Audio, Subtitles, shown only where there is a real choice), the primary action, and one action row.

- **Movie** (`screens/detail/Detail.tsx`): Play/Resume · My List · Favorite · Watched · Trailer · ⋯ (Play from beginning, Rate…, Add to playlist…, Add to collection…, Download…, Edit metadata…, Refresh metadata). Then Cast (12, "See all" opens a paged dialog), Crew (key crew, "All crew"), recommendation rows, Extras, then Details (release date, studio, network, country, edition, genres as links, ratings, date added) ending with **Files** (per file: summary, size, bitrate, every audio and subtitle track, path for owners).
- **Show** (`screens/detail/Show.tsx`): the same hero and row; a season switcher (tabs up to 8, a dropdown beyond; Specials last), season progress, list or grid of episodes (windowed), a pinned Next up row, a season ⋯ (Play season, Mark season watched, Show settings for owners). Cast, crew, recommendation rows, Details. An **episode is a side panel** over the show (`EpisodePanel.tsx`), with its own URL.
- **Album, artist, book, author, series, collection** (`screens/library/Entity.tsx`): the hero with Play, Shuffle (music), My List (books), Favorite, Finished (books), Download (album, book), ⋯; numbered track rows with disc headings; an artist's Popular tracks, Songs and releases by type; Details.
- **Person** (`screens/people/Person.tsx`): portrait, life dates, biography, Known for, and a windowed grid of credits with All / Cast / Crew chips.

### Search, Saved, the rest

- **Search** (`screens/search/Search.tsx`): one field, type chips with counts (All, Movies, Shows, Episodes, People, Artists, Albums, Songs, Audiobooks, Live TV), a Top result when one title matches strongly, then sections in the chips' order. `/` focuses it from anywhere. A "Recent" list lives in memory.
- **Saved** (`screens/saved/Saved.tsx`, `Resource.tsx`): My List, Favorites, Playlists, Collections, Saved views, History. Sort and Unwatched/In progress chips on the first two; a period filter and Clear history on History. A playlist page: Play, Shuffle, rename, delete, reorder (Move up/down), remove. A collection page: rename, delete, "visible to everyone" switch, remove an item. A saved view page reruns its query.
- **Channels** (guide/list, recordings, reminders), **Downloads** (files the server prepared for this browser), **Watch Together**, the **notifications** bell (Unread / All / Archived).

### Multi-select and bulk actions

A checkbox on every card enters selection mode (`app/selection.tsx`, `screens/shared/SelectionBar.tsx`): Watched, Unwatched, My List add/remove, Favorite add/remove, Playlist, Collection (owner), Edit (owner), Refresh (owner), Delete (owner). "Select all N" means the whole filtered set, sent as one server job. Shift-click selects a range; Cmd/Ctrl+A selects what is visible; Esc leaves.

### Settings

One Settings screen with two headings, authored once in client-core for every client (`core/presentation/settings-structure.ts`): **Account** (Profile & account, Playback, Audio & subtitles, Quality, Music, Appearance, Language & region, Privacy, This device, About) and, for owners, **Server** (Dashboard, Play history, Libraries, Live TV & DVR, People, Playback, Remote access, Storage & backups, Schedule, Troubleshooting, General). A search field finds a row by its words. Account rows save on change; a server page has one Save. The full list is in the settings inventory below.

### Owner tools on content

- **Metadata editor** (`screens/shared/MetadataEditor.tsx`): six tabs. General (typed fields with a lock per field, the automatic value and its source beside each, "Use it" to restore), Artwork (choose among provider candidates per role; lock; Find more; Re-fetch), Tags (genres), People (credits), Matching (current match, search, candidates, Use this match, retry, identity lock), History (every change, Undo to here, Lock all / Unlock all). Bulk mode edits the bulk-safe fields and adds/removes genres across a selection.
- **Delete** (`app/delete-media-dialog.tsx`): the server previews what would go (files, size, shared files kept, libraries that block deletion); the owner chooses "also remove the files" and types the title (or the count) to confirm.
- **Per-show settings** (four switches), **Scan library**, **Fix permissions**, and the Server pages.

### First run and empty states

`screens/auth/ServerSetup.tsx` is one page: name the server, choose how people sign in, create the owner. An empty Home, library, search and rail each say what is missing and (for the owner) link to the fix (`screens/shared/NoLibraries.tsx`). A lost server replaces the content area with one "Reconnecting" state (`shell/Connection.tsx`).

## How the others do it

### Jellyfin web

**Frame.** A top app bar (back, menu, title, cast/"Play on", SyncPlay, search, user) and a slide-out drawer listing Home, each library, and admin links. No persistent library list on desktop in the legacy layout; the modern layout adds a permanent drawer. The home page has two tabs (Home, Favorites). Libraries appear as "My Media" tiles on Home.

**Home.** Up to ten home-section slots, each chosen by the user from a fixed list (My Media, My Media small, Active recordings, Continue watching, Continue listening, Continue reading, Latest media, Next up, Live TV, None), plus library order, "hide watched from latest", and which libraries are grouped/hidden (`jellyfin-web/src/components/homeScreenSettings/homeScreenSettings.template.html`, `homeScreenSettings.js:332,510-519`).

**Libraries.** Fixed tabs per collection type (`jellyfin-web/src/apps/modern/features/libraries/constants/views/*.ts`): Movies — Movies, Suggestions, Favorites, Collections, Genres, Studios, Playlists; Shows — Series, Suggestions, Upcoming, Genres, Networks, Episodes, Collections, Playlists; Music — Albums, Suggestions, Album artists, Artists, Playlists, Songs, Genres, Collections. Each grid has Play all, Shuffle, Queue, Sort (Name, Random, Community rating, Critic rating, Date added, Date played, Parental rating, Play count, Release date, Runtime — `components/SortButton.tsx:27-63`), a Filter panel (status: played/unplayed/favorite/resumable; series status; features: subtitles, trailer, special features, theme song, theme video; genres; parental ratings; tags; video types: Blu-ray, DVD, HD, SD, 3D; years; studios; audio and subtitle languages — `components/filter/*`), View settings (grid or list; image type; show title; show year; card layout — `ViewSettingsButton.tsx:113-258`), an alphabet picker, and numbered pagination with a user-set page size.

**Detail page.** One page for every item type (`apps/legacy/controllers/itemDetails/index.html`, `index.js`): Play/Resume, Replay, Trailer, Instant mix, Shuffle, Download, Played toggle, Favorite, Split versions, More. Version/Video/Audio/Subtitle selects. Sections: Next up, children (seasons, episodes, tracks), lyrics, More from this season, More from this artist, collections it is in, More like this, series air time and schedule, tags, music videos, additional parts, **Scenes** (chapters), Specials, Cast & crew, Guest cast, external links, person birth/death details.

**Item menu.** One function builds every item menu (`components/itemContextMenu.js:44-380`); see inventory (c).

**Owner tools.** Edit metadata (a single long form with external IDs, per-field lock checkboxes and a whole-item lock), Edit images (browse per image type and provider, upload, delete, reorder backdrops), Edit subtitles (search providers, download, upload, delete), Edit lyrics, Identify (by name, year or any external ID), Media info, Refresh metadata (three modes plus replace images and replace trickplay), Delete, Group/Split versions.

**User settings.** Profile (picture, password), Quick Connect, Display, Home, Playback, Subtitles, Controls (`apps/legacy/routes/user/settings/index.tsx`).

**Dashboard** (`apps/dashboard/routes/_asyncRoutes.ts:5-32`): Dashboard, General settings, Users (profile, access, parental control, password), Libraries (+ Display, Metadata, NFO), Playback (Transcoding, Resume, Streaming, Trickplay), Devices, Activity, Networking, API keys, Backups, Logs, Plugins (catalogue, installed, repositories), Scheduled tasks, Live TV (+ recordings), Branding.

**First run.** A six-step wizard (`apps/wizard/controllers`: start/language, user, library, metadata settings, remote access, finish).

### jellyfin-vue

A smaller, modern single-page app: home, library, item, series, music album, artist, genre, person, search, a metadata page, and settings for account, subtitles, server, devices, API keys, users, logs and activity (`jellyfin-vue/packages/frontend/src/pages`). Useful mainly as a sign of where Jellyfin's own designers simplified: one item page per type family, far fewer settings.

### Plezy

A client only (no admin console), but with client-side choices Portico does not offer (`plezy/lib/screens/settings/*`, appearance string keys): theme, display scale, library density and grid spacing, grid or list, show or hide the hero, **hide spoilers**, unwatched-count badges, episode poster mode (series poster, season poster, episode thumbnail), tap action for Continue Watching and for episodes (play or details), a rebinding keyboard-shortcuts screen, an alpha jump bar with a scroll handle, a folder-tree browse, a filter builder with operators, "force TV mode", and "require profile selection on open". Its item menu (`plezy/lib/widgets/media_context_menu.dart`): mark watched/unwatched, remove from Continue Watching, rate, play version, shuffle play, play from beginning, go to series, file info, view details, delete (with a warning naming how many files and whether a file is shared).

### Navidrome and Audiobookshelf

- Navidrome (`navidrome/ui/src`): album grid or table with presets (recently added, most played, top rated, random, starred), artists, songs table with choosable columns, playlists (including smart playlists), internet radios, **public share links**, a "missing files" admin view, per-player transcoding settings, about 30 **themes**, a Personal page (theme, language, default view, desktop notifications, ReplayGain mode, Last.fm and ListenBrainz scrobbling), and a hotkeys help dialog.
- Audiobookshelf (`audiobookshelf/client/pages`): library shelf, series, collections, playlists, authors, narrators, per-library and per-user **statistics**, a **batch edit** page, per-book edit / manage / chapter editor pages, an upload page, public shares, and config pages for libraries, users, sessions, backups, logs, notifications, email, authentication (OpenID), RSS feeds, API keys, genre and tag management and custom metadata providers.

### Plex and Emby (public knowledge, not verified here)

Plex Web is the stated standard for Portico's title pages. Things it is known for in this domain: library view modes (grid, details, list), advanced filters with AND/OR groups, smart collections and smart playlists built from filters, multi-select bulk edit, "Fix match" / "Match", "Get Info" (with the raw XML), "Analyze", "Optimize", "Refresh metadata", a poster/background picker with upload and URL, per-library advanced settings, a Dashboard with Now Playing and Alerts, and a separate "Plex Dash" admin app. Emby's web app is Jellyfin's ancestor and shares most of the Jellyfin inventory above, plus more per-user home-screen control.

## Inventory (a): screens side by side

"—" means the product has no such screen. "Partial" means some of it exists; the note says what.

### Viewer side

| Screen or area | Portico web | Jellyfin web | Notes |
|---|---|---|---|
| Primary navigation | Left rail: Home, Search, Saved, every library, Channels entries; bell, Settings, profile menu. Phone: top bar + 5 bottom tabs | Top app bar + drawer (Home, libraries, admin). Home has Home / Favorites tabs | Portico's libraries are always one click away; Jellyfin's are a drawer or Home tile away |
| Library order and visibility | Owner sets one order for everyone (`Libraries.tsx` Move up/down). No per-viewer order, hide or pin | Per user: library order, which libraries appear on Home and in Latest | Portico's client-core has `library-pins.ts`, but no web control uses it (see B-18.27) |
| Global search | `/search`, type chips with counts, Top result, in-memory recents, `/` shortcut | Search page: field, per-type sections, suggestions when empty | Portico has no in-library search field |
| Home | Hero + server-composed rows; Customize (reorder, hide, add saved views) | Ten configurable section slots; no hero in legacy | Different authorities: server composes vs user picks from a fixed list |
| Home row "See all" | Library tab, row page, or saved-view page | Section header links to the library tab | — |
| Favorites | Saved › Favorites (one mixed grid, Unwatched/In progress chips) | Home › Favorites tab, sectioned by type; a Favorites tab in each movie library | Portico has no per-type split |
| Library: suggestions | Discover tab (recommendation and personal rows) | Suggestions tab (continue, latest, recommendations, next up) | — |
| Library: main grid | Windowed grid or table; filters; sort; letter rail; save view; card size | Paged grid or list; filter panel; sort; alphabet picker; view settings | See A-18.6 to A-18.9 |
| Library: collections | Collections tab | Collections tab with **New collection** button | Portico creates collections from Saved or a card menu only |
| Library: genres | Music only (Genres tab). Video genres are a filter | Genres tab for movies, shows and music (rows of items per genre) | Deliberate in Portico (`library-tabs.ts:4-6`) |
| Library: studios / networks | Filter only | Studios / Networks tab | — |
| Library: episodes | — | Episodes tab (flat, sortable, filterable) | Portico has "Recently aired" (shows ordered by latest episode) |
| Library: upcoming | — | Upcoming tab (episodes airing soon) | Needs provider air dates for episodes not on disk |
| Library: playlists | Music only | Playlists tab in movies, shows, music | — |
| Library: favorites | — (quick filter chip "Favorites") | Favorites tab | Equivalent result, different place |
| Play all / Shuffle a library | — (comment at `Library.tsx:154-157`: the server refuses a query selector) | Play all, Shuffle, Queue on every grid | F-18.4 |
| Movie page | Yes | Yes | See A-18.11 to A-18.14 |
| Show page | Yes, seasons as URL state | Series page; each season is its own page | — |
| Season page | — (season is a tab; "Season info" not built) | Yes (poster, overview, episodes, cast, More from…) | Doc/code item D-18.3 |
| Episode page | Side panel over the show | Full detail page | Deliberate (Spec — Title Pages §3) |
| Album / artist | Yes | Yes (+ Instant mix, music videos, lyrics section) | — |
| Audiobook, author, series | Yes | Books library is e-books; audiobooks are music-like | Portico is ahead for audiobooks |
| Person page | Yes | Yes (items sectioned by type; birth place; external links; favorite) | — |
| Genre / studio page | Opens the library filtered | Item-by-name page | — |
| Collection page | Entity page (library collections); Saved resource page (personal collections) | BoxSet detail page, sectioned by type | Two different "collection" things in Portico (A-18.17) |
| Playlist page | Yes (play, shuffle, rename, delete, move up/down, remove) | Yes (drag reorder, move to top/bottom, edit: public, collaborators) | — |
| My List (watchlist) | Yes | — (plugin territory) | Portico ahead |
| Saved views | Yes | — | Portico ahead |
| History | Saved › History with period filter and Clear | — for viewers (admin Activity only) | Portico ahead |
| Star rating | 0.5–5 stars | — (favorite only) | Portico ahead |
| Not interested / reset recommendations | Yes | — | Portico ahead |
| Queue / now playing page | In the player | Queue page, Now playing bar, remote control page | Player domain |
| Live TV | Channels (guide, list, recordings, reminders) | Live TV tabs: Programs, Guide, Channels, Recordings, Schedule, Series | Other auditor |
| Photos, home videos, books (e-books, comics, PDF), music videos, mixed libraries | — | Yes, each with its own views and readers | F-18.47 |
| Downloads | List of server-prepared files to save | Direct file download from the item menu; no list | Different models |
| Watch Together / SyncPlay | Page + rail entry while in a group | SyncPlay menu in the app bar at all times | — |
| Notifications | Bell with inbox (Unread, All, Archived) | Toasts only | Portico ahead |
| Profile switch | Profile menu (rail only) | Sign out to the user picker | B-18.14 for phones |
| Server switch | Profile menu › Switch server | Select Server page | — |
| Sign-in | Portico Account, SSO, direct; profile picker with PIN; TV code approval | User tiles, manual login, Quick Connect, forgot-password PIN | Identity auditor |
| First run | One setup page | Six-step wizard | A-18.33 |
| Not found / errors | In-frame Not found; route crash boundary; coded messages | Generic alerts; "page not found" fallback | — |
| Keyboard help | — | — (Navidrome and Plezy have one) | F-18.53 |

### Owner / admin side

| Screen or area | Portico web | Jellyfin dashboard | Notes |
|---|---|---|---|
| Where it lives | Settings › Server (11 pages), same screen as personal settings | Separate Dashboard app with its own drawer (about 25 pages) | A-18.2 |
| Dashboard | Watching now (Stop + message), Alerts, Health tiles with history, Background tasks, Viewing statistics | Server info, active sessions (message, pause/stop, session info), activity, alerts, running tasks, paths, **Restart / Shut down** | Portico has no restart or shutdown control |
| Play history | Server › Play history (filters) | Activity page (all events) | — |
| Libraries list | Add, order, open, scan state, "files need attention" review | Add, rename, remove, scan all, edit images, manage folders | **Portico cannot remove a library** (B-18.5) |
| Library options | Tabs: General (folders, scan depth, scan now, full scan, fetch lyrics), Metadata (source online/local, provider, language, country), Scanning (analysis steps by cost, preview-picture interval and width, chapter pictures), Advanced (allow deletion, days kept) | One long dialog, about 40 options (see inventory b) | A-18.25 |
| Display / metadata / NFO defaults | — | Three pages | — |
| Users | Accounts list; account page (role, libraries, sign-in, limits, devices); add account; invitations; API keys | Users list; add; edit (Profile, Access, Parental control, Password); delete | Portico has no delete-account control (F-18.32) |
| Profiles inside an account | Yes (name, picture, colour, PIN, limits, order) | — (one user = one profile) | Portico ahead |
| Devices | Per account, with approve / block | Global list, rename, delete, delete all | — |
| Playback / transcoding | One page: status, 9 conversion settings, 6 capacity caps, 3 streaming limits | Four pages: Transcoding (about 50 settings), Resume, Streaming, Trickplay | A-18.31 |
| Networking | "Remote access": status, Portico Account link, reachability, certificate, who may sign in, home networks, advanced mapping | One form: ports, base URL, bind addresses, LAN networks, proxies, HTTPS certificate, remote IP allow/deny list | A-18.30 |
| Storage | Disk usage with clean-up, retention, remote sources (WebDAV, rclone), backups, deleted titles (trash) | Backups page; paths on the dashboard | Portico ahead on trash and remote sources |
| Scheduled work | Scheduled jobs list, maintenance windows, background priority | Tasks list, run now, per-task triggers (daily, weekly, interval, startup, time limit) | A-18.27 |
| Logs | Live message log with levels, "record more detail for 30 minutes", records, support export (redacted) | Log file list, viewer with watch, download | — |
| Capabilities | Server capabilities panel (what this machine can do and why not) | — | Portico ahead |
| Plugins | — | Catalogue, installed, repositories, per-plugin pages | A-18.29 |
| Branding | — | Login disclaimer, custom CSS, splash screen | A-18.32 |
| Live TV admin | Sources, tuners, recording rules and defaults, storage, Library Channels builder | Tuners, guide providers, recording settings, channel mapping | Other auditor |
| API keys | Yes, with scopes | Yes, no scopes | Portico ahead |
| Feedback inbox | Yes (viewers report a problem; owner reads it) | — | Portico ahead |
| Metadata manager | — (edit from each title; bulk from a selection) | Folder-tree "Metadata manager" page | — |
| Updates | General › Version and updates | — (package manager) | — |

## Inventory (b): settings

"P only" = only Portico exposes it; "J only" = only Jellyfin does. Jellyfin label keys are quoted from its templates so they can be searched.

### Viewer preferences

| Group | Portico (`settings-structure.ts:155-285`) | Jellyfin (`displaySettings`, `homeScreenSettings`, `playbackSettings`, `subtitlesettings` templates) | Only on one side |
|---|---|---|---|
| Next episode | Play next automatically; countdown (off, 5, 10, 15 s); "Still watching?" and after how many episodes (2–5) | Play next automatically; next-video info overlay; Still watching prompt | Countdown length P only |
| Skipping | Intro, credits, recap: ask / auto / off; skip back 5–30 s; skip forward 10–60 s | Media segment actions (per type: none / ask / skip); skip back / forward length | Roughly equal |
| Speed | Default video speed; default audiobook speed | — (speed is a player menu only) | P only |
| Watched threshold | 80–100 % (Advanced) | Admin-wide resume settings (min %, max %, min duration, audiobook minutes) | P per viewer; J per server |
| Audio language | Ordered list of preferred languages | One preferred language; "Play default audio track regardless of language" | Ordered list P only; "play default track" J only |
| Subtitles | Mode: off / forced only / always; ordered list of languages | Mode: default / smart / only forced / always / none; one language | "Smart" (subtitles when audio is foreign) J only |
| Subtitle look | Size (4), background (3), preview | Styling mode (auto / custom / native), size, weight, font (8), text colour (11), background colour, drop shadow (5), vertical position, burn-in policy, PGS rendering, bitmap aspect mode, always burn in when transcoding | Most appearance options J only |
| Quality | Limit at home, away, (cellular on phones): automatic / original / 4K / 1080 / 720 / 480 | Home network quality, Internet quality, max video resolution, Chromecast quality, music quality | — |
| Delivery method | Automatic / Original only / Always convert; HDR automatic / off | Preferred transcode codecs (video, audio), allowed audio channels, DTS and TrueHD toggles, Hi10P, fMP4 container, limit segment length, always remux FLAC / MP3, disable VBR, external players | One simple choice P; many expert toggles J |
| Remember choices | — (server decides tracks per title) | Remember audio selections; remember subtitle selections | J only |
| Cinema mode | — | Enable cinema mode (prerolls) | J only |
| Music | Volume leveling off / track / album; crossfade 0–12 s; gapless; show lyrics | Audio normalization (off / track / album) | Crossfade, gapless switch, lyrics switch P only |
| Appearance | Card size (S/M/L); show backdrops; reduce motion; default library view grid / list; unread badge | Theme (7), dashboard theme, display mode (auto / desktop / mobile / TV), custom CSS, disable server CSS, backdrops, theme songs, theme videos, details banner, blurhash, faster animations, library page size, screensaver and its timings, slideshow interval | Card size, reduce motion P only; themes, CSS, display mode, theme media, screensaver, page size J only |
| Home | Customize dialog: order, hide, saved-view rows | Ten section slots; library order; folder groups; hide watched from Latest; TV home screen orientation | See A-18.5 |
| Next up | — | Max days in Next up; rewatching in Next up; episode images in Next up; display missing episodes within seasons | J only |
| Language & region | Language (English variants at launch); 12 / 24-hour clock | Display language (90+); date/time locale | Clock choice P only |
| Privacy | Pause watch history; show my activity to members; remember search history; reset recommendations | — | P only |
| This device | Device name; Link a TV; this server (disconnect) | Controls (gamepad, smooth scroll); Quick Connect | — |
| Per-show (owner, applies to everyone) | Hide seasons; number across seasons; ranges of 100; newest first | Display order in the metadata editor (aired, DVD, absolute, …) | DVD order J only; ranges and newest-first P only |

**Portico registry keys with no settings row** (`server/operations/preferences.go:161-215` against `settings-structure.ts`): `music.defaultSpeed` (deliberate: "a song plays at the speed it was recorded", `settings-structure.ts:184`), `playback.sleepTimerMinutes` (set from the player), `playback.startedThresholdPercent`, `privacy.includeInWatchTogether`, `home.rowOrder` / `home.hiddenRowIds` (Customize dialog), the per-network `quality.*` and `delivery.*` keys (written by the composite rows). `privacy.includeInWatchTogether` and `playback.startedThresholdPercent` are enforced or read by the server but no person can change them from any client.

### Profile and account limits (owner sets)

| | Portico | Jellyfin |
|---|---|---|
| Per profile (`screens/settings/Security.tsx:38-110`) | Rating system; highest rating; allow unrated; hidden labels; allow downloads, Live TV, DVR, Watch Together; PIN | (No profiles) |
| Per account (`screens/server/People.tsx`, `Access.tsx`) | Role (Member, Administrator); libraries visible; disabled; streams at once; signed-in devices; quality away from home; highest rating; hidden labels | Administrator; hide from login; disabled; libraries; channels; **access schedule** (days and hours); max parental rating; block items with no rating by type; **allow / block tags**; allow media deletion per library; collection management; subtitle management; Live TV browse / manage; remote control of others and shared devices; content download; **allow playback, audio transcoding, video transcoding, remuxing, force remote transcoding**; allow remote access; remote bitrate limit; SyncPlay access level; login attempts before lockout; max active sessions; authentication provider; password reset provider |

### Server settings

| Area | Portico (`settings-structure.ts:302-460`) | Jellyfin (dashboard route label keys) |
|---|---|---|
| General | Server name; updates; alert thresholds (storage warning and critical %, certificate days) | Server name; preferred display language; cache path; metadata path; Quick Connect; parallel image encoding limit; library scan fan-out concurrency |
| Libraries (per library) | Name; folders (local or network share, scan depth per folder); metadata source and language/country; analysis steps (inspect, local metadata, subtitles, checksums, chapter images, preview thumbnails, waveforms, loudness, fingerprints, intro/credit detection); preview-picture interval and width; chapter pictures; allow deletion; days kept in trash | Enable library; real-time monitoring; metadata language and country; **prefer embedded titles / extras titles / episode info over file names**; **automatically group series spread over folders**; **automatically add to collection**; **refresh internet metadata every N days**; metadata downloaders and **image fetchers per item type with order and limits**; **save artwork and NFO into media folders**; subtitle downloaders, languages, skip rules, perfect-match, save beside media; lyrics fetchers and save location; embedded subtitles policy (all / text / image / none); chapter image extraction (and during scan); trickplay extraction (and during scan; save beside media); LUFS scan; non-standard artists tag; custom tag delimiters and whitelist; special-season display name; photos |
| Library defaults | — | Display: date-added behaviour (file time or import time), folder view, specials within seasons, group movies / shows into collections, external content in suggestions. Metadata: preferred language and country, dummy chapter length, chapter image resolution. NFO: Kodi user for watched data, save image paths, path substitution, extra thumbs |
| Playback | Allow converting; hardware (auto, software, VideoToolbox, VAAPI, QSV, NVENC, AMF) and device; encoder speed; tone mapping and method; repackaging; planning policy (fidelity, compatibility, least server work); temp folder; caps (server, per account, conversions total / hardware / software / background); remote quality limit; upload speed; paused-session timeout | Hardware acceleration type and device; hardware decoding per codec; enhanced NVDEC; system-native decoder; hardware encoding; Intel low-power encoders; HEVC and AV1 encoding; tone mapping (algorithm, mode, range, desat, peak, param) and VPP / VideoToolbox variants; thread count; ffmpeg path; transcode path; fallback fonts; audio VBR; down-mix boost and algorithm; max muxing queue; encoder preset; H.264 and H.265 CRF; deinterlace method and double rate; on-the-fly subtitle extraction; throttling and delay; segment deletion and keep seconds. Resume: min %, max %, min duration, audiobook minutes. Streaming: remote bitrate limit. Trickplay: hardware, scan behaviour, priority, interval, widths, tile size, quality, threads |
| Remote access / networking | Remote access on/off; IPv6; certificate request and port; who may sign in from outside (anyone, owner only, nobody); encryption policy; LAN discovery; home networks; treat WAN as LAN; extra addresses; (advanced) port mapping by PCP / NAT-PMP / UPnP, public port, gateway, trusted proxies, advertised interface, custom certificate domain and paths | HTTP and HTTPS ports; enable HTTPS; require HTTPS; certificate path and password; **base URL**; bind addresses; IPv4 / IPv6; LAN networks; known proxies; published server URIs; auto-discovery; port ranges; allow remote access; **remote IP allow or deny list** |
| Storage and retention | Retention days for diagnostics, notifications, play history, task history; backups to keep; per-category generated-file retention; remote sources; backups; trash | Backups (create, restore, what to include) |
| Schedule | Scheduled jobs; maintenance windows (days, start, hours, which work waits); background priority (lower / normal) | Per-task triggers and time limits; run now |
| Live TV | Stream buffer; retry window; guide days; user agent; logo import; discovery; recording padding, keep policy, conversion after recording, folder templates | Guide days; recording paths (general, movie, series); padding; post-processor and arguments; save recording NFO and images |
| Branding | — | Login disclaimer; custom CSS; splash screen image |
| Plugins | — | Repositories; install, configure, disable, remove |

## Inventory (c): actions on an item

### Portico

The card menu comes from one model (`core/presentation/entry-actions.ts`) in six groups; the page's own ⋯ comes from that model too for shows and music (`origin: 'page'`), but from a second model for movies (`core/presentation/detail-actions.ts`). "Owner*" means owner signed in with an account that lives on this server (see B-18.8).

| Item | On the page's action row | In the page's ⋯ | In the card / row ⋯ |
|---|---|---|---|
| Movie | Play or Resume · My List · Favorite · Watched · Trailer | Play from beginning · Rate… · Add to playlist… · Add to collection… · Download… · Edit metadata… (Owner*) · Refresh metadata (Owner*) | Play / Resume · Play from beginning · Play next · Add to queue · Watch Together (start a group) or Watch with group / Add to group queue · My List · Favorite · Mark watched · Remove from Continue Watching · Not interested (recommendation rows) · Rate… · Add to playlist… · Add to collection… · Download… · Details · Edit metadata… · Refresh metadata · **Delete…** (owner) |
| Show | Resume S2 E4 · My List · Favorite · Watched | Download all… · Edit metadata… · Refresh metadata | My List · Favorite · Mark show watched · Download all… · Details · Edit metadata… · Refresh metadata |
| Season | (Season ⋯ on the show page) Play season · Mark season watched · Show settings (owner) | — | Mark watched · Download all… · Details · Edit metadata… · Refresh metadata |
| Episode | (Panel) Play / Resume · Play from beginning · Mark watched | Same as the row ⋯ | As a movie's card menu |
| Album | Play · Shuffle · Favorite · Download | Edit metadata… · Refresh metadata | Favorite · Download all… · Details · Edit metadata… · Refresh metadata |
| Track | — (plays on click) | — | Play · Play next · Add to queue · group actions · personal actions the server offers · Rate… · Add to playlist… · Download… · Details · Edit metadata… · Refresh metadata · Delete… |
| Artist | Play · Shuffle · Favorite | Edit metadata… · Refresh metadata | Favorite · Details · Edit metadata… · Refresh metadata |
| Audiobook | Resume or Play · My List · Favorite · Finished · Download | Edit metadata… · Refresh metadata | My List · Favorite · Mark finished · Download all… · Details · Edit metadata… · Refresh metadata |
| Personal collection | Visible-to-everyone switch | Rename · Delete | (Item inside it) Go to item · Remove from collection |
| Playlist | Play · Shuffle | Rename · Delete | (Entry) Go to item · Move up · Move down · Remove from playlist |
| Saved view | — | Rename · Delete | Normal card menus |
| Person | — | — | — |
| Selection (bulk) | Watched · Unwatched · My List add/remove · Favorite add/remove · Playlist · Collection (owner) · Edit (Owner*) · Refresh (Owner*) · Delete (owner) | | |

### Jellyfin

One list, filtered by item type and the user's policy (`jellyfin-web/src/components/itemContextMenu.js:52-380`):

Play · Play all from here · Stop playback · Clear queue · Add to queue · Play next · Shuffle · Instant mix · Select (enter multi-select) · Add to collection · Add to playlist · Cancel recording · Cancel series recording · Download all · Download · **Copy stream URL** · Delete (worded per type) · Edit playlist · Edit metadata · **Edit images** · **Edit subtitles** · **Edit lyrics** · **Identify** · **Media info** · Record · Refresh metadata · Remove from playlist · Move to top · Move to bottom · Remove from collection · Share · View album · View artist · View lyrics.

On the detail page in addition: Replay, Trailer, Played, Favorite, **Split versions**. In multi-select (`components/multiSelect/multiSelect.js:194-256`): Select all, Add to collection, Add to playlist, Delete, **Group versions**, Mark played, Mark unplayed, Refresh metadata.

### What each side has that the other lacks

- **Only Portico:** My List, star rating, Not interested, Remove from Continue Watching, start Watch Together from a card, Download with a quality choice and "all / unwatched / next N" policy, undo of metadata edits, bulk actions over a whole filtered set as one job.
- **Only Jellyfin:** Play all from here, Shuffle on any folder, Instant mix, Copy stream URL, Edit images (upload, URL, delete), Edit subtitles, Edit lyrics, Identify by ID, Media info dialog, three refresh modes, Group / Split versions, Share, View album / View artist jumps, Move to top / bottom.

## Architectural differences

Sizes are rough: **small** = a day or two in one layer; **medium** = a week or so, or two layers; **large** = server, client-core and every client.

### A-18.1 Who decides what a screen contains

- **Portico:** The server composes rows, order, labels, sorts and filter vocabulary; client-core authors the layout (tabs, settings pages, action menus, toolbar) once for every client; the web only draws (`06 Clients.md` "Rules both clients follow"; `core/presentation/*`).
- **Jellyfin:** Each page is its own controller that builds queries against a general Items API and decides its own sections, sorts and filters (`jellyfin-web/src/apps/legacy/controllers/**`, `apps/modern/features/libraries/constants/views/*.ts`). Every client re-implements this.
- **Why it matters:** Portico's clients stay consistent and a new client is cheap; a change of layout is one edit. The cost is that nothing can be added on the web alone: a new tab, filter or menu item usually needs the server or client-core first, and viewers cannot rearrange much themselves.
- **Assessment:** **Portico ahead** for consistency and scale; deliberate (`06 Clients.md`). The trade-off shows up as the long missing-features list below: Jellyfin's breadth comes from each page being free to query whatever it likes.

### A-18.2 One Settings screen against a separate admin console

- **Portico:** Personal and server settings are one destination with two headings and a search field (`settings-structure.ts:6-24`, `screens/settings/Settings.tsx`). Eleven server pages; status at the top of a page, settings below, one Save per page.
- **Jellyfin:** User settings are six pages under the user menu; administration is a separate Dashboard app with its own drawer and about 25 pages (`apps/dashboard/routes/_asyncRoutes.ts`).
- **Why it matters:** Portico's owner finds everything in one place and can search it. Jellyfin's split keeps a long admin surface away from ordinary users and gives it room to grow (plugins add pages).
- **Assessment:** **Portico ahead** on findability (search over settings is something Jellyfin lacks). Deliberate (Justin, 2 Oct 2026). Watch the page count: the Server heading is already eleven pages, not the ten the docs say (D-18.1).

### A-18.3 Navigation: a permanent rail of libraries against an app bar and drawer

- **Portico:** Every library is a rail entry, in one order the owner sets for everyone (`Shell.tsx:292-330`; `Libraries.tsx` "Move up / Move down" calls `PUT /v1/admin/libraries/order`). No per-viewer order, hiding or pinning on the web.
- **Jellyfin:** Libraries are drawer entries and Home tiles; each user orders them and chooses which appear on Home (`homeScreenSettings.template.html`: `HeaderLibraryOrder`, `LabelSelectFolderGroups`).
- **Plezy:** A side rail on desktop/TV, bottom bar on phones; optional "always keep sidebar open", "group libraries by server".
- **Why it matters:** With many libraries (a family server can have ten or more) one global order suits nobody in particular; a child's profile sees the same long list as the owner (minus libraries it cannot access).
- **Assessment:** **Trade-off.** Portico's rail is the better default. Per-profile order or "hide from my rail" is the missing half; client-core already has `library-pins.ts` and the consumer-check script mentions a `navigation.pinnedLibraryIds` key that no longer exists in the registry, so this was started and dropped. Small to medium to finish.

### A-18.4 Search: a destination with chips against a field with sections

- **Portico:** A page with type chips carrying per-type counts, a Top result, sections in a fixed order, a per-group "See all" that pages with Previous / Next, and the query in the address (`Search.tsx`, `core/presentation/search-model.ts`). The server enforces query rules (too short, too many words) and the page says so.
- **Jellyfin:** A search page with per-type sections and suggestions when empty; no counts, no top result.
- **Why it matters:** Portico's is easier to scan and link. It has no in-library search, and its recents are memory-only (B-18.3).
- **Assessment:** **Portico ahead.** One open point: the address carries the words, which the navigation contract forbids (D-18.5).

### A-18.5 Home: server-composed rows with an order dialog against user-chosen slots

- **Portico:** The server decides which rows exist; the viewer reorders and hides them and can add saved views as rows; the layout is stored on the server per profile (`Customise.tsx`, registry keys `home.rowOrder`, `home.hiddenRowIds`). A hero sits on top.
- **Jellyfin:** Ten slots, each a choice from a fixed list, stored per user **per client app**; plus "hide watched from Latest" and library grouping.
- **Why it matters:** Portico's layout follows the profile to every device; Jellyfin's must be set up again on each client. Portico cannot turn the hero off, cannot hide watched items from a "recently added" row, and has no per-library "Latest" opt-out.
- **Assessment:** **Portico ahead** on the model. Consider two viewer switches Jellyfin and Plezy both have: "show hero" and "hide watched from recently added" (small each, server-side row rule plus one preference).

### A-18.6 A windowed endless grid against numbered pages

- **Portico:** Only the rows in view are mounted; the server gives a total and a position index, so the letter rail is one request and Back restores the exact window from memory before any request (`Library.tsx:302-439`, `core/collections/windowed.ts`).
- **Jellyfin:** Pages of a user-set size (default 100; 0 turns paging off and loads everything), with the alphabet picker applied as a **filter** on the first letter.
- **Why it matters:** Portico scales to millions without a page concept and never loses the place. Jellyfin's letter filter works with any sort and any filter; Portico's rail appears only when sorted by title **and** unfiltered (`Library.tsx:416`).
- **Assessment:** **Portico ahead** on the grid. The rail's narrow availability is the one weak spot (F-18.10).

### A-18.7 Filters built from the server's vocabulary against a fixed panel

- **Portico:** The server publishes fields, operators, sorts and quick filters; client-core decides how each is chosen (list of values with counts, yes/no, a few choices, range presets) and which sit on the bar per library kind (`browse-toolbar.ts:23-140`). Fields include actor, director, writer, studio, network, collection, tag, label, resolution, dynamic range, audio language, duration, date added, last played, my rating. Filters live in the URL and can be saved as a named view.
- **Jellyfin:** A fixed dialog: played state, favorites, series status, features (subtitles, trailer, extras, theme media), genres, parental ratings, tags, video types, years, studios, audio and subtitle languages.
- **Plex (public knowledge):** Advanced filters with nested AND/OR groups and any-field conditions; a filter can become a smart collection.
- **Why it matters:** Portico's is broader than Jellyfin's and is the only one of the two with saved views. It cannot express OR between fields or "is not", and has no typed number or date entry by design (presets only), so "between 1994 and 1997" or "rating under 5" cannot be asked from the toolbar even though the server's query language supports them.
- **Assessment:** **Portico ahead** of Jellyfin, **behind Plex**. Deliberate ("never two number boxes", `browse-toolbar.ts:150-153`). A custom-range row at the end of each preset list would close most of the gap without changing the model (small, client-core plus each client).

### A-18.8 View modes

- **Portico:** Grid or table, remembered per library in the browser; three card sizes stored as a profile preference for the web device class (`Library.tsx:73-84`). The table has fixed columns.
- **Jellyfin:** Grid or list per library tab, plus image type (poster, thumb, banner, and for music disc/logo), show title, show year, "card layout".
- **Plezy:** Grid or list, density, spacing, display scale, episode poster mode.
- **Why it matters:** Poster-only grids are the right default for movies; a thumb/landscape view is what people use for home videos, sports and episodes, none of which Portico has as a library type yet. Portico's table is better than Jellyfin's list (sortable headers, resolution and rating columns).
- **Assessment:** **Trade-off.** Portico's contract confines landscape cards to a show's page (`media-presentation/contract.md` §6.3), so this is deliberate.

### A-18.9 Where sort and filter state lives

- **Portico:** In the address. Opening a library always starts at Discover with default sort; nothing is remembered between visits except a saved view (`Library.tsx:176`, tab change resets sort).
- **Jellyfin:** In local storage per library tab: the last sort, filters and view come back every time.
- **Why it matters:** Portico's links are honest and shareable, and Back works. A viewer who always wants "Date added, newest first" must re-pick it every visit or keep a saved view.
- **Assessment:** **Trade-off (subjective).** Remembering the last sort per library tab on the device is cheap (browser storage, like `libraryViews`) and does not conflict with URLs: the URL still wins when present. See F-18.3.

### A-18.10 Library tabs

- **Portico:** Short, authored lists (table in "How Portico does it"); Discover first and default; no Categories, Genres (except music), Studios, Episodes, Upcoming or Favorites tabs.
- **Jellyfin:** Seven or eight tabs per library, the main grid first and default.
- **Why it matters:** Portico opens on recommendations, Jellyfin on the full grid. People who come to a library to browse everything take one more click in Portico every time (and the choice is not remembered, A-18.9).
- **Assessment:** **Trade-off.** Deliberate (Justin, 2 Oct 2026). A per-profile "open libraries on: Discover / the grid" choice would serve both habits (small).

### A-18.11 One hero for every type; an episode is a panel

- **Portico:** `TitleHero` is shared; a season is URL state on the show page; an episode is a side panel with a deep link (`Spec — Title Pages.md` §2–3; `Show.tsx`, `EpisodePanel.tsx`). `/media/<episode>` redirects into the show (`Detail.tsx:111-114`).
- **Jellyfin:** One details page template for all types; seasons and episodes are full pages, each with its own cast, chapters, media info and "More from this season".
- **Why it matters:** Portico keeps the viewer in the show and avoids three nearly identical pages. The panel is 480 px wide, so an episode's guest cast is a line of text (not linked to people), there are no chapters, no "more from this season" and no room for an episode's own extras.
- **Assessment:** **Trade-off**, deliberate. The panel's limits matter most for long-form episodes (documentaries, specials, anime films filed as episodes).

### A-18.12 Pick before play

- **Portico:** Version, Audio and Subtitles appear under the synopsis only when there is a real choice, preselected; Subtitles reads "Automatic" until changed (`PlaybackChoices.tsx`).
- **Jellyfin:** Version, Video, Audio and Subtitle selects are shown whenever a file has more than one stream (`itemDetails/index.js:197-326`).
- **Why it matters:** Same feature, quieter in Portico. Portico's Version list depends on the catalogue merging files into one title, which the Backlog says it does not do yet for same-folder files.
- **Assessment:** **Portico ahead** on presentation; behind in effect until versions merge (Backlog, "A second file of the same title becomes a second title").

### A-18.13 Technical details: a Files section against a Media Info dialog

- **Portico:** Details ends with Files: one block per file with a summary line, facts, every audio and subtitle track, and the path for owners (`TitleFiles.tsx`, `core/presentation/title-files.ts`). "There is no separate Media information dialog" (`Spec — Page Content` §1).
- **Jellyfin:** "Media Info" opens a dialog with every stream's full properties (profile, level, bit depth, reference frames, colour space, sample rate, default/forced flags…) and a copy button.
- **Plex (public knowledge):** "Get Info" plus the raw XML.
- **Why it matters:** Portico's is visible without a click and readable. Owners debugging "why does this transcode?" want the long form and a way to copy it into a forum post.
- **Assessment:** **Trade-off.** A "Copy details" action on a file block, with the full probe facts, would serve the debugging case without a dialog (small).

### A-18.14 Two action models for one title

- **Portico:** A card's menu is built by `entryActions` from the entry plus a detail read; a movie page's ⋯ is built by `detailActions` with a different set of inputs; show and music pages reuse the card menu with `origin: 'page'` (`Detail.tsx:71-108`, `EntryActions.tsx:101-128`, `Show.tsx` hero ⋯).
- **Jellyfin:** One `getCommands(options)` for cards, lists and the page's More button, with `options` switching items off.
- **Why it matters:** In Portico the same movie offers different actions depending on where you opened the menu (B-18.7). The contract says menus and detail must use the same action IDs and permission results (`media-presentation/contract.md` §15.6).
- **Assessment:** **Portico behind** (accidental). Small: have `Detail.tsx` open the shared sheet with `origin: 'page'` as the show page does, or feed `detailActions` the same capabilities.

### A-18.15 Personal state vocabulary

- **Portico:** My List, Favorite (heart), Watched, a 0.5–5 star rating, Not interested (recommendations only), Remove from Continue Watching; music has no "watched", a book is "Finished".
- **Jellyfin:** Played and Favorite only.
- **Why it matters:** Portico gives recommendations more signal and people a real watchlist. Four personal marks on every title is also more to explain; the hero row is already five buttons wide.
- **Assessment:** **Portico ahead.** Deliberate (`Spec — Title Pages` §1: "Like/Dislike does not exist").

### A-18.16 Bulk actions as server jobs

- **Portico:** Selection actions are one job per 200 items, or one job for the whole filtered set fenced by the catalogue revision, with progress and a failure summary (`SelectionBar.tsx:36-66`, `Library.tsx:117-138`).
- **Jellyfin:** The browser loops over the selected items and calls the API once per item; "Select all" is what is on the page.
- **Why it matters:** Portico can mark 40,000 episodes watched in one action and survives a closed tab. It also means one click can act on far more than is visible (B-18.13).
- **Assessment:** **Portico ahead.**

### A-18.17 Two things called "collection"

- **Portico:** A library's **Collections tab** shows collections that belong to the catalogue (provider "belongs to collection" groupings; entity pages under `/library/<id>/collection/<id>`). **Saved › Collections** are a viewer's own collections, private or shared with the server (`Saved.tsx:186-205`, `Resource.tsx`). "Add to collection…" adds to the second kind. The two are not shown together anywhere.
- **Jellyfin:** One kind: a BoxSet in a server-wide Collections library, made by an admin (or a user with collection-management permission) or automatically from metadata.
- **Why it matters:** A viewer who makes a "Christmas films" collection and shares it will look for it under the Movies library's Collections tab and not find it; it is under Saved. The contract says "Collections are semantic grouping, not playlists with a different name" (§14.5), but the personal kind behaves like an unordered playlist.
- **Assessment:** **Unclear.** Needs a decision: either shared personal collections also appear in the matching library's Collections tab, or the two get different names ("Collections" for the catalogue's, "Lists" for personal).

### A-18.18 The metadata editor

- **Portico:** Every change is a revision-fenced command; a field edited locks itself; the automatic value and its source are shown beside the field; every change is in a History tab with "Undo to here"; bulk edit is a server job (`MetadataEditor.tsx`).
- **Jellyfin:** One form, saved whole; a whole-item lock and a list of lockable field groups; no history, no undo; more fields (ratings, external IDs, air days, status, display order, date added, 3D format).
- **Why it matters:** Portico's is safer (you can see and undo what a refresh or a mistake did) and clearer about where a value came from. It edits fewer fields.
- **Assessment:** **Portico ahead** on design, **behind** on field coverage (F-18.27).

### A-18.19 Fixing a wrong match

- **Portico:** The editor's Matching tab: current match and status, a search box (title only; only for film/TV, since music matching uses scanned tags), candidates, "Use this match", retry, and an identity lock (`MetadataEditor.tsx:460-502`). Selecting a candidate always takes its first episode order (`:491`).
- **Jellyfin:** Identify dialog: search by name and year or by any external ID (IMDb, TMDB, TVDB…), pick a result, optionally replace images.
- **Why it matters:** Searching by ID is the reliable fix for remakes and same-named titles. Portico's Backlog still lists "Fix Match UI" as planned although this tab exists (D-18.7).
- **Assessment:** **Portico behind** slightly. Add "year" and "provider ID" inputs and an order choice (small to medium, server search already takes a query).

### A-18.20 Artwork

- **Portico:** Choose among the provider's candidates per role (poster, backdrop, logo, …), lock the choice, "Find more", "Re-fetch". No upload, no image URL, no removal (`MetadataEditor.tsx:286-325`).
- **Jellyfin:** Per image type: browse remote images by provider with language filter, upload a file, delete, add several backdrops and reorder them.
- **Why it matters:** Owners of home videos, obscure films, bootleg concerts and audiobooks routinely supply their own art. Portico honours a `poster.jpg` beside the file, so there is a way, but it means file access to the media folder.
- **Assessment:** **Portico behind.** Upload is medium (an owner upload route, storage, a candidate with provider "upload").

### A-18.21 Refresh

- **Portico:** One action, "Refresh metadata", which queues a re-fetch for a single playable item (`server/metadata/details.go:30-54`). It is offered on containers too, where the server refuses (B-18.6). Library-level: Scan now, Scan all files.
- **Jellyfin:** A dialog: scan for new and updated files / search for missing metadata / replace all metadata; replace existing images; replace trickplay images; recursive on a series, season, album or library.
- **Why it matters:** "Refresh this whole show" is the common request (new season, corrected episode titles).
- **Assessment:** **Portico behind.** Medium: a container refresh job (the bulk job machinery already exists).

### A-18.22 Delete

- **Portico:** A server preview decides what may be deleted and what must be typed; files optionally kept; a per-library "allow deletion" switch and trash retention; a Deleted titles panel (`delete-media-dialog.tsx`, `LibrarySettings.tsx` Advanced tab). Bulk delete is the opposite: one click, no dialog (`SelectionBar.tsx:95-96`).
- **Jellyfin:** A yes/no confirmation; files go immediately; permission is per user per library.
- **Plezy:** Warns with the count of files and whether a file is shared between items.
- **Why it matters:** Portico's single delete is the safest of the three. The gap between it and bulk delete is very wide.
- **Assessment:** **Portico ahead** for single delete; see B-18.13 for bulk.

### A-18.23 How permission shows in the UI

- **Portico:** The web has one switch, `owner` (role is exactly "owner", `session.tsx:1027`), plus a second, narrower one for metadata tools (owner **and** signed in with a server-local account, `EntryActions.tsx:91`). Members see no admin surface. An "Administrator" tier exists in the server and in the People page but unlocks nothing on the web (B-18.9).
- **Jellyfin:** A policy object per user with about twenty-five flags drives each menu item (`itemHelper.canEdit`, `canEditImages`, `canEditSubtitles`, `user.Policy.EnableCollectionManagement`, `item.CanDelete`, `item.CanDownload`…).
- **Why it matters:** Jellyfin lets an owner hand one job to one person ("my brother may fix subtitles", "my partner may delete from Movies"). Portico is all or nothing.
- **Assessment:** **Trade-off.** Portico's simplicity is a feature for a family server. Either finish the Administrator tier or remove it from the UI.

### A-18.24 People: accounts with profiles, invitations and device approval against users

- **Portico:** An account (server-local or Portico Account member) holds up to eight profiles, each with PIN and limits; invitations by link and code; API keys with scopes; per-account devices with approve/block; limits per account and per profile.
- **Jellyfin:** Users only. Strong per-user controls Portico lacks: access schedule, allow/block by tag, per-type "block unrated", lockout after failed logins, hide from the login screen, delete user.
- **Why it matters:** Portico's structure fits households (one login, several people). It has no "bedtime" schedule and no tag-based allow-list for young children, the two parental controls Jellyfin owners rely on.
- **Assessment:** **Portico ahead** structurally; missing schedule and tag allow-list (F-18.33).

### A-18.25 Library options: a dozen choices against forty

- **Portico:** Source (online or local only), provider, language, country, analysis steps grouped by cost, preview-picture settings, deletion policy. One provider per library by decision.
- **Jellyfin:** About forty (inventory b), including fetcher order per item type, save-artwork-and-NFO, real-time monitoring, embedded-title preferences, automatic series grouping, automatic collections, periodic refresh, subtitle and lyrics download.
- **Why it matters:** Portico's page can be understood in a minute. Options Jellyfin owners actually reach for and Portico has no equivalent of: prefer embedded titles (for well-tagged files), save artwork/NFO beside media (portability), real-time monitoring on/off, automatic collections.
- **Assessment:** **Trade-off**, largely deliberate (`Spec — Page Content` §0.5). The four named above are worth an explicit yes/no each.

### A-18.26 Dashboard and server control

- **Portico:** Health tiles with history (processor, memory, graphics, network, disk), alerts with thresholds, Watching now with Stop and a message, tasks with pause/resume/retry/cancel, viewing statistics, a Play history page.
- **Jellyfin:** Sessions with remote control and messaging, activity log, alerts, running tasks, storage paths, and Restart / Shut down buttons.
- **Why it matters:** Portico's dashboard tells an owner more about the machine. It has no way to restart the server from the web (after a config change or a stuck transcode) and no way to message a viewer without stopping them.
- **Assessment:** **Portico ahead** on insight; missing restart (F-18.37).

### A-18.27 Scheduled work

- **Portico:** A list of scheduled jobs with last and next run, maintenance windows ("only do heavy work inside these hours"), and one priority choice.
- **Jellyfin:** Each task has its own triggers (daily at, weekly at, every N, on startup), a time limit and a Run button.
- **Why it matters:** Portico's model matches its principle (work happens when the library changes, not on timers). An owner still wants "run it now" for a backup or a clean-up.
- **Assessment:** **Trade-off.** "Run now" per job is small.

### A-18.28 Logs and diagnostics

- **Portico:** A structured message log with a live tail over server-sent events, level filter, a single "record more detail for 30 minutes" control, a records view, a redacted support export, and a capabilities panel.
- **Jellyfin:** Raw log files, a viewer with a watch toggle, download.
- **Assessment:** **Portico ahead.**

### A-18.29 No plugins

- **Portico:** None; no extension points in the web app.
- **Jellyfin:** Plugins add metadata providers, subtitle providers, auth providers, notification targets, pages in the dashboard, and whole features (intro skipper, playback reporting, webhooks, Trakt, OpenSubtitles).
- **Why it matters:** Everything a plugin does in Jellyfin must be first-party in Portico or not exist. Several F-entries below are plugin features in Jellyfin.
- **Assessment:** **Trade-off**, deliberate (no external contributions; one provider per library). Outbound webhooks are the cheapest partial answer if integrations matter (see F-18.40).

### A-18.30 Networking

- **Portico:** A status-first "Remote access" page with automatic certificates, port mapping and a policy for who may sign in from outside.
- **Jellyfin:** A manual form; no automation; but a base URL (for reverse proxies under a path) and a remote IP allow/deny list.
- **Assessment:** **Portico ahead** for the common case. Reverse-proxy users will ask for a base path (F-18.42).

### A-18.31 Transcoding settings

- **Portico:** Nine conversion settings and six caps, in plain words ("Encoder speed", "Repackage without converting").
- **Jellyfin:** About fifty, in ffmpeg's words.
- **Assessment:** **Trade-off.** Portico's is right for most owners. Things with no Portico control that owners of weak hardware commonly set: quality (CRF) and per-codec hardware decoding switches.

### A-18.32 Themes, custom CSS and branding

- **Portico:** One dark theme (`web/src/ui/base.css:19` `color-scheme: dark`); no theme choice, no custom CSS, no login message, no splash.
- **Jellyfin:** Seven themes including light, a separate dashboard theme, server-wide and per-user custom CSS, a login disclaimer and a splash image (`themes/`, `apps/dashboard/routes/branding`).
- **Navidrome:** About thirty themes.
- **Why it matters:** Custom CSS is how the Jellyfin community restyles the app; it is also a support burden and breaks with every release. A light theme is an accessibility matter for some people.
- **Assessment:** **Trade-off.** No custom CSS is defensible. A light theme and a one-line login message are the two pieces worth considering.

### A-18.33 First run

- **Portico:** One page; libraries, remote access and a Portico Account are left for later, and the empty Home points to "Add library" (`ServerSetup.tsx:1-12`).
- **Jellyfin:** Six steps including the first library and metadata language.
- **Why it matters:** Portico reaches a working sign-in faster. The owner's first sight of the product is then an empty Home; nothing asks for the display language or walks them to a first scan.
- **Assessment:** **Trade-off.** A short checklist on the empty Home (add a library, turn on remote access, invite someone) would keep the one-page setup and still guide.

### A-18.34 Notifications

- **Portico:** An inbox behind a bell (Unread, All, Archived; profile and admin audiences), an owner broadcast, retention settings, channel reminders as toasts.
- **Jellyfin:** Toasts for server events; a per-session "send message".
- **Assessment:** **Portico ahead.** No push or email by decision.

### A-18.35 Keyboard and input

- **Portico:** `/` for search; `M` and `P` on a focused card; arrow keys within a shelf and an episode list; Esc and Cmd/Ctrl+A in selection mode; ordinary Tab order elsewhere.
- **Jellyfin:** Arrow-key spatial navigation across the whole app (it doubles as the TV client), media keys, a gamepad option.
- **Plezy, Navidrome:** A shortcuts screen; Plezy lets you rebind.
- **Why it matters:** Portico's keys exist but are undiscoverable except by screen-reader hint (`Shell.tsx:167`). There is no key to go Home, to a library, or back.
- **Assessment:** **Trade-off.** A `?` help overlay is small (F-18.53).

### A-18.36 Layout modes

- **Portico:** Three widths: phone (≤720 px, bottom tabs), tablet (≤1080 px, icon rail), desktop. No ten-foot layout on the web; TVs get native apps.
- **Jellyfin:** Auto / Desktop / Mobile / TV display modes; the TV mode is what LG and Samsung apps wrap.
- **Why it matters:** Portico has no client for webOS or Tizen, and its web app would be unusable with a remote there.
- **Assessment:** **Trade-off**, deliberate (native TV apps). Worth stating in the docs that smart-TV browsers are not supported.

### A-18.37 Visual language

- **Portico:** A hero with backdrop and logo on Home and every title; a fixed "atmosphere" background; one poster geometry per row; card scale; a single switch for backdrops and one for reduced motion.
- **Jellyfin:** Backdrops are an opt-in user setting; a "details banner" toggle; blurhash toggle; theme songs and theme videos that play on a title's page.
- **Assessment:** **Portico ahead** on polish and coherence. Theme music is the one piece of "atmosphere" Jellyfin and Plex have that Portico lacks (F-18.16).

### A-18.38 Languages, and how strings reach the screen

- **Portico:** A typed catalogue (4,570 keys) with en-US as source and derived en-GB / en-CA spellings; English only at launch; a lint rejects raw strings. In practice about fifteen screens bind to the en-US catalogue directly and a few dozen strings bypass the catalogue (B-18.12, B-18.22).
- **Jellyfin:** About ninety languages through community translation; display language and date locale chosen per user.
- **Assessment:** **Portico behind** on coverage (a stated launch choice); the binding problems should be fixed before a second language is added.

### A-18.39 Five ways to page

- **Portico:** (1) Windowed endless grid (libraries, person credits, episodes). (2) A sideways shelf that loads more as it scrolls (Home, Discover). (3) A "Show more" button (grid and list sections, track lists). (4) Previous / Next that replaces the page (Saved lists, search groups, pickers). (5) A sentinel that appends on scroll (playlist and collection pages). The owner console adds "Latest / Older".
- **Jellyfin:** Numbered pages nearly everywhere.
- **Why it matters:** Each Portico idiom was chosen for a reason, but a viewer meets three of them within Saved alone. Previous / Next loses the earlier page (and any selection context) and has no total.
- **Assessment:** **Portico ahead** where it uses the windowed grid; the Previous / Next lists (My List, Favorites, History) are the outliers and, for someone with a thousand favorites, the weakest lists in the app. Moving them onto the windowed collection is medium.

### A-18.40 Addresses and links

- **Portico:** Path-based URLs for every destination and pivot; filters in the query; secrets only in the fragment; cards are anchors (`Card.tsx:77-93`).
- **Jellyfin:** Hash routes (`#/details?id=…&serverId=…`); view state not in the URL.
- **Assessment:** **Portico ahead.** Exceptions noted in B-18.25 (some cards are not links) and B-18.17 (a season card's address).

### A-18.41 The browser tab's title

- **Portico:** Always "Portico" (`web/index.html:8`; nothing sets `document.title`).
- **Jellyfin:** Sets it to the page or item name (`scripts/libraryMenu.js:722-775`) and to the playing title (`controllers/playback/video/index.js:267`).
- **Why it matters:** History, bookmarks, tab search and the task switcher all show the same word for every Portico page; screen-reader users get no page announcement on navigation.
- **Assessment:** **Portico behind.** Small (one hook reading the page header).

### A-18.42 Downloads in a browser

- **Portico:** The viewer asks for a quality; the server prepares a file; a Downloads page lists files to save; a whole show can be requested with a policy (all, unwatched, next N) (`app/downloads.tsx`).
- **Jellyfin:** "Download" saves the original file directly; "Download all" for a season or album; "Copy stream URL".
- **Assessment:** **Trade-off.** Portico's is richer and policy-aware; Jellyfin's is instant for "give me the file".

### A-18.43 Where Watch Together starts

- **Portico:** From a card menu or the player; the rail shows it only while in a group (`Shell.tsx:263-270`). On phones there is no rail entry at all.
- **Jellyfin:** A SyncPlay button in the app bar at all times.
- **Assessment:** **Trade-off.** Portico keeps the frame quiet; discoverability is lower, and someone invited by word of mouth has no obvious place to "join" (the join link is the way in).

### A-18.44 Media types

- **Portico:** Movies, TV, Anime, Music, Audiobooks, recordings.
- **Jellyfin:** Also photos, home videos, music videos, e-books and comics (with readers), and mixed libraries.
- **Assessment:** **Portico behind** on breadth, **ahead** on audiobooks. Scope decision.

### A-18.45 Loading, empty and error states

- **Portico:** Page-shaped skeletons, named empty states with the next step, coded error text (never a raw message), a "this changed elsewhere" notice with refresh, one "Reconnecting" state for the whole page.
- **Jellyfin:** A spinner, toasts and generic dialogs.
- **Assessment:** **Portico ahead.**

### A-18.46 Accessibility affordances

- **Portico:** Table, grid, tab and radio roles on custom controls; labels on icon buttons; a reduced-motion setting; the page is made inert under the full player; card shortcuts described once for assistive technology. No light or high-contrast theme, no text-size setting on the web, static tab title.
- **Jellyfin:** Focus management built for remotes; weaker labelling; themes include light.
- **Assessment:** **Portico ahead** on semantics; the tab title (A-18.41) and theme (A-18.32) are the gaps. A proper review belongs to the accessibility auditor.

### A-18.47 How a long-running show is presented

- **Portico:** Four owner switches per show, applied by the server for everyone (hide seasons, absolute numbers, ranges of 100, newest first).
- **Jellyfin:** A "display order" field per series (aired, DVD, absolute and more), a per-user "show missing episodes" option, and specials placed within seasons by air date.
- **Assessment:** **Trade-off.** Portico's ranges and newest-first are ahead for daily shows and long anime; DVD order and missing episodes are behind (Backlog lists both as not built). The web's group switcher is currently broken (B-18.1).

### A-18.48 The account site lives inside the app

- **Portico:** `/account` (sessions, security, servers, notifications) renders inside the product shell when a server is open and in a light shell when not; a server switcher lists the account's servers with online state (`router.tsx:179-182`, `ServerSwitcher.tsx`).
- **Jellyfin:** No account service; a "Select server" page lists saved servers.
- **Assessment:** **Portico ahead.**

### A-18.49 Feedback is built in

- **Portico:** "Send feedback" in the profile menu and "Report a problem" in the player go to an owner-side Feedback panel with diagnostics.
- **Jellyfin:** None.
- **Assessment:** **Portico ahead.** On phones the menu entry is unreachable (B-18.14).

### A-18.50 Back

- **Portico:** Title pages and the person page draw their own Back button and call the browser's history (`Detail.tsx:109`, `Person.tsx:67`, `Entity.tsx`, `Resource.tsx`); a Home row page always goes to Home; a library row page clears the `row` parameter.
- **Jellyfin:** A Back arrow in the app bar on every page, wired to its own router history.
- **Why it matters:** Portico's rule differs per page (B-18.18), and a page opened in a new tab (which the app encourages by making cards links) has no history to go back to.
- **Assessment:** **Portico behind** slightly. One shared "back or up" helper with a per-page fallback parent would make it uniform (small).

### A-18.51 "This device" settings are per kind of device

- **Portico:** Quality limits, playback method, HDR, card size, backdrops and reduce motion are stored per profile **per device class** (web, mobile, television) on the server (`server/operations/preferences.go:139-146,211-213`), and the row says "This device" (`SettingsRows.tsx:28`). Grid/list and the rail's width are truly per browser (`app/preferences.ts`).
- **Jellyfin:** Display and playback settings are stored per user per client app name; quality is per device (local storage).
- **Why it matters:** Setting "720p away from home" on a laptop also sets it on the office desktop browser. Usually harmless; surprising for quality limits.
- **Assessment:** **Trade-off.** Either reword the badge ("Web browsers") or store these per device (B-18.19).

### A-18.52 No mark for "not watched yet"

- **Portico:** Cards show a check when watched and a bar when in progress; nothing for unwatched; no unwatched-episode count on a show (`06 Clients.md` "Card marks").
- **Jellyfin, Plex, Plezy:** An unwatched count badge on shows and seasons (Plezy makes it optional).
- **Assessment:** **Trade-off**, deliberate. The count on a show card answers "is there anything new here?" at a glance; season progress on the show page partly covers it.

## Missing features and capabilities to consider

Each entry: what it is; who has it; the value; rough size; any conflict with Portico's principles; an opinion. "J" = Jellyfin web. Items the Backlog already lists say so.

### Libraries and browsing

**F-18.1 Remove a library (and correct its type).** J: Remove and Rename in the library card menu (`apps/dashboard/features/libraries`: `ButtonRemove`, `MessageAreYouSureYouWishToRemoveMediaFolder`). Portico has no route and no control to delete a library; only its folders can be removed (see B-18.5). Value: essential housekeeping; a library made with the wrong kind is otherwise permanent. Size: medium (a background job that retires the library's titles, personal state and artwork). No conflict. **Opinion: needed before release.**

**F-18.2 Search inside a library.** J: the search page can be scoped; Plex and Plezy have a title field on the grid. Portico hides the server's `title` field from the toolbar on purpose (`browse-toolbar.ts:38`) although the contract lists search-within-library as a peer control (`media-presentation/contract.md` §6.2.1). Value: the fastest way to find one title in a 5,000-film grid without leaving it or losing filters. Size: small (the field and its `contains` / `starts-with` operators exist). **Opinion: add; it also gives the letter rail a substitute while filtered.**

**F-18.3 Remember the last sort (and tab) per library.** J remembers sort, filters and view per library tab. Portico always opens on Discover with the default sort. Size: small (browser storage, URL still wins). **Opinion: add for sort and tab; do not remember filters (a forgotten filter looks like missing media).**

**F-18.4 Play all, Shuffle and Queue for a library or a filtered result.** J: three buttons on every grid (`PlayAllButton.tsx`, `ShuffleButton.tsx`, `QueueButton.tsx`). Portico has Shuffle on albums, artists, playlists; the library page has none because the playback service refuses a query selector in its first release (`Library.tsx:154-157`). Value: "shuffle my 90s comedies", "shuffle all music". Size: medium (server: query selector for a queue). **Opinion: valuable for music above all.**

**F-18.5 Random order.** J: "Random" in every sort menu. Portico publishes sorts from the server; I did not find a random sort in the toolbar model (the label map at `Library.tsx:45` has a word for it, so one may have been planned). Value: discovery. Size: small to medium; conflicts mildly with stable paging (needs a seed per visit). **Opinion: nice to have.**

**F-18.6 Upcoming and missing episodes.** J: an Upcoming tab and a per-user "display missing episodes within seasons" (`displaySettings.template.html`), plus filters "missing" and "unaired". Portico shows only what is on disk. Value: collectors see gaps; viewers see what airs next. Size: large (provider episode lists for shows, kept fresh). Backlog: not listed. **Opinion: the missing-episode marker inside a season is the valuable half.**

**F-18.7 A flat Episodes view.** J: Episodes tab, sortable and filterable. Portico: "Recently aired" orders shows, not episodes. Value: "latest episodes added", "unwatched episodes under 30 minutes". Size: small (an `episodes` pivot tab if the server already browses episodes). **Opinion: optional.**

**F-18.8 Favorites split by type.** J's Favorites tab sections movies, shows, episodes, albums, songs, people. Portico's Saved › Favorites is one grid mixing posters and squares with no type filter. Size: small (type chips, as Search has). **Opinion: add type chips; a music lover's favorites drown their films.**

**F-18.9 More card and view options.** J: image type, show/hide title and year. Plezy: density, spacing, unwatched counts, episode poster mode. Portico: three sizes. **Opinion: low priority; see A-18.8 and A-18.52.**

**F-18.10 Jump rail when filtered, and on other sorts.** Portico hides the rail when any filter is applied or the sort is not title (`Library.tsx:416`), although `formatBrowseAnchor` can already label years, decades, ratings and durations. J's letter picker works with filters. Size: small to medium (server: anchors for filtered sets). **Opinion: at least show year/decade anchors when sorted by year.**

**F-18.11 Folder browse.** J: "Display a folder view" option; Plezy: a folder tree. Value: owners with hand-organised folders and home videos. Conflicts with the catalogue-first model. **Opinion: skip unless home videos become a library type.**

### Title pages

**F-18.12 Media info in full, and copy.** J: Media Info dialog with every stream property and Copy; "Copy stream URL". Value: support and debugging. Size: small (a copy action on the Files block; fuller probe facts if the server has them). **Opinion: add the copy action.**

**F-18.13 External links and provider IDs.** J shows links (IMDb, TMDB, TVDB, MusicBrainz…) on every title and edits the IDs. Portico's show workspace already parses a `providerSourceUrl` restricted to three hosts (`core/show-workspace.ts`) but no screen prints it; only an artist's biography source is linked (`Entity.tsx` Details). Value: "is this the right film?", and jumping to cast and reviews. Size: small. **Opinion: add under Details.**

**F-18.14 More facts as links.** Portico links genres only (`GenreLinks.tsx`). J and Plex link studio, network, year, tags, directors. Size: small (the filter fields exist). **Opinion: add studio, network and year.**

**F-18.15 Chapters on the page.** J: a "Scenes" row of chapter thumbnails that start playback there. Portico generates chapter images (library setting) but shows chapters only inside the player. Size: small. **Opinion: add to movie Details; cheap and attractive.**

**F-18.16 Theme music and backdrop video.** J: per-user toggles "Theme songs" and "Theme videos"; Plex plays TV theme music. Size: medium (a provider or local `theme.mp3`, plus a quiet player). **Opinion: optional delight; off by default.**

**F-18.17 Remote trailers.** J can play internet trailers when the library has none locally. Portico's Trailer button appears only for a local trailer extra (`Detail.tsx:151,197`). Size: medium; conflicts with "nothing through Hosted" only if proxied (it need not be). **Opinion: a "Watch trailer on YouTube" link out is the cheap version.**

**F-18.18 Season info, season cast, show extras, show facts.** All specified in `Spec — Title Pages` §2 and `Spec — Page Content` §2 and not built (D-18.3); end year, status and show rating are in the Backlog. **Opinion: finish what the spec promises or trim the spec.**

**F-18.19 Group and split versions.** J: multi-select "Group versions", detail "Split versions". Portico has a Version control but no way for an owner to say "these two are the same film" (the Backlog notes even automatic merging is missing). Size: medium. **Opinion: needed once versions exist.**

**F-18.20 Person page extras.** J: birthplace, external links, favorite a person, credits sectioned by type. Portico: one grid with Cast/Crew chips. **Opinion: low priority; sectioning movies and shows is the most useful.**

**F-18.21 Reviews.** Plex shows critic reviews. Neither Jellyfin nor Portico does. **Opinion: skip.**

### Owner tools on content

**F-18.22 Upload, link or remove artwork.** See A-18.20. **Opinion: upload is the one owners will miss.**

**F-18.23 Subtitle search, upload and delete from the web.** J: Edit subtitles (search providers, download, upload a file, delete). Portico: none in the web UI that I found. Size: medium to large (provider integration belongs to another auditor; upload alone is small). **Opinion: upload-a-subtitle is the valuable minimum.**

**F-18.24 Lyrics editing and upload.** J: Edit lyrics, upload. Portico: "Fetch missing lyrics" per music library only. **Opinion: low priority.**

**F-18.25 Refresh modes and container refresh.** See A-18.21. **Opinion: add "Refresh this show / album / artist".**

**F-18.26 Identify by ID and year; choose episode order.** See A-18.19.

**F-18.27 More editable fields.** J edits community and critic rating, custom rating, date added, runtime, status, air days and time, display order, studios, people with types, external IDs, and a whole-item lock. Portico's schema (`server/metadata/repair_schema.go:58-110`) covers titles, dates, rating, studio/network, country, numbers, tags, labels, genres and credits. **Opinion: date added (to fix "recently added" after a re-import) and external IDs are the two asked for most.**

**F-18.28 A "what needs fixing" view for video.** J has a Metadata manager; Plex has "Unmatched" filters. Portico has an Unmatched tab for music albums only and a "files need attention" dialog for unparsed episode files. Size: small (a `match` field on movies and shows). **Opinion: add an Unmatched filter for every library; it is the owner's to-do list.**

**F-18.29 Create a collection where collections live.** J: "New collection" on the Collections tab; edit a collection's artwork and description. Portico: only from Saved or a card menu, and personal collections never appear under a library (A-18.17). **Opinion: decide A-18.17 first.**

**F-18.30 Automatic and smart collections.** J: "Automatically add to collection" per library. Plex: smart collections from a filter. Portico's saved views are close to a personal smart collection already. **Opinion: letting an owner publish a saved view to everyone would be Portico's version (small to medium).**

**F-18.31 Playlists: drag to reorder, move to top/bottom, share with named people, import.** J has all four (`itemContextMenu.js:310-340`, playlist editor). Portico: Move up / Move down one step at a time (`Resource.tsx:86-104`). Size: small for drag (Home's Customize already has the code). **Opinion: drag reorder is needed; moving one of 300 tracks by single steps is not workable.**

### People and permissions

**F-18.32 Delete an account.** J: Delete user. Portico's People page can only disable (`People.tsx`; the strings `web.people.deleteTitle/deleteBody` exist unused). **Opinion: needed.**

**F-18.33 Access schedule and tag rules.** J: allowed days and hours per user; allow-only or block by tag; block unrated per type. Portico: rating ceiling, unrated switch, hidden labels. Size: medium. **Opinion: a schedule ("no viewing after 9 pm") is the classic parental control and fits profile Limits.**

**F-18.34 Finer permissions.** See A-18.23. **Opinion: only if the Administrator tier is kept.**

**F-18.35 Edit another person's preferences.** J: "Edit this user's preferences" for admins. Value: setting a child's subtitle or language defaults without signing in as them. Size: small to medium. **Opinion: useful for households.**

**F-18.36 A server-wide devices page.** J: every device, rename, delete, delete all. Portico: devices per account. **Opinion: low priority.**

### Server administration

**F-18.37 Restart and shut down from the web.** J: both, with confirmation (`apps/dashboard/routes/index.tsx`). Portico: none. Size: small. **Opinion: add Restart.**

**F-18.38 Run a scheduled job now.** See A-18.27.

**F-18.39 An activity log of everything.** J: Activity page (sign-ins, failures, plays, plugin events). Portico: Play history, plus an Audit lane in Troubleshooting. **Opinion: mostly covered; a "sign-ins" filter in the audit view would finish it.**

**F-18.40 Outbound webhooks.** J via plugin; Audiobookshelf has notification settings. Value: Discord/Home Assistant notices without plugins or email. Size: medium. Conflicts lightly with "no push or email" (it is owner-configured and outbound only). **Opinion: consider after release.**

**F-18.41 Login message.** J Branding: a disclaimer on the sign-in page. Size: small. **Opinion: cheap and wanted by people who share a server.**

**F-18.42 Base path and remote IP allow/deny.** J networking. **Opinion: base path matters to reverse-proxy users; decide whether sub-path hosting is supported at all.**

**F-18.43 Library defaults owners expect.** J: date-added from file time or import time; save artwork and NFO beside media. **Opinion: the date-added choice prevents the "everything is new again after a migration" complaint.**

**F-18.44 Server paths.** J: cache, metadata and transcode paths. Portico: temp folder only (one state folder by principle). **Opinion: deliberate; skip.**

### Personalisation

**F-18.45 A light theme.** See A-18.32.

**F-18.46 Per-viewer library order and hiding.** See A-18.3.

**F-18.47 More media types.** Photos, e-books and comics, home videos, music videos. Scope decision; listed for completeness.

**F-18.48 Next Up options.** J: max days a show stays in Next Up; include rewatches; episode images. Portico's server decides. **Opinion: "drop shows I abandoned after N days" is the one worth a setting (or a server rule).**

**F-18.49 Spoiler hiding and tap behaviour.** Plezy: blur stills and synopses of unwatched episodes; choose whether tapping an episode plays or opens details. Size: small each. **Opinion: spoiler hiding is a genuinely loved option.**

**F-18.50 Subtitle appearance options.** J: font, colour, shadow, position (inventory b). Portico: size and background. Player auditor's call; listed because the controls live in Settings.

**F-18.51 "Smart" subtitle mode and remembered track choices.** J: show subtitles only when the audio is not in my language; remember the last audio/subtitle choice. Portico's "forced" mode is narrower than "smart". Other auditors own the logic.

**F-18.52 Screensaver / slideshow.** J: backdrop screensaver and photo slideshow. **Opinion: skip for the web.**

### Small conveniences

**F-18.53 A keyboard shortcut help overlay** (`?`), and keys for Home, Search, Back. Navidrome and Plezy have one. Size: small.

**F-18.54 Copy link in the card menu.** Backlog (pass 8). Right-click opens Portico's menu, hiding the browser's.

**F-18.55 Page titles in the browser tab.** See A-18.41.

**F-18.56 Direct download of the original file** and "Copy stream URL". See A-18.42.

**F-18.57 Public share links.** Navidrome and Audiobookshelf can share an album or a book by link to someone without an account. Conflicts with the access model. **Opinion: skip.**

**F-18.58 Viewer statistics.** Audiobookshelf: listening stats per user and per library. Portico: owner-only viewing totals. **Opinion: a "Your year" page is a marketing feature for later.**

**F-18.59 Episode menu in the grid layout, and "See all" for a show's cast.** Parity gaps inside Portico; see B-18.15 and B-18.16.

## Possible bugs and risks in Portico

All from reading source. "Confirmed by reading" means the code path was traced and the outcome follows from the code; it does not mean it was seen happening. None of these appear in `Backlog.md` unless stated.

### B-18.1 On a show with seasons hidden or split into ranges, the group tabs do nothing on the web

- **What goes wrong:** An owner turns on "Hide seasons" or "Split into ranges of 100" (the per-show settings built 2 Oct). The show page then offers groups (All, 1–100, 101–200, Specials) in place of seasons. Clicking any group calls a function that ignores which group was clicked and always asks for the group `unassigned_absolute`. If the show has no such group, the service throws before any request and the click has no effect; if it has one, every tab selects that one.
- **Evidence:** `web/src/app/detail.ts:57` — `const selectGroup = useCallback(() => void service.selectGroup('unassigned_absolute')…` (no parameter). `web/src/screens/detail/Show.tsx:247` passes it as `onGroup`; the switcher calls `onGroup(group.id)` (`Show.tsx:321`, and the dropdown branch below it). `core/show-workspace.ts:83` throws "Choose a server-provided episode group." for an unknown group. The native app's hook takes the id (`apps/portico-react-native/apps/native/src/app/detail.ts:63`).
- **How the comparison surfaced it:** Checking how Portico handles long shows against Jellyfin's display orders.
- **Confidence:** Confirmed by reading. **Severity:** High for the feature (ranges and hidden seasons are unusable on the web past the first group); nothing else is affected.

### B-18.2 The metadata editor silently drops a Tags or Labels edit when the field is already locked

- **What goes wrong:** In the General tab, a change is only registered when the edit has a new text value or a changed lock. List fields (Tags, Labels) carry their change in `values`, which that test does not look at. The first edit of an unlocked list field is saved (because editing also locks it, and the lock change registers). Any later edit of the same field (now locked) registers nothing: the footer keeps saying "No changes yet" and there is no Save button.
- **Evidence:** `web/src/screens/shared/MetadataEditor.tsx:192-200`: list edits set `edit.values` (`:197`), then `if (edit.value !== undefined || edit.locked !== undefined) changes[f] = edit;` (`:199`). The validation loop a few lines later does handle `edit.values`, which shows the intent. Tags and Labels are list fields for movies, episodes and songs (`server/metadata/repair_schema.go:47-49,76-77`).
- **Workaround today:** Toggle the lock off and on, which makes the lock differ.
- **Confidence:** Confirmed by reading. **Severity:** Medium.

### B-18.3 "Remember search history" does nothing on the web, and "Clear" does not clear what the server stored

- **What goes wrong:** Settings › Privacy shows "Remember search history" on the web only. The server keeps up to 20 past queries per profile and offers `GET` and `DELETE /v1/search/history`, but it records a query only when the client sends `record=1`, and no client code sends it or reads the history. The web's "Recent" chips are kept in a module variable: they vanish on reload, ignore the setting, and "Clear" empties only that variable. So the switch changes a stored preference that nothing acts on. If another client (or an older build) did record history, the web offers no way to see or delete it.
- **Evidence:** Row at `core/presentation/settings-structure.ts:268`. Server: `server/catalog/search_history.go:36-55`, `server/httpapi/search.go:150-200`. Web: `Search.tsx:33-45` (`kept`), `:91` (`service.select(value.trim(), selected)` with no `record`), `:120` (`clearRecent`). `grep` for `search/history` in `web/src`, `client-core/src` and the native app: no matches.
- **Confidence:** Confirmed by reading. **Severity:** Medium (a privacy control that does not do what it says; recents that do not survive a reload).

### B-18.4 The Music › Genres tab and the Audiobooks › Series tab stop at 200 entries

- **What goes wrong:** Both tabs read an "aggregate" pivot. The server builds it from the facet counter with a fixed limit of 200 values and reports the total as the number it returned, so there is no next page and no sign that anything is missing. A music library with more than 200 distinct genres (common with untidy tags) or an audiobook library with more than 200 series shows only 200. The web's Genres request asks for `limit: 200` and reads one page.
- **Evidence:** `server/catalog/browse_engine.go:968-988` (`Limit: browseFacetLimit`, `out.PageInfo.Total = len(out.Entries)`); `server/catalog/browse_contract.go:25` (`browseFacetLimit = 200`), `:257,263` (genres and series are aggregate pivots); web `screens/library/LibraryViews.tsx:143`; Series tab defined at `core/presentation/library-tabs.ts:47`.
- **How the comparison surfaced it:** Jellyfin pages its Genres tab; Audiobookshelf has a full Series page.
- **Confidence:** Confirmed by reading (which 200 are kept depends on the facet's ordering, which I did not trace). **Severity:** Medium; it contradicts "correctness over limits".

### B-18.5 A library cannot be removed

- **What goes wrong:** There is no "Remove library" anywhere. The route table has no `DELETE` for a library, the server has no function that deletes one, and the web's Libraries page offers only Edit, Move up, Move down, Rename, Scan and folder removal. A library created with the wrong kind (kind is fixed at creation) or no longer wanted stays in every rail forever; the nearest thing is removing all its folders, which leaves an empty library.
- **Evidence:** `server/httpapi/route_lanes.go` (library mutations are `PATCH /v1/admin/libraries/{id}`, `PUT …/order`, `…/scan-policy`, `…/settings`, `DELETE …/sources/{source}` only); no match for `DeleteLibrary|RemoveLibrary|DELETE FROM libraries` in `server/`; `web/src/screens/server/Libraries.tsx:79,258`. The only "Remove library" button is a mock in the developer gallery (`screens/dev/Gallery.tsx:96`).
- **Confidence:** Confirmed by reading. **Severity:** High as a product gap (not a malfunction).

### B-18.6 "Refresh metadata" is offered on shows, seasons, albums, artists and books, where the server refuses it

- **What goes wrong:** The card menu shows "Refresh metadata" for every entry kind when the viewer is a local owner, and sends the entry's id to `POST /v1/items/{id}/metadata/refresh`. The server accepts only an entity of kind 1 (a playable item) and returns an error for anything else ("direct movie metadata refresh is not available for this item kind"). The menu then says the refresh could not start. The same happens from a show's or album's own ⋯, and bulk Refresh reports `unsupported_item_kind` per item.
- **Evidence:** `web/src/screens/shared/EntryActions.tsx:88-90,125` (`refreshMetadata: !!local`, no kind test); `server/httpapi/detail.go:66-80`; `server/metadata/details.go:14,30-42`; `server/httpapi/bulk_command_hooks.go:94-95`.
- **Confidence:** Probable (I did not trace how a show's public id resolves in `resolveEntity`; if it resolved to an item the outcome would differ). **Severity:** Medium: refreshing a show is the most common refresh.

### B-18.7 A movie's page offers fewer and different actions than the same movie's card

- **What goes wrong:** The movie page builds its ⋯ from a separate model. Compared with the card menu it has no Delete, no Play next / Add to queue, no Watch Together, and it shows "Download…" whenever the movie is playable, without the checks the card menu makes (downloads available on this server; at least one download option for this item). An owner standing on a movie's page cannot delete it; a viewer on a server with downloads switched off still sees Download… and gets a dialog explaining it is off.
- **Evidence:** `core/presentation/detail-actions.ts:107-118` (no `queue`, `group`, `delete`; `download: !isTV && play?.playback ? 'media' : undefined`); `web/src/screens/detail/Detail.tsx:97-108`; card menu `EntryActions.tsx:73-82,101-128`.
- **Contract:** `media-presentation/contract.md` §15.6 requires the same action IDs and permission results in menus and detail.
- **Confidence:** Confirmed by reading. **Severity:** Medium.

### B-18.8 Owner tools depend on how the owner signed in, inconsistently

- **What goes wrong:** The web shows the Server settings, Scan, Delete and bulk Delete to anyone whose role is "owner". It shows Edit metadata, Refresh metadata and their bulk versions only when the owner's session authority is "local" (an account that lives on this server). The server accepts an owner with "hosted" authority on all of these routes. So an owner who signs in with their Portico Account can delete a film and change every server setting but cannot fix its title. Separately, the server shows file paths, lyrics tools and marker tools only to local owners, so those differ too.
- **Evidence:** Client: `web/src/app/session.tsx:1027`; `EntryActions.tsx:91,124-126`; `SelectionBar.tsx:92-96`; `Detail.tsx:71`; `detail-actions.ts:105`. Server: `server/httpapi/owner_authority.go:15-40` (owner routes accept local and hosted), `server/httpapi/detail.go:60,109` and `lyrics.go:97` (local only).
- **Confidence:** Confirmed by reading that the conditions differ; whether a hosted-authority owner session occurs in practice is for the identity auditor. **Severity:** Medium.

### B-18.9 The "Administrator" role can be granted but gives nothing in the web app

- **What goes wrong:** People › an account › Role offers Member or Administrator, and the page says administrators "can manage libraries and people". The web treats only the exact role "owner" as privileged, so an administrator sees no Server heading and no owner tools. On the server most admin routes also require the owner; a few (member management) accept the administrator tier.
- **Evidence:** `web/src/screens/server/People.tsx:85`; string `web.access.peopleLede`; `web/src/app/session.tsx:1027`; `server/identity/access_tier.go:46-60`; `server/httpapi/owner_authority.go:19-26` (`p.Role != "owner"` → forbidden).
- **Confidence:** Confirmed by reading for the web; server coverage of the tier not traced route by route. **Severity:** Medium (a control that promises something the product does not deliver).

### B-18.10 The profile menu offers "Limits and permissions…" and "Delete profile…" to someone who cannot use them

- **What goes wrong:** The shared model adds Limits for every profile and Delete for every non-main profile without checking whether the viewer may manage profiles. The web hides only one action it has no handler for (Forget devices). A person signed in on a non-managing profile therefore sees both, and gets an error when they try.
- **Evidence:** `core/presentation/profile-actions.ts:60,74-76` (no `canManage` test, unlike Rename and Change PIN at `:57-66`); `web/src/screens/settings/Profiles.tsx:134-143`. The group's own caption says "Only the main profile manages…" (`Profiles.tsx:125`).
- **Confidence:** Probable (the model is clear; I did not trace the server's refusal or whether the opening read fails first). **Severity:** Medium for a restricted child profile seeing "Limits and permissions…" on its own entry.

### B-18.11 The check that every preference has a consumer no longer runs

- **What goes wrong:** `npm run check:web` includes a script that fails the build when a preference key has no reader. It looks for the native app at `…/portico-react-native/apps/apple/src`; that folder is now `apps/native`. When the folder is missing the script prints "skipped" and exits successfully. Its exception list also names keys that are no longer in the registry (`music.shuffleDefault`, `music.repeatDefault`, `navigation.*`).
- **Evidence:** `apps/portico-server/packages/client-core/scripts/check-preference-consumers.mjs:39-43,58-64`; `apps/portico-react-native/apps/` contains `native` and `vega` only.
- **Consequence seen in this audit:** B-18.3 (a setting with no effect) is exactly what the ratchet was meant to catch (it would not have, since the server "reads" the key, but the guard is off regardless).
- **Confidence:** Confirmed by reading. **Severity:** Medium (a safety net silently disabled). Overlaps auditor 16.

### B-18.12 About fifteen screens ignore the profile's language choice

- **What goes wrong:** These files take their words from `defaultI18n` (the en-US source catalogue, fixed at module load) instead of the viewer's catalogue: Saved, a saved resource, Search, Person, Notifications (the bell panel), Settings › Profiles and Security, Downloads, Watch Together, DVR, the guide helpers, Server › General, and all four Account pages. Today that means a viewer in Britain or Canada sees "Favourites" in menus and "Favorites" on the Saved tab, and these screens ignore the 12/24-hour choice where they format times through the same object. When a second language ships, they stay English.
- **Evidence:** `const t = defaultI18n.t;` at `screens/saved/Saved.tsx:20`, `saved/Resource.tsx`, `search/Search.tsx:18`, `people/Person.tsx:16`, `settings/Notifications.tsx:10`, and the files listed by `grep -rl defaultI18n web/src/screens`. The viewer's catalogue is `useI18n()` / `currentI18n()` (`web/src/app/i18n.ts:22-37`).
- **Confidence:** Confirmed by reading. **Severity:** Low today, Medium the day a second language is added.

### B-18.13 Bulk Delete has no confirmation, including for "Select all N" (deliberate; design risk)

- **What happens:** In selection mode the owner's Delete acts on one click. With "Select all 1,240" chosen it submits one job for the whole filtered set. There is no preview, count check or typed confirmation; the single-title delete, by contrast, asks the owner to type the title.
- **Evidence:** `web/src/screens/shared/SelectionBar.tsx:95-96` (comment: "Justin's decision: trash is single-step with no preview or confirmation") and `:44-49` (whole-set target).
- **Why record it:** The Delete button sits one position from Refresh and a few from Watched in a bar that changes width on phones (icons only, `compact`). The action is recoverable only if the library keeps deleted titles (`trashRetentionDays` can be 0, `LibrarySettings.tsx` Advanced) and only until it expires.
- **Confidence:** Confirmed by reading that no confirmation exists; what "trash" does to files when retention is 0 was not traced. **Severity:** High potential impact, low likelihood. Deliberate. A confirmation only when the selection is the whole set, or over some size, would keep the fast path for a handful of titles.

### B-18.14 On a phone-width browser there is no way to switch profile or server, send feedback or open Help

- **What goes wrong:** Those entries live in the profile menu, which is part of the rail. Below 720 px the rail is not rendered; the avatar in the top bar goes straight to Settings. Settings has no "switch profile" control (the Profiles panel renames, sets PINs and deletes; it does not switch) and no "switch server" (This device offers Disconnect). So on a phone the only route to another profile is to sign out.
- **Evidence:** `web/src/shell/Shell.tsx:168` (`{!compact ? <Rail …/> : null}`), `:366-430` (`ProfileMenu`, used only by `Rail`), `:452-467` (`TopBar`: avatar → `/settings`).
- **Confidence:** Confirmed by reading. **Severity:** Medium for households using phone browsers.

### B-18.15 A show's cast stops at twelve with no "See all"

- **What goes wrong:** The show page prints the first twelve cast members and nothing more; the movie page has "See all" opening a paged dialog of the full cast and crew.
- **Evidence:** `web/src/screens/detail/Show.tsx:284` (`people.cast.slice(0, 12)`, no action) against `Detail.tsx:209-240`.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-18.16 In the episode grid layout an episode has no ⋯ menu

- **What goes wrong:** The list layout's row has Play and ⋯; the grid layout's card has neither a ⋯ button nor a right-click menu, so Mark watched, Add to playlist and Download are reachable only by opening the panel.
- **Evidence:** `web/src/screens/detail/EpisodeCard.tsx:11-32` (no `onMore`), used at `Show.tsx:273`; the row passes `onMore` at `Show.tsx:274`.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-18.17 A season card's address stops working after switching season

- **What goes wrong:** A card whose destination is a season opens `/show/season?library=…&season=<season id>` (the literal word "season" stands in for the show id). Choosing another season on that page rewrites the address to `…&season=<number>`. Reloading, bookmarking or sharing that address gives the page neither a show id nor a season id; the workspace request is rejected before it is sent and the error is swallowed, so the page shows its loading skeleton indefinitely.
- **Evidence:** `web/src/app/open.ts:21-24`; `Show.tsx:66-73` (route built without a show id when it is "season" and without a season id when the parameter is numeric), `:136-139` (`chooseSeason` writes the number); `core/show-workspace.ts` `validateShowWorkspaceRoute` throws when all three ids are absent; `web/src/app/detail.ts:52-55` catches and logs only in development.
- **Confidence:** Probable; it needs a season card to exist somewhere in the web UI (the server emits them in a show's content projection, `server/catalog/content.go:344`), which I did not confirm is reachable. **Severity:** Low.

### B-18.18 Back buttons behave differently per page and can leave Portico

- **What goes wrong:** The person page always calls `history.back()`. Title pages call it when `history.length > 1`, otherwise go Home. `history.length` counts the tab's whole history, including whatever site was open before Portico, so on a page opened from a link in a chat, Back leaves the app. A page opened in a new tab (cards are links) has length 1: Back goes Home on a title page, and does nothing on a person page.
- **Evidence:** `people/Person.tsx:67`; `detail/Detail.tsx:109`; `detail/Show.tsx` (`back`); `library/Entity.tsx` (`back` falls back to the library); `saved/Resource.tsx` (falls back to Saved).
- **Contract:** NAV-8, Web: Back unwinds Portico before leaving the application.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-18.19 "This device" badges sit on settings that apply to every browser the profile uses

- **Evidence and effect:** See A-18.51. `SettingsRows.tsx:28` prints the badge for rows whose values are stored in the `profile-device-class` scope (`server/operations/preferences.go:139-146,211-213`).
- **Confidence:** Confirmed by reading. **Severity:** Low (a label that misleads).

### B-18.20 Non-owners can add one title to a collection but not several

- **Evidence:** The card menu offers "Add to collection…" whenever the server does (`EntryActions.tsx:120`); the selection bar shows Collection only to the owner (`SelectionBar.tsx:92`), although personal collections belong to any viewer (`Saved.tsx` New collection).
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-18.21 Changing someone's role takes effect on selection, with no confirmation

- **Evidence:** `web/src/screens/server/People.tsx:77,85`: the Role select calls `setRole` in `onChange`, outside the page's Save.
- **Confidence:** Confirmed by reading. **Severity:** Low (and moot while B-18.9 stands).

### B-18.22 English text written straight into components

- **What goes wrong:** Despite the string lint, these are not in the catalogue: the card menu's accessible name "Actions for …" (`EntryActions.tsx:48`); "Added to …", "This item could not be added…", "The collection changed…" (`EntryActions.tsx:306-307,321`); most of the metadata editor's chrome ("Edit …", "pending changes", "No changes yet", "Automatic", "From …", "No candidates yet", "locked / unlocked", role and history labels, the mixed-kinds warning: `MetadataEditor.tsx:28,34,44,248,254,283,301,366,402,506,563,650`); sort names in category browses (`Library.tsx:45`, with a lint exemption); "N of M" on grid sections (`Sections.tsx:114`); "Edit <username>" (`People.tsx:126`); library kinds and chapter modes in library settings (`LibrarySettings.tsx:82,186`); "Task" in the jobs table; "<remaining> left" (`core/presentation/detail-actions.ts:74`). The player's post-play card is also raw English (`web/src/player/PostPlay.tsx:53-62`; player auditor).
- **Confidence:** Confirmed by reading. **Severity:** Low at an English-only launch.

### B-18.23 Disc headings depend on an English phrase inside a subtitle

- **Evidence:** `web/src/app/library-words.ts:131-136` finds the disc number by matching `/^Disc \d+$/` in a track's subtitle text, and `Entity.tsx` groups multi-disc albums with it.
- **Risk:** If the server ever words or translates that subtitle differently, multi-disc albums silently become one flat list. A disc number field on the entry would remove the coupling.
- **Confidence:** Confirmed by reading (latent). **Severity:** Low.

### B-18.24 Dates and language names follow the browser, not the profile's Language & region

- **Evidence:** `toLocaleDateString(undefined, …)` and `Intl.DisplayNames(undefined, …)` in `detail/Detail.tsx:256-259`, `detail/TitleFiles.tsx:42`, `detail/EpisodeCard.tsx` (`formatAirDate`), `library/LibraryViews.tsx:42`, `shared/Sections.tsx` (added date), `people/Person.tsx:18-21`, `shared/MetadataEditor.tsx:280-281,513`. The profile's locale is available as `useI18n().locale`.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-18.25 Cards in Saved are not links

- **What goes wrong:** `06 Clients.md` says title cards are links so middle-click, new tab and Copy link work. Cards in My List, Favorites, History, a collection page and a saved view page are built without an address, so they are buttons.
- **Evidence:** `saved/Saved.tsx:204-209` (`MediaGrid`), `:195` (History), `saved/Resource.tsx:172-173` — no `href`; compare `shared/Sections.tsx:290` and `people/Person.tsx:125`.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-18.26 Search words are written into the address

- **What goes wrong:** The search page puts the query in the URL (`/search?q=…`), so it enters browser history and any synced history. The navigation contract says search text is excluded from route URLs and history payloads (NAV-6.8), and the router file's own header comment says "Search text is deliberately not a route parameter" (`router.tsx:51`) forty lines above the route that defines `q` (`router.tsx:118-120`).
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (privacy on a shared computer; the profile's "Remember search history" switch does not govern it).

### B-18.27 Parts that exist but cannot be reached or do nothing

- Pinned libraries and pinned saved resources: `core/library-pins.ts` and server routes `/v1/saved-pins/*` exist; no web or native screen calls them.
- The library settings' provider API-key field (`LibrarySettings.tsx:173`) renders only when a provider reports `supportsApiKey`; no provider in the server sets it (`server/administration/library_settings.go:109` is the only definition). The specs say there are no API keys.
- "Forget remembered devices" is in the profile action model and hidden on the web for want of a handler (`Profiles.tsx:137`).
- Registry keys nobody can set from any client: `privacy.includeInWatchTogether`, `playback.startedThresholdPercent`.
- About 530 of the catalogue's 4,570 strings are referenced by no client source (web, client-core, native app, Vega app or server), by a literal-and-prefix search. The largest groups are `web.live.*` (52), `web.hostedAccount.*` (34), `settings.server.*` (32) and `web.console.*` (23): leftovers of screens that were replaced. Some of the count will be keys built at run time.
- **Confidence:** Confirmed by reading (the string count is an estimate). **Severity:** Low (maintenance weight, and translators would be paid to translate dead text).

### B-18.28 Home's "See all" guesses the destination from the row's id

- **Evidence:** `home/Home.tsx:41` sends a library row to that library, adding "sorted by date added" when the row's kind or id matches `/recent|added/i`; every other library row lands on the library's default tab (Discover), which is not "all of this row".
- **Risk:** A new server row with a library (say "Top rated in Movies") gets a See all that shows something else. The row could carry its own destination, as Discover rows do (`seeAll`).
- **Confidence:** Confirmed by reading (latent). **Severity:** Low.

### B-18.29 Removing an entry from a playlist or collection is immediate, with no Undo

- **Evidence:** `saved/Resource.tsx:109` and `:178` run the removal from the menu item. Elsewhere Portico pairs instant removal with Undo (Continue Watching, Not interested).
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-18.30 The episode panel loses facts on a direct visit and does not link people

- **Evidence:** The air date comes only from the card that opened the panel (`EpisodePanel.tsx` description uses `preview?.airDate`), so a deep link or reload shows none. Guest cast, director and writer are plain text; on the movie page the same people are links.
- **Confidence:** Confirmed by reading. **Severity:** Low.

## Where Portico is stronger

Worth keeping, and worth saying out loud when comparing with Jellyfin:

1. **Every view is a link.** Library tab, sort, filters, season, episode panel and settings page all have addresses; cards are anchors. Jellyfin's view state is invisible to the URL.
2. **The browse toolbar.** Server-published fields with counts, presets instead of number boxes, applied filters as chips, and **saved views** that can sit on Home. Jellyfin has nothing like saved views.
3. **A grid with no pages** that restores its exact place on Back without a request, and a jump rail that costs one request at any library size.
4. **Bulk actions that really are bulk:** one fenced job for a whole filtered set, with progress and a failure summary.
5. **A metadata editor you can trust:** per-field locks that set themselves, the automatic value shown beside yours, full history with undo.
6. **Single-title delete** with a server preview, a typed confirmation, shared-file awareness and a trash.
7. **Personal state:** My List, favorites, star ratings, Not interested with Undo, Remove from Continue Watching with Undo, a viewer-visible History with Clear.
8. **One Settings screen with search**, authored once for every client, with plain-language server settings (status first, one Save per page).
9. **Profiles inside accounts**, with PINs and limits, invitations, scoped API keys and device approval.
10. **The owner's dashboard:** machine health with history, alert thresholds, stop-with-message, a live structured log, a redacted support export, a capabilities panel, remote storage sources, backups with restore.
11. **Notifications as an inbox**, and a built-in feedback channel from viewer to owner.
12. **States:** page-shaped skeletons, named empty states that say who can fix them, coded error text, one "Reconnecting" state.
13. **Audiobooks** as a first-class type (authors, series, Finished, resume).
14. **Pick-before-play** that appears only when there is a choice.
15. **Search** with counts per type and a Top result.

## Doc/code disagreements

- **D-18.1 Ten server pages or eleven.** `06 Clients.md` ("ten pages, one Save per page") and the comment at `settings-structure.ts:288` say ten; the structure defines eleven (Dashboard, Play history, Libraries, Live TV & DVR, People, Playback, Remote access, Storage & backups, Schedule, Troubleshooting, General — `settings-structure.ts:302-460`).
- **D-18.2 Library pivots.** `media-presentation/contract.md` §6.1 lists Categories for movies and TV, Episodes for TV, Collections and Genres for audiobooks, "Tracks" for music. The authored tabs (`library-tabs.ts:32-49`, 2 Oct) have none of Categories, Episodes or audiobook Collections/Genres, add "Recently aired", and call tracks "Songs". The contract is stale.
- **D-18.3 The show page against its spec.** `Spec — Title Pages.md` §2 promises a season ⋯ with "Season info (season poster, overview, air dates)", a "Cast · Season 2" row with a "Whole series" toggle, an Extras section, and Details with network, studio, country, original title, genres; `Spec — Page Content` §2 adds first aired and status. Built: Play season, Mark watched, Show settings; show-level cast only; no Extras on a show; Details with network, studio, country, genres (`Show.tsx:189-202,284-285`). The Backlog covers end year, status and rating, not the rest.
- **D-18.4 "No API keys, never pick a source."** `Spec — Page Content` §0.5 says the owner never chooses individual sources and there are no API keys. The library Metadata tab shows a provider select per media kind and has an API-key input (`LibrarySettings.tsx:160-176`). The key field is unreachable today (B-18.27); the provider select is live. The Backlog notes "two providers under one media kind are allowed by validation".
- **D-18.5 Search text in the address.** NAV-6.8 forbids it; `router.tsx:51` says it is deliberately absent; `router.tsx:118-120` and `Search.tsx:69-74` put it there (B-18.26).
- **D-18.6 Link contract.** `Spec — Link Contract.md` lists `/account?tab=profiles|sessions|security`. The route accepts `sessions|security|servers|notifications` (`router.tsx:182`); `tab=profiles` silently opens Sessions.
- **D-18.7 "Fix Match UI" is listed as planned** in the Backlog's last section; the editor's Matching tab is that UI (`MetadataEditor.tsx:460-502`).
- **D-18.8 Search within a library** is a required peer control in `media-presentation/contract.md` §6.2.1; the toolbar hides the title field (`browse-toolbar.ts:38`).
- **D-18.9 Same actions in menus and detail** (`media-presentation/contract.md` §15.6) — not so for movies (B-18.7).
- **D-18.10 "Title cards are links"** (`06 Clients.md`) — not in Saved (B-18.25).
- **D-18.11 "Background work is deprioritized, never paused"** (the brief's summary of the principle). The owner console offers Pause on a running job (`web/src/screens/server/Activity.tsx:34`) and a switch "Only do heavy work inside these hours" (`web.maintenance.pauseHeavyWork`). Both are the owner's choice rather than the server's, so this may be consistent with the intent; the wording of the principle should say so.
- **D-18.12 The preference-consumer check** is described in `06 Clients.md` as part of `check:web`; it exits early (B-18.11).
- **D-18.13 Collections and playlists "Play / Shuffle"** (`Spec — Title Pages` §1 table; `Spec — Page Content` §7 "Hero with a mosaic"). A playlist page has both. A personal collection page has neither and uses a plain page header with a small mosaic, not the shared hero (`Resource.tsx:144-185`).
- **D-18.14 Person page.** `Spec — Page Content` §6 says the filmography is "grouped by movies and shows, with role chips". The page is one grid with All / Cast / Crew chips (`Person.tsx:86`).
- **D-18.15 Downloads as a destination.** NAV-5.1 lists `downloads` as "handheld/offline-capable clients only"; the web has `/downloads`, a phone bottom tab and a profile-menu entry for server-prepared files. Probably the contract predates web downloads.

## Open questions for Justin

1. **Removing a library** (B-18.5): is its absence intended? If not, what should happen to history, My List entries and playlists that point into it?
2. **The Administrator role** (B-18.9): finish it (which Server pages and owner tools does an administrator get?) or take it out of the People page?
3. **Owners signed in with a Portico Account** (B-18.8): should they get Edit and Refresh metadata, file paths and the other "local owner" tools?
4. **Collections** (A-18.17): one idea or two? Should a shared personal collection appear in its library's Collections tab, or should personal ones be renamed?
5. **Bulk delete** (B-18.13): keep one click for everything, or confirm when the selection is "all N" or above a size?
6. **Search in the address** (D-18.5): keep linkable searches and amend NAV-6.8, or take the words out of the URL?
7. **Search history** (B-18.3): wire the web's Recent list to the server's history (so it follows the profile and the setting means something), or remove the setting and the server feature?
8. **What a library opens on** (A-18.10, F-18.3): always Discover, the last tab used, or a preference? Should the last sort be remembered?
9. **Search inside a library** (F-18.2): the contract requires it and the toolbar hides it. Which is right?
10. **Phone-width web** (B-18.14): is it a supported surface? If yes, where do Switch profile, Switch server, Feedback and Help go?
11. **Smart-TV browsers** (A-18.36): confirm the web app is not meant for them, and say so in the docs.
12. **Per-profile library order or hiding** (A-18.3): wanted? The pins plumbing exists unused.
13. **"This device"** (A-18.51): should quality and appearance be per browser, or should the label say "all web browsers"?
14. **Theme and branding** (A-18.32): a light theme? a login message? Custom CSS is presumably a no.
15. **Library options** (A-18.25): of prefer-embedded-titles, save-artwork-and-NFO-beside-media, real-time monitoring on/off and automatic collections, which (if any) should exist?
16. **Artwork upload** (A-18.20) and **subtitle upload** (F-18.23): in scope for release?
17. **Refresh for a whole show, album or artist** (A-18.21): in scope? Today the menu item fails (B-18.6); at minimum hide it.
18. **Restart from the web** (F-18.37): wanted?
19. **Delete an account** (F-18.32): wanted, and what happens to its profiles' data?
20. **Parental controls** (F-18.33): is a viewing schedule in scope?
21. **Unwatched counts on show cards** (A-18.52) and **spoiler hiding** (F-18.49): still a no / worth a setting?
22. **Should a collection be playable** (D-18.13)?
23. **Dead strings** (B-18.27): clear them before the first translation is commissioned?
24. **Episode pages** (A-18.11): is the 480 px panel enough for feature-length episodes, or should the panel be able to expand to a full page?

## Evidence boundary

- Nothing was executed. No browser, no server, no tests, no build. Every "what goes wrong" is derived from reading code; the confidence line on each B-entry says how far the path was traced.
- Layout, spacing, focus rings, contrast, animation, touch targets and real responsiveness cannot be judged from source; statements about phone and tablet layouts describe which components render at which width, not how they look.
- Server behaviour was read only where a UI question needed it (permissions, limits, the preference registry). I did not trace `resolveEntity`, the trash job, facet ordering, or which routes accept the administrator tier.
- The catalogue "dead string" count is a text search and over-counts keys assembled at run time.
- Jellyfin's behaviour is taken from its source at commit `319dbfc` and from long familiarity; I read templates and command lists rather than running it. Statements about its defaults (page size 100, per-client home settings) are from prior knowledge and were not re-verified line by line.
- Plex and Emby statements are from public knowledge only.
- The Channels guide, DVR, Downloads, Watch Together, the Portico Account pages and the sign-in flows were inventoried, not audited.
- The native apps were not compared screen by screen; one hook was read to confirm B-18.1 is web-only.

## Outside my domain

Short notes for the auditors who own these areas:

- **Player (17):** `web/src/player/PostPlay.tsx:53-62` has raw English strings ("Still watching?", "Up next", "Playing in Ns"), unlike the rest of the app.
- **Catalogue / browse (03, 10):** aggregate pivots are capped at 200 and report the cap as the total (B-18.4). The `aired` episode sort and container grids are already in the Backlog.
- **Identity (01):** the administrator tier is accepted by member-management functions but not by `ownerAuthorityTx`; the web's `owner` flag ignores it (B-18.9). The "local owner only" rule is applied to some owner features and not others (B-18.8).
- **Web engineering (16):** `check-preference-consumers.mjs` exits 0 when it cannot find `apps/apple/src` (B-18.11); about fifteen screens import `defaultI18n` at module scope (B-18.12).
- **Server API:** `GET`/`DELETE /v1/search/history` and `/v1/saved-pins/*` have no client. `POST /v1/items/{id}/metadata/refresh` accepts only kind-1 entities while clients send container ids.
- **Operations:** job Pause and "only inside these hours" against the "never paused" principle (D-18.11).

