# 5. Personal state and jobs

Personal state is per viewer (profile), revision-fenced, and receipted. Small
writes go straight at items or containers; large or mixed sets go through bulk
jobs; files for offline go through downloads. History is a read, not a second
write path.

## Watched, favorites, watchlist

One item: `PUT /v1/items/{id}/personal-state (openapi.yaml, operationId
setItemPersonalState)` with a mandatory `expectedRevision` fence and an
`operationId` idempotency key. A mismatch answers `409` with the current state
beside the error — re-read, re-plan, retry:

<!-- example: personal-mutation-request file=openapi.yaml operationId=setItemPersonalState direction=request schema=@request /v1/items/{id}/personal-state put -->
```json
{
  "operationId": "op_01J6X7QZ9MAB0CDEF123456789",
  "expectedRevision": 41,
  "watched": true
}
```

Up to 200 intents at once: `PUT /v1/items/personal-state:batch
(search-people.openapi.yaml, operationId setPersonalStateBatch)`. Each row
applies under the same conflict rules independently — one rejected row never
rolls back the rest — and `expectedRevision` per row is optional (omit to apply
to whatever the row is at now). The batch carries the retry receipt: replaying
the identical body under the same `operationId` returns the first outcome:

<!-- example: personal-batch-request file=search-people.openapi.yaml operationId=setPersonalStateBatch direction=request schema=#/components/schemas/PersonalBatchRequest -->
```json
{
  "operationId": "batch_01J6X7QZ9MAB0CDEF123456789",
  "items": [
    { "itemId": "item_0001", "watched": true },
    { "itemId": "item_0002", "favorite": true }
  ]
}
```

Favorites, watchlist, and rating ride the same mutation shape (`favorite`,
`watchlisted`/`watchlist`, `rating`, `progressSeconds`) — there are
no separate favorite or watchlist resources. Whole containers (show, season,
album, book) use `GET /v1/containers/{kind}/{id}/personal-state
(jobs.openapi.yaml, operationId get_container_personal_state)` (revision plus
visible-member watched/unwatched counts) and `PUT
/v1/containers/{kind}/{id}/personal-state (jobs.openapi.yaml, operationId
set_container_personal_state)` (one watermark write; explicit item intent wins,
inheritance requires `added_at <= watermark`, later items stay unwatched):

<!-- example: personal-container-write file=jobs.openapi.yaml operationId=set_container_personal_state direction=request schema=@request /v1/containers/{kind}/{id}/personal-state put -->
```json
{
  "expectedRevision": 3,
  "watched": true
}
```

<!-- example: personal-container-read file=jobs.openapi.yaml operationId=get_container_personal_state direction=response status=200 schema=@response /v1/containers/{kind}/{id}/personal-state get 200 -->
```json
{
  "revision": 4,
  "watched": true,
  "watermark": "2026-09-01T12:00:00.000Z",
  "watchedCount": 11,
  "unwatchedCount": 2
}
```

Reads: `GET /v1/personal-history (openapi.yaml, operationId getPersonalHistory)`
and `POST /v1/personal-history/actions (openapi.yaml, operationId
mutatePersonalHistory)`. Removing from Continue Watching, per-item quotas, and
a `personal-state:apply` selector endpoint are **not specified**.

## Bulk jobs

`POST /v1/jobs (jobs.openapi.yaml, operationId create_job)` captures a
selection and enqueues one of `personal-state | playlist-add | collection-add
| metadata-edit | refresh | trash`. The selector is the tagged union
(`container` with `kind=show|season|album|artist|book|collection|playlist|library`
plus `id`; `items` with 1–200 `ids`; `query` with `libraryId`, `pivot`, and the
browse expression `filter`/`sort`):

<!-- example: personal-job-request file=jobs.openapi.yaml operationId=create_job direction=request schema=#/components/schemas/JobRequest -->
```json
{
  "operationId": "job_01J6X7QZ9MAB0CDEF123456789",
  "command": "personal-state",
  "selector": {
    "items": { "ids": ["item_0001", "item_0002"] }
  },
  "args": { "watched": true }
}
```

Creation answers `202` with the durable job; `totalKnown` is false until
immutable membership capture finishes. `GET /v1/jobs/{id} (jobs.openapi.yaml,
operationId get_job)` polls it and `GET /v1/jobs/{id}/failures
(jobs.openapi.yaml, operationId get_job_failures)` pages at most 100 visible
failures:

<!-- example: personal-job-response file=jobs.openapi.yaml operationId=create_job direction=response status=202 schema=#/components/schemas/Job -->
```json
{
  "jobId": "job_01J6X7QZ9MAB0CDEF123456789",
  "command": "personal-state",
  "state": "running",
  "total": 2,
  "totalKnown": true,
  "done": 1,
  "failed": 0
}
```

A single `operations` resource with progress events, selection preview tokens,
`exclude`/`range`/`asOf` selector modifiers, and cancel verbs are **not
specified**.

## Downloads

Offline starts with preparations: `POST /v1/downloads/preparations
(downloads.openapi.yaml, operationId createDownloadPreparations)` (exactly one
of `mediaId`, `mediaIds` ≤ 100, `containerId`, or `nextAfterMediaId`, plus
`quality`):

<!-- example: personal-preparation-request file=downloads.openapi.yaml operationId=createDownloadPreparations direction=request schema=#/components/schemas/PreparationRequest -->
```json
{
  "operationId": "dl_01J6X7QZ9MAB0CDEF123456789",
  "mediaId": "item_0001",
  "quality": "original"
}
```

Then `GET /v1/downloads/preparations (downloads.openapi.yaml, operationId
listDownloadPreparations)`, `GET /v1/downloads/preparations/{id}
(downloads.openapi.yaml, operationId getDownloadPreparation)`, `POST
/v1/downloads/preparations/{id}/grant (downloads.openapi.yaml, operationId
createDownloadGrant)` (mint the transfer grant), and `GET
/v1/downloads/artifacts/{grant} (downloads.openapi.yaml, operationId
transferDownloadArtifact)` (bytes). Receipts are `POST
/v1/downloads/receipts (downloads.openapi.yaml, operationId
issueDownloadReceipts)` with revalidate/revoke/revocation reads; usage and
settings are `GET /v1/downloads/usage (downloads.openapi.yaml, operationId
getDownloadUsage)` and the settings pair. Request-style offline (`POST
/v1/downloads/requests (download-requests.openapi.yaml, operationId
create_download_request)`, `GET /v1/downloads/requests/{id}
(download-requests.openapi.yaml, operationId get_download_request)`) is the
newer queue for the same lifecycle. Rolling subscriptions (`mode=all|unwatched|
next` with `maxBytes`), whole-show-in-one-call semantics beyond `containerId`,
and download progress over events are **not specified**.
