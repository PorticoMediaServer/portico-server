# Portico Web Classic: feature checklist

The reference is the current web UI in `apps/portico-server/web/src`: its routes (`app/router.tsx`),
every folder under `screens/`, `player/`, `shell/`, `admin/`, and the string catalogue
(`packages/i18n/src/catalog/en-US.ts` and `en-US/web.ts`). Each row names the web source and where
this build covers it. **Screen** is the id in the screen index (`/dev/screens`), which opens that
page or state directly; every id is also screenshotted by `npm run shots`.

A tick means the feature is built on the fixture services, reachable, and was reviewed in headless
screenshots at 1440×900, 1920×1080, 1024×768 and 390×844.

## Routes and URLs

The URLs match the current web app, so links and bookmarks carry over.

| Current web route | Web Classic | ✓ |
|---|---|---|
| `/` Home | `screens/home/Home.tsx` | ✓ |
| `/home/rows/$rowId` a row's See all | `screens/home/HomeRow.tsx` | ✓ |
| `/library/$libraryId?view&sort&direction&filters` | `screens/library/Library.tsx` (view, sort, direction and filters in the URL) | ✓ |
| `/library/$libraryId/$view/$entityId` genre, studio, collection… | `screens/library/Entity.tsx` | ✓ |
| `/media/$itemId` | `screens/detail/Media.tsx` (movie, home video, album, artist, audiobook, photo album) | ✓ |
| `/show/$showId` and `/show/$showId/episode/$episodeId` | `screens/detail/Show.tsx` (episode opens as a panel over its show) | ✓ |
| `/person/$personId` | `screens/detail/Person.tsx` | ✓ |
| `/search` | `screens/search/Search.tsx` (query and kind in the URL, a deliberate change: results survive reload and Back) | ✓ |
| `/saved?view&sort&filter&period`, `/saved/$kind/$resourceId` | `screens/saved/Saved.tsx`, `Resource.tsx` | ✓ |
| `/channels`, `/channels/$entry?view&group&favorites`, `/live` redirect | `screens/live/Channels.tsx`, `Recordings.tsx`; `/live?view=dvr` redirects to recordings | ✓ |
| `/settings`, `/settings/$section` | `screens/settings/*` | ✓ |
| `/server` → `/server/overview`, `/server/$section`, `/server/$section/$id` | `screens/server/*` | ✓ |
| `/account?tab` | `screens/account/Account.tsx` | ✓ |
| `/together`, `/downloads` | `screens/together`, `screens/downloads` | ✓ |
| `/device`, `/claim`, `/reset`, `/verify-email`, `/join`, `/together/join`, `/app/account-return` | `screens/auth/Auth.tsx` | ✓ |
| Unknown paths | `screens/NotFound.tsx` (in the shell and outside it) | ✓ |
| `/gallery` developer pages | `/dev/screens` screen index | ✓ |

## Getting in (web: `screens/auth`, `shell/ServerlessShell`, `app/session`)

| Feature | Web | Web Classic | Screen | ✓ |
|---|---|---|---|---|
| Landing: Portico Account, code from another device, direct server, invitation, watch downloads | `Gate.tsx` | Landing | `welcome` | ✓ |
| Portico Account sign-in, Apple and Google | `SignIn.tsx`, `AccountForm.tsx` | AccountSignIn | `sign-in` | ✓ |
| Create an account with a strength meter | `AccountForm.tsx` | AccountSignIn (create) | `create-account` | ✓ |
| Forgot password | `SignIn.tsx` | AccountSignIn (forgot) | `forgot-password` | ✓ |
| Two-step code or recovery code | `web.twoStep` | TwoStep | `two-step` | ✓ |
| Hosted server chooser (yours, shared, unreachable) | `HostedChooser.tsx`, `web.chooser` | ChooseServer, `shell/Switchers.tsx` ServerList | `server-chooser` | ✓ |
| Profile grid with PIN, kids badge, forgot PIN | `ProfileGrid.tsx`, `DirectProfiles.tsx` | ProfileGrid | `profiles` | ✓ |
| Direct server sign-in, Quick Connect, owner recovery | `DirectJoin.tsx`, `web.direct` | DirectSignIn, QuickConnect, ResetPassword (owner) | `direct-sign-in`, `quick-connect`, `owner-recovery` | ✓ |
| Waiting for the owner to approve this device | `DeviceApproval.tsx`, `web.device` | WaitingForApproval | `waiting-approval` | ✓ |
| Sign in this browser with a code and QR from another device | `LinkRoutes.tsx`, `web.link` | Activate | `activate` | ✓ |
| Link a TV: enter its code, check, approve or deny | `LinkRoutes.tsx`, `/device` | LinkDevice | `link-tv` | ✓ |
| Join by invitation: with the account, with a username and password, decline, expired | `DirectJoin.tsx`, `web.join`, `web.directJoin` | Join | `join`, `join-expired` | ✓ |
| Claim a server to a Portico Account | `ClaimApproval.tsx`, `web.claim` | Claim | `claim` | ✓ |
| First-run server setup (name, owner, libraries, remote access) | `ServerSetup.tsx`, `web.setup` | Setup | `setup` | ✓ |
| Restore a saved connection, slow-connection note | `Restore.tsx`, `web.restore` | Restore | `restore` | ✓ |
| Email verification, confirmed and expired | `web.verify` | VerifyEmail | `verify-email`, `verify-expired` | ✓ |
| Password reset with an authenticator or recovery code | `web.reset` | ResetPassword | `reset-password` | ✓ |
| Finishing sign-in after Apple or Google | `AccountReturn.tsx`, `web.accountReturn` | AccountReturn | `account-return` | ✓ |
| Server unavailable: retry countdown, choose another, downloads | `shell/Connection.tsx` | Unavailable | `unavailable` | ✓ |
| Offline, downloads still play | `ServerlessShell.tsx`, `web.serverless` | Offline (plays in place) | `offline` | ✓ |
| Session kept across reloads (server and profile) | `app/session.tsx` | `app/session.tsx` resume | `home` | ✓ |

## Shell (web: `shell/`)

| Feature | Web | Web Classic | Screen | ✓ |
|---|---|---|---|---|
| Left rail with libraries, collapsible to icons (pushes the page on tablets) | `Shell.tsx` Rail | `shell/Shell.tsx` Sidebar; collapsed by default below 1280 px | `home` | ✓ |
| Phone: top bar and bottom tab bar, library picker | `Shell.tsx` TopBar, BottomBar | TopBar, BottomBar, Libraries sheet (with Live TV entries) | `home` at 390 | ✓ |
| Live TV entries in the rail: all channels, each source, Library Channels, Recordings | `ChannelsRail` | Sidebar Live TV group (`app/liveSources.ts`) | `guide` | ✓ |
| Library scanning count in the rail | `useSidebarScans` | Sidebar scan pill | `admin-libraries` › Scan | ✓ |
| No-libraries state (add a library for owners) | `shared/NoLibraries.tsx` | Home empty state | `home` (no libraries) | ✓ |
| Profile menu: who and where, switch profile, switch server, account, feedback, help, sign out | `ProfileMenu`, `ServerSwitcher.tsx` | ProfileMenu | `profile-menu`, `profile-switcher`, `server-switcher` | ✓ |
| Notifications badge (respects the badge preference) | `NotificationsLink`, `app/inbox` | Sidebar and top bar | `home` | ✓ |
| Watch Together indicator while in a group | `TogetherLink` | Sidebar group pill | `together-room` | ✓ |
| Connection banners: reconnecting, offline, back online | `Connection.tsx`, `web.connection` | `shell/ConnectionBanner.tsx` | `state-reconnecting`, `state-offline` | ✓ |
| Registration custody prompt | `CustodyPrompt.tsx`, `web.custody` | Manage server › General notice | `admin-general` | ✓ |
| "/" opens Search from anywhere | `useSearchShortcut` | `useGlobalShortcuts` plus g-then-letter jumps | `shortcuts` | ✓ |
| Toasts with Undo | `ui/Feedback.tsx` | `ui/Overlay.tsx` ToastProvider | any action | ✓ |
| Kids profile: limited navigation and content | `app/session.tsx`, limits | Session limits hide Live TV, DVR, Together and downloads as set | `state-kids` | ✓ |
| Skip link, keyboard shortcuts legend | — (new) | Skip to content; `?` opens the legend | `shortcuts` | ✓ |

## Home and libraries (web: `screens/home`, `screens/library`, `screens/shared/Sections`)

| Feature | Web | Web Classic | Screen | ✓ |
|---|---|---|---|---|
| Featured hero with rotation, Play, More info, Watchlist | `Home.tsx` | Hero (pauses on hover and focus; glow follows the title) | `home` | ✓ |
| Rows: Continue Watching (remove with Undo), Up Next, recently added, Picks for you, Because you watched, Trending, On now, saved views | `Home.tsx`, `web.continue`, `home.row.*` | HomeShelf | `home` | ✓ |
| Customize Home: show, hide, reorder (drag or buttons), reset | `Customise.tsx`, `home.customize.*` | `Customize.tsx` | `customize-home` | ✓ |
| Row See all | `HomeRowPage.tsx` | HomeRow | `home-row` | ✓ |
| Discover, Browse, Collections, Categories pivots | `Library.tsx` | Library views per kind | `library-movies`, `library-collections`, `library-categories` | ✓ |
| Browse: sort and direction, quick filters, filter panel, grid or list, A–Z jump | `Library.tsx`, `BrowseFilters.tsx`, `web.filters`, `filter.*` | Browse, FilterPanel, AlphaRail, ListView | `library-browse`, `library-filtered`, `library-filter-panel`, `library-list` | ✓ |
| Nothing matches, clear filters | `web.empty` | Browse empty state | `library-empty-filter` | ✓ |
| Save a filtered view | `SaveView.tsx` | SaveViewDialog | `library-browse` › More | ✓ |
| Virtualised grids, paged fetches (O(visible) for huge libraries) | `WindowedGrid.tsx`, `browse-page-source.ts` | `ui/Windowed.tsx` WindowedGrid and WindowedList | `library-browse` | ✓ |
| TV, anime, music (artists, albums, songs, genres), audiobooks (books, authors, series), photos (albums, all photos), home videos | `Library.tsx` (`releases`, `songs`, `authors`, `series`) | `viewsFor()` | `library-tv` … `library-homevideos` | ✓ |
| Genre, studio, decade, network, collection, author and series pages with Play all and Shuffle | `Entity.tsx`, `web.entity` | Entity | `entity-genre`, `entity-studio`, `entity-collection`, `entity-author` | ✓ |
| Loading skeletons for rows and grids | `LoadingShelves`, `LoadingGrid` | CardSkeleton, Shelf and CardGrid loading | any, with `?latency=4` | ✓ |

## Titles (web: `screens/detail`, `screens/people`, `screens/saved/Resource`)

| Feature | Web | Web Classic | Screen | ✓ |
|---|---|---|---|---|
| Movie: play or resume, from the start, trailer, Watchlist, watched, favorite, rate, more | `Detail.tsx`, `title.*` | `Movie.tsx`, ActionRow | `movie` | ✓ |
| Versions and editions, audio and subtitle choice before playing | `Detail.tsx` | Movie selectors | `movie-versions` | ✓ |
| Media information | `title.mediaInformation` | MediaInfoDialog | `media-info` | ✓ |
| Cast and crew, extras, more like this, part of a collection | `Detail.tsx` | CastRow, Shelves, eyebrow link | `movie-blender` | ✓ |
| Show: next episode, seasons and Specials, episodes with progress, season options (play, shuffle, mark watched, download) | `Show.tsx`, `title.season*` | `Show.tsx` | `show`, `show-anime` | ✓ |
| Episode panel over the show: previous, next, go to show | `EpisodePanel.tsx`, `EpisodeCard.tsx` | EpisodePanel | `episode` | ✓ |
| Person: photo, life dates, biography, known for, filmography (all, acting, crew; sort) | `Person.tsx`, `web.person` | `Person.tsx` | `person` | ✓ |
| Artist: popular songs, albums, similar artists | `Detail.tsx` | ArtistDetail | `artist` | ✓ |
| Album: play, shuffle, track list with lyrics mark, discs, more by artist | `Detail.tsx`, `title.disc*` | AlbumDetail, TrackList | `album` | ✓ |
| Audiobook: resume chapter, start over, chapter list | `Detail.tsx`, `title.resumeChapter` | AudiobookDetail | `audiobook` | ✓ |
| Photo album: grid, viewer, slideshow, photo info | `Detail.tsx` | PhotoAlbumDetail, PhotoViewer | `photo-album`, `photo-viewer` | ✓ |
| Home video | `Detail.tsx` | MovieDetail (home) | `home-video` | ✓ |
| Playlist, personal collection, saved view: play, reorder (drag or buttons), remove with Undo, rename and describe, share, copy link, delete | `Resource.tsx`, `web.saved` | `Resource.tsx` | `playlist`, `collection`, `saved-view` | ✓ |
| Title not available | `title.unavailableItem` | Media fallback | `state-unavailable-title` | ✓ |

## Dialogs and actions (web: `screens/shared`, `app/`)

| Feature | Web | Web Classic | Screen | ✓ |
|---|---|---|---|---|
| Actions menu: play, from start, shuffle, play next, add to queue, watch with a group, details, go to show or album, watched, Watchlist, favorite, rate, remove from Continue Watching, Not interested, playlist, collection, download, copy link, select, edit, fix match, refresh, delete, report | `EntryActions.tsx`, `not-interested-notice.tsx` | `app/actions.tsx` (card ⋯, right-click, long press on touch, Shift+F10 or the context-menu key) | `actions-menu` | ✓ |
| Multi-select bar: watched, unwatched, Watchlist, favorites, playlist, collection, edit, refresh, delete | `SelectionBar.tsx`, `web.selection` | `shell/SelectionBar.tsx` (Shift-click or X to select) | `selection` | ✓ |
| Add to playlist or collection, or create one with the items | `PlaylistPicker`, `CollectionPicker` | `dialogs/PlaylistPicker.tsx` | `add-to-playlist`, `add-to-collection` | ✓ |
| Rate, with like or dislike | `RatingDialog` | `dialogs/RateDialog.tsx` | `rate` | ✓ |
| Download with quality; shows as a standing request | `app/downloads.tsx`, `downloads.quality.*` | `dialogs/DownloadDialog.tsx` | `download` | ✓ |
| Delete media, type to confirm, files to deleted titles | `delete-media-dialog.tsx` | `dialogs/DeleteMediaDialog.tsx` | `delete-media` | ✓ |
| Metadata editor: general with per-field locks and lock all, artwork, tags and genres, cast and crew, fix match, history with Undo; bulk edit | `MetadataEditor.tsx`, `web.metadata` | `dialogs/MetadataEditor.tsx` | `metadata-editor`, `fix-match` | ✓ |
| Report a problem or send an idea, with diagnostics | `FeedbackDialog.tsx`, `web.feedback` | `dialogs/FeedbackDialog.tsx` | `feedback` | ✓ |

## Playback (web: `player/`)

| Feature | Web | Web Classic | Screen | ✓ |
|---|---|---|---|---|
| Video player: play, pause, skip back and forward, volume, full screen, auto-hiding chrome | `Player.tsx`, `engine.tsx` | `player/VideoPlayer.tsx` (simulated picture, real clock) | `player-movie` | ✓ |
| Timeline with chapter ticks, intro and credits segments, scrubbing preview | `Timeline.tsx`, `trickplay` | `player/Timeline.tsx` | `player-movie` | ✓ |
| Audio, subtitles (size, background, timing offset), quality, speed | `options.ts`, `player.opt.*` | OptionsPanel | `player-options` | ✓ |
| Skip intro, recap and credits (manual or automatic by preference) | `SkipPrompt.tsx` | VideoPlayer skip prompts | `player-intro` | ✓ |
| Up Next countdown, Still watching, finished with more like this | `PostPlay.tsx`, `player.post.*` | UpNext, still watching, finished screen | `player-upnext`, `player-finished` | ✓ |
| Queue panel with reorder and remove | `QueueList.tsx`, `web.queue` | QueuePanel | `player-queue` | ✓ |
| Play on: Portico devices, Google Cast, AirPlay; hand-over, stop, continue here | `PlayOn.tsx`, `app/cast.tsx`, `web.cast`, `playOn.*` | PlayOnDialog | `player-playon` | ✓ |
| Live player: channel up and down, record, live captions, mini guide | `live.ts`, `MiniGuide.tsx`, `LiveCaptions.tsx` | VideoPlayer (live), MiniGuide | `player-live`, `player-miniguide` | ✓ |
| Channel can't start (tuners busy and other reasons), try again, back to the guide | `channel-problem`, `web.channelProblem` | VideoPlayer problem screen | `player-channel-problem` | ✓ |
| Music: mini player, Now Playing with lyrics that follow along, queue, shuffle, repeat, sleep timer | `Player.tsx` MiniBar, `Lyrics.tsx` | `player/AudioPlayer.tsx` | `now-playing`, `mini-player` | ✓ |
| Audiobook player: chapters, speed, sleep timer, skip 15 and 30 | `Player.tsx` | NowPlaying (book) | `audiobook-player` | ✓ |
| Group pill while watching together | `GroupPill.tsx` | GroupPill in the player | `together-room`, then play | ✓ |
| Keyboard: Space/K, J/L and arrows, M, F, C, Shift+N, G, R, Page Up/Down, Esc, `?` legend | `Player.tsx` key handler, `ShortcutLegend` | VideoPlayer key handler, Shortcuts dialog | `shortcuts` | ✓ |
| Progress saved; watched at the end | `engine.tsx` | PlayerProvider saves every 15 s and on close | any | ✓ |
| Owner stopped playback | `owner-stop.ts` | Manage server › Now Playing › Stop | `admin-now-playing` | ✓ |

## Live TV and DVR (web: `screens/channels`, `screens/live`, `app/channels`)

| Feature | Web | Web Classic | Screen | ✓ |
|---|---|---|---|---|
| Guide grid: 3 days, now line, earlier, now and later, days, groups, favorites, sticky channel column, arrow-key navigation | `Channels.tsx` GuideGrid, `web.live`, `guide.*` | GuideGrid | `guide` | ✓ |
| Channel list with now and next and progress | `ChannelList` | ChannelList | `channel-list` | ✓ |
| Sources: all channels, each tuner or playlist, Library Channels | `channelEntries` | `/channels/:source` | `guide-source`, `library-channels` | ✓ |
| Favorite and hide channels | `saveChannelPreference` | ChannelCell menu, list star | `guide` | ✓ |
| Program sheet: watch, record, record series (channel, new only), remind | `ProgramSheet` | ProgramSheet, SeriesDialog | `program-sheet` | ✓ |
| Recordings: upcoming, recorded, series rules (edit, pause, priority, delete), conflicts, history, space | `DVR.tsx`, `web.dvr`, `web.recordingRules`, `dvr.*` | `Recordings.tsx` | `recordings` … `recordings-space` | ✓ |
| Reminders while Portico is open | `ChannelReminders` | ProgramSheet Remind me (toast confirms) | `program-sheet` | ✓ |

## You (web: `screens/search`, `saved`, `together`, `downloads`, `settings`, `account`)

| Feature | Web | Web Classic | Screen | ✓ |
|---|---|---|---|---|
| Search everything: grouped results, kinds, See all, recent searches and clear, browse tiles, nothing found | `Search.tsx`, `web.search` | `Search.tsx` | `search-start`, `search`, `search-kind`, `search-none` | ✓ |
| Saved: Watchlist and Favorites (filter, sort), playlists, collections (new), saved views, history by period with clear | `Saved.tsx`, `saved.*` | `Saved.tsx` | `saved` … `saved-history` | ✓ |
| Watch Together: start (who controls), join with a code, groups on the server, room with code and link, new code, members, make host, queue, suggestions, leave, end for everyone | `Together.tsx`, `app/together.tsx`, `web.together` | `Together.tsx` | `together`, `together-room` | ✓ |
| Downloads: progress, pause, retry, remove, storage, show requests, quality, space limit, remove after watching, next episode | `Downloads.tsx`, `web.downloads` | `Downloads.tsx` | `downloads` | ✓ |
| Settings hub | `Settings.tsx` | `Settings.tsx` (section list beside the section) | `settings` | ✓ |
| Preferences from the server registry (playback, subtitles, languages, quality, streaming, music, privacy, search, region), profile or browser scope | `Settings.tsx`, `server-preferences.tsx`, `web.prefs` | `Preferences.tsx` | `settings-preferences` | ✓ |
| Appearance: card size, backdrops, reduce motion, featured billboard | `preferences.ts` | Preferences › Appearance | `settings-preferences` | ✓ |
| Reset recommendations | `ResetRecommendations.tsx` | Preferences › Recommendations | `settings-preferences` | ✓ |
| Profile: name, colour, PIN (add, change, remove) | `settings/Profiles.tsx`, `web.profile` | AccountSection | `settings-account` | ✓ |
| Profiles: add, edit, kids, limits (rating, unrated, downloads, Live TV, recording, Together), reorder, delete | `Profiles.tsx`, `web.limits`, `limits.*` | ProfilesSection, ProfileEditor | `settings-profiles`, `settings-profile-editor` | ✓ |
| Security: password, two-step with recovery codes, sign out everywhere | `settings/Security.tsx`, `web.twoStep` | SecuritySection | `settings-security` | ✓ |
| Devices: this browser, trust, rename, approve, sign out | `Security.tsx` DevicesSection, `web.devices` | DevicesSection | `settings-devices` | ✓ |
| Notifications: unread, all, archived, for me or server alerts, mark all read, archive, links to where to act | `settings/Notifications.tsx`, `notifications.*` | NotificationsSection | `settings-notifications` | ✓ |
| Servers: switch, refresh, sign in directly, leave, add | `account/Servers.tsx`, `web.servers` | ServersSection | `settings-servers` | ✓ |
| About, licenses, help | `web.about` | AboutSection | `settings-about` | ✓ |
| Link a TV from Settings | `settings.linkTV` | Settings hub › Link a TV | `link-tv` | ✓ |
| Portico Account: where you're signed in, email, security, Apple and Google, servers and leave, invitations and paste a code, notifications, export, delete account | `account/*`, `web.hostedAccount`, `web.hostedSecurity`, `web.account`, `web.deleteAccount` | `screens/account/Account.tsx` | `account` … `account-data` | ✓ |

## Manage server (owners and administrators; web: `screens/server`, `admin/`)

Every section is a page of typed blocks from `AdminApi` (`src/data/adminApi.ts`), rendered by
`screens/server/Blocks.tsx`: stats with sparklines, notices, settings that save as they change,
facts with copy, lists with actions, filters and details, tables, charts, meters and timelines.
Actions open a form (with a folder picker where needed) or a confirmation, including type-to-confirm.

| Feature | Web | Screen | ✓ |
|---|---|---|---|
| Overview: watching now, processor, memory, network, storage, needs attention, charts, recent playback, message everyone | `Overview.tsx`, `OverviewCharts.tsx`, `web.console`, `web.telemetry`, `web.broadcast` | `admin-overview` | ✓ |
| Now Playing: sessions with details, stop with a message | `NowPlaying.tsx`, `web.nowPlaying` | `admin-now-playing` | ✓ |
| Activity and background tasks | `Activity.tsx`, `web.activity` | `admin-activity` | ✓ |
| Logs by level, temporary capture, support export, retention | `Logs.tsx`, `web.logs` | `admin-logs` | ✓ |
| Diagnostics: capabilities and health checks | `Diagnostics.tsx`, `web.capabilities` | `admin-diagnostics` | ✓ |
| Viewer feedback: open, answered, closed; reply | `Feedback.tsx` | `admin-feedback` | ✓ |
| Alerts and thresholds | `web.alerts` | `admin-alerts` | ✓ |
| Libraries: add (with folder picker), scan, scan all files, needs attention, folders (add, scan, remove), rename, delete | `Libraries.tsx`, `web.libraries`, `web.folderPicker` | `admin-libraries`, `admin-library-detail`, `admin-add-library`, `admin-folder-picker` | ✓ |
| Library settings: metadata source and language, scanning, missing files, scrubbing pictures, chapters, lyrics, deletion | `LibrarySettings.tsx`, `web.librarySettings` | `admin-library-settings` | ✓ |
| Metadata sources and lookups, waiting for consent | `MetadataSource.tsx`, `web.metadataSource`, `web.metadataLookup` | `admin-metadata` | ✓ |
| Storage: remote storage (WebDAV, rclone), mount and unmount, volumes | `Storage.tsx`, `web.storage` | `admin-storage` | ✓ |
| Live TV sources: tuners, playlists, guide data, channel numbers, defaults | `LiveTV.tsx`, `LiveSourceExtras.tsx`, `web.liveTv`, `web.liveSource` | `admin-live-tv` | ✓ |
| DVR settings: defaults, folders, who can record, space | `DVRSettings.tsx`, `web.dvrSettings` | `admin-dvr` | ✓ |
| Library Channels and templates | `Channels.tsx`, `web.libraryChannels` | `admin-channels` | ✓ |
| Channel Builder with a schedule preview | `ChannelBuilder.tsx`, `web.channelBuilder` | `admin-channel-builder` | ✓ |
| Transcoding: conversion, hardware acceleration, HDR, capacity | `Transcoding.tsx`, `web.transcoding` | `admin-transcoding` | ✓ |
| Network: remote access, addresses, port mapping, certificate, home networks, trusted proxies | `Network.tsx`, `web.network`, `web.connectivity`, `server.cert.*` | `admin-network` | ✓ |
| People: accounts, roles, profiles, disable | `People.tsx`, `web.people` | `admin-people` | ✓ |
| Access: per-person limits, invitations with link and code, device approval, API keys | `Access.tsx`, `web.access` | `admin-access` | ✓ |
| Scheduled tasks | `Schedules.tsx`, `web.schedules` | `admin-schedules` | ✓ |
| Maintenance: quiet hours, background priority, deleted titles (put back, remove), state folder permissions, updates | `Maintenance.tsx`, `MaintenanceWindows.tsx`, `StatePermissions.tsx`, `web.maintenance`, `web.trash` | `admin-maintenance` | ✓ |
| Backups: back up now, keep, restore, restore from a file | `Backups.tsx` | `admin-backups` | ✓ |
| General: name, version, Portico Account connection, registration custody | `General.tsx`, `web.general`, `web.custody` | `admin-general` | ✓ |
| Only owners and administrators | `ServerFrame` owner check | Server page guard | `state-kids` › Manage server | ✓ |

## States and quality

| Feature | Web | Web Classic | Screen | ✓ |
|---|---|---|---|---|
| Loading skeletons for every page kind | `ui/Feedback.tsx` | `PageLoading`, `DetailLoading`, skeleton cards and rows | `?latency=4` | ✓ |
| Empty states with a next step | `web.empty`, `saved.empty.*` | `EmptyState` everywhere | e.g. `search-none` | ✓ |
| Errors with Try again | `app/errors.tsx` | `ErrorState` on every query | — | ✓ |
| Not found, inside and outside the shell | `NotFound.tsx` | `NotFound.tsx` | `state-not-found` | ✓ |
| Keyboard: visible focus, roving focus in rows and grids, focus trapped in dialogs and returned | — | `ui/roving.ts`, Radix dialogs | all | ✓ |
| Reduced motion (system and preference) | `data-reduce-motion` | `base.css`, Preferences | — | ✓ |
| Toggleable simulated latency | — | `?latency=0`, `?latency=4` | any | ✓ |
