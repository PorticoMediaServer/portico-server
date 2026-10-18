# Lane report: native-screens

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/native-native-screens`, branch `audit-implementation/native-screens`, 13 commits on top of `ec357b6` (59 files, +2147 −278). Work ran on personal mini-1 (`ehler-homelab-mini-1`). Nothing pushed, deployed, installed or built. No Swift, Objective-C or Kotlin was changed, and `packages/*` is untouched. Nothing was run on a device, a simulator or Vega: everything below is source and unit-test evidence.

# 1. Outcome

**Sort and filter in every grouping (N8, T1; the 14 October decision).**
- Library tabs: no tab hides Sort any more. Recently aired opens latest-episode-first and can be re-sorted; the choice is remembered per tab like any other. Music's Genres tab (and any tab of groups) now shows the bar with what the server offers for groups (name, title count, recently added).
- Category links (`genre:…`, `decade:…`, `studio:…`, `series:…`) open the library tab that lists those titles with the category as one ordinary filter, so they have every sort and filter. Same rule as the web lane's `categoryDestination`.
- Album and artist pages (their songs), author pages (books), collection pages, book-series pages, the show page (the selected season's episodes, or the whole show when it is one list), a person's page, a playlist and a saved view all carry the library's own Sort and Filter. Each keeps its own order ("Default order" in the Sort menu) until the viewer chooses a sort or a filter; then the list is the common browse route's answer with the group's membership condition always sent first. Clearing filters returns to the group, never to the library.
- On every device the group controls are two buttons, Sort and Filter (Filter opens the full grouped list the library has). On television they are a focus row between the hero and the list.
- Focus and position: where a page already had a windowed list (songs, episodes, a person's credits) the sorted list replaces it inside the same scroller, so the hero and controls are not redrawn and focus stays on the control. Where a page is made of sections (collection, author, series, playlist, saved view) the sorted titles are an in-page grid or list with "Show more", under the same header. A new sort keeps the old titles on screen until the new page arrives.

**Real Undo (N7).** Marking a show, a season, a book, or (from a card's menu) a show, season, album or book now shows a toast with Undo while the server's receipt is good. Undo posts the receipt to the undo route; it never marks the other way. An expired receipt says "The time to undo this change has passed."

**Appearance and captions (N9).** Card size (Small, Medium, Large; the web's three steps 0.82, 1, 1.25) scales posters, landscape tiles and square covers on shelves and grids, on phone, tablet and TV. Show backdrops off removes artwork behind title pages (they take the compact hero) and Home. Both rows, and subtitle Size and Background, are now in native Settings (Appearance; Audio & Subtitles). The last server values are mirrored on the device so the app opens at the size it will have.

**Engine setting change (N10, T4).** A changed Player setting now re-plans the playing session in place (`changePlayback({replan: true})`) after the capability document is published: same session, queue entry, position, pause and choices. Downloads and channels are untouched.

**Play on (N12).** The bar shows the playing device's own position (and length), read from `GET /v1/playback/controls`, and opens full controls: timeline, skip back and forward from the reported position, Start over, Play or Pause, Stop. A control the server does not list for this profile is not offered. Nothing is taken from the phone's player or guessed between reads; every action is followed at once by a fresh read.

**Offline (N6).** A download's player now offers every stored kind: DVB, VobSub (index plus its `.sub`), and a styled track with its attached fonts, in the engine's `render` command. The signed-out launch flow plays downloads in Portico's own two-engine player (the same `PlayerHost`), not the system player.

**Smaller items.** Play next and Add to queue on the title page; Watch Together loads start paused through the player's own `play` (and a same-title reload no longer starts playing); a running session that falls outside the account's allowed hours shows the hours state and carries on by itself when they return; cards that arrive after a row or in-page grid was first shown fade in; a role change drops what the viewer had been shown; the metadata editor's remaining English is from the catalogue.

# 2. Decisions I made

1. **Group controls are two buttons everywhere**, not the full chip bar: a group page already has a hero and an action row. Filter still opens every filter the library has.
2. **A group's own order is the default and is named "Default order"** (existing key `web.saved.sort.other`). It is offered only while no filter is on; a filtered list is always in one of the server's orders.
3. **Cross-library groups are sorted one library at a time.** A person's page and a playlist show a library button first in the bar when their titles are in more than one library; it defaults to the library holding most of them. Switching library starts again from the default order.
4. **A person's role filter (All, Cast, Crew) is hidden while a sort or filter is on.** The browse route cannot ask for a credit role; pretending would be dishonest.
5. **Sorted lists play in the order shown** (loaded entries, no container request). "Play season" and the hero's Play keep the season's own order.
6. **Group choices live with the screen** and are not remembered between visits. Library tabs keep their existing per-tab memory.
7. **Reorder and remove on a playlist are offered only in its own order.**
8. **VobSub index is stored as `.idx`** (was `.vobsub`). The player finds the pictures by swapping a three-letter extension for `sub`, and the engine contract says "the `.idx` address, the `.sub` beside it". Under `.vobsub` the pictures could never be found. Downloads completed earlier are completed again on the next pass (marker raised to 3).
9. **A VobSub track is offered only when both files are on the device**; a styled track as soon as its script is, with whichever fonts have arrived.
10. **Card size uses the web's three steps**, not the raw percentage.
11. **Play on position is shown as read, not extrapolated.** Polling is every 5 s, every 2 s while the full controls are open.
12. **Television fade starts at 35% opacity**, because a fully transparent view cannot take focus. No fade on first showing, on scroll, under Reduce Motion, or when more than 60 cards change at once.
13. **A role change gets a new document pool** rather than a navigation reset: open screens re-read in place.
14. **Metadata editor wording follows the catalogue** where its sentence differs (credits: "Open a movie or episode"; artwork count: "N to choose from"; unknown artwork role: "Picture").
15. **Undo toast wording reuses `title.seasonMarkedWatched`** with the show's or album's name, and `title.finished` / `title.notFinished` for a book. Neutral keys are proposed in section 7.
16. The cheap helper was used once (metadata editor literals); its diff was read line by line against the catalogue, and I then finished the near-miss cases by hand.

# 3. Bugs

No bug register was assigned to this lane. Found and fixed while working:
- Local VobSub subtitles could never draw (index stored under a six-letter extension).
- DVB subtitle files were stored with a download but never offered.
- A Watch Together reload of the same title while paused started playing (the pause was given to the old playback, then undone by the new play request).
- `fieldLabel` named sorts that are not fields from their id ("Count") instead of the catalogue ("Title count", "Recently added", "Random").
- Genre cover cache was shared across viewers on one device; it is now per viewer and bounded.
- Browse vocabulary cache was not keyed by role.

# 4. API changes

None. New consumers of existing routes: `POST /v1/containers/{kind}/{id}/personal-state/undo`; `GET /v1/playback/controls`; `PATCH` session with `replan: true`; `POST /v1/libraries/{id}/browse` with `memberOf`, `collection`, `person`, `list` and saved-view scopes; one more `GET …/browse-capabilities` per group page opened.

# 5. Schema changes

None. Device bookkeeping: a download's "extras complete" marker is 3 (was 2); device preferences gain `cardScale` and `showBackdrops` (mirrors of the server values).

# 6. Owner or viewer settings added or wired

| Setting | Type, default | Takes effect | Screen |
|---|---|---|---|
| `appearance.cardSizePercent` (existing) | Small / Medium / Large, Medium; per kind of device | Poster, landscape and square card widths everywhere inside the shell | Settings › Appearance, phone, tablet and TV |
| `appearance.showBackdrops` (existing) | switch, on; per kind of device | Title page backdrops, Home hero art | Settings › Appearance |
| `playback.subtitleSize`, `playback.subtitleBackground` (existing) | choices; profile | Already applied by every engine | Settings › Audio & Subtitles (were player-only on native) |
| Player setting (existing) | unchanged | Now re-plans the playing session in place | unchanged |

The three Settings additions come from a native adapter (`apps/native/src/screens/settings/native-structure.ts`) because the shared structure still lists these rows for the web only.

# 7. Needed from others

**Shared packages (canonical copy, then re-sync; each lets a native stand-in go)**
- `presentation/settings-structure.ts`: add `phone` and `tv` to `cardSize`, `appearance.showBackdrops` and the `appearance` subtitle section (without `subtitlePreview` off the web). Then delete `native-structure.ts`.
- `presentation/library-tabs.ts`: the web lane's `defaultSort` rename, `categoryDestination` and the Series tab's `opens`. Native reads either name already (`tabDefaultSort`), and its local `categoryDestination` / `groupTabOpens` in `app/library-view.ts` should then become imports. Note: native keeps `contains-any` for a genre link, the web draft uses `in`.
- `PlaybackService.play`: a starting state, so `app/paused-load.ts` can go.
- `HandoffSnapshot`: publish the committed handoff id (still wrapped in `app/play-on.tsx`).

**Catalogue keys that do not exist (nothing is staged in code; these are what better wording needs)**
- `settings.section.subtitleAppearance.device`: "The caption size and colours set in this device’s Accessibility settings are applied as well." (no note is shown today)
- `title.markedWatched`: "{title} marked as watched"; `title.markedUnwatched`: "{title} marked as unwatched" (today `title.seasonMarked…` carries a show's or album's name)
- `title.bookMarkedFinished`: "{title} marked as finished"; `title.bookMarkedNotFinished`: "{title} marked as not finished"
- `sort.ownOrder`: "Default order" (today `web.saved.sort.other`)
- `playOn.openControls`: "Show controls" (today `player.showControls`)
- `web.metadata.departmentDefault`: "Acting" (still a literal, used as a new credit's default department)

**Native engine owners (out of this lane's boundary: no engine changes)**
- Local styled subtitles ignore their fonts. `Native/PorticoMpvController.swift` (`if kind == "ass", !url.isFileURL, let fonts …`) and `PorticoMpvController.kt` (`if (kind == "ass" && !local && fonts.isNotEmpty())`) fetch fonts for streams only. JavaScript now sends `render.fonts` as `file:` addresses inside the download's folder (`sub-<id>-<rev>.font<N>`); the engines need to check each with `allowedLocalSource`, place them in a folder and set `sub-fonts-dir` before `sub-add`.
- `PorticoDownloads.play` (AVPlayerViewController; `PorticoOfflinePlayerActivity`) is no longer called from the launch flow. Retire it once the signed-out player is accepted on devices.
- iOS partial-transfer continuation when the address changes (unchanged).

**Server**
- A way to sort and filter groups that span libraries or are rankings: My List, Favorites, a Home row's See all, a personal collection. The browse route is per library and has no membership field for these (`list` joins playlists only, as far as the source shows).
- The download bundle does not carry a font's name; fonts are kept as `font0`…`font15`.
- `GET /v1/playback/controls` has no duration; the title's length is read from the item.
- S1 (a saved view as a queue selector) is unchanged: Play on a long saved view still takes the first 500.

**Base health**
- 24 native tests fail and `test/channels-model.test.ts` hangs **at this lane's starting commit** (verified on a clean export of `ec357b6`): `capability-profile` 1, `native-playback` 10, `queue-step-group` 4, `queue-step` 1, `v1-resume` 3, `channels-model` 5. They fail inside the copied `packages/client-core/src/playback-v1/testing/fake-server.ts` ("Invalid encoded route parameter") and the guide reads after the last package refresh. Every test that fails on this branch also fails there; none imports a file this lane changed.

# 8. Test notes for other environments

Run here: native and Vega type-checks (app and tests) clean; both string lints clean; Vega 72/72; every test file added or touched passes (13 files: engine-replan, container-undo, settings-native-rows, card-scale, play-on-controls, paused-load, offline-subtitle-files, offline-extras, library-view, browse-pages, group-scope, arrivals, plus library-tabs and browse). Native suite without the hanging file: 567 of 586 pass; the 19 failures are the pre-existing ones above.

On devices (iPhone, iPad, Apple TV, Android phone and tablet, Android TV, Fire TV; Vega once it can sign in):
1. **Group controls.** Open an album, an artist, an author, a collection, a book series, a show, a person, a playlist and a saved view. Pass: Sort and Filter appear after a moment; Sort lists Default order first; choosing a sort or filter changes the list without the page jumping or the hero flashing; on TV focus stays on the button after the menu closes and Up/Down reaches the hero and the list; Clear returns to the same group. Fail: the whole library appears, focus jumps to the rail, or the page scrolls to the top. Check that sorted episodes and songs look like the unsorted ones (stills, numbers, progress): that depends on the browse route's entries carrying the same fields.
2. **Recently aired.** Sort control present; change it, leave, return: remembered.
3. **Category link.** Press a genre on a Discover row or title page: the tab opens with a removable filter chip.
4. **Undo.** Mark a half-watched season watched, press Undo within 30 s: the exact earlier mix returns. Wait past 30 s: the expired sentence. Repeat for a show, a book and an album from a card menu.
5. **Card size and backdrops.** Change each in Settings on phone and TV: cards resize in place, grids re-column, focus stays; relaunch opens at the saved size with no visible resize.
6. **Engine change.** Mid-episode, paused, with a queue: switch Player setting. Pass: same position, still paused, Up Next intact, one new generation.
7. **Play on.** Send to a TV, open the controls: the position follows the TV within 2 s, scrubbing and skipping move the TV, pausing with the TV remote shows here.
8. **Offline.** Download titles with DVB, VobSub and styled subtitles; airplane mode; play with mpv. Pass: all three kinds listed and drawn; VobSub shows pictures. Known gap: styled fonts fall back until the engine change above. Sign out, open Downloads from sign-in: the same player, not the system one.
9. **Hours.** With allowed hours ending in a few minutes, stay in the app: the hours page replaces the content within about a token lifetime; when hours return it clears and the screen reloads.
10. **Watch Together.** Change title while paused, and move to another moment of the same title while paused: no sound, stays paused.
11. **Fade.** Leave Home open during a scan: new cards fade in; on TV the focused card never loses focus.

# 9. Not finished

- **My List, Favorites, History, a Home row's See all and personal collections have no new controls.** My List and Favorites keep the sort chips the server's own list offers. See the server item in section 7.
- **A book's chapters, a disc, and an episode group that is not a season** have no controls: the server has no pivot or selector for them.
- **Playlists tab in a music library** is unchanged (no server vocabulary for a list of playlists).
- **Local styled-subtitle fonts are passed but not yet used by the engines** (engine change, out of boundary).
- **Fade-in covers shelves and in-page grids only**, not the large windowed grids (library, person, saved): they have no full key list to tell a new card from a scrolled one.
- **No offline next-episode queue.**
- **Hand-offs from earlier reports left as they were**: exact pre-play subtitle track identity, playlist occurrence anchor, a visible More affordance on cards, the search keyboard and Android edge-to-edge, generic-box TV layout. Each needs shared, server or native work, or a design call.
- **Group pages read the browse vocabulary on every open** (one small request each).
- **Nothing was visually checked.** Layout of the two-button row under each hero, and the sorted in-page grids, need eyes on a phone, a tablet and a TV.

## Question for the owner

N1 to N5 and T2 (owner administration on phones and TVs: integrations and tokens, member deletion and schedules, metadata attention and library removal, runtime and diagnostics panels, Live TV source configuration) were not built, per the earlier decision reserving broad owner administration for the web. **Should the native apps get these owner panels, and if so on phones and tablets only, or on television too?**
