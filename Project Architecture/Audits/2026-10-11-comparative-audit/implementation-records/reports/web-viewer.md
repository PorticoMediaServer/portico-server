# Lane report: web-viewer — the viewer-facing web screens

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/server-web-viewer`, branch `audit-implementation/web-viewer`, on top of `6f1530a`. Twelve commits, `6b37a22` … `a7144ed`; the working tree is clean. `web/dist` (ignored) holds the last build. Nothing pushed. Execution host: `ehler-personal` (mini-1, Ubuntu). No subagent and no helper model was used.

**How far it was checked.** `npx tsc --noEmit` is clean in `web/` and `packages/client-core`. `lint:strings`, `lint:raw-errors`, `lint-glossary` and `lint:design --strict` are clean. The test files for what changed were run one at a time or in small batches and pass (list in section 8). Unlike the earlier web lanes, **this lane was looked at in a real browser**: a disposable loopback server with generated credentials and synthetic media, driven through headless Chrome, both with the repository's own driver (`web/test/integration/browser-smoke.mjs`, full run: every group passes, see section 8) and with my own screenshots at 1440×1000 and 390×844 (`_program/scratch/web-viewer/run1/screenshots/`). What was *not* seen in a browser is listed in section 9.

Two test files fail on the starting point and still fail, for reasons that are not this lane's: `web/test/web-playback.test.ts` (17 of its tests; the start in its harness ends in `error`) and one test in `web/test/sign-in-gates.test.ts` ("a device the owner denied…"). I bisected both to `6f1530a`.

## 1. Outcome

**Sort and filter, the same everywhere (W1, W2).**

- Every library tab has the Sort control. **Recently aired** opens on "Latest episode · Newest first" and can be re-sorted like any other tab; the choice is remembered for the tab, like the main view. No tab hides Sort any more.
- **Genres** (music) and **Series** (audiobooks) are read through the same windowed list as every other tab, so they page however many there are (the 200 cap is gone), and they carry the toolbar: sort by name, title count or recently added, and filter by anything a member can be filtered by.
- A **category link** (a genre on a title page, a decade, a studio, a series) now opens the tab that lists those titles with the category as one ordinary filter chip, so it has every sort and filter. The old narrow "category browse" with its three sorts is only a fallback for a category kind the app does not know.
- The toolbar is on every group page, kept to the group (clearing filters never leaves it):
  - a **collection's** titles; an **author's** books;
  - an **artist's** albums, and separately the artist's songs;
  - an **album's** songs;
  - a **season's** episodes (or the whole show's, for a show without seasons);
  - a **person's** titles;
  - a **playlist**; a **saved view** (its own conditions stay in force underneath);
  - **My List** and **Favorites**.
- A group page keeps its own order until something is chosen (the collection's order, the season's, the playlist's, an artist's albums by type). That order is the first entry in Sort ("Collection order", "Episode order", "Playlist order", "Default order"). Choosing a sort or a filter swaps the list for the server's, drawn with the same cards or rows the page already uses.
- Lists that **span libraries** (a person's titles, a playlist, My List, Favorites) are sorted one library at a time: a row of chips ("All libraries", then each library that holds part of the list, with its count) appears when more than one does. With one library, its toolbar is simply on the list.
- Sort and filter names all come from the catalogue now. Before, "Community rating", "Personal rating", "Last played at", "Duration seconds" and "Added" were the server's ids spelled out; they read "Audience rating", "Your rating", "Last played", "Duration", "Date added". Direction words follow what is measured ("Most first", "Largest first", "First to last").
- **Random** draws one shuffle for as long as the app is open (paging and Back read the same one); choosing Random again draws another.
- The Playlists lists (Saved, and a music library's tab) offer the one sort the server gives them, drawn as the same Sort control. My List's and Favorites' own sort is drawn the same way.

**Undo (W3).** Marking a show, season, album or book watched or unwatched, from its page *or from a card's menu*, shows one toast at the bottom of the window ("Marked "Long Show" as watched. Undo") for as long as the server takes it back. Undo sends the mark's own receipt to the undo route; it never "marks the other way". If the server no longer offers it, the toast says so and the mark stands.

**Subtitles (W4).** A subtitle choice, Off and a timing correction are one change to the playing session and nothing else. A refusal is said ("The subtitles couldn't be changed.") and the subtitles stay as they were. The web no longer calls the subtitle route to select anything (verified in the browser: two `PATCH /v1/playback/sessions/{id}`, no `PUT …/subtitles`).

**Labels (W5).** The player, the post-play card, lyrics, the seek bar and the skip prompt had about 60 English literals; all now read from the catalogue (most keys already existed and were simply not used on the web). The sentences that say what stopped playback (`playbackUserMessage`, 36 of them) moved from a table in `client-core` into the catalogue.

**From the browser run.**

- Two unnamed subtitle files in one language no longer read alike: they gain their format ("English · SRT", "English · ASS"), then a number if that is the same too. A subtitle's origin reads from the catalogue ("Beside the file").
- Two versions with the same headline no longer read alike in the Version control: they gain a bit rate, else a size, else "Version 2".
- Artist, album and author pages have the toolbar (above).

**Defects found while looking, and fixed.**

- **A card's menu for a show, album, book or artist had its My List, Favorite and Watched rows greyed out for ever** when opened from a library grid (the menu waited on a read that never starts). They work now, and the menu no longer makes a request that always answers 404.
- **"Add to playlist…" and "Add to collection…" from a title page's ⋯ menu closed the moment they opened.** They stay open now.
- **Episode rows were too far apart**, more so the longer the list took to load (160 px or more instead of 128 px on a desktop; on a phone 112 px or more instead of 79 px). The loading placeholder was being measured in again and again.
- **A title's facts were spread down the page** beside a tall Files list (Genres at the top, Added 400 px lower). They sit together now.
- **Music's Genres tab offered "In progress" and "Unplayed"** quick filters; music has neither.

**Quiet updates.** Downloads, Recordings and the channel guide are read again while they are open: on the one hint the server sends (a notice when a download or recording finishes or fails, or a recording conflicts) and twice a minute while the page is in front. What is shown stays until the answer is in; a failed read says nothing; nothing is read while the tab is hidden.

**Other.**

- A title starts without asking the server anything first. The Dolby Vision colour check before every video start is gone: the server no longer plays Dolby Vision in approximate colours, so the answer was always "no".
- Closing the tab on a live channel ends its session at once, instead of leaving it to its lease.
- A paused Watch Together group loads its title paused, before the first frame.

## 2. Decisions I made

1. **A group page opens in its own order, and the toolbar is how you leave it.** The alternative was to replace every authored list (an album's discs, an artist's albums by type, a playlist's order) with the engine's flat list. Keeping the authored list as the first Sort entry keeps every page looking as it did until the viewer asks for something else.
2. **The page's own order is not offered while a filter is on.** The server orders what it filters, and it has no sort for "playlist position" or "collection order". With a filter on, Sort shows the server sort in use (title by default). Listed in section 7.
3. **Lists that span libraries are sorted one library at a time, by chips.** A library's vocabulary and values are its own, and the browse route is per library. The chips use the same component as Search's and the person page's. "All libraries" is the list as it was.
4. **On a person's page the toolbar is offered under "All", not under Cast or Crew.** The server's person condition covers every credit; pretending it meant "as cast" would be wrong. Choosing Cast or Crew clears the sort and filters.
5. **A group's sort and filters live in the address**, like a library tab's (Back and a shared link keep them). They are not remembered between visits; a collection opens in its own order each time. Library tabs still remember, as before.
6. **A category is a filter, not a place.** A genre link opens the library tab with a removable "Genre: Drama" chip. That is what the Genres tab already did and what the earlier browser run accepted; taking the chip off shows the whole tab.
7. **For My List and Favorites in one library, that library's toolbar replaces the list's own three filters and six sorts** (it has all of them and more, except "Progress"). With several libraries, "All libraries" keeps the list's own controls.
8. **Favorites in a music library lists albums** (those favourited, or holding a favourite song), as the library's Albums tab does with the Favorites toggle. A favourite song is not listed by itself there.
9. **What a group already is, is not offered as a filter on it** (no "Favorites" toggle on Favorites, no "Collection" filter on a collection).
10. **Undo is one toast, everywhere**, in the look the Channels screen already used for its notices. The line that the console lane put at the top of the show and book pages is replaced by it, so a mark made lower down the page (a season) or from a menu shows its Undo where the person is. The menu closes on a whole-title mark so the toast can be reached.
11. **A refused subtitle change is said once in the player's notice line**; a session that has moved on (409) is read again and asked once more, silently, before anything is said.
12. **Version tie-breakers**: bit rate, then size, then the number the Files list already gives ("Version 2"). **Subtitle tie-breakers**: "Forced"/"SDH" when marked, then format, then a number.
13. **Quiet updates use the device's clock where the server sends nothing** (30 s, only while the tab is in front and input has paused). It runs through the same hub as every other quiet read, so it is gathered, retried and silent in the same way. It is a stand-in: when the server sends hints for these lists (section 7), the clock should go.
14. **`LibraryTab.fixedSort` is renamed `defaultSort`** in `client-core` (pre-release rule: change it, do not keep both). Native screens that read `fixedSort` will stop compiling when the package is synchronised, which is the intent: they must offer Sort too.
15. I corrected two catalogue sentences the lints refused once they were in the catalogue: "Retry to reconnect" → "Try again to reconnect", and "programme … Go Live" → "program … Go to live".

## 3. Bugs

My brief named items, not audit documents; by item:

- **W1** — done on the web, with the three gaps in section 9 (Home row pages, personal collections, a book's parts).
- **W2** — done. `fixedSort` is gone from `client-core` and the web.
- **W3** — done.
- **W4** — done on the web. The route and `SubtitleService.choose` / `.import` remain for the native apps (S2).
- **W5** — done for the web player and viewer screens and for `playbackUserMessage`. Not done: `SubtitleService`'s own error sentences and the queue player's, which the native apps read (section 9).
- **W6** — not in this lane's scope.
- **B-18.4** (Genres and Series stop at 200): fixed on the web.
- **B-07.10** (letter jumps for article-stripped titles): not touched.
- **A-17.33** (a read before every video start): fixed.
- Browser run: grouping parity — fixed; unnamed subtitle labels — fixed; indistinguishable version labels — fixed.
- `review-server-late`: paused Together load — fixed on the web; DVR channel picker continuation — not present on the web (the web has no caller of `DVRClient.channels`).
- `web-player` "not finished": live channel session on tab close — fixed; Dolby Vision pre-play read — fixed. The rest are in section 9.

## 4. API changes

None on the server; no route was added, changed or removed, and no server file was touched.

`packages/client-core` (shared with the native apps):
- `presentation/library-tabs.ts`: `LibraryTab.fixedSort` → `defaultSort`. New `libraryTabSort()`, `categoryDestination()`. The audiobook Series tab gains `opens: {tab: 'browse', field: 'series'}`.
- `presentation/browse-toolbar.ts`: new `browseNameId()`, `sortDirectionKind()`. `toolbarModel` treats every music tab as audio.
- `presentation/title-files.ts`: new `versionNames()`, `subtitleTrackNames()`.
- `freshness-hub.ts`: new `dependsOn.clock(name)` and `hub.note(dependency)`.
- `downloads-service.ts`: new quiet `revalidate()`.
- `playback-messages.ts`: sentences come from the catalogue (`playback.message.*`); same function, same codes.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

None. Sort and filter choices are in the address (group pages) or the existing per-tab memory (library tabs).

## 7. Needed from others

**Server (each is an exact missing piece; nothing here was worked around by guessing).**

1. **No membership condition for a personal collection** (`saved_resources` of kind `collection`). The browse vocabulary has `collection` (catalogue collections), `memberOf`, `person` and `list` (playlists), but nothing for these. Needed: a field (say `savedCollection`, checked for ownership and shares in SQL as `list` is) and its surface. Until then a personal collection's page has no toolbar.
2. **No membership condition for a Home recommendation row** (Recommended, Trending now, Picks for you): `/home/rows/{id}` has no toolbar. Either a `seeAll` destination for those rows, or a row predicate.
3. **No browse that spans libraries.** `POST /v1/libraries/{id}/browse` is per library, so a person's titles, a playlist, My List and Favorites are sorted one library at a time (decision 3). A cross-library browse for the `person`, `list`, `watchlisted` and `favorite` scopes would remove the chips.
4. **No server sort for a group's own order**: playlist position, collection order, episode order (season and episode number; `aired` is not the same thing for specials), an artist's albums by type. With them, a group's own order could survive a filter (decision 2).
5. **No role on the person condition.** `person` matches any credit. A role or department operand would let Cast and Crew keep the toolbar (decision 4).
6. **No pivot for a book's parts or a disc's tracks**, so those lists have no toolbar. (`songs` with `memberOf` = a disc id is untested; I left discs out.)
7. **No feed events for the guide, recordings or downloads.** The web reads them on a 30 s clock (decision 13). Needed: one hint each, to the profile, when the guide is brought in again (`live.guide`), when this profile's recordings or rules change, and when this profile's downloads change. The dependencies to re-point are `dependsOn.clock('guide' | 'dvr' | 'downloads')`.
8. **`GET /v1/items/{id}/detail` answers 404 for a show, season, album, artist, book or author.** The web no longer asks. If "Add to playlist" and "Add to collection" should be offered for a whole album or show, the menu needs to learn that from somewhere.
9. **People from local metadata have no page.** With `.nfo` cast and crew the credits exist (34 in my fixture) but `catalog_people` is empty, cast cards do not link anywhere, and search finds no person. Whether a person without a provider identity should get a page is a product call.
10. **S2 can go ahead**: the web is off `PUT /v1/items/{itemId}/playback/{session}/subtitles`. Remaining callers are `SubtitleService.choose` and `.import` in `client-core`, used by the native apps (including the Watch Together `reserve`/`select` path).

**Native lanes (N8, T1, V-lanes).**
- `fixedSort` → `defaultSort`; offer Sort on Recently aired (`libraryTabSort()` gives the opening order).
- Use `categoryDestination()` for genre, decade, studio and series links; `browseNameId()` and `sortDirectionKind()` for sort and filter names; `versionNames()` and `subtitleTrackNames()` for the Version and Subtitles menus.
- The group-page rules are in `web/src/app/group-spec.ts` (pure, tested): which pivot holds a group's members, the fixed condition, when a group is offered, and how a list that spans libraries is scoped. It should move to `client-core` when a second client needs it.
- `playbackUserMessage` now needs the catalogue's `playback.message.*` keys (they travel with `packages/i18n`).

**web-owner lane (shared files I changed, minimally).**
- `web/src/shell/Shell.tsx`: one line, `<WatchedMarkToast />`.
- `web/src/ui/Feedback.tsx` + `.module.css`: new `Toast` (the Channels screen's toast, moved to the kit; Channels uses it).
- `web/src/ui/Menu.tsx`: `AnchoredMenu` ignores a focus change in its first 600 ms (the menu-from-a-menu fix).
- `web/src/ui/index.ts`: exports `Toast`.
- `web/src/app/freshness.tsx`: new `useQuietRefresh`.
- `packages/i18n` catalogue: new keys under `player.*`, `playback.message.*`, `watched.*`, `web.group.*`; `lib.count` gains genres and series; `web.filters.direction` gains count, size and order.
- The rail's scan pill shows "0" while a scan has found nothing yet (`Shell.tsx`, `scanning && scanCount`). I left it; it is the shell's.

**Documents.** `Spec — Title Pages` and `06 Clients.md` should say that group pages carry the library toolbar and how a list that spans libraries is scoped; `Spec — Playback Protocol v1` §5.5 that the web selects subtitles only by session change; the M27 Dolby Vision pre-play warning is retired on the web.

## 8. Test notes for other environments

**Run here.**

- The repository's browser driver, full run on the build with W1–W5 in it: `first-run, subtitles, browse, groups, freshness, console, media, profiles, tokens, downloads, csp, screen-walk (57 routes × 2 widths), sign-out` all pass, exit 0. Output: `_program/scratch/web-viewer/driver1/`. A second full run on `57ece72` (everything but the last one-file commit, which was then checked by hand: Random on a library tab sends a seed and reshuffles) also passes every group, exit 0: `driver2/`. I extended the driver's `groups` mode: each group page must show Sort, and choosing Title must leave the member on the page (passes at both widths).
- My own disposable server (`_program/scratch/web-viewer/run1/`, launcher `dev-up.mjs`, extra media `more-media.py`; generated owner, loopback only) with 8 films, 2 shows, 2 artists and 5 albums, 2 authors, a playlist across two libraries, My List and Favorites entries. Seen working: Genres tab with toolbar; artist page (both toolbars, sorted both ways); author page; show page (own order, sorted, phone); Recently aired with the full Sort menu and Random; a category link landing as a filter chip; My List and Favorites with library chips; a playlist own/Films/Television; a show marked watched from its card menu → toast → Undo; subtitles chosen and turned off with only session PATCHes; a film starting with no pre-play read; add to playlist from a title page.
- Unit tests, all passing: client-core `library-tabs` (new), `browse-names` (new), `track-version-names` (new), `browse`, `library-memory`, `freshness-hub` (+1), `downloads-service` (+3), `playback-messages`, `local-metadata-only`, `i18n`; web `group-spec` (new), `watched-mark` (new), `subtitle-one-path` (new), `together-paused-load` (new), `video-kinds` (new), and the 26 existing web test files that touch a changed file (96 pass; the one failure is the `sign-in-gates` test that fails on the base).

**To check by hand (any browser; a server with real metadata).**

1. A library with more than 200 genres or series: scroll to the last; the count in the toolbar equals the number of cards.
2. A real **collection** page (my fixtures made none): Sort shows "Collection order" first; choose Year, add a Genre filter, clear it: the list never shows a title from outside the collection.
3. A **person** with titles in two libraries (needs provider metadata): chips appear with counts; pick one, sort by Year; choose Cast: the chips and toolbar go and the credits return.
4. An audiobook **series** card and a genre link on a title page: each lands on the tab with one chip.
5. An album with two discs: disc headings in track order; sort by Title: one flat list, and pressing a song plays on in that order.
6. Mark a season watched from the season's ⋯ menu, wait 35 seconds: the toast goes by itself. Mark again and press Undo: half-watched episodes come back half-watched.
7. Two browsers on one profile: ask for a download in one; within about 30 seconds it appears in the other's Downloads without a reload. The same for a recording set from the guide.
8. Watch Together: pause the group, then have a second member join: their player loads paused, with no sound before the group plays.
9. Safari and Firefox: the toast's position above the mini player; the library chips wrapping on a narrow phone.

Pass means the above with no error notice and no blanked list. Fail is any list that empties and refills, a toolbar that shows a title from outside its group, or a console error.

## 9. Not finished

- **No toolbar** on: a Home row's full page (Recommended, Trending, Picks), a personal collection, a book's parts, a disc. Each needs the server piece named in section 7.
- **Not seen in a browser**: the person page's toolbar (no people in local-metadata fixtures), a collection page's toolbar and an audiobook Series tab with entries (my fixtures produced neither), a saved view's toolbar, Favorites in a music library, the quiet updates' timing, the Together paused load, the live-channel tab close. These are covered by unit tests of their rules, or only by reading.
- **Bulk selection on a group page's sorted grid** was not looked at.
- **A group's own order cannot be kept through a filter** (decision 2).
- **A group's sort is not remembered between visits** (decision 5).
- **`SubtitleService` error sentences and the queue player's messages** are still English literals in `client-core`. The subtitle image worker's two failure strings are internal and never shown.
- From the earlier lanes' lists, still open and not attempted here: a device-drawn subtitle is not carried into Picture in Picture or a cast; no "forced only" control; Up next during credits has no artwork or countdown; MP3, ADTS and Ogg deep resume; output-latency compensation; media keys with the music engine (B-17.19); Safari native HLS; saved lists, search and the person page are not kept after Back; Watch Together is not on the freshness hub; the five `refresh-required` notices.
- **The scratch server and browser are stopped.** Their state, media and screenshots are kept under `_program/scratch/web-viewer/` (generated credentials only; outside Git).
- **Whole suites were not run** (rule 3). The two test files that fail on the base still fail.
