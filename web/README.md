# portico-web

Portico's web app. The server serves this build on its own port, and Hosted
serves a Hosted-only build of it at web.getportico.tv.

It shares code with the other clients through:

- `packages/client-core`: the headless core (services, contracts, parsing).
- `packages/design`: design tokens, compiled into `src/ui/tokens.css`.
- `packages/i18n`: strings and message formatting.

`src/bridge/` holds the browser transport and storage adapters.

## Running it

1. Start a development server (never against a server whose data you care about):

   ```bash
   (cd server && go build -o ../.scratch/portico-server ./cmd/server)
   PORTICO_DEV_DIR=/path/to/a/copied/state server/dev/run-demo.sh   # listens on :32500
   ```

2. Start the web app:

   ```bash
   cd web && npm install && npm run dev
   ```

   Vite serves http://127.0.0.1:5173 and proxies `/v1` and `/v2` to
   `127.0.0.1:32500`.

3. Before committing:

   ```bash
   npm run check    # regenerate tokens, design-boundary lint, tsc
   ```

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

## Known gaps

- Hosted account workspace (`/account`: profiles, sessions, security,
  invitations) is not built; dev testing is direct-sign-in only.
- Remove library is disabled pending a server-side confirmation flow.
- The player has no explicit ended/replay state; it returns to the previous
  screen when the queue drains.
- The library header crowds on the narrowest phones when a library has more
  than four views.
- Inventory file browser in Server → Libraries shows sources only, not a
  directory tree.
