# Outcome

The library browse API now supplies one title sort/filter vocabulary for ordinary library tabs and scoped title groups. Container, collection, person, private/shared-list membership and text-search predicates can be combined with watched state, genres, year/decade, ratings, resolution, dynamic range, languages, favourites, missing/availability and version count. The viewer's library scope and content restrictions stay in SQL; member predicates also check the actual member's restriction, so a hidden child cannot satisfy a visible parent's filter.

Genre, series and decade/category grids now accept name, count and recent-addition order plus member filters, have exact totals, and continue by signed keys beyond 200 groups. Full title sorting is used by the title pivot a group opens.

New indexed scalar keys, maintained profile keys and matching retained-generation keys support stable paging. Profile sorting no longer reads and sorts the viewer's entire history. Source edits, member metric changes, parent renames and subtitle-evidence changes feed the derived keys or query revisions. The capabilities contract, generated registry contract and client-core parser/builder/toolbar are updated. No screen was changed, committed, pushed or deployed.

This is not a claim that every legacy composer or every grouping shape now meets the strict large-library performance requirement; see Not finished.

## Inventory: surfaces and their previous behaviour

| Server-composed surface | Previous sorts | Previous filters / paging | Full-control destination now |
|---|---|---|---|
| Library movie/show/episode/collection/artist/album/song/author/book tabs | title, added, year, runtime, community rating; personal rating/last played where applicable; For you for works; aired/latest aired for TV | Bounded expression grammar; indexed columns/blocks for five scalar keys, correlated/profile history and offset fallbacks for the rest | POST /v1/libraries/{id}/browse with the selected entity pivot |
| Music Genres, audiobook Series, movie/TV Categories aggregate pivots | title asc/desc only | No expression; cached entire facet set, offset continuation; base had already removed the 200-value total cap | Same route with aggregate pivot; name/count/recentActivity and member expressions |
| Titles within genre, series, year/decade, studio or other category | Main entity sorts when reached through a browse predicate | Preset predicate | Same entity route; combine the preset and new filters with all |
| Collection titles, including saved collection scopes | Separate stored/display order in detail/resource composer; browse collection predicate existed | Collection membership, entitlement, restrictions | Same entity route with collection equals public collection ID |
| Show/season/album/artist/book members | Detail/listening composer chooses episode/track/part/release order | Entity/season/disc/group selector and restrictions | Same entity route with memberOf equals parent public ID, choosing episodes/songs/albums/books as appropriate |
| Person titles | Person composer title/credit order only | Person, role, entitlement and restrictions | Same entity route with person equals person token, within a library |
| Saved views | Stored browse pivot, expression and sorts | Same expression grammar | Existing saved-view execution already calls BrowseEntities; new fields/sorts are available there |
| Search title results | Separate global search: relevance, title, releaseYear, dateAdded (narrower for some groups) | Text, libraries, kind groups; keyset for titles | Full-control in-library result list uses search contains text on the common browse route; global envelope remains unchanged |
| People directory/search | relevance/title | Text/entitlement; offset | Directory is a people list, not a media-title list; person-title scope above |
| Live channel search | relevance/title | Text; offset | Channel numbers/relevance remain the channel directory's semantics |
| Personal playlists / Saved resources | Stored list order | Membership, authorisation, restrictions | Same entity route with list equals token; ownership and shares checked in SQL |
| My List / Favourites / History | Dedicated personal composers' order and named filters | Personal state / history cursor | Title browse uses watchlisted/favorite/lastPlayed/playCount; dedicated history event order remains its own surface |
| Home/Discover rails, recommended See all, listening selections / queues | Server-chosen ranking or playback order | Curated selection; per-row/queue paging | Curated ranking/playback intent is preserved; user title browsing can use the common vocabulary |

Capabilities contain exact per-pivot sort/filter ID lists. Surface descriptors identify the common endpoint and membership field; clients intersect a descriptor with the selected pivot's options. Definitions are limited to kinds present in that library. A resolved aggregate publishes its three group sorts, not media-title keys. The builder refuses a sort/filter absent from its chosen pivot. Unknown/malformed individual capability entries remain tolerantly skippable.

# Decisions I made

- Work ran on personal mini-1 in the assigned worktree. Startup context-sync could not write its lock because that directory is read-only; canonical project, standards, verification and attribution guidance was read directly.
- One primary sort per browse request, maximumSorts=1. Arbitrary multi-column combinations had no matching maintained index. Existing saved/client queries must follow the newly published limit; no compatibility path was added.
- Random order is a rotation of a stable FNV-derived uint32 index, with an optional uint32 seed (default 0). Both halves are index walks. The same seed keeps the order across page sizes; changing it invalidates the cursor scope. Direction changes preserve it in the shared toolbar.
- File size and resolution sort by the largest file/highest resolution attached to a title or its members. Runtime/community rating retain the existing member-maximum semantics. Critic rating also uses the maximum recorded critic value; provider scales are not converted by this lane.
- Track order combines disc and track number, so discs stay together. Album/artist keys use natural parent sort keys and follow renames through a bounded parent fanout.
- A group list's recentActivity means its most recent title addition, advertised as “Recently added”; it is catalogue activity, not someone else's private viewing history.
- Direct personal favourites/ratings/dates on a container are included alongside its member state. A container's own personal record also feeds its maintained profile keys; a new album-intent test covers this.
- Personal rating and last-played orders preserve the existing valued/rest split: valued rows use their indexed key and entity tie-break; rows without a value use title order. Play count uses the same rule and the viewer's completed-play totals.
- Subtitle languages include current file evidence and current shared or viewer-owned uploads. Other viewers' private uploads and obsolete source evidence do not match. Their changes invalidate exact filtered totals.
- Text filters fold case and accents without stripping title articles. Identity tokens use exact comparison.
- No settings were invented. Detail previews, global search envelopes and playback queues retain their existing defaults; new full-control screens should use the advertised common browse surface.

# Bugs

All entries in documents 07, 12 and 18 and their bug-register rows were read. This lane fixes grouping/query work and reports other ownership boundaries rather than modifying the parallel lanes' personal-state, scanning, administration or screen areas. “Already fixed according to the fix log” is distinguished from a source/test check here.

- **B-07.1** — Already fixed on the base in steady state; source has isolated failures/retries and second-tier steps. Rebuild failure policy was not changed.
- **B-07.2** — Already fixed on the base through retained serving generations. This lane extended their columns and indexes; retained-generation tests pass.
- **B-07.3** — Not fixed in this lane: scanning, persistence operations or freshness ownership; not rechecked as a current defect here.
- **B-07.4** — Already fixed on the base for browse continuations. New scalar/profile/group cursors also carry keys, not revision-fenced offsets; removal and publication tests pass. Catch-all revision policy remains freshness-server ownership.
- **B-07.5** — Already fixed on the base: search uses the request snapshot reader.
- **B-07.6** — Already fixed on the base by search-sort normalization, including one-character and punctuated titles; preserved in the new search expression.
- **B-07.7** — Already fixed on the base by CJK index tokens and substring verification; the new search expression preserves the verification.
- **B-07.8** — Already fixed on the base: date-period comparisons are preserved.
- **B-07.9** — Already fixed on the base: seek uses sorttext.Letter, not a first UTF-8 byte.
- **B-07.10** — Not fully fixed here: shared buckets already fold accents and recognise other scripts, but raw-title article/explicit-sort-title reanchoring still needs a server sort-head consumer in client screens. New keyset cursors do not infer an anchor from a title.
- **B-07.11** — Not fixed in this lane: scanning, persistence operations or freshness ownership; not rechecked as a current defect here.
- **B-07.12** — Fixed: textual predicates fold Unicode case/accents using the same sorttext.Fold implementation on both operands. Collection/container/person/list identity tokens remain exact. Integration tests cover accented titles and genres.
- **B-07.13** — Not fixed in this lane: scanning, persistence operations or freshness ownership; not rechecked as a current defect here.
- **B-07.14** — Not fixed in this lane: scanning, persistence operations or freshness ownership; not rechecked as a current defect here.
- **B-07.15** — Not fixed in this lane: scanning, persistence operations or freshness ownership; not rechecked as a current defect here.
- **B-07.16** — Not fixed in this lane: scanning, persistence operations or freshness ownership; not rechecked as a current defect here.
- **B-07.17** — Already fixed on the base: search reads natural sort keys and actual show/book years. Preserved; broader title sorting is available through the common browse surface.
- **B-07.18** — Not fixed in this lane: scanning, persistence operations or freshness ownership; not rechecked as a current defect here.
- **B-07.19** — Not fixed in this lane: scanning, persistence operations or freshness ownership; not rechecked as a current defect here.
- **B-12.1** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.2** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.3** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.4** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.5** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.6** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.7** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.8** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.9** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.10** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.11** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.12** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.13** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.14** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.15** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.16** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.17** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.18** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.19** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.20** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.21** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.22** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.23** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.24** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.25** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-12.26** — Not fixed in this lane: personal-state, playback or recommendation workflow ownership; not rechecked as a current defect here.
- **B-18.1** — Already fixed according to the supplied fix log; not revalidated by this server lane.
- **B-18.2** — Already fixed according to the supplied fix log; not revalidated by this server lane.
- **B-18.3** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.4** — Server cap already removed on the base; replaced aggregate cache/offset paging with exact keyset pages and maintained unfiltered group keys. Existing 205-series test passes. Client screens still need to consume every continuation, as assigned to client lanes.
- **B-18.5** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.6** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.7** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.8** — Not fixed here: the bug register refutes the broad first-pass interpretation; identity/owner-tool presentation is outside this lane.
- **B-18.9** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.10** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.11** — Already fixed according to the supplied fix log; not revalidated by this server lane.
- **B-18.12** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.13** — Not changed: recorded as a deliberate trade-off in the bug register; unrelated to browse sorting.
- **B-18.14** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.15** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.16** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.17** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.18** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.19** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.20** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.21** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.22** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.23** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.24** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.25** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.26** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.27** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.28** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.29** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.
- **B-18.30** — Not fixed in this lane: client screen/administration ownership; not rechecked as a current defect here.

# API changes

- **POST /v1/libraries/{id}/browse**: no new route. Expanded sort vocabulary adds releaseDate, criticRating, episodeCount, album, artist, trackNumber, fileSize, resolution, random and playCount. Existing aired/latestAired now read derived indexed keys. Random selections have optional seed. maximumSorts changes from 3 to 1.
- Query fields add subtitleLanguage, versions, memberOf, person, list and search. Private-list ownership/share checks and private-subtitle ownership checks are inside SQL.
- Aggregate pivots previously accepted only title sorting and refused expressions. They now accept title/count/recentActivity and the same member-expression grammar. Their signed cursor stores the last group value and key, not an offset. No legacy cursor compatibility was added.
- **GET /v1/libraries/{id}/browse-capabilities**: pivots add sorts, filters and paging; sorts add seeded; document adds surfaces with endpoint, membershipField, sorts, filters and paging. Group pivots are advertised as queryable.
- client-core types, parser, builder and browseWithin helper support these shapes. Toolbar groups subtitle/version controls with file filters, hides internal membership IDs, permits the availability filter for viewers and preserves random seeds when flipping direction.
- apigen regenerated packages/contracts/src/{types,schemas}.ts and server/api/portico.openapi.json. Existing routes retain their registry key scopes/restriction classification; no registration changed or new route was added. No route removed.

# Schema changes

- Baseline browse rows add release/latest-air date, critic rating, episode count, album, artist, disc/track, file size, resolution and random keys, with scope/key/entity indexes. Retained rows, views, capture triggers and previous-generation indexes carry the same keys.
- Member metrics add exact air/critic/size/resolution values and entity/key indexes. Container maxima use index starts, not full member rescans.
- Episode counts and personal-play totals use narrow delta-maintained counter tables. Sparse personal sort rows retain exact rating/last-played/play-count values with profile/library/kind/key indexes. Source play-count edits join the existing bounded personal fanout.
- Facet members retain recent additions; new catalog_browse_groups stores name/count/recent keys by library/field/value with three indexes. Both models participate in lazy retained publications. Subtitle source/resource feeds bump affected library content and order revisions. The browse-order feed includes all new scalar keys. Changed exact personal keys publish the owning viewer's revision; unchanged keys are not rewritten or republished.
- Browse rows version 2→3; item metrics 1→2; facet counts 3→4. New parent-rename derivation **34**, version 1, uses catalog_fanouts and bounded keysets. Integration must avoid assigning another lane this permanent domain number.
- Personal play-count table/index creation moves from 0007_play_history.sql to the baseline so the baseline feed triggers can name it. No new migration or old-database compatibility was added. The rest of the existing migration chain is unchanged.
- schema-sync refreshed known columns/objects; the focused persistence schema checks pass.

# Owner or viewer settings added or wired

None. Sorts/filters are query controls governed by capabilities. Random seed is request state, not a persisted preference.

# Needed from others

- Web/native/Vega screen lanes: fetch pivot-scoped capabilities, intersect surface and pivot options, draw the new controls, carry subgroup predicates into their title requests, and consume every aggregate nextCursor. Use browseWithin for container/collection/person/list/search scopes. The current global search/person/resource/detail envelope routes still have their previous narrow/default orders; screens need the common browse route for full-control library title lists.
- Client package synchronisation to the native/Vega worktrees belongs to integration; this lane cannot write those worktrees.
- Client screens must use server sort heads for raw-title article/explicit-sort reanchoring (B-07.10), rather than guessing from a display title.
- API/document owner: the pre-existing handwritten server/api/browse.openapi.yaml and server/api/openapi.yaml duplicate generated contracts. They were not patched or deleted here; publishing/integration should remove the duplicate documents in accordance with the programme rule.
- Integrator: preserve new keys in both current and previous-generation models, the personal count creation move, facet version 4, and domain 34. Regenerate combined API/schema after merging all lanes.

# Test notes for other environments

Focused checks executed on Ubuntu, with GOFLAGS=-p=2 throughout. No full test suite or full application build ran.

Passed at the recorded checkpoints:
- go build ./... (Linux), GOOS=windows go build ./..., and GOOS=darwin go build ./... .
- go vet ./internal/catalog ./internal/compactcatalog ./internal/dbwork.
- New catalogue grouping tests: exact scalar key ordering in both directions, high-precision critic keys, restricted-profile pages, profile index plans, private/shared scope predicates, seeded random/page-size stability, removed anchors, group count/recent/name order, query filters, parent renames, episode counts/air dates, Unicode matching, private subtitles/current evidence and filtered-total invalidation. A later check covers exact personal-key publication and confirms unchanged play counts do not emit another revision.
- Existing TestBrowseEveryOrderPagesMatchTheOrder (all seven old orders, both directions, unrestricted/restricted), TestBrowseCompilesEveryOperator, quick filters/capabilities, and TestAggregatePagingBeyondTwoHundred.
- compactcatalog maintained personal bucket/class intersection and retained-generation tests. All ten new scalar-key query plans use current/previous order indexes without a temporary sort. Personal key selection uses profile indexes without materialising its order.
- HTTP production-router overlap construction, route credential-scope declarations, browse vocabulary, cursor/range paging and existing anchor checks.
- client-core and web npx tsc --noEmit; single client-core browse.test.ts (12 tests), including surface scopes, per-pivot options and seeded request/toolbar round trips.
- apigen, schema-sync and git diff --check.

One broader **selected** HTTP run failed TestBrowseReadinessFailsClosedDuringDerivedRebuild, an unchanged base test that asserts 503 during a domain rebuild. The actual response is 200 with retained content, matching the programme's authoritative decision. The other selected checks passed. A separate selected For-you run failed TestDiscoverCarriesPicksForYou (missing the expected Science Fiction heading); the integrator report already documents this test as failing on the base. Recommendation row choice was not repaired in this lane. No claim is made that all unrelated base failures are resolved.

Cold manual acceptance: build a fresh server with this schema, import video/music/books, create a restricted profile and a private/shared playlist, then browse a library and each title scope with every advertised sort/filter. Use at least 205 genres/series and continue all pages. Pass: no duplicates/skips, stable random order, matching visible counts, private uploads/lists never match another profile, and a scan/metadata edit does not throw away the current page. Edit an album/artist name and subtitle evidence; the keys/counts should refresh after background work. Repeat on Windows/macOS with the same fresh database setup; cross-compilation is not runtime acceptance.

Large-library acceptance still needed: millions of titles, large restriction classes, thousands of grouping values, deep cursors and direct ranges during rebuild. Monitor query plans, writer-held time and request latency. Exact filtered/restricted group aggregation and direct ranges without blocks are the known scaling limitations below.

# Not finished

1. **Strict “every accepted grouping sort from a maintained key” is not fully met for arbitrary filtered/restricted group count/recent order.** Unfiltered group order is maintained and indexed. An arbitrary predicate changes group counts, so that shape calculates exact visible keys in SQL over indexed memberships before its bounded keyset page. It avoids Go sorting and offset paging, but SQLite can aggregate/sort the full matching grouping set. Query-specific maintained group orders/visibility-class group keys are still needed for the strict large-library requirement.
2. **New scalar/profile/group keys have no maintained position blocks.** Cursor pages seek directly; direct range requests on those shapes walk bounded keyset chunks from the beginning. Memory is bounded and no unbounded OFFSET was added, but deep direct ranges cost the requested position. New sorts currently publish no scrub-position rail. Existing five scalar block orders retain their bounded in-block offsets.
3. **Legacy composers and screens are not all migrated.** Full-control browsing exists through the common in-library route, including person/list/search scopes. Existing global search, person/detail/list envelope routes retain their previous defaults and narrower accepted sort/filter request shapes. Client adaptation is explicitly assigned later, but this lane does not claim those original routes themselves now accept the full vocabulary.
4. **For you retains its existing bounded recommendation-candidate ranking path.** It is preserved as a pre-existing recommendation option and is not converted into a new fully maintained order by this lane.
5. No browser/device/native build or production-scale runtime test was performed. The unrelated bug entries above remain for their owning lanes.

Snapshots 001 and 002 were saved during implementation. Final cumulative snapshot 003 contains the finished tree and tests, subject “Complete scoped browse sorting contracts and verification”. The subsequent owner-authorized merge changes are recorded below; no additional snapshot was taken.

## Merge with the base

All five conflict files are resolved in the working tree. No staging, commits or snapshots were made; the Git index remains for the owner to stage and commit.

- `browse_engine.go`: retained the position-anchor capability helper and extended stale-range relocation to new scalar indexes (including seeded random); profile orders retain their viewport. The aggregate implementation remains in `browse_groups.go`. Adapted that implementation to keep the base's fallback to name order when title sorts carry onto groups, and to ignore carried seek tokens while retaining filters, count/activity sorts and keyset paging.
- `browse_expression.go`: retained restriction-aware member predicates, Unicode folding and exact opaque-ID comparisons; switched SQL calls to the base's `portico_fold_text` function.
- `sort_functions.go`: kept the base's single `portico_fold_text` registration, including NULL preservation, alongside the existing search/sort functions.
- `0001_baseline.sql`: retained both lanes' schema additions, grouping/indexed-sort maintenance triggers and consolidated personal/play-history/show/facet/notice objects; kept one personal-play-count index.
- `0007_play_history.sql`: removed following the base's baseline-only schema. Current play-history/count objects are in the baseline; no obsolete startup backfill remains.

All browse/search/facet registry routes already declare `Requires: apikit.ScopeRead`; the route-scope test passes. Regenerated contracts with `GOFLAGS=-p=2 go run ./cmd/apigen` in `server/` and schema records with `GOFLAGS=-p=2 python3 scripts/schema-sync.py`; no generated file was merged by hand. Adapted older tests to accepted group filters, the advertised one-sort limit, and the profile viewport rule. Added coverage for carried sorts/seek and seeded-random anchor relocation.

Verification passed:
- `GOFLAGS=-p=2 go build ./...` on Linux, Windows amd64 and macOS arm64.
- `go vet` on catalog, compactcatalog, dbwork, persistence and httpapi.
- Targeted catalog `TestGrouping|TestBrowse|TestAggregatePivot|TestFacetsRespect`; compactcatalog browse/generation/personal/visibility sorts; dbwork schema/statement-cache tests; persistence schema/known-object/known-column/second-open/digest tests. Additional seeded-anchor and group regression tests passed.
- HTTP browse vocabulary, retained-generation readiness, production-router and credential-scope checks.
- Web and client-core `npx tsc --noEmit`, and all 12 client-core browse tests.
- Working-tree conflict-marker scan (`rg --hidden --no-ignore`, including ignored files) and `git diff --check` are clean.

One additional check remains failing: `TestReadPathGoldenEquivalence`, with 56 differences against the old recording, including newly accepted year sorts for books/songs, the expanded capability vocabulary and changed content revision counters. Its fixture explicitly guards against re-recording during read-model work; it was left unchanged. This is separate from the passing requested package checks. The previously failing readiness test has been replaced by the base's retained-generation test, which now passes. Earlier implementation/scaling gaps in “Not finished” remain unchanged.
