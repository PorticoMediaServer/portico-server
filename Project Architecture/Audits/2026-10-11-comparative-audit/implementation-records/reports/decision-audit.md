# Outcome

Reviewed the 31 authoritative programme bullets and all 61 original decisions against this server checkout and the read-only main-native checkout. Fixed six cross-path gaps: Automatic starting below Original; Automatic rejecting an mpv remux; a profile-wide PIN lockout added by implementation; guide compatibility scans that enumerated the whole lineup; one malformed descriptive download-bundle row rejecting otherwise usable extras; and missing same-session capability re-planning. Also integrated three independently source-verified e2e-server repairs: preserve discovered ASS/SSA style through first cache fill, publish ordinary conversion occupancy as retryable 429, and return the actually scanned selected asset. Corrected a reference to a deleted API document. The three fixes were copied from e2e-server snapshot 004 after source/diff review; integrate once, not twice. This lane additionally declares the same capacity refusal on prepare-next and guards all five conversion-creating operation contracts.

The release is **not delivered end to end**. The server and canonical shared client carry the core decisions, but grouped-view controls and several owner screens are missing on clients. Vega's installed transport refuses authenticated operation, and platform/playback/capacity acceptance is still required. Neither old reports nor passing compiler checks are treated as device evidence.

Execution host: personal Ubuntu mini-1, ehler-homelab-mini-1. Starting tree was clean. Context-sync failed because its state lock is outside the writable sandbox; the canonical Portico, project standards, verification and attribution notes were read directly. No commits, deployment, push, install, full suite or helper agent was used.

## Reading the matrices

“Delivered” below means the observable source path was followed, not that physical devices or release packages were accepted. “Server only” includes shared wire consumers where those exist. A deliberate exclusion is delivered when the excluded surface is absent. “Partly” names its remaining boundary.

Paths in server cells are relative to server/internal unless cmd/server or packaging is named. Core cells are relative to packages/client-core/src. Web cells are relative to web/src. Native cells refer to /home/justin/projects/Portico/.worktrees/audit-implementation/main-native/apps; native shared-package copies must be refreshed from this lane before integration. Native phone/tablet/TV share the named screens; Vega inherits those screens but authenticated networking is blocked (V1 below).

## Original decision register

Every D-number is included, even where the owner did not explicitly answer it. Those rows name the conservative disposition rather than promoting the auditor's recommendation to an owner decision.

| Decision | Decided / disposition | Server package or route and effect | Shared client | Web app | Native apps | Verdict |
|---|---|---|---|---|---|---|
| D-01 | Separate title/files; retain state by identity | compactcatalog/identity.go, catalog/title_merge.go, retained_state.go: identity ledger, merge, restore on re-add; /v1/admin/titles/merge | title-identity.ts, detail.ts retain versions/missingSince | detail pages and server Libraries support merge/files | Detail/Show read availability; file-management tools incomplete | Partly: model delivered, native owner tools missing (N3) |
| D-02 | Smallest bad unit; retain safety bounds | ingestion/inventory.go, subtitles parsers, livechannels guide parsers, audiotags, localmetadata; issue records survive | tolerant.ts; library-content/search/guide; downloads.ts fixed descriptive bundle rows | server scan/attention diagnostics; partial lists remain | scan issues and tolerant package copies; local extras preserve valid rows after sync | Delivered in source; malformed envelope/authority still refused |
| D-03 | Both names; file and embedded evidence win | naming/naming.go and ingestion identification keep useful folder fallback and meaningful file authority | scan-issues.ts, media-item.ts read results | Libraries and title files show resulting identity | Detail/Show render resulting title/season | Delivered |
| D-04 | Native primary plus mpv; Automatic/Native/mpv | playback/engine_choice.go, capability document, /v1/playback/sessions: native direct/remux first; mpv copy/remux correction | playback-v1/legacy-bridge.ts carries presentation.engine | Browser engine, no mpv platform option | native/src/player/engine-choice.ts and bridge capability-document; Apple/Kotlin modules; no mpv on Vega | Partly: required engines wired in source; platform build/acceptance and engine limits remain |
| D-05 | One rich capability document | playback/capability_document.go, playbackv1/devices.go; /v1/me/devices/current/capabilities stores exact device document used by planner | playback-v1/capabilities.ts and device publisher | player profile publication and browser probes | bridge/capability-document.ts and capability-publisher.ts; native probes reach server | Delivered in source; measured device limits still required |
| D-06 | Local API, keys, webhook, MCP; no plugins | httpapi/api_credentials_routes.go, webhook_routes.go, mcp.go; scope/role checks; /v1/mcp dispatches API operations | api-access.ts, generated contracts | server/ApiAccess.tsx and Assistants.tsx | PeoplePanels only API keys; no tokens/webhooks/MCP panels | Partly: server/web delivered; native screens missing (N1) |
| D-07 | Hostname proxy, overlay, CGNAT, IPv4/6, four server OSes; no subpath/relay | networkpolicy/policy.go, networking/certificate_listener.go and cmd/server listeners, httpapi/connection_policy.go, packaging service files; TLS origin and proxy trust paths | server-address.ts, route-connection.ts, certificates.ts | auth accepts owner address; Network/Remote settings | native auth HTTP enabled; same address/route core; Vega blocked | Partly: source/platform packaging present; acceptance/Vega remain |
| D-08 | Push quietly, retain content, split semantic/observation counters | apievents/eventfeed; catalog retained browse/search slots, personal observation revisions; /v1/events | freshness-hub/pool/actions; bounded reread and merge | app/freshness.tsx; content/search/Show stay mounted | app freshness pools and retained screens; native card fade absent | Partly: core delivered; independent screen gaps and Vega remain |
| D-09 | One pre-release baseline; natural search/sort, reclamation, explicit units; 0–10 personal rating | sorttext, catalog/search.go, persistence pragmas/reclamation; RFC3339 and integer-ms ordinary boundaries; specialised subtitles retain explicit microseconds | wire-units.ts, browse.ts, rating parsers | library toolbar, Search, rating controls | same core parsers and rating controls | Delivered in source; special subtitle microseconds are a disclosed implementation choice |
| D-10 | Windows/macOS/Linux/Docker; listed clients; Roku outside programme | packaging Windows service/macOS daemon/Linux unit/Docker; storage platform admission | platform client profiles | browser application delivered | Apple/Android source plus Vega; physical set unqualified | Partly: no release/platform acceptance; Vega unavailable |
| D-11 | Feature complete only on all first-party surfaces; hide true impossibilities | API readiness cannot establish screen parity | settings-structure platform gates expose missing surfaces | Richest owner console; grouped views incomplete | Missing owner panels, grouped controls, offline lifecycle work | Not delivered: this matrix identifies parity gaps |
| D-12 | One shipped web playback/controller path | /v1/playback/sessions and queues | playback-v1/sessions.ts, v1-queue-player.ts; screen-facing PlaybackService retained | playback-v1/web-playback.ts, player engine; changes preserve queue | native controller uses same session/queue core | Delivered in source; standalone subtitle fallback still listed under P09 |
| D-13 | Remove old start/login routes | httpapi/foundation.go registers typed current routes; /v1/sessions and /v2 playback absent | current generated start/sign-in calls | current session start and direct identity | current direct auth and session starts | Delivered |
| D-14 | Browse/search always serve maintained prior content during rebuild | compactcatalog derive browse/search serving generations; catalog browse/search hydrate with current authority; no preparing read response | freshness actions keep list and refresh continuations | Library/Search retain document | Library/Home/Search retain document | Delivered in source; sustained load acceptance outstanding |
| D-15 | Tolerant confident matching plus attention list | metadata screen search/scoring/publish; verified score/margin; /v1/admin/metadata/attention and counts | metadata-attention.ts, metadata-repair.ts | server/NeedsAttention.tsx and MetadataLookup | MetadataEditor per title; no library-wide attention inbox | Partly: server/web delivered; native attention list missing (N3) |
| D-16 | Owner chooses one provider per library; TMDB video default, local-only | metadata/library_agent.go, screen policy and provider admission; local agent gates acquisition/search/artwork | metadata-settings.ts and library-management.ts | LibrarySettings and MetadataSettings | LibrariesPanel edits agent; provider-global credentials/attention panels absent | Partly: policy delivered; owner panel parity missing |
| D-17 | HEVC/HDR/multichannel outputs, tone map where needed | playback/conversion_targets.go and decoder rich targets; qualified HEVC/AV1, FLAC/Opus/AAC/surround; copied DV/base metadata | capability/session target fields and audio render | browser measured codecs shape plan | native measured decoders/output and mpv profiles; tvOS mpv SDR, Vega conservative SDR/stereo | Partly: converter/planner wired; physical HDR/audio acceptance, Vega and native limitations remain |
| D-18 | Discover/select without import ceremony; ASS/image device rendering, charset tolerance | subtitles inventory/cache/originals; download originals; burn with sealed bounded source/fonts | subtitles.ts, subtitle-originals.ts; session PATCH choices | player/subtitles libass/bitmap overlays; iPhone system fullscreen limited | mpv raw files; native text reader; local companion/fonts not fully passed | Partly: server delivered; offline companion/fonts and platform qualification (N6) |
| D-19 | Original default; opt-in Automatic starts Original then adapts | playbackv1/sessions.go corrected throughput start path; quality step PATCH retains occurrence | choices.ts AutomaticQuality; qualityRequest names explicit steps | player menu and settings home/remote quality | PlayerOptions/native controller measures then changes; permanent saved choices | Delivered in source after this lane; Vega runtime blocked |
| D-20 | Owner resource defaults adjustable; hostile-input bounds remain | operations/settings.go, medialimits, decoder, livechannels settings; runtime readers; reserve/control lanes | server-admin forms and settings structure | console resource controls and usage | ordinary form rows exist; resource usage/status panels web-only | Partly: server delivered; native usage panel parity and load acceptance |
| D-21 | Gentle/normal/fast background pace, provider politeness | workpolicy/dbwork background policy; metadataprovider/http.go provider pace; scan checkpoint readers | maintenance form backgroundSpeed | Schedule/maintenance speed dropdown | ServerForms maintenance row | Delivered in source; import throughput not measured |
| D-22 | Progress-aware I/O with independent hard safety limits | progressio transport; subtitles extraction; backup bounded checkpoints; shared live acquisition no wall-clock termination | bounded reads, playback retry deadlines | fragment/engine progress watchdog | download module partial identity/range work; iOS new URL can restart partial | Partly: source wired; iOS continuation/platform acceptance remains |
| D-23 | Wire inert settings; remove only impossible policy | operations setting_consumers guard plus actual runtime reads; temporaryDirectory, background cap, guide/DVR, history and started percentage | server-admin server forms; preference readers | console form consumers | native form rows and player settings; caption rows in player | Partly: settings wired; native usage/screens not all available |
| D-24 | Dark only; no themes or custom CSS; system accessibility | No theme/plugin routes | design/presentation fixed palettes | fixed dark UI, accessibility preferences | native system captions/text behavior; Vega subset | Delivered for exclusion; visual/accessibility device acceptance required |
| D-25 | Remember tab/sort/filter and per-title/show tracks | playbackv1/track_memory.go writes only successful choices; preferences | library-memory.ts per-tab and caller-driven links | app/library-memory.ts; player session choices | native library memory and successful track choices | Delivered; grouped-view memory/control handoff still required |
| D-26 | No owner answer; defer optional spoiler protection/new view modes | No spoiler preference/server projection policy exists | No spoiler-specific shared setting exists | No spoiler toggle or suppression | No spoiler toggle/suppression | Not delivered; spoiler protection **dropped by Justin 15 Oct** (view modes remain later work) |
| D-27 | Accurate scope labels; engine local, quality per class | operations preference scope definitions; no invented per-device storage | presentation/settings-structure scopes/help | settings badges use deviceClass | engine-setting-store local; saved home/remote quality class-scoped | Delivered in source; poster size on native remains screen gap (N9) |
| D-28 | Local owner webhooks; no push/email infrastructure | webhooks delivery worker plus database triggers; /v1/admin/webhooks; private addresses allowed, redirects refused | api-access.ts webhook client | ApiAccess create/edit/test/log | No webhook panels | Partly: server/web delivered; native panels missing |
| D-29 | No Jellyfin/Subsonic compatibility release layer | No compatibility handlers | No such client services | No compatibility UI | No such feature | Delivered exclusion |
| D-30 | No state import/export; tracker sync deferred | No import/export routes; retained local state is internal, not export | No transfer services | No transfer controls | No transfer controls | Delivered exclusion |
| D-31 | Own code; only agreed playback dependencies | Own audiotags/charset/parsers; existing x/text; FFmpeg | Own bounded consumers; existing libraries retained | Approved vendored libass WASM/fonts | Approved MPVKit/Android pinned libmpv and libass; existing platform libraries | Delivered in source; bundled licence/source obligations need release check |
| D-32 | Outside contributions deferred; no new contribution system | CONTRIBUTING and existing project policy | No contribution machinery | No translation contribution UI | No translation contribution UI | Delivered conservative exclusion |
| D-33 | One state folder by default; cache/backups relocatable; no trash directory | cmd/server reads PORTICO_CACHE_DIR/BACKUP_DIR; dedicated conversion child; delete unlinks original | storage-status.ts reports destinations | StorageStatus read-only places | No places panel | Partly: server/web delivered; native panel absent (N4) |
| D-34 | Self-restart on Windows/plain Docker, console action | supervisor/cmd/server and operations restart; restore schedules normal start | server-operations.ts restart action | Maintenance restart panel | No restart panel | Partly: server/web delivered; native action missing (N4) |
| D-35 | Relay future; existing multi-server chooser retained | Advertised direct routes; no relay protocol | known-servers/route-selection keep multiple servers | ServerSwitcher | native server chooser; Vega cannot authenticate | Partly: direct multi-server source delivered; Vega blocker |
| D-36 | Light passwords, no account lockout, HTTP, plaintext state, TV role; genuine defects fixed | identity credential retry buckets; removed global PIN lockout; backup scrub ends sign-ins but preserves integration keys; TV approver role | direct auth/PIN and restore APIs | auth forms and owner backup UI | native sign-in/profile/TV code | Delivered in source; integration-key restore resurrection is explicit trade-off |
| D-37 | Correct misleading device-key claim; no new proof-of-possession feature | bearer session families/device installation bindings, guide identity docs | credential-format/route identity retain actual signatures | No device-key promise | installation and refresh-binding state; not secret device keys | Delivered conservative choice |
| D-38 | Preserve open first-owner setup and same-origin controls; no public setup token | httpapi/setup_transport.go and request_locality.go and origin policy; JSON/native setup allowed from any address | setup-onboarding.ts | setup forms same-origin | native setup client | Delivered conservative choice; not DNS rebinding protection |
| D-39 | One age mapping engine; stricter account/profile intersection; no new kids-mode redesign | identity/ratings and contentaccess; multi-country spelling mapping; account denied labels/tags versus profile labels still differ | identity-restrictions.ts | People/profile limits | Profiles limits read/edit where authorized | Age rules delivered; country-specific tables, one label rule and per-library unrated **dropped by Justin 15 Oct** |
| D-40 | Permission edits/ownership apply live; revoke disabled access | identity local memberships/session-family live tier updates; api key triggers retain scoped current role | identity/restriction services and refresh | People edits and role dialog | Profiles/settings use current session; admin member tools incomplete | Delivered for server authority; device transitions need acceptance |
| D-41 | All stated media journeys at release; social defect fixes; car/Roku/light excluded | catalog, playback, listening, livechannels/dvr, downloads, recommendations and social APIs | Shared journey services | All journey screens, owner/tools gap list below | Native journeys exist; Vega unavailable; parity gaps | Partly: cannot call release ready |
| D-42 | One live input per channel; private per-profile DVR, one stored recording | linearinput shared pool; livechannels/dvr/shared.go and recordingmedia hard-link publication; per-owner authority | channel-guide.ts, dvr.ts keep private records | Channels/DVR and source controls | Channels/Recordings; source configuration extras incomplete | Delivered in server/source; physical tuner and filesystem sharing acceptance required |
| D-43 | Device copies survive tidy; bundle titles/art/subtitles; resume | downloads claims/receipts separated; bundle routes original/companion/fonts; source selected by best version | downloads.ts exact grants and tolerant descriptions; downloads-service.ts | Downloads manages server claims (no browser offline store) | offline-downloads stores bundle; player/offline-extras consumes chapters/markers/text; signed-out launch and rich subtitle companion/fonts incomplete | Partly: server delivered; native lifecycle/rich extras gaps (N6) |
| D-44 | Book near-end completion/resume/rewind; lyrics automatic; shuffle Play next; car out | listeningrules near-end 30s capped 5%; catalog audio tags/lyrics; playbackv1 shuffle permutation/queue edits | listening, audio-effects, queues, lyrics | player listening/lyrics/queue paths | native audio engines/lyrics/comfort rewind; AAC exact behavior unqualified | Delivered in source; acoustic/device acceptance still required |
| D-45 | Bounded wait and existing social controls, no redesign | social separate playback/governance/queue/settings fences; readiness max five seconds; eligible transfer | social-playback/group-session and cast controller recovery | Together and receiver routes | Together/GroupPill; Play on remote transport panel missing | Partly: social core delivered; remote controls/Vega/device acceptance |
| D-46 | No stable pre-release API; release API becomes v1; no compatibility promise now | Only current typed registry; old start routes removed | Removed guide 404/405 compatibility enumeration in this lane; optional external service contracts do not create server routes | Current core consumers | Needs native package refresh; copied old fallback survives until sync | Delivered in canonical code; sync required |
| D-47 | User-requested report to own server; no automatic external telemetry | servicelog/client reports: sanitized bounded storage, per-device/account limits/retention; /v1/diagnostics/client-logs | client-reports.ts | ClientReports owner list/read/delete and support reporting | SettingsPage Send asks before sending; captures crash locally | Partly: source present; native owner review panels absent and crash/platform qualification required |
| D-48 | Apple licence/distribution grant needs qualified owner/legal review | LICENSE and third-party notices; no invented grant | Not a wire feature | libass/FriBidi notices retained | Pinned LGPL bundles and notice/source obligations | Not delivered: legal/store submission acceptance outside this lane |
| D-49 | Keep checked shared-package copies and source pin | Canonical server packages; native scripts/shared-packages.mjs | Generated routes/types/builder; no independently edited copy | Canonical packages used directly | Read-only copied packages, revision/hash checks; this lane's changes need sync | Partly: copying pipeline exists; integration must refresh copies |
| D-50 | Paths-changed API and bounded native watcher with fallback | ingestion/targeted.go/reported_missing.go/watch helper; /v1/libraries/{id}/paths-changed; positive named removal proof, root checks | scan-issues/library-inventory.ts; external callers use generated route | Libraries scans/status | LibrariesPanel ordinary scans | Delivered in source; OS/network filesystem acceptance required |
| D-51 | Default hidden/sample exclusions and bounded ignore rules | naming ignore and ingestion directory config; reads .ignore/.plexignore; skipped rules/issues reported | scan-issues.ts | Libraries issue display | native scan issue display | Delivered in source |
| D-52 | No explicit owner answer; do not add write-beside-media option | No NFO/artwork/subtitle export writer or setting | No setting | No control | No control | Not delivered; **dropped by Justin 15 Oct** |
| D-53 | Remove library; keep personal identity state | catalog library removal/retirement releases roots; retained_state.go; owner delete preview/confirmation | library-management.ts | Libraries removal flow | LibrariesPanel removes folders only; no whole-library removal | Partly: server/web delivered; native removal missing (N3); large-library bounded retirement needs acceptance |
| D-54 | Upstream FFmpeg retained; owner qualified override | operations ffmpegPath restart setting; cmd/server qualification; sandbox and hardware grid probes | server-admin runtime form/check result | Conversion row/check/status | ordinary setting row, detailed check feedback incomplete | Partly: server delivered; native diagnostic parity and hardware qualification |
| D-55 | Short force-stereo/no-passthrough/no-HDR overrides | playback capability Overrides narrow planning | capability builder and preferences/settings controls | This device override switches republish document | SettingsPage/bridge capability publisher after saved prefs; Vega conservative restrictions | Delivered in source on capable platforms |
| D-56 | Bounded pre-upgrade recovery copy when numbered migrations apply | persistence migration backup and identity scrub; fresh baseline refuses old development digest rather than compatibility upgrade | storage/backup status consumers | Maintenance copies | BackupsPanel ordinary copies; local upgrade/recovery listing missing | Delivered source mechanism; release migration path still unqualified |
| D-57 | Default opt-out update notices; feed shape correct | operations updater/default feed; pinned HTTPS/private owner-selected origin; packaging manifest matches reader | server-operations/settings forms | Maintenance update switch | UpdatesPanel and runtime form | Delivered in source; actual signed release feed acceptance required |
| D-58 | Delete member/profile and blank names in owner history | identity/member_erasure.go exact owner/member boundaries; preview then deletion; device names also blanked | direct-account.ts deletion preview/client methods | People deletion preview/confirm | PeoplePanels edits but no member delete preview flow | Partly: server/web delivered; native deletion flow missing (N2) |
| D-59 | Say owner sees meaningful plays despite paused/cleared personal history | catalog activity preserves owner ledger independently | settings privacy help/catalogue | Personal/privacy help | Native personal settings use same catalogue sentence | Delivered in source |
| D-60 | Container watched mark includes progress; real Undo | catalog/container state snapshots members, 5000 bound and single-use 30s undo token/revision | container-state.ts | detail WatchedUndo; card-menu undo absent | Show/Entity mark but no Undo affordance | Partly: server delivered; card/native undo missing (W3/N7) |
| D-61 | Keep existing meaningful-rewatch semantics (owner did not replace contract) | catalog watched intent clears only after sustained unfinished rewatch; completion counts real plays | personal state reads progress/watched separately | Cards/detail show server state | Same state on native cards | Delivered conservative choice; owner should confirm against original recommendation |

## Authoritative programme bullets

P-numbers follow section 1's original bullet order. D references point to the concrete paths/effects in the preceding table.

| Decision | What was decided | Server | Shared client | Web | Native | Verdict |
|---|---|---|---|---|---|---|
| P01 | Plex tolerance and playback; API openness | naming/ingestion/metadata/playback, D02–06 | tolerant/session/keys | Library/Search/player/console | Same core, platform engines | Partly: parity/acceptance gaps |
| P02 | No plugins/themes/CSS; local API/keys/webhooks/MCP | api_credentials_routes/webhook_routes/mcp; no extension loader | api-access and registry | Integrations/Assistants | API keys only | Partly: N1; exclusions delivered |
| P03 | Allow chosen unsafe transport; quiet TLS guidance | request_locality/setup_transport accept HTTP; note route | connection-note/server-address | PlainHttpNote | ATS/arbitrary HTTP and Android; Vega transport blocker | Partly: HTTP delivered; Vega |
| P04 | Adjustable defaults; wire inert settings | operations settings/runtime readers, D20–23 | forms/settings model | Console forms | Rows exist, web-only panels | Partly: N1–N5/N9 |
| P05 | Reachable UI/console under load | dbwork/reserved sockets/bounded workers | deadlines/cancellation | retained documents | reserved control/read transport | Partly: source protections, unmeasured capacity |
| P06 | No push/email; own code except agreed media dependencies | local inbox/webhooks; own tags/charset | bounded owned consumers | approved libass WASM | approved mpv; no push integration | Delivered source/exclusions |
| P07 | All clients or hide true platform impossibilities | APIs alone insufficient | platform gates show omissions | richest owner surface | parity gaps; Vega unavailable | Not delivered end to end |
| P08 | Server-composed screens plus bounded query addition | catalog content/home/browse | projections/query builder | renders content | renders projections | Delivered architecture; grouped controls missing |
| P09 | No compatibility; current pre-release baseline | current registry, old login/start/YAML gone | removed guide compatibility enumeration | current v1; subtitle fallback remains | same fallback; sync needed | Partly: coordinated subtitle mutation cleanup remains |
| P10 | Separate title/files; retain identity/history | compactcatalog identities/links, catalog retention | title-identity/detail | merge/files/missing | availability; management gap | Partly: server delivered; N3 |
| P11 | Unlink disk; retain missing; root safety; identity restore | catalog missing/delete_title/retained_state; ingestion root guards | library management/title identity | typed delete/missing list | management incomplete | Partly: server delivered; N3 |
| P12 | Folder+file evidence; file/embedded wins; confident tolerance | naming/naming.go, ingestion identity, metadata scoring | published identity readers | title/season results | same results | Delivered source |
| P13 | Smallest bad unit; skips reported; hard bounds retained | parser/scan issue records | tolerant.ts; bundle fix | valid rows retained | same after package sync | Delivered inspected paths |
| P14 | Owner provider per library; TMDB video default/local-only | metadata/library_agent.go and provider admission | metadata/library services | LibrarySettings/MetadataSettings | agent row, global panels missing | Partly: N3 |
| P15 | No state import/export | No route | No service | No control | No control | Delivered exclusion |
| P16 | Wide common sort/filter everywhere | browse_contract surfaces/membership/indexed sorts | browseWithin/toolbar; Recently aired fixedSort still hides control | grouped pages lack common toolbar | Entity/Show/Person/Saved/HomeRow gaps | Partly: W1/N8/T1/V2 |
| P17 | Native primary; Automatic/Native/mpv | engine_choice corrected mpv remux | session engine plus new replan action | browser player | native engine/profiles; setting handoff N10 | Partly: source + N10/platform acceptance |
| P18 | HEVC/HDR/multichannel conversion and deep profiles | conversion_targets/decoder rich targets | capability/session fields | codec/HDR probes | device probes, conservative mpv/Vega limits | Partly: hardware/output acceptance |
| P19 | One capability document | device document -> planner | one builder/parser | publishes one document | publisher includes engines | Delivered source |
| P20 | Original default; opt-in adaptive choice; saved/session choice | playbackv1 saved/start/step corrected | choices/AutomaticQuality | settings/player | native measurements/settings | Delivered source; Vega blocked |
| P21 | Broad subtitles; device ASS/image, high-quality burn fallback | original/cache/burn/font paths | subtitle originals/session/bundle | libass/bitmap; iPhone fullscreen limited | mpv/text; offline companions/fonts gap | Partly: N6/platform acceptance |
| P22 | Push ~30s; quiet retained screens, delayed failure | events + scoped counter producers | freshness pool/actions | freshness/content | mounted/pool refresh; no native fade | Partly: source + Vega/specific omissions |
| P23 | Browse/search always answer during rebuild | serving generations/checkpoints | retained reads/fresh cursors | Library/Search | same services | Delivered source, capacity unqualified |
| P24 | Only actual changes conflict; split counters/silent retry | personal/social/session revisions | freshness-actions and CAS consumers | quiet refresh; whole edits fenced | revision-required adapter; subtitle fallback conflict | Partly: second subtitle path/full settings fences |
| P25 | Four server OSes, proxy, overlay, IPv4/6, CGNAT; no subpath/relay | networking/networkpolicy/packaging | route/address consumers | owner address/settings | Apple/Android; Vega blocker | Partly: source + acceptance |
| P26 | HTTP anywhere; home list defines locality; certs; Hosted TLS | networkpolicy/cert listener/publication | address/cert/note services | Network/cert controls | native HTTP; Vega transport absent | Partly: source + Vega |
| P27 | Stated media breadth; social fixes; car/Roku/light excluded | catalogue/listening/live/DVR/downloads/discovery/social | journey services | all journeys | native journeys, parity/Vega gaps | Partly; exclusions honored |
| P28 | One tuner input/channel, separate viewer conversion | linearinput shared pool/session outputs | guide/live clients | Channels/player | Channels/player | Delivered source; physical tuner test |
| P29 | Private DVR intent; shared stored programme | dvr/shared.go and recordingmedia hard links | private dvr services | private DVR | private Recordings | Delivered source; filesystem acceptance |
| P30 | Copies survive tidy; metadata/art/subtitles travel | claims/receipts and bundle originals/fonts | bundle/receipt consumers | server claims | extras consumed; signed-out/companion/fonts gaps | Partly: N6 |
| P31 | Light passwords/no lockout/plain state/TV role; fix defects | credential buckets; global PIN lockout removed; role/auth rules | auth/PIN/device | setup/sign-in/People | same identity | Delivered inspected policy; per-device retry delay retained |

# Decisions I made

- The programme overrides earlier audit recommendations and report decisions. Spoiler protection, sidecar export, country tables, per-library unrated/kids allow-list and unified label rules were dropped by Justin on 15 Oct; new view modes remain a conservative deferral, not a release promise.
- Allow mpv plain repackaging in Automatic when native would convert. Native direct/plain remux still wins; no mpv conversion preference was added.
- Automatic starts at Original even with a slow estimate. Fixed/original throughput refusal and explicit subsequent adaptive moves remain.
- Remove the accounts lane's profile-wide PIN lockout under the explicit no-lockout decision. Retain persisted per-device retry delays, credential checks, hash-slot bounds, PIN revisions and trust binding.
- Remove guide compatibility enumeration completely. A current endpoint failure triggers quiet recovery, never a whole-lineup scan.
- Salvage malformed descriptive chapter/marker/audio bundle rows with skip counts. Bundle identity, grant paths, receipts, policies, sizes and authority-bearing collections remain strict.
- Add an explicit session re-plan action for freshly published device capabilities. Keep the occurrence and saved session choices. Live channels reject it; their wall-clock transport has separate controls.
- Static delivery is not device acceptance. Stored settings were credited only when a runtime consumer was traced; old report gaps were rechecked against integrated code.
- Re-read project standards/verification after the replacement AGENTS.md revision arrived; the lane scope and authorization did not change.

## Decisions taken on the owner's behalf that he should confirm

These are review items, not a request to pause this lane. Later fixes take precedence over an earlier report. Every available report's decision section was considered; process-only entries (test harness selection, sandbox failure, merge mechanics) are omitted.

- accounts-security: Judge remote sign-in at the signed-in device; TV-code approval's location does not decide locality.
- accounts-security: Owner-only is stored owner tier; sign-in policy does not reapply to choosing profiles/refresh, but allowed hours do.
- accounts-security: Outside-hours pauses access without revoking sign-ins; unspecified time zone means server time.
- accounts-security: Narrow Cast target authority to playback/media/self-pairing operations, while TV code retains approver role.
- accounts-security: Device approval exempts the owner, but administrators still need approval.
- accounts-security: Backup scrub and restore sign-out retain passwords, two-factor seeds and integration credentials.
- accounts-security: A restored older backup can revive a subsequently revoked API key/token.
- accounts-security: Deleted people/profile/device names are blanked according to the erasure target.
- accounts-security: Member deletion authority matches disable authority; administrator cannot delete owner/administrator.
- accounts-security: Changing a profile PIN still ends that profile's sign-ins; ownership transfer and role edits apply live.
- accounts-security: A device waiting on TV approval still has the ten-minute code deadline.
- accounts-security: Recipient-bound invitations use account username, not email; post-disable invitation timing decides readmission.
- accounts-security: Twenty cross-device wrong PINs locked the entire profile for fifteen minutes; THIS LANE REMOVED that rule under no-lockout.
- accounts-security: Retain ninety days of ended-session records and wait up to three seconds for a hashing slot.
- api-platform: Keys/tokens have independent lifetimes; password changes and ordinary sign-out do not end integrations.
- api-platform: Keys follow current member roles, never beyond chosen scopes; disabling a member permanently revokes them.
- api-platform: Keep four scopes (read/playback/library/admin); identity/credential/device approval operations remain session-only.
- api-platform: Member-created tokens are subject to device approval; owner's keys are not.
- api-platform: Limits are 100 server keys, 25 tokens/member, ten-year expiry, 20 webhooks, 1000 queued deliveries and 200 log rows/webhook.
- api-platform: Paths-changed is owner-only and uses server-visible paths; no server path-mapping feature.
- api-platform: Delay title.added about 45 seconds; private recording libraries do not emit title events.
- api-platform: Private webhook destinations are accepted; redirects are refused; repeated failure does not automatically disable a webhook.
- api-platform: MCP accepts keys/tokens only and invokes the local API under their scope/role; no owner enable switch.
- android-mpv: Pin the older LGPLv3 full libmpv build; it lacks passthrough, profile-5 Dolby Vision and deinterlacing support.
- android-mpv: Use an app-owned local relay for mpv network reads and system TLS validation.
- android-mpv: Try direct HDR output only with matching screen/decoder and no mpv-drawn subtitles; otherwise tone-map.
- android-mpv: Low-memory TV output can render at interface resolution; direct HDR stays panel-sized.
- android-mpv: System text rendering handles streaming plain captions; mpv local text/font fallback has script limitations.
- android-mpv: Frame-rate matching requires Android 11; older Fire OS cannot expose that ability.
- android-mpv: Pause background video, retain PiP as on-screen, keep music/books playing.
- android-native-review: Destroy the previous engine before releasing its lease; no retained idle player.
- android-native-review: A partial download requires durable digest/byte identity; unverifiable partials are discarded once.
- android-native-review: Signed-out offline launcher remains a system-player path without rich extras/mpv.
- apple-mpv: Pin LGPL MPVKit; compile without it to an unavailable engine and hide the choice.
- apple-mpv: mpv on Apple TV tone-maps to SDR; no PiP/AirPlay video/live channels/music/bitstream passthrough.
- apple-mpv: Automatic styled-subtitle selection may choose mpv, losing HDR on tvOS even on an HDR television.
- apple-mpv: Apple/Android software decode ceilings are device-class estimates, not measurements.
- apple-mpv-review: Destroy mpv before lease release to avoid its audio teardown silencing the successor.
- apple-mpv-review: Prefer safe local grant relay reads; local playlists do not gain arbitrary app-file access.
- apple-native-review: Caption system colour overrides apply to plain captions; authored ASS retains its own styling.
- apple-native-review: Cellular-policy changes can force an iOS partial-download restart.
- audio-engines: Resume rewind is 15 seconds after five minutes and 30 after one hour; explicit seeks reset it.
- audio-engines: Normalization is off by default; unknown gain is unity, and gain cannot amplify without a measured peak.
- audio-engines: Unindexed deep lossy web seeks yield to ordinary playback, losing optional effects/gapless for that session.
- audio-engines: Gapless joins require equal source rate/channels and normal speed; late successors never stall outgoing audio.
- console: Integrations groups keys/tokens/webhooks/MCP; owner is offered keys rather than a duplicate personal-token list.
- console: Expiry uses named choices; Everything is mutually exclusive with other scopes.
- console: Metadata is its own console page; speed and priority are together on Schedule.
- console: Home networks/access addresses/proxies use add/remove lists; trust moved out of Advanced.
- console: Missing cards say Unavailable because unavailable roots and missing files share the card flag.
- console: Member-delete preview is confirmation without typed text; download revocation also has no typed confirmation.
- console: Storage destinations are displayed, not edited; they are startup configuration.
- console: Watched Undo is on title pages only, leaving card-menu marks without an offer.
- console: Remote playback controls live in This device on web, rather than the player.
- conversion-outputs: Missing hardware/codec/quality qualification selects software; failed hardware retries software under current ceilings.
- conversion-outputs: Dolby Vision without usable base needs qualified reshaping; do not approximate its colours.
- conversion-outputs: Upstream AC-3/E-AC-3 encode at most 5.1; other codecs/copy cover supported 7.1.
- conversion-outputs: Retain 200 sanitized failures for thirty days; never retain raw source paths/commands.
- conversion-outputs: Custom rejected FFmpeg remains requested in settings, with qualified default tools used.
- docs-internal: Retained integration credentials and download sign-out behavior override old prose; Vega transport failure is a release gap.
- downloads: Removing a server claim does not delete device copies; explicit receipt revocation remains the withdrawal action.
- downloads: Sign-out keeps files unless removal is selected; another profile can see occupied bytes and remove orphaned copies.
- downloads: Transfer grant lives 24 hours idle, can revive through renewal up to seven days, and rechecks live authority.
- downloads: Original source bytes are read in place; size/mtime custody plus the device digest replaces a full server copy.
- downloads: Server storage ceiling counts converted copies only; receipt renewal is batched once daily.
- downloads: Multipart titles have no single-file download; ordinary versions choose the best source.
- downloads: No WorkManager/new dependency; Android resumes a stopped transfer next app launch.
- freshness-clients: Retained documents are in memory per viewer/server, dropped on scope change; cold start still reads.
- freshness-clients: Fifteen seconds before unreachable UI; any success clears the line without restarting the page.
- freshness-clients: Plain-HTTP tabs use independent long polls where real Web Locks are unavailable.
- freshness-clients: Hidden feeds stop after thirty seconds unless playing; progress refresh paced up to fifteen seconds.
- freshness-clients: Whole-document console PUT conflicts are not blindly replayed; native card fade was omitted.
- freshness-server: Copy-on-change browse and two search slots retain prior generations; a new restriction class serves protected empty content.
- freshness-server: Retain seven-day/20,000-event bounded feed history; fixed small maintenance checkpoints.
- grouping-sorts: One indexed primary sort, maximumSorts=1; random is deterministic indexed rotation with optional uint32 seed.
- grouping-sorts: File size/resolution/container totals use member maxima; provider rating scales are not converted.
- grouping-sorts: Personal valued rows precede missing values; missing values use title order.
- grouping-sorts: Subtitle language includes only current source evidence and the caller's authorized uploads.
- hosted: Third-party sign-in uses existing identity assertions and unregistered PKCE redirects; Hosted grants no local API role/token scope.
- hosted: Consent occurs on the account origin; browser family/account epoch/return URI are exact, codes single-use and short-lived.
- hosted: Preserve terminal signer-revocation knowledge through outage; new signed artefacts require the current certified signer.
- hosted-followups: Root certificate revisions stay in JavaScript's exact integer range; fresh-only Hosted format is 21.
- review-hosted: One-second consent-button delay and account display name (not unique username) remain.
- identity: Default missing-title retention is thirty days; immediate missing publication does not delete the title.
- identity: Local-only shows on different source folders stay separate until provider/manual merge; yearless versus yearful films remain separate until matched.
- identity: Automatic provider merge keeps the newly matched title's id with no redirect for the absorbed id.
- identity: Retain removed-title personal state up to 100,000 entries/profile, oldest dropped first, no age expiry.
- identity: Delete but keep files remains an explicit option; next scan can restore by identity.
- identity: Maximum deleted container is 5000 titles; file size/mtime/root authority must still match.
- leftovers-native: Preserve navigation history/mounted inactive screens; retained-history memory is not fully bounded.
- leftovers-native: Credential-bearing Vega networking fails before dispatch until a safe redirect-refusing transport exists.
- leftovers-native: First-install Keychain cleanup uses exact services and protected-data checks; no server uninstall inference.
- leftovers-server-a: Anime alternate numbering/offset is explicit, never guessed; ambiguous files wait for owner ordering.
- leftovers-server-a: Keep sustained-rewatch contract semantics rather than silently adopting “always keep watched”.
- leftovers-server-a: Subtitle Off wins; a sole default/forced track can be inferred under bounded ambiguity rules.
- leftovers-server-a: Optional preview/image analysis tolerates recoverable decoder damage; exact audio/checksum stages remain strict.
- leftovers-server-a: Basic analysis stays default; Complete prepares useful viewer features, not automatic checksum/fingerprint work.
- leftovers-server-b: Known-duration early EOF does not finish a title; approved credits or configured threshold still can.
- leftovers-server-b: Offline clocks within 24-hour future skew are clamped to receipt time; larger skew refused per entry.
- leftovers-server-b: Broadcast work checkpoints 50 recipients and does not promise notice to profiles created afterwards.
- limits-settings: Lowering resource limits keeps existing artifacts/streams; future work uses new limits.
- limits-settings: Temporary background pause leaves playback, DVR capture, expiry and essential cleanup running.
- limits-settings: Recovery listener stays private-authorized plain HTTP while DB/TLS config is unavailable.
- livetv: Default guide budgets are 512 MiB/50,000 channels/5,000,000 programmes; bounded owner increases, excess ordinary rows skipped.
- livetv: Shared readers join current input; exact source/channel/programme controls storage sharing.
- livetv: Required programme time outranks optional padding; match corrected one-time programmes within thirty minutes only with unique identity/facts.
- livetv: Timeshift remains thirty minutes/1 GiB ceiling; report actual retained span.
- music-audiobooks: Album identity is explicit release ID first, otherwise album artist/title/year; do not split free-text artist punctuation.
- music-audiobooks: Books finish with thirty seconds left, capped at 5% for short files, and retain resume.
- music-audiobooks: Same-duration replacement may retain lyrics; arbitrary matching .txt is supplied lyrics unless recognized as a log.
- music-audiobooks: Shuffle repeats a stable permutation; switching shuffle off returns source order.
- naming: Source root is not title evidence; meaningful file title wins while weak/generic filenames borrow folder evidence.
- naming: Bare final years remain title text with weaker alternate candidates; named editions are versions.
- naming: Episode ranges link at most eight; episode zero/disc structures require separate association support.
- naming: Ignore patterns follow bounded Plex syntax, not Git negation rules.
- native-app: Saved view Play is capped at first 500 titles with a notice; full server-query queue support remains absent.
- native-app: Held invitation links have thirty minutes; duplicate delivery uses bounded five/ten-second windows.
- native-app: Confirmed key-loss reset keeps downloads and reloads JavaScript; ambiguous key unavailability does not wipe.
- native-app: Back from filters applies live; title refresh remains gated to supported server kinds.
- native-health: Only explicit 428/revision_required refusals may be followed by a bounded exact reread/retry; no interrupted write replay.
- native-playback: Device-specific native mpv profile overrides the coarse shared mpv table; native engines declare no in-file audio switching.
- native-playback: Streaming audio-track changes re-plan server-side to retain choices for next episode; local downloads switch in place.
- native-playback: Player-setting changes restart the queue entry; NEW REPLAN ACTION from this lane should replace that path.
- native-playback: Caption Size/Background are in player options on apps while the shared settings page restricts them to web.
- native-playback: Passthrough uses Media3/HDMI receiver evidence; software throughput estimates are not fake network measurements.
- native-sync: ATS allows arbitrary HTTP; the server decides the quiet outside-home note.
- native-sync: Native reports are sent only when requested; crash evidence is retained locally.
- native-sync: Unavailable replaces media length in single-meta-slot rows.
- networking: Home networks are an exact replacement list, empty means all remote; defaults include directly attached subnets.
- networking: Proxy headers trusted only from the owner list; no WAN/overlay/private-address exception.
- networking: Connection ceiling requires restart and is bounded to 65,536; reserved control sockets are not capacity proof.
- ops-packaging: Windows LocalService/service SID and macOS _portico daemon are defaults; authenticated UNC needs owner service account.
- ops-packaging: Manual backups survive pruning; regular cache/backups move independently while rollback/upgrade copies remain beside live DB.
- ops-packaging: Client reports have 100 total/five per device/256 KiB body/1 MiB daily account bounds.
- ops-packaging: Official builds require an injected key matching independently pinned public verification material.
- personal-state: Container watched marks supersede older member intents; newer actions win; 30-second single-use Undo has real revision checks.
- personal-state: Container mark work is atomic and bounded to 5000 members; larger shows act by season.
- personal-state: Completion counts are real plays, not manual marks; sustained rewatch semantics remain.
- personal-state: Shared popularity requires three opted-in eligible profiles; paused/private viewing still reaches owner history.
- planner-wiring: Prefer accepted lossless output to avoid repeated lossy encoding; downmix/rate/depth loss is not labelled lossless preservation.
- planner-wiring: Original uses H.264/HEVC quality 20 or AV1 28; published ladder caps still apply.
- playback-core: Rich capability wire version is 2; decoder/output/HDR/passthrough facts are separate, not a second translation.
- playback-core: Fixed quality is a ceiling; unknown measured-video bitrate uses a conservative size-based estimate.
- playback-core: Best version compares at most eight by least processing, then height/bitrate.
- playback-core: Throughput rejects a clearly impossible fixed/original request but retry-without-estimate can play anyway.
- playback-core: Automatic startup fitting from throughput was implemented; THIS LANE REMOVED it to honor Original start.
- playback-core: Integration quality defaults are per device class/home versus remote; transport hint decides no policy.
- providers-matching: Verify details, accept score >=0.78 with >=0.07 identity margin; guesses below 0.90 remain in attention.
- providers-matching: Local artwork default becomes 2 GiB; exact image limits remain.
- providers-matching: Default metadata en-US/US certifications; explicit foreign/empty region honored, no silent foreign rating.
- providers-matching: Retry outages indefinitely with bounded spread/backoff; auth/validation failures wait for correction.
- registry: Keep actual route error/revision families instead of pretending one uniform response envelope.
- registry: Optional consumer contracts advertise no absent production routes; all real operations have scopes/registry/matrix admission.
- review-accounts-security/review-api-platform: Preserve independent restored key lifetime and exact transaction authority; no new owner policy.
- review-downloads: Current receipt/library permissions outrank cached replay; temporary read errors never permanently revoke copies.
- review-freshness: Separate observations from semantic state; active search invalidation once per bounded batch.
- review-identity: Verify integrated title/retention behavior rather than isolated lane claims; no new product choice.
- review-livetv: Withdrawn recording leader may capture only for authorized followers; private publication stays withdrawn.
- review-livetv: Shared finalization retries three times; missing final XMLTV stop with no next airing is skipped rather than invented.
- review-networking: Same-origin DNS rebinding remains a trade-off of open arbitrary-host first setup; no new detector.
- review-ops: Recovery copies remain beside DB and at most three; regular backups stay in selected destination.
- review-playback-core: Controller re-plans use the playing device's capabilities/locality/current token, not the controlling phone's.
- review-playback-pipeline: Copied targets preserve real stream description; unknown geometry/cadence uses conservative bounds.
- review-scan: Positive exact named-file removal can mark immediately; folder-only/inconclusive evidence preserves root safeguards.
- scan-loop: File List Only still reads bounded ignore configuration; ordinary scans do not trust unchanged directory mtime.
- scan-loop: Watcher debounce is two seconds quiet/ten seconds maximum, bounded roots/watches, periodic fallback.
- scan-loop: Keep twenty terminal scans/source, prune heavy work after fifteen minutes and changes at 10,000/thirty days.
- search-sort: Natural numeric sort and bounded Unicode/CJK search; zero is a real personal rating and null clears it.
- search-sort: Ordinary instants are UTC RFC3339 and positions integer milliseconds; explicit operational units stay named.
- server-docs: Current generated registry wins; no old login/playback/YAML recipe is retained as another contract.
- server-followups: Per-version delete scopes preview fingerprint to the exact title/file, retaining shared files.
- server-followups: Attention totals exclude in-progress work; download usage counts actual stored conversion bytes.
- server-followups: Profile download revocation handles 100 claims/receipts per call; repeat while more is true.
- server-followups: Administrator transport controls are pause/resume/seek only; readback contains no media URL.
- social: Five-second readiness wait; play may bypass it; old readiness cannot release a new seek.
- social: Host self-claim after two minutes silence requires current connected readiness; privacy anonymizes occupancy rather than hiding it.
- social: Same-profile receiver scope and bounded group/command/handoff quotas remain.
- subtitles-text: Charset detection is deterministic sampled scoring using existing x/text; very short samples remain heuristic.
- subtitles-text: Cache two tracks concurrently/64 fills; progress deadline is two minutes plus separate two-hour hard maximum.
- subtitles-text: Originals include ASS/DVB/PGS/VobSub and bounded attached fonts; unsupported burn features do not gain burn authority.
- subtitles-text: Weak unapproved scene guesses are hidden; subtitle provider language follows profile then English.
- vega: Without output evidence declare SDR/stereo and no decoded Dolby; device may qualify broader capability later.
- vega: SDK smooth=false is not treated as inability; decoder size evidence sets capability.
- vega: Source-buffer sequence mode and measured segment ranges align channel playback; Automatic uses measured large segments.
- web-app: Unknown harmless rows/values are tolerated; authorization, fences, grants and receipt outcomes stay exact.
- web-app: Request deadlines are 30s reads/60s writes/120s console; no automatic ambiguous mutation replay.
- web-app: Library memory is per tab; explicit links never overwrite it; paused search-history preference means no local recent list.
- web-player: Keep screen-facing PlaybackService while using the shared v1 controller; web seeks remain local.
- web-player: Automatic down/up thresholds/hysteresis are viewer-selected, never the default.
- web-player: Prepared quality is represented by its fixed requested quality, not a duplicate player menu.
- web-subtitles: Approved WASM package includes LGPL FriBidi; ship notices/pinned source; no asm.js fallback.
- web-subtitles: iPhone fullscreen cannot host drawn ASS/image overlays; text/burn fallback remains.
- web-subtitles: ASS drawing is capped at 1440 lines/4K canvas; authored placement stays and position is device-local.
- web-subtitles: Up Next during credits does not auto-play; iPhone uses native HLS while desktop Safari prefers hls.js.
- base-health/base-health-2/integrator/native-health/review-registry/review-web-logic/security-sweep: Process/validation/merge choices do not establish a new product promise; their evidence does not replace the specific rows above.
- decision-audit: mpv may remux in Automatic; remove global PIN lockout, start Automatic Original, remove guide compatibility scans, salvage harmless bundle rows, and provide same-session capability replan.

- e2e-server: Preserve discovered ASS/SSA style at discovery and cache fill; explicit text rendering remains available. Its three small fixes were source-verified and integrated here verbatim.
- e2e-server: Conversion occupancy returns retryable 429/conversion_capacity with two-second Retry-After, distinct from account stream policy's 409.
- e2e-server: Audio measurement stays background; missing selected asset is a lookup-order defect, not a reason to decode whole books synchronously.
- e2e-server: Name-only NFO people remain unlinked to canonical provider identities; no guessed person identity is inserted.
- review-server-late: File absence reconciliation is factual and never grants extra title-deletion authority; exact source/version fences remain.
- review-server-late: DVR picker examines at most 256 raw indexed candidates and returns at most thirty permitted rows, possibly empty with continuation; no excluded identity in the cursor.
- handoff-draft: Source-delivered versus physical acceptance are separate; compound bug statuses must be adjudicated rather than counted by substrings. Native whole-library removal is absent, despite the initial D53 row; this report corrects it after reading the actual screen.
- e2e-hosted/e2e-web/review-native-js/scale-check/test-plans: isolated fixtures, private roots, fresh baselines and retained acceptance targets do not create a new product policy. Their latest decision sections were read before report finalization.

# Bugs

The assigned audit document for this lane is 00-FINAL-REPORT.md (decision register). The table covers every explicit B-number it references and the three additional IDs named only in ranges, with BUG-REGISTER.md supplying the original claim; range shorthand is followed through the decision/domain trace rather than treated as proof of a fresh incident. FIX-LOG-2026-10-12.md was consulted to avoid redoing fixed base work. Domain mechanisms were checked against integrated code. “Already fixed in source” does not close a physical-device/runtime claim.

The five original policy/implementation discrepancies and capability re-plan gap repaired here are not all separate historical B-number claims. Their regressions and changes are in API/test sections below. No claim is made that this lane independently reproduced all 588 incidents in the full register.

| Bug | Current disposition | Evidence / remaining boundary |
|---|---|---|
| B-01.2 | already fixed in source | identity direct membership/device/session paths; httpapi/connection_policy.go and api_credentials_routes.go; relevant observable decision paths are mapped in the two matrices above. |
| B-01.3 | already fixed in source | identity direct membership/device/session paths; httpapi/connection_policy.go and api_credentials_routes.go; relevant observable decision paths are mapped in the two matrices above. |
| B-01.4 | already fixed in source | identity direct membership/device/session paths; httpapi/connection_policy.go and api_credentials_routes.go; relevant observable decision paths are mapped in the two matrices above. |
| B-01.5 | already fixed in source | identity direct membership/device/session paths; httpapi/connection_policy.go and api_credentials_routes.go; relevant observable decision paths are mapped in the two matrices above. |
| B-01.10 | already fixed in source | identity direct membership/device/session paths; httpapi/connection_policy.go and api_credentials_routes.go; relevant observable decision paths are mapped in the two matrices above. |
| B-02.1 | already fixed in source | playback capability_document/engine_choice and playbackv1 sessions/changes/quality; current HTTP start/PATCH routes; relevant observable decision paths are mapped in the two matrices above. |
| B-02.2 | already fixed in source | playback capability_document/engine_choice and playbackv1 sessions/changes/quality; current HTTP start/PATCH routes; relevant observable decision paths are mapped in the two matrices above. |
| B-02.4 | already fixed in source | playback capability_document/engine_choice and playbackv1 sessions/changes/quality; current HTTP start/PATCH routes; relevant observable decision paths are mapped in the two matrices above. |
| B-02.6 | fixed additionally in this lane | Existing current-session quality/version/audio path was present; new replan:true supports capability-setting changes without restarting the occurrence/queue or dropping saved choices. Native caller handoff N10. |
| B-02.7 | already fixed in source | playback capability_document/engine_choice and playbackv1 sessions/changes/quality; current HTTP start/PATCH routes; relevant observable decision paths are mapped in the two matrices above. |
| B-02.8 | not present as defect | Meaningful sustained rewatch still clears watched by the preserved contract, D61; do not silently change it. |
| B-02.12 | already fixed in source | playback capability_document/engine_choice and playbackv1 sessions/changes/quality; current HTTP start/PATCH routes; relevant observable decision paths are mapped in the two matrices above. |
| B-02.16 | already fixed in source for the bypass | V1 queue starts/re-plans share engine-specific failure memory; chosen six-hour/seven-day retention remains a disclosed policy, not device acceptance. |
| B-03.1 | already fixed in source | decoder qualified target construction and playback conversion_targets/HLS admission/retention; relevant observable decision paths are mapped in the two matrices above. |
| B-03.3 | already fixed in source | decoder qualified target construction and playback conversion_targets/HLS admission/retention; relevant observable decision paths are mapped in the two matrices above. |
| B-03.4 | already fixed in source | decoder qualified target construction and playback conversion_targets/HLS admission/retention; relevant observable decision paths are mapped in the two matrices above. |
| B-03.5 | already fixed in source | decoder qualified target construction and playback conversion_targets/HLS admission/retention; relevant observable decision paths are mapped in the two matrices above. |
| B-03.7 | not present | Second review refuted the keyframe-expression claim; no contradictory override was reintroduced. |
| B-03.8 | already fixed in source | decoder qualified target construction and playback conversion_targets/HLS admission/retention; relevant observable decision paths are mapped in the two matrices above. |
| B-03.10 | already fixed in source | decoder qualified target construction and playback conversion_targets/HLS admission/retention; relevant observable decision paths are mapped in the two matrices above. |
| B-04.1 | already fixed in source | subtitles inventory/cache/original/device delivery and bounded parsers; relevant observable decision paths are mapped in the two matrices above. |
| B-04.4 | already fixed in source | subtitles inventory/cache/original/device delivery and bounded parsers; relevant observable decision paths are mapped in the two matrices above. |
| B-04.5 | already fixed in source | subtitles inventory/cache/original/device delivery and bounded parsers; relevant observable decision paths are mapped in the two matrices above. |
| B-04.9 | already fixed in source | subtitles inventory/cache/original/device delivery and bounded parsers; relevant observable decision paths are mapped in the two matrices above. |
| B-04.10 | already fixed in source | subtitles inventory/cache/original/device delivery and bounded parsers; relevant observable decision paths are mapped in the two matrices above. |
| B-04.21 | already fixed in source | Weak unapproved scene hypotheses no longer reach viewer Skip prompts; chapter/qualified markers remain. |
| B-04.31 | already fixed in source | subtitles inventory/cache/original/device delivery and bounded parsers; relevant observable decision paths are mapped in the two matrices above. |
| B-05.1 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.2 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.3 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.4 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.6 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.7 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.8 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.10 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.11 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.12 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.16 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.18 | already fixed in source, runtime unqualified | Root identity acceptance/rebound and source custody exist; network filesystem inode stability still needs real mount acceptance. |
| B-05.19 | already fixed in source | Ordinary scans list file facts regardless of unchanged directory mtime; bounded ignore configuration is independently read. |
| B-05.20 | already fixed in source | Terminal inventory work/change history now has bounded pruning; active work is retained. |
| B-05.21 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.27 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-05.28 | already fixed in source | naming/naming.go plus ingestion inventory/identification/sidecar/watcher and catalog identity/missing paths; relevant observable decision paths are mapped in the two matrices above. |
| B-06.1 | already fixed in source | metadata provider scoring/publish/freshness/attention and local-artwork quota; relevant observable decision paths are mapped in the two matrices above. |
| B-06.2 | already fixed in source | metadata provider scoring/publish/freshness/attention and local-artwork quota; relevant observable decision paths are mapped in the two matrices above. |
| B-06.3 | already fixed in source | metadata provider scoring/publish/freshness/attention and local-artwork quota; relevant observable decision paths are mapped in the two matrices above. |
| B-06.4 | already fixed in source | metadata provider scoring/publish/freshness/attention and local-artwork quota; relevant observable decision paths are mapped in the two matrices above. |
| B-06.11 | already fixed in source, capacity unqualified | Provider/scan rates read gentle/normal/fast policy, but large first-import duration has not been measured here. |
| B-06.15 | already fixed in source | metadata provider scoring/publish/freshness/attention and local-artwork quota; relevant observable decision paths are mapped in the two matrices above. |
| B-06.24 | already fixed in source | metadata provider scoring/publish/freshness/attention and local-artwork quota; relevant observable decision paths are mapped in the two matrices above. |
| B-07.1 | already fixed in source | catalog/compactcatalog serving browse/search generations, scoped revisions, retained state and bounded derivation; relevant observable decision paths are mapped in the two matrices above. |
| B-07.2 | already fixed in source | catalog/compactcatalog serving browse/search generations, scoped revisions, retained state and bounded derivation; relevant observable decision paths are mapped in the two matrices above. |
| B-07.4 | already fixed in source | catalog/compactcatalog serving browse/search generations, scoped revisions, retained state and bounded derivation; relevant observable decision paths are mapped in the two matrices above. |
| B-07.6 | already fixed in source | catalog/compactcatalog serving browse/search generations, scoped revisions, retained state and bounded derivation; relevant observable decision paths are mapped in the two matrices above. |
| B-07.7 | already fixed in source | catalog/compactcatalog serving browse/search generations, scoped revisions, retained state and bounded derivation; relevant observable decision paths are mapped in the two matrices above. |
| B-07.15 | not fixed as a universal allegation | Identity/inventory/event retention and incremental reclaim exist; this decision trace does not prove every insert-only table has a lifetime bound. No new unbounded table was added. |
| B-08.1 | already fixed in source | httpapi request_locality/networkpolicy, current registry and scoped browse revisions; relevant observable decision paths are mapped in the two matrices above. |
| B-08.7 | already fixed in source | httpapi request_locality/networkpolicy, current registry and scoped browse revisions; relevant observable decision paths are mapped in the two matrices above. |
| B-08.8 | already fixed in source | httpapi request_locality/networkpolicy, current registry and scoped browse revisions; relevant observable decision paths are mapped in the two matrices above. |
| B-09.1 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-09.2 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-09.3 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-09.4 | already fixed in source | The Scheduled Task launch path is gone; Windows now uses SCM/service SID. Needs actual installation/upgrade acceptance. |
| B-09.5 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-09.7 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-09.8 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-09.9 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-09.10 | not present | Forced secureConnectionsPolicy removed because it cannot be meaningful under never-force-TLS; certificates and HTTP guidance remain. |
| B-09.11 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-09.13 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-09.18 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-09.19 | already fixed for sign-ins; retained integration trade-off | Backup scrub/restore end sign-ins, deliberately preserve keys/tokens/passwords/two-factor seeds. An old restore can revive a later-revoked key. |
| B-09.21 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-09.35 | already fixed in source | operations/backup/persistence/cmd-server/packaging and networkpolicy resource/transport paths; relevant observable decision paths are mapped in the two matrices above. |
| B-10.1 | already fixed in source | audiotags/catalog audio identity/listeningrules/lyrics and playbackv1 shuffled queues; relevant observable decision paths are mapped in the two matrices above. |
| B-10.2 | already fixed in source | audiotags/catalog audio identity/listeningrules/lyrics and playbackv1 shuffled queues; relevant observable decision paths are mapped in the two matrices above. |
| B-10.3 | already fixed in source | audiotags/catalog audio identity/listeningrules/lyrics and playbackv1 shuffled queues; relevant observable decision paths are mapped in the two matrices above. |
| B-10.4 | already fixed in source | audiotags/catalog audio identity/listeningrules/lyrics and playbackv1 shuffled queues; relevant observable decision paths are mapped in the two matrices above. |
| B-10.6 | already fixed in source | audiotags/catalog audio identity/listeningrules/lyrics and playbackv1 shuffled queues; relevant observable decision paths are mapped in the two matrices above. |
| B-10.8 | already fixed in source | audiotags/catalog audio identity/listeningrules/lyrics and playbackv1 shuffled queues; relevant observable decision paths are mapped in the two matrices above. |
| B-10.16 | already fixed in source | audiotags/catalog audio identity/listeningrules/lyrics and playbackv1 shuffled queues; relevant observable decision paths are mapped in the two matrices above. |
| B-10.17 | already fixed in source | audiotags/catalog audio identity/listeningrules/lyrics and playbackv1 shuffled queues; relevant observable decision paths are mapped in the two matrices above. |
| B-11.1 | already fixed in source | linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above. |
| B-11.2 | already fixed in source | linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above. |
| B-11.3 | already fixed in source | linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above. |
| B-11.5 | already fixed for ordinary excess rows | Excess channel/programme rows are skipped with diagnostics; hostile document/lexical/decompression bounds still refuse oversized inputs. |
| B-11.6 | already fixed in source | linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above. |
| B-11.7 | already fixed in source | linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above. |
| B-11.8 | not fixed on all screens | Server can attach XMLTV to HDHomeRun; native add-source hides guideUrl for hdhomerun. N5 identifies the screen fix. |
| B-11.9 | already fixed in source | linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above. |
| B-11.10 | already fixed in source | linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above. |
| B-11.11 | already fixed in source | linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above. |
| B-11.13 | already fixed in source | linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above. |
| B-11.14 | already fixed in source | linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above. |
| B-11.26 | already fixed in source | linearinput shared acquisition, livechannels bounded guide parsing and dvr shared private captures; relevant observable decision paths are mapped in the two matrices above. |
| B-12.1 | already fixed in source | catalog personal/container state, retained identity and downloads offline-progress writers; relevant observable decision paths are mapped in the two matrices above. |
| B-12.5 | already fixed in source | catalog personal/container state, retained identity and downloads offline-progress writers; relevant observable decision paths are mapped in the two matrices above. |
| B-12.11 | already fixed in source | catalog personal/container state, retained identity and downloads offline-progress writers; relevant observable decision paths are mapped in the two matrices above. |
| B-12.18 | already fixed in source | catalog personal/container state, retained identity and downloads offline-progress writers; relevant observable decision paths are mapped in the two matrices above. |
| B-12.20 | already fixed in source | catalog personal/container state, retained identity and downloads offline-progress writers; relevant observable decision paths are mapped in the two matrices above. |
| B-13.1 | already fixed in source | social command/readiness revisions and private bounded receiver recovery; scoped Cast target authority; relevant observable decision paths are mapped in the two matrices above. |
| B-13.2 | already fixed in source | social command/readiness revisions and private bounded receiver recovery; scoped Cast target authority; relevant observable decision paths are mapped in the two matrices above. |
| B-13.3 | already fixed in source | social command/readiness revisions and private bounded receiver recovery; scoped Cast target authority; relevant observable decision paths are mapped in the two matrices above. |
| B-13.7 | already fixed in source | social command/readiness revisions and private bounded receiver recovery; scoped Cast target authority; relevant observable decision paths are mapped in the two matrices above. |
| B-13.14 | already fixed in source | social command/readiness revisions and private bounded receiver recovery; scoped Cast target authority; relevant observable decision paths are mapped in the two matrices above. |
| B-14.1 | already fixed in source | downloads claim/receipt/bundle paths; native offline-downloads storage/player paths; relevant observable decision paths are mapped in the two matrices above. |
| B-14.2 | not fixed completely | Android persists range identity/resume; iOS can reuse same-URL resume data but a newly issued transfer URL or changed cellular policy can restart bytes. Native platform work. |
| B-14.3 | already fixed in source | downloads claim/receipt/bundle paths; native offline-downloads storage/player paths; relevant observable decision paths are mapped in the two matrices above. |
| B-14.4 | already fixed in source | downloads claim/receipt/bundle paths; native offline-downloads storage/player paths; relevant observable decision paths are mapped in the two matrices above. |
| B-14.6 | already fixed in source | downloads claim/receipt/bundle paths; native offline-downloads storage/player paths; relevant observable decision paths are mapped in the two matrices above. |
| B-14.10 | already fixed in source | downloads claim/receipt/bundle paths; native offline-downloads storage/player paths; relevant observable decision paths are mapped in the two matrices above. |
| B-14.12 | already fixed in source for supported native engines | Original remains offered and mpv can take non-native containers; signed-out system-player launcher still needs N6, and device codec acceptance remains. |
| B-14.14 | already fixed in source | downloads claim/receipt/bundle paths; native offline-downloads storage/player paths; relevant observable decision paths are mapped in the two matrices above. |
| B-15.1 | already fixed in source | identity exact local/session authority plus httpapi request_locality/setup_transport and Hosted trust provenance; relevant observable decision paths are mapped in the two matrices above. |
| B-15.2 | already fixed in source | identity exact local/session authority plus httpapi request_locality/setup_transport and Hosted trust provenance; relevant observable decision paths are mapped in the two matrices above. |
| B-15.3 | already fixed in source | identity exact local/session authority plus httpapi request_locality/setup_transport and Hosted trust provenance; relevant observable decision paths are mapped in the two matrices above. |
| B-15.4 | not present as a programme defect | Open arbitrary-host first setup is deliberate; ordinary cross-origin setup is rejected. Same-origin rebinding protection is not claimed. |
| B-15.5 | not present as a programme defect | Nothing encrypted at rest remains explicit. Scrubbed sign-ins are separate from stored password/second-factor/integration-key policy. |
| B-15.6 | already fixed in source | identity exact local/session authority plus httpapi request_locality/setup_transport and Hosted trust provenance; relevant observable decision paths are mapped in the two matrices above. |
| B-15.7 | already fixed in documentation | Current identity guide describes bearer/session-family/installation binding; no proof-of-possession device-key claim. |
| B-15.8 | already fixed in source | identity exact local/session authority plus httpapi request_locality/setup_transport and Hosted trust provenance; relevant observable decision paths are mapped in the two matrices above. |
| B-15.11 | not present as hidden detection | Explicit trusted proxy forwarding/home networks define locality. A gateway that conceals an address cannot supply evidence the server never received. |
| B-15.12 | already fixed in source for learned revocation | Pinned root certificate/check-in provenance ends dependent sessions once learned; isolated-server propagation still needs runtime acceptance. |
| B-15.13 | not present as a programme defect | TV code retains approver role, exactly as instructed; Cast target is separately narrowed. |
| B-16.3 | already fixed in source | web bounded transport/tolerant parsers/accessibility and viewer-scoped revalidating caches; relevant observable decision paths are mapped in the two matrices above. |
| B-16.6 | not fixed for every user-facing string | Regional/catalogue consumers exist; native staged new-words/player-words and some raw fallback labels still need string consolidation. N11/W5. |
| B-16.8 | already fixed in source | web bounded transport/tolerant parsers/accessibility and viewer-scoped revalidating caches; relevant observable decision paths are mapped in the two matrices above. |
| B-16.10 | fixed additionally in this lane | Integrated tolerant parsers existed; download bundle chapter/marker/audio descriptions still failed whole-bundle parsing. Now skip/count only bad rows with exact authority/bounds. |
| B-16.13 | already fixed in source | web bounded transport/tolerant parsers/accessibility and viewer-scoped revalidating caches; relevant observable decision paths are mapped in the two matrices above. |
| B-16.14 | already fixed in source | web bounded transport/tolerant parsers/accessibility and viewer-scoped revalidating caches; relevant observable decision paths are mapped in the two matrices above. |
| B-16.15 | already fixed in source | web bounded transport/tolerant parsers/accessibility and viewer-scoped revalidating caches; relevant observable decision paths are mapped in the two matrices above. |
| B-16.19 | already fixed in source | web bounded transport/tolerant parsers/accessibility and viewer-scoped revalidating caches; relevant observable decision paths are mapped in the two matrices above. |
| B-17.1 | fixed additionally in this lane | Existing current-session quality/version/audio path was present; new replan:true supports capability-setting changes without restarting the occurrence/queue or dropping saved choices. Native caller handoff N10. |
| B-17.2 | fixed additionally in this lane | Existing current-session quality/version/audio path was present; new replan:true supports capability-setting changes without restarting the occurrence/queue or dropping saved choices. Native caller handoff N10. |
| B-17.3 | already fixed in source | web playback-v1/web-playback and shared session/queue bridge; player adapter/watchdog; relevant observable decision paths are mapped in the two matrices above. |
| B-17.4 | already fixed in source | web playback-v1/web-playback and shared session/queue bridge; player adapter/watchdog; relevant observable decision paths are mapped in the two matrices above. |
| B-17.5 | already fixed in source | web playback-v1/web-playback and shared session/queue bridge; player adapter/watchdog; relevant observable decision paths are mapped in the two matrices above. |
| B-17.8 | already fixed in source | web playback-v1/web-playback and shared session/queue bridge; player adapter/watchdog; relevant observable decision paths are mapped in the two matrices above. |
| B-17.25 | already fixed in source | The shipped adapter/controller has focused tests; this lane also exercised real session re-plans through HTTP. Browser playback acceptance still pending. |
| B-18.1 | already fixed in source | web library/detail/console/settings and common core services; relevant observable decision paths are mapped in the two matrices above. |
| B-18.3 | already fixed in source | web library/detail/console/settings and common core services; relevant observable decision paths are mapped in the two matrices above. |
| B-18.4 | already fixed in source | web library/detail/console/settings and common core services; relevant observable decision paths are mapped in the two matrices above. |
| B-18.5 | already fixed on server/web; native screen missing | DELETE /v1/admin/libraries/{id} and web typed-confirmation flow exist. Native LibrariesPanel only removes a folder via removeSource, not the library. N3. |
| B-18.8 | already fixed in source | web library/detail/console/settings and common core services; relevant observable decision paths are mapped in the two matrices above. |
| B-18.12 | not fixed for every user-facing string | Ordinary settings/screens use catalogue; staged/raw fallback strings remain a parity/language handoff. W5/N11. |
| B-18.13 | already fixed in source | web library/detail/console/settings and common core services; relevant observable decision paths are mapped in the two matrices above. |
| B-18.14 | already fixed in source | web library/detail/console/settings and common core services; relevant observable decision paths are mapped in the two matrices above. |
| B-18.19 | already fixed in source | Shared settings scopes identify profile-device-class instead of claiming these defaults are one physical device; engine setting is local. |
| B-18.27 | not fixed on every screen | Server settings have runtime readers; native owner resource/metadata/integration/diagnostic panels and native card sizing remain incomplete. N1–N5/N9. |
| B-18.28 | already fixed in source | web library/detail/console/settings and common core services; relevant observable decision paths are mapped in the two matrices above. |
| B-19.1 | already fixed in source | main-native pending links/viewer scope/lifecycle and Apple scene manifests; relevant observable decision paths are mapped in the two matrices above. |
| B-19.2 | already fixed in source | main-native pending links/viewer scope/lifecycle and Apple scene manifests; relevant observable decision paths are mapped in the two matrices above. |
| B-19.18 | already fixed in source | Current iOS/tvOS Info.plist has UIApplicationSceneManifest and AppDelegate.swift defines SceneDelegate; the older native-review report is superseded. Apple archive/launch acceptance still needed. |
| B-20.1 | already fixed in source | main-native capability_document/engine/player/module paths; canonical capability/session delivery; relevant observable decision paths are mapped in the two matrices above. |
| B-20.2 | already fixed in source | main-native capability_document/engine/player/module paths; canonical capability/session delivery; relevant observable decision paths are mapped in the two matrices above. |
| B-20.3 | already fixed in source | main-native capability_document/engine/player/module paths; canonical capability/session delivery; relevant observable decision paths are mapped in the two matrices above. |
| B-20.4 | already fixed in source, runtime unqualified | Android video lifecycle pause/PiP handling exists in both engines; physical Home/background test still required. |
| B-20.5 | already fixed in source, runtime unqualified | Native ready/display-switch gating exists; delayed-window and HDR mode-switch timing require actual Apple TV acceptance. |
| B-20.7 | already fixed in source, runtime unqualified | System pause facts update client intent; native audio session/display lifecycle still needs device check. |
| B-20.8 | already fixed in source | main-native capability_document/engine/player/module paths; canonical capability/session delivery; relevant observable decision paths are mapped in the two matrices above. |
| B-20.9 | already fixed in source, runtime unqualified | Capability codec tags force repackaging for hev1/dvhe instead of unqualified AVPlayer direct play. |
| B-20.10 | already fixed in source | main-native capability_document/engine/player/module paths; canonical capability/session delivery; relevant observable decision paths are mapped in the two matrices above. |
| B-20.11 | already fixed in source | main-native capability_document/engine/player/module paths; canonical capability/session delivery; relevant observable decision paths are mapped in the two matrices above. |
| B-20.12 | already fixed in source | main-native capability_document/engine/player/module paths; canonical capability/session delivery; relevant observable decision paths are mapped in the two matrices above. |
| B-20.19 | not fixed universally | Some non-indexed/local/unknown-duration sources still lack exact seek/EOF evidence; bounded fallback exists, but device corpus acceptance and signed-out original playback remain N6. |
| B-21.5 | not fixed completely | Visible rows use windowed collections but inactive navigation/history stays retained; no measured all-history memory bound. Do not declare capacity parity. |
| B-21.7 | already fixed in source | main-native common screen and windowed collections/presentation paths; relevant observable decision paths are mapped in the two matrices above. |
| B-21.11 | not fixed completely | Native Saved view Play still caps first 500 titles with notice; no full bounded server-query queue selector. N8/T1. |
| B-22.1 | already fixed in source | shared signed route/credential refresh/transport paths; native scoped routing; relevant observable decision paths are mapped in the two matrices above. |
| B-22.2 | already fixed in source | shared signed route/credential refresh/transport paths; native scoped routing; relevant observable decision paths are mapped in the two matrices above. |
| B-22.3 | already fixed in source | shared signed route/credential refresh/transport paths; native scoped routing; relevant observable decision paths are mapped in the two matrices above. |
| B-22.4 | already fixed in source | shared signed route/credential refresh/transport paths; native scoped routing; relevant observable decision paths are mapped in the two matrices above. |
| B-22.5 | already fixed in source | shared signed route/credential refresh/transport paths; native scoped routing; relevant observable decision paths are mapped in the two matrices above. |
| B-22.6 | not present as a programme defect | Plain HTTP cannot authenticate every later socket; owner choice is allowed. Signed proof authenticates the probe only. |
| B-22.8 | already fixed in source | shared signed route/credential refresh/transport paths; native scoped routing; relevant observable decision paths are mapped in the two matrices above. |
| B-22.18 | already fixed in source | shared signed route/credential refresh/transport paths; native scoped routing; relevant observable decision paths are mapped in the two matrices above. |
| B-22.19 | already fixed for credential disclosure; not operationally delivered | Vega refuses unsafe SDK credential requests before dispatch; authenticated app is unavailable until a safe transport exists. V1. |
| B-22.20 | already fixed in source | shared signed route/credential refresh/transport paths; native scoped routing; relevant observable decision paths are mapped in the two matrices above. |
| B-25.1 | already fixed in source | packaging/release scripts and required signing identities/workflow input; relevant observable decision paths are mapped in the two matrices above. |
| B-25.2 | already fixed in source | Release tooling accounts for the maintained SQLite-driver vet exception without disguising other vet failures; full release CI not run here. |
| B-25.3 | already fixed in source | packaging/release scripts and required signing identities/workflow input; relevant observable decision paths are mapped in the two matrices above. |
| B-25.4 | already fixed in source | packaging/release scripts and required signing identities/workflow input; relevant observable decision paths are mapped in the two matrices above. |
| B-25.5 | already fixed in prerequisite source | Hosted toolkit/contracts were synced and checked in its lane; this checkout contains consumer contracts, not writable Hosted implementation. No independent Hosted runtime acceptance claimed. |
| B-25.6 | already fixed in source | Official packaging requires injected signing identity matching pinned verification material; no generated signing identity fallback. |
| B-25.12 | not fixed | Apple distribution/legal grant and bundled-source/store compliance need owner-qualified review; no legal conclusion invented. |
| B-25.19 | already fixed in source | packaging/release scripts and required signing identities/workflow input; relevant observable decision paths are mapped in the two matrices above. |
| B-25.20 | already fixed in source | packaging/release scripts and required signing identities/workflow input; relevant observable decision paths are mapped in the two matrices above. |
| B-02.3 | already fixed in source | One capability document preserves embeddedAudioSwitching; native deliberately declares false while mpv declares its real switching ability. No second wire translation. |
| B-02.5 | already fixed in source | Rich capability document reaches the planner with decoder/output/HDR/subtitle/transport facts intact. Device probes still need acceptance. |
| B-11.4 | already fixed in source | Malformed ordinary programme rows are skipped/counted; hostile whole-document/lexical/entity bounds remain. |

### Additional fixes verified here

- Automatic throughput starts: HTTP test requires both explicit and saved Automatic to start Original, then explicitly re-plan lower without replacing the occurrence.
- MPV copy/remux: planner tests require native direct/remux priority, MPV remux over native video conversion, failed route exclusions, and native conversion when MPV would also convert.
- Profile-wide PIN lockout: 25 wrong PINs on other devices no longer deny the correct PIN elsewhere; exact persisted per-device debt still denies its offending device.
- Guide compatibility fallback: 404/405/503 never enumerate the old whole-lineup source. Current bounded source/channel/programme paths retain cancellation and authorization-handle guards.
- Bundle tolerance: malformed descriptive rows are individually dropped and counted; oversized lists, preparation mismatch and unsafe grant paths still reject.
- E2E source-confirmed joins: discovered ASS style/cache renderer, typed conversion-capacity error/status/message, and SelectedAsset scan-before-return are integrated from e2e-server snapshot 004 and covered by its exact focused regressions.
- Capability re-plan: publishing mpv capabilities then PATCH replan keeps session/queue/state/quality/audio/subtitle choice and starts the new generation at current position.

# API changes

| Route / surface | Old behavior or shape | New behavior or shape | Consumer / contract |
|---|---|---|---|
| POST /v1/playback/sessions | Automatic with throughput could start on a lower rung | Automatic starts Original regardless of supplied estimate; saved Automatic follows same rule | Existing current start shape; real HTTP regressions |
| PATCH /v1/playback/sessions/{id} | No way to request a new plan solely after publishing changed capabilities | Optional replan:boolean; true creates a new presentation under current playing-device capability/policy, keeping occurrence, queue, state and saved version/quality/tracks, at supplied positionMs (or last observation) | playbackv1.Change, generated contracts/types/schemas/OpenAPI; SessionsClient/PlaybackChoices/V1QueuePlayer/PlaybackService pass the one-shot action |
| PATCH /v1/playback/sessions/{id}, live channel | New field did not exist | replan:true returns field-input error; live session's wall-clock controls remain its own path | Same typed route/error registry; no new lane/scope |
| PUT device capabilities / current session planning | Automatic selected native conversion even when mpv could copy/remux | mpv copy/remux is admissible only when native compromises/burns; native direct/plain remux keeps priority | One capability document, existing session presentation.engine |
| Direct profile selection | Wrong PINs across devices could close an entire profile | No profile-wide cross-device lockout; offending device's persisted retry delay remains | Same sign-in/profile API shape; no weakening of PIN proof/revision/trust or hash bound |
| Shared guide directory/summary reads | 404/405 silently invoked old whole-lineup enumeration | Current /v1/guide/channels and /v1/guide/sources are required; no compatibility enumeration; failures propagate for quiet recovery | guide/legacy-source adapter remains a row-model adapter, not an old-server transport; no screen-facing API name change |
| Download bundle descriptive arrays | One malformed chapter/marker/audio description rejected the bundle | Valid rows remain, invalid rows skipped/counted | downloads.ts parsers only; paths/receipts/scopes/bounds remain strict |

No route was added or removed in this lane. Conversion-producing start/change/create/advance/prepare-next operations now declare conversion_capacity: retryable 429 with Retry-After: 2 rather than internal 500; its shared message is catalogue-backed. Other admission/refusal/authority checks are unchanged. The existing PATCH route remains typed on apikit, declares playback credential scope, and keeps its existing restriction/admission classification. API generation was run after the new request field; the optional replan boolean and declared conversion-capacity error on five producing operations are the expected contract changes. The API guide documents capability re-planning. admin-access.md no longer refers to a deleted handwritten YAML contract.

# Schema changes

None. No baseline or migration was edited; schema-sync was unnecessary. Replan is a request action, not stored session state. Existing saved request JSON already carries the quality/version/track choices and is reused. Removed global PIN lockout needed no table change because per-device attempts still use the existing table.

# Owner or viewer settings added or wired

No new persistent setting was added. The conversion-capacity refusal adds one catalogue sentence, error.playbackConversionCapacity, reused by shared playback messages and queue errors. The important existing settings traced are below; this distinguishes runtime wiring from screen availability.

| Setting | Type / default | Observable consumer / effect | Screen availability |
|---|---|---|---|
| quality.home / quality.remote | String choice; original | playbackv1 Caller.SavedQuality -> savedQuality -> present; this lane fixes Automatic start with throughput | Web/native Settings and player menus |
| Player engineSetting | Device choice; automatic | One capability document -> playback.planDevice -> presentation.engine; new replan action applies a newly published document mid-session | Native Settings exists; mid-title caller still needs N10; hidden when no engine |
| Capability overrides forceStereo/disablePassthrough/disableHdr | Booleans; false | Decoder/output/transport planner checks narrow acceptable output | Web/native This device controls; Vega conservative capability limits |
| library.missingTitles / library.missingTitlesDays | Choice after_days / integer 30 days | catalog.MissingRule -> cutoff -> bounded EmptyMissingDue, startup/safety/owner path; unreachable roots protected | Web/native missing-title policy/list |
| Metadata agent/provider | Library online with TMDB video default; local option | metadata agent admission gates matching/search/refresh/artwork; provider policy controls requests | Web/native Libraries provider choice exists; global metadata/attention missing on native |
| localArtworkLimitBytes | Integer; 2 GiB | metadata/runtime_settings.go -> local artwork admission; measured usage readback | Web MetadataSettings; native missing |
| backgroundSpeed | gentle/normal/fast; normal | administration -> dbwork/workpolicy -> scan/provider/checkpoint pacing | Web/native maintenance form row |
| temporaryDirectory | Absolute path; empty uses state directory | cmd/server effective startup selection -> dedicated portico-hls child; reported requested/effective/restart state | Web/native runtime setting rows |
| maxBackgroundSessions | Integer; 0 unlimited owner policy | startup and RegistryDeliverySettings.Refresh -> decoder background conversion admission | Web/native setting row |
| sessionStorageMiB | Integer; 4096 | Playback/decoder retention trims closed played segments before stopping over-budget production | Web/native form; detailed native usage panel missing |
| maxConcurrentSessions/maxHardwareSessions/maxSoftwareSessions | Integers; 0 unlimited owner ceiling | Playback admission; current new-operation policy, no eviction when lowered | Web/native rows |
| analysisStorageGiB / analysisStageStorageMiB | Integers; 32 GiB / 256 MiB | Media analysis artifact/stage admission; existing artifacts retained on lowered limit | Form rows; detailed native usage missing |
| subtitleTrackLimit / subtitleFileLimitMiB | Integers; 32 / 32 MiB | connection-policy cache -> medialimits -> inventory/acquisition; independent text/cue/font/process bounds remain | Form rows; no new switch |
| lanNetworks | String CIDR/address list; loopback/private/link-local/attached subnets | networkpolicy exact membership, cached on save; empty makes everyone remote; sign-in/quality/admission/rate paths agree | Web/native connectivity row |
| trustedProxies / accessUrls / advertiseUrl | Lists / strings; empty | Trusted bounded forwarded-chain parsing and advertisement; Hosted receives TLS routes only | Web/native connectivity forms |
| connectionLimit | Integer; 0 host default | Listener ordinary ceiling plus reserved control sockets, applies on restart | Web/native row; no proven capacity number |
| Guide horizon/size/count/group/channel controls | Existing bounded guide/source settings; D42 budgets | livechannels acquisition/import/filter/guide publication and linearinput buffering | Web complete; native source controls partly present, N5 |
| DVR padding/retention | Existing defaults, snapshot per commitment/group | Programme core time outranks optional padding; private shared capture/retention paths | Web/native ordinary rows |
| search.rememberHistory | Boolean; true | Explicit search commit/result opening writes history only when enabled; no disabled local recent list | Web/native preferences/search |
| playback.startedThresholdPercent / playedThresholdPercent | Numeric existing registry defaults | Personal progress/completion policy; book near-end rules stay specialized | Shared settings rows; separate owner play history still retained |
| Caption size/background | Existing profile settings | Web overlay/native caption command; system overrides applied per actual system policy | Web settings; native player menu, settings-page parity N9 |
| appearance.cardSizePercent | Existing class preference | Web card layout consumes it | Native has no reader/control effect; N9 |
| apiRequestsPerMinute | Integer; 600 | Scoped credential admission/rate accounting, not reserved console connections | Web Integrations; native panel missing |
| Client log retention | Existing bounded 1–90 day category setting | Startup and maintenance prune uploaded reports without clearing daily budget | Web owner report UI; native owner report UI missing |

# Needed from others

## Web

- **W1 — common controls on every grouping.** In screens/library/Entity.tsx (collection/category/artist/album/book containers), screens/detail/Show.tsx (selected show/season/episode group), screens/people/Person.tsx (person's credits), screens/home/HomeRowPage.tsx, and screens/saved/Resource.tsx (My List/Favorites/playlist/view), add the same BrowseBar/filter controls used by Library. Read GET /v1/libraries/{id}/browse-capabilities for the selected pivot; use the advertised surfaces and bounded POST /v1/libraries/{id}/browse with a fixed base membership predicate (collection, memberOf, person, list, or the saved view's query). Intersect viewer filters with that base; clearing filters must never escape the group. Keep any person-role scope honest rather than pretending a general person predicate expresses a selected credit role. Render advertised applicable fields/operators and sort/direction, not a local short list. Genre/category links should retain the base query while exposing all valid title controls.
- **W2 — tabs still override available sort.** packages/client-core/src/presentation/library-tabs.ts declares Recently aired fixedSort, and Library hides its Sort control. Change this into a default sorting preference and consume it on every client; Recently aired may default latestAired desc but the viewer must be able to choose any valid sort. Music Genres/Playlists and other custom-rendered tabs also need the common applicable toolbar rather than bypassing it. Preserve per-tab memory, explicit-link scope and unblanked refresh.
- **W3 — Undo where the mark was made.** EntryActions card/menu marks do not expose the container action's real undoToken/undoUntil after the menu closes. Use container-state.ts undoContainerState and POST /v1/containers/{kind}/{id}/personal-state/undo while the receipt is valid; offer the same confirmation/toast affordance on show/season/album/book action surfaces. Do not simulate Undo by marking unwatched.
- **W4 — remove subtitle fallback only after caller conversion.** player/options.ts retries failed session PATCH through SubtitleService.choose, which invokes PUT /v1/items/{itemId}/playback/{session}/subtitles. Make ordinary title selection/off/timing solely a session PATCH (subtitles.trackId/delivery and subtitleOffsetMs). Handle its actual conflict/refusal without a second mutation path. Management/read/opaque-original/grant routes remain necessary. The standalone route also serves social-control/current media-generation callers; trace those before removing the mutation handler/contract and update canonical service consumers plus native copies together.
- **W5 — exact language/label consolidation.** Some raw fallback player/owner labels and native staged strings remain. Resolve them in packages/i18n while checking current native new-words/player-words readers; no per-screen English stand-in should be called a completed language feature.
- **W6 — capacity and visible acceptance.** Browser/player/owner console under simultaneous scans/conversions still needs bounded saturation and recovery observation. No visual/browser session was run by this source lane.

## Phone and tablet (iPhone, iPad, Android)

The shared native file paths below are under main-native/apps/native/src. This worktree is read-only to the lane.

- **N1 — Integrations parity.** Settings/ServerPage has APIKeysPanel but no personal access-token, member-token administration, webhook or assistant panel; shared settings-structure explicitly limits these to web. Provide personal token create/list/revoke via /v1/me/access-tokens; owner token list/revoke via /v1/admin/access-tokens; webhook list/create/edit/delete/test/delivery log via /v1/admin/webhooks and its id/test/deliveries routes; assistant connection instructions with local POST /v1/mcp. Reuse api-access.ts and the copy-once credential display. Native key creation currently chooses a scope preset but has no expiry choice; expose expiresAt within the server's bound. Add the integration rate field apiRequestsPerMinute to the same owner surface. API keys, tokens and webhook data are local, not Hosted.
- **N2 — People actions and full limits.** screens/server/PeoplePanels.tsx already has per-account device approval/block, role/library/basic stream/rating/label edits—do not rebuild those. Add member-deletion preview/confirmation using GET /v1/direct/members/{id}/deletion-preview and DELETE /v1/direct/members/{id}; honor returned revision/preview, role restrictions and erased-history explanation. Expose the schedule/timezone/allowed-hours editor instead of merely saying the saved schedule is kept. Add supported ratingSystem selection/age readback where the contract permits it. No country table or per-library unrated policy exists to invent in the screen.
- **N3 — Library identity/metadata owner tools.** LibrariesPanel already selects provider and online/local agent. Add a library-wide attention page and exact waiting count via /v1/admin/metadata/attention and /counts, following returned entry.actions, revisions, candidate identity/confidence/reasons and parent navigation; pending work is not waiting-owner count. Add global /v1/admin/metadata/settings (localArtworkLimitBytes, includeAdult/provider credential presence, measured usage) and provider credential controls. Add whole-library removal via DELETE /v1/admin/libraries/{id}, using the actual library revision and typed confirmation; current removeSource removes only a folder. Add merge via POST /v1/admin/titles/merge; split through the filtered inventory file lookup and /v1/libraries/{id}/inventory/{objectId}/separate; per-version delete through POST /v1/admin/items/{id}/files/{fileId}/delete/preview then DELETE /v1/admin/items/{id}/files/{fileId}. The server now has a filtered asset lookup; do not copy the old whole-file-list enumeration from early console reports. Show edition, file id and actual missingSince on the title file list. Do not claim episode split works: supported split kinds are advertised by title-identity.ts.
- **N4 — Runtime/diagnostic owner panels.** ServerPage lacks the web resourceUsage/storageStatus/downloadUsage/setAside/clientReports/restart/remoteControls panels. Add GET /v1/admin/resource-usage, /v1/admin/storage/status, /v1/downloads/usage, profile revoke POST /v1/admin/downloads/revoke (repeat while more), background pause POST /v1/admin/maintenance/background-pause, existing restart operation, set-aside diagnostics, and client-report list/read/delete/download /v1/admin/diagnostics/client-logs. Reuse corresponding canonical services and server-admin forms; requested/effective/restart-needed must not imply an FFmpeg override is already in use. The diagnostics submit row already exists and asks before sending.
- **N5 — Live source configuration parity.** LivePanels.tsx hides guideUrl in the HDHomeRun add form even though the server accepts XMLTV. Add guide attachment/mapping and saved-source group/channel choices from the typed source-choice endpoints; persist enabled channel keys and filters, not the hashed viewer channel id. Include owner guide horizon/budget fields, startup/live buffer behavior, duplicate/skip diagnostics and actual guide mapping readback. Preserve independent tuner input sharing and private DVR intent; current per-source buffer/filter/logo/user-agent rows need no duplicate rebuild.
- **N6 — Offline rich bundle and signed-out player.** bridge/offline-downloads stores the files, and player/offline-extras/usePlayer already uses chapters/markers/text; those older handoffs are closed. Remaining: expose DVB (the local form preference currently only considers ass/pgs/vobsub/vtt), VobSub companion .sub and ASS attached-font files in OfflineExtras and pass them to the engine render command. Preserve exact local paths, both-file association and attachment names within app-owned storage. Route signed-out launch through the same two-engine/local-extra player: Native/PorticoDownloads.m still starts AVPlayerViewController, and Android PorticoDownloads.kt starts PorticoOfflinePlayerActivity/VideoView. Original MKV/rich captions then need mpv where supported. iOS changed transfer URL/cellular policy may still restart a partial file; completing true continuation is native background-download work. No next-episode offline queue currently exists.
- **N7 — Real container Undo.** Show.tsx and Entity.tsx mark watched but expose no receipt-based Undo. Use container-state undoToken/undoUntil and the exact undo endpoint/service on show/season/album/book action and success UI. Keep the thirty-second window, single-use token and genuine-change refusal.
- **N8 — Common grouping controls and full saved view playback.** Library.tsx/BrowseFilters already render a broad toolbar on ordinary pivots. Entity.tsx, Show.tsx, Person.tsx, HomeRow.tsx, Saved.tsx and grouped/genre/category destinations need the same applicable controls with fixed membership predicate and common browse API described in W1. Do not let a changed filter replace the group. Recently aired must allow overriding its default, as W2. Saved-view Play still enrolls only first 500 titles; a full bounded server queue selector is a separate server prerequisite (S1 below), not something a screen should solve by enumerating unbounded results.
- **N9 — Settings that have no native effect.** Card size preference appearance.cardSizePercent has no native reader. Apply it through native layout/windowing metrics or hide the control only if the platform truly cannot honor it; current code has no such technical prohibition. Add caption Size/Background to the listening/playback Settings page, not solely PlayerOptions; keep system overrides and stored scope accurate. Keep new owner rows reachable after adding their panel renderers; do not blindly remove platforms:['web'] and render empty panels.
- **N10 — Player-setting re-plan caller.** After capability-publisher successfully publishes a changed engine preference, replace player/engine-replan.ts queue.check(true) with PlaybackService.changePlayback({replan:true}), with current position carried by the session bridge. The new server/shared path preserves version/quality/audio/subtitles/state/queue and reuses current playing-device authority. Do not use this for local downloads or live channels. Re-sync canonical packages first.
- **N11 — Catalogue and fresh UI finish.** Delete staged English stand-ins in app/new-words.ts and player/player-words.ts only after adding exact catalogue keys. Native newly arriving cards currently appear in place rather than fading; qualify a bounded animation wrapper for TV focus/Vega before using it. Viewer/role/server changes must drop prior-private documents; outside-hours state must resume when access returns.
- **N12 — Receiver/Play on transport controls.** The native Play on surface offers Start over but lacks the complete pause/resume/current seek/readback/control list now available at GET /v1/playback/controls and session control APIs. Render actual playing-device position/actions/revision; use remote-playback and session-control consumers and retain same-profile authorization. Do not invent a position from the controller phone's local player.

## Television (Apple TV, Android TV, Fire TV)

- **T1 — apply W1/W2/N7/N8 to TV screens.** Remote-focusable sort/filter chips, group-scope intersection, direction, reset and receipt Undo are required on the same native Show/Entity/Person/HomeRow/Saved/Library screens. A remote is not a technical inability to sort, filter or undo. Keep stable focus while quietly refreshing; no current screen blanking.
- **T2 — owner action parity.** Shared server settings pages often restrict owner surfaces to web/phone, leaving TV without integration/metadata/diagnostic/delete actions. Use the same N1–N5 actions in focusable sheets or explicitly document a true platform impossibility; do not treat screen work as an allowed platform exception. Secure credential copy-once may use the existing Share/readable presentation where actually supported.
- **T3 — native player qualification.** Apple TV mpv is SDR/tone-mapped; native direct/remux preserves HDR when possible. Android/Fire TV need physical HDR direct-output/HDMI layouts/passthrough/frame-rate matching tests. No mpv PiP/AirPlay video/live/music functionality is claimed; those declarations/controls must stay hidden where absent.
- **T4 — TV current-entry changes.** Wire N10 on TV too, retain queue and pause/position across engine-setting changes. Native text/rich font fallback and system captions require the same offline/streaming parity in N6/N9.

## Vega

- **V1 — native transport blocker, before screen completion.** apps/vega/src/bridge/transport.ts sdkRequestAllowed only admits public safe reads; authenticated bodies/headers are refused before dispatch, including sign-in/session/control/events. Its installed NativeNetworkingKepler/RCTNetworking API cannot enforce pre-follow redirect refusal. Supply a native transport that refuses redirects before transmitting a body or credential and provides the bounded stream/cancellation semantics. Checking responseURL after transmission or adding speculative SDK options is insufficient. Do not relax this protection. This is outside both writable checkouts.
- **V2 — inherited common screens.** Once safe transport exists, run W1/W2/N1–N5/N7–N12 and T1/T2 through the Vega shared-screen overrides and focus engine. Do not advertise parity merely because imports resolve.
- **V3 — honest capability and feature gates.** SDR/stereo are current conservative declarations; no mpv, no background downloads where the SDK cannot supply them, no absent casting/engine features. Qualify real panel HDR/audio output before broadening the document; the server already tone-maps and plans from current published capability.
- **V4 — runtime acceptance.** Guide source-buffer timeline ranges, slow delivery, request IDs, system Home pause and Menu/Back focus need an actual Vega device. Source tests did not establish authenticated app operation.

## Server / cross-lane follow-ups not implemented here

- **S1 — full saved-view server queue selector.** Existing first-party native Play takes first 500 items. The present queue source contract has no fully bounded saved-view/query selector that streams membership into durable queue capture with current viewer restrictions and revision/ordering semantics. This is a substantial queue/capture feature, not a safe screen-only mechanical change; it remains an exact implementation gap. No unbounded client enumeration was added.
- **S2 — retire the standalone subtitle mutation coherently.** Ordinary session PATCH and PUT media-generation subtitle selection coexist. Canonical SubtitleService/social-control consumers and both screen wrappers must move before deleting the duplicate mutation route/contract/ceiling path. This lane did not remove a still-called route behind read-only native consumers. Existing authority/ceiling checks remain intact; W4 states the precise caller changes.
- **S3 — optional unapproved expansions (dropped by Justin 15 Oct: spoiler controls, NFO/artwork writing, country rating tables, per-library unrated/kids allow-list, unified profile/account label rules).** No spoiler setting/suppression, NFO/artwork/subtitle write-beside-media export, country-specific rating tables, per-library unrated switch or allow-list kids mode was invented. These were not explicit programme decisions. If selected later they require server, shared client and all screens together.
- **S4 — generic retention allegation.** Identity/inventory/event/client-report retention and incremental reclamation are present; no independent all-table proof of every insert-only table's bound was completed. B-07.15 is not declared universally closed.
- **S5 — shared tab default versus fixed sort.** The common fixedSort property and all web/native consumers must change together, as W2/N8/T1 specify. It is a presentation/screen integration gap left exact in this report, not a claim that the common browse API lacks a sort.
- **S6 — native package refresh.** Integrator must run the prescribed shared-packages script from the writable native integration lane and verify source revision/hash equality. Changes here include the new replan field/consumer, guide fallback removal and bundle tolerance; this lane cannot write main-native or commit its mirror.
- **S7 — source and legal release obligations.** No licensed native/WASM notices were removed. Qualify/store the exact corresponding source inventory with release artifacts and obtain the Apple distribution rights/store decision. No licence advice or public release is inferred from static code.

# Test notes for other environments

## Executed in this lane

- GOFLAGS=-p=2 go build ./... from server: passed after the fixes and new PATCH request.
- Touched-package go vet for internal/playback, internal/playbackv1, internal/identity, internal/subtitles and internal/httpapi: passed.
- HTTP selected tests: QualityDefaultsAndAutomaticSteps; ThroughputEstimate; ReplanUsesCurrentCapabilitiesAndKeepsChoices; PlayerFailureReplansInPlace; CapabilityOverridesAndEngines; SubtitleReplanRoutesHoldToTheMembersCurrentCeiling; ProductionRouterConstructsWithEveryMediaRoute; AllProductionRoutePatternsAreCompatible: passed.
- Identity selected tests: WrongPINsDoNotLockOtherDevicesOut; DirectPINFailureDebtSurvivesServiceReconstruction; PINLockoutIsScopedToTheDeviceAndEscalates; AProfilePINIsCheckedWithoutHoldingTheWriter: passed.
- Planner selected tests: AutomaticEngineChoice; SubtitleNeedsBurnIn; CodecQualityLaddersAndOriginalQualityMode: passed.
- Joined E2E regressions: DiscoveredASSCachePreservesStyleAndOriginal; StyledResourceCanDeliverPlainTextAndRetainOriginalAfterMetadataEdit; SelectedAssetReturnsTheScannedVersion; PlaybackV1ConversionCapacityUsesRetryableEnvelope; ConversionAdmissionIsTypedRetryable: passed. EveryConversionCreatingOperationDeclaresCapacityRefusal also passes, including prepare-next.
- Single-file shared regressions: guide-directory.test.ts (9), guide-legacy.test.ts (8), downloads.test.ts (22), v1-queue-player.test.ts (36), playback-messages.test.ts (4): all passed. Current guide fixtures replaced old-server compatibility fixtures; transport cancellation, authorized opaque handles and bounds remain tested.
- npx --no-install tsc --noEmit -p web/tsconfig.json: passed after all shared changes/new field, including the final capacity message.
- node packages/client-core/scripts/check-generated-routes.mjs: passed.
- GOFLAGS=-p=2 go run ./cmd/apigen: completed; expected replan field and conversion-capacity error declarations in the generated artifacts.
- git diff --check: passed. No full suite, full web/native build, Xcode/Gradle/Docker/package build, physical device, GPU, tuner or capacity test was run.
- One new HTTP regression initially named the stored SQL column request_json instead of request. Corrected the test and reran; production behavior assertions had already passed before the final stored-action check. A guide fixture initially expected the cached legacy transport's five requests; the current viewport legitimately makes two bounded directory pages while total is unknown plus four named programme blocks. Updated the assertion to verify exactly that bounded behavior.

## Acceptance someone can follow cold

1. **Automatic quality, browser and each native platform.** Use a fresh baseline database and current packages. Save Automatic for home, Original remote. Start a high-bitrate film with an explicit low throughput estimate, then repeat with omitted quality/saved Automatic. Pass: initial presentation is Original; measured shortfall/stall later causes a requested lower rung; recovery raises it. Fixed quality is an actual ceiling. Failure: startup silently chooses lower, quality is reset on re-plan, or queue/position disappears.
2. **Capability re-plan.** Publish a two-engine device document, start a multi-entry queue, choose a version, Japanese audio, subtitle/off and a fixed quality, pause near a minute. Change device engine preference, publish successfully, PATCH current session with replan:true/current If-Match and positionMs. Pass: same session/queue/entry/state and choices, one new generation using new engine at current position. Failure: new queue/occurrence, old engine, dropped track/quality/version, unauthorized controller capabilities used, or no revision guard. Native UI must first be wired as N10.
3. **mpv versus native conversion.** On Apple/Android/TV use a source native cannot decode but mpv can copy into its accepted streaming container (or exclude mpv's direct route). Pass: native direct/remux still wins ordinary files; this source uses mpv copy/remux and original streams; exclusions are not retried; when both would need video conversion, native remains preferred. Confirm real subtitles/seek/HDR fidelity on the actual chosen engine.
4. **PIN policy.** With a secondary PIN-protected profile, issue wrong PINs from at least 25 distinct authenticated installations, then immediately use the right PIN on another installation. Pass: right PIN succeeds; the offending device's persisted retry delay still applies; password/PIN revision/trust and role/profile/library limits remain. Restart during debt and repeat on the offending device. Failure: profile-wide denial, lost per-device debt, or bypassed proof/revision.
5. **Bounded guide.** Use more than 2000 channels from live plus Library Channels, open middle/final pages and surf ends. Pass: current directory/summary endpoints only, at most visible named channels per programme request, cancellations reach transport, source change quietly recovers. Return 404/405/503 once from directory/summary. Pass: retained screen/retry, no whole-lineup guide enumeration.
6. **Bundle tolerance and security.** A ready bundle with one bad chapter/marker/audio description and other valid extras should retain/use valid rows and count only the malformed rows. Invalid grant traversal, preparation mismatch, receipt/signature/byte mismatch or excessive collection length still fails. On device, erase source/claim and play after server cleanup; artwork/title/text captions/chapters should still work. Rich companions/fonts/DVB and signed-out common engine need N6 first.
7. **Live/DVR on real tuner and storage.** Tune two profiles plus one recording on one channel. Pass: one upstream acquisition, independently canceled readers/output/conversions; both profiles' same-airing recordings share immutable bytes but neither sees the other's intent. Delete/cancel leader mid-capture and restart. Pass: authorized follower finishes, no canceled owner's publication, final links retain bytes until last deletion.
8. **Networks and server OSes.** On Windows LocalService/custom service account, macOS daemon, Linux and plain Docker: install/add linked/root library, reboot/upgrade/restart/restore, separate cache/backup destinations; plain HTTP public IPv4/global IPv6/overlay and hostname TLS proxy with explicit trusted forwarding. Pass: allowed HTTP never refused by address classification; home list alone controls locality; Hosted advertises TLS only; control UI remains authorized/reachable.
9. **Load/recovery.** On modest and target hardware run scans/matching/analysis with concurrent direct streams/conversions and browsing. Observe response latency, writer checkpoints, reserve saturation, cancel/retry and recovery; the old million-item/hundred-stream goals remain unestablished. Pass needs an owner-agreed bound, not “go build succeeded”.
10. **Apple/Android/Vega release.** Build signed actual artifacts outside this lane. Apple scene manifests now exist: validate launch/deep links/mode switching/Keychain reinstall and lifecycle. Android validates native library packaging/release/seek/range resume/HDMI. Vega must first obtain safe credential networking (V1); otherwise authentication fails by design. Package licence/source inventory and Apple distribution grant require separate qualified acceptance.

# Not finished

- End-to-end product parity is not complete. W1–W6, N1–N12, T1–T4 and V1–V4 are exact handoffs, with routes/fields and current implemented parts distinguished.
- Full Saved-view durable queue membership (S1), coordinated standalone subtitle mutation retirement (S2), and fixed-sort presentation/screen conversion (S5) remain implementation gaps. They were not filled with unbounded enumeration, an unsafe route deletion, or edits to read-only native screens.
- Optional unanswered feature expansions (S3) remain deferred; this is not an owner-approved decision to promise them at release.
- This decision trace did not independently reproduce all 588 bug-register incidents or prove every table/retained screen has a lifetime/memory bound (S4). The report contains dispositions for all 191 explicit/range-expanded bug IDs in the assigned final decision document; source checks do not close runtime incidents.
- Native canonical package sync (S6), physical codecs/HDR/audio/fonts/tuner/filesystem behavior, sustained capacity, release packaging/store/legal acceptance (S7) remain.
- No original checkout, native checkout, Hosted code or Git metadata was written. All code changes are local snapshots, not commits. No push/deployment occurred.

## Snapshot receipt and integration instructions

- Base: ce344737a44d6593a4f7cd9809f404b4aabbb024, branch audit-implementation/decision-audit.
- Snapshot 001: Start Automatic quality at the original.
- Snapshot 002: Keep automatic playback faithful and guide reads bounded.
- Snapshot 003: Replan player capabilities without replacing the session.
- Final snapshot 004: Preserve playback choices and subtitle fidelity. 36 tracked files, 101034 patch bytes, zero new files. SHA-256: 1a36000ef344a9e730c7fcab76409bbc1899efed94eca198cd6decc5e8719b80. Snapshots are cumulative; apply 004 once, not all four. The current tracked diff exactly matches 004, and there are no untracked files. No worktree change was made after this final snapshot.
- The final result also contains reviewed E2E server fixes already in e2e-server snapshot 004. Deduplicate those source hunks during integration; this lane's own prepare-next declaration/test extends that fix.
- Native package copies were not changed. Run the standard canonical shared-package sync in the native integration lane after merging and wire N10 to the new action.
- Final report correction: native library-folder removal is not whole-library removal. D53/B18.5/N3 now reflect the actual LibrariesPanel code.

# Merge with the base

- `server/internal/httpapi/audio_capacity_test.go`: kept the base's typed retryable-capacity response tests and its synthetic public route's `ScopeOpen` declaration. Retained this lane's registry coverage regression for start/change/create/advance/prepare-next; the empty incoming side did not replace that behavior. All five production operations already declare `ScopePlayback` and `conversion_capacity` on the merged base. No removed trash setting or HTTPS policy was reintroduced.
- Read the named networking, identity, API-platform, providers/matching, music/audiobooks, web-app, web-player and naming reports. Regenerated API/contracts with apigen and schema artifacts with schema-sync; no generated file was hand-merged. No Git staging, commit or new snapshot was attempted. The earlier final-snapshot receipt describes the pre-merge lane result, not the present merged tree.
- Verification passed on ehler-homelab-mini-1: whole-worktree conflict-marker scans with rg and grep; `git diff --check`; `GOFLAGS=-p=2 go run ./cmd/apigen`; `python3 scripts/schema-sync.py` (including persistence check); `GOFLAGS=-p=2 go build ./...` for Linux amd64, Windows amd64 (`GOOS=windows GOARCH=amd64`) and macOS arm64 (`GOOS=darwin GOARCH=arm64`); `GOFLAGS=-p=2 go vet ./internal/httpapi`; `npx --no-install tsc --noEmit` from web; and the generated client-route check. Regeneration made no additional unstaged generated-file changes.
- Targeted HTTP tests passed with `-count=1`: ConversionAdmissionIsTypedRetryable; PlaybackV1ConversionCapacityUsesRetryableEnvelope; EveryConversionCreatingOperationDeclaresCapacityRefusal; EveryRouteDeclaresACredentialScope; ProductionRouterConstructsWithEveryMediaRoute; AllProductionRoutePatternsAreCompatible; PlaybackV1QualityDefaultsAndAutomaticSteps; PlaybackV1ThroughputEstimate; PlaybackV1ReplanUsesCurrentCapabilitiesAndKeepsChoices; PlaybackV1PlayerFailureReplansInPlace; PlaybackV1RejectedReplanKeepsWorkingMediaAndProgress; PlaybackV1ReplanRecordsCurrentLocality.
- The file remains unmerged in Git's index until Justin stages the resolved contents. This is expected because staging/committing is explicitly prohibited for this job. Cross-builds establish compilation, not Windows/macOS runtime acceptance. Context-sync again failed at its read-only state lock; canonical project and verification guidance was read directly.
