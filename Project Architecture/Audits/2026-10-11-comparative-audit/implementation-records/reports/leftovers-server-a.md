# Outcome

All 27 assigned coverage entries were checked against the register, domain findings, verification directions, fix log and current code. Both high-priority metadata findings have working server implementations and focused regressions. TMDB anime supports official and show-owned alternate orders with explicit numbering offsets, retaining local absolute numbers and original provider coordinates. Adding one episode does not fetch or republish its fresh siblings; expired episode documents support conditional refresh and region-specific reuse.

Secondary profiles cannot manage account device/remembered-account state. Retired session authority and viewer-only grants are removed from identity admission and downstream gates. Owner APIs expose and clear remembered playback failures, including final-conversion failures. Legacy local XML retains usable evidence. Optional preview analysis tolerates bounded corruption, corrects display geometry and handles HDR. Backup snapshots have time, WAL-growth and disk-reserve budgets. Production watchdog checks diagnose a stuck writer. The server schema is one final baseline.

Linux compilation, focused regressions, production-router construction, generated contracts, schema checks and web TypeScript checks pass. No commit, push or deployment was made. Snapshots 001–003 are saved; 003 is the final cumulative snapshot. The historical Apple sign-out remains unproven and unresolved.

# Decisions I made

- Alternate anime numbering is explicit, never guessed from gaps. `local number + offset` selects a position in the chosen provider order. Official counting excludes specials; alternate groups follow the owner's selected group's ordering. Seasonal files in a mixed show use the base order without the absolute offset. Ambiguous, foreign, duplicate or out-of-range mappings are refused; an affected absolute file settles to `needs_order` while other files continue.
- Preserve the documented sustained-rewatch semantics. The programme does not replace that contract: completion clears only after a real unfinished rewatch starts, not a brief replay.
- Primary profiles/account-tier sessions control account Devices and remembered sign-ins. A valid secondary session gets 403, not a sign-in failure. Any viewing profile may still approve a television, with its own role as required by the programme; PIN-free profile selection is unchanged.
- Portico Account sign-ins remain local account sessions. Removed legacy Hosted-family issuance/horizon helpers, device/restriction/Top Shelf bypasses, downstream authority branches and their obsolete positive fixtures. Active account assertions, setup auth modes, claim management and historical policy test infrastructure are preserved. Central erasure receipt namespaces are not session admission.
- An explicitly off subtitle policy wins. Otherwise one untagged forced track may accompany known audio; multiple untagged forced tracks require manual choice. With subtitles explicitly enabled and no language list, the sole non-forced default is selected. Defaults never replace a requested language.
- Optional image/trickplay/scene analysis allows FFmpeg recoverable decode errors with a 5% error-rate ceiling; exact audio conversion/loudness/fingerprint recipes retain `-xerror`. Output, source, process and time bounds remain. Source-change failures still discard all results.
- Complete prepares viewer navigation/audio features rather than automatic checksum and diagnostic fingerprint passes. Custom retains those explicit operations. Basic remains the default. Separate useful video/audio decodes remain; no measured throughput claim is made.
- HDR10/HLG previews use the existing software tone mapper. Dolby Vision without a compatible base layer requires the qualified Vulkan/libplacebo path and hardware sandbox grant; without it the optional preview stage fails rather than publishing wrong colours. SAR correction uses the pinned FFmpeg scale filter's `reset_sar=1`.
- Retire the production `PORTICO_FINITE_VOD` switch. It is no longer read at startup. The experimental internal/test implementation and its protective limits are not a released owner feature.
- Backup copying is allowed a five-minute context, at most 64 MiB of additional allocated WAL, and a 128 MiB reserve on source/destination volumes, plus remaining destination bytes. Budget checks are spaced by 250 ms. Pressure failure leaves a failed partial backup, releases the snapshot and permits a later retry; existing backup cleanup owns partial folders.
- Preserve snapshot consistency and bounded single-writer transactions. Do not invent a measured contention defect for B-07.18 or detect/block an owner's external database viewer for B-07.19.
- Required context sync failed opening its read-only lock file. Existing canonical project standards/verification/Portico guidance were read on `ehler-homelab-mini-1`; freshness beyond the loaded revision was not established.

# Bugs

| ID | Outcome |
|---|---|
| B-01.14 | Already fixed: `httpapi/onboarding.go` delegates `privateSetupPeer` to the same configured `requestIsRemote` used by playback; the networkpolicy parser remains shared. |
| B-01.21 | Fixed: Devices/EditDevice and Remember/ForgetBrowserAccount require account management; secondary-profile regression covers all four. TV approval and PIN-free profile switching remain deliberate programme decisions. |
| B-01.22 | Fixed for session admission: removed Hosted family issuance/renewal/horizon, fail-open restrictions and authority branches, viewer_only declaration/propagation, and legacy per-profile PIN counters. Retired-authority, quick-connect and local-family tests pass. Development-only policy fixtures and central erasure namespaces remain separate from session admission. |
| B-01.23 | Not fixed: historical simulator incident has no correlated request trace or established cause. V01 refutes the guessed single-profile-cleanup bug and the current native source gates cleanup on accountGrant. Added/tested `shouldRetireAccountSession` defensive same-family check; native adoption and actual incident reproduction remain external work. |
| B-02.8 | Not a correctness defect under the current saved-state contract: sustained unfinished rewatch intentionally clears completion. Kept `catalog/personal_activity.go` semantics. |
| B-02.16 | Fixed server/core gap: owner GET/DELETE diagnostic/reset APIs and core parser/service; final video-conversion failures now persist while reporting no further escalation. Existing six-hour generic/seven-day certain, per-engine/family learning is retained. Owner screen remains needed. |
| B-02.22 | Fixed production exposure: startup no longer reads `PORTICO_FINITE_VOD`; an environment variable cannot force all HLS streams through that experiment. Internal experiment limits remain. |
| B-04.10 | Already fixed: CanonicalAsset text-only fallback salvages inline-attachment ASS without rich/burn admission; tolerant event processing retains usable dialogue and original bytes. Inline-attachment/tolerant regressions pass. |
| B-04.18 | Fixed: one untagged forced track fallback and sole default selection with explicitly enabled/no-language preference; explicit off and preferred-language tests pass. |
| B-04.23 | Already fixed: `assets.CleanChapters` isolates invalid entries and retains a bounded timeline with partial warnings; chapter regression passes. No unsupported origin subtraction was invented. |
| B-04.28 | Fixed for optional visual analysis: recoverable corruption no longer aborts on its first error; 5% ceiling and publication bounds remain. Exact audio recipes stay strict; real damaged-file qualification remains in test notes. |
| B-04.35 | Reduced default Complete cost: checksum/diagnostic fingerprint passes are explicit Custom choices; actual persisted Complete profile and Custom override regression passes. Shared-decode optimization remains a future improvement, not a measured capacity claim. |
| B-04.38 | Fixed command/fact wiring: preview source colour facts, HDR tone mapping, qualified Dolby Vision reshaping and display-aspect correction reach chapter/trickplay/scene graphs. Closed-recipe tests pass; visual/GPU acceptance remains external. |
| B-04.39 | Already fixed: current `web-media-player.ts` ignores HLS subtitle offset changes below 20 ms; the former bridge path is gone and an existing fragment-jitter regression covers it. No browser-flicker reproduction is claimed. |
| B-05.5 | Already fixed: `storage/inventory_page.go` skips unreadable/out-of-root symlinks with protected warning issues; root containment remains enforced. Inventory regression passes. |
| B-05.29 | Completed remaining parser gap: bounded US-ASCII/ISO-8859-1/Windows-1252 conversion for NFO/OPF, bounded UTF-8 partial audio fields and warning propagation. Existing NFO list/optional-field/trailing-URL salvage is retained; DOCTYPE, byte, node and depth bounds remain. Parser regressions pass. |
| B-05.31 | Already fixed: `commitAudio` calls `naming.DiscFolder`, recognizes labeled disc folders, and release/tag grouping joins equivalent albums across folders. Existing grouping regression passes. |
| B-06.20 | Already fixed: TMDB portrait acquisition uses h632 rather than original and automatic retained source variants are shared; `TestRetainedSourceArtworkIsAcquiredOnlyOnce` passes. Explicit owner refresh still fetches current content. |
| B-06.21 | Fixed: official/alternate-group absolute mapping plus signed explicit offsets, owner selection validation, original-coordinate provenance, local-number preservation and per-file needs-order isolation. Provider and full selection/publication regressions pass. |
| B-06.22 | Already fixed: TMDB includeAdult defaults false and `SetIncludeAdult` is wired through the owner metadata settings. |
| B-06.23 | Already fixed main bounded-cost findings: provider watcher is one second, artwork watcher two seconds, and artwork-history GC streams rows. No claim that image auth/selection queries are eliminated or measured at scale. |
| B-06.24 | Completed: region-specific episode cache, conditional expired-document refresh, retained freshness/budget gates and existing fresh-sibling fan-out skip. Regression proves one new file yields one request/publication; changed language/region/age invalidate reuse. |
| B-07.13 | Fixed budget/release gap: bounded snapshot context, allocated-WAL growth/disk-reserve checks and clean abort. Injected-pressure release and concurrent-write backup regressions pass; multi-gigabyte live pressure requires separate qualification. |
| B-07.14 | Fixed: folded every post-baseline development migration into localized final CREATE definitions and deleted 0002–0011. No startup personal-history aggregation, facet backfill or anime rematch is shipped. Schema sync/checks pass. |
| B-07.16 | Fixed: watchdog checks actual heldSince in ordinary production; one log per stuck hold names class/entry point and health becomes degraded. No waiter/strict mode is needed; release recovers. Regression passes. |
| B-07.18 | Old 64-unit default is already mitigated: current `compactcatalog.BatchLimit` is 16, domains checkpoint/fan out boundedly and foreground pacing runs after release. No current excessive hold was established; representative weak-host measurement is prescribed below. |
| B-07.19 | Deliberate owner choice, not a demonstrated defect: keep SQLite concurrency controls and add the quiet README note to inspect a backup copy. No external-program detection or blocking. |

# API changes

- Added registered, typed `GET /v1/admin/playback/route-failures`: owner access, admin key scope, bounded latest 100 active failures, `{items, more}`; each row has sourceId, engine, route, code, detail, reportedAtMs and expiresAtMs. No hashes, tokens, paths or viewer credentials are returned.
- Added registered, typed `DELETE /v1/admin/playback/route-failures/{sourceId}`: owner/admin scope, idempotent 204; clears learned routes for the selected source across sign-in families. Invalid source identifiers return invalid_request. Authorization and production-router tests pass.
- Existing show metadata selection keeps its typed `order` string. It now accepts bounded `tmdb-absolute:<official-or-tmdb-group:ID>:<signed-offset>` for TMDB absolute mapping. No JSON shape change; current-candidate and provider-owned-order validation remain. `tmdbAbsoluteOrder` constructs it in client-core.
- Devices/EditDevice/RememberBrowserAccount/ForgetBrowserAccount retain shapes but now require primary/account management. Valid secondary profiles receive 403.
- `/v1/direct/refresh` retains its wire contract; obsolete internal Hosted verifier argument was removed and local account admission remains mandatory.
- No public routes removed in this lane. API generation refreshed contracts/OpenAPI. No new viewer-facing route was added, so viewer restriction classifications are unchanged.
- Core additions: playback failure-memory parser/read/clear services, anime order construction and defensive account-session cleanup predicate, with a focused test file.

# Schema changes

- One `0001_baseline.sql`; removed development migrations 0002 through 0011 and set embedded current schema version to 1. Final existing schema behavior is preserved: cast recovery columns/index, libraries.position, show settings, play history/count tables, converted flag, facet triggers, TMDB default policy trigger and positive recommendation seed index; dropped personal reaction/unused feature index remain dropped. Empty-install seeds are preserved. No compatibility upgrades or data backfills were added.
- Removed direct_profiles.pin_attempts/pin_locked_until and viewer_only grant enum; cast_target remains.
- Existing explicit local/Hosted authority CHECKs now accept only local authority. Account/claim/auth-mode Hosted settings remain meaningful and are not removed.
- playback_route_failures admits video_conversion and has a reported_at_ms index for owner diagnostics.
- Ran schema-sync after changes; known columns/objects are refreshed and Schema/KnownSchema checks pass.

# Owner or viewer settings added or wired

- No new settings fields. Existing Complete scan tier now prepares navigation/audio features without checksum/diagnostic fingerprint passes; defaults remain Basic, and existing Custom operations preserve owner overrides. Existing settings/analysis screens already render the server's operation list.
- Existing subtitle policy/default flags are wired as described above; no new switch.
- Existing owner TMDB adult-search setting is already wired, default false.
- Retired PORTICO_FINITE_VOD startup environment control. Failure-memory reset is an owner action, not a new preference.
- Backup/gate bounds are internal safeguards, not a new wall of owner switches.

# Needed from others

1. All owner consoles: show active failure-memory rows and expiry, with a per-source clear/retry action using the new core service. Refresh quietly after clear. The routes work now; no UI was edited here.
2. All show identification screens: offer official/provider alternate order plus an integer absolute-number offset. Explain `local number + offset`; construct the order with `tmdbAbsoluteOrder`, preserve seasonal behavior, and display needs_order per affected file. Existing raw order consumers accept the string, but offset controls are not yet present.
3. Native session workflow may adopt `shouldRetireAccountSession` in acceptDirect/releaseStep/cancelPending as defensive hardening. Do not label that the cause/fix of the historical Apple incident: V01 explicitly says current single-profile cleanup is guarded.
4. Secondary profiles: gate account Devices/edit/remember/forget controls on the existing CanManage profile/account capability. Television approval remains available as the programme requires.
5. Private product contracts need their Complete-pass description updated (libraries-and-scanning contract §§68–75/109–123). The schema instruction is now implemented: no post-baseline development migration files. Private docs are read-only to this lane.
6. Development-only legacy policy Apply fixtures and historical central profile-erasure namespaces still exist outside session admission. Future test-fixture modernization should use current local Portico Account admission; none can issue a retired family through identity now.

# Test notes for other environments

Verified here (Ubuntu personal mini-1):

- `GOFLAGS=-p=2 go build ./...` from server: passes.
- Targeted identity: retired authority, secondary-profile account controls, authorization families and Quick Connect: passes.
- Metadata: absolute official/group/offset provider fixtures; conditional episode request; full offset selection/publication, local numbering and needs-order isolation; freshness/one-new-episode/publication count/language/region tests; retained artwork acquisition: passes.
- Local metadata NFO/OPF/legacy encodings, subtitle choices/text-only ASS, inventory symlink/root containment, cleaned chapters, album grouping and Complete/custom profiles: passes.
- Decoder closed analysis recipes, preview colour/SAR/qualified Dolby Vision, mediaanalysis sprite/source/publication/binding tests, backup pressure release/concurrent writes and production gate/watchdog tests: passes. Existing sprite staging expectation was updated from obsolete per-frame artifacts to the current sheet/descriptor output; its retirement/seal/lease checks remain.
- HTTP Foundation registry, production router, owner failure-memory authorization, diagnostics and retired Cast authority refusal: passes. Obsolete positive Hosted-family fixtures were removed/replaced with refusal checks.
- `go run ./cmd/apigen`, schema sync/Schema/KnownSchema, web `npx tsc --noEmit -p web/tsconfig.json`, and single core test file `playback-failure-memory.test.ts`: passes. `git diff --check`: passes.
- Package vet for metadata/metadataprovider/identity/decoder/dbwork/backup/catalog/mediaanalysis/httpapi: passes. Windows amd64 and Darwin arm64 cross-compilation of backup/decoder/dbwork/identity/metadata: passes. Linux server compilation passes after the final vet cleanup.

Cold qualification steps:

1. Apple incident: simulator or device with clean protected app state, trace direct/Portico Account sign-in, automatic single-profile acceptance, explicit profile choice, cancelled choice, then server restart. Correlate each DELETE /v1/sessions/current with the request's token family and the accepted viewer family; record revocation reason and controller/family IDs without saving tokens. Pass: accepted viewer survives temporary-session cleanup/restart. Fail: cleanup deletes that same family. Preserve competing explanations; two old explicit_signout records alone establish no cause.
2. Preview colour/geometry: use bundled qualified FFmpeg with known square-pixel SDR, anamorphic DVD, PQ HDR10, HLG and Dolby Vision profile 5 files. Select Complete/Custom image stages, open chapter/trickplay thumbnails and compare aspect/colour with a properly rendered player. Verify RGB previews are SDR and SAR=1 with the intended display shape. Profile 5 requires qualified Vulkan/libplacebo; absence should fail only its optional stage, with other catalogue/playback usable.
3. Corrupt preview tolerance: generate a disposable otherwise decodable file with one damaged region, run image/trickplay/scene stages, inspect useful retained samples. A small recoverable fault may complete; a decode error rate above 5%, decoder failure, changed source or exceeded output/time budget must publish nothing for that stage. Exact audio facts must still fail on reported decoding errors. No damaged-file visual runtime test was performed here.
4. Large backup: disposable multi-gigabyte WAL database on both same-volume and external backup destinations, sustained background writes while copying. Record elapsed copy time, source WAL allocation, remaining disk and checkpoint outcomes. Pass: consistent verified backup within budget, or a failed partial copy with snapshot released when its budget trips; foreground writes/reads recover. Never test on demo/real state.
5. Stuck gate: isolated debug harness holds the actual write gate over 20 seconds with strict mode off. After watchdog ticks, expect one holder/class log and degraded health; release it and expect recovery. Do not enable the strict panic path in production.
6. Weak hardware derived workload: representative disposable catalogue, concurrent playback/progress writes and production derived worker. Record by-class max/current hold duration and foreground p95/p99 waits. Any claimed long-batch defect must correlate with real holds, not multiply per-entity whole-drain timing. The old default 64 claim does not describe current BatchLimit=16.
7. Windows amd64/macOS arm64: build the touched packages/server with GOFLAGS=-p=2, then run backup reserve/abort, source symlink warnings and preview tool execution on that OS. Cross-compilation is not filesystem, GPU or player acceptance.

# Not finished

- B-01.23 historical Apple sign-out cause/reproduction is unresolved. No device/simulator was run.
- First-party screens for anime offsets, owner failure-memory reset and secondary-profile capability gating are outside this lane. Core/server pieces are available and exact handoffs are above.
- Real HDR/Dolby Vision/damaged-file preview, multi-gigabyte backup-pressure and weak-host contention qualification remain. Separate consumed analysis decodes have not been consolidated; no capacity/throughput claim is made.
- No full suite or native platform build, deployment or push was attempted. Original delivery ended at cumulative snapshot 003; the later merge edits are described below.

# Merge with the base

- Read the networking, identity, api-platform, providers-matching, music-audiobooks, web-app, web-player and naming reports to identify the current contracts and removed paths.
- `server/api/guide/README.md`: kept the base's linked guide/navigation and current-source distinction, plus this lane's owner playback failure diagnostic/reset and anime-offset documentation. Removed the duplicated old contents list and replaced the base's now-stale missing-generated-routes example with the regeneration instruction; generated artifacts were regenerated from the merged source.
- `server/internal/mediaanalysis/staging_test.go`: kept the base's real persistence-backed test service, one sprite sheet plus descriptor, explicit absence of per-frame images, and sixteen-frame summary assertion. Preserved decoder-retirement, retained-capture sealing and active-reader deletion checks from both sides; removed the duplicate summary assertion.
- Both owner failure-memory routes already declare `Requires: apikit.ScopeAdmin`; no removed trash or mandatory-HTTPS path was restored.
- Context sync again failed on its read-only lock; canonical verification guidance was read directly. No staging, commit or snapshot was attempted.

- Verification passed: no conflict markers in the worktree; `git diff --check`; `go run ./cmd/apigen`; schema-sync and Schema/KnownSchema checks; full `GOFLAGS=-p=2 go build ./...` for Linux, Windows amd64 and macOS arm64; vet for mediaanalysis/httpapi/persistence; focused sprite, registry, owner-route and scope tests; `npx tsc --noEmit` from web. The merge remains unstaged for Justin to commit.

## Backup and persistence merge with the base

- `backup/copy.go`: adopted the base's configurable snapshot timeout, WAL-growth check and report interval, including WAL stat error propagation. Integrated this lane's source/destination disk reserves and page-boundary guard into the same copy engine. Production uses the tighter combined defaults: two minutes, 64 MiB WAL growth, 128 MiB disk reserve plus remaining copy bytes. Removed the duplicate old pressure error in favour of `ErrBackupPressure`.
- `backup/copy_pressure_test.go`: kept all three regressions: real concurrent WAL-growth abort, snapshot deadline, and injected guard abort with subsequent writes/checkpoint/truncation proving release. The injected test now uses the base error.
- `persistence/README.md`: combined the single-baseline/no-development-migrations rule with the base's explicit no-startup-backfill statement; retained the backup-copy inspection note.
- `0001_baseline.sql`: retained the base's resumable notice fan-out table and indexes plus the existing final show/personal/capture schema; selected its descriptive section comment. No removed trash or HTTPS setting was reintroduced. Regenerated API/contracts and schema artifacts with the prescribed generators; no generated file was manually merged. Owner failure routes still declare the admin scope.
- Re-read the eight requested reports and the notice fan-out evidence. Context sync still cannot write its read-only lock. No staging, commit or snapshot was attempted.
- Verification passed: no conflict markers in the worktree; `git diff --check`; `GOFLAGS=-p=2 go run ./cmd/apigen`; schema-sync and Schema/KnownSchema checks; full `GOFLAGS=-p=2 go build ./...` for Linux, Windows amd64 and macOS arm64; vet for backup/persistence/httpapi; focused backup pressure/deadline/concurrent-write, migration recovery, registry and owner-route tests; `npx tsc --noEmit` from web. Native Windows/macOS runtime qualification remains as described above. The resolved merge remains unstaged for Justin to commit.
