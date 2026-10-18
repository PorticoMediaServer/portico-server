# V09 — Verification of web architecture, web player and web UX feature parity

## Summary
The first pass mostly holds up, with meaningful scope corrections. Tier 1: **5 Confirmed, 15 Confirmed, narrower, and 7 Unsettled — needs runtime**. Tier 2: **12 Confirmed, 10 Confirmed, narrower, and 1 Refuted**. All 50 requested checks are complete; the 31 Low entries are copied without re-checking. These are source verdicts, not runtime reproductions.

The strongest launch concerns are **B-17.1**, dropped quality/version/audio intent, and **B-17.4**, a recovery loop that cancels viable slow HLS loads. Queue replacement, external video-pause handling, group selection and container refresh are also real. New finding **N-V09.1** shows that the wired v1 bridge cannot enter the advertised engine-failure reporting/fallback path; the relevant tests inject a different API. Repair that integration together with queue-preserving replanning.

**B-18.8 is refuted for current Portico Account admission:** it now issues local server authority, so the legacy authority gates do not establish the claimed owner denial. Bulk trash is deliberately single-step and remains recoverable even at zero retention; the original irreversible-loss scenario does not apply to that path. Children may read their own limits, and compact users can still open Help through About. Five current High ratings become Medium after tracing these narrower triggers/impacts. Safari/AirPlay/fullscreen/Web Audio, buffer pressure, cue jitter and Back-restoration ordering need the focused acceptance checks described below.

Paths in citations are relative to `/home/justin/projects/Portico`; archive-internal hls.js paths identify members of the named shipped tarball. **S = hours; M = days; L = a week or more.** Sizes in the table are the primary repair scope; conditional/feature-complete alternatives are noted in the entries.

## Verdict table

Tier 1 entries first, followed by Tier 2.

| ID | Claim (short) | First pass (confidence / severity) | Verdict | Class | Your severity | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-16.1 | Back window can be saved after disposal | Probable / Medium | Unsettled — needs runtime | Defect (conditional) | Medium | S | No |
| B-16.2 | Old tab erases new remembered viewer | Probable / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-16.6 | Profile language/clock bypasses | Confirmed by reading / Low today; High future | Confirmed, narrower | Defect | Low today; Medium future | M | No |
| B-16.7 | One HTTP long-poll per tab | Probable / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-16.9 | Retry retains failed lazy import | Probable / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-16.11 | Reconnect unmounts routed page | Probable / Low–Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-16.16 | Virtual-list semantics/focus gaps | Confirmed by reading / Low–Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-16.17 | Primitive help/label/style gaps | Confirmed subclaims / Low (subclaims) | Confirmed, narrower | Defect / Design risk | Low | S | No |
| B-17.1 | Quality/version/audio choices dropped | Confirmed by reading / High | Confirmed | Defect | High | M | No |
| B-17.2 | Restart and Replay discard queue | Confirmed by reading / High | Confirmed, narrower | Defect | Medium | M | No |
| B-17.3 | External element pause is undone | Confirmed by reading / High | Confirmed, narrower | Defect | Medium | M | No |
| B-17.4 | Recovery aborts slow active loads | Confirmed by reading / High | Confirmed, narrower | Defect | High | M | No |
| B-17.5 | HLS loads zero before resume seek | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-17.6 | AirPlay and MMS HLS incompatibility | Probable / Medium | Unsettled — needs runtime | Unfinished feature | Medium (conditional) | M | No |
| B-17.13 | iPhone fullscreen fallback absent | Probable / Medium | Unsettled — needs runtime | Unfinished feature | Medium (conditional) | M | No |
| B-17.14 | High-bitrate buffer pressure | Speculative / Medium | Unsettled — needs runtime | Design risk | Medium (conditional) | S | No |
| B-17.15 | Late sessions and overlapping Next | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-17.17 | Original audio seeks read prefix | Confirmed by reading / Medium | Confirmed, narrower | Unfinished feature | Medium | M | Yes — audio seek mechanism |
| B-17.18 | iOS Web Audio background/interruption | Speculative / Medium | Unsettled — needs runtime | Design risk | Medium (conditional) | M | No |
| B-17.19 | Web Audio system-media activation | Speculative / Medium if it fails | Unsettled — needs runtime | Design risk | Medium (conditional) | M | No |
| B-17.24 | Mapping wobble may rebuild cues | Speculative / Low–Medium | Unsettled — needs runtime | Design risk | Low | S | No |
| B-17.25 | Wired adapter lacks behavior test | Design risk / Medium | Confirmed | Design risk | Medium | M | No |
| B-18.1 | Group click ID ignored | Confirmed by reading / High | Confirmed | Defect | Medium | S | No |
| B-18.5 | No library removal operation | Confirmed by reading / High | Confirmed | Missing feature mislabelled as a bug | Medium | L | No |
| B-18.6 | Container refresh uses item-only route | Probable / Medium | Confirmed | Defect / Unfinished feature | Medium | S | No |
| B-18.10 | Profile actions exceed management | Probable / Medium | Confirmed, narrower | Defect | Low | S | No |
| B-18.13 | Single-step whole-set bulk trash | Confirmed by reading / High potential impact | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-16.3 | Only index assets get immutable cache | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-16.5 | Grid measurement/render overhead | Confirmed by reading / Low–Medium | Confirmed | Design risk | Low | S | No |
| B-16.8 | General requests lack deadline | Confirmed by reading / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-16.10 | Unknown enum rejects document | Confirmed by reading / Medium | Confirmed | Design risk | Medium | M | No |
| B-16.12 | Browser floor/CSS fallback absent | Confirmed by reading / Low/Medium conditional | Confirmed, narrower | Design risk | Low | S | No |
| B-16.13 | No signed-in main/skip link | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-16.14 | Fixed title; no route announcement | Confirmed by reading / Low–Medium | Confirmed | Defect | Medium | S | No |
| B-16.15 | Focused card omits viewing state | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-16.23 | No hook-dependency enforcement | Confirmed by reading / Medium | Confirmed | Design risk | Low | S | No |
| B-17.7 | Unload lacks final report/Stop | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-17.8 | Expired lease fails instead of recovery | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-17.9 | Player keys ignore modifiers | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-17.11 | Touch picture tap pauses | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-18.2 | Locked list edit omitted from changes | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-18.3 | Search-history control not wired | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-18.4 | Aggregate tabs silently cap at 200 | Confirmed by reading / Medium | Confirmed | Defect | Medium | M | No |
| B-18.7 | Detail/card actions differ | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-18.8 | Hosted owner denial premise outdated | Confirmed by reading / Medium | Refuted | Design risk (legacy gates) | Low | S | No |
| B-18.9 | Admin lacks web management tools | Confirmed by reading / Medium | Confirmed, narrower | Unfinished feature | Medium | M | No |
| B-18.11 | Preference-consumer check skips | Confirmed by reading / Medium | Confirmed | Defect | Medium | S | No |
| B-18.12 | Fixed English on screens | Confirmed by reading / Low today; Medium future | Confirmed | Defect | Low today; Medium future | M | No |
| B-18.14 | Compact menu missing; Help still works | Confirmed by reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-18.26 | Search words enter route address | Confirmed by reading / Low–Medium | Confirmed, narrower | Defect | Medium | S | No |

## Tier 1 — detailed verification

### B-16.1 Back restoration can read an already-disposed collection
- **Claim:** Leaving a library clears its pages before the child saves the Back-restoration window.
- **What I traced:** `apps/portico-server/web/src/app/browse.ts:59,69` owns and disposes the collection; `apps/portico-server/web/src/screens/library/Library.tsx:114,371-385` reads it during the child cleanup; `apps/portico-server/packages/client-core/src/collections/windowed.ts:312-317` clears the pages immediately; `apps/portico-server/web/src/app/list-position.ts:48` discards an empty save. All paths are in the routed Library screen (`apps/portico-server/web/src/app/router.tsx:21`). No alternative save path was found. The pure store tests do not exercise React cleanup ordering.
- **Verdict:** **Unsettled — needs runtime.** The destructive ordering is plausible and the code offers no protection, but this review did not establish the actual parent/child passive-effect order in the installed React/router combination. Cheapest check: on a disposable local fixture, scroll beyond page one, open a title, then Back while observing whether `saveListWindow` receives nonempty items before disposal; nonempty storage and instant position restore refute the claimed ordering, zero items confirm it. No production endpoint is needed.
- **Class:** Defect if the ordering is reproduced.
- **Severity:** **Medium**; affects viewers returning to a deep library list, not all library navigation.
- **Fix direction:** Save the window in the owning cleanup before disposal, or defer collection disposal consistently with `apps/portico-server/web/src/app/content.ts:28-30`. **Size: S**; browse hook, Library screen and one lifecycle test.
- **Related:** B-16.11 can independently cause list unmount; B-19.13 concerns native geometry, not this lifecycle. No direct duplicate in the B-index. **Backlog:** No.

### B-16.2 An old tab can erase the new tab's remembered viewer
- **Claim:** A profile/account change in one tab causes another tab's eventual renewal to clear the new browser-wide selected viewer.
- **What I traced:** `apps/portico-server/packages/client-core/src/viewer-selection.ts:57-60,83,87-92` publishes the candidate, revokes the prior token and clears storage unconditionally. `apps/portico-server/packages/client-core/src/server-connections.ts:225-231` rejects a stored family mismatch; `apps/portico-server/packages/client-core/src/session-refresh.ts:139-148` dispatches sign-out. `apps/portico-server/web/src/app/session.tsx:991-998,983-989,944-968` reaches `commit.clear()`; `apps/portico-server/web/src/bridge/viewer-selection-storage.ts:42-50` deletes the shared record without comparing ownership. The separate routed-credential store correctly compares families (`apps/portico-server/packages/client-core/src/server-connections.ts:147`), but that does not guard the selected-viewer store. Rotation only rewrites an existing matching family (`apps/portico-server/web/src/app/session.tsx:442`). The same-family multi-tab renewal test (`apps/portico-server/packages/client-core/test/session-refresh.test.ts:241-259`) does not cover profile replacement.
- **Verdict:** **Confirmed, narrower.** The shared-record deletion is a source-established race once the old tab renews. It does not instantly sign out every tab: the new tab continues from memory, and a vanished viewer record primarily breaks reload restoration. Direct sign-in is sufficient; Hosted behavior requires its additional account-context path.
- **Class:** Defect.
- **Severity:** **Medium**; households using several tabs lose persistent sign-in after switching viewers.
- **Fix direction:** Add atomic compare-and-clear by server and session family to selected-viewer storage and pass the retiring identity through disconnect. Preserve deliberate explicit sign-out semantics; do not silently move an old tab into another person's profile. **Size: M**; viewer-selection, browser storage, session provider and cross-tab tests.
- **Related:** B-22.3 (HTTP lock coordination), B-22.4 (store read failure) and B-22.5 (replacement detection) agree on adjacent credential risks, but have distinct triggers. **Backlog:** No; unexplained Apple sign-outs are not proof of this browser bug.

### B-16.6 Language and clock preferences have bypasses
- **Claim:** Some screens follow the profile's regional catalogue and clock, while others use fixed US English or browser formatting.
- **What I traced:** `apps/portico-server/web/src/app/i18n.ts:19-37` reads `region.locale`/`region.hourCycle`, but publishes its global object in a passive effect. `apps/portico-server/web/src/screens/search/Search.tsx:18`, `saved/Saved.tsx:20,194`, `people/Person.tsx:16,20` and `live/guide.ts:24-34` bypass it. Other renderers use `currentI18n()` without a catalogue subscription. `portico-internal/Project Architecture/06 Clients.md:19` explicitly promises regional English; `portico-internal/Product Contracts/accessibility-and-language/contract.md` A11Y-13 requires consistent formatting. English-only launch does not excuse the already exposed regional/clock settings.
- **Verdict:** **Confirmed, narrower.** Fixed-catalogue and browser-clock paths are definite. Global-effect ordering is a separate latent risk; I did not prove every `currentI18n()` screen stays stale. Fixed byte abbreviations alone do not prove a launch defect.
- **Class:** Defect.
- **Severity:** **Low today**; regional English/clock users see inconsistent labels and times. **Medium if another language ships**, rather than the first pass's prospective High: translation polish breaks, not core media access.
- **Fix direction:** Use the subscribed profile I18n object in screens and pass it into formatting helpers; unify date/time formatting. **Size: M**; app/i18n, screens and shared formatters.
- **Related:** Merge B-18.12 and B-18.24 into this; B-18.22 is the raw-string subset. **Backlog:** No.

### B-16.7 Each tab reserves an HTTP connection for events
- **Claim:** Several tabs on HTTP/1.1 can consume the origin's connection budget with continuous long-polls.
- **What I traced:** `apps/portico-server/web/src/player/engine.tsx:119-133` creates and starts an EventsClient for every signed-in shell; `apps/portico-server/packages/client-core/src/playback-v1/events.ts:134-143,195-204` always polls when no stream opener is supplied, with a 25-second wait and no visibility pause. `apps/portico-server/server/internal/httpapi/events_routes.go:75-104` accepts that long-poll. It is independent of whether media is playing. The artwork budget acknowledges the browser connection constraint (`apps/portico-server/web/src/ui/artwork-store.ts:31-38`).
- **Verdict:** **Confirmed, narrower.** The connection reservation and contention condition are real; the exact starvation duration and threshold depend on browser scheduling, other traffic and negotiated protocol. Plain HTTP is not itself sufficient to prove every request stalls.
- **Class:** Design risk; choosing HTTP is deliberate, unlimited duplicate tab feeds are not a required trade-off.
- **Severity:** **Medium** for many tabs sharing an HTTP/1.1 origin; negligible on multiplexed HTTP/2.
- **Fix direction:** Share a feed where supported, or close idle background polls and reconcile on foreground without disrupting active playback. SSE alone still consumes an HTTP/1.1 connection. **Size: M**; engine/events and browser tab coordination.
- **Related:** B-08.6 concerns server event fan-out, B-22.25 retry synchronization; adjacent, not duplicates. **Backlog:** No.

### B-16.9 Lazy-import retry cannot fetch an upgraded chunk
- **Claim:** A tab using an old build can fail on a newly visited lazy screen after an upgrade, and boundary reset repeats the rejected import.
- **What I traced:** `apps/portico-server/web/src/app/router.tsx:17-40` creates module-lifetime React.lazy identities. `apps/portico-server/web/src/screens/NotFound.tsx:28-33` resets the route boundary instead of reloading. `apps/portico-server/server/internal/httpapi/installation.go:65-68` refuses a missing asset; no preload-error/build-version recovery was found in web source. Reset does not replace the lazy object, whose rejected promise remains cached.
- **Verdict:** **Confirmed, narrower.** Requires deletion/replacement of the old assets and an as-yet-unloaded route chunk; screens already loaded keep rendering. A missing chunk caused by a transient delivery error has the same retry limitation.
- **Class:** Defect.
- **Severity:** **Medium**; upgrade-time open tabs need a manual reload for some screens.
- **Fix direction:** Recognize dynamic-import failures and offer/reload a fresh document once with a loop guard. **Size: S**; router/RouteErrorScreen and preload-error handling.
- **Related:** B-16.10 is API version skew, not asset loading. **Backlog:** No.

### B-16.11 Reconnecting replaces and unmounts the route
- **Claim:** A route outage lasting over 2.5 seconds discards the current screen's local state.
- **What I traced:** `apps/portico-server/web/src/shell/Shell.tsx:174` wraps the routed Outlet, not just a visual layer. `apps/portico-server/web/src/shell/Connection.tsx:26-51` replaces children with StatusScreen after the timer; `apps/portico-server/web/src/bridge/server-connections.ts:46-49` forwards network changes. Shared providers outside the Outlet survive, but page-owned state does not. Settings' beforeunload protection cannot intercept this React unmount (`apps/portico-server/web/src/screens/settings/ServerForms.tsx:199`).
- **Verdict:** **Confirmed, narrower.** Unsaved page-local forms and panels can be lost; not all search state is lost because Search keeps a scoped module-level session. A blip resolved before the timer does not trigger replacement.
- **Class:** Defect; the status screen is intended, avoidable loss of drafts is not.
- **Severity:** **Medium** for users editing settings during roaming/reconnect; Low for ordinary browsing.
- **Fix direction:** Keep currently authorized content mounted under a blocking inert overlay; still discard it on viewer/authorization changes. **Size: S**; Connection overlay and shell.
- **Related:** B-16.1 concerns one restoration consequence; B-22.14 concerns route failure detection. **Backlog:** No.

### B-16.16 Virtualized grids omit full-list position and focus retention
- **Claim:** Assistive technology only sees mounted cells and a focused cell can disappear when scrolled out.
- **What I traced:** `apps/portico-server/web/src/ui/WindowedGrid.tsx:118-120,174-197` renders only the overscan window under a region; cells carry no total/position semantics and there is no focused-row pin or focus fallback. Library uses it at `apps/portico-server/web/src/screens/library/Library.tsx:424-432`; Show also uses it. Card focus handling does not keep a removed grid cell mounted. A11Y-4.7 requires a deterministic fallback.
- **Verdict:** **Confirmed, narrower.** Missing position semantics are definite; the resulting focus destination is browser/assistive-technology dependent. Ordinary keyboard focus that remains in the visible window is unaffected.
- **Class:** Defect.
- **Severity:** **Medium** for keyboard/screen-reader use of long grids.
- **Fix direction:** Choose suitable collection semantics, publish total and item position, and pin the focused row or restore focus to a predictable surviving target before eviction. **Size: M**; WindowedGrid and list/card adapters.
- **Related:** B-16.13 and B-16.15 are separate accessibility barriers; B-19.13 is native virtualization. **Backlog:** No.

### B-16.17 Primitive accessibility omissions
- **Claim:** Help text is disconnected on Select/TextArea, option IDs can become names, and font/contrast support has gaps.
- **What I traced:** `apps/portico-server/web/src/ui/Field.tsx:19,43-62` emits help/error IDs without linking Select/TextArea, unlike Input at line 31. `apps/portico-server/web/src/ui/Tabs.tsx:32-39` permits fallback to IDs, but the production Segmented call sites inspected provide readable labels. Pixel type sizes are in `apps/portico-server/web/src/ui/base.css` and `tokens.css`; no forced-colors/prefers-contrast styling was found. Browser zoom still works and CSS omissions alone do not demonstrate bad contrast.
- **Verdict:** **Confirmed, narrower.** Automatic help association is the concrete defect (callers can manually supply aria-describedby). Raw IDs and contrast are latent concerns, not demonstrated broken production controls; pixel sizing does not prove failure of the contract's zoom/reflow requirement.
- **Class:** Defect for help association; Design risk for the remaining primitive defaults.
- **Severity:** **Low**; screen-reader users may miss field help, while errors also have role=alert.
- **Fix direction:** Mirror Input's describedby composition for Select/TextArea and require explicit option labels; verify high-contrast/text-size behavior before changing tokens. **Size: S** for association/labels, M for any measured styling gaps; ui/Field, Tabs, tokens.
- **Related:** B-16.13–16 and B-18.22 agree on adjacent accessibility/localization debt. **Backlog:** No.

### B-17.1 Requested quality, version and audio are dropped
- **Claim:** The player visibly accepts these choices but starts the server's default presentation again.
- **What I traced:** `apps/portico-server/web/src/player/Player.tsx:347-362` sends quality/version to engine.play; `apps/portico-server/web/src/player/engine.tsx:220-225` calls the installed V1QueuePlayer. `apps/portico-server/packages/client-core/src/index.ts:858-863,1092-1095` sends an audio choice through the same path. `apps/portico-server/packages/client-core/src/playback-v1/v1-queue-player.ts:275-276` explicitly ignores all three arguments; `:332,360` starts without them. The server supports these fields (`apps/portico-server/server/internal/playbackv1/sessions.go:78-97`). `apps/portico-server/web/src/player/options.ts:46-64` installs a default-track list, then marks the rendition plan unavailable; no production call to installAudioPlan was found. Requested audio is subsequently labelled selected by `apps/portico-server/packages/client-core/src/index.ts:853-855`.
- **Verdict:** **Confirmed.** Neither the route nor server supplies a substitute for the dropped user request. The unwired media adapter does not change this path.
- **Class:** Defect.
- **Severity:** **High**; viewers cannot lower stream quality or reliably choose an alternate version/audio track using exposed controls.
- **Fix direction:** Map offered choices into v1 session change/start fields and preserve the existing queue occurrence; selected labels must reflect the committed presentation. **Size: M**; V1QueuePlayer, PlaybackService, Player/options and shared v1 request mapping.
- **Related:** Merge B-02.6; B-02.12 concerns subtitle replanning ceilings. Shared native callers merit the same repair, but native playback was not independently exercised. **Backlog:** No.

### B-17.2 Player restarts replace the queue with one item
- **Claim:** A quality/version/audio restart or Replay loses later series entries.
- **What I traced:** `apps/portico-server/web/src/player/engine.tsx:220-225` → `apps/portico-server/packages/client-core/src/playback-v1/v1-queue-player.ts:275-276,318-349` creates a one-item queue. Replay calls PlaybackService.play (`apps/portico-server/packages/client-core/src/index.ts:1311-1313`). Server queue creation explicitly retires the device's previous queue (`apps/portico-server/server/internal/playbackv1/queue_commands.go:412-418`). The service contains a route-failure escalation that calls play (`apps/portico-server/packages/client-core/src/index.ts:1453-1466`), but the wired V1PlaybackApi has no reporter and normal web starts do not populate lastPlay; see N-V09.1.
- **Verdict:** **Confirmed, narrower.** User-triggered choices and Replay definitely replace the queue. The claimed automatic-fallback trigger is currently unreachable through the wired bridge; if enabled without changing the restart path, it would cause the same queue loss.
- **Class:** Defect.
- **Severity:** **Medium** (first pass High); series continuation is interrupted, but the next episode remains accessible and there is no media/data loss.
- **Fix direction:** Replan/restart the current queue entry with the same queue and occurrence; Replay must restart that entry rather than create a queue. **Size: M**; core queue player/service and web engine. Coordinate with B-17.1.
- **Related:** B-17.1 shares the repair; B-20.16 is native Next wiring, not queue replacement. **Backlog:** No.

### B-17.3 External video pauses are reapplied as Play
- **Claim:** A pause performed directly on the media element is undone because Portico's desired state remains playing.
- **What I traced:** `apps/portico-server/web/src/player/Player.tsx:200-203,438` forwards an ordinary pause as a fact. `apps/portico-server/packages/client-core/src/index.ts:1432-1437` updates position but only changes intent for ended; `:770-780` immediately reapplies the adapter. `apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts:428-438` sees paused element plus playing intent and calls play. The final-pause guard (`apps/portico-server/web/src/bridge/web-playback-binding.ts:14-15`) only covers completion. `apps/portico-server/web/src/bridge/useSystemListening.ts:12-17` installs media-session pause handlers only for audio kinds.
- **Verdict:** **Confirmed, narrower.** A mid-title external element pause follows this loop. Which OS/headset/PiP controls deliver that event varies by browser; those device examples were not reproduced. Portico's own Pause first updates intent and works.
- **Class:** Defect.
- **Severity:** **Medium** (first pass High); affected viewers cannot rely on browser/OS pause controls, though the application's Pause remains available.
- **Fix direction:** Register video Media Session actions and reconcile unsolicited element pauses into intent while distinguishing internal load/seek/teardown pauses. **Size: M**; Player, adapter, system-listening and PlaybackService.
- **Related:** B-20.7 independently identifies the native intent/observation mismatch; agree conceptually, different adapters. **Backlog:** No.

### B-17.4 Recovery can repeatedly cancel a viable slow fragment
- **Claim:** Short load budgets and periodic startLoad calls turn recoverable slow delivery into repeated aborted downloads.
- **What I traced:** `apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts:191-215` caps fragments at 8 seconds to first byte/15 seconds total; `:447-450` enters recovery on an eight-second stall; `:516-536` calls its retry at 1,2,4,8,10-second intervals until more than two seconds are buffered or a 90-second empty-buffer interval elapses. Read-only inspection of the shipped `third_party/hls.js/hls.js-1.7.2-portico.1.tgz`, `package/src/controller/stream-controller.ts:141-147` and `base-stream-controller.ts:176-186`, establishes that startLoad stops/aborts the current fragment. The library's default fragment total is 120 seconds (`package/src/config.ts:596-599`). Guards for route blocking and successful recovery exist but do not protect an in-flight slow load.
- **Verdict:** **Confirmed, narrower.** The cancellation mechanism is definite. Eventual failure needs a stream/link that cannot finish a useful fragment between retries; buffered playback may survive. Near the final two seconds, repeated loads are possible, but a normal ended transition can settle playback before an error.
- **Class:** Defect.
- **Severity:** **High**; weak-server/remote viewers can be denied otherwise playable HLS media precisely when patient buffering would help.
- **Fix direction:** Retry only after a load actually fails or stops making progress; do not restart a healthy active load, and give fragments a budget compatible with their duration/size. **Size: M**; wired adapter's recovery/loading policy and deterministic fake-loader tests.
- **Related:** B-03.8 is server retention pressure, not client abort policy; B-17.5 magnifies startup work. **Backlog:** No.

### B-17.5 HLS initially loads from zero before applying resume
- **Claim:** Resume requests first load the beginning, then seek to the requested position.
- **What I traced:** `apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts:205` passes zero unless recovering a route; `:328-359` waits for metadata/seekability to assign pending seek. `apps/portico-server/packages/client-core/src/index.ts:1029` installs the real resume target. Server legacy HLS can start work at zero (`apps/portico-server/server/internal/playback/hls.go:771-785`) and starts a missing requested segment at its index (`:900-905`); `apps/portico-server/server/internal/playback/service.go:851-857` also dispatches a separate finite-delivery path, so one converter behavior must not be generalized to all HLS.
- **Verdict:** **Confirmed, narrower.** Wrong initial client start position is definite. A redundant converter start/restart depends on delivery mode, existing artifacts and server readiness; it is not established for every resume.
- **Class:** Defect.
- **Severity:** **Medium**; resumed HLS titles can start more slowly and consume needless network/conversion work.
- **Fix direction:** Initialize hls.js from the pending/session resume target and retain the seek-completion fence. **Size: S**; wired adapter, with coverage of zero/nonzero starts and server timeline mapping.
- **Related:** B-17.4 is slow-load recovery; B-17.17 is audio prefix reading. **Backlog:** No.

### B-17.6 Safari AirPlay with media-source HLS
- **Claim:** Safari offers AirPlay even when the current HLS source cannot be handed to a receiver.
- **What I traced:** `apps/portico-server/web/src/player/PlayOn.tsx:15-29` gates on picker availability alone. The adapter prefers hls.js when supported (`apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts:178-188`); native HLS is only its unsupported-engine fallback. The shipped hls.js archive's `package/src/utils/mediasource-helper.ts:3-14` prefers ManagedMediaSource; `package/src/controller/buffer-controller.ts:325-333` sets disableRemotePlayback for it. No receiver-fetchable alternate source is attached by Portico.
- **Verdict:** **Unsettled — needs runtime.** Source confirms an offered action incompatible with the preferred MMS path, but picker/receiver behavior on the supported Safari build was not tested. The absolute claim covering every HLS session is too broad: the native fallback differs. Cheapest check: current Safari plus one Apple TV, compare direct-play and an HLS session using MMS, invoke Play on → AirPlay, and check whether the receiver plays and timeline remains correct. HLS success refutes the asserted failure on that build; disabled picker/no handoff confirms it.
- **Class:** Unfinished feature on the MMS path; existing direct/native support should be retained.
- **Severity:** **Medium**, conditional; Safari viewers using converted/remuxed AirPlay are affected.
- **Fix direction:** Choose native HLS or attach a safe native alternate for external playback, then verify grant/timeline handling; otherwise gate the unavailable action honestly. **Size: M**; PlayOn and adapter/client capabilities.
- **Related:** B-13 entries about Portico TV discovery are separate; no direct duplicate in B-index. **Backlog:** No.

### B-17.13 Fullscreen lacks an unsupported-browser fallback
- **Claim:** The exposed Fullscreen action silently does nothing on iPhone engines lacking element fullscreen.
- **What I traced:** `apps/portico-server/web/src/player/Player.tsx:236-243` only calls stage.requestFullscreen when present and only gives a notice when a called API rejects. There is no webkitEnterFullscreen or fallback in web source. The player uses playsInline (`:431`).
- **Verdict:** **Unsettled — needs runtime.** Silent no-op on an engine without requestFullscreen is definite; whether the current supported iPhone/Safari build has that limitation is not established here. Cheapest check: open a video on the intended minimum iPhone Safari version, press Fullscreen and record element/native fullscreen plus captions/controls; successful transition refutes the device-wide claim.
- **Class:** Unfinished feature for unsupported fullscreen engines.
- **Severity:** **Medium**, conditional; affected phone viewers cannot get the offered fullscreen presentation.
- **Fix direction:** Capability-gate and explain the action or implement native-video fullscreen with caption/control acceptance. **Size: M** for complete native fallback, S for honest capability gating; Player and subtitle surface.
- **Related:** B-17.12 concerns double-click, not API availability. **Backlog:** No.

### B-17.14 High-bitrate buffer memory is not established
- **Claim:** Fixed time targets may overfill source buffers for high-bitrate remuxes.
- **What I traced:** `apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts:199-211` has a low-memory branch (20 seconds forward, 8 back) and a normal branch (30 forward, 30 back), plus 16/32 MiB maxBufferSize. In the shipped archive, `package/src/controller/base-stream-controller.ts:1451-1462`, maxBufferSize is used to derive a time target with **Math.max**, then capped by maxMaxBufferLength; it is not a hard memory cap. Thus the normal time floor wins at high bitrates. Existing hls.js quota adaptation is a mitigation, not proof of no stalls.
- **Verdict:** **Unsettled — needs runtime.** Source confirms a bitrate-independent minimum, not a real quota/stall defect. Cheapest check: one local high-bitrate remux in an intended browser, play beyond both buffer targets, record buffered ranges, quota errors/recovery and memory; repeated quota/stall cycles establish a defect, stable bounded playback refutes the harmful effect for that device.
- **Class:** Design risk.
- **Severity:** **Medium**, conditional; high-bitrate media on limited-memory browsers is the relevant audience, not every HLS viewer.
- **Fix direction:** Measure first; if harmful, cap time targets from bitrate and available-memory class, including back buffer. **Size: S** policy change after acceptance measurements; wired adapter.
- **Related:** B-17.4 is a separate timing failure. **Backlog:** No.

### B-17.15 Superseded successful requests can leave an unowned session
- **Claim:** Late sessions are dropped without Stop; simultaneous Next requests can leave the client on the old session.
- **What I traced:** `apps/portico-server/packages/client-core/src/playback-v1/v1-queue-player.ts:332-333,458-468` drops successful stale-generation results; the 412 handler refreshes the queue but never adopts the first response's session. Native adoption explicitly stops stale ownership (`:392-397`), ordinary adoption does not. `apps/portico-server/server/internal/playbackv1/queue_commands.go:994-1004,1085-1108` advances using an expected queue revision and creates a replacement session. `apps/portico-server/packages/client-core/src/index.ts:1022` does not immediately fail when an old session is still installed; its eventual server-ended event/report does (`:718-722`). Retry uses old PlaybackService position against the refreshed current queue entry (`v1-queue-player.ts:605-610`).
- **Verdict:** **Confirmed, narrower.** Missing cleanup of a successful discarded advance-session is definite; the double-Next failure needs overlapping requests and response ordering. Queue create only starts a session when startSeconds is absent (`apps/portico-server/packages/client-core/src/playback-v1/v1-queue-player.ts:329-349`); a discarded create with an explicit position may orphan only its queue. Some later successful replacements retire an orphan, so not every dropped answer occupies a slot for the whole lease. Stream counts are unlimited by default; configured caps make this more disruptive.
- **Class:** Defect.
- **Severity:** **Medium**; rapid queue actions can leave unused slots and broken continuation, especially with owner stream caps.
- **Fix direction:** Serialize/fence advance commands, settle ambiguous 412 from a fresh current-session read, and stop successful sessions that lost ownership. Bind retry position to the same item/occurrence. **Size: M**; V1QueuePlayer/bridge and concurrency tests.
- **Related:** B-17.7 is unload abandonment, B-17.2 queue replacement; distinct triggers. **Backlog:** No.

### B-17.17 Non-indexed audio resume walks the prefix
- **Claim:** Deep resume of original MP3/FLAC can read a large prefix before producing sound, exceeding the seek deadline.
- **What I traced:** `apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts:394-396` selects DecodeAudioRender when an eligible audio plan and effects are active. Gapless is enabled by default (`apps/portico-server/packages/client-core/src/audio-effects.ts:8,22`), so this is not an obscure disabled setting. `apps/portico-server/web/src/bridge/DecodeAudioRender.ts:94-95,130-164` opens TrackStream at the requested frame; `apps/portico-server/web/src/bridge/audio/track.ts:149-169` walks packets when demuxer.seek is absent. MP3/FLAC demuxers supply no seek method; FLAC parses unused seek points (`apps/portico-server/web/src/bridge/audio/flac.ts:64-81,96`). RangeReader fetches 256 KiB blocks into a 64 MiB cache (`apps/portico-server/web/src/bridge/audio/bytes.ts:9,15,76-98`); PlaybackService's pending seek deadline is 15 seconds (`apps/portico-server/packages/client-core/src/index.ts:1334`). Converted audio and indexed MP4 use other paths.
- **Verdict:** **Confirmed, narrower.** Large prefix reading is definite on original decode plans, with cached blocks reducing requests. Timeout/failure depends on prefix size and link throughput; this is not every MP3/FLAC play. The prefix is parsed, not decoded, correcting the Backlog wording.
- **Class:** Unfinished feature (seek indexing), with a user-visible timeout consequence.
- **Severity:** **Medium**; long single-file audiobooks resumed remotely are the clearest affected case.
- **Fix direction:** Wire FLAC seek points and MP3 Xing/VBRI/CBR indexing, use bounded verified fallback seeking, and budget startup against actual work. **Size: M**; audio demuxers/TrackStream and seek deadline behavior.
- **Related:** B-17.5 concerns video; B-10.30 concerns MP3 parsing near the start, not deep seek. **Backlog:** **Yes**, mechanism explicitly listed; failure consequence is additional context.

### B-17.18 iOS background/interruption behavior of Web Audio
- **Claim:** The bare AudioContext path may stop under lock/background/silent-mode or remain interrupted after a call.
- **What I traced:** `apps/portico-server/web/src/bridge/DecodeAudioRender.ts:60-71,143-151` creates a context and has gesture handling but no statechange listener; transport only handles suspended. `apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts:394-396` clears the media element's source. No navigator.audioSession use exists in web source. Gapless defaults make this path ordinary when the server supplies an eligible plan. The effect of these omissions is platform behavior, not derivable solely from TypeScript.
- **Verdict:** **Unsettled — needs runtime.** Cheapest check: the same eligible music track on the minimum and current supported iOS Safari, toggle silent mode, lock for a minute, then simulate one call interruption; continued/resumed sound refutes each suspected failure, a stopped context unrecovered after foreground confirms it. Use a private disposable fixture.
- **Class:** Design risk pending device acceptance.
- **Severity:** **Medium**, conditional; listening reliability on iPhone/iPad may be affected.
- **Fix direction:** Set the platform audio-session playback policy where available and reconcile context interruptions without bypassing user pause/autoplay rules. **Size: M** including device acceptance; DecodeAudioRender and listening bridge.
- **Related:** B-17.19 shares the active-media boundary but is a controls claim. **Backlog:** No.

### B-17.19 Web Audio may not activate system media controls
- **Claim:** Media Session metadata/handlers might not activate lock-screen/media-key controls without a sourced media element.
- **What I traced:** `apps/portico-server/web/src/bridge/useSystemListening.ts:12-32` sets metadata, handlers and playbackState correctly for music/books; `apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts:394-396` leaves the element unsourced in rendering mode. The existing projection knows about the dormant element (`apps/portico-server/web/src/bridge/system-listening-state.ts`). Nothing proves browsers ignore those handlers; explicit Media Session state is a plausible counterargument.
- **Verdict:** **Unsettled — needs runtime.** Cheapest check: on each intended Safari/Chromium/Firefox target, play an eligible Web Audio track, then test hardware pause/next and the lock-screen card. Working controls refute the risk on that engine; absent/nonworking controls while in-page controls work confirm it. Do not infer one browser's outcome for all.
- **Class:** Design risk.
- **Severity:** **Medium**, conditional; listeners using system controls are affected if the engine does not treat the context as active media.
- **Fix direction:** Preserve current handlers; add an appropriate media-element/platform activation strategy only after measurement, with no unintended audible output. **Size: M**; listening bridge/renderer and browser acceptance.
- **Related:** B-17.18 shares the Web Audio platform boundary; B-17.3 is the definite video-intent bug. **Backlog:** No.

### B-17.24 Subtitle mapping changes rebuild the cue set
- **Claim:** Tiny per-fragment mapping changes can cause frequent full cue replacement and visible flicker.
- **What I traced:** `apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts:228-238` compares the exact string of startPTS minus start; `apps/portico-server/web/src/player/options.ts:127-145,207` clears and reconstructs cues on that event. No tolerance or diff exists. The renderer is otherwise fenced to the current resource and valid authorized document. Whether the mapping varies and whether visible cues blink depends on actual fragments/browser text-track rendering.
- **Verdict:** **Unsettled — needs runtime.** The full rebuild on a changed offset is confirmed; the frequent wobble/flicker claim is not. Cheapest check: play one copied HLS stream with sidecar subtitles across ten keyframe-cut fragments and log offset deltas/cue replacement counts; identical offsets refute repeated rebuild for that fixture, tiny changing deltas plus replacements establish needless work.
- **Class:** Design risk.
- **Severity:** **Low** (first pass Low–Medium); possible subtitle rendering overhead, without evidence of a present failure.
- **Fix direction:** Compare offsets with a justified tolerance and retain active cues where possible; do not conceal real timeline discontinuities. **Size: S**; adapter/options.
- **Related:** Merge B-04.39, which makes the same conditional claim. **Backlog:** No.

### B-17.25 The shipped adapter lacks a behavior-driving test
- **Claim:** Existing fake-media tests cover an adapter that the web does not instantiate.
- **What I traced:** `apps/portico-server/web/src/player/Player.tsx:16,138` imports/useWebPlaybackAdapter; the only WebMediaPlayer references found are its definition and `apps/portico-server/web/test/web-media-player.test.ts`. `apps/portico-server/web/test/playback-traffic.test.ts` asserts source patterns, and binding tests cover pure fences; neither drives the hook's load, pause, resume or recovery state machine. Core queue tests exercise a different boundary.
- **Verdict:** **Confirmed, narrower.** “No behavioral test” applies to this wired adapter, not to all playback code or all ownership fences.
- **Class:** Design risk (coverage gap).
- **Severity:** **Medium**; all browser playback changes lack direct regression protection at the faulty adapter boundary.
- **Fix direction:** Extract the adapter state machine behind a small fake-element/loader interface and drive real user/system actions, deadlines and stale events, or unify onto the tested adapter after parity work. **Size: M**; wired adapter, Player integration and web tests.
- **Related:** B-16.23 is general tooling; B-25.10 concerns release device gates. These agree but should not become duplicate fixes. **Backlog:** No.

### B-18.1 Episode-group selection discards the clicked ID
- **Claim:** Range/hidden-season group controls always select unassigned_absolute.
- **What I traced:** `apps/portico-server/web/src/screens/detail/Show.tsx:239-249,321,338` passes the chosen group into the hook; `apps/portico-server/web/src/app/detail.ts:57` accepts no argument and hardcodes another group. `apps/portico-server/packages/client-core/src/show-workspace.ts:83` validates against server groups and either throws or selects that hardcoded group. The synchronous throw is outside the appended promise catch. The core works with a supplied ID; the web binding is the fault, not a missing server range feature.
- **Verdict:** **Confirmed.** Both tab and dropdown paths are wired to this callback, with no alternate handler or default guard correcting it.
- **Class:** Defect.
- **Severity:** **Medium** (first pass High); affected long/flat shows cannot switch groups, while ordinary season navigation and direct playback still work.
- **Fix direction:** Forward the selected ID and convert validation failures into a visible recoverable outcome. **Size: S**; app/detail, Show binding and a group-selection integration check.
- **Related:** No direct duplicate in B-index; B-18.17 is a distinct season-route defect. **Backlog:** No; “per-show settings not built” lists other settings, not these built switches.

### B-18.5 There is no shipped library-removal operation
- **Claim:** An owner cannot remove an unwanted library from the product.
- **What I traced:** `apps/portico-server/web/src/screens/server/Libraries.tsx:258-273` offers rename, scan and music lyrics; source removal is separate. `apps/portico-server/server/internal/httpapi/route_lanes.go:89` registers source deletion, with no library deletion; production handler/function searches found no library removal operation. SQL deletions found in tests are fixture setup, not user routes. The developer-gallery mock is not shipped navigation. `portico-internal/Product Contracts/settings-and-operations/contract.md:55,272` reserves removal to the owner and names it as a destructive operation; I found no decision prohibiting it.
- **Verdict:** **Confirmed.** Removing all sources is not the same as removing the library identity and navigation entry.
- **Class:** **Missing feature mislabelled as a bug.** The first pass already describes it as a product gap; retain it as planned feature work, not a broken existing control.
- **Severity:** **Medium** (first pass High); owners cannot clean up accidental/unwanted libraries, but this does not damage playable media.
- **Fix direction:** Define detach/history/resource retention explicitly, then add an owner-only library-removal job/API and confirmed UI without deleting source files implicitly. **Size: L**; administration/catalog, reference cleanup, library routes and Libraries screen.
- **Related:** No direct duplicate in B-index. **Backlog:** No.

### B-18.6 Container menus offer an item-only metadata refresh
- **Claim:** Refresh metadata on a show/season/album/artist/book fails on the item refresh endpoint.
- **What I traced:** `apps/portico-server/web/src/screens/shared/EntryActions.tsx:88-91,125` enables refresh solely for a local owner and sends the entry ID. `apps/portico-server/server/internal/httpapi/detail.go:66-79` delegates to QueueRefresh; `apps/portico-server/server/internal/metadata/details.go:30-42` requires catalog entity kind 1. Crucially, `apps/portico-server/server/internal/metadata/screen_base.go:76-81` → `apps/portico-server/server/internal/entityid/entityid.go:68-78` resolves the exact public entity, not an underlying playable child. Show rows are kind 2, artists 5 and albums 6 (`apps/portico-server/server/internal/catalog/content_entities.go:193-214`), so the auditor's unresolved ID question does not rescue the path. Bulk refresh uses the same check and names unsupported_item_kind (`apps/portico-server/server/internal/httpapi/bulk_command_hooks.go:82-95`).
- **Verdict:** **Confirmed.** The ordinary container public ID is rejected; successful item refresh is not evidence of container support.
- **Class:** Defect in action availability; recursive container refresh is an Unfinished feature.
- **Severity:** **Medium**; owners get an offered action that cannot perform its stated task on common container pages.
- **Fix direction:** Immediately gate to supported targets, then implement bounded per-container refresh jobs if desired. **Size: S** for honest gating; **M** for bounded container refresh; EntryActions/shared action model, metadata and bulk hooks.
- **Related:** Merge B-06.10 (movie-only refresh); same backend limitation, web exposure adds the trigger. **Backlog:** No.

### B-18.10 Profile actions exceed management authority
- **Claim:** Non-managing profiles are offered Limits and Delete they cannot use.
- **What I traced:** `apps/portico-server/packages/client-core/src/presentation/profile-actions.ts:60,74-75` does not gate either action by canManage. `apps/portico-server/web/src/screens/settings/Profiles.tsx:121-161` renders both; Delete calls the preview. `apps/portico-server/server/internal/identity/direct_profiles.go:438-449,461-469` requires directCallerTx with management; `apps/portico-server/server/internal/identity/direct_accounts.go:351-353` refuses a valid non-manager. For Limits, the counterevidence is explicit: `apps/portico-server/server/internal/identity/restrictions.go:225-248` allows a profile to **read its own** limits, while saving requires management (`:280`). `apps/portico-server/web/src/screens/settings/Security.tsx:49-72` nevertheless renders editable controls and Save for that read-only viewer. Tests at `apps/portico-server/server/internal/identity/restrictions_test.go:103-115` encode own-profile read permission.
- **Verdict:** **Confirmed, narrower.** Delete and other profiles' limits fail before opening. A child's own limits are intentionally readable and open successfully; the defect there is editable controls that cannot save, not merely seeing Limits.
- **Class:** Defect in UI permission handling; server enforcement works.
- **Severity:** **Low** (first pass Medium); nuisance/false affordance, not a restriction bypass or unauthorized deletion.
- **Fix direction:** Hide Delete unless canManage, allow own Limits as an explicitly read-only view, and hide edit/Save when unauthorized. **Size: S**; profile-actions, Profiles and Security dialog.
- **Related:** B-18.9 is server-role privilege, distinct from account profile management; B-01.21 concerns other account-level controls. **Backlog:** No.

### B-18.13 Single-step bulk trash is recoverable, including at zero retention
- **Claim:** Bulk Delete has no confirmation, with potentially irreversible whole-set impact when retention is zero.
- **What I traced:** `apps/portico-server/web/src/screens/shared/SelectionBar.tsx:47,75-96` sends a fenced whole-set trash job without confirmation; line 95 explicitly records Justin's single-step decision. `apps/portico-server/server/internal/httpapi/bulk_command_hooks.go:23-27` wires **TrashJobItem**, not the single-title deletion implementation. `apps/portico-server/server/internal/administration/trash_job.go:35-38,83-93,113-120` moves files into held recoverable trash and gives zero retention an expires_ms of zero. Expired-only purge selects **expires_ms>0** (`apps/portico-server/server/internal/administration/deletion.go:750`), and manual EmptyTrash requires a typed count (`:770-771`). `apps/portico-server/server/internal/administration/trash_job_test.go:57-74` explicitly tests zero-retention recovery. Library media-deletion policy is also enforced (`trash_job.go:54-55`); default trash retention is 30 days (`apps/portico-server/server/internal/administration/library_settings.go:172`).
- **Verdict:** **Confirmed, narrower.** One click does trash the set. The feared immediate irreversible zero-retention bulk deletion is **refuted**: it remains held indefinitely until an explicit purge. Positive retention can expire, so an unnoticed accidental action still has a residual risk.
- **Class:** **Deliberate trade-off**, not a defect in missing confirmation.
- **Severity:** **Medium** (first pass High potential impact); a mistaken whole-set action disrupts availability and requires recovery, with eventual loss possible after positive retention/purge, not immediate destruction by this button.
- **Fix direction:** Keep the authorized single-step behavior; improve outcome/count visibility and easy restore/undo access without adding confirmation. **Size: S** for messaging/recovery shortcut, M for bulk restore UX; SelectionBar and trash UI. A new confirmation would change Justin's recorded decision and is not assumed authorized.
- **Related:** B-18.29 covers unrelated resource removal. No direct bulk-trash duplicate in B-index. **Backlog:** No.

## Tier 2 — quick checks

Each check below opened the entry and its cited source. Severities are current product impact; future-language and unsupported-device conditions remain explicit. Backlog was checked for all entries; none below has the specific defect listed.

| ID | Independent check, verdict and planning direction |
|---|---|
| B-16.3 | **Confirmed, narrower — Defect, Medium, S.** `apps/portico-server/server/internal/httpapi/installation.go:84-87` only grants immutable caching to assets/index-*; Vite route chunks/fonts fall outside that pattern. Exact historical asset counts were not re-counted; update static asset matching after distinguishing hashed files from unversioned public assets. |
| B-16.5 | **Confirmed — Design risk, Low, S.** `apps/portico-server/web/src/ui/WindowedGrid.tsx:130-155,201` rebuilds the observer and reads layout for moving first/last; `apps/portico-server/web/src/screens/library/Library.tsx:431-432` supplies fresh renderer callbacks. Stabilize measurement subscriptions/renderers if measured cost warrants it; no slowdown was reproduced. |
| B-16.8 | **Confirmed, narrower — Design risk, Medium, M.** `apps/portico-server/packages/client-core/src/index.ts:338-370,387-414` passes only caller cancellation, and RouteConnection.fetch adds no operation deadline. Console, lyrics, refresh and Search have separate deadlines, so “every request” would be wrong; add suitable default read deadlines without blindly replaying ambiguous writes. Merge B-22.8. |
| B-16.10 | **Confirmed — Design risk, Medium, M.** `apps/portico-server/packages/client-core/src/home.ts:63-71` rejects unknown artwork shape while dropping unknown content kinds. Do not weaken security/ownership validation; define additive presentation fallbacks or an explicit version-compatibility response. No current version-skew failure was established. |
| B-16.12 | **Confirmed, narrower — Design risk, Low, S.** `apps/portico-server/web/src/ui/Card.module.css:5,9-10` relies on :has and color-mix without fallbacks; `apps/portico-server/web/vite.config.ts:19` has no explicit build target. Unsupported CSS removes intended presentation, but affected supported devices and blanket “transparent surfaces” are not established; declare a browser floor and test/gate required features. |
| B-16.13 | **Confirmed — Defect, Medium, S.** `apps/portico-server/web/src/shell/Shell.tsx:169-175` and `apps/portico-server/web/src/ui/Page.tsx:6-7` render divs, with no signed-in main landmark/skip link found. Add a persistent main target and first-focusable skip link; ServerlessShell's main is not the signed-in path. |
| B-16.14 | **Confirmed — Defect, Medium, S.** `apps/portico-server/web/index.html:8` supplies the fixed title; no document.title assignment, router head metadata or route-announcement mechanism was found. Update title and announce committed destination changes without narrating every loading state. |
| B-16.15 | **Confirmed, narrower — Defect, Medium, S.** `apps/portico-server/web/src/ui/Card.tsx:91,94,100-111` names the focused link with only title and gives it shortcut help, not caption/watched/progress description. Captions can still be read separately; add a concise state description to the card's focus target without duplicating repeated shortcut hints. |
| B-16.23 | **Confirmed — Design risk, Low, S.** `apps/portico-server/web/package.json:6-32` check scripts/dependencies contain no hook lint, and design/string scripts do not enforce dependencies. Add a narrowly scoped hook rule and resolve meaningful violations; this is a process risk, not a present user defect. |
| B-17.7 | **Confirmed, narrower — Defect, Medium, M.** No playback pagehide/unload keepalive handler exists; `apps/portico-server/web/src/player/engine.tsx:119,133` has no journal and normal React cleanup only helps when it runs, while `apps/portico-server/server/internal/playbackv1/sessions.go:24` bounds abandonment at 120 seconds. Send best-effort final timeline/Stop on pagehide with BFCache semantics considered; abrupt process death still relies on leases. |
| B-17.8 | **Confirmed — Defect, Medium, M.** `apps/portico-server/packages/client-core/src/playback-v1/legacy-bridge.ts:214,254-266` learns expiry and `apps/portico-server/packages/client-core/src/index.ts:718-722` fails the current play rather than starting a replacement. Recover specifically lease_expired at the same queue occurrence/position; do not restart an owner-terminated or transferred session. |
| B-17.9 | **Confirmed — Defect, Medium, S.** `apps/portico-server/web/src/player/Player.tsx:214-229` ignores modifier keys and prevents defaults for lowercase shortcut matches outside fields/dialogs. Skip Ctrl/Cmd/Alt combinations and composition; no runtime claim that every browser delivers every reserved shortcut is necessary. |
| B-17.11 | **Confirmed — Defect, Medium, S.** `apps/portico-server/web/src/player/Player.tsx:425,440-442` reveals chrome on pointerdown then toggles playback on click, irrespective of pointer type. Branch touch gestures to reveal/toggle chrome while preserving explicit transport and mouse actions. |
| B-18.2 | **Confirmed — Defect, Medium, S.** `apps/portico-server/web/src/screens/shared/MetadataEditor.tsx:197-199` builds edit.values but excludes it from the change test; later validation handles it. Include values in dirty/change assembly and verify a second edit to an already locked Tags/Labels field. |
| B-18.3 | **Confirmed, narrower — Defect, Medium, M.** `apps/portico-server/web/src/screens/search/Search.tsx:33-45,91,110-120` records scoped in-memory recents and clears only them; the shared search supports record but this caller never requests it (`apps/portico-server/packages/client-core/src/search.ts:142-149`). The server API exists (`apps/portico-server/server/internal/httpapi/search.go:150-192`); wire preference-respecting history/read/clear and committed searches, without recording typeahead. Fresh web searches do not secretly populate the server store. |
| B-18.4 | **Confirmed — Defect, Medium, M.** Aggregate pivots bypass ordinary page limiting (`apps/portico-server/server/internal/catalog/browse_engine.go:400-405`), read at most 200 facets, then report the returned length as total (`:978-987`; `browse_contract.go:25`). `apps/portico-server/web/src/screens/library/LibraryViews.tsx:143` reads one Genres page; add real paged aggregates/counts and client continuation, including audiobook Series. |
| B-18.7 | **Confirmed, narrower — Defect, Medium, M.** `apps/portico-server/packages/client-core/src/presentation/detail-actions.ts:108-117` omits queue/delete/group inputs and offers downloads solely from playback; `apps/portico-server/web/src/screens/detail/Detail.tsx:71-108` renders that model. Card grouping action is conditional on an existing controllable room (`EntryActions.tsx:106`), so not every card always offers Watch Together; unify available actions/permission results and download offer gating. |
| B-18.8 | **Refuted for the current sign-in path — Design risk in legacy gates, Low, S.** Full trace below: signing in through a Portico Account now produces local server authority, so it does not lose metadata tools on that account basis. |
| B-18.9 | **Confirmed, narrower — Unfinished feature, Medium, M.** The selectable admin role (`apps/portico-server/web/src/screens/server/People.tsx:85`) does not qualify for owner-only web management (`apps/portico-server/web/src/app/session.tsx:1027`; `apps/portico-server/packages/client-core/src/presentation/settings-structure.ts:493`). It is not literally benefit-free: Administrative grants wider library visibility and member-management powers server-side (`apps/portico-server/server/internal/identity/access_tier.go:43-59`); finish the intended delegated management surface or remove misleading promises. |
| B-18.11 | **Confirmed — Defect, Medium, S.** `apps/portico-server/packages/client-core/scripts/check-preference-consumers.mjs:39-42` appends apps/apple/src and exits successfully when absent; maintained source is apps/native. Fix path/discovery and make a missing required checkout an explicit failing check in the intended CI job; update stale exceptions. |
| B-18.12 | **Confirmed — Defect, Low today, M.** Search/Saved/Person and other modules bind defaultI18n at module load; see B-16.6's independently opened paths. Merge into that repair; a second language would raise impact to Medium, not a separate fix per document. |
| B-18.14 | **Confirmed, narrower — Defect, Medium, S.** Full trace below: compact browsing loses the general menu, but Help is still reachable through Settings → About and playback feedback still exists. |
| B-18.26 | **Confirmed, narrower — Defect, Medium, S.** `apps/portico-server/web/src/screens/search/Search.tsx:69-74` replaces the current route with q after 400 ms, and `apps/portico-server/web/src/app/router.tsx:118-120` explicitly accepts it. It does not append a history entry per keystroke, but the private words are still in a history/bookmark/sync-capable address, contrary to `portico-internal/Product Contracts/architecture-amendments.md:125`; keep ordinary search in scoped memory and remove q from route state. |

### B-18.8 — promoted: Portico Account sign-in now uses local authority
- **Claim:** A Portico Account owner can administer/delete but cannot edit/refresh metadata because the client excludes hosted authority.
- **What I traced:** The gates do differ (`apps/portico-server/web/src/screens/shared/EntryActions.tsx:91,124-126`; `apps/portico-server/web/src/screens/shared/SelectionBar.tsx:93-96`). However `apps/portico-server/web/src/app/session.tsx:839` calls **porticoSignIn**, whose shared implementation sends an identity assertion to **/v1/direct/sign-in** (`apps/portico-server/packages/client-core/src/portico-servers.ts:80-87`). That registered route verifies Hosted identity then calls **PorticoSignIn** (`apps/portico-server/server/internal/httpapi/direct_identity.go:66-92`). `apps/portico-server/server/internal/identity/portico_accounts.go:66-105` loads server membership and uses directSignInTx; `apps/portico-server/server/internal/identity/direct_accounts.go:281,299` issues **local** authority. Choosing another profile likewise issues local (`apps/portico-server/server/internal/identity/direct_profiles.go:379-384`). Old migrated credentials get session_migrated (`apps/portico-server/server/internal/identity/refresh.go:34-37`), and the web readmits them (`session.tsx:991-996`). These are current production routes, not tests.
- **Verdict:** **Refuted** for the alleged current-user trigger. “Signed in through a Portico Account” is no longer synonymous with viewer.authority hosted. A fresh admitted owner passes the same local gate as a password owner. Old branches still differ, but their presence alone does not establish this user-facing failure on the current membership path.
- **Class:** Design risk in legacy compatibility gates; the original audit's explanation is outdated.
- **Severity:** **Low** (first pass Medium); cleanup/compatibility risk, with no demonstrated current Portico Account owner denied metadata editing by this condition.
- **Fix direction:** Verify supported migration/older-server compatibility and remove or explain obsolete hosted-authority distinctions; retain tests that direct and Portico-admitted owners receive the same actions. **Size: S**; session/action models and compatibility tests. No new ownership privilege is needed.
- **Related:** B-21.12 assumes the same authority premise on native; flag it for its lane to re-trace, without claiming native verification here. **Backlog:** No.

### B-18.14 — promoted: missing compact menu, with working alternatives
- **Claim:** Under 720 px the web offers no profile/server switching, feedback or Help.
- **What I traced:** `apps/portico-server/web/src/shell/Shell.tsx:168,366-425,452-463` omits Rail/ProfileMenu and points the top avatar to Settings. `apps/portico-server/web/src/screens/settings/AccountPanels.tsx:49-59,73-82` exposes profile management and Disconnect, not a chooser. Counterevidence: About has an actual Help link (`apps/portico-server/web/src/screens/settings/AccountPanels.tsx:110-113`), and About is authored for all viewers (`apps/portico-server/packages/client-core/src/presentation/settings-structure.ts:284`). The player also opens FeedbackDialog for Report a problem (`apps/portico-server/web/src/player/Player.tsx:491,624`); this remains usable on compact VOD/audio playback. The Hosted account Servers page shows memberships and Leave, not Switch (`apps/portico-server/web/src/screens/account/Servers.tsx:92-108`).
- **Verdict:** **Confirmed, narrower.** The general profile/server switch menu and general feedback entry disappear. “No way to open Help” is false; playback-specific feedback is also available. Disconnect/reconnect is a workaround for switching, not an equivalent direct switch.
- **Class:** Defect in compact navigation coverage.
- **Severity:** **Medium**; phone-browser households have avoidable ceremony to change viewing identity/server and no general feedback entry.
- **Fix direction:** Reuse ProfileMenu from the compact top-bar avatar and preserve the existing Settings destination inside it. **Size: S**; Shell/ProfileMenu.
- **Related:** No direct duplicate in B-index; B-21.3 concerns a native tablet destination, not this menu. **Backlog:** No.

## Not re-checked (Low)

- **B-16.4 — WindowedGrid's mapped-scroll mode has discontinuities near the ends** — Not re-checked (Low, per worklist).
- **B-16.18 — The strict style CSP probably blocks the scroll-lock stylesheet that Radix dialogs inject** — Not re-checked (Low, per worklist).
- **B-16.19 — The browse-capabilities cache never expires and is not keyed by a revision** — Not re-checked (Low, per worklist).
- **B-16.20 — Failed list pages ignore Retry-After and retry on a fixed 5 s clock** — Not re-checked (Low, per worklist).
- **B-16.21 — Paged admin lists keep an old cursor when their inputs change** — Not re-checked (Low, per worklist).
- **B-16.22 — Credits paging can double-append and hides failures** — Not re-checked (Low, per worklist).
- **B-16.24 — Dev-only screens are emitted into the production web folder** — Not re-checked (Low, per worklist).
- **B-16.25 — CI and the shipped image build the web with different Node versions** — Not re-checked (Low, per worklist).
- **B-17.10 — Shortcuts stop working after using a slider** — Not re-checked (Low, per worklist).
- **B-17.12 — Double-click for fullscreen also pauses and resumes** — Not re-checked (Low, per worklist).
- **B-17.16 — Reported speed is always 1; other speeds cause extra reports** — Not re-checked (Low, per worklist).
- **B-17.20 — Subtitles disappear during an outage the video survives** — Not re-checked (Low, per worklist).
- **B-17.21 — In Firefox, subtitles can sit under the controls** — Not re-checked (Low, per worklist).
- **B-17.22 — The seek bar has two overlapping ways to seek** — Not re-checked (Low, per worklist).
- **B-17.23 — Automatic level switching in hls.js is left on** — Not re-checked (Low, per worklist).
- **B-17.26 — Small items** — Not re-checked (Low, per worklist).
- **B-18.15 — A show's cast stops at twelve with no "See all"** — Not re-checked (Low, per worklist).
- **B-18.16 — In the episode grid layout an episode has no ⋯ menu** — Not re-checked (Low, per worklist).
- **B-18.17 — A season card's address stops working after switching season** — Not re-checked (Low, per worklist).
- **B-18.18 — Back buttons behave differently per page and can leave Portico** — Not re-checked (Low, per worklist).
- **B-18.19 — "This device" badges sit on settings that apply to every browser the profile uses** — Not re-checked (Low, per worklist).
- **B-18.20 — Non-owners can add one title to a collection but not several** — Not re-checked (Low, per worklist).
- **B-18.21 — Changing someone's role takes effect on selection, with no confirmation** — Not re-checked (Low, per worklist).
- **B-18.22 — English text written straight into components** — Not re-checked (Low, per worklist).
- **B-18.23 — Disc headings depend on an English phrase inside a subtitle** — Not re-checked (Low, per worklist).
- **B-18.24 — Dates and language names follow the browser, not the profile's Language & region** — Not re-checked (Low, per worklist).
- **B-18.25 — Cards in Saved are not links** — Not re-checked (Low, per worklist).
- **B-18.27 — Parts that exist but cannot be reached or do nothing** — Not re-checked (Low, per worklist).
- **B-18.28 — Home's "See all" guesses the destination from the row's id** — Not re-checked (Low, per worklist).
- **B-18.29 — Removing an entry from a playlist or collection is immediate, with no Undo** — Not re-checked (Low, per worklist).
- **B-18.30 — The episode panel loses facts on a direct visit and does not link people** — Not re-checked (Low, per worklist).

## New findings

### N-V09.1 The wired web bridge cannot report engine failure or request fallback
- **Claim:** Ordinary web video decode/source failure cannot enter the intended server-informed compatibility fallback, even when another delivery route exists.
- **What I traced:** `apps/portico-server/web/src/player/engine.tsx:117-121` constructs **PlaybackService with V1PlaybackApi**, not HttpLocalApi. `apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts:278-281,557-559` invokes service.engineFailed for exhausted media/decode errors. `apps/portico-server/packages/client-core/src/index.ts:1454-1458` immediately fails if api.reportRouteFailure is absent. `apps/portico-server/packages/client-core/src/playback-v1/legacy-bridge.ts:42-296` implements the v1 bridge without this optional method; the implemented reporter at `apps/portico-server/packages/client-core/src/index.ts:453-454` belongs to **HttpLocalApi**, which is not the injected PlaybackApi. The registered endpoint already accepts v1 sessions and maps their presentation to the backend (`apps/portico-server/server/internal/httpapi/delivery_routes.go:152-180`); this is a missing bridge call, not a missing server API. No assignment/decorator supplying it was found. An independent second guard also fails on a first normal web play: engine.startSingle calls V1QueuePlayer directly (`apps/portico-server/web/src/player/engine.tsx:225`), so lastPlay is never recorded through PlaybackService.play (`apps/portico-server/packages/client-core/src/index.ts:1094`). Neither beginQueueIntent nor adoptQueueSession installs it (`:1009-1033`). Existing `apps/portico-server/packages/client-core/test/engine-failure.test.ts:4-14` injects a fake API with reportRouteFailure and calls service.play, bypassing both integration gaps.
- **Verdict:** **Confirmed by independent static trace.** No runtime is necessary to show that this optional-method guard prevents the subsequent report call. This does not mean every decode failure is recoverable: a compatible fallback must exist server-side. It means the wired web never attempts the advertised reporting/escalation at this boundary. Hls.js's limited local recoverMediaError attempts are separate and remain present.
- **Class:** Defect.
- **Severity:** **Medium**; viewers encountering a wrong capability/file decision get an error and repeated default retries where automatic server replanning might succeed. Frequency and real-browser recovery success remain unmeasured.
- **Fix direction:** Implement failure reporting on the v1 bridge using the existing v1-capable endpoint, capture the originating user choices and item/occurrence on queue adoption, and replan that occurrence without replacing the queue. **Size: M**; V1PlaybackApi, V1QueuePlayer, PlaybackService and a test using the actual wired combination.
- **Related:** Corrects the automatic-fallback premise in B-17.2 and the favorable fallback description in source document 17. B-02.13 concerns ignored timeline errors; its separate reporting route does not rescue this missing caller. B-20.6 should re-check its native bridge before relying on engine failures being reported. **Backlog:** No.

## Disagreements worth Justin's attention

- **B-18.8 is refuted for the current path.** Fixing user-visible Portico Account owner metadata denial should not be planned from legacy hosted-authority branches; current admission issues local authority. B-21.12 needs the same identity check in its own lane.
- **B-18.13 is a deliberate, recoverable bulk-trash choice.** The immediate irreversible zero-retention scenario is wrong for the wired bulk job. Positive retention/purge still carries a residual accidental-loss risk. Preserve the recorded single-step decision; improve recovery visibility without silently adding a new confirmation flow.
- **B-18.10:** children are deliberately allowed to read their own limits. Hide unauthorized edit/Delete, not the useful read-only policy view.
- **B-18.14:** the compact menu gap is real, but Help remains available in Settings → About, and player feedback works. Scope a compact-menu fix accordingly.
- **B-17.2:** quality/audio/version/Replay do discard the queue, but the claimed automatic-fallback trigger is unwired today; **N-V09.1** is the prior missing step. Plan those repairs together so enabling fallback does not introduce queue loss.
- Current High remains justified for **B-17.1** and **B-17.4**. I lower **B-17.2, B-17.3, B-18.1, B-18.5 and B-18.13** from High to Medium: they are real continuation/control/navigation/product or recovery issues, with narrower audience/impact than a general playback failure or immediate permanent deletion.
- **B-18.10** drops Medium → Low; **B-18.8** drops Medium → Low after refutation; **B-16.23** drops Medium → Low as a process risk. **B-16.6's future-language rating** drops High → Medium; current Low is unchanged. The variable Low–Medium ratings are resolved in the verdict table for planning, not claimed as measured severity.
- **B-16.1** remains open: its source lifecycle hazard is credible, but the specific React cleanup ordering was not reproduced. Do the small Back-restoration check before treating it as a confirmed release defect.

## Evidence boundary

- Reviewed the active working tree on `ehler-homelab-mini-1` (personal mini-1), 11 October 2026, against the complete V09 worklist and the full B entries in documents 16–18. Read the verification/original audit briefs, existing project guidance, Clients/Backlog, relevant normative playback, identity, language/accessibility, navigation, library/operations and decision/amendment passages. First-pass descriptions were treated as hypotheses, not evidence.
- **All 27 Tier 1 and 23 Tier 2 entries were reached.** Low entries below were copied from the worklist and deliberately not checked. B-index searches identified duplicates/adjacent claims; cross-lane IDs are merge pointers, not certifications of those lanes.
- Static review only: no Portico build, server, suite, browser, device, hosted/demo endpoint, MCP call or network request was used. No comparison code was executed. The shipped hls.js archive was read with Python tarfile directly to verify the exact loader/buffer implementation; no install/build occurred. No behavioral throwaway experiment was run.
- Browser scheduling/HTTP connection starvation, iOS fullscreen/AirPlay/background/media-key behavior, source-buffer quota behavior, cue-mapping wobble and the Back-restoration cleanup order remain runtime boundaries. Each unsettled entry names a focused local/disposable experiment and its interpretation; these are proposed checks, not checks completed.
- Timing, memory, accessibility presentation and device coverage have not been measured. Negative searches establish missing code in the inspected production source, not absence of every external integration. Historical generated bundle/asset counts were not independently certified.
- Context refresh was attempted as required but failed when context-sync tried to open its lock outside the writable sandbox. Existing guidance was readable; that failure does not change the audited source evidence.
- Only this lane output was written inside Portico. Scratch reading/report helpers were kept in `/tmp`; no source or Git state was modified.
