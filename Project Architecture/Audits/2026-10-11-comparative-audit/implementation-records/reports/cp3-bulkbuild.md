# cp3-bulkbuild — building a library from scratch quickly

Worktrees: `server-cp3-bulkbuild` and `native-cp3-bulkbuild`, branch `audit-implementation/cp3-bulkbuild` (not pushed). `audit-implementation/main` merged into both twice (last at 11:30 on 16 Oct, merge `e134b6a0` in the server branch); schema-sync and apigen produce no changes after it. The native branch has no changes of its own (fast-forward to main; shared packages identical to the server branch, nothing to mirror).

## 1. Outcome

**50,000 files from empty to every title browsable, searchable and fully derived: 45.9 min → 20.7 min wall, 69 → 24 processor-minutes** (half films in a folder each, half episodes; real scanner and storage helper, derived worker beside it, measured side by side on a loaded shared host). **200,000 files: 67.8 min after the change (20.3 ms a file, the same per file as at 50k, so the curve is flat); before the change it did not finish — after 65 minutes it had catalogued 9,800 of 215,000 titles and was slowing, because each new folder read every folder registered before it** (§1 item 1). Titles now appear as the scan reaches them. A full rebuild of 200k titles' derived data from facts: 58.9 → 52.6 min, processor time −15%.

Building a library from empty is now several times faster and its titles appear as the scan finds them instead of at the end. Four things made it slow; all four are fixed:

1. **A hidden quadratic in every bounded table.** Each insert into one of the 60 tables with a storage limit ran an existence test written as one `OR` of two key matches. The SQLite the server now ships (3.53) plans that as a scan of the whole table. A scan records every folder it sees in such a table (`inventory_directory_cache`), so each new folder read every folder before it: 0.9 ms per folder at 2,000 folders, 4.4 ms at 6,000, about 22 ms at 50,000. The check now reads the table's counter first (the existence test only runs at the limit) and each key match is its own index seek: a flat ~35 µs. This was the largest cost at 50k and 200k folders and is invisible at small sizes.
2. **One process per folder.** On Linux every folder listing and every folder check started the server's own binary as a helper (about 20 ms of processor time each, two per folder). A library with a folder per film spent most of its scan starting processes. One helper now lists or checks up to 32 folders of the same root, under the same timeout, with a per-folder result.
3. **One transaction per folder.** The scanner now publishes a batch of small folders in one transaction of at most one page of entries (the same bound a single folder page had), with each folder in its own savepoint so a replayed page still leaves nothing behind. Anything the batch can't take (errors, unstable folders, remote sources) goes through the old one-folder path.
4. **Derived work redone as members arrive.** Browse rows and search for titles now come first; memberships and availability next; the fanouts that summarise members into shows, seasons, albums and artists after their inputs settle. A container whose members changed is queued once (new derived domain 35) instead of being re-derived after every batch of its members — a big artist was re-derived hundreds of times while its songs arrived. During a full rebuild the album/artist sort-key fanout no longer walks every member a second time. Measured side by side on a full rebuild of 200k titles from facts, processor time fell 15% and wall time 11%; in a 50k rebuild (not side by side, so load differed) sort-fact refreshes fell 262k → 151k and row upserts 121k → 57k. The bigger effect of the new order is in a scan, where titles become browsable first.

Progressive visibility: the scanner now outruns derivation, and the derived worker yields the write gate to it, so without a bound titles became browsable only after the whole scan. A scan step now waits (up to one second) while more than 2,000 titles wait for their browse row or search document, so titles show up within seconds of being found. Metadata matching is untouched: the scan never calls a provider and matching runs from its queue under each provider's rate limit afterwards, as before.

Same data either way: a new equivalence test builds the same mixed library (films, a show's episodes, an album artist's songs with credits and genres) title by title with a drain after each, and all at once, and compares 42 derived tables (ids that are random per database masked; counted-block boundaries compared by their totals). It catches a skipped container refresh.

## 2. Decisions I made

- **Bulk path shape.** I did not write a second, set-based implementation of every derived domain. Profiles showed the cost spread over ~30 statements per title, dominated by index and trigger maintenance that a set-based insert pays all the same (a browse row has 22 indexes and 25 triggers). The wins were structural: removing a quadratic, process spawns and per-folder commits, ordering derivation by dependency and collapsing repeated container work. "Deferred index builds" were not done: the derived tables are shared by every library and are read while being written, so dropping and rebuilding indexes is not safe.
- **Container refresh waits** while other derived work is queued, or at most 10 seconds, and then takes a normal batch. During a long metadata trickle that never lets the queue empty, containers are still refreshed about once every 10 seconds a batch at a time (≥ ~20k containers an hour); their rows' summary facts (longest file, newest member, availability state) lag by at most that.
- **Domain priorities**: browse rows and search get their turn up to 1 s ahead of the plain round robin, memberships and availability 0.5 s; every domain with work still gets a turn about once a second.
- **Title backlog bound** 2,000 titles, wait up to 1 s per scan step. A stalled derived worker therefore slows a scan to one step a second rather than stopping it.
- **Folder batches** ≤ 32 folders and ≤ one page of entries (64 normally, 128 on the fast background setting) per transaction; folder checks 32 per step (was 8).
- **Storage limit checks** rewritten in the baseline schema (pre-release; development databases must be recreated). Behaviour is identical: `NOT (A OR B)` became `NOT A AND NOT B`, and the counter comparison moved first.
- Not done, deliberately: the statement cache re-measures each statement's memory after every use (`sqlite3_stmt_status`, about 5% of a build's processor time). Measuring less often would loosen a memory bound another lane chose to keep exact; left as is and noted.

## 3. Audit items closed

Justin's 15 Oct answer "Building a library from scratch must be quick. If it isn't, fix it now" (remaining-sweep item 13, "a bulk empty-build rebuild path"). No bug IDs assigned. Unnumbered defects fixed: the quadratic storage-limit checks (all 60 bounded tables), two helper processes per scanned folder.

## 4. API changes

None.

## 5. Schema changes

`0001_baseline.sql`: the 60 `*_bound_check` triggers test the table's counter first and look the row up by key with one index seek per match. `scripts/schema-sync.py` run (persistence tests pass). New derived domain number 35 (`DomainContainerRefresh`) is a code constant; no table changes.

## 6. Settings added

None.

## 7. Tests run

### Measurements

Host: ehler-personal (12 threads, shared with the other lanes; load average 4–30 during the runs). Before = branch base `6ae3ccf4` plus the measurement tooling; after = this branch. Each pair ran at the same time on the same host, so they saw the same load; absolute numbers on an idle server will be lower. Processor time includes the storage helper processes.

`TestBuildLibraryFromEmpty` (`internal/ingestion`, performance tier; `PORTICO_PERFORMANCE_TIER=release PORTICO_BUILD_FILES=N`): an empty server, a film library (one folder per film) and a TV library (shows of 20 episodes in two season folders), half the files each; the real scan loop with the real storage helper, the derived worker beside it paced as in the server, the maintenance loop's log copying running. Times are from the scan's start to the moment named.

| Files | | First title browsable | Half browsable | All browsable | All searchable | Everything derived | Processor (own + helpers) |
|---|---|---|---|---|---|---|---|
| 2,000 | before | 1 s | 10 s | 36 s | 36 s | 53.9 s | 35.1 + 48.5 s |
| 2,000 | after¹ | 2 s | 18 s | 21 s | 17 s | 23.9 s | 26.5 + 4.1 s |
| 50,000 | before | 6 s | 10.9 min | 37.7 min | 37.7 min | **45.9 min** | 38.0 + 31.4 min |
| 50,000 | after | 5 s | 10.0 min | 17.5 min | 17.5 min | **20.7 min** | 20.3 + 3.5 min |
| 200,000 | before | — | — | — | — | **not finished**: 9,798 of 215,000 titles after 65 min, rate falling | — |
| 200,000 | after | 17 s | 28.8 min | 56.8 min | 56.6 min | **67.8 min** | 70.6 + 13.3 min |

¹ The 2,000-file pair ran before the storage-limit fix and the one-page cap on folder batches; both matter little at that size.

Per file after the change: 12 ms (2k, light load), 25 ms (50k, load ~17), 20 ms (200k, load ~5–8): flat with size. The before run at 200k spent its time registering the film folders (92,000 of 100,000 after 65 minutes, each insert scanning the table).

Full rebuild of derived data from facts (`scalebench -bulk-facts -narrow-facts -facts-only`, then `-reuse -derive-only -no-probes`, STAT4 cleared; the scenario of a derivation version bump), 200k titles, side by side: before 58:54 wall, 2,836 processor-s; after 52:34 wall, 2,423 processor-s (−11% / −15%). At 50k (not side by side): before 27:18 / 895 s, after 11:15 / 535 s, with sort-fact refreshes 262k → 151k and browse-row upserts 121k → 57k.

Storage-limit check, 2,000 inserts into `inventory_directory_cache` per round: before 885 → 2,770 → 4,440 µs per insert at 2k/4k/6k rows; after 39 → 31 → 32 µs.

`TestScanAddRateStaysFlatAsTheLibraryGrows` (after, 8 rounds of 3,000): derivation 10.3–15.2 ms per new file with no upward trend; scan per listed file falling 9.5 → 2.2 ms; one more file at 24,000 costs 37 derived units and shows after 7.

### Tests

- `go test ./internal/compactcatalog ./internal/ingestion ./internal/catalog ./internal/catalog/scalebench ./internal/storage ./internal/dbwork ./internal/persistence` — pass before and after the second merge (after it, the equivalence test needed its search-prefix comparison updated for main's new prefix table, commit below; then pass).
- `go test ./internal/metadata` — pass (after the first merge).
- New: `TestBulkBuildDerivesWhatIncrementalBuildDoes` (scalebench; 42 derived tables equal between a title-by-title and an all-at-once build; fails if container refreshes are skipped), `TestStorageLimitChecksReadTheCounterFirstAndSeekTheirKeys` (persistence; all 60 checks), `TestInventoryBatchListsAndChecksSeveralFoldersInOneHelper` (storage), `TestBuildLibraryFromEmpty` (ingestion, performance tier).
- Adjusted: `TestScanLoopGivesWayToForegroundWorkBetweenSteps` and `TestDurableContinuationAndCancellation` count steps rather than folders (a step now takes a batch of folders).
- `GOOS=windows` and `GOOS=darwin` builds of storage, ingestion and catalog pass. `scripts/schema-sync.py` and `go run ./cmd/apigen`: no changes. Shared packages: identical to the server (`shared-packages.mjs --check`). Web and native lint/type-checks not run: this lane changes no web, native or shared-package code, and the worktrees have no `node_modules`.

## 8. Device test notes

None on devices. On a real server (Linux), a first scan of a large library: watch that titles appear in browse within seconds of the scan reaching them, the owner's scan progress moves steadily, and the web UI stays responsive (scan transactions are bounded to one page of entries as before). On Windows and macOS the folder listing still uses the retained-stream helper one folder at a time (batched commits and checks still apply); worth timing a 10k-folder scan there.

## 9. Not finished

- **200k before run not completed** (it would have taken many hours); its partial progress is the evidence.
- Measurements ran on a loaded shared host; idle-machine numbers will be lower for both sides. The film/TV mix is synthetic (names only, no media analysis); real libraries add ffprobe analysis per file, which this lane did not change.
- No separate set-based derivation path (see §2). The remaining cost per title is index and trigger maintenance spread over ~30 statements (a browse row: 22 indexes, 25 triggers); further large gains need fewer indexes/triggers per row, which is a schema design question.
- On Windows and macOS folder listings still start one retained-stream helper per folder; batched checks and commits apply there. Batching those listings needs the stream cursor format handled across folders.
- The statement cache's per-use memory measurement (~5% of a build's processor time) left as is (§2).
- Not checked: eleven non-limit triggers also test `(…) OR (…)` (MusicBrainz sealing, TVDB pages, episode boundaries); none is on the library-build path, but they may scan under the newer SQLite planner too.
- Observation, not mine: `audit-implementation/main` now contains a 24 MB binary at `server/scalebench` (a built scale tool, executable bit set), committed by another lane; it should probably be removed and ignored.

### Commits (server, `audit-implementation/cp3-bulkbuild`)

- `90dfd33f` Measure building a library from empty with the real scanner, and derived cost per domain
- `2412c8f1` Read what a statement writes once per statement text instead of on every execution
- `fa604bf2` Build a library's titles first and refresh each show, album and artist once its members settle
- `61a70a47` List or check several folders of a library in one storage helper
- `2c23b9f8` Scan small folders a batch at a time and skip a redundant fanout in a full rebuild
- `f87bfa73` Let derived work keep up with a scan so titles appear as they are found
- `21dddd87` Count the storage helpers' processor time in the library build measurement
- `aab50f43` Keep a batch of scanned folders within one page of entries and keep the log copied during the build measurement
- `22e24e55` Check a bounded table's storage limit before looking up the row, and look it up by its key
- merges `57fb599f`, `e134b6a0` of `audit-implementation/main`
- `d85eb01c` Compare search prefixes by their text now that a prefix is stored once (equivalence test follows main's schema)

Native (`audit-implementation/cp3-bulkbuild`): no commits of its own; fast-forwarded to main.

Scratch: scale databases, fixtures, binaries and the Go cache deleted; only logs remain in `_program/scratch/cp3-bulkbuild/` (~0.5 MB).
