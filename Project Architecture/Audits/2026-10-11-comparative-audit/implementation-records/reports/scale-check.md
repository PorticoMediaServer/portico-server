## Outcome

Partial completion. The lane has working fixes and measured regressions, but it does **not** establish that a complete mixed library of 200,000 or 2,000,000 titles stays responsive. The full browse matrix already exposes slow requests at 10,000 playable titles. Do not treat this report as capacity acceptance.

Fixed:
- Browse continuation and counted-block page selection merge two bounded indexed runs: equal key with an ID bound, and greater/lower key. This avoids a library-prefix scan without regressing long runs of equal years, dates or absent metrics.
- Deep personal-rating, last-played and play-count ranges use title blocks when the profile has no values for that sort. Existing cursor phases and restriction checks remain in force.
- Missing-title totals and needs-attention totals use synchronously maintained per-library counts. Global missing pagination has a time/ID index and explicit library-specific predicates.
- Attention pages resolve public identities with the page and fetch their top three candidates together. Page query count no longer grows with page length.
- Abandoned-title taste adjustments load a bounded 200-item window in two statements, preserving repeated work contributions and positive-signal exclusions.
- Extended the existing scalebench with facts-only seeding, vocabulary measurements, rebuild/checkpoint probes, three profiles, 1–3 new film versions, event replay, a 500-episode action and undo, removal and memory measurements. Retained a cheap default index guard and 10k/200k/2m performance-tier variants.

### What was measured

Host: Ubuntu `ehler-homelab-mini-1` (personal mini-1), branch `audit-implementation/scale-check`, Go 1.27.1/amd64, `GOFLAGS=-p=4`. These are catalogue service measurements on a shared host, not authenticated HTTP, real playback, physical scanning or a full server process. Linux RSS samples are in KiB. Except the initial two-sample reference run, routes have **one sample**; p50/p95 labels in the harness therefore describe the same observation, not a statistical distribution.

The existing reference writer produced about 2% films, 39% episodes, 59% songs, shows/seasons, 12-track albums/artists, six credits and three genres per playable item, content ratings on one in five and progress on one in a hundred. The final fixture code seeds three profiles, one measured with PG-13 restrictions. The largest show in the reference fixture has 1,100 episodes and its largest artist about 5,000 tracks. Newly inserted films have 1–3 versions. The 500-episode scenario adds ten seasons of 50 episodes.

| Mixed catalogue observation | Result | Evidence/qualification |
| --- | ---: | --- |
| Original 10k seed, writes plus derivation | 160.123 s | Existing reference helper, before fixes, one profile |
| Original database per playable item | 8,091.648 bytes | 80,916,480 bytes; includes facts, derived data and indexes |
| Original unrestricted Home | 463.9–563.9 ms; 479 statements | Two samples; no throughput acceptance |
| Home after batched abandonment reads | 798.6 ms; 81 statements | One sample; query-count reduction proven, wall-clock improvement **not** proven |
| Restricted Home in that run | 551.2 ms; 91 statements | One sample |
| First import from queued facts / empty browse | 269.290 s | 840 checkpoint probes; max combined browse/search 592.4 ms; max gate hold 540 ms. One failed harness invocation had committed a 16-unit search batch before the measured resume; browse was empty. This is not a pristine-zero search rebuild timing. |
| Browse/search derivation version bump | 47.421 s | 119 checkpoints; max combined read 447.0 ms, max gate hold 83 ms |
| Bump with explicit core-group success checks | 69.806 s | Later ~11–12k mixed fixture; 156 probes; max read 553.9 ms, max gate hold 186 ms |
| First 1,000-film insertion | 2.057 s synchronous + 13.743 s derivation | 15.800 s through full derivation; original amplification instrument was invalid and is not used |
| Checked fresh 1,000-film insertion into existing mixed catalogue | 2.596 s synchronous; 12.681 s through publication | First search visibility 261.2 ms after derivation starts, about **2.857 s** from insertion start; all 1,000 confirmed searchable |
| Write amplification for that insertion | 203,624 writer `total_changes()`; 203.624/title | Includes trigger/derived changes; not bytes, SQL statement count or a full scanner/provider workload |
| 10,000 event append/replay burst | 370.3 ms, 100 pages, all 10,000 replayed | No HTTP SSE fanout/subscriber/network measurement |
| Mark 500 episodes watched | 442.8 ms; gate hold 442 ms | Atomic existing action, service-level synthetic owner grant |
| Undo that action | 208.7 ms; gate hold 208 ms | Existing undo receipt; final harness also checks counts |
| Remove Movies library | 5.708 s total; max gate hold 173 ms | Real removal adapter in bounded transactions, synchronously advanced after a real operation receipt; scheduler dispatch latency not measured |
| Reusable pages after removal | 3,645,440 bytes | Database before removal 104,083,456 bytes; database file does not shrink automatically |
| Missing list | 38.8 ms; 3 statements | Empty missing list in the mixed fixture; non-empty pagination/count transitions covered by functional test |
| Needs-attention list | 2.13 ms | 50 pending entries, actionable total zero; populated-candidate and retirement tests cover the maintained counters and batching |

A fresh **three-profile** run completed successfully, including explicit success checks for every selected search group and watched-count checks after marking and undoing the 500-episode show. Before deletion it had 100 progress rows per profile (33 completed for each of the two additional profiles); after the Movies removal it retained 98 rows per profile. This clean run supersedes the resumed empty-rebuild timing above:

| Fresh three-profile run | Observation | Process RSS KiB / writer hold |
| --- | ---: | --- |
| Derive from empty | 241.740 s; 785 browse/search checkpoints, no failed core groups | 68,840 KiB; max hold 315 ms; combined read max 217.3 ms |
| Version bump (browse + search) | 45.496 s; 117 checkpoints | 85,616 KiB; max hold 42 ms; combined read max 263.7 ms |
| Add 1,000 new films with 1–3 versions | 2.012 s synchronous; 10.834 s through confirmed publication | 82,688 KiB; derived hold max 33 ms |
| First new film appears in search | 234.9 ms after derivation starts; about **2.247 s** from insertion start | Unique query namespace excludes previously added films |
| Amplification | 206,805 writer changes, 206.805/title | Includes derivation and checkpoint-induced work |
| Unrestricted Home, cold | **13,913.531 ms**, 80 statements | 69,764 KiB; unresolved slow observation, no attribution proven |
| Restricted Home | 461.987 ms, 91 statements | 70,544 KiB |
| Search Ti / Tit / Title | 152.4 / 172.0 / 240.7 ms | About 71–72k KiB |
| Search Show / Album / Artist / Person | 77.4 / 119.8 / 122.7 / 481.6 ms | About 71–72k KiB |
| Missing / needs-attention | 61.6 / 3.0 ms | 76,996 / 77,152 KiB; list population qualifications remain above |
| Event append/replay burst | 379.6 ms; all 10,000 events, 100 pages | 82,384 KiB |
| Mark / undo 500 episodes | 466.3 / 212.3 ms | 82,000 / 82,076 KiB; holds 466 / 212 ms; counts verified 500 then zero |
| Remove Movies library | 5.709 s | 82,244 KiB; largest hold 234 ms |
| Reusable pages after removal | 4,186,112 bytes; before file 104,935,424 bytes | No mixed-database VACUUM measurement |

The cold Home outlier is a failure to establish responsiveness even at 10k. No cause was established; it must not be discarded as contention without profiling. The 80-statement count is substantially better than the original 479, but lower statement count does not establish acceptable latency.

### Browse coverage and remaining slowness

[Complete browse measurements](scale-check-browse.csv): **1,084 requests**, 542 first pages and 542 deep ranges, across movies/movies, movies/categories, movies/collections, TV/shows, TV/episodes, TV/categories, TV/collections, music/artists, music/albums, music/songs and music/genres. Every published sort direction and one legal representative predicate for each advertised filter field was exercised. This is not every operator, Boolean combination, grouped detail route or audiobook/live-TV surface. Deep requests used start 50,000 and clamp to the last result when the fixture is smaller.

Median first-page observation: **5.221 ms**; slowest **631.933 ms**. Median deep observation: **4.840 ms**; slowest **14,514.930 ms** (`music/genres/filter/playState/deep`). Other grouped deep filters exceeded ten seconds. These slow cases remain unresolved.

| Personal sort on the mixed Songs view | Before deep | After deep | After statements |
| --- | ---: | ---: | ---: |
| Last played, descending, empty values | 2,062.333 ms | 37.738 ms | 15 |
| Personal rating, descending, empty values | not independently paired | 43.036 ms | 15 |
| Play count, descending, empty values | not independently paired | 36.953 ms | 15 |

Search-as-you-type (10 core groups, all checked successful; optional Live TV excluded because this fixture has none): Ti 223.6 ms, Tit 162.1 ms, Title 162.3 ms, Title + prefix 134.8 ms, Show 66.2 ms, Album 115.4 ms, Artist 151.9 ms, Person 504.7 ms. Exact broad-match ranking and totals still grow with the match set and have a 600 ms group budget.

### Three-size index traversal proof

This uses a **narrow synthetic index database**, not a complete mixed catalogue. It cheaply isolates the actual SQL selector and index definitions so a whole-library scan cannot hide behind a small fixture. It includes unique titles and deliberately identical years/release dates/absent added dates, durations and ratings. All three sizes passed; each row selected and its ID order were checked. `ANALYZE` statistics and complete `EXPLAIN QUERY PLAN` output are retained in the evidence log. SQLite's planner reports structural access paths, not actual rows visited; actual examined-row counters were not collected.

| Rows | Old deep title selector | New selector | Database bytes | Process peak KiB | Reusable bytes after half-delete | Bytes after VACUUM |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 10,000 | 2.612 ms | 0.557 ms | 1,687,552 | 27,820 | 811,008 | 811,008 |
| 200,000 | 51.174 ms | 0.638 ms | 35,139,584 | 32,824 | 17,731,584 | 15,777,792 |
| 2,000,000 | 674.204 ms | 0.917 ms | 355,168,256 | 62,912 | 177,561,600 | 160,686,080 |

The old title query planned only library/kind constraints. Each new query has an equality arm constrained by key **and ID**, plus a next-key range arm. Each arm reads at most the page plus the within-block offset; the final merge sorts at most twice that bound. A temp B-tree in that bounded merge is expected. An initially attempted tuple rewrite regressed equal-value seeks on the server driver; it was discarded and is absent from the final tree. The system Python SQLite planner was not sufficient to choose the production SQL.

## Decisions I made

- Kept exact response shapes, totals, filters, sort semantics and restriction checks. No lower-bound totals or silent candidate truncation were introduced to conceal a slow query.
- Used synchronous counts for owner lists because they are facts, with a small update per changed row. No background generation is needed; no derivation version changed because derived content did not change.
- The no-value personal-sort shortcut is conservative: if any valued row exists in that profile/library/kind, it keeps the existing path, including cases where that row is hidden.
- The current public shape cannot make arbitrary exact filtered totals and every arbitrary direct range bounded just by replacing an OFFSET with a keyset loop. Those cases need further read-model work; the remaining loops are called out below.
- Context refresh failed at its sync lock because that directory is read-only. Existing project, standards and verification notes were read. No push, deployment, commit, model helper or sub-agent was used.

## Bugs

The lane brief names no numbered domain audit documents. Domain 07's bug section and the bug register/fix log were nevertheless checked against the current source:

- B-07.1 — already fixed: poison isolation, retries, fair step-worker turns and poison rebuild completion exist on the base; no rewrite here.
- B-07.2 — already fixed on base: retained relational rows and two FTS generations serve during rebuilds. Measured checkpoint responses; full-scale HTTP/UI acceptance remains absent.
- B-07.3 — already fixed: moved/reused place tests and title/file identity handling exist; focused reused-place test passed.
- B-07.4 — not fully fixed here: ordinary progress ticks no longer bump the viewer revision, and browse uses key continuations; search/general revision granularity still needs the API/freshness lane's reconciliation.
- B-07.5 — already fixed: search reads its revision through the same snapshot; concurrent-change snapshot test exists.
- B-07.6 — already fixed in source: punctuation folding and one-character search supported; no search normalization change here.
- B-07.7 — already fixed in source: CJK terms plus normalized substring checks exist; no tokenizer redesign here.
- B-07.8 — already fixed in fix log and current date compiler; no raw date comparison rewrite here.
- B-07.9 — already fixed: seek uses `sorttext.Letter`, not the first UTF-8 byte.
- B-07.10 — not fixed here: the shared client helper now supports Unicode, but article/sort-key handling across its callers was not established in this lane. Needs client-lane verification.
- B-07.11 — not present: the raw catalogue item-delete route/method described by the audit is gone; retained-title removal paths exist.
- B-07.12 — already fixed in source: descriptive filters use `portico_fold_text`; no case/accent change here.
- B-07.13 — already bounded on base: backup has a two-minute snapshot limit and 64 MiB WAL-growth limit. Multi-gigabyte backup success under scanning was not measured.
- B-07.14 — not present: only the directly edited baseline ships; old development migrations are gone.
- B-07.15 — already fixed in source: bounded dictionary/identity/previous-generation retention exists. Long-term churn capacity was not measured.
- B-07.16 — already fixed in source: `dbwork/health.go` calls the stuck-holder check outside strict mode.
- B-07.17 — already fixed: title ordering uses the entity sort key and release year uses the stored entity year.
- B-07.18 — original 64-unit batch no longer present: production limit is 16. Measured holds are reported above; huge-container and million-title acceptance still missing.
- B-07.19 — already documented: persistence README directs external inspection to a backup copy. Deliberate external-reader/write-lock interference was not tested.

New findings without existing bug IDs: library-prefix seek scans (fixed), long tied-key tuple regression (discarded), empty personal-sort deep walks (fixed), owner-list recounts/candidate round trips (fixed), abandoned-taste round trips (fixed), grouped filtered deep ranges and broader direct-range walks (not fixed).

## API changes

None. Routes, typed registry contracts, key scopes, restriction classifications and non-UI consumers keep their shapes. No apigen run is required because no route or contract changed. Attention total still excludes in-progress reasons even when the page includes pending rows, preserving its existing meaning.

## Schema changes

Edited the baseline only; ran `python3 scripts/schema-sync.py` and updated the known-object and known-column inventories.

- `catalog_missing_title_counts(library_id,total)`, maintained on insert/delete/library move, cascading with its library handle.
- `catalog_missing_titles_time(missing_since_ms,entity_id)` for global key pagination.
- `screen_metadata_attention.retired`, initialized from the title, updated on title retirement and preserved when work recreates its row. Parent disappearance during cascades resolves as retired.
- `screen_metadata_attention_counts(library_id,total)`, maintained for undismissed, active, actionable entries through reason/generation/dismissal/move/retirement/deletion changes.
- `screen_metadata_attention_active` partial index.

No compatibility shim, old-database upgrade or derivation version bump. Initial counts are generated by fact mutations, not copied at startup.

## Owner or viewer settings added or wired

None. This lane changes query and counter implementation, not user preferences.

## Needed from others

- Library/query lane: extend maintained position support to indexed fact sorts and populated personal sorts. `walkIndexedRange` still walks all earlier pages for direct ranges without blocks. Grouped filtered membership paths also need bounded query plans; see the CSV's 10–14.5 second examples.
- Library/query/API lanes: exact arbitrary filtered totals currently scan matches on a cold query/revision. Keep exact semantics or explicitly coordinate a changed contract with every client; do not hide this by claiming a keyset loop is bounded.
- Home lane: investigate the fresh three-profile cold Home observation of 13.9 seconds, including serving-view plans, cache state and planner statistics after an empty derivation. Do not explain it away without evidence.
- Search lane: broad-prefix exact ranking/counts scale with matches and can exhaust group budgets. A measured plan for bounded as-you-type results is still needed.
- Integration: rerun the retained harness after other lanes' scanner/title/version/derivation/query changes. This worktree measures the shared starting base plus this lane only.
- Architecture documents: domain 07 and the older catalogue architecture prose describe preparation failures/old migration behavior that the current base removed. Update the canonical documents elsewhere.
- Client lanes: verify article/sort-key bucket re-anchoring and the freshness/revision changes; there is no new screen or string from this lane.

## Test notes for other environments

Passed focused Go tests for all browse orders and ad hoc paging, retained generations/upserts, missing-count changes, attention retirement/dismissal/deletion/top-three candidate batching, abandoned taste, reused title places, removal leftovers, reference seed resume and writer connection accounting. The deep index proof ran at all three sizes. `go build ./...` and `go vet` on touched packages passed. Final schema/known-schema tests passed (12.985 s); poison/step fairness/rebuild completion tests passed (2.192 s). No full suite, native build, deployment or full load test was run.

Run from `server/` with `GOFLAGS=-p=4` (or `-p=2` on a more contested host):

```bash
PORTICO_PERFORMANCE_TIER=deep PORTICO_SCALE_SCRATCH=/absolute/disposable/scratch \
  go test ./internal/catalog -run TestBrowseCompositeSeekCapacity -count=1 -v -timeout=30m
```

The narrow test deletes its databases on completion. Default tier uses 256 rows; release uses 10k/200k; deep adds 2m. A pass requires correct IDs/order, indexed bound plans and deep reads under the tier threshold. This does not prove full catalogue capacity.

For a real mixed fixture, use a new `.scratch` directory:

```bash
go run ./internal/catalog/scalebench -dir /absolute/.scratch/catalog -items 10000 -facts-only -samples 1 -idle 1s
go run ./internal/catalog/scalebench -dir /absolute/.scratch/catalog -items 10000 -reuse -exercise -samples 1 -idle 1s
```

`-matrix-only` keeps the write/rebuild scenarios out. `-scenarios-only` skips the browse matrix. The full exercise removes the Movies library at the end. Repeat on new databases for 200k and 2m, serially within the 20 GB scratch limit. Keep logs/JSON, then delete the database/WAL/SHM files. Fixtures are phantom media facts, not real playback or provider matching. Linux process RSS is measured; on Windows/macOS use the host process monitor, and do not interpret the tool's fallback zero as measured memory.

Evidence is in `_program/scratch/scale-check/`: baseline/after reference logs, `exercise-10000.log` (complete matrix before optional-Live-TV harness selection was corrected), `scenarios-10000.log`, `seek-capacity.log`, checked write/publication logs and the fresh three-profile log. Reports retain the CSV. Cleanup verified: removed six owned database/sidecar files totaling 406,949,888 bytes. Scratch now contains 261,568 bytes of logs and JSON measurements; the browse CSV is in reports. No databases remain. Cumulative snapshots 001 and 002 were taken after compile/test groups; the final cumulative snapshot is taken after this report and cleanup.

## Not finished

- No complete mixed 200k or 2m fixture/rebuild/addition/Home/search/list/watch/removal/event/memory run. Only the narrow index/database reclamation proof reached those sizes.
- Cold unrestricted Home returned in 13.9 seconds in the final three-profile fixture; its cause remains unprofiled.
- Grouped filtered deep pages, arbitrary filtered exact counts, populated personal-sort direct ranges and newer fact-sort direct ranges remain proportional to matches/depth. Their measured 10k slowness is real.
- No cold-cache modest-hardware capacity, full server RSS, concurrent authenticated browse/owner-console availability, SSE fanout or live scanning/matching/provider workload acceptance.
- No exhaustive sort/filter combinations on every detail/grouping surface or audiobook/live-TV fixtures; the CSV covers the listed eleven pivots and one predicate per filter field.
- Missing-list mixed-fixture timing is empty; actionable attention was functionally verified with populated candidates and counts, not measured with millions of actionable rows.
- Mixed-database post-delete physical shrink was not measured with VACUUM; only reusable pages were counted. VACUUM reclamation was measured at all three narrow index sizes.
