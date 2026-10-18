# cp3-identity — library identity, metadata and background work

Branch `audit-implementation/cp3-identity` in `server-cp3-identity` (portico-server) and `native-cp3-identity` (portico-react-native). `audit-implementation/main` was merged into both, last at server `759b8f54` and native `1ccf5cb`. Contracts were regenerated (no diff) and the shared packages were re-mirrored from server `15c0dc37`. No subagents or helpers were used. Nothing was pushed.

## 1. Outcome

All ten items (A5–A12, A21, A24) are done. Both of Justin's 15 Oct additions are handled too:
- **Local-metadata people** now get a person page.
- **Missing-titles retention** already matched his decision; only the wording of the "Never" choice changed.

- **A5:** the visibility-class builder stops at its batch boundaries while the owner has paused background work. It also gives way briefly when someone is using the server.
- **A6:** the deep-analysis time limit now follows the file: 10 minutes, plus the longer of one pass over its bytes at 8 MiB/s or 3× its running time. It is capped at 24 hours. It used to be a fixed 6 hours.
- **A7:** the owner's show merge moves episodes 100 per write transaction. Each step re-checks permission and re-reads both shows. Seasons and the show itself follow in the last step.
- **A8:** a merged-away title id now leads to the title that stayed. This works in:
  - every id lookup (`entityid.Resolve`);
  - personal-state lookups;
  - the path ids of the title routes (items, shows, seasons, albums, artists, books, containers, admin items, and any `{itemId}`), through a new apikit path-rewrite hook.
- **A9:** removing a library no longer leaves a hidden, empty library row.
  - A container that something still refers to has those references cleared and is removed on a second try.
  - Rows that point at the library through a foreign key that blocks deletion are removed before the library itself.
- **A10:** albums, artists and books are now joined automatically by a provider identity Portico is sure of:
  - **Albums:** a published MusicBrainz release joins every album in the library with that release. Tracks at the same place, with the same name, become versions of one track (this uses main's `mergeContainerTx`).
  - **Artists:** a release with exactly one credited artist joins artists with that MusicBrainz artist id.
  - **Books:** an ISBN or ASIN in an audiobook's tags joins books.
  - **Tracks:** they now carry their MusicBrainz recording identity, so kept watch state can be restored. Tracks are never joined by it.
  - **Owner split:** "Not this album… / Not this book…" (web, phone and tablet) moves every file in one folder to an album or book of its own. Identity never joins it back automatically.
- **A11:** music and audiobooks now have a needs-attention list on the server, web and phone/tablet. It lists:
  - albums MusicBrainz could not settle;
  - albums and tracks with a release or recording to choose;
  - books with no author.

  They share the film/TV routes, page order and per-library counts. Entries can be dismissed, and albums and tracks can be looked up again. The badge and tab now show for music and audiobook libraries.
- **A12:** `title.updated` now fires when only a show's, album's or book's own details change (overview, tags and so on, or its artwork).
- **A21:** two metadata-matching races are fixed:
  - When a file's analysis lands, it no longer throws away an episode fetch that is already running. An episode's file tags are no longer part of its matching fence.
  - The show's dispatch no longer resets episodes whose fetch is running. Those fetches are still checked against the show's publication when they finish.
- **A24:** the owner's recording notices (no tuner free, storage full, waiting for a tuner, started late, failed because of live viewing) now come from the string catalogue on web, phone, tablet and TV. Other notices keep the server's words.
- **Justin addition 1 — local-metadata people:**
  - People named only in local metadata (an NFO credit with no provider id) get a `name:` key and a person page. This is the key a show's credits already use.
  - When the provider later credits the same name on the same title, the two become one person. Either the local person takes on the provider's identity, or their credits move to the existing provider person.
  - Their old person-page id still works, and later local credits find them.
- **Justin addition 2 — missing titles:** already as he decided:
  - a server setting with Right away / After a number of days (30 by default) / Never;
  - the owner can empty them by hand on web and native.

  The only change: the choice now reads "Never (keep until I remove them)".

## 2. Decisions I made

- **What counts as a confident identity for A10:**
  - MusicBrainz release for albums;
  - the release's single credited artist for artists (a compilation with several credited artists joins nobody);
  - ISBN/ASIN tags for books.

  Joining by recording would wrongly merge compilations, so it is not done.
- **The album join runs in its own transaction** after the MusicBrainz publication commits. Moving tracks would otherwise trip the publication's own fence and loop. After a join, the album's job matches once more.
- **The owner split works by folder** ("these files are not this album"), not by undoing a merge. A separated container goes into a new `catalog_kept_apart` table, which automatic joining skips. Scans keep a track on a separated album.
- **A8 rewrites only the title routes' path ids.** Ids sent in request bodies resolve through `entityid.Resolve` and the personal lookup. The roughly 380 places that resolve ids in SQL with `pid_blob` still give not-found for a merged-away id.
- **A11 lists tracks only when a choice is needed.** Unresolved tracks would flood the list. Unavailable (the provider is down) is left off because it fixes itself.
- **A6 throughput constants:** 8 MiB/s, 3× running time, 10-minute floor, 24-hour ceiling.
- **Local people use the existing `name:` + `lower(trim(name))` key** (ASCII lowercase only, matching SQLite), so items and shows name the same person. A provider person is adopted only on evidence from the same title.

## 3. Audit items closed

| Item | What changed | Test |
|---|---|---|
| A5 | `visibilityBoundary` (PaceBackground) between batches in rebuild, refresh and clear; WaitBackground between classes | `catalog.TestVisibilityBuilderHonoursBackgroundPause` |
| A6 | `deepAnalysisBudget(size,duration)` | `ingestion.TestDeepAnalysisBudgetFollowsTheFile` |
| A7 | `mergeShowChildrenTx(limit)`, batched `MergeTitles` | `catalog.TestShowMergeRunsInBatchesAndRedirectsOldIds` (230 + 40 episodes) |
| A8 | `catalog_merged_ids`, Resolve/personalID fallback, apikit `RewritePathValues`, `SurvivingTitleID` | same test; `apikit.TestPathRewriteReachesTypedAndHTTPHandlers` |
| A9 | `clearReferencesTx` (schema-driven, cached) | `catalog.TestRemovedLibraryOfEveryKindLeavesNoHiddenRow` (tv, music, audiobook) |
| A10 | release/artist/recording identities in `mb_publish.go`; `joinBookByIdentifierTx`; `KeepApartTx`; `SeparateFolder` (inventory action `separate-folder`) | `catalog.TestAlbumsJoinByReleaseAndOwnerCanSeparateAFolder`, `TestBooksJoinByIdentifier` |
| A11 | `listening_metadata_attention` + triggers into the shared counts; listing union; `dismiss` and `retry` actions; client kinds `album`/`song`/`book` | `metadata.TestMusicAndBooksNeedAttention`; client-core `attention-words` test updated |
| A12 | 5 container-detail triggers | `webhooks.TestEveryEventTriggerRunsAndQueuesItsEvent` (extended) |
| A21 | episode fence without tags; analysis trigger skips a leased episode; dispatch skips leased episodes | `metadata.TestEpisodeAnalysisDoesNotResetAnInFlightFetch`; existing show-wake tests pass |
| A24 | client-core `noticeWords` + `notice.recordingConflict.*` keys; web inbox and native Notifications use it | client-core `notice-words.test.ts` |
| Addition 1 | `LocalPersonKey`, `AdoptLocalPersonTx`, `catalog_people_aliases`, person-id rewrite, show-people SQL follows aliases | `catalog.TestLocalPeopleHavePagesAndJoinTheProviderPerson` |

## 4. API changes

- **apikit:**
  - new `Registry.RewritePathValues(PathRewrite)`;
  - `ServeHTTP` now sets `q.Pattern` before the path values.
- **Inventory action:** `POST /v1/admin/libraries/{id}/inventory/{object}/separate-folder` returns `{itemId}` (the new album or book). It uses the existing typed route with its `{action}` parameter.
- **Needs-attention (same routes):**
  - entry `kind` may now be `album`, `song` or `book`;
  - new action `retry`;
  - commands accept those kinds.
- **Path ids:** a merged-away title id in a title route's path, and an old person-page id in `/v1/people/{id}…`, answer as the survivor.
- **Generated contracts:** apigen produced no diff.

## 5. Schema changes

All in the `0001_baseline.sql` baseline; `schema-sync.py` was run.

- **New tables:**
  - `catalog_merged_ids`
  - `catalog_kept_apart`
  - `catalog_people_aliases`
  - `listening_metadata_attention` (with its indexes and count, retire, music and book triggers)
- **New webhook triggers:**
  - `webhook_show_details_updated`
  - `webhook_album_details_updated`
  - `webhook_book_details_updated`
  - `webhook_container_art_updated`
  - `webhook_container_art_added`
- **Changed trigger:** `screen_wake_video_names_INSERT` now skips a leased episode.

## 6. Settings added

None. The only wording change is the "Never" choice for missing titles.

## 7. Tests run

- **Go:** the focused tests named in §3 all pass. Broader runs also pass:
  - `go test ./internal/catalog -run 'Merge|Separate|Audio|Album|Book|Join|Container|LocalPeople|Removed|Visibility'`
  - `./internal/metadata -run 'MB|MusicBrainz|Music|Credit|People|NFO|Local|Attention|ShowMatch|NewEpisode'`
  - `./internal/httpapi -run 'Attention|Merge|Inventory|Person|People'`
  - `./internal/webhooks -run TestEveryEventTrigger`
  - `apikit ./...`
  - `persistence` (through `schema-sync.py`)
- **Builds and checks:**
  - `go build ./...`, gofmt and vet on the touched packages;
  - apigen produced no diff.
- **Web:**
  - `tsc --noEmit` passes;
  - `lint:hooks` passes;
  - `lint:strings` passes (0 hard-coded strings, raw errors clean, glossary passed).
- **Client-core:** the notice-words, attention-words, metadata-attention and attention tests pass.
- **Native:**
  - `tsc --noEmit` passes;
  - `lint:hooks` passes;
  - the `owner-rules` test passes (30/30);
  - `shared-packages --check` matches the recorded commit.
- **Not run:** whole suites, end-to-end drivers, emulators and Vega checks (per tonight's brief).

## 8. Device test notes

- **Music library, two folders of one release** (FLAC and MP3, different album tags) with MusicBrainz on:
  - they become one album and each track shows two versions;
  - on a track, the file tools offer "Not this album…"; using it gives the folder its own album, and a rescan keeps it there.
- **Audiobooks, two copies with the same ISBN tag:** they become one book. "Not this book…" separates them again.
- **Owner › Libraries › a music or audiobook library:**
  - the Needs attention tab and badge appear;
  - "Look it up again" works on an album that could not be settled.
- **Merge two large shows (thousands of episodes) while browsing:**
  - the rest of the UI keeps answering;
  - an old link to the merged-away show opens the show that stayed.
- **Pause background work:** visibility rebuilds (restricted profiles' library views) stop and resume.
- **An NFO-only actor:** their name links to a person page. After TMDB matches the film, the same page shows the TMDB person, and the old link still works.
- **DVR conflict notices:** shown in the viewer's language on web, phone and TV.

## 9. Not finished

- **A8 coverage:** ids resolved directly in SQL (`pid_blob`, about 380 places) do not follow a merged-away id, for example in playback-start bodies. Path ids and Resolve-based lookups do.
- **A10 owner split on TVs:** none (owner tools are phone/tablet only).
- **A10 artist split:** there is no "Not this artist"; the owner fixes it through album metadata.
- **A10 split size:** `SeparateFolder` handles at most 4,000 files of one container per step.
- **A11 actions:** no MusicBrainz candidate picking inside the list. The entry links to the album page, which has the existing MusicBrainz review.
- **Local people portraits:** none unless artwork already exists for that identity key. A show-level NFO cast still isn't listed on person pages, which read item credits only (this was already true before tonight).
- **A7 interrupted merges:** a show merge stopped between steps leaves both shows partly merged. Running the merge again finishes it.
- **A6** is a size and duration budget, not a progress signal.

## Commits

**portico-server** (`audit-implementation/cp3-identity`):
- `2e44e174` A5
- `21e63c0e` A6
- `bd5f87b0` A7+A8
- `5eefeafd` format
- `f01a2a42` A9
- `c36a838f` A12
- `dbcad276` A21
- `b15bcdae` A24
- `9139f4c9` A11 server
- `579f7f79` A11 web
- `ba7cd59e` A10 server
- `577b2b8f` A10 web
- `37d4deeb` local people
- `17660704` format
- merges: `3a411392`, `f3e5e542`, `26601168`, `15c0dc37`

**portico-react-native** (`audit-implementation/cp3-identity`):
- `7a73ed2` A11/A24 native
- `c146d1d` A10 native
- merges and mirrors: `8ad0477`, `1f6b1e7`, `bae107a` (packages from server `15c0dc37`)
