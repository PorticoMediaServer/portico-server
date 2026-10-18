# Second-round independent server review

Scope: only the follow-up commits requested by Justin. Verdicts use RESOLVED / RESOLVED WITH NITS / NOT RESOLVED / NEW PROBLEM. Worktrees remain read-only. Host: ehler-homelab-mini-1. Context refresh failed at the sandbox-protected context-sync lock; existing guidance is used. Go commands use GOCACHE=/tmp/gocache-codex GOPROXY=off; no dependencies, endpoints, browser, or subagents.

Completed review of 22 requested follow-ups. Ordinary suites pass; four independent adversarial probes fail. **17 RESOLVED / 1 RESOLVED WITH NITS / 4 NOT RESOLVED / 0 NEW PROBLEM.** Four blocker findings across three commits, plus one should-fix finding and one testing nit.

## Identity, operations and live TV

Paths in this section are relative to `server-identity-ops-live`.

### bf149c9 — Roll back a restore whose authority file or identity schema the server could not start on (bug B-09.35)
- Verdict: NOT RESOLVED
- What it does: Reuses the exact authority-file reader and checks networking claim schema before finalizing a restore, alongside the existing signing-key check. This closes both specific identity examples in the first review.
- Findings:
  1. **Blocker — `server/cmd/server/restore_verification.go:45`, interacting with `server/cmd/server/main.go:398`: successful identity verification is still not sufficient for mandatory startup.** A staged folder backup with its `maintenance_policy` singleton absent passes migrations, quick_check, authority/claim-schema/signing-key checks and is recorded `restored`; the later mandatory background-priority query returns `sql: no rows in result set` and startup exits every time after the marker is gone. Independently reproduced using real Stage/ApplyStaged and the exact main.go query; the database and authority files are otherwise the valid started-state fixture. This is the requested additional startup-prerequisite search, not a new behavior caused by the follow-up. Validate mandatory restored-state singleton/configuration prerequisites (or make their normal idempotent initialization happen within the rollback boundary) before finalization; add missing-row/invalid-required-configuration regressions and run the complete startup prerequisite path on the accepted candidate. Do not describe the three identity steps as all fatal startup readers.
- Tests run: `go test ./cmd/server/... ./internal/networking/... ./internal/backup/...` — PASS. Four rejection regressions exercise real folder staging/apply/rollback and normal identity initialization on the recovered state; the valid-folder test passes. Existing foreign/same-server and read-only tests also pass. Anonymous-overlay `TestReviewerRestoreCannotFinalizeBeforeMandatoryStartupRows` — FAIL: restore outcome `restored`, then the mandatory startup query fails with no rows. This is an executed restore plus production-query reproduction, not a full server startup.
- Residual risk: Installed-server restart and filesystem failures remain unqualified; database acceptance is not a guarantee that every startup initializer succeeds.

### e0f8155 — Refuse a long owner operation on a canceled connection instead of running it with nothing to end it (bug B-09.1)
- Verdict: RESOLVED
- What it does: Marks HTTP/1 requests whose cancellation was detached and refuses them before acquiring an unbudgeted owner-lane slot. Budgeted routes retain their existing behavior.
- Findings: None. The outer production router creates the marker; all three long routes select the flagged lane. A connected request retains cancellation, HTTP/2 remains untouched, and refusal closes the poisoned connection and supplies retry guidance. No authority check was relaxed.
- Tests run: `go test ./internal/httpapi -run 'TestLongOwner|TestUnbudgeted|TestCanceledConnection' -count=1` — PASS. Includes real keep-alive poisoning, all three production routes, fresh-connection retry, refusal before admission and unchanged budgeted handling.
- Residual risk: Existing restore file copies and mutex waiting remain non-cancellable until later database work. The revised comment/report accurately limits the claim; this commit resolves detached cancellation, not that separate IO limitation.

### 35a0599 — Ask Debian's service policy before a package starts or restarts the server (bug B-09.3 / B-25.20)
- Verdict: RESOLVED
- What it does: Routes Debian start/restart through `deb-systemd-invoke`; checks active state before upgrading so an inactive service is not started. RPM retains direct systemctl actions.
- Findings: None for the reported Debian policy bypass on normal installations. Lifecycle classification and helper arguments match the installed helper; enablement remains separate from activation. The fallback when the Debian helper is absent still uses systemctl; qualification should include the package's declared/runtime helper availability.
- Tests run: Shell syntax and policy qualification results are recorded in the final verification section. No package installed.
- Residual risk: Real dpkg/RPM lifecycle, abort paths, and upgrade from older unconditional removal scripts require the lane's disposable-VM cases. Its revised note specifies a real long-running payload and clean older-version setup for each case.

### c89d573 — Count a live producer's output as it is appended so buffer eviction cannot hide a healthy run (bug B-11.17)
- Verdict: RESOLVED
- What it does: Measures each producer's appended-media duration using a cumulative mutex-protected counter instead of the surviving buffer window. Evicted output contributes; reconnect gaps and prior producers do not.
- Findings: None. The counter advances only on accepted append, and the runtime takes a difference around one monitored producer. It preserves the five-failure limit and resets only after the existing one-minute healthy threshold. Retention/byte-cap eviction cannot reduce the measurement.
- Tests run: `go test ./internal/linearbuffer/... ./internal/playbackruntime/...` — PASS, including short-retention/small-byte-cap eviction and threshold/repeated-failure cases.
- Residual risk: Real upstream interruptions and encoded segment cadence were not exercised; the deterministic buffer test proves the eviction defect is closed.

### 47ca090 — Run retention before a recording on a full disk is judged, however long the pass takes (bug B-11.22)
- Verdict: RESOLVED
- What it does: Runs eligible retention before taking the capture transaction when storage is short, then waits while deletions are queued or retention could not complete. It removes the first-observation timing assumption.
- Findings: None. No nested write-gate acquisition: retention runs before the protected-capture transaction. Existing retention policy, Keep/reader protection, deletion grace, bounded work and recording end-window checks remain. No emergency deletion or new retention policy is introduced.
- Tests run: `go test ./internal/livechannels/...` — PASS. The slow-volume/multiple-due-recordings regression lets a pass outlast the retry interval, verifies retention queues first, deletion releases space, and a later claim starts; the queued-deletion/no-space/end-window cases pass.
- Residual risk: Real full-disk unlink latency and capture startup remain hardware qualification. Retention errors keep a recording waiting only within its existing time window.

### f698dab — Keep live viewers on their stream when a guide refresh leaves it unchanged (bug B-11.1)
- Verdict: RESOLVED
- What it does: Separates live stream identity from guide generation: the fence hashes the channel locator and owner network approvals and includes tuner count. Refresh still updates the session selection/programme without retiring an unchanged producer.
- Findings: None in the traced production path. `ResolveTx` still calls `liveTVAllowed` and current access policy; `liveSelection` joins the active generation and refuses removed channels, disabled sources and narrowed viewer access. `CheckLinearProducer` resolves afresh, compares the fence and existing policy/producer revisions; a locator, approvals or tuner-count change stops the old producer. Media authorization still resolves current policy for retained selections. Library-channel schedule/asset rules are unchanged. An unreadable input falls back to revision, causing conservative retuning rather than retaining an unreadable source indefinitely.
- Tests run: Livechannels, playbackruntime package suites above — PASS. Input/fence tests exercise guide-only refresh, a single changed locator, tuner changes and unreadable-input fallback; playback's pure retune test is included in the focused verification below.
- Residual risk: No media-tier tuner run. The lane supplies a usable cold test for unchanged guide, real URL change, disabled source and tuner exhaustion. Permission withdrawal/removal traces are source evidence, not executed end-to-end interruption tests.

## Scanning and metadata

Paths in this section are relative to `server-scanning-metadata`.

### f1e4f04 — Retire a queued automatic provider image when its slot holds local or locked artwork (bug B-06.3)
- Verdict: RESOLVED
- What it does: Checks local/locked selection precedence at claim and again at publication, retiring incompatible automatic jobs as stale. Explicit owner jobs, local jobs and previews retain their existing intent.
- Findings: None. The comparison uses the resolved integer entity, exact kind/role/subject and existing selection/candidate joins. The second check shares the publication transaction and closes the during-download race; the existing lease, fence, revision and enabled-provider checks remain. An inherited job with the same revision can no longer replace an unlocked local poster.
- Tests run: `go test ./internal/metadata/... -count=1` — PASS, including `TestQueuedProviderArtworkGivesWayToALocalOrLockedSelection`, inherited automatic jobs and explicit owner replacement. Vet PASS.
- Residual risk: Real artwork transport not used. A stale result acquired during a race may leave installed content-addressed files for normal lifecycle cleanup; it is never selected.

### c951cab — Recover a parked title only when its recorded reason is an outage, not on a retry time alone (bug B-06.12)
- Verdict: RESOLVED
- What it does: Requires both a due retry timestamp and an explicit outage-only reason. New retryable HTTP refusals use an unambiguous provider-failure code; old `request_rejected` rows remain terminal because their status cannot be recovered.
- Findings: None. Old 401/403/404 and local/validation failures cannot enter the recovery query, while known old network/backoff rows can. Ordinary due work is selected first, attempts/backoff remain bounded per turn, and provider cooldown/pacing still applies. Ambiguous old 429/5xx rows requiring owner retry is the correct conservative tradeoff without migration.
- Tests run: Metadata suite — PASS, including nine pre-upgrade fixtures over three days and new network/429/503 versus terminal classification cases. Vet PASS.
- Residual risk: No live provider outage. Sparse indefinite recovery remains the already-disclosed policy, not an aggregate provider request budget.

### ea56bd6 — Trust a scan's folder listing for sidecars only while the folder is unchanged (bug B-05.22)
- Verdict: RESOLVED
- What it does: Stores listing identity/revision, rechecks each consulted directory once per analyzed file and forgets changed/unreadable listings. Finished/cancelled/failed jobs release their cached listings.
- Findings: None for the demonstrated pre-analysis late-sidecar defect. The real inventory-directory seam checks root identity/symlink policy and directory evidence; parent artwork directories get separate checks. Missing evidence, partial listings, restarts and failures fall back to ordinary reads. Locks are released before helper IO; cached name maps are not mutated after completion. The once-per-file check is consistent with the first review's once-per-turn recommendation.
- Tests run: `go test ./internal/ingestion/... ./internal/localmetadata/... -count=1` — PASS, including the real-MP4 late-poster regression, second ordinary scan, changed-neighbor/absent-revision and cancellation cleanup. Vet PASS.
- Residual risk: Timestamp fidelity and helper savings need real-filesystem/platform measurement. A sidecar added after that file's analysis remains an existing scanner completion/invalidation limitation; this fix does not claim otherwise. Mid-analysis filesystem mutation still has the ordinary bounded observation race.

### 3901993 — Make a continuing library scan give way to foreground work between steps (bug B-05.1)
- Verdict: RESOLVED WITH NITS
- What it does: Applies existing cancellation-aware background pacing after an advancing quantum releases its transactions, while retaining the per-source claim until the pause ends.
- Findings:
  1. **Nit — `server/internal/ingestion/cadence_test.go:202` (pressure-loop coverage): the new test observes pacing under a foreground probe, but never queues an actual foreground writer.** Source tracing plus existing gate tests establish retained priority discipline; add a combined production-loop/foreground-writer regression when qualifying contention so the explicit “no starvation” claim has direct evidence. This is a test-strength limitation, not an observed starvation defect.
- Tests run: Ingestion, dbwork and worker suites — PASS. Advancing, unchanged and sustained-pressure scans pass. `PaceBackground` waits 2–50 ms only outside transactions; no-job returns to the shared signal/safety tick, unchanged/refused work does not emit urgent continuation. Foreground writers retain their higher gate class, and scan background age promotion stops at interactive; this change never promotes scans above foreground or holds the gate during the pause.
- Residual risk: Weak-machine read latency/CPU competition is unmeasured. The first review's D-21 rollout question about idle storage-limited scan speed remains for Justin; this follow-up resolves missing pressure pacing and does not itself resolve that product choice.

### 9bcab2a — Recognise frame sizes by shape in any letter case and keep them out of the series name (bug B-05.12)
- Verdict: RESOLVED
- What it does: Makes technical-tag filtering case-insensitive, bounds frame recognition by known video widths/aspect shape, and removes recognized tags from series/year evidence.
- Findings: None for the three first-round cases. Real long-number `x` grammar survives unless the pair is recognizable frame evidence; diagnostic input remains intact. Prefix removal covers seasonal and air-date parsing and the folder/name comparison, not only suffix markers.
- Tests run: Full catalogue suite PASS. Independent in-memory overlay `TestReviewerEarlierAdversarialEpisodeNames` — PASS for uppercase resolution, uppercase DD/x264, frame before marker, folder variant, `101x102`, `1x02.720p`, and `4x12 1280x720`. Each now returns Show/year 0 and the expected marker; added cropped/portrait/year-prefix/air-date fixtures also pass in the suite.
- Residual risk: Inherently ambiguous dimension-shaped long season names remain interpreted as frame tags; no automatic repair of previously catalogued wrong titles.

### aeaefd5 — Let only a directly following year displace an earlier one as the release year (bugs B-05.7 / B-06.1)
- Verdict: RESOLVED
- What it does: Removes global bracket-year precedence in both parsers, allowing only directly adjacent release evidence to replace a title number. A bracketed release year cannot be displaced by a later encode/group date.
- Findings: None for the first-round regression. `Movie.2019.1080p.[2020]` is Movie/2019 again; existing numeric-title fixtures and adjacent `Blade Runner 2049 (2017)` remain correct. Index arithmetic is guarded in the matcher; the catalogue's first pattern necessarily supplies a preceding character. The disclosed Directors Cut case returns to the base's limitation, rather than inventing more filename heuristics.
- Tests run: Catalogue/metadata suites PASS, including the first-round numeric/adversarial fixtures and `TestScreenNamesKeepAYearThatBelongsToTheTitle`. Independent catalogue overlay also reran `TestFilenameTitleKeepsAYearThatBelongsToTheTitle` — PASS.
- Residual risk: Bare numeric matcher queries and nonadjacent title-number/release evidence remain partial-fix limitations; existing catalogue titles are not repaired by ordinary rescan.

## Catalogue and playback

Paths in this section are relative to `server-catalogue-playback`.

### 8222d41 — Keep a rebuild waiting on an id that never derives and try set-aside ids in turn, backing off (bug B-07.1)
- Verdict: RESOLVED
- What it does: Restores the rebuild readiness fence while letting healthy queued work and secondary workers pass a stalled entity. Poison retries rotate within a domain and have an internal 5-second-to-5-minute exponential schedule unaffected by commit wakes.
- Findings: None. Extracted `CheckReadiness` is byte-for-byte identical to `82aaa54`; the rebuild-completion branch again clears `rebuilding` only when `Backfill` returns no IDs, with the exact pre-change SQL. Failed enumeration rolls its cursor/Touch transaction back and stalls instead of skipping the bad entity. The all-domain correlated JSON scan is replaced by per-domain membership probes. A successful retry clears its poison entry; failing retries retain durable dirty work. This restores the old rule, not a new degraded-ready policy under D-14.
- Tests run: `go test ./internal/compactcatalog/...` — PASS, including stalled rebuild/ErrBuilding, higher poison recovery, bounded backoff and the supervised commit-noise loop. Targeted race result is in final verification.
- Residual risk: Poison memory is still proportional to distinct failed IDs. Very large failure populations and real derived-version rebuilds were not stress-tested; readers intentionally keep waiting on an unrecoverable rebuild.

### b48bc48 — Plan every re-plan of a presentation under its stored ceilings and refuse one that has none (bug B-02.12)
- Verdict: NOT RESOLVED
- What it does: Plans subtitle changes and copy fallback from the stored target, intersects current owner ceilings, persists narrowing, and refuses identifiable v1 rows without a target. These resolve both original missing-target/fallback blockers for the ordinary stored-reach cases.
- Findings:
  1. **Blocker — `server/internal/playbackv1/sessions_change.go:118`, `server/internal/playback/subtitle_delivery.go:118`: subtitle-generated bytes still escape a current member ceiling.** Subtitle-only PATCH does not set `bytes` or call admission; the legacy selection callback likewise obtains no current member cap. Start on LAN under no remote member cap, then burn in from outside LAN (or tighten the member cap): the stored target contains the old ceiling, and `narrowTarget` consults only owner configuration. An owner video ceiling above the member's limit therefore does not close the bypass. The report acknowledges and defers this exact first-round finding. Re-admit byte-changing subtitle operations with the existing-session exclusion and intersect the returned member ceiling with the frozen target; cover the legacy route and narrower-member/reach-change cases. Current resource authorization is necessary but does not enforce bitrate admission. This is existing enforced policy, not a new product decision.
  2. **Blocker — `server/internal/playback/subtitle_delivery.go:119`, `server/internal/playback/hls_produce_support.go:156`: current owner remote video ceilings rely on a different locality classifier from v1 admission.** `V1Choice.Remote` is set by `httpapi.requestIsRemote` (configured LAN networks/rightmost trusted-proxy hop), but `policy.NetworkClass` comes from `NetworkClassFromRequest` (generic private-address/first-forwarded-hop classification). A private peer excluded from the owner's LAN is remote for v1 and local in the stored policy. Start a 1.6-Mbit/s copied source with a 2-Mbit/s admitted cap, tighten the owner's remote video ceiling to 1 Mbit/s, then copy-timeline fallback: the production planner produces 2 Mbit/s because `narrowTarget` skips the current video limit for stored `local`. Independent Go overlay reproduced this exact plan. Carry verified v1 reach/current admission into both replan paths (or conservatively narrow where reach cannot be proven); do not use the legacy delivery hint as remote authority. Add the divergent-classifier/owner-tightening regression.
- Tests run: Playback/playbackv1 suites and focused HTTP subtitle/playback suite — PASS. `go test -overlay <anonymous-memory-overlay> ./internal/playback -run TestReviewerFallbackNarrows -count=1 -v` — FAIL: `actual remote fallback at 2000000 above current owner cap 1000000; stored network=local`. The probe uses the existing fixture and real CreateV1/downgradeCopyPlan, with no encoded bytes or server started. Finding 1 is source-confirmed and explicitly acknowledged by the lane.
- Residual risk: Real encoding/burn-in and integrated member admission remain unqualified. Stored ceilings are not widened by normal target arithmetic, but that alone does not prove compliance with current applicable limits. Old orphan/private candidates without a v1 association remain a disclosed recognition limit.

### 0dede93 — Compare browse timestamps at the precision they are written with (bug B-07.8)
- Verdict: RESOLVED
- What it does: Retains operand fractions and UTC offset normalization, using indexed text bounds plus a bounded list of same-second short strings that sort incorrectly. Documents the operand's day/second/fraction-unit interval semantics.
- Findings: None for the first-round fraction loss. `.123` is before a `.500` operand and `.999` is after it; short equivalent timestamps are placed correctly around fractional bounds, including UTC midnight and date-only stored values. SQL uses placeholders, at most a few extra values, and no function around the column. NULL/not-equals and date-day behavior remain explicit.
- Tests run: Full catalogue suite — PASS, including 18 fractional/offset cases, midnight/date-only cases and prior day/whole-second cases; API specification suite PASS. Vet PASS.
- Residual risk: Operator semantics intentionally refer to the unit written in the operand (e.g. `.500` names a millisecond period), now documented. No real UI saved-view test or query benchmark.

### d0bc72b — Choose a converted audio rate the output codec and the device both take (bug B-03.15)
- Verdict: NOT RESOLVED
- What it does: Splits the sample-rate list by output codec and rejects unsupported explicit AC-3/E-AC-3 builder rates. When no supported rate fits the device cap, it returns zero.
- Findings:
  1. **Should fix — `server/internal/playback/delivery_decision.go:372`, `:399`: zero means omit `-ar`, not refuse an incompatible output.** A 48-kHz source with an accepted 4-kHz AAC ceiling is still admitted for AAC conversion with `AudioSampleRate=0`; the actual graph is `-c:a aac -b:a 192k -ac 2`, allowing 48-kHz output above the cap. E-AC-3 with a 24-kHz cap similarly gets a codec-selected rate of at least 32 kHz, outside the device intersection. The new tests explicitly require this unsafe zero result and only assert argument construction succeeds. Return an incompatibility or select a compatible alternate codec when the intersection is empty; distinguish unknown/keep-valid-rate from no-supported-rate and test actual planned refusal/fallback. The decoder guard cannot fix an omitted rate.
- Tests run: Playback and decoder suites — PASS. Independent overlay `TestReviewerNoSupportedRateMustRefuseOrFallback` — FAIL, admitting the 48-kHz source with the unconstrained AAC arguments above. The codec-specific positive rates and builder rejection are correct; the empty intersection finding remains. Local `ffmpeg -hide_banner -h encoder=eac3` also confirms supported rates 48000/44100/32000; that is an installed-encoder capability read, not a conversion/device qualification.
- Residual risk: No hardware decoder used; the planner/argument failure is deterministic. Actual FFmpeg may auto-select a codec rate, which still cannot satisfy an empty device intersection.

### 4becca1 — Re-authorize a subtitle document being sent without taking the write gate (bug B-04.32)
- Verdict: RESOLVED
- What it does: Runs ordinary and manifest chunk authorization in read snapshots, extracting the exact manifest entitlement checks for shared use. Object lease acquisition still takes the writer gate once at open.
- Findings: None. Principal/item/source/session/generation, resource visibility/revision and timing checks remain. `readTx` calls the same authorizer and closes its snapshot on refusal. Repeated checks no longer acquire established-playback writer priority, removing the starvation interaction with background work. Removing an object reopen during manifest checking is safe while the already-open reader holds its object lease.
- Tests run: Subtitle suite and focused HTTP Subtitle/Fairness tests — PASS. `TestDocumentChecksDoNotWaitForOrTakeTheWriteGate` performs 100 checks while another writer owns the gate, verifies unchanged acquisition counters, then refuses withdrawn access. Vet PASS.
- Residual risk: High-volume live subtitle streaming and concurrent file-retirement IO not run; ordinary request budgets/snapshot limits remain.

### 8fa7063 — Leave a paused lyrics run untouched until the provider's pause has passed (bug B-10.5)
- Verdict: RESOLVED
- What it does: Remembers the provider deadline in the bulk worker and makes intermediate wakes read-only; writes waiting once and returns to fetching only after consuming an actual song outcome.
- Findings: None for phase churn/self-wake. Cancellation and current provider settings are checked before the pause guard; deadline expiry retries the pending song. A repeated refusal does not rewrite waiting. The LRCLIB deadline is read under its mutex, and bulk pause state belongs to its one supervised worker. Shared wakes may still cause bounded read-only checks, but neither early provider requests nor repeated phase commits.
- Tests run: Lyrics suite — PASS, including real supervised-loop fake-transport 429/commit-noise/resumption and deterministic repeated-refusal tests. Targeted race result in final verification.
- Residual risk: Live LRCLIB Retry-After behavior not contacted. Pause state resets with the worker/process, so one recovery turn may reestablish waiting after restart; the provider client still controls outgoing pace.

### f602e88 — Describe the media types an original file is served with (bug B-08.15)
- Verdict: RESOLVED
- What it does: Updates the grant endpoint description to list actual original video/audio MIME mappings, prepared-media types and remote relay behavior.
- Findings: None. The prose matches this lane's `originalContentType`/`audioContentType`, preserves the existing QuickTime-as-MP4 mapping, and does not promise browser support or change runtime behavior.
- Tests run: `go test ./internal/apispec/...` and focused HTTP `TestOriginalContentType` — PASS; read the fixed MIME maps and relay/prepared branches.
- Residual risk: Real client handling of each container is still the original commit's device qualification, not established by documentation.

## Social and downloads

Paths in this section are relative to `server-social-downloads`.

### fe48dbc — Keep an audiobook's resume point with an online session that started after an offline observation (bug B-14.11)
- Verdict: NOT RESOLVED
- What it does: Dates the bound online book writer using its indexed observation creation/last-report timestamps before a different-part pointer move. This protects the first-round different-part activation window.
- Findings:
  1. **Blocker — `server/internal/downloads/progress.go:267`: same-part observations bypass the new online evidence fence.** Start part 5 at noon with a 900-second stored resume and no activity (or old activity), then sync an 11:00 observation of part 5 at 300 seconds. `SyncProgress`'s activity check accepts it; `followBook` returns through the same-part UPDATE before checking the session creation timestamp. Independent Go probe observed book position 300000 with writer `new-online`, replacing the newer online 900000 position. This lane's original same-part update introduces book-pointer regression even though the older per-item sync has a related pre-existing window. Compare the session evidence before *any* book-position update, including same part; retain newer pointer/writer and add same-part absent/old-activity activation fixtures. Merely preserving `playback_id` does not preserve the online position.
- Tests run: Downloads/persistence suites — PASS, including added different-part start/report cases. Independent anonymous overlay `TestReviewerOlderSamePartCannotReplaceOnlineResume` — FAIL with `pos=300000 writer="new-online"`. Uses the existing lane helper's real playback_sessions insert/production observation trigger and exact activation upsert, not a real Service.Start; production activation source independently confirms no activity write.
- Residual risk: Full multipart/native outbox integration and delayed prepared occurrence activation remain unqualified. Broader B-14.10 arbitration remains outside this review, but cannot justify the reproduced book-pointer overwrite.

### 2baf3bd — Order audiobook parts listened to offline within the same second by when each was observed (bug B-14.11)
- Verdict: RESOLVED
- What it does: Reads the current part's millisecond offline watermark using the exact stable viewer key from reconciliation and requires a strictly newer observation for a cross-part move.
- Findings: None for the subsecond ordering finding. The watermark update and book move share the gated progress transaction, so forward/reverse batches and retries cannot use request arrival order. Equal millisecond evidence conservatively preserves the existing part. This resolves ordering across parts; it does not close fe48dbc's same-part online fence gap.
- Tests run: Downloads/persistence suites — PASS, including one-batch and separate-batch cases in both arrival orders and existing stale/owner-scoped receipt cases. Vet PASS.
- Residual risk: Native outbox generation and full online/offline integration not exercised. Activity remains whole seconds; the independent owner-scoped mark restores the precision needed for this specific ordering rule.

### 1d6820e — Keep sending a group's current title id to every member until clients can follow a hidden title (bug B-13.9)
- Verdict: RESOLVED
- What it does: Removes only current-title redaction, restoring the base snapshot/receipt/event IDs and event-reader signature. Keeps joined-membership reads and host-only authority IDs; documents the residual opaque-ID disclosure.
- Findings: None. Compared the full delta against base and the parent: `events.go`, HTTP event routes and privacy/store event tests return to base; `ReadGroup`/`ReadQueue` still use `loadJoinedMember`, SSE opens through `ReadGroup` and rechecks `StreamMember`, and non-host device/playback IDs remain empty/null. The independently accepted lobby/unknown-entry behavior is retained. `api/social.md` accurately distinguishes queue placeholders from the still-disclosed current ID; duplicate receipts no longer promise dynamic redaction. Hidden-title privacy remains unfixed by explicit deferral.
- Tests run: Social and focused HTTP `TestSocial` suites — PASS, including departed group/queue/events/refusal/rejoin, host authority identifiers and host transfer. Vet PASS. No TypeScript changed; no client suite rerun was needed for restoration of the old wire behavior.
- Residual risk: Server/client coordinated hidden-current behavior still required before redaction returns, including duplicate receipts and visible→hidden→visible player/readiness transitions on web/native.

## Skipped items and outstanding gates

This is a second-round scope review; accepted original commits were not reassessed. First-round skip judgments remain in their respective review files.

- Identity B-01.8 still needs Justin's definition of Hosted attachment; there is no follow-up and no evidence that the definition was decided. f698dab now resolves B-11.1 without selecting shared-input or household-DVR policy under D-42.
- Scanning D-21 remains a rollout question for the original cadence change plus 3901993: the technical pressure gap is repaired, but idle storage-limited speed is still the implementation. Do not read this technical verdict as Justin choosing an owner-facing pace policy. Durable NFC identity reconciliation, numeric-title matching, local/provider fan-out and post-analysis sidecar invalidation retain the previously disclosed partial-fix limits; none authorizes schema changes in this pass.
- Catalogue D-14 is deliberately unchanged: a stalled rebuild remains building. A different availability policy needs Justin's choice. The lane's deferral of *current member admission for byte-generating subtitle replans* is **not justified**: the binding playback spec already requires member/admin ceilings for every session; findings under b48bc48 need no new policy choice. A conservative refusal when original limits cannot be recovered is valid.
- Social B-13.9 current-title redaction is safely held for coordinated server/client work. Existing B-13.5 presence, B-13.19 terminal behavior, B-14.15 artifact/device retention (D-43), B-14.16 client receipt recovery and native continuation gaps retain the first-round skip judgments. The separate first-round B-13.21 disabled-membership blocker received no listed follow-up and remains outstanding; this report does not clear it or re-review b5da76a. Justin's re-admission/departure policy question is still explicit in that lane's report.

## Cross-commit issues

- b48bc48 and d0bc72b interact in replanning output: a frozen video ceiling is preserved in the common path, but output capability/empty-rate intersection is still invalid. Passing plan persistence and subtitle tests does not qualify encoded output or current member admission.
- 3901993 now uses the same pacing rule as derived/bulk work; 4becca1 removes the repeated subtitle reader write-gate pressure that the first review identified. Source priority tracing and isolated suite/race checks pass; sustained integrated browsing/playback/scan load remains unmeasured.
- fe48dbc and 2baf3bd fix different evidence gaps in the same query. Apply both, then repair the same-part early return; cross-part timestamp ordering cannot protect a branch that never consults the evidence. The catalogue lane's already-accepted online audiobook progress change still needs the combined Start-before-telemetry/offline-sync regression at integration.
- The lanes overlap in `server/internal/playback/subtitle_delivery.go`, playback tests and HTTP APIs as noted in the reports. These four worktrees share one Git object repository, but no merge was performed and no actual merge conflict is claimed. The identity lane's earlier release/backup overlap with platform-packaging is outside the listed follow-ups and remains an integration note.
- Hygiene: listed follow-ups contain no DDL/new dependency/native shared-copy edits or generated attribution. Subjects are plain behavior sentences, authors are Justin, `git diff --check` over each follow-up range and `gofmt -l` on changed Go files are clean. All four `git status --short` results are empty. Production goroutine additions use supervision; testing goroutines follow existing test conventions.

## Verification and reproducibility

Commands below ran from each lane's `server/`, with `GOCACHE=/tmp/gocache-codex GOPROXY=off`. Default tier only; no media/performance tier, real data, Hosted/provider endpoint, browser, dependency download, package installation or real server startup. Anonymous overlays append probes to existing test files via Linux memfd and point Go's overlay JSON at `/proc/<python-pid>/fd/<fd>`; no test source or overlay was written to a worktree or disk. Temporary test fixtures/caches are the normal test output. Pre-fix red runs in lane reports are not claimed as my executions.

| Lane | Command | Result |
|---|---|---|
| identity-ops-live | `go test ./cmd/server/... ./internal/networking/... ./internal/backup/... ./internal/livechannels/... ./internal/linearbuffer/... ./internal/playbackruntime/...` | PASS (cmd/server 43.649 s; DVR 4.191 s; remaining packages pass) |
| identity-ops-live | `go test ./internal/httpapi -run 'TestLongOwner|TestUnbudgeted|TestCanceledConnection' -count=1` | PASS, 2.929 s |
| identity-ops-live | `go test ./internal/playback -run 'TestLinearRetuneFollows|TestLinearRecovery' -count=1` | PASS, 0.007 s |
| scanning-metadata | `go test ./internal/ingestion/... ./internal/catalog/... ./internal/metadata/... ./internal/localmetadata/... ./internal/dbwork/... ./internal/worker/... -count=1` | PASS (ingestion 31.299 s; catalog 276.876 s; metadata 87.691 s) |
| scanning-metadata | `go test -race ./internal/ingestion -run 'TestScanLoopGivesWayToForeground|TestScanLoopDoesNotRespin' -count=1` | PASS, 40.278 s |
| catalogue-playback | `go test ./internal/compactcatalog/... ./internal/catalog/... ./internal/playback/... ./internal/playbackv1/... ./internal/decoder/... ./internal/subtitles/... ./internal/lyrics/... ./internal/apispec/...` | PASS (compactcatalog 75.743 s; catalog 276.531 s; playback 158.535 s; vod 130.389 s; remaining packages pass) |
| catalogue-playback | `go test ./internal/httpapi -run 'Subtitle|Fairness|TestOriginalContentType|TestPlaybackV1' -count=1` | PASS, 26.609 s |
| catalogue-playback | `go test -race ./internal/compactcatalog -run 'TestSupervisedLoopTries|TestStalledRebuild|TestSetAsideIDs' -count=1` | PASS, 98.563 s |
| catalogue-playback | `go test -race ./internal/lyrics -run 'TestSupervisedLyrics|TestBulkLyricFetchWaits' -count=1` | PASS, 39.345 s |
| social-downloads | `go test ./internal/downloads/... ./internal/social/... ./internal/persistence/...` | PASS, 18.107 / 4.326 / 74.636 s |
| social-downloads | `go test ./internal/httpapi -run 'TestSocial' -count=1` | PASS, 15.771 s |

`go vet` passed separately for identity's cmd/server/httpapi/livechannels/linearbuffer/playbackruntime/playback packages; scanning's ingestion/catalog/metadata/localmetadata/dbwork; catalogue's compactcatalog/catalog/playback/playbackv1/decoder/subtitles/lyrics/httpapi; social's social/downloads/httpapi. No output. `sh -n` and `dash -n packaging/linux/postinstall.sh` passed from the repository root; read the installed `deb-systemd-invoke` implementation and nfpm wiring. The lane's policy-101 stub qualification is reported evidence, not my execution. One attempted syntax command used the wrong working directory and failed to locate the script; the corrected root commands passed. HTTP whole-package suites were intentionally avoided because of the supplied playback-helper and /dev/fuse exclusions.

Independent probes:

| Probe | Method and expected invariant | Observed result |
|---|---|---|
| `TestReviewerRestoreCannotFinalizeBeforeMandatoryStartupRows` | Append to `cmd/server/restore_verification_test.go`: `startedState`, `stageBackupFolder`, delete only `maintenance_policy` singleton in candidate; real `ApplyStaged(...verifyRestoredState...)`; open result and run exact main.go background-priority query. A candidate unable to meet the mandatory query must not be finalized. | FAIL twice, including explicit requested cache/proxy environment. Outcome `restored`, then `sql: no rows in result set`. |
| `TestReviewerOlderSamePartCannotReplaceOnlineResume` | Append to downloads' book test: fresh `newBookHarness`, optional day-old `listenedOnline`, `startedOnline("new-online", parts[4], 900, now-1h)`, then sync same part at 300 seconds observed now-2h; assert pointer stays 900000/writer. | FAIL with both absent and day-old activity: 300000, writer `new-online`. |
| `TestReviewerNoSupportedRateMustRefuseOrFallback` | Append to audio-rate test: chrome AAC cap 4000; existing `decide` on H.264 MKV/DTS 48000-Hz stereo; refuse an admitted conversion with zero unconstrained rate. | FAIL: admitted graph `[-c:v copy -c:a aac -b:a 192k -ac 2]`, nil error. |
| `TestReviewerFallbackNarrowsActualRemoteOwnerCeiling` | Append to replan test: `newReplanFixture(1600000)`, actual CreateV1 with delivery NetworkLocal but `V1Choice{Remote:true, AdminMaxVideoBitrateBPS:2000000}`; lower configured owner video cap to 1000000; actual `downgradeCopyPlan`; assert ceiling. | FAIL: planned 2000000, owner 1000000, stored network local. |
| `TestReviewerEarlierAdversarialEpisodeNames` | Append to episodic test and call actual parser on all seven first-round uppercase/prefix/folder/long-number names; also run existing filename-title fixtures. | PASS, all expected Show/year 0 and episode markers. |

Each Go overlay probe was run as `go test -overlay <anonymous-memory-overlay> ./<package> -run '<probe-name>' -count=1 -v`. The source functions/fixture calls above make the probes reconstructable without treating a synthetic SQL approximation as production execution. The caller/locality disagreement in b48bc48 and the missing admission path were independently traced through HTTP and service code; no public network request was necessary.

## Summary

| Commit | Bug | Verdict | Blockers |
|---|---|---|---|
| bf149c9 | B-09.35 | NOT RESOLVED | Mandatory startup singleton can be absent after a recorded-successful restore. |
| e0f8155 | B-09.1 | RESOLVED | — |
| 35a0599 | B-09.3 / B-25.20 | RESOLVED | — |
| c89d573 | B-11.17 | RESOLVED | — |
| 47ca090 | B-11.22 | RESOLVED | — |
| f698dab | B-11.1 | RESOLVED | — |
| f1e4f04 | B-06.3 | RESOLVED | — |
| c951cab | B-06.12 | RESOLVED | — |
| ea56bd6 | B-05.22 | RESOLVED | — |
| 3901993 | B-05.1 | RESOLVED WITH NITS | — (existing D-21 rollout question retained) |
| 9bcab2a | B-05.12 | RESOLVED | — |
| aeaefd5 | B-05.7 / B-06.1 | RESOLVED | — |
| 8222d41 | B-07.1 | RESOLVED | — |
| b48bc48 | B-02.12 | NOT RESOLVED | Current member admission absent; divergent reach classification bypasses current owner cap. |
| 0dede93 | B-07.8 | RESOLVED | — |
| d0bc72b | B-03.15 | NOT RESOLVED | — (empty output-rate intersection still admitted; Should fix) |
| 4becca1 | B-04.32 | RESOLVED | — |
| 8fa7063 | B-10.5 | RESOLVED | — |
| f602e88 | B-08.15 | RESOLVED | — |
| fe48dbc | B-14.11 | NOT RESOLVED | Older same-part offline evidence overwrites a newer online book position. |
| 2baf3bd | B-14.11 | RESOLVED | — |
| 1d6820e | B-13.9 | RESOLVED | — |

**17 RESOLVED / 1 RESOLVED WITH NITS / 4 NOT RESOLVED / 0 NEW PROBLEM.** Four blockers, one Should fix, one Nit. The failures are remaining gaps in the requested fixes, not a claim that all were newly introduced by these follow-ups. No decision is needed to preserve current member/owner ceilings or newer online evidence, or to reject a restore that fails deterministic mandatory startup validation.
