# Handoff draft lane

## Outcome

Completed `Project Architecture/Audits/2026-10-11-comparative-audit/IMPLEMENTATION-REPORT-2026-10-14.md` in the named internal worktree. Main reading path is approximately 6,150 words (about half an hour); appendices cover all 73 lane reports, changed settings, components/licences, every open bug, all 588 original IDs and the remaining product choices with alternatives.

The document distinguishes last fully tested bases, later merged work, ongoing-lane fixes and all absent platform acceptance. The main finding is that the programme has delivered substantial server/shared-client source but not release parity; Vega authenticated transport is unavailable, frontend/server gaps remain, Apple native code is uncompiled, and Hosted cutover needs separate fresh state plus a final toolkit resync.

Execution host: ehler-homelab-mini-1 (personal Ubuntu). No application code, original checkout, Git metadata, deployments or publishing changed. No helper/subagent, suite, application build or browser/device interaction used in this lane. Justin writing style skill and canonical project/verification guidance read. Source inventory was captured once in /tmp/handoff-evidence to avoid chasing continuously changing reports; hashes below record exactly what was read. Later final reports need a new owner reconciliation.

## Decisions I made

- Keep this a dated owner draft, not a release acceptance or promise to incorporate reports after the capture. Latest fully tested server is ce34473; subsequent late-server merge 189b32f was observed. Native 2617834 and internal 416708d were inspected clean. No full-suite result is attributed to the newer server head.
- Main report follows the owner's replies by topic, with a short decision reading path and further choices/alternatives in Appendix E. All implementation choices are attributed to lane evidence rather than recast as owner approvals.
- Use second-review severity from the 588-row register. Preserve a separate explicit disposition for the old ledger's eight first-pass High labels. Do not hide deliberate trade-offs inside the fixed count.
- Partly delivered client contracts/screens, unqualified release/test coverage and unresolved risks stay open. Source-resolved implementation is not downgraded solely because physical acceptance has not occurred; the evidence level is carried separately.
- Prioritize remaining-bug owners over unrelated “not fixed here” comments. Explicitly adjudicate compound statuses instead of trusting a substring match.
- Override a contradictory B-18.13 disposition: main-server SelectionBar invokes the shared preview/typed-confirmation dialog with a 200-title bound. Physical file deletion is not reported as a no-confirmation intentional flow.
- Keep B-02.16 open for owner failure-memory visibility/clear UI and B-08.12 open for response-composition cost despite successful partial server corrections.
- Do not copy the audit originals into the worktree: they are absent here, present read-only in the original internal checkout, and duplication was not requested. Report explains landing beside them later.
- Do not call all new findings fixed: completed review reports close 104 findings; provisional registry/web/native/security findings remain separate. Later decision pass's five corrections are identified as pending final integration/sync.
- Keep late report/source disagreements explicit. Decision-audit's D-53 says native library removal exists; LibrariesPanel only exposes folder removal in the inspected source. Do not infer library removal from removeSource. Hosted-followups' byte-level toolkit claim was true at an earlier checkpoint but the current comparison differs.

## Bugs

All 588 original register IDs are individually accounted for in the owner document's Appendix D: **362 reported fixed, 162 already fixed, 9 reported not present, 10 intentional, 45 open/partial**. Open reviewed severities: **2 High, 22 Medium, 21 Low**. Reported status is not independent proof of 588 outcomes.

High open: B-11.8 HDHomeRun guide/mapping owner flow (backend implemented, web setup missing); B-22.19 Vega pre-follow-refusing transport (disclosure contained, authenticated functionality unavailable). The previous coverage-open view had approximately 90 unresolved entries and first-pass severities; this lane rebuilt a current reading view rather than editing that shared ledger.

Completed review inventory: identity 10, API 6, networking 10, accounts 5, playback core 7, downloads 6, Hosted 10, freshness 8, scan 8, pipeline 9, Live TV 12, ops 9, late server 4 = **104** (1 blocker, 32 high, 54 medium, 17 low). Hosted's tenth item is added coverage. Withdrawn/disproved freshness/scan suspicions are excluded. Apple/Android source reviews have different, non-comparable finding inventories and are not added to that total.

Provisional: registry 3 medium implemented/pending final receipt; security 4 medium/1 low recorded fixed but overall review pending; web 3 high/3 medium with five fix milestones and remaining Range-reader receipt; native JavaScript 4 high/6 medium/1 low with no final repair receipt at capture. Decision audit additionally reports five decision corrections. These need final integration and package sync.

## API changes

None by this lane. Report describes programme route/contracts and warns optional generated consumer protocols do not create production handlers. Source spot check confirms 622 production OpenAPI operations. No endpoint guarantee is inferred solely from generation.

## Schema changes

None by this lane. Source confirms only 0001_baseline.sql ships in the server migrations folder and Hosted CurrentHostedSchemaVersion is 21. Counts/cache/schema changes in ongoing lanes are reported as pending integration, never as tested deployed state.

## Owner or viewer settings added or wired

None by this lane. Owner document Appendix B lists programme controls with types/defaults, runtime effects, screen reachability and gaps. Actual subtitle setting key is subtitleFileLimitMiB; older quality/trash/TLS inventories were not propagated. Newer decision pass removes the accidental profile-wide PIN lockout, rather than asking the owner to approve it.

## Needed from others

- Orchestrator final reconciliation: incorporate final decision-audit, registry/security/web/native reviews, loopback runtime/browser/Hosted/scale results and all final test plans; update owner draft counts and source heads after merges.
- Complete final frontend parity and exact playlist/view/subtitle contracts, native lifecycle downloads, owner controls/freshness, supported accessibility/browser floor and real Vega transport. Exact grouped lists are in owner sections 4–5.
- Synchronize Hosted toolkit again after server generation/permission changes settle. Current source comparison: server 23 files, Hosted 20, five common-file differences plus three server-only files. Changed paths: generate.go, generate_test.go, http.go, http_test.go, registry.go, registry_test.go, templates/index.ts.txt, templates/route-builders.ts.txt. This lane did not change the copy.
- Finish staged application/native compilation and environment acceptance, licensing/source/relink materials/signing and fresh-database Hosted rehearsal. No deployment authority exercised by this lane.
- Land this draft alongside the original audit files. Preserve shared lane reports or archive them internally so relative links keep evidence available after worktrees move. test-plans/ is a pending sibling deliverable, not a verified directory in this worktree.

## Test notes for other environments

This lane ran document/source checks only: all 588 ledger IDs exactly once, total/count consistency, all 119 existing lane links resolve, main/appendix structure, no placeholder markers, credential-pattern/byline scans and final whitespace/readback. It did not run the programme's builds/tests again.

Source spot checks establish presence, not behavior. Detailed environment plans remain another lane's work. Owner report explicitly distinguishes every no-device/no-human-browser/no-Windows-or-macOS-runtime/no-GPU/no-tuner/no-Swift-or-Objective-C-compile limit, and lists pass/fail boundaries by environment.

### Claims independently checked in source

- **identity:** Missing-title defaults and restore-preserved credential policy read in schema/settings; bulk preview/typed-confirmation path read. Other title/asset/state outcomes not traced end to end.
- **api-platform:** Credential lifetime triggers and member-disable rule read in baseline; session-only member/remembered routes read. API keys/webhooks/MCP behavior and tests not exercised.
- **accounts-security:** Restore scrub statements read: integration family survives; renewal/TV/factor/Cast secrets are removed. No live restore or raw-byte scrub test rerun.
- **registry:** Generated production OpenAPI count is 622 on inspected base; not every binding/body/union/builder was rechecked.
- **networking:** CSP WebAssembly allowance read; source/value settings inspected. Listener/proxy/certificate tests not rerun.
- **ops-packaging:** Merged backup copy source inspected: two-minute snapshot, 64 MiB WAL growth, 128 MiB reserve. No OS installer/runtime/release test rerun.
- **hosted-followups:** Own/main-internal format-21 refusal code read; whole toolkit byte comparison now fails (five common differences, three server-only files). Earlier identical claim is dated, not current.
- **web-subtitles:** Pinned provenance/license/COPYRIGHT files read: 4.2.4 and FriBidi LGPL-2.1-or-later. No engine execution, real corpus or final distribution acceptance.
- **apple-mpv:** SPM project wiring/pin read. No Apple compile or native execution.
- **android-mpv:** Gradle release pin read. Binary recipe/licence conclusions remain attributed to reports; no native build or ABI checks rerun here.
- **leftovers-native:** Both app scene/ATS plist declarations and Vega pre-dispatch refusal source read. Native lifecycle/installation/transport execution not performed.
- **review-server-late:** Late three fix commits/merge observed on main-server; no independent regression run or full-base revalidation.

Additional document checks: generated OpenAPI contains 622 operations; plain HTTP/scene keys exist in iOS/tvOS/Top Shelf plists; Vega refuses protected exchanges before constructing the request; missing retention defaults are 30 days; key-guard/epoch/role/disable triggers exist; member deletion/remembered-account metadata is session-only; wrapper provenance explicitly records FriBidi LGPL. These statements are source presence only.

### Every other claim not independently verified

All reported build/test totals and successes (including 85/4 Go packages, 1848/512/20 shared/web/Cast, 514/64 native/Vega, Android four-ABI linkage, PostgreSQL flows, generated-media decodes, watcher/runtime/NSIS fixtures, review regressions and scale timings) were read from reports/log inventories, not executed or reproduced by this lane. No complete per-test log audit, remote status audit, native binary inspection, human/browser/device/provider/tuner/GPU/OS/store/licence acceptance or real deployment was performed. “Nothing pushed/deployed/merged into Justin's own branches” is programme-log testimony; inspected worktree branches/heads alone cannot prove historical remote activity.

The following is the exhaustive claim-family inventory for all source reports used. Every behavior/test/acceptance assertion within each family's Outcome, Decisions, Settings, Needed/Not finished and appended fix receipts remains attributed to that report, except the narrow source-presence checks explicitly above. This is intentionally more conservative than presenting lane authors' assertions as this lane's verification.

- **accounts-security:** Unified new-sign-in admission, live permission changes, Cast-only authority, member erasure and restore scrubbing. Account/profile/TV policies were repaired; native management screens, residual restriction differences and historical Apple sign-out acceptance remain.
- **android-mpv:** Added Android/Android TV/Fire TV mpv module, relay and JNI wiring with declared codec/output limits. JavaScript/profile checks passed; a full application build and physical HDR/audio/subtitle/lifecycle validation remain.
- **android-native-review:** Repaired download ownership/integrity, callbacks, relay lifecycle, captions and engine teardown. Standalone Kotlin/Java and four-ABI JNI linkage/checksums passed. No Gradle packaging, APK or device run.
- **api-platform:** Implemented keys, tokens, scopes/budgets, title query, changed-path scanning, signed webhooks and MCP. Integrations survive password/session changes and restore. Follow-ups and reviews supersede its early missing-screen and missing-service lists.
- **apple-mpv:** Added separate mpv engine and system-validated relay for Apple platforms, plus engine choices and declared limits. Apple TV output is SDR. Swift source was never compiled or exercised.
- **apple-mpv-review:** Repaired render-surface/audio lifetime, seek ordering, bounded fonts/HLS relay and capability declarations. JavaScript/project parsing checks passed; Swift tests and native linking have not run.
- **apple-native-review:** Repaired AVPlayer validation, callback queues, downloads, captions, audio facts and Top Shelf isolation/privacy. Project/plist/type checks passed. Its initial scene/reinstall gaps were later addressed in source by leftovers-native, with no Apple compile.
- **audio-engines:** Implemented continuous web resampling, bounded deep-seek fallback, runtime decoder probes, Android gain and Apple AAC trim/rewind/fallback source. Narrow tests passed; Apple limited AAC target and real HE-AAC/device output qualification remain.
- **base-health:** Combined early lanes, repaired cross-lane identity/revision/restriction/units/intake failures and ran full server/shared/web/Cast suites. All requested checks passed except known vendored vet warnings. Later merges required the second pass.
- **base-health-2:** Repaired later retention/auth/PIN/request/recording/playback collisions. Final report records 85 Go packages, 1,848 shared tests, 512 web, 20 Cast, four apikit packages and cross/dev builds passing, with vendored vet warnings retained.
- **console:** Three web passes delivered integrations, metadata/attention, titles/files, account/devices, networking, operations, scan issues, resource readback and title-page Undo. Custom owner pages remain web-only; its final unfinished list drives the remaining frontend work.
- **conversion-outputs:** Added qualified rich picture/audio outputs, fMP4 publication, HDR/tone-map/burn-in/font paths, hardware fallback and bounded diagnostics/working space. Ordinary planner wiring came later. GPU/native output and richer prepared recipes remain unproved/incomplete.
- **decision-audit:** 31-bullet/61-decision trace and five corrections; matrix delivery/consumer claims are source-report evidence; D-53 native remove-library claim conflicts with inspected folder-only UI.
- **docs-internal:** Updated 41 architecture/contract/spec/backlog/decision documents. Separates requirements, reported source and unverified acceptance; preserves decision history and explicitly records Vega transport and Hosted fresh-database consequences. Later changes still need reconciliation.
- **downloads:** Implemented durable device receipts, resumable originals/conversions, offline context, renewal and play history. Its appended merge carries styled/image originals and fonts; later follow-ups preserve chosen version and merged claims. Platform recovery/clock/signed-out player gaps remain.
- **e2e-hosted:** Runtime work is in progress on disposable loopback Hosted/server state. Reports fresh-format initialization, real password/claim, directory, first-party, app PKCE and TV flows passing; rotation/outage work and final evidence remain unfinished, with no production deployment.
- **e2e-server:** Real loopback server scenarios with disposable state and generated media are underway. A final runtime result is not available; the passing base suite must not be substituted for this end-to-end report.
- **e2e-web:** Production build/headless Chrome work is underway against a disposable loopback server. Final runtime and visual issue list was not available. Automation does not establish a human browser acceptance walk.
- **freshness-clients:** Added living viewer-scoped document hub, quiet reads, event sharing and delayed connectivity errors. Tests use simulated transports/locks. Some pages/deep-list refresh and HTTP tab sharing remain incomplete; native cards do not fade in.
- **freshness-server:** Added retained browse generations/two FTS slots, separate revision domains, private coalesced feed, failure isolation and bounded reclamation. Client/console consumers came later. No target-scale throughput or physical network acceptance claimed.
- **grouping-sorts:** Added common scoped title vocabulary, indexed scalar/profile orders, exact paged aggregates and capability contracts. Filtered aggregate sorting/deep direct ranges retain disclosed cost gaps; existing legacy envelope screens need migration.
- **hosted:** Added no-registration S256-PKCE app consent returning the ordinary server-bound assertion, signer/revocation/outage fixes and fresh-only format. Review/follow-ups supersede format-20/toolkit/provenance gaps. No existing service was changed.
- **hosted-followups:** Synchronized scoped toolkit/99 JSON routes, durable root-certificate check-in and transaction-time consent revocation; improved consent focus/direction/retry behavior. Format 21 requires separate empty state. Fleet/provider/deployment acceptance and local oversized-root hardening remain.
- **identity:** Separated titles/assets/versions, retained state, missing-title rules, real file deletion and background library removal. Later review fixed confinement/asset/approved-set issues; later intake/wiring delivered editions/parts. General audio/container merge and large transaction behavior remain limited.
- **integrator:** Combined overlapping server/native work, restored compatible schema/settings/units and package copies, and exposed hidden integration defects. Its early grouping/title-order question was subsequently overruled by the wide subgroup sort decision.
- **leftovers-native:** Disposed 116 register entries and repaired Apple scenes/reinstall, native control reserve/runtime facts/frozen pages, settings reachability and LAN promotion. Vega disclosure was contained by refusing authenticated operation. Exact playlist/subtitle/view selectors and frontend gaps remain.
- **leftovers-server-a:** Disposed 27 entries: anime orders/offsets, fresh episode reuse, account-management boundaries, legacy admission removal, failure-memory APIs, tolerant local XML, preview rules, backup bounds and watchdog. Historical Apple incident and owner screens remain.
- **leftovers-server-b:** Disposed 32 entries: internal error mapping, early EOF/credits fallback, bounded backup/fan-out, offline future stamps, baseline consolidation and release-input/vulnerability guards. Native clock/recovery/CI, advisory resolution and Apple rights remain open.
- **limits-settings:** Wired runtime controls and still-watching, added owner analysis/subtitle limits, consolidated background pace/pause, introduced corrupt-state recovery and source-consumer guards. Structural witnesses do not prove device effects; its early quality inventory predates final consolidation.
- **livetv:** Implemented shared acquisition/private readers, tolerant streamed/spooled guides, import controls, shared private DVR bytes and recovery facts. Backend tests passed; owner guide/countdown consumers, Windows custody, real tuners and load acceptance remain.
- **music-audiobooks:** Added bounded broad audio-tag reader, lyrics/art/chapter/grouping, natural part order, comfort rewind, queue shuffle semantics and durable audio facts. Own-renderer/native follow-ups came later. Online book provider and physical/sample-exact codec coverage remain absent.
- **naming:** Added bounded pure naming/evidence/ignore reader and hundreds of corpus tests without a dependency. Scan-loop later wired configuration and identity/evidence into production. Disc playback is deliberately unsupported; hostile bounds remain.
- **native-app:** Repaired viewer-scope lifetime, virtualized Saved/History, links, setup/account UX and profile-aware accessibility. Later lanes canonicalized staged strings. Saved views still cap playback at 500 and exact playlist/subtitle selectors need contracts.
- **native-health:** Made refreshed native/Vega packages and existing browse controls pass: 514 native/64 Vega tests, app/test type checks and lints. Added bounded silent 428 recovery. These are JavaScript checks, not native application builds or rendering evidence.
- **native-playback:** Moved native video onto shared session controller, in-place choices, capability publication, Automatic measurement, local extras and caption/display source wiring. SDK/device behavior remains unproved; setting-change/session and exact subtitle-selector gaps remain.
- **native-sync:** Mirrored packages, canonicalized staged strings, joined playback settings/version picker, HTTP/ATS/address memory, approval/outage handling, handoff/report/privacy and optional sign-out removal. Its join appendix supersedes early type/string/version gaps; custom owner parity remains missing.
- **networking:** Unified locality/proxy handling, HTTP/TLS/IPv6 listener, external routes, own-certificate reload, discovery topology and console reserve. Independent review repaired h2/trust replay/policy refresh defects. Real proxy/multicast/container/device behavior remains unqualified.
- **ops-packaging:** Added restart parent, detached restore, SCM/macOS/Linux/Docker recipes, alternate storage, notices/reports, signing inventory and reclamation. Fixture/cross checks passed. No installer/reboot/signing/notarization/Docker/GPU acceptance was performed.
- **personal-state:** Implemented bounded atomic container marks/Undo, completion/history/privacy ledgers, Home destinations and corrected Continue behavior. Download/quiet-counter integration came later. Physical EOF/offline/large-container and complete UI acceptance remain.
- **planner-wiring:** Connected converter qualification/target persistence to ordinary/live/offline planning, signal-preserving choices and executable fMP4 tests. Live publisher remains TS; optimized downloads remain H.264 SDR stereo; mastering-display/MaxCLL facts are not captured.
- **playback-core:** Created one capability document, native/mpv choice, Original/fixed/Automatic policy and generation-fenced session changes, remembered tracks and failure memory. Producer/client/review lanes supersede its early missing-output/menu notes. Real hardware claims remain untested.
- **providers-matching:** Added verified fuzzy/yearless matching, attention actions, provider-specific pace, locale/credential/art/NFO policy and refresh. Scan-loop wired sidecars; leftovers completed anime/episode-cache cases. Fixtures are not live provider acceptance.
- **registry:** Declared all retained production API handlers, adopted generated path/response/body helpers and removed duplicate YAML specs. Base-health-2 records 622 operations and separate optional/development contracts. Later registry review identified destination/schema declaration defects, with fixes pending final receipt.
- **review-accounts-security:** Found and fixed five findings: revoked-signer descendants/pending paths, transaction-time device authority, restore integrity/retry and waiting-device admission. Regressions passed. Original residual client/privacy/incidents are not certified closed.
- **review-api-platform:** Found six findings, later fixed in appended phase: pre-admission SQL, cursor cost, webhook fairness/order, MCP gap and use accounting. Its initial findings-only “unfixed” text is historical; appended fixes and base integration supersede it.
- **review-downloads:** Found and fixed six access/replay/activity/address/metadata-publication defects while preserving standalone copies. Focused repository tests passed; native storage/restart/expiry/signed-out operation remains separate acceptance.
- **review-freshness:** Found eight confirmed findings and fixed them, including event replay gaps, dismissal/history/search hints, counters, Unicode predicates and diagnostic wire time. One suspicion was withdrawn. Large-capacity and missing screen consumers remain separate.
- **review-hosted:** Reported ten closed findings including real-server challenge mismatch, cookie lookup, check-in/signing availability and safe consent/callback behavior. One item added coverage. Isolated PostgreSQL checks passed; browser/device/fleet/cutover acceptance remains unperformed.
- **review-identity:** Found ten findings and later repaired cross-disk asset identity, symlink-safe deletion, approved-set consistency, show/provider/tag/retention identity and offline missing protection. Regression tests passed; device/file-system and large-transaction acceptance remains.
- **review-livetv:** Found and fixed 12 parser/acquisition/capture/authorization/recovery/control defects. In particular shared owners survive cancellation/finalization and restricted metadata stays hidden. No physical tuner/NAS/GPU or complete saturation acceptance.
- **review-native-js:** Ongoing source review has recorded 11 findings: four High, six Medium and one Low across offline acknowledgements/enrollment, playback intent, Vega seeks/streams, trace redaction and hook tests. No final closed-fix receipt yet; supersedes any implication native tests establish completeness.
- **review-networking:** Found and fixed ten h2/trust replay/reserve/origin/scoped-address/policy/publication/status defects. Initial findings-only text is preserved historically; the appended implementation receipt is current. Actual supported topologies/capacity remain unqualified.
- **review-ops:** Found and fixed nine restore/copy/scrub/report/retention/supervisor defects, with focused regressions. Native power-loss/service/release acceptance and rights/signing work remain. No original database or deployment was touched.
- **review-playback-core:** Found and fixed seven transactional/authorization/ceiling/locality/control-context defects. Refused changes preserve working playback; source visibility is rechecked. Physical throughput/player behavior and the distinct subtitle mutation boundary remain unproved.
- **review-playback-pipeline:** Found and fixed nine geometry/copy/colour/header/fMP4/audio-clock/signalling/orphan-walk defects. Real generated conversion checks complement source assertions; physical GPU/HDR and confinement behavior still require acceptance.
- **review-registry:** Ongoing review reports three Medium fixes implemented: safe route parameters, accurate analysis/lyric/status contracts and real nullable-union types/decoders. Targeted checks reported; final receipt/integration was not complete at this draft checkpoint.
- **review-scan:** Confirmed eight findings and fixed them, including unsafe weak audio identity, stale revival/removals, unreadable subtrees, bounded absence checks and extra/artwork evidence. One suspicion disproved. Native watchers/NAS/provider/large-load acceptance remains.
- **review-server-late:** Latest report declares four Medium defects fixed: playlist request identity, exact-version retry, deletion fact reconciliation and restricted DVR picker continuation. Targeted regressions reported; final snapshot/integration and ordinal-cursor boundary follow-up still need final receipt.
- **review-web-logic:** Ongoing review recorded six findings: three High and three Medium. Five have appended fix milestones for subtitle budgets, controller lifetime, settings safety, feed resync and memory ordering; music Range response bound and final integration remain open at checkpoint.
- **scale-check:** 10k fixture and 10k/200k/2M index-only timing/count repairs are provisional reported measurements, not reproduced or full mixed-library capacity.
- **scan-loop:** Wired relative naming/tags/IDs/editions/parts/extras/discs, ignore/evidence lifecycle, tolerant pages, native watchers, change-driven limited scans and bounded retention. Linux watcher exercised; macOS cgo and real platform mounts remain uncompiled/unqualified.
- **search-sort:** Added natural/Unicode sorting, richer indexed search/CJK groups, aggregate paging, integer 0–10 ratings and general millisecond/UTC boundaries. Common grouping controls came later; raw-title anchors/Saved search/specialized time shapes remain incomplete.
- **security-sweep:** 622-row permission matrix and four medium/one low fixes are provisional report evidence; final review and integration incomplete.
- **server-docs:** Updated 34 public documents and two guides, installation/API/media/dependency notices and executable examples. Later registry removal supersedes its kept-YAML note. Source-backed instructions are not proven deployment, artifact licence or signing acceptance.
- **server-followups:** Delivered exact attention/artwork/resource status, one-version preview/delete, download identity pinning, source pickers, owner transport controls and role freshness. Web third-pass consumers followed; later review caught request/deletion/picker edge cases.
- **social:** Repaired room readiness/revisions/presence/privacy and Cast recovery/takeover; exposed handoff control through the shared API. Some adapters still ignore paused start or need event/storage wiring; no Cast/native synchronization device run.
- **subtitles-text:** Delivered lazy track discoveries, tolerant legacy text/cues, raw originals/fonts, provider login and preview/marker/conditional-response fixes. Later limits expand inventory/file defaults. Renderer/burn/offline integration and real corpus/provider acceptance remain separate.
- **test-plans:** Fifteen environment plans plus index are being assembled; eight installation/recovery plans and subsequent browser/GPU/network drafts reported. Remaining native/Vega/tuner/Hosted/integration coverage and final snapshot were unfinished. No plan is an executed acceptance result.
- **vega:** Implemented shared engine contract, live sequence buffering, Automatic facts, captions/capability/remote shims and bridge tests. Later transport safety containment overrides its parity claim: authenticated operation unavailable. SDR/stereo and unsupported features remain explicit.
- **web-app:** Implemented bounded tolerant presentation parsing, per-tab memory, search privacy/history, stable reconnect/page state, role confirmation and non-player UX fixes. Security identities/receipts remain strict. Later hub/console work supersedes its early missing-field list.
- **web-player:** Replaced old adapter with shared controller and persistent video element; repaired session/timeline/lease/queue/quality/failure/keyboard/seek behavior. Later subtitle/audio lanes fill parts of its gap list. Actual hls.js/element/media-key behavior remains unqualified.
- **web-subtitles:** Added vendored browser libass, own PGS/VobSub/DVB workers, fallback fonts/position, 3x audiobook/up-next and native-HLS policy. Merger wired raw plans; late web review repairs combined-memory budgets. Real corpus/browser/PiP/cast/font coverage remains incomplete.

Specific unresolved verification/consistency questions carried forward: actual source ordering/cost for every grouping; full legacy-envelope migration; specialized time boundaries; exact native remove-library parity; offline companions/fonts through local MPVKit/Android launch; sustained rewatch/early unknown-duration EOF; 5000-member writer latency; original-file mutation/digest and disk custody; high-load reserve/event/writer recovery; shared-capture physical retirement; root-mounted/network filesystem identity; Apple signed scene/reinstall/Keychain background behavior; real browser WASM/bitmap composition and captions; configured provider responses; exact HDR/mastering/audio output; release artifact dependency/licence/signature proof; fleet revocation/cutover/rollback and toolkit parity. No conclusion beyond reports/source presence is claimed.

## Not finished

No requested handoff-draft implementation remains. The requested owner draft and cumulative snapshots are complete. The programme itself is unfinished; all known pending source/UI/platform/release work and unverified claims are documented. This draft cannot include final results that other active lanes have not delivered. The named sibling test-plans directory does not yet exist in this worktree; its draft report explicitly remains in progress.

Context-sync failed at task start because /home/justin/.local/state/justin-agent-context/sync.lock is read-only. Existing guidance was read directly; the updated user-provided AGENTS revision 903b1fbea30aec5db763f43747827250650b340b was incorporated without restarting the task.

### Snapshots and evidence

- Snapshot 001: Draft the owner implementation summary and remaining work.
- Snapshot 002: Add the lane evidence settings licences and complete bug dispositions.
- Final snapshot 003: Finish the owner handoff draft with checked evidence and open work. Cumulative snapshots supersede preceding snapshots; do not sum patches. No product-tree change will follow the final snapshot.

### Captured source-report inventory

Names, last-modified UTC and SHA-256 below identify all 72 external reports at the read checkpoint. /tmp/handoff-evidence is scratch, not a durable deliverable; the source hashes and dispositions are in this report. The owner draft is the sole product-tree artifact.

| Report | Captured mtime UTC | SHA-256 |
|---|---|---|
| accounts-security.md | 2026-10-13T06:42:33.122099+00:00 | 591860712c5f486672331963b8e8be098d744c5940a2fa36830c3d11336c93aa |
| android-mpv.md | 2026-10-13T06:15:48.198479+00:00 | 50ccd71dfd624c328ee83518a8cba26204f8602c4b68b2c9e62136b07da0fab2 |
| android-native-review.md | 2026-10-13T13:19:29.296337+00:00 | 795ab6b7692a6ca93c908c3de67c9a50f11265ff93708ecd77a502fa25681b98 |
| api-platform.md | 2026-10-13T05:07:40.679669+00:00 | c37629a8cb2cf31c440ea30209dd99d091420ae741f33b5f17805b1aba9cb7e1 |
| apple-mpv-review.md | 2026-10-13T05:57:28.099290+00:00 | b54a4db55a54adcaaca477bb1c14c50e67a5091fc3424a6d2bb85792082f7a32 |
| apple-mpv.md | 2026-10-13T04:46:06.369489+00:00 | 30fbfe546afb64ab1eac79cd74d25de74f0dffceb8c3f20b0728da007f6378bf |
| apple-native-review.md | 2026-10-13T13:05:38.854950+00:00 | 77f7cbff9246989b638cb20b91adb5f2b73926029a5fc18b5fffb0386aa12fba |
| audio-engines.md | 2026-10-13T05:43:38.088317+00:00 | ecf11d1ef755df23fc76fc0f620842c7af2172042b8b5314c16896460990b8c5 |
| base-health-2.md | 2026-10-13T15:38:30.742323+00:00 | b597968e3778112088aff736ffbdea3ffaa81b6cded556f3fff07e848f91a139 |
| base-health.md | 2026-10-13T12:22:13.343933+00:00 | 92774fb5a7e11546354456de490378ff844c3748695dbec78261928eea8464d4 |
| console.md | 2026-10-13T10:57:12.061619+00:00 | e1aa9e5f1cbf98fe2d3f29558475135152ba5b3af6fb1dbeebcf663a37d91506 |
| conversion-outputs.md | 2026-10-13T05:58:30.707169+00:00 | b1df0cd7abec53f9938850d28264bcfffac869f2aceb637de7c229633255de6e |
| decision-audit.md | 2026-10-13T16:01:24.252688+00:00 | 348e18e2f6ae23848301188bc3ed65fcbc09f5c7681e257dbfd8e0352eb0c5c0 |
| docs-internal.md | 2026-10-13T12:59:29.005901+00:00 | 21db7495aff8776d15f838ca397f863275cf77745ea948cc15533f70d84873fb |
| downloads.md | 2026-10-13T06:30:48.611118+00:00 | 26a728d9b087c5af303cec8bcdb62cf287073bd90037db0acca217a65f7da45e |
| e2e-hosted.md | 2026-10-13T16:02:04.959522+00:00 | 49ef6d562d4d2d1c4167cff3fe5d2d93524bf550e870192d7f160660074798fc |
| e2e-server.md | 2026-10-13T16:00:51.525471+00:00 | 265e8c8a9b2f0e36e4b6d10cde3f1b19e201740e24fbb978427cf8989299bc8b |
| e2e-web.md | 2026-10-13T15:56:37.374604+00:00 | 8822dc9f68198dfcc70250e48edab4a76c698b28baef28440846c8c87f3ac517 |
| freshness-clients.md | 2026-10-13T09:34:21.206446+00:00 | c3fc70b2b3b882de070e7c2af7cb3c7d10ed5689f8843f16a444cad677c38b37 |
| freshness-server.md | 2026-10-13T08:02:38.496344+00:00 | ad1d3a4ebf4cc1c9625c394bc7e9da6425b2710de072611df5eb907bc6e49e3c |
| grouping-sorts.md | 2026-10-13T13:35:57.705472+00:00 | 075ac840646bd4193e20f2ef12b0ba7e40cf4ce9821f1f46a5e023e2a24e27b2 |
| hosted-followups.md | 2026-10-13T12:50:39.005072+00:00 | a643da04ce766f8388a0294481a00bc977f4f1f63c6b8bdc217b03b1ba629c88 |
| hosted.md | 2026-10-13T04:58:10.800086+00:00 | 8b4a2f46994b56b2d5059ac2d8e63c356a720807e903d8e96d2f217f28ed4a3e |
| identity.md | 2026-10-13T05:06:10.941180+00:00 | 42c6606eef99acf055564bbb729aba50cf67ae63e42ad785b18e15f0cca3f6a9 |
| integrator.md | 2026-10-13T10:44:54.353313+00:00 | 00fc78b2f681fd9fbd4857ed3e87ab7b985addba44d1d2c4a583591bd39d6076 |
| leftovers-native.md | 2026-10-13T12:59:53.927053+00:00 | 251f21a065be94070a4285514bbae381cb2a89dd38276b3a2aaa9ffa7b8c8050 |
| leftovers-server-a.md | 2026-10-13T13:22:32.282308+00:00 | 6d5206af506ebd8720ff167b5ddaa1e5144423cef750dc6c41992aacc9f535b4 |
| leftovers-server-b.md | 2026-10-13T13:12:07.922105+00:00 | bb1107c9a4843356946692d219f88e990bb03422c19224a944ba0e9470b3859a |
| limits-settings.md | 2026-10-13T07:46:17.755566+00:00 | 19715d7a5a857d853120a305d2e9f2515f8ae3e84c1231c7fa80ccfd89c8253d |
| livetv.md | 2026-10-13T06:19:16.906940+00:00 | 30eccf036c5bac31517e858bc3e9a50bf860ee053942a22cb490e71142f26f46 |
| music-audiobooks.md | 2026-10-13T04:44:59.138229+00:00 | 0a828c579dfe46af7afc7c663b097ff765fbddd77807c0ca9a3460d9bd4289aa |
| naming.md | 2026-10-13T04:12:38.095548+00:00 | 010631b783484a5ada19d8a22a3c12013cc36a46e1fc1c69e129ce0d8f68fbd5 |
| native-app.md | 2026-10-13T04:42:55.681956+00:00 | a9ada4a75ea86ab3dde4865bd9d7d5c21001a1a6351a588515afcafec126b5cd |
| native-health.md | 2026-10-13T15:11:57.244984+00:00 | 6036161a237c960ddd7a4cd3dc2ec0d2dc6c63056bbe37a1379a32f2257e59bc |
| native-playback.md | 2026-10-13T07:18:36.050959+00:00 | f674ec9d6374599248d69c951e9342c0c9ee0740767e6c4e256410b423a19f7d |
| native-sync.md | 2026-10-13T07:37:45.207656+00:00 | 4782ea379301cad0c4ec4d1877bc82a1deabae3f3d0cbdb5ba9de2094e7e887e |
| networking.md | 2026-10-13T04:37:06.512334+00:00 | 1f3b57c15adff1ff1d28f8cfdf1cd8def0e1a704db5268bb242380272dd9ab09 |
| ops-packaging.md | 2026-10-13T05:38:35.473070+00:00 | c3c014571fb82cf3b741d2c5e5c143a66281b8b83bf5369b68bd8419f518f2b4 |
| personal-state.md | 2026-10-13T07:51:35.911230+00:00 | 6b7633cb428e16d5ed40087c0ac1aa68fa1684013daf97d13f054e1e3d14bea2 |
| planner-wiring.md | 2026-10-13T07:50:05.276697+00:00 | c5cddbec08340049478d0c05d67f9e56a3ef05e77939e3a00bcbcc3d348e32a7 |
| playback-core.md | 2026-10-13T06:20:41.945036+00:00 | 2f087e3a12569905f13477b8f2f6be0b5ab998ba715c7593a7c23cd199a22198 |
| providers-matching.md | 2026-10-13T05:01:08.895816+00:00 | 7e3a73589d3606e035a49e3782cd70cae57dd509d50a1528ad91ae500a1bdc0d |
| registry.md | 2026-10-13T14:42:09.883159+00:00 | c0966b5f149f58a984e730de64256f2df4731a362caaf4f5ef802bd8110682a1 |
| review-accounts-security.md | 2026-10-13T07:56:34.902465+00:00 | 2660937a43b2ea4beaa83976f01aed9228997163ee07133b011b93c5f3fa1adc |
| review-api-platform.md | 2026-10-13T06:21:11.227233+00:00 | e6e5136decf9730d219ca47df25a61a9329c22fb9efc77cdc85ae7737f917b97 |
| review-downloads.md | 2026-10-13T08:06:20.704311+00:00 | c3ebe335e696fd280a7b62a4d2ed668d5d2ed13e10be798502ba1209e5f4e67a |
| review-freshness.md | 2026-10-13T13:02:25.955786+00:00 | 11f2e25bb1f5ce630b2f439e03f13ddfc204f3ee150ac5ee4f406ccd55a0cdaa |
| review-hosted.md | 2026-10-13T07:49:55.999477+00:00 | 72ad837a4700932fc70a76c173c548209ef717499e3b809dff9e47f82ed7b767 |
| review-identity.md | 2026-10-13T06:46:38.464980+00:00 | 323a6bc430e93f53fe0594a4fca1ee53500c282b9a6e069b4cf17b57e18d57c6 |
| review-livetv.md | 2026-10-13T13:16:27.675972+00:00 | c4c0cf18dbfcb7409260ca976279955ebd3372c80d203b0ad0cece3b278444be |
| review-native-js.md | 2026-10-13T16:02:15.756863+00:00 | 48e10b532a83b9ca8a5ba9fb42a72679ecc681051358307b097cf88375b5b13c |
| review-networking.md | 2026-10-13T07:40:25.822292+00:00 | a1ce703b15785f6a6582e54c4412f7f96008e3510902dd3c2561dc9a439a776c |
| review-ops.md | 2026-10-13T13:11:24.845702+00:00 | 09a9d8d159b81c7ee4e048b4af9e48c76ff654a946d6b7ff050e92d76cf6f2dd |
| review-playback-core.md | 2026-10-13T07:49:26.738833+00:00 | a93d176e378e4962275b054c60e86c013ed5482bc82db9128a999cb232dc449a |
| review-playback-pipeline.md | 2026-10-13T13:19:11.690556+00:00 | 6c6e89e24c6eaa10570bd1139c8f7efcf6ba9eaa19daca539f434e1f193d06ef |
| review-registry.md | 2026-10-13T15:59:00.416301+00:00 | 733bed9c6c085d8e94e340defdf6704eb17860a66a8c571b434b9e8bc081f12c |
| review-scan.md | 2026-10-13T13:16:06.123159+00:00 | adaf49fbf7846835423b89753d873cb19cb938c0d0c2032fc47ab20f3be9497c |
| review-server-late.md | 2026-10-13T16:03:10.675792+00:00 | 7409d689959ef9deaace624881aa8563acafba9e19c8a46af5fd9127fc262518 |
| review-web-logic.md | 2026-10-13T16:00:18.396348+00:00 | 454786a09d2d37059bf92a555c3824fde303e66b78708ae2f508c04d874137ef |
| scale-check.md | 2026-10-13T16:03:07.746489+00:00 | 96a65e0e2632ab6441a4fc853e1b87cb6c7685a03db0c60ddbed0a614783fb98 |
| scan-loop.md | 2026-10-13T07:46:49.175246+00:00 | a8b58858a9134db20a562b13cd28697112bbb535ba4154899493ecae55258839 |
| search-sort.md | 2026-10-13T07:19:23.303818+00:00 | 71894f5159ee2f1ee91afe0b053a8625db19c7c792118bef3c67c75e419e5469 |
| security-sweep.md | 2026-10-13T16:03:39.807491+00:00 | 9f042ab895b94e4ba930394d9e45fbfe16e48f4ca5a33ef8ce431367d2f1c8e9 |
| server-docs.md | 2026-10-13T12:59:07.843255+00:00 | 02a9f47d96a4709ba2aaa4783c57913c2f509f086133964b2b93de1f2f9b5aa6 |
| server-followups.md | 2026-10-13T08:30:25.801744+00:00 | 51d9295ff06e8d7f35aa64a3804cdf85d818c1d9246b7e26d74811bae2239b46 |
| social.md | 2026-10-13T05:42:15.189098+00:00 | 8c79c6c96956bee4da46e96378976a2cf0a157ceb12b59b7feafb00bfdcb6d26 |
| subtitles-text.md | 2026-10-13T07:09:23.781802+00:00 | 9763d19a0e4e1b67d0a08405f33385abee23a719486187ec662e16cce2d4c969 |
| test-plans.md | 2026-10-13T15:57:42.339845+00:00 | 5fbd97fe55d8574f275f1dfb4268b6946181892a80e41f1cfd75908262f19386 |
| vega.md | 2026-10-13T08:26:34.843907+00:00 | cb547c7a627079b9235577990b148da5b5612a714a0f7ae519b4cff19adf6c3f |
| web-app.md | 2026-10-13T05:18:07.739522+00:00 | 1ae3eb74496d0b92ba223822015acda5d35b89eb1c3eead00c3681eefe340287 |
| web-player.md | 2026-10-13T04:54:02.794490+00:00 | a2fa66e3b85e4e10fc514edd013d5131ed6bd8c6c5b7b97184fcf2ef39312523 |
| web-subtitles.md | 2026-10-13T06:31:06.193885+00:00 | 3ff98f83140536567c309c5628c013d5d8e518a45055ffddbb037b428c0698cc |

Owner draft SHA-256 before final snapshot: 150dc6c75a8a95779542540b7eb02184ae701b0da14c4c250f47193f92ee0a1b
