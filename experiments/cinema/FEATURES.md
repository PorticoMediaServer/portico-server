# Portico Cinema — feature inventory

The reference is Portico's web UI (`apps/portico-server/web/src`): its routes (`app/router.tsx`),
every folder under `screens/`, `player/`, `shell/` and the dialogs and actions under
`screens/shared/`, plus the brief's additions (photos, home videos, audiobook sleep timer, TV text entry).

Each row names the web source, what the feature is, and where it lives in Cinema.
Screen ids in `code` open directly with the debug deep link `portico-cinema://screen/<id>` or from
Settings › About › Screen index. ✅ = built and walked on iPhone/iPad and Apple TV.

## 1. Getting in

| | Feature (web source) | Where in Cinema |
|---|---|---|
| ✅ | Landing / launch: finding servers, opening a server (`auth/Gate`, `portico.landing.*`) | `landing` — marquee splash, "Finding your servers" then opens the last server |
| ✅ | Portico Account sign-in: username/email + password, Sign in with Apple/Google, create account, forgot password, email code (`auth/SignIn`, `auth/AccountForm`) | `signin` (+ `signin-create`, `signin-forgot`) |
| ✅ | Hosted chooser: pick a server, current server, last seen, unreachable/waiting states, check now, sign in directly (`auth/HostedChooser`) | `chooser` |
| ✅ | Direct server sign-in: address, username, password, two-step code step, forced new-password step, owner recovery sign-in (`auth/SignIn` direct) | `signin-direct` (+ code and new-password steps) |
| ✅ | Device approval wait ("waiting for the owner to approve") (`device.approval.*`) | `device-waiting` |
| ✅ | Joining by invitation: Portico Account invite (accept/decline, libraries shared) and direct join (code, name, username, password) (`auth/DirectJoin`, `LinkRoutes` join) | `join`, `join-direct` |
| ✅ | Profile grid with PIN entry, remember profile, other profiles (`auth/ProfileGrid`, `auth/DirectProfiles`) | `profiles` — who's watching, PIN pad |
| ✅ | Link a TV with a code or QR (TV shows code + QR; phone enters code, reviews and approves device) (`auth/DeviceApproval`, `settings.linkTV`) | TV: `link-tv` (code + QR); phone: `device-approve` |
| ✅ | Claiming a server: connect server to Portico Account, approve/deny, owner profile (`auth/ClaimApproval`, `ClaimConnectButton`) | `claim` |
| ✅ | Server setup first run: welcome, server name, how people sign in, recovery owner (`auth/ServerSetup`) | `setup` (multi-step) |
| ✅ | Restore: checking saved connection, reconnecting, use another account (`auth/Restore`) | `restore` |
| ✅ | Email verification (`LinkRoutes` verify) | `verify-email` |
| ✅ | Password reset: new password, confirm, two-step code (`LinkRoutes` reset) | `reset-password` |
| ✅ | Account return (finishing provider sign-in) (`auth/AccountReturn`) | `account-return` |
| ✅ | Server unavailable / offline states (`web.chooser.unreachable*`, `web.signIn.error.*`) | `server-unavailable`, `offline` |
| ✅ | Sign-in ended / server reset → sign-in screen (`web.connection.resetTitle`, `web.signIn.ended`) | `signin-ended` |
| ✅ | Server switcher: account servers, online/offline/can't reach, current, signed in directly, choose profile (`shell/ServerSwitcher`) | Profile menu › Switch server, or the iPhone header server pill (`chooser`) |
| ✅ | Profile switcher (`settings.switchProfile`) | Profile menu › Switch profile (`profiles`) |
| ✅ | Custody prompt: accept ownership of server registration (`shell/CustodyPrompt`) | Custody banner with Accept on Server › General (`admin/general`) and in `states` |

## 2. Shell and navigation

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Primary navigation: Home, Search, Saved, Downloads, Live TV, Watch Together, Libraries list, Manage server, Settings (`shell/Shell`) | TV: the Aisle, a left rail that widens on focus; iPad: the same rail as a collapsible sidebar; iPhone: five tabs (Home, Library, Live, Saved, Search) plus a header with the server switcher and profile menu |
| ✅ | Rail badges: unread notifications, in a group, library scanning (`web.shell.unread/inGroup/scanning`) | Rail item badges |
| ✅ | No libraries in rail / add a library (`web.shell.noLibraries`) | Rail empty state + `no-libraries` |
| ✅ | Profile menu: settings, switch profile, switch server, account, send feedback, help, sign out (`shell/Shell`) | Rail profile item (TV/iPad) or header avatar (iPhone) › profile menu |
| ✅ | Reconnecting banner (`shell/Connection`) | Global banner (`reconnecting` demo) |
| ✅ | Not found (`screens/NotFound`) | `not-found` |
| ✅ | Notifications inbox (`app/inbox`, `settings/Notifications`): all/unread/archived, mark read/unread, archive, mark all read, load more | `notifications` |

## 3. Home

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Hero / featured content (`home.row.featured`, Home details button) | Marquee hero that follows focus + featured carousel |
| ✅ | Continue Watching / Continue Listening with progress, resume, remove from Continue Watching + undo (`web.continue.*`) | Home rows, card menu |
| ✅ | Up Next | Home row |
| ✅ | Recently added / Recently added in {library} | Home rows |
| ✅ | Recommendations: Picks for you, Because you watched, More like, Starring, Directed by, genre/mood/theme for you, Hidden gems, Top rated you haven't seen, Rediscover, Trending, Popular on this server, Viewers also watched, New for you (`home.row.*`) | Home rows (editorial titles) |
| ✅ | Watchlist and Favorites rows | Home rows |
| ✅ | Not interested on recommendation cards (+ undo) (`entry.notInterested`) | Card menu on recommendation rows |
| ✅ | Row See all page (`HomeRowPage`) | `home-row` |
| ✅ | Customise Home: show/hide rows, reorder, always-on rows, add saved view row, remove, reset to default (`home/Customise`) | `home-customise` |
| ✅ | Home empty state (owner vs member) (`web.empty.home*`) | `home-empty` |

## 4. Libraries

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Library kinds: Movies, TV, Anime, Music, Audiobooks (+ Photos, Home videos per brief) | Rail › Libraries |
| ✅ | Library views: Discover, Browse, Collections, Categories (+ Releases, Songs, Authors, Series for music/books) (`router libraryViews`) | Library screen tabs |
| ✅ | Switch library, eyebrow (kind · count) | Library header |
| ✅ | Browse grid/list toggle (`web.prefs.grid/list`) | Browse toolbar |
| ✅ | Filters: quick filters (unwatched, in progress, recently added, favorites, watchlist, HD, 4K…) and fields (genre, decade, year, rating, studio, network, resolution, audio language, person…), range, yes/no, find, clear all (`BrowseFilters`) | Filters sheet |
| ✅ | Sort by + order (`web.filters.sort*`) | Sort sheet |
| ✅ | Save view (name, share) (`SaveView`) | Save view dialog |
| ✅ | Alphabet jump (`web.filters.jumpTo`) | Letter strip on Browse |
| ✅ | Category chip / Categories view | Categories tab → entity page |
| ✅ | Entity pages: genre, studio, network, collection, decade, playlist, artist, author, series (`library/Entity`) | `entity` |
| ✅ | Scan library files / manage library (owner) + scanning state (`web.library.scan*`) | Library ⋯ menu, scanning banner |
| ✅ | Library empty / no match / no collections states | Library states |
| ✅ | No libraries state (owner: add library; member: nothing shared) (`shared/NoLibraries`) | `no-libraries` |

## 5. Titles

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Movie detail: hero, facts (year, rating, runtime, resolution, score), synopsis, genres (`detail/Detail`) | `movie` |
| ✅ | Play / Resume (remaining) / Play from beginning, Trailer | Detail actions |
| ✅ | Versions and editions picker | Detail › Versions sheet |
| ✅ | Audio and subtitle choices before playing | Detail › Audio & subtitles sheet |
| ✅ | Media information (container, video, audio, resolution, duration per version) | Detail › Details tab |
| ✅ | Cast and crew (directed by, written by, all cast/all crew) | Detail › Cast & Crew |
| ✅ | Extras (trailers, featurettes) | Detail › Extras |
| ✅ | Related titles (More like this) | Detail › More like this |
| ✅ | Watchlist, Favorite, Rate (stars, clear), Watched/Unwatched, Like/Dislike, Not interested | Detail action bar + More menu |
| ✅ | Add to playlist, Add to collection (pick or create) (`EntryActions`, `playlist-add`) | Add to playlist sheet |
| ✅ | Play next, Add to queue, Watch with group, Add to group queue | More menu |
| ✅ | Download… / Download all… (quality choice) | More menu → download sheet |
| ✅ | Metadata editor: General fields with lock/unlock, Artwork chooser, Tags & genres, Cast & crew, Fix match (search provider, candidates, use match), History (undo) (`shared/MetadataEditor`) | `metadata-editor` |
| ✅ | Refresh metadata, Delete… (owner) (`app/delete-media-dialog`) | More menu → confirm |
| ✅ | Feedback / Report a problem (kind, category, message, diagnostics, sent/duplicate) (`shared/FeedbackDialog`) | `feedback` |
| ✅ | Show detail: seasons, season menu (play season, mark season watched), episodes list, watched counts (`detail/Show`) | `show` |
| ✅ | Episode panel/page: details, cast, previous/next episode, play/resume (`detail/EpisodePanel`) | `episode` |
| ✅ | Next up on show (resume S1 E3) | Show hero button |
| ✅ | Mark show watched/unwatched | Show more menu |
| ✅ | Specials, jump to season/episode | Season picker + Jump to |
| ✅ | Person page: bio (read more), born/died, known for, credits all/cast/crew (`people/Person`) | `person` |

## 6. Music

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Artist page: circular hero, albums, appearances, top songs, shuffle | `artist` |
| ✅ | Album page: tracklist with discs, play, shuffle, add to queue/playlist | `album` |
| ✅ | Songs view / track lists | Music library › Songs |
| ✅ | Playlists: detail, play, shuffle, reorder, remove, rename, delete | `playlist` |
| ✅ | Now playing screen: artwork, scrubber, controls, shuffle/repeat, lyrics (synced), queue (`player/Lyrics`, `QueueList`) | `now-playing` |
| ✅ | Queue: reorder, remove, play now, save as playlist | Now playing › Queue |
| ✅ | Mini player (`player.minimize`, mini states) | Floating mini player above content |

## 7. Audiobooks

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Audiobook detail: author, narrator, series, chapters, resume chapter | `audiobook` |
| ✅ | Listening player: chapters, speed, sleep timer (minutes / end of chapter) | `now-playing` (audiobook mode) |
| ✅ | Authors and Series views | Audiobooks library tabs |

## 8. Photos and home videos (brief)

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Photo library: albums, gallery grid | `photos` library, `photo-album` |
| ✅ | Photo viewer with swipe/arrow navigation and slideshow | `photo-viewer` |
| ✅ | Home videos library with playback | Home videos library → player |

## 9. Search

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Search across all types with type filters (All, Movies, Shows, Episodes, People, Artists, Albums, Songs, Books, Channels) (`search/Search`) | `search` |
| ✅ | Grouped results, see all per group, counts | Search results |
| ✅ | Recent searches + clear | Search empty state |
| ✅ | Validation (too short / too many words), no matches, partial results | Search states |
| ✅ | TV text entry | On-screen keyboard grid on TV |

## 10. Saved

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Watchlist, Favorites, Playlists, Collections, Saved views, History tabs (`saved/Saved`) | `saved` |
| ✅ | Filters (all/unwatched/in progress), sort + order, period for history | Saved toolbar |
| ✅ | Clear history (confirm) | History tab |
| ✅ | New playlist / New collection (name, description, shared) | Saved create dialog |
| ✅ | Saved resource page: playlist/collection/saved view with rename, delete, remove item, reorder, go to item (`saved/Resource`) | `resource` |
| ✅ | Empty states per tab | Saved |

## 11. Selection bar

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Multi-select in grids, select all, count (`shared/SelectionBar`) | Browse › Select |
| ✅ | Bulk: watched/unwatched, watchlist add/remove, favorite add/remove, add to playlist, add to collection, edit metadata (bulk), refresh, delete; progress and partial results | Selection bar |

## 12. Live TV

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Channel list (numbers, logos, now/next, favorites, hidden, groups, sources, search, sort) (`channels/Channels`) | `live` › Channels |
| ✅ | Guide grid: timeline, now line, earlier/later, jump to now, day picker, filters | `live` › Guide |
| ✅ | Programme sheet: watch live, record, record series (new/all, keep, start early/end late), remind, more for channel, favorite/hide channel | `programme` sheet |
| ✅ | Reminders (set, stop) | Programme sheet + badge |
| ✅ | DVR: Upcoming, Recorded, Series rules, History, Storage; conflicts (skip / record instead), cancel, delete, keep, edit/pause/resume/delete rule (`live/DVR`) | `dvr` |
| ✅ | Library Channels (custom channels) in Live TV | Live › Library Channels group |
| ✅ | Live states: no sources, no channels, stale guide, server-wide unavailable, channel problem | Live states |
| ✅ | Live player: live badge, behind live, jump to live, channel up/down, in-player mini guide, live captions, record (`player/MiniGuide`, `LiveCaptions`) | `live-player` |

## 13. Player

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Video chrome: play/pause, skip back/forward, title, close, minimize (`player/Player`) | `player` |
| ✅ | Timeline with preview thumbnails, chapter marks, intro/credits segments (`player/Timeline`) | Player scrubber |
| ✅ | Chapters list | Player › Chapters |
| ✅ | Skip intro / recap / credits prompt (`player/SkipPrompt`) | Player prompt |
| ✅ | Audio, subtitles (timing, find subtitles), quality (automatic/original/convert), speed options; playback info (`player.opt.*`) | Player options panel |
| ✅ | Post-play next episode countdown, still watching?, replay, rate this, more like (`player/PostPlay`) | Post-play overlay |
| ✅ | Queue list in player (`player/QueueList`) | Player › Up next |
| ✅ | Play on: AirPlay, Google Cast, another Portico device; connecting/approve/failure states (`player/PlayOn`, `app/cast`) | Play on sheet |
| ✅ | Report a problem from the player | Player › More |
| ✅ | Owner stopped playback message; unplayable; reconnecting; Dolby Vision warning | Player states |
| ✅ | Watch Together group pill in player (`player/GroupPill`) | Player pill |

## 14. Watch Together

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Lobby: start a group (name, who controls), join with code (`together/Together`) | `together` |
| ✅ | Group page: members with status, host controls, make host, invite code (copy/new), queue (watch this, remove), play/pause for everyone, leave, end for everyone, reconnecting/host away/ended states | `together-group` |
| ✅ | Join via link (`together/join`) | `together-join` |
| ✅ | Group pill | Player + rail badge |

## 15. Downloads (phone and tablet)

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | On this device list with progress, pause/resume, remove, storage used/free (`screens/downloads`) | `downloads` |
| ✅ | On your server: prepared downloads, states (waiting, preparing, ready, paused, expired), stop preparing, save | Downloads › On your server |
| ✅ | Whole shows and seasons requests with policy (all / unwatched / next N) | Downloads › Requests |
| ✅ | Download settings: Wi-Fi only, remove after watching, download next episode, storage limit, quality | Downloads › Settings |
| ✅ | Downloads off / empty states | Downloads states |

## 16. Account

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Portico Account overview: profiles (add, rename, color, picture, PIN, delete, reorder) (`account/Account`, `settings/Profiles`) | `account` › Profiles |
| ✅ | Security: password change, email change, sign-in methods (Apple/Google connect/disconnect), two-step on/off with QR, recovery codes, delete account (`account/Security`) | `account` › Security |
| ✅ | Where you're signed in (sessions), sign out one / all others (`account/Account`) | `account` › Sessions |
| ✅ | Devices (server account): approve, trusted, rename, remove, sign out everywhere (`settings/Security`) | `account` › Devices |
| ✅ | Account notifications + preferences + download your data (`account/Notifications`) | `account` › Notifications |
| ✅ | Servers on the account: leave server, owner (`account/Servers`) | `account` › Servers |
| ✅ | Invitations: waiting, accept (choose profile), decline, paste link, earlier (`web.hostedAccount.*`) | `account` › Invitations |

## 17. Settings

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Settings hub: account, server, preferences, notifications, downloads, together, link a TV, about, open server console, sign out (`settings/Settings`) | `settings` |
| ✅ | Preferences: playback quality per network (mode, resolution, HDR), audio/subtitle languages, subtitle style, library view, reduce motion, text size, play from other devices (`web.prefs.*`, `preferences.*`) | `settings-preferences` |
| ✅ | Profiles and limits (rating ceiling, unrated, features: downloads/live/DVR/together, hidden labels) (`settings/Security` limits) | `settings-profiles`, `profile-limits` |
| ✅ | Notifications settings | `notifications` |
| ✅ | Security (two-step, devices) | `account` |
| ✅ | Reset recommendations (confirm) (`settings/ResetRecommendations`) | `settings-preferences` |
| ✅ | Servers: connected to, your servers, refresh, switch, disconnect | `settings-servers` |
| ✅ | About: versions, legal, terms, privacy, help, TMDB | `settings-about` (+ Screen index) |

## 18. Server administration (owner and admin only)

| | Section (web `/server/$section`) | Where in Cinema |
|---|---|---|
| ✅ | Overview: health, attention items (acknowledge), who is watching, telemetry charts, recent playback | `admin/overview` |
| ✅ | Now Playing: sessions, delivery, stop with message | `admin/now-playing` |
| ✅ | Activity: playback now/recent, background tasks | `admin/activity` |
| ✅ | Logs: messages, level filter, following, capture, support export, retention | `admin/logs` |
| ✅ | Diagnostics: server capabilities, connectivity checks | `admin/diagnostics` |
| ✅ | Viewer feedback inbox: reports, status, reply | `admin/feedback` |
| ✅ | Libraries: list, add library (kind, folder picker, metadata source), scan, cancel scan, folders, files needing attention, rename | `admin/libraries` |
| ✅ | Library settings: matching, language, trickplay, deleting, rescan interval, missing after, fetch lyrics | `admin/library-settings` |
| ✅ | Metadata: metadata source (online/local), lookups, agents | `admin/metadata` |
| ✅ | Storage: WebDAV shares, rclone remotes, mount/unmount | `admin/storage` |
| ✅ | Live TV: sources (add remote/discover/upload), source settings, channel map, refresh guide | `admin/live-tv` |
| ✅ | DVR: recording defaults, tuners, live defaults, who can record, recording rules, recording space | `admin/dvr` |
| ✅ | Channels (Library Channels): list, health, on/off, regenerate, logo, delete, templates | `admin/channels` |
| ✅ | Channel Builder: name, libraries, kinds, conditions, order, episodes, advanced, preview | `admin/channel-builder` |
| ✅ | Transcoding: allow, capacity, per-account overrides | `admin/transcoding` |
| ✅ | Network: remote access, addresses, port mapping, certificate, access from outside, advanced | `admin/network` |
| ✅ | People: accounts, profiles, add account, edit, libraries they can see, disable, delete | `admin/people` |
| ✅ | Access & API keys: roles, limits, invitations (create/copy/revoke), devices (approve/block), API keys (create/revoke) | `admin/access` |
| ✅ | Schedules: tasks, enabled, window, catch up | `admin/schedules` |
| ✅ | Maintenance: quiet hours, background priority, storage cleanup, deleted titles (put back, remove), updates, permissions, broadcast message | `admin/maintenance` |
| ✅ | Backups: list, back up now, restore, restore from file, delete | `admin/backups` |
| ✅ | Alerts: storage and certificate thresholds | `admin/alerts` |
| ✅ | General: server name, version, Portico Account connection, recovery owner | `admin/general` |
| ✅ | Owner-only gate for members | `admin-denied` |

## 19. States

| | Feature | Where in Cinema |
|---|---|---|
| ✅ | Loading skeletons (rows, grids, detail) | Every data screen while loading (`states` demo) |
| ✅ | Empty states | Per screen + `states` demo |
| ✅ | Errors with retry | Per screen + `states` demo |
| ✅ | Reconnecting banner | Global banner |
| ✅ | Toasts with undo | Global toast host |
