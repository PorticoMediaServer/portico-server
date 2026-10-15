# portico-web

Portico's web app. The server serves this build on its own port, and Hosted
serves a Hosted-only build of it at web.getportico.tv.

It shares code with the other clients through:

- `packages/client-core`: the headless core (services, contracts, parsing).
- `packages/design`: design tokens, compiled into `src/ui/tokens.css`.
- `packages/i18n`: strings and message formatting.

`src/bridge/` holds the browser transport and storage adapters.

## Running it

Follow [BUILDING.md](../BUILDING.md) to build/run the server with disposable
state. From the repository root, `npm ci` installs the shared workspace and
`npm run dev:web` starts Vite on `http://127.0.0.1:5173`, proxying `/v1` to
`127.0.0.1:32500`. There is no `/v2` playback API.

For a focused type check, use `(cd web && npx tsc --noEmit)`. The broader
`npm run check --workspace portico-web` also regenerates tokens, runs design,
preference and string checks, and runs the web tests; it is not just a type check.

## Supported browsers

Chrome and Edge 111, Firefox 113, and Safari 16.4 (iOS and iPadOS 16.4) or
newer, on any platform. This is the floor the build targets (`browserFloor` in
`vite.config.ts`): the app's syntax is written for these engines and its
styles rely on what they support (colour mixing above all). An older browser
is told so plainly by `browser-check.js`, which runs before the app, rather
than being shown an empty or half-drawn page. Change the floor in both places
and in this paragraph together.

Two features are added where a supported browser lacks them: `AbortSignal.any`
and `AbortSignal.timeout` (some television browsers), and, on a plain-HTTP page,
`crypto.randomUUID`, a tab-local stand-in for Web Locks, and the server identity check's cryptography
(`src/app/polyfills.ts`).

## Layout

| Path | Role |
| --- | --- |
| `src/ui/` | The only place raw colours, fonts and layers may appear. Tokens, base styles, and every primitive (Button, Card, Shelf, Dialog, Field, Settings rows…). |
| `src/app/` | Session (sign-in, restore, viewer selection), router, content helpers, preferences. |
| `src/shell/` | Rail on wide screens; top bar + bottom tabs on phones. |
| `src/screens/` | One folder per surface: auth, home, library, detail, search, saved, live, settings, server (administration). |
| `src/player/` | Playback engine bound to the device queue, Now Playing surfaces, option popovers/sheet, mini bar. |
| `src/admin/` | Console client hooks and label maps shared by the server screens. |
| `docs/administration.md` | The administration information architecture and the rules every settings page follows. |

`npm run lint:hooks` (oxlint, React's hook rules only) fails the check when a
hook is called conditionally or its dependency list is incomplete. A deliberate
exception carries `// eslint-disable-line react-hooks/exhaustive-deps -- why`
on the line it excuses.

`scripts/lint-design.mjs` fails the build if hex/rgb colours, font families or
z-index values appear outside `src/ui` and `src/bridge`. Use semantic tokens
from `base.css` (`--surface-*`, `--text-*`, `--layer-*`) instead.

## Conventions

- Routes are code-based TanStack Router routes with validated search params
  (`src/app/router.tsx`). Library views come from the server's navigation
  contract; the router accepts the full set (`discover`, `browse`, `songs`, …).
- Core services are created with `useService(factory, deps)`, which disposes
  on a deferred tick. The app deliberately does not use React StrictMode
  because core services are not double-effect safe.
- Card and list rows never nest interactive elements; secondary controls use
  `Card`'s overlay or `ListRow`'s `actions` slot.
- Compact layout (≤720px) is decided by `useCompact()`; dialogs become bottom
  sheets and the player's five option boxes collapse to one Options sheet.

## Where the browser draws the picture

The player draws styled (ASS) and image (PGS, VobSub, DVB) subtitles on a canvas over the
video. Some surfaces show the video without the page:

- **Picture in Picture.** Chrome and Edge open a window of the page's own (document Picture in
  Picture), and the video and its subtitle canvas move there together. Elsewhere (Safari) the
  browser's video-only window is used: while it is open the subtitle is asked for the ordinary way
  (text for a styled track the server has as text, otherwise burned into the picture) and drawn on
  the page again when the picture comes back. Firefox's own Picture in Picture toggle is not
  reachable from the page and shows text tracks only.
- **iPhone full screen.** An iPhone cannot make a page full screen; the system's full-screen player
  is used, with its own controls. It shows text subtitles in the system's caption style (the
  player's size and background settings do not apply). A drawn styled track is shown there as
  text where the server has it; an image subtitle, or a styled one that would have to be burned
  in, is not shown until the picture returns to the page, because a new picture would close the
  full-screen player. The phone turns to landscape by itself; the page cannot lock it.
- **Casting.** The television plays its own session and draws text only, so the subtitle chosen
  in the player is sent with the title and the server burns in styled or image subtitles there.
- **Music.** Device-decoded music plays through an `<audio>` element fed from the AudioContext, so
  media keys, headset buttons and the lock screen reach it; where the element cannot be used the
  audio goes straight to the speakers without those controls.

## Administration and integrations

Owner pages are described in [administration.md](docs/administration.md).
The server's [developer guide](../server/api/guide/README.md) describes the same
API used by this app. Source checks and automated tests do not establish browser
layout or native feature parity; use a browser and target devices for acceptance.
