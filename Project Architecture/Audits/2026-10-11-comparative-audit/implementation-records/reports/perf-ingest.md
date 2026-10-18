# perf-ingest — the cost of adding, changing and removing items

## Outcome

**Is the time to add an item constant as the library grows?** For the catalogue itself, yes — before and after: the write API plus derivation costs the same per item from 20k to 200k items (flat curve, identical row changes per item), and adding a film, episode, song, metadata write, progress tick, watched action or removal costs the same at 5k, 20k, ~100k and 200k. **On the real scan path it was not**: before this pass, every scan (even one that found a single new file, even an unchanged rescan) re-derived every title of the library, so the cost of adding a file rose linearly with the library (derivation per new file 13.8 → 33.0 ms from 2k to 12k files; one new file in a 12k library cost 75,041 derived units and waited behind 25,508 of them before showing in browse). That is fixed: derivation per new file is flat (8.8 → 9.8 ms), and one new file costs 34 derived units and shows after 6.

The other growth found and fixed was in the work that runs beside a scan: the metadata and artwork queue claims sorted the whole backlog inside the write gate once per provider request (1.3 → 7 → 92 ms at 2k → 20k → 200k pending; now a constant 0.4 ms), the minute-by-minute scan history pruning read the whole subtitle listing once per retained run (1 → 122 ms; now flat), the scan's directory-alias check was quadratic in the number of folders, removing a file read the whole analysis queue, and the MusicBrainz claim read the whole backlog during every rate-limit pause. The scan never calls a metadata provider; matching is queued and now takes newly added titles first. Absolute cost also fell ~15–30% (statement cache). Remaining per-scan cost that grows with the library is by design: a scheduled scan lists every file (~0.6–1.2 ms per unchanged file); reported paths use the targeted scan.

## Framing (owner guidance, 14 Oct)

- Adding a very large amount of media is expected to take time; absolute speed is secondary. What matters most is that the **time per item stays roughly constant as the library grows**, so a 5-million-item library and a 50,000-item library both feel reasonable. Anything whose per-item cost (adding, changing, removing, ordinary personal-state writes) rises with library size comes first, measured as a rate at several sizes.
- At large scale third-party metadata providers' rate limits are the bottleneck. The scan must never wait on providers; scanned titles must appear in browse promptly from their files and names; metadata arriving later in a trickle must be cheap per item; the matching queue must work through a large backlog in a sensible order without starving the scan.
- Absolute speed-ups that keep the curve flat are welcome but secondary.

## How it was measured

- `scalebench` gained `-interleave` (films, episodes and songs arrive scattered through the seed, as a growing library's would), `-curve-window N` (write and derivation ms per item for every window of N items, writer row changes per item, and with `-curve-shapes` the statement shapes with their mean cost per call, so a shape whose mean grows with the library is visible), `-seed-stop`, `-cpuprofile`, and `-reuse -ingest [-ingest-removal]` (on an existing fixture: 400 films with 1–3 versions, 200 episodes into the largest show, 200 songs into the largest artist's album, 300 metadata trickle writes (title, overview, genres, eight credits), 400 progress ticks, marking and undoing a 500-episode show, and Movies library removal in 20-second windows; each with ms per unit, max write-gate hold, writer row changes per unit, statement-cache reuse and the costliest statement shapes). Derivation in the scenarios uses the server's own batch size (8 units per transaction).
- `ingestion.TestScanAddRateStaysFlatAsTheLibraryGrows` (performance tier) drives the **real scanner** over a growing TV folder tree in rounds (names only, no probing), draining the derived worker after every scan quantum, and reports scan and derivation ms per new file and per listed file per round.
- Machine: ehler-personal (i5-10500T, 6c/12t, 30 GiB), shared with two other perf lanes and other work (load average 8–10 during runs), so single numbers carry noise of roughly ±15%; compare shapes, not single values.

## Findings (review)

Severity: **G** = cost grows with library or backlog size (first priority); **A** = absolute cost, flat curve (secondary).

### F1 (G, high) — every scan re-derives the whole library; an unchanged rescan too
Evidence: an unchanged rescan of 40 episodes queued 40 availability units plus one library-wide availability fan-out (test `TestUnchangedRescanQueuesNoDerivedWork` fails on the base). In the scan benchmark the derived units per 500-file round grew 7,514 → 10,616 → 13,738 (≈6 extra units per *existing* file per scan).
Why: (a) the inventory upsert rewrites `state`, `root_incarnation`, `unsupported_reason` on every observation, and `AFTER UPDATE OF` triggers fire whether or not the value changed — each queued availability for the file's titles and bumped the library revision; (b) every scan sets its source's health to `inventory_running` and back, and `catalog_availability_source_health` queued a library-wide fan-out (domain 28) on any health change, although availability only depends on the offline/root-changed/removing boundary; (c) `deriveAvailability` queued a browse-row re-derivation (the costliest derivation, ~1–1.5 ms) for every item it checked, changed or not.
Effect: adding one file to a 1M-item library started a scan whose derived work was ~2M availability checks plus ~2M browse-row rebuilds — hours of background work and write-gate time for one file. This is the main reason "time to add an item" grew with the library on the real scan path.
Expected gain: per-scan derived work proportional to what changed, not to the library.

### F2 (G, high) — metadata and artwork queue claims sort the whole backlog inside a write transaction
Evidence: `EXPLAIN QUERY PLAN` of the screen-metadata claim: covering-index range over every due row plus `USE TEMP B-TREE FOR ORDER BY` (the `ORDER BY CASE …` cannot use an index); artwork discovery claim: `SCAN artwork_discovery` + temp sort (no index at all); artwork job claim: multi-index OR + temp sort. Timed through the production driver inside a write transaction: 1.3 ms at a 2k backlog, 7.0 ms at 20k, 92 ms at 200k — linear; ~0.5 s per claim at 1M.
Why it matters: claims happen once per provider request and hold the single write gate, so a large backlog (exactly the situation after scanning a big library) stalls the scanner and every other writer for that long per request — quadratic total cost to work through a backlog.
Expected gain: constant claim cost (one indexed probe per state).

### F3 (G, medium) — scan directory alias check reads every directory of the run, per directory
`SELECT EXISTS(… inventory_directories WHERE job_id=? AND identity=? AND relative_path<>?)` cannot use the partial unique index `inventory_directories_visited (… WHERE identity!='')` because `identity=?` does not prove `identity!=''`; the planner falls back to a `job_id` prefix range. It runs once per directory page and once per child directory, so a scan costs O(directories²) (a 100k-album music library: ~10¹⁰ index steps).

### F4 (G, medium) — removing a file reads the whole analysis queue
`inventory_analysis_queue(object_id)` had no index: every `DELETE … WHERE object_id=?` (missing-title removal, library removal) and the foreign-key check on each `inventory_objects` delete scanned the queue, which keeps a row per file per retained scan job.

### F5 (G, low) — moved-file check can read every file of the source
The relocation query starts from `inventory_objects_directory (source_id, root_incarnation)` without planner statistics and its count subquery from `inventory_objects_path (source_id)`; both now start from the asset.

### F6 (A, medium) — the statement cache was too small for the write path
128 entries; the scan page, the derived worker's domains and the trigger programs compiled into their statements use a few hundred distinct statements, so ~6% of executions (≈16 per film added) re-parsed and re-compiled a statement, often one with dozens of triggers. CPU profile of the scan benchmark: SQL parsing (`sqlite3RunParser`) 26% of all CPU. With 512 entries misses fell 6,492 → 195 per 400 films, inside the unchanged byte budget (evictions ≈ 0).

(more findings below as measured)

### F7 (A/metadata, medium) — every new episode sends its show back to pending
`screen_episode_create` and `screen_wake_item_assets_INSERT` reset the show's work row to `pending` (and clear its lease) for each episode or file added. While a scan adds a show's episodes page by page, a show that the matcher has already claimed or matched is matched again after every page: extra provider requests and a lost in-flight result, at exactly the time provider rate limits are the bottleneck. Not changed here (the reset carries the show's episode dispatch, owned by the metadata code); mitigated by taking shows oldest-first (below). Recommended follow-up for the metadata owners: debounce the reset (push the show's `next_attempt` a short settle time ahead instead of clearing it), with a test clock that agrees with SQLite's.

### Confirmed not to grow (review + measurement)
- The catalogue write API plus derivation per added item (films, episodes, songs with six credits and three genres): 28.2 / 29.2 / 25.7 / 29.9 ms per item at 20k / 40k / 60k / 80k (interleaved seed, base code; table below). No statement shape's mean cost grows with the library.
- The scan never calls a metadata provider: ingestion and the catalogue write path contain no network calls; matching is queued by triggers (`screen_metadata_work`, `mb_jobs`, artwork queues) and run by the separate metadata worker. Local metadata (NFO, embedded tags) is read from the files during analysis. What the scanner did share with the matcher is the write gate, which F2 made the matcher hold for a time proportional to the backlog.
- Progress ticks, history and play-count writes are primary-key work; a tick costs ~2–3 ms, independent of library size (scenario numbers below).
- Marking a show watched / undo is one bounded statement set (24 statements) whose cost follows the show's size (bounded at 5,000 members), not the library's.

### F8 (G, high) — scan history pruning read the whole subtitle listing every minute
`PruneSource` runs every minute for every source in a write transaction. Its working-row delete for `subtitle_inventory_entries` joined on `job_id`, but the table's key starts with `source_id` and it had no `job_id` index, so the plan was `SCAN w` **once per retained completed run** (up to 20), and the foreign-key check when an old run's job row was deleted scanned it again. The table holds one row per listed file per scan, so during a scan of a large library each idle prune pass held the write gate for a time proportional to the library: 1.0 ms → 121.6 ms from a 2k to a 200k listing with only three retained runs (≈0.8 s with twenty; seconds at 1M files).

### F9 (G, low) — minute cleanup read three personal tables whole
`CleanupPersonalReceipts` (every minute, write transaction) deletes rows older than 30 days from `personal_explicit_events`, `personal_activity_receipts` and `saved_resource_receipts`, none of which had a `created_at` index: a full read of 30 days of explicit personal actions every minute. Grows with usage rather than library size; fixed with three indexes because it is cheap.

### Checked, flat in absolute terms (no change)
- An unchanged rescan costs ~0.63 ms per listed file (1,000-file profile: inventory upsert 82 µs, subtitle listing upsert 33 µs, guarded asset update 26 µs, a few primary-key reads); 1M files ≈ 10 minutes of bounded quanta. Listing every file on each scheduled pass is a documented design choice (a timestamp does not prove a file unchanged); reported paths use the targeted scan.
- Write amplification per added title is high but flat: ~210–260 row changes per film/episode/song through the write API (including derivation and triggers), ~600 for the reference item with six credits and three genres, ~700 for a metadata trickle write with eight credits and two genres (the search document's prefix rows are deleted and re-inserted whole). Not changed: it does not grow with the library.

### F10 (A/benchmark) — the watched-action benchmark measured its own access check
The scale benchmark's synthetic owner grant for "mark a 500-episode show watched" was `item IN (SELECT id FROM catalog_entities WHERE library_id=<tv>)`, a list of every entity of the TV library, so the action's measured cost followed the library's size (118 ms at 20k, 171 ms at 98k). The server's own check (`contentaccess.VisibleItemsSQL`) is per item. The benchmark now uses a per-item check; earlier capacity reports' watched/undo figures include this artifact.

### F11 (G, medium) — MusicBrainz claim walked the whole backlog during every cooldown
The claim condition includes "provider not cooling down", which every pending job fails during a pause, so each MusicBrainz step read the entire pending set (one job per song and album) before concluding there was nothing to do — on every metadata wake, for the whole length of each rate-limit pause. A read (outside the gate), but CPU proportional to the music library, exactly while the provider is the bottleneck. Remaining (not changed, recorded by the earlier capacity lane): a music library whose MusicBrainz policy is turned off leaves its jobs pending, and each claim walks past them.

## Fixes (commits on `audit-implementation/perf-ingest`)

| Commit | Finding | Change |
|---|---|---|
| Claim metadata and artwork work in constant time … / Match shows oldest first … | F2 | Screen-metadata, artwork-discovery and artwork-job claims are unions of one-row indexed probes per state (and per kind/order), merged by a final sort over at most nine probe rows. New/changed indexes: `screen_metadata_due(status,target_kind,next_attempt,target_id,lease_until)`, `artwork_discovery_due`, `artwork_jobs_claim` (expression on main-artwork role), `artwork_jobs_storage_retry` (partial). Order: owner searches → shows dispatching episodes → shows (oldest first) → titles (**newest first**); within a class never-tried before retries. |
| Keep scan directory and file-move checks on their indexes | F3, F4, F5 | `identity<>''` + `INDEXED BY inventory_directories_visited`; relocation query driven by `inventory_objects_asset`; `inventory_analysis_object(object_id)` index. |
| Stop unchanged rescans from re-deriving the whole library | F1 | `WHEN` clauses on `inventory_availability_object`, `catalog_availability_object_update` (only real changes) and `catalog_availability_source_health` (health only when it crosses the offline/root-changed/removing boundary; enabled/incarnation/generation as before). `deriveAvailability` collects the items whose stored availability changed (`RETURNING`) and re-derives only their browse rows. Derived content is unchanged, so no derivation version bump. |
| Keep the catalogue write path's statements prepared | F6 | Statement cache entries 128 → 512 (byte budget unchanged). |
| Find a scan's subtitle listing rows by job when pruning history | F8 | `subtitle_inventory_entries_job(job_id)`. |
| Index the expiry of personal events and receipts cleaned every minute | F9 | `created_at` indexes on three tables. |
| Do not search the MusicBrainz backlog while the provider asks us to wait | F11 | Cooldown read first; no claim search while it is in force (exactly the old condition). |
| Benchmark commits | — | `scalebench` curve/interleave/ingest scenarios; `TestScanAddRateStaysFlatAsTheLibraryGrows`; per-item access in the watched benchmark (F10). |

Tests added (each fails on the base code where noted): `TestUnchangedRescanQueuesNoDerivedWork` (fails on base: 40 availability units + 1 library fan-out), `TestQueueClaimsReadOneIndexedRowPerState` (plan guard for all four claims), `TestScreenClaimTakesTheNewestTitleFirst`, `TestScreenClaimCostIsFlatAcrossBacklogSizes` (performance tier; base 1.3 → 7.0 → 92 ms, now 0.42 → 0.42 → 0.43 ms at 2k/20k/200k), `TestScanHistoryPruningFindsSubtitleListingRowsByJob` (fails on base), `TestScanHistoryPruningCostIsFlat` (performance tier; base 1.0 → 121.6 ms, now 1.0 → 1.4 ms), `TestMBClaimSkipsTheBacklogDuringACooldown`, `TestScanAddRateStaysFlatAsTheLibraryGrows` (performance tier, real scanner).

## Before and after measurements

All rates are milliseconds per unit; "before" is the base commit `11a93f65`, "after" this branch. The machine was shared and loaded (load average 8–10), so differences under ~15% are noise; the shape (does it rise with size?) is the result.

### 1. The real scanner adding files to a growing TV library (`TestScanAddRateStaysFlatAsTheLibraryGrows`, rounds of 2,000 new episode files, names only)

| Library after round | Before: derived units / derive ms per new file / total ms per new file | After: derived units / derive ms / total ms |
|---|---|---|
| 2,000 | 29,944 / 13.8 / 20.0 | 25,689 / 8.8 / 14.2 |
| 4,000 | 42,415 / 17.5 / 25.7 | 25,651 / 8.7 / 16.5 |
| 6,000 | 54,916 / 22.0 / 31.5 | 25,650 / 8.8 / 16.6 |
| 8,000 | 67,415 / 24.9 / 35.5 | 25,651 / 8.9 / 18.2 |
| 10,000 | 79,915 / 30.0 / 42.8 | 25,651 / 9.3 / 19.4 |
| 12,000 | 92,414 / 33.0 / 46.8 | 25,650 / 9.8 / 21.2 |
| **One more file at 12,000** | **75,041 units, 25,508 of them before it showed in browse, 74.8 s** | **34 units, visible after 6, 13.0 s** |

Shape: **before, the cost of adding a file rose linearly with the library** (derivation per new file 13.8 → 33.0 ms over 2k → 12k; each scan re-derived every existing title, and the new title waited behind them). **After, derivation per new file is flat** (8.8 → 9.8 ms; identical derived work per round). The remaining rise in "total" is the scanner listing every existing file on each pass (≈0.6–1.2 ms per existing file, by design); with a path report the targeted scan avoids it. The 13 s for one more file is that listing of 12,000 files; the derived work is 34 units.

### 2. Queue claims (production driver, inside a write transaction)

| Backlog | Screen-metadata claim before | after |
|---|---|---|
| 2,000 | 1.3 ms | 0.42 ms |
| 20,000 | 7.0 ms | 0.42 ms |
| 200,000 | 92.3 ms | 0.43 ms |

Before: linear (≈0.5 s per claim at 1M, held once per provider request under the write gate). After: constant. Artwork discovery/job claims: plan guard (no table scan, no backlog sort); the old artwork-discovery plan was a full table scan plus sort.

### 3. Scan history pruning (every minute per source, write transaction)

| Subtitle listing rows (scan in progress), 3 retained runs | Before | After |
|---|---|---|
| 2,000 | 1.0 ms | 1.0 ms |
| 200,000 | 121.6 ms | 1.4 ms |

### 4. Catalogue write API + derivation per added item (interleaved reference seed: films, episodes, songs with six credits, three genres, ratings; derivation drained after every 32 items)

| Library size (window end) | Before: ms/item (row changes/item) | After: ms/item (row changes/item) |
|---|---|---|
| 20,000 | 28.2 (606.2) | 24.0 (606.2) |
| 40,000 | 29.2 (596.0) | 24.4 (596.0) |
| 60,000 | 25.7 (579.0) | 20.7 (579.0) |
| 80,000 | 29.9 (595.8) | 23.3 (595.8) |
| 100,000 | — | 25.8 (598.4) |
| 120,000 | — | 22.3 (534.5) |
| 140,000 | — | 22.9 (550.4) |
| 160,000 | — | 29.6 (626.2) |
| 180,000 | — | 19.2 (536.5) |
| 200,000 | — | 18.3 (532.6) |

Shape: **flat** before and after (no statement shape's mean cost rises with size; per-shape table in `after-200k.log`). Row changes per item are identical before and after, so the derived content is the same. The before run stopped at ~98k when the benchmark's own scratch-size check (`du`) failed because test files were being deleted beside it (fixed by moving benchmark directories); its 98k database was used for the before scenarios below. After is ~15–20% faster at equal size, mostly the statement cache (F6). Writer-side cost is ~6–7 ms/item; derivation ~12–18 ms/item at the server's batch size.

### 5. Adding, changing and removing on an existing library (`scalebench -ingest`, server batch size)

| Scenario (ms per unit) | Before 5k | Before 20k | Before 98k | After 20k | After 200k |
|---|---|---|---|---|---|
| Add film with 1–3 versions (write + derive) | 24.5 | 22.2 | 17.9 | 19.3 | 19.2 |
| Add episode to the largest show | 21.5 | 21.7 | 21.5 | 17.5 | 18.2 |
| Add song to the largest artist's album | 25.1 | 29.8 | 25.0 | 20.5 | 21.1 |
| Metadata trickle (title, overview, 2 genres, 8 credits) | 22.1 | 28.2 | 27.6 | 22.9 | 22.4 |
| Progress tick (incl. derived work) | 3.1 | 3.6 | 3.0 | 3.4 | 2.5 |
| Mark 500 episodes watched / undo (ms per action) | 144 / 107* | 118 / 98* | 171 / 171* | 123 / 117 | 120 / 88 |
| Remove Movies library, per entity | 10.2 | 11.8 | 12.6 | 11.3 | 11.7 |
| … of which `DELETE FROM catalog_entities` mean | 3.9 | 4.2 | 5.4 | — | 3.95 |
| Removal max write-gate hold | 207 | 162 | 303 | 163 | 498 |

\* before figures used the benchmark's library-wide access list (F10), not the server's per-item check.

Shape: **every per-item rate is flat from 5k to 200k.** Removal per entity is flat (the entity delete's mean moves 3.9 → 5.4 ms over 5k → 98k before and is 3.95 ms at 200k after: B-tree depth, not a scan). One removal transaction held the gate for 498 ms at 200k (single outlier during the final clean-up phase on a loaded machine; 4 titles per transaction elsewhere); not investigated further.

## Decisions I made

- **Metadata claim order.** Owner searches first, then shows whose episodes are being dispatched, then shows oldest-first, then titles (films, episodes) **newest-first**; within a class, never-tried before due retries. Newest-first for titles puts what was just added (and is on Recently Added) ahead of an old backlog. Shows stay oldest-first because every new episode resets its show to pending (F7): newest-first would rematch the show a scan is still filling, spending provider requests. "What the person is looking at first" is not implemented: it would need a write on a read path (a details view bumping a priority), which I judged out of scope for a performance pass; recorded under Needed from others.
- **Artwork job claim.** Pending jobs keep the exact old order (main artwork first, then oldest). A due retry is now taken in the order its retry time came round rather than by creation time when competing with other retries; it still competes with pending jobs by (main artwork, created). This keeps every probe one indexed row.
- **Health flips do not re-derive availability.** A source's health only matters to availability when it crosses offline / root changed / removing; other health changes (`inventory_running`, scan finished, warnings) no longer fan out across the library. The library revision bump and admin revision bump on any health change are kept (cheap, and screens may show scan state).
- **Statement cache 512 entries** inside the existing byte budget (4 MiB at most per connection on this host), rather than raising the memory budget: the entry count, not bytes, was the binding limit.
- **Scope.** I did not change derivation content, the scan's design of listing every file on each pass, the single write gate, checkpoint sizes, or crash-safety. No derivation version bumps were needed (derived content is identical; the seed's row changes per item are identical before and after: 606.2 at 20k).

## Bugs

No `B-…` IDs are assigned to this lane. New defects found and fixed are F1–F5, F8, F9, F11 above. Two observations outside my area, not fixed:
- `inventory_item_availability` (baseline view), fallback branch for a file no inventory record knows: `NOT EXISTS(SELECT 1 FROM library_sources s WHERE s.id=cl.library_id AND …)` matches only the library's first folder (whose source id equals the library id); an added folder being offline is not seen there. Needs the availability owner to confirm whether that branch should consider every source of the library.
- F7 (show reset on every new episode).

## API changes

None.

## Schema changes

Edited `0001_baseline.sql` and ran `scripts/schema-sync.py`:
- Indexes added: `artwork_discovery_due`, `artwork_jobs_claim` (expression), `artwork_jobs_storage_retry` (partial), `inventory_analysis_object`, `subtitle_inventory_entries_job`, `personal_activity_receipts_expiry`, `saved_resource_receipts_expiry`, `personal_explicit_events_expiry`.
- Index changed: `screen_metadata_due` → `(status,target_kind,next_attempt,target_id,lease_until)`.
- Triggers changed (WHEN clauses only): `inventory_availability_object`, `catalog_availability_object_update`, `catalog_availability_source_health`.
No derivation versions changed.

## Owner or viewer settings added or wired

None.

## Needed from others

- **Metadata owners:** F7 — debounce the show reset on new episodes (push `next_attempt` a settle time ahead instead of clearing it and the lease), with a test clock that agrees with SQLite's `now`. Optionally a "being viewed" priority for matching (a details view nudging the title's claim order) if product wants it.
- **Availability owner:** the `s.id=cl.library_id` comparison in `inventory_item_availability` (see Bugs).
- **Architecture docs:** the matching order (titles newest-first, shows oldest-first) and the rule that scans and health flips do not re-derive unchanged titles.

## Test notes for other environments

Host ehler-personal, Ubuntu amd64, Go with `GOFLAGS=-p=4`; `GOOS=windows` and `GOOS=darwin` builds pass. Packages tested: metadata, ingestion, compactcatalog, dbwork, catalog, catalog/scalebench, persistence, subtitles (all pass). Performance-tier proofs (set `PORTICO_PERFORMANCE_TIER=release`, use a disposable `TMPDIR` with a few GB free):
```
go test ./internal/ingestion -run 'TestScanAddRateStaysFlatAsTheLibraryGrows|TestScanHistoryPruningCostIsFlat' -v   # PORTICO_SCAN_ROUNDS / PORTICO_SCAN_ROUND_FILES size the first
go test ./internal/metadata -run 'TestScreenClaimCostIsFlatAcrossBacklogSizes' -v
```
Pass: the scan rounds report constant `derived_units` per round and `SCAN_ONE_MORE` derived units in the tens (not proportional to the library); claim times flat across 2k/20k/200k; the prune pass flat across 2k/200k listing rows. To repeat the seed curve and ingest scenarios: `go build -o /scratch/sb ./internal/catalog/scalebench`, then from a scratch directory `sb -dir X/.scratch/c -items 200000 -interleave -curve-window 20000 -curve-shapes 25 -seed-stop 200000`, and `sb -dir X/.scratch/c -items 200000 -reuse -ingest -ingest-removal` (removes the Movies library at the end; keep the benchmark directory away from anything else being written, because the scratch-budget check runs `du` over the parent of `.scratch` and fails if files vanish under it).
On macOS/Windows: run the same tiers; only relative shapes matter. A real-device check worth doing once: add one file to a large library and confirm it appears in browse within seconds of its scan, with the derived queue (diagnostics) not jumping by the library's size.

## What remains and why I stopped

- **Stopped because** every per-item rate measured (scan, catalogue writes, metadata trickle, progress ticks, watched actions, removal) is now flat from 5k/20k to 200k, and the growth found in background work beside the scan is fixed. What is left is absolute cost, which the owner ranked secondary.
- **Absolute cost, not pursued:** derivation dominates (≈12–18 ms per item at the server's 8-unit batches; browse-row derivation ≈24% of CPU, `refreshBrowseSortFacts` ≈12% and is called several times per item per batch — a per-transaction de-duplication might save 5–10%). Write amplification ~210–260 rows per film/episode/song, ~700 per metadata write (whole search prefix rows rewritten). The scanner's per-quantum overhead is ~0.6–1.2 ms per unchanged listed file.
- **During a large first scan** the scanner (~5 ms/file) outpaces derivation (~9 ms/file), so browse fills in scan order with a lag equal to the derivation backlog; it cannot be removed without making derivation itself cheaper.
- **Not measured:** one million items (the brief forbids completing it; the 200k curve is flat with no growing shape), a real filesystem scan above 12k files, the metadata worker running against stubbed slow providers end to end (the claim cost and the scan's independence from providers are proven separately), Windows/macOS timings.
- **F7** (show rematched after each scan page) and the availability-view source check are for other owners (above).
- One 498 ms removal transaction at 200k (outlier) not investigated.

Evidence logs were in `_program/scratch/perf-ingest/logs/` and are summarised above; the scratch folder (databases, Go cache, binaries) has been deleted as the brief requires.
