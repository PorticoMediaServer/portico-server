# V10 — Verification of native architecture, native UX parity, and client connectivity

## Summary
All **24 Tier 1** entries and **29 Tier 2** entries were checked; eight Low entries are listed without re-checking as requested. Tier 1: **7 Confirmed, 12 Confirmed, narrower, 5 Unsettled — needs runtime**. Tier 2: **16 Confirmed, 12 Confirmed, narrower, 1 Unsettled — needs runtime**. No entire numbered finding was refuted; two specific subclaims were refuted: B-22.7’s IPv6 scheme description and B-22.9’s native default-fetch assertion. B-22.13’s event/id discussion was narrowed to distinguish absent separate limits from existing per-line bounds. “Confirmed” here describes independently traced source behavior, not runtime reproduction.

The strongest release-relevant confirmations are the incompatible secure LAN signed-route producer/consumer (B-22.1/2), shared HTTP-tab refresh ownership (B-22.3), transient storage failures classified as session endings (B-22.4), and native auxiliary streams bypassing the selected route (B-22.18). Token renewal is also a concrete trigger for native service churn (B-19.2) and an idle Saved replacement (B-21.13), rather than merely a hypothetical profile-switch race. Current server authorization explicitly supports Hosted owner metadata editing, strengthening B-21.12.

Several first-pass severities or scopes exceeded established impact. Saved/history work grows with visited pages rather than automatically loading the whole library; unsigned replacement cleanup is a recovery/availability defect; Android implicit retry is a policy defect with conditional duplicate effects. Plain HTTP’s inability to authenticate future sockets is a recorded transport trade-off, with residual risk and an overbroad trust promise to clarify. Apple archive validation/scenes, native plural availability, exact Menu callback behavior, assistive menu access and same-link multicast behavior remain explicit acceptance checks. No additional numbered finding was needed.

## Verdict table
| ID | Claim (short) | First pass (confidence / severity) | Verdict | Class | Your severity | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-19.2 | Every access-token renewal rebuilds the data services of every mounted screen | Confirmed recreation; probable effect / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-19.4 | If the Android Keystore key is lost, the app can never read or save most of its records again | Confirmed logic; speculative frequency / Medium (rare, but permanent and invisible) | Confirmed, narrower | Defect | Medium | M | No |
| B-19.7 | The iPad build lists three orientations, which App Store validation normally rejects for a multitasking app | Probable / Medium only because it blocks an upload; the fix is one line | Unsettled — needs runtime | Design risk | Medium, conditional | S | No |
| B-19.13 | The windowed grid assumes equal row heights and exact pixel arithmetic at any size | Speculative / Low today; Medium at scale | Confirmed, narrower | Defect | Medium | M | No |
| B-19.17 | Plural rules may silently fall back to English on devices | Speculative / Low now; Medium for non-English | Unsettled — needs runtime | Design risk | Low | M | No |
| B-19.18 | No scene manifest in the Apple apps | Speculative SDK effect / High if SDK requirement applies | Unsettled — needs runtime | Unfinished feature | High, SDK-conditional | M | No |
| B-21.5 | History and adjacent appended saved lists grow unwindowed | Confirmed by reading / High | Confirmed, narrower | Defect | Medium | M | Yes, History only |
| B-21.12 | Hosted owners are blocked from native metadata edit by authority gate | Probable / Medium | Confirmed | Defect | Medium | S | No |
| B-21.13 | Saved hook can skip selecting a replacement service with the same route | Probable / Medium | Confirmed | Defect | Medium | S | No |
| B-21.14 | Async item deep links have no visible intent/viewer fence | Probable / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-21.16 | TV long-press menus lack a proven visible accessible alternative on some card paths | Speculative / Medium | Unsettled — needs runtime | Unfinished feature | Medium, conditional | M | Yes, recommendation path |
| B-22.1 | Secure LAN direct names invalidate signed route documents | Confirmed by reading / High | Confirmed | Defect | High | M | No |
| B-22.2 | Multiple secure LAN addresses collapse to duplicate signed origins | Confirmed by reading / High | Confirmed | Defect | High | S/M | No |
| B-22.3 | Plain-HTTP tab-local locks cannot protect a shared rotating credential | Confirmed by reading / High | Confirmed | Defect | High | M | No |
| B-22.4 | A temporary protected-store read failure is classified as sign-out | Confirmed by reading / High | Confirmed | Defect | High | M | Related symptom only |
| B-22.5 | Unsigned replacement detection can discard a valid saved family | Confirmed by reading / High | Confirmed, narrower | Defect | Medium | M | No |
| B-22.6 | A ready HTTP route is not bound to future sockets | Confirmed by reading / High (design risk) | Confirmed, narrower | Deliberate trade-off | Medium | S/L | No |
| B-22.14 | JSON proxy errors can leave a dead route classified ready | Probable / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-22.15 | Native FIFO can starve renewal/timeline behind reads | Probable / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-22.17 | Known-server labels can retain obsolete logical addresses | Probable / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-22.18 | Auxiliary native streams bypass route selection and verification | Confirmed by reading / High | Confirmed | Defect | High | M | No |
| B-22.19 | Vega rejects redirects after credential transmission can occur | Confirmed late refusal; SDK forwarding unknown / High | Confirmed, narrower | Defect | High, forwarding-conditional | M/L | No |
| B-22.20 | Android inherits implicit retries of replayable mutation bodies | Confirmed policy mismatch; probable side effects / High | Confirmed, narrower | Defect | Medium | M | No |
| B-22.24 | Same-link multi-interface mDNS advertisements may conflict with each other | Speculative / Medium | Unsettled — needs runtime | Design risk | Medium, conditional | M | No |
| B-19.1 | A link that launches the app from cold is thrown away | Confirmed by reading / Medium (home-screen cards are a primary TV entry point; Fire and Android TV kill apps often) | Confirmed, narrower | Defect | Medium | M | No |
| B-19.3 | On Android TV and Fire TV the remote's Menu key presses the focused control | Confirmed by reading / Medium (unexpected activation of destructive buttons) | Unsettled — needs runtime | Defect if activation occurs | Medium, conditional | M | No |
| B-19.6 | An uncaught JavaScript error outside rendering ends the app with no record | Confirmed by reading / Medium as a risk multiplier: it turns any small bug into an unexplained crash | Confirmed, narrower | Missing feature mislabelled as a bug | Low | M | No |
| B-19.10 | A television that does not report "television" gets a touch layout with no visible focus | Confirmed by reading / Low to Medium depending on how many owners use such boxes | Confirmed, narrower | Design risk | Medium, hardware-conditional | M | No |
| B-19.12 | On Apple, deleting the app does not sign it out | Confirmed by reading / Low to Medium (privacy on a device that changes hands; a surprising platform difference) | Confirmed, narrower | Design risk | Low | M | No |
| B-19.14 | Mounted screens are never released or frozen | Confirmed by reading / Low Apple TV; potentially Medium weak TVs | Confirmed, narrower | Design risk | Medium on weak devices | M | No |
| B-21.1 | Play on has no permanent Enter a TV Code action | Confirmed by reading / Medium | Confirmed | Unfinished feature | Medium | S | No |
| B-21.2 | Programme-sheet Cancel Recording bypasses confirmation | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-21.3 | Wide-tablet Downloads loses its primary navigation entry | Confirmed by reading / Medium | Confirmed | Unfinished feature | Medium | S | No |
| B-21.4 | Saved Spotlight and Siri index reads a deliberately absent Home row | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-21.6 | Published native preferences can be consumed or stored without a reachable control | Confirmed by reading / Medium | Confirmed, narrower | Unfinished feature | Medium | M | Partial, card size |
| B-21.7 | This device wording actually controls the whole device class | Confirmed by reading / Medium | Confirmed | Documentation error | Medium | S | No |
| B-21.8 | Playing a duplicate playlist occurrence starts the first matching item | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-21.9 | Pre-play subtitles omit unknown-language tracks and merge distinct same-language choices | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-21.10 | Vega exposes unsupported speed and audio-effect preferences | Confirmed by reading / Medium | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-21.11 | Saved View Play and Shuffle only act on the loaded prefix | Confirmed by reading / Medium | Confirmed | Unfinished feature | Medium | M | No |
| B-22.7 | Manual scheme guessing omits the default port and misclassifies hosts | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-22.8 | General browser requests can remain pending without a deadline | Confirmed by reading / Medium | Confirmed | Design risk | Medium | M | No |
| B-22.9 | Server-list watch single-flight can wedge on a hung fetch | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-22.10 | Local refresh ignores server retry floors and can bypass its own timer | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | Related symptom only |
| B-22.11 | Playback and watch parsing ignore HTTP-date Retry-After | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-22.12 | Browser Hosted errors discard Retry-After | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S/M | No |
| B-22.13 | SSE frame cap does not bound accumulated parsed data | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-22.16 | Browser clock ahead by roughly 30 seconds rejects fresh route proof | Confirmed by reading / Medium | Confirmed | Design risk | Medium | M | No |
| B-22.21 | HTTP-only discovery cannot bootstrap a required-HTTPS server | Confirmed by reading / Medium | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-22.22 | A successful remote route may never return to LAN without a path event | Confirmed by reading / Medium | Confirmed | Design risk | Medium, network-conditional | M | No |
| B-22.23 | Cross-origin web clients cannot read Report-Every-Ms | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-22.25 | Event retries can synchronize on a large server-provided floor | Confirmed by reading / Medium | Confirmed | Design risk | Medium | S | No |
| B-22.26 | Native Hosted activation polling bypasses the shared outage gate | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |

## Tier 1 — detailed verification

References are relative to `/home/justin/projects/Portico`. Where a paragraph starts with a checkout path, subsequent shortened paths use that same checkout/root unless explicitly marked server, web, Hosted, or canonical core. Native source: `apps/portico-react-native/apps/native/`; native core copy: `apps/portico-react-native/packages/client-core/`; canonical core: `apps/portico-server/packages/client-core/`. Both implementations were read at the relevant boundaries; no shared-package synchronization command was run.

### B-19.2 Token renewal rebuilds viewer-scoped screen services
- **Claim:** Renewing an access token needlessly recreates mounted screen services and reloads their projections.
- **What I traced:** Native `apps/portico-react-native/apps/native/src/app/session.tsx:262` wires `onRotated` to `viewer.rotate`; `apps/portico-react-native/packages/client-core/src/index.ts:535` publishes a new session without changing generation. `packages/client-core/src/presentation/content.ts:12` in that checkout returns a fresh scope object; native `src/app/content.ts:17` uses its identity to construct a service, disposes the previous service at line 25, and selects again at line 32. `src/screens/Saved.tsx:51`, `src/app/detail.ts:15–16,44–45`, and `src/app/section-window.ts:13–18` follow the same reconstruction path; browse retains its store independently at `src/app/browse.ts:64`. These are live hooks, not unused adapters.
- **Verdict:** **Confirmed, narrower.** The recreation is certain for hooks keyed this way, not literally every screen/data service; browse is keyed differently. Focus loss and the length of a visible loading flash remain device questions. Saved can do worse than refetch: B-21.13 leaves its replacement idle. Core's unchanged viewer generation does not protect React object-identity dependencies.
- **Class:** Defect.
- **Severity:** **Medium** — active native viewers hit it during ordinary renewal, roughly thirteen minutes after a fresh fifteen-minute token, with cumulative cost across retained screens.
- **Fix direction:** Key scope/service lifetime by stable authority/account/profile/server values while continuing to rotate bearer references. **Size: M**; native content/detail/search/section/Saved/person hooks and shared presentation scope helper.
- **Related:** B-21.13 is the same trigger with a different consequence; B-14.8/9 report analogous request-following/offline-handshake renewal work. They agree. Not specifically in Backlog.

### B-19.4 Android record-store recovery after Keystore key loss
- **Claim:** Losing the common Keystore key strands old records and prevents read-modify-write recovery.
- **What I traced:** Native Android `android/app/src/main/java/tv/getportico/portico/native/PorticoRecordStore.kt:38` reads existing files with `key(false)` and returns Unreadable on missing/decryption-failed key; line 63 allows direct writes to create a new key. `PorticoRuntime.kt:149` rejects unreadable loads. Native `src/bridge/queue-storage.ts:9` requires a successful load before any change. `PorticoProfileTrust.kt:47` generates the installation only on Missing, not Unreadable; `src/app/session.tsx:43` catches installation failure without explaining it. `packages/client-core/src/server-connections.ts:235` requires installation identity to renew.
- **Verdict:** **Confirmed, narrower.** Old encrypted records and installation identity cannot recover through these paths; direct save/delete of individual records can still work. Thus “can never save most records” means existing read-modify-write records until explicit reset, not a permanently broken binary. A new key does not decrypt files sealed by the old one. Device frequency is unestablished.
- **Class:** Defect — preserving an ambiguous transient read is sensible, but there is no explicit lost-key recovery flow.
- **Severity:** **Medium** — rare affected Android/Fire installations can remain stuck across relaunch and repeated sign-in.
- **Fix direction:** Distinguish unavailable versus irretrievably lost key, offer a verified local-store reset/reinitialization, and explain loss once. Do not automatically wipe on every storage exception. **Size: M**; RecordStore, Runtime, ProfileTrust, native session/storage UI.
- **Related:** B-22.4 can turn the same read failure into sign-out; it agrees but addresses classification rather than recovery. Not in Backlog.

### B-19.7 iPad orientation/upload validation
- **Claim:** Missing upside-down portrait with no fullscreen opt-out blocks App Store validation.
- **What I traced:** Native `ios/Portico/Info.plist:89` declares only three iPad orientations; the file has no `UIRequiresFullScreen`. The iOS target uses this plist. Apple’s current [supportedInterfaceOrientations documentation](https://developer.apple.com/documentation/uikit/uiviewcontroller/supportedinterfaceorientations) also describes incomplete orientation declarations as an opt-out from multitasking; source configuration alone therefore does not prove this exact upload rejection for the intended SDK.
- **Verdict:** **Unsettled — needs runtime.** The plist omission is verified; the claimed current validation failure is not. Cheapest check: validate the intended release archive through Xcode/App Store Connect on the Mac; ITMS-90474 confirms the claim, successful validation refutes the blocker.
- **Class:** Design risk (release configuration), pending validator evidence.
- **Severity:** **Medium, conditional** — affects release upload if the intended validator rejects it, not current playback.
- **Fix direction:** Add upside-down portrait if all orientations are intended; validate rather than opting out of tablet multitasking casually. **Size: S**; iOS Info.plist and archive validation.
- **Related:** B-25.10 lacks Apple release acceptance; no duplicate defect. Not in Backlog.

### B-19.13 Windowed-grid row geometry and large offsets
- **Claim:** Row-zero height cannot safely describe every card row, and huge offsets may lose native precision.
- **What I traced:** Native `src/ui/Grid.tsx:129` derives one stride, line 135 jumps by that stride, line 157 measures only row zero, and line 171 supplies equal-height `getItemLayout`. `src/ui/Card.tsx:61` uses a minimum caption height, with variable title-line count at line 62; `src/ui/Text.tsx:34` permits Dynamic Type. The windowed Library/HomeRow card renderer uses `src/screens/Home.tsx:223` two-line book captions (`src/screens/Library.tsx:310` uses the grid). Loaded rows need not all have the same measured height. Windowing itself is real and bounded (Grid lines 178–183).
- **Verdict:** **Confirmed, narrower.** The row-height assumption is a source-level defect for variable captions/font sizes. The specific million-item precision impact and blank-band magnitude are **not** established, nor is the assertion that every competing native list avoids large-offset issues. Cheapest follow-up: put short titles in row zero and wrapped book titles below, then jump near the end under normal and enlarged text settings; measure landing offset. Separately use a synthetic total and tail jump to examine precision on each native engine.
- **Class:** Defect (row geometry); separate latent design risk (offset precision).
- **Severity:** **Medium** — books or enlarged-text users can get incorrect jumps; extreme-library precision remains a scale acceptance risk.
- **Fix direction:** Give cards a consistent accessible row footprint or use measured variable-row layouts; investigate offset rebasing only after device evidence. **Size: M** for geometry, **L** if rebasing is required; Grid/Card/Text.
- **Related:** B-16.4 concerns the web grid’s different mapped-scroll algorithm and is not a duplicate. Not in Backlog.

### B-19.17 Plural fallback on native runtimes
- **Claim:** A native runtime without Intl.PluralRules silently uses English rules for future languages.
- **What I traced:** Native checkout `packages/i18n/src/messageformat.ts:111` caches Intl rules or null; lines 119–123 supply English cardinal and ordinal fallback. Architecture `portico-internal/Project Architecture/06 Clients.md:19` specifies English-only launch. Source cannot establish the exact Intl implementation shipped on each device.
- **Verdict:** **Unsettled — needs runtime.** Conditional fallback is confirmed; an affected shipping runtime/language is not. Cheapest check: evaluate `typeof Intl.PluralRules` and `new Intl.PluralRules('ar').select(0)` in each release JS runtime; absence/incorrect categories confirms future-language exposure.
- **Class:** Design risk.
- **Severity:** **Low** today — launch English is covered by the fallback; reassess before non-English catalogues ship.
- **Fix direction:** Establish release-runtime Intl capability and add a bounded locale implementation if required. **Size: M**; i18n/bootstrap/runtime acceptance.
- **Related:** B-16.6 and B-18.12 concern inconsistent locale adoption, not missing plural runtime support. Not in Backlog.

### B-19.18 Apple scene lifecycle
- **Claim:** Classic app-delegate windows may fail to launch under the SDK that requires scenes.
- **What I traced:** Native `ios/Portico/AppDelegate.swift:17,47` and `tvos/Portico/AppDelegate.swift:16,36` own classic windows; neither target’s Info.plist supplies a scene manifest. Podfiles mention Xcode 27. Apple’s [WWDC25 UIKit announcement](https://developer.apple.com/videos/play/wwdc2025/243/) states that the release after iOS 26 requires scenes for apps built with its latest SDK. That confirms a real migration requirement, not the effective merged plist or launch of this unbuilt archive.
- **Verdict:** **Unsettled — needs runtime.** No source-level scene integration was found. Cheapest check: inspect the built release plist and launch the intended Xcode/SDK archive once on each Apple target; a scene-required launch error confirms the blocker, a successful launch establishes that target’s acceptance.
- **Class:** Unfinished feature (required lifecycle migration under the new SDK).
- **Severity:** **High, conditional on SDK** — a launch blocker for affected new-SDK builds; not proof that older shipped binaries fail.
- **Fix direction:** Integrate a SceneDelegate/RN factory lifecycle, move window ownership, and preserve URL/activity/audio callbacks. **Size: M**; both Apple app delegates, plists, URL/intents wiring.
- **Related:** B-25.10 is missing physical/Apple acceptance, not a duplicate implementation bug. Not in Backlog.

### B-21.5 Appended Saved/history lists are not windowed
- **Claim:** Visited pages stay mounted and work grows with all loaded Saved entries.
- **What I traced:** Native `src/screens/Saved.tsx:288` appends history pages, scans every entry at line 295, and renders every day at line 315; HistoryGrid uses a nested grid. `src/ui/Grid.tsx:54` explicitly maps every row when scrolling is disabled. Saved resource lists map accumulated rows at Saved line 361; `src/screens/title/SavedPages.tsx:140` maps playlist occurrences and line 200 appends resource media before the nested full-height grid at line 230. My List/favorites have a separate windowed path.
- **Verdict:** **Confirmed, narrower.** Work grows with **pages visited**, not automatically the entire library; there is no observed responsiveness or memory threshold. Still violates the recorded O(visible) client rule.
- **Class:** Defect.
- **Severity:** **Medium** (High → Medium) — long browsing sessions/large personal lists on weak TV hardware, rather than an unconditional high-impact failure.
- **Fix direction:** One windowed scroller with stable group headers and occurrence keys; retain complete pagination. **Size: M**; Saved, SavedPages, shared personal/saved window adapters.
- **Related:** B-19.14 increases retained-screen cost but is independent. **Backlog: yes**, History at `Backlog.md:65`; adjacent lists extend it.

### B-21.12 Hosted owner metadata actions are hidden
- **Claim:** Native owner editing is incorrectly restricted to direct/local authority.
- **What I traced:** Native `src/player/EntryMenu.tsx:49` and `src/screens/Show.tsx:64` require local owner and non-TV. Shared `packages/client-core/src/presentation/detail-actions.ts:105` repeats it. Actual server `server/internal/httpapi/router.go:794` registers editor routes; `metadata_editor.go:29,51` uses `manualMetadataOwner` and `manualMetadataAuthorize`. `manual_metadata.go:38,44` dispatches Hosted owners to `ownerAuthorityTx`; `owner_authority.go:30` accepts Hosted authority with current membership checks. The stale local-only helper comment is not the production Hosted guard. `owner_authority_test.go:122` tests that transaction seam.
- **Verdict:** **Confirmed.** The server supports currently authorized Hosted owners; native’s authentication-method gate is narrower than server authorization. No deliberate local-only metadata exception was found.
- **Class:** Defect.
- **Severity:** **Medium** — Hosted owners on phones/tablets cannot reach ordinary metadata editing; TV exclusion is a distinct platform policy.
- **Fix direction:** Gate owner actions on server-supported/current authority, remove the extra local restriction, preserve server authorization. **Size: S**; EntryMenu, Show, detail-actions plus matching web policy review.
- **Related:** B-18.6 is about unsupported refresh entity kinds, not this authority gate. Not in Backlog.

### B-21.13 Saved replacement service stays idle
- **Claim:** The Saved hook’s route-only selection guard suppresses initialization of a replacement service.
- **What I traced:** Native `src/screens/Saved.tsx:51–61` recreates and resets the service on session scope change, but line 69 skips selecting when the route string matches. `src/app/content.ts:32` and Saved line 97 correctly also compare service identity. Renewal wires through `src/app/session.tsx:269` without unmounting Shell.
- **Verdict:** **Confirmed.** B-19.2 supplies a routine source-proven trigger: token rotation changes session identity while My List/favorites route remains the same. This no longer needs a hypothetical server/profile transition. Core cannot initialize a service whose select method is never called.
- **Class:** Defect.
- **Severity:** **Medium** — a mounted Saved tab can reset to idle at normal renewal and stay empty until route change/remount; no data leak is claimed.
- **Fix direction:** Track service plus route in the guard, alongside stabilizing scope lifetime. **Size: S**; Saved hook.
- **Related:** Merge with B-19.2 for implementation planning; both agree. Not in Backlog.

### B-21.14 Late deep-link intent lacks a viewer fence
- **Claim:** An older asynchronous item link can navigate/play after a newer intent or viewer choice.
- **What I traced:** Native `src/shell/Shell.tsx:258` snapshots navigator and starts `api.item`/`routeForLinkedTitle`; lines 266–268 use completion without intent generation, abort, or a current-viewer check. Playback uses the current player-intent ref, while the request used the prior API. No coalescing is applied to repeated push actions. `Product Contracts/navigation-and-input/contract.md` NAV-11/NAV-15 require scoped pending intent/coalescing.
- **Verdict:** **Confirmed, narrower.** The missing publication fence and duplicate-push possibility are certain. Lower authorization/services can reject content, so stale **intent** is not proof of cross-profile content disclosure or successful wrong-server playback.
- **Class:** Defect.
- **Severity:** **Medium** — users opening links during slow lookup and then switching viewer or opening a newer link.
- **Fix direction:** Own pending links by viewer/server and monotonically increasing intent generation; abort/fence completions and coalesce semantic routes. **Size: M**; Shell, link-intent/navigation integration.
- **Related:** B-19.1 loses cold-start links; same owner should solve both. Not in Backlog.

### B-21.16 Accessible alternative to TV long-press actions
- **Claim:** Recommendation dismissal and playlist row management lack an established accessible alternate affordance.
- **What I traced:** Native Home `src/screens/Home.tsx:213,293` routes card long press to More; `src/screens/title/SavedPages.tsx:167–168` enters playlist management by long press. `src/ui/Focusable.tsx:108` passes long press through, but does not synthesize accessibility actions; these callers supply none. Card/ListRow do not add a dedicated visible More action for these paths. `Product Contracts/accessibility-and-language/contract.md` A11Y-4.9 requires alternative access.
- **Verdict:** **Unsettled — needs runtime.** Explicit alternate UI is absent, but source absence does not prove every platform’s assistive activation/remote Menu behavior is unusable. Cheapest check: focus these exact rows with VoiceOver/Switch Control on tvOS and TalkBack on Android; try offered actions and ordinary remote Menu/hold. If reorder/remove and Not interested cannot be reached without hold, the reachability defect is confirmed.
- **Class:** Unfinished feature (alternate affordance/acceptance).
- **Severity:** **Medium** — assistive/limited-input TV users, conditional on runtime behavior.
- **Fix direction:** Add a focusable More action or explicit semantic action that opens the same menu. **Size: M**; card/row primitives, Home, SavedPages and accessibility acceptance.
- **Related:** B-19.3’s synthetic Menu is not a proven accessible replacement. **Backlog: yes**, Not interested at line 9; playlist alternate is extra scope.

### B-22.1 Secure LAN name rejected by signed-route verifier
- **Claim:** A secure claimed-server LAN alias invalidates the complete signed route document.
- **What I traced:** Server `apps/portico-server/server/internal/networking/remote_manager.go:411–418` publishes HTTPS LAN literals when TLS is ready; Hosted `portico-internal/hosted-services/hosted/internal/postgres/routes.go:164–175` rewrites them to LANHostname and keeps class lan. DNS `hosted-services/dns/directdns/table.go:35` names them under direct.getportico.tv. Canonical core `apps/portico-server/packages/client-core/src/route-identity.ts:42–46,82` only accepts private literals/localhost/.local for lan. `server-connections.ts:59` verifies the whole document before constructing/racing discovery. Hosted `directory/routes.go:142–151` signs the transformed array without another consumer validation.
- **Verdict:** **Confirmed.** Native mDNS does not rescue first Hosted connect because rejection precedes its race. Existing remembered local sessions may still work, so this is not every claimed-server session.
- **Class:** Defect.
- **Severity:** **High** — new/recovery Hosted connection to a claimed server publishing ready secure LAN candidates fails before valid WAN candidates are tried.
- **Fix direction:** Define and validate the exact signed LAN alias form/address mapping on both producer and consumer; preserve pin verification. **Size: M**; Hosted RouteSets/directdns, core route-identity, server candidate tests.
- **Related:** B-22.2 independently also invalidates the document; B-22.21 limits the mDNS fallback. Not in Backlog.

### B-22.2 Secure LAN candidates collapse to duplicate aliases
- **Claim:** Multiple LAN IPs on the same port produce duplicate signed candidate URLs.
- **What I traced:** Server `remote_manager.go:411` loops each LAN address. Hosted `hosted/internal/postgres/routes.go:167` uses only namespace/label, losing the source address; line 175 appends every result. `hosted/internal/directory/routes.go:118` deduplicates incoming literals **before** this transform, whereas line 146 signs its final result unchanged. Core `route-identity.ts:83` rejects duplicate final origins.
- **Verdict:** **Confirmed.** Fixing only the private-host classifier in B-22.1 leaves this independent rejection for same-port multi-address servers (including dual-family cases).
- **Class:** Defect.
- **Severity:** **High** — Hosted connect/recovery for these secure multi-address servers loses the whole candidate set.
- **Fix direction:** Deduplicate and consumer-validate after normalization; document whether aliases represent a bounded multi-address DNS set or interface-specific names. **Size: S** for deduplication, **M** for changing alias model; Hosted RouteSets/directdns and core conformance cases.
- **Related:** B-22.1 agrees; repair together. B-22.24 is separate multicast behavior. Not in Backlog.

### B-22.3 HTTP browser tabs race rotating credentials
- **Claim:** Tab-local fallback locks cannot serialize a shared IndexedDB refresh family.
- **What I traced:** Web `apps/portico-server/web/src/app/polyfills.ts:52,58` installs a per-runtime Map lock when Web Locks is absent; `web/src/app/session.tsx:60` registers navigator locks as credentialLock. BrowserProtectedStore `web/src/bridge/server-connections.ts:21,32` separately reads and writes using that fallback. Core `server-connections.ts:228–246` reads, generates/persists a request ID, then sends its own captured ID. Overlapping contexts can both read the old credential and choose different IDs. Server `internal/identity/refresh.go:39–51` revokes on predecessor reuse outside exact receipt replay.
- **Verdict:** **Confirmed.** Different tab-local locks plus separate read/write transactions do not provide a cross-tab critical section. The second-tab test (`packages/client-core/test/session-refresh.test.ts:250`) runs sequentially under one in-process lock (`:115`); it does not refute the overlapping-browser case. The test titled plain HTTP (`:264`) explicitly removes refreshToken, unlike the actual version-2 browser vault.
- **Class:** Defect — first-class HTTP is deliberate; incorrect credential coordination is not.
- **Severity:** **High** — two same-origin ordinary LAN-HTTP tabs renewing together can revoke the shared family and strand both.
- **Fix direction:** Real cross-context credential ownership/serialization with crash-safe pending IDs, or independent per-tab families. **Size: M**; polyfills, BrowserProtectedStore, credentialLock/session coordination.
- **Related:** B-16.2 reports shared-tab selection/sign-out interference, a distinct race in the same ownership boundary. Not in Backlog.

### B-22.4 Storage read failure ends a valid session
- **Claim:** A transient protected-store read is treated as a replaced family and causes durable cleanup.
- **What I traced:** Core `server-connections.ts:230` catches parse/read errors and substitutes undefined; line 231 throws signOut=true. `session-refresh.ts:139–148` invokes onSignedOut. Native `src/app/session.tsx:1053,1075–1084` clears selection/connection; web `src/app/session.tsx:991–998,983–986` disconnects and forgets. Native load failures are explicitly different from Missing (`PorticoRuntime.kt:149`), but core erases that distinction.
- **Verdict:** **Confirmed.** Server refusal is not required. Later cleanup may fail if the store remains unreadable, but once readable it can discard a valid connection. Network exceptions from the refresh request are preserved elsewhere and do not guard the earlier store read.
- **Class:** Defect; violates networking contract §11’s preserve-on-unknown rule.
- **Severity:** **High** — ordinary native/browser storage unavailability can discard recoverable sign-in without authority evidence.
- **Fix direction:** Preserve unreadable/corrupt/missing/replaced distinctions; retry unavailable storage, explicitly handle corruption, clean up only on definitive refusal/selection loss. **Size: M**; server-connections, session-refresh, both app restore/session adapters.
- **Related:** B-19.4 overlaps on Android trigger. Backlog line 56 lists unexplained Apple sign-outs, **related symptom only**; this cause is not established for those incidents.

### B-22.5 Unsigned replacement hint triggers forgetting
- **Claim:** Another server ID at the old address causes loss of valid pinned credentials.
- **What I traced:** Web `src/bridge/restore-policy.ts:25–45` reads unsigned system JSON, then returns reset on different ID; `src/app/session.tsx:283,299` calls signInEnded, whose line 986 forgets the protected connection. Native `src/app/session.tsx:201–208,496–503` does the same and its signInNeeded line 1080 forgets. Native helper uses raw fetch/default redirects. The preceding signed reconnect may fail; this fallback does not require possession of the saved key.
- **Verdict:** **Confirmed, narrower.** Availability/durable recovery damage is confirmed; it does not reveal credentials in this uncredentialed request or revoke the server’s family by itself. Legitimate DHCP reuse or proxy misrouting is enough; an attacker is not required.
- **Class:** Defect.
- **Severity:** **Medium** (High → Medium) — affected restores must sign in again and lose cached recovery evidence; no authenticated-data exposure shown.
- **Fix direction:** Keep old pin/family; present replacement as an untrusted hint and require explicit verified replacement selection. **Size: M**; both restore policies and sign-in-needed cleanup decisions.
- **Related:** B-22.4 shares premature cleanup; B-22.6 is a separate HTTP disclosure risk. Not in Backlog.

### B-22.6 Ready HTTP origin outlives socket verification
- **Claim:** A reused HTTP address can receive future bearer/refresh bytes without a new identity proof.
- **What I traced:** Core `route-connection.ts:100` returns a ready origin indefinitely; lines 142–149 check internal route generation, not socket peer identity. Only exceptions/non-JSON gateways trigger recovery at lines 154–159. `index.ts:364–368` reacts to 401 by renewing; `server-connections.ts:246` then posts the stored refresh body through the same ready origin. A normal 401 from a reassigned address is not a route exception.
- **Verdict:** **Confirmed, narrower.** The cached verification lifetime is real. Secret capture needs address reassignment/new socket (or an on-path adversary), neither demonstrated. A fresh proof before each request would still have a race on unauthenticated HTTP; this is fundamentally transport authentication, not simply a missing timer.
- **Class:** **Deliberate trade-off** — plain HTTP is explicitly first-class (`portico-internal/Project Architecture/01 Overview.md:51`), and `portico-internal/Product Contracts/architecture-amendments.md:21–23` records trusted-LAN HTTP as an exception and says the nonce proof does not itself prevent a relay. The identity-probe promise should be stated as initial/recovery verification, not cryptographic protection of later HTTP traffic; that documentation boundary needs alignment.
- **Severity:** **Medium** (High → Medium) — trusted-LAN HTTP users have residual bearer risk on address reassignment/untrusted path; HTTPS users have ordinary TLS hostname validation.
- **Fix direction:** Clarify the HTTP boundary and preserve/ease automatic verified LAN TLS; narrowing exposure via re-probes is only mitigation. Strong proof-bound requests would be a separately scoped protocol change. **Size: S** for documentation, **L** for request authentication; networking contract, route-connection, server identity/transport.
- **Related:** B-15.7 reports non-proof-bound bearer tokens and agrees. Do not count intentional HTTP twice as a new security defect. Not in Backlog.

### B-22.14 JSON gateway errors do not invalidate route
- **Claim:** A JSON-formatted proxy outage is mistaken for an application refusal and prevents failover.
- **What I traced:** Core `route-connection.ts:167` gatewayFailure checks 502/503/504 and content type lacking json; fetch line 154 otherwise returns response with ready state intact. `index.ts:395–412` parses an ApiError, not a topology failure. `test/route-connection.test.ts:74` deliberately expects JSON 503 to avoid recovery; it proves the overload distinction, not that every JSON response comes from Portico.
- **Verdict:** **Confirmed, narrower.** Exact source behavior is established for a problem+json proxy error. Whether an actual deployment supplies it is unknown. A genuine server_busy JSON response should continue **not** invalidating its route.
- **Class:** Design risk (ambiguous gateway provenance).
- **Severity:** **Medium** — installations whose proxies use JSON outages can remain on a dead route despite usable alternatives.
- **Fix direction:** Bounded, shared identity re-probe for ambiguous gateway status without mutation replay or reconnect storms. **Size: M**; route-connection and failure classification.
- **Related:** B-22.8/9 also produce hung recovery but by deadlines, not classification. Not in Backlog.

### B-22.15 Native FIFO delays critical requests
- **Claim:** Slow browse bursts occupy control slots ahead of renewal/timeline work.
- **What I traced:** Apple native `Native/PorticoPlayer.m:114,129–150` starts twelve and queues sixty-four in order; NSURLSession’s per-task fifteen-second bound is configured at line 122 and task begins at line 131. Android `PorticoTransport.kt:46–56,88–107` uses one Dispatcher for all requests and refuses after 76 entries. Native `src/bridge/server-connections.ts:56` sends route/API requests to Runtime.networkRequest. Core refresh has its own fifteen-second outer abort (`server-connections.ts:243`); timeline has a twenty-second outer deadline in `playback-v1/legacy-bridge.ts:202`.
- **Verdict:** **Confirmed, narrower.** This is bounded head-of-line delay/refusal, not mathematical indefinite starvation: queued earlier calls eventually expire/finish. A critical outer deadline can expire before admission. No actual user burst/load threshold has been measured.
- **Class:** Design risk.
- **Severity:** **Medium** — native users with concurrent slow reads may miss authentication/timeline deadlines.
- **Fix direction:** Reserve small critical-operation capacity or prioritize it, include queue residence in operation budgets, preserve total bounds. **Size: M**; Apple transport, Android Dispatcher/admission, Vega transport and request policy.
- **Related:** B-19.9 is a different serial native bridge/storage queue. Not in Backlog.

### B-22.17 Remembered direct address remains obsolete
- **Claim:** A working physical failover is not reflected in a direct-server chooser entry.
- **What I traced:** Core `route-connection.ts:39,102` retains logicalOrigin while writing physical lastGood. KnownDirectServers `known-servers.ts:84,91` remembers the supplied address and updates only profile fields later. Native `src/app/session.tsx:56,385` supplies record.serverUrl. Actual chooser `src/screens/SettingsPages.tsx:100–102` displays that address and sets DirectSignIn origin from it; it does not restore a per-server credential/pin. `server-connections.ts` persists one selected record.
- **Verdict:** **Confirmed, narrower.** Active sessions can recover using lastGood, and Hosted servers have a separate chooser. A remembered **direct** server switched away from can prefill an obsolete address; this is a source-proven chooser path, not merely a label speculation.
- **Class:** Defect.
- **Severity:** **Medium** — direct-only multi-server users after endpoint changes need to correct an address manually.
- **Fix direction:** Store latest verified connect hint separately from stable logical ownership and retain its pin; use it for chooser pairing. **Size: M**; known-servers, session recorder, protected connection store and chooser.
- **Related:** B-22.5 can turn obsolete-address recovery into credential loss. Not in Backlog.

### B-22.18 Native auxiliary streams bypass selected route
- **Claim:** Watch Together/receiver streams use raw logical URLs instead of verified physical routing.
- **What I traced:** Native `src/app/together.tsx:21` opens api.baseUrl+path with bearer through `src/bridge/event-stream.ts:26–40` raw XHR. Receiver `src/app/receiver.tsx:57` does likewise. The stream adapter neither calls RouteConnection nor refuses redirects before follow. Core GroupSessionService consumes the injected stream. Web `web/src/app/together.tsx:22` explicitly uses api.routeFetch. Native `src/player/PlayerHost.tsx:62,77` mounts these providers from Shell, not orphaned implementations.
- **Verdict:** **Confirmed.** Token refs rotate but do not fix destination choice. Failover can leave feeds pointed at the old logical origin; raw stream opening bypasses identity/redirect enforcement of the routed API.
- **Class:** Defect.
- **Severity:** **High** — native Watch Together/receiver users during route changes; old/reassigned HTTP routes also carry a bearer.
- **Fix direction:** Add a route-owned streaming transport that resolves the selected origin, rejects redirects before credentials follow, and reconnects on route generation. **Size: M**; event-stream, together/receiver, RouteConnection and native stream transport.
- **Related:** B-22.6 shares HTTP exposure but this additionally bypasses existing selected-route safeguards. Not in Backlog.

### B-22.19 Vega redirect enforcement occurs too late
- **Claim:** Rejecting the final redirected response cannot stop sensitive request bytes reaching the redirect target.
- **What I traced:** Vega `src/bridge/transport.ts:8` explicitly documents SDK auto-follow; line 71 opens XHR and line 93 sends the body before line 88 checks responseURL. `src/bridge/runtime.ts:65,75` wires network/profile methods to this transport. There is no pre-follow redirect callback or redirect control here. Apple/Android transport refuses redirects earlier.
- **Verdict:** **Confirmed, narrower.** The source violates the no-follow enforcement model. Exact SDK 307/308 body and Authorization forwarding is still unverified, so **actual disclosure is not confirmed**. Cheapest check: on a Vega device against isolated non-Portico fixture endpoints, post a dummy secret through a 307/308 and capture the target body/header before the promise rejects.
- **Class:** Defect.
- **Severity:** **High** — credential-bearing Vega exchanges under a redirecting endpoint; impact is conditional on SDK forwarding.
- **Fix direction:** Use a transport with pre-follow refusal, including same-origin redirects; do not rely on responseURL. **Size: M** if an SDK seam exists, **L** for a native bridge; Vega transport/runtime.
- **Related:** B-22.18 raw native stream redirect behavior is adjacent. Not in Backlog.

### B-22.20 Android implicit mutation replay
- **Claim:** OkHttp can replay buffered mutations below core’s no-replay policy.
- **What I traced:** Android `PorticoTransport.kt:46–56` disables redirects but does not override retryOnConnectionFailure; lines 78–83 create buffered ByteArray bodies. Gradle pins OkHttp 4.12.0. Its [Builder source](https://raw.githubusercontent.com/square/okhttp/parent-4.12.0/okhttp/src/main/kotlin/okhttp3/OkHttpClient.kt) defaults connection retries on. Its [recovery interceptor](https://raw.githubusercontent.com/square/okhttp/parent-4.12.0/okhttp/src/main/kotlin/okhttp3/internal/http/RetryAndFollowUpInterceptor.kt) lines 81–89,134–158 retries eligible IOException failures after sending when body is replayable and another route is available; method is not restricted to reads. Core `route-connection.ts:135` promises no automatic mutation replay.
- **Verdict:** **Confirmed, narrower.** Wire-policy mismatch is certain. Duplicate committed side effects need a recoverable reset plus retry route and a mutation without effective idempotency; neither every reset nor every POST duplicates. Refresh’s stored request ID protects exact replay. No duplicate product operation was reproduced.
- **Class:** Defect (transport policy), with conditional side-effect risk.
- **Severity:** **Medium** (High → Medium) — Android/Fire users on qualifying transport failures; blanket high duplicate-impact claim exceeds established evidence.
- **Fix direction:** Make retry eligibility explicit/one-shot for unsafe writes, disable implicit failure replay where necessary; separately preserve safe connect-before-send alternatives. **Size: M**; PorticoTransport and operation/idempotency policy.
- **Related:** B-22.19 is a different underlying-stack policy gap. Not in Backlog.

### B-22.24 Same-link mDNS self-conflict
- **Claim:** Two server interfaces on the same link can mistake each other’s address RRset for a name conflict.
- **What I traced:** Server `apps/portico-server/server/cmd/server/main.go:1034` runs this discovery implementation on the listener; `internal/networking/discovery.go:196–206` calls a differing same-name A/AAAA record a conflict. Snapshot lines 240–259 use identity-derived host names with only supplied addresses; sockets lines 288–319 supply per-interface addresses with one shared serial suffix. Lines 440–466 withdraw/increment serial on conflict. Packet admission checks private/on-link peer and TTL evidence, not whether it is another local interface.
- **Verdict:** **Unsettled — needs runtime.** Conflict logic is coherent with the claimed trigger, but actual cross-interface delivery depends on multicast socket/interface/OS behavior. Cheapest check: isolated host with two distinct private addresses/interfaces on the same VLAN, default wildcard bind, run discovery and capture advertisements; repeated self-generated suffix changes/withdrawals confirms it, stable announcements without cross-delivery limits that topology’s exposure. Do not run it against production.
- **Class:** Design risk.
- **Severity:** **Medium** — uncommon same-link multi-interface NAS/bridged configurations if the receive path delivers the peer interface’s packets.
- **Fix direction:** Treat local interface RRsets coherently or filter verified local-source self-announcements; validate with mature DNS-SD behavior. **Size: M**; discovery snapshot/socket/conflict handling.
- **Related:** B-22.2 is independent signed HTTPS alias collapse, not mDNS. Not in Backlog.


## Tier 2 — quick checks

Quick-check paths below use these prefixes: **N** = `apps/portico-react-native/apps/native/`; **NC** = `apps/portico-react-native/packages/client-core/`; **C** = `apps/portico-server/packages/client-core/`; **W** = `apps/portico-server/web/`; **S** = `apps/portico-server/server/`; **V** = `apps/portico-react-native/apps/vega/`. All are relative to `/home/justin/projects/Portico`. Sizes include implementation, not physical-device acceptance time.

| ID | Independent check / verdict | Class; size |
|---|---|---|
| B-19.1 | **Confirmed, narrower.** `N/src/shell/Shell.tsx:259,294` drops the initial link when the real API is absent; `N/src/app/session.tsx:311,458` shows that ordinary remembered restore creates this window. Apple’s delayed resend (`N/ios/Portico/PorticoIntents.swift:15`) can rescue a quick restore, so not every cold launch loses its link. Medium. | Defect; M, scoped pending-link owner in Shell/session. Related B-21.14. |
| B-19.3 | **Unsettled — needs runtime.** Synthetic Select-down/up is certain (`N/android/app/src/main/java/tv/getportico/portico/MainActivity.kt:61–70`); exact no-long-press callback behavior in the pinned RN release was not independently established. Promoted check below. Medium conditional. | Defect if activation occurs; M, key dispatch/Focusable. |
| B-19.6 | **Confirmed, narrower.** No app global exception recorder in `N/index.js`/`N/App.tsx`; render boundary only logs (`N/src/shell/ErrorBoundary.tsx:15`). “No record anywhere” and universal fatal behavior exceed this evidence. Promoted below. Low. | Missing feature mislabelled as a bug; M, native bootstrap/local diagnostics. |
| B-19.10 | **Confirmed, narrower.** `N/src/ui/theme.ts:11,115–116` picks touch metrics solely from Platform.isTV; `N/src/ui/FocusHalo.tsx:20` draws no halo off TV. Conditional unsupported/misreported TV hardware, not proven on a shipping supported TV. Medium conditional. | Design risk; M, device classification/theme plus device acceptance. |
| B-19.12 | **Confirmed, narrower.** Selection/connection uses Keychain (`N/Native/PorticoPlayer.m:301,308`, queue records `:221`), with no first-install purge marker found. Retained valid Keychain items can restore after reinstall; deletion cannot contact the server to revoke a family. Actual reinstall/horizon behavior untested. Low. | Design risk; M, first-install marker and scoped credential cleanup; do not describe uninstall as server logout. |
| B-19.14 | **Confirmed, narrower.** Roots deliberately retain visited screens (`N/src/shell/Roots.tsx:6–10,23–25`); stack options (`N/src/shell/Shell.tsx:110`) add no freeze/depth policy. Pop/sign-out do release pages, so “never” is too broad. Memory failure threshold is unmeasured. Medium on weak devices. | Design risk; M, retained-root budgets, cyclic route coalescing/freeze. Related B-19.2 and B-21.5; retention itself is deliberate. |
| B-21.1 | **Confirmed.** Destination sheet only renders AirPlay/Cast/nearby rows (`N/src/player/Destinations.tsx:90–122`); `NC/src/presentation/settings-structure.ts:277` exposes the separate Link TV setting. Contracted always-available manual-code action is missing at this surface. Medium. | Unfinished feature; S, Destinations using existing LinkTVSheet. |
| B-21.2 | **Confirmed.** Programme Cancel calls `dvr.cancel` directly (`N/src/screens/channels/ProgramSheet.tsx:69,168`); Recordings confirms (`N/src/screens/Recordings.tsx:235`). Medium. | Defect; S, programme confirmation with same cancellation consequences. |
| B-21.3 | **Confirmed.** Tablet width ≥700 selects rail (`N/src/shell/Shell.tsx:129,324`), whose primary array omits Downloads (`N/src/shell/Rail.tsx:18–24,117`). Settings storage still links it (`N/src/screens/settings/SettingsPage.tsx:285`). Medium. | Unfinished feature; S, capability-gated non-TV rail destination. |
| B-21.4 | **Confirmed.** Home searches absent watchlist rows (`N/src/screens/Home.tsx:47–48`); current server layout excludes them intentionally (`S/internal/catalog/home_rows.go:1070–1073`). Native index replaces both domains (`N/Native/PorticoActivity.m:79,98`). Medium. | Defect; M, independent bounded Saved index feeder, preserving no-Home-shelf decision. |
| B-21.6 | **Confirmed, narrower.** Hidden remember-history and badges controls are web-only (`NC/src/presentation/settings-structure.ts:250,268`) but consumed by native Search `:64`/Rail `:39`. Appearance fields have no native consumers in the inspected source; card sizing is already known (`Backlog.md:63`). Medium. | Unfinished feature; M, settings applicability/privacy controls and appearance consumers. |
| B-21.7 | **Confirmed.** `N/src/screens/settings/SettingsPage.tsx:135` says This device, while `N/src/app/server-preferences.tsx:87,96` writes declared scope plus deviceClass; Shell supplies mobile/television at `:306`. Medium. | Documentation error (UI scope wording); S, clarify class-scoped rows without changing storage policy. |
| B-21.8 | **Confirmed.** Row tap slices at occurrence (`N/src/screens/title/SavedPages.tsx:152,167`), but `N/src/app/container-play.ts:58` supplies only itemId to the whole playlist. `N/src/player/usePlayer.ts:444,452` sends this selector; `S/internal/playbackv1/queue_build.go:269` chooses first match. Positional anchoring exists at `:332` but is not sent. Medium. | Defect; M, occurrence-aware selector/absolute authorized position and revision. |
| B-21.9 | **Confirmed.** `N/src/screens/title/PlaybackChoices.tsx:42,46,58,67` filters unknown language, collapses by language, and sends language intent. In-player track selection does not repair the pre-play omission. Medium. | Defect; M, track identity plus descriptive subtitle choices. |
| B-21.10 | **Confirmed, narrower.** Settings omits speed/effects capability fields (`N/src/screens/settings/SettingsPage.tsx:46`), enabling default gates (`NC/src/presentation/settings-structure.ts:185,233–235`). Vega reports only rate 1 (`V/src/bridge/playback.ts:269`) and no native listening owner (`V/src/overrides/native/bridge/native-listening-kinds.ts:13`). Leveling efficacy is still unestablished. Medium. | Unfinished feature; M, target UX capability context/settings gates. Related B-20.17, speed preference consumption. |
| B-21.11 | **Confirmed.** View supplies undefined container (`N/src/screens/title/SavedPages.tsx:209,221`); local loaded-entry fallback/shuffle is explicit (`:30–36`). Named playlist/collection selectors are different and work over server scope. Medium. | Unfinished feature; M, server query/view queue selector; do not load all results client-side. |
| B-22.7 | **Confirmed, narrower.** Default port/public IPv4/local-host acceptance mismatch confirmed; the first pass’s IPv6 HTTPS subclaim is false. Promoted below, including copied-function experiment. Medium. | Defect; S, shared canonical address candidate helper. |
| B-22.8 | **Confirmed.** `C/src/index.ts:338,376,387` forwards optional caller signal without default general deadline; routed fetch bounds probes, not the subsequent ready-route request. Some callers and native transport have their own limits. Medium. | Design risk; M, operation-class browser deadlines excluding deliberate streams/long polls. Duplicate B-16.8 agrees. |
| B-22.9 | **Confirmed, narrower.** Core watcher lacks its own deadline and web callers omit signals (`C/src/portico-servers.ts:181,188,200`; `W/src/app/session.tsx:403,409,415`). Actual native `N/src/bridge/useHostedAccount.ts:34` explicitly supplies bounded nativeNetworkFetch, contrary to first-pass native scope. Promoted below. Medium, web path. | Defect; S, watcher-owned bounded request/flight release. Related B-22.8. |
| B-22.10 | **Confirmed.** `C/src/server-connections.ts:262` wraps away retry floor; `C/src/session-refresh.ts:84,90,109,162–168` has a timer but no not-before check for reactive runs or jitter. Server auth limiter asks 60 s (`S/internal/httpapi/router.go:1300`). Medium. | Defect; M, preserve floor and shared proactive/reactive not-before. Backlog line 56 is related symptom only. |
| B-22.11 | **Confirmed.** `C/src/playback-v1/http.ts:44` accepts digits only; watcher `C/src/portico-servers.ts:191` uses Number(header), unlike shared date-aware parser `C/src/index.ts:134`. Header whitespace may be normalized by a transport; HTTP-date loss remains real. Medium. | Defect; S, reuse shared Retry-After parser at both seams. |
| B-22.12 | **Confirmed, narrower.** `W/src/bridge/account.ts:105` drops retry floor while `C/src/hosted-gate.ts:41` expects it. This BrowserAccountApi **does** have a 15 s deadline (`account.ts:99`); the separate raw page adapters `account-api.ts:16`/`account-sessions-api.ts:8` lack an intrinsic one. Medium. | Defect; S for header preservation; M if unifying auxiliary Hosted gate adapters. |
| B-22.13 | **Confirmed, narrower.** `C/src/sse.ts:16,26,30,35` bounds residual buffer but accumulates data[] across short lines until blank separator. Event/id are overwritten and individually bounded by line/buffer processing, not another accumulating array. Promoted below. Medium. | Defect; S, count current-frame aggregate and parse complete frames before residual cap. |
| B-22.16 | **Confirmed.** `C/src/route-identity.ts:72,105` accepts issue-time future skew but no expiry skew; server proof lasts 30 s (`S/internal/networking/route_identity.go:121`). Ahead-clock rejection is arithmetic, actual clocks untested. Medium. | Design risk; M, explicit bounded clock policy/error wording. |
| B-22.21 | **Confirmed, narrower.** Apple/Android discovery constructs HTTP literals (`N/Native/PorticoNetwork.swift:84`, Android `native/PorticoNetwork.kt:304`). Claimed, TLS-ready **required-policy** servers reject identity-proof HTTP with httpsUrl (`S/internal/httpapi/secure_connections.go:147,178–190`), discarded by `C/src/route-identity.ts:88`. Unclaimed/default-permissive/loopback cases are not implicated. Medium. | Unfinished feature; M, secure discovery hints/upgrading with pin proof. Related B-22.1. |
| B-22.22 | **Confirmed.** `C/src/route-connection.ts:87,100,117,126` retains ready winner and cancels losers; no LAN promotion work remains without client path change/failure. This is a preference policy, not proof of failed playback. Medium conditional network cost. | Design risk, current ready-route policy; M if adding event-driven/bounded promotion; preserve no periodic Hosted chatter. |
| B-22.23 | **Confirmed.** Server writes Report-Every-Ms (`S/internal/httpapi/playback_v1_routes.go:285`) but CORS list omits it (`S/internal/httpapi/router.go:1264`); client only adopts readable header (`C/src/playback-v1/sessions.ts:107`). Same-origin and native unaffected. Medium. | Defect; S, expose the intended control header. |
| B-22.25 | **Confirmed.** Events sleep max(local jitter, floor) (`C/src/playback-v1/events.ts:153`); directed close delay goes straight to sleep (`:143,176`). Common large floor removes spread. Medium fleet recovery risk. | Design risk; S, positive jitter beyond the server floor, retaining exact-deadline semantics separately. |
| B-22.26 | **Confirmed.** Native activation’s actual setup screen uses nativeTVActivation; `N/src/bridge/setup-codes.ts:21,29–31` calls ungated HttpLocalApi. `NC/src/setup-code.ts:143,179–185` backs off creation but retries transport-failed polls without exponential jitter. Protocol pending/slow-down is handled. Medium. | Defect; M, Hosted gate/outage retry seam while preserving valid codes and manual entry. |

### Promoted Tier 2 check — B-19.3 Menu-as-Select
- **Claim:** Menu activates controls lacking a long-press action.
- **What I traced:** The production MainActivity synthesizes a center-key press held 800 ms (`N/android/app/src/main/java/tv/getportico/portico/MainActivity.kt:61–89`); Focusable only passes a long-press handler when provided (`N/src/ui/Focusable.tsx:107–108`). [RN TV’s official interaction documentation](https://github.com/react-native-tvos/react-native-tvos/blob/main/README.md) establishes Select and held-Select as press/long-press input, but the exact pinned 0.83.10-1 Pressability source was unavailable through the public fetch attempts; node_modules was excluded by the brief.
- **Verdict:** **Unsettled — needs runtime** for universal activation without onLongPress; the unsafe key remapping is source-confirmed. Cheapest check: focus a non-destructive test button with only onPress in the actual release Android/Fire build and press Menu once. Callback execution confirms the claim; no callback refutes that specific release behavior. No destructive control needs to be tested.
- **Class/severity:** Defect if callback occurs; **Medium**, unexpected activation on those remotes. Scope excludes non-TV input and busy/disabled controls.
- **Fix direction:** Deliver context-menu intent separately and route only to an existing menu action. **Size: M**; MainActivity, Focusable/remote event dispatch. **Related:** B-21.16; not in Backlog.

### Promoted Tier 2 check — B-19.6 Global crash diagnostics
- **Claim:** Every non-render JS exception terminates the app without a record anywhere.
- **What I traced:** `N/index.js` installs event/random/URL/decoder bootstrap and registers App; `N/App.tsx` mounts ErrorBoundary; `N/src/shell/ErrorBoundary.tsx:15`/ScreenBoundary log render failures. No app global exception journal/handler was found. No injected failure or OS log read was performed.
- **Verdict:** **Confirmed, narrower.** Absence of an app-owned durable recorder is established. A framework/OS crash log can still exist; release fatal/unhandled-rejection policy depends on runtime and exception type. An exception handler is not itself required for valid ordinary operations. Cheapest runtime check if crash semantics matter: throw a dummy timer exception in an isolated release fixture and inspect device logs plus next-launch diagnostics.
- **Class/severity:** **Missing feature mislabelled as a bug; Low** (Medium → Low) for diagnostic quality, not an independently demonstrated crash trigger.
- **Fix direction:** A redacted bounded local error journal with previous-crash reporting; preserve framework fatal semantics rather than swallowing errors. **Size: M**; native bootstrap/diagnostic storage. **Related:** B-25.10 acceptance coverage; not specifically in Backlog.

### Promoted Tier 2 check — B-22.7 Manual address guessing
- **Claim:** Bare-address heuristics omit Portico’s local port and disagree with strict route parsing.
- **What I traced:** `W/src/app/session.tsx:166–172` and `N/src/app/direct-connect.ts:21–27` are identical helpers; native calls it before inspection (`direct-connect.ts:44`). `C/src/route-identity.ts:42–54` accepts HTTP only for canonical private/local hosts. No later default-port insertion appears before the proof URL.
- **Verdict:** **Confirmed, narrower.** Bare private IPv4 requests port 80 instead of 32500; public IPv4 and unsupported .lan/.home/single-label names become HTTP and are rejected by routeOrigin. **Refuted subclaim:** bracketed private IPv6 is guessed HTTP, not HTTPS: truncating at the first colon leaves a single-label fragment that matches `[^.]+`. Public bracketed IPv6 is also guessed HTTP and rejected.
- **Experiment:** Copied exactly this helper from native direct-connect.ts to `/tmp/v10-normalize.js`, erased only TypeScript annotations/export, and ran `node /tmp/v10-normalize.js`. Inputs private IPv4, public IPv4, `server`, `box.lan`, `[fd00::1]`, `[2001:db8::1]` all produced HTTP URLs with no inserted port. This tests the copied pure helper, not sign-in or running Portico. No dependencies or network listener.
- **Class/severity:** **Defect; Medium** for manual-address onboarding; explicit canonical URLs remain usable.
- **Fix direction:** One private-host-aware helper with documented local default port and preserved explicit choices. **Size: S**; shared normalization, both sign-in adapters. **Related:** B-22.21 is separate required-HTTPS bootstrap; not in Backlog.

### Promoted Tier 2 check — B-22.9 Watcher deadline/platform scope
- **Claim:** One hung server-list fetch wedges future foreground/manual checks, including an asserted default native fetch path.
- **What I traced:** `C/src/portico-servers.ts:181` shares one flight, line 188 sends optional signal, and line 200 clears only on settlement. Web `W/src/app/session.tsx:403,409,415` supplies no signal. Native `N/src/bridge/useHostedAccount.ts:29–34` actually supplies `nativeNetworkFetch`; `N/src/bridge/useAccountDirectory.ts:50` forwards it into the watcher. That transport calls the bounded Runtime at `N/src/bridge/server-connections.ts:56`.
- **Verdict:** **Confirmed, narrower.** Browser flight ownership is unbounded as claimed. **Refuted subclaim:** native does not use default raw core fetch on the production account-directory path; its explicit adapter gives the native task timeout, although FIFO waiting adds latency (B-22.15). Neither shared HostedGate nor an interactive refresh supersedes the browser's unresolved flight.
- **Class/severity:** **Defect; Medium** for browser directory checks on a stalled transport.
- **Fix direction:** Bound the watcher operation independently of caller/platform and always release shared ownership on deadline. **Size: S**; portico-servers watcher, leaving native bounds intact. **Related:** B-22.8 / B-16.8 agree for browser deadlines. Not in Backlog.

### Promoted Tier 2 check — B-22.13 SSE bounds
- **Claim:** A supposedly bounded SSE reader can accumulate an unbounded frame.
- **What I traced:** `C/src/sse.ts:25–35` caps buffer before extracting lines, then removes each line into data[]; dispatch at line 19 resets only at a blank separator. Event/id replace their strings rather than append.
- **Verdict:** **Confirmed, narrower.** Unbounded repeated data lines and rejecting a large chunk of otherwise valid small frames are real. Event/id have no separate explicit limit, as noted in the first pass, but they cannot accumulate without bound: each parsed line has already passed the buffer cap and replaces the prior value. A single huge arriving chunk can temporarily allocate before rejection, a general transport boundary rather than a separate event/id leak.
- **Class/severity:** **Defect; Medium** for browser/core streaming readers receiving malformed or badly framed data. Native raw XHR additionally limits held text, narrowing that path.
- **Fix direction:** Track aggregate current-frame size and process complete frames before checking residual limits. **Size: S**; sse reader and boundary fixtures. **Related:** B-22.18 concerns route ownership, not frame accounting; not in Backlog.


## Not re-checked (Low)

- **B-19.5** — An image that has loaded once is never retried and gets no placeholder if it later fails (not re-checked).
- **B-19.8** — The Android TV density override has gaps and wide side effects (not re-checked).
- **B-19.9** — On Apple, storage, PIN hashing and request submission share one queue (not re-checked).
- **B-19.11** — Vega keeps sign-in records as plain files (not re-checked).
- **B-19.15** — The test harness leans on a React internal, and native tests are not run (not re-checked).
- **B-19.16** — User-visible English outside the catalogue (not re-checked).
- **B-19.19** — Small contract leftovers (not re-checked).
- **B-21.15** — Back from filters commits live changes rather than restoring a draft (not re-checked).

## New findings

None. Corrections to existing claims are recorded under their original IDs.

## Disagreements worth Justin's attention

- **B-19.6: Medium → Low.** Missing app-owned diagnostics is a missing capability, not evidence every non-render exception causes an unrecorded fatal crash.
- **B-19.12: Low–Medium → Low.** Keychain persistence permits retained sign-in, but uninstall is not an actionable server revocation event and actual reinstalls were not exercised.
- **B-19.13: Low today/Medium at scale → Medium for variable rows.** Equal-row assumptions can already conflict with wrapped captions/enlarged text; enormous-offset precision remains unmeasured.
- **B-19.17: Low now/Medium later → Low for current release.** No non-English launch language or affected shipped runtime was established.
- **B-21.5: High → Medium.** The O(visible) violation is real; automatic whole-library loading and a measured failure threshold are not.
- **B-22.5 and B-22.6: High → Medium.** Separate unsigned-reset cleanup from credential disclosure, and separate the deliberate HTTP transport boundary from a broken enforcement control. No source-only report should imply later HTTP sockets are cryptographically authenticated.
- **B-22.20: High → Medium.** Underlying automatic replay is source-confirmed; committed duplicate effects depend on failure/retry eligibility and operation idempotency. Review unsafe-write policy, not every POST as equally exposed.
- **B-22.7:** IPv6 is guessed HTTP, contrary to the first pass. The broader default-port/parser mismatch remains confirmed.
- **B-22.9:** native explicitly supplies its bounded transport; keep the unbounded-flight claim on the browser path.
- **B-22.13:** data[] is unbounded across lines; event/id overwrite indirectly bounded strings. The lack of a separate explicit event/id cap is not a second cumulative leak.
- **B-19.3:** downgrade confidence to runtime-required for callback behavior of the pinned native dependency; the key remapping itself is established.
- **B-19.7 / B-19.18:** keep release checks explicit. A source plist and an SDK announcement are not successful/failed archive validation or device launch.
- **B-21.13 / B-21.12:** upgrade confidence from Probable to Confirmed for the source defect: ordinary token renewal supplies the first trigger, and live Hosted owner server authorization disproves any necessity for the second client restriction.

No product decision or rollout approval was requested; these are review conclusions for Justin’s planning.

## Evidence boundary

This was a read-only static second review of the current checkouts on **ehler-homelab-mini-1**. The required context-sync refresh was attempted first after the verification briefs but failed because its lock path was read-only; existing canonical project/verification guidance was read. Reviewed applicable architecture, Backlog, decisions and identity/networking/navigation/settings/metadata/accessibility requirements; no undocumented preference was treated as authorization to change the product. Backlog matches above distinguish exact work from historical symptoms or stale entries.

No Portico build, test suite, install, formatter, server, endpoint, state store, browser, computer-use or MCP tool was used. No subagents were spawned. Public Apple/RN/OkHttp documentation/source was consulted only for platform/library behavior; no Portico source or private detail was sent externally. Builds/merged plists, runtime framework callbacks, App Store validator outcomes, multicast delivery, physical-device focus, SDK redirect forwarding, native Intl and actual load/memory thresholds remain unestablished. The intended RN Pressability release source could not be fetched independently; this is why B-19.3 is runtime-required. Existing tests were read as potential counterevidence, never executed; sequential single-runtime refresh tests do not establish simultaneous cross-tab behavior.

One allowed scratch experiment copied only the pure native serverAddress helper (TypeScript syntax removed) into `/tmp/v10-normalize.js` and ran it with Node and six literal inputs. It performed no network I/O, dependency downloads, persistent cache setup or Portico execution. Results are bounded to that copied helper. No other experiment was run.

Only this designated Markdown report was written under Portico. Temporary review notes/scripts were written under `/tmp`. **Coverage: all requested entries reached; none omitted.** Conditional severities are planning estimates, not reproduced release blockers.
