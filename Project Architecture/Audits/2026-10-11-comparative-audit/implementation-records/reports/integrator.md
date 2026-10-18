# Integrator — four server merges, two native merges

**State: all six steps done and committed. Nothing pushed.**

- main-server (`audit-implementation/main`): `afa05fd` review-networking, `9ebf953` server-followups, `9e7657e` search-sort, `5d30777` audio-engines, `e939fbb` web rating follow-up.
- main-native (`audit-implementation/main`): `1ef6936` audio-engines, `2357f5d` freshness-clients, `6c3d60b` packages refreshed, `1696b6f` fixes.

What an owner should know first:
1. Three of the four server merges hid a break that produced no conflict marker: a settings save that would have hung (merge 1), a method defined twice (merge 2), and a search schema left half in each lane's shape so no search write or rebuild could work (merge 3). All three are fixed and tested.
2. Where two lanes built the same thing twice I kept one design and said which: Home "See all" follows personal-state; search rebuilds follow freshness-server with search-sort's content; FLAC seeking uses web-player's quick start and falls back to audio-engines' bounded search; the Android player uses native-playback's single builder with audio-engines' gain stage.
3. search-sort changed units on the wire without changing every sender. I fixed the ones that broke the product (music queue loading, offline viewing, star ratings on web and apps) and listed the rest.
4. The base had many failing tests before I started. Each section says which tests were already failing and which the merge affected.

Working report, updated after each merge. Baselines for "was this already failing" were taken from a plain export of the commit before each merge (in the session scratch directory, not a worktree).

## Server merge 1 — review-networking (commit afa05fd) — DONE

**Conflict:** `server/internal/httpapi/connection_policy.go` only.

**What the two sides had done.** Both had independently added a "never let an older settings document replace the live one" fence to the policy cache, and both had added a mutex with the same name (`applyMu`) for different purposes:
- base (limits/subtitles work): `set()` takes `applyMu` itself, skips a document older than the live one, and applies the owner's subtitle limits in the same step;
- lane: `applyLiveSettings()` (new, in `connection_settings.go`, merged cleanly by git) takes `applyMu`, re-reads the committed settings and then calls `set()`.

Git merged the second file without complaint. Taken together the result **deadlocked on every owner settings save** (a non-reentrant mutex taken twice on one goroutine). This was the semantic break in this merge.

**Decision.** `set()` keeps the base behaviour (lock, fence, subtitle limits, store). A new `setLocked()` holds the body; `applyLiveSettings()` calls `setLocked()` because it already holds the lock. The fence is "older than live is ignored" (base's `<`), not the lane's "older or equal": the same revision is the same document, so re-applying is harmless, and it keeps both lanes' tests true. The lane's compare-and-swap loop is dropped as redundant under the mutex. Lock order checked: `mu → applyMu` only (first-read path); `applyMu` is never held when `mu` is taken.

**Fixed afterwards.**
- `internal/networking/remote_contact_test.go` `TestATopologyChangeThatChangesNoRouteDoesNotCallHosted` fails on the lane tip by itself (not a merge artefact): the lane made Hosted publication TLS-only, so the fixture's plain-HTTP LAN address is no longer published and "a LAN address change publishes once" could not happen. Gave the test a trusted certificate (the lane's own `installSyntheticCertificate` helper) so its LAN address is published; the test's purpose is unchanged.

**Checks.** `go build ./...` linux, windows, darwin: pass. `go vet` cmd/server, httpapi, networking, networkpolicy, operations: pass. `go test ./internal/networkpolicy ./internal/networking`: pass. `go test ./cmd/server -run 'SIGTERM|Start|Ready|HTTP2|Connection|Owner'`: pass. httpapi `ProductionRouter|Scope|Credential|Registry|Settings|…` plus both lanes' cache tests (`TestSettingsReplayDoesNotRestoreProxyTrust`, `TestConnectionPolicyRejectsOlderSaveCompletion`, `TestOlderSettingsReceiptCannotLowerRunningSubtitleLimit`, `TestConnectivitySaveAppliesRuntimeSettings`, `TestDeviceRegistrationUsesVerifiedClientAddress`): pass except one already failing on the base (below). apigen: no generated change. noscope: none.

**Already failing on the base before this merge (not caused by it, left alone):**
- `internal/httpapi` `TestAdministrationOwnerBoundaryAndLibrarySettingsRoundTrip` — "no-feed update state: unavailable".
- `internal/operations` `TestStoredSettingsHaveRuntimeConsumers` — `administration.LibrarySettings.monitorChanges` has no reviewed runtime consumer.
- `go build -tags devtrust ./...` — `internal/httpapi/dev_e2e.go` uses `loopbackPeer`, which the networking lane removed. Development-only build, but it does not compile.

## Server merge 2 — server-followups (commit 9ebf953) — DONE

**Conflicts (15 files; the brief listed 8 — the five generated contract files and two `operations` files conflicted as well, the latter because merge 1 had just changed them).**

| File | Decision |
|---|---|
| `packages/contracts/src/*` (4), `server/api/portico.openapi.json` | Generated: took the base side, then regenerated with apigen. The lane's routes are present in the result. |
| `packages/client-core/src/index.ts` | Kept both export lists (freshness, event feed, Home service, scan issues **and** playback controls). |
| `server/internal/httpapi/route_lanes.go` | Kept both sets of lane entries. |
| `server/internal/persistence/migrations/0001_baseline.sql` | Kept both blocks appended at the end of the file (freshness generations/retention objects **and** the lane's download indexes, maintenance status and live channel choices). schema-sync passes. |
| `server/cmd/server/main.go`, `httpapi/router.go` | Kept both sides' dependencies: `ConnectionUsage`, `LocalArtworkUsage` (base) and `CacheDirectory` (lane). |
| `operations/store.go`, `operations/settings.go` | Three "value in use since startup" projections now live side by side: temporary directory (base), connection limit (merge 1) and FFmpeg path (lane). Each reports its running value as effective and lists the field under restart-pending only when the saved value differs. Written in the base's form. |
| `httpapi/inventory.go` | Each side had deleted a different hand-written route because it had become a typed route elsewhere (lane: library inventory → `inventory_read.go`; base: inventory changes → `scan_loop.go`). Both deletions kept. |
| `catalog/home_rows.go`, `catalog/discovery.go` | **Followed the base, dropped the lane's version.** Both sides added a "See all" destination. The base (personal-state) made it a tagged destination on every Home row (library / row / saved view) and already gives the Discover recent section a browse destination. The lane's plainer `{pivot,sort}` on recent rows only is a subset of that and would have been a second, conflicting shape for the same field. Nothing of the lane's is lost: recent rows still name pivot and sort, inside the tagged form. |

**Semantic break found beyond the textual conflicts.** Both sides added a method called `ArtworkUsage` to the local-metadata service with different signatures and purposes (base: never waits, may answer "not measured", for the resource-usage page; lane: measures the folder if nobody has yet, for the metadata settings figure). Git merged it silently; it did not compile. Kept both; the lane's is now `MeasuredArtworkUsage` and the metadata service is wired to it. Only one of the two sides increments the artwork byte ledger (the lane's fix), so there is no double counting — checked.

**Verified the lane's "keep these when merging" hooks survived:** `removedPathEvidence` / `applyReportedMissingTx` in `ingestion/targeted.go`; `moveDownloadClaimsTx` in title merge and version separation.

**Checks.** Build linux/windows/darwin: pass. `go vet` on all 13 touched packages: pass. apigen and schema-sync: run, clean. noscope: none. `cmd/server -run 'SIGTERM|Start|Ready'`: pass. Whole-package tests for downloads, administration, localmetadata, livechannels, backup, ingestion (named areas): pass. All 11 tests the lane added: pass. httpapi required set plus route lanes, MCP, inventory, Home, attention, storage, downloads: pass apart from tests already failing before this merge. `tsc --noEmit` in `web` and in `packages/client-core`: clean. client-core tests the lane changed (30) and the areas both sides touched (137): pass.

**Failing before this merge and unchanged by it** (compared against an export of afa05fd; identical messages before and after):
- `internal/metadata`: `TestArtworkSlowPublicationDoesNotBlockReadsOrPreviews` (deadline exceeded), `TestValidatorsArePersistedAndNotLostOnANotModified`, `TestAFailedConditionalRefreshDoesNotRenewTheWindow`.
- `internal/playbackv1`: `TestLargeContainersListInIndexOrder`.
- `internal/persistence`: `TestProductionStatementsPrepareAgainstTheBaseline` — `livechannels/programme_spool.go` uses a table `programmes` that is not in the baseline (five statements).
- `internal/catalog`: `TestDiscoverCarriesPicksForYou`, `TestWave2PhysicalAlbumEnrichesMissingEditionWithoutMergingConflicts`.
- `internal/httpapi`: `TestLibraryDiscoveryAndCollectionPermissions` (Home answers 400), `TestRestrictedProfileRouteMatrix` and `TestRestrictedProfileWriteRouteMatrix` (routes with no restriction classification, and three download-artifact probes answering 401). The unclassified list is line-for-line the same before and after: this merge added no unclassified route.
- plus the two from merge 1's list (`TestAdministrationOwnerBoundaryAndLibrarySettingsRoundTrip`, `TestStoredSettingsHaveRuntimeConsumers`).

## Server merge 3 — search-sort (commit 9e7657e) — DONE

This was the merge with real cross-lane design work. The lane had never seen the freshness rebuild machinery (two search generations, generation-retained browse rows, the event feed) or the typed Home routes: its "merge of an older base" predates them.

**Conflicts (15 files; nine listed in the brief plus `client-core/src/console.ts` and the five generated contract files).**

| File | Decision |
|---|---|
| `packages/contracts/src/*`, `server/api/portico.openapi.json` | Generated: took the base, regenerated with apigen. |
| `packages/client-core/src/console.ts`, `catalog/search.go` | Import lines only; kept both. |
| `compactcatalog/derive_search.go` | **Kept both, redesigned the join.** freshness-server's part: which generation ("slot") is written, and that the full-text index follows its documents through the tables' own triggers. search-sort's part: what a document contains (the expanded search text, the folded text used to confirm substring matches, people names). The lane wrote the index by hand as well; with the base's triggers that would have indexed every title twice and then removed it twice, so the hand-written index statements are dropped and only the document statements remain, run against whichever slot is being built. |
| `compactcatalog/write.go` | Followed the base: deleting a title removes its document and the trigger removes the index row. The lane's manual index delete is dropped for the same reason. |
| `httpapi/browse_routes.go` | Took the lane's extracted `browseHandler` / `facetsHandler` (they are now typed registry routes) and carried over the base's one change inside that code: the facets validator uses the stable viewer identity, not the token hash. Checked that no other token-hash use came back anywhere in `httpapi` (same 20 sites as before the merge). |
| `httpapi/home.go`, `home_routes.go`, `saved_resources.go` | **Followed the base.** personal-state already serves `GET /v1/home`, `GET /v1/home/rows/{id}`, `GET /v1/content` and `GET /v1/personal-history` as typed routes. The lane's change in these files was only to wrap the old hand-written handlers so its own "projection" could register them; those handlers no longer exist. The lane's millisecond and whole-number-rating wire format is applied by the response types themselves, so the base's typed routes emit it without further change. |
| `httpapi/notifications_routes.go` | Followed the base: the two notification listeners (`/v1/notifications/wait`, `/events`) stay deleted; the one event feed replaces them. |
| `httpapi/route_lanes.go` | Kept both sets of entries. |

**Semantic breaks beyond the textual conflicts (none of these produced a conflict marker):**

1. **Baseline schema left half-and-half.** Git merged the SQL cleanly into a state that could not work: the serving search tables had the lane's columns (`search`, `normalized`) while the second generation's tables and all six index triggers still had the base's (`title`). Every search write would have failed, and a rebuild could never have switched. Gave `catalog_search_documents_next` / `catalog_search_titles_next` the same shape as the serving pair and rewrote the six triggers to index `search`. This is exactly the item freshness-server's report asked the integration to do.
2. **Duplicate registrations.** `unit_projection_routes.go` registered projections for the four routes the base now serves typed, and called `contentHomeRoutes`, which the base removed. Removed those four projections and the call. apigen confirms no duplicate route.
3. **Two schema methods for content cards** (`ContentEntry.JSONSchema` on both sides; did not compile). Kept the lane's, because it describes the millisecond fields and 0–10 rating that are actually sent, and it reflects over the struct so the base's newer fields (`missing`, `discNumber`) are included. Deleted the base's `catalog/content_schema.go`.
4. **Genre and series paging refused a continuation after any library change.** The lane's new paging returned "stale list" when a cursor or range came from an earlier revision. That is the behaviour freshness-server removed from title lists on the owner's instruction (a person never sees a conflict unless the thing they acted on changed). Changed it to match: the continuation keeps its place in the list as it now is. Extended the lane's 205-series test to cover a change mid-paging.

**Defects that arrive with the lane itself (fail on the lane's own tip, not caused by merging) — fixed because they break the product or tests in plain sight:**

- **Music and audiobook queue loading answered 400.** The lane's projection of `GET /v1/libraries/{id}/listening/selection` listed the wrong query parameters (`shuffle`) and omitted the ones the handler reads and the shared client always sends (`mode`, `resume`, `startItem`). Corrected the list. I then compared every other projection's parameter list with its handler; the rest match or are supersets.
- **Job and schedule routes answered a bare "404 page not found"** on a server without a scheduler (outside the error envelope). The projection now answers an unmounted handler with the standard not-found envelope.
- Stale test expectations updated to the lane's deliberate behaviour: personal-rating anchor is `9` on the 0–10 scale (fixture was changed by the lane, the expectation was not); a one-character people search is valid; two console tests mounted the console routes a second time on top of the registry (now set up the way the production router does). The restricted-profile count test treated a facet page's `total` (a count of genre choices) as a count of titles; it now checks each choice's own count against the visible titles instead, which is the stricter and correct reading.

**Added test:** `TestSearchGenerationsCarryTheSameDocumentsAndStayConsistent` — two rebuilds, asserting each generation holds the lane's document shape, the accent-folded match works in each, and both full-text indexes pass SQLite's integrity check (which fails if the index were written twice).

**Checks.** Build linux/windows/darwin: pass. `go vet` on the 12 touched packages: pass. apigen, schema-sync (with its persistence test), noscope: clean. `cmd/server -run 'SIGTERM|Start|Ready'`: pass. Whole-package runs of `catalog`, `compactcatalog`, `httpapi`, `dbwork`, `persistence`, `operations`, `downloads`, `apiwire`, `sorttext`, `segmentmarkers`, `assets`, compared test-by-test with an export of the commit before the merge: **the only test failing after the merge that passed before is `TestExtraDetailOmitsUnsupportedRelatedContract`** (below). All seven tests the lane added pass. `tsc --noEmit` in `web` and `packages/client-core`: clean.

**Left failing, with reasons:**
- `httpapi` `TestExtraDetailOmitsUnsupportedRelatedContract` — the hand-written `server/api/openapi.yaml` still describes `duration` / `progressSeconds` (23 old-unit field names in that file). Fails on the lane's own tip. That file duplicates the generated contract; per the programme brief it should be deleted or regenerated by the API owner, not patched here.
- `httpapi` restriction matrices (already failing before): the lane's lane-table entries add 18 read routes and 2 write routes to the "no restriction classification" list — the console, notification and feedback routes (`/v1/notifications*`, `/v1/feedback*`, `/v1/admin/feedback*`, `/v1/admin/console/{alerts,diagnostics,exports,jobs,panels,schedules}`, `/v1/admin/playback/history`, `POST /v1/feedback`, `POST /v1/feedback/reports`). These routes existed before; they are newly visible to the test. Feedback can name a title, so they need real probes, not a one-line reason — left for the lane that repairs the matrices.
- `httpapi` `TestReadPathGoldenEquivalence` (already failing, 275 cases differ before the merge): 42 more cases now answer 400, because the lane makes genre/series/category groupings refuse a sort other than title and refuse letter seeking. Deliberate in the lane. The web library screen does not appear to send those today (it reads these groupings through the content route, as far as I traced; the native apps were not checked), but a client that carries a chosen sort across to the Genres tab would get an error rather than a title-sorted list. Worth a product decision: refuse, or quietly fall back to title.
- `packages/client-core`: 34 tests fail after the merge; none is new to the merge (15 failed on the base, 38 on the lane's tip; 25 come from the lane alone). The lane-only ones are test fixtures still written in the old wire units (seconds, epoch numbers): `inbox` (10), `inbox-coverage` (4), `group-session-coverage` (3), `segment-markers` (2), `title-split` (2), `show-workspace` (2), `catalog` (2), and one each in `telemetry`, `stream-options`, `remote-onboarding`, `recommendation-rows`, `playback-history`, `personal-labels`, `linear-lan`, `downloads-service-coverage`, `bulk-jobs`. I confirmed the cause on two of them, not on all 25.

## Server merge 4 — audio-engines, web (commit 5d30777) — DONE

**Conflicts (the 8 files listed).** The other side is the web-player lane, which had implemented three of the same things as this lane, differently.

| File | Decision |
|---|---|
| `web/src/bridge/audio/flac.ts`, `types.ts`, `track.ts` | **Both lanes wrote "start a FLAC part-way without reading what comes before".** web-player: use the seek table, else a few proportional reads corrected from the frames' own numbers; if nothing is confirmed, read from the start. audio-engines: a bounded search in which every candidate frame is confirmed by its checksum; if nothing within 30 seconds of the position is confirmed, give the position to the ordinary player rather than read the whole file before it. Kept both, in that order: the FLAC `seek` tries the base's quick start; if it confirms nothing and the position is more than 30 seconds in, the lane's bounded search runs (and may hand on to the ordinary player); within 30 seconds of the start the caller walks, as on the base. `seek` keeps the base's signature (may answer "nothing confirmed"); the lane's `locate` stays on the demuxer as the bounded search. The lane's rule that MP3, ADTS and Ogg positions more than 30 seconds in go to the ordinary player is kept unchanged (web-player's report had left exactly that to this lane). |
| `web/src/bridge/DecodeAudioRender.ts` | **Both lanes handled an interrupted audio context.** web-player: declare the page as playing media on iPhone/iPad and start an interrupted context again while the listener still means to play. audio-engines: while the context is not running the position clock stops, and when it runs again the schedule is brought up to date. Kept both: one context-state handler does the lane's part and then the base's resume; the base's "page became visible" resume is kept too. |
| `web/src/bridge/useSystemListening.ts`, `web/src/player/Player.tsx` | Kept both parameters: the base's `byline` (so films and episodes show "show · episode" in the system's media controls) and the lane's `chapters` (so a book shows author, narrator and the chapter being read). A title with a credit of its own shows that; otherwise the byline. |
| `web/test/audio-demux.test.ts`, `server/internal/localmetadata/audio_test.go` | Kept both sides' tests (and both imports). |

**Added test:** a FLAC with nothing to confirm a deep start is handed on, and one near the start is walked — the dispatch between the two lanes' FLAC paths. The fixtures in the repository are a few seconds long, so the deep branch cannot be exercised on real audio here.

**Checks.** `tsc --noEmit` in `web` and `packages/client-core`: clean. All ten `web/test/audio-*.test.ts` files (39 tests, both lanes'): pass. Web player tests (20) and client-core `audio-render-v2` / `audio-effects` (9): pass. Go: build linux/windows/darwin pass; vet and whole-package tests for `localmetadata` and `storage` pass (WEBP cover test and the base's artwork-capacity test both run); `httpapi -run 'ProductionRouter|Scope|Credential|Registry'` and `cmd/server -run 'SIGTERM|Start|Ready'` pass. No route or schema change in this merge, so apigen and schema-sync were not needed.

**Test note for a real device or browser (not possible here):** seek an hour into a single-file FLAC audiobook over a slow link, with and without a seek table: playback should start within a few seconds with a handful of range requests. Then repeat with a FLAC whose middle is damaged: it should start through the ordinary player (no gapless or effects for that session) rather than download the whole file. On iPhone Safari, start music, take a call, end it: the position must not advance during the call and playback must resume by itself.

## Follow-up on the server base after merge 3 (commit e939fbb)

Found while checking the native side: with ratings now kept as a whole number from 0 to 10, the web's two rating controls were still sending stars. A half-star rating (4.5) was refused by the server, and a whole-star rating was stored as half its value (five stars became 5 out of 10, half of what the person meant). search-sort's report left this to "client lanes"; it could not be left on a base people will run. The title page's rating dialog and the after-play stars now convert at the edge (two points to a star) and display the stored value as stars. `tsc` clean; the web's post-play and detail tests pass (8).

## Native merges (main-native) — DONE

Commits: `1ef6936` audio-engines, `2357f5d` freshness-clients, `6c3d60b` shared packages refreshed from main-server, `1696b6f` fixes.

**5a. audio-engines.** Conflicts: `PorticoPlaybackController.kt` and four files under `packages/` (taken from the base and then replaced wholesale by the package refresh, as the brief says).

`PorticoPlaybackController.kt` — the other side is the native playback work, which had folded "build a player" into one function used for both the main player and the crossfade's second player (transport-stream seek search, screen-size cap, display frame-rate handling, bandwidth measurement). The lane builds each by hand in order to add its gain stage. **Followed the base's single builder and carried the lane's gain stage into it:** a player built for listening gets the gain processor and a sound-only output (so normalization can raise a quiet track, and never bypasses gain through passthrough); a player built for a title with a picture keeps the ordinary output. The crossfade's second player is built by the same function and starts silent through the gain stage. The main player keeps announcing itself to the system exactly as the base had it. The lane's rules for when a player must be discarded (entering listening with a picture player; leaving listening) merged cleanly and are unchanged; leaving listening now also clears the base's "this player came from a crossfade" flag. **Not compiled** — there is no Android build here; I proof-read the result. Test note: build the app, play an album with normalization on and a 6-second crossfade across three tracks, then start a film. Pass: no jump in level at each join, the film plays with passthrough available as before. Fail: a compile error in `build(...)`, silence after a crossfade, or a film playing through the music output.

**5b. freshness-clients.** Merged without conflicts.

**Shared packages.** `shared-packages.mjs --write` from main-server, committed; `--check` reports identical (checked again after the last commit).

**6. Type-checks and lint.** `tsc --noEmit` and `tsc -p test/tsconfig.json --noEmit` were clean in `apps/native` and `apps/vega` straight after the refresh. Type-checking cannot see a change of units on the wire, so I looked for what the refreshed packages broke that it would miss:
- **Offline viewing was posted in seconds; the server now takes milliseconds and refuses the old field.** `offline-downloads.ts` converts on the way out with the shared helper. The native test and its stand-in server were updated to the real shape (39 offline tests pass).
- **Star ratings**, as on the web: the title screen's rating sheet now converts between stars and the 0–10 value. Vega shares this screen.
- **`npm run lint:strings`** reported one finding in `screens/Downloads.tsx` (an expression the lint reads as text; it predates these merges and freshness-clients' report mentions it). Hoisted the expression into a named value; native reports 0 over baseline, Vega 0.
- Checked and found fine: native reads of library content and episodes go through the shared parser (which converts units); the duration filter keeps its label through the server's unchanged label key.

Results: native `tsc` (app and tests) clean; Vega `tsc` (app and tests) clean; native and Vega `lint:strings` clean; native test files for downloads, offline, listening and titles: 50 run, pass.

## Noticed, wrong, and out of scope

- **Restriction matrices** (`httpapi`) fail on the base and list more routes after merge 3 (see that section). Needs someone to classify about 30 routes, some with real probes.
- **`server/api/openapi.yaml`** (hand-written) contradicts the generated contract in 23 places after the unit change. Delete or regenerate.
- **Client-core tests**: 34 fail on the merged base. 25 arrive with search-sort (fixtures still in the old units); the other 9 were already failing.
- **One time convention is not yet one.** New routes from other lanes still answer in other forms: catalogue failures (`failedAt`, epoch milliseconds), download receipt `viewing.positionSeconds`, plus the telemetry, subtitle, Watch Together and legacy playback shapes search-sort listed itself.
- **`go build -tags devtrust`** does not compile (`dev_e2e.go` uses a removed `loopbackPeer`). Present before these merges.
- **`TestProductionStatementsPrepareAgainstTheBaseline`**: `livechannels/programme_spool.go` reads a table `programmes` the baseline does not have. Present before these merges; looks like a real runtime failure in the guide spool, not just a test.
- **Genre/series/category groupings refuse a non-title sort** (400) since merge 3. A product choice worth making deliberately.
- The other tests listed as "failing before" in each section above.

## Not finished

- Nothing was run in a browser, on a device, or under Gradle/Xcode. The Android controller resolution is uncompiled.
- The FLAC deep-seek dispatch is unit-tested with a synthetic stream only; the repository has no long audio fixture.
- I did not run whole suites (by instruction). "Failing before / after" comparisons cover the packages each merge touched, not the whole tree.
- The 25 stale client-core fixtures, the restriction matrices and the hand-written OpenAPI file are left as described.
