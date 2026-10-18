## Outcome
Natural title ordering, leading-symbol removal, language article handling and diacritic folding are implemented. Search indexes display/original/sort/alternative titles, canonical and credited people names, artists, authors, narrators, years, parent context and catalogue collections. CJK substring searches use indexed candidates, not a library-wide LIKE scan. Exact display titles rank first. Collections and Authors are published search groups. Search respects authorized libraries and existing content restrictions.

Genres and Series aggregate pivots now expose their real totals and page beyond 200 using bounded limits, signed cursors or direct ranges. Personal ratings use integer 0–10 storage/validation, with null distinct from zero. Recommendation thresholds and shared half-star labels/presets use that scale.

General catalogue/owner API boundaries now emit integer millisecond media times and RFC 3339 UTC instants. Shared parsers consume the new wire fields and convert to existing UI model units. Registered inline success schemas contain no old duration/progress field names after the final contract generation. This does not establish that every specialized or unregistered protocol is unified; the remaining exceptions are explicit below.

Server build, scoped Go tests, scoped vet, schema sync, contract generation and web TypeScript checks passed during implementation. Final checks passed: server build; scoped vet; normalized/container-year search, 205-series paging, integer-rating, personal-sort, playback-history, capture-expiry and registry tests; web TypeScript check; schema sync and contract generation. No commits, pushes, deployments or full suites were run. Execution host: personal mini-1. Context sync could not update its read-only lock directory; existing project guidance was read. The permitted helper produced no inventory or edits, so work continued without it.

## Decisions I made
- Natural numbers encode magnitude without parsing into a fixed-size integer. Leading zeroes compare as the same number and entity identity breaks ties. Numeric titles remain in the # jump bucket.
- Explicit sort titles retain their articles but receive the same punctuation/number/Unicode normalization. Unknown metadata languages retain their article behavior; absent metadata language defaults to English as before.
- Search remains library/kind-scoped inside FTS with the existing page sizes and per-group budgets. Added normalization is linear and bounded to 64 KiB per search document; credits, artist context and aliases have separate count bounds. CJK candidates require a true normalized substring, preventing reordered bigram false matches.
- Zero is a real personal rating; null clears it. Community/provider ratings retain their original provider scales.
- General API instants use UTC RFC 3339, including optional empty-string sentinels already represented internally as zero. General media positions/durations use integer milliseconds with safe-integer bounds. Civil dates remain dates; explicit operational settings (minutes, days, seconds) retain their named units. Engine and database duration calculations remain seconds internally.
- Unit conversions live at serialization/parsing boundaries. Request conversion still rejects unknown fields and old wire field names. No compatibility path for old databases or wire formats was added.
- Duration browse filters now use the `durationMs` field and integer operands. The compiler converts operands to internal seconds before binding, preserving the indexed comparison rather than multiplying the SQL column. Shared minute presets submit milliseconds.
- F-07.15 is database reclamation/size accounting, not search/sort content. Shared brief rule 7 and this lane’s machinery boundary place that with operations/freshness; it was not implemented here.

## Bugs
- **B-07.1** — Already fixed in the base for steady state per fix log; rebuild/failure machinery belongs to freshness-server.
- **B-07.2** — Not fixed here: serving previous generations during rebuild belongs to freshness-server. No readiness/generation machinery changed.
- **B-07.3** — Not fixed here: outside this lane’s ownership (scanning, persistence operations or freshness machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-07.4** — Not fixed here: outside this lane’s ownership (scanning, persistence operations or freshness machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-07.5** — Already fixed in the base; search still reads the revision through its snapshot.
- **B-07.6** — Fixed: normalized punctuated abbreviations, prefix/component tokens and one-character searches.
- **B-07.7** — Fixed: indexed CJK unigrams/bigrams followed by actual substring verification, including single characters.
- **B-07.8** — Already fixed in the base; date-period comparisons preserved.
- **B-07.9** — Fixed: seek uses the same Unicode-folded letter function as anchors, rather than the first UTF-8 byte.
- **B-07.10** — Not fully fixed: shared bucket matching now folds marks, skips punctuation and recognizes non-Latin letters. Raw-title article/explicit-sort reanchoring still needs a server sort-head consumer in client screens.
- **B-07.11** — Not fixed here: outside this lane’s ownership (scanning, persistence operations or freshness machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-07.12** — Not fully fixed: search exact/prefix boosts and facet typeahead now fold Unicode; arbitrary browse string predicates still use SQLite NOCASE/LIKE. Generic filter redesign is outside this lane.
- **B-07.13** — Not fixed here: outside this lane’s ownership (scanning, persistence operations or freshness machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-07.14** — Not fixed here: outside this lane’s ownership (scanning, persistence operations or freshness machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-07.15** — Not fixed here: outside this lane’s ownership (scanning, persistence operations or freshness machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-07.16** — Not fixed here: outside this lane’s ownership (scanning, persistence operations or freshness machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-07.17** — Fixed: title search uses the stored natural sort key; shows/books release-year sorts use their real year rather than a constant.
- **B-07.18** — Not fixed here: outside this lane’s ownership (scanning, persistence operations or freshness machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-07.19** — Not fixed here: outside this lane’s ownership (scanning, persistence operations or freshness machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.1** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.2** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.3** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.4** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.5** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.6** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.7** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.8** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.9** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.10** — Already fixed in the base per fix log; preserved client retry handling.
- **B-08.11** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.12** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.13** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.14** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.15** — Already fixed in the base per fix log; original-file MIME handling untouched.
- **B-08.16** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.17** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-08.18** — Not fixed here: outside this lane’s ownership (API/authentication/networking/realtime machinery); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.1** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.2** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.3** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.4** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.5** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.6** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.7** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.8** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.9** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.10** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.11** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.12** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.13** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.14** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.15** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.16** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.17** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.18** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.19** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.20** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.21** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.22** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.23** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.24** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.25** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-12.26** — Not fixed here: outside this lane’s ownership (personal-state/recommendation workflows or client screens); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.1** — Already fixed in the base per fix log; show selection untouched.
- **B-18.2** — Already fixed in the base per fix log; metadata editor untouched.
- **B-18.3** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.4** — Fixed on the server: Genres/Series aggregate pivots return accurate totals, bounded windows and signed continuations. Client windowing remains for client lanes.
- **B-18.5** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.6** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.7** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.8** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.9** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.10** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.11** — Already fixed in the base per fix log; no whole-repository preference checks run.
- **B-18.12** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.13** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.14** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.15** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.16** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.17** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.18** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.19** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.20** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.21** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.22** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.23** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.24** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.25** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.26** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.27** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.28** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.29** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.
- **B-18.30** — Not fixed here: outside this lane’s ownership (client screens and presentation); retained for the owning lanes. Archived finding not asserted as a current runtime reproduction.

Accepted feature findings: F-07.5 and the scoped portion of F-12.10 are implemented (original/alternative titles, people/year/context, CJK and prefixes, collections/authors). F-07.10 is implemented. F-07.15 remains with operations/freshness. A-08.41 is substantially implemented for the general API, with specialized/unregistered formats still requiring reconciliation. D-09’s pre-release choices are reflected in the new stored search content, sort derivation version and baseline rating form. Typo correction, general Latin infix matching, episode-coordinate parsing and global playlist/genre search were not part of the explicit deliverable list and were not added.

## API changes
- `GET /v1/search`: searchMode `token_prefix` → `normalized_prefix`; one-character queries are valid; Collections/Authors groups and catalogue-backed labelKey fields added. Title ordering uses sort keys; release-year ordering works on containers. Envelope/paging structure preserved.
- `POST /v1/libraries/{id}/browse`: aggregate pivots support limit/cursor/range and accurate pageInfo totals/start/hasMore/nextCursor, with title ascending/descending ordering. Duration predicate field `durationSeconds` → `durationMs`.
- `GET /v1/libraries/{id}/facets`: adds `total`; existing bounded typeahead list remains, while aggregate paging uses the complete cached facet set. Shared parser validates the total.
- Catalogue item/content/detail/history/title-list/source/marker wire fields: `duration` or `durationSeconds` → `durationMs`; `progressSeconds` → `progressMs`; `positionSeconds` → `positionMs`; `startSeconds`/`endSeconds` → `startMs`/`endMs`. Source durations and detail markers follow the same convention.
- Personal-state rating requests/responses, bulk-job argument schemas, userRating and detail action ranges: half-step 0.5–5 → integer 0–10. Detail action min/max/step are 0/10/1.
- Notifications, jobs, schedules, measurements, feedback, evidence/export/capture and playback-history instants: epoch millisecond numbers → UTC RFC 3339 strings (nullable fields remain nullable). Export/capture requests use the same string convention. Playback history preserves exact millisecond duration; CSV uses RFC 3339 instants and durationMs.
- Download options and deferred progress wire fields use durationMs/positionMs. Shared response parsers are updated; native deferred-progress writers must emit positionMs when packages are synchronized.
- Typed registry definitions and existing key scopes were added for the affected hand-registered general projections, retaining the original authorization/recheck handlers. No route paths were removed or kept as compatibility aliases. Original request admission categories were retained where the registry can express them. Supplemental YAML contracts were adjusted where touched; generated registry contracts are canonical.

Affected general projections registered in this lane:
- `GET /v1/content`
- `GET /v1/libraries/{id}/content`
- `GET /v1/libraries/{id}/browse`
- `GET /v1/libraries/{id}/discover`
- `GET /v1/collections/{id}/items`
- `GET /v1/items`
- `GET /v1/items/{id}`
- `GET /v1/items/{id}/detail`
- `PUT /v1/items/{id}/personal-state`
- `PUT /v1/items/personal-state:batch`
- `GET /v1/home`
- `GET /v1/home/rows/{id}`
- `GET /v1/saved-resources/{id}/content`
- `GET /v1/personal-history`
- `GET /v1/admin/play-history`
- `GET /v1/admin/play-history/summary`
- `GET /v1/items/{id}/download-options`
- `POST /v1/downloads/progress`
- `GET /v1/libraries/{id}/listening/selection`
- `GET /v1/people/{id}`
- `GET /v1/notifications/inbox`
- `GET /v1/notifications/unread-count`
- `POST /v1/feedback/reports`
- `POST /v1/admin/feedback/reports/{id}/status`
- `GET /v1/feedback/reports`
- `GET /v1/feedback/reports/{id}`
- `GET /v1/admin/feedback/reports`
- `GET /v1/admin/feedback/reports/{id}`
- `GET /v1/feedback`
- `GET /v1/feedback/{id}`
- `GET /v1/admin/feedback`
- `GET /v1/admin/feedback/{id}`
- `POST /v1/feedback`
- `PATCH /v1/admin/feedback/{id}`
- `GET /v1/notifications`
- `GET /v1/admin/console/jobs`
- `POST /v1/admin/console/jobs`
- `POST /v1/admin/console/jobs/{id}/{action}`
- `GET /v1/admin/console/schedules`
- `PUT /v1/admin/console/schedules`
- `GET /v1/admin/console/panels/{panel}`
- `GET /v1/admin/console/diagnostics/{lane}`
- `POST /v1/admin/console/diagnostics/capture`
- `POST /v1/admin/console/exports`
- `GET /v1/admin/console/exports/{id}`
- `GET /v1/admin/playback/history`
- `GET /v1/admin/console/alerts`

Remaining time/unit deviations to reconcile with owning lanes:
- `GET /v1/admin/telemetry`, `GET /v1/admin/telemetry/now`, `GET /v1/admin/transcode/capacity`: telemetry sample/point/observation times remain epoch milliseconds. Collector/history wiring was left with server operations; measurement-panel times are converted here.
- Subtitle resource/source/plan/document/selection routes (`/v1/items/{id}/subtitles*`, playback-session subtitle routes): decimal-string microsecond durations/offsets/cue positions remain; changing the precise subtitle clock and all transport adapters belongs with subtitle/playback owners.
- Watch Together, handoff and receiver routes (`/v1/groups/*`, `/v1/handoffs/*`, receiver/cast transport): decimal-string microsecond media clocks remain. These require joint server/client protocol changes.
- Legacy playback offers/session/chapters/delivery routes retain second-valued engine shapes; the playback lane is deleting those routes. The separate prepared-version facts (`Facts.duration`) and precision probe/trickplay/analysis microsecond documents still need their owning media lane’s wire review.
- Notification events/wait and other raw envelopes that carry changed Notice records should be included in the API lane’s final registry/error-contract reconciliation; their shared Notice parsers already consume RFC 3339.

## Schema changes
Edited only the baseline (no compatibility migration):
- `catalog_search_documents` retains display title and adds `search` (expanded normalized index text) and `normalized` (substring verification text).
- `catalog_search_titles` indexes search/kind rather than title/kind. Deletion/replacement writers now use the matching search text, including physical entity deletion cleanup.
- Collection and Author kind rows become searchable.
- People-name insert/update triggers normalize their index content. Search backfills refresh credited people names in bounded entity batches.
- `personal_items.rating` and `catalog_personal_sort_members.rating` are INTEGER with 0–10 bounds, nullable.
- Search derivation version 2 → 3. Browse-row version 1 → 2; its content recomputes stored entity keys before rebuilding rows/counts. Parent/collection/artist/song changes queue the search documents they affect using the existing mechanisms. No worker/generation/event machinery was changed.
- Ran `python3 scripts/schema-sync.py`; known columns/schema checks are synchronized.

## Owner or viewer settings added or wired
No new switches or settings. Existing sort-title/metadata-language facts feed sorting. Existing rating/detail action ranges and duration filter presets use the new units/scale.

## Needed from others
- Client lanes: window Genres/Series using pageInfo totals/cursors/ranges; synchronize shared packages into native/Vega; display rating/2 as half-stars, preserving null versus zero. Render the added search groups and their catalogue label keys.
- Client reanchoring: consume a server sort-head/key when the displayed title differs from its article-stripped or explicit sort title; Unicode/punctuation matching alone cannot infer those facts.
- Freshness-server: merge the derivation content/version changes with generation workers so old content continues serving during rebuild, and preserve source invalidation for accepted aliases. This lane intentionally did not change readiness guards.
- Personal-state clients: emit progressMs and integer ratings, including offline intent and deferred-download writers. No server personal-state conflict/scanning workflow was redesigned.
- API/operations/playback/subtitle/social owners: finish the time deviations listed above and retire duplicate/stale handwritten contract documents. Existing Catalogue/Metadata architecture descriptions of title-only search, lexical sort keys, 0.5–5 ratings and second-valued general fields need updating to the generated contracts and final programme behavior.
- Operations/freshness: F-07.15 reclamation/space-accounting remains outstanding.
- Saved-resource collections are a separate personal model; catalogue collections are indexed here. Global Saved-resource collection search needs its personal/share authorization projection and was not added by borrowing catalogue permissions.

## Test notes for other environments
Checks performed on Ubuntu personal mini-1, with GOFLAGS=-p=2:
- `go build ./...` (allowed server build).
- Scoped `go vet` of changed Go packages.
- Focused sort, search, deletion/update, per-kind year ordering, people permission filtering, browse paging, personal-rating, personal-sort, playback-history, capture-expiry, API-wire and registry tests. The 205-series fixture traverses three pages and a direct range without duplicate/lost rows.
- `python3 scripts/schema-sync.py`, `go run ./cmd/apigen`, `npx tsc --noEmit` in web.
- Single shared-client search test file (24 tests) and wire-unit test file (3 tests), all passing in completed runs. No full suite was run.

Cold acceptance procedure for client lanes (all supported first-party platforms):
1. Start a fresh database from the new baseline, then populate films with Rocky 2/10, leading punctuation, accented names and language articles. Verify title order and exact letter jumps, including Cyrillic/CJK and numeric #.
2. Populate original/accepted alternative titles, casts/directors, music artist context, book authors/narrators, years and catalogue collections. Search spiderman, wall e, MASH, X, Japanese/Chinese/Korean middle substrings and typed prefixes. Exact display titles must precede context matches. Restricted accounts must never see another library or hidden title/person context.
3. Add more than 200 genre and series values; traverse/window to the last value and compare total with the fixture. Repeat under restrictions. A stale cursor/range must be rejected by the existing fence; a fresh range must not lose entries.
4. Rate an item with 0, 1, 9 and 10; clear with null; verify persistence, half-stars, filters, sort buckets and restart receipts. Fractions, negative values, 11 and unknown mutation fields must be rejected.
5. Exercise catalogue/details/markers/title list, owner jobs/history/feedback, notification inbox, capture/export and deferred downloads using generated wire fields. Validate UTC string dates and integer milliseconds, then verify UI durations/seeks/progress are unchanged after parser conversion.
6. With freshness-server merged, rebuild on a populated library while browsing/searching and watching; previous content must keep serving and normalized search/sort must converge after publication. Large-library latency/capacity and real-device behavior have not been established by these unit checks.

## Not finished
- A-08.41 is not completely unified across the specialized/unregistered telemetry, subtitle, social, legacy playback and prepared/probe/analysis shapes listed above. Those owner-lane changes and all native writers need reconciliation before release.
- Global Saved-resource collection search and raw-title article/explicit-sort client reanchoring are not complete.
- Client screens have not been edited or exercised. Genres/Series windowing and half-star presentation need the client lanes; shared parsers/types/presets are implemented here.
- Database reclamation/size reporting (F-07.15), production rebuild availability and hardware/device acceptance remain outside this lane’s implementation.
- Broad performance/capacity measurements and a full test suite were deliberately not run.

Snapshots: 001 and 002 retained early cumulative milestones; 001 preceded a compile correction, 002 was compiling. 003 records the compiling catalogue API/paging milestone. Final snapshot 004 is being saved as the final cumulative source checkpoint; no worktree edits will follow it.

## Merge with the base

All listed file conflicts are resolved, preserving both lanes’ behaviour and following base removals. No staging, commit or snapshot was performed. Git’s merge index is deliberately left for Justin to stage and commit. Context refresh was attempted but the context-sync lock remains read-only.

- `browse.ts`: keep row-tolerant, deduplicated facets and the search-sort total/page bounds.
- `console.ts`: keep tolerant reply/job/list/fact parsing with RFC3339 instant conversions.
- `detail.ts`: keep tolerant optional parts and the base's missingSince, edition and partIndex alongside integer rating and millisecond conversions.
- `downloads.ts`: keep new history_paused outcomes, echoed invalid identifiers and tolerant rows with positionMs conversion.
- `library-content.ts`: keep tolerant display fields/play actions, normalized media units and integer 0–10 user ratings.
- `listening.ts`: keep bounded known-action filtering with millisecond resume conversion.
- `notifications.ts`: keep neutral diagnostics/status rules and tolerant threads with RFC3339 conversion per report/event.
- `personal-saved.ts`: keep strict server/viewer fences, tolerant lists and history positionMs conversion.
- `play-history.ts`: keep tolerant/deduplicated entries and optional totals with durationMs/positionMs conversion.
- `saved.ts`: keep tolerant navigation/play actions with startMs conversion.
- `search.ts`: keep tolerant capabilities/results, scope/query fences, authors/collections, label keys and normalized_prefix.
- `downloads.openapi.yaml`: follow base deletion; downloads now have typed registry contracts generated by apigen.
- `console.go`: retain projection delegation and the guard avoiding duplicate typed console settings routes.
- `downloads_routes.go`: follow the base's complete typed download registry; existing downloads wire adapters provide normalized units. Remove obsolete download projections.
- `foundation.go`: register both search/unit projections and the base's conversion settings before Freeze.
- `router.go`: keep extracted item handler/projection; follow base removal of obsolete playback-session registration and retain networkpolicy/trusted-proxy handling.


Merge follow-through: optional display durations now degrade individually, while strict unit parsing remains for receipts and writes. Saved playback conversion failures leave only the play action out. Added explicit lane-table entries for projected console/notification/feedback reads and mutations; every projection obtains its mandatory Requires scope from the exhaustive base scope table (search/browse/facets require read; personal writes require playback; console mutations require admin). The focused scope-completeness test passes. Added documentation for the bundled Cast receiver assets after coverage found the existing route undocumented. Updated old test fixtures at the mock-server boundary, old natural-title/search expectations and the download progress request to the normalized contract. Added literal-wire regressions for tolerant millisecond display fields and listening resume with unknown actions.

Verified: apigen and schema-sync (including persistence check); focused HTTP tests for registry/access/scopes/lanes/OpenAPI, search, browse, downloads, notifications/feedback, capture, playback history, MCP and conversion settings; focused apiwire/catalog/downloads/operations tests for normalized units, search/order/paging/facets, integer ratings, offline progress and playback history; 234 selected client-core tests; web `npx --no-install tsc --noEmit`; `git diff --check`. Recursive grep finds no conflict markers in repository sources. An unrestricted dependency scan found only a Markdown heading underline (`=======`) in node_modules/js-tokens/README.md, with no opening/closing markers; it is not a merge conflict and was preserved.

Final `GOFLAGS=-p=2 go build ./...` passes for Linux amd64, Windows amd64 and macOS arm64. Targeted `go vet` passes for httpapi, catalog, downloads, operations and apiwire. The unrestricted recursive grep for opening/closing markers also returns zero matches, including dependencies. No full test suite, deployment or push was performed.
