# Outcome

The native/shared logic pass is implemented and snapshotted. Apple apps now use a single scene lifecycle, reinstall cleanup cannot release a prior installation’s sign-in, and PIN/Keychain work no longer blocks HTTP submission. Apple/Android control traffic has reserved admission and a deadline including queue residence. Inactive native roots and covered stack pages freeze rendering. Runtime facts govern PiP and improve Fire TV/Leanback layout detection. Native privacy/badge settings are reachable. A verified remote connection can promote to a recovered LAN route without blanking its screen or calling Hosted.

Vega’s credential-redirect defect is contained by refusing sensitive exchanges before dispatch. **Vega sign-in, saved-session restore and authenticated control/streams are unavailable until a native transport that can refuse redirects is supplied.** This is a functional release gap, not a parity claim.

All 116 assigned register IDs have individual dispositions below. “Already fixed” means source evidence on the base; device/browser behaviour is not implied. Visual work and server contracts outside this lane are explicit handoffs. Work ran on personal mini-1 (`ehler-homelab-mini-1`, private address 100.120.50.24); no pushes, deployment, commits, package installation or full builds/suites were performed.

# Decisions I made

- Keep plain HTTP and describe its trust boundary accurately. A signed probe cannot authenticate later unencrypted sockets. No TLS enforcement/detection was added.
- Reserve ten concurrent read slots and control capacity within the existing 76-request bound. GET/HEAD are reads; mutations use control capacity. Apple has separate read/control socket pools; Android has separate dispatchers and keeps one-shot write bodies.
- Keep visited navigation/history for Back and freeze inactive rendering rather than arbitrarily dropping history. This does not bound retained-history memory.
- Treat Fire TV flavor and advertised Leanback as explicit TV facts. Do not guess that any large or keyboardless Android device must be a TV.
- Reinstall cleanup checks the marker before deleting; an unprepared iOS installation must have protected data available. Only six exact Portico Keychain services are deleted. Existing marked installations keep background playback/journal access while the device is locked. No server-side uninstall revocation is inferred.
- Vega’s installed NativeNetworkingKepler.sendRequest and RCTNetworking.kepler expose no redirect-policy parameter. Adding speculative fields or checking the final URL cannot prevent disclosure. Fail before sending credentials; do not invent a native transport implementation or add an unapproved dependency. Public nonce proof carries only a canonical origin and a 43-character nonce.
- Ship the existing regional English plural fallback. A non-English catalogue needs actual Hermes/Vega acceptance before being enabled.
- Shared packages were edited only in the canonical server worktree and copied with the required shared-packages script. Native’s cumulative patch also includes base-copy drift that the required sync reconciles (catalogue-failures, title identity, settings strings/server form rows); these are canonical source copies, not independent edits.
- Context-sync could not refresh because its lock directory is read-only. Existing canonical project standards, verification, project and attribution guidance were read.
- Apple migration follows [Apple’s scene lifecycle transition guidance](https://developer.apple.com/documentation/uikit/transitioning-to-the-uikit-scene-based-life-cycle) and [React Native’s factory integration](https://reactnative.dev/docs/integration-with-existing-apps.html); this establishes API direction, not a successful archive/device run.

# Bugs

- **B-16.1** — Already fixed: web/src/app/browse.ts retains the window through usePooled; the old parent-dispose/child-save ownership no longer applies. Browser Back acceptance remains device work.
- **B-16.2** — Already fixed: client-core viewer-selection uses conditional saved-family clearing; web selection storage coordinates shared writes.
- **B-16.3** — Already fixed: httpapi installation serves hashed assets immutably (also in FIX-LOG).
- **B-16.4** — Not fixed here: web/src/ui/WindowedGrid.tsx update still snaps at maxScrolled while small deltas move 1:1. Frontend owner must smooth the compressed end ranges.
- **B-16.5** — Already partly fixed: WindowedGrid measurement subscription is stable; Library.tsx EngineResults inline cell renderers still need frontend memoisation.
- **B-16.6** — Already fixed in the maintained locale helpers/reactive web catalogue; no new locale policy introduced.
- **B-16.7** — Not fixed here: plain-HTTP visible tabs can still own separate event requests. Web player/bridge needs an IndexedDB-coordinated feed owner or an equivalent HTTP-capable shared transport; secure-only sharing is insufficient.
- **B-16.8** — Already fixed: client-core index.ts requestDeadlineMs/run deadline includes ordinary requests; streams retain their distinct lifetimes.
- **B-16.9** — Already fixed: lazy-screen failure recovery reloads the failed bundle (FIX-LOG and web retry path).
- **B-16.10** — Already partly fixed: home/library presentation parsing has tolerant row handling. Older-client/newer-server compatibility is not a pre-release obligation; retain strict identity/security validation.
- **B-16.11** — Already fixed: web shell Connection keeps the routed page underneath its reconnect cover.
- **B-16.12** — Not fully fixed: web card focus fallback exists, but web/vite.config.ts still has no build.target/browser floor. Frontend/packaging owner must declare the supported engines and test CSS features.
- **B-16.13** — Already fixed: signed-in web shell has the main target and skip link.
- **B-16.14** — Already partly fixed: route titles/announcements exist. The previously reported rapid-heading and identical-heading announcement cases need web navigation acceptance, outside native non-screen scope.
- **B-16.15** — Already fixed: web Card names viewing/progress state at its focus target.
- **B-16.16** — Already fixed: web WindowedGrid position semantics and focus fallback are present; screen-reader acceptance remains unrun.
- **B-16.17** — Already partly fixed: Field help association and named options are present. Pixel type sizes and forced-colour styling are frontend work in web/src/ui/Field.tsx, Tabs.tsx, tokens.css and base.css.
- **B-16.18** — Not fixed here: web/src/ui/Dialog.tsx still uses Radix scroll locking; server CSP still has style-src self. Packaging/web owner must make the lock work under that CSP without broadly allowing arbitrary style injection.
- **B-16.19** — Already fixed: web browse.ts RevalidatingCache is viewer-scoped, bounded to 32 entries and rechecks each use. No server vocabulary revision exists.
- **B-16.20** — Already fixed: client-core collections/windowed.ts respects per-page server retry guidance.
- **B-16.21** — Already fixed: web/admin/cursor-read.ts resets ownership with its dependency identity.
- **B-16.22** — Already fixed: web/app/detail.ts credits paging has subject generation, per-flight ownership and surfaced error state.
- **B-16.23** — Not fixed here: web/package.json has no hook-rule enforcement. This is web tooling outside the permitted native/shared-code area; adding a new lint dependency also needs the programme dependency decision.
- **B-16.24** — Already fixed: production web route wiring gates developer routes; no production build was run to recount emitted chunks.
- **B-16.25** — Not fixed here: server Dockerfile uses Node 24 while server CI specifies 22.22.2. Packaging owner must align and pin both; this lane does not own server build wiring.
- **B-18.1** — Already fixed: web detail group selection carries the selected group id.
- **B-18.2** — Already fixed: MetadataEditor includes list values when determining changed locked fields.
- **B-18.3** — Already fixed: web Search uses committed searches and server history with preference-aware clear/read.
- **B-18.4** — Not fixed here: web LibraryViews GenresGrid still asks for one limit-200 page; browse aggregate/client paging must be reconciled by server and frontend owners.
- **B-18.5** — Already fixed on this base: DELETE /v1/admin/libraries/{id} is registered; shared administration consumers exist. This lane added no library route.
- **B-18.6** — Already fixed for invalid menu offers: EntryActions uses refreshMetadataPath(kind,id). Broader container refresh implementation belongs to its server owner.
- **B-18.7** — Already fixed in shared detail-actions/web Detail action composition; no new screen work here.
- **B-18.8** — Not present for current Portico Account admission: it receives local server authority, as the second review established. Do not resurrect a legacy hosted-authority route.
- **B-18.9** — Not fixed here: selectable admin tier still differs from exact-owner web management gates. Accounts/server and web owners must expose the actual delegated permissions in web/src/app/session.tsx and screens/server/People.tsx.
- **B-18.10** — Already fixed: profile action model gates management while preserving permitted read-only limits.
- **B-18.11** — Already fixed: check-preference-consumers.mjs points to apps/native rather than retired apps/apple.
- **B-18.12** — Already fixed: maintained web screens use viewer catalogue helpers rather than fixed module-scope en-US.
- **B-18.13** — Contradicted by deliberate single-step bulk deletion decision; no confirmation was added. The programme file-delete/trash semantics supersede the second review’s older recoverable-file assumption.
- **B-18.14** — Already fixed: compact shell has the profile/general menu; Help remains reachable through About.
- **B-18.15** — Already fixed at the frontend level: Show offers the supplied cast beyond twelve. This is not proof of server cast paging.
- **B-18.16** — Already fixed: episode grid has the action alternative.
- **B-18.17** — Already fixed: season routing preserves an actual season identity rather than substituting a season number into an id path.
- **B-18.18** — Already fixed: web internal Back helper supplies a Portico fallback.
- **B-18.19** — Already fixed: class-scoped settings use class wording, distinct from installation-local values.
- **B-18.20** — Already fixed: bulk personal collection actions follow permission availability rather than exact owner alone.
- **B-18.21** — Already fixed: People role selection opens askRole confirmation before setRole.
- **B-18.22** — Already partly fixed: web catalogue coverage improved. Metadata fallback field labels and any remaining player copy belong to their frontend owners; no screen text invented here.
- **B-18.23** — Not fixed in the web consumer: library-content already carries discNumber, but Entity.tsx album grouping still calls discNumberFromSubtitle(entry.subtitle). Frontend must use entry.discNumber and remove that English parsing path.
- **B-18.24** — Already fixed: maintained date/language formatters use the profile catalogue/region.
- **B-18.25** — Already fixed: Saved media cards supply navigation addresses.
- **B-18.26** — Already fixed: ordinary web Search uses scoped memory rather than writing private query text to route search.
- **B-18.27** — Not fully fixed: dead catalogue cleanup exists, but pinned resource controls, provider API-key applicability, remembered-device handler and registry-only choices require their owning screens/server pieces. See exact handoffs below.
- **B-18.28** — Already fixed on the client: Home uses homeRowSeeAll and row.seeAll destinations. Server must author the correct destination for each row; a library fallback cannot represent arbitrary future subgroups.
- **B-18.29** — Not fixed here: web/screens/saved/Resource.tsx immediate entry removal still needs a visible Undo action and an occurrence-safe restore operation.
- **B-18.30** — Not fixed here: web/screens/detail/EpisodePanel.tsx needs server-backed air date on direct visits and navigable guest/director/writer entries.
- **B-19.1** — Already fixed: app-level link-intents owns initial/warm links above Shell; pending-link fences/coalesces them. Scene adoption here preserves cold URL delivery.
- **B-19.2** — Already fixed: app/viewer-scope.ts caches by viewer/server value, so token renewal preserves service scope.
- **B-19.3** — Already fixed: Android Menu emits its own intent, and Focusable/useMenuKey routes it only to a menu action; no synthetic Select remains.
- **B-19.4** — Already fixed: secure-records preparation/recovery distinguishes temporary failure and confirmed key loss; older Android/Fire uses explicit reset confirmation.
- **B-19.5** — Already fixed for failure/retry and unbounded shown-set: ArtworkPipeline bounds shown to 4000 and has reload backoff. Remaining retry-URL cache duplication is a low-cost performance followup; not claimed eliminated.
- **B-19.6** — Already fixed: index.js installs error-capture before App; bounded redacted error-journal persists a fatal record and preserves framework fatal handling. Five journal tests passed; injected device fatal timing is untested.
- **B-19.7** — Already fixed: iPad plist has all four orientations. Archive/upload validation remains unrun.
- **B-19.8** — Not fixed: TelevisionDisplay still relies on RN internal flags and process-level density; rotation/font-scale behaviour needs Android TV hardware. The detection change here also supplies the same TV hint to density setup, but does not claim to repair RN rotation metric overwrites.
- **B-19.9** — Fixed here: Keychain, profile PIN and record work run on PorticoRecordQueue; transport submission stays on the module queue.
- **B-19.10** — Partly fixed here: a Fire TV flavor or Leanback feature supplies a TV hint before the app loads, and TelevisionDisplay uses the same hint. Generic boxes that declare neither television UI mode nor Leanback cannot safely be inferred as TVs; an explicit layout choice remains product/frontend work.
- **B-19.11** — Contradicted by the owner’s no-at-rest-encryption requirement: Vega sandboxed records remain plain files with integrity slots. No new key-storage promise or encryption dependency added.
- **B-19.12** — Fixed here: a missing Apple installation marker clears only six exact Portico services, after protected data is available on iOS. Cleanup failures remain retryable; established installations retain background access. This is local reinstall cleanup, not server revocation on uninstall.
- **B-19.13** — Already fixed for row geometry: Card/Grid reserves measured caption geometry. Large-offset float precision remains a hardware/large-library acceptance item, not reproduced here.
- **B-19.14** — Fixed in source here: inactive roots suspend their render tree and covered native stack screens set freezeOnBlur; visited roots/history remain retained. This reduces rendering, not a claim of bounded retained-history memory. TV focus and Vega acceptance remain unrun.
- **B-19.15** — Partly fixed here: hook driver fails explicitly when the expected React dispatcher seam is absent; native test config is checked in CI; macOS CI compiles/runs actual URL, download-owner and installation-storage contract tests. React-internal hook imitation and missing Vega SDK CI remain explicit gaps.
- **B-19.16** — Fixed for the identified non-screen paths: session cleanup/local-profile messages now use the catalogue; unused English fallback parameters were removed. direct-connect and browse already use active catalogue labels; Artwork’s label was fixed on base. Native string lint passes. No screen restyling performed.
- **B-19.17** — Not present for shipped catalogues: only regional English is shipped and its fallback rules agree. A future non-English launch requires actual Hermes/Vega plural-rule acceptance; no new polyfill dependency or unsupported-language claim added.
- **B-19.18** — Fixed in source here: iOS/tvOS plists declare a single application scene; scene-owned windows start React and preserve cold/warm allowed links. iOS forwards universal links/Handoff/Spotlight too. Xcode 27/device acceptance remains outstanding.
- **B-19.19** — Partly fixed here: PiP uses the exported runtime capability, including Android API/feature detection; Vega stays false. Unmounted native search keyboard and inert Android adjustResize are exact frontend/native followups, not removed blindly.
- **B-21.1** — Already fixed: Destinations has the permanent manual TV-code action.
- **B-21.2** — Already fixed: ProgramSheet cancellation uses confirmation.
- **B-21.3** — Already fixed: non-TV wide Rail offers Downloads with the same capability gate.
- **B-21.4** — Already fixed: independent bounded Saved indexing feeds Spotlight/Siri instead of depending on an absent Home My List shelf.
- **B-21.5** — Already fixed: Saved history/playlist/resource rendering uses virtualized PageList/windowed owners.
- **B-21.6** — Partly fixed here: shared settings structure exposes search.rememberHistory and notifications.badges on native, which already consume them. cardSizePercent/showBackdrops consumers still need frontend work; do not remove owner-overridable settings.
- **B-21.7** — Already fixed: SettingsPage class-scoped rows use the class labels.
- **B-21.8** — Not fixed: PlaylistScreen still sends an item-id container anchor. Existing QueueAnchor.position has no playlist revision/occurrence fence, so using a loaded-page index would be wrong after authorization filtering or a concurrent reorder. Needs the server-owned occurrence/revision selector, then native container-play and SavedPages wiring.
- **B-21.9** — Not fixed: PlaybackChoices still collapses subtitle languages and omits unknown language; PendingTrackChoice still accepts language/resource id only. Frontend must list catalogued resources/discoveries by source and stable identity; server/shared owner must carry a discovered file track through session materialisation without silently choosing a same-language alternative.
- **B-21.10** — Already fixed: SettingsPage supplies playerSettingsSupport playbackSpeed/audioEffects, and Vega’s override disables unsupported controls.
- **B-21.11** — Partly fixed on base: Saved view gathers up to 500 titles before playing, avoiding merely the loaded page. It is still not a full server query selector for larger views; no claim of whole-view playback beyond the bound.
- **B-21.12** — Already fixed: shared detail/menu edit permission follows actual owner permissions; current Hosted admission issues local server authority.
- **B-21.13** — Already fixed: Saved hook selection guard includes service identity as well as route.
- **B-21.14** — Already fixed for late results and duplicate navigation: pending-link/LinkFence binds viewer and intent; the underlying lookup need not finish before stale results are dropped. Lookup abortion remains an efficiency followup.
- **B-21.15** — Contradicted by the recorded Plex-like live-filter decision: Back keeps live changes. NAV-6.7 needs correction; no rollback workflow added.
- **B-21.16** — Already fixed semantically: Focusable offers a named accessibility menu action. Visible More affordances on recommendation/playlist cards and physical remote/VoiceOver/TalkBack validation remain frontend acceptance work.
- **B-22.1** — Already fixed: route-identity accepts the signed secure LAN alias while retaining pin/signature validation.
- **B-22.2** — Already fixed in Hosted’s route origin de-duplication per FIX-LOG; Hosted source is outside these worktrees and was not rebuilt here.
- **B-22.3** — Already fixed in maintained browser credential coordination: shared selected-store operations use the IndexedDB coordination path rather than trusting a tab-local fallback lock alone. Two-tab HTTP race acceptance remains unrun.
- **B-22.4** — Already fixed: shared refresh/storage restore retains a temporarily unreadable sign-in; permanent record damage has its own native code.
- **B-22.5** — Already fixed: unsigned reset hints cannot discard the selected credential family; retirement needs authorized evidence.
- **B-22.6** — Contradicted as a requirement to authenticate arbitrary future plain-HTTP sockets: HTTP from any address is explicitly permitted. Initial pin proof and recovery checks remain; subsequent HTTP traffic is not cryptographically bound to that proof. No socket-authentication claim or forced-TLS policy introduced.
- **B-22.7** — Already fixed: shared manual address candidates preserve explicit choices and apply Portico’s bare-local default port.
- **B-22.8** — Already fixed: ordinary HttpLocalApi operation deadlines, distinct from deliberate streams/long polls.
- **B-22.9** — Already fixed: PorticoServerListWatcher owns a bounded request and releases its shared flight on timeout.
- **B-22.10** — Already fixed: server-connections retains retry floors; session-refresh proactive/reactive paths share not-before deadlines and positive jitter.
- **B-22.11** — Already fixed: playback/watch paths use the shared HTTP-date-aware Retry-After parser.
- **B-22.12** — Already fixed: web account adapter preserves retry floors. Browser auxiliary adapters belong to the web/Hosted owners.
- **B-22.13** — Already fixed: SSE parser bounds aggregate frame data and handles complete frames/CRLF splits.
- **B-22.14** — Already fixed for the concrete application/problem+json case: gatewayFailure distinguishes Portico application/json from proxy JSON dialects. A proxy deliberately imitating application/json remains an ambiguous-provenance case; no source-only guarantee added.
- **B-22.15** — Fixed in source here on Apple/Android: ten read slots, reserved control dispatch/admission and end-to-end queued deadline. Apple uses separate read/control socket pools so HTTP connection admission cannot undo the reservation. Vega sensitive control is unavailable under the redirect safety gate, not claimed equivalent.
- **B-22.16** — Already fixed: signed nonce proof uses symmetric two-minute clock allowance after signature validation and a clocks-disagree outcome.
- **B-22.17** — Already fixed: native session subscribes to ready route origins and writes KnownDirectServers.setAddress independently from logical session ownership.
- **B-22.18** — Already fixed on Apple/Android: auxiliary native streams open through the pinned selected route. Vega credential streams now fail before SDK dispatch because of its redirect boundary.
- **B-22.19** — Security defect contained here: Vega rejects credential headers, credential-capability queries and sensitive bodies before constructing XHR; only tightly shaped public nonce proof POSTs are excepted. Credential-bearing streams are also refused. A redirect-controllable native Vega transport is NOT implemented; sign-in/restore/control functionality is unavailable until that exists.
- **B-22.20** — Already fixed and retained: Android mutation client disables retryOnConnectionFailure and request bodies are one-shot.
- **B-22.21** — Contradicted by current policy: required-HTTPS server middleware was removed; plain HTTP must remain accepted from every address. Secure aliases remain additional candidates, not a mandatory discovery bootstrap.
- **B-22.22** — Fixed here: bounded local promotion on foreground/request activity, at most once a minute, with no Hosted chatter or ready-screen blanking; four candidate bound and five-second lifetime.
- **B-22.23** — Already fixed: CORS exposes Report-Every-Ms.
- **B-22.24** — Already fixed structurally: server discoveryInterfaceSuffix gives each interface distinct names. Same-VLAN multi-interface mDNS conformance/device acceptance remains unrun; not a claim of complete DNS-SD conformance.
- **B-22.25** — Already fixed: event retry floor gets additional positive spread and long delays are chunked safely.
- **B-22.26** — Already fixed: native activationGate shares Hosted outage guidance while preserving valid codes/protocol intervals and explicit manual actions.

# API changes

No server routes, contracts or schema were changed. No legacy route was added. Shared Settings applicability changes expose existing fields on phone/TV. The internal `transport_unavailable` code has catalogue-backed, non-retry error presentation on every context. Runtime constants add Android `pictureInPictureSupported` and `television`; Apple’s existing PiP constant is now consumed. RouteConnection’s selected origin may promote from remote to proven LAN while its phase stays ready.

# Schema changes

None. No schema-sync was required.

# Owner or viewer settings added or wired

- `search.rememberHistory`: existing profile boolean, default true; now shown on native Privacy. Native Search already consumes it. No new storage/scope.
- `notifications.badges`: existing profile boolean, default true; now shown on native Appearance. Native Rail already consumes it. No new storage/scope.
- No new owner settings or manual device-layout setting added. Existing cardSizePercent/showBackdrops need frontend consumers, not removal.

# Needed from others

- **Vega native transport is a release blocker.** Replace SDK-controlled XHR for authenticated control and streaming with a real pre-follow-refusing native HTTP implementation (normal TLS validation, no cookies/cache, bounded bodies, cancellation, deadlines, no mutation replay). Then remove the temporary SDK safety refusal only from the replaced paths. Test 301/302/303/307/308 with bearer/password/refresh/profile-proof payloads against a second capture endpoint; the target must receive zero requests. Installed SDK currently supplies no supported pre-follow hook. Sensitive subtitle/media XHR paths also need the same boundary before claiming complete platform parity.
- `apps/native/src/screens/title/SavedPages.tsx` / PlaylistScreen, `src/app/container-play.ts`: server-owned occurrence anchor plus playlist revision, authorized source order, and queue selector consumer; do not substitute an index in the loaded page.
- `apps/native/src/screens/title/PlaybackChoices.tsx` and shared pending-track-choice/subtitle materialisation: show each unknown/SDH/forced track separately by stable source/resource/discovery identity and retain that exact selection through audio restarts. Existing subtitle catalogue resource IDs are title/source scoped; discoveries may still need materialisation before a session can choose them. Do not merge by language.
- `apps/native/src/screens/title/saved-hooks.ts` / useGatherView and SavedResourceScreen: server selector for a full saved query/view, with permission/revision fencing, so views larger than 500 do not imply whole-view playback of a prefix.
- `apps/native/src/ui/theme.ts`, title backdrops and shared Appearance rows: consume cardSizePercent/showBackdrops consistently; preserve platform capability hiding only where truly unsupported.
- Generic Android devices advertising no TV mode/Leanback need a deliberate layout-choice product flow; cannot infer the device class safely. TelevisionDisplay RN rotation metric overwrite/font-scale/internal-flag coupling remains for Android native owner.
- Native Search must decide whether to mount PorticoSearchKeyboard; Android window/IME integration must reconcile adjustResize with edge-to-edge rather than deleting the declaration alone.
- Web `WindowedGrid.update` compressed-end mapping; `Library.EngineResults` memoized cell callbacks; browser floor/target; forced-colour/font preference styling; rapid/same-name route announcements.
- Web event feed owner for plain-HTTP visible tabs; server/web CSP-compatible Radix lock; hook-rule tooling without an unapproved dependency; packaging Node version alignment.
- Web `LibraryViews.GenresGrid` and audiobook Series paging; `Entity` album grouping must use the already available `discNumber`; delegated admin screen gates; `saved/Resource` Undo; `detail/EpisodePanel` direct-visit facts and people links.
- `library-pins`/saved pins, provider API-key applicability, remembered-device action and registry-only `privacy.includeInWatchTogether` / `playback.startedThresholdPercent` need their owning screens/server decisions. Catalogue presence alone does not prove control reachability.
- Product contracts: NAV-6.7 must describe live filters; native architecture docs must describe Vega sandbox storage and the scene/storage/control queue changes. The programme’s immediate file deletion supersedes the older bulk-trash recovery assumption in V09.
- Release CI: macOS contract tests now have a job; real Apple archive/scene lifecycle and native transport tests remain needed. Vega SDK CI needs an environment with its installed private SDK; React hook tests still imitate a React internal seam and must not be presented as renderer tests.
- Minimal native-language edits made here: Native/PorticoPlayer.m (record dispatch, admission/socket pools, installation preparation); new Native/PorticoInstallationStorage.h; iOS/tvOS AppDelegate.swift/Info.plist (scene); Android PorticoRuntime.kt (facts), PorticoTransport.kt (dispatch/deadline), TelevisionDisplay.kt (same TV hint). Native review owners should reconcile these exact files.

# Test notes for other environments

Completed on Ubuntu with individual files/allowed checks:

- Shared `route-connection.test.ts`: 10 passed, including held remote route, LAN promotion, no Hosted request, failure cooldown disposal and foreground fencing.
- Vega `transport.test.ts`: 2 passed; password/refresh/bearer/capability-query/capability-path/custom proof headers are refused before any XHR construction; public nonce proof is strictly shaped.
- Vega `event-stream.test.ts`: 9 passed; bounded streaming tests plus refusal before a credential-bearing stream is dispatched. Public stream framing tests do not establish authenticated streaming support.
- Native `error-journal.test.ts`: 5 passed; redaction, bounds, fatal record serialization and report trimming.
- Native `errors-present.test.ts`: 4 passed; unavailable transport has catalogue copy and no retry in sign-in, playback and Hosted contexts.
- Native app and test TypeScript configurations passed; Vega app and test TypeScript configurations passed. Native string/raw-error/glossary lints passed (existing diagnostic-only warnings remain listed by the lint).
- Both scene plists parsed and their single-scene declaration checked with plistlib. This is not archive validation.
- New macOS CI compiles actual production URL allowlist, download-attempt guard and installation-cleanup helper. It was not run here. No Xcode, Gradle, Docker, full repo test or device build ran.

Apple acceptance (Mac with the named source tree, Pods and Xcode 27 SDK; iPhone, iPad and Apple TV):

1. Archive iOS/tvOS and install a release build. Cold launch normally and through each allowed portico item/play/account-return/device link. On iOS also cold/warm universal links, Spotlight/Handoff/Siri. Pass: one expected destination for the receiving viewer, no blank launch, no duplicate root. Rejected URLs must never navigate. Background URL-session completion must still call its handler.
2. Run the three `apple-contracts` CI commands. The installation test injects a locked Keychain, confirms no prepared marker/quarantine clearing on failure, exact-service cleanup on retry and no cleanup after the marker. Compile/run the new test with `clang -fobjc-arc -framework Foundation -framework Security`.
3. Sign in, update without removing the app, relaunch: sign-in retained. Remove/reinstall: no previous sign-in, installation ID or PIN vault released. Reboot and exercise background launch before first unlock: no wipe; after unlock retry restores the established marker/state. While already prepared, lock the device during background playback: journal/control access retains ordinary after-first-unlock behaviour. Failure: automatic erasure of an established installation or old credentials released after reinstall.
4. Hold a PIN hash/Keychain operation while sending a public API read/control. Pass: transport submission continues; storage remains serial and unreadable storage never repairs itself opportunistically.
5. Point read requests at a delaying test endpoint; occupy ten read tasks and at least 56 waiting reads, then issue a timeline/renew/end write. Pass: control has reserved dispatch/socket capacity; further reads hit the read admission bound; all queued operations settle within their original deadline; cancellation/reload releases once. Test duplicate identifiers and old callbacks after reusing an identifier, especially across the two NSURLSession pools. No redirected credential request may be followed.
6. Visit all roots, push several pages, switch away/back while playing. Pass: Back/scroll/content retained, hidden roots do not render each context update, focus restores correctly, playback continues. Repeated long history is still retained; measure memory separately rather than claiming a bounded stack.

Android/Fire acceptance (API 24+, Google phone/tablet/TV and actual Fire OS):

- Use the same read-burst/control and deadline/cancel/reload checks. Confirm mutation bodies never silently replay after a response-reset fixture and redirects never follow.
- Fire flavor or FEATURE_LEANBACK with non-TV UI mode must get TV layout, focus ring and the same design density. Phone/tablet without those facts stays touch. Rotate a TV/projector or change font scale and explicitly measure RN’s density-reset gap; it is not fixed here.
- PiP offered only on a non-TV API-26+ device that declares FEATURE_PICTURE_IN_PICTURE; unavailable models must hide it. Confirm Apple actual PiP support agrees with its existing runtime flag.

Vega acceptance:

- Current source must refuse password/refresh/profile/bearer control and authenticated streams before constructing the SDK request, showing the unavailable-device message rather than a retry promise. Do not release this as functional Vega sign-in parity.
- After a real native transport replaces the refusal, repeat redirect-capture tests for all redirect statuses and every credential form; assert zero second-endpoint requests, correct TLS validation and bounded/cancellable request/stream lifetimes.
- Resume frozen roots and test focus/navigation and playback on the actual Vega renderer; TypeScript/shared React shape alone does not establish Suspense/native-stack integration.

LAN promotion acceptance: connect via a remote route while LAN is down, restore LAN without a device path event, then foreground or make an ordinary API request. Pass: current screen stays ready, a pinned proof selects LAN within the bounded check, old late failures cannot replace it, no Hosted polling occurs. Leave LAN down and repeatedly interact: no more than one promotion check per minute.

# Not finished

- **A redirect-controllable native Vega HTTP/stream transport is not implemented. The safety containment leaves Vega authenticated operation unavailable.**
- Occurrence/revision playlist anchor, exact pre-play subtitle identity/materialisation, complete saved-view queue selector and the frontend/packaging handoffs above.
- Android rotation density and unadvertised generic-box layout policy; retained-history memory is not bounded; React-internal hook harness and Vega SDK CI gaps.
- Apple archive/store/scene launch, first-unlock/reinstall/background, native admission under actual socket load, TV focus and all physical device acceptance remain unrun. The installation-storage test source exists, but only macOS CI/device execution can validate the native environment.
- No promise that all 116 findings are fixed: source-resolved cases, deliberate trade-offs and remaining gaps are separated above.

Final cumulative snapshots: `_program/snapshots/leftovers-clientcore/002` and `_program/snapshots/leftovers-native/002`. Milestone 001 snapshots are retained too. Shared packages checked identical to the canonical server worktree. Both git diff --check checks passed. No source-tree changes followed the final snapshots.
