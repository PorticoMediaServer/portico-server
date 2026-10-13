# Backlog

October implementation reconciliation. Report inventory read at 2026-10-13 12:56 UTC. Reports are live: later appended outcomes supersede initial handoffs. Items below are remaining implementation or acceptance work, not a new feature proposal. Each item links its source report. Delivered work is removed from the old 2 October list. No deployment state is inferred.

## Catalogue, intake and metadata

- **identity:** Clean hidden removed-library derived rows; bound large show merges; resolve untagged replacement identity, album/artist/book identity merge and retained per-play/playlist state. Absorbed IDs have no redirect. Edition/name/ignore wiring is now supplied by scan-loop; do not reopen that old handoff. Source: [identity](../../_program/reports/identity.md).

- **review-identity:** Resolve R1–R10 source findings and retain regressions; qualify best-playable version selection, multipart edition grouping and all absorbed playback/social references. Later server-followups moves download references, but does not certify all references or large staged merges. Source: [review-identity](../../_program/reports/review-identity.md).

- **naming:** Pure name reader is complete; native/filesystem runtime qualification remains. End-to-end naming/ignore/sidecar wiring from its original gap list is supplied by scan-loop. Source: [naming](../../_program/reports/naming.md).

- **scan-loop:** Qualify Windows/macOS watchers (including cgo FSEvents), network/FUSE remounts, provider first-import performance and hostile-input setup fixture; integrate diagnostics/message provenance and change-feed reset consumers. Later console supplies monitoring/skip UI, so that screen handoff is no longer listed as absent. Source: [scan-loop](../../_program/reports/scan-loop.md).

- **providers-matching:** Real-provider and physical-client acceptance; wider person portrait identity and local-artwork selected-original retention policy. Scanner sidecar reobservation is supplied by scan-loop. No remote audiobook provider or source-write export is implied. Source: [providers-matching](../../_program/reports/providers-matching.md).

- **music-audiobooks:** Full background decode remains; no general packets-first measurement. No generic manual album merge, ambiguous compilation detection, acoustic lyric identity or online audiobook enrichment. Real tag/lyric/less-common-codec/gapless and suspended playback acceptance is required. WEBP/gain/seek follow-ups now exist in audio-engines. Source: [music-audiobooks](../../_program/reports/music-audiobooks.md).

- **search-sort:** Complete specialised telemetry/subtitle/social/prepared time fields, Saved-resource collection search, explicit/article sort-head reanchoring and client Genres/Series windowing. Ratings and basic aggregate paging have landed; do not reopen them. Source: [search-sort](../../_program/reports/search-sort.md).

- **grouping-sorts:** - Focused verification, generated contracts and snapshots pending. - Arbitrary filtered/restricted group count sorts calculate exact counts in SQL rather than from a query-specific maintained order; unrestricted groups use maintained indexes. - Direct ranges on keys without position blocks walk bounded keyset chunks; cursor pages seek directly. - Dedicated detail/list/search envelope routes retain their existing default/curated orders. New controls must query their advertised common browse surface. This is part of the client handoff, not a claim of screen completion. Source: [grouping-sorts](../../_program/reports/grouping-sorts.md).

- **review-scan:** Review and fixes are in progress. Source: [review-scan](../../_program/reports/review-scan.md).

## Playback, conversion and subtitles

- **playback-core:** Owner route-failure inspection/reset and forget-track choice; complete per-engine display/rate facts. Native/web session integration, Automatic and rich ordinary targets are supplied by later lanes; real device/codec/HDR/receiver qualification remains. Source: [playback-core](../../_program/reports/playback-core.md).

- **review-playback-core:** No independent seven-finding fix remains. Qualify physical playback/throughput; original owner failure controls remain open. Failure memory follows a sign-in family, not a proved device-wide record. Source: [review-playback-core](../../_program/reports/review-playback-core.md).

- **conversion-outputs:** GPU/confinement/native OS/HDR/font/receiver qualification; qualified DV reshaping/base limits and E-AC-3 7.1 constraints. Rich ordinary target wiring is supplied by planner-wiring; rich prepared/experimental finite paths remain separate. Source: [conversion-outputs](../../_program/reports/conversion-outputs.md).

- **planner-wiring:** Live fMP4 init/media publisher and full Automatic/mpv presentation; rich HEVC/HDR/multichannel prepared recipe identity/storage/packaging; source mastering-display/MaxCLL capture, audio-depth declarations, obsolete web approximate-DV prompt removal and offline original integration. Source: [planner-wiring](../../_program/reports/planner-wiring.md).

- **native-playback:** Apple/Android SDK/device builds; native start-throughput and reason parsing; same-session Player-setting capability replan; fallback-font mixed-script coverage, Settings caption rows and chapter thumbnails. Recheck stale mirror failures after native-sync; no React-hook coverage or runtime proof is claimed. Source: [native-playback](../../_program/reports/native-playback.md).

- **apple-mpv:** Real MPVKit/SDK/linkage, display/HDR/receiver, subtitle, seek and lifecycle qualification; software limits and early EOF. PiP/live/music mpv remain unavailable. Read later apple-mpv-review fixes before reopening corrected source defects. Source: [apple-mpv](../../_program/reports/apple-mpv.md).

- **apple-mpv-review:** Apple framework compilation, pinned MPVKit ABI/symbols, physical teardown/seek/relay/track/display acceptance. Source review fixes do not prove HDR output, audio routing or all glyphs. Source: [apple-mpv-review](../../_program/reports/apple-mpv-review.md).

- **android-mpv:** Build Gradle/JNI/libmpv and test physical Android/Fire devices. No mpv passthrough, interlace or DV5; no mpv PiP/live/music. HDR with drawn subtitles uses SDR; HDR output, software ceilings/fonts and Fire receiver reporting remain unqualified. Android 11 frame-rate API excludes older Fire OS. Add release notices/source custody. Source: [android-mpv](../../_program/reports/android-mpv.md).

- **web-player:** Real hls.js/element/live/native-HLS behaviour, automatic direct-file measurement, live session close, Safari converted AirPlay, output latency/media keys, localisation and obsolete DV pre-play read. Later server/web subtitle/audio work supersedes old Original/default-ceiling/3×/FLAC gaps; unindexed lossy deep seeks use ordinary fallback. Source: [web-player](../../_program/reports/web-player.md).

- **web-subtitles:** Real browser/libass canvas/WASM worker/policy and real bitmap fixtures; broader fonts (including Korean), DVB character/progressive forms and VobSub CHG_COLCON; forced-only control and PiP/Cast subtitle fallback; Safari converted AirPlay, up-next artwork/countdown and remaining literals. Original parser/device-beside-stream merges must be qualified. Source: [web-subtitles](../../_program/reports/web-subtitles.md).

- **subtitles-text:** Real DVD/DVB/timing/glyph/HDR/confined/native/provider acceptance; complete offline originals/companions/fonts and consistent font/geometry handling. Existing 32 MiB bitmap/32-track bounds remain. No cross-episode intro detector is implemented. Source: [subtitles-text](../../_program/reports/subtitles-text.md).

- **audio-engines:** Apple SDK and actual LC/HE-AAC/priming/gapless/deep-seek/output qualification; Android full controller/app/device build, Chrome/Firefox/Safari decoder/Media Session/background-key acceptance and real HE-AAC/multi-hour fixtures. Unindexed lossy web seeks deliberately lose optional effects in ordinary fallback. Source: [audio-engines](../../_program/reports/audio-engines.md).

- **review-playback-pipeline:** Review, fixes and verification are ongoing. No snapshots yet. Source: [review-playback-pipeline](../../_program/reports/review-playback-pipeline.md).

## Personal state and downloads

- **personal-state:** All-client container Undo and native typed See all/Next Up integration; premature EOF terminal evidence, physical offline journal upload and 5,000-member latency qualification. Web title-page Undo is later supplied by console; card-menu Undo remains absent. Source: [personal-state](../../_program/reports/personal-state.md).

- **downloads:** Carry styled/image originals, companions/fonts, chapter/trickplay images; consume offline subtitles/chapters/markers/audio labels and next-episode queue. iOS partials restart after changed address; Android dead-process restart and clock rollback remain open. Device settings are device-wide; transfer tend/recover paths need acceptance. Source: [downloads](../../_program/reports/downloads.md).

- **review-downloads:** Propagate canonical shared changes to native and qualify real receipt/access/transfer behaviour. Six independent server findings are fixed; original local player, iOS partial, next-episode/images and device gaps remain. Source: [review-downloads](../../_program/reports/review-downloads.md).

## Identity, API and networking

- **accounts-security:** Restricted profiles still manage devices/approve TV; prune failed-attempt ledger; remove residual Hosted authority branches; investigate historical Apple sign-outs; country/per-library unrated policy and label/tag parity; device-approval records without installation IDs. Client permission/hours/device handling needs acceptance. Keys restored from backup are an explicit disclosed exception, not an assumed scrub task. Source: [accounts-security](../../_program/reports/accounts-security.md).

- **review-accounts-security:** Independent five findings fixed. Reconcile all new route restriction/visibility matrices and client propagation/real proxy/Hosted/crash acceptance. Later base-health reports matrix fixes; verify against the final integrated revision rather than reopen that older count blindly. Source: [review-accounts-security](../../_program/reports/review-accounts-security.md).

- **api-platform:** Credential lookup admission, fair ordered webhook scheduling, title-query plans, MCP API-contract/default coverage and owner activity/other-member controls need review/follow-up. Generic legacy error mapping/304 composition and whole registry migration remain separate. Later console supplies credential/webhook screens; later server-followups supplies attention tools. Source: [api-platform](../../_program/reports/api-platform.md).

- **review-api-platform:** Six findings remained unfixed in this findings-only checkout: pre-admission lookup, title query, webhook scheduling/order, MCP defaults and last-used path. Later base-health repairs title sort seeks and server-followups supplies attention tools; reconcile remaining findings against final code. MCP mutation tools need contract-level tests. Source: [review-api-platform](../../_program/reports/review-api-platform.md).

- **registry:** Registry migration, generated-document guards, client service migration and inventory remain in progress. Source: [registry](../../_program/reports/registry.md).

- **networking:** Real Windows/macOS listening, scoped IPv6 URLs, discovery, DNS-rebinding/PNA, proxy/container/certificate behaviour and load/reserve acceptance. Follow-up review fixes supersede ten initial source findings. Deliberately open setup is not DNS-rebinding protected. Source: [networking](../../_program/reports/networking.md).

- **review-networking:** No F1–F10 source fixes remain. Qualify real OS/zone/multicast/proxy/container/capacity boundaries; retain deliberate open HTTP/setup policy. Connection-limit restart/effective/proxy replay/HTTP2/TLS publication now have regressions. Source: [review-networking](../../_program/reports/review-networking.md).

## Freshness, presentation and client parity

- **freshness-server:** Large-library/weak-hardware checkpoint latency, full failure-ledger paging/export and complete special-screen/event adoption. Generic 304 cost and other domains remain separate. Later console supplies the latest-failure screen. Source: [freshness-server](../../_program/reports/freshness-server.md).

- **freshness-clients:** Special Live TV/DVR/Downloads/social/owner panels need hub dependencies; recommendation/trending/popularity self-freshness and title-specific personal hints need server vocabulary. Deep appended rows/paged Saved views, HTTP cross-machine per-tab poll, native fade, Back caches, restart persistence and TV/browser handover/focus acceptance remain limited. Source: [freshness-clients](../../_program/reports/freshness-clients.md).

- **review-freshness:** No confirmed finding from this lane is left unfixed. External SQL guards and inherited client/operations/API boundaries above remain. Real-device, weak-hardware, large-library and physical crash/power-loss acceptance remain unverified; the reopen test is deterministic database/worker recovery, not a power-cut test. Snapshots 001 and 002 are cumulative compiling milestones. Final snapshot 003 follows final report/checks and captures the complete tree; no worktree edits follow it. Source: [review-freshness](../../_program/reports/review-freshness.md).

- **web-app:** Guide/linear tolerant parsers, neutral unknown owner activity, exact listening preference handling, reactive locale helper calls, skipped-part diagnostic presentation and unused-string review. Retain source-listed unresolved browser/navigation/input bugs; later Home fields/missing/disc/console follow-ups supersede their old absent-field notes. Real layout/CSP/reconnect/accessibility checks remain. Source: [web-app](../../_program/reports/web-app.md).

- **native-app:** Apple scene launch/build, platform input/accessibility and remaining B-19.* gaps, localisation, playlist occurrence anchors, subtitle pending choices, request classes, remembered secure hints, artwork retry, Saved view playback beyond 500 and mini-player error wording. Later native-sync supplies initial staged strings/address/library-memory work; verify remaining shared seams on final mirror. Source: [native-app](../../_program/reports/native-app.md).

- **native-sync:** Connect pending version before Play, player queue/title delete actions, paused first-frame Watch Together and destination session events; Play on skip/scrub, stable duplicate-name Cast proof and Android Cast module; running-session hours refusal. Staged report/download/activation strings/literals and actual TV/platform layouts remain. Owner tools remain web-first. Source: [native-sync](../../_program/reports/native-sync.md).

- **vega:** Physical Vega build/runtime/probes/live MSE/layout/focus; HTTP redirects are refused after follow; no qualified HDR/surround/display matching/caption OS style, gapless/crossfade/level/speed/discovery/downloads/Cast/home rows. Direct HTTPS throughput only reacts to stalls; non-sequence programme transitions can hitch. Hide unsupported capabilities. Source: [vega](../../_program/reports/vega.md).

## Live TV and social

- **livetv:** Capture-leader cancellation transfer, active guide extensions, standalone XMLTV-only setup/manual guide reuse/catch-up; large playlist streaming beyond bounded strings; full guide load/tuner recovery, Windows locks/retirement and real codec acceptance. Client reservation event/countdown, private recorded identity attach and per-source readback remain handoffs. Source: [livetv](../../_program/reports/livetv.md).

- **review-livetv:** Phase 1 reproductions, Phase 2 fixes, verification and snapshots remain. Source: [review-livetv](../../_program/reports/review-livetv.md).

- **social:** Paused/unpresented initial load, actual destination session-event control, native stable controller identity/storage, duplicate TV names, configured Cast application ID and narrow cleanup admission. B-13.14/.21/.22 follow-ups, SSE generator and physical sync/Cast/load acceptance remain. Source: [social](../../_program/reports/social.md).

## Operations, settings, Hosted and integration

- **ops-packaging:** Full recovery console, wider source logging, real SCM/launchd/Linux service lifecycle/Docker/GPU and release-runner/signing qualification. Independently approved public signing pin, macOS installer identity and final source/notice custody must be configured without inventing keys. Large backup WAL/restore/reclamation acceptance remains. Source: [ops-packaging](../../_program/reports/ops-packaging.md).

- **limits-settings:** Complete remaining deep-analysis progress-bound and visibility-worker pause boundary; real scan/conversion/connection/slow-NAS/tuner/GPU capacity. Later console supplies pause/usage controls; native effective-policy/device acceptance remains. Source: [limits-settings](../../_program/reports/limits-settings.md).

- **console:** Third pass still lacks Genres/Series windowing, owner Now Playing pause/resume/seek, remote scrub/seek, event freshness for special panels, card-menu Undo, per-source import limits, tuner person names, subtitle-provider credentials and web DV prompt removal. All three screen passes need actual browser click-through; broad native owner tools remain web-first. Source: [console](../../_program/reports/console.md).

- **hosted:** No-email/mailoutbox/onboarding reconciliation and deployment/restore notes; real provider/cookie/OS consent callback/Cast/ACME/DNS/proxy/fleet acceptance. Later review fixes the original ten Hosted defects; current server signingCertificate consumer is present, but full fleet revocation needs qualification. Current apikit sync follows the later follow-up, not the original claim. Source: [hosted](../../_program/reports/hosted.md).

- **review-hosted:** All ten independent findings are fixed. Real consent keyboard/double-click/browser/cookie/provider/app-scheme acceptance, replica revocation and actual release/restore qualification remain; source apikit drift reported later needs hosted-followups. Source: [review-hosted](../../_program/reports/review-hosted.md).

- **hosted-followups:** No known Hosted source implementation item remains from this lane. Local-server handling of externally root-issued revisions beyond its persistence range is the exact residual contract hardening noted above, outside this writable lane. External client shared-outage wiring, canonical toolkit publication, documentation reconciliation and real browser/provider/device/fleet/deploy/rollback acceptance remain. The consent label remains a non-unique display name by the recorded decision. Multi-replica address admission remains process-local; the account quota is database-wide. No full test suites, packaging pipelines or deployment were run. Source alignment and repository tests do not establish production fleet revocation latency. Source: [hosted-followups](../../_program/reports/hosted-followups.md).

- **integrator:** Physical Android controller resolution and real long-file FLAC seek; recheck stale client fixtures/restriction matrices/hand-written schema after later base-health. Integration test passes are not native/browser/device acceptance. Source: [integrator](../../_program/reports/integrator.md).

- **base-health:** Final source tests are reported, but actual native builds, browser/device/GPU/tuner/proxy/capacity acceptance and retirement of duplicate hand-written OpenAPI remain. Use this later report when judging stale fixture/matrix failures from earlier reports. Source: [base-health](../../_program/reports/base-health.md).

- **leftovers-server-a:** Current-code review, fixes, tests and snapshots remain in progress. Progress checkpoint: TMDB absolute anime now maps official and show-owned alternate groups, with an explicit signed numbering offset in the existing order field. Local absolute numbers remain unchanged; original seasonal coordinates remain provider provenance. Client-core supplies tmdbAbsoluteOrder. Episode cache reads now include region; expired held episodes use conditional requests where supported. Provider tests passed; the server compiled. Existing new-episode/freshness regression passed; additional publication-count/region regressions are being run. Progress checkpoint: primary-profile admission now guards account Devices, EditDevice, RememberBrowserAccount and ForgetBrowserAccount. Quick Connect keeps approver-role semantics. Retired hosted session issuance/restriction bypasses and viewer_only Further exact findings and acceptance steps remain in the report. Source: [leftovers-server-a](../../_program/reports/leftovers-server-a.md).

- **leftovers-server-b:** - Native first-frame pause, signed-out-file cleanup policy/integration, Android process-dead recovery and persisted monotonic receipt expiry (B-13.18, B-14.14, B-14.20 and device portion of B-14.21). - Native Apple/Vega CI/physical acceptance and cross-repository revision locks (B-25.10, B-25.21) cannot be edited in either writable worktree. - External architecture/product-contract wording and component-specific Apple licence decision (B-25.12 explicitly skipped). - OS signed-installer acceptance and the newly observed unfixed npm advisory (B-15.19). - Physical media/tuner/disk/installer acceptance listed above. None is implied by passing source/unit checks. Source: [leftovers-server-b](../../_program/reports/leftovers-server-b.md).

- **review-ops:** Review and fixes are in progress. Source: [review-ops](../../_program/reports/review-ops.md).

- **server-docs:** Public document review, edits, example checks, licence inventory and snapshots are in progress. Source: [server-docs](../../_program/reports/server-docs.md).

- **server-followups:** Generic TV/anime separate still requires explicit placement; multipart downloads still refuse several files; no merged-ID redirect or disconnected-receipt rewriting; music attention queue, aggregate vocabulary, per-source limit readback, tuner names and subtitle credentials remain. Actual remote playback/FFmpeg/tuner/native acceptance is open. Source: [server-followups](../../_program/reports/server-followups.md).

## Additional active reports

- **android-native-review:** Review in progress; no completion claim. Source: [android-native-review](../../_program/reports/android-native-review.md).

- **apple-native-review:** Source review and limited validation in progress. Source: [apple-native-review](../../_program/reports/apple-native-review.md).

- **leftovers-native:** - **A redirect-controllable native Vega HTTP/stream transport is not implemented. The safety containment leaves Vega authenticated operation unavailable.** - Occurrence/revision playlist anchor, exact pre-play subtitle identity/materialisation, complete saved-view queue selector and the frontend/packaging handoffs above. - Android rotation density and unadvertised generic-box layout policy; retained-history memory is not bounded; React-internal hook harness and Vega SDK CI gaps. - Apple archive/store/scene launch, first-unlock/reinstall/background, native admission under actual socket load, TV focus and all physical device acceptance remain unrun. The installation-storage test source exists, but only macOS CI/device execution can validate the native environment. - No promise that all 116 findings are Source: [leftovers-native](../../_program/reports/leftovers-native.md).

## Historical work needing revalidation

These old backlog items are not verified complete by the programme reports. They are retained as revalidation work, not assertions that a defect remains. The original 2 October Backlog is preserved in Git history; source below is the later report relevant to the area.

- Recommendation dataset manifest/public key publication, award/mood/music-radio extensions, pending Not interested versus Reset ordering and TV accessibility: [personal-state](../../_program/reports/personal-state.md), [freshness-clients](../../_program/reports/freshness-clients.md). No current dataset-release evidence was available.

- Local NFO person-to-provider portrait links, Wikipedia language, selected client fact fields, credit cap, unknown-artist album visibility, play-count song ordering and advanced per-show settings: [providers-matching](../../_program/reports/providers-matching.md), [search-sort](../../_program/reports/search-sort.md), [console](../../_program/reports/console.md). Do not reopen old strict naming, provider-chain, NFO crew, version or file-rescan defects merely from the historical list.

- Actual Hosted backup units/restore drill, live infrastructure identifier rename, real Google/Apple onboarding and native Apple capability registration, fresh certificate issuance/renewal and real Cast application registration: [ops-packaging](../../_program/reports/ops-packaging.md), [hosted](../../_program/reports/hosted.md). No production access was performed. Historic email-delivery checks, force-publishing plans and Roku ports are removed from this programme.

## Outside this programme

Relay remains future work. Roku, CarPlay, Android Auto, light theme, plugins, themes, custom CSS and watch-state import/export are excluded. DLNA/desktop companion and other unapproved historical feature plans are not release completion gates. Existing design documents do not grant new scope.

## Documentation evidence still needed

- Revalidate specialised Library Channels builder/schedule/logo and channel-guide UI examples against the final registry/client revision. The specifications are retained as requirements and explicitly do not claim all-client implementation. Source: [docs-internal](../../_program/reports/docs-internal.md), [livetv](../../_program/reports/livetv.md), [native-sync](../../_program/reports/native-sync.md).
- Latest native follow-ups supersede earlier Apple scene/reinstall, request-class/privacy and Vega redirect descriptions. Vega authenticated operation is deliberately unavailable until a redirect-refusing transport is supplied. Source: [leftovers-native](../../_program/reports/leftovers-native.md).
- Latest Hosted format is 21, fresh-only; toolkit/check-in/replica consent fixes are supplied by the final follow-up. Earlier format-20 and copied-toolkit drift notes are historical. Source: [hosted-followups](../../_program/reports/hosted-followups.md).
