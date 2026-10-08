# Portico Enrichment Dataset — Project Architecture

**Status:** First draft, planning baseline  
**Project:** `/Users/justin/Documents/Codex Projects/Portico Enrichment`  
**Last updated:** 2026-09-25

## Project summary

Build an auditable, compact, open dataset that adds descriptive tags and similar-title links to Portico titles. The first domain is screen works: films, television series, and anime films and series. The dataset is a transport artifact that Portico servers can import; it is not a runtime service and does not make recommendations on its own.

The pipeline maintains a broad **private build corpus** of qualifying Wikidata works for source matching and resolving similar-title references. The **published set** is narrower: it contains matchable tagged works and matchable identity-only works only when a published similar-title list references them. Works without a Portico-matchable provider ID can support resolution in the private corpus but are never published.

GPT-6 Luna performs source assessment and enrichment in bounded Codex sessions. There is no OpenAI API, Batch API, model API key, or automated model endpoint in this project. The first run is a small 1,000-title pilot; a full Wikidata dump is considered only after the pilot, quality review, and scale gate.

The Portico lead owns server-side download, signature verification, import, recommendation use, and user-facing attribution. Those changes are outside this project.

## Goals

- Produce deterministic, compact SQLite datasets, starting with one `screen` file.
- Support exact matching against provider IDs already held by Portico (TMDB, TVDB, AniList, and IMDb fallback).
- Give Portico a controlled vocabulary for mood, tone, theme, pacing, style, setting, narrative, audience, intensity, and screen-specific tags.
- Preserve source provenance and enough durable job state to resume, audit, and reproduce every batch.
- Keep the full working corpus private while publishing only useful matchable rows.
- Measure quality, coverage, throughput, review effort, resolution, and storage before scaling.

## Scope

### In scope

- Screen domain: movies, television series, and anime (including series and films).
- Wikidata-derived candidates and provider identifiers; Wikipedia source text, eligible Fandom fallback for anime/TV, and Luna model knowledge for confidently identified works when available facts and article text are insufficient to support a tag.
- Vocabulary drafting, review, versioning, tagging, similar-title generation and resolution, validation, and SQLite build tooling.
- A local resumable ledger for candidate status, source revisions, decisions, outputs, failures, and retries.
- Small pilot, quality reporting, later scale planning, deterministic artifact production, manifest/signing tooling, and documented release preparation.
- Keep the current 105 MB canary cache on local storage because SSD access is not needed yet. The verified encrypted APFS SSD is available for this work; if a larger pilot cache needs it, use a dedicated writable folder without reformatting, preserve the possible whole Codex Projects migration, and keep at least 50 GiB free.

### Out of scope

- Changes to the Portico application or server, production imports, recommendation ranking, UI, personalization, or live user data.
- Music, books, albums, artists, audiobooks, and their enrichment runs. The schema may reserve support for future domains as specified by the lead plan.
- Fetching a full Wikidata dump before the pilot gates pass.
- Using any OpenAI API, Batch API, third-party model API, paid model endpoint, or model API key.
- Using TMDB, TVDB, AniList, IMDb, MyAnimeList, Letterboxd, Rotten Tomatoes, Metacritic, Trakt, other commercial title sites, or general web search as content sources or validation. Provider IDs are allowed only when sourced from Wikidata.
- Publishing copied article prose, quotations, summaries, or source packets.
- Publishing the dataset, signing with the real private key, pushing the repository, or creating a public release before the G4 decision and Justin's signing/publishing action.

## Users and operating model

- **Justin:** approves the vocabulary at G1, reviews the pilot and scale proposal at G2/G3, and controls the real signing key and publication decision at G4.
- **Codex with GPT-6 Luna:** performs bounded source assessment and enrichment in interactive Codex sessions. All work is linked to a versioned instruction set and source revisions. Luna is the enrichment worker; the project lead owns planning, schema stewardship, gates, review, and reporting.
- **Portico lead:** consumes a published artifact later, in a separate application task.

Sessions should be restartable. Every batch must be small enough to review, validated before checkpointing, and recorded so work can continue in a new session without losing decisions or repeating accepted work. Session throughput is measured from actual elapsed time; API-token and API-cost metrics do not apply and must not be invented.

## Scope and design direction

This is a headless data product. It has no UI. Human review uses readable Markdown reports and inspectable CSV/SQLite extracts where useful; those are project tools, not a product interface. Any future review UI would require a separate scope and architecture decision.

Use the compact, plain SQLite transport schema in the Portico lead plan as the contract. Keep one file per domain, integer QID keys, normalized provider and vocabulary tables, no repeated tag strings in work rows, no additional published indexes, stable ordering, and `PRAGMA user_version = 1`. The `screen` artifact should scale comfortably below 1 GB; the pilot measures actual bytes per work before scale estimates are trusted.

Tags are descriptive recommendation features only. They do not independently produce a ranked list, assert objective quality, or encode a user's preferences. Portico decides if and how to use them in its own recommendation logic.

## Backend and data pipeline

The implementation is a local Python 3.12+ pipeline using the standard library and `requests`. Use gzip -9 for artifact compression to match the Portico server's Go standard library. Keep a single Makefile and deterministic build inputs, including a stable `built_at` value supplied as a build input. Detailed source and stage rules are in [Data Pipeline and Dataset Contract](Data%20Pipeline%20and%20Dataset%20Contract.md).

At a high level:

1. **Candidate discovery:** query Wikidata in bounded, paginated, single-query batches; retrieve entity details in batches. Keep the broad private build corpus separate from the publishable set.
2. **Source matching:** use the Wikidata sitelink first (English first, otherwise the longest usable article); search only approved Wikimedia APIs and licensed Fandom wikis when a sitelink is absent. Pin each source revision and record match evidence. Missing Wikipedia text does not automatically make a work untaggable.
3. **Evidence packet:** form a bounded, revision-pinned packet from allowed facts and source passages where available. Cache it locally for resumption; never include it in the published dataset.
4. **Luna enrichment:** provide the versioned vocabulary, facts, and packet in a Codex session. For confidently identified works, Luna may use model knowledge when the available facts and article text do not adequately support a tag, even if an article exists. Record private per-tag evidence as `facts`, `source_text`, or `model_knowledge`; record `no_source` as the decision basis only when the work has zero supported tags. Model-knowledge-derived publication is capped at medium confidence and requires a recorded Wikidata revision. Require strict structured output and omit unsupported tags or low-confidence matches.
5. **Resolution and validation:** resolve suggested similar titles against the private corpus, retain only targets with matchable IDs, apply schema/vocabulary/applicability/sanity checks, and record review samples and failures.
6. **Publication selection:** include tagged works that meet publishability rules plus identity-only works referenced by published similar lists. Exclude unreferenced identity-only rows, unmatchable works, and low-confidence results.
7. **Artifact build:** sort rows, create the SQLite file deterministically with a stable `built_at` input, run integrity checks, gzip -9 the file, calculate hashes and sizes, and prepare manifest bytes plus a separate `.sig` signature file. Real signing and public release remain at G4.

The long-term update model may use full builds and deltas as defined by the lead plan. A delta replaces complete rows for changed works and carries removals and QID redirects. Updates are not part of the initial pilot.

## Security and provenance

Security and licensing constraints are first-order architecture requirements. See [Security and Provenance](Security%20and%20Provenance.md) for the operational controls.

- No API keys or credentials are needed for Luna. Never request or use `OPENAI_API_KEY`.
- Never place a secret, contact address, signing key, or private source cache in Git. `CONTACT_EMAIL` is supplied locally only for the Wikimedia User-Agent.
- The real dataset signing key belongs to Justin and is never read by the pipeline. Signing-tool tests use only a throwaway key created in a temporary directory.
- Allowlisted sources only: Wikidata (CC0), Wikipedia (CC BY-SA 4.0), and Fandom only after the individual wiki's footer confirms a compatible CC BY-SA license. Commercial provider IDs may appear only when recorded in Wikidata.
- Keep article text and prompts out of published files and Git. Publish tags, identifiers, source revision references, attribution, and other schema fields only.
- Retain per-work source URLs/revisions and a `NOTICE` with required contributor/source credit and AI-generation disclosure.
- Respect API etiquette, concurrency limits, `maxlag`, backoff, retry headers, and one SPARQL request at a time.

## Platform, storage, and deployment

- Development and enrichment run locally in the separate repository at `/Users/justin/Documents/Codex Projects/Portico Enrichment`.
- The external SSD is verified encrypted APFS and is available for this work while preserving the possible whole Codex Projects migration. The current 105 MB canary cache stays local because it does not need SSD access. If a larger pilot cache uses the SSD, place it in a dedicated writable folder, do not reformat, and maintain the 50 GiB free-space floor.
- Initial pilot acquisition must be limited to the sample's necessary Wikidata/Wikipedia/Fandom API requests. It does not require or authorize a full dump.
- No full dump is being acquired now. If a later scale decision makes one useful, record the storage plan and source checksum before acquisition; do not place it in Git or the Portico repository. SSD use for such data must preserve the same migration plan and 50 GiB free-space floor.
- The artifact is portable plain SQLite compressed with gzip -9. No runtime service, cloud deployment, or database credentials are needed to build it.
- Public hosting is a future release action. The lead plan names GitHub Releases as a target, but the repository/publication location remains for Justin to decide at G4.
- Do not push, force, rebase, or hard-reset. Keep project-only changes inside this repository; never edit the Portico repository as part of this task.

## Pilot and acceptance gates

### G1 — Vocabulary approval

Draft 150–220 screen-relevant tags with stable IDs/names, clear spoiler-free definitions, applicable work kinds, and strength semantics. Include intensity/audience dimensions. Future music/book vocabulary may be reserved only if explicitly marked unused. Justin approves the vocabulary before tagging the pilot.

### G2 — 1,000-title pilot

Use a mixed sample of films, series, anime, popularity levels, and source languages. No full dump. Validate the pipeline from candidate selection through report. Acceptance evidence includes:

- Candidate/source/tagging counts and coverage; tags supported by facts, source text, and model knowledge, plus works with no usable evidence.
- Similar-title resolution rate and examples of unresolved candidates.
- Number of distinct identity-only targets, including how many are below the tagging threshold.
- Independent 2% Luna re-tag agreement using Jaccard, with 0.6 as the target, plus a documented 200-random/100-famous review where sample size permits within this pilot protocol.
- Twenty readable sample records, quality/error patterns, vocabulary issues, elapsed time, titles per session/hour, and human review effort.
- Raw/compressed bytes, bytes per work, schema/integrity results, and a reasoned scale estimate.
- Durable batch ledger and source revision provenance sufficient to reproduce and audit decisions.

Report missing measurements honestly. The 1,000-title gate must produce a reviewable `REPORT-G2.md` and a specific recommendation to adjust or approve vocabulary, instructions, batch size, and the next tranche. No broad enrichment begins until Justin approves the next stage.

### G3 — Scale approval

Present a scale plan grounded in pilot throughput, quality, review burden, coverage, bytes per work, and source/API behavior. Set tranche size and stop conditions. Decide whether a full dump is warranted; if so, make a storage plan before acquisition that preserves the possible Codex Projects migration and 50 GiB free-space floor. Process the next tranche only at the approved size; pause on repeated integrity or source/API failures and report them.

### G4 — Sign and publish

Build and validate release artifacts, manifest, attribution, hashes, and compression. Justin reviews the concrete result, uses the real key to sign, and decides whether and where to publish. No real-key signing or public release is performed by the pipeline author.

## Known risks and open decisions

- **Licensing interpretation:** Wikidata identifiers and facts are CC0; tags informed by Wikipedia/Fandom and permitted model knowledge are intended to be published under CC BY-SA 4.0 with attribution and no copied prose. Confirm that this treatment and the exact output license are acceptable before public release; provenance does not itself settle legal interpretation.
- **Fandom variability:** licenses and content vary by wiki. Each candidate wiki needs a recorded footer/license check; unknown or incompatible terms mean skip it.
- **Source ambiguity and coverage:** title collisions and sparse data can still limit coverage. Missing articles do not automatically force an untagged outcome; model-knowledge fallback is allowed only for confidently identified works, has a medium-confidence publication ceiling, and requires a Wikidata revision in provenance. Match evidence must use year (±1) plus a Wikidata director/creator or country agreement where searching is needed.
- **Model consistency:** session-to-session variation can affect tags and similar lists. Version prompts/vocabulary, use strict validation, independent re-tagging, human review, and stop/adjust gates.
- **Pilot review capacity:** the plan's 200 random plus 100 famous manual review exceeds a 1,000-title pilot if 2% re-tag is separately counted; scheduling and review selection need to be reported clearly, and no sample should be represented as statistically stronger than it is.
- **Date mismatch in source plan:** the provided plan says it was authored/revised 2026-09-26, while this project context is 2026-09-25. Treat its schema/source rules as the supplied requirements, but confirm the plan version/date before relying on any later edits or change history.
- **SSD use:** the SSD is available for this project. Keep the 105 MB canary local for now; use a dedicated folder on the verified encrypted APFS volume if a larger pilot cache needs it, without reformatting and while preserving 50 GiB free for the possible Codex Projects migration.
- **Provider property verification:** confirm Wikidata property meanings, including a TVDB movie property if any, during implementation before collecting those IDs.
- **File size:** the ~400 bytes/work and ~60 MB raw targets are estimates. Measure actual pilot output and revise scale expectations from evidence.

## Source of truth

The lead plan at `Portico/Project Architecture/Remediation/Title Enrichment Dataset — Plan (Codex).md` is the detailed requirements source for the SQLite schema, allowed sources, request etiquette, and gates. This architecture draft records the project interpretation and user decisions for the separate repository. If the contract and this summary diverge, stop and resolve the discrepancy before implementation.
