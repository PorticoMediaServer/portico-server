# Security and Provenance

**Status:** First draft, planning baseline  
**Applies to:** all candidate, source, enrichment, build, and release work

## Security boundaries

This project has no application accounts, server secrets, or model API. GPT-6 Luna is used interactively in Codex sessions. Do not request, store, or use `OPENAI_API_KEY` or another model API key. Do not send project material to an external model endpoint.

The repository is the code and documentation boundary. Keep downloaded source text, raw session packets, intermediate candidate data, and build intermediates in ignored local storage. Never place these materials in the Portico repository. The published dataset is a deliberate output boundary containing schema-approved identity, IDs, tags, similar QIDs, source revision pointers, and metadata; it must not contain article passages, copied summaries, prompts, or credentials.

The external SSD is verified encrypted APFS and available for this work while preserving the possible whole Codex Projects migration. The current 105 MB canary cache stays local because SSD access is not needed yet. A larger pilot cache may use a dedicated writable folder on the SSD without reformatting and while keeping at least 50 GiB free. A full dump is not being acquired now and may only be considered after a later scale decision.

## Credentials and signing

- `CONTACT_EMAIL` is supplied by Justin in the local environment for the required Wikimedia User-Agent. Never hard-code it, print it in reports, or commit environment files containing it.
- No real signing key is needed to develop or validate the pipeline. The dataset signing key belongs to Justin; the implementation must accept a caller-provided key path only for the G4 release action.
- Test signing behavior exclusively with a throwaway Ed25519 key generated under a temporary directory. Delete the throwaway key after the test and verify that no real key path was accessed.
- Never include private keys, credentials, local environment files, or key material in Git, artifacts, or logs. Ignore `*.key` and relevant local environment files.

## Source allowlist and licensing provenance

All source access must follow the lead plan:

- Wikidata is CC0 and may supply candidate facts and provider IDs.
- Wikipedia article text is CC BY-SA 4.0; record article site and revision. Attribute sources and contributors in the per-work provenance and `NOTICE`.
- Fandom is a fallback for anime/TV only when no Wikipedia article in any language is usable. Inspect and record the footer license of each wiki used. Skip any wiki with unknown or incompatible terms; preserve its recorded license in `fandom_wikis` and the attribution notice. For a confidently identified work, Luna model knowledge may support tags when available facts and article text are insufficient, even if an article exists, subject to the evidence and confidence cap below.
- Do not access restricted commercial databases or general web search for content or validation. Do not use outside provider ID endpoints. IDs are accepted only from Wikidata.

The intended dataset license is CC BY-SA 4.0, with source-specific attribution and an explicit disclosure that an AI model generated tags using recorded sources and, where applicable, model knowledge. No article sentences or quotations are retained in the output. Before a public release, Justin should decide whether the stated licensing treatment is acceptable; this architecture records the chosen direction but does not purport to provide a legal conclusion.

## Provenance and audit trail

Every published tag set must be traceable to:

1. A stable Wikidata QID and relevant source facts.
2. An accepted Wikipedia or eligible Fandom article, its site/language, revision ID, and URL reconstructed from that revision when source text supports the tag; for model-knowledge-supported tags, the Wikidata revision used to establish identity.
3. A private per-tag evidence category: `facts`, `source_text`, or `model_knowledge`. Record `no_source` as a work-level decision basis only when the work has zero supported tags. Evidence categories and rationales remain in the private ledger, not the published schema.
4. The vocabulary and prompt version used in a Luna session.
5. The batch/session record, normalized result, validation checks, and any human review disposition.

Luna may use model knowledge for a confidently identified work when available facts and article text are insufficient to support a tag, even if an article exists. Published tags relying on model knowledge are capped at medium confidence and require a recorded Wikidata revision. High-confidence publication requires `facts` or `source_text` support under this contract.

Candidate discovery should record the Wikidata query/build version and the entity facts needed to explain selection. Store source packet fingerprints and revision identifiers in durable state so identical source input can be recognized without placing full text in the published database. Preserve failures and retries rather than silently replacing earlier evidence.

The data model's `work_sources` records source kind, site, and revision. `fandom_wikis` stores per-wiki license. `NOTICE` credits Wikidata, Wikipedia contributors, and relevant Fandom contributors, and states that tags were AI-generated using those sources and, where applicable, model knowledge. Source URLs/revisions and the attribution text in `meta` should remain consistent with the notice.

## Request and cache controls

- Include the exact project User-Agent with locally provided contact email.
- Limit concurrency to four requests per host; MediaWiki API requests use `maxlag=5`; issue one SPARQL query at a time. If `wbgetentities` hits `maxlag`, retrieve the entity through Wikidata EntityData and preserve its revision metadata.
- Respect `429`, `503`, and `Retry-After`; use bounded backoff and durable retry status.
- Pin the revision before text extraction. Do not follow a mutable latest page after packet construction without recording the new revision and invalidating or rebuilding the packet.
- Keep the current 105 MB canary cache in local ignored storage because SSD access is not needed yet. A larger pilot cache may use a dedicated writable folder on the verified encrypted APFS SSD without reformatting; preserve the possible Codex Projects migration and keep 50 GiB free. Exclude caches from Git and release build inputs except for controlled packet generation. Release construction must explicitly allowlist intended tables/fields rather than copying cache files.
- Make caches removable and reconstructable from source references. Retention duration is not fixed yet; choose and document it before a scaled run.

## Integrity and release safeguards

- Keep the broad build corpus and published set separate at the data and build stages.
- Enforce referential checks, allowed source values, tag applicability, strength bounds, similar target membership, and the tagged/identity-only rules before build.
- Produce deterministic ordering and a reproducible checksum for identical inputs and versions.
- Run SQLite integrity checks and verify gzip -9 compression, artifact size, exact manifest bytes, detached `.sig`, and SHA-256 before release review. Supply stable `built_at` as build input.
- Sign the exact manifest bytes with Ed25519 and write the signature separately to `.sig`, only at G4, using Justin's key outside automated pilot processing. The public verification key and manifest URL are handed to the Portico lead after publication.
- Record checksums of downloaded dump inputs if a future approved full build uses one, and keep the dump outside Git and release output.

## Risks requiring active handling

- Fandom licenses can differ by community and may change; re-check at use time and retain evidence of the applicable footer/license.
- A source revision reference makes attribution auditable but does not establish that generated tags are legally unencumbered. Public release decision remains with Justin.
- Source matching can select a similarly named work. Require year and a director/creator or country cross-check where searching rather than following a Wikidata sitelink; low-confidence results remain unpublished.
- Luna may infer unsupported or spoiler-sensitive tags. Version instructions, prefer omission, validate automatically, inspect pilot samples, measure independent agreement, and adjust before scaling.
- Cached article text can be mistaken for publishable content. Separate directories, Git ignore rules, build allowlists, and release inspection must all preserve the boundary.
- SSD storage is available for this project; use only a dedicated writable folder if needed, do not reformat, and preserve the possible Codex Projects migration with at least 50 GiB free.
- Model knowledge can produce plausible but unsupported tags. Keep per-tag evidence private and auditable, require confident identity, cap model-derived publication at medium confidence, and measure errors in the pilot.
