## Outcome
Nine operations findings confirmed and fixed: **0 blocker, 2 high, 7 medium, 0 low**. Malformed restores cannot replace/delete the live database; restore completion survives failed result writes and interrupted cleanup; local recovery copies remain visible and bounded with external backups; pre-upgrade copies are durable and scrubbed; early recovery follows its parent; successful client reports stay readable; manual backups cannot become scheduled copies; saved client report age retention now runs.

Reviewed base `1097d54` on Ubuntu `ehler-homelab-mini-1`. No commits, push, deployment, release or subagents. Context-sync could not write its read-only lock; existing canonical guidance was read. Programme/lane briefs, source reports and merge notes, the operations/engineering audit entries, register/fix log and verification directions informed source review; reports were checked against code and focused reproductions.

## Decisions I made
- Keep the programme's accepted HTTP, plaintext state, owner-chosen destinations, service accounts, API-key restoration and no compatibility rules.
- Treat a backup manifest as a list of supported state files, rather than permission to overwrite arbitrary server files. Validate its journal again before moving live state. Windows normalization/device aliases are refused on Windows; duplicate case aliases are refused on Windows/macOS; ordinary Linux names keep Linux semantics.
- Restore has durable terminal states with a stable completion timestamp. Cleanup replay neither re-verifies nor rolls back a database that has since accepted writes. Failed verification without a recovery database remains unresolved.
- Keep regular backups in the selected destination and recovery copies beside the live database. Expose both, prune local recovery copies to three at restore completion, and preserve manual copies.
- A manual request during a scheduled copy gets the existing retryable busy response; it never borrows scheduled retention.
- Pre-upgrade copies use the existing identity scrubber. Passwords, second factors and independent API credentials retain the established backup policy.
- Client-category log retention applies to uploaded reports. No entry keeps the existing count-bounded history; a saved 1–90 day entry is applied on startup and each five-minute maintenance tick. Today's upload budget survives retention.

## Bugs
Original audit statuses are separate from the nine independent findings below. “Already fixed” means inspected current source and the focused checks listed here; native acceptance is not implied.

| ID | Status |
|---|---|
| B-05.28 | Already fixed in platform source: Windows root identities/helpers and linked roots exist; builds compile. Physical Windows filesystems remain unverified. |
| B-09.1 | Already fixed: detached restore job and separate long diagnostics lane. Stage/scrub/crash checks pass. |
| B-09.2 | Already fixed: parent relaunches exit 75; owner Restart is registered/scoped. Relaunch/drain and owner-scope tests pass. Unexpected fatal worker exits propagate an error to the parent; installed service managers provide their configured crash recovery. |
| B-09.3 | Already fixed: Linux maintainer scripts branch on install/upgrade/removal and retain running state. Native package ordering acceptance remains. |
| B-09.4 | Already fixed: scheduled launch replaced with SCM service and recovery actions. Windows runtime acceptance remains. |
| B-09.5 | Already fixed in source: LocalService/service SID, state ACL provisioning and firewall; account configuration survives upgrades. Native access/UNC acceptance remains. |
| B-09.6 | Already fixed: packaged null output plus rotated server log; no unbounded wrapper files. |
| B-09.7 | Already replaced by networking's connection admission/reserved console capacity; real SIGTERM under light load passes. Full capacity target is unestablished. |
| B-09.8 | Already removed under the accepted-HTTP policy; home networks determine locality. Network/overlay acceptance belongs to networking. |
| B-09.9 | Already wired: startup reads the conversion directory and tracks requested/applied value. |
| B-09.10 | Not present: mandatory-HTTPS setting/old option paths removed by networking. |
| B-09.11 | Already fixed: feed writer/parser/default feed/opt-out/cache/owner notice. Generated-feed tests pass. |
| B-09.12 | Already wired: notices run on startup/five-minute ticks. |
| B-09.13 | Already replaced by identity's file deletion/missing-title model; no state-folder file trash retained in this path. |
| B-09.14 | Already fixed in unit/guidance: render/video groups and additive writable-path drop-ins. Actual GPU/selected-path acceptance remains. |
| B-09.15 | Already fixed: boundary-only permission check/fix; explicit unsupported Windows assessment. |
| B-09.16 | Already fixed: streaming hash verification. R1 additionally restricts supported filenames and regular files. |
| B-09.17 | Fixed further: publication/directory durability (R2), true manual retention under concurrent scheduling (R7). Existing shared admission, failure alert, disk checks and DST behavior retained. |
| B-09.18 | Fixed further: external destination plus visible/restorable/bounded local recovery copies (R3). |
| B-09.19 | Fixed further: pre-upgrade copies now use the same scrub policy (R8); existing restore sign-out and pre-restore scrub tests pass. API credentials/passwords retain deliberate backup semantics. |
| B-09.20 | Already fixed: no-lock restore admission refuses instead of claiming success. |
| B-09.21 | Already fixed in default/package binds: dual-stack-capable addresses. Real IPv6 reachability belongs to networking/native acceptance. |
| B-09.22 | Already fixed: joined HTTP/Hosted shutdown before cleanup. Real SIGTERM/readiness tests pass. |
| B-09.23 | Fixed further: uploaded-client age retention wired (R9). Early/history/redaction/file retention checks pass. Source-wide log-level instrumentation remains each domain's responsibility. |
| B-09.24 | Already unified by networking's shared locality policy; no new proxy classifier introduced. |
| B-09.25 | Already addressed by networking's bind/proxy parsing and recovery surface. No live proxy/environment deployment tested. |
| B-09.26 | Already addressed in image/guidance: GPU libraries, build-only Python, TZ/discovery instructions. Docker/GPU behavior is unverified here. |
| B-09.27 | Already fixed: configured portable launchers; two packaging fixture tests pass. |
| B-09.28 | Outside this review: scheduling/screen wording belongs to scheduler/client lanes; no timing semantics changed. |
| B-09.29 | Already addressed by the current persistent recovery surface; R5 closes early recovery's orphan lifetime. HTTP recovery and child EOF tests pass. |
| B-09.30 | Not a reproduced defect: online copy pins a consistent snapshot. Large-library WAL growth during active import still needs measurement. |
| B-09.31 | Already replaced by macOS system daemon/package/uninstall; launcher only opens the console. Native lifecycle acceptance remains. |
| B-09.32 | Already fixed: recover-owner requires the instance lock before opening state. R8 adds copy-scrub wiring there. |
| B-09.33 | Not present as described: mandatory-HTTPS preference removed; health-check/readiness agree in real local process tests. |
| B-09.34 | Already narrowed: coarse startup/recovery replies without raw errors, public version without build fingerprint. Recovery secrecy regression passes. |
| B-09.35 | Fixed further by R1/R2 recovery hardening; actual foreign bare-DB rollback and healthy-state acceptance tests pass. |
| B-25.1 | Already fixed with B-09.11; generated bytes drive the reader. |
| B-25.2 | Already fixed: CI/Makefile/builder use shared vet policy for maintained SQLite driver. |
| B-25.3 | Already fixed: absolute Windows ZIP output, verified with portable fixture. |
| B-25.4 | Already fixed in release/image recipes: architecture-qualified FFmpeg inputs. No Docker build run. |
| B-25.5 | Outside this worktree: Hosted copy/checker follow-up listed as fixed in FIX-LOG; not independently run here. |
| B-25.6 | Already fixed: missing/unpinned signer fails closed; no invented official key. Release seal fixture checks pass. |
| B-25.7 | Already fixed: final package/feed authenticated inventory; tamper fixtures pass. |
| B-25.8 | Already fixed: authenticated macOS signed derivative; eight fixtures pass. Real Apple codesigning remains unverified. |
| B-25.9 | Already required in release recipe: exact source SHA plus qualified media/performance tier. Workflow was not executed. |
| B-25.10 | Not fixed here: native/device release gates are outside this worktree; actual Apple/Vega/device acceptance remains. |
| B-25.11 | Already supplied in this tree/package/container recipes: font/icon/SQLite/scaling/decoder/hls notices. Native distribution and exhaustive inventory remain release-integrator work. |
| B-25.12 | Not resolved by code: Apple component distribution rights require the release owner's review. No legal conclusion asserted. |
| B-25.13 | AST guard works; recovery goroutine corrected. Two unchanged storage/webhooks violations are listed under Needed from others. |
| B-25.14 | Existing SQL guard correctly says it covers literals; private architecture overstatement/dynamic-query coverage belongs to database/docs owners. |
| B-25.15 | Outside this review: comprehensive hostile-route battery coverage remains API/security test-owner work. |
| B-25.16 | Already adjusted: release race timeout 60m; qualification 120m. Runtime duration of full tiers not measured here. |
| B-25.17 | Already retired: obsolete Python schema harness; current Go schema tests remain. |
| B-25.18 | Not fixed here: migrations 3/8/11 still have data-dependent historical work. Squashing domain migrations is the database integrator's scope; durable/scrubbed pre-upgrade safety copies are implemented. |
| B-25.19 | Already fixed: immutable resolved SHA across tests/build/publish and final tag check. No workflow run. |
| B-25.20 | Already fixed with B-09.3. |
| B-25.21 | Outside this worktree: cross-repository native/Roku dependency tuple pinning remains coordinator/client work. |
| B-15.17 | Accepted owner-selected HTTP(S)/internal feed destinations remain, per programme unsafe-choice rule. Response/time/redirect bounds and credential refusal exist; release links are HTTPS-only. |
| B-15.19 | Partly addressed by existing pinned signer/package integrity. Mutable action refs in server-trust/first CI job, tagged Docker bases and advisory vulnerability-scanning omissions remain release-policy follow-up; installers remain deliberately unsigned previews. |

## API changes
`POST /v1/admin/backups`: same request and 202 receipt; adds existing `409 request_in_progress` when a scheduled backup is active. Previously a manual request could return the scheduled job's id. Metadata and generated contracts regenerated with `GOFLAGS=-p=2 go run ./cmd/apigen`. Generic client-core error handling already supports this envelope; no request/response parser/type changes are needed. No routes removed. Existing owner/admin/session scopes and restriction classifications remain.
`GET /v1/admin/backups` keeps its shape and now includes local pre-restore copies with an external regular destination; their existing ids work for restore/delete.

## Schema changes
No SQL/schema changes; schema-sync unnecessary. Internal restore journal adds `completedAt` and terminal `restored`/`rolled_back` states. The result timestamp is stable RFC3339Nano across completion retries; its public type stays a date-time string. No compatibility path added.

## Owner or viewer settings added or wired
- Existing `logRetention` client-category days: integer 1–90 when present; default absent means count-bounded retention. Uploaded client reports now consume it during initialization and five-minute maintenance. Existing log retention screen exists. Registry consumer/description updated; no setting added.
- Existing backup destinations, scheduled keep count and manual retention preserved; local recovery-copy cap (three) now holds independently of the schedule. No new screen/control.

## Needed from others
- Storage/webhooks: contain `internal/storage/inventory_watch_windows.go:32` and `internal/webhooks/dispatch.go:194`. The unchanged baseline violates the working AST goroutine guard. No outside-domain code changed here.
- Library/watcher integration: `TestStoredSettingsHaveRuntimeConsumers` reports missing reviewed consumer for `administration.LibrarySettings.monitorChanges`. The field/guard are unchanged from HEAD; this is guard inventory coverage, not proof that the watcher is unwired.
- Native/release integrators: OS install→upgrade→uninstall, GPU/tuner/container, signing/custody/release workflow and real power-loss acceptance. Configure approved signing identities/keys outside source. No signing secrets inspected.
- Database integrator: historical migration/backfill policy and dynamic-query coverage. Native/client coordinator: release gates and pinned sibling inputs. Private documentation owners: reconcile operations/backup paths/services and API-key restoration policy with the programme/source.
- Other client lanes retain ownership of Restart, progress and report screens on every first-party platform; this review changed no screen. Current platform parity was not verified here.

## Test notes for other environments
Focused Ubuntu tests passed for restore stage/hash/scrub/crash/rollback/result-write failure/terminal replay/newer data; external recovery list/stage/delete/prune; manual versus scheduled admission; migration publication-failure and scrub-failure gates; real identity migration-copy sign-out isolation; incremental space reclamation/WAL contents/pruning; report cap/tied receipt/readback/redaction/budget/owner download/delete; client age-retention initialization; recovery secrecy/parent EOF; restart relaunch/drain; production router/pattern compatibility/credential scopes; early/previous log retention/redaction; emitted update feed/cache/response bounds.
Real disposable server tests passed for readiness/health agreement and SIGTERM drain under light load with no children left. Builds passed for Linux, Windows amd64/arm64 and Darwin arm64; no native runtime acceptance claimed. Targeted vet passed on backup, persistence, servicelog, cmd/server and httpapi. Portable packaging (2), macOS sealing (8), final artifact custody/tamper (3) fixtures passed. API artifacts regenerated; diff check clean. No full suites or release/build pipelines run. Two broader guards have the unchanged cross-lane gaps above.

Cold acceptance on disposable Windows/macOS/Linux installations: (1) use state and regular backup destinations on separate disks; take a manual backup, modify a fixture title/state, restore, and require the pre-restore row appears and restores by id; after five completed restores require only three local recovery copies. Manual copies must survive scheduled pruning. (2) interrupt at terminal completion, block result-file publication or cleanup, repair and restart; require previous/newly committed state survives and the completion timestamp/sign-out happens once. Corrupt/reserved manifest paths must refuse before live state moves. (3) install a same-baseline upgrade with a pending migration; require a synced, sign-in-free recovery copy and preserved passwords/state; interrupted publication/scrubbing must prevent migration. (4) create reports with a saved short client retention, restart and let maintenance tick; expired reports go, recent reports and today's budget stay. (5) force an early state-directory startup failure, verify the safe recovery page, then stop/kill its supervising parent; no worker/socket may remain. Exercise service install, reboot, running/stopped upgrades, configured accounts, stop and uninstall with real packages; original state/media remain. Follow the source lane's detailed GPU/UNC/device procedures. Real power loss/filesystem durability remains distinct from injected-error tests.

## Not finished
All nine independent operations findings are fixed. Outstanding work is native/power-loss/release acceptance and the explicitly assigned cross-lane guards, migration policy, client parity, licensing and supply-chain policy follow-ups above. No full suite, release, deployment or push was run. Final verification/snapshot receipt appears at the end of this report.

## Findings (recorded before each fix)

### R1 — A backup manifest can overwrite and then delete the live database
Severity: high. `server/internal/backup/stage.go:175–233`, `files.go:282`, `apply.go:406–421`.
`stateFileNames` accepts any relative name apart from a few traversal spellings. A valid backup manifest carrying a second database as `server.sqlite` passes its hashes. Switching first installs `portico.db` as the live database, then overwrites it with that state entry. If verification refuses, rollback puts the previous database back, then deletes it because the manifest did not list a previous state file called `server.sqlite`. The original database has already been moved out of its recovery folder. This is a data-loss defect with an owner-selected malformed backup, not a claim of unauthenticated access. Hash verification also follows links and can read non-regular files before staging refuses them.
Smallest fix: validate canonical, unique names against the supported state-file set, including reserved database/WAL names, before reading/copying; refuse non-regular files and intermediate links. Recheck the restore journal at startup before any moves.

### R2 — Restore completion discards its recovery state too early
Severity: high. `server/internal/backup/apply.go:316–343,427–428`.
Rollback removes the pre-restore manifest/folder before saving `restore-result.json`. A result write failure leaves a `rolling_back` marker whose next attempt cannot read its deleted manifest. Successful restore ignores staging removal failure; a leftover `applied` journal is verified again on later startup and can produce another restore result/sign-out or roll newer live state back. File moves are not synced at their directory boundaries before durable state markers advance.
Smallest fix: durably record terminal restored/rolled-back state and a stable completion time, publish the result, then clean up. Terminal journal replay must only finish publication/cleanup. Sync rename/removal directory boundaries before recording the next phase.

### R3 — External backup destinations hide and indefinitely retain local recovery copies
Severity: medium. `server/internal/backup/backup.go:280–285,491–589`, `stage.go:199`.
Apply always writes pre-restore copies under local `state/backups`, but List/Delete/resolve-by-id/Prune use only the configured external root. Owners cannot see or restore those recovery copies by id; pruning never removes them, so repeated restores accumulate full databases on the state disk.
Smallest fix: list the configured regular destination and the local recovery destination together; resolve/delete each id in the correct root; retain the newest three local recovery copies independently of scheduled retention.

### R4 — Pre-upgrade copies are pruned before their names are durable
Severity: medium. `server/internal/persistence/migration_backup.go:85–106`.
The copy's bytes are synced, but its rename is not followed by a directory sync. Older recovery copies are then deleted and migrations proceed. A power loss can retain deletion of old names without retaining the newest copy's name. The report claims a durable pre-upgrade safety copy, which this ordering does not establish.
Smallest fix: sync the recovery directory after publication and before pruning; propagate failures before migrations. Sync completed pruning as well. Fault injection should prove no migration/prune happens after failed publication durability.

### R5 — Early startup recovery no longer follows the supervising parent's lifetime
Severity: medium. `server/cmd/server/startup.go:244–258`, `main.go:153`.
Errors before a listener/reporter exists return out of `run`, cancelling its context and parent watcher. Main then starts `serveEarlyStartupFailure` with a new signal-only context. When the supervisor disappears, this recovery worker keeps its socket indefinitely because it no longer watches the parent pipe. Normal Stop eventually kills it after the 40-second drain timeout; parent failure leaves an orphan.
Smallest fix: give the early recovery surface the same parent-pipe fence as normal worker startup. Reproduce in a child process by closing its parent input without sending a signal.

### R6 — A successful client-report upload can immediately prune itself
Severity: medium. `server/internal/servicelog/clientlogs.go:134–139`.
Per-device and global retention sort equal-millisecond arrivals by a random id. At the cap, a newly inserted id below the tied existing ids is removed before Store returns its successful receipt. The owner cannot read that report; the account budget was still charged. Existing tie tests check paging/counts, not readback of each accepted receipt.
Smallest fix: reserve one retention slot for the just-accepted report, keeping the newest remaining four/99 entries. Reproduce with tied timestamps and known higher ids, and require readback plus unchanged caps.

### Reproduction results before each finding's fix
- R1: `TestRestoreManifestCannotReplaceTheLiveDatabaseAsAStateFile` failed: “malformed manifest deleted the live database during rollback” (`server.sqlite` missing).
- R2: `TestRollbackResultFailureCanResume` failed: “rollback could not resume after result storage was repaired”. Further trace: failSwitch also discards its journal and claims rolled_back if the previous database is missing, allowing startup after failed verification. A missing-copy reproduction is added before fixing that path.
- R3: `TestExternalDestinationIncludesLocalRecoveryCopies` failed: “external backup destination hid the local pre-restore copy”.
- R4: filesystem durability ordering established by source trace; power-loss reproduction is not claimed. A failure-injection test will cover its new publication boundary and the migration gate.
- R5: child-process test failed after three seconds: “early recovery worker outlived its parent's pipe”.
- R6: both device/global retention tests returned 201 then owner readback returned 404 `not_found`.
The first report/parent test write used server-relative paths from inside server and wrote no files; corrected before running those reproductions.

## Fixes
R1: canonical supported state names now validated before hashes/copies and again from the startup journal; duplicate names and linked/special manifest files refused. Reserved live-database manifest reproduction plus existing targeted stage/scrub/unresolved tests pass. More validation regressions are being added before the first snapshot.
R2: terminal restored/rolled_back journal states now retain one nanosecond completion identity across retries. Recovery manifests remain until terminal publication and result durability; cleanup errors propagate, and terminal replay performs no verification or rollback. A missing previous database leaves failed verification unresolved. Renames, removed WALs, nested state directories, staging publication and ordinary backup directories are synced before phase/publication boundaries. Existing crash-point regressions, result-write failure, missing-copy refusal and new terminal cleanup replay/newer-write tests all pass. Directory fsync is the Unix guarantee; Windows retains flushed files plus NTFS rename journaling, requiring native power-loss acceptance.
R3: regular and local recovery destinations are listed together; recovery ids resolve/delete against their actual local folder. A completed restore prunes local recovery copies to three even with no backup schedule, and scheduled pruning sees both destinations. External-destination list/stage/delete/prune regression and restore crash/replay regressions pass.
R4: the migration-copy directory is synced before pruning or allowing migrations, with pruning synced afterward. `TestMigrationWaitsForDurableRecoveryCopyBeforePruning` injects a publication sync error, preserves every old copy and proves the new migration did not run; after repair, migration and three-copy retention pass. This is failure injection at a new boundary, not an actual pre-fix power-loss test.
R5: early recovery now watches the supervised worker's input pipe. The isolated child-process EOF reproduction passes in 0.033s. Recovery's stop goroutine also uses the existing panic-containment helper.
R6: the just-accepted report owns one retention slot; the other four/device and 99/global are selected separately. Both tied-timestamp 201→owner-readback reproductions pass alongside existing budget/redaction/owner-only/download/delete/cap tests.

Checkpoint: snapshot 001 contains R1/R2 implementation and the initial reproduction files, with the known R3/R5/R6 tests still pending then. Linux build and backup vet passed before that snapshot. All six fixes are now in the working tree; final combined checks and a final snapshot remain.

### R7 — A manual backup request can become a scheduled, pruneable copy
Severity: medium. `server/internal/backup/backup.go:356–366`.
Source review of backup admission found one further retention defect while checking the combined tree: a manual request joins an active scheduled job and aliases its operation id to that job. Its copy remains `scheduled`, so automatic pruning deletes what the owner deliberately requested as a manual copy. The inverse path already refuses joining a manual job. Smallest fix: refuse joining a different backup kind with the existing busy response; retry later creates a real manual copy. Add a blocked-scheduled-copy reproduction before changing admission.
R7 reproduction failed before the fix: “manual backup joined pruneable scheduled job …: <nil>”. Different-kind admission now returns ErrBackupRunning, mapped to the existing 409 request_in_progress contract. Same-kind replay/join remains. Registry-generated artifacts regenerated; a real owner-route busy-response regression is added.

### R8 — Pre-upgrade recovery copies retain live sign-ins
Severity: medium. `server/internal/persistence/migration_backup.go:30–89`, `server/cmd/server/main.go` database/restore opening.
The new migration safety-copy path bypasses the identity scrubber entirely. It publishes a full online database copy with live session families and renewal/pending-admission secrets, while regular and pre-restore copies are scrubbed. This misses the source report's policy that every backup copy holds no live sign-ins. The current schema embeds migrations 1–11. This reproduction exercises the real copy boundary and a synthetic next migration; a released upgrade exploit is not claimed.
Smallest fix: inject the existing identity copy scrubber into persistence at normal startup and owner-recovery entry points, invoke it on the unpublished independent copy, and refuse publication/migration if it fails. Preserve passwords, factor seeds and independent API credentials exactly as the existing scrubber does.
R8 reproduction failed before the fix: “pre-upgrade recovery copy retains 1 live sign-ins”. Persistence now has a startup scrubber hook, supplied by normal worker startup and recover-owner before opening or applying state. The existing identity scrubber runs only on the unpublished migration copy; failed scrubbing refuses publication and migration. Added both real identity/copy isolation and scrub-failure migration-gate regressions.

Additional baseline verification gap: the goroutine discipline test finds two unchanged violations outside this lane: `internal/storage/inventory_watch_windows.go:32` and `internal/webhooks/dispatch.go:194`. The recovery goroutine in this lane is corrected; storage/webhooks owners must contain theirs. Restart-parent relaunch/drain tests passed within that combined run. The AST guard itself works; this is new source violating it, not a failed guard.
R3 follow-up during final diff review: listing/pruning also recursively measured every file in every backup on each poll. The list now computes held-file bytes from the manifest plus the database/WAL and manifest metadata, avoiding another whole-directory walk. It still parses the manifest and enumerates backup folders; this is not a claim of constant-time listing of unlimited manual copies.

R1 platform follow-up: reject Windows device names, reserved characters and dot/space normalization aliases within manifests. This is source-backed boundary validation, not a Windows exploit reproduction. [Microsoft naming rules](https://learn.microsoft.com/en-us/windows/win32/fileio/naming-a-file) and [Object Manager whitespace normalization](https://learn.microsoft.com/en-us/troubleshoot/windows-client/shell-experience/file-folder-name-whitespace-characters) establish the aliases; a platform-independent table test covers the Windows helper.

### R9 — The client-log retention setting never removes client reports
Severity: medium. `server/cmd/server/administration.go:48–57`, `main.go:1062–1080`, `server/internal/servicelog/clientlogs.go:214`.
Final settings-consumer tracing found the stored client reports have no age-retention caller. The owner can save client-category log retention, but only the message recorder consumes it; ClientLogStore.Prune is unreachable. Count/byte caps still hold, but an old report can remain forever. Smallest fix: apply the existing client-category days at administration initialization and each five-minute maintenance tick; preserve the daily upload budget for the current day. Add a real initialization fixture with old/current report rows before changing the consumer.

R9 reproduction with client-category retention explicitly saved to 30 days failed before the fix: old=1, current=1, today's budget=100. Administration initialization and each five-minute owner-maintenance tick now consume that category. The configured initialization regression passes with old=0, current=1 and unchanged budget=100. An absent category retains the original bounded history. The first fixture incorrectly assumed a default age limit; corrected it to save the actual setting, then reproduced again before implementation.

The operations settings-consumer guard also reports unchanged `administration.LibrarySettings.monitorChanges` inventory coverage; other selected settings tests passed. The client category consumer description was updated, with no new setting or API shape. Client-core's eight backup transport/restart tests pass.

## Final fixes and verification receipt

| Finding | Status | Regression/evidence |
|---|---|---|
| R1 high | Fixed: supported canonical paths, reserved database/journal refusal, duplicate/platform aliases and regular-file verification; journal recheck. | `TestRestoreManifestCannotReplaceTheLiveDatabaseAsAStateFile`, `TestRestoreNamesAndLinkedManifestFilesAreRefused`, Windows name table |
| R2 high | Fixed: stable terminal completion, recovery metadata retained through publication, fail-closed missing-copy verification, directory durability. | `TestRollbackResultFailureCanResume`, `TestFailedAppliedRestoreWithMissingRecoveryCopyStaysUnresolved`, `TestCompletedRestoreCleanupReplayKeepsItsResultAndNewerState`, existing crash suite selected by name |
| R3 medium | Fixed: both destinations listed/resolved/deleted, local copy cap at completion, list avoids recursive file measurement. | `TestExternalDestinationIncludesLocalRecoveryCopies`, existing list/prune/restore checks |
| R4 medium | Fixed: sync publication before prune/migration, sync pruning. | `TestMigrationWaitsForDurableRecoveryCopyBeforePruning`; source durability trace, not pre-fix power-loss reproduction |
| R5 medium | Fixed: early recovery follows the parent pipe. | `TestEarlyStartupRecoveryStopsWhenItsParentDisappears` |
| R6 medium | Fixed: accepted upload reserves a retention slot. | `TestAcceptedClientReportSurvivesTiedRetention` device/global cases plus owner readback and existing bounds |
| R7 medium | Fixed: different backup kinds never join; retryable 409 documented. | `TestManualBackupCannotJoinAPruneableScheduledCopy`, `TestManualBackupDuringScheduledCopyReturnsRetryableBusy` |
| R8 medium | Fixed: pre-upgrade copy uses existing identity scrub before publication. | `TestMigrationRecoveryCopyHoldsNoLiveSignIns`, `TestFailedMigrationCopyScrubRefusesPublicationAndMigration` |
| R9 medium | Fixed: saved client-category retention applied on initialization and maintenance. | `TestAdministrationInitializationPrunesExpiredClientReports`, including current-day quota preservation |

Final Linux, Windows amd64 and Darwin arm64 builds pass after the last production changes. Windows arm64 also compiled during this review before the final portable validation/retention additions; no physical execution claimed. Final vet on backup, persistence, servicelog, operations, cmd/server and httpapi passes. Targeted tests and fixture results above are observed passes, including eight client-core backup tests. Final apigen regeneration passes and `git diff --check` is clean. The only observed failing broader guards are the three unchanged storage/webhooks/watcher inventory entries already listed; no exemptions were added.

Snapshot **002** is the final cumulative delivery against `1097d54`, superseding checkpoint 001. Subject: “Keep recovery copies durable and preserve requested backups”. The worktree will not be edited after it; this report is written before capture. All nine findings fixed; remaining gaps are named above.

## Merge with the base

Resolved `server/cmd/server/startup.go`: kept the base's `startup.recovery` name for the shared contained shutdown loop, retained our early-recovery `WatchParent` fence, and removed the automatically duplicated import. Both sides' behavior remains. Read the requested lanes' relevant report sections; followed the current networking/identity/API contracts.

Own backup/Restart/state-permission and owner report routes already require admin scope; client upload requires session. Regenerated contracts/OpenAPI with apigen and persistence metadata with schema-sync, without hand-merging generated files.

Passed: Linux, Windows amd64 and macOS arm64 builds; targeted vet; startup/recovery/restore/report tests; router/scope tests; web `npx tsc --noEmit`; complete marker scan and diff check. The prior storage/webhooks goroutine and watcher-consumer guard gaps are resolved on the merged base: their guard tests now pass.

Context refresh still cannot write its lock. No staging, commit or snapshot was attempted. Git's unmerged index remains for Justin to stage and commit; working-file conflicts are resolved.
