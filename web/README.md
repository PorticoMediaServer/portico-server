# Portico Web

Portico's production browser application. Its visual direction grew from the Plex Rhyme exploration, but this package is an independent production implementation with real API adapters, authentication, playback, administration, responsive behavior, accessibility, and failure handling.

## Commands

```sh
npm install
npm run dev
npm run typecheck
npm run lint
npm run test
npm run build
npm run verify:hosted
```

Development defaults to the explicit fixture runtime. Set `VITE_PORTICO_RUNTIME_MODE=bundled` to use the same-origin Portico API or `VITE_PORTICO_RUNTIME_MODE=hosted` with an HTTPS Portico Cloud origin to exercise direct-only remote bootstrap. Production defaults to bundled mode and rejects fixture configuration.

The bundled server overrides `/portico-config.js` with `mode: "bundled"`. `npm run verify:hosted` produces and checks the static Hosted build intended for `web.getportico.tv`; it discovers an authorized server through Hosted Services and then connects to that server directly.

## Boundaries

- Do not introduce a second browser client or import prototype code.
- Do not copy server response types into route components.
- Do not add global, route-specific token systems.
- Render application overlays through the shared portal primitives.
- Keep video cards poster-shaped; music cards are square.
