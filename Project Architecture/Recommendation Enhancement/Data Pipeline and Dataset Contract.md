# Data Pipeline and Dataset Contract

**Status:** First draft, planning baseline  
**Applies to:** Portico Enrichment Dataset, screen domain first

## Contract

The published artifact is a compact, plain SQLite transport file. The detailed SQL schema in the Portico lead plan is authoritative; do not alter table or field semantics without coordination with the Portico lead. The schema is domain-ready (`screen`, with future `music` and `books` values), versioned, and designed for hundreds of thousands of works under 1 GB per domain.

The file contains work identity, provider IDs as Wikidata records them, typed vocabulary tags with strengths 1–3, up to 15 similar-title references, source site/revision provenance, and metadata. It contains no copied prose. Use stable integer QID keys, integer vocabulary/provider keys, `STRICT` tables as defined, no additional published indexes, deterministic ordering, and `PRAGMA user_version = 1`.

`tagged = 0` means an identity-only row retained solely because at least one published similar list references it. Such rows have no tags and null confidence. A tagged row has confidence 1 (medium) or 2 (high); low-confidence works are omitted. Every similar target must be present in `works`, in the same domain, not self-referential, and have an ID Portico can match.

## Corpus boundaries

### Private build corpus

Retain all qualifying screen works with sufficient Wikidata identity evidence (class, label, plus year or director/creator/country), regardless of whether a provider ID exists. Use it to resolve similar titles and match source articles. Keep this data and its intermediates private and out of release artifacts.

### Published candidates

A published tagged work must:

1. Be part of the qualifying build corpus.
2. Have at least one Portico-matchable provider ID: TMDB, TVDB, AniList, or IMDb.
3. Meet the kind-specific sitelink threshold selected to approach the lead plan's population targets, with actual counts reported.
4. Have at least one supported tag and non-low confidence after validation. Source text is useful but not mandatory when Luna can confidently identify the work and the available facts and article text do not adequately support a tag; model knowledge may fill that gap under the evidence rules below.

MyAnimeList-only works are not matchable by Portico and are excluded. A work with no matchable ID may be used privately for resolution but is never published. A matchable work without sufficient evidence to tag remains private unless it is referenced as a similar-title target.

### Identity-only targets

After published similar lists have been resolved, include an otherwise non-published matchable work with `tagged = 0` only if at least one published list points to it. Report distinct identity-only targets and the count below the tagging sitelink threshold at G2. Do not publish other untagged works.

## Allowed source inputs and evidence

| Input | Permitted use | Constraint |
|---|---|---|
| Wikidata / Wikidata SPARQL / QLever mirror / EntityData | Candidate classes, labels, IDs, year, directors/creators, genres, countries, language, content ratings, sitelink popularity, revisions | CC0; provider IDs only as recorded in Wikidata; verify property meanings before use; EntityData is the current entity-retrieval fallback when `wbgetentities` hits `maxlag` |
| Wikipedia API, any language | Article lead and relevant premise/plot, themes, production, reception text | CC BY-SA 4.0; English first, otherwise longest usable article; pin revision and retain attribution |
| Fandom API | Fallback for anime and TV without a usable Wikipedia article in any language | Check and record each wiki's footer license; skip unknown or incompatible licenses |

Forbidden: TMDB, TVDB, AniList, IMDb, MyAnimeList, Letterboxd, Rotten Tomatoes, Metacritic, Trakt, other commercial sites, and general web search. Do not use these sites for source text, validation, or title matching. Their identifiers may be included only if Wikidata supplies them.

If a candidate has no suitable Wikipedia sitelink, search only Wikipedia's API and, if the type and license rules permit it, Fandom's API. Accept an article match only when the year is within ±1 and a director/creator or country agrees with Wikidata. For confidently identified works, Luna may use model knowledge when available Wikidata facts and article text do not adequately support a tag, whether or not an article exists. If no tags are supported by any permitted basis, record `no_source` as the decision basis and emit zero tags; the work is not publishable as a tagged work. Record source URL, site/language, revision ID, and match decision where article text is used. Record the relevant Wikidata revision for every model-knowledge-derived tag; use EntityData as the retrieval fallback when `wbgetentities` hits `maxlag`.

For each accepted article, prepare a bounded packet (lead plus relevant premise/plot, themes, production, and reception sections; approximately 12,000 characters maximum). The packet is private cache material, pinned to the source revision, and is not published or committed.

## Per-tag evidence and Luna session workflow

- Run GPT-6 Luna in Codex sessions. Do not call a model API or request a model API key.
- Before G1 approval, draft the vocabulary and seek approval; do not use an unapproved vocabulary to enrich the pilot.
- Use a versioned prompt prefix containing the complete approved vocabulary and per-title Wikidata facts plus the allowed source packet. Keep each output tied to the QID and source revision(s).
- In private job state, record one evidence category for each emitted tag: `facts` (structured Wikidata facts), `source_text` (revision-pinned Wikipedia or eligible Fandom text), or `model_knowledge` (Luna's knowledge for a confidently identified work when facts/article text are insufficient). Record `no_source` as a work-level decision basis only when no tags are supported; that outcome has zero tags. These evidence labels and decision details are private audit data and are not added to the published SQLite schema unless the lead approves a schema change.
- A work tagged from model knowledge must be confidently identified. Any published model-knowledge-derived tag is capped at medium confidence, and its evidence record must include the Wikidata revision used to establish identity. High-confidence publication requires support from `facts` or `source_text` evidence.
- Require the lead plan's strict output shape: `match` (`high`, `medium`, `low`), tags from the approved vocabulary with strengths 1–3, and suggested similar titles with title/year/kind.
- Tag only source-supported or widely known characteristics; prefer omission. Use 6–20 tags on a tagged result, obey `applies_to`, avoid spoilers, and set low when identity is doubtful. Low results are not published.
- Similarity means kinship of tone and theme, not genre alone. Suggest no more than 15 titles; anime may refer to anime or live action as allowed by the plan.
- Determine practical batch size in the pilot. Validate and durably checkpoint each batch before starting another session. Do not claim API token or cost figures.

Suggested title resolution order: normalized title + year ±1 + kind against the private candidate corpus; follow Wikidata redirects; if still unresolved, use a Wikidata label search limited to candidate classes. Drop unresolved results and report resolution rate. Keep a resolution trace so reviewers can see why a candidate matched or was dropped.

## Validation and release build

Before a batch is accepted:

- Validate strict JSON shape, allowed vocabulary membership, tag-kind applicability, strength bounds, and the 6–20 tag rule for tagged results.
- Validate self-link exclusion, target resolution, same-domain membership, and target matchability.
- Treat tags as recommendation features only. Do not use content ratings or tags to enforce age restrictions; age restrictions are handled elsewhere by Portico.
- Record rejected rows, repair/retry decisions, and final decisions in the durable ledger.
- Independently re-tag a random 2% in separate Luna sessions; calculate tag Jaccard agreement and compare with the 0.6 pilot target. Keep the independent outputs and sampling seed/selection auditable.
- Complete the specified review of 200 random and 100 famous titles; record the sampling approach, reviewer effort, errors, and vocabulary issues. If overlap occurs with the independent sample, report it rather than double-counting work.

For the artifact, sort rows consistently, set the schema page size, and supply `built_at` as a stable input so repeated builds from the same inputs and build metadata produce identical SQLite bytes. Run `VACUUM` and `PRAGMA integrity_check`; do not run `ANALYZE` (there are no indexes to analyze). Compress with gzip -9 only, matching the Portico server's Go standard library. Record SQLite and compressed bytes, bytes per work, SHA-256, row counts, schema/vocabulary versions, and stable build timestamp. Write the exact manifest bytes as a standalone file and place the Ed25519 signature over those exact bytes in a separate `.sig` file; do not embed the signature in the manifest.

Real signing is a Justin-controlled G4 action. Any signing tool test uses a throwaway key generated in a temporary directory and must not access or ask for the real key.

## Durable operational records

Keep a resumable, local ledger with at least:

- Run ID, stage, batch ID, QID, kind, published-candidate status, and selection threshold/version.
- Candidate facts needed to explain selection, with their Wikidata source/revision as available.
- Source URL, host/site/language, revision ID, license check for Fandom, match status, and match rationale; record the Wikidata revision used to establish identity for model-knowledge evidence.
- Packet fingerprint and prompt/vocabulary version; keep packet text itself in ignored cache storage only.
- Per-tag evidence category (`facts`, `source_text`, or `model_knowledge`) and private support/rationale; record work-level `no_source` only for zero-tag outcomes. The published schema remains unchanged.
- Luna session/batch association, raw structured output, normalized output, validation result, reviewer disposition, and error/retry history.
- Similar-title suggestions and resolution trace, publication/reference status, and output row identity.
- Build artifact checksum, byte counts, and relevant report links.

Avoid secrets, personal email, and source article text in normal logs. Store contact data only in the local environment configuration used to form the User-Agent. Caches and raw source packets must be excluded from Git and release builds.

## Request etiquette

- Send `User-Agent: PorticoEnrichment/0.1 (https://getportico.tv; $CONTACT_EMAIL)`. Set `CONTACT_EMAIL` locally; never hard-code or commit it.
- Maximum four concurrent requests per host; use `maxlag=5` on MediaWiki action API.
- Run one SPARQL request at a time. Paginate and split by class/year to avoid timeouts.
- Honor 429/503 and `Retry-After`; back off rather than retrying aggressively.
- Retrieve Wikidata entities in batches of 50 with `wbgetentities` where it succeeds; when it hits `maxlag`, use Wikidata EntityData as the current fallback and retain revision metadata. Keep the current 105 MB canary cache on local storage; if a larger pilot cache needs the SSD, use a dedicated writable folder on the verified encrypted APFS volume, do not reformat, and preserve the 50 GiB free-space floor. Downloaded text never enters the dataset.

## Gate deliverables

- **G1:** approved vocabulary file, version, applicability definitions, and recorded decision.
- **G2:** `REPORT-G2.md` with the full pilot metrics, review findings, 20 sample titles, identity-only target counts, disk measurements, errors, and recommendation for the next tranche.
- **G3:** `REPORT-G3.md` / scale plan with tranche size, throughput and quality basis, storage estimate, cache/dump location plan, stop conditions, and explicit approval record before any full dump or larger tranche; preserve the SSD migration plan and 50 GiB free-space floor.
- **G4:** release candidate, validation report, attribution/NOTICE, manifest and hashes, throwaway-key tool verification, and handoff for Justin's real signing and publishing decision.
