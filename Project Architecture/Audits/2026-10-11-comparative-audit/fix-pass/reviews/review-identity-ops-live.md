# Independent review — identity-ops-live

Worktree: `/home/justin/projects/Portico/.worktrees/audit-fixes/server-identity-ops-live`.
Base: `82aaa54`; reviewed HEAD: `9592046`. Worktree remained read-only and clean.

Environment: Ubuntu host `ehler-homelab-mini-1`. Context refresh was attempted but blocked by the read-only sandbox at the context-sync lock file; existing project and verification guidance was read. No dependencies downloaded, endpoints contacted, or subagents used.

## Evidence and scope

Read the reviewer and implementation briefs, every commit in `82aaa54..HEAD`, the lane report, both audit passes for each listed bug, the complete decision register, the normative Backups spec, the relevant Hosted-at-Scale migration/claim provisions, and Live TV/DVR contract provisions. Paths below are relative to the reviewed worktree unless stated otherwise. Findings labelled source traces are not claimed as runtime reproductions. No base checkout, extra test source, package installation, workflow dispatch, or image build was performed.

All Go commands used `GOCACHE=/tmp/gocache-codex GOPROXY=off`, from `server/`. The shared package run was:

```
go test ./internal/identity/... ./internal/backup/... ./internal/networking/... ./internal/administration/... ./internal/livechannels/... ./internal/playbackruntime/... ./cmd/server/...
```

All nine reported packages passed, including DVR and library-channel subpackages. `go vet` passed for those packages plus `internal/httpapi/...`. The playback helper built successfully with `go build -o /tmp/portico-review-identity-helper ./cmd/server`. The HTTP suite was run with `PORTICO_PLAYBACK_TEST_HELPER` set to that binary. Both full runs failed; the JSON rerun identified the sole failure as `TestStorageOwnerProjectionReceiptsAndClock`. It also failed alone at `mounts_test.go:81` (400 instead of 202). `/dev/fuse` is absent in this sandbox, so the unchanged `MountSupported` rejects the start request; the fixture and that guard are identical at the base. Focused lane-relevant HTTP tests passed. Full commands and results are recorded below.

### dc36e6d — Expire approved sign-in codes that were never collected so setup clean-up keeps running (bug B-01.5 / B-15.2)

- Verdict: ACCEPT
- What it does: Reads an expired approval's row ID and nullable token hash, revokes a family only when a hash exists, then expires that exact row. This removes the NULL scan failure that rolled back the whole setup-pruning transaction.
- Findings: None. Approval does not issue a family; collection changes the row to consumed and writes its hash together. The existing gated transaction serializes collection and pruning, the 64-row bound is retained, and the update uses the indexed primary key. Existing hashed approvals still go through the original family-revocation path. Unrelated maintenance error logging remains unchanged.
- Tests run: Shared package command above: `internal/identity` passed, including `TestPruneSetupExpiresApprovedCodeThatWasNeverCollected`; package vet passed. The regression checks expiration, clearing approval material, subsequent rate-limit cleanup, preservation of the approver's session, and later row deletion. Reading the parent confirms its NULL-to-string scan fails before these assertions.
- Residual risk: No platform-specific qualification required. Other maintenance errors are still discarded, as at the base commit.

### d23a911 — Let restore staging and on-demand integrity work run past the short request budget (bug B-09.1)

- Verdict: ACCEPT WITH CHANGES
- What it does: Moves exactly restore staging, integrity checking, and cascade-orphan repair onto a two-slot maintenance lane without the ordinary ten-second deadline. Other heavy admin requests retain their deadline and lane.
- Findings:
  1. **Should fix — `server/internal/httpapi/admission.go:87`, interacting with `server/internal/httpapi/connection_context.go:30` and `router.go:1239`: an already-cancelled HTTP/1 request loses its cancellation before entering the newly unbudgeted lane.** `detachCanceledConnection` uses `context.WithoutCancel` because its safety argument relies on lane budgets. A legitimate owner request arriving after an expired keep-alive read deadline, or after disconnect before handler entry, can therefore run restore/repair with neither a request deadline nor any future disconnect cancellation. Capacity limits concurrency but cannot reclaim those slots. Preserve or refuse cancelled contexts for these long routes, or give the detached path a separate explicit bound; extend the existing cancelled-connection test through a long owner route and assert termination and slot release. This is a source-confirmed interaction; no long abandoned restore was run.
- Tests run: `TestLongOwnerOperationsAreNotCutOffByTheRequestBudget` is included in the HTTP suite; package vet passed. Its fake mux proves the route mapping, context class, and deadlines, but bypasses the outer production router and cannot catch finding 1. HTTP suite results are recorded below.
- Residual risk: Real multi-gigabyte copy/integrity work and cancellation during filesystem IO remain unqualified. `Stage`'s existing copies and `stageMu.Lock` do not observe context; cancellation is noticed only when subsequent database work runs. The report's statement that all operations end when the owner's request goes away is therefore too strong even apart from finding 1.
- Security trace: The typed restore route has `Access: apikit.Owner`; `apikit.Registry.admit` invokes `v1Authorize` before reading the body or calling `Stage`. Both raw integrity routes call `d.owner` before their work. Owner authority rechecks the live family/account and rejects non-owners; neither a role string nor the fairness credential cache grants access. Missing credentials return before any body read, and supplied invalid/non-owner credentials encounter bounded read snapshots (default 30 seconds when no request deadline exists). No unauthenticated or non-owner path to the long handler itself was found. Denied requests can briefly enter the two-slot lane because admission precedes authorization; this is not proof of an indefinite unauthorized slot hold.

### 389194a — Verify backup manifests by streaming so a large database is never held in memory (bug B-09.16)

- Verdict: ACCEPT
- What it does: Streams each manifest file into SHA-256 and compares both the digest and bytes read, instead of allocating a slice the size of the file.
- Findings: None. Existing manifest-name checks and error mapping remain intact; every successful open is closed on both read-success and read-error paths. The database is included in pre-restore manifests, so this is the production path that previously allocated database-sized memory. It does not change the backup format or relax integrity validation.
- Tests run: Shared package command: `internal/backup` passed, including `TestManifestVerificationDoesNotHoldWholeFilesInMemory`; vet passed. Its 96 MiB sparse-file allocation threshold distinguishes streaming from the parent's whole-file read and checks wrong hash, wrong length, and missing file refusal.
- Residual risk: A database larger than physical RAM was not restored end to end. Filesystem cancellation and special-file handling are existing concerns outside this narrow allocation fix.

### 168165c — Roll back a restored database whose server identity keys are not in the state folder (bug B-09.35)

- Verdict: REJECT
- What it does: Adds a read-only key-registry check to the startup restore callback, before the restore marker is finalized. A foreign bare database whose signing-key incarnation is absent now triggers rollback rather than being kept and failing later startup.
- Findings:
  1. **Blocker — `server/cmd/server/restore_verification.go:29–32`: the callback still finalizes restores before checking the other mandatory networking identity prerequisites.** A backup folder can carry the correct database and signing keys plus an invalid `supervisor/authority-incarnation.v1` (for example 64 `z` bytes, with the manifest recording those bytes' correct size/hash). `Stage` checks hashes and copies that file, and this callback checks only the database and signing keys, so `finishApplied` records `restored` and removes the staged marker. Startup subsequently calls `openCurrentAuthority` (`networking_initializer.go:33–94`), whose `NewAuthorityGate` rejects anything except 64 lowercase hex characters; the server then fails every start, outside rollback. Independently, removing a mandatory networking identity/claim trigger while retaining the current migration ledger and valid identity rows passes these two checks but fails `persistence.VerifyNetworkingClaims` at `main.go:356` after finalization. Neither is a missing feature or permission to adopt an identity; both are existing startup acceptance rules. Run a **read-only** authority-file validator and `persistence.VerifyNetworkingClaims` inside the callback before returning success. Add folder-restore regressions for invalid authority bytes and a missing claim/identity schema object; assert rollback restores the previous database and files and normal startup prerequisites pass. These are deterministic source traces, not executed reproductions.
- Tests run: Shared command: `cmd/server`, `internal/networking`, and `internal/backup` passed; their vet checks passed. Existing foreign-bare, same-server-bare, read-only, and crash/rollback tests exercise the intended key mismatch and restore machinery, but the new tests omit folder-restored authority/schema rejection.
- Residual risk: A real installed-server restart is unqualified. The previous state remains in the pre-restore safety copy in the failure described above, but recovery requires manual work because the automatic marker has been removed.
- Trust-code trace: `holdsIdentityTx` and `unmarkedStateTx` extract the old checks without weakening them: application ID/current identity equality, validated public-key-derived server ID and claim state, exact incarnation file, fixed-size key bytes, seed-derived private-key consistency, and public-key equality remain enforced. `OpenCurrentKeys` still uses its authority runner and gated transaction. `VerifyStateKeys` only opens a snapshot/root and reads, never creates keys or rewrites identity; empty unmarked state is accepted only with no accounts, durable identities, legacy key material, or registry entries. Deleting/changing just the candidate ID or incarnation does not evade the key check. The rejection concerns the incomplete startup boundary, not a weakened extracted helper.

### dd7bf3c — Publish the update manifest in the shape the server's update check reads (bug B-09.11 / B-25.1)

- Verdict: ACCEPT
- What it does: Adds the reader's `channels.stable` build list to the existing release document, pointing the owner at the version's release page.
- Findings: None. The reader, version ordering, HTTPS URL filtering, fetch bounds, existing asset fields, and disabled automatic installation remain unchanged. It does not pick a default feed URL or pre-empt D-57. The added integration test executes the actual writer rather than duplicating its JSON shape in a fixture.
- Tests run: Shared command: `internal/administration` passed, including `TestUpdatesReadTheManifestTheReleasePipelinePublishes` with Node available; vet and `node --check scripts/write-public-update-manifest.mjs` passed. The test checks newer/current builds, release URL, and `AutoInstall=false`.
- Residual risk: No published asset or GitHub redirect chain was fetched. Feed discovery/documentation remains a separate task.

### 852a2ea — Start the Linux service on install and keep it running across package upgrades (bug B-09.3 / B-25.20)

- Verdict: ACCEPT WITH CHANGES
- What it does: Distinguishes Debian lifecycle arguments and RPM package counts, starts fresh installations, and restarts an already-running server on upgrades without disabling it in the old package's removal hook.
- Findings:
  1. **Should fix — `packaging/linux/postinstall.sh:27,31`: Debian start/restart bypasses the administrator's `policy-rc.d`.** `/run/systemd/system` detects a live systemd instance but does not authorize starting services. A Debian host with a policy helper returning 101 still gets a direct `systemctl start` or `try-restart`. The locally installed `/usr/bin/deb-systemd-invoke` explicitly consults that helper before calling systemctl; the new start path does not. Use the Debian policy-aware service helper on Debian lifecycle branches, preserving the current inactive-service upgrade behavior, and keep RPM handling separate. Add a qualification case with `policy-rc.d` returning 101 and assert no service starts/restarts. The lane acknowledges this omission; it should be corrected rather than treated solely as an environment limitation.
- Tests run: `sh -n` and `dash -n` on both scripts passed. Read `nfpm.yaml` wiring, Debian configure/remove/upgrade/abort argument branches, and RPM `%post` 1/2 and `%preun` 0/1 ordering. No DEB/RPM was generated or installed. Ordinary new-to-new upgrade branches are correct: old pre-removal leaves the running service alone; the new post-install try-restarts it and leaves enablement unchanged.
- Residual risk: Qualify first install, running upgrade, stopped upgrade, disabled upgrade, removal, and policy-denied install on disposable Debian and RPM VMs. The lane's detailed steps are usable, but restore the older package explicitly before repeating the stopped-upgrade case; the suggested arbitrary tiny payload must actually provide a long-running service to establish `is-active`. Upgrades from the base commit's old unconditional removal scripts remain broken as the report states; no published old release was established here. Debian abort-remove also cannot reconstruct pre-removal disabled/stopped state without a recorded prior state.

### de01fef — Write the Windows portable archive to the requested output folder (bug B-25.3)

- Verdict: ACCEPT
- What it does: Resolves the Windows output directory before changing into the payload and passes the resulting absolute payload/output paths to both ZIP and NSIS.
- Findings: None. Inputs are copied before the directory change, `OUT` already exists, quoted paths preserve spaces, and archive contents remain rooted at the payload. NSIS's `OutFile` and recursive `File` directives now receive usable absolute paths even when it changes its working directory to the installer script. Linux/macOS branches are untouched.
- Tests run: `bash -n scripts/package-release.sh` passed. Read the exact workflow call using relative `dist` and the NSIS `OUTPUT_FILE`/`STAGE_DIR` consumers. No fresh packaging run was performed by this reviewer; the lane's ZIP/stub evidence was reviewed, not counted as my execution.
- Residual risk: The report provides a specific real-NSIS release-runner smoke for both x64/arm64 and the upload glob. Actual installation/start on Windows remains unqualified and has unrelated known lifecycle issues.

### e1d5a13 — Run the release test suite on the tag being published (bug B-25.19)

- Verdict: ACCEPT
- What it does: Gives reusable CI an optional checkout ref and passes the release's selected tag into all three CI checkouts.
- Findings: None for this scoped mismatch. The input is declared as a string under `workflow_call`, the caller supplies it at the reusable-job level, and every repository/server/client checkout consumes it. An empty input leaves checkout's normal push/PR selection intact. The local reusable workflow definition still comes from the caller revision, which is an existing GitHub Actions constraint rather than a source checkout mismatch introduced here.
- Tests run: PyYAML parsed both workflows; inspected the input, caller expression, dependency/output chain, and all three checkout entries. No `actionlint` is installed; no remote workflow was run.
- Residual risk: The report's dispatch-from-B/tag-at-A and push/PR smoke is specific and useful. Tag names remain mutable/ambiguous throughout the pre-existing release pipeline; resolving `refs/tags/<name>^{commit}` once and passing its immutable SHA to every job would additionally close that race. This commit aligns CI with the existing packaging selection; it does not establish immutable artifact/check identity.

### d1da321 — Give the container release build the qualified FFmpeg bundle for each architecture (bug B-25.4)

- Verdict: ACCEPT
- What it does: Downloads, hashes, and checks the target of both Linux FFmpeg bundles in the Docker job, then selects the matching `TARGETOS-TARGETARCH` directory in a build stage. Flat native local bundles remain supported.
- Findings: None. The download names match the existing native jobs/toolchain publish naming. The workflow uses explicit path context (`context: .`), so pre-build generated files are included; `.dockerignore` does not exclude them. Automatic target args are declared in the selection stage, the final copy contains only the selected bundle, and full in-image verification remains in place. A missing or wrong architecture cannot silently skip hash/execution verification.
- Tests run: Workflow YAML parse and `bash -n` on the extracted download step passed. Read the Docker stages, build-push inputs, `.dockerignore`, toolchain naming, and both verifiers. No build/image pull/download was performed by this reviewer.
- Residual risk: Both real architectures still require the report's buildx/QEMU and fork-release qualification. Native archive layout, availability, executable permissions, and full arm64 verification speed remain runtime assumptions. The report's local stand-in-stage evidence demonstrates selection only, not a qualified release image.

### f25712a — Accept the standard XMLTV document type declaration while refusing every entity declaration (bug B-11.2)

- Verdict: ACCEPT
- What it does: Allows one tightly parsed XMLTV DOCTYPE before the root, while retaining whole-document preflight and rejecting subsets, other declarations, duplicates, and misplaced directives.
- Findings: None. Both production decoding passes use strict `encoding/xml.NewDecoder` defaults with no custom `Entity` map. The helper cannot accept `[`/`]`, `<`/`>`, parameter-reference `%`, or entity-reference `&` in its quoted literals, and trailing declaration material fails the final empty-rest check. Preflight still visits nested content that the subsequent parser may skip. External DTD names are inert strings: the Go decoder has no DTD loader.
- Tests run: Shared command: `internal/livechannels` passed, including `TestGuideAcceptsOnlyTheHarmlessDocumentTypeDeclaration` and `TestParserRejectsHostileAndAmbiguousData`; vet passed. Existing cases try internal/external/parameter entity declarations, subsets after SYSTEM, nested directives, and `&x;` under a harmless DOCTYPE. Additional constructed source-review probes were `<!DOCTYPE tv PUBLIC "id" "file:///tmp/portico-entity.dtd"><tv><channel id="&x;"/></tv>` and a DOCTYPE containing an ENTITY hidden in an internal-subset comment: the first reaches strict tokenization's undeclared-entity refusal without loading the file; the second cannot pass the helper's grammar. No custom-entity expansion path was found. Built-in XML escapes and numeric character references remain ordinary supported XML, not DTD entity expansion. These extra probes were reasoned against the decoder/helper, not executed as new tests.
- Residual risk: A real provider's guide compatibility is unqualified. Existing total byte/node/depth/attribute bounds and all-or-nothing row policy remain; this does not decide D-02.

### ce1801a — Reset live channel recovery after a sustained run of healthy output (bug B-11.17)

- Verdict: ACCEPT WITH CHANGES
- What it does: Resets failure counting after the completed producer appears to have added a minute of media, while retaining the five-failure cap and backoff for short runs.
- Findings:
  1. **Should fix — `server/internal/playbackruntime/linear.go:449–459`: retained media duration is not produced media duration.** Once buffer eviction advances `after.StartUS`, the calculation discards everything the healthy producer emitted before that retained edge. `ConfigureChannels` allows retention as short as 12 seconds (`linear.go:156`), and `linearbuffer.Buffer` evicts by both time and byte cap (`buffer.go:284–293`). With a 12-second rolling window, a producer can emit healthy media for hours but the helper sees only roughly 12 seconds and increments the lifetime failure count; five isolated hiccups still exhaust recovery. Byte-limited high-bitrate buffers can fail similarly. For a concrete helper vector, attempts=4, before=[0,12s], after=[588s,600s], no gaps returns 5 although the same producer added hundreds of seconds. Track cumulative accepted media for the current producer independently of retained window eviction, or establish/reset health while output is being published. Test both retention and byte eviction after more than a minute of output, with gaps and earlier producers excluded.
- Tests run: Shared command: `internal/playbackruntime` passed, including `TestLinearRecoveryBudgetResetsAfterSustainedOutput`; vet passed. Its long-run vectors keep `StartUS=0`, so they cannot catch eviction. The producer-loop wiring was traced through `produceMonitored`, publication, and the buffer's mutex-protected `Window` and eviction.
- Residual risk: No real flaky tuner/stream or session-loop recovery harness was run. A one-minute health threshold is a reasonable local retry heuristic, not a new owner-facing product policy, but its measurement must survive supported buffer limits.

### 9592046 — Let a recording that comes due on a full disk wait for queued deletions before failing (bug B-11.22)

- Verdict: ACCEPT WITH CHANGES
- What it does: Defers a storage floor/cap failure on first observation and while deletion work exists, preserving the capture window and retention/Keep/reader safeguards.
- Findings:
  1. **Should fix — `server/internal/livechannels/dvr/worker.go:340–350`, interacting with `capturePass:94–119`: the timer does not guarantee retention gets a turn before terminal failure.** The first shortage sets `next_attempt_ms=now+3s` via `later`. A capture pass can make eight claims with a separate one-second context for each, then runs retention. With several due recordings and slow successful storage/DB checks, the first, highest-priority recording becomes due again after three seconds inside that same loop. Its reason already equals the shortage reason and no deletion has been queued yet, so the new code terminally fails it before reaching `RetentionOne`. Thus the original defect remains on a busy/slow-storage production path. Run eligible retention/deletion before deciding terminal storage rejection, or explicitly distinguish a completed maintenance opportunity from elapsed retry time. Add a full `capturePass` regression with multiple due recordings, advancing the fake clock during checks, and an eligible retention candidate; assert the first recording survives until maintenance and can start once space is freed.
- Tests run: Shared command: `internal/livechannels/dvr` passed, including `TestStorageShortageWaitsForQueuedDeletionsBeforeFailingARecording`; vet passed. The new test calls `claimNext` directly and manually inserts/removes deletion rows, so it cannot prove ordering against the production eight-claim loop. It also never turns the fake storage check healthy and proves an eventual successful claim; add that positive continuation assertion.
- Residual risk: Real full-disk capture, deletion IO, and late partial recording remain unqualified. The EXISTS probe is constant-result bounded work, introduces no schema changes, and does not override Keep or active-reader protection. Waiting while any deletion exists may last until this recording's window ends if that deletion is blocked; this remains bounded and preferable to unsafe emergency deletion.

## Skipped items

### B-01.8 — Hosted attachment flag

**Skip justified.** The flag still reads retired `policy` storage (`identity/pin_recovery.go:57`), while the current claim path removes it and the release build excludes its development writer. `Capabilities` and the bundled sign-in/invitation pages consume the result. Onboarding mode and acknowledged claim installation are different facts: `SetupClaimInstalledTx` requires current installed intent and acknowledgement, whereas the onboarding auth mode can already be Hosted before that point. Picking either for a flag that also gates recovery methods changes client/auth behavior. Justin should define attachment for pending claims and a direct server claimed later, then the implementation can replace the retired-table read with the chosen current projection and test both cases. Imported legacy rows may still contain policy.

### B-11.1 — Guide refresh retunes viewers

**Skip is not justified by the technical premises given; return it for a fresh bounded-fix assessment.** The retune remains real: the resolver stamps active generation/source revision, and both session refresh and producer checks compare them. However, `livechannels/store.go:88–92` implements `seal` as `[]byte(v)`, and `livechannels/remote.go:87–92` implements `unseal` as a bounded UTF-8 conversion; both ignore the AAD argument. `locator_sealed` is already stored plaintext, so there is comparable stream identity and no cryptographic unseal cost per 500 ms tick. A new database column/migration is not a prerequisite to comparing or hashing the indexed channel's bounded locator and relevant input approvals/configuration in the existing resolver read.

The rest of the lane's caution is sound: source revision also fences real configuration changes, `InputTx` enforces active generation, and the session/runtime keys must change together with a test proving guide-only refresh preserves the producer but changed URL/approvals/disabled access still fence it. Those are implementation/verification concerns, not evidence that D-42's shared input/household DVR choices must be decided first. Correct the report's obsolete encryption assertion and assess the modest per-selection input-fence approach on current code before declaring this too large. This reviewer did not implement that fix or claim its runtime safety; a deliberate short deferral for that assessment is preferable to a rushed merge.

## Cross-commit issues

- B-09.1 and the base router's cancelled-connection recovery interact as described in d23a911 finding 1. Recheck that path when changing admission/auth middleware; the fake-mux deadline test alone cannot establish production cancellation.
- Streaming manifests and the long restore lane fix independent resource constraints. The new key callback closes the foreign-bare-key case, but the unprotected authority/schema prerequisites remain a merge blocker for B-09.35. Keep the extracted helpers and add the missing read-only checks; do not solve it by adopting missing identities or creating authority material from a restored database.
- Recovery reset and the B-11.1 skip are distinct. A guide retune still resets the selection key/count; it must not mask failed healthy-run measurement in tests that use one fixed generation.
- Other available audit-fix branches were checked only for overlapping paths. `audit-fixes/platform-packaging` also modifies `.github/workflows/release.yml`, `scripts/package-release.sh`, and `server/internal/backup/backup_test.go`. Reconcile these changes and rerun script/YAML/backup checks on the integrated tree; this is an overlap warning, not a review of that lane's commits. No other available branch shares a changed path with this lane.
- Commits have no schema changes, new dependencies, native package-copy edits, or unrelated source changes. Commit messages are plain behavior sentences; authors are Justin Ehler without generated/coauthor trailers. Added Go files are gofmt-clean, diff whitespace checks pass, and added goroutines do not bypass `supervise.Go`.

## Final verification

All Go commands below used `GOCACHE=/tmp/gocache-codex GOPROXY=off` in the worktree's `server/` directory.

- `go test ./internal/identity/... ./internal/backup/... ./internal/networking/... ./internal/administration/... ./internal/livechannels/... ./internal/playbackruntime/... ./cmd/server/...` — PASS for all nine reported packages.
- `go build -o /tmp/portico-review-identity-helper ./cmd/server` — PASS.
- `PORTICO_PLAYBACK_TEST_HELPER=/tmp/portico-review-identity-helper go test ./internal/httpapi/...` — FAIL. Retried the full suite with `-json`: only `TestStorageOwnerProjectionReceiptsAndClock` failed; all other test cases and the fixture subpackage passed. `go test ./internal/httpapi -run '^TestStorageOwnerProjectionReceiptsAndClock$' -count=1` reproduced the same 400 at line 81. Source inspection establishes the environment reason: `/dev/fuse` is missing; the unchanged `MountSupported` guard rejects a mount start which this base fixture unconditionally expects to succeed. This is not attributed to the lane, and the complete HTTP suite is not reported as passing.
- `PORTICO_PLAYBACK_TEST_HELPER=/tmp/portico-review-identity-helper go test ./internal/httpapi -run 'Test(LongOwnerOperationsAreNotCutOffByTheRequestBudget|BackupRoutesOwnerBoundaryAndLifecycle|HostedOwner|AdmissionQueue|CanceledConnectionIsServedAndClosed|EveryRegisteredRouteHasALane|LaneSpecsAreCoherent)' -count=1` — PASS (2.260s).
- `go test ./internal/livechannels -run 'Test(GuideAcceptsOnlyTheHarmlessDocumentTypeDeclaration|ParserRejectsHostileAndAmbiguousData)$' -count=1` — PASS (uncached focused entity/DOCTYPE check).
- `go vet ./internal/identity/... ./internal/httpapi/... ./internal/backup/... ./internal/networking/... ./internal/administration/... ./internal/livechannels/... ./internal/playbackruntime/... ./cmd/server/...` — PASS.
- `go build -tags release -o /dev/null ./cmd/server` — PASS.
- `node --check scripts/write-public-update-manifest.mjs`; `sh -n` and `dash -n` on both Linux maintainer scripts; `bash -n scripts/package-release.sh`; PyYAML parse of both changed workflows; `bash -n` on the extracted Docker bundle-download step — PASS.
- `git diff --check 82aaa54..HEAD` and `gofmt -l` over every changed Go file — clean. `git status --short` — clean before and after review.

No platform installations, external services, dependency downloads, or real media/hardware qualification were performed. Regressions recommended in findings were not added to the read-only worktree.

## Summary

| Commit | Bug | Verdict | Blockers |
|---|---|---|---|
| dc36e6d | B-01.5 / B-15.2 | ACCEPT | 0 |
| d23a911 | B-09.1 | ACCEPT WITH CHANGES | 0 |
| 389194a | B-09.16 | ACCEPT | 0 |
| 168165c | B-09.35 | REJECT | 1: missing authority/schema checks before restore finalization |
| dd7bf3c | B-09.11 / B-25.1 | ACCEPT | 0 |
| 852a2ea | B-09.3 / B-25.20 | ACCEPT WITH CHANGES | 0 |
| de01fef | B-25.3 | ACCEPT | 0 |
| e1d5a13 | B-25.19 | ACCEPT | 0 |
| d1da321 | B-25.4 | ACCEPT | 0 |
| f25712a | B-11.2 | ACCEPT | 0 |
| ce1801a | B-11.17 | ACCEPT WITH CHANGES | 0 |
| 9592046 | B-11.22 | ACCEPT WITH CHANGES | 0 |

Totals: **7 ACCEPT, 4 ACCEPT WITH CHANGES, 1 REJECT; 1 Blocker and 4 Should fix findings.** B-01.8 needs Justin's attachment definition. B-11.1 needs reassessment against plaintext source storage, not an automatic dependency on D-42.
