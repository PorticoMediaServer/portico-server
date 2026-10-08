# Apple / React Native code audit

Date: 2026-09-25. Baseline: split repository `62463dd`. Scope: `apps/portico-react-native`; canonical server/package sources were read only to establish contracts and copy equivalence. This is a static source audit. No application, test, build, simulator, service, deployment, network request, or dependency installer was run. No application code was changed.

Status: completed static audit. 20 established source defects, all P2; 4 separate conditional/acceptance concerns (3 conditional P2, 1 P3). Second-pass verdicts are in `08-adversarial-server-reviewer.md`: NC-001 was downgraded and NC-024 moved to conditional; the other mechanisms survive with narrowed scope. Findings below are source-established unless explicitly classified as conditional. Severity indicates practical priority, not a claim of runtime reproduction. There are no established P0 findings.

## Source path convention

`APPLE` means `/Users/justin/Documents/Codex Projects/Portico/apps/portico-react-native/apps/apple`.
`RN` means `/Users/justin/Documents/Codex Projects/Portico/apps/portico-react-native`.
`SERVER` means `/Users/justin/Documents/Codex Projects/Portico/apps/portico-server`.
All paths and line ranges below are baseline source references using these roots. Offline coordinator findings NC-003 and NC-006–009, plus concern NC-017, apply to iOS/iPadOS: `APPLE/src/bridge/offline-downloads.ts:110` explicitly excludes TV.

## Established defects

### NC-001 — Top Shelf fails to retire or rebind its owner state

**P2 · High confidence in owner-lifecycle defect; visible privacy outcome conditional on usable cached posters**

**Evidence:** `APPLE/src/app/top-shelf.ts:13–23`; `APPLE/src/app/device.tsx:29–49`; `APPLE/App.tsx:12–20`; `APPLE/src/shell/Shell.tsx:308–345` (`DeviceApprovalCover` is a child of the authenticated shell); `APPLE/Native/PorticoTopShelf.m:48–67`; `APPLE/tvos/TopShelf/ContentProvider.swift:60–109`.

**Trigger / chain:** Sign in as A and obtain a shelf token. Sign out, or choose profile B on the same origin while the token has more than 20 days remaining. `refreshTopShelf` keys reuse only on origin and expiry; it cannot distinguish server/profile/account/session ownership. The only JS clear call is an effect inside `DeviceApprovalCover`, conditioned on `!session`. Root removes the entire Shell when authentication is no longer ready, so that child does not render the signed-out value and has no clearing unmount cleanup. The extension keeps reading the shared token and cached feed independently. A refresh started under A also has no generation fence before `native.configure`.

**Impact:** With usable posters already cached (for example from a previous working version), A's viewing history, titles, progress and artwork can remain visible on the Apple TV home screen after sign-out or to another profile. NC-019 prevents newly fetched posters in this baseline, so an empty artwork cache currently masks visible cards; repairing it makes the missing owner cleanup directly visible as well. The visible out-of-app disclosure remains conditional on that usable-cache state, which was not established on a deployed device. It is not a claim that clicking a shelf tile bypasses the main app's item/play authorization. Fix NC-019 together with this owner defect so restoring artwork does not expose the latent privacy failure.

**Counterevidence:** The extension clears cache on an explicit HTTP 401/403; native `clear()` removes the feed file and token if actually called. Server-side counterevidence matters: `SERVER/server/internal/identity/topshelf.go:75–139` checks expiry, device approval and current viewer validity. Local grants rederive the account/profile without requiring the originating session family; hosted grants require any unrevoked family for that profile, so revoking its last hosted family can make the next online feed fail. The client therefore does not necessarily keep a valid online feed after every hosted sign-out. It still fails to clear locally, reuses the previous profile token on a same-origin switch, and shows the cached feed on network failure before it can observe rejection. Native clear also leaves the poster directory, whose filenames are scoped only by item ID. NC-019 can mask this on an empty poster cache but does not remove the retained-token/history defect.

**Fix:** Own shelf state at the session boundary outside the authenticated subtree. Store an explicit server/account/profile/session owner with the token; rotate or clear on owner change. Fence every async issue/configure and extension publication with that owner epoch. Clear feed and artwork for the retired owner; avoid using unscoped item IDs across servers.

**Suggested regressions:** A → sign-out; A → B on one server; server A → B with colliding item IDs; delayed token issuance after sign-out; extension load concurrent with clear; cold offline extension load after owner change.

### NC-002 — An externally supplied invitation fragment can crash the native app

**P2 · High confidence · Untrusted deep-link parsing**

**Evidence:** `APPLE/tvos/Portico/PorticoURLRoutes.swift:75–88,115–123,147–163,183–190`. This source is compiled into both iOS and tvOS targets.

**Trigger / chain:** Open `portico://invite#=` (or an accepted universal-link path with fragment `#=`). `fragmentPairs` splits on `&`, then splits `=` using Swift's default omission of empty subsequences. For the pair `=`, the second split is empty, but `kv[0]` is indexed unconditionally. `allows` reaches this parser before React Native can reject anything. The `/join` and account-return universal-link paths also call it.

**Impact:** A malformed external link produces an index-out-of-range trap and terminates the application. No authentication or valid invitation is needed to reach the parser.

**Counterevidence:** Scheme, host, userinfo and identifier allowlists are present and strong on other route branches. They do not guard the collection index.

**Fix:** Parse pairs with explicit empty-field behavior, guard the pair shape before indexing, reject missing/duplicate keys and malformed percent encoding, and bound fragment size before parsing.

**Suggested regressions:** Empty fragment; `=`, `==`, `&=&`, missing key, missing value, percent-encoded separator, duplicate key, very long fragment, and malformed fragments for every custom/universal route. These are proposed tests, not executed reproductions.

### NC-003 — Receipt verification is cached by mutable receipt ID rather than signed content

**P2 · High confidence · Offline authorization integrity**

**Evidence:** `APPLE/src/bridge/offline-downloads.ts:116–117,378–386,398–404,411–440`.

**Trigger / chain:** A receipt verifies once. Renewal or a later native entry event replaces its envelope but retains the receipt ID. `storeReceipt` writes the replacement; `verifyAll` skips every ID already in `verified`; `playable` asks only whether that ID was previously verified and then checks the new claims. Consequently a replacement with a changed payload/key/signature inherits the old verification result without verifying its bytes. There is also no invalidation when a remembered signing key changes.

**Impact:** A well-shaped replacement envelope can inherit an old cryptographic verdict without checking its new signed bytes. This is an iOS/iPadOS verification-cache invalidation defect, not a demonstrated capability bypass. Exploiting it requires a separately established response/storage injection path; honest renewal supplies a valid new signature. No unauthenticated injection or forged Ed25519 signature was established.

**Counterevidence:** `RN/packages/client-core/src/downloads.ts:216–247` bounds/decodes the envelope, requires a 64-byte signature, validates claims, matches receipt/key IDs to payload, and checks mirrored claims agree with that payload. Editing only the exposed expiry field is rejected. `receiptUsable:255–260` checks authority/account/profile/server and expiry. First-seen receipts undergo native verification. The remaining gap is the cryptographic verdict for replacement signed bytes. Server renewal deliberately preserves the receipt ID (`SERVER/server/internal/downloads/receipts.go:450–463`), making replacement reachable without implying honest renewal is forged.

**Fix:** Cache verification of the exact signed payload/signature/key fingerprint, scoped to the authoritative server; invalidate on any envelope or key change. Bind receipt claims to the downloaded preparation/item/artifact before authorizing its file. Make a late verification result prove the same envelope still occupies the entry.

**Suggested regressions:** Same receipt ID with altered payload, signature, key or expiry; renewal during an in-flight verify; key rotation; two servers with the same key ID; a valid receipt attached to the wrong downloaded artifact.

### NC-004 — Top Shelf acknowledges success after falling back to ordinary preferences

**P2 · High confidence · Credential storage failure path**

**Evidence:** `APPLE/Native/PorticoTopShelf.m:18–27,35–46,48–55`; `APPLE/tvos/TopShelf/ContentProvider.swift:33–47`.

**Trigger / chain:** Any `SecItemAdd` failure for the shared group causes `configure` to persist the live feed bearer in shared `NSUserDefaults` and resolve success. The comment describes unsigned builds, but this branch has no DEBUG/signing/environment guard. The helper deletes the previous Keychain entry before attempting the write. The extension explicitly reads the fallback.

**Impact:** A release-time entitlement/storage failure silently weakens the intended credential boundary and leaves a live limited bearer in the ordinary shared preferences container. The user gets no failure result; a diagnostic warning is logged, so this is not silent in the logs.

**Counterevidence:** This is a scoped shelf/feed token, not the account refresh token; successful writes use `AfterFirstUnlockThisDeviceOnly`. No claim is made that every sandboxed application can read the app group.

**Fix:** Fail closed with a typed unavailable state; use update-or-insert without first destroying the valid token; remove legacy defaults only after verified migration. Any development-only fallback must be explicitly excluded from shipping targets.

**Suggested regressions:** Missing entitlement; locked/unavailable Keychain; failed update after an existing token; successful migration; release build has no plaintext fallback.

### NC-005 — Recent searches leak between profiles and ignore the display side of the history preference

**P2 · High confidence · Privacy / UI state**

**Evidence:** `APPLE/src/screens/Search.tsx:35–41,65–91,154–164`.

**Trigger / chain:** Search as A, sign out/switch profile or server without killing the process, then open an idle Search screen as B. `recent` is module-global and contains no owner. The preference gates only `remember()`; rendering old recent queries is unconditional. Turning history off therefore still displays earlier queries.

**Impact:** Another viewer of the same running application can see A's searches. The list can also violate the current viewer's explicit history preference.

**Counterevidence:** The remote `SearchService` is viewer-scoped and disposed correctly; the list is bounded to eight strings and is not persisted across process death. Those do not protect the module-global local history.

**Fix:** Scope history to a deliberate owner key, clear it at sign-out, hide/clear when disabled, and fence deferred remember callbacks against the submitted query and owner.

**Suggested regressions:** A → B, server switch, history off after existing searches, slow search response after sign-out, and process restart.

### NC-006 — Container requests omit automatic iOS/iPadOS transfer enrollment

**P2 · High confidence · Incomplete download flow**

**Evidence:** `APPLE/src/app/downloads.tsx:64–95`; `APPLE/src/bridge/offline-downloads.ts:264–298,300–325`.

**Trigger / chain:** Download a season/show/other container. The container branch calls `requestContainer`, displays the accepted/preparing result and returns. Only the explicit-item branch calls `offlineDownloads.want`. The collector downloads a ready preparation only if `stored.wanted[owner][itemId]` exists. No container-request membership reconciliation supplies those wants.

**Impact:** The server can prepare the whole container while the device never automatically receives its files. A user leaves the app believing a season is downloading but later has no offline media. The existing manual transfer action can recover individual ready preparations.

**Counterevidence:** Auto-next explicitly performs both preparation and `want`, documenting why both steps are required. Server request following and receipt ownership do not enroll the missing items. This finding is separate from the shared `DownloadsService` follower-generation defect reported by the parent audit.

**Fix:** Persist device-owned container intent and reconcile its paged prepared members into the same transfer queue, including completion and reconnect/relaunch paths. Preserve bounded pagination and owner/request cancellation semantics.

**Suggested regressions:** Container larger than 100; delayed preparation; close/reopen while preparing; reconnect; cancel request; owner switch; only the device which requested transfer receives files.

### NC-007 — More than 100 local receipts stops all renewal

**P2 · High confidence · Client/server batch contract**

**Evidence:** `APPLE/src/bridge/offline-downloads.ts:392–406`; `SERVER/server/internal/downloads/receipts.go:354–358`; `SERVER/server/internal/downloads/types.go:99–104` (`MaxBatchTargets = 100`).

**Trigger / chain:** One owner has 101 or more receipt-bearing entries. `renewReceipts` posts all receipt IDs in one request. The server rejects that request above its 100-target limit; the client swallows the error and never chunks/retries smaller batches.

**Impact:** That owner's entire iOS/iPadOS batch is refused, including the first 100; other owners' smaller batches are not shown to fail. Unchanged receipts eventually expire despite reconnection. Removing enough held entries can make a later batch succeed; the failure lasts while the oversized batch remains.

**Counterevidence:** Ownership filtering prevents another profile's receipts being submitted, and small batches work. Neither addresses the upper bound.

**Fix:** Chunk by the published maximum, use bounded concurrency and per-batch operation IDs, preserve generation fencing, and expose actionable repeated renewal failures.

**Suggested regressions:** 0, 1, 100, 101, 250 receipts; one failed batch with others successful; profile switch mid-batch; refusal deletes only the matched owned entry.

### NC-008 — The device download limit excludes transfers already in progress

**P2 · High confidence · Resource policy / concurrency**

**Evidence:** `APPLE/src/bridge/offline-downloads.ts:283–298,331–351`; `APPLE/Native/PorticoDownloads.m:362–367`.

**Trigger / chain:** With a 5 GB limit and no completed downloads, start two prepared 4 GB files. Native `usedBytes` counts only `ready` entries. `blocked()` adds the candidate size to that number, with no reservation for queued/downloading/paused files. `native.start` resolves after starting, so the collector can authorize further files while the first remains in flight.

**Impact:** The explicit download-storage cap can be substantially exceeded and disk space checks overcommit the same free space. Low space can then interrupt media and unrelated application storage.

**Counterevidence:** Native operations are serialized and URLSession connections are limited; neither reserves bytes across sequential starts acknowledged before completion. Completed downloads and a 500 MB free-space margin are accounted for. OS download errors eventually limit physical writes, but are not enforcement of the user's configured cap.

**Fix:** Reserve remaining expected bytes atomically before obtaining/starting a grant; account for all active transfers and partial files; release reservations on terminal removal/failure and rebuild them after relaunch. Recheck capacity after grant waits.

**Suggested regressions:** Concurrent and sequential start acknowledgements, paused/resumed files, failed start, remove while in flight, unknown free space, and restart with partial transfers.

### NC-009 — Offline bookkeeping reports durable success when writes were skipped or failed

**P2 · High confidence · Persistence / recovery**

**Evidence:** `APPLE/Native/PorticoDownloads.m:112–117,354–358,400–413`; `APPLE/src/bridge/offline-downloads.ts:176–191,262,266–273`.

**Trigger / chain:** Disk write failure, unavailable protected storage, an oversized metadata string, or state reaching the 1,048,576-character cap. Native writes use `error:nil`; `writeState` silently skips an oversized document and resolves success. `setMeta` similarly acknowledges a missing entry or oversized metadata. JS `persist()` additionally swallows a rejection and publishes the settings as though saved. A state read/JSON failure is treated as empty state and then immediately persisted.

**Impact:** Download settings, outstanding wants, receipt keys, viewing progress and resume intent can disappear on restart even though the UI acknowledged the operation. A read failure followed by a successful empty-state write can overwrite recoverable bookkeeping. If protected storage also rejects the write, that overwrite does not occur; not every transient read failure causes data loss.

**Counterevidence:** Writes are atomic and use protected-file options, which help successful writes. They do not prove a write took place or distinguish missing from unreadable state. Keychain playback journals elsewhere correctly return storage failures; downloads need the same discipline.

**Fix:** Return and handle typed read/write/size failures, preserve unreadable files for recovery, and publish durable success only after successful save. Bound/split and prune the growing per-owner state deliberately instead of silently dropping writes at the cap.

**Suggested regressions:** Disk full, denied write, read unavailable vs genuinely missing, malformed JSON, exactly-at/over-limit state, failed receipt metadata write, crash between atomic replacement stages.

### NC-010 — Native audio accepts oversized range responses without enforcing a receive-byte budget

**P2 · High confidence · Disk/resource exhaustion boundary**

**Evidence:** `APPLE/Native/PorticoAudioRender.m:135–157,178–207`.

**Trigger / chain:** An audio endpoint ignores the initial Range and returns a whole HTTP 200 body, returns a 206 with a matching start but excessive end/body, or streams an unbounded unknown-length body. The client allows 200 at offset zero, verifies only the 206 start, and writes every received data chunk. `_limit` is tested before starting another request, not while receiving the current request. Cache release occurs in `pump`, which does not run again while the task remains active.

**Impact:** An oversized accepted response can exceed the intended range/read-ahead/prepared-byte/cache limits, causing excessive transfer and on-disk growth. Existing network timeouts can end a request; the proven gap is a missing per-response byte ceiling, not infinite execution or measured disk exhaustion. The prepared deck's byte allowance is also unenforced against an oversized accepted response.

**Counterevidence:** Redirects are refused, normal range requests are small, and cache trimming exists for cooperative responses. Those controls do not bound one non-cooperative response. No TLS or authentication bypass is alleged.

**Fix:** Parse and verify the full Content-Range and stable total; record the exact permitted response length; cancel before writes cross request, grant/preparation, stream and disk budgets. If range-ignoring streams are supported, give that fallback an explicit bounded spool policy.

**Suggested regressions:** Ignored Range, 206 wrong end/total/start, huge Content-Length, endless chunked response, limit crossed across chunk boundaries, cancellation and partial-file recovery.

### NC-011 — Native artwork and Top Shelf body limits are checked after unbounded accumulation

**P2 · High confidence · Memory/network resource boundary**

**Evidence:** `APPLE/Native/PorticoSystemMedia.m:230–245`; `APPLE/tvos/TopShelf/ContentProvider.swift:76–95,125–149`. Contrast `APPLE/Native/PorticoListeningOwner.m:865–879`, whose listening artwork request sets an incremental byte limit.

**Trigger / chain:** A poster or feed endpoint returns an unexpectedly large or endless body. Completion-handler `dataTask` APIs accumulate the data before the 4 MB/8 MB checks execute; the feed has no body-size cap before JSON decoding at all. Four poster tasks can accumulate concurrently.

**Impact:** A malformed selected media-server response can consume memory/bandwidth far beyond the intended limits; process termination is a possible consequence, not a measured result. Canonical server response controls, timeouts and poster concurrency reduce ordinary exposure. No arbitrary unauthenticated response-injection path was established. Limiting thumbnail dimensions only limits the decoded output after the response has arrived.

**Counterevidence:** Poster concurrency is four, timeouts exist, selected output is only 20 cards, and image decoding uses thumbnail APIs. These do not impose a byte ceiling on network accumulation or feed parsing.

**Fix:** Use delegate-driven incremental body accounting, early Content-Length rejection plus streaming enforcement, bounded feed schema/entry/string counts, and task cancellation on owner change. Reuse the bounded request pattern already present in the listening owner.

**Suggested regressions:** Oversized declared response, chunked body just over limit, malicious feed counts/strings, concurrent maximum-size images, malformed image decode, owner retirement during receive.

### NC-012 — A stale playback preflight can override a newer play or reopen a closed player

**P2 · High confidence · User-intent ordering / navigation**

**Evidence:** `APPLE/src/player/usePlayer.ts:324–364,392–432`. The sequence fallback callbacks at lines 407 and 420 also call `startSingle` after a rejected asynchronous queue operation without proving that the original play remains current.

**Trigger / chain:** Tap video A, then video B; resolve B's playback-options request before A's. `play` starts A only after `checkDolbyFor` resolves and has no request-intent generation check. `startSingle` creates the playback intent only then. Thus A is treated as the newest intent when its old preflight finally completes. Closing the player while a request is pending is likewise not a cancellation of that callback.

**Impact:** Within the mounted player lifecycle, late preflight can start the wrong title, interrupt a newer play, or restart after `leave`. UI collapse is intentional minimization, not cancellation. Full owner teardown/service disposal may prevent post-unmount starts; cross-profile or post-unmount reopening is not independently established. A stale warning can also replace the current pending choice.

**Counterevidence:** The playback service and native coordinator fence intents after an intent has been submitted. `describeGeneration` protects detail fetches started in `startSingle`. Neither can know that a late call to `startSingle` came from an older tap.

**Fix:** Allocate an intent epoch before any preflight and carry it through options, warning acceptance and start. Invalidate on newer play, leave, viewer change and unmount; check after every await and confirmation.

**Suggested regressions:** Deferred A/B preflights resolving in both orders, A video then B audio, leave before response, stale warning acceptance, sequence vs single competition, profile switch during preflight.

### NC-013 — Video interruptions resume even when the viewer had paused

**P2 · High confidence · Playback lifecycle**

**Evidence:** `APPLE/Native/PorticoSystemMedia.m:333–345`; `APPLE/src/player/VideoPlayer.tsx:258–268`.

**Trigger / chain:** A video is intentionally paused before an audio-session interruption. The native handler records `_interrupted = YES` without remembering whether playback was intended to be running. On an interruption end carrying `ShouldResume`, it emits `play`. JS's command handler toggles any currently non-playing video to playing; it does not inspect the interruption reason or previous intent.

**Impact:** If an active paused video receives an interruption-end notification carrying ShouldResume, this handler emits play without proving prior/current user intent. Whether particular calls, Siri or alarms deliver that sequence was not tested. A user pause during the interruption can likewise be overridden under that event condition.

**Counterevidence:** The native listening/music owner retains its `_intent` through interruption and therefore does not have this exact error. The finding applies to the video system-media branch.

**Fix:** Snapshot the pre-interruption intended state and owner/intent revision; resume only if it was playing and no subsequent user command/owner change invalidated the snapshot. Clear interruption state when retiring video.

**Suggested regressions:** Already paused; playing; manual pause during interruption; new video during interruption; interruption ends without ShouldResume; owner is retired before end.

### NC-014 — Search and Saved retain and mount every loaded page

**P2 · High confidence · Large-library performance**

**Evidence:** `APPLE/src/app/paging.ts:13–35`; `APPLE/src/screens/Search.tsx:121–130,180–199,294–305`; `APPLE/src/screens/Saved.tsx:210–225,235–265,295–308,324–345,366–386`; `APPLE/src/ui/Grid.tsx:48–57`.

**Trigger / chain:** Keep scrolling a large narrowed search/Saved collection. `useAppendedPages` keeps every cursor page in a map with no eviction and builds a new full deduplicated array on every page. The screen uses a scrolling Page with `Grid scrollEnabled={false}`; that Grid branch explicitly `rows.map(renderRow)` mounts every row rather than virtualizing.

**Impact:** Within one browsing generation, memory/native view count grows with visited results, and each appended page processes accumulated history. Query/scope reset and finite result sets bound its lifetime and eventual size. Dropped frames, focus loss or termination are risks requiring device measurement, not observed outcomes. This contradicts the documented O(visible) client collection requirement.

**Counterevidence:** Other collection screens use the existing windowed-grid abstraction; this is not a blanket claim that every list is unvirtualized. Fetching one server page at a time prevents one huge response but does not bound retained/rendered state.

**Fix:** Use one virtualized/windowed outer collection with stable item identity and a bounded reloadable page cache. Preserve focus/restoration using logical indices instead of retaining mounted historical pages.

**Suggested regressions:** Thousands of results, rapid forward/backward scrolling, query/filter/owner change, removal from a retained page, bounded mounted cells/page retention, TV focus restoration. Runtime performance was not measured in this audit.

### NC-015 — The shared iOS Field drops its visible label from the input's accessibility semantics

**P2 · High confidence in missing association; device impact unverified · Accessibility**

**Evidence:** `APPLE/src/ui/Field.tsx:33,41–43,60–64,77–93`; concrete consumers include `APPLE/src/screens/auth/DirectSignIn.tsx:100–101` and `APPLE/src/screens/auth/HostedSignIn.tsx:245–246` (username/password labels without explicit accessibility labels).

**Trigger / chain:** Render a Field with a visible `label` and no explicit `accessibilityLabel` at its call site. The wrapper destructures `label`, renders it as a separate Text, and never passes it to the iOS TextInput or associates the two. It even removes an identical placeholder, removing the remaining input hint. TV's Focusable branch correctly assigns the label, so this is platform-specific.

**Impact:** The iOS/iPadOS Field does not establish the visible label as the input's programmatic accessible name by default. Platform heuristics may announce nearby text in some layouts; precise VoiceOver utterances were not observed. The visible label alone does not establish the intended relationship.

**Counterevidence:** A caller supplying `accessibilityLabel` through `rest` works; Search does so. The finding concerns ordinary callers relying on the Field's documented label prop. Exact VoiceOver announcements need device acceptance.

**Fix:** Default `accessibilityLabel` to `label` while preserving explicit overrides, and associate hint/error state appropriately without duplicate announcements.

**Suggested regressions:** Label with/without duplicate placeholder, explicit accessibility override, secure field/reveal, error/hint, text resizing and VoiceOver traversal on iPhone/iPad.

### NC-019 — Top Shelf removes the required artwork capability from every fresh poster URL

**P2 · High confidence · Native/server contract mismatch**

**Evidence:** `APPLE/tvos/TopShelf/ContentProvider.swift:130–143,179–186`; `SERVER/server/internal/httpapi/topshelf.go:123–128,136–141,216–219`.

**Trigger / chain:** Load the shelf with an empty artwork cache. The server emits `/v1/topshelf/art/{id}?grant=<capability>&w=400`. The extension replaces all query items with `size=thumbnail`, deleting `grant`. The server's artwork handler exclusively resolves the query capability; the feed bearer added to Authorization is not a substitute. The request is refused, and `item()` omits every entry without a cached poster.

**Impact:** Fresh Top Shelf installations/cards remain blank even when feed loading and token issuance succeed. This is a deterministic source-level protocol mismatch, not a provisioning hypothesis.

**Counterevidence:** A previously cached poster bypasses fetching and can still render; feed JSON itself may be valid. The server deliberately uses a separate per-item short-lived artwork capability.

**Fix:** Preserve the exact server-authored capability URL after validating its expected origin/path; retain its size parameters or negotiate supported ones. Do not replace the capability with an unrelated bearer scheme. Update native/server integration fixtures together.

**Suggested regressions:** Actual-shaped feed URL with required grant, first load with no posters, new item after old cache exists, expired capability and feed refresh, valid server rejection behavior.

### NC-020 — Concurrent Home shelf paging strands the second shelf

**P2 · High confidence · State concurrency**

**Evidence:** `APPLE/src/app/home.ts:84–103`; `APPLE/src/screens/Home.tsx:112–114,281–318`.

**Trigger / chain:** Request another page for rows A and B before either resolves. Both capture the same `state.document`. A's success creates a new document object. B's success then returns the previous state because `prev.document !== document`, even though B's row and cursor remain current. That early return also leaves B in `paging` indefinitely. Home removes its `onMore` while that set contains B.

**Impact:** The second shelf silently discards a valid page and cannot load again until the whole Home document is refreshed. This can occur through normal movement between shelves on a slow connection.

**Counterevidence:** Generation and per-row cursor checks are present and useful. The additional whole-document reference equality incorrectly treats an unrelated row's append as invalidation. The rejection/error branch does clear its paging flag; the successful-but-discarded branch does not.

**Fix:** Fence by document generation and the specific row/cursor identity, allowing unrelated rows to update. Always retire the matching in-flight marker when a request completes, including discarded success, without clearing a newer request's marker.

**Suggested regressions:** A/B completions in either order, overlapping rows plus full refresh, two requests for one row, stale cursor response, and error after another row succeeds.

### NC-021 — Channel-source cache reuses a retired authenticated API after re-sign-in

**P2 · High confidence · Session/cache lifecycle**

**Evidence:** `APPLE/src/app/channels.ts:29–69`; `APPLE/src/app/session.tsx:368–375`; `RN/packages/client-core/src/presentation/content.ts:12–16`.

**Trigger / chain:** Sign out and sign back into the same viewer at the same server origin without terminating the process. `sourceStores` is module-global and keyed by viewer identity/origin, not API/session identity. `sourcesStore` returns the old `SourcesStore`; its private API remains the one captured by the first constructor. The newly authenticated API passed to the lookup is ignored. Already-loaded cached sources are returned without revalidation, and Refresh issues its request through the retired API.

**Impact:** Live TV/source visibility can stay stale after reauthentication. Refresh uses the retired API and fails once its credentials/route authority expire or are revoked; failed logout or still-valid credentials may let it work temporarily. The rest of the app can have a valid new sign-in. A failed cached first load is also not automatically retried on a later mount because `snapshot.loading` is false.

**Counterevidence:** Entries are bounded to four cache keys and keyed by viewer/origin, reducing unrelated-viewer disclosure. The current `useMemo` depends on API changes but this merely re-runs a lookup that returns the obsolete store.

**Fix:** Keep shared stores within a live session/provider lifetime, or bind/rebind API identity with cancellation and fresh generation. Dispose on retirement/eviction and deliberately revalidate on re-sign-in.

**Suggested regressions:** Sign-out/re-sign-in same profile, token rotation on a stable API, a genuinely new API for the same viewer, initial failure then remount, source permissions changed between sign-ins.

### NC-022 — The playback timeline has no accessible adjustment action

**P2 · High confidence in missing semantics; device impact unverified · Accessibility**

**Evidence:** `APPLE/src/player/Timeline.tsx:21–35,39–60`.

**Trigger / chain:** Attempt timeline seeking using assistive technology. On iPhone/iPad the timeline is only a View with PanResponder gestures, with no accessible role/value/actions. On TV it declares `adjustable` but supplies no increment/decrement accessibility actions or handler; physical left/right handling elsewhere is not an accessibility action implementation.

**Impact:** The custom timeline does not implement programmatic accessible adjustment semantics on handheld or TV. The precise VoiceOver behavior and whether users are blocked on each target must be confirmed on device. Separate skip buttons offer an alternative, so this is not a claim that all playback seeking is inaccessible.

**Fix:** Expose an accessible adjustable element on both platforms, with value/min/max and increment/decrement actions mapped to bounded seek steps. Keep live/disabled semantics and announce the resulting position without excessive updates.

**Suggested regressions:** VoiceOver increment/decrement, live and unavailable-duration states, pending seek, beginning/end clamps, TV screen-reader navigation and normal remote seeking. Device behavior remains untested here.

### NC-023 — Early play retains reduced capability selection for the queue-controller lifetime

**P2 · High confidence · Startup ordering / background audio**

**Evidence:** `APPLE/src/bridge/useNativePlayback.ts:84–85,92–133`; `APPLE/src/bridge/native-listening-kinds.ts:21–24`; `APPLE/src/bridge/playback-paths.ts:31–36`; `RN/packages/client-core/src/playback-v1/v1-queue-player.ts:148–151,374–396`.

**Trigger / chain:** The viewer plays before `/v1/capabilities` resolves. `ensureQueue()` constructs the queue player while `supportsTransitions=false` and `serverKinds=[]`. `nativeListeningFor(serverKinds)` closes over that empty array; V1QueuePlayer stores both options. Later the capabilities callback assigns new local variables but does not update the installed controller; it explicitly returns if `holder.controller` exists. The queue therefore retains disabled transitions/native audio selection after capabilities succeed.

**Impact:** A fast first play or slow capabilities response can leave songs/audiobooks on the JS-owned path for the queue lifetime, losing native background ownership and supported gapless/crossfade behavior. Later titles on that queue retain the initial wrong capability choice.

**Counterevidence:** A player first created after capabilities arrive receives the correct options. Playing local media and later recreating the server queue can also incidentally recover. The defect is ordering-dependent, not universal audio failure.

**Fix:** Gate initial server queue creation on a bounded capabilities readiness state with explicit fallback/retry semantics, or update a live capability source read at each start and safely transition/recreate an idle controller after discovery. Do not replace a playing controller silently.

**Suggested regressions:** Deferred capability response after immediate audio play, transient capabilities failure/retry, first video then music, capability publication timeout, already-active queue when discovery completes.

## Conditional concerns / acceptance gaps

These are not counted as established production incidents. The condition and missing evidence are explicit.

### NC-024 — Pending Spotlight callbacks lack an owner fence; final post-clear visibility is unresolved

**Conditional P2 · High confidence in missing source fence; OS ordering/visible outcome unverified · Privacy lifecycle**

**Evidence:** `APPLE/Native/PorticoActivity.m:44–46,75–104,110–114`; `APPLE/src/screens/Home.tsx:43–49`; `APPLE/src/app/session.tsx:794,998,1026,1071`.

Home indexing requests asynchronous domain deletion and then submits captured titles in the completion callback. A later sign-out/profile clear submits another deletion without invalidating the earlier callback. Depending on CoreSpotlight completion/operation ordering, stale titles may be submitted after that privacy clear. The source lacks an owner epoch, but final system-index visibility is not established by this audit.

**Counterevidence:** Exported native methods already run on the main queue; this is not a simultaneous mutable-state race. The Siri title-cache file is written synchronously during indexTitles and removed synchronously during clear; the later callback does not recreate that file. Session transitions do call clear, public indexing is disabled, and CoreSpotlight ordering may affect the result. A requested 30-day expiry does not prove 30 days of post-clear visibility. This entry is excluded from established defects.

**Fix / verification:** Fence by owner epoch and serialize the complete asynchronous delete/index sequence without blocking the main queue. A pre-submission epoch check alone cannot retract an already submitted old insertion; cleanup must be the final durable operation. Use a controllable index adapter for adverse orderings and then inspect final Spotlight results on signed devices. Verify direct Siri cache cleanup separately.

### NC-016 — Top Shelf poster credential destination validation is host-only

**P2 if attacker-controlled feed/image path is reachable · High confidence in check; exploitability conditional**

`APPLE/tvos/TopShelf/ContentProvider.swift:130–141` accepts an image URL when only its host matches. It does not require the same scheme/port, forbid userinfo, constrain the API path, or install a redirect policy. The token is then attached to the request. A feed with an absolute same-host/different-port HTTPS URL could send the limited feed bearer to another service on that host. Normal server feed generation uses expected relative artwork URLs; no observed malicious feed was supplied and OS redirect/header/ATS behavior was not tested. Enforce exact normalized origin and a narrow artwork path grammar, reject credentials/fragments and redirect destination changes. Regress foreign port/scheme, userinfo, scheme-relative URL, redirect and expected relative path.

### NC-017 — Remembered download signing keys are not scoped to their server

**P2 if key-ID collision or replacement is possible · High confidence in map shape; exposure conditional**

`APPLE/src/bridge/offline-downloads.ts:411–428` stores all public keys as `stored.keys[key.id]` without server identity. A later server publishing the same key ID overwrites the first server's verifier. Correct signatures then fail after restart; together with the ID-only verification cache this also yields inconsistent warm/cold behavior. The canonical server generates randomized 32-character digest IDs (`SERVER/server/internal/downloads/receipts.go:130–143`); no practical honest-server collision is established. Deliberately conflicting IDs or corrupted/imported state remain a namespace-isolation concern. Cold verification with an overwritten wrong key fails closed, so this is primarily conditional availability/inconsistent verification behavior, not proven signature bypass. Use `{serverId,keyId}` plus key fingerprint, validate the receipt's server binding, and handle rotation explicitly. Regress two server fixtures with identical key IDs and different keys.

### NC-018 — Shipping/privacy/native acceptance remains unproved

**P3 · Configuration acceptance gap, not a claim of rejection**

The Apple targets, Podfiles, entitlements, ATS dictionaries, app delegates and privacy manifests were read statically. Native downloads use `NSURLVolumeAvailableCapacityForImportantUsageKey` (`APPLE/Native/PorticoDownloads.m:364`), and `Native/PorticoAudioRender.m:57–60` also reads capacity while the checked-in app privacy manifests enumerate other required-reason categories. Confirm that the shipping merged privacy manifest covers every used API category and the selected reason applies; source-only review cannot certify the final embedded manifests or current store review outcome. Top Shelf has its own Info.plist/entitlements and needs separate acceptance for local server transport, app/keychain groups, revocation and offline caches. Do not infer from source declarations that signed-device provisioning, Cast receiver registration, background transfer restoration, PiP, AirPlay, Dolby/HDR, remote focus or VoiceOver work on actual hardware.

## Coverage and positive controls

- Native source inventory covers Swift/Objective-C modules for network discovery/crypto, secure credentials/profile trust, account authentication, downloads and file selection, playback ownership, listening timeline/queue commit/replay, audio decode/cache, subtitle parsing/selection, audio-track selection, Now Playing/remote commands/interruption/route reset, Top Shelf, Activity/Spotlight/Handoff, Cast, Siri Remote touch, capabilities, native keyboard, image picker and durable queue journals. Production branches were prioritized over debug conformance harnesses.
- React boundaries traced include Root/Shell/authenticated mounting, session restoration/sign-out/profile selection, secure-storage wrappers, server routes, native playback coordinator, offline playback and device ownership, download UI/service integration, player intent/preflight/subtitle/audio hooks, search/paging, artwork pipeline, Field/Text/Grid/Shelf and channel-guide windowing. Further targeted coverage included Home mutation/paging and Spotlight updates, route/open translation, retained root stacks and Menu exit behavior, browse/episode/section page adapters, libraries/source stores, reminders, inbox lifecycle, shared sheets/focus primitives and the timeline. This is not a claim that every presentation-only line or every debug harness was individually read.
- Positive controls observed: Keychain `ThisDeviceOnly` use for normal credentials; explicit missing vs unreadable secure-storage handling; local PIN durable rate limiting and authenticated encryption; bounded Bonjour discovery; authenticated route proof verification; normal TLS trust without a general trust-all delegate; request redirect refusal and incremental byte bounds in several native transports; generation/lease/native owner fencing; local playback path containment via realpath; bounded subtitle files/cues; owner-scoped offline entry visibility and progress; download content SHA-256 checking; durable queue reservation and error handling; native listening monotonic lease/timer checks and lost commit replay; artwork thumbnail downsampling and normal windowed collection abstractions.
- These controls reduce risk but are not runtime security acceptance or proof against every exploit. No secrets were copied into this report.

## Coverage ledger

The `rg --files` inventory contained 613 files under the RN application root: 404 under `apps/apple`, 207 in the three copied shared packages, and the root/package-sync metadata. This includes assets, configuration and test sources; it is not a count of independently reviewed implementation units.

| Area | Static review performed | Remaining proof boundary |
| --- | --- | --- |
| Native identity/transport | Account authentication, runtime/profile HTTP requests, credentials and selected-viewer handling, trust/PIN helpers, Swift network crypto/discovery, route grammar and both app delegates | Signed build entitlements; real certificate/proxy/LAN behavior; OS callback delivery |
| Native downloads | Manifest/state persistence, grants, background task ownership/resume, hash verification, storage policy, cellular changes, receipt bridge and local playback containment | Background relaunch, disk pressure, protection classes and device file behavior |
| Native playback | Production player/module/controller, listening owner loop, report/lease/control/commit/timer retirement, queue journal, audio range/decode/cache, subtitle/audio selection, system media, remote and route ownership | Real codecs, AV timing, render quality, output changes, PiP/background execution |
| Platform integration | Top Shelf app bridge and extension, Handoff/Spotlight activity, Cast transport, Siri Remote, keyboard, capabilities, image/subtitle picker and route picker | SDK/system behavior, Cast receiver setup, focus and accessibility acceptance |
| RN session/navigation | Root/Shell/retained roots, sign-in/session/selected-viewer paths, server/credential bridge modules, source/list scope, route/open helpers and lifecycle hooks | Interactive transitions; not every auth/settings visual branch individually inspected |
| RN playback/download UI | usePlayer, useNativePlayback/coordinator, player system commands, timeline, download provider/screens/offline auth path | Gesture/remote usability and perceived timing |
| RN lists/state | Search, Saved, Home paging, channel sources/reminders/guide, browse/section/episode windows, inbox, library hooks, artwork scheduling/cache and core grid/shelf/page/focus/sheet/field/text primitives | Large-library memory/frame measurements; some title/settings screens were structurally searched rather than read line-by-line |
| Packaging | Package manifests/locks, Metro/Babel/project/Podfile declarations, app and extension plist/entitlement/privacy files; copied-package content comparison | No build, code signing, dependency advisory lookup or App Store validation |
| Tests | Test inventory, focused routing/Top Shelf/Dolby/offline ownership/stub reading, native/debug harness presence | No test results claimed; debug harness bodies not comprehensively reviewed |

Unreviewed or lighter-covered areas are explicitly not clean bills of health. Binary artwork/fonts, generated dependency contents, vendored SDK internals and runtime environment data were not audited. No secret-bearing local configuration was opened for disclosure.

## Shared package copy check

A read-only SHA-256 comparison was performed against canonical `SERVER/packages` using the same source/document/package scopes as `RN/scripts/shared-packages.mjs`. No repository synchronization script was executed and no files were copied.

| Package | Canonical files | RN files | Content/path differences |
| --- | ---: | ---: | ---: |
| client-core (`src`, package.json, tsconfig.json) | 186 | 186 | 0 |
| design (`src`, package.json, COMPONENTS.md) | 6 | 6 | 0 |
| i18n (`src`, scripts, baselines, package.json) | 15 | 15 | 0 |

The shared implementation is audited in the parent/shared-client report. This report deliberately does not duplicate the `DownloadsService.followRequest` generation cancellation finding or general shared list-window cache issues. Zero copy drift establishes equivalence only, not correctness.

## Verification limitations and recommended acceptance order

No tests were executed. Test sources read include `top-shelf.test.ts`, `porticourlroutes-tests.swift`, `dolby-vision.test.ts`, the offline ownership cases and `react-native-stub.mjs`. Top Shelf tests exercise origin/expiry and explicit clear calls, not Root unmount or owner transitions. Native route tests omit empty-key fragments. Dolby tests exercise warning parsing, not asynchronous intent races. Offline tests cover ownership, expiry, first verification failure and deferred progress batches; their crypto bridge defaults to a stubbed acceptance verdict and native writes always succeed, so they do not establish real cryptography/persistence or cover the >100 renewal envelope. Reading them establishes intended assertions, not passing tests or device behavior. No dependency advisories were fetched, signing/build output inspected, network response exercised, server contacted, or account action performed. Timing/resource findings above are derived from reachable control flow and missing checks; suggested inputs remain proposed regression cases.

Prioritize NC-001 through NC-005 as security/privacy boundary repairs, then container/receipt/storage correctness, then network resource budgets and player intent ordering. After code fixes, run focused mocked regressions and signed-device acceptance for lifecycle, background transfers, native focus/input, output routes, extension caches and accessibility. Keep those runtime results distinct from this static audit.
