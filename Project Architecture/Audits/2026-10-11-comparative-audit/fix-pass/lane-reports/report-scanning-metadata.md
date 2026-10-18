# Lane report: scanning and metadata (Go server)

Worktree: /home/justin/projects/Portico/.worktrees/audit-fixes/server-scanning-metadata (branch audit-fixes/scanning-metadata, base 82aaa54)

### B-05.1 — Scan speed ceiling of about three steps per second per folder source
- Status: FIXED
- What I found: Real. `Service.Run` (server/internal/ingestion/service.go) drove the loop through `worker.Run`, whose only wake was the shared commit signal; `worker.RunWith` floors a shared wake at `min(step's wait, MinSignalInterval)` and the step asked for 300 ms while anything was in flight, so each source got one bounded quantum per 300 ms no matter how quickly the quantum finished. `dbwork.Yield` is documented as "never an admission gate", and nothing in the architecture documents or INVENTORY_CONTRACT.md describes a deliberate pace, so the floor was accidental.
- Change: the loop now runs through `worker.RunWith` with a second, urgent signal that a quantum goroutine wakes only when `processQuantum` reports that the run durably advanced (phase, cursors, counters or the head of its pending directory/analysis queue changed — read by one indexed query before and after the step) and the job is still running. Every other outcome (admission refused, source busy, paused/cancelled, failure, playback wait, a remote page still pending, a deep-analysis wait, a panic) reports false and falls back to the old 300 ms poll, so no path can spin. Admission, the four-source cap, one quantum per source, `last_turn` fairness, the background write class and `dbwork.Yield` are untouched. The 300 ms literal became the package variable `quantumPoll` so a test can prove the loop no longer depends on it.
- Commit: 056a306 "Continue a library scan as soon as each step advances instead of waiting on a fixed poll"
- Tests: server/internal/ingestion/cadence_test.go — `TestScanLoopContinuesAsSoonAsAQuantumAdvances` (production `Run` loop with the poll and the shared floor set to an hour: 41 directories scanned to completion; failed before the fix after 20 s with the job still in `inventory`, passes in about 1 s), `TestScanLoopDoesNotRespinAQuantumThatDidNotAdvance` (a refused quantum is started exactly once in 500 ms), `TestQuantumReportsWhetherTheRunAdvanced`. Ran `go test ./internal/ingestion/... ./internal/worker/... -count=1` (ok), `go test ./internal/persistence/ -run 'Statement|Scan|Plan'` (ok), `go test ./internal/supervise/` (ok), `go test ./cmd/server/ -run 'Media|Default'` (ok), `go vet`, `gofmt -l` clean.
- Not verified: real throughput on a large library and the effect on a one-core machine under playback; no measurement was taken, only the removal of the timer. Scanning is now bounded by disk, CPU and the write gate, which is what `dbwork/pressure.go` says is intended. Test note: on the target hardware, start a full scan of a library with a few thousand folders and time the `inventory` phase (Jobs list, or `inventory_runs.phase` / `inventory_directories` counts); before this change it could not beat about 3.3 folders a second per source. Pass: the rate is limited by the disk, and while the scan runs, browsing and starting playback stay responsive (compare request latency with the scan paused). Fail: the UI stalls or playback control lags while a scan runs — that would mean the write-gate priority alone is not enough on that machine.
- Review first: the final report files scan pace under decision D-21 together with provider pace. The coordinator's brief puts B-05.1 in scope and D-21's provider pace out of it; this change touches only the scan loop's own wake and no provider code. If D-21 is meant to cover scan pace as an owner choice too, this commit is the one to hold back.
- Docs to update: none found that state the 300 ms pace. (INVENTORY_CONTRACT.md says pages hold "at most 128 entries" while local pages request 32 — pre-existing, not touched.)

### B-05.12 — Resolution tags such as `1920x1080` are read as season × episode
- Status: FIXED
- What I found: Real. `seasonalToken` (server/internal/catalog/episodic.go:22) accepts any `N…xN…` run with no shape rule, so `[Group] Show - 05 [1920x1080].mkv` was filed as season 1920 episode 1080 with show title "Show - 05 [", and a real `S01E01` next to `1920x1080` or `DD5.1x264` was rejected as `conflicting_episode_markers`. Reproduced by the new test before the change.
- Change: `ParseEpisode` now takes its markers from `episodeMarkers`, which drops two shapes that cannot be a season-by-episode marker: both sides three or four digits (a frame size) and `Nx264`/`Nx265` written directly after a decimal point (`5.1x264`, the codec against a channel layout). Everything else the pattern matched still matches (`1x264`, `12x103`, `S2019E1100` are pinned by tests). I did not add a left boundary to the `s…e…` form (`Shows1e1.mkv`): names such as `TheShowS01E01.mkv` rely on today's behaviour and that is a strictness change, not the resolution defect.
- Commit: f77e524 "Stop reading frame sizes and codec tags as season and episode numbers"
- Tests: server/internal/catalog/episodic_test.go `TestFrameSizeAndCodecTagsAreNotEpisodeMarkers` (failed on six cases before, passes after). `go test ./internal/catalog/ -count=1` ok (143 s), `go vet ./internal/catalog/` clean, `gofmt -l` clean.
- Not verified: nothing device-dependent. Files already catalogued under a wrong season keep that filing until their folder is rescanned in a way that re-parses them.
- Docs to update: none found.

### B-05.7 and B-06.1 — A year inside a title is taken as the release year / produces an empty query
- Status: FIXED (the unambiguous cases only; one commit — the same first-year-wins rule in two parsers, plus a matcher-only consequence)
- What I found: Real, and it is one defect in two places. The scanner's `FilenameTitle` (server/internal/catalog/catalog.go:354-365) cut at the first year after a separator, so `Blade Runner 2049 (2017).mkv` became "Blade Runner", 2049. The matcher's `screenFilename` (server/internal/metadata/screen_match.go:55-58) did the same and also accepts a year at position zero, so `1917 (2019).mkv` produced an empty search string; and because the matcher re-parses the catalogue's own title, `1917` + catalogue year 2019 was additionally flagged `local_release_years_conflict` (the new test showed queries `[]` and a conflict before the change). Fixing only the scanner would have turned "wrong title" into "right title, forced manual match", so both had to change together.
- Change: each parser keeps its own token grammar and first-year default, and yields to a later year only when the name is unambiguous: (1) a year in brackets `(YYYY)`/`[YYYY]` is the release year; (2) a year written directly after another (`Blade.Runner.2049.2017`) is the release year; (3) matcher only: a year that opens the name yields to any later year, since cutting there leaves no title. In `screenQueries`/`screenLocalYearConflict` the catalogue title is no longer stripped of a year-like number that differs from the release year the catalogue already holds. Deliberately unchanged: a bare `1917.mkv` (matcher still produces no query), `Blade Runner 2049.mkv`, `Cyberpunk 2077 Edgerunners`, `Space 1999`, and years apart from each other such as `Film.1999.Remastered.2019` (still 1999) — all pinned by tests. I did not add a trailing-boundary rule for `My Movie 20131209` / `Movie.Title.2048x1080`: it changes single-year names and is not needed for the cases above.
- Commit: 6c61342 "Keep a year that belongs to a title when a bracketed or adjacent release year follows it"
- Tests: server/internal/catalog/filename_title_test.go `TestFilenameTitleKeepsAYearThatBelongsToTheTitle`, `TestShowFolderKeepsAYearThatBelongsToTheTitle`; server/internal/metadata/screen_match_test.go `TestScreenNamesKeepAYearThatBelongsToTheTitle`. All failed before for the stated reason. `go test ./internal/catalog/ -count=1` ok, `go test ./internal/metadata/ -count=1` ok, `go vet` and `gofmt -l` clean.
- Not verified: a live provider search for these titles (provider fakes only). Titles already catalogued with the wrong title/year keep them: a movie entity is titled once when its file is first linked (`commitMovieItem`), so existing rows need a rematch or re-import; I did not add a repair pass.
- Docs to update: none found describing the year rule.
- Remaining (not fixed, needs a decision): bare numeric titles (`1917.mkv`, `2012.mkv`) still produce no search; that is title-versus-year guessing and sits with matching strictness (D-15).

### B-05.30 — A stopped scan does not say where it stopped
- Status: FIXED (in the administrator's notice; the job row's text is unchanged because that needs storage)
- What I found: Real. `fail()` (server/internal/ingestion/inventory.go:345) stores one of four codes in `jobs.error` and raises `notify.NotifyScanFailure` with the code only; the phase functions know the directory (`inventoryPage`'s `relative`, `verifyPage`'s `d.path`, `reconcilePage`'s parent folder) and returned a bare error.
- Change: the three phase functions wrap their failure in a small `stopError` that carries the folder relative to the source root (it unwraps and prints exactly as the cause, so classification, `errors.Is` checks and the stored code are unchanged). `fail()` passes that folder to `NotifyScanFailure`, which adds `It stopped at "<folder>".` to the body and a `path` argument. Safety: the notice goes only to the account-admin inbox (existing audience isolation), carries a path relative to the source and never the absolute root, and the folder is included only if it passes the notice text validator (no control characters or bidirectional overrides, at most 300 bytes) and is a single line; otherwise it is omitted so an awkward folder name cannot make the notice — and with it the failure transaction — fail. `jobs.error` still holds only the code: three readers compare it by equality (`ingestion.Get`, `catalog.adminJobAction`, `diagnostics/report.go`) and there is no other column for detail, so the Jobs list still shows its generic sentence. The support report still contains codes only.
- Commit: a7f1ea0 "Name the folder a stopped scan was reading in the administrator's notice"
- Tests: server/internal/ingestion/scan_failure_notice_test.go `TestStoppedScanTellsTheOwnerWhereItStopped` (an unreadable sub-folder; failed before with the old body, passes after, and asserts the absolute root is absent); server/internal/operations/notifications_test.go `TestScanFailureNoticeNamesOnlyADisplayableFolder` (newline, control character, bidirectional override, over-long and empty names are left out and the notice is still raised). `go test ./internal/operations/ ./internal/ingestion/ -count=1` ok; `go vet`, `gofmt -l` clean; `go build ./...` ok.
- Not verified: how the web and native notification lists render the longer body (server text is shown as written today; no client change was made). Remote-source failures inside `readDirectoryPage` are covered by the same wrap but were not exercised with a remote fake.
- Docs to update: none found that quote the notice text.

### B-05.32 — Accented names written in two Unicode forms may not be recognised as equal
- Status: FIXED in part (the folder-versus-file comparison); the identity-key half is SKIPPED
- What I found: Real, and in two separable places. (a) `ParseEpisode` compared the season folder's series name with the file's using `strings.EqualFold` with no normalisation (server/internal/catalog/episodic.go, the block that raises `folder_and_filename_series_conflict`), so `Café/Season 1/Café - S01E01.mkv` with one name composed and the other decomposed was rejected — reproduced by the new test. (b) Show, artist, album and book identity keys are the lower-cased, space-collapsed raw name (`episode_repository.go:123`, `audio_repository.go:112` `localKey`), so the same name in two forms is two identities.
- Change: (a) only. `sameSeriesName` normalises both names to NFC before the case-insensitive comparison (`golang.org/x/text/unicode/norm`, already a dependency of the module). A genuinely different name (`Cafe` vs `Café`) is still a conflict.
- Commit: 9abc772 "Treat a series folder and file name as equal when they differ only in Unicode form"
- Tests: server/internal/catalog/filename_title_test.go `TestShowFolderAndFileNameAgreeAcrossUnicodeForms` (failed before with `folder_and_filename_series_conflict`). Targeted catalogue tests ok; full `go test ./internal/catalog/` result is in the closing section.
- Not verified: a real macOS-written folder on a Linux file system (the strings were built in the test, which settles the comparison itself).
- Docs to update: none found.
- Reason (SKIPPED part b): normalising the keys changes the stored identity of every name already catalogued in decomposed form. Those keys live in `catalog_identities.source_key` and the `local_key` columns (with unique indexes and a trigger), and a library that already holds such a name would get a second show/artist on the next scan and lose the watch state attached to the first unless the stored keys are rewritten in the same step. That is a data migration, which this pass excludes. Until then, files of one show spelled in both forms still become two shows (as they already do in a flat folder layout).

### B-06.3 — The "is this slot already taken?" check for provider posters never matches
- Status: FIXED
- What I found: Real. `seedScreenArtwork` (server/internal/metadata/artwork_integration.go:73) bound `t.ID`, the 22-character public id, against the INTEGER `entity_id` columns of `artwork_selections` and `artwork_jobs`, so the check was always false; the sibling check a few lines later in `artwork_discovery.go:111` binds the resolved integer. The new test showed a poster already held under the current state queued again on a second seeding.
- Change: resolve the entity once with the existing `artworkEntity` helper and bind the integer in both branches. Nothing else: the check's own conditions (locked, local, or chosen under the current state; an owner or local job in flight) are as written. I did not add the second reviewer's extra publication-side precedence check in `artwork_worker.go` — with the guard working, the provider job that used to overwrite a local selection is no longer queued, and a second check is a design addition.
- Commit: 9cf8362 "Stop re-queuing provider artwork for a slot that is already held"
- Tests: server/internal/metadata/artwork_reseed_test.go `TestProviderArtworkIsNotRequeuedForAnOccupiedSlot` — a held provider poster is not queued or downloaded again on reseed, and a local selection made under an earlier state is neither queued over nor replaced (failed before at the first assertion). `go test ./internal/metadata/ -count=1` ok; `go vet`, `gofmt -l` clean.
- Not verified: real provider image downloads (fake HTTP transport).
- Docs to update: none found.

### B-06.5 — A season folder's `poster.jpg` becomes the show's poster; `seasonNN-poster.jpg` is not recognised
- Status: FIXED
- What I found: Real. `ReadSidecars` (server/internal/localmetadata/sidecars.go:38-75) searched the episode's own folder first and its parent second for the show's poster, backdrop and logo, so `Show/Season 01/poster.jpg` was recorded as the show poster (the new test showed all three roles taken from the season folder), and the season-poster names were only `seasonN`/`seasonNN` `.jpg|.png` plus `season-specials-poster.jpg`.
- Change: when the episode's folder is a season folder by the catalogue's own rule (`Season 1`, `season_02`, `Specials`), show images come from the parent folder only; the season folder's `poster.jpg|png` / `folder.jpg|png` is that season's poster (used after the explicit season names); and `seasonNN-poster.jpg|png` is added to the season names. No new storage or role was needed: the scan already carries `season/N/poster` and `commitAssetSidecars` already writes it as the season's local poster. A season folder's `fanart.jpg` / `logo.png` is now ignored (there is no season backdrop or logo role) instead of being given to the show. A show whose episodes sit directly in its folder is unchanged.
- Commit: fc6337c "Give a season folder's images to the season instead of the show and recognise seasonNN-poster files"
- Tests: server/internal/localmetadata/sidecars_test.go `TestSeasonFolderImagesAreNotTheShows` (failed before; covers season folder only, season plus show folder, `season01-poster.jpg`, `Specials/folder.jpg`, flat layout). `go test ./internal/localmetadata/ -count=1` ok; `go vet`, `gofmt -l` clean.
- Not verified: end-to-end through a real scan and the artwork worker (the reader is unit-tested with a fake file reader; the commit path it feeds already existed and has its own tests).
- Docs to update: `Specs/Spec — Page Content by Media Type.md` line 10 lists the honoured local file names (`season01.jpg`); it could mention `season01-poster.jpg` and the season-folder rule.
- Note: a show that already recorded a season folder's image as its local show poster keeps that row; the code states that a sidecar's removal is not detected (not on my list).

### B-06.6 — TheTVDB titles get no studio and no network
- Status: FIXED — UNTESTED ON a live TheTVDB account (fixtures only)
- What I found: Real. The TheTVDB adapter flattened both company shapes into a list of names and emitted every one as relation kind `company` (server/internal/metadataprovider/tvdb_screen.go, the `v.Companies` loop); the provider-neutral publisher `screenCompanyFacts` (server/internal/metadata/screen_publish.go:391) only turns `production_company`, `studio` and `network` into the studio and network facts. So a TheTVDB-matched title could never have either.
- Change: the adapter keeps each company's kind. A film's companies arrive grouped (`studio`, `network`, `production`, `distributor`, `special_effects`): the group gives the kind. A show's arrive as one list with `companyType.companyTypeName` on each: "Network", "Studio" and "Production Company" map to the three kinds. Distributors, effects houses and companies with no type stay `company`, as before, so nothing new is published for them. The publisher is untouched (its handling of the three kinds is already covered by `screen_facts_test.go`).
- Commit: 153776d "Carry the studio and network kinds of TheTVDB companies so they are published"
- Tests: server/internal/metadataprovider/screen_test.go `TestScreenTVDBCompaniesKeepTheirKind` (both shapes; failed before with every kind `company`). `go test ./internal/metadataprovider/ -count=1` ok; `go vet`, `gofmt -l` clean.
- Not verified / test note for a live check: the show shape (`companyType.companyTypeName` with the names "Network", "Studio", "Production Company") is written from TheTVDB v4's published schema, not from a captured response. Environment: a server with a TheTVDB key and a TV library whose agent uses TheTVDB. Steps: match a show known to have a network and a production company (any current network series), let the screen worker publish it, then open the show's metadata editor or `GET` its page. Pass: the Network and Studio fields are filled and the show appears under that network/studio filter. Fail: both empty — in that case capture `/series/{id}/extended` and compare the `companies` entries with the two field names above (TheTVDB also returns `originalNetwork`/`latestNetwork`, which this change does not read).
- Docs to update: none found.

### B-06.12 — A provider or internet outage of about a quarter of an hour parks titles permanently
- Status: FIXED
- What I found: Real. `failScreen` (server/internal/metadata/screen_worker.go:197-228) parked a row as `unavailable` on its sixth failure whatever the cause, `claimScreen` (:32) never claims `unavailable`, and nothing else re-arms it short of a file/settings change or the owner's retry. Separately, `screenBeforeRequest` returns the provider's standing cooldown as a 429 `provider_backoff` error, which went through the same path and counted as an attempt (the new test showed one attempt used per cooldown wait).
- Change (no schema change; uses the existing `status`, `attempts`, `next_attempt` columns):
  1. A cooldown wait (`provider_backoff`) defers the row to the end of the cooldown, keeps it `pending` and counts no attempt.
  2. Parking is unchanged (still `unavailable` after five tries, still immediate for a non-retryable provider error), but a row parked for a *retryable provider* error (network failure, 429, 5xx) keeps a retry time: the existing doubling delay with a one-hour floor (`screenRecoveryCoolOff`) and the existing 24-hour ceiling. Each further failure counts an attempt, so the wait keeps growing: about 1 h, 1 h, 64 min, 2 h, 4 h, then 8.5 h between tries.
  3. `claimScreen` takes such a row only when nothing else is due (a second, indexed query on `status='unavailable' AND next_attempt>''`), so recovery never delays new titles or owner work.
  4. The bounded-retry protection is kept for what it was written for: a local publication failure that has used its five tries, a non-retryable provider refusal and a missing entity/scope are parked with an empty retry time and are never claimed again on their own. The two parks in `claimScreen` now clear `next_attempt` explicitly so they cannot be picked up by the recovery query.
  The provider pace is untouched: there is still one worker step per pass and the provider adapters' own interval.
- Commit: cc6110c "Retry titles parked by a provider outage after a cool-off and stop cooldown waits using up retries"
- Tests: server/internal/metadata/screen_recovery_test.go — `TestTitleParkedByAnOutageIsRetriedAfterACoolOff`, `TestParkedTitleBacksOffFurtherWhileTheOutageLasts`, `TestTitleParkedByAFinalRefusalStaysParked`, `TestWaitingOutAProviderCooldownCostsNoAttempts` (all four failed before). `go test ./internal/metadata/ -count=1` ok; `go vet`, `gofmt -l` clean.
- Not verified: a real outage against a real provider (fake provider and injected clock only). The worker loop is woken by commits or its five-minute safety tick, so a recovery try can start up to five minutes after its retry time on an otherwise idle server.
- Review first: (a) retryable failures now retry indefinitely, at most about three times a day per title once the delay reaches its ceiling — say if you want a final stop after N recoveries; (b) rows parked *before* this change all carry an old retry time, so each gets one recovery try (one provider request at most) the first time the worker is idle; rows whose cause was final are then parked with no retry time.
- Docs to update: none found that describe the retry policy.

### B-06.13 — An owner's Fix Match search waits behind the background backlog
- Status: FIXED
- What I found: Real. The single claim query in `claimScreen` (server/internal/metadata/screen_worker.go:32) ordered due rows as fan-out shows, shows, items, then retry time and id; `status='searching'` — written only by the owner's search (`screen_repair.go:157`, `screen_review.go:601`) — had no rank of its own. The new test queued a search on the fifth of several pending films and the worker claimed the oldest pending film instead.
- Change: the ordering's first key puts `searching` rows first; the rest of the order is as it was. It is bounded by construction: a search is one explicit owner request, and a search that fails, goes stale or meets a provider cooldown is rewritten to `pending` by the existing failure paths, so it cannot keep the front of the queue. The provider cooldown check and pace still apply to the search itself.
- Commit: 148ad48 "Run an owner's metadata search ahead of the automatic matching backlog"
- Tests: server/internal/metadata/screen_search_priority_test.go `TestOwnerSearchIsClaimedAheadOfTheBacklog` (failed before). `go test ./internal/metadata/ -count=1` ok; `go vet`, `gofmt -l` clean.
- Not verified: the wait as the owner sees it in the web editor during a real import (the editor polls; the server now claims the search on its next pass, which is still at most one step per pass and behind a step already in flight).
- Docs to update: none found.

### B-06.24 — Adding one episode re-fetches every episode of the show; the freshness rules do not cover episodes
- Status: FIXED (the provider requests; the fan-out itself is unchanged — see "Remaining")
- What I found: Real. A woken show republishes and goes to `pending_children`; `dispatchScreenChildren` (server/internal/metadata/screen_worker.go) resets every non-owner episode to `pending`; and `screenEpisodeStep` (server/internal/metadata/screen_publish.go:984) called `ScreenEpisode` directly, bypassing the freshness window, refresh budget and remembered-document path that `screenRefreshRecord` gives films and shows. `freshnessEpisode` (30 days) was defined and unused. The new test reproduced it: with four matched episodes, adding a fifth cost five episode requests.
- Change: `screenEpisodeStep` now gets a TMDB episode's document from `screenEpisodeRecord` (in screen_freshness.go beside the film/show rule it mirrors). If the row still holds the document it last published **for the same show id, order, season, episode number and language**, and that document is inside its window (`freshnessEpisode`, shortened by the existing recently-added rule), it is republished without asking. Past the window it is refreshed only while the provider's existing refresh budget allows, otherwise the held document is used. An episode never fetched is always asked for, unbudgeted, exactly as for films. A re-identified show, a changed episode order, a renumbered file or a new metadata language does not match the held document, so those still fetch. No new storage: it reads `screen_metadata_candidates` (already written on every publication) and `metadata_document_freshness` under kind `episode`.
- Commit: 0049b5c "Reuse a fresh episode document when a show fans out instead of asking the provider for every episode"
- Tests: server/internal/metadata/screen_episode_freshness_test.go — `TestAddingAnEpisodeDoesNotRefetchTheOthers` (one new episode now costs one request; held episodes keep their published titles and end `matched`), `TestHeldEpisodeDocumentIsRefetchedWhenItNoLongerAnswers` (no requests inside the window; all refreshed past it; all refetched in the new language after a language change). Both failed before. `go test ./internal/metadata/ -count=1` ok; `go vet`, `gofmt -l` clean.
- Not verified: against live TMDB, and with TMDB episode-group orders (the group path makes three requests per episode and is skipped the same way, but the fake only models the official order).
- Remaining (not changed): the show still resets every episode and each is still republished locally, one worker step per pass, so a 900-episode show still occupies the single worker for about 900 passes after one new file — without provider traffic, and (with B-06.3 fixed) without re-downloading stills. Skipping the reset for an unchanged parent means changing the fan-out and the worker's pace (B-06.11, D-21). Also unchanged: AniList episodes still call `ScreenDetails` for the parent work per episode (a different path, not named in the claim), and there is no owner "refresh" for a show or episode that forgets an episode's freshness (`QueueRefreshTx` is films only), so an owner who wants an episode re-read inside its window has no direct way to ask.
- Docs to update: none found; the header of screen_freshness.go already names the fan-out as a case this rule covers.

### B-05.22 — Looking for sidecars that are not there costs dozens of process starts per file
- Status: FIXED (for local folder sources, in scans that list the directory)
- What I found: Real, as the second review narrowed it. For each analysed file `ReadSidecars` tries up to about 28 image names, `VideoNFO` up to three, `MovieArtwork` four and the audio paths four to nine more (server/internal/localmetadata/sidecars.go, video.go, audio.go, audio_evidence_ingestion.go). Each goes through the storage client, whose read guard starts a helper process to stat the path before the read (server/internal/ingestion/analysis.go `readContext`, storage/client.go `ReadSmall`); hits were cached, misses were not. A missing file and a transient storage failure are indistinguishable at that boundary (the helper's error is untyped), so I did **not** cache failed reads: a single I/O hiccup would have hidden a real poster for the rest of the scan.
- Change: the scan already lists every directory through the same guarded helper before analysing anything in it. `noteDirectoryPage` (new file server/internal/ingestion/sidecar_listing.go) remembers, per scan job and per directory, the sidecar-like names (`.jpg .png .nfo .opf .json`) that listing returned; analysis passes `localmetadata.WithListing` on its read context; and each of the five lookup sites skips a name the scan knows is not there. It is a negative cache per scan per directory, as Backlog.md asks, built from positive evidence. Rules that keep it safe: only a listing this process saw from first page to last counts; everything else is "not known" and storage is asked exactly as before (a directory an incremental scan skipped as unchanged, a listing resumed after a restart, a remote source, an analysis-only job, an untracked extension, anything past the bounds of 100,000 directories or 400,000 names per scan); names are compared case-insensitively and in one Unicode form so a file system that would open the file is never told it is absent; the listing can only skip a read, never add one or change what a successful read does. Memory is bounded and a scan's listing is dropped when another scan starts and that job is no longer queued, running or paused.
- Commit: 7d7fe54 "Skip sidecar lookups for files the scan already saw are not in the folder"
- Tests: server/internal/ingestion/sidecar_listing_test.go `TestScanRemembersWhichSidecarsADirectoryHolds` (a real inventory pass: present whatever the letter case, absent when listed, unknown for untracked kinds, unlisted directories and other jobs), `TestPartialDirectoryListingIsNotTrusted`; server/internal/localmetadata/sidecars_test.go `TestSidecarsNotListedByTheScanAreNotRead` (one read instead of about thirty when the directory is known; unchanged when it is not). The existing real-scan tests with the real storage helper and ffmpeg still find their sidecars end to end (`TestRealMovieScanDiscoveryPaginationAndCollections` uses a `-poster.png`, `TestRealLocalAudioScanChaptersArtAndBookResume` a `metadata.opf`). `go test ./internal/ingestion/ ./internal/localmetadata/ -count=1` ok; `go vet`, `gofmt -l` clean.
- Not verified: the saving itself was not measured (no count of helper starts in a real import), and nothing was run on macOS or Windows, where listing goes through the retained-stream path — this change does not call storage differently there, it only reads the pages the scan already receives. Test note for a measurement: on a Linux server, first-import a TV library of a few hundred episodes with no artwork files and compare the count of `--portico-storage-helper` process starts during the analysis phase before and after (for example with `strace -f -e trace=execve -c` on the server process, or `perf trace`); pass is roughly thirty fewer starts per episode.
- Behaviour change to know about: a sidecar added to a directory *after* this scan listed it is no longer picked up by files analysed later in the same scan; it is picked up when a later scan lists the directory and analyses a file there. The Backlog item's "per-scan" wording accepts this.
- Docs to update: Backlog.md line 23 ("Missing sidecars aren't cached") — done for local sources; what remains is remote sources, incremental scans of unchanged directories and analysis-only jobs, which still probe every name.


## Commits (git log --oneline 82aaa54..HEAD, oldest last)

```
7d7fe54 Skip sidecar lookups for files the scan already saw are not in the folder
0049b5c Reuse a fresh episode document when a show fans out instead of asking the provider for every episode
148ad48 Run an owner's metadata search ahead of the automatic matching backlog
cc6110c Retry titles parked by a provider outage after a cool-off and stop cooldown waits using up retries
153776d Carry the studio and network kinds of TheTVDB companies so they are published
fc6337c Give a season folder's images to the season instead of the show and recognise seasonNN-poster files
9cf8362 Stop re-queuing provider artwork for a slot that is already held
9abc772 Treat a series folder and file name as equal when they differ only in Unicode form
a7f1ea0 Name the folder a stopped scan was reading in the administrator's notice
6c61342 Keep a year that belongs to a title when a bracketed or adjacent release year follows it
f77e524 Stop reading frame sizes and codec tags as season and episode numbers
056a306 Continue a library scan as soon as each step advances instead of waiting on a fixed poll
```

## Final verification

All from `/home/justin/projects/Portico/.worktrees/audit-fixes/server-scanning-metadata/server`, default test tier, after the last commit:

- `gofmt -l` on every changed Go file: no output.
- `go build ./...`: ok.
- `go vet ./internal/ingestion/ ./internal/catalog/ ./internal/metadata/ ./internal/metadataprovider/ ./internal/localmetadata/ ./internal/notify/ ./internal/operations/`: clean.
- `go test ./internal/ingestion/... ./internal/worker/... ./internal/catalog/ ./internal/metadata/... ./internal/metadataprovider/... ./internal/localmetadata/... ./internal/notify/... ./internal/operations/... ./internal/persistence/... ./internal/supervise/... ./cmd/server/... -count=1`: all `ok` (notify has no test files). This includes the statement-plan guards in `persistence` and the goroutine discipline guard in `supervise`.
- Wider, for packages that consume what changed: `go test ./internal/httpapi/... ./internal/scanevents/... ./internal/telemetry/... ./internal/diagnostics/... ./internal/administration/... ./internal/storage/... -count=1`: all `ok` except one test in `internal/httpapi`, `TestListeningHTTPPreferenceEnvelopeAndCAS`, which stops at `listening_test.go:21: explicit reviewed helper binary required` because the environment variable `PORTICO_PLAYBACK_TEST_HELPER` is not set on this machine (`playback_fixture_test.go:43-46`). It fails the same way without my changes (no file under `internal/httpapi` is touched) and is not related to this lane.

No schema change, no new dependency (`golang.org/x/text/unicode/norm` was already used by `sorttext` and `metadata`), no new goroutine, no change to provider pace, matching strictness, folder-versus-file authority or ignore rules.

## Summary

| ID | Status | Commit |
|---|---|---|
| B-05.1 | FIXED | 056a306 |
| B-05.12 | FIXED | f77e524 |
| B-05.7 + B-06.1 | FIXED (unambiguous cases; one commit) | 6c61342 |
| B-05.30 | FIXED (in the admin notice) | a7f1ea0 |
| B-05.32 | FIXED in part (comparison); identity keys SKIPPED (needs a data migration) | 9abc772 |
| B-05.22 | FIXED (local sources, listed directories) | 7d7fe54 |
| B-06.3 | FIXED | 9cf8362 |
| B-06.5 | FIXED | fc6337c |
| B-06.6 | FIXED — UNTESTED ON live TheTVDB | 153776d |
| B-06.12 | FIXED | cc6110c |
| B-06.13 | FIXED | 148ad48 |
| B-06.24 | FIXED (provider requests; fan-out unchanged) | 0049b5c |

## Noticed outside my list (not changed)

- `server/internal/ingestion/INVENTORY_CONTRACT.md` says inventory pages hold "at most 128 entries"; local pages are requested 32 at a time (`inventory.go`, `Limit: 32`). The helper's maximum is 128, so the document describes the bound, not the practice.
- `verifyPage` (`ingestion/inventory.go`): when the verify cursor is exhausted but some directory of the job is not `done`, the phase update is skipped by its `NOT EXISTS` guard and the function returns nil, so the job would sit in `verifying` forever. I could not construct the state; with the new continuation rule it idles at the old poll and does not spin.
- A catalogue title is set once, when a file is first linked (`commitMovieItem`, `EnsureEntityTx` keeps existing facts). Titles already catalogued wrongly by the old year and marker rules are not repaired by a rescan.
- `jobs.error` holds only a code and three readers compare it by equality, so the Jobs list cannot show where a scan stopped without a column for detail; only the notice does now.
- `QueueRefreshTx` (`metadata/details.go`) accepts films only (`kind != 1` is refused), so there is no owner action that forgets the freshness of a show's or an episode's provider document.
- `failScreen` rewrites any failing row to `pending`, including an owner's `searching` row that only met a provider cooldown; the search still runs later but no longer as a search-status row.
- The screen claim query orders with a `CASE` expression over every due row, so each claim sorts the whole due backlog (index `screen_metadata_due` gives the range, not the order). That is cost per pass during a large import, separate from provider pace.
- AniList episodes call `ScreenDetails` for the parent work once per episode on every fan-out (`screen_publish.go`, the anilist branch); the freshness rule added for TMDB episodes does not cover it.
- `storage/inventory_page_stream.go` starts `expireInventoryStreams` with a bare `go`; the discipline test passes, so it is presumably exempted there, but it is the one unsupervised start I came across.
- `sidecars.go`/`local_artwork.go`: a removed sidecar is never noticed (stated in the code); B-06.5's old misattributed show posters therefore stay until replaced.

---

# Follow-up after independent review (12 October 2026)

Six follow-up commits on the same branch, none amending an earlier one. Each finding was first reproduced with a test in this worktree; all six reproduced, so none was rejected.

### Review of 056a306 (B-05.1) — scan continued without pacing against foreground pressure
- Verified: true. `dbwork.Yield` only counts and checks cancellation (`dbwork/pressure.go:77`); the pressure-responsive step is `dbwork.PaceBackground`, used by the three other continuous bulk loops (`compactcatalog/derive.go:187`, `lyrics/bulk.go:168`, `catalog/bulk_jobs.go:409`) and never called by the scan loop. New test showed 0 pauses for 24 folders with a foreground probe held true.
- Change: after a step that advanced, the quantum goroutine calls `dbwork.PaceBackground(work, started)` before it releases the source and wakes the loop. Idle: next step at once. Foreground in flight: a pause of three times the step's duration, at least 2 ms and at most 50 ms, then the step runs — it always progresses. Cancel/pause interrupts the pause. No new constant and no setting: it is the existing rule, unchanged. Write class (`ClassBackgroundMedia`), admission, four-source cap, per-source exclusion and the no-progress fallback to the 300 ms poll are as before.
- Commit: 3901993 "Make a continuing library scan give way to foreground work between steps"
- Test: `cadence_test.go` `TestScanLoopGivesWayToForegroundWorkBetweenSteps` (production `Run` loop; every advancing step is followed by a pacing call, no step starts within 2 ms of one, and the scan still completes under constant pressure). Ran 9 times clean.
- D-21: I did not revert. The floor could be removed without inventing a pace because Portico already has a rule for exactly this kind of loop and I applied it as it stands. What remains true: with nothing in the foreground a scan is now limited by storage, which is D-21's "as fast as the storage allows" option for the idle case, without an owner choice. If Justin wants scan pace to be his decision in the idle case too, hold 056a306 and 3901993 together; nothing else in the lane depends on them. UNTESTED ON a constrained machine with real playback and browsing (test note under B-05.1 above still applies).

### Review of f77e524 (B-05.12) — three should-fix items
- Verified: all three. `Show.S01E02.1920X1080.mkv` and `…DD5.1X264…` were still conflicts; `Show - 1920x1080 - S01E02.mkv` gave title "Show -", year 1920 (and a series conflict under `Show/Season 1/`); `Show.101x102.mkv` had become unrecognised.
- Change: the two filters are case-insensitive; a pair of long numbers is a frame size only when the first is a width video is encoded at and the second can be its height (240 or more, between 3:1 wide and 1:2.4 tall), so `101x102`, `2019x100`, `720x101` are markers again while cropped (`1920x804`) and portrait (`1080x1920`) frames are recognised; and the recognised tags are taken out of the text the series name and year are read from, in both the season and the air-date forms.
- Commit: 9bcab2a "Recognise frame sizes by shape in any letter case and keep them out of the series name"
- Test: `episodic_test.go` `TestFrameSizeTagsInAnyCaseAndPositionLeaveTheEpisodeReadable` — the reviewer's names plus the folder variant, a year-then-frame prefix, an air-date name, and the long-number fixtures (8 of 13 cases failed before).

### Review of 6c61342 (B-05.7 / B-06.1) — a late bracketed year displaced the release year
- Verified: `Movie.2019.1080p.[2020].mkv` gave "Movie 2019 1080p"/2020 in the catalogue and "Movie 2019"/2020 in the matcher.
- Change: bracket preference across the whole name is gone. In both parsers the first year is the release year unless another year is written **directly after it**, bare or bracketed (`Blade Runner 2049 (2017)`, `Blade.Runner.2049.2017`); a bracketed year is never displaced; the matcher's rule for a year that opens the name is unchanged. A year further along changes nothing.
- Commit: aeaefd5 "Let only a directly following year displace an earlier one as the release year"
- Test: added to `filename_title_test.go` and `screen_match_test.go`: the reviewer's name, `Movie (2019) 1080p [2020]`, `Movie (1999) (2019)`, `2001 A Space Odyssey (1968) 1080p [2020]`, `Blade Runner 2049 [2017] 1080p`.
- Difference from the reviewer's suggestion: I bounded by adjacency, not by "before the first technical tag", because the catalogue parser has no list of technical tags and adding one is a larger change. The cost: `Blade Runner 2049 Directors Cut (2017).mkv` is back to the base result ("Blade Runner", 2049). Pinned in the test so it is visible.

### Review of 9cf8362 (B-06.3) — a provider job already in the queue could still replace local art
- Verified: queued the provider image for a slot holding an unlocked local selection, as a seeder would write it; `ArtworkStep` downloaded it and replaced the selection.
- Change: `artworkSlotHeld` in `artwork_worker.go`, checked twice — when the job is claimed (so nothing is downloaded) and again inside the publication transaction (in case the slot was taken during the download). An automatic provider job (no owner actor, not local, not a preview) is retired as `stale` / `slot_held` when the slot's selection is locked or local. This is the rule all five seeders already apply before queueing; an image the owner chose is not automatic and is still installed (tested).
- Commit: f1e4f04 "Retire a queued automatic provider image when its slot holds local or locked artwork"
- Test: `artwork_reseed_test.go` `TestQueuedProviderArtworkGivesWayToALocalOrLockedSelection` (local and locked; both failed before with a second download).

### Review of cc6110c (B-06.12) — failures that were terminal before were retried after upgrade
- Verified: the base `failScreen` wrote a retry time on every park, so `next_attempt>''` selected every old parked row. Seven pre-upgrade fixtures (401/403/404 as `request_rejected`, exhausted `request_rejected`, `invalid_response`, `response_too_large`, `publication_failed`, `identity_mismatch`, `metadata_scope_unavailable`) each made provider requests.
- Change: recovery now needs two marks: a retry time **and** a recorded reason that only an outage writes — `unavailable` (network failure), `provider_unavailable`, `provider_backoff`. When a retryable 429/5xx parks a title its reason is recorded as `provider_unavailable` instead of the status-blind `request_rejected` (the web editor already has a label for that code, `MetadataEditor.tsx:445`). Old rows are read conservatively: `request_rejected` covers both final and retryable statuses and the status was not kept, so those rows stay parked until the owner retries or the title changes.
- Commit: c951cab "Recover a parked title only when its recorded reason is an outage, not on a retry time alone"
- Tests: `screen_recovery_test.go` `TestTitlesParkedByAnEarlierBuildRecoverOnlyFromAnOutage` (nine old-row fixtures: two recover, seven make zero requests over three passes and three days), `TestOnlyAnOutageLeavesAParkedTitleARetryTime` (401, 403, 404, malformed response, exhausted local failure stay final; network failure, 503 and 429 recover).
- Unchanged: provider request pace, cooldown table and Retry-After handling.
- Retry schedule, per title: see the coordinator reply; in short 30 s, 1, 2, 4, 8 min; then parked; if the cause was an outage, 1 h, 1 h, 1 h 4 min, 2 h 8 min, 4 h 16 min, then every 8 h 32 min with no end, only when the worker has nothing else due.

### Review of 7d7fe54 (B-05.22) — a late sidecar stayed missing
- Verified with the reviewer's scenario (real MP4, poster written after the folder was listed and before analysis): no local poster after the scan.
- Change: a listing now stands only while the directory is as it was listed. Each listed directory keeps its identity and revision; before a listing is used for a file, the directory is read again with the same call the scan's own verification uses (`InventoryDirectory`), once per analysed file per directory; if it differs, or cannot be read, that directory is forgotten and every name is asked for as before. A listing with no revision is not kept. A scan's listing is also dropped when the scan completes, fails or is cancelled. Cost: one helper start per directory consulted per file (normally two) in place of about thirty.
- Commit: ea56bd6 "Trust a scan's folder listing for sidecars only while the folder is unchanged"
- Tests: `late_sidecar_test.go` `TestSidecarAddedAfterItsFolderWasListedIsStillRead` (found by the same scan and still there after a second ordinary scan; failed before); `sidecar_listing_test.go` extended (a changed folder is not trusted while its unchanged neighbour is; a listing without a revision is not kept; a cancelled scan's listing is dropped).
- Not changed, and not caused by the cache: a sidecar added after its media file's analysis has completed is not read by a later ordinary scan, because unchanged media is not re-analysed. That is true with the cache disabled as well. Residual: the check relies on the directory's modification evidence, as the incremental scan does; a file system that does not move a folder's timestamp when a file is added would hide a late sidecar from both.

## Commits after review

```
3901993 Make a continuing library scan give way to foreground work between steps
ea56bd6 Trust a scan's folder listing for sidecars only while the folder is unchanged
c951cab Recover a parked title only when its recorded reason is an outage, not on a retry time alone
f1e4f04 Retire a queued automatic provider image when its slot holds local or locked artwork
aeaefd5 Let only a directly following year displace an earlier one as the release year
9bcab2a Recognise frame sizes by shape in any letter case and keep them out of the series name
```

## Verification after review

From `server/`: `gofmt -l` on every changed Go file — no output; `go build ./...` ok; `go vet` on ingestion, catalog, metadata, metadataprovider, localmetadata, notify, operations — clean.

`go test ./internal/ingestion/... ./internal/worker/... ./internal/catalog/... ./internal/metadata/... ./internal/metadataprovider/... ./internal/localmetadata/... ./internal/notify/... ./internal/operations/... ./internal/persistence/... ./internal/supervise/... ./internal/dbwork/... ./internal/storage/... ./cmd/server/... -count=1` — all `ok` (notify has no test files).
