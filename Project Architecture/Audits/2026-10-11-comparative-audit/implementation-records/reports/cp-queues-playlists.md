# cp-queues-playlists — whole-view playback and playlist correctness

Execution: Ubuntu mini-1, worktrees `server-cp-queues-playlists` (from main-server `f719c2a0`) and `native-cp-queues-playlists` (from main-native `4f0689c`), branch `audit-implementation/cp-queues-playlists`. No subagents or helpers. Nothing pushed. The session was interrupted once by an app restart. All edits survived, and the last commit, the typechecks and the test suites were rerun afterwards. `node_modules` is symlinked from Justin's main checkouts, read-only, as the other lanes do (the links are ignored by Git).

## 1 Outcome

- **Lists play exactly what is shown.** Play, Shuffle and play-from-here now send the server's own selector instead of the rows already loaded. This covers sorted or filtered lists, saved views and personal collections. Where it applies:
  - Web, phone/tablet, TV and Vega (Vega reuses the native screens).
  - The library list view and Songs tab.
  - An album's or artist's sorted songs.
  - A season's sorted or filtered episodes.
  - A playlist that has been sorted or filtered.
  - Saved views and personal collections.
  - The Play and Shuffle buttons on an album, artist or collection page while its list is sorted or filtered.
- **The 500-title limit on native saved views is gone.** The native code that read a saved view page by page before playing it (`gatherView` / `useGatherView` / `view-play.ts`) is deleted, along with its two strings.
- **The pressed copy plays.** When a playlist holds the same title twice, pressing the second copy starts at the second copy. The queue anchor now names the playlist entry (`anchor.entryId`), on both web and native.
- **Removals can be undone.** Removing an entry from a playlist (web and native) or a personal collection (web) shows an Undo toast. Undo is the server's: it puts back the same entry (same entry id; in a playlist, the same order key, so it returns to its place). Native has no collection-removal control, so there is nothing to undo there.
- **Downloaded episodes continue offline.** When a downloaded episode ends, the player shows Up Next with the next downloaded episode of the same show, and counts down using the viewer's own Autoplay and countdown settings. Replay of a download now replays the local file instead of asking the server.
- **Native personal collections play correctly.** Their Play button had been naming the collection as a catalog `collection` container, which the server cannot resolve, so the play failed silently. It now uses the saved selector.

## 2 Decisions I made

- **Occurrence anchor:**
  - The anchor is `{entryId, itemId?}`. Playlist entry tokens are stable through reorders, so no playlist revision fence is needed.
  - If the pressed entry was removed meanwhile, play falls back to `itemId` (that title's first copy), else to the start of the playlist.
  - `entryId` is accepted only with a playlist container (422 otherwise).
  - Its position is counted on the existing `catalog_playlist_entries_page` index, once per play.
- **Duplicate handling in the queue scanner:** it tracked each key's source position by id, which is wrong when an id repeats. It now keeps a per-occurrence position list. This also fixes the resume accounting of a partially built playlist that contains duplicates.
- **Undo design:**
  - The Undo token rides the existing endpoints instead of new routes:
    - Playlist: `POST /v1/playlists/{id}/entries` with `{undoToken}` (no `itemId`).
    - Collection: `PUT /v1/saved-resources/{id}/entries` with `{undoToken}` alone.
  - Lifetime is 30 seconds, the same as container Undo.
  - Each person has one live token per list, so a new removal replaces the previous offer.
  - Expired rows are cleared 16 at a time by later removals.
  - Undo still sends `expectedRevision`. The existing client silent re-read and retry covers a concurrent change.
- **Personal collection play order:** a collection now plays in the order it is shown (title, then entry id). The server's saved selector had been ordering by entry id, which is a random token, so playback order did not match the screen.
- **Header Play of a sorted list:** it anchors at the list's first loaded playable item, which is the first item of the query.
- **Offline Up Next:**
  - "Next" means: same show (`info.parentId`), a ready download, strictly after the current episode by season then episode number. Specials are skipped unless a special has just ended (the same rule as the server's show selector).
  - Autoplay and the countdown come from the cached server preferences `playback.autoplayNext` and `playback.upNextCountdownSeconds`, not from a device setting.
- **Not changed:**
  - Home shelf track rows still play their loaded shelf: a recommendation shelf has no server selector.
  - Non-season episode groups (arcs, ranges) still play their loaded list: the v1 selector cannot name a range. The cp-browse-parity lane owns group orders.
  - The library list view keeps its old loaded-rows fallback only for callers that pass no query; every current caller passes one.
- **Native fade for large windowed grids:** not touched. It does not fall out of this work, as the brief allowed.

## 3 Audit bugs closed and decision gaps

- **B-21.11 — closed.**
  - Changed: every first-party sorted, filtered or saved-view Play, Shuffle and play-from-here handler now forwards a `query` or `saved` selector, on web and native (phone/tablet/TV) and on Vega via the shared screens.
  - Tests: web `container-play.test.ts` and `entity-play.test.ts`; native `container-play.test.ts` and `entity-play.test.ts`; server `TestQueueKeysResolveSavedViewsAndSharedCollections` (now also checks collection order).
- **B-21.8 — closed.**
  - Changed: playlist rows send `occurrenceSequence(playlistId, entryId)`, and the server anchors at that entry.
  - Tests: HTTP `TestPlaylistPlaysFromThePressedOccurrence` (second copy, plain item anchor, removed copy, entry-only anchor, rejection outside a playlist); web and native container-play tests.
- **B-18.29 — closed.**
  - Changed: occurrence-safe receipt Undo for playlist and collection entry removal (server, client-core, web, native playlist).
  - Tests:
    - Catalog: `TestPlaylistEntryRemovalUndoesInPlace` (same entry id and order back; spent, lapsed and other people's tokens refused; token plus item refused).
    - Catalog: `TestCollectionEntryRemovalUndoes`.
    - client-core: `saved.test.ts` and `personal-saved.test.ts` undo cases.
    - web: source wiring test.
- **Decision gap closed:** offline next-episode queue on native. Test: `offline-next.test.ts` (order, unfinished downloads skipped, specials, non-episodes).

## 4 API changes

- **Queue `anchor`** (on queue create and add-segment) accepts `{"entryId": "<playlist entry id>", "itemId"?: "..."}` with a playlist container. The generated contract type stays `unknown`.
- **Playlist removal receipt:** `PlaylistReceipt` gains optional `undoToken` and `undoUntil`. The removal route fills them.
- **Playlist add with a token:** `PlaylistMutation` gains `undoToken`. An add carrying `undoToken` and no `itemId` restores the removed occurrence and answers its `entryId`.
- **Collection removal and restore:**
  - `SavedResourceMutation` gains `undoToken`.
  - `SavedResourceReceipt` gains `undoToken` and `undoUntil` when entries were removed.
  - An `entries` change carrying `undoToken` alone restores them, reported as `entries.added`.
- **Saved collection play order:** a saved collection's queue selector now plays in title order (it had been entry-id order).
- **Regeneration:** contracts, OpenAPI and `schemas.ts` regenerated with `apigen` (`clientcontractgen` produced no change). No routes added or removed. Scopes and matrices are unchanged.
- **client-core:**
  - `QueueAnchor` gains `entryId`.
  - `SavedIntent` gains `{action: 'restore', undoToken}`.
  - `SavedReceipt` gains `undoToken` and `undoUntil`.
  - `PersonalSavedIntent` `entries` gains `undoToken`.
  - The personal-saved `result` gains `undo: {token, until}`.

## 5 Schema changes

`0001_baseline.sql` gains:
- the `saved_entry_undo` table (`token`, `owner_key`, `kind`, `resource_id`, `entry_id`, `item_id` FK with cascade, `order_key`, `expires_ms`; primary key `(token, entry_id)`);
- the indexes `saved_entry_undo_list` and `saved_entry_undo_expiry`.

`schema-sync.py` was rerun.

## 6 Settings added

None. Offline Up Next reads the existing server preferences `playback.autoplayNext` and `playback.upNextCountdownSeconds`.

## 7 Tests run

| Command | Result |
| --- | --- |
| `go test ./internal/playbackv1/` | ok |
| `go test ./internal/catalog/` (full package) | ok |
| `go test ./internal/httpapi/ -run 'Queue\|Playlist\|Shuffle'` | ok |
| `go test ./internal/httpapi/ -run 'Playlist\|Saved\|Restriction\|Collection\|OpenAPI\|Contract\|Route'` | One FAIL on the first run, under machine load; the failing test name was not captured. Passed on the immediate rerun. |
| `go vet` on `playbackv1`, `catalog`, `httpapi` | clean |
| persistence (via `schema-sync.py`) | ok |
| `npm run test:core` | 1949/1949 |
| web `npm test` | 553/553 |
| web `tsc --noEmit` | clean |
| web `lint:strings` | clean |
| native `npm test` | 666/666 |
| native `tsc` (app) | clean |
| native `tsc -p test/tsconfig.json` | clean |
| Vega `tsc` | clean |
| Vega `npm test` | 87/87 |
| `shared-packages.mjs --check` | identical |

GOCACHE deleted at the end.

## 8 Device test notes

1. **Duplicate playlist entry.** Make a playlist A, B, A, C (Save queue as playlist works). Press the third row.
   - Pass: the 3rd entry plays and Up Next is C.
   - Fail: it starts at row 1.
   - Do this on web, iPhone, Apple TV, Android TV and Vega.
2. **Remove and Undo in a playlist.** Remove the 2nd entry and press Undo within about 8 seconds.
   - Pass: it returns at position 2 (not at the end), and the count is restored.
   - Wait more than 30 seconds before pressing Undo (web keeps the toast up to 30 s): pass is "That couldn't be undone…".
   - On TV, check the Undo button in the toast can take focus.
3. **Remove and Undo in a collection (web).** Pass: the title reappears in the same place.
4. **Large sorted view.** In a library of more than 1,000 songs, sort the Songs tab by year (descending) and press a song near row 900.
   - Pass: that song plays, and Next follows the shown order past the loaded window.
   - Repeat with a filter, with an album's sorted songs, with a season's episodes sorted by air date, with a sorted playlist, and with a saved view of more than 500 titles (native Play and Shuffle).
   - Pass for Shuffle: the queue covers the whole view (queue total equals the view total).
5. **Personal collection on native.** Press Play.
   - Pass: playback starts, in the order shown (by title).
   - Before this change it did nothing.
6. **Offline next episode.** Download S1E1, S1E2 and S2E1 of a show. Turn on airplane mode, then play S1E1 from Downloads to the end.
   - Pass: Up Next shows S1E2 with the countdown from the server's Autoplay setting; it plays when the countdown ends, and Cancel stops it.
   - With Autoplay off: the card shows "Ready" and Play now.
   - With "delete watched downloads" on: the next episode still appears.
   - At the end of S2E1: the Finished card, and Replay replays the file.

## 9 Not finished

- Home shelf track rows and non-season episode groups still play loaded lists: no server selector exists for either (see Decisions).
- The library list view keeps its loaded-200 fallback for callers without a query. There are currently none.
- Native has no collection-entry removal UI, so it has no collection Undo. Web has both.
- **Possible conflict with cp-browse-parity:** if that lane adds group-only sorts (for example a group's own order) that the library pivot does not offer, query selectors built from those groups would be refused by the queue's sort resolution. The queue side would then need the same sort vocabulary. Please check this when the lanes are merged.
- Native fade or visual skip for large windowed grids: not done (out of scope unless free).

## Commits

**portico-server** (`audit-implementation/cp-queues-playlists`):
- `ad556f5f` Play the pressed copy when a playlist holds a title more than once
- `87f372a8` Let a removal from a playlist or collection be undone, putting back the same entry in its place
- `ee75265e` Play a personal collection in the title order it is shown in
- `e26023a8` Play sorted and filtered lists, saved views and collections whole on the web, and offer Undo after removing an entry
- `384939e2` Play and shuffle the sorted or filtered list from an album, artist or collection page's own buttons

**portico-react-native** (`audit-implementation/cp-queues-playlists`):
- `a855a32` Bring in the shared packages with playlist occurrence anchors and removal Undo
- `e9a7737` Play sorted and filtered lists, saved views and collections whole on phones, tablets and TVs, play the pressed playlist copy, and offer Undo after removing a playlist entry
- `feffb24` Play and shuffle the sorted or filtered list from an album, artist or collection page's own buttons
- `02de5f2` Carry on to the next downloaded episode of the same show when one ends offline
