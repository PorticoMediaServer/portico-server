# Recommendations

October programme status: recommendation design below is retained. Integer 0–10 personal ratings, separate progress counters, readable catalogue generations and privacy-aware shared popularity now govern its inputs. Dataset publication, independent row freshness hints and real-device presentation remain unverified or unfinished in [Backlog](Backlog.md). Sources: [search/sort](../../_program/reports/search-sort.md), [personal state](../../_program/reports/personal-state.md), [freshness clients](../../_program/reports/freshness-clients.md). This is not a new capacity or deployment claim.

Personal Home rows, personal Discover rows in every library, title pages that know what else you'll like, a For you sort in browse, and saved views on Home. The goal is Plex's shape with much better results. A household server has only a handful of viewers, so the engine models each profile's taste carefully and leans on the crowd signal open metadata already carries.

Constraints the design keeps:

- **Fresh.** A watch or a library change shows on the next load. No time-based caches.
- **Cheap.** Library-wide work happens when the library changes. A page load does bounded, indexed work, independent of library size.
- **Complete.** A row shows the top of a ranking; every "See all" is a complete, correct list.
- **Private.** Kids restrictions apply at retrieval. Signals from other people come only from members who opted in, and only when the owner allows community activity.
- **Zero setup.** Every data source ships ready, is free, and allows non-commercial use.

## Three layers

1. **Library side, maintained when the library changes** (catalogue domain 32, `compactcatalog/derive_rec_facets.go`):
   - Each work's **features**: genres, keywords and tags, studio, network, decade, language, credited people (the first five billed carry an extra `p5:` feature), directors, creators, franchise and series, plus its members' facets (a show's episodes, an album's songs; people count when they appear in at least three members). Dataset tags appear as `x:<tag>`, and strength-3 tags also as `x3:<tag>`.
   - **Best-first postings** `catalog_rec_postings(facet, library_id, quality, entity_id)`: every feature's works, ordered by quality, so retrieval is an index seek.
   - **Rarity** `catalog_rec_df`, adjusted by difference per batch.
   - **Quality:** a Bayesian vote-weighted rating (m = 100, C = 6.3; unrated works get 6.0), so a 9.1 from 12 votes doesn't beat an 8.4 from 40,000.
   - **Similar titles** `catalog_similar`: TMDB and AniList recommendation lists and dataset lists, stored by the target's provider id and resolved through `catalog_external_ids` when read. A title's arrival never rewrites another title's links.
2. **Profile side, maintained when the profile acts** (`compactcatalog/rec_profile.go`):
   - Triggers on favorites, ratings, likes, watchlist, history, progress and not-interested flags queue `rec_profile_jobs` in the same transaction. The worker's `StepRecProfiles` applies each change exactly, so retractions and re-faceting stay exact.
   - Each work's contribution is a **signal** (weight, engaged, hidden, finished).
   - **Taste** is kept at two horizons (half-lives of 182.5 and 14 days), stored scaled to a per-profile epoch that is rebased when it falls more than 1,500 days behind.
3. **Request time** (`catalog/rec_engine.go`):
   - A **session** loads the profile's taste. Pending jobs are overlaid read-only with the worker's own computation, so a Not interested counts on the very next read.
   - **Retrieval** seeks the postings of the profile's strongest features plus the similar titles of recent favorites and finishes.
   - **Rescoring** of the best few hundred: TF-IDF over typed feature weights, normalized by item mass and taste norm, plus 0.35 × similarity and 0.06 × (quality − 6.3), plus a freshness bonus. A taste match is required except at cold start.
   - **Eligibility:** available (a container through any available member), not watched, not in progress, not disliked or not interested, not abandoned, within restrictions, and not poorly rated (quality under 5.5 from 50 or more votes).
   - **Diversity,** per page of 12: at most two per franchise or series and three per director, creator, artist or author, with repeat penalties. One card in eight within the first 48 places explores lower down, among candidates scoring at least 60% of the best. The pass is bounded (the first 60 places, each chosen among the next 100 candidates).
   - **Memo** under every revision the result read: catalogue, viewer, taste, `rec_data_revision`, restriction fence, libraries, day.

Abandonment (started, under 25% watched, untouched for 21 days) counts mildly against what a title is like. It's decided at read time, and a work the profile otherwise enjoyed isn't counted against.

## Surfaces

**Home**
- Recommended (all libraries, both horizons).
- **The personal rows**, one arrangeable family (listed as "More recommendations" in Customize Home) that expands into up to five rows, strongest first, rotating daily among close ones:
  - genre, person (actor, director, creator, artist, author), mood or theme (dataset tags)
  - Next in the franchise, Next in the series
  - Hidden gems, Top rated, Rediscover (there is no "New for you" row: Home's Recently added shelf says what is new)
- A title appears in at most one personal row. The family's full rankings are deduplicated against each other and against Recommended's first page; franchise and series rows are exempt because they're precise.
- A row needs 6 titles, except franchise and series rows (2) and Rediscover (3).
- Saved views can be pinned to Home as rows (layout ids `view:<id>`).
- **Default Home order** (2 Oct 2026): Continue Watching, Continue Listening, Recently added (one shelf per library, `recent_<library>`), Favorites, Recommended for you, the personal rows, Trending now, Popular on this server. The per-library "Recently added in …" rows exist but are off until a profile places them in Customize Home. There is no "Featured" row: Home's hero is what the viewer was last in the middle of (see [Catalogue](03%20Catalogue%20and%20Metadata.md), Home).

**Library › Discover:** the same generators scoped to the library, plus Short picks (films under 100 minutes), next book in a started series, and more from the artists and authors the profile plays.

**Title pages:** More like *X* (provider similar titles, then content similarity, ranked by the viewer's taste), Starring *top-billed actor*, From *creator*, the collection row, and Viewers also watched. That last row comes from members sharing their activity who finished *X* (the 50 most recent, at least three) and then their recent finishes.

**Browse:** the **For you** sort ranks any pivot and filter by the profile's taste. Filtered sets of up to 2,000 are ranked whole, and larger ones through the engine's candidates. Ranked, unwatched titles come first, then the rest in title order. It's descending only, sorts on its own, and has no position anchors. Its browse revision includes the taste revision, pending jobs, `rec_data_revision` and the day, so a cursor never crosses a reordering. A personal row's **See all** opens browse with that row's filter, sorted For you, and exists only where a browse filter equals the row (genre, actor, director, artist, author, Short picks, New for you).

**Feedback:** Not interested on every recommendation card, with Undo. Reset recommendations in Privacy zeroes the learned weights but keeps exclusions and history.

## API

- Home rows carry `family` and a parameterised `titleText`. Dynamic row ids take the form `for_you:genre:<key>` or `for_you:person:<provider>:<id>`. Paging a dynamic row recomputes it; its cursor carries the revisions, and a change answers `stale_continuation`.
- Title-page relations: `more_like`, `starring`, `creator`, `viewers_also_watched`.
- `notInterested` on the personal batch endpoint; `POST /v1/me/recommendations:reset`.
- The `forYou` browse sort; content sections' `seeAll {pivot, query, sort}`.

## Title enrichment dataset

An open CC BY-SA 4.0 dataset of mood, tone, theme, pacing, style, audience and intensity tags (strengths 1–3) and similar-title lists, built separately in `portico-recommendation-enhancement` from Wikidata (CC0) and Wikipedia and Fandom (CC BY-SA) only. It never uses TMDB, TVDB or AniList content. That repository's plan and schema are the contract.

The server's side is `internal/enrichment`:

- A daily check fetches a signed manifest (Ed25519 over `manifest.json`'s exact bytes, in a detached `.sig`) and the full or delta gzip files for the domains this server has libraries for. Each file is SHA-256 checked and capped at 2 GiB expanded.
- Library works match dataset works by exact provider id (TMDB film or TV, TVDB series, AniList, TVDB film, IMDb), never by title and year. Works gaining ids are matched as they arrive, and works that lose them are reconciled.
- Tags become features; similar titles become `catalog_similar` rows with source `portico-dataset`.
- It's **off until the first release is published**: `enrichment.PublishedManifest` and `PublishedPublicKey` are empty (environment overrides `PORTICO_DATASET_MANIFEST` and `PORTICO_DATASET_PUBLIC_KEY` exist for testing). Without it, every row except mood and theme works the same.

## Measured

On synthetic taste catalogues (`catalog/rec_scale_test.go`, `-recscale=N`):

| | 10k films (Mac) | 100k films (runner) |
|---|---|---|
| Recommended, cold / memoised | ~15 ms / 0.3 ms | 80 ms / 0.5 ms* |
| Personal rows, cold | 155 ms | 280 ms* |
| Home, cold / memoised | 259 ms / 30 ms | 607 ms / 170 ms* |

\* Measured before the bounded diversity pass and the artwork-resolution fix; both lowered these numbers. Memoised Home is flat with library size (25 ms at 1k films, 30 ms at 10k).
