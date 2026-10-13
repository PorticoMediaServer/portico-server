# Portico server API — developer guide

Derived from the actual OpenAPI files in `server/api/` (integration base)
and the normative `Spec — API v1 Style Guide.md`. Each endpoint citation names the
file and `operationId` so it can be checked mechanically. Every JSON example is
extracted and schema-validated by `check-examples.mjs` in this directory:

```bash
node server/api/guide/check-examples.mjs
```

Conventions used in every section:

- `METHOD /v1/...` always names a route that exists in the cited OpenAPI file.
  Where the style guide describes behavior the OpenAPI does not define, the text
  says **not specified** instead of inventing it.
- `code` values are the stable machine-readable strings from `x-error-codes`.
- Timestamps in examples use RFC 3339 millisecond UTC; durations on catalogue
  shapes are float seconds (the style guide's integer-millisecond rule is a
  target, not the current wire format — see `01-overview.md`).

Sections:

1. `01-overview.md` — base URL, versioning, errors, pagination, idempotency, revisions, rate limits.
2. `02-authentication.md` — direct sign-in, Portico Account challenge, sessions, refresh, TV code.
3. `03-browsing.md` — libraries, browse and content, anchors, search, Home and layout.
4. `04-playback-v1.md` — sessions, queues and selectors, timeline, media grants, events.
5. `05-personal-state-and-jobs.md` — watched, favorites, watchlist, bulk jobs, downloads.
6. `06-events.md` — the `/v1/events` feed and notifications.
7. `07-keys-tokens-and-scopes.md` — API keys, personal access tokens, scopes, rate and concurrency limits.
8. `08-for-tools.md` — the bounded title query and the "these paths changed" report.
9. `09-webhooks.md` — outbound webhooks: events, payloads, checking the signature, retries.
10. `10-mcp.md` — the built-in MCP server for AI assistants.

Owner playback diagnostics include `GET /v1/admin/playback/route-failures`
(the latest 100 active remembered failures, with expiry and no credentials).
`DELETE /v1/admin/playback/route-failures/{sourceId}` clears all remembered
routes for that file so devices can retry it after a player or driver change.
Both routes require an owner and the admin key scope.

For anime files numbered across seasons, show identification accepts the
provider's official order or a show-owned TMDB episode group. An explicit
`order` such as `tmdb-absolute:official:-12` maps local episode 15 to position 3;
`tmdb-absolute:tmdb-group:<groupId>:-12` applies the same offset to the selected
group. The offset is added to the local number. Local numbers remain unchanged,
and original seasonal coordinates are retained as provenance. Client-core's
`tmdbAbsoluteOrder` constructs this bounded order value. Seasonal files in the
same show use the selected base order without the absolute offset.
