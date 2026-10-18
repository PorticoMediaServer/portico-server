# Independent review — web-client-core

Worktree: `/home/justin/projects/Portico/.worktrees/audit-fixes/server-web-client-core`, base `82aaa54`.

Review in progress. Repository access is read-only. Context refresh was attempted but blocked by the sandbox's read-only sync lock; existing canonical guidance is being read directly. Host: `ehler-homelab-mini-1`.

## Evidence and execution limits

I read all 18 individual commit diffs, the implementation brief, the lane report, each corresponding first-pass audit entry and second-review entry/quick-check row, the final report's open decisions, and the relevant identity, networking, accessibility, and social playback contracts. Paths below are relative to the reviewed worktree unless explicitly absolute. Native adapter references are to the canonical native checkout and were read only.

Independent checks at HEAD:

- `npm run test:core`: **PASS, 1,477 tests**, no failures/skips.
- `npm run test:cast`: **PASS, 16 tests**, no failures/skips.
- `npm run check --workspace portico-web`: **BLOCKED**, exit 1 at `tokens`: it attempts to rewrite `web/src/ui/tokens.css`, and the read-only filesystem rejects it. No repository file changed.
- The non-writing remainder of that check was run separately: `npm run lint:design --workspace portico-web -- --strict`, `node packages/client-core/scripts/check-preference-consumers.mjs`, `npm run lint:strings --workspace portico-web`, `node node_modules/typescript/bin/tsc --noEmit -p web/tsconfig.json`, and `npm test --workspace portico-web`: **all PASS; 417 web tests**. String/error lint retains its existing dev-only pending warnings.
- Generated tokens were recomputed in memory from the existing generator, replacing its write with a read/compare: **exact match** with checked-in `tokens.css`.
- Preference check with `PORTICO_REACT_NATIVE_REPO=/home/justin/projects/Portico/apps/portico-react-native`: **PASS, 56 keys, zero exceptions**; the default worktree run also passes and explicitly discloses the absent sibling native checkout.
- `git diff --check 82aaa54..HEAD`: **PASS**. `git status --short`: clean.
- Additional inline Node probes used production modules and/or an in-memory adaptation of the existing test harness. They did not write source or create fixtures. Results are described per commit. These establish deterministic logic behavior, not browser/device acceptance. An initial browser-store probe hit the existing component harness's unsupported `export async function` transformation; it was rerun successfully using the installed TypeScript transformer and the actual store class. An initial Cast probe omitted VM globals; the corrected probe succeeded.

The lane report supplied is a condensed final-message reconstruction, not the full per-ID report required by the implementation brief. Its native/browser test notes are useful but do not establish device acceptance. The context-sync command could not acquire its read-only lock; existing canonical project and verification guidance was read directly. No browser, MCP, service endpoint, dependency download, repository mutation, or subagent was used.

## Commit reviews

### 060737e — Keep a saved sign-in when a renewal gets a 403 that is not the server ending it (bug B-01.9)

- Verdict: **ACCEPT**
- What it does: Narrows terminal 403 renewal responses to `access_limit`, preserving the refresh credential and exact pending request ID for other 403 responses. The family/token fence on deletion remains intact.
- Findings: None requiring changes. I traced `identity_routes.go:209–240`, `identity/refresh.go`, `identity/direct_policy.go:84`, Hosted `attachment_family.go`, and `access/enforcement.go:74`: invalid/reused/revoked credentials and lost membership return unauthorized; refresh admission's schedule denial revokes the just-issued family and maps to `403 access_limit`. The changed branch still ends those real terminal paths. Rejected origins and gateway HTML do not establish family revocation, so preserving them is correct. Both native and browser ultimately use this shared function.
- Tests run: `npm run test:core` passes, including both added proxy/origin cases and the access-limit refusal, existing revocation/reuse/migration tests, and exact retry recovery. The added tests would take the old deletion branch without this fix.
- Residual risk: A proxy that continually blocks refresh causes continuing unavailable retries, deliberately preserving unknown authority state. Status-only 401 deletion remains a **pre-existing** narrower unresolved part of the audit claim; this commit does not fix error provenance generally. The stale `web/src/bridge/restore-policy.ts` comment saying “401/403” should eventually be corrected. Real proxy behavior was not tested.

### dc5d0e6 — Keep a saved sign-in when the protected store cannot be read during renewal (bug B-22.4)

- Verdict: **REJECT**
- What it does: Separates exceptions from `storage.read()` from a successfully read absent, replaced, or invalid remembered record, making every read exception retryable.
- Findings:
  1. **Blocker — `packages/client-core/src/server-connections.ts:234–237`: permanent corruption is now an endless transient retry.** Browser `web/src/bridge/server-connections.ts:23–31` performs JSON parsing and AES-GCM/UTF-8 decoding *inside* `read()`, so invalid JSON, incomplete encrypted records, and failed authentication never reach the “parse means replaced” branch. The native adapter's `NativeQueueStore.change()` also parses JSON inside its read transaction; Android maps oversized/bad-format/decrypt failures to the same `secure_storage` rejection as temporary failures, and Apple's `PorticoSecureStorageRead.h` rejects unreadable/non-UTF-8 bytes with that same code. `session-refresh.ts:139–177` retries these forever, ultimately every 120 seconds; there is no recovery classification/state. The browser's `change()` decodes first too, preventing ordinary replacement or deletion of a corrupt vault. **Recommend:** expose unavailable versus deterministically corrupt outcomes from the adapters; preserve and retry unknown/locked storage, but stop automatic retries on confirmed corruption and provide a deliberate local recovery/replacement path without claiming server revocation or silently erasing unknown credentials. Add browser malformed-JSON/missing-key/decrypt-failure and native malformed-JSON tests, plus transient-then-readable and absent-family tests.
- Tests run: `npm run test:core` passes, including both added transient-read/absent-family cases. A separate inline probe instantiated the real transpiled `BrowserProtectedStore`, supplied a malformed version-2 record (`plain: '{broken'`), and called `refreshStoredSession` three times: each returned `store_unavailable`, `signOut=false`, with **zero** server refresh calls. Adapter injection simulated stored corruption, not a real IndexedDB outage. The added tests use generic temporary exceptions and cannot catch this permanent case.
- Residual risk: The fix correctly preserves an actually locked Keychain, busy IndexedDB, and other unknown failures. Once readable, missing/replaced records still terminate, and real server refusal still uses the preceding commit's checks. A genuinely ended family behind permanently unreadable storage cannot be checked; repeated retry does not restore usability or prove authority. OS Keychain/Keystore fault acceptance remains required.

### 82d8403 — Abandon a server-list check that never answers so later checks can run (bug B-22.9)

- Verdict: **ACCEPT**
- What it does: Gives each version-check or list-fetch transport a separate 30-second deadline and releases both watcher and Hosted-gate ownership even when transport ignores abort.
- Findings: None requiring changes. `packages/client-core/src/portico-servers.ts:166–176` attaches rejection handlers to both raced promises; a late transport rejection is consumed by `Promise.race`. The `finally` clears the timer and caller listener for success, rejection, synchronous throw, and timeout. The watcher flight and gate key are also released on settlement. Late request completion cannot call `adopt`/`onServers`, because those are outside the bounded transport promise. Both steps can take 30 seconds individually, so a changed-version check followed by a hung list can occupy up to about 60 seconds; this is bounded, not one overall 30-second operation.
- Tests run: `npm run test:core` passes, including both added ignoring-abort transports. Inline probes exercised immediate success, synchronous throw, timeout, and a rejection delivered after timeout: **zero live deadline timers and zero unhandled rejections** after settlement.
- Residual risk: A transport that ignores its signal can retain its own resources until it settles; the race cannot forcibly terminate it. Caller cancellation only aborts the transport, so an ignoring transport still waits until the deadline and can then be classified as an outage. No new browser-only API was introduced; ordinary controllers, signals, timers, and promises are native-compatible.

### af9f250 — Honour a Retry-After given as a date in playback requests and server-list checks (bug B-22.11)

- Verdict: **ACCEPT**
- What it does: Parses trimmed seconds and future HTTP dates for Playback v1 errors, and uses the shared parser for server-list errors. Playback exposes milliseconds, while the watcher and gate retain seconds.
- Findings: None requiring changes. `playback-v1/http.ts:42–49` rounds future dates upward and retains body-seconds fallback. The local parser avoids importing the broad index back into playback internals; both transports already have appropriate date support. No retry floor is shortened on these changed paths.
- Tests run: `npm run test:core` passes. Added tests invoke the actual playback `call()` boundary, and run watcher failure through HostedGate with a long future date. Both would lose the header floor without the fix.
- Residual risk: Clock skew affects absolute dates. `playback-v1/sessions.ts:71` still converts the preparing-response header with `Number()`, so a **pre-existing separate 202 path** still mishandles HTTP dates. This is not proof that all playback Retry-After handling is fixed.

### 067b35a — Keep the account service's Retry-After on browser Portico Account errors (bug B-22.12)

- Verdict: **ACCEPT**
- What it does: Carries the response header into `BrowserAccountApi`'s `ApiError`, so callers using the shared Hosted gate retain the service's delay floor.
- Findings: None requiring changes. `web/src/bridge/account.ts:96–107` retains existing credentials, CSRF, redirect, deadline, and error handling. Header preservation is the appropriately small fix; it does not silently broaden which account adapters participate in the gate.
- Tests run: Non-writing web check components and `npm test --workspace portico-web` pass. The new test exercises the real API, then HostedGate, and observes a floor of at least 300 seconds; the missing-header case stays undefined.
- Residual risk: Auxiliary `account-api.ts`/`account-sessions-api.ts` gate participation remains outside this commit. Actual Hosted/proxy responses were not exercised.

### e6f9c63 — Cap an event-stream frame by the data it holds rather than by the size of one read (bug B-22.13)

- Verdict: **ACCEPT WITH CHANGES**
- What it does: Bounds accumulated data values across chunks and permits a large transport read containing many valid small frames. Individual lines and unfinished residual input are also checked.
- Findings:
  1. **Should fix — `packages/client-core/src/sse.ts:18,33–40`: the aggregate retained-frame cap excludes `event` and `id`.** A single frame containing 250,000-character event, id, and data values is accepted and dispatched with **750,000 retained characters**, exceeding the stated 262,144 frame cap. This is finite retention, not a new unbounded accumulation claim; replacing event/id values is bounded individually. **Recommend:** account for current event and id values, subtracting overwritten values and respecting persistent last-event-id semantics, along with data and unfinished input. Add combined-field and overwritten-field boundary fixtures. Empty data lines also have array-entry overhead; describe the cap as character/content accounting rather than an exact heap-byte guarantee.
  2. **Should fix — `packages/client-core/src/sse.ts:30–34`: split CRLF is still parsed as two line endings.** `['data: first\r', '\ndata: second\r', '\n\r', '\n']` produces two frames instead of one `first\nsecond` frame. This is **pre-existing**, but the frame-parser rewrite and new cap tests should include it: maintain a pending-CR/skip-next-LF state across reads. Otherwise chunk boundaries can spuriously dispatch/reset accounting or discard continuation fields.
- Tests run: `npm run test:core` passes, including all four new accumulation/large-read/comment/line cases. Inline production-reader probes reproduced both findings. The original repeated-small-data leak is fixed; large complete-frame reads now work. No infinite retained-data construction survived the new `held` check.
- Residual risk: One very large transport chunk is allocated before the parser can reject it, a pre-existing transport-boundary limitation. On oversized/error exit the reader releases its lock but does not itself cancel the source; callers should terminate the transport. React Native's text-stream adapter avoids constructing a TextDecoder for string chunks; this commit introduces no new browser-only API.

### 88abfbf — Treat a registry error that asks for the same request again as retryable (bug B-08.10)

- Verdict: **ACCEPT**
- What it does: Recognizes registry `retry: 'same_request'` alongside the legacy retryable boolean. Refresh/sign-in retry classes on non-5xx errors remain non-retryable through this boolean interface.
- Findings: None requiring changes. `packages/client-core/src/index.ts:398–411` preserves explicit legacy false and the established 5xx fallback; it classifies a request rather than automatically replaying mutations. The test explicitly distinguishes `after_refresh` from a same-request replay, correcting the first audit's broad claim about cursor retries.
- Tests run: `npm run test:core` passes. Added cases cover request-in-progress, rate limiting and header floor, after-refresh/never, and explicit legacy overrides. Existing API tests pass.
- Residual risk: An explicit registry retry class other than `same_request` on a 5xx still inherits the historical status fallback. No production registry route returning contradictory 5xx/never was established; future envelope unification should make that precedence explicit.

### a9ae9c8 — Reload the page when Try again follows a screen file that could not be loaded (bug B-16.9)

- Verdict: **ACCEPT**
- What it does: Recognizes the supplied Chromium/Firefox/Safari import errors and Vite CSS preload errors, then makes the explicit Try again action reload the document.
- Findings: None requiring changes. `web/src/screens/NotFound.tsx:31–46` only reloads after a user click, so an automatic reload-loop guard is not needed for this implementation. Ordinary render errors still reset the existing boundary. A new document replaces rejected module-lifetime lazy identities.
- Tests run: Non-writing web checks and 417 web tests pass, including message matching and actual retry callback behavior. These tests would invoke reset rather than reload without the fix.
- Residual risk: The manual test note is useful and names all three engines; it must ensure the destination chunk was not already loaded and actually remove old assets. Only an upgrade served in real browsers can validate the message variants and resumed route. Repeated user clicks during a continuing asset outage can still fail, appropriately without automatic looping.

### 40748d2 — Give the signed-in shell a main region and a Skip to content link (bug B-16.13)

- Verdict: **ACCEPT**
- What it does: Adds a first-focusable localized skip link and changes the routed content wrapper into a programmatically focusable main landmark, following persistent rail/top-bar controls.
- Findings: None requiring changes. `web/src/shell/Shell.tsx:123–130,174–186` focuses the persistent main directly and prevents fragment navigation. The fixed-position link does not participate in the shell grid; its visibility and z-index are appropriate. Negative tabindex allows the main to take focus without adding a regular tab stop. There is no traversal/observer work proportional to grid size: the action is one ID lookup and focus operation. The same frame is made inert under the full player, so the skip link is not an escape from the active player layer.
- Tests run: Non-writing web checks and 417 tests pass, including source-order/landmark assertions and the focus/preventDefault callback test. These are not real DOM focus tests.
- Residual risk: The lane's manual note is specific and useful: desktop/phone widths, first Tab, Enter, next Tab, unchanged URL, one main landmark with VoiceOver/NVDA. Browser sequential focus after programmatic main focus and scroll positioning still need that check; suppressing the region-wide ring must not obscure the next control's focus ring.

### d6f07cd — Name each page in the document title and announce a change of destination once (bug B-16.14)

- Verdict: **ACCEPT WITH CHANGES**
- What it does: Infers page names from main-region headings, with a Home override; updates the tab title and a polite live region after pathname changes. It coalesces body mutations into one read per 250 milliseconds.
- Findings:
  1. **Should fix — `web/src/app/page-title.tsx:23–26,44–58`: first nonempty DOM text is not verified destination metadata.** A fallback heading consumes the one announcement before the actual name arrives. For example `web/src/screens/library/Library.tsx:200` renders a generic localized library fallback before verified name/projection availability. The state machine announces that fallback, then suppresses the real name. An inline probe confirmed the same behavior for a stale name followed by a new one: first read announces, later corrected name does not. This probe establishes the state machine, not a reproduced browser navigation race. **Recommend:** supply route/destination identity, a ready/name signal, and viewer scope from the page/router; announce only the verified name belonging to the current destination. Add an effect-level test with a >250 ms delayed title and a stale/fallback title already present, rather than only helper tests. Do not rely on increasing the debounce interval.
  2. **Should fix — `web/src/app/page-title.tsx:44–66`: announcements are not fenced by viewer scope.** The root-mounted component preserves state and queued text across profile/server changes; neither the effect dependencies nor its read checks carry the active viewer fence. A11Y §4.10 explicitly requires scope fencing. **Recommend:** reset/cancel the pending announcement on scope transitions and check the originating scope at publication. Test a delayed destination read while a profile transition occurs. This is a missing explicit protection, not proof that private titles were spoken on a device.
  3. **Nit — `web/src/app/page-title.tsx:53,61–63`: unrelated body activity drives title queries.** Player clocks, scan counters, overlays, and grid churn can schedule document-wide selector work about four times per second even when the page heading is stable. The implementation does not scan the catalogue or grow a queue of timers; cost is on mounted DOM/mutation volume. **Recommend:** observe the current page-title element/heading container or publish metadata from React, and measure a large windowed grid with playback open. The comment says “after the page last changed,” but this is throttling from the first mutation, not debouncing to quietness. No measured browser jank is claimed.
- Tests run: Non-writing web checks and 417 tests pass. Inline `pageTitleStep` probe confirms the first-name consumption described above. The new test suite never mounts `PageTitle` with a MutationObserver or exercises effect cleanup/scope changes.
- Residual risk: Real browsers and assistive technology must verify exactly one meaningful navigation announcement and tab history titles. The lane's manual note calls out profile switching and large-grid jank, but these were not exercised. The title read excludes portalled dialogs and the player in ordinary layout; that alone is not a viewer fence.

### 78243e2 — Include watched and progress state in a card's accessible name (bug B-16.15)

- Verdict: **ACCEPT**
- What it does: Adds localized watched/progress state to the artwork focus target's accessible name and hides the visual watched tick from assistive traversal.
- Findings: None requiring changes. `web/src/ui/Card.tsx:38–49,103,112` keeps the visible title at the beginning for voice-control name matching and uses the existing locale percent formatter. Positive progress is clamped to 100%; zero/absent state leaves the original title. It describes current rendered state rather than changing watched/rewatch policy, so it does not decide D-61. Rendering costs only a couple of short string formats per rendered card, and React.memo remains intact.
- Tests run: Non-writing web checks and 417 tests pass, including actual callback/rendered-prop tests for unwatched, watched, progress, both states, zero, and over-range progress. Localized catalogue additions pass strings/glossary checks.
- Residual risk: Screen-reader pronunciation and duplicate reading need VoiceOver/NVDA as the note specifies. Other caption and unavailable-state semantics remain pre-existing accessibility work; this does not claim to fix them.

### cdc964a — Leave browser shortcuts alone in the player when Ctrl, Cmd or Alt is held (bug B-17.9)

- Verdict: **ACCEPT**
- What it does: Filters modified/composing keystrokes before revealing player chrome or acting, and normalizes single-character letter case.
- Findings: None requiring changes. `web/src/player/shortcut-key.ts:7–13` and `Player.tsx:216–223` preserve prior field/menu/dialog exclusions and plain-key transport behavior. Cmd stays excluded even with AltGraph; Ctrl/Alt reported solely as AltGraph retains typed characters. The browser-only KeyboardEvent type is confined to web code, not shared client-core.
- Tests run: Non-writing web checks and 417 tests pass; new helper tests cover all modifier families, composition, Caps Lock-style case, arrows/Escape, and AltGraph, and assert integration before `reveal()`.
- Residual risk: Windows/macOS/browser reserved-key delivery and international layout behavior need the supplied manual matrix. Source assertions supplement helper tests but cannot establish browser shortcut dispatch.

### 92ecf29 — Select the episode group that was chosen on a show with hidden seasons or ranges (bug B-18.1)

- Verdict: **ACCEPT**
- What it does: Forwards the ID selected by the group's tabs/dropdown to the shared workspace and catches synchronous validation as well as asynchronous rejection.
- Findings: None requiring changes. `web/src/app/detail.ts:59` now matches both `Show.tsx` consumers and the service's server-provided group validation. It does not fabricate a group or loosen validation. Ignoring invalid-group errors follows the hook's existing selection handling; visible selection-error presentation could improve independently.
- Tests run: Non-writing web checks and 417 tests pass. Added tests exercise the actual hook callback for three IDs and a synchronously rejected group. The old implementation would select the wrong ID and let a synchronous error escape.
- Residual risk: Run the report's real-show hidden-seasons/ranges check, including the compact dropdown, to verify service results and episode presentation. No server pagination/grouping behavior was changed.

### e30294b — Save a Tags or Labels edit when the field is already locked (bug B-18.2)

- Verdict: **ACCEPT**
- What it does: Counts list-valued edits as pending changes even when neither the scalar value nor locked flag changed.
- Findings: None requiring changes. `web/src/screens/shared/MetadataEditor.tsx:195–202` now matches validation/save assembly, including empty arrays (explicit clearing). Exporting GeneralTab is a small test seam, not a product behavior change. Existing list parsing, field limits, permissions, and lock handling remain intact.
- Tests run: Non-writing web checks and 417 tests pass. The new harness test edits an already locked list, observes Save becoming available, invokes it, and asserts the exact list-only payload; it would fail for the original defect. Existing metadata validation/bulk tests also pass.
- Residual risk: UI and server receipt handling still need a real edit. The report's one-line browser note is weaker than the other notes: a cold tester should edit and save Tags/Labels twice, reopen after each save, then clear the list while it stays locked; pass requires all three server-backed readbacks to match.

### e5b18fb — Run the preference-consumer check again instead of skipping it (bug B-18.11)

- Verdict: **ACCEPT**
- What it does: Looks for maintained native source, always checks this repository, rejects an explicitly invalid native path, and removes obsolete exceptions. Importing the checker no longer exits the test process early.
- Findings:
  1. **Nit — `packages/client-core/test/preference-consumers.test.ts:95–103`: the native-path fixture proves directory acceptance, not actual reading.** It creates an empty native source directory and passes because every current key already has another match. **Recommend:** add a fixture whose only consumer is in native source and assert its recorded reader; retain the invalid-path failure case. This does not invalidate the confirmed restoration of the real registry check.
  2. **Nit — `packages/client-core/test/preference-consumers.test.ts:78–101`: new test names retain `X-01` audit/workstream labels.** The implementation brief prohibits audit IDs in added tests/comments. Remove those prefixes from the newly added names; old names need not be broadly rewritten.
- Tests run: `npm run test:core` passes, including all seven preference-consumer tests (previously import exited before they ran). Checker passes both default worktree discovery and explicit actual native checkout: 56 keys, zero exceptions. Web checks pass.
- Residual risk: This remains a lexical safety net, not proof each setting has an operational effect. Existing `settings-structure.ts`, i18n lines, and `operations/preference_consumers.go` can count key registration/default/projection as reading. That weakness predates the fix and is candidly reported; strengthening it is separate work rather than justification to leave the check disabled. No CI-native checkout requirement was identified that this commit silently removes.

### bfcd24d — Keep casting on the TV when the last sender disconnects (bug B-13.7)

- Verdict: **ACCEPT WITH CHANGES**
- What it does: Detaches the final sender/controller without queuing Stop, leaving receiver-owned reporting, token renewal, and playback running. Normal Stop, title completion, and explicit server-ended responses can still close the playback session.
- Findings:
  1. **Should fix — `server/receiver/cast/receiver.js:645`: natural completion cleanup depends on a successful final timeline report.** If `report('ended')` throws, `stopPlayback(true)` never runs and the outer catch swallows the failure. Once the final sender is gone there is no disconnect Stop to clear that abandoned playback object; the reporting loop can continue and the UI stays playing. An in-memory adaptation of the existing harness made timeline requests fail after disconnect, set CAF to IDLE, and emitted MEDIA_FINISHED: **playbackId remained `ps_1`, UI remained playing, and no DELETE occurred**. This is a **pre-existing completion error path newly exposed by retaining unattended playback**, not a regression on the successful finish path. **Recommend:** put local teardown/Stop cleanup in a `finally` independent of final progress reporting, preserve any retryable remote cleanup receipt as appropriate, and test failed final reporting with no sender.
  2. **Should fix — `server/receiver/cast/receiver.js:203–208,437–449`: verified terminal pairing/authorization failure can retry indefinitely.** The report loop ends playback for 404/410/session_ended, but a rejected receiver reconnect (e.g. a permanently revoked device credential returning 401) only increments retry failures and schedules another report. The sender no longer supplies an automatic exit. **Recommend:** classify confirmed receiver revocation/terminal expiry after renewal failure and stop locally, while preserving transport/5xx uncertainty; add a terminal reconnect fixture. A single expired access bearer that successfully renews must continue to work.
- Tests run: `npm run test:cast` passes 16/16, including both new disconnection tests and existing token renewal. The successful finish and remote Stop paths are covered. The additional failed-final-report harness probe reproduces finding 1; finding 2 is source-traced, not a hardware reproduction. No live Chromecast or service was contacted.
- Residual risk: With a healthy server and finite title, **MEDIA_FINISHED followed by successful cleanup**, explicit remote/authorized Stop, or 404/410/session_ended ends unattended playback. Reporting renews the server lease every 10 seconds (30 paused), so sender absence by itself does not expire it. Paused playback and live media have no natural finish; no new absence/passout timeout is implemented. Do not introduce one as a substitute for terminal cleanup or silently decide product lifecycle policy. CAF's default idle timeout is not represented by the test harness. The supplied physical ten-minute test is sensible but must also include final-report failure and revoked credentials. Re-pair stopping the film remains a separate pre-existing reconnect-contract gap.

### 92d46e0 — Let a Watch Together follower's seek finish before correcting its position again (bug B-13.2)

- Verdict: **REJECT**
- What it does: Adds an optional shared seeking flag, supplies it from the web adapter, and suppresses corrective seeks/rate changes while that flag is true. Playback seek success or its 15-second timeout clears the web flag.
- Findings:
  1. **Blocker — `packages/client-core/src/group-session.ts:354–356`: finishing one slow seek can immediately start the next forever.** A moving group advances during the seek; on completion the player lands at the old target and is again beyond the 3-second band. With deterministic eight-second seeks, a production GroupFollower probe issued targets **30, 38, 46, 54, 62, 70 at 8-second intervals**. No overlapping seek survives, but the player spends the whole interval seeking and never becomes synchronized. This is the post-completion mechanism also documented as B-13.3, not a claim that this commit created it or that every actual engine lands this way. It matters to the requested B-13.2 slow-seek recovery. The new test deliberately ends after asserting a second seek, so cannot establish convergence. **Recommend:** correlate completion and apply a bounded local settling/catch-up policy that demonstrates eventual convergence under repeated stable seek latency (for example latency-aware targeting or sustained bounded rate recovery rather than immediately hard-seeking again). Add a 30–60-second fake-clock test with several slow completions and measured residual drift; preserve host authority and avoid a global room-pause decision under D-45.
  2. **Blocker (integration) — `/home/justin/projects/Portico/apps/portico-react-native/apps/native/src/app/together.tsx:32`: the native video adapter still omits `seeking`.** I independently confirmed the lane's stated follow-up. Mirroring this shared package alone leaves native on the old restart-while-pending behavior because the optional flag defaults to absent/false. **Recommend:** coordinate the one-line `seeking: !!s.pendingSeek` app-adapter change in the native lane before calling B-13.2 fixed on supported platforms, and test native ordinary video with a seek over three seconds. The read-only reviewer must not make that change, and the implementation lane correctly avoided editing the native shared-package copy.
  3. **Should fix — `packages/client-core/src/group-session.ts:345–358`: buffering itself still permits corrective feedback.** With `ready=true`, `buffering=true`, and no pendingSeek, seek/rate actions continue. The second review and social contract §13.6 explicitly suppress both while buffering. Include buffering in the suppression guard and report buffering/settling readiness consistently; test recovery before accepting it as a complete follower fix. This is an existing adjacent gap, not a new regression.
- Tests run: `npm run test:core` and non-writing web checks pass. The new test proves one pending seek is not superseded and corrections resume after completion; it does not prove resumed corrections converge. The independent repeated-eight-second-completion probe demonstrates the nonconvergent logic under an adapter behavior allowed by the current contract. Source review confirms `index.ts:1336,1364,1380` clears pendingSeek on successful correlated completion or timeout, so the new flag itself is not permanently latched on the ordinary web video path.
- Residual risk: Device acceptance must record target, actual landing position, seek revisions, readiness, and drift after several completions. The lane's “one revision per host seek” pass criterion is inconsistent with its test, which expects a subsequent corrective revision after a single completion; revise the note to distinguish legitimate convergent correction from endless correction. Native listening uses a different seek owner and needs separate capability evidence; the one-line native adapter addition is specifically necessary for ordinary video.

### d2723d7 — Back off activation polling from 5 to 30 seconds with jitter when the service cannot be reached (bug B-22.26)

- Verdict: **ACCEPT WITH CHANGES**
- What it does: Shares an exponential retry helper between create failures and unanswered polls, resets poll failure count on protocol answers/new codes, and preserves larger service Retry-After floors. Normal pending/slow-down polling remains protocol-owned.
- Findings:
  1. **Should fix — `packages/client-core/src/setup-code.ts:90–91`: jitter disappears entirely at the cap.** From failure four onward the pre-cap base is 40 seconds, so `min(30s, base * positiveJitter)` is always exactly 30 seconds. An inline probe with random 0 and 0.999 produced different early waits but **identical 30,000 ms capped waits**. A sustained activation outage therefore loses per-attempt spreading at its steady retry cadence. **Recommend:** choose a jittered interval within an explicit capped range (e.g. a randomized upper band ending at 30 seconds), while honoring larger Retry-After as a floor, and test entropy after reaching the cap. Networking §10 calls for exponential backoff *with jitter*, not just early-attempt jitter.
  2. **Should fix (completion of B-22.26) — `packages/client-core/src/setup-code.ts:160–197` and native `src/bridge/setup-codes.ts:25–33`: Hosted activation still bypasses the shared origin outage gate.** This commit fixes cadence only, as reported, but the audit's confirmed gate bypass remains. Networking §10 already requires shared circuit state; deciding whether to report/obey that gate is not an identified open D-number product decision. **Recommend:** a follow-up at the Hosted adapter seam so automatic create/poll/cleanup report and obey the appropriate origin gate without gating direct local activation or interactive manual code approval. Preserve valid displayed codes, exact operation IDs, and protocol intervals. If deliberately deferred, mark this bug partially fixed rather than complete.
- Tests run: `npm run test:core` passes all added fake-clock cadence/reset/create tests. Inline helper probe confirms the capped-jitter issue. Added tests expect 30,30 and therefore encode rather than detect loss of jitter. Error, envelope, durable-journal, cancellation and key checks remain unchanged; no new native-incompatible browser API was added.
- Residual risk: Current protocol-error Retry-After handling, cancellation of in-flight work, and terminal/identity classification have older gaps outside this cadence change; do not infer complete networking conformance from these tests. Actual Hosted outage/foreground/background/manual-entry interaction still requires the adapter-level integration check. No Hosted endpoint was used.

## Skipped items

### B-18.4 — Genres and Series stop at 200

**Skip justified for this lane; defect remains open.** I traced `server/internal/catalog/browse_engine.go:968–988`, `browse_contract.go:25`, and the client Genres request. Aggregate browsing bypasses ordinary paging, fetches at most `browseFacetLimit=200` facet values, reports the returned length as total, and supplies no continuation. Sending another client cursor or a larger requested limit cannot retrieve the missing values. Simply removing the cap would turn this into unbounded server/client work and violate the pass's resource guardrails.

The report should say this needs coordinated **server aggregate counts/continuation plus client consumption**, rather than imply a product choice or impossibility. No schema migration or owner decision was established as necessary. A focused server/client follow-up can safely solve it; it was appropriate not to improvise server pagination inside the web lane or fabricate completeness. Test with more than 200 distinct music genres and audiobook series, checking stable order, total, all pages, and bounded work. I did not run a server on catalogue data.

## Cross-commit issues

1. **Sign-in classification:** 060737e preserves unrelated 403s while still ending known schedule revocation; dc5d0e6 preserves all adapter exceptions. The latter must gain local corruption recovery, not be “fixed” by treating every unreadable store as a server revocation or by reverting transient preservation. Browser and native adapters must expose consistent outcomes. The existing status-only 401 provenance problem is separate and still unresolved.
2. **Shared packages/native integration:** The lane correctly changes the canonical server shared packages rather than their native copy. After review corrections, mirror `group-session.ts`, `index.ts`, `playback-v1/http.ts`, `portico-servers.ts`, `server-connections.ts`, `setup-code.ts`, `sse.ts`, and the changed i18n catalogues through the established shared-package workflow. Mirroring alone does not add `seeking` to native `apps/native/src/app/together.tsx`. No newly introduced `window`, `document`, `navigator`, MutationObserver, KeyboardEvent, or AbortSignal.any/timeout dependency was found under changed `packages/client-core/src`; browser-specific changes stay under web. Existing fallback TextDecoder/Web Crypto assumptions in core are pre-existing and adapters still matter.
3. **Follower completion is not convergence:** The in-flight guard addresses B-13.2's cancellation mechanism, but exposes/retains the post-completion B-13.3 cycle. The latter was independently recorded as an unsettled design risk, and this review's fake-adapter probe establishes a permitted deterministic nonconvergent case, not real-device frequency. Repair locally without deciding D-45's global wait/host-role policy. The social-downloads lane report also flags end-of-title target clamping/readiness as unfinished client work; this lane's guard does not address that.
4. **Accessibility:** The main landmark makes page-name discovery possible, but pathname alone does not authenticate a DOM name or fence a profile's announcement. Coalescing mutations prevents timer accumulation; it does not prove large-grid or player performance. The card change describes displayed watched/progress state and leaves D-61 untouched.
5. **Hosted retry behavior:** Header fixes retain service floors in the APIs/gate, and watcher deadlines correctly report unavailable failures. Activation cadence still uses its own clock/gate bypass and loses jitter at the cap; all of these need testing together at the adapter boundary without contacting production Hosted in this review. Shared circuit participation is already required by networking §10, not an identified new owner decision.
6. **Cast lifecycle:** Last-sender detachment now correctly leaves the receiver autonomous. That autonomy also removes the old disconnect cleanup fallback for failed completion and terminal reconnect paths; those require tests and local teardown. Do not add an arbitrary sender-absence timeout or globally alter passout/lifecycle behavior as a replacement for this bug fix. The pre-existing re-pair path stops active playback, contrary to the reconnect contract; it remains separate work.
7. **Merge surface:** I compared changed-file sets against all available `audit-fixes/*` branches in this server repository. **No changed-file intersections** with the seven other server lane branches were found at the reviewed heads. That is not a trial merge or semantic integration test. The native adapter and package synchronization are still required cross-repository coordination. The report's follow-up warning is accurate, and the visible native lane report does not list the `together.tsx` fix as implemented.
8. **Hygiene and review scope:** All 18 commits have plain behavioral subjects and Justin's identity; changed files match their described fixes, and no schema, dependency, persistence migration, or unrelated native shared-copy edits were found. Some added preference-check test labels retain historical workstream IDs (noted as a nit). Repository was clean before/after independent checks. No fixes, commits, publishes, or service operations were performed.

## Summary table

| Commit | Bug | Verdict | Blockers |
| --- | --- | --- | --- |
| 060737e | B-01.9 | ACCEPT | — |
| dc5d0e6 | B-22.4 | REJECT | Permanent corruption is classified as retryable forever, with no usable recovery path |
| 82d8403 | B-22.9 | ACCEPT | — |
| af9f250 | B-22.11 | ACCEPT | — |
| 067b35a | B-22.12 | ACCEPT | — |
| e6f9c63 | B-22.13 | ACCEPT WITH CHANGES | — |
| 88abfbf | B-08.10 | ACCEPT | — |
| a9ae9c8 | B-16.9 | ACCEPT | — |
| 40748d2 | B-16.13 | ACCEPT | — |
| d6f07cd | B-16.14 | ACCEPT WITH CHANGES | — |
| 78243e2 | B-16.15 | ACCEPT | — |
| cdc964a | B-17.9 | ACCEPT | — |
| 92ecf29 | B-18.1 | ACCEPT | — |
| e30294b | B-18.2 | ACCEPT | — |
| e5b18fb | B-18.11 | ACCEPT | — |
| bfcd24d | B-13.7 | ACCEPT WITH CHANGES | — |
| 92d46e0 | B-13.2 | REJECT | Repeated slow completions need not converge; native video adapter lacks seeking flag |
| d2723d7 | B-22.26 | ACCEPT WITH CHANGES | — |

**Totals: 12 ACCEPT, 4 ACCEPT WITH CHANGES, 2 REJECT; three blocker findings across two commits. One skipped ID, justified within lane scope.**

### Owner decisions

None of the requested correction paths requires a new product decision based on evidence reviewed. Preserve the existing gates: D-45 governs any global Watch Together wait/pause policy; D-39 governs schedule-policy changes; D-61 governs rewatch semantics. This pass must not quietly choose them. A new sender-absence limit for paused/live Cast sessions would be a distinct lifecycle policy discussion, not the recommended fix for completion/terminal cleanup. The native integration change, corruption recovery classification, verified destination metadata/fencing, retry jitter, and aggregate pagination follow-up can be made concrete without asking Justin to resolve those open product choices first.
