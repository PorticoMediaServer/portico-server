# 1. Overview

## Base URL

The server is its own origin. `openapi.yaml` declares `servers: [{url: /}]` — the
LAN address, remote hostname, or `http://localhost:32500` in development. There
is no separate API host. Start with `GET /v1/system (openapi.yaml, operationId
getSystemInfo)`, which is unauthenticated and returns the stable server id,
name, `setupRequired`, `hostedConfigured`, and build fingerprint.

<!-- example: overview-system-response file=openapi.yaml operationId=getSystemInfo direction=response status=200 schema=#/components/schemas/SystemInfo -->
```json
{
  "id": "srv_01J6X7QZ9MAB0CDEF123456789",
  "name": "Harbor",
  "setupRequired": false,
  "hostedConfigured": true,
  "version": "0.9.0",
  "buildId": "1038c2ad"
}
```

## Versioning

Three namespaces are in use (see `server/api/README.md` and the
`openapi.yaml` info block):

- `/v1` is the product surface: identity, catalogue, browsing, personal state,
  metadata, storage, live TV, and the original single-session playback model
  (`POST /v1/playback/sessions (openapi.yaml, operationId createPlaybackSession)`).
- `/v2` is the renderer-independent playback control plane — controllers,
  command lanes, occurrences and queues (for example `POST
  /v2/playback/controllers (openapi.yaml, operationId
  registerPlaybackController)` and `PUT /v2/queues/{id} (openapi.yaml,
  operationId createQueue)`). It is a different model, not a newer spelling of
  `/v1`.
- Capability URLs (`GET /v1/media/{grant} (openapi.yaml, operationId
  getMediaStream)`) are grant-bound byte streams, not REST resources.

There is no `/v3` Portico namespace. Additive evolution rules, `api.level`, and
a deprecation window are **not specified** in the OpenAPI: this is pre-release
software and routes are reshaped rather than deprecated.

## Error envelope

Every JSON error is `{"error": {"code", "message", "retryable"}}`
(`openapi.yaml`, schemas `Error` / `ErrorEnvelope`). `code` is stable and
machine-readable; `message` is safe to display; `retryable` says whether
repeating the identical request could succeed. Some handlers add sibling fields
next to `error` (`current`, `serverId`, `viewerFence`, `currentRevision`,
`fields`, `serverTime`), and the `/v2` control routes wrap the envelope with
`protocolVersion: "2.0"` plus `stage` and `action`. Each operation lists the
codes it can produce under `x-error-codes`.

<!-- example: overview-error-envelope file=openapi.yaml operationId=getItem direction=response status=default contentType=application/json schema=@response /v1/items/{id} get default -->
```json
{
  "error": {
    "code": "not_found",
    "message": "Nothing here matches that address.",
    "retryable": false
  }
}
```

Handle two families explicitly:

- Optimistic concurrency: writes carry a body fence (`expectedRevision` and
  friends). A mismatch answers `409` with a `*_conflict` code and often the
  current state beside `error`. Re-read, re-plan, retry.
- Idempotency: mutations carry a body key (`operationId`, `requestId`, or
  `idempotencyKey`). Replaying the same key returns the original receipt;
  personal-state and saved-resource receipts are replayable for 30 days
  (`receiptLifetimeSeconds: 2592000`). Reusing a key for a different payload
  answers `409`/`422` depending on the family.

The style guide's extended envelope (`detail`, `retry`, `retryAfterSeconds`,
`fields[]`, `current`, `requestId`, fixed status mapping) is **not specified**
in the current OpenAPI.

## Pagination and cursors

Paged reads take `cursor` and `limit` and return `nextCursor`. An empty
`nextCursor` (`""`) means the listing is exhausted; anything else is opaque and
must be sent back verbatim. Never construct or mutate a cursor — an
unrecognised one answers `400 invalid_cursor`. A page may be shorter than
`limit` while still returning a `nextCursor`: rows the viewer may not see are
removed after the page is read, so only an empty `nextCursor` ends the walk.
Composed surfaces re-check the permission fence after composing and answer
`409 stale_continuation` when it moved mid-read; discard the cursor and start
over.

Limits vary by surface and are documented per operation: most catalogue reads
default to 40; `GET /v1/search (openapi.yaml, operationId search)` caps at 40;
structured browse (`POST /v1/libraries/{id}/browse (browse.openapi.yaml,
operationId browseLibrary)`) caps at 100. Clamp-vs-reset behavior differs by
family (catalogue lists reset out-of-range limits to the default; browse,
search and home clamp; console/admin reads reject), so read the operation's
description rather than assuming one rule.

<!-- example: overview-listitems-response file=openapi.yaml operationId=listItems direction=response status=200 schema=@response /v1/items get 200 -->
```json
{
  "items": [
    {
      "id": "item_0001",
      "libraryId": "lib_movies",
      "title": "Harbor Lights",
      "kind": "movie",
      "duration": 6540.0,
      "progressSeconds": 0.0,
      "available": true,
      "addedAt": "2026-09-01T12:00:00.000Z"
    },
    {
      "id": "item_0002",
      "libraryId": "lib_movies",
      "title": "North Ferry",
      "kind": "movie",
      "duration": 5820.0,
      "progressSeconds": 320.5,
      "available": true,
      "addedAt": "2026-09-02T12:00:00.000Z"
    }
  ],
  "nextCursor": ""
}
```

Signed, versioned cursors with a published TTL, `total`/`revision` envelopes,
and `start`+`positionIndex` random access are **not specified** as a uniform
contract: cursor encoding, `total` presence, and window parameters differ per
family (browse and home rows expose their own `revision`/`positionIndex`
shapes — see section 3).

## Idempotency keys

There is no `Idempotency-Key` header in the OpenAPI. Idempotency is per-route
body fields: `operationId` on personal-state writes (`PUT
/v1/items/{id}/personal-state (openapi.yaml, operationId
setItemPersonalState)`), `requestId` on playback session creation (`POST
/v1/playback/sessions (openapi.yaml, operationId createPlaybackSession)`),
`operationId` on bulk jobs (`POST /v1/jobs (jobs.openapi.yaml, operationId
create_job)`), and `idempotencyKey` on layout writes (`PUT /v1/home/layout
(home.openapi.yaml, operationId putHomeLayout)`). Key grammars, replay windows,
and conflict codes (`operation_conflict`, `queue_operation_conflict`,
`operation_expired`, `idempotency_key_reused`) differ per family — read the
operation. A single header, scope rule, `Idempotent-Replayed` response header,
and uniform `422`/`409` mapping are **not specified**.

## Revisions and If-Match

There is no `If-Match` / `If-None-Match` / `ETag`-on-JSON contract in the
OpenAPI and no `412` / `428` statuses. Concurrency is body fenced:
`expectedRevision` (usually integer) on the mutating body, with `409`
`*_conflict` answers. Strong `ETag` handling exists only on a few binary
reads (tile and chapter images). Uniform opaque-string revisions, mandatory
preconditions, and `current`-on-`412` are **not specified**.

## Rate limits with Retry-After

Back-pressure is explicit. `429` with a `Retry-After` header means a rate limit
or admission cap. Documented cases include 20 authentication attempts per
minute per peer (`POST /v1/sessions (openapi.yaml, operationId createSession)`,
`POST /v1/direct/sign-in (openapi.yaml, operationId directSignIn)`), 120
personal writes per minute per profile, queue command/event poll caps (`429
queue_busy` with `Retry-After`), and per-peer/per-account caps on `POST
/v1/quick-connect/review (openapi.yaml, operationId reviewQuickConnect)`
(`rate_limited`). `503` with `retryable: true` means a dependency is
unavailable and the request is worth retrying (`search_unavailable`,
`playback_source_unavailable`, and friends).

<!-- example: overview-rate-limited file=openapi.yaml operationId=reviewQuickConnect direction=response status=default contentType=application/json schema=@response /v1/quick-connect/review post default -->
```json
{
  "error": {
    "code": "rate_limited",
    "message": "Too many attempts. Try again shortly.",
    "retryable": true
  }
}
```

`RateLimit` / `RateLimit-Policy` headers, per-`(client_id, principal)` limits,
published limit tables, and operation-counted quotas are **not specified**.
