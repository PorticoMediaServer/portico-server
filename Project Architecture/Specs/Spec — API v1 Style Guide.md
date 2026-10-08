# Portico API v1 — style guide (normative)

The conventions every Portico API follows (server and Hosted).

## Amendments to §4

1. **Single version:** all routes are `/v1`. The Media track's `/v3` labels and today's `/v2` plane don't exist in the target.
2. **Server error messages are US English**, and every error carries a stable registered `code`. Clients localise by `code` from the shared catalogue; `message` is only a fallback. User-facing server strings (e.g. Home row titles) travel as `{code, params}` **plus** an English `fallback`, e.g. `{"title":{"code":"row.recently_added_in","params":{"library":"Movies"},"fallback":"Recently added in Movies"}}`.
3. **Tolerant reader on clients:** clients MUST ignore unknown response fields and map unknown open-enum values to a fallback. Strictness stays only for identity, scope, signatures, revisions and URLs (the contract-drift policy). The server stays strict on requests (§4.8.1).
4. **No `null` in responses** (§4.3.3) applies. Clients treat absent and default the same.
5. **Artwork** is a typed object on cards: `{"poster":{"url":"…?w={w}","width":…,"height":…,"blurhash":"…"}, "backdrop":…, "logo":…}`. URLs are versioned and immutable, and accept `w` (clients request the rendered width × device pixel ratio, rounded up to the server's size buckets). The card's `presentation` (`poster|square|landscape|circle`) is declared by the **server** from the kind and context, following Justin's rule.
6. **Rows** (Home, Discover, See all) declare `{id, title (localisable object), presentation, items(card), page}`. No "why featured" text.
7. **Feature capability**: `GET /v1/capabilities` returns per-viewer feature states `enabled | disabled_by_owner | not_permitted | unavailable`; clients hide features that are `disabled_by_owner` or `not_permitted`, and show an explanatory state for `unavailable` only to owners.
8. **Events**: one `/v1/events` endpoint, as SSE plus a long-poll fallback (≤ 55 s), in a realtime class exempt from the 5 s budget. Topics are the union of the API and Media records plus `device.command`, `playback.transfer_offered`, `group.timeline`, `admin.sessions`, `server.notice`.
9. **Selectors** use the tagged union `{"container":{"kind","id"}} | {"query":{…browse body…}} | {"items":{"ids":[…≤500]}}` everywhere: queues, playlists, collections, personal-state apply, downloads, metadata batch edits, save view.
10. **Operations** (`/v1/operations/{id}`): `{id, kind, state: queued|running|succeeded|failed|canceled, progress:{done,total}, result?, error?}` with `operation.updated` events.
11. **Auth for first-party clients now**: short-lived access tokens (15 min) plus rotating refresh tokens bound to the device; device-bound sessions (identity). Refresh works identically over HTTP and HTTPS (Justin, 22 Sep: plain HTTP is first-class; claimed servers may set "Secure connections: Required"). Only email, password and 2FA changes require the current password (+2FA). OAuth 2.1 (PKCE and the device grant; D-API-7) for third parties, member-approvable; personal access tokens replace API keys; API-key creation is disabled until scopes are enforced.
12. **CORS**: bearer routes allow any origin without credentials; setup and sign-in routes are first-party only.
13. **Transport:** HTTP/2 and TLS session resumption enabled on the server's HTTPS listener; JSON `GET`s have ETags; compression is allowed (clients never require `Content-Length`).
14. **Security headers:** every HTML response from the server and Hosted web hosts carries a CSP (script/style self plus the Cast sender origin where needed; no `unsafe-inline` scripts), `frame-ancestors 'none'`, HSTS on HTTPS, `Referrer-Policy: no-referrer` and `X-Content-Type-Options: nosniff` (SEC-13).
15. **Scale:** every route declares a cost class; nothing synchronous is more than `page_sized` (§4.10.4). Collections over big sets support `start` + `positionIndex` (random access and letter jumps) and cursors.

## Source of truth and generation
- A shared Go module `apikit` (usable by server and Hosted): typed route registry (method, path, auth mode and scopes, **access rule**, lane, request/response types, errors, limits, cost class, examples), `apierror`, cursor, idempotency, preconditions, and `apigen`.
- `apigen` emits `portico.openapi.json` (OpenAPI 3.1) plus **TypeScript** into `packages/contracts/` (types, route table, error codes, limits, and small tolerant decoders). Client-core imports from there; hand-written parsers are deleted as each family migrates.
- **Access rule is mandatory** per route (`public | device | viewer(library) | viewer(item, restrictions) | owner | admin(scope)`). The registry refuses to start if a route lacks one, and a test enumerates every route. This is how content restrictions (SEC-02/X-21/BE-SRV-01) become impossible to forget.
- **Golden fixtures:** handler tests record responses into `server/testdata/contracts/*.json` (and Hosted equivalents). `packages/contracts` tests parse every fixture with the generated decoders, and a request-replay test sends every client-core request builder's output through the server router. CI: `.github/workflows/ci.yml` runs Go tests (server, Hosted with a Postgres service, DNS), `npm run test:core`, web check and build, the contract tests, and breaking-change detection (active at release).
