# cp2-reads — reads that still grow with library size

Host ehler-personal (mini-1), 15–16 Oct 2026 (resumed on 16 Oct after the 15 Oct stop; the coordinator's WIP commit `1eed43f0` was reviewed, its two benchmark probe files dropped and the rest reworded into `cd1eff49`), from main-server `6ae3ccf4` / main-native `ea762a7`, branch `audit-implementation/cp2-reads` in `server-cp2-reads` and `native-cp2-reads` (main merged in both; see the end). Committed with Justin's identity, never pushed. No subagents or helpers. Lean measurement as asked mid-lane: one before and one after run at 50k and 200k on reused fixtures, no 1M. The host was heavily shared (load average 20–39 during most runs), so the deterministic SQLite VM-step counts are the shape evidence and the milliseconds are noisy.

## 1. Outcome

Every read in the lane's list (and the remaining-sweep additions A22/A23) now costs what it returns or a fixed bound, not the library, except where section 9 says otherwise:

- **Search, broad multi-word prefixes.** Every prefix of a number (up to six digits) is now an ordinary search term with its own maintained count, so a typed number is one term like a word and the rarest word leads. "Title 0", "Title 00" and "Title 0000" no longer walk a broad word's postings looking for a rare number, and never fall back to ranking every full-text match. The numeric "terminal key" machinery (ranges of up to 128 terms, the full-text fallback past that) is deleted.
- **Search sorted by release year or date added** reads the browse rows' year/added indexes in order (a 4,096-row window, tested against the typed words), and when a typed word is rare, starts from that word's postings. Before, both ranked every full-text match (12–13M VM steps at 200k, and the episode and song groups ran out of their 600 ms budget and returned an error).
- **People.** A person's credit count and departments now come from counts the credit writes keep (`catalog_person_credit_counts`) for viewers without a content restriction. A name search counts its matches up to 1,000 (`totalCountExact:false` beyond); a broader one (a letter or two typed) pages people in name order, those whose name starts with the typed text first, instead of ranking every match by bm25.
- **Songs/episodes first page sorted ascending by duration or community rating** were sorting the whole library: in the fixture every song has the same duration, and SQLite does not see that an equality on an expression index's key satisfies an ORDER BY on the expression, so the "tie run" read and sorted every equal row. The tie run now orders by identity alone. This applies to every sort with large ties.
- **Deep pages in fact orders** (release date/aired, latest aired, critic rating, episode count, album, artist, track, file size, resolution) keep counted blocks like title and the other column orders, for the kinds each applies to; an unrestricted viewer's direct position starts at its block, and a stale range's anchor is re-located through the blocks.
- **Filters that matched few titles but tested every title** now start from the values that match: versions (a per-title version count kept by the file links), duration and resolution (files indexed by length and picture height), actor/director/writer (the credited names kept once each, tested once per name), subtitle language (the languages subtitle tracks carry, kept once each, plus the viewer's own uploads), and "has no genre" (titles whose own entity no genre source names, kept on the entity, plus the episodes of such shows).
- **Container card counts** (an album's songs, an artist's albums, a show's episodes on a card) come from the maintained membership counter instead of counting memberships per page.
- **Home**: a container card without artwork tries at most its first 64 members for a poster to borrow; Continue Listening reads at most the profile's 1,000 most recent unfinished activities; Continue Watching considers at most the 512 most recently watched shows and tests at most 64 later episodes of each for the next one (a restricted viewer refused most of a long history walked all of it). These bounds do not bind in the fixture; the restricted Home still grows (section 9).
- **The full composed Home (`HomeRows`)** is used by one first-party client: the Apple TV Top Shelf feed (`GET /v1/topshelf`). It now composes only until four rows have entries (`HomeRequest.Rows`), which is all the feed shows. Web, phone/tablet, TV and Vega use the manifest plus per-row reads; the per-row costs above are what made `HomeRows` grow.
- **Filter choices on a list across libraries** (A23) are counted over the list's own titles (its person, playlist, collection, Home row, My List, Favorites or history members, at most 10,001 as its pages read them), not the whole libraries. `GET /v1/browse/facets` takes `group` and `value`; web, phone/tablet, TV and Vega send them from the list's group condition (shared rule `facetGroupOf` in client-core).

Per-item write cost stays flat (constant extra triggers, no growth with library size); measured numbers in section 7.

## 2. Decisions I made

- **Numbers are prefixes like words.** Storing up to six prefixes per number (instead of one six-character terminal key) costs more postings rows per document (a year adds three, a six-digit number five), constant per document. In exchange every typed word has an exact maintained count, the numeric range code is gone, and no query falls back to full-text ranking for a number. Numbers longer than six digits still post their first six; a seven-digit query still uses the full-text index, as before.
- **Search sorts break ties by the internal id** for release year and date added too (title already did), so a page from an ordered index and a page ranked from full-text matches continue each other. The order values are the browse row's year and added date (what their indexes hold), read by key.
- **Broad name searches are ordered by name, not bm25.** Above 1,000 matches the people group shows names starting with the typed text first, then the rest in name order; at or below 1,000 the bm25 relevance (with the exact-name and name-prefix boosts) is unchanged. The total says "at least 1,000" (`totalCountExact:false`), as broad catalogue groups already do. If 20,000 people in name order hold neither the page nor the end, it ranks after all.
- **Restricted viewers' people counts stay live** (tested per credit against the restriction): a maintained count cannot know a restriction. Their cost follows the page's people's credits, not the library.
- **Fact-order blocks are kept for an unrestricted viewer only.** A restricted viewer's deep fact page keeps the published-class path (positions from the class, live restriction on the page). Adding fact keys to visibility classes would double the class maintenance for a rare view.
- **"Has no genre" candidates** are the titles whose own entity has no genre source, and episodes of shows with none. If more than 20,000 titles qualify, the per-title test is kept (where "has no genre" is broad the 10,000 cap bounds it). Counting genre sources on `catalog_entities` bumps the library revision on a genre change, as other entity updates do.
- **Home bounds**: Continue Listening window 1,000 unfinished activities; Continue Watching windows of 512 recently watched shows and 64 later episodes per show; poster borrowing tries 64 members. A row that a viewer's restrictions empty beyond those windows shows what the window holds.
- **Top Shelf** composes Home only until four rows have entries; `HomeRows` stays (the feed needs it) rather than being retired.
- **Container card counts** read `catalog_browse_member_links.members` (new column, kept by the membership triggers). It counts the live memberships, while a rebuild may still serve the previous generation's rows for a short while; the count may lead the card by that difference.
- **Reads must not join the serving view.** `catalog_browse_rows` is read through a two-generation view; inside a join SQLite reads it whole. New reads that need a browse row's value read it by key (scalar subquery) or in a window of its own (this is what kept the first version of the sorted-search walk slow; fixed before the after-runs).

## 3. Audit bugs closed and decision gaps closed

No B-numbers are assigned to these reads; they are the items of the lane brief, `final-pass.md` §7, `perf-reads.md` "Not finished" and the remaining sweep A22/A23:

| Item | What changed | Test |
|---|---|---|
| 1 Broad multi-word search prefixes | numeric prefixes as terms; rare-word lead; no FTS fallback | `facettext` prefix test; `TestBroadPrefixesSelectBoundedScopedPages`, `TestNumericPrefixesKeepUnicodeAndTypingSteps`, `TestPrefixCursorSurvivesDictionaryGrowth` |
| 2 Per-person credit counts (People group) | maintained counts; bounded totals; name walk | `TestBroadPeopleSearchWalksNamesInOrder` (new), people tests |
| 3 Songs/episodes ascending duration/rating first page | tie run ordered by identity | browse order/boundary tests; plan check by hand |
| 4 Deep pages in fact orders | counted blocks for nine fact orders | `TestFactOrderPositionsFromBlocksMatchTheOrder` (new, media tier), `CheckBrowseBlocks` |
| 5 `HomeRows` | Top Shelf composes four rows; per-row bounds | Home tests; new benchmark route `home-topshelf` |
| 6 Filters: versions, duration, resolution, credit names, genre presence | value-side selection from new counts/indexes/vocabularies | `TestValueSideFiltersMatchTheTitleTest` (new: parity with the per-title test for 16 cases) |
| 6 Search by release year / date added | ordered-index walk and rare-word postings | `TestSearchCatalogPageRanksBeforeHydrating` (updated tie rule) |
| 6 Restricted Home | Continue Watching window, Continue Listening window, bounded poster borrowing | Home/Continue tests |
| A22 subtitle-language filter | language vocabulary + viewer uploads, value side | `TestGroupingSubtitleFilterReadsCurrentEvidenceAndPrivateUploads` |
| A22 container card counts (F12) | maintained `members` counter | browse tests |
| A22 Home poster borrowing | first 64 members | Home tests |
| A22 Continue Listening | 1,000-activity window | `TestHomeContinueListeningStopsAtItsMaximum` (plan check updated) |
| A23 cross-library facet counts | counted within the list's members | `TestFacetsAcrossCountWithinTheList` (new), client-core `facetGroupOf` test |

## 4. API changes

- `GET /v1/browse/facets`: new optional query parameters `group` (one of `person`, `cast`, `crew`, `list`, `savedCollection`, `homeRow`, `watchlisted`, `favorite`, `lastPlayedAt`) and `value` (the person/list/collection/row id; empty for the three personal lists). With them the values are counted over that list's titles. Regenerated with `go run ./cmd/apigen` (`packages/contracts/src/routes.ts`, `server/api/portico.openapi.json`).
- `GET /v1/search`: the people group's `totalCount` is exact up to 1,000; beyond, `totalCount` is 1,000 and `totalCountExact:false` (clients already hide inexact counts). Release-year and date-added sorts break ties by the internal id (cursors are opaque; pre-release, no compatibility kept).
- No routes removed. `HomeRows` gains `HomeRequest.Rows` (internal).

## 5. Schema changes

`0001_baseline.sql`, then `scripts/schema-sync.py`:
- `catalog_search_prefix_terms`: `terminal` column and `catalog_search_numeric_prefix` index removed; `prefix` is UNIQUE. Search derivation version 8 → 9 (rebuild of postings).
- New `catalog_person_credit_counts(person_id, library_id, department_id, credits)` with triggers on `catalog_credits` (insert, delete, move) and on `catalog_entities` (credits leave before their entity; library move).
- New `catalog_credit_names(name, credits)` with triggers on `catalog_credits`; index `catalog_credits_name(credited_name, entity_id)`.
- New `catalog_item_version_counts(item_id, versions)` with index on `(versions, item_id)` and triggers on `catalog_asset_links`.
- Indexes `catalog_assets_duration`, `catalog_assets_height`.
- New `asset_subtitle_languages(language, tracks)` with triggers on `asset_subtitles`; index `asset_subtitles_language(language, asset_id)`.
- `catalog_entities.genre_sources` (count of genre sources naming the entity) with triggers on `catalog_term_sources`; partial index `catalog_entities_genreless(kind, id) WHERE genre_sources=0`.
- `catalog_browse_member_links.members` (memberships per row), kept by the existing membership triggers.
- `catalog_browse_blocks.ordering` now 0–14: generated join/leave/move triggers for nine fact orders (orderings 6–14), each limited to the kinds its sort applies to. The block SQL was generated from the year ordering's triggers by substitution; the generator itself (`blocks.py`, named in the baseline comment) is not in the repository, and I did not add one (see section 9).

## 6. Settings added

None.

## 7. Tests run and measurements

Tests (narrow, as asked; after merging `audit-implementation/main` 16 Oct):

| Command | Result |
|---|---|
| `go test -timeout 40m ./internal/catalog` | ok (535 s; an earlier run hit the default 10 min package timeout while a 200k benchmark shared the host) |
| `go test ./internal/compactcatalog ./internal/facettext` | ok |
| `go test ./internal/persistence ./internal/httpapi -run 'Statements|Scan|Facet|Across|TopShelf|Browse|Search|People'` | ok (includes the no-catalogue-scan guards, which caught two statements during the lane; both fixed) |
| `PORTICO_TEST_TIER=media go test ./internal/catalog -run TestFactOrderPositionsFromBlocksMatchTheOrder` | ok (before the merge) |
| `go run ./cmd/apigen`, `python3 scripts/schema-sync.py` | regenerated, committed |
| client-core `node --test test/browse.test.ts` | 14/14 |
| web `tsc --noEmit`, `npm run lint:hooks`, browse tests | pass (one web test caught a re-export the type-stripping loader refused; fixed) |
| native `tsc --noEmit`, `npm run lint:hooks` | pass |
| native shared packages | mirrored with `--write` from the merged server branch, lock committed |

Not run: full server suite, web `npm run check`, native/Vega `npm test`, end-to-end drivers (left to the combined verification).

**Measurements.** `scalebench` fixtures `-bulk-facts -narrow-facts -facts-only` at 50k and 200k, `sqlite_stat4` cleared, full derive; the before fixtures with the base binary, the after fixtures re-seeded with the lane's schema. Reads: `-matrix-only -focus-samples 3 -only '<the lane's routes>'` (focused browse for every sort and the 31 typed searches, both viewers; matrix filters on versions, duration, resolution, actor/director/writer, genre, subtitle language; search sorts; Home and each Home row, both viewers). Steps are the maximum over samples of SQLite VM steps (deterministic); ms are medians on a shared host (load 20–39), so read the steps. "Final" is the lane's code at `cd1eff49` (before the main merge, whose schema changes the fixtures predate).

| Read | routes | steps med/max before-50k | steps med/max final-50k | steps med/max before-200k-all | steps med/max final-200k | ms med/max before-50k | ms med/max final-50k | ms med/max before-200k-all | ms med/max final-200k |
|---|---|---|---|---|---|---|---|---|---|
| Search, 31 typed prefixes (open) | 31 | 12,314 / 434,409 | 12,284 / 66,176 | 12,314 / 1,316,387 | 12,283 / 56,383 | 19.7 / 89 | 50.2 / 115 | 10.2 / 239 | 20.5 / 46 |
| Search, 31 typed prefixes (PG-13) | 31 | 70,924 / 502,170 | 74,881 / 604,036 | 71,791 / 2,873,399 | 70,166 / 613,649 | 66.3 / 543 | 85.3 / 329 | 40.4 / 396 | 65.2 / 167 |
| Search "Title 0" / "Title 00" (open) | 2 | 434,409 / 434,409 | 16,588 / 19,188 | 1,097,652 / 1,097,652 | 14,034 / 14,034 | 85.1 / 89 | 55.0 / 60 | 77.2 / 81 | 44.0 / 46 |
| Search "Title 0000" (open) | 1 | 64,297 / 64,297 | 14,040 / 14,040 | 1,316,387 / 1,316,387 | 14,035 / 14,035 | 26.6 / 27 | 58.8 / 59 | 239.0 / 239 | 40.8 / 41 |
| Search people prefixes P..Person (open) | 6 | 64,641 / 64,641 | 62,146 / 62,154 | 200,786 / 200,786 | 56,383 / 56,383 | 25.9 / 32 | 62.5 / 76 | 33.4 / 39 | 30.2 / 38 |
| Search people prefixes P..Person (PG-13) | 6 | 348,565 / 348,565 | 604,028 / 604,036 | 1,256,064 / 1,256,064 | 613,649 / 613,649 | 132.2 / 172 | 175.3 / 197 | 118.3 / 124 | 134.8 / 142 |
| Search sorted by release year | 3 | 3,039,115 / 3,039,115 | 308,658 / 326,110 | 12,136,141 / 12,136,141 | 337,365 / 469,876 | 271.8 / 314 | 58.5 / 78 | 880.5 / 966 | 54.3 / 72 |
| Search sorted by date added | 3 | 3,387,836 / 3,387,836 | 398,387 / 1,583,923 | 13,194,143 / 13,194,143 | 361,582 / 826,800 | 380.3 / 474 | 69.4 / 235 | 888.9 / 1103 | 39.1 / 138 |
| Songs/episodes first page, duration/rating asc (open) | 4 | 289,381 / 344,756 | 20,772 / 22,951 | 1,101,784 / 1,319,644 | 23,546 / 25,714 | 18.7 / 24 | 9.5 / 11 | 61.5 / 86 | 8.8 / 12 |
| Deep page, fact orders (open) | 46 | 86,791 / 191,877 | 20,470 / 141,782 | 90,011 / 217,909 | 53,519 / 123,822 | 5.7 / 18 | 11.5 / 28 | 5.1 / 20 | 10.0 / 22 |
| Deep page, fact orders (PG-13) | 46 | 74,147 / 203,138 | 63,343 / 120,790 | 198,628 / 291,159 | 198,628 / 291,159 | 11.5 / 21 | 21.7 / 97 | 32.9 / 82 | 56.3 / 143 |
| Browse first page, every sort (open) | 155 | 35,530 / 344,756 | 15,113 / 391,085 | 59,277 / 1,319,644 | 15,103 / 245,231 | 5.0 / 32 | 8.2 / 52 | 4.8 / 86 | 6.6 / 37 |
| Filter versions | 242 | 373,788 / 2,364,495 | 181 / 1,214,272 | 405,190 / 9,457,105 | 181 / 4,823,707 | 23.8 / 218 | 1.5 / 213 | 123.0 / 4673 | 1.6 / 689 |
| Filter durationMs | 242 | 236,487 / 1,559,030 | 174 / 1,273,499 | 298,550 / 6,235,485 | 174 / 5,059,004 | 27.8 / 231 | 1.9 / 262 | 165.7 / 4589 | 2.2 / 1017 |
| Filter resolution | 56 | 80,230 / 1,196,176 | 12,239 / 273,628 | 318,464 / 4,784,440 | 196 / 164,783 | 9.2 / 161 | 5.5 / 40 | 148.4 / 3091 | 1.5 / 43 |
| Filter actor/director/writer | 420 | 87,590 / 853,675 | 10,568 / 883,773 | 321,883 / 3,414,127 | 8,164 / 3,414,127 | 11.6 / 124 | 11.1 / 154 | 120.9 / 1666 | 10.3 / 1064 |
| Filter genre is-missing | 22 | 52,630 / 2,970,853 | 45,276 / 1,660,231 | 209,973 / 11,882,588 | 33,018 / 6,588,591 | 5.1 / 272 | 16.8 / 252 | 133.1 / 5765 | 11.9 / 1059 |
| Filter genre (other operators) | 198 | 37,612 / 1,500,184 | 40,728 / 1,508,784 | 45,010 / 2,569,036 | 44,584 / 2,569,036 | 8.1 / 224 | 23.5 / 244 | 17.8 / 1038 | 27.9 / 473 |
| Filter subtitleLanguage | 140 | 5,006 / 1,862,825 | 185 / 1,929,325 | 18,179 / 7,451,105 | 187 / 7,451,105 | 1.2 / 74 | 1.8 / 154 | 4.0 / 1163 | 1.8 / 624 |
| Home, all rows (open) | 1 | 480,920 / 480,920 | 428,369 / 428,369 | 984,738 / 984,738 | 668,880 / 668,880 | 104.2 / 104 | 254.4 / 254 | 376.3 / 376 | 354.8 / 355 |
| Home, all rows (PG-13) | 1 | 796,419 / 796,419 | 708,189 / 708,189 | 2,530,892 / 2,530,892 | 2,262,114 / 2,262,114 | 167.2 / 167 | 360.9 / 361 | 836.1 / 836 | 880.4 / 880 |
| Home row continue_listening (open) | 1 | 156,457 / 156,457 | 171,516 / 171,516 | 594,585 / 594,585 | 340,487 / 340,487 | 19.6 / 20 | 79.6 / 80 | 156.8 / 157 | 97.5 / 97 |
| Home row continue_listening (PG-13) | 1 | 174,819 / 174,819 | 184,534 / 184,534 | 672,189 / 672,189 | 540,911 / 540,911 | 26.6 / 27 | 79.4 / 79 | 216.1 / 216 | 186.5 / 187 |
| Home rows, each (open) | 9 | 37,889 / 156,457 | 34,461 / 171,516 | 57,496 / 594,585 | 45,486 / 340,487 | 11.6 / 26 | 19.1 / 80 | 21.1 / 157 | 22.1 / 115 |
| Home rows, each (PG-13) | 9 | 92,943 / 201,882 | 91,227 / 184,534 | 226,546 / 727,618 | 87,807 / 727,564 | 26.5 / 48 | 57.1 / 79 | 83.1 / 230 | 27.0 / 193 |


Reading the table:
- **Flat now (×≈1 from 50k to 200k):** typed searches including "Title 0…" (14k steps at both sizes; before ×2.5 to ×20), people search for the open viewer (56k vs 201k before at 200k), release-year/date-added sorted search (≈0.3–0.4M at both sizes; before 12–13M at 200k with the episode and song groups timing out), songs/episodes ascending duration/rating (≈21–24k; before ×3.8), selective filters on versions/duration/resolution/credited names/subtitle language (medians 0.2–8k steps; before 0.3–0.4M at 200k), "has no genre" median.
- **Bounded, not flat:** PG-13 people search (≈0.6M at both sizes; counts up to 1,001 matches with the live restriction); Continue Listening (grows until its 1,000-activity window fills: the fixture profile has 500 activities at 50k, 2,000 at 200k).
- **Still growing (section 9):** the restricted Home (PG-13 Home 0.7M → 2.3M: Continue Watching and the recommendation rows), deep fact pages for a restricted viewer, presence filters (`is-present`/`is-missing`) on fields almost every title has (the per-title test, bounded by the 10,000 cap only when broad), and "has no genre" on the Genres grouping and container pivots, where the candidates exceed 20,000.
- **Top Shelf** (new route `home-topshelf`, Home composed until four rows have entries): 50k open 258k steps (full Home 429k), PG-13 387k (708k); 200k open 447k (661k), PG-13 1.40M (2.27M). It halves the composition; what remains grows with the Continue rows above.
- Deep fact pages, open viewer: medians 87k → 20k (50k) and 90k → 54k (200k); the remaining cost is the in-block walk and page hydration.

**Write cost (constant, slightly higher):**

| | before 50k | after 50k | before 200k | after 200k |
|---|---:|---:|---:|---:|
| Add film, ms per unit (write + derive) | 15.1 | 15.7 | 28.7 | 31.4 |
| Add episode to the largest show | 13.1 | 14.4 | 23.5 | 26.4 |
| Add song to the largest artist | 13.1 | 14.4 | 20.7 | 25.1 |
| Metadata trickle | 19.2 | 18.1 | 31.2 | 33.1 |
| Progress tick | 2.7 | 1.5 | 2.9 | 3.5 |
| Mark / undo 500 episodes, ms | 83 / 94 | 117 / 98 | 166 / 158 | 137 / 130 |
| Bulk fact seed (20k, side by side), ms/item first → last batch | 2.57 → 1.74 | 2.86 → 2.37 | | |

Each before/after pair ran side by side on the same host at the same time; the 200k pair ran while the catalogue tests were also running, which is why both 200k columns are about twice the 50k ones (the base code shows the same doubling, which is load, not size: final-pass measured add film 11.28 → 11.22 ms on a quiet host). The lane's maintained counts, vocabularies, indexes and nine fact-order block sets cost about 5–20% per added title and 20–35% on the raw fact seed; that is constant per item.

Fixture sizes: queue units for the empty rebuild 798,908 (before, 50k) → 840,480 (after, 50k), +5% (search postings for number prefixes, the fact blocks' row triggers).

## 8. Device test notes

Server-side reads; no device-specific behaviour. To check by hand on any client against a large library:
- Search "Title 0" or a word plus a short number in a large library: results appear at once and paging continues without repeats. Sort search results by release year or date added on a several-word query: every group answers (before, episode and song groups could time out).
- People search with one or two letters: names starting with the letters first, then the rest in name order; the people count reads as "1,000+" style (clients already hide inexact counts).
- A list across libraries (a person's titles, a playlist, My List): open Filters → a value list (genre, content rating…): the counts are the list's own titles, not the whole library (web, phone/tablet, TV, Vega all send the list's group).
- Songs sorted by duration ascending, and any fact sort (album, track, file size…) scrolled or scrubbed far down: positions land immediately (open viewer).
- Apple TV Top Shelf still shows four rows (Mac/tvOS check; the feed now composes only those).

## 9. Not finished

- **Restricted Home still grows** (PG-13 Home 0.8M → 2.3M steps 50k → 200k): Continue Watching's per-show work (the profile's partially watched episode in a show is found by walking the show's episodes; the fixture's 79 started shows grow in size) and the recommendation rows' restriction filtering (`recommended`, `for_you`: 0.15M → 0.44M). The 512-show and 64-episode windows I added do not bind in the fixture. Needs a per-show "newest partial episode" key (or the rec pool restricted per class); not done.
- **Deep fact-order pages for a restricted viewer** still page by offset over the class (74k → 199k median). Fact keys on visibility classes were not added (decision in section 2).
- **Presence filters on near-universal fields** (`versions/durationMs is-missing`, `subtitleLanguage/actor/director/writer is-present`) still test each title: up to 3–7M steps at 200k when they match nothing. `is-present` is broad and capped; `is-missing` of a field every title has matches nothing and scans. A per-field "has any" fact would fix them; not done.
- **"Has no genre" on container pivots and the Genres grouping** falls back to the per-title test when the candidates exceed 20,000 (artists and albums rarely carry their own genre): 6.6M steps at 200k on `music/genres`.
- **Title-substring filter** (`title contains`) is still a per-title LIKE: it needs an n-gram index; not done.
- **Filtered/restricted group counts (A22)**: covered only through the faster filters above; no separate change to grouping counts.
- **No 1M runs** (as instructed), and the 50k → 200k ratios for writes are dominated by host load (section 7).
- **The counted-block trigger generator** (`blocks.py`, named in the baseline) does not exist in the repository; the new fact-order triggers were generated by substitution from the year triggers in a scratch script (not committed). A generator or a Go test that re-derives the trigger text from a template would make future changes safer.
- Native: only type-check and hook lint; Vega reads the same screens.

## Commits

portico-server (`server-cp2-reads`, branch `audit-implementation/cp2-reads`): `ab7d5fbe`, `466d08f7`, `9fafeb41`, `b73d4056`, `e1110acb`, `26d3aa76`, `12c76657`, `e82adaec`, `3626bfdd`, `6029dc72`, `c4cef832`, `24ae517a`, `0a9084c8`, `30b6f7ee`, `c84ac39e`, `074dd645`, `a152f396`, `ac5a9038`, `e897337c`, `5e742182`, `868d1b09`, `cd1eff49` (reworded WIP), merge `b8e52d10`, `e9c85646`.

portico-react-native (`native-cp2-reads`): `8db9e13` (list group for facet counts), `ca6aa01` (mirror), merges `6a47255`, `894f8a6` (mirror re-written from the merged server, lock committed).

Scratch: fixtures (all four databases), binaries and the Go cache deleted; the per-route measurement logs (7 MB, no credentials) are kept in `_program/scratch/cp2-reads/logs/` (`before-*`, `final-*`, `ingest-*`, `final-table.md`).
