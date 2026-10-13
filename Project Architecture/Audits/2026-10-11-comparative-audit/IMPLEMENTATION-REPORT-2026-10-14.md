# Portico implementation report — 14 October 2026

**Owner draft. Last fully tested server base `ce34473`; observed server head `189b32f`; native `2617834`; internal `416708d`.** Reports were captured once for this draft; later lane fixes are labelled as reported work awaiting final integration/verification. The first seven sections are the reading path; appendices hold the settings, lane records and bug reconciliation. This is a report of the implementation branches, with outstanding work carried forward. It is not release approval.

## 1. Summary

Portico has a much stronger server foundation and a substantially revised playback path. Titles now survive file replacement. Scanning and matching are more tolerant. Playback uses one capability document, chooses between the platform player and mpv, and can request richer conversions. The local API now has working keys, personal tokens, signed webhooks, MCP and generated route contracts. These are meaningful changes to the product you asked for.

**It is not ready to release.** The combined server tests pass. Native and Vega JavaScript tests pass. That does not establish device playback, platform installation or visual quality. No physical device, human browser walk, Windows/macOS runtime, GPU or tuner acceptance has been performed for this programme. Swift and Objective-C have never been compiled. Android has narrow compiler and linker checks, but no complete application build. Further reviews, browser automation, local integration runs and scale measurements were still underway at this draft's checkpoint.

Six things matter most:

1. **The server changes are real, but complete client parity is not.** The web console gained three passes of screens. Phone and TV owner tools remain largely absent. Sort/filter controls, remaining quiet refresh consumers, native title actions and offline launch paths need the final frontend pass. Your rule that a feature is done on every first-party client has not yet been met.
2. **Vega is currently a release blocker.** A safety fix refuses credential-bearing requests before the SDK can follow a redirect. It closes the disclosure path by making sign-in, saved-session restore, authenticated control and streams unavailable. A native transport that can refuse redirects before sending credentials is still required. Passing 64 bridge tests does not make Vega functional.
3. **The largest playback bets remain unproved at their boundary.** Apple mpv, native audio changes, Android HDR switching, browser subtitle rendering and shared tuner/DVR behavior need actual platform tests. Apple TV mpv tone-maps to SDR. Vega currently declares stereo and SDR. Those are known capability limits, not test failures.
4. **The databases are deliberately incompatible with older pre-release builds.** The local server has one final baseline. Hosted requires a separate empty format-21 database. A Hosted cutover loses the old database's accounts, claims and directory state unless they are deliberately recreated. Preserve the old release and database; do not treat this as a binary-only upgrade.
5. **The reviews earned their keep.** The completed server/Hosted reviews report 104 findings corrected, including deletion escaping a library root, revoked signers admitting sessions, failed re-plans invalidating working playback and one person's recording cancellation harming another's. Late review lanes are still running. The original 588-entry register is a source-status inventory, not 588 reproduced runtime defects.
6. **There are product choices to sign off.** Keys survive password changes and restores; a revoked key can return with an older backup. Missing titles default to 30 days. An unproducible selected quality is refused. Browser libass contains LGPL FriBidi and needs a WebAssembly CSP allowance. Android mpv includes LGPL version-3 FFmpeg. Apple allows arbitrary HTTP loads. Section 3 spells out the alternatives.

My recommendation: finish the frontend and Vega transport work, take the late review and integration results, then compile the Apple and Android applications before any release decision. Run a private acceptance pass against disposable state and representative media. Keep the existing demo's conversion restriction. Only then land the branches and consider a separate Hosted cutover. There is no reason to redo the architecture now; the remaining work is integration, a few real server gaps and proving the experience.

## 2. What was built against your decisions

The order follows your replies: library tolerance and identity; playback; openness and networking; freshness and owner control; complete media scope and parity. Where an early lane said something was missing and a later lane delivered it, the later evidence is used.

| Your decision | What exists on the branches | Clients and limits | How far checked |
|---|---|---|---|
| Plex-style titles separate from files | Stable title/file associations, versions, edition/part facts, provider merges, owner merge/split, immediate missing-file state, timed/never/immediate emptying, provider/tag-based retained personal state | Web title/file controls, missing cards, one-version deletion and remove-library flow. Native owner workflows are incomplete. Untagged audio replacement still relies on path; artists/albums/books lack general provider merge tools | Server regression tests and integrated suite; scheduler and physical disk behavior still require acceptance |
| Filename and embedded data win folder conflicts | Bounded naming reader, hundreds of naming cases, relative-path evidence, misspellings/yearless matches, episode zero/ranges/stacked parts, extras ownership, ignores, sidecar updates and tolerant subtree coverage | Server intake; web scan diagnostics and monitoring control. Disc directories appear once as unsupported logical titles; disc playback was not built | Naming corpus, Linux real watcher helper, targeted intake tests, non-cgo Windows/macOS compile checks; macOS FSEvents C bridge uncompiled |
| Owner picks provider per library, TMDB default/local-only | Confidence-based matching, three attention candidates, explicit accept/search/ID/dismiss, provider credentials, bounded provider-specific pace, richer NFO/art and explicit anime order/offsets | Web metadata/attention screens exist. Anime offset controls and native attention screens are still needed. No new online audiobook provider | Provider fixtures/race tests and server checks; no live-provider acceptance or first-import speed claim |
| Native primary; Automatic/Native/mpv | One stored capability document with per-engine facts. Native direct/remux wins; mpv can avoid video conversion, lossy audio or burn-in. Shared session controller replaces old bridges | iPhone/iPad/Apple TV and Android/Android TV/Fire TV source implementations. Vega has its own engine and no mpv. Engine-setting changes still restart a current queue entry in some native paths | Shared/native tests and type checks; Android standalone Kotlin/Java checks and JNI link checks for four ABIs; no Apple compile or full Android build |
| Original default; explicit Automatic and fixed qualities | Home/remote saved defaults are Original. Fixed values act as ceilings. Session PATCH changes quality/version/tracks without replacing the queue. Automatic starts at Original and requests down/up steps | Web and shared native playback consumers. Direct-file Automatic mostly reacts to stalls; no fragment throughput exists there. Settings are device-class scoped; player engine/overrides are installation-local | Planner/session regressions, web/native tests; no real connection adaptation acceptance |
| HEVC/HDR/multichannel conversion | Executable target records; qualified H.264/HEVC/available AV1 and AAC/AC-3/E-AC-3/Opus/FLAC paths; HDR10/HLG preservation or tone mapping; true copy/remux with conversion off; independent fMP4 outputs | Delivered only where the device/encoder/transport declares support. No-base Dolby Vision requires qualified reshaping. E-AC-3 encoding is limited to 5.1; 7.1 can copy or use another codec | Real generated H.264/HEVC fMP4 and restart/seek decode checks; GPU/output-display/native receiver acceptance absent |
| Styled/image subtitles on device where possible | Lazy discoveries, tolerant text encodings/cues, original ASS/PGS/VobSub/DVB plus fonts, browser libass worker and own bitmap decoders, native mpv rendering paths, high-quality burn-in | Desktop/iPad web overlays; iPhone web fullscreen uses system text or burn-in. mpv native paths. Drawn web subtitles do not follow PiP/casting; fonts/bitmap subsets still have gaps | Source, generated samples, parsers and worker tests. No real browser render or genuine DVD/DVB subtitle corpus acceptance |
| Full local openness, no plugins/Hosted dependency | Working owner keys/member tokens, four scopes and live permissions, isolated credential budgets, bounded title query, paths-changed, signed queued webhooks and built-in MCP | Web Integrations and member token screens. Local server owns these features. Native owner integration screens remain web-only. Some dormant clients still describe absent operations | Scope/registry/credential tests; generated contract spot check confirms 622 production operations. External integrations not exercised |
| No stable pre-release API; same API for first party | All production API bindings declared; generated OpenAPI and TypeScript builders adopted across shared clients; stale YAML specifications removed; separate development and optional consumer documents | Shared contracts copied into native, including the newly required contracts package. Optional consumer declarations do not create backend endpoints | Registry guards and combined suites. Separate domain retry/receipt conventions remain; there is no claim of one uniform envelope |
| HTTP anywhere; owner's home networks decide locality | Same HTTP/TLS listener, IPv4/IPv6, trusted proxy parsing, own certificates/reload, explicit external origins, TLS-only Hosted advertising, no overlay detection | Web/native address selection and quiet HTTP note. Apple ATS arbitrary loads enabled, including Top Shelf. Scoped IPv6 URLs and browser network policies require acceptance | Ubuntu listener/proxy/certificate regressions, Windows/macOS cross-compiles. No real owner proxy/overlay/container topology acceptance |
| Browse/search always answer; quiet updates | Retained relational browse generation, two FTS slots, bounded checkpoints, failed-title isolation; distinct command/semantic/progress counters; one audience-scoped event feed | Shared freshness hub; web/native Home/library/detail/show/Saved/search and selected console consumers. Remaining screens do not all subscribe; visible plain-HTTP tabs may each long-poll | Server/event/revision tests and simulated client hub checks. No capacity claim; protected new restriction classes may initially show safe empty content |
| Good defaults and meaningful controls; console reachable | Gentle/Normal/Fast pace, one-hour owner pause, owner resource limits, progress-aware acquisition, fixed console connection reserve, corrupt-state recovery page, setting-consumer guards | Web settings, diagnostics, storage status, scan issues, restart/restore/report controls. Some native controls exist as shared generic rows; custom owner pages do not | Focused saturation/recovery and setting tests; not proof against OS exhaustion or a hostile connection flood |
| Four server platforms and reversible operations | Restart parent, Windows SCM service, macOS system daemon/PKG path, Linux upgrade lifecycle, Docker/portable recipes, detached restore staging, bounded backups and reclamation | Server packages and docs; signing/custody inputs remain operator work. Separate cache/backup roots are startup choices | Linux build/runtime fixture checks, NSIS fixture compile and Windows/macOS cross-builds. No actual install/reboot/update/signing/notarization/Docker test |
| Film/TV/anime/music/books all at release | Video intake plus own broad audio-tag reader, album grouping, local/timed lyrics, chapters, bounded background audio facts, comfort rewind, accurate queue/shuffle operations and renderer fallback | Web/native music engines changed. Vega hides unsupported speed/gapless/effects. Apple capped audio target and a genuine HE-AAC corpus remain incomplete | Generated audio/demux/resampler/JVM tests; Apple audio source uncompiled; no physical gapless or output-route acceptance |
| One tuner input per channel; private DVR stored once | Shared acquisition/private readers, per-viewer conversions, tolerant streamed/spooled guide import, partial recording recovery and capture sharing with private hard-linked ownership | Backend and shared guide/DVR contracts; guide mapping/countdown/quiet-refresh consumers still needed. Live publisher is still MPEG-TS | Guide fixtures, sharing/recovery/race regressions and integrated Go suite. No real tuner, five-million-row guide or Windows custody acceptance |
| Downloads outlive server tidying with their context | Independent receipts, original in-place serving, resumable transfers, offline bundles, retained title/art/subtitle facts, offline activity history and current-access revalidation | Native Downloads and common player consume much of the bundle. Signed-out launcher paths still use restricted system players; player consumption of standalone styled/image assets and chapters/trickplay coverage need checking | Server/shared/native regressions and narrow Kotlin checks. iOS changed transfer addresses can restart; Android process-dead recovery and trusted receipt clock unfinished |
| Watch Together/casting defect fixes; no redesign | Split room fences, five-second seek/load readiness wait, bounded follower correction, hidden-title redaction, independent Cast playback/reconnect, owner transport API | Shared/web/native adapters; owner Now Playing lacks pause/resume/seek UI, remote bar lacks seek, initial paused load may emit sound. Android Cast module remains absent | Protocol and follower simulations; no actual Cast receiver or synchronized device playback |
| Same sorting/filtering in every subgroup | Common scoped browse vocabulary across containers/people/lists/search, maintained scalar/profile keys, wide predicates; aggregate grids page beyond 200 | Native existing controls now intersect scoped advertisements. Full web/native screen adoption and aggregate paging remain unfinished; old detail/search/list envelopes retain narrow defaults | Scoped SQL/contract tests. Filtered group count/recent sorts can still aggregate/sort the full matching group set; deep ranges on new keys walk from the start |

Your exclusions stand: no plugins, themes, custom CSS, push notifications, email, light theme, state import/export, CarPlay, Android Auto, sub-path hosting or relay. Roku is outside this programme. No missing feature is made acceptable merely by type-checking its screen.

## 3. Choices made on your behalf

These are implementation choices to confirm or overrule. Some follow your direction closely; others change what a viewer will notice. The alternatives describe the cost of changing them. Every additional choice and its alternative is preserved in Appendix E. Defaults/limits are in Appendix B; lane decision records are indexed in Appendix A.

### Credentials, privacy and recovery

- **Keys and personal tokens have independent lifetimes.** Password changes, sign-out everywhere and ordinary device sign-out leave them alive. Current role/libraries/restrictions still apply; explicit revoke, expiry, disable/delete ends them. The alternative is to retire integrations with sessions and require reconfiguration. Password survival was also recorded as a direct instruction during the programme.
- **Restore signs people out but keeps keys.** Restoring an older backup can revive a key revoked after the backup, as well as old account/password/approval state. Review keys after restore. The alternative is to revoke every key during restore. That breaks integrations but gives a simpler recovery boundary.
- **Member deletion and remembered-account management refuse keys.** These remain session-authorized account operations even for an admin-scoped key. The alternative is a separately scoped automation capability with explicit destructive/account authority; it was not added.
- **Deleting people keeps owner history with names blanked.** Profile and device names are also blanked. Deleted personal history is distinct from the owner's retained record. The alternative is removing owner evidence; that would reverse the owner-history position.
- **Downloads stay at sign-out unless removal is chosen.** Other profiles can reclaim their occupied space without seeing their titles. Device-wide download settings remain device-wide. The alternative is automatic account-file removal. Native cleanup acceptance is still required.
- **Hosted format 21 needs an empty database.** Use a separate database and retain the prior release/database for rollback. Recreate accounts, claims and directory/provider/DNS state before cutover. The alternative is a deliberate migration project; this build has no upgrade bridge.
- **Public-client consent needs no app registration.** It displays an unverified app name and exact callback, delays decision buttons one second, uses S256 PKCE and returns only a server-bound identity assertion. Cancel navigates to the displayed destination without a code. The account label is a non-unique display name. The alternative is registered apps, a stronger identity label or a different sign-in UX.

### Library and personal state

- **Missing titles stay 30 days by default.** The owner can choose immediate, another interval or never. Missing-file marks are immediate; unreachable roots are protected. A previously populated root that appears wholly empty is protected on ordinary scans; an explicit verified removal can settle its last file. The alternative is a longer safety default or retaining everything until manual emptying.
- **Unmatched show identity uses its exact source-relative folder.** Separate folders wait for provider/owner merge. Yearless and year-bearing films wait for accepted identity. Provider merge keeps one ID and adds no redirect for the absorbed ID. The alternative is aggressive name-only merging or maintained redirects.
- **Matching accepts at 0.78 with a 0.07 identity margin; below 0.90 stays in attention.** Validated IDs and owner choices win. US certifications/en-US are default video policy; unknown field locales stay unknown. The alternative is different acceptance/attention thresholds or locale defaults after real corpus testing.

### Playback, rendering and licensing

- **A quality the server cannot produce is refused.** It does not quietly serve Original above the selected ceiling. Clients should offer Original. The alternative is an explicit fallback with a visible actual-quality result.
- **Apple uses MPVKit `1.1.0-n9.0.2`, LGPL product.** Metal/MoltenVK remains the output path. Apple TV mpv is SDR, with no PiP, AirPlay video, live channels, music or bitstream passthrough. iPhone/iPad HDR declarations remain unproved. The alternative is a qualified patched output build, including its distribution materials.
- **Android uses media-kit libmpv `v1.1.11`, full flavour.** Its FFmpeg is LGPL version 3, linked with mbedTLS. It is older and lacks passthrough, deinterlacing and profile-5 reshaping. An own build without mbedTLS could change licence/components and add missing features. That was not built.
- **Browser ASS uses vendored `@jellyfin/libass-wasm` `4.2.4`.** It includes FriBidi under LGPL-2.1-or-later; notices, pins and provenance are retained. CSP allows `wasm-unsafe-eval` to compile WebAssembly. The alternative is an independently built/reviewed bundle or dropping browser libass. Distribution compliance is still an artifact review, not established here.
- **Apple App Transport Security arbitrary loads are enabled.** This is needed for owner-named public HTTP servers; system TLS validation still applies to TLS. The extension follows the same HTTP choice. The alternative is narrowing HTTP and reversing the product decision. Store justification remains necessary.
- **Vega declares SDR and stereo.** SDK “smooth” is ignored; reported decoder sizes drive capability limits. No output-route/display fact can substantiate HDR/surround. The alternative is qualified SDK facts or explicit viewer overrides. The current safe transport refusal takes precedence over earlier parity claims.

### Operations, integrations and shared media

- **DVR is private per profile but stores shared bytes once.** Sharing requires the same airing/channel/source; independent hard links preserve deletion privacy. Cancellation/recovery was strengthened by review. Active guide extensions are not dynamically followed. The alternative is separate captures or a new active-window policy.
- **The console connection reserve is fixed at 32.** Owner media/keys do not borrow it. Connection overrides require restart and are resource limits, not viewer capacity promises. Home-network lists replace defaults; an empty list means all remote. The alternative is a different reserve/default policy after load testing.
- **Windows runs as LocalService with service SID; macOS as `_portico`.** Authenticated UNC access needs an owner service account. Cache and ordinary backups may move; rollback/pre-upgrade copies remain beside the live database. The alternative is different service/storage ownership, with installation consequences.
- **Manual backups stay until deleted; scheduled keep count defaults to seven.** Spring-forward schedules use the first real minute. Backup pinning has time/WAL/disk bounds and aborts under pressure. The source check finds a two-minute snapshot, 64 MiB additional WAL and 128 MiB disk reserve. Larger-data acceptance is still required; earlier five-minute/128-MiB-WAL report values are superseded.
- **Background pause lasts one hour.** Work finishes its current quantum; playback, DVR, auth and essential cleanup stay active. Lower limits apply to new work without evicting existing inventory. The alternative is indefinite pause or immediate cancellation.


## 4. What is not finished

### Screens and client consumers

This consolidates later report handoffs, including work missed by early console summaries. A service/parser being present is not a completed screen.

- **Browse:** full sort/filter controls in every genre, series, category, collection, person, search and list scope; migrate old envelope lists to common scoped browse; follow every aggregate cursor beyond 200; consume authoritative sort heads when raw titles differ; finish compressed-scroll mapping, memoized cells and deep-list refresh/Back behavior.
- **Web management:** owner Now Playing pause/resume/seek; seek on remote controls; truthful delegated-admin gates; HDHomeRun guide URL/mapping; reservation countdown; per-source import-limit readback, tuner person names, subtitle provider credentials; owner failure-memory list/clear; anime alternate-order offset controls; exact edition/version labels.
- **Freshness:** Live TV/guide, DVR, Downloads, Watch Together and the remaining keys/tokens/webhooks/devices/reports/storage pages; plain-HTTP visible-tab sharing; native card fade-in that preserves TV focus; removal of rare stale-list notices where unnecessary. Appended deep pages can retain removed cards until reopening.
- **Personal actions:** Undo from card menus and saved-resource entry removal; direct episode air dates and guest/director/writer links; web album grouping using `discNumber`; native Play next/Add to queue/Delete; secondary-profile capability gates and outside-hours handling during an active session.
- **Native owner parity:** member deletion/rating-system picker, tokens/webhooks/MCP, metadata/attention, merge/split/remove-library, restart/restore progress/report reading and detailed storage/scan/transport controls remain largely web-only. Generic shared rows do not close these gaps. Technical impossibility has not been established for these pages.
- **Native playlists/views/subtitles:** occurrence-and-revision queue anchors for duplicate playlist entries; whole saved-view playback beyond 500; exact pre-play subtitle source/resource/discovery identity, including unknown language/SDH/forced alternatives; remembered-choice clear controls; chapter thumbnails; appearance card-size/backdrop consumers; keyboard/IME integration and an explicit layout choice for generic Android boxes without TV facts.
- **Offline:** signed-out Apple/Android launch must use the common capable player, retain receipt authorization and expose stored extras. Styled/image originals and fonts are already carried by the appended download implementation. Finish/verify their player consumption, chapter/trickplay images and next-episode queues where missing. Verify optional per-account sign-out cleanup and device-wide settings disclosure.
- **Together/Cast:** guaranteed paused initial engine start, live duration/seeking/buffering facts, session-change receiver consumption and stable duplicate-TV proof identity. Android has no Cast module. Real remote controls need actual receiving players to honor them.
- **Accessibility/wording/build:** browser floor/target, forced-colour and font scaling, rapid/same-name announcements, focus/remote/VoiceOver/TalkBack, CSP-compatible dialog scroll lock, hook tooling, residual player/metadata English, diagnostic skipped-part counts and Node build-version alignment. Keep real owner choices wired rather than deleting them.

### Server and shared-contract work

- Final late-lane integration includes the native/web/registry/security findings above. Do not retain stale package copies after controller, route-builder, schema or permission fixes. The captured decision pass also removes an unintended profile-wide PIN lockout and makes Automatic start at Original even when throughput is supplied; these corrections must land before claiming those decisions are met.
- Hosted toolkit sync has drifted again: a direct byte comparison finds five changed common files and three files present only in the server copy. Final resync/generated-contract verification is required; the earlier byte-for-byte report was a dated checkpoint.
- Apple bitrate-limited listening still lacks the supported AAC converted-target recipe/selection required by the own audio renderer. The ordinary video/audio converter supporting AAC does not itself close this listening-target gap.
- Filtered/restricted aggregate count/recent sorting still calculates all matching group keys. New sort shapes lack maintained direct-position blocks; deep ranges walk from the beginning. Recommendation “For you” remains a bounded candidate path. Finish or measure these before claiming strict large-library behavior.
- Rich prepared/optimized download recipes remain H.264 SDR/stereo AAC; HEVC/HDR/multichannel offline conversions need recipe identity/target storage/packaging. Mastering-display/MaxCLL facts are not captured, so rich ordinary HDR does not promise every mastering field. Live Automatic/mpv selection is not fully implemented.
- Uniform specialized telemetry/subtitle/social/probe/analysis time fields are not fully reconciled. General catalogue/offline fields use milliseconds and UTC instants; do not infer every internal/specialized protocol was converted.
- Playlist occurrence selectors, full saved-query queues and exact discovered-subtitle materialization need server/shared contracts before honest client controls can ship. Global Saved-resource collection search and general artist/album/book merge are absent.
- Profile label blocks and account label/tag denials still differ. Per-library block-unrated and fully country-specific rating tables were not built. Failed-attempt retention/device records from clients without installation IDs still need bounded lifecycle review.
- Local handling of externally root-issued certificate revisions above SQLite's persistence range remains a specific hardening gap. Normal Hosted artifacts are now bounded to JavaScript-safe integers. It is not a normal rotation failure.
- Some dormant client protocols have generated optional declarations but no backend handlers. They must be implemented or retired when product scope is settled; generation is not server support.
- Large show merges/container marks still have bounded but potentially long writer holds. Catalogue failure diagnostics expose only the latest 100. Artwork originals are quota-limited but not automatically pruned. Broader logging, webhook container-change coverage and measured event/stream capacity remain gaps or qualification work.
- Live publisher remains MPEG-TS; rich live fMP4, true Windows custody/retirement, active guide-window extensions and very large playlist snapshot streaming are not complete. Review strengthened shared-capture cancellation/recovery, but seamless handoff is not physically established.

### Platform and release gaps

- **Vega:** safe authenticated transport is missing. HDR, surround, system caption style/display matching, effects/speed/gapless, downloads, nearby discovery, Cast and home-screen rows are absent or hidden. The transport blocker must be fixed before any parity claim.
- **Apple:** no Swift/Objective-C compile, SDK link, XCTest target run, signed archive, store validation or scene/reinstall/background acceptance. Scene and protected-data-aware reinstall source now exist; older reports saying they were absent are superseded. Apple TV mpv HDR/PiP/AirPlay/live/passthrough limits remain.
- **Android/Fire:** no full Gradle/CMake packaging/APK install. Process-dead download recovery needs a platform job. mpv lacks passthrough/deinterlace/DV5; HDR direct output and mixed fonts need device work; RN TV rotation/density remains open. Actual Fire OS is required.
- **Downloads/audio/browser:** trusted monotonic receipt expiry, real HE-AAC fixtures, sample-exact joins across outputs, background media keys and Safari AirPlay/PiP drawn-subtitle behavior remain unresolved or unqualified.
- **Delivery:** exact cross-repository dependency locks, Apple/Vega release-build/SDK CI, OS signer/notarization provisioning, complete native artifact notices/relinking/source materials and Apple distribution rights remain. A reported `source-map-js` advisory has no accepted resolution in the lane evidence; obtain the current vulnerability result before release.

## 5. What to test, and where

The detailed acceptance handoff belongs in [test-plans/](test-plans/). That lane was still running at this draft checkpoint; verify its final index and environment files before treating this link as a completed plan. Individual lane reports also contain cold-start procedures. This document deliberately does not substitute a passing unit test for those procedures.

| Environment | First checks | What constitutes a pass |
|---|---|---|
| Ubuntu server, disposable state/media | Real HTTP setup/direct/code/profile/key paths; restricted reads/actions; scan/missing/replacement/delete; re-plan with rejected alternatives; offline receipts; restore/crash recovery | Correct readback, no unauthorized content, preserved working playback on refusal, no unapproved deletion, sessions retired after restore and documented key exception |
| Human browser walk: Chrome/Firefox/Safari, phone and desktop widths | Sort/filter/paging/Back/focus, copy-once key/token flow, reconnect, two tabs over HTTPS and ordinary HTTP, subtitle workers/CSP, audio/media keys, PiP/fullscreen/AirPlay | Content stays visible, controls change the real resource, no first-frame pause leak, correct subtitle glyph/timing and no credential/state crossover |
| Mac: iPhone/iPad/Apple TV builds and devices | Compile/link before install; SPM product, scene launch/links, locked-first-unlock/reinstall, downloads, engine teardown/relay, native/mpv HDR/audio/subtitles | Clean signed builds and actual playback/seek/output/focus with truthfully hidden unsupported features; no retained sign-in after reinstall |
| Android, Android TV, real Fire TV | Full Google/Fire builds, four-ABI/16-KiB packaging, lower OS versions, control reserve, engine handoff, HDR/subtitles/audio, Wi-Fi/force-kill downloads, IME/density | Correct packaged native libraries, no replay/redirect credential leak, control commands survive read saturation, privacy-safe download recovery and real panel/output behavior |
| Actual Vega plus installed SDK | Build and replace safe transport refusal; capture all redirect types; auth/streams; decoder facts/live MediaSource/focus | Zero credential-bearing requests at redirect target; working bounded native transport; then real sign-in/playback. Current refusal is a safe failure, not this pass |
| Windows/macOS/Linux installers and Docker | Install/upgrade/reboot/remove, service account/ACLs/UNC, watcher/root replacement, recovery/restore, proxy/IPv6/multicast topology | One working supervised process, preserved chosen service settings/state, no accidental media deletion, actual private reachability and documented Docker discovery behavior |
| GPUs and real media | VAAPI/QSV/NVENC/VideoToolbox/AMF, HDR10/HLG/DV, 4K, rotated/anamorphic media, rich audio, genuine bitmap/ASS corpus | Correct colours/geometry/channel layout/timing, actual qualified sandbox grants, safe fallback, accurate advertised encoder/decoder support |
| Real tuners/NAS | Two viewers plus two private DVR owners, cancellation/restart/reconnect, guide correction/preemption, slow mounts and large guides | One channel input, independent access/output, shared bytes retained for remaining owner, honest partial gaps and usable guide rows |
| Hosted staging with separate empty PostgreSQL | Consent/cookies/provider sign-in/callback, two replicas and signer revocation, server check-in, claims/directory/DNS and rollback | Identity envelope admitted only by the local server; revoked consent rejected at transaction time; current certificate delivered; recreated claims work; no invalid signer rollback |
| Scale on representative weak/strong hosts | 10k/200k/2M catalogue scenarios, long writer holds, first import, filtered group sorts, backup WAL pressure, viewers plus direct streams | Recorded latency/refusal/recovery and memory/disk outcomes against explicit targets. The 200-browser/100-stream target remains unestablished |

Provisional measurements now exist: a real 10k mixed fixture took 160.12 seconds to seed/derive and initially issued 479 Home-read statements; a separate index-only 2M-row deep page improved from 489.28 to 0.476 ms. This is reported single-shape evidence on a shared host, not a 2M-title capacity result. Loopback Hosted reports real password/claim/identity/PKCE/TV paths and revocation/rate checks passing, with a newly observed client outage-gate bypass under investigation. The running local integration, browser automation and scale lanes can provide stronger final evidence later. They cannot retroactively establish physical-device or human visual acceptance.

## 6. Bugs and independent reviews

The refreshed reconciliation covers all **588 unique register IDs**, using the remaining-bug lanes and later follow-ups rather than selecting an unrelated lane's “not fixed here.” It keeps partially delivered implementation and unresolved acceptance/design cases open. Severity below uses the register's **second-review severity**, which differs from the first-pass labels used by the older coverage ledger.

| Disposition | Count |
|---|---:|
| Reported fixed | 362 |
| Already fixed on the base | 162 |
| Reported not present | 9 |
| Intentional under current decisions | 10 |
| Open or partly delivered | 45 |
| **Total** | **588** |

The open group has **2 High, 22 Medium and 21 Low** entries.

These are reported dispositions, not independently reproduced outcomes for every entry. “Fixed” means a lane reports a change addressing the source finding; “already fixed” includes earlier base/fix-pass evidence. Tests and physical acceptance remain separate. Ten intentional choices are kept separate from nine findings reported not present. Calling accepted behavior a fix would be misleading.

The two remaining High entries under the second review are:

- **B-11.8 — HDHomeRun guide/recording flow.** Backend XMLTV ID/number/name mapping exists, but the web creation form still hides the guide URL and needs mapping controls. The backend fix alone does not give the owner a complete recording setup flow.
- **B-22.19 — Vega redirect credentials.** Leakage is contained before SDK dispatch. Authenticated Vega functionality is unavailable until a real pre-follow-refusing transport is supplied. This is still a release blocker.

The older ledger's eight first-pass High entries also deserve an explicit disposition: B-06.21 anime order/offset mapping and B-06.24 fresh-sibling publication cost are reported fixed by leftovers-server-a; B-13.3 follower drift is reported source-resolved with extended latency simulations, awaiting physical timing; B-19.18 scene launch is fixed in source, never compiled; B-22.6 is deliberately retained HTTP trust rather than a promised authenticated plaintext socket; B-22.19 is the blocker above; B-25.10 release/device coverage and B-25.12 Apple component distribution remain open (both are Medium in the second-review register). Their first-pass severity is not silently discarded.

### Completed independent review findings

| Review | Blocker / High / Medium / Low | Total | Final recorded result |
|---|---:|---:|---|
| Identity/title/file | 0 / 3 / 7 / 0 | 10 | All corrected in appended fixes |
| API platform | 0 / 2 / 3 / 1 | 6 | All corrected in appended fixes |
| Networking | 0 / 2 / 5 / 3 | 10 | All corrected in appended fixes |
| Accounts/security | 0 / 2 / 3 / 0 | 5 | All corrected |
| Playback core | 0 / 6 / 1 / 0 | 7 | All corrected |
| Downloads | 0 / 3 / 3 / 0 | 6 | All corrected |
| Hosted | 1 / 0 / 3 / 6 | 10 | Nine defects corrected; one coverage addition retained, reported ten closed |
| Freshness | 0 / 0 / 5 / 3 | 8 | All corrected; a withdrawn ninth suspicion is excluded |
| Intake/scanning | 0 / 5 / 3 / 0 | 8 | All corrected; one disproved suspicion excluded |
| Conversion pipeline | 0 / 2 / 4 / 3 | 9 | All corrected |
| Live TV/DVR | 0 / 5 / 6 / 1 | 12 | All corrected |
| Operations | 0 / 2 / 7 / 0 | 9 | All corrected |
| Late server follow-ups | 0 / 0 / 4 / 0 | 4 | Four corrected; late merge observed at `189b32f`, no new full-base suite claimed |
| **Total** | **1 / 32 / 54 / 17** | **104** | Repository regressions/source evidence; no release acceptance implied |

The most serious examples explain why these passes mattered. Identity deletion could follow a swapped parent symlink outside the admitted library and could remove newly attached, unpreviewed titles. Different disks could share an asset through inode coincidence. Playback refusals could destroy a working presentation, and unauthorized changes could cancel another viewer's preparations. Session reads/replays could bypass current profile/content fences. Revoked Hosted signers could admit pending sessions; device decisions trusted authority checked before their transaction. Downloads replay could revive revoked access. Operations restore could replace/remove the live database incorrectly. Shared DVR cancellation could cut another owner's recording. Hosted app sign-in rejected the challenge every real server issued. Networking could close HTTP/2 before its first request and replay revoked proxy trust into memory.

Apple mpv, Apple native and Android native reviews also made substantial source/lifetime/input/packaging repairs. They use different inventories and do not supply comparable complete severity totals, so they are not added to the 104. Apple native code still has no compiler evidence. The late registry review reports **3 Medium** fixes implemented, with final verification still in progress. The security sweep records **4 Medium and 1 Low** fixes, but its overall review remains unfinished. Web logic records **3 High and 3 Medium** findings; five have fix milestones and the music Range-reader bound still lacks a final fix receipt. Native JavaScript records **4 High, 6 Medium and 1 Low** findings with no completed repair receipt at this checkpoint. These provisional counts are separate from the 104 completed review findings and must be finalized after integration. The late native findings include erased offline acknowledgements, cross-viewer enrollment, stale starts replacing new playback and diagnostic credential disclosure. The late web findings include combined bitmap memory amplification, old controller operations replacing new sessions and unsafe stale settings saves. Their recorded repairs still need final integration and native package refresh. Do not turn “all completed review findings corrected” into “no defects left.”

Appendix D lists every open entry and the full per-domain disposition. The original bug register and 12 October fix log remain historical sources.

## 7. How to land it

Nothing in this programme has been pushed, merged into your own working branches or deployed. Integration merges happened only between programme branches. The three repositories each have their own **`audit-implementation/main`** branch:

| Repository | Programme worktree | Checked head | Latest recorded base validation |
|---|---|---|---|
| portico-server | `main-server` | `189b32f0f327432bcc5d49d455fe6eaaa02ab65c`; last fully tested `ce344737a44d6593a4f7cd9809f404b4aabbb024` | 85 tested Go packages, 4 apikit packages, 1,848 shared-client tests, 512 web tests, 20 Cast tests; Linux, Windows amd64, macOS arm64 non-cgo and devtrust builds pass |
| portico-react-native | `main-native` | `261783468807925f45257ad9fdce5a831227372c` | Native 514/514, Vega 64/64; app/test type checks and string lints pass; packages reported identical after contracts-copy fix |
| portico-internal | `main-internal` | `416708d966af526103164844bb9bd05d670a6147` (also this draft fork) | Hosted build/focused tests and isolated PostgreSQL-backed tests reported passing; current format-21 follow-ups checked selectively, no final full combined internal pass or deployment claimed |

The server's `go vet` still returns exit 1 for **63 known vendored SQLite unsafe-pointer findings**, with no Portico finding in the final report. Cross-builds are not Windows/macOS execution. Native tests run JavaScript with stand-ins; they do not compile Swift/Objective-C, build APKs or run TV renderers. The server validation was on the final base-health-2 tree before its integration merge; `189b32f` adds the late server fixes. No full test result for that newer head is claimed. The three named base trees were clean when inspected. Hosted's current toolkit is no longer identical to the server copy; resync it after the new generator/permission changes settle.

Recommended order:

1. Take the final decision/browser/integration/scale/review reports and finish their concrete gaps. Reconcile this dated draft rather than reusing its counts after more changes land.
2. Finish frontend controls and consumers, Vega transport and the named server contracts. Verify package copies against exact server/native revisions. Run the final integrated health pass after those merges; do not weaken restriction matrices to obtain green results.
3. Compile/archive Apple applications on the Mac; build Google/Fire applications on Ubuntu and Vega with its real SDK. Run the acceptance matrix privately with fresh state and representative media. Resolve native distribution materials and signing inputs before publishing packages.
4. Review the final diffs and settings/credential choices. Preserve your originals and older databases. Land server/shared contracts first, native matched to that exact revision next, and internal docs/Hosted matched to the same toolkit after that. Push only when you choose to publish; no lane has done so.
5. Use fresh local state for the changed baseline. Keep old state with its matching old binary for rollback. Do not point the new server at an older pre-release database and expect migration.
6. Treat Hosted as its own staged cutover. Provision a separate empty format-21 database, validate claims/provider/cookie/DNS/certificates, then switch traffic. Rollback uses the retained compatible release/database/config and a still-valid signer; never resurrect a revoked signer or silently forget learned revocations.
7. Keep the existing demo's conversion setting off. Do resource/GPU/load tests elsewhere. Release only after first-party parity and actual environment evidence are written down.

The audit originals are read-only in the original internal checkout. They were not present in this draft fork's audit directory and were not duplicated by this lane. Land this report beside those historical files when the programme's internal documents are gathered. All lane reports below are in the shared programme directory, outside the application repositories.

## Appendix A. Lane records

All report filenames in this appendix refer to `/home/justin/projects/Portico/.worktrees/audit-implementation/_program/reports/`. Links resolve in the current worktree layout. Archive those reports with the internal handoff if the branches move elsewhere. Final appendices in an older report take precedence over its initial gap list; later owning-lane reports take precedence over unrelated “not mine” entries.

**accounts-security.** Unified new-sign-in admission, live permission changes, Cast-only authority, member erasure and restore scrubbing. Account/profile/TV policies were repaired; native management screens, residual restriction differences and historical Apple sign-out acceptance remain. [Report](../../../../_program/reports/accounts-security.md).

**android-mpv.** Added Android/Android TV/Fire TV mpv module, relay and JNI wiring with declared codec/output limits. JavaScript/profile checks passed; a full application build and physical HDR/audio/subtitle/lifecycle validation remain. [Report](../../../../_program/reports/android-mpv.md).

**android-native-review.** Repaired download ownership/integrity, callbacks, relay lifecycle, captions and engine teardown. Standalone Kotlin/Java and four-ABI JNI linkage/checksums passed. No Gradle packaging, APK or device run. [Report](../../../../_program/reports/android-native-review.md).

**api-platform.** Implemented keys, tokens, scopes/budgets, title query, changed-path scanning, signed webhooks and MCP. Integrations survive password/session changes and restore. Follow-ups and reviews supersede its early missing-screen and missing-service lists. [Report](../../../../_program/reports/api-platform.md).

**apple-mpv.** Added separate mpv engine and system-validated relay for Apple platforms, plus engine choices and declared limits. Apple TV output is SDR. Swift source was never compiled or exercised. [Report](../../../../_program/reports/apple-mpv.md).

**apple-mpv-review.** Repaired render-surface/audio lifetime, seek ordering, bounded fonts/HLS relay and capability declarations. JavaScript/project parsing checks passed; Swift tests and native linking have not run. [Report](../../../../_program/reports/apple-mpv-review.md).

**apple-native-review.** Repaired AVPlayer validation, callback queues, downloads, captions, audio facts and Top Shelf isolation/privacy. Project/plist/type checks passed. Its initial scene/reinstall gaps were later addressed in source by leftovers-native, with no Apple compile. [Report](../../../../_program/reports/apple-native-review.md).

**audio-engines.** Implemented continuous web resampling, bounded deep-seek fallback, runtime decoder probes, Android gain and Apple AAC trim/rewind/fallback source. Narrow tests passed; Apple limited AAC target and real HE-AAC/device output qualification remain. [Report](../../../../_program/reports/audio-engines.md).

**base-health.** Combined early lanes, repaired cross-lane identity/revision/restriction/units/intake failures and ran full server/shared/web/Cast suites. All requested checks passed except known vendored vet warnings. Later merges required the second pass. [Report](../../../../_program/reports/base-health.md).

**base-health-2.** Repaired later retention/auth/PIN/request/recording/playback collisions. Final report records 85 Go packages, 1,848 shared tests, 512 web, 20 Cast, four apikit packages and cross/dev builds passing, with vendored vet warnings retained. [Report](../../../../_program/reports/base-health-2.md).

**console.** Three web passes delivered integrations, metadata/attention, titles/files, account/devices, networking, operations, scan issues, resource readback and title-page Undo. Custom owner pages remain web-only; its final unfinished list drives the remaining frontend work. [Report](../../../../_program/reports/console.md).

**conversion-outputs.** Added qualified rich picture/audio outputs, fMP4 publication, HDR/tone-map/burn-in/font paths, hardware fallback and bounded diagnostics/working space. Ordinary planner wiring came later. GPU/native output and richer prepared recipes remain unproved/incomplete. [Report](../../../../_program/reports/conversion-outputs.md).

**decision-audit.** Latest captured report traces 31 programme bullets and all 61 original decisions. It reports five corrections: Automatic must start Original, mpv remux acceptance, removed profile-wide PIN lockout, bounded guide fallback and tolerant download extras. Final receipt/sync remains pending; its native library-removal row conflicts with the source spot check. [Report](../../../../_program/reports/decision-audit.md).

**docs-internal.** Updated 41 architecture/contract/spec/backlog/decision documents. Separates requirements, reported source and unverified acceptance; preserves decision history and explicitly records Vega transport and Hosted fresh-database consequences. Later changes still need reconciliation. [Report](../../../../_program/reports/docs-internal.md).

**downloads.** Implemented durable device receipts, resumable originals/conversions, offline context, renewal and play history. Its appended merge carries styled/image originals and fonts; later follow-ups preserve chosen version and merged claims. Platform recovery/clock/signed-out player gaps remain. [Report](../../../../_program/reports/downloads.md).

**e2e-hosted.** Runtime work is in progress on disposable loopback Hosted/server state. Reports fresh-format initialization, real password/claim, directory, first-party, app PKCE and TV flows passing; rotation/revocation/rate checks report progress, while a client outage-gate bypass and final evidence remain unfinished, with no production deployment. [Report](../../../../_program/reports/e2e-hosted.md).

**e2e-server.** Real loopback server scenarios with disposable state and generated media are underway. A final runtime result is not available; the passing base suite must not be substituted for this end-to-end report. [Report](../../../../_program/reports/e2e-server.md).

**e2e-web.** Production build/headless Chrome work is underway against a disposable loopback server. Final runtime and visual issue list was not available. Automation does not establish a human browser acceptance walk. [Report](../../../../_program/reports/e2e-web.md).

**freshness-clients.** Added living viewer-scoped document hub, quiet reads, event sharing and delayed connectivity errors. Tests use simulated transports/locks. Some pages/deep-list refresh and HTTP tab sharing remain incomplete; native cards do not fade in. [Report](../../../../_program/reports/freshness-clients.md).

**freshness-server.** Added retained browse generations/two FTS slots, separate revision domains, private coalesced feed, failure isolation and bounded reclamation. Client/console consumers came later. No target-scale throughput or physical network acceptance claimed. [Report](../../../../_program/reports/freshness-server.md).

**grouping-sorts.** Added common scoped title vocabulary, indexed scalar/profile orders, exact paged aggregates and capability contracts. Filtered aggregate sorting/deep direct ranges retain disclosed cost gaps; existing legacy envelope screens need migration. [Report](../../../../_program/reports/grouping-sorts.md).

**handoff-draft.** Assembled this dated owner draft, reconciled every original register ID, checked selected high-impact claims against source and recorded unverified statements separately. Documentation only; no application code, Git metadata, original checkout or deployment changed. [Report](../../../../_program/reports/handoff-draft.md).

**hosted.** Added no-registration S256-PKCE app consent returning the ordinary server-bound assertion, signer/revocation/outage fixes and fresh-only format. Review/follow-ups supersede format-20/toolkit/provenance gaps. No existing service was changed. [Report](../../../../_program/reports/hosted.md).

**hosted-followups.** Synchronized scoped toolkit/99 JSON routes, durable root-certificate check-in and transaction-time consent revocation; improved consent focus/direction/retry behavior. Format 21 requires separate empty state. Fleet/provider/deployment acceptance and local oversized-root hardening remain. [Report](../../../../_program/reports/hosted-followups.md).

**identity.** Separated titles/assets/versions, retained state, missing-title rules, real file deletion and background library removal. Later review fixed confinement/asset/approved-set issues; later intake/wiring delivered editions/parts. General audio/container merge and large transaction behavior remain limited. [Report](../../../../_program/reports/identity.md).

**integrator.** Combined overlapping server/native work, restored compatible schema/settings/units and package copies, and exposed hidden integration defects. Its early grouping/title-order question was subsequently overruled by the wide subgroup sort decision. [Report](../../../../_program/reports/integrator.md).

**leftovers-native.** Disposed 116 register entries and repaired Apple scenes/reinstall, native control reserve/runtime facts/frozen pages, settings reachability and LAN promotion. Vega disclosure was contained by refusing authenticated operation. Exact playlist/subtitle/view selectors and frontend gaps remain. [Report](../../../../_program/reports/leftovers-native.md).

**leftovers-server-a.** Disposed 27 entries: anime orders/offsets, fresh episode reuse, account-management boundaries, legacy admission removal, failure-memory APIs, tolerant local XML, preview rules, backup bounds and watchdog. Historical Apple incident and owner screens remain. [Report](../../../../_program/reports/leftovers-server-a.md).

**leftovers-server-b.** Disposed 32 entries: internal error mapping, early EOF/credits fallback, bounded backup/fan-out, offline future stamps, baseline consolidation and release-input/vulnerability guards. Native clock/recovery/CI, advisory resolution and Apple rights remain open. [Report](../../../../_program/reports/leftovers-server-b.md).

**limits-settings.** Wired runtime controls and still-watching, added owner analysis/subtitle limits, consolidated background pace/pause, introduced corrupt-state recovery and source-consumer guards. Structural witnesses do not prove device effects; its early quality inventory predates final consolidation. [Report](../../../../_program/reports/limits-settings.md).

**livetv.** Implemented shared acquisition/private readers, tolerant streamed/spooled guides, import controls, shared private DVR bytes and recovery facts. Backend tests passed; owner guide/countdown consumers, Windows custody, real tuners and load acceptance remain. [Report](../../../../_program/reports/livetv.md).

**music-audiobooks.** Added bounded broad audio-tag reader, lyrics/art/chapter/grouping, natural part order, comfort rewind, queue shuffle semantics and durable audio facts. Own-renderer/native follow-ups came later. Online book provider and physical/sample-exact codec coverage remain absent. [Report](../../../../_program/reports/music-audiobooks.md).

**naming.** Added bounded pure naming/evidence/ignore reader and hundreds of corpus tests without a dependency. Scan-loop later wired configuration and identity/evidence into production. Disc playback is deliberately unsupported; hostile bounds remain. [Report](../../../../_program/reports/naming.md).

**native-app.** Repaired viewer-scope lifetime, virtualized Saved/History, links, setup/account UX and profile-aware accessibility. Later lanes canonicalized staged strings. Saved views still cap playback at 500 and exact playlist/subtitle selectors need contracts. [Report](../../../../_program/reports/native-app.md).

**native-health.** Made refreshed native/Vega packages and existing browse controls pass: 514 native/64 Vega tests, app/test type checks and lints. Added bounded silent 428 recovery. These are JavaScript checks, not native application builds or rendering evidence. [Report](../../../../_program/reports/native-health.md).

**native-playback.** Moved native video onto shared session controller, in-place choices, capability publication, Automatic measurement, local extras and caption/display source wiring. SDK/device behavior remains unproved; setting-change/session and exact subtitle-selector gaps remain. [Report](../../../../_program/reports/native-playback.md).

**native-sync.** Mirrored packages, canonicalized staged strings, joined playback settings/version picker, HTTP/ATS/address memory, approval/outage handling, handoff/report/privacy and optional sign-out removal. Its join appendix supersedes early type/string/version gaps; custom owner parity remains missing. [Report](../../../../_program/reports/native-sync.md).

**networking.** Unified locality/proxy handling, HTTP/TLS/IPv6 listener, external routes, own-certificate reload, discovery topology and console reserve. Independent review repaired h2/trust replay/policy refresh defects. Real proxy/multicast/container/device behavior remains unqualified. [Report](../../../../_program/reports/networking.md).

**ops-packaging.** Added restart parent, detached restore, SCM/macOS/Linux/Docker recipes, alternate storage, notices/reports, signing inventory and reclamation. Fixture/cross checks passed. No installer/reboot/signing/notarization/Docker/GPU acceptance was performed. [Report](../../../../_program/reports/ops-packaging.md).

**personal-state.** Implemented bounded atomic container marks/Undo, completion/history/privacy ledgers, Home destinations and corrected Continue behavior. Download/quiet-counter integration came later. Physical EOF/offline/large-container and complete UI acceptance remain. [Report](../../../../_program/reports/personal-state.md).

**planner-wiring.** Connected converter qualification/target persistence to ordinary/live/offline planning, signal-preserving choices and executable fMP4 tests. Live publisher remains TS; optimized downloads remain H.264 SDR stereo; mastering-display/MaxCLL facts are not captured. [Report](../../../../_program/reports/planner-wiring.md).

**playback-core.** Created one capability document, native/mpv choice, Original/fixed/Automatic policy and generation-fenced session changes, remembered tracks and failure memory. Producer/client/review lanes supersede its early missing-output/menu notes. Real hardware claims remain untested. [Report](../../../../_program/reports/playback-core.md).

**providers-matching.** Added verified fuzzy/yearless matching, attention actions, provider-specific pace, locale/credential/art/NFO policy and refresh. Scan-loop wired sidecars; leftovers completed anime/episode-cache cases. Fixtures are not live provider acceptance. [Report](../../../../_program/reports/providers-matching.md).

**registry.** Declared all retained production API handlers, adopted generated path/response/body helpers and removed duplicate YAML specs. Base-health-2 records 622 operations and separate optional/development contracts. Later registry review identified destination/schema declaration defects, with fixes pending final receipt. [Report](../../../../_program/reports/registry.md).

**review-accounts-security.** Found and fixed five findings: revoked-signer descendants/pending paths, transaction-time device authority, restore integrity/retry and waiting-device admission. Regressions passed. Original residual client/privacy/incidents are not certified closed. [Report](../../../../_program/reports/review-accounts-security.md).

**review-api-platform.** Found six findings, later fixed in appended phase: pre-admission SQL, cursor cost, webhook fairness/order, MCP gap and use accounting. Its initial findings-only “unfixed” text is historical; appended fixes and base integration supersede it. [Report](../../../../_program/reports/review-api-platform.md).

**review-downloads.** Found and fixed six access/replay/activity/address/metadata-publication defects while preserving standalone copies. Focused repository tests passed; native storage/restart/expiry/signed-out operation remains separate acceptance. [Report](../../../../_program/reports/review-downloads.md).

**review-freshness.** Found eight confirmed findings and fixed them, including event replay gaps, dismissal/history/search hints, counters, Unicode predicates and diagnostic wire time. One suspicion was withdrawn. Large-capacity and missing screen consumers remain separate. [Report](../../../../_program/reports/review-freshness.md).

**review-hosted.** Reported ten closed findings including real-server challenge mismatch, cookie lookup, check-in/signing availability and safe consent/callback behavior. One item added coverage. Isolated PostgreSQL checks passed; browser/device/fleet/cutover acceptance remains unperformed. [Report](../../../../_program/reports/review-hosted.md).

**review-identity.** Found ten findings and later repaired cross-disk asset identity, symlink-safe deletion, approved-set consistency, show/provider/tag/retention identity and offline missing protection. Regression tests passed; device/file-system and large-transaction acceptance remains. [Report](../../../../_program/reports/review-identity.md).

**review-livetv.** Found and fixed 12 parser/acquisition/capture/authorization/recovery/control defects. In particular shared owners survive cancellation/finalization and restricted metadata stays hidden. No physical tuner/NAS/GPU or complete saturation acceptance. [Report](../../../../_program/reports/review-livetv.md).

**review-native-js.** Ongoing source review has recorded 11 findings: four High, six Medium and one Low across offline acknowledgements/enrollment, playback intent, Vega seeks/streams, trace redaction and hook tests. No final closed-fix receipt yet; supersedes any implication native tests establish completeness. [Report](../../../../_program/reports/review-native-js.md).

**review-networking.** Found and fixed ten h2/trust replay/reserve/origin/scoped-address/policy/publication/status defects. Initial findings-only text is preserved historically; the appended implementation receipt is current. Actual supported topologies/capacity remain unqualified. [Report](../../../../_program/reports/review-networking.md).

**review-ops.** Found and fixed nine restore/copy/scrub/report/retention/supervisor defects, with focused regressions. Native power-loss/service/release acceptance and rights/signing work remain. No original database or deployment was touched. [Report](../../../../_program/reports/review-ops.md).

**review-playback-core.** Found and fixed seven transactional/authorization/ceiling/locality/control-context defects. Refused changes preserve working playback; source visibility is rechecked. Physical throughput/player behavior and the distinct subtitle mutation boundary remain unproved. [Report](../../../../_program/reports/review-playback-core.md).

**review-playback-pipeline.** Found and fixed nine geometry/copy/colour/header/fMP4/audio-clock/signalling/orphan-walk defects. Real generated conversion checks complement source assertions; physical GPU/HDR and confinement behavior still require acceptance. [Report](../../../../_program/reports/review-playback-pipeline.md).

**review-registry.** Ongoing review reports three Medium fixes implemented: safe route parameters, accurate analysis/lyric/status contracts and real nullable-union types/decoders. Targeted checks reported; final receipt/integration was not complete at this draft checkpoint. [Report](../../../../_program/reports/review-registry.md).

**review-scan.** Confirmed eight findings and fixed them, including unsafe weak audio identity, stale revival/removals, unreadable subtrees, bounded absence checks and extra/artwork evidence. One suspicion disproved. Native watchers/NAS/provider/large-load acceptance remains. [Report](../../../../_program/reports/review-scan.md).

**review-server-late.** Reports four Medium repairs: playlist request identity, exact-version retry, deletion fact reconciliation and restricted DVR picker continuation. The ordinal cursor avoids hidden identities. Its three fix commits and merge are now visible on main-server; a new full-base suite remains required. [Report](../../../../_program/reports/review-server-late.md).

**review-web-logic.** Ongoing review recorded six findings: three High and three Medium. Five have appended fix milestones for subtitle budgets, controller lifetime, settings safety, feed resync and memory ordering; music Range response bound and final integration remain open at checkpoint. [Report](../../../../_program/reports/review-web-logic.md).

**scale-check.** Reports indexed browse/count/hydration repairs, a real 10k fixture and a separate 10k/200k/2M index traversal proof. The 2M deep-page observation improved from 489.28 to 0.476 ms; this is not mixed-catalogue capacity. Broader tests/integration remain pending. [Report](../../../../_program/reports/scale-check.md).

**scan-loop.** Wired relative naming/tags/IDs/editions/parts/extras/discs, ignore/evidence lifecycle, tolerant pages, native watchers, change-driven limited scans and bounded retention. Linux watcher exercised; macOS cgo and real platform mounts remain uncompiled/unqualified. [Report](../../../../_program/reports/scan-loop.md).

**search-sort.** Added natural/Unicode sorting, richer indexed search/CJK groups, aggregate paging, integer 0–10 ratings and general millisecond/UTC boundaries. Common grouping controls came later; raw-title anchors/Saved search/specialized time shapes remain incomplete. [Report](../../../../_program/reports/search-sort.md).

**security-sweep.** Ongoing cumulative review inventories 622 routes and records four Medium/one Low fixes: credential expiry cache, cross-member playback scope, administrative feed audience, diagnostic secrets and image-hash restrictions. Overall review/integration is unfinished; no final security verdict. [Report](../../../../_program/reports/security-sweep.md).

**server-docs.** Updated 34 public documents and two guides, installation/API/media/dependency notices and executable examples. Later registry removal supersedes its kept-YAML note. Source-backed instructions are not proven deployment, artifact licence or signing acceptance. [Report](../../../../_program/reports/server-docs.md).

**server-followups.** Delivered exact attention/artwork/resource status, one-version preview/delete, download identity pinning, source pickers, owner transport controls and role freshness. Web third-pass consumers followed; later review caught request/deletion/picker edge cases. [Report](../../../../_program/reports/server-followups.md).

**social.** Repaired room readiness/revisions/presence/privacy and Cast recovery/takeover; exposed handoff control through the shared API. Some adapters still ignore paused start or need event/storage wiring; no Cast/native synchronization device run. [Report](../../../../_program/reports/social.md).

**subtitles-text.** Delivered lazy track discoveries, tolerant legacy text/cues, raw originals/fonts, provider login and preview/marker/conditional-response fixes. Later limits expand inventory/file defaults. Renderer/burn/offline integration and real corpus/provider acceptance remain separate. [Report](../../../../_program/reports/subtitles-text.md).

**test-plans.** Fifteen environment plans plus index are being assembled; eight installation/recovery plans and subsequent browser/GPU/network drafts reported. Remaining native/Vega/tuner/Hosted/integration coverage and final snapshot were unfinished. No plan is an executed acceptance result. [Report](../../../../_program/reports/test-plans.md).

**vega.** Implemented shared engine contract, live sequence buffering, Automatic facts, captions/capability/remote shims and bridge tests. Later transport safety containment overrides its parity claim: authenticated operation unavailable. SDR/stereo and unsupported features remain explicit. [Report](../../../../_program/reports/vega.md).

**web-app.** Implemented bounded tolerant presentation parsing, per-tab memory, search privacy/history, stable reconnect/page state, role confirmation and non-player UX fixes. Security identities/receipts remain strict. Later hub/console work supersedes its early missing-field list. [Report](../../../../_program/reports/web-app.md).

**web-player.** Replaced old adapter with shared controller and persistent video element; repaired session/timeline/lease/queue/quality/failure/keyboard/seek behavior. Later subtitle/audio lanes fill parts of its gap list. Actual hls.js/element/media-key behavior remains unqualified. [Report](../../../../_program/reports/web-player.md).

**web-subtitles.** Added vendored browser libass, own PGS/VobSub/DVB workers, fallback fonts/position, 3x audiobook/up-next and native-HLS policy. Merger wired raw plans; late web review repairs combined-memory budgets. Real corpus/browser/PiP/cast/font coverage remains incomplete. [Report](../../../../_program/reports/web-subtitles.md).

## Appendix B. New and newly wired settings

The tables cover changed controls, rather than repeating every unchanged setting. “Screen” means reported/source-present, never a completed visual or device test. Defaults/limits describe this implementation, not a tested capacity. The old 20 per-network quality fields, physical-trash/grace controls, WAN-as-LAN and forced-TLS choices were retired; do not copy them from an early report's settings inventory.

### Owner and library controls

| Control / key | Type and default | Effect and reachability |
|---|---|---|
| Missing titles `library.missingTitles`, `missingTitlesDays` | Immediately / after days / never; after 30 days, 1–3650 | Last available file gone; automatic cleanup at worker checkpoints. Web Storage & backups; generic phone rows reported |
| Monitor changes `monitorChanges` | Per-library boolean, true | Native watcher plus periodic fallback; manual/configured scans still work. Web Scanning tab; other custom clients pending |
| Background speed `backgroundSpeed` | Gentle / Normal / Fast, Normal | Shared provider/database/source pace and process priority. Web Schedule; shared row. Separate priority choice removed |
| Background pause | Owner action, no pause; one hour | Cooperative batches/admissions stop until deadline or Resume. Web Schedule; other clients need action UI |
| Analysis storage `analysisStorageGiB` | Integer, 32 GiB; 1–16384 | New analysis artifact admission/publication. Shared Storage & backups row |
| Per-stage output `analysisStageStorageMiB` | Integer, 256 MiB; 1–4096 and no larger than total | Stage budget. Shared Storage & backups row |
| Tracks per file `subtitleTrackLimit` | Integer, 128; 32–4096 | Useful bounded subtitle inventory on new scan. Shared storage row |
| Largest subtitle `subtitleFileLimitMiB` | Integer, 128 MiB; 32–512 | New sidecar/acquisition/embedded bitmap reads. Shared storage row |
| Local artwork `localArtworkLimitBytes` | Integer bytes, 2 GiB | New ingestion quota and deduplicated usage ledger; retained images survive lowering. Web Metadata, GB display |
| Adult search `includeAdult` | Boolean, false | TMDB search. Web Metadata |
| Provider credentials `tmdbReadToken`, `tvdbProjectKey` | Optional secrets, unset | Hot overrides, reads show presence only; blank restores application credentials. Web Metadata |
| Library provider/language/region | Curated choices; TMDB video, en-US/US; local-only available | Adapter/field ownership and certification policy. Web library metadata; native controls incomplete |
| Anime provider order/offset | Owner selection plus signed integer offset | Absolute order mapping; affected files can need order. Backend/core present; dedicated offset screens pending |
| FFmpeg `ffmpegPath` | Absolute path, empty uses default | Qualification/status before restart; no hot running-tool change. Web Playback, requested/effective/refusal status |
| Conversion working space `sessionStorageMiB` | Integer, 4096 MiB; 64–1048576 | Per-session pressure/reclamation. Web Playback uses GB; sub-GB values display poorly until edited |
| Temporary directory `temporaryDirectory` | Absolute path, empty | Restart; dedicated scratch child. Existing Playback row |
| Background conversions `maxBackgroundSessions` | Integer, 0 means no extra owner cap | Future background process admission. Existing Playback row |
| Tone mapping `hdrToneMapping` | Boolean, true | HDR planning; qualified algorithms/backends still apply. Existing Playback row |
| Remote sign-in `remoteSignInPolicy` | Allow / Owner only / Off, Allow | Every new issuance using receiving device locality. Existing connectivity screen |
| New-device approval `deviceApprovalRequired` | Boolean, false | Real device registration, waiting states and decisions; owner exempt. Web People › Devices; shared/native row |
| Member rating system `ratingSystem` | Published system or empty, empty | Unified age rule with unrated behavior. Web People account limits; native picker pending |
| Self-registration policy | Existing choice, off | Invite-only now works. No first-party policy-setting screen reported |
| Home networks `lanNetworks` | List, standard local/attached ranges | Sole locality authority; empty means all remote. Web lists and shared rows |
| Trusted proxies `trustedProxies` | List, empty | Verified address/scheme/host chains. Existing screen; web add/remove list |
| External address `advertiseUrl`, extra `accessUrls` | Origin/string list, empty | Local candidate publication; only HTTPS reaches Hosted. Web Remote access and shared phone rows |
| Ordinary connections `connectionLimit` | Integer, 0 auto; maximum 65536 | Restart; fixed 32-console reserve additional. Web Advanced and shared phone row |
| Own certificate path/key/domain | Strings, empty together | Validated/reloaded PEM source with serving-state readback. Existing/web/shared rows |
| LAN discovery/interface | Boolean true / string empty | Hot discovery lifecycle; explicit interface preference, no name-based automatic exclusions. Existing screens |
| API request rate `apiRequestsPerMinute` | Integer, 0 means 600/minute; up to 100000 | Per-key/token rate at once. Web Integrations |
| Block member token creation `blockMemberAccessTokens` | Boolean, false | Stops new plain-member tokens, preserves existing. Web Integrations |
| Live import limits `importLimits` | 512 MiB / 50,000 channels / 5,000,000 programmes | Owner maxima 1 GiB / 100,000 / 10,000,000; bounded per-record parsers remain. Web server-wide fields; per-source readback pending |
| Source groups/channels and mapping/numbering | Existing lists/default all and source fields | Filter before staging, stable provider choice IDs; web source pickers delivered, HDHomeRun guide setup pending |
| Live buffer/retry/user agent/horizon/logo/discovery | Existing values/defaults | Runtime consumers wired; source stream buffer is startup delay, not retained timeshift length |
| DVR padding/keep policy | Existing numbers/count/days | Capture and retention now use them; private ownership/shared storage preserved |
| Download storage `maxPreparedBytes`, `retentionDays` | Existing ceiling / 30-day claim retention | Converted artifacts only count as server prepared bytes; originals are in-place. Device copies survive claim expiry |
| Updates `updates.feedUrl` | Official feed; empty opts out | Periodic cached owner notice; custom feeds retained. Web “Check for new versions” switch |
| Scheduled backup keep count | Integer, seven | Scheduled copies only; manual copies retained until deletion. Existing screen |
| Certificate/storage notices and log/history retention | Existing thresholds/days | Five-minute notices and bounded maintenance consumers. Existing settings; acceptance pending |
| `PORTICO_CACHE_DIR`, `PORTICO_BACKUP_DIR` | Startup absolute paths, default under state | Transient media and ordinary backups; rollback copies stay beside database. Read-only web storage display, no editing field |
| OpenSubtitles username/password | Startup optional secrets, unset | Login/renewal; anonymous API-key path remains. No credential UI; old static token input retired |

### Viewer and device controls

| Control | Type / default | Effect and reachability |
|---|---|---|
| `quality.home`, `quality.remote` | Original / Automatic / fixed; Original | Device-class saved start defaults; session menu overrides for that viewing. Web/native rows |
| Player engine `playerEngine` / `engineSetting` | Automatic / Native / mpv; Automatic | Installation-local capability publication and planning; shown only where mpv is built. Native settings/player menu |
| `forceStereo`, `disablePassthrough`, `disableHdr` | Device booleans, false | Narrow declared capabilities; passthrough/HDR controls hidden where unavailable. Web/native wiring reported |
| Caption size/background | Choices, medium/translucent | Native text and web captions; authored ASS preserves style. Native player panel; native Settings section parity pending |
| Drawn subtitle vertical position | Device value, 0; −10% to +30% | Web overlay positioning in 2% steps; Up/Down/Reset in subtitle menu |
| Audiobook default speed | Choice, 1×; now through 3× | Server/shared/web extended choices; native engine/screen/device limits still need qualification |
| Music normalization, gapless, crossfade | Off / true / zero seconds | Gain/peak-safe PCM, continuous compatible joins, bounded crossfade. Existing controls; unsupported Vega effects hidden |
| Started threshold | Integer 1–25%, 5% | Known-duration Continue eligibility without 30-second ceiling; existing screen reachability noted as a remaining handoff |
| Played threshold | Integer 75–100%, 95% | Music and explicit video override; otherwise video library policy. Books use time remaining |
| Still-watching protection/count | Boolean true / integer 3, 2–5 | Server queue auto-advance policy and Continue prompt; manual next resets run. Existing controls now consumed |
| Search history `search.rememberHistory` | Profile boolean, true | Commit-only history; off shows none. Web and native Privacy |
| Badges `notifications.badges` | Profile boolean, true | Native Appearance row now exposed as well as web consumer |
| Pause history / show activity to members | Existing booleans | Owner records unaffected; private progress/counts and shared popularity honor privacy |
| Together activity `privacy.includeInWatchTogether` | Profile-server boolean, true | False anonymizes participant/position but preserves visible room occupancy/readiness; exact screen reachability still needs reconciliation |
| Appearance card size/backdrops/reduce motion | Existing values | Web consumers exist; native card/backdrop consumers unfinished, preserved as owner-overridable controls |
| Remove downloads at sign-out | Per-action boolean, false | Exact account's files only; keeps other profiles. Native option reported, physical cleanup untested |
| Library tab/sort/filter memory; fit/fill | Device/viewer memory, no new switch | Links preserve explicit intent; per-tab library memory; web fit/fill menu/key |

Remembered audio/subtitle choice, failure-memory clear, container Undo, version delete, reboot/restart, restore and report upload are actions, not extra saved switches. No MCP enable switch exists.

## Appendix C. Added components and licences

This is the source inventory, not a legal conclusion or proof that a final package contains complete notices/source/relinking materials. B-25.12 remains open. Review the actual signed artifact and `THIRD-PARTY-NOTICES.md`, plus the native dependency notices, before distribution.

| Added/bundled component | Version / purpose | Recorded licence and distribution concern |
|---|---|---|
| Browser libass WebAssembly | Vendored `@jellyfin/libass-wasm` 4.2.4, hash/provenance pinned | Bundle includes FriBidi LGPL-2.1-or-later; other parts recorded as MIT/ISC/FreeType/Zlib/BSD/BSL/public-domain. Keep corresponding source/submodules and notices; CSP needs wasm-unsafe-eval |
| Apple MPVKit | SPM exact `1.1.0-n9.0.2`, LGPL product, binary module Libmpv | LGPL component inventory, relinking/replaceability/source and store distribution require artifact review. Do not substitute the GPL product |
| Android libmpv | media-kit `v1.1.11` full flavour, four SHA-256-pinned ABIs | mpv `gpl=false`, FFmpeg `disable-gpl` plus `enable-version3`: LGPL version-3 FFmpeg with mbedTLS. libass/dav1d/other linked components need full bundle notices/source. Older features deliberately absent |
| libmpv client header | Checked-in JNI API header | ISC, preserve header notice |
| Browser fallback fonts | Noto script fonts, Droid Sans Fallback, embedded Liberation Sans | Noto/Liberation SIL OFL 1.1; Droid Apache 2.0. Pins/package origins recorded. Korean fallback absent in browser bundle |
| Server burn-in fallback fonts | Noto CJK/Thai/Devanagari alongside existing DejaVu | Noto SIL OFL, embedded licences; DejaVu's existing font notice must travel. Font attachments retain bounds |

FFmpeg was already a server dependency. Existing React Native/Media3/Google Cast/graphics/browser dependencies were not replaced wholesale by this programme. Packaging/docs added or corrected existing Manrope OFL and Lucide ISC notices. Broad tags, bitmap decoders, queue/freshness logic, MCP, watcher and relay work were written without adding a new framework dependency. The public docs lane inventories 234 npm package/version/licence rows across the server lockfiles; that does not establish the complete native binary graph. Published release artifacts remain the authority for their own dependency bill.

## Appendix D. Reconciled original bug register

This is a reproducible reading view of reports, not a new audit. All 588 IDs are accounted for. It starts from every report's Bugs section and the fix log, prefers the three remaining-bug owning reports, excludes unrelated out-of-area statements as completion evidence, and explicitly adjudicates compound/partial wording. A source check overrides one contradiction: B-18.13 is fixed by the actual selection-bar preview/typed-confirmation dialog, despite a remaining-bug report calling it an intentional no-confirmation flow.

Open includes partial source fixes, unimplemented client consumers, unresolved incidents and explicit acceptance/design risks. Fixed and already-fixed entries still need their environment acceptance. B-02.16 remains open for the missing owner screen; B-08.12 remains open for response-composition cost. Completed source repairs are not downgraded solely because a physical device has not been run, but findings explicitly about test/release coverage remain open.

| ID | Reviewed severity | Original claim | Current gap / owning report |
|---|---|---|---|
| B-01.23 | Medium | Related Backlog item: two Apple sign-ins ended with explicit_signout | Not fixed: historical simulator incident has no correlated request trace or established cause. V01 refutes the guessed single-profile-cleanup bug and the current native source gates cleanup on accountGrant. Added/tested… [leftovers-server-a](../../../../_program/reports/leftovers-server-a.md) |
| B-02.16 | Low | Route-failure memory: risks of the design | Server/core failure list and clear APIs exist; owner console visibility/clear screen remains unfinished. [leftovers-server-a](../../../../_program/reports/leftovers-server-a.md) |
| B-04.35 | Medium | "Complete" scanning is far heavier than its label suggests (design risk) | Complete default no longer runs checksum/fingerprint passes; shared-decode optimization remains unfinished. [leftovers-server-a](../../../../_program/reports/leftovers-server-a.md) |
| B-07.10 | Low | The client's letter-bucket match disagrees with the server's letters | Not fixed here: client bucket matching needs the authoritative sort key/language contract from search-sort. [freshness-server](../../../../_program/reports/freshness-server.md) |
| B-08.12 | Low | If-None-Match, ETag and encoding nits | Encoding negotiation and compressed validator correctness fixed; conditional validators still require response composition. [networking](../../../../_program/reports/networking.md) |
| B-10.11 | Medium | A bitrate limit on an Apple device turns the audio plan off | Not fixed here — server AAC converted target dependency; no server writes in this lane. [apple-native-review](../../../../_program/reports/apple-native-review.md) |
| B-10.28 | Medium | The gapless fixtures leave whole classes of file untested (design risk) | Not fixed end to end — real HE-AAC and physical codec/output matrix still required. [apple-native-review](../../../../_program/reports/apple-native-review.md) |
| B-11.8 | High | HDHomeRun sources cannot have a guide, so they cannot record | Backend fixed HDHomeRun guide mapping by XMLTV ID, number or display-name; owner form needs guide URL/mapping controls. [livetv](../../../../_program/reports/livetv.md) |
| B-11.27 | Medium | The pre-emption countdown is not delivered | Backend fixed: session responses expose reservationRequiredAtMs and profile-scoped channel.reservation-required events carry it for every affected shared-channel viewer. Duplicate retries do not repeat events. Client countdown… [livetv](../../../../_program/reports/livetv.md) |
| B-13.10 | Medium | Cost grows with the square of the group size | Hot projection/event paths improved; no ceiling-scale benchmark establishes elimination of group-size cost. [social](../../../../_program/reports/social.md) |
| B-13.18 | Low | Loading a title in a paused group plays it for a moment | Not finished in screens/engine adapters. Shared GroupPlayer.load already passes desired initial playing state, but web/src/app/together.tsx ignores it; native initial preparation also needs a guaranteed paused engine start.… [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |
| B-14.14 | Medium | Files of a signed-out account or another profile stay on the device, invisible and unremovable | Other-profile reclaim and optional removal at sign-out exist; retained files/device-wide scope policy and physical cleanup acceptance remain. [native-sync](../../../../_program/reports/native-sync.md) |
| B-14.20 | Medium | Android transfer lifetime | Not finished in the native platform. Durable server requests and DownloadsService.followDevice already recover claims on the next app run; the downloads lane reports Wi-Fi/retry/service-timeout improvements. Automatic recovery… [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |
| B-14.21 | Low | Clocks | Partly fixed. Server future timestamps are clamped before comparison/history/watermark writes; a regression proves they cannot replace later online resume. Device receipt expiry still uses its wall clock through receiptUsable;… [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |
| B-15.19 | Low | Supply-chain loose ends | Partly fixed. Remaining tagged checkout/setup-go uses are pinned; Docker base manifests are digest-pinned after hash verification against Docker Registry responses; pinned govulncheck and npm vulnerability jobs added. Release… [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |
| B-16.4 | Low | WindowedGrid's mapped-scroll mode has discontinuities near the ends | Not fixed here: web/src/ui/WindowedGrid.tsx update still snaps at maxScrolled while small deltas move 1:1. Frontend owner must smooth the compressed end ranges. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.5 | Low | WindowedGrid re-creates its ResizeObserver and forces layout on every row change; inline renderers defeat per-page memoisation | Already partly fixed: WindowedGrid measurement subscription is stable; Library.tsx EngineResults inline cell renderers still need frontend memoisation. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.7 | Medium | Each open tab holds a long-poll; on plain HTTP several tabs can starve the connection pool | Not fixed here: plain-HTTP visible tabs can still own separate event requests. Web player/bridge needs an IndexedDB-coordinated feed owner or an equivalent HTTP-capable shared transport; secure-only sharing is insufficient. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.10 | Medium | Strict parsers reject whole documents for unknown enum values | Already partly fixed: home/library presentation parsing has tolerant row handling. Older-client/newer-server compatibility is not a pre-release obligation; retain strict identity/security validation. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.12 | Low | No declared browser floor; older engines degrade silently | Not fully fixed: web card focus fallback exists, but web/vite.config.ts still has no build.target/browser floor. Frontend/packaging owner must declare the supported engines and test CSS features. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.14 | Medium | Every page has the same document title; navigation is not announced | Already partly fixed: route titles/announcements exist. The previously reported rapid-heading and identical-heading announcement cases need web navigation acceptance, outside native non-screen scope. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.17 | Low | Smaller accessibility defects in primitives | Already partly fixed: Field help association and named options are present. Pixel type sizes and forced-colour styling are frontend work in web/src/ui/Field.tsx, Tabs.tsx, tokens.css and base.css. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.18 | Low | The strict style CSP probably blocks the scroll-lock stylesheet that Radix dialogs inject | Not fixed here: web/src/ui/Dialog.tsx still uses Radix scroll locking; server CSP still has style-src self. Packaging/web owner must make the lock work under that CSP without broadly allowing arbitrary style injection. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.23 | Low | React hook dependency rules are not enforced by any tool | Not fixed here: web/package.json has no hook-rule enforcement. This is web tooling outside the permitted native/shared-code area; adding a new lint dependency also needs the programme dependency decision. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-16.25 | Low | CI and the shipped image build the web with different Node versions | Not fixed here: server Dockerfile uses Node 24 while server CI specifies 22.22.2. Packaging owner must align and pin both; this lane does not own server build wiring. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-17.19 | Medium | Media keys and lock-screen controls for music depend on Web Audio counting as "media" | Not fixed in this lane: client UI/player/device behavior is outside this independent server/shared protocol review. [review-playback-core](../../../../_program/reports/review-playback-core.md) |
| B-18.4 | Medium | The Music › Genres tab and the Audiobooks › Series tab stop at 200 entries | Not fixed here: web LibraryViews GenresGrid still asks for one limit-200 page; browse aggregate/client paging must be reconciled by server and frontend owners. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-18.9 | Medium | The "Administrator" role can be granted but gives nothing in the web app | Not fixed here: selectable admin tier still differs from exact-owner web management gates. Accounts/server and web owners must expose the actual delegated permissions in web/src/app/session.tsx and screens/server/People.tsx. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-18.22 | Low | English text written straight into components | Already partly fixed: web catalogue coverage improved. Metadata fallback field labels and any remaining player copy belong to their frontend owners; no screen text invented here. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-18.23 | Low | Disc headings depend on an English phrase inside a subtitle | Not fixed in the web consumer: library-content already carries discNumber, but Entity.tsx album grouping still calls discNumberFromSubtitle(entry.subtitle). Frontend must use entry.discNumber and remove that English parsing path. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-18.27 | Low | Parts that exist but cannot be reached or do nothing | Not fully fixed: dead catalogue cleanup exists, but pinned resource controls, provider API-key applicability, remembered-device handler and registry-only choices require their owning screens/server pieces. See exact handoffs… [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-18.29 | Low | Removing an entry from a playlist or collection is immediate, with no Undo | Not fixed here: web/screens/saved/Resource.tsx immediate entry removal still needs a visible Undo action and an occurrence-safe restore operation. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-18.30 | Low | The episode panel loses facts on a direct visit and does not link people | Not fixed here: web/screens/detail/EpisodePanel.tsx needs server-backed air date on direct visits and navigable guest/director/writer entries. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-19.8 | Low | The Android TV density override has gaps and wide side effects | Not fixed: TelevisionDisplay still relies on RN internal flags and process-level density; rotation/font-scale behaviour needs Android TV hardware. The detection change here also supplies the same TV hint to density setup, but… [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-19.10 | Medium | A television that does not report "television" gets a touch layout with no visible focus | Partly fixed here: a Fire TV flavor or Leanback feature supplies a TV hint before the app loads, and TelevisionDisplay uses the same hint. Generic boxes that declare neither television UI mode nor Leanback cannot safely be… [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-19.15 | Low | The test harness leans on a React internal, and native tests are not run | Partly fixed here: hook driver fails explicitly when the expected React dispatcher seam is absent; native test config is checked in CI; macOS CI compiles/runs actual URL, download-owner and installation-storage contract tests.… [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-19.19 | Low | Small contract leftovers | Partly fixed here: PiP uses the exported runtime capability, including Android API/feature detection; Vega stays false. Unmounted native search keyboard and inert Android adjustResize are exact frontend/native followups, not… [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-21.6 | Medium | Published native preferences can be consumed or stored without a reachable control | Partly fixed here: shared settings structure exposes search.rememberHistory and notifications.badges on native, which already consume them. cardSizePercent/showBackdrops consumers still need frontend work; do not remove… [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-21.8 | Medium | Playing a duplicate playlist occurrence starts the first matching item | Not fixed: PlaylistScreen still sends an item-id container anchor. Existing QueueAnchor.position has no playlist revision/occurrence fence, so using a loaded-page index would be wrong after authorization filtering or a… [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-21.9 | Medium | Pre-play subtitles omit unknown-language tracks and merge distinct same-language choices | Not fixed: PlaybackChoices still collapses subtitle languages and omits unknown language; PendingTrackChoice still accepts language/resource id only. Frontend must list catalogued resources/discoveries by source and stable… [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-21.11 | Medium | Saved View Play and Shuffle only act on the loaded prefix | Partly fixed on base: Saved view gathers up to 500 titles before playing, avoiding merely the loaded page. It is still not a full server query selector for larger views; no claim of whole-view playback beyond the bound. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-22.19 | High | Vega rejects redirects after credential transmission can occur | Credential leak contained before SDK dispatch; authenticated Vega sign-in/restore/control/streams remain unavailable pending native transport. [leftovers-native](../../../../_program/reports/leftovers-native.md) |
| B-25.10 | Medium | Apple/Vega and physical native behavior are not automated release gates | Not finished: Apple/Vega release-build and physical acceptance gates are in the native repository, outside writable scope. Native CI's source omission was confirmed read-only. Existing bridge tests do not prove device… [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |
| B-25.12 | Medium | GPL Apple distribution rights remain unresolved | Skipped as instructed: component-specific Apple distribution rights/licence review is not code and cannot be settled by a runtime test. [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |
| B-25.21 | Medium | Cross-repo CI checks moving default branches, not recorded dependency revisions | Not finished: native and Roku sibling checkouts still omit dependency refs, confirmed in read-only source. Those workflows and dependency locks are outside writable scope; no server-side dispatch pretending to validate a… [leftovers-server-b](../../../../_program/reports/leftovers-server-b.md) |

### All IDs by domain and disposition

F = reported fixed; A = already fixed on the base; N = reported not present; I = intentional under current decisions; O = open/partial. Numbers after each domain are that domain's bug suffixes, so `01: 1` means B-01.1. This compact ledger preserves every ID without 588 repetitive paragraphs.

**01.** F: 1, 2, 3, 4, 6, 7, 8, 10, 11, 12, 13, 15, 16, 17, 18, 19, 20, 21, 22; A: 5, 9, 14; N: —; I: —; O: 23.

**02.** F: 1, 2, 3, 4, 5, 7, 9, 10, 11, 13, 14, 15, 18, 19, 20, 21, 22; A: 6, 12, 17; N: —; I: 8; O: 16.

**03.** F: 2, 8, 9, 10, 11, 12, 13, 14, 16, 18, 19; A: 1, 3, 4, 5, 6, 15; N: 7, 17; I: —; O: —.

**04.** F: 1, 2, 3, 4, 5, 6, 7, 8, 9, 11, 12, 13, 14, 17, 18, 19, 20, 21, 22, 24, 25, 26, 27, 28, 29, 30, 33, 34, 36, 37, 38, 40; A: 10, 15, 16, 23, 31, 32, 39; N: —; I: —; O: 35.

**05.** F: 2, 3, 4, 6, 7, 8, 9, 10, 11, 13, 14, 15, 16, 17, 18, 20, 21, 23, 24, 25, 26, 27, 29, 32, 33; A: 1, 5, 12, 19, 22, 28, 30, 31; N: —; I: —; O: —.

**06.** F: 1, 2, 4, 7, 8, 9, 15, 21, 24; A: 3, 5, 6, 10, 11, 12, 13, 14, 16, 17, 18, 19, 20, 22, 23; N: —; I: —; O: —.

**07.** F: 1, 2, 3, 4, 6, 7, 9, 11, 12, 13, 14, 15, 16, 17; A: 5, 8, 18; N: —; I: 19; O: 10.

**08.** F: 1, 2, 3, 4, 5, 6, 7, 8, 11, 13, 14, 16, 18; A: 10, 15, 17; N: 9; I: —; O: 12.

**09.** F: 1, 2, 4, 5, 7, 8, 9, 10, 12, 13, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 28, 29, 30, 31, 32, 34, 35; A: 3, 6, 11, 14, 15, 16, 27; N: 33; I: —; O: —.

**10.** F: 1, 2, 3, 4, 6, 7, 8, 9, 10, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 29, 30, 32, 33, 34; A: 5; N: 31; I: —; O: 11, 28.

**11.** F: 3, 4, 5, 6, 7, 9, 10, 11, 13, 14, 15, 16, 18, 19, 20, 23, 24, 25, 26, 28, 29, 30, 32, 33, 34; A: 1, 2, 12, 17, 21, 22; N: 31; I: 35; O: 8, 27.

**12.** F: 1, 2, 3, 4, 5, 6, 8, 9, 11, 12, 13, 14, 15, 16, 18, 19, 20, 21, 23, 24, 25, 26; A: 7, 10, 17, 22; N: —; I: —; O: —.

**13.** F: 1, 4, 5, 6, 8, 9, 11, 12, 13, 14, 15, 16, 17, 19, 20, 21, 22, 23; A: 2, 3, 7; N: —; I: —; O: 10, 18.

**14.** F: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 12, 13, 15, 16, 17, 18, 19, 22, 23, 24, 25, 26, 27; A: 11; N: —; I: —; O: 14, 20, 21.

**15.** F: 1, 3, 4, 5, 6, 8, 11, 12, 15, 16, 17, 18, 21, 23; A: 2, 10, 14, 20, 24; N: 7; I: 9, 13, 22; O: 19.

**16.** F: —; A: 1, 2, 3, 6, 8, 9, 11, 13, 15, 16, 19, 20, 21, 22, 24; N: —; I: —; O: 4, 5, 7, 10, 12, 14, 17, 18, 23, 25.

**17.** F: 1, 2, 3, 4, 5, 6, 7, 8, 10, 11, 12, 13, 14, 15, 16, 17, 18, 20, 21, 22, 23, 24, 25, 26; A: 9; N: —; I: —; O: 19.

**18.** F: 13; A: 1, 2, 3, 5, 6, 7, 10, 11, 12, 14, 15, 16, 17, 18, 19, 20, 21, 24, 25, 26, 28; N: 8; I: —; O: 4, 9, 22, 23, 27, 29, 30.

**19.** F: 9, 12, 14, 16, 18; A: 1, 2, 3, 4, 5, 6, 7, 13; N: 17; I: 11; O: 8, 10, 15, 19.

**20.** F: 1, 2, 4, 5, 6, 7, 9, 10, 11, 12, 14, 15, 17, 18, 19, 20, 21, 22, 23; A: 3, 8, 13, 16; N: —; I: —; O: —.

**21.** F: —; A: 1, 2, 3, 4, 5, 7, 10, 12, 13, 14, 16; N: —; I: 15; O: 6, 8, 9, 11.

**22.** F: 15, 22; A: 1, 2, 3, 4, 5, 7, 8, 9, 10, 11, 12, 13, 14, 16, 17, 18, 20, 23, 24, 25, 26; N: —; I: 6, 21; O: 19.

**25.** F: 2, 5, 6, 7, 9, 11, 13, 14, 15, 16, 17, 18; A: 1, 3, 4, 8, 19, 20; N: —; I: —; O: 10, 12, 21.


## Appendix E. Remaining choices and their alternatives

These complete section 3. They are implementation choices, not extra owner decisions.

### Credentials, privacy and recovery

- **Live permission edits keep sessions alive.** Disable/delete, password replacement and PIN changes still end affected sessions. Clients quietly refresh live roles. New-device approval exempts the owner but not administrators. The alternative is broad sign-out on each policy edit.
- **A secondary profile cannot manage account devices/remembered sign-ins, but may still approve a TV with its own role.** TV approval follows your explicit role decision. The alternative is primary-profile-only TV approval.
- **Remote policy is evaluated at new sign-in using the receiving device's locality.** Renewal/profile selection are not new sign-ins; allowed hours still apply. Under Owner only, valid and invalid member passwords remain distinguishable by response. The alternative leaks the owner's username by refusing before password verification, or uses a broader uniform response redesign.
- **Allowed schedules without a timezone use server time.** Outside-hours refusals preserve credentials. The alternative is UTC schedules or immediate session revocation.
- **Crash reports are captured locally and sent only on request.** Owner reports are bounded/redacted. The alternative is automatic server upload; none was added.
- **Root revocations are permanent learned facts.** Returning to a revoked signing key is not valid rollback. Missed fleet delivery follows existing daily/weekly check-ins, not the Hosted process's 30-second fence. The alternative would need new fleet delivery guarantees.
### Library and personal state

- **Retained personal state is bounded at 100,000 removed titles per profile, oldest first, without a time limit.** It includes watched/progress/rating/favourite/My List/count, not every per-play row or playlist occurrence. The alternative is a larger/unbounded archive or a timed expiry.
- **Delete means physical unlink, with approved-set checks.** Bulk file deletion has preview and typed confirmation; a changed/shared file can be kept. A container action covers at most 5,000 titles, selection requests 200. “Delete but keep files” still permits a later scan to return the title. The alternative is removing the keep-files action or a durable large-job design. No catalogue trash folder remains.
- **Anime alternate order uses explicit signed offsets.** Local number plus offset maps to an owner-selected provider order; seasonal files keep base order. Ambiguity becomes Needs order for that file. The alternative is guessed offsets, with wrong-match risk.
- **Tag lists retain genuine artists and punctuation.** Album release IDs win; fallback is album artist/title/year. Generic tags are too weak to merge audio. A matching TXT sidecar is lyrics unless recognized as a rip log. The alternative is heuristic artist splitting or narrower lyric suffixes.
- **Complete scan prepares useful navigation/audio features, without default checksum/fingerprint diagnostics.** Custom retains those operations. Disc folders are represented truthfully but are unplayable. The alternative is heavier Complete or actual disc-playback development.
- **Mark container is atomic and has a 30-second Undo.** It resets member resume and completion intent; Undo refuses later edits/playback/membership changes. Sustained unfinished rewatch clears watched until completion. The alternative is preserving old member exceptions or changing rewatch semantics.
- **Credits completion falls back to the chosen percentage without an approved current marker.** Early known-duration EOF preserves resume; unknown-duration natural end remains accepted. The alternative is requiring markers or rejecting unknown-duration terminal evidence.
- **Books complete within 30 seconds of end, capped at 5% for short files; resume is retained.** Resume rewinds 15 seconds after five minutes away, 30 after an hour. Explicit seeks stay explicit. The alternative is completion percentages or no comfort rewind.
- **Popular requires three eligible, opted-in viewers.** Recommendation day uses server local date. Continue limits use the largest participating library maximum; film resume does not age out; show detail ignores Home dismissal/age. The alternative is different popularity/privacy/day/continuation rules.
- **Filters apply live and Back keeps them.** Tab choices are remembered per viewer/server/device; explicit links do not overwrite memory. An old browser tab does not silently follow another profile. The alternative is draft filters or automatic cross-tab profile switching.
- **Only one sort is accepted per browse request.** Random is a stable seeded index rotation; file size/resolution and certain container metrics use member maxima. Valued personal rows precede unvalued title-order rows. Aggregate “Recently added” means title addition, not private viewing. The alternative is new compound/query-specific maintained orders.
### Playback, rendering and licensing

- **Fixed quality is a ceiling; Original can use a quality-mode conversion.** Current ordinary planner rates are the governing ladder. A member cap lifted later can widen the next plan; lower limits always apply. Versions are compared for least processing, then resolution/bitrate, with at most eight compared. The alternative is forced conversion, fixed historical session limits or wider version enumeration.
- **The later decision pass corrects two earlier choices.** Automatic must start at Original even with a throughput estimate, and profile-wide PIN lockout contradicts your no-lockout policy. Both are reported corrected in that lane, pending final integration. Existing per-attempt/input admission bounds remain; the alternative is retaining behavior that conflicts with your direction.
- **Automatic is client-driven.** It reacts to sustained shortfall/stalls and cautious recovery, with a 15-second change gap and increasing backoff after a failed upgrade. Direct files react mainly to stalls. A low-throughput refusal can be retried without the estimate. The alternative is server probing or more aggressive adaptation.
- **Unknown analyzed-file bitrate uses a conservative resolution estimate.** Unknown output routes never enable passthrough. Software decoding tiers are estimates. The alternative is refusing more media until measured facts exist; actual hardware limits still need calibration.
- **No approximate Dolby Vision conversion.** A compatible base layer may be used; no-base profile 5 requires qualified reshaping. Lossless audio output is preferred where accepted, even from a lossy source, to avoid another lossy encode. The alternative is broader but degraded fallback.
- **Remember failures per engine/file/device.** Definite delivery faults last seven days; generic faults six hours. Owner read/clear APIs exist but need screens. The alternative is shorter/no memory or automatic broader retry.
- **Native platform players declare no embedded audio switching.** Track changes use session re-plan so choices are remembered; mpv server-audio changes can reload briefly. Player-setting changes may create a new session on the same queue entry. The alternative needs exact choice/settings APIs that retain all session options without reload.
- **Native mpv network reads use system-validated relays.** Grant-scoped reads, bounded HLS rewriting and no redirects keep credentials away from mpv's own TLS stack. Engine release waits for actual teardown rather than retaining an idle handle. The alternative is a materially different native transport/ownership design.
- **Android HDR direct output gives way to SDR when mpv must draw styled/image subtitles.** Phones/TVs use different decoder paths; frame-rate switching follows system policy. Mixed-script/untagged fallback fonts remain limited. The alternative is an upgraded rendering/font build; physical HDR validation comes first.
- **Web styled subtitles render at at most 1440 lines, on a canvas capped at 4K.** They keep authored position and use a device-local vertical adjustment. iPhone fullscreen cannot carry that overlay. PiP/casting do not yet switch subtitle mode. The alternative is a lower cap, full-resolution cost or complete mode-switch integration.
- **Web bitmap support is bounded and partial.** DVB character/progressive objects and VobSub in-cue colour changes are omitted; forced flags have no viewer-only filter control. Browser Korean fallback is absent. The alternative is expanded parsers/fonts and real corpus tests.
- **Web seeks are local and one video element survives title changes.** iPhone uses native HLS; Mac/iPad Safari use hls.js first. Frame step assumes 24 fps; End goes five seconds before end. Up next during credits is an offer, never automatic playback. The alternative is broader native-HLS/AirPlay support or new source-rate/control semantics.
- **Normalization never guesses missing gain or peak.** Unity when unavailable; album falls back to track; amplification requires peak and caps at 32×. Unindexed lossy deep web seeks fall back to ordinary playback, losing optional effects. The alternative is more analysis/indexing rather than inaccurate sample claims.
### Operations, integrations and shared media

- **Timeshift stays at 30 minutes/1 GiB and reports actual span.** High bitrate yields less rewind. New live readers start at the current input chunk. The alternative is owner-adjustable retention/history replay with extra resource work.
- **Guide overflow skips excess rows; oversized documents are refused.** Large guides spool to private SQLite; selected groups/channels filter before staging. The alternative is larger limits after load qualification, not removing hostile-input bounds.
- **Download receipts renew daily.** Online revocation ordinarily reaches a device within a day; disconnected copies rely on their receipt lifetime. Transfer addresses expire after 24 idle hours and can renew within seven days. Original files are hashed once then checked by size/mtime; device digest catches hidden mutation. The alternative is more frequent renewal or copying originals.
- **Multipart titles cannot become one download; one readable version can.** Unfetched claims expire after the existing 30-day default, freeing server claims rather than device files. No automatic Android restart while process-dead was delivered. Device receipt expiry remains wall-clock based. The alternative is bundle-of-parts, JobScheduler and a persisted trusted monotonic clock.
- **Watch Together waits up to five seconds with a 750-ms readiness band.** Play can bypass the wait; buffering stays local; private participation is anonymous but occupies a visible room slot. Non-hosts leave after ten minutes; connected ready members may claim an absent host after two. The alternative is a more elaborate room redesign, outside scope.
- **Cast recovery is installation-local and same-profile.** A sender gets a private token; duplicate TV names still need better proof-storage identity. The alternative is a stable receiver-ID store or new cross-profile control policy.
- **Four integration scopes remain flat.** Admin covers everything allowed to keys; playback includes personal media state/DVR, library can report changed paths without catalogue reads. Keys use ordinary connections rather than the console reserve. URLs are server-visible paths with no built-in mount mapping. The late security sweep narrows cross-member owner transport from playback to admin scope while leaving own-profile controls at playback; that fix awaits final integration. The alternative is more scopes or mapping configuration.
- **Webhooks allow private destinations, never redirects and never auto-disable a failing destination.** Each has bounded queue/logs; full queues drop new events. Title-added waits about 45 seconds for metadata; private recording libraries are excluded from title events. Some container-only metadata changes still lack title-updated events. The alternative is different event coverage/backpressure policy.
- **MCP calls the same API with chosen credentials.** It has no enable switch and accepts keys/tokens only. Some control/attention tools were added by follow-ups; no claim of arbitrary absent backend operations. The alternative is a separately authorized protocol surface, which would conflict with one-API simplicity.
- **Recovery serves a plain HTTP status page while database/TLS settings are unavailable.** It offers no unauthenticated repair. An invalid custom FFmpeg keeps console access and falls back to default tools, with requested/actual status. The alternative is no recovery listener or stopping all service on tool rejection.
- **Update notices use the official feed with opt-out; custom feeds may be private.** Network policy pins DNS and confines redirects. No signing identity is generated; release artifacts need an approved pinned key and final inventory. The alternative is manual-only update checks or a different signer ceremony.
- **The UI groups Integrations on one page and hides owner personal-token creation in favour of keys.** Expiry has named choices; merge uses a two-title selection; deletion uses server preview; storage locations are read-only; Undo currently belongs to the title page. The alternative is different screen placement/control affordances during frontend completion.


- **Optional spoiler controls and writing metadata beside media were not introduced.** The owner had not approved those optional proposals; the later decision matrix records conservative deferral. The alternative is an explicit product choice followed by new projection/storage/control work. This is separate from the requested completed library/provider flows.

### Additional bounded defaults and small consumer choices

- **Integrations have fixed count/lifetime caps:** 100 keys/server, 25 tokens/member, ten-year expiry horizon, 20 webhooks, 1,000 pending deliveries and 200 log entries/destination. Credentials default to four ordinary plus two long-lived requests in flight. The alternative is owner-adjustable larger caps after resource qualification.
- **Provider pace is deliberately conservative.** Gentle/Normal/Fast use bounded request and title concurrency; MusicBrainz stays at one request/second and AniList stays below its documented allowance. Retryable outages eventually back off from 15 minutes to six hours, respecting Retry-After; authentication/validation failures wait for correction. The alternative is different measured provider budgets, not unrestricted retry.
- **Owner diagnostics are bounded rather than complete exports.** Client reports cap at 100 total/five per device, 256 KiB/report and 1 MiB/account/day. Conversion failures keep the newest 200 for 30 days; catalogue rebuild failures show the latest 100 with a More indication. The alternative is pagination/export and another retention policy.
- **Scan bookkeeping keeps twenty summaries/source and 10,000 changes or 30 days.** Watchers coalesce after two quiet seconds/ten continuous seconds, with 64 roots and 4,096 directory watches. Failed/network watches fall back every five minutes; local safety inventory every fifteen. Expired change cursors require rereading. The alternative is different bounds/cadence after platform/load tests.
- **File List Only may read ignore configuration.** It still avoids media/local metadata analysis. Owner ignore changes can make old entries missing; unreadable/changing evidence protects only its affected subtree. Extras need one unambiguous owner and retain no automatic owner beyond bounded evidence. The alternative is stricter configuration-only intake or an explicit owner selection.
- **Optional visual analysis tolerates up to 5% decode errors.** Exact audio/loudness/fingerprint paths remain strict. Source changes discard results; qualified Dolby Vision is required. The alternative is abandoning optional images sooner or relaxing exact audio accuracy, which was not done.
- **Subtitle cold fills are shared and bounded.** Two fills run concurrently, 64 can be in flight; first-reader cancellation does not cancel shared extraction. Original/font attachment names never become paths. Provider language falls back through the viewer's list then English; bad credentials stop retry. The alternative is eager extraction, different concurrency or explicit provider language controls.
- **The native reinstall reset is exact and protected-data-aware.** It deletes only six Portico Keychain services when an installation marker is absent; established locked installations keep ordinary background behavior. Unreadable protected state waits for retry rather than erasing itself. The alternative is broader first-launch deletion, which risks real background sessions.
- **Kept navigation history freezes rather than being discarded.** Retained-history memory is not thereby bounded. Runtime Fire/Leanback facts decide TV layout; generic large-screen guesses are not used. Shipped regional English keeps existing plural fallback; more languages need real Hermes/Vega qualification. The alternative is bounded history, explicit layout choice and a separately qualified language expansion.
- **Invitation links can wait thirty minutes while signed out.** Other held links keep ten minutes; duplicate-delivery windows are five seconds or ten for initial launch. Key-loss reset asks first and keeps downloads; after reset JavaScript reloads. The alternative is shorter links or automatic destructive recovery.
- **Native network admission reserves control within 76 requests and ten active reads.** Apple uses separate socket pools; Android uses separate dispatchers and one-shot writes. Ordinary reads/changes/console requests have distinct deadlines including waiting time; deliberate streams retain their own lifetimes. The alternative is another bounded admission model after real socket-load tests.
- **Presentation parsing skips only descriptive units.** Unknown authority/identity/cursor/revision/limits and uncertain write receipts remain strict. Download unknown quality names remain readable; an unnamed claim remains listed rather than hidden. The alternative is whole-document rejection or dangerously guessed authority.
- **New restricted generations may show protected empty content.** Old readable content stays during ordinary rebuild; unpublished restricted anchors are omitted. Expired signed cursors become scoped positioning hints. Kept documents are memory-only, failures wait fifteen seconds, hidden feeds stop after thirty seconds unless playing. The alternative is persisted snapshots or more polling, with privacy/resource costs.
- **Late stale settings fixes compare edited keys, not just revision numbers.** Unrelated changes may retry quietly; genuinely changed keys preserve the draft and conflict. Queued/late player operations are bound to their original session. The alternative is unsafe full-document replay, which is rejected by the review.
- **Optional/development protocol contracts remain separate.** Two development operations compile only in the development trust build; twelve retained consumer protocols do not add production handlers. Special subtitle operations retain explicitly named microseconds even though general media fields use milliseconds. The alternative is completing/retiring those protocols and a deliberate specialized-unit rewrite.
- **DVR picker continuation may return an empty page with a cursor.** It advances by examined source ordinal without disclosing denied IDs. Clients must continue until the cursor ends. The alternative is unbounded filtering work to fill every page, which would harm responsiveness.
- **Large owner notices walk fifty profiles/checkpoint and resume for fourteen days.** The starting last-profile bound excludes newly created profiles from that delivery promise. Profile download revocation walks at most 100 claims/receipts per call. The alternative is larger transactions or a new durable job API.
- **Live guide parsing retains narrow hard bounds even when document limits rise.** Manual preview/save retains a 48 MiB body; huge remote guides use streamed ingestion. Entity declarations remain refused; missing stops are inferred, overlaps trimmed and conflicting repeat flags prefer Repeat. Catch-up metadata is retained without catch-up playback. The alternative is new import/catch-up workflows, not lifting parser safety.
- **Software/output assumptions remain conservative.** Apple combines H.264 bit depths under one lower software ceiling when the document cannot express per-depth bounds. Android direct HDR is contingent on the actual surface/decoder path; television render resolution depends on the device tier. The alternative is richer per-depth/output facts and qualified native libraries.
- **Ordinary audio preserves known rates/depth and uses 48 kHz when unknown.** AC-3/E-AC-3 encoding stays at 5.1; compatible 7.1 copying or FLAC/AAC/Opus is preferred where admitted. Web decoder alignment is probed before admitting optional effects. The alternative is stronger codec qualification/indexes rather than guessed sample accuracy.
