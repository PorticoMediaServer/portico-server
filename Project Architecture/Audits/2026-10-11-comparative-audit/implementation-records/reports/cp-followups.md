# cp-followups: Justin's answers on the completion-pass choices (15 Oct)

Worktrees: `server-cp-followups` and `native-cp-followups`, both on branch `audit-implementation/cp-followups`. `audit-implementation/main` (server 00767ddb, native 5b6231f, including final-pass-2 and "start next episode during the credits") is merged into both. After the merge, contracts were regenerated, the schema re-synced and the shared packages re-mirrored (`--check` says identical to server 010b5459). No subagents or helpers were used. Nothing was pushed.

## 1 Outcome

**Picture in Picture (item 1).** Picture in Picture is now the browser's own window with the plain video.
- A burned-in subtitle stays, because it is part of the picture.
- A drawn subtitle (styled or image, on the page's canvas) is not shown in Picture in Picture. It comes back when the picture returns to the page.
- Opening Picture in Picture never asks the server for anything: no conversion and no reload.
- Removed: the Chrome/Edge document Picture in Picture wrapper (the window, copied styles, the "Playing in Picture in Picture" placeholder and its two strings) and the Safari path that switched to server delivery or burn-in while Picture in Picture was open.
- Kept: the iPhone full-screen text hand-over. It is full screen, not Picture in Picture.
- The `web/README.md` limits note is updated. A plain text track belongs to the video element, so Safari's window may show it; Chrome's does not.
- Native was checked: Picture in Picture changes no subtitle and triggers no burn-in. `subtitlesCarried` is only reported. No change was needed there.

**Exact resume in MP3, ADTS and Ogg without converting (item 2).** The FLAC deep start is gone: `AudioPlan.DeepStart`, the converted route's service of direct plans, the client `deepStart` and their tests. It is replaced by a per-file **seek index**.
- **When it is built.** The index is made once per file, in the same background measurement that already produces the audio facts (`audiofacts`, run by the facts backfill). After the ffprobe pass, the file is read once more through the same fenced input. Only frame and page headers are parsed; payloads are skipped, nothing is decoded and no FFmpeg runs.
- **What it holds.** Byte offsets at packet or page boundaries, each with the raw frame it starts at.
  - Entries are about 3 s apart, with at most 4096 per file. A longer file gets wider spacing, so the size stays flat.
  - Frames are counted the way the web demuxers count them.
    - MP3: the Xing/Info/VBRI frame is skipped, and LAME delay is handled by the plan's trim.
    - ADTS: 1024 frames per raw block.
    - Opus: frames come from each packet's TOC, with pre-skip included.
    - Vorbis: each entry carries the page granule.
  - An MP3, ADTS or Opus index is stored only if its frame total equals the measured raw frames exactly. If it doesn't, the file simply has no index.
- **Where it lives.** Table `audio_seek_index`. It is fenced by the file's size and mtime and deleted with the asset, the same rules as the facts. `Save` replaces or removes it together with the facts.
- **How the client gets it.** A direct MP3, ADTS or Ogg plan names it as `seekIndex: {url}`, served by `GET /v1/media/{grant}/audio-index`.
- **How the web decoder uses it.**
  1. On a part-way start, it reads the index once per presentation.
  2. It picks the last entry at or before the target. If that entry leaves too few packets for the codec's pre-roll, it steps back one entry.
  3. It Range-reads the file from that entry's offset, decodes, and discards samples up to the exact frame.
  4. Nothing before the entry is fetched.
- Gapless trims, crossfade and gains are untouched, because the plan and trim are the same.
- The browser's own `<audio>` path (non-gapless) still uses plain Range seeking, unchanged.

**One rule for player choices (item 3).** Choices made in Settings apply everywhere. Choices made in the player last for that playback only.
- **Removed:**
  - "Forget last choice", everywhere: the web button, the native menu row, `DELETE /v1/items/{itemId}/playback-memory`, the client method and four strings.
  - The server's per-title track memory: the `playback_track_memory` table, its read on every start, its write on every choice, and the preview's `last_used` preference.
- **Carried within one playback.**
  - The audio and subtitle tracks picked in the player are carried by the server through the queue. They travel as languages (audio language; subtitles on with a language, or off) in the session's stored request. Every start of a queue entry inherits them from the session the queue last played: autoplay, Up Next, Next and Previous, a restart in place, a lease recovery.
  - A new queue is a new playback, so its first entry uses the Settings again.
  - Where the next file lacks the language, the Settings decide for that file. The carried choice continues to the following one.
- **Client choices are held for one playback**, a "playback run" kept by the client and shared by web, native and Vega (`PlaybackSnapshot.playbackRun`, `PlayerSessionChoices` in client-core). A new run starts on a fresh Play, a new queue or a download. It is unchanged by queue moves.
  - **Speed** (web video; native video and audiobook) now holds through the queue. Before, it reset on every episode.
  - **Web audiobook speed** no longer writes the profile's `audiobooks.defaultSpeed`. It is held for the playback.
  - **Web picture fill**, **subtitle position (raise)** and **Forced only** are no longer kept in `localStorage`.
  - **Native Forced only** is no longer a device preference.
  - **Native in-player caption size and background** no longer write the profile's Settings.
  - **Native in-player Player (engine) choice** no longer changes the device's Settings. It is a session override in the engine-setting store, so the capability document carries it for this playback. Before a new playback starts, the override is dropped and the start waits for the Settings' engine to be republished (bounded at 4 s; `PlaybackOptions.beforeFreshPlayback`).
- Quality was already right: the player's choice is held for the queue and reset by a new Play.

**Manual order for collections (item 4).**
- **Personal collections (saved resources).** Each entry now has an order key, using the playlists' key scheme.
  - New entries append. That covers single adds, bulk "add to collection" jobs and Undo, which puts an entry back in its old place.
  - The owner, or an editor of a shared collection, moves entries with `PUT /v1/saved-resources/{id}/order`: up to 200 entries, with an anchor ("" is the front) or swapped in place. The move is fenced by the collection's revision. Only the moved entries' keys are rewritten, which is tested on a 2000-entry collection.
  - The listing, the queue selector ("Play" and "Shuffle" from the page) and the cross-library browse group order now follow the collection's order. Before, it was title order.
  - "Collection order" is the default sort of a personal collection on web and native.
- **Library collections (owner-curated).**
  - Owners could not set the order before, and no client edited library collections. Members added through the API all had an empty key, so they showed in title or id order.
  - Now: titles append; `PUT /v1/collections/{id}/order` moves them; the collection carries a `revision` (moved by triggers on every membership or order change); legacy members with empty keys get keys, in their current order, on the first move.
  - The library collection page (web and native) now lists through the browse engine in collection order from the start.
- **Clients.**
  - Web: a "Reorder" mode on the personal collection page and, for the owner or Administrator, on the library collection page. Drag a row by its handle, or use the row's menu (Move to top / Move up / Move down / Move to bottom), which works from the keyboard. The page shows the move at once and reverts if the server refuses.
  - Phone and tablet: the same Reorder mode with move actions in each row's menu. TV has none, because TV does not edit collections.
  - A move refused because the collection changed meanwhile is re-sent once against the new revision, silently.

## 2 Decisions I made

- **What carries through a playback:** the audio language, the subtitle choice (on with a language, or off), quality (already held), speed, Forced only, subtitle position, picture fill, the native caption look and the native player engine.
- **What does not carry:** the **subtitle timing offset**. It belongs to one file's sync. It stays with the session (and re-plans) but is not carried to the next episode, as in Plex.
- **Forced only gets no new Settings default.** Settings already has "Show subtitles: Forced only", which picks forced tracks. Defaulting the in-player line filter from that mode would hide most lines of a full track a viewer picks by hand, and that mode is the default for every profile. The in-player switch now starts off at each new playback.
- **Subtitle position and picture fill get no new Settings default.** Neither was obviously needed. Subtitle position is the likeliest candidate if you miss it.
- **"Session" for the server** means the queue. A start of an entry of the same queue carries the tracks from the session the queue last played; a new queue (any fresh Play) does not.
- **Seek index storage** is a separate rowid table rather than a column on the WITHOUT ROWID facts table, so a large blob doesn't sit in the facts rows. The plan carries only the route.
- **The seek index is served by a route**, not embedded in the plan, so sessions and presentations don't carry up to tens of KB each. The client fetches it only for a part-way start.
- **Spacing:** about 3 s per entry, at most 4096 entries.
- **Pre-roll:** the client steps back one entry if the target entry leaves fewer than the codec's pre-roll packets. The pre-roll counts are the existing ones: MP3 3, AAC 2, Opus 4.
- **Vorbis entries** are page granules. No client decodes Vorbis from an index today: web doesn't decode Vorbis, and Android's Media3 seeks Ogg itself. The index is stored and tested for it.
- **Collection reorder conflicts** answer `412 revision_mismatch`, as playlist order moves do. The other saved-resource routes still answer `409 playlist_conflict`. Clients re-read and re-send a move once.
- **Web drag and drop** uses the browser's own drag events, a dependency-free choice. On touch browsers the row menu is the way to move.
- **"Move to bottom"** is offered only when the whole list is loaded. Otherwise the last loaded entry isn't the last.
- **The library collection page always lists through the browse engine** in collection order. Its old own list was title order and is not shown for collections now.
- **An extra fix:** a playlist's group order (`groupOrder` sort and the list across libraries) was read from `position`, which a playlist reorder never updates. It now uses the order keys, as everything else does.

## 3 Audit bugs closed and decision gaps closed

| Gap | What changed | Test |
|---|---|---|
| PiP subtitles (15 Oct decision) | Document PiP wrapper and Safari PiP hand-over removed; README updated; native checked | web suite (no render harness for the player; device-only) |
| Exact resume in MP3/ADTS/Ogg without converting (15 Oct) | `audiofacts/seekindex.go`, `audio_seek_index`, `/audio-index`, plan `seekIndex`, web `packetsFrom` + indexed start; FLAC deep start removed | Go `TestSeekIndexLandsOnTheExactFrame` (real FFmpeg: VBR MP3 with Xing, CBR MP3 without, ADTS AAC, Ogg Opus, Ogg Vorbis; decodes from entries and checks the exact frame; a +1024 mutation fails all five), `TestSeekIndexEncoding`, `TestSeekIndexStaysBounded`, `TestPlaybackV1DirectPlanNamesItsSeekIndex` (route, stale file, no conversion served); web `audio-seek-index.test.ts` (exact first sample from synthetic MP3 and ADTS streams with a stand-in decoder; a one-frame shift fails), `audio-render.test.ts` (index read once, never `audio-converted`); client-core `audio-render-v2.test.ts` |
| One rule for player choices (15 Oct) | Section 1 | Go `TestPlaybackV1InPlayerTrackChoicesLastOnePlayback` (Settings at start; in-player audio choice not kept for a fresh start or a new queue; carried by completion and Previous; the forget route is gone; fails if the carry is removed), `TestCarriedSubtitleChoice`; client-core `player-session-choices.test.ts`, `v1-queue-player.test.ts` (run kept by Next and autoplay, new by Play; a fresh Play waits for `beforeFreshPlayback`, Next doesn't), `listening.test.ts` (speed held for the run, never saved, told to the native owner) |
| Collections manual order (15 Oct) | Section 1 | Go `TestCollectionManualOrder` (append, front/after/swap, stale revision, replay, invalid moves, editor yes/viewer no, Undo keeps place, group order), `TestCollectionMoveWritesOnlyTheMovedEntries` (2000 entries, 2 keys rewritten), `TestLibraryCollectionManualOrder` (append, re-add keeps place, move, conflict, outside title, legacy empty keys), `TestQueueKeysResolveSavedViewsAndSharedCollections` (plays in its own order, then after a move), `TestSavedCollectionOrderRoute` (HTTP: listing order, move, 412, 400, refused move changes nothing); client-core `manual-order.test.ts`, `library-collection-order.test.ts`, `personal-saved.test.ts` (move, silent single re-send on conflict, editor/viewer), `browse-groups.test.ts`; web `group-spec.test.ts`; native `group-scope.test.ts` |

## 4 API changes

- **Removed:**
  - `DELETE /v1/items/{itemId}/playback-memory` (`forget_playback_track_memory`).
  - The direct plan's `presentation.audioRender.deepStart`.
  - `GET /v1/media/{grant}/audio-converted` no longer serves a direct plan; only converted plans.
- **Changed:**
  - `GET /v1/items/{id}/playback-options` `preferred`: `subtitleId` and `subtitlesOff` are removed. `reason` is now `profile_language` or `default`; `last_used` no longer exists.
  - The planner trace's `chosenBy` value `remembered` is renamed `carried` (a queue carried the language).
  - `GET /v1/collections/{id}` and the collection lists: `Collection` gains `revision`.
  - Saved resource `actions` can include `reorder` (owner and editor of a collection).
- **Added:**
  - `GET /v1/media/{grant}/audio-index` (`get_media_grant_audio_index`): `{version: 1, offsets: [...], frames: [...]}`. Grant-scoped like `/audio`; `404` when the plan names no index or the file changed.
  - `presentation.audioRender.seekIndex: {url}` on direct MP3, ADTS and Ogg plans that have an index.
  - `PUT /v1/saved-resources/{id}/order` (`reorder_saved_collection`). Body: `{operationId, expectedRevision, entryIds (1–200), afterEntryId?}`. Answers as the other saved-resource writes, or `412 revision_mismatch`. Owner or editor.
  - `PUT /v1/collections/{id}/order` (`reorder_library_collection`). Body: `{expectedRevision, itemIds (1–200), afterItemId?}`. Owner access, library scope. Answers with the `Collection`, or `412 revision_mismatch`.
- **Client-core:**
  - `PlaybackSnapshot.playbackRun`; `PlaybackOptions.beforeFreshPlayback`; `beginQueueIntent(itemId, fresh)`.
  - `PlayerSessionChoices` / `playerSessionChoices`.
  - `parseSeekIndex` / `SeekIndex`.
  - `PersonalSavedIntent` `reorder`.
  - `presentation/manual-order.ts`; `library-collection-order.ts`.
  - `acrossOwnOrder('savedCollection')` is now true.
- Contracts and OpenAPI were regenerated with `go run ./cmd/apigen`, after the merge too.

## 5 Schema changes (baseline, synced)

- **Dropped:** `playback_track_memory` and its index.
- **Added:** `audio_seek_index(asset_id PK → catalog_assets ON DELETE CASCADE, size, modified_ns, entries BLOB CHECK 2..131072)`.
- **`saved_resource_entries`:** new `order_key TEXT NOT NULL DEFAULT ''` and index `saved_resource_entries_order(resource_id, order_key, id)`.
- **`catalog_collections`:**
  - New `revision INTEGER NOT NULL DEFAULT 1`.
  - The member insert and delete triggers now also move it.
  - New trigger `catalog_collection_member_order` (on update of `order_key`).

## 6 Settings added

- None.
- Device-local values removed:
  - Web: `localStorage` `portico.player.fill.v1`, `portico.player.subtitle-forced-only.v1` and `portico.player.subtitle-raise.v1`.
  - Native: the preference `subtitleForcedOnly`.
- **Device state kept as is, please confirm:**
  - Volume and mute (web `localStorage`, native system volume).
  - The Picture in Picture window's place and size (the system's).
  - The web library's grid or list layout and poster size (device conveniences, not player choices).
  - The native Android Layout setting (a Settings value, not a player choice).

## 7 Tests run (final code, after the merge of main)

| Command | Result |
|---|---|
| `go build ./...`, `go vet ./internal/...` | pass (vet's only messages are pre-existing, in `internal/thirdparty/sqlite`) |
| `go test ./internal/playbackv1 ./internal/audiofacts ./internal/catalog ./internal/compactcatalog ./internal/persistence ./internal/subtitlevideo` | ok |
| `go test ./internal/httpapi ./internal/playback` | playback ok. httpapi: every test passes except `TestListeningHTTPPreferenceEnvelopeAndCAS`, which stops at "explicit reviewed helper binary required". That helper is missing on this host; it is a known environment gap (cp-native-playback reported the same), not a code failure. |
| `npm run test:core` | 1984/1984 |
| `web: npm run check` (tokens, design lint, hooks lint, preference consumers, strings, tsc, tests) | pass, 584 tests |
| `node --test server/receiver/cast/test/receiver.test.cjs` | 21/21 |
| native `tsc --noEmit`, `lint:hooks`, `npm test`, `lint:strings` | pass, 676 tests, 0 hard-coded strings |
| Vega `tsc --noEmit`, `npm test` | pass, 87 |
| `scripts/shared-packages.mjs --check` (native) | identical to server 010b5459 |

`compactcatalog`'s `TestPerItemDerivationStatementsSeek` failed on my branch before the merge, for a reason not caused by this lane (a scan of `saved_entry_undo`). It passes after the merge of main.

## 8 Device test notes

1. **Picture in Picture, web.** In Chrome, Edge and Safari, play a title with ASS or PGS subtitles drawn, then press Picture in Picture.
   - Pass: the window shows the video without those subtitles, and no reload or new stream starts (DevTools Network shows no new `/v1/media` request).
   - Close the window: the drawn subtitles are back at once.
   - With a burned-in subtitle (owner forces burn-in), the window shows it.
   - On an iPhone, full screen with ASS still shows text lines in the system player.
2. **Native Picture in Picture.** On iPhone and Android, Picture in Picture with mpv-drawn subtitles: no re-plan and no burn-in.
3. **Seek index.**
   - After upgrading, existing MP3, ADTS and Ogg facts have no index until re-measured. On a test server, run `DELETE FROM audio_render_facts WHERE container IN ('mp3','adts','ogg');` and the background backfill measures them again.
   - Then, in Chrome and Safari on macOS with gapless on, resume a 1-hour VBR MP3 audiobook at 40:00 on a throttled link.
   - Pass: the Network panel shows one `audio-index` request, then Range requests starting a few seconds' worth of bytes before the target, never from byte 0, and no `audio-converted`. Playback starts at the right word; the next track joins gaplessly.
   - Repeat with an ADTS `.aac` and an Ogg Opus file.
   - Check that `audio_seek_index` has rows (`SELECT count(*) FROM audio_seek_index`).
4. **Player choices.**
   - In a TV show, pick the Japanese audio and English subtitles in the player and let it autoplay into the next episode: the same tracks.
   - Press Play on an episode from the show page: the Settings' languages again.
   - Change the speed to 1.5×: the next episode keeps it; a new Play starts at the Settings' speed.
   - Native: choose mpv in the player, then start another title from its page. Pass: it starts on the Settings' player (Automatic), and the Settings screen never changed.
   - Native: change the caption size in the player. Pass: Settings › Audio & Subtitles is unchanged; a new playback uses the Settings' size.
5. **Personal collection order.**
   - Web: open a collection with 5+ titles, press Reorder, drag one to the top, and use "Move to bottom" from a row's menu (keyboard: Tab to the row's ⋯, Enter, arrows).
   - Pass: the order holds after reload, on native, and when pressing Play (the queue follows it).
   - In a second tab, add a title, then move one in the first tab: it succeeds without a conflict message.
   - A shared editor can reorder; a viewer sees no Reorder.
6. **Library collection order.**
   - As owner, create a collection with the API (`POST /v1/libraries/{id}/collections`, then items) and open it: titles show in the order added.
   - Reorder on web and on a phone or tablet. Pass: a member profile sees the new order; Play follows it.
   - On TV there is no Reorder.

## 9 Not finished

- **Nothing was run on a device or in a browser.** That includes Picture in Picture, real decoding from an index (the client test uses a stand-in decoder; the exact-frame proof with real decoders is the server test with FFmpeg), drag and drop, and the native reorder screens. No browser smoke run was done.
- **The seek index is used only by the web decoder.**
  - Apple's ExtAudioFile engine and Android's Media3 seek MP3, ADTS and Ogg themselves. Neither used the old FLAC deep start either.
  - MP3 seeking in those engines is as before: approximate in Media3 without a Xing table, and possibly a full scan in AudioFile.
  - Using the index there is follow-up native work.
- **Files measured before this change have no index** until their facts are measured again (note 3). There is no automatic re-measure, to avoid a compatibility shim.
- **Web reorder mode.**
  - Library collections: only the first 500 titles are arranged on the page.
  - Personal collections: the list re-reads from the first page after each move.
- **Native has no drag handles.** Moves are by menu. The brief allowed either.
- **The native player-engine session override** depends on the capability document republishing within 4 s before a new playback starts. On a slow link, the first title of a new playback could still be planned for the previous engine choice.
- **Vorbis index entries are not used by any client** (see Decisions).
- **No render tests** for the new React wiring: the web Reorder modes, the Picture in Picture removal, `usePlaybackChoice` in the player, or the native screens.
- **Saved-resource writes other than reorder still answer `409 playlist_conflict`** rather than a registered error code. Their contracts don't list it. This predates this lane.

## Commits

portico-server (`audit-implementation/cp-followups`):
- fa7d135f Player choices last one playback: Settings decide every start, in-player changes carry through the queue and nothing is remembered per title
- 8eb95f67 Picture in Picture shows the plain video: burned-in subtitles stay, drawn ones wait for the page, and opening it never converts or reloads
- 087efc5b Start MP3, ADTS and Ogg part-way from a per-file seek index read directly, instead of converting to FLAC
- 47d9350b Collections keep a manual order: new titles append, owners and editors move them, and the order is what lists, sorts by default and plays
- 010b5459 Merge of audit-implementation/main

portico-react-native (`audit-implementation/cp-followups`):
- f3ea14b Player choices last one playback: no Forget last choice, and speed, caption look, Forced only and the player chosen in the player hold only until a new playback
- ba651f5 Collections keep a manual order on phones and tablets (includes the shared-package mirror of the seek index and collection order)
- 275905f Merge of audit-implementation/main (re-mirrored from server 010b5459)
