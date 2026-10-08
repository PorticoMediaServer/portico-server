# Frontend rework — log (from 2 Oct 2026)

The record the handoff asks for (`2026-10-02-frontend-rework-handoff.md` §9): what was built, each
decision made where the handoff was silent, every server/API/schema change, what was not done and
why, the functional checks that were run, and open questions. One section per pass and platform.

Commits are in `apps/portico-server` on branch `frontend-rework` unless a repository is named.

---

## Pass 4 — title pages: web and server

**Commit:** `30af2f0` (portico-server). Specs and backlog in portico-internal (same branch).

### Built

| Area | What | Main files |
|---|---|---|
| Action row | One row on every title: Play/Resume · Watchlist · Favorite · Watched · ⋯. Shows now carry Watchlist and Favorite; albums, artists and books carry Favorite. Music has no "watched"; a book says "Finished". The ⋯ never repeats the row | `client-core/presentation/detail-actions.ts`, `web/screens/detail/Detail.tsx`, `Show.tsx`, `library/Entity.tsx`, `shared/EntryActions.tsx`, `shared/container-watched.ts` |
| Movie page | Tagline on its own line; Files in Details (what the file is, size, bitrate, frame rate, every audio and subtitle track, path for owners); the thin Media information dialog and its menu item are gone; Refresh metadata is in ⋯ | `Detail.tsx`, `detail/TitleFiles.tsx`, `ui/TitleHero.tsx`, `client-core/presentation/title-files.ts` |
| Show page | The movie's action row; facts are year · seasons · genres · network; "N of M watched" beside the season switcher; Next up pinned above the list when it is not the first row; rows take focus (Enter plays, Up/Down move); placeholders tinted per show; rows line up with the heading; a link without `?library=` resolves it | `Show.tsx`, `EpisodeCard.tsx`, `Show.module.css`, `client-core/presentation/title-page.ts` |
| Episode panel | Files list, ⋯ (download, playlist, queue), tinted placeholder, catalogue fallback title | `EpisodePanel.tsx` |
| Album, artist, audiobook | Favorite; one row (Play · Shuffle · Favorite · [Finished] · Download · ⋯); the album's artist link in the title colour on the blurred cover (a `TitleHero` `onContext` link, not a one-off); the release type said once; "Songs" heading on an artist's list; "Finished" / "Not started"; the artist initial skips "The" | `Entity.tsx`, `ui/TitleHero.tsx`, `app/library-words.ts` |
| Pick before play | Subtitles lists the file's own tracks (with "Automatic" as the untouched default); the choice is applied in order and is what plays | `PlaybackChoices.tsx`, `player/Player.tsx`, `player/options.ts`, `client-core/presentation/language.ts` |
| Strings | `kindLabel` removed (unused); resolution and rating-source labels, "Episode", stereo/mono and the rest go through the catalogue | `i18n/catalog/en-US.ts`, `title-files.ts` |

### Bugs found by looking at commit `4782bfc` in the browser, and fixed

1. **Pick before play did not play what was picked.** Three separate causes:
   - The Subtitles control never appeared: it read the session's subtitle plan, which does not
     exist before Play. It now lists the chosen file's subtitle tracks, picked by language.
   - The subtitle choice compared the stream's `fra` with the plan's `fr` and was dropped. There is
     now one matcher (`sameLanguage`, client-core) for `fra` / `fr` / `fr-CA`.
   - On a server without an audio plan, naming an audio track starts the title again; the subtitle
     choice had been applied to the session being replaced. The player now settles audio first and
     applies subtitles to the session that follows.
2. **The player stacked subtitle lines on every re-render** (older bug, exposed here): it disabled
   its text track and then tried to remove the cues, but a disabled track lists none. Cues are now
   removed first.
3. **The Audio pill always said "Default"** when the server offers a plain track list: the web never
   told the player which track was in use. It now names it ("Français · French · AAC Stereo").
4. **A show's ⋯ did nothing visible** on the first click only because the menu's code loads lazily;
   it works, and it no longer repeats Watchlist/Favorite/Watched.
5. Tagline ran into the synopsis; album facts said "Album" twice; the album action row wrapped to
   two lines; episode rows were indented one gutter too far.
6. **A watchlisted show did not appear in Saved** (server; see below).

### Server, API and schema changes

| Change | Detail |
|---|---|
| `GET/PUT /v1/containers/{kind}/{id}/personal-state` | `kind` also accepts `artist`. GET adds `watchlisted`, `favorite`, `libraryId`. PUT takes exactly one of `watched`, `watchlisted`, `favorite`; the flags are plain values (last write wins, `expectedRevision` ignored for them). Watchlist: shows. Favorite: shows, albums, books, artists. A season refuses both. Stored in the title's `personal_items` row, where Not interested already sat |
| Saved lists, Home's Watchlist and Favorites rows | Include saved shows, artists, albums and books (they joined playable items only, so a saved show vanished). Entries are hydrated the way recommendation rows are |
| Recommender | Needs no change for the new flags: a work's signal already includes its own row (favorite +2, watchlist +0.5). Like (+1.5) and Dislike (−2, hidden) terms removed in `rec_profile.go`, `recommendation_engine.go`, `recommendation_pool.go` |
| Personal state | `reaction` removed from `PersonalState`, `PersonalMutation`, the `reaction` detail action, and `downloads/progress.go` |
| Migration `0003_drop_reaction.sql` | Drops `personal_items.reaction` outright (recreates the one trigger that named it) and deletes offline evidence about it. No data carried over |
| `GET /v1/items/{id}/detail` | Adds `files[]`: id, path (owners only), size, container, duration, bitRate, available, video {codec, width, height, dynamicRange, hdr10Plus, frameRate, bitDepth, bitRate}, audio[] and subtitles[] (embedded and sidecar). `detail_files.go` |
| TMDB credits (`metadataprovider/tmdb.go`) | A record still carries 128 credits, but the best-ranked 24 key crew (director, writers, creators, composer, producers) are held before the rest fills in. Before, cast plus TMDB's crew order (stunts first) pushed the director out |
| Migration `0004_refetch_tmdb_credits.sql` | Forgets TMDB document freshness so each title is fetched again under the new rule at its next refresh |
| Contract docs | `server/api/openapi.yaml`, `jobs.openapi.yaml`, `guide/05-…md` updated to match |

Existing tests that named the removed column or field were edited to the equivalent signal
(`related_fresh_test.go`, `personal_batch_test.go`, `recommendation_engine_test.go`,
`rec_feedback_test.go`, `catalogtest/taste.go`, client-core `detail.test.ts`,
`personal-labels.test.ts`). No new tests. Not run (final suite at the end). Known to need
attention then: `client-core/test/detail-actions.test.ts`, `entry-actions.test.ts` and
`detail-extras.test.ts` still mention `reaction` or expect Watched inside More.

### Decisions where the handoff was silent

1. **Flags are written on the container route, not through the item mutation.** The handoff says
   to extend the container route. The flags have no revision fence of their own there: they are
   booleans set to an explicit value, so last write wins. Storage is the same `personal_items` row
   an item uses, which is why the recommender and Saved needed no second path.
2. **Watchlist is offered for shows only; Favorite for shows, albums, books, artists**, exactly as
   the handoff lists them. An audiobook has no Watchlist button (backlog: "Want to listen").
3. **"Mark as played" is gone from the album page.** The review says music never has "watched";
   the previous commit had kept a played toggle.
4. **Download stays on the album and audiobook row as an icon**, because those pages have no other
   place for it and the row still fits on one line. On a movie it remains in ⋯, as before.
5. **Subtitles default reads "Automatic", not "Off".** Before Play the page cannot know what the
   viewer's subtitle preference will select, and "Off" would be wrong for someone on Always.
6. **The show's colour is derived**, a stable hue from the show id (`titleTintHue`), because the
   server publishes no dominant colour. Backlog item added for the real thing.
7. **The show hero drops "0 watched · 4 unwatched"** in favour of season progress beside the
   switcher; "Anime" is not printed as a network (pass 7 rule, applied here since the line changed).
8. **Episode ambition (item 7):** built all three — pinned Next up, season progress, keyboard rows —
   in the one list component.
9. **The 128-credit budget stays** and only its order changed. It contradicts "credits are never
   cut off" for very large productions; raising it touches record validation and storage budgets,
   which belongs in the backend audit. Open question below.
10. **`0004` only forgets freshness**; it does not wake every title's metadata work. Titles pick the
    fix up at their next refresh, or at once from ⋯ → Refresh metadata.

### Not done, and why

- **B13 on the demo data is only half fixed.** The provider now keeps the director (verified: a
  freshly matched title shows "Directed by Denis Villeneuve"), but Dune and Barbie each have a
  local `movie.nfo` that lists ten actors, and local metadata owns the whole `credits` field, so
  the online crew is fetched and deliberately not published. The page shows TMDB rows left from
  before the NFO took the field. This is a local-versus-online precedence rule, not a frontend
  matter; it is in the backlog with the two options. **Needs your decision.**
- **Version dropdown unverified.** A second file of a title is catalogued as a second title, so no
  local title has two versions. The control's code path is unchanged from `4782bfc`. Backlog.
- **Show years range, status, rating, first aired:** the catalogue stores none of them. Left out
  rather than shown empty. Backlog.
- **Cast portraits** are initials on local data (no portraits fetched); layout only.

### Functional checks (web, 1512×900, local server, owner account)

| Check | Result |
|---|---|
| Movie (Dune): facts line, TMDB badge, tagline, Starring line, cast capped at 12 with See all, Details, Files | Pass. No content-rating badge (the title has none stored) |
| Movie ⋯ | Rate, playlist, collection, Download, Edit metadata, Refresh metadata; nothing from the row |
| Freshly matched movie (test title "Interstellar") | "Directed by …", "Written by …", "Starring …" all present |
| Pick before play, two audio tracks and two subtitle tracks | Audio and Subtitles appear (no Version: one file). Chose French + French → player shows "Français · French · AAC Stereo", one French cue on screen. Before the fixes: English audio, no subtitles |
| Dropdowns absent when there is no choice | Pass (Dune's single-track file shows none) |
| Show (Breaking Bad): action row, Watchlist and Favorite persist across reload, appear in Saved › Watchlist | Pass |
| `/show/<id>` with no `?library=` | Resolves and loads |
| Episode list: still plays, copy opens the panel without scrolling to the top, hover Play and ⋯, watched check after marking, air date, "1 of 4 watched", Next up pinned as S1 E2 | Pass |
| Keyboard: Down, Down, Up across rows with a visible focus ring | Pass |
| Grid/list toggle, season ⋯ menu | Present; toggle not re-exercised after the change |
| Episode panel | Opens beside the list, placeholder tinted, crew lines, previous/next |
| Artist, album, audiobook | One action row each; artist link readable; "Not started" |
| API: season refuses favorite; artist GET returns flags | Pass |
| `tsc --noEmit`, `lint:strings`, `go vet` on changed packages | Clean |

Test media added to the local dev library for these checks (disposable): `Interstellar (2014)`,
a four-minute clip with English and French audio and subtitles. `~/PorticoDev/server-pre-0003.sqlite`
is a copy of the dev database from before the migrations.

### Open questions for Justin

1. **NFO versus online credits** (B13, above): cast-only ownership with online crew filling in, or
   NFO owns everything and the other rows are removed?
2. **Credit budget of 128 per provider record:** keep, or raise it so "See all" is complete for
   large productions? (Key crew are safe either way now.)
3. **Watchlist for audiobooks** ("Want to listen"): wanted?
4. **Download on the album/audiobook row** as an icon, or inside ⋯ like a movie's?

---

## Pass 3 and pass 4 — Apple (iPhone and Apple TV)

**Commits:** portico-react-native `56053aa`; shared models in portico-server `c240952`, `46786e7`.

### Built

| Area | What | Main files (apps/apple/src) |
|---|---|---|
| Browse toolbar (pass 3 port) | Drawn from `toolbarModel`: one Sort control (field and direction), quick toggles, the library kind's promoted filters, "Filter" as a grouped list (About the title · Your activity · The file) with a second level of values, applied chips with Clear all. On Apple TV the two levels are a panel at the right and nothing is typed (fields that need typing are left out there) | `screens/BrowseFilters.tsx` |
| Filter controls, shared | How a field is chosen is one rule for every client: values with counts, yes/no, a few choices, or a range offered as presets ("Under 90 min", "8 or higher", "In the last 30 days"). Year is chosen by decade. The words on an applied chip are shared too | client-core `presentation/browse-toolbar.ts` (`filterControlOf`, `filterValuesField`, `rangePresets`, `appliedFilterLabel`) |
| Movie page | Facts line, score, tagline, credit lines, the action row (Play · Watchlist · Favorite · Watched · ⋯), cast capped at 12 with See all, related rows through `relatedRowsForPage`, Details and Files; the Media information sheet and Like/Dislike are gone | `screens/Detail.tsx`, `screens/title/TitleHero.tsx`, `TitleDetails.tsx` |
| Pick before play | Audio and Subtitles buttons that open a menu; the choice is applied by the same rule as the web (`advancePendingChoice`: audio first, subtitles on the session left standing) | `screens/title/PlaybackChoices.tsx`, `player/usePlayer.ts`, client-core `pending-track-choice.ts` |
| Show page | The movie's action row with Watchlist and Favorite; season progress beside the switcher; Next up pinned above the list on iPhone and iPad; episode rows with still, number and title, air date and runtime, synopsis, progress and check | `screens/Show.tsx`, `screens/title/Episodes.tsx`, `SeasonSwitcher.tsx` |
| Apple TV episodes | A focusable list of episodes with the focused episode's still, title, air date and synopsis large beside it, and an Episode details button | `screens/title/Episodes.tsx` (`EpisodeBrowser`) |
| Album, artist, audiobook | Favorite; "Finished" on a book; the artist link in the eyebrow; Details | `screens/Entity.tsx` |
| Entry menu | Rebuilt on the shared `entryActions`; a page's own ⋯ does not repeat the row | `player/EntryMenu.tsx`, `app/container-state.ts` |

### Bugs found and fixed on the way

1. **Choosing subtitles crashed the app.** The subtitle service was given the shared id generator, which calls `crypto.getRandomValues`; Hermes has no `crypto`. Pick before play was the first thing to choose a subtitle without the menu, so it hit it at once. The player now makes its operation ids the way the rest of the Apple app does.
2. **`portico://item/<id>` opened nothing for a show** (review B16), and the same for an album, a book or an artist. A link carries only the id; it is now tried as a playable title and then asked for by kind (the container state route answers with the library). Top Shelf cards for shows depend on this.
3. **Apple TV: the filter values list did not take focus** (it stayed on Done, since the values arrive after the panel opens).
4. **An applied Year filter read "Decade: 1950, 2000"**, on the web too. It reads "Year: 1950s, 2000s" now, from one shared function.

### Decisions where the handoff was silent

1. **No Version control on Apple yet.** Audio and Subtitles are offered before play; a title with two versions cannot be made locally (a second file becomes a second title, see the backlog), so the control could not be checked and was left out rather than shipped unseen.
2. **Fields that need typing are absent from the Apple TV filter panel** (Actor, Director, Writer, Author). The server offers no counted values for people, so on a TV they would need the keyboard. Backlog: counted people values.
3. **The pinned Next up row is not shown on Apple TV**: the list there opens on the next-up episode and shows it large.
4. **The Apple TV episode list is a bounded list inside the page.** React Native warns about a virtualized list inside a scroll view in development builds; the list has its own fixed height, so it still mounts only the rows near focus.

### Not done, and why

- **Genre on a show library offers nothing, and neither does Year on Anime**, on any client. It is server data (see the backlog: genres are stored for movies, songs and book files only; counts are per episode). The buttons are still shown, because the same fields work for Movies and the model does not know a facet is empty until asked. **Open question below.**
- Review items B7 (collapsed rail) and B8 (Jump to) on Apple TV are pass 8; the rail's icons are already shown.

### Functional checks

Both simulators run Debug builds from this branch against the **local** dev server (see "The demo" below for why).

| Check | Result |
|---|---|
| iPhone: movie page (facts, TMDB score, tagline, credit lines, choices, action row on one line) | Pass |
| iPhone: pick before play, French audio and French subtitles on the test clip | French subtitle line on screen once the crash above was fixed |
| iPhone: show page by link; action row; "1 of 4 watched"; Next up pinned; episode rows with stills, air dates, check on the watched one | Pass |
| iPhone: toolbar (Sort, toggles, Genre / Year / Studio, Filter); Filter list in groups; a field opens its values | Pass; Year on Anime is empty (server, above) |
| iPhone: profile page, playback page, a value picker | Pass |
| Apple TV: show page by link; action row; episode list with the large preview following focus | Pass |
| Apple TV: Movies toolbar on one row; Filter panel at the right; Year by decade with counts; choosing decades filters the grid (83 → 17) and shows the chip and Clear all | Pass after fixes 3 and 4 |
| `tsc`, strings lint (0 hard-coded, baseline 5), eslint on changed files (no errors) | Clean |

Apple TV was driven with the project's own remote driver (`experiments/cinema/tvos/RemoteDriverUITests`, an XCUITest that presses Siri Remote buttons from a command file), run against the "Portico TV" simulator. Its build products are under that project's `build/dd-driver`.

---

## Pass 5 — one Settings screen (web, iPhone, Apple TV)

**Commits:** portico-server `c240952`, `5f55ce3`; portico-react-native `56053aa`.

### Built

| Area | What | Main files |
|---|---|---|
| The structure | `settingsStructure({owner, platform, capabilities})` returns headings (Account, Server) → pages → sections → rows. Rows are a registry preference, a composite (one decision over several keys, or a value the device keeps), an action, or a custom panel. A registry key no row names does not appear. Also: page addresses, a search over rows, and the read/patch functions for composites | client-core `presentation/settings-structure.ts`; copy in `i18n/catalog/en-US/settings.ts` |
| Account pages | Profile & account · Playback · Audio & subtitles · Quality & data · Music · Appearance · Language & region · Privacy · This device · About, in the handoff's order and with its rows | same |
| Composites | Highest quality per network (Automatic / Original / 4K / 1080p / 720p / 480p), Playback method (Automatic (recommended) / Always play the original / Always convert), HDR (Automatic / Off), Card size (Small / Medium / Large) | `readComposite`, `compositePatch` |
| Web | `/settings/$section`: page list under Account and Server on the left with a search field, the page on the right; `/settings` opens the first page. Rows save on change with a brief "Saved". Ordered language pickers; a subtitle preview that follows size and background. On a narrow screen the list is the first screen and a page pushes | `web/screens/settings/Settings.tsx`, `SettingsRows.tsx`, `AccountPanels.tsx`, `Settings.module.css` |
| Web, Server heading | The ten pages are listed for owners and each shows the console panels that belong to it (regrouped, not yet reworked: that is pass 6). `/server`, `/server/<section>` and `/server/<section>/<id>` redirect to the page that took the section over; every in-app link goes to the new address; "Open server console" is gone | `web/screens/settings/ServerPages.tsx`, `app/server-pages.ts`, `app/router.tsx`, `screens/server/Server.tsx` |
| iPhone and iPad | A grouped list: who is signed in, then the pages under Account; each page pushes. The same rows with switches, value pickers and the language list | `screens/Settings.tsx`, `screens/settings/SettingsPage.tsx` |
| Apple TV | Pages in a column at the left, the page's rows beside them; a page opens as focus reaches it; moving back returns to the open page; label and value are on one line of about 750 points | `screens/Settings.tsx` (`TelevisionSettings`) |
| Removed | The registry-rendered Preferences screen on all three clients; the web hub and its Notifications page (the bell has them); Apple's Account and About screens became pages | — |

### Server, API and schema changes

| Change | Detail |
|---|---|
| `playback.subtitleMode` replaces `playback.subtitlesEnabled` | `off` \| `forced` \| `always` (default `forced`, which is what `false` meant). `off` selects nothing, not even forced tracks. `operations/preferences.go`, `playback/delivery_context.go`, `subtitles/autoselect.go` (`AutoChoice.Off`). No stored value is carried over (pre-release). The bootstrap contract fixture is updated |
| Planner statistics (not a Settings change; found while deploying) | `persistence/planner_stats.go`, `schema.go`, commit `407cb50`: see "The demo" below |

### Decisions where the handoff was silent

1. **"Away from home" covers two registry networks** (`wifi` and `unknown`); it shows the stricter of the two and writes both. A browser cannot tell them apart, and an Apple device reports Wi-Fi away from home as `wifi`.
2. **A limit writes mode, height and clears the bit-rate ceilings.** The registry's defaults cap "other networks" at 20 Mbps as well as 1080p; leaving a ceiling behind would quietly lower the limit a person had just chosen.
3. **A stored combination only an API client could have written reads as the nearest choice** in a composite (never a fourth "Custom" option). A plain preference row whose value is not among its choices shows that value as an extra option.
4. **Left off Apple because Apple does not act on them:** subtitle size and background (the system player styles subtitles from Accessibility), backdrops, card size, remember searches, the notification dot. Reduce motion and (Apple TV) text size and "Play from other devices" are the device's own values there.
5. **Not in Settings at all:** `privacy.includeInWatchTogether` (not enforced by the server), `music.defaultSpeed` (the handoff: no music speed), `playback.startedThresholdPercent` and `playback.sleepTimerMinutes` (the player's own control). They stay in the registry for API clients.
6. **Watched threshold** is offered as 80 / 85 / 90 / 95 / 100 % under Advanced.
7. **Device name:** the web can rename this browser (its entry in the server account's device list), so the row is there for server accounts. On Apple the name is the system's and the row is read-only.
8. **Sign out is in Profile & account only** (one home). The Apple list keeps four entries that are not settings pages, because Apple has no profile menu to hold them: Switch profile, Switch server, Notifications, Watch Together, and Invite for owners. Pass 8 moves the bell to the iPhone top bar.
9. **iPhone and Apple TV do not show the Server heading yet**; the "Manage your server on the web" row stays until pass 6 builds the native pages. Nothing is shown that does not work.
10. **Old addresses:** `/settings/account` → Profile & account, `/settings/preferences` → Playback, `/settings/servers` → This device, `/settings/notifications` → the first page.
11. **Switch profile / Switch server** on the web are in the profile menu only, as decided; the special case that sent them through the old account page is gone.

### Not done, and why

- **iPad two-pane layout.** iPad uses the iPhone's pushed list for now; the handoff groups it with the web. Backlog (pass 8 candidate).
- **Downloads storage row** (phones) is built from the app's own figures; not seen in the simulator because the test profile has no downloads feature enabled there.
- Tests that name the old preference key or count registry-rendered rows are not updated beyond the bootstrap fixture; the final run will say.

### Functional checks

| Check | Result |
|---|---|
| Web 1512×900: every Account page opens; the list marks the current page | Pass |
| Web: add French then English as audio languages, move English up; "Saved" appears | Pass |
| Web: search "hdr" → one match → opens Quality & data with the row marked | Pass |
| Web: set Away from home to 720p, method to Always play the original, HDR Off; values read back; reset | Pass |
| Web: `/server/overview` → `/settings/server-dashboard`; People page shows People and Access panels | Pass |
| Web 375×812: `/settings` is the page list; a page pushes with a back link | Pass |
| iPhone: list, Profile & account (profiles, password, two-step, devices), Playback, a picker | Pass |
| Apple TV: two columns, focus down the pages, right into rows, a picker, left returns to the open page | Pass |
| `tsc` and strings lint on web and Apple; `go build`, `go vet` | Clean |

---

## The demo: two things found while deploying

1. **Every start pinned the demo's one core for more than 20 minutes** and heavier requests (`/v1/home`, the Top Shelf feed, a show page) timed out. Two goroutine dumps (19 and 23 minutes after two starts) were in the planner-statistics pass, sampling `live_programmes`. Cause, reproduced on a copy of the demo database: the pass had stored "0 rows" for tables that were empty at the time; SQLite then plans every lookup on such a table as a scan once it fills (a rowid probe on the 59,373-row guide table took 1.6 ms, not microseconds), and the pass's own 20,000 probes per index became 20,000 scans. **Fixed in `407cb50`:** an empty table gets no entry, as ANALYZE gives it none, and old zero entries are cleared when the database opens. On the copy: 703 zero entries removed, the whole pass 1.2 s, a second pass 0.4 s. This also removes a foreground cost, since guide queries were planned with the same entry. I took the dumps with SIGQUIT, which restarts the server; the demo was restarted twice by me for this.
2. **Both simulators lost their demo sign-in** ("Your sign-in to demo.getportico.tv has ended") when next opened after those restarts. The server logged `authorization family revoked … reason=explicit_signout` for two families at those moments and no `refresh_reuse`. A plain restart of the local server does not reproduce it, so the cause is not known. It is in the backlog for the backend audit. I did not sign them back in to the demo (not my credentials to type); both simulators are signed in to the local dev server instead, the iPhone with the local test account and the Apple TV by code approval from the local web app.

### Open questions for Justin (added)

5. **Genre and Year on show libraries** need server work (show-level genres and years in the browse index, facet counts of shows). Do it inside pass 7 (TV and Anime), or leave the two buttons and take it in the backend audit?
6. **`subtitleMode` default is Forced only.** Right default, or Off?
7. **The four non-settings entries on the Apple Settings list** (Switch profile, Switch server, Notifications, Watch Together): acceptable there until pass 8, or should Apple get a profile menu like the web's?

---

## Pass 6 — the Server heading (web, iPhone, Apple TV)

Commits: portico-server `1dbd7e9`, `3401323`, `d2ea099`; portico-react-native `7edbe5b`, `ae27e1f`.

### Built

- **Ten Server pages under Settings**, authored in `client-core/src/presentation/settings-structure.ts`: Dashboard, Libraries, Live TV & Channels (Sources · Recording · Library Channels), People (Accounts · Invitations · API keys), Playback, Remote access, Storage & backups, Schedule, Troubleshooting, General. A page is sections of rows; a plain setting is a `server-setting` row (form, key, label, help, control, condition); lists and status are `custom` panels each client draws.
- **One Save per page.** `client-core/src/server-admin/draft-form.ts` (`DraftForm`: load, edit, dirty, validate, save, discard; `FormSet`: a page's forms behind one Save and one Discard) and `server-forms.ts` (the nine settings documents as forms: runtime, connectivity, remote, certificate, alerts, live defaults, recording, maintenance, logs). The web shows a sticky bar at the bottom of the page; the iPhone shows a bar above the tab bar. Panels that keep their own draft (a library's settings, recording space, an account's limits on the web, who may record) join the page's Save through `useInlineForm`.
- **One refresh per page.** Web: `PageRefresh` in `ui/Settings.tsx`; the per-card "Updated … Refresh" lines report to it. iPhone: pull down.
- **Libraries.** A library opens as a page with General · Metadata · Scanning · Advanced (web: `?id=&tab=`; iPhone: a sheet with the same four). The list is in the owner's order, with Move up / Move down (web menu; iPhone "Change the order"), and every client's rail and tabs follow it. A library with files that need attention, or a scan with warnings, says so on its row and the label opens the list of files.
- **People.** One list of accounts; an account opens to role, libraries, sign-in, limits and its devices. Devices are managed in the account, not in a separate global list.
- **Schedule.** Scheduled jobs (what runs, in which windows, last run, next window) above the maintenance windows; extra schedules edit in a dialog/sheet.
- **Troubleshooting.** "Record more detail for 30 minutes" replaces the level menu, the debug window button and the component capture. Messages, records, capabilities, support export, viewer feedback and "Message everyone" follow.
- **`formatBytes` everywhere** in the console (the web's own `bytes` now calls it; transfer rates are `formatBytes(n)/s`).
- **The panels' rules and words are in client-core** (`server-admin/console-words.ts`, `dashboard.ts`, `status-words.ts`, `now-playing.ts`, `panel-logic.ts`, `capabilities.ts`, `recording-policy.ts`, `metadata-agent.ts`, `metadata-screen.ts`, `library-policy.ts`), moved out of the web so the iPhone app uses the same copy. The web files re-export the names their tests import.
- **iPhone and iPad: all ten pages, native** (`apps/apple/src/screens/settings/ServerPage.tsx`, `ServerForms.tsx`, `screens/server/*.tsx`, `app/server-console.ts`). The "Manage your server on the web" row is gone.
- **Apple TV: the Dashboard** is listed under Server for an owner session (see the open question about linked TVs).

### Server, API and schema changes

- **Library order.** `libraries.position` (migration `0005_library_order.sql`, schema version 5); `GET /v1/libraries`, the admin directory and Home rows order by `position, name, id`; a new library goes last. New route `PUT /v1/admin/libraries/order {libraryIds}` (owner; every id exactly once), in `api/openapi.yaml` and `route_lanes.go`.
- **Devices of one account.** `GET /v1/admin/access/devices?account=<id>` filters by account (`access/devices.go`, `admin-access.openapi.yaml`).
- **Message log level.** `servicelog` reads the level after an unknown `[tag]` prefix instead of filing the line as info.
- No setting was added that the server does not honour. Rows the old console showed as "not available on this server" are gone.

### Decisions where the handoff was silent

- **A setting the server does not have is not drawn.** DLNA and default subtitle/audio language for the server appear in the review's page list but the server has no such settings; they are left out rather than drawn dead. A whole group is left out, heading included, when its document answers 404.
- **File counts are not shown on the library list.** The only count the directory has is the last scan's processed files, which reads as the library's size and is not. The scan's own line on the Scanning tab keeps it.
- **Role, libraries, disabled and password of an account save from the account's dialog (web) or sheet (iPhone)**, since they are separate records with their own revisions; limits save with the page on the web and with the sheet on iPhone.
- **iPhone: the typed value of a setting is entered in a sheet** (the row shows the value). Lists of lines use a multi-line field.
- **iPhone, where a web control needs a file or a large table, the control is not offered** rather than offered broken: uploading a playlist file, uploading a channel logo, renumbering channels one by one, the Library Channel builder (templates are created as they stand), a new recording rule for another profile (rules are made from the guide; here they are paused, resumed and removed), choosing custom scan steps, the per-kind provider/language table of a library, restoring a backup from a browsed file (the path is typed), and the live tail of the message log (the newest 200 lines, pull to refresh). The support export goes to the system share sheet instead of a download. Connecting the server to a Portico Account opens the web page, because approval happens in a browser.
- **Server pages on Apple are for the owner role only**, as on the web (`viewer.role === 'owner'`).
- **"Direct play" on the status panel reads "Original quality"** (the glossary lint refuses the old term once it is in the catalogue).

### Not done, and why

- **Apple TV Dashboard not checked on the simulator.** A TV linked with a code gets a member session (see the open question), so the Server heading is absent there. The TV column renders the same `ServerPageBody` the iPhone does; type-check and lint pass.
- **iPad two-pane Settings**: still the phone layout (carried from pass 5).
- **A library with two metadata providers for one media kind** (the local Movies library has TMDB and TheTVDB both under `movie`) shows two rows with the same name. The server's validation allows one entry per kind and provider, not one per kind. Logged for the backend audit; the web no longer warns about a duplicate key.

### Functional checks

- Web (local server, owner, 1512×900 and 375×812): all ten pages render; Playback: toggle HDR conversion → the method row appears → Save → "Saved"; Libraries: Move up changed the list and the rail at once; a library's four tabs; "Allow deleting files" raised the Save bar and Discard cleared it; Live TV three tabs; Troubleshooting; General; People list. No horizontal scroll at phone width. Type-check, strings lint, design lint clean.
- iPhone simulator (local server, owner): the Server heading lists ten pages; Dashboard (watching now, alerts, health); Playback (status, authored rows, a typed-value sheet, Save bar on toggle, Discard); Libraries (list in order, a library's sheet, Scanning tab); People; Storage & backups; Troubleshooting; Live TV & Channels. Type-check and lint clean.

### Open questions for Justin (added)

8. **A TV linked with a code is a member session.** `identity/quick_connect.go` issues `TierMember` whoever approved the code, so an owner who links a TV does not get the Dashboard there (nor any owner-only control). Should a code approved by the owner carry the owner's role, or is a linked TV meant to stay a viewer? I left the server as it is.
9. **Two metadata providers under one media kind** in a library's settings: allowed by the server today. One per kind, or an ordered chain the page should present as "first, then"?

## Pass 7 — library types (web, iPhone; Apple TV by the shared code)

Commits: portico-server `b63a87c`, `5243472`; portico-react-native `1c28065`.

### Built

- **Library tabs are authored** in `client-core/src/presentation/library-tabs.ts`, Discover first and the default everywhere. TV/Anime: Discover · Shows · Recently aired · Collections. Movies: Discover · Movies · Collections. Music: Discover · Artists · Albums · Songs · Genres · Playlists (+ Unmatched for owners). Audiobooks: Discover · Books · Authors · Series. The Categories tab and its code are gone on web and Apple.
- **Recently aired**: episodes by air date, newest first, windowed and server-sorted; the tab has no Sort control and no letter rail. Cards read "S1 E2 · Sep 17 · Show".
- **List view is a table on the web** for every library (`web/src/screens/library/LibraryViews.tsx`): thumbnail, title, year, runtime, rating, resolution, date added; a header click sorts on the server. **Songs** is always a table (title, artist, album, duration, date added); on iPhone it is rows with the cover, "artist · album" and the length. The letter rail shows only for title sort.
- **Per-show settings** (hide seasons, number episodes across seasons, split into ranges of 100, newest first), stored per show on the server and edited by the owner from the season's ⋯ menu (web dialog, iPhone sheet). With absolute numbering the page says "Episode 1043" and "Play Episode 2", never a season.
- **Show and title pages**: the original title is a line under the title; "Anime" is never printed as a network (`networkName`); specials last.
- **Music**: a song starts in the mini player (web and iPhone; Apple TV has no mini player and opens full); the mini player gains previous, shuffle and repeat on the web, previous and a progress line on iPhone; the playing track is marked in song tables, album track lists and artist pages; Now Playing on the web centers the cover with lyrics beside it and the title once; music has no 10/30-second skip and no Speed (audiobooks keep both); artist pages group releases as Albums, Singles & EPs, Compilations, Appears on; genres are cards that open Albums filtered to the genre.
- **Audiobooks**: square covers for book, audiobook file, series and chapter and for the hero; cards name the author; book titles take two lines; "Finished" wording through `watchedWords(kind)`.
- **One caption rule** for cards and rows on every client (`client-core/src/presentation/card-caption.ts`): an album names its artist, a book its author, an episode its number, date and show, a movie or show its year; nothing reads "0 items".
- **Card marks**: no unwatched dot anywhere; a watched check and the progress bar (the Apple card gained the check).
- **Search**: the field and chips stay put; chips and result sections share one authored order (`search-model.ts`); "Audiobooks", not "Books"; a Top result block when one title is the words typed; years on movie and show results; stills for episodes; the query and the chip are in the URL on the web (`/search?q=&type=`).
- **Home**: Continue Watching first; its cards say "S1 E3 · 24 min left" with the show as the title; one **Recently added** row straight after it, merged across the libraries; then the recommendation rows. Customize Home reorders by drag on the web (the arrows stay for the keyboard).

### Server, API and schema changes

- **Sort `aired`** on the episodes pivot (expression sort on the episode's release date; `browse_contract.go`). Expression sorts now page by offset and skip letter anchors.
- **Browse entries** carry `year`, `rating`, `resolution` (from the best file's height), `airDate` (episodes), `artist` (albums), `author` (books), `album` (songs), `absoluteNumber` (episodes of a show numbered across seasons). `author` and the album `artist` are also filled on Home rows and search results (`catalog/entry_names.go`, one query per page).
- **Field `match`** on albums (`matched`/`unmatched`; unmatched = filed under "Unknown artist"), offered to owners only.
- **Search**: the all-groups preview is 12 per group (was 5); movie and show results carry `year`.
- **Show settings**: table `catalog_show_settings` (migration `0006_show_settings.sql`, schema version 6); the show workspace returns `settings` and, for a flat show, groups `all`, `range:<start>` and `specials` in place of seasons; `PUT /v1/admin/libraries/{id}/shows/{show}/settings` (owner; revision-fenced, 409 `show_settings_conflict`). In `api/openapi.yaml` and `route_lanes.go`.
- **Home rows**: new row `recent_all` ("Recently added", priority 30); per-library `recent_<library>` rows are hidden unless the profile's layout places them; the personal family is titled "More recommendations" in Customize Home; the generated "New for you" row is removed.

### Decisions where the handoff was silent

- **Default Home order — for Justin to confirm.** Featured, Continue Watching, Continue Listening, Recently added (all libraries), Watchlist, Favorites, Recommended for you, the personal rows, Trending now, Popular on this server. "Recently added in <library>" rows are off by default and can be switched on in Customize Home. A profile that had already saved a layout keeps what it saved.
- **One name per recommendation idea**: "Recommended for you" stays; the family switch in Customize Home is "More recommendations" (it was "Picks for you"); "New for you" is gone because Recently added plus Recommended already say it.
- **No "Plays" column** in the Songs table: the server keeps no play count per song. Backlog.
- **"Popular tracks" on an artist page is still headed "Songs"**: without play counts the order is not popularity, so the page does not claim it.
- **Unmatched** means albums made from untagged files (under "Unknown artist"). They still appear in the main Albums grid: leaving them out would turn every Albums browse into a filtered query without counted blocks or a letter rail. Backlog.
- **Recently aired has no counted ordering** (it is an expression sort). Fine at today's sizes; a multi-million-episode library needs one. Backlog.
- **Genre covers** are read per visible card (four on the web, one on Apple), not for the whole list.
- **A song opened from the Songs table plays alone**; an album's tracks still queue the album.
- **Episode captions lead with the number** ("S1 E2 · Sep 17 · Show"), so a narrow phone card keeps the number when the line is cut.
- **Per-show settings**: "hide seasons" and "ranges" both make the show flat; ranges are of 100 with Specials as a group of its own; absolute numbers skip specials; a flat show's hero says its episode count, not "1 season".
- **iPhone Songs** are rows, not a table with sortable headers; the Sort control above it drives the same server sorts. Genre cards on Apple show one cover, not a mosaic.
- **Apple TV keeps opening music full screen**, and never shows shuffle or repeat for video (done here rather than in pass 8, since the transport row was being edited).
- **Audiobook covers are square** per the 2 October handoff; my memory note that said posters is updated.
- **An empty Series tab** says "No series yet", not "Nothing in this library yet".

### Not done, and why

- **Drag to reorder on Apple's Customize Home**: the arrows remain; the handoff asks for drag "there" on the web dialog.
- **Scrubber, shuffle and repeat in the iPhone mini bar**: the bar is one row; it gained previous and a progress line, and the rest is one tap away in Now Playing.
- **Apple TV not driven through these screens in this pass** (the code is shared with iPhone and type-checks); it is exercised in pass 8, which is TV-heavy.

### Functional checks

- Web (local server, owner, 1512×900): TV tabs and Recently aired in grid and list; Movies list table with header sorting, rail only for title sort; Music Songs table, Genres, Unmatched, album captions with artists, an artist page; mini player start, playing-row mark, Now Playing; show settings dialog (hide seasons + absolute numbering → "Play Episode 2", flat list) and back; Search (Top result, sticky field, URL, years, author on the audiobook); Audiobooks Books tab with authors and square covers, empty Series tab; Home rows in the new order ("Continue Watching | Recently added | Watchlist | Favorites | Recommended for you | …"), Customize Home lists the per-library rows switched off and every row draggable. Type-check, strings lint and design lint clean.
- iPhone simulator: Home ("4 min left", Recently added, watched checks); Anime tabs and Recently aired; Music tabs, album captions, Songs rows with the letter rail, a song starting in the mini bar with previous/next; Breaking Bad with seasons hidden and absolute numbers ("Play Episode 2", "Episode 3 · …"), the Show settings sheet, saved back to seasons ("Resume S1 E2"). Type-check clean.

### Open questions for Justin (added)

10. **Default Home order** as listed above: confirm, or say where Watchlist and Favorites should sit relative to the recommendation rows.
11. **Play counts**: should the server keep a per-song (and per-title) play count so Songs can have a Plays column and an artist page a true "Popular" list?

## Pass 8 — platform polish (web, player, iPhone, Apple TV)

Commits: portico-server `7f05953`, `6b08677` (and the test and spec commits listed under "The very end"); portico-react-native `ff0d913`.

### Built

**Web**
- **Title cards are links.** The card's art is an `<a href>` built from the same route rule as navigation (`entryRoute` in `web/src/app/open.ts`), so middle-click, Cmd/Ctrl-click and the browser's own Copy link work; a plain click still opens in place; P and M still play and open More. Search results, person credits and cast circles have links too.
- **One page header.** `PageHeader` always reserves the line above the title (the way back, the eyebrow, or nothing), and Home (without a hero) and Settings use the same measure (`--page-title-top`, `--page-title-size`), so every page's title sits at the same height.
- **Card size** Small · Medium · Large: in the library toolbar (grid view) and in Appearance. The setting existed but changed nothing; grids and shelves now multiply their card width by `--card-scale`, and a windowed grid re-measures when it changes.
- **The mobile tab bar is opaque.**

**Player**
- **Playback info** under More, on the web as a panel over the picture's corner and on iPhone/Apple TV as a sheet: whether the file plays as it is, is repackaged, or is being converted and why (the session's own reason codes in plain words), video and audio codecs and size, HDR, container, bitrate, and on the web dropped frames and buffer, refreshed each second. One model in `client-core/src/presentation/playback-info.ts`.
- **Watch Together** in the web player's More menu (the title goes to the Together page, as from a title's menu). Play on / cast was already in the player's top bar and is unchanged.
- **Search for subtitles** in the web subtitle menu when the server has a provider (Apple already had it; the row reads the same on both).
- **The page behind the full web player is `inert`.**
- **Apple**: an episode's title line reads "Severance · S1 E2" with the episode title under it; no shuffle or repeat for video on Apple TV (done in pass 7).

**iPhone and iPad**
- **A navigation bar on every pushed screen.** Each stack screen has a scope (`ui/NavBar.tsx`); its scrollable reports the offset; the page header (or a title page's status-bar scrim) names the screen. Once the header or hero has scrolled away, a solid bar with Back and the title takes its place, so content never runs under the clock or the Dynamic Island. The status-bar backings are opaque.
- **Search, the bell and the account button** in the top bar of every tab (Home, Library, Saved, Downloads, and Channels after its Guide/List switch; portico-react-native `663a99e`); the bell has a dot while something is unread.
- **Home's backdrop scrolls away** with the page.
- **Empty states use the web's copy** (one set of strings; the Apple-only duplicates are deleted).
- **US spelling**: words are US English unless the profile chooses another catalogue in Language & region. The simulator, set to Canada, had been reading "Favourites".

**Apple TV**
- **The rail's focus uses the same bright ring as content.**
- **The browse header is two rows**: the title (with the count in its eyebrow), then one row with the tabs, Sort, Filters and Jump to. The tabs are one Up from the grid and Jump to no longer overlaps the first row of cards (review item B8).
- **Home refreshes whenever the app or the screen regains focus** (it was gated at five minutes).

### Decisions where the handoff was silent

- **Cmd/Ctrl-click on a card opens a new tab** (it used to select). Shift-click still selects a range, and the checkbox selects one. A card in selection mode is a button, not a link.
- **Right-click on a card still opens Portico's More menu**, not the browser's. The browser's Copy link is on middle-click/Cmd-click paths and in its own menu when the More menu is not taken (keyboard, long press on touch).
- **Playback info names the panel "Playback info"**: the glossary lint refuses "Playback information" in the catalogue.
- **Apple's Playback info has no dropped frames or buffer rows**: the native player does not report them to the app.
- **The web's subtitle search asks in the profile's language** and applies a result for this viewer only, as Apple does.
- **Words default to US English** even when the device region is not the US; formats still follow the device. en-CA remains a catalogue a profile can choose.
- **The accent token is the same on web and Apple** (`designColors.accent`, #409fd4). The simulator renders it in a wider gamut, which is why screenshots look more saturated; no code change.
- **iPhone filter sheet and the Apple TV filter panel** already were a grouped list with the current value on the right and a two-level side panel without text entry; on Apple TV the quick-filter chips moved into that panel so the header fits two rows.
- **iPhone show page action row** already matched the web (Play · Watchlist · Favorite · Watched · ⋯).
- **Collapsed rail (B7)** already showed icons with the selected destination marked; unchanged apart from the focus ring.
- **Cast circles** already showed initials when there is no portrait; confirmed on Apple TV and iPhone.

### Not done, and why

- **The Apple TV Dashboard is still unverified on the simulator**: the TV signs in with a code, which gives a member session (open question 8).

### Functional checks

- Web (local server, owner, 1512×900): a card's `href` (`/media/…?library=…`), P on a focused card started playback; the frame was `inert` under the player and not after closing it; More listed Watch Together, Playback info, Report a problem, Keyboard shortcuts; Playback info read "Original quality: the file plays as it is · H.264 · 640×360 · AAC · MP4 · Dropped frames 2 of 449 · Buffer 16 seconds"; Card size Large widened the cards from 174 to 212 px with no overlap, and back to Medium; every page's `h1` at 54 px.
- iPhone simulator: Home with Search, bell (unread dot) and account, the backdrop scrolling away under an opaque bar; Settings and a show page with the navigation bar after scrolling ("‹ Settings", "‹ Breaking Bad"); Continue Watching "Breaking Bad · S1 E2 · 1 min left".
- Apple TV simulator (re-linked with a code approved from the local web session): rail focus ring; Movies → Movies tab with the two-row header, Sort menu, Filters side panel (About the title / Your activity), a title page with cast initials, a show page.

## The very end — builds, full suites, specs, Backlog, deploy

Commits: portico-server `224570e`, `ebe827e`; portico-react-native `1b9d63d`, `663a99e`, `53ba2d5`; portico-internal `573db0f`, `2c713f7`, `a5d0349` and this one. Demo: `ebe827ef` (readiness 200); `5243472` is the rollback.

### Builds

- Web: `npm run check` (tokens, design lint, preference consumers, strings lint, type-check, tests) and `npm run build` pass.
- Apple: Release builds for the iPhone and Apple TV simulators succeed (`xcodebuild … -configuration Release`), from the final commit.

### Suites, run once, and what they flagged

| Suite | First run | After fixes |
|---|---|---|
| client-core (`npm run test:core`) | 1409 of 1412 | 1412 |
| web (`npm test`) | 317 of 396 | 396 |
| Apple (`npm test`) | 218 of 220 | 220 |
| Go, full preset on the runner (`runner-test.sh --full`, tier all) | 68 packages pass, 9 fail | every package passes |

- **client-core (3)**: the title page's quick actions now include Watched; words default to US English; audiobook covers are square. The tests were updated to those rules.
- **web (79)**: 59 came from the test harness, which could not load a web file that re-exports a rule from client-core (the console's rules moved there in pass 6) or that gained an import the test did not stub. The harness (`web/test/helpers/component-harness.mjs`) now loads unstubbed client-core modules for real, lays a test's stubs over them, understands `export … from`, and treats other unstubbed modules as inert. The other 20 pinned source text or helpers that passes 4–6 changed (Settings structure, Link a TV row, crew dialog, related rows needing three titles, log capture replaced by "Record more detail", `LibrarySettingsSections`); each was rewritten to assert the current behavior.
- **Apple (2)**: the Settings page is exempt from the reconnecting screen (`SettingsPage` added to the list); See all targets come from `libraryTabForPivot`.
- **Go**:
  - *Fixtures*: 41 test inserts wrote `INSERT INTO libraries VALUES(…)` with four values; the table has had `position` since pass 6. They now name their columns (access, administration, downloads, metadata, operations and most of httpapi failed for this alone). No production code inserts that way.
  - *catalog*: Home tests expect one Recently added shelf and the per-library shelves off until placed; search uses `SearchPreviewLimit`; a title has four actions (no reaction).
  - *persistence*: `catalog_show_settings` added to the schema allow-list; the planner coverage test now checks that every index is readable by the pass and that empty tables get no statistics (the rule since `407cb50`; the old assertion wanted statistics for every index of an empty database).
  - *sorttext*: a title with no metadata language sorts as English ("The Abbey" under A), as pass 3 made it.
  - *httpapi*: the read-path golden was re-recorded (`PORTICO_GOLDEN_UPDATE=1` on the runner). I diffed old against new before accepting it: 117 of 434 cases changed, all by added entry fields (`year`, `resolution`, `artist`, `author`, `album`), the Home row set, and the removed reaction action; no status, membership or ordering change in any browse case; four new `recent_all` cases (open and restricted viewers, two pages). The restriction matrix also covers `recent_all`.
  - *httpapi timed out in the full preset* at the preset's 30 minutes. It is not the rework: run alone with `-timeout 90m` and per-test timings, the package takes 2,082 s at the rollback tag and 2,080 s on this branch (354 tests, none failing, no test more than 3 s slower). Six queue-scale tests of about 204 s each are most of it. Backlog.
- Rerun after the fixes, only what had failed: the seven quick packages; the catalog tests named above plus every `TestSearch*`; the whole of httpapi (it had not finished the first time).
- **End-to-end**: the repository's one end-to-end script, `web/test/integration/setup-flow.mjs`, fails at "the checklist shows until setup is finished" (`state.ready` is already true after the owner is created). The setup code (`identity/onboarding.go`, `httpapi/onboarding.go`) and the script are unchanged since the rollback tag, so this predates the rework; I did not change either. Backlog.

### Specs and docs updated

- `06 Clients.md` (authored structure in client-core, Server pages under Settings on web and iPhone, card links, page header, card size, player rules, card marks, navigation bar, Apple TV header, US English default), `05 Recommendations.md` (personal rows naming, no "New for you", default Home order), `03 Catalogue and Metadata.md` (Home rows, authored library tabs, entry fields, `aired`, `match`, show settings), `Specs/Spec — Title Pages.md` (what of §4 is built).
- `apps/portico-server/web/docs/administration.md` rewritten for the Server pages.
- `Backlog.md`: the items listed under each pass's "Not done", plus the two below.

### Left for Justin

- The branch `frontend-rework` is not merged in any of the three repos, and the tag `pre-frontend-rework-2026-10-02` has not moved.
- Open questions 1–11 above. The ones that change what people see by default: the Home row order (10), a linked TV's role (8).
- Local test state: the dev server's library order was changed while testing pass 6 (Anime, Movies, Audiobooks, Music, TV Shows); Breaking Bad's show settings were switched on and back off; the Apple TV simulator was re-linked with a code.

## After the rework — Justin's answers, built the same day (2 Oct)

Justin answered four of the open questions (8, 10, 11 and the US-English default). This section is what was built from those answers, in the same form as the passes.

Commits: portico-server `21d46f3`, `56655f6`, `a27b814`, `8b0a11f`, `cc97fc9`; portico-react-native `1b04a50`, `b36d86d`; portico-internal `17a1d65` and this one.

### Built

- **"Featured" is gone, everywhere.** The row, its daily rotation, its cache, its index (`catalog_browse_feature`, dropped in migration 0007), its string, its place in Customize Home, the `featured` title of the old Home projection, and the "Featured" section of the Apple TV Top Shelf.
- **Home's hero is what the viewer was last in the middle of.** The Home document names it (`hero: {rowId, entryId}`): the first entry of Continue Watching or of Continue Listening, whichever was played more recently. Because it is those rows' own first entry, it is an unfinished title or the next episode of the show last watched, by the rows' existing rules. `client-core` `homeHero` (`presentation/home-hero.ts`) writes every word: the row's name above, the show as the title with "S1 E3 · episode title" under it, the artist and album for a song, the author for an audiobook, then year · rating · length · two genres, a progress line, and "Resume · 24m left" / "Resume" / "Play". A title with no wide artwork (a song, an audiobook) shows its cover beside the words over a softened copy of the cover. Web, iPhone and Apple TV draw it from that one model.
- **A device linked with a code is signed in as whoever approved it**, with that person's role (`identity/quick_connect.go`). The role is read when the device collects the session, so a role that changed in between is the one it gets. The approval screens say "It will be signed in as you".
- **Regional English.** Words follow the device's region again (the change that forced US English is reverted). en-CA and a new en-GB are **derived** from en-US by spelling rules (`i18n/src/catalog/regional.ts`) instead of a hand-kept list: the old en-CA file listed 7 messages while the source said "favorite" in about 40. The rule turns 39 messages for Canada and 76 for Britain. Canada keeps "-ize" and "program"; Britain gets "-ise" and "programme". Every other English-speaking region reads en-GB; English with no region, the US and the Philippines read the source. Language & region lists English (United Kingdom).
- **Play history.** The server keeps every play (`play_history`, migration 0007): who, what, which device and platform, when, how far, and whether it finished. The names are copied into the row, so it still reads correctly after the title, the account or the device is gone. It is kept in full by default; Storage & backups › What Portico keeps has a "Play history" row (empty = Forever, or a number of days), pruned 5,000 rows a tick.
- **Settings › Server › Play history** (web, iPhone, iPad): newest first, with filters for library, person and time (all time, 24 hours, 7, 30, 90 days) and the number of plays the filters match. Columns on the web: User, Type, Title (with its library under it), Player, Platform, Played (date, time, and "Stopped at 29%" for a play that was not finished). The iPhone list shows the same facts per row. Model, columns, filters and wording are in `client-core/src/play-history.ts`.
- **Play counts.** `play_counts` (per title, the whole server) and `personal_play_counts` (per viewer), both incremented once when a play completes, both seeded from existing history by the migration. Songs carry the viewer's `plays`; the web Songs table has a **Plays** column. An artist page's five most played songs read the counts: **"Popular"** (the whole server) when the owner has turned on activity shared across profiles, **"Your most played"** otherwise. That list now sits above the full song list on the web and on Apple.

### Server, API and schema changes

- Migration `0007_play_history.sql`: `play_history` (+ three indexes), `play_counts`, `personal_play_counts`; drops `catalog_browse_feature`. Schema version 7.
- `GET /v1/admin/play-history` (owner or administrator): `accountId`, `libraryId`, `period`, `cursor`, `limit` (1–200); `total` on the first page. In `openapi.yaml`, with a lane and a restriction classification.
- Settings field `playHistoryDays` (0–36500, 0 = keep everything, the default).
- Home document: `hero`; the `featured` row and row kind are gone (`home.openapi.yaml`, the browsing guide). A saved layout that still names `featured` has it dropped when read, so it keeps saving.
- Content entries: `plays` (songs, the viewer's own) and `author` on an audiobook file. Home's own rows now name albums' artists and books' authors like every other page of cards.
- `ContentEnvelope.featured` removed from the old Home projection (`GET /v1/content?view=home`).
- Quick connect: the issued session's role is the approver's (member for a profile that is not the account's first); `viewer_only` is granted only when the approving session was itself viewer-only.

### Decisions where Justin's answer was silent

1. **No hero when nothing is in progress.** A new profile, or one that has finished everything, sees Home open with its header and first row; nothing is substituted for the hero. (The two Continue rows are required rows and cannot be hidden in Customize Home, so a viewer cannot lose the hero that way.)
2. **The hero's title stays in its row** as the first card. Removing it would make the row disagree with the same row everywhere else.
3. **No Watchlist button on the hero.** It made sense on a suggested title; on something already being watched it does not. Resume/Play and Details only.
4. **The owner's play history records every play, including a viewer who paused their own history.** "Pause watch history" still stops the viewer's own History, watched marks and recommendations; its help text now says the owner can still see what was played. A viewer clearing their History does not touch the owner's record. Reason: you asked for the whole history of the server, and Plex records regardless. If you want pausing to hide plays from the owner as well, it is one condition in `AcceptPlaybackProgress`. **Open question 12.**
5. **A play is recorded once it has really started** (the point at which it would resume: 30 seconds or the viewer's started threshold, whichever is sooner), not when a title is opened and closed. It is updated until it ends; the count goes up once, when it completes.
6. **The log starts now.** Existing viewer history seeds the counts, but not the owner's log: those rows never recorded a device, and the personal key cannot be turned back into names in SQL.
7. **"Popular" is server-wide only when the owner shares activity across profiles** (`home.communityActivityEnabled`, the existing rule for anything built from other people's plays). Otherwise the list is the viewer's own and is headed "Your most played". **Open question 13.**
8. **Plays is not a sortable header yet** (Backlog), and iPhone song rows do not show it.
9. **A viewer-only session links a viewer-only device.** "That user gets logged in" should not give the new device more than the approving session had.
10. **Only spelling is regional.** "Movies", "Trash" and other vocabulary stay the same in every region.
11. **Apple TV Top Shelf:** the second section is now the viewer's next Home row under its own name (here "Continue Listening"; for most people "Recently added"), not a mix of several rows under one invented name. **Open question 14.**
12. **Play history is its own Server page**, after Dashboard, not a Dashboard panel. The Dashboard's 30-day Viewing panel is unchanged (Backlog: fold its session history into the new table).

### Not done, and why

- Sorting Songs by plays, a title search and per-row delete in Play history, "who watched this" on a title page, and folding the Dashboard's session history into the new table: Backlog.
- Apple TV does not list Play history (television shows the Dashboard only, as decided).
- The old Home projection is still served, without its featured title: Backlog.

### Functional checks

- **Web** (local server, Chrome): Home hero for an episode in progress ("Breaking Bad · S1 E2", Resume), then the next episode after it finished ("S1 E3", Play), then an audiobook in progress (cover beside the words, "Andy Weir", Resume). Play history empty, then three rows after playing an episode, a song and part of an audiobook, with device "Chrome on Mac", platform "Web browser" and "Stopped at 29%". Retention set to 365 days, saved, read back from the settings document, cleared to Forever. Songs table: Plays 1 on the song played. Artist page: "Your most played" above Songs. Language set to English (United Kingdom): "Customise Home", "Favourites"; set back.
- **iPhone simulator**: Home hero; Settings › Server › Play history with its three filters and the plays.
- **Apple TV simulator**: Home hero (video and listening). Signed the TV out, linked it again with a code approved by the owner on the web: the new session's role is `owner` with no viewer-only grant, Settings shows Server › Dashboard, and the Dashboard opens (Watching now, alerts, health, background tasks). **This also closes "Apple TV Dashboard not checked on a device" from the end of the rework.** Top Shelf on the tvOS home screen: Continue Watching and Continue Listening, no "Featured".

### Builds and suites

- Web: `npm run check` and `npm run build` pass. Apple: Release builds for the iPhone and Apple TV simulators succeed from the final commit and are the builds now installed on both simulators.
- client-core 1413 of 1413, web 394 of 394, Apple 217 of 217. Two per-client hero tests (`web/test/home-hero.test.ts`, `apple/test/home-hero.test.ts`) tested the old meta line and the Watchlist button by reading each client's source; they are replaced by one test of `homeHero` in client-core, where that logic now lives.
- Go on the runner, every package at the default tier, then `httpapi`, `playback`, `playbackv1` and `catalog` again with the media tests (`--tier all`, 90-minute limit): every package passes (httpapi 2,069 s and catalog 759 s with the media tests, as before).
- What the first Go run flagged, all from these changes, all fixed and rerun:
  - *identity*: two quick connect tests asserted the old rule (a linked TV is a member and manages nothing). They now assert the new one: the TV has the approver's account and role, an owner's TV reads what the owner reads, a member's TV does what the member's session does, and a viewer-only session still links a viewer-only TV.
  - *catalog, httpapi*: the most played list is seeded through the counts it now reads.
  - *httpapi*: the new route needed a lane and a restriction classification (administrator only). The read-path golden was re-recorded: I diffed old against new first — the four `featured` row cases are gone and the two Home documents changed (no Featured row, `hero` added); no browse, search or detail case moved.
- Migration 0007 was checked on the local database (version 7 recorded, counts seeded from four completed plays) and by the persistence suite (fresh install, apply-after-higher-numbers, schema allow-list).

### Deploy and state left behind

- Demo: `cc97fc94`, readiness 200 two seconds after the switch; `ebe827ef` is the rollback and the state was copied aside first (`.bak-pre-cc97fc94`). The demo's database is now schema 7; its play history starts with this deploy.
- The branch `frontend-rework` is still not merged in any of the three repos; the tag `pre-frontend-rework-2026-10-02` has not moved.
- Local test state: the dev database is at schema 7 (a copy from before the migration is `~/PorticoDev/server-pre-0007.sqlite`); three plays were made on the dev account (an episode, a song, part of an audiobook); the Apple TV simulator was signed out and linked again, and is now an owner session; both simulators run the final Release builds. The local server, Metro and the Apple TV remote driver are stopped.

### Open questions for Justin (added)

12. **Pause watch history and the owner's record:** should a viewer who pauses their history also be left out of the owner's Play history? Built: no, the owner's record is complete, and the setting says so.
13. **"Popular" on an artist page** is the whole server's plays only when "share activity across profiles" is on; otherwise it is "Your most played". Should it always be server-wide?
14. **Top Shelf's second section** is the viewer's next Home row. Good, or should it always be Recently added?

## Review of the rework, and the fixes that followed (2 Oct, later)

The handed-off work was reviewed against the bar "would I ship this tomorrow": the log read in full, every main screen used on web (desktop and phone width), the iPhone app and the Apple TV app, plus a read of the security-sensitive server changes and the main new web files. The structure held (rules in client-core, one Settings screen, native server pages on iPhone, the Apple TV episode browser). What did not was a list of specific defects, fixed here.

Commits: portico-server `db5dcaf` and the one after it; portico-react-native `a13980b` and the one after it; portico-internal this one.

### Justin's decisions (2 Oct, after the review)

1. **A row is posters or squares, never both.** A standing rule, as Plex does it. Recently aired keeps the show's poster on each newly available episode (unchanged).
2. **Recently added is per library** ("Recently added in TV Shows"), for whichever libraries the server has. Library types are never combined in one row.
3. **Do the server work so show libraries filter by Genre and Year now.** And: **genres on a title page are links** to that library's grid filtered to the genre.

### Built

| Area | What | Main files |
|---|---|---|
| Show libraries filter by genre and year | A show's genres are terms on the show (as a movie's are). In a show library the **show** holds the facets, not its episodes: counts read "Drama 16" beside "24 shows". A show's values are its own genres and year plus what any of its episodes says about the file or title. The genre filter matches an episode through its show | `compactcatalog/derive_facet_counts.go` (version 3), `catalog/browse_contract.go`, `catalog/browse_facets.go`, `metadata/screen_publish.go`, migration `0008_show_facets.sql` |
| Home | The merged "Recently added" row is gone. Each library has its own row, on by default, straight after the Continue rows, in the owner's library order | `catalog/home_rows.go`, `home_recent_source.go` |
| Library Discover | Continue Watching in a library is Home's, for that library: it includes the next episode of a show being watched (it listed only started titles, so TV Shows had none while Home did) | `catalog/discovery.go` |
| Genre links | Web: links in the hero facts line and in Details. iPhone: tappable in the facts line and as buttons in Details. Apple TV: buttons in Details (the facts line cannot take focus). All open the title's library on its grid with the genre applied as an ordinary filter | client-core `genreFilter`; `web/screens/detail/GenreLinks.tsx`; Apple `title/TitleHero.tsx`, `title/TitleDetails.tsx`, `screens/Library.tsx` (route param `genre`) |
| Web filter controls | The web now draws the shared rule the Apple apps already used: Year by decade with counts (newest first), ranges as presets, Any / Yes / No as choices. No number boxes. A promoted chip names its field and lights while it filters; what is selected is said once, on the applied line | `web/screens/library/BrowseFilters.tsx`; client-core `orderFilterValues` |
| Songs | A song plays on through the list in the order shown (web and Apple). Lengths are m:ss. No watched check on a song | `web/screens/library/LibraryViews.tsx`; Apple `screens/Library.tsx` |
| Track names | One rule for what a track is called: the language, then the track's own title only when it adds something ("English · SDH"). The web Audio pill no longer reads "Français · French"; the Subtitles pill and menu name the language instead of "Subtitles" | client-core `trackName` (`presentation/language.ts`), `audioTrackLabel`; `web/player/Player.tsx`; Apple `player/PlayerOptions.tsx` |
| Web player | Subtitles sit clear of the bottom edge and rise above the controls while those are shown (WebKit/Blink) | `web/player/Player.module.css` |
| Title pages (web) | The hero ends a section's distance above what follows (it was 8 px). Entity pages keep a section's distance between sections. A book in one file has no "Book parts" list and its length is in the hero. An album's Details section appears only when it adds something the hero does not say | `ui/TitleHero.module.css`, `screens/library/Entity.tsx` |
| Library header (web) | The count is said once (in the toolbar). Grid/List and card size show only where they change something. Unmatched has its own empty state. Card titles clamp to two lines | `screens/library/Library.tsx`, `ui/Card.module.css` |
| Search | The Top result says what the title is in the singular ("Show · 2008") | client-core `topResultKindLabel` |
| Settings | A local account no longer sees "Manage your Portico Account" on the web. The Server page is **Streaming** ("Playback" is the viewer's page in the same list). The page list fits a 900 px window | `settings/AccountPanels.tsx`, `i18n/.../settings.ts`, `Settings.module.css` |
| Server pages | Health tiles a server cannot measure are left out on the web as on iPhone (one rule). Retention rows are authored (no "trickplay", no alphabetical key order). Addresses are listed once each, without IPv6 link-local ones. A certificate that was never requested says so | client-core `measuredLoadTiles`, `retentionRows`, `listedAddresses`, `certificates.ts` |
| iPhone | The letter rail narrows the grid, not the page header, so Search, the bell and the account button no longer move between tabs. Notifications is not a Settings row on phones (the bell is on every tab) | Apple `screens/Library.tsx`, `screens/Settings.tsx` |
| Apple TV and iPhone | A person's initials are the portrait's placeholder, so a slow or missing portrait never leaves an empty circle | Apple `ui/Artwork.tsx`, `ui/Card.tsx` |

### Server, API and schema changes

- Migration `0008_show_facets.sql` (schema version 8): three triggers on `catalog_episodes` that queue the show's facets when an episode joins, leaves or moves; and a one-time carry-over of already published show genres into `catalog_terms` / `catalog_term_sources`.
- Facet derivation version 3: rebuilt on first start (the backfill visits episodes to clear their old counts and shows to count them). On the dev database it finished within seconds.
- Home: row `recent_all` removed; `recent_<library>` rows are on by default (priority 30, in library order).
- No route added or removed.

### Decisions where the instructions were silent

- **Counts in a show library are shows**, for every facet (Genre, Year, Network, Studio, Resolution…), including on the Recently aired tab. Counting episodes read as a bug beside "24 shows".
- **Recently aired is unchanged**: one poster card per episode, as decided. A show with several new episodes shows several cards. Collapsing them into one card per show ("4 new episodes") would need a server pivot of its own; not done, offered below.
- **A song's queue is the songs already loaded after it** (up to 200), in the table's order. Nothing is fetched to build it.
- **On Apple TV genres are buttons in Details**, not in the facts line.
- **The Portico Account row stays in the settings structure** (the iPhone uses it to link a local account); only the web panel is silent for a purely local account.

### Not done, and why

- **Dashboard "Plays" and Play history count different things** (sessions against completed plays). Folding the Dashboard panel into the play history table is in the Backlog already.
- **Schedule shows the next run, not the last run.**
- **Apple TV: a deep link lands focus on the rail**, not on Play.
- **The web player's More menu and Playback info** could not be opened under automation; unverified by me.

### Functional checks

- Web (local server, 1512×900): TV Shows › Genre lists eight genres with show counts; choosing Drama gives 16 shows. Home rows read "Continue Watching | Continue Listening | Recently added in Anime | … in Movies | … in Audiobooks | … in Music | … in TV Shows | Watchlist | Favorites | Recommended for you | …". A genre link on a movie opens Movies with "Genre: Drama" applied (25 movies). Year lists decades with counts. Songs: 1:00 lengths, no check, and the mini player has previous and next after starting from the table. Audiobook page without "Book parts". Player with French subtitles: the line sits above the controls; pills read "English · AAC Stereo" and "French". Settings: Streaming in the list, authored retention rows, five addresses in place of eighteen, "No certificate has been requested", no Portico Account notice. TV Shows Discover now has Continue Watching.
- iPhone simulator (Release build): Home; a movie's genre in the facts line opens the filtered grid; the header buttons stay put on a tab with the letter rail.
- Apple TV simulator (Release build): a movie page opens with focus on Resume and every cast circle shows initials at once.
- iPhone: Details shows the genres as buttons; the Files list reads "French · AAC · Stereo" (no repeated "Français").
- Type-check and strings lint clean on web and Apple.

### Suites (run once, after the fixes)

- client-core 1413 of 1413, web 394 of 394, Apple 217 of 217.
- Go on the runner, default tier: 71 packages passed on the first run and two tests failed; both were from these changes and both are fixed and rerun (catalog, httpapi and compactcatalog pass):
  - `TestVisibilityClassBuildsRestrictedItemsAndContainersOffRequest` caught a real gap: with shows holding the facets, a restricted viewer's counts were being decided by the show's own rating keys, which cannot say whether any episode is visible to them. A restricted viewer's facets in a show library are now counted live, a show counting when one of its episodes is visible (cached per catalogue revision, as before); the decade count reads the visibility class's show totals. Unrestricted viewers still read the maintained counts.
  - The read-path golden was re-recorded after diffing old against new: the four `recent_all` cases are gone, the two Home documents list the per-library rows, the recent rows' priority is 30, and the ten TV category cases count shows (6 episodes became 2 shows). No status, membership or ordering change anywhere else.
- The media tier was not rerun: nothing here touches playback or media handling.

### Backlog (added)

- A restricted viewer's facets in a show library are a live count over the library's visible episodes (cached per revision). Correct, and fine at hundreds of thousands of episodes; a multi-million-episode library wants maintained per-class show counts for every facet, as decade already has.
- Recently aired could collapse a show's episodes of one air date into one card ("4 new episodes"); it needs a server pivot of its own.

### Deploy

- Demo: `5ba9fb9c`, readiness 200; `cc97fc94` is the rollback and the state was copied aside first (`.bak-pre-5ba9fb9c`). The demo's database is now schema 8.
- Both simulators run Release builds of portico-react-native `a13980b` (the later shared-package commit changes only the Files list wording).
- The branch `frontend-rework` is still not merged; the tag `pre-frontend-rework-2026-10-02` has not moved.

### Recently aired: one card per show (Justin, 2 Oct, after the fixes)

- **Built.** The tab reads the **shows** pivot under a new sort, `latestAired` (a show's most recently aired episode, newest first); each show entry on such a page carries `latestEpisode` {airDate, seasonNumber, episodeNumber, title, count}. One caption rule in client-core (`cardCaption`): the episode when one aired that day ("S1 E4 · Sep 6"), the count when several did ("4 episodes · Sep 17"). The card opens the show. Web, iPhone and Apple TV draw it from the shared tab and caption. Files: `catalog/browse_contract.go`, `browse_engine.go` (`nameLatestEpisodes`), `content_types.go`; client-core `library-tabs.ts`, `card-caption.ts`, `library-content.ts`.
- **API.** New sort `latestAired` on the shows pivot (also offered in the Shows tab's Sort menu as "Latest episode"); new entry field `latestEpisode`. No schema change. The episode-level `aired` sort remains for API clients.
- **Decisions.** A show with no dated episode sorts last. The filters on the tab are the Shows tab's (they filter shows). The count is of episodes that aired on the newest date, not of unwatched episodes. A show numbered across seasons still reads "S1 E4" here (the absolute number is not computed for this caption; Backlog).
- **Cost.** Like `aired`, the order is an expression: a page is ordered by a pass over the library's episodes; the caption reads the episodes of the page's shows only. The Backlog item for a counted ordering now covers `latestAired`.
- **Checks.** Web: TV Shows › Recently aired lists 24 shows, one card each ("Monster · 4 episodes · Sep 17", "Lanterns · S1 E4 · Sep 6"). client-core 1413, web 394, Apple 217 pass; the read-path golden is unchanged; catalog browse tests pass.
- **Deploy.** Demo: `5e3b17d9`, readiness 200 (`5ba9fb9c` is the rollback). Go `catalog` and `httpapi` pass on the runner. Both simulators run Release builds of portico-react-native `a5ea115`.

## Justin's answers to the numbered questions, built the same day (2 Oct, evening)

Justin read the numbered questions with my recommendations and changed four; the rest stand as recommended (download icon kept, iPhone Settings shortcuts kept, the Popular rule, the Top Shelf rule, the credit cap left for the backend audit).

### Justin's decisions

- **Watchlist is renamed My List** (his suggestion, to stop the name fighting non-video libraries). Books carry it too.
- **Subtitles:** an Off choice must exist and the default is Forced. This was already what ships (`playback.subtitleMode`: Off · Forced only · Always, default Forced; Off selects nothing). No change.
- **One metadata provider per library**, as Plex and Jellyfin have it; never mixed. TMDB by default, TheTVDB as the alternative in the library's settings.
- **The owner always sees what is and was watched.** A viewer cannot hide plays from the owner. Retention is the owner's choice and the default is permanent, for history and statistics.
- **Question 1 (NFO versus online credits):** my recommendation stood: the NFO owns what it lists, the provider fills in the rest.

### Built

- **My List.** Every user-facing "Watchlist" is "My List" (title pages, card menus, the selection bar, filters, Saved's first tab, Home's row, empty states) on web, iPhone and Apple TV. The stored flag, API field and row id stay `watchlisted`/`watchlist`; only words changed. A book page has the My List button (web and Apple) and a book is listed under Saved › My List and in Home's row. Files: `i18n` catalogue, `client-core/personal-labels.ts`, `container-state.ts`, server `container_personal.go` (`containerSaves`), `home_rows.go`, `saved.go`, web `library/Entity.tsx`, Apple `screens/Entity.tsx`.
- **One provider per library.** A new library's policy is `["tmdb"]` (`["anilist"]` for anime); the policy `PUT`, the library-settings document and the bridge between them all refuse more than one. Migration 0009 keeps the first provider of a stored chain and turns the other providers' own switches off. The library's Metadata tab already had one select per library; its label is now "Provider", and the iPhone's library sheet gained the same choice (it had only online/local). The server's description of the online source no longer says "TMDB and TheTVDB". Files: `metadata/screen_review.go`, `administration_policy.go`, `library_agent.go`, `administration/library_settings.go`, migration `0009_one_provider_play_delivery.sql`, Apple `server/LibrariesPanel.tsx`.
- **NFO cast with online crew.** When a local NFO owns the credits, the provider's credits are published for the departments the NFO does not list. A film's credits are rows per provider, so they sit side by side; a show's are one field, so the provider's share is the new `creditsOnline` field, read beside `credits` by the show's people, the show page and the recommendation facets. A changed NFO re-splits from the accepted provider record. Files: `metadata/screen_publish.go` (`screenCreditsBesideLocal`), `show_people.go`, `catalog/show_workspace.go`, `compactcatalog/derive_rec_facets.go`.
- **Viewing statistics from the play history.** The Dashboard's Viewing panel (web, iPhone, Apple TV) reads `GET /v1/admin/play-history/summary?period=24h|7d|30d|all`: plays, people, hours watched, share converted, most played, most active. It has an "All time" period and agrees with the Play history page, because both read `play_history`. Each play now records `converted`. Files: `catalog/play_history.go` (`PlaySummary`), `httpapi/play_history.go`, `client-core/play-history.ts`, web `server/OverviewCharts.tsx`, Apple `server/Dashboard.tsx`.

### Server, API and schema changes

- Migration **0009**, schema **9**: the `screen_policy_create` trigger writes one provider; stored chains are cut to their first provider; `play_history.converted` added.
- New route `GET /v1/admin/play-history/summary` (administrator only; in the route lanes, the restriction matrix and OpenAPI).
- `PUT /v1/libraries/{id}/metadata/screen` and the library settings document answer 400 for more than one provider.
- Removed: the TMDB cast fallback for TheTVDB shows (`screen_credits_fallback.go`).
- `catalog.LibraryForShow`, `LibraryForSeason` and the collection lookup return not-found for a malformed id before it reaches SQL (see Suites).

### Decisions where the instructions were silent

- **Anime keeps AniList as its default**, with TMDB and TheTVDB selectable. "TMDB for most libraries" left room for it and it was the first of the old chain. AniList names no episodes, so this is in the Backlog as a question.
- **Fix Match offers only the library's provider.** Matching a title against another provider means changing the library's provider first.
- **The owner's history needed no change for "cannot hide":** a play is written whatever the viewer's own History setting says, and nothing prunes it unless the owner sets days (default 0 = forever). What was not permanent was the Dashboard's statistics (playback sessions, 30 days); that is what moved.
- **Per-department split for credits**, not cast-versus-crew: the NFO owns each department it lists. The NFO parser reads only `<actor>` today, so in practice the NFO owns the cast.
- **Mixed lists keep poster tiles.** Home's My List and Favorites rows are poster rows; a square cover is fitted inside a poster tile, so the row's shape never changes (the row rule). The Saved grids show each card in its own shape, as before.

### Not done, and why

- **NFO cast has no photos.** Local names are not linked to the provider's people, so their circles show initials. The link also has to reach the artwork step; Backlog.
- **TheTVDB libraries have no cast** until the cast is read from TheTVDB's own record (the removed fallback was the only source), and contribute no Trending feed. Backlog.
- **The session-based history route and its CSV export still exist** for API clients; no Portico client calls them. Backlog.
- **Credit cap** (question 2): left for the backend audit, as agreed.

### Functional checks

- Web (local server, owner): TV Shows › Metadata shows one Provider select (TMDB); the dev database after 0009 reads Movies `["tmdb"]`, TV Shows `["tmdb"]`, Anime `["anilist"]`, with TheTVDB off and TMDB artwork off for the AniList library. Dashboard › Viewing: 8 plays, 1 person, matching the 8 rows of the play history, with Today · 7 days · 30 days · All time. Saved's first tab and Home's row read "My List"; no "Watchlist" on Home. The Martian's page has My List; pressing it reads "In My List" and the book appears under Saved › My List and in Home's row. Barbie after a metadata refresh: credits went from TMDB 128 actors and no crew to NFO 10 actors plus TMDB 18 crew, and the page reads "Directed by Greta Gerwig · Written by Noah Baumbach".
- iPhone and Apple TV (Release builds): Saved › My List lists the show and the book.

### Found on the way and fixed

- **iPhone Saved header under the status bar.** With items in a media tab, the pinned title and tabs had no safe-area padding and the grid added it below them (a blank band). Present since before the rework; it shows only when a list has items. `WindowedGrid` has a `topInset` switch and Saved pads the pinned block.
- **Three lookups passed a malformed id to SQL.** In tests an integer-looking id given to `pid_blob` fails loudly; shows' seasons and episodes, a season's episodes and deleting a collection reached it. They now answer not-found first. The hostile-input test only showed this because the new route shifted which value each route receives (Backlog: try every value on every route).

### Suites

- client-core 1413 of 1413, web 394 of 394, Apple 217 of 217 (two wording assertions updated for My List).
- Go on the runner, default tier: 71 packages passed and two tests failed on the first run; both are fixed and `catalog`, `httpapi` and `metadata` pass on the rerun: the row-title test (My List is a named feature, like Continue Watching) and the hostile-input test above. New test: `TestScreenLocalCastKeepsProviderCrew`. The read-path golden changed only in the four My List titles.

### Deploy and state left behind

- portico-server `8af6d26`, portico-react-native `3e851a5`, both on `frontend-rework` (not merged).
- Demo: `8af6d262`, readiness 200 (`5e3b17d9` is the rollback; the state was copied aside first). The demo's database is now schema 9: its TV library keeps TMDB, its anime library AniList. Titles there with a local NFO (Dune, Barbie) gain their director and writers at their next metadata refresh.
- Both simulators run Release builds of portico-react-native `3e851a5` (the Apple TV build predates only the touch-only Saved header fix).
- The local dev database was restored to its pre-0009 copy so the final migration ran whole; that ended the web, iPhone and Apple TV sessions, and all three are signed in again. `~/PorticoDev/server-pre-0009.sqlite` and `server-0009-first-attempt.sqlite` are copies that can be deleted.

## Follow-up on the consequences (Justin, 2 Oct, late)

### Justin's decisions

- **My List is a destination only, not a Home row.**
- **NFO cast showing initials is fine.** The Backlog item for linking NFO names to provider people stays as an idea, not a defect.
- **TheTVDB cast:** build it if the Backlog's fix is real.
- **Anime default:** open to options; "if TMDB is the better default, even for anime, use it." Jellyfin's built-in provider for shows is TMDB (TheTVDB, AniList and AniDB are plugins) and Plex's default agent is TMDB/TheTVDB-based, so TMDB as the default matches both.

### What I had wrong

- **"TheTVDB libraries have no cast" was wrong.** TheTVDB's own record already carries the cast and the provider already reads it (the dev library's shows came back with 6 to 44 actors each, in billing order). The TMDB fallback removed earlier only covered a show whose TheTVDB record lists no characters. Nothing needed building for the cast.
- **What was actually broken was changing a library's provider.** Titles already matched kept the old provider's identity, which the library then refused to refresh ("provider disabled"), so an existing library never moved to the provider its owner picked. That made the TheTVDB choice, and any change of default, do nothing for existing titles. This is what got built.

### Built

- **Home has no My List row.** The `watchlist` row is gone from Home, Customize Home and `GET /v1/home/rows/{id}`; a stored layout that still names it (or `recent_all`) keeps saving, the id is dropped. Saved › My List is unchanged. Files: `catalog/home_rows.go`, web `app/home.ts`, catalogue.
- **TMDB is the default for anime libraries**, with TheTVDB and AniList selectable. An anime library on TMDB uses TMDB's film and show trend feeds. Migration 0010 moves libraries that were on AniList (only ever the default) to TMDB. Files: `metadata/screen_review.go`, `library_agent.go`, `discovery.go`, migration `0010_tmdb_for_anime_rematch.sql`.
- **Changing a library's provider re-matches the library.** Every title matched with the provider it left goes back to unmatched and is searched for again; what it shows stays until the new match is published, and a different identity supersedes the old provider's details, ratings, tags and credits. A show that moves to TheTVDB retires the other provider's facts on its episodes, which TheTVDB's own episode publication otherwise refuses to touch. Identity locks on re-matched titles are released. Files: `metadata/screen_review.go` (`screenRematchLibraryTx`), `screen_publish.go` (`screenRetireEpisodeFactsTx`, identity-change superseding), `screen_worker.go`, `administration_policy.go`.
- **An automatic TheTVDB match is no longer recorded as the owner's.** Every show the matcher placed with TheTVDB was being marked owner-selected with an identity lock, which is why such shows could never be matched again. Only an owner's own selection locks now. File: `metadata/tvdb_selection.go`.
- **The provider select offers only what the library can use.** AniList is offered to anime libraries alone, and "Local files only" is not listed beside the Metadata source choice that already says it. One rule in `client-core/administration.ts` (`providerChoices`), used by web and iPhone; the server's document lists AniList for anime only.
- **An episode whose show has no current identity waits for it** ("needs parent match") instead of failing as "provider disabled".

### Server, API and schema changes

- Migration **0010**, schema **10**: the policy trigger writes TMDB for every film and show library; AniList libraries move to TMDB; every title matched with a provider its library does not use is sent to be matched again, with its identity lock released.
- `GET /v1/home/rows/watchlist` answers 404; `availableProviders` in a library's settings document depends on the library's kind.

### Functional checks (local server, real providers)

- **Anime, by migration:** before, on AniList: 7 shows matched, 1 needing a choice, 28 episodes matched to a work with no episode titles, 4 waiting. After, on TMDB: all 8 shows and all 32 episodes matched, episodes named ("To You, in 2000 Years: The Fall of Shiganshina (1)"), no AniList facts left.
- **TV Shows to TheTVDB (in the web settings):** 23 of 24 shows matched (TheTVDB does not find "Monster: The Lizzie Borden Story", which keeps what it had); 92 of 96 episodes carry TheTVDB details, the other 4 belong to the unmatched show; no identity locks were created. TheTVDB's records carry the cast for every matched show.
- **Back to TMDB:** all 24 shows and 96 episodes matched with TMDB; no TheTVDB credits, evidence or fields left.
- Web: Home has no My List row and no failing requests; Saved › My List is there; the TV library's Provider select lists TMDB and TheTVDB only.

### Not done, and why

- **A TheTVDB library still has no Trending rows** (TheTVDB publishes no trend feed). Backlog.
- **A title the new provider cannot find** keeps its old details and waits for Fix Match; nothing on its page says why. Backlog.
- **Home's Favorites row** mixes media types the same way My List did. Left as it is; asked.

### Suites, builds and deploy

- client-core 1413 of 1413, web 394 of 394, Apple 217 of 217.
- Go on the runner, default tier: all 73 packages pass in one run. New tests: `TestScreenProviderChangeRematchesTheLibrary`; the Home tests use Favorites where they used the My List row; the read-path golden lost only the four `row:watchlist` cases.
- iPhone (Release build): Anime › Metadata shows Provider: TMDB with TMDB, TheTVDB and AniList to choose from, and the reworded source description.
- portico-server `a13c11d`, portico-react-native `42a2f68`, both on `frontend-rework` (not merged).
- Demo: `a13c11d8`, readiness 200 (`8af6d262` is the rollback; the state was copied aside first). The demo is now schema 10; its anime library moved to TMDB and re-matches in the background. Not checked on the demo beyond readiness.
- The local dev database was restored twice more to re-run migration 0010; web, iPhone and Apple TV are signed in again. `~/PorticoDev/server-pre-0010.sqlite` can be deleted.

## Favorites leaves Home; the Android port is planned (3 Oct)

- **Justin:** "Favourites should only exist as a tab of the Saved menu." Built like My List: the `favorites`
  row is gone from Home, Customize Home and `GET /v1/home/rows/{id}`; a stored layout naming it keeps
  saving. The Home tests use Recently added and Trending rows where they used the saved rows; the
  read-path golden lost its four `row:favorites` cases; the regional-spelling test reads "Favourites"
  from the Saved tab's title. Suites: client-core 1413, web 394, Apple 217; `catalog` and `httpapi`
  pass on the runner. Web: Home shows only Continue Watching, Continue Listening and the per-library
  Recently added rows for this viewer; Saved › Favorites is unchanged.
- portico-server `9cfe7fa`, portico-react-native `bde1110`; demo `9cfe7fa8`, readiness 200
  (`a13c11d8` is the rollback).
- **Android port:** planned, not started. The handoff for the agent who builds it is
  `2026-10-03-android-port-handoff.md` (one React Native app for all four platforms; the native layer
  module by module; five phases with checkpoints; the journeys to compare against the Apple apps).

### Port lanes (3 Oct, later)

- Justin widened the port to **Android mobile, Android TV, Fire TV (Fire OS and Vega OS) and Roku**,
  each aiming for visual parity with the Apple TV app, built by separate agents overnight. Three
  handoffs: `2026-10-03-android-port-handoff.md` (Android, Android TV, Fire OS as a `firetv` flavor),
  `2026-10-03-vega-port-handoff.md` (Vega SDK app in `apps/vega` importing the shared screens),
  `2026-10-03-roku-port-handoff.md` (BrightScript/SceneGraph translation in `apps/roku`, generated
  design system and strings, logic tests mirroring client-core's). Each lane works in its own worktree
  and branch (`android-port`, `vega-port`, `roku-port` off `frontend-rework`), commits only there, and
  leaves its log in portico-internal uncommitted for the lead to merge.
- To give the lanes one name to start from, `apps/portico-react-native/apps/apple` is now
  **`apps/native`** (pods reinstalled for ios and tvos; type-check and the 217 tests pass; a clean iOS
  Release build is the check). The old `ios/build` cache failed with disk I/O errors after the move and
  was deleted.
