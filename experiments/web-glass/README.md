# Portico — web

**Live preview:** http://127.0.0.1:5404/ · **Screen index (every screen and state):** http://127.0.0.1:5404/screens
**Production preview:** http://127.0.0.1:5414/ (after `npm run build && npm run preview`)

Portico is a self-hosted media server for a whole household: films, TV, anime, music, audiobooks, photos, home videos and live TV with a DVR, served from a box at home to every screen. This folder is its web client, running against a complete fixture household ("the Lindqvist-Okafor family" and their server, **Harbor**), so every screen is real and clickable without a backend.

```
npm install        # once
npm run dev        # → http://127.0.0.1:5404/
```

The first visit lands on the welcome screen. Sign in with any email and a password of six or more characters, then any six-digit code; or press **Continue with Passkey**. Maya's profile PIN is **1234**. To skip all that, open any page with `?as=maya` (or `daniel`, `noor`, `theo`).

---

## The design idea

**A cinema lobby made of glass.** Portico's canvas is a deep navy room lit by one blue light falling from the top left — the Portico glow — and everything you touch floats in front of it as liquid glass: translucent, slightly refractive, with a bright specular rim where the light catches it. The content (posters, stills, album art, your own photos) is always the brightest thing on screen; the chrome is lens, not frame. Consumer surfaces are calm and editorial, like Apple TV and Apple Music: big art, big type, one clear action. The **Server Console** is a different instrument entirely — dense tables, live charts, keyboard-first — the way Logic sits next to GarageBand: same materials, different job.

### Why it beats Plex

| Plex today | Portico |
| --- | --- |
| Your library shares Home with ad-supported streaming, “Discover” and upsells. | Portico shows **only what’s on your server**. No ads, no third-party catalogs, no Plex Pass tiers. |
| Continue Watching, Up Next and audiobooks live in different apps (Plex, Plexamp, Prologue). | **One household app**: films, shows, anime, music with synced lyrics, audiobooks with chapters/speed/sleep timer, photos, home videos and live TV share one queue, one search and one Continue shelf. |
| “Why is this transcoding?” is buried in a dashboard. | The player’s **stream details** (press `i`) and the console’s Now Playing say *exactly* why (“styled ASS subtitles aren’t supported by the client renderer”) and what the fix is. |
| A cloud account is required to sign in to your own server. | **Direct sign-in** on the server, passkeys and two-step built in; the cloud account is optional. |
| Admin UI is a web form sprawl with a sidebar. | A **pro console**: sortable dense tables with `j`/`k`, a live log with filters, live CPU/GPU/network charts, health checks that link to the fix, `⌘K` to any setting. |
| Anime is “TV with a genre”. | **Anime is a first-class library**: original titles, Japanese audio + English subtitles by default, simulcast badges (“New every Saturday”), AniDB/AniList agents. |
| Parental controls are a checkbox list. | **Per-profile rating ceilings for film and TV, library access, bedtime and daily limits**, a real Kids Home, and a “Ask a grown-up” wall instead of settings. |
| Casting and “watch together” are separate features in separate places. | **Play On** (Portico/AirPlay/Google Cast/DLNA) and **Watch Together** sit in the same glass controls in every player. |

## Navigation model

There is **no sidebar anywhere**.

- **Desktop and tablet:** a floating **glass capsule** at the top centre holds the six primary destinations (Home, Movies, TV Shows, Anime, Music, Live TV) and a **More** menu (Audiobooks, Photos, Home Videos, Watchlist, History, Downloads). A liquid “lens” glides under the active tab. To the right: **Search** (`⌘K` / `/`), **Play On**, **Notifications** and your **profile** (switch profile, settings, the Server Console, sign out). The capsule refracts what scrolls beneath it.
- **The command palette (`⌘K`)** is the power path: it searches the whole library as you type (titles, people, episodes, lyrics, channels, the live guide), jumps to any page or setting, and runs actions (“Scan all libraries”, “Reduce transparency”). `g` then a letter jumps anywhere (`g m` Movies, `g c` Console); `?` lists every shortcut.
- **Phones (390–430 pt):** an iOS-style floating **glass tab bar** — Home, Library, Music, Live — with **Search** as its own round button, both inside the home-indicator safe area and in thumb reach. Pages use **large titles** that hand off to a compact glass bar as you scroll; detail pages use floating glass back/more buttons over full-bleed art; menus become **action sheets** and dialogs become **bottom sheets** with a grabber and drag-to-dismiss; navigation plays iOS push/pop transitions. The mini player docks above the tab bar and expands into a full-screen Now Playing sheet.
- **The Server Console** has its own chrome: a glass bar with the server’s status, four groups (Monitor, Library, People, System), a row of numbered section tabs (`1`–`9` switch), `r` refreshes, `/` filters, `Esc` returns to Portico. On phones it becomes an inset-grouped list, and every table turns into stacked cards.

## Architecture

- **Vite + React 19 + TypeScript**, React Router 8, TanStack Query (data + loading/error states), TanStack Virtual (every long list), Zustand (session, player, UI), Lucide icons, Inter as the non-Apple fallback for SF Pro. No UI kit: the design system is ~2,500 lines of plain CSS with tokens (`src/styles/`).
- **Every screen is code-split**; the shell is ~150 kB gzipped plus the fixture catalog (~70 kB).
- **Glass** (`src/styles/glass.css`, `src/lib/refraction.ts`): tint + blur + saturation, a gradient specular rim drawn with a masked `::before`, and a pointer-following sheen. In Chromium, the three floating bars (top capsule, tab bar, mini player) also get **real edge refraction**: a displacement map computed for the element’s exact size and run as an SVG `backdrop-filter`. Blur is used only where content scrolls under chrome, sheets and menus; cards and rows use cheap translucent fills. `prefers-reduced-transparency` (or Settings → Appearance) swaps every glass surface for solid navy with the same depth; `prefers-reduced-motion` removes Ken Burns, parallax and transitions.
- **Scale:** grids, song lists, the log viewer and the guide render O(visible). `/movies?view=library&scale=250000` loads a 250,000-item library; headless Chromium scrolls it at 59 fps with ~80 cells mounted.

### The service layer, and how a real backend plugs in

All data goes through one typed contract, **`PorticoApi`** in `src/services/api.ts` (≈ 80 methods: browse, detail, search, personal state, playback, live TV, and the admin console). Two implementations:

- `src/services/fixture/` — the in-memory household. Watch state, watchlists, downloads, profiles and admin changes persist to `localStorage` like a server would. **Latency is simulated** (140–480 ms) so skeletons and spinners are real; add `?latency=fast|normal|slow|hang` or `?fail=all|<methodName>` to any URL to review loading and error states.
- `src/services/http.ts` — the same contract over REST (`/api/v3/...`, bearer token, `X-Portico-Profile`). Point the app at a server with:

  ```
  VITE_PORTICO_SERVER=https://harbor.example.com:32500 npm run dev
  ```

Screens only ever call hooks in `src/services/queries.ts`, so swapping backends changes no UI code.

### Fixture data and media (all freely licensed)

- **Films:** 40 fictional titles with original posters typeset from Unsplash photography (via `picsum.photos/seed/…`), plus the **Blender Studio open movies** (CC BY) and **public-domain classics** (Keaton, Chaplin, Lang, Méliès, *Charade*, *Detour*…) with their real posters and stills from Wikimedia Commons. Credits: `src/data/generated/commons-credits.json` and each title’s About section.
- **Playback is real:** *Coffee Run* (Blender Studio, CC BY) and the *Sintel* trailer are bundled as H.264, with scrub-preview sprites. Longer titles map their runtime onto these streams. Music plays synthesized ambient beds; audiobooks play narrated openings of their public-domain texts.
- **TV & anime:** 18 series, 196 episodes, with written episode synopses. **Music:** 12 artists, 18 albums, 162 songs, original synced lyrics. **Audiobooks:** 12 public-domain classics with real chapter lists. **Photos:** 101 photos in 8 trips/places with EXIF. **Live TV:** 14 channels, 30 hours of guide, recordings, series rules.
- Rebuild everything with `npm run art` (posters, logos, covers, stills, palettes; renders HTML templates in Playwright) and `node scripts/build-media.mjs` (video, sprites, audio). Outputs are committed, so neither is needed to run the app.

## Scripts

| | |
| --- | --- |
| `npm run dev` | Dev server on http://127.0.0.1:5404 |
| `npm run build` | Type-check and build to `dist/` |
| `npm run preview` | Serve the production build on http://127.0.0.1:5414 |
| `npm run shots` | Screenshot every screen in the index at 1440×900, 1920×1080, 390×844 and 430×932 into `shots/`; fails on console errors or sideways scrolling. `-- --only=console` filters. |
| `npm run smoke` | Playwright walk-through of the main flows (sign-in, PIN, search, playback, music, watchlist, DVR, console); fails on any console error |
| `npm run art` | Regenerate fixture artwork |

## A short guided tour

1. **Welcome** (`/welcome`) — the household’s own posters drift behind a single line of copy; the server on your network is already found.
2. **Who’s watching** — glossy avatars; Maya’s is PIN-locked (1234), Theo’s is a Kids profile.
3. **Home** — a cinematic billboard with title logos and a Ken Burns drift, then *Continue Watching* (films, episodes and home videos in one row), *New Episodes*, *On Live TV Now*, a *Top 10 in Your Household*, *Memories* from Photos. Switch to **Theo** to see the Kids Home.
4. **Meridian** (`/show/meridian`) — seasons as a segmented control, the next episode highlighted, skip-intro metadata, per-episode watched toggles.
5. **Play an episode** — glass transport, chapter ticks on the scrubber, frame previews on hover, *Skip Intro*, *Up Next* at the credits, audio/subtitle tracks, quality with an honest “converted on Harbor”, Watch Together, Play On, and `i` for stream details.
6. **Music → Northern Static** — press play: the mini player floats in; open it for full-screen Now Playing with **synced lyrics** and a drag-to-reorder queue.
7. **Live TV** (`/live`) — a real guide with a live “now” line; record an episode or a whole series; *Recordings* shows a partial recording and explains why (weak signal on tuner 2).
8. **`⌘K`** anywhere — type “cary grant”, “salt”, “smart”, or “scan”.
9. **Server Console** (profile menu → Server Console) — live load charts, sessions with the *why* of every transcode, a 36,000-line live log with level/source/text filters, drives with SMART warnings, backups with restore, users with per-person bandwidth limits and library access.
10. **On a phone** — the floating tab bar, large titles, bottom sheets, and the console as a native-feeling grouped list.

## Status and next steps

**Status (27 Sep 2026):** feature-complete against the brief on fixture data. Every screen in the index renders at all four review sizes with no console errors and no sideways scrolling; sign-in → profile → browse → play → music → live TV → console flows are exercised by `npm run smoke`.

Known limitations and next steps:

- **Playback is simulated on real media.** Every title plays one of two bundled films, looped to the title’s runtime; subtitles are descriptive captions rendered by the player. A real backend supplies HLS/DASH manifests and WebVTT tracks through `getPlayable`.
- **Refraction is Chromium-only.** Safari and Firefox get the same glass without edge lensing (they don’t support SVG filters in `backdrop-filter`).
- **Drag-to-reorder** exists for the music queue; playlists and agent order use buttons instead of drag.
- **Some admin actions are acknowledged rather than executed** in the fixture (e.g. restore, add tuner, speed test values are canned).
- **Next:** PWA install with a service worker; a light theme for daytime photo browsing; request/approval workflow for kids and friends (“Noor requested a title”) as its own inbox.
