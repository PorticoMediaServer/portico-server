# Lane report: web-app — shared client behaviour and the non-player web app

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/server-web-app`, branch `audit-implementation/web-app`, 99 commits of its own plus two merges of `audit-implementation/main` (`853e919`, `b8635f0`); the branch is level with the base, nothing in the base is missing from it. Nothing pushed, nothing deployed. The three follow-up jobs (merge, native strings, parser re-read) are in section 10.

How it was checked: `npx tsc --noEmit` in `web/` is clean; `npm run lint:strings` and `lint:design --strict` are clean; the preference-consumer check passes (56 keys). The test files for the changed areas pass: 110 web test files (every non-player one) and 177 client-core test files. **Nothing was run in a browser, on a device or against a server.** Everything visual and everything about focus, screen readers and multi-tab behaviour is unverified at runtime (section 8).

How the work was done: three helper agents ran for the first part of the session (tolerant parsing, moving screens onto the catalogue) and were stopped by a host restart; their commits are on the branch, and I reviewed and finished the uncommitted remainder myself. Later the cheap helper model made one small change, which I read line by line and partly rejected (see Decisions).

## 1. Outcome

**Tolerant client parsing.** A newer server no longer breaks an older app over one value. `packages/client-core/src/tolerant.ts` holds the rules and helpers, and the response parsers follow them: a field the app does not know is ignored; an unknown enumerated value is dropped, read as a stated neutral value, or takes only its own row; one unreadable row is left out and the rest kept; repeated ids keep the first row. Sizes, depths and lengths are unchanged, and identity stays exact (server, library, entity and scope ids, viewer fences, revisions, cursors, operation ids, roles and limits). What was skipped is counted per document and part (`skippedParts()`, `onSkipped()`), with nothing from the response kept. Covered: Home, library pages, browse and its capabilities, title pages, show pages, people, search and search history, related rows, recommendations, container state, the guide (channels, programmes, sources, directory, summaries), DVR pages, listening journeys, Saved, collections, playlists, history, notifications, inbox, account sessions and notifications, downloads, prepared media, preferences, profiles, and the owner console documents (activity, administration, access, libraries and inventory, storage, remote access and sources, telemetry, diagnostics, attention, bulk jobs, match review, release details, the metadata editor, Library Channels, recording policy).

**Remembered choices.** A library reopens on the tab this profile last used there, with that tab's last sort and filters. The behaviour is in `client-core/src/library-memory.ts` (storage is an adapter, so native can supply its own), stored per profile and server on the device, bounded, and wired on web. A link that names a tab, sort or filter is followed as it is and does not change what is remembered.

**Accessibility.** Long lists now tell assistive technology their full length and each card's position, and focus goes to the list instead of the page body when a focused card scrolls out. The title table numbers its rows in the whole table. A select's or text area's help and error are read with the control; every segmented option has a name. (Main landmark, skip link, page titles and card state were already on the base branch.)

**Language catalogue.** Every web screen takes its words from the profile's catalogue and its dates, times, numbers, sizes and language names from the profile's formats, including the metadata editor and the server console. One catalogue instance is shared by the signed-in app and set before screens draw. 136 strings of replaced screens were removed.

**Phone width.** The top-bar avatar opens the profile menu: Settings, Switch profile, Switch server, Account, Send feedback, Help, Sign out.

**See all.** A Home row goes where the server's row says (`seeAll`), otherwise by the server's `recent` kind to the library newest first, otherwise to the row's own page. Nothing is read out of a row's id.

**"This device".** Settings the server keeps per kind of device now say "All web browsers", "All phones and tablets" or "All TVs". Only settings the one device keeps say "This device".

**Plain HTTP note.** One quiet line on the signed-out screens when the server says the page came over plain HTTP from outside the home networks, with a link. Never blocks.

**Other fixes.** Back to a library returns to the remembered place; a library's sort and filter vocabulary is rechecked each time it is opened; every JSON request has a deadline; a list page the server refused waits as long as it asked; a connection wobble no longer discards a page someone had edited; a tab whose sign-in ended no longer erases the profile another tab chose; an unsigned answer naming another server no longer ends a valid sign-in; two tabs on plain HTTP can renew a sign-in together; renewal and event reconnects honour and spread the server's wait; a device clock up to two minutes off still accepts a server proof; a proxy's own JSON error counts as a lost route; a typed address tries the server's port and both schemes; "Remember search history" works and Clear clears the server's copy; search words are no longer written into the address; a title's page offers the same actions as its card; Refresh metadata is offered only where the server can do it; profile Limits and Delete are offered only to someone who can use them; a show's cast has See all; an episode card has a ⋯ menu; a season card's address is corrected to its show; every Back button stays inside Portico; Saved's cards are links; bulk Add to collection is open to everyone; changing someone's role asks first; developer screens are left out of production builds.

## 2. Decisions I made

1. **What stays strict.** Anything a later request sends back, anything that decides what someone may do, and all bounds. A profile's limits, a member's role and a write receipt's per-target rows are never read more permissively. The helper model proposed dropping unreadable rows from a bulk edit receipt; I rejected that (an outcome would become unknown) and kept only "ignore an unused field on a success".
2. **Neutral readings.** Unknown artwork shape reads as poster; unknown guide programme lineage as "known only by its time slot"; unknown channel unavailability reason as plainly unavailable; unknown match status as unresolved; unknown guide state as ready when there are channels, otherwise no guide data. A guide channel is available only when the server says so outright and gives no reason against, so it is never read as more available than stated.
3. **Lists bounded by "how many values this app knows"** (search groups, sorts, directions) are now bounded by a fixed 64, so a newer server's extra value is dropped instead of refusing the answer.
4. **Rows dropped rather than neutralised:** a recording in a state the app does not know (it could not offer the right actions), a Library Channel with a setting the app does not know (it is edited and saved whole), an entry of an unknown kind.
5. **Remembered choices are per tab.** Choosing a tab restores that tab's own sort and filters; filters no longer carry over from one tab to another. Arriving by a link does not change memory. A remembered filter the library no longer offers is dropped instead of failing the page.
6. **Multi-tab profile switch** (the audit's open question): the old tab still goes to the sign-in screen when the server ends its sign-in, but it no longer erases the browser's saved sign-in, so a reload lands in the newly chosen profile. I did not make the old tab follow silently into another person's profile.
7. **Reconnecting:** the page stays mounted under the status. When the server returns, a page someone had typed into or changed is given back as it was; an untouched page is started afresh so it reloads its data (today's behaviour).
8. **Request deadlines:** 30 s for reads, 60 s for changes, 120 s for owner-console changes, a long poll's own wait plus 15 s. A timeout is a retryable failure, never a silent cancel. Nothing is replayed automatically.
9. **Search words leave the address**, as the navigation contract says. A link carrying `?q=` is searched once and the query is then removed. Back still restores the search from the in-tab session.
10. **Search history** is recorded on Enter or on opening a result, never while typing. With the setting off, no recent searches are shown, not even for the current visit.
11. **Refresh metadata** is hidden on shows, seasons, albums, artists and books until the server can refresh them.
12. **Title page Delete** uses the same typed-confirmation dialog as the card. Download appears only when this server and this title can be downloaded.
13. **A profile may open its own limits read-only**; Delete and other profiles' limits need the managing profile.
14. **See all fallback:** until the server sends `seeAll` on Home rows, only rows the server marks `kind: "recent"` open the library newest first; any other row opens as its own page. The library's own Discover sections still use the old id rule (an existing tested requirement); it can go when the server sends the field there too.
15. **Unused strings:** I removed only strings of viewer screens that were replaced (`web.live.*`, `web.hostedAccount.*`, old settings, preferences, filter and server-chooser panels) and that no web, shared, native, Vega, Roku, Hosted or server source refers to. About 200 more unreferenced strings were kept on purpose: owner-console and shared (non-`web.`) ones that the console and native lanes may be about to use.
16. **Plain HTTP note** is decided by the server (`plainHttpOutsideHome`), not guessed from the address; any failure to find out means no note.
17. **Address candidates:** home-looking hosts try `http://host:32500` first; public ones try HTTPS first and plain HTTP last (the owner allows plain HTTP from anywhere).
18. **Clock allowance** applies only to the proof a server signs over the client's own nonce (replay is impossible there). Hosted route documents keep their exact expiry.
19. **Role change** now asks for confirmation; what an administrator can actually do was left to the accounts lane.
20. `library-navigation.ts` is an older navigation state service that reads only its own storage; it is not a server document and was left alone. It overlaps in purpose with the new library memory and is unused by web screens.

## 3. Bugs

Document 16
- B-16.1 fixed (list remembers before its pages are released; scroll offset followed as it changes). Needs the runtime check in section 8.
- B-16.2 fixed (compare-and-clear of the saved viewer; see decision 6).
- B-16.3 already fixed.
- B-16.4 not fixed: the mapped-scroll discontinuity needs the windowed lab in a browser.
- B-16.5 partly fixed: the resize observer is no longer rebuilt per scrolled row; inline renderers still defeat per-page memoisation.
- B-16.6 fixed.
- B-16.7 not fixed: the feed is started by the player engine (web-player lane). The long poll now has a deadline.
- B-16.8 fixed.
- B-16.9 already fixed.
- B-16.10 fixed for the parsers listed in Outcome; remainder in section 9.
- B-16.11 fixed.
- B-16.12 partly fixed: cards keep a focus ring without `:has()`. No browser floor is declared and no build target set.
- B-16.13, B-16.15 already fixed. B-16.14 already fixed; its two noted gaps remain.
- B-16.16 fixed (position semantics and focus fallback).
- B-16.17 fixed for help association and option names; pixel type sizes and forced-colours are unchanged.
- B-16.18 not fixed: needs a server CSP decision (nonce or hash for the dialog scroll lock).
- B-16.19 fixed by rechecking on each use; a real revision needs a server field (section 7).
- B-16.20, B-16.21, B-16.22 fixed.
- B-16.23 not fixed: adding ESLint is a new dependency.
- B-16.24 fixed.
- B-16.25 not fixed: CI and Dockerfile belong to the packaging lane.

Document 18
- B-18.1, B-18.2, B-18.11 already fixed.
- B-18.3 fixed.
- B-18.4 not fixed: needs server paging of aggregate pivots.
- B-18.5 not fixed: library removal is the identity lane's.
- B-18.6 fixed by gating; container refresh itself is server work.
- B-18.7 fixed.
- B-18.8 refuted by the second review; nothing done.
- B-18.9 not fixed: delegated administration is the accounts lane's.
- B-18.10 fixed.
- B-18.12 fixed.
- B-18.13 deliberate; unchanged.
- B-18.14 fixed.
- B-18.15 fixed (lists the cast the show's page carries; it is not paged from the server).
- B-18.16, B-18.17, B-18.18 fixed.
- B-18.19 fixed.
- B-18.20, B-18.21 fixed.
- B-18.22 fixed, except the player's post-play card (web-player lane) and the metadata editor's fallback field names (used only when the server sends no label).
- B-18.23 not fixed: needs a disc number on track entries from the server.
- B-18.24, B-18.25, B-18.26 fixed.
- B-18.27 partly fixed: 136 strings removed. Pinned libraries, the provider API-key field, "Forget remembered devices" and the two unreachable registry keys are untouched.
- B-18.28 fixed on the client; server field needed (section 7).
- B-18.29 not fixed (Undo for removing a playlist or collection entry).
- B-18.30 not fixed (episode panel air date on a direct visit; people links).

Document 22 (web and shared parts)
- B-22.1, B-22.2, B-22.4, B-22.9, B-22.11, B-22.12, B-22.13, B-22.23 already fixed.
- B-22.3 fixed: one renewal request id is agreed through the store, whose change is a single database transaction on plain HTTP.
- B-22.5 fixed on web; native has the same code and needs the same rule.
- B-22.6 not fixed: deliberate trade-off of plain HTTP; unchanged.
- B-22.7 fixed in client-core and web; native to adopt.
- B-22.8 fixed.
- B-22.10 fixed.
- B-22.14 fixed for proxies that answer in their own JSON dialect; a proxy answering plain `application/json` is still indistinguishable.
- B-22.16 fixed for nonce-bound proofs.
- B-22.17 fixed on web (`KnownDirectServers.setAddress`); native to call it.
- B-22.25 fixed.
- B-22.15, B-22.18, B-22.19, B-22.20, B-22.21, B-22.22, B-22.24, B-22.26: native, Vega, server or policy; not this lane.

## 4. API changes

None made on the server. The client now reads:
- `GET /v1/networking/connection` (added by the networking lane): `{secure, remote, plainHttpOutsideHome}`. Parser: `client-core/src/connection-note.ts`.
- `GET` and `DELETE /v1/search/history` (existing routes, previously unused): `readSearchHistory`, `clearSearchHistory` in `search.ts`.
- An optional `seeAll` on a Home row, same shape as a content section's. The server does not send it yet.

Client-core surface added: `tolerant.ts`, `library-memory.ts`, `server-address.ts`, `connection-note.ts`; `RequestTimeoutError`, `requestDeadlineMs`; `settingsScopeLabel` and the `deviceClass` settings scope; `canRefreshMetadata`; `KnownDirectServers.setAddress`; `ViewerSelectionCommit.clear(retiring)` and the optional storage method `clearIfFamily`; new optional `DetailViewer` fields `queue`, `group`, `downloads`, `canDelete`.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

- `search.rememberHistory` (existing, boolean, default on): now takes effect on web. Its row already exists in Settings › Privacy.
- No setting added. Remembered library choices have no switch.
- Relabelled only: quality limits, playback method, HDR, card size, backdrops and reduce motion carry the device-kind badge.

## 7. Needed from others

Server
- **Home rows:** send `seeAll` on a row that has a full list elsewhere, JSON `seeAll: {pivot: string, query?: BrowseNode, sort: [{field, direction}]}` (the existing `ContentSeeAll` struct), `omitempty`, on `catalog.HomeRow`. For `recent_<library>` rows: the library's main pivot with `sort: [{field: "added", direction: "desc"}]`. The same on Discover's recently-added section lets the last id-based rule in `web/src/app/library-words.ts` (`seeAllTarget`) go.
- **Browse capabilities:** add a `revision` to `GET /v1/libraries/{id}/browse-capabilities` so clients can key the vocabulary on it instead of rechecking on every visit.
- Disc number on track entries (B-18.23); paging for genre and series aggregates (B-18.4); refresh for shows, albums, artists and books (B-18.6); a CSP allowance for the dialog scroll lock (B-16.18).

Web-player lane
- I made two small edits in `playback-v1/events.ts` (reconnect waits are spread beyond a server floor) and updated its tests; `requestRaw` now has a deadline.
- Still theirs: one event feed per tab on plain HTTP (B-16.7), the post-play card's English.

Native lanes (the shared packages are copied to the native repository)
- Wire `LibraryMemory` with a device storage adapter and call `libraryOpening` / `rememberTab` / `rememberSort` / `rememberFilters` on the library screen.
- Show `settingsScopeLabel(row.scope, platform)` instead of testing `row.scope === 'device'`; device-class rows are now `scope: 'deviceClass'`.
- Use `serverAddressCandidates` and `firstAnswering` in place of `direct-connect.ts`'s scheme guess.
- Apply the restore rule: an unsigned different server id keeps the saved sign-in (web's `restore-policy.ts` is the reference).
- Call `KnownDirectServers.setAddress` when the verified route changes.
- `profileActions` no longer offers Limits for another profile or Delete without `canManage`; `detailActions` offers queue, group and delete only when the new viewer fields are passed.
- None of the 136 removed strings is referenced by native, Vega or Roku source as checked in these worktrees.

Website
- The note links to `https://getportico.tv/docs/remote/enable-remote-access`. That page should say how to turn on a secure connection (own certificate, reverse proxy), or a dedicated page should exist and the constant `SECURE_CONNECTION_HELP_URL` be pointed at it.

Documents to correct
- `Product Contracts` navigation contract and `router.tsx` now agree (search text is not in the address); the audit's doc/code disagreement D-18.5 is closed.
- `06 Clients.md`: "title cards are links" is now true in Saved.

## 8. Test notes for other environments

All of these need a browser against a real server (the demo server is enough except where noted). Pass means the described result; anything else is a failure.

1. **Remembered choices.** Open a library, pick a non-default tab, change sort, add a filter, go Home, click the library in the rail: it reopens on that tab, sort and filter, and the address shows them. Switch profile: the other profile gets its own. Follow a "See all" link, then open the library from the rail: memory is unchanged by the link.
2. **Back to a list.** Scroll deep into a large library, open a title, press Back: the grid repaints at the same place without starting from the top.
3. **Reconnecting.** Edit a server settings field without saving, drop the network for five seconds, restore it: the "Reconnecting" status covers the page, and the edit is still there afterwards. Repeat on Home without touching anything: Home reloads.
4. **Two tabs.** Signed in as profile A in two tabs, switch to B in tab one. Tab two goes to the sign-in screen on its next request; reload either tab: it opens as B (before: signed out).
5. **Plain HTTP, two tabs** (`http://<lan-ip>:32500`): leave both open past one token renewal (about 13 minutes): both stay signed in.
6. **Phone width** (under 720 px): the avatar opens a menu with Switch profile, Switch server, Send feedback and Help, and each works.
7. **Screen reader** (VoiceOver or NVDA) on a library grid: a card is announced with its position and the total; scroll far with a card focused: focus is on the list, and Tab continues from visible cards. In list view the rows are announced with their row number.
8. **Layout of the list-item wrappers.** Library grid, Saved, person credits, episode list and section grids must look exactly as before in grid and list view at three card sizes. The wrappers use `display: contents`; any change in spacing or row height is a failure.
9. **Episode card ⋯ button**: appears bottom-right of the still on hover and on focus, always on touch; it must not cover the watched tick. Right-click opens the same menu.
10. **Search.** Type a query: the address stays `/search`. Press Enter, reload: the query is under Recent searches. Clear, reload: it is gone. Turn the setting off: no recents at all.
11. **Plain HTTP note.** With the networking lane merged, open the sign-in page over HTTP from an address outside the home networks: one quiet line with a link above the legal links. From inside, or over HTTPS: no line.
12. **Language and formats.** Set Language & region to English (UK) and a 24-hour clock: Saved says "Favourites", the guide ruler and every date and time follow, including the server console and the metadata editor.
13. **Title page menu.** On a movie as owner: Delete, Play next and Add to queue (once something has played) appear, Download only when downloads are on.
14. **Production build** (`npm run build` in `web/`): no `Gallery`, `WindowedLab`, `ChannelsLab`, `BrowseLab` or `Features` chunk in `dist/assets`.
15. **Older server.** Point the app at a server that sends an unknown enum value or an extra row kind (or edit a response in dev tools): the page still shows, minus that part.

## 9. Not finished

- **Tolerant parsing not applied to two files:** `packages/client-core/src/guide/reads.ts` and `packages/client-core/src/linear-api.ts`. The Live TV lane is editing both; whoever merges that lane should apply the rules in `tolerant.ts` to them afterwards. Everything else is done or deliberately exact (section 10).
- **Owner Activity** (`activity.ts`): a playback row whose state or delivery this app does not know is still left out of the list. No web screen uses this service today; when one does, give the row a neutral "unknown" state so a viewer who is playing is never hidden from the owner.
- **Listening preferences** (`listening.ts`): a playback rate or timer length this app does not know still refuses the document, because saving sends the whole document back and a guessed value would overwrite the real one. Left for the player lanes.
- **Nothing was seen in a browser.** The grid wrappers, the episode ⋯ button, the phone-width menu, the reconnecting cover and the read-only limits dialog are the changes most likely to need a visual adjustment.
- **`currentI18n()`** remains in 19 files (37 calls). Most are in callbacks and error handlers. About a dozen are helper functions that run while a screen draws (filter and sort labels in `app/browse.ts`, card captions in `app/content.ts`, channel names in `app/channels.tsx`, the not-interested notice, the password hint). They read the right catalogue, because it is now set before screens draw, but they do not make their screen redraw by themselves when the language setting changes; that screen shows the new words the next time it draws.
- **Bugs not fixed:** B-16.4, B-16.7, B-16.14's two gaps, B-16.18, B-16.23, B-16.25, B-18.4, B-18.5, B-18.9, B-18.23, B-18.29, B-18.30, B-22.6, B-22.22; partly: B-16.5, B-16.12, B-16.17, B-18.27 (see section 3 for why).
- **See all** depends on a server field that is not sent yet (section 7).
- **Skipped-part counts** are collected but shown nowhere; a diagnostics panel could list them.
- **Unused strings:** roughly 200 unreferenced strings were kept deliberately (decision 15) and should be reviewed once the console and native lanes land.

## 10. Follow-up jobs

### 10.1 Merge of the base

Merged `audit-implementation/main` into the branch (`853e919`). Type-check, string lint and design lint are clean; the test files the merge touched pass. Both sides' behaviour is kept:

- **Plain-HTTP note.** There is now one reader and one sentence. The note calls the networking lane's route (`GET /v1/networking/connection`, through `HttpLocalApi.connection()`), shows that lane's sentence (`web.connectivity.httpOutsideHome`) and keeps this lane's link (`signIn.plainHttp.link`). My stand-in reader and my sentence (`signIn.plainHttp.note`) are gone. Any failure to read the route shows nothing; the note never blocks sign-in.
- **`server-administration.ts`.** Kept my row-by-row reading of addresses, interfaces and the custom certificate, and the networking lane's `connection`, `connectionLimit`, `advertiseUrl` and `serving`. The connection facts are read part by part: a document that cannot be read means "not secure, not remote, no note", never a refused Connectivity page. The file holds a NUL byte inside a pattern, so Git treats it as binary and showed no conflict markers; it was merged by hand.
- **One contradiction between lanes.** The networking lane's test expected an unknown discovery state to refuse the Connectivity page; mine reads it as a neutral value. I kept the tolerant reading and added the networking lane's connection assertions to the same test.
- **A defect that came in with the base.** `packages/client-core/test/v1-queue-player.test.ts` had lost a closing `});` in the base's own merge, so the file did not parse. Restored.
- **`web.account.title.hosted` / `.local`** stay removed: nothing refers to them.
- **What the other lanes added.** The playback lanes' new readers (`playback-v1/*`) are theirs and stay exact. `playback-v1/choices.ts` builds requests and reads no server document. Nothing else new needed the rules.

### 10.2 Native stand-in strings: old name → catalogue key

The native apps' `word(id)` and `engineText(id)` already read the catalogue under the same id once it is there, so for every row marked "same" the swap is: delete the stand-in entry and call `i18n.t('<id>')` at its call sites. The wording in the catalogue is character-for-character the stand-in's (checked by script).

From `apps/native/src/app/new-words.ts`:

| Stand-in | Catalogue key | File |
|---|---|---|
| `signIn.savedUnreadable` | same | `packages/i18n/src/catalog/en-US.ts` |
| `signIn.reset.title` | same | `en-US.ts` |
| `signIn.reset.body` | same | `en-US.ts` |
| `signIn.reset.action` | same | `en-US.ts` |
| `signIn.reset.confirmTitle` | same | `en-US.ts` |
| `signIn.reset.consequence` | same | `en-US.ts` |
| `signIn.reset.confirm` | same | `en-US.ts` |
| `signIn.reset.failed` | same | `en-US.ts` |
| `signIn.reset.working` | same | `en-US.ts` |
| `signIn.reset.reopen` | same | `en-US.ts` |
| `player.cannotPlay.title` | same | `en-US.ts` |
| `player.cannotPlay.thisTitle` | same | `en-US.ts` |
| `player.cannotPlay.gone` | same (`{title}`) | `en-US.ts` |
| `player.cannotPlay.denied` | same (`{title}`) | `en-US.ts` |
| `player.cannotPlay.unsupported` | same (`{title}`) | `en-US.ts` |
| `player.cannotPlay.conversionOff` | same (`{title}`) | `en-US.ts` |
| `player.cannotPlay.unreadable` | same (`{title}`) | `en-US.ts` |
| `player.cannotPlay.fileMissing` | same (`{title}`) | `en-US.ts` |
| `a11y.artwork.unavailable` | same | `en-US.ts` |
| `a11y.artwork.unavailableFor` | same (`{title}`) | `en-US.ts` |
| `saved.view.gathering` | same | `en-US.ts` |
| `saved.view.partial` | same (`{count, number}`) | `en-US.ts` |
| `settings.scope.handhelds` ("All your phones and tablets") | **`settings.scope.allPhones`** ("All phones and tablets") | `en-US.ts` |
| `settings.scope.televisions` ("All your TVs") | **`settings.scope.allTVs`** ("All TVs") | `en-US.ts` |

The last two were not added a second time: the catalogue already had these labels from the "This device" relabelling, with `settings.scope.allBrowsers` for web. The wording differs by the word "your". Native should not pick the key itself: `settingsScopeLabel(scope, platform)` in `packages/client-core/src/presentation/settings-structure.ts` returns the right key for a row.

From `apps/native/src/player/engine-strings.ts` (all in `packages/i18n/src/catalog/en-US/settings.ts`):

| Stand-in | Catalogue key |
|---|---|
| `settings.row.player` | same |
| `settings.help.player` | same |
| `settings.player.automatic` | same |
| `settings.player.native` | same |
| `settings.player.mpv` | same |

`engine-media.ts` has no words for a person. `EngineSettingRow.tsx` can be replaced by the shared row:

| Native stand-in | Shared settings structure |
|---|---|
| `EngineSettingRow` component | row `playerEngine` (a `composite` row, page `quality`, section `method`) in `settings-structure.ts` |
| `mpvAvailable` | capability `playerEngines: true` in the capabilities the app passes to the structure |
| the three values | `PLAYER_ENGINES` (`'automatic'`, `'native'`, `'mpv'`), type `PlayerEngineSetting` |
| where it is kept | `storage: 'device'`, scope "This device"; the app keeps writing `preferences.playerEngine` on the device |

The row is listed for phones and TVs only and only when the app reports `playerEngines`. Web never reports it, so web never shows it (`test/settings-scope.test.ts`).

### 10.3 Line-by-line re-read of the helpers' parser commits

I read every line of `git diff 0e03d99 HEAD` for each parser the stopped helpers changed: profile-management, account-sessions, administration, preferences, downloads, bulk-jobs, download-requests, home, library-content, show-workspace, recommendations, saved, personal-saved, notifications, account-notifications, activity, attention, console, library-management, library-pins, media-analysis, play-history, playback-history, prepared-media, remote-access, remote-sources, recording-policy, server-operations, storage-management, support-diagnostics, telemetry, server-administration and metadata-review. Three things were wrong and are fixed, each with a test:

1. **Sign-in review** (`account-sessions.ts`). Another device's sign-in of a kind or state this app did not know was left out of the list, so the owner of the account could not see or end it. It is now listed (unknown kind reads as an app sign-in; unknown state as active, or as ended when it carries an end time). This device's own row stays exact.
2. **Sharing** (`saved.ts`, `personal-saved.ts`). Someone a playlist or collection is shared with was left out when their role was unknown. They are now listed as a viewer, the least access.
3. **Who has access** (`server-administration.ts`). The lists of members, devices and API keys dropped a row whose role, trust or scope could not be read. An owner would then see a list of access with someone missing from it. These three lists are now all-or-nothing again: every row has to be readable or the page is refused with the "does not match this server" message. Extra fields are still ignored and a repeated row keeps its first appearance. This is a deliberate exception to "one bad row is dropped": hiding a person or a key is worse than asking the owner to update the app.

Everything else read correctly. Two things I left as they are and listed under Not finished: Owner Activity's playback rows, and listening preferences.

### 10.4 The remaining files

| File | Result |
|---|---|
| `index.ts` | `libraries()` returned the server's JSON unchecked; a missing list or a row without a name would break the rail. It now goes through `library-list.ts`: the answer must carry a list, a library without an id or name is left out alone, a repeat keeps the first, an unknown kind is kept and shown as a plain library. `system()`, `me()`, `item()` and `items()` return single documents whose identity fields are checked where they are used (sign-in, server identity); left as they are. |
| `guide/legacy-source.ts` | Has no reader of its own; it uses `parseChannelGuide` and the guide directory reader, which already follow the rules. Its own checks are a repeated cursor, a changed revision and a changed viewer, which stay exact. No change. |
| `preferences-reader.ts` | Its one check is the server and viewer the answer belongs to: identity, exact. The preferences themselves are read by `parsePreferenceSnapshot`, already tolerant. No change. |
| `console-transport.ts` | Size limits, content type and error codes of the transport: bounds, exact. No change. |
| `server-admin/metadata-agent.ts` | Already reads option by option (an unreadable option is skipped, a repeat keeps the first); the revision and the current choice stay exact because a change needs them. No change. |
| `server-admin/panel-logic.ts` | Builds requests and checks operation ids; reads no list. No change. |
| `inbox.ts` | Reads through the notification readers, already tolerant. No change. |
| `guide/reads.ts`, `linear-api.ts` | **Not touched**: the Live TV lane is editing them. |
| Playback, credentials, sessions, route identity, setup codes, hosted security | Deliberately exact, as before. |

### 10.5 Second merge of the base

Merged `audit-implementation/main` again (`b8635f0`) after the metadata providers, titles-and-files and API platform lanes landed. Type-check, string lint and design lint are clean; the touched test files pass. Nothing was run in a browser.

- **Held deletions are gone; missing titles replace them.** Followed the base: my row-tolerant `parseTrashPage` and its test are deleted, and Storage & backups shows the base's Missing titles panel. I kept only this lane's part of that screen: dates through the profile's formats. No new screen was built.
- **API keys.** Followed the base: the old `parseAccessAPIKey` shapes are deleted from `server-administration.ts` (merged by hand again, because of the NUL byte) and the API keys panel reads through `api-access.ts`, with this lane's catalogue wiring kept.
- **Delete results** (`administration.ts`). The base's new shape (`bytesDeleted`, `filesDeleted`) with this lane's reading: the removed and failed totals are counted from the receipts when the server's totals disagree; an unreadable receipt still refuses the result.
- **Title detail** (`detail.ts`). This lane's part-by-part reading plus the base's `partIndex`, `edition` and `missingSince`.
- **Library sources** (`library-inventory.ts`). This lane's row-by-row reading without the grace period the base removed.
- **Catalogue** (`en-US/web.ts`): both sides' strings kept. **`packages/contracts`**: the base's generated files, untouched by hand.
- **A test the automatic merge broke:** the storage test took its sample category from a report that no longer has one; it now builds its own.

Tolerant reading applied to the readers those lanes added (`2d916db`):

| Reader | Result |
|---|---|
| `metadata-attention.ts` | The library, page size, cursor and each row's revision stay exact. A title that cannot be read is left out alone; a repeat keeps its first row; an action this app does not know is not offered; a suggested match that cannot be read, or from an unknown provider, is left out, so it cannot be accepted by mistake. |
| `title-identity.ts` (missing titles) | Page size, cursor and total stay exact (the total is what the owner types to remove them all). An unreadable title is left out alone; a rule this app does not know reads as `never`, and each row still carries its own removal date. Receipts for emptying, merging and removing a library stay exact. |
| `api-access.ts` | Lists of keys, tokens and webhooks are read **whole**, like members and devices: an unreadable row, scope or state refuses the list rather than hiding a credential or a destination. The delivery log and the title query leave out one unreadable row; a scope on offer that this app does not know is not offered. The paths-changed report is a receipt and stays exact. |
| `metadata-settings.ts` | One document of plain values that is sent back on save; nothing enumerated. No change. |

Still for a later lane, as those lanes' reports say: the screens for tokens, webhooks, MCP, merging titles and the needs-attention list. The API keys panel still takes its scope names from `server-admin/panel-logic.ts`; the `api.scope.*` strings are in the catalogue for whoever builds those screens.
