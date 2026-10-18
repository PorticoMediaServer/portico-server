## Outcome

Completed the independent review and fixes on base `aac183d`, in the named worktree only. **Six findings: 0 blockers, 3 high, 3 medium, 0 low; all six fixed.** Download access now follows current library and content permissions, including cached replies and titles moved between libraries. Revoked receipts cannot be replayed into authorization. Offline reports keep the newest resume point and play dates while preserving older history and counts. Housekeeping cannot expire a claim refreshed between its candidate read and write. Large queued lists refresh with at most three bounded list requests per poll.

Host: `ehler-homelab-mini-1` (personal Ubuntu host). Context refresh failed because its lock directory was read-only; existing canonical guidance was read. No commits, push, deployment, helpers or subagents. Snapshot 001 records the first five fixes; later cumulative snapshots contain the complete change.

## Decisions I made

- Current permissions take precedence over exact idempotent response replay. Cached signed receipts are returned only while their live receipt and permission rows still allow them; revoked replies carry no signature.
- Library membership is checked inside the caller's transaction through the same router policy used by playback. The title's current library is authoritative. A deleted title still renews copies if its old library allows access, or the library itself was removed.
- Database/context errors in receipt permission guards cause a retry, never permanent revocation.
- Fresh offline observations reserve an observation-specific intent, while keeping one play/history identity. Stale evidence records history without rewinding personal or owner play dates.
- Older loaded active claims refresh in sparse groups of 200. The client makes two active-state list requests and at most one sparse request per tick. Very large loaded lists rotate over several ticks; the current list stays visible.
- Existing native/player gaps remain with those lanes. This worktree contains the server and canonical shared packages, not the native application.

## Bugs

Statuses refer to current server/shared code verified here; native outcomes are explicitly distinguished from this review's evidence.

- B-14.1: already fixed on server; receipts survive claim retention/removal/replacement. Existing durability tests pass.
- B-14.2: already fixed server Range/validators and renewable addresses; native iOS address-change restart remains documented by the original lane, outside this checkout.
- B-14.3: already fixed server 24-hour sliding/renewable grants; existing grant/HTTP tests pass. OS suspension acceptance remains device work.
- B-14.4: already fixed; options ask the encoder's producible recipes. Recipe/options tests pass.
- B-14.5: already fixed; originals hash in bounded windows and serve in place through the production confined storage opener; digest sharing/resume/no-copy tests pass.
- B-14.6: server already issues receipts before transfer; native receipts-first integration was not re-reviewed or run here.
- B-14.7: already fixed server title snapshots/bundles and shared parsers; native offline browsing acceptance remains.
- B-14.8: already fixed shared followDevice/request rediscovery; shared service tests pass. Native enrollment gaps in N-V08.1 remain outside this checkout.
- B-14.9: native daily renewal/manifest batching not re-verified here; shared service disposal/backoff was read. No native source writes permitted.
- B-14.10: original separate writer already replaced by the normal writer; additional ordering/date defects fixed as F3/F4. Offline history, thresholds, paused-history, book and stale-state tests pass.
- B-14.11: already fixed; book pointer tests pass.
- B-14.12: not fixed here; device capability/engine selection requires the native/player lanes, acknowledged by the original report.
- B-14.13: already fixed state-only revision updates and shared silent conflict retry; related tests pass.
- B-14.14: native storage/sign-out behavior not changed or independently accepted here; original report describes a reclaim UI and device-wide settings. Sign-out preservation is an existing product decision.
- B-14.15: already fixed claim unavailability at tidy/grant resolution; device receipts remain independent. Existing tests pass; tidy race fixed as F5.
- B-14.16: server unknown-receipt/preparation semantics already correct; native recovery not independently exercised here.
- B-14.17: already fixed bounded capture restarts; container-request tests pass.
- B-14.18: not device-verified here; original lane reports durable incoming-file recovery. Requires iOS interruption acceptance.
- B-14.19: not device-verified here; original lane reports OS-task reconciliation. Native source is outside this lane.
- B-14.20: not device-verified here; original lane reports retries/Wi-Fi recovery and restart on next app run. Restart while the process is dead remains absent.
- B-14.21: not fixed; wall-clock receipt checking remains the original lane's deliberate non-DRM trade-off. No new clock hardening.
- B-14.22: server/shared renewal and progress answers already carry canonical viewing state; native consumption was not independently verified.
- B-14.23: already fixed retrying failed claims and returning existing claims; related tests pass.
- B-14.24: already fixed bounded pruning of ended claims, requests, receipts and obsolete bookkeeping; existing tests pass.
- B-14.25: already fixed periodic/read-first housekeeping; race during its write admission fixed as F5.
- B-14.26: already fixed best-present-version selection and distinct several_files refusal; no explicit version selector was added.
- B-14.27: server reason validation/retry collision already fixed; HEAD has no former first-use window. Device completion/removal/redirect behavior is outside this checkout and was not runtime accepted.
- B-12.3: already fixed explicit mark ordering; original stale-observation tests pass, fresh same-play arbitration strengthened as F4.
- B-12.5: already fixed normal history/count writer integration; latest-date regressions fixed as F3.
- N-V08.1: shared request rediscovery already present; native page-failure retry and total backlog bounds cannot be validated/fixed in this worktree.
- Document 22: offline journal/launch entries A-22.41 and A-22.47 add no download bug IDs; general connectivity defects remain their owning lanes' work.
- F1–F6: fixed; evidence and tests below.

## API changes

`GET /v1/downloads/preparations` keeps its existing response shape and adds optional `ids`, a comma-separated set of at most 200 preparation IDs. Sparse reads return all accessible requested rows (200-row bound regardless of limit), omit absent/denied rows, reject malformed/oversized IDs and reject combining IDs with a cursor. The existing read scope is retained. Registry metadata, generated `packages/contracts/src/routes.ts`, shared polling consumer and `server/api/downloads.md` are updated; `go run ./cmd/apigen` was run. No route removals and no response schema changes.

Preparation reads/actions, grant issue/resolution, receipt issue/renewal and offline progress now enforce current library/content access. Cached replies cannot bypass those checks. Receipt guard infrastructure failures return errors instead of revoking copies.

## Schema changes

None. Existing baseline tables and indexes are used; schema-sync is not required.

## Owner or viewer settings added or wired

None.

## Needed from others

- Copy the canonical shared package changes into the native repository through its normal shared-packages script, after these server/API changes are integrated. Native code was not writable from this lane.
- Coordinate the small fixes in `catalog/personal_activity.go`, `catalog/play_history.go` and `playback/offline_play.go` with the personal-state/playback lanes; these are the exact offline writer paths concerned by F3/F4.
- Existing original-lane gaps: local player wiring for subtitles/chapters/markers and engine selection, iOS partial reuse after an address change, offline next-episode queue, chapter/trickplay images and device acceptance.
- Original contract/document corrections listed in the downloads lane report remain: sign-out semantics, grant/receipt lifetimes and removal of the original-file snapshot design.

## Test notes for other environments

Linux build and vet, touched-package Windows/macOS cross-compilation, targeted server tests, all four shared download test files and web TypeScript checking passed. Detailed commands/results are below. No full test suite, device, simulator, browser, deployment, Xcode or Gradle run was performed.

For native/browser integration on the merged server:
1. A member claims a film, then the owner removes that library from their access (also test moving the title to a denied library). Retry old preparation/receipt operation IDs. Pass: list/bundle access is concealed, no transfer is served, receipt reply refuses without a signature. A deleted title with otherwise retained access must still renew its existing copy.
2. Sync play A, then newer play B, then a still-newer observation of A. Pass: resume uses the newest A observation, both plays remain in history, and replaying older A cannot move resume or dates backwards. Repeat with online playback and manual watched marks between offline observations.
3. Queue and page through 350 downloads. Finish an older row. Pass: each poll makes at most three preparation list requests and the older row settles within two polls; no request per row. Repeat while withdrawing library access.
4. Renew a grant as retention cleanup runs. Pass: the freshly used claim remains ready and the grant works. Existing deterministic test covers the transaction gap; real scheduling remains platform acceptance.
5. Repeat original downloads lane's large ranged transfers/source replacement/receipt durability checks on Windows/macOS and native devices; source confinement, OS file custody and downloader behavior need runtime acceptance, not just cross-compilation.

## Not finished

No remaining unfixed F1–F6 finding in this checkout. Native package propagation and the existing device/player gaps listed above are outside this review's writable scope and are not claimed complete. Physical device/real-network/hardware acceptance and full suites remain unperformed by programme instruction. Very large loaded queued lists refresh older rows across multiple bounded polls; hundreds of simultaneous network requests are eliminated.

## Phase 1 findings

File/line references below refer to base `aac183d`; findings were recorded before their source fixes.

### F1 — Current library permission omitted from download access (high)

Files: server/internal/downloads/bundle.go:165, receipts.go:313, progress.go:148, service.go:405 and :442; grants.go:300; httpapi/downloads_routes.go:280.

Contentaccess.VisibleKnownItemTx applies content restrictions, not library membership. Bundle, receipt issue, grant issue and progress do not call the library gate; preparation reads do not even apply content restrictions. A member who previously claimed an item can keep reading its metadata or obtain signed offline permission after losing its library. ResolveGrant returns the preparation's stored library, so after a title moves it checks permission on the old library while serving the current title's bytes. Smallest fix: install a transaction-local library callback, use current title library plus content restrictions throughout download reads/writes and transfer resolution, conceal denied claims, and preserve deleted-title receipt semantics. Add permission withdrawal and library-move regression tests.

### F2 — Idempotent receipt replies bypass current revocation (high)

Files: server/internal/downloads/receipts.go:286 and :410.

IssueReceipts returns a cached signed receipt before inspecting title restrictions or revocation. Revalidate checks only the downloads switch on replay, skipping individual receipt revocation, current content restrictions and library guard. Replaying the original operation after withdrawal returns authorization and tells a reconnecting client to keep its copy. Smallest fix: inspect cached receipts against live receipt and permission rows before returning signed envelopes; preserve idempotency without renewing the cached signature. Reproduce both issue and renewal retries after explicit revocation and after content/library withdrawal.

### F3 — Older offline history regresses the latest-play timestamp (medium)

File: server/internal/catalog/personal_activity.go:111 (called by downloads/progress.go:177); catalog/play_history.go:42 and :49.

The shared writer unconditionally sets LastPlayedAt to the offline observation even when canonical=false. An older offline play is correctly refused the resume point but makes the viewer's latest-play timestamp older, contradicting the report's 'history only' claim. Smallest fix: preserve the later timestamp while still recording the older play in history. Owner history's updated_ms and play_counts.last_played_ms also regress through the same writer. Regression: sync newer then older plays, verify personal_items, both histories and play-count dates.

### F4 — A later observation of an existing offline play cannot reclaim canonical state (high)

File: server/internal/playback/offline_play.go:73 and personal_progress.go:45.

Offline activation permanently reuses the first play ordinal and INSERT OR IGNORE keeps its claim at that ordinal. If another offline play is accepted between reports, the first play's genuinely newer observation passes newest(), returns applied, and advances download_progress_marks, but activatePersonalWriter silently refuses its older ordinal, leaving the previous resume point. Smallest fix: when freshness has been established, reserve authority per observation and advance that play's scoped claim ordinal before activation. Regression: A@t1, B@t2, A@t3 must resume A@t3 while history still has two plays; stale A must remain refused.

### F5 — Housekeeping expires a claim renewed between read and write (medium)

File: server/internal/downloads/worker.go:209.

Tidy selects expired/removed/replaced claims outside its write transaction, then rechecks only state. IssueGrant can refresh used_ms after the read and before the writer, leaving state ready; housekeeping still expires the freshly used claim and revokes its grants. Similar races apply to restoration of a missing title/file. Smallest fix: re-evaluate each ending predicate inside the writer before ending the claim. Evidence is the two-transaction trace; add a controlled between-phase test.


### F6 — Large queued lists trigger a request for every row beyond the first active page (medium)

File: packages/client-core/src/downloads-service.ts:111–121.

The comment assumes a cap of 200 live preparations, but downloads/types.go explicitly caps running work only, not queued/ready claims. Poll reads only the first 100 queued and 100 running rows, then treats each loaded active row outside those pages as a possible transition and fetches it individually. A viewer who has paged through a large queued show/music download issues hundreds of sequential requests on every tick. Smallest sound fix: refresh missing active rows in bounded sparse pages and rotate through older loaded claims. The initial reproduction made 250 individual requests for 350 queued claims; the final regression permits two active pages plus one sparse page and verifies a later claim settles.


## Fixes

- F1: fixed through transaction-local current library/content checks, concealed preparation reads and cached submit/action replies, consistent accepted counts after filtering cached batches, and current-library transfer resolution. Tests: `TestFilteredBatchReplayKeepsCountsConsistent`, `TestPreparationReadsConcealRestrictedTitle`, `TestGrantUsesTitlesCurrentLibrary`, `TestLibraryWithdrawalStopsDownloadAccess`.
- F2: fixed cached issue/renew replies by inspecting live receipt revocation and item/library permission; denied outcomes carry no signature. Tests: `TestReceiptReplayRespectsRevocation`, `TestReceiptReplayDistinguishesGuardDenialFromFailure`, plus library-withdrawal coverage. `ErrNotVisible` is a real denial; transient guard failures remain retryable and leave receipts unrevoked.
- F3: fixed personal LastPlayedAt, owner play_history.updated_ms and play_counts.last_played_ms to retain the latest observation. Tests: `TestOlderOfflinePlayKeepsLatestPlayDate`, `TestStaleReportKeepsOwnerPlayDate`. Older completed plays still count and appear in history.
- F4: fixed observation-specific personal intent reservation and exact viewer/item/play claim advancement. Test: `TestLaterOfflineObservationReclaimsResume`, including a subsequent stale A report and two-row history assertion.
- F5: fixed by repeating each ending predicate in the write snapshot. Test: `TestTidyKeepsClaimUsedAfterCandidateRead` refreshes a grant after candidate collection. It failed against the original worker with only the observation hook added, then passes with the fix.
- F6: fixed with one bounded sparse preparation read per tick, rotating through up to 200 older loaded claims. Tests: shared service's 350-row regression, `TestSparsePreparationPageIsBoundedAndViewerScoped`, and HTTP sparse-query coverage in `TestDownloadTransferSupportsRangeAndValidators`.

The original F1–F4 regression functions were run against unchanged base source after fixture corrections and failed on their claimed behavior. F5 failed with only a controlled observation hook on the old implementation. F6 reproduced 250 individual probes in one tick before its fix. The expanded F3 owner-date assertions also failed before their fixes. Before-fix output is preserved at `/tmp/review-downloads-before.txt`; tests themselves are repository files.

## Verification and snapshots

- Snapshot 001: `Keep download permissions and offline progress current` (first five fixes, before later owner-date and sparse-page changes).
- Snapshot 002: `Refresh queued downloads in bounded batches` (complete sparse-page API/client implementation plus owner history date fixes).
- Final snapshot 003: `Keep download access checks and batch replies consistent` (cumulative final tree, including the last cached-batch/receipt-guard regression checks). No tree/report edits after it.

Final verification, all passed:

- `GOFLAGS=-p=2 go build ./...` on Linux.
- `GOFLAGS=-p=2 go vet ./internal/downloads ./internal/catalog ./internal/playback ./internal/httpapi`; downloads vet repeated after the final two small permission-path changes.
- `GOFLAGS=-p=2 go test ./internal/downloads -run Test -count=1` (20.411 s), then targeted final receipt/sparse-page/batch-count regression tests (1.916 s). Eleven new regression functions cover the six findings.
- `GOFLAGS=-p=2 go test ./internal/httpapi -run 'TestDownload|TestEveryRouteDeclaresACredentialScope' -count=1` (1.904 s). Earlier scope-focused HTTP checks also passed.
- `GOFLAGS=-p=2 go test ./internal/catalog ./internal/playback -run 'Test.*(Personal|Offline|Progress|PlayHistory|History)' -count=1` (23.915 s / 6.468 s).
- `GOFLAGS=-p=2 GOOS=windows go build ./internal/downloads ./internal/catalog ./internal/playback ./internal/httpapi` and the same with `GOOS=darwin`; downloads recompiled on both after the final portable permission/count changes. Compilation does not establish platform runtime behavior.
- Each of the four client-core download files run individually with `node --test`: downloads-service 13, downloads-service-coverage 6, downloads 21, download-requests 9; **49 tests** total.
- `npx tsc --noEmit` in `web/`.
- `GOFLAGS=-p=2 go run ./cmd/apigen` in `server/`; generated route metadata carries `ids` and the existing read scope. No response contract changed.
- `git diff --check` passed. Final review found no secrets, compatibility paths, new dependencies, user-facing strings or unrelated schema edits in this patch.

Full suites were intentionally not run. Before-fix failure transcripts are local evidence in `/tmp/review-downloads-before.txt`; the committed-by-integration tests and snapshot artifacts provide durable reproduction. Native/browser/device acceptance remains as described in the test notes and original lane report.

## Merge with the base

Source conflicts resolved; no staging, commits or snapshots performed. Read the eight named lanes' reports and updated schema/registry guidance.

- `personal_activity.go`: kept the base's meaningful-evidence gate, separate personal counting, state/history fences, thresholds and dated watched-state writer. Added this lane's monotonic time check, so meaningful evidence updates LastPlayedAt only when newer than the stored date.
- `play_history.go`: kept the base's activity-sharing privacy gate and independent personal counts, with this lane's max timestamp updates for owner history and shared play counts. Private completed plays remain in owner history without entering shared counts.
- Download routes retain their explicit read/playback/admin/open scopes; the registry scope-completeness test passes. Regenerated contracts with `go run ./cmd/apigen` and schema metadata with `python3 scripts/schema-sync.py`; no generated files were hand-merged.
- Passed Linux, Windows and macOS `GOFLAGS=-p=2 go build ./...`, catalog vet, targeted catalog personal/history/privacy/ordering tests (6.750 s), download regressions (7.162 s), web `npx tsc --noEmit`, and `git diff --check`. Schema-sync's persistence tests also passed (19.721 s).
- Recursive grep found no conflict markers in the worktree. Git still lists the two paths as unmerged because staging is reserved for Justin. Context refresh failed on the read-only lock, as before.
