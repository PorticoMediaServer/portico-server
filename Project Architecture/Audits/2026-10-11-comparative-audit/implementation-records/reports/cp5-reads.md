# cp5-reads — the growing reads cp2-reads left

Host ehler-personal (mini-1), 16 Oct 2026. Branch `audit-implementation/cp5-reads` in `server-cp5-reads` (from 3d1f2b66) and `native-cp5-reads`. `audit-implementation/main` merged into both twice, the second time at `fd7814ed` (server) / `6aeb48c` (native). Committed with Justin's identity, never pushed. No subagents or helpers. Lean testing as the brief asked. Measured with scalebench at 50k and 200k (never 1M); fixtures, binaries and the Go cache deleted at the end, logs kept in `_program/scratch/cp5-reads/logs/` (5.7 MB).

## 1. Outcome

Server only; no API shape changed, so no client work was needed (the native worktree only carries the merges and the shared-package lock).

- **Restricted Home.** At 200k, the PG-13 Home falls from 2.26M to 1.20M steps and Top Shelf from 1.40M to 0.86M.
  - Continue Watching: 727k → 192k. Three changes:
    - The profile's unfinished episodes are kept by show (`profile_show_partials`), so a show whose newest episode is refused looks at the few the profile started, not every episode.
    - The next episode is looked for in the same season first, through the season index; the old whole-show ordering only runs when that season has nothing.
    - A started show the viewer can see none of is skipped with one lookup in the published visibility class.
  - Recommended: 441k → 182k. More recommendations: 398k → 131k. A candidate is tested against its library's published class before the live restriction (which walked every episode of a refused show), both in the pool's eligibility test and on the row's read.
- **Restricted library lists.** Every restricted list now checks the class first, then runs the live test (one key lookup refuses a hidden show).
  - Personal sorts on Shows, first page: 355k → 11k steps.
  - For you, first page, worst case: 1.6M → 0.39M.
  - "Available" filter, deep position: 7.9M → 0.13M. It is now served from the class's own rows and counts.
- **"Is present" / "is missing" on fields almost every title has.**
  - Each title keeps counters of its file links and of its acting, directing and writing credits, with one index over them.
  - "Is missing" reads the few titles that lack the field; "is present" reads the few that have it; each answer is the complement of the other.
  - Presence on a field few titles have (subtitles, sparse credits) is selected from its values.
  - Worst cases at 200k: versions/duration is-missing 5.06M → 0.42M; subtitle language 7.45M → 0.16M; credits is-present 3.41M → 0.84M.
  - On Shows, writer/director is-present was the 0.84M case. The last two commits name the few shows holding such an episode: at 50k, 209k → 404 steps. The 200k run predates this.
- **"Has no genre" on artists, albums and the Genres grouping.** An artist or album lacks a genre only if one of its songs does, so the songs without a genre name the candidates. A grouping's filter now knows the kinds it counts. Music genres/albums/artists at 200k: up to 6.59M → 184 steps.
- **Title text.** Each list row's folded title is kept as trigrams (an FTS5 trigram table maintained by triggers, `catalog_title_grams`). At 200k:
  - "contains / starts with / equals / in" matching a few titles reads those rows: 0.59–1.66M → 10–23k steps.
  - Text matching nothing: 0.85–1.65M → under 200 steps.
  - Broad text still takes the per-title path, after a probe of at most 2,000 rows.
- **History past 10,000.**
  - The history sorted by title reads past any length: the profile's own rows keep their title's sort key, which moves with a rename.
  - Last played reads the history index directly.
  - Playing the history in its own order, by last played, or by title queues all of it, not the most recent 10,000.

Per-item write cost stays flat: a few constant trigger writes per title, credit, file link and progress row.

## 2. Decisions I made

- **The published class is a first test, not the answer.**
  - A restricted list, the rec rows and Continue Watching first ask whether the title's library class holds it. A title the class lacks is refused at once; for one it holds, the live restriction still decides.
  - The cost: a title newly allowed since the class's last refresh stays hidden until the class catches up (seconds, through its journal). Recently Added and restricted totals already behave this way.
  - Nothing newly restricted is shown: the live test still runs.
  - A library whose class is not yet built falls back to the live test alone.
- **Counters are the answer for playable titles.**
  - A film, episode, song or book file is its own member, so its counters answer duration, cast, director and writer presence without testing the title again.
  - For containers they only name candidates, which are tested exactly. On containers only "has it" is enumerated: "lacks it" would miss a container with no members.
- **Probes stop at 2,000 rows** for presence, title text and the counters' candidates (value filters keep cp2's 20,000).
  - Learning that a broad condition is broad must stay cheap. It costs a broad title or presence filter a fixed 14–25k steps (a few ms), visible as the small "worse" entries in section 7.
  - A condition matching 2,000–20,000 titles behaves as before (per-title test).
- **"Has no genre" for artists and albums** starts from the songs without a genre. An artist or album with no songs at all is therefore not offered as a candidate. Such rows are retired in practice, but this is a change in an edge case.
- **Restricted deep pages in fact orders are left as they are** (release date, album, file size…; 199k → 200k steps at 200k).
  - Making them flat needs those keys on visibility-class rows, with counted blocks: 9 more indexes and triggers on every class build. cp2 rejected that for a rare view, and I agree.
  - A walk of the unrestricted index with a class check is only cheaper for shallow positions in dense classes, so I did not add it either.
- **History beyond 10,000:**
  - Title and last played sorts read the whole history.
  - The other sorts (year, date added, duration, ratings, play count) still sort the 10,000 most recent, flagged as a lower bound.
  - The letter rail is not published for a history beyond 10,000: counting letters would read the whole history on every first page.

## 3. Items closed

No audit bug numbers apply. These are the items from cp2-reads' "Not finished" list in the lane brief.

| Item | Change | Test |
|---|---|---|
| Restricted Home (Continue Watching) | `profile_show_partials`; same-season next-episode step; class test on started shows | `TestContinueFallbackStartsWithTheShowsPartialEpisodes` (plan now on the partials index), Home/Continue tests |
| Restricted Home (rec rows) | class test in `recEligible` and on the row read (`publishedClassesSQL`) | recommendation and restriction tests |
| Restricted lists generally | class test ahead of the live restriction in `browseSelect`; class rows for restricted "available" deep pages | `TestAvailabilityCountersMatchRestrictedPages` (extended: every direct position, both viewers) |
| Presence filters | fact-gap counters + index; presence from counters, or from few values; versions on playable lists constant | `TestPresenceGenreAndTitleFiltersMatchTheTitleTest` (new: parity with the per-title test, films, shows, episodes, songs, albums, artists), `TestFactGapCountersFollowCreditsAndLinks` (new), `TestGroupingPresenceFiltersCountTheirSongs` (new) |
| Has no genre on groupings and artist/album lists | song-based candidates; grouping compiler knows its kinds | same parity test (albums, artists, songs), grouping test |
| Title substring | `catalog_title_grams` trigram index + candidates | parity test (contains/starts-with/equals/in/none/short) |
| History sorted / queued beyond 10,000 | `catalog_personal_sort_rows.sort_key` + `catalog_personal_history_title`; history reads for title and last played; whole-history queue | `TestHistoryAcrossLibrariesSortsAndQueuesPastTheMembersLimit` (new; rename moves the title) |

## 4. API changes

None to the API shape.

Behaviour changes:
- History in title or last-played order pages past 10,000.
- A queue of the history in its own order, by last played or by title holds all of it.
- Filter results are unchanged; they are only cheaper to read.

## 5. Schema changes

All in `0001_baseline.sql`, then `scripts/schema-sync.py`:
- `catalog_entities`: new columns `asset_links`, `acting_credits`, `directing_credits`, `writing_credits`.
  - Kept by triggers on `catalog_asset_links` and `catalog_credits`. The credit triggers use the actor, director and writer fields' own label tests.
  - Index `catalog_entities_fact_gaps(kind, <gap mask expression>, id)`.
- New `catalog_title_grams`: an FTS5 table with no stored content, trigram tokenizer, case-sensitive over folded text, `detail='none'`. Insert, update-of-title and delete triggers on `catalog_browse_rows` keep it.
- New `profile_show_partials(profile_id, item_id, show_id, updated_at)` with two indexes.
  - Triggers on `progress_activity` (insert, update, delete) and on `catalog_episodes` when an episode moves to another show.
- `catalog_personal_sort_rows.sort_key`, filled by the personal-sort refresh and moved by a trigger when a browse row's sort key changes.
  - Partial index `catalog_personal_history_title(profile_id, sort_key COLLATE NOCASE, entity_id) WHERE last_played_at<>''`.

## 6. Settings added

None.

## 7. Tests run and measurements

| Command | Result |
|---|---|
| `go test -timeout 40m ./internal/catalog ./internal/compactcatalog ./internal/catalog/scalebench ./internal/ingestion` (after the final merge) | ok (574 s / 214 s / 108 s / 147 s) |
| `go test ./internal/persistence` (includes the no-catalogue-scan guard, which caught one statement during the lane; fixed) | ok |
| `go test ./internal/httpapi -run 'Restriction\|Matrix\|Browse\|Home\|Across\|Queue\|Search\|TopShelf'` | ok |
| `go test -timeout 40m ./internal/catalog` after the last commit | ok (240 s) |
| `go build ./...`, `go vet` on catalog and compactcatalog, gofmt on touched files | clean |
| `go run ./cmd/apigen` | no diff |
| native `scripts/shared-packages.mjs --write` | lock only, committed |
| `PORTICO_PERFORMANCE_TIER=release PORTICO_BUILD_FILES=2000 go test ./internal/ingestion -run TestBuildLibraryFromEmpty` | mine 20.3 ms/file vs main 25.2 ms/file (run one after the other, load 13); side by side 25.5 vs 21.0. That is noise either way: no visible cost to building a library from scratch. |

Not run: the full server suite, web/native/Vega suites, end-to-end drivers (left to the combined verification). No native, web or Vega code changed.

**Scale.** `scalebench -bulk-facts -narrow-facts -facts-only`, `sqlite_stat4` cleared, full derive. Before fixtures were built with main's binary; after fixtures were seeded with the lane's schema.

Reads: `-matrix-only -focus-samples 3` over Home and its rows, the presence and title filter matrix, focused PG-13 reads, and the new `title-filter/…` routes (a rare, an absent and a broad text, both viewers).

Steps are the deterministic maximum SQLite VM steps per request. Milliseconds are medians on a shared host (load 7–15, often with tests running beside), so read the steps.

The 50k pair below ran after both 50k fixtures took the same `-ingest` writes. The 200k pair is untouched fixtures.

| Read | routes | before 50k | after 50k | before 200k | after 200k |
|---|---|---|---|---|---|
| Home, all rows (PG-13) | 1 | 711,507 | 632,788 | 2,262,215 | 1,200,838 |
| Home, all rows (open) | 1 | 428,704 | 428,559 | 669,072 | 669,161 |
| Top Shelf (PG-13) | 1 | 389,924 | 344,963 | 1,396,080 | 860,402 |
| Continue Watching row (PG-13) | 1 | 93,102 | 48,195 | 727,496 | 191,871 |
| Recommended row (PG-13) | 1 | 157,323 | 143,350 | 440,914 | 182,047 |
| More recommendations row (PG-13) | 1 | 115,161 | 95,370 | 398,282 | 131,497 |
| Filter is-missing versions/duration (med / max) | 56 | 44,572 / 1,273,499 | 184 / 105,535 | 94,303 / 5,059,004 | 184 / 421,714 |
| Filter is-present versions/duration | 56 | 60,561 / 800,573 | 55,554 / 568,344 | 101,176 / 1,117,847 | 66,559 / 546,436 |
| Filter is-missing actor/director/writer | 54 | 86,775 / 452,091 | 81,586 / 285,972 | 166,865 / 1,128,709 | 93,826 / 694,422 |
| Filter is-present actor/director/writer | 54 | 78,479 / 883,773 | 48,789 / 216,251 | 243,960 / 3,414,127 | 20,290 / 836,233 |
| Filter subtitle language is-present/missing | 36 | 50,896 / 1,929,325 | 226 / 124,696 | 57,979 / 7,451,105 | 228 / 164,806 |
| Filter genre is-missing, music genres/albums/artists | 6 | 275,894 / 1,660,231 | 17,564 / 18,710 | 1,044,276 / 6,588,591 | 121 / 184 |
| Title contains a rare text (4 lists, both viewers) | 4 | 217,101 / 414,736 | 638 / 643 | 591,844 / 1,663,726 | 10,493 / 23,476 |
| Title contains a text nothing has | 4 | 217,101 / 414,736 | 151 / 179 | 851,133 / 1,647,226 | 151 / 179 |
| Title contains a broad text | 4 | 64,902 / 2,000,808 | 59,017 / 1,190,136 | 159,062 / 6,685,929 | 112,617 / 3,926,642 |
| Title filter, matrix (broad "Title") | 224 | 16,094 / 633,215 | 29,173 / 643,728 | 39,172 / 1,411,932 | 52,521 / 1,425,955 |
| PG-13 "available", deep | 18 | 144 / 2,000,239 | 203 / 48,075 | 144 / 7,899,363 | 203 / 127,481 |
| PG-13 personal sorts on Shows, first page | 6 | 29,925 | 13,420 | 355,117 | 11,258 |
| PG-13 For you, first page | 3 | 188,168 / 202,552 | 167,106 / 176,386 | 478,081 / 1,606,888 | 242,092 / 388,875 |
| PG-13 fact-order deep pages | 46 | 63,343 / 120,790 | 64,352 / 121,799 | 198,628 / 291,159 | 199,637 / 292,168 |
| PG-13 focused reads, all | 401 | 24,285 / 2,000,239 | 23,684 / 604,036 | 40,705 / 7,899,363 | 31,072 / 613,649 |

Full per-route data: `logs/{before,final}-200k.json`, `logs/{before-post,final}-50k.json`. Earlier iterations (`after*-50k`) show the probe-size tuning. `logs/final-table.md` has the table with medians and ms.

**Got worse.**
- Broad title text and broad credit presence on groupings and films: +14k to +25k steps. That is the bounded probe that learns they are broad.
- The broad title/presence paths are otherwise unchanged.

**Write cost (50k, `-ingest`, side by side, ms per unit, before → after):**

| Write | before | after |
|---|---:|---|
| Add film | 16.2 | 16.8 |
| Add episode to the largest show | 15.9 | 17.3 |
| Add song to the largest artist | 15.0 | 16.5 |
| Metadata trickle | 21.9 | 20.0 |
| Progress tick | 1.79 | 2.40 (derive part noisy) |
| Mark / undo 500 episodes, ms | 101 / 107 | 108 / 102 |

**Fixture growth:** 6,017 → 6,117 bytes per item at 200k (+1.7%). Derived units for the empty rebuild: 3,409,641 → 3,470,079 (+1.8%).

## 8. Device test notes

Server-side reads only; no device-specific behaviour. To check by hand on any client against a large library:

1. **Kid's profile Home** (PG-13 or a blocked label), with a long history in a big TV library.
   - Pass: Home opens as fast as an unrestricted profile's.
   - Pass: Continue Watching shows the next allowed episode of partly watched shows.
   - Pass: a show the profile may not see never appears.
2. **Kid's profile, Shows list:**
   - Sorted by last played or rating; then filtered to "Available" and scrubbed far down. Pass: pages land at once.
   - Under For you. Pass: the first page appears at once.
3. **Filters:**
   - "Director is missing", "Duration is missing", "Subtitle language is present" on Episodes. Pass: the results are the same as before, immediately.
   - "Genre has none" on music Albums/Artists/Genres. Pass: the same.
   - "Title contains" a rare word or a code. Pass: the same, immediately; a common word behaves as before.
4. **History with more than 10,000 titles** (seeded database):
   - Saved › History sorted by title. Pass: scrolls past 10,000 in title order, count reads "10,000+".
   - Play the history in its own order. Pass: the queue holds the whole history.
5. **Renames:** rename a watched title. Pass: History in title order moves it to its new place.

## 9. Not finished

- **Restricted deep fact-order pages** are unchanged (decision in section 2): ~200k steps at 200k, growing with the class.
- **Restricted Home still grows a little.**
  - Continue Watching: 48k → 192k from 50k to 200k. Continue Listening is bounded by its 1,000-activity window.
  - What remains follows the profile's history per show, which this fixture scales with the library (one activity per 100 items). It does not follow the catalogue.
  - Not separated further.
- **Broad conditions under a restriction** keep their per-title cost. Example: PG-13 "title contains a broad text" on songs, 6.7M → 3.9M at 200k; the count to 10,000 tests each title live. A22's filtered/restricted counts are not otherwise addressed.
- **Deep pages of broad filters** (start 50,000 into "genre is present") still walk to the position: 2.5M at 200k, unchanged.
- **Subtitle language "is missing"** when nearly every title has subtitles has no counter (validity depends on each file's evidence), so it still tests each title. "Is present" when few have them is fast.
- **Presence of a counted field on container lists:**
  - "Lacks it" is not enumerated (empty containers).
  - "Has it" is enumerated from members (`f99c37c6`, `c637808e`). It was measured only on a fresh 50k fixture (`logs/last2-50k.json`):
    - Shows writer/director is-present 209k → 404 steps; writer is-missing 164k → 2.7k.
    - Actor is-present (many) 79k → 86k; collections actor 142 → 7.2k (the bounded 500-row probe).
- **History beyond 10,000** in the other sorts, and its letter rail (section 2).
- The trigger-generated fact-order block SQL still has no generator in the repository (cp2-reads' note); not touched.

## Commits

**portico-server** (`audit-implementation/cp5-reads`):

| Commit | What |
|---|---|
| `1b1e6d56` | Continue Watching partials, rec class test |
| `f1763885` | Presence counters, missing genre, title trigrams |
| `f0a435f9` | History by title and last played past 10,000, whole-history queue |
| `f4ecb896` | Scalebench title-filter routes |
| `62510f0b` | Restricted "available" deep pages from the class |
| `aab19913` | Class test ahead of the live restriction in lists |
| `c1c72a9e` | Merge |
| `3183df2d` | Groupings of playable titles as item lists |
| `e8490377` | Narrow probes, counted fields from counters |
| `541516d4` | Playable titles answered by counters |
| `18f9493e` | Same-season next episode, started-show class test, rec pool class test |
| `e35bfc5b` | Merge |
| `f99c37c6` | Container "has it" from members |
| `c637808e` | Smaller probe for container members |

**portico-react-native** (`audit-implementation/cp5-reads`):

| Commit | What |
|---|---|
| `c3e1c1f` | Lock |
| `51b8e79` | Merge |
| `d52f03a` | Lock |
| `804ae05` | Lock, mirrored from server `c637808e` |
