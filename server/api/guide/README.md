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
