# Portico Web Cinema — feature checklist

The reference is the current web UI in `apps/portico-server/web/src`: every route in
`app/router.tsx`, every folder under `screens/`, `player/`, `shell/`, `admin/`, the shared dialogs
and actions in `screens/shared/`, and the string catalogue (`packages/i18n/src/catalog/en-US.ts`
and `en-US/*.ts`), which surfaces features the route list hides.

Each row names the feature (with its source in the current web UI) and where it lives here.
Paths are real URLs; `?demo=<state>` opens a player, dialog or problem state over a page. Every
row is linked from the screen index at **`/dev/screens`** and screenshotted by `npm run shots`.

✅ = built, working on fixture data, and reviewed at 1440×900, 1920×1080, 1024×768 and 390×844.

## 1. Getting in (`screens/auth`, `app/session`, link routes)

| | Feature (current web source) | Where in Web Cinema |
|---|---|---|
| ✅ | Landing: finding servers, opening the last one (`auth/Gate`, `portico.landing.*`) | `/welcome/landing` |
| ✅ | Portico Account sign-in: username/email + password, Apple, Google, errors (`auth/SignIn`, `signIn.error.*`) | `/welcome/signin` (poster-wall frame) |
| ✅ | Create account: name, username rules, email, password, terms (`auth/AccountForm`) | `/welcome/create` |
| ✅ | Forgot password / reset link sent (`auth.forgotPassword*`, `web.reset`) | `/welcome/forgot` |
| ✅ | Two-step code at sign-in, recovery code, trust this browser (`web.twoStep`, `auth.enterVerificationCode`) | `/welcome/code` |
| ✅ | Direct server sign-in: address, username, password, owner recovery key (`auth/SignIn` direct, `web.direct`) | `/welcome/direct` |
| ✅ | Forced new password on first direct sign-in (`auth.changePassword`) | `/welcome/new-password` |
| ✅ | Hosted chooser: servers, current, last seen, unreachable, check now, sign in directly, sign out (`auth/HostedChooser`, `web.chooser`) | `/welcome/servers` |
| ✅ | Profile grid, PIN pad (keyboard too), wrong PIN, forgot PIN, remember profile (`auth/ProfileGrid`, `auth/DirectProfiles`, `pin.*`) | `/welcome/profiles` |
| ✅ | Server setup (first run): welcome, name, how people sign in, owner and recovery key (`auth/ServerSetup`, `web.setup`) | `/welcome/setup` |
| ✅ | Restore: checking saved connection, reconnecting, use another account (`auth/Restore`, `launch.*`) | `/welcome/restore` |
| ✅ | Sign in with a code from another device (TV code + QR, code refresh) (`activation.*`, `quickConnect.*`) | `/welcome/link-tv` |
| ✅ | Waiting for device approval (`device.approval.*`) | `/welcome/waiting` |
| ✅ | Server unavailable (`web.chooser.unreachable*`, `savedServer.*`) | `/welcome/unavailable` (the Gate sends you here for an unreachable server) |
| ✅ | Offline with downloads (`auth.offlineDownloads*`) | `/welcome/offline` |
| ✅ | Sign-in ended / server reset (`savedServer.endedTitle`, `web.connection.resetTitle`) | `/welcome/ended` |
| ✅ | Link a TV: enter code, review device, scope (server or account), approve, deny (`auth/DeviceApproval`, `linkTV.*`) | `/device` |
| ✅ | Claim a server for your Portico Account: approve or deny (`auth/ClaimApproval`, `web.claim`) | `/claim` |
| ✅ | Password reset link: new password, repeat, two-step code, done (`LinkRoutes` reset) | `/reset` |
| ✅ | Email verification: verifying, done, expired (`LinkRoutes` verify) | `/verify-email`, `/verify-email#token=expired` |
| ✅ | Join by invitation: with a Portico Account (choose profile, accept, decline) or with a code (direct join) (`LinkRoutes` join, `auth/DirectJoin`, `invitation.*`) | `/join` |
| ✅ | Join a Watch Together group from a link (`LinkRoutes` together join) | `/together/join#code=…` |
| ✅ | Account return after provider sign-in (`auth/AccountReturn`) | `/app/account-return` |
| ✅ | Server switcher: account servers, online/offline, current, direct sign-ins (`shell/ServerSwitcher`) | Profile menu › Switch server |
| ✅ | Profile switcher, PIN profiles go through the PIN pad (`settings.switchProfile`) | Profile menu › Switch profile |
| ✅ | Custody prompt: keep this server registered to you (`shell/CustodyPrompt`, `custody.*`) | Notice with Accept on `/server/general` |

## 2. Shell and navigation (`shell/Shell`, `shell/Connection`)

| | Feature | Where |
|---|---|---|
| ✅ | Left rail on desktop: Home, Search, Saved, Downloads, Watch Together, libraries, Channels (all, per source, Library Channels, Recordings), Manage server, Notifications, Settings, profile | `shell/Shell.tsx` — translucent rail over full-bleed art; collapses to icons with tooltips (pushes, never covers); tablet starts collapsed |
| ✅ | Phone: top bar (wordmark, downloads, notifications, profile) and bottom tabs (Home, Library picker, Channels, Saved, Search) | < 768 px |
| ✅ | Rail badges: unread count, in a group (Live), library scanning | Rail |
| ✅ | No libraries / add a library (owner) vs nothing shared (member) (`web.shell.noLibraries`, `shared/NoLibraries`) | Home empty state, library empty states |
| ✅ | Profile menu: who and where, switch profile, switch server, account, send feedback, help, sign out (`ProfileMenu`) | Rail profile item / phone avatar |
| ✅ | “/” opens Search from anywhere (`WEB-SEARCH-02`) | Global shortcut |
| ✅ | Reconnecting / offline banner (`shell/Connection`) | `/?connection=reconnecting`, `/?connection=offline` |
| ✅ | Not found and route error screens (`screens/NotFound`) | `/nothing/here`; router error component |
| ✅ | Skip to content link, landmarks, document titles per page | Shell, `useTitle` |
| ✅ | Hide features the profile or server has off (downloads, live TV, DVR, together) | Rail, tab bar, settings follow profile limits |

## 3. Home (`screens/home`, `app/home`)

| | Feature | Where |
|---|---|---|
| ✅ | Featured hero with details and play (`home.row.featured`, `web.home.details`) | `/` — full-bleed marquee carousel, pauses on hover/focus, arrow keys, reduced-motion aware |
| ✅ | Continue Watching with progress and time left; remove from Continue Watching with Undo (`web.continue.*`, `entry.continueRemoved`) | `/` row + card menu |
| ✅ | Up Next, Continue Listening | `/` rows |
| ✅ | Recently added / Recently added in {library} | `/` rows |
| ✅ | Recommendations: Picks for you, Because you watched, genre and mood for you, Starring, Directed by, Hidden gems, Top rated you haven’t seen, Trending, Popular on this server, Viewers also watched, Rediscover, New for you (`home.row.*`) | `/` rows (editorial titles with the subject in blue; Popular as a ranked Top 10) |
| ✅ | Watchlist and Favorites rows; saved views as rows | `/` rows |
| ✅ | Like, dislike and Not interested on recommendations, with Undo (`entry.notInterested*`) | Card preview and More menu |
| ✅ | A row’s See all as a grid (`HomeRowPage`) | `/home/rows/$rowId` |
| ✅ | Customize Home: show/hide, reorder, always-on rows, add a saved view, remove, reset to default (`home/Customise`, `home.customize.*`, `confirm.resetHome`) | `/?customize=true` |
| ✅ | Home empty state for owners and members (`web.empty.home*`) | `/` with no rows |
| ✅ | Editorial collection feature | `/` between rows |

## 4. Libraries (`screens/library`, `app/browse`, `app/libraries`)

| | Feature | Where |
|---|---|---|
| ✅ | Movies, TV, Anime, Music, Audiobooks, Photos, Home videos | `/library/$libraryId` |
| ✅ | Views: Discover, Browse, Collections, Categories, Releases, Songs, Authors, Series (+ Timeline) (`libraryViews`) | Library tabs (`?view=`) |
| ✅ | Library eyebrow (kind · count), library hero on Discover | Library header |
| ✅ | Grid and list layouts (`web.prefs.grid/list`) | Browse toolbar (`?layout=`) |
| ✅ | Quick filters (unwatched, in progress, watched, recently added, favorites, watchlist, 4K, HD) (`filter.quick.*`) | Browse chips |
| ✅ | Field filters with counts: genre, decade, content rating, mood, resolution, audio language, studio, network, length; find within long lists; year range; clear (`BrowseFilters`, `filter.field.*`) | Filters sheet (`?demo=filters`) |
| ✅ | Sort by and order (`web.filters.sort*`) | Browse toolbar |
| ✅ | Active filter chips, clear all, no-match state (`library.noMatch`) | Browse meta line |
| ✅ | Save view: name, show on Home, share (`SaveView`, `lib.saveView*`) | Browse › Save view |
| ✅ | Alphabet jump (`lib.jumpToLetter`) | Letter strip when sorted by title |
| ✅ | Large libraries: only visible pages are fetched and rendered (`ui/WindowedGrid`) | `WindowedGrid` + `useBrowse` |
| ✅ | Collections view; Categories view (genres, moods, decades, studios, networks) | Library tabs |
| ✅ | Entity pages: genre, mood, decade, studio, network, collection (rename, delete), author, series (`library/Entity`) | `/library/$libraryId/$view/$entityId` |
| ✅ | Scan library files, manage library (owner), scanning banner (`web.library.scan*`, `app/library-scan`) | Library ⋯ menu; Anime shows scanning |
| ✅ | Library empty states (`library.emptyLibrary*`, `library.emptyCollections*`) | Per view |
| ✅ | Multi-select and bulk actions (see §11) | Browse › Select (`?demo=select`) |

## 5. Titles (`screens/detail`, `app/detail`, `screens/people`)

| | Feature | Where |
|---|---|---|
| ✅ | Film hero: facts, rating, runtime, resolution/HDR/Atmos badges, critic and audience scores, tagline, synopsis with Read more (`detail/Detail`, `ui/TitleHero`) | `/media/$itemId` |
| ✅ | Play / Resume with time left, Play from beginning, Trailer (`entry.resumeRemaining`, `title.trailer`) | Action bar |
| ✅ | Versions and editions; audio and subtitle choice before playing (`title.versions`, `media-selection`) | Version / Audio / Subtitles pickers |
| ✅ | Media information per version (`title.mediaInformation`, `mediaInfo.*`) | Details section |
| ✅ | Cast and crew (starring, directed by, written by, all cast) | Credits lines + Cast & Crew row |
| ✅ | Extras (trailer, behind the scenes, interviews, deleted scenes); chapters | Extras and Chapters rows |
| ✅ | More like this | Row |
| ✅ | Watchlist, Watched, Favorite, Rate (stars, clear), Like/Dislike, Not interested (`title.rate`, `entry.*`) | Action bar + More menu + Rate dialog (`?demo=rate`) |
| ✅ | Add to playlist, add to collection (pick or create) (`EntryActions`, `playlist-add`) | Dialogs (`?demo=playlist`, `?demo=collection`) |
| ✅ | Play next, Add to queue, Watch with a group, Add to group queue (`entry.playNext`, `entry.group.*`) | More menu |
| ✅ | Download / Download all (quality; show policy) (`entry.download*`, `downloads.quality.*`) | Download dialog (`?demo=download`, `?demo=download-show`) |
| ✅ | Metadata editor: General with locks, Artwork, Tags & genres, Cast & crew, Fix match, History with undo; bulk edit (`shared/MetadataEditor`, `web.metadata.*`) | Sheet (`?demo=metadata`, `?demo=fix-match`) |
| ✅ | Refresh metadata; Delete (owner) with file option and trash (`app/delete-media-dialog`, `deleteMedia.*`) | More menu (`?demo=delete`) |
| ✅ | Report a problem: kind, category, message, diagnostics, sent, duplicate (`shared/FeedbackDialog`, `feedback.*`) | Dialog (`?demo=feedback`) |
| ✅ | Show: seasons (and Specials), season menu (play season, mark season watched/unwatched, download, add to playlist), episodes with progress and watched (`detail/Show`, `title.season*`) | `/show/$showId` |
| ✅ | Next up / Resume S·E on the show; jump to an episode (`title.jumpTo*`) | Show hero, Jump to dialog |
| ✅ | Episode panel with its own URL: details, versions, cast, previous/next, play/resume, mark watched (`detail/EpisodePanel`) | `/show/$showId/episode/$episodeId` |
| ✅ | Person: portrait, born/died, birthplace, bio with Read more, known for, credits all/acting/crew (`people/Person`, `web.person.*`) | `/person/$personId` |
| ✅ | Title not on the server | `/media/nothing-here` |

## 6. Music

| | Feature | Where |
|---|---|---|
| ✅ | Artist: circular portrait, bio, top songs, albums, singles & EPs, similar, play, shuffle | `/media/<artist>` |
| ✅ | Album: tracklist with discs, playing indicator, lyrics marker, per-track menu (play next, queue, playlist, favorite, go to album/artist, report) | `/media/<album>` |
| ✅ | Songs view | `/library/lib-music?view=songs` |
| ✅ | Playlists: play, shuffle, reorder, remove with Undo, rename, share, delete | `/saved/playlist/$id` |
| ✅ | Now Playing: artwork, scrubber, shuffle/repeat, volume, synced lyrics (click a line to seek), queue (play now, reorder, remove, save as playlist), Play on (`player/Lyrics`, `player/QueueList`) | `?demo=nowplaying`, `?demo=nowplaying-queue` |
| ✅ | Mini player (`player.mini*`) | Docked bottom; above the tab bar on phones (`?demo=mini`) |

## 7. Audiobooks

| | Feature | Where |
|---|---|---|
| ✅ | Book page: author (links to the author), narrator, series, chapters with where you are, resume chapter | `/media/<book>` |
| ✅ | Listening: chapters, back 15 / forward 30, speed, sleep timer (minutes, end of chapter, end of book) | Now Playing (`?demo=audiobook`) |
| ✅ | Authors and Series views | Audiobooks tabs |

## 8. Photos and home videos

| | Feature | Where |
|---|---|---|
| ✅ | Photo albums and a justified gallery | `/library/lib-photos`, `/media/<album>` |
| ✅ | Viewer: arrows and keys, info panel, download, slideshow with Ken Burns | `?photo=…`, `&slideshow=true` |
| ✅ | Home videos: browse, timeline, detail, playback | `/library/lib-home` |

## 9. Search (`screens/search`)

| | Feature | Where |
|---|---|---|
| ✅ | Search everything with kinds: All, Movies, Shows, Episodes, People, Artists, Albums, Songs, Books, Channels (`web.search.*`) | `/search` |
| ✅ | Grouped results with See all per group and counts | Results |
| ✅ | Recent searches and clear; suggestions | Idle state |
| ✅ | Validation (too short, too many words), no matches, try All | States |
| ✅ | Search text stays out of the URL (except `?q=` for links) | By design, as in the current web |

## 10. Saved (`screens/saved`)

| | Feature | Where |
|---|---|---|
| ✅ | Watchlist, Favorites, Playlists, Collections, Saved views, History tabs (`saved.tab.*`) | `/saved?view=` |
| ✅ | Filter (all/unwatched/in progress), sort and order; history period (`web.saved.sort.*`, `web.saved.period`) | Toolbars |
| ✅ | Clear history (confirm) (`web.saved.clearHistory*`) | History |
| ✅ | New playlist / new collection: name, description, shared (`web.saved.new*`) | Header action |
| ✅ | Resource pages: playlist, collection, saved view: rename, delete, remove item, reorder, open in library, show on Home (`saved/Resource`) | `/saved/$kind/$resourceId` |
| ✅ | Empty states per tab (`saved.empty.*`) | Each tab |

## 11. Selection bar (`shared/SelectionBar`, `bulk-job`)

| | Feature | Where |
|---|---|---|
| ✅ | Select mode, count, select all, clear, done | Browse › Select |
| ✅ | Bulk: watched, unwatched, watchlist add/remove, favorites add/remove, playlist, collection, download, edit metadata, refresh, delete; progress and results | Floating bar |

## 12. Live TV (`screens/channels`, `screens/live`, `app/channels`)

| | Feature | Where |
|---|---|---|
| ✅ | Channels by source: all, each source, Library Channels, Recordings (`channels.*`) | `/channels/$entry` (+ `/live` redirects) |
| ✅ | On now row | Top of Channels |
| ✅ | Guide grid: time header, now line, earlier/later, now, day (yesterday/today/tomorrow), groups, favorites; keyboard (arrows, Enter, W, R, N) (`guide.*`) | Guide view |
| ✅ | Channel list: numbers, logos, now and next with progress, favorites, hidden (show hidden), search channels and programs, sort (`channels.sort*`) | `?view=list` |
| ✅ | Programme sheet: watch live, remind, record, record series (new/all, this/any channel, keep, start early, end late), more for channel, favorite, hide (`guide.*`) | Click any programme |
| ✅ | Reminders fire as a toast 2 minutes before, while Portico is open | `app/Reminders` |
| ✅ | Recordings: Upcoming, Recorded, Series rules, History, Storage; conflicts (skip / record instead), cancel, stop, keep, delete, edit/pause/resume/delete rule (`live/DVR`, `recordings.*`) | `/channels/recordings?tab=` |
| ✅ | Stale guide notice, no channels, Live TV off for profile, source missing (`live.stale*`, `channels.noSources*`) | States |
| ✅ | Live player: live badge, behind live and Jump to live, channel up/down, mini guide, captions, record (`player/MiniGuide`, `player/LiveCaptions`, `web.player.channel*`) | `?demo=live`, `?demo=live-guide` |
| ✅ | Channel problem (`web.channelProblem`) | `?demo=channel-problem` |

## 13. Player (`player/*`)

| | Feature | Where |
|---|---|---|
| ✅ | Chrome: play/pause, back/forward 10, next, volume, time, title, close, minimize, full screen, picture in picture (`player/Player`) | `?demo=player` |
| ✅ | Timeline with preview frames, chapter ticks, intro and credits tinted; keyboard seeking (`player/Timeline`) | Scrubber |
| ✅ | Chapters list | Chapters panel |
| ✅ | Skip intro / next episode during credits / skip credits (`player/SkipPrompt`) | `?demo=player-intro` |
| ✅ | Audio, subtitles (origin, forced, find subtitles, timing), quality (automatic, original, convert), speed, sleep timer, playback info (`player/options`, `player.opt.*`) | Panels (`?demo=player-audio`, `player-options`, `player-info`) |
| ✅ | Post-play: next episode countdown, cancel, Still watching?, film finished with Replay, Rate this, More like this (`player/PostPlay`, `player.post.*`) | `?demo=postplay`, `still-watching`, `postplay-film` |
| ✅ | Queue in the player: play now, earlier/later, remove, shuffle, repeat (`player/QueueList`) | `?demo=player-queue` |
| ✅ | Play on: this browser, Portico devices, AirPlay, Google Cast; connecting, waiting to allow, failed, playing elsewhere (`player/PlayOn`, `playOn.*`, `cast.*`) | `?demo=player-playon` |
| ✅ | Keyboard shortcuts and the shortcuts list (`player.shortcuts`) | `?`, `?demo=player-shortcuts` |
| ✅ | Report a problem from the player | Top bar flag |
| ✅ | Owner stopped playback with message; unplayable; reconnecting; Dolby Vision warning (`web.player.ownerStopped*`, `player.*`) | `?demo=owner-stopped`, `unplayable`, `player-reconnecting`, `dolby-vision` |
| ✅ | Watch Together group pill (`player/GroupPill`) | `?demo=group` |

## 14. Watch Together (`screens/together`, `app/together`)

| | Feature | Where |
|---|---|---|
| ✅ | Start a group (name, who controls), join with a code, groups on this server (`web.together.*`) | `/together` |
| ✅ | Group: people with status, host, make host, invite code (copy code, copy link, new code), queue (watch this, remove), pause/play for everyone, leave (as host: confirm), end for everyone, host reconnecting, ended (`confirm.endGroup`, `confirm.leaveAsHost`) | `/together?group=tg-1` |

## 15. Downloads (`screens/downloads`, `app/downloads`)

| | Feature | Where |
|---|---|---|
| ✅ | On this device: progress, pause/resume, remove (confirm), play, storage used/free (`downloads.*`) | `/downloads` |
| ✅ | On your server: preparing, ready to save, expired, stop preparing (`confirm.stopPreparing`, `downloads.requestPreparing`) | `/downloads?tab=server` |
| ✅ | Whole shows with policy (unwatched, next 3, all) | Server tab |
| ✅ | Settings: Wi-Fi only, remove after watching, next episode, quality, storage limit | `/downloads?tab=settings` |
| ✅ | Empty and off-for-profile states | States |

## 16. Account (`screens/account`, `settings/Security`, `settings/Profiles`)

| | Feature | Where |
|---|---|---|
| ✅ | Portico Account overview: profiles, name, email, username (`account/Account`) | `/account` |
| ✅ | Security: password change with strength, email change, Apple/Google connect/disconnect, two-step on/off with QR and setup key, recovery codes, delete account (type to confirm) (`account/Security`, `web.hostedSecurity`, `web.deleteAccount`) | `/account?tab=security` |
| ✅ | Where you’re signed in: approve waiting, trust, rename, sign out one, sign out everywhere else (`settings/Security` DevicesSection, `accountSessions.*`) | `/account?tab=sessions`, `/settings/account` |
| ✅ | Servers on the account: owner, leave (`account/Servers`, `membership.*`) | `/account?tab=servers` |
| ✅ | Invitations: waiting (accept, decline), paste a link, earlier (`web.hostedAccount.*`, `invitations.*`) | `/account?tab=invitations` |
| ✅ | Email preferences and download your data (`account/Notifications`) | `/account?tab=notifications` |

## 17. Settings (`screens/settings`)

| | Feature | Where |
|---|---|---|
| ✅ | Settings hub: profile, server, preferences, profiles, notifications, downloads, together, link a TV, account, about, server console (owner), sign out (confirm) | `/settings` |
| ✅ | Preferences from the server registry: playback (autoplay, countdown, still-watching check, intro/recap/credits skip, skip lengths, speed, thresholds), audio and subtitles (languages, default on, size, background, lyrics), quality per network and HDR, streaming (direct play, repackage, conversion, receive from other devices), music, privacy, search history, language and region, notifications badge; saved as you go (`web.prefs.*`, `pref.*`) | `/settings/preferences` |
| ✅ | Appearance for this browser: backdrops, card size, text size, reduce motion, hover previews | Preferences › Appearance |
| ✅ | Reset recommendations (confirm) (`settings/ResetRecommendations`) | Preferences › Recommendations |
| ✅ | Profiles: add, name, color, picture, kids, PIN set/change/remove, limits (highest rating, unrated, downloads, live TV, recording, together, hidden labels), reorder, delete (type to confirm); only the main profile manages (`settings/Profiles`, `limits.*`, `profile.*`) | `/settings/profiles` |
| ✅ | Account on this server: switch profile/server, password, two-step, devices | `/settings/account` |
| ✅ | Servers: connected to, your servers, check now, switch, leave, sign in directly (`servers.*`) | `/settings/servers` |
| ✅ | Notifications inbox: all/unread/archived, mark read/unread, archive with Undo, mark all read, load more, what to be told about (`settings/Notifications`, `notifications.*`) | `/settings/notifications` |
| ✅ | About: versions, help, terms, privacy, licenses, TMDB credit, screen index (`about.*`, `web.about.*`) | `/settings/about` |

## 18. Server administration (`screens/server`, `admin/*`) — owner and admins

| | Section (current web) | Where |
|---|---|---|
| ✅ | Overview: stats, attention items (open, dismiss), health, 24-hour telemetry charts, 30-day viewing, storage, recently played (`server/Overview`, `OverviewCharts`) | `/server/overview` |
| ✅ | Now Playing: sessions with delivery, hardware, network; stop with a message (`server/NowPlaying`) | `/server/now-playing` |
| ✅ | Activity: playing now, background tasks with progress, cancel, retry, filters (`server/Activity`) | `/server/activity` |
| ✅ | Logs: level, filter, follow, detailed capture, retention, support export (`server/Logs`) | `/server/logs` |
| ✅ | Diagnostics: capabilities with reasons and technical details, connectivity checks, run again (`server/Diagnostics`, `web.connectivity`, `web.capabilities`) | `/server/diagnostics` |
| ✅ | Feedback inbox: reports, status, reply (`server/Feedback`) | `/server/feedback` |
| ✅ | Alerts: storage and certificate thresholds, offline, failed scans (`server/AlertThresholds`) | `/server/alerts` |
| ✅ | Libraries: cards with folders, scan/cancel, scan all, files needing attention, rename, remove (type to confirm), add library (kind, folder picker, metadata source) (`server/Libraries`, `web.folderPicker`) | `/server/libraries` |
| ✅ | Library settings: matching source, language, country, lyrics, rescan, missing after, seek-bar pictures, chapter pictures, deleting, keep deleted (`server/LibrarySettings`) | `/server/library-settings?id=` |
| ✅ | Metadata: providers, API keys, consent, refresh, lyrics, look-up tool (`server/MetadataSource`, `MetadataLookup`) | `/server/metadata` |
| ✅ | Storage: WebDAV shares and rclone remotes, add, mount/unmount, remove (`server/Storage`) | `/server/storage` |
| ✅ | Live TV: sources (tuner discovery, M3U, Xtream, XMLTV, upload), source settings (guide from, numbering, logos, pause, remove), channel map, refresh guide, recordings folder and space (`server/LiveTV`, `LiveSourceExtras`, `web.liveSource`) | `/server/live-tv` |
| ✅ | DVR: tuners, defaults, keep, after recording, folder patterns, sidecars, live defaults, who can record, series rules (`server/DVRSettings`, `web.dvrSettings`, `web.recordingRules`) | `/server/dvr` |
| ✅ | Library Channels: list, health, on/off, rebuild, logo, delete, templates (`server/Channels`, `web.libraryChannels`) | `/server/channels` |
| ✅ | Channel Builder: name, number, libraries, kinds, conditions, order, episodes, advanced, live preview (`server/ChannelBuilder`, `channel-builder.ts`) | `/server/channel-builder` |
| ✅ | Transcoding: allow, limits, hardware, tone mapping, isolation, per-account limits (`server/Transcoding`) | `/server/transcoding` |
| ✅ | Network: status, remote access, who from outside, port mapping, port, encryption, remote quality, upload, idle pause, advanced (`server/Network`, `web.network`) | `/server/network` |
| ✅ | People: accounts, profiles, role, libraries, limits, disable, delete, add server account (`server/People`) | `/server/people` |
| ✅ | Access & API keys: invitations (create, copy, revoke), devices (approve, block), API keys (create shown once, revoke) (`server/Access`) | `/server/access` |
| ✅ | Schedules: tasks, times, length, catch up, on/off, last result (`server/Schedules`, `MaintenanceWindows`) | `/server/schedules` |
| ✅ | Maintenance: update, permissions fix, quiet hours, priority, storage cleanup, deleted titles (put back, remove due), message everyone (`server/Maintenance`, `web.broadcast`, `web.trash`) | `/server/maintenance` |
| ✅ | Backups: list, back up now, restore (confirm), restore from file, download, delete (`server/Backups`) | `/server/backups` |
| ✅ | General: name, version, ID, data folder, Portico Account connection, recovery owner, custody prompt (`server/General`, `web.general`) | `/server/general` |
| ✅ | Owner/admin gate for members (`web.console.owner*`) | `/server/overview?as=member` |
| ✅ | Unsaved changes bar (Discard / Save) on every settings form | All forms |

## 19. States

| | Feature | Where |
|---|---|---|
| ✅ | Loading skeletons (heroes, rows, grids, detail, lists, admin) with simulated latency (`?latency=`) | Everywhere; `/?latency=4`; Preferences › Demo data |
| ✅ | Empty states | Every list and page |
| ✅ | Errors with Try again | `ErrorState` on every data page |
| ✅ | Toasts with Undo, announced politely | Global |
| ✅ | Reduced motion (system setting and in-app) | `prefers-reduced-motion`, Preferences › Appearance |
| ✅ | Keyboard: visible focus everywhere, roving arrows in rows, tabs, segmented controls and the guide; Radix dialogs and menus trap and return focus | Everywhere |
