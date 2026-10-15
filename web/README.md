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

## Administration and integrations

Owner pages are described in [administration.md](docs/administration.md).
The server's [developer guide](../server/api/guide/README.md) describes the same
API used by this app. Source checks and automated tests do not establish browser
layout or native feature parity; use a browser and target devices for acceptance.
