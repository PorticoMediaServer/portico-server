# Outcome
Matching now accepts clear yearless titles and minor spelling errors, normalises common title variants, tolerates one-year release differences, and uses kind/runtime evidence. An indexed, owner-only needs-attention API lists unmatched/uncertain titles with three candidates and supports accept, search, provider ID and dismiss commands. Existing history, undo, locks and publication fences are reused.

Provider-specific request rates and bounded concurrent title processing replace the universal one-per-second pace. Credential overrides, local-artwork quota and adult-search choice are implemented. TMDB show artwork/aggregate credits, TheTVDB film artwork, preferred-language image ranking, shared retained-source artwork and official absolute anime coordinates are implemented. NFO input is wider and tolerant; removed unlocked local fields release ownership after complete evidence reconciliation while owner locks preserve the selected relationships. Locale-specific freshness and explicit show/episode refresh are implemented.

Focused implementation checks and three client-core tests passed. Linux build, selected package vet, typed-route/contract tests, TypeScript, provider race tests and Windows/macOS cross-compiles passed. Final artifact checks passed. Existing numeric-name, artwork-precedence, season-folder, company-projection and owner-search-priority fixes remain intact. Context refresh failed because its lock directory is read-only; canonical guidance was loaded directly on ehler-homelab-mini-1.

Delivery snapshots: 001 “Match clear titles without a year and expose metadata needing attention”; 002 “Fetch metadata at provider limits and preserve local evidence”. Final cumulative snapshot 003 records typed evidence reads, safe NFO reconciliation, bounded hints, known/unknown field locales, explicit refresh/cache fixes, continued artwork pacing, scoped attention revisions and final verification.

# Decisions I made
- Automatic acceptance requires verified details, a score of at least 0.78 and a 0.07 margin over distinct identities. Scores under 0.90 stay in needs-attention. Owner choices and validated IDs are definitive; a routine refresh never upgrades a tentative guess to certainty.
- Typed IMDb IDs from files, folders, NFOs and known provider URLs resolve through TMDB find, never by fetching an owner-supplied URL. Known numeric provider IDs remain direct hints. Hints are bounded to 16; at most four conflicting direct identities are fetched. Search without a hard provider year filter, then score year locally, so festival/regional dates and misspellings remain recoverable.
- Provider field locales are recorded only where the adapter can establish them (including original-language titles and TheTVDB translations); unavailable per-field fallback languages are explicitly unknown, never relabelled with the requested language. Requested language/region remain in the publication receipt and library policy.
- Normalise case, punctuation, diacritics, articles, ampersands, common Roman numerals and Cyrillic/Greek transliteration; provider aliases cover other scripts.
- Attention revisions are input generations; incidental worker lease/status revisions are ignored. Existing selection routes are retried internally only while that input generation remains unchanged; their candidate authority digests still fence real changes. Episodes expose parent navigation rather than unsupported identify/search actions. A dismissal lasts until input generation changes. Pending, unavailable and ordering/mapping issues are also discoverable in the list.
- Default local artwork storage is 2 GiB, owner-adjustable. Per-image byte/pixel limits remain. Capacity is logged; retained covers remain usable even below the new quota.
- New screen policies default to US certifications alongside en-US metadata. An owner may explicitly choose another country or clear the region; foreign ratings are never silently substituted.
- Metadata settings accept optional TMDB read-token and TheTVDB project-key overrides; reads expose presence only. Blank overrides restore the application credential. AniList needs no key; MusicBrainz remains its existing opt-in source. Audiobook online-source work belongs to music-audiobooks; no invented provider is offered here.
- Shared maintenance backgroundSpeed is gentle/normal/fast, default normal. Normal requests: TMDB 10/sec, TheTVDB 2/sec, AniList below 30/minute; fast: TMDB 20/sec, TheTVDB 5/sec; gentle: TMDB 2/sec, TheTVDB 1/sec, AniList 15/minute. MusicBrainz stays one/sec. Provider response consumption is bounded to four simultaneous requests (one for AniList/MusicBrainz/AcoustID). Title batches are 2/4/8. Foreground pressure still yields through dbwork.PaceBackground. Artwork lanes also continue at bounded checkpoints rather than sleeping a second per image; the existing single decode/worker bound remains.
- Published limits checked: [TMDB](https://developer.themoviedb.org/docs/rate-limiting) describes about 40/sec; [AniList](https://docs.anilist.co/guide/rate-limiting) currently says 30/minute. TheTVDB documents no numeric allowance in the reviewed [v4 reference](https://github.com/thetvdb/v4-api/blob/main/docs/swagger.yml), so its rates are deliberately conservative. No measured first-import time is claimed.
- Retryable outages have quick exponential attempts, then indefinite recovery from 15 minutes, doubling to six hours with a small per-title spread. Retry-After is never undercut; cooldown waiting consumes no attempt. Final validation/auth failures stay final until a correction/retry.
- Absolute TMDB anime counts official positive seasons and excludes specials; original provider coordinates are kept separately and local numbering is unchanged. Alternative/community offsets are not guessed.
- [TMDB aggregate-credit documentation](https://developer.themoviedb.org/reference/tv-series-aggregate-credits) confirms full series cast/crew rather than newest-season scope; the detail request now appends aggregate_credits.

# Bugs
- B-06.1: base numeric-title fix preserved; bare numeric matcher titles now also survive. Scanner grammar belongs to naming.
- B-06.2: fixed; confident yearless/fuzzy matching and paged needs-attention.
- B-06.3: already fixed; occupied/local/locked guards preserved and covered by source-sharing regression checks.
- B-06.4 / B-10.2: fixed intake quota; configurable 2 GiB default, cached byte accounting, visible capacity log, raise-and-retry test. Automatic local-original pruning is not added.
- B-06.5: base season scoping/name fix preserved; movie backdrops/folder posters and episode thumbs added. Removed sidecar observation still needs scanner integration noted below.
- B-06.6: already fixed; TVDB company projection preserved.
- B-06.7: implemented TMDB show alternatives/logos/seasons/portraits and TVDB film artworks; show-artwork locale/season/portrait fixture passed; live-provider acceptance remains outstanding.
- B-06.8: fixed by appending and parsing series aggregate credits; fixture passed; current official docs settle the static scope question.
- B-06.9: fixed stale locale reuse through candidate locale/region predicates and freshness variants; known field locales are retained and unreported fallback locales are explicitly unknown.
- B-06.10: films, shows and episodes now force-refresh through the typed registry; bulk backend uses the same target-aware queue. UI availability remains client/catalog lane work.
- B-06.11: provider-specific rates, bounded concurrent title batches, pressure pacing and acquisition reuse implemented; throughput is not benchmarked.
- B-06.12: base indefinite recovery preserved; capped/sparser schedule and timeout classification improved; cooldown does not spend retries.
- B-06.13: already fixed; owner searches remain first in claim ordering.
- B-06.14: new screen policy region defaults to US; explicit empty region stays truthful.
- B-06.15: fixed unlocked field ownership and complete observed NFO evidence retirement, with regression-tested owner relationship lock preservation. Unchanged-file sidecar detection remains a scanner integration dependency.
- B-06.16: usable fields survive bad optional numbers, bare ampersands, URL-only NFOs/typed scraper hints, trailing scraper text and long optional lists. Invalid identifiers are isolated; conflicting identifiers remain bounded evidence for manual selection. Directives, depth, size and structural budgets stay enforced.
- B-06.17: optional provider lists are bounded per entry before validation, and invalid search results are isolated. Skipped provider fields are recorded in accepted evidence, fields and the owner state issues list.
- B-06.18: role/language/score diversity precedes TVDB truncation.
- B-06.19: automatic image ranking/request languages prefer library language, neutral, English, other.
- B-06.20: automatic retained source variants are reused without another download/encode; explicit owner artwork choices still fetch the current source; TMDB portraits use h632. Selections remain title-scoped; no catalogue person-schema redesign.
- B-06.21: official absolute numbering now maps to TMDB seasons and receives title/still metadata; alternate offsets remain explicit needs-order.
- B-06.22: adult search is now an owner setting, default false.
- B-06.23: provider watcher reduced to one second, artwork watcher to two seconds; owner artwork history streams instead of one concatenated allocation. Auth/access checks on image reads remain.
- B-06.24: base episode freshness retained; fresh matched children are skipped during parent fan-out rather than all republished. Fixture checks passed.
- V05 personal-state B-12 entries are outside this lane and belong to identity/personal-state lanes; no personal-state changes made.

# API changes
- New GET /v1/libraries/{id}/metadata/attention?cursor=&limit= (default 25, max 50): libraryId, entries, nextCursor. Entry includes entityId, item/show target kind, itemKind, parentId, supported actions, input-generation revision, reason, confidence and top three typed candidates with evidence/orders.
- New POST same route: kind, entityId, expectedRevision, action accept/search/id/dismiss, and action-specific candidateKey/provider/providerId/query/order. Uses existing selection/search/history and scoped authorisation. Typed stale-input responses use revision_mismatch (412); worker progress does not cause one.
- New GET/PUT /v1/admin/metadata/settings: revision, localArtworkLimitBytes, includeAdult and credential-presence booleans. PUT includes expectedRevision and optional tmdbReadToken/tvdbProjectKey; submitted secrets never return.
- GET /v1/items/{id}/metadata/screen and GET /v1/shows/{id}/metadata/screen moved to the typed registry. Accepted records now include bounded skipped-field notes and known field-language maps; existing client-core screen parsing consumes the unchanged state structure and field language values.
- POST /v1/items/{id}/metadata/refresh moved from direct ServeMux to typed registry; same queued receipt. New POST /v1/shows/{id}/metadata/refresh uses the same queue. Endpoint supports film/show/episode work.
- Typed maintenance settings gains backgroundSpeed; client-core maintenance parser/type updated. API contracts regenerated with apigen. New client-core metadata-attention and metadata-settings services/parsers exported.

# Schema changes
Baseline only; schema-sync run and passed its persistence check. Added metadata_runtime_settings singleton; screen_metadata_attention table/index/triggers; force_refresh work flag; provider/origin artwork-candidate index. Screen-policy region default becomes US. No title/file identity tables redesigned.

# Owner or viewer settings added or wired
- backgroundSpeed: shared maintenance choice gentle/normal/fast, default normal; wired provider pace, title concurrency and foreground pressure yield. Screen needed from client lanes.
- localArtworkLimitBytes: positive integer, default 2147483648; local artwork ingest reads it, max owner input 2^50; screen needed.
- includeAdult: boolean, default false; TMDB matching searches; screen needed.
- tmdbReadToken/tvdbProjectKey: optional owner strings; immediate adapter override and retry of blocked credential jobs, blank restores application value; read side is presence booleans; screen needed.
- Per-library provider and local metadata only remain existing settings; curated registry advertises actual video providers, MusicBrainz for music and local for all kinds. Existing library source/settings screens can render supplied choices.

# Needed from others
- Client lanes: needs-attention screen and four actions using exported services, setting controls, refresh action for show/episode, and i18n mappings for reasons/local issues.
- Naming lane: wire screenBase.Evidence/MatchEvidence, ordered file/embedded/folder title candidates with Source, Year, IDs and Edition; measured runtime already comes from catalog_assets. Only basenames leave the server. Scanner must reobserve sidecars when NFO/art changes even if media size/mtime does not; a confirmed complete observation uses VideoMetadataStatus=observed. Failures/outages must retain prior evidence.
- Identity/catalog lane: client action availability and eventual person-level portrait ownership; retained acquisition reuse is implemented here without changing person schema.
- Music-audiobooks lane: online audiobook sources and their provider choices; current MB descriptors that imply book matching need reconciliation there.
- Documentation owners: correct two-signal requirement, anime defaults, provider chains/credentials, NFO fields, quotas and refresh in metadata/contract.md, decisions.md, Catalogue architecture and relevant specs. D-52 export is not requested by this delivery list and remains a separate explicit source-write feature.

# Test notes for other environments
Completed: Linux server compile; Windows amd64 and Darwin arm64 cross-compile; go vet on metadata/metadataprovider/localmetadata/httpapi/dbwork/administration/catalog; npx --no-install tsc --noEmit from web; apigen and typed metadata/maintenance/OpenAPI coverage checks; schema-sync persistence check; three client-core attention/settings tests; focused Go regressions for attention/dismissal/revision/authorisation, NFO ownership/optional parsing and locked relationship preservation, capacity raise, retained source reuse, locale/show refresh, episode freshness, aggregate cast, absolute coordinates, bounded concurrent responses, recovery and existing transport/season guards. Provider race checks passed for concurrent response bounds, cache single-flight, concurrent TVDB auth and credential cache invalidation. Focused final regression selection across httpapi/metadata/metadataprovider/localmetadata/dbwork passed; one owner-artwork expectation remains preserved by bypassing reuse for explicit owner requests. No full suite or native build ran.

For live providers: use an isolated new database/library and authorised provider keys; import a yearless show, misspelled movie, numeric title and absolute anime across two seasons. Confirm titles/stills, candidate reasons, Fix Match/history/undo, changed language/country, immediate explicit refresh, and no remote traffic for local-only libraries. Switch TMDB/TheTVDB with crosswalk evidence; check artwork locale/season roles. During first import simulate 429 with Retry-After and a prolonged outage, restore service and confirm recovery without manual changes. Vary shared speed and verify start rates/concurrency remain under the listed caps and owner browse stays responsive.

For files: import NFO with directors/writers/tags/ratings, trailing URL, runtime text and one invalid optional number; confirm warning plus retained useful fields. Remove its plot tag and then the NFO, force scanner reanalysis, and confirm unlocked provider data replaces it while owner locks/undo survive. Lower artwork quota below a new image, verify capacity note, raise quota and rescan; existing covers must keep working. Scan film fanart/folder art, season posters and episode thumbs. Repeat a shared actor/source image and verify only one download.



Reproducible checks used (from worktree root unless specified):
- `GOFLAGS=-p=2 go -C server build ./...`
- `GOOS=windows GOARCH=amd64 GOFLAGS=-p=2 go -C server build ./...`
- `GOOS=darwin GOARCH=arm64 GOFLAGS=-p=2 go -C server build ./...`
- `GOFLAGS=-p=2 go -C server vet ./internal/metadata ./internal/metadataprovider ./internal/localmetadata ./internal/httpapi ./internal/dbwork ./internal/administration ./internal/catalog`
- `GOFLAGS=-p=2 go -C server test ./internal/httpapi -run 'TestTypedMetadataAttention|TestTypedMaintenanceSettingsRevisionAndAuthority|TestOpenAPICovers' -count=1`
- Focused metadata/local/provider checks include `TestAttention*`, `TestProviderFieldLocale*`, `TestMetadataSettings*`, `TestNFORetirement*`, `TestRemovedNFORelationships*`, `TestLocaleChange*`, `TestShowArtwork*`, `TestRetainedSource*`, `TestQueuedProviderArtworkGivesWay*`, `TestScreenConfidence*`, `TestMatchingUses*`, `TestAddingAnEpisode*`, `TestHeldEpisode*`, `TestAnOwnerRefresh*`, `TestAStaleDocument*`, outage recovery tests, `TestTypedHints*`, `TestForcedEpisode*`, `TestTMDBAbsolute*`, bounded list/body tests and NFO/artwork capacity/season tests.
- `GOFLAGS=-p=2 go -C server test -race ./internal/metadataprovider -run 'TestCredentialCacheInvalidation|TestProviderConcurrentBodiesRemainBounded|TestScreenCacheSingleFlightAndCancellation|TestTVDBLoginSingleflightAndConcurrent401' -count=1`
- `node --experimental-strip-types --test packages/client-core/test/metadata-attention.test.ts`
- From `web`: `npx --no-install tsc --noEmit`.


# Not finished
Implementation and focused verification are complete; final delivery is cumulative snapshot 003. Client screens and live-provider/physical-client acceptance remain outside this server lane. Scanner-driven detection of changed/deleted NFO/art beside an unchanged media file remains an explicit integration gap: this lane reconciles complete observations and preserves partial/outage evidence. The local artwork quota is adjustable and visible; automatic original pruning was not requested and is not implemented. Unreported provider field locales are truthfully unknown. No other known lane-local implementation gap remains. Alternative absolute offsets/community order mappings are not invented. No push/deploy/commit performed; cumulative snapshots are the delivery.
