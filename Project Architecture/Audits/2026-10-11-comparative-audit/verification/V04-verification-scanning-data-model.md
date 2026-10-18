# V04 — Verification of 05 libraries/scanning/naming and 07 data model/persistence/query

## Summary

All 24 Tier 1 entries and 21 Tier 2 entries were reached. Tier 1: **7 Confirmed, 14 Confirmed, narrower, 3 Unsettled — needs runtime, 0 Refuted**. Tier 2: **5 Confirmed, 16 Confirmed, narrower, 0 Unsettled, 0 Refuted**. The first pass largely identified real source behavior, but often treated conditional behavior, declared limits or missing capabilities as blanket defects. No complete finding was refuted; several examples, scope statements and fix suggestions were corrected. The seven Low entries were copied without rechecking.

The strongest confirmations are the production scan pacing, stale in-place files rejecting original playback, the Windows local-library identity blocker, incremental symlink-alias constraint failure, derivation step-worker starvation and old-path identity reuse. Windows support is explicitly advertised, removing the first pass’s uncertainty about whether that platform matters. The stale-file recovery is not guaranteed within a week: integrity mode is chosen only when another scan is queued, and scheduling defaults off.

Conservative changed-inventory rejection, withholding incompatible rebuilding projections, defensive XML budgets, optional scheduling and missing-file grace are recorded choices. Their residual usability/availability costs remain worth planning, but they should not be filed as accidental safeguards failing. CJK substring search, abbreviation aliases and disc grouping are capability decisions. NAS junk ingestion is reduced to Medium; retained scan bookkeeping is High at sustained scheduled scale. Remount evidence stability and derivation gate latency remain unmeasured, with concrete experiments recorded below.

## Verdict table

Confidence labels summarize the first pass: Reading = Confirmed by reading; Probable/Speculative retain their stated evidence boundary. Original compound severity labels are retained here. **Size:** S = hours, M = days, L = a week or more. Severity on deliberate choices describes residual impact, not a defect designation.

| ID | Claim (short) | First pass (confidence / severity) | Verdict | Class | Your severity | Size | In Backlog? |
|---|---|---|---|---|---|---|---|
| B-05.1 | Scan speed has a hard ceiling of about three steps per second per folder source | Reading / High | Confirmed, narrower | Defect | High | S | No |
| B-05.2 | Any folder changing during a scan fails the whole scan — and then nothing gets analysed | Reading / High | Confirmed, narrower | Deliberate trade-off | High | M | No |
| B-05.3 | A file that changes after it was first seen stays stale — and probably unplayable — for up to a week | Reading / Probable (secondary impact) / High | Confirmed, narrower | Defect | High | M | Yes — stale facts; consequence adds detail |
| B-05.4 | One unreadable sub-folder stops every scan of that source | Reading / High | Confirmed | Defect | High | M | No |
| B-05.6 | Hidden and system files are catalogued as media | Reading / High | Confirmed | Defect | Medium | M | No |
| B-05.7 | A year inside a title is taken as the release year | Reading / Medium-High | Confirmed, narrower | Defect | Medium | S | No |
| B-05.8 | A second file for a movie is a second movie; an upgraded file loses the title's personal state | Reading / High | Confirmed, narrower | Unfinished feature | High | L | Yes — second-file grouping |
| B-05.10 | Flat show folders, and the show folder's year, are only understood inside a Season N folder | Reading / High | Confirmed | Defect | High | M | No |
| B-05.11 | The folder-must-match rule rejects correctly organised shows | Reading / High | Confirmed, narrower | Defect | High | M | No |
| B-05.18 | File identity includes the device number, which is not stable across reboots on many setups | Probable / High | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-05.19 | A directory's timestamp is not always evidence that nothing was added | Probable / Medium-High | Unsettled — needs runtime | Design risk | Medium | M | No |
| B-05.20 | Scan bookkeeping is never pruned | Reading / Medium-High | Confirmed, narrower | Design risk | High | M | No |
| B-05.25 | Deleting media on another volume copies it into the server's state folder | Probable / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-05.28 | On Windows the server cannot add a library | Probable / Critical | Confirmed, narrower | Defect | Critical | M | No |
| B-05.29 | Local metadata files are rejected whole when they exceed a budget | Reading / Probable (secondary impact) / Medium | Confirmed, narrower | Deliberate trade-off | Medium | M | No |
| B-05.32 | Accented names written two ways may not be recognised as equal | Speculative / Low-Medium | Confirmed | Defect | Medium | S | No |
| B-05.33 | With "follow symlinks" on, a folder alias inside the root may make every incremental scan fail on a database constraint | Probable / Medium | Confirmed | Defect | Medium | S | No |
| B-07.1 | One permanently failing entity stops all second-tier derived workers | Reading / High | Confirmed | Defect | High | M | No |
| B-07.2 | Raising a derivation version takes browsing or search offline for the whole rebuild | Reading / High | Confirmed, narrower | Deliberate trade-off | High | L | No |
| B-07.3 | A new file at a moved movie's old path is attached to the moved movie | Probable / Medium | Confirmed | Defect | Medium | M | No |
| B-07.11 | DELETE /v1/items/{id} is a raw entity delete | Reading / Probable (secondary impact) / Low to Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-07.13 | The backup pins one snapshot for the whole copy | Speculative / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-07.16 | Nothing reports a stuck write gate in production | Speculative / Low to Medium | Confirmed, narrower | Design risk | Low | S | No |
| B-07.18 | Derived batches may hold the write gate for long stretches | Speculative / Low to Medium | Unsettled — needs runtime | Design risk | Low | M | No |
| B-05.5 | With "follow symlinks" on, one dangling or out-of-root link stops the scan | Reading / Medium | Confirmed | Deliberate trade-off | Medium | M | No |
| B-05.9 | Renaming a file to fix it does not change how Portico read it | Reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-05.12 | Resolution tags such as 1920x1080 are read as season × episode | Reading / Medium | Confirmed | Defect | Medium | S | No |
| B-05.13 | Other correct names the episode reader refuses or shortens | Reading / Medium | Confirmed, narrower | Defect | Medium | M | No |
| B-05.14 | An extras folder name anywhere in the path swallows everything under it | Reading / Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-05.15 | A ripped Blu-ray or DVD folder becomes dozens of titles | Reading / Medium | Confirmed, narrower | Missing feature mislabelled as a bug | Medium | M | No |
| B-05.16 | Samples and bare trailers become titles | Reading / Medium | Confirmed | Defect | Medium | S | No |
| B-05.17 | Titles are left mangled until (and unless) a provider match replaces them | Reading / Low-Medium | Confirmed, narrower | Defect | Low | S | No |
| B-05.21 | Every scan re-queues every file | Reading / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-05.22 | Looking for sidecars that are not there costs dozens of process starts per file | Reading / Medium | Confirmed, narrower | Defect | Medium | M | Yes — missing-sidecar cache |
| B-05.23 | A new library never rescans by default | Reading / Medium | Confirmed | Deliberate trade-off | Medium | S | No |
| B-05.24 | Deleted files stay "available" for a day and are never removed on their own | Reading / Medium | Confirmed, narrower | Deliberate trade-off | Medium | S | No |
| B-05.26 | One unplaceable file makes a TV library permanently "degraded" | Reading / Low-Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-05.27 | Show identity is "title + year", which both splits and merges wrongly | Reading / Medium | Confirmed, narrower | Design risk | Medium | L | No |
| B-05.30 | A stopped scan does not say where it stopped | Reading / Low-Medium | Confirmed, narrower | Defect | Medium | S | No |
| B-05.31 | Multi-disc albums split when disc folders are not named exactly CD1 / Disc 1 | Reading / Low-Medium | Confirmed, narrower | Missing feature mislabelled as a bug | Low | S | No |
| B-07.4 | Cursors and range revisions go stale on every progress report and every scan commit | Reading / Medium | Confirmed, narrower | Design risk | Medium | M | No |
| B-07.5 | A search can fail with "the library changed" although it read one consistent snapshot | Reading / Medium | Confirmed | Defect | Medium | S | No |
| B-07.6 | Some titles cannot be found by search at all | Reading / Medium | Confirmed, narrower | Missing feature mislabelled as a bug | Medium | M | No |
| B-07.7 | Search finds Chinese, Japanese and Korean titles only from their first character | Reading / Medium | Confirmed, narrower | Missing feature mislabelled as a bug | Medium | L | No |
| B-07.8 | Date filters compare raw text | Reading / Low to Medium | Confirmed, narrower | Defect | Medium | S | No |

## Tier 1 — detailed verification

### B-05.1 Scan speed has a hard ceiling of about three steps per second per folder source

- **Claim:** The production scanner imposes a roughly 300 ms minimum between quanta for each local source, independent of idle hardware capacity.
- **What I traced:** `apps/portico-server/server/cmd/server/main.go:432,702` constructs and runs Service.Run; `apps/portico-server/server/internal/ingestion/service.go:331,362,377,388` starts one quantum per source and requests 300 ms. `apps/portico-server/server/internal/worker/worker.go:108,166` treats commit wakeups as shared and floors them to the requested delay. Local inventory requests 32 entries (`apps/portico-server/server/internal/ingestion/inventory.go:133`); analysis consumes one pending object (`apps/portico-server/server/internal/ingestion/analysis.go:162,169`).
- **Verdict:** **Confirmed, narrower** — The throughput ceiling is real for this path. The helper tests use the unpaced process loop (`apps/portico-server/server/internal/ingestion/service.go:398`). First-import estimates are lower bounds, not measured completion times. Verification still checks skipped directories in batches of eight (`apps/portico-server/server/internal/ingestion/inventory.go:451`); the first-pass one-eighth overhead is reasonable as a lower-bound step count. Managed remote pages need not share the local 32-entry limit, and four sources can progress concurrently. No continuation wake bypasses the floor.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** High: imports and rescans of large local libraries spend most of their time waiting, even with spare resources; exact rates remain unmeasured.
- **Fix direction:** Run bounded time slices or immediate completion-driven continuation, retaining admission, per-source concurrency and foreground priority. Touch ingestion/service.go and worker wake wiring; verify production-loop cadence. (**Size:** S).
- **Related:** B-05.21 amplifies the same cost; Q-24.1 addresses write pressure, not this idle-capacity limiter.

### B-05.2 Any folder changing during a scan fails the whole scan — and then nothing gets analysed

- **Claim:** An observed directory revision change fails the source job before its normal analysis phase.
- **What I traced:** `apps/portico-server/server/internal/storage/inventory_page.go:146,200` rejects changed pages. `apps/portico-server/server/internal/ingestion/inventory.go:166,207` rechecks each completed directory. `apps/portico-server/server/internal/ingestion/service.go:456,472` sends errors to fail rather than analysis. `apps/portico-server/server/internal/catalog/inventory.go:335` creates analysis rows, but analyzeOne is reached only in basic phases.
- **Verdict:** **Confirmed, narrower** — Confirmed for a detected change in a listed directory before verification, not literally any write anywhere: in-place file writes may leave directory revisions unchanged (B-05.3), and already-verified directories can change later. Other sources and independently scheduled provider matching continue. Existing completed analysis remains. Conservative failure and explicit retry are recorded in `apps/portico-server/server/internal/ingestion/INVENTORY_CONTRACT.md:64`; Basic waits for completed inventory in `portico-internal/Product Contracts/libraries-and-scanning/contract.md:97`. Do not label the conservative absence safeguard itself a defect. Its analysis coupling creates a serious usability risk.
- **Class:** Deliberate trade-off. **In Backlog?** No.
- **Severity:** High: repeatedly changing sources may never enrich newly inventoried files while already published catalogue rows remain visible.
- **Fix direction:** Keep failed/partial inventories non-authoritative for missing reconciliation; permit separately fenced analysis of committed observations, or bounded directory retries with explicit warning coverage. Touch ingestion inventory/analysis/service and lifecycle contract. (**Size:** M).
- **Related:** B-05.4 and B-05.5 share terminal failure/analysis coupling; B-05.30 covers the missing diagnosis.

### B-05.3 A file that changes after it was first seen stays stale — and probably unplayable — for up to a week

- **Claim:** An in-place file mutation can be invisible to incremental scans and then fail original-file playback until a full observation updates its facts.
- **What I traced:** `apps/portico-server/server/internal/ingestion/incremental.go:47,69` compares only the directory fingerprint; `apps/portico-server/server/internal/storage/inventory_page.go:46` hashes directory identity, change token, size and mtime. `apps/portico-server/server/internal/ingestion/incremental_test.go:86` explicitly expects stale size after overwriting a file. Production delivery is `apps/portico-server/server/internal/httpapi/media_original.go:38` → `apps/portico-server/server/internal/assets/playback_reader.go:17,42` → `apps/portico-server/server/internal/storage/playback_descriptor_unix.go:128`, which rejects size/mtime mismatch. Converted source opening uses the same facts (`apps/portico-server/server/internal/assets/playback_input.go:62`).
- **Verdict:** **Confirmed, narrower** — The original cited strongInputPreparer is unwired: searches found its constructor and PrepareInput only in its implementation/tests. The active path independently confirms rejection for original reads. Existing prepared/private presentations or another version can avoid it, so not every playback mode is necessarily unavailable. Seven days is a mode-selection threshold on the next queued scan (`apps/portico-server/server/internal/ingestion/incremental.go:24`), not an autonomous weekly timer; interval zero or failed scans can leave it stale indefinitely.
- **Class:** Defect. **In Backlog?** Yes — stale facts; consequence adds detail.
- **Severity:** High: ordinary in-place copies/tag edits can leave a visible title unable to serve its original source; no source-refresh retry was found in that delivery path.
- **Fix direction:** Track unsettled files and revisit their directories; introduce bounded per-object reobservation on mismatch and invalidate revision-bound analysis before retry. Keep playback validation strict. Touch ingestion/incremental, inventory facts and assets/playback delivery. (**Size:** M).
- **Related:** Q-24.3 agrees. B-12.18 must distinguish this replacement case from same-object moves, which preserve state.

### B-05.4 One unreadable sub-folder stops every scan of that source

- **Claim:** An unreadable descendant directory terminates the whole source scan and repeatedly blocks remaining inventory and analysis.
- **What I traced:** `apps/portico-server/server/internal/ingestion/inventory.go:96` orders pending paths; `apps/portico-server/server/internal/storage/inventory_page.go:135` returns an open error; `apps/portico-server/server/internal/ingestion/service.go:472` fails the job. No directory-level permission skip/retry guard exists on this path.
- **Verdict:** **Confirmed** — A persistent permission error repeats on every complete traversal. Other sources are isolated, and previously committed rows survive. Rejecting incomplete absence evidence is deliberate (`portico-internal/Product Contracts/libraries-and-scanning/contract.md:313`), but it does not require aborting all readable descendants. Permissions are not identified per object as the contract calls for at line 203.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** High: a root containing one inaccessible folder can never complete ingestion of otherwise readable media.
- **Fix direction:** Record per-directory failures, continue safe inventory/analysis and keep affected coverage non-authoritative for absence. Add a standard system-folder exclusion policy. Touch storage listing, ingestion inventory/failure reporting and coverage schema. (**Size:** M).
- **Related:** B-05.2, B-05.5 and B-05.30 agree about failure propagation; B-05.6 overlaps system-folder filtering.

### B-05.6 Hidden and system files are catalogued as media

- **Claim:** Recognized extensions inside hidden/resource-fork/recycle paths are accepted as media.
- **What I traced:** `apps/portico-server/server/internal/storage/inventory_page.go:162` excludes only invalid names, symlinks when disabled and nonregular files. `apps/portico-server/server/internal/catalog/inventory.go:125,141` descends every directory and filters files by InventoryMedia; `apps/portico-server/server/internal/assets/assets.go:261` supplies extensions. Production association follows at inventory.go:353.
- **Verdict:** **Confirmed** — No ignore-name or hidden-path check intervenes. An exact copied FilenameTitle returned "Movie", 2009 for ._Movie (2009).mkv. Basic probe failure does not undo provisional publication. An extras folder can alter a subset of interpretations but does not protect recycle/resource-fork paths.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** Medium, reduced from High: catalogue pollution and warnings affect NAS/Mac libraries; the finding does not establish loss of valid media or a server outage.
- **Fix direction:** Add an explicit shared scanner exclusion policy for OS/NAS artefacts and resource-fork names, with an owner override for intentional hidden media. Touch storage/ingestion enumeration and fixtures. (**Size:** M).
- **Related:** B-05.4 overlaps system directories; B-05.16 is related junk ingestion with different names.

### B-05.7 A year inside a title is taken as the release year

- **Claim:** The first filename year-like number truncates title parsing even when that number belongs to the title.
- **What I traced:** `apps/portico-server/server/internal/catalog/catalog.go:354` uses an optional trailing separator and first match; `apps/portico-server/server/internal/catalog/catalog.go:510` uses it for movies; `apps/portico-server/server/internal/catalog/episodic.go:33` uses it for shows. `apps/portico-server/server/internal/metadata/screen_match.go:16,55,73` separately parses a year but prefers the catalogue year for search.
- **Verdict:** **Confirmed, narrower** — Copied Go code returned Blade Runner/2049 and Wonder Woman/1984. The metadata regex has a trailing boundary, unlike FilenameTitle, so the two parsers are not identical for resolution/date strings. NFO/manual/provider facts can later repair a title, and explicit provider IDs offer another route, but automatic name search starts from erroneous evidence.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** Medium: affected titles can be misnamed/mismatched, recoverable through local evidence or owner matching; the impact is title-specific.
- **Fix direction:** Centralize filename year extraction, prefer explicit bracketed release years and bounded standalone trailing years, and keep title-internal digits. Touch catalog filename/episode parser and metadata/screen_match. (**Size:** S).
- **Related:** B-05.17 overlaps cleanup; B-06.16 concerns NFO recovery rather than filename matching.

### B-05.8 A second file for a movie is a second movie; an upgraded file loses the title's personal state

- **Claim:** Distinct files at distinct paths become separate movie entities; a replacement with a new name can leave viewer state on the old title.
- **What I traced:** `apps/portico-server/server/internal/catalog/inventory.go:193,203,306` distinguishes new assets from exact object reuse; `apps/portico-server/server/internal/catalog/catalog.go:496,511` uses ItemKey; `apps/portico-server/server/internal/compactcatalog/write.go:85,164` maps the path key to an entity. Audio does likewise at `apps/portico-server/server/internal/catalog/audio_repository.go:287`. Exact object+size+mtime reuse at inventory.go:207 preserves an existing asset link.
- **Verdict:** **Confirmed, narrower** — Confirmed for a distinct upgraded file under a new path. A plain rename/move of unchanged bytes preserves the original item and state; no claim of all renames losing state is justified. The old entity/state is retained, rather than erased. Ambiguous copies intentionally remain separate (`portico-internal/Product Contracts/metadata/contract.md:134`); automatic proven identity grouping is the known unfinished part. Owner association exists (`apps/portico-server/server/internal/catalog/inventory_admin.go`).
- **Class:** Unfinished feature. **In Backlog?** Yes — second-file grouping.
- **Severity:** High: automated quality upgrades can make a new card appear unwatched and detach normal access to existing personal state.
- **Fix direction:** Implement proven same-title version association and state-preserving merge/aliases, with explicit handling of editions and conflicts. Do not blindly merge by filename. Touch inventory association, compactcatalog identities, metadata equivalence and viewer/reference merge registry. (**Size:** L).
- **Related:** B-12.18 agrees only for distinct replacement files; it overextends to ordinary moves. B-07.3 is the opposite wrong-merge hazard and should be solved with the same identity redesign.

### B-05.10 Flat show folders, and the show folder's year, are only understood inside a Season N folder

- **Claim:** Flat show folders do not supply missing series title/year; only recognized Season N/Specials parents supply that context.
- **What I traced:** `apps/portico-server/server/internal/catalog/inventory.go:367` invokes ParseEpisode on the source-relative path. `apps/portico-server/server/internal/catalog/episodic.go:41,44,132` sets showFolder only for recognized season directories. Show keys include year at `apps/portico-server/server/internal/catalog/episode_repository.go:123`.
- **Verdict:** **Confirmed** — Exact copied Go parser rejects Severance (2022)/S01E01.mkv and gives Doctor Who year 0 in a flat folder versus 2005 in a Season 2 folder. A filename already carrying its own full identity succeeds; flat layouts are not all rejected. No upstream folder resolver fills this gap. Manual assignment is a recovery, not automatic parsing.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** High: common bare-numbered flat seasons fail automatic show ingestion; mixed layouts split show identities.
- **Fix direction:** Resolve show/season context from the containing hierarchy before filename parsing, with explicit flat-folder and nested-disc rules. Touch catalog/episodic, episode association and naming fixtures; plan existing-show reconciliation. (**Size:** M).
- **Related:** B-05.27 is the identity consequence; B-05.11 concerns strict comparison after folder context is found.

### B-05.11 The folder-must-match rule rejects correctly organised shows

- **Claim:** Folder/file series names must pass EqualFold after limited cleanup; common punctuation/spacing/ID-hint variants fail.
- **What I traced:** `apps/portico-server/server/internal/catalog/episodic.go:33,135` uses FilenameTitle then strings.EqualFold; `apps/portico-server/server/internal/catalog/inventory.go:371` calls it before provider matching. `apps/portico-server/server/internal/catalog/episode_repository.go:104` stores issues and creates no episodes for an issue.
- **Verdict:** **Confirmed, narrower** — Copied Go confirmed Mr. Robot and The Office (US) failures. The first report describes Mr. Robot spacing in reverse: the folder cleans to two spaces and the file prefix to one, but the failure stands. Provider matcher normalization (`apps/portico-server/server/internal/metadata/screen_match.go:24`) is more permissive but never fixes a file that has no episode entity. Romaji versus English is genuinely ambiguous and should remain an owner decision; punctuation/spacing differences are needless rejection.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** High: standard release names can exclude an entire show from usable automatic episode ingestion.
- **Fix direction:** Use a canonical comparison for punctuation, whitespace, Unicode and supported provider hints; make folder authority explicit without silently resolving truly different names. Touch episodic/name helpers, issues and fixtures. (**Size:** M).
- **Related:** B-05.32 is its Unicode subcase; B-05.26 reports persistent warnings.

### B-05.18 File identity includes the device number, which is not stable across reboots on many setups

- **Claim:** A remount that changes device/inode evidence may be mistaken for a replaced source root and force reanalysis.
- **What I traced:** `apps/portico-server/server/internal/storage/local_observation_linux.go:16` emits dev/inode. `apps/portico-server/server/internal/storage/inventory_page.go:51` returns that as RootIdentity; `apps/portico-server/server/internal/ingestion/inventory.go:52` rejects mismatch. `apps/portico-server/server/internal/catalog/library_sources.go:334` requires acceptance and changes incarnation. `apps/portico-server/server/internal/catalog/inventory.go:193` replaces old-incarnation locations.
- **Verdict:** **Unsettled — needs runtime** — Conditional mechanism confirmed; actual device instability on supported mounts was not established. The listed filesystem families are too broad to accept as a universal claim. Fsid is used for separate mount checking (`apps/portico-server/server/internal/storage/mount_identity_unix.go:32`) and itself does not prove a persistent identity suitable across reboots. Cheapest experiment: on a disposable representative NFS/SMB/FUSE mount, capture root dev/inode and child dev/inode/size/mtime, unmount/remount in reversed mount order, and compare. Root evidence change with the same backing dataset confirms this trigger; stable evidence refutes it for that setup.
- **Class:** Design risk. **In Backlog?** No.
- **Severity:** Medium, reduced from conditional High: affected remounts stop source work and require owner intervention, but do not establish destructive loss; incidence is unknown.
- **Fix direction:** Add filesystem-aware persistent identity evidence where reliable, with conservative explicit rebind and diagnostics when continuity is uncertain. Touch storage root/mount observations and library_sources incarnation logic. (**Size:** M).
- **Related:** Q-24.2 concerns safe unmount handling and agrees that safeguards must remain; B-05.8 is a separate logical identity issue.

### B-05.19 A directory's timestamp is not always evidence that nothing was added

- **Claim:** Backends that do not change directory fingerprints for new children may be skipped until a full scan.
- **What I traced:** `apps/portico-server/server/internal/ingestion/incremental.go:49,69` trusts a cached directory revision; `apps/portico-server/server/internal/storage/inventory_page.go:46` includes size, mtime, ctime/change token and object identity. Linux direct listing uses getdents; `apps/portico-server/server/internal/storage/inventory_page.go:151` selects the streamed guard only for stream/non-Linux. Known children still get visited (incremental.go:84).
- **Verdict:** **Unsettled — needs runtime** — The mechanism requires all fingerprint inputs, including size and ctime, to remain equal. The first report considered timestamps largely alone and its general filesystem precision statements were not verified. No universal mergerfs/SMB/rclone failure is established. Managed remote adapters do not use this local skip path. Cheapest experiment: on the intended disposable union/cloud OS mount, record the exact four fingerprint fields before and after adding a child on another branch; an unchanged fingerprint despite a new child confirms the skip trigger. Then compare an incremental and full inventory in a later isolated product test.
- **Class:** Design risk. **In Backlog?** No.
- **Severity:** Medium: new files can remain unseen on affected OS-mounted backends; frequency and prevalence require evidence.
- **Fix direction:** Offer per-source always-enumerate policy for weak directory evidence and guard recently modified/coarse revisions. Touch ingestion/incremental, source settings and storage capability classification. (**Size:** M).
- **Related:** B-05.3 shares the directory checkpoint, but in-place file mutation is independently established; do not merge the two triggers completely.

### B-05.20 Scan bookkeeping is never pruned

- **Claim:** Terminal scan history retains per-directory and per-object queue rows without automatic pruning.
- **What I traced:** `apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:1883,1900,1910` defines history tables. `apps/portico-server/server/internal/catalog/inventory.go:136` creates directory work; `apps/portico-server/server/internal/ingestion/analysis.go:123` writes an object queue row on each Basic scan, including done rows. finishScanTx deletes only the active claim at analysis.go:570. Global delete search found queue deletion only in `apps/portico-server/server/internal/catalog/purge.go:71`.
- **Verdict:** **Confirmed, narrower** — Confirmed for retained bookkeeping, not literally every inventory table: active/policy pending rows are deleted, and object purge deletes related analysis rows. File List Only avoids the all-object enqueue pass. The event feed intentionally has no retention in `apps/portico-server/server/internal/ingestion/INVENTORY_CONTRACT.md:147`; that is a trade-off, not accidental proof of a bug. Repeated directory/queue history still scales with number of completed scans.
- **Class:** Design risk. **In Backlog?** No.
- **Severity:** High at sustained scheduled-scan scale: growing state and backups can consume disk even on unchanged libraries; first-pass arithmetic is plausible row-count arithmetic, not a measured byte forecast.
- **Fix direction:** Prune terminal per-run work in bounded maintenance batches, retaining data needed for mode selection and owner diagnostics. Define event cursor expiry/retention independently. Touch ingestion terminal cleanup, inventory_runs/queues schema indexes and maintenance. (**Size:** M).
- **Related:** B-05.21 creates the retained done rows; B-07.15 is a separate insert-only dictionary/ledger lifetime problem.

### B-05.25 Deleting media on another volume copies it into the server's state folder

- **Claim:** An authorized physical delete copies cross-volume media into state/trash, potentially filling the database volume.
- **What I traced:** `apps/portico-server/server/internal/httpapi/administration.go:194,206` wires owner administration Delete. `apps/portico-server/server/internal/administration/deletion.go:403,417,432` handles DeleteFiles, skips shared/missing files and journals moves to trashRoot (line 301). `apps/portico-server/server/internal/administration/file_operations.go:365,393,417` tries a hard link then copies into a private temporary file; source removal happens after complete publication.
- **Verdict:** **Confirmed, narrower** — Confirmed only when DeleteFiles is selected and library deletion permission allows it; disabled by default, with typed confirmation and revision checks. Retention is configurable, not invariably 30 days. Catalog-only inventory trash uses another route and never moves bytes. Recovery preserves originals on incomplete copy, but no destination free-space reservation was found. The first report calls this rename fallback; current code uses hard-link/unlink then copy, with the same cross-volume cost.
- **Class:** Design risk. **In Backlog?** No.
- **Severity:** Medium: owners physically deleting large files on separate media disks can exhaust state storage; arbitrary scans do not trigger it.
- **Fix direction:** Reserve/check destination capacity and keep recoverable trash on the media filesystem where permitted, excluded from scanner traversal. Touch administration deletion/file_operations and trash manifest/recovery; keep original-file durability. (**Size:** M).
- **Related:** B-09.18 concerns backups on state disk; B-18.13 must distinguish catalogue trash from this physical delete path.

### B-05.28 On Windows the server cannot add a library

- **Claim:** Windows local/OS-mounted library creation cannot obtain the required root identity.
- **What I traced:** `apps/portico-server/server/internal/storage/local_observation_other.go:1,7` covers Windows and returns unsupported/empty identity. `apps/portico-server/server/internal/storage/client.go:340` discards the error and emits an empty object identity; `apps/portico-server/server/internal/catalog/catalog.go:147` rejects it. SaveSource and scans also reject at `apps/portico-server/server/internal/catalog/library_sources.go:266` and `apps/portico-server/server/internal/ingestion/inventory.go:49`. `apps/portico-server/README.md:43` explicitly advertises Windows x64/ARM64.
- **Verdict:** **Confirmed, narrower** — Source-confirmed release blocker for local libraries on advertised Windows targets, rather than a conditional unsupported-platform concern. Windows playback-specific helpers do not implement observedObjectBinding. Managed remote sources can provide remote identity and are outside the blanket claim that no library at all can be added. No cross-compilation or Windows runtime was performed.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** Critical for Windows local server usefulness: normal local library creation fails deterministically at the identity admission boundary.
- **Fix direction:** Implement Windows observed object/root bindings using volume and file IDs and revision evidence, and verify retained scan/probe capability at the same time. Touch storage/local_observation Windows implementation, root observation/scan helpers and platform acceptance fixtures. (**Size:** M).
- **Related:** B-09.5 and B-25.3 are independent Windows access/package blockers; Q-24.30 shares platform acceptance scope.

### B-05.29 Local metadata files are rejected whole when they exceed a budget

- **Claim:** Local metadata parsing rejects complete documents when a field/structure budget or unsupported syntax is encountered.
- **What I traced:** `apps/portico-server/server/internal/localmetadata/video_parse.go:49,67,75,104,152,214` enforces whole-document budgets/DOCTYPE/trailing-text rejection; no CharsetReader is installed. `apps/portico-server/server/internal/localmetadata/evidence.go:70` rejects text beyond 4096 bytes. `apps/portico-server/server/internal/localmetadata/video_parse_test.go:24` explicitly asserts external declarations and oversized inputs are rejected.
- **Verdict:** **Confirmed, narrower** — Limits and whole-document rejection are deliberate defensive bounded-input behavior, compatible with Basic budgets (`portico-internal/Product Contracts/libraries-and-scanning/contract.md:97`). An isolated standard-library XML decoder check accepted UTF-8 and rejected both US-ASCII and ISO-8859-1 declarations without a CharsetReader. Credit rejection occurs after 100 accepted credits, not necessarily 100 actor tags if some lack names; OPF bound is bytes, not characters. Unsupported legacy encodings/Kodi mixed NFO are compatibility gaps; DOCTYPE rejection should remain.
- **Class:** Deliberate trade-off. **In Backlog?** No.
- **Severity:** Medium: local-only users can lose all evidence from oversized/legacy sidecars, while media inventory remains; this is not parser unboundedness or media disappearance.
- **Fix direction:** Keep byte/depth limits and DOCTYPE rejection; add safe bounded encoding conversion and optional field truncation with explicit partial-evidence diagnostics. Touch localmetadata video/evidence parsers and analysis warning projection. (**Size:** M).
- **Related:** B-06.16 duplicates the same whole-NFO rejection and should own the combined metadata compatibility task; Q-24.5 concerns opposite resource-exhaustion risk.

### B-05.32 Accented names written two ways may not be recognised as equal

- **Claim:** Canonically equivalent composed/decomposed Unicode series or artist names can fail equality or create separate identities.
- **What I traced:** `apps/portico-server/server/internal/catalog/episodic.go:137` uses EqualFold without Unicode normalization. `apps/portico-server/server/internal/catalog/episode_repository.go:123` lowercases/collapses whitespace only; `apps/portico-server/server/internal/catalog/audio_repository.go:112,212` does the same for artist identity. The provider matcher uses NFKD at `apps/portico-server/server/internal/metadata/screen_match.go:24`, but that is downstream.
- **Verdict:** **Confirmed** — An exact copied Go parser returned folder_and_filename_series_conflict for Café/Season 1/Cafe\u0301 - S01E01.mkv; EqualFold returned false. This settles string behavior without an actual Mac/Linux filesystem. Only differing canonical encodings trigger it; ordinary accents with the same encoding work. The DB ledger uses raw normalized-by-case keys, not the sort key, so later accent-folded sorting does not merge identities.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** Medium, standardized upward from Low–Medium: affected shows fail ingestion and equivalent artists split, beyond a cosmetic sort difference.
- **Fix direction:** Normalize identity/equality inputs to NFC before case/whitespace cleanup, preserving original display text; reconcile existing keys without losing state. Touch episodic, episode_repository, audio localKey and identity alias migrations. (**Size:** S).
- **Related:** B-05.11 is the broader folder comparator issue; merge this into that parser/identity correction where possible.

### B-05.33 With "follow symlinks" on, a folder alias inside the root may make every incremental scan fail on a database constraint

- **Claim:** A cached same-root directory alias can collide with the per-job visited identity index on incremental scans.
- **What I traced:** `apps/portico-server/server/internal/catalog/inventory.go:133,136` caches both child paths but uses INSERT OR IGNORE for the visited index. `apps/portico-server/server/internal/ingestion/incremental.go:84,109` recreates both paths with empty identity; line 113 fills identity with UPDATE. `apps/portico-server/server/internal/catalog/inventory.go:115` does likewise for a listed directory. Unique index is `apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:3717`.
- **Verdict:** **Confirmed** — No visited-identity check precedes those UPDATEs. Scratch SQLite with the exact copied inventory_directories table and unique index allowed two empty-identity paths then rejected the second identical-identity UPDATE. This establishes the database constraint interaction, not a full product scan. Needs follow_symlinks (default off), cached aliases and unchanged parent; full traversal INSERT OR IGNORE alone is safe.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** Medium: an otherwise valid alias layout can make every incremental scan fail until full mode or topology change.
- **Fix direction:** Apply visited-identity handling before updates and cache replay; safely skip duplicate directory work without treating it as absent coverage. Touch ingestion/incremental and catalog/CommitInventoryPage with a two-scan alias fixture. (**Size:** S).
- **Related:** B-05.5 is a different symlink failure; B-05.2 shares analysis starvation after terminal failure.

### B-07.1 One permanently failing entity stops all second-tier derived workers

- **Claim:** A permanently failing queued derivation prevents all step workers from ever receiving a turn.
- **What I traced:** `apps/portico-server/server/cmd/server/main.go:958` starts NewWorker.Run. `apps/portico-server/server/internal/compactcatalog/derive.go:246` selects any rebuilding/dirty domain; runSteps occurs only at line 248 when none exists. `apps/portico-server/server/internal/compactcatalog/derive.go:322` retries poison-only queues, and setAside at line 443 changes scheduling time without removing the dirty row. Steps list is line 388.
- **Verdict:** **Confirmed** — Poison isolation helps other domain/entity work, so not every derivation stops. It cannot reach the second-tier branch while a poisoned row remains. Restart clears in-memory isolation, not the durable dirty row. Tests found for derivations do not disprove this branch starvation. Persistent healthy import traffic can also delay steps until complete drain. No concrete naturally occurring poison entity was established; the failure-isolation defect is conditional but deterministic.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** High when triggered: personal sorting, restricted visibility counts, collection/book counts, eligibility and recommendation profiles stop converging server-wide.
- **Fix direction:** Schedule a bounded step-worker turn independently of primary queue emptiness; quarantine/retry poison rows with durable diagnostics and bounded backoff. Touch compactcatalog/Worker.Step, setAside and step scheduling. (**Size:** M).
- **Related:** B-07.2 can prolong rebuilding if a poison entity prevents completion; Q-24.6 concerns derived/read cost rather than this liveness defect.

### B-07.2 Raising a derivation version takes browsing or search offline for the whole rebuild

- **Claim:** Version bumps withhold browse/search projections globally while the required domain rebuilds.
- **What I traced:** `apps/portico-server/server/internal/compactcatalog/derive.go:209` sets rebuilding on version increase; line 337 clears after empty backfill. CheckReadiness at line 127 tests domain rebuilding only. `apps/portico-server/server/internal/catalog/compact_browse.go:6` checks six domains globally; `apps/portico-server/server/internal/catalog/search.go:480` checks search domain 17. Browse is wired at browse_engine.go:397.
- **Verdict:** **Confirmed, narrower** — This deliberately implements `portico-internal/Project Architecture/03 Catalogue and Metadata.md:26`, which says ErrBuilding during rebuild. It conflicts with `apps/portico-server/server/internal/persistence/README.md:40` promising old rows keep serving. Browse/search are affected only when their required domain changes; unrelated rebuilds do not necessarily take them offline, and search live/people groups can remain independently useful. Old derived rows need not be safe under new code semantics, so serving them unchanged is not a trivial fix. Duration is unmeasured.
- **Class:** Deliberate trade-off. **In Backlog?** No.
- **Severity:** High at large-library upgrade scale: required projections are withheld across all libraries until background reconstruction finishes; not a claimed measured hours-long outage.
- **Fix direction:** Decide whether this availability trade-off is acceptable and reconcile README. To preserve service use versioned/shadow projections with validated atomic switch and storage budget. Touch compactcatalog derivation/readiness, browse/search readers and migration rules. (**Size:** L).
- **Related:** B-25.18 covers blocking migration rewrites, a separate upgrade mechanism. B-07.1 can indefinitely extend this rebuild boundary.

### B-07.3 A new file at a moved movie's old path is attached to the moved movie

- **Claim:** Reusing a moved file’s old path can link unrelated new media to the moved movie’s original entity.
- **What I traced:** `apps/portico-server/server/internal/catalog/inventory.go:203,236` recognizes a unique unchanged object moved from P to Q; existing association skips re-keying at line 306. `apps/portico-server/server/internal/compactcatalog/write.go:164,178` reuses the live entity named by the original root/path key. A new asset at P reaches commitMovieItem (`apps/portico-server/server/internal/catalog/catalog.go:511`) and links to that entity.
- **Verdict:** **Confirmed** — No relocation path updates or disowns the ledger identity. The original asset remains linked at Q, so the new P asset shares the old movie. New metadata does not automatically split this association; EnsureEntityTx preserves facts. Limited to reuse of an old same-library path after recognized continuity, not any arbitrary new file. Audio uses the same ItemKey, though deeper regrouping is outside this movie proof.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** Medium: reused inbox/generic names can silently put two films under one title and viewer state.
- **Fix direction:** Separate pathname aliases from immutable logical identity, and retire/reassign locator ownership on recognized moves; only reuse a title after proven continuity. Touch catalog inventory relocation, compactcatalog identities/ItemKey and version linking. (**Size:** M).
- **Related:** B-05.8 is the inverse identity error and shares the remedy; B-07.15 describes the never-pruned ledger but not this live-entity collision.

### B-07.11 DELETE /v1/items/{id} is a raw entity delete

- **Claim:** The registered owner DELETE item route bypasses the administration deletion/purge workflow.
- **What I traced:** `apps/portico-server/server/internal/httpapi/router.go:1110` calls Catalog.Delete; `apps/portico-server/server/internal/catalog/catalog.go:328` uses Background context and DeleteEntityTx after deleting playback/provider rows. `apps/portico-server/server/internal/compactcatalog/write.go:580` deletes one entity. Parent/show/album references lack cascade (`apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:41,129,154`). Personal_items and progress cascade at lines 345 and 1277.
- **Verdict:** **Confirmed, narrower** — Container deletion with children fails a restrictive FK and rolls back, rather than partially deleting it. A simple deletable movie loses personal state but retains inventory assets; a later listing can recreate it via inventory.go:306. An unchanged incremental skip does not recreate it, so "next scan" is too broad. Permanent owner play_history survives by design (`apps/portico-server/server/internal/persistence/migrations/0007_play_history.sql:9`), unlike personal_history. No maintained client caller of this raw DELETE route was found; it is still exposed to owner/API consumers.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** Medium: owner API deletion fails for containers or unexpectedly loses viewer state/reappears after re-enumeration; not an unauthenticated delete.
- **Fix direction:** Remove/deprecate the raw route or delegate to a single authorized revision-fenced administration semantic; propagate request context. Handle descendants/shared assets and clearly define catalogue-only reappearance. Touch httpapi/router, catalog/Delete and administration/purge registry. (**Size:** M).
- **Related:** B-05.25 is the separately guarded physical delete path. B-07.3/B-07.15 explain identity reuse on later recreation.

### B-07.13 The backup pins one snapshot for the whole copy

- **Claim:** A backup pins its source WAL snapshot until the database copy finishes; disk-pressure consequences are conditional.
- **What I traced:** `apps/portico-server/server/internal/backup/copy.go:37,43` begins and materializes a read snapshot, then performs Step(16) at line 71 on the same raw connection. It releases snapshot and connection at lines 111/117 before destination fsync and quick_check. `apps/portico-server/server/internal/backup/backup.go:374` checks free space once before copying. Checkpoint pressure thresholds are `apps/portico-server/server/internal/dbwork/wal.go:147`.
- **Verdict:** **Confirmed, narrower** — The pinned boundary is explicit and intentional to prevent copy restarts. Quick_check does not extend it. No backup-specific pressure abort was found; free-space precheck and checkpoint escalation mitigate but cannot recycle frames protected by this reader. Neither material WAL growth nor slower reads is established here; log length alone does not guarantee latency. VACUUM INTO also reads a long snapshot, so it does not by itself fix the risk.
- **Class:** Design risk. **In Backlog?** No.
- **Severity:** Medium latent operational risk: concurrent high write volume plus a slow large backup can consume state disk; real thresholds and duration need measurement.
- **Fix direction:** Bound backup snapshot lifetime or abort/retry when WAL/free-space budgets cross a reserve; schedule scan-idle when practical without silently pausing all background work. Touch backup/copy, backup/service and dbwork pressure metrics. Cheapest acceptance: large disposable database backup plus bounded writes, recording copy duration, WAL bytes, free space and checkpoint outcomes. (**Size:** M).
- **Related:** B-09.30 is the same mechanism (rated Low there); Q-24.26 adds disk-exhaustion impact. Merge into one measured backup-pressure item.

### B-07.16 Nothing reports a stuck write gate in production

- **Claim:** Production lacks an automatic stuck-write-gate alarm naming the holder, although gate hold duration is exposed.
- **What I traced:** `apps/portico-server/server/internal/dbwork/gate.go:90,120,166` enables holder stack/timer only through traceHolders → strict (line 393); StrictEnv is `apps/portico-server/server/internal/dbwork/context.go:14`. CurrentHeldMilli is reported by class at gate.go:335,355; `apps/portico-server/server/internal/httpapi/admin_concurrency.go:146` publishes WriteGate Stats.
- **Verdict:** **Confirmed, narrower** — An observable diagnostic exists, so "nothing reports" is too broad. No gate-specific automatic alarm or stuck-holder attribution exists with strict mode off. The generic database watchdog can report connection/query failure (`apps/portico-server/server/internal/dbwork/health.go:120`), but does not inspect gate age or identify the holder. WithWriteTx defers rollback (`apps/portico-server/server/internal/dbwork/write.go:335`) and context-carried nested writes reuse a transaction, mitigating the suggested self-deadlock. No real leaking/nested fresh-context call path was established. Strict timer measures a waiter, not exactly age of the holder.
- **Class:** Design risk. **In Backlog?** No.
- **Severity:** Low latent diagnosis gap: a future stuck holder could stall writers with poor automatic attribution; no current production wedge is proven.
- **Fix direction:** Add low-overhead threshold logging/health reporting keyed to actual heldSince; optionally capture attribution only after threshold. Touch dbwork/gate and diagnostics/health projection. (**Size:** S).
- **Related:** B-07.18 concerns long but completing batches, not a proven leaked gate.

### B-07.18 Derived batches may hold the write gate for long stretches

- **Claim:** A bounded 64-unit derivation transaction might still hold the single writer too long on weak hardware.
- **What I traced:** `apps/portico-server/server/internal/compactcatalog/derive.go:44,243,361` executes a domain batch inside one maintenance write transaction. `apps/portico-server/server/internal/dbwork/write.go:331` holds the gate until commit/rollback. `apps/portico-server/server/internal/compactcatalog/derive.go:186` calls PaceBackground only after release; `apps/portico-server/server/internal/dbwork/background_pacing.go:17` bounds between-batch foreground breathing room.
- **Verdict:** **Unsettled — needs runtime** — Priority can select the next writer but cannot preempt the current batch. Conversely, 64 units is already bounded, and some derivations have smaller fanout stages. The architecture’s per-entity whole-drain timing does not measure a single domain’s hold; multiplying it by 64 is not a reliable contention estimate. Cheapest experiment: on a disposable representative large catalogue on weak hardware, run the production derivation worker while issuing playback/progress writes and collect by-class max/current held milliseconds plus foreground p95/p99 wait. Only excessive foreground waits attributable to these holds establish the performance defect.
- **Class:** Design risk. **In Backlog?** No.
- **Severity:** Low pending measurement: a possible latency problem under derivation load, not demonstrated playback failure.
- **Fix direction:** If measured, split expensive domains or adapt batch budget to elapsed time between safely resumable units. Touch compactcatalog domain Drain functions/Worker.Step and gate metrics. Size M for measured batch correction; measurement alone S. (**Size:** M).
- **Related:** Q-24.1 covers the same general foreground-versus-background pressure; B-07.16 is diagnosis rather than batch duration.

## Tier 2 — quick checks

Entries needing materially corrected scope/classification are expanded to the full format here; they remain counted in their assigned Tier 2 coverage.

### B-05.5

**Confirmed — Deliberate trade-off; Medium; Size M.** With follow_symlinks enabled, EvalSymlinks failure or a root escape returns a page error (`apps/portico-server/server/internal/storage/inventory_page.go:175`), then fails the source (`apps/portico-server/server/internal/ingestion/service.go:472`). Escape rejection is required/tested (`apps/portico-server/server/internal/storage/inventory_page_test.go:135`); retaining that safeguard while isolating a dangling-link warning would touch storage/ingestion coverage. **In Backlog?** No.

### B-05.9 Renaming a file to fix it does not change how Portico read it — expanded check

- **Claim:** A rename correcting a previously accepted wrong title/episode does not update its automatic name interpretation.
- **What I traced:** `apps/portico-server/server/internal/catalog/inventory.go:203,306` preserves exact asset continuity then runs association only if no entity link exists. `apps/portico-server/server/internal/catalog/catalog.go:502` reuses movie links; `apps/portico-server/server/internal/compactcatalog/write.go:139` preserves facts. `apps/portico-server/server/internal/catalog/inventory.go:371` reparses when association is absent.
- **Verdict:** **Confirmed, narrower** — An accepted S01E02 renamed S01E03 remains linked to episode 2. However a file with a naming issue and no entity links is reparsed on listing and can become valid after rename; a general claim that renaming never helps is wrong. Version-triggered issue reparsing also exists (`apps/portico-server/server/internal/catalog/episodic_reparse.go:29`) and is wired at startup (`apps/portico-server/server/cmd/server/main.go:968`), but does not update accepted wrong links on ordinary renames.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** Medium: owners correcting accepted names see stale mapping; issue-only files have a recovery path.
- **Fix direction:** Reinterpret automatic associations on relocation, preserving manual/provider locks and viewer state; touch inventory association, episodic linking and filename metadata provenance. (**Size:** M).
- **Related:** B-05.10/B-05.11 are parser rejection cases; B-12.18 incorrectly generalizes renames as state loss rather than preserved-but-stale interpretation.

### B-05.12

**Confirmed — Defect; Medium; Size S.** The unbounded x marker accepts 1920x1080 as season/episode and multiple markers trigger conflict (`apps/portico-server/server/internal/catalog/episodic.go:22,40,51`). Exact copied Go reproduced both outcomes. Tighten marker boundaries/ranges and ignore resolution/codec tokens in the episode parser. **In Backlog?** No.

### B-05.13

**Confirmed, narrower — Defect; Medium; Size M.** The date-to-year-season conflict, terminal Part N rejection and x-range shortening follow `apps/portico-server/server/internal/catalog/episodic.go:61,82,111`; copied Go reproduced representative cases. The eight-link cap and multipart manual assignment are deliberate bounds, so retain those while fixing date hierarchy and silent marker shortening in the parser. **In Backlog?** No.

### B-05.14

**Confirmed, narrower — Defect; Medium; Size S.** Every relative directory segment is eligible as an extras folder before media-kind association (`apps/portico-server/server/internal/catalog/extras.go:130`, `apps/portico-server/server/internal/catalog/inventory.go:360`), swallowing the show Extras/top-level Shorts. TV/anime Specials is explicitly exempt at extras.go:132. Restrict extras recognition to appropriate media-parent depth; touch extras classifier and fixtures. **In Backlog?** No.

### B-05.15

**Confirmed, narrower — Missing feature mislabelled as a bug; Medium; Size M.** M2TS/VOB are supported ordinary files (`apps/portico-server/server/internal/assets/assets.go:261`), and each unassociated asset creates a path-keyed movie (`apps/portico-server/server/internal/catalog/catalog.go:511`). Disc-layout grouping is absent; `portico-internal/Product Contracts/libraries-and-scanning/contract.md:206` says MAY, and INVENTORY_CONTRACT explicitly chooses ordinary extracted-file handling. Optional grouping/suppression is a catalogue feature; touch disc detection/inventory association. **In Backlog?** No.

### B-05.16

**Confirmed — Defect; Medium; Size S.** Neither the extras folder map nor suffix list recognizes Sample/sample or bare trailer.mkv (`apps/portico-server/server/internal/catalog/extras.go:25,53,148`), so supported files reach ordinary title/episode association. Add contextual sample/standalone-trailer exclusions/classification in extras and scanner policy; avoid suppressing actual films named Trailer/Sample globally. **In Backlog?** No.

### B-05.17

**Confirmed, narrower — Defect; Low; Size S.** FilenameTitle retains release tags/separators/hints and does not collapse internal whitespace (`apps/portico-server/server/internal/catalog/catalog.go:354`). Matcher cleanup is better (`apps/portico-server/server/internal/metadata/screen_match.go:40`), and NFO/manual evidence can replace it; local-only unannotated files remain untidy. Consolidate cleanup/provenance in catalog/metadata filename helpers; severity Low for display-only cases, B-05.11 owns identity failures. **In Backlog?** No.

### B-05.21 Every scan re-queues every file — expanded check

- **Claim:** Ordinary Basic scans perform an all-object enqueue sweep and retain a per-run row even for unchanged completed objects.
- **What I traced:** `apps/portico-server/server/internal/ingestion/analysis.go:41` exits early for File List Only; line 50 walks available current-incarnation objects without unsupported reasons. Lines 89–123 compute missing operations and insert done/pending/skipped rows. analyzeOne at line 169 selects pending only. `apps/portico-server/server/internal/catalog/inventory.go:336` separately queues changed observations.
- **Verdict:** **Confirmed, narrower** — The all-object bookkeeping and paced pages are real. "Every file" excludes unavailable/retired/unsupported objects and File List Only; it does not mean every file is analyzed again. The sweep also checks missing deep-operation heads and STRM target-policy versions, so simply deleting it would miss valid reanalysis needs. It is a bounded but library-wide recurring cost.
- **Class:** Design risk. **In Backlog?** No.
- **Severity:** Medium: unchanged large scheduled sources incur avoidable traversal/writes and retained history, without rereading all media bytes.
- **Fix direction:** Maintain pending-by-revision/policy/stage evidence and selectively queue affected objects, retaining deep-head/policy invalidation. Touch ingestion/analysis, inventory analysis readiness and deep-operation invalidation. (**Size:** M).
- **Related:** B-05.1 sets page cadence; B-05.20 retains these rows. Plan them together.

### B-05.22 Looking for sidecars that are not there costs dozens of process starts per file — expanded check

- **Claim:** A directory’s missing sidecar candidates are repeatedly probed per analyzed file, without negative caching.
- **What I traced:** `apps/portico-server/server/internal/localmetadata/sidecars.go:42,57,112` iterates names and caches hits only. Analysis binds a root/read guard (`apps/portico-server/server/internal/ingestion/analysis.go:308`); `apps/portico-server/server/internal/storage/client.go:367` executes CheckScanRead before launching the blob helper at line 390. The guard’s InventoryStat at analysis.go:320 returns the missing-path error.
- **Verdict:** **Confirmed, narrower** — Dozens of helper launches per analyzed episode are possible from candidate stat misses, within the sidecar time budget. The first-pass implication of stat plus read helper for each missing candidate is wrong: the read helper is skipped. VideoNFO also performs a direct pre-stat (`apps/portico-server/server/internal/localmetadata/video.go:43`) before ReadSmall. Per-run completed analysis avoids the work on clean ordinary rescans (B-05.21).
- **Class:** Defect. **In Backlog?** Yes — missing-sidecar cache.
- **Severity:** Medium: first import or legitimate reanalysis of large directories with no sidecars pays repeated process-launch cost.
- **Fix direction:** Use bounded per-scan directory name/negative caches, invalidated by directory revision, without preventing newly added sidecars from being read. Touch localmetadata sidecars/video and ingestion read context. (**Size:** M).
- **Related:** The Backlog explicitly records this; B-05.1 compounds the import latency.

### B-05.23

**Confirmed — Deliberate trade-off; Medium; Size S.** Primary-source trigger leaves interval_seconds=0 (`apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:1856,4122`); the web create form/helper does not set a recurring interval (`apps/portico-server/web/src/screens/server/Libraries.tsx:212`). Add-folder drafts use 3600 (`apps/portico-server/packages/client-core/src/server-admin/panel-logic.ts:151`). Scheduling is explicitly optional; reconcile defaults or explain the choice during creation, touching the form/helper and source defaults. **In Backlog?** No.

### B-05.24

**Confirmed, narrower — Deliberate trade-off; Medium; Size S.** At least two completed absence observations and elapsed configured grace precede missing state (`apps/portico-server/server/internal/ingestion/inventory.go:294,310`); default is 24h and there is no automatic item purge. Retained availability is not guaranteed playable bytes. This protects history and unreliable mounts (`portico-internal/Product Contracts/libraries-and-scanning/contract.md:310`); improve missing-candidate presentation/docs rather than shorten grace automatically. **In Backlog?** No.

### B-05.26 One unplaceable file makes a TV library permanently "degraded" — expanded check

- **Claim:** A persistent unassigned episodic file keeps warning state, and the predicate propagates that warning to clean sources in the same library.
- **What I traced:** finishScanTx uses EXISTS on episodic_sources by library_id at `apps/portico-server/server/internal/ingestion/analysis.go:560`; it applies that condition to jobs and library_sources at lines 561/567. Assignment clears issues through `apps/portico-server/server/internal/catalog/episode_repository.go:104`. Purge can delete episodic_sources (`apps/portico-server/server/internal/catalog/purge.go:75`).
- **Verdict:** **Confirmed, narrower** — Persistent warnings are intentionally tested (`apps/portico-server/server/internal/ingestion/scan_attention_test.go:17`) and are truthful for the affected library; they are not permanent in all circumstances, since assignment/purge can clear them. The sharper defect is source attribution: a naming issue in source A labels source B degraded too. No ignore naming action was found, but absence of that feature alone does not make a warning false.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** Medium, rather than Low–Medium: multi-source owners can be sent to repair the wrong folder, while one unassigned issue keeps repeated warnings.
- **Fix direction:** Scope source-health issues through inventory_objects/source_id while keeping a separate library aggregate; optionally add a deliberate ignore/acknowledgment action. Touch finishScanTx and episode-issue owner projection. (**Size:** S).
- **Related:** B-05.11 creates these issues; B-05.30 is the separate missing path diagnosis.

### B-05.27

**Confirmed, narrower — Design risk; Medium; Size L.** ShowKey is lowercased/collapsed title plus year, without source/folder or provider identity (`apps/portico-server/server/internal/catalog/episode_repository.go:123`). Different year evidence splits and same-title/year evidence merges; filename year wins when nonzero (`apps/portico-server/server/internal/catalog/episodic.go:137`). Introduce folder/source candidates plus proven canonical identity merges, with migration of references; combine with B-05.8/B-07.3 identity work. **In Backlog?** No.

### B-05.30

**Confirmed, narrower — Defect; Medium; Size S.** fail stores a coarse error code and Get replaces most failures with generic attention text (`apps/portico-server/server/internal/ingestion/inventory.go:345,364`, `apps/portico-server/server/internal/ingestion/service.go:164`); no failing path is retained by fail. Job/source and phase are exposed, so not all context is lost. Store safe owner-only relative-path/reason diagnostics in ingestion failures and notices. **In Backlog?** No.

### B-05.31

**Confirmed, narrower — Missing feature mislabelled as a bug; Low; Size S.** Disc 1 - Studio/Disc 2 - Live do not match the anchored disc-folder regex and remain distinct album folder keys (`apps/portico-server/server/internal/catalog/audio_repository.go:92,254,414`). Many CD/Disk/Disc separators and numbers already work, so not literally only CD1/Disc 1. Extend explicit disc-layout recognition with ambiguity checks in audio_repository; metadata equality alone should not merge distinct editions. **In Backlog?** No.

### B-07.4

**Confirmed, narrower — Design risk; Medium; Size M.** Progress activity inserts/updates bump viewer revisions (`apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:3946`, `apps/portico-server/server/internal/catalog/personal_activity.go:138`); cursor/range checks reject stale revisions (`apps/portico-server/server/internal/catalog/discovery.go:161`, `apps/portico-server/server/internal/catalog/browse_engine.go:578`). Accepted progress and catalogue mutations invalidate; not literally every scan commit/no-op. Split membership/order versus decoration revisions; visible screen impact remains unmeasured. **In Backlog?** No.

### B-07.5

**Confirmed — Defect; Medium; Size S.** Search groups use compose’s read snapshot (`apps/portico-server/server/internal/httpapi/search.go:127`, `apps/portico-server/server/internal/catalog/reads.go:70`), but before/after searchRevision uses raw s.db (`apps/portico-server/server/internal/catalog/search.go:274,350`), so concurrent publication can reject even a first-page snapshot. Use s.read() for revision checks while retaining the outer authorization fence; client clears results at `apps/portico-server/packages/client-core/src/search.ts:178`. **In Backlog?** No.

### B-07.6 Some titles cannot be found by search at all — expanded check

- **Claim:** The minimum-length/punctuation tokenizer makes some titles impossible to search by their displayed name and has no abbreviation alias search.
- **What I traced:** `apps/portico-server/server/internal/catalog/search.go:114` splits punctuation and requires a token of at least two runes. `apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:326` installs unicode61 remove_diacritics 2 FTS with token-prefix indexes. `apps/portico-server/server/api/search-people.openapi.yaml:35,332` explicitly documents both minimum query tokens and token_prefix mode.
- **Verdict:** **Confirmed, narrower** — The isolated SQLite tokenizer check found MASH/SWAT do not match punctuated abbreviations; M*A*S*H terms would match the FTS document but NormalizeSearch rejects all-single-letter tokens. Single X/M/Q/Z queries are rejected. However "ac dc" finds AC/DC and "wall" finds WALL·E, so the whole list is not unsearchable at all. This is a documented query/alias capability gap, not deviation from the declared token-prefix API.
- **Class:** Missing feature mislabelled as a bug. **In Backlog?** No.
- **Severity:** Medium: literal displayed-title lookup fails for some short/abbreviated titles, affecting normal discoverability.
- **Fix direction:** Add bounded exact-title/normalized-abbreviation lookup alongside prefix search, with explicit one-rune exact-query semantics; preserve broad-prefix budgets. Touch catalog NormalizeSearch/group queries, derived search documents and API capabilities. (**Size:** M).
- **Related:** B-07.7 is a separate tokenizer substring capability. F-07.5/F-07.7 describe search scope/tokenization features; consolidate planning.

### B-07.7 Search finds Chinese, Japanese and Korean titles only from their first character — expanded check

- **Claim:** The current token-prefix search lacks within-word CJK substring matching and rejects one-character queries.
- **What I traced:** `apps/portico-server/server/internal/persistence/migrations/0001_baseline.sql:326` sets unicode61; `apps/portico-server/server/internal/catalog/search.go:119,129` splits only nonletters/numbers and creates prefix terms. `apps/portico-server/server/api/search-people.openapi.yaml:332` advertises token_prefix.
- **Verdict:** **Confirmed, narrower** — Scratch SQLite with the same tokenizer rejected 神隠し for 千と千尋の神隠し and accepted 千と. It also found 기생충 영화 from 영화, disproving the blanket claim that every Chinese/Japanese/Korean title can only be found from the entire title’s first character. Matches start at tokenizer word boundaries; unsupported infix search is a missing feature, not a broken promised substring API.
- **Class:** Missing feature mislabelled as a bug. **In Backlog?** No.
- **Severity:** Medium: native-script unspaced titles are difficult to discover by an internal phrase; spaced Korean/title words can still work.
- **Fix direction:** Add an explicit language-aware n-gram/trigram or segmented search projection with bounded indexed queries, storage cost and rebuild strategy; touch derived FTS documents, NormalizeSearch and query capability contract. (**Size:** L).
- **Related:** B-07.6 overlaps query minimum/normalization. F-07.7 is the same missing substring feature and should own it.

### B-07.8 Date filters compare raw text — expanded check

- **Claim:** Browse date comparisons accept date-only and RFC3339 inputs without converting them to the stored field’s temporal representation.
- **What I traced:** `apps/portico-server/server/internal/catalog/browse_expression.go:267` accepts both date forms and comparison at line 362 binds raw values. `apps/portico-server/server/internal/catalog/browse_contract.go:104,105,115` points releaseDate/dateAdded/lastPlayedAt to raw source text. `apps/portico-server/server/internal/catalog/discovery.go:90` stamps millisecond UTC dateAdded text.
- **Verdict:** **Confirmed, narrower** — Scratch SQL equality and <= between 2026-10-05T14:23:11.123Z and 2026-10-05 both returned false. UTC-offset instants also sort incorrectly by raw text. ReleaseDate often stores YYYY-MM-DD, so date-only equality for that field can be correct; the claim that all fields behave identically is too broad. Date/time control hints make the silent mixed-format result a real query correctness defect.
- **Class:** Defect. **In Backlog?** No.
- **Severity:** Medium: saved date filters quietly omit matching media or compare offset instants incorrectly.
- **Fix direction:** Normalize timestamp operands to UTC/numeric instants and define date-only day-range semantics per field; compile day equality/inclusive bounds appropriately. Touch browse validation/compiler and documented date field semantics. (**Size:** S).
- **Related:** B-07.12 is a separate text collation issue; no duplicate date finding found.

## Not re-checked (Low)

- **B-07.9** — The seek-by-letter request breaks on non-ASCII letters — **not re-checked**.
- **B-07.10** — The client's letter-bucket match disagrees with the server's letters — **not re-checked**.
- **B-07.12** — Case and accent handling is ASCII-only in filters — **not re-checked**.
- **B-07.14** — Migrations 0003, 0007, 0008 and 0010 do what the migration rules forbid — **not re-checked**.
- **B-07.15** — Insert-only tables grow without bound — **not re-checked**.
- **B-07.17** — Search sort quirks — **not re-checked**.
- **B-07.19** — Opening the live database in another program — **not re-checked**.

## New findings

None outside the assigned entries. More precise observations, including clean-source warning attribution under B-05.26, are kept within their existing findings to avoid duplicate planning items.

## Disagreements worth Justin's attention

- **B-05.3:** The cited strong preparation implementation is unwired, but the active original-file path independently enforces stale size/mtime. “Up to a week” is not a recovery guarantee when scans are manual or failing.
- **B-05.8 / B-12.18:** Ordinary unchanged-object moves preserve title/state. Distinct upgraded bytes under a new filename can create a separate title; state is retained on the old entity, not universally destroyed. Avoid adopting the broader rename-loss claim.
- **B-05.18 / B-05.19:** Do not assume all named mount families change identity or omit directory fingerprint changes. Both need representative backend evidence before selecting an automatic rebind/skip-policy change.
- **B-05.28:** Advertised Windows local libraries are source-blocked; remote adapters narrow the blanket “cannot add any library” wording. Treat this as an advertised-platform blocker despite no Windows runtime test.
- **B-05.29 / B-06.16:** Bounded whole-document rejection is defensive policy. Keep DOCTYPE rejection; decide encoding/field compatibility separately. The bound counts bytes and accepted credits, not necessarily characters/raw actor tags.
- **B-07.2:** Rebuild withholding is deliberate in catalogue architecture; persistence README promises the opposite. Reconcile the docs and choose an availability design; shadow projections are a substantial change, not just removal of a readiness check.
- **B-07.11:** Containers fail atomically, the permanent owner play log survives, and a movie is only recreated after actual re-enumeration, not an unchanged-directory rescan. Maintained clients use other deletion routes; the owner API exposure remains.
- **B-07.13 / B-09.30:** Merge these backup/WAL findings. The snapshot ends before quick_check. Replacing online backup with VACUUM INTO would not remove long-read snapshot pinning on its own.
- **B-05.9 / B-05.21 / B-05.22:** Renaming an unassociated issue can help; done queue rows do not rerun unchanged analysis; missing candidate guards usually launch one stat helper, not stat plus blob. Keep the real costs/accepted-wrong-interpretation problem without overstating them.
- **B-07.6 / B-07.7:** Minimum query length and token-prefix semantics are explicit API choices. All-single-letter titles/abbreviation aliases and CJK infix lookup remain capability gaps. Spaced later words and longer component tokens already work.

Severity changes/standardizations are visible per row: B-05.6 High → Medium; B-05.7 Medium–High → Medium; B-05.18 High → Medium; B-05.19 Medium–High → Medium; B-05.20 Medium–High → High; B-05.32 Low–Medium → Medium; B-07.11 Low–Medium → Medium; B-07.16 and B-07.18 Low–Medium → Low. Tier 2 compound labels become B-05.17/B-05.31 Low, B-05.26/B-05.30/B-07.8 Medium. Other High/Medium ratings are retained; B-05.28 remains Critical with firmer supported-platform evidence.

## Evidence boundary

Reviewed the working trees on `ehler-homelab-mini-1` on 11 October 2026. Read the verification brief and worklist in full; read the original brief, domain entries, catalogue architecture, scanner implementation contract, relevant product contracts/decisions and Backlog. Context-sync was attempted and failed because its lock file is outside the writable sandbox; existing shared project/verification guidance was read. No guidance refresh was claimed.

No Portico source/state was modified. Only this deliverable and disposable files under `/tmp/v04-verification` were written. No builds, Portico test-suite runs, dependency installs, formatters, server executions, endpoint requests, browser/computer-use/MCP actions or subagents were used. Existing tests were read as intended-behavior evidence, never claimed as passing.

Isolated experiments, all standard-library-only and outside checkouts:

1. Copied `catalog/episodic.go` in full and the exact `yearPattern`/`FilenameTitle` block from `catalog/catalog.go` into `/tmp/v04-verification/naming.go`; changed only package/import scaffolding and added sample inputs. Ran `go run /tmp/v04-verification/naming.go` with `GOCACHE`, `GOPATH`, `GOMODCACHE` under that scratch directory and `GOENV=off GOTOOLCHAIN=local`. It reproduced wrong movie years, flat-folder and punctuation mismatch, resolution-as-episode, daily-show conflict, Part N rejection, x-range shortening and NFC/NFD mismatch. It proves the copied parsers, not the running scanner.
2. Python `-I` with standard `sqlite3`: copied the exact inventory_directories CREATE TABLE and visited UNIQUE INDEX into an in-memory database, then inserted two empty-identity paths and assigned the same identity by UPDATE. The second UPDATE raised the expected UNIQUE constraint error. Foreign-key enforcement was not needed for this isolated index check; no entire Portico schema/database was opened.
3. Python `-I` with standard `sqlite3`: constructed a small standalone FTS5 table using the source's exact `unicode61 remove_diacritics 2` tokenizer and `prefix='2 3 4'` configuration. Queried dotted abbreviations, AC/DC, WALL·E, Japanese unspaced title substrings/prefixes and a later Korean spaced word. Also evaluated raw timestamp/date-only equality and <=, both false. This proves local SQLite configuration behavior, not Portico search routing or its vendored SQLite binary.
4. A small Go `encoding/xml` decoder experiment in `/tmp/v04-verification/xml.go` used the same default CharsetReader absence as ParseVideoNFO. UTF-8 reached EOF; declared US-ASCII and ISO-8859-1 failed. No full NFO service or parser publication path was executed.

Performance, mount identity stability, filesystem timestamp fidelity, Windows acceptance, disk exhaustion, viewer-visible cursor recovery and natural occurrence of poisoned derivations remain runtime boundaries. Concrete cheapest experiments are recorded with the relevant findings. No coverage omissions: **24/24 Tier 1, 21/21 Tier 2; seven Low entries deliberately not re-checked.**
