# Navigation and Input Contract

> Current Portico product contract. October decisions supersede the September reconciliation where noted.


Contract ID: `PC-NAVIGATION-INPUT`  
Revision: `portico.october.1`  
Status: **approved**  
Owners: Product, Client Foundations, Client Core, Accessibility  
Imports: `PC-FOUNDATION` viewer scope; `PC-GOVERNANCE` lifecycle/exception law; `PC-GLOSSARY` vocabulary  
Coordinates with: `PC-VISUAL`, `PC-MEDIA-PRESENTATION`, `PC-IDENTITY`, `PC-NETWORKING`, `PC-PLAYBACK`, `PC-ACCESSIBILITY-LANGUAGE`  
Applies to: Web, bundled Server Web, Client Core, iOS, Android, tvOS, Android TV, Fire TV, Roku, Cast controller surfaces, and future first-party clients

## NAV-1. Purpose and scope

Portico navigation must feel like one product even though browsers, native stacks, television focus engines, and constrained runtimes use different mechanics. This contract owns:

- durable destinations, route identity, hierarchy, history, and current location;
- primary navigation, pivots, tabs, transient surfaces, and full-page workflows;
- Back, dismiss, cancel, activation, and application-exit outcomes;
- pointer, touch, keyboard, remote, and game-controller equivalence;
- television focus identity, neighbors, containment, restoration, and virtualization;
- deep links, pending route intent, bounded restoration, and invalid destinations;
- navigation continuity during data refresh, route recovery, viewer changes, and playback;
- transition meaning and permitted platform adaptation.

It does not choose framework packages, component trees, pixel styling, server routes, media playback delivery, or assistive-technology copy. Those belong to Project Architecture, `PC-VISUAL`, `PC-PLAYBACK`, and `PC-ACCESSIBILITY-LANGUAGE` respectively.

## NAV-2. Non-goals

- One router library across every platform is not a goal.
- Pixel-identical navigation chrome is not a goal.
- Persisting an unrestricted router tree is prohibited and is not a continuity goal.
- A navigation route is not an authentication, authorization, data-cache, or playback authority.
- Hover, focus, current destination, and selected product state are not collapsed into one state.
- Compatibility with pre-release route aliases or parallel navigation stores is not required.

There are no unresolved product decisions in this revision. New destination kinds, altered root behavior, or automatic focus-driven selection require a contract change.

## NAV-3. Canonical terminology

These terms are deliberately different:

| Term | Meaning | Cardinality |
| --- | --- | --- |
| `current` | The durable destination rendered by the one navigation authority. | Exactly one inside an active product scope. |
| `focused` | The ephemeral target that would receive keyboard/remote/gamepad activation now. | Exactly one in the active directional focus scope; none when the platform intentionally suppresses focus. |
| `selected` | A durable product choice such as active profile, server, library pivot, quality, subtitle, or primary destination. | Zero or more by domain. |
| `pressed` | A physical or logical activation is down but has not necessarily committed. | Ephemeral. |
| `invoker` | The stable semantic target that opened a transient surface. | One per presented surface where restoration is possible. |
| `primary destination` | A root-level product location selected without accumulating root history. | One current root. |
| `secondary destination` | A history-bearing page reached from a root or another secondary destination. | Bounded stack. |
| `transient surface` | A popover, menu, tooltip, temporary filter/sort surface, confirmation, or player utility that does not represent a durable location. | Bounded layered stack. |
| `durable modal` | A screen-like exclusive workflow with its own lifecycle, such as credential, PIN, or destructive confirmation. | Bounded history/presentation. |
| `viewer scope` | `{authority, accountId, profileId, serverId?, authorizationRevision}`; `serverId` is absent while a Portico Account has no selected server. | One verified scope. |

Moving focus MUST NOT change selection unless the control is explicitly a preview-only control whose selected value remains unchanged until activation. Focus MUST NOT open a new page, start playback, commit a choice, or mutate data.

## NAV-4. Authority and state model

1. Every active viewer scope MUST have exactly one mutable navigation authority. A second router, shadow stack, component-local history, or independently mutable `current` value is prohibited.
2. Client Core owns the framework-independent destination grammar, semantic identity, capability requirements, serialization rules, restoration fences, and conformance outcomes.
3. Each client router owns the platform-native representation of current destination and bounded history.
4. Screens MAY own transient presentation and scroll/focus state. They MUST NOT invent durable route history.
5. Playback owns the media session. Navigation owns only player presentation intent and the route by which the user entered or left it.
6. Server and Hosted Services MAY publish resource identifiers and capabilities. They MUST NOT publish client layout, raw navigator actions, focus coordinates, or platform-specific routes.
7. Cached navigation is presentation data, never proof of identity, resource existence, capability, or authorization.

The navigation lifecycle is:

`unverified → account-flow | profile-security → product-scope → fail-closed`

Ordinary connecting, refreshing, server-unavailable, offline, stale, and route-recovery states remain inside `product-scope`. They MUST NOT replace the product navigator or sign the user out.

## NAV-5. Destination hierarchy

### NAV-5.1 Primary destinations

The canonical consumer roots are:

| ID | Availability | Notes |
| --- | --- | --- |
| `home` | all product clients | Default authorized root. |
| `library:{libraryId}` | each authorized named library on all product clients | Stable per-library destination with canonical pivot; not one generic combined Library page. |
| `channels` | only when Live TV/Library Channels capability exists | May carry a canonical replace-style tab. |
| `saved` | all product clients | May carry a canonical replace-style tab. |
| `downloads` | handheld/offline-capable clients only | Remains reachable while the server is offline. |

Primary navigation MUST show only applicable destinations. Selecting a primary destination replaces the active root and clears secondary history for the former root; it MUST NOT stack a trail through previously selected roots. Re-selecting the current root MAY perform one documented platform convention, such as returning its retained scroll position to the start, but MUST NOT create history or refetch data merely because it was selected again.

Web and Server Web MAY expose additional administrative primary areas in an admin rail. Those remain product destinations with stable IDs and the same history, accessibility, and Back rules; they are not consumer roots automatically copied to television.

### NAV-5.2 Secondary destinations

Canonical secondary classes include Search, Settings/Account Hub sections, media detail, person detail, notifications, Saved-resource detail, Watch With Friends, and player presentation. A secondary destination MUST have a stable semantic identity. Repeating a request for the already-current semantic identity focuses or replaces it; accidental duplicate copies are prohibited.

A genuine content hierarchy MAY stack distinct detail identities. A cyclic path, repeated equivalent route, or unbounded drill-down MUST coalesce or replace rather than grow forever.

### NAV-5.3 Route parameters

Routes MAY contain bounded, serializable identifiers and presentation intent: media, person, library, channel, recording, group, download, pivot, tab, settings-section, season/episode selection, and player context.

Routes MUST NOT contain credentials, tokens, PINs, signed grants, raw playback/continuation URLs, private file paths, complete API responses, callbacks, class/native instances, query clients, or unrestricted local URLs. Unknown fields and unknown enum values fail closed.

Semantic identity excludes replace-style state. For example, `library:{libraryId}` remains the same destination while its pivot changes; `media-detail:{mediaId}` remains the same destination while an episode within that detail is selected.

## NAV-6. Pivots, tabs, filters, and transient state

1. A pivot or tab is durable selection inside one destination, not a new page for every activation.
2. A linkable pivot/tab MAY replace the current route parameter. It MUST NOT add one history entry per tap or remote press.
3. Focus passing across a pivot does not activate it. Activation selects it, updates `aria-selected`/native equivalent, moves its controlled panel into the current state, and places focus predictably.
4. Changing a pivot/tab MUST preserve common page chrome and SHOULD preserve compatible content geometry.
5. An unavailable pivot is omitted when capability is known absent. A temporarily unavailable but normally applicable pivot MAY remain visible and disabled with a reason.
6. Filters, sort, grid/list mode, expanded rail, hover actions, open popovers, unsaved text, and player utility panels are presentation state unless a Product Contract explicitly makes them linkable.
7. Applying filters updates the current destination in place. Cancel or Back restores the last committed selection; it does not silently commit the focused draft.
8. Search typing never creates route history. A submitted query stays in the current scoped in-memory Search service and may survive an in-session detail visit. It is excluded from route URLs, browser-history payloads and persisted restoration. Private search text is not restored across launches without separate approval.

## NAV-7. Surface classes and dismissal

### NAV-7.1 Full-page surfaces

The following MAY own the full screen: signed-out/account creation, explicit profile/PIN security before product entry, the persistent signed-in product frame, full-screen player, and fail-closed security. Once the authenticated handheld product frame is mounted, its primary lower navigation remains mounted and geometrically stable across primary and secondary product destinations, ordinary loading/refresh/recovery, empty/error states, and in-product modal presentation. Full-screen Player is the only ordinary product destination that replaces/hides it. An in-product PIN/security modal may make the still-mounted frame inert but does not tear it down.

A workflow becomes a durable modal or secondary page when it has meaningful Back history, deep-link identity, multi-step validation, credential/security exclusivity, or content that cannot be understood as a short-lived tool. Sort menus, filter menus, tooltips, compact choices, context actions, and ordinary confirmations remain transient.

### NAV-7.2 Modal and overlay law

1. The top modal makes underlying content inert to pointer, keyboard, remote, gamepad, and assistive technology.
2. Opening records the semantic invoker and moves focus into the surface. Closing restores that invoker if it still exists and is eligible; otherwise it uses the documented nearest workflow fallback.
3. A modal contains focus. Directional or sequential movement MUST NOT escape.
4. A modal has a visible, reachable, semantically named Close or Cancel action, platform Back/Menu dismissal, and accessibility escape where supported unless completing the workflow is the only safe outcome. Dismissal is idempotent; background inertness MUST NOT create a dead end.
5. Outside-click/tap dismissal is permitted only for reversible, non-destructive transient surfaces. It is prohibited for required acknowledgement, destructive commit, or credential/PIN entry.
6. Route or viewer transition dismisses stale transient surfaces before the replacement destination is published.
7. Nested transient surfaces are bounded. Back/Escape dismisses exactly the top eligible surface per discrete activation.

## NAV-8. Canonical Back hierarchy

Back means **unwind the user's current presentation one level**, never “close the app” or “jump home” merely because a platform implementation lost its stack.

One discrete Back is resolved once in this order:

1. let a system-owned text editor, keyboard, dictation, or accessibility surface consume Back when it owns the event;
2. close the top transient overlay/player utility and restore its invoker; on television, an entered in-player discovery region is dismissed here, restoring the current media/player context and resetting the next chrome engagement to the transport/control region rather than the last discovery card;
3. close or cancel the top durable modal according to its dirty/destructive policy;
4. unwind player presentation according to `PC-PLAYBACK`—video returns to the invoking destination and ends only when that player-exit outcome requires it; audio may return to browsing while playback continues;
5. navigate one secondary-history entry;
6. apply the platform root rule below.

Root rules:

- **handheld:** delegate to the operating system after the product root; iOS interactive edge Back is enabled on ordinary secondary routes and disabled only for a documented conflict.
- **Web:** browser Back navigates Portico history before leaving the application. In-app links MUST not replace the browser's expected history with a parallel stack.
- **television with a collapsed leading rail:** first root Back reveals/focuses the rail on its currently selected destination; the next discrete root Back may pass to the operating system. A rail opened by D-pad closes on Back and restores its invoker. A rail opened by root Back MUST NOT close and reopen in a loop.
- **Roku/Android TV/tvOS:** held or system-reserved Back/Menu remains with the operating system. Root exit is never gated by a confirmation dialog.

Visible Back buttons are required only when the platform or workflow needs them. Television remote Back remains authoritative; tiny hyperlink-style Back controls are prohibited. When the only visible choices are destructive/commit actions, a normal-sized Cancel action MUST be present.

## NAV-9. Input abstraction and activation

### NAV-9.1 Semantic actions

Platforms map physical inputs into this minimum vocabulary:

`move`, `activate`, `back`, `dismiss`, `play-pause`, `seek-back`, `seek-forward`, `context-menu`, `info`, `search`, and `home/system-home`.

The operating system retains reserved Home, power, volume, accessibility, and voice-assistant gestures. Portico MUST NOT capture them as ordinary application navigation.

### NAV-9.2 Activation transaction

Activation has `requested → committing → committed | cancelled` states.

- One physical press, click, tap, accessibility activation, or gamepad activation commits at most once.
- Key repeat for Select/Enter/Space/OK coalesces until key release. Directional repeat MAY continue at a bounded platform-appropriate rate.
- A viewer epoch, route epoch, destroyed invoker, superseding intent, or abort signal cancels stale work and late completion.
- A disabled, hidden, loading-placeholder, or inert target cannot activate even if a stale handler fires.
- An ambiguous mutation outcome reconciles with its authority; navigation MUST NOT replay the mutation merely because the expected destination did not appear.

### NAV-9.3 Touch and pointer

- Every pointer action has keyboard semantics and every essential hover action is reachable without hover.
- Touch uses tap for activation. Complex gestures have visible, single-pointer alternatives.
- Swipe Back follows native convention and cannot conflict with scrubbing, drawing, or horizontal rails; a measured conflict may disable it only at the affected surface.
- Pointer down MUST NOT commit a destructive action before pointer up/cancel semantics can operate.
- Hover MAY reveal supplementary actions but cannot alter selection, load a different destination, or be the only discovery path.

### NAV-9.4 Keyboard

- `Tab`/`Shift+Tab` follow semantic DOM/native order; arrows operate widgets whose established pattern uses arrows.
- `Enter` and, for button-like controls, `Space` activate once. `Escape` follows the Back hierarchy only for the active Web/desktop surface and never bypasses an unsaved/destructive boundary.
- Visible shortcut hints use localized Product Language. Application shortcuts MUST avoid operating-system/browser reserved combinations and MUST be disabled while text entry owns them.
- The product MUST remain fully operable without a pointer.

### NAV-9.5 Remote and gamepad

- D-pad/arrow movement is discrete and predictable; Select/OK activates; Back/Menu unwinds; Play/Pause controls media when a media session owns it.
- Every visible television action is reachable. A control placed outside any clear directional path is a defect.
- Gamepads map the primary face button to activate and the platform-standard secondary/back button to Back. Additional buttons may accelerate tasks but cannot be required.
- Remote “Info/Options” opens context only when the current target advertises such an action. It never guesses a destructive command.

## NAV-10. Television focus model

1. Exactly one eligible target is focused within the active television focus scope after an interactive screen becomes ready. Loading reservations, hidden content, disabled actions, and the native video surface are never focus targets.
2. Focus targets and semantic sections use stable IDs. Coordinates, absolute array indices, localized labels, transient node addresses, or rendered positions are not identity. A queue or playlist duplicate requires an authoritative occurrence ID; indistinguishable catalog duplicates are deduplicated.
3. The platform focus engine owns ordinary directional movement within and between visible sections. An application MUST NOT process the same ordinary D-pad event through a JavaScript spatial coordinator and issue a competing focus request.
4. A semantic section observes its current target but does not redirect ordinary entry. A native focus guide, one-shot destination, or explicit native request is permitted only for a demonstrated geometric gap, modal trap, rail edge, route/modal return, or bounded virtualized restoration. Ordinary cross-row entry uses native nearest geometry rather than the row's remembered child.
5. Preferred-focus and forced-destination claims are one-shot. They are cleared after the resulting focus event and MUST NOT remain as permanent competing claims. Held directional input MUST NOT synthesize ordinary traversal; repeat handling is reserved for a documented nonspatial operation such as seeking.
6. Compatible refresh, reorder, and page append preserve the exact semantic target when it remains eligible. If the currently focused item is authoritatively removed while its collection is active, native focus geometry chooses the nearest eligible target; the acceptance outcome for an ordered `[A, B, C]` collection is successor C, then predecessor A. A list may reproduce that policy during explicit restoration only when it already owns authoritative prior/current stable occurrence order. When a route returns after the remembered item vanished and that ordering evidence is unavailable, restoration clears the stale target and uses the section entry rather than constructing a second client-side ordering model. A query, filter, source, or dataset-identity change invalidates result-section memory and returns to query/filter/section entry instead of selecting an unrelated item at an old index.
7. Exact off-window restoration is best-effort and owned by the list. It requires stable ID-to-index mapping, deterministic geometry, transition readiness, a bounded attempt count, and explicit mounted/missing/cancelled completion. Any newer native focus, blur, modal change, viewer/fence change, or dataset-identity change cancels the request. Loading reservations are never focused.
8. Returning from a secondary destination or modal restores its semantic invoker when it remains eligible. Returning to a retained root MAY restore its last current semantic address after the native transition is ready. No route simultaneously arms auto-focus, a destination, and an explicit request for the same restoration.
9. Child-focus memory is in-session only and fenced by authority/account, server, profile, authorization generation, and route identity. It is cleared before a profile, account, server, authorization, or incompatible dataset replacement is published and is never serialized as navigation restoration.
10. Background refresh and live events MUST NOT move focus. If the focused item is deleted, fallback occurs at most once and is announced according to `PC-ACCESSIBILITY-LANGUAGE`.
11. A modal that makes background content inert MUST expose a visible, reachable, correctly labelled Close or Cancel action, platform Back/Menu handling, and accessibility escape where supported. Dismissal is idempotent. Native presentation isolates native modals; in-tree overlays disable only their background sibling, never their own modal subtree.
12. Focus and selected visuals remain distinct. The visual treatment comes from `PC-VISUAL`.

## NAV-11. Deep links and external intents

The canonical grammar covers product roots, library/pivot, media/person detail, Search, Settings sections, notifications, Watch With Friends, supported player intents, invitations, and device authorization where the owning contracts allow them.

An untrusted link follows this pipeline:

1. parse a strict canonical scheme/allowlisted HTTPS origin; reject credentials, fragments containing secrets, unknown fields, oversized values, and unknown destinations;
2. store at most one bounded pending intent, normally expiring after ten minutes;
3. establish or restore the required account identity without treating the link as authorization;
4. resolve the exact server and profile;
5. verify platform capability, current permission, and resource existence against the owning authority;
6. dispatch through the one active navigation authority;
7. consume the intent exactly once.

A destination is server-relative unless its owning contract declares it account-only. A captured authenticated link binds to the current server. A share/invitation that intentionally targets another server carries a bounded opaque target server ID in the external intent envelope, not inside media identity. With no bound/current server, one eligible server may be selected automatically; multiple eligible servers require the server chooser; zero servers preserves the signed-in account and shows the no-server state. Portico MUST NOT probe every server for a matching opaque media ID or silently choose a same-looking item on another server.

A newer explicit link supersedes the prior pending intent. Account/profile/server mismatch, expiry, cancellation, unsupported capability, missing resource, or denied authorization discards it. A failed pending intent falls back to the legal product root with a classified message only when the user needs to act; it MUST NOT show a router error or retry forever.

Deep links simulate a legal manual route so Back has a predictable destination. Platform-owned direct-play entry points MAY require a direct system return; that exception is scoped to the platform entry contract, never applied to ordinary Portico links.

## NAV-12. Restoration and continuity

### NAV-12.1 Persisted restoration

Portico persists only safe, bounded hints:

- last authorized primary destination per verified viewer/platform scope;
- last selected library and canonical pivot when still authorized;
- bounded screen-owned scroll identity;
- Downloads when explicitly selected and valid local content exists.

It does not restore player as proof of an active session, an arbitrary detail stack, expired PIN/credential screens, private search text, transient surfaces, or unavailable/unauthorized destinations.

Every record is fenced by authority, account, server, profile, authorization revision, Product Contract revision, navigation revision, platform class, and route-affecting capability revision. Any mismatch fails silently to the legal root. Restoration never produces an error page.

### NAV-12.2 Refresh and connectivity

- A compatible data refresh preserves route, shell, scroll, focus, selection, and player.
- Route changes among verified localhost, loopback, LAN, and public-direct transports do not change navigation identity.
- Server unavailable retains the signed-in shell and safe current destination. Account and server switch remain reachable.
- A viewer transition synchronously fences old route dispatch, pending intents, focus/scroll memory, overlays, and late async completion before publishing the new scope.
- A server switch resets server-scoped history and resolves the new server's legal root. A profile switch cannot reveal another profile's route title, artwork, query, focus, or playback context.
- Permission revocation replaces only the no-longer-authorized destination with the nearest legal parent/root; a security-boundary failure may replace the entire product tree.

## NAV-13. Player navigation

1. The active player instance and media session MUST NOT be owned by a disposable route screen.
2. Route focus/blur, tab selection, transient-panel changes, and server-route recovery MUST NOT recreate or stop playback.
3. Full-screen video presentation is a destination/presentation state over one session. Mobile may minimize it and continue; television video exit follows `PC-PLAYBACK`; audio can return to browsing while remaining active.
4. Player Back order is panel → utility layer → player presentation. It is not page history until player-owned layers are exhausted.
5. The native video rendering node does not own remote focus. Visible product controls own it while chrome is shown; one route-scoped player-stage interaction target may own it while chrome is hidden so input can reveal controls without an application-wide invisible focus sink.
6. Player deep links are authorized as playback intents, not trusted playback URLs.
7. Fatal player states keep truthful actions focusable, including Retry only when retry can help and Back to the invoking detail/root.

## NAV-14. Transitions

Transitions communicate route meaning; they never conceal missing state.

| Semantic transition | Product meaning | Default adaptation |
| --- | --- | --- |
| `forward` | enter a secondary destination | native push/slide on handheld; restrained 180–220ms fade-through with at most 8–12px lift or 1–2% scale on TV; router-preserving transition on Web |
| `back` | return one history level | native pop/interactive gesture on handheld; reverse restrained TV fade; immediate or subtle Web transition |
| `replace` | change root, pivot, tab, or equivalent identity | fade-through/crossfade; no spatial history implication |
| `fullScreen` | enter/leave player or exclusive workflow | platform-native full-screen presentation or restrained fade |
| `none` | security fence, first paint, or continuity-sensitive correction | immediate atomic replacement |

Transitions MUST be interruptible, MUST NOT block input after the destination is ready, and MUST NOT cause focus/scroll resets. Reduced Motion uses immediate or brief opacity-only change. Remote focus movement and ordinary row/control interaction are not page transitions.

## NAV-15. Invalid, interrupted, and out-of-order navigation

- Unknown or malformed route: discard and open the legal root; log bounded diagnostics without private values.
- Known destination, resource missing: show the owning destination's not-found state with a path to its legal parent.
- Capability removed: replace with nearest applicable parent/root; do not leave a dead selected tab.
- Authorization denied: remove protected presentation immediately, classify the failure, and preserve the valid account/product scope when possible.
- Offline with verified retained data: keep the route and data, mark stale/offline non-disruptively, and recover automatically.
- Offline without data: retain route-shaped geometry and provide truthful account/server/download alternatives.
- User abandons a workflow: Back/cancel unwinds it without committing drafts; returning later reconciles durable authority before resuming.
- User changes origins/devices mid-flow: onboarding/deep-link authority is reloaded from durable state; navigation state never claims the operation succeeded.
- Late async completion: ignored when viewer, route, intent, or activation generation changed.
- Duplicate activation: coalesced and reconciled, not stacked.

Raw router, URL parser, JavaScript, framework, or transport errors MUST NOT be user-visible.

## NAV-16. Platform adaptation

| Platform | Native mechanism expected | Outcome that cannot change |
| --- | --- | --- |
| Web/Server Web | semantic links/buttons, browser history, React Router or successor, DOM focus | canonical identities, browser-credible Back, keyboard access, persistent frame |
| iOS/iPadOS | native stack, tabs, sheets, interactive edge Back | same hierarchy, one authority, invoker restoration, player continuity |
| Android | native stack, predictive/system Back, tabs/sheets | same hierarchy and Back priority before OS exit |
| tvOS | native/maintained route stack plus tvOS focus engine | current/focused/selected distinction, semantic focus restoration, rail root behavior |
| Android TV/Fire TV | platform navigation plus D-pad focus | every action reachable, predictable Back to root then OS, semantic focus outcomes |
| Roku | SceneGraph route/focus implementation | same destination grammar, Back outcomes, semantic focus identity, no coordinate-owned product law |
| Cast | receiver/player state plus controller navigation | receiver does not invent browsable history; controller owns media selection |

Platform conventions MAY strengthen predictability. They MUST NOT remove a destination, user choice, Back level, accessible path, or security fence because it is inconvenient to implement.

## NAV-17. Privacy and security

- Route state, analytics, logs, crash reports, restoration, and links MUST contain only bounded opaque resource identifiers and approved presentation intent.
- Private search queries, file paths, credentials, contact email, PINs, tokens, grants, and signed URLs MUST NOT be placed in ordinary route state or focus IDs.
- A title shown from cached navigation is cosmetic until viewer authorization is verified.
- External links use allowlisted origins and strict parsing. An HTTP origin is permitted only for explicit local development/server contexts, never a public universal link.
- Screen names and accessibility announcements MUST NOT reveal a prior viewer's media after a scope transition.
- Navigation observability cannot block or mutate navigation.

## NAV-18. Conformance

`conformance.json` is the minimum platform-neutral outcome suite. `navigation-state.schema.json` defines the persisted product-level record shape. Existing `tv-interaction-outcomes.v1.json` projections MUST converge on these rules; a divergent generated Roku copy is not a second authority.

Required evidence includes:

- destination grammar parse/serialize round trips and malicious-input rejection;
- root, secondary, replace, coalescing, modal, Back, and player histories;
- pointer, touch, keyboard, remote, gamepad, and assistive activation parity;
- television native spatial outcomes, opt-in focus guides, virtualization, refresh, removal, and bounded restoration;
- viewer/profile/server transition fencing and stale async cancellation;
- deep-link authorization, expiry, mismatch, interruption, and legal Back path;
- route restoration mismatch and silent fallback;
- real iOS/Android browser Back gestures and tvOS/Android TV/Roku remote checks;
- reduced-motion and accessibility announcement review.

Minimum applicability:

| Rule family | Web | Client Core | Apple | Android | Roku | Cast |
| --- | --- | --- | --- | --- | --- | --- |
| Destination grammar/identity | required | authority | required | required | required | controller only |
| History/Back | required | outcomes | required | required | required | receiver limited |
| Deep links/restoration | required | authority | required | required | supported subset | controller only |
| Directional focus | keyboard subset | fixtures | tvOS required | TV required | required | not applicable |
| Player continuity | required | outcomes | required | required | required | required |
| Viewer fencing | required | authority | required | required | required | required |

## NAV-19. Contract exceptions

An exception requires product-owner approval, the closest durable code comment, an entry in the contract exception registry, and conformance evidence. The comment MUST be exactly:

```text
PORTICO-CONTRACT-EXCEPTION
Contract: PC-NAVIGATION-INPUT#<section-id>
Scope: <platform, surface, or capability>
Decision: <the exact deviation>
Reason: <why the canonical rule cannot or should not apply>
Approved-by: <name or recorded decision reference>
Review-when: <expiry date, capability change, or other objective trigger>
Evidence: <test or fixture that prevents accidental expansion>
```

When a defect applies to more than one client, change this contract and align all clients rather than accumulating platform exceptions.

## NAV-20. Comparative precedent

Plex and Jellyfin were reviewed as evidence, not authority. The adopted lessons and rejected precedents are recorded in [`precedent-review.md`](../../../References/Contract%20Baseline%202026-09-05/navigation-and-input/precedent-review.md). The central conclusion is that recognizable cross-platform roots and platform-native mechanics are useful, while focus and Back behavior become fragile when they are implicit, index-based, or split across old and new UI stacks.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.

## October scope and evidence

Libraries remember per-profile/server tabs, sorts and filters on the device. Search text stays in scoped memory outside normal URLs/history. Server-selected See all replaces ID guesses. Subgroup sorting/filter parity is required but still in progress. Paused screen/focus restoration and deep appended-list freshness retain gaps.

Sources: [web-app](../../../_program/reports/web-app.md), [native-app](../../../_program/reports/native-app.md), [native-sync](../../../_program/reports/native-sync.md), [freshness-clients](../../../_program/reports/freshness-clients.md), [grouping-sorts](../../../_program/reports/grouping-sorts.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
