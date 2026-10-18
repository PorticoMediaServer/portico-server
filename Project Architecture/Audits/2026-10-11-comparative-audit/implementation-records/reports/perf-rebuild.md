# perf-rebuild — derived-data rebuild throughput, write-gate holds and trickle cost

Worktree `server-perf-rebuild`, branch `audit-implementation/perf-rebuild` (not pushed). 17 commits on top of `11a93f65`.

## Outcome

- An empty rebuild costs about half the CPU per title it did (10k: 170 s → 88 s; 50k: 1,050 s → 511 s), and the per-title cost now stays flat from 200k to one million: queue drain in the first minutes was 198 → 693 units/s at 200k and 165 → 714 units/s at one million (before: 17% slower per unit at 1m than at 200k; after: no slowdown).
- The only causes found of per-title cost rising with size were per-group: a show's facets and every container's availability were recomputed by walking all members, once per member batch (quadratic per group) and once per single-member change. Both now come from counters kept per member change.
- One title's metadata arriving (the long provider trickle) costs 4–10 derived units and a few milliseconds whatever the size of its show, album or artist. Before, a show's details re-derived every episode (257 ms median, 1.4 s worst for a 250-episode show) and an artist's re-queued every song (8.1 s worst for a 5,004-song artist).
- Write-gate holds from derived work: p99 72–88 ms → 45–52 ms; the longest hold 2,053 ms (1m) / 782 ms (200k) → 680 / 307 ms. Long holds were commits running SQLite's automatic checkpoint inside the gate; page copying now happens outside it.
- Browse and search keep answering from the previous generation exactly as before; no reader, API or restriction logic changed.

## Framing (owner, 14 Oct)

A long rebuild of a very large library is expected. What matters most is that the cost per title stays roughly constant as the library grows (a 50,000-title and a 5-million-title library should both be decent), so work whose per-title cost rises with library or group size comes first: growing index maintenance, whole-table scans or counts per batch, per-group work that grows with the group, generation steps proportional to total size, cache thrash past a size threshold. Absolute speed-ups that keep the curve flat come second (bulk filling, fewer statements per title, larger batches the gate allows). Real-world point: provider metadata arrives over hours or days, so titles are re-derived in a long trickle; one title's new details must cost a small, constant amount (no re-deriving whole groups, libraries or search postings for one title's change).

## Findings

(severity, evidence, expected gain)

1. **High — re-deriving a whole container for one member's change (group-size superlinear, rebuild and trickle).**
   - *Show facets*: `deriveFacetCounts` for a show read the union of every episode's files, collections and attributes (`... FROM catalog_episodes ep ... WHERE ep.show_id=?1`, three scans). It ran once per batch of episodes during a rebuild (O(E²/8) per show) and once per single-episode change in steady state (O(E) per trickle update). At 10k titles: 2,974 show derivations for 39 shows (76 per show).
   - *Container availability*: `refreshBrowseSortFacts` computed a container's `availability_state` with `EXISTS(member JOIN asset_links available=0)`, which walks every member when none is unavailable (the normal case). It runs on every container refresh, and containers refresh once per member batch. The synthetic first artist has 5,004 songs at every size ≥10k: about 1,250–1,900 refreshes × a 5,004-member walk. A real "Various Artists" with tens of thousands of tracks or a 1,000-episode show makes this quadratic.
   - Expected gain: per-title cost independent of group size; trickle update O(1).
2. **High — one show/album/artist/collection detail re-derives every member (trickle).** `setFields`, `SetTermsTx`, `SetCreditsTx`, `SetAttributesTx` all called `touchEntity`, which queues search documents for every episode of a show, every song of an album or artist, every member of a collection, and season rows, on *any* change — an overview, a genre list, a credit list, a label. Members only carry the parent's title/sort title/language/year, its artist/author and where it lives. With metadata trickling in per title, a 1,000-episode show re-derived 1,000 search documents for each of its several metadata writes.
3. **High (constant factor, every size) — statement cache thrash.** The per-connection prepared statement cache held at most 128 entries; the derived worker cycles through about 225 distinct statements (2.7 MB, well inside the 4 MB byte budget). LRU over a cycle larger than the cache misses almost always: 155k misses for 10k titles (≈15 re-prepares per title), and statements that fire many triggers are expensive to compile. CPU profile at 10k: `sqlite3Prepare` 23% of all CPU (36.8 s of 158 s), of which cache misses 22.5 s.
4. **Benchmark fidelity — STAT4 statistics in fixtures only.** The scale tool runs a full `ANALYZE`, which writes `sqlite_stat4`; production writes only `sqlite_stat1` (`persistence/planner_stats.go`). With STAT4 present, every rebind of a parameter compared against a sampled index column expires the statement, so it is re-prepared on its next step: a further 14 s (9%) of re-prepares at 10k that production never pays. Earlier rebuild timings included this. Fixtures here have `sqlite_stat4` cleared after seeding; the tool itself still runs `ANALYZE` (left as is so earlier lanes' commands reproduce; clear `sqlite_stat4` as described in Test notes).
5. **Medium — sort facts refreshed twice per browse-row derivation.** `deriveBrowseRow` deferred `refreshBrowseSortFacts`, and the `refreshEntityMetrics` it calls defers it again: two 11-subquery UPDATEs per row (80,819 calls at 10k for ~10.7k entities).
6. **High — restriction-class build quadratic in the library.** `RebuildVisibilityClass` reads the library in 100-row chunks with `SELECT max(entity_id) FROM (SELECT entity_id FROM catalog_browse_rows WHERE library_id=? AND entity_id>? ORDER BY entity_id LIMIT 100)`. Planned through a `(library_id, …)` index, every chunk sorts the whole library (`EXPLAIN`: `SEARCH catalog_browse_rows USING COVERING INDEX catalog_browse_rating (library_id=?) | USE TEMP B-TREE FOR ORDER BY`): L/100 chunks × L rows. It plausibly explains the "music class still incomplete after 16 minutes" at one million in scale-check-3 (not re-measured at 1m). Expected gain: one pass over the rows.
7. **Medium — every title's browse row derived twice in a rebuild.** Availability derivation (domain 20) queued a browse-row derivation (domain 18) for every item of its batch, changed or not, and the browse row read availability as 0 until then. Domains run round-robin in number order, so 18 derives the row, 20 queues it again, 18 derives it again: `INSERT INTO catalog_browse_rows` ran 3.07 times per title at 50k. Expected gain ≈ one full row derivation per title (the row upsert alone is ≈ 0.45 ms with its 25 triggers). The same path also missed a correctness case: a file going missing that does not change a title's added date never reached its show's or album's `availability_state`.
8. **Medium — fixed 8-unit batches.** Containers (show, season, album, artist) are re-derived once per member batch, and each commit flushes a search segment, so small batches multiply container and per-commit work; unit costs vary from ~0.05 ms to many ms, so a fixed count both under-fills cheap batches and over-holds the gate on heavy ones. Step durations at 50k with 8 units: p50 3.4 ms, p95 34 ms, p99 74 ms.
9. **High (gate) — automatic checkpoint inside commits.** `wal_autocheckpoint=1000` on the writer makes every commit that pushes the log past 4 MiB copy pages into the database (with fsyncs) while holding the write gate. Batches whose transaction body took 17 ms held the gate 690 ms; this is the source of the 1–2 s holds measured at 50k–1m. The maintenance loop already copies pages from a read connection, but only every 30 s.
10. **Medium (trickle) — an artist's overview re-queued MusicBrainz matching for every song and album of the artist** (`mb_artist_side_update` fired on any column of `catalog_artists`; the matcher reads none of them). 65 ms of gate and 5,004 song jobs for one overview.
11. **Evidence on growth with library size (no further algorithmic cause found).** Statements per title are identical at 10k and 50k for every top statement, but every statement — down to a primary-key read like `SELECT public_id FROM catalog_entities WHERE id=?` (11.5 → 15.3 µs) — is 15–35% slower per call at 50k. That is page-cache/B-tree depth, not algorithmic: the writer connection has a 4 MiB page cache for a database of 60 MB (10k) to several GB (1m). Measured separately below: a 64 MiB writer cache gave no gain, so the remaining rise is B-tree depth and CPU caches.

Read-path items offered from the reads lane (filter keys, fact-sort counted blocks, ordered search keys): **not taken** — the owner then asked to skip the first two; the third needs reader changes in code the reads lane owns. `audit-implementation/main` was therefore not merged into this branch.

## Fixes

1. Benchmark tooling (`scalebench`): `-derive-only`, `-derive-for`, `-no-probes`, `-cpuprofile`, `-trickle N` (single-title updates by kind, drained after each, with slow-update statement profiles), `-bump`, `-classes`, `-writer-cache-mib`, per-batch hold percentiles and slow-batch attribution (`Worker.LastBatch`).
2. Statement cache entry bound 128 → 512 (the byte budget remains the memory bound). Commit "Keep the derived worker's statements prepared between titles".
3. Sort facts refreshed once per derivation. Commit "Refresh a browse row's sort facts once per derivation".
4. Members are re-derived only when facts they carry change (backbone fields, or `artist_id`/`author`/`library_id`/`show_id`/`album_id`/`book_id`); terms, credits and attributes of an entity queue only the entity. Commit "Re-derive a container's members only when facts they carry change".
5. Container availability from maintained counters (`catalog_browse_member_links`, kept by triggers on memberships and file links); show facets from maintained per-show value counts (`catalog_show_facet_episodes`, `catalog_show_facet_values`) moved by each episode's own difference. Domain 33 version 4 → 5. Commit "Keep container availability and show facets in counters instead of member scans".
6. Restriction-class build walks the library forward by row id (`NOT INDEXED` on the chunk and scan statements): one pass instead of a sort of the whole library per 100 rows. Commit "Read a library once when building a restriction class".
7. Hold-paced batches: the background worker sizes each domain's batch to hold the write gate for about 20 ms (commit included), between 2 and 64 units, from that domain's recent cost per unit; callers that pass a fixed limit (tests, tools, `Drain`) keep it. Commits "Size derived-data batches to a write-gate hold time instead of a fixed count", "Count the commit in a derived batch's write-gate hold".
8. Availability derivation queues a title's browse row (and its metrics fanout to containers) only when the published row's availability or availability state differs; a row not yet through availability derivation reads its links' flags. Commit "Derive a title's browse row again only when its availability changed, and carry the change to its containers".
9. WAL page copying moved out of the write gate: the maintenance schedule runs a PASSIVE checkpoint on a read connection every 2 s while commits arrive (was 30 s); the writer connection's automatic checkpoint is raised from 1,000 to 16,000 pages and is only a backstop. Commit "Copy committed pages into the database outside the write gate".
10. An artist's side-table details (overview, tags) no longer send every song and album of the artist back to MusicBrainz matching (`mb_artist_side_update` now fires only on an identity change). Found by the trickle measurement: one overview for the 5,004-song artist re-queued 5,004 song jobs and held the gate 65 ms. Commit "Send an artist's songs back to matching only when the artist's identity changes".

## Measurements

Method. `scalebench` (this lane added `-derive-only`, `-derive-for`, `-no-probes`, `-cpuprofile`, `-trickle N`, `-bump`, `-writer-cache-mib`, per-step hold percentiles and slow-step attribution). Fixtures: `-bulk-facts -narrow-facts -facts-only` (the shape of scale-check-3), then `sqlite_stat4` cleared to match production statistics. One large run at a time from this lane; the host was shared with the other perf lanes (load average 7–12 throughout), so wall times carry noise of perhaps ±15%; CPU seconds are given where they matter. "Units" are derived-queue units (about 3.3 per title across domains). No concurrent read probes in these runs (`-no-probes`); readers keep the previous generation exactly as before (no change to serving).

### Empty rebuild, per-title cost by size

| Size | Before: wall / titles per s / CPU ms per title | After: wall / titles per s / CPU ms per title |
|---|---|---|
| 10k | 170.0 s / 59 / 17.1 | 88.0 s / 114 / ≈9 |
| 50k | 1,050 s / 48 / 19.7 | 511 s / 98 / 10.6 |
| 200k, first 12 min (queue units drained per s) | 198 | 693 (3.5×) |
| 200k full | 44.1 min in scale-check-3 (with probes); ≈55 min projected from the 198 units/s above | 29.5 min (1,767 s, of which 141 s are log copies the tool runs between batches and the server runs concurrently) |
| 1m, first 15 min (queue units drained per s) | 165 | 714 (4.3×) |

Shape answer. Before: 198 units/s at 200k → 165 at 1m, about 17% more cost per unit at five times the size, and a few hundred milliseconds to seconds per group change in large groups. After: 693 units/s at 200k → 714 at 1m (flat within noise); per-title CPU 10k → 50k rises ≈15–20% in both versions from B-tree depth. Remaining 200k time is dominated by domain 25 (item metrics fanned out to containers), which with paced batches ends up as the last 20 minutes of the run; its per-unit work is constant (seeks), so this is absolute cost, not growth.

Shape: statements per title are identical at 10k and 50k for every top statement (before and after), so the work per title is constant; each statement is 15–35% slower per call at 50k (B-tree depth and memory hierarchy), which is the remaining mild rise. The group-size terms (show facets, container availability) were constant in this fixture only because its groups have fixed sizes (250-episode shows, a 5,004-song first artist at every size); in a library whose largest groups grow, they were quadratic per group and are now gone.

Writer page cache experiment (200k, 12 min, after the fixes): 4 MiB → 306k units left; 64 MiB → 314k left. No gain (the OS page cache already serves the pages); the policy is unchanged.

### Write-gate holds (per committed batch, commit included)

| | p50 | p95 | p99 | max |
|---|---|---|---|---|
| Before, 200k, 8-unit batches | 7.7 ms | 40.6 ms | 71.9 ms | 782 ms |
| Before, 50k (earlier run) | 3.4 ms | 33.6 ms | 74.2 ms | 1,298 ms |
| Paced, autocheckpoint in commit, 200k | 14.0 ms | 48.0 ms | 100.7 ms | 690 ms |
| Final (paced, commit counted, page copying outside the gate), 200k | 13.5 ms | 36.8 ms | 52.0 ms | 307 ms |
| Final, 50k | 13.2 ms | 29.4 ms | 43.5 ms | 655 ms |
| Final, 10k | 12.2 ms | 29.4 ms | 45.0 ms | 140 ms |

The long holds before were commits that ran SQLite's automatic checkpoint inside the gate (`SLOW_STEP … in_tx_ms=17` for a 690 ms step). The remaining rare 300–650 ms outliers are single batches on a heavily loaded shared host.

### Version-bump rebuild (browse rows + search)

10k: before 20.6 s, after 14.0 s. 200k after: 280.7 s, holds p50 19 ms / p99 154 ms / max 344 ms (no like-for-like 200k before run; scale-check-2 recorded 202.6 s on older code with a smaller search model).

### Trickle: one title's details at a time (10k fixture; show = 250 episodes, first artist = 5,004 songs)

| Update | Before: p50 / max ms, units, statements | After: p50 / max ms, units, statements |
|---|---|---|
| Show overview + studio + genres | 257 / 1,386, 268, 1,451 | 11.3 / 15.6, 6, 213 |
| Album overview + label | 20.6 / 52, 28, 269 | 5.3 / 7.2, 4, 147 |
| Artist overview | 5.0 / 8,094, 193, 1,024 | 5.0 / 7.8, 4, 141 |
| Episode overview + studio | 9.6 / 47, 5, 217 | 9.5 / 51, 5, 220 |
| Movie overview + tagline + studio | 9.8 / 49, 10, 290 | 11.8 / 20.6, 10, 288 |

At 200k (after): medians movie 11.8 ms, episode 7.8 ms, show 10.2 ms, album 5.5 ms, artist 4.8 ms, with the same units and statements per update as at 10k (5 / 6 / 4 / 4 per episode / show / album / artist). Two tool artefacts in that run, not per-update costs: the first movie update drained the 200k visibility-sort jobs the class builds had just queued (13.2 s, 65k units), and p95 ≈ 220 ms for every kind is the writer's 64 MiB backstop checkpoint, because the trickle loop does not run the server's 2-second log copy.

After the fixes every kind of single-title update costs a few units and a few hundred statements whatever its group size (`TestEpisodeTrickleCostIndependentOfShowSize` holds a 30- and a 300-episode show to the same units and no group walk).

### Restriction-class build

The quadratic plan is shown by `EXPLAIN` (finding 6) and held by `TestVisibilityClassChunksWalkForward`. In time it did not separate from noise at 50k (old music class 2.4 s first run / 8.0 s second run; new 2.7 s first run / 7.4 s second run — whichever ran second on this host was 3× slower), so no measured speed-up is claimed below 50k; the old plan reads L²/100 index entries, which is about 3.5 billion for the one-million fixture's 588k-song library. New build at 200k (PG-13 class, after the full derive): movies 3,921 rows 0.24 s, TV 79,057 rows 4.6 s, music 129,414 rows 9.6 s (13–17k rows/s).

## What remains and why I stopped

- The remaining per-title cost is spread over ~30 statements, led by the browse-row upsert (≈0.45 ms with its 25 triggers: six counted-block orderings, visibility, personal and eligibility job feeds, facet and availability counters, retention) and the 11-subquery sort-facts update. Further large gains need a bulk empty-build path (filling without per-row triggers and secondary indexes, then building blocks and counters set-wise) — a separate derivation implementation of every domain with its own crash and serving guarantees. The owner's guidance puts absolute speed second; the curve is flat, so I stopped here.
- Per-call cost still rises mildly with size (B-tree depth): ≈15–20% CPU per title from 10k to 50k, none measurable from 200k to 1m in queue drain rate.
- Not measured: a full one-million rebuild to completion (by rule), restricted-class builds at one million, and rebuilds with concurrent read probes (the earlier lanes' probe numbers stand; serving is unchanged).

## Decisions I made

- Statement cache: raised the entry bound only; the per-connection byte budget (≤4 MiB) is unchanged, so worst-case memory is unchanged.
- Writer page cache left at 4 MiB (64 MiB measured no gain).
- Batch pacing target 20 ms per batch, commit included, 2–64 units; fixed-limit callers unchanged. The previous lane's 8-unit limit remains the first batch and the limit for tools and visibility writes.
- WAL: the maintenance schedule copies new commits every 2 s (PASSIVE, read connection, no gate); the writer's automatic checkpoint becomes a 16,000-page (64 MiB) backstop for when the maintenance loop is held back. The 192 MiB hard bound and TRUNCATE escalation are unchanged.
- Dependent re-derivation: members follow a parent only for backbone fields (title, sort title, language, year) and the side-table fields `artist_id`, `author`, `library_id`, `show_id`, `album_id`, `book_id`; terms, credits and attributes are the entity's own. Retirement, deletion, merges and other `TouchEntityTx` callers keep the full fan-out.
- A browse row read before its title's availability is derived takes availability from its file links' flags instead of 0; availability derivation then queues the row only on a real difference.
- Narrowed `mb_artist_side_update` to identity (`local_key`) changes: outside this lane's area but a clear trickle cost; the MusicBrainz code reads no `catalog_artists` column.
- Fixture fidelity: `sqlite_stat4` removed after seeding in all my runs; earlier lanes' rebuild timings included STAT4 re-prepares that production never pays.

## Bugs

No `B-` IDs are assigned to this lane. Unnumbered defects fixed: a missing file that did not change a title's added date never reached its show's/album's `availability_state` (now carried through the metrics fanout; `TestFileAvailabilityReachesTitleAndShow`); an episode removed from a show without its browse row changing could leave its contribution in the show's facets (episode delete/move triggers now also queue the episode).

## API changes

None.

## Schema changes

Edited `0001_baseline.sql`, ran `scripts/schema-sync.py`:
- New `catalog_show_facet_episodes` and `catalog_show_facet_values`; episode delete/move facet triggers also queue the episode. Domain 33 (facet counts) version 4 → 5.
- New `catalog_browse_member_links` with six triggers on `catalog_browse_memberships` and `catalog_asset_links`.
- `mb_artist_side_update` fires only on `local_key` changes.
Pre-release: existing development databases must start fresh (digest changed); no compatibility code.

## Owner or viewer settings added or wired

None.

## Needed from others

- Reads lane follow-ups: the owner decided to skip the filter keys and fact-sort counted blocks (real views are rarely that large); ordered keys for release-year/date-added search remain open with the reads lane.
- Music/providers lane: confirm no MusicBrainz input depends on `catalog_artists` overview/tags (the code reads none).
- Final full-suite pass (not run here, by rule).

## Test notes for other environments

Host: Ubuntu, i5-10500T, shared with other lanes. Reproduce from `server/` with a lane-local `GOCACHE`:

```sh
go build -o /tmp/sb ./internal/catalog/scalebench
/tmp/sb -dir <scratch>/.scratch/f200k -items 200000 -samples 1 -idle 1ms -bulk-facts -narrow-facts -facts-only
# clear STAT4 so statistics match production, e.g. with any SQLite shell on the copy: DELETE FROM sqlite_stat4;
/tmp/sb -dir <scratch>/.scratch/f200k -items 200000 -samples 1 -idle 1ms -reuse -derive-only -no-probes -derive-for 12m
/tmp/sb ... -reuse -derive-only -no-probes -bump -classes -trickle 30   # full run, version bump, class builds, trickle
```

Read `DRAIN` (queue left, units/s), `STEPS` (hold percentiles), `SLOW_STEP` (domain, units, hold), `TRICKLE` and `CLASS_BUILD` lines. Pass: queue units/s within ~15% between 200k and 1m in the first 15 minutes; trickle updates of every kind ≤ ~10 units and no `TRICKLE_SLOW` naming a member walk; step p99 ≲ 60 ms on an unloaded host. Tests: `go test ./internal/compactcatalog -run 'GroupCounters|Trickle|FileAvailability|ArtistDetails|Batches|PacedBatches'`, `./internal/catalog -run TestVisibilityClassChunksWalkForward`, `./internal/dbwork -run 'TestWriterAutomaticCheckpointIsOnlyABackstop|Checkpoint|Statement'`. On Windows/macOS: repeat the 10k/200k partial runs; the WAL change should keep the `-wal` file in tens of MiB during a rebuild with the server's maintenance loop running.

## Not finished

- No full one-million rebuild (by rule); 1m evidence is the first 15 minutes of each.
- Class-build speed-up is shown by plan, not by a before/after large-library timing (noise at 50k).
- The 200k trickle run did not tick the log copy (see its note); the 10k trickle runs are the clean before/after.
- No concurrent-probe rebuild run on the final code.
- Full server test suite not run (touched packages pass: compactcatalog, dbwork, catalog, catalog/scalebench, persistence via schema-sync, metadata MusicBrainz subset; Windows/macOS cross-builds pass).
- Scratch folder and Go cache deleted at the end (logs summarised above).
