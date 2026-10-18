## Outcome

Implemented indexed grouped filtering/ranges, sparse personal-state reads, a fast initial Home manifest with independent row hydration, bounded recommendation/recent previews, cached search ranking inputs, cheaper no-op derivation, and batched container actions. Same-route 200k show subgroup genre-missing first/deep reads improved 4,173.929/3,304.182 → 15.247/7.742 ms. Cold complete-metadata 10k Home improved 14,540.290 → 101.033 ms. At one million, the initial Home manifest is 0.420/0.257 ms open/restricted; full row composition is 552.840/577.835 ms. The final grouped-text tier proof at two million matching documents improves 1,469.247 → 138.802 ms with identical results.

Saved 6,945 route/write observations at each of 10k, 200k and the authorized one-million fallback. Two million full mixed titles would exceed the new 8 GB scratch limit; only separately labelled structural tests use that size. One-million empty/bump rebuilds took 12,548.106/2,452.618 s, with 215/687 failed group/request probes and 2,090/1,618 ms maximum writer holds. Six broad search queries still fail at 200k and one million. These are failed capacity criteria: every advertised operator is retained, but every sort/filter is not yet fast at scale.

The one-million library removal originally stalled on repeated rolled-back 24-title batches. Four-title transactions completed the resumed removal in 1,435.237 s, maximum writer hold 1,004 ms. Reclamation recorded 145,993,728 reusable bytes; physical VACUUM was skipped for the scratch budget. All synthetic databases have been deleted. Observed scratch checkpoints peaked at 6,830,559,232 bytes, below 8 GB; the harness guards allocations separately. Focused tier/functional tests and the full server build pass. Snapshots 001–018 preserve the compiling implementation; 018 is final. Final `go build ./...` and `git diff --check` pass. Cleanup found no SQLite/database/WAL/SHM files in the scratch root; its empty temporary directories were removed. Final scratch allocation is about 1.59 GB (cache, binary and evidence). No code changes follow snapshot 018.

## Decisions I made

- Preserve all advertised sorts and operators, exact browse totals and restrictions inside SQL. Direct filtered ranges evaluate their matching membership/order once; arbitrary cold totals still cost work over matches. No universal maintained position index for every predicate is claimed.
- Personal flag/rating/date predicates now start from matching profile state and reverse member indexes, preserving direct container intent and actual-member restrictions. Negative group membership reuses the indexed positive selection. All three final sizes use this change.
- First Home response is a bounded row manifest; client-core composes two rows concurrently and publishes each independently. A failed or stalled row times out without delaying the manifest or another row. Quiet refresh preserves existing cards. Viewer fences apply to rows, children, hero and pagination.
- Recently Added examines at most 4,096 candidate works; abandoned recommendations inspect the newest 200 old unfinished activities. Complete library browse is retained. Recommendation descriptors keep the most recurring 64 eligible member traits; complete source/browse metadata is retained.
- Use the production SQLite driver and mixed fixture. Narrow large fixtures keep one genre per title, credits on films and representative episodes, all films' one-to-three versions, shows/seasons/episodes, artists/albums/tracks, three profiles and restricted viewers. This is fixture setup, not scanner throughput; adding 1,000 still uses the write API.
- Final narrow 10k uses 5,342 bytes/title and final 200k 4,618 bytes/title. Two million projects to at least 9.24 GB before WAL/cache, exceeding the owner's new 8 GB total limit; use the authorized one-million fallback. One large benchmark at a time. Run `du` before each seed; budget includes lane-local Go cache/logs. Harness stops at 7 GB, preserving headroom; production WAL housekeeping runs during seed/derive. Delete each database after saving results; skip physical VACUUM when its extra allocation cannot fit.
- Rebuild continuity probes use selective prefix `Title 000000`; broad as-you-type prefixes are separately measured. This differs from the original full-fixture `Title` continuity workload.
- Search ranking uses precomputed folded titles and public identities, preserving exact scores and tie order. These eliminate three per-match conversions; both search slots carry the fields.
- Continue fallback begins at the show member index, then seeks that profile's activity by item; a denied/completed latest item does not rescan the entire profile for each show.
- Play-state queries evaluate canonical watched/default precedence only for candidate items with explicit watched intent, positive progress or a watched container default. Unplayed selection excludes only affected entities with no eligible unplayed member, preserving empty entities and live member restrictions. Parity against the former expression covers every state subset, negatives, two profiles, age restrictions, newer overrides, cleared intents, unknown/future additions and empty containers.
- During rebuilds, empty retained overlays use indexed base reads; a referenced table with retained rows still selects the original views. Overlay presence is resolved within the same snapshot. This changes plans, not published content.
- Broad genre presence/missing predicates use correlated member seeks; selective values use vocabulary-first expansion. All final sizes use this adjustment.
- No-op browse/sort/metric/Home derivation skips equivalent writes; counted buckets move only changed axes. All seven show attribute fields are resolved in one member walk. These preserve content and require no version bump.
- Rebuild source enumeration advances only through a queued source prefix whose members actually finished; gaps and unfinished fanouts stop advancement.
- A failed generation lookup now fails scalar `Scan` instead of returning the active search slot as accidental content. The focused regression and full server build pass; this follow-up changes only the failure path, so successful mixed measurement results still apply.
- Final grouped text-search follow-up: a required positive genre/memberOf/collection conjunction uses an FTS row-id probe, avoiding materializing every matching document before intersecting the group. Plain, alternative and negated group searches keep the original plan. Functional/range tests pass; the before/after tier comparison passes at 10k/200k/2m matching documents, with identical selected identities. This is a cost-only change, with no schema or derivation content change. All full mixed matrices predate this last predicate change; six grouped search observations must be read as before-fix numbers, not final-code acceptance.
- Library removal now commits four playable titles per transaction instead of 24. The old 1m run endlessly rolled back its first batch under the harness’s one-second step context; the same fixture completed. History retention regression and full build pass. The interrupted run’s raw 6,945 log measurements are preserved; it did not flush baseline DB size or visibility-build timing. The resumed removal is measured separately: 1,435.237 s with a 1,004 ms maximum writer hold; all movie entities are gone, post-removal derivation finished, and reclamation is recorded. The failed cheap-helper diagnosis returned no output; diagnosis and changes were completed directly.
- Full plan tracing is opt-in, omits bound values, and caps 1,024 statements at 64 KiB. Ordinary tracing remains at 110 characters.
- Actual host: personal mini-1 (`ehler-homelab-mini-1`). Context-sync could not acquire its read-only lock; existing project/standards/verification guidance was read. No commits, push, deployment, full suite or extra agents.

## Bugs

Read domain 07's complete bug entries, the corresponding register entries and fix-log records. The numbered issues remain base-branch work unless noted:

- B-07.1 — already fixed on base: fair step-worker turns, poison isolation/retry and rebuild completion. Focused poison/fairness tests pass after the rebuild-prefix change.
- B-07.2 — already fixed on base: retained relational and FTS generations keep serving. This lane adds the ordinary published-generation index path; retention/reopen/upsert tests pass.
- B-07.3 — already fixed on base; no identity changes in this lane.
- B-07.4 — partially fixed on base; progress ticks no longer move browse membership revisions. General revision granularity remains integration work, not changed here.
- B-07.5 — already fixed on base: search revision reads its snapshot.
- B-07.6 — already fixed on base: punctuation folding and one-letter searches exist.
- B-07.7 — already fixed on base: CJK normalization and substring validation exist.
- B-07.8 — already fixed on base: date periods are compiled, not raw text comparisons.
- B-07.9 — already fixed on base: seek uses `sorttext.Letter`.
- B-07.10 — not changed here; client article/sort-key re-anchoring needs client-lane reconciliation.
- B-07.11 — not present: the raw item-delete route is absent.
- B-07.12 — already fixed on base: text predicates use Unicode folding.
- B-07.13 — already bounded on base; large busy-database backup acceptance not measured here.
- B-07.14 — not present: only the directly edited baseline ships.
- B-07.15 — already addressed on base with bounded retention/pruning; long-term churn not measured here.
- B-07.16 — already fixed on base: production health checks report stuck holders.
- B-07.17 — already fixed on base: stored year and sort key are used.
- B-07.18 — production batch size is 16 on base. Measured writer holds and remaining fanout work are reported below; further capacity acceptance is not achieved.
- B-07.19 — already documented: inspect a backup copy.

Unnumbered findings fixed here: unnecessary retained-view plans outside rebuilds; repeated aggregate membership evaluation for skipped pages; repeated filtered range walks for indexed fact orders; untyped abandoned-facet JSON join; abandonment candidate bounds applied after expensive checks; unbounded recent preview fallback; queued/source rebuild overlap.

## API changes

- `GET /v1/home`: previously composed all previews serially; now publishes server-ordered row descriptors with `deferred:true`. Every descriptor carries its viewer fence and its existing preview endpoint. The first screen response does no shelf query.
- `GET /v1/home/rows/{id}`: ordinary rows carry `viewerFence` and, for initial Continue previews, the server-selected `hero`. `id=for_you` now returns the recommendation family's descriptor with at most five `children`; those are ordinary non-deferred previews and cannot nest another family. It was previously refused as an unknown row.
- Continue hero candidates now carry optional `selectionKey`, generated by the server from activity order and layout order. Each Continue preview composes only itself. Client-core selects the greatest server key among visible returned candidates, so response arrival order does not decide the hero. No routes removed or added. Existing typed registry, read key scope and restriction classification remain. Ran `go run ./cmd/apigen`; generated contracts, schemas and OpenAPI updated.
- Client-core parses the new shapes and loads at most two deferred rows concurrently. It publishes each result independently, preserves prior cards during refresh, isolates row failures/timeouts and discards replies with a different viewer fence. The web already uses this shared Home service; no screen implementation changed.
- The manifest reserves four slots for expanding the personal family, keeping a fully expanded document within the existing 64-row bound.

## Schema changes

Edited the baseline directly and ran schema-sync (including generated known schema/columns and persistence validation). Added derived `catalog_rec_member_work`, `catalog_rec_member_counts`, partial ordered `catalog_rec_member_best`, facet delta and parent-move triggers. Search documents in both slots now also store `title_fold` and `entity_pid`; domain 17 version **3 → 4** fills those cached ranking inputs. Exact relevance, totals and cursor tie order are preserved. Domain 32 version **5 → 6** rebuilds the changed recommendation descriptors. A member's parent mapping survives cascade until its queued cleanup removes the exact contribution. Also added a partial source-label override index for genre predicates; that index changes cost, not derived content.

## Owner or viewer settings added or wired

None.

## Needed from others

- First-party client lanes must copy updated shared client-core/contracts through the programme's package-sync workflow and verify Home on physical native platforms. No screen implementation changed here.
- Update architecture prose for the initial Home row manifest and independent previews.
- These are catalogue-process measurements, not authenticated HTTP, physical file scanning, playback, SSE subscriber fanout or full-server idle.

## Test notes for other environments

Evidence: `_program/scratch/scale-check-2/`. `measurements.csv` inventories measured routes, errors, statement counts and RSS; JSON/logs preserve detail. One sample per route means the exported p50/p95 are the same observation, not a percentile distribution. RSS is sampled after reads, at worker steps and after VACUUM; it is not a continuous process high-water measurement. Cold Home means a fresh service cache, with OS/SQLite pages potentially warm from seed/derive. Rebuild timing drains `Worker.Step` at the production 16-unit transaction bound with checkpoint housekeeping and concurrent read probes; the harness does not add the background worker’s production pacing delays. Main first-page counts may be cached for the subsequent deep page. `STAT1` logs and before/after `EXPLAIN QUERY PLAN` establish lookup/sort direction; they are not measured SQLite VM row counters.

| Narrow mixed measurement before final grouped-text fix | 10k | 200k | 1m fallback |
|---|---:|---:|---:|
| Facts-only seed, s | 5.922 | 156.241 | 890.232 |
| Facts-only DB bytes / title | 2526.0 | 1831.5 | 1811.5 |
| Catalogue-service idle statements / minute | 0 | 0 | 0 |
| Empty rebuild, s | 92.268 | 2220.153 | 12548.106 |
| Version-bump rebuild, s | 8.295 | 202.604 | 2452.618 |
| Rebuild probes empty / bump | 325 / 25 | 5000 / 262 | 13558 / 991 |
| Rebuild probe errors empty / bump (groups or requests) | 0 / 0 | 0 / 0 | 215 / 687 |
| Max writer hold empty / bump, ms | 73 / 31 | 562 / 340 | 2090 / 1618 |
| Max concurrent browse+search empty / bump, ms | 193.344 / 157.967 | 940.801 / 675.823 | 2864.816 / 3288.704 |
| Visibility class build, s | 0.739 | 100.076 | Not preserved |
| Initial Home manifest open / restricted, ms | 0.282 / 0.293 | 0.277 / 0.248 | 0.420 / 0.257 |
| Cold full Home open / restricted, ms | 79.784 / 102.894 | 162.268 / 815.612 | 552.840 / 577.835 |
| Continue Watching open / restricted, ms | 9.072 / 13.842 | 12.908 / 569.626 | 28.556 / 39.672 |
| Continue Listening open / restricted, ms | 5.201 / 6.638 | 63.219 / 89.371 | 340.678 / 454.259 |
| Worst actual subgroup first / deep, ms | 125.374 / 73.684 | 371.125 / 221.887 | 1730.069 / 858.057 |
| Worst main browse first / deep, ms | 96.933 / 145.311 | 1901.400 / 3464.069 | 9815.275 / 8071.082 |
| Worst search query, ms (errors included) | 115.961 | 1434.279 | 2393.391 |
| Route observations / errors | 6945 / 0 | 6945 / 6 | 6945 / 6 |
| Missing / needs attention, ms | 0.618 / 1.343 | 0.565 / 1.379 | 37.905 / 1.411 |
| Add 1,000 films total, s | 10.253 | 10.960 | 14.787 |
| Add synchronous insertion, ms | 2143.492 | 2158.073 | 2437.095 |
| First addition visible after drain starts, ms | 247.884 | 269.635 | 380.560 |
| DB changes / added title | 177.792 | 178.780 | 179.104 |
| 10k-event append/replay, ms | 448.531 | 414.668 | 429.436 |
| Mark / undo 500 episodes, ms | 73.742 / 79.127 | 208.846 / 208.352 | 757.279 / 778.483 |
| Remove Movies library, s | 6.479 | 168.062 | 1435.237 |
| Removal max writer hold, ms | 196 | 946 | 1004 |
| Derived DB bytes / playable title | 5341.6 | 4618.3 | Not preserved |
| After writes, pre-removal DB bytes / seeded title | Not separately recorded | Not separately recorded | 4864.7 |
| Before removal / post-drain reusable bytes | 65142784 / 4673536 | 999231488 / 32485376 | 4864737280 / 145993728 |
| VACUUM, ms | 1022.781 | 11968.213 | Skipped for budget |
| After VACUUM / reclaimed bytes | 54059008 / 11083776 | 863789056 / 135442432 | Not run for budget |
| Highest sampled catalogue RSS including VACUUM, KiB | 90548 | 200992 | 148452 |

The mixed matrices predate the final grouped-text predicate fix. Ten-thousand and 200k removal measurements use 24-title batches; one million uses a resumed four-title removal, excluding the interrupted attempt and subsequent derivation. The one-million original report was not flushed when the stalled process was interrupted. Raw route logs recover the measurements, but baseline derived DB size and visibility-build time were not preserved. Its 4,864.7 bytes/title is explicitly the post-write physical allocation before resumed removal, not a replacement baseline. The facts-only seed size is separately recorded. Idle observes the catalogue service only; full-server workers were not started. Physical VACUUM at one million was skipped because its estimated extra allocation was 9,729,474,560 bytes. Every measured database was deleted after saving its numbers.

Final runs measure every advertised sort/direction and operator at first and direct deep pages, including actual genre, show and artist subgroups. Deep start 50,000 clamps to each result set; the synthetic show/artist/genre groups do not supply a genuine 50,000th result. The largest artist is 5,000 tracks; early shows reach about 1,100 episodes and each narrow movie genre is small. Larger matched subgroups remain an acceptance gap. Movie, TV and music fixtures are populated; observations on unpopulated optional pivots are not capacity evidence. The full operator matrix uses the open viewer; restricted basic browse, Home rows and functional restriction/parity tests are separate evidence. Representative operands are used; this does not exhaust Boolean combinations or every operand. Full route details, RSS and statement counts are in `measurements.csv`; raw logs contain per-row Home costs, `STAT1`, query plans and write details. The missing fixture is empty; needs attention returns 50 pending entries with an initially zero unresolved counter. The rebuild error counter counts failed search groups or whole requests, not distinct probes. The event result is catalogue append/replay, not SSE network fanout. First visibility is measured from the start of derivation, after synchronous insertion. The before-delete database includes the 1,000 additions and events, so it differs from the bytes/title baseline.

Earlier complete-metadata 10k evidence is `verified-exercise-10000.json`: empty/bump 152.886/12.468 s, no probe failures, Home 101.033/137.403 ms, manifest 0.361 ms, 7,963 bytes/title. It has 1,475 route observations and predates the final batched writes. It must not be treated as an identical fixture to the narrow final runs.

Before/after grouped plans on the same analyzed driver fixture: before materializes the serving compound view, scans it, and builds an automatic facet index; after seeks library browse matches once, then primary-key seeks `(entity_id,field)`, ranking group vocabulary once. Evidence `group-plans-before-after.log`. New genre predicates resolve vocabulary and overrides first, then seek three inherited membership paths, with playable-member restrictions still in SQL. Unicode/operator/restriction tests pass. `genre-membership-capacity.log`: 10k/200k/2m unrelated memberships, 0.423/0.324/0.341 ms, indexed seeks at every size.

Same-size, same-route comparisons between the preserved before/final 200k narrow runs (one observation each on a shared host):

| Route | Before, ms | After, ms |
|---|---:|---:|
| Show subgroup, genre missing, first | 4,173.929 | 15.247 |
| Show subgroup, genre missing, deep | 3,304.182 | 7.742 |
| Artist subgroup, genre present, first | 4,023.828 | 86.925 |
| Artist subgroup, genre missing, deep | 2,070.502 | 40.557 |
| Genre group listing, played, deep | 1,637.235 | 564.384 |
| Show subgroup, file size descending, deep | 8.374 | 8.712 |

The last row was already fast before the final read changes; no improvement is claimed. The artist played deep query similarly measured 97.453 → 102.883 ms. Broad search retained the same six timeouts. Empty rebuild changed 1,974.424 → 2,220.153 s; bump changed 220.500 → 202.604 s. These shared-host observations do not establish a rebuild speed improvement.

Final grouped text predicate proof (`group-search-capacity.log`), one observation per tier:

| Matching unrelated FTS documents | Before, ms | After, ms |
|---|---:|---:|
| 10,000 | 7.103 | 1.355 |
| 200,000 | 134.664 | 13.415 |
| 2,000,000 | 1469.247 | 138.802 |

Before uses a global FTS `LIST SUBQUERY` (`0:M2`); after uses a `CORRELATED SCALAR SUBQUERY` with row-id equality (`0:=M2`) and a document primary-key seek. Both return the identical one selected title. Positive conjunctive grouping, nested/reversed clauses, plain search, alternatives, negatives and compiler reuse pass the scope regression. This removes global identity materialization; the measured FTS cost still grows with match density, so it is not proof of constant work or of a dense 50,000-member group. The tier test requires the plan and a 250 ms upper bound. Databases were created sequentially, with `du` before each tier, and automatically removed by `t.TempDir`.

Tier tests already passed at 10k/200k/2m: filtered grouped range join direction, abandoned facet lookup, maintained top-member traits, no-op browse deltas, rebuild source prefix. These isolate read-model structure; they are not full mixed-library acceptance. Recent preview with 2m stale rows: 31.455 ms, 17 statements, 4,096-candidate bound. Top-member traits: 0.187/0.196/0.176 ms, exact 64 outputs through the covering index. Rebuild prefix: two million sources derived once in 26.899 s. No-op derive: zero data changes; changed letter: three bucket statements. Functional move/delete/count parity passes.

Current atomic container actions: 8/500/5,000 members use 24 traced statements for both mark and undo. Focused 500 mark/undo 96.649/82.035 ms; 5,000 754.857/917.843 ms. Snapshot/state/profile parity, expiry, unknown/added members, changed seasons, nested and overlapping intent checks pass. The 5,000 action ceiling is preserved.

To repeat the structural checks on another host, from the worktree root set a writable lane-local `GOCACHE`, use `GOFLAGS=-p=4 PORTICO_PERFORMANCE_TIER=deep`, and run `go -C server test ./internal/catalog -run 'Test(GroupedSearchDoesNotMaterializeUnrelatedMatches|FilteredGroupRangeCapacity|GenreMembershipCapacity|SearchUsesPublishedRankInputsAtCapacity|ContinueFallbackStartsWithShowMembers|RecentlyAddedBoundsExaminedCandidates|AbandonedFacetJoinCapacity)$' -count=1 -v`. Run the compactcatalog member-count, no-op delta and rebuild-prefix files separately with the same tier; do not overlap large runs. All emitted plans must keep the selective membership/facet/activity seeks and page hydration boundary; result/restriction parity failures are failures even if latency is low. Functional Home, sparse-play-state, read-generation and container action tests run without the deep tier. The exact reproducible mixed command is in the final paragraph below.

Client Home/parser tests and TypeScript checking pass; the recovery HomeService run includes stalled shelf, timeout queue slots, family bounds, viewer fencing, pagination and response-order-independent hero selection. Server tests cover metadata-only manifest, no sibling Continue reads and hero ordering. Re-run on Windows/macOS and physical clients: initial screen must arrive before stalled row, other rows must arrive independently, quiet refresh keeps cards, account/profile switching must discard old replies. Under a rebuild, catalogue/search remain available and live restrictions still prevent hidden titles.

## Not finished

Million-title capacity acceptance is not achieved. Broad as-you-type search retains six timeouts at both 200k and one million. Empty/bump rebuilds at one million recorded 902 failed group/request probes, and writer holds still reach 2.090/1.618 seconds. Main-library availability filters reach 9,815.275 ms first and 8,071.082 ms deep; random direct deep pages retain repeated bounded keyset batch walks (515 statements and 3,464.069 ms at 200k). Continue Listening costs 340.678/454.259 ms at one million and grows with profile history. Restricted Continue Watching at 200k costs 569.626 ms. Four-title removal makes progress but still holds the gate up to 1,004 ms and takes 1,435.237 seconds. Every advertised sort/filter remains available, but their universal fast-page requirement is not satisfied.

Further work should maintain exact counters for common predicates such as availability in each published restriction class while respecting live restriction changes; maintain group/order positions for densely matched groups; bound or maintain the profile-history work behind Continue Listening; split expensive parent aggregation into smaller worker units; and attribute maximum writer holds to their individual transactions. Broad exact search ranking/totals need a faster read model. Raising the unchanged 600 ms group budget would mask the failure. Generic Boolean predicates still require an exact fallback whose work can grow with matches. These are recommendations, not implemented fixes.

A full two-million mixed fixture cannot fit the owner’s 8 GB budget, so the authorized one-million fallback is used. Genuine 50,000th matched subgroup pages, populated optional audiobook/Live TV pivots, dense missing/unresolved queues, full restricted operator combinations, physical clients, scanner/network throughput, SSE fanout and full-server idle remain unmeasured. Baseline derived size and visibility-build duration at one million were lost when the stalled removal process was interrupted; scoped seed/post-write size measurements survive. Physical VACUUM at one million was skipped for budget. The six grouped-text mixed observations predate the final fix; separately labelled tier evidence verifies it, but a complete mixed-matrix rerun on the final tree was not performed.

Focused catalogue/compactcatalog/dbwork/HTTP tests, client Home/parser tests, web TypeScript checking, schema-sync validation, vet and the server build pass. Search-cache tiers measure 3.770/4.172/5.094 ms for selective search at 10k/200k/2m; Continue fallback at 200k/2m is 9.685/9.742 ms through a show-keyed covering index and activity primary-key lookup. These structural tests do not erase the mixed-library failures above. The Continue plan assertion was corrected to accept SQLite’s equivalent covering episode-position index with `show_id=?`; no weakened result/restriction assertion.

Reproduce mixed measurements sequentially with `bash ../_program/scratch/scale-check-2/run_final.sh` after building `./internal/catalog/scalebench` into that scratch root with lane-local `GOCACHE` and `GOFLAGS=-p=4`. It seeds one narrow mixed baseline at a time, checks the budget, measures all operators and writes, saves JSON/logs and deletes the database before the next seed. Scope is 10k/200k/1m. Two-million structural tests remain separate; run their tiers sequentially and check `du` before each seed. Final cleanup and snapshot evidence is recorded in Outcome.
