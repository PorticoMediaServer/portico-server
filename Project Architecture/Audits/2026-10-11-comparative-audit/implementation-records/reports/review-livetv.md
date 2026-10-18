# Outcome
Completed the independent review and fixed 12 findings: 5 high, 6 medium and 1 low; no blockers. Shared recordings survive guide reconciliation, one owner's cancellation/deletion and restart recovery. Remaining owners attach only validated immutable bytes, with finite retries for temporary finalization failures. DVR metadata and capture now respect profile/account ratings.

Guide parsing has independent token/record/alias memory bounds, and channel mapping uses indexes. Live startup follows byte progress; an abandoned shared opener is cancelled during retirement. Idle callbacks recheck recent progress. Committed background controls are projected even after an HTTP disconnect, and the settings-consumer guard covers the integrated monitoring setting.

Linux build, Windows amd64/macOS arm64 command cross-builds, targeted Go tests/vet, focused race checks, web TypeScript checking, API generation and diff checks passed. No full suite, deployment, push or Git commit. Snapshots 001–002 are milestones; final cumulative snapshot 003 is the integration handoff. The worktree is frozen after that final snapshot.

# Decisions I made
- Work stayed in the named worktree and shared report/snapshot directory on ehler-homelab-mini-1 (personal Ubuntu). No agents/helpers were used. Context-sync could not write its read-only lock; the available canonical Portico/standards/verification guidance was read.
- Keep the cancelled/deleting leader's visible state and private staging namespace. Its existing writer may continue only for a current authorized follower, with exact worker token/generation fencing. The withdrawn owner receives no catalogue publication. Its bytes cannot be cleaned up until pending attachments finish. Crash recovery preserves this arrangement; final immutable readiness is scheduler-stamped.
- Retry shared finalization up to three attempts through durable claims, then fail honestly and release source retention. Authority/cancellation/invalid-input refusals do not gain a retry bypass. Existing ordinary capture recovery limits remain unchanged.
- Use the existing catalogue rating predicate, narrowed by account limits, for DVR as well as guide/playback. Hidden recording metadata becomes the established restricted slot; ID/time/state remain usable for private cancellation/deletion. Rule labels and filters authored by that same profile remain its own input, rather than programme metadata. New restricted commitments and rule anchors are refused; previews/discovery omit forbidden airings; active workers reauthorize.
- Add internal 1 MiB lexical-token and per-record XML bounds, retaining overall owner budgets, depth/node/attribute limits, entity rejection, harmless DOCTYPE support and compressed/expanded limits. Oversized hostile fields/records refuse the document. Ordinary malformed programme rows remain tolerated.
- Keep at most 16 guide aliases per selected channel, deduplicate memberships in constant time and report excess guide matches with a catalogue string. This bounds repeated-name cross products independently of programme/document budgets. Unicode name keys preserve EqualFold behavior and whitespace tolerance; explicit mappings remain authoritative.
- Remove only the redundant live acquisition wall clock. DNS, connection/header/idle-read and byte bounds remain. Retirement cancels an opener before waiting for its readiness, so progressing abandoned work cannot hold a tuner indefinitely.
- The first implementation report's claimed fallback end time is not present: a final XMLTV programme without stop or a following airing is skipped with diagnostics. That conservative behavior is retained rather than inventing an airing length.

# Bugs
These original IDs were reconciled against current code and scoped tests; “already fixed” does not claim fresh physical-device acceptance.

- B-11.1 — already fixed; stream fences exclude guide generation; the fence regression passes.
- B-11.2 — already fixed; harmless DOCTYPE accepted, entities/internal subsets refused; parser regressions pass.
- B-11.3 — already fixed for shorter dates, omitted zone, inferred next stop and legacy encodings; a final unknown end is deliberately skipped.
- B-11.4 — already fixed for ordinary odd rows/overlaps/flags/unknown elements; hostile XML bounds strengthened in F2.
- B-11.5 — fixed residual memory/scale defects in F2/F3; raised owner budgets and the large spool/import fixtures are preserved.
- B-11.6 — already fixed; gzip magic/header paths retain compressed/expanded limits.
- B-11.7 — already fixed; fallback/variant identity, BOM and per-row playlist tolerance remain covered.
- B-11.8 — backend already fixed; guide ID/number/name mapping is preserved and indexed. The current web HDHomeRun creation form still hides its guide URL; screen work remains external.
- B-11.9 — backend consumers already present; retained source filters, numbering, mappings, hidden channels, logos, user agent, horizon, buffer and retry settings. Existing acquisition uses its accepted buffer/retry values until a fresh upstream acquisition; guide refresh alone still does not retune.
- B-11.10 — already fixed; distinct-channel reservations and soft padding/core-window scheduling preserved.
- B-11.11 — upstream/producer retry already present; progressing startup fixed in F4, and shared finalization retry added in F12.
- B-11.12 — fixed residual seal-before-publication recovery gap in F11; shared withdrawn-leader recovery covered in F7.
- B-11.13 — already fixed; unique nearby guide corrections preserve explicit commitments; regression passes.
- B-11.14 — already fixed in source; highest-bandwidth master selection and shared resource acquisition pass local/race fixtures; real-provider/decoder acceptance remains external.
- B-11.15 — already fixed in source; live planning calls the current shared delivery planner/capability model. Android/Fire acceptance remains external.
- B-11.16 — already fixed in source; unknown live facts no longer independently force conversion without an applicable ceiling.
- B-11.17 — already fixed; healthy-output recovery resets retained.
- B-11.18 — already fixed in source; bounded transient segment authority retries retained, explicit refusal remains immediate.
- B-11.19 — already fixed; optional/unknown URI-bearing HLS tags are dropped and supported URLs rewritten; shared HLS race fixture passes.
- B-11.20 — already fixed in source; completeness uses programme core with padding/tolerance bounds.
- B-11.21 — already fixed for the full-walk write lock; measurement is outside it and remains conservative under concurrent writes. Idle shared-inode usage/removal check passes; no saturation capacity claim.
- B-11.22 — already fixed; retention/queued deletion gets its turn before terminal storage refusal; targeted shortage tests pass.
- B-11.23 — already fixed; transport validation no longer computes a discarded hash.
- B-11.24 — already fixed; reliable EP identifiers, not movies/specials/generic IDs, derive series identity.
- B-11.25 — already fixed in source; reliable episode identity deduplicates series discovery; unknown episodes stay distinct.
- B-11.26 — fixed residual follower expiry, rule-fence, leader withdrawal and temporary finalization failures in F1/F6/F7/F12. One-inode independent deletion remains verified.
- B-11.27 — server event/session deadline already present; the current web player has no reservation countdown consumer. Client wiring remains external.
- B-11.28 — descriptor/configured-host enrichment already present; actual LAN hardware acceptance remains external.
- B-11.29 — global hidden/number overlays already present; personal/global hiding remain distinct.
- B-11.30 — live viewing gate already fixed; residual raw DVR rating disclosures/admission fixed in F10.
- B-11.31 — not present in this base's directory query; SQL paging retained. Import-time quadratic mapping is separately fixed in F3.
- B-11.32 — already fixed; five-minute superseded-generation grace and chunked pruning retained.
- B-11.33 — already fixed in source; refresh lease renews every 20 seconds with token/revision fencing.
- B-11.34 — already fixed in shared guide time code; no time-grid code changed here.
- B-11.35 — deliberate bounded timeshift policy, not a defect; actual buffer coverage remains authoritative.
- A-22.37 — already fixed; guide reads remove completed results and share only in-flight work.
- B-09.29 — already fixed; recovery handler/category copy stays reachable and does not disclose the private error; focused handler tests pass.
- B-09.28 — already fixed; scheduled tasks offer backups only; pause/runtime semantics preserved, disconnect projection fixed in F8.
- B-18.11 — fixed residual integrated guard failure in F9; both current stored-setting/preference guards pass. Structural witnesses are not a proof of every platform's setting effect.
- B-04.5 / B-04.11 — owner subtitle track/file bounds already present; the targeted useful-track limit test passes; physical subtitle acceptance remains external.
- B-03.8 / B-03.10 / B-09.9 — existing conversion storage/admission/startup-path consumers retained; no new conversion engine/path work in this review.
- Other cross-domain/native IDs catalogued by the limits implementation report — not independently re-reviewed or changed here; their domain/client lanes retain ownership. This review does not convert those reports into runtime evidence.

# API changes
No route, credential scope, restriction-matrix classification or DTO field was added/removed. Existing registered contracts remain the authority; apigen ran successfully and generated no diff.

Behavior changes: DVR list/detail/overlaps and recording mutation receipts redact forbidden programme/title/series/item facts; scheduling/receipt replay and new rule anchors refuse forbidden programmes; series preview/discovery excludes them. Capture/follower authority checks apply current profile/account ratings. Response structures and client-core parsers stay unchanged. Import diagnostics can report skipped XML channel rows with excess guide aliases, using new catalogue key live.import.guideMatchesLimited.

# Schema changes
No baseline schema change or compatibility migration. The existing checkpoint_json column gains an internal optional sharedReady marker, written only after immutable validation for a withdrawn capture leader. No client accepts or supplies that marker. Existing token/generation/cancellation, allocation and share tables retain authority.

# Owner or viewer settings added or wired
No new user setting. Background pause/resume/speed now finish projecting a committed change after request cancellation, with a bounded independent read context. The integrated monitorChanges boolean (default true) already drives automatic watching; its real ingestion query is now in the consumer guard. Existing screen remains.

# Needed from others
- Web/client lanes: HDHomeRun guide URL/mapping creation controls and reservation countdown/event consumption remain incomplete in the current inspected web paths. Keep private recording ownership and restricted-slot display when rendering DVR results.
- Integrator: take the final cumulative snapshot, including the new regression files and the single i18n catalogue key. No schema sync or client parser change is required.
- Existing limits-lane handoffs remain outside these reviewed core files: ingestion/deep_analysis.go still has a six-hour outer cap, and catalog/visibility_classes.go still lacks cooperative pause boundaries. These were explicitly unfinished in the original limits report; this lane did not restructure their scanning/freshness cores.
- Product/architecture documents still describing independent tuner capture, strict ordinary-row rejection, old line-up limits or unconditionally continuous background work need updating by their owning lanes. The earlier Live TV report's seamless leader-cancellation gap is now closed by F7; its final-stop fallback claim should be corrected to the retained skip behavior.

# Test notes for other environments
Verified on Ubuntu with GOFLAGS=-p=2; no full suites or device builds:
- go build ./...; Windows amd64 and macOS arm64 ./cmd/server cross-builds.
- Focused livechannels parser/security/selection/spool/gzip/sharing/preemption/fence fixtures; DVR shared ownership/reconciliation/withdrawal/recovery/retries/rating/receipt/grant/storage/annotation fixtures; recordingmedia final-checkpoint and inode-removal fixtures.
- Focused administration pause/commit-disconnect/window tests, stored setting/preference guards, monitoring semantics, subtitle limits and analysis storage bounds.
- Console ordinary/reserve/HTTP2 access, startup recovery/category catalogue, production-router overlap, scope, owner pause/usage, upload admission and recording feature-switch tests.
- Focused progressio/shared-input race checks, plus the new DVR withdrawal/retry/active-policy race checks.
- go vet on linearinput, livechannels, livechannels/dvr, recordingmedia, progressio, administration and operations; npx tsc --noEmit -p web/tsconfig.json; apigen; git diff --check.
- Progressing HLS startup fixture took over 30 seconds with one-second byte progress and completed; an abandoned progressing opener physically closed after the grace interval. Earlier baseline failed at the old wall clock.
- Same-host 10,000-channel one-iteration mapping benchmark: 19.302 seconds before; 0.535 seconds after the final Unicode/alias bounds (about 36×). The intermediate indexed-only result was 0.248 seconds. These are local fixtures, not production-scale guarantees.

External acceptance:
1. Use a disposable one-tuner HDHomeRun or private provider fixture. Start two viewers and two profiles recording one airing. Confirm one upstream/channel, separate viewer conversions and uninterrupted remaining consumers when one viewer leaves. Cancel/delete the capture leader mid-programme: only the remaining owner gets a recording, with its earlier bytes intact. Cancel all owners: input retires after grace. Revoking the last remaining recording authority must stop work.
2. Repeat leader withdrawal and restart inside the live window; it must resume the retained prefix for the follower, preserve cancelled/deleting state and publish no item to the withdrawn owner. Inject a crash after recovered bytes seal but before publication: the next start must locate the exact durable object rather than an old prefix. Inject a temporary shared link/probe/database failure: retry; three failed attempts end honestly without retaining a source forever.
3. Lower profile and account ratings after scheduling/while recording. DVR lists, details, overlaps, previews and receipt replays must expose no forbidden title/description/episode/item metadata; new restricted commitments and running capture must be refused. Allowed recordings and private cleanup must still work.
4. Import a real large mixed-encoding/gzip guide. Ordinary odd rows survive with diagnostics; huge fields/records, deep nesting, entities and expanded-size overflow fail within bounds. Repeated display-name aliases produce a bounded notice rather than a cross product. Explicit mappings remain authoritative. Observe old content during checkpoint publication.
5. On Windows/macOS/Linux services, slow NAS/network and actual native/web engines, verify physical retirement, restart recovery, codec/HDR/surround copies and permissions. Cross-compilation is not runtime acceptance. Combine scan/conversion/connection saturation and verify owner login, pause/resume, terminate and restart remain reachable; focused reserve tests do not establish the full capacity target.

# Not finished
No identified review finding remains unfixed. Physical tuners/providers, full simultaneous saturation, slow NAS/GPU/native playback and platform-specific retirement were not exercised. The explicit client and scanning/freshness handoffs above remain with their lanes; no release-wide acceptance claim is made.

# Findings (Phase 1)
The observations below were recorded before their respective implementation changes. Evidence and final resolution follow them.

## F1 — Scheduled reconciliation fails a shared follower during finalization
Severity: high. `server/internal/livechannels/dvr/scheduled.go:90`.
`ExpireMissed` excludes followers whose leader is capturing/finished, but `ReconcileScheduled` independently marks every elapsed scheduled intent failed. A follower remains scheduled with reason shared-capture while its leader records. At programme end, concurrent intent maintenance can mark it failed before the leader seals/publishes. It then cannot acquire the hard link and loses the recording. Smallest fix: preserve a committed shared attachment during normal reconciliation, while still enforcing owner/rule revocation. Baseline reproduction failed as expected; see evidence below.

## F2 — Raised XML document limit permits document-sized token and row allocations
Severity: high. `server/internal/livechannels/parse.go:286,341,629`.
`xml.Decoder.Token` builds whole character-data, comments and attribute tokens before `guideTokens` checks depth/attribute count. `DecodeElement` also accumulates arbitrary repeated title/description elements in a single row. A valid remote guide can now be 1 GiB; one hostile field/row can therefore consume multiples of that memory despite streaming/spooling. Smallest fix: a byte bound before XML token allocation plus a separate per-record bound, retaining total size/depth/entity limits. Baseline reproduction failed as expected; see evidence below.

## F3 — Automatic guide-channel mapping scans every selected channel for every XML channel
Severity: medium. `server/internal/livechannels/parse.go:294`.
The nested channel/name comparison is O(playlist channels × guide channels). At the supported 50,000 defaults that is 2.5 billion channel checks before programme parsing; two upload lanes can do it simultaneously. Smallest fix: build indexes for provider ID, number and normalized display name once, preserving explicit mappings and multi-channel aliases. Reasoned loop trace and a 10,000-channel benchmark: 19.302 seconds before indexing. A semantic fixture also verifies aliases and explicit mapping precedence.

## F4 — Live acquisition still has a fixed 30-second total deadline
Severity: medium. `server/internal/linearinput/gateway.go:158`.
The acquisition timer cancels the gateway context while a bounded HLS manifest is still making progress, contrary to the progress-based timeout claim. `remotemedia` already bounds DNS/dial/headers and idle reads. Removing the timer also requires explicit shared-opener cancellation during retirement so abandoned progressing acquisitions cannot retain a tuner forever. Smallest fix: rely on bounded bytes and idle transport reads, cancel the channel's acquisition before awaiting initialization on retirement. Baseline reproduction failed as expected; see evidence below.

## F5 — A queued idle callback can cancel after new progress
Severity: medium. `server/internal/progressio/progress.go:56`.
An AfterFunc callback may already be running and blocked on Watch.mu. Ping can reset the timer before that old callback acquires the mutex; Reset does not cancel a started callback. The old callback then cancels immediately even though progress just arrived. Smallest fix: check the time since actual last progress under the mutex and reschedule an early callback. TestQueuedExpiryDoesNotCancelNewProgress failed with "queued expiry cancelled newly progressing work".

## F6 — Shared attachment bypasses the unclaimed series-rule fence
Severity: medium. `server/internal/livechannels/dvr/worker.go:278,340`.
`sharedCaptureTx` claims a scheduled follower before the branch checking rule enabled/deleted/revision. A just-disabled or edited rule can therefore attach completed media before rule reconciliation has had its turn. The ordinary new-capture path refuses the same intent. Smallest fix: apply that rule fence before both shared and ordinary unclaimed dispatch, preserving the existing treatment of physically active captures. Baseline reproduction failed as expected; see evidence below.

## F7 — Cancelling or deleting a capture leader cuts the other owner's recording
Severity: high. `server/internal/livechannels/dvr/store.go:285`, `retention.go:98`, `worker.go:430`, `shared.go:56`.
The implementation report acknowledges this gap. Cancellation invalidates the sole writer's claim; the follower detaches and starts a new capture, losing all bytes acquired before cancellation. Active deletion is worse: pending-delete is treated as finished in sharedCaptureTx, so it can attempt to link an unsealed partial and fail permanently. Smallest sound fix: keep the internal capture alive only for still-authorized committed followers, preserve the cancelled/deleting owner's visible state, publish no item to that owner, and seal/link for the remaining owners. Retain this policy through crash recovery and never continue without a recording authority. Recorded before implementation; the baseline reproduction failed as described below.

## F8 — Committed background controls can stay unapplied after request cancellation
Severity: medium. `server/internal/administration/background_policy.go:58,65`.
Pause/resume and speed mutation commit before applying the process projection, then read that projection using the HTTP request context. A disconnect after commit cancels this read; no periodic projection refresh exists. Work can stay running despite a durable pause, or stay paused despite a durable resume, until another maintenance edit/restart. Smallest fix: refresh from committed authority with a short independent context, serialized as before. Recorded before implementation; the baseline reproduction failed as described below.

## F9 — The stored-setting consumer guard fails on the integrated monitoring setting
Severity: low. `server/internal/operations/setting_consumers_test.go:212`.
Targeted verification failed: "stored setting administration.LibrarySettings.monitorChanges has no reviewed runtime consumer". The setting is implemented: ingestion/monitor.go reads the literal monitorChanges in its source query, and TestMonitorSettingDisablesOnlyAutomaticWatching checks its effect. Smallest fix: register that actual reader/effect in the guard, rather than exempt the setting. This is an integration/verification defect, not an inert control.

## F10 — Raw DVR metadata bypasses programme rating restrictions
Severity: high. `server/internal/livechannels/dvr/query.go:147`, `presentation.go:40,86`, `store.go:184`, `worker.go:243`.
Guide slots and playback enforce profile/account ratings, but DVR schedule/list/detail/overlap/rule-preview responses return full programme facts, and scheduling/capture only check the recording grant/source policy. A profile can keep seeing a mature programme after restrictions change or schedule it by known occurrence ID. Smallest fix: reuse the shared profile/account content predicate inside authorized DVR transactions, reduce hidden programmes to the existing restricted slot, refuse new restricted commitments, and fence active capture on policy changes. No new restriction vocabulary or client shape is needed. Recorded before implementation; the baseline reproduction failed as described below.

## F11 — Recovery can seal bytes that have no durable final checkpoint
Severity: high. `server/internal/recordingmedia/driver.go:294-306`.
Normal Capture writes the exact closed object checkpoint before validation/sealing. Recover resumes/appends bytes and then validates/seals without persisting the final checkpoint through CaptureRequest.Checkpoint. A crash after recovery seals but before publication leaves SQLite pointing to an older prefix/digest while the retained stage is gone; the next recovery cannot find either that object or the retained writer. Smallest fix: persist the final retired writer/object checkpoint before recovery validation/sealing, matching the normal capture protocol. Recorded before implementation; the baseline reproduction failed as described below.

## F12 — A transient shared-media failure permanently loses the follower
Severity: medium. `server/internal/livechannels/dvr/shared.go:125`.
runSharedClaim marks every linking/probe/database error terminal on the first attempt, even while the source capture remains safely retained. A temporary probe/IO/database failure can therefore leave the second owner without a recording that already exists. Smallest fix: retry shared finalization a small bounded number of times using the existing durable retry machinery; preserve immediate authority/cancellation refusals and release the source after terminal failure. Recorded before implementation; the baseline reproduction failed as described below.

# Fixes (Phase 2)
- F1 — fixed. Scheduled/series reconciliation preserves an already committed shared attachment through leader finalization. TestSharedFollowerSurvivesEndDuringLeaderFinalization failed for recording/finalizing/completed/incomplete-playable before and passes now.
- F2 — fixed. guideByteReader bounds lexical tokens before decoder allocation; guideTokens bounds each record; mapping aliases have a separate cap. Oversized text/comment/attribute/CDATA and repeated-description tests accepted/read entire fields before and now refuse within bounds. The repeated-name fixture previously accepted the cross product without a notice and now bounds it.
- F3 — fixed. Guide-channel ID/number/name indexes replace the nested full scan; duplicate membership sets replace repeated alias scans. Semantic alias/explicit-map and Unicode fixtures pass. Benchmark evidence is above.
- F4 — fixed. Removed the acquisition total timer; shared retirement cancels its opener before waiting for readiness. TestProgressingManifestCanOutlastThirtySeconds failed at baseline after 33 seconds and passes; abandoned acquisition and grace/rejoin/lifetime race checks pass.
- F5 — fixed. The idle callback checks monotonic elapsed time since last progress under its mutex. TestQueuedExpiryDoesNotCancelNewProgress failed before and passes, including under -race; real idle/overall/parent bounds still pass.
- F6 — fixed. The unclaimed rule enabled/deleted/revision fence runs before either shared attachment or fresh capture. Disabled/deleted/edited follower cases attached media before and are refused/deferred now.
- F7 — fixed. Exact-generation withdrawn leaders continue only for authorized pending followers, preserve withdrawal state, checkpoint/recover privately, signal readiness only after validation and avoid own catalogue publication. Cancellation/deletion renewal failed before. New seal/readiness, remaining-authority, no-authority, follower-only publication and recovery tests pass. Cleanup waits for pending links; existing one-inode unlink behavior passes.
- F8 — fixed. ApplyBackgroundPolicy uses a five-second independent context after committed mutation, retaining serialized durable readback. TestCommittedBackgroundPolicySurvivesCancelledRequest retained Normal before and projects committed Fast now. Existing pause/resume/idempotence tests pass.
- F9 — fixed. Registered the actual monitorChanges literal reader/effect. The stored-settings guard failed before; settings/preferences guards and monitoring semantic fixture pass now. No informational exemption was added.
- F10 — fixed. A shared transactional catalogue/profile/account predicate protects DVR public facts and recording admission/worker authority. The baseline profile/account test disclosed full mature title/description/episode metadata. It passes now, as do cancellation receipt redaction and active rating-policy renewal checks.
- F11 — fixed. Recover persists the exact final retired writer checkpoint before validation/sealing. TestRecoveryPersistsFinalCheckpointBeforeValidation reached decoder validation instead of the refusing checkpoint before; it now stops at that checkpoint and proves the retained stage remains resumable.
- F12 — fixed. Shared finalization uses durable bounded retries; explicit authority/invalid-input failures stay final. TestSharedFinalizationRetriesTransientFailureWithFiniteBudget failed terminally on attempt zero before; now proves three retries and terminal exhaustion.

# Snapshots
- 001: compiling first fixes and pending next regressions; it is an intermediate milestone, not the complete handoff.
- 002: compiling completed shared-capture, rating and recovery fixes.
- 003: final cumulative snapshot after report and verification. Use this snapshot for integration; no worktree changes follow it.
