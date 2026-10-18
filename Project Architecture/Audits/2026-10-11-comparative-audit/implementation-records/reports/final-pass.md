# final-pass

Host ehler-personal (mini-1), 14 Oct 2026 17:47–19:45. Worked directly in `main-server` and `main-native` (branch `audit-implementation/main`); committed, never pushed. No subagents, no helper model.

## 1. Outcome

- **Native apps show capped totals like the web.** Phone, tablet, TV and Vega grids and lists treat a broad filter's total as open-ended: the grid grows as pages arrive, never shrinks when an earlier page loads, and settles on the exact count at the end; counts read "10,000+ movies" (library count, TV eyebrow, a person's filtered credits, genre and category tiles). Scroll position and TV focus are unaffected (rows keep stable keys). Inexact search counts are no longer shown on native either.
- **The hung web test is fixed.** The open handle was the audio renderer's 50 ms timer; every test now releases it even on failure or timeout, and the whole web suite exits by itself (71 s).
- **Everything passes.** Server `go test ./...` (87 packages), apikit, client-core (1,947), web check (546), cast (20), production web build, Windows/macOS cross-builds, both native type-checks, Vega type-check, both string lints, native tests (660), Vega tests (87), shared packages identical. `go vet` shows only the known vendored SQLite warnings. Failures found were stale tests/fixtures or guard listings after the perf merges; no guard, bound or security test was weakened.
- **End to end passes:** server driver 18/18 groups; browser driver 13/13 groups, 114/114 screens.
- **Scale shape:** per-title add cost is flat 50k → 200k and seed writes are flat to 1M; rebuild work per title is constant (16 queue units per title at every size) and the main-phase drain rate is flat 200k → 1M (3,332 → 3,274 units/s), with a ~10–18% per-title rise 50k → 200k from B-tree depth. Main browse and search reads are flat in the median (×1.13 and ×1.00 steps for 4× size), but a few specific reads still grow (listed under step 5).

## 2. Decisions I made

- Native: Show/Album/Artist track and episode lists take the growing total but show no count text for it (a single group does not approach 10,000).
- Native genre tiles say "10,000+ songs"; the shared card caption for a lower-bound group count says "10,000+ titles" (the web's existing rule), so the native and web read the same.
- Fixed in both web and native: a list reloaded after a stale continuation past an open-ended total keeps its position instead of jumping back to 9,999.
- Took the scale-check-3 request into native as well (hide `totalCountExact:false` search counts), because it is the same "don't show a number the server didn't count" rule and native was showing them.
- The web test hang was fixed in the test (30 s per-test timeout so `after` always disposes the renderer), not by `--test-force-exit` and not by `unref()` in product code: the runner must exit by itself and the product timer must keep running in browsers.
- `scalebench` is a development tool, not built into the server: its `du` call was listed in the spawn inventory with that reason; its two goroutines were moved to `supervise.Go` (rather than exempting the package) so the goroutine guard stays unconditional.
- The read-path golden was regenerated only after classifying every one of the 276 differing cases (see step 3). The group-pivot range clamp (53fdbd1c) was accepted as intended: item pivots have clamped a past-the-end range to the last item since the baseline.
- Scale run: seeding 1M facts is set-up for the 1M run and was allowed to finish (14 min); the 1M **rebuild** was stopped by `-derive-for 12m` at 2.33M of ≈16M units (≈14%). The 1M reads and 1M add scenarios need a complete 1M rebuild (the ingest scenario drains the whole queue first), so they were not run; 1M write cost comes from the seed batches.
- Reads were measured for the open viewer only (`-skip-class-build`); restricted-viewer reads were covered by perf-reads.

## 3. Bugs

No B-numbers assigned to this lane. Defects found and fixed: native grid stopping at 10,000 under a broad filter (lower-bound flag dropped in the native page source); web and native reload past the cap jumping back to 9,999; native showing inexact search counts; web test leaking the renderer timer when an await never settled. Stale tests/fixtures after merges: native Home fixture, httpapi discovery test, read-path golden, scalebench vs spawn/goroutine guards.

## 4. API changes

None. (Clients now honour existing fields `pageInfo.totalIsLowerBound`, entry `totalAtLeast` → `countIsLowerBound`, and search `totalCountExact`.)

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

None.

## 7. Needed from others

- Search reads that still grow with library size (step 5): broad multi-word prefixes ("Title 0", "Title 0000": prefix-term expansion and a full-text relevance fallback, up to 258 ms at 200k) and the people group's per-person credit counts. Worth a follow-up in the search area.
- Browse: first page of songs/episodes sorted by duration or community rating **ascending** grows linearly (×3.8 steps, 70 ms at 200k); deep fact-order pages grow as already accepted by the owner.
- The full composed Home (`HomeRows`, all shelves in one call) doubles 50k → 200k (166 ms); clients now use the manifest plus per-shelf requests, so this matters only if a caller still composes everything at once.
- Architecture docs: describe capped totals and open-ended client totals (perf-reads already lists this).

## 8. Test notes for other environments

- Native capped totals on devices: on a library with more than 10,000 titles matching a broad filter (e.g. title contains a common letter), the count under the filter bar (phone/tablet) or the TV eyebrow reads "10,000+ …"; scrolling or letter-jumping past 10,000 keeps loading; reaching the end shows the exact count; TV focus stays on the focused card while the count changes. Fail: the grid stops at 10,000 or jumps back after a library change.
- Native search: a several-word search over a large library shows no count on its chips or "See all" when the server could not count exactly.
- Reproduce the scale shape: from `server/`, build `./internal/catalog/scalebench`, then per size `sb -dir X/.scratch/fN -items N -samples 1 -idle 1ms -bulk-facts -narrow-facts -facts-only`, clear `sqlite_stat4`, `-reuse -derive-only -no-probes [-derive-for 12m]`, `-reuse -matrix-only -skip-class-build -focus-samples 3 -only '^(focus/open/|open/(home-manifest|home|movies|songs)$)'`, `-reuse -ingest`. Compare DRAIN units/s by phase and `maxVmSteps` between sizes.

## 9. Not finished

- No 1M read or 1M add (ingest) measurement: both need a complete 1M rebuild, which the brief forbids. 1M evidence is the seed write cost (whole run) and the first 12 minutes of the rebuild.
- The rebuild's domain-25 tail phase (item metrics fanned out to containers) was measured at 50k and 200k only (−18% per unit 50k → 200k); at 1M it starts after ≈40 minutes and was not reached.
- The growing reads listed in section 7 are reported, not fixed.
- Device runs (iOS, tvOS, Android, Fire TV, Vega hardware) not possible here; native coverage is type-checks and node tests.

## Step log

### Step 1 — native capped totals (done)
- `main-native` c0f2047: shared packages refreshed from main-server (10 files: browse lower bound, windowed collection, library-content `countIsLowerBound`, Home/search/contract changes from the perf lanes). Both native type-checks passed unchanged after the sync.
- `main-server` 6bce45a3: shared `cardCaption` now captions a group counted only up to 10,000 as "10,000+ titles" (same rule as the web's `kindCaptionFor`), so category/decade/collection tiles on every native platform read it; web `browsePageSource` no longer clamps a stale-continuation reload past an open-ended total back to 9,999 (it kept the old exact-total clamp). Tests: client-core `presentation.test.ts`, web `browse-exact.test.ts`.
- `main-native` 10b4d9d: `browsePageSource` passes `totalIsLowerBound` to the windowed collection (previously each page's capped total was taken as exact, so the grid would have stopped at 10,000 and flipped size as pages arrived) and keeps the position on reload past the cap; `BrowsePages` (group lists inside a page) carries the flag; new `app/browse-count.ts` (`browseCountLabel`, `titleCountLabel`, `songCountLabel`) used by the library count under the filter bar (phone/tablet), the TV library eyebrow, a person's sorted/filtered credit count and music genre tiles. Idle snapshots gained `totalIsLowerBound: false`. Grid rows keep stable `row:N` keys, so growth re-renders but never remounts rows: scroll position and TV focus are untouched. Vega runs these same screens (metro reads `apps/native/src`), so it is covered. Test: `apps/native/test/browse-total-cap.test.ts` (growth, no shrink on earlier pages, settles at end, exact below threshold, reload past cap keeps position, BrowsePages flag, all labels).
- Decision: show/album/artist track and episode lists (Show, Entity) take the growing total but show no count text for it; a single group never approaches 10,000 members, so no label change there.

### Step 2 — hung web test (done)
- Open handle: `DecodeAudioRender`'s 50 ms `setInterval` (web/src/bridge/DecodeAudioRender.ts:76), released only by `dispose()`. The day-old hung processes came from lanes on branches older than b827f3a9 (13 Oct), when a failing assertion (missing `Content-Range` in the fixture) skipped the trailing `dispose()`. b827f3a9 moved disposal into `ctx.after`; the remaining way to hang was an await that never settles (an `after` hook only runs when the test ends, and node:test has no default timeout).
- `main-server` b9c3fed5: the three renderer tests get a 30 s timeout; `after` runs on timeout (verified with a scratch test), so the timer is always released. Verified: the file passes and exits in ~5 s; a deliberately broken copy (no `Content-Range`) fails 3 tests and exits in 2.3 s; whole web suite `npm test` (546 tests) exits by itself in 71 s. No product code changed.

### Step 3 — run everything (first pass findings)
Host loaded by other work (load 3–9). GOFLAGS=-p=4, lane GOCACHE. Command logs kept in `_program/scratch/final-pass/logs/` (1.4 MB, no credentials); databases, binaries and Go cache deleted.

| Command | First result | Cause | Change |
|---|---|---|---|
| playback helper build | exit 0 | | |
| `go vet ./...` (server) | exit 1, 63 diagnostics, all `possible misuse of unsafe.Pointer` in `internal/thirdparty/sqlite` | known vendored SQLite | none |
| `go vet ./...` (apikit) | exit 0 | | |
| `go test ./...` (server), 9 min | 3 packages failed | see below | |
| — `mediaexec.TestEveryProcessSpawnIsMediaexecOrListed` | `scalebench/scratch_budget.go:scratchUsage` runs `du` unlisted | perf lanes' benchmark × spawn inventory | listed with reason (development tool, not built into the server); 47a26f0d |
| — `supervise.TestEveryGoroutineIsStartedThroughThisPackage` | two bare `go func()` in scalebench (`exercise.go` probes, `main.go` log copies) | perf lanes' benchmark × goroutine discipline | both now `supervise.Go`; 030f9e2b. Guard unchanged. |
| — `httpapi.TestLibraryDiscoveryAndCollectionPermissions` | Home returned only deferred shelves (empty entries) | fast-cold-Home manifest (43851acc) × old test reading entries inline | test reads the manifest then each deferred shelf from its endpoint, as clients do; the permission-leak and "unauthorized change leaves Home identical" comparisons now cover the composed shelves (as strong as before); 19752fe6 |
| — `httpapi.TestReadPathGoldenEquivalence` | 276 cases differ | 219 only gain `totalIsLowerBound:false`; others: group pivots clamp a range past the end to the last item (53fdbd1c, now the same as item pivots since baseline), search relevance/title ties ordered by internal id (perf-reads), new `totalCountExact`, deferred Home shelves + per-row `viewerFence`, catalogue revision counters of the fixture | regenerated with `PORTICO_GOLDEN_UPDATE=1` after classifying every differing case (no entry set changed except the deterministic tie-break page of `search:Person` movies, 40 equal matches); f719c2a0 |
| `go test ./...` (apikit) | exit 0 | | |
| `npm run test:core` | 1,947/1,947 pass | | |
| `npm run check --workspace portico-web` | exit 0, 546/546 | | |
| `npm run test:cast` | 20/20 | | |
| production web build (`--configLoader runner`) | exit 0 (chunk-size warnings only) | | |
| Windows / macOS (amd64, arm64) `CGO_ENABLED=0 go build ./...` | exit 0 | | |
| native `npm test` | 659/660: `content-forward` Home fixture lacked row `viewerFence` | shared Home contract now requires it | fixture fixed; 3de2ff0 |
| native — search totals | not a test failure: native showed search counts the server marks inexact (`totalCountExact:false`, scale-check-3 "needed from others") | | chips, "See all (N)" and the focused-group count shown only when exact (web does the same); 8c8b973 |
| native `npm test` rerun | 660/660 | | |
| Vega `npm test` | 87/87 | | |
| `go test ./...` (server) rerun after fixes | exit 0: 87 packages ok (80 reused from the first run's cache with identical inputs; apispec, scalebench, dbwork, httpapi, mediaexec, persistence, supervise re-ran), 17 without tests | | |

### Step 4 — end to end (pass, no changes needed)
- Production server built with `make build-server` (CGO off, trimpath) into scratch; web bundle from the production build above.
- Server driver `scripts/e2e-server.py --binary …`: **all 18 scenario groups pass** in 75 s (fresh loopback server, generated media, generated credentials). Browse timing n=263 median 10.8 ms / max 69 ms; search n=184 median 17.2 ms / max 123 ms. Not exercised, as before: PGS generation (no encoder), canonical people grouping (local NFO only).
- Browser driver `web/test/integration/browser-smoke.mjs` (headless `/usr/bin/google-chrome` over DevTools, production bundle): **all 13 groups pass** (first-run, subtitles, browse, groups, freshness, console, media, profiles, tokens, downloads, csp, screen-walk, sign-out); screen walk rendered 114/114 screens (57 desktop, 57 phone). 4 min 27 s.
- No server or Chrome process left behind (checked). Evidence folders (holding the generated credentials) deleted after the results were recorded here.

### Step 5 — shape check at scale (done)
Method (as perf-rebuild/perf-ingest/perf-reads): `scalebench` built from main-server; fixtures `-bulk-facts -narrow-facts -facts-only`, `sqlite_stat4` cleared after seeding (production statistics), then `-reuse -derive-only -no-probes` (empty rebuild), `-reuse -matrix-only -skip-class-build -focus-samples 3 -only '^(focus/open/|open/(home-manifest|home|movies|songs)$)'` (open-viewer main browse first/deep pages for every sort of movies, shows, episodes, artists, albums, songs, plus 31 typed search prefixes; VM steps are deterministic, times are medians of 3), `-reuse -ingest` (add/change on the derived library at server batch size). Host otherwise lightly loaded (load 1–4).

Numbers (50k and 200k complete; 1M bounded, see below):

| | 50k | 200k | ratio |
|---|---:|---:|---:|
| Bulk seed (write with production triggers), ms/item first → last batch | 0.74 → 0.50 | 0.68 → 0.51 | flat |
| Empty rebuild: queue units per title | 15.9 (794,722) | 16.0 (3,209,096) | 1.00 |
| Empty rebuild wall / titles per s | 340 s / 147 | 1,608 s / 124 | −16% per title |
| — phase A (all domains interleaved) units/s | 3,720 | 3,332 | −10% |
| — first 5 min units/s | 2,396 (phase A ends at 100 s) | 3,533 | n/a |
| — phase B (item-metrics fan-out, domain 25, the tail) units/s | 1,731 | 1,419 | −18% |
| Step hold p50 / p99 / max ms | 12.5 / 36.5 / 138 | 12.2 / 41.4 / 424 | |
| Add film (write + derive) ms | 11.28 | 11.22 | flat |
| Add episode to largest show ms | 10.44 | 10.21 | flat |
| Add song to largest artist ms | 10.20 | 10.23 | flat |
| Metadata trickle ms | 14.48 | 14.13 | flat |
| Progress tick ms | 1.63 | 1.42 | flat |
| Mark / undo 500 episodes ms | 81 / 77 | 82 / 79 | flat |

Reads (open viewer, VM steps median over routes; time medians of 3 samples):

| Reads | routes | steps 50k | steps 200k | ×  | median ms 50k → 200k | max ms 50k → 200k |
|---|---:|---:|---:|---:|---|---|
| Browse first page, every sort, 6 pivots | 173 | 21,405 | 24,294 | 1.13 | 3.6 → 3.8 | 25 → 70 |
| Browse deep page (min(50,000, end)) | 173 | 79,695 | 90,243 | 1.13 | 4.3 → 6.8 | 24 → 42 |
| Search, 31 typed prefixes | 31 | 12,314 | 12,314 | 1.00 | 11.0 → 10.4 | 48 → 258 |
| Home manifest | 1 | 377 | 377 | 1.00 | 0.7 → 0.3 | |
| Movies / songs content page | 2 | 13,895 / 6,252 | 13,995 / 6,252 | 1.0 | 6.5/2.0 → 5.5/2.0 | |
| Full composed Home (all shelves in one call; clients now load shelves separately) | 1 | 480,884 | 984,648 | 2.05 | 100 → 166 | |

Reads that still grow with the library (4× size): first page of songs/episodes sorted by duration ascending or community rating ascending (×3.8 steps, 20 → 70 ms); deep pages of songs/episodes in fact orders (duration, community rating, year: ×4–6, up to 42 ms — the deep-fact-page counted blocks the owner chose to skip); typed searches whose second word is broad: "Title 0"/"Title 00" (×2.5, 76 ms; prefix-term expansion), "Title 0000" (×20, 27 → 258 ms; one group falls back to the full-text relevance ranking over its matches), "P…Person" (×3.1, 34 ms; the people group counts every credit of each listed person). Single-word and title-word searches are flat.


One million (bounded):

| | 50k | 200k | 1M |
|---|---:|---:|---:|
| Bulk seed ms/item, median per 200k window | 0.5–0.7 | 0.5–0.7 | 0.80, 1.26*, 0.62, 0.61, 0.66 (0–1M) |
| Seed total | 27 s | 130 s | 861 s |
| Rebuild phase A (all domains) units/s | 3,720 (10–100 s) | 3,332 (10–483 s) | 3,274 (10–618 s; still phase A at 12 min) |
| Rebuild first 10 min units/s | (finished in 5.7 min) | 3,052 (phase B began at 8 min) | 3,274 |
| Step hold p50 / p95 / p99 / max ms | 12.5 / 26.3 / 36.5 / 138 | 12.2 / 26.9 / 41.4 / 424 | 12.3 / 25.4 / 40.9 / 356 |
| Units drained when stopped | all 794,722 | all 3,209,096 | 2,334,021 of ≈16M (stopped at 12 min by `-derive-for`) |

\* the 200k–400k window overlapped native type-checks I ran on the same host.

**Answers.**
- *Is the per-title rebuild cost constant from 50k to 1M?* The work per title is constant (16.0 queue units per title at 50k and 200k; statements per unit unchanged). The drain rate is flat from 200k to 1M (3,332 → 3,274 units/s in the main phase, −2%, within noise; step holds unchanged). From 50k to 200k it is 10% (main phase) to 18% (domain-25 tail) slower per unit, the B-tree-depth rise perf-rebuild also measured; the tail phase at 1M was not reached.
- *Is the per-title add cost constant?* Yes, 50k → 200k: add film 11.28 → 11.22 ms, episode 10.44 → 10.21, song 10.20 → 10.23, metadata change 14.5 → 14.1, progress tick 1.6 → 1.4, mark/undo 500 episodes 81/77 → 82/79 ms. To 1M, the write side (seed batches with production triggers) stays at 0.6–0.8 ms/item; the derive side per title is the rebuild rate above (flat to 1M).
- *Do the main browse and search reads stay flat?* In the median, yes (50k → 200k): browse first page ×1.13 steps (3.6 → 3.8 ms), deep page ×1.13 (4.3 → 6.8 ms), 31 typed searches ×1.00 (11.0 → 10.4 ms), Home manifest and content pages unchanged. The exceptions that grow are listed above and in section 7. Reads were not measured at 1M (needs a complete 1M rebuild).

Clean-up: 50k/200k databases deleted after measuring, 1M database deleted after its 12-minute run, Go cache and binaries deleted; peak scratch use 4.4 GB (limit 6 GB). No test, server, Chrome or benchmark process left running.
