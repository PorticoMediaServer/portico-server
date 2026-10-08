# Frontend rework — handoff (2 Oct 2026)

For the agent continuing this work. Read this whole file, then the review it implements:
`portico-internal/Project Architecture/Audits/2026-10-01-frontend-review.md` (441 lines; §2 bugs, §3 shell,
§4 settings, §5 console, §6 sort/filter, §7 title pages, §8 library types, §9 player, §10 platform notes).
Where this file and the review disagree, **this file wins** (it carries Justin's later decisions).

You may complete everything that remains: the rest of pass 4, passes 5–8, and the iPhone/tvOS ports of
every pass. The previous lead (Fable) will review your work on return, so keep the log in §9.

---

## 1. Justin's rules (non-negotiable)

1. **All eight passes, on web, iPhone and tvOS, now.** Split the work however you like; do not stop at a
   pass boundary to ask whether to continue.
2. **No Luna or Sol lanes, no subagents for frontend work.** Do it in the lead session.
3. **Tests: almost none.** No unit/e2e runs while building. At most `npx tsc --noEmit` and
   `npm run lint:strings` (web) at the end of a pass, plus a short functional check in the browser or
   simulator of what you just changed. Full unit and end-to-end runs happen once, on final builds, after
   all passes. Do not write new tests as you go. Spend tokens on delivery.
4. **Never idle.** If a build or server is starting, work on the next piece.
5. **No AI attribution** in commits (no Co-Authored-By, no "Generated with"), and no model or agent names
   in code, files, branches or commit messages.
6. **Pre-release: no sacred cows.** Prefer the structural fix. Change APIs and schemas now rather than
   keep a shim. No real users, demo data is disposable.
7. **US English.** Hide features that are disabled; never show "not available on this server" rows.
8. **Scale rule:** client cost is O(visible), never O(library). Keep windowed collections; never load a
   whole library to sort, filter or count.
9. **Correctness over limits:** never truncate sort/filter/search/credits; a capped view always has a
   "See all" that pages the rest.
10. **Artwork shapes:** posters for film/TV/anime (including episodes outside the show page), landscape
    stills only inside a show page and in Search's Episodes section, music square, artist hero circle.
    **Audiobook covers become square** (decided in this rework; see pass 7).
11. Review your own diff rather than adding tests. Get the shape right the first time.

## 2. Decisions Justin gave (apply exactly)

- **One Settings screen.** Personal and server settings are one destination. Pages are listed on the
  left under two headings: **Account** (personal) and **Server** (owner only). There is no separate
  "Manage server" destination.
- **The web rail always stays visible.** The console never replaces it.
- **Phone tabs:** Home, Library, Channels, Saved, Downloads. **Search is a button in a standard top bar
  on every page.**
- **"Saved" keeps its name and all its contents.**
- **Library Discover tab stays and is the first thing a library opens on.** Do not demote the
  recommendation rows.
- **Card marks:** no unwatched mark anywhere; watched = a check (as Plex); in-progress = the progress bar
  at the bottom of the card. No unwatched-count badges either.
- **Heart = Favorite**, the same icon and word on every media type. **Like/Dislike is removed as a
  concept throughout Portico.** Star ratings stay. "Not interested" covers hiding.
- **Pick before play:** three dropdowns (Version, Audio, Subtitles) under the title in the hero,
  preselected so the usual viewer never touches them.
- **Episode lists:** rethink on every platform; ambitious is welcome. He dislikes the current one most.
- **Player options row:** keep Sleep visible. All buttons shown when there is room, collapsing to one
  button at small sizes. The player layout itself is fixed.
- Everything else in the review is agreed: Categories tab removed (genres/decades/studios are filters),
  Watch Together off the rail (shown only while in a group), Downloads under the profile menu on web,
  Notifications as a bell, stills in Search's Episodes section, list view as a real table, and so on.

## 3. Where things are

- Root: `/Users/justin/Documents/Codex Projects/Portico/`
  - `apps/portico-server` — Go server (`server/`), web client (`web/`, React 19, TanStack Router, Radix
    via `radix-ui`, CSS modules), shared packages `packages/client-core`, `packages/design` (tokens →
    `web/src/ui/tokens.css`), `packages/i18n` (`src/catalog/en-US.ts` shared, `en-US/web.ts` web only).
  - `apps/portico-react-native` — `apps/apple` (one React Native app for iOS and tvOS) with its own
    copy of the shared packages. Sync with `node scripts/shared-packages.mjs --write` after changing
    `packages/` in portico-server. Screens in `apps/apple/src/screens`, shell in `src/shell`.
  - `portico-internal` — specs (`Project Architecture/`), this file, `Backlog.md`, deploy scripts.
- **Branch `frontend-rework`** is checked out in portico-server, portico-react-native and
  portico-internal. Work there. Rollback point: tag `pre-frontend-rework-2026-10-02` in all five repos.
- **Commits** (portico-server needs the identity inline):
  `git -c user.name="Justin Ehler" -c user.email="justin@MBP.local" commit -m "…"`.
  Commit at least once per pass per repo, with a plain description of what changed.
- **Local dev (web):** build `cd server && go build -o ../.scratch/portico-server ./cmd/server`, run
  `apps/portico-server/.scratch/dev-local.sh` (binds 127.0.0.1:32500, state in `~/PorticoDev/state`,
  media in `~/PorticoDev/media`) and `npm run dev` in `web/` (http://127.0.0.1:5173, proxies /v1 and
  /v2). Run both as background shell commands; the preview launcher cannot start them here. The owner
  test account is in `.scratch/DEV-ACCOUNT.md` (localhost only; do not print it in chat).
  A TMDB token is read by the launcher from the demo host; never echo it.
- **Apple:** the iOS and tvOS simulators have the app installed. tvOS has no touch: drive it through
  Device Hub (`com.apple.dt.Devices`) with arrow keys and the toolbar's Select/Back/Home buttons;
  `portico://item/<id>` deep links work. Screenshots through the simulator tool.
- **Demo deploy** for Justin's review at pass boundaries: `portico-internal/scripts/deploy-demo.sh <commit>`.
  Demo state is disposable (back up first, as the script does).
- Go tests/benchmarks belong on the runners, not the Mac — but see rule 3: none until the end.

## 4. What is done

portico-server, branch `frontend-rework`:

- `fb27fde` — passes 1–3 on web.
  - Bugs B1–B6, B10–B12, B14, B15 fixed (`Chip` is `forwardRef` and spreads props; Storage hook order;
    `WindowedGrid` re-measures on presentation change; no "Nothing here yet" under titles; GPU tile;
    heights; quality label; category chip names; article stripping with empty language).
    **Still open from §2 of the review: everything not in that list** (B7 tvOS collapsed rail, B8 tvOS
    Jump-to overlap, B13 Dune's missing director — check on local data, plus B9 and B16; read §2).
  - Shell: rail cache (`web/src/app/rail-cache.ts`), `ShellSkeleton`/`PageSkeleton`, route fallback is
    a page skeleton (the shell never tears down), rail = Home/Search/Saved (+Together only in a group) ·
    libraries · channels · footer with bell popover, Settings, profile menu (Downloads inside). Phone
    top bar has Search and the bell; bottom bar Home/Library/Channels/Saved/Downloads.
  - Sort/filter: `packages/client-core/src/presentation/browse-toolbar.ts` (`toolbarModel`,
    `nextToolbarSort`, `fieldSelectionSummary`, `decadePresets`) and the web `BrowseBar` in
    `web/src/screens/library/BrowseFilters.tsx`.
- `4782bfc` — pass 4 on web (title pages). **Compiles (`tsc` clean, strings lint clean) but has not
  been checked in a browser.** Your first job is that check (§6, pass 4).
  - client-core: `presentation/title-page.ts` (`relatedRowsForPage`, `dedupeNames`, `showPeople`,
    `showCreditLines`, `activeYearsLabel`), `TitleFacts` on the detail projection, show credits parsed
    into `WorkspaceShow.credits`, new `ContentEntry` fields (`airDate`, `overviewSource`, `country`,
    `activeBeginYear/EndYear`, `label`, `albumType`), Like/Dislike removed from `entry-actions.ts` and
    `detail-actions.ts`.
  - server: `catalog/detail.go` `titleFacts()` → `facts` on the detail response.
  - web: `Detail.tsx` (facts line, rating badges, credit lines, Watchlist · Favorite · Watched · ⋯,
    `PlaybackChoices` in the hero, cast capped at 12 with a "See all" dialog, related rows filtered,
    Details + Files section), `PlaybackChoices.tsx`, `player/pending-choice.ts` consumed in
    `Player.tsx`, `Show.tsx` (credit lines, people shelves, list-first episodes with a grid toggle
    stored in `localStorage` `portico.episodes.layout`, season ⋯ menu, Details), `EpisodeRow` in
    `EpisodeCard.tsx`, `EpisodePanel.tsx` (air date, choices), `library/Entity.tsx` (m:ss track times,
    Shuffle first, no "watched" on music, "finished" for audiobooks, artist country/years, Details).

Nothing has been done in `apps/portico-react-native` yet. All Apple work is open.

## 5. How to build it (shape and style)

This is how the finished passes were built; keep to it so the result reads as one hand.

- **Model in client-core, rendering per client.** Every decision about *what* appears (which filters are
  promoted, which rows a settings page has, how credits split, which related rows survive) is a pure
  function in `packages/client-core/src/presentation/`, exported from `presentation/index.ts`, with no
  rendering. Web, iPhone and tvOS each draw it with their own controls. `browse-toolbar.ts` and
  `title-page.ts` are the pattern. If you find yourself writing the same rule in a web screen and an
  Apple screen, move it to client-core.
- **The registry and the server contract are vocabulary, not layout.** Never render a registry or a
  capability list in its own order with fallback labels. Author the structure; a key no page references
  does not appear. Every row has authored copy and a unit, or it does not ship.
- **No dead UI.** A control exists only if the server can honour it. If a planned control has no server
  support, either add the server support (preferred when small and clearly right) or leave it out and
  add a line to `portico-internal/Backlog.md`. Never a disabled row explaining what is missing.
- **Strings** go in the catalogue (`title.*`, `web.*`, `entry.*` …), sentence case, no key fallbacks, no
  hardcoded English in components. Run `npm run lint:strings` at pass end.
- **Code style:** match the file you are in. The web screens are dense single-line JSX with short
  comments that state the rule and its source ("Spec — Title Pages §2", "Justin, 2 Oct 2026"). Styles in
  the screen's `.module.css` using tokens (`--page-gutter`, `--surface-raised`, `--text-secondary`,
  `--radius-control`, `--line-focus`, `--accent`); avoid new inline `style={{…}}`. Any component used
  as a Radix `asChild` trigger must forward its ref and spread props.
- **One component per concept across pages.** `TitleHero` is the hero for every title; the browse bar is
  the toolbar for every grid; settings rows are one row component. Extend the shared piece (as
  `TitleHero` gained `choices`) rather than fork it.
- **State that belongs in the URL goes in the URL** (season, filters, sort, search text, settings page).
  Device preferences go in `localStorage` under `portico.*`. Profile preferences go through the
  preference registry.
- **Focus and keyboard are part of done** on web (visible focus ring, Escape closes, menus are Radix).
  On tvOS every screen must be fully reachable with the remote and never require text entry for filters.

## 6. Remaining work, pass by pass

### Pass 4 — title pages (finish)

1. **Check the web build** at http://127.0.0.1:5173 (rebuild the server first; `facts` is new): a movie
   (Dune in Movies), a show (Severance), an episode panel, an artist, an album, an audiobook. Confirm:
   facts line and badges, credit lines, the three dropdowns appear only when there is a real choice and
   the chosen audio/subtitle is what plays, cast "See all", related rows (none under three titles, no
   duplicates), Details/Files, episode list rows (still plays, copy opens the panel, hover actions,
   watched check, progress, air date), layout toggle, season ⋯ menu. Fix what is off.
2. **Show-level Watchlist and Favorite.** The show hero has Watched and ⋯ only, because the container
   personal-state route (see `web/src/screens/shared/container-watched.ts` and its server handler)
   carries watched only. Extend that route to carry `watchlisted` and `favorite` for shows (and albums,
   artists, books for Favorite), then make the show hero row identical to the movie's:
   Play/Resume · Watchlist · Favorite (heart) · Watched · ⋯. Feed both into the recommender the same way
   an item's are (`compactcatalog/rec_profile.go`: favorite +2, watchlist +0.5).
3. **Remove Like/Dislike on the server too.** Clients no longer send it. Drop the `reaction` action from
   the personal-state mutation and its storage, and the like/dislike terms from `rec_profile.go`.
   **No migration:** there is no data worth keeping, so drop the column/rows outright (Justin, 2 Oct).
   Remove any remaining UI mention (web grep `reaction`, `'like'`, `dislike`; Apple `EntryMenu.tsx`,
   `Detail.tsx`).
4. **Version line.** If the old "SD" chip or the thin media-information block is still on the movie
   page, remove it; the Version dropdown and the Files list replace it. Files should show, per source:
   path (owners only), size, container, video codec/resolution/HDR, each audio track (language, codec,
   layout), each subtitle track, bitrate. Add what is missing to the server's source projection.
5. Catalogue the hardcoded English left in detail code (`kindLabel`, `qualityLabel` "SD", season/episode
   strings).
6. Episode placeholder (no still) tinted with the show's colour, per spec. Album artist link readable
   on the blurred cover (a `TitleHero` context-link style, not a one-off).
7. **Go further on episodes if it clearly helps** (Justin invited ambition): a "next up" row pinned at
   the top of the season when it is not the first episode; season progress ("6 of 10 watched") beside
   the switcher; keyboard up/down through rows with Enter to play. Keep it one list component.
8. Update `Spec — Title Pages` and `Spec — Page Content by Media Type` in portico-internal to match what
   was built (list-first episodes on web, no Like/Dislike, pick before play, action row).

### Pass 5 — one Settings screen (personal pages)

**Model:** `packages/client-core/src/presentation/settings-structure.ts`, exporting
`settingsStructure(context) → {headings: [{id: 'account'|'server', pages: [{id, title, icon, sections:
[{id, title?, rows: SettingsRow[]}]}]}]}` where `context` is `{owner, platform: 'web'|'phone'|'tv',
capabilities}`. A `SettingsRow` is one of: `{kind: 'preference', key, label, help?, unit?, scope:
'profile'|'device', control}` referencing a registry key; `{kind: 'composite', id, …}` for a row that
reads and writes several keys; `{kind: 'action', id, …}` (sign out, reset recommendations, Link a TV);
`{kind: 'custom', id}` for a panel a client draws (profiles, devices, subtitle preview). The registry
stays the storage and validation contract (`operations/preferences.go`); the structure is authored.

**Account pages (in this order):**

| Page | Rows |
|---|---|
| Profile & account | name, avatar, PIN, password, two-step, devices, sign out. Direct-sign-in servers must still manage profiles, sessions and MFA here. Switch profile / Switch server stay in the profile menu |
| Playback | auto-play next episode + countdown; "Still watching?" after N **episodes**; skip intros / credits / recaps; skip back / forward amounts (seconds); default speed for video and for audiobooks (no music speed); watched threshold (percent, under Advanced) |
| Audio & subtitles | preferred audio languages and preferred subtitle languages as **ordered pickers** (never comma-separated text); subtitles Off / Forced only / Always; subtitle appearance (size, background) with a live preview |
| Quality & data | one "limit" control per network — On this network, Away from home, On cellular (phones only): Automatic / Original / 4K / 1080p / 720p / 480p; **Playback method** as one choice (Automatic (recommended) / Always play the original / Always convert) that writes a consistent combination of the three tri-state policies; HDR Automatic / Off as its own row |
| Music | volume leveling, crossfade, gapless, show lyrics |
| Appearance | card size Small / Medium / Large, backdrops, reduce motion, default library view — each labelled "This device" |
| Language & region | language, time zone, clock |
| Privacy | pause history, show my activity, remember searches, reset recommendations |
| This device | device name, Link a TV, downloads storage (phones) |
| About | versions, legal, dataset credit |

**Rules:** per-device vs per-profile is a label on the row, not a page eyebrow. Personal rows save on
change with a brief confirmation. The composite rows hide the registry's internals: the three policy
keys and the per-network mode/ceiling pairs stay in the registry for API clients. Notifications,
Downloads and Watch Together are not settings pages (bell, profile menu / phone tab, title + player).
Sleep-timer-style rows with bare numbers get units or proper choices.

**Layouts:**
- **Web / iPad:** `/settings/$section`. Inside the content area (rail untouched): a left column listing
  pages under the **Account** and **Server** headings, a right pane with the page. A search field above
  the list filters rows across all pages and jumps to the match. `/settings` opens the first page.
  `/server/$section` and `/server/$section/$id` redirect to the matching `/settings/…` page so old
  links work; "Manage server" disappears from every menu.
- **iPhone:** a grouped list of pages (two sections, Account and Server) that pushes each page. The
  Server section lists all ten server pages for owners (see pass 6).
- **tvOS:** left column of pages, rows on the right, label and value within one glance (not 1,700 px
  rows). Nothing more than a few presses away. Under Server, owners get the Dashboard only.

Delete the registry-rendered Preferences page on all three clients once the new pages cover every key
worth showing.

### Pass 6 — the Server heading (console regrouped)

Fifteen sections (`web/src/screens/server/Server.tsx`) become ten pages under the **Server** heading of
the same Settings screen. Move the existing panels; do not rewrite working forms.

| New page | Built from | Notes |
|---|---|---|
| Dashboard | Overview, Activity (watching now) | Now playing first, then alerts, then one row of health tiles (CPU, memory, GPU, network, disk) with **one refresh for the page**, background activity with progress, viewing statistics below. No per-card "Updated just now · Refresh" |
| Libraries | Libraries, LibrarySettings, MetadataSource | List with count, size, last scan, status; a status of warnings/degraded opens the warnings. A library is a **page with tabs** — General (name, type, folders, visibility) · Metadata (source, language, ordering) · Scanning (when, what to analyse) · Advanced (deleting, trash) — not a modal. Add reorder and a per-library scan schedule (server support if missing) |
| Live TV & Channels | LiveTV, DVRSettings, Channels, ChannelBuilder, LiveSourceExtras | Sub-pages: Sources · Recording (who may record, rules, defaults, space) · Library Channels |
| People | People, Access | One list of accounts; an account opens to role, libraries, limits (streams, quality, recording), profiles, devices. Sub-pages: Invitations · API keys. Devices appear in one place per account |
| Playback | Transcoding + the streaming limits now in Network | Transcoding (allow, hardware acceleration, encoder, tone mapping, temp folder, concurrent limits); streaming limits (remote quality cap, upload speed, end paused streams); subtitle and audio defaults. Expose every setting the server already has; add small missing ones; backlog the rest |
| Remote access | Network | A status sentence first ("Reachable at … · HTTPS on"), then Portico Account connection, certificate, who may sign in from outside, home networks, extra addresses, advanced. Free tier = port forward or no remote; no hole punching |
| Storage & backups | Storage (remote sources), Maintenance (disk usage, retention), Backups | Disk usage with clean-up, **retention in one place**, remote sources, backups and restore, deleted titles |
| Schedule | Schedules, MaintenanceWindows | The windows editor and **every** scheduled job including backups, with last and next run |
| Troubleshooting | Logs, Diagnostics, Feedback, StatePermissions | Logs with dates on every line and correct levels; capabilities; support export; viewer feedback. One "Record more detail for 30 minutes" control in place of the three |
| General | General, AlertThresholds, updates from Maintenance | Server name, version and updates (one place), DLNA (off by default; exposes everything when on), alert thresholds |

**Rules:** one save model for server pages — **one sticky Save / Discard bar per page** covering every
dirty form on it (server settings are consequential, so explicit save; personal settings save on
change). One byte-unit formatter everywhere (`formatBytes` from client-core). Each duplicated concept in
the review's §5 table gets exactly one home. Status and settings are separated: status at the top of a
page, forms below.

**Server management on every phone (Justin, 2 Oct — he sees this as a competitive advantage):** all ten
Server pages are available on web, mobile web **and the iPhone app**, complete, not a subset. tvOS
shows the **Dashboard only** under the Server heading (who is watching, alerts, health, scan now).
There is no "manage on the web" row anywhere on phones.

How to build that without writing every form twice:

- The Server pages are part of the same authored `settingsStructure` as the personal pages. Each page
  lists sections and rows; a plain setting is a `{kind: 'server-setting', …}` row (label, help, unit,
  control: switch / choice / number / text / duration / bytes) bound to a read/write pair on a
  client-core service. Web and iPhone both render these rows with their generic row component, so the
  bulk of Playback, Remote access, General, Storage & backups retention, Recording defaults and the
  library tabs is written once.
- All reads, writes, validation and dirty tracking live in client-core services (most exist already —
  e.g. `library-management.ts`; move whatever logic still sits inside `web/src/screens/server/*.tsx`
  down into client-core as you regroup each page). A page exposes one `dirty / save() / discard()`
  so the single Save bar works the same on both clients.
- What cannot be a generic row is a `{kind: 'custom', id}` panel each client draws natively: Dashboard
  tiles and now-playing, the library list and folder picker, the accounts list and account page,
  invitations, API keys, the maintenance-windows editor, the schedule list, the log viewer, live-TV
  sources, the channel builder, backups and restore. Build each on iPhone as a proper iOS screen
  (grouped lists, pushed detail pages, sheets for create/edit, swipe actions for delete), not a
  shrunken copy of the web layout.
- Mobile web: the same pages, responsive — the page list becomes the first screen and each page pushes,
  as on iPhone; tables collapse to rows; the Save bar sits above the tab bar.
- Destructive actions (delete library, restore a backup, remove an account) confirm the same way on
  both clients, with the same wording from the catalogue.
- Order of work on iPhone: Dashboard → Libraries (incl. scan now and warnings) → People → Playback →
  Remote access → Storage & backups → Schedule → Live TV & Channels → Troubleshooting → General.

### Pass 7 — library types

- **Library tabs.** Discover is first and the default on every library. TV/Anime: Discover · Shows ·
  Recently aired · Collections. Movies: Discover · Movies · Collections. Music: Discover · Artists ·
  Albums · Songs · Genres · Playlists. Audiobooks: Discover · Books · Authors · Series. No Categories
  tab (genres, decades, studios are filters; Music's Genres tab is genres only, with artwork).
- **TV and Anime.** "Recently aired" = episodes by air date, newest first (windowed, server-sorted).
  Per-show settings from `Spec — Title Pages §4`: hide seasons, absolute numbering, ranges, newest
  first; stored per show on the server, edited from the show's ⋯ menu by owners. With absolute
  numbering on, episode rows and codes read "Episode 1043", never "Season 1, Episode 1043". Original
  title is a subtitle under the title, not a fact. Do not print "Anime" as a network. Specials last.
- **Music.** Songs is a **table** (title, artist, album, duration, plays, date added; sortable headers
  that drive the server sort), also for song search results. Album cards always show the artist.
  Artist page sections by release type (Albums, Singles & EPs, Compilations, Appears on) with Popular
  tracks first. Owner-only **Unmatched** view for untagged files instead of mixing duplicate albums and
  "Unknown artist" into the grids. **Playing a track starts in the mini player** and leaves the user on
  the page; the full Now Playing opens on click. Mini player gains previous, a scrubber, shuffle,
  repeat. The playing track is marked in track lists. Now Playing: centred cover, lyrics beside it,
  title once, no 10/30-second skip or Speed for music (keep both for audiobooks).
- **Audiobooks.** Square covers: change `shapeFor` in `presentation/card-shape.ts` for `book`,
  `audiobook_file`, `book_series`, `chapter` (and the hero), then check every row that mixes kinds.
  Cards show the author; titles clamp to two lines. "Finished" / "Not started" wording throughout.
- **List view is a table** on web for every library: thumbnail, title, year, runtime, rating,
  resolution, date added, with sortable column headers (same server sorts as the Sort control).
- **Letter rail** only for title sort.
- **Search.** Field and type chips sticky; rows filled to the width; year on movie and show results;
  section order equals chip order; "Audiobooks" not "Books"; a **Top result** block when one title
  matches strongly; query in the URL; stills in the Episodes section.
- **Home.** Continue Watching first; its cards say "S1 E3 · 24 min left". One "Recently added" row
  directly after it by default, then the recommendation rows (order remains user-editable in Customize
  Home; add drag to reorder there). One name per recommendation idea — no "Recommended for you" /
  "Picks for you" / "New for you" overlap. Log this default-order change for Justin to confirm.
- **Card marks** on all three clients: remove every unwatched dot; watched check; progress bar.

### Pass 8 — platform polish

- **Web:** title cards are anchors (middle-click, open in new tab, copy link) while keeping P/M keys;
  one page-header component so titles sit at one height; card size Small/Medium/Large in the grid
  toolbar and in Appearance; mobile web tab bar opaque enough that headings do not show through.
- **Player (layout fixed, Sleep stays):** a playback information overlay under More (direct play or
  converting and why, video/audio codec, resolution, bitrate, dropped frames, buffer); Play on / cast
  and Watch Together entries in the web video player; "Search for subtitles" in the subtitle menu when
  the server can; the page behind the player is `inert`; tvOS titles read "Severance · S1 E2" and never
  show shuffle/repeat for video.
- **iPhone:** a real navigation bar on every pushed screen (appears on scroll, Back + title, content
  never under the status bar or Dynamic Island); Search button and bell in the standard top bar of
  every tab; Home backdrop scrolls away; show page action row matches web; filter UI as an iOS grouped
  list with the current value on the right; settings rows without duplicated host text; US spelling
  ("Favorites"); the same empty-state copy as web; compare the accent token against web.
- **tvOS:** rail focus uses the same bright ring as content; the collapsed rail shows icons with the
  selected one marked (B7); Jump-to no longer overlaps (B8); the browse header takes at most two rows
  and tabs are one Up from the grid; filters are a right-hand side panel with two levels (field →
  values), no text entry; cast circles show initials when there is no portrait; Home refreshes when the
  app or screen regains focus.

### Apple ports of passes 1–7 (do alongside or after each pass)

After any change under `apps/portico-server/packages/`, run `node scripts/shared-packages.mjs --write`
in portico-react-native. Then, in `apps/apple/src`:

- `screens/BrowseFilters.tsx`, `Library.tsx`, `Saved.tsx`: render `toolbarModel` — Sort as one control,
  promoted fields, quick toggles, "+ Filter" grouped list with a summary on each row, applied chips
  with Clear all, decade presets inside Year, ranges as pickers. Remove the 27-accordion sheet.
- `screens/Detail.tsx`, `Show.tsx`, `Entity.tsx`, `screens/title/*`: the same facts line, credit lines,
  action row (Watchlist · Favorite · Watched · ⋯), pick-before-play row, cast cap with See all, related
  rows through `relatedRowsForPage`, Details section; episode rows (still, number + title, air date ·
  runtime, synopsis, progress, check) on iPhone; on tvOS a focusable episode list with the still and
  synopsis of the focused episode shown large beside it. Remove Like/Dislike from `EntryMenu`.
- `screens/Settings.tsx`, `SettingsPages.tsx`, `screens/settings/*`: render `settingsStructure` —
  personal pages on both, **all ten Server pages on iPhone**, Dashboard only on tvOS (pass 6).
- `shell/*`: phone tabs and top bar, tvOS rail (pass 8).

## 7. Order and checkpoints

Suggested: finish pass 4 (web, then Apple for passes 3–4) → pass 5 → pass 6 → pass 7 → pass 8, porting to
Apple as each model lands. At each pass end: `npx tsc --noEmit` and `npm run lint:strings` in `web/`,
the Apple type-check, a short functional check of the changed screens, commit in each repo, append to
the log, and deploy the commit to the demo. Then continue without waiting.

## 8. At the very end (only then)

Build final web and Apple builds; run the full unit and end-to-end suites once (Go on the runners);
fix what they flag, rerunning only what failed; update the affected specs in
`portico-internal/Project Architecture/`; move anything deliberately left out to `Backlog.md`; deploy.

## 9. The log (required — this is what gets reviewed)

Keep `portico-internal/Project Architecture/Audits/2026-10-02-frontend-rework-log.md`, one section per
pass and platform:

- what was built, with commit hashes and the main files;
- every decision you made where this handoff was silent or ambiguous, and why;
- every server/API/schema change;
- anything from this handoff or the review you did **not** do, and why;
- the functional checks you ran and what they showed;
- open questions for Justin.

Expect minor revisions on review, not rework: when unsure, choose the option closest to §5's shape and
write it in the log rather than stopping to ask.
