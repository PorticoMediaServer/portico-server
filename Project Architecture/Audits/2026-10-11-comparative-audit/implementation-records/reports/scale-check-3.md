## Outcome

Implementation is in place; final capacity acceptance is still pending. The fresh 200k baseline is complete. The million-title baseline is complete and its database has been removed. The 200k updated exercise is complete and its database was removed. The million-title updated empty rebuild is running with eight-unit checkpoints and the later numeric/class-range fixes. At 4h46m58s it had 199,567 queued units remaining, 110,344 KiB sampled RSS, 4.8 GiB scratch usage and no logged probe failures; this is progress evidence, not a completed rebuild result. No completion or universal latency claim is made yet.

Implemented:
- Availability is a published scalar backed by covering indexes, exact unrestricted rollups, retained rating/label counters, and counts within each published restriction class.
- Direct filtered/fact ranges select narrow identities before page hydration. Random direct ranges use maintained counted blocks in both open and restricted models, resolving a seeded rotation to at most two page-sized runs.
- All 100 existing block split/next-row selectors use disjoint equality/ID and next-key seeks with a bounded merge, including runs of identical/default values.
- Derived queue acknowledgement uses primary-key deletes with the original revision guard.
- Search has short text-prefix postings, compact numeric terminal postings, cached ranking tiers and maintained counters in both search slots. Each library/term supplies a bounded stream; a small merge selects the page. Matching remains exhaustive through continuation or the unchanged full-text fallback. An exact zero permission count returns before reading any candidate stream.
- Positive genre, container, collection, person and list short-word filters probe each selected document’s prefix rows, eliminating repeated broad FTS work.
- Cold restricted main browse serves authorized content while a new class builds instead of returning a successful empty preparation gap.

Evidence lives in `_program/scratch/scale-check-3/`. The report will receive final tables and test results before the final snapshot.

| Fresh starting observation | 200k | 1m |
|---|---:|---:|
| Full matrix route observations / errors | 6939 / 9 | 6938 / 10 |
| Main first-page median over routes / maximum, ms | 14.069 / 1111.525 | 39.097 / 5994.390 |
| Main deep-page median over routes / maximum, ms | 25.640 / 3833.631 | 117.462 / 5696.804 |
| Broad typing search timeouts | 6 | 6 |

The 200k file includes one extra `derive-empty` observation; the same 6,938 read-route names are present at both sizes. At 1m the four grouped text routes timed out at 30 seconds, in addition to six search group timeouts. Maximum sampled RSS was 118.1 MiB at 200k and 115.0 MiB at 1m.

Updated 200k comparison (the executable predates the final eight-unit checkpoints, numeric typing correction and class-driven fact ranges):

| Same read-route set at 200k | Before | After |
|---|---:|---:|
| Main first-page median / maximum, ms | 14.069 / 1111.525 | 13.182 / 1213.779 |
| Main deep-page median / maximum, ms | 25.640 / 3833.631 | 8.728 / 565.322 |
| Sort-only first-page median / maximum, ms | 4.751 / 132.343 | 4.806 / 67.986 |
| Sort-only deep-page median / maximum, ms | 9.025 / 3833.631 | 7.433 / 42.205 |
| Eight broad search queries median / maximum, ms | 857.918 / 1436.050 | 13.663 / 51.781 |
| Those search queries with errors | 6 | 0 |
| Peak sampled RSS, MiB | 118.1 | 128.1 |

The updated 200k report has 7,699 observations, including the extra focus/write scenarios. Seven errors are confined to the added `Title 0`, `Title 00`, `Title 000` and `Title 0000` typing checks across the two profiles. The numeric dictionary fanout exceeded 128 and selected the full-text fallback; the final implementation keeps a bounded text lead and checks the wide numeric word through the entity’s reverse index. Its growing-dictionary regression passes; the million-item run will measure these queries. The five-sample focus run at 200k still exposed restricted fact deep pages: PG-13 songs/file-size descending was 633.023 ms median and 715.211 ms p95. Final class-driven ranges retain the live permission predicate before LIMIT and have a regression that checks the class drives the selection.

These are one observation per route; a median over routes is not a per-request latency percentile. The final run adds five repeated observations of each main sort/direction, common availability filter and typing step, for both profiles. Focused deep reads use `min(50000,max(0,total-40))`, so restricted/small scopes select a valid page rather than an out-of-range empty response; each offset is logged as `FOCUS_RANGE`. The comparable one-sample matrix retains its original fixed offset of 50,000.

## Decisions I made

- Preserve exact browse counts, sort direction, filtering, authorization and cursor scope. Search totals explicitly carry `totalCountExact`; multi-word intersections, pending generations, member restrictions or excessive bucket cardinality use an unknown total rather than an exhaustive request-time count.
- Prefix relevance orders exact title, title prefix, title word and context; ties use stable internal identities. The least frequent indexed term drives multi-word selection and supplies its cached prefix/word/context tier; the full-query exact title has separate priority. This is a coarse maintained relevance model, not the fallback BM25 score or a full-phrase-prefix bonus. Its identity is signed into the cursor, so subsequent publications cannot change the cursor's ranking model. Other search sorts retain their existing full-text selection and exact-count paths; the prefix latency claims apply to relevance sorting. Non-default search sort latency is not established by the typing matrix.
- Index text prefixes through twelve characters. Numeric words share their six-character terminal key; shorter numeric queries expand a bounded dictionary range scoped to the authorized libraries and kind. More than 128 terminals, excessive stream fanout, long tokens and CJK use the full-text selector; no candidates are silently truncated.
- Literal pipe characters in permission sets prevent an exact bucket decision. Such requests keep authoritative permission SQL and unknown search totals. Added a partial vocabulary index so this test does not scan labels.
- Count readers stream buckets; unrestricted word/availability totals have dedicated rollups. A search intersection does not become an owner-facing complete count.
- The million-title **baseline preparation**, not its read measurements, uses 500-unit fixture writes. It was paused/resumed at a commit boundary, with direct revision acknowledgements and bounded block writes. The original trigger definitions were restored after main derivation. During the untouched read executable’s subsequent class preparation, TV completed with original writes in 690.279 s; music was still incomplete after about 16 minutes. The same bounded selectors were temporarily reapplied to finish that disposable preparation and are restored after class publication. Trigger substitutions preserve row/block outputs and change only writes, while the matrix measures read paths. The reported baseline visibility-build duration is excluded from comparison because this preparation was accelerated. This does not re-measure original million-title rebuild holds/failures; the previous lane remains the evidence for those starting numbers.
- Based on the 200k hold measurements, production worker checkpoints were reduced from 16 to eight; visibility writes keep 100-row read chunks but commit eight selected rows at a time, and cleanup/refresh use eight-row writes. These changes landed after the 200k process started, so the million-title run uses the new checkpoint size.
- The million-item empty rebuild began before the final range output-alias correction. A temporary abort trigger in this disposable fixture stops it at class preparation after the full empty rebuild finishes; its log/partial report are retained, the trigger is removed, and the same completed database is measured with the corrected executable. This is benchmark handoff, not a failed browse/search probe or a production schema change.
- Context-sync failed because its lock directory is read-only. Existing canonical project and verification guidance was readable and was loaded.

## Bugs

- B-07.1 — already fixed on base: poisoned units yield and step workers receive fair turns; focused worker tests pass.
- B-07.2 — fresh original-code 200k empty rebuild had 4,825 concurrent probes, zero failures, a 1,730 ms worst combined browse/search read and a 542 ms peak gate hold. Retained browse and separate search generations already exist on base. This lane keeps prefix rows/counters with the active search slot and removes the large cold-policy main-browse blank. The updated 200k empty rebuild completed with 11,179 probes, zero failures and a 137.558 ms worst combined browse/search read (16-unit worker). The 200k atomic watched/undo actions held the gate for 243/235 ms. The 200k version-bump rebuild had 1,415 probes, zero failures and a 56.662 ms worst combined read; adding 1,000 films had 41 probes, zero failures and a 76.063 ms worst combined read. The million-title exercise remains pending. Rebuild probes use the open viewer and all search groups; restricted read parity and cold preparation are covered by the route matrix and functional tests, not a concurrent restricted rebuild probe loop.
- B-07.3 — already fixed on base, backed by the prior capacity lane’s moved/reused-place test; no identity changes here.
- B-07.4 — base cursors re-anchor/retry rather than reject ordinary revision movement. No broad revision-counter redesign here; prefix cursors preserve their ranking model.
- B-07.5 — already fixed: search revisions read the same snapshot as results.
- B-07.6 — already fixed punctuation/one-character normalization, preserved by the prefix path; focused search tests pass.
- B-07.7 — existing CJK index/substring validation preserved through the full-text path; focused search tests pass.
- B-07.8 — existing compiled date comparisons preserved; browse tests pass.
- B-07.9 — existing `sorttext.Letter` seek preserved.
- B-07.10 — not fixed here in full: client-core already folds accents and supports Unicode letters. It still cannot infer the language-dependent article-stripped server key from a raw title. The web’s stale title range does not call this bucket helper; native callers need client-lane verification against the server sort key.
- B-07.11 — raw item-delete route is absent on base; no delete API changes.
- B-07.12 — Unicode folding is retained. The canonical availability enum avoids a redundant folding expression so its index can be used; input values still fold before comparison.
- B-07.13 — no backup changes or large busy-backup acceptance in this lane.
- B-07.14 — only the directly edited baseline ships; no compatibility migrations added.
- B-07.15 — base retention remains; new prefix dictionary entries prune only when neither search slot references them.
- B-07.16 — production write-gate health reporting remains unchanged.
- B-07.17 — stored year/sort-key search ordering remains; cached prefix relevance adds the described tiers.
- B-07.18 — fixes implemented: bounded block writes and guarded primary-key acknowledgement. The 200k run with 16-unit worker checkpoints reached 492 ms; its 100-row visibility writes reached 473 ms. Production worker, visibility publication and class-refresh writes now checkpoint after eight rows/units, and visibility cleanup deletes eight rows. Worker/visibility tests pass. The million-title run will measure the smaller checkpoints; the in-flight 200k executable keeps its recorded sizes.
- B-07.19 — no production database was opened externally. Disposable synthetic fixtures were inspected and resumed; all databases will be removed at the end.

## API changes

`GET /v1/search`: existing typed device route and read key scope retained. Added required boolean `groups[].totalCountExact`. When false, `totalCount` describes only the returned page and must not be displayed as a complete count; `hasMore`/`nextCursor` remain authoritative. Client-core parses the flag, validates count accuracy, and the web hides unknown totals. Generated contracts, schemas and OpenAPI updated with apigen. No routes added or removed. Restriction predicates remain inside every candidate selector and hydration stays page-bounded.

## Schema changes

Edited the baseline and ran schema-sync. Added the published availability state, its indexes and retained counters/rollups; class availability counts; random class key/index and ordering 5 blocks; scoped prefix postings/counters in both search slots; an interned text/numeric prefix dictionary and its range index; cached search scope/restriction inputs; a partial ambiguous-permission-key index. Domain 18 version 3 → 4; domain 17 version 4 → 8. Existing per-item write selectors changed without changing their output.

Measured plan changes (large-tier timing logs will be added below):

| Path | Starting plan | Updated plan / bound |
|---|---|---|
| Deep availability page | Materialize all candidates, window-rank the match set, temporary sort, then hydrate | Covering availability equality seek; materialize only `LIMIT 40 OFFSET 50000` identities; hydrate by primary key |
| Grouped text prefix | Correlated FTS row-ID/MATCH invoked for each member; large prefix posting work can repeat | Per-entity prefix reverse-index seek plus vocabulary primary-key check for each query word |
| Broad relevance prefix | FTS matches plus full relevance/title-bonus ordering | Prefix primary-key equality on term/kind/library/tier, separate stable-ID continuation arms; each stream capped at page+1; temporary merge contains only bounded streams |
| Deep seeded random | Up to 500 cursor-page SQL walks at the requested range | Counted maintained random blocks locate a rotated start; at most two bounded position runs and one page-sized hydration |
| Equal-value block split | Covering index constrained only to library/kind; OR residual walks the equal-value prefix | Two disjoint key seeks, equality arm includes entity ID; merge at most 1,025 rows per arm |
| Dirty acknowledgement | Tuple membership uses two JSON lists and a Bloom filter; the default-size plan already probes the primary key | Primary-key search `(domain,entity_id)` plus revision guard; Eight guarded deletions per production checkpoint (the tier stresses 16) |
| Availability total | Membership/asset correlated predicates repeated over matching titles | Maintained rollup or published restriction-class count; ambiguous cold policies retain authoritative predicates |

The acknowledgement change removes tuple/list planning and retains explicit per-key revision guards; no queue-scan speedup is claimed from the default-size plan, which already used a primary-key search. At 1m queue rows, the guarded 16-key batch took 0.817 ms and both before/after plans probe the primary key.

The before availability plan is saved in `before-matrix-200000.log` lines 894–895. Correlated block/queue/page tier logs record actual `EXPLAIN QUERY PLAN` evidence. The updated exercise captures prefix plans with `-explain search/Title`.

Tier observations at 1m (fixture creation is excluded from these read/checkpoint timings):

| Check | Before | After |
|---|---:|---:|
| Correlated equal-key block selector | 163.768 ms | 1.036 ms |
| Guarded 16-key acknowledgement | Original plan also uses primary key | 0.817 ms |
| Grouped text query with 1m unrelated FTS matches | 86.166 ms | 0.840 ms |
| Filtered title at offset 50k | Full ranking on base | 6.825 ms |
| Filtered file-size at offset 50k | Cursor-page walks on base | 20.640 ms |
| Filtered seeded random at offset 50k | Full/walked ranking on base | 327.377 ms |
| Text-prefix single-group medians, open | Search timeouts in route matrix | 1.46–1.77 ms |
| Text-prefix single-group medians, restricted | Search timeouts in route matrix | 3.49–4.07 ms |
| Exact zero-visible prefix result | Candidate stream could still walk | 0.514 ms |

Prefix numbers above use `Ti`, `Tit`, `Title`, `Longpre`, `Longprefix`. The numerical boundary fixture was corrected to give the millionth padded number terminal `000001`, and its rerun includes `Title 0` plus checks that returned numeric titles actually match. Counted random is checked separately against an exhaustive reference in both read models; revised tiers are queued after the exercises.

## Owner or viewer settings added or wired

None.

## Needed from others

- Native/client lanes must sync shared contracts/client-core and hide unknown search totals using `totalCountExact`.
- Architecture prose must describe prefix ranking, conditional search totals, retained counters and counted random rotations.
- Other content/listening collection paths still contain cold-class preparation gates; this lane fixes modern main browse. Generic exact Boolean/grouped predicates and heavily populated personal histories may need further maintained positions/counts.

## Test notes for other environments

Host: ehler-homelab-mini-1, Ubuntu/amd64, GOFLAGS=-p=4. Measurements are catalogue services and synthetic data, not authenticated HTTP, physical scanning, playback or device acceptance. Original read baseline: commit `a8b9a618da39391a97385ce563eed461d20132de`, preserved `before-original` executable. Hardware: Intel i5-10500T (6 cores / 12 threads), 30 GiB RAM, shared with other programme work. At 1m playables the reused mixed shape has 19,607 films, 392,156 episodes and 588,237 songs, plus containers. Narrow facts retain one genre per title and sparse credits. About 10% of playables are G, 10% R and the rest unrated; the PG-13 profile blocks unrated content. Progress/history is sparse (1%); personal rating/play-count values and optional media groups are largely empty. This is not a dense-history or one-million-film fixture.

The complete touched-package run passed compactcatalog, dbwork, facettext, persistence and scalebench. Catalog functional assertions passed; its sole failure was a guard naming the superseded full-ranking query. The guard now requires the bounded page and the entire range order/continuation file passes. Focused browse/search/generation/worker tests, schema-sync, API generation, web TypeScript checking and client search tests pass. The new default-size tests cover actual split SQL, deep hydration boundaries, prefix typing with restrictions, broad ten-character text prefixes, Unicode numeric prefixes, ambiguous labels, cursor dictionary growth, cold restricted browse and maintained counts. Windows/amd64 and macOS/arm64 server cross-compiles pass. Initial tiers through 1m passed for queue acknowledgement, bounded page hydration, text/numeric prefix search and grouped prefix filtering. Direct filtered-random selection was 327.377 ms at 1m, above the median aim; it is distinguished from the counted unfiltered path. The page tier now also checks counted seeded random against an exhaustive reference in open and cached visibility scopes, both directions, three samples and a bounded statement count. Its default-size run passes; the revised large tiers run after the capacity exercises. The one allowed full server suite remains pending.

To reproduce the capacity checks cold, run one command at a time from the worktree’s `server/` directory, on a disposable local filesystem with the same Go/SQLite build. The `deep` tier includes 10k, 200k and 1m; the subtest selector below limits each invocation to 1m. Check scratch size before each invocation, and keep temporary databases outside production data:

```sh
GOFLAGS=-p=4 PORTICO_PERFORMANCE_TIER=deep go test ./internal/compactcatalog -run '^TestBlockSplitsSeekInsideEqualKeyRuns$/^1000000$' -count=1 -v
GOFLAGS=-p=4 PORTICO_PERFORMANCE_TIER=deep go test ./internal/compactcatalog -run '^TestDerivedAcknowledgmentSeeksOnlyItsBatch$/^1000000$' -count=1 -v
GOFLAGS=-p=4 PORTICO_PERFORMANCE_TIER=deep go test ./internal/catalog -run '^TestFilteredPagesHydrateOnlySelectedRows$/^1000000$' -count=1 -v
GOFLAGS=-p=4 PORTICO_PERFORMANCE_TIER=deep go test ./internal/catalog -run '^TestBroadPrefixesSelectBoundedScopedPages$/^1000000$' -count=1 -v
GOFLAGS=-p=4 PORTICO_PERFORMANCE_TIER=deep go test ./internal/catalog -run '^TestGroupedSearchDoesNotMaterializeUnrelatedMatches$/^1000000$' -count=1 -v
```

A pass requires the query-plan/statement-count and result assertions, not just a faster number. The random tier compares both directions and both scopes against exhaustive rotation order. The class fact range checks 40 live-authorized rows and selection from the class before hydration. The numeric boundary checks actual returned title prefixes at the millionth ID; the dictionary-growth test checks both fresh queries and continuation without duplicate IDs. Logs retain before/after EXPLAIN output.

On Windows/macOS and clients: repeat the scoped tiers sequentially; verify retained content during empty/version rebuilds, no hidden cards, stable seeded wrap/cursors, metadata changes reflected by the class updater, and quiet refresh preserving current cards. Unknown search totals must stay hidden and navigation must remain available. A profile permitting no matches must return an exact empty result without a prefix-page query; the new default-size test passes and the same assertion is in each capacity tier.

## Not finished

Fresh before measurements at both sizes, the 200k updated exercise and the initial tiers are complete. The million-title exercise, revised counted-random/numeric-boundary tiers, the full server suite and final cleanup/snapshot remain. The updated 200k empty rebuild took 44.1 minutes; no rebuild-throughput improvement is claimed. Universal <100 ms browse/search and zero failed rebuild requests are not yet established. Full-text fallbacks (including non-relevance search sorts), generic exact grouped filters, cold complex policies, dense personal histories and atomic large actions remain possible scale limits and will be reported honestly.
