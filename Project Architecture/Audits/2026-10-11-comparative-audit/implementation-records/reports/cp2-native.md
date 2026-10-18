# cp2-native: native app gaps

Worktrees: `server-cp2-native` and `native-cp2-native`, both on branch `audit-implementation/cp2-native`, from main-server `6ae3ccf4` and main-native `ea762a7`. No subagents or helpers were used. Nothing was pushed. Mid-lane, the coordinator asked for the most code with the narrowest tests: type-checks plus the tests that prove the new code. The full suites are left to the combined run at the end.

## 1 Outcome

1. **Remove from a personal collection on native.** On phones, tablets and TVs, the owner or an editor of a personal collection can long-press a title and get **Go to item** or **Remove from collection**. The removal shows the toast "Removed … from the collection." with **Undo**. Undo uses the same server token as the web, so the same entry goes back in the same place (occurrence-safe). An Undo that is refused says "That couldn't be undone…". A refused removal shows the server's reason. Saved views and collections the viewer can't edit keep the usual title menu.
2. **Sorted My List, Favorites and History draw cards by kind.** The sorted and filtered grid now sizes its cells from the kinds it has read: square for albums, posters for films and shows, landscape for episodes. It uses the same rule as the unsorted lists: one shape when all the titles share it, posters for a mixed list. Before this, it was always posters.
3. **Forced only, only when it means something.**
   - The server now counts an image subtitle's lines and forced lines when it keeps the original. For PGS, it reads composition headers. For VobSub, it reads the subpicture control sequences.
   - It publishes the counts at `…/lines`. Every PGS or VobSub delivery names this as `linesUrl`.
   - The native player reads the count once per track and shows **Forced only** only when some lines are forced and some are not. That is the web's rule.
   - The choice still lasts one playback (player choices rule).
   - I checked by reading the source that our own engine honours the option:
     - FFmpeg 8.1.2's `pgssubdec` sets `AV_SUBTITLE_FLAG_FORCED` on each object with composition flag 0x40, and `dvdsubdec` sets it on subpictures with command 0x00.
     - mpv 0.41's `sd_lavc.c` (`read_sub_bitmaps`) drops every rect without that flag while `sub-forced-events-only` is set. It reads the option each time it decodes, so a change applies from the next decoded line.
     - These are the exact sources in our media-engine bundle (`portico-media-engine-android-source-r1.tar`).
     - Both engines set the option from the command (`PorticoMpvController.kt:1282`, `PorticoMpvController.swift:1098`).
4. **Drag handles for collection order** (phones and tablets): each row in Reorder mode has a grip handle.
   - Press and drag it up or down. The rows it passes step aside, and letting go sends the move.
   - The page does not scroll while a row is held.
   - The row menu (top, up, down, bottom) stays, and it is what screen readers use: the handle is hidden from accessibility.
   - This covers personal collections and owner library collections, which share the same component. TV still has no reorder.
5. **Exact resume in MP3 and ADTS on native audio engines, from the server's seek index.**
   - **Android (Media3):** when the plan names a seek index, the player reads it before opening the file. This happens for a part-way start, a resume or any ADTS file. A wrapping extractor then:
     - replaces Media3's seek map with the index (the last entry at or before the position, one entry earlier when the decoder needs pre-roll: MP3 3 packets, AAC 2);
     - stamps the first sample after the seek with the entry's exact time, so Media3 drops exactly what lies before the position.
     - This works whether or not gapless or levelling is on: the plain stream path carries the plan as `seekPlan` for this.
     - ADTS files were not seekable in Media3 before (a resume started at 0). They now seek.
   - **Apple (Core Audio):** the decode engine reads the index on its deck queue at the first part-way seek. It opens a second AudioFile over the bytes from the chosen entry, decodes packet by packet with its own converter (no priming) and drops the frames up to the exact raw frame. Only the bytes from the entry onward are fetched. Before this, AudioFile estimated MP3 positions or read every byte before them, and the ADTS packet lookup did the same.
   - On Apple, a part-way start in such a file now goes through the decode engine even with no effects on, because AVPlayer would only land near the position.
   - **Ogg:** see Decisions.
6. **Forgot PIN on phones, tablets and TVs.**
   - The profile menu in Settings › Profiles now shows **Forgot PIN…** where the shared model offers it: a PIN-protected profile, seen by a profile that can't manage profiles.
   - The PIN pad of the profile picker has **Forgot PIN?** too.
   - Both open the existing recovery sheet: the signed-in account sets a new PIN with `POST /v1/direct/profiles/{id}/pin-reset`, the same route and rules as the web.
   - Nothing about the account-security rules changed.

## 2 Decisions I made

- **Forced only uses the server's count, not the device's.** mpv has no property that says whether a track's lines carry forced flags, and parsing the track on the phone would download a large PGS file a second time.
  - The count is taken when the original is kept, at no extra I/O: the bytes are already in memory there.
  - It is stored with the original (`subtitle_original_revisions.forced_lines/total_lines`).
  - Native image tracks drawn by mpv are always handed the original, so the count exists by the time the player asks for it.
  - **Downloads** (offline image tracks) have no count, so Forced only is not offered there.
- **The `/lines` route is grant-scoped like `/fonts`.** It has the same `CheckOriginal` pin, the same lane, and the same "media grant" exemption in the restriction matrix.
- **A PGS line** is a composition that shows at least one object. It is forced when every object it places is forced, as the web counts it. **A VobSub line** is a subpicture; it is forced when any of its control sequences carries command 0x00.
- **Ogg is left to Media3 on Android.** Its OggExtractor seeks by bisection on the granule position and stamps samples from the granule, so it already lands exactly. A substituted seek map would be ignored there anyway: the Ogg seeker does its own search. On Apple, AudioFile has no Ogg type in our decoder (Ogg isn't in `ARDecoder hint:`), so Ogg on Apple is outside this engine as before.
- **Android reads the index only when it will be used:** a part-way start, a resume, or ADTS, which cannot seek at all without it. The read is bounded at 4 s. A track started from 0 doesn't pay for the request. Later seeks in that track then use Media3's own placement, except for ADTS.
- **Landing precision on Android** is the decoded buffer that contains the position. Media3 drops whole output buffers before the seek time, so the landing is at most one packet early (about 26 ms for MP3). Apple drops frames exactly.
- **Collection removal is offered on TV too**, as playlist removal already is. Reorder stays off TV, as cp-followups decided.
- **The drag handle is a second way, not the only way.** The menu moves stay, and screen readers are pointed to them (the existing `order.handle` hint).
- **Forgot PIN on the picker** builds its identity client on the picker's account API (`pending.api`). The server decides whether that session may reset. If it may not, the sheet shows the server's reason.

## 3 Audit bugs closed and decision gaps closed

| Gap | What changed | Test |
|---|---|---|
| Native removal from a personal collection, with occurrence-safe Undo (cp-queues-playlists "Not finished") | `SavedPages.tsx` entry menu, removal, Undo toast; `saved-hooks.ts` `outcome()` | native `tsc` (UI wiring; the server Undo is covered by `TestCollectionEntryRemovalUndoes`) |
| Sorted My List / Favorites / History card shapes on native | `Saved.tsx` `loadedKinds` / `dominantShape` / `shapeWidth` / `shapeAspect` | `tsc` |
| "Forced only" only for mixed image tracks; own mpv honours it | Server `image_lines.go`, column, `/lines`, `linesUrl`; client-core `parseSubtitleLines` / `forcedChoiceOf`; native `linesUrlOf`, gating in `usePlayer.ts`; mpv/FFmpeg verified by source | Go `TestPGSLinesCountForcedCompositions`, `TestVobSubLinesCountForcedSubpictures`; client-core `subtitle-originals.test.ts` (new case); httpapi restriction matrix |
| Collection reorder with drag handles | `ManualOrder.tsx` (PanResponder handle, step-aside rows, `onDragging`); `SavedPages.tsx` and `Entity.tsx` stop scrolling while dragging | `tsc`, `lint:hooks` |
| Seek-index exact resume on native | Android `PorticoSeekIndex.kt` (+ controller and owner wiring); Apple `PorticoAudioRender.m` (`loadIndex`, `seekIndexed:`), `PorticoListeningOwner.m` | Kotlin type-check (`native-ktcheck.sh`: exit 0, no errors); Apple not compiled (see §8) |
| Forgot PIN on phones/tablets/TV | `Profiles.tsx` (menu row + sheet), `DirectProfiles.tsx` (PIN pad "Forgot PIN?") | `tsc` |

No other audit bug IDs belong to this lane.

## 4 API changes

- **Added:** `GET /v1/media/subtitles/{grant}/{resource}/{revision}/lines` (`get_v1_media_subtitles_grant_resource_revision_lines`). It answers `{forced, total}`. It is public with grant authority on the media lane, `ScopeOpen`, like `/fonts`.
- **Changed:** a subtitle `Delivery` gains `linesUrl` (PGS and VobSub). The v1 `presentation.subtitles[]` files gain `linesUrl`.
- **client-core:**
  - `SubtitleDelivery.linesUrl`
  - `SubtitleOriginal.linesUrl`
  - `SubtitleLines`, `parseSubtitleLines`, `forcedChoiceOf`
  - the v1 presentation subtitle file `linesUrl`
  - `validateSubtitleDelivery` accepts `/lines` addresses.
- Contracts and OpenAPI were regenerated (`go run ./cmd/apigen`), and the shared packages were mirrored to native (`--write`, lock updated).
- **Native engine commands (internal):** the Android listening command carries `seekPlan` (the presentation's plan), so the plain stream path can use the index.

## 5 Schema changes

`0001_baseline.sql`: `subtitle_original_revisions` gains:

- `forced_lines INTEGER NOT NULL DEFAULT 0 CHECK(forced_lines>=0)`
- `total_lines INTEGER NOT NULL DEFAULT 0 CHECK(total_lines>=forced_lines)`

All three inserts name their columns. `schema-sync.py` was run (persistence ok). Originals cached before this change have 0/0, so they show no Forced only until they are cached again. Development databases need recreating, as usual.

## 6 Settings added

None.

## 7 Tests run

| Command | Result |
|---|---|
| `go build ./...` (server) | ok |
| `go test ./internal/subtitles -run Lines` | ok |
| `go test ./internal/httpapi -run 'Route\|OpenAPI\|Contract\|Lane\|Subtitle\|Registry\|Scope'` | First run: `TestRestrictedProfileRouteMatrix` asked for the new route's classification. I added it (media grant, like `/fonts`). Rerun with `-run 'TestRestrictedProfileRouteMatrix\|Route\|OpenAPI\|Lane'`: ok |
| `python3 scripts/schema-sync.py` | ok (persistence tests ok) |
| client-core `node --test test/subtitle-originals.test.ts test/subtitles*.test.ts` | 21/21 |
| native `tsc --noEmit` (app) | clean |
| native `lint:hooks` | clean |
| Android Kotlin type-check (`_program/native-ktcheck.sh`) | exit 0, no errors |
| native `npm test` | 676/676 |
| native `tsc -p test/tsconfig.json`, Vega `tsc --noEmit` | clean |
| `shared-packages.mjs --check` (native) | identical to server `0d414e5e` |
| `git merge audit-implementation/main` (both) | already up to date (main was still at the branch base, `6ae3ccf4` / `ea762a7`) |

GOCACHE deleted at the end.

## 8 Device test notes

1. **Collection removal (phone, tablet, TV).** Open a personal collection you own. Long-press a title, then Remove from collection.
   - Pass: the title goes, and the toast offers Undo. Undo within about 8 s puts it back in the same place, and the order holds after you pull to refresh.
   - Wait more than 30 s and press Undo (if the toast is still up): "That couldn't be undone".
   - As a viewer of a shared collection (not an editor): long-press shows the normal title menu, with no Remove.
   - On TV: the menu takes focus, and Undo in the toast is focusable.
2. **Sorted shapes.** Add albums and films to My List.
   - With albums only, choose a sort: square cards. With films only: posters. Mixed: posters (album art fitted).
   - Repeat in Favorites and History.
3. **Forced only.** Use a Blu-ray rip with a full English PGS track that has forced alien-dialogue lines, and a separate forced-only track.
   - Player = mpv (or Automatic, which picks mpv for PGS). Choose the full track, open the subtitle options.
   - Pass: **Forced only** is shown. Turn it on: only the forced lines show from the next line on.
   - Choose the forced-only track: Forced only is not shown. Choose a track with no forced lines: not shown.
   - Repeat with a DVD VobSub track that has forced subpictures.
   - Network check: one `GET …/lines` per track choice, answering `{forced: N, total: M}`.
   - Downloads: Forced only is not offered (no count offline).
4. **Drag handles (phone, tablet).**
   - Open a personal collection with 10 or more titles, press Reorder and drag a row's grip from position 2 to position 7. Pass: the rows step aside while dragging, the page doesn't scroll, the order holds after letting go and after a reload, and Play follows it.
   - Drag on the row itself (not the grip): the page scrolls as usual. Tap the row: the move menu.
   - VoiceOver/TalkBack: the grip is not announced, and the row's menu still moves.
   - The owner's library collection page (Reorder): the same.
   - Watch for this on **iOS:** if the list scrolls under a held row, report it (the page's scroll is disabled while dragging, and the handle blocks the native responder).
5. **Exact resume, Android.** Use a 1-hour VBR MP3 audiobook (Xing header) and an ADTS `.aac`. First make sure the facts have the index: `SELECT count(*) FROM audio_seek_index`, and see the cp-followups note on re-measuring.
   - Play the MP3 to 40:00, stop, then resume from the title page.
   - Pass: it starts at the same word, not seconds away. Logcat or the proxy shows one `GET …/audio-index`, then a Range request starting near the entry's offset (not at byte 0).
   - Seek within the track: lands on the word asked.
   - ADTS: resume part-way. Before this, it started from 0; now it lands.
   - Repeat with gapless on (the render path) and off (the plain path).
   - An Ogg Opus audiobook: unchanged, exact as before.
6. **Exact resume, Apple (cannot be compiled here; check on the Mac first):**
   - Build both schemes (`Portico` iOS and `Portico-tvOS`) in Xcode. `PorticoAudioRender.m` must compile with no warnings in the new code:
     - the `ARIndexFetch` class above `ARDecoder`;
     - the static functions `ARShiftedRead`, `ARShiftedSize` and `ARIndexInput` inside `@implementation ARDecoder` (they read ivars, as `ARRead` does);
     - the `MIN(_ixSkip,(long long)got)` cast.
   - `PorticoListeningOwner.m`: the `renderCommand` change uses the file-static `V1DecodePlan` and `V1Dictionary`.
   - Run the §18.8 conformance harness (`PORTICO_AUDIO_CONFORMANCE`, Debug build) to confirm start-from-0 decoding is unchanged.
   - Device: the same MP3 and ADTS resumes as in note 5.
     - Pass: lands on the word.
     - Charles or Proxyman shows `audio-index` once per track, then Range requests from the entry's offset.
     - A seek back to 0 decodes as before (the indexed decoder is left).
     - Gapless joins into the next track are unchanged.
   - HE-AAC ADTS (if you have one): the landing is right (the decoder's rate is scaled).
   - Then play with gapless, crossfade and levelling all **off**, and start part-way in an MP3: the decode engine is used (the Now Playing timeline still works, and pause and seek work).
7. **Forgot PIN.**
   - On a phone, choose a PIN-protected profile in the picker and press **Forgot PIN?**. Pass: the recovery sheet appears, a new 4-digit PIN saves, "PIN changed", and the new PIN then opens the profile.
   - In Settings › Profiles, as a profile that can't manage profiles, open a PIN-protected profile's menu: **Forgot PIN…** opens the same sheet.
   - On Apple TV and Android TV: the picker's PIN pad shows Forgot PIN?, and the sheet is focus-navigable.

## 9 Not finished

- **Nothing was run on a device, an emulator or the Mac.** That includes the drag gesture, the Media3 extractor at runtime (Kotlin was only type-checked) and all the Apple code (not compiled).
- **Forced only for downloads:** not offered, because there is no count offline. A download bundle could carry the count; that was not done.
- **Originals cached before this change** count 0/0 until they are cached again. There is no automatic re-count (no compatibility shim).
- **Android index use for seeks after a start from 0 (MP3):** the index is read only when the track starts part-way, so a later scrub in a track that started at 0 uses Media3's own placement. ADTS always reads it.
- **Ogg on Apple** is not decoded by this engine at all (unchanged).
- **Web Forced only** still decides from the cues it decodes. It does not use `/lines`; it didn't need to.
- **No render tests** for the new React wiring (collection entry menu, drag rows, Forgot PIN entry points, shapes).
- **Merge with `audit-implementation/main`:** done, but there was nothing to bring in. Main had not moved past the branch base when this lane finished, so if the verification lane commits later, the orchestrator's merge will be the first real one.
- **Not run:** the full `npm run test:core`, web `npm run check`, and the full Go suites (by the coordinator's instruction; the combined run covers them).

## Commits

portico-server (`audit-implementation/cp2-native`):
- `780c3848` Count an image subtitle's forced lines when its original is kept, so a device offers Forced only just for tracks that mix forced and ordinary lines
- `0d414e5e` Classify the subtitle line count route with the other grant-scoped subtitle files

portico-react-native (`audit-implementation/cp2-native`):
- `8443026` Sorted My List, Favorites and History draw cards in their kind's shape, as the lists' own order does
- `87242b9` Remove a title from a personal collection on phones, tablets and TVs, with Undo that puts it back in its place
- `69281ae` Reorder a collection on phones and tablets by dragging a row's handle; the row menu stays for moving without dragging
- `cb5cb93` Resume and seek in MP3 and ADTS audio exactly on phones, tablets and TVs, from the server's seek index
- `5a657f3` Forgot PIN on phones, tablets and TVs: from a profile's menu in Settings and from the PIN pad when choosing a profile
- `c353482` Offer Forced only in the player just for image subtitles that mix forced and ordinary lines (the server's count), with the shared packages that carry it
- `e165c9b` Record the server commit the shared packages come from
