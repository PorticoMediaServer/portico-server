# Portico Web Cinema

A from-scratch web frontend for Portico, built as the *creative* interpretation: your own media
library presented like a premium streaming service in the browser. It is feature-complete against
the current web UI (`apps/portico-server/web/src`), runs entirely on fixture data, and keeps every
data access behind service interfaces so real Portico services can replace the fixtures.

Pairs with the Portico Cinema app (`experiments/cinema`), whose artwork and catalogue it reuses.

## Run it

```sh
npm install
npm run dev          # http://127.0.0.1:5402
npm run build        # type-checks (tsc) and builds to dist/
npm run preview      # serves the build at http://127.0.0.1:5412
npm run shots        # screenshots every screen at 1440×900, 1920×1080, 1024×768, 390×844 into shots/
```

- **Screen index:** <http://127.0.0.1:5402/dev/screens> (or 5412 on the preview) links every
  screen and state — 170-odd entries, from sign-in to the Channel Builder, including player,
  dialog and problem states.
- **Simulated latency** is on by default so skeletons and transitions are real. `?latency=0` turns
  it off, `?latency=3` slows it down (it sticks for the tab); Settings › Preferences › Demo data
  has the same switch.
- **Review switches** (this visit only): `?as=member|admin|owner`, `?profile=pr-kids`,
  `?connection=reconnecting|offline`, `?demo=<state>` (see `src/app/demo.tsx`).
- `npm run shots -- movie home` shoots only matching screen ids; `--sizes=390x844` limits sizes;
  `BASE=http://127.0.0.1:5402` shoots a running dev server. It fails if any page logs a console
  error.

## The design idea

**The front of a cinema, lit by what you’re about to watch.** Portico’s navy canvas and its blue
projection glow, falling from the top left, are the house lights. A second, softer beam takes on
the colour of whatever artwork you hover, focus or open, so moving through the library quietly
relights the room — while it always stays recognisably Portico blue.

- **Full-bleed art that runs under the rail.** Home, libraries and every title page open on
  artwork that extends behind the translucent, blurred left rail, with the identity block anchored
  low. Rows scroll up over the marquee, as on the paid services.
- **Hover previews that grow out of the card.** Rest the pointer on a poster and it expands into a
  preview with the backdrop, the title set large, quick actions (play, watchlist, watched, like,
  more) and the key facts. Keyboard focus never pops previews; it lifts the card with a bright ring
  and a glow in the art’s colour, so navigation stays predictable.
- **Editorial typography.** Two families only, both sans: *Archivo*, set expanded and heavy
  (width 118%, weight 800) for marquee titles and headings, and *Manrope* (Portico Sans) for
  everything else. Row titles read like an editor wrote them: “Because you watched **Soft
  Landing**”, “Starring **Nils Berg**”, with the subject in Portico blue; Popular is a ranked
  Top 10; a collection gets an editorial feature between rows.
- **A premium player.** The fixture build has no video, so the picture is the title’s own frames
  crossfading with a slow drift; around it is the real thing — a timeline with chapter frames,
  tinted intro and credits, Skip intro, a side panel for audio/subtitles/quality/speed/sleep/info/
  chapters/queue/Play on, a post-play countdown, Now Playing with synced lyrics, a live player
  with its mini guide.
- **The real Portico brand**: the official wordmark and Play-P symbol (mono white, unmodified),
  and the favicon and app icon from the brand set.

## Architecture

```
src/
  main.tsx               mounts the router; imports the stylesheets
  app/
    router.tsx           TanStack Router route tree (code-based), lazy screen chunks, the services instance
    session.tsx          who is signed in, to which server, as which profile; role; connection
    Gate.tsx             keeps the app frame behind sign-in, a reachable server and a profile
    player.ts            the playback store (queue, position ticker, tracks, sleep, Play on, problems)
    prefs.ts             this browser’s display preferences (rail, motion, text size, card size…)
    tint.ts              the ambient glow’s colour (a registered CSS custom property that transitions)
    selection.ts, together.ts, demo.tsx, Reminders.tsx
  data/
    types.ts, services.ts, admin.ts   the domain model and the service interfaces every screen uses
    hooks.ts             useServices, useQuery (cached, re-runs after mutations), useAction
    art.ts               artwork keys → URLs (the only place that knows where images live)
    latency.ts           simulated network delay (?latency=)
    fixture/             the fixture backend: db.ts builds the catalogue, backend.ts implements
                         every service, adminData.ts the server console
    fixtures/            content.json (catalogue) and art.json (image keys and dominant tints)
  ui/                    design-system components: Button, Icon, Overlay (dialog, sheet, menu,
                         popover, tooltip, confirm, prompt), Controls (fields, select, switch,
                         slider, segmented, chips, choice cards, stepper), Feedback (skeletons,
                         states, notices, badges, progress, toasts), Media (artwork, avatar, brand), Layout
  shell/                 the rail, phone top bar and tabs, profile menu, switchers, connection banner
  player/                VideoPlayer, Timeline, Panels, PostPlay, NowPlaying, MiniPlayer, GroupPill
  screens/               auth, home, library, detail, person, search, saved, live, together,
                         downloads, settings, account, server (23 console sections), shared, dev
  styles/                tokens.css (every colour, size, radius, shadow, duration), base.css,
                         ui.css, shell.css, cards.css, screens.css, player.css, admin.css
```

- **Routing.** TanStack Router with the current web’s URLs, so links and bookmarks carry over:
  `/`, `/home/rows/$rowId`, `/library/$libraryId` (view, sort, direction, filters and layout in the
  query), `/library/$libraryId/$view/$entityId`, `/media/$itemId`, `/show/$showId` and
  `/show/$showId/episode/$episodeId`, `/person/$personId`, `/search`, `/saved`,
  `/saved/$kind/$resourceId`, `/downloads`, `/together`, `/channels/$entry` (`/live` redirects),
  `/settings`, `/settings/$section`, `/account`, `/server/$section`, and the link routes `/device`,
  `/claim`, `/reset`, `/verify-email`, `/join`, `/together/join`, `/app/account-return`. Getting in
  lives under `/welcome/$screen` so each step is linkable for review. Screens are code-split.
- **Design tokens.** One token file (`styles/tokens.css`) and a small set of global stylesheets
  that only use those variables; no Tailwind, no CSS-in-JS. The ambient `--tint` is registered with
  `@property` so the glow crossfades.
- **Components.** Radix primitives (via `radix-ui`) for dialogs, menus, popovers, tooltips,
  switches, checkboxes and sliders — focus trapping, return focus, typeahead and Esc for free.
  Lucide for icons (ISC, tree-shaken).
- **Data layer.** Screens only call `useServices()`; nothing imports fixtures. Mutations bump a
  version so every `useQuery` refreshes. The browse grid asks for pages as they scroll into view
  and renders only the rows near the viewport (`WindowedGrid` + `useBrowse`), so the cost follows
  what is on screen, not the size of the library. The viewing profile’s limits (rating ceiling,
  hidden labels) are applied by the service, as the server does.

## Swapping in real services

1. Implement `Services` (`src/data/services.ts`) and `AdminService` (`src/data/admin.ts`) over
   Portico’s HTTP APIs; `apps/portico-server/packages/client-core` already has the clients and
   most shapes (the fixture types were modelled on them). Keep the method names and no screen
   changes.
2. Replace `artUrl()` in `src/data/art.ts` with the server’s image endpoint
   (`${server}/art/${key}?w=${width}`), and take tints from the server’s artwork metadata.
3. Construct the real services in `src/app/router.tsx` instead of `createFixtureServices()`, and
   call the `subscribe` listeners from server events so lists refresh.
4. Replace the ticker in `src/app/player.ts` with the real player engine (hls.js or the current
   `playback-v1` engine) feeding the same store fields; `VideoPlayer` renders a `<video>` where the
   frames are now.
5. Remove `src/app/demo.tsx`, `src/data/latency.ts` and `/dev/screens` (or keep the index behind a
   development flag).

## Fixture data and artwork

- The catalogue (`src/data/fixtures/content.json`) is the Portico Cinema app’s: 183 films (real
  public-domain films and Blender open movies with accurate credits, plus invented titles), 24 TV
  series and 12 anime (270 episodes), 24 artists and 60 albums with synced lyrics, 28 audiobooks,
  40 live channels with a rolling three-day guide, photo albums and home videos, four profiles (one
  kids, one with a PIN), three servers (one unreachable), and a fully populated server console.
- Artwork is the Cinema app’s licensed set (Wikimedia Commons public domain, Blender Foundation CC
  BY, Unsplash via picsum, generated typography), converted to WebP at display sizes by
  `npm run art` (`scripts/build-art.mjs`, needs `cwebp`): about 54 MB in `public/art`.
- Fonts are self-hosted WOFF2 (SIL OFL): Manrope and Archivo (width axis), Latin and Latin Extended.

## Decisions

- **Rail pushes, never covers.** Desktop: expanded with labels, collapsible to icons with tooltips
  (remembered). 768–1199 px: starts collapsed and expands on demand. Under 768 px: a top bar and a
  five-tab bottom bar (Home, Library picker, Channels, Saved, Search); the rail’s other entries live
  in the Library picker and the profile menu.
- **Artwork shapes follow Portico’s rules**: film, TV, anime and audiobooks (episodes included) are
  posters in rows and grids; landscape stills appear only inside show and episode pages (and for
  extras, chapters, home videos and photos); music is square; artists and people are circles. Hover
  previews show the backdrop because they are a preview panel, not a card.
- **Continue Watching uses posters** with a progress line and an S·E tag for episodes, per the
  shape rules, rather than the landscape tiles the streaming services use.
- **Server console sections.** The current web groups 15 nav sections and nests Library settings,
  DVR, Channel Builder, Diagnostics, Now Playing, Backups and Alerts inside them. Here all 23 are
  first-class pages in a stable section list (Console, Media, Live TV, Server), and settings forms
  use an explicit Save/Discard bar instead of saving on every keystroke.
- **Getting in has URLs** (`/welcome/<screen>`) so every step can be reviewed and linked; in the
  real app the Gate chooses the step.
- **Search text stays out of the URL**, as in the current web; `?q=` only seeds it for links.
- **Player as an overlay store, not a route**, so music and minimized video keep playing while you
  browse; `?demo=` opens any player state for review.
- **Explicit confirmation for destructive actions**, with type-to-confirm for deleting accounts,
  profiles and libraries; everything reversible gets an Undo toast instead.

## Status and next steps

- **Status:** feature-complete against the current web UI (see [`FEATURES.md`](FEATURES.md), every
  row ticked). `npm run build` passes with no type errors; `npm run shots` takes 692 screenshots (173
  screens and states × four sizes) with no console errors, and every size was reviewed and fixed.
  An interaction smoke pass (watchlist, playlists, player, episode panel, filters and saved views,
  bulk actions, Customize Home, search, the guide, console forms, sign-out and back in) passes.
- **Polish I’d still do:**
  - Real video in the player (hls.js) and real thumbnails in the timeline from seek-bar sprites.
  - A two-axis virtualized guide for thousands of channels (rows are windowed by page today only
    in Browse; the guide renders every channel row).
  - Shared-element transitions from a card to its title page (the View Transitions API).
  - Drag-and-drop reordering for playlists, the queue and Customize Home (buttons work today).
  - The Kids profile gets a simpler Home layout, not only filtered content.
  - The fixture keeps one set of watch state for every profile (limits are per profile); real
    services keep it per profile.
