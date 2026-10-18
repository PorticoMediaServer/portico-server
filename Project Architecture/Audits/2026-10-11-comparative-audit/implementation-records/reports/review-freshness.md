# Review freshness

## Outcome

Independent review of the merged freshness-server, search-sort and personal-state work is complete. Eight confirmed findings were fixed: **0 blocker, 0 high, 5 medium, 3 low**. One candidate was withdrawn after its regression passed on the starting code.

Devices now receive invalidations after active search batches, Continue Watching dismissals/replays and History-only changes. A reconnect behind a completely pruned event ring requests a quiet resync. Continue counts include the observation revision without changing Home or command counters. Browse text filters fold Unicode case and accents. Catalogue failure dates use the general API instant convention. The old range test now verifies quiet recovery rather than requiring a conflict.

Generation rollback/database-reopen, two FTS slots, poison recovery, scoped event delivery, Undo, History/privacy, normalization/paging, route construction and targeted checks passed. Two general SQL guards still fail on unchanged source outside these fixes; details below. No deployment, push, Git mutation or subagent was used. Starting tree: clean `1097d54`; host: personal Ubuntu `ehler-homelab-mini-1`.

## Decisions I made

- Preserve separate semantic, observation and command revisions. Explicit commands and replay clearing an effective dismissal are semantic changes; ordinary position/History observations remain observations.
- Publish one revision per affected library per bounded active search batch. Replacement slots stay quiet until the generation switch. An active no-op derivation may also emit an invalidation; batches and existing two-second coalescing bound this cost.
- History uses the existing private feed and shared client's global personal/progress dependencies. Its resource is `{kind:"personal",id:"history"}`. This avoids inventing another listener or invalidating Home on every History tick.
- Fold display-text predicates through the existing Unicode normalization logic. Preserve punctuation, SQL NULL, literal wildcard escaping, opaque collection selectors, and numeric/date/bounds handling.
- Preserve the console's numeric date model: only the failure timestamp serialization/parser boundary changes.
- Use the existing baseline/schema-sync policy. No compatibility migration, route alias or legacy wire parser was added.
- Context sync could not write its read-only lock directory. Relevant canonical project, standards and verification guidance was read directly.
- Withdraw R3: the watched command field already contains both watched and resume values. Retain its passing regression; do not change correct production code.

## Bugs

These are the scoped inherited IDs, checked against the current source and selected regressions rather than accepted from the authors' reports. External areas are identified explicitly.

| ID | Status |
|---|---|
| B-07.1 | Already fixed: durable failures, isolated retries and downstream checkpoints; selected poison/retry tests pass. |
| B-07.2 | Already fixed: retained browse rows and independent search slots; rollback/reopen, first-build and switch tests pass. |
| B-07.3 | Already fixed by identity's title/file model; no identity redesign in this review. |
| B-07.4 | Already fixed for quiet continuations and pure tick counters; R2/R8 fix missed semantic invalidations and a stale progress-sensitive count. |
| B-07.5 | Already fixed: search revisions and groups share a snapshot; concurrent-publication test passes. |
| B-07.6 | Already fixed: short titles and punctuation-normalized search; selected normalization/input tests pass. |
| B-07.7 | Already fixed: indexed CJK candidates plus substring verification; normalized-scope test passes. |
| B-07.8 | Already fixed: day/instant/fraction comparisons; selected date tests pass. |
| B-07.9 | Already fixed: Unicode letter seek uses sorttext.Letter; stored-sort/letter regression passes. |
| B-07.10 | Not fully fixed here: raw-display-title client bucket fallback still lacks authoritative article/explicit-sort information. Server-side quiet range recovery passes; client fallback contract remains programme work. |
| B-07.11 | Already fixed: the raw item delete path was replaced by title/version deletion rules; no deletion redesign here. |
| B-07.12 | Fixed here (R5): generic browse string/set predicates now fold Unicode case and accents. |
| B-07.13 | Not fixed here: backup still pins one snapshot; WAL-pressure qualification and backup policy are operations work. |
| B-07.14 | Not fixed here: old post-baseline migration files remain; global migration cleanup is outside this lane. |
| B-07.15 | Already fixed for scheduled dictionary/identity/generation/event pruning; referenced-term retention test passes. Database space reclamation remains operations work. |
| B-07.16 | Not fixed here: strict-mode holder tracing remains; no stuck writer was reproduced. Production alarm policy is operations work. |
| B-07.17 | Already fixed: natural title keys and real container years; selected search year/sort tests pass. |
| B-07.18 | Mitigated in the base by 16-unit batches/checkpoints; no weak-hardware hold-time measurement claimed. |
| B-07.19 | Not fixed here: live external database readers/writers are an operations/documentation issue. |
| B-08.2 | Not fixed here: legacy failure mapper still defaults to 400; broad error conversion belongs to API-platform. |
| B-08.6 | Already fixed structurally: private audience wakes, bounded coalescing, idle SSE without database reads. Selected audience/idle tests pass; 200-device capacity unverified. |
| B-08.8 | Already fixed: resource revisions and token-stable viewer identity; validator tests pass. |
| B-08.12 | Base retains gzip/asset/ETag fixes; generic registry post-composition 304 cost remains API-platform work. |
| B-08.17 | Already fixed for duplicate notification listeners and installation stream replacement; production feed/replacement tests pass. Capacity unverified. |
| B-12.1 | Already fixed: atomic bounded marks clear member progress; mark/Undo tests pass. |
| B-12.2 | Already fixed on server: detail continuation ignores Home age/dismissal; regression passes. Native screen qualification is outside this tree. |
| B-12.3 | Already fixed in timestamped shared writer: explicit/newer evidence fences; older-offline regression passes. |
| B-12.4 | Already fixed: the explicitly unmarked anchor is eligible; detail regression passes. |
| B-12.5 | Already fixed in shared history/count writer and merged downloads integration; physical offline uploads unverified here. R7 adds missing History feed hints. |
| B-12.6 | Already fixed: effective container completion feeds recommendations; season/Undo regression passes. |
| B-12.7 | Already fixed in timestamped server evidence writer; historical timestamp regression passes. |
| B-12.8 | Not reproduced: terminal evidence remains an engine contract. Truncated-file/player qualification is outstanding. |
| B-12.9 | Not a defect under the accepted credits-only policy; preserved. |
| B-12.10 | Base preserves library video policy unless explicitly overridden and explains it in preference help; no new setting. |
| B-12.11 | Already fixed: known-duration started threshold is the configured percentage. |
| B-12.12 | Already fixed: unchanged paused/brief observations do not move personal recency; paused regression passes. |
| B-12.13 | Already fixed: unknown duration continuation/bounds; selected regressions pass. |
| B-12.14 | Already fixed: noncanonical completion is accepted subject to fences; count/watched regression passes. |
| B-12.15 | Already fixed in source: premiere requires a recent addition and first available episode. |
| B-12.16 | Already fixed: recommendation day uses the server's local date. |
| B-12.17 | Base server respects search-history preferences; full client privacy qualification is outside this review. |
| B-12.18 | Already fixed by retained title identity; replacement/source qualification belongs to identity. |
| B-12.19 | Base repairs purged anchors; Undo declines changed membership; selected membership regression passes. |
| B-12.20 | Already fixed for ordinary observation/command/Home separation; passing stability tests preserved through R2/R7/R8. |
| B-12.21 | Already fixed: public popularity requires three eligible profiles. |
| B-12.22 | Already fixed: paused/opted-out plays excluded from shared counts; privacy regressions pass. |
| B-12.23 | Already fixed: episode continuation uses its linked segment duration; regression passes. |
| B-12.24 | Already fixed in source: merged Continue uses the largest participating cap. |
| B-12.25 | Already fixed: explicit resume creates activity without fake History; regression passes. |
| B-12.26 | Already fixed: separate short-lived Undo restores prior completion/progress/intent; selected Undo tests pass. |
| B-13.1 | Base social uses separate playback/governance counters; social redesign and device tests remain outside this lane. |
| B-18.4 | Already fixed server paging: 205-value aggregate regression passes. |
| B-18.28 | Already fixed: Home rows carry tagged destinations; server/HTTP Home tests pass. |
| B-19.2 | Base server/core viewer identity is token-stable; validator test passes. Physical native renewal qualification remains external. |
| A-08.24 / A-22.43 | Base unified authenticated stream publishes source hints; R1/R2/R4/R7/R8 close replay/publication gaps. Device transport acceptance remains external. |
| A-08.41 | R6 fixes the merged catalogue diagnostic instant omission. Specialized telemetry/subtitle/social clock exceptions remain with their owners. |

## API changes

- `GET /v1/admin/catalogue/failures`: `failures[].failedAt` changes from epoch-millisecond **number** to UTC RFC 3339 **string**, using `apiwire.Instant`. Registry remains owner-only with `ScopeAdmin`. OpenAPI/generated TypeScript schemas/types regenerated with apigen. Client-core converts back to numeric milliseconds for the existing screen. No path added or removed.
- Browse routes: textual predicates become Unicode-case/diacritic-insensitive; request/response shapes, numeric/date units and validation bounds are unchanged.
- `/v1/events`: no envelope/route shape change. History-only observations now emit private `progress.changed`, resource `{kind:"personal",id:"history"}`, revision and `data.observation` both decimal strings from `personal_activity_revisions`. Explicit clear/reset emits private `personal.changed` for that resource without a personal counter. Existing client global progress/personal dependencies consume both; shared hub regression proves observation hints do not move semantic membership.
- Active search publication and dismissal/replay reuse existing library/personal hints. Behind readers receive existing `stream.resync` when the retained ring is empty.

## Schema changes

Local baseline changes only, followed by schema-sync:
- `personal_revision_update` also recognizes a changed personal command revision, covering dismissal/manual edits without treating pure observations as commands.
- `progress_replay_membership` publishes a semantic viewer revision only when a new playback binding ends an effective dismissal.
- Two History observation triggers coalesce private History hints when its revision changes.
- Generated known schema synchronized. No new tables, columns or indexes; no post-baseline migration or compatibility adapter.

## Owner or viewer settings added or wired

None. Existing privacy, completion, Continue and sort settings retain their behavior. No new screen/string is needed.

## Needed from others

- **Base-health / Live TV:** `TestProductionStatementsPrepareAgainstTheBaseline` falsely treats five disposable spool statements as missing baseline `programmes` tables. The private spool CREATE TABLE is visible in `livechannels/programme_spool.go:38`; statement scanner does not recognize that loop. Repair the scanner's transient-schema coverage, not production schema.
- **Scan/metadata / base-health:** `TestProductionStatementsDoNotScanTheCatalogue` flags `catalog/sidecar_sources.go:62`: the artwork existence query can scan `inventory_objects` under its large-table model. Both sources match HEAD and were not edited here. Resolve the query plan in that lane; no scan-guard exemption was added.
- **Client/query contract:** acknowledged B-07.10 fallback needs a server-authoritative sort head/key and its web/native consumers for article/explicit-sort titles. This review did not add that cross-client feature. Server quiet reanchoring is independently checked.
- **API/operations/playback/subtitle/social:** inherited B-08.2, specialized instant/media clock exceptions, backup pressure/legacy migrations/holder diagnostics and engine terminal-evidence qualification remain outside this lane. Existing architecture/contracts describing preparing responses, raw title search or old general units need their documentation owners' updates.
- Synchronize updated generated contracts/client-core into native/Vega before release. No screen edits are required by this lane's timestamp boundary or existing event types.

## Test notes for other environments

Passed on Ubuntu with GOFLAGS=-p=2:
- Linux `go build ./...` at two production milestones; final source changes after it are test-only.
- Windows amd64 and macOS arm64 cross-builds of catalog, compactcatalog, dbwork, eventfeed and httpapi with CGO_ENABLED=0.
- Targeted vet of those five packages.
- Focused generation/first-build/two-slot/FTS/poison/retry/retention/event isolation, hidden-page and indexed endpoint checks.
- New database-close/reopen and rollback generation test verifies the old serving title exactly once, then successful replacement publication.
- Selected catalogue tests for Undo, expiry/profile scope/member changes, recency, rewatch/counts, privacy, older evidence, missing/unknown-duration/episode ranges, History access, Home destinations, Unicode/date predicates, bounds, CJK/short-title search, 205-value paging, exact rating scale, continuation and concurrent snapshot publication.
- Focused HTTP SSE/long-poll/private audience/idle heartbeat/revocation, owner diagnostic instant, Undo restrictions, Home, production-router, route scopes/lanes/OpenAPI checks.
- apigen and repeated schema-sync, including the latter's focused persistence checks.
- Client-core TypeScript no-emit; catalogue-failure file (4 tests), freshness-hub file (13 tests), wire-units file (4 tests). Web TypeScript no-emit also passes.
- `git diff --check` passes.

General SQL preparation/scan guards were run and **do not pass**, for the unchanged sources listed above; no changed query was flagged. This is not a whole-suite pass. No full suites, physical playback, proxy/network/200-device load, production upgrade, deployment or push was performed.

Cold acceptance:
1. On a fresh disposable database, start a populated browse/search rebuild. Read as owner and restricted viewer, interrupt/restart mid-build and repeat the search switch. Pass: old content remains readable, no duplicates/restriction leaks, replacement publishes, and owner failure/retry diagnostics stay coherent.
2. On two devices for one profile plus another profile, dismiss/restore a title, replay it, clear History, and send accepted History-only evidence. Pass: matching-profile screens reread quietly within about 30 seconds; the other profile learns no private state; pure ticks keep Home layout and command revisions stable.
3. Disconnect with a resume cursor, remove all retained events through the maintenance path, reconnect. Pass: a behind cursor gets resync and the current screen quietly catches up; a current/new connection remains quiet.
4. Exercise accented/non-Latin title/genre predicates, numeric/date filters, literal percent/underscore values and diagnostic dates in each synchronized first-party client. Pass: results match the corrected contract and diagnostic dates retain their exact milliseconds.
5. Qualify large-library batch/transaction times, 200 real subscriptions, native renewal/reconnect, and truncated-file terminal evidence separately. Cross-compilation and these fixtures do not establish those outcomes.

## Not finished

No confirmed finding from this lane is left unfixed. External SQL guards and inherited client/operations/API boundaries above remain. Real-device, weak-hardware, large-library and physical crash/power-loss acceptance remain unverified; the reopen test is deterministic database/worker recovery, not a power-cut test.

Snapshots 001 and 002 are cumulative compiling milestones. Final snapshot 003 follows final report/checks and captures the complete tree; no worktree edits follow it.

## Findings and fixes

Findings were recorded in the report before their corresponding production edits. These records retain the failing evidence and smallest fixes; paths/lines refer to the implicated starting-tree code where possible.

| Finding | Severity | File/line | Wrong behavior and evidence before fix | Fix and regression |
|---|---|---|---|---|
| R1 — Empty ring replay gap | Medium | eventfeed/feed.go:198 | A pruned empty ring keeps its maximum sequence but Read misses the gap. New regression: `after 0: want resync=true; got {Events:[] NextAfter:2 More:false}`. | Treat after < durable maximum with empty ring as resync. `TestPrunedEmptyRingResyncsOnlyBehindReaders` passes. |
| R2 — Dismissal invalidation missing | Medium | migrations/0001_baseline.sql:4681; catalog/personal.go:501 | A dismissal changes command revision only; semantic trigger omits it. Regression: `dismiss=true: semantic revision 2 -> 2, hints=0`. | Include actual command revision changes; dismiss/restore hints pass in `TestContinueDismissalPublishesSemanticChange`. Pure tick stability stays passing. |
| R3 — Suspected manual resume revision | Withdrawn | catalog/personal.go:208,530 | Initial suspicion compared only watched. Baseline regression PASS proves personalField includes progress too. | No production change. `TestExplicitResumeAdvancesCommandRevision` retains independent coverage. Not counted as a finding/fix. |
| R4 — Active search publication missing | Medium | compactcatalog/derive_search.go:35–46 | A source hint can arrive before queued index work; publication then emits no hint. Regression: `active search publication: revision 3 -> 3, hints 0`. | Bounded per-library active-batch publication. `TestSearchBatchPublishesActiveSlotChanges` also proves replacement slots stay private. |
| R5 — ASCII browse text predicates | Low | catalog/browse_expression.go:368,490 | Five title/genre assertions fail: amelie/AMÉLIE/COMÉDIE do not find their equivalent accented titles/genres. Author report explicitly left this item incomplete. | Deterministic null-preserving Unicode fold at the comparison boundary. `TestBrowseTextPredicatesFoldUnicodeAndAccents` passes, with literal wildcard and negative cases. |
| R6 — Diagnostic wire time omitted | Low | httpapi/freshness_routes.go:15; core/catalogue-failures.ts:18 | HTTP returns `1.760000000123e+12`; the literal RFC 3339 client row is skipped (undefined instead of 1760000000123). | apiwire.Instant + boundary conversion and regenerated contracts. HTTP instant regression and client file pass. |
| R7 — History-only hints missing | Medium | catalog/personal_activity.go:437–450; baseline.sql:4390–4392 | Noncanonical evidence writes History/revision only, and clear History emits no feed change. Baseline noncanonical regression reports zero progress.changed hints. | Private coalesced History observation hints and explicit command hint. `TestHistoryOnlyChangesPublishPrivateHints` and shared hub regression pass. First clear fixture lacked its session progress row and was corrected after the FK properly rejected it. |
| R8 — Continue count/replay membership stale | Medium | catalog/continue_count.go:6–10,23; baseline.sql:4151 | Existing replay-count regression fails `0 1 <nil>` after reopening; new replay hint test fails `membership revision 3 -> 3, hints=0`. | Count key includes observations; a narrowly guarded replay trigger advances semantic membership. Existing count and new `TestReplayingDismissedTitlePublishesMembershipChange` pass. |
| R9 — Test demands obsolete conflicts | Low | catalog/browse_test.go:269,280 | Reanchor test stops at `stale range admitted <nil>` because it still requires conflict errors the programme prohibits. Current engine already recovers correctly. | Assert recovered position/rows/fresh revision, exact anchor relocation and vanished-anchor fallback. `TestBrowseRangeReanchorsAfterAPublication` passes; no production change. |

## Merge with the base

Both conflicts are resolved on disk. No staging, commit or snapshot was performed; Git still reports unmerged index entries until Justin stages the files. Read the eight requested lane reports; retained the base's trash removal, networkpolicy/trusted proxies and credential/API integrations. Context-sync's lock remains read-only.

- `browse_test.go`: retained this lane's stronger quiet-recovery assertions for rows, position and fresh revision, exact anchor relocation and vanished-anchor fallback. They cover the base's same quiet-recovery behavior.
- `continue_count.go`: adopted the base's separate observation counter in the cache key. It keeps this lane's progress-sensitive invalidation, and avoids caching when progress moves during the count without returning a progress-only conflict. Removed the superseded summed-revision helper.

Regenerated contracts/OpenAPI with apigen and schema metadata with schema-sync; no generated files were hand-merged. The owner diagnostic route still declares ScopeAdmin; no route was added. Recursive grep, including dependencies, finds no Git conflict blocks (dependency license underlines, a scanner comment and nested TypeScript angle brackets are ordinary text). Whitespace check passes.

Passing checks: full server builds for Linux, Windows amd64 and macOS arm64 (cross-builds use CGO_ENABLED=0); catalog/httpapi vet; focused catalogue range/reanchor/operator/date/Unicode/count/dismissal/replay/history/command-stability tests; HTTP production-router, credential scope/lane/OpenAPI, owner diagnostics and validator tests; SQL preparation/scan guards; web and client-core TypeScript no-emit. Both general SQL guard failures from the original delivery are now resolved by the merged base. All requested merge checks pass.
