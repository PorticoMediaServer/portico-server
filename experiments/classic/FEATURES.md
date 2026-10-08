# Portico Classic: feature inventory

This inventory was taken from the web app (`apps/portico-server/web/src`), using its screens,
shell, player and app folders and the `web.*` string catalogue in `packages/i18n`. Each row maps a
web feature to where it lives in Classic. The **Screen id** column is the entry in the in-app
screen index (Settings › Screen index). Development builds also open it from
`portico-classic://screen/<id>`.

A tick means the feature is built against the fixture services and was walked on the iPhone and
Apple TV dev simulators. On TV that includes checking focus. The iPad layout was spot-checked on
the main surfaces. How each control feels on a real remote still needs checking by hand (see the
README).

## Getting in (web: `screens/auth`, `shell/ServerlessShell`, `app/device`)

| Feature | Web | Classic | Screen id | ✓ |
|---|---|---|---|---|
| Landing: Portico Account, code from another device, direct server, invitation, watch downloads | `Gate.tsx` | `screens/auth/Auth.tsx` Landing | `landing` | ✓ |
| Portico Account sign-in, with Apple and Google on phone and tablet | `SignIn.tsx`, `AccountForm.tsx` | AccountSignIn | `sign-in` | ✓ |
| Create account, with a password strength meter | `AccountForm.tsx` | AccountSignIn (create) | `create-account` | ✓ |
| Forgot password | `SignIn.tsx` | AccountSignIn (forgot) | `forgot-password` | ✓ |
| Two-step code or recovery code | `SignIn.tsx` (`web.twoStep`) | TwoStep | `two-step` | ✓ |
| Direct server sign-in, Quick Connect link, owner recovery sign-in | `DirectJoin.tsx` (`web.direct`) | DirectSignIn | `direct-sign-in` | ✓ |
| Hosted server chooser (online, shared, unreachable) | `HostedChooser.tsx` | ServerChooser | `server-chooser` | ✓ |
| Profile grid with PIN entry, kids badge and forgot PIN | `ProfileGrid.tsx`, `DirectProfiles.tsx` | Profiles | `profiles` | ✓ |
| Join by invitation: accept as account, username and password, or decline | `DirectJoin.tsx` (`web.join`, `web.directJoin`) | Join | `join` | ✓ |
| Activate this TV with a code and QR | `LinkRoutes.tsx` (`web.link`) | Activate | `activate` | ✓ |
| Quick Connect code for a direct server | `DirectJoin.tsx` | QuickConnect | `quick-connect` | ✓ |
| Link a TV or approve a device from a phone | `LinkRoutes.tsx` | Settings › Link a TV | `link-tv` | ✓ |
| Claim a server (connect to a Portico Account) | `ClaimApproval.tsx`, `ClaimConnectButton.tsx` (`web.claim`) | Claim | `claim` | ✓ |
| First-run server setup (name, libraries, remote access) | `ServerSetup.tsx` (`web.setup`) | Setup | `setup` | ✓ |
| Restore a saved connection, with a slow-connection note | `Restore.tsx` | Restore | `restore` | ✓ |
| Email verification result | `LinkRoutes.tsx` (`web.verify`) | VerifyEmail | `verify-email` | ✓ |
| Password reset with authenticator or recovery code | `LinkRoutes.tsx` (`web.reset`) | ResetPassword | `reset-password` | ✓ |
| Waiting for the owner to approve this device | `DeviceApproval.tsx` | DeviceApproval | `device-approval` | ✓ |
| Server unavailable: retry countdown, watch downloads, choose another | `shell/Connection.tsx` | ServerUnavailable | `server-unavailable` | ✓ |
| Offline, with downloads still playable | `ServerlessShell.tsx` | Offline | `offline` | ✓ |
| Finishing sign-in and returning to the app | `AccountReturn.tsx` | Not needed natively. Sign-in completes in the app. | n/a | ✓ |

## Shell (web: `shell/`)

| Feature | Web | Classic | Screen id | ✓ |
|---|---|---|---|---|
| Navigation: rail on TV, sidebar on iPad, header and tab bar on iPhone | `Shell.tsx` | `shell/Shell.tsx`, `Rail.tsx`, `PhoneChrome.tsx` | `home` | ✓ |
| Libraries in navigation, scanning indicator, no-libraries state | `Shell.tsx`, `shared/NoLibraries.tsx` | Rail, Libraries tab | `home` | ✓ |
| Profile menu: switch profile, switch server, account, settings, sign out | `Shell.tsx`, `ServerSwitcher.tsx` | `shell/ProfileMenu.tsx` | `home` (avatar) | ✓ |
| Server switcher showing online, shared and unreachable servers | `ServerSwitcher.tsx` | ProfileMenu › ServerSwitcher | `servers` | ✓ |
| Connection banners: reconnecting, offline, back online | `Connection.tsx` (`web.connection`) | `shell/ConnectionBanner.tsx` | `state-reconnecting`, `state-offline`, `state-online` | ✓ |
| Accept ownership of the server's Portico registration | `CustodyPrompt.tsx` | Manage server › General notice | `admin-general` | ✓ |
| Notifications badge and inbox | `app/inbox.tsx` | Rail and header bell › Notifications | `notifications` | ✓ |
| Watch Together pill while in a group | `player/GroupPill.tsx` | `player/GroupPill.tsx` | `together` | ✓ |
| Mini player docked above the tabs | `player/Player.tsx` | `player/MiniPlayer.tsx` | `now-playing` (minimise) | ✓ |
| Toasts with undo | `ui/Feedback.tsx` | `app/toast.tsx` (no undo on TV) | any action | ✓ |
| Kids profile: limited navigation and content | `app/session.tsx` | Session limits | `state-kids` | ✓ |

## Browse (web: `screens/home`, `screens/library`, `screens/search`)

| Feature | Web | Classic | Screen id | ✓ |
|---|---|---|---|---|
| Home hero with rotation, play, details, Watchlist | `Home.tsx` | `screens/home/Hero.tsx` | `home` | ✓ |
| Home rows: Continue Watching (remove, undo), Up Next, recently added, picks, recommendations | `Home.tsx` (`web.continue`) | `Home.tsx` | `home` | ✓ |
| Customise Home: show, hide and reorder rows | `Customise.tsx` | `CustomizeHome.tsx` | `home` › Customize Home | ✓ |
| Row "See all" page | `HomeRowPage.tsx` | `HomeRow.tsx` | `home-row` | ✓ |
| Library Discover | `Library.tsx` | `Library.tsx` Discover | `library-movies` | ✓ |
| Browse grid and list, sort, direction, quick chips, filters sheet, A–Z jump | `Library.tsx`, `BrowseFilters.tsx` (`web.filters`) | `Library.tsx` Browse | `library-browse` | ✓ |
| Save a filtered view | `SaveView.tsx` | Browse › Save view | `library-browse` | ✓ |
| Collections and categories | `Library.tsx` | Collections, Categories | `library-collections`, `library-categories` | ✓ |
| TV, anime, music (artists, albums, songs), audiobooks, photos, home videos libraries | `Library.tsx` | `Library.tsx` per kind | `library-tv`, `library-anime`, `library-music`, `library-songs`, `library-audiobooks`, `library-photos`, `library-homevideos` | ✓ |
| Genre, studio and collection pages with play all and shuffle | `Entity.tsx` (`web.entity`) | `Entity.tsx` | `entity-genre`, `entity-studio`, `entity-collection` | ✓ |
| Search across everything, with kind filters, recent searches and clear | `Search.tsx` (`web.search`) | `Search.tsx` (TV on-screen keyboard) | `search` | ✓ |

## Titles (web: `screens/detail`, `screens/people`, `screens/saved/Resource`)

| Feature | Web | Classic | Screen id | ✓ |
|---|---|---|---|---|
| Movie: play or resume, play from start, trailer, Watchlist, watched, favourite, more | `Detail.tsx` | `Movie.tsx` | `movie`, `movie-generated`, `movie-blender` | ✓ |
| Audio and subtitle choice before playing | `Detail.tsx` | Movie | `movie` | ✓ |
| Versions, extras, media information | `Detail.tsx` | Movie | `movie-generated` | ✓ |
| Cast and crew, more like this, part of a collection | `Detail.tsx` | Movie | `movie` | ✓ |
| Show: seasons, specials, episodes, next episode, shuffle, season options | `Show.tsx`, `EpisodePanel.tsx` | `Show.tsx` | `show` | ✓ |
| Episode page: previous, next, go to show | `EpisodeCard.tsx` | `Episode.tsx` | `episode` | ✓ |
| Person page: photo, biography, filmography | `Person.tsx` (`web.person`) | `Person.tsx` | `person` | ✓ |
| Artist and album: play, shuffle, favourite, add to playlist, track list with lyrics mark | `Detail.tsx` | `Music.tsx` | `artist`, `album` | ✓ |
| Audiobook: resume chapter, start over, chapter list | `Detail.tsx` | `Audiobook.tsx` | `audiobook` | ✓ |
| Photo album: grid, viewer, slideshow | `Detail.tsx` | `PhotoAlbum.tsx` | `photo-album` | ✓ |
| Home video | `Detail.tsx` | `Movie.tsx` (home video) | `home-video` | ✓ |
| Playlist, personal collection and saved view pages: reorder, remove, rename, share, delete | `Resource.tsx` (`web.saved`) | `Resource.tsx` | `playlist`, `collection`, `saved-view` | ✓ |

## Dialogs and actions (web: `screens/shared`, `app/`)

| Feature | Web | Classic | Screen id | ✓ |
|---|---|---|---|---|
| Action menu: play, play next, add to queue, watch with group, Watchlist, favourite, watched, remove from Continue Watching, not interested, like, dislike, rate, playlist, collection, download, select, details, edit, refresh, delete, report | `EntryActions.tsx`, `app/not-interested-notice.tsx` | `app/actions.tsx` | `actions` | ✓ |
| Multi-select bar: watched, Watchlist, favourites, playlist, collection, edit, refresh, delete | `SelectionBar.tsx`, `app/selection.tsx` (`web.selection`) | `app/selection.tsx` | `actions` › Select… | ✓ |
| Add to playlist or collection, or create one | `EntryActions.tsx` | `app/dialogs.tsx` PlaylistPicker | `actions` › Add to playlist… | ✓ |
| Rate | `EntryActions.tsx` | RatingSheet | `actions` › Rate… | ✓ |
| Download with quality choice | `app/downloads.tsx` | DownloadSheet (phone and tablet) | `actions` › Download… | ✓ |
| Delete media, with type-to-confirm | `app/delete-media-dialog.tsx` | DeleteMediaSheet | `actions` › Delete… | ✓ |
| Metadata editor: general, artwork, tags and genres, cast and crew, fix match, history with undo; per-field locks, lock all, bulk edit | `shared/MetadataEditor.tsx`, `app/metadata-editor.tsx` (`web.metadata`) | `app/MetadataEditor.tsx` | `metadata-editor` | ✓ |
| Report a problem (feedback) | `shared/FeedbackDialog.tsx` (`web.feedback`) | `app/FeedbackSheet.tsx` | `feedback` | ✓ |

## Playback (web: `player/`)

| Feature | Web | Classic | Screen id | ✓ |
|---|---|---|---|---|
| Video player: transport, skip back and forward, timeline with chapters and thumbnails | `Player.tsx`, `Timeline.tsx` | `VideoPlayer.tsx`, `Timeline.tsx` | `player` | ✓ |
| Audio and subtitles, quality, speed, subtitle offset | `options.ts` | `PlayerOptions.tsx` | `player` | ✓ |
| Skip intro, recap and credits | `SkipPrompt.tsx` | VideoPlayer | `player-episode` | ✓ |
| Up Next countdown, still watching, more like this | `PostPlay.tsx` | VideoPlayer PostPlay | `player-episode` | ✓ |
| Queue panel | `QueueList.tsx` | `QueuePanel.tsx` | `player-episode` | ✓ |
| Play on (AirPlay, Google Cast, Portico devices) with hand-over, stop and continue here | `PlayOn.tsx`, `app/cast.tsx` (`web.cast`) | `PlayOn.tsx` | `player` › Play on | ✓ |
| Live player: mini guide, channel up and down, record, live captions | `MiniGuide.tsx`, `LiveCaptions.tsx`, `live.ts` | VideoPlayer (live), `MiniGuide.tsx` | `player-live` | ✓ |
| Channel can't start (tuners busy and other reasons), try again, back to guide | `channel-problem.ts` (`web.channelProblem`) | VideoPlayer (live) | `player-channel-problem` | ✓ |
| Music now playing: lyrics that follow along, queue, shuffle, repeat, sleep timer | `Lyrics.tsx`, `QueueList.tsx` | `NowPlaying.tsx` | `now-playing` | ✓ |
| Audiobook player: chapters, speed, sleep timer, skip back and forward | `Player.tsx` | `NowPlaying.tsx` (book) | `audiobook-player` | ✓ |
| Owner stopped playback message | `owner-stop.ts` | Manage server › Now Playing › Stop | `admin-now-playing` | ✓ |

## Live TV and DVR (web: `screens/live`, `screens/channels`, `app/channels`)

| Feature | Web | Classic | Screen id | ✓ |
|---|---|---|---|---|
| Guide: 72 hours, now line, earlier and later, today and tomorrow, sources, groups, favourites | `screens/live` (`web.live`) | `Live.tsx`, `Guide.tsx` | `live-guide` | ✓ |
| Channels list with now and next, favourite and hide | `Channels.tsx` | Live › Channels | `live-channels` | ✓ |
| Program sheet: watch, record, record series, remind | `screens/live` | `ProgramSheet` | `live-guide` › program | ✓ |
| Recordings: upcoming, recorded, series rules, history, storage, conflicts | `DVR.tsx` (`web.dvr`, `web.recordingRules`) | `Recordings.tsx` | `live-recordings` | ✓ |

## You (web: `screens/saved`, `screens/settings`, `screens/account`, `screens/together`, `screens/downloads`)

| Feature | Web | Classic | Screen id | ✓ |
|---|---|---|---|---|
| Saved: Watchlist, favourites, playlists, collections, saved views, with sorting and filters | `Saved.tsx` | `Saved.tsx` | `saved` | ✓ |
| History by period, clear history | `Saved.tsx` | Saved › History | `saved-history` | ✓ |
| Watch Together: join with a code, start a group, who controls, queue, host hand-over, end for everyone | `Together.tsx`, `app/together.tsx` (`web.together`) | `Together.tsx` | `together` | ✓ |
| Downloads: progress, storage, keep policy, go to title (phone and tablet; explained on TV) | `Downloads.tsx` (`web.downloads`) | `Downloads.tsx` | `downloads` | ✓ |
| Settings home | `Settings.tsx` | `Settings.tsx` | `settings` | ✓ |
| Preferences (playback, subtitles, appearance, library view, languages), saved to profile or device | `Settings.tsx`, `app/server-preferences.tsx` (`web.prefs`) | `Preferences.tsx` | `preferences` | ✓ |
| Notifications: unread, all, archived, by source, mark all read | `settings/Notifications.tsx`, `account/Notifications.tsx` | Settings › Notifications | `notifications` | ✓ |
| Profiles: add, rename, colour, PIN, reorder, limits and restrictions | `settings/Profiles.tsx` (`web.profile`, `web.limits`) | Settings › Profiles | `profiles-settings` | ✓ |
| Reset recommendations | `settings/ResetRecommendations.tsx` | Settings › Preferences | `preferences` | ✓ |
| Security: password, two-step with recovery codes, sign out everywhere | `settings/Security.tsx` (`web.twoStep`) | Settings › Security | `security` | ✓ |
| Devices: this device, trust, rename, approve, sign out | `settings/Security.tsx` (`web.devices`, `web.device`) | Settings › Devices | `devices` | ✓ |
| Servers: switch, refresh, sign in directly, disconnect | `account/Servers.tsx` (`web.servers`) | Settings › Servers | `servers` | ✓ |
| About, licences and help | `Settings.tsx` (`web.about`) | Settings › About | `about` | ✓ |
| Portico Account: profiles, where you're signed in, sign out other devices, email, Apple and Google sign-in, servers and leave, invitations and paste a code, notifications, export, delete account | `account/*` (`web.hostedAccount`, `web.hostedSecurity`, `web.account`, `web.deleteAccount`) | `Account.tsx` | `account` | ✓ |

## Manage server (owner and administrators; web: `screens/server`)

| Feature | Web | Classic | Screen id | ✓ |
|---|---|---|---|---|
| Overview: activity, processor, memory, network, storage, attention items, watching now, charts, message everyone | `Overview.tsx`, `OverviewCharts.tsx` (`web.console`, `web.telemetry`, `web.broadcast`) | Admin › Overview | `admin-overview` | ✓ |
| Now Playing sessions, stop playback | `NowPlaying.tsx` | Admin › Now Playing | `admin-now-playing` | ✓ |
| Activity and background tasks | `Activity.tsx` | Admin › Activity | `admin-activity` | ✓ |
| Logs by level, detailed capture, support export | `Logs.tsx` (`web.logs`) | Admin › Logs | `admin-logs` | ✓ |
| Diagnostics: server capabilities and health checks | `Diagnostics.tsx` (`web.capabilities`) | Admin › Diagnostics | `admin-diagnostics` | ✓ |
| Viewer feedback: open, answered, closed; reply | `Feedback.tsx` | Admin › Viewer feedback | `admin-feedback` | ✓ |
| Alerts and thresholds | `AlertThresholds.tsx` (`web.alerts`) | Admin › Alerts | `admin-alerts` | ✓ |
| Libraries: add (with folder picker), scan, scan all files, attention review, folders (add, scan, remove) | `Libraries.tsx` (`web.libraries`, `web.folderPicker`) | Admin › Libraries, library detail | `admin-libraries`, `admin-library-detail` | ✓ |
| Library settings: metadata source and language, rescan interval, scan depth, missing files, scrubbing pictures, chapters, lyrics, deletion | `LibrarySettings.tsx` (`web.librarySettings`) | Admin › Library settings | `admin-library-settings` | ✓ |
| Metadata sources and lookups, including waiting for consent | `MetadataSource.tsx`, `MetadataLookup.tsx` | Admin › Metadata | `admin-metadata` | ✓ |
| Storage: media volumes, remote storage (WebDAV, rclone), mount and unmount | `Storage.tsx` (`web.storage`) | Admin › Storage | `admin-storage` | ✓ |
| Live TV sources: add tuner, playlist or guide; channels and numbers; defaults | `LiveTV.tsx`, `LiveSourceExtras.tsx` (`web.liveTv`, `web.liveSource`) | Admin › Live TV | `admin-live-tv` | ✓ |
| DVR settings: recording defaults, folders, who can record, space | `DVRSettings.tsx` (`web.dvrSettings`) | Admin › DVR | `admin-dvr` | ✓ |
| Library Channels and templates | `Channels.tsx` (`web.libraryChannels`) | Admin › Library Channels | `admin-channels` | ✓ |
| Channel Builder with preview | `ChannelBuilder.tsx` | Admin › Channel Builder | `admin-channel-builder` | ✓ |
| Transcoding: conversion, hardware acceleration, HDR, capacity | `Transcoding.tsx` (`web.transcoding`) | Admin › Transcoding | `admin-transcoding` | ✓ |
| Network: remote access, addresses, port mapping, outside rules, custom certificate, home networks, trusted proxies, interface | `Network.tsx` (`web.network`, `web.connectivity`) | Admin › Network | `admin-network` | ✓ |
| People: accounts, roles, profiles, disable | `People.tsx` (`web.people`) | Admin › People | `admin-people` | ✓ |
| Access: limits per person, invitations with link and code, device approval, API keys | `Access.tsx` (`web.access`) | Admin › Access & API keys | `admin-access` | ✓ |
| Scheduled tasks | `Schedules.tsx` (`web.schedules`) | Admin › Scheduled tasks | `admin-schedules` | ✓ |
| Maintenance: quiet hours, background priority, deleted titles (put back, remove what's due), permissions, updates | `Maintenance.tsx`, `MaintenanceWindows.tsx` (`web.maintenance`, `web.trash`) | Admin › Maintenance | `admin-maintenance` | ✓ |
| Backups: back up now, keep, restore, restore from a file | `Backups.tsx` | Admin › Backups | `admin-backups` | ✓ |
| General: name, version, Portico Account connection, registration custody | `General.tsx` (`web.general`) | Admin › General | `admin-general` | ✓ |
| State folder permissions | `StatePermissions.tsx` | Admin › Maintenance and Diagnostics | `admin-maintenance` | ✓ |

## States

| Feature | Web | Classic | Screen id | ✓ |
|---|---|---|---|---|
| Loading skeletons, empty states, errors with retry | `ui/Feedback.tsx`, `app/errors.tsx` (`web.empty`) | `ui/Controls.tsx`, `ui/Page.tsx` | throughout | ✓ |
| Not found | `NotFound.tsx` | `misc/NotFound.tsx` | any unknown route | ✓ |
| Developer screens | `screens/dev` | Settings › Screen index | `settings` | ✓ |
