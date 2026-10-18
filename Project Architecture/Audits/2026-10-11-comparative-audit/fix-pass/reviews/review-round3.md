# Third and final independent review

Scope: only the fifteen requested final fixes. Verdicts use RESOLVED / RESOLVED WITH NITS / NOT RESOLVED / NEW PROBLEM, as requested for this round. Repositories remain read-only. Host: ehler-homelab-mini-1. Context refresh was attempted but failed because its lock is outside the writable sandbox; existing guidance is used. No dependency downloads, browser/MCP tools, endpoints or subagents.

Paths below are relative to the indicated integration worktree. I read the two briefs, the prior findings and final lane sections, the requested diffs and surrounding callers, matching audit/verification entries, and the backup/playback/navigation/networking/social contract rules. The review does not reopen other commits. Severity distinguishes a release-blocking policy/ownership/startup defect from a narrower should-fix or nit.

## Server integration

### 09837cc — Refuse offline audiobook listening that is older than the online session holding that part (bug B-14.11)

- Verdict: **RESOLVED**
- What it does: Reads the book pointer and its existing evidence before changing either the part's progress or the book's position. The same-part path now observes session creation/report time; cross-part movement retains the millisecond watermark fence.
- Findings: None requiring changes. A rejected observation leaves the book writer and the part's own resume intact. The read and updates share the gated transaction; indexed lookups remain keyed by viewer/item/session. Newer same-part evidence still advances progress, and ordinary non-book progress remains unchanged.
- Tests run: Full downloads and persistence suites **PASS**. Independent overlay `TestReviewerOlderSamePartCannotReplaceOnlineResume` **PASS**, both absent and day-old activity: a session begun one hour ago at 900 seconds rejects a two-hour-old 300-second observation and retains `new-online` as writer. Added same-part start/tie/last-report and arrival-order tests pass.
- Residual risk: Native offline outbox and actual multipart playback were not exercised; this probe uses production SyncProgress and the existing exact activation fixture, not a physical online session.

### 2f1bf19 — Never aim a Watch Together seek near the end of a title, and wait out a seek the player does not report on (bug B-13.2 / related B-13.3)

- Verdict: **RESOLVED**
- What it does: Exposes duration, adds compensation only when enough title remains, and waits twenty seconds when an adapter cannot report completion. Explicit completion learns up to the same twenty-second bound, only when the player actually reached its requested aim.
- Findings: None requiring changes to the reviewed findings. The previous 90→106-second request on a 100-second title is now 90→98; a faster second seek does not complete early. Ordinary playback motion no longer masquerades as completion. The seventeen-second fixture now converges with a seventeen-second lead; failed seeks teach no lead, paused seeks wait, buffering suppresses feedback, and a host clock change discards stale settling evidence. Unknown duration disables compensation. The supported wait and lagging outcome above that bound are documented, without changing D-45's group-wide wait policy.
- Tests run: `npm run test:core` **PASS**, including the prior adversarial shapes (variable latency near the end, no-flag moving old position, seventeen-second convergence, failed/paused seeks, host advancement and buffering). `npm run test:cast` **PASS, 18/18**; web tests and web TypeScript **PASS**. Native mirrored `group-session.ts` is byte-identical to the canonical file.
- Residual risk: Real decoder landing and variable device latency remain unqualified. A no-completion-flag adapter avoids a storm but does not learn compensation; its test deliberately remains behind with one correction per twenty seconds. Both production video adapters now supply the flag.

## Native integration

### a2eab94 — Type the stubbed native runtime so tests that extend it type-check (bug B-22.4 test integration)

- Verdict: **RESOLVED**
- What it does: Gives the runtime stub an open method dictionary so tests can install and remove bridge methods without violating inferred object types.
- Findings: None. This is confined to the test stub; production bridge typing and behavior are unchanged. The annotation is on the correct object property and enables the actual added storage tests.
- Tests run: `npx --no-install tsc --noEmit`, `npx --no-install tsc -p test/tsconfig.json --noEmit`, `npm test`, `npm run lint:strings` **all PASS**.
- Residual risk: The permissive fake cannot prove native methods exist; this commit addresses the test compiler finding only.

### a3ce986 — Tell the Watch Together follower when a seek has not landed yet (bug B-13.2)

- Verdict: **RESOLVED**
- What it does: Supplies `seeking: !!pendingSeek` from the live native engine state while retaining the pending target as position.
- Findings: None. The provider reads `live.current`, so it does not freeze the first engine snapshot. The ordinary video seek's existing correlated pending state is the right completion signal. Combined with the refreshed shared follower, this avoids treating ordinary position movement as a landing.
- Tests run: Native app/test TypeScript, full `npm test` and strings lint **PASS**; canonical follower tests **PASS**.
- Residual risk: Android/Apple seek events and timeout completion require a device. Native audio owner paths without this engine's pending state remain a distinct adapter qualification.

### 4e7f795 — End a saved sign-in whose stored connection is damaged instead of retrying for ever (bug B-22.4)

- Verdict: **RESOLVED** (for the concrete malformed-JSON / structurally damaged / invalid-UTF-8 adapter cases)
- What it does: Maps confirmed record damage into `invalid_saved_connection`, ends restore instead of retrying it forever, and lets an explicit new connection replace the damaged record. No-op changes leave it intact.
- Findings: None demonstrating a healthy or temporarily unreadable sign-in ending. Android's structural check runs under the same store transaction/lock as read and sees atomically renamed records: a healthy saved record has the correct header, nonce/tag minimum and bounded length. It does not infer damage from Keystore errors. Apple requires `errSecSuccess`, actual returned NSData and failed UTF-8 decoding; every OSStatus failure stays retryable. Valid UTF-8 malformed JSON is classified in the JS adapter on both platforms. Other queues retain their own errors; the opt-in damage/replacement behavior is limited to the selected connection adapter.
- Tests run: Native app/test TypeScript, full native suite and strings lint **PASS**, including nine real-adapter storage cases for read/update/clear, malformed JSON, native damage, transient failure and replacement. Android/Objective-C source traced; no platform binary compiled here.
- Residual risk: Ciphertext authentication failure, pre-Android-12 missing-key ambiguity and Apple `errSecDecode` still retry, deliberately; this does not classify every possible permanent corruption. Real lock/unlock, truncation and post-replacement relaunch need the report's device controls. The Android intact-header bit flip is explicitly a preserve/retry control, not a recovery claim.

### 0fc94fe — Open the record store directory with a flag the Android SDK provides (bug B-19.4)

- Verdict: **RESOLVED** (the missing-SDK-constant build finding)
- What it does: Opens the trusted parent directory read-only before fsync, removing the nonexistent public `O_DIRECTORY` symbol.
- Findings: None for this change. `Os.open`, `Os.fsync` and `Os.close` remain paired and the path is the app-controlled store parent. Removing the type restriction does not add caller-selected paths or widen key-reset authority.
- Tests run: Native app/test TypeScript, suite and lint **PASS**. `javap -classpath /home/justin/Android/Sdk/platforms/android-36/android.jar android.system.OsConstants` confirms **O_RDONLY present, O_DIRECTORY absent**. Lane's standalone Kotlin compile is reported evidence; no Gradle build performed by this reviewer.
- Residual risk: Full Android app build and directory durability on Android/Fire OS remain unverified. The separately deferred old-Android lost-key recovery decision remains open.

### 3adb1c3 — Ask for route recovery when an auxiliary stream times out before it opens (bug B-22.18)

- Verdict: **RESOLVED**
- What it does: Treats a timeout before response headers as connection evidence on both platforms; an already-open idle stream continues to use the non-route `event_timeout` outcome.
- Findings: None requiring changes. Apple tests the outstanding opening reject block, which is cleared at response acceptance. Android's opening callback now reports transport; its established-stream read catch still distinguishes InterruptedIOException as idle. The JS adapter retains abort, stream-generation and route-authority fences before requesting recovery.
- Tests run: Native app/test TypeScript, full suite and string lint **PASS**, including route-recovery and idle/cancel/local-error controls and opening/idle classification guards.
- Residual risk: Native callbacks were not executed on a device. The revised black-hole/idle acceptance procedure is specific; it must distinguish time before headers from idle after headers.

### 5c31d51 — Tell the Watch Together follower how long the title is (bug B-13.2 / related B-13.3)

- Verdict: **RESOLVED**
- What it does: Supplies known positive duration from the live engine state, enabling the shared follower's end-room guard on native video.
- Findings: None. `s.duration` is the engine's duration in seconds; zero/unknown becomes undefined, for which the follower conservatively disables aiming ahead. This completes native wiring after the canonical follower refresh without editing the native shared-package copy in this commit.
- Tests run: Native app/test TypeScript, full suite and strings lint **PASS**; canonical near-end and unknown-duration follower tests **PASS**. Mirrored core file matches server integration.
- Residual risk: A physical title near completion, slow seek then fast seek, still needs the report's two-device check.


## Server integration — remaining open items

### 5d4b8e0 — Keep a restore only when the restored state has everything start-up needs and never recreates (bug B-09.35)

- Verdict: **NOT RESOLVED** (the missing-policy finding is fixed; another fatal prerequisite remains, and verification adds a healthy-backup refusal)
- What it does: Checks schema objects/columns, seeded-table presence, required policy values and restored store directories, then rebases paths inside the rollback boundary. Shared readers keep the new maintenance/DVR checks aligned with startup.
- Findings:
  1. **Blocker — `server/cmd/server/restore_verification.go:65–67,73`, with `server/cmd/server/main.go:345` and `server/internal/administration/file_operations.go:240–243`: mandatory file-journal recovery can still fail specifically because of the restored database.** The exclusion comment incorrectly says only the filesystem can stop recovery. A valid retained journal for `media-delete/review-operation-1` whose digest matches the previous database, alongside an otherwise startable restored database holding a different digest for that same receipt key, passes verification and is recorded restored. The next mandatory `RecoverFileOperations` returns `ErrConflict` on every start; the marker is already gone. Independently reproduced with actual folder Stage/Apply and the production recovery function, using an empty-move journal to isolate the database predicate. **Smallest correct change:** share a read-only journal/receipt compatibility preflight with recovery and run it before restore finalization; reject a digest disagreement while rollback remains available. Do not run irreversible journal file actions inside a verification routine that may later roll back the database. Add matching/missing/mismatched receipt controls and correct the exclusion comment.
  2. **Should fix — `server/internal/persistence/restored_state.go:109–119`, surfaced as a schema failure at `server/cmd/server/restore_verification.go:78,127`: a healthy restore newly depends on creating a complete reference database in the system temporary directory.** An untouched whole-folder backup is rolled back if TMPDIR is absent/unwritable/full, even though the state directory and candidate database are usable. Independent probe sets TMPDIR to a nonexistent path only after preparing the healthy backup: Apply rejects it as “missing part of the schema,” rather than identifying the failed reference creation. This is a new acceptance dependency, not missing backup state; its error is also cached by `sync.Once`. **Smallest correct change:** generate/embed required column names with a freshness test, just as schema object names are already generated, eliminating the production temporary database. At minimum preserve the actual environmental cause rather than reporting a damaged schema.
- Tests run: `go test ./cmd/server -run 'Restore|StateKey' -count=1` **PASS**; persistence/downloads/mediaartifact/storage suites **PASS**; focused mounts Inspect test **PASS**. Independent missing-maintenance-row probe **PASS**, rolling back and leaving the recovered state startable. Independent baseline-only older-schema fixture, upgraded through real `Open` then both new schema/seed checks, **PASS**. Independent journal-compatibility and unavailable-TMPDIR probes **FAIL** as above. Vet on all touched Go packages **PASS**. Full mounts suite reaches two known `/dev/fuse` host-semantics failures; these are not restore regressions.
- Residual risk: The older-schema probe validates migration plus acceptance predicates, not an installed-version upgrade and full listener startup. Trigger/view bodies and arbitrary column types are not compared. Schema/table presence cannot establish every constructor's data-dependent operation; the report's broad “everything startup needs” claim remains too strong. No installed server or real state was used.

### 73a7ed3 — Hold a subtitle re-plan to the member's current ceilings and to admission's view of the connection (bug B-02.12)

- Verdict: **NOT RESOLVED**
- What it does: Reads current connection/member/upload ceilings for request-driven subtitle changes and intersects them with stored v1 targets and current owner limits. Persists admission reach so a generic private-address hint cannot bypass the owner remote ceiling on fallback.
- Findings:
  1. **Blocker — `server/internal/playback/subtitle_delivery.go:165`: a genuine legacy session still ignores the current ceiling supplied by the new HTTP route.** The route attaches `ReplanCeilings`, but the Target-nil, non-v1 branch returns `rung.Target(policy,height)` without applying `current`. Independent real `Service.Create(...,"auto",...)` → `ChangeSubtitleDeliveryTx` probe, with remote/current member cap 1,000,000, plans burned-in video at **8,000,000 bit/s**. The “legacy selection” HTTP test actually names a v1 media session, so it does not exercise this branch. Owner limits can likewise depend on the conflicting legacy network hint here. **Smallest correct change:** narrow the legacy rung target by the verified current remote classification/member cap and current owner ceilings before planning; fail closed for byte-changing requests without verified ceilings. Add an actual legacy Create/PUT fixture. D-13's possible removal of legacy starts does not authorize leaving a currently reachable replan policy bypass.
  2. **Blocker — `server/internal/playback/hls_produce_support.go:184`, with `server/internal/playback/subtitle_delivery.go:143–147`: copy-timeline fallback still cannot see a newly tightened member cap.** It passes `current=nil, request=false`; only the previously frozen member cap is available. A remote session admitted at 2 Mbit/s can begin fallback conversion at 2 Mbit/s after its member cap becomes 1 Mbit/s. The final lane report explicitly confirms this gap. The current owner cap is now read correctly, but that does not enforce a narrower current member cap. **Smallest correct change:** wire a current-policy ceiling reader into fallback using the established session principal and stored verified reach, and intersect before conversion. Do not widen an unreadable member limit to unlimited. Add start-at-2/tighten-to-1/fallback regression, plus an unaffected LAN control.
- Tests run: Focused playback tests and HTTP subtitle/playback-v1 routes **PASS**. `TestSubtitleReplanRoutesHoldToTheMembersCurrentCeiling` explicitly rerun with `-v`: **PASS, not skipped**, including v1 PATCH, PUT by v1 ID/media ID, spoofed locality headers and lowered caps. Independent earlier divergent-classifier/owner-tightening fallback probe **PASS** (≤1 Mbit/s). Independent actual legacy burn-in/member-cap probe **FAIL** (8 Mbit/s). Existing LAN/current-member and LAN/owner-remote-cap controls **PASS**; a healthy current-format LAN replan is not generally refused. Vet **PASS**.
- Residual risk: Encoded output and integrated member-cap changes during media production were not qualified. Reading caps outside the selection transaction is also a snapshot, not proof against a concurrent policy change. Old v1 plans without recoverable targets are deliberately refused; older stored reach is treated conservatively as remote.

### 6eb7c5b — Refuse an audio conversion the device has no rate for, or convert to a codec it has one for (bug B-03.15)

- Verdict: **NOT RESOLVED**
- What it does: Separates an empty output-rate intersection from “keep source rate,” rejects affected routes/renditions, and tries a supported passthrough codec or AAC when the preferred codec has no acceptable rate.
- Findings:
  1. **Should fix — `server/internal/playback/delivery_decision.go:362–363`: unknown input rate still bypasses output feasibility.** The function returns `(0,true)` before checking whether the output codec has any rate within the device cap. With DTS sample rate unobserved and AAC MaxSampleRate=4,000, production planning admits `-c:v copy -c:a aac -b:a 192k -ac 2` without `-ar`. AAC has no supported rate at/below 4 kHz, irrespective of the unobserved source rate; passthrough codecs with an empty intersection have the same early return. This preserves the reviewed unsafe “zero means unconstrained” case whenever probe facts omit rate. **Smallest correct change:** compute output intersection first; reject/fallback when empty even for unknown source rate. When source rate is unknown but the intersection is nonempty, set an explicit supported output rate within the device/48-kHz bounds. Add unknown-rate tiny-cap and ordinary 48-kHz-cap fixtures.
- Tests run: Focused sample-rate/audio playback tests **PASS**, including positive resampling, E-AC-3→AAC fallback and known-rate refusal. Independent original 48-kHz/tiny-AAC-cap probe now **PASS** (route refused); the same probe with sample rate zero **FAIL** (unconstrained AAC conversion admitted). Vet **PASS**.
- Residual risk: No encoded output or decoder device used. Planned unknown-rate behavior must be bounded before FFmpeg chooses a rate; argument construction alone cannot establish that property.

### 13bf948 — Replace a stored vault key that cannot decrypt along with the record it guards (bug B-22.4)

- Verdict: **NOT RESOLVED** (narrow remaining non-CryptoKey case; no Blocker)
- What it does: Treats wrong algorithm/type/usages as permanent damage and atomically persists a newly generated AES-GCM key together with a replacement credential record.
- Findings:
  1. **Should fix — `web/src/bridge/server-connections.ts:30–32`: key metadata shape is not proof that the value is a CryptoKey.** A stored plain object `{type:'secret',algorithm:{name:'AES-GCM'},usages:['encrypt','decrypt']}` passes `usable`, then real WebCrypto decrypt throws TypeError. Renewal still reports retryable store unavailability and explicit replacement still fails before save. The added non-key fixture only uses `{type:'secret'}`, so it tests missing fields rather than brand validation. This is deterministic invalid material, not IndexedDB contention. **Smallest correct change:** validate actual CryptoKey identity/brand as well as metadata, preserving valid IndexedDB-cloned keys; classify demonstrably non-key objects as damage and replace key+record together. Add the complete-shaped plain-object fixture.
- Tests run: Core, web suite and web TypeScript **PASS**. Independent expansion of the real browser-vault test with the complete-shaped plain object **FAIL** (TypeError instead of classified damage); all nine original cases in that run **PASS**. HMAC, encrypt-only, missing-key and tampered-cipher cases now recover correctly. Transient IndexedDB failures preserve the credential.
- Residual risk: Actual IndexedDB contention and cloning on supported browsers remain unqualified. This failure was exercised through the real vault class and real WebCrypto using the existing storage harness.

## Native integration — remaining details and open items

### 5e254f4 — Enroll every member of a large download request in bounded batches (bug N-V08.1)

- Verdict: **NOT RESOLVED** (the stable single-request fifty-page cutoff is fixed)
- What it does: Carries a continuation cursor across bounded page batches and pauses reading when wanted items reach a backlog threshold. Enrolled-item deduplication avoids collecting the same member again within the running owner/request lineage.
- Findings:
  1. **Should fix — `apps/native/src/bridge/offline-downloads.ts:371,389–392`: a transient page failure is treated as the tail until request counts change.** A 900-member request fails once after member 499; network recovers, the service republishes the same counts, and enrollment remains at **500**, with no retry. Counts need never change on an already-settled request, so the remainder is silently omitted; reconnect with the same in-memory cursor/tail also does not clear the suppression. This contradicts the report's “network off/on carries on” device criterion. **Smallest correct change:** separate actual exhaustion from retryable failure, retain the failed cursor and schedule a bounded backoff retry independent of count changes. Preserve terminal/refused-cursor behavior. Add a recovered failure with unchanged total/ready/preparing/queued.
  2. **Should fix — `apps/native/src/bridge/offline-downloads.ts:364,376–377`: backlog and page bounds do not cover the whole pass.** Every request's first page is enrolled unconditionally, before any backlog check; fifty pages is also per request, not per invocation. Independent sixty-request first-page fixture creates **6,000 wanted members** despite the stated 5,000 bound. Arbitrarily many tracked requests can overflow the one-megabyte state document; the check is a member count for one owner's bucket, not a document-byte bound. **Smallest correct change:** enforce one pass-wide page/work budget and available backlog/serialized-byte room for initial and continuation pages alike, preserving unconsumed initial-page members/cursors for later passes. Add multiple-request and existing-other-owner/document-size controls.
- Tests run: Native app/test TypeScript, full suite and strings lint **PASS**. Existing real-class 5,443-member fixture **PASS**: all members collected or wanted, 54 distinct continuation cursors, bounded backlog for that single request, no duplicates in that trace. In-memory expansion of the same harness: recovered unchanged-count failure **FAIL** (500 vs 900); sixty first pages **FAIL** (6,000 backlog). Four original enrollment cases in that expanded run **PASS**.
- Residual risk: No large physical-device download run. Cursor and enrolled set remain in memory; process death/durable request-following is separately deferred B-14.8. The known permanently uncollectable-prefix head-of-line limit and set growth remain disclosed. This commit should not claim universal completion or a one-megabyte guarantee from the single-request test.

### 518f31c — Keep a late link result from opening for a different viewer (bug B-19.1 / B-21.14 / NAV-11)

- Verdict: **NOT RESOLVED** for the full prior NAV-11 entry; **the delayed-launch wrong-viewer finding itself is resolved**. No new production wrong-viewer blocker was established.
- What it does: Reserves launch time/viewer before the OS returns the URL, invalidates superseded initial intents, and adds a Shell generation/viewer fence around delayed title/route results.
- Findings:
  1. **Should fix — `apps/native/src/shell/pending-link.ts:109`, `src/shell/Shell.tsx:309`: the earlier consume-before-dispatch loss remains.** The owner clears the held intent before the deferred callback runs; same-viewer Shell teardown in that tick discards it. Attach on confirmed navigator readiness and acknowledge consumption only after accepted delivery, preserving the original expiry on same-viewer replacement. The report explicitly leaves this finding open; it is not repaired by launch reservation.
  2. **Should fix — `apps/native/src/shell/pending-link.ts:90`, `src/screens/auth/AuthFlow.tsx:58–63,94`, `src/screens/auth/HostedSignIn.tsx:84`: the prior duplicate-delivery and independent launch-reader gaps remain acknowledged.** The same launch plus later URL event can dispatch twice; auth consumers can separately retain/replay an invitation beyond the central owner's expiry/supersession rules. Share launch ownership/receipts with those consumers, preserving account-return handling, and coalesce a duplicate platform launch without suppressing a later deliberate repeated link. These are prior open items, not newly re-reviewed commits.
- Tests run: Native app/test TypeScript, full suite and strings lint **PASS**. New actual-intake deferred-promise tests verify launch reservation, viewer mismatch and newer-event supersession; pure fence tests verify lookup invalidation on newer links/viewer/unmount. I also extracted the production handler and exercised a synthetic retained-Shell render/effect gap: it calls a changed player ref while the old fence remains. **That is not a demonstrated production bug:** subsequent full caller tracing shows `session.tsx:410–417` clears the viewer at the handoff fence, and `ViewerSelectionCommit` awaits native storage and server verification before publishing; Root unmounts Shell outside ready scope. The synthetic probe omitted this mandatory selection boundary, so it is excluded from findings/counts.
- Residual risk: OS launch/event identity, navigator readiness and React scheduling on devices remain qualification. The existing source checks still need the report's slow-server/profile-switch device test. The remaining NAV-11 delivery/duplicate/auth-consumer gaps are independent of the now-correct launch reservation.

### 1531aaf — Keep a dropped connection or an interruption from being taken for a media failure (bug B-20.6)

- Verdict: **RESOLVED WITH NITS**
- What it does: Keeps the first classified URL/POSIX cause, limits Apple log evidence to recent unnamed failures, recognizes log-only URL transport errors and excludes known interruptions from startup route learning. Android timeout/remote-process failures use the same “uncertain” policy.
- Findings:
  1. **Nit — `apps/native/Native/PorticoPlaybackController.m:476`: replace the four numeric literals with their public enum names.** **None of the four values is wrong:** `AVErrorSessionWasInterrupted=-11818`, `AVErrorMediaServicesWereReset=-11819`, `AVErrorOperationInterrupted=-11847`, `AVErrorOutOfMemory=-11801`. Apple's public documentation confirms these four non-format meanings: [session interruption](https://developer.apple.com/documentation/avfoundation/averror-swift.struct/code/sessionwasinterrupted?language=objc), [media services reset](https://developer.apple.com/documentation/avfoundation/averror-swift.struct/code/mediaserviceswerereset?language=objc), [operation interruption](https://developer.apple.com/documentation/avfoundation/averror-swift.struct/code/operationinterrupted?language=objc), [out of memory](https://developer.apple.com/documentation/avfoundation/averror-swift.struct/code/outofmemory?language=objc). Current DocC pages/JSON do not expose raw numeric assignments; I cross-checked the actual assignments in Apple's [AVError.h from the public iPhoneOS18 SDK mirror](https://github.com/xybp888/iOS-SDKs/blob/master/iPhoneOS18.0.sdk/System/Library/Frameworks/AVFoundation.framework/Headers/AVError.h). This is header evidence, not an Apple-platform compile. Using the declared symbols removes the memory-based maintenance risk and lets SDK compilation check it.
  2. **Nit — `apps/native/Native/PorticoPlaybackController.m:29`: the status-comment pattern still excludes “HTTP response 503” / “unrecognized http response 503”.** The prior reviewer correctly qualified the real emitted wording as unconfirmed. Exact numeric log status and URL-domain evidence now handle many such failures; an unmatched fresh unknown log suppresses learning through `uncertain` but gives a plain failure rather than network recovery. Add optional `response` to the status grammar and a native status-comment fixture; capture actual AVFoundation logs before claiming runtime coverage.
- Tests run: Native app/test TypeScript, full suite and strings lint **PASS**, including uncertain startup suppression, explicit engine fallback and HTTP status parity. Source traced through Objective-C error/underlying/log branches and Android Media3 sets into the shared classifier. Four public-documentation meanings and SDK-header numeric assignments checked as above. No native classifier compiled/executed on Apple here.
- Residual risk: Real CoreMedia-only wrapping and log freshness/correlation still need device captures. Freshness is a fifteen-second heuristic, not causal correlation. The earlier platform notes about preserving `hasPlayed` across route-resume reloads, crossfade promotion and DataNotAllowed parity remain separate disclosed nits; these were not solved by this commit.

## Checks and reproducibility

All commands used local installed dependencies. No dependency downloads, repository mutation, real database, demo/Hosted endpoint, browser/MCP tool or subagent was used. Go environment for every command was `GOCACHE=/tmp/gocache-codex GOPROXY=off`. HTTP and cmd/server test runs used `PORTICO_PLAYBACK_TEST_HELPER=/tmp/claude-1000/-home-justin-projects-Portico/e84a5e65-b175-4fb1-ada8-497cedb519f8/scratchpad/integration/playback-test-helper` where appropriate.

| Worktree / command | Result |
| --- | --- |
| Server root: `npm run test:core` | PASS |
| Server root: `npm run test:cast` | PASS, 18/18 |
| Server root: `npm test --workspace portico-web` | PASS |
| Server root: `node node_modules/typescript/bin/tsc --noEmit -p web/tsconfig.json` | PASS |
| Server/server: `go test ./cmd/server -run 'Restore\|StateKey' -count=1` | PASS, 52.162 s |
| Server/server: `go test ./internal/persistence ./internal/downloads ./internal/mediaartifact ./internal/mounts ./internal/storage -count=1` | Persistence/downloads/mediaartifact/storage PASS; mounts has two host-FUSE failures (`TestCommandReceiptsReplayCASAndRemovalRecovery`, `TestReceiptRestartUsesOneOwnedReplacement`), not attributed to the final changes |
| Server/server: `go test ./internal/mounts -run 'TestInspectPrivateDirectory' -count=1` | PASS |
| Server/server: `go test ./internal/playback ./internal/playbackv1 -run 'Replan\|Ceiling\|SampleRate\|Book\|Subtitle\|Copy\|Audio' -count=1` | Playback PASS, 37.700 s; playbackv1 pattern matched no tests, so its full suite was subsequently run |
| Server/server: `go test ./internal/playbackv1/... -count=1` | PASS, 1.479 s |
| Server/server: `go test ./internal/httpapi -run 'Replan\|Subtitle\|TestPlaybackV1' -count=1` | PASS, 41.510 s |
| Server/server: `go test ./internal/httpapi -run '^TestSubtitleReplanRoutesHoldToTheMembersCurrentCeiling$' -count=1 -v` | PASS, 2.056 s, not skipped |
| Server/server: `go vet ./cmd/server ./internal/persistence ./internal/playback ./internal/playbackv1 ./internal/downloads ./internal/mediaartifact ./internal/mounts ./internal/storage ./internal/httpapi` | PASS |
| Native/apps/native: `npx --no-install tsc --noEmit` | PASS |
| Native/apps/native: `npx --no-install tsc -p test/tsconfig.json --noEmit` | PASS |
| Native/apps/native: `npm test` | PASS |
| Native/apps/native: `npm run lint:strings` | PASS; existing pending diagnostics only |
| Both requested final ranges: `git diff --check`; both `git status --short` | PASS; clean |
| Native canonical follower mirror compared with server integration | Identical |

The first independent audio-overlay attempt had a reviewer probe pointer-type mistake and did not run; it was corrected before the results above. The first browser-probe transform tried the server's version-only TypeScript package, so it did not run; it was rerun with the existing Rolldown transform. The first extracted Shell handler needed removal of a TypeScript non-null assertion; the successful run uses the same handler logic. These harness setup failures are not product findings.

Go probes were appended to existing test source in Linux memfd and run with `go test -overlay /proc/<review-process>/fd/<overlay> ... -run TestReviewer -count=1 -v`; no probe source or overlay file was written into a repository. The source fixture calls make them reproducible:

| Probe | Production boundary and result |
| --- | --- |
| `TestReviewerRestoreCannotFinalizeBeforeMandatoryStartupRows` | `startedState` → `stageBackupFolder` deleting only maintenance singleton → real Apply/verification → previous state restored and `startable` succeeds: PASS |
| `TestReviewerHealthyOlderSchemaMeetsRestoreRequirements` | dbwork handle → `migrateWith(steps[:1])` → close → real `Open` installs later migrations → schema/seed acceptance: PASS |
| `TestReviewerRestoredReceiptCannotBlockMandatoryJournalRecovery` | Previous receipt/journal digest `current-digest`; candidate same key `earlier-digest`; real folder Apply → candidate kept → actual `administration.NewAt(...).RecoverFileOperations`: FAIL with ErrConflict |
| `TestReviewerHealthyRestoreDoesNotNeedSystemTemporaryDirectory` | Untouched started-state backup, set TMPDIR to a nonexistent child only after staging; fresh test process/once state; real Apply: FAIL, healthy candidate refused as schema damage |
| `TestReviewerFallbackNarrowsActualRemoteOwnerCeiling` | Actual CreateV1 with NetworkLocal hint / Remote=true admission, stored 2-Mbit target, owner lowered to 1 Mbit, actual downgradeCopyPlan: PASS |
| `TestReviewerLegacySubtitleHonorsCurrentMember` | Actual legacy Service.Create(auto), real gated subtitle callback with current remote/member=1 Mbit: FAIL, conversion 8 Mbit |
| `TestReviewerNoSupportedRateMustRefuseOrFallback` | Production planner and CodecArgs with chrome AAC MaxSampleRate=4,000: known 48-kHz source refused (PASS), unobserved source rate admits unbounded AAC arguments (FAIL) |
| `TestReviewerOlderSamePartCannotReplaceOnlineResume` | Existing book harness exact online activation at 900 s / one hour ago, absent or day-old activity, real SyncProgress 300 s / two hours ago: PASS, retains position/writer |

JS probes were transformed and executed in memory using the existing installed compiler/loader and native/component harnesses. The existing browser store's damage map was expanded with the complete-shaped non-CryptoKey value; real WebCrypto returned TypeError. The enrollment harness was expanded with a recovered failed page with unchanged counts and sixty initial request pages: wanted=500/900 and 6,000 respectively. The extracted Shell handler also permits a synthetic render/effect timing gap, but the probe omitted the actual selection handoff that clears the viewer and unmounts Shell before publication. It is explicitly excluded from product findings; device scheduling was not exercised.

## Skipped items and owner decisions

The final requested commits introduce no new product-policy choice. The old-Android/Fire OS lost-key case remains explicitly deferred: before API 31, absent alias can be transient, and automatically clearing every secure record is not justified by that ambiguous evidence. Justin still needs to choose an explanatory permanent-lockout policy or a confirmed reset/reinitialization flow; the lane's proposed distinct native code, notice and user-confirmed whole-store reset is a concrete safe option. The restored-record and SDK-constant fixes do not resolve that decision.

The final web report leaves prior page-announcement corrections open and cites missing browser qualification. Lack of an available browser alone is not a skip justification under FIX-BRIEF rule 4; the resource/route-bound heading work should remain an implementation open item, not be relabeled a product decision. No earlier commit was assigned a new verdict here. Other existing decisions—D-13 legacy route removal, D-45 group-wide waiting and D-36 restore credential semantics—remain unchanged. The current-member/fallback and retained-journal findings above are correctness requirements independent of those choices.

## Cross-commit issues

1. **Watch Together integration is complete for the reviewed inputs:** `a3ce986` supplies pending state, `5c31d51` supplies duration, and the separately refreshed shared source matches canonical `2f1bf19`. No native-copy edit is needed to close these findings. Real devices remain the acceptance boundary.
2. **Storage integration now classifies the known native damaged-record cases, but browser brand validation is incomplete.** These adapters share the same core damage classification. Do not “fix” the browser object case by making every crypto/store exception terminal; retain transient-store controls and atomic key+record replacement.
3. **Ceiling coverage is partial, despite passing v1 HTTP fixtures.** Actual legacy sessions and requestless fallback must use the same applicable current ceilings. `6eb7c5b` also participates in these replans: neither a stored target nor a codec rate policy may silently become “unlimited” when input evidence is incomplete.
4. **Restore predicates must fit the rollback boundary:** restored path rebasing is database-only and can roll back; actual file-journal recovery can irreversibly move/remove bytes. Share its read-only compatibility predicate, not its side effects. Preserve the previous database and journals on a refused candidate. Remove the reference-schema temporary-IO dependency without loosening required schema checks.
5. **Link intake and delivery are separate authority boundaries.** Launch reservation closes the late initial-result finding, and the real viewer handoff tears down the old Shell before publication. Central intake still cannot acknowledge a deferred navigation it has already consumed, or coalesce the separate platform/auth consumers. Preserve the viewer handoff when repairing those remaining delivery gaps.
6. **Enrollment remains vulnerable at retry/global-bound boundaries:** the single-request test proves the fifty-page cutoff closed, but cannot establish full-pass work/byte bounds or recovery with unchanged counts. Device network-off/on acceptance must include a fully prepared request whose counts remain stable.
7. All fifteen subjects describe behavior in plain language; no schema/dependency changes or unrelated shared native-package edits occur in these requested commits. The changes touch the same previously edited functions, so any future merge should preserve the ceiling, damage and group-state additions together. No speculative merge was performed.

## Summary

| Commit | Bug | Verdict | Blockers |
| --- | --- | --- | --- |
| 5d4b8e0 | B-09.35 | NOT RESOLVED | 1 — retained journal/receipt disagreement can block startup after finalization |
| 73a7ed3 | B-02.12 | NOT RESOLVED | 2 — legacy ignores current ceilings; fallback ignores tightened member cap |
| 6eb7c5b | B-03.15 | NOT RESOLVED | 0 — unknown-rate empty intersection still admitted |
| 09837cc | B-14.11 | RESOLVED | 0 |
| 2f1bf19 | B-13.2 / B-13.3 | RESOLVED | 0 |
| 13bf948 | B-22.4 | NOT RESOLVED | 0 — key-shaped non-CryptoKey remains unrecoverable |
| a2eab94 | B-22.4 test integration | RESOLVED | 0 |
| a3ce986 | B-13.2 | RESOLVED | 0 |
| 4e7f795 | B-22.4 concrete adapter cases | RESOLVED | 0 |
| 5e254f4 | N-V08.1 | NOT RESOLVED | 0 — unchanged-count retry and pass-wide bounds |
| 0fc94fe | B-19.4 SDK build | RESOLVED | 0 |
| 518f31c | B-19.1 / B-21.14 / NAV-11 | NOT RESOLVED | 0 — delayed wrong-viewer launch repaired; prior delivery/duplicate/auth-reader should-fixes remain |
| 1531aaf | B-20.6 | RESOLVED WITH NITS | 0 — all four Apple numeric codes are correct |
| 3adb1c3 | B-22.18 | RESOLVED | 0 |
| 5c31d51 | B-13.2 / B-13.3 | RESOLVED | 0 |

Totals: **8 RESOLVED / 1 RESOLVED WITH NITS / 6 NOT RESOLVED / 0 NEW PROBLEM verdicts**. **Three Blocker findings, seven Should-fix findings, two Nits.** The restore's temporary-reference requirement is an additional new regression inside its NOT RESOLVED section; that verdict prioritizes the remaining fatal prerequisite. In the original brief's vocabulary: eight clean accepts, one accept with nits, six changes still required. These are owner open items for the final pass, not requests for another review/fix round.
