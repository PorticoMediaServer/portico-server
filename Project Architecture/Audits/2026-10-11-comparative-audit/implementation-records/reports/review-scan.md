# Outcome

Independent intake review and fixes are complete on `audit-implementation/review-scan`, base `1097d54`, host `ehler-homelab-mini-1`. **Eight confirmed findings: 0 blocker, 5 high, 3 medium, 0 low. All eight are fixed.** One suspected depth issue (S5) was disproved by the existing publication gate.

Scans protect ambiguous empty roots, isolate unreadable children and keep reported removals missing despite stale pages. Immediate absence checks are supervised and bounded. Audio titles/releases and narrations no longer merge on weak shared tags; Unicode-equivalent tags group consistently. Extra ownership cannot rely on truncated evidence, and artwork retirement uses indexed file lookups.

Final validation: 63 selected top-level Go tests pass (including the 657-case naming corpus), three typed/production-router tests pass, Linux build and touched-package vet pass, Windows amd64/macOS arm64 non-cgo affected-package builds pass, and client-core TypeScript passes. No full suite, native build, commit, push or deployment.

# Decisions I made

- Confirm reports against integrated code. Record each finding before editing its affected production path; review continued while targeted checks exposed further cases.
- Exact named-file evidence permits immediate missing publication. Folder-only, overflow, fallback and inconclusive helper evidence preserve the empty-root safeguard.
- The removal proof shares a two-second budget across paths. Slow or denied reads queue the ordinary scan without granting removal authority.
- Music release IDs are authoritative; otherwise known year/edition distinctions matter. Only contradictory evidence splits a retagged version. Missing fields may enrich compatible evidence; manual pins and prior state stay.
- Retain the existing 512-candidate extra-owner bound. Overflow leaves ownership unresolved. This is deliberate conservatism, not a claim to resolve every huge flat folder.
- Correct the old album test that contradicted the programme: the same release ID across folders is one album.
- Context-sync could not write its lock in this sandbox. Readable canonical project/standards/verification guidance was used; no successful refresh is claimed.

# Bugs

Original bug status below is limited to library intake. “Already fixed” means the integrated implementation is present and was checked by source/selected regressions; it is not new native/live-provider acceptance. Playback, lyrics, queue, and other non-intake B-10 entries remain with their owning lanes.

- B-05.1 — already fixed: urgent continuation and foreground yielding pass; no fixed-clock ceiling restored.
- B-05.2 — already fixed: changed-directory coverage is isolated and retried; selected tests pass.
- B-05.3 — already fixed for in-place evidence; S7 additionally fixes stale-page revival after reported removal.
- B-05.4 — fixed here as S8: an unreadable child no longer invalidates its entire parent page.
- B-05.5 — already fixed: dangling/outside links remain bounded protected skips; selected tests pass.
- B-05.6 — already fixed: built-in exclusions retained; S1 prevents unrelated hidden junk authorizing root absence.
- B-05.7 — already fixed: shared numeric-title/year corpus passes.
- B-05.8 — already fixed for film versions; S3 fixes audio releases and divergent retagged versions.
- B-05.9 — already fixed in name re-reading; S3 retires stale audio keys on single-file corrections.
- B-05.10 — already fixed: flat/translated season corpus and episode attachment tests pass.
- B-05.11 — already fixed: typed file evidence wins contradictory folder evidence; selected tests pass.
- B-05.12 — already fixed: resolution/codec evidence remains bounded and corpus passes.
- B-05.13 — already fixed: episode zero and stacked parts attach in the selected integration test.
- B-05.14 — already fixed in contextual classification; S4 fixes ownership inferred from a truncated candidate prefix.
- B-05.15 — already fixed as one truthful unavailable disc-folder title; selected test passes. Disc playback is outside scope.
- B-05.16 — already fixed: sample/extra classification retained and naming corpus passes.
- B-05.17 — already fixed: shared display cleanup retained; corpus passes.
- B-05.18 — already fixed in the integrated source/fsid identity rules; different-filesystem guard passes. Real remount acceptance remains.
- B-05.19 — already fixed: ordinary local scans enumerate file facts; selected in-place-change tests pass. Representative network runtime remains.
- B-05.20 — already fixed: bounded terminal/managed history and expired-cursor checks pass.
- B-05.21 — already fixed: selective enqueue advances across an unchanged candidate page; selected test passes.
- B-05.22 — already fixed: scan listing/sidecar invalidation retained; S9 removes a whole-inventory retirement query.
- B-05.23 — already fixed: default monitoring starts and performs a real limited scan in Linux tests.
- B-05.24 — fixed follow-ups S1/S2/S7: verified removals stay immediate, ambiguity stays protected, stale pages cannot revive them.
- B-05.25 — already fixed by identity: no state-folder trash path restored; no deletion code changed here.
- B-05.26 — already fixed: persistent naming attention is source scoped; owner issue routes remain owner only.
- B-05.27 — already fixed in full source-relative show identity; episode integration/corpus checks pass.
- B-05.28 — already implemented in the integrated Windows identity/helper paths; affected packages cross-compile. Native Windows/ACL acceptance remains.
- B-05.29 — already fixed for tolerant bounded NFO evidence; no size/depth/encoding/structural bound removed.
- B-05.30 — already fixed: owner diagnostics identify paths; typed diagnostic route test passes.
- B-05.31 — already fixed: canonical disc hints and album grouping across folders pass; S3 preserves release distinctions.
- B-05.32 — fixed audio remainder as S6: NFC/NFD tags share artist/album identity. Name-reader Unicode controls pass.
- B-05.33 — already fixed: alias coverage and two-scan regression pass.
- B-06.1 — already fixed: shared reader/corpus and matcher numeric-name path retained.
- B-06.2 — already fixed: confident yearless/fuzzy matching and scoped attention remain; selected scoring/scope tests pass.
- B-06.3 — already fixed: local/owner artwork precedence retained; sidecar carrier handling reviewed.
- B-06.4 — already fixed: old 128 MiB store cap is absent; current owner quota/per-image bounds retained.
- B-06.5 — already fixed: season/episode sidecar routing and changed-sidecar enqueue retained.
- B-06.6 — already fixed: typed TVDB companies project studios/networks; no provider projection changed.
- B-06.7 — already implemented: provider artwork adapters remain; live-provider acceptance not claimed.
- B-06.8 — already fixed: TMDB aggregate credits requested by the adapter; no live response validation here.
- B-06.9 — already fixed: locale predicates and field-language provenance retained; no policy/freshness redesign.
- B-06.10 — already fixed: film/show/episode refresh uses registered typed handlers.
- B-06.11 — already fixed fixed-rate ceiling: shared speed/bounded provider concurrency retained. Actual first-import timing unmeasured.
- B-06.12 — already fixed: retryable provider failures retain recovery; no retry policy changed.
- B-06.13 — already fixed: owner search has claim priority; no claim ordering changed.
- B-06.14 — already fixed: US default policy and explicit empty-region choice retained.
- B-06.15 — already fixed: complete observed NFO retirement passes its selected regression.
- B-06.16 — already fixed for tolerant optional fields; structural/security budgets remain.
- B-06.17 — already fixed: adapter validation isolates bounded optional fields; no provider parser change here.
- B-06.18 — already fixed: provider image diversity ordering retained.
- B-06.19 — already fixed: library-language image ordering retained.
- B-06.20 — already fixed: retained source artwork reuse and bounded portrait variants retained.
- B-06.21 — already implemented: official absolute coordinates retained; alternative/community numbering is not guessed.
- B-06.22 — already fixed: includeAdult owner setting remains false by default.
- B-06.23 — already fixed provider polling/history cost; S9 additionally fixes a scan-driven artwork retirement query.
- B-06.24 — already fixed: episode freshness/fan-out rules retained; no episode refresh design changed.
- B-10.14 — already fixed: narrator/composer forms retained; S3 separates different narrations before grouping.
- B-10.15 — already fixed: natural audiobook ordering and tag/filename part numbering retained.
- B-10.16 — already fixed: bounded tolerant chapters remain in own reader; no chapter parser changed.
- B-10.17 — fixed integration defects as S3: release/year/edition distinctions, compatible partial evidence and per-copy retagging.
- B-10.18 — already implemented: bounded own APEv2/ID3 gain evidence retained.
- B-10.19 — already fixed common cover naming/parent/type behavior; WEBP decoding and over-bound images remain outside this review/fix scope.
- B-10.29 — already implemented broader bounded audio intake formats; decoder/device qualification is outside this review.
## Phase 1 findings (before production edits)

### S1 — Ambiguous monitor reports bypass the empty-root guard — high
Location: `server/internal/ingestion/monitor.go:151–155,184`; `server/internal/ingestion/targeted.go:296`; `server/internal/catalog/missing.go:52`.
Periodic fallback and watcher overflows enqueue `.` through ReportPaths; startTargetedTx sets reported=1 even though no exact removed-file evidence exists. SourceLooksUnmounted then suppresses the empty-root safeguard for that run. A network/union mount returning an empty readable view with the same root identity can mark every file missing and allow immediate title cleanup. Reproduction: TestDirectoryHintDoesNotAuthorizeEmptyRoot reports the root after its prior content vanishes. Smallest fix: retain empty-root protection for ambiguous directory reports/fallbacks; exact verified file removals may still publish immediately. Native watcher events should retain named file paths so real last-file removals remain possible.

### S2 — Immediate removal performs unbounded filesystem calls in the server — high
Location: `server/internal/ingestion/reported_missing.go:71–117`.
removedPathEvidence directly opens/stats/reads OS paths from the request/monitor thread, without the supervised helper or Client.Guard. A blocked network filesystem can hang the request and the single monitor indefinitely; context cancellation cannot interrupt these calls. There can be 200 paths across 4096 roots. TestReportedRemovalRespectsStorageGuard reproduces bypass of the configured storage guard. Smallest fix: perform retained-root absence proof inside the existing killable storage helper with closed results, a bounded total request budget and the same storage guard; inconclusive evidence merely queues a scan.

### S3 — Track identity merges distinct releases before album grouping — high
Location: `server/internal/catalog/audio_repository.go:239–250,323–336,453–497`; `server/internal/catalog/audio_grouping.go:17–21`.
audioTagKey omits release ID, year and edition. Two identically named/numbered songs on different releases become one title before the album grouping code sees the distinguishing tags. Its second grouping reparents the combined title and can empty the first album; personal state and versions are conflated. The fallback album key also omits edition, so rejecting an incompatible candidate still resolves that same album through EnsureEntityTx. TestAudioTitlesDoNotMergeDifferentReleases covers different MusicBrainz release IDs, years and editions. Smallest fix: scope semantic track identity to the release evidence, and distinguish known fallback editions in album identity while preserving compatible partial-ID enrichment.

### S4 — Truncated extra-owner candidates can establish a false sole owner — medium
Location: `server/internal/catalog/extra_owner.go:17–58`.
The 512-row limit counts files/versions, then len(owners)==1 authorizes fallback ownership. Another title beyond that limit is invisible. A trailer explicitly named for that later title can attach to the first title. TestExtraOwnershipDoesNotTrustTruncatedCandidates supplies 512 versions of Alpha and one Beta. Smallest fix: detect overflow and decline ambiguous ownership; never infer uniqueness from truncated evidence.

### S5 — Depth warning does not stop the walk — medium
Location: `server/internal/storage/inventory_ignore.go:31–32`; `server/internal/catalog/inventory.go:149–204`.
Exceeding the name-reader depth bound returns a protected warning from ignore loading, but inventory still lists and enqueues descendants indefinitely until filesystem limits. This also catalogues media the bounded reader cannot identify. TestScanStopsAtNameReaderDepthBound creates a deep subtree plus healthy root media. Smallest fix: reject only over-bound child coverage at inventory publication, emit the existing protected depth reason, and continue healthy files. Apply the gate to managed listings too.

Failing-test outputs and final dispositions will be added after the initial run.


## S6 — Unicode-equivalent music tags split artists and albums — medium (recorded before its fix)
Location: `server/internal/catalog/audio_repository.go:135,252–259`; `server/internal/catalog/audio_grouping.go:21`.
The shared naming reader normalizes NFC, but music's localKey only lowercases and collapses spaces. New tag scans with `Beyoncé` and the decomposed equivalent create distinct artist/album identities. TestUnicodeEquivalentAudioTagsShareArtistAndAlbum fails on the production base: `artists=2 albums=2`. This is the audio remainder of B-05.32. Smallest fix: NFC-normalize audio local identity keys and compared artist/album display values while preserving punctuation and actual accents.


## S7 — A stale full-scan page can revive a reported removed file — high (before its fix)
Location: `server/internal/catalog/inventory.go:252–405`; `server/internal/ingestion/reported_missing.go:applyReportedMissingTx`.
A full scan reads a page, then an owner/watcher reports a deletion and publishes missing immediately, then the older page commits. observeInventoryTx unconditionally restores availability. Later verification protects the changed directory but does not undo already-published availability; the queued limited scan may wait behind a long full scan. Smallest fix: record exact removed-path coverage against the currently active run in the same transaction as missing publication; that run must not publish an older observation for that path. A subsequent limited run may restore a reappearing file. No broad lock or filesystem call inside the writer.


## S8 — Unreadable directory identity aborts its parent's page — high (before its fix)
Location: `server/internal/storage/inventory_page.go:242`; `server/internal/storage/directory_filesystem_unix.go:19`; `server/internal/catalog/inventory.go:183`.
The filesystem-ID strengthening opens every child directory. An unreadable child makes snapshot return an empty identity; CommitInventoryPage rejects the entire parent page, so protection expands to the root and readable siblings are not imported/reconciled. Existing TestScanLoopProtectsUnreadableSubtreeButContinues fails in this sandbox under the normal non-root user: `healthy subtree did not reconcile`. Smallest fix: isolate a child whose identity cannot be observed as one protected entry_unreadable diagnostic before page publication. Do not relax filesystem identity checks.

## S9 — Retiring a sidecar selection can scan the entire inventory — medium (before its fix)
Location: `server/internal/catalog/sidecar_sources.go:62`.
The available-other-copy query lets SQLite choose inventory_objects as the outer relation. TestProductionStatementsDoNotScanTheCatalogue reproduces `SCAN o USING INDEX inventory_objects_path` with large-catalogue statistics. It runs inside the writer for each removed artwork carrier, violating scan responsiveness at scale. Smallest fix: lead from the indexed artwork owner carriers, then explicitly seek inventory_objects through its existing asset index. No new index/schema or whole-library operation.


# API changes

No public route, request/response shape, removed route, key scope or client parser changed. Existing owner-only scan/settings/change-feed routes remain typed and registered. The existing scan-issues response can now include reason `reported_file_removed` with messageKey `scan.issue.reported_file_removed`; its plain sentence is in packages/i18n. No enum/wire contract changed, so apigen is not required.

Internal helper: new closed `inventory-absence` operation returns absent/errorCode through the supervised protocol. Native watcher events retain a named relative path where the OS supplies it; the existing ReportPaths mapping still narrows to the known containing folder.

# Schema changes

None. No baseline, migration, schema-sync or compatibility change. Audio semantic/album/edition keys use unambiguous bounded JSON tuples internally; no support for pre-release databases created by older key logic is added.

# Owner or viewer settings added or wired

None added. Existing monitorChanges, backgroundSpeed, per-library provider/local-only policy, symlink policy and missing-title cleanup rules remain. Their existing screens need no new control for these fixes.

# Needed from others

- Native packaging: carry the added shared catalogue string through the normal shared-package sync. Build/qualify macOS FSEvents with cgo/frameworks. Non-cgo kqueue supplies directory hints, so ambiguous last-file removals remain protected until exact path evidence is available; no named-file capability is invented.
- Platform/runtime qualification below: Windows ACL/junction/watcher behavior, macOS framework runtime, representative NFS/SMB/FUSE/remounts, real provider responses and throughput.
- Documents: describe the named-file versus ambiguous-directory removal distinction and release-scoped audio identity. Earlier report claims of universal unreadable-subfolder isolation are corrected by S8. No documentation outside the two authorized workspaces was edited.

# Test notes for other environments

Evidence and commands are in `_program/scratch/review-scan/`:

- `base-reproductions.log`: original failing release/version/narration/Unicode/extra/root/guard tests using a Go overlay of HEAD; S5 passes unchanged.
- `base-followup-reproductions.log` and initial `contract-query-checks.log`: stale-page, hidden-junk and unreadable-child failures; the original physical-source query guard records the full-inventory scan. The guard scans physical source files, so Go overlays do not change its source-parser input.
- `final-verified-tests.log`: all 63 selected top-level tests across ingestion, catalog, storage, naming, metadata and persistence pass. The 657-case corpus is included. No tests were skipped in this Ubuntu run.
- `contract-query-checks.log`: all three HTTP contract/router selections pass; its earlier persistence failure is fixed and passes in final-verified-tests.log.
- `final-audio-tests.log`, `coverage-followup-tests.log`, `followup-fix-tests.log`: narrower change-specific passes. Earlier failing intermediate logs remain as evidence.
- `GOFLAGS=-p=2 go -C server build ./...`; touched-package `go vet`; `GOOS=windows GOARCH=amd64 CGO_ENABLED=0` and `GOOS=darwin GOARCH=arm64 CGO_ENABLED=0` builds of storage/ingestion/catalog pass.
- `npx --no-install tsc --noEmit -p packages/client-core/tsconfig.json` and `git diff --check` pass. No schema/routes were changed, so generators were not needed.

Cold acceptance:

1. Disposable source on local and representative network/removable storage. Import files, then make the admitted root unreadable, disconnect its volume, expose an empty view, or leave only hidden configuration. Trigger ordinary, fallback, overflow and folder-only reports. Pass: old inventory/state stays, source needs attention, no missing-title cleanup can consume false absence. A confirmed named removal in a readable unchanged root must still become missing immediately.
2. Run a full scan; pause after a listing page is read, delete a listed file and report its exact path, then commit/replay the old page. Pass: it remains missing. Return the file and finish the next limited scan; it must become available on the retained title.
3. Linux/macOS POSIX permissions or Windows equivalent ACLs: deny one child subtree, delete one healthy sibling and add another. Pass: the old unreadable media stays available in inventory, healthy additions/removals reconcile, owner diagnostics name the child. Repeat with every old title in the unreadable subtree: complete with warnings, not a false empty-root/offline diagnosis.
4. Qualify native add/write/delete/overflow/root-rename watches on NTFS/SMB and APFS/SMB. Build macOS with cgo and Apple frameworks. Pass: known file paths reach ReportPaths, last-file deletion is immediate when verifiable, escape/root-change proofs fail closed, cancellation retires helpers. Cross-compilation here is not runtime acceptance.
5. Tagged album copies: same known release across folders/artists/years, differing release IDs, differing fallback years/editions, partial release IDs, literal separators, and NFC/NFD artist/album values. Pass: true copies share a release; contradictory releases/narrations remain separate. Retag one version to another release: its former version and watched state stay on the old title; compatible enrichment must not split the versions.
6. Large libraries while browsing/playback runs: exercise watcher storms, blocked absence helpers, repeated history pruning and disappearing artwork carriers. Pass: the two-second proof budget is shared; work/checkpoints/candidate bounds hold; artwork-carrier lookups seek by asset. Measure real provider/storage timing separately; no throughput claim is made here.

# Not finished

No confirmed finding remains unfixed. Native Windows/macOS runtime, macOS cgo/framework compilation, real network-mount/remount behavior, live-provider behavior and sustained large-load acceptance are unverified. Automatic extra ownership over the retained 512-candidate bound deliberately remains unresolved. Broader client/playback/lyrics/queue gaps from earlier lanes are outside this intake review.

Snapshots 001 and 002 record compiled milestones. Final cumulative snapshot is `_program/snapshots/review-scan/003`, taken after final validation. This records the completed lane before the subsequently requested base merge below.

# Fixes

Initial reproduction run (five selected repository tests): S1 failed with a complete scan and `missing=1`; S2 failed with removal evidence despite denied storage operation; S3 failed in all three cases with `songs=1 albums=1`; S4 failed with `Beta trailer attached to "Alpha"`. S5 passed.

## S5 — dismissed with evidence
The initial trace missed `naming.IgnorePath`: catalogue publication already stops over-depth children for local and managed pages. Local ignore loading also protects the boundary folder. TestScanStopsAtNameReaderDepthBound passes on the unchanged production base (129 folders plus healthy root media). No fix is required; retain this defensive control as review coverage. The earlier S5 description is historical and disproved.

## S1 and S2 — fixed, initial verification passes
Ambiguous directory reports no longer disable the empty-root guard. Native Linux, Windows and macOS/FSEvents events retain named paths; exact verified missing files still publish before the limited scan, so genuine last-file removal remains immediate. Overflow and non-cgo macOS directory hints remain conservative. Immediate absence proof now runs through the supervised storage helper, respects Client.Guard, verifies the acquired root plus contained readable parent and absent leaf, and shares a two-second evidence budget; inconclusive evidence queues the normal scan. Existing authentication and writer fences are unchanged.
Passing regressions: original S1/S2 tests; real Linux monitor starts a limited folder scan; explicit last-file removal; helper readable/present/changed-root/missing-parent/path-escape controls; outside-parent symlink control; native add/in-place-write/removal events and helper cancellation. Native Windows/macOS runtime acceptance remains required.

S3 follow-up reproduction, before the attachment correction: TestRetaggedAudioVersionDoesNotMoveOtherVersions starts with two copies of one tagged track, changes one copy's year, and fails with `songs=1 albums=1`. Adding a stronger key alone is insufficient: IdentifyTrackTx reidentifies the entire shared title. The fix must detach only the newly contradictory file, preserve the old title's remaining file/state, and retire stale automatic keys when a single-file title is corrected.

S1 additional trace, recorded before the follow-up fix: the empty-root guard also treats any `ignored_name` issue (including unrelated `.keep`/`.plexignore` configuration) as proof that prior media was intentionally excluded. TestHiddenJunkDoesNotAuthorizeEmptyRoot exercises a plain scan with hidden configuration remaining after its media vanishes. Only an ignore issue that actually covers previously available media may suppress the guard; unrelated NAS/hidden junk is not such evidence.

## S3 — fixed
Music semantic title keys now distinguish release ID, fallback album year and known editions; explicit release ID remains authoritative across folders/years. Audiobook part keys distinguish narration, edition and book identifiers. Missing disc is disc one. A changed copy with contradictory evidence detaches from its shared title; compatible incomplete evidence can enrich it. Single-file corrections release stale automatic title keys. Album grouping includes known fallback editions, upgrades compatible partial release evidence and keeps its identity ledger synchronized, preserving manual pins and state-bearing empty containers.
Tests: different release IDs/years/editions fail on HEAD and pass after; shared-version retagging preserves the old version and watched state; distinct narrations fail on HEAD and pass after; partial release enrichment preserves versions and removes the stale weak album alias; existing replacement/restoration/pinned and empty-album tests pass. The older Wave2 test expected the same release ID in a different folder to make a third album; that contradicts Justin's programme decision and the music lane's delivered contract. Its assertion now requires two distinct releases across folders.

## S4 — fixed
Extra ownership reads at most 513 candidates, detects overflow and declines to infer uniqueness from the bounded prefix. Candidate reads use an indexed half-open path range and avoid the old DISTINCT/ORDER materialization. Names containing SQL wildcard characters stay literal. The original 512-version Alpha/Beta reproduction passes; existing ordinary extra detail/playability passes. Bound decision: if more than 512 candidate file links exist under the owning folder, automatic ownership remains unresolved instead of picking from incomplete evidence.

## S6 — fixed
Audio local keys and compared album/artist display values normalize NFC. Unicode-equivalent names stay one artist/album; distinct punctuation and accents are retained. The original failing two-artist/two-album test passes with one of each.

## S7 — fixed
The reproduction fails before with `older page revived deleted file: missing=0`. Exact reported removal now records a protected path in the active run atomically with missing publication. That run cannot revive the path from a stale/replayed listing; the following limited run can restore a returned file. The owner diagnostics use a catalogue string explaining the removal. TestOlderInventoryPageCannotReviveReportedRemoval passes, including the subsequent reappearance control.

## S1 follow-up — fixed
Hidden/configuration junk is no longer enough to bypass the empty-root guard. Only an exclusion covering previously available media counts as intentional removal. TestHiddenJunkDoesNotAuthorizeEmptyRoot fails before the follow-up and passes after; an explicit rule excluding all prior media still reconciles normally.

## Snapshot evidence
Snapshot 001 was taken after Linux compilation and touched-package vet: bounded removal helper, named native events and empty-root protection. Final cumulative snapshots will include the later audio/extra/race corrections. No commit, push or deployment.

S3 additional input case, before its correction: a literal `|` in an edition label breaks the old four-field separator encoding and loses compatible enrichment. TestAlbumEditionWithSeparatorEnrichesUnknownEvidence fails with two albums. Edition evidence is being encoded as a bounded JSON tuple, so owner-supplied punctuation cannot become structure.

## S8 — fixed
A child whose physical identity cannot be obtained is skipped and protected individually with the existing entry_unreadable diagnostic. Root and acquired filesystem checks stay strict. A protected prior-media subtree also counts as evidence that the root is not empty, allowing an all-unreadable-child run to complete with warnings instead of a false offline alarm. Both the originally failing healthy-sibling reconciliation test and TestUnreadableOnlySubtreeFinishesWithProtectedMedia pass; the latter failed before the final coverage adjustment. Hidden-junk, true empty-root and explicit-ignore controls still pass.

## S9 — fixed
The sidecar-retirement existence query now begins with owner artwork carriers and seeks the existing inventory_objects_asset index for each carrier. The production query-plan guard fails on HEAD and passes after the fix under both large-catalogue statistics models. No index, schema or bound was weakened.

## Merge with the base

Merge follow-up complete. No staging, commit or new snapshot; Git still lists the three paths as unmerged until Justin stages the resolved contents.

- `sidecar_sources.go`: keep the base's second CROSS JOIN together with this lane's forced asset index and carrier-first lookup; both constrain the query plan without changing artwork retirement evidence.
- `wave2_album_identity_test.go`: retain the two-release expectation and precise release-ID diagnostic, plus the base's 3/1 track-distribution assertion. A known release remains one album across folders; conflicting release IDs remain separate.
- `inventory_page.go`: protect every entry lacking physical identity; unreadable directories use the base's `directory_unreadable` reason, other entries retain `entry_unreadable`. Healthy siblings continue to reconcile.
- All four scan routes already require `apikit.ScopeLibrary`; the base's identity, network-policy and API-platform replacements remain in place. Ran `GOFLAGS=-p=2 go run ./cmd/apigen` and `python3 scripts/schema-sync.py` successfully; generated files already match the merged source. All requested checks pass: full `GOFLAGS=-p=2 go build ./...` for Linux and Windows/amd64 plus macOS/arm64 (cross-builds use `CGO_ENABLED=0`); vet for catalogue, storage, ingestion, persistence and HTTP API; 49 targeted tests including the affected packages, query-plan guard and registry/router/scope checks; web `npx tsc --noEmit`; `git diff --check`; recursive conflict-marker grep (excluding Git metadata and dependencies). Cross-builds establish compilation, not native runtime acceptance. Evidence: `_program/scratch/review-scan/merge-*.log`.
