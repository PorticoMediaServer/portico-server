# 16 — Web app engineering architecture

Auditor 16 of the 11 October 2026 comparative audit. Read-only source review. Nothing here was run, built or reproduced in a browser; every statement about behaviour is what the source says should happen.

Path shorthand used throughout (all relative to `/home/justin/projects/Portico`):

| Shorthand | Real path |
|---|---|
| `web/…` | `apps/portico-server/web/…` |
| `core/…` | `apps/portico-server/packages/client-core/src/…` |
| `i18n/…` | `apps/portico-server/packages/i18n/…` |
| `design/…` | `apps/portico-server/packages/design/…` |
| `contracts/…` | `apps/portico-server/packages/contracts/…` |
| `server/…` | `apps/portico-server/server/…` |

Comparison paths are written `jellyfin-web/…`, `jellyfin-vue/…`, `navidrome/…`, `audiobookshelf/…`, `streamyfin/…`, `jellyfin-desktop/…`.

## Scope and sources

**Domain.** How the web app is engineered: code structure, state and data fetching, routing, the shared-core strategy, build and bundle, performance techniques, design system and theming, internationalisation (i18n), accessibility (a11y), browser support, and testing. The web video/audio player belongs to auditor 17 and screen-by-screen feature parity to auditor 18; I only touch them where the architecture shows through.

**Portico, read in full or nearly so**

- Docs: `Project Architecture/06 Clients.md`, `Backlog.md`, `web/README.md`; Product Contracts `accessibility-and-language/contract.md` (normative sections), and targeted reads of `navigation-and-input/contract.md`, `visual/contract.md`, `decisions.md`; `Audits/2026-10-01-frontend-review.md` (scanned for overlap).
- Build and tooling: `web/package.json`, `vite.config.ts`, `index.html`, `tsconfig.json`, `web/scripts/lint-design.mjs`, `tokens.mjs`, `i18n/scripts/lint-strings.mjs`, `lint-raw-errors.mjs`, `lint-glossary.mjs`, `core/../scripts/check-preference-consumers.mjs`, root `package.json`, `.github/workflows/ci.yml` (web steps), `Dockerfile` (web stage).
- The built output in `web/dist` (a local build dated 9 Oct): file sizes, gzip sizes (computed locally), `index.html` preload list, and source maps to attribute each chunk's contents.
- App core: `web/src/main.tsx`, `app/App.tsx`, `app/router.tsx`, `app/session.tsx` (all 1,086 lines), `app/content.ts`, `home.ts`, `home-recovery.ts`, `browse.ts`, `browse-page-source.ts`, `detail.ts`, `libraries.ts`, `rail-cache.ts`, `list-position.ts`, `server-changes.ts`, `device-events.ts`, `viewer-scope.ts`, `preferences.ts`, `server-preferences.tsx`, `i18n.ts`, `errors.tsx`, `device.tsx`, `foreground-retry.ts`, `polyfills.ts`, `inbox.tsx`, `not-interested.ts`, `open.ts`.
- Shell and UI kit: `shell/Shell.tsx` (first half), `Connection.tsx`, `ServerSwitcher.tsx`; `ui/WindowedGrid.tsx`, `static-collection.ts`, `Grid.tsx`, `Card.tsx` + CSS, `Artwork.tsx`, `artwork-store.ts`, `artwork-path.ts`, `Shelf.tsx`, `Dialog.tsx`, `Menu.tsx`, `Field.tsx`, `Tabs.tsx`, `Button.tsx` (head), `base.css`, `tokens.css`, `viewport.ts`, `i18n.ts`.
- Bridge (non-media): `bridge/api.ts`, `server-connections.ts`, `viewer-selection-storage.ts`, `profile-trust.ts`, `restore-policy.ts`, `account.ts`, `protectedArtwork.ts`.
- Screens, selectively: `screens/library/Library.tsx` (the windowed results half), `home/Home.tsx`, `search/Search.tsx` (first half), `auth/Gate.tsx`, `auth/SignIn.tsx` (head), `NotFound.tsx`, `live/guide.ts` (formatters), `player/engine.tsx` (provider wiring only), `admin/console.ts`, `poll-while.ts`, `cursor-read.ts`.
- Shared packages: `core/index.ts` (transport, `HttpLocalApi`, `ApiError`), `collections/windowed.ts`, `session-refresh.ts`, `server-connections.ts` (refresh), `viewer-selection.ts`, `route-connection.ts` (fetch wrapper), `playback-v1/events.ts`, `sse.ts`, `home.ts` (parser), `library-content.ts` (types), `presentation/errors.ts`; `i18n/src/index.ts`, `messageformat.ts`, `catalog/regional.ts`, `catalog/index.ts`; `design/COMPONENTS.md` (head); `contracts/src` (index, runtime, decoders, routes, test).
- Server, only where it decides web behaviour: `server/internal/httpapi/installation.go` (static files), `cache_headers.go`, `artwork_response.go`, `compression.go` (head), `router.go` (security headers).
- Tests: `web/test/register.mjs`, `resolve.mjs`, `helpers/component-harness.mjs`, `list-position.test.ts` (part), `integration/setup-flow.mjs` (head); counts of the rest.

**Comparison projects, read**

- **jellyfin-web** (master, package version 13.0.0): `package.json`, `src/index.jsx`, `RootApp.tsx`, `RootAppRouter.tsx`, `utils/query/queryClient.ts`, `components/QueryClientEventHandler.tsx`, `components/layoutManager.js`, `serviceworker.js`, `scripts/browser.js` (device detection), `scripts/keyboardNavigation.js`, `scripts/gamepadtokey.js`, `components/images/imageLoader.js`, `lib/globalize/index.js`, `elements/emby-itemscontainer` (event wiring), `scripts/serverNotifications.js`, `components/CustomCss.tsx`, `themes/index.ts`, `components/apphost.js` + `constants/appFeature.ts`, `eslint.config.mjs`, `webpack.common.js` (chunking), test file list, `strings/` count.
- **jellyfin-vue**: `packages/frontend/package.json`, `src` layout, `composables/apis.ts` (head), `composables/page-title.ts`, store and worker layout, `vite.config.ts` (targets), `packages/tauri-runtime/README.md`, `ui-toolkit` layout.
- **Navidrome UI**: `ui/package.json`, `src/sw.js`, `swNavigation.js`, `eventStream.js`, activity reducer (grep), theme and language counts.
- **Audiobookshelf client**, **Streamyfin**: `package.json` and config only, for stack contrast.
- **jellyfin-desktop**: `native/` file list, for the wrapper pattern.

**Not reached.** Plezy (not a web client; out of scope per the prompt). The full `web/src/screens/server/**` console (33 files), `screens/channels/Channels.tsx` guide virtualisation, `player/**` and `bridge/audio/**` (auditor 17), most of `core/presentation/**` beyond the error presenter, the React Native app's use of client-core (only the package copy was diffed), and the Hosted service's own web host and headers. No browser, device or performance measurement of any kind.

## How Portico does it today

**One single-page app, one shell.** The web app is a React 19 single-page application built with Vite (`web/package.json`). `main.tsx` mounts `<RouterProvider>`; the root route wraps everything in a local-preferences provider and the session provider; the `app` route wraps signed-in pages in `Gate` (decides: restoring / sign-in / shell) and `Shell` (rail or bottom tabs, plus thirteen nested providers for libraries, inbox, artwork, server preferences, player, selection, metadata editor, delete, cast, Watch Together, downloads, channel sources) — `web/src/shell/Shell.tsx:151-195`. React StrictMode is deliberately off because core services are not double-effect safe (`web/src/main.tsx:8-11`).

**Very few dependencies.** Nine runtime packages: React, React DOM, TanStack Router, Radix UI (accessible unstyled primitives for dialogs, menus, tabs, switches), a patched hls.js, `qrcode`, and `@noble/curves` + `@noble/hashes` (a JavaScript Ed25519 and SHA-256 used when the page is on plain HTTP and the browser hides WebCrypto). No state library, no data-fetching library, no CSS framework, no component kit, no ESLint, no Prettier, no test runner package (tests use Node's built-in runner). The toolchain is very new: TypeScript 7.0.2 and Vite 8.2.2.

**The shared core does the thinking.** `packages/client-core` is 2.25 MB of headless TypeScript (no UI) with 182 test files. It owns the HTTP transport (`HttpLocalApi`, `core/index.ts:212-494`), sign-in and token renewal, a route layer that races several addresses for one server and pins its identity (`core/route-connection.ts`), every response parser, and one small "service" per screen family (Home, library content, detail, show workspace, search, saved, guide, inbox…). Each service exposes `subscribe` / `getSnapshot`, and the web binds it with `useSyncExternalStore` (71 call sites) or the `useService` helper (`web/src/app/content.ts:15-33`). It also holds the **authored presentation structure**: the Settings page tree, a library's tabs, card captions, search chips, title-page facts, error wording rules — so web and native render the same structure. The React Native repo carries a byte-identical copy of `client-core`, `design` and `i18n` (I diffed the three `src` trees: zero differences).

**Server-authored documents.** The server composes Home rows, library sections, sort and filter vocabularies, labels and recommendations; the client renders. Every response is validated by a hand-written "bounded parser" (length caps, type checks, frozen output) before use, e.g. `core/home.ts:36-80`. Unknown card kinds are dropped; most other malformed fields reject the whole document.

**Data fetching is per screen, with no shared cache.** A screen mounts, creates its service or runs its hook, fetches, and throws the result away on unmount. There is no query cache between screens. The exceptions are small and deliberate: a module-level map of browse capabilities (`web/src/app/browse.ts:21`), the search session kept in a module variable (`web/src/screens/search/Search.tsx:34-45`), a five-entry in-memory memory of visited list windows for Back (`web/src/app/list-position.ts`), the rail's last libraries in `localStorage` (`rail-cache.ts`), and the artwork blob cache. Stale answers are fenced by generation counters and `AbortController`s rather than by a library. Mutations are mostly "send, then refetch", with a few hand-rolled optimistic cases (Not interested, Remove from Continue Watching).

**Transport.** `HttpLocalApi.request` adds the bearer token, asks the refresher for a live token first, retries once on a 401 after renewing, and turns failures into `ApiError` with `code`, `status`, `retryable`, `retryAfterSeconds`. It has no default timeout and no retry policy of its own; callers add those (Home has a 15 s deadline and backoff in `home-recovery.ts`; console reads have 10 s; most of the 82 direct `api.request` call sites in `web/src` pass no signal). Reads are retried once after the route layer recovers; writes are never replayed.

**Live updates.** One event feed, `/v1/events`, is read by `EventsClient` (`core/playback-v1/events.ts`). It is designed for server-sent events with a long-poll fallback, but the web constructs it without a stream opener (`web/src/player/engine.tsx:120`), so it long-polls (25 s rounds). Only two things in the web UI listen: library scan progress and "the server ended this playback". Everything else refreshes when the screen mounts, when the user acts, or on a timer (inbox unread count every 60 s while visible).

**Sign-in state.** Access tokens last 15 minutes and rotate; the refresh credential lives in IndexedDB encrypted with a non-extractable WebCrypto key (plain JSON on plain HTTP, where WebCrypto is unavailable). Tabs serialise renewal with the Web Locks API and adopt each other's rotations (`core/server-connections.ts:225-270`, `core/session-refresh.ts`). `session.tsx` is one 1,086-line controller exposing one context value with about forty fields.

**Routing.** TanStack Router with code-defined routes and hand-validated search params (`web/src/app/router.tsx`). Real URLs (history mode, not `#`), so library tabs, filters (a JSON array in `?filters=`), sort, detail pages and settings sections are linkable. Most screens are code-split with `React.lazy` behind a page-shaped skeleton. The router's scroll restoration is on; preloading is set to "intent".

**Lists.** Big lists use a sparse windowed collection (`core/collections/windowed.ts`) plus `WindowedGrid` (`web/src/ui/WindowedGrid.tsx`): only visible rows are in the DOM, pages of 60 are fetched by offset and pinned to one server revision, at most ten pages stay in memory, and the page's own scrollbar covers the whole list (with a mapped mode above 8 million pixels). Letter jumps use a server-provided position index.

**Artwork.** Images need a bearer token, so they are fetched with `fetch`, turned into blobs and object URLs, and scheduled by a shared `ArtworkScheduler` (viewport order, 4 or 10 parallel requests by HTTP version, byte-budgeted LRU, backoff honouring `Retry-After`) — `web/src/ui/artwork-store.ts`, `Artwork.tsx`. Three server sizes (400 / 800 / 1920) are picked from the frame's width × device pixel ratio.

**Design system.** `packages/design` holds tokens and a component spec; `scripts/tokens.mjs` compiles tokens to CSS custom properties; `src/ui` holds every primitive and is the only place raw colours, font families and z-index numbers may appear, enforced by `scripts/lint-design.mjs`. Styling is CSS Modules plus inline `style`. One dark theme. One bundled font (Manrope, Latin subset).

**i18n.** `packages/i18n` is a zero-dependency ICU MessageFormat subset (named arguments, plural, select, number/date) with about 4,570 message IDs. US English is the source; Canadian and British English are derived by spelling rules at first use (`i18n/src/catalog/regional.ts`). Dates, numbers and durations go through `Intl` with a profile-level locale and 12/24-hour preference. `lint-strings` is a ratchet against hard-coded copy in JSX.

**Checks.** `npm run check` = regenerate tokens, design lint (strict), preference-consumer check, string lints, `tsc`, and 108 Node test files. CI also runs the client-core tests and a production build. There are no browser tests.

**Build and serving.** `vite build` with default targets; in the Docker image source maps are off (`Dockerfile:48`). The Go server serves the folder with a single-page fallback (`server/internal/httpapi/installation.go:38-89`) and a strict Content-Security-Policy. A second "Hosted-only" build (`VITE_HOSTED_WEB`) is served at web.getportico.tv and talks to the user's servers cross-origin.

## How the others do it

### jellyfin-web (the official Jellyfin web client)

- **Three generations in one bundle.** A legacy app (HTML templates, jQuery-era "controllers", custom elements named `emby-*`, a global event bus), a "modern" React + MUI app, a React dashboard app, and a setup wizard, all mounted under one React Router **hash** router; which user-facing app you get depends on a layout setting (`jellyfin-web/src/RootAppRouter.tsx:22-35`). The migration has been running for years: 245 `.js`, 316 `.ts`, 289 `.tsx`, 68 `.html` files today.
- **Sixty runtime dependencies**, including MUI 6, Emotion, TanStack Query 5 with a persisted cache, axios, jQuery, lodash, date-fns, Swiper, hls.js, flv.js, pdf.js, epub.js, libass, and a row of polyfills (`core-js`, `whatwg-fetch`, `intersection-observer`, `resize-observer-polyfill`, `webcomponents.js`, `proxy-polyfill`).
- **Data layer.** New code uses TanStack Query through the generated `@jellyfin/sdk`: stale after 60 s, kept 24 h, retried twice, never retried on 401, persisted to IndexedDB and busted by build version (`jellyfin-web/src/utils/query/queryClient.ts`). Legacy code calls `jellyfin-apiclient` directly. A WebSocket pushes `UserDataChanged`, `LibraryChanged`, timers and remote-control commands; list containers patch cards in place or refresh (`jellyfin-web/src/elements/emby-itemscontainer/emby-itemscontainer.js:144-299`), and the query cache is invalidated on a "refresh needed" event.
- **Client-composed screens.** The client decides which rows Home has, issues one query per row, and builds cards itself (`cardBuilder.js`, 1,272 lines). Lists are paginated (100 per page by default; 0 means "load everything"); there is no virtualisation.
- **Every screen size and many devices.** `browser.js` detects webOS, Tizen, Titan OS, Vidaa/Hisense, Xbox, PS4, Vega and old Edge; `layoutManager` picks mobile / desktop / tv; `focusManager` (500 lines), `keyboardNavigation`, `gamepadtokey` and `inputManager` give spatial (arrow-key) navigation and gamepad support. The browserslist goes back to Chrome 27 and iOS 10, with Babel, `es-check` on the output and an ESLint rule (`compat`) that flags unsupported browser APIs. The same bundle is what the webOS, Tizen and desktop wrappers load; a `window.NativeShell` object lets a wrapper supply device profile, fullscreen, downloads and so on (`jellyfin-web/src/components/apphost.js`).
- **Theming.** Six colour schemes (dark, light, appletv, blueradiance, purplehaze, wmc) as MUI CSS-variable themes, plus server-wide custom CSS (branding) and per-user custom CSS injected as `<style>` (`jellyfin-web/src/components/CustomCss.tsx`).
- **i18n.** 108 language files translated through Weblate, loaded one chunk per language on demand, flat JSON with positional `{0}` placeholders and no plural rules; RTL handled by setting `dir` and loading `rtl.scss`; `lang` is kept on `<html>` (`jellyfin-web/src/lib/globalize/index.js`).
- **Images.** Plain image URLs with a cache tag; lazy loading by IntersectionObserver; blurhash placeholders decoded in a Web Worker (`components/images/imageLoader.js`).
- **Quality gates.** ESLint (react-hooks, jsx-a11y, compat, sonarjs, import), Stylelint, TypeScript check; Vitest with only 12 test files. A service worker exists only to handle notification clicks.

### jellyfin-vue (the alternative "modern rewrite")

Vue 3 + Vite + Vuetify + UnoCSS in a pnpm workspace (`frontend`, `ui-toolkit`, `i18n`, `shared`, `tauri-runtime`, `configs`, `vite-plugins`). Data comes through typed composables over `@jellyfin/sdk` into a reactive store backed by IndexedDB (Dexie), so items are shared between screens and updated by the WebSocket. Blurhash and other heavy work run in Web Workers through Comlink. It has its own virtual scroller (`ui-toolkit/src/components/JVirtual`), i18next with 44 locales, Storybook, ESLint, a modern-only browser target (`defaults and supports es6-module`, build target ES2024), a page-title composable, and a separate **Tauri runtime package** that injects desktop-only behaviour around the unchanged web build.

### Navidrome UI and Audiobookshelf client (briefly)

- **Navidrome**: React 17 + react-admin 3 + Redux + Material-UI 4. It is an installable Progressive Web App: Workbox precaches the app shell and serves an offline page when the server is down (`navidrome/ui/src/sw.js`). A native `EventSource` stream delivers `refreshResource`, `scanStatus`, `serverStart` and keep-alives; the client reconnects after 5 s. 58 themes, 36 languages (translations served by the server).
- **Audiobookshelf**: Nuxt 2 + Vuex + socket.io + Tailwind, also a PWA (`@nuxtjs/pwa`), 43 language files, Cypress component tests.

### Streamyfin (React Native, only for the shared-core contrast)

Uses `@jellyfin/sdk` + TanStack Query with a persisted cache + Jotai atoms. The "shared core" there is the published SDK plus a query cache, not a first-party headless package of behaviour.

## Architectural differences

### A-16.1 One app versus three apps in a long migration
- **Portico:** One React app, one shell, one design system, written once (`web/src`, about 2 MB of TypeScript). The owner console is the Server section of Settings in the same app.
- **Jellyfin:** jellyfin-web ships a legacy app, a modern app, a dashboard app and a wizard side by side, with notes in the code such as "The app will crash if these get removed from the DOM" (`jellyfin-web/src/RootAppRouter.tsx:43-46`) and "This is an anti-pattern" (`src/index.jsx:37`). jellyfin-vue is a clean rewrite that is still not the default client.
- **Why it matters:** Portico has no legacy tax: one way to build a screen, one focus system, one error path. Jellyfin pays for every change twice but can keep very old devices working.
- **Assessment:** **Portico ahead.** Nothing to adopt. The lesson from Jellyfin is the cost of ever letting a second way of building screens in.

### A-16.2 Dependency surface: 9 packages versus 60
- **Portico:** React, React DOM, TanStack Router, Radix UI, hls.js (patched tarball), qrcode, two `@noble` crypto packages (`web/package.json`). `@types/qrcode` sits in `dependencies` rather than `devDependencies` (harmless).
- **Jellyfin:** 60 runtime and 70 dev dependencies. jellyfin-vue about 27 runtime.
- **Why it matters:** Fewer supply-chain and upgrade risks, a bundle the team fully understands. The price is that Portico re-implements what libraries give for free: a query layer, a virtualiser, a message formatter, an image scheduler, a test harness.
- **Assessment:** **Trade-off (subjective).** Consistent with the project's "no external contributions, understand everything" stance. The hand-rolled pieces are good, but each is a place bugs live (see B-16.1, B-16.4, B-16.9). I would keep the small surface and add only tooling (lint, a DOM test environment), not runtime libraries.

### A-16.3 Bleeding-edge toolchain versus conservative one
- **Portico:** React 19.2.8 (exact pin), TypeScript 7.0.2 (the new native compiler line), Vite 8.2.2 (Rolldown bundler), no Babel, no browserslist, default build target. CI uses Node 22.22.2 (`.github/workflows/ci.yml:76`) while the Docker image builds the web with `node:24-bookworm` (`Dockerfile:30`).
- **Jellyfin:** React 18.3, TypeScript 5.9, webpack 5 + Babel + PostCSS preset-env, browserslist, `es-check` to prove the output parses on the oldest target.
- **Why it matters:** Portico gets fast builds and modern output, and accepts the risk of young tools (a TS 7 or Rolldown regression lands on the only build). The CI/Docker Node mismatch means the artefact that ships is not built by the toolchain that was tested.
- **Assessment:** **Trade-off (subjective)**, leaning fine for a pre-release product. Aligning the Docker and CI Node versions is a small fix worth doing.

### A-16.4 Bespoke design system on Radix versus a component kit
- **Portico:** Tokens and a written component spec in `packages/design`, shared with the native app; primitives in `web/src/ui` built on Radix behaviour (focus trap, roving focus, typeahead) with Portico's own CSS. A lint script keeps raw colours, fonts and z-index inside `src/ui` (`web/scripts/lint-design.mjs`).
- **Jellyfin:** MUI 6 in new code, hand-made `emby-*` custom elements in old code, SCSS files; jellyfin-vue uses Vuetify plus its own `ui-toolkit`.
- **Why it matters:** Portico's look is its own and identical in intent across web and native; the boundary lint is something neither Jellyfin client has. Cost: every new control is written and made accessible by hand.
- **Assessment:** **Portico ahead** on coherence. The risk is accessibility detail work (see B-16.13 to B-16.17).

### A-16.5 CSS approach
- **Portico:** CSS Modules + CSS custom properties + a fair amount of inline `style={{…}}`. Heavy use of modern CSS: `color-mix()`, `:has()`, `aspect-ratio`, `inert`, container-free fluid `clamp()` widths. Sizes are in pixels.
- **Jellyfin:** SCSS with autoprefixer and PostCSS preset-env down-levelling; MUI's Emotion CSS-in-JS for new parts. jellyfin-vue: UnoCSS utilities + Lightning CSS targeting a browserslist.
- **Why it matters:** Portico's CSS is small (139 KB total) and has no runtime cost, but nothing down-levels it: a browser without `color-mix()` or `:has()` silently loses surfaces and focus rings (B-16.12). Pixel font sizes ignore a user's browser default text size (zoom still works).
- **Assessment:** **Trade-off (subjective).** Fine if the support matrix is "current evergreen browsers" and that is written down (see Open questions).

### A-16.6 Data layer: hand-rolled services versus a query cache
- **Portico:** Per-screen services in client-core with `subscribe/getSnapshot`; ad-hoc hooks (`useHome`, `useLibraries`, `useRead`) elsewhere. Request de-duplication, cancellation, stale-answer fencing, retry and backoff are written case by case: Home has all of them (`web/src/app/home-recovery.ts`), the windowed collection has de-dupe/abort/LRU, the admin `useRead` has a sequence fence but no abort (`web/src/admin/console.ts:22-38`), and many one-off `api.request(…).then(setState)` effects have only an `active` flag.
- **Jellyfin:** TanStack Query gives all of that uniformly to new code (de-dupe by key, cancellation, retries, background refetch, devtools). jellyfin-vue has one typed composable (`useApi`/`useBaseItem`) feeding a store. Navidrome leans on react-admin's data provider.
- **Why it matters:** Portico's best paths are more careful than a default query library (foreground-only retries, server `Retry-After` floors, revision fences). Its ordinary paths are less uniform, and each screen author decides again. There is no devtool to see what is loading.
- **Assessment:** **Trade-off (subjective).** Portico's choice is deliberate and tied to the shared core (the same services run on React Native without React Query). Adopting TanStack Query would be large and would fight the "services own fences" design. A cheaper step is one shared `useRead`-style helper in client-core with abort, deadline, fence and `Retry-After`, and moving the stragglers onto it.

### A-16.7 No cache between screens versus cached, persisted queries
- **Portico:** Leaving a screen discards its data; returning refetches. Deliberate under the "Fresh: no time-based caches" principle. Back-navigation speed is handled by special cases (search session, list-window memory, artwork cache).
- **Jellyfin:** Queries are fresh for 60 s, kept for 24 h and persisted to IndexedDB, so a reload or Back paints from cache and revalidates. jellyfin-vue keeps a normalised item store in IndexedDB.
- **Why it matters:** Portico always shows current data and never a stale page, at the cost of a skeleton on every Back to Home, a title or a show. On a slow link or weak server that is felt. Jellyfin is snappier and occasionally shows stale state.
- **Assessment:** **Trade-off (subjective).** A middle path that keeps the principle: keep the last document per route in memory keyed by the server's revision pair (`revision.catalog`, `revision.viewer` already ride on every Home/content document, `core/home.ts:6`), paint it instantly on Back, and revalidate. That is "memo keyed by exact revision", not a time cache. Medium size; touches the content/Home/detail hooks.

### A-16.8 Server-authored documents versus client-composed screens
- **Portico:** The server sends finished rows, order, headings (as message codes with parameters and a fallback), sorts and filters; clients never rank or regroup. Web code mostly maps a `ContentSection` to a `Shelf` or grid.
- **Jellyfin:** The web client composes Home from user settings, fires a request per section, and builds captions and badges itself. Different Jellyfin clients therefore differ in rows, ordering and wording.
- **Why it matters:** Portico gets consistency across six platforms and can change Home without shipping clients; one request per Home instead of a dozen. It gives up client-side flexibility: a third-party client or a user script cannot easily invent a new row, and every UI experiment needs a server change. Velocity is high for "same thing everywhere", lower for "try this only on web".
- **Assessment:** **Portico ahead** for a first-party, multi-platform product. Keep watching for client-side decisions creeping back in; one small example is `web/src/screens/home/Home.tsx:41`, which decides "this row continues by date added" by testing the row's id against `/recent|added/i`.

### A-16.9 Authored presentation structure in the shared core
- **Portico:** Settings pages and rows, library tabs, card captions, search chip order, title-page facts, playback-info rows and the error-wording rules live in `core/presentation/**` and are rendered by both web and native.
- **Jellyfin:** Each client authors its own settings screens and captions. Streamyfin and jellyfin-web have nothing in common above the SDK.
- **Why it matters:** One place to change a rule; native ports get behaviour "for free". The cost is that web-only presentation needs a core change and a copy to the other repository.
- **Assessment:** **Portico ahead.** This is the most distinctive architectural asset in my domain.

### A-16.10 Shared core distributed by copying a folder
- **Portico:** The React Native repo "carries an identical copy" of `client-core`, `design` and `i18n` (06 Clients). Today the copies are identical (verified by diff). The reason given for a hand-written message formatter is that the native bundler does not resolve outside its own `node_modules` (`i18n/src/messageformat.ts:8-12`).
- **Jellyfin:** `@jellyfin/sdk` is a published npm package consumed by web, Vue and Streamyfin.
- **Why it matters:** Copying works for one developer and one moment; it is the kind of arrangement that drifts silently. Nothing in CI checks the two copies match (I did not find such a check; the copy lives in another repository).
- **Assessment:** **Portico behind** on mechanism, though not on outcome today. A checksum check in each repo's CI, or a git subtree/private package, would be small.

### A-16.11 Runtime validation of every response versus trusting generated types
- **Portico:** Hand-written bounded parsers validate and freeze everything. Unknown content kinds are dropped; other violations throw `invalid_home` / "unreadable response".
- **Jellyfin:** Generated TypeScript types from OpenAPI and no runtime validation; a surprising payload fails wherever it is first touched.
- **Why it matters:** Portico fails early and safely, bounds memory, and never renders garbage. But strict parsers turn small version skew into a dead page: `validateHomeRow` rejects the whole Home document if `artworkShape` is not one of three known strings (`core/home.ts:63`). The bundled web app always matches its server, but the Hosted-only build at web.getportico.tv and the native apps meet many server versions.
- **Assessment:** **Trade-off (subjective).** I would keep validation and make the tolerance rule explicit and uniform: unknown enum value on an optional presentation field degrades (drop the row or use a default), never the document. See B-16.10.

### A-16.12 Two parallel type systems: generated contracts nobody imports
- **Portico:** `packages/contracts` is generated from the Go route registry: 66 route descriptors, 132 types, JSON schemas and a schema-driven `decode()` (447 KB of source). No file in `client-core` or `web/src` imports it; two comments say "hand-written until generated contracts land". The clients reference about 221 distinct `/v1/…` path literals, so the generated set covers roughly a third of the surface. Backlog lists "Client-core: move onto the generated `packages/contracts` types".
- **Jellyfin:** One generated SDK is the only API surface in new code.
- **Why it matters:** Every API change must be made in Go and again by hand in TypeScript, with nothing proving they agree except tests against fixtures. A trap for the planned migration: the generated decoder looks up one big `schemas` object (`contracts/src/runtime.ts`, `schemas.ts` is 339 KB), so importing a single decoder would pull the whole table into the bundle unless the generator is changed to emit per-type schemas.
- **Assessment:** **Portico behind** today (duplication), with a deliberate plan. Finishing it is large. Before starting, decide whether the generated decoder or the hand-written bounded parsers are the long-term validator; running both would be the worst outcome.

### A-16.13 Transport: no default deadline, no built-in retry
- **Portico:** `HttpLocalApi.request` (`core/index.ts:387-420`) has no timeout unless the caller passes a signal, and no retry beyond "renew the token once on 401" and "retry a read once after the route recovers". Only 13 of the 82 direct `api.request` call sites in `web/src` mention a signal.
- **Jellyfin:** `jellyfin-apiclient` wraps fetch with a per-request timeout and a reconnect attempt on network failure; TanStack Query adds retries.
- **Why it matters:** A request that is accepted and then stalls (a sleeping disk, an overloaded SQLite writer, a half-open socket) leaves a spinner forever on any screen that did not add its own deadline. See B-16.8.
- **Assessment:** **Portico behind** on the default; ahead on the careful paths. A default deadline for non-streaming JSON requests in `HttpLocalApi` (overridable) is small.

### A-16.14 Live updates: long-poll with two listeners versus a WebSocket that drives the UI
- **Portico:** One `/v1/events` feed with cursor resume, de-duplication, jittered backoff and a resync signal. The web passes no stream opener, so it long-polls; `readEventStream` (the fetch-based SSE reader) is used only by the diagnostics tail, Watch Together and remote playback. UI listeners: scan progress and session end. Home, libraries, "watched" ticks and Continue Watching do not react to changes made on another device or by a scan finishing.
- **Jellyfin:** WebSocket; cards update when user data changes anywhere, lists refresh on library changes (debounced 10 s in the background), timers and recordings update, and any web client can be remote-controlled (`MoveUp`, `Select`, `DisplayMessage`, `GoHome`…). Navidrome: native EventSource with `refreshResource` events.
- **Why it matters:** In Portico a tab left open on Home shows what was true when it loaded. Watch an episode on the TV and the web's Continue Watching does not move until you navigate. Also, each open tab holds one long-poll request; on plain HTTP/1.1 a browser allows six connections per server across all tabs (see B-16.7).
- **Assessment:** **Portico behind.** The event feed already exists and is well made; what is missing is (a) wiring the SSE opener on web, and (b) a small set of "document changed" events (viewer revision bumped, catalogue revision bumped) that Home/library/detail hooks answer with a refetch. That fits the revision model exactly: an event carries the new revision pair, the screen compares and reloads. Medium size, server and client.

### A-16.15 Session and token handling in the browser
- **Portico:** Short-lived access token + rotating refresh credential, renewal serialised across tabs by Web Locks with an idempotent request id stored before sending; credentials encrypted at rest in IndexedDB; identity of the server pinned by key. On plain HTTP the lock becomes per-tab and storage is plain (`web/src/app/polyfills.ts:46-75`).
- **Jellyfin:** A long-lived access token in `localStorage`, no rotation.
- **Why it matters:** Portico's design is far stronger and far more complex; complexity shows up as multi-tab edge cases (B-16.2) and a very large controller (`session.tsx`). Auditor 15 owns the security judgement.
- **Assessment:** **Portico ahead** on security design, **behind** on simplicity. The 1,086-line `SessionProvider` with one context value is the riskiest single file in the web app to change; splitting sign-in flows (direct, Portico Account, restore) into separate modules behind the same context would be a medium refactor with no behaviour change.

### A-16.16 One giant session context
- **Portico:** `useSession()` returns one memoised object with ~40 fields and 40 dependencies (`web/src/app/session.tsx:1016-1072`). Any change (busy flag, error text, server list, token rotation through `scope`) re-renders every component that calls `useSession()`. The code works around it in places by keying on `sessionIdentity` instead of the token.
- **Jellyfin:** `useApi()` context holds `api`, `user`; other state lives in query results or legacy singletons.
- **Why it matters:** Unnecessary re-renders of the whole shell about every 13 minutes and on every sign-in state blip; more importantly it makes every consumer depend on everything.
- **Assessment:** **Portico behind**, mildly. Splitting into two or three contexts (identity: api/session/owner; account/servers; sign-in flow) is small to medium.

### A-16.17 Routing: real URLs with validated params versus a hash router
- **Portico:** History-mode URLs; every search param is validated and bounded by hand in the route table; legacy addresses redirect (`/live`, `/server/...`). The server falls back to `index.html` for unknown non-asset paths that accept HTML (`server/internal/httpapi/installation.go:64-72`).
- **Jellyfin:** Hash URLs (`/#/details?id=…`), which need no server fallback and work under any sub-path or `file://` wrapper.
- **Why it matters:** Portico's links are cleaner and shareable, typed end to end, and card art is a real `<a href>` (middle-click, copy link). Hash routing is what makes jellyfin-web trivially hostable under a base path and inside packaged TV apps.
- **Assessment:** **Portico ahead** for a browser product. See A-16.18 for the sub-path cost.

### A-16.18 The app assumes it is served from the root of the origin
- **Portico:** Absolute asset URLs (`/assets/...`), `/manifest.json`, no Vite `base`, and helpers that say so ("the app is served from the root", `web/src/app/open.ts:31`). The API is always `/v1/...` on the same origin.
- **Jellyfin:** A configurable Base URL lets the whole server live under `https://example.com/jellyfin`, a very common reverse-proxy setup. Navidrome has `BaseUrl` too.
- **Why it matters:** An owner who already runs a reverse proxy with one hostname and several apps under paths cannot put Portico at `/portico`. They need a dedicated hostname or port.
- **Assessment:** **Portico behind** if sub-path hosting matters to the audience (it is one of the most-requested options in every self-hosted project). Listed as F-16.6. Medium: Vite `base`, router `basepath`, API prefix, server prefix stripping, manifest.

### A-16.19 Code splitting and preloading
- **Portico:** Screens are split with `React.lazy` inside the route component (`web/src/app/router.tsx:17-20`); the player and actions sheet are fetched when the browser is idle (`web/src/shell/Shell.tsx:38-45`). `defaultPreload: 'intent'` is set, but routes have no loaders, the lazy components are not router-lazy, and cards navigate with `navigate()` (83 call sites) rather than `<Link>` (17), so hovering a card preloads nothing.
- **Jellyfin:** webpack splits every npm package into its own chunk and lazy-loads legacy controllers and language files; no hover preloading either.
- **Why it matters:** The first visit to Library, a title or a show in a session pays a chunk download after the click. The setting suggests intent that the code does not deliver.
- **Assessment:** **Unclear / small gap.** Either remove the inert setting or make it real: preload the Library/Detail/Show chunks on idle like the player, or move to the router's `lazy` route components so intent preloading works. Small.

### A-16.20 First-load weight
- **Portico (local build, 9 Oct, minified):** entry `index` 486 KB (153 KB gzip) + `session` 884 KB (258 KB gzip) + `engine`, `listening`, `not-interested` and a dozen small preloaded chunks: roughly 1.6 MB raw / 480 KB gzip of JavaScript before first paint, plus 67 KB CSS and a 24 KB font. hls.js (575 KB / 176 KB gzip) and the server console (311 KB) load later. From the source maps: the first-load chunks contain about 930 KB of client-core source, 291 KB of English message catalogue, 537 KB of react-dom, 220 KB of `@noble` crypto, and the Radix/Floating-UI overlay stack.
- **Jellyfin:** Not measured here (no build run). It is widely known to be heavier (MUI + legacy + polyfills), but it lazy-loads languages and legacy pages.
- **Why it matters:** On a phone over a weak uplink or a low-end Android TV box browser, half a megabyte of compressed script before anything paints is noticeable; parse/compile time on slow CPUs matters more than transfer.
- **Assessment:** **Unclear** without measurement, but three cuts are obvious and small: load `@noble` only when `crypto.subtle` is missing (it is imported unconditionally at `web/src/app/polyfills.ts:3-4`); split the message catalogue by surface (the 145 KB `web.ts` block includes all console and settings copy); stop `core/index.ts` (a 101 KB barrel with the 1,000-line legacy `PlaybackService` class in it) from being pulled whole into the entry.

### A-16.21 Static asset caching and compression
- **Portico:** Only files whose name starts with `assets/index-` are served `immutable`; every other hashed chunk, stylesheet and the font get `Cache-Control: no-cache` and are revalidated on every load (`server/internal/httpapi/installation.go:84-87`). Compression is gzip on the fly per request (`compression.go`); there are no pre-compressed (Brotli) files.
- **Jellyfin:** ASP.NET static files with ETags; Navidrome and Audiobookshelf precache hashed assets in a service worker.
- **Why it matters:** See B-16.3. Every page load costs one conditional request per chunk (about twenty are preloaded), which on HTTP/1.1 is three or four round trips of queueing before the app starts.
- **Assessment:** **Portico behind**; small fix (mark all `assets/*` immutable; optionally pre-compress at build).

### A-16.22 Browser support: undeclared modern-only versus declared ancient
- **Portico:** No browserslist, no documented matrix. The build target is Vite's default; CSS needs `:has()` (Firefox 121+, Safari 15.4+, Chrome 105+) and `color-mix()` (Chrome 111+, Safari 16.2+, Firefox 113+); JavaScript uses Web Locks, `structuredClone`-era APIs, `AbortSignal.any`. One polyfill file targets "older TV browsers (Chromium < 116, Safari < 17.4)" (`web/src/app/polyfills.ts:5-10`), which is a narrower and different floor than the CSS implies.
- **Jellyfin:** Declared: last two versions of evergreen browsers plus Chrome 27/38/47/53/56/63, Edge 18, iOS > 10, Firefox ESR; enforced by Babel, PostCSS, `es-check` and `eslint-plugin-compat`.
- **Why it matters:** Nobody can answer "does Portico work on my 2021 TV's browser / an iPad stuck on iOS 15 / Firefox ESR?" from the repository. The honest answer from reading the code is "probably partly, with missing focus rings or transparent panels".
- **Assessment:** **Unclear (decision needed).** Modern-only is a perfectly good choice; it should be written down and enforced (a `browserslist` plus `build.target`/Lightning CSS targets, and a one-screen "this browser is too old" guard). Small.

### A-16.23 Plain-HTTP engineering
- **Portico:** Treats `http://192.168.x.x:32500` as first-class: fills in `crypto.randomUUID`, a per-tab `navigator.locks`, a JavaScript SHA-256/Ed25519 for server identity checks, and plain credential storage (`web/src/app/polyfills.ts:39-77`, `web/src/bridge/server-connections.ts:6-10`), with tests (`web/test/insecure-context.test.ts`, `polyfills.test.ts`).
- **Jellyfin:** Needs none of this because it uses none of those APIs; HTTP just works.
- **Why it matters:** Portico chose strong primitives and then had to rebuild them for the most common home setup. The fallbacks weaken guarantees quietly: on plain HTTP tabs cannot coordinate token renewal (B-16.2 note), and credentials are stored unencrypted (deliberate, documented).
- **Assessment:** **Portico ahead** in care; it is a real cost of the "HTTP stays first-class" principle and worth knowing that the cross-tab guarantees only exist on HTTPS and localhost.

### A-16.24 List virtualisation versus pagination
- **Portico:** Sparse windowed collection, O(visible) DOM and memory, one scrollbar for the whole library, abort and LRU eviction, revision-pinned pages, and recovery when a pinned revision expires (`web/src/app/browse-page-source.ts`). A mapped-scroll mode handles lists taller than browsers allow.
- **Jellyfin:** Pages of 100 with Previous/Next buttons (or "0 = everything", which loads the whole library into the DOM). jellyfin-vue has a virtual scroller.
- **Why it matters:** Portico can show a million-item library as one scroll; Jellyfin's "load all" option is a well-known way to hang a browser. Virtualisation costs: browser find-in-page cannot see unrendered rows, focus is lost if the focused card scrolls out of the window, screen readers are not told the list size (B-16.16), and the component is intricate (B-16.4, B-16.5).
- **Assessment:** **Portico ahead**, clearly, for the stated 5–10 million item goal.

### A-16.25 Letter jumps
- **Portico:** The server returns a position index (letter → offset); the rail scrolls the same list to that offset (`web/src/screens/library/Library.tsx:436`).
- **Jellyfin:** The alpha picker applies a `NameStartsWith` filter, replacing the list with a filtered one.
- **Why it matters:** Portico keeps one continuous list (you can scroll up from "M" into "L"); Jellyfin's is simpler and works for any sort only as a filter.
- **Assessment:** **Portico ahead.**

### A-16.26 Scroll position on Back
- **Portico:** Two mechanisms at once: the router's scroll restoration (sessionStorage) and a hand-made memory of the last five list windows that re-renders the remembered rows before any request (`web/src/app/list-position.ts`, `Library.tsx:314-385`).
- **Jellyfin:** Legacy views keep the last three pages alive in the DOM (`jellyfin-web/src/components/viewContainer.js:245`), so Back shows the old DOM untouched; new React pages rely on the query cache.
- **Why it matters:** Returning to where you were is the most common complaint about media web UIs. Portico's design is the most ambitious of the three, and I believe it does not currently work for the main case (B-16.1).
- **Assessment:** **Portico ahead in design, probably behind in effect** until B-16.1 is checked.

### A-16.27 Artwork delivery: authorised fetch + object URLs versus plain image URLs
- **Portico:** Artwork needs `Authorization`, so every image is fetched by script, held as a Blob with an object URL, and managed by a custom scheduler and byte-budgeted cache (16 MB on ≤2 GB devices, else 48 MB; `web/src/ui/artwork-store.ts:45-47`). Versioned URLs (`?v=<digest>`) are served `private, immutable`.
- **Jellyfin:** Image URLs are unauthenticated-by-default GETs with a `tag` for cache busting; the browser's own loader, cache, decoding and memory management do everything. Lazy loading is an IntersectionObserver setting `src`.
- **Why it matters:** Portico's posters cannot be hot-linked or enumerated without a session (a privacy win), and the scheduler orders work by screen position and backs off under load. It costs: JavaScript memory for encoded bytes that the browser would otherwise manage; no `srcset`; no native priority hints or `loading="lazy"`; a failed blob cannot be "open image in new tab"; and a second cache to tune. A cookie-authorised image route would give the same privacy with native loading, but Portico avoids cookies on servers on purpose.
- **Assessment:** **Trade-off (subjective)**, deliberate (comment at `Artwork.tsx:51-54`). I would keep it, and note that the browser HTTP cache still helps across reloads because versioned artwork is `immutable`.

### A-16.28 Image sizes and device pixel ratio
- **Portico:** One of three widths (400, 800, 1920) chosen from frame width × `devicePixelRatio` so different screens share cache entries (`web/src/ui/artwork-path.ts:12-27`).
- **Jellyfin:** Asks for the exact pixel size per card (`fillWidth`/`fillHeight`, quality), so the server resizes on demand and caches many variants.
- **Why it matters:** Portico bounds server work and disk to three variants per image (good for weak boxes); a 170 px poster on a 1× screen still downloads the 400 px file. Jellyfin sends fewer bytes per image and does unbounded resize work.
- **Assessment:** **Portico ahead** for the "weak server, many viewers" target. A fourth small bucket (about 240) would be the only thing to consider, and only with measurements.

### A-16.29 Placeholders while images load
- **Portico:** A neutral frame with a kind icon (or an initial), then a fade-in; Backlog notes a "dominant colour" is planned and clients tint from the title's id meanwhile.
- **Jellyfin:** Blurhash (a 20–30 character string that decodes to a blurred preview) drawn on a canvas, decoded in a Web Worker; jellyfin-vue does the same.
- **Why it matters:** Blur previews make a grid feel loaded sooner and stop the "wall of grey" on slow links.
- **Assessment:** **Portico behind** (already half-planned). See F-16.9.

### A-16.30 Home is rendered in full
- **Portico:** Every Home row and every card in it mounts at once; rows page horizontally by cursor when the shelf's end scrolls into view (`web/src/screens/home/Home.tsx:98`, `ui/Shelf.tsx:35-37`). The server comment budgets "seventeen rows of up to sixty entries" (`server/internal/httpapi/compression.go:11-12`), so Home can start with about a thousand cards.
- **Jellyfin:** Home sections are requested and rendered separately with small limits (typically 12–24 items each).
- **Why it matters:** A thousand cards is roughly ten thousand DOM nodes plus a thousand IntersectionObserver targets on first paint. Fine on a laptop; a risk on phones and TV-class CPUs.
- **Assessment:** **Unclear** without measurement. Cheap mitigations if it shows: `content-visibility: auto` on shelves below the fold, or mount rows as they approach the viewport.

### A-16.31 Input modes: pointer, touch and keyboard only
- **Portico:** Mouse, touch (long-press for the card menu) and keyboard (Tab, roving arrow keys within a shelf and the guide grid, `/` for Search, `M`/`P` on a focused card). No gamepad, no TV-remote key codes, no spatial navigation, no "TV layout".
- **Jellyfin:** Three layouts (mobile, desktop, tv), a spatial focus manager, gamepad-to-key translation, remote-control key codes for webOS/Tizen/Xbox, an input manager that also receives remote commands from other devices.
- **Why it matters:** Portico's web app is not usable from a couch with a remote or controller. That is consistent with the plan (TVs get React Native apps), and the navigation contract even forbids a JavaScript spatial coordinator on TV platforms (`navigation-and-input/contract.md` line 204).
- **Assessment:** **Trade-off (subjective), deliberate.** It does mean LG webOS and Samsung Tizen — the two largest TV platforms, both web-app platforms — have no path from this codebase (see A-16.32).

### A-16.32 Is the web app a base for TV and desktop wrappers?
- **Portico:** No host abstraction. TVs use React Native (tvOS, Android TV, Fire TV, Vega). Nothing in `web/src` detects a wrapper. A desktop companion is planned (Backlog, "Planned features").
- **Jellyfin:** The web bundle *is* the webOS, Tizen, Xbox and desktop app; `window.NativeShell.AppHost` lets the wrapper declare features (`AppFeature` lists 28, from `Fullscreen` and `FileDownload` to `ExternalPlayerIntent` and `MultiServer`) and jellyfin-desktop injects an mpv-backed player (`jellyfin-desktop/native/mpvVideoPlayer.js`, `nativeshell.js`). jellyfin-vue isolates Tauri code in its own package.
- **Why it matters:** If the desktop companion is a web view around this app, it will need exactly this seam (native player, global media keys, tray, auto-start, downloads to disk, secure credential storage in the OS keychain instead of IndexedDB). Designing the seam before the wrapper avoids scattering `if (isDesktop)` through screens. The existing `bridge/` folder is the natural place: it already isolates transport, storage and playback adapters.
- **Assessment:** **Unclear (decision needed).** See F-16.14 and Open questions.

### A-16.33 Theming and personalisation
- **Portico:** One dark theme, one accent, by contract (`visual/contract.md` speaks only of the dark theme). Card size (Small/Medium/Large), Reduce motion and Show backdrops are server-stored profile preferences mirrored into the browser (`web/src/shell/Shell.tsx:204-220`). No custom CSS; the CSP (`style-src 'self'`) would block injected styles anyway.
- **Jellyfin:** Six themes including light, per-user and server-wide custom CSS, a large community of CSS themes; Navidrome ships 58 themes.
- **Why it matters:** Custom CSS is one of Jellyfin's most-used community features and also its biggest source of "the update broke my UI". Portico's fixed look protects the design and the support burden; it gives up light mode (an accessibility need for some people with astigmatism or in bright rooms) and any owner branding.
- **Assessment:** **Trade-off (subjective), deliberate.** The token pipeline makes a light theme or an accent choice cheap later (tokens are already the only colour source), which is the valuable part.

### A-16.34 Fonts
- **Portico:** One variable Manrope file, Latin subset (24 KB), with a `unicode-range`; other scripts fall back to system fonts (`web/src/ui/base.css:7-16`).
- **Jellyfin:** Noto Sans including separate CJK font packages loaded per language.
- **Why it matters:** Titles in Cyrillic, Greek, Japanese, Korean, Chinese, Arabic or Vietnamese render in a different (system) typeface next to Manrope UI text. Legible, slightly inconsistent; small download.
- **Assessment:** **Trade-off (subjective).** Reasonable while the UI is English-only.

### A-16.35 Message format
- **Portico:** ICU MessageFormat subset with named arguments, `plural`, `selectordinal`, `select`, number and date arguments; catalogue values are TypeScript objects so IDs are type-checked (`i18n/src/messageformat.ts`). 116 plural messages already.
- **Jellyfin:** Flat JSON, positional `{0}`, no plural support (strings like "{0} items" are simply wrong in many languages). jellyfin-vue uses i18next (plurals supported).
- **Why it matters:** Portico's catalogue is ready for languages with several plural forms and for type-safe message IDs. A hand-written parser is one more thing to maintain, but it is small and tested.
- **Assessment:** **Portico ahead.**

### A-16.36 Catalogue loading
- **Portico:** The whole catalogue (about 291 KB of source) is imported statically into the first-load chunk. Regional English is derived at runtime. There is no per-locale lazy loading mechanism yet (`i18n/src/catalog/index.ts`).
- **Jellyfin:** One chunk per language, loaded on demand; English is the fallback bundle.
- **Why it matters:** Today it is just bundle weight. With ten languages the current shape would ship megabytes of strings to everyone.
- **Assessment:** **Portico behind** structurally; fix before the second real language (dynamic `import()` per locale, keep en-US inline as the fallback as the contract requires). Medium.

### A-16.37 Languages and translation workflow
- **Portico:** English only (US source; CA and GB by spelling rules; "more languages planned"). About 4,570 message IDs — more than twice jellyfin-web's ~1,950 — and no translator-facing workflow, export format or context notes file.
- **Jellyfin:** 108 languages via Weblate with community translators. Navidrome 36, Audiobookshelf 43, jellyfin-vue 44.
- **Why it matters:** For a consumer media product, language coverage is a headline feature in much of the world. Portico's catalogue is large and written in TypeScript modules, which mainstream translation platforms do not ingest directly; and "no external code contributions" needs a position on translation contributions specifically.
- **Assessment:** **Portico behind** (known, deliberate for launch). See F-16.3.

### A-16.38 How code reaches the catalogue
- **Portico:** Three access paths: the `useI18n()` hook (61 files; each call builds its own formatter object, `web/src/app/i18n.ts:35`), a module-level `currentI18n()` used inside render in 43 files (not reactive), and the fixed `defaultI18n` bound at module scope in 17 screen files. The UI kit has a fourth setter (`ui/i18n.ts`).
- **Jellyfin:** One global `globalize.translate()`; a language change fires an event and clears the query cache.
- **Why it matters:** With one language the three paths look the same. They are not: see B-16.6. A single source (context or an external store read with `useSyncExternalStore`) would make locale changes reliable.
- **Assessment:** **Portico behind**; small-to-medium cleanup that should happen before real localisation.

### A-16.39 Locale-aware formatting
- **Portico:** One formatter set (`i18n.date/time/number/duration/relativeTime/list/bytes`) honouring a profile's locale and 12/24-hour choice, with the time zone always the device's. The contract forbids hand-authored separators and units. In practice several screens bypass it (B-16.6) and the formatter itself hard-codes byte units and a `", "` date-time joiner (`i18n/src/index.ts:184`, `:223`). `Intl` formatter objects are constructed on every call.
- **Jellyfin:** date-fns locales plus `toLocaleString`, with a separate "date/time locale" user setting.
- **Why it matters:** Portico's model (UI language and formatting locale separate, per profile, server-stored) is better. The gaps are consistency, not design.
- **Assessment:** **Portico ahead** in design.

### A-16.40 Right-to-left
- **Portico:** None: `<html lang="en">` is static (`web/index.html:2`), nothing sets `dir`, and styles use physical `left`/`right` about 130 times against 3 logical properties. The contract requires mirroring (A11Y-15).
- **Jellyfin:** Sets `dir="rtl"` for Arabic, Persian, Urdu, Hebrew and loads `rtl.scss`.
- **Why it matters:** Only when an RTL language is added; retrofitting logical properties across 41 CSS files is tedious but mechanical.
- **Assessment:** **Portico behind**, not urgent. A lint rule in `lint-design` rejecting new physical properties would stop the debt growing. Small.

### A-16.41 Text the server sends
- **Portico:** Server-authored labels arrive as `{code, params, fallback}` (`core/home.ts:9`); the client looks the code up in its own catalogue and falls back to the server's English. Errors arrive as stable codes; raw server messages are never shown (`core/presentation/errors.ts`, enforced by `lint-raw-errors`).
- **Jellyfin:** The server localises some strings itself and sends display text; clients often show server error text.
- **Why it matters:** Portico's UI language is decided in one place (the client) and an old client can still show a new row. It is also why the catalogue is so large.
- **Assessment:** **Portico ahead.**

### A-16.42 Error presentation
- **Portico:** One presenter maps code/status/type to a category, a title, a body and the single truthful action (Try again / Refresh / Sign in / Update / none), with cancellations silent and Hosted failures worded as the account service, never "your server" (`core/presentation/errors.ts`, `web/src/app/errors.tsx`).
- **Jellyfin:** Mixed: toasts, alerts, console logs, a connection-error page.
- **Why it matters:** Consistent, translatable failure UX on every screen and platform.
- **Assessment:** **Portico ahead.**

### A-16.43 "Server unreachable" UX
- **Portico:** The shell always opens; an unreachable server replaces the page area with one "Reconnecting" status after 2.5 s and comes back by itself (`web/src/shell/Connection.tsx`); restore retries with backoff only while the tab is visible (`foreground-retry.ts`); a sign-in that ended goes straight to the sign-in form with server and username filled in.
- **Jellyfin:** A full-page connection error with Retry; individual requests fail in place.
- **Why it matters:** Portico's is calmer and avoids a wall of per-card errors. Replacing the page area unmounts the screen underneath (B-16.11).
- **Assessment:** **Portico ahead**, with one risk.

### A-16.44 Error boundaries
- **Portico:** Only the router's error components (root and default) — `web/src/app/router.tsx:63,198`. No boundary around the player, the shell's overlays or individual shelves, and no handler for failed chunk loads (B-16.9).
- **Jellyfin:** A route `errorElement` and a few component boundaries; legacy code simply logs.
- **Assessment:** **Roughly equal; both thin.** A boundary around `PlayerSurface` and one per lazy overlay is small and stops a rendering bug in a menu from taking playback down with it.

### A-16.45 Accessibility engineering
- **Portico:** A written contract (WCAG 2.2 AA), Radix primitives, labelled fields with `aria-describedby`/`aria-invalid`, focus return after dialogs and menus, `inert` under the full player, a visible focus ring token, reduced-motion honoured from both the OS and a profile setting, `role="status"`/`alert` in feedback components. No automated a11y check and no lint for it.
- **Jellyfin:** `eslint-plugin-jsx-a11y` in lint; MUI's built-in semantics in new code; legacy elements vary; TV focus handling is strong.
- **Why it matters:** Portico's foundations are better than Jellyfin's legacy UI; the gaps are specific and listed as B-16.13 to B-16.17 (no main landmark or skip link, static document title, card state not announced, virtual list not described, two field types missing descriptions).
- **Assessment:** **Portico ahead on intent, mixed on detail.**

### A-16.46 Page titles and route announcements
- **Portico:** `document.title` is never set anywhere in `web/src`; every tab, history entry and bookmark is "Portico". No live region announces navigation. The contract requires a route announcement and a skip link (A11Y-4.3, 4.11).
- **Jellyfin:** Sets the document title per page (`jellyfin-web/src/scripts/libraryMenu.js:749,775`); jellyfin-vue has a `usePageTitle` composable.
- **Assessment:** **Portico behind.** Small fix (one hook reading the page header title).

### A-16.47 Multi-server on the web
- **Portico:** A server's own web app is that server's; switching to another directly-signed-in server navigates the browser to the other server's address (`web/src/shell/ServerSwitcher.tsx:47-51`). With a Portico Account, the Hosted-only build lists servers with presence and connects cross-origin; one active sign-in per browser.
- **Jellyfin:** One web client can hold credentials for several servers and switch among them in place (`ServerConnections`), limited by CORS and mixed content the same way.
- **Why it matters:** Portico's model is cleaner per origin and keeps CSP tight (`connect-src 'self'` plus Portico's certificate hostnames). It means there is no "all my servers in one tab" unless you use the Hosted build over HTTPS.
- **Assessment:** **Trade-off (subjective).**

### A-16.48 Multi-tab behaviour
- **Portico:** The saved sign-in is browser-wide; "a tab keeps the viewer it started with until it is reloaded" (`web/src/bridge/viewer-selection-storage.ts:4-8`). Only the Portico Account credential listens for `storage` events; local preferences, the rail cache and the selected viewer do not sync between tabs.
- **Jellyfin:** Token and settings in `localStorage`; tabs are independent and mostly consistent because nothing rotates.
- **Why it matters:** Rotation plus per-tab viewers creates states Jellyfin cannot have (B-16.2).
- **Assessment:** **Portico behind** on robustness here, because of its stronger security model.

### A-16.49 Installability and offline shell
- **Portico:** A web manifest and icons (`web/public/manifest.json`), no service worker. The page cannot be installed with offline support in Chromium's sense, shows the browser's error page when the server is down at load time, and re-requests its chunks each visit.
- **Jellyfin:** Service worker only for notification clicks; manifest present. **Navidrome / Audiobookshelf:** full PWAs with precached shells and an offline page; jellyfin-vue has none.
- **Why it matters:** For a self-hosted server that restarts for upgrades and backups, a precached shell means the app opens instantly, shows "Reconnecting" in its own UI, and upgrades atomically (old chunks are still in the cache — which also removes B-16.9). Service workers need a secure context, so plain-HTTP LAN users would not get it.
- **Assessment:** **Portico behind** Navidrome/Audiobookshelf, level with Jellyfin. See F-16.2.

### A-16.50 Where preferences live
- **Portico:** Server-owned preference registry with scopes and revisions; the web mirrors appearance values into `localStorage` for first paint (`web/src/app/preferences.ts`, `server-preferences.tsx`). A CI check fails if a registry key has no consumer (`check-preference-consumers.mjs`).
- **Jellyfin:** `DisplayPreferences` on the server plus a lot of `localStorage` settings that do not roam.
- **Assessment:** **Portico ahead.**

### A-16.51 Static analysis
- **Portico:** Purpose-built lints: design boundary, undefined CSS variables, off-grid spacing, hard-coded strings (ratchet), raw error text, glossary terms, preference consumers. **No ESLint at all** — yet 76 comments in `web/src` say `eslint-disable-next-line react-hooks/exhaustive-deps`, referring to a rule that no tool runs. No formatter.
- **Jellyfin:** ESLint with react-hooks (rules-of-hooks as error, exhaustive-deps as warning), jsx-a11y, compat, sonarjs, import ordering; Stylelint; es-check.
- **Why it matters:** Portico's custom lints catch product rules no generic tool knows. But with 320 `useEffect` calls and hand-managed dependency arrays, the two React hook rules are the cheapest bug-finders available, and they are not running. The stale `eslint-disable` comments make it look as if they are.
- **Assessment:** **Portico behind** on generic linting, **ahead** on product-specific linting. Adding ESLint with only `react-hooks` and `jsx-a11y` is small; expect a few hundred findings to triage once.

### A-16.52 Tests
- **Portico:** Many fast unit tests of pure logic (108 web files, 182 client-core files, about 1.8 MB of test code). "Component" tests use a home-made harness that rewrites a `.tsx` file's imports with regular expressions and supplies a fake React in which `useEffect` does nothing (`web/test/helpers/component-harness.mjs:52-67`); it "models hook storage, not rendering, focus or React scheduling" by its own description. No DOM environment, no real React rendering, no browser end-to-end tests, no visual regression, no accessibility automation. The one integration script is known to fail (Backlog).
- **Jellyfin:** 12 Vitest files (jsdom available); jellyfin-vue has Storybook; Audiobookshelf has Cypress component tests.
- **Why it matters:** Portico's logic coverage is far beyond any comparison project. What is untested is exactly where my suspected bugs sit: effect ordering, unmount behaviour, focus, layout, and anything that needs a real browser (B-16.1, B-16.4, B-16.9).
- **Assessment:** **Portico ahead on unit tests, behind on rendering/e2e.** A small Playwright suite against the dev server and the existing demo fixtures (sign in, Home, scroll a library, open a title, Back, play, switch profile in two tabs) would cover the biggest risks. Medium.

### A-16.53 Development aids
- **Portico:** Dev-only routes under `/gallery` (component gallery, feature fixtures, windowed-grid lab, browse lab, channels lab) and a `SessionFixture` for canned sessions. The routes are not registered in production, but their chunks are still emitted because the lazy imports sit at module top level (`web/src/app/router.tsx:41-45`; `web/dist/assets` contains `Gallery-*.js`, `Features-*.js`, `WindowedLab-*.js`, `BrowseLab-*.js`, `ChannelsLab-*.js`).
- **Jellyfin:** React Query devtools (hidden on TVs); jellyfin-vue: Storybook and Vue devtools.
- **Assessment:** **Roughly equal.** Wrapping the dev `split()` calls in `import.meta.env.DEV` would keep fixture code out of the shipped folder (small).

### A-16.54 No StrictMode
- **Portico:** Off on purpose because services have real side effects (`web/src/main.tsx:8-11`).
- **Jellyfin:** Also effectively off for the legacy tree; new code tolerates it.
- **Why it matters:** StrictMode's double-invoked effects are how React surfaces missing cleanups. Turning it off hides that class of bug rather than proving its absence; React's newer features (Activity, concurrent transitions) assume effect-safety.
- **Assessment:** **Trade-off (subjective), deliberate.** `useService` already defers disposal by a tick precisely to survive effect replay (`web/src/app/content.ts:20-31`), so the remaining distance to StrictMode-safe may be small; worth a dev-only experiment.

### A-16.55 Extension points
- **Portico:** None in the web client: no plugin loader, no custom scripts, no themes. Consistent with "no external code contributions".
- **Jellyfin:** Client plugins (players, screensavers, SyncPlay are plugins: `jellyfin-web/src/plugins`), server plugins that inject dashboard pages as HTML, community scripts and CSS.
- **Why it matters:** Jellyfin's openness is a large part of its ecosystem and of its breakage. Portico's server-authored documents are themselves a controlled extension point (new row kinds arrive from the server).
- **Assessment:** **Trade-off (subjective), deliberate.**

### A-16.56 One codebase, two builds (bundled and Hosted-only)
- **Portico:** `VITE_HOSTED_WEB` (or the hostname `web.getportico.tv`) switches four files into "Hosted" behaviour (`SignIn.tsx:17`, `ServerSwitcher.tsx:10`, `HostedChooser.tsx:68`, `DirectJoin.tsx:16`). The Hosted build talks to servers cross-origin with bearer tokens and relies on the server's CORS allow-list and on servers having certificates (an HTTPS page cannot call an HTTP server).
- **Jellyfin:** No hosted web client. (Plex has app.plex.tv, closed source.)
- **Why it matters:** The Hosted build is the one place where a web client meets servers of many versions, so version-skew tolerance (A-16.11, B-16.10) matters there first.
- **Assessment:** **Portico ahead** in capability; treat the Hosted build as the compatibility canary.

### A-16.57 Client-visible concurrency control
- **Portico:** Writes carry expected revisions and idempotency keys (`saveHomeLayout` with `expectedRevision` and `idempotencyKey`, preferences with document revisions, operation ids for Not interested), and the error presenter has a "changed → Refresh" category.
- **Jellyfin:** Last write wins.
- **Assessment:** **Portico ahead.**

### A-16.58 Content-Security-Policy posture and its side effects
- **Portico:** Strict CSP on every response: `script-src 'self' https://www.gstatic.com; style-src 'self'; …; frame-ancestors 'none'` (`server/internal/httpapi/router.go:1242`).
- **Jellyfin:** No CSP by default; custom CSS and plugin pages depend on inline styles and scripts.
- **Why it matters:** Strong default; it constrains libraries that inject `<style>` elements at runtime (B-16.18) and rules out user CSS without a nonce scheme.
- **Assessment:** **Portico ahead** (auditor 15 owns the security view).

### A-16.59 Search and the address bar
- **Portico:** The query and type chip are written to the URL (`/search?q=…&type=…`, replaced after 400 ms so typing does not fill history) — `web/src/screens/search/Search.tsx:69-74`.
- **Jellyfin:** `#/search?query=…` likewise.
- **Why it matters:** Linkable searches; but the navigation contract says search text is "excluded from route URLs, browser-history payloads and persisted restoration" (NAV item 8) and decision R14 says "no ordinary route URL/history payload", and the router file contradicts itself (`router.tsx:51` vs `:118`). See Doc/code disagreements.
- **Assessment:** **Unclear (decision needed).**

### A-16.60 URL design details
- **Portico:** Filters are a JSON array in `?filters=` (bounded to 2,048 characters); show and entity links carry `?title=…` so the header can paint before data; a season addressed without its show uses the literal path segment `season` (`/show/season?season=<id>`, `web/src/app/open.ts:21-24`, special-cased at `web/src/app/detail.ts:118`).
- **Jellyfin:** `#/details?id=…&serverId=…`.
- **Why it matters:** Portico's URLs are richer; the `title` param makes URLs long and puts titles in browser history and any shared link, and the `season` sentinel is a small wart that will confuse anyone reading logs.
- **Assessment:** **Trade-off (subjective); minor.**

### A-16.61 Client diagnostics
- **Portico:** A localStorage flag enables artwork timing logs (`web/src/app/artwork-metrics.ts`); route errors go to the console. No client error reporting to the owner's server, no global `unhandledrejection` handler.
- **Jellyfin:** A `ClientLog` upload endpoint (used by some clients); Streamyfin uses Sentry.
- **Why it matters:** When a viewer says "the page went blank", the owner has nothing to look at. A privacy-respecting option is a bounded client-error report to the owner's own server (never to Hosted).
- **Assessment:** **Portico behind**, mildly. See F-16.13.

## Missing features and capabilities to consider

### F-16.1 Live UI refresh from server events
- **What:** Screens update when something changes elsewhere: watched state and resume position after playing on another device, new items after a scan, a recording finishing, a library added by the owner.
- **Who has it:** Jellyfin (WebSocket `UserDataChanged`, `LibraryChanged`, timers), Navidrome (`refreshResource` over EventSource), Audiobookshelf (socket.io), jellyfin-vue (store updated from the socket).
- **Value:** High; "I watched it on the TV and the web still shows it unwatched" is a daily papercut.
- **Size:** Medium. The feed exists; add revision-bump events and have Home/content/detail hooks refetch when their revision is older. Wire the SSE opener on web at the same time.
- **Principles:** Fits "Fresh" (revision-keyed), and "Hosted stays cheap" is unaffected (server-local feed).
- **Opinion:** The most valuable item in this file.

### F-16.2 Service worker: precached shell, atomic upgrades, offline page
- **What:** Cache the hashed app shell so the app opens without the network, shows its own "Reconnecting" state when the server is down, and keeps old chunks until a new version is activated.
- **Who has it:** Navidrome (Workbox + offline.html), Audiobookshelf (`@nuxtjs/pwa`).
- **Value:** Faster repeat loads everywhere; fixes stale-chunk failures after upgrades; makes "Install app" real on Android and desktop Chromium.
- **Size:** Medium. Needs an update prompt ("A new version is ready") and care not to cache API or media. Only works on HTTPS and localhost.
- **Principles:** No conflict; nothing time-based if assets are content-hashed.
- **Opinion:** Worth doing after B-16.3 and B-16.9 are fixed the simple way.

### F-16.3 A translation pipeline and more languages
- **What:** A way for translators to work (export to a standard format such as XLIFF/JSON with context notes, import back, completeness report), lazy loading per locale, and a position on accepting translation contributions.
- **Who has it:** Jellyfin (Weblate, 108 languages), Navidrome (36), Audiobookshelf (43), jellyfin-vue (44).
- **Value:** High outside English-speaking markets.
- **Size:** Large overall (4,570 messages), but the engineering part (export/import, lazy load, lint for missing/mismatched variables — the contract already demands this in A11Y-14.4) is medium.
- **Principles:** "No external code contributions" needs an explicit carve-out or an internal/paid translation route.
- **Opinion:** Do the engineering part before 1.0 even if only one extra language ships, because it flushes out B-16.6.

### F-16.4 Light theme and accent choice
- **What:** A light colour scheme (following `prefers-color-scheme` or a setting) and perhaps a small accent palette.
- **Who has it:** Jellyfin (light + 4 others), Navidrome (58), jellyfin-vue (light/dark).
- **Value:** Accessibility and preference; brand flexibility for owners.
- **Size:** Medium (tokens are centralised; every surface needs a contrast pass; artwork-led pages need care).
- **Principles:** Conflicts with the current visual contract ("dark theme"); Justin's call.
- **Opinion:** Not for launch; keep the door open by never hard-coding dark assumptions outside tokens (the lint already does most of this).

### F-16.5 Custom CSS or owner branding
- **What:** Server-wide custom CSS, a login message, a logo.
- **Who has it:** Jellyfin (branding custom CSS, login disclaimer, splash), Emby.
- **Value:** Loved by tinkerers; a support burden.
- **Size:** Small technically, but needs a CSP nonce or a served stylesheet route.
- **Principles:** Cuts against the fixed design system.
- **Opinion:** I would not add CSS. A server name and optional sign-in note are enough.

### F-16.6 Hosting under a sub-path (Base URL)
- **What:** Serve the whole server at `https://host/portico/`.
- **Who has it:** Jellyfin, Navidrome, Audiobookshelf.
- **Value:** High for owners with one hostname behind a reverse proxy.
- **Size:** Medium (Vite `base`, router base path, API prefix in `HttpLocalApi`, server prefix handling, manifest and icon URLs, CSP unaffected).
- **Opinion:** Likely to be asked for in the first week after release.

### F-16.7 Keyboard-only "TV mode" / spatial navigation and gamepad
- **What:** Arrow-key navigation across the whole UI, a 10-foot layout, gamepad mapping.
- **Who has it:** Jellyfin (focusManager, gamepadtokey, tv layout).
- **Value:** Lets a browser on a TV, an HTPC, a Steam Deck or a console browser use Portico; a route to webOS/Tizen.
- **Size:** Large.
- **Principles:** The navigation contract assigns TVs to native focus engines.
- **Opinion:** Skip unless webOS/Tizen become targets; then it is the main cost of those ports.

### F-16.8 Per-page document titles and navigation announcements
- **What:** "Dune — Portico" in the tab, history and bookmarks; a polite live-region announcement on navigation; a Skip to content link.
- **Who has it:** Jellyfin (titles), jellyfin-vue (title composable).
- **Size:** Small. Required by Portico's own contract.
- **Opinion:** Do it.

### F-16.9 Blur placeholders for artwork
- **What:** A tiny blurred preview (BlurHash or ThumbHash) or at least a dominant colour per image, shown until the image decodes.
- **Who has it:** Jellyfin, jellyfin-vue (decoded in workers), Plex.
- **Size:** Medium (server computes at artwork install, ships ~30 bytes per card; client decodes off the main thread). Backlog already plans a dominant colour.
- **Opinion:** A dominant colour gets most of the benefit for a fraction of the work and payload.

### F-16.10 Hover/idle preloading of the next screen
- **What:** Fetch the route chunk (and optionally the document) when a card is hovered or focused.
- **Who has it:** TanStack Router supports it natively; Portico has the switch on but nothing behind it.
- **Size:** Small for chunks (idle preload), medium for data.
- **Opinion:** Do the chunk half.

### F-16.11 Remote control of a web session
- **What:** Another device (the phone app) drives this browser tab: navigate, play, show a message.
- **Who has it:** Jellyfin (general commands over the WebSocket to any client).
- **Size:** Medium; mostly auditor 13's domain. The web would need an input manager to receive commands.
- **Opinion:** Note only.

### F-16.12 A "new version available — reload" prompt
- **What:** The open tab notices the server now serves a different web build and offers to reload.
- **Who has it:** Navidrome (`serverStart` event carries the version), PWAs via the service-worker update flow.
- **Size:** Small: the build id is already written next to the web files (`portico-build.json`, `Dockerfile:49`) and `/v1/system` reports a version; compare on reconnect or on the first failed chunk.
- **Opinion:** Do it together with B-16.9.

### F-16.13 Client error reports to the owner's server
- **What:** A bounded, rate-limited "this page crashed" report (code, route, build, browser) stored in the server's diagnostics.
- **Who has it:** Jellyfin (`ClientLog` endpoint), Streamyfin (Sentry).
- **Size:** Small to medium.
- **Principles:** Must stay on the owner's server; never Hosted.
- **Opinion:** Useful once there are users who are not the owner.

### F-16.14 A host abstraction for desktop and TV wrappers
- **What:** One documented interface a wrapper implements (native player, fullscreen, media keys, tray, credential store, downloads, exit), with feature flags.
- **Who has it:** Jellyfin (`NativeShell.AppHost`, 28 `AppFeature`s), jellyfin-vue (`tauri-runtime` package).
- **Size:** Medium to design; pays back when the desktop companion starts.
- **Opinion:** Decide the companion's architecture first (Open questions).

### F-16.15 Declared browser support with an "unsupported browser" screen
- **What:** A support matrix in the docs, enforced build targets, and a plain HTML message for browsers below it.
- **Who has it:** Jellyfin (browserslist, es-check, eslint-compat).
- **Size:** Small.
- **Opinion:** Do it.

### F-16.16 Rendering tests (DOM), end-to-end tests and automated accessibility checks
- **What:** Real React rendering in tests, a handful of browser journeys, axe-core on key screens, optional screenshot diffs of the gallery.
- **Who has it:** Audiobookshelf (Cypress component), jellyfin-vue (Storybook), none of them thorough.
- **Size:** Medium.
- **Opinion:** High value given how much behaviour is in effects.

### F-16.17 ESLint with the React hooks rules
- See A-16.51. **Size:** Small. **Opinion:** Do it.

### F-16.18 Find-in-page and "select all" semantics for huge lists
- **What:** Browser find cannot see virtualised rows; Jellyfin's paginated lists can be searched per page. Portico has a library search field, which is the right substitute; "Select all" covers only what is on screen (`Library.tsx:386`).
- **Opinion:** Note only; the bulk-job path presumably covers "everything matching this filter".

### F-16.19 Web Workers for heavy client work
- **What:** Moving decode work (blur previews, audio decode, large JSON validation) off the main thread.
- **Who has it:** jellyfin-web (blurhash worker, libass worker), jellyfin-vue (Comlink workers).
- **Portico:** No `Worker`, `AudioWorklet` or `OffscreenCanvas` anywhere in `web/src`.
- **Opinion:** Mostly auditor 17's topic (audio engine); see "Outside my domain".

### F-16.20 Text-size friendliness
- **What:** Type scale in `rem` so the browser's default font size setting is respected (zoom already works).
- **Who has it:** MUI/Vuetify defaults are rem-based.
- **Size:** Small to medium (tokens generate the CSS).
- **Opinion:** Cheap now, annoying later.

## Possible bugs and risks in Portico

Confidence scale: **Confirmed by reading** (the code plainly does this), **Probable** (follows from the code plus documented platform behaviour; needs a runtime check), **Speculative**. None of these was reproduced.

### B-16.1 The Back-restore memory for library lists is probably never filled when you leave the library
- **What goes wrong:** Scroll deep into a library, open a title, press Back. The design (PERF-S04) is that the list repaints instantly at the same place from an in-memory window. I believe nothing is saved, so Back starts a fresh list from the top and the router's scroll restoration can only reach as far as the first page of placeholders.
- **Evidence:** The windowed collection is created in `LibraryScreen` and disposed in that component's effect cleanup (`web/src/app/browse.ts:69`, called from `web/src/screens/library/Library.tsx:114`). The child `EngineResults` saves the window in *its* unmount cleanup by reading `collection.itemAt(i)` (`Library.tsx:371-385`). When React removes a subtree it runs effect cleanups parent first, then children. So the parent's `collection.dispose()` — which clears every page (`core/collections/windowed.ts:316`, `current.pages.clear()`) — runs before the child reads. `itemAt` then returns `undefined`, the loop collects zero items, and `saveListWindow` returns early on an empty window (`web/src/app/list-position.ts:48`). The memory does work when only the list key changes inside the same mount (sort, filter, tab), because on updates children clean up first.
- **How the comparison surfaced it:** Asking how each client keeps your place on Back (Jellyfin keeps three views alive in the DOM).
- **Confidence:** Probable. The unit tests cover the pure store only (`web/test/list-position.test.ts`); nothing renders React.
- **Severity:** Medium (a headline scale feature silently off; users lose their place in big libraries).
- **Backlog:** Not listed.
- **Fix direction:** Save from the parent before disposing, or defer disposal by a tick as `useService` does (`web/src/app/content.ts:28-30`).

### B-16.2 Switching profile (or signing in as someone else) in one tab signs other tabs out and erases the browser's saved sign-in
- **What goes wrong:** Tab A and tab B are open as profile P1. In tab A you switch to profile P2. Tab B's next request or scheduled renewal finds the stored credential now belongs to another sign-in family and treats that as "sign-in ended". Its sign-out path then clears the browser-wide "selected viewer" record — which by now is tab A's new P2 record. Tab A keeps working from memory, but after the next reload of any tab the browser is signed out.
- **Evidence:** A published new viewer revokes the old one (`core/viewer-selection.ts:83`, `h.revoke(prior)`). In tab B, renewal throws `session_replaced` with `signOut = true` (`core/server-connections.ts:231`); the refresher calls `onSignedOut` (`core/session-refresh.ts`, `failed()`); `session.tsx:991-999` calls `signInEnded`, which calls `disconnectServer()` (`session.tsx:983-989`, `944-968`), which calls `commit.clear()`; `ViewerSelectionCommit.clear()` writes `null` unconditionally (`core/viewer-selection.ts:87-92`) into the shared IndexedDB store (`web/src/bridge/viewer-selection-storage.ts:42-51`). Tab A's rotation handler only rewrites the record if one with its family is still there (`session.tsx:442`), so it never restores it. By contrast `forgetNativeSession` is correctly conditional on the family (`core/server-connections.ts:147`).
- **Also:** The stated intent is "a tab keeps the viewer it started with until it is reloaded" (`viewer-selection-storage.ts:7-8`); in practice tab B is thrown to the sign-in form within one request or at most ~13 minutes.
- **How surfaced:** The multi-tab question; Jellyfin's static token cannot produce this.
- **Confidence:** Probable (traced end to end by reading; needs a two-tab runtime check on both a direct sign-in and a Portico Account sign-in).
- **Severity:** Medium.
- **Backlog:** Not listed. (Backlog's "Two Apple sign-ins ended without an obvious cause" is a different platform but the same family of problem — a sign-in ended by a path nobody expected.)
- **Fix direction:** Make `clear()` conditional on the stored record's family (compare-and-delete), and have a tab whose family was replaced reload into the new viewer instead of signing out. On plain HTTP note that tabs cannot coordinate at all (per-tab locks).

### B-16.3 Only two of about seventy-five hashed assets are cached as immutable
- **What goes wrong:** Every page load revalidates almost every script, stylesheet and the font with a conditional request.
- **Evidence:** `server/internal/httpapi/installation.go:84-87` sets `no-cache` and upgrades to `public, max-age=31536000, immutable` only when `strings.HasPrefix(name, "assets/index-")`. The current build emits 145 files in `assets/`, of which only `index-*.js` and `index-*.css` match; `session-*.js` (884 KB), `hls-*.js`, `Player-*.js`, `ServerPages-*.js`, every route chunk and `manrope-var-latin-*.woff2` do not. `index.html` preloads 17 script chunks and 3 stylesheets.
- **How surfaced:** The caching-headers question.
- **Confidence:** Confirmed by reading (server code and build output). The latency cost is not measured.
- **Severity:** Medium (performance on every load, worst on HTTP/1.1 and remote links; no correctness impact).
- **Backlog:** Not listed.
- **Fix direction:** Treat every file under `assets/` as immutable (Vite content-hashes all of them).

### B-16.4 WindowedGrid's mapped-scroll mode has discontinuities near the ends
- **What goes wrong:** For lists taller than 8,000,000 px (about 143,000 single-line song rows at 56 px, or about 160,000 posters at six columns), scrolling is mapped. A scrollbar drag or letter jump lands proportionally, then small scrolls move 1:1. Near the end, the remaining real pixels are far fewer than the remaining virtual pixels, so after scrolling the remaining real distance the view jumps to the very end, skipping items in between (they are still reachable by scrolling up from the end). The mirror image happens near the top.
- **Evidence:** `web/src/ui/WindowedGrid.tsx:25` (`MAX_HEIGHT`), `:75-89` (the four-way mapping: `scrolled >= maxScrolled` snaps to `maxVirtual`; `Math.abs(delta) < viewport * 3` moves 1:1).
- **Confidence:** Probable (arithmetic by reading; the dev `WindowedLab` exists to try it).
- **Severity:** Low (only very large flat lists; nothing is unreachable).
- **Backlog:** Not listed.

### B-16.5 WindowedGrid re-creates its ResizeObserver and forces layout on every row change; inline renderers defeat per-page memoisation
- **What goes wrong:** Scrolling costs more main-thread time than the design intends.
- **Evidence:** The measuring `useLayoutEffect` depends on `first` and `last` (`WindowedGrid.tsx:155`), so each time the visible row window moves it disconnects and re-creates the observer and calls `getComputedStyle` + `getBoundingClientRect` synchronously (`:129-154`). Separately, `PageCells` is `React.memo` (`:201`) but receives `renderItem`/`renderPlaceholder` as fresh arrow functions from `EngineResults` on every render (`Library.tsx:431-432`), and `EngineResults` re-renders whenever the first visible index changes (`setRange`, `:345`), so every visible card component is re-invoked per scrolled row (the `Card` itself is memoised, which limits the damage to the wrapper level).
- **Confidence:** Confirmed by reading that the work happens; its cost is unmeasured.
- **Severity:** Low to Medium on slow devices.
- **Backlog:** Not listed.

### B-16.6 Regional language and time preferences are applied inconsistently
- **What goes wrong:** A profile set to British or Canadian English, or to a 24-hour clock, sees a mixture: some screens follow the preference, others always use US English or the browser's default formats.
- **Evidence (confirmed by reading):**
  - Seventeen screen modules bind `const t = defaultI18n.t` at module scope, which is the fixed en-US catalogue: `web/src/screens/search/Search.tsx:18`, `saved/Saved.tsx:20` (and `:194` for relative time), `saved/Resource.tsx:21`, `people/Person.tsx:16`, `live/DVR.tsx:15`, `downloads/Downloads.tsx:10`, `together/Together.tsx:11`, `settings/Profiles.tsx:12`, `settings/Security.tsx:9`, `settings/Notifications.tsx:10` (and `:61`), `server/General.tsx:9`, and five files under `screens/account/`. The guide's "Today / Tomorrow / Yesterday" also use it (`screens/live/guide.ts:32-34`).
  - Forty-three files call `currentI18n()` during render. That value is a module variable updated in an effect (`web/src/app/i18n.ts:36`), so a component that renders before the effect, or does not re-render after the preference loads, keeps the old catalogue.
  - Dates and times formatted with the browser's locale instead of the profile's: the guide's time ruler (`screens/live/guide.ts:24-25`, no `hour12`), `screens/people/Person.tsx:20`, `screens/library/LibraryViews.tsx:42`, `screens/detail/EpisodeCard.tsx:75`, `screens/server/Maintenance.tsx:14`, `DVRSettings.tsx:10`, `Diagnostics.tsx:139,155`, `Channels.tsx:168`, `screens/together/Together.tsx:259`.
  - Hard-coded English fragments with manual plurals that the string lint cannot see (template literals with interpolation): `screens/server/LiveTV.tsx:59` (`"… channels · … programs"`), `screens/server/Libraries.tsx:284` (`"… files"`), `screens/server/Channels.tsx:110`; and `setError('…')` literals in `web/src/app/session.tsx:262, 286, 300, 508, 962, 998`. `app/libraries.ts:43-53` (`libraryKindLabel`) returns English nouns.
  - In the formatter itself: byte units are fixed English abbreviations (`i18n/src/index.ts:223`) and `dateTime` joins date and time with `", "` (`:184`), both against A11Y-13.
- **How surfaced:** Jellyfin's single `globalize` entry point versus Portico's three.
- **Confidence:** Confirmed by reading; visible effect today is limited to spelling ("Favorites"/"Favourites") and 12/24-hour inconsistency.
- **Severity:** Low today, High the day a second language ships (mixed-language screens).
- **Backlog:** Not listed. `lint-strings` baseline is empty for web, which gives false comfort: the lint only inspects JSX text and named props.

### B-16.7 Each open tab holds a long-poll; on plain HTTP several tabs can starve the connection pool
- **What goes wrong:** Browsers allow six HTTP/1.1 connections per server, shared by all tabs. Each signed-in Portico tab keeps one `/v1/events` request open for up to 25 s at a time, continuously, visible or hidden. With several tabs (plus a playing video, which also holds connections), ordinary requests queue behind long-polls and pages feel stuck for seconds. HTTPS with HTTP/2 does not have the limit.
- **Evidence:** `web/src/player/engine.tsx:120` constructs `new EventsClient({http})` with no `openStream`, so `pollOnly()` is always true (`core/playback-v1/events.ts:134`); `start()` is unconditional (`engine.tsx:129`) and there is no visibility check in the loop. The artwork store already budgets "four of six" connections on HTTP/1.1 (`web/src/ui/artwork-store.ts:31-38`), which shows the limit is known, but the event poll is not counted. The long-poll request has no client-side deadline either (`events.ts:195-205`), so a silently dead connection parks the feed until the browser gives up.
- **How surfaced:** Jellyfin's WebSocket does not count against the HTTP connection limit.
- **Confidence:** Probable (well-known browser behaviour; not measured here).
- **Severity:** Medium for the "plain HTTP stays first-class" audience with many tabs; Low otherwise.
- **Backlog:** Not listed.
- **Fix direction:** Pause the feed in hidden tabs when nothing is playing; share one feed across tabs with a `BroadcastChannel` or `SharedWorker` (secure contexts) ; add a deadline a little above `waitSeconds`.

### B-16.8 Most requests have no deadline
- **What goes wrong:** If the server accepts a request and never answers, the screen that issued it shows its loading state indefinitely.
- **Evidence:** `HttpLocalApi.request` → `exchange` → `send` pass only the caller's signal (`core/index.ts:338-420`); `RouteConnection.fetch` adds none (`core/route-connection.ts:137-161`). Examples with no signal: `api.libraries()` for the rail (`web/src/app/libraries.ts:26`), browse capabilities (`web/src/app/browse.ts:31`), `api.system()` and `/v1/direct` after sign-in (`session.tsx:452-468`), credits paging (`web/src/app/detail.ts:139`), and the page fetches of the windowed collection (signal is for abort only, `browse-page-source.ts:34-37`).
- **Confidence:** Confirmed by reading that no deadline exists; how often servers stall is a runtime question.
- **Severity:** Medium (design risk; the backlog's own notes mention a 20-minute planner stall on the demo, which is the kind of event that would expose it).
- **Backlog:** Not listed.

### B-16.9 After a server upgrade, an open tab cannot load new screens and "Try again" cannot fix it
- **What goes wrong:** A tab loaded before an upgrade still runs the old entry script. Opening a screen whose chunk it has not loaded yet requests an old hashed file that no longer exists; the import fails; the route error screen appears. Its "Try again" button resets the boundary and re-renders the same `React.lazy` component, which re-throws the cached failure. Only a manual browser reload helps. Old tabs also keep talking to a newer API with strict parsers (A-16.11).
- **Evidence:** Lazy screens are module-level `React.lazy` wrappers (`web/src/app/router.tsx:17-45`); the error screen calls `reset()` (`web/src/screens/NotFound.tsx:28-36`); missing assets return a JSON 404, not `index.html` (`server/internal/httpapi/installation.go:65-68`); there is no `vite:preloadError` listener and no build-id comparison anywhere in `web/src` (grep).
- **How surfaced:** Navidrome's `serverStart` version event and PWA update flows.
- **Confidence:** Probable (React caches a rejected lazy import; not reproduced).
- **Severity:** Medium (every self-hoster upgrades with tabs open).
- **Backlog:** Not listed.
- **Fix direction:** On a failed dynamic import, reload once (guarded against loops); and/or F-16.12.

### B-16.10 Strict parsers reject whole documents for unknown enum values
- **What goes wrong:** A server that introduces a new value for a presentation field makes an older client's whole page fail with "unreadable response" instead of degrading.
- **Evidence:** `core/home.ts:63` rejects the Home document when `artworkShape` is not `poster|square|landscape`; the same function requires a dozen fields to be present and typed (`:63-72`). `library-content.ts` drops unknown entry *kinds* gracefully but not other enumerations. The Hosted-only web build and the native apps are the clients that meet mixed server versions.
- **Confidence:** Confirmed by reading (latent; no skew exists yet).
- **Severity:** Medium as a design risk for the first post-release protocol change.
- **Backlog:** Not listed.

### B-16.11 A short connection wobble unmounts the current page and discards its state
- **What goes wrong:** When the route layer reports `probing`, `offline` or `identity_mismatch` for more than 2.5 s, the overlay renders a status screen *instead of* the routed page, so the page component unmounts. Unsaved form edits on a Settings/Server page, a scroll position, an open filter panel or a half-typed search are lost when it returns.
- **Evidence:** `web/src/shell/Connection.tsx:22-51`: `if (serverProblem || shown) return <StatusScreen …/>` else `children`. The route layer enters `probing` on any browser `online`/`offline`/connection-change event (`web/src/bridge/server-connections.ts:44-48`, `core/route-connection.ts:96`). `ServerForms.tsx:199` guards only `beforeunload`.
- **Confidence:** Probable.
- **Severity:** Low to Medium (laptops that roam between Wi-Fi access points or toggle a VPN).
- **Backlog:** Not listed.
- **Fix direction:** Keep the page mounted but `inert` and cover it.

### B-16.12 No declared browser floor; older engines degrade silently
- **What goes wrong:** On a browser without `:has()` the card focus ring disappears entirely, because the card's own outline is removed and the replacement relies on `:has(:focus-visible)` (`web/src/ui/Card.module.css:2-5,9`). Without `color-mix()` every surface defined through it (`web/src/ui/base.css`, ten declarations there and more in component styles) becomes invalid at use and falls back to transparent. The JavaScript target is Vite's default with no `build.target`.
- **Evidence:** as cited; `web/src/app/polyfills.ts:5-10` shows the team does expect some older TV browsers.
- **Confidence:** Confirmed by reading (CSS semantics); which real devices are affected is unknown.
- **Severity:** Low if the matrix is "current evergreen"; Medium if TV browsers or old iPads are expected to work.
- **Backlog:** Not listed.

### B-16.13 The signed-in shell has no main landmark and no skip link
- **What goes wrong:** Keyboard and screen-reader users must pass through the whole rail (every library) on each page; there is no "main" region to jump to.
- **Evidence:** `web/src/shell/Shell.tsx:169-178` uses `<div className={s.main}>` / `<div className={s.content}>`; no `<main>`, `role="main"` or skip link exists in the shell (grep). `ServerlessShell` and `AuthFrame` do use `<main>`. The contract requires a first-focusable Skip to content link (A11Y-4.11).
- **Confidence:** Confirmed by reading. **Severity:** Medium (WCAG 2.4.1). **Backlog:** Not listed.

### B-16.14 Every page has the same document title; navigation is not announced
- **Evidence:** No assignment to `document.title` in `web/src` (grep); `web/index.html:8` is the only title. Contract A11Y-4.3 requires a route announcement.
- **Confidence:** Confirmed by reading. **Severity:** Low to Medium (WCAG 2.4.2; also hurts tabs, history and bookmarks for everyone). **Backlog:** Not listed.

### B-16.15 A card's accessible name is the title only; watched and progress state are not conveyed with it
- **What goes wrong:** A screen reader hears "Dune, link". The caption ("S1 E3 · 24 min left", year, artist), the watched tick and the progress bar are separate or mouse-only: the tick is a `<span title>` (`web/src/ui/Card.tsx:100`), the link is labelled `aria-label={title}` (`:91`) which overrides its content, and Play/More are `tabIndex={-1}`.
- **Confidence:** Confirmed by reading. **Severity:** Medium for screen-reader users (contract A11Y-2.1 "current value/state"). **Backlog:** Not listed. The Play/More keyboard shortcuts (`M`, `P`) are a reasonable design and are described once in the shell.

### B-16.16 Virtualised grids do not tell assistive technology their size or position, and focus can fall out
- **Evidence:** `WindowedGrid` renders a `div` with optional `role="region"` (`web/src/ui/WindowedGrid.tsx:183`); no `aria-setsize`/`aria-posinset`/row count. A focused card that scrolls beyond the overscan window is unmounted; nothing restores focus (contract A11Y-4.7 asks for a deterministic fallback).
- **Confidence:** Confirmed by reading (attributes absent); focus loss is Probable.
- **Severity:** Low to Medium. **Backlog:** Not listed.

### B-16.17 Smaller accessibility defects in primitives
- `TextArea` and `Select` render help/error text with an id but never reference it with `aria-describedby` (only `Input` does): `web/src/ui/Field.tsx:43-50, 53-62` versus `:31`. Confirmed by reading. Low.
- `Segmented` falls back to the raw option id as the accessible name (`aria-label={o.label ?? o.id}`, `web/src/ui/Tabs.tsx:36`). Confirmed. Low.
- Type sizes are fixed pixels (`web/src/ui/tokens.css`, `base.css` body `font-size: 15px`), so a larger browser default font size has no effect (zoom works). Confirmed. Low; contract A11Y-7.1 is arguably not met.
- No `forced-colors` / `prefers-contrast` handling anywhere (grep). Low.

### B-16.18 The strict style CSP probably blocks the scroll-lock stylesheet that Radix dialogs inject
- **What goes wrong:** Radix Dialog uses `react-remove-scroll`, which creates a `<style>` element at runtime to lock body scrolling and compensate for the scrollbar. The server's policy is `style-src 'self'` with no nonce, which blocks script-created `<style>` elements. Expected effect: console CSP violations each time a modal opens, the page behind a dialog can still be scrolled by keyboard or scrollbar, and a small layout shift where the scrollbar compensation is missing. The Vite dev server sends no CSP, so development would not show it.
- **Evidence:** `server/internal/httpapi/router.go:1242`; the built `session-*.js` contains the injector (`document.createElement('style')` … `__webpack_nonce__`), and `web/src/ui/Dialog.tsx` uses `RDialog` from Radix. No nonce is configured anywhere (grep for `nonce` in `web/src` is empty).
- **Confidence:** Probable (needs a look at the console on a server-served build).
- **Severity:** Low. **Backlog:** Not listed.

### B-16.19 The browse-capabilities cache never expires and is not keyed by a revision
- **What goes wrong:** A library's sort/filter vocabulary is fetched once per (sign-in, library, pivot) and kept in a module-level map for the life of the page. If the server's answer changes (an upgrade, a library setting, facets appearing after the first scan) the tab keeps the old vocabulary until reload. This is the one client cache I found that is neither time-bound nor revision-keyed, against the "memos are keyed by exact revisions" principle. Entries from earlier sign-ins are never removed.
- **Evidence:** `web/src/app/browse.ts:21-36`.
- **Confidence:** Confirmed by reading. **Severity:** Low. **Backlog:** Not listed (Backlog does mention "Nothing to filter by yet" on show libraries, which this cache could prolong after a fix lands server-side).

### B-16.20 Failed list pages ignore `Retry-After` and retry on a fixed 5 s clock
- **Evidence:** `core/collections/windowed.ts:118, 242`: an errored page is eligible again after `retryAfterMs` (default 5,000) on the next `ensureRange`, whatever the server's 429/503 said. Home and artwork do honour `Retry-After`.
- **Confidence:** Confirmed by reading. **Severity:** Low (retries happen only as the user scrolls). **Backlog:** Not listed.

### B-16.21 Paged admin lists keep an old cursor when their inputs change
- **Evidence:** `useCursorRead` keeps a stack of cursors in state and never resets it when `deps` change (`web/src/admin/cursor-read.ts:11-20`). `screens/server/Access.tsx:92` passes `[api, accountId]`; if the same component instance is reused for another account while on page 2, the next request sends account B with account A's cursor.
- **Confidence:** Speculative (depends on whether the component is remounted per account). **Severity:** Low. **Backlog:** Not listed.

### B-16.22 Credits paging can double-append and hides failures
- **Evidence:** `web/src/app/detail.ts:127-145`: `busy` is a ref reset by an earlier request's `finally`, so after switching titles mid-request a second identical page request for the new title can start before the first answers and both append; errors are swallowed (`() => {}`), leaving "more" doing nothing with no message.
- **Confidence:** Probable for the silent failure (by reading); Speculative for the double append. **Severity:** Low. **Backlog:** Not listed.

### B-16.23 React hook dependency rules are not enforced by any tool
- **What:** A latent-bug factory rather than one bug: 320 effects, 76 suppressions of a rule that is not installed, StrictMode off, and no rendering tests. B-16.1 is the kind of defect this combination lets through.
- **Evidence:** `web/package.json` (no eslint), `grep -c exhaustive-deps` = 76.
- **Confidence:** Confirmed by reading. **Severity:** Medium as a process risk. **Backlog:** Not listed.

### B-16.24 Dev-only screens are emitted into the production web folder
- **Evidence:** A-16.53. The routes are unreachable in production, so this is weight and tidiness, not exposure (the chunks are plain JavaScript anyone could fetch by guessing the hashed name; they contain fixture data only as far as I read).
- **Confidence:** Confirmed (source + local build). **Severity:** Low. **Backlog:** Not listed.

### B-16.25 CI and the shipped image build the web with different Node versions
- **Evidence:** `.github/workflows/ci.yml:76` (22.22.2) versus `Dockerfile:30` (`node:24-bookworm`, unpinned minor). With TypeScript 7 and Vite 8 both new, "tested on 22, shipped from 24" is an avoidable variable.
- **Confidence:** Confirmed by reading. **Severity:** Low. **Backlog:** Not listed.

## Where Portico is stronger

- **Scale-first lists.** The sparse windowed collection with revision-pinned pages, abort, LRU and stale-revision recovery is beyond anything in jellyfin-web, which paginates or loads everything.
- **One shared, tested headless core** with authored presentation structure, identical in the web and native repositories today. No comparison project shares this much behaviour across platforms.
- **Server-authored documents** make six clients consistent and Home a single request.
- **A real error model**: stable codes in, one presenter out, one truthful recovery action, cancellations silent, raw server text never shown — and lints that keep it that way.
- **Careful failure behaviour**: foreground-only retries with jitter, server `Retry-After` floors, generation fences and `AbortController`s on the important paths, an artwork scheduler that backs off without holding slots.
- **Security-minded session design**: short-lived rotating tokens, cross-tab renewal coordination, encrypted credential storage, pinned server identity, strict CSP — with a worked-out plain-HTTP fallback.
- **Design-system discipline** enforced by a build-failing lint (colour, font, z-index, undefined variables, spacing grid), and tokens shared with native.
- **i18n foundations**: ICU plurals and named arguments, type-checked message IDs, UI language separate from formatting locale, regional English derived by rule so it cannot go stale, glossary lint.
- **Real URLs** with validated, bounded search params; cards are real links.
- **Tiny dependency surface** and a 139 KB stylesheet.
- **Optimistic concurrency and idempotency** visible in the client (revisions, idempotency keys).
- **Preference registry with a consumer check**, so a setting cannot exist without something reading it.
- **Depth of unit tests** on client logic (290 test files across web and core).

## Doc/code disagreements

1. **Search text in the URL.** `navigation-and-input/contract.md` (NAV item 8) and `decisions.md` R14 say search text is excluded from route URLs and history; `web/src/app/router.tsx:118-120` and `screens/search/Search.tsx:69-74` put `q` in the address. The router file's own header comment (`router.tsx:51`, "Search text is deliberately not a route parameter") contradicts the code seventy lines below it.
2. **Event feed transport.** `core/playback-v1/events.ts:1-12` describes SSE with a long-poll fallback and says `readEventStream` "produces these on web"; the web never passes `openStream` (`web/src/player/engine.tsx:120`), so it is long-poll only. The same header says features "never open their own streams", while `core/group-session.ts:167` and `core/remote-playback.ts:202` open their own.
3. **"A tab keeps the viewer it started with until it is reloaded"** (`web/src/bridge/viewer-selection-storage.ts:7-8`) versus the refresh path that signs the tab out on `session_replaced` (B-16.2).
4. **Accessibility contract versus shell.** A11Y-4.11 (Skip to content), A11Y-4.3 (route announcement), A11Y-15 (RTL mirroring), A11Y-13 (no hand-authored separators or units; locale-aware byte sizes), A11Y-12.8 (no concatenated fragments) are not implemented or are violated at the places cited in B-16.6 and B-16.13/14. The contract is marked "approved … reconciled 5 September 2026".
5. **`web/README.md`.** "Known gaps: Hosted account workspace (`/account`…) is not built" — the workspace exists (`web/src/screens/account/*`, route at `router.tsx:182`). "`npm run check` — regenerate tokens, design-boundary lint, tsc" omits the string lints, preference check and tests that the script actually runs (`web/package.json`). The README's "Compact layout (≤720px)" is accurate.
6. **06 Clients says contracts are in use soon** ("Client-core is moving onto it"); no import exists yet (A-16.12). Accurate as intent; worth stating plainly that it has not started.
7. **Polyfill comment versus effective support.** `web/src/app/polyfills.ts:5-10` targets Chromium < 116 and Safari < 17.4 "older TV browsers"; the CSS requires features (`:has`, `color-mix`) that set a different floor, and no document states one (B-16.12).
8. **`defaultPreload: 'intent'`** (`router.tsx:195`) implies hover preloading; nothing is preloaded (A-16.19).
9. **`eslint-disable` comments** (76) imply ESLint is part of the checks; it is not installed (A-16.51).
10. **`HomeRow.cacheTtlSeconds`** is required by the parser (`core/home.ts:16,65,77`) and never used by any client; under "no time-based caches" the field looks like a leftover on the wire.
11. **Backlog item "Web: `web/src/bridge/profile-vault-crypto.ts` is unused; remove it"** is still true (the file is present, 28 lines).

## Open questions for Justin

1. **Browser support.** Is the web app "current evergreen browsers only", or should it work in TV browsers, on older iPads, in Firefox ESR? Whatever the answer, do you want it written down and enforced (build targets plus an "unsupported browser" page)?
2. **TVs that only run web apps.** LG webOS and Samsung Tizen have no React Native path. Are they out of scope for good, or should the web app keep a door open (spatial navigation, a host abstraction)?
3. **Desktop companion.** Is it a wrapper around this web app (Electron/Tauri with a native player), or a separate native app? If a wrapper, should a host interface be designed now in `bridge/`?
4. **Live refresh.** Do you want open screens to update from server events (watched state, new items), on the revision model? It changes how "fresh" feels more than anything else here.
5. **Back and reload speed versus freshness.** Is painting the last document instantly (keyed by revision) and revalidating acceptable under the "no time-based caches" rule?
6. **Search in the URL.** Keep the linkable `?q=` (and amend the contract and decision R14), or remove it (privacy of search terms in history)?
7. **Multi-tab intent.** When a profile is switched in one tab, should other tabs follow to the new profile, stay on the old one, or show "this tab's profile changed — reload"?
8. **Sub-path hosting** (`/portico` behind a reverse proxy): in or out for 1.0?
9. **Translations.** When more languages come, who translates, in what tool, and are outside translation contributions allowed despite "no external code contributions"?
10. **Light theme / accent / branding.** Permanently out, or "later"? (It affects whether RTL and theming debts are worth preventing now with lint rules.)
11. **Service worker.** Do you want an installable, precached web app on HTTPS, accepting that plain-HTTP users will not get it?
12. **Tooling.** Are you willing to add ESLint (hooks and a11y rules only) and a small browser test suite, given the otherwise dependency-light stance?
13. **Contracts migration.** Should the generated decoders replace the hand-written bounded parsers, or only the types? And is unknown-enum tolerance a rule you want stated?
14. **Client crash reports to the owner's server**: wanted, or does it conflict with the product's quietness?

## Evidence boundary

- **Nothing was executed.** No build, test, lint, dev server, browser, device or network request to any Portico system. Bundle sizes come from an existing local `web/dist` (9 Oct), which may differ from the Docker build (which disables source maps and may be a different commit); gzip figures were computed from those files.
- **Runtime-dependent findings** are marked Probable or Speculative: B-16.1 (depends on React's parent-before-child cleanup order on unmount, which is documented behaviour but was not observed here), B-16.2 (two-tab behaviour), B-16.4 (scroll arithmetic), B-16.7 (connection limits), B-16.9 (lazy-import failure caching), B-16.11, B-16.18 (CSP enforcement against script-created styles).
- **Performance** claims (first load, Home with a thousand cards, scroll cost, conditional-request latency) are reasoning from structure, not measurements. No device classes were tested; the brief notes performance targets are not yet established.
- **Accessibility** findings are from markup and CSS only. No screen reader, keyboard walk-through, contrast measurement or zoom/reflow check was done.
- **Coverage gaps:** the server console screens, the Channels guide's own virtualisation, the player and audio engine, most of `core/presentation`, the React Native consumers of client-core, and Hosted's web serving were not reviewed in depth. jellyfin-web was read selectively (it is large); statements about it describe the files cited, not every legacy page. jellyfin-web's bundle size was not measured. Plex, Emby and Infuse were not consulted.
- **Comparison repos are snapshots** (shallow clones, October 2026); upstream may differ.

## Outside my domain (noted in passing)

- **Audio decoding appears to run on the main thread.** `web/src/bridge/audio/**` (FLAC/MP3/Ogg decoders, resampler, time-stretch) and `bridge/DecodeAudioRender.ts` (`setInterval(…, 50)`) use no `Worker` or `AudioWorklet` anywhere in `web/src`. For auditor 17: check jank during library scrolling while music plays, and behaviour in background tabs where timers are throttled.
- **Hosted-only web requires HTTPS on the server.** An HTTPS page cannot call an HTTP server (mixed content), so web.getportico.tv only works with servers that have a certificate. For the networking auditor.
- **`/v1/events` consumers that open their own streams** (`core/group-session.ts`, `core/remote-playback.ts`) add to the per-tab connection count in B-16.7. For auditor 13.
- **Dev sign-in path.** `devE2ESignIn` is compiled out of production web builds by `import.meta.env.DEV` (`web/src/app/session.tsx:726`); whether the server endpoint it calls exists in release builds is a question for auditor 15.
