# base-health — the combined server base against its own tests

Worktree `server-base-health`, branch `audit-implementation/base-health` (head `5f5216f`; 19 commits of its own since the first merge, plus two merges of `audit-implementation/main`; nothing pushed). Host: ehler-personal (mini-1).
Logs: first full run `_program/scratch/base-health/run2/`, final full run `_program/scratch/base-health/final/`.

## 1. Outcome

Every suite in the brief passes on the combined base. Final results are at the end of this file (section 10).

The first full run found 13 failing server packages (one of them not compiling), 28 failing shared-client tests, and nothing wrong in the web check, Cast or `apikit`. About half of the server failures were tests or inventories that had not caught up with another lane's deliberate change. The other half were real defects that only appear when the lanes are combined; the ones an owner or viewer would have met:

- Removing a library read whole tables over and over, and a new library could be handed the removed one's internal number while rows still pointed at it.
- The Continue Watching count went stale after progress moved.
- A rating of 8 out of 10 was weighed by recommendations as "eight stars out of five".
- An album with untagged discs was "disc 1" in one place and "disc 0" in another, so the album page and playback disagreed.
- About twenty change routes had lost the rule that a change naming no revision is refused, so a client that forgot the revision would silently overwrite.
- A page of the title list sorted everything after the cursor instead of reading in order.
- The Genres, Decades and Series tabs answered an error when the viewer's chosen sort was not "title".
- The inbox handed a restricted profile the identifiers of titles and libraries hidden from it (inside a notice's link), and reporting a problem with a hidden title answered differently from reporting one that does not exist.
- One sub-folder that could not be opened stopped the scan of its whole parent folder.
- Any film or episode sitting directly in a library's root never had its subtitle files found, and every such scan finished "with warnings".
- A missing `.nfo` was reported as an unreadable one.
- With the owner's tone mapping switch off, an HDR title would not play at all on a device without HDR.

The earlier, interrupted run's commit was reviewed line by line. It did not compile in three places, had loosened two fences that should have held, had dropped the positive half of one test, and had classified several routes in the restriction matrices with probes that could not fail. All corrected (section 3, "Earlier run").

## 2. Decisions I made

1. **A removed library's leftovers.** The clean-up added by the freshness work read whole derived tables once per 64-row batch. Rewritten so every statement goes through an index: class projections are cleared through their class; per-title rows and the totals built from them are left to the derivations that own them, which remove both sides together. `catalog_libraries.id` is now AUTOINCREMENT so a removed library's handle is never given to a new library while such rows remain. New test `TestRemovedLibraryLeavesNoRowsUnderItsHandle`.
2. **A missing disc number is disc one, everywhere.** The music work made untagged tracks sort as disc one, but the album page, disc sections, selection, downloads and playback still grouped them as "disc 0 / Unnumbered disc". One rule now; there is no "Unnumbered disc" section (the web never showed one).
3. **A change with no revision is refused on every typed route (428 `revision_required`).** The rule now lives in the route registry (`apikit`) for PUT, PATCH and DELETE bodies that carry `expectedRevision`, with its own test. POST is left alone (create bodies share types). Two tests that expected a plain 400 for the same refusal now expect 428.
4. **Groupings (genres, decades, series) answer in title order when asked for another sort**, say so in `applied`, and ignore a letter jump. The search-and-sort work had made this a 400. A screen that carries the viewer's chosen order onto the Genres tab is not making an error, and "browse must always answer". (The integrator asked for this call.)
5. **Read-path golden file recorded again** after checking every kind of difference (listed in the test's header). Unchanged and verified: grouping members and counts, every restricted viewer's totals, all status codes. One accidental change found on the way and fixed: rows with no added date answered `"addedAt": ""` instead of `null`.
6. **Name parsing.** `Show.720x101.mkv`: a common picture width followed by a long number is a picture size (the newer reader's rule, closer to Plex); the old expectation of season 720 was updated and `S720E101` is asserted to still work. A stacked part (`- pt1`) is attached as part 1 of its episode rather than left for the owner.
7. **Album identity test** follows the music work's rule: one release identifier in two folders is one album; a different release in the same folder is a different album.
8. **Popular on this server** needs three viewers; the restriction test that still used two passed its "hidden history is not counted" assertion for the wrong reason. It now has three.
9. **Recommendation rows by day.** One test depended on the calendar (rows of equal strength take turns daily); its fixture now gives the genre row titles of its own. A paging cursor issued yesterday keeps working today (the freshness decision: "continuing lists") and the test says so.
10. **Library settings route** answers the bare document, names a refused value in `fields` again, and answers 412 for a stale revision.
11. **A sub-folder that cannot be opened is skipped, the rest of its parent is scanned, and the owner is told which folder** (issue `directory_unreadable`, protected so nothing under it is marked missing). Before, one such entry failed the whole parent page. This is the owner's "reject the smallest unit" rule.
12. **A notice's link is left out when it leads somewhere its reader may not see.** The notice text still shows; only the navigate action (which carries the title's or library's identifier) is dropped. Chosen over hiding the whole notice because notices are written for the profile and their text is already safe.
13. **A report about a hidden title answers exactly like a report about a title that does not exist** (404), on the report routes and the console.
14. **Tone mapping off no longer takes a title away.** With the owner's switch off, an HDR title on a device without HDR was refused outright ("users may make unsafe choices; do not block them", "it just plays"). It is now converted without tone mapping (washed-out colours, as the owner asked for), and the plan's trace carries the existing "tone mapping is off" note. The converter still refuses an HDR-to-SDR target that leaves tone mapping out by accident: the plan has to say `unmapped` explicitly.
15. **Playback decision matrix.** Expectations moved to the audio lane's "lossless first" rule (FLAC where the device plays it, Opus copied where it plays), and the matrix now runs with a server that has the Dolby Vision tools, plus a second test for a server that has not. Surround is asserted to survive where the device has the speakers for it.
16. **Upload lane**: the earlier run changed the test to "two at a time, no time budget, ends with the client". I checked this against the limits lane's code: it is deliberate (slow uploads keep going, with a 60-second no-progress cut-off), so the expectation stands.
17. The hand-written `server/api/openapi.yaml` was brought in line with the millisecond names for the schemas tests validate. It duplicates the generated contract and should be retired (section 7).

## 3. Failures found, cause, change

"Collision" names the two pieces of work that met.

### Did not compile
| Where | Cause | Change |
| --- | --- | --- |
| `internal/downloads/requests.go` | Earlier run declared `query` twice | Renamed; kept the visibility filter it was adding |
| `httpapi/admin_access_test.go` and four other test files | Earlier run: call missing its path, unused imports | Fixed |

### Real defects (code changed)
| Test that caught it | Cause | Change |
| --- | --- | --- |
| `TestPerItemDerivationStatementsSeek` | Collision, freshness × catalogue guard: library clean-up scanned whole tables per batch; deleting a title scanned two child tables with no index | Clean-up rewritten on indexes; four indexes added; handle never reused |
| `TestContinueDismissalIsProfileScopedAndReturnsOnlyOnReplay` | Collision, personal state (split counters) × Continue Watching count cache: the cache key ignored the progress counter | Key includes it |
| `TestRecNextBookLoadsSeriesSignals` and rating signal tests | Collision, search-and-sort (ratings 0–10) × recommendations (weight assumed 0–5) | Weight reads the rating as stars; new test |
| `TestListeningMembershipVisibilityUsesIndexedPaths`, album tests | Collision, music fixes × listening pages: disc 0 versus disc 1 | One rule (decision 2) |
| `TestAdministrationMissingRevisionHTTP`, member deletion, playlists | Collision, typed-route migration × revision rule: missing `expectedRevision` read as 0 | Rule in the registry (decision 3) |
| `TestTitleCursorSeeksSortIndex` | Collision, search-and-sort × freshness (serving views): a text cast over the two-part view forced a full sort | Cast removed; test now requires a seek in both halves and no temporary sort |
| `TestReadPathGoldenEquivalence` | Search-and-sort: groupings refused a non-title sort; `addedAt` empty string | Decisions 4 and 5 |
| `TestAdministrationOwnerBoundaryAndLibrarySettingsRoundTrip` | Collision, scan work (typed route) × administration: refusal lost its field name | Field errors restored |
| `TestRestrictedProfileRouteMatrix`, `…WriteRouteMatrix` | About 45 routes from other lanes unclassified; two real leaks behind them (notice links; hidden versus absent on reports) | Decisions 12 and 13; every route classified by a probe that reads the answer (below) |
| `TestScanLoopProtectsUnreadableSubtreeButContinues` | Collision, identity review (entry identity must be observed) × scan loop: one unopenable entry failed the parent | Decision 11 |
| `TestRealMovieScanDiscoveryPaginationAndCollections`, `TestScanFinalSourceCheckUsesRealIsolatedRootBeforeReconciliation` | Collision, subtitles lane × scan loop: a title at the library root asked for folder `""`, which the storage helper refuses, so no sidecar was ever found there and every scan ended with warnings | Root is `.` end to end |
| `TestSidecarAddedAfterItsFolderWasListedIsStillRead` | Storage helper could not say "no such file", so a missing `.nfo` became `video_nfo_unavailable` | Helper answers `not_found`; client returns the ordinary not-exist error |
| `TestDeliveryToneMapArguments` | Collision, playback settings × planner: owner switch off refused the title | Decision 14 |
| `TestNoRawWriteTransactionsOutsideDBWork` | Guide spool opened its own transaction (fixed by the earlier run; reviewed, kept) | On the write gate |

### Tests and inventories that had not caught up (test changed, with the reason)
| Test | Why the test was the one to change |
| --- | --- |
| 28 shared-client tests (`client-core`) | Fixtures in seconds and 0–5 stars after the wire moved to milliseconds, RFC 3339 instants and 0–10; title lookup is one request now; a stale cursor restarts quietly and keeps the grid |
| `TestPlaylistOrderRightsSnapshotAndCAS`, `TestShowWorkspaceRejectsCrossedParentsAndCursorFences` | Earlier run had loosened them to accept a stale snapshot; restored to refuse it |
| `TestRecommendationCursorRejectsDailyRotationChange`, `TestRecommendationRestrictionSameAvailableChildAndCommunity`, `TestDiscoverCarriesPicksForYou` | Decisions 8 and 9 |
| `TestFrameSizeTagsInAnyCaseAndPositionLeaveTheEpisodeReadable`, `TestReparseEpisodicIssuesOnceAfterAParserChange`, `TestWave2PhysicalAlbumEnrichesMissingEditionWithoutMergingConflicts` | Decisions 6 and 7 |
| `TestDeleteWithoutFilesLeavesThemAndKeepsEachProfilesState` | Rating fixture on the old scale |
| `TestAdministrationTierBoundaries`, `TestLibraryDiscoveryAndCollectionPermissions`, `TestExtraDetailOmitsUnsupportedRelatedContract`, `TestOpenAPICoversRegisteredRoutes` | Routes renamed or retired by other lanes (API keys, client logs, Home); the retired form is now asserted to be refused |
| `TestEveryProcessSpawnIsMediaexecOrListed`, `TestStoredSettingsHaveRuntimeConsumers`, `TestProductionStatementsPrepareAgainstTheBaseline` | Inventories missing other lanes' additions. Each addition was read first: the new process spawns go through the sandboxed runner; `monitorChanges` really is read by the folder monitor; the guide spool's statements are now prepared against the spool's own tables rather than skipped |
| `TestSubtitleRequestsTakeTheWriteGateInTheirOwnClass` | Counted exactly one extra hold of the gate; now asserts the class, which is what it protects |
| `TestAnalysisPreviewCapturesCloseWithoutEarlyPublication` | Previews are one sheet plus a descriptor since the preview lane |
| `TestSupervisedLyricsRunSitsOutAProviderPause` | Lyrics now need an exact recording match; the fake provider answered a different one |
| `TestAFailedConditionalRefreshDoesNotRenewTheWindow`, `TestValidatorsArePersistedAndNotLostOnANotModified` | Metadata documents are per language and region; the test mixed two |
| `TestArtworkSlowPublicationDoesNotBlockReadsOrPreviews` | An unchanged artwork address is no longer fetched twice; the test now changes it |
| `TestStoppedScanTellsTheOwnerWhereItStopped` | Its trigger (an unopenable folder) no longer stops a scan. Replaced by three tests: the folder is skipped and named; a scan that really stops tells the owner without the disk path; the notice names the folder |
| `TestClientDeclaredTablesDriveDeliveryMatrix`, `TestChromecastFalseHEVCClaimChangesExactlyAffectedRows`, `TestDeliveryCopyRemuxSelection`, `TestFragmentedSignallingFollowsActualDolbyVisionLayer`, `TestLegacySubtitleReplanAppliesCurrentCeilings` | Decision 15; the planner now needs to be told what the server's tools can do; a fixed quality's rate comes from the codec table |
| `TestMPEGTSSeekRestartsOnIndexedPacket` | Sandbox read path listed before the file existed |
| Grid validator test | Earlier run kept only "an unrelated write keeps the validator"; added "a change to a title in the grid changes it" |

### Restriction matrices
Fixture now wires the console, access, credentials and self-service areas, seeds reports, notices with links to hidden things, and download preparations, and searches every answer for the hidden titles' names as well as their identifiers (96 markers). Classified by a real probe: inbox and notice routes, both report APIs, download requests, preparations and bundles, the title list, saved views, listening selection, and the tool endpoint (`POST /v1/mcp`, exercised with the restricted profile's own access token because it refuses sessions). Admin GET routes are asserted to refuse a restricted member. Nothing was allow-listed to get green.

### Load-sensitive, noted and not chased
`TestDefaultMediaCompositionAddsScansAndBrowsesWithoutRootsEnv` and `TestFiniteRealRestartReplaysPersistedQueueInOrder` (both passed in every full run here). `ingestion` recovery test: given up to eight more work quanta before it must have failed, because the number depends on how the scan loop chunks.

## 4. API changes

- Typed PUT/PATCH/DELETE routes whose body has `expectedRevision`: a missing or null revision answers **428 `revision_required`** (was: treated as 0, or a 400 on two routes). `revision_required` is in each such route's error list in the generated contract.
- `GET /v1/content` groupings with a non-title `sort`: 200 in title order with `applied.sort` saying so (was 400).
- `addedAt`: `null` when unknown (was `""` briefly).
- `PUT` library settings: a refused value answers `invalid_request` with `fields`.
- Inbox (`/v1/notifications…`): a navigate action to a title or library the reader may not see is omitted.
- Report routes and console: hidden title ≡ absent (404).
- Disc sections: no `…:disc:0` identifiers; untagged tracks are in disc 1.
- Playback plan: `toneMapOff` (boolean, present only when true); conversion target `unmapped` (same).
- Storage helper `stat`: answers `not_found` instead of an error.

## 5. Schema changes

`0001_baseline.sql` (schema-sync run, leaves no diff): `catalog_libraries.id INTEGER PRIMARY KEY AUTOINCREMENT`; indexes `catalog_book_groups_every(library_id,id)`, `catalog_personal_visibility_sort_bucket_class(class_id,generation)`, `playback_track_memory_item(item_id)`, `inventory_artwork_sources_entity(entity_id)`.

## 6. Owner or viewer settings added or wired

None added. The playback setting `toneMapping` off now means "convert without tone mapping" instead of "refuse" (decision 14). `monitorChanges` was confirmed to have a real consumer and recorded in the settings inventory.

## 7. Needed from others

- **Clients**: handle 428 `revision_required` like a stale revision (re-read, retry); expect FLAC or copied Opus audio where the old expectations said AAC; show something for `toneMapOff` if wanted (a quiet note at most).
- **Playback**: a Dolby Vision title that cannot be reshaped on a server without the tools is refused with the wording "No permitted delivery route…", which reads like a permission problem. Needs plain wording.
- **API owner**: retire the hand-written `server/api/openapi.yaml` (duplicates the generated contract); remove the duplicate older report and notification routes left beside the new ones; a few wire fields still carry "seconds" in their names.
- **Build**: `go build -tags devtrust ./...` does not compile on the base (not part of this lane's commands; not fixed).
- **Restriction matrix fixture** cannot exercise the listening and subtitle services (not wired in the fixture); those routes are classified from their handlers and their own packages' tests.
- Five files that were not gofmt-clean on the base were formatted (no behaviour change).

## 8. Test notes for other environments

- **Unopenable folder** (Linux or macOS): in a library, `chmod 000` one sub-folder, scan. Pass: the scan completes, everything else in the parent appears, the owner's notices name the folder, and nothing under it is marked missing. Fail: the parent folder's titles are missing or the scan stops.
- **Windows**: same test with a folder whose ACL denies the service account. The Windows observation code is compiled here (`GOOS=windows go build ./...` passes) but not run.
- **Root-level title with a sidecar**: put `Film (2020).mkv` and `Film (2020).en.srt` directly in a library root, scan. Pass: the subtitle is offered and the scan ends "complete", not "complete with warnings".
- **Tone mapping off**: owner setting off, play an HDR10 title in a browser without HDR. Pass: it plays, colours washed out. Fail: "cannot play".
- **Removing a large library** (100k+ titles) on a slow disk: the interface must stay responsive throughout; a library added straight afterwards must show none of the removed one's titles.

## 9. Not finished

- Nothing in the brief's command list is failing.
- Not done, by scope: the items in section 7.
- Six more commits landed on `audit-implementation/main` while this ran (console and web screens; no server or `apikit` files). They are merged into this branch as its last commit, with one conflict in a shared-client test both sides had fixed (kept this branch's version, which asserts more; the code under test is identical). The three JavaScript suites were run again after that merge; the Go suites were not, because the merge changed no Go file. Anything merged onto the base after `1097d54` is for the second pass.

## 10. Final results of every command

Go commands run at `5b39681`; JavaScript commands run again at the head `5f5216f` after the last merge (which changed no Go file). Go runs used `GOFLAGS=-p=4` and `PORTICO_PLAYBACK_TEST_HELPER` set to a freshly built helper. Logs in `_program/scratch/base-health/final/`.

| Command | Result |
| --- | --- |
| `go vet ./...` (server) | Exit 1, and every finding is in the vendored SQLite driver (`internal/thirdparty/sqlite`, "possible misuse of unsafe.Pointer"); nothing in Portico's own packages |
| `go test -count=1 ./...` (server) | Pass: 85 packages ok, 16 with no tests, 0 failures. Both load-sensitive start-up tests passed |
| `go test -count=1 ./...` (apikit) | Pass: 4 packages ok |
| `npm run test:core` | Pass: 1827 of 1827 (1823 before the last merge added four) |
| `npm run check --workspace portico-web` | Pass: exit 0, 512 of 512 |
| `npm run test:cast` | Pass: 20 of 20 |
| `go build ./...`, `GOOS=windows`, `GOOS=darwin` (server) | All three build |
| `python3 scripts/schema-sync.py`, `go run ./cmd/apigen` | No diff |

First full run, for comparison (`run2/`): server 13 packages failing (32 failing tests, plus the catalogue package not compiling and ten more failures behind that); shared client 28 failing of 1823; web check, Cast and apikit passing.
