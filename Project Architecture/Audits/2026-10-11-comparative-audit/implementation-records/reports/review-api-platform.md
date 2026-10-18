# Independent review: API platform

**Current fix status: all six numbered findings are fixed and verified. See the appended Fixes section; the original review below records the pre-fix state.**

## 1. Outcome

Completed a findings-only review of checkout `a5028946ded2f2ed7793b7fed72552fbf298fe25`, including the API platform merge `a76a65b` and its credential-lifetime follow-up. **Six findings: 0 blocker, 2 high, 3 medium, 1 low.** Five have failing scratch reproductions; the last-used finding is a source trace. No source files were edited, no snapshots or commits were taken, and the working tree remains clean.

Scope enforcement, bearer digest storage, issuance/replay, revocation, live member roles, content restrictions and targeted scan isolation held in the targeted checks. Fix the pre-admission credential lookup, title cursor query and webhook scheduler before calling the platform complete. MCP's guide defaults fail, and its required attention-list tool is still absent despite the attention API now existing.

Review ran on `ehler-homelab-mini-1`. Context refresh was attempted but its lock directory was read-only; the existing project and verification guidance was readable and used. Programme and both lane briefs, the implementation report, relevant audit bug entries, verification directions and fix log were read.

## 2. Decisions I made

- This is independent review, not implementation. Source and schema remain untouched; Go test overlays map scratch files into packages without writing those packages.
- Accepted the owner's explicit choices: plain HTTP, private webhook addresses, digest storage for bearer secrets, secrets unencrypted at rest, credentials surviving password changes and sign-out, and restored credentials coming back with an older backup. These are not findings.
- Reported concrete behavior and bounded reproductions. No live saturation test, device test, deployment or external MCP client was run. A passing fixture or static trace is not a claim of production acceptance.
- Kept merge/coverage failures separate from behavior findings when the declared registry metadata still handles the runtime route.

## 3. Bugs

### Findings in the reviewed implementation

### F1. Paused webhook can starve healthy destinations — high
- Location: `server/internal/webhooks/dispatch.go:129` (global `LIMIT 64` before per-webhook grouping and pause filtering).
- A failing webhook with at least 65 earlier due deliveries occupies every row of successive batches. The dispatcher skips its paused batch without considering the healthy destination whose row lies beyond the SQL limit. Until enough old rows move forward, other destinations receive nothing; continuous backlog can keep this happening.
- Reproduction: scratch Go overlay test `TestReviewPausedWebhookDoesNotStarveHealthy`, two destinations, 65 old pending events on the failing destination and one on the healthy destination, two dispatcher steps. Output: `bad requests=1 healthy requests=0`; assertion failed.
- Smallest sound fix: select a bounded set of eligible destinations first, excluding paused ones, then fetch a bounded due batch per destination. Do not apply one delivery-level limit before fair destination selection.

### F2. Webhook retries reorder lifecycle events — medium
- Location: `server/internal/webhooks/dispatch.go:129` and `:213` / retry scheduling in `attempt`.
- Selection is by next attempt time rather than each destination's oldest unfinished event. The first failure retries after 30 seconds but pauses the destination only 15 seconds. At 16 seconds a newer `playback.stopped` is delivered while `playback.started` is still waiting, reversing the lifecycle for a receiver. The implementation explicitly promises deliveries in order.
- Reproduction: `TestReviewWebhookRetryPreservesEventOrder` makes started fail once, advances the store clock 16 seconds, then permits successful responses. Output: `requests=2 last event=playback.stopped`; assertion failed because the second event arrived before the first retry.
- Smallest sound fix: pick the oldest pending delivery per destination, and do not send newer deliveries until that head succeeds or is finally abandoned. Combine with F1's fair destination scheduling.

### F3. Invalid API credentials bypass database admission — high
- Location: `server/internal/httpapi/api_gate.go:349` (credential resolution); `server/internal/httpapi/router.go:1312`–1320 (normal admission runs only afterwards).
- Every request with a short dotted bearer, even a made-up one, calls `ResolveCredential` before any admission lane, deadline, concurrency cap or per-credential rate limit. Failed lookups are not cached, and the cheap authentication limiter only covers certain POST routes. An anonymous caller can repeatedly use `GET /v1/me` with a bogus key to consume database read-pool work outside the protections that ordinary session requests receive.
- Reproduction: `TestReviewCredentialLookupIsInsideAdmission`: 128 requests with the same bogus key, all 401, performed 130 measured database statements (128 secret lookups plus initial settings reads), with no throttling. This proves the bypass; actual saturation/latency under concurrent hostile load was not measured.
- Smallest sound fix: bound credential resolution before touching SQL using a small authentication/admission lane, a request deadline and the canonical client-address cheap limiter. Retain the per-credential limits after successful resolution. A bounded negative cache can reduce repeats but cannot alone protect against randomized secrets.

### F4. Title cursors scan the preceding library prefix on every page — medium
- Location: `server/internal/catalog/title_query.go:242` (disjunctive cursor predicate) and `:249` (ordered query).
- The cursor condition `sort_key > ? OR (sort_key = ? AND entity_id > ?)` is not planned as an index range. SQLite seeks only to `(library_id,kind)` and filters every earlier index entry before returning the page. Late pages cost linear work in their cursor position; walking a whole library costs quadratic work in library size rather than bounded work per page. This contradicts the bounded indexed query claim.
- Reproduction against the server's SQLite driver: `TestReviewTitleCursorPlanSeeksToSortKey` prints `SEARCH e USING INDEX catalog_browse_title (library_id=? AND kind=?)`, without a sort-key range, and fails its seek assertion. A standalone Python SQLite reproduction of the same predicate/index on 100,000 rows returned 100 rows at both positions but took about 700 VM steps at cursor 0 and 540,700 at cursor 90,000. Python is supporting scale evidence; the Go test establishes the actual server planner behavior.
- Smallest sound fix: add a planner-usable range on the leading sort expression (`>=` ascending, `<=` descending) alongside the tie-breaking condition, or use a tuple seek where SQLite supports the expression/collation. Verify title, year and added sorts in both directions with late-cursor plan and work checks. Keep SQL restrictions before the page limit.

### F5. MCP guide's advertised default window is never supplied — medium
- Location: `server/internal/httpapi/mcp.go:529`–537.
- `tv_guide` promises that omitted times show the next three hours; neither time is required in its input schema. Its adapter supplies a default limit but forwards empty start/end parameters. The API requires both timestamps, so calling the tool without them always fails.
- Reproduction: `TestReviewMCPGuideDefaultsWork`, using the complete HTTP router and a real Live TV store: `{}` produced `isError=true` with `The channel source or guide is invalid. Check the input and try again.`; explicitly supplying start and end succeeded against the same server.
- Smallest sound fix: set absent start to current UTC truncated to seconds and absent end to start plus three hours, then preserve the guide's existing window validation. Add the missing default-window integration test.

### F6. Last-used activity never reaches storage after an idle burst — low
- Location: `server/internal/httpapi/api_gate.go:222`–238.
- Uses are buffered until another credential request happens at least one minute after the previous flush. There is no timer or shutdown flush. A script that uses a key once shortly after startup and then becomes idle can leave `lastUsedAt` blank indefinitely, including when the owner subsequently lists the keys with a session; a restart discards that use. This is more than the described approximate one-minute timestamp precision.
- Evidence: static trace: `used` is the only caller of `RecordCredentialUse`; its early-return branch keeps the use only in `g.uses`; `Credentials` reads only `api_credential_use`; owner-session list requests never call `used`. No wall-clock wait or live restart was run.
- Smallest sound fix: periodically flush the bounded usage map off the request path and flush it during graceful shutdown. If a write fails, retain/merge the batch for the next attempt.

### Referenced bug status

These are statuses of the relevant API-platform bug IDs, not a new full identity/security audit. No fixes were made by this review.

| Bug | Review status |
|---|---|
| B-01.3 | Fixed in the reviewed code: keys now authenticate; targeted issuance/scope/revoke tests pass. F3 and F6 remain. |
| B-08.1 / B-15.8 | Networking fix present; canonical request-origin handling is called. Proxy deployment not reproduced here. |
| B-08.2 | Not fixed, acknowledged in the implementation report: legacy `failure` still defaults unknown errors to 400. |
| B-08.3 | Fixed server behavior: a newcomer replaces the old stream; event tests pass. Client still retains the permanent-409 fallback. |
| B-08.4 | Fixed: opening cursor frame is present and tested. |
| B-08.5 | Fixed: advertised and client maximum are 25 seconds. |
| B-08.6 | Not fixed, acknowledged: general wake fan-out remains; fresh concurrency/load behavior not measured. |
| B-08.7 / B-15.3 | Networking fix present: address handling is shared; key budgets use credential identity. F3 is a new pre-admission exception. |
| B-08.8 | Not fixed, acknowledged: grid validators still include broad write/time/token inputs. |
| B-08.9 | Intentional safeguard retained; new credential/query responses passed fixtures. No new null-response defect found in sampled branches. |
| B-08.10 | Already fixed on the base in shared error/retry parsing; client retry tests pass. |
| B-08.11 | Partly fixed: disabled key authentication docs/code removed. Existing cursor/idempotency/precondition documentation drift is outside this lane. |
| B-08.12 | Partly fixed: gzip quality/weak-validator changes exist. Response construction still precedes 304. The old asset-cache caveat in the lane report is stale: `installation.go:94` caches every recognized content-hashed asset immutably, matching the B-16.3 fix log. |
| B-08.13 | Fixed access-label guard: registry requires matching item/library placeholder names and authorizer checks them; scope coverage test passes. |
| B-08.14 | Revision-independent third-party cursors are implemented; first-party continuation fences remain. F4 defeats the bounded-work claim for late pages. |
| B-08.15 | Already fixed on the base; not a new defect in this lane. |
| B-08.16 / B-15.15 | Fixed public fingerprint removal in current handler; duplicate OpenAPI YAML still needs correction, as the lane acknowledges. |
| B-08.17 | Not fixed, acknowledged: one realtime pool remains; credential stream cap is separate. |
| B-08.18 | Networking upload-timeout changes present; real slow uploads not reproduced. |
| B-15.7 | Existing documentation problem about cryptographic device binding; bearer/device/family binding is not proof of a device private key. No such new guarantee introduced by the credential implementation. |
| B-15.23 | Cheap limiter now covers TV start/review/decision and invitation paths; TV polling deliberately excluded. |
| B-01.18 | Existing session-family retention gap remains; credential pruning deletes credential rows, not their associated identity families/devices. No full identity-retention review was undertaken. |
| B-01.19 | Explicit new credential revoke calls the family cleanup hook. Broader session-revocation paths remain for accounts-security review. |

## 4. API changes

None made. Reviewed the new registry contracts for credentials, `/v1/titles`, `/v1/libraries/paths-changed`, webhook management, `/v1/mcp`, plus the legacy route scope table and client-core services/parsers. Session-only credential management and scope/role separation were exercised. See F3–F5 for behavior requiring changes.

Current coverage checks fail after merges:

- `TestEveryRegisteredRouteHasALane`: seven metadata routes are absent from the literal `routeLanes` table (attention GET/POST, metadata-settings GET/PUT, show refresh, item/show screen GET). They declare registry lane metadata; this is a coverage/wiring reconciliation needed from the integrating lanes.
- `TestOpenAPICoversRegisteredRoutes`: `route_scopes.go` retains four removed-path entries: `DELETE /v1/items/{id}`, `GET /v1/admin/trash`, `POST /v1/admin/trash/empty`, `POST /v1/admin/trash/{id}/restore`. The source-scanning test treats them as routes and fails. Remove the stale scope entries; do not restore deleted routes to silence it.

The implementation report's claim that both coverage tests pass does not hold on this reviewed merged checkout.

## 5. Schema changes

None made. Reviewed credential tables/lifetime triggers, source notices/run targets, webhook queue/indexes/fan-out and lifecycle triggers in the baseline. Persisted queue, digest storage, SQL visibility checks and private-recording title exclusion exist. The trigger fixture disables foreign keys/check constraints, so its pass proves event production for its synthetic rows, not acceptance of real capture/sign-in/playback data. Existing signing/delivery tests use real HTTP listeners.

## 6. Owner or viewer settings added or wired

None made. Reviewed `apiRequestsPerMinute` (integer, 0 means 600 by default) and `blockMemberAccessTokens` (boolean, false). Rate reads use current connection settings; token-creation setting exempts administrators and does not revoke existing credentials. Console screens are outside this lane and remain needed.

## 7. Needed from others

- API platform: F1–F6. Fix fair webhook scheduling and order together; preserve signature, timeout, redirect, queue and authorization bounds.
- API platform / providers integration: expose the brief's unmatched/low-confidence title MCP tool over the **existing** `GET /v1/libraries/{id}/metadata/attention` (`metadata_attention.go:31`), which is on the registry and library-scoped. The lane report's explanation that no such route exists is stale after the providers merge. Per-title `get_match` and unplaced TV files do not replace this list.
- Registry/providers/library integration: reconcile the seven lane-classification entries and delete the four stale trash/item-deletion scope entries described above. Preserve the new title/file deletion model.
- Client lanes: token, webhook, MCP configuration and settings screens; API-key expiry/scopes/state/last-used presentation remain unfinished as already declared in the implementation report. Server and client-core transport helpers exist.
- Documents: remove the retired public fingerprint/stream-conflict shapes from older YAML, update superseded disabled-key architecture guidance, and correct the completion/test claims in the lane report.
- MCP adapter integration tests should call mutations through their actual API contracts, not only check that tool names are listed. Current MCP tests exercise listing/filtering, library/title reads, denied calls, settings and webhook listing, but not all 40 tools.

## 8. Test notes for other environments

All Go commands used `GOFLAGS=-p=2`; no full builds/suites, browser sessions, deployment or hardware work was done.

### Passed in this checkout

- HTTP API: `go test ./internal/httpapi -run 'TestAPIKey|TestCredential|TestEveryRouteDeclares|TestScopesDoNot|TestTitleQuery|TestPathsChanged|TestMCP|TestWebhooksAre' -count=1`.
- Webhooks: validation and lifecycle triggers with `-run 'TestWebhook|TestEvery|TestTitle|TestPlayback|TestRecording|TestScan|TestMember|TestStorage'`; separate signing/retry/log and catalogue production tests with `-run 'TestDeliveryIsSignedRetriedAndLogged|TestCatalogueChangesBecomeEventsOnlyForSubscribers'`.
- Ingestion: `-run 'TestReportedPaths|TestIncrementalScanSkips'`. The reported-path fixture includes local path isolation, queuing behind an active scan and deleted-folder behavior; this is not Windows/network-share acceptance.
- Password/sign-out independence, disabling/deletion, live-role changes and event tests passed within the later HTTP API selection; that selection overall failed the two coverage checks listed in section 4.
- Client-core: `node --experimental-strip-types --test test/api-access.test.ts test/server-administration.test.ts test/events-retry.test.ts`: **25 passed, 0 failed**.
- Apikit registry scope/access/metadata tests: `GOFLAGS=-p=2 go test . -run 'Test.*Scope|Test.*Metadata|Test.*Access' -count=1`: passed.
- `GOFLAGS=-p=2 go vet ./internal/httpapi ./internal/webhooks ./internal/access ./internal/catalog ./internal/ingestion`: passed.
- Two scratch MCP checks passed: fractional title limit was rejected, and `read_logs(category=playback,limit=10)` worked.

### Reproductions

Scratch directory: `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/review-api-platform/`.

From the reviewed checkout's `server/` directory:

```sh
GOFLAGS=-p=2 go test -overlay /home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/review-api-platform/overlay.json ./internal/webhooks -run TestReview -v -count=1
GOFLAGS=-p=2 go test -overlay /home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/review-api-platform/overlay.json ./internal/httpapi -run TestReview -v -count=1
python3 /home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/review-api-platform/title_query_plan.py
```

The failing scratch assertions intentionally express the desired behavior. F3's test proves measured unthrottled SQL access, not throughput saturation. F4's test checks the actual Go driver's plan with the production join/seek/order form; the Python run is a separate minimal index/predicate scale experiment. Reproductions are scratch only, not tests added to the repository.

### Follow-up acceptance

- On a private Linux server, add a failing receiver with a 65+ event backlog and a healthy receiver. Healthy events must arrive independently; after a failed start event, its later stop must wait for success/final abandonment. Verify signatures and unchanged event IDs on retries.
- Under a small controlled bogus-key burst, measure SQL acquisitions and owner-console latency. Resolution must acquire a bounded slot before SQL and excessive attempts must be refused cheaply. Repeat behind the configured trusted proxy using the canonical client-address handling.
- On a large library, query late pages for title/year/added in both directions. Plans must show the sort-key range and page work must not grow with cursor position. Verify restrictions and equal-key tie ordering still hold.
- On a server with Live TV enabled, `tv_guide({})`, start-only and end-only windows must satisfy the published defaults; explicit invalid windows must still fail. Try a real MCP client separately.
- Use a key once, leave all integrations idle for over a minute, then list credentials with an owner session and restart gracefully. The recorded last-used time must survive; F6 is otherwise unchanged.
- Validate targeted scans on Windows, macOS and network sources using paths as seen by that server, including unreachable roots, deleted folders and notices arriving during scans. Real tuner/GPU and native-client acceptance remain outside this review.

## 9. Not finished

Review is complete; **all six findings remain unfixed** because this lane is findings-only. The brief's full MCP attention tool, owner activity coverage and general administration of another member's pause/resume/seek have not been established as delivered. The implementation report acknowledges limited cross-member playback control and missing attention coverage; the attention-route explanation now needs correction. No new general playback-control design was attempted.

No live saturation, load capacity, actual tuner capture, native clients, external MCP client, Windows/macOS runtime, backup/restore integration or real reverse-proxy tests were run. The two merged-checkout coverage checks currently fail. Do not infer full release acceptance from the targeted passes.

### Verdict by API-platform scope item

| Brief item | Verdict | Evidence / limits |
|---|---|---|
| 1. Registry scopes and enforcement | **Sound** | Mandatory scope/access metadata, default-deny unclassified routes, edge stamp, nested API checks, scope and live-role tests; stale removed-path table entries need cleanup. |
| 2. Owner API keys | **Sound with listed fixes** | Issue once, digest storage, bounded create/replay, scope/role, expiry and explicit revoke present; F6 last-used persistence. |
| 3. Personal access tokens | **Sound with listed fixes** | Own-account management, role/content/library checks, device approval and disable/delete behavior tested; F6 applies equally. |
| 4. Third-party sign-in | **Sound** | Existing ordinary direct and device-code sessions documented without app registration. No Hosted OAuth work is claimed; external-app/device acceptance not run. |
| 5. Bad-client protection | **Not sound** | F3 allows anonymous SQL work before admission/deadline/rate controls. Valid-credential buckets and concurrency caps work in targeted tests; media bodies are explicitly exempt from credential concurrency. |
| 6. Bounded title query | **Sound with listed fixes** | SQL restrictions, provider lookup, validation and revision-independent cursor behavior pass; F4 late-page work is unbounded in cursor position. |
| 7. Paths changed | **Sound** | Owner/library scope, local absolute-path validation, notice queue and targeted missing-file scope checked; local scan fixture passes. Hardware/network-source cases remain unverified. |
| 8. Signed outbound webhooks | **Sound with listed fixes** | HMAC, persisted retry queue, log/test API, private-address policy and bounded transport exist; F1 starvation and F2 ordering failures. Trigger-only passes are not real event-producer acceptance. |
| 9. Built-in MCP | **Not sound** | Authenticated JSON-RPC and scope/role filtering/nested enforcement pass; F5 advertised defaults fail, full attention tool absent despite available API, and not every adapter is integration-tested. |
| 10. Outsider documentation | **Sound with listed fixes** | New authentication/scope/query/webhook/MCP guides and client-core helpers present. Correct F4/F5/F6 claims and stale merged report/YAML/route inventory as above; real-client protocol interoperability remains unverified. |

## Fixes

Fix work started on branch `audit-implementation/review-api-platform`. The user authorized source edits and snapshots, superseding the earlier read-only restriction. No commits, pushes or deployments. Original review above is retained as dated evidence; the statuses in this section supersede it.

- F1–F6: fixed. No finding was withdrawn; no numbered finding remains open.

### F1 and F2 — fixed

The dispatcher selects destinations before queue rows, gives every eligible destination a bounded share of the 64-delivery pass, and reads each queue in creation/insertion order. A head waiting for its retry or initial delay blocks its own later deliveries. The next wake is based on queue heads, not later due events. No schema/API shape change.

Real regressions: `TestPausedWebhookDoesNotStarveHealthy` and `TestWebhookRetryPreservesEventOrder` in `internal/webhooks/dispatch_order_test.go`. Both failed before the change and passed afterwards. Retry test additionally confirms start-before-stop on recovery, stable event ID and valid signature. Targeted validation, trigger, signing/retry/log and catalogue-event tests passed; `go vet ./internal/webhooks` passed.

### F3 — fixed

Every SQL credential resolution now acquires one of four slots and has a two-second context deadline. Unknown failures have one address/network budget across routes, using the existing canonical trusted-proxy parser and bounded limiter maps. Known credentials still re-prove live state after cache expiry without spending the failure budget; a failed live resolution removes their remembered entry. In-flight failures can exceed the nominal address budget by at most the other three already admitted resolutions. No new account lockout or transport restriction.

`TestCredentialLookupThrottlesInvalidSecretsBeforeSQL` failed before the fix at attempt 20 (401 instead of a cheap 429), then passed: randomized pk/pat/dotted secrets across paths cannot rotate the budget, throttled requests execute zero SQL, untrusted forwarded headers do not evade it, known expired-cache credentials still work, and a separate client has its own budget. `TestCredentialResolutionBoundsConcurrencyAndDeadline` confirms the fifth resolver never reaches the lookup callback, a deadline is supplied, slots return and canceled calls stop. Existing credential, key, scope and MCP tests passed; `go vet ./internal/httpapi` passed. Separate source passes checked cached, revoked, nested-MCP, alternate-prefix and forwarded-address paths. Live saturation remains unmeasured.

### F4 — fixed

Added a planner-usable inclusive range on the selected sort expression alongside the existing exclusive cursor/tie-break predicate in `internal/catalog/title_query.go`. This retains filtering, collation, ordering and opaque cursor behavior without changing schema or touching scanning/derived/search/personal-state code.

`TestTitleCursorSeeksSortIndex` in `internal/httpapi/title_query_seek_test.go` captures the full SQL actually executed by the service through a test driver connector, then explains that statement against the real database. Diagnostic SQL traces truncate their text and cannot prove this property, so the regression does not rely on them. Before the fix, all three sorts in both directions had only a library/kind index prefix. Afterwards all six show a sort-key range; complete walks preserve ties without duplicates or omissions. Existing restricted title-query HTTP tests and `TestBrowseCursorPagesTheWholeLibrary` passed. `go vet ./internal/catalog ./internal/httpapi` passed. No new index or schema synchronization was needed.

### F5 — fixed

The MCP guide adapter now supplies current UTC truncated to whole seconds when start is omitted, and start plus three hours when end is omitted. Explicit values continue through the guide's existing validation. Tool text tells paginating callers to reuse the returned start/end window with the cursor.

`TestMCPGuideDefaultWindow` in `internal/httpapi/mcp_guide_defaults_test.go` failed before the fix for `{}`; it now checks empty, start-only, end-only and explicit windows through the complete HTTP router and real Live TV store. Malformed, reversed and over-24-hour windows still fail; explicit times are unchanged. Existing MCP authentication, filtering, nested scope and settings tests passed. `go vet ./internal/httpapi` passed. Ran `GOFLAGS=-p=2 go run ./cmd/apigen` successfully after the MCP change; generated contracts were unchanged because no route or wire shape changed.

### F6 — fixed

The credential gate now starts one timer after its first pending use and flushes after one minute without further traffic; it has no running timer while idle. Flushes serialize, and a failed batch merges back into uses that arrived meanwhile, preserving the latest timestamp and combined count. The timer callback retains the server's panic containment. `httpapi.New` returns an HTTP wrapper with a narrow shutdown-flush hook; the server calls it after draining HTTP and before closing the database, with its own ten-second budget. Test fixtures stop/flush their timers before database cleanup.

`TestCredentialUseFlushesWithoutAnotherRequest` failed before the fix with no `lastUsedAt` after an idle minute. It now passes with a virtual clock in an isolated test subprocess, because the process-global SQL write gate's channels cannot cross virtual-clock test boundaries. `TestCredentialUseFlushRetainsFailedBatch` proves a canceled write loses neither timestamps nor requests and retry does not duplicate the batch. `TestCredentialUseFlushOnShutdown` checks persistence immediately, repeated flush idempotence and timer shutdown. `go vet ./internal/httpapi ./cmd/server` and `TestMainWiresEveryHTTPDependency` passed. This is approximate last-used metadata, not billing/audit accounting; a forced process kill can still discard the pending minute. A graceful flush that encounters a persistent database error is logged rather than claimed successful.

### Final validation and scope

Passed after the fixes:

```sh
# From server/; every command uses the shared-machine parallelism limit.
GOFLAGS=-p=2 go test ./internal/httpapi -run 'TestCredential|TestAPIKey|TestScopesDoNot|TestEveryRouteDeclares|TestTitleQuery|TestTitleCursor|TestMCP|TestPasswordChange|TestDisablingOrDeleting|TestACredentialHolds' -count=1
GOFLAGS=-p=2 go test ./internal/webhooks -run 'TestPausedWebhook|TestWebhookRetry|TestWebhookSameMillisecond|TestDeliveryIsSignedRetriedAndLogged' -count=1
GOFLAGS=-p=2 go test ./internal/catalog -run 'TestBrowseCursor|TestBrowseOperator' -count=1
GOFLAGS=-p=2 go test ./cmd/server -run TestMainWiresEveryHTTPDependency -count=1
GOFLAGS=-p=2 go vet ./internal/httpapi ./internal/catalog ./internal/webhooks ./cmd/server
GOFLAGS=-p=2 go run ./cmd/apigen
```

The catalogue command selects the real `TestBrowseCursorPagesTheWholeLibrary` test (there is no additional operator test matching that expression). Earlier webhook validation, trigger and catalogue-event selections also passed. The server command compiles the application and its dependencies, and the targeted wiring test passes. No whole suites/full builds were run. `git diff --check` passed. No API routes, scopes, wire shapes, owner/viewer settings or schema changed; client-core edits and schema-sync were therefore unnecessary.

Source changes are confined to `internal/webhooks/dispatch.go`, `internal/httpapi/api_gate.go`, `internal/catalog/title_query.go`, `internal/httpapi/mcp.go`, router return/test-fixture cleanup wiring and five lines of server shutdown wiring. Five real regression-test files were added. No commits, pushes, deployments or work in other lanes' trees.

Snapshots are cumulative against `a5028946ded2f2ed7793b7fed72552fbf298fe25` under `_program/snapshots/review-api-platform/`: `001` covers webhook scheduling and credential admission; `002` covers the remaining paging/default-window/usage fixes; `003` is the final complete snapshot. No tree changes follow the final snapshot.

**Remaining scope outside the six findings:** the original report's MCP attention-tool/other-adapter gaps, client screens, older API-document drift and two merge-coverage failures remain for their owning lanes. They were not silently resolved or claimed complete here. Real load saturation, reverse proxies, real MCP clients, graceful restart under active playback and Windows/macOS runtime acceptance remain untested. Follow-up acceptance steps above still apply; the five scratch reproductions are historical evidence, while the repository tests are now canonical regressions.
