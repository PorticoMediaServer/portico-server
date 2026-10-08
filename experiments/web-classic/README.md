# Portico Web Classic

A from-scratch web frontend for Portico, the standard way, done carefully: Plex Web, Jellyfin and
Emby at their best. It runs on fixture data behind a service interface, and covers every page,
dialog and action of the current web UI (`apps/portico-server/web/src`), including all of server
administration. [FEATURES.md](FEATURES.md) maps each current-web feature to where it lives here.

It is the web companion of the Portico Classic app (`experiments/classic`): the same household,
the same licensed artwork, the same calm and dense design language, rethought for a browser with a
mouse and a keyboard.

## Running it

```
npm install
npm run dev        # http://127.0.0.1:5401
npm run build      # type-check and build into dist/
npm run preview    # serve the build at http://127.0.0.1:5411
npm run shots      # screenshot every screen at 4 sizes into shots/ (needs `npm run preview` running)
```

- **Screen index:** http://127.0.0.1:5411/dev/screens (or :5401 in dev) lists every page and state
  (165 of them), including dialogs, player states and banners, which open through `?demo=`.
- The app opens signed in as **Justin** (owner) on the server **Lighthouse**. Sign out from the
  profile menu to see the signed-out flow. Profiles Maya and Nora have the PIN **1234**; Theo is a
  kids profile. Studio Annex is the server that can't be reached.
- **Latency** is simulated so loading states are real. `?latency=0` turns it off for the tab,
  `?latency=4` slows it down, `?latency=1` restores it.
- **Keyboard:** press `?` anywhere for the shortcuts.

## Design idea

Plex-like structure, finished to a higher standard than Plex, Jellyfin or Emby manage.

- **A persistent sidebar** lists the libraries, Live TV sources and Recordings, with Home, Saved,
  Downloads and Watch Together above and Manage server, Notifications, Settings and the profile at
  the foot. It collapses to icons (below 1280 px by default; expanding pushes the page, it never
  covers it). Phones get a top bar and a five-tab bottom bar with a Libraries sheet.
- **Rows and poster grids** carry everything. Cards follow Portico's artwork rules: posters for
  film, TV, anime and audiobooks (episodes show their show's poster), square for music, round for
  people and artists, landscape only for home videos, channels and inside show pages. Watched ticks,
  unwatched counts and progress sit on the artwork. Hover reveals Play and a ⋯ menu; right-click
  gives the same menu.
- **Title pages put Play first.** The backdrop falls into the canvas behind a large poster, then
  the facts, the play and personal buttons, the overview, credits, and version, audio and subtitle
  choices. Cast, extras and related titles follow as rows.
- **Portico's blue glow** falls from the top left onto the navy canvas, breathes very slowly, and
  tints toward the featured title's colour on Home and title pages.
- **Keyboard first-class:** each row and grid is a single Tab stop with arrow-key movement (Up and
  Down move by a row in grids), Enter opens, P plays, X selects, the context-menu key opens actions.
  `/` searches, `g` then a letter jumps (h Home, s Saved, l Live TV, t Settings, m Manage server),
  and the guide moves by program with the arrows. Focus is always visible and returns to where it
  came from when a dialog closes.
- Type is Manrope throughout, self-hosted as one variable font. The brand marks are the real
  Play-P symbol and wordmark (`public/brand`), never recoloured.

## Architecture

```
index.html, src/main.tsx      entry; dev flags (latency) read once
src/styles                    tokens.css (every colour, size and duration), base.css (reset, glow, focus, text roles)
src/ui                        design system: Button, Icon, Bits (artwork, avatar, badge, chips, states),
                              Form (fields, select, switch, segmented, slider, radio cards, PIN),
                              Overlay (dialog/sheet/panel, menus, context menu, tooltip, popover, toasts),
                              Card, Shelf and CardGrid, Windowed (virtualised grid and list), Page, Tabs, roving focus
src/shell                     Shell (sidebar, phone bars), switchers, connection banner, selection bar, shortcuts, demo runner
src/screens/*                 one folder per area: auth, home, library, detail, search, saved, live, together,
                              downloads, settings, account, server, dev (screen index)
src/player                    PlayerProvider (simulated clock and queue), VideoPlayer, Timeline, Panels, AudioPlayer
src/app                       router, session, prefs, actions (entry menu and dialogs), selection, links, formatting
src/data                      types, the PorticoClient and AdminApi interfaces, DataProvider, fixtures/
scripts/art.py                builds public/art (WebP) from experiments/classic/assets
scripts/shots.mjs             headless screenshots of every screen; fails on console errors
```

- **Routing:** TanStack Router with code-split routes. URLs match the current web app
  (`/library/:id?view=…&sort=…&filters=…`, `/media/:id`, `/show/:id/episode/:id`, `/channels/:source`,
  `/server/:section/:id`, …), so every pivot, filter and panel is linkable and survives reload.
- **Design tokens:** all colours, spacing, radii, card sizes and motion are CSS custom properties in
  `tokens.css`, re-set per breakpoint (poster width, gutter, type sizes) and by the card-size
  preference. Components use CSS modules that only reference tokens. No CSS framework.
- **Components:** Radix UI primitives for dialogs, menus, context menus, tooltips, popovers,
  switches, sliders, radio groups, toggle groups and tabs; everything else is local.
- **Data:** screens only talk to `PorticoClient` (`src/data/services.ts`) and `AdminApi`
  (`src/data/adminApi.ts`) through `useQuery` and topics. The fixture implementation is a separate
  chunk loaded behind the start-up screen. Browse fetches pages by offset and the grid renders only
  the rows in view, so a library of millions costs the same as one of hundreds.
- **Admin:** each Manage server section is a page of typed blocks (stats, settings, facts, lists,
  tables, charts, meters, timelines, actions with forms or confirmations), rendered by
  `screens/server/Blocks.tsx`. Settings save as they change.
- **Playback** is simulated: a still with slow motion and a real clock. Every control, panel, prompt
  and state is real, and progress is written back (Continue Watching, watched).

## Swapping in real services

1. Implement `PorticoClient` (`src/data/services.ts`) against the Portico server API. The fixture
   client (`src/data/fixtures/client.ts`) documents each method's behaviour: what it returns, what it
   rejects with, and which topics it emits after a change. The shapes follow
   `apps/portico-server/packages/client-core/src` loosely; the projection is the client's job.
2. Implement `AdminApi`: map the server's admin endpoints onto the block types in
   `src/data/adminTypes.ts`, or add block types and render them in `Blocks.tsx`.
3. Change `createServices()` in `src/data/DataProvider.tsx`. Nothing else imports fixtures.
4. Artwork: fixtures return local URLs (`/art/<kind>/<id>.webp`); a server returns its own. `Entry.tint`
   is the artwork's dominant colour (placeholders and the glow); it's optional.
5. Playback: put a real engine (hls.js or native) behind `usePlayer()` in
   `src/player/PlayerProvider.tsx`, keeping its actions (play, seek, set options, queue, cast).

## Decisions

- **Session:** the browser remembers the server and profile, and a reload resumes without the PIN,
  as a session token would. First visit opens as Justin so review starts in a populated library.
- **Search in the URL** (`/search?q=…&type=…`): the current web keeps it out of the URL; here results
  survive reload and Back, which suits a web page.
- **Episodes open as a panel over their show** (the current web's behaviour), at their own URL.
- **Settings sections:** Account, Profiles, Security and Devices are separate sections beside
  Preferences, Notifications, Servers and About, with a section list on wide screens. The Portico
  Account workspace (`/account`) holds account-wide things (sessions, email, Apple and Google,
  servers and invitations, export and delete).
- **Manage server** keeps the classic app's 23 sections (the current web's 15 plus Now Playing,
  Diagnostics, Alerts, Library settings, Metadata, DVR, Channel Builder and Backups as their own pages).
- **Live TV** is one sidebar group: every channel, each source by name, Library Channels, and
  Recordings, as in the current web.
- **Artwork** comes from the Classic app's licensed set (Wikimedia Commons public domain, Blender
  Foundation CC BY, Unsplash via picsum, and generated art), re-encoded as WebP by `scripts/art.py`.
- **QR codes** use the `qrcode` package (also used by the current web) for the sign-in-with-a-code page.
- **Home videos** get a landscape hero on their page (the one place a landscape title page makes sense);
  every other title keeps its poster, per Portico's artwork rules.
- **Touch:** cards drop the hover Play and ⋯ buttons on touch screens; a long press opens the same
  actions menu, and every title page has its own ⋯ button.
- **Review states** (`?demo=…`) exist only so the screen index can open dialogs, panels and playback
  directly; `src/app/demo.tsx` and `src/shell/DemoRunner.tsx` can be removed without touching screens.
- **US English** throughout.

## Status and next steps

**State:** feature-complete against FEATURES.md; `npm run build` passes with no type errors; the last
`npm run shots` run passed for all 165 screens at 1440×900, 1920×1080, 1024×768 and 390×844 with no
console errors and no sideways scrolling (the script fails on either). The visual pass covered every
page at all four sizes; what it changed is listed below. Servers are stopped; work is committed.

### Refinement done

- **Phones:** found and fixed sideways overflow on title pages (a centred facts grid) and on the Account
  page; the settings and admin section lists collapse to the page or a picker; recordings rows, playlist
  headers and admin stats re-laid for narrow screens; the player letterboxes on portrait screens; tabs keep
  the current one in view.
- **Artwork:** letterboxed backdrops are cropped when the WebP set is built (one had a captured player bar);
  hero and title backdrops blend into the glow with masks instead of fading to a flat colour; the player
  uses the sharpest picture available; category tiles and Shared badges keep contrast on busy images.
- **Focus and keyboard:** dialogs and panels focus themselves or their first field, not the close button;
  rows and grids keep one tab stop as paged cards arrive; Shift+F10 and the context-menu key open a card's
  menu; the guide keeps program titles in view when a program started before the visible window.
- **Copy and data:** readable dates (life dates, photo dates), public-domain films show Rights rather than a
  fake studio, log lines come from the right part of the server, songs list their album and m:ss lengths,
  home video rows no longer repeat each other, the current device has a real name.
- **Accessibility:** buttons in profile, photo, folder and collection grids keep their button role (they
  had been given list roles); the shell never renders without a session while signing out; Settings
  sections have their own headings on wide screens.
- **Structure:** Manage server collapses the sidebar to icons below 1600 px, since it has its own section
  list; the Settings overview shows state at a glance on wide screens instead of repeating the list;
  sticky toolbars get their backdrop only once stuck; URL search params are plain strings.

### Next steps

1. Playback is simulated. Wire `usePlayer()` to hls.js or native playback and the server's delivery API.
2. The guide renders every channel row (its programs are windowed by time); a server with thousands of
   channels would want the rows virtualised like the browse grid.
3. Real-device checks: touch long-press menus on iOS Safari, and screen-reader passes (VoiceOver, NVDA).
4. The Photos viewer could preload neighbours and support pinch zoom on touch.
5. The sidebar scrolls at 900 px tall once Live TV sources are listed; a server with many libraries may
   want the Live TV group collapsible.
6. Fixture data quirks remain in places (a kids profile's Continue Watching includes a PG action film;
   picsum photographs occasionally repeat between an episode still and a photo album).
