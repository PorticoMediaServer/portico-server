# 8. Routes for tools: the title query and "these paths changed"

Portico's screens are composed by the server (`03-browsing.md`). Two routes
exist for tools that are not screens.

## The title query

`GET /v1/titles (portico.openapi.json, operationId query_titles)` — scope
`read`. It lists the titles of **one kind** in **one library**, a page at a
time, or finds a title by its id at a metadata provider. The caller's library
access and content restrictions are applied inside the query, as everywhere
else: a page is never filtered after it is cut.

It answers only what an index can answer:

| Parameter | Values |
|---|---|
| `library` | A library id from `GET /v1/libraries (openapi.yaml, operationId listLibraries)`. Required for a list. |
| `kind` | `movie`, `show`, `season`, `episode`, `artist`, `album`, `song`, `book`, `author`, `collection`. Required for a list. |
| `sort` | `title` (default), `added`, `year`. |
| `order` | `asc` (default), `desc`. |
| `yearFrom`, `yearTo` | Only with `sort=year`. |
| `addedAfter`, `addedBefore` | RFC 3339 times; only with `sort=added`. |
| `limit` | 1–100, default 50. |
| `cursor` | `nextCursor` from the previous page. |
| `provider`, `providerId` | A lookup instead of a list: `tmdb`, `tvdb`, `imdb` or `anilist`, and the id there. `library` and `kind` may narrow it. |

A range is accepted only on the key being sorted by; anything else, an unknown
parameter, or a repeated parameter is `400 invalid_request` naming the field.

```json
{
  "items": [
    {
      "id": "kq3Jm0Zr1yQ8b2nWcP4d5A",
      "kind": "movie",
      "title": "The Matrix",
      "libraryId": "movies",
      "year": 1999,
      "addedAt": "2026-10-01T18:22:04.000Z",
      "available": true,
      "durationSeconds": 8160,
      "rating": 8.2,
      "posterUrl": "/v1/metadata/item/kq3Jm0Zr1yQ8b2nWcP4d5A/art/poster?v=3f9a&w=400",
      "providerIds": {"tmdb": "603", "imdb": "tt0133093"}
    }
  ],
  "limit": 50,
  "nextCursor": "eyJrZXlzIjpbInRoZSBtYXRyaXgiXX0.c2ln"
}
```

`available` is `false` while every file of the title is missing from disk.
Details, files and playback are the title's own routes (`GET
/v1/items/{id}/detail (openapi.yaml, operationId getItemDetail)`).

**Walking a whole library.** Follow `nextCursor` until it is absent. The cursor
seeks to the last row you saw; it is not tied to the library's revision, so the
walk finishes even while a scan is adding titles. A title added behind the
cursor is not seen on this walk, one added ahead of it is, and none is returned
twice. A cursor belongs to the query and the caller that made it and lasts 30
minutes (`400 invalid_cursor` otherwise). To pick up what changed since a walk,
ask for `sort=added&order=desc` and stop at the first title you already have.

"Do you have this film?" is one request:

```
GET /v1/titles?provider=tmdb&providerId=603
```

## These paths changed

`POST /v1/libraries/paths-changed (portico.openapi.json, operationId
report_changed_paths)` — scope `library`, owner's key. It is the call a
download manager makes after it imports, upgrades or deletes a file, in place
of asking for a scan of the whole library.

```json
{
  "paths": [
    {"path": "/media/tv/Severance/Season 02/Severance - S02E01.mkv", "change": "added"},
    {"path": "/media/films/Old Film (1999)", "change": "removed"}
  ]
}
```

- `path` is absolute and written **the way the Portico server sees it**. If the
  tool runs in another container or on another machine, map its paths to the
  server's first.
- `change` is optional (`added`, `changed`, `removed`). The scan looks at the
  folder either way.
- Up to 200 paths per request.

The answer (`202`) says what will happen for each path:

```json
{
  "paths": [
    {"path": "/media/tv/Severance/Season 02/Severance - S02E01.mkv", "status": "queued", "libraryId": "tv", "sourceId": "tv", "folder": "Severance/Season 02"},
    {"path": "/media/films/Old Film (1999)", "status": "queued", "libraryId": "films", "sourceId": "films", "folder": "Old Film (1999)"}
  ],
  "jobs": [
    {"id": "9gk2", "sourceId": "tv", "status": "queued", "phase": "inventory", "processed": 0, "analyzed": 0, "warnings": 0, "scanMode": "incremental"}
  ]
}
```

| `status` | Meaning |
|---|---|
| `queued` | The path is inside a library folder; `folder` is the folder that will be scanned, relative to it (`.` is the library folder itself). |
| `outside_libraries` | No library folder contains this path. Nothing is scanned. Check the path mapping. |
| `invalid` | Not an absolute path, or too long. |

What the scan does:

- It lists `folder` and everything under it, and nothing else. A path Portico
  has not seen before is scanned from the nearest folder above it that Portico
  already knows, which is where a new folder or file is found.
- A reported folder that has been deleted can no longer be listed, so the scan
  looks from the folder above it, where it can see that the folder is gone.
- New and changed files are matched as in any scan. A file that is gone from a
  scanned folder is marked missing, exactly as a full scan would mark it; its
  title stays.
- Nothing outside the reported folders is judged, so a file missing elsewhere is
  not noticed by this scan.
- If the library folder is already being scanned, the report is kept and
  scanned when that scan ends; `jobs` then names the scan in progress. Reports
  for the same folder made twice are one report.

Follow progress with `GET /v1/admin/ingestion/jobs/{id} (openapi.yaml,
operationId getIngestionJob)`, or subscribe to the `scan.finished` webhook
(`09-webhooks.md`).

### Sonarr and Radarr

Both can call a URL when they import or upgrade a file (*Settings → Connect →
Webhook* posts their own payload, which Portico does not read; use *Custom
Script*, or any small relay, to send the body above). A one-line script:

```
curl -fsS -X POST "$PORTICO/v1/libraries/paths-changed" \
  -H "Authorization: Bearer $PORTICO_KEY" -H "Content-Type: application/json" \
  -d "{\"paths\":[{\"path\":\"$sonarr_series_path\"}]}"
```

Give the key the `library` scope only.
