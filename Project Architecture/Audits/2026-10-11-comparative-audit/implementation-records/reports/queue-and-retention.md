# Outcome

Implemented complete server-side queue selectors for browse queries, saved views and saved collections. The query uses the browse vocabulary for library, pivot, grouping membership, filters and ordering; viewers never enumerate the queue on the client. Playback can begin after the first 256 playable keys. The existing durable chunk builder captures the remainder, preserves source order and seeded shuffle, applies viewer restrictions in SQL, and exposes the accepted source revision.

Completed a source inventory of all 649 ordinary baseline tables and three FTS virtual tables. Added missing disposable-record cleanup, constant-time resource ceilings for long-lived/replay records, and analysis summary reclamation. Existing catalogue dictionary/identity cleanup was present before this lane and is now paced through backgroundSpeed. The detailed table inventory below distinguishes direct deletion, cascades, fixed cardinality, intentional personal retention and the new controls.

Execution: personal Ubuntu mini-1, ehler-homelab-mini-1. Initial tree clean; base 6f1530acb899fcc0241f4174492119826164fd8c. No commit, Git metadata mutation, push or deployment. context-sync failed at its read-only state lock; programme/lane briefs and canonical Portico/project standards/verification guidance were read directly. The cheap read-only helper script returned no text; no helper result or edit was used.

# Decisions I made

- A query selector contains libraryId plus the browse API's pivot/query/sort. Groupings use the same membership predicates as browse; aggregate group-card pivots must be resolved to their underlying title pivot and filter. Cursor/range/limit/seek are paging controls and cannot narrow a queue selector.
- Saved references accept both private saved views and authorized saved collections. Collections retain their saved-entry order; views retain their saved definition. A private view remains private; server-visible collections use existing resource permissions. Playlists continue using the existing playlist container selector.
- Query containers expand to playable members in parent browse order, then canonical collection, episode, album/disc/track or book-part order. Overlapping query containers include a title once. Whole-query television includes the members selected by browse, including specials; the existing show-container selector's special handling is unchanged.
- Ordered and shuffled query playback starts from the first captured batch. A supplied seed is stable as the queue grows; the first shuffled pick is from that initial batch, rather than requiring a full-size counting pass. Existing queue edits preserve played occurrences. Explicit IDs remain bounded to 500; a queue remains bounded to ten million keys.
- Preserve one WAL read snapshot and the compiled SQL/arguments until capture completes. This also preserves recommendation ranking across a composition-day change. At most two query captures pin read connections; each has a 30-minute hard lifetime. Pool acquisition retains the HTTP deadline and a 30-second maximum admission wait. A saturated capture budget uses the existing retryable queue_building error; no foreground/control admission was weakened.
- After a process restart, resume only if the recorded source revision still agrees. If the catalogue, personal selector inputs or authority changed, keep the captured prefix, mark the segment failed and retain its existing occurrences; never combine two memberships. A new Play captures a new selection. Saved-collection restart fencing conservatively includes the authorized libraries' catalogue/personal revisions.
- Disposable operation receipts keep 30 days, marker-edit history 90 days, expired pairing records seven days, count caches seven days and stale rate/reference records one day, with reference/liveness checks. Existing personal receipts keep their existing 30-day policy and permanent saved_operation_fences.
- Replay/revocation/removal evidence is not age-pruned. Guard long-lived ledgers, tombstones, retained revision rows and accumulating caches with finite resource ceilings, checked through existing primary/unique indexes in constant time on new records. Exact retries and updates still work at the ceiling. Limits are compiled safety bounds, not a new owner settings wall. A new record can be refused at a ceiling; existing records are preserved. Natural-key interning/UPSERTs and intent replays remain valid even when their integer primary key is assigned by SQLite. Hosted backlogs and obsolete directory cache entries have ceilings as well as their ordinary drain paths.
- Do not expire the owner's intentionally permanent play history, personal history, title-linked watch state or progress. Existing erasure/title/provider-state lifecycle remains authoritative. The ordering/replay ledgers have resource ceilings; that is separate from history's age policy.
- Keep physical media deletion with its owning custody/lease code. Analysis summary rows go only after their artifacts and heads are gone, using the owner's analysis RetainDays. Prepared-job metadata goes only after its generation is cleaned and no version references it; deleted version metadata must have no session pin.
- Fixed-sort presentation changes and subtitle-mutation retirement must move their screen wrappers together. They cannot safely be completed by changing this lane's shared code and deleting still-called server routes.

# Bugs

- B-07.15: fixed for the lane's source scope. Catalogue people/term/attribute/credit-label/identity pruning was already present; added bounds/pruning for the remaining append-only or long-lived ledgers and caches found in the baseline inventory. No production size, multi-year growth or sustained-load reproduction is claimed.
- S1: implemented, with the explicit restart/source-change safety behavior above.
- S4: implemented and inventoried below. Personal history intentionally kept forever is recorded as a decision, not classified as a leak.
- Shared queue test fixture: fixed a pre-existing raw slash passed to an already-encoded media route parameter. This initially made 30 queue-player cases fail before their actual assertions; the one-line fixture repair restores the legitimate media path. All 36 cases then passed.

The lane's explicit bug assignment is B-07.15; the decision report's other domain incidents are not silently reclassified as this lane's fixes. FIX-LOG-2026-10-12.md and the B-07.15 domain/register entries were read; prior catalogue fixes were not redone.

# API changes

- POST /v1/queues and POST /v1/queues/{id}/segments: selector.query was an untyped JSON placeholder and always rejected as unsupported_selector. It is now typed as {libraryId, pivot?, query?, sort?}, with the same bounded expression parsing as browse. Added selector.saved: {id} for an authorized saved view/collection. Exactly one of container/query/saved/items is required.
- Queue segment headers, including queue command replies and GET queue/window/entry results where the header is present, now optionally carry sourceRevision. Existing container/item selectors omit it. Client-core preserves it in QueueSegment.
- No route added or removed. Existing queue routes remain on apikit, ScopePlayback and their existing restriction/write/feature matrices. New selector paths additionally have HTTP privacy regressions. Both apigen and clientcontractgen regenerated typed consumers; the latter carries the selector in retained device-command request types without adding a production route.
- Client-core adds QueueBrowseSelector, the saved union member and QueueClient.createSelector. V1QueuePlayer.playSelector accepts shuffleSeed. There is no client-side membership enumeration or compatibility translation.

# Schema changes

Edited 0001_baseline.sql directly and regenerated known_schema.go/known_columns.go through scripts/schema-sync.py. No migration or compatibility/backfill path was added.

- queue_v1_snapshots.source_revision records the accepted membership/order fence.
- maintenance_retention_rows keeps one indexed creation timestamp per disposable record. Its text key supports ordinary rowids and the WITHOUT ROWID count cache's selector key. Insert/delete triggers keep the age ledger synchronized; expiry queries use the age index and recheck liveness under the writer gate. Group/receiver child records are dismantled before parent removal.
- maintenance_table_bounds stores one counter/ceiling per compiled bounded table. Guard triggers check a new primary or natural unique key through its existing index; inserts/deletes maintain counters. Baseline counts are seeded from actual baseline rows. No per-insert full-table count runs.
- analysis_results_retention indexes retired_ms/id for the bounded reference-safe summary cleanup.

## Added age/reference policies

The full predicates are in server/internal/tableretention/retention.go. A dedicated supervised worker checks every 15 minutes and drains backlog in one-second turns. It is separate from WAL checkpoint monitoring, so an owner background pause cannot stall checkpoint housekeeping. Expired party/receiver histories are dismantled child-first in row quanta before deleting their parent, avoiding large cascades. Gentle/Normal/Fast use at most 64/128/256 deletions per table per turn, bounded by the caller's limit, with PaceBackground between released transactions. Catalogue dictionary scanning uses the same speed-dependent candidate sizes; analysis retention also paces completed sweeps.

| Table | Age before eligibility | Protection |
|---|---:|---|
| `metadata_owner_history` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `analysis_probe_bindings` | 1 days | Live/reference predicate rechecked in the maintenance write; see source |
| `analysis_marker_sets` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `analysis_markers` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `metadata_relationship_decisions` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `artwork_jobs` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `subtitle_sidecars` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `personal_device_sequences` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `remote_analysis_status` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `remote_object_refs` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `playback_requests` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `playback_legacy_replacement_requests` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `subtitle_operations` | 30 days | Disposable receipt/cache/history |
| `playback_marker_skips` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `analysis_marker_history` | 90 days | Disposable receipt/cache/history |
| `admin_receipts` | 30 days | Disposable receipt/cache/history |
| `queue_v1_container_counts` | 7 days | Disposable receipt/cache/history |
| `social_rate` | 1 days | Live/reference predicate rechecked in the maintenance write; see source |
| `social_cast_bootstraps` | 7 days | Live/reference predicate rechecked in the maintenance write; see source |
| `social_cast_devices` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `social_receivers` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `social_groups` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `access_invitations` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `prepared_media_jobs` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `prepared_media_versions` | 30 days | Live/reference predicate rechecked in the maintenance write; see source |
| `lc_playback_refs` | 1 days | Live/reference predicate rechecked in the maintenance write; see source |
| `live_allocations` | 7 days | Live/reference predicate rechecked in the maintenance write; see source |
| `live_reservations` | 7 days | Live/reference predicate rechecked in the maintenance write; see source |

## Added resource ceilings

These rows retain replay/revocation/revision meaning, cap an accumulating cache, or bound soft-deleted owner resources. Their ordinary existing cleanup/cascade remains active. Caches and live parent rows also have ceilings where a parent can persist forever. Exhaustion rejects a new primary key; updates/exact replays remain available. The SQL diagnostic stays in the server's existing error handling; no new client-visible string or settings control was introduced.

| Table | Maximum rows |
|---|---:|
| `admin_channel_logos` | 100,000 |
| `catalog_attribute_fields` | 10,000 |
| `catalog_dataset_vocabulary` | 1,000,000 |
| `catalog_playlists` | 1,000,000 |
| `console_report_events` | 10,000,000 |
| `content_rating_ages` | 1,000,000 |
| `direct_profiles` | 10,000,000 |
| `download_receipt_keys` | 10,000 |
| `dvr_owner_revisions` | 1,000,000 |
| `dvr_recordings` | 1,000,000 |
| `dvr_rules` | 1,000,000 |
| `hosted_claim_cleanup_receipts` | 100,000 |
| `hosted_membership_journal` | 10,000,000 |
| `hosted_profile_erasure_receipts` | 1,000,000 |
| `hosted_profile_revisions` | 1,000,000 |
| `hosted_server_events` | 1,000,000 |
| `hosted_signing_revocations` | 100,000 |
| `inventory_directory_cache` | 10,000,000 |
| `lc_channels` | 100,000 |
| `lc_receipts` | 1,000,000 |
| `library_navigation_revisions` | 1,000,000 |
| `live_channel_preferences` | 5,000,000 |
| `live_enable_receipts` | 1,000,000 |
| `live_preference_receipts` | 1,000,000 |
| `live_remote_save_bindings` | 100,000 |
| `live_source_identities` | 100,000 |
| `live_source_removals` | 100,000 |
| `lyric_fetch_runs` | 10,000,000 |
| `lyric_resources` | 10,000,000 |
| `lyric_revisions` | 10,000,000 |
| `mb_candidate_reasons` | 10,000,000 |
| `mb_recording_aliases` | 10,000,000 |
| `mb_release_aliases` | 10,000,000 |
| `metadata_movie_candidates` | 10,000,000 |
| `music_match_observations` | 10,000,000 |
| `networking_claim_cancellations` | 100,000 |
| `networking_claim_intents` | 100,000 |
| `networking_server_identities` | 100,000 |
| `notification_inbox` | 1,000,000 |
| `personal_activity_revisions` | 1,000,000 |
| `personal_conflicts` | 10,000,000 |
| `playback_personal_claims` | 10,000,000 |
| `playback_personal_intents` | 10,000,000 |
| `playback_personal_profile_fences` | 1,000,000 |
| `playlist_directory_revisions` | 1,000,000 |
| `rec_profile_revisions` | 1,000,000 |
| `remote_sources` | 100,000 |
| `saved_operation_fences` | 10,000,000 |
| `saved_pin_order_revisions` | 1,000,000 |
| `saved_resources` | 1,000,000 |
| `social_group_invites` | 10,000,000 |
| `social_group_members` | 10,000,000 |
| `social_groups` | 10,000,000 |
| `social_handoffs` | 10,000,000 |
| `social_receiver_grants` | 10,000,000 |
| `subtitle_original_files` | 10,000,000 |
| `subtitle_original_revisions` | 10,000,000 |
| `subtitle_render_revisions` | 10,000,000 |
| `subtitle_resources` | 10,000,000 |
| `viewer_revisions` | 10,000,000 |

## All-table source inventory

Every ordinary table declared by CREATE TABLE in the baseline and every declared FTS virtual table is listed. SQLite's FTS shadow tables and sqlite_sequence follow their owning index/primary table and are not separate application insert paths. Live catalogue/configuration tables can grow with owner-owned live content; that is product state, rather than append-only housekeeping history. Cascades and referenced parent keys are explicit below. Capped child histories include subtitles/lyrics, matches/candidates, social handoffs/invites/members/grants, feedback events and conflict evidence; a permanent parent therefore cannot imply unbounded history growth. A direct delete citation alone establishes a source deletion path, not production throughput; the named subsystem policies and acceptance tests remain relevant. Reference/unused baseline tables with no current append writer are identified without inventing a retention worker for them.

| Table | Insert evidence | Lifetime/bound and deletion evidence |
|---|---|---|
| `catalog_kinds` | `Baseline/trigger/subsystem declaration` | Baseline vocabulary; schema bounds IDs to 1–99. |
| `catalog_libraries` | `server/internal/compactcatalog/write.go:50` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `catalog_entities` | `server/internal/compactcatalog/write.go:206` | Direct delete: server/internal/compactcatalog/write.go:600 |
| `catalog_identities` | `server/internal/compactcatalog/identity.go:116` | Daily cursor-based orphan pruning, 64/128/256 candidates per transaction; server/internal/compactcatalog/retention.go:39; Direct delete: server/internal/compactcatalog/identity.go:149 |
| `catalog_assets` | `server/internal/compactcatalog/write.go:642` | Direct delete: server/internal/compactcatalog/write.go:815 |
| `catalog_asset_links` | `server/internal/compactcatalog/write.go:860` | ON DELETE CASCADE from catalog_assets, catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/write.go:881 |
| `catalog_item_details` | `server/internal/metadata/artwork_worker.go:604` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `catalog_item_availability` | `server/internal/compactcatalog/derive_availability.go:28` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/derive_availability.go:27 |
| `catalog_missing_titles` | `server/internal/compactcatalog/write.go:779` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/library_cleanup.go:58 |
| `catalog_missing_title_counts` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_libraries; baseline foreign keys |
| `retained_title_state` | `server/internal/catalog/retained_state.go:175` | Direct delete: server/internal/catalog/retained_state.go:170 |
| `retained_title_state_counts` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/persistence/migrations/0001_baseline.sql:183 |
| `retained_title_identities` | `server/internal/catalog/retained_state.go:180` | ON DELETE CASCADE from retained_title_state; baseline foreign keys |
| `catalog_shows` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `catalog_seasons` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `catalog_episodes` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `catalog_artists` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `catalog_albums` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `catalog_songs` | `server/internal/catalog/inventory_admin.go:327` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `catalog_song_artists` | `server/internal/compactcatalog/write.go:1080` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/write.go:1076 |
| `catalog_books` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `catalog_book_files` | `server/internal/catalog/inventory_admin.go:332` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `catalog_book_chapters` | `server/internal/compactcatalog/write.go:1102` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/write.go:1098 |
| `catalog_collections` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `catalog_collection_members` | `server/internal/compactcatalog/write.go:1051` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/write.go:1053 |
| `catalog_playlists` | `server/internal/catalog/playlists.go:247` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go; Direct delete: server/internal/identity/direct_profiles.go:583 |
| `catalog_playlist_entries` | `server/internal/catalog/playlist_window.go:76` | ON DELETE CASCADE from catalog_playlists; baseline foreign keys; Direct delete: server/internal/catalog/playlists.go:284 |
| `catalog_people` | `server/internal/metadata/show_people.go:23` | Daily cursor-based orphan pruning, 64/128/256 candidates per transaction; server/internal/compactcatalog/retention.go:39 |
| `catalog_credits` | `server/internal/compactcatalog/write.go:999` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/write.go:967 |
| `catalog_term_sources` | `server/internal/compactcatalog/write.go:939` | ON DELETE CASCADE from catalog_entity_terms; baseline foreign keys; Direct delete: server/internal/compactcatalog/write.go:913 |
| `catalog_related_facets` | `server/internal/compactcatalog/derive_availability.go:92` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/derive_availability.go:91 |
| `catalog_search_documents` | `server/internal/compactcatalog/derive_search.go:39` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/derive_search.go:38 |
| `catalog_dirty` | `server/internal/metadata/repair_fields.go:480` | Direct delete: server/internal/compactcatalog/derive.go:475 |
| `catalog_derivations` | `server/internal/compactcatalog/derive.go:234` | Fixed compiled derivation-domain keys; upsert per domain |
| `personal_items` | `server/internal/catalogtest/taste.go:436` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/title_merge.go:198 |
| `catalog_browse_rows` | `server/internal/compactcatalog/derive_browse.go:211` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/derive_browse.go:189 |
| `catalog_browse_counted_rows` | `server/internal/compactcatalog/derive_browse.go:223` | Direct delete: server/internal/compactcatalog/derive_browse.go:185 |
| `catalog_browse_item_metrics` | `server/internal/compactcatalog/derive_browse.go:526` | Direct delete: server/internal/compactcatalog/derive_browse.go:529 |
| `catalog_browse_member_metrics` | `server/internal/compactcatalog/derive_browse.go:541` | Direct delete: server/internal/compactcatalog/derive_browse.go:107 |
| `catalog_browse_metric_cursors` | `server/internal/compactcatalog/derive_browse.go:416` | Direct delete: server/internal/compactcatalog/derive_browse.go:422 |
| `catalog_home_counted_rows` | `server/internal/compactcatalog/derive_browse.go:277` | Direct delete: server/internal/compactcatalog/derive_browse.go:276 |
| `catalog_movie_category_cursors` | `server/internal/compactcatalog/browse_categories.go:49` | Direct delete: server/internal/compactcatalog/browse_categories.go:101 |
| `catalog_movie_category_members` | `server/internal/compactcatalog/browse_categories.go:84` | Direct delete: server/internal/compactcatalog/browse_categories.go:30 |
| `catalog_movie_category_visible_edges` | `server/internal/compactcatalog/browse_category_visibility.go:220` | Direct delete: server/internal/compactcatalog/browse_category_visibility.go:228 |
| `catalog_personal_sort_item_jobs` | `server/internal/compactcatalog/personal_sort_buckets.go:145` | Direct delete: server/internal/compactcatalog/personal_sort_buckets.go:86 |
| `catalog_personal_sort_members` | `server/internal/compactcatalog/personal_sort_buckets.go:282` | Direct delete: server/internal/compactcatalog/personal_sort_buckets.go:278 |
| `compact_visibility_classes` | `server/internal/catalog/visibility_classes.go:332` | Direct delete: server/internal/compactcatalog/library_cleanup.go:63 |
| `compact_visibility_rows` | `server/internal/catalog/visibility_incremental.go:193` | Direct delete: server/internal/compactcatalog/library_cleanup.go:47 |
| `compact_visibility_unavailable` | `server/internal/catalog/visibility_incremental.go:16` | Direct delete: server/internal/compactcatalog/library_cleanup.go:48 |
| `catalog_book_position_contrib` | `server/internal/compactcatalog/listening_context_evidence.go:305` | ON DELETE CASCADE from catalog_assets, catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/listening_context_evidence.go:228 |
| `catalog_book_series_contrib` | `server/internal/compactcatalog/listening_context_evidence.go:297` | ON DELETE CASCADE from catalog_assets, catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/listening_context_evidence.go:225 |
| `catalog_book_evidence_jobs` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/listening_context_evidence.go:41 |
| `catalog_book_groups` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/library_cleanup.go:53 |
| `catalog_personal_sort_state` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `catalog_fanouts` | `server/internal/compactcatalog/derive_browse_sorts.go:48` | Direct delete: server/internal/compactcatalog/derive_browse_sorts.go:64 |
| `catalog_rec_facets` | `server/internal/compactcatalog/derive_rec_facets.go:148` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/derive_rec_facets.go:145 |
| `catalog_facet_members` | `server/internal/compactcatalog/derive_facet_counts.go:151` | Direct delete: server/internal/compactcatalog/derive_facet_counts.go:140 |
| `catalog_facet_counts` | `server/internal/compactcatalog/derive_facet_counts.go:162` | Direct delete: server/internal/compactcatalog/derive_facet_counts.go:170 |
| `catalog_container_added` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/persistence/migrations/0001_baseline.sql:5331 |
| `catalog_rec_postings` | `server/internal/compactcatalog/derive_rec_facets.go:279` | Direct delete: server/internal/compactcatalog/derive_rec_facets.go:268 |
| `catalog_rec_df` | `server/internal/compactcatalog/derive_rec_facets.go:184` | Direct delete: server/internal/compactcatalog/derive_rec_facets.go:188 |
| `catalog_similar` | `server/internal/catalogtest/taste.go:186` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/screen_links.go:72 |
| `catalog_external_ids` | `server/internal/catalogtest/taste.go:164` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/screen_links.go:88 |
| `rec_profile_jobs` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/rec_profile.go:214 |
| `rec_profile_signals` | `server/internal/compactcatalog/rec_profile.go:260` | Direct delete: server/internal/compactcatalog/rec_profile.go:258 |
| `rec_profile_taste` | `server/internal/compactcatalog/rec_profile.go:272` | Direct delete: server/internal/compactcatalog/rec_reset.go:28 |
| `rec_data_revision` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `rec_profile_revisions` | `server/internal/compactcatalog/rec_profile.go:286` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `enrichment_domains` | `server/internal/enrichment/enrichment.go:486` | Fixed installed enrichment-domain keys |
| `catalog_dataset_works` | `server/internal/enrichment/enrichment.go:635` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/enrichment/enrichment.go:628 |
| `catalog_dataset_tags` | `server/internal/enrichment/enrichment.go:797` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/enrichment/enrichment.go:628 |
| `catalog_dataset_vocabulary` | `server/internal/enrichment/enrichment.go:548` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `enrichment_jobs` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/enrichment/enrichment.go:589 |
| `catalog_browse_blocks` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/library_cleanup.go:54 |
| `compact_visibility_blocks` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/library_cleanup.go:44 |
| `configuration` | `server/internal/persistence/schema.go:162` | Compiled configuration keys; replaces the same key |
| `accounts` | `server/internal/identity/self_registration.go:181` | Direct delete: server/internal/identity/member_erasure.go:204 |
| `libraries` | `server/internal/catalogtest/catalogtest.go:93` | Direct delete: server/internal/catalog/library_removal.go:329 |
| `metadata_jobs` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `provider_evidence` | `server/internal/metadata/tvdb_publication_apply.go:338` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/repair_retry.go:56 |
| `jobs` | `server/internal/ingestion/service.go:144` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; Direct delete: server/internal/ingestion/retention.go:122 |
| `scan_queue` | `Baseline/trigger/subsystem declaration` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; Direct delete: server/internal/catalog/library_removal.go:260 |
| `playback_sessions` | `server/internal/playback/service.go:441` | Direct delete: server/internal/playback/service.go:824 |
| `progress_activity` | `server/internal/playback/service.go:679` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; ON DELETE CASCADE from progress; baseline foreign keys; Direct delete: server/internal/catalog/personal_activity.go:424 |
| `playback_artifacts` | `server/internal/playback/hls.go:323` | Direct delete: server/internal/playback/hls.go:1103 |
| `playback_requests` | `server/internal/playback/service.go:543` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `progress` | `server/internal/playback/personal_progress.go:76` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/title_merge.go:206 |
| `library_network_policy` | `server/internal/playback/remote.go:83` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `remote_playback` | `server/internal/playback/service.go:474` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `book_resume` | `server/internal/playback/personal_progress.go:83` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/library_removal.go:361 |
| `audio_tag_evidence` | `server/internal/catalog/audio_evidence.go:86` | Direct delete: server/internal/catalog/library_removal.go:308 |
| `audio_source_metadata` | `server/internal/catalog/audio_evidence.go:115` | Direct delete: server/internal/catalog/library_removal.go:309 |
| `episode_asset_boundaries` | `server/internal/catalog/episode_repository.go:216` | ON DELETE CASCADE from catalog_assets, catalog_entities; baseline foreign keys; Direct delete: server/internal/persistence/migrations/0001_baseline.sql:5008 |
| `episodic_sources` | `server/internal/catalog/episode_repository.go:105` | Direct delete: server/internal/catalog/library_removal.go:310 |
| `policy` | `server/internal/hosted/legacy_policy_development.go:55` | Direct delete: server/internal/hosted/trust.go:126 |
| `restriction_outbox` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `restrictions` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `library_revisions` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `viewer_revisions` | `server/internal/catalog/container_undo.go:88` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from libraries; baseline foreign keys |
| `personal_receipts` | `server/internal/catalog/personal.go:553` | 30-day receipts; unresolved conflict evidence stays while its conflict exists; server/internal/catalog/personal.go:559; ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `metadata_details` | `server/internal/catalogtest/taste.go:180` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/repair_retry.go:56 |
| `metadata_ratings` | `server/internal/catalogtest/taste.go:208` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/repair_retry.go:56 |
| `detail_jobs` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `playlist_shares` | `server/internal/catalog/playlists.go:318` | ON DELETE CASCADE from catalog_playlists; baseline foreign keys; Direct delete: server/internal/identity/direct_profiles.go:580 |
| `playlist_receipts` | `server/internal/catalog/playlists.go:339` | 30-day receipts; unresolved conflict evidence stays while its conflict exists; server/internal/catalog/personal.go:559; Direct delete: server/internal/identity/direct_profiles.go:582 |
| `playlist_directory_revisions` | `Baseline/trigger/subsystem declaration` | Primary-key resource ceiling; no age expiry of revision evidence. |
| `metadata_provider_cooldowns` | `server/internal/persistence/metadata_discovery.go:316` | Fixed first-party provider identifiers; upsert per provider |
| `tvdb_jobs` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `tvdb_series_candidates` | `server/internal/metadata/screen_publish.go:772` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/tvdb_publication_stage.go:152 |
| `tvdb_episode_evidence` | `server/internal/metadata/tvdb_publication_stage.go:329` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/tvdb_publication.go:224 |
| `tvdb_episode_links` | `server/internal/metadata/tvdb_publication_apply.go:350` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/repair_retry.go:84 |
| `admin_revision` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `library_configuration` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `job_observations` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from jobs; baseline foreign keys |
| `metadata_publication_heads` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `metadata_provider_policies` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `metadata_work` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `metadata_publication_operations` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `metadata_operation_identities` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from metadata_publication_operations; baseline foreign keys |
| `metadata_operation_sources` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from metadata_publication_operations; baseline foreign keys |
| `metadata_stage_movies` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from metadata_publication_operations; baseline foreign keys |
| `metadata_stage_genres` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from metadata_publication_operations; baseline foreign keys |
| `metadata_stage_credits` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from metadata_publication_operations; baseline foreign keys |
| `mb_jobs` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `mb_album_heads` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `mb_song_heads` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `mb_provider_policies` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `mb_recording_evidence` | `server/internal/metadata/mb_evidence_store.go:107` | Direct delete: server/internal/metadata/mb_lifecycle.go:98 |
| `mb_release_evidence` | `server/internal/metadata/mb_evidence_store.go:266` | Direct delete: server/internal/metadata/mb_lifecycle.go:95 |
| `mb_recording_credits` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from mb_recording_evidence; baseline foreign keys |
| `mb_release_credits` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from mb_release_evidence; baseline foreign keys |
| `mb_release_group_types` | `server/internal/metadata/mb_evidence_store.go:273` | ON DELETE CASCADE from mb_release_evidence; baseline foreign keys |
| `mb_release_media` | `server/internal/metadata/mb_evidence_store.go:278` | ON DELETE CASCADE from mb_release_evidence; baseline foreign keys |
| `mb_release_tracks` | `server/internal/metadata/mb_evidence_store.go:290` | ON DELETE CASCADE from mb_release_media; baseline foreign keys |
| `mb_track_credits` | `server/internal/metadata/mb_evidence_store.go:17` | ON DELETE CASCADE from mb_release_tracks; baseline foreign keys |
| `mb_recording_aliases` | `server/internal/metadata/mb_publish.go:120` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from mb_recording_evidence; baseline foreign keys |
| `mb_release_aliases` | `server/internal/metadata/mb_publish.go:215` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from mb_release_evidence; baseline foreign keys |
| `mb_album_links` | `server/internal/metadata/mb_publish.go:222` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `mb_song_links` | `server/internal/metadata/mb_publish.go:123` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `mb_publication_operations` | `server/internal/metadata/mb_claim.go:353` | ON DELETE CASCADE from mb_jobs; baseline foreign keys; Direct delete: server/internal/metadata/mb_lifecycle.go:87 |
| `mb_operation_sources` | `server/internal/metadata/mb_claim.go:357` | ON DELETE CASCADE from mb_publication_operations; baseline foreign keys |
| `mb_operation_artists` | `server/internal/metadata/mb_claim.go:362` | ON DELETE CASCADE from mb_publication_operations; baseline foreign keys |
| `mb_candidates` | `server/internal/metadata/repair.go:675` | ON DELETE CASCADE from mb_jobs; baseline foreign keys; Direct delete: server/internal/metadata/mb_candidates.go:57 |
| `mb_candidate_reasons` | `server/internal/metadata/repair.go:679` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from mb_candidates; baseline foreign keys |
| `mb_selection_receipts` | `server/internal/metadata/mb_review.go:261` | ON DELETE CASCADE from mb_jobs; baseline foreign keys; Direct delete: server/internal/metadata/mb_review.go:258 |
| `mb_album_reconciliations` | `server/internal/metadata/mb_publish.go:225` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `mb_album_hint_scans` | `server/internal/metadata/mb_claim.go:237` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `audio_metadata_policies` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `audio_local_evidence` | `server/internal/catalog/audio_evidence.go:65` | ON DELETE CASCADE from catalog_assets, libraries; baseline foreign keys; Direct delete: server/internal/catalog/audio_evidence.go:69 |
| `music_acoustid_health` | `server/internal/metadata/music_acquisition.go:58` | CHECK(singleton=1) |
| `music_match_observations` | `server/internal/metadata/mb_candidates.go:70` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from mb_jobs; baseline foreign keys |
| `tvdb_publication_heads` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `tvdb_provider_policies` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `tvdb_publication_sets` | `server/internal/metadata/tvdb_publication.go:230` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/screen_publish.go:790 |
| `tvdb_publication_pages` | `server/internal/metadata/tvdb_publication_stage.go:318` | ON DELETE CASCADE from tvdb_publication_sets; baseline foreign keys |
| `tvdb_evidence_page_ownership` | `server/internal/metadata/tvdb_publication_stage.go:332` | ON DELETE CASCADE from tvdb_episode_evidence, tvdb_publication_pages, tvdb_publication_sets; baseline foreign keys |
| `tvdb_projection_coverage` | `server/internal/metadata/tvdb_publication_apply.go:363` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/tvdb_publication.go:218 |
| `tvdb_publication_operations` | `server/internal/metadata/tvdb_publication_apply.go:243` | Direct delete: server/internal/metadata/tvdb_publication.go:20 |
| `tvdb_projection_targets` | `server/internal/metadata/tvdb_publication_apply.go:140` | ON DELETE CASCADE from tvdb_publication_operations; baseline foreign keys |
| `tvdb_publication_candidates` | `server/internal/metadata/tvdb_publication_stage.go:69` | ON DELETE CASCADE from tvdb_publication_operations; baseline foreign keys |
| `tvdb_publication_candidate_aliases` | `server/internal/metadata/tvdb_publication_stage.go:73` | ON DELETE CASCADE from tvdb_publication_candidates; baseline foreign keys |
| `metadata_movie_candidates` | `server/internal/metadata/repair.go:659` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `metadata_movie_selections` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/repair_retry.go:56 |
| `metadata_owner_fields` | `server/internal/metadata/repair_fields.go:346` | Direct delete: server/internal/catalog/manual_metadata.go:194 |
| `metadata_owner_history` | `server/internal/metadata/repair.go:373` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies; Direct delete: server/internal/metadata/repair.go:383 |
| `metadata_relationship_decisions` | `server/internal/metadata/repair_relationship_edits.go:54` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies; Direct delete: server/internal/persistence/migrations/0001_baseline.sql:5799 |
| `metadata_repair_cascades` | `server/internal/metadata/repair_relationships.go:283` | Direct delete: server/internal/metadata/repair_relationships.go:280 |
| `metadata_repair_cascade_items` | `server/internal/metadata/repair_relationships.go:350` | ON DELETE CASCADE from metadata_repair_cascades; baseline foreign keys |
| `artwork_entity_heads` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/persistence/migrations/0001_baseline.sql:5478 |
| `artwork_objects` | `server/internal/metadata/artwork_worker.go:294` | Direct delete: server/internal/metadata/artwork_compaction.go:132 |
| `artwork_candidates` | `server/internal/metadata/artwork_state.go:237` | Direct delete: server/internal/metadata/artwork_upload.go:221 |
| `artwork_selections` | `server/internal/metadata/artwork_upload.go:130` | Direct delete: server/internal/metadata/artwork_upload.go:203 |
| `artwork_jobs` | `server/internal/metadata/artwork_state.go:265` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies; Direct delete: server/internal/persistence/migrations/0001_baseline.sql:5801 |
| `artwork_previews` | `server/internal/metadata/artwork_upload.go:122` | ON DELETE CASCADE from artwork_candidates; baseline foreign keys; Direct delete: server/internal/metadata/artwork_upload.go:221 |
| `artwork_discovery` | `server/internal/metadata/artwork_discovery.go:180` | Direct delete: server/internal/metadata/artwork_discovery.go:276 |
| `artwork_dirty` | `server/internal/metadata/tvdb_publication_apply.go:345` | Direct delete: server/internal/metadata/artwork_discovery.go:53 |
| `playback_origin_roots` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from libraries; baseline foreign keys; Direct delete: server/internal/persistence/migrations/0001_baseline.sql:4374 |
| `playback_origin_assets` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_assets; baseline foreign keys |
| `playback_origin_items` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `playback_origin_associations` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_assets, catalog_entities; baseline foreign keys; Direct delete: server/internal/persistence/migrations/0001_baseline.sql:5009 |
| `playback_origin_shows` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `playback_origin_seasons` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `playback_origin_books` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `playback_origin_albums` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `asset_stream_facts` | `server/internal/assets/streams.go:98` | ON DELETE CASCADE from catalog_assets; baseline foreign keys |
| `asset_streams` | `server/internal/assets/streams.go:106` | ON DELETE CASCADE from asset_stream_facts; baseline foreign keys; Direct delete: server/internal/assets/streams.go:102 |
| `playback_hls_reservations` | `server/internal/playback/audio_plan.go:60` | Direct delete: server/internal/playback/hls.go:1106 |
| `asset_subtitle_facts` | `server/internal/subtitles/persist.go:33` | ON DELETE CASCADE from catalog_assets; baseline foreign keys |
| `subtitle_sidecars` | `server/internal/subtitles/persist.go:50` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `asset_subtitles` | `server/internal/subtitles/persist.go:55` | ON DELETE CASCADE from asset_subtitle_facts; baseline foreign keys; Direct delete: server/internal/subtitles/persist.go:40 |
| `subtitle_collections` | `server/internal/subtitles/service.go:727` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `subtitle_resources` | `server/internal/subtitles/persist.go:98` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `subtitle_revisions` | `server/internal/subtitles/persist.go:101` | ON DELETE CASCADE from subtitle_resources; baseline foreign keys; Direct delete: server/internal/subtitles/service.go:841 |
| `subtitle_render_revisions` | `server/internal/subtitles/persist.go:104` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from subtitle_revisions; baseline foreign keys |
| `subtitle_original_revisions` | `server/internal/subtitles/service.go:631` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from subtitle_revisions; baseline foreign keys |
| `subtitle_original_files` | `server/internal/subtitles/original_files.go:75` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from subtitle_revisions; baseline foreign keys |
| `subtitle_inventory_evidence` | `server/internal/subtitles/persist.go:37` | ON DELETE CASCADE from catalog_assets; baseline foreign keys |
| `subtitle_remote_sessions` | `server/internal/playback/service.go:457` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `playback_subtitle_presentations` | `server/internal/subtitles/session.go:316` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `subtitle_operations` | `server/internal/subtitles/service.go:422` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `subtitle_provider_candidates` | `server/internal/subtitles/provider.go:400` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/subtitles/provider.go:391 |
| `playback_subtitle_state` | `server/internal/subtitles/session.go:37` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `subtitle_selection_operations` | `server/internal/subtitles/session.go:329` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `playback_manifest_subtitles` | `server/internal/subtitles/session.go:273` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `playback_subtitle_pins` | `server/internal/subtitles/session.go:269` | ON DELETE CASCADE from playback_sessions; baseline foreign keys; Direct delete: server/internal/subtitles/service.go:814 |
| `playback_observations` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `playback_admin_operations` | `server/internal/playback/activity.go:305` | Direct delete: server/internal/playback/activity.go:275 |
| `playback_delivery_plans` | `server/internal/playback/delivery_plan.go:1189` | ON DELETE CASCADE from playback_sessions; baseline foreign keys; Direct delete: server/internal/playback/hls_produce_support.go:286 |
| `playback_copy_timelines` | `server/internal/playback/hls_copy_timeline.go:303` | ON DELETE CASCADE from catalog_assets; baseline foreign keys |
| `playback_hls_windows` | `server/internal/playback/hls.go:529` | ON DELETE CASCADE from playback_sessions; baseline foreign keys; Direct delete: server/internal/playback/subtitle_delivery.go:91 |
| `playback_hls_demand` | `server/internal/playback/hls.go:761` | ON DELETE CASCADE from playback_sessions; baseline foreign keys; Direct delete: server/internal/playback/hls_restart.go:62 |
| `playback_hls_stages` | `server/internal/playback/hls.go:795` | ON DELETE CASCADE from playback_sessions; baseline foreign keys; Direct delete: server/internal/playback/subtitle_delivery.go:94 |
| `playback_vod_plans` | `server/internal/playback/finite_hls.go:173` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `playback_vod_intervals` | `server/internal/playback/finite_hls.go:178` | ON DELETE CASCADE from playback_vod_plans; baseline foreign keys |
| `asset_chapter_facts` | `server/internal/assets/chapters.go:38` | ON DELETE CASCADE from catalog_assets; baseline foreign keys |
| `asset_chapters` | `server/internal/assets/chapters.go:46` | ON DELETE CASCADE from asset_chapter_facts; baseline foreign keys; Direct delete: server/internal/assets/chapters.go:42 |
| `playback_source_pins` | `server/internal/playback/service.go:445` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `authorization_session_families` | `server/internal/identity/families.go:146` | Direct delete: server/internal/identity/prune_sessions.go:108 |
| `authorization_family_tokens` | `server/internal/identity/families.go:152` | Direct delete: server/internal/identity/prune_sessions.go:28 |
| `playback_owner_policy` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `playback_marker_skips` | `server/internal/playbackv1/skips.go:46` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `playback_preparation_episode_bindings` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `playback_preparation_book_bindings` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `playback_source_versions` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `playback_observed_acquisitions` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `playback_source_observations` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `playback_preparation_dependencies` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `media_analyses` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `media_analysis_dependencies` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `media_analysis_formats` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `media_analysis_streams` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `media_analysis_chunks` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `media_analysis_item_mappings` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `library_sources` | `server/internal/catalog/library_sources.go:334` | Direct delete: server/internal/catalog/library_removal.go:328 |
| `library_scan_policies` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/catalog/library_removal.go:327 |
| `source_strm_analysis` | `server/internal/remotesources/policy.go:66` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `inventory_policy_pending` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/ingestion/service.go:314 |
| `inventory_runs` | `server/internal/ingestion/service.go:152` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; Direct delete: server/internal/catalog/library_removal.go:306 |
| `inventory_source_active` | `server/internal/ingestion/service.go:160` | Direct delete: server/internal/ingestion/service.go:222 |
| `inventory_directories` | `server/internal/ingestion/service.go:156` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; Direct delete: server/internal/ingestion/targeted.go:322 |
| `inventory_objects` | `server/internal/catalog/inventory.go:394` | Direct delete: server/internal/catalog/library_removal.go:237 |
| `inventory_changes` | `server/internal/ingestion/inventory.go:298` | Direct delete: server/internal/ingestion/retention.go:91 |
| `inventory_scan_issues` | `server/internal/catalog/inventory_issues.go:16` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; ON DELETE CASCADE from jobs; baseline foreign keys |
| `inventory_video_names` | `server/internal/catalog/video_names.go:48` | ON DELETE CASCADE from catalog_assets; baseline foreign keys; Direct delete: server/internal/catalog/video_names.go:63 |
| `inventory_video_ids` | `server/internal/catalog/video_names.go:59` | ON DELETE CASCADE from inventory_video_names; baseline foreign keys; Direct delete: server/internal/catalog/video_names.go:55 |
| `inventory_sidecars` | `server/internal/catalog/inventory_sidecars.go:37` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; ON DELETE CASCADE from library_sources; baseline foreign keys; Direct delete: server/internal/ingestion/sidecar_changes.go:74 |
| `inventory_sidecar_changes` | `server/internal/ingestion/sidecar_changes.go:71` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; ON DELETE CASCADE from jobs; baseline foreign keys; Direct delete: server/internal/ingestion/sidecar_changes.go:136 |
| `inventory_operation_jobs` | `server/internal/ingestion/operation_group.go:18` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; Direct delete: server/internal/catalog/library_removal.go:305 |
| `inventory_analysis_queue` | `server/internal/ingestion/deep_analysis.go:100` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; Direct delete: server/internal/catalog/library_removal.go:234 |
| `inventory_version_links` | `server/internal/catalog/inventory_admin.go:345` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/administration/version_deletion.go:174 |
| `dvr_catalog_retirements` | `server/internal/livechannels/dvr/retention.go:99` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `analysis_probe_bindings` | `server/internal/catalog/inventory.go:742` | Added indexed 1-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `analysis_results` | `server/internal/mediaanalysis/work.go:370` | Owner RetainDays; retire changed sources; remove summaries only after artifact/head references disappear; 32 rows per turn; server/internal/mediaanalysis/retention.go; Direct delete: server/internal/mediaanalysis/retention.go:204 |
| `analysis_heads` | `server/internal/mediaanalysis/work.go:383` | Direct delete: server/internal/mediaanalysis/retention.go:54 |
| `analysis_artifacts` | `server/internal/mediaanalysis/work.go:374` | Direct delete: server/internal/mediaanalysis/retention.go:184 |
| `analysis_marker_sets` | `server/internal/mediaanalysis/view.go:157` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `analysis_markers` | `server/internal/mediaanalysis/work.go:390` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `analysis_marker_history` | `server/internal/mediaanalysis/markers.go:146` | Added indexed 90-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `analysis_marker_receipts` | `server/internal/mediaanalysis/markers.go:150` | Direct delete: server/internal/mediaanalysis/retention.go:112 |
| `analysis_stage_runs` | `server/internal/mediaanalysis/work.go:147` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17 |
| `console_documents` | `server/internal/operations/evidence.go:219` | Direct delete: server/internal/operations/evidence.go:509 |
| `console_receipts` | `server/internal/operations/store.go:199` | Direct delete: server/internal/operations/evidence.go:502 |
| `console_audit` | `server/internal/operations/store.go:220` | Direct delete: server/internal/operations/evidence.go:539 |
| `console_records` | `server/internal/operations/evidence.go:83` | Direct delete: server/internal/operations/evidence.go:90 |
| `console_alerts` | `server/internal/operations/evidence.go:421` | Direct delete: server/internal/operations/evidence.go:504 |
| `console_exports` | `server/internal/operations/evidence.go:335` | Direct delete: server/internal/identity/profile_erasure.go:105 |
| `console_reports` | `server/internal/operations/feedback.go:147` | Direct delete: server/internal/operations/evidence.go:506 |
| `console_report_events` | `server/internal/operations/feedback.go:150` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from console_reports; baseline foreign keys |
| `console_notifications` | `server/internal/operations/feedback.go:338` | Direct delete: server/internal/operations/evidence.go:507 |
| `console_operations` | `server/internal/operations/jobs.go:249` | Direct delete: server/internal/operations/evidence.go:508 |
| `console_schedules` | `server/internal/operations/jobs.go:848` | 100 schedules, enforced by operations/jobs.go:827 |
| `console_settings_revision` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `dvr_recording_shares` | `server/internal/livechannels/dvr/shared.go:31` | ON DELETE CASCADE from dvr_recordings; baseline foreign keys; Direct delete: server/internal/livechannels/dvr/shared.go:46 |
| `live_import_diagnostics` | `server/internal/livechannels/store.go:272` | Superseded generations after grace, current/source-dependent generations protected, rows dismantled in batches; server/internal/livechannels/generation_retention.go:24; ON DELETE CASCADE from live_generations; baseline foreign keys |
| `live_channel_attributes` | `server/internal/livechannels/store.go:324` | Superseded generations after grace, current/source-dependent generations protected, rows dismantled in batches; server/internal/livechannels/generation_retention.go:24; ON DELETE CASCADE from live_generations; baseline foreign keys; Direct delete: server/internal/livechannels/generation_retention.go:57 |
| `dvr_private_libraries` | `server/internal/livechannels/dvr/publication.go:87` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `dvr_catalog_provenance` | `server/internal/livechannels/dvr/publication.go:181` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `continue_dismissals` | `server/internal/catalog/personal.go:504` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/personal.go:506 |
| `personal_evidence_fences` | `server/internal/catalog/personal_activity.go:410` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `personal_completed_plays` | `server/internal/catalog/play_history.go:116` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `personal_history` | `server/internal/catalogtest/taste.go:445` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/personal_activity.go:440 |
| `personal_activity_revisions` | `server/internal/catalog/personal_activity.go:443` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `saved_operation_fences` | `server/internal/catalog/personal.go:151` | Added primary-key ceiling 10,000,000; baseline bound triggers and tableretention/bounds.go |
| `personal_snapshots` | `server/internal/catalog/personal.go:126` | 30-day receipts; unresolved conflict evidence stays while its conflict exists; server/internal/catalog/personal.go:559; ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `personal_device_sequences` | `server/internal/catalog/personal.go:495` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `personal_explicit_events` | `server/internal/catalog/personal.go:484` | 30-day receipts; unresolved conflict evidence stays while its conflict exists; server/internal/catalog/personal.go:559; ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/personal.go:567 |
| `personal_conflicts` | `server/internal/catalog/personal.go:490` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `personal_activity_receipts` | `server/internal/catalog/rec_feedback.go:85` | 30-day receipts; unresolved conflict evidence stays while its conflict exists; server/internal/catalog/personal.go:559 |
| `saved_resources` | `server/internal/catalog/resources.go:539` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go; Direct delete: server/internal/identity/direct_profiles.go:584 |
| `saved_resource_shares` | `server/internal/catalog/resources.go:626` | ON DELETE CASCADE from saved_resources; baseline foreign keys; Direct delete: server/internal/identity/direct_profiles.go:581 |
| `saved_resource_entries` | `server/internal/catalog/bulk_commands.go:233` | ON DELETE CASCADE from saved_resources; baseline foreign keys; Direct delete: server/internal/catalog/title_merge.go:107 |
| `saved_resource_receipts` | `server/internal/catalog/resource_pins.go:94` | 30-day receipts; unresolved conflict evidence stays while its conflict exists; server/internal/catalog/personal.go:559 |
| `saved_pins` | `server/internal/catalog/resource_pins.go:87` | Direct delete: server/internal/catalog/resources.go:567 |
| `saved_resource_generation` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `playback_legacy_replacement_requests` | `server/internal/playback/service.go:547` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `playback_physical_source_pins` | `server/internal/playback/service.go:551` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `subtitle_inventory_entries` | `server/internal/subtitles/managed_inventory.go:23` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; ON DELETE CASCADE from jobs; baseline foreign keys |
| `playback_vod_retries` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `playback_vod_subtitle_intents` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `playback_vod_observations` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `playback_personal_intents` | `server/internal/playback/personal_progress.go:14` | Added primary-key ceiling 10,000,000; baseline bound triggers and tableretention/bounds.go |
| `playback_personal_claims` | `server/internal/playback/personal_progress.go:46` | Added primary-key ceiling 10,000,000; baseline bound triggers and tableretention/bounds.go; Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40 |
| `playback_personal_fences` | `server/internal/catalog/personal.go:167` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `playback_personal_profile_fences` | `server/internal/identity/profile_erasure.go:69` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `lyric_resources` | `server/internal/lyrics/local.go:97` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `lyric_revisions` | `server/internal/lyrics/local.go:103` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from lyric_resources; baseline foreign keys |
| `lyric_selections` | `server/internal/lyrics/service.go:315` | ON DELETE CASCADE from playback_sessions; baseline foreign keys; Direct delete: server/internal/catalog/purge.go:54 |
| `lyric_candidates` | `server/internal/lyrics/acquisition.go:257` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/lyrics/acquisition.go:235 |
| `lyric_fetch_runs` | `server/internal/lyrics/bulk.go:148` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from libraries; baseline foreign keys |
| `metadata_runtime_settings` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `screen_metadata_consent` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `screen_metadata_policies` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `screen_metadata_work` | `server/internal/catalogtest/taste.go:139` | ON DELETE CASCADE from libraries; baseline foreign keys; Direct delete: server/internal/persistence/migrations/0001_baseline.sql:5850 |
| `screen_metadata_candidates` | `server/internal/metadata/screen_publish.go:59` | ON DELETE CASCADE from screen_metadata_work; baseline foreign keys; Direct delete: server/internal/metadata/screen_publish.go:50 |
| `screen_metadata_publications` | `server/internal/metadata/screen_publish.go:182` | ON DELETE CASCADE from screen_metadata_work; baseline foreign keys; Direct delete: server/internal/metadata/screen_publish.go:103 |
| `screen_metadata_attention` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities, libraries, screen_metadata_work; baseline foreign keys; Direct delete: server/internal/persistence/migrations/0001_baseline.sql:2319 |
| `screen_metadata_attention_counts` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `screen_metadata_fields` | `server/internal/catalogtest/taste.go:148` | ON DELETE CASCADE from screen_metadata_work; baseline foreign keys; Direct delete: server/internal/metadata/screen_repair.go:186 |
| `video_nfo_evidence` | `server/internal/localmetadata/video_store.go:38` | ON DELETE CASCADE from catalog_assets, libraries; baseline foreign keys; Direct delete: server/internal/localmetadata/video_store.go:15 |
| `video_nfo_status` | `server/internal/localmetadata/video_store.go:44` | ON DELETE CASCADE from catalog_assets, libraries; baseline foreign keys |
| `screen_anime_seasons` | `server/internal/metadata/screen_review.go:704` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/screen_repair.go:114 |
| `metadata_repair_work_receipts` | `server/internal/metadata/repair_relationships.go:354` | ON DELETE CASCADE from metadata_repair_cascade_items; baseline foreign keys |
| `metadata_discovery_items` | `server/internal/persistence/metadata_discovery.go:243` | Direct delete: server/internal/persistence/metadata_discovery.go:167 |
| `metadata_discovery_refresh` | `server/internal/persistence/metadata_discovery.go:249` | Direct delete: server/internal/persistence/metadata_discovery.go:170 |
| `metadata_tags` | `server/internal/catalogtest/taste.go:217` | Direct delete: server/internal/metadata/screen_publish.go:545 |
| `audio_genre_projection` | `server/internal/compactcatalog/local_genres.go:108` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `metadata_document_freshness` | `server/internal/metadata/freshness.go:155` | Direct delete: server/internal/metadata/freshness.go:170 |
| `metadata_refresh_budget` | `server/internal/metadata/freshness.go:220` | Fixed provider/span windows; spend is overwritten |
| `prepared_media_source_revisions` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_assets; baseline foreign keys |
| `prepared_media_jobs` | `server/internal/preparedmedia/service.go:287` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `prepared_media_versions` | `server/internal/preparedmedia/worker.go:542` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `prepared_media_session_pins` | `server/internal/preparedmedia/artifacts.go:107` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `hosted_profile_revisions` | `server/internal/hosted/profile_policy.go:28` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `hosted_profile_erasure_receipts` | `server/internal/hosted/profile_policy.go:61` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `direct_profiles` | `server/internal/identity/direct_profiles.go:119` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from accounts; baseline foreign keys |
| `direct_profile_trust` | `server/internal/identity/direct_profiles.go:474` | ON DELETE CASCADE from direct_profiles; baseline foreign keys; Direct delete: server/internal/identity/pin_recovery.go:121 |
| `profile_restrictions` | `server/internal/identity/restrictions.go:313` | ON DELETE CASCADE from direct_profiles; baseline foreign keys |
| `profile_avatar_objects` | `server/internal/identity/avatars.go:221` | Direct delete: server/internal/identity/avatars.go:245 |
| `profile_avatars` | `server/internal/identity/avatars.go:226` | ON DELETE CASCADE from direct_profiles; baseline foreign keys; Direct delete: server/internal/identity/avatars.go:333 |
| `identity_devices` | `server/internal/identity/api_credentials.go:167` | Direct delete: server/internal/identity/member_erasure.go:194 |
| `identity_device_families` | `server/internal/identity/families.go:149` | ON DELETE CASCADE from identity_devices; baseline foreign keys; Direct delete: server/internal/identity/prune_sessions.go:107 |
| `identity_browser_accounts` | `server/internal/identity/browser_accounts.go:61` | ON DELETE CASCADE from accounts; baseline foreign keys; Direct delete: server/internal/identity/devices.go:414 |
| `identity_account_factors` | `server/internal/identity/two_factor.go:215` | ON DELETE CASCADE from accounts; baseline foreign keys; Direct delete: server/internal/identity/two_factor.go:312 |
| `identity_recovery_codes` | `server/internal/identity/two_factor.go:222` | ON DELETE CASCADE from accounts; baseline foreign keys; Direct delete: server/internal/identity/two_factor.go:218 |
| `identity_factor_challenges` | `server/internal/identity/two_factor.go:338` | ON DELETE CASCADE from accounts; baseline foreign keys; Direct delete: server/internal/persistence/maintenance.go:18 |
| `topshelf_tokens` | `server/internal/identity/topshelf.go:66` | Direct delete: server/internal/persistence/maintenance.go:19 |
| `content_rating_ages` | `server/internal/compactcatalog/ratings.go:51` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `listening_book_groups` | `server/internal/catalog/listening_groups.go:49` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `search_history` | `server/internal/catalog/search_history.go:48` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; Direct delete: server/internal/catalog/search_history.go:51 |
| `library_navigation` | `server/internal/catalog/library_pins.go:200` | ON DELETE CASCADE from libraries; baseline foreign keys; Direct delete: server/internal/catalog/library_pins.go:196 |
| `library_navigation_revisions` | `server/internal/catalog/library_pins.go:205` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `saved_pin_order` | `server/internal/catalog/library_pins.go:114` | Direct delete: server/internal/catalog/library_pins.go:110 |
| `saved_pin_order_revisions` | `server/internal/catalog/library_pins.go:119` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `item_extras` | `server/internal/catalog/extras.go:96` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `personal_batch_receipts` | `server/internal/catalog/personal_batch.go:194` | 30-day receipts; unresolved conflict evidence stays while its conflict exists; server/internal/catalog/personal.go:559 |
| `telemetry_rollups` | `server/internal/telemetry/collector.go:295` | Direct delete: server/internal/telemetry/collector.go:302 |
| `social_groups` | `server/internal/social/groups.go:304` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `social_group_members` | `server/internal/social/groups.go:308` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from social_groups; baseline foreign keys |
| `social_group_invites` | `server/internal/social/invites.go:89` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from social_groups; baseline foreign keys |
| `social_group_queue` | `server/internal/social/queue.go:313` | ON DELETE CASCADE from social_groups; baseline foreign keys; Direct delete: server/internal/social/queue.go:289 |
| `social_group_receipts` | `server/internal/social/transport.go:164` | ON DELETE CASCADE from social_groups; baseline foreign keys; Direct delete: server/internal/social/transport.go:168 |
| `social_group_events` | `server/internal/social/events.go:101` | ON DELETE CASCADE from social_groups; baseline foreign keys; Direct delete: server/internal/social/events.go:104 |
| `social_receivers` | `server/internal/social/cast.go:285` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `social_receiver_grants` | `server/internal/social/receivers.go:394` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from social_receivers; baseline foreign keys |
| `social_handoffs` | `server/internal/social/handoff.go:257` | New primary-key ceiling 10,000,000; tableretention/bounds.go and baseline triggers; ON DELETE CASCADE from social_receivers; baseline foreign keys |
| `social_cast_bootstraps` | `server/internal/social/cast.go:197` | Added indexed 7-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `social_cast_devices` | `server/internal/social/cast.go:332` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `social_rate` | `server/internal/social/store.go:277` | Added indexed 1-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `artwork_uploads` | `server/internal/metadata/artwork_upload.go:119` | ON DELETE CASCADE from artwork_candidates; baseline foreign keys; Direct delete: server/internal/metadata/artwork_upload.go:221 |
| `notification_revision` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `notification_inbox` | `server/internal/notify/notify.go:286` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `notification_records` | `server/internal/notify/notify.go:351` | ON DELETE CASCADE from notification_inbox; baseline foreign keys; Direct delete: server/internal/operations/notifications.go:888 |
| `feedback_details` | `server/internal/operations/feedback_reports.go:432` | ON DELETE CASCADE from console_reports; baseline foreign keys |
| `download_settings` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `download_receipt_keys` | `server/internal/downloads/receipts.go:153` | Added primary-key ceiling 10,000; baseline bound triggers and tableretention/bounds.go |
| `download_preparations` | `server/internal/downloads/preparations.go:212` | Direct delete: server/internal/downloads/worker.go:267 |
| `download_grants` | `server/internal/downloads/grants.go:195` | ON DELETE CASCADE from download_preparations; baseline foreign keys; Direct delete: server/internal/downloads/preparations.go:291 |
| `download_receipts` | `server/internal/downloads/receipts.go:353` | Direct delete: server/internal/downloads/worker.go:273 |
| `download_receipt_sequence` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `download_progress_marks` | `server/internal/downloads/progress.go:186` | Direct delete: server/internal/downloads/worker.go:274 |
| `download_source_digests` | `server/internal/downloads/source_digest.go:47` | Direct delete: server/internal/downloads/worker.go:275 |
| `admin_documents` | `server/internal/administration/service.go:315` | Direct delete: server/internal/catalog/library_removal.go:313 |
| `admin_receipts` | `server/internal/administration/service.go:352` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `admin_dvr_groups` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/administration/dvr_runtime.go:229 |
| `admin_channel_presets` | `server/internal/administration/library_channels.go:357` | Direct delete: server/internal/administration/library_channels.go:404 |
| `admin_channel_logos` | `server/internal/administration/library_channels.go:534` | Added primary-key ceiling 100,000; baseline bound triggers and tableretention/bounds.go |
| `admin_channel_numbers` | `server/internal/administration/live_sources.go:487` | Direct delete: server/internal/administration/live_sources.go:482 |
| `maintenance_policy` | `server/internal/workpolicy/policy.go:51` | CHECK(singleton=1) |
| `maintenance_windows` | `server/internal/workpolicy/policy.go:61` | Direct delete: server/internal/workpolicy/policy.go:54 |
| `maintenance_window_days` | `server/internal/workpolicy/policy.go:65` | ON DELETE CASCADE from maintenance_windows; baseline foreign keys |
| `dvr_recording_grants` | `server/internal/recordingaccess/grants.go:57` | Direct delete: server/internal/identity/profile_erasure.go:26 |
| `dvr_defaults` | `server/internal/livechannels/dvr/store.go:39` | One document per owner; owner revision cardinality has a resource ceiling |
| `direct_memberships` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from accounts; baseline foreign keys |
| `access_invitations` | `server/internal/access/invitations.go:255` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `access_limits` | `server/internal/access/limits.go:330` | ON DELETE CASCADE from accounts; baseline foreign keys |
| `access_receipts` | `server/internal/access/store.go:171` | Direct delete: server/internal/access/store.go:174 |
| `client_log_uploads` | `server/internal/servicelog/clientlogs.go:149` | Direct delete: server/internal/servicelog/clientlogs.go:154 |
| `diagnostics_debug_window` | `server/internal/httpapi/admin_diagnostics.go:136` | CHECK(singleton=1) |
| `content_rating_pending` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/ratings.go:54 |
| `inventory_directory_cache` | `server/internal/catalog/inventory.go:195` | Added primary-key ceiling 10,000,000; baseline bound triggers and tableretention/bounds.go; Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; ON DELETE CASCADE from library_sources; baseline foreign keys; Direct delete: server/internal/ingestion/directory_cache.go:23 |
| `playback_route_failures` | `server/internal/playback/route_failures.go:85` | ON DELETE CASCADE from authorization_family_tokens; baseline foreign keys; Direct delete: server/internal/playback/route_failures.go:161 |
| `playback_track_memory` | `server/internal/playbackv1/track_memory.go:84` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `identity_refresh_credentials` | `server/internal/identity/families.go:155` | ON DELETE CASCADE from authorization_session_families; baseline foreign keys; Direct delete: server/internal/identity/member_erasure.go:193 |
| `identity_refresh_receipts` | `server/internal/identity/refresh.go:114` | ON DELETE CASCADE from authorization_session_families; baseline foreign keys; Direct delete: server/internal/identity/prune_sessions.go:23 |
| `hosted_trust_state` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `hosted_signing_revocations` | `server/internal/hosted/trust.go:105` | Added primary-key ceiling 100,000; baseline bound triggers and tableretention/bounds.go |
| `hosted_policy_signers` | `Baseline/trigger/subsystem declaration` | One primary-key row per admitted server policy; shipped code reads it to revoke signer families. No append-only event history. |
| `networking_web_claims` | `server/internal/networking/claim_web.go:73` | Direct delete: server/internal/networking/claim_web.go:44 |
| `hosted_server_events` | `server/internal/hosted/events.go:35` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go; Direct delete: server/internal/hosted/events.go:78 |
| `managed_mounts` | `server/internal/mounts/service.go:319` | Direct delete: server/internal/mounts/commands.go:341 |
| `mount_allocations` | `server/internal/mounts/allocations.go:65` | Direct delete: server/internal/mounts/service.go:324 |
| `mount_backend_configs` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from managed_mounts; baseline foreign keys |
| `mount_controls` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from managed_mounts; baseline foreign keys |
| `mount_operations` | `server/internal/mounts/commands.go:115` | Direct delete: server/internal/mounts/commands.go:152 |
| `mount_revision` | `Baseline/trigger/subsystem declaration` | Baseline inserts id=1; shipped writers only update that row (mounts/commands.go, baseline mount_revision_* triggers). |
| `live_source_identities` | `server/internal/livechannels/store.go:265` | Added primary-key ceiling 100,000; baseline bound triggers and tableretention/bounds.go |
| `live_sources` | `server/internal/livechannels/store.go:429` | One row per capped live_source_identities ID |
| `live_generations` | `server/internal/livechannels/store.go:268` | Superseded generations after grace, current/source-dependent generations protected, rows dismantled in batches; server/internal/livechannels/generation_retention.go:24; Direct delete: server/internal/livechannels/generation_retention.go:76 |
| `live_channel_versions` | `server/internal/livechannels/store.go:321` | Superseded generations after grace, current/source-dependent generations protected, rows dismantled in batches; server/internal/livechannels/generation_retention.go:24 |
| `live_channel_logos` | `server/internal/livechannels/store.go:337` | Superseded generations after grace, current/source-dependent generations protected, rows dismantled in batches; server/internal/livechannels/generation_retention.go:24 |
| `live_logo_jobs` | `Baseline/trigger/subsystem declaration` | One primary-key row per live source, whose identity table is capped; baseline live_logo_* triggers overwrite it. |
| `identity_credential_state` | `server/internal/identity/direct_accounts.go:759` | ON DELETE CASCADE from accounts; baseline foreign keys; Direct delete: server/internal/identity/direct_accounts.go:573 |
| `identity_credential_attempts` | `server/internal/identity/credential_security.go:299` | ON DELETE CASCADE from accounts; baseline foreign keys |
| `authorization_family_grants` | `server/internal/identity/session_grants.go:55` | ON DELETE CASCADE from authorization_session_families; baseline foreign keys |
| `direct_profile_pin_attempts` | `server/internal/identity/direct_profiles.go:440` | ON DELETE CASCADE from direct_profiles, identity_devices; baseline foreign keys; Direct delete: server/internal/identity/pin_recovery.go:118 |
| `client_log_daily_usage` | `server/internal/servicelog/clientlogs.go:138` | Direct delete: server/internal/servicelog/clientlogs.go:160 |
| `quick_connect_v1` | `server/internal/identity/quick_connect.go:200` | Direct delete: server/internal/identity/restore.go:65 |
| `setup_rate_limits_v1` | `server/internal/identity/quick_connect.go:123` | Direct delete: server/internal/identity/quick_connect.go:501 |
| `onboarding_state_v1` | `server/internal/identity/onboarding.go:137` | CHECK(singleton=1) |
| `identity_account_attempt_budget` | `server/internal/identity/credential_security.go:332` | ON DELETE CASCADE from accounts; baseline foreign keys; Direct delete: server/internal/identity/credential_security.go:302 |
| `api_idempotency` | `Baseline/trigger/subsystem declaration` | Expiry is at most seven days; apikit/idempotency/store.go Sweep, called by cmd/server/maintenance.go in 100-row writer quanta. |
| `identity_refresh_predecessors` | `server/internal/identity/refresh.go:117` | ON DELETE CASCADE from authorization_session_families; baseline foreign keys; Direct delete: server/internal/identity/prune_sessions.go:34 |
| `identity_unknown_attempts` | `server/internal/identity/credential_security.go:190` | Direct delete: server/cmd/server/maintenance.go:138 |
| `networking_remote_settings` | `server/internal/networking/remote_store.go:85` | One server document per defined scope |
| `networking_remote_state` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `networking_mappings` | `server/internal/networking/remote_store.go:214` | Direct delete: server/internal/networking/remote_store.go:227 |
| `networking_route_names` | `server/internal/networking/remote_store.go:269` | Direct delete: server/internal/networking/remote_withdrawal.go:213 |
| `networking_address_absence` | `server/internal/networking/remote_withdrawal.go:128` | Direct delete: server/internal/networking/remote_withdrawal.go:141 |
| `networking_certificate_settings` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `networking_certificate_state` | `server/internal/networking/certificate_store.go:87` | Direct delete: server/internal/networking/certificate_store.go:366 |
| `networking_certificates` | `server/internal/networking/certificate_store.go:182` | Direct delete: server/internal/networking/certificate_store.go:252 |
| `hosted_claim_cleanup_schedule` | `server/internal/hosted/claim_cleanup.go:201` | One row per capped networking_claim_intents operation |
| `hosted_claim_cleanup_receipts` | `server/internal/hosted/claim_cleanup.go:130` | Added primary-key ceiling 100,000; baseline bound triggers and tableretention/bounds.go |
| `networking_control_schedule` | `server/internal/hosted/current_schedule.go:66` | Direct delete: server/internal/hosted/current_schedule.go:62 |
| `networking_server_identities` | `server/internal/networking/claim_identity_store.go:129` | Added primary-key ceiling 100,000; baseline bound triggers and tableretention/bounds.go |
| `networking_claim_identity` | `server/internal/networking/claim_identity_store.go:133` | CHECK(singleton=1) |
| `networking_claim_intents` | `server/internal/networking/claim_sqlite.go:272` | Added primary-key ceiling 100,000; baseline bound triggers and tableretention/bounds.go |
| `networking_claim_cancellations` | `server/internal/networking/claim_sqlite.go:593` | Added primary-key ceiling 100,000; baseline bound triggers and tableretention/bounds.go |
| `networking_claim_authority` | `Baseline/trigger/subsystem declaration` | One primary-key authority row per capped claim operation; retained authority must not be age-pruned. |
| `networking_claim_schema` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `artwork_variants` | `server/internal/metadata/artwork.go:263` | ON DELETE CASCADE from artwork_objects; baseline foreign keys |
| `playback_device_capabilities` | `server/internal/playback/capability_document.go:879` | ON DELETE CASCADE from identity_devices; baseline foreign keys |
| `playback_v1_sessions` | `server/internal/playbackv1/transitions.go:381` | Direct delete: server/internal/playbackv1/queue_sweep.go:134 |
| `api_events` | `server/internal/apievents/events.go:53` | Direct delete: server/internal/apievents/events.go:69 |
| `queues_v1` | `server/internal/playbackv1/queue_commands.go:380` | Direct delete: server/internal/playbackv1/queue_sweep.go:154 |
| `queue_v1_snapshots` | `server/internal/playbackv1/queue_build.go:188` | ON DELETE CASCADE from queues_v1; baseline foreign keys; Direct delete: server/internal/playbackv1/queue_commands.go:149 |
| `queue_v1_chunks` | `server/internal/playbackv1/queue_build.go:193` | ON DELETE CASCADE from queue_v1_snapshots; baseline foreign keys; Direct delete: server/internal/playbackv1/queue_sweep.go:146 |
| `queue_v1_segments` | `server/internal/playbackv1/queue_commands.go:449` | ON DELETE CASCADE from queue_v1_snapshots, queues_v1; baseline foreign keys; Direct delete: server/internal/playbackv1/queue_commands.go:107 |
| `queue_v1_removals` | `server/internal/playbackv1/queue_commands.go:906` | ON DELETE CASCADE from queue_v1_snapshots; baseline foreign keys; Direct delete: server/internal/playbackv1/queue_commands.go:146 |
| `queue_v1_container_counts` | `server/internal/playbackv1/queue_counts.go:103` | Added indexed 7-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `queue_v1_create_receipts` | `server/internal/playbackv1/queue_commands.go:420` | Direct delete: server/internal/playbackv1/queue_sweep.go:126 |
| `live_remote_save_bindings` | `server/internal/livechannels/remote.go:329` | Added primary-key ceiling 100,000; baseline bound triggers and tableretention/bounds.go |
| `live_remote_previews` | `server/internal/livechannels/remote.go:215` | Direct delete: server/internal/livechannels/remote.go:205 |
| `live_remote_configs` | `server/internal/livechannels/remote.go:358` | Direct delete: server/internal/livechannels/remote.go:609 |
| `live_source_settings` | `server/internal/livechannels/remote.go:365` | Direct delete: server/internal/livechannels/store.go:201 |
| `live_source_dependencies` | `server/internal/livechannels/allocator.go:232` | Direct delete: server/internal/identity/profile_erasure.go:121 |
| `live_source_removals` | `server/internal/livechannels/remote.go:614` | Added primary-key ceiling 100,000; baseline bound triggers and tableretention/bounds.go |
| `live_allocations` | `server/internal/livechannels/allocator.go:228` | Added indexed 7-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `live_allocation_reconcile_cursor` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `live_reservations` | `server/internal/livechannels/dvr/store.go:246` | Added indexed 7-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `live_channel_preferences` | `server/internal/livechannels/preferences.go:74` | Added primary-key ceiling 5,000,000; baseline bound triggers and tableretention/bounds.go |
| `live_preference_receipts` | `server/internal/livechannels/preferences.go:77` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `live_operations` | `server/internal/livechannels/store.go:275` | Superseded generations after grace, current/source-dependent generations protected, rows dismantled in batches; server/internal/livechannels/generation_retention.go:24 |
| `live_programmes` | `server/internal/livechannels/store.go:358` | Superseded generations after grace, current/source-dependent generations protected, rows dismantled in batches; server/internal/livechannels/generation_retention.go:24 |
| `live_programme_metadata` | `server/internal/livechannels/store.go:361` | Superseded generations after grace, current/source-dependent generations protected, rows dismantled in batches; server/internal/livechannels/generation_retention.go:24 |
| `live_enable_receipts` | `server/internal/livechannels/store.go:522` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `lc_maintenance_cursor` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `lc_channels` | `server/internal/livechannels/library/candidates.go:594` | Added primary-key ceiling 100,000; baseline bound triggers and tableretention/bounds.go |
| `lc_receipts` | `server/internal/livechannels/library/reorder.go:69` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `lc_generations` | `server/internal/livechannels/library/candidates.go:597` | Inactive, unleased channel generations removed in 512-row batches; server/internal/livechannels/library/prune.go:15; Direct delete: server/internal/livechannels/library/prune.go:47 |
| `lc_candidates` | `server/internal/livechannels/library/generator.go:278` | Inactive, unleased channel generations removed in 512-row batches; server/internal/livechannels/library/prune.go:15 |
| `lc_entries` | `server/internal/livechannels/library/generator.go:307` | Inactive, unleased channel generations removed in 512-row batches; server/internal/livechannels/library/prune.go:15 |
| `lc_rule_state` | `server/internal/livechannels/library/generator.go:157` | Inactive, unleased channel generations removed in 512-row batches; server/internal/livechannels/library/prune.go:15 |
| `lc_used` | `server/internal/livechannels/library/generator.go:207` | Inactive, unleased channel generations removed in 512-row batches; server/internal/livechannels/library/prune.go:15 |
| `lc_playback_refs` | `server/internal/livechannels/library/guide.go:519` | Added indexed 1-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `lc_template_installs` | `Baseline/trigger/subsystem declaration` | Fixed first-party template_key set, one row per template install. |
| `dvr_owner_revisions` | `server/internal/identity/profile_erasure.go:124` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `dvr_receipts` | `server/internal/livechannels/dvr/store.go:110` | Direct delete: server/internal/identity/profile_erasure.go:130 |
| `dvr_rules` | `server/internal/livechannels/dvr/rules.go:148` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `dvr_recordings` | `server/internal/livechannels/dvr/store.go:233` | Added primary-key ceiling 1,000,000; baseline bound triggers and tableretention/bounds.go |
| `dvr_rule_work` | `server/internal/livechannels/dvr/reconcile.go:268` | Direct delete: server/internal/identity/profile_erasure.go:118 |
| `dvr_maintenance_cursors` | `server/internal/livechannels/dvr/reconcile.go:276` | Fixed maintenance task keys |
| `dvr_delete_work` | `server/internal/livechannels/dvr/retention.go:124` | Direct delete: server/internal/livechannels/dvr/retention.go:334 |
| `dvr_retention_retry` | `server/internal/livechannels/dvr/retention.go:191` | Direct delete: server/internal/livechannels/dvr/retention.go:199 |
| `dvr_cleanup_work` | `server/internal/livechannels/dvr/cleanup.go:19` | Direct delete: server/internal/livechannels/dvr/cleanup.go:74 |
| `dvr_storage_policy` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `remote_sources` | `server/internal/remotesources/configuration.go:234` | Added primary-key ceiling 100,000; baseline bound triggers and tableretention/bounds.go |
| `remote_source_operations` | `server/internal/remotesources/configuration.go:260` | Direct delete: server/internal/remotesources/configuration.go:250 |
| `remote_listing_sessions` | `server/internal/remotesources/inventory.go:78` | One latest ready snapshot per retained directory; terminal/orphan listings dismantled in 256-row batches; server/internal/ingestion/remote_retention.go:13; Direct delete: server/internal/remotesources/service.go:392 |
| `remote_native_listings` | `server/internal/remotesources/inventory_native.go:124` | One latest ready snapshot per retained directory; terminal/orphan listings dismantled in 256-row batches; server/internal/ingestion/remote_retention.go:13; ON DELETE CASCADE from remote_listing_sessions; baseline foreign keys |
| `remote_listing_progress` | `server/internal/remotesources/dav_continuation.go:117` | One latest ready snapshot per retained directory; terminal/orphan listings dismantled in 256-row batches; server/internal/ingestion/remote_retention.go:13; ON DELETE CASCADE from remote_listing_sessions; baseline foreign keys |
| `remote_listing_entries` | `server/internal/remotesources/dav_continuation.go:102` | One latest ready snapshot per retained directory; terminal/orphan listings dismantled in 256-row batches; server/internal/ingestion/remote_retention.go:13; ON DELETE CASCADE from remote_listing_sessions; baseline foreign keys; Direct delete: server/internal/remotesources/dav_continuation.go:82 |
| `remote_inventory_checks` | `server/internal/remotesources/inventory_proof.go:82` | One latest ready snapshot per retained directory; terminal/orphan listings dismantled in 256-row batches; server/internal/ingestion/remote_retention.go:13; Direct delete: server/internal/remotesources/service.go:393 |
| `remote_scan_cursors` | `server/internal/remotesources/inventory.go:286` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; Direct delete: server/internal/remotesources/service.go:394 |
| `remote_scan_generations` | `server/internal/remotesources/fences.go:23` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17 |
| `remote_object_refs` | `server/internal/remotesources/fences.go:64` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `remote_analysis_status` | `server/internal/ingestion/remote.go:41` | Added indexed 30-day cleanup with live/reference checks; tableretention/retention.go Policies |
| `live_logo_failures` | `server/internal/administration/library_channels.go:805` | Direct delete: server/internal/administration/guide_image_worker.go:247 |
| `profile_show_activity` | `server/internal/catalog/title_merge.go:253` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/personal_activity.go:427 |
| `continue_show_dismissals` | `server/internal/catalog/title_merge.go:256` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/personal_activity.go:430 |
| `listening_book_series` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/persistence/migrations/0001_baseline.sql:4837 |
| `library_metadata_agents` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from libraries; baseline foreign keys |
| `artwork_compaction_failures` | `server/internal/metadata/artwork_compaction.go:38` | ON DELETE CASCADE from artwork_objects; baseline foreign keys |
| `playback_private_presentations` | `server/internal/playback/service.go:522` | ON DELETE CASCADE from playback_sessions; baseline foreign keys; Direct delete: server/internal/playback/v1.go:265 |
| `playback_v1_prepared` | `server/internal/playbackv1/transitions.go:257` | ON DELETE CASCADE from queues_v1; baseline foreign keys; Direct delete: server/internal/playbackv1/queue_sweep.go:122 |
| `audio_render_facts` | `server/internal/audiofacts/store.go:46` | ON DELETE CASCADE from catalog_assets; baseline foreign keys |
| `audio_render_fact_failures` | `server/internal/audiofacts/store.go:64` | ON DELETE CASCADE from catalog_assets; baseline foreign keys; Direct delete: server/internal/audiofacts/store.go:56 |
| `playback_audio_plans` | `server/internal/playback/audio_plan_v2.go:265` | ON DELETE CASCADE from playback_sessions; baseline foreign keys |
| `playback_channel_sessions` | `server/internal/playback/channel_sessions.go:87` | ON DELETE CASCADE from playback_v1_sessions; baseline foreign keys |
| `account_portico_links` | `server/internal/hosted/membership_push.go:497` | ON DELETE CASCADE from accounts; baseline foreign keys; Direct delete: server/internal/hosted/claim_cleanup.go:152 |
| `hosted_membership_journal` | `server/internal/identity/portico_accounts.go:294` | Added primary-key ceiling 10,000,000; baseline bound triggers and tableretention/bounds.go; Direct delete: server/internal/hosted/membership_push.go:320 |
| `hosted_membership_sync` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `hosted_identity_nonces` | `server/internal/hosted/identity_assertion.go:164` | Direct delete: server/internal/hosted/identity_assertion.go:148 |
| `hosted_identity_challenges` | `server/internal/hosted/identity_assertion.go:81` | Direct delete: server/internal/identity/restore.go:67 |
| `portico_admitted_families` | `server/internal/identity/portico_accounts.go:150` | ON DELETE CASCADE from authorization_session_families; baseline foreign keys |
| `portico_factor_challenges` | `server/internal/identity/portico_accounts.go:107` | ON DELETE CASCADE from identity_factor_challenges; baseline foreign keys |
| `hosted_migrated_families` | `Baseline/trigger/subsystem declaration` | No current append writer found after tracing this table name in Go/SQL; baseline/reference declaration remains, with no runtime growth path in this build |
| `live_programme_icons` | `server/internal/livechannels/store.go:365` | Superseded generations after grace, current/source-dependent generations protected, rows dismantled in batches; server/internal/livechannels/generation_retention.go:24; Direct delete: server/internal/livechannels/generation_retention.go:60 |
| `live_programme_images` | `server/internal/administration/guide_image_worker.go:244` | Direct delete: server/internal/administration/guide_image_worker.go:308 |
| `live_icon_jobs` | `Baseline/trigger/subsystem declaration` | One primary-key row per live source, whose identity table is capped; baseline live_icon_* triggers overwrite it. |
| `maintenance_window_tasks` | `server/internal/workpolicy/policy.go:73` | ON DELETE CASCADE from maintenance_windows; baseline foreign keys |
| `networking_claim_credentials` | `server/internal/networking/claim_sqlite.go:429` | Direct delete: server/internal/networking/claim_cleanup.go:166 |
| `catalog_terms` | `server/internal/compactcatalog/write.go:929` | Daily cursor-based orphan pruning, 64/128/256 candidates per transaction; server/internal/compactcatalog/retention.go:39 |
| `catalog_entity_terms` | `server/internal/compactcatalog/write.go:932` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/write.go:945 |
| `catalog_credit_labels` | `server/internal/compactcatalog/write.go:1014` | Daily cursor-based orphan pruning, 64/128/256 candidates per transaction; server/internal/compactcatalog/retention.go:39 |
| `catalog_asset_issues` | `server/internal/compactcatalog/derive_availability.go:78` | Direct delete: server/internal/compactcatalog/derive_availability.go:75 |
| `catalog_browse_memberships` | `server/internal/compactcatalog/derive_browse.go:356` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/derive_browse.go:335 |
| `catalog_browse_buckets` | `server/internal/compactcatalog/derive_browse.go:136` | Direct delete: server/internal/compactcatalog/derive_browse.go:106 |
| `compact_visibility_counts` | `server/internal/catalog/visibility_incremental.go:197` | Direct delete: server/internal/compactcatalog/library_cleanup.go:45 |
| `compact_visibility_unavailable_counts` | `server/internal/catalog/visibility_incremental.go:25` | Direct delete: server/internal/compactcatalog/library_cleanup.go:49 |
| `catalog_home_buckets` | `server/internal/compactcatalog/derive_browse.go:278` | Direct delete: server/internal/compactcatalog/derive_browse.go:275 |
| `catalog_collection_visible_edges` | `server/internal/compactcatalog/collection_visible_counts.go:203` | Direct delete: server/internal/compactcatalog/collection_visible_counts.go:209 |
| `catalog_collection_visible_counts` | `server/internal/compactcatalog/collection_visible_counts.go:206` | Direct delete: server/internal/compactcatalog/collection_visible_counts.go:219 |
| `catalog_collection_visible_pair_jobs` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/collection_visible_counts.go:79 |
| `catalog_collection_visible_item_jobs` | `server/internal/compactcatalog/collection_visible_counts.go:179` | Direct delete: server/internal/compactcatalog/collection_visible_counts.go:131 |
| `catalog_collection_visible_state` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `catalog_movie_category_summaries` | `server/internal/compactcatalog/browse_categories.go:202` | Direct delete: server/internal/compactcatalog/browse_categories.go:206 |
| `catalog_attribute_fields` | `server/internal/compactcatalog/write.go:1022` | Added primary-key ceiling 10,000; baseline bound triggers and tableretention/bounds.go |
| `catalog_attribute_terms` | `server/internal/compactcatalog/write.go:1036` | Daily cursor-based orphan pruning, 64/128/256 candidates per transaction; server/internal/compactcatalog/retention.go:39 |
| `catalog_item_attribute_edges` | `server/internal/compactcatalog/write.go:1039` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/write.go:1027 |
| `catalog_movie_category_visible_summaries` | `server/internal/compactcatalog/browse_category_visibility.go:237` | Direct delete: server/internal/compactcatalog/browse_category_visibility.go:241 |
| `catalog_movie_category_visible_pair_jobs` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/browse_category_visibility.go:89 |
| `catalog_movie_category_visible_item_jobs` | `server/internal/compactcatalog/browse_category_visibility.go:196` | Direct delete: server/internal/compactcatalog/browse_category_visibility.go:149 |
| `catalog_movie_category_visible_state` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `catalog_visibility_sort_edges` | `server/internal/compactcatalog/visibility_sort_buckets.go:225` | Direct delete: server/internal/compactcatalog/library_cleanup.go:42 |
| `catalog_visibility_sort_buckets` | `server/internal/compactcatalog/visibility_sort_buckets.go:228` | Direct delete: server/internal/compactcatalog/library_cleanup.go:41 |
| `catalog_visibility_sort_pair_jobs` | `server/internal/compactcatalog/visibility_sort_buckets.go:161` | Direct delete: server/internal/compactcatalog/library_cleanup.go:43 |
| `catalog_visibility_sort_entity_jobs` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/visibility_sort_buckets.go:114 |
| `catalog_visibility_sort_state` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `catalog_personal_sort_rows` | `server/internal/compactcatalog/personal_sort_buckets.go:356` | Direct delete: server/internal/compactcatalog/personal_sort_buckets.go:333 |
| `catalog_personal_sort_buckets` | `server/internal/compactcatalog/personal_sort_buckets.go:366` | Direct delete: server/internal/compactcatalog/personal_sort_buckets.go:378 |
| `catalog_personal_sort_member_jobs` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/personal_sort_buckets.go:150 |
| `catalog_personal_sort_entity_jobs` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/personal_sort_buckets.go:208 |
| `catalog_personal_visibility_sort_edges` | `server/internal/compactcatalog/personal_visibility_sort_buckets.go:182` | Direct delete: server/internal/compactcatalog/personal_visibility_sort_buckets.go:165 |
| `catalog_personal_visibility_sort_buckets` | `server/internal/compactcatalog/personal_visibility_sort_buckets.go:185` | Direct delete: server/internal/compactcatalog/personal_visibility_sort_buckets.go:168 |
| `catalog_personal_visibility_sort_profile_jobs` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/personal_visibility_sort_buckets.go:59 |
| `catalog_personal_visibility_sort_class_jobs` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/personal_visibility_sort_buckets.go:119 |
| `catalog_book_group_members` | `server/internal/compactcatalog/listening_context.go:103` | ON DELETE CASCADE from catalog_book_groups; baseline foreign keys; Direct delete: server/internal/compactcatalog/listening_context.go:95 |
| `catalog_book_context` | `server/internal/compactcatalog/listening_context.go:41` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/compactcatalog/library_cleanup.go:52 |
| `catalog_book_series_terms` | `server/internal/compactcatalog/listening_context_evidence.go:328` | Direct delete: server/internal/compactcatalog/listening_context_evidence.go:359 |
| `catalog_book_series_spellings` | `server/internal/compactcatalog/listening_context_evidence.go:331` | Direct delete: server/internal/compactcatalog/listening_context_evidence.go:351 |
| `catalog_book_position_terms` | `server/internal/compactcatalog/listening_context_evidence.go:374` | Direct delete: server/internal/compactcatalog/listening_context_evidence.go:391 |
| `catalog_book_evidence_clock` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `catalog_book_visible_file_edges` | `server/internal/compactcatalog/listening_group_visible.go:127` | Direct delete: server/internal/compactcatalog/library_cleanup.go:31 |
| `catalog_book_visible_counts` | `server/internal/compactcatalog/listening_group_visible.go:134` | Direct delete: server/internal/compactcatalog/library_cleanup.go:30 |
| `catalog_book_group_visible_edges` | `server/internal/compactcatalog/listening_group_visible.go:213` | Direct delete: server/internal/compactcatalog/library_cleanup.go:29 |
| `catalog_book_group_visible_counts` | `server/internal/compactcatalog/listening_group_visible.go:216` | Direct delete: server/internal/compactcatalog/library_cleanup.go:28 |
| `catalog_book_visible_file_jobs` | `server/internal/compactcatalog/listening_group_visible.go:266` | Direct delete: server/internal/compactcatalog/listening_group_visible.go:78 |
| `catalog_book_group_visible_pair_jobs` | `server/internal/compactcatalog/listening_group_visible.go:145` | Direct delete: server/internal/compactcatalog/listening_group_visible.go:187 |
| `catalog_book_group_visible_state` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `catalog_content_eligible_rows` | `server/internal/compactcatalog/content_eligible.go:156` | Direct delete: server/internal/compactcatalog/content_eligible.go:149 |
| `catalog_content_eligible_counts` | `server/internal/compactcatalog/content_eligible.go:159` | Direct delete: server/internal/compactcatalog/content_eligible.go:145 |
| `catalog_content_eligible_jobs` | `server/internal/compactcatalog/content_eligible.go:199` | Direct delete: server/internal/compactcatalog/content_eligible.go:163 |
| `catalog_content_eligible_child_jobs` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/compactcatalog/content_eligible.go:206 |
| `catalog_content_eligible_state` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `personal_jobs` | `server/internal/catalog/bulk_jobs.go:350` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; Direct delete: server/internal/catalog/bulk_jobs.go:170 |
| `personal_job_items` | `server/internal/catalog/bulk_capture.go:158` | ON DELETE CASCADE from personal_jobs; baseline foreign keys; Direct delete: server/internal/catalog/bulk_jobs.go:170 |
| `personal_job_failures` | `server/internal/catalog/bulk_jobs.go:564` | ON DELETE CASCADE from personal_jobs; baseline foreign keys; Direct delete: server/internal/catalog/bulk_jobs.go:170 |
| `container_personal_state` | `server/internal/catalog/container_personal.go:165` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; Direct delete: server/internal/catalog/library_removal.go:358 |
| `personal_watched_intents` | `server/internal/personalstate/watched.go:61` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/container_undo.go:186 |
| `container_personal_undo` | `server/internal/catalog/container_undo.go:56` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/container_undo.go:51 |
| `container_personal_undo_items` | `server/internal/catalog/container_undo.go:72` | ON DELETE CASCADE from catalog_entities, container_personal_undo; baseline foreign keys |
| `personal_job_destinations` | `server/internal/catalog/bulk_commands.go:178` | ON DELETE CASCADE from personal_jobs; baseline foreign keys; Direct delete: server/internal/catalog/bulk_jobs.go:170 |
| `personal_job_details` | `server/internal/catalog/bulk_capture.go:162` | ON DELETE CASCADE from personal_job_items; baseline foreign keys |
| `download_requests` | `server/internal/downloads/requests.go:165` | Direct delete: server/internal/identity/profile_erasure.go:78 |
| `download_request_items` | `server/internal/downloads/requests.go:428` | ON DELETE CASCADE from download_requests; baseline foreign keys; Direct delete: server/internal/downloads/requests.go:376 |
| `download_preparation_authority` | `server/internal/downloads/preparations.go:223` | ON DELETE CASCADE from download_preparations; baseline foreign keys; Direct delete: server/internal/identity/profile_erasure.go:81 |
| `authorization_family_renewals` | `server/internal/identity/families.go:360` | Direct delete: server/internal/identity/families.go:413 |
| `queue_v1_saves` | `server/internal/playbackv1/queue_save.go:87` | Personal state follows a profile/title; profile erasure deletes it; history intentionally may remain forever; server/internal/identity/profile_erasure.go:40; Direct delete: server/internal/playbackv1/queue_save.go:258 |
| `show_people_credits` | `server/internal/metadata/show_people.go:17` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/show_people.go:16 |
| `music_artist_evidence` | `server/internal/metadata/music_artist.go:157` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `tvdb_episode_credit_evidence` | `server/internal/metadata/tvdb_episode_credits.go:88` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/metadata/tvdb_episode_credits.go:102 |
| `local_artwork` | `server/internal/catalog/local_artwork.go:66` | ON DELETE CASCADE from libraries; baseline foreign keys; Direct delete: server/internal/catalog/sidecar_sources.go:71 |
| `inventory_artwork_sources` | `server/internal/catalog/sidecar_sources.go:46` | ON DELETE CASCADE from catalog_assets, catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/sidecar_sources.go:42 |
| `compact_visibility_dirty` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from compact_visibility_classes; baseline foreign keys; Direct delete: server/internal/compactcatalog/library_cleanup.go:46 |
| `playback_conversion_targets` | `server/internal/playback/conversion_target.go:42` | ON DELETE CASCADE from playback_delivery_plans; baseline foreign keys |
| `playback_conversion_failures` | `server/internal/playback/conversion_history.go:22` | Direct delete: server/internal/playback/conversion_history.go:26 |
| `queue_v1_play_spans` | `server/internal/playbackv1/queue_order.go:109` | ON DELETE CASCADE from queues_v1; baseline foreign keys; Direct delete: server/internal/playbackv1/queue_order.go:102 |
| `audio_render_fact_requests` | `server/internal/audiofacts/store.go:84` | ON DELETE CASCADE from catalog_assets; baseline foreign keys; Direct delete: server/internal/audiofacts/store.go:201 |
| `api_credentials` | `server/internal/access/credentials.go:270` | ON DELETE CASCADE from accounts; baseline foreign keys; Direct delete: server/internal/access/credentials.go:441 |
| `api_credential_use` | `server/internal/access/credentials.go:419` | ON DELETE CASCADE from api_credentials; baseline foreign keys |
| `inventory_path_notices` | `server/internal/ingestion/targeted.go:248` | ON DELETE CASCADE from library_sources; baseline foreign keys; Direct delete: server/internal/ingestion/targeted.go:243 |
| `inventory_run_targets` | `server/internal/ingestion/targeted.go:326` | Terminal scan history: 20 runs per source; scratch after 15 min; obsolete incarnations; 256 rows per quantum; server/internal/ingestion/retention.go:17; ON DELETE CASCADE from jobs; baseline foreign keys; Direct delete: server/internal/ingestion/targeted.go:414 |
| `webhooks` | `server/internal/webhooks/webhooks.go:379` | Direct delete: server/internal/webhooks/webhooks.go:483 |
| `webhook_deliveries` | `server/internal/webhooks/webhooks.go:591` | ON DELETE CASCADE from webhooks; baseline foreign keys; Direct delete: server/internal/webhooks/dispatch.go:306 |
| `webhook_state` | `server/internal/webhooks/webhooks.go:673` | Fixed subsystem state keys; replaces the same key |
| `webhook_events` | `server/internal/webhooks/webhooks.go:626` | Fan-out trigger deletes each event immediately; pending deliveries capped at 1000 per webhook; Direct delete: server/internal/persistence/migrations/0001_baseline.sql:6068 |
| `catalog_generations` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `catalog_derivation_failures` | `server/internal/compactcatalog/derive.go:655` | Direct delete: server/internal/compactcatalog/derive.go:574 |
| `catalog_browse_rows_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_browse_blocks_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_browse_buckets_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_browse_counted_rows_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_browse_memberships_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_browse_member_metrics_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_home_buckets_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_item_availability_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_facet_counts_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_related_facets_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_search_documents_next` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `api_change_pending` | `server/internal/httpapi/events_routes.go:313` | Direct delete: server/internal/apievents/pending.go:53 |
| `catalog_retention_cursors` | `server/internal/compactcatalog/retention.go:57` | Direct delete: server/internal/compactcatalog/retention.go:124 |
| `catalog_movie_category_members_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_movie_category_summaries_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_directory_revision` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `database_maintenance_status` | `Baseline/trigger/subsystem declaration` | Schema singleton key: at most one row |
| `live_channel_choices` | `server/internal/livechannels/store.go:291` | Superseded generations after grace, current/source-dependent generations protected, rows dismantled in batches; server/internal/livechannels/generation_retention.go:24; ON DELETE CASCADE from live_generations; baseline foreign keys |
| `catalog_show_settings` | `server/internal/catalog/show_settings.go:92` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `play_history` | `server/internal/catalog/play_history.go:106` | Direct delete: server/internal/operations/evidence.go:518 |
| `play_counts` | `server/internal/catalog/play_history.go:51` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/title_merge.go:221 |
| `personal_play_counts` | `server/internal/catalog/play_history.go:124` | ON DELETE CASCADE from catalog_entities; baseline foreign keys; Direct delete: server/internal/catalog/title_merge.go:218 |
| `catalog_browse_groups` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/persistence/migrations/0001_baseline.sql:6542 |
| `catalog_browse_groups_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_facet_members_previous` | `Baseline/trigger/subsystem declaration` | Sparse retained serving generation; compactcatalog/retention.go PruneTx deletes released/older generations |
| `catalog_browse_episode_counts` | `Baseline/trigger/subsystem declaration` | ON DELETE CASCADE from catalog_entities; baseline foreign keys |
| `catalog_personal_play_totals` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/persistence/migrations/0001_baseline.sql:6607 |
| `notice_broadcast_work` | `server/internal/operations/notifications.go:751` | Direct delete: server/internal/operations/notifications.go:830 |
| `maintenance_retention_rows` | `Baseline/trigger/subsystem declaration` | Direct delete: server/internal/persistence/migrations/0001_baseline.sql:6678 |
| `maintenance_table_bounds` | `Baseline/trigger/subsystem declaration` | Exactly the compiled ResourceBounds table-name set; counters update in place. |
| `catalog_search_titles` | `Baseline/trigger/subsystem declaration` | FTS index of its live external-content documents; baseline insert/update/delete triggers and SQLite segment merging; no independent history |
| `catalog_people_names` | `server/internal/compactcatalog/derive_search.go:35` | FTS index of its live external-content documents; baseline insert/update/delete triggers and SQLite segment merging; no independent history; Direct delete: server/internal/compactcatalog/derive_search.go:34 |
| `catalog_search_titles_next` | `Baseline/trigger/subsystem declaration` | FTS index of its live external-content documents; baseline insert/update/delete triggers and SQLite segment merging; no independent history |

# Owner or viewer settings added or wired

- backgroundSpeed: existing Gentle/Normal/Fast enum, default Normal. Wired to new housekeeping quantum sizes and pacing, existing catalogue dictionary/identity pruning, and completed analysis-retention sweeps. The existing owner maintenance screen remains its control.
- Analysis RetainDays: existing owner policy now also reclaims retired analysis summary rows after physical/reference cleanup. No new setting or screen.
- Existing PlayHistoryDays (0 means forever), personal history/privacy, trash/provider-state and subsystem log/cache retention retain their current semantics. No history import/export or new retention settings were introduced.

# Needed from others

## Client selector wiring (S1)

Every web/native/TV/Vega list, saved view and grouping Play/Shuffle handler should replace first-page/first-500 enrolment with these two request lines, using the existing V1QueuePlayer instance and current authorized browse state:

```ts
const source: Selector = {query: {libraryId, pivot, query: applied.query ?? undefined, sort: applied.sort}};
await queuePlayer.playSelector(source, {shuffle, shuffleSeed});
```

For a saved resource, use the resource identity directly:

```ts
const source: Selector = {saved: {id: resource.id}};
await queuePlayer.playSelector(source, {shuffle, shuffleSeed});
```

For a saved playlist, retain `{container: {kind: 'playlist', id: playlist.id}}`. Do not pass cursor/range/limit/seek. Group/category/person/collection tabs must use their underlying title pivot and exact browse membership expression. The common screen/player wrapper may need to forward the selector instead of only ContentEntry arrays; it should retain its current identity/poster/engine admission handling and use the returned session/window. Web's sequence wrapper is web/src/player/engine.tsx (currently playSelector only for containers); native's saved-page wrapper is apps/native/src/screens/title/SavedPages.tsx usePlayList. Equivalent generic group/library handlers and TV/Vega shared screen overrides need the same forwarding. This lane changes the canonical service/types, not those screen contracts.

## Further follow-ups reviewed

- S2: leave the standalone subtitle mutation registered until coordinated conversion. Convert canonical SubtitleService targets/reservation/social-control use and web/src/player/options.ts plus native subtitle wrappers to session PATCH with subtitles.trackId/delivery and subtitleOffsetMs. Media-generation IDs currently passed to the standalone service are not automatically v1 session IDs; no unsafe cast or route deletion was added. Then delete the duplicate PUT contract/handler/ceiling path in the coordinated lane. Management/read/original/grant routes remain.
- S3: spoiler controls, write-beside-media export, country-specific ratings, per-library unrated switches and allow-list kids mode remain unapproved optional expansions; no server-only fragments were invented.
- S5: replace shared presentation fixedSort with a default sorting preference only while updating all web/native/TV consumers and memory behavior together. Changing the shared field alone would silently remove Recently aired defaults or retain forced sorting through a compatibility alias. Leave this coordinated presentation/screen change to its owner.
- S6: integrator must sync canonical packages into the writable native integration lane using scripts/shared-packages.mjs and verify revision/hash equality. This lane cannot write main-native.
- S7: native/WASM notices, corresponding-source inventory, Apple distribution/store rights and release packaging are outside this lane. No licence notice was removed.
- The decision report's W/N/T/V screen gaps, Vega safe authenticated transport, offline rich-subtitle/font/device lifecycle work, native owner panels and actual device acceptance remain the named lanes' responsibilities. This work does not declare product parity.
- The original data-model audit's blanket dictionary/identity-retention statement needs updating: those source paths existed before this lane. The old generic allegation is replaced by this table inventory and the exact new bounds, not by a claimed production lifetime test.

# Test notes for other environments

## Executed here

- Schema sync and its persistence checks passed after the final baseline additions.
- Linux amd64 build and Windows amd64/macOS arm64 cross-build passed. Final read-connection/pacing changes passed the final Linux compile and touched-package vet; cross-builds establish compilation, not platform runtime behavior.
- Targeted catalog tests: all movie browse orders match queue order (including personal valued/rest streams, For you and seeded random); saved-view definition/order, private-view refusal and shared collection access.
- Targeted playbackv1 tests: unchanged-revision restart, changed-source restart refusal retaining the 256-key prefix, complete growing seeded shuffle with no loss/duplicates, request-deadline-aware connection admission.
- Targeted HTTP tests: 520-item query with immediate first playback and concurrent edit/add, member content/library restrictions, seeded query shuffle; all-route credential scope/router-pattern guards; restriction read/write and feature-switch matrices.
- Targeted retention tests: bounded expired receipt removal, fresh records and active job protection, age-ledger deletion, permanent fence exact retry/update at a full ceiling, reusable capacity after deletion, equality of every declared ceiling to its baseline row, Gentle/Fast quantum behavior, child-first group dismantling, and natural-key interning/intent replay at a full ceiling.
- Analysis tests: 32-row summary quanta, preservation of live/recent/head/artifact-referenced summaries, existing physical reader-custody exclusion.
- Shared single files: queue-building.test.ts (4 cases), v1-queue-player.test.ts (36 cases) passed. TypeScript web/shared consumers and generated-route check passed. Touched-package vet and diff whitespace checks passed; final repeated checks are recorded with the snapshot receipt.
- Corrections during verification: fixture used a display library name instead of its opaque ID; age ledger initially assumed rowid for a WITHOUT ROWID cache; maintenance setting fixture omitted required updated_ms; secondary client contract was stale and then exposed nullable request typing; all were corrected and the affected checks rerun. The old fake-server URL fixture failure was independently confirmed unchanged in HEAD and repaired. Its failed long-running invocation was interrupted, then the full single file passed with a bounded exit.
- No full repository test suite, frontend/native release build, deployment, tuner/GPU run or physical device acceptance.

## Cold acceptance steps

1. Fresh final baseline, a video library with >500 and then >100,000 accessible titles. Send POST /v1/queues with the query selector and startPlayback. Pass: a first session/window appears after the initial captured batch; capture grows to every matching title in browse order, including final pages. Explicit item requests still reject >500 IDs. Compare grouping/genre/person/collection/list filters and each applicable sort to browse, with overlapping containers and missing files.
2. Repeat query shuffle with the same seed, then advance while capture is growing. Pass: no duplicate/lost matches; repeated unchanged membership has stable order; already played/current occurrences remain stable. Startup is allowed to select from the first captured batch. Native/Web/TV/Vega screen handlers first need the selector forwarding described above.
3. Pause test builds or use a large capture; edit metadata/add/remove/restrict titles while the process continues. Pass: captured membership/order comes from the original snapshot, while current authority still hides/refuses revoked entries in windows/playback. Restart before capture completes. Pass: unchanged source resumes; a changed source keeps only captured keys and reports failed, without a mixed queue. A new Play uses a new selection.
4. On modest hardware run two large captures while scanning and browsing; try a third capture and exhaust the background read pool. Pass: control/browse remains reachable, capture saturation is retryable, pool admission ends at the request deadline, capture permits/connections release after completion/failure/deadline. Observe WAL pressure over the maximum read lifetime; no million-item latency/capacity claim is inferred from unit tests.
5. Age disposable receipts/reference records in a copied test database, keep an active session/job/version pin, and set Gentle then Fast. Run housekeeping. Pass: small quanta with 64 versus 256 maximum deletions, active/pinned rows preserved, age rows removed with their records, no orphan foreign keys. Lower a resource ceiling in a throwaway database: a new key is refused at the limit, exact replay/update remains valid, deletion releases capacity. Production ceilings are compiled, not a settings screen.
6. Retire analysis results with referenced bytes and use a long owner retention period. Pass: bytes/head references remain until their existing custody/retention paths permit removal; summary rows disappear only afterwards and in 32-row batches. Confirm playback/download readers hold their physical exclusion on Linux, Windows and macOS filesystems.
7. Owner keeps play history forever and chooses never to empty missing titles. Run retention repeatedly. Pass: personal/server history and identity-restored watch state remain; only disposable bookkeeping is pruned. Test profile erasure and stale command/credential replay after cleanup; permanent guards must still refuse resurrection.

# Not finished

- Client screens still need selector forwarding; no first-party platform can be advertised as playing every grouped view until those handlers use the new selector. This lane's authorized server and non-screen shared-client implementation is complete.
- Coordinated S2 subtitle retirement, S5 tab default/toolbar conversion, S6 native package sync, S7 release/legal work and the decision report's other platform screen/transport gaps remain exact cross-lane work.
- A capture that loses its read snapshot after its source changes cannot recover the uncaptured old membership. It deliberately preserves the accepted prefix and fails that segment; there is no claim of a fully materialized frozen million-item queue before background capture completes.
- Source inventory and bounded tests do not establish sustained throughput, total disk size under every owner input, actual million-item capture/WAL pressure, multi-year retention operation or Windows/macOS/device playback. Production ceilings may refuse new records at exhaustion; no owner UI for increasing them was added.
- Final verification passed. The final snapshot receipt is appended below; no worktree changes follow that snapshot.

## Final verification checkpoint

Final source passed Linux build, touched-package vet, the tableretention and analysis-summary regression selections, current queue query HTTP/scope/router guards, checkpoint/receipt/cadence housekeeping regressions, TypeScript and generated-route/whitespace checks. Windows amd64 and macOS arm64 cross-builds passed after the dedicated worker and source-fence changes. Parent cleanup and natural-unique-key ceiling regressions passed. Background pause is checked before each new retention quantum, with the retention worker isolated from WAL monitoring. No full suite or deployment ran.

Snapshot 001 captured the initial complete-selector implementation; snapshot 002 added the detailed record bounds and cleanup. Snapshots are cumulative; integrate only the final snapshot.

## Final snapshot receipt

- Final cumulative snapshot: `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/snapshots/queue-and-retention/003`. Subject: Keep saved-view playback complete and record cleanup bounded.
- Base: 6f1530acb899fcc0241f4174492119826164fd8c. Tracked patch: 243,059 bytes; 9 new files, preserved in untracked.tar. SHA-256 of tracked.patch: `4a7cbd89fb42cedcc03e3021bf2ca32ec2f664c8d11622f05340b67382624fdb`.
- Apply snapshot 003 once, including its new-file archive; do not stack 001/002. Schema/API generated artifacts are part of the snapshot.
- Readback verified the current tracked diff and untracked file list exactly match 003. No worktree file was changed after it. The report is outside the implementation worktree.
