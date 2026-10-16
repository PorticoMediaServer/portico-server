# Catalogue and metadata

The catalogue separates logical titles from physical files. `internal/compactcatalog` owns relational facts and projections; `internal/catalog` composes authorised browse, search, Home and detail documents. Metadata and technical file analysis have separate authority.

## Titles, files and missing media

A title has a stable identity, hierarchy and personal state. It can have several files, versions, editions and parts. Provider identity can merge matching titles within a library. Filename/title/year and useful audio tags provide provisional identity; ambiguous or untagged audio still has path-dependent limitations. A show begins with source-relative folder identity until accepted provider identity or an owner merge joins it.

A complete healthy observation marks an absent file missing immediately. It stops being offered. A title with no available copies remains visible and missing, with `missingSince`. A verified reported removal can mark the final file missing without waiting for a second scan. Unreachable roots, changed mount identity and unreadable coverage cannot prove absence. Ordinary unexpectedly empty roots remain protected; verified watcher/path reports handle real last-file deletion.

The owner's missing-title rule is immediately, after days (default 30), or never. Reappearance reattaches when identity matches. Final title removal preserves bounded personal state for recognised identity, including provider identity. This is not a promise to restore every history row or playlist position. Removing a library leaves disk media intact and dismantles catalogue state in background batches. Some derived rows can leave a hidden empty library record.

Delete previews and typed confirmation precede deletion from disk. There is no state-folder trash copy or restore-from-trash route. An owner can preview/delete one physical version; other versions and state remain, while the last deletion follows missing-title retention. Changed files are not silently substituted into a confirmed deletion. [Identity](../../_program/reports/identity.md), [identity review](../../_program/reports/review-identity.md) and [server follow-ups](../../_program/reports/server-followups.md) record merge-reference, multipart and large-transaction limits.

## Intake and naming

`internal/naming` combines bounded folder, filename and embedded evidence, with provenance and confidence. Usable file evidence wins contradictory folders: S02E01 within Season 1 is season 2. Flat shows, translated season folders, E01/01 within Season 2, episode zero, editions and split parts have reader support and scan integration. Audio tags outrank filename hints.

Scans enumerate current local file facts instead of treating unchanged directory times as proof. A damaged row, bad NFO line, unreadable subtree or failed probe leaves usable neighbours and name-based titles. Owner diagnostics name skipped units. `.plexignore` and `.ignore` are bounded scan configuration, including in File List Only; that tier still does not open media or ordinary sidecar content. Disc folders are grouped unsupported titles, not a guessed main stream.

Monitoring defaults on: native watchers feed limited scans, with debounce, bounded watches and periodic network/failure fallback. Sidecar additions/changes/removals invalidate affected evidence. Jobs queue only missing analysis evidence and prune bounded terminal history. Native Windows/macOS watcher and network-share acceptance remain unverified. [Scan loop](../../_program/reports/scan-loop.md) supersedes earlier naming/sidecar wiring gaps in [naming](../../_program/reports/naming.md).

## Metadata and attention

The owner selects one provider per library. TMDB defaults for films, TV and anime; TheTVDB or AniList can be selected where supported. Local metadata only makes no online matching calls. Music uses MusicBrainz and local tag identity; audiobooks remain local. There is no automatic provider chain or Hosted metadata broker.

Matching accepts convincing yearless titles and minor misspellings, using typed IDs, normalised titles, kind/runtime and conflicting evidence. Low-confidence/unmatched titles enter the owner-only needs-attention queue with candidates, accept/search/provider-ID/dismiss actions and exact totals. Owner edits and locks outrank refresh. Local evidence is reconciled only after complete observations; outages retain prior evidence. Provider rates, concurrency and owner-search priority are bounded.

Artwork is selected by role and retained locally. Local-artwork usage has its own quota ledger, separate from fetched artwork. Provider locale may be unknown rather than invented. Per-library provider changes schedule rematching and retain presentation until replacement is published. Live-provider results remain unqualified. Sources: [providers/matching](../../_program/reports/providers-matching.md), [music/audiobooks](../../_program/reports/music-audiobooks.md), [console](../../_program/reports/console.md).

## Read projections, sorting and freshness

Facts queue bounded derivations for search, browse, edges, availability, counts, facets and recommendations. Browse serves the previous relational generation during rebuild; search builds a separate FTS slot and switches publication. First builds serve committed work. Failed titles are recorded and retried without blocking healthy work. There is no normal “being prepared” browse/search state.

Read documents use consistent snapshots, restrictions in queries and bounded indexed candidate work. Natural sort strips relevant leading articles/symbols and folds diacritics. Search indexes alternative titles, parent context, people, artists, authors, narrators, years and catalogue collections; CJK uses indexed substring candidates. Genres and Series have real totals and paging. Totals are exact up to 10,000; past that a page says the total is a lower bound and every client shows "10,000+" while paging on (see [02 Server](02%20Server.md)). Ratings store integer 0–10; clients display half-stars by dividing by two, with unset distinct from zero.

The 14 October decision requires the main sort/filter vocabulary in every subgroup. [cp-browse-parity](../../_program/reports/cp-browse-parity.md) delivers it on web, phone/tablet, TV and Vega: every grouping (a person, playlist, collection, Home row, My List, Favorites, History, a show, album, book or series) has the library's whole Sort and Filter, a group's own order is the server sort `groupOrder`, and lists across libraries are one list (`POST /v1/browse`). Personal collections keep a manual order, which is their default sort and the order they play in (Justin, 15 Oct). Per-grouping performance limits are in [grouping-sorts](../../_program/reports/grouping-sorts.md). [Search/sort](../../_program/reports/search-sort.md) records remaining Saved-collection search, reanchoring and specialised time-field gaps.

Separate ordering, directory, personal and progress counters avoid conflicts caused by unrelated position ticks. The event stream sends scoped hints; the [client hub](../../_program/reports/freshness-clients.md) quietly rereads visible dependencies. Appended lists, some special screens and device focus still need acceptance or further integration.

## Current independent review

The independent [scan review](../../_program/reports/review-scan.md) remains in progress. Its appended fixes address ambiguous monitor/fallback empty-root evidence, unbounded removal calls, distinct-release track identity, extra ownership, Unicode grouping and stale-page revival. Final review/integration/native acceptance remains required; the original finding text alone is not current fix status.

The completed [freshness review](../../_program/reports/review-freshness.md) reports eight fixes for active-search/History/dismissal hints, quiet resync after ring pruning, observation-keyed Continue counts, Unicode filters and failure dates. Two unchanged SQL guards and real load/crash/device acceptance remain open.
