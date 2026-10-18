# perf-followups — show re-matching on new episodes (F7) and the availability fallback

Branch `audit-implementation/perf-followups`, worktree `server-perf-followups`. Two commits:
- `Keep a matched or in-flight show's match when a scan adds its episodes`
- `Judge a file no scan knows by the library folder that holds it`

## 1. Outcome

- **A new episode or file now causes only the work it needs.**
  - If the show is already matched, it is not matched again. Only the new episode's own details are fetched, through the work row the episode gets when it is created.
  - If the show's match is in flight (claimed), that work is kept: the result is published, not discarded. The show's dispatch then walks every episode, so the new ones are picked up (and any that were claimed early and parked as `needs_parent_match` are woken).
  - If the show is not yet matched, nothing changes: a new episode wakes it as before.
- **Files no scan knows** (recordings, files added outside a scan) are now judged by the library folder whose path holds them, not always by the library's first folder. A second folder going offline now makes its files unavailable.

Tests (each fails on the base code, checked by stashing the change):
- `metadata.TestNewEpisodesDoNotRematchAMatchedShow`: a matched show gets 9 new episodes in 3 pages, with the matcher stepping between episodes and settling between pages, and the clock moved past every freshness window. It asserts one show publication in total, no change to the show's generation, no extra show request, and exactly one provider request for each new episode. On the base code: 1 → 10 show publications and generation 10 → 28.
- `metadata.TestInFlightShowMatchSurvivesNewEpisodes`: the show's search is held at the provider while two episodes are added; the result is then published (`pending_children`, generation unchanged), the dispatch matches all 6 episodes, there is no second show request, and each new episode is fetched once. On the base code the result is discarded. With the new triggers but without the new publication fence, it is still discarded (`stale_result_discarded`), which shows both parts are needed.
- `metadata.TestNewEpisodeStillWakesAShowNotYetMatched`: an unmatched show is still woken.
- `compactcatalog.TestUninventoriedFilesFollowTheirOwnFolder`: three folders (one a Windows-style `D:\Films\` root) and a file in no folder (`/mx/…`, which must not count as inside `/m`). Offline, root-changed and removed folders are checked through the view and the derived `catalog_item_availability`. On the base code a second folder going offline left its file available.

## 2. Decisions I made

- **What "matched" means for a show.** The work row has an accepted publication and status `matched` or `pending_children`. Any other status (including `unmatched`, `needs_selection`, `unavailable`, or `pending` with an earlier identity) counts as not yet matched and is woken exactly as before.
- **What "claimed" means.** `lease<>''`, not `lease_until` compared with SQLite's clock. Every path that ends a claim clears the lease. An abandoned lease (crashed worker) keeps the row's claimable status, so the claim is taken again when it expires and sees the new episodes. This also avoids the test-clock disagreement the perf-ingest report warned about.
- **The publication fence for shows (new).** Without this, keeping the lease was not enough: a show's input digest covers every episode file and the TVDB source/hierarchy revisions, all of which move when an episode is added, so the in-flight result was still thrown away at publish time.
  - A show claim now records the highest file id at claim time (`screenClaim.Bound`). `screenCurrent` compares a fence that covers only those files (paths, sizes, availability, names, NFO, runtime) and leaves out the two revisions.
  - Removing or changing any of the claimed files still moves the digest or the generation, so those results are still discarded.
  - Films and episodes are fenced exactly as before.
  - The stored input digest (candidates, owner selection) is unchanged in kind.
- **Source health in the matcher's digest** is now reduced to what matters to it: offline, removing, root changed, or readable. Before, a scan starting or finishing (`inventory_running` ↔ `ready`) was enough to discard an in-flight result or make an owner's pick among candidates stale.
- **Scope of the trigger change.** It covers the two triggers the brief named, plus the same show-side reset in:
  - `screen_wake_video_names_INSERT`: the analysis of each new file, which would otherwise undo the fix on real scans with probing.
  - `screen_wake_video_nfo_evidence_INSERT`: for a matched or claimed show, it now wakes the show only when the new file's NFO is show-level and its digest is new among the show's files. Every episode carrying a copy of `tvshow.nfo` no longer re-matches the show, but a new show-level NFO still does.
  - All item-side resets, and every UPDATE and DELETE trigger, are unchanged.
- **An unmatched show whose search is in flight** is not woken by new episodes. It keeps its in-flight result, as the brief asks for claimed shows.
- **Availability fallback rule.**
  - The folder whose `root` or `configured_root` is a path prefix of the file decides. `/` and `\` are both accepted as separators, and trailing separators are ignored. If any folder holding the file is offline or disabled, the file is unavailable.
  - A file in no folder of the library is unavailable only when every folder is. In a single-folder library this is exactly the old behaviour.
  - No trigger was changed: a folder crossing the offline boundary already re-checks its library through the existing bounded fan-out, so no whole-library work was added. The extra subqueries read only that library's few `library_sources` rows, through the `(library_id, root)` unique index.

## 3. Bugs

No `B-…` IDs are assigned to this lane. F7 and the availability-view observation from `perf-ingest.md` are both fixed.

## 4. API changes

None.

## 5. Schema changes

`0001_baseline.sql` (`scripts/schema-sync.py` run; no other generated file changed):
- Triggers `screen_episode_create`, `screen_wake_item_assets_INSERT`, `screen_wake_video_names_INSERT` and `screen_wake_video_nfo_evidence_INSERT`: the show reset is a separate statement guarded as above. Item resets are unchanged.
- View `inventory_item_availability`: the fallback branch now uses folder-by-path.

No derivation version bump: no derived content changes for existing data except availability, which is re-derived through the existing triggers.

## 6. Owner or viewer settings added or wired

None.

## 7. Needed from others

- **Architecture docs:** the matching rules. A new episode does not re-match a matched or claimed show. A show claim is fenced to the files it was claimed with. The availability rule for files no scan knows.

## 8. Test notes for other environments

Host ehler-personal (Ubuntu amd64), `GOFLAGS=-p=2`. Run:
```
go test ./internal/metadata -run 'TestNewEpisodesDoNotRematchAMatchedShow|TestInFlightShowMatchSurvivesNewEpisodes|TestNewEpisodeStillWakesAShowNotYetMatched' -v
go test ./internal/compactcatalog -run TestUninventoriedFilesFollowTheirOwnFolder -v
```
Pass is all `ok`. On Windows, the availability test's `D:\Films\` folder exercises backslash roots.

Real-device check worth doing once: with TMDB on, add a season of episodes to a matched show in a large library. The show must not reappear as "searching" or be re-matched (its publication history gains no entry), and each new episode gains its title once.

## 9. Not finished

- **A narrower race on episodes** (not shows): a new file's analysis (`inventory_video_names` insert) still resets the episode's own work row. If that episode's fetch is in flight at that moment, the result is discarded and fetched again. If the episode was already matched, the second pass is served from the remembered document inside its freshness window, with no provider request. I left item-side resets alone because film matching relies on them, and episode inputs include file names.
- **The dispatch** (`dispatchScreenChildren`) still resets episodes without checking their lease. This is a pre-existing race and was not changed.
- **No real-scanner benchmark was run.** The fix is proven at tier level with the catalogue write path and the real matcher.

Checks: see the end of this file.

## Checks run

`go build ./...`, `GOOS=windows` and `GOOS=darwin` builds: pass. `go vet` on metadata, compactcatalog, persistence: pass. `go test` (count=1): metadata, compactcatalog, persistence, ingestion, catalog — all pass. Route guards `go test ./internal/httpapi -run 'Scope|ProductionRouter|Registry'`: pass. Scratch Go cache deleted.
