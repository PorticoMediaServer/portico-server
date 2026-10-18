# Accounts and security independent review

## 1. Outcome
Five findings confirmed with failing repository tests and fixed: **0 blocker, 2 high, 3 medium, 0 low**. Hosted signer revocation now fences pending sign-ins and Cast redemption/reconnection; member-device decisions recheck live authority inside their transaction; waiting devices cannot bypass remote sign-in or account schedules; restore scrubbing preserves the rollback backup's integrity and retries failures safely.

Reviewed base `aac183d` on `ehler-homelab-mini-1`. No commits, push, deployment, external-service access or subagents. Read both programme briefs, the source lane's complete report including merge notes, relevant project/verification guidance, documents 01/15, BUG-REGISTER, FIX-LOG and V01. Context refresh failed on its read-only lock directory; existing guidance remained readable.

## 2. Decisions I made
- Preserve the deliberate HTTP, password, no-lockout, unencrypted-state, TV-role and independent API-credential lifetime decisions. API keys remain live across restore exactly as the merged base specifies.
- Revocation is checked in the same write transaction as initial admission or descendant provenance recording. Pending second-factor completion refuses before spending a recovery code. Descendant provenance reads propagate storage errors.
- Re-resolve a device-management caller's current session tier in its transaction: demotion answers forbidden, without ending a valid sign-in. Revoked sessions still fail authentication. Both account and session must authorize the decision.
- A pending-device response is still admission and must pass the remote policy and schedule before it commits a device/account/invitation.
- A corrupt restore outcome or unscrubbable automatic rollback copy stops startup. This is the existing fail-closed restore policy applied consistently, not a new owner approval step.
- Pre-restore scrubbing verifies the original manifest, journals an in-progress scrub, keeps original state-file hashes, streams updated database/WAL hashes, then atomically publishes the manifest. Completed copies are skipped on subsequent startups. Database/WAL symlinks are rejected before opening SQLite.

## 3. Bugs
These are reconciliations of the original audit IDs, separate from the five independent findings below. “Already fixed” means current source plus the targeted checks where listed; it does not assert device acceptance.

| ID | Status |
|---|---|
| B-01.1 | Already fixed: closed-hours renewal refuses without revocation; pending-device bypass additionally fixed by R5. |
| B-01.2 | Fixed: existing issuance guard retained; R5 closes the pending-device path. |
| B-01.3 | Already fixed by API-platform: credential admission/scopes exist; outside this review's API-key boundary. |
| B-01.4 | Already wired; R2 fixes stale administrator device decisions. HTTP approval-switch regression passes. |
| B-01.5 | Already fixed: NULL-safe expiry/pruning. |
| B-01.6 | Already fixed: pending TV registration commits and survives until approval. Regression passes. |
| B-01.7 | Already fixed: owner exemption and real cross-account device list. Regression passes. |
| B-01.8 | Already fixed: installed claim defines Hosted attachment. |
| B-01.9 | Already fixed in client-core: 403 retains sign-in. No new client changes here. |
| B-01.10 | Already fixed: account/profile limits narrow one age-scale predicate. Rating regressions pass. |
| B-01.11 | Already fixed for listed spellings/normalization; conservative ambiguous-country interpretation remains deliberate. |
| B-01.12 | Already fixed: member/profile erasure blanks play-history names. Member deletion regression passes. |
| B-01.13 | Already fixed: transactional registration redeemer installed; invitation and self-registration regressions pass. |
| B-01.14 | Already replaced by networking's shared locality function; real proxy acceptance remains outside this lane. |
| B-01.15 | Already fixed: cancellable bounded hashing wait. |
| B-01.16 | Already fixed: PIN proof computed outside writer transaction. |
| B-01.17 | Already fixed: installation IDs moved from addresses; other devices' IDs hidden. |
| B-01.18 | Already fixed for ended session retention. Not fixed here: old failed-attempt ledger pruning remains the source lane's disclosed low-priority gap. |
| B-01.19 | Already fixed at audited revocation paths: family hook used; policies/tier changes no longer revoke. |
| B-01.20 | Already fixed at audited administrative refusals; R2 also preserves forbidden versus signed-out on live demotion. |
| B-01.21 | Not fixed: source lane explicitly retained ordinary restricted-profile device/TV actions as shared-account behaviour. Cast targets are separately refused. |
| B-01.22 | Not fixed in full: retired routes/table removed; development Hosted/viewer-only branches and other legacy state remain. No current production issuer that widens into those branches was found. Removing them is a broader identity cleanup, not one of these five reproduced defects. |
| B-01.23 | Not fixed: historical Apple explicit-signout incident not reproduced; physical Apple environment required. |
| B-15.1 | Fixed with B-01.2/R5. |
| B-15.2 | Already fixed with B-01.5. |
| B-15.3 | Already addressed by networking's canonical source-address derivation; real proxy acceptance not performed. |
| B-15.4 | Not changed here: first-run boundary belongs to networking; current origin/content-type/Host guards inspected. Deliberate reachable-first-run setup remains. |
| B-15.5 | Fixed for session credentials in backup/restore, including R3/R4. Password hashes and factor seeds remain by decision. Original backup scrub regression preserved and passes. |
| B-15.6 | Already unified by networking; no alternate parser added. |
| B-15.7 | Not fixed in private architecture document (read-only). Exact correction from source lane remains required: device records and bearer/refresh credentials, not proof of a device private key. |
| B-15.8 | Already addressed by networking's trusted-proxy origin handling; no live proxy tested. |
| B-15.9 | Not changed: deliberate no-lockout trade-off. |
| B-15.10 | Already fixed with B-01.15. |
| B-15.11 | Not changed here: owner home-networks definition and hidden-client-address deployment choices belong to networking. |
| B-15.12 | Fixed for learned-revocation persistence by R1; existing check-in certificate verification/revocation tests pass. Fleet revocation-delivery latency is not established here. |
| B-15.13 | Not changed: approver's TV role is an explicit product decision. |
| B-15.14 | Already fixed for URL/header exposure with B-01.17; intended unauthenticated remembered-name lookup remains. |
| B-15.15 | Already fixed: public response omits build fingerprint; version intentionally remains. |
| B-15.16 | Already narrowed CSP paths; physical Cast sender acceptance remains outstanding. |
| B-15.17 | Not fixed in this lane: release-feed network policy belongs to ops-packaging. No independent new defect filed here. |
| B-15.18 | Already fixed: folder-picker rechecks resolved paths. |
| B-15.19 | Not fixed in this lane: build/release workflows belong to ops-packaging. |
| B-15.20 | Not fixed in this lane: provider override/credential availability belongs to providers; shared shipped credentials are deliberate. |
| B-15.21 | Already fixed: profile-wide PIN attempts plus per-device guard. |
| B-15.22 | Not present as the old mandatory-HTTPS behaviour: programme explicitly accepts HTTP from every address. |
| B-15.23 | Already fixed for code creation/invitation cheap limiter; ordinary TV polling remains exempt on purpose. |
| B-15.24 | Already fixed: safely encoded download name and formula-safe CSV cells. |
| B-13.14 | Already narrowed Cast session grant; R1 fixes post-revocation Cast reissuance. Reach/admin/key/token denial and own-only unpairing regressions pass. |
| N-V01.1 | Fixed in full for new-session-after-revocation cases by R1, in addition to the source lane's existing family sweep. |
| B-09.19 | Fixed for sign-ins/rollback-copy usability by R3/R4. Independent API keys deliberately retain backup-time standing. |
| B-13.21 / Q-5 | Already fixed: old invitation cannot undo a later disable; recipient binding by signed Portico username. Invitation regressions pass. |

## 4. API changes
No routes or request/response shapes added, changed or removed. Existing registry scopes stay unchanged; no client-core changes or apigen regeneration needed. Device-list/trust requests recheck authority inside their transactions. Refused pending-device sign-ins now use the existing remote-policy/schedule errors. Cast redeem/reconnect after signer revocation use the existing 403 refusal. Invalid restore-result JSON now returns a storage error instead of being mistaken for no restore.

## 5. Schema changes
No SQL schema changes; schema-sync is unnecessary. Backup manifests gain `signInsRemoved` as internal completion metadata. A temporary `.sign-in-scrub` marker records an interrupted pre-restore scrub; it is removed after atomic manifest publication.

## 6. Owner or viewer settings added or wired
No new settings. Existing remote sign-in (allow / owner-only / off, default allow) and device approval (boolean, default false) now hold together, including waiting-device registration. Existing account schedules now hold on that waiting path too. Existing screens remain usable.

## 7. Needed from others
- Integration must complete the restriction-route matrices for other lanes' new routes: six read routes (downloads/console), twenty-two write/HEAD routes (subtitles, credentials/MCP, library metadata), plus reconcile three invalid-download-grant probes. Classify with real hidden-vs-absent assertions where routes carry content; do not blanket-exempt them. These matrix/table/route definitions are unchanged from HEAD. This run did not reproduce a hidden-title leak.
- Keep the source lane's native/shared-package synchronization, real-device waiting-state/permission UI and rating picker follow-up. This review does not establish which screens other client lanes have since finished.
- Apply the source lane's exact private architecture correction for device records versus keys and the privacy sentence: “The owner of this server can always see what is played on it, by whom and on which device. Pausing or clearing your own history does not change that.”
- Preserve the backup completion marker and integrity workflow when ops-packaging combines restore changes. Bad restore records or corrupt automatic rollback backups now produce a startup error; repair the named artifact and restart to retry.

## 8. Test notes for other environments
Targeted checks performed here (no full suites/build pipelines):
- Linux `GOFLAGS=-p=2 go build ./...` passes.
- `GOFLAGS=-p=2 go vet ./internal/identity ./internal/httpapi ./internal/backup ./cmd/server` passes.
- Identity targeted tests pass: all new reproductions, member-device approval, live library/profile/tier changes, member deletion, ratings, TV signer inheritance, backup byte scrubbing/restore signout, open/invite-only registration.
- HTTP targeted tests pass: Cast authority/refusals, own-only unpairing, revoked-signer redeem/reconnect, remote-sign-in policy, approval switch and administrator/member boundaries. Broader combined HTTP run fails only the existing two restriction matrices described above.
- Hosted targeted tests pass: root revocation/restart, admitted families, check-in certificate validation and revocation.
- Access targeted tests pass: live role changes, schedules, invitation/disabled-member/recipient rules, age-scale limits and Live TV programme limits.
- Backup targeted tests pass: original no-live-session/failing-scrub tests, pre-restore integrity and interruption/corruption/link guards, restaging and ordinary apply cycle.
- cmd/server targeted tests pass: signout once, invalid restore records, failed-copy retry, and a real backup Create → Stage → Apply → startup scrub → Stage automatic rollback copy by id and absolute folder.
- Final-tree `GOFLAGS=-p=2 GOOS=windows go build ./...` and `GOFLAGS=-p=2 GOOS=darwin go build ./...` pass. Cross-compilation does not establish filesystem/device runtime acceptance.

Cold follow-up: on Windows/macOS, create two signed-in devices, take and restore a backup, restart, confirm both sessions require sign-in, then stage the automatic pre-restore backup by id and folder; pass means both stage successfully. Sign in again and restart normally; pass means that session survives. Interrupt startup during rollback-copy scrubbing, restart, confirm the marker resumes and the backup remains restorable. A corrupt manifest/state file must refuse, rather than acquire new hashes. Tests use injected interruption; abrupt power loss and filesystem durability on real hardware are not proven.

Hosted/Cast acceptance: admit a member with signer A, leave a local factor challenge, approved TV code and Cast bootstrap uncollected; pair another Cast screen and retain its reconnect token. Learn a root-verified revocation of A through check-in. All existing A sessions must end and all four pending/reconnect attempts must refuse; signer B and password sessions must still work. For device-authority races, queue an administrator's trust change behind a writer, demote/sign out that administrator first, release the writer, and verify the device was not changed (403 for demotion, authentication failure for logout).

## 9. Not finished
All five independent findings are fixed. Remaining programme work: cross-lane restriction-matrix coverage; source lane's disclosed legacy cleanup/failed-attempt retention and historical Apple incident; any outstanding client/document work; physical-device/proxy/Hosted and abrupt-crash acceptance. No full test suites, release pipelines or deployment were run. No untouched-base test binary was executed; unchanged restriction classifications establish the pre-existing coverage gap by source comparison.

## Findings (phase 1, before fixes)

### R1 — A revoked Hosted signer can admit new sessions through credentials issued earlier
Severity: high. `server/internal/identity/portico_accounts.go:126,135`, `two_factor.go:369`, `quick_connect.go:563`; `server/internal/social/cast.go:325,415`.
Learning a revocation sweeps only families existing at that moment. Pending local second-factor challenges and approved TV codes retain the signer and can create a later family; a Cast device token reconnects after its family was revoked, and an unredeemed bootstrap can also create one. Recording/inheriting the signer checks no revocation table. The next certificate of the same revision skips another sweep, so the new family remains usable. An assertion verified before the sweep can likewise race with admission. Smallest fix: check signer revocation in the same transaction that records admission, and before staging a Portico second-factor challenge. Reproduce pending challenge, TV collection, Cast redemption and reconnect after revocation, plus fresh admission racing with revocation.

### R2 — Member device management trusts a principal checked before its transaction
Severity: high. `server/internal/identity/member_devices.go:130,235` and `server/internal/httpapi/member_contract.go:30,45`.
Both device listing and trust mutation use the caller's previously resolved role without rechecking its session or live membership in the transaction. A queued request can approve/block a device after its administrator was signed out or demoted; ownership transfer can leave the former owner deciding about the new owner's devices. Other access writes correctly use an in-transaction authority callback. Smallest fix: resolve live family and membership inside these snapshots/write transactions, retaining the narrower session role. Reproduce a captured principal used after logout/demotion/transfer and verify no mutation.

### R3 — Scrubbing the automatic pre-restore backup invalidates its integrity manifest
Severity: medium. `server/cmd/server/restore_sessions.go:58`, `server/internal/backup/apply.go:213`, `stage.go:205`.
The pre-restore manifest hashes its database (and possibly WAL), then startup scrubs the database without updating that manifest. Stage checks these hashes and refuses the very backup meant to recover the prior state. The startup test makes a manifestless directory and only counts sessions, so it misses restoration failure. Smallest fix: a backup-owned scrub operation that preserves verification of untouched state files and refreshes only database/WAL entries after scrubbing. Reproduce a real manifest, startup scrub, then Stage by id and path.

### R4 — Restore scrub failures are silently abandoned
Severity: medium. `server/cmd/server/restore_sessions.go:30,46,61`.
An unreadable restore result is treated as no restore; pre-restore scrub failures are merely logged after the main database is marked handled, so next startup never retries. This contradicts the report's no-live-sign-ins-in-backups guarantee. Smallest fix: propagate unreadable result/scrub errors, validate a successful restore's identity, and retry set-aside copies on later starts even if the primary database was handled. Reproduce invalid result and a failed scrub followed by a repaired copy.

### R5 — A waiting device bypasses sign-in admission and commits otherwise forbidden registration
Severity: medium. `server/internal/identity/direct_accounts.go:277`, `self_registration.go:207`.
The admission checks live in IssueTx, but directSignInTx returns a successful pending-device response before reaching IssueTx. With remote owner-only plus device approval, outside self-registration creates the member (and consumes an invitation) even though that member may not sign in there. Account schedules are similarly skipped while waiting. Smallest fix: apply both admission rules at directSignInTx before device registration/pending response, as well as at issuance. Reproduce owner-only registration with approval enabled, ensure no account/device/invitation changes; cover outside-hours pending-device login.

## Fixes

| Finding | Resolution | Passing reproduction |
|---|---|---|
| R1 high | Fixed: in-transaction signer gate for initial/provenance admission and pending second factor; fail-closed inheritance. | `TestRevokedSignerCannotFinishAPendingTelevisionApproval`, `TestRevokedSignerCannotFinishAPendingSecondFactor`, `TestPorticoAdmissionRechecksSignerAfterAssertionVerification`, `TestRevokedSignerCannotRedeemOrReconnectACastTarget` |
| R2 high | Fixed: family liveness/current session tier plus membership tier checked inside device snapshot/write transaction. Demotion remains a valid sign-in. | `TestMemberDeviceDecisionRechecksLiveAuthority` (logout, ownership transfer, administrator demotion), existing device-management tests |
| R3 medium | Fixed: pre-restore scrub preserves original integrity checks, updates database/WAL entries atomically, and resumes through a durable marker. | Upgraded `TestStartAfterARestoreSignsEveryoneOutAndScrubsTheSetAsideCopy`, `TestRealRestoreKeepsItsScrubbedRollbackBackupRestorable`, `TestPreRestoreScrubResumesWithoutRehashingChangedState`, corruption/link guards |
| R4 medium | Fixed: invalid restore result/errors stop startup, copy failures retry independently of main handled state. | `TestUnreadableRestoreResultStopsStartup`, `TestRestoreScrubbingRetriesAFailedSetAsideCopy` |
| R5 medium | Fixed: policy and schedule run before successful waiting-device admission. | `TestPendingDeviceStillPassesSignInAdmission`, `TestRemoteOwnerOnlyRegistrationCannotCommitAWaitingMember`, existing remote-policy/approval/schedule coverage |

Snapshot 001 contains the first admission/device group plus reproduction tests. Final cumulative snapshot **002** contains all five fixes and preserved original tests. No finding remains unfixed.

### Reproduction results before implementation changes
- R1: identity tests failed: "a revoked signer issued a television session", "a revoked signer completed second factor" (owner session), "an assertion verified before revocation admitted after revocation". HTTP Cast test failed for both redeem and reconnect: 200, expected 403.
- R2: each logout/transfer/demotion subtest failed: "stale administrator listed devices", "stale administrator blocked a device"; readback showed `denied` rather than `approved`.
- R3: upgraded startup test failed staging the scrubbed backup: `backup files do not match their manifest`.
- R4: invalid-result test failed (`treated as no restore`); failed-copy test failed (`did not stop startup`, then `1 live sessions` on retry).
- R5: pending-device test failed with nil error; outside registration returned `pending=true err=<nil>`.
All are repository tests, run against production code before fixes. The first shell attempt used incorrect relative output paths under server and wrote no test files; corrected paths before reproduction.

### First fix group
R1 fixed: signer revocation is checked inside initial Portico admission and admission recording/inheritance; a pending second factor checks it before consuming a code. Inheritance propagates database errors instead of silently dropping provenance. R2 fixed: member-device reads and decisions recheck family liveness and both tiers in the transaction. Existing device tests now supply authenticated principals rather than fabricated role-only values. R5 fixed: sign-in policy and account schedules run before pending-device registration can return success. Targeted identity and HTTP regressions pass; Linux `GOFLAGS=-p=2 go build ./...` and identity vet pass. Restore tests intentionally still fail pending R3/R4 fixes.

### Restore fix design and follow-up verification
R3/R4 now use a backup-owned pre-restore scrub. The original manifest must verify before mutation. A durable marker ties an interrupted scrub to that verified manifest; state files continue to require their original hashes. Database/WAL hashes are streamed after the scrub and published atomically; a completion flag prevents rehashing the database on every startup. Invalid/unknown/untimed restore results and scrub errors stop startup; copy retries run independently of the main database's handled marker. The startup fixture now reads backup state through a read-only SQLite connection (opening a normal writing handle can itself change file bytes after manifest creation). Added a real Create → Stage → Apply → startup-signout → Stage rollback-copy integration test, rather than relying only on a synthetic folder.
R2 additionally resolves the current token tier inside the transaction, so concurrent demotion is 403, not a false sign-out. The demotion reproduction now uses a real administrator, not a manually demoted owner.

### Existing restriction guards
The targeted broader HTTP run passed the account/Cast/member checks but the two pre-existing restriction matrices failed: six read routes and twenty-two write/HEAD routes from downloads, console, subtitles, API credentials/MCP, and library metadata lack classifications. Three download grant probes also answer `401 invalid_download_grant` for an invalid URL grant although the session is valid. These route/table definitions are unchanged from HEAD and concern other lanes; this is a coverage gap, not a reproduced hidden-title disclosure. Integration must reconcile them without exempting title-bearing routes blindly. No matrix entries were weakened here.
During final diff review, found a test-file name collision: the new backup scrub tests had replaced existing backup tests. Restored the original file verbatim from HEAD and moved the new tests to `pre_restore_scrub_test.go` before the second snapshot. Original tests remain intact.


### Final verification and delivery
Final backup/cmd targeted tests, Linux/Windows/macOS builds, and backup/cmd vet pass after the final atomic intent-marker/manifest hardening. Identity/httpapi vet and targeted tests pass after their last production edits. `git diff --check` is clean; the original backup scrub test file is identical to HEAD. Snapshot 002 is final and cumulative against `aac183d`; no worktree edits follow it. Two high and three medium findings fixed; none left from this review. Existing cross-lane matrix coverage and physical-environment acceptance are explicitly outstanding above.
