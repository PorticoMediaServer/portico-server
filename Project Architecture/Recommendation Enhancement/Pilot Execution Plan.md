# Title enrichment pilot execution plan

Status: working plan for the 1,000-title local pilot. The lead brief remains the dataset contract. The vocabulary in `vocabulary/tags.json` is version 1, draft; the pilot may expose changes needed before a wider build.

## Pilot question

Can a reproducible Wikidata sample, permitted revision-pinned source packets, and bounded Luna sessions produce useful recommendation tags and matchable similar-title links at a rate and quality that justify scaling?

## Sequence

1. **Foundation.** Establish the separate repository, code/data licenses, ignored local cache, private work ledger, exact transport schema, and validation tests. No release or Portico server integration is in scope for this pilot.
2. **Candidate canary.** Run a small, frozen Wikidata acquisition and inspect class assignment, provider IDs, duplicates, languages, facts, rate limits, and query failures. Record query text and acquisition time. Correct the intake before selecting 1,000 titles.
3. **Frozen pilot sample.** Select 500 films, 250 non-anime series, and 250 unique anime works where the source pool supports it. Stratify each kind by sitelink count and source language availability. Include low-popularity and non-English works. Keep unmatchable works in the private build corpus; tag only matchable eligible works. Preserve the seed, candidate pool, query text, and exact QIDs.
4. **Source canary.** Retrieve a bounded set of Wikipedia pages through official APIs with the required contact User-Agent, `maxlag=5`, sequential or at most four concurrent requests per host, and retry/backoff. Pin each source revision. Measure article sizes and the request cost of finding the longest non-English article. Skip a title when identity evidence is insufficient. Fandom remains a later fallback after wiki-specific license verification.
5. **Luna canary.** Give Luna small packet batches from the frozen sample, the versioned vocabulary, and a strict output contract. Checkpoint each batch, validate outputs, and inspect the evidence behind tags. Luna may use stable, widely known knowledge when the supplied sources are insufficient for a confidently identified work. Record that basis and a concise private claim for each tag; cap published confidence at medium when model knowledge is used. Increase batch size only after errors and review time are known.
6. **Complete the 1,000-title attempt.** Process eligible titles in bounded sessions. Record decisions based on Wikidata facts, source text, model knowledge, or no usable evidence, plus failures, retries, elapsed time, and titles per session/hour. Resolve similar suggestions against the private corpus by identity evidence; retain only matchable targets. Include identity-only target rows only when a published similar list references them.
7. **Quality and artifact.** Independently re-tag a seeded 2% sample; calculate Jaccard tag agreement. Review a seeded 200-title random sample plus 100 prominent titles against allowed sources, recording overlap and reviewer time. Build a deterministic `screen` SQLite file, verify its schema and integrity, compress it, and measure bytes per work. Produce 20 readable examples and a concise G2 report.

## Stop conditions

- Repeated source/API errors, unexpected rate limiting, or ambiguous identity matches: pause acquisition and investigate before retrying in bulk.
- Source text is not clearly from an allowed, revision-pinned site: omit that source and do not infer unsupported tags.
- Validation or referential integrity failure: stop artifact generation until the offending batch is corrected.
- Low-confidence title match: keep the record private and untagged; do not publish it.

## Scope and storage

The source/article cache, candidate pool, batch packets, and model outputs are local build data and remain outside Git. The external NVMe is the intended larger temporary data location. Its mount identity has been checked, but its root is currently writable only by root; until a scoped writable folder is available, keep the small pilot cache in the project's ignored local `.cache/`. No full Wikidata dump is part of this pilot.

## Decisions after G2

Use observed coverage, tag quality, similar-link resolution, request behavior, throughput, review effort, and bytes per work to revise the vocabulary and select the next tranche. A dump-based pipeline and weekly delta design become concrete only after the pilot shows what they must solve.
