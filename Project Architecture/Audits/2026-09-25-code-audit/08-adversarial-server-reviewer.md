# Adversarial review of Apple / React Native findings

Date: 2026-09-25. Reviewer: independent server-area reviewer reassigned to challenge `04-apple-react-native.md`. This pass checks all 24 NC IDs against source, callers, guards and counterevidence. It is not an independent exhaustive audit of the entire native application. Internal repository HEAD was reported as externally changed to `8f412e4`; the coordinator reported unchanged inventoried source hashes. This review does not certify that external change.

Static inspection only: no application, test, build, simulator, network request, service or deployment was run. Only this report was written. Suggested runtime regressions are future verification work, not results.

## Recommended classification changes

Retain **20 established source defects, all P2**, with the wording/scope below. Retain **4 separate conditional or acceptance concerns: NC-016, NC-017 and NC-024 at conditional P2, and NC-018 at P3**. No established P0/P1 is recommended for this current baseline. In particular:

- **NC-001: downgrade established severity from P1 to P2.** The owner/cleanup defect is established. Visible out-of-app disclosure depends on usable pre-existing posters; NC-019 prevents populating an empty poster cache through the current canonical flow. Treat the P1 privacy outcome as conditional on such a cache, or on fixing NC-019 without fixing ownership.
- **NC-024: move from established defects to conditional P2.** Source proves an unfenced asynchronous indexing callback. It does not prove the final CoreSpotlight ordering or an actual post-clear disclosure. Native exported methods are already serialized on the main queue.
- **NC-003: retain established P2 for reusing a stale cryptographic verdict, not a demonstrated capability bypass.** Receipt parsing checks considerably more than the original counterevidence states; replacement must be a well-shaped, internally consistent envelope. No unauthenticated path for injecting forged replacement bytes was established.
- **NC-006 through NC-009, NC-003 and NC-017: label offline coordinator consequences iOS/iPadOS only.** `APPLE/src/bridge/offline-downloads.ts:110` explicitly excludes TV. They are not established tvOS download flows.
- **NC-015 and NC-022: retain P2 missing accessibility semantics, with exact VoiceOver behavior unverified. NC-018 remains an acceptance gap, not a demonstrated privacy-manifest violation or store rejection.**

`APPLE` = `apps/portico-react-native/apps/apple`; `RN` = `apps/portico-react-native`; `SERVER` = `apps/portico-server`. Paths below are relative to `/Users/justin/Documents/Codex Projects/Portico`. These aliases apply only to source references, not to additional claims of coverage.

| ID | Verdict | Recommended classification |
| --- | --- | --- |
| NC-001 | Downgrade and narrow | Established P2 lifecycle defect; visible P1 outcome conditional |
| NC-002 | Uphold | Established P2 |
| NC-003 | Narrow | Established P2 verification defect; exploit unproved |
| NC-004 | Narrow | Established P2 |
| NC-005 | Uphold | Established P2 |
| NC-006 | Narrow platform/impact | Established P2, iOS/iPadOS |
| NC-007 | Narrow platform/owner | Established P2, iOS/iPadOS |
| NC-008 | Narrow platform | Established P2, iOS/iPadOS |
| NC-009 | Narrow platform/failure outcome | Established P2, iOS/iPadOS |
| NC-010 | Narrow resource claim | Established P2 receive-bound defect |
| NC-011 | Narrow resource claim | Established P2 receive-bound defect |
| NC-012 | Narrow lifecycle claim | Established P2 |
| NC-013 | Narrow event condition | Established P2 |
| NC-014 | Narrow performance claim | Established P2 |
| NC-015 | Narrow platform acceptance | Established P2 semantics defect |
| NC-016 | Uphold conditional classification | Conditional P2 |
| NC-017 | Narrow strongly | Conditional P2; honest collision not practical evidence |
| NC-018 | Uphold acceptance-only classification | P3 acceptance gap |
| NC-019 | Uphold | Established P2 |
| NC-020 | Uphold | Established P2 |
| NC-021 | Narrow failure claim | Established P2 |
| NC-022 | Narrow platform acceptance | Established P2 semantics defect |
| NC-023 | Narrow duration wording | Established P2 |
| NC-024 | Downgrade certainty/classification | Conditional P2; OS final ordering unresolved |

## Per-finding challenge and corrected conclusions

### NC-001 — Downgrade to established P2; condition the visible privacy outcome

**Independent chain:** `APPLE/App.tsx:12–20` removes the authenticated Shell when phase is not ready. The cover's only signed-out clear is inside that subtree (`src/app/device.tsx:29–49`; `src/shell/Shell.tsx:308–345`). `src/app/top-shelf.ts:13–23` reuses by origin/expiry and has no owner or async publication epoch. Native clear at `Native/PorticoTopShelf.m:62–67` deletes token/feed, not the poster directory. The extension keys posters by item ID and requires a local poster before publishing an item (`tvos/TopShelf/ContentProvider.swift:125–149,179–186`).

**Counterevidence and correction:** Current canonical fresh poster acquisition fails under NC-019, so a clean installation does not establish a visible history disclosure. Existing usable cache is a necessary extra condition for that outcome until the artwork bug is repaired. Server grant validation checks expiry, approved device and valid viewer (`SERVER/server/internal/identity/topshelf.go:75–139`). Hosted revocation of the last valid family may cause the next online fetch to reject; local validity is not tied to the originating session family. Network fallback still permits retained cached data. Correct wording: **“Top Shelf fails to retire/rebind owner state on sign-out or same-origin profile changes; existing usable cached posters can expose the prior owner's feed.”** Do not claim every hosted logout leaves a valid online grant.

**Fix pitfalls / verification:** Cleanup must live outside the unmounted subtree; clearing only the token does not isolate old posters or late configure calls. Include owner epoch in issue/configure/cache publication and remove retired-owner artwork. Verify clean-cache masking separately from seeded-cache sign-out/profile switch, delayed issuance, offline extension load and server-side revocation. P1 privacy priority can be justified once the pre-existing-cache deployment condition is established, but it is not demonstrated here.

### NC-002 — Uphold established P2

**Independent chain:** `APPLE/tvos/Portico/PorticoURLRoutes.swift:115–123` performs a default-empty-omitting second split and indexes `kv[0]`. For an individual pair `=`, this array is empty. Invitation guards at `75–88` invoke parsing before they can reject the malformed value; universal routes also reach it at `147–163,183–190`. Both app delegates call the route allowlist (`ios/Portico/AppDelegate.swift:13,54`; `tvos/Portico/AppDelegate.swift:12,41`).

**Counterevidence and correction:** Existing scheme/host/path allowlists restrict which branch is entered, but an allowed invite route with malformed fragment enters this parser. No valid token or authenticated state guards the array access. The source-level trap conclusion is supported, although no Swift binary was executed.

**Fix pitfalls / verification:** Guard empty components without changing accepted percent-encoding or applying decoding twice. Explicitly define duplicates and empty keys/values; add cold/warm custom-link and universal-link cases. A JS-only parser fix cannot protect the earlier native allowlist.

### NC-003 — Narrow to established P2 verification-cache invalidation defect

**Independent chain:** `APPLE/src/bridge/offline-downloads.ts:116–117,378–386,398–440` remembers only verified receipt IDs; storage replacement and later verification do not invalidate that result. Server renewal deliberately signs updated claims under the same receipt ID (`SERVER/server/internal/downloads/receipts.go:450–463`), so mutable same-ID envelopes are a normal reachable path. `playable` then evaluates current claims under the old ID-level verdict. This coordinator is iOS/iPadOS only (`offline-downloads.ts:110`).

**Additional counterevidence:** `RN/packages/client-core/src/downloads.ts:216–247` bounds and decodes payload/signature, requires a 64-byte signature, validates all claims, checks envelope receipt/key IDs against payload, and requires mirrored claims to agree with the decoded payload. Changing only the exposed expiry field is rejected. `receiptUsable:255–260` checks authority/account/profile/server and expiry. The native-event decoder (`APPLE/src/bridge/offline-downloads.ts:529–534`) re-parses stored envelopes before publishing them. First-seen envelopes undergo native signature verification. Honest current-server renewal supplies a valid signature; its successful playback alone is not an exploit.

**Correct wording:** **“A well-shaped replacement receipt with an already verified ID inherits the prior cryptographic verdict without checking the replacement signed bytes. This is a source-established verification defect; acceptance of attacker-forged authority requires a separately demonstrated response/storage injection path.”** No forged signature, unauthenticated internet exploit, or merely editable unsigned claims path was established. Artifact-to-receipt binding deserves regression coverage but is not proved as an additional exploitable mismatch by this finding.

**Fix pitfalls / verification:** Cache exact server/key fingerprint/payload/signature identity and fence late verification against the current entry. Do not treat decoder agreement as signature verification. Test honest renewal plus changed payload and mirrored claims, invalid new signature, key change and verification races; preserve fail-closed first-verification behavior. Validate any proposed exploit at the real protected-storage/response boundary before describing it as a capability bypass.

### NC-004 — Narrow wording; uphold established P2

**Independent chain:** `APPLE/Native/PorticoTopShelf.m:18–27,35–46` deletes an old Keychain item before adding, then falls back to shared defaults on any add failure; no release exclusion guards this path. `tvos/TopShelf/ContentProvider.swift:33–47` consumes that fallback. Native expiry reading also migrates legacy values (`PorticoTopShelf.m:48–55`).

**Counterevidence and correction:** It logs a warning; “silent” should mean **no user-visible failure**, not no diagnostic. This is a limited shelf bearer and app-group storage, not an account refresh credential exposed to every application. Normal successful storage uses a device-only accessibility class. No actual signed-build entitlement failure was observed. Correct wording: **“Keychain failure resolves configuration success after persisting the live shelf bearer in ordinary shared preferences.”**

**Fix pitfalls / verification:** Update-or-insert must preserve the previous valid token until replacement succeeds. A migration must remove fallback only after confirming secure storage, and must make failure visible without repeatedly leaking fallback values. Exercise failure branches in release configuration and the extension's read path; do not claim fixing app code alone proves signing acceptance.

### NC-005 — Uphold established P2 with same-process scope

**Independent chain:** `APPLE/src/screens/Search.tsx:35–41` stores eight queries in a module-level array. The preference only gates `remember` (`80–90`); idle display (`154–164`) does not check it. The viewer-scoped SearchService is independently disposed (`44–59`), which does not reset the module array.

**Counterevidence and correction:** This is bounded volatile state, not persisted cross-launch history. Service scoping protects remote search results but not the rendered recent strings. Correct wording retains **same running process** and **local recent-query visibility**, rather than suggesting remote authorization leakage.

**Fix pitfalls / verification:** Choose a deliberate authority/server/account/profile owner and clear/hide on opt-out; a remount alone cannot clear module state. Fence deferred remember callbacks as well as rendering. Verify A→B, same-account different server, disable-after-history, and process restart separately.

### NC-006 — Narrow to automatic iOS/iPadOS transfer; uphold P2

**Independent chain:** `APPLE/src/app/downloads.tsx:64–95` returns after `requestContainer`; only the explicit-item branch calls `offlineDownloads.want`. `src/bridge/offline-downloads.ts:264–298` requires a wanted item before collecting. Auto-next explicitly adds wants (`300–325`). Searches of the corresponding provider and coordinator call sites found no alternate container-member enrollment. The coordinator excludes TV (`110`).

**Counterevidence and correction:** Server preparation can succeed; manual transfer can recover individual items. This is not proof that every container request or every download action fails. Correct wording: **“Container preparation does not enroll its members for automatic device transfer on iOS/iPadOS.”** The shared follower-generation defect is independent and may additionally mask progress; fixing that alone will not add wants.

**Fix pitfalls / verification:** Persist owner/device/container intent and reconcile all paginated members, including later-prepared items. Marking only the initial visible page is incomplete. Cancellation must not delete other devices' preparation or another owner's wants. Verify delayed preparation, over-100 containers, relaunch/reconnect and owner changes.

### NC-007 — Uphold P2; narrow to one owner's iOS/iPadOS batch

**Independent chain:** `APPLE/src/bridge/offline-downloads.ts:392–406` submits every held owner receipt in one renewal; `SERVER/server/internal/downloads/receipts.go:354–358` rejects more than `MaxBatchTargets=100` (`types.go:99–104`). The catch does not split it. `RN/packages/client-core/src/downloads.ts:264` also caps parsed outcomes at 100.

**Counterevidence and correction:** Native connection concurrency limits transfers, not retained receipt count. No universal retained-file count cap proves 101 impossible. Owner filtering is present; other owners' valid small batches are not shown to fail. Removing enough entries can allow a later renewal to recover. Correct wording: **“With more than 100 held receipts, that owner's renewal batch is rejected in full until the batch is reduced or chunked; unchanged receipts eventually expire.”** Scope is iOS/iPadOS (`offline-downloads.ts:110`).

**Fix pitfalls / verification:** Use stable per-batch operation IDs, bounded retries and owner epochs; a single overall ID reused with different payloads may violate idempotency. Verify 100/101/250, partial success, removals and profile change without treating successful reconnection alone as successful renewal.

### NC-008 — Uphold P2; serialized callbacks do not enforce the cap

**Independent chain:** `APPLE/src/bridge/offline-downloads.ts:283–298,331–351` authorizes each candidate against completed `usedBytes`; `Native/PorticoDownloads.m:362–367` counts ready entries only. Native start (`278–317`) resolves after initiating the task, before completion, so even sequential JS starts can overcommit the same allowance. This is iOS/iPadOS only.

**Counterevidence and correction:** Native work is on a serial queue and URLSession connections are limited (`PorticoDownloads.m:61,141`). That prevents some dictionary races and limits concurrency, but two permitted transfers can exceed a smaller total byte cap. Completed bytes and the free-space margin are useful existing checks. Correct wording is a **missing reservation/accounting invariant**, not a demonstrated native memory race.

**Fix pitfalls / verification:** Reserve expected remaining bytes durably/atomically, reconcile partial files and release on terminal transitions. Avoid counting partial bytes both as used and fully reserved. Test sequential immediate acknowledgements as well as true concurrent calls, paused tasks, restart, unknown size and changing free space. Disk exhaustion was not induced.

### NC-009 — Narrow failure outcome; uphold P2 on iOS/iPadOS

**Independent chain:** `APPLE/Native/PorticoDownloads.m:99–117,354–358,400–413` conflates read failures with absence and ignores write errors; size rejection/missing entry can still resolve success. JS read/persist (`src/bridge/offline-downloads.ts:176–191,262,266–273`) replaces invalid/unreadable state with empty state and swallows save failures. The 1,048,576 guard is a string-length guard, not a verified byte quota.

**Counterevidence and correction:** Atomic/protected writes and native serialization are real controls. A storage-protection error may also prevent the subsequent empty write, so **not every transient read failure overwrites state**. Correct wording: **“The bridge can acknowledge unsaved state, and a read failure followed by a successful empty-state write can overwrite recoverable bookkeeping.”** The iOS/iPadOS coordinator scope must be explicit.

**Fix pitfalls / verification:** Distinguish missing, unreadable, malformed and oversized state; preserve a recovery copy and surface typed errors. A missing entry may represent legitimate concurrent removal and needs an explicit result rather than indiscriminate fatal failure. Bound growth deliberately without silently losing receipts or intent. Verify read-fail/write-success separately from both-fail and only publish durable success after write confirmation.

### NC-010 — Narrow to missing per-response budget; uphold P2

**Independent chain:** `APPLE/Native/PorticoAudioRender.m:135–157,178–207` checks limits before starting a request, accepts offset-zero 200 and checks only 206 start, then writes all received chunks without enforcing that response's requested/budgeted length. `pump` cannot trim/start another operation while the current task remains active. The delegate is serialized and locking exists; serialization does not bound received bytes.

**Counterevidence and correction:** Normal canonical responses honor requested ranges; redirects are refused; cache policies and network timeouts exist. Therefore “unbounded” should mean **no explicit receive-byte ceiling for one accepted response**, not infinite wall-clock execution or guaranteed disk exhaustion in ordinary playback. The adverse outcome requires an oversized/misbehaving endpoint response. Correct wording: **“A non-cooperative accepted audio response can exceed the intended range/prepared/cache allowance before completion.”**

**Fix pitfalls / verification:** Bound actual chunks, not merely declared Content-Length, and validate full Content-Range consistency. Preserve legitimate unknown-length formats only with a bounded spool policy; do not introduce unbounded fallback retries on Range refusal. Verify a controlled oversized/chunked server response, partial-file cleanup and cancellation. No such server was run here.

### NC-011 — Narrow resource impact; uphold P2

**Independent chain:** `APPLE/Native/PorticoSystemMedia.m:230–245` and `tvos/TopShelf/ContentProvider.swift:76–95,125–149` use completion-handler data tasks before enforcing poster byte limits; the feed has no pre-decode body cap. Four poster tasks may be in flight. Thumbnail downsampling occurs after download.

**Counterevidence and correction:** Network timeouts, poster concurrency, a 20-card output selection, and canonical server response controls reduce normal exposure. They do not prove an incremental receive-byte bound. A malformed/compromised trusted media response is required for arbitrary excess, not an arbitrary unauthenticated URL accepted by every branch. **Memory termination is a possible consequence, not a measured result.** NC-019 currently blocks successful fresh poster decoding but does not itself bound a malicious oversized error/body response accumulated before completion.

**Fix pitfalls / verification:** Enforce streaming limits plus declared-length rejection, owner cancellation and bounded feed schema sizes. A response's Content-Length can be absent or false. Retain thumbnail and concurrency controls rather than treating them as substitutes. Verify body accounting and process memory with controlled oversized responses after source fixes; no runtime acceptance is claimed.

### NC-012 — Uphold P2; narrow navigation/lifecycle claims

**Independent chain:** `APPLE/src/player/usePlayer.ts:324–364` allocates playback intent in `startSingle`, after an unfenced asynchronous Dolby preflight. Sequence fallbacks at `392–432` similarly call it after awaits. A stale A completion after B's completion becomes the newest submitted play. `leave:497–505` stops playback and collapses without invalidating the preflight; `collapse:506` is deliberately only UI minimization.

**Counterevidence and correction:** Downstream native/service intent guards protect submitted intents, and detail requests use `describeGeneration`; they do not order the preceding user taps. Conversely, unmount/service disposal may prevent old requests from starting after a full owner teardown. Correct wording: **“Within the mounted player lifecycle, a late preflight can override newer play or restart after leave; cross-profile or post-unmount reopening is not independently established.”** Do not conflate collapse with stop.

**Fix pitfalls / verification:** Allocate/fence before preflight and carry the epoch through dialogs, sequence fallbacks and leave. Aborting a fetch alone does not guard an already resolved promise/dialog callback. Verify reordered A/B, A-video/B-audio, mounted leave and stale confirmation; test owner teardown separately rather than assuming identical reachability.

### NC-013 — Uphold P2 under the specific interruption event condition

**Independent chain:** `APPLE/Native/PorticoSystemMedia.m:333–345` sets `_interrupted` without storing prior playback intent and emits play on end with ShouldResume. `_active` identifies a video owner, including paused video (`75–78`); it is not a playing-intent flag. JS (`src/player/VideoPlayer.tsx:258–268`) toggles a non-playing matching item in response. `clearVideo:204–210` does not reset the interruption flag.

**Counterevidence and correction:** The native listening owner has its own intent discipline; this is the video branch. OS delivery of ShouldResume in each real-world scenario was not tested, so do not state all calls/alarms resume paused video. Correct wording: **“If an active paused video receives an interruption-end notification carrying ShouldResume, this handler emits play without proving prior or current user intent.”**

**Fix pitfalls / verification:** Record intended playback and owner revision, not merely post-interruption engine rate (which the OS may already have paused). User pause or owner retirement must invalidate resumption. Test actual notification sequences on device, including no ShouldResume, already paused and pause during interruption. The proposed trigger remains device-unverified.

### NC-014 — Uphold P2; narrow quantitative/performance claims

**Independent chain:** `APPLE/src/app/paging.ts:13–35` retains cursor pages and rebuilds a deduplicated accumulated list. Search (`src/screens/Search.tsx:121–130,180–199,294–305`) and Saved (`src/screens/Saved.tsx:210–225,235–265,295–308,324–345,366–386`) feed nested non-scrolling grids. `src/ui/Grid.tsx:48–57` maps all rows in that branch rather than using its virtualized alternative.

**Counterevidence and correction:** Query/scope reset clears history; a finite result set and per-request paging bound eventual work. Other screens use windowing. Correct wording: **“Within one browsing generation, retained pages and mounted rows grow with visited results rather than the visible viewport.”** Dropped frames, focus loss and process termination are risks requiring measurement, not established observed symptoms. No claim of a literal infinite data structure or all screens being nonvirtualized is warranted.

**Fix pitfalls / verification:** Virtualize the outer scroll owner; replacing only an inner list can retain nested-scroll/focus problems. Preserve stable logical IDs and restoration with bounded reloadable pages. Test mount/page counts across long forward/back browsing, resets and TV focus; do not claim O(visible) merely from choosing a component name.

### NC-015 — Retain P2 semantics defect; narrow platform outcome

**Independent chain:** `APPLE/src/ui/Field.tsx:33,41–43,60–64,77–93` extracts the visible label, removes an identical placeholder, and does not default the handheld TextInput's accessibilityLabel to it. `src/screens/auth/DirectSignIn.tsx:100–101` and `HostedSignIn.tsx:245–246` are real callers relying on that label. TV's Focusable path does pass a label.

**Counterevidence and correction:** Explicit caller-supplied accessibilityLabel survives through rest props; platform heuristics might announce neighboring text in some layouts. Source alone cannot establish every precise VoiceOver utterance. Correct wording: **“The shared iOS/iPadOS Field does not establish the visible label as the input's programmatic accessible name by default.”** Retain established missing semantics and device-unverified impact; do not label TV affected by this exact branch.

**Fix pitfalls / verification:** Preserve explicit overrides and distinguish name from hint/error; avoid announcing both label and duplicated placeholder. Validate empty, secure, error and dynamically sized fields with VoiceOver on hardware after code repair.

### NC-016 — Uphold conditional P2; limit credential-forwarding claims

**Independent chain:** `APPLE/tvos/TopShelf/ContentProvider.swift:130–143` checks host rather than exact origin before attaching the limited feed bearer. Normal server entry construction uses relative expected paths (`SERVER/server/internal/httpapi/topshelf.go:216–219`).

**Counterevidence and correction:** No attacker-controlled canonical feed route or malicious response was exercised. Different-scheme requests may encounter ATS; Foundation redirect behavior may remove Authorization. Those runtime policies are not established by this source. NC-019 currently strips the normal artwork grant, an independent interoperability failure. Correct wording: **“If a supplied feed contains a same-host/different-port accepted URL, the code lacks exact-origin validation before initially attaching its feed bearer; other redirect/downgrade leakage remains platform-dependent.”** Do not report demonstrated cross-host redirect exfiltration.

**Fix pitfalls / verification:** Check normalized scheme/host/effective port, path grammar and userinfo before attaching credentials; explicitly handle redirects. Preserve the legitimate per-item grant required by NC-019. Test actual URLSession policies rather than assuming all headers follow all redirects. Keep this outside established-defect impact counts.

### NC-017 — Strongly narrow; retain conditional P2 only

**Independent chain:** `APPLE/src/bridge/offline-downloads.ts:411–428` indexes keys globally by key ID, without server namespace. A deliberately same-ID key can replace another verifier. This affects the iOS/iPadOS coordinator only.

**Strong counterevidence:** `SERVER/server/internal/downloads/receipts.go:130–143` creates the key ID from `identity.Digest(identity.Token())[:32]` alongside random Ed25519 key generation. The implementation does not use a predictable shared constant or short per-server counter. No practical honest-server collision is established. A copied identical key/ID is not itself a conflicting verifier. Cold verification against a wrong overwritten key normally **fails closed**, causing availability failure, not signature bypass; warm-cache behavior is separately covered by NC-003.

**Correct wording:** **“Server namespace is absent from remembered signing-key storage. Conflicting IDs supplied deliberately or through corrupted/imported state could replace a verifier; ordinary independent canonical servers are not shown to collide.”** Conditional P2 is reasonable for adversarial namespace isolation; absent that threat model it is defense-in-depth, not a demonstrated production outage. Do not count an unproven collision as an established vulnerability.

**Fix pitfalls / verification:** Namespace by verified server identity and key fingerprint, migrate existing records without accepting arbitrary claimed owners, and invalidate corresponding cached verdicts. Test intentional same-ID fixtures and cold/warm behavior; do not use an artificial collision fixture to claim natural collisions are likely.

### NC-018 — Uphold P3 acceptance gap; no demonstrated manifest violation

**Evidence challenged:** `APPLE/Native/PorticoDownloads.m:364` uses important-usage disk-capacity APIs. `Native/PorticoAudioRender.m:57–60` also reads capacity, so checking only the download-enabled platform would miss another native caller. The checked-in iOS/tvOS app privacy files enumerate other categories; they are not the final merged product manifests.

**Counterevidence and corrected wording:** Store requirements, selected reason applicability, linked dependency manifests and final signed artifact contents were not established in this code-only pass. No external policy lookup, build or submission was authorized. Correct wording: **“Confirm disk-capacity API declarations and applicable reasons in each shipping merged manifest; native/platform acceptance remains unproved.”** Do not turn this into “missing required declaration” or “App Store rejection” without that evidence. The broader provisioning, AirPlay/PiP/Cast/VoiceOver acceptance list is a verification plan, not a set of additional discovered defects.

**Fix pitfalls / verification:** Do not copy a reason code simply to satisfy a scanner. Inspect the final manifests and actual use against current primary platform requirements when that work is authorized; perform signed-device acceptance independently of source checks. Retain P3 and exclude from established code-defect count.

### NC-019 — Uphold established P2

**Independent chain:** `SERVER/server/internal/httpapi/topshelf.go:136–141,216–219` supplies and requires a query artwork grant. `APPLE/tvos/TopShelf/ContentProvider.swift:130–143` replaces the query with size=thumbnail. Its Authorization feed token is not consulted as a substitute by the art handler. The extension omits entries without a poster (`179–186`).

**Counterevidence and correction:** Cached posters bypass acquisition, so the correct trigger is a fresh/uncached card; successful feed JSON is not sufficient. No intermediary adapter or alternate Authorization branch was found in this path. Deterministic source contract mismatch is sufficient to retain P2 without claiming a device reproduction.

**Fix pitfalls / verification:** Preserve the server-authored grant and supported sizing after strict destination validation; do not make art generally accessible or weaken auth to accommodate the client. Combine with NC-001 owner cache cleanup and NC-016 destination checks. Verify cold cache, new item, expiry/refetch and rejected grants together.

### NC-020 — Uphold established P2

**Independent chain:** `APPLE/src/app/home.ts:84–103` captures the document object per row request; a successful row append changes it. Another successful row response returns early on document reference inequality and retains its paging marker. Home disables its next callback while that row is marked (`src/screens/Home.tsx:112–114`). Independent shelf actions (`281–318`) permit different rows to be requested before either settles.

**Counterevidence and correction:** Per-row cursor and document-generation checks are present, errors clear markers, and full refresh recovers. None makes unrelated rows share a single serialized request. Correct wording retains a **valid second success discarded until refresh** rather than permanent application failure.

**Fix pitfalls / verification:** Fence row/cursor/generation without whole-document pointer equality, and retire the exact completed request marker even when discarding success. Unconditional cleanup must not clear a newer request's marker. Test both completion orders, same-row duplicates, full refresh, errors and owner change.

### NC-021 — Uphold P2; narrow when refresh fails

**Independent chain:** `APPLE/src/app/channels.ts:29–69` holds module-global stores keyed by viewer/origin with a constructor-captured API and no rebinding. `src/app/session.tsx:368–375` creates a new authenticated API for selected viewer; sign-out (`995–1020`) retires the session. `RN/packages/client-core/src/index.ts:202–282` holds token on the API instance; normal refresh mutates the current instance rather than all retired stores. Stable-instance token rotation is distinct from a new sign-in object.

**Counterevidence and correction:** Cache size is four; eviction/restart recovers. A failed server logout or still-valid token may let the old API work temporarily, so refresh does not necessarily fail immediately after every sign-in. Correct wording: **“Same-viewer/origin re-sign-in can reuse stale cached sources and a retired API; refresh uses old credentials and fails once those credentials or route authority are revoked/expired.”** It is not a blanket cross-profile data leak because viewer is part of the key.

**Fix pitfalls / verification:** Rebind with cancellation/generation or scope stores to session lifetime. Do not discard useful state on every token rotation of the same object unnecessarily. Test new API/same viewer, stable API/new token, revocation failure, initial-load failure and eventual cache eviction.

### NC-022 — Retain P2 missing semantics; narrow exact VoiceOver consequence

**Independent chain:** `APPLE/src/player/Timeline.tsx:21–35,39–60` exposes handheld seeking only through PanResponder; TV has an adjustable role and text value but no accessibility increment/decrement actions/handler. Inspection of `src/ui/Focusable.tsx:42–44,72–117` found no wrapper implementation that supplies those missing actions. Ordinary remote handling elsewhere is not evidence of an assistive action mapping.

**Counterevidence and correction:** Separate skip controls provide an alternative, and source alone does not prove every OS/version screen-reader gesture or announcement. Correct wording: **“The custom timeline does not implement programmatic accessible adjustment semantics on handheld or TV; whether and how VoiceOver users are blocked must be confirmed on target devices.”** This does not mean all playback seeking is inaccessible. Retain established semantics defect separately from device acceptance.

**Fix pitfalls / verification:** Provide role/value/range/actions with bounded seek steps, disabled/live state and sensible announcement frequency. Do not assume declaring adjustable supplies the callback. Exercise VoiceOver plus ordinary remote/touch after repair, including unknown duration, live windows and pending seeks.

### NC-023 — Uphold P2; replace “permanently” with controller lifetime

**Independent chain:** `APPLE/src/bridge/useNativePlayback.ts:84–85,92–133` permits early queue creation with false transitions and empty kinds. `native-listening-kinds.ts:21–24` closes over the original array; later assignment does not mutate that array. `playback-paths.ts:31–36` preserves an existing holder controller. `RN/packages/client-core/src/playback-v1/v1-queue-player.ts:148–151,374–396` retains and uses those options. Capability completion exits when a controller already exists.

**Counterevidence and correction:** Later controller recreation, local-mode transition or relevant dependency/session changes can recover. This is not permanent installation state, nor universal audio failure. Correct wording: **“An early play can freeze reduced capability selection for the lifetime of that installed queue controller, including later tracks on it.”** Native background ownership and transitions are the affected choices; actual background termination timing was not exercised.

**Fix pitfalls / verification:** A capabilities gate needs timeout/error behavior so it does not block all playback indefinitely. Updating capability options must not replace a playing owner without a coordinated handoff. Test delayed success, failure/retry, early video followed by audio, controller recreation and already active native playback.

### NC-024 — Move to conditional P2; final OS ordering unresolved

**Independent source fact:** `APPLE/Native/PorticoActivity.m:75–104` captures up to 100 titles, requests asynchronous domain deletion, then indexes that captured array in its callback. `clear:110–114` requests a separate deletion without invalidating the earlier callback. Home initiates indexing (`src/screens/Home.tsx:43–49`); session transitions do call clear (`src/app/session.tsx:794,998,1026,1071`). There is no application owner epoch in this native callback.

**Important counterevidence:** `PorticoActivity.m:46` puts exported methods on the main queue. Native JS calls are therefore already serialized; this is not a simultaneous mutable-state race. The Siri title-cache file is written synchronously in indexTitles and removed synchronously in clear; the asynchronous callback only indexes Spotlight and does **not** recreate that file. CoreSpotlight may impose ordering/transaction behavior that determines final visibility; none was tested or established from the source inspected. A 30-day requested expiry is not proof of 30-day persistence after cleanup.

**Correct wording and classification:** **“A pending Spotlight deletion callback has no owner-generation check before submitting its captured titles. Depending on CoreSpotlight completion/operation ordering, stale titles may be submitted after a later privacy clear; final post-clear visibility is unresolved.”** Move out of established defect count into conditional P2. Keep high confidence in the missing source fence, not high confidence in a demonstrated disclosure.

**Fix pitfalls / verification:** Add owner epoch and serialize the complete asynchronous deletion/index sequence, checking on the owner queue. A check before index submission alone cannot cancel an already submitted old insertion or guarantee a later clear is the final durable operation. Never block the main thread waiting for OS completion. Use a controllable index adapter to test reorderings, then confirm final Spotlight results on signed devices; separately verify direct Siri cache deletion remains correct.

## Review coverage and limits

Every NC ID received an explicit verdict. Targeted independent reads covered Root/Shell/session boundaries, Top Shelf JS/native/extension and server grant/artwork handlers; both route delegates and Swift fragment parsing; offline request enrollment, receipt parsing/signing/renewal, native persistence/start/storage, coordinator ownership and verification; audio range callbacks, system artwork and interruption handling; player preflight/leave and queue capability closure; Search/Saved paging/grid, fields, timeline/Focusable, Home paging, channel store/API identity, and native Activity callbacks. Existing report references were compared with the relevant caller/guard paths; no runtime result was inferred from a test file or a compilation assumption.

This pass does not re-audit every package, dependency or source line. It cannot determine OS scheduling, actual memory pressure, current App Store policy, signed entitlements, device accessibility behavior or deployed historical artwork-cache prevalence. Resource and ordering findings identify reachable missing bounds/guards; their maximal symptoms remain scenarios to verify. The classification changes above are intended to prevent those proof gaps being counted as demonstrated incidents.
