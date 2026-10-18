# Outcome

Completed the independent native/Vega JavaScript review and repairs. **18 confirmed findings repaired: 7 high, 10 medium, 1 low; no critical findings.** Each finding has a regression demonstrated failing before repair and passing afterward. Native passes **547/547** tests (514 at the supplied starting point); Vega passes **72/72** (64 at the starting point). Both application and test TypeScript configurations and both string lints pass.

Delayed play checks, download lookups and abandoned sign-in/restore work cannot replace a newer intent. Credential rotation no longer retires its own server family. Cached title identities and account-directory requests remain with their API/viewer. Offline observations remain ordered and are removed only when acknowledged; continued housekeeping retries deferred progress/next-episode work and renews receipts. Enrollment stays bounded, viewer-scoped and retryable. Changed grants reach the downloader. Background paused/ended playback releases the event feed. Vega obeys seek/rate barriers, rejects stale commands, verifies actual landed positions and settles an opening stream on cancellation. Diagnostics remove credentials and grant addresses before storage, console output, overlays or sinks. The hook driver follows replacement external stores.

This is source and unit-test evidence, not device/build acceptance or release parity. Vega's pre-existing authenticated-transport restriction remains a release gap.

# Decisions I made

- Work ran only on personal Ubuntu `ehler-homelab-mini-1` in the assigned native worktree and programme report/snapshot directories. Context sync could not write its lock; canonical project, standards and verification guidance were read directly. The replacement shared guidance supplied during work was retained. No install, helper, agent, commit, push, deployment, Xcode/Gradle build or full-repository test was performed.
- Preserve the original playback intent across asynchronous boundaries. A failed queue start does not create a second queue/play mutation: the real shared queue player owns failure presentation and recovery. No compatibility fallback is retained.
- Reserve a download play intent before its asynchronous local-file lookup. Superseded lookups may finish reading, but cannot start playback.
- Candidate generation fences cover the entire sign-in/restore workflow, not just the shared commit's verification stage. Use another preserves its saved credential; stale rollback cannot republish a viewer. Same-server/family credential rotation is never retirement.
- Keep the 5,000-member enrollment backlog and 50-page pass. Count every first page, retain a partially processed page, scope continuations by owner/request, and retry a transient failure after 30 seconds even with unchanged counts. Native wake/minute maintenance provides retry triggers.
- A changed download grant is dispatched through the existing native start/resume contract only while the same viewer, token and unheld transfer remain current. A receipt's artifact digest/size must still match. Same-token renewal does not restart the transfer. Platform-specific resume limitations remain those described below.
- A progress receipt settles only its matching play/item pairs and the exact observation sent. Incomplete answers end that drain rather than loop. Same-title observations receive strictly ordered millisecond timestamps, including pending stored observations, using a bounded 400-title in-memory clock map. The integrated server's <= millisecond comparison was read to verify this need; server clock/ordering policy was not changed.
- Coalesce receipt renewal for a viewer/generation. A forced access-withdrawal check waits for an ordinary renewal then still runs; periodic failure leaves work eligible for a later pass. Existing settings and permission/signature validation remain authoritative.
- Vega stays paused until JavaScript retires a seek, and cannot run at an unsupported rate. Retained-position restoration is checked within half a second and bounded to ten seconds; failure keeps the session and reports the contract's existing words.
- Arbitrary bridge strings are traced by length, never prefixes. Redaction happens before truncation or transmission. Existing sandboxed Vega storage and plain HTTP policy are preserved.
- No screen appearance was judged or changed. PlayerHost has only the intent-ordering call change. No new catalogue strings or settings were needed.

# Bugs

Phase 1 findings (original lines, recorded before repairs):

| Finding | Severity | Original location | What is wrong / evidence | Smallest fix |
|---|---|---|---|---|
| Offline progress acknowledgements lose newer or unreadable observations | High | `apps/native/src/bridge/offline-downloads.ts:854–886` | Deletion compares only millisecond timestamps, so a later observation in the same millisecond is erased. All submitted entries are deleted even when the tolerant receipt parser dropped a result. | Match the exact submitted observation and acknowledged play/item pair; order same-millisecond title observations strictly; stop after incomplete answers. The integrated server uses <= millisecond ordering, confirmed in its progress writer. |
| Transfer grant renewal does not update the downloader and is not viewer-fenced | Medium | `apps/native/src/bridge/offline-downloads.ts:626–632` | A new token is only patched into metadata. Native keeps the URL passed to `start`. The asynchronous grant callback patches after disconnect/profile changes. | Fence the answer, and hand a changed grant back through native start while preserving artifact identity. |
| Container enrollment stops forever after transient failure | Medium | `apps/native/src/bridge/offline-downloads.ts:477–483` | Catch marks the page as a completed tail; unchanged counts suppress every retry. Existing test explicitly requires the broken behaviour. N-V08.1 remains present. | Track a retry floor separately from completed tails; wake retries unchanged requests without looping. |
| Enrollment backlog is unbounded across requests | Medium | `apps/native/src/bridge/offline-downloads.ts:432–442` | Every request's first page bypasses ENROLL_BACKLOG. Enough requests overflow native's bounded state and cannot survive restart. N-V08.1 remains present. | Apply the bound to every enrollment; do not advance a partially enrolled page. |
| Enrollment conflates viewers and servers | High | `apps/native/src/bridge/offline-downloads.ts:439–448` | Another profile's copy suppresses a wanted title; continuation state is keyed only by request ID, reused across viewer/server connections. | Scope continuation state to owner/request and compare existing files only under the owner. |
| Stale play checks and queue fallbacks can replace newer playback | High | `apps/native/src/player/usePlayer.ts:689–778` | Offline play and channel tune never increment the preflight intent. Both queue-start catch handlers call startSingle unconditionally after a newer play or leave; this would also create a second mutation on an uncertain queue response. PlayerHost awaits localMedia before issuing the download intent, allowing that delayed lookup to replace a newer play. A previous Dolby confirmation also remains actionable. | One latest-intent check for every start, fallback and confirmation; invalidate on cleanup. |
| Vega runs while a seek is still pending or rate is unsupported | Medium | `apps/vega/src/bridge/playback.ts:261–286` | apply() only tests wantPaused/away; it calls play with pendingSeek, in-flight seek or rate != 1. The existing seek test only checks facts, not paused state. | Hold actual playback until seek retirement and supported rate. |
| Vega accepts stale bindings and invalid/retired seeks | Medium | `apps/vega/src/bridge/playback.ts:435–451` | refusal() ends after route checks; it omits the contract's stale-binding and seek rules. Invalid positions reach element time, older intents can replace the current item. | Add ordered binding/seek checks, including changed-route exception and retired revision rejection. |
| Cancelling Vega SSE before headers never settles opening | Medium | `apps/vega/src/bridge/event-stream.ts:145–158` | cancel stores no opening reject; it clears open timeout and removes the stream while openNetworkStream's promise remains pending. Background/route cancellation can hold reconnect indefinitely. | Retain and reject the pending open; settle once before abort and ignore stale callbacks. |
| Diagnostic traces can disclose credentials and grant addresses | High | `apps/native/src/app/debug.ts:29–39`; `apps/vega/src/debug.tsx:38–44,56,82–94` | Native trace prints prefixes of string args/results. Vega Error messages, warning strings and stacks go to overlay/sink without redaction; ordinary console also sees raw originals. ErrorJournal only catches long opaque values and misses named shorter secrets. | Redact at each trace boundary before truncation; describe arbitrary bridge strings by type/length; redact named secrets and capability paths. |
| Hook test driver keeps the old store subscription | Low | `apps/native/test/hook-driver.ts:116–124` | useSyncExternalStore subscribes only when no unsubscribe exists; replacing subscribe leaves listeners on the previous service. Scope replacement tests can pass while ignoring actual subscriptions. | Track subscribe identity; unsubscribe/resubscribe on replacement and test actual old/new notifications. |

Additional Phase 1 finding, before repair:

- **High — a slow saved-session restore can sign in again after Sign out or Use another.** `apps/native/src/app/session.tsx:505–514,1019–1042`. Restore starts before the shared selection commit exists; Sign out clears that commit, but does not invalidate the pending restore. Its late result then starts a new commit and publishes the old viewer. Profile selection and Quick Connect similarly omit a generation fence around their final awaits. Smallest repair: bind the entire candidate workflow to the existing sign-in generation, invalidate on exit/cancel, pass a candidate fence into the commit, and suppress stale rollback publication. Add actual provider-hook regressions for a held restore and profile selection.

- **Medium — cached episode/song identity crosses API/viewer lifetimes.** `apps/native/src/player/usePlayer.ts:89,639–651`. The module-global extraTitles map is keyed only by item ID; another server with the same ID receives the previous server's show/artist without rereading. Smallest repair: a bounded cache per API identity, held weakly, and a two-hook same-ID/different-server regression.

- **High — directory work can fetch under a replacement account.** `apps/native/src/bridge/useAccountDirectory.ts:35–40`. listServers awaits accessSession without checking that its account matches the owner of the watcher/cache; an old directory read can therefore send the replacement account's bearer under the previous account's workflow. Smallest repair: check account ID and cancellation immediately after that await and before the request; exercise the real hook with a held credential lookup and an account switch.

- **Medium — Vega claims seeks and route-position restoration succeeded without checking where it landed.** `apps/vega/src/bridge/playback.ts:245–255,349–358`. loaded() swallows a retained-position seek failure and publishes ready; sought(true) trusts seeked without comparing the requested and observed positions. This breaks the half-second engine contract and can restart a resumed title from the beginning. Smallest repair: check actual landed position, hold readiness during a bounded retained-position restoration, and fail while preserving the session if restoration cannot be achieved. Regressions inject a clamping media element.

- **High — verifying a rotated saved session asks to revoke its own family.** `apps/native/src/app/session.tsx:469–475`, `packages/client-core/src/viewer-selection.ts:83–87` (read-only). The shared commit compares access tokens to retire the previous sign-in; a refreshed token in the same family differs, and the native revoke callback unconditionally calls DELETE /v1/sessions/current for the previous record. This is a family-revocation request against the newly selected sign-in. Smallest native fix: omit retirement within the same server/family, including failed restore candidates from that established family; add a successful-restore regression asserting no logout. Shared owner must make the commit's retirement decision by family identity rather than bearer value.

- **Medium — a paused or ended title keeps the background event feed running indefinitely.** `apps/native/src/bridge/useNativePlayback.ts:117`. busy is itemId presence, which remains true for paused/error/post-play screens. This contradicts the report's "while playing" policy and bypasses the 30-second background grace. Smallest repair: classify busy from playing intent plus an active starting/ready phase, and drive runEventFeed through background, pause, resume and terminal transitions. The old predicate is extracted unchanged for the failing regression before replacement.

- **Medium — ongoing download housekeeping never retries deferred viewing/auto-next or daily receipt renewal.** `apps/native/src/bridge/offline-downloads.ts:283–300,307,610`. These tasks only run on connect/final progress; foreground/network wake and the minute interval run tend(), which handles only transfers/bundles. A failed upload or next-episode request therefore stays pending after the network returns, and a continually connected app eventually expires receipts. Smallest repair: run all owner-scoped maintenance from tend, coalesce receipt renewals and preserve forced permission checks; test network wake and the next renewal day.

Automatic-quality policy and focus ownership have been inspected; no further confirmed finding is recorded there. These findings are source evidence; regressions and repairs follow in Phase 2.


## Repair status and regression evidence

All 18 findings above are **fixed in this lane**. Original locations refer to the starting tree. Logs below contain the actual failing assertions, not merely import/type errors.

| Findings | Regression evidence |
|---|---|
| Progress acknowledgement/ordering | `offline-ownership.test.ts`: same-millisecond replacement, incomplete receipt, and server-style stale timestamp ordering. `offline-before.log`, `progress-order-before.log`; green in final suite. |
| Transfer renewal/address ownership | `offline-ownership.test.ts`: changed address dispatched; old-generation grant cannot patch/start. Eight combined offline/enrollment regressions failed in `offline-before.log`. |
| Enrollment failure, bound, viewer isolation | `container-enrollment.test.ts`: unchanged-count recovery after the floor, 70 first pages bounded at 5,000, foreign copy does not suppress, repeated request IDs isolated across viewers. Existing test wording corrected; its immediate-retry assertion still verifies the floor. |
| Play checks, fallbacks and download lookup | Real `usePlayerEngine` with runtime boundaries stubbed: five stale start cases, uncertain queue response, late local lookup; `player-before.log` and `player-extra-before.log`. |
| Cached identity | Two real player-hook instances with the same item ID and different API identities; `identity-before.log`. |
| Sign-in/restore candidate fencing | Real SessionProvider and shared ViewerSelectionCommit: five abandoned restores, sign-out/Use-another during verification, and a positive current restore; `session-before.log`, `session-after.log`. |
| Same-family retirement | Positive verified rotation previously requested logout of the old token; `family-before.log`. Final positive regression asserts no retirement request. |
| Directory account ownership | Real directory hook with held credential lookup, then replacement account/disabled directory; `directory-before.log`. No old-owner bearer dispatch after repair. |
| Vega seek/rate/binding rules | Real engine against SDK media stand-ins; five original regressions in `vega-before.log`, plus clamped seek/route restoration in `vega-position-before.log`. |
| Vega opening cancellation | Real stream/XHR boundary: cancellation before headers rejects, and the retired callback cannot affect a replacement stream; `vega-before.log`. |
| Diagnostics | Real native trace/journal and real Vega debug module (existing TypeScript transpiler only for JSX): shorter named secrets, bridge string prefixes, capability paths, console/sink output; `debug-native-before.log`, `debug-vega-before.log`. |
| Hook driver subscriptions | A replacement external store removes the old listener, reacts to the new one and unsubscribes on unmount; `hook-before.log`. |
| Background feed policy | Original item-presence predicate extracted unchanged, then replaced after two failing lifecycle regressions; `feed-before.log`. Real runEventFeed stops after background pause/end and restarts for playback. |
| Ongoing offline maintenance | Network wake drains deferred progress/next episode without reconnect; next-day receipt renewal coalesces overlapping wakes; `maintenance-before.log`, `maintenance-after.log`. |

No assertions were weakened to make an existing failure disappear. Tests use synthetic tokens/receipts only. Hook tests exercise production hooks and shared services through the existing dispatcher driver, with authentication/device/UI boundaries stubbed. They are not React renderer, scheduler, native-module or server acceptance tests.

## Background register dispositions

The lane brief assigns supporting reports rather than a separate numbered audit domain. Documents 14 and 19–22 and the fix log/register were consulted for context. The following are individual dispositions for their entries; "already fixed" is source-level evidence from the maintained consumer/report, not renewed hardware/server acceptance. Server-only and native-language items are outside this worktree's permitted JavaScript repair scope. This is not a claim that every background audit item is closed.

| ID | Disposition |
|---|---|
| B-14.1 | Already fixed: copies are removed only after owner-scoped access withdrawal, not server retention. |
| B-14.2 | Client renewal fixed further here; Android resume is native-owned. iOS changed-URL partial resume remains a native limitation. |
| B-14.3 | Fixed further here: grant renewal reaches the downloader and respects viewer/hold/artifact fences; 24-hour grant policy is server-owned. |
| B-14.4 | Already fixed upstream; consumer uses server-authored producible qualities. |
| B-14.5 | Server-only; reported fixed upstream with originals served in place. No server build/read-through claimed. |
| B-14.6 | Already fixed: receipt is requested before native start. |
| B-14.7 | Already fixed: bundle/title metadata and local artwork drive device rows. |
| B-14.8 | Already fixed: stable viewer-scoped service and device request following; enrollment recovery strengthened here. |
| B-14.9 | Fixed further: daily batched renewal is now also maintained while continuously connected, with single-flight. |
| B-14.10 | Fixed further: observations and acknowledgements remain ordered; canonical server personal-state writer is upstream. |
| B-14.11 | Already fixed upstream; client carries later-part observations in milliseconds. |
| B-14.12 | Already fixed on the base: offline engine choice and original-file note exist; device codec acceptance remains. |
| B-14.13 | Already fixed upstream/shared consumer: revision belongs to state, with bounded explicit-conflict recovery. |
| B-14.14 | Already fixed for removal/reclaim controls; keeping files at sign-out and device-wide settings is the recorded decision. |
| B-14.15 | Server-only; reported fixed upstream. Device copy retention is preserved. |
| B-14.16 | Already fixed in recover(): unknown receipts/claims reissue without deleting the held copy. |
| B-14.17 | Server-only; reported fixed upstream. Native now retries failed continuation pages independently of counts. |
| B-14.18 | Native-language/device acceptance item; prior native repair not recompiled here. |
| B-14.19 | Native-language task reconciliation item; prior repair not exercised on an OS downloader here. |
| B-14.20 | Client tending is repaired; native background lifetime/process-death acceptance remains with Android owner. |
| B-14.21 | Not changed: wall-clock receipts and cross-device observation clocks remain the recorded non-DRM policy; no clock enforcement added. |
| B-14.22 | Fixed further here: server acknowledgements/renewal update resume only without newer local work; ordered observations are retained. |
| B-14.23 | Already fixed in upstream claim restart and current client feedback. |
| B-14.24 | Server-only housekeeping; outside this lane. |
| B-14.25 | Server-only worker transactions; outside this lane. |
| B-14.26 | Already fixed upstream: versions selected, multi-part refusal explained; no new native selector. |
| B-14.27 | Mixed server/native small entries remain with owners; JS receipt/revocation/renewal contracts and bounds preserved. |
| B-19.1 | Already fixed: one app-level pending-link owner, fencing and duplicate coalescing remain; link tests pass. |
| B-19.2 | Already fixed: stable viewer scopes survive token renewal; API identity cache repaired further here. |
| B-19.3 | Already fixed: independent Menu intent and focused action ref; actual remote acceptance remains. |
| B-19.4 | Already fixed: double-look key-loss/reset flow and native declarations retained; secure-record tests pass. |
| B-19.5 | Already fixed for bounded retry/failure state. Retry-URL platform cache duplication remains a shared/native efficiency follow-up. |
| B-19.6 | Fixed further here: bounded journal/global capture exists and diagnostics redact named short secrets/capability paths. |
| B-19.7 | Native plist/upload item; prior orientation fix not archived here. |
| B-19.8 | Not fixed here: native density/rotation/internal-flag behaviour requires Android owner and hardware. |
| B-19.9 | Native record/HTTP queue item; prior native repair not compiled here. |
| B-19.10 | Partly fixed on base for Fire/Leanback facts. Generic unadvertised boxes need explicit layout policy/device acceptance. |
| B-19.11 | Deliberate sandboxed plain Vega files; no new encryption requirement, per programme policy. |
| B-19.12 | Native installation/reinstall cleanup item; prior repair not recompiled here. |
| B-19.13 | Already fixed for caption/row geometry; large-offset native float precision remains hardware/frontend work. |
| B-19.14 | Already fixed for inactive-root/covered-stack freezing; retained-history memory and real focus acceptance remain. |
| B-19.15 | Fixed further here: driver replaces store subscriptions; app/test types and full suites run. React internal driver/device CI limits remain. |
| B-19.16 | Already fixed for identified non-screen catalogue paths; both string lints pass. Diagnostics remain technical records. |
| B-19.17 | Not present for shipped regional English catalogues; future non-English Hermes/Vega acceptance belongs to i18n owner. |
| B-19.18 | Native scene lifecycle item; prior scene repair not built with Xcode here. |
| B-19.19 | PiP capability consumption already fixed; SearchKeyboard/edge-to-edge integration remains frontend/native owner work. |
| B-20.1 | Already fixed: one capability document separates decoded audio, passthrough and output. Receiver acceptance remains. |
| B-20.2 | Already fixed: capability document preserves native tone-mapping facts; device acceptance remains. |
| B-20.3 | Already fixed: matching-origin/grant plain HTTP subtitle handling retained. |
| B-20.4 | Already fixed in the base consumer/native implementation; engine contract and relevant JS tests retained. Hardware acceptance was not repeated. |
| B-20.5 | Already fixed in the base consumer/native implementation; engine contract and relevant JS tests retained. Hardware acceptance was not repeated. |
| B-20.6 | Already fixed: typed failure classification avoids blaming network/delivery failures on the codec. |
| B-20.7 | Fixed further for Vega seek/rate/position barriers; systemPaused handling already follows intent. Hardware acceptance remains. |
| B-20.8 | Already fixed in the base consumer/native implementation; engine contract and relevant JS tests retained. Hardware acceptance was not repeated. |
| B-20.9 | Already fixed in the base consumer/native implementation; engine contract and relevant JS tests retained. Hardware acceptance was not repeated. |
| B-20.10 | Already fixed in the base consumer/native implementation; engine contract and relevant JS tests retained. Hardware acceptance was not repeated. |
| B-20.11 | Already fixed in the base consumer/native implementation; engine contract and relevant JS tests retained. Hardware acceptance was not repeated. |
| B-20.12 | Already fixed for Portico caption style. Vega has no system caption-style API; existing limitation remains. |
| B-20.13 | Already fixed: transient subtitle rereads retain existing cues within their freshness bound. |
| B-20.14 | Already fixed in the base consumer/native implementation; engine contract and relevant JS tests retained. Hardware acceptance was not repeated. |
| B-20.15 | Already fixed: chapters/previews and capability claims match maintained player wiring. |
| B-20.16 | Already fixed: video/group/native-listening queue step paths remain, with passing queue tests. |
| B-20.17 | Already fixed: profile speed applied only on supported targets. |
| B-20.18 | Already fixed for bounded courtesy check; its late starts are fenced further here. |
| B-20.19 | Native extractor/fallback acceptance item; clamped Vega seeks now report failure instead of false success. |
| B-20.20 | Already fixed in the base consumer/native implementation; engine contract and relevant JS tests retained. Hardware acceptance was not repeated. |
| B-20.21 | Already fixed in the base consumer/native implementation; engine contract and relevant JS tests retained. Hardware acceptance was not repeated. |
| B-20.22 | Already fixed in the base consumer/native implementation; engine contract and relevant JS tests retained. Hardware acceptance was not repeated. |
| B-20.23 | Already fixed in the base consumer/native implementation; engine contract and relevant JS tests retained. Hardware acceptance was not repeated. |
| B-21.1 | Already fixed: permanent manual TV-code destination. |
| B-21.2 | Already fixed: recording cancellation confirmation. |
| B-21.3 | Already fixed: capable non-TV wide rail offers Downloads. |
| B-21.4 | Already fixed: bounded Saved indexing independent of Home My List. |
| B-21.5 | Already fixed: retained lists have a windowed owner; actual long-session focus/memory acceptance remains. |
| B-21.6 | Partly fixed: search-history/badge controls are reachable; appearance card/backdrop consumers remain frontend work. |
| B-21.7 | Already fixed: class-scoped settings use class wording. |
| B-21.8 | Not fixed here: needs server/shared occurrence plus playlist-revision anchor; item ID cannot identify duplicates. |
| B-21.9 | Not fixed here: needs shared discovered-track/resource identity through pre-play selection and frontend chooser work. |
| B-21.10 | Already fixed: Vega unsupported speed/audio-effects controls are capability-hidden; actual engine rate barrier repaired here. |
| B-21.11 | Partly fixed on base: bounded 500-title gather with explicit notice; full saved-query selector remains server/shared work. |
| B-21.12 | Already fixed: actual permission/role gates replace direct-authentication assumptions. |
| B-21.13 | Already fixed: service identity participates in selection guard; test driver now models replaced subscriptions correctly. |
| B-21.14 | Already fixed: pending link/viewer intent fence and route coalescing; player delayed intents repaired further here. |
| B-21.15 | Not changed: live filters survive Back by the recorded product choice; NAV-6.7 needs correction. |
| B-21.16 | Semantic alternate already present; visible More affordances and physical remote/accessibility acceptance remain frontend work. |
| B-22.1 | Already fixed in maintained shared/server/native code per fix log and supporting report; no new platform/network acceptance claimed. |
| B-22.2 | Already fixed in maintained shared/server/native code per fix log and supporting report; no new platform/network acceptance claimed. |
| B-22.3 | Browser-only rotating-store coordination; outside this native lane. |
| B-22.4 | Already fixed: transient unreadability preserves sign-in; ambiguous key loss remains confirmation-led. |
| B-22.5 | Already fixed: unsigned replacement hints preserve the pinned family. |
| B-22.6 | Recorded plain-HTTP channel trade-off, not changed or represented as cryptographically protected sockets. |
| B-22.7 | Already fixed: bounded shared scheme/port candidates including IPv6. |
| B-22.8 | Already fixed: ordinary request deadlines distinct from deliberate streams. |
| B-22.9 | Already fixed for bounded shared watcher; native account ownership repaired further here. |
| B-22.10 | Already fixed in maintained shared/server/native code per fix log and supporting report; no new platform/network acceptance claimed. |
| B-22.11 | Already fixed in maintained shared/server/native code per fix log and supporting report; no new platform/network acceptance claimed. |
| B-22.12 | Already fixed in maintained shared/server/native code per fix log and supporting report; no new platform/network acceptance claimed. |
| B-22.13 | Already fixed in maintained shared/server/native code per fix log and supporting report; no new platform/network acceptance claimed. |
| B-22.14 | Already fixed in maintained shared/server/native code per fix log and supporting report; no new platform/network acceptance claimed. |
| B-22.15 | Apple/Android native admission already repaired; Vega authenticated transport is unavailable under its safety gate. |
| B-22.16 | Already fixed in maintained shared/server/native code per fix log and supporting report; no new platform/network acceptance claimed. |
| B-22.17 | Already fixed in maintained shared/server/native code per fix log and supporting report; no new platform/network acceptance claimed. |
| B-22.18 | Already fixed for selected-route opening; Vega early cancellation and background feed policy repaired further here. |
| B-22.19 | Security containment already present. NOT functionally complete: Vega needs a real redirect-refusing native authenticated transport. |
| B-22.20 | Already fixed in native Android one-shot/non-replaying write transport; source guards pass. |
| B-22.21 | Required-HTTPS bootstrap requirement superseded by explicit HTTP-from-any-address policy. |
| B-22.22 | Already fixed for bounded verified LAN promotion; hardware/network acceptance remains. |
| B-22.23 | Already fixed in maintained shared/server/native code per fix log and supporting report; no new platform/network acceptance claimed. |
| B-22.24 | Server/mDNS conformance/device item; outside this JavaScript lane. |
| B-22.25 | Already fixed: retry floors retain positive jitter and bounded timers. |
| B-22.26 | Already fixed: shared Hosted activation gate retained, with existing tests passing. |
| N-V08.1 | Fixed further here: unchanged-count transient continuation recovery, first-page backlog bound, and owner-scoped cursors all have failing-before/passing-after regressions. |
| N-V02.1 | Shared session path already present; native control/engine fencing retained and exercised. |

# API changes

No server route, wire shape, registry or schema was changed; no legacy route was added. Vega's command rejection and seek/readiness behaviour now enforce the existing engine contract. Internal `playOffline` can accept a pending LocalMedia lookup so intent is reserved before reading the file. Both callers were updated. Existing typed shared services/parsers remain read-only.

# Schema changes

None. Device bookkeeping remains version 3; the observation-clock cache is bounded in-memory state only.

# Owner or viewer settings added or wired

No new settings or controls. Existing downloads auto-next and daily receipt maintenance now run on network/foreground wake and the minute tending interval. Existing Player/quality/subtitle settings retain their defaults and screens. No automatic quality or engine default changed.

# Needed from others

- **Shared client owner:** in `packages/client-core/src/viewer-selection.ts`, define retirement/unused-candidate identity by server plus session family (and viewer scope), rather than differing bearer strings. The native callback guards same-family retirement locally; its positive restore regression must stay when the shared decision is unified. Do not edit copied packages here.
- **Vega native owner (release blocker, inherited):** authenticated SDK XHR exchanges still fail before dispatch under the existing `transport_unavailable` safety gate. Implement bounded native control/stream transport that refuses redirects before credentials/body transmission, validates TLS, cancels reliably and never replays uncertain writes. Then test every redirect status with capture endpoints before replacing the safety refusal. The JS repairs do not supply this missing SDK/native facility.
- **Server/shared and frontend owners:** occurrence/revision-aware playlist selectors (B-21.8), exact pre-play subtitle resource/discovery identity (B-21.9), and full saved-view/query queue selectors beyond the existing 500-title gather (B-21.11). Do not substitute a loaded-page index or language alone.
- **Frontend/native owners:** existing appearance consumers, visible accessible More affordances, generic-box TV-layout policy, SearchKeyboard/edge-to-edge integration, native rotation/very-large-grid precision and retained navigation-memory acceptance. No visual design change was attempted here.
- **Documents:** native/vega/freshness/download report claims must be read with these corrections: Vega originally lacked seek/binding/restoration guarantees; the native feed used title presence rather than actual playing state; ongoing download maintenance omitted renewals/retries; delayed play/sign-in ownership and same-family retirement needed this pass. Update `ENGINE-CONTRACT`/architecture client descriptions to mention Vega's implementation and existing transport restriction; NAV-6.7 should describe the approved live-filter policy. These read-only documents were not edited.
- **Clock boundary:** the server clamps future observations and compares millisecond marks; no cross-device clock calibration or total ordering is claimed. If equal-time observations across devices/clock shifts need disambiguation, the server/shared contract needs an owner-bound play observation sequence. Native same-title observations are now ordered without removing the existing server skew/explicit-action checks.

# Test notes for other environments

All automated checks ran on Ubuntu against existing node_modules symlinks, without installing or modifying dependencies. Logs live in `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/review-native-js/`.

| Check | Result / log |
|---|---|
| Native full suite, `node --experimental-strip-types --import ./test/register.mjs --test --test-concurrency=2 test/*.test.ts` | **547/547**, zero failed/skipped/cancelled; `native-full-final.log` |
| Vega full suite, equivalent `--experimental-transform-types` | **72/72**, zero failed/skipped/cancelled; `vega-full.log` |
| Native `npx tsc --noEmit` | Exit 0; `native-app-typecheck-final.log` |
| Native `npx tsc -p test/tsconfig.json --noEmit` | Exit 0; `native-test-typecheck-final.log` |
| Vega application/test type-checks | Exit 0; `vega-app-typecheck-final.log`, `vega-test-typecheck-final.log` |
| Native/Vega `npm run lint:strings` | Exit 0; `native-strings-final.log`, `vega-strings-final.log` |
| `git diff --check`; `git diff HEAD -- packages` | Clean whitespace; no shared-package changes |

The native full suite was rerun after the final native lifecycle/housekeeping changes. The unchanged Vega tree was not redundantly retested after its passing full suite and type-checks. Existing Node module-type/experimental warnings and already-baselined diagnostic string-lint notices remain; no checks fail. The test-only JSX loaders use the existing TypeScript dev dependency to run the actual SessionProvider/Vega debug module, not copies of their implementation.

Cold device acceptance (integrated test server, no production deployment):

1. **iPhone/iPad/Apple TV and Android/Android TV/Fire TV, normal builds:** delay playback options, tap another title, play a download, tune a channel, close the player and switch profiles. Pass: only the newest deliberate intent starts, including a slow local-file lookup; no fallback queue is created after an uncertain response. With native/mpv, switch engines mid-title, rapidly choose tracks/quality, seek and advance the queue. Pass: one engine lease, retained position/queue/pause and no superseded facts/actions.
2. **Authentication:** delay saved-session restore and `/v1/me` verification separately. Sign out, Use another, forget the saved sign-in, cancel a profile choice or close the app. Pass: the old viewer never reappears; Use another retains its original saved family. Let an ordinary restore renew the token in the same family. Pass: selected viewer survives and no DELETE of that family is sent. Repeat Quick Connect, two-factor and required-password steps with cancellation/profile switches; no obsolete completion is admitted.
3. **Downloads on phones/tablets:** pause/lose Wi-Fi, make the server issue a replacement grant, restore connectivity with the app still running. Pass: replacement address reaches the downloader; integrity stays checked; a held transfer remains held. Switch viewer while renewal is delayed: no old result patches/starts it. On Android the same artifact resumes partial bytes; iOS may restart a partial when URL changes, the native limitation from downloads.md.
4. **Journal and maintenance:** record a rapid pause/stop, delay the first upload, then release it; send an incomplete progress acknowledgement. Pass: latest viewing is retained/ordered and only acknowledged observations settle, with no drain loop. Fail progress/auto-next while offline, restore network without reconnect/relaunch. Pass: wake/minute maintenance completes both. Stay connected through the next renewal day; pass: one renewal batch per viewer/day, overlapping wake coalesced. Force access withdrawal; pass: its check is not skipped by the daily gate.
5. **Container enrollment:** a large show plus enough requests to fill the backlog; fail one continuation without changing counts. Pass: no more than 5,000 waiting members, no missing member when space frees, retry after the floor, no repeated-cursor loop. Repeat two viewers/servers with equal request/item IDs; neither inherits the other's cursor/copy suppression.
6. **Background feed:** visible idle app owns its feed. Background a paused/error/ended title: after about 30 seconds the feed is closed. Playing audio/video retains it; Play/foreground starts it again with its cursor. Repeated visibility/route changes and an open cancelled before headers must settle, not accumulate streams.
7. **Vega with its eventual secure transport:** media element must stay paused until seek command retirement; unsupported rate stays paused. Clamp a seek/retained route position with a test element/proxy. Pass: seekFailed or retained-position error, no false ready/start-at-zero, ten-second restoration deadline. Same media path on a new verified route may resume; stale intent/binding/retired seek commands must be refused without changing the current player.
8. **Diagnostics and TV focus:** enable developer diagnostics only in a test build, induce errors containing synthetic short/long credentials and grant paths, send the explicit report. Pass: no secret in console, overlay, sink or stored report. Refresh/reorder/remove focused TV cards and replace a screen's store; pass: current action/focus belongs to the surviving card and old listeners are removed. Layout/render/focus behaviour requires real renderer/remote/screen-reader checks; this lane did not visually inspect it.

# Not finished

No confirmed local review finding remains unrepaired and no requested automated check is failing. The upstream family-retirement decision, inherited Vega authenticated transport release blocker and background register frontend/server/native handoffs above remain with their owners. No claim of full native/first-party release parity is made.

No Xcode, Gradle, Vega build, real authenticated server exchange, OS downloader, device playback, renderer, TV remote or screen-reader acceptance ran here. The cold acceptance instructions above remain unexecuted. Wall-clock cross-device ordering and iOS changed-address partial resume retain the stated limits.

Cumulative snapshots: 001 (initial findings), 002 (play/download/Vega/diagnostic repairs), 003 (candidate fencing and position checks), 004 (same-family restoration). Final **005** captures the verified tree; no source-tree changes follow it. Reports/logs stay in the programme directory. Nothing committed, pushed or deployed.
