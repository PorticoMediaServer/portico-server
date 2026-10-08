# Frontend review: web, iOS and tvOS (1 Oct 2026)

Reviewed against the demo server (`demo.getportico.tv`, owner session) in Chrome at 1512×805 and 430×932, the "Portico iPhone" simulator and the "Portico TV" simulator, plus the web, Apple and client-core source. This is an opinion document: nothing was changed in code.

**Not covered:** iPad, sign-in and onboarding, a live Watch Together session, casting / Play on, offline downloads, DVR recording, profile restrictions. The guide could not be judged because the demo server answers `GET /v1/guide` with `503 guide_unavailable`. Cast and artist photos are missing on the demo, so people rows were judged on layout only.

**Side effects of the review:** the demo profile now has four plays (Dune, two Severance episodes, one Pink Floyd track, one Breaking Bad episode on TV). Nothing else was changed.

---

## 1. Verdict

The visual foundation is good and worth keeping: one token set shared by all three clients, a calm dark palette, consistent type, a rail that works on web and TV, a strong hero, a metadata editor that is better than Plex's. The product looks like one product.

What holds it back is not the styling. It is four structural things:

1. **Settings are the storage schema, rendered.** Personal preferences are grouped by the key prefix of each field, in registry order. The server console is 15 sections that grew by feature, with the same concept in two or three places.
2. **Sort and filter exist three times**, each different, and on web the main one is currently broken.
3. **Detail pages are well behind your own spec.** `Spec — Page Content by Media Type` describes complete pages; the built pages have roughly the hero and the primary list. Music and audiobooks reuse video vocabulary.
4. **Library types have no real hierarchy of their own.** TV and Anime are "a grid of shows"; Music is "three grids"; nothing in the browse structure is specific to how people use those media.

Plus a layer of bugs that make the web app feel less finished than it is. Those come first because several of them distort any judgement of the design.

---

## 2. Broken today

| # | Where | What happens | Cause |
|---|---|---|---|
| B1 | Web › library › **Filters** | The panel opens off-screen (at y = −1288), so nothing appears. | `Chip` in `web/src/ui/Tabs.tsx:46` takes five named props and forwards neither `ref` nor the rest. Radix `Popover.Trigger asChild` has no anchor, so the content stays at its unpositioned `translate(0, -200%)`. |
| B2 | Web › library › **Sort** | The menu never opens. | Same `Chip`: `DropdownMenu.Trigger asChild` passes `onPointerDown`, which `Chip` drops. Used in `BrowseFilters.tsx:49` and `:54`. |
| B3 | Web › Server › **Storage** | "Something didn't load"; the whole console frame is replaced. | `ReferenceError: Cannot access 'p' before initialization`. `screens/server/Storage.tsx:61–62` read `mountIds` and `davIds` inside the `useService` factory, which `useMemo` runs during render, but both are declared with `useState` further down at lines 73–74. |
| B4 | Web › library › grid → list | List rows keep the grid's row height (one row every ~330 px) until reload. | Not traced fully. `WindowedGrid` holds the measured row pitch in state and appears not to re-measure when the presentation changes. |
| B5 | Web › show page | A "Nothing here yet" block is printed under the episodes. | `Show.tsx:217` renders a `StateView` when the recommendation read is ready and empty. Same in `Detail.tsx`. Your spec §0.1 says an empty section is omitted. |
| B6 | Web › Server › Overview › Load | "Graphics processor" and "Video encoder" tiles: the label is overprinted by the help text and clipped. | Not traced. The "No GPU reporter" sentence is rendered into a tile sized for a number. The same sentence appears three times on the page. |
| B7 | tvOS rail, collapsed | The collapsed rail is an empty strip: no icons, only the edge of the selected row's highlight. | The comment in `apple/src/shell/Rail.tsx:109` describes exactly this as fixed; the running build still shows it. Either the simulator build is older or the `marginLeft: hiddenWidth` switch does not apply. |
| B8 | tvOS library browse | The "Jump to" button overlaps the top of the first poster. | Header rows are stacked without reserving the button's height. |
| B9 | Web + tvOS title | Genres read "Science Fiction, Adventure, Science Fiction". | `genres.slice(0, 3)` without de-duplicating across providers (`Detail.tsx`). |
| B10 | Everywhere | Resolution reads "1,080p", "1,440p". | `pref.unit.height` is `'{value}p'` and the number is locale-formatted. Appears in Preferences and in the player's quality footer. |
| B11 | Web player | Button says "Quality **Original**", its menu's selected row says "**Automatic** · Plays the file as is". tvOS says "Quality · Automatic". | Two labels for one state. |
| B12 | Web categories | Selecting a decade shows the chip "Category: decade:2020". | Raw category id used as the label. |
| B13 | Web › Dune | No director anywhere; Crew shows only Hans Zimmer although the editor lists 138 cast and crew. | The detail's first credit page carries no directing credit, so both the "Directed by" line and the key-crew shelf come up empty. Likely server-side selection of key crew. |
| B14 | Web episode panel | Opening the panel scrolls the page behind it back to the top. | Probably the route change to `/show/$id/episode/$id` resetting scroll; not traced. |
| B15 | Web music | "The Beatles" sorts under T in Music; "The Avengers" sorts under A in Movies. | Article stripping is applied to one sort title and not the other. |
| B16 | tvOS | `portico://item/<showId>` does nothing, silently. | Not traced; the same link with a movie id opens the movie. Matters if Top Shelf ever links a show. |

B1 and B2 together mean that, on web today, a library cannot be sorted or filtered at all except through the five quick chips. That alone probably explains a good part of your unease about sort and filter.

---

## 3. Loading and the shell being torn down

You noticed this while I was navigating. Three separate things produce it:

1. **Every reload shows the full-screen "Opening Portico / Checking your saved connection" card.** `Gate.tsx:19` returns `<RestoreScreen />` whenever `session.phase === 'restoring'`, and `restoring` starts as `true` on every load. A signed-in user with a stored session should never see this. Render the shell immediately from the stored session and cached library list, and show the restore card only when there is no stored session or restore fails.
2. **The rail builds itself in three steps.** Libraries, then (a second later) the Channels group, then the Notifications badge. Each step moves everything below it. At 805 px tall the rail then overflows; it scrolls, but with `scrollbar-width: none`, so Settings and the profile are simply cut off. Cache the rail's contents per server, pin the footer, and let only the middle scroll.
3. **Route-split screens show nothing while their chunk loads.** `split()` in `router.tsx` uses `<Suspense fallback={null}>`, so opening a show gives a blank content area, then the page. The library header does the opposite: it renders four speculative tabs (Discover / Movies / Collections / Categories) and then drops to the real two for TV. Both should be a page-shaped skeleton that does not change shape.

On iOS the same pattern exists in a different form: "Switch profile" replaces the whole app, tab bar included, with a mostly empty "Who's watching?" screen, and Back returns you to Home rather than to Settings where you were. That should be a sheet over the current screen.

Artwork is the other half of perceived loading. Below-the-fold rows show placeholders for one to two seconds after they scroll into view. Prefetch one viewport ahead.

---

## 4. Personal settings

### What is there

Settings is a short hub (account, server, Preferences, Notifications, Downloads, Watch Together, Link a TV, About). All real preferences sit behind one row, on one page, generated from the server's preference registry. The group of a field is the part of its key before the first dot (`operations/preferences.go:60`), and groups are emitted in registry order. The result on web:

- The jump bar reads **Playback, Music, Audiobooks, Playback, Streaming, Quality, Music, Privacy, Search, Language and region, Appearance, Notifications**. Playback and Music each appear twice because fields from other groups sit between them. The second "Playback" button scrolls to the first (duplicate `id`).
- "Music" and "Audiobooks" are each a box containing only "Default speed".
- Labels that have no catalogue entry fall back to the key: "Sleep timer minutes", with the choices 0 / 30 / 60 / 90 / 120.
- "Counts as started 5" and "Counts as watched 95" have no unit. "Ask after 3" (iOS and TV correctly say "3 episodes").
- Preferred audio and subtitle languages are a text box for comma-separated codes ("en, fr"), although the code comment above it says languages are never free text.
- **Streaming** exposes the engine's policy matrix: "Play original files", "Repackage when needed" and "Allow conversion", each with Allow / Prefer / Never / Require. Twelve combinations, most of them meaningless or contradictory to a person.
- **Quality** is four network rows (Home, Wi-Fi, Cellular, Other), each with a mode, a resolution ceiling and an unlabeled switch (it is HDR). Mode and ceiling overlap ("Original quality" with an 8K ceiling). A desktop browser is shown Cellular.
- The eyebrow says "Saved to your profile", but Streaming says "this browser" and Appearance says "applies to this browser".
- On tvOS the same page is one list of 40-odd rows, so the last setting is about forty presses away.
- Notifications, Downloads and Watch Together are destinations, not settings, yet they are rows here and also items in the web rail.

### What I would build

Stop rendering the registry. Keep the registry as the storage and validation contract, and author the settings structure by hand, once, in client-core, as a list of pages → sections → rows that reference registry keys. Every client renders that structure. A key that no page references does not appear.

```
Settings
├─ Profile & account      name, avatar, PIN, password, two-step, devices, sign out
│                         (Switch profile / Switch server are actions in the profile menu)
├─ Playback               Auto-play next episode (+ countdown)
│                         Still watching? (after N episodes)
│                         Skip intros / credits / recaps
│                         Skip back / forward amounts
│                         Default speed: Video · Audiobooks      (drop Music speed)
│                         Watched threshold (percent)            (advanced)
├─ Audio & subtitles      Preferred audio languages   (picker, ordered)
│                         Preferred subtitle languages (picker, ordered)
│                         Subtitles: Off / Forced only / Always
│                         Subtitle appearance: size, background, with a live preview
├─ Quality & data         On this network: Automatic / Original / limit to 1080p / 720p …
│                         Away from home: the same
│                         On cellular: the same            (phones only)
│                         Playback method: Automatic (recommended) /
│                           Always play the original / Always convert
│                         HDR: Automatic / Off
├─ Music                  Volume leveling, crossfade, gapless, show lyrics
├─ Appearance             Card size (Small / Medium / Large), backdrops, reduce motion,
│                         default library view (Grid / List)   "this device"
├─ Language & region      Language, time zone, clock
├─ Privacy                Pause history, show my activity, reset recommendations,
│                         remember searches
├─ This device            Name, Link a TV, downloads storage (phones)
└─ About
```

Rules that go with it:

- **One page per group, a two-pane layout on web and iPad** (groups on the left, rows on the right), a pushed list on iPhone, and on TV a left column of groups with the rows on the right, so nothing is forty presses away.
- **Per-device and per-profile are labelled on the row** ("This device"), not in a page eyebrow that is half wrong.
- **The streaming matrix becomes one choice.** The three tri-state policies stay in the registry for API clients; the UI writes a consistent combination.
- **Quality becomes "limit" per network**, one control per row, with HDR as its own row.
- **Every row gets authored copy and a unit**, or it does not ship.
- **Search** in settings on web once there are more than ~30 rows.
- Notifications becomes a bell in the shell (rail footer on web, top bar on phone), not a settings page with a "Back to Settings" link. Downloads and Watch Together leave Settings (see §8).

---

## 5. Server settings ("Manage server")

### What is there

A second navigation column inside the rail's content area with 15 sections in three groups. Individually many pages are careful. Together they are hard to use, for these reasons:

**The same thing lives in several places.**

| Concept | Where it appears |
|---|---|
| People and access | **People** (accounts, Invite, Add account, "Your profiles") and **Access & API keys** (People again with roles, Invitations again, Devices, API keys) |
| Devices | Settings › Account lists five; Server › Access says "No devices yet" |
| Scheduling | **Scheduled tasks** is empty ("No schedules"), while the window editor is in **Maintenance › Background work**, and nightly backups run without appearing in either |
| Storage | **Media › Storage** is remote sources (rclone, WebDAV); **Maintenance › Storage** is disk usage. Same word, different things |
| Log retention | Maintenance ("Keep logs for") and Logs & diagnostics ("Retention › Server records") |
| Version and updates | General › Version and Maintenance › Updates |
| Who is watching | Overview › Watching now and Activity › Watching now |
| Processor and memory | Overview › Resources and Overview › Load, on the same page |
| Scan depth and metadata source | On the library page and again inside its Settings dialog |
| Personal profiles | Settings › Account and Server › People › "Your profiles" |

**Settings are filed by implementation, not by intent.** "End paused videos after", "Quality limit away from home" and "Upload speed" are in Network under "Access from outside". "Who can record" is in Live TV. Alert thresholds are in Maintenance.

**Pages are long forms with several save bars.** Network has three Discard / Save pairs; Live TV & DVR has three; Logs & diagnostics has eight sections including three different ways to get more detail ("Detail kept", "Record everything for a while", "Temporary capture").

**Status and settings are mixed.** Every card carries its own "Updated just now · Refresh" (seven on Overview). Network's remote access shows "Unconfigured", "Remote access is being checked" and "Checked 4 hours ago" together.

**It is incomplete where owners look first.** Transcoding has three controls and nothing about hardware acceleration, encoder, tone mapping or temp location. I found no way to reorder libraries or give one a scan schedule. There is no subtitles section, no metadata section, no playback-defaults section, and no place for DLNA, which you have already decided on. Library status says "Finished with warnings" and "Degraded" with no way to read the warnings.

**Rows that say "not available on this server"** (four in Recording defaults) contradict your own rule that absent features are hidden.

### What I would build

Split what the owner *watches* from what the owner *sets*, and group settings by what they are for.

```
Manage server
│
├─ Dashboard        Now playing first, then alerts, then health (CPU, memory, GPU, network,
│                   disk) as one row of tiles with one refresh for the page.
│                   Background activity (scans, analysis) with progress.
│                   Viewing statistics below the fold.
│
├─ Libraries        List with item count, size, last scan, status (click → the warnings).
│   └─ a library    Tabs: General (name, type, folders, visibility) · Metadata (agent,
│                   language, ordering) · Scanning (when, what to analyse, cost shown) ·
│                   Advanced (deleting, trash). A page, not a modal.
│
├─ Live TV & Channels
│   ├─ Sources      tuners and playlists, guide data
│   ├─ Recording    who may record, rules, defaults, space
│   └─ Library Channels
│
├─ People           One list of accounts. An account opens to: role, libraries, limits
│   ├─ Invitations  (streams, quality, recording), profiles, devices.
│   └─ API keys
│
├─ Playback         Transcoding: allow, hardware acceleration, encoder, tone mapping,
│                   temp folder, concurrent limits.
│                   Streaming limits: remote quality cap, upload speed, end paused streams.
│                   Subtitles and audio defaults.
│
├─ Remote access    Status sentence first ("Reachable at … · HTTPS on").
│                   Portico Account connection, HTTPS certificate, who may sign in from
│                   outside, home networks, extra addresses, advanced.
│
├─ Storage & backups  Disk usage with clean-up, retention (one place), remote sources,
│                   backups and restore, deleted titles.
│
├─ Schedule         The maintenance windows editor and every scheduled job, including
│                   backups, with last and next run.
│
├─ Troubleshooting  Logs, capabilities, support export, viewer feedback.
│                   One "Record more detail for 30 minutes" control instead of three.
│
└─ General          Server name, updates and version, DLNA, alerts.
```

That is ten entries instead of fifteen, and each of today's duplicates gets one home. Further:

- **One save model.** Either every control saves on change with a visible confirmation (as Preferences does), or the page has one sticky save bar. Not three bars on a page.
- **The console replaces the rail, it does not sit beside it.** Entering Manage server should swap the rail's contents for the console navigation with a "Back to Portico" row at the top. Two navigation columns leave about 920 px for content on a 1512 px screen.
- **Use the same byte units everywhere.** Overview says MB and GB, Maintenance says MiB and GiB.
- **Log lines need dates** (entries four days apart show only a time), and a line the server logged as a warning should not be tagged INFO.
- **On phones**, "Manage your server on the web" is a reasonable position, but the Dashboard (who is watching, scan now, alerts) is exactly what an owner wants on a phone. I would ship that one screen natively.

---

## 6. Sort and filter

### What is there

Three different systems:

1. **Library browse, web:** five quick chips, a Filters chip, the count, a Sort chip, "Save view", and a letter rail. Filters and Sort are broken (B1, B2).
2. **Category browse, web:** opening a category from the Categories tab shows the same grid with a *different* toolbar: sort as three chips (Title / Date added / Year), no Filters, no letter rail, a different column count, and the raw chip "Category: decade:2020".
3. **Apple:** Filters and Sort come first, then quick chips. Sort is a clean sheet. Filters is a sheet of about 27 centred accordion headings in registry order (Year, Decade, Date added, Play state, Favorite, Watchlist, Genre, Collection, Content rating, Availability, Release date, Your rating, Last played, Tag, Label, Audience rating, Critic rating, Resolution, Dynamic range, Audio language, Studio, Network, Duration, Actor, Director, Writer…). Year is open by default, showing two number boxes with the placeholders "0" and "9999". On Apple TV that means the first thing the filter dialog offers is typing a year with the remote.

Other problems:

- Nothing shows what is selected. A collapsed accordion heading carries no summary, so after choosing "Drama" and "1990s" the sheet looks unchanged until you reopen each one.
- The quick chips duplicate fields that are also in Filters (Play state, Favorite, Watchlist) with no shared state shown.
- Sort options are thin. The iOS sheet (the only one I could open) offers Title, Added, Year, Duration, Audience rating, Your rating, For you and Last played. There is no Release date, Critic rating or Random, and "Duration" is offered for shows.
- The Saved tab uses a third layout again: "SHOW" chips and "SORT BY" with a separate Ascending chip, shown even when the list is empty.
- The web list view is not a real list: title, year, "1m", date added, with no columns to sort by.
- The Categories tab mixes decades, genres and every studio, including dozens with one title, in one grid under a heading that repeats the tab name. The mosaic artwork did not load in the time I watched.
- On tvOS the browse page spends five stacked rows (eyebrow, title, tabs, filter bar, count, Jump to) before the first poster.

### What I would build

One toolbar, identical in structure on every grid in the product (library, category, collection, search "See all", Saved):

```
[ Sort: Title ↑ ]  [ Unwatched ]  [ Genre ▾ ]  [ Year ▾ ]  [ Resolution ▾ ]  [ + Filter ]      80 movies   [▦ ▤]
   Drama ✕   1990–1999 ✕   Clear all
```

- **Sort is one control** that shows field and direction; choosing the current field flips direction.
- **Three or four filter chips per library type are promoted** to the bar: Movies → Genre, Year, Resolution; Shows → Genre, Status, Network; Music → Genre, Decade; Audiobooks → Author, Narrator, Genre. Each opens its own small menu with counts and a search box.
- **"+ Filter" opens the full list**, grouped: *Your activity* (play state, favorite, watchlist, your rating, last played), *About the title* (genre, year or decade, content rating, studio or network, people, collection, tags), *About the file* (resolution, HDR, audio language, duration, date added, availability). Decade is a preset inside Year, not a second field.
- **Applied filters are removable chips** on a second line, with Clear all. This is the missing feedback.
- **Ranges are pickers**, never raw number boxes: a year range from the library's actual minimum and maximum, rating as steps.
- **On TV**, filters are a side panel on the right with two levels (field → values), each a short list. No text entry.
- **Categories goes away as a tab.** Genres, decades and studios are filters. If you want a browsable "Genres" page, make it genres only, with artwork that loads.
- **The list view becomes a table** on web: poster thumbnail, title, year, runtime, rating, resolution, date added, with clickable column headers. Server owners will live in it.
- **The letter rail stays** for title sort. For other sorts show the matching scale (years, date added) or hide it.
- **Save view** is good. Give saved views a home in the library header (a menu beside the title) rather than only under Saved.

In code, this is one `BrowseToolbar` model in client-core (promoted fields per library kind, grouping, labels, summaries) rendered by each client, replacing the two web toolbars and the Apple sheet. `Chip` needs `forwardRef` and prop spreading regardless.

---

## 7. Title pages

Your spec is right. The pages are not there yet. Against `Spec — Page Content by Media Type`:

| Page | Specified | Built |
|---|---|---|
| Movie | Content rating, logo, cast with photos, key crew, Extras, Collections, Details | Hero and synopsis are good. No content rating, no tagline (the editor has "It begins."), no studio, no director, no Details section, no collection. A lone "SD" chip sits under the poster. Media information is five lines |
| Show | Years range, content rating, community rating, cast and crew, More like this, Extras, Details | Hero facts are start year, season count, network, studio, genres. **No cast at all on web or iOS** (`Show.tsx` has no credits shelf). No Watchlist or Favorite button; the iOS More sheet has only "Mark show as watched" and "Edit metadata". No rating, no status |
| Episode panel | Air date, guest stars, director and writer, Media information, actions | Good structure. No air date. Cast is one comma-separated sentence. "Directed by" lists four names. No menu for download, playlist or media info |
| Artist | Image over background, genres, country, years, biography, Popular tracks, albums by type, similar artists | Initial in a circle, "1 album · 4 songs", an unlabelled alphabetical track list, then Albums. The library grid says 8 songs for the same artist |
| Album | Artist link, type, label, genres, total length, Shuffle beside Play, More by artist | Artist link is blue on a grey blur and nearly invisible. Facts end in "0 watched · 4 unwatched". Actions are Play, then on a second row "Mark as watched", Download, Shuffle. Tracks show "1m" rather than 3:42 |
| Audiobook | Cover, author, narrator, series, duration, description, chapters, More in series | Title, "1 item · 0 watched · 1 unwatched", Play, "Mark as watched", and a "Book parts" list. No cover, author or description |

### Changes I would make beyond closing those gaps

**Hero**

- Movies and shows get the same action row: **Play / Resume**, **Watchlist**, **Watched**, **⋯**. Today movies have Play, Watchlist, heart, ⋯ and shows have Play and ⋯.
- Cut the personal signals from four to two. Today a title can be put on the Watchlist, hearted as a Favorite, Liked or Disliked, and Rated in stars. Keep **Watchlist** (intent) and **one** rating (stars, or thumbs if you want it to drive recommendations). Favorite and Like mean the same thing to a person.
- Put the things people decide with on the facts line: year · content rating · runtime · genres, then rating badges (show more than `ratings[0]`). Under the synopsis: "Directed by …" and "Starring …" as links.
- Replace the floating "SD" chip with a **version line** beside the actions: `4K · Dolby Vision · Atmos · English, French subtitles`, opening Media information.
- Add **audio, subtitle and version selection before playing** (three compact menus under the actions). Plex and Jellyfin owners expect to pick the track or the 4K versus 1080p file before pressing Play.

**Body**

- Order per your spec: primary content → people → related → extras → details.
- **Details** as a two-column definition list at the bottom: studio, country, release date, original title, collection, languages, external links, and a "Files" disclosure with path, size, container, video, audio and subtitle tracks, bitrate. Media information today is too thin for this audience.
- Related rows: hide any row with fewer than three items, and never show two rows with the same contents. Dune shows "Starring Timothée Chalamet" and "With Timothée Chalamet" with the same two posters, plus two one-poster genre rows.
- Cast: show the first 12 with a "See all" page. A 49-person horizontal scroller is not usable. On tvOS the cast circles are empty; draw initials there as web does.

**Show page**

- Web episodes: the current grid is three 380 px stills per row, so a 22-episode season is eight screens. Use a **list** on web (still at 200 px, then number, title, air date, runtime, two-line synopsis, progress, ⋯) with the grid as an option. This is where I would depart from the spec's "grid of 2–4 columns".
- Season switcher: move "Play season" and "Mark season as watched" into the switcher's ⋯ menu, as the spec says. Today "Play Season 1" floats in the middle of the row.
- Episodes need watched ticks, progress bars and air dates. None are visible now.
- "Episodes" sits 20 px under the poster with no separation from the hero.
- Missing stills fall back to a flat "S1 E1" tile, which is correct per spec; tint it with the show's colour as the spec asks.

**Music and audiobooks**

- No "watched" on music, ever. Albums do not need "Mark as watched" or watched counts. Audiobooks use "Finished" and "Not started", which the audiobook filter chips already do.
- Durations in audio contexts are `m:ss` or `h:mm:ss`, not "1m".
- The album artist link needs a readable colour on the blurred cover.

---

## 8. Library types and navigation

### The rail

The web rail has up to 16 items in four blocks: Home, Search, Saved, Downloads, Watch Together · five libraries · Channels, source, Recordings · Manage server, Notifications, Settings, profile, collapse.

- **Watch Together** is a form for creating or joining a group. It is not a place. It belongs on a title ("Watch with others") and in the player, with the active group shown as a pill in the shell.
- **Downloads** on web is a list of files the server prepared. It belongs under the profile menu. On iPhone it is a real destination and deserves its tab.
- **Notifications** becomes a bell with the badge, opening a popover.
- **Settings** and **Manage server** move into the profile menu (the menu already says who and where you are), leaving the rail for content: Home, Search, Saved · libraries · Channels.

The three clients disagree about primary navigation: web rail (above), mobile web tabs (Home, Library, Saved, Search), iPhone tabs (Home, Library, Channels, Saved, Downloads, with Search as an icon), tvOS rail (Home, Search, Saved, Channels, libraries, Settings). Pick one set of five for phones — **Home, Library, Search, Saved, Downloads** with Channels inside Library — and use it for both mobile web and iPhone.

"Saved" holds Watchlist, Favorites, Playlists, Collections, Saved views and History. History is not saved, and Collections are owner-made. I would call the destination **My Library** or split it: Watchlist and History are things you watch; Playlists and Collections are things you organise.

### Home and library Discover

A library's Discover tab shows the same rows as Home, filtered to one library (Recommended for you, New for you, Trending now, Recently added). With five libraries that is the same page six times. Give each library type a Discover of its own, or drop Discover and open libraries on the grid with one "Continue" strip above it.

Home itself: Continue Watching is correctly first. After it I would put **Recently added** (the row Plex and Jellyfin users look for first), then recommendations. Today the order is Recommended, New for you, Trending, then five "Recently added in X" rows at the bottom, two of them without "See all". "Recommended for you", "New for you" and "Picks for you" are three names for nearly the same idea.

Continue Watching cards show the show poster and "In Perpetuity / Severance". Add "S1 E3" and time left. On iOS and tvOS in-progress items also carry the blue "unwatched" dot, which is contradictory; web shows no unwatched marks at all. Decide once: I would mark unwatched with a corner on every platform and never on an item that has progress.

### TV and Anime

Today: Discover and a grid of shows. A show is seasons and episodes. Anime is identical, including "Anime" printed as the network.

- Tabs: **Shows · Recently aired · Calendar · Collections**. "Recently aired" (episodes by air date) and "Up next" are how people actually use a TV library.
- Show cards carry an **unwatched count** badge and, in list view, the year range and status.
- Build the per-show settings in `Spec — Title Pages §4` (hide seasons, absolute numbering, ranges, newest first). For anime that section *is* the hierarchy: One Piece as "Season 1, Episode 1043" is wrong, and it is the first thing an anime user will check. Show the original title as a subtitle, not in the facts line.
- Specials last, as specified.

### Music

Today: Discover, Artists, Albums, Songs. Each is a grid of large squares, including Songs, where 40 songs fill eight rows.

- Tabs: **Home · Artists · Albums · Songs · Genres · Playlists**.
- **Songs is a table**: title, artist, album, duration, plays, date added, sortable. Search results for songs likewise (rows, not 200 px squares).
- Album cards always show the artist. Today "21 / 2011 · 4 songs".
- Artist page sections by release type (Albums, Singles & EPs, Compilations, Appears on), per your spec.
- Untagged files currently create a second copy of each album ("Abbey Road (2019 Mix)" twice, one without art) and an "Unknown artist" with 14 songs titled "01 Second Hand News". Group these under an **Unmatched** view for the owner rather than mixing them into the album grid.
- **Playing a track should not take over the screen.** Clicking a track on web opens the full-screen Now Playing. Start playback in the mini player and keep the user on the album; the full view opens on click. The mini player needs previous, a scrubber, and shuffle and repeat. The current track should be marked in the track list.
- Now Playing shows the title twice, puts the artwork small in the top-left with an empty middle, and offers 10-second and 30-second skips and a Speed control for music. Use a centred cover with lyrics beside it, and hide skip and speed for music (keep them for audiobooks).

### Audiobooks

- **Square covers.** Your rule says posters for audiobooks; I would revisit it. Audiobook art is square almost without exception, so a 2:3 frame either crops the cover or letterboxes it. Music is already square, and a library is one type, so the "a row never changes shape" rule still holds.
- Cards show author; clamp titles to two lines (one title currently wraps to four).
- "Watchlist" in an audiobook library should read "Want to listen" or just reuse Saved.

### Search

- The field scrolls away with the page. Keep it and the type chips sticky.
- Results are capped at five per section, leaving an empty column at 1512 px. Fill the row.
- Movie and show results drop the year that every other card has.
- Section order differs from chip order (People is fifth in the chips, last in the results).
- "Books" here, "Audiobooks" everywhere else.
- Add a **Top result** block. A query that matches one title exactly should lead with it.
- Search text is deliberately not in the URL (`router.tsx`). I would put it there: reload, back and sharing a search all break without it.
- Episode results are three identical show posters in a row. In the Episodes section specifically I would use stills. Every item in that row is an episode, so the row's shape does not vary.

---

## 9. Player

The layout is fixed by your rule, so this is about content and wording only.

- **No playback information.** "More" holds Report a problem and Keyboard shortcuts. An overlay showing direct play or converting, video and audio codec, bitrate, resolution, dropped frames and buffer is expected by this audience and is your best support tool.
- No Play on / cast control and no Watch Together entry in the video player on web.
- Subtitles menu shows only "Off" when the file has none. Offer "Search for subtitles" there if the server can.
- Five or six labelled pills are always visible (Audio, Subtitles, Quality, Speed, Sleep, Up next / More). Sleep is rare for video; move it under More.
- tvOS shows shuffle and repeat for a TV episode, and titles it "Pilot / Episode 1" without the show or season. Web shows "Severance · S1 E2". Use the web form everywhere.
- tvOS uses a fully custom transport. That is a choice, but it gives up the system behaviours Apple TV users know (swipe-down info, Siri "what did they say"). If you keep it, match the system's gestures.
- Web: the page behind the player stays in the accessibility tree and focus order. Mark it inert while the player is open.
- The player is not a route, so the browser Back button does not close it and a reload loses it.

---

## 10. Platform notes

### Web

- Cards are `<button>`s, not links. No middle-click, no "open in new tab", no link preview, no right-click copy link. Title cards should be anchors.
- Every card carries the hidden sentence "Press M for more actions, P to play." Screen readers will hear it 60 times on Home. Say it once per row or put it in a help overlay.
- Page titles sit at slightly different heights (Search 45 px, Saved 50 px, library 67 px with an eyebrow). The header component should fix this.
- Density: 6.5 cards across at 1512 px with 176 px posters. A "Card size" setting exists as a bare number field (100). Make it Small / Medium / Large and put it in the grid toolbar.
- Mobile web: the translucent tab bar lets the row heading underneath show through it.
- The "Customize Home" dialog is good. Add drag to reorder alongside the arrows.

### iPhone

- **There is no navigation bar.** Pushed pages scroll under the status bar and Dynamic Island with nothing behind them (episode text collides with the clock on a show page), the Back button scrolls away with the content, and on Home the row heading shows through a translucent band. Every pushed screen needs a bar that appears on scroll, with Back and the title.
- The Home backdrop stays fixed and remains visible, dimmed, behind the rows after scrolling.
- The show page has a full-width Play button and then a lone "··· More" button on its own row.
- The filter sheet's rows are centred text with chevrons and edge-to-edge inputs that do not align with the title. It should look like an iOS grouped list with the current value on the right.
- Settings rows: "demo.getportico.tv / demo.getportico.tv" as title and subtitle; "Signed in to demo.getportico.tv as demo" directly above it.
- The simulator showed "Favourites"; if that is the en-CA trial locale it is fine, otherwise it breaks the US-English rule.
- Empty-state copy differs from web for the same state ("Nothing saved here yet." versus "Your Watchlist is empty").
- The accent blue renders more saturated on Apple than on web. Worth one side-by-side check of the token.

### tvOS

- **Rail focus is nearly invisible.** A focused rail item gets a dark fill (`colors.raisedHigh`, `ring={false}`, `scale={1}`), which is almost the same as the selected item's fill. Content uses a bright ring. From a sofa you cannot tell which rail item has focus. Use the same ring, or a light fill with dark text.
- The collapsed rail shows nothing (B7). It should show icons, with the selected one marked.
- The expanded rail takes 18% of the width on every screen where focus is in it.
- Settings and Preferences are full-width rows 1,700 px wide with the value at the far right edge. Use a two-column layout and keep label and value within one glance.
- Home did not show Continue Watching until I navigated away and back, after playing on another device. Refresh Home when the app or the screen regains focus.
- Opening a library lands focus on the first card, below the tabs, and the browse grid opens with its header scrolled off. Fine for speed, but the tab row should be reachable with one Up, and the header should not need five rows (B8).

---

## 11. Code observations

Only what bears on the frontend's quality.

- **`Chip` is the root of B1 and B2.** Any component used as a Radix `asChild` trigger must forward its ref and spread unknown props. Worth a lint rule or a test that mounts each trigger.
- **Settings and filters are schema-driven with fallback labels** (`sentenceFromKey`). That guarantees nothing is missing and also guarantees nobody designed it. Keep the registry for validation; author the presentation.
- **Detail screens are single large components** with inline styles and repeated `currentI18n().t(...)` calls (`Detail.tsx` is 315 lines of one function; `web/src/screens` has 526 inline `style={{…}}`). Web `Show.tsx` and `Detail.tsx` have drifted apart: one has cast and Watchlist, the other does not. A shared `TitlePage` composition (hero, sections by slot) driven by one client-core model per media type would keep them aligned and would make the spec's section order enforceable.
- **Hardcoded English in web detail code:** `Season ${…}, Episode ${…}`, `kindLabel` ("Movie", "Episode"), `qualityLabel` ("SD"). These bypass the catalogue.
- **Readability.** `packages/client-core/src` has 571 lines longer than 300 characters and 31 longer than 1,000 (for example `library-management.ts:76` is a whole method on one line). Web has 556 lines over 300. Code like that cannot be reviewed, and your working rule is review over tests. Run a formatter with a line limit across client-core and both clients before the UI work starts.
- **Good:** tokens generated from one design package for every client; shared presentation helpers in client-core (`detailActions`, `preferenceValueLabel`, library words); windowed grids with a server position index; the artwork store with metrics; the route table with real URLs for every destination; the connection overlay that keeps the shell.

---

## 12. Suggested order

1. **Fix the broken things** in §2 (B1–B5 and B7 first). Half a day of work removes most of the "unfinished" feeling.
2. **Shell and loading** (§3): no restore card for a stored session, stable rail, page skeletons, rail contents reduced (§8).
3. **Sort and filter** (§6): one toolbar model in client-core, three renderers.
4. **Title pages** (§7): do the spec's gap analysis per page, then build movie → show → album → artist → audiobook. Unify the hero actions first.
5. **Personal settings** (§4): authored structure in client-core, then the three layouts.
6. **Server console** (§5): regroup to ten sections, one save model, then fill Playback and Libraries.
7. **Library types** (§8): TV per-show settings and Recently aired; Music table, mini player behaviour and release types; audiobook covers.
8. **Platform polish** (§10): iPhone navigation bar, tvOS rail focus.

Decisions I need from you before building: the two-signal proposal (Watchlist plus one rating), square audiobook covers, removing Categories as a tab, moving Settings and Manage server into the profile menu, the phone tab set, and list-first episodes on web.
