# perf-reads — worst cases of browse, search, Home and the event feed

## Outcome

Reads that used to grow with the library now grow, at most, with what they return or with a fixed bound:

- **Selective filters** (artist, album, author, narrator, person, tag, label, studio, network, content rating, audio language, series, dynamic range, community and critic rating) are answered from their values: the matching member items are found through the value's vocabulary or index, named as an id list (at most 20,000 matching items), and the exact count and the first page start from them. Measured at 50k and 200k they are flat (e.g. songs `tag equals` 1.38M → 205 VM steps at 50k).
- **Broad filters** (owner decision, 14 Oct): a posted filter's total is counted up to 10,000. Beyond it the response says `totalIsLowerBound: true`, `total` is 10,000, and pages keep working by cursor and by direct range past it. Client-core, the shared collection and the web show "10,000+" from the string catalogue and let the grid grow as pages arrive. Filtered groupings do the same per group (`totalAtLeast` on the entry) and, under a broad filter, count only the groups on the returned page.
- **Filtered groupings** (Genres, Categories, Decades) evaluate the filter only on the titles that can hold the grouped field, found by kind on the index, instead of every artist, album, season and episode (tv/categories median 0.53M → 3.4k steps at 50k).
- **Restricted viewers**: deep pages in fact orders and every page of container grids (albums, shows, artists) check the live restriction on the rows of the page only, using the published visibility class for positions; a row restricted since the class published is left out and the page refilled.
- **Sorted search**: title order reads its page from the title index for broad words (5.3M → 81k steps at 50k for `Ti`); totals come from the maintained counts (one indexed word) or are marked not exact; the full-text rank is no longer computed for sorts that do not use it.
- **Continue Watching** for a restricted viewer finds a show's next episode by testing its later episodes in number order, stopping at the first that passes (pg13 Continue row 2.73M → 0.73M steps, 577 → 98 ms at 200k).
- **Ordered music listening** no longer builds a Bloom filter over the whole catalogue before its first row (2.8M → 2.4k steps at 200k, flat); the library mix's upper bound is one seek per kind.
- Every read now reports SQLite VM steps (`dbwork.Cost.VMSteps`, `maxVmSteps` in the benchmark), and the new tests assert shapes (flat with library size, bounded by the page) rather than times.

Final before/after tables are below.

## Framing (owner guidance, 14 Oct)

A 5-million-item library and a 50,000-item library should both feel decent. For reads this means latency should stay roughly flat as the library grows. Priority went to any read whose cost rises with library size or with the size of its matching set, measured at several sizes, rather than to shaving milliseconds off reads that are already flat.

To answer "does it grow?" independently of machine noise (three lanes share the host), each measured read records SQLite virtual-machine steps. Steps are deterministic for a given database, so the ratio of steps between the 50k and 200k fixtures is the shape answer; wall-clock times are reported alongside and are noisy (other lanes' benchmarks and derivations ran concurrently). On this host one million steps is roughly 80–120 ms.

## Findings (review, before fixes)

Narrow mixed fixture (`-bulk-facts -narrow-facts`), 50k and 200k playable items, open viewer unless named. Steps are per request.

| # | Finding | Severity | Evidence (before) | Cause | Fix |
|---|---|---|---|---|---|
| F1 | Filtered groupings evaluated the client's predicate on every browse row of the library, including artists, albums, seasons and episodes, which never hold a facet; a container's member predicate re-walked all its members, so a music library walked every song three times. | High | music/genres filter median 1.28M steps at 50k, 4.4M at 200k (×3.6); tv/categories 0.53M → 2.1M | `matched` CTE scanned all library rows | holders by kind (shows and non-episode playable kinds, via `catalog_kinds` flags) |
| F2 | Selective member filters were tested once per title; a filter matching nothing still walked the whole pivot for its exact count and first page; on container pivots each title walked all its members. Cost grew with the library, not with the matches. | High | songs `artist equals` 1.5M, `tag equals` 1.38M, `communityRating at-least` 2.44M; artists `communityRating equals` 4.48M (50k); ×3.9 at 200k | correlated `EXISTS` per title | value-driven id lists (F2 fix below) |
| F3 | A restricted viewer's deep page in a fact order (track, artist, album, file size …) re-tested the live restriction on every row before the offset. | Medium | pg13 songs trackNumber deep 0.37M steps | restriction inside `LIMIT/OFFSET` | class positions, live check on the page |
| F4 | A restricted viewer's container pages (albums, shows, artists) re-tested the live restriction on up to a block (~1,500 rows) before the in-block offset; each test walks members. | Medium | pg13 albums every sort ~0.89M steps, 80–90 ms, every page | same, inside the block walk | same |
| F5 | Sorted search computed the full-text rank for every match it never used, then counted every match again. | High (named in brief) | `Ti` by title 5.3M steps (0.45 s) at 50k | `-rank` on the candidate set; FTS count | rank only for relevance; maintained counts; title walk |
| F6 | Containers without their own poster borrow a child's; the walk ran even for containers that had artwork. | Low | Home recent rows | unconditional walk | skip when artwork exists |
| F7 | Broad filters and filtered groupings counted every match exactly. | Structural | songs `artist is-present` 0.66M → 1.2M steps (×3.8) | exact totals | owner decision: 10,000+ |
| F8 | Restricted Continue Watching tested every later episode of each started show (availability, watched state, restriction) before ordering them to take one. | High | pg13 Continue 93k steps at 50k, 2.73M at 200k (×29: started shows × show size) | `ORDER BY … LIMIT 1` after full evaluation | order cheaply, test lazily |
| F9 | Ordered music listening (library) built a Bloom filter over every catalogue entity and every availability row before its first row (SQLite turns a selective join/EXISTS into a Bloom-filtered join, and builds the filter from a full scan). | High | 0.71M steps at 50k, 2.83M at 200k for a 101-song page | Bloom filter construction | LEFT JOIN with COALESCE, CASE-wrapped EXISTS |
| F10 | The library mix computed `max(id)` over every entity of the library. | Low | 6 ms at 50k, linear | index `(library_id,kind,id)` cannot answer max over kinds | one seek per kind |
| F11 | Deep pages in fact orders (album, artist, track, file size, resolution, release, latest aired, critic, episode count) page by `OFFSET` over the fact index: cost grows with the position. Counted blocks exist only for title, added, year, duration, rating and random. | Medium | open deep fact pages 0.1–0.3M steps at 50k | no counted blocks for fact orders | not fixed (derived data; see Not finished) |
| F12 | A container card's count is a count of its memberships per page (O(container size)). | Low | artist pages with large artists | no maintained member count | not fixed (derived data) |
| F13 | Filters with no value-side index (versions, duration, resolution, subtitle language, credits by name, genre presence) still test each title. | Medium | songs `versions equals` 1.85M steps at 50k | per-title correlated subqueries | bounded now by the 10,000 cap for broad values; selective ones remain linear (see Not finished) |

Reviewed and left alone: the event feed (`eventfeed.Read`) walks events after the client's cursor filtered by audience, bounded by retention and the page size; notification counts are bounded by the per-inbox cap; Home manifest and most Home rows are flat; relevance search was already flat (prefix postings).

## Fixes (commits on `audit-implementation/perf-reads`)

1. `174ad4dd`, `103f16d0`, `e4e0024c` — benchmark: `-only` route filter, per-route statement profile, `-skip-class-build`, resumable derivation, VM steps per route; `dbwork.Measure` now reports `VMSteps` (from the driver's `SQLITE_STMTSTATUS_VM_STEP`, recorded when rows close).
2. `9115027b`, `ca3550b8`, `54433f1b` — filtered groupings test the predicate only on facet holders, found by kind on the index (`compactcatalog.FacetHolderSQL`, beside `deriveFacetCounts` whose condition it states).
3. `670a7067`, `d28236de`, `0f7a54f5`, `e19c1dec` — value-driven member filters: a bounded count (≤ 20,001 matching items) decides; few matches are named as an id list (`e.entity_id IN (json list)`), so count and page start from them; none gives a constant false; broad or presence predicates keep the per-title test (where a container stops at its first matching member). Item pivots join only the item's own membership. Attribute fields test the condition on the term key (lower-case, trimmed text folds like its values) before reading edges; artists and albums are scanned by kind on the title index; ratings use the `(provider,value,item_id)` index; persons start from their credits (item or container credits).
4. `0e9d1032`, `96edf00e` — restricted class ranges and container pages: positions from the class, live restriction on the selected rows only, refilled when a newer restriction drops rows.
5. `01f3b623` — child poster walk only for containers without artwork.
6. `cf7f621c`, `6f5960ee` — sorted search: no rank for non-relevance sorts; totals from maintained counts (one indexed word) or `totalCountExact:false` (several words); broad title-sorted search reads each library's title index (window 4,096 titles, probes the prefix postings, stops at the page) and falls back to ranking matches when the window holds neither a page nor the library's end. Title ties now break by the internal id for both paths.
7. `bc07d585`, `b766b22b` — library mix upper bound with one seek per kind.
8. `a0f09aab` — next episode: later episodes ordered by number in a bounded subquery (LIMIT keeps SQLite from flattening it or dropping its order), then tested in order, stop at first.
9. `868ffb3c`, `9a0eb7a2`, `f9ac9b2d`, `00166700` — 10,000+ totals (owner decision), server and clients; broad filtered groupings count the page's groups only (by name or recency) in one statement.
10. `a0054341` — ordered library listening without Bloom-filter construction.

Tests (all assert shapes or bounds, not times): `read_shape_test.go` (grouping holders; selective filters flat at 80 vs 320 songs; attribute term folding parity; restricted deep fact page; restricted container page; sorted search counts; title walk equals ranked pages and stays flat; library mix bound; ordered listening flat beside 600 unrelated titles; person filter parity), `browse_total_cap_test.go` (exact below, exact at, capped above, paging by cursor past the cap, range past the cap, groupings by name/count/recency), client-core `browse.test.ts` and `windowed-collection.test.ts` (lower-bound parsing, growth, settling on the end). Updated: `group_range_capacity_test.go`, `browse_page_boundary_test.go` (class range now checks the live predicate on the page and refills), `legacy_view_plan_test.go` (the next-episode subquery is a bounded alias), `search_page_test.go` (title ties by internal id).

## Before and after (VM steps per request, median over the routes of an area; times are single noisy samples)

| Area (routes) | before 50k | before 200k | after 50k | after 200k | 200k max ms before → after |
|---|---:|---:|---:|---:|---:|
| music/genres (370) | 1,275,708 | 4,417,913 | 375,600 | 515,643 | 1972 → 721 |
| movies/movies (558) | 32,562 | 102,071 | 20,036 | 44,780 | 1963 → 528 |
| pg13/home (1) | 795,232 | 4,532,129 | 795,232 | 2,530,832 | 1466 → 376 |
| music/artists (362) | 24,255 | 92,362 | 9,655 | 39,996 | 1408 → 759 |
| music/songs (410) | 500,735 | 1,217,937 | 112,286 | 146,758 | 1312 → 1175 |
| tv/categories (532) | 533,754 | 2,134,692 | 3,366 | 12,536 | 1236 → 404 |
| tv/episodes (576) | 470,684 | 1,656,584 | 94,299 | 164,780 | 883 → 901 |
| focus/pg13/music (156) | 175,732 | 420,349 | 82,995 | 148,584 | 875 → 796 |
| open/home (1) | 480,972 | 984,641 | 480,972 | 984,641 | 812 → 209 |
| music/albums (416) | 58,942 | 174,074 | 26,438 | 81,402 | 744 → 543 |
| focus/pg13/search (31) | 70,924 | 71,791 | 70,924 | 71,791 | 722 → 497 |
| tv/shows (582) | 6,844 | 20,295 | 3,974 | 10,046 | 675 → 692 |
| movies/collections (516) | 121 | 121 | 139 | 139 | 605 → 506 |
| tv/collections (516) | 121 | 121 | 139 | 139 | 602 → 527 |
| pg13/home-row (9) | 92,934 | 226,546 | 92,934 | 226,533 | 588 → 112 |
| focus/pg13/tv (130) | 108,796 | 144,385 | 94,713 | 99,288 | 513 → 540 |
| focus/open/search (31) | 12,314 | 12,314 | 12,314 | 12,314 | 418 → 317 |
| open/listening-mix (1) | 580,372 | 2,053,060 | 418,886 | 1,406,284 | 275 → 55 |
| movies/categories (516) | 33,478 | 133,460 | 33,787 | 125,784 | 266 → 293 |
| focus/open/music (156) | 61,476 | 77,153 | 61,476 | 77,153 | 246 → 101 |
| grouped/artist/filter (358) | 521,318 | 489,748 | 362,744 | 350,012 | 218 → 209 |
| grouped/genre/filter (504) | 64,439 | 76,913 | 32,751 | 38,916 | 175 → 160 |
| grouped/show/filter (520) | 96,962 | 90,440 | 70,563 | 67,782 | 165 → 175 |
| focus/pg13/movies (60) | 25,147 | 60,363 | 20,595 | 26,278 | 151 → 91 |
| pg13/listening-mix (1) | 661,217 | 1,976,010 | 499,731 | 1,329,234 | 131 → 49 |
| grouped/genre/sort (54) | 64,649 | 77,297 | 64,659 | 77,307 | 125 → 48 |
| search (8) | 12,314 | 12,293 | 12,314 | 12,314 | 118 → 61 |
| open/home-row (9) | 37,957 | 57,435 | 37,957 | 57,435 | 109 → 83 |
| grouped/artist/sort (52) | 526,237 | 523,540 | 551,262 | 548,565 | 97 → 91 |
| pg13/listening-ordered (1) | 728,980 | 2,848,287 | 28,107 | 28,423 | 85 → 3 |
| open/listening-ordered (1) | 708,136 | 2,827,190 | 2,379 | 2,379 | 80 → 2 |
| focus/open/tv (130) | 83,523 | 88,883 | 83,523 | 88,883 | 74 → 69 |
| focus/open/movies (60) | 17,945 | 29,660 | 17,945 | 29,660 | 61 → 45 |
| grouped/show/sort (56) | 102,467 | 102,211 | 107,972 | 107,716 | 18 → 21 |

Whole browse/search/Home matrix (7,690 routes measured at both sizes, one sample each; steps are deterministic, times are noisy):

| | before | after |
|---|---:|---:|
| Routes whose steps more than double from 50k to 200k | 3,466 | 2,374 |
| Steps per request at 200k: median / p99 / max | 96,738 / 7.80M / 23.2M | 43,422 / 4.35M / 13.2M |
| Time at 200k: median / p99 / max | 12.0 / 961 / 1,972 ms | 6.1 / 441 / 1,207 ms |
| Routes over 200 / 500 / 1,000 ms at 200k | 905 / 327 / 69 | 309 / 57 / 4 |

Selected reads (VM steps; 50k → 200k shows the shape):

| Read | Before 50k → 200k | After 50k → 200k | 200k time before → after |
|---|---|---|---|
| songs `artist equals` (no match) | 1.50M → 6.00M (linear) | 3.1k → 11.9k | 1,143 → 4 ms |
| genres filtered by artist `contains-any` | 1.83M → 7.33M | → 11.9k at 200k | 1,464 → 5 ms |
| genres filtered by `genre is-present` (broad) | 2.70M → 10.1M | 1.04M → 1.09M (flat, capped) | 1,121 → 135 ms |
| songs `artist is-present` (broad, capped count) | 0.66M → 2.61M | → 0.29M | 318 → 30 ms |
| categories (TV decades) `resolution not-equals` | → 5.90M | → 1.35M | 1,050 → 252 ms |
| pg13 songs trackNumber deep page | 0.37M → 1.43M | → 0.29M | 235 → 73 ms |
| pg13 albums title page (any page) | 0.89M | 0.14M → 0.15M (flat) | 84 → 20 ms |
| pg13 Continue Watching row | 93k → 2.73M (×29) | 93k → 0.73M | 588 → 104 ms |
| pg13 Home (all rows) | 0.80M → 4.53M | 0.80M → 2.53M | 1,466 → 376 ms |
| ordered music library page | 0.71M → 2.83M | 2.4k → 2.4k (flat) | 80 → 2 ms |
| search `Ti` sorted by title | 5.33M at 50k | 73k → 47k (flat) | 21 ms at 200k |
| search `Ti` sorted by release year / date added | 5.47M / 5.82M at 50k | 3.04M → 12.1M / 3.39M → 13.2M (linear) | 810 / 1,024 ms |

Shape answers: selective member filters, filtered groupings (by name or recency), restricted container and fact pages, ordered listening and title-sorted search are now flat or near flat between 50k and 200k; broad filters are bounded by the 10,000 cap (the count stops growing with the matching set). Still linear: selective filters without a value-side index (versions, duration, resolution, subtitle language, actor/director/writer by name, genre presence — the fixture's "no match" operands make these the worst routes left), deep pages in fact orders (by offset), release-year and date-added sorted search, and the restricted Home rows' remaining work (pg13 Home 0.8M → 2.5M steps, grows with the profile's started shows).

## Decisions I made

- Exact totals stay for plain library views and for filters whose maintained summaries answer (availability) and below 10,000 matches. Above, `pageInfo.totalIsLowerBound: true` with `total: 10000` (a lower bound, shown "10,000+"). I named the field after a lower bound rather than reusing search's `totalCountExact` so that every other producer of `BrowsePageInfo` stays correct by default (false = exact).
- Grouping entries above the cap set `count: 10000` and the existing `totalAtLeast: 10000`. Under a broad filter (more than 10,000 matching titles) a grouping sorted by name or recent activity counts only the groups on the returned page; sorted by count it counts every group up to the threshold (necessary to order them).
- The value-driven selection is used up to 20,000 matching member items; above that the per-title test is kept (cheaper there, and the count is capped anyway). Presence operators always use the per-title test.
- Attribute filters assume a term's key (`lower(trim(value))`) folds exactly as its values do (`sorttext.Fold` trims and case-folds). A parity test covers accents, case, whitespace, ß and the dz digraph; the exact condition is still re-checked on the edge's own value.
- Artist and album vocabulary scans assume credited artists and albums are artist/album entities (the only writers create them so).
- A restricted viewer's class drives positions; the live restriction filters the selected rows. During the short window between a restriction change and the class refresh a page can hold one row fewer at a deep position (it is refilled where the class guarantees the rows exist). Nothing restricted is ever shown.
- Title-sorted search breaks ties by internal id (as relevance prefix search does), so title-index pages and ranked pages continue each other; pre-release, no cursor compatibility kept.
- I did not run a one-million fixture: the 200k derivation alone took 35–45 minutes on the shared host; the shape answers come from 50k/200k step ratios plus fixture-size tests.

## Schema changes

None. No indexes added. (`FacetHolderSQL` reads `catalog_kinds.playable`.) Derived content unchanged; no derivation version bump.

## API changes

- `POST /v1/libraries/{id}/browse` (and every `BrowseResult`): `pageInfo.totalIsLowerBound` (boolean, always present). True only for a posted filter matching more than 10,000 titles; `total` is then 10,000. `hasMore`/`nextCursor` remain authoritative; a direct `range.start` past the threshold is no longer clamped to the total.
- Grouping entries (`pivot` genres/categories/series): `totalAtLeast` (existing optional field) set with `count` = 10,000 when a group exceeds the threshold.
- `GET /v1/search`: non-relevance sorts now report `totalCountExact:false` for several-word queries (count from the page), exact maintained totals for one indexed word; title-sorted results break ties by internal id.
- Regenerated with `go run ./cmd/apigen`: `packages/contracts/src/schemas.ts`, `packages/contracts/src/types.ts`, `server/api/portico.openapi.json`.

## Client files changed (for the integrator to mirror into the native packages)

- `packages/client-core/src/browse.ts` — `BrowsePageInfo.totalIsLowerBound`; open-ended total (one page past what arrived while more remains; exact at the end); `hasMore` from the server when open-ended.
- `packages/client-core/src/collections/windowed.ts` — `PageResult.totalIsLowerBound`, `CollectionSnapshot.totalIsLowerBound`; an open-ended total only grows and settles when a page knows the end.
- `packages/client-core/src/library-content.ts` — `ContentEntry.countIsLowerBound` from `totalAtLeast`.
- `packages/client-core/src/presentation/content.ts` — `captionFor` renders "10,000+ items" for a lower-bound count (en-US fallback).
- `packages/client-core/test/browse.test.ts`, `packages/client-core/test/windowed-collection.test.ts` — new tests.
- `packages/i18n/src/catalog/en-US.ts` — `lib.countAtLeast` ("10,000+ movies").
- `web/src/app/browse-page-source.ts`, `web/src/app/group-browse.ts`, `web/src/app/library-words.ts`, `web/src/app/content.ts`, `web/src/screens/library/BrowseFilters.tsx`, `web/src/screens/library/Library.tsx`, `web/src/screens/shared/GroupBrowse.tsx` — pass the flag through; the count label and grouping tile captions use `lib.countAtLeast`.

## Needed from others

- **Native clients** (iPhone, iPad, Apple TV, Android, Android TV, Fire TV, Vega): sync client-core/contracts; their browse grids must use `CollectionSnapshot.totalIsLowerBound` (grow as pages arrive) and show counts with `lib.countAtLeast`; grouping tiles must read `countIsLowerBound`. Not done here (native packages are outside this worktree).
- **perf-rebuild lane / integrator**: counted blocks for the fact orders (F11) and a maintained member count for album/artist cards (F12) would make deep fact pages and container cards constant; both are derived data. Indexes on `catalog_assets(duration)` / `(height)` and a maintained version count would make the remaining selective file filters (duration, resolution, versions) value-driven like the others (F13).
- Architecture prose: browse totals are exact up to 10,000 and a lower bound beyond; value-driven filter selection; class-driven restricted pages.

## Test notes for other environments

Host ehler-homelab-mini-1 (Ubuntu, i5-10500T, shared with two other perf lanes). Fixtures: `scalebench -bulk-facts -narrow-facts` at 50k and 200k, derived with the production worker, restricted class built (PG-13 + block unrated). Reproduce: `go run ./internal/catalog/scalebench -dir <.scratch dir> -items 200000 -reuse -matrix-only -skip-class-build -focus-samples 3`; compare `maxVmSteps` between sizes. Clients: on any grid, filter a large library broadly (e.g. title contains a common letter): the count reads "10,000+ …", scrolling past 10,000 keeps loading, and the end of the list settles the count.

## Not finished

- **Selective filters with no value-side index** remain linear in the library: versions, duration, resolution, subtitle language, actor/director/writer (credited names), genre presence/absence, title substring. At 200k the worst take 0.8–1.2 s (6–12M steps) when they match nothing. Fix: indexes on `catalog_assets(duration)` and `(height)`, a maintained version count per item and a credited-name vocabulary, then the same value-driven selection. Not done: derived data and write cost belong with the perf-rebuild lane.
- **Deep pages in fact orders** (album, artist, track, file size, resolution, release, latest aired, critic, episode count; and these under a filter) page by offset: linear in the position. Counted blocks per fact ordering (open and per restricted class) would make them constant; derived data.
- **Release-year and date-added sorted search** rank every match (3.0M steps at 50k, 12–13M at 200k). The title walk needs an ordered index; `catalog_entities` has none on year, and date added lives in `catalog_item_details`. A maintained sort key on the search postings (or entity indexes) would allow the same walk.
- **Restricted Home rows**: pg13 Home 2.5M steps at 200k (Continue still walks each started show's later episodes until an allowed one, plus the recommendation and recent rows' restriction checks); grows with the profile's history and show sizes.
- **Container card counts** (F12) and **Home poster borrowing** for containers without artwork still walk members.
- **Native clients** have not been updated for `totalIsLowerBound` / `countIsLowerBound` (outside this worktree); see Needed from others.
- No one-million-item measurement (time); shapes are from 50k/200k step ratios and fixture-size tests. The full server suite was not run (by instruction); the touched packages (catalog, compactcatalog, dbwork, thirdparty/sqlite), httpapi contract tests, client-core browse/collection/library-content tests, web browse tests, `tsc --noEmit`, string lint and Windows/macOS cross-builds pass.
- Scratch fixtures, logs and the Go cache were deleted at the end; the tables above are the record.
