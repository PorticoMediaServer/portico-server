# Freshness clients

Lane: open screens that stay current, quietly. Two worktrees, both committed on `audit-implementation/freshness-clients`:

- `server-freshness-clients` (portico-server: `packages/client-core`, `packages/i18n`, `web/`): 17 commits over `e292def`.
- `native-freshness-clients` (portico-react-native: `apps/native`, `apps/vega`, copied `packages/`): 5 commits over `4f60939`. `scripts/shared-packages.mjs --check` against the server worktree reports identical.

**How far it was checked.** `npx tsc --noEmit` is clean in `web/`, `apps/native` (app and `test/tsconfig.json`) and `apps/vega` (app and tests). Web `lint:strings` and `lint:design --strict` are clean; Vega `lint:strings` is clean. Native `lint:strings` reports one finding that is not from this lane (`screens/Downloads.tsx:78`, a ternary the lint reads as text; it is there with my one-line change to that file reverted). The test files for what changed were run one at a time and pass: 20 client-core files (4 of them new), 22 web files, 10 native files, 1 Vega file. Two further files fail for reasons that were there before this lane (section 3). **Nothing was run in a browser, on a device or against a server.** Everything about timing, focus, scroll position and animation is unverified at runtime; section 8 says what to check and section 9 says where I expect trouble.

Execution host: `ehler-personal` (mini-1, Ubuntu). No subagents and no helper model were used.

## 1. Outcome

**One shared behaviour, in `client-core`.**

- `freshness-hub.ts`: one hub per signed-in viewer on one server. It listens to the device's one event feed and turns the server's hints into named dependencies (the list of libraries; a library's titles; a library's order; this profile's watched state; this profile's playback positions; a settings document; any other event type). A document on screen says what it was built from and how to read itself again without disturbing what is shown. The hub reads it again only when one of those moved. It never reads on a timer and never because the person came back.
- A document that is covered or closed only remembers that it is out of date, and catches up once when it is shown again. Nothing is read while the app is in the background.
- Hints that arrive together are answered by one read (0.4 s gathering, plus up to a quarter more at random so clients do not all ask at once). A playback position moving on another device is answered at a slower pace: after 3 s, and at most once every 15 s per document. A watched tick is a real change and is not held back.
- A read waits for a pause in the person's input (0.35 s of stillness, at most 8 s) before it changes what is on screen. On television every remote key and every move of focus counts as input.
- A failed read keeps what is on screen and is tried again after 2, 5, 10, 20, 30, then 60 seconds. A refusal the server will repeat (gone, not allowed) is taken as the answer and becomes the screen's own state.
- Nothing is said until the server has been out of reach for 15 seconds (the feed is down, the route is offline, or quiet reads keep failing). Then one small line appears: "Can't reach {server}. Trying again." It goes away with the first thing that succeeds.
- `freshness-pool.ts`: the last known document. A screen's document is kept after the screen closes (24 on the apps, 32 on the web, least recently used first out). Opening the same thing again shows it at once; it is read again only if something it depends on moved while it was away. A first load that failed with nothing to show is tried again by itself on the same waits, and for its first two attempts the screen goes on showing that it is loading rather than an error.
- `freshness-actions.ts`: `retryWhenStale` (an out-of-date answer is met with a re-read and another attempt, silently, twice; only a change that keeps losing, no longer applies, or failed for another reason reaches the person) and `mergeRevalidatedList` (a fresh first page merged into a list that was paged further: nothing below the person disappears, unchanged rows are the same objects).
- `event-feed.ts`: how the stream is opened on every platform (`eventStreamOpener`: resume header, a refusal read as a status, and a watchdog for a stream that says nothing), when the feed runs (`runEventFeed`: while the app is in front or playing; stopped 30 s after neither; resumed from the last event id), and one stream shared between the windows of one browser (`sharedStreamOpener`).
- `home-service.ts`: Home as one document every app shows the same way. A quiet read is merged into what is shown: a row that did not change is the same object, a row that was paged sideways keeps its tail, a new card pushes the row along.
- Every document service has a quiet `revalidate()` that leaves `phase` alone and rejects without changing anything when the read fails: library content, the windowed list, title details, the show page, search, saved lists, personal lists, a Home row's full page. A quiet read that finds nothing changed publishes nothing.

**Conflicts are no longer the person's problem.**

- A next page read after the library moved is accepted (the server re-anchors continuations). The client's own "the library changed, refresh to continue" check is gone for libraries, search, saved lists and shows. A page the server really refuses restarts the list from its first page, keeping what is on screen until it arrives. A cursor that comes back unchanged ends that section instead of asking for a refresh.
- Marking a title watched, adding it to the watchlist or a favourite, when the state moved elsewhere first: the title is read again and the change sent with the current revision. If another device already made the same change, it is simply done.
- Playlist changes (add, remove, rename, share) the same way. A new order chosen against an old order is still shown as a conflict, because that one is real.
- Saving the Home layout the same way.
- The owner's console: a PATCH of changed values (server settings, preferences, feedback triage) or a command on a row (pause, cancel, retry a task; acknowledge an alert) is sent again with the revision the server named. A task's revision moves with its own progress, so this was the most common false conflict. A PUT that replaces a whole value is not re-sent.

**Web.**

- The feed is a stream (`fetch` read as it arrives) wherever the browser gives Web Locks (any secure page, and localhost). Every tab signed in to the same server as the same viewer shares one stream; a lock decides which tab holds it and a broadcast channel carries each event to the rest. Over plain HTTP from another machine each visible tab uses the long poll the server kept. A hidden tab that is not playing stops its feed after 30 s and makes no requests.
- Home, library tabs, the library grid, title pages, show pages (and the episode list on them), entity pages (album, artist, season, collection), search, saved lists, playlist and collection pages, a Home row's full page and a person's titles are read again in place. Home, library tabs, the grid, titles and shows are also kept after Back.
- New cards in a row already on screen fade in (320 ms, the existing `--motion-slow` and `--ease-out`; nothing new). A first paint, a new page and a new sort animate nothing.
- The rail's libraries follow `libraries.changed`; the notification badge and open inbox follow `notification.changed` (the one-minute timer is gone); preferences follow `settings.changed`; the owner's task list follows scan and task events as well as its existing 20 s read.
- A server that stops answering no longer covers the page with "Reconnecting" or starts the page afresh when it returns. The page stays as it is and one small line appears after 15 s. An address that answers as a different server still covers the page.

**Phones, tablets and televisions (`apps/native`), and Vega (which shares those screens).**

- The feed is the native stream reader on `/v1/events` (it was a 10-second long poll), with the long poll as the fallback. The Android and Apple native readers and Vega's now allow `/v1/events` and no longer allow the retired `/v1/notifications/events`. The television receiver shares the feed instead of opening a stream on a route the server removed (this was a type error on the base after the server work landed).
- The same screens as the web are read again in place. A screen covered by another is not read until the person comes back to it.
- Lists that are built on a document (a show's episodes, an album's or artist's tracks) used to be thrown away and rebuilt whenever their document was read again. They now take the new first page behind what is shown and swap to it whole, at the same place, with the same row keys. This is what keeps the focused row focused on television.
- Screens are no longer refreshed because five minutes have passed. Pull to refresh stays on phones and tablets.
- The same small "Can't reach" line, drawn over the shell. It takes no room, no touches and no focus.

## 2. Decisions I made

1. **Dependencies are named, and counters are compared as strings.** A hint whose counter equals the last one seen is a replay and moves nothing. The first position hint for a library does not count as a watched-state change.
2. **Staleness is a flag on a living document, not a stored stamp.** A kept document goes on listening while its screen is closed. `stream.resync`, or a different feed, makes every document out of date.
3. **Kept documents live in memory only**, per signed-in viewer, and are dropped when the viewer or server changes. Nothing is written to disk. A cold start still loads from the server.
4. **Progress is paced** (3 s, then at most every 15 s per document). Home and lists depend on positions for resume bars; reading them at the rate another device reports its position would be wasteful.
5. **15 seconds before "Can't reach" is shown.** A success from any source restarts every other source's clock.
6. **Any success clears the line; the page is never restarted.** The old web rule ("an untouched page is started afresh after an outage") is removed with its test. Documents are brought up to date in place instead.
7. **An address answering as another server still covers the page** on web and apps. Nothing on the page should be acted on against a server that may not be the person's.
8. **Web tabs share one stream only where Web Locks are real.** Over plain HTTP from another machine the lock is a per-tab stand-in, so tabs there keep the long poll (which tabs can run side by side without taking the stream from each other).
9. **Shared-stream events cross tabs on a BroadcastChannel.** That channel is same-origin only, so it is the same trust boundary as the page. Events remain hints: the data is always read from the server.
10. **A hidden tab or backgrounded app stops its feed after 30 s** unless something is playing. On return it resumes from the last event id, so nothing in between is lost.
11. **Pool keys.** Library content is kept per destination (library, view, entity), so a new sort keeps the list on screen while the new order loads. The grid is kept per library and pivot. A show is kept per address and returns to the season the person left.
12. **`refresh-required` still exists** as a phase, but is only reached when the first page itself is refused as out of date. The notices for it were left in place for that case.
13. **Console PUTs are not re-sent** on a conflict (schedules). A full replace could overwrite another person's change.
14. **Native cards do not fade in.** The web fade is one CSS class. On the apps it needs a wrapper around every card in a virtualised row, which I could not check against television focus or Vega's animation shim. New cards appear in place.
15. **Native lists are not pooled individually; their screens stay mounted.** Home, library content, titles and shows are pooled on the apps as on the web. The native library grid belongs to its mounted screen and is read again quietly there.
16. **Time-based refresh is off by default on the apps.** It stays on (`auto: true`) for Downloads and the person page, which the feed says nothing about.
17. **The owner's task list keeps its 20 s read** and gains events. Task progress counts do not all produce events.
18. **Deleted as replaced:** web `home-recovery.ts` and two tests, the web's `useAppendedSections`/`mergeSectionPages`, native `home-requests.ts` and its test.

## 3. Bugs

The lane brief names documents 16, 19, 21 and 22. Those hold about 80 entries, most of them owned and already reported by the web-app, native-app, networking and native-sync lanes. I confirmed and acted on the ones that concern freshness; I did not re-examine the rest.

| ID | Status |
|---|---|
| B-16.7 | Partly fixed. Tabs share one stream on a secure page; a hidden tab stops its feed. Over plain HTTP from another machine each *visible* tab still holds a long poll. |
| B-16.11 | Fixed further. web-app kept the page mounted under a cover; the page is now never covered or restarted for a server that is merely away. |
| B-16.19 | Already fixed (web-app). Left as is. |
| B-16.20 | Already fixed (the windowed list honours the server's wait); the quiet read uses the same waits. |
| B-19.2 | Already fixed at the scope level (native-app). Kept documents are now keyed on that same stable scope. |
| B-19.14 | Not fixed. Mounted screens are still never released. A covered screen now reads nothing, which removes its network cost but not its memory. |
| B-21.5 | Not changed. Saved lists still append; a quiet read replaces the page it is on, in place. |
| B-21.13 | Already fixed on this base (the native Saved hook compares the service as well as the route). |
| B-22.13 | Already fixed (`sse.ts`); the new opener reads through it. |
| B-22.18 | Already fixed; the feed's stream goes through the same verified route reader. |
| B-08.17 (server report) | Client half done: one feed per device on the apps, shared between tabs on the web, and the receiver shares it. |
| A-22.43 (server report) | Done: the apps open an authenticated stream instead of polling every ten seconds. |
| A-16.7, A-16.14 | Addressed by the hub, the pool and the shared stream. |
| All other B-16.x, B-19.x, B-21.x, B-22.x | Not examined in this lane. See `web-app.md`, `native-app.md`, `networking.md`, `native-sync.md`. |

The native string lint finding above (`screens/Downloads.tsx:78`) is for the downloads lane or base-health. Two test files fail on this base for reasons outside this lane, before and after my changes: web `test/maintenance-windows.test.ts` (`backgroundPriorityOrDefault` does not exist in the source) and native `test/content-forward.test.ts` ("an unknown entry kind drops only that entry from Home"). For base-health.

## 4. API changes

No server route changed. Client-core surface:

- New modules, all exported from `index.ts`: `freshness-hub.ts` (`FreshnessHub`, `dependsOn`, `titlesWithPersonalState`, `FreshDocument`), `freshness-pool.ts` (`DocumentPool`, `serviceDocument`, `QuietService`), `freshness-actions.ts` (`retryWhenStale`, `isStaleAnswer`, `ALREADY_DONE`, `mergeRevalidatedList`, `arrivals`), `event-feed.ts` (`eventStreamOpener`, `runEventFeed`, `sharedStreamOpener`), `home-service.ts` (`HomeService`, `mergeHomeDocument`).
- `EventsClient.onAny(listener)`: every delivered event. Stream control frames (`stream.*`) are no longer passed to type listeners (none listened for them).
- `revalidate(context?)` on `LibraryContentService`, `DetailService`, `ShowWorkspaceService`, `SearchService`, `SavedService`, `PersonalSavedService`, `HomeRowPager`; `revalidate({fetchPage?, hold?})` on the windowed collection. `retry()` added to `DetailService` and `HomeRowPager`; `retryRead()` on `PersonalSavedService`.
- `LibraryContentSnapshot` gains `appended` (every page read so far, one list per section) and `head` (the fresh first page from the last quiet read). `next(sectionId)` now works on any section of the page, not only the one paged last.
- `ReceiverHost` consumers on the apps now pass the shared `EventsClient` (the server lane changed the type; the native call site is fixed here).
- Behaviour: the continuation fence in content, search, saved and show services now refuses only another viewer's answer or one older than what is shown. `ConsoleClient` re-sends a PATCH or a row command on `console_conflict`.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

None. The timings in section 1 are fixed defaults in `FreshnessHub` (constructor options exist for tests).

## 7. Needed from others

- **Server (freshness-server follow-up).** (a) Task progress does not always produce an event, so the owner's task list keeps a 20 s read; an `operation.updated` for every progress step would let it go. (b) Recommendation, trending and "popular on this server" rows have no counter; they are read again only when a library or the profile moves. If they should move by themselves, they need a hint. (c) Confirm that a stream retired by a newer one from the same installation is ended without `stream.closed`; the client treats that as a lost stream and backs off, which is what makes two browsers on plain HTTP settle. (d) Live TV guide and DVR have no freshness events; those screens keep their own reads.
- **Server.** `personal.changed` and `progress.changed` name a library, not a title. Every open title page in that library is read again when any title in it is marked. A title id on the hint would let a title page ignore the rest.
- **native-app / apple and android lanes.** The two native allow-list edits (`Native/PorticoPlayer.m`, `PorticoEventStream.kt`) were written here and not compiled. Each is a one-string change.
- **base-health.** The two failing tests named in section 3.
- **Documents.** `Project Architecture/08` (or wherever client data flow is described) should describe the hub, the pool and the shared stream. `web/README.md` and `apps/native/src/bridge/README.md` still describe the feed as a long poll. The audit's B-16.11 fix note ("the page stays mounted under the status") is superseded.
- **Catalogue.** One new string: `web.connection.cantReach` ("Can't reach {server}. Trying again."). `web.connection.reconnectingBody` is now shown only when an address answers as a different server; its wording ("isn't answering right now") no longer fits that case and should be reviewed.

## 8. Test notes for other environments

### Two-device script (change on one, watch the other)

Environment: one server with a film library and a TV library; device A is a browser signed in as a profile; device B is a second browser, a phone, or a television signed in as the same profile. Keep B's network inspector or a proxy open to see requests. Do not touch B while watching unless a step says so.

1. **Idle makes no requests.** Leave B on Home for two minutes. Pass: one long-lived `/v1/events` request and nothing else. Fail: repeated `/v1/home` or `/v1/events?waitSeconds=` reads on a secure page.
2. **Watched tick.** On A, mark a film watched. Pass: within about 5 s B's card shows the tick, in place, with no spinner, no skeleton and no scroll movement; one `/v1/home` read. Open the film's library on B first and repeat: the grid card updates and the grid does not move.
3. **Continue Watching.** On A, play an episode for a minute and stop. Pass: B's Continue Watching row gains or moves the card within about 20 s; the rest of Home does not re-render (on the web, cards in other rows do not flash). While A plays, B makes at most one Home read every 15 s.
4. **New title.** Copy a film into the library folder and scan. Pass: on B the film appears in Recently Added with a short fade (web) and pushes the row along; the library grid gains it without jumping; the rail shows the scan dot.
5. **Removed and missing.** Delete a file on disk and scan. Pass: B's card for that title shows as unavailable or goes, in place. Open the title on B first and repeat: the page changes to its missing state without a reload.
6. **Deep in a list.** On B scroll a library to its 2,000th title and stop. On A mark a title near the top watched. Pass: B stays exactly where it was; no placeholders flash. Scroll up: the tick is there.
7. **Television focus.** B is a television. Focus the fifth card of a Home row. On A mark another title in that row watched. Pass: focus stays on the same card. Hold Right to scroll a long row while A makes changes: nothing moves until the remote has been still for a third of a second. Repeat on a show's episode list and a 300-track album, focused deep in the list. Pass: the focused row stays focused and the list does not return to its top. **This is the case to try hardest.**
8. **Back fetches nothing.** On B open Home, a library, a title, then press Back twice. Pass: each screen is there at once, at its old scroll position, with no request (unless step 2 to 5 changed something meanwhile, then one quiet read).
9. **Covered screens.** On a phone open a title over Home. On A change something. Pass: no `/v1/home` read while the title covers Home; one read when you go back, and Home updates in place.
10. **Background.** Put B's app in the background (or hide the tab) for two minutes. Pass: the `/v1/events` request ends after about 30 s and nothing else is requested. Change something on A. Bring B back. Pass: one `/v1/events` request with `Last-Event-ID`, then one quiet read of what is on screen.
11. **Two tabs.** Open B in two tabs on HTTPS. Pass: exactly one `/v1/events` stream between them; both update in step 2. Close the tab that holds the stream: the other opens one with `Last-Event-ID` and carries on. Repeat over plain HTTP from another machine: each visible tab long-polls; neither stalls.
12. **Server away.** Stop the server. Pass: B's page stays exactly as it is; after about 15 s one small line says it can't reach the server; no error states, no blank page. Navigate to a library not yet opened: it shows as loading for a few seconds, then its own error with Try again. Start the server. Pass: the line goes, the library fills in by itself, and the page you were on is brought up to date without reloading. On television check the line takes no focus.
13. **No conflicts.** On A and B open the same title. Add it to the watchlist on A. Within a second, mark it a favourite on B. Pass: both stick, no message. Rename a playlist on A, then add a title to it on B from a stale page: no message. As owner, open Activity on B, start a scan, and press Pause on the running task: it pauses with no "the document changed" message. Reorder a playlist on B after A reordered it: this one should say so.
14. **Paging across a change.** On B open a long saved list and scroll to load three pages. Change something on A. Scroll on. Pass: the next page loads; no "the library changed" notice.
15. **Owner.** With Activity open on B, start a scan on A. Pass: the task appears within a couple of seconds and its count moves.

### Platform notes

- **Android and Apple builds.** `PorticoEventStream.kt` and `Native/PorticoPlayer.m` each allow `/v1/events`. Build, sign in, and confirm in the server log one stream per device and no `waitSeconds` polls. If the native reader refuses the path the app falls back to the 10 s long poll; that is the sign the native change did not land.
- **Reverse proxy.** Behind nginx without `proxy_buffering off` the stream may be held back. The client gives a stream 10 s to produce its first bytes and 70 s between bytes; three failures in a row move that client to the long poll for the rest of the run. Pass: updates still arrive, a little later.
- **Vega.** The stream is an `XMLHttpRequest` read as it grows, ended at 1 MiB or 35 minutes and reopened from the last event. Check step 7 on a Fire TV with the Vega build.
- **Reduced motion (web).** The card fade is suppressed by the existing global rule.

## 9. Not finished

- **Nothing was run.** All of section 8 is untested. The parts I trust least, in order: (1) television focus when a card moves within a virtualised row (Continue Watching reordering after playback: the focused card keeps its key and its focus, but the row may not scroll to follow it); (2) the shared stream between browser tabs, whose hand-over was only tested against a simulated lock and channel; (3) the "kept for a moment" hold used while a screen is still being drawn (5 s) under React's development double-render; (4) Vega's navigation package exporting `NavigationContext` at runtime (its types do).
- **Native cards do not fade in** (decision 14).
- **Saved lists, search, a Home row's page and the person page are not kept after Back** on the web; they reload when reopened and are then kept current. On the apps their screens stay mounted, so this does not show there.
- **A list paged by appending is refreshed from its first page.** A title removed from deep in an appended list keeps its card until that list is reopened. The library grid does not have this limit (it re-reads what is in view).
- **Saved lists that have been paged** refresh only the page they are on.
- **Live TV, the guide, DVR, Downloads, Watch Together and the owner console's other pages** were not wired to the hub. The console's measurements and charts are time series and keep their timers.
- **`refresh-required` notices remain** in five web screens and six native ones for the rare case in decision 12. They were not removed.
- **Plain HTTP from another machine** still costs one long poll per visible tab (B-16.7, partly fixed).
- **Not persisted:** the last known documents do not survive an app restart.
- **Documents** named in section 7 are not updated.
