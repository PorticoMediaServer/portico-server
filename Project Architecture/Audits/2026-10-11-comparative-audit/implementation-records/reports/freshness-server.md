# Freshness server

## Outcome

Browse keeps serving its previous relational generation during a derivation rebuild. Ancillary sort/category/contribution backfills no longer discard a readable restricted page; unavailable anchors are simply omitted. Content documents with an unpublished restriction class answer quietly with protected empty content. Rows are retained on first change, in relational tables with matching order indexes; there is no whole-library copy at startup. Search builds an independent FTS generation and switches it when enumeration finishes. A first build serves committed work as it arrives. Normal derivation batches are now 16 units, with a downstream-worker checkpoint every fourth turn.

A failed title no longer prevents a rebuild finishing. Its reason and attempt count are durable; other titles continue, and the failed title retries with backoff. Owners can read the latest 100 failures through the typed admin API.

Browse ordering, semantic personal state, progress observations, personal commands and library-directory changes have distinct counters. A position tick preserves the title's personal command revision, Home revision and personal snapshots. Valid signed continuations survive publications and expiration quietly; ranges fall back to the nearest available position when their anchor disappears. Search always runs its groups and revision reads in one snapshot.

One authenticated `/v1/events` feed carries library, personal, progress, settings, notification, receiver and existing scan/playback events. Private profile/device hints wake matching subscribers in memory. Library hints are checked against current access in the read snapshot. Hints are coalesced by audience/resource and dispatched at most every two seconds. SSE heartbeats do no database work; the feed has no safety polling timer. Authorization writes wake streams, token expiry bounds their lifetime, and an installation's new stream retires its old connection. Dedicated notification listeners are removed.

Removing a library now clears derived library/class rows in bounded batches. Daily maintenance walks bounded windows of unused people, terms, attributes, labels and orphaned identities, removes retired browse generations, and expires event history. FTS documents own insert/update/delete triggers, including cascaded title deletion, so both search indexes stay consistent.

Client-core has typed freshness subscriptions, stable viewer-scope identity across token renewal, shared receiver subscriptions, and reliable resync delivery. No client screens were built in this lane.

## Decisions I made

- Use copy-on-change relational retention for browse and two physical FTS slots for search. This avoids copying millions of rows before serving starts. The serving browse generation remains stable until the replacement is ready; current source facts, hydration and authorization checks still apply.
- Ancillary projections serve their maintained data during backfill; unfinished anchor summaries are omitted. A new restriction class without any published generation stays protected and empty rather than returning a preparing response. Playback admission still keeps its authority checks.
- Keep cursor signatures, viewer/query binding and input bounds. An expired signed continuation becomes a positioning hint and receives a fresh bounded expiry, rather than surfacing a stale-list error.
- Preserve command revisions/snapshots on pure progress observations. First playback activity and watched changes still advance semantic state.
- Keep long poll only as the fallback on the unified endpoint; do not retain duplicate notification endpoints.
- Library directory hints disclose no inaccessible library ids. Their own monotonic counter covers creation, removal, retirement and name/kind edits.
- Fixed maintenance defaults: daily sweeps, 64 candidates per transaction, 100 ms pacing; event history is at most the existing 20,000-event ring plus its append checkpoint allowance, and at most seven days old after scheduled pruning. No new settings wall.
- Minimal social wiring only: move its receiver-wait limiter to a dedicated helper and bridge its existing receiver bus into the shared feed. No Watch Together behavior or counters changed.
- Context sync could not create its read-only lock; relevant shared guidance was read directly. Host: personal Ubuntu `ehler-homelab-mini-1`. No commits, pushes or deployment.

## Bugs

| ID | Status |
|---|---|
| B-07.1 | Fixed: poison rows are durable, skipped during enumeration, retried later; downstream checkpoints continue. Steady-state isolation was already fixed on the base. |
| B-07.2 | Fixed: serving browse generation and independent search generation; first build is readable as it progresses. |
| B-07.3 | Already fixed by identity; left intact. |
| B-07.4 | Fixed for this lane: order/semantic/observation counters, silent continuation/range recovery. |
| B-07.5 | Already fixed through snapshot revision reads; strengthened the public Search method to create its own snapshot when needed. Concurrent-publication test passes. |
| B-07.6 | Not fixed here: title/query normalization belongs to search-sort. Still present on this lane's base. |
| B-07.7 | Not fixed here: CJK matching belongs to search-sort. Still present on this lane's base. |
| B-07.8 | Already fixed on the base (day/instant date comparisons). |
| B-07.9 | Fixed: seek uses the server's Unicode-folded letter function, rather than the first UTF-8 byte. |
| B-07.10 | Not fixed here: client bucket matching needs the authoritative sort key/language contract from search-sort. |
| B-07.11 | Already fixed by identity: raw delete route removed. |
| B-07.12 | Not fixed here: filter/search Unicode comparison belongs to search-sort. |
| B-07.13 | Not fixed here: backup snapshot/WAL-pressure work belongs to ops-packaging. No runtime backup-pressure claim made. |
| B-07.14 | Not fixed here: existing post-baseline migrations remain; squashing/removing those is a persistence/ops integration change outside this lane. All my schema edits are in the baseline. |
| B-07.15 | Fixed: bounded dictionary/identity/generation/event sweeps; FTS lifecycle follows document deletion. Identity already removes a title's ledger at final removal. |
| B-07.16 | Not fixed here: write-gate holder alarms belong to dbwork/operations. Existing diagnostics expose held duration; no real leaked gate reproduced. |
| B-07.17 | Not fixed here: search sort definitions belong to search-sort. |
| B-07.18 | Mitigated: server batches reduced from 64 to 16, downstream checkpoints interleaved, normal background pacing retained. Weak-hardware hold-time acceptance remains unmeasured. |
| B-07.19 | Not changed: external SQLite reader/writer pressure is an operational/documentation risk; owner documentation belongs to ops-packaging. |
| B-08.2 | Not fixed here: the legacy global failure mapper still defaults unknown errors to 400. API-platform owns the broad error/validation conversion; the new typed route uses registry errors. |
| B-08.6 | Fixed for private hints and idle feed work: audience-filtered wakes, coalescing, no idle SSE authentication queries or feed safety polls. Library hints still wake subscribers for live per-library permission checks. Capacity under 200 real devices remains unmeasured. |
| B-08.8 | Fixed: grid validators use resource catalogue/semantic/settings revisions and stable viewer identity, without token hash or process-wide write count. |
| B-08.12 | Encoding quality, weakened gzip ETags and content-hashed chunk caching were already fixed on the base. Generic registry 304 composition cost remains API-platform work; grids now validate before composition against relevant state. |
| B-08.17 | Duplicate notification listeners removed; one installation stream replaces an older connection. Existing realtime admission bounds retained. Native bridge must share the feed. |
| B-12.20 | Fixed: pure observations preserve Home/command revisions and do not create personal snapshots. Progress itself remains current. |
| B-13.1 | Not changed: Watch Together command/observation fences belong to social; expressly excluded by the lane brief. |
| B-19.2 | Server/core portion fixed: token-independent validators and cached viewer-scope objects. Native hook/session integration remains native-app's responsibility. |
| A-08.24 | Library/personal/progress/settings changes now published; existing scan progress retained. |
| A-22.43 | Core supports one shared authenticated SSE feed. Native app must provide its stream opener instead of explicit ten-second polling. |

## API changes

- `GET /v1/events` (registry, `Requires: ScopeRead`): SSE remains the default. Resume with `Last-Event-ID`; fallback `?after=<decimal-id>&waitSeconds=0..25`. JSON was `{events,nextAfter}`, now `{events,nextAfter,more}`. `more` means continue immediately; server streams/long polls drain hidden pages without waiting for another write. The unused `topics` parameter is removed.
- SSE controls: `stream.opened` establishes a resume position; `stream.position` advances through filtered events; `stream.resync` requests a quiet reread; `stream.closed` is now a valid event envelope with `data.reconnectAfterMs:1000`. Heartbeats are comments every 25 seconds. Maximum life is the lesser of 30 minutes and token/family expiry.
- New `GET /v1/admin/catalogue/failures` (registry, owner only, `Requires: ScopeAdmin`, admin scope `library`): `{failures:[{domain,entityId?,title?,reason,attempts,failedAt}],more}`. Latest 100 failures; `failedAt` is epoch milliseconds. This is a bounded diagnostic view, not an all-record export.
- Removed `GET /v1/notifications/events` and `GET /v1/notifications/wait`. Ordinary inbox, count, capability and mutation routes stay; consume `notification.changed` on the unified feed.
- Presentation viewer fences across catalogue, Home, search, details and console documents now use stable server/viewer identity rather than token hashes; notification scope is stable too. Authentication and token-bound security checks retain the actual token hash.
- Personal observation handling now preserves the returned personal command `revision` on a pure progress tick.
- Contract generation (`go run ./cmd/apigen`) updated OpenAPI, contract index and TypeScript contracts. Notification/social API documentation updated.

### Exact event contract for client lanes

All events retain `{id,type,at,resource:{kind,id},revision,data?}`. Feed `id` is its own decimal resume sequence, not a domain command revision. Counter values in `data` are decimal strings; do not parse them as floating point.

| Type | Audience/resource | Data/counter behavior | Client action |
|---|---|---|---|
| `library.changed` | Authorized viewers, `library:<library-id>`; resource library | `content` and `contentOrder`; coalesced invalidation covers title additions, updates, removal, missing/restored files and generation publication | Quietly reread affected library/search/detail documents. |
| `libraries.changed` | Authenticated devices; resource catalogue `libraries` | Monotonic `directory`; no library ids/names disclosed | Reread the authorized library directory quietly. |
| `personal.changed` | Only the authority/account/profile's devices; resource library | `personal`, `observation` | Reread personal decorations, affected personal lists and Home. |
| `progress.changed` | Same private profile audience; resource library | Same counters; observation moves, semantic personal counter stays | Reread visible personal/progress state; preserve Home/list layout and command revision. The hint is an invalidation, not a progress patch. |
| `settings.changed` | Server settings: all authenticated devices, resource settings `server`; preferences: private profile, resource settings `preferences` | Document revision in envelope; preference data includes `scope` and `deviceClass` | Reread applicable settings/preferences quietly. |
| `notification.changed` | Existing profile/admin inbox audiences | Inbox revision in envelope | Refresh inbox/count; do not open another listener. |
| `receiver.inbox_changed` | Existing private receiver profile audience | `{kind,receiverId?}` from the existing bus | Shared ReceiverHost rereads its inbox; commands still come from authenticated inbox data. |
| `library.scan.updated` | Existing authorized library audience | Existing scan revision/state/found; existing producer's ten-second progress throttle | Keep using LibraryScanTracker; no new scan implementation here. |

`subscribeFreshness` and `parseFreshnessChange` are exported from client-core. `ReceiverHost.events` now takes the shared `EventsClient`, not a second stream opener. The feed can replay hints after reconnect; a `stream.resync` must reread visible data while retaining the current screen. The delivery transport alone does not implement screen loading/error behavior.

## Schema changes

Baseline-only changes, followed by schema sync:

- `catalog_generations`; relational `_previous` tables, `_serving` views and retention triggers for browse rows/blocks/buckets/counts/memberships/metrics, Home buckets, availability, facets, related rows and movie category rows/summaries. Previous rows have the original order indexes prefixed by generation. Serving browse views expose normalized order columns so SQLite merges indexed runs for title, added, year, duration and rating instead of materializing a library. Before-insert retention checks the original primary key so UPSERT updates retain the original row rather than an absence marker.
- `catalog_search_documents_next` / `catalog_search_titles_next`; FTS lifecycle triggers on both slots.
- `catalog_derivations.active_slot`, `cleaning`, `first_build`; generation publication invalidations.
- `catalog_derivation_failures` plus recent-failure index.
- `library_revisions.order_revision`; `viewer_revisions.observation_revision`; semantic trigger guards.
- `catalog_directory_revision`; coalesced directory triggers.
- `api_change_pending`; transactional change-hint triggers.
- `catalog_retention_cursors`; role/department reference indexes and a library index on browse counted rows.

Development databases need recreation under the pre-release rule. No compatibility migrations were added.

## Owner or viewer settings added or wired

None. Existing personal preferences are now pushed to the profile's devices when saved. Maintenance/coalescing/batch limits are implementation defaults, not user settings.

## Needed from others

- **web-app / native-app:** use the shared event feed for the event table above, retain visible screens during quiet rereads, delay failure presentation, and provide native authenticated SSE. Remove notification-specific listener wiring. Pass the shared EventsClient into ReceiverHost.
- **console:** surface `/v1/admin/catalogue/failures` and its retry reason/attempt count. No owner screen was added here.
- **search-sort integration:** its new search-document/FTS columns must be applied to BOTH physical slots and all index lifecycle triggers. If it adds browse/category columns, update the corresponding previous table/view/index definitions too. Keep generation switching and the worker lifecycle from this lane when resolving overlapping derivation edits. Its B-07.6/.7/.10/.12/.17 work remains its own.
- **personal-state integration:** preserve the semantic-trigger guards and the observation-vs-command change in AcceptPlaybackProgress when merging its state rules. This lane changed revision/snapshot behavior only.
- **api-platform:** global legacy error mapping B-08.2 and generic registry post-composition 304 cost.
- **ops-packaging / dbwork:** backup snapshot-pressure bounds, post-baseline squash policy, write-gate alarms, and documentation advising use of backup copies for external database inspection.
- **social:** own Watch Together fences remain untouched. The only shared seam changes are the receiver limiter and unified receiver-event subscription.
- Read-only architecture `03 Catalogue and Metadata.md` still describing ErrBuilding during derivation rebuilds needs correction. Audit/schema migration descriptions are dated evidence, not the new contract.

## Test notes for other environments

Passing checks completed during implementation:

- Linux `GOFLAGS=-p=2 go build ./...` at compiling milestones.
- Focused compactcatalog generation/switch/retry/retention tests; steady-state poison and unchanged-availability tests.
- Focused catalog browse/continuation, Home anchor, concurrent search, personal observation/command, paused history and library-removal tests. Removal now asserts both library rows actually disappear and state still restores.
- Focused apievents/eventfeed coalescing, rollback/profile isolation, hidden-page paging, indexed endpoints, replacement and permission tests.
- Focused HTTP SSE delivery/replacement/revocation, removed notification routes, owner-only failure view, idle-heartbeat statement count, and token/resource validator tests.
- `TestProductionStatementsDoNotScanTheCatalogue` passes after tightening three newly flagged queries.
- Web TypeScript no-emit check; client-core freshness test file (3 tests).
- Final Linux build and focused vet passed after the last runtime changes.
- Final generation checks passed: previous browse rows, two search switches, first-build committed search batches, UPSERT retention and all five indexed browse sorts (no temporary order trees).
- Final restricted browse/content checks passed with ancillary backfill flags unset and with no published restriction class; existing live restriction checks still hide newly blocked titles. Content continuation tests now expect quiet publication recovery.
- Final dictionary retention test verifies an orphan is removed and a referenced term stays. Poison rebuild/checkpoint/backoff tests pass.
- Final HTTP tests passed for unified stream delivery/replacement/revocation, zero idle-heartbeat reads, owner diagnostics, token-stable/resource-scoped tags and changed-restriction invalidation.
- Final schema sync, statement preparation/scan guards, trigger index checks, typed API generation, TypeScript no-emit check, client-core freshness tests (3), and diff whitespace check passed.
- Snapshots 001–003 captured compiling milestones. Snapshot 004 captured the verified runtime milestone. A final reference sweep then corrected stale notification-route documentation, scope entries and acceptance fixtures. Final cumulative snapshot 005 captures the completed tree; the original lane delivery ended there; the later authorized merge is recorded below.

Cold acceptance steps:

1. **Version rebuild at scale:** fresh disposable server/database, populate a large film/TV library, record browse/search results and maintenance gate hold times, raise search/browse derivation versions, restart. Keep browsing/searching with owner and restricted profiles throughout. Pass: prior content remains available, no preparing/conflict errors; new generation becomes visible at completion; startup does not copy the full library. Check FTS integrity after title removal and a second rebuild. Fail: offline/blank previous screen, missing restricted checks, corrupted FTS or a rebuild that never completes.
2. **Poison title/restart:** inject one deterministic derivation failure in a disposable build. Pass: other titles and downstream workers finish; owner failure view shows reason; restart preserves the failure; fixing the cause clears it on retry. Fail: all workers stall or restart forgets the failure/retries it continuously.
3. **200 browsers/devices and weak hardware:** one shared feed per installation, simultaneous scan and playback. Record SQLite statements, dispatch delay, write-gate hold durations and UI response times. Pass: private progress does not wake unrelated profiles; changed screens learn within roughly 30 seconds (normally a few); no repeated idle SSE reads; console/browse stay reachable. No measured claim at this scale is made here.
4. **Native/proxy/network:** native streaming through its authenticated adapter, nginx/Tailscale/IPv6 routes, idle longer than heartbeat interval, token renewal and network reconnection. Pass: comments flush, renewed services keep their identity and visible/focused cards, old stream retires, revoked/expired authority ends the stream, current screen survives reconnect. Test constrained-platform unified long poll separately.
5. **Windows/macOS:** run focused tests/build on the appropriate host, scan/delete/restore files, remove a library containing restricted classes and audio group projections. Pass: no hidden empty library remains, personal state survives, and scheduled cleanup leaves all FKs and both FTS indexes valid.
6. **Retention:** disposable database with referenced and orphaned dictionary rows, old events and a retired generation; run the daily worker/check diagnostics. Pass: only unused rows and expired history disappear, work stays bounded and yielding, current generation and references remain intact.

## Not finished

- First-party screen quiet refresh and delayed error presentation, native SSE transport wiring, and the owner failure screen belong to client/console lanes and are not implemented here.
- B-07.6/.7/.10/.12/.17, B-07.13/.14/.16/.19 and the broad B-08.2 mapper are explicitly outside this lane's permitted area; see Needed from others. Watch Together B-13.1 is expressly excluded.
- No full suites, native builds, deployment, physical-device checks, real proxy/network acceptance, or million-title/200-client load acceptance were run. Per-unit derivation time can still be long; 16-unit batches mitigate rather than prove weak-hardware latency.
- The diagnostics API shows the latest 100 failures, with `more`; it does not expose pagination/export of the full failure ledger.
- Snapshot 001: Keep browse and search available during catalogue rebuilds. Snapshot 002: Push scoped changes through one feed and keep progress revisions separate. Snapshot 003: Keep browse ordering indexed and viewer identity stable during renewal. Snapshot 004: Serve published restricted content through backfills and retain rows correctly on upsert. Final snapshot 005: Use the shared event feed throughout notification documentation and checks. Final build and focused verification passed. The original lane delivery ended at snapshot 005; the later authorized merge is recorded below.


## Merge with the base

- `remote-playback.ts`: kept the base's destination-session controls, serialized inbox reads and bounded handoff processing. Kept this lane's shared `EventsClient` subscriptions and disposal. Receiver hints now retain the base's receiver-id filtering and remain active during playback; reconnect/backoff stays owned by the shared feed. Only connection-state changes wake the receiver poll, so unrelated event frames do not trigger inbox reads. Updated receiver event tests to the shared envelope/feed contract.
- `route_lanes.go`: followed the base's current route vocabulary (including new provider/subtitle/download/credential routes and replacement paths), retained removal of the two notification listeners, and classified the catalogue-failures route. Its registry scope remains `ScopeAdmin`; `/v1/events` remains `ScopeRead`. Removed the superseded `event_stream_exists` error from the event route metadata.
- Regenerated contracts with `GOFLAGS=-p=2 go run ./cmd/apigen` and schema metadata with `GOFLAGS=-p=2 python3 scripts/schema-sync.py`; no generated file was hand-merged.
- Verification passed: Linux, Windows amd64 and macOS arm64 `GOFLAGS=-p=2 go build ./...` (cross-builds use `CGO_ENABLED=0`); `go vet ./internal/httpapi`; targeted HTTP lane/scope/OpenAPI/event/revocation/diagnostic/validator tests; 17 receiver and remote-playback tests; web `npx tsc --noEmit`; whitespace check; recursive grep found no conflict markers. The OpenAPI coverage check now recognizes the scope table’s static Cast app entry as assets rather than an API operation.
- That merge was resolved on disk without staging, committing or taking a snapshot; its index resolution was left to Justin.


### Subsequent base merge: scan exports, personal evidence and typed Home

- `index.ts`: retained both the freshness exports and the base's scan-issue services/types.
- `personal_activity.go`: preserved the base's meaningful-activity test, historical timestamps, chronology/manual fences, canonical-writer rules, completion deduplication and audiobook resume. Pure position/recency observations store current state without incrementing the command revision or creating snapshots. First activity, watched transitions and newly counted plays remain semantic changes. Unchanged paused reports and rejected older evidence create no snapshots.
- `home_routes.go`: followed the base's move to one typed Home/Home-row registration, removed duplicate direct handlers, and adapted its row handler to this lane's resource/viewer-stable `d.gridTag`. Both registry reads declare `ScopeRead`; owner failure diagnostics retain `ScopeAdmin`.
- Added the new base scan-issues route's missing Browsing classification, matching its typed metadata and existing Library scope.
- Verification passed: contract and schema regeneration (including schema-sync persistence checks); Linux, Windows amd64 and macOS arm64 `GOFLAGS=-p=2 go build ./...` (cross-builds use `CGO_ENABLED=0`); catalog/httpapi vet; focused personal-state, progress-counter, audiobook, Home, scope/lane, OpenAPI, validator and typed scan-diagnostics tests; six freshness/scan-issue client tests; web `npx tsc --noEmit`; whitespace check. Recursive grep found no remaining conflict markers.
- The three files are resolved on disk. Git still lists them as unmerged until Justin stages them. No git add, commit or snapshot was performed.
