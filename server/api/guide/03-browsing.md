# 3. Browsing

All browsing is already scoped to the viewer: the server filters libraries,
rows, and entries through the membership policy before composing, so a client
must never re-filter. Render the server's `defaultView`, row order, sorts, and
filters rather than choosing its own.

## Libraries

`GET /v1/libraries (openapi.yaml, operationId listLibraries)` lists the
libraries this viewer may see. (The same `operationId` is also declared in
`browse.openapi.yaml`; the `openapi.yaml` entry below is the reference.)

<!-- example: browsing-libraries-response file=openapi.yaml operationId=listLibraries direction=response status=200 schema=@response /v1/libraries get 200 -->
```json
{
  "items": [
    { "id": "lib_movies", "name": "Movies", "kind": "movie", "defaultView": "grid" },
    { "id": "lib_shows", "name": "Shows", "kind": "tv", "defaultView": "grid" }
  ]
}
```

`kind` is `movie|tv|anime|music|audiobook`. Per-library vocabulary (fields,
operators, sorts, quick filters, `queryLimits` including `maximumLimit`) is
published by `GET /v1/libraries/{id}/browse-capabilities (browse.openapi.yaml,
operationId getBrowseCapabilities)`; facet counts come from `GET
/v1/libraries/{id}/facets (browse.openapi.yaml, operationId getLibraryFacets)`.

## Browse and content

Two browse spellings exist. The structured one is `POST
/v1/libraries/{id}/browse (browse.openapi.yaml, operationId browseLibrary)`,
which takes the typed expression tree (`all|any|not` or a
`field`/`operator`/`value` predicate), sorts, `limit` (1–100), `cursor`, and
windowed `range`/`seek`:

<!-- example: browsing-browse-request file=browse.openapi.yaml operationId=browseLibrary direction=request schema=@request /v1/libraries/{id}/browse post -->
```json
{
  "limit": 40,
  "query": { "field": "title", "operator": "contains", "value": "harbor" }
}
```

The legacy cursor-paged row read `GET /v1/libraries/{id}/browse (openapi.yaml,
operationId browseLibrary)` (same `operationId`, different method and
query-param shape) is still registered; prefer the `POST` spelling for
filtered queries.

The response carries the resolved query, entries, page info, and letter anchors:

<!-- example: browsing-browse-result file=browse.openapi.yaml operationId=browseLibrary direction=response status=200 schema=@response /v1/libraries/{id}/browse post 200 -->
```json
{
  "pivot": "items",
  "applied": {
    "query": { "field": "title", "operator": "contains", "value": "harbor" },
    "sort": []
  },
  "entries": [
    { "id": "item_0001", "kind": "movie", "title": "Harbor Lights" }
  ],
  "pageInfo": {
    "start": 0,
    "total": 1,
    "revision": "rev_42",
    "hasMore": false,
    "nextCursor": ""
  },
  "positionIndex": [
    { "key": "H", "index": 0 }
  ]
}
```

For screens a person looks at, prefer the server-composed surfaces — one
request per screen: `GET /v1/libraries/{id}/content (openapi.yaml, operationId
getLibraryContent)` (heading, tabs, sorts, filters, sections; `view` and
`entityId` select grid, show, album, artist, and other sub-surfaces), `GET
/v1/content (openapi.yaml, operationId getGlobalContent)` (`view=home`,
`watchlist`, `favorites`, `playlists`; `view=home` requires exactly
`limit=12`), `GET /v1/items (openapi.yaml, operationId listItems)` (flat
cursor-paged items), `GET /v1/items/{id} (openapi.yaml, operationId getItem)`
(one row with this viewer's watched flag and resume position), and `GET
/v1/items/{id}/detail (openapi.yaml, operationId getItemDetail)` (item,
personal state, action list, metadata blocks, markers; `related=all` opts
non-movie kinds into related rows):

<!-- example: browsing-item-response file=openapi.yaml operationId=getItem direction=response status=200 schema=#/components/schemas/Item -->
```json
{
  "id": "item_0001",
  "libraryId": "lib_movies",
  "title": "Harbor Lights",
  "kind": "movie",
  "duration": 6540.0,
  "progressSeconds": 0.0,
  "available": true,
  "addedAt": "2026-09-01T12:00:00.000Z"
}
```

The composed content envelope is the same shape everywhere:

<!-- example: browsing-content-response file=openapi.yaml operationId=getGlobalContent direction=response status=200 schema=@response /v1/content get 200 -->
```json
{
  "heading": {},
  "scope": {},
  "revision": {},
  "navigation": [],
  "query": {},
  "sorts": [],
  "filters": [],
  "sections": []
}
```

A universal `filter` grammar, `POST …:query` collection reads, `sort=field`,
`q=`, and single-vocabulary booleans are **not specified**: each family keeps
its own filter spelling, and only browse publishes a typed grammar.

## Position anchors

Windowed collections expose random access. In `POST
/v1/libraries/{id}/browse (browse.openapi.yaml, operationId browseLibrary)`,
`range` pins a `start` index against the `pageInfo.revision` the client last
saw, `seek` jumps by prefix, and `positionIndex` returns letter anchors (`key`
is the uppercase first letter, `#` otherwise) present only when the first sort
is title ascending. Home rows carry their own `revision`, `start`, `anchorId`,
and cursor paging (section below). A `start` against a stale revision answers
`409`. Cross-collection `positionIndex` anchors and published sort ladders are
**not specified**.

## Search

`GET /v1/search (openapi.yaml, operationId search)` searches exactly the
libraries this viewer may see under a three-second server deadline (timeout
answers `503 search_unavailable`, never a partial page). Only `q`, `group`,
`cursor`, `limit` (1–40), and `record` are accepted; pass `record=1` on a
committed search — a submit or result open, never a keystroke or continuation.
Canonical people search is `GET /v1/people (search-people.openapi.yaml,
operationId searchPeople)` (`q` 2–128 chars, `limit` 1–100) with credits at
`GET /v1/people/{id} (search-people.openapi.yaml, operationId readPerson)`
(`role=cast|crew|all`, cursor-paged). Search history scoping and recording
rules beyond the `record` flag are per-operation behavior; a unified
search-capabilities document is **not specified** in the cited response
schemas.

## Home and layout

`GET /v1/home (home.openapi.yaml, operationId getHome)` returns the composed
home document (`serverId`, `viewerFence`, up to 64 populated `rows` in
configured order, `hero`, `layout`, `revision`, `generatedAt`). `hero` names the
entry Home opens with (`rowId`, `entryId`): the first entry of Continue Watching
or of Continue Listening, whichever was played more recently; it is absent when
nothing is in progress. Rows declare their own
`kind` (`continue|ondeck|saved|recommendation|community|recent|related`),
`artworkShape` (`poster|square|landscape`), policy state, paging (`entries`,
`total`, `start`, `limit`, `hasMore`, `nextCursor`, `anchorId`, `revision`),
and endpoints. `GET /v1/home/rows/{id} (home.openapi.yaml, operationId
getHomeRow)` pages one row. Layout customization is `GET /v1/home/layout
(home.openapi.yaml, operationId getHomeLayout)`, `PUT /v1/home/layout
(home.openapi.yaml, operationId putHomeLayout)` (body fenced by
`expectedRevision` with an `idempotencyKey`), and `POST
/v1/home/layout/reset (home.openapi.yaml, operationId resetHomeLayout)`:

<!-- example: browsing-layout-request file=home.openapi.yaml operationId=putHomeLayout direction=request schema=@request /v1/home/layout put -->
```json
{
  "expectedRevision": 7,
  "rowOrder": ["continue", "watchlist", "recent_lib_movies"],
  "hiddenRowIds": ["trending"],
  "idempotencyKey": "layout-0001"
}
```

Localisable row titles (`{code, params}` plus English `fallback`), capability
gating (`GET /v1/capabilities`), and a bootstrap bundle are **not specified**.
