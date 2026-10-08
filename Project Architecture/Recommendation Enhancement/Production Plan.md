# Production Plan: model-knowledge dataset

**Status:** Approved by Justin, 30 September 2026. Supersedes the source-grounded approach in `Pilot Execution Plan.md` and the Luna-only workflow in `Project Architecture.md`; the contract's schema, matching and release rules still apply except where this plan changes them.

**Evidence:** `trials/model-trial-1/REPORT.md` (Luna vs GPT 6 Sol vs Muse Spark, 59 titles) and `trials/model-trial-2` (1,000 titles, GPT 6.1 Sol).

## Principles

1. **Quality and reliability first; no manual corrections.** Every judgment a reviewer would make is an automatic check. Justin's only manual step is signing a release.
2. **The lead mostly waits.** A driver script owns the queue, the lanes and every check. The lead launches it, reads one progress file occasionally, and steps in only when the driver stops on something it can't handle. No check uses the lead's tokens.
3. **Model knowledge is the main source.** Wikidata supplies identity, ids and facts; GPT 6.1 Sol supplies tags and similar titles from its own knowledge; Wikipedia text is only for titles the model doesn't know.
4. **Ship weekly, improve continuously.** Each weekly version carries new releases, newly added titles and refinements together, as a delta.

## Models

- **GPT 6.1 Sol** (Codex lanes, `LANE_MODEL=gpt-6.1-sol LANE_EFFORT=high`) does all primary tagging. Its output is saved and shipped once it passes the automatic checks; no second model pass is required first.
- **GPT 6 Luna** runs later as a **refinement pass**, possibly much later. Luna never overwrites Sol. Where they agree, the tag's confidence rises. Where they strongly disagree, the title is re-run on Sol in a fresh lane, which decides. A Luna "yes" never overrides a Sol "no".
- **Lanes are fresh per chunk of about 100 titles.** Trial 2 showed that a long context makes Sol steadily more cautious: "yes" on popular titles fell from 53% to 31% over 425 rows and recovered to 46% in a fresh run. Codex also ends a turn at roughly 220k tokens (about 425 rows), so long lanes aren't possible anyway.

## Data

### Private build database (captures everything)

For every Wikidata film, series and anime film or series: all labels and aliases, original title and language, all release and start dates, country, top cast, directors and creators, genres, franchise and series membership, based-on, sitelink count, and provider ids (TMDB P4947/P4983, TVDB P4835/P12196, IMDb P345, AniList P8729). Bulk-fetched from the QLever Wikidata mirror. It stays private and may never ship.

### What the model sees per title

`ref`, `title`, `original_title` (only when different), `year`, `kind`, `anime`, `people` (director or creator), `cast` (top 3). No `qid`, provider ids or anything TMDB-branded: the model can't use them, and ids in the prompt could create false confidence. That adds roughly 5% to per-title cost.

### Candidate order

Works with at least one Portico-matchable id, most popular (sitelinks) first. Trial 2 found 951 of 953 sampled titles have one.

## Vocabulary and output

- `vocabulary/tags-v2.json`: 319 screen tags in 14 families, scored 0–3 per family (`vocabulary/v2-notes.md`).
- The brief's "yes" is reworded to "you could confidently describe this work to a friend"; caution is aimed at doubtful identity and obscure or post-training titles.
- For "yes", 10–15 similar titles.
- **New:** each similar title carries a one-word relation: `tone`, `theme`, `premise`, `style` or `creator`.
- The identity check is **structured** (director, lead cast, studio), so a script can compare it with Wikidata.
- `premise` and `check` text are private audit data and are never published.

## Automatic checks (the replacement for manual review)

Every chunk passes all of these before its rows are accepted. A failed chunk is re-run in a fresh lane; a chunk that fails twice is logged and skipped, never shipped.

| Check | Rule |
|---|---|
| Format and vocabulary | The existing checker; rows with made-up tags or out-of-range strengths are retried. |
| Made-up controls | Every chunk contains planted fictional titles. Any "yes" rejects the whole chunk. |
| Identity | The structured check (director, cast) must match Wikidata. A mismatch re-runs the row; a second mismatch skips the title. |
| Drift | Every chunk contains titles from the **reference set** (about 300 titles tagged by repeated Sol runs until stable). Agreement with their settled tags and the "yes" rate on popular titles must stay within bounds. |
| Similar titles | Each suggestion must resolve to a Wikidata work with the right year (±1) and kind, using aliases and original titles; unresolved suggestions are dropped. The build attaches provider ids; the model never produces ids. |

## Lanes

1. **Primary (Sol):** candidates by popularity, about 6 lanes in parallel.
2. **Wikipedia (Sol, separate lane, built early):** titles the primary lane answered "no" and that have an article. Article sections (lead, plot, reception minus review-site names) are **pre-fetched in bulk**, revision-pinned, and sent with the row. Lanes never browse. This is also the weekly new-release path, since recent titles are outside the model's knowledge.
3. **Refinement (Luna):** any time later, as above.

## Licence

**One CC0 database.** Tags, strengths and similar-title links are facts and ideas about works, not Wikipedia's expression, so they aren't "Adapted Material" under CC BY-SA. Three conditions keep that true:
1. No text leaves the pipeline.
2. Each tag's source (model, Wikidata facts, Wikipedia text) is recorded privately, so Wikipedia-derived rows could be split out later with one query if ever needed.
3. NOTICE credits Wikidata and Wikipedia anyway.

`tags-v2.json` and `DATA_LICENSE` move to CC0-1.0 at release.

## Release

- SQLite build to the existing transport schema, plus the similar-title relation column (a schema v2 coordinated with the server).
- Checks at release: integrity, deterministic bytes, and a coverage check against the demo library's real TMDB ids.
- Weekly deltas through the server's existing delta import. Work ids and tag rows are stable across versions, so refinements don't churn recommendations.
- A regression report against the reference set for every version.
- Justin signs the manifest.

## Before the full run: a dress rehearsal

About 2,000 titles through the whole automated pipeline: primary lanes, a deliberately corrupted chunk (must be rejected), controls, identity checks, the Wikipedia lane on the "no" rows, similar-title resolution, the build and the coverage check. If its report is clean, the full run starts.

## Server follow-ups (separate work, not this project)

- Add `subgenre` to the theme rows and `appeal` to the mood rows in `catalog/rec_generators.go`.
- Read the similar-title relation for "why" labels.
- Use intensity as a soft preference.
- Later: count early abandonment and repeatedly ignored suggestions as mild negatives; co-watching between a server's own users.
