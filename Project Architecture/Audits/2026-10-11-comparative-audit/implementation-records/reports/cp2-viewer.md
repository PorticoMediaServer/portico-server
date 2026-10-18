# cp2-viewer — viewer browsing and playback gaps

Execution: Ubuntu mini-1. Worktrees `server-cp2-viewer` (from main-server `6ae3ccf4`) and `native-cp2-viewer` (from main-native `ea762a7`), branch `audit-implementation/cp2-viewer` in both. `audit-implementation/main` is merged into both (server at `2281643a`). Nothing pushed. No subagents or helpers. `node_modules` are symlinks to `main-server` and to Justin's main checkouts (read only, ignored by Git). Testing was kept narrow at the coordinator's request: type-checks plus the tests that cover new logic.

## 1 Outcome

- **Pins work on every client.**
  - Viewers can pin and unpin libraries, and arrange the pinned ones.
  - Viewers can pin and unpin collections, saved views and playlists, and arrange those too.
  - Pinned libraries lead the library list in the viewer's order, with a rule before the unpinned ones.
  - Pinned collections, saved views and playlists get a **Pinned** section in the rail:
    - web sidebar;
    - wide tablet and TV rail, where they are focusable rail items that open in Saved;
    - on phones (web compact and native handheld) they lead the Saved screen instead.
  - **Edit pins** turns library pins on and off and moves them up or down, and moves or unpins saved items:
    - web: a pencil button next to the "Libraries" heading in the rail;
    - native (phone, tablet, TV, Vega): a button on Saved, which opens a sheet of plain focusable buttons.
  - **Pin / Unpin** is on each playlist, collection and saved-view page: in the page menu on web, as a hero action on native.
  - A pin change reaches the viewer's other devices through the feed. The server now emits a `personal.changed` hint, and the rail and library list re-read.
- **Episode groups play whole.** A flat show's non-season groups (its whole list, its specials, absolute-only episodes, ranges of 100):
  - pressing an episode sends the server a query selector (`episodeGroup` condition + `groupOrder`), on web, phone/tablet, TV and Vega;
  - the queue covers the whole group in the show's order, starting at the pressed episode;
  - the queue is named after the show.
- **Home rows play whole.**
  - The server resolves a `homeRow` condition through Home at the moment Play is pressed: the row's members, up to the row toolbar's cap of 500. It then plays exactly that, in the row's order or sorted/filtered as shown.
  - The queue is named after the row.
  - The Home row page (See all) has **Play** and **Shuffle** on web and native.
  - Song, episode and part rows on Home play on from the pressed one through the whole row: web cards; native list rows.
  - A sorted or filtered Home row now sends the server query. The client guard that forced "play from the screen" is gone on web and native.
- **Lists across libraries have a letter rail.**
  - `POST /v1/browse` publishes `positionIndex` (each letter and its first position) on the first page of any across-library list sorted by title.
  - Web shows the rail automatically: group grids use `EngineResults`.
  - Native shows it on the sorted My List, Favorites and History grid in Saved and on the sorted Home row page. On TV it appears as the header's "Jump to".
- **History across libraries runs past 10,000.** In its own order (most recent first, or oldest first), the history across libraries is read page by page:
  - from the maintained per-title last-played facts, through a new partial index;
  - the old 10,000-member pre-read is not used;
  - the total is exact to 10,000, then an open lower bound that grows as pages are read.
- **The list of playlists has a full toolbar.** This covers Saved › Playlists and the music library's Playlists tab, on web and native.
  - **Sort:** Name, Date created, Last changed, Number of items; each ascending or descending.
  - **Filter:** All, Mine, Shared with me, Pinned; each with a count.
  - When a filter leaves nothing, a "Show all" way back is offered.

## 2 Decisions I made

1. **Pin semantics.** Pinning a library moves it to the top of the list in the viewer's order. Unpinned libraries stay visible below a rule; nothing is hidden. This favours ease of use: a pin never makes a library harder to find.
2. **Where saved pins show.**
   - The rail on desktop web, wide tablet and TV.
   - The top of Saved on phones.
   - The rail order is the server's `saved_pin_order`. Pins never arranged follow, by name.
   - Anything deleted or no longer shared simply drops out of the list.
3. **One editor, reachable on TV.** Pin arranging uses up/down buttons rather than drag on every platform, so it works with a remote. Drag stays as the collection-order pattern only.
4. **GET `/v1/saved-pins/order` now lists names and pin revisions.** The rail can draw and unpin without a read per item. It is capped at the arrangement's 200.
5. **Home row snapshot.**
   - The HTTP queue routes (`create_queue`, `add_queue_segment`) resolve each `homeRow` that a query selector names, through Home, before the queue is built. The resolved members travel to the selector compiler in the request context.
   - Selectors without a Home row pay nothing.
   - **Known limit:** a very long row of shows can need a background continuation of the queue build. If the server restarts in the middle of that, the continuation cannot re-resolve the row: the queue keeps what was built and stops growing.
6. **Pivot for a whole Home row.**
   - A row of songs, episodes or parts plays those items (`items` pivot).
   - A row of titles plays each title's own list (`titles` pivot).
   - Plain movie and show rows on web Home still play only the pressed title, as Plex does. The whole-row Play is on the row page.
7. **Episode groups use the query selector, not a new container kind.** It already carries viewer restrictions and the show's group order. Its label is the show's title.
8. **Letter index across libraries is title sort only**, ascending or descending. Value buckets for other sorts were not added.
9. **History beyond 10,000 applies to its own order.**
   - Sorting the history by another field (title, year…) still sorts the 10,000 most recent, flagged as a lower bound, because sorting needs the full set.
   - Playing the history as a queue still covers the 10,000 most recent.
   - The new reader is built on the derived per-title facts (`catalog_personal_sort_rows`). These are filled by the background personal-sort jobs, so a just-played title can take a moment to rise to the top.
10. **Playlist directory groups** are "Mine", "Shared with me" and "Pinned". I added no "contains music/video" filter: it would need a new maintained per-playlist fact.
11. **"Last changed"** is a new `catalog_playlists.updated_at`. A trigger sets it whenever the playlist's revision, name or description changes.

## 3 Audit bugs closed and decision gaps

- **B-18.27 (remainder): library and saved-item pin controls — closed.**
  - `library-pins.ts` now has consumers on web, native phone/tablet, TV and Vega.
  - It gains `readPinOrder`, `savePinOrder`, `setResourcePin`, `read/saveLibraryNavigation`, `withLibraryPinned`, `movedPin` and `withPinRetry`. `withPinRetry` silently re-reads and retries once on a 409.
  - Tests: client-core `library-pins.test.ts` (names and pin revisions parsed, pin/move arithmetic, retry); Go `TestPinOrderAndPinnedLibraries` (named list, arranged first, deleted pins dropped).
- **Whole-group playback (cp-queues-playlists "Not done") — closed.**
  - Tests: `TestQueuePlaysAHomeRowSnapshot` (row order, sorted row, unresolved row refused); client-core `browse-groups.test.ts`; web and native `container-play.test.ts`.
- **Lists spanning libraries (cp-browse-parity "Not finished") — closed for the letter rail and the History cap.**
  - Tests: `TestBrowseAcrossLibraries` (letter index), `TestHistoryAcrossLibrariesRunsPastTheTotalsLimit` (own order, past the limit, deep range, filtered and oldest first).
- **Music Playlists tab sort and filter — closed.** Tests: `TestPlaylistDirectorySortsAndFilters` and client-core `saved.test.ts` (playlist groups accepted, others refused).

## 4 API changes

- `GET /v1/saved-pins/order`: each `order[]` entry now has `name` and `pinRevision`. It lists every pinned resource the viewer can still open, arranged ones first, capped at 200. The PUT is unchanged.
- `GET /v1/saved/content?view=playlists`:
  - `sort` is `title` | `created` | `updated` | `count`; the default direction is ascending for name and descending for the rest;
  - `filter` is `all` | `mine` | `shared` | `pinned`;
  - the envelope publishes these sorts (label keys `playlists.sort.*`) and a `filter` filter with counts.
- `POST /v1/browse` (across libraries):
  - `positionIndex` is filled on the first page under a title sort;
  - the history group in `groupOrder` pages beyond 10,000, with `totalIsLowerBound`.
- **Queue selectors** (`create_queue`, `add_queue_segment`): a `query` naming `homeRow` is now accepted. It is resolved through Home at request time. A sorted Home row and an `episodeGroup` with `groupOrder` already parsed; they now play.
- **Feed:** pin, pin-order and library-navigation changes emit `personal.changed` (`resource_id` `pins` or `library-navigation`).
- **Regeneration:** contracts, OpenAPI and `schemas.ts` regenerated with `apigen`. No routes were added or removed. Scopes are unchanged.
- **client-core additions:**
  - `Library.pinned`;
  - `PinnedResource`;
  - `PlaylistListFilter` / `playlistListFilters`;
  - `episodeGroupQuery` and `homeRowQuery` (`presentation/browse-groups.ts`).
- **Web/native `container-play`:** `episodeGroupSequence` and `homeRowSequence`.

## 5 Schema changes (baseline)

- `catalog_playlists.updated_at TEXT NOT NULL DEFAULT ''`, plus the trigger `catalog_playlists_updated`. Write cost per playlist change is one row update.
- Partial index `catalog_personal_history ON catalog_personal_sort_rows(profile_id,last_played_at,entity_id) WHERE last_played_at<>''`. It adds one index entry per played title per profile, so write cost stays flat.
- `schema-sync.py` was run.

## 6 Settings added

None. Pins are existing per-viewer server state.

## 7 Tests run

| Command | Result |
| --- | --- |
| `go build ./...`, `go vet` on `catalog`, `httpapi`, `playbackv1` | clean |
| `go test ./internal/catalog -run 'Pin\|Playlist\|Queue\|Across\|History\|GroupOrder\|HomeRow'` | ok |
| `go test ./internal/httpapi -run 'Contract\|OpenAPI\|Route\|Scope\|Pin\|Playlist\|Saved\|Queue'` | ok |
| `go test ./internal/playbackv1 -run 'Query\|Capture'` | see below |
| `schema-sync.py` (persistence tests) | ok |
| client-core `library-pins`, `browse-groups`, `saved*`, `personal-saved` tests | 74/74 |
| web `tsc --noEmit` | clean |
| web `lint:hooks` (oxlint) | clean |
| web `lint:strings`, `lint:design --strict`, `tokens` | clean |
| web preference-consumers check | clean |
| web `container-play`, `saved-*`, `section-append`, `type-scale` tests | pass |
| native `tsc` (app and test config) | clean |
| native `lint:hooks`, `lint:strings` | clean |
| native `container-play.test.ts` | 13/13 |
| Vega `tsc` | clean |
| `shared-packages.mjs --check` | identical |

The `playbackv1 -run 'Query|Capture'` row: it passes, but `TestGrowingQueryShuffleKeepsTheSameSeededOrder` failed 1 run in 4 under machine load. It passed 3/3 alone. The test is timing-based and is not touched by this lane; the query path it exercises is unchanged.

Full suites were not run (coordinator's instruction).

## 8 Device test notes

1. **Pins, web.** Rail › pencil next to "Libraries".
   - Pin Music, then Movies. Pass: Music then Movies at the top, a rule, then the rest.
   - Move Movies up. Pass: the order follows.
   - Open a playlist › menu › Pin. Pass: it appears under "Pinned" in the rail.
   - Open a second browser on the same profile. Pass: it follows within about 30 s.
   - On a phone-width window: the pinned items lead Saved.
2. **Pins, native.** Saved › Edit pins: the same checks as web.
   - TV (Apple TV, Android TV, Vega): every button in the sheet takes focus. Pinned playlists appear in the rail between the destinations and the libraries, and open the playlist with Saved beneath (Back returns to Saved).
   - iPhone: pinned items appear above the tabs on Saved.
3. **Episode group.** A flat show (owner setting) with more than 100 episodes. Pick "101–200" and press episode 150.
   - Pass: Up Next continues 151, 152… to 200, and the queue is titled with the show's name.
   - Repeat with "Specials".
4. **Home row.**
   - Recommended › See all › Play. Pass: the queue holds the row's titles in row order.
   - Sort by Year, then Play. Pass: year order.
   - A Home row of songs: pressing the 3rd song plays on through the rest of the row (on web, from the card's Play).
5. **Letter rail across libraries.**
   - Saved › My List › Sort: Title with titles in two libraries. Pass: the letter rail appears (TV: Jump to) and jumps correctly.
   - The same on a Home row page.
6. **History past 10,000.** Needs a profile with more than 10,000 played titles (a seeded database).
   - Saved › History with a sort or filter in its own order. Pass: scrolling continues past 10,000 and the count reads "10,000+".
7. **Playlists list.** Saved › Playlists and Music › Playlists.
   - Sort by Number of items and Last changed.
   - Filter Mine / Shared with me / Pinned. Pass: the counts are right.
   - A filter with no match. Pass: "Show all" returns.

## 9 Not finished

- **No browser or device run.** Only type-checks and unit tests were done (coordinator's instruction).
- **History beyond 10,000:**
  - only in its own order; other sorts on History still sort the 10,000 most recent;
  - playing the history as a queue covers the 10,000 most recent;
  - deep pages use OFFSET over the index, so their cost grows with depth into the viewer's own history (not with library size).
- **Home row continuation:** a background queue continuation after a restart cannot re-resolve a Home row (see Decision 5).
- **Letter index:**
  - only for lists across libraries under a title sort;
  - per-library group pages with a query condition still publish none (unchanged);
  - on native the rail is only on the Saved and Home-row windowed grids. Person and playlist pages draw paged sections without a rail.
- **Cross-device library-pin freshness:** this rides on the `personal.changed` hint. Native and web re-read the library list on every personal change, which is one small extra read per watched/rated mark while the app is open.
- **No "contains music/video" playlist filter** (Decision 10).
- **GOCACHE:** `_program/scratch/gocache-cp2-viewer` is deleted at the end of the lane.

## Commits

**portico-server** (`audit-implementation/cp2-viewer`):
- `db3bc269` Named pin list, playlist directory sorts and filters, Home row snapshot play
- `0c1d4e6c` Letter index across libraries; history across libraries past 10,000
- `4eaffa53` Web: pins in the rail and editor, episode-group and Home-row plays, playlist list toolbar
- `f830f64e` Pin change hints; rail reorders libraries
- `17df60a1` Merge of `audit-implementation/main`
- `1e5a8892` Test updates

**portico-react-native** (`audit-implementation/cp2-viewer`):
- `483787d` Shared-package mirror
- `170d4db` Native pins (rail, Saved, editor, page actions), whole-group plays, letter rail, playlist list toolbar
- `329f37b` Library reorder on pin changes
- `59362f9` Merge of `audit-implementation/main`
- `d0a8cd4` Mirror lock
