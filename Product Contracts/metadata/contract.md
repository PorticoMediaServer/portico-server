# Metadata, Matching, Provenance, and Artwork Contract

> Current Portico product contract, reconciled 5 September 2026.


Contract ID: `PC-METADATA`  
Revision: `portico.reconciled.1`  
Status: **approved**  
Owners: Product, Server Metadata, Catalog, Security, Client Foundations  
Imports: `PC-FOUNDATION` permissions and resource lanes; `PC-GOVERNANCE` lifecycle/exception law; `PC-GLOSSARY` vocabulary  
Coordinates with: `PC-LIBRARY-SCANNING`, `PC-MEDIA-PRESENTATION`, `PC-ACCESSIBILITY-LANGUAGE`, `PC-SETTINGS-OPERATIONS`  
Applies to: Portico Server, bundled Server Web, Client Core, and every first-party client that displays or manages metadata

## Scope and non-goals

This contract owns descriptive metadata authority, evidence, matching, accepted identities, field provenance, language/region, people and relationships, artwork selection/storage, manual edits and locks, refresh, provider failure, attribution, and public projections.

It does not own technical media facts such as codecs, streams, chapters, bitrate, or seek evidence; scanner identity and playback capability; user watch state; provider commercial licensing decisions; or the visual layout of cards/detail pages. Those facts may be presented beside metadata but remain under their owning contracts.

TMDB is the built-in primary provider for movies and television. TheTVDB is a built-in selectable provider and default fallback for movies and television. Both work without ordinary owner key setup, subject to provider availability and licensing. Portico does not require Hosted Services as a metadata proxy.

Before the first remote metadata request for a new server, setup MUST explain that enabled providers receive bounded title/identity queries directly from that server and present the initial provider policy: TMDB primary, TheTVDB fallback. The owner can disable either provider or choose a different supported order before confirming; confirmation is durable and editable later in Server Console. Dismissing or abandoning the disclosure sends no provider query and setup resumes at the same decision.

There are no unresolved metadata product decisions in this revision. This pre-release contract has one revisioned evidence model; an implementation MUST NOT preserve a weaker legacy overwrite path alongside it.

## 1. Canonical evidence model

### 1.1 Canonical item and revision

Each media item has one server-owned opaque identity and one monotonically increasing `metadataRevision`. A metadata revision is a complete, independently projectable evidence snapshot, not an implicit delta that depends on stale rows.

Every applied revision MUST record:

- item ID, revision, base revision, state, trigger, actor, and timestamps;
- the source kind and provider, if any;
- every accepted/locked normalized field and relationship with provenance;
- accepted, candidate, rejected, and superseded provider identities;
- artwork choices and their provenance;
- explicit locks;
- an ETag/digest covering the canonical projected state;
- the operation outcome, including conflicts and no-op results.

A writer MUST compare the expected revision immediately before commit. A stale writer cannot publish over a newer manual edit, match, artwork selection, or provider refresh.

### 1.2 Source kinds

Canonical source kinds are:

- `manual` — an explicit owner/user edit or Identify decision;
- `nfo` — supported local NFO/OPF evidence;
- `embedded` — supported embedded media tags/art;
- `file` — external sidecar/artwork file evidence;
- `scanner` — filename/path/hierarchy inference;
- `provider` — remote metadata provider evidence;
- `system` — derived/repair/merge state that cites its underlying evidence.

“Current value” is a decision, not proof that one source is inherently correct. Every value retains its source, provider, confidence, locale/region, observed time, and decision.

### 1.3 Technical/descriptive boundary

Technical media facts live in the media-analysis record and are immutable for one exact source revision. Descriptive metadata never changes codecs, duration, streams, or source availability. Provider runtime is advisory only when it conflicts with measured technical duration; both may be retained with distinct provenance.

## 2. Provider policy

### 2.1 Built-in provider capabilities

| Media family | Primary | Fallback / selectable | Local evidence |
| --- | --- | --- | --- |
| Movies | TMDB | TheTVDB | NFO, embedded where supported, filenames, local artwork |
| Shows, seasons, episodes, specials | TMDB | TheTVDB | NFO, filenames, local artwork |
| Anime | AniList | TMDB/TheTVDB only when explicitly selected and supported | NFO, filenames, local artwork |
| Music artists, albums, tracks | MusicBrainz | AcoustID for opt-in fingerprint-assisted identification; Cover Art Archive for art | Embedded tags, NFO where supported, filenames, local art/lyrics |
| Audiobooks and books | Local OPF/NFO/embedded evidence | A future provider requires an approved contract update | Filenames, author/series/narrator tags, local artwork |
| Live channels/programs/recordings | Configured tuner/lineup/guide authority | Explicit owner mappings; descriptive provider enrichment only when separately approved | Channel/guide fields, local logos, recording-derived facts |

LRCLIB or another lyric service is a lyrics provider, not the canonical music identity provider. Subtitles and lyrics retain their own source, format, language, timing, and rights/provenance.

### 2.2 TMDB

TMDB MUST be available out of the box for supported movie/television matching through a Portico-managed application credential. An owner MAY install an advanced override or disable the provider. An override takes precedence only after validation.

TMDB requests MUST use the item/library language and region policy. Search considers original, translated, alternate, filename-derived, and accepted identity evidence. Popularity may break an otherwise equal tie; it MUST NOT outweigh a contradictory title/year/provider identity.

### 2.3 TheTVDB

TheTVDB MUST be available out of the box for supported movies/television as an integrated provider, not a required plugin. It is selectable as primary per library/item and is the default fallback when TMDB cannot make a sufficiently confident match or cannot supply required supported fields.

TheTVDB authentication tokens are server-private, cached for a bounded lifetime, and refreshed without exposing the application key. Provider-specific episode ordering/season types MUST be represented as an explicit selected ordering policy; one provider’s episode order cannot silently renumber an already accepted hierarchy.

### 2.4 Credentials and provider calls

Built-in application identifiers may be recoverable from distributed software and MUST NOT be treated as end-user secrets. They nevertheless remain Server-only build/configuration inputs and MUST NOT be returned by APIs, copied into browser/native bundles unnecessarily, displayed, logged, or included in diagnostics.

Provider calls are made directly by the Portico Server. Hosted Services MUST NOT receive titles, filenames, provider queries, provider IDs, artwork URLs, or metadata snapshots.

Every provider has bounded concurrency, response size, timeout, retry, and rate limits. `429` honors `Retry-After`; retries use cancellation-aware exponential backoff and single-flight where requests are equivalent. A provider outage never blocks library inventory or ordinary playback.

## 3. Matching and accepted identity

### 3.1 Evidence hierarchy

Matching evidence, strongest first, is generally:

1. an explicit manual Identify acceptance;
2. a valid typed provider ID from a prior accepted revision or owner-authored local metadata;
3. a provider crosswalk that proves equivalence to an accepted typed ID;
4. multiple agreeing strong signals: normalized title plus exact/near year, hierarchy/episode coordinates, author/artist/release identity, or exact source fingerprint;
5. filename/folder/embedded/local title and year evidence;
6. provider popularity or weak textual similarity.

Evidence that names the wrong entity kind, conflicts with an accepted hierarchy, or fails the provider’s ID syntax is rejected rather than merely down-weighted.

### 3.2 Candidate decision

All match candidates MUST retain provider, external ID/type, normalized score, evidence reasons, query evidence digest, observed time, and status. Confidence is in `[0,1]` and means Portico’s confidence in identity equivalence, not provider popularity or metadata completeness.

Automatic acceptance requires either:

- an exact valid typed provider identity; or
- confidence at or above `0.85`, at least two independent strong signals, no hard contradiction, and a margin of at least `0.12` over the next incompatible candidate.

Otherwise the item remains unmatched or retains its prior accepted identity and exposes candidates for owner review. An implementation MAY use a more conservative threshold; it MUST NOT use a less conservative one without a contract change or approved exception.

### 3.3 Accepted identities

An item may retain accepted identities from multiple providers when typed crosswalk evidence proves they describe the same canonical item. Within one provider and external type, only one identity is accepted at a time.

Automated evidence may establish the first accepted identity or reinforce the same one. It MUST NOT silently displace a different accepted identity. Manual Identify transactionally supersedes the old identity, records the actor and evidence, and fences all refresh work.

Manual Identify with a specific provider restricts that operation to the chosen provider/identity. Other providers MUST NOT run a fresh name search and overwrite the result. Future supplementary providers may add non-conflicting fields only through a proven crosswalk or explicit per-item permission.

### 3.4 Ambiguous items, duplicates, and merge

Ambiguous files remain separate candidates; Portico does not merge solely because normalized titles match. A merge requires accepted identity equivalence, exact content evidence, or explicit owner action.

Merge is one durable operation over a closed registry of all references. It preserves provider histories, revisions, locks, artwork, versions, people, relationships, Saved state, playback/viewer state, and audit records. Bounded staging may prepare a large merge; one fenced publication boundary switches canonical identity and its validated alias/projection generation. Readers see a coherent generation, not partially rewritten identity. Unknown references or conflicting protected state block publication instead of deleting evidence. See the metadata amendment for reconciliation and undo boundaries.

## 4. Field and relationship decisions

### 4.1 Canonical vocabulary

Portico uses typed fields rather than provider-shaped JSON. Common descriptive fields include display/original/sort/alternate titles, summaries, taglines, genres, tags/keywords, content ratings/certifications, community/critic ratings with scale and vote count, release events, original language, available metadata languages, countries, status, companies/studios/networks/labels/publishers, provider identities, and external links that pass URL policy.

Kind-specific vocabulary includes at least:

- movie: edition/version labels, collection/franchise membership, production/release events, and credited people;
- show: lifecycle status, creators, networks, selected episode ordering, and series-level relationships;
- season: selected ordering identity, season type/number/title, and show relationship;
- episode/special: season/show relationship, selected and alternate order coordinates, air/release event, production code, and guest/crew credits;
- anime: provider format/status/source material, season/year, episode count, studios, relations, synonyms, tags, and adult-content evidence without inventing a regional certification;
- music: the entity and relationship vocabulary in section 11.1;
- book/audiobook and Live/DVR: the vocabularies in sections 11.2 and 11.3.

`unknown`, `known empty`, `not applicable`, `redacted`, and `temporarily unavailable` are distinct states. A blank provider field is not proof of known empty, and a client MUST NOT collapse an unknown date/rating/relationship into a false zero, placeholder identity, or January 1 value.

### 4.2 Decision records

Every normalized field/relationship stores:

- field or relationship kind and typed target where applicable;
- value/display value plus normalized comparison form;
- source kind/source/provider and snapshot reference;
- confidence and decision (`candidate`, `accepted`, `locked`, `rejected`, `superseded`);
- locale, language, country/region, role, ordinal, attributes, and observed time where applicable;
- exact metadata revision.

Provider ordering is a preference for filling or supplementing fields, not blanket permission to overwrite manual or locked state. A lower-priority provider MAY fill a missing supported field. It MUST NOT replace a non-empty higher-priority accepted field unless the active refresh policy explicitly allows it and the evidence is retained.

Conflicting values may coexist as evidence; only one projected value (or ordered set for multi-value fields) is canonical for a given locale/context. Conflict resolution is deterministic and explains the selected source in owner detail.

## 5. Language, locale, region, and ordering

### 5.1 Selection

Metadata language is a normalized BCP 47 language tag. Region is an ISO 3166-1 alpha-2 code. Library settings override server defaults; per-operation owner selection may override the library for that operation.

The preferred resolution order is:

1. exact requested language-region;
2. requested base language in the selected region;
3. requested base language without region;
4. the item’s original language;
5. provider-neutral/default value;
6. server fallback language.

Portico retains the actual locale returned. It never labels fallback English as French or a primary release date as a selected regional theatrical date.

### 5.2 Dates, certifications, and orderings

Dates are typed and carry precision (`year`, `month`, `day`, `instant`) and role (`premiere`, `theatrical`, `digital`, `physical`, `broadcast`, `air`). Unknown components remain unknown; no January 1 invention is permitted.

Certifications retain country and scheme. A US rating cannot stand in for a different region, and an adult-content boolean is not a certification.

Television ordering (`aired`, `dvd`, `absolute`, `streaming`, provider season type, or approved custom order) is an explicit library/show choice. Changing it is a reconciled hierarchy operation with preview and state preservation, not an incidental provider refresh.

## 6. Local, embedded, and owner-authored metadata

Local evidence modes are:

- `off` — ignore descriptive local evidence except explicit provider IDs required for identity repair;
- `supplement` — use local IDs, people, artwork, and missing fields without replacing non-empty provider values;
- `prefer` — local supported fields take precedence over provider values.

The default is `prefer`, but every field retains provenance. Local NFO/OPF parsing is bounded, schema-tolerant, and confined to the media root. Remote URLs inside local files are not fetched without the remote-artwork security and rights policy.

Embedded tags are media-kind-aware. Music tags are useful identity and descriptive evidence; arbitrary container title tags MUST NOT unexpectedly rename commercial movies/shows unless the library preference permits them.

User watch state, play count, Saved state, and profile preferences found in NFO or tags MUST NOT be imported into multiple users or treated as descriptive metadata. Any future import is a separate explicit migration contract.

Portico does not write provider metadata, artwork, NFO, OPF, tags, or lyrics beside owner media by default. A future owner-enabled export is a separate atomic operation with source write-capability checks, collision/backup policy, and a preview; a metadata refresh never acquires filesystem write authority implicitly.

## 7. Manual edits, locks, and undo

Manual edits publish through the same revision transaction as provider refreshes. Changed fields lock by default. An owner may explicitly edit without locking only when the UI says future refreshes may replace the value.

Locks are granular and apply to:

- scalar and typed fields;
- each multi-value field or relationship class;
- people/credit roles and order;
- provider identity choice;
- hierarchy/order choice;
- artwork role/selection;
- local-vs-provider authority where selected.

Unlocking is explicit and does not immediately discard the current value. The next refresh may replace it under the active policy. “Lock all” and “unlock all” require confirmation and preview the scope.

Every manual revision retains the prior revision and supports a bounded owner-visible restore/undo that creates a new revision. Undo never rewrites history.

## 8. Refresh and cascades

Canonical refresh intents are:

| Intent | Behavior |
| --- | --- |
| `fill_missing` | Add missing supported fields/images only; preserve accepted non-empty state |
| `refresh_unlocked` | Re-fetch and reconsider unlocked fields while respecting identity and locks |
| `replace_unlocked` | Replace all unlocked supported fields/images from the selected identity/provider policy |
| `identify` | Owner selects one provider candidate; identity change and refresh commit atomically |
| `repair_assets` | Reacquire missing/corrupt selected local assets without rematching identity |
| `reconcile_hierarchy` | Refresh descendants/order using accepted parent identity and selected ordering |

Ordinary filesystem scans MUST NOT constantly re-fetch unchanged metadata. Initial discovery, changed relevant local evidence, missing required metadata, explicit refresh, missing/corrupt selected asset, provider-policy revision, and scheduled age policy are valid triggers.

Show, anime, music, audiobook, and collection cascades use durable paged continuations with per-item leases/retries. They MUST support arbitrary supported depth and more than 500 descendants without fixed truncation. Restart, duplicate page, cancellation, and provider rate limit preserve progress.

A parent refresh MUST NOT claim that a hierarchy refresh is complete or publish an internally contradictory parent/child batch. Child revisions MAY publish incrementally because large hierarchies cannot be one database transaction; each child is atomic, carries the hierarchy-operation revision, and leaves the operation visibly in progress or partially complete. A failed descendant remains on its previous valid revision.

## 9. People, credits, and relationships

People are canonical typed entities where provider/local identity permits it; otherwise they remain scoped named credits until safely reconciled. The same display name alone never merges people.

Credits retain person identity/name, department, role/job/character, billing order, language, provider, and revision. Creator, director, writer, actor, guest star, narrator, author, album artist, track artist, composer, conductor, producer, studio, network, label, and publisher are distinct roles.

Relationships such as collection/franchise, season/show, episode/season, album/artist, track/album, book/series, adaptation, sequel/prequel, related work, keyword, company, country, language, and external ID are typed. Clients MUST NOT infer relationship meaning from display strings.

Provider portraits are selected artwork with the same local-storage, provenance, revision, and cleanup rules as media artwork. Failed staging cannot orphan or publish a portrait.

## 10. Artwork and attribution

### 10.1 Artwork lifecycle

Provider artwork URLs are transient acquisition inputs only. An interactive artwork request MUST NOT contact a metadata provider.

A selected provider image is downloaded outside the database transaction, bounded by URL/origin/redirect/size/type/dimension policy, decoded and normalized as required, content-identified, written privately, validated, and atomically installed before selection commits. Failed refresh retains the last valid selected image.

Artwork retains role (`poster`, `backdrop`, `thumbnail`, `still`, `logo`, `square`, `portrait`, `banner`, `cover`), locale, dimensions, vote/rank evidence, provider, provider image identity, selection state, and revision. Selected art is always retained locally. Optional caching of additional originals is bounded, policy-controlled, and included in storage estimates/cleanup.

The same artwork file may satisfy multiple roles only when its shape/crop contract permits it. Clients consume server-owned artwork endpoints and never provider URLs.

### 10.2 Provider attribution

Attribution is a product requirement, not an optional legal footer.

- Every Server/Web/client About or Credits surface that displays TMDB-derived data MUST show an approved TMDB logo and the required notice: **“This product uses the TMDB API but is not endorsed or certified by TMDB.”** The TMDB mark remains less prominent than Portico’s.
- When TheTVDB-derived metadata is shown, the applicable detail/info surface MUST provide visible attribution with a direct link on pointer/touch devices, using wording at least as clear as **“Metadata provided by TheTVDB.”** Television clients print the provider URL or show a non-misleading information surface when links cannot open.
- Mixed-provider detail MAY consolidate attribution into a provider-information panel, but it MUST remain reachable from the content displaying the data and must identify every applicable provider.
- Provider marks are never recolored, distorted, or presented as endorsement.
- Provider licensing/attribution requirements are versioned operational inputs. A provider whose current terms cannot be satisfied is disabled rather than used without attribution.

Portico MUST retain a provider-policy record with terms-review date, applicable retention requirements, attribution text/assets, credential owner, and disable/purge procedure. Historical statements about provider terms are not current verified legal facts and must not become an automatic purge timer. Populate operational terms from the applicable provider agreement and authoritative current documentation before distribution. This reconciliation preserves distributed project keys and the approved attribution experience; it introduces no metadata broker or ordinary owner-key requirement.

## 11. Music, books, lyrics, and Live metadata

### 11.1 Music

Music identity distinguishes artist, release group, release, medium/disc, recording, and track. Artist-credit join phrases and multi-artist credits are preserved. ISRC, barcode, catalog number, label, country, status, packaging, script/language, work, and relationship evidence remain typed.

AcoustID fingerprinting is opt-in Complete/Custom work and uses an owner/provider credential policy where required. A fingerprint proposes candidates; it does not overwrite accepted MusicBrainz identities without the normal confidence and margin rules.

Lyrics retain provider/local/upload source, language, format, synchronized/unsynchronized state, timing, and rights/provenance. A missing lyric is not metadata-refresh failure. Provider-fetched lyrics are never silently written beside owner media.

### 11.2 Books and audiobooks

Author, narrator, series, series position, edition, publisher, language, identifiers, chapter structure, and cover are typed. Local OPF/NFO/embedded evidence is authoritative according to local mode. A title/author name match alone is insufficient to merge editions or books. No unsupported remote book provider is implied by a generic provider interface.

### 11.3 Live TV and DVR

Lineup/guide providers own channel/program schedule evidence. Channel ID, provider locator, number, callsign, logo, program ID, start/end, episode/season, rating, and guide revision retain source provenance.

Metadata-provider enrichment of a live program requires a stable accepted mapping and MUST NOT delay tuning, guide display, or recording. A guide correction may update a recording’s descriptive metadata but cannot change the recorded source, time range, or viewer state. A vanished channel is disabled/retired rather than having its historical recordings and mappings erased.

## 12. Provider failure, staleness, and conflict

Provider failures are classified as `offline`, `timeout`, `rate_limited`, `authentication`, `not_found`, `malformed`, `rights_or_terms`, and `unknown`.

- Timeout, `5xx`, malformed response, or rate limit preserves all current accepted metadata and assets.
- `not_found` for one provider does not erase a prior identity without corroborating evidence or explicit repair.
- Authentication/licensing failure disables new provider work and alerts the owner without exposing credentials.
- A provider returning empty fields is not authority to clear non-empty accepted fields unless `replace_unlocked` explicitly requested replacement and the provider successfully returned a complete record.
- One provider’s exception does not roll back another provider’s committed supplementary evidence.
- Last successful and last attempted refresh are distinct.
- Stale metadata remains usable and is labeled in owner diagnostics; viewer surfaces do not become empty merely because refresh is due.

When evidence conflicts, preserve both, select deterministically under policy, and show the owner the conflict. A provider outage never becomes permanent data loss.

The only exception to retaining stale provider content is a current licensing, rights, erasure, or retention obligation. Such a purge is a deliberate `rights_or_terms` operation with an audited provider-policy revision; it removes only the affected provider content and never deletes independent manual/local evidence, media identity, watch state, or the media item itself. Portico MUST NOT disguise a required purge as an ordinary refresh failure.

## 13. Privacy and public projections

Provider searches reveal queries about an owner’s library to the selected provider. Portico’s privacy/first-run information MUST explain that enabled metadata providers receive title/identity queries directly from the server.

No provider receives account email, username, profile, watch history, local path, remote-storage credential, IP topology beyond ordinary request transport, or unrelated library contents. Query only the minimum title/year/typed identity/language/region needed.

Viewer list/card projections are lean and omit raw snapshots, paths, URLs, match candidates, rejected/superseded identities, provider errors, and acquisition data. Detail projections expose only bounded accepted/locked evidence needed by the product. Owner metadata tools may see candidates, provenance, conflicts, and sanitized provider status; raw response bodies remain private diagnostics.

`lockedByUserId` is internal audit evidence. Viewer and ordinary owner projections expose only whether a value is locked and, where needed, whether the current actor may unlock it; they MUST NOT expose the locking actor's opaque account/user identifier. An authorized audit export may include the actor only under the Settings/Operations redaction and access rules.

Snapshots are bounded, hashed, versioned, and subject to provider retention/purge policy. Logs contain provider, operation ID, opaque media ID, status class, duration, and retry—not full queries, artwork URLs, credentials, raw JSON, or private paths.

## 14. Conformance

Serialized metadata decisions validate against `metadata-decision.schema.json`. Scenario fixtures live in `conformance.json`.

A conformant implementation proves:

- built-in TMDB primary and TheTVDB fallback/selectable behavior without ordinary owner keys;
- an exact provider ID wins over name popularity while invalid/mismatched IDs fail closed;
- auto-match threshold, independent-signal, contradiction, and margin rules;
- manual Identify cannot be overwritten by another provider’s name search;
- locks protect scalar, multi-value, people, relationship, identity, and artwork choices;
- stale concurrent refresh loses its revision CAS and publishes no artifact/state;
- provider outage/empty response preserves the last valid revision and artwork;
- normal unchanged scan makes no unnecessary provider request;
- language/region fallback is labeled truthfully;
- unknown dates retain precision rather than invented components;
- metadata/artwork staging and revision commit are atomic and crash-safe;
- descendant cascades resume after restart and exceed 500 items without truncation;
- merge blocks on unknown references/conflicting protected state;
- viewers never receive provider URLs, raw snapshots, local paths, or candidates;
- TMDB and TheTVDB attribution appears on every required client/surface;
- music, audiobook, lyrics, and Live relationships preserve typed provenance.

Minimum applicability matrix:

| Rule family | Server | Hosted | Web admin | Client Core | Consumer clients |
| --- | --- | --- | --- | --- | --- |
| Provider calls/evidence | required | prohibited | manage/project | canonical DTOs | no raw evidence |
| Matching/accepted identity | required | not applicable | required | states/actions | selected result only |
| Manual edits/locks | required | not applicable | required | revision/ETag | where editing offered |
| Artwork local delivery | required | prohibited | required | server URLs only | required |
| Attribution | provider record | not applicable | required | provider marker | required |
| Locale/region | required | not applicable | required | canonical tags | required |
| Failure/staleness | required | not applicable | required | canonical states | preserve valid data |

Approved exceptions use the governance form exactly, with `Contract: PC-METADATA#<section>` and conformance evidence.

The informative comparison behind these decisions is in `plex-jellyfin-precedent-review.md`; it cannot override this contract.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.
