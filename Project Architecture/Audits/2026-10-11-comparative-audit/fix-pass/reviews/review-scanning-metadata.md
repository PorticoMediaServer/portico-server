# Independent review — scanning-metadata

Review completed 12 October 2026. Worktree remained read-only; base `82aaa54`, lane `audit-fixes/scanning-metadata`.

Execution host: `ehler-homelab-mini-1`. Context refresh was attempted but could not acquire its lock because the shared context state is read-only in this sandbox; existing guidance is being read directly. No worktree changes, external services, browser, subagents, or dependency downloads.

## Evidence and review method

Read the reviewer brief and fix brief, the lane report, both audit domains and their V04/V05 verification entries, the scanning/metadata contracts, relevant architecture text and D-15/D-21. Inspected `git show` for all twelve commits and their production callers. All Go commands use `GOCACHE=/tmp/gocache-codex GOPROXY=off`. Additional adversarial Go probes use a `go test -overlay` whose source and JSON are held in Linux anonymous memory files (`memfd`); no source or overlay file is written into the worktree or filesystem. They call the actual HEAD parsers and exact base-commit parser functions side by side.

Result: **6 ACCEPT, 2 ACCEPT WITH CHANGES, 4 REJECT.** Four blocker findings are listed below; the scan verdict also requires Justin to resolve the scan-pace part of D-21. Existing suites pass, but three additional regression probes fail on HEAD.

## Commit reviews

Paths below are relative to the lane worktree. Line numbers refer to HEAD. Test commands run from `server/`; every command has the prefix `GOCACHE=/tmp/gocache-codex GOPROXY=off`.

### 056a306 — Continue a library scan as soon as each step advances instead of waiting on a fixed poll  (bug B-05.1)

- Verdict: REJECT
- What it does: Adds an immediate continuation signal after a successful quantum changes the durable scan position. Refused, failed, cancelled and unchanged quanta do not generate that signal.
- Findings:
  1. **Blocker — `server/internal/ingestion/service.go:389`, `:457`; `server/internal/dbwork/pressure.go:77`.** Continuation is immediate under foreground pressure as well as when idle. The retained `Yield` does **not** yield CPU or time: it increments a counter and checks cancellation. Production admission (`workpolicy.Allowed`) always allows work. The existing `dbwork.PaceBackground` is the pressure-responsive mechanism used by other continuous bulk loops, but this loop never calls it. The writer gate still prioritizes foreground writes, and storage/decoder children still have bounded pools and low OS priority; these do not pace the parent process's SQL, catalog publication and scheduling work competing with browsing reads. Add cancellation-aware pressure pacing after the quantum releases its transactions and before waking continuation, retaining guaranteed background progress. Add a production-loop foreground-pressure regression test. Weak-machine browsing/playback safety is **unverified**, not a demonstrated runtime failure.
- Tests run: `go test ./internal/ingestion/... ./internal/worker/... ./internal/dbwork/... -count=1` passed in the combined suite. A verbose targeted rerun passed `TestScanLoopContinuesAsSoonAsAQuantumAdvances` (3.92 s), `TestScanLoopDoesNotRespinAQuantumThatDidNotAdvance`, and `TestQuantumReportsWhetherTheRunAdvanced`. Vet and formatting passed.
- Residual risk: Hardware contention needs measurement with active playback and browsing. Test on a constrained CPU with a large scan, compare foreground latency and playback starts against an idle scan, and verify scans continue progressing under sustained pressure. The lane's hardware note is useful, but cannot replace a pressure-path test.

**Priority assessment:** The indexed before/after comparison deliberately excludes `last_turn`; no self-wake comes merely from a bookkeeping write. With no job, the worker waits for a shared wake/schedule/safety tick; with unchanged work, shared wakes remain floored. I found no new empty-work busy loop. Per-source exclusion, four-source cap, fairness, cancellation and background write class remain intact. The pre-existing `analysis.go:195` playback-busy path still defers same-source analysis; this patch does not establish the stronger “never paused” principle for that path. The accidental *polling* throughput floor is real, but this implementation also chooses storage-limited scan speed under load. D-21 explicitly asks “fixed pace” versus “as fast as the storage and provider allow” for **scans and providers**. It is therefore **not merely a neutral timer repair**: it implements the fast-scan portion of D-21 without the owner choice or adequate foreground-pressure pacing. Hold this commit pending that decision and the blocker fix; provider request pace itself is unchanged.

### f77e524 — Stop reading frame sizes and codec tags as season and episode numbers  (bug B-05.12)

- Verdict: ACCEPT WITH CHANGES
- What it does: Filters frame-shaped and channel-layout/codec-shaped matches before deciding whether a filename has conflicting episode markers. Common lowercase resolution suffixes now leave real episode markers usable.
- Findings:
  1. **Should fix — `server/internal/catalog/episodic.go:38-48`.** The detector is case-insensitive but the filters are case-sensitive. `Show.S01E02.1920X1080.mkv` and `Show.S01E02.DD5.1X264.mkv` still return `conflicting_episode_markers`. Normalize matched text or make both filters case-insensitive; add uppercase fixtures.
  2. **Should fix — `server/internal/catalog/episodic.go:45`, `:128`; `server/internal/catalog/catalog.go:366`.** Dropping a resolution from the marker set leaves it in the title prefix. Actual HEAD result for `Show - 1920x1080 - S01E02.mkv` is title `Show -`, year **1920**, season 1/episode 2. Under `Show/Season 1/` it fails `folder_and_filename_series_conflict`. Exclude recognized resolution tags from the series-name evidence as well as episode markers, preserving the original input for diagnostics; test a frame size before the genuine marker.
  3. **Should fix — `server/internal/catalog/episodic.go:38`.** Any pair of three/four-digit numbers is deemed a frame size, including previously valid `Show.101x102.mkv`. The parser explicitly supports seasons up to four digits. This changes accepted naming grammar beyond recognizing resolutions. Use recognized dimension pairs or contextual resolution evidence, and retain an accepted long-number fixture for the `x` syntax as well as `S…E…`.
- Tests run: Full `go test ./internal/catalog/... -count=1` passed; added lane fixtures pass. In-memory `go test -overlay <memory-overlay> ./internal/catalog -run TestReviewerParsers -count=1 -v` exercised all requested names. `Show.1x02.720p` gives Show/1/2; `Show 4x12 1280x720` gives Show/4/12; the middle requested name produces the bad title/year above. Uppercase and long-number cases confirmed the additional gaps. Vet/formatting passed.
- Residual risk: Previously misfiled files are not repaired automatically. No device/service dependency for the parser fixes.

### 6c61342 — Keep a year that belongs to a title when a bracketed or adjacent release year follows it  (bugs B-05.7, B-06.1)

- Verdict: ACCEPT WITH CHANGES
- What it does: Prefers explicit bracketed or adjacent release years over an earlier title number in both parsers, and stops reparsing a catalog title's internal number as conflicting release evidence when the catalog already supplies the release year. This reaches production ingestion and screen matching.
- Findings:
  1. **Should fix — `server/internal/catalog/catalog.go:371`; `server/internal/metadata/screen_match.go:52`, `:103`.** Bracket precedence is applied across the whole name, including after known release/encoding tags. `Movie.2019.1080p.[2020].mkv` was title `Movie`, release 2019 in the matcher; now it is title `Movie 2019`, release 2020. The catalog likewise changes from `Movie`, 2019 to `Movie 2019 1080p`, 2020. A late encode/group date can now override earlier release evidence and pull release tags into the title. Bound bracket preference to the title/release region before technical suffixes, and add such a two-year suffix fixture in both parsers. Do not describe arbitrary later bracketed years as unambiguous without this restriction.
- Tests run: Full `go test ./internal/catalog/... ./internal/metadata/... -count=1` passed. In-memory overlays compared the actual HEAD functions against exact `82aaa54` functions; the requested examples had no newly incorrect result. Vet/formatting passed.
- Residual risk: Real provider searches were not performed. Existing catalog titles remain unchanged until an appropriate repair/reimport. Bare numeric titles retain their old matcher failure; the patch fixes a subset of the reported defect.

Parser evidence (filename extensions omitted here for readability):

| Input | Catalog HEAD title / year | Matcher HEAD title / year | Change from base |
|---|---|---|---|
| `2001 A Space Odyssey (1968)` | `2001 A Space Odyssey` / 1968 | same / 1968 | Catalog already correct; matcher fixed |
| `1917 (2019)` | `1917` / 2019 | same / 2019 | Catalog already correct; matcher fixed |
| `Blade Runner 2049 (2017)` | `Blade Runner 2049` / 2017 | same / 2017 | Both fixed |
| `2012 (2009)` | `2012` / 2009 | same / 2009 | Catalog already correct; matcher fixed |
| `Movie.1080p.2019.x264` | `Movie 1080p` / 2019 | `Movie` / 2019 | Both unchanged; catalog retains pre-existing technical text |
| `Show (2010) - S01E01` | `Show` / 2010 | same / 2010 | Both unchanged |
| `2019` | `2019` / 0 | empty / 2019 | Both unchanged; numeric-title limitation remains |
| `Movie.2019.1080p.[2020]` | `Movie 2019 1080p` / 2020 | `Movie 2019` / 2020 | New regression when the final year is suffix evidence |

### a7f1ea0 — Name the folder a stopped scan was reading in the administrator's notice  (bug B-05.30)

- Verdict: ACCEPT
- What it does: Carries a source-relative folder through scan failure classification and adds it to the administrator's notice when safely displayable. Stored job error codes and their equality-based consumers are preserved.
- Findings: None requiring changes. `stopError.Unwrap` preserves `errors.Is/As`; the wrappers cover inventory reading, verification and absence checks. `NotifyScanFailure` uses the existing bounded text validator and omits unusable names without aborting the failure transaction. The audience remains admins, and no absolute source root is added. Root failures intentionally retain the generic notice. This follows the existing producer catalog's server-generated notice wording.
- Tests run: `go test ./internal/ingestion/... ./internal/operations/... -count=1` passed. Verbose targeted runs confirmed `TestStoppedScanTellsTheOwnerWhereItStopped` actually ran (not skipped), and `TestScanFailureNoticeNamesOnlyADisplayableFolder` passed. Vet/formatting passed.
- Residual risk: Native/web rendering of long but valid folder names and a managed-remote failure were not exercised. Job-list detail is still generic; completing that part would need an agreed storage representation rather than putting text into the code field.

### 9abc772 — Treat a series folder and file name as equal when they differ only in Unicode form  (bug B-05.32)

- Verdict: ACCEPT
- What it does: Applies NFC normalization before the existing case-insensitive folder/file series comparison. Display text and durable identity keys are untouched.
- Findings: None requiring changes. Canonically equivalent forms compare equal, while `Cafe` versus `Café` remains a conflict. This does not introduce accent folding or merge unrelated series. The dependency already existed.
- Tests run: `go test ./internal/catalog/... -count=1` passed, including `TestShowFolderAndFileNameAgreeAcrossUnicodeForms`. Vet/formatting passed.
- Residual risk: Physical Mac/Linux filename interoperability was not exercised. Show/artist/album/book identity-key normalization remains unfixed and is discussed under skipped items.

### 9cf8362 — Stop re-queuing provider artwork for a slot that is already held  (bug B-06.3)

- Verdict: REJECT
- What it does: Resolves the public entity ID to its integer catalog ID before the occupied-slot query. Newly seeded automatic provider jobs now respect current local/locked selections and pending owner/local jobs.
- Findings:
  1. **Blocker — `server/internal/metadata/artwork_integration.go:77`; `server/internal/metadata/artwork_worker.go:493`, `:529`.** Correct seeding leaves previously queued automatic provider jobs untouched. A provider job queued by the old broken seeder *after* an unlocked local selection has the same selection revision and source fence, so the worker still installs it over that local selection after upgrade. I reproduced this with the exact parent-commit seeder and the lane's fixture: one inherited provider job remained; HEAD reseeding followed by `ArtworkStep` replaced `local-poster` and made a second image request. Add a publication-side local/owner precedence check for automatic provider jobs (respect explicit owner selections), and preferably discard obsolete jobs before acquisition too. This is also the publication guard requested by V05's fix direction.
- Tests run: Full metadata suite and `TestProviderArtworkIsNotRequeuedForAnOccupiedSlot` passed. In-memory `go test -overlay <memory-overlay> ./internal/metadata -run 'TestReviewer(Locked|OldQueued)' -count=1 -v`: locked-local variant **passed**; inherited-provider-job variant **failed**, replacing the local selection. Existing unlocked-local fixture passed. Vet/formatting passed.
- Residual risk: Locked and unlocked local selections are protected against **new** automatic seeding; only the locked case is safely protected from this inherited-job path. Real image service behavior was not used (bounded fake HTTP transport). Existing installed selections need not be discarded to fix this.

### fc6337c — Give a season folder's images to the season instead of the show and recognise seasonNN-poster files  (bug B-06.5)

- Verdict: ACCEPT
- What it does: Separates a recognized season directory from its show parent for show artwork and adds common `seasonNN-poster` candidates. A season folder's poster/folder image can fill its season's poster slot.
- Findings: None requiring changes. The folder expression matches the catalog rule, including Specials. `dirs[1:]` and `dirs[:1]` are safe because the current directory is always included; parents remain contained below the source root. Flat show folders preserve their previous behavior. The change does not grant any new filesystem authority.
- Tests run: `go test ./internal/localmetadata/... ./internal/ingestion/... -count=1` passed. Verbose `TestSeasonFolderImagesAreNotTheShows` passed for distinct season/show artwork, parent `season01-poster`, Specials and flat layouts. Vet/formatting passed.
- Residual risk: Already misattributed images are not removed, and unchanged completed media may not be reanalyzed on a normal rescan. Correcting old stored artwork requires a separate repair; this commit is correct for newly analyzed files.

### 153776d — Carry the studio and network kinds of TheTVDB companies so they are published  (bug B-06.6)

- Verdict: ACCEPT
- What it does: Preserves typed company relationships from TheTVDB's grouped movie and typed-series shapes, enabling the existing studio/network publisher. Unknown/distributor/effects categories retain generic company behavior.
- Findings: None requiring changes. The 256-company bound remains, positive IDs are still required, and normal record validation/publication applies. Publisher mapping of `studio`/`production_company` to studios and `network` to networks was traced and is covered by existing metadata tests.
- Tests run: `go test ./internal/metadataprovider/... ./internal/metadata/... -count=1` passed. Verbose `TestScreenTVDBCompaniesKeepTheirKind` passed for both response shapes. Vet/formatting passed.
- Residual risk: Fixtures are authored from an asserted schema, not captured live evidence. The lane correctly labels this untested on live TheTVDB. Its cold-test note gives credentials/environment, match steps, output fields, filter check and response fields to inspect on failure; it is adequate. No live endpoint or external documentation was fetched during this review.

### cc6110c — Retry titles parked by a provider outage after a cool-off and stop cooldown waits using up retries  (bug B-06.12)

- Verdict: REJECT
- What it does: Lets idle screen-worker passes recover parked rows with a due timestamp, leaves new terminal failures permanently parked, and stops retained provider cooldown waits consuming attempts. Newly parked retryable failures get at least an hour between recovery attempts, with exponential growth.
- Findings:
  1. **Blocker — `server/internal/metadata/screen_worker.go:39`.** A non-empty old `next_attempt` is not evidence of a retryable error. Before this patch, `failScreen` stored a timestamp for **every** unavailable row, including 401/403/404, malformed/validation failures and exhausted local publication failures. The new query reclaims all of them. A fake 404 row stored exactly as the old implementation would store it made **one provider request** on HEAD before being parked again. Thus the requirement “cannot retry non-retryable failures” is not met across upgrade. Persist and require explicit recovery eligibility for new rows using existing storage, and treat old ambiguous terminal codes conservatively. If old HTTP status was not stored, do not infer it from `request_rejected`, which covers retryable and terminal statuses alike. Add pre-upgrade fixtures for 401/403/404, validation and exhausted local publication failures, alongside the retryable outage fixture.
- Tests run: Full `go test ./internal/metadata/... ./internal/metadataprovider/... -count=1` passed; all four new recovery tests passed in a verbose targeted run. In-memory `go test -overlay <memory-overlay> ./internal/metadata -run TestReviewerPreviouslyTerminalFailure -count=1 -v` **failed**: calls=1, attempts increased from 1 to 2, then retry time cleared. Vet/formatting passed.
- Residual risk: No real outage was induced. A large parked backlog can use the provider's entire existing allowance continuously; sparse per-title retries are not an aggregate daily recovery budget. This is bounded by the existing request pace, not proof that the volume is suitable for a particular provider.

**Priority assessment:** For newly classified failures, `Retryable()` only admits 429, 5xx and `unavailable`; terminal errors and exhausted local publication failures receive an empty retry time. Recovery is claimed only after ordinary due work. Both acquisition paths still call `screenBeforeRequest`, and the provider transport still uses its shared one-second request interval, serialized pacing acquisition, cancellation and retained Retry-After. The change introduces no unbounded rapid retry loop or bypass of provider cooldown. The exponential exponent is capped at 10, so its actual long-term delay is **30,720 seconds (8 h 32 min)**, about 2.8 attempts/day/title, indefinitely—not a 24-hour eventual delay. The 24-hour bound only takes effect for longer Retry-After. Provider-wide cooldown remains durable, including across restarts. Indefinite sparse recovery is a recovery-policy choice worth recording; it does not itself alter D-21's per-provider pace. The upgrade classification blocker must be fixed regardless of the chosen lifetime.

### 148ad48 — Run an owner's metadata search ahead of the automatic matching backlog  (bug B-06.13)

- Verdict: ACCEPT
- What it does: Adds `searching` as the first claim-order class ahead of show fan-out and automatic title work, retaining the existing due-time and lease conditions.
- Findings: None requiring changes for the identified due-search ordering defect. Search state comes from authorized owner repair commands; cooldown, enabled-provider checks, lease/input fences and provider request pacing remain in force. The CASE-sort cost predates this commit; this is an extra rank, not a new unindexed whole-library read.
- Tests run: Full metadata suite and verbose `TestOwnerSearchIsClaimedAheadOfTheBacklog` passed. Vet/formatting passed.
- Residual risk: This provides strict owner-search priority, not a quota against an owner continuously submitting distinct searches. A failing/stale/cooldown-deferred search becomes `pending` under existing failure paths and then loses the new priority; the query override remains stored. That limitation predates this change and should be addressed if priority must survive cooldown. No interactive UI import test was performed.

### 0049b5c — Reuse a fresh episode document when a show fans out instead of asking the provider for every episode  (bug B-06.24)

- Verdict: ACCEPT
- What it does: Applies the existing freshness window and durable refresh budget to TMDB episodes using the stored candidate document. Cached evidence is used only when its parent identity, order, season, episode and requested language still answer the current question.
- Findings: None requiring changes. The candidate read is bounded to one target; stored records are validated; changed parent/coordinates/language do not reuse the old document. First acquisition is unbudgeted, held stale evidence is preserved when refresh budget is exhausted, and publication still runs through lease/generation/input/identity fences and field locks. The request and document-timestamp writes occur outside the publication transaction, following the established film/show design.
- Tests run: Full metadata suite passed; verbose `TestAddingAnEpisodeDoesNotRefetchTheOthers` and `TestHeldEpisodeDocumentIsRefetchedWhenItNoLongerAnswers` passed. Additional in-memory `go test -overlay <memory-overlay> ./internal/metadata -run TestReviewerEpisodeKeyAndBudget -count=1 -v` **passed**, rejecting cache reuse independently after parent ID, order, season, number or language changes, and reusing stale held evidence with both budgets exhausted. Vet/formatting passed.
- Residual risk: Cached reuse is TMDB-only; AniList still follows its existing per-parent-request path, and TheTVDB remains delegated. All episodes still receive local worker turns/publications after fan-out, so a very long show can still delay automatic metadata work. Explicit episode Retry now also follows freshness rather than forcing a request; a separate force-refresh intent remains missing. The official-order fixture does not establish live episode-group behavior. These limits should remain visible rather than expanding this narrowly scoped fix into fan-out/provider-pacing policy.

### 7d7fe54 — Skip sidecar lookups for files the scan already saw are not in the folder  (bug B-05.22)

- Verdict: REJECT
- What it does: Builds a bounded per-job directory-name cache from committed local listings and attaches it to analysis contexts so absent image/NFO/OPF/JSON candidates skip storage stat/read helpers. Unknown or incomplete listings retain the original read path.
- Findings:
  1. **Blocker — `server/internal/ingestion/sidecar_listing.go:119-124`; `server/internal/ingestion/analysis.go:313`, `:397`; `server/internal/catalog/inventory.go:337`.** A completed listing is trusted without checking its directory revision at analysis time. A sidecar added after inventory/verification is skipped, but the media's local-metadata operation is then marked complete. A later ordinary scan sees the new sidecar yet does not queue unchanged media for analysis, because eligibility depends on media/policy revision, not directory/sidecar change. Reproduced with a real synthetic MP4: add its poster immediately before Basic analysis; with the negative cache, local poster count is 0 after the scan **and after a second ordinary scan**; with the cache disabled, count is 1 after both scans. This is a persistent missed local selection, not a harmless one-scan delay. V04 explicitly requested revision invalidation without hiding added sidecars. Validate each cached directory revision once per analysis batch/turn (including parent artwork directories), fall back to unknown on change, and/or reliably invalidate affected local-metadata completion for deferred evidence. Add the late-sidecar-and-next-scan regression test. Do not just change the report's wording.
- Tests run: Full ingestion/localmetadata suites and their new listing tests passed. In-memory `go test -overlay <memory-overlay> ./internal/ingestion -run TestReviewerLateSidecar -count=1 -v` **failed** only with the negative cache enabled; the comparison subtest disabling `scanner.listings` before analysis passed. Used the same qualified FFmpeg/storage-helper setup as existing ingestion discovery tests. Vet/formatting passed.
- Residual risk: No helper-start reduction measurement or Mac/Windows retained-stream run was performed. The lane's process-count measurement note is adequate for validating savings but would not catch the late-sidecar defect.

**Cache lifetime assessment:** Negatives are keyed by job ID and are not persisted; a new job or process cannot consult an earlier job's negatives. Partial/resumed/remote/analysis-only/unlisted directories fall back safely. A paused job keeps the same cache and can resume much later with stale negatives. A completed job's map also remains in memory until a later scan first records a page and prunes ended jobs; the added test even consults it after completion. Thus stale negative *data* can outlive completion in memory, but is not reused across new scan IDs. The lasting missed-artwork state above outlives scans independently of cache eviction. Bounds are per job (100,000 directories/400,000 names), not a global service cap; many paused scans can accumulate multiple maps. None of this changes root containment/read authorization: the optimization only skips reads, and admitted reads still use the existing guard.

## Skipped items and partial fixes

- **B-05.32 durable identity keys:** The skip is justified. NFC changes to `catalog_identities.source_key`/local keys can collide with existing equivalent identities and change which rows subsequent scans find. Preserving watch/owner state requires data reconciliation, which the no-migration brief forbids. The equality-only fix is safely separable and useful.
- **Bare numeric filenames in B-05.7/B-06.1:** Leaving existing behavior intact avoids introducing a guess in this limited patch, but citing D-15 as the complete reason is too broad. Preserving `1917` as search text can be separated from relaxing automatic acceptance: the owner could still receive candidates under the existing two-signal rule. A follow-up can safely investigate that narrower correction without deciding best-result-wins. No new regression in the requested year-only case was found here.
- **Job-list failure detail in B-05.30:** Keeping error codes stable rather than embedding arbitrary detail is justified under the no-schema-change rule. The administrator notice provides useful path context now.
- **B-06.24 local fan-out/other providers:** Limiting the commit to TMDB episode acquisition is a coherent partial correction. Unnecessary repeated local publication can also be investigated as duplicate work; it does not inherently require choosing a faster provider pace under D-21. Correctly deciding whether parent policy/selection changed is essential before suppressing fan-out. AniList parent requests and missing explicit force-refresh remain outside the implemented fix.
- **B-05.22 remote/unlisted/restarted cases:** Safe fallbacks are justified; caching helper failures would conflate absence and transient I/O. Deferring a newly added sidecar indefinitely is **not** justified by the Backlog's “per-scan” cache wording, as demonstrated above.

## Cross-commit issues

1. `f77e524` and `6c61342` interact when a resolution appears before an episode marker: ignoring the frame as an episode does not prevent `1920` becoming a release year. Fix that evidence path together; NFC normalization does not repair an already corrupted prefix.
2. `9cf8362` and `0049b5c` correctly remove repeated provider/still work for newly seeded, held selections. Inherited unsafe provider-artwork jobs must be made harmless before claiming local selections are protected after upgrade.
3. `fc6337c` corrects fresh season artwork attribution, but `7d7fe54` can suppress an owner's newly added local poster and mark the operation complete. Existing incorrect art and skipped unchanged-media analysis also limit the impact of a simple rescan.
4. `cc6110c` recovers only when no normal work is due. A continual new-item/fan-out backlog can delay recovery indefinitely. `148ad48` protects currently due owner searches but does not preserve their priority after cooldown/stale/failure demotion. No change here bypasses provider pacing.
5. Source changes are confined to the listed Go paths. No migration, dependency file, credentials, native/shared-package copy or provider-host allowlist changes were found. Commit subjects are plain behavior sentences, authorship is Justin's, and commit bodies have no generated/coauthor trailers. Production goroutine starts still go through `supervise.Go`; bare goroutines in the new cadence tests are test-only and the discipline guard passes.

Sibling-lane changed-file lists were compared against `82aaa54`: catalogue-playback, hardware-conversion, hosted-routes, identity-ops-live, platform-packaging, social-downloads and web-client-core have **no changed-file overlap** with this lane. No textual merge conflict is predicted from those lists; combined behavioral tests are still needed after integration.

## Verification record

The following full suite passed on HEAD:

```sh
GOCACHE=/tmp/gocache-codex GOPROXY=off go test \
  ./internal/ingestion/... ./internal/worker/... ./internal/catalog/... \
  ./internal/metadata/... ./internal/metadataprovider/... ./internal/localmetadata/... \
  ./internal/notify/... ./internal/operations/... ./internal/persistence/... \
  ./internal/supervise/... ./internal/dbwork/... -count=1
```

`internal/notify` has no test files. Catalog took 139.7 seconds; ingestion 13.0 seconds; metadata 40.6 seconds; persistence 39.9 seconds; dbwork 41.7 seconds. All completed successfully. A separate `GOCACHE=/tmp/gocache-codex GOPROXY=off go test ./internal/storage/... -count=1` passed (6.9 seconds), covering the production storage priority/admission consumers.

`go vet` with the same environment and all packages in the full suite completed successfully. `git diff --name-only 82aaa54..HEAD -- '*.go' | xargs gofmt -l` and `git diff --check 82aaa54..HEAD` produced no output. Worktree `git status --porcelain=v1` was empty before and after testing. The lane's broad build and unrelated HTTP checks were read as its evidence, not represented as independent runs here.

Verbose targeted runs confirmed the newly added non-parser tests executed and passed, including the permission-sensitive failure notice test. Parser tests executed in the full catalog/metadata suites. Additional memory-overlay probes called actual production functions:

| Probe / command tail (after `go test -overlay <memory-overlay>`) | Result |
|---|---|
| `./internal/catalog ./internal/metadata -run TestReviewer -count=1 -v` | Parser comparisons completed; old-terminal 404 assertion failed |
| `./internal/ingestion -run TestReviewerLateSidecar -count=1 -v` | Cache-disabled comparison passed; cached next-scan assertion failed |
| `./internal/metadata -run 'TestReviewer(Locked\|OldQueued)' -count=1 -v` | Locked-local case passed; old queued provider replacement assertion failed |
| `./internal/metadata -run TestReviewerEpisodeKeyAndBudget -count=1 -v` | Passed |

`<memory-overlay>` was a JSON mapping to anonymous Linux memory descriptors generated by a Python driver. No test file was installed in the worktree. For a cold reproduction in an authorized implementation worktree:

- **Terminal retry:** Start from `outageFixture`; set its fake provider to `Error{Provider:"tmdb", Status:404, Code:"request_rejected"}`; seed the work row as `unavailable`, error `request_rejected`, attempts 1, `next_attempt` one hour before the injected clock. Call `ScreenStep`; require zero provider calls. HEAD makes one.
- **Inherited artwork:** Start from `TestProviderArtworkIsNotRequeuedForAnOccupiedSlot` immediately after installing `local-poster` with `locked=0`. Inside a gated transaction, get `acceptedScreenArtwork` and `artworkFence`; call an exact copy of `seedScreenArtwork` from `9cf8362^`. This queues one provider job with the existing local selection's revision. Run HEAD `seedArtwork` and `ArtworkStep`; require the local candidate unchanged and no second HTTP request. HEAD replaces it.
- **Late sidecar:** Use the qualified FFmpeg/supervised storage setup from `TestRealMovieScanDiscoveryPaginationAndCollections`, with one synthetic `Movie (2020).mp4`. Drive `processQuantum` until `basic_pending`, then write a valid `Movie (2020)-poster.png`. Complete the scan and a second ordinary queued scan without changing media/policy. Require `catalog_item_details.poster_url LIKE 'local:%'`. HEAD finds zero. Repeat with `scanner.listings=nil` immediately before first analysis to isolate the cache change; it finds one.
- **Episode reuse:** Settle `episodeFreshnessFixture`. Build a `screenBase` from one stored episode candidate's target/language/coordinates; `rememberedEpisodeRecord` must reject independent changes to parent ID/order/season/episode/language. Advance 40 days, exhaust both `metadata_refresh_budget` windows at the injected clock, and call `screenEpisodeRecord`; require the held identity and zero extra fake requests. HEAD passes.

Synthetic decoder tests logged that bubblewrap cannot create a namespace in this sandbox and used the existing qualified fallback. This does not invalidate the parser/cache/queue assertions, but it is not decoder-sandbox acceptance. No live Portico data, hosted endpoint, demo, provider API, browser or physical device was used.

## Summary

| Commit | Bug | Verdict | Blockers |
|---|---|---|---|
| 056a306 | B-05.1 | REJECT | Immediate scan continuation lacks pressure pacing; D-21 scan choice must be resolved |
| f77e524 | B-05.12 | ACCEPT WITH CHANGES | None; uppercase, prefix-resolution and long-number grammar fixes required |
| 6c61342 | B-05.7 / B-06.1 | ACCEPT WITH CHANGES | None; later suffix-year override must be bounded |
| a7f1ea0 | B-05.30 | ACCEPT | None |
| 9abc772 | B-05.32 | ACCEPT | None |
| 9cf8362 | B-06.3 | REJECT | Previously queued automatic provider job can replace unlocked local art |
| fc6337c | B-06.5 | ACCEPT | None |
| 153776d | B-06.6 | ACCEPT | None |
| cc6110c | B-06.12 | REJECT | Previously terminal errors are automatically retried after upgrade |
| 148ad48 | B-06.13 | ACCEPT | None |
| 0049b5c | B-06.24 | ACCEPT | None |
| 7d7fe54 | B-05.22 | REJECT | Late local sidecars can remain missing after the next ordinary scan |

Justin's required decision: resolve D-21's scan pace before accepting immediate continuation. Separately record whether indefinite sparse provider recovery (about 2.8 tries/day/title after backoff saturates) is the intended lifetime; neither approval can substitute for terminal-error classification or foreground-pressure protection. The remaining fixes have concrete code/test corrections and do not require a new product choice.
