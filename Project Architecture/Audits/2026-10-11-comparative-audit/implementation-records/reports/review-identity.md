# Review of identity implementation

## 1. Outcome
Review complete at `a5028946ded2f2ed7793b7fed72552fbf298fe25`, on `ehler-homelab-mini-1`. **Ten findings: 0 blocker, 3 high, 7 medium, 0 low.** Eleven adversarial scratch tests reproduced the ten findings (show identity has two reproductions). Twenty-two selected existing Go tests, five client-core tests and one additional scheduler integration probe passed. Read-only: no source edits, snapshots, commits, pushes or deployments. The affected scope should not be accepted until the findings and the explicit completion gaps below are addressed.

## 2. Decisions I made
- Review the integrated base, not an isolated lane commit. Read the shared brief, identity brief and implementation report completely, relevant original bug entries and second-review entries, and the fix log.
- Use Go overlays to add probes without changing any source file. All overlay code and logs are under `_program/scratch/review-identity/`.
- Context-sync failed because its state-lock directory is read-only. Existing project and verification guidance was read.
- The five-minute missing cleanup claim is supported by `worker.SafetyTick`; no finding against that scheduling claim.

## 3. Bugs — findings

### R1. Different disks can become one physical asset — high
Location: `server/internal/catalog/inventory.go:235–240`; `server/internal/storage/stable_identity.go:59–66`.
The persisted identity drops the device number, then the move search compares inode, size and mtime across every source, allowing a cross-source match when only the basename agrees. Inodes are not globally unique. Two distinct files on different disks with equal inode, size, mtime and basename share an asset token. Observing the second rewrites that shared asset's path, so the first library now plays the second disk's bytes. A restricted library can consequently resolve to content from a different library. Root continuity is also judged only by inode (`SameRoot`); filesystem roots commonly share inode numbers, so different replacement disks can be accepted automatically.
Evidence: `TestReviewDifferentFilesystemsDoNotShareAsset` supplies `/a/Movie.mkv` (`dev:10:ino:123`) and `/b/Movie.mkv` (`dev:11:ino:123`) under different roots. It fails: `distinct filesystem files became 1 assets; surviving path=/b/Movie.mkv`.
Smallest sound fix: namespace object identity by a reliably identified filesystem/source; never infer a cross-source move from inode and basename. Auto-adopt changed root evidence only with proven backing-filesystem continuity or a bounded verified sample, otherwise require the existing replacement acceptance.

### R2. Parent symlinks let deletion escape the admitted root — high
Location: `server/internal/administration/deletion.go:279–301,368–379`.
`underAny` is lexical and `Lstat` only protects the final path component. Replacing a parent directory (including the root) with a symlink to another location causes both the stat and `os.Remove` to follow that parent. The server permanently deletes files outside the admitted root while reporting success.
Evidence: `TestReviewDeleteRejectsParentSymlinkEscape` moves the fixture's media directory outside its configured root and symlinks the root to it. Delete removes both outside files: `Removed:1 FilesDeleted:2 BytesDeleted:22`.
Smallest sound fix: resolve/open and verify the root and descendants through a bounded, symlink-safe filesystem operation; bind deletion to the admitted root and verified file identity. A preflight `EvalSymlinks` alone leaves a race between verification and unlink.

### R3. Delete re-queries and removes titles added after its preview — high
Location: `server/internal/administration/deletion.go:482–497`; `server/internal/catalog/delete_title.go:55–68`.
The initial snapshot checks the preview revision, but after filesystem deletion each write batch resolves the target's current members and deletes all of them without rechecking the captured members/assets. A scan or owner attachment between those phases can add an unpreviewed version or episode. Its catalogue records and personal associations are removed even though its disk file was never among the files deleted. The administration mutex does not serialize scanners/catalogue mutations.
Evidence: `TestReviewDeleteDoesNotRemoveNewUnpreviewedVersion` injects a new linked version at the second authorization boundary (the first write after file deletion), simulating an intervening scan. Delete returns success; the new file remains on disk but its asset is purged.
Smallest sound fix: persist/bind the bounded approved title and file set, delete only those identities, and recheck per-title membership/revisions before each batch. Preserve new members/files and reconcile partial filesystem results durably.

### R4. Different nested show folders merge by basename — medium
Location: `server/internal/catalog/attach.go:144–156,183–193`.
The show identity includes source ID and the nearest folder's normalized name, but omits that folder's relative path. `/tv/UK/The Office/` and `/tv/US/The Office/` are distinct folders in one source and collapse into one show; same-number episodes become versions of one another. This is still the wrong merge described by B-05.27.
Evidence: `TestReviewNestedShowFoldersRemainSeparate` imports S01E01 from both paths and fails: `two distinct folders became 1 shows`.
A second error is the `folderNamesShow` gate: a small filename spelling difference rejects the folder identity and falls back to title/year. `TestReviewSameShowFolderDoesNotSplitOnSpelling` imports `Severance S01E01` and `Severence S01E02` in the same folder and fails: `same folder became 2 shows because one filename had a minor misspelling`.
Smallest sound fix: use the complete relative show-directory locator within its source as the unmatched identity, without requiring every episode filename to spell the folder title identically. Preserve distinct filesystem paths; only a provider identity or owner action should merge separate folders.

### R5. A provider claim is looked up but never recorded — medium
Location: `server/internal/compactcatalog/identity.go:233–247,289–299`.
`ResolveTitleTx` reads `TitleClaim.Provider` to find an already matched entity, but records only parsed keys and the locator. The first file carrying a provider hint does not establish that identity. A second file with the same provider hint and a different parsed name becomes another title; retention cannot remember the supplied provider identity either.
Evidence: `TestReviewProviderClaimsFindPriorUnmatchedFile` attaches two names with `tmdb/movie/348`; it fails: `same provider claim became two titles: 1 2`.
Smallest sound fix: persist validated provider claims through the canonical provider-identity write path, reconcile conflicts, and expose the identity to later attachment and retention. Also wire the naming/local-evidence callers that the implementation report already identifies as pending.

### R6. Retained audio state is not restored when analysis supplies tags — medium
Location: `server/internal/catalog/attach.go:121–134`; caller `server/internal/catalog/audio_repository.go:322–330`.
Normal inventory creates a path-identified track before tags are read. State restoration occurs only at initial creation. Later `IdentifyTrackTx` adds the semantic tag key but returns without restoring retained state when no live holder exists. A removed watched track re-added under another filename remains unwatched even when its tags identify it exactly; its saved state remains stranded.
Evidence: `TestReviewAudioRetainedStateRestoresAfterTags` removes a tagged watched track, creates its replacement without tags, then identifies it with the same tag key. It fails: `restored watched rows=0; retained rows=1`.
Smallest sound fix: restore retained state after newly establishing tag/provider identity, after kind facts needed for identity are available. Add an integration test through the two-pass `commitAudio` flow and a MusicBrainz identity publication hook.

### R7. Offline protection is bypassed by missing-title removal — medium
Location: `server/internal/catalog/missing.go:321–323,368–392`.
The missing-file query excludes offline/root-changed sources, but the title query only excludes active scans. A previously missing title on an offline source is selected, and `RemoveTitleTx`/`forgetAssetTx` deletes its inventory and assets anyway. This contradicts this function's stated guarantee that offline-source absences are left alone and prevents an unverified recovered copy from reattaching to its retained title.
Evidence: `TestReviewOfflineMissingTitleIsNotEmptied` marks a title missing, marks its source offline, then empties. It fails: `offline title deleted: {Titles:1 Files:0 More:false}; file objects=0`.
Smallest sound fix: apply source-health/identity guards to title removal as well, including every contributing source; recheck after a healthy authoritative inventory before removing a title.

### R8. A nonregular replacement is silently treated as an absent file — medium
Location: `server/internal/administration/deletion.go:279–283,463–465`.
A directory or final-component symlink at a catalogued path makes `Present=false`. Delete skips `removeMediaFile`, whose `file_changed` check would have rejected it, then removes the title and reports success. The report promises a changed file keeps its title. Stat errors are conflated with absence too.
Evidence: `TestReviewDeleteReportsNonRegularReplacement` replaces one media file with a directory before preview/delete. It fails: `Removed:1 FilesDeleted:1 Code:""; titles remaining=0`.
Smallest sound fix: distinguish ENOENT from changed type and stat failure; carry that result into deletion and reject/retain the affected title with the existing receipt code.

### R9. Owner deletion and library removal do not enforce the retained-state bound — medium
Location: `server/internal/catalog/retained_state.go:293–318`; callers `server/internal/catalog/delete_title.go:63–68` and `server/internal/catalog/library_removal.go:200–205`.
The only call to `PruneRetainedStateTx` is in `EmptyMissing` when that pass removes titles. Explicit deletion and library removal can accumulate retained state indefinitely; an idle server's empty missing sweeps never prune it. The advertised 100,000-per-profile bound is not applied to those removal paths.
Evidence: `TestReviewEmptyLibraryRemovalRetainsUnboundedState` seeds exactly 100,000 retained rows, removes a library containing one additional watched title through successive removal batches until completion, and observes 100,001 rows; it fails: `library removal completed with 100001 retained rows, limit 100000`. Call-site search finds only `missing.go:396`.
Smallest sound fix: apply bounded pruning from every retention-producing removal path or a dedicated scheduled retention worker, without requiring missing-title removals.

### R10. A merged title loses the absorbed title's provider identities — medium
Location: `server/internal/catalog/title_merge.go:68–97`; cascade in `server/internal/persistence/migrations/0001_baseline.sql:803`.
Merge moves local ledger keys and personal state but does not move `catalog_external_ids` before deleting the absorbed entity. Its provider identities cascade away. If only that entity carried IMDb/TMDB/TVDB identity, the surviving title cannot later be found or remembered under it. Retention after removal loses the provider key, so a re-add in another library cannot restore the state that was just merged.
Evidence: `TestReviewMergePreservesProviderIdentityOfRemovedTitle` puts IMDb `tt0078748` on the absorbed film, merges it into another film and removes the survivor. It fails: `merged-away provider identity lost: retained imdb identities=0`. This is separate from R5: a correctly persisted provider identity is destroyed by merge.
Smallest sound fix: transfer compatible provider identities before deleting the absorbed title. Reject or resolve conflicting identities explicitly, respecting provider uniqueness and kind; keep all valid identities available to future attachment and retention.

### Original bug dispositions within this lane

| Bug | Independent review result |
|---|---|
| B-05.8 | Partly fixed: normal same-title/year films become versions and retain state; provider claims fail R5 and merged provider identities fail R10. Audio continuity has R6. |
| B-05.9 | Film/episode reinterpretation is wired; the focused rename test passes. Show identity still splits on spelling (R4). |
| B-05.18 | Device renumbering test passes, but the replacement identity logic is not sound across filesystems (R1). |
| B-05.24 | Immediate missing marks, reattachment and rule-based emptying work in focused tests; offline emptying violates its guard (R7). |
| B-05.25 | Old state-folder copy mechanism removed; disk unlink now occurs. Physical deletion has R2, R3 and R8. |
| B-05.27 | Not fully fixed: both nested-folder merge and same-folder spelling split reproduced (R4). |
| B-07.3 | Live locator reuse test passes. No recurrence of the specific vacated-path ledger error found; cross-filesystem asset reuse is a different defect (R1). |
| B-07.11 | Raw item delete handler removed. Dead route-scope entries remain but do not expose a handler. Semantic deletion has R2/R3/R8. |
| B-07.15 | Entity-removal paths prune identity ledger entries. Newly introduced retained-state retention is not bounded on all paths (R9). People/term/label pruning belongs to another lane. |
| B-09.13 | Catalogue trash, state-folder copies, retention settings and restore/empty handlers removed; focused disk-deletion test passes. |
| B-09.18 | Backup destination is outside the identity deliverables; no independent backup review performed. The original bug is not resolved by deleting the trash mechanism. |
| B-12.18 | Same-title film upgrades retain watched, rating and favourite state in passing tests. Re-add/identity continuity remains incomplete under R5/R6/R10. |
| B-12.19 | Explicit removal repairs the Next Up pointer to a remaining episode; focused show-deletion test passes. |
| B-18.5 | Remove-library API hides the library and enqueues removal; existing HTTP test and new scheduler probe pass. R9 affects bounded state retention. |
| B-18.13 | Bulk destructive job removed; web selection invokes preview/typed confirmation and disables deletion above 200 explicit loaded IDs. No unconfirmed bulk-delete route found. |

The other bugs in the broad audit documents concern other lanes and are not claimed reviewed or fixed here.

## 4. API changes
No changes made. New missing-title list/empty, title merge and library removal routes have typed registry contracts, explicit owner access and required key scopes. Their mutation callbacks recheck authorization inside transactions. The selected HTTP test verifies unauthenticated denial, wrong typed name, stale revision, hiding and enqueuing; member-role denial was checked by tracing the shared owner route enforcement, not a new member-session runtime test.

Client-core services/parsers match the new missing-title and deletion field names, merge response and remove-library request. Five single-file client-core tests pass. Raw item deletion and trash handlers are gone. Separate-version action uses the existing bounded, owner-only inventory action route. No additional authentication or authorization finding beyond R1/R2's filesystem identity/boundary issues was established.

## 5. Schema changes
No changes made. Reviewed the identity ledger, missing-title/state tables, indexes and deletion cascades against their callers. Baseline schema initializes successfully in the targeted tests. R9 is missing pruning call-site coverage, not a missing index; R10 is a missing transfer before the provider-ID cascade.

## 6. Owner or viewer settings added or wired
No changes made. Checked `library.missingTitles` (`immediately`, `after_days`, `never`, default `after_days`) and `library.missingTitlesDays` (1–3650, default 30). Rule/default tests pass; periodic cleanup is backed by the five-minute worker safety tick. Immediate cleanup can be deferred until a later worker pass because the loop throttles sweeps to thirty seconds, consistent with the programme's freshness window. The web missing-title rule/list exists; the other screens in the implementation report remain external follow-up work.

## 7. Needed from others
- Identity owner: fix R1–R10 and retain the scratch cases as regression coverage in normal tests.
- Naming/integration owner: supply edition, split-film part and provider hints to the attachment API. The current production `commitMovieItem` still supplies only title/year. The rewritten name reader does not by itself finish this wiring.
- Playback owner: generic height/size ordering works, but “best playable” selection is not demonstrated. `playbackv1.choice` takes `Versions[0]` and `playback.v1AssetRow` selects the tallest/largest available asset; neither tries another version when its plan is incompatible. Also verify multiple copies/editions of split titles: `versionsOf` has no persistent version grouping identity. These are explicit completion gaps, not evidence of device acceptance.
- Provider/music owner: MusicBrainz identity publication and retained-state restoration are not wired for tracks; album/artist/book provider merging is omitted in the lane report.
- Downloads/social/playback owners: reconcile references to absorbed item IDs in their own tables. The implementation report explicitly leaves these out. Updating legacy `playback_sessions` alone does not establish v1 session/queue/download continuity.
- Client owners: version picker, missing badge/time, merge/separate and remove-library controls, descendant counts and failed-delete receipt reasons; native consumers/screens were not changed in this lane. The plain deletion codes are parsed, but the web dialog does not render their specific reason.
- Product documentation owners: correct the outdated two-scan/grace and state-folder trash documents named in `identity.md`.

## 8. Test notes for other environments

### Commands and evidence on Ubuntu
Run from this checkout's `server/`, with the existing programme Go cache and `GOFLAGS=-p=2`:

```sh
GOFLAGS=-p=2 go test -overlay=/home/justin/projects/Portico/.worktrees/audit-implementation/_program/scratch/review-identity/overlay.json ./internal/catalog ./internal/administration -run '^TestReview' -count=1 -v
```

The overlay maps the existing title-identity and deletion test files to scratch copies containing appended probes. It changes no source file. Eleven negative tests reproduce R1–R10; the real-scheduler library-removal probe passes. Runs were split across `probes.log`, `followup-probes.log`, `retention-probe.log` and `scheduler-probe.log`. The retention follow-up starts at the bound and verifies complete removal, rather than merely seeding an already oversized table. R3 uses the second authorization callback to deterministically inject an attachment at the write boundary; it proves the mutation ordering defect, not a timed concurrent goroutine race.

A targeted non-overlay run across catalog, administration, storage, ingestion, httpapi and playbackv1 passed **22 existing tests**. The exact anchored `-run` selection and outcomes are in `existing-tests.log`; this was not a whole-package or repository test run.

From repository root:

```sh
node --experimental-strip-types --test packages/client-core/test/title-identity.test.ts
```

Five tests pass; see `client-tests.log`. Working tree remained clean (`git status --porcelain=v1`). No builds, full suites, live media playback, production filesystem mutation or deployment were performed.

### Follow-up acceptance
- Linux/network mounts: use disposable independent filesystems with identical inode/basename/size/mtime observations, then reverse mount order. Pass: same proven backing root retains its identity; different roots/files never share asset tokens or leak their paths. Also test an unmounted empty root and an unrelated mounted root with coincident root inode.
- Windows/macOS: exercise deletion with changed parents, final symlinks/junctions, read-only paths and partial filesystem errors. Pass: no outside-root file is deleted, changed/failed files retain titles, receipt reasons are explicit.
- All platforms: hold deletion after the approved snapshot, attach a new version/episode, then resume. Pass: only the approved member/file set is removed, new content stays catalogued, partial outcomes survive cancellation/retry.
- Music/audiobooks: real two-pass tag scans and MusicBrainz matching after removal/re-add under a different path. Pass: progress, watched and saved state return once identity is proven.
- Scale: remove libraries and merge very large shows while browsing and playing another library. The small scheduler test passes; sustained responsiveness and large-show merge latency are unmeasured. Show merging still loads/merges all children in one write transaction (`title_merge.go:238–371`), as the lane report discloses, and needs bounded/checkpointed execution before the programme's scale claim can be accepted.

## 9. Not finished
This independent review is complete; source fixes are deliberately not made in this read-only lane. Hardware/OS/network-mount acceptance, actual codecs and broad load testing remain unverified. The explicit integration/client omissions above remain implementation work.

The implementation report also discloses that per-play personal-history rows and playlist positions are not restored after final removal, some removed-library derived rows stay hidden, untagged audio replacements are identified only by path, and native screens are absent. These are completion limitations, not newly discovered hidden fixes. The permanent owner `play_history` is distinct from cascading profile `personal_history`; do not claim that every history row disappears.

### Verdict per identity brief scope item

| Scope item | Verdict | Evidence and remaining work |
|---|---|---|
| 1. Titles separate from files and personal state | **Not sound** | Normal film/episode version/state cases pass, but R1 can attach wrong physical bytes, R4 wrongly merges/splits shows, R5 ignores supplied provider claims, and R10 destroys proven identities on merge. Audio/tag and provider integration omissions remain. |
| 2. Versions and best playable selection | **Not sound** | Detail/options carry resolution, codec, range, tracks, size and edition; ordinary whole-file versions sort and can be named. Production edition/part wiring and best-playable selection are unfinished; multipart grouping lacks explicit version identity. |
| 3. Missing files/titles, emptying and offline protection | **Sound with the listed fixes** | Immediate missing/reattachment/default/rules pass. Empty-root stop and publication fences were traced; offline title removal needs R7. Real unmount/remount acceptance also depends on R1. |
| 4. Delete from disk with preview/confirmation | **Not sound** | Ordinary disk delete, permissions and confirmation tests pass, and state-folder trash is gone. R2 escapes the admitted root, R3 removes unpreviewed attachments and R8 silently drops changed-type targets. |
| 5. State outlives removal under bounded identity | **Not sound** | Matched-film re-add restores watched/progress/rating/list/play count in passing tests. R5/R6/R10 lose identity or restoration, and R9 leaves owner deletion/library removal unbounded. MusicBrainz publication is pending. |
| 6. Remove a library | **Sound with the listed fixes** | Route hides immediately; both queueing and actual scheduler completion with retained watched state pass. Apply R9 and the state-identity fixes; derived-library leftovers and scale acceptance remain explicitly limited. |
| 7. Robust file identity across device changes | **Not sound** | Simulated device/inode renumbering passes, but R1 trades the old false replacement problem for cross-filesystem aliasing and unsafe root continuity. |

## Fixes

Fix work started on `audit-implementation/review-identity`. Regression probes have been promoted into repository tests. High findings R1–R3 first; no commits are permitted. Context-sync still cannot write its lock; existing guidance read. Security boundary and compatibility checks are performed as separate passes by this lane because the programme forbids extra agents. User-requested report and snapshot destinations override the security skill artifact defaults.

### R1 — fixed
File move candidates now require the same source and root incarnation; basename/inode coincidence across disks cannot reuse an asset. Linux/macOS directory observations include a nonzero filesystem ID obtained from an opened, identity-checked directory. A changed device number is auto-adopted only with matching filesystem ID and root inode; unknown evidence requires explicit replacement acceptance. No schema or routes changed.
Tests: `TestDifferentFilesystemsDoNotShareAsset` and `TestRootIdentityRejectsDifferentFilesystems` failed before the change and pass afterward. Existing same-source locator, stable-object and renumbered device/inode controls pass. `GOFLAGS=-p=2 go vet ./internal/catalog ./internal/storage` passes. Independent bypass pass checked different sources, different incarnations, nested filesystems, zero filesystem IDs and direct SameRoot callers. Real network-filesystem ID stability and macOS execution remain acceptance work.

### R2 and R8 — fixed
Physical deletion now verifies and opens the canonical admitted root, checks the opened root against the pre-open directory identity, opens its parent through `os.Root`, and stats/removes the final name through that same pinned parent. Absolute and escaping descendant links cannot redirect the operation. Every requested file is rechecked, including nonregular replacements and paths initially reported absent; only verified absence succeeds without unlinking. Changed types return `file_changed` and retain the title.
The original `TestDeleteRejectsParentSymlinkEscape` and `TestDeleteReportsNonRegularReplacement` fail before and pass after. Additional nested escape and final symlink controls pass; ordinary deletion, changed-file, typed confirmation and keep-files controls pass. Targeted administration vet passes. No route/response changes. Platform caveat: Go's descriptor-backed `os.Root` guarantees cover the supported server OSes; Windows/macOS runtime checks remain outstanding.

### R3 — fixed
The approved target captures bounded public title IDs and a digest of each title's file tokens, paths, sizes/timestamps and associations. Catalogue batches resolve those public IDs and remove only unchanged approved titles. A new version preserves its title; a new episode preserves its container and is never swept into a later batch. Files already removed are marked missing/unavailable in bounded transactions even when their title is preserved, and partial byte totals reflect actual filesystem deletion.
`TestDeleteDoesNotRemoveNewUnpreviewedVersion` fails before and passes after, also checking the old deleted versions are no longer offered. `TestDeleteDoesNotRemoveNewUnpreviewedEpisode` checks the separate container path. Existing delete and show-removal controls pass; targeted catalog/administration vet passes. No schema or routes changed. This keeps the existing request receipt lifecycle; interrupted filesystem work is reconciled by missing inventory rather than resurrecting the removed trash journal.

### R4 — fixed
Show-folder identity now includes the exact complete relative directory path within the source. Folder/name comparison tolerates one bounded spelling edit/transposition for non-short names while preserving genuinely different names in assorted folders. `TestNestedShowFoldersRemainSeparate` and `TestSameShowFolderDoesNotSplitOnSpelling` both fail before and pass after. Existing source-separated/provider-merged show, episode hierarchy and absolute anime controls pass. Catalog vet passes.

### R5 — fixed
Provider claims are normalized, bounded, recorded in `catalog_external_ids` and used to resolve later versions and retained state. Conflicting accepted identities cannot be silently overwritten or merged by a parsed-title fallback; separate provider claims remain separate titles. Production movie attachment now forwards the name reader's closest provider hint per provider (file evidence takes precedence over folder hints). No route changes.
`TestProviderClaimsFindPriorUnmatchedFile` fails before and passes after. Named provider-hint grouping and conflicting-claim controls pass, together with existing film version/replacement/state tests. Catalog and compactcatalog vet pass.

### R10 — fixed
Merge transfers every compatible provider identity before deleting the absorbed title. A provider/kind conflict rejects and rolls back the merge rather than losing either identity. `TestMergePreservesProviderIdentityOfRemovedTitle` fails before and passes after; `TestMergeRejectsConflictingProviderIdentities` verifies both entities/files survive rejection. Existing film/show merge and retained-state controls pass. Catalog/compactcatalog vet pass.

### R6 — fixed
Tag identification restores retained state as soon as a semantic key is established, including the merge path. Accepted MusicBrainz recording publication invokes the same restoration after recording facts are written. Newly supplied provider claims also restore state on an existing title. The real two-pass audio test additionally exposed a restrictive stale `catalog_asset_issues` reference; asset deletion now releases that file-owned evidence. The initial album-pointer diagnosis was disproved by `PRAGMA foreign_key_check`, and the speculative pointer change was removed.
The original `TestAudioRetainedStateRestoresAfterTags` fails before and passes after. `TestAnalyzedAudioTagsRestoreRetainedState` exercises removal and actual inventory/tag passes; `TestMusicBrainzPublicationRestoresRetainedWatchedState` exercises accepted provider publication. New-provider restoration and existing MusicBrainz restart/state controls pass. Catalog, compactcatalog, metadata and administration vet pass.

### R7 — fixed
Missing-title removal now applies the same source-health protection as missing-file removal. It conservatively defers title emptying for the whole library while any source is offline, root-changed or inventory-running, matching the existing library-wide active-scan guard. This also protects titles whose file evidence was pruned in an earlier pass; no origin schema is required. Individual healthy missing-file cleanup can continue.
`TestOfflineMissingTitleIsNotEmptied` fails before and passes after. `TestMissingTitlesWaitForSourceRecovery` covers all three guarded health states and successful emptying after recovery. Existing rule, reattachment and folder-removal controls pass. Catalog vet passes.

### R9 — fixed
All retention-producing paths enforce the 100,000-per-profile bound inside the shared `retainStateTx`, after identities are recorded. New baseline table `retained_title_state_counts` and insert/delete triggers maintain exact profile counts; overflow eviction uses the existing oldest-first profile index and work proportional to newly retained rows, avoiding a full history recount per title. The existing global prune hook reads the maintained counts. `python3 scripts/schema-sync.py` completed successfully and refreshed known schema/column records; pre-release databases must be recreated as required by the programme.
`TestLibraryRemovalBoundsRetainedState` fails before and passes after. `TestExplicitDeletionBoundsRetainedState` verifies owner deletion and oldest-first eviction; `TestRetainedCountsFollowRestorationAndCascade` verifies restored/deleted state updates counts without disturbing another profile. Persistence schema checks and targeted vet pass.

### Additional checks while completing these fixes
The filesystem-evidence change was checked against the descriptor-based observed-root path. Inherited directory handles have an opaque name, so their filesystem ID is read directly from the acquired handle rather than reopening that name. `TestDirectoryIdentityUsesInheritedHandle` and remount controls pass. No authentication, key scope, route shape, client contract or owner setting changed in this fixer pass; apigen is therefore not required. The earlier review sections remain historical evidence; this Fixes section records their resolution.

### Final verification and snapshot
All ten reported findings are fixed; none was dismissed or left open. The security-boundary review additionally reproduced a replacement root containing identical-sized/timestamped files after the first deletion patch. Deletion now compares the acquired directory's identity against the recorded admitted source identity. `TestDeleteRejectsReplacementRootWithMatchingFiles` fails before that follow-up and passes after; `TestDeleteAcceptsUnchangedAdmittedRoot` verifies legitimate deletion. The deletion digest also includes kind, library and parent membership, so reassigned titles cannot pass as the original approved members.

Final checks completed successfully:
- **48 selected tests** across catalog, administration, metadata, storage and ingestion; every original failing reproduction now passes as a repository test. The exact test names/results are in `_program/scratch/review-identity/final-fix-tests.log`.
- Actual observed-storage read/root-rename and replaced-root/symlink-escape tests pass, alongside the acquired/inherited directory-handle identity test.
- `GOFLAGS=-p=2 go vet ./internal/catalog ./internal/compactcatalog ./internal/administration ./internal/metadata ./internal/storage ./internal/persistence` passes.
- `GOFLAGS=-p=2 go build ./...` passes on Linux.
- `GOFLAGS=-p=2 GOOS=windows GOARCH=amd64 go build ./internal/storage ./internal/administration` and `GOFLAGS=-p=2 GOOS=darwin GOARCH=arm64 go build ./internal/storage ./internal/administration` pass. These compile the affected filesystem paths and dependencies; they do not establish OS runtime acceptance.
- Known-schema objects/columns, immutable baseline checks, and production statement preparation/scan guards pass. Statement guards were rerun after the final SQL changes. `git diff --check` passes.

No full test suite, deployment, push or commit was run. The baseline changed; recreate development databases. No route or client response shape changed, so no client parser or apigen update is needed. Runtime verification on Windows/macOS, real remounts and large-load acceptance remains outstanding. The earlier client/playback/integration gaps under “Needed from others” remain outside these ten findings; MusicBrainz restoration and movie provider-hint forwarding are now wired here.

Cumulative snapshots: `001` filesystem/source identity; `002` approved, bounded-root deletion; `003` show/provider identity; `004` retained-state restoration and bounds. **Final cumulative snapshot: `_program/snapshots/review-identity/005`**, taken after all validation. No source changes follow it. The earlier review verdicts describe the reviewed base and are superseded for R1–R10 by this Fixes section.
