# Lane report: identity — titles, versions, missing files, deleting, removing a library

Worktree `server-identity`, branch `audit-implementation/identity`, 9 commits on top of `0e03d99` (`c7873bc` … `7f4c9b7`). Nothing pushed. The server builds for Linux, Windows and macOS; the web app and client-core type-check.

## 1. Outcome

**A title is separate from its files.**
- A film, episode, track or audiobook part is found by what it is, not by where its file sits. A second file that is the same title becomes another version of it. Replacing a file with a better encode under a new name keeps the title, its id and everyone's state.
- "The same title" means, in order: the same provider identity; otherwise the same title and year (films), the same place in its show (episodes), or the same artist, album, disc, track and title from tags (music and audiobook parts).
- A show is the folder its episodes sit in. Different years or spellings in file names inside one folder no longer split it, and two folders with the same title no longer merge on title and year alone.
- When a provider match gives two titles of one library the same identity, they become one title: files become versions, a show takes the other's seasons and episodes, and every profile's state is combined.
- The owner can also merge two titles by hand, and can split one file off a title it was wrongly grouped with.
- Renaming a file re-reads its name (it used to keep the old reading).

**Versions.** Each title lists its files best first, with resolution, codecs, dynamic range, audio tracks, size, part number and an edition label where known. Playback can name a version; with none named the server plays the best one that is on disk. A title with several versions can be downloaded (it was refused before).

**Missing files.**
- A file that a complete scan of its folder no longer finds is marked missing at once and stops being offered or played. No second scan, no waiting period.
- A title left with no file stays in the library, marked missing with the time, shown as unavailable. A file that is the same title brings it back.
- The owner chooses when missing titles are removed: immediately, after a number of days (default 30), or never. There is an "empty now" action.
- A folder that cannot be read, or is suddenly empty where it held media, marks nothing missing: the scan stops as "source unavailable".

**Deleting.** Deleting from Portico deletes the file on disk. The trash folder in the state directory, its three retention settings, restore, empty and its recovery journal are gone. The preview and typed confirmation stay.

**State that outlives removal.** When a title is finally removed, each profile's watched mark, progress, rating, favourite, My List flag and play count are kept and given back when the same title is added again, to the same library or another.

**Removing a library.** `DELETE /v1/admin/libraries/{id}`. The library disappears at once and is taken apart in the background. Files on disk are not touched and personal state is kept as above. Removing one folder from a library marks its files missing.

**Device numbers (B-05.18).** A library folder is recognised as the same folder when only its device number changed after a restart, and files are no longer treated as replaced because their device or inode number changed.

### What was run
- New tests, all passing: `internal/catalog/title_identity_test.go` (13 tests), `internal/administration/deletion_test.go` (4), `internal/storage/stable_identity_test.go` (2), `internal/ingestion/renumbered_identity_test.go` (1), `internal/httpapi/title_routes_test.go` (1), `packages/client-core/test/title-identity.test.ts` (5).
- Not covered by a test: the `library-removal` and `folder-removal` jobs running under the real scheduler (their batch functions are tested directly), and merging albums or audiobook parts through a real tag scan.
- Whole-package runs, passing: `compactcatalog`, `ingestion`, `storage`, `administration`, `persistence` (including the statement-scan guards), `playback`, `playbackv1`, `downloads`, `operations` settings tests, `metadata` screen tests.
- `catalog`: passes except `TestDiscoverCarriesPicksForYou`, which fails the same way on the untouched base commit.
- `httpapi`: passes except `TestListeningHTTPPreferenceEnvelopeAndCAS` ("explicit reviewed helper binary required"), which I did not run on the base commit; its message is about a missing helper binary on this machine, not about my routes.
- Not run: the full repository suites, the native apps, anything on Windows or macOS beyond compiling.

## 2. Decisions I made

1. **Missing is immediate; the per-folder "mark missing after" setting is removed.** Nothing destructive follows from the mark any more, and a dead file must not stay playable for a day. It is gone from the schema, API, client-core and the folder dialog.
2. **A show is its folder within one source.** Two folders of the same show on two disks are two shows until a provider matches both (then they merge automatically) or the owner merges them. In a library with local metadata only they stay separate until merged by hand. I chose this over merging by folder name because a wrong merge cannot be undone by matching, and a wrong split can.
3. **Films with and without a year are different titles until matched** ("Alien" and "Alien (1979)"). A provider match merges them.
4. **Automatic merge on provider identity** happens when a match is published. The title being matched keeps its id; the other's id stops resolving (no redirect is kept).
5. **Bulk delete job removed.** The selection bar's Delete used a job with no preview or confirmation, which was defensible with a recoverable trash and is not with deletion from disk. A selection now goes through the same preview and typed count, 200 titles at a time.
6. **`DELETE /v1/items/{id}` removed** (B-07.11). The owner delete routes are the only way.
7. **"Delete but keep the files" stays** (`deleteFiles: false`): the title goes, the next scan finds the files again and state comes back.
8. **A file that is no longer the catalogued one is never deleted.** Size and modified time must match what the library recorded, and the path must be under one of the library's folders. Otherwise the title stays, with `code: file_changed`.
9. **Deleting a show, season, album, artist or book** now deletes everything beneath it, up to 5,000 titles per request (it failed with a database error before).
10. **Kept state is bounded** at 100,000 removed titles per profile, oldest dropped first. No time limit.
11. **Identity ledger rows leave with the title** when it is finally removed, so the ledger no longer grows for ever (part of B-07.15).
12. **An accepted new root keeps its files** when relative path, size and modified time match (copying a library to a new disk no longer re-analyses everything).
13. **Tracks merge by tags only when tags are specific**: artist, album, track number and title all present, and not ripper placeholders such as "Unknown Artist".
14. **A removed library's row may stay as a hidden, empty row** if derived tables still reference it (see section 9). Its folders are freed either way.
15. **Contract fixtures re-recorded**: `server/testdata/contracts/*maintenance-settings.json` were already stale against the base code (they listed a "backups" retention the code no longer has); re-recording dropped that along with trash.

## 3. Bugs

| Bug | Status |
|---|---|
| B-05.8 second file is a second movie; upgrade loses state | Fixed (films, tracks, parts). |
| B-05.9 renaming does not change the reading | Fixed for films and episodes. Audio is identified by tags, not names. |
| B-05.18 device number in file identity | Fixed and tested by rewriting the recorded numbers and rescanning; not exercised on a real NFS/SMB/FUSE remount (section 8). |
| B-05.24 deleted files stay available for a day, never removed | Fixed. |
| B-05.25 / B-09.13 delete copies into the state folder | Fixed: the trash no longer exists. |
| B-05.27 show identity is title and year | Fixed. |
| B-07.3 new file at a moved file's old path joins the moved title | Fixed. |
| B-07.11 raw item delete route | Fixed: route removed. |
| B-07.15 insert-only tables | Partly: the identity ledger is pruned. People, terms and labels are not mine (freshness lane). |
| B-12.18 state lost on rename or replace | Fixed. |
| B-12.19 show's Next Up anchor dangles | Fixed for removal: the marker moves to the nearest remaining episode. |
| B-18.5 a library cannot be removed | Fixed. |
| B-18.13 bulk delete without confirmation | Fixed by decision 5. |
| B-09.18 backups only on the database volume | Not fixed: belongs to ops-packaging (my brief cites it only next to the trash). |
| B-07.1, .2, .4, .5 derived workers, rebuilds, revisions, search snapshot | Not fixed: freshness-server lane. |
| B-07.6, .7, .8, .9, .10, .12, .17 search and filter behaviour | Not fixed: outside this lane. |
| B-07.13, .14, .16, .18, .19 backup snapshot, migration rules, gate alarms | Not fixed: outside this lane. |
| Other B-05.x (scan loop, naming) and B-12.x (personal state rules) | Not mine: naming, scan-loop and social lanes. |

## 4. API changes

**New (typed registry, owner only)**
- `GET /v1/admin/missing-titles?libraryId&cursor&limit` → `{items:[{id,libraryId,kind,title,parentTitle?,year?,missingSince,removedAt?}],nextCursor?,total,rule,days}`.
- `POST /v1/admin/missing-titles/empty` `{libraryId?}` → `{titles,files,more}`; repeat while `more`.
- `POST /v1/admin/titles/merge` `{fromId,intoId}` → `{itemId}`.
- `DELETE /v1/admin/libraries/{id}` `{confirmation,expectedRevision}` → 202 `{libraryId,removing:true}`. Errors `confirmation_mismatch` (409, new code), `revision_mismatch` (412).
- `POST /v1/admin/libraries/{id}/inventory/{object}/separate` → 200 `{itemId}`.

**Changed**
- Delete preview: `trashRetentionDays` removed from the preview and each target; each target gains `titles`.
- Delete result: `bytesTrashed` → `bytesDeleted`; receipts lose `trashEntryId`, `filesMoved` → `filesDeleted`, `code` is `file_changed` or `file_not_removed`.
- Item detail: `missingSince` added; each entry of `files[]` gains `partIndex` and `edition`, and the list is best version first.
- Playback options: `versions[]` is best first and `edition` is per version.
- Library settings: `trashRetentionDays` removed. Folder settings: `missingGraceSeconds` removed.
- Inventory listing: states `trashed` and `missing_candidate` no longer occur.
- Storage usage: the `trash` category and `entries` are gone. Maintenance retention: the `trash` key is gone.
- Console job kinds: `catalog-trash-cleanup` replaced by `missing-titles-cleanup`; new `library-removal` and `folder-removal`.

**Removed**
- `GET /v1/admin/trash`, `POST /v1/admin/trash/{id}/restore`, `POST /v1/admin/trash/empty`.
- `DELETE /v1/items/{id}`.
- Inventory actions `trash`, `restore`, `forget`.
- Bulk job command `trash`.

Specs updated: `server/api/openapi.yaml`, `admin-libraries.openapi.yaml`, `jobs.openapi.yaml`, `admin-libraries.md`, the generated `portico.openapi.json` and `packages/contracts`.

## 5. Schema changes (all in `0001_baseline.sql`; `schema-sync.py` run)

- `catalog_identities`: primary key is now `(root, source_key)`; `public_id` is no longer unique (a title has several names); new index `catalog_identities_public`.
- `catalog_asset_links`: new column `edition`.
- New `catalog_missing_titles(entity_id, library_id, missing_since_ms)`.
- New `retained_title_state` and `retained_title_identities`.
- Removed `admin_trash`, its three indexes and three triggers; `inventory_item_availability` no longer reads it.
- Removed `inventory_objects.trashed_at` and `library_sources.missing_grace_seconds`.
- Any existing development database must be recreated (the baseline changed).

## 6. Owner or viewer settings

| Name | Type | Default | Takes effect | Screen |
|---|---|---|---|---|
| `library.missingTitles` | `immediately` / `after_days` / `never` | `after_days` | `catalog.EmptyMissingDue`, run after library changes and at least every five minutes | Yes: Settings › Storage & backups › Missing titles (web) |
| `library.missingTitlesDays` | integer 1–3650 | 30 | same | Yes, shown when the rule is "after a number of days" |

Removed: `library.trashRetentionDays`, per-library `trashRetentionDays`, maintenance retention `trash`, per-folder `missingGraceSeconds`.

## 7. Needed from others

**naming lane**
- Fill `catalog.MovieFile.Edition`, `.Part` and `.Provider` (name hints such as `{edition-…}`, `cd1`, `{tmdb-…}`) where `commitMovieItem` and `reinterpretInventoryTx` build it. Today they are empty, so no edition labels come from file names.
- `resolveShowTx` (`catalog/attach.go`) decides the show folder itself with a small season-folder pattern; replace `showFolder` with the new reader's answer.

**providers-matching lane**
- The merge hook is one call at the end of `publishScreenLinks` (`metadata/screen_links.go`). Keep it when that file is rewritten: `catalog.AdoptProviderIdentityTx(ctx, tx, entity, identity)`.
- After `reinterpretMovieTx` renames a title in place, nothing asks for a rematch.
- Music has no equivalent hook yet (MusicBrainz recording ids are used only for kept state).

**music-audiobooks lane**
- The attach point for audio is `catalog.AttachTrackTx` and `catalog.IdentifyTrackTx`, called from `commitAudio` (three small edits there; expect a merge conflict if that function was rewritten).

**scan-loop lane**
- `ingestion.reconcilePage` now only calls `catalog.SourceLooksUnmounted` and `catalog.MarkInventoryMissingTx`.
- An empty-folder stop reuses the existing `scan_source_unavailable` code; a clearer notice ("this folder is empty but held media before") needs its own code and string.

**playback-core lane**
- `compactcatalog.BestVersionOrder` picks by presence, height, then size. Choosing the version a given device plays directly is the planner's job.
- `playbackv1.versionsOf` still groups files by container, codec and height.

**downloads lane**
- Merged titles: download, social and session tables keyed by item id are not re-pointed (they have no index on it).

**console / web lanes (screens)**
- Version picker on the title page (data: detail `files[]`, playback options `versions[]`).
- Missing badge and "missing since" on title pages and cards (data: detail `missingSince`).
- Remove-library control with typed name (`removeLibrary` in client-core).
- Merge two titles, and split a file off a title (`mergeTitles`, `objectAction(…, 'separate')`).
- Delete dialog: show `titles` for a show or season, and the per-item `code` when a file was kept (strings `deleteMedia.titlesCovered`, `.fileChanged`, `.fileNotRemoved` are in the catalogue, unused).
- Native apps: every change in section 4 (not touched here).

**documents to correct**
- `Project Architecture/03 Catalogue and Metadata.md` and `Product Contracts/libraries-and-scanning/contract.md` (two scans plus grace; catalogue trash), `metadata/contract.md` ("ambiguous copies stay separate"), `02 Server.md` and the settings contract (state-folder trash).

## 8. Test notes for other environments

**Device renumbering (any Linux box with a network or FUSE mount)**
1. Add a library on an NFS, SMB or rclone mount; scan to completion with analysis on.
2. Unmount, mount something else first so the device number changes, remount, restart Portico, scan.
3. Pass: the scan completes, no "folder changed" notice, `inventory_changes` has no `replaced` rows, no file is re-analysed. Fail: the source shows as changed, or every file is probed again.
4. Repeat with a mount that does not keep inode numbers: pass is the same.

**Unmounted disk**
1. With a scanned library, unmount its disk so the mount point is an empty folder; scan.
2. Pass: the scan fails as source unavailable; nothing is listed under Missing titles. Fail: titles appear there.

**Delete on Windows and macOS**
1. Allow deleting on a library; delete a film with two versions; then a read-only file; then replace a file on disk without scanning and delete its title.
2. Pass: both versions leave the disk and no `trash` or `file-operations` folder appears in the state folder; the read-only file gives "couldn't be removed" and its title stays; the replaced file stays on disk with its title.

**Upgrade and re-add**
1. Watch a film; replace its file with one of another name and quality; scan.
2. Pass: one title, still watched, the new file plays, the old one is not offered.
3. Delete a watched film's file outside Portico, scan, press "Remove them now", put the file back, scan. Pass: the film returns watched.

**Remove a library** with a few thousand titles while something plays from another library: pass is that it vanishes from every client at once, the console job `library-removal` finishes, and playback elsewhere does not stall.

## 9. Not finished

- **Removed library row.** Derived per-library tables (visibility classes, browse buckets and similar) are not cleared by the removal job. When any still reference the library, its row stays as a hidden, empty row; nothing can reach it and the same folder can be added again. I did not check whether stale derived counts for that row are ever dropped.
- **Edition labels and split-film parts from file names** are plumbed but not read (naming lane, section 7).
- **Show merge runs in one transaction.** Merging a very large show (thousands of episodes) holds the write lock for the whole merge. Not measured.
- **No redirect from a merged-away title's id** to the one that remains.
- **Audiobook parts and music without usable tags** are still known only by path; a replacement under a new name is a new part or track.
- **Albums, artists and books** are not merged by provider identity.
- **Kept state does not include** per-play history rows or playlist positions (playlist entries of a removed title lose their pointer, as before).
- **Native apps and the remaining web screens** in section 7 are not built. The web has only the missing-titles list and rule, the delete dialog wording and the selection-bar change.
- **Pre-existing failing tests** left as found: `TestDiscoverCarriesPicksForYou` (catalog) and `TestListeningHTTPPreferenceEnvelopeAndCAS` (httpapi).
- An existing state folder's old `trash/` and `file-operations/` directories are no longer read; delete them by hand.
