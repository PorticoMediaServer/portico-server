# Lane report: cp-browse-parity — the same wide sort and filter in every grouping

Worktrees `server-cp-browse-parity` and `native-cp-browse-parity`, branch `audit-implementation/cp-browse-parity` in both. Both branches have `audit-implementation/main` merged in (server at `24b3ca88`, native at `ad332df`), including cp-queues-playlists, cp-web-playback and cp-polish. Nothing pushed. No subagents or helpers. Execution host: mini-1 (Ubuntu). `node_modules` are symlinks to the main checkouts' (ignored, not committed).

## 1. Outcome

Every grouping now has the library's whole Sort and Filter, on web, phone/tablet, TV and Vega (Vega runs the native screens):

- **Lists that span libraries are one list.** A new bounded surface, `POST /v1/browse`, sorts and filters a group across all the viewer's libraries: a person's titles, a playlist, a personal collection, a Home row, My List, Favorites and History. The per-library chips (web) and library picker (native) are gone.
- **New groups with a toolbar:** a Home row's See all (Recommended, Trending, Picks), a personal collection, a book's parts, one disc of an album, and a flat show's non-season groups (whole list, specials, absolute-only episodes, ranges of 100).
- **Native My List, Favorites and History** have the full toolbar in place of their narrower controls. A playlist's page (music playlists included) has it across libraries.
- **A person's page keeps the toolbar under Cast and Crew**, because the server now has `cast` and `crew` conditions.
- **A group's own order is a server sort (`groupOrder`):** playlist position, collection order, a show's or season's episode order, an album's track order, an artist's releases by type, an author's books in series order, a book's part order, a Home row's order, and History's most-recent-first. It **stays in force under a filter** and pages by position.
- **Playing a sorted or filtered view** (the cp-queues-playlists query selector) accepts `groupOrder` and lists across libraries (`libraryId: "*"`). The queue plays exactly what the pages show, in the same order.
- **Each group's sort and filters are remembered on the device**, per profile, like a library tab's (web and native).
- **B-07.10:** browse entries carry `sortLetter`, the server's own letter, and the client letter match uses it.
- **B-18.23:** web disc headings come from `entry.discNumber`. The English subtitle parser is deleted.

## 2. Decisions I made

1. **A list across libraries must name its group.** `/v1/browse` refuses a query that has no person, cast, crew, playlist, personal collection, Home row, My List, Favorites or History condition. The group's members are read first, capped at 10,000 (the same figure as the totals cap; over it, `totalIsLowerBound`). Filtering and sorting then run on those members only. Cost follows the size of the group, never the size of the libraries. Paging is by position inside that bounded set, and `paging: "offset"` is advertised.
2. **Cross-library vocabulary** is the per-library field and sort table for the pivot's kinds. There are two pivots: `titles` (film, show, album, book) and `items` (film, episode, song, book part; used for playlists and History). For You is refused because it ranks one library at a time. Facet values (`GET /v1/browse/facets`) are the per-library maintained counts added together. They count the library, not the group, exactly as per-library group pages already do.
3. **One sort id, `groupOrder`, for every group's own order.** It is published with `requiresGroup: true`. Library-tab toolbars hide it, and group pages offer it as their own first choice ("Collection order", "Playlist order", "Row order", "Part order"…). It is refused without a group condition.
4. **Rules I set for specific group orders:**
   - **Artist:** albums, then EPs, singles, compilations, then appearances. Newest first within each type, unknown year last.
   - **Author:** books in series order, then standalone books by title.
   - **Show:** seasons in order with specials last (as the show page does).
   - **A title that holds several members** (a show in a collection) takes the place of its first member.
5. **Person roles are two fields, `cast` and `crew`,** using the same Acting/Cast split as the person page. I did not add an operator.
6. **Personal collections:** the condition matches an entry itself, the title that holds a saved entry (checked against the restriction on that entry), and the parts of a saved title. Access is checked in SQL: owner, share, or "visible to everyone". A personal collection has no stored order, so it opens in title order and offers no group order.
7. **Home rows** are resolved through Home itself when the request arrives, capped at 500 titles, so the toolbar works on exactly what the row shows. A queue cannot name "what Home shows now", so a sorted Home row plays from the list on screen and does not send a server query.
8. **My List and Favorites:** the toolbar replaces each list's own Unwatched/In-progress chips and its own sorts (including "Progress") on web and native. The list still opens in its own order. History keeps its by-day view and period chips until a sort or filter is chosen.
9. **Group memory** lives in a store of its own beside library memory (`<viewer>#groups`, last 64 groups), so groups never push libraries out. A link that names a sort or filter wins over memory.
10. **Book parts** are a new audiobook pivot `parts`, flagged `members: true` so no client draws it as a tab. **A disc** is the album's songs with a new `discNumber` condition.
11. **Revisions:** a page's revision now includes the playlist's and personal collection's own revisions and the Home row's current members. Before this, a playlist edit could leave a cached filtered total stale.

## 3. Audit bugs and decision gaps closed

- **B-07.10:** server `ContentEntry.sortLetter` (`sorttext.Letter(sort_key)`); client-core `titleBucketValue`; `bucketIndexFor` takes the server's letter as the key. Tests: client-core `browse-groups.test.ts` ("The Wire" files under W).
- **B-18.23:** web `discGroups()` reads `entry.discNumber`; `discNumberFromSubtitle` is deleted. Test: web `entity-tracks.test.ts`.
- **Gaps from report §4 and Appendix 0 "Sort and filter everywhere":**
  - Home row See all, personal collections, book parts and discs, and non-season episode groups: fields, pivots and toolbars done on all clients.
  - Native My List, Favorites, History and Playlists: full toolbar.
  - Cross-library lists: one bounded surface.
  - A group's own order is a server sort and survives filters.
  - Person Cast/Crew keeps the toolbar.
  - Group choices are remembered on web and native.

## 4. API changes

- **New routes**, all with `Requires: apikit.ScopeRead`, lane `browsing`, and classified in both restriction matrices:
  - `POST /v1/browse` (`browse_across`)
  - `GET /v1/browse-capabilities?pivot=titles|items` (`get_browse_capabilities`)
  - `GET /v1/browse/facets` (`get_browse_facets`)
- **New browse fields:**
  - `cast`, `crew`
  - `savedCollection`
  - `episodeGroup` (`"<showId>/<group>"`)
  - `discNumber`
  - `homeRow`
- **New sort:** `groupOrder`.
- **New audiobook pivot:** `parts`.
- **Capability and entry changes:**
  - Sorts carry `requiresGroup`; pivots carry `members`.
  - New surfaces: `cast-titles`, `crew-titles`, `saved-collection-titles`, `episode-group`, `row-titles`. Across libraries there are also `watchlist`, `favorites` and `history`.
  - Paging may be `offset` (across libraries).
  - Entries carry `sortLetter` and, across libraries, `libraryId`.
- **Queue selector:** accepts `libraryId: "*"` and `groupOrder`.
- **client-core:**
  - `browse.ts`: `acrossLibraries`, plus `browsePath`, `browseCapabilitiesPath` and `browseFacetsPath` (built with the generated route builders) and `libraryTabSorts`.
  - New `presentation/browse-groups.ts`.
  - The toolbar model hides `requiresGroup` sorts.
  - The parser accepts a vocabulary that names no library.
- Contracts were regenerated with `go run ./cmd/apigen` after each merge.

## 5. Schema changes

None.

## 6. Settings added

None. Group memory is device state, like library-tab memory.

## 7. Tests run

- `go build ./...` and `go vet ./internal/catalog ./internal/httpapi`: clean.
- `go test ./internal/catalog` (whole package) after the final merge: **ok**. New tests:
  - `TestGroupOrderSurvivesFiltersAndPages`
  - `TestGroupOrderOfShowsAlbumsAndBookParts` (includes episode groups)
  - `TestPersonalCollectionAndPersonRoleScopes`
  - `TestBrowseAcrossLibraries`
  - `TestQueueFollowsGroupOrder`
  - `TestQueueAcrossLibrariesFollowsTheList`
- `go test ./internal/httpapi -run 'Route|Scope|Lane|Browse|Registry|Contract|Credential|Overlap|Vocabulary|Restricted|Queue|Playback'`: everything passes except `TestPlaybackV1QueueQueryCapturesBeyond500AndKeepsItsSnapshot`, which is flaky. Run alone it failed 1 time in 3 and passed 2 times. It depends on a 10 ms sleep and paused queue builds; I did not change that code.
- client-core `npm test`: **1969 pass, 0 fail.**
- web `npm run check`: **exit 0**. That covers tokens, strict design lint, oxlint hook rules, preference consumers, string lints, `tsc` and the full web test suite (580 pass).
- native `tsc --noEmit`: clean. `npm test`: **679 pass**. `npm run lint:hooks`: **exit 0**. ESLint on the files I touched: 0 errors (existing warnings only). `lint:strings`: clean.
- Vega `npm test`: **87 pass.**
- No lockfile changed, so the licence lists were not regenerated.

## 8. Device test notes

1. **My List, Favorites and History, on each device including TV and Vega.** Use a profile with titles in two libraries. The Sort and Filter row appears under the tabs, and the remote reaches it from the tabs. Choose Year and the list becomes one grid that mixes both libraries. Add a Genre filter, clear it, and the list returns to the whole My List. Play opens the right title.
2. **Playlist with entries from films and TV.** Add a filter: the order stays "Playlist order". Press Play: the queue follows the filtered list in playlist order.
3. **Person page.** Switch between All, Cast and Crew with a sort chosen: the toolbar stays. A director who also acted appears under both roles as appropriate.
4. **Home "Recommended" See all.** Sort by Year: the row's titles reorder. Choose "Row order" again: the original order returns.
5. **Audiobook with many parts, and a disc page of a multi-disc album.** Sort and filter, then clear back to the group's own order.
6. **Flat show (owner setting) with ranges.** Pick "101–200" and filter Unwatched: only that range shows.
7. **Memory.** Leave a collection sorted by Year, open another collection, then return: the first is still sorted by Year. A second profile on the same device does not see it.
8. **Letter rail (B-07.10).** In a library with "The …" titles sorted A–Z, scroll, then trigger a refresh: you are re-anchored under the right letter.

## 9. Not finished

- **No browser or device run.** The web browser driver and physical TV focus were not checked in this lane, only with unit and type checks.
- **Cost of lists across libraries.** Each page sorts the whole bounded group (at most 10,000 members) in SQL. Cost is flat in library size but proportional to group size. These lists publish no position anchors or letter rail.
- **History across libraries** reads the 10,000 most recently played titles. Beyond that its total is a lower bound.
- **Home rows** are recomputed through Home on every request, which costs one recommendation pass per page. A sorted Home row plays from the screen rather than from a server queue.
- **Cross-library facet counts** are library-wide counts, not counts within the group.
- **Native sorted My List/Favorites/History** draw every result as a poster card, including square album art.
- **The music "Playlists" tab** (the list of playlist names) keeps the single name sort the server publishes. The full toolbar is on each playlist's own page.
- **Flaky test:** `TestPlaybackV1QueueQueryCapturesBeyond500AndKeepsItsSnapshot` (see section 7). Not touched.
- **GOCACHE** `_program/scratch/gocache-cp-browse-parity` is deleted at the end of the lane.

## Commits

Server (`server-cp-browse-parity`):
- `5b199eac` Server vocabulary, group order, lists across libraries, sort letters
- `990c8a4c` Web and client-core toolbars, across-library groups, memory, B-18.23
- `72bda427` Web disc toolbar
- `d14b6226` Range-sweep test adjustment
- `3f48f9b0`, `95b8eed4`, `fe7ae361` Merges of `audit-implementation/main`; queue across libraries; Home-row play guard

Native (`native-cp-browse-parity`):
- `a1df8d9`, `28503a6`, `5f82c79` Shared-package mirrors
- `efdd1e6` Person, playlist and personal collection across libraries; group memory; own order under filters
- `1c6bf6d` My List, Favorites, History, Home row, book parts, disc and episode groups
- `e501f68` Merge of main, plus the Home-row play guard
- `5ca5715` Unused import
