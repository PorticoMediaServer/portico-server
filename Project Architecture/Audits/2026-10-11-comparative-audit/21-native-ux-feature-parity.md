# 21 — Native app user experience and feature parity

## Scope and sources

Primary evidence uses `N = apps/portico-react-native/apps/native`, `V = apps/portico-react-native/apps/vega`, `C = apps/portico-server/packages/client-core/src`. `JTV = jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv`, `W = wholphin/app/src/main/java/com/github/damontecres/wholphin`. These abbreviations are path prefixes relative to `/home/justin/projects/Portico`; a citation such as `N/src/screens/Settings.tsx:27` means that exact source file. Comparison citations start with the checkout name. References to normative contracts are relative to `portico-internal/Product Contracts`.

The main source paths inspected were the native route registration, shell/rail/bottom bar, Home and customization, library/browse/filter windows, Search, Saved/history/resources, detail/show/episode/container presentation, downloads, channels/guide/programme/DVR, profile/security/server selection, invitation/membership, settings structures and native panels, metadata editor, player controls/options/destinations, native activity/search/shelf integrations, and Vega entry/shims/module adapters. The supplied architecture, contracts, Backlog and port logs were reviewed for deliberate decisions and dated limitations. Comparison inspection prioritizes Plezy's complete 2,339-line English catalogue, settings pages, platform detector, context menu, library/detail/download/profile/main screen paths; then Swiftfin's shared settings/item views and iOS administration; Android TV's preference definitions and Compose settings screens; Wholphin preferences/home settings/discover; Streamyfin settings model and UI consumers.

Inventory terms: **implemented** means a source path exists and is wired where stated; **not found** means the inspected source did not establish it, not a universal assertion about the product. **Conditional** means permission/server/device/backend gating applies. A component filename alone is not feature evidence. This report avoids crediting obsolete `NOTES-*.md` statements as current code.

Public pages were fetched as text through the shell, without a browser or computer-use tools. Infuse, Plex and Apple references are public product/documentation claims, not source audits. No private source was sent in these requests. Plezy is the first comparison in the inventories because it is the named client benchmark; the explanatory overview follows the brief's Jellyfin-first order.

Read-only source review dated 11 October 2026. Covers phone, tablet and TV navigation, screens, actions, preferences, native integrations, and player controls/menus; excludes player-engine internals. No builds, tests, devices, browser, MCP, or Portico network endpoints were used. Evidence below describes source behavior, not reproduced runtime behavior. Comparison checkouts are read as untrusted data.

Context refresh was attempted as instructed but failed because `/home/justin/.local/state/justin-agent-context/sync.lock` is outside the writable sandbox. Existing project and verification guidance was read. Execution host reports `ehler-homelab-mini-1`.

## How Portico does it today

Portico has one shared React Native consumer app with platform modules under matching names. iPhone/iPad, Android phone/tablet, tvOS, Android TV and Fire TV render the same screen code. Vega imports that same `App` and adapts React Native and selected client-core files through Metro overrides (`V/src/Root.tsx:1`, `V/src/shims/react-native.ts:1`). This is source reuse, not proof of equal device behavior.

The server composes content rows, sort/filter capabilities, entity projections and playback choices; shared client-core authors the visible presentation grammar. `C/presentation/settings-structure.ts:1` explicitly separates the server's preference vocabulary from authored page layout. A server field without an authored row does not automatically become a setting. `C/preferences.ts:1` validates the registry and effective values and tolerates unknown preference types. Native `ServerPreferencesProvider` forwards revisioned patches according to each field's scope (`N/src/app/server-preferences.tsx:81`). Settings therefore have three distinct storage patterns: profile/server, profile/device-class on the server, and genuinely installation-local values in `N/src/app/preferences.ts:7`.

On a phone the persistent bottom bar offers Home, Library, Saved, conditional Channels and conditional Downloads. The Library tab opens the last selected authorized library; that library has a title dropdown. Wide tablets use the same leading rail as TV, while narrower tablet windows revert to tabs (`N/src/shell/Shell.tsx:128`). The product uses a custom roots navigator with kept-alive stacks; roots mount when selected and remain hidden when inactive (`N/src/shell/Roots.tsx:15`). Selecting a different root restores its prior stack; selecting the current root pops to its top. All libraries share one `LibraryRoot`, rather than one root stack per library (`Shell.tsx:103`).

Signed-out TV begins with an activation QR/code; handheld begins with account/password/provider sign-in. Direct server sign-in remains independent of Hosted. Direct sign-in handles server address, password or Quick Connect, two-step challenge, required password change, pending device approval and profile selection (`N/src/screens/auth/AuthFlow.tsx:92`, `DirectSignIn.tsx:98`). Hosted account login can precede server selection; the landing screen supports zero/one/multiple-server outcomes. Local profiles have avatar/PIN/limits management, with typing-heavy TV management handed to web (`N/src/screens/settings/SettingsPage.tsx:215`). Known direct servers remember address/name and require renewed sign-in when selected (`SettingsPages.tsx:92`). This is multiple-server switching, not a merged media universe.

Home reads server rows and a relevant hero, supports row reorder/hide/reset and adding Saved Views, and uses exact layout revision on save (`N/src/screens/HomeCustomise.tsx:21`). Recommendation cards have Not interested and Undo; continue-watching cards have removal and Undo. Libraries have kind-specific authored tabs, Discover, server capabilities, quick/promoted/advanced filters, sort direction, alphabet/jump and saved-view creation on handheld (`Library.tsx:116`, `BrowseFilters.tsx:92`). Ordinary video/book browsing is poster-first; music is square-first. Show detail owns a single season/episode workspace, including inherited episode deep-link selection, and uses thumbnails there. The episode layout is distinct on TV and handheld (`Show.tsx:324`, `:387`).

Detail uses a shared hero, facts/ratings/synopsis/credits, personal state and More. Pre-play audio/subtitle choices are implemented, but version/quality are not exposed in that component (`N/src/screens/title/PlaybackChoices.tsx:12`). The currently authored shared action model still puts rate and playlist organization in More (`C/presentation/detail-actions.ts:1`), despite the newer contract's immediate-action requirement. Media information is a deliberately opened sheet, keeping technical data away from routine cards (`TitleSheets.tsx:47`).

Saved provides My List, Favorites, Playlists, Collections, Saved Views and History (`N/src/screens/Saved.tsx:26`). My List/favorites use a windowed grid; history and resource lists append pages into ordinary views. Playlist detail supports occurrence-aware remove/reorder menus, but its play anchor loses occurrence identity. Downloads show local protected files separately from server preparation and batch requests, include quality/storage/Wi-Fi rules, and remain available from the signed-out/offline flow (`Downloads.tsx:23`, `AuthFlow.tsx:105`). Television deliberately offers no downloads.

Channels combines Live TV, library channels and recordings. The guide changes layout by width/input, has a programme sheet with Watch/Record/Series/Remind/Favorite, and separate DVR screens. Owner Settings contains Dashboard, play history, libraries, Live/DVR, people/API keys, streaming, remote access, storage/backups, schedules/jobs, troubleshooting and general settings; TV deliberately resolves only Dashboard (`C/presentation/settings-structure.ts:293`). These are native forms/panels, not an embedded web console.

Player chrome stays above the browsing shell under one persistent player host. Audio uses a small handheld mini bar or a TV now-playing pill; video is full-screen on TV, with handheld native PiP. Controls include timeline, applicable previous/next/skip/shuffle/repeat, audio/subtitle/quality/speed/sleep, queue/lyrics/chapter utilities, playback information, Watch Together and reporting. Siri Remote touch scrubbing is a native module; Vega converts remote key names and handles focus/keyboard differences (`VideoPlayer.tsx:285`, `V/src/shims/react-native.ts:100`). The present audit evaluates those controls and gates, not delivery or decoder correctness.

## How the others do it

Jellyfin's official native clients are separate products with separate UI and settings laws. Swiftfin is SwiftUI on Apple; jellyfin-androidtv is a Kotlin/Compose plus existing browsing/detail/player stack on Android/Fire TV. Their settings mix server user configuration with local client preferences. They consume Jellyfin item DTOs and API query choices, compose Home and detail locally, and have many device tuning options. Swiftfin's native iOS admin dashboard covers users, devices, active sessions, tasks/triggers, logs, API keys, activity and backups (`swiftfin/Swiftfin/Views/AdminDashboardView/AdminDashboardView.swift:1`). It is gated to iOS in `Shared/Views/SettingsView/SettingsView.swift:75`; it is not a tvOS admin suite. Android TV gives ten Home section slots, local library display preferences, channel/guide preferences, media segment actions and a rating-filtered screensaver. Its OS-facing TV provider is Android channel integration, not television-provider single sign-on.

Plezy is a Flutter application with explicit phone/TV/pointer adaptations and a backend-neutral media layer for Plex, Jellyfin and Emby. It is more than a player: profiles own connections, multiple servers supply Home/search/libraries, trackers and Seerr extend discovery, and adapters gate backend-specific metadata/actions (`plezy/lib/widgets/media_context_menu.dart:71`). Local Plezy profiles are distinct from Plex Home identities. Connections may be added or borrowed from another profile with PIN checking. One account preference screen edits the upstream account itself, separately from local app settings (`plezy/lib/i18n/en.i18n.json:954`). This means local UX settings, upstream state, and third-party service state are deliberately separate authorities.

Plezy's appearance, navigation and playback behavior are highly configurable. The current source establishes System/Light/Dark/OLED, density/spacing/grid-list/episode-art, spoiler masking, Home-hub versus library recommendations, source naming/grouping, card indicators and TV focus/backdrop modes. Its player UI settings include gestures, shortcut editing, track memory/scope, subtitle styling, skip behavior, performance overlay, external players and tuning controls. Download UI supports sync rules, keep-N-unwatched, batch operations and Android background-work diagnostics. Metadata editing, matching and library maintenance depend on backend/permissions. Top Shelf and Watch Next are implemented in native bridges. Do not infer a Chromecast sender, a configurable accent, or a dedicated screensaver from the task's starting-point list: those were not established in the inspected Plezy settings/source searches.

Wholphin focuses on Android TV with a Compose UI, configurable Home rows, combined continue-watching/next-up patterns, Seerr discovery/requests, playlists, trailers, theme songs and play-with choices. It stores app preferences in a generated protobuf/DataStore model while retaining server user configuration (`W/preferences/UserPreferences.kt:9`). Streamyfin is the closest React Native technology comparison, but has its own Expo routes, extensive MMKV/Jotai settings, gestures, downloads, Chromecast, remote sessions, plugin integration and phone/TV variants. Its optional Streamyfin plugin can seed and lock client settings (`streamyfin/utils/atoms/settings.ts:558`), a different model from Portico's authored first-party contract.

[Infuse's public feature page](https://firecore.com/infuse) describes direct SMB/NFS/WebDAV/cloud access, library filters and custom containers, Trakt, iCloud settings/connections/progress sync and downloads/transfers. It can operate without a media server, so its file-browser UX is an architectural alternative, not a drop-in Portico feature. [Plex's big-screen customization documentation](https://support.plex.tv/articles/customizing-the-apps/) documents installation-local source pinning and Home customization across Apple TV/Android TV/Fire TV. This is a useful personalization benchmark; it is not evidence that every modern Plex mobile screen is identical.

Apple's [tvOS design guidance](https://developer.apple.com/design/human-interface-guidelines/designing-for-tvos) emphasizes native focus, cinematic legible artwork, easy/infrequent multiuser sign-in and Siri Remote interactions. It also recognizes TV PiP. Its [iPad guidance](https://developer.apple.com/design/human-interface-guidelines/designing-for-ipados) calls for large-screen composition, mixed touch/keyboard/pointer input and adaptation to multitasking/orientation. Apple TV app and Netflix-style hero/Continue Watching/detail conventions are qualitative references only here; their closed implementations were not inspected.

## Screen inventory

The tables below are a destination inventory, not a claim of tested visual equality. Portico target labels: **I** iPhone/iPad; **A** Android handheld; **T** tvOS; **G** Google Android TV; **F** Fire TV; **V** Vega. A dash means the app/form factor does not apply; “unestablished” means no source-backed parity was established. Screen families combine closely related modal/subscreen actions; the source-index appendix names the actual files.

### Phone

| Screen/workflow | Portico platforms and implementation | Plezy | Swiftfin | jellyfin-androidtv | Notes |
|---|---|---|---|---|---|
| Account/sign-in/create/recovery | I/A: Hosted provider/password, direct server password/Quick Connect; account help web handoff | Plex QR/browser; Jellyfin/Emby credentials/Quick Connect | Server connection + user login/Quick Connect | — (TV-only) | Portico account services and direct server are separate authorities. |
| Server/address/discovery/chooser | I/A: Address + remembered direct servers; Hosted directory/chooser; nearby TV approval | Multiple URLs, local server discovery, profile-bound connections | Local discovery/address, saved servers, users | — (TV-only) | Portico nearby player discovery is not a merged-library connection system. |
| Profile/avatar/PIN | I/A: Direct profile picker, avatar, PIN, limits; Hosted selection/handoff | Plex Home + local profiles, PIN, borrow connection | Server-user avatar/pin/password and local user security | — (TV-only) | Client-local profiles are not interchangeable with server authorization. |
| Home/hero/rows | I/A: Server Home + resume hero + bounded shelves | Discover/Home hubs across connections | Client composed resume/next-up/recent | — (TV-only) | All are media-first but ownership of composition differs. |
| Home customization | I/A: Hide/reorder/reset, add saved view | Hub/source/library management and home layout choice | Recently added/played, rewatch and next-up days | — (TV-only) | Portico has persistent query rows; Plezy has broader source aggregation. |
| Library list/switcher | I/A: Named libraries; phone dropdown, wide rail | Grouped/merged connections and library management | Library drawer/list and favorites/random | — (TV-only) | One Portico LibraryRoot is reused for different libraries. |
| Library Discover/Browse peers | I/A: Kind-specific Discover/movies/shows/episodes/music/books/collections/categories; owner unmatched | Recommended/Browse/Collections/Playlists; grouping/folders | Library display types and filter drawer | — (TV-only) | Portico authoring is shared rather than inferred from backend kind. |
| Sort/filter/jump/view | I/A: Capability sorts, quick/advanced facets, alpha/jump; grid; handheld Save View | Rich operators/ranges/text/include-exclude, alpha, grid/list/folders/random | Filters, sort, letter picker orientation, layout/list columns | — (TV-only) | Portico TV filters need no text; native lacks grid/list choice. |
| Movie detail | I/A: Backdrop, ratings/facts/synopsis, personal actions/More, cast/extras/related/info | Version/track/quality choices, trailers, rate/edit/match/file info | Custom action bar/menu, playback chooser, trailers/refresh/edit/delete | — (TV-only) | Portico version chooser missing from pre-play component. |
| Show/season/episode | I/A: Canonical show workspace + season chooser + episode panel/rows + four show switches | Season tabs/posters, episodes, spoiler masks, specials ordering | Item/detail groups, missing seasons/episodes, playable source selection | — (TV-only) | Portico deep-linked episode stays inside show context. |
| Album/artist/track/book | I/A: Entity/Detail, ordered tracks/chapters, listening queue/lyrics/book speed | Artist/album/track/instant mix/music player; book-specific parity not established | Video-oriented; full music/book UX not established | — (TV-only) | Portico has explicit audiobook UX alongside music. |
| Person/credits | I/A: Person biography/credits, cast/crew shelves | Actor-media results and catalog people | Person/item groups | — (TV-only) | Portico cast See all differs between movie and show. |
| Trailers/extras/gallery | I/A: Extras rows; Gallery is development-only design gallery | Trailers/extras + catalog trailer links | Trailer choices and item groups | — (TV-only) | Design Gallery is not a user photo library. |
| Search | I/A: Grouped all/type results, recent/clear, native keyboard | Cross-server search and catalog search | Search + filters/suggestions from API | — (TV-only) | Portico ranks on server; scope differs. |
| Saved/My List/Favorites | I/A: Six Saved tabs incl collections/views/history | Watchlist/explore plus upstream favorites/collections/playlists | Favorites/library containers; no identical six-tab hub established | — (TV-only) | My List is not a Home shelf in Portico. |
| Playlist/collection/view detail | I/A: Play/shuffle; playlist move/remove; saved resources/read views | Create/add/remove/reorder/delete collection/playlist; smart playlist indication | Collection/item grouping; management varies | — (TV-only) | Portico lacks complete native container CRUD/share. |
| History | I/A: Saved personal history/clear confirmation; owner full play history | Upstream history through services; no equivalent dedicated audited screen | Server activity/admin sessions | — (TV-only) | Portico personal and owner histories have distinct privacy roles. |
| Downloads/offline | I/A: Local files, preparation/batch status, Wi-Fi/space/autoremove/quality, offline auth entry | Grouped tree, batch pause/resume/retry/delete, sync rules, background diagnostics | No comparable download surface established in inspected Swiftfin views | — (TV-only) | Portico TV omission deliberate; phone/tablet remain meaningful. |
| Live/guide/channels | I/A: Guide/All/source/library channels + programme sheets | Guide/What’s On/Recordings/schedule/rules/favorite reorder | Live/programme/recording item groups | — (TV-only) | Portico library channels are an additional first-party concept. |
| DVR recordings/rules/conflicts | I/A: Recordings and series rules, padding/keep/actions; owner source/tuner/storage pages | Record options/rules/schedule/re-evaluate | Record action + admin permission | — (TV-only) | Do not credit stale NOTES saying recording is absent. |
| Watch Together/remote | I/A: Together groups/invites/queue; Play on nearby receiver/Cast/AirPlay | Relay rooms + companion remote with transport/navigation/text | SyncPlay parity not established in reviewed UI | — (TV-only) | SharePlay is a distinct system feature and is not these groups. |
| Notifications/account feed | I/A: In-app notifications, AccountFeed/Membership/invitations | Service/session status; no Portico-style notification hub | Logs/server activity | — (TV-only) | Portico deliberately has no push transport. |
| Settings/profile/security/devices | I/A: Authored account/settings pages; local security/devices; web account handoffs | General/Appearance/Playback/Services/Downloads/Advanced and profiles | Local user/server/customization/playback/quality/logs | — (TV-only) | Owner/admin distinction affects exposed panels. |
| Owner/admin/metadata | I/A: Native server pages and metadata repair on permitted handheld/local owner | Metadata adapters + library maintenance + server tasks | iOS native administration/editor | — (TV-only) | Portico hosted-owner metadata gate remains a gap/risk. |
| About/donation/licences/diagnostics | I/A: App/server version and legal/dataset footer; owner support exports/logs | Donate/about/licences/local logs/upload/reset/export/import | About/repositories/licences/local logs; iOS server logs | — (TV-only) | Portico local app-log/share and licence detail UX is thin. |
| Full player/mini/utility screens | I/A: Shared chrome, menus, audio mini or TV pill, native handheld PiP | Phone/desktop-TV chrome, queue/tracks/settings/chapters/performance | SwiftUI controls/actions/gestures + alternate native player | — (TV-only) | Only UI/chrome considered; engine capabilities not assessed. |

### Tablet

| Screen/workflow | Portico platforms and implementation | Plezy | Swiftfin | jellyfin-androidtv | Notes |
|---|---|---|---|---|---|
| Account/sign-in/create/recovery | iPad/A tablet: Hosted provider/password, direct server password/Quick Connect; account help web handoff | Plex QR/browser; Jellyfin/Emby credentials/Quick Connect | Server connection + user login/Quick Connect | — (TV-only) | Portico account services and direct server are separate authorities. |
| Server/address/discovery/chooser | iPad/A tablet: Address + remembered direct servers; Hosted directory/chooser; nearby TV approval | Multiple URLs, local server discovery, profile-bound connections | Local discovery/address, saved servers, users | — (TV-only) | Portico nearby player discovery is not a merged-library connection system. |
| Profile/avatar/PIN | iPad/A tablet: Direct profile picker, avatar, PIN, limits; Hosted selection/handoff | Plex Home + local profiles, PIN, borrow connection | Server-user avatar/pin/password and local user security | — (TV-only) | Client-local profiles are not interchangeable with server authorization. |
| Home/hero/rows | iPad/A tablet: Server Home + resume hero + bounded shelves | Discover/Home hubs across connections | Client composed resume/next-up/recent | — (TV-only) | All are media-first but ownership of composition differs. |
| Home customization | iPad/A tablet: Hide/reorder/reset, add saved view | Hub/source/library management and home layout choice | Recently added/played, rewatch and next-up days | — (TV-only) | Portico has persistent query rows; Plezy has broader source aggregation. |
| Library list/switcher | iPad/A tablet: Named libraries; phone dropdown, wide rail | Grouped/merged connections and library management | Library drawer/list and favorites/random | — (TV-only) | One Portico LibraryRoot is reused for different libraries. |
| Library Discover/Browse peers | iPad/A tablet: Kind-specific Discover/movies/shows/episodes/music/books/collections/categories; owner unmatched | Recommended/Browse/Collections/Playlists; grouping/folders | Library display types and filter drawer | — (TV-only) | Portico authoring is shared rather than inferred from backend kind. |
| Sort/filter/jump/view | iPad/A tablet: Capability sorts, quick/advanced facets, alpha/jump; grid; handheld Save View | Rich operators/ranges/text/include-exclude, alpha, grid/list/folders/random | Filters, sort, letter picker orientation, layout/list columns | — (TV-only) | Portico TV filters need no text; native lacks grid/list choice. |
| Movie detail | iPad/A tablet: Backdrop, ratings/facts/synopsis, personal actions/More, cast/extras/related/info | Version/track/quality choices, trailers, rate/edit/match/file info | Custom action bar/menu, playback chooser, trailers/refresh/edit/delete | — (TV-only) | Portico version chooser missing from pre-play component. |
| Show/season/episode | iPad/A tablet: Canonical show workspace + season chooser + episode panel/rows + four show switches | Season tabs/posters, episodes, spoiler masks, specials ordering | Item/detail groups, missing seasons/episodes, playable source selection | — (TV-only) | Portico deep-linked episode stays inside show context. |
| Album/artist/track/book | iPad/A tablet: Entity/Detail, ordered tracks/chapters, listening queue/lyrics/book speed | Artist/album/track/instant mix/music player; book-specific parity not established | Video-oriented; full music/book UX not established | — (TV-only) | Portico has explicit audiobook UX alongside music. |
| Person/credits | iPad/A tablet: Person biography/credits, cast/crew shelves | Actor-media results and catalog people | Person/item groups | — (TV-only) | Portico cast See all differs between movie and show. |
| Trailers/extras/gallery | iPad/A tablet: Extras rows; Gallery is development-only design gallery | Trailers/extras + catalog trailer links | Trailer choices and item groups | — (TV-only) | Design Gallery is not a user photo library. |
| Search | iPad/A tablet: Grouped all/type results, recent/clear, native keyboard | Cross-server search and catalog search | Search + filters/suggestions from API | — (TV-only) | Portico ranks on server; scope differs. |
| Saved/My List/Favorites | iPad/A tablet: Six Saved tabs incl collections/views/history | Watchlist/explore plus upstream favorites/collections/playlists | Favorites/library containers; no identical six-tab hub established | — (TV-only) | My List is not a Home shelf in Portico. |
| Playlist/collection/view detail | iPad/A tablet: Play/shuffle; playlist move/remove; saved resources/read views | Create/add/remove/reorder/delete collection/playlist; smart playlist indication | Collection/item grouping; management varies | — (TV-only) | Portico lacks complete native container CRUD/share. |
| History | iPad/A tablet: Saved personal history/clear confirmation; owner full play history | Upstream history through services; no equivalent dedicated audited screen | Server activity/admin sessions | — (TV-only) | Portico personal and owner histories have distinct privacy roles. |
| Downloads/offline | iPad/A tablet: Local files, preparation/batch status, Wi-Fi/space/autoremove/quality, offline auth entry; absent from wide-tablet primary rail (B-21.3) | Grouped tree, batch pause/resume/retry/delete, sync rules, background diagnostics | No comparable download surface established in inspected Swiftfin views | — (TV-only) | Portico TV omission deliberate; phone/tablet remain meaningful. |
| Live/guide/channels | iPad/A tablet: Guide/All/source/library channels + programme sheets | Guide/What’s On/Recordings/schedule/rules/favorite reorder | Live/programme/recording item groups | — (TV-only) | Portico library channels are an additional first-party concept. |
| DVR recordings/rules/conflicts | iPad/A tablet: Recordings and series rules, padding/keep/actions; owner source/tuner/storage pages | Record options/rules/schedule/re-evaluate | Record action + admin permission | — (TV-only) | Do not credit stale NOTES saying recording is absent. |
| Watch Together/remote | iPad/A tablet: Together groups/invites/queue; Play on nearby receiver/Cast/AirPlay | Relay rooms + companion remote with transport/navigation/text | SyncPlay parity not established in reviewed UI | — (TV-only) | SharePlay is a distinct system feature and is not these groups. |
| Notifications/account feed | iPad/A tablet: In-app notifications, AccountFeed/Membership/invitations | Service/session status; no Portico-style notification hub | Logs/server activity | — (TV-only) | Portico deliberately has no push transport. |
| Settings/profile/security/devices | iPad/A tablet: Authored account/settings pages; local security/devices; web account handoffs; settings still phone pushes | General/Appearance/Playback/Services/Downloads/Advanced and profiles | Local user/server/customization/playback/quality/logs | — (TV-only) | Owner/admin distinction affects exposed panels. |
| Owner/admin/metadata | iPad/A tablet: Native server pages and metadata repair on permitted handheld/local owner | Metadata adapters + library maintenance + server tasks | Native iOS admin suite + editor | — (TV-only) | Portico hosted-owner metadata gate remains a gap/risk. |
| About/donation/licences/diagnostics | iPad/A tablet: App/server version and legal/dataset footer; owner support exports/logs | Donate/about/licences/local logs/upload/reset/export/import | About/repositories/licences/local logs; iOS server logs | — (TV-only) | Portico local app-log/share and licence detail UX is thin. |
| Full player/mini/utility screens | iPad/A tablet: Shared chrome, menus, audio mini or TV pill, native handheld PiP | Phone/desktop-TV chrome, queue/tracks/settings/chapters/performance | SwiftUI controls/actions/gestures + alternate native player | — (TV-only) | Only UI/chrome considered; engine capabilities not assessed. |

### TV

| Screen/workflow | Portico platforms and implementation | Plezy | Swiftfin | jellyfin-androidtv | Notes |
|---|---|---|---|---|---|
| Account/sign-in/create/recovery | T/G/F/V: activation QR/code + direct server address/Quick Connect/password fallback; create/recovery handed off | Plex QR/browser; Jellyfin/Emby credentials/Quick Connect | Server connection + user login/Quick Connect | Server selection + user/password/Quick Connect | Portico account services and direct server are separate authorities. |
| Server/address/discovery/chooser | T/G/F/V shared: Address + remembered direct servers; Hosted directory/chooser; nearby TV approval | Multiple URLs, local server discovery, profile-bound connections | Local discovery/address, saved servers, users | Discovery/address/manual server entry, saved servers/users | Portico nearby player discovery is not a merged-library connection system. |
| Profile/avatar/PIN | T/G/F/V shared: Direct profile picker, avatar, PIN, limits; Hosted selection/handoff | Plex Home + local profiles, PIN, borrow connection | Server-user avatar/pin/password and local user security | Saved/server users with avatars and automatic-sign-in options | Client-local profiles are not interchangeable with server authorization. |
| Home/hero/rows | T/G/F/V shared: Server Home + resume hero + bounded shelves | Discover/Home hubs across connections | Client composed resume/next-up/recent | Configured Home row slots | All are media-first but ownership of composition differs. |
| Home customization | T/G/F/V shared: Hide/reorder/reset, add saved view | Hub/source/library management and home layout choice | Recently added/played, rewatch and next-up days | Ten row slots plus Next Up cutoff | Portico has persistent query rows; Plezy has broader source aggregation. |
| Library list/switcher | T/G/F/V shared: Named libraries; phone dropdown, wide rail | Grouped/merged connections and library management | Library drawer/list and favorites/random | Library tiles and smart/grid displays | One Portico LibraryRoot is reused for different libraries. |
| Library Discover/Browse peers | T/G/F/V shared: Kind-specific Discover/movies/shows/episodes/music/books/collections/categories; owner unmatched | Recommended/Browse/Collections/Playlists; grouping/folders | Library display types and filter drawer | Smart browse, row/grid categories | Portico authoring is shared rather than inferred from backend kind. |
| Sort/filter/jump/view | T/G/F/V shared: Capability sorts, quick/advanced facets, alpha/jump; grid; handheld Save View; Save View authoring absent from inline TV toolbar | Rich operators/ranges/text/include-exclude, alpha, grid/list/folders/random | Filters, sort, letter picker orientation, layout/list columns | Sort, favorites/unwatched, alpha and per-library size/type/direction | Portico TV filters need no text; native lacks grid/list choice. |
| Movie detail | T/G/F/V shared: Backdrop, ratings/facts/synopsis, personal actions/More, cast/extras/related/info | Version/track/quality choices, trailers, rate/edit/match/file info | Custom action bar/menu, playback chooser, trailers/refresh/edit/delete | Detail actions, media/source/track info, trailers | Portico version chooser missing from pre-play component. |
| Show/season/episode | T/G/F/V shared: Canonical show workspace + season chooser + episode panel/rows + four show switches | Season tabs/posters, episodes, spoiler masks, specials ordering | Item/detail groups, missing seasons/episodes, playable source selection | Show/season/episode browsing and next episode | Portico deep-linked episode stays inside show context. |
| Album/artist/track/book | T/G/F/V shared: Entity/Detail, ordered tracks/chapters, listening queue/lyrics/book speed | Artist/album/track/instant mix/music player; book-specific parity not established | Video-oriented; full music/book UX not established | Music/audio browsing; depth differs | Portico has explicit audiobook UX alongside music. |
| Person/credits | T/G/F/V shared: Person biography/credits, cast/crew shelves | Actor-media results and catalog people | Person/item groups | Person browsing/detail | Portico cast See all differs between movie and show. |
| Trailers/extras/gallery | T/G/F/V shared: Extras rows; Gallery is development-only design gallery | Trailers/extras + catalog trailer links | Trailer choices and item groups | Trailer/external player options | Design Gallery is not a user photo library. |
| Search | T/G/F/V shared: Grouped all/type results, recent/clear, native keyboard | Cross-server search and catalog search | Search + filters/suggestions from API | Text/voice search via platform | Portico ranks on server; scope differs. |
| Saved/My List/Favorites | T/G/F/V shared: Six Saved tabs incl collections/views/history | Watchlist/explore plus upstream favorites/collections/playlists | Favorites/library containers; no identical six-tab hub established | Favorites/playlists/collections | My List is not a Home shelf in Portico. |
| Playlist/collection/view detail | T/G/F/V shared: Play/shuffle; playlist move/remove; saved resources/read views | Create/add/remove/reorder/delete collection/playlist; smart playlist indication | Collection/item grouping; management varies | Playlist details/create and collections | Portico lacks complete native container CRUD/share. |
| History | T/G/F/V shared: Saved personal history/clear confirmation; owner full play history | Upstream history through services; no equivalent dedicated audited screen | Server activity/admin sessions | User resume/watched + activity paths | Portico personal and owner histories have distinct privacy roles. |
| Downloads/offline | T/G/F/V: deliberately unavailable | Grouped tree, batch pause/resume/retry/delete, sync rules, background diagnostics; TV gates must be checked per operation | No comparable download surface established in inspected Swiftfin views | No offline download destination established | Portico TV omission deliberate; phone/tablet remain meaningful. |
| Live/guide/channels | T/G/F/V shared: Guide/All/source/library channels + programme sheets | Guide/What’s On/Recordings/schedule/rules/favorite reorder | Live/programme/recording item groups | Guide/Live/recordings with TV filters | Portico library channels are an additional first-party concept. |
| DVR recordings/rules/conflicts | T/G/F/V shared: Recordings and series rules, padding/keep/actions; owner source/tuner/storage pages | Record options/rules/schedule/re-evaluate | Record action + admin permission | Timers/recordings/guide recording | Do not credit stale NOTES saying recording is absent. |
| Watch Together/remote | T/G/F/V shared: Together groups/invites/queue; Play on nearby receiver/Cast/AirPlay | Relay rooms + companion remote with transport/navigation/text | SyncPlay parity not established in reviewed UI | Remote session support varies; no Plezy-style room parity established | SharePlay is a distinct system feature and is not these groups. |
| Notifications/account feed | T/G/F/V shared: In-app notifications, AccountFeed/Membership/invitations | Service/session status; no Portico-style notification hub | Logs/server activity | Notifications/status dialogs | Portico deliberately has no push transport. |
| Settings/profile/security/devices | T/G/F/V shared: Authored account/settings pages; local security/devices; web account handoffs | General/Appearance/Playback/Services/Downloads/Advanced and profiles | Local user/server/customization/playback/quality/logs | Authentication/customization/playback/screensaver/telemetry/about | Owner/admin distinction affects exposed panels. |
| Owner/admin/metadata | T/G/F/V: Dashboard only; metadata editor suppressed | Metadata adapters + library maintenance + server tasks | tvOS: no iOS admin Dashboard | No comparable broad native server console | Portico hosted-owner metadata gate remains a gap/risk. |
| About/donation/licences/diagnostics | T/G/F/V shared: App/server version and legal/dataset footer; owner support exports/logs | Donate/about/licences/local logs/upload/reset/export/import | About/repositories/licences/local logs; iOS server logs | About/licence list/telemetry/developer/cache | Portico local app-log/share and licence detail UX is thin. |
| Full player/mini/utility screens | T/G/F/V: full-screen video/audio, TV pill, no PiP/download; Siri touch T only | Phone/desktop-TV chrome, queue/tracks/settings/chapters/performance | SwiftUI controls/actions/gestures + alternate native player | TV controls, queue, media segments, clock | Only UI/chrome considered; engine capabilities not assessed. |

### Form-factor details that the common screen names hide

- **Phone:** Portrait and landscape retain tabs, safe-area top controls and native push/back. Field entry uses platform text input. No source-backed full-screen brightness/volume swipe, hold-to-speed, pinch zoom or configurable gesture editor was found; the timeline itself has touch drag. The UI has no general haptic preference/consumer path comparable to Streamyfin.
- **Tablet:** A wide tablet gets the rail at 700 points, while component `useSpecPlatform` uses a 600-point short-side rule and wide content uses 900 points. These are different thresholds serving different layers, not one documented tablet breakpoint. Responsive grids and wide guide are present. Two-pane Settings, master/detail persistence, keyboard-shortcut editing, app-owned external-display presentation and Stage Manager-specific outcomes were not established. Width adaptation does not establish multitasking quality.
- **TV:** Rail expansion, native focus guides, large targets, two-row browse header, modal traps and remote Back hierarchy are implemented. Android converts density to the 1920×1080 reference design; Vega uses a transformed canvas plus focus/snap/typing adapters. TV version/track selection, focus return after keyboard, long-press actions and now-playing reachability require actual hardware validation.
- **Target integrations:** Google Android has a Cast sender and TV home-channel integration; Fire flavor omits Google Cast/home rows. Vega currently stores a shelf configuration but does not publish home recommendations. Apple has Spotlight/Handoff/App Intents, absent from Android/Vega adapters. These gaps do not disappear because UI files are shared.

## Settings inventory

This inventory distinguishes **rendered settings**, **stored fields**, **composite controls** (one UI choice writing several fields), and **actions/panels**. It does not count a preference constant as proof that a user can reach it. The detailed tables retain binding names so every setting remains findable even if its label changes. Platform gates are source gates, not device certification.

### Cross-app setting groups and parity

| Group / individual choices | Portico native | Plezy first | Swiftfin | Android TV | Wholphin / Streamyfin | Assessment |
|---|---|---|---|---|---|---|
| Theme; system/light/dark; OLED/AMOLED; accent | Fixed theatre palette; device Reduce Motion; no theme/accent/OLED choice | System/Light/Dark/OLED; configurable accent not established | iOS appearance; user accent picker | Theme choice | Both have appearance settings; detailed bindings below | Deliberate visual contract difference; OLED directly conflicts with pure-black ban. |
| Poster/card density; spacing; grid/list; image type; title/subtitle labels | Fixed native metrics; card size/default view/backdrops rows web-only | Density 1–5; spacing tight/normal/spacious; grid/list; episode poster/thumbnail | Layout/poster type/list columns; label/subtitle; indicators | Size/type/grid direction per library | Home/library card/scale options | Portico behind in personal display controls. |
| Home rows/hero/source display | Server layout reorder/hide/reset/add saved view; no My List shelf | Hubs, global home layout, server names, library grouping/hide/reorder; hero handheld toggle | Recent added/played; rewatch; Next Up age | Ten Home slots; Next Up cutoff | Wholphin row editor; Streamyfin combined/upcoming/hero filters | Portico saved query rows strong; no merged sources. |
| Spoilers; episode art; missing episodes | No explicit spoiler control/consumer established | Blur unwatched thumbnails/descriptions; series/season/thumb choice | Missing seasons/episodes; series thumb choice | Series thumbnails and indicators | Streamyfin episode/Home poster switches | Spoiler masking is high-value missing parity. |
| Autoplay; countdown; still watching | Profile/server autoplay, 0/5/10/15 countdown; still-watching + 2–5 episodes | Autoplay/countdown; shuffle-from-start | Server autoplay + player behavior | Queueing/Next Up behavior/timeout; inactivity prompt | Both expose next/still-watching | Similar outcomes with different authorities. |
| Intro/credits/recap/commercial/preview | Ask/auto/off intro, credits, recap | Intro/credits modes, delay, chapter regex fallback | Supplement selector; media-segment action control unestablished | Actions by media-segment type | Streamyfin has all five named segment types | Portico lacks commercial/preview user rows. |
| Seek back/forward; rewind on resume | 5/10/15/30 back; 10/15/30/60 forward; no resume rewind setting | Small/large skip and rewind on resume | Intervals/resume offset | Skip intervals + resume subtraction | Wholphin broad intervals; Streamyfin intervals | Portico interval ranges narrower deliberately validated. |
| Speed/sleep/watched thresholds | Video/book default speed; watched threshold; sleep timer during playback; registry sleep default not in Settings | Speed scope; sleep default; playback scope memory | Speed/gesture boost; resume rules | Player settings/idle choices | Streamyfin speed per media/show | Portico default sleep field not an exposed settings row. |
| Quality by network | Home/Away/mobile Cellular; composite height + literal original/automatic | Default + cellular; covered-quality direct-play preference | Playback quality/device profile | Max bitrate | Streamyfin default bitrate and network/header tools | Portico ahead in explicit home/away separation; scope label issue B-21.7. |
| Delivery/HDR/device tuning controls | Original/Automatic/Convert; HDR auto/off; owner streaming pages | External player, decoding/buffer/DV/HDR/passthrough/channel choices | Player/profile/quality plus native behaviors | Codec/level/HDR/buffer/audio/refresh overrides | Wholphin/Streamyfin tuning controls | Listed as UX choices only, engines excluded. |
| Audio/subtitle language/mode/memory | Ordered languages; Off/Forced/Always; pre-play track/language menus | Upstream preferences + remember/follow track choices; SDH/forced rules | Server audio/subtitle + remember settings | Uses server language choices + player options | Streamyfin audio/default/remember/sub-on-mute | Portico lacks smart foreign-audio/SDH UI policy. |
| Subtitle style/preview | Native settings omit style; registry has size/background | Size/colors/border/opacity/top-bottom/margins/bold/italic/ASS settings | Configuration controls | Size/weight/color/stroke/background/offset preview | Wholphin subtitle settings; Streamyfin rich style | Recheck native Android/Vega ownership before hiding controls. |
| Music | Leveling off/track/album, crossfade 0–12, gapless, synced lyrics | Music quality, session restore, normalization utilities | Video-focused inventory | Audio behavior/library settings | Wholphin music preferences; Streamyfin local/cache/leveling | Portico first-party book/music separation worth keeping. |
| Downloads | Quality/preparation/local statuses, Wi-Fi, quota, watched remove; own storage | Custom location, Wi-Fi, watched remove, sync rules, background diagnostic, batch | Not established | TV omission | Streamyfin quality/storage and iOS Live Activity | Portico no keep-N rolling rules/custom root. |
| Gesture/haptic/input | Timeline drag/Siri touch; no gesture/haptic/shortcut editor | Brightness/volume/pinch, remembered brightness; editable shortcuts/gamepad | Pan/swipe/hold/multitap/two-finger/pinch settings | Remote native navigation | Streamyfin all common gestures/disable haptics | Portico behind for handheld ergonomics. |
| Clock/theme music/backdrop/focus | TV text-size/Reduce Motion; fixed focus; no clock/theme-music setting | System clock widget/focus-glow/corner-backdrop/effects; no dedicated screensaver established | Custom poster/item/home design | Clock/backdrop/theme/watched + screensaver | Wholphin theme-song volume; Streamyfin TV theme music/type | Device-polish gap; do not turn absent setting into assumed bug. |
| Startup/profile/auto-login/idle | Account/server/profile flow; no ask-profile-every-open switch | Ask profile on open, startup section, force-TV | Local-user security/start behavior | Auto sign-in, user selection/sort | Streamyfin TV inactivity logout | Client conveniences should never weaken server restrictions. |
| Privacy/search/history/activity | Pause history, member activity, reset recommendations; remember-search toggle web-only despite native consumer | Tracker/library filters, crash-reporting/local options | Local privacy/security/logs | Telemetry consent | Streamyfin Sentry/plugin settings | Portico deliberate no push, but hidden native privacy toggles matter. |
| External services/discovery/request | No native Trakt/Seerr/MDBList/Marlin/Discord UI established | Trakt/MAL/AniList/Simkl/Seerr/MDBList; Discord desktop | Jellyfin server API | Jellyfin server/API | Wholphin Seerr; Streamyfin plugin/Seerr/MDBList/Marlin | Medium/large optional ecosystem work. |
| Diagnostics/data/reset/about/licences | Owner server logs/export/detail window, app/server version; no equivalent local user log/reset/import/export/licence explorer | Logs copy/upload/clear, debug, cache clear, export/import/reset, donate/about/licences | Local logs/about + iOS server admin | Telemetry/developer/cache/about/licences | Both have support/log paths | Portico owner diagnostics strong; client self-service thinner. |


Swiftfin player-setting details beyond the binding index: custom backward/forward intervals clamp to 1–600 seconds; resume offset 0–30 seconds; server autoplay; chapter slider; preview-image scrubbing; supplements selector; trailing timestamp type; audio language/play-default-track/remember-audio; subtitle language/mode/remember-subtitle/force-burn-in; subtitle font/size 1–20/color. `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:175`, `:232`, `:294`, `:331`, `:385`. The prompt mentioned Swiftfin media-segment actions; a dedicated segment-type action setting was not established in the inspected current views, so Android TV/Streamyfin supply that comparison rather than assumed Swiftfin parity.

The prompt also mentioned Android TV focus color/rating badge types/theme music and Plezy accent/screensaver/casting. These are search targets, not automatically established current settings. Current Android TV settings expose theme/backdrop/clock/watched/series thumbnails and library size/type/direction; a distinct arbitrary focus-color or rating-badge-type control was not established in this checkout. Wholphin supplies actual theme-color/theme-song settings. A Chromecast-branded device mention or media output route in Plezy is not proof of a Cast sender. These limits are recorded to avoid turning requested examples into invented features.

### Portico: every published preference and whether native renders it

All canonical keys below come from `apps/portico-server/server/internal/operations/preferences.go`. This is the exhaustive current registry, including the twenty network fields. **All-native** means phone/tablet plus TV/Vega shared renderer; **mobile** means phone/tablet; TV-only device values are listed separately. A server can omit a row through missing registry keys. This table does not claim hidden fields are secretly exposed.

| Preference | Default/domain (source declaration) | Scope | Native settings visibility / consumption | Source |
|---|---|---|---|---|
| `playback.skipBackSeconds` | `10, []int{5, 10, 15, 30}` | Profile/server | All-native authored row | `operations/preferences.go:164` |
| `playback.skipForwardSeconds` | `30, []int{10, 15, 30, 60}` | Profile/server | All-native authored row | `operations/preferences.go:165` |
| `playback.autoplayNext` | `true` | Profile/server | All-native authored row | `operations/preferences.go:166` |
| `playback.upNextCountdownSeconds` | `10, []int{0, 5, 10, 15}` | Profile/server | All-native authored row | `operations/preferences.go:167` |
| `playback.passoutProtection` | `true` | Profile/server | All-native authored row | `operations/preferences.go:168` |
| `playback.passoutAfterEpisodes` | `3, 2, 5` | Profile/server | All-native authored row | `operations/preferences.go:169` |
| `playback.introSkip` | `"ask", skipModes` | Profile/server | All-native authored row | `operations/preferences.go:170` |
| `playback.creditsSkip` | `"ask", skipModes` | Profile/server | All-native authored row | `operations/preferences.go:171` |
| `playback.recapSkip` | `"ask", skipModes` | Profile/server | All-native authored row | `operations/preferences.go:172` |
| `playback.defaultSpeed` | `1, playbackSpeeds` | Profile/server | All-native authored row | `operations/preferences.go:173` |
| `music.defaultSpeed` | `1, playbackSpeeds` | Profile/server | Published; deliberately no music speed row | `operations/preferences.go:174` |
| `audiobooks.defaultSpeed` | `1, playbackSpeeds` | Profile/server | All-native authored row | `operations/preferences.go:175` |
| `playback.sleepTimerMinutes` | `0, []int{0, 30, 60, 90, 120}` | Profile/server | Published; player utility; no standing Settings row | `operations/preferences.go:176` |
| `playback.startedThresholdPercent` | `5, 1, 25` | Profile/server | Published; no authored native row | `operations/preferences.go:177` |
| `playback.playedThresholdPercent` | `95, 75, 100` | Profile/server | All-native authored row | `operations/preferences.go:178` |
| `playback.preferredAudioLanguages` | `ordered language list; semantic default` | Profile/server | All-native authored row | `operations/preferences.go:179` |
| `playback.preferredSubtitleLanguages` | `ordered language list; semantic default` | Profile/server | All-native authored row | `operations/preferences.go:180` |
| `playback.subtitleMode` | `"forced", []string{"off", "forced", "always"}` | Profile/server | All-native authored row | `operations/preferences.go:183` |
| `playback.subtitleSize` | `"medium", []string{"small", "medium", "large", "extra-large"}` | Profile/server | Web-only authored style; not native Settings | `operations/preferences.go:184` |
| `playback.subtitleBackground` | `"translucent", []string{"none", "translucent", "opaque"}` | Profile/server | Web-only authored style; not native Settings | `operations/preferences.go:185` |
| `playback.showSyncedLyrics` | `true` | Profile/server | All-native authored row | `operations/preferences.go:186` |
| `delivery.directPlay` | `"prefer", deliveryModes` | Profile/device-class | Composite Playback method (all-native) | `operations/preferences.go:188` |
| `delivery.directStream` | `"allow", deliveryModes` | Profile/device-class | Composite Playback method (all-native) | `operations/preferences.go:189` |
| `delivery.transcode` | `"allow", deliveryModes` | Profile/device-class | Composite Playback method (all-native) | `operations/preferences.go:190` |
| `music.audioNormalization` | `"off", []string{"off", "track", "album"}` | Profile/server | All-native authored row | `operations/preferences.go:194` |
| `music.crossfadeSeconds` | `0, 0, 12` | Profile/server | All-native authored row | `operations/preferences.go:195` |
| `music.gapless` | `true` | Profile/server | All-native authored row | `operations/preferences.go:196` |
| `privacy.pauseWatchHistory` | `false` | Profile/server | All-native authored row | `operations/preferences.go:198` |
| `privacy.showActivityToMembers` | `true` | Profile/server | All-native authored row | `operations/preferences.go:199` |
| `privacy.includeInWatchTogether` | `true` | Profile/server | Published; no authored native row | `operations/preferences.go:200` |
| `search.rememberHistory` | `true` | Profile/server | Web-only row; native Search reads it but cannot toggle it | `operations/preferences.go:202` |
| `region.locale` | `"locale", 35` | Profile/server | All-native authored row | `operations/preferences.go:205` |
| `region.hourCycle` | `"auto", []string{"auto", "h12", "h23"}` | Profile/server | All-native authored row | `operations/preferences.go:206` |
| `home.rowOrder` | `HomeLayoutMaxRows, HomeLayoutRowIDMaxLength` | Profile/server | Home customization, not generic Settings | `operations/preferences.go:208` |
| `home.hiddenRowIds` | `HomeLayoutMaxRows, HomeLayoutRowIDMaxLength` | Profile/server | Home customization, not generic Settings | `operations/preferences.go:209` |
| `appearance.showBackdrops` | `true` | Profile/device-class | Web-only row; native hero does not read this field | `operations/preferences.go:211` |
| `appearance.cardSizePercent` | `100, 75, 150` | Profile/device-class | Web-only composite cardSize; native metrics fixed (known Backlog) | `operations/preferences.go:212` |
| `appearance.reduceMotion` | `false` | Profile/device-class | Web registry row; native uses installation-local reduceMotion | `operations/preferences.go:213` |
| `notifications.badges` | `true` | Profile/server | Web-only row; native shell reads it but cannot toggle it | `operations/preferences.go:215` |
| `quality.local.mode` | qualityModes; network default | Profile/device-class | Home composite; individual field not rendered | `operations/preferences.go:152` |
| `quality.local.maxVideoBitrateMbps` | video presets; 0 means no bitrate ceiling | Profile/device-class | Home composite; individual field not rendered | `operations/preferences.go:153` |
| `quality.local.maxAudioBitrateKbps` | audio presets; 0 means no ceiling | Profile/device-class | Home composite; individual field not rendered | `operations/preferences.go:154` |
| `quality.local.maxVideoHeight` | videoHeights | Profile/device-class | Home composite; individual field not rendered | `operations/preferences.go:155` |
| `quality.local.allowHDR` | network HDR default | Profile/device-class | HDR composite (all four networks) | `operations/preferences.go:156` |
| `quality.wifi.mode` | qualityModes; network default | Profile/device-class | Away (wifi and unknown share one row) composite; individual field not rendered | `operations/preferences.go:152` |
| `quality.wifi.maxVideoBitrateMbps` | video presets; 0 means no bitrate ceiling | Profile/device-class | Away (wifi and unknown share one row) composite; individual field not rendered | `operations/preferences.go:153` |
| `quality.wifi.maxAudioBitrateKbps` | audio presets; 0 means no ceiling | Profile/device-class | Away (wifi and unknown share one row) composite; individual field not rendered | `operations/preferences.go:154` |
| `quality.wifi.maxVideoHeight` | videoHeights | Profile/device-class | Away (wifi and unknown share one row) composite; individual field not rendered | `operations/preferences.go:155` |
| `quality.wifi.allowHDR` | network HDR default | Profile/device-class | HDR composite (all four networks) | `operations/preferences.go:156` |
| `quality.cellular.mode` | qualityModes; network default | Profile/device-class | Cellular (mobile only) composite; individual field not rendered | `operations/preferences.go:152` |
| `quality.cellular.maxVideoBitrateMbps` | video presets; 0 means no bitrate ceiling | Profile/device-class | Cellular (mobile only) composite; individual field not rendered | `operations/preferences.go:153` |
| `quality.cellular.maxAudioBitrateKbps` | audio presets; 0 means no ceiling | Profile/device-class | Cellular (mobile only) composite; individual field not rendered | `operations/preferences.go:154` |
| `quality.cellular.maxVideoHeight` | videoHeights | Profile/device-class | Cellular (mobile only) composite; individual field not rendered | `operations/preferences.go:155` |
| `quality.cellular.allowHDR` | network HDR default | Profile/device-class | HDR composite (all four networks) | `operations/preferences.go:156` |
| `quality.unknown.mode` | qualityModes; network default | Profile/device-class | Away (wifi and unknown share one row) composite; individual field not rendered | `operations/preferences.go:152` |
| `quality.unknown.maxVideoBitrateMbps` | video presets; 0 means no bitrate ceiling | Profile/device-class | Away (wifi and unknown share one row) composite; individual field not rendered | `operations/preferences.go:153` |
| `quality.unknown.maxAudioBitrateKbps` | audio presets; 0 means no ceiling | Profile/device-class | Away (wifi and unknown share one row) composite; individual field not rendered | `operations/preferences.go:154` |
| `quality.unknown.maxVideoHeight` | videoHeights | Profile/device-class | Away (wifi and unknown share one row) composite; individual field not rendered | `operations/preferences.go:155` |
| `quality.unknown.allowHDR` | network HDR default | Profile/device-class | HDR composite (all four networks) | `operations/preferences.go:156` |

The default network policies differ: local/Wi-Fi allow more than cellular/unknown. The Home/Away/Cellular UI rewrites mode and clears both video and audio bitrate ceilings when choosing a height (`C/presentation/settings-structure.ts:601`). Thus it is not a harmless display alias for an arbitrary existing network policy. Hidden API-only combinations render as nearest UI choice; changing the row normalizes them.

### Portico: installation-local values, actions, and account panels

| Setting/action/panel | Platforms | Scope and behavior | Evidence |
|---|---|---|---|
| Reduce Motion | All native | Installation-local; OR with system preference; cannot override system Reduce Motion | `N/src/ui/theme.ts:158`, `N/src/app/preferences.ts:7` |
| Text size Standard/Large | All TV incl Vega | Installation-local; 1× / 1.2× type scale | `N/App.tsx:48`, `C/presentation/settings-structure.ts:248` |
| Receive from devices | All TV incl Vega | Installation-local allow receiver; help describes other devices of this profile | `settings-structure.ts:278` |
| Legacy autoplayNext/musicSpeed/audiobookSpeed/subtitleLanguage local fields | Storage only | Persisted local parse defaults; not the current authored settings rows; do not credit as additional user choices | `N/src/app/preferences.ts:13` |
| Device name | All native | Native identity editor | `N/src/screens/settings/SettingsPage.tsx:264` |
| Link a TV | Mobile | Approval/manual code/nearby discovery workflow | `settings-structure.ts:277`, `N/src/screens/LinkTV.tsx:1` |
| Profile identity/avatars/names/PIN/limits | Mobile local-account management; TV web handoff | Server account/profile authority, permitted manager only | `SettingsPage.tsx:215`, `Profiles.tsx:1`, `Security.tsx:1` |
| Password; two-step/recovery codes; device/session revocation | Mobile; TV limited/handoff | Local-account manager constraints; Hosted account handoff separately | `SettingsPage.tsx:229`, `Security.tsx:1` |
| Portico Account/membership/invitation/account deletion | Conditional mobile; printed/QR TV handoffs | Distinct account/server authorities; native deletion confirmation sheet | `SettingsPage.tsx:246`, `ServerMembership.tsx:1`, `DeleteAccount.tsx:1` |
| Switch/refresh server; connect direct | All native | Known direct servers and Hosted directory; direct credentials reentered | `N/src/screens/SettingsPages.tsx:83` |
| Downloads storage/rules | Mobile | Wi-Fi-only, quota 0/5/10/25/50/100 GB; watched remove; preparation versus local files | `N/src/screens/Downloads.tsx:20` |
| Reset recommendations | All native | Explicit scoped reset flow; confirmation | `N/src/screens/settings/ResetRecommendations.tsx:1` |
| Sign out | All native | Confirmation explains local-download consequence | `N/src/screens/SettingsPages.tsx:72` |
| About | All native | App version, server name/version, legal/dataset attribution | `SettingsPage.tsx:288` |
| Show-specific switches | Show detail | Four server show settings; ordering/keep/delete-after not built | `N/src/screens/Show.tsx:432`, `C/presentation/title-page.ts` |

### Portico: every authored owner setting

These rows resolve on owner phone/tablet (iOS/Android); TV/Vega exposes Dashboard panels only. All depend on the server returning the corresponding draft value. The following table enumerates every `server-setting` row; native custom panel inventories follow it. Source is `C/presentation/settings-structure.ts`.

| Owner group | Form.field | Control/domain from source | Native platforms | Source |
|---|---|---|---|---|
| sources | `liveDefaults.streamBufferSeconds` | number; min 0, max 120 | Owner phone/tablet | `settings-structure.ts:318` |
| sources | `liveDefaults.retryWindowSeconds` | number; min 0, max 600 | Owner phone/tablet | `settings-structure.ts:319` |
| sources | `liveDefaults.guideDays` | number; min 1, max 21 | Owner phone/tablet | `settings-structure.ts:320` |
| sources | `liveDefaults.userAgent` | text;  | Owner phone/tablet | `settings-structure.ts:321` |
| sources | `liveDefaults.logoImport` | Switch;  | Owner phone/tablet | `settings-structure.ts:322` |
| sources | `liveDefaults.discoveryEnabled` | Switch;  | Owner phone/tablet | `settings-structure.ts:323` |
| sources | `recording.prePaddingSeconds` | number; min 0, max 120, scale 60 | Owner phone/tablet | `settings-structure.ts:330` |
| sources | `recording.postPaddingSeconds` | number; min 0, max 240, scale 60 | Owner phone/tablet | `settings-structure.ts:331` |
| sources | `recording.keepPolicy.mode` | choice;  | Owner phone/tablet | `settings-structure.ts:332` |
| sources | `recording.keepPolicy.keepCount` | number; min 1, max 1000 | Owner phone/tablet | `settings-structure.ts:333` |
| sources | `recording.keepPolicy.keepDays` | number; min 1, max 3650 | Owner phone/tablet | `settings-structure.ts:334` |
| sources | `recording.conversion.mode` | choiceFrom;  | Owner phone/tablet | `settings-structure.ts:335` |
| sources | `recording.conversion.deleteOriginal` | Switch;  | Owner phone/tablet | `settings-structure.ts:336` |
| sources | `recording.folderTemplate` | text;  | Owner phone/tablet | `settings-structure.ts:337` |
| sources | `recording.movieFolderTemplate` | text;  | Owner phone/tablet | `settings-structure.ts:338` |
| transcoding | `runtime.transcodingEnabled` | Switch;  | Owner phone/tablet | `settings-structure.ts:353` |
| transcoding | `runtime.hardwareBackend` | choice; ['auto', 'software', 'videotoolbox', 'vaapi', 'qsv', 'nvenc', 'amf'] | Owner phone/tablet | `settings-structure.ts:354` |
| transcoding | `runtime.hardwareDevice` | text;  | Owner phone/tablet | `settings-structure.ts:355` |
| transcoding | `runtime.x264Preset` | choice; ['ultrafast', 'superfast', 'veryfast', 'faster', 'fast', 'medium', 'slow'] | Owner phone/tablet | `settings-structure.ts:356` |
| transcoding | `runtime.hdrToneMapping` | Switch;  | Owner phone/tablet | `settings-structure.ts:357` |
| transcoding | `runtime.hdrToneMappingAlgorithm` | choice; ['hable', 'mobius', 'reinhard', 'gamma', 'linear', 'clip'] | Owner phone/tablet | `settings-structure.ts:358` |
| transcoding | `runtime.directStreamRemux` | Switch;  | Owner phone/tablet | `settings-structure.ts:359` |
| transcoding | `runtime.planningPolicy` | choice; ['maximum_fidelity', 'maximum_compatibility', 'minimize_server_work'] | Owner phone/tablet | `settings-structure.ts:360` |
| transcoding | `runtime.temporaryDirectory` | text;  | Owner phone/tablet | `settings-structure.ts:361` |
| transcoding | `runtime.serverCap` | number; min 1, max 1000000 | Owner phone/tablet | `settings-structure.ts:364` |
| transcoding | `runtime.perAccountCap` | number; min 1, max 1000000 | Owner phone/tablet | `settings-structure.ts:365` |
| transcoding | `runtime.maxConcurrentSessions` | number; min 0, max 64 | Owner phone/tablet | `settings-structure.ts:366` |
| transcoding | `runtime.maxHardwareSessions` | number; min 0, max 64 | Owner phone/tablet | `settings-structure.ts:367` |
| transcoding | `runtime.maxSoftwareSessions` | number; min 0, max 64 | Owner phone/tablet | `settings-structure.ts:368` |
| transcoding | `runtime.maxBackgroundSessions` | number; min 0, max 64 | Owner phone/tablet | `settings-structure.ts:369` |
| limits | `connectivity.remoteBitrateLimitKbps` | number; min 0, max 200, scale 1000 | Owner phone/tablet | `settings-structure.ts:373` |
| limits | `connectivity.uploadCapacityKbps` | number; min 0, max 10000, scale 1000 | Owner phone/tablet | `settings-structure.ts:374` |
| limits | `connectivity.pausedSessionTimeoutMinutes` | choice;  | Owner phone/tablet | `settings-structure.ts:375` |
| remote | `remote.enabled` | Switch;  | Owner phone/tablet | `settings-structure.ts:382` |
| remote | `remote.ipv6Open` | Switch;  | Owner phone/tablet | `settings-structure.ts:383` |
| remote | `certificate.enabled` | Switch;  | Owner phone/tablet | `settings-structure.ts:387` |
| remote | `certificate.publicPort` | number; min 1, max 65535 | Owner phone/tablet | `settings-structure.ts:388` |
| remote | `connectivity.remoteSignInPolicy` | choice; ['allow', 'owner-only', 'off'] | Owner phone/tablet | `settings-structure.ts:391` |
| remote | `connectivity.secureConnectionsPolicy` | choice; ['required', 'preferred', 'lan-plain-allowed'] | Owner phone/tablet | `settings-structure.ts:392` |
| home | `connectivity.lanDiscoveryEnabled` | Switch;  | Owner phone/tablet | `settings-structure.ts:395` |
| home | `connectivity.lanNetworks` | lines;  | Owner phone/tablet | `settings-structure.ts:396` |
| home | `connectivity.treatWanAsLan` | Switch;  | Owner phone/tablet | `settings-structure.ts:397` |
| home | `connectivity.accessUrls` | lines;  | Owner phone/tablet | `settings-structure.ts:398` |
| home | `remote.mapping` | Switch;  | Owner phone/tablet | `settings-structure.ts:402` |
| home | `remote.pcp` | Switch;  | Owner phone/tablet | `settings-structure.ts:403` |
| home | `remote.natpmp` | Switch;  | Owner phone/tablet | `settings-structure.ts:404` |
| home | `remote.upnp` | Switch;  | Owner phone/tablet | `settings-structure.ts:405` |
| home | `remote.publicPort` | number; min 1, max 65535 | Owner phone/tablet | `settings-structure.ts:406` |
| home | `remote.gateway` | text;  | Owner phone/tablet | `settings-structure.ts:407` |
| home | `connectivity.trustedProxies` | lines;  | Owner phone/tablet | `settings-structure.ts:408` |
| home | `connectivity.advertisedInterface` | choiceFrom;  | Owner phone/tablet | `settings-structure.ts:409` |
| home | `connectivity.customCertificateDomain` | text;  | Owner phone/tablet | `settings-structure.ts:410` |
| home | `connectivity.customCertificatePath` | text;  | Owner phone/tablet | `settings-structure.ts:411` |
| home | `connectivity.customCertificateKeyPath` | text;  | Owner phone/tablet | `settings-structure.ts:412` |
| retention | `runtime.diagnosticDays` | number; min 1, max 30 | Owner phone/tablet | `settings-structure.ts:419` |
| retention | `runtime.notificationDays` | number; min 1, max 180 | Owner phone/tablet | `settings-structure.ts:420` |
| retention | `runtime.playHistoryDays` | number; min 1, max 36500 | Owner phone/tablet | `settings-structure.ts:422` |
| retention | `runtime.jobDays` | number; min 1, max 30 | Owner phone/tablet | `settings-structure.ts:423` |
| retention | `maintenance.backupKeepCount` | number; min 1, max 365 | Owner phone/tablet | `settings-structure.ts:424` |
| schedule | `maintenance.backgroundTaskPriority` | choice;  | Owner phone/tablet | `settings-structure.ts:435` |
| general | `runtime.name` | text;  | Owner phone/tablet | `settings-structure.ts:450` |
| alerts | `alerts.storageWarningPercent` | number; min 1, max 50 | Owner phone/tablet | `settings-structure.ts:455` |
| alerts | `alerts.storageCriticalPercent` | number; min 1, max 50 | Owner phone/tablet | `settings-structure.ts:456` |
| alerts | `alerts.certificateWarningDays` | number; min 1, max 60 | Owner phone/tablet | `settings-structure.ts:457` |

Owner custom panels, each separately authored rather than automatically emitted from a registry: now playing, alerts, health, activity, statistics, full play history, libraries, live sources, tuners, recording rules, recording storage, library channels, accounts, invitations, API keys, transcode status, remote status, account connection, certificate status, advertised addresses, disk usage, retention, remote sources, backups, deleted titles, job schedules/windows, jobs, detailed diagnostics window, logs, records, capabilities, support export, feedback, state-folder details, server identity and updates. Sources: `C/presentation/settings-structure.ts:303–453`; native implementations under `N/src/screens/server/` and `N/src/screens/settings/ServerForms.tsx`/`ServerPage.tsx`. The Dashboard is not the entire console.

### Portico custom-panel editable controls (nested owner/profile/editor forms)

These are user-facing controls authored inside native panels, in addition to the shared `server-setting` rows. The label IDs retain exact vocabulary; fields rendered from dynamic server schemas, library names or task names are described after the table. Owner server panels are phone/tablet-only except Dashboard; profile/security/editor gates depend on the current account/resource. A label sample does not establish mutation correctness; only listed B findings received full defect tracing.

| Native panel | Editable label/choice IDs | Control type | Evidence |
|---|---|---|---|
| `InviteSheet.tsx` | `invite.link` | Field | `N/src/screens/InviteSheet.tsx:45` |
| `InviteSheet.tsx` | `web.directJoin.code` | Field | `N/src/screens/InviteSheet.tsx:46` |
| `InviteSheet.tsx` | `invite.label` | Field | `N/src/screens/InviteSheet.tsx:51` |
| `MetadataEditor.tsx` | `profile.name` | Field | `N/src/screens/MetadataEditor.tsx:282` |
| `MetadataEditor.tsx` | `web.metadata.department` | Field | `N/src/screens/MetadataEditor.tsx:283` |
| `MetadataEditor.tsx` | `web.metadata.addGenre`, `web.metadata.addPerson` | Field | `N/src/screens/MetadataEditor.tsx:288` |
| `MetadataEditor.tsx` | `web.metadata.searchProvider` | Field | `N/src/screens/MetadataEditor.tsx:353` |
| `MetadataEditor.tsx` | `web.metadata.addGenres` | Field | `N/src/screens/MetadataEditor.tsx:483` |
| `Profiles.tsx` | `profile.name` | Field | `N/src/screens/Profiles.tsx:175` |
| `Recordings.tsx` | `recordings.upcoming`, `recordings.recorded`, `recordings.rules`, `recordings.storage` | Segmented | `N/src/screens/Recordings.tsx:165` |
| `Recordings.tsx` | `recordings.editRule` | Segmented | `N/src/screens/Recordings.tsx:323` |
| `Recordings.tsx` | `recordings.keepUpcoming`, `recordings.cancelUpcoming` | Segmented | `N/src/screens/Recordings.tsx:360` |
| `Security.tsx` | `limits.allowUnrated` | ToggleRow | `N/src/screens/Security.tsx:87` |
| `Security.tsx` | `auth.password` | Field | `N/src/screens/Security.tsx:197` |
| `Security.tsx` | `auth.code` | Field | `N/src/screens/Security.tsx:204` |
| `Security.tsx` | `account.currentPassword` | Field | `N/src/screens/Security.tsx:214` |
| `Security.tsx` | `profile.pinRecoveryCode` | Field | `N/src/screens/Security.tsx:215` |
| `Security.tsx` | `auth.newPassword` | Field | `N/src/screens/Security.tsx:268` |
| `Security.tsx` | `account.confirmPassword` | Field | `N/src/screens/Security.tsx:269` |
| `Dashboard.tsx` | `web.nowPlaying.messageLabel` | Field | `N/src/screens/server/Dashboard.tsx:54` |
| `Dashboard.tsx` | `web.telemetry.period` | Segmented | `N/src/screens/server/Dashboard.tsx:129` |
| `LibrariesPanel.tsx` | `web.libraries.type` | PickRow | `N/src/screens/server/LibrariesPanel.tsx:127` |
| `LibrariesPanel.tsx` | `profile.name` | Field | `N/src/screens/server/LibrariesPanel.tsx:129` |
| `LibrariesPanel.tsx` | `server.libraries.lookUpOnline` | ToggleRow | `N/src/screens/server/LibrariesPanel.tsx:134` |
| `LibrariesPanel.tsx` | `web.metadataSource.title` | PickRow | `N/src/screens/server/LibrariesPanel.tsx:232` |
| `LibrariesPanel.tsx` | `web.librarySettings.source` | PickRow | `N/src/screens/server/LibrariesPanel.tsx:237` |
| `LibrariesPanel.tsx` | `web.libraries.scanDepth` | PickRow | `N/src/screens/server/LibrariesPanel.tsx:252` |
| `LibrariesPanel.tsx` | `server.library.rescan` | PickRow | `N/src/screens/server/LibrariesPanel.tsx:253` |
| `LibrariesPanel.tsx` | `web.librarySettings.allowDelete` | ToggleRow | `N/src/screens/server/LibrariesPanel.tsx:261` |
| `LibrariesPanel.tsx` | `web.librarySettings.keepDeleted` | TypedRow | `N/src/screens/server/LibrariesPanel.tsx:262` |
| `LivePanels.tsx` | `web.libraries.kind` | Segmented | `N/src/screens/server/LivePanels.tsx:125` |
| `LivePanels.tsx` | `profile.name` | Field | `N/src/screens/server/LivePanels.tsx:126` |
| `LivePanels.tsx` | `web.liveTv.tunerAddress`, `web.liveTv.serverAddress`, `web.liveTv.playlistUrl` | Field | `N/src/screens/server/LivePanels.tsx:127` |
| `LivePanels.tsx` | `web.liveTv.guideUrl` | Field | `N/src/screens/server/LivePanels.tsx:128` |
| `LivePanels.tsx` | `web.direct.username` | Field | `N/src/screens/server/LivePanels.tsx:129` |
| `LivePanels.tsx` | `web.account.password` | Field | `N/src/screens/server/LivePanels.tsx:130` |
| `LivePanels.tsx` | `web.liveTv.whoCanWatch` | PickRow | `N/src/screens/server/LivePanels.tsx:132` |
| `LivePanels.tsx` | `web.liveTv.refreshGuide` | PickRow | `N/src/screens/server/LivePanels.tsx:133` |
| `LivePanels.tsx` | `web.liveSource.provides` | PickRow | `N/src/screens/server/LivePanels.tsx:199` |
| `LivePanels.tsx` | `web.liveSource.guideFrom` | PickRow | `N/src/screens/server/LivePanels.tsx:200` |
| `LivePanels.tsx` | `server.live.filter.mode` | PickRow | `N/src/screens/server/LivePanels.tsx:207` |
| `LivePanels.tsx` | `web.liveSource.numbering` | PickRow | `N/src/screens/server/LivePanels.tsx:214` |
| `LivePanels.tsx` | `web.liveSource.startAt` | TypedRow | `N/src/screens/server/LivePanels.tsx:215` |
| `LivePanels.tsx` | `web.liveSource.countBy` | TypedRow | `N/src/screens/server/LivePanels.tsx:216` |
| `LivePanels.tsx` | `web.liveSource.bringLogos` | ToggleRow | `N/src/screens/server/LivePanels.tsx:221` |
| `LivePanels.tsx` | `web.liveSource.replaceLogos` | ToggleRow | `N/src/screens/server/LivePanels.tsx:222` |
| `LivePanels.tsx` | `web.liveSource.streamBuffer` | TypedRow | `N/src/screens/server/LivePanels.tsx:227` |
| `LivePanels.tsx` | `web.liveSource.retryFor` | TypedRow | `N/src/screens/server/LivePanels.tsx:228` |
| `LivePanels.tsx` | `web.shell.profile` | PickRow | `N/src/screens/server/LivePanels.tsx:338` |
| `LivePanels.tsx` | `web.dvrSettings.allowRecording` | ToggleRow | `N/src/screens/server/LivePanels.tsx:339` |
| `LivePanels.tsx` | `web.dvrSettings.inheritRow` | ToggleRow | `N/src/screens/server/LivePanels.tsx:340` |
| `LivePanels.tsx` | `web.liveTv.keepRecordings` | TypedRow | `N/src/screens/server/LivePanels.tsx:371` |
| `LivePanels.tsx` | `web.liveTv.episodesPerSeries` | TypedRow | `N/src/screens/server/LivePanels.tsx:372` |
| `LivePanels.tsx` | `web.liveTv.leaveFree` | TypedRow | `N/src/screens/server/LivePanels.tsx:373` |
| `LivePanels.tsx` | `web.liveTv.capUpTo` | TypedRow | `N/src/screens/server/LivePanels.tsx:374` |
| `PeoplePanels.tsx` | `web.people.allLibrariesLater` | ToggleRow | `N/src/screens/server/PeoplePanels.tsx:61` |
| `PeoplePanels.tsx` | `web.access.roleLabel` | PickRow | `N/src/screens/server/PeoplePanels.tsx:109` |
| `PeoplePanels.tsx` | `web.people.disableAccount` | ToggleRow | `N/src/screens/server/PeoplePanels.tsx:110` |
| `PeoplePanels.tsx` | `auth.newPassword` | Field | `N/src/screens/server/PeoplePanels.tsx:113` |
| `PeoplePanels.tsx` | `web.access.maxStreams` | TypedRow | `N/src/screens/server/PeoplePanels.tsx:117` |
| `PeoplePanels.tsx` | `web.access.remoteQuality` | TypedRow | `N/src/screens/server/PeoplePanels.tsx:118` |
| `PeoplePanels.tsx` | `web.access.highestRating` | PickRow | `N/src/screens/server/PeoplePanels.tsx:119` |
| `PeoplePanels.tsx` | `limits.allowUnrated` | ToggleRow | `N/src/screens/server/PeoplePanels.tsx:120` |
| `PeoplePanels.tsx` | `web.limits.hiddenLabels` | TypedRow | `N/src/screens/server/PeoplePanels.tsx:121` |
| `PeoplePanels.tsx` | `web.direct.username` | Field | `N/src/screens/server/PeoplePanels.tsx:150` |
| `PeoplePanels.tsx` | `web.people.profileName` | Field | `N/src/screens/server/PeoplePanels.tsx:151` |
| `PeoplePanels.tsx` | `auth.password` | Field | `N/src/screens/server/PeoplePanels.tsx:152` |
| `PeoplePanels.tsx` | `profile.name` | Field | `N/src/screens/server/PeoplePanels.tsx:248` |
| `PeoplePanels.tsx` | `web.access.scopeLabel` | PickRow | `N/src/screens/server/PeoplePanels.tsx:249` |
| `SchedulePanels.tsx` | `web.maintenance.windowName` | TypedRow | `N/src/screens/server/SchedulePanels.tsx:29` |
| `SchedulePanels.tsx` | `web.dvr.on` | ToggleRow | `N/src/screens/server/SchedulePanels.tsx:30` |
| `SchedulePanels.tsx` | `web.maintenance.days` | PickRow | `N/src/screens/server/SchedulePanels.tsx:31` |
| `SchedulePanels.tsx` | `web.maintenance.startsAt` | TypedRow | `N/src/screens/server/SchedulePanels.tsx:32` |
| `SchedulePanels.tsx` | `web.maintenance.hours` | TypedRow | `N/src/screens/server/SchedulePanels.tsx:33` |
| `SchedulePanels.tsx` | `web.schedules.enabled` | ToggleRow | `N/src/screens/server/SchedulePanels.tsx:85` |
| `SchedulePanels.tsx` | `web.schedules.windowStarts` | Field | `N/src/screens/server/SchedulePanels.tsx:86` |
| `SchedulePanels.tsx` | `web.schedules.windowLength` | Field | `N/src/screens/server/SchedulePanels.tsx:87` |
| `SchedulePanels.tsx` | `web.schedules.catchUpRow` | ToggleRow | `N/src/screens/server/SchedulePanels.tsx:88` |
| `StoragePanels.tsx` | `web.maintenance.olderThan` | Field | `N/src/screens/server/StoragePanels.tsx:50` |
| `StoragePanels.tsx` | `settings.server.retention.logs` | TypedRow | `N/src/screens/server/StoragePanels.tsx:71` |
| `StoragePanels.tsx` | `web.maintenance.restoreFromFile` | TypedRow | `N/src/screens/server/StoragePanels.tsx:155` |
| `StoragePanels.tsx` | `profile.name` | Field | `N/src/screens/server/StoragePanels.tsx:277` |
| `StoragePanels.tsx` | `web.claim.address` | Field | `N/src/screens/server/StoragePanels.tsx:278` |
| `StoragePanels.tsx` | `web.direct.username` | Field | `N/src/screens/server/StoragePanels.tsx:279` |
| `StoragePanels.tsx` | `web.account.password` | Field | `N/src/screens/server/StoragePanels.tsx:280` |
| `StoragePanels.tsx` | `web.storage.insecureLocal` | ToggleRow | `N/src/screens/server/StoragePanels.tsx:281` |
| `StoragePanels.tsx` | `web.storage.rcloneBinary` | Field | `N/src/screens/server/StoragePanels.tsx:297` |
| `StoragePanels.tsx` | `web.storage.rcloneRemoteName` | Field | `N/src/screens/server/StoragePanels.tsx:298` |
| `StoragePanels.tsx` | `web.storage.rcloneConfig` | Field | `N/src/screens/server/StoragePanels.tsx:299` |
| `Troubleshooting.tsx` | `settings.server.detailWindow` | ToggleRow | `N/src/screens/server/Troubleshooting.tsx:26` |
| `Troubleshooting.tsx` | `web.logs.level` | PickRow | `N/src/screens/server/Troubleshooting.tsx:50` |
| `Troubleshooting.tsx` | `feedback.category` | PickRow | `N/src/screens/server/Troubleshooting.tsx:51` |
| `Troubleshooting.tsx` | `web.logs.records` | Segmented | `N/src/screens/server/Troubleshooting.tsx:86` |
| `Troubleshooting.tsx` | `web.logs.timeWindow` | PickRow | `N/src/screens/server/Troubleshooting.tsx:138` |
| `Troubleshooting.tsx` | `web.feedback.replyLabel` | Field | `N/src/screens/server/Troubleshooting.tsx:189` |
| `Troubleshooting.tsx` | `web.broadcast.title` | Field | `N/src/screens/server/Troubleshooting.tsx:219` |
| `Troubleshooting.tsx` | `web.broadcast.message` | Field | `N/src/screens/server/Troubleshooting.tsx:220` |
| `Troubleshooting.tsx` | `web.broadcast.adminOnly` | ToggleRow | `N/src/screens/server/Troubleshooting.tsx:222` |
| `Troubleshooting.tsx` | `web.broadcast.importance` | PickRow | `N/src/screens/server/Troubleshooting.tsx:223` |

Dynamic controls also matter: per-library allowed access; classification/health/source selection; library scan tier plus current custom policy, per-source rescan cadence; one metadata agent/provider per library media kind; member/admin role, disable account, password and content rating/unrated limits; recording grants by profile, inherit policy, recording rule options; channel-source filter include/exclude values/numbering/logo import; jobs/windows task selection; user-defined API-key scope; remote-source credentials/paths; metadata editor’s server-defined fields/locks/artwork candidate roles and relationship order. Examples: `N/src/screens/server/LibrariesPanel.tsx:232`, `:252–263`, `PeoplePanels.tsx:109–120`, `LivePanels.tsx:199–222`, `SchedulePanels.tsx:30–34`, `MetadataEditor.tsx:221–250`. Exact backend validation, secret handling and administrative authorization are not audited in this UX domain.

### Plezy: rendered preference bindings, including specialist settings

The table is extracted from actual UI `SettingsService` bindings in settings/download/profile screens, not from every string in the catalogue. A binding may repeat in multiple controls; first evidence is shown. A row whose control is platform gated is not universal. Key gates: cellular/gestures are mobile; TV-only glow/full cards/corner backdrop; season-poster tabs and Home hero toggle non-TV; visual-effects selector Android; engine/HDR/display controls vary by OS/player; shortcut editor only supported keyboard platforms; custom download folder only where picker exists; Discord desktop. Account preference rows are upstream account state, not local settings. Current source does not establish a customizable accent or a dedicated Plezy screensaver.

| User-facing binding | Portico native counterpart or gap | First UI evidence |
|---|---|---|
| `alwaysKeepSidebarOpen` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:137` |
| `appLocale` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/general_settings_screen.dart:91` |
| `audioChannelLimit` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:43` |
| `audioDownmixNormalize` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:545` |
| `audioNormalization` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:506` |
| `audioPassthrough` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:510` |
| `autoCheckUpdatesOnStartup` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/settings_screen.dart:632` |
| `autoHidePerformanceOverlay` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/settings_screen.dart:556` |
| `autoPip` | Native PiP affordance; no equivalent standing switch established | `plezy/lib/screens/settings/playback_settings_screen.dart:458` |
| `autoPlayNextEpisode` | playback.autoplayNext | `plezy/lib/screens/settings/playback_settings_screen.dart:305` |
| `autoRemoveWatchedDownloads` | Downloads watched removal | `plezy/lib/screens/settings/settings_screen.dart:432` |
| `autoSkipDelay` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:346` |
| `automotiveUiScale` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:240` |
| `boxFitScope` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:209` |
| `cellularQualityPreset` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:659` |
| `clickVideoTogglesPlayback` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:276` |
| `continueWatchingAction` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:304` |
| `crashReporting` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/settings_screen.dart:542` |
| `creditsPattern` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:361` |
| `customExternalPlayers` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/external_player_screen.dart:44` |
| `customRelayUrl` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/settings_screen.dart:954` |
| `defaultCreditsPattern` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:365` |
| `defaultIntroPattern` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:358` |
| `defaultQualityPreset` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:649` |
| `deinterlace` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:496` |
| `directPlayCoveredQuality` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:674` |
| `disableDolbyVision` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:44` |
| `displaySwitchDelay` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:563` |
| `downloadOnWifiOnly` | Downloads Wi-Fi-only | `plezy/lib/screens/settings/settings_screen.dart:425` |
| `downmixCenterBoost` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:536` |
| `dvConversionMode` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:586` |
| `enableCompanionRemoteServer` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/settings_screen.dart:517` |
| `enableDebugLogging` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/settings_screen.dart:549` |
| `enableDiscordRPC` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/services_settings_screen.dart:48` |
| `enableHardwareDecoding` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:451` |
| `enableTraktWatchedSync` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/trakt_settings_screen.dart:78` |
| `episodeAction` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:314` |
| `episodePosterMode` | Fixed poster inheritance; no choice | `plezy/lib/screens/settings/appearance_settings_screen.dart:293` |
| `exitFullscreenOnPlayerClose` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:283` |
| `focusGlow` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:90` |
| `followServerTrackSelections` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:250` |
| `forceSkipMarkerFallback` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:340` |
| `forceTvMode` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/general_settings_screen.dart:48` |
| `gestureBrightnessSwipe` | Gap: no gesture editor | `plezy/lib/screens/settings/playback_settings_screen.dart:394` |
| `gesturePinchToZoom` | Gap: no gesture editor | `plezy/lib/screens/settings/playback_settings_screen.dart:413` |
| `gestureVolumeSwipe` | Gap: no gesture editor | `plezy/lib/screens/settings/playback_settings_screen.dart:407` |
| `gridSpacing` | Gap: fixed spacing | `plezy/lib/screens/settings/appearance_settings_screen.dart:282` |
| `groupLibrariesByServer` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:144` |
| `hdrSdrConversion` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:603` |
| `hideSpoilers` | Gap: no spoiler switch/consumer established | `plezy/lib/screens/settings/appearance_settings_screen.dart:63` |
| `introPattern` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:354` |
| `isValidSkipPattern` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/settings_utils.dart:514` |
| `libraryDensity` | Gap: fixed native card metrics | `plezy/lib/screens/settings/appearance_settings_screen.dart:201` |
| `liveTvDefaultFavorites` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:169` |
| `matchContentFrameRate` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:41` |
| `matchContentResolution` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:42` |
| `matchDynamicRange` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:40` |
| `matchRefreshRate` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:39` |
| `maxVolume` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:552` |
| `mpvConfigText` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/mpv_config_screen.dart:40` |
| `mpvPresets` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/mpv_config_screen.dart:358` |
| `musicQualityPreset` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:700` |
| `playNextCountdown` | playback.upNextCountdownSeconds | `plezy/lib/screens/settings/playback_settings_screen.dart:317` |
| `playbackBufferTier` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:633` |
| `playbackSpeedScope` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:199` |
| `refusedVideoCodecs` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:681` |
| `rememberBrightnessLevel` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:401` |
| `rememberTrackSelections` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:244` |
| `requireProfileSelectionOnOpen` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/general_settings_screen.dart:41` |
| `resumeMusicOnLaunch` | Different persistent player state; no comparable switch established | `plezy/lib/screens/settings/playback_settings_screen.dart:256` |
| `rewindOnResume` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:177` |
| `scrobblePref` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/tracker_settings_screen.dart:115` |
| `seekTimeLarge` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:169` |
| `seekTimeSmall` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:161` |
| `selectedExternalPlayer` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/external_player_screen.dart:43` |
| `shaderPresetScope` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:204` |
| `showChapterMarkersOnTimeline` | No comparable standing switch established | `plezy/lib/screens/settings/playback_settings_screen.dart:262` |
| `showEpisodeNumberOnCards` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:50` |
| `showExploreTab` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:130` |
| `showHeroSection` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:103` |
| `showNavBarLabels` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:151` |
| `showSeasonPostersOnTabs` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:57` |
| `showServerNameOnHubs` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:117` |
| `showUnwatchedCount` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:157` |
| `showWatchedIndicators` | Fixed checkmark rule; no native switch | `plezy/lib/screens/settings/appearance_settings_screen.dart:69` |
| `shuffleStartsFromBeginning` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:311` |
| `skipCreditsMode` | playback.creditsSkip | `plezy/lib/screens/settings/playback_settings_screen.dart:333` |
| `skipIntroMode` | playback.introSkip | `plezy/lib/screens/settings/playback_settings_screen.dart:326` |
| `sleepTimerDuration` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:185` |
| `specialsOrdering` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:268` |
| `startInFullscreen` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/general_settings_screen.dart:61` |
| `startupSection` | No native startup destination switch established | `plezy/lib/screens/settings/general_settings_screen.dart:110` |
| `subAssOverride` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:54` |
| `subtitleAnchorToScreen` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:118` |
| `subtitleBackgroundColor` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:167` |
| `subtitleBackgroundOpacity` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:159` |
| `subtitleBold` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:124` |
| `subtitleBorderColor` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:148` |
| `subtitleBorderSize` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:140` |
| `subtitleFontSize` | Native style control gap | `plezy/lib/screens/settings/subtitle_styling_screen.dart:89` |
| `subtitleItalic` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:129` |
| `subtitlePosition` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:102` |
| `subtitleRenderResolution` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:63` |
| `subtitleTextColor` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:97` |
| `subtitleUseMargins` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/subtitle_styling_screen.dart:111` |
| `syncOffsetScope` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:214` |
| `themeMode` | Gap: fixed native theme (deliberate) | `plezy/lib/screens/settings/appearance_settings_screen.dart:187` |
| `trackerFilterIdsPref` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/tracker_account_settings_body.dart:69` |
| `trackerFilterModePref` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/tracker_account_settings_body.dart:69` |
| `tunneledPlayback` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:572` |
| `tvCornerSpotlightBackdrop` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:83` |
| `tvFullCardLayout` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:76` |
| `useExoPlayer` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/playback_settings_screen.dart:38` |
| `useExternalPlayer` | Gap: integrated player only | `plezy/lib/screens/settings/external_player_screen.dart:33` |
| `useGlobalHubs` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:111` |
| `validCustomPlayerFields` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/external_player_screen.dart:201` |
| `videoPlayerNavigationEnabled` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/settings_screen.dart:508` |
| `viewMode` | Gap: native grid only; web has view choice | `plezy/lib/screens/settings/appearance_settings_screen.dart:272` |
| `visualEffects` | No matching native setting established; see grouped comparison | `plezy/lib/screens/settings/appearance_settings_screen.dart:330` |

Additional Plezy settings that are not simple local `SettingsService` constants: upstream preferred audio/subtitle language, choose audio by language, subtitle mode (manual/default/always/forced/smart), SDH and forced preference, missing episodes, hide played in latest, collections view, rewatch in Next Up, watched-indicator/review visibility; per-profile name/PIN/default/remove/borrow connection; URL lists; Seerr instance/auth/request destinations; tracker scrobbling/watched sync and allow/block library filters; mpv.conf line/preset editing; imported shader choices; external-player custom package/scheme/command; hotkey recording/reset; download rule keep-N/unwatched/specials/delete-associated; log actions; export/import/reset/cache clear and donation/update actions. Exact labels and intended semantics: `plezy/lib/i18n/en.i18n.json:954`, `:1753`, `:2038`, `:2082`, `:2187`. These are capabilities to consider, not proof every backend supports every option.

### Swiftfin: settings bound in the shared and platform settings views

`@Default` is local defaults storage; `@StoredValue(.User...)` may represent selected user-scoped storage and is not automatically Jellyfin server configuration. iOS-only gates apply to gesture settings/appearance/admin Dashboard; shared pickers adapt to tvOS. Below is every binding found in the inspected settings-view set, including the gesture editor and App Settings. The binding is retained because several controls have compound configuration objects (poster/subtitle), expanded below.

| Binding / configuration | Portico native counterpart or gap | First settings evidence |
|---|---|---|
| `.Customization.Home.maxNextUp` | Server Home customization; different semantics | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/HomeSection.swift:21` |
| `.Customization.Home.resumeNextUp` | Server Home customization; different semantics | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/HomeSection.swift:23` |
| `.Customization.Home.showRecentlyAdded` | Server Home customization; different semantics | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/HomeSection.swift:17` |
| `.Customization.Home.showRecentlyPlayed` | Server Home customization; different semantics | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/HomeSection.swift:19` |
| `.Customization.Library.enabledDrawerFilters` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/LibrarySection.swift:24` |
| `.Customization.Library.letterPickerOrientation` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/LibrarySection.swift:30` |
| `.Customization.Library.randomImage` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/LibrarySection.swift:26` |
| `.Customization.Library.rememberLayout` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/LibrarySection.swift:33` |
| `.Customization.Library.rememberSort` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/LibrarySection.swift:35` |
| `.Customization.Library.showFavorites` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/LibrarySection.swift:22` |
| `.Customization.Library.style` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/LibrarySection.swift:28` |
| `.Customization.Poster.configuration` | No same native control established | `swiftfin/Shared/Views/SettingsView/IndicatorSettingsView.swift:14` |
| `.Customization.Search.enabledDrawerFilters` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/CustomizeSettingsView.swift:14` |
| `.Customization.itemBarActionButtons` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/ItemSection.swift:28` |
| `.Customization.itemMenuActionButtons` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/ItemSection.swift:30` |
| `.Customization.itemViewType` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/ItemSection.swift:22` |
| `.Customization.shouldShowMissingEpisodes` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/ItemSection.swift:37` |
| `.Customization.shouldShowMissingSeasons` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/ItemSection.swift:35` |
| `.Customization.shouldShowRecommendations` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/ItemSection.swift:33` |
| `.Customization.tabBarPlacement` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/CustomizeSettingsView.swift:18` |
| `.Experimental.mpvPlayer` | No same native control established | `swiftfin/Shared/Views/SettingsView/ExperimentalSettingsView.swift:18` |
| `.Experimental.serverConnectionAutoSwitch` | No same native control established | `swiftfin/Shared/Views/SettingsView/ExperimentalSettingsView.swift:20` |
| `.Experimental.videoPlayerEPG` | No same native control established | `swiftfin/Shared/Views/SettingsView/ExperimentalSettingsView.swift:22` |
| `.Server.servers` | No same native control established | `swiftfin/Shared/Views/AppSettingsView.swift:36` |
| `.User.customDeviceProfiles` | No same native control established | `swiftfin/Shared/Views/SettingsView/DeviceProfileSettings/EditDeviceProfileView.swift:15` |
| `.User.enabledTrailers` | No same native control established | `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/ItemSection.swift:25` |
| `.User.forceDVTranscode` | No same native control established | `swiftfin/Shared/Views/SettingsView/PlaybackQualitySettingsView.swift:30` |
| `.User.forceHDRTranscode` | No same native control established | `swiftfin/Shared/Views/SettingsView/PlaybackQualitySettingsView.swift:32` |
| `.User.forceSubtitleBurnIn` | Audio/subtitle language and in-player options; standing style gap | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:59` |
| `.User.previewImageScrubbing` | No same native control established | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:47` |
| `.VideoPlayer.Gesture.doubleTouchGesture` | Gap: no gesture editor | `swiftfin/Swiftfin/Views/GestureSettingsView.swift:29` |
| `.VideoPlayer.Gesture.horizontalPanAction` | Gap: no gesture editor | `swiftfin/Swiftfin/Views/GestureSettingsView.swift:19` |
| `.VideoPlayer.Gesture.horizontalSwipeAction` | Gap: no gesture editor | `swiftfin/Swiftfin/Views/GestureSettingsView.swift:21` |
| `.VideoPlayer.Gesture.longPressAction` | Gap: no gesture editor | `swiftfin/Swiftfin/Views/GestureSettingsView.swift:23` |
| `.VideoPlayer.Gesture.longPressSpeedMultiplier` | Gap: no gesture editor | `swiftfin/Swiftfin/Views/GestureSettingsView.swift:25` |
| `.VideoPlayer.Gesture.multiTapGesture` | Gap: no gesture editor | `swiftfin/Swiftfin/Views/GestureSettingsView.swift:27` |
| `.VideoPlayer.Gesture.pinchGesture` | Gap: no gesture editor | `swiftfin/Swiftfin/Views/GestureSettingsView.swift:31` |
| `.VideoPlayer.Gesture.verticalPanLeftAction` | Gap: no gesture editor | `swiftfin/Swiftfin/Views/GestureSettingsView.swift:33` |
| `.VideoPlayer.Gesture.verticalPanRightAction` | Gap: no gesture editor | `swiftfin/Swiftfin/Views/GestureSettingsView.swift:35` |
| `.VideoPlayer.Overlay.chapterSlider` | No same native control established | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:45` |
| `.VideoPlayer.Overlay.trailingTimestampType` | No same native control established | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:64` |
| `.VideoPlayer.Playback.appMaximumBitrate` | No same native control established | `swiftfin/Shared/Views/SettingsView/PlaybackQualitySettingsView.swift:23` |
| `.VideoPlayer.Playback.appMaximumBitrateTest` | No same native control established | `swiftfin/Shared/Views/SettingsView/PlaybackQualitySettingsView.swift:25` |
| `.VideoPlayer.Playback.appMaximumResolution` | No same native control established | `swiftfin/Shared/Views/SettingsView/PlaybackQualitySettingsView.swift:21` |
| `.VideoPlayer.Playback.compatibilityMode` | No same native control established | `swiftfin/Shared/Views/SettingsView/PlaybackQualitySettingsView.swift:27` |
| `.VideoPlayer.Playback.customDeviceProfileAction` | No same native control established | `swiftfin/Shared/Views/SettingsView/DeviceProfileSettings/CustomDeviceProfilesView.swift:15` |
| `.VideoPlayer.Subtitle.configuration` | Audio/subtitle language and in-player options; standing style gap | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:57` |
| `.VideoPlayer.barActionButtons` | No same native control established | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:33` |
| `.VideoPlayer.jumpBackwardInterval` | Back skip setting | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:29` |
| `.VideoPlayer.jumpForwardInterval` | Forward skip setting | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:31` |
| `.VideoPlayer.menuActionButtons` | No same native control established | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:35` |
| `.VideoPlayer.resumeOffset` | No same native control established | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:40` |
| `.VideoPlayer.supplements` | No same native control established | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:52` |
| `.VideoPlayer.videoPlayerType` | No same native control established | `swiftfin/Shared/Views/SettingsView/VideoPlayerSettingsView.swift:24` |
| `.appAppearance` | Gap: fixed native palette | `swiftfin/Shared/Views/AppSettingsView.swift:29` |
| `.backgroundSignOutInterval` | No same native control established | `swiftfin/Shared/Views/AppSettingsView.swift:21` |
| `.selectUserAllServersSplashscreen` | No same native control established | `swiftfin/Shared/Views/AppSettingsView.swift:18` |
| `.selectUserUseSplashscreen` | No same native control established | `swiftfin/Shared/Views/AppSettingsView.swift:16` |
| `.sendProgressReports` | No same native control established | `swiftfin/Shared/Views/SettingsView/DebugSettingsView.swift:20` |
| `.signOutOnBackground` | No same native control established | `swiftfin/Shared/Views/AppSettingsView.swift:23` |
| `.signOutOnClose` | No same native control established | `swiftfin/Shared/Views/AppSettingsView.swift:25` |
| `.userAccentColor` | Gap: fixed native palette | `swiftfin/Shared/Views/SettingsView/SettingsView.swift:21` |
| `.userAppearance` | Gap: fixed native palette | `swiftfin/Shared/Views/SettingsView/SettingsView.swift:17` |

Compound controls not to miss: poster title visibility/subtitle field/favorite/progress/played/unplayed-none-dot-count/series-thumb; item view style/trailer selection/recommendations/bar buttons/menu buttons/missing seasons/missing episodes; library favorites/random-image/filter-drawer selection/remember sort/remember layout/grid-list/poster type/list columns 1–4/letter orientation; gesture horizontal pan versus swipe exclusion, long press action and speed 0.1–10, multitap, double-touch, pinch, left/right vertical pans. See `swiftfin/Shared/Views/SettingsView/CustomizeSettings/Components/{PosterSection,ItemSection,LibrarySection,HomeSection}.swift:1`, `swiftfin/Swiftfin/Views/GestureSettingsView.swift:17`.

### jellyfin-androidtv: every preference used by settings screens

This table collects actual `rememberPreference` UI bindings across all settings screens. It excludes migration-only constants and last-played-channel housekeeping. Child selection screens repeat a setting rather than create a second one. Home section slots and library display preferences are included separately. The current checkout uses `ui/settings/screen`, rather than the prompt's older `ui/preference` path.

| UI preference | Portico native counterpart or gap | UI evidence |
|---|---|---|
| `alwaysAuthenticate` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/authentication/SettingsAuthenticationScreen.kt:118` |
| `appTheme` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/SettingsCustomizationScreen.kt:45` |
| `assDirectPlay` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:231` |
| `audioBehaviour` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:270` |
| `audioNightMode` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:281` |
| `autoLoginServerId` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/authentication/SettingsAuthenticationAutoSignInScreen.kt:40` |
| `autoLoginUserBehavior` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/authentication/SettingsAuthenticationAutoSignInScreen.kt:39` |
| `autoLoginUserId` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/authentication/SettingsAuthenticationAutoSignInScreen.kt:41` |
| `backdropBehavior` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/SettingsCustomizationBackdropScreen.kt:26` |
| `bufferLength` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:114` |
| `channelOrder` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideChannelOrderScreen.kt:26` |
| `cinemaModeEnabled` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackPrerollsScreen.kt:32` |
| `clockBehavior` | Gap: no player/browse clock policy | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/SettingsCustomizationClockScreen.kt:26` |
| `colorCodeGuide` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideOptionsScreen.kt:67` |
| `crashReportEnabled` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/SettingsTelemetryScreen.kt:32` |
| `crashReportIncludeLogs` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/SettingsTelemetryScreen.kt:46` |
| `debuggingEnabled` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/SettingsDeveloperScreen.kt:44` |
| `disableUiModeWarning` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/SettingsDeveloperScreen.kt:56` |
| `enableSmartScreen` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/library/SettingsLibrariesDisplayScreen.kt:92` |
| `favsAtTop` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideOptionsScreen.kt:56` |
| `gridDirection` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/library/SettingsLibrariesDisplayGridScreen.kt:31` |
| `homeNextUpMaxDays` | Server-authored rows; no equivalent user age cutoff row | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/home/SettingsHomeNextUpCutoffScreen.kt:41` |
| `imageType` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/library/SettingsLibrariesDisplayImageTypeScreen.kt:31` |
| `liveTvDirectPlayEnabled` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:257` |
| `liveTvGuideFilterKids` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideFiltersScreen.kt:28` |
| `liveTvGuideFilterMovies` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideFiltersScreen.kt:25` |
| `liveTvGuideFilterNews` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideFiltersScreen.kt:27` |
| `liveTvGuideFilterPremiere` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideFiltersScreen.kt:30` |
| `liveTvGuideFilterSeries` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideFiltersScreen.kt:26` |
| `liveTvGuideFilterSports` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideFiltersScreen.kt:29` |
| `maxBitrate` | Quality Home/Away network composites | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:127` |
| `mediaQueuingEnabled` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/nextup/SettingsPlaybackNextUpScreen.kt:50` |
| `nextUpBehavior` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/nextup/SettingsPlaybackNextUpBehaviorScreen.kt:26` |
| `nextUpTimeout` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/nextup/SettingsPlaybackNextUpScreen.kt:73` |
| `pgsDirectPlay` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:219` |
| `photoPlayerPresentationDelay` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackPhotoPlayerScreen.kt:44` |
| `playbackRewriteVideoEnabled` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackPlayerScreen.kt:44` |
| `playerZoomMode` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:188` |
| `posterSize` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/library/SettingsLibrariesDisplayImageSizeScreen.kt:31` |
| `preferExoPlayerFfmpeg` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:292` |
| `refreshRateSwitchingBehavior` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:139` |
| `resumeSubtractDuration` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:65` |
| `screensaverAgeRatingMax` | Gap: no app/dream screensaver settings | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/screensaver/SettingsScreensaverAgeRatingScreen.kt:25` |
| `screensaverAgeRatingRequired` | Gap: no app/dream screensaver settings | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/screensaver/SettingsScreensaverScreen.kt:61` |
| `screensaverInAppEnabled` | Gap: no app/dream screensaver settings | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/screensaver/SettingsScreensaverScreen.kt:35` |
| `screensaverInAppTimeout` | Gap: no app/dream screensaver settings | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/screensaver/SettingsScreensaverScreen.kt:47` |
| `seriesThumbnailsEnabled` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/SettingsCustomizationScreen.kt:89` |
| `showHDIndicator` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideOptionsScreen.kt:29` |
| `showLiveIndicator` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideOptionsScreen.kt:30` |
| `showNewIndicator` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideOptionsScreen.kt:31` |
| `showPremiereIndicator` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideOptionsScreen.kt:32` |
| `showRepeatIndicator` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/livetv/SettingsLiveTvGuideOptionsScreen.kt:33` |
| `softwareCodecsEnabled` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackCodecScreen.kt:78` |
| `sortBy` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/authentication/SettingsAuthenticationScreen.kt:82` |
| `stillWatchingBehavior` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackInactivityPromptScreen.kt:26` |
| `subtitleTextStrokeColor` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/subtitle/SettingsSubtitleTextStrokeColorScreen.kt:27` |
| `subtitlesBackgroundColor` | Standing native subtitle style gap; in-player tracks/timing exist | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/subtitle/SettingsSubtitlesBackgroundColorScreen.kt:27` |
| `subtitlesBurnDuringTranscode` | Standing native subtitle style gap; in-player tracks/timing exist | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:243` |
| `subtitlesOffsetPosition` | Standing native subtitle style gap; in-player tracks/timing exist | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/subtitle/SettingsSubtitlesScreen.kt:48` |
| `subtitlesTextColor` | Standing native subtitle style gap; in-player tracks/timing exist | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/subtitle/SettingsSubtitlesScreen.kt:45` |
| `subtitlesTextSize` | Standing native subtitle style gap; in-player tracks/timing exist | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/subtitle/SettingsSubtitlesScreen.kt:47` |
| `subtitlesTextWeight` | Standing native subtitle style gap; in-player tracks/timing exist | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/subtitle/SettingsSubtitlesScreen.kt:44` |
| `tvProviderEnabled` | Google-TV home integration exists; no native enable/disable setting | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/SettingsCustomizationScreen.kt:101` |
| `userAVCLevel` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAVCLevelScreen.kt:27` |
| `userHEVCLevel` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackCodecScreen.kt:49` |
| `videoStartDelay` | No same native setting established | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/playback/SettingsPlaybackAdvancedScreen.kt:150` |
| `watchedIndicatorBehavior` | Fixed watched check; no switch | `jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/ui/settings/screen/customization/SettingsCustomizationScreen.kt:67` |

Further settings represented outside direct `rememberPreference` names: ten configurable Home slots (`UserSettingPreferences.homesection0`–`9`), backward/forward seek length, per-library `posterSize`, `imageType`, `gridDirection`, `enableSmartScreen`, favorites/unwatched filters and sort direction; live guide channel sort/favorites-at-top/color-code/HD/live/new/premiere/repeat flags; automatic sign-in target/server/user chooser and account ordering/removal; external-player selection; media-segment action per segment type; photo slide interval. Defaults and scopes: `JTV/preference/UserSettingPreferences.kt:17`, `LibraryPreferences.kt:21`, `LiveTvPreferences.kt:16`, `ui/settings/screen/authentication/`.

### Wholphin: every named app preference in the authored preference layer

Wholphin's `AppPreference` declarations pair a title/default/getter/setter and validation with a settings UI component. The table indexes those declarations; experimental options remain experimental and not all are exposed in every release UI. Home row editing and user-server configuration add further choices. These are Android TV-specific; no phone/tablet UX is implied.

| Named preference | Portico native relation | Declaration evidence |
|---|---|---|
| `Ac3Supported` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:405` |
| `AdvancedSettings` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:593` |
| `Animate` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ScreensaverPreference.kt:103` |
| `AssSubtitleMode` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:434` |
| `AutoCheckForUpdates` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:564` |
| `AutoPlayNextDelay` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:292` |
| `AutoPlayNextUp` | Server Home layout; client content-size/rewatch policy differs | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:258` |
| `BackdropStylePref` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:812` |
| `CinemaMode` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:482` |
| `ClearImageCache` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:718` |
| `ControllerTimeout` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:120` |
| `CustomizeHome` | Server Home layout; client content-size/rewatch policy differs | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:733` |
| `DebugLogging` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:951` |
| `DecodeAv1` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:470` |
| `DimPercentage` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ScreensaverPreference.kt:191` |
| `DimToggle` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ScreensaverPreference.kt:179` |
| `DirectPlayDoviProfile7` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:459` |
| `DirectPlayPgs` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:447` |
| `DisableAudioOffload` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ExperimentalPreference.kt:75` |
| `DisplayTogglesPref` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:531` |
| `DownMixStereo` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:420` |
| `DpadSeekModePref` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:678` |
| `Duration` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ScreensaverPreference.kt:66` |
| `Enable` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ExperimentalPreference.kt:10` |
| `Enabled` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ScreensaverPreference.kt:9` |
| `ExoPlayerPreferences` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1190` |
| `ExoPlayerSettings` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:910` |
| `ExperimentalSettings` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ExperimentalPreference.kt:22` |
| `ExternalPlayerApp` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:899` |
| `FfmpegPreference` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:705` |
| `GlobalContentScale` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:692` |
| `HomeClickToPlay` | Server Home layout; client content-size/rewatch policy differs | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:220` |
| `HomePageItems` | Server Home layout; client content-size/rewatch policy differs | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:155` |
| `ImageDiskCacheSize` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:981` |
| `InstalledVersion` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:550` |
| `ItemTypes` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ScreensaverPreference.kt:159` |
| `LiveTvChannelSortByWatched` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1027` |
| `LiveTvColorCodePrograms` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1038` |
| `LiveTvFavoriteChannelsBeginning` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1016` |
| `LiveTvShowHeader` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1005` |
| `ManageMedia` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:825` |
| `MaxAge` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ScreensaverPreference.kt:117` |
| `MaxBitrate` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:363` |
| `MaxDaysNextUp` | Server Home layout; client content-size/rewatch policy differs | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:170` |
| `MaxResolutionPref` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:392` |
| `MpvConfFile` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:945` |
| `MpvGpuNext` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:933` |
| `MpvHardwareDecoding` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:922` |
| `MpvPreferences` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1205` |
| `MpvSettings` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:916` |
| `NavDrawerSwitchOnFocus` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:775` |
| `OneClickPause` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:837` |
| `OssLicenseInfo` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:587` |
| `PassOutProtection` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:318` |
| `PlayThemeMusic` | Gap: theme-song/display personalization | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:233` |
| `PlaybackDebugInfo` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:246` |
| `PlayerBackendPref` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:885` |
| `PreferAc3ForSurround` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ExperimentalPreference.kt:51` |
| `PreferDolbyVisionOverHdr10Plus` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ExperimentalPreference.kt:40` |
| `ProtectProfilePreference` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:976` |
| `QuickConnect` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1057` |
| `RefreshRateSwitching` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:855` |
| `RememberSelectedTab` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:494` |
| `ResolutionSwitching` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:870` |
| `RewatchNextUp` | Server Home layout; client content-size/rewatch policy differs | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:208` |
| `ScreensaverSettings` | Gap: no app screensaver | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1105` |
| `SeekBarSteps` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:141` |
| `SeerrIntegration` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1050` |
| `SendAppLogs` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:767` |
| `SendCrashReports` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:747` |
| `ShowClock` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:800` |
| `ShowLogos` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:519` |
| `ShowNextUpTiming` | Server Home layout; client content-size/rewatch policy differs | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:787` |
| `SignInAuto` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:964` |
| `SkipBack` | Skip intervals/segment choices; ranges differ | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:91` |
| `SkipBackOnResume` | Skip intervals/segment choices; ranges differ | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:270` |
| `SkipCommercials` | Skip intervals/segment choices; ranges differ | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:632` |
| `SkipForward` | Skip intervals/segment choices; ranges differ | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:62` |
| `SkipIntros` | Skip intervals/segment choices; ranges differ | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:606` |
| `SkipOutros` | Skip intervals/segment choices; ranges differ | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:619` |
| `SkipPreviews` | Skip intervals/segment choices; ranges differ | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:645` |
| `SkipRecaps` | Skip intervals/segment choices; ranges differ | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:658` |
| `SkipSegmentPreferences` | Skip intervals/segment choices; ranges differ | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1213` |
| `SkipSegments` | Skip intervals/segment choices; ranges differ | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:671` |
| `SlideshowDuration` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1065` |
| `SlideshowPlayVideos` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1093` |
| `Start` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ScreensaverPreference.kt:210` |
| `StartDelay` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ScreensaverPreference.kt:33` |
| `SubtitleStyle` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:849` |
| `ThemeColors` | Gap: theme-song/display personalization | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:506` |
| `Update` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:557` |
| `UpdateUrl` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:576` |
| `UserInterfaceLanguage` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:740` |
| `UserPinnedNavDrawerItems` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:725` |
| `UserProfileSettings` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:599` |
| `VideoTunneling` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ExperimentalPreference.kt:28` |
| `advancedPreferences` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1228` |
| `basicPreferences` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1113` |
| `bitrateValues` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:348` |
| `durationValues` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ScreensaverPreference.kt:59` |
| `experimentalPreferences` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ExperimentalPreference.kt:87` |
| `liveTvPreferences` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1331` |
| `screensaverPreferences` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/AppPreference.kt:1339` |
| `startDelayValues` | No same native user setting established | `wholphin/app/src/main/java/com/github/damontecres/wholphin/preferences/ScreensaverPreference.kt:22` |

Home customization additionally selects/hides/reorders rows and row display content, max items and combined continue watching/next up; Seerr connection/discovery/request configuration is separate. Server-user configuration covers language, subtitle mode, remembered tracks, autoplay, latest exclusions and media permissions (`W/preferences/UserPreferences.kt:16`, `ui/preferences/user/UserProfilePreferencesPage.kt`). Subtitle configuration has its own page. See architectural entries for the value of native TV-specific tuning without adopting arbitrary client whole-library work.

### Streamyfin: complete Settings type index and render evidence

This is an exhaustive index of the current Settings type. A stored model field is explicitly marked **model-only** when no reference in settings components or routes was found; those rows are not counted as proven visible controls. Compound objects (`home`, `libraryOptions`, per-title speed maps) have subchoices noted in the group comparison. Plugin-seeded/locked configuration differs from ordinary editable preferences. Reference evidence is static consumer text, not a tested navigation path.

| Setting key | Settings UI reference status | Portico native relation | Model source |
|---|---|---|---|
| `home` | UI reference `streamyfin/components/settings/VideoPlayerSelector.tsx:22` | No same native control established | `streamyfin/utils/atoms/settings.ts:406` |
| `deviceProfile` | Model-only in this inspected settings set | No same native control established | `streamyfin/utils/atoms/settings.ts:407` |
| `mediaListCollectionIds` | Model-only in this inspected settings set | No same native control established | `streamyfin/utils/atoms/settings.ts:408` |
| `preferedLanguage` | UI reference `streamyfin/components/settings/AppLanguageSelector.tsx:25` | Ordered preferred language rows / region locale | `streamyfin/utils/atoms/settings.ts:409` |
| `searchEngine` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/plugins/streamystats/page.tsx:41` | No same native control established | `streamyfin/utils/atoms/settings.ts:410` |
| `videoPlayer` | UI reference `streamyfin/components/settings/VideoPlayerSelector.tsx:48` | No same native control established | `streamyfin/utils/atoms/settings.ts:412` |
| `marlinServerUrl` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/plugins/marlin-search/page.tsx:29` | Gap: external client integration/settings | `streamyfin/utils/atoms/settings.ts:413` |
| `streamyStatsServerUrl` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/plugins/streamystats/page.tsx:37` | Gap: external client integration/settings | `streamyfin/utils/atoms/settings.ts:414` |
| `streamyStatsMovieRecommendations` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/plugins/streamystats/page.tsx:44` | Gap: external client integration/settings | `streamyfin/utils/atoms/settings.ts:415` |
| `streamyStatsSeriesRecommendations` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/plugins/streamystats/page.tsx:47` | Gap: external client integration/settings | `streamyfin/utils/atoms/settings.ts:416` |
| `streamyStatsPromotedWatchlists` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/plugins/streamystats/page.tsx:50` | Gap: external client integration/settings | `streamyfin/utils/atoms/settings.ts:417` |
| `downloadQuality` | Model-only in this inspected settings set | No same native control established | `streamyfin/utils/atoms/settings.ts:418` |
| `showDownloadLiveActivity` | UI reference `streamyfin/components/settings/AppearanceSettings.tsx:122` | No same native control established | `streamyfin/utils/atoms/settings.ts:420` |
| `defaultBitrate` | UI reference `streamyfin/components/settings/PlaybackControlsSettings.tsx:81` | No same native control established | `streamyfin/utils/atoms/settings.ts:421` |
| `libraryOptions` | Model-only in this inspected settings set | No same native control established | `streamyfin/utils/atoms/settings.ts:422` |
| `defaultAudioLanguage` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:109` | Ordered preferred language rows / region locale | `streamyfin/utils/atoms/settings.ts:423` |
| `playDefaultAudioTrack` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:320` | No same native control established | `streamyfin/utils/atoms/settings.ts:424` |
| `rememberAudioSelections` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:333` | No same native control established | `streamyfin/utils/atoms/settings.ts:425` |
| `defaultSubtitleLanguage` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:127` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:426` |
| `subtitleMode` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:133` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:427` |
| `rememberSubtitleSelections` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:428` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:428` |
| `subtitlesOnMute` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:442` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:430` |
| `subtitlesOnMuteAllowRestart` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:459` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:436` |
| `showHomeTitles` | Model-only in this inspected settings set | No same native control established | `streamyfin/utils/atoms/settings.ts:437` |
| `defaultVideoOrientation` | UI reference `streamyfin/components/settings/PlaybackControlsSettings.tsx:28` | No same native control established | `streamyfin/utils/atoms/settings.ts:438` |
| `forwardSkipTime` | UI reference `streamyfin/components/settings/MediaToggles.tsx:20` | No same native control established | `streamyfin/utils/atoms/settings.ts:439` |
| `rewindSkipTime` | UI reference `streamyfin/components/settings/MediaToggles.tsx:21` | No same native control established | `streamyfin/utils/atoms/settings.ts:440` |
| `showCustomMenuLinks` | UI reference `streamyfin/components/settings/AppearanceSettings.tsx:20` | No same native control established | `streamyfin/utils/atoms/settings.ts:441` |
| `disableHapticFeedback` | UI reference `streamyfin/components/settings/PlaybackControlsSettings.tsx:30` | No same native control established | `streamyfin/utils/atoms/settings.ts:442` |
| `subtitleSize` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:546` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:443` |
| `subtitleBackground` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:633` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:444` |
| `subtitleBackgroundOpacity` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:647` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:445` |
| `subtitleBackgroundPadding` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:666` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:446` |
| `subtitleFont` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:207` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:447` |
| `subtitleColor` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:509` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:448` |
| `subtitleMarginY` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:562` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:449` |
| `subtitleAlignX` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:226` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:450` |
| `subtitleAlignY` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:244` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:451` |
| `safeAreaInControlsEnabled` | UI reference `streamyfin/components/settings/PlaybackControlsSettings.tsx:29` | No same native control established | `streamyfin/utils/atoms/settings.ts:452` |
| `seerrServerUrl` | UI reference `streamyfin/components/settings/Seerr.tsx:42` | Gap: external client integration/settings | `streamyfin/utils/atoms/settings.ts:453` |
| `seerrApiKey` | UI reference `streamyfin/components/settings/Seerr.tsx:43` | Gap: external client integration/settings | `streamyfin/utils/atoms/settings.ts:455` |
| `autoLoginSeerr` | UI reference `streamyfin/components/settings/Seerr.tsx:256` | Gap: external client integration/settings | `streamyfin/utils/atoms/settings.ts:462` |
| `useKefinTweaks` | UI reference `streamyfin/components/settings/KefinTweaks.tsx:11` | No same native control established | `streamyfin/utils/atoms/settings.ts:463` |
| `hiddenLibraries` | UI reference `streamyfin/components/settings/AppearanceSettings.tsx:21` | No same native control established | `streamyfin/utils/atoms/settings.ts:464` |
| `enableH265ForChromecast` | UI reference `streamyfin/components/settings/ChromecastSettings.tsx:14` | No same native control established | `streamyfin/utils/atoms/settings.ts:465` |
| `stillWatchingPreset` | UI reference `streamyfin/components/settings/PlaybackControlsSettings.tsx:97` | No same native control established | `streamyfin/utils/atoms/settings.ts:467` |
| `autoPlayNextEpisode` | UI reference `streamyfin/components/settings/PlaybackControlsSettings.tsx:238` | playback.autoplayNext | `streamyfin/utils/atoms/settings.ts:468` |
| `syncPlayIgnoreWait` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:848` | No same native control established | `streamyfin/utils/atoms/settings.ts:470` |
| `skipIntro` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:73` | Portico intro/credits/recap ask/auto/off; no commercial/preview rows | `streamyfin/utils/atoms/settings.ts:472` |
| `skipOutro` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:74` | Portico intro/credits/recap ask/auto/off; no commercial/preview rows | `streamyfin/utils/atoms/settings.ts:473` |
| `skipRecap` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:75` | Portico intro/credits/recap ask/auto/off; no commercial/preview rows | `streamyfin/utils/atoms/settings.ts:474` |
| `skipCommercial` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:76` | Portico intro/credits/recap ask/auto/off; no commercial/preview rows | `streamyfin/utils/atoms/settings.ts:475` |
| `skipPreview` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:77` | Portico intro/credits/recap ask/auto/off; no commercial/preview rows | `streamyfin/utils/atoms/settings.ts:476` |
| `showResumeDialog` | UI reference `streamyfin/components/settings/PlaybackControlsSettings.tsx:225` | No same native control established | `streamyfin/utils/atoms/settings.ts:478` |
| `defaultPlaybackSpeed` | UI reference `streamyfin/components/settings/PlaybackControlsSettings.tsx:112` | Video/book default speed rows | `streamyfin/utils/atoms/settings.ts:480` |
| `playbackSpeedPerMedia` | Model-only in this inspected settings set | No same native control established | `streamyfin/utils/atoms/settings.ts:481` |
| `playbackSpeedPerShow` | Model-only in this inspected settings set | No same native control established | `streamyfin/utils/atoms/settings.ts:482` |
| `mpvCacheEnabled` | UI reference `streamyfin/components/settings/MpvBufferSettings.tsx:30` | No same native control established | `streamyfin/utils/atoms/settings.ts:484` |
| `mpvCacheSeconds` | UI reference `streamyfin/components/settings/MpvBufferSettings.tsx:66` | No same native control established | `streamyfin/utils/atoms/settings.ts:485` |
| `mpvDemuxerMaxBytes` | UI reference `streamyfin/components/settings/MpvBufferSettings.tsx:77` | No same native control established | `streamyfin/utils/atoms/settings.ts:486` |
| `mpvDemuxerMaxBackBytes` | UI reference `streamyfin/components/settings/MpvBufferSettings.tsx:88` | No same native control established | `streamyfin/utils/atoms/settings.ts:487` |
| `mpvVoDriver` | UI reference `streamyfin/components/settings/MpvVoSettings.tsx:28` | No same native control established | `streamyfin/utils/atoms/settings.ts:489` |
| `enableHorizontalSwipeSkip` | UI reference `streamyfin/components/settings/GestureControls.tsx:37` | Gap: no gesture settings | `streamyfin/utils/atoms/settings.ts:491` |
| `enableLeftSideBrightnessSwipe` | UI reference `streamyfin/components/settings/GestureControls.tsx:38` | Gap: no gesture settings | `streamyfin/utils/atoms/settings.ts:492` |
| `enableRightSideVolumeSwipe` | UI reference `streamyfin/components/settings/GestureControls.tsx:39` | Gap: no gesture settings | `streamyfin/utils/atoms/settings.ts:493` |
| `enableHoldToSpeed` | UI reference `streamyfin/components/settings/GestureControls.tsx:40` | Gap: no gesture settings | `streamyfin/utils/atoms/settings.ts:494` |
| `holdToSpeedRate` | UI reference `streamyfin/components/settings/GestureControls.tsx:41` | No same native control established | `streamyfin/utils/atoms/settings.ts:495` |
| `enablePinchToZoom` | UI reference `streamyfin/components/settings/GestureControls.tsx:42` | Gap: no gesture settings | `streamyfin/utils/atoms/settings.ts:496` |
| `enableDoubleTapToSeek` | UI reference `streamyfin/components/settings/GestureControls.tsx:43` | Gap: no gesture settings | `streamyfin/utils/atoms/settings.ts:497` |
| `hideVolumeSlider` | UI reference `streamyfin/components/settings/GestureControls.tsx:44` | No same native control established | `streamyfin/utils/atoms/settings.ts:498` |
| `hideBrightnessSlider` | UI reference `streamyfin/components/settings/GestureControls.tsx:45` | No same native control established | `streamyfin/utils/atoms/settings.ts:499` |
| `usePopularPlugin` | Model-only in this inspected settings set | Gap: external client integration/settings | `streamyfin/utils/atoms/settings.ts:500` |
| `mergeNextUpAndContinueWatching` | UI reference `streamyfin/components/settings/AppearanceSettings.tsx:73` | No same native control established | `streamyfin/utils/atoms/settings.ts:501` |
| `showUpcomingOnHome` | UI reference `streamyfin/components/settings/AppearanceSettings.tsx:84` | No same native control established | `streamyfin/utils/atoms/settings.ts:503` |
| `hiddenHomeHeroSections` | Model-only in this inspected settings set | No same native control established | `streamyfin/utils/atoms/settings.ts:510` |
| `hiddenHomeHeroMediaTypes` | Model-only in this inspected settings set | No same native control established | `streamyfin/utils/atoms/settings.ts:511` |
| `useEpisodeImagesForNextUp` | UI reference `streamyfin/components/settings/AppearanceSettings.tsx:97` | No same native control established | `streamyfin/utils/atoms/settings.ts:514` |
| `nativeVideoPlayerTV` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:906` | No same native control established | `streamyfin/utils/atoms/settings.ts:517` |
| `nativeVideoPlayerAndroidTV` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:924` | No same native control established | `streamyfin/utils/atoms/settings.ts:519` |
| `showHomeBackdrop` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:1432` | No same native control established | `streamyfin/utils/atoms/settings.ts:520` |
| `showHeroCarousel` | UI reference `streamyfin/components/settings/AppearanceSettings.tsx:55` | No same native control established | `streamyfin/utils/atoms/settings.ts:526` |
| `tvTypographyScale` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:309` | No same native control established | `streamyfin/utils/atoms/settings.ts:527` |
| `showSeriesPosterOnEpisode` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:1443` | No same native control established | `streamyfin/utils/atoms/settings.ts:528` |
| `tvThemeMusicEnabled` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:1450` | No same native control established | `streamyfin/utils/atoms/settings.ts:529` |
| `hideRemoteSessionButton` | UI reference `streamyfin/components/settings/AppearanceSettings.tsx:110` | No same native control established | `streamyfin/utils/atoms/settings.ts:531` |
| `hideWatchlistsTab` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/plugins/streamystats/page.tsx:52` | No same native control established | `streamyfin/utils/atoms/settings.ts:532` |
| `audioLookaheadEnabled` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/music/page.tsx:212` | No same native control established | `streamyfin/utils/atoms/settings.ts:534` |
| `audioLookaheadCount` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/music/page.tsx:129` | No same native control established | `streamyfin/utils/atoms/settings.ts:535` |
| `audioMaxCacheSizeMB` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/music/page.tsx:110` | No same native control established | `streamyfin/utils/atoms/settings.ts:536` |
| `preferLocalAudio` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/music/page.tsx:164` | No same native control established | `streamyfin/utils/atoms/settings.ts:538` |
| `musicNormalizationMode` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings/music/page.tsx:90` | music.audioNormalization | `streamyfin/utils/atoms/settings.ts:540` |
| `audioTranscodeMode` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:162` | No same native control established | `streamyfin/utils/atoms/settings.ts:542` |
| `wikidataAwardsEnabled` | UI reference `streamyfin/components/settings/PluginSettings.tsx:55` | No same native control established | `streamyfin/utils/atoms/settings.ts:545` |
| `openSubtitlesEnabled` | UI reference `streamyfin/components/settings/PluginSettings.tsx:66` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:546` |
| `openSubtitlesApiKey` | UI reference `streamyfin/components/settings/SubtitleToggles.tsx:47` | Tracks/languages/timing exist; style/mute policy gaps | `streamyfin/utils/atoms/settings.ts:548` |
| `inactivityTimeout` | UI reference `streamyfin/app/(auth)/(tabs)/(home)/settings.tv.tsx:651` | No same native control established | `streamyfin/utils/atoms/settings.ts:550` |
| `sentryEnabled` | UI reference `streamyfin/components/settings/PluginSettings.tsx:16` | No same native control established | `streamyfin/utils/atoms/settings.ts:552` |


## Item actions inventory

Actions are conditional on item kind, server permission, online state and backend. “Model-only” means the shared action ID exists but this native caller does not pass/handle it. Portico evidence: `C/presentation/entry-actions.ts:13`, `detail-actions.ts:64`, `N/src/player/EntryMenu.tsx:49`, `N/src/screens/title/SavedPages.tsx:99`, `N/src/screens/channels/ProgramSheet.tsx:166`. Plezy evidence: `plezy/lib/widgets/media_context_menu.dart:71` and its backend-neutral menu context, download rules and handlers; Swiftfin: `Shared/Views/ItemView/Components/ActionButtons/`; Android TV: item-detail/actions and browsing contexts.

| Item/context | Portico native actions actually exposed | Plezy | Swiftfin / Android TV | Missing or caution |
|---|---|---|---|---|
| Movie/playable video | Play/Resume/Beginning; queue next/add; My List/favorite/watched; playlist; download handheld; details; permitted metadata edit | Same core actions plus version, file info, external play, rate, collection, metadata/match/quick tags/delete where backend allows | Playback source/quality/tracks, favorite/played, trailers and permitted admin actions | Portico card menu does not pass rating/add-to-collection/refresh/delete even though shared model supports these; detail exposes some via More. |
| Continue Watching card | Remove from Continue Watching with Undo; normal item menu | Remove from Continue Watching, watched/unwatched, details | Resume/state actions | Portico removal has its own optimistic scope/fence helper. |
| Recommendation card | Not interested with Undo; item menu via long press | Catalog watchlist/rate/request where supported; normal library menu | Recommendation/item actions | Accessible non-long-press alternative still needs proof. |
| Show | Play next/resume, My List/favorite/bulk watched, show switches; container download and organize context where provided | Shuffle, watched/unwatched, download keep-N/sync, collection, metadata and deletion | Show/season actions, trailers, missing-episode configuration | Portico has four show switches; not full ordering/retention policy. |
| Season | Play/shuffle, mark watched/unwatched, applicable download container, episode jump | Same plus sync/metadata/delete/backend options | Play season/shuffle/state/context | Check specials and multi-file deletion scope; Portico media delete is not a general native card action. |
| Episode | Play/resume/detail panel/More, watched, queue; show workspace selection | Play/details preference, version, rate, go-to-series, spoiler masking, downloads/metadata | Parent navigation and source/track actions | Portico pre-play subtitle choice loses track identity for duplicate languages. |
| Music track | Play/queue next/add, personal state, playlist/download; album/artist via entity routes | Play-next/add-to-queue, instant mix, album/artist, rate/favorite/download | Music/context actions vary | Portico deliberately has no music speed or skip transport; audiobook keeps them. |
| Album/artist/disc/book | Play/shuffle via whole-container selector, track/chapter navigation, personal/container state; downloads for supported container kinds | Album/artist play/shuffle/instant mix and containers | Library/item grouping | Explicit book/chapter UX is Portico strength. |
| Person | Open biography/credits; credit title opens its own destination | Actor/media results; catalog cast | Person browsing | No invented arbitrary Play action. |
| Playlist entry | Tap plays from item anchor; long press Move Up/Down/Remove if manageable | Reorder/remove, item context, offline/sync actions | Playlist actions vary | Portico duplicate occurrence play anchor loses distinction; remove has no confirmation/Undo. |
| Playlist/collection | Play/Shuffle, playlist remove/reorder entries; collection/view title opens resource | Create/name/add/remove/reorder/delete; smart container indications | Native playlist/collection coverage differs | Portico native create/rename/delete/share/resource grants not complete. |
| Saved View | Open current server query; loaded subset play/shuffle; add/remove Home row | Saved/smart grouping depends on backend | Filter/custom Home options | Portico view play/shuffle lacks whole-query selector. |
| Download | Play if verified; pause/resume/retry; remove with confirmation; server prepare/cancel/remove | Batch pause/resume/retry/delete, rules, folder/collection delete, versions | Not comparable TV destination | Portico successful local file and server preparation are separate states. |
| Live channel | Tune, favorite, programme detail; guide/source views | Favorite/reorder, Watch Live/start where timeshift permits | Guide/tune/favorite and channel options | Portico Play on excludes channels. |
| Programme | Watch/channel, record once/series, padding/keep, remind, favorite, cancel | Watch live/start, record options/rules/cancel with confirm | Record/guide actions | Portico programme-sheet cancellation bypasses confirmation. |
| Recording/rule | Detail/play, delete/cancel/rule options in recording screens | Scheduled/rules/edit/delete/re-evaluate | DVR item/timer actions | Do not generalize safe confirmations elsewhere to programme sheet. |
| Library | Switch, Discover/pivots, filters/sorts/jump, owner unmatched, saved view | Reorder/hide, scan/analyze/refresh/empty trash with confirms | Library display/filter/random and admin elsewhere | Portico scan/maintenance is mainly owner Settings. |

## System integration inventory

“Not established” is deliberately weaker than “unsupported.” The inspected sources are sufficient for named Portico integrations, but this is not a vendor entitlement audit. OS features that happen automatically do not imply app-specific implementation. Android TV’s `tvProviderEnabled` is its **TV channel database provider**, not pay-TV identity/SSO.

| Integration | Portico I/iPad | Portico TV/Android/Fire/Vega | Plezy | Swiftfin / androidtv / others | Source/value/remaining question |
|---|---|---|---|---|---|
| tvOS Top Shelf | — | T: extension + scoped feed token; V: storage-only; G/F: different OS | tvOS native extension + live fetch | Swiftfin integration requires separate proof | `N/Native/PorticoTopShelf.m`, `N/tvos/TopShelf/`; `plezy/tvos/TopShelfExtension/ShelfFetcher.swift:1`. |
| Android TV Watch Next | — | G TV implemented; Fire omitted; Vega no publication | Native Watch Next bridge/deep links | androidtv OS integration | `N/android/app/src/main/java/tv/getportico/portico/native/PorticoTopShelf.kt:35`; `plezy/android/app/build.gradle.kts:431`. Home rows don't prove system rows. |
| Android TV channels | — | G: one channel of next Home row; Fire/Vega absent | Watch Next established; full separate channel parity not established here | androidtv `tvProviderEnabled` UI + provider | Fire/Vega absence is documented; no substitute Google service required. |
| Home-screen widgets | No WidgetKit implementation established | No Android widget implementation established | No general widget established | Streamyfin download Live Activity is a separate surface | Do not call Top Shelf or Live Activity a widget. |
| Siri/Assistant/search voice | App Intents “Resume/Play title”; system dictation | T: Siri Remote keyboard/touch; Android TV platform keyboard voice; V system keyboard | TV text/remote input; custom virtual keyboard also exists | androidtv voice search; Swiftfin OS keyboard | `N/ios/Portico/PorticoIntents.swift:103`, `Native/PorticoSearchKeyboard.m:6`; no full Assistant catalog intent established. |
| Spotlight / system universal search | Current title activity + bounded Continue/Saved title index | Android Activity methods no-op; tvOS cross-app catalog search not established | No equivalent phone index established | Distinct from in-app search; vendor universal-search participation unestablished | `N/Native/PorticoActivity.m:7`; Saved feeder is wrong (B-21.4). |
| Handoff | Current title via NSUserActivity; Hosted web URL | No-op Android/Vega; TV link handling exists | Not established | Infuse iCloud sync is not Handoff | `PorticoActivity.m:62`, `N/ios/Portico/AppDelegate.swift:78`; source IDs must resolve on receiving account/server. |
| SharePlay | Not established | Not established | Relay Watch Together, not SharePlay | Neither SyncPlay nor server rooms establish Apple GroupActivities | Possible integration sits over existing room UI, with additional lifecycle/legal/device concerns. |
| CarPlay | No app integration established | — | Not established | Infuse/Plex media output routes do not imply CarPlay app | Portico music can use OS media commands; that is not an in-car browse app. |
| Android Auto / Automotive | — | No Auto/Automotive browse/driver-state integration established | **Android Automotive OS** driver restrictions present; do not mislabel as Android Auto | Streamyfin car support unestablished | `plezy/android/app/src/main/kotlin/com/edde746/plezy/car/CarRestrictionsMonitor.kt:17`; vehicle system features explicitly veto TV detection. |
| Watch app | No watchOS target established | — | Not established | Not established in audited apps | Potential music remote distinct from broad watch media client. |
| Shortcuts / App Intents | Play title / resume App Shortcuts | Android/Vega no corresponding exposed catalog intent | Keyboard shortcuts are unrelated to App Intents | Swiftfin shortcut parity unestablished | `N/ios/Portico/PorticoIntents.swift:103`; title candidates bounded. |
| OS launcher quick actions | No dedicated shortcut menu established | No dynamic/static launcher shortcuts established | Not established | Generic deep links separate | Useful Downloads/Search/Resume entry points would be small UI adapters. |
| Notifications | In-app alerts; no push by product decision | In-app alerts; Android transfer/service notifications separate | Android download progress/complete; background restrictions UX | Streamyfin download Live Activity; androidtv telemetry/status | No push is deliberate (`06 Clients.md`, Rules); cannot assume timely off-app social invitations. |
| Screensaver / dream | — | No app screensaver/dream implementation established | Dedicated app screensaver not established | androidtv LibraryDreamService + in-app idle/age filter; Wholphin screensaver prefs | `JTV/integration/dream/LibraryDreamService.kt`; protect restricted artwork. |
| Deep / universal links | Allowlisted native routes and pending auth invite flow | TV shelf links; Android manifest intent filters; Vega Linking adaptation | Watch Next/Top Shelf/custom links | Swiftfin navigation routing, androidtv OS links | `N/src/shell/Shell.tsx:249`; initial URL/session/readiness fencing needs review (B-21.14). |
| iPad multitasking / rotation | Responsive window/rail/grid, portrait/landscape declared | Android tablet width class | Responsive layout with landscape mobile rail | Swiftfin native SwiftUI layouts | `N/ios/Portico/Info.plist`; no device multitasking acceptance established. |
| Keyboard shortcuts | Native text input; no app shortcut editor established | TV semantic remote commands | Editable shortcut recorder; gamepad keyboard synthesis | Swiftfin gestures; androidtv D-pad | Keyboard pointer equivalence on iPad requires hardware/assistive proof. |
| Pointer / trackpad | Shared hover/focus primitives | Tablet/mouse platform dependent | Mouse cursor/hover/desktop/mobile-rail adaptations | SwiftUI defaults help but don't establish all interactions | `N/src/ui/Focusable.tsx`; no pointer-specific acceptance evidence. |
| Stage Manager / multiwindow | Window resize adaptation; no dedicated scene/multiwindow UX established | — | Desktop windows not iPad Stage Manager proof | Apple iPad conventions call for adaptation | Need narrow-width, detached keyboard, interruptions, focus return acceptance. |
| External display | No second-screen scene/presentation controller established | HDMI video output handled by OS; app-specific split output unestablished | Display matching controls are different | Infuse/Plex may have OS output; not audited | Do not count AirPlay as proof of wired dual-window presentation. |
| Game controllers | No full mapping/editor established | Siri Remote micro-gamepad touch module; platform D-pad can deliver navigation | universal_gamepad + semantic event synthesis | Native TV/gamepad platforms vary | `N/Native/PorticoSiriRemote.m:47`, `plezy/lib/utils/key_event_simulator.dart`; button/hold parity needs actual controller checks. |
| HDMI-CEC / power / idle | — | No app CEC mapping control established; OS-reserved input remains system | TV suspension/wakelock policies present | androidtv idle/prompt/screensaver behavior | Media session/play-pause implementation doesn't prove CEC hardware behavior. |
| TV provider / single sign-on | Hosted Apple/Google sign-in is account SSO | No Apple TVProvider entitlement/pay-TV SSO established | Plex authorization is not pay-TV SSO | androidtv provider name denotes channels, not identity | Useful to clarify terminology before prioritizing integration. |
| Cast/AirPlay/receiver handoff | Apple native AirPlay + Cast if configured | Android handheld Google sender; Fire TV no sender; TVs receive Portico handoff; Vega no discovery | Chromecast sender not established here; companion remote is distinct | Streamyfin Chromecast; Infuse public Cast/AirPlay | `N/src/player/Destinations.tsx:19`; permanent TV code missing there. |
| Native image/file pickers | Avatar/image picker, subtitle file import | Android picker adapters; TV authoring omitted; Vega returns null | Metadata file/URL artwork, shader/import/external player | Swiftfin item editing on iOS | OS permission cancellation/failure UX must be checked independently. |
| System Now Playing/media commands | Audio/media session/background declaration | Native Android service/session; Vega semantic system-media adapter | Music mini-player and native media controls | Swiftfin/AndroidTV platform sessions | Here only reachability/state labels are assessed, not engine/background correctness. |

Companion Android native integration classes share `N/android/app/src/main/java/tv/getportico/portico/native/`. Wholphin/Android TV Kotlin shorthand is resolved in the detailed settings inventory.


## Architectural differences

The entries below separate architectural ownership from visible feature presence. “Ahead” does not claim release acceptance; it judges the source design for Portico’s stated scale/privacy goals. Effort estimates concern adopting the relevant UX capability, not a full platform rewrite.

### A-21.1 Server-composed media surfaces versus client-composed hubs

- **Portico:** Home/library rows, ranking, labels and browse pages arrive as server projections; shared code authors presentation. `Project Architecture/06 Clients.md`, Rules; `N/src/screens/Home.tsx:35`, `Library.tsx:43`.
- **Jellyfin / Plezy / other:** Plezy merges connected sources and builds discovery locally; Swiftfin/Android TV compose their own resume/next-up/recent sections. `plezy/lib/providers/libraries_provider.dart:230`; `JTV/ui/settings/screen/home/SettingsHomeScreen.kt:34`.
- **Why it matters:** A new Portico platform inherits content decisions, and large-library work can remain server bounded. Independent clients gain flexible service integration and client-specific choices.
- **Assessment:** **Portico ahead.** Deliberate thin-client principle. Keep it; adding cross-server composition would be large work in scoped identity, aggregation and presentation, not just an extra shelf. A stronger client customization contract could change the balance.

### A-21.2 One shared screen implementation across six native targets

- **Portico:** All native screens are shared; platform modules preserve matching names. `N/src/shell/Shell.tsx:94`; Android and Vega port logs.
- **Jellyfin / Plezy / other:** Swiftfin and official Android TV have independent frameworks and UI trees. Plezy similarly shares Flutter screens with explicit platform branches.
- **Why it matters:** Portico reduces drift but propagates Apple assumptions and defects to every port. A shared file is not evidence of supported native capabilities.
- **Assessment:** **Trade-off (subjective).** Deliberate Apple-reference strategy. Small capability fixes are worthwhile; replacing with separate native UIs would be large and likely worsen parity maintenance. Device acceptance could change the judgment.

### A-21.3 Vega emulates Apple React Native semantics

- **Portico:** Canvas, focus, text input, key naming, snap scrolling and Reanimated are shimmed. `V/src/shims/react-native.ts:1`, `focus.tsx:5`, `text-input.tsx:29`.
- **Jellyfin / Plezy / other:** Plezy adapts UI through Flutter focus/input; Wholphin uses Android TV Compose directly.
- **Why it matters:** Vega shares authoring but owns substantial framework behavior. Focus restoration/typing/held-key bugs may sit in adapters rather than screens.
- **Assessment:** **Unclear.** Medium maintenance/acceptance cost. The source documents concrete SDK incompatibilities; keep adapters narrow and validate semantic outcomes on hardware before declaring parity.

### A-21.4 Multiple servers are separate navigation universes

- **Portico:** One server/viewer is selected; known direct and Hosted servers are switched through Servers. `N/src/screens/SettingsPages.tsx:83`.
- **Jellyfin / Plezy / other:** Plezy connections aggregate libraries/Home/search and may group source labels. `plezy/lib/providers/libraries_provider.dart:230`, `lib/screens/search_screen.dart:39`.
- **Why it matters:** Portico keeps server-local policy/state clear but makes multi-server users repeatedly change context. Plezy has partial-failure and provenance obligations.
- **Assessment:** **Trade-off (subjective).** Deliberate single selected-server authority; saved contract explicitly excludes aggregation. Large to add merged catalog identity/search while preserving privacy. Do not mistake switching for parity.

### A-21.5 Hosted account authorization can succeed without a usable server

- **Portico:** Account authentication and server selection are distinct; native landing handles zero servers/offline server. `onboarding/contract.md:29`; `N/src/screens/auth/AuthFlow.tsx:97`.
- **Jellyfin / Plezy / other:** Swiftfin/Android TV typically need a server before user login; Plezy has account/connection/profile onboarding.
- **Why it matters:** Account sign-in need not falsely fail because a media server is offline. It also adds account-versus-server state that UI must explain.
- **Assessment:** **Portico ahead.** Deliberate independent-authority rule. Keep it; medium UX work for coherent empty/error states and device sign-in, with no need to copy competing server-coupled flow.

### A-21.6 TV setup is a first-class code/QR approval workflow

- **Portico:** Activation uses one code/QR session; direct server has Quick Connect/password fallback. `N/src/screens/auth/AuthFlow.tsx:103`; `onboarding/contract.md:124`.
- **Jellyfin / Plezy / other:** Plezy Plex authorization QR/browser and Jellyfin Quick Connect; official clients have user/server pickers and Quick Connect.
- **Why it matters:** Remote typing is avoided; phone approval must remain available without discovery. Portico currently misses the Play on entry required by contract.
- **Assessment:** **Portico ahead.** Deliberate code-centric design. Keep it; small UI repair B-21.1 completes the promised path. Cryptographic/idempotency internals are outside this report.

### A-21.7 Profile authority differs from client-local profile convenience

- **Portico:** Local server profiles support avatar/PIN/limits; TV management hands off. `N/src/screens/Profiles.tsx:1`, `SettingsPage.tsx:215`.
- **Jellyfin / Plezy / other:** Plezy has local profiles with borrowed connections and Plex Home profiles; Swiftfin/Android TV primarily select Jellyfin users. `plezy/lib/screens/profile/borrow_connection_screen.dart:43`.
- **Why it matters:** Same visible avatar grid hides different restrictions and credential ownership. Borrowing a connection is not creation of a restricted server profile.
- **Assessment:** **Trade-off (subjective).** Portico server-authoritative restrictions are desirable; local convenience profiles would be medium work and should never grant broader access. A household usability study might justify a launch profile prompt.

### A-21.8 Remembered direct servers do not silently retain cross-server login

- **Portico:** Server list remembers name/address and asks for renewed direct sign-in. `N/src/screens/SettingsPages.tsx:96`.
- **Jellyfin / Plezy / other:** Plezy keeps per-profile signed-in connections and multiple URLs; Swiftfin/Android TV retain local server/user selections.
- **Why it matters:** Portico is simple and explicit but multi-server direct users have more ceremony. Hosted directory reduces it for account users.
- **Assessment:** **Trade-off (subjective).** Small/medium to retain independent credential families securely, but scope/logout/download semantics must remain exact. The current choice appears explicit in source comments, not a runtime accident.

### A-21.9 Native owner console is unusually broad

- **Portico:** Owner handheld settings has native libraries/live/people/streaming/remote/storage/schedule/troubleshooting/general/history panels. `C/presentation/settings-structure.ts:302`.
- **Jellyfin / Plezy / other:** Swiftfin iOS has users/devices/tasks/logs/API keys/activity/backups; Plezy focuses on metadata and library maintenance. `swiftfin/Swiftfin/Views/AdminDashboardView/AdminDashboardView.swift:1`.
- **Why it matters:** An owner can operate the server away from a desktop; this creates input/draft/confirmation obligations on small screens.
- **Assessment:** **Portico ahead.** Deliberate documented native Server pages. Keep and advertise carefully; medium polish for complex phone forms. Swiftfin shows that user/device/task administration is not inherently a web-only task.

### A-21.10 TV administration deliberately resolves only Dashboard

- **Portico:** TV owner page whitelist is Dashboard. `C/presentation/settings-structure.ts:291–308`.
- **Jellyfin / Plezy / other:** Swiftfin also gates admin Dashboard to iOS; Plezy can expose some metadata/library actions on TV. `swiftfin/Shared/Views/SettingsView/SettingsView.swift:75`.
- **Why it matters:** TV cannot manage all server settings, despite identical shared component imports. Typing-heavy operations can be handed off coherently.
- **Assessment:** **Trade-off (subjective).** Deliberate. Small to add read-only useful status, medium for safe TV-native mutations. Retain the omission if the handoff and scope are clearly communicated.

### A-21.11 Authored settings layout rather than registry-generated UI

- **Portico:** The registry is vocabulary; shared structure decides page/section/control/order. `C/presentation/settings-structure.ts:1`.
- **Jellyfin / Plezy / other:** Plezy/Swiftfin/Android TV author settings in each UI; Streamyfin plugin seeds/locks a settings object.
- **Why it matters:** A published setting can be valid and consumed yet unreachable. Shared authoring reduces inconsistent names while concentrating responsibility for platform gates.
- **Assessment:** **Portico ahead.** Deliberate good architecture. Small to maintain a rendered-versus-consumed matrix; audit B-21.6/10 because field existence alone is insufficient.

### A-21.12 Server device-class settings are labelled as this device

- **Portico:** Registry quality/delivery use profile-device-class; native writes mobile/television, not installation identity. `C/preferences.ts:5`, `N/src/app/server-preferences.tsx:96`.
- **Jellyfin / Plezy / other:** Plezy local appearance/playback settings are distinct from upstream account preference edits. Infuse publicly syncs settings via iCloud.
- **Why it matters:** A change on one phone may affect another phone of the same profile; UI wording suggests otherwise.
- **Assessment:** **Portico behind.** B-21.7 is confirmed wording/storage mismatch. Small to label correctly; large if true per-installation server overrides are desired. Keep the distinction explicit.

### A-21.13 Installation-local and server preference models coexist

- **Portico:** Native local store retains legacy autoplay/speed/subtitle fields alongside current server rows. `N/src/app/preferences.ts:13`.
- **Jellyfin / Plezy / other:** Streamyfin centralizes settings with overrides; Plezy separates account preferences and local settings.
- **Why it matters:** Two sets of similarly named values make it easy for future controls to update the wrong authority or to appear saved while unused.
- **Assessment:** **Unclear.** Small cleanup/documentation work after consumer mapping, not automatic deletion of legacy storage. A proven migration policy would make the coexistence harmless.

### A-21.14 Home saved query rows are server-backed resources

- **Portico:** Home customization can add Saved Views and revision-fence layout. `N/src/screens/HomeCustomise.tsx:21`, `Library.tsx:116`.
- **Jellyfin / Plezy / other:** Android TV uses ten section slots; Swiftfin recent/next-up switches; Plezy hub/library management.
- **Why it matters:** A personal row can remain a live, correct query rather than a cached item list. That is particularly useful at library scale.
- **Assessment:** **Portico ahead.** Deliberate server/query architecture. Keep; medium to expose equivalent TV creation with guided naming, small to improve discovery of the existing mobile workflow.

### A-21.15 My List is a destination rather than a Home shelf

- **Portico:** Saved owns My List; Home deliberately excludes it. `media-presentation/contract.md`, §14.2.
- **Jellyfin / Plezy / other:** Plezy Explore/trackers and Plex watchlist conventions may place saved intent nearer discovery.
- **Why it matters:** Portico keeps Home compact and media-neutral naming consistent; returning to saved intent costs navigation. Spotlight feeder still assumes the removed row.
- **Assessment:** **Trade-off (subjective).** Explicit 2 October product decision. Keep unless Justin changes it; small repair for B-21.4 should fetch Saved independently, not reintroduce a prohibited shelf.

### A-21.16 Fixed theatre theme versus extensive appearance personalization

- **Portico:** Native palette/fonts/metrics are fixed; only Reduce Motion/TV text scale. `N/src/ui/theme.ts:26`, `settings-structure.ts:241`.
- **Jellyfin / Plezy / other:** Plezy has System/Light/Dark/OLED; Swiftfin accent; Wholphin colors.
- **Why it matters:** Consistent contrast/materials can reduce support and accessibility drift. Some users need daylight/light themes or prefer OLED.
- **Assessment:** **Trade-off (subjective).** Deliberate visual contract rejects pure black/arbitrary theme divergence. Medium to add reviewed variants, small for constrained accent if product wants it. Do not silently adopt AMOLED against the contract.

### A-21.17 Artwork geometry is semantic rather than freely chosen

- **Portico:** Posters for video/book, squares music, episode thumbs confined to show detail except recorded search exception. `media-presentation/contract.md:33`; shared presentation helpers.
- **Jellyfin / Plezy / other:** Plezy chooses series/season/thumb for episodes; Swiftfin portrait/square/landscape and Android TV image type.
- **Why it matters:** Portico shelf geometry is stable and recognizable; users lose high-density episode-thumbnail browsing choices.
- **Assessment:** **Trade-off (subjective).** Deliberate decision. Medium contract/presentation change to expose alternative views while preserving identity and row geometry. The baseline is coherent, not a defect solely because competitors differ.

### A-21.18 Watched indicators are a fixed product rule

- **Portico:** Checkmark/progress, no generic unwatched mark; native indicator toggles absent. `06 Clients.md`, Card marks; `N/src/ui/Card.tsx`.
- **Jellyfin / Plezy / other:** Swiftfin has favorite/progress/played/unplayed-none-dot-count; Plezy count/check switches; Android TV watched behavior.
- **Why it matters:** Portico reduces badge clutter but hides count/style choices used by completion-oriented viewers.
- **Assessment:** **Trade-off (subjective).** Deliberate sparse marks. Small/medium reviewed indicator preferences; adding unplayed counts requires clear hierarchy/server projections, not client-wide scans.

### A-21.19 Library peers are shared per media kind

- **Portico:** Discover first, then specific video/music/book/entity peers and owner unmatched. `N/src/screens/Library.tsx:43`, `C/presentation/library-tabs.ts`.
- **Jellyfin / Plezy / other:** Plezy uses Recommended/Browse/Collections/Playlists with grouping; Swiftfin custom library layout; Android TV smart display.
- **Why it matters:** Portico clients expose the same taxonomy for books/music/video and owner repair. More peers can clutter TV if width/focus rules are weak.
- **Assessment:** **Portico ahead.** Deliberate shared authoring. Keep; small responsive overflow/focus improvements if real long labels or many tabs fail.

### A-21.20 Browse capability vocabulary comes from server

- **Portico:** Filter fields/operators/value pages and sorts are published; no client catalog sorting. `N/src/screens/BrowseFilters.tsx:30`, `Library.tsx:102`.
- **Jellyfin / Plezy / other:** Plezy backend adapters expose broad operators/text/ranges; official clients supply narrower native options.
- **Why it matters:** Portico can keep complete filter/sort truth at millions of items. TV controls must faithfully represent published types rather than just enumerate easy values.
- **Assessment:** **Portico ahead.** Deliberate O(visible)/correctness principle. Medium guided operator/range UX; preserve query semantics and never load all items to imitate another client.

### A-21.21 TV filtering avoids text entry and separates field/value levels

- **Portico:** Inline Sort/Filter/Jump; right panel with field groups/values, remote focus. `N/src/screens/BrowseFilters.tsx:55–99`.
- **Jellyfin / Plezy / other:** Plezy supports value search and text/range pages; Swiftfin/Android TV letter/filter controls.
- **Why it matters:** Portico is faster for remote use and accessible when fields are discrete; freeform text predicates are harder to author.
- **Assessment:** **Trade-off (subjective).** Deliberate client rule. Small preset/operator controls, medium support for number/date ranges without keyboard. Evidence of needed searches could justify a narrow text entry path.

### A-21.22 Native view modes lag the web and competition

- **Portico:** Grid metrics fixed; libraryView/cardSize rows web-only. `C/presentation/settings-structure.ts:243–249`, `N/src/ui/theme.ts:114`.
- **Jellyfin / Plezy / other:** Plezy grid/list/density/spacing; Swiftfin list columns; Android TV per-library size/image/grid direction.
- **Why it matters:** A 10-million-item library still needs ergonomic scan density and tablet list views, independently of network efficiency.
- **Assessment:** **Portico behind.** Backlog recognizes poster size. Medium shared render contract/control work; bound both grid and list. Fixed geometry can remain default.

### A-21.23 Alphabet and jump use bounded browser windows

- **Portico:** Native alphabet/jump operates through browse/windowed collection rather than sorting all titles locally. `N/src/screens/BrowseFilters.tsx:243`, `Library.tsx:162`.
- **Jellyfin / Plezy / other:** Plezy alpha strategies/helpers, Swiftfin configurable letter picker, Android TV alphabet navigation.
- **Why it matters:** Correct server offsets can make far-away letters cheap. Focus restoration across unmapped windows remains a hardware risk.
- **Assessment:** **Portico ahead.** Keep server-authoritative jump; small per-form-factor affordance polish. It would be worse to copy an in-memory whole-list alpha index.

### A-21.24 Whole-container playback and loaded-view playback differ

- **Portico:** Shows/seasons/albums/playlists/books use server selectors; Saved Views shuffle only loaded entries. `N/src/app/container-play.ts:1`, `SavedPages.tsx:24`.
- **Jellyfin / Plezy / other:** Plezy container launcher and playlist continuation; random/shuffle choices occur in other clients.
- **Why it matters:** Portico avoids truncating ordinary container playback, but Saved View play/shuffle currently has different completeness.
- **Assessment:** **Trade-off (subjective).** Server selector is stronger architecture; B-21.11 identifies the view limit. Medium query selector support should reuse browse identity rather than materialize the entire view client-side.

### A-21.25 One canonical show workspace instead of many detached child screens

- **Portico:** Show/season/episode routes converge on Show with selected episode panel. `N/src/app/routes.ts:17`, `Show.tsx:324`.
- **Jellyfin / Plezy / other:** Plezy media detail/season tabs; official clients often distinct show/season/item groups.
- **Why it matters:** Context stays recognizable and resume season selection can be stable. It increases complexity of focus/jump/deep-link restoration.
- **Assessment:** **Portico ahead.** Deliberate media contract §9. Keep; medium acceptance coverage for specials, groups, empty seasons and deep-linked unavailable episodes.

### A-21.26 No native episode spoiler policy established

- **Portico:** No spoiler preference/consumer appears in inspected registry/show/episode presentation.
- **Jellyfin / Plezy / other:** Plezy hideSpoilers actively feeds episode cards; `plezy/lib/widgets/episode_card.dart:119`.
- **Why it matters:** Unwatched art and synopsis can spoil a season. This is a content privacy choice distinct from parental restriction.
- **Assessment:** **Portico behind.** Medium new profile preference plus artwork/summary/title rules on every surface. Media contract calls for profile spoiler policy; no source-backed fulfillment was established.

### A-21.27 Pre-play choices omit version/quality and collapse subtitle identity

- **Portico:** Audio index and subtitle language choices exist; version intentionally deferred. `N/src/screens/title/PlaybackChoices.tsx:12`.
- **Jellyfin / Plezy / other:** Swiftfin playback button exposes version/quality/audio/subtitle stream pickers; Plezy version split button/status.
- **Why it matters:** Users cannot choose edition/cut or same-language SDH/commentary before play; an audio label may describe default rather than effective policy.
- **Assessment:** **Portico behind.** Medium source-specific preflight chooser over existing offers; do not fetch/start playback merely to display choices. B-21.9 distinguishes exact source behavior.

### A-21.28 Context and detail actions share IDs but differ in actual capabilities

- **Portico:** Entry model includes rate/collection/refresh/delete; native card caller omits some while detail exposes rate/organize in More. `N/src/player/EntryMenu.tsx:57`, `C/presentation/entry-actions.ts:13`.
- **Jellyfin / Plezy / other:** Plezy snapshots per-open backend/permission context and exposes richer actions; Swiftfin customizes action bars/menus.
- **Why it matters:** Shared IDs prevent label drift only if each caller binds actions. A model feature is not native feature parity.
- **Assessment:** **Portico behind.** Small/medium alignment of capability mapping and handlers. Audit actual reachable actions per kind and context rather than advertise the model union.

### A-21.29 Playlist mutations can be windowed, but playback loses occurrence identity

- **Portico:** Move/remove uses occurrence IDs and relative anchors; playing passes itemId. `N/src/screens/title/SavedPages.tsx:90`, `N/src/app/container-play.ts:59`.
- **Jellyfin / Plezy / other:** Plezy disables playlist editing until fully loaded; `plezy/lib/screens/playlist/playlist_detail_screen.dart:175`.
- **Why it matters:** Portico’s mutation design suits large lists better. Duplicate song/movie entries require occurrence-aware Play as well as remove/reorder.
- **Assessment:** **Portico ahead.** Keep bounded mutations; small/medium B-21.8 repair. Competitor whole-load gating is a precedent to avoid, not an argument against native editing.

### A-21.30 Personal history and resources fall outside the windowed browsing pattern

- **Portico:** Saved media uses WindowedGrid; history/day/resource/playlist views map accumulated entries. `N/src/screens/Saved.tsx:284`, `SavedPages.tsx:140`.
- **Jellyfin / Plezy / other:** Plezy uses sliver/grid budgets and pagination; Android TV settings uses lazy lists.
- **Why it matters:** Large saved sets can become slow even though the library grid is efficient. Infinite append is not O(visible).
- **Assessment:** **Portico behind.** Known history Backlog plus adjacent same-pattern risks. Medium screen-windowing work with semantic stable keys; do not solve with an arbitrary history cap.

### A-21.31 Downloads distinguish server preparation from protected local bytes

- **Portico:** Native UI separates server work, tracked batch requests and local entries/playability. `N/src/screens/Downloads.tsx:23`, `:98`.
- **Jellyfin / Plezy / other:** Plezy download tree combines file/sync/rule status and offline detail, with background-work diagnostics.
- **Why it matters:** Preparation success is not mistaken for completed local availability. Extra states need clear copy and retry/removal semantics.
- **Assessment:** **Portico ahead.** Deliberate saved-download contract. Keep; medium improvement for grouping/batch management; engine transfer integrity is outside this UI review.

### A-21.32 Rolling offline rules are not exposed

- **Portico:** Portico offers one-shot media/container prepare/download and local rules, no keep-N-unwatched sync UI established.
- **Jellyfin / Plezy / other:** Plezy has keep-synced/keep-N/unwatched/specials and rule management. `plezy/lib/i18n/en.i18n.json:1809`, `lib/screens/downloads/sync_rules_screen.dart:21`.
- **Why it matters:** Trips and episodic commuting require fewer manual download decisions; rule deletion must not delete files shared by other rules.
- **Assessment:** **Portico behind.** Medium/large server/client rule lifecycle and storage ownership. Compatible with O(visible) if background preparation evaluates bounded changes; avoid endless periodic polling.

### A-21.33 Offline authorization is a visible scoped state rather than permanent copy

- **Portico:** Portico contract has 30-day verified receipts, explicit scope and sign-out removal; Downloads shows truthful block reasons. `saved-state-and-downloads/contract.md`, §10; `Downloads.tsx:42`.
- **Jellyfin / Plezy / other:** Plezy can retain Plex downloads on sign-out and recover them on later sign-in; `plezy/lib/i18n/en.i18n.json:899`.
- **Why it matters:** Portico’s stricter privacy keeps prior viewers’ protected media from lingering as usable content. Travel/offline users need clear verification date and reconnect guidance.
- **Assessment:** **Trade-off (subjective).** Deliberate authority/privacy policy. Keep unless explicitly changed; medium UX for expiry explanation. Source here does not establish receipt conformance at runtime.

### A-21.34 Native platform text entry is an explicit product rule

- **Portico:** Fields use native TextInput; Apple search native module exists but current Search renders Field. `N/src/screens/Search.tsx:138`; `Native/PorticoSearchKeyboard.m:6`.
- **Jellyfin / Plezy / other:** Plezy includes TV virtual keyboard and optional platform inputs; official clients use platform search patterns.
- **Why it matters:** Portico gets OS dictation/phone keyboard accessibility when platform supports it; Vega mounting/focus adapters remain substantial.
- **Assessment:** **Trade-off (subjective).** Deliberate rule. Small to remove stale claims of inline Apple search if unwired, medium if intentionally integrating that container. A custom keyboard should require clear benefit.

### A-21.35 App voice intentions are distinct from dictation

- **Portico:** Apple App Intents can Resume/Play title from bounded current/saved index; Android module no-op. `N/ios/Portico/PorticoIntents.swift:103`, `N/android/app/src/main/java/tv/getportico/portico/native/PorticoActivity.kt:9`.
- **Jellyfin / Plezy / other:** Android TV has voice-search flow; Plezy has system shelf links and keyboard/gamepad handling.
- **Why it matters:** Voice typing alone does not let an assistant discover or play a title while app closed. Apple privilege creates real per-platform parity differences.
- **Assessment:** **Portico ahead.** Keep Apple integration and repair feeder. Medium Android assistant/catalog adapters if justified; avoid claiming unsupported global catalog access.

### A-21.36 Search scope and suggestions differ

- **Portico:** Portico server groups/ranks, query debounce, per-group continuation and in-session recent list. `N/src/screens/Search.tsx:49`.
- **Jellyfin / Plezy / other:** Plezy searches connected media servers and external catalogs; Streamyfin can choose Jellyfin/Marlin/Streamystats.
- **Why it matters:** Portico has complete server-scoped results with fewer external failure modes; richer discovery/search engine choice is absent.
- **Assessment:** **Trade-off (subjective).** Deliberate server matching. Medium request/catalog integration; large merged results. The native remember-history consumer needs an accessible toggle.

### A-21.37 Handheld input focuses on visible controls, not gesture customization

- **Portico:** Touch timeline drag, native Back and sheets; no full-screen seek/brightness/volume/pinch editor found. `N/src/player/Timeline.tsx:23`, `VideoPlayer.tsx:388`.
- **Jellyfin / Plezy / other:** Swiftfin gesture editor supports pan/swipe/hold/multi-touch; Plezy brightness/volume/pinch; Streamyfin double-tap/hold-to-speed and haptics.
- **Why it matters:** One-handed viewers lose fast familiar operations. Visible controls remain necessary as accessible alternatives.
- **Assessment:** **Portico behind.** Medium input layer/preferences plus conflict resolution with scroll/back; keep every essential operation reachable without gestures.

### A-21.38 Tablet layout adapts width but is not a full tablet workspace

- **Portico:** Wide rail/grids/guide, phone-pushed settings and no shortcut editor/external scene established. `N/src/shell/Shell.tsx:128`, `Settings.tsx:27`.
- **Jellyfin / Plezy / other:** Swiftfin native list columns/regular item headers; Plezy Now Playing becomes two-pane at width 800 and landscape. `plezy/lib/screens/music/now_playing_screen.dart:292`.
- **Why it matters:** A tablet can show browse/selection or settings/value at once; phone geometry wastes width and increases transitions.
- **Assessment:** **Portico behind.** Backlog knows two-pane Settings. Medium shared tablet shell/detail composition, keyboard/pointer and resize acceptance. OS multitasking support alone is not parity.

### A-21.39 TV focus relies on native geometry plus semantic repair guides

- **Portico:** Shared Focusable/RowGuide/rail and Vega guide adapter avoid a general client spatial navigator. `N/src/shell/Rail.tsx:86`, `V/src/shims/focus.tsx:5`.
- **Jellyfin / Plezy / other:** Plezy owns Flutter focus nodes/memory and virtual keyboard/gamepad translation; Compose TV uses focus keys.
- **Why it matters:** Native spatial behavior can be smoother but hidden/virtualized nodes and route return need explicit fences. Fixed coordinates are not portable focus identity.
- **Assessment:** **Trade-off (subjective).** Deliberate NAV-10 model. Keep native movement; medium device/assistive validation. Do not mark runtime focus traps confirmed without a failing graph/device path.

### A-21.40 Root Back preserves in-app hierarchy and delegates OS exit

- **Portico:** TV root Back focuses rail, then OS; no exit confirmation. `N/src/shell/Shell.tsx:160`.
- **Jellyfin / Plezy / other:** Plezy source has press-Back-again exit policy; custom main-screen input pathways. `plezy/lib/screens/main_screen.dart:1650`.
- **Why it matters:** Portico favors platform exit behavior; competitors reduce accidental exit. Both must handle keyboard/panels before shell.
- **Assessment:** **Trade-off (subjective).** Explicit NAV-8 root rule. Keep; small held-key/CEC/controller acceptance. Copying double-back would conflict with current contract.

### A-21.41 Kept-alive root stacks preserve context but contradict current root law

- **Portico:** Inactive roots keep pushed pages; selecting another resumes it. `N/src/shell/Roots.tsx:5`.
- **Jellyfin / Plezy / other:** Media clients commonly retain per-tab context; Plezy main screen owns tab/route state.
- **Why it matters:** Useful continuity, but NAV-5.1 says replacing a root clears secondary history. One reused LibraryRoot also limits per-library independent state.
- **Assessment:** **Trade-off (subjective).** Explicit source comments show intended behavior; contract disagreement must be resolved. Small doc change or medium router policy change depending on Justin’s choice.

### A-21.42 Audio continuation uses a tiny mini bar or TV shell pill

- **Portico:** Audio persistent mini bar on handheld; TV top-right pill; video PiP handheld, full-screen-only TV. `N/src/player/MiniPlayer.tsx`, `media-presentation/contract.md:281`.
- **Jellyfin / Plezy / other:** Plezy music mini/player, Swiftfin media controls, Streamyfin native/JS chrome variants.
- **Why it matters:** Audio browsing stays usable without duplicating transport chrome. Portico omits TV PiP even though tvOS supports it.
- **Assessment:** **Trade-off (subjective).** Deliberate visual/media contract. Keep audio shell clarity; medium TV PiP integration requires product change, not an accidental gate removal.

### A-21.43 Fixed player-control grammar with utilities separate from settings

- **Portico:** Five Audio/Subtitles/Quality/Speed/Sleep boxes, phone utility collapse, queue/info/together/report disclosure. `N/src/player/PlayerOptions.tsx:16`.
- **Jellyfin / Plezy / other:** Plezy version-quality/track/settings/chapter panels and customizable shortcuts; Swiftfin action-bar/menu selectors.
- **Why it matters:** Portico recognizable grammar helps new ports; capability-specific boxes must not advertise impossible Vega effects.
- **Assessment:** **Trade-off (subjective).** Deliberate fixed player layout. Small to capability-gate; medium wider utility choices. Engine controls listed as settings are not assessed for decoder quality here.

### A-21.44 Scoped system shelf feed versus upstream token/catalog fetch

- **Portico:** Portico Top Shelf/Watch Next reads one server feed using a narrow token. `N/android/app/src/main/java/tv/getportico/portico/native/PorticoTopShelf.kt:35`, `N/src/app/top-shelf.ts`.
- **Jellyfin / Plezy / other:** Plezy native shelf fetchers read connected source state; official Android TV integrates its system provider.
- **Why it matters:** Server authoring can preserve restrictions/row identity without exposing reusable broad app credentials to an extension. Vega deliberately avoids third-party personalization.
- **Assessment:** **Portico ahead.** Keep narrow feed, verify refresh/removal on profile changes. Medium system integration per OS, constrained by privacy rather than imitation.

### A-21.45 No push alerts is a deliberate product boundary

- **Portico:** In-app notification/bell and no server email/push. `06 Clients.md`, Rules.
- **Jellyfin / Plezy / other:** Plezy/Streamyfin use local download notifications and external service state; closed apps may use push.
- **Why it matters:** Off-app group invites/recording failures will not arrive immediately. Local transfer notification does not contradict no push server transport.
- **Assessment:** **Trade-off (subjective).** Deliberate. Small local notifications, large opt-in push service/cert/platform work. Do not add periodic Hosted chatter to simulate push without product decision.

### A-21.46 Discovery is local/server media rather than an external ecosystem hub

- **Portico:** No native tracker/Seerr/MDBList/Marlin/Discord settings established.
- **Jellyfin / Plezy / other:** Plezy integrates multiple trackers/Seerr/MDBList; Wholphin Seerr; Streamyfin plugin/search providers. `plezy/lib/screens/settings/services_settings_screen.dart:29`, `streamyfin/utils/atoms/settings.ts:406`.
- **Why it matters:** Competitors help find/request titles absent from the server, retain watch state across libraries and expose community lists. Portico keeps metadata/private media out of unrelated services.
- **Assessment:** **Trade-off (subjective).** Medium optional adapters, large unified catalog/identity. Strong user value; preserve explicit opt-in and no Hosted media telemetry.

### A-21.47 Native Live/DVR sits beside first-party library channels

- **Portico:** Channels consolidates guide, sources, recordings and synthetic library channels. `N/src/screens/channels/ChannelsScreen.tsx`, `ProgramSheet.tsx:21`.
- **Jellyfin / Plezy / other:** Plezy guide/What’s On/recordings/rules; Android TV TV guide/category filters; Wholphin Seerr and Live TV detail.
- **Why it matters:** Portico offers a continuous channel-style catalog experience with owner management; consumer recording confirmations are inconsistent.
- **Assessment:** **Portico ahead.** Keep channel abstraction and native DVR. Medium guide/favorite/reorder polishing; small cancellation repair. Tuner/timeshift engines excluded.

### A-21.48 Native metadata repair includes provenance/locks but authentication gates matter

- **Portico:** Repair editor panels expose fields/artwork/candidates/relationships; callers require local owner and non-TV. `N/src/screens/MetadataEditor.tsx:221`, `EntryMenu.tsx:49`.
- **Jellyfin / Plezy / other:** Plezy metadata adapters gate kind/backend/admin; Swiftfin iOS editor/identify/admin.
- **Why it matters:** Portico has richer owner evidence but Hosted owner can lose an otherwise supported feature through client gate.
- **Assessment:** **Unclear.** Medium capability-driven edit policy; B-21.12. Do not assume local authority is the desired security requirement without reconciling contracts/server permission.

### A-21.49 Owner server diagnostics are rich; ordinary client diagnostics are thin

- **Portico:** Server logs/detail window/support export and about versions; no comparable local app log viewer/share established. `N/src/screens/server/Troubleshooting.tsx`, `SettingsPage.tsx:288`.
- **Jellyfin / Plezy / other:** Plezy copy/upload/clear logs/cache/settings export-import-reset; Swiftfin local logs; Android TV telemetry/developer pages.
- **Why it matters:** An app failure on phone/TV may need client facts rather than server logs. Owners and ordinary viewers need different safe support paths.
- **Assessment:** **Portico behind.** Small/medium bounded redacted client diagnostics UI. Keep strong server console; local user export must not expose stored auth/session/private catalog.

### A-21.50 Language design prioritizes English regional completeness

- **Portico:** Shared source en-US with derived en-CA/en-GB; launch English-only. `06 Clients.md`, i18n paragraph.
- **Jellyfin / Plezy / other:** Plezy bundles many language catalogs; official clients have localized resources.
- **Why it matters:** Shared exact messages prevent native/web label drift, but international first-release UX is less broad. Region/audio/subtitle/metadata languages stay separate.
- **Assessment:** **Trade-off (subjective).** Deliberate English launch. Medium translation/RTL process; small clarify unavailable languages. Source derivation does not establish VoiceOver/TalkBack conformance.

### A-21.51 Kids restrictions are server policy, not a separate playful shell

- **Portico:** Profile limits/PIN exist; ordinary media UI is shared, no special kids home/theme established. `N/src/screens/Security.tsx`, `Profiles.tsx`.
- **Jellyfin / Plezy / other:** Plezy local profile examples include Kids but do not confer server restrictions; Swiftfin admin user rating/tags/schedules.
- **Why it matters:** Server authorization is the right boundary. Shared UI may still expose confusing owner switches, external services or adult-looking artwork if policy projections fail.
- **Assessment:** **Trade-off (subjective).** Keep server restrictions; medium simplified kid shell and profile launch behavior if wanted. Acceptance must check every system shelf/offline/search/menu, not merely profile picker.

### A-21.52 External players are not a first-party UX option

- **Portico:** Integrated player/destinations only; no choose-external-app setting established.
- **Jellyfin / Plezy / other:** Plezy external-player command/package/scheme editor; Android TV external player selection; Wholphin play-with options.
- **Why it matters:** Users cannot fall back to a favorite app or test a rendering issue with one action. Portico keeps progress/queue/group behavior under one authority.
- **Assessment:** **Trade-off (subjective).** Medium external handoff with clear progress limitations; substantial correctness implications. Good optional escape hatch, but lower priority than core reachable controls.

### A-21.53 Device tuning is mostly owner policy or implicit capability

- **Portico:** Native quality/method/HDR plus server streaming pages, few client buffer/rendering toggles.
- **Jellyfin / Plezy / other:** Plezy/Android TV expose hardware/buffer/frame-rate/audio/DV/HDR preferences; Streamyfin separates engine from native chrome.
- **Why it matters:** Source reflects real device scars, but a huge settings panel can burden ordinary users. Portico should expose user-helpful tuning only when consumed.
- **Assessment:** **Trade-off (subjective).** Small UI diagnosis, medium tested opt-in controls. Engine correctness and efficacy intentionally outside this domain; absence alone is not a playback bug.

### A-21.54 Donation/about/licence experience differs

- **Portico:** About shows versions/legal/dataset footer; no native donation/licence explorer established. `N/src/screens/settings/SettingsPage.tsx:288`.
- **Jellyfin / Plezy / other:** Plezy donate/about/package licences; Swiftfin app links/about; Android TV licence list.
- **Why it matters:** Trust/support/legal attribution are tangible app screens, not simply README text. Donation is a product funding decision.
- **Assessment:** **Portico behind.** Small native credits/licence detail and support links, medium generated inventory process outside read-only audit. Donation only if Justin wants it.

### A-21.55 Direct-file clients move catalog ownership into the app

- **Portico:** Portico requires server identity/catalog; client does not browse NAS/cloud independently.
- **Jellyfin / Plezy / other:** Infuse public page supports SMB/NFS/WebDAV/cloud, iCloud and Trakt. [Public source](https://firecore.com/infuse).
- **Why it matters:** An Infuse user can start without running a server; Portico offers shared household authorization and server-owned catalog instead.
- **Assessment:** **Trade-off (subjective).** Deliberate server product identity. Large standalone/offline import/catalog mode; likely not necessary for launch parity, but worth recording rather than silently omitting.


## Missing features and capabilities to consider

This list records missing native capabilities, including deliberate omissions. It is not a proposed launch checklist. Small = mainly existing UI/core wiring; medium = new shared UI/state/integration; large = new authority/protocol/platform architecture. Priorities are opinions, not authorization to implement.

### F-21.1 Spoiler masking with explicit reveal

Plezy binds hideSpoilers to episode thumbnail/description presentation (`plezy/lib/widgets/episode_card.dart:119`). Apply a profile policy consistently to show rows, episode panel, Search, Home art and notifications.

**User value and assessment:** High value before release; define title/summary/art exceptions and clear reveal behavior. **Rough size:** Medium. **Principles:** No principle conflict; authorized presentation masking is not client catalog filtering.

### F-21.2 Pre-play version and quality selector

Swiftfin playback button has source/version/quality/audio/subtitle pickers (`swiftfin/Shared/Views/ItemView/Components/ActionButtons/PlaybackActionButton.swift:66`); Plezy version split button. Portico offers exist but PlaybackChoices intentionally defers version.

**User value and assessment:** High priority for editions/cuts/multiple files. **Rough size:** Medium. **Principles:** Reuse server offers and exact source identity; no client scan or speculative playback.

### F-21.3 Exact subtitle preselection and SDH/audio-description preferences

Plezy upstream account preferences include forced/SDH/smart foreign-audio; Swiftfin picks actual streams. Portico language dedup loses same-language track identity.

**User value and assessment:** High value, especially accessibility and anime. **Rough size:** Medium. **Principles:** Compatible with shared language/track policy; cannot silently infer desired accessible track.

### F-21.4 Native grid/list/card-density options

Plezy density/spacing/grid-list; Swiftfin columns/poster types; Android TV per-library display. Bind Portico cardSizePercent and an authored native list view.

**User value and assessment:** Useful on tablets and large libraries; keep default fixed art policy unless changed. **Rough size:** Medium. **Principles:** Both modes must remain bounded and preserve semantic item actions.

### F-21.5 Two-pane tablet Settings and regular-width media workspaces

Swiftfin regular headers/list columns; Plezy landscape Now Playing two-pane at 800. Portico wide rail does not make settings two-pane.

**User value and assessment:** High value and explicitly known Backlog work. **Rough size:** Medium. **Principles:** No principle conflict; semantic order should remain stable under narrow multitasking.

### F-21.6 Handheld gesture controls with visible alternatives

Plezy brightness/volume/pinch; Swiftfin pan/swipe/hold/multitap/two-touch; Streamyfin seek/hold speed/double tap/haptic switch.

**User value and assessment:** Strong ergonomic parity gain after basic controls are stable. **Rough size:** Medium. **Principles:** Accessibility requires explicit buttons; resolve Back/scroll/scrub conflicts.

### F-21.7 Keyboard shortcuts and pointer-complete tablet navigation

Plezy recorder/editor detects collisions, reset and player/navigation actions. Portico no equivalent app shortcut editor established.

**User value and assessment:** Prioritize common play/pause/search/back/focus rather than a large editor first. **Rough size:** Medium. **Principles:** Do not hijack OS keys or active text editor.

### F-21.8 Multi-server merged Home/search/library source selection

Plezy multiple connection registry aggregates and labels sources; partial search errors remain visible.

**User value and assessment:** Valuable to the named competitor’s audience; not a small cosmetic port. **Rough size:** Large. **Principles:** Cross-server identity, restrictions, dedup, state ownership and Hosted privacy require explicit design.

### F-21.9 Rolling keep-N offline episode rules

Plezy sync rules retain N unwatched and reconcile specials/rule-owned files. Portico one-shot prepare/batch exists.

**User value and assessment:** High value for travel; state and deletion ownership should be specified first. **Rough size:** Medium/large. **Principles:** Background revision-driven work fits scale; do not require Hosted polling or full client lists.

### F-21.10 Grouped and batch download management

Plezy download tree groups library/show/season/album/collection, batch pause/resume/retry/delete.

**User value and assessment:** Useful even without rolling rules; Portico local/server separation should remain. **Rough size:** Medium. **Principles:** Window groups and retain exact receipt scope; no arbitrary silent cleanup.

### F-21.11 Android background-download health screen

Plezy checks background restrictions/standby/notifications/data saver and links to device-specific help.

**User value and assessment:** High support value on real Android devices; do not imply OS background guarantees. **Rough size:** Small/medium. **Principles:** No new media authority; explain only observed restrictions.

### F-21.12 Download Live Activity / local completion entry

Streamyfin iOS download Live Activity plus Lock Screen/Dynamic Island setting (`streamyfin/utils/atoms/settings.ts:419`).

**User value and assessment:** Optional polish with good visibility; privacy-safe title policy needed. **Rough size:** Medium. **Principles:** Local progress surface is compatible with no server push.

### F-21.13 Complete playlist/collection native CRUD and sharing

Plezy creation/name/add/remove/reorder/delete; Portico current playlist remove/reorder and entry pickers are partial.

**User value and assessment:** High value if Saved is a first-class destination; no need to copy full-load editing gates. **Rough size:** Medium. **Principles:** Use stable occurrence IDs and server grants, preserve duplicate semantics and confirmation.

### F-21.14 TV creation of Saved Views

Portico handheld browse can name/save query; inline TV toolbar lacks Save View; existing views can be added to Home.

**User value and assessment:** Useful Portico-only strength to expose, not just competitive copying. **Rough size:** Small/medium. **Principles:** Remote naming should use native keyboard; query kept on server.

### F-21.15 Broader context actions and consistent immediate detail actions

Plezy richer context rate/collection/edit/file info; Swiftfin action-bar/menu configuration. Portico model union does not mean callers expose all.

**User value and assessment:** High priority consistency and reachability; fixed canonical placement can remain. **Rough size:** Small/medium. **Principles:** Availability must come from resource permission, not client role guesses.

### F-21.16 Native share links and share-sheet actions

Portico links exist but no ordinary native item Share action was established. Plezy catalog/external-service navigation is related but is not credited as a proven native share-sheet equivalent; this is a capability suggested by the available link grammar.

**User value and assessment:** Useful household workflow; ensure link resolves selected server/viewer correctly. **Rough size:** Small/medium. **Principles:** Allowlisted route IDs only; never grants/tokens/paths in shared URL.

### F-21.17 Play-on-device from detail and permanent TV-code entry

Portico Play on currently focuses active playback receiver/Cast/AirPlay; onboarding contract requires full-size manual code peer regardless of discovery.

**User value and assessment:** Repair first; improving detail entry comes afterward. **Rough size:** Small. **Principles:** Matches current contract; retain manual path without LAN permission.

### F-21.18 Screensaver with profile-safe art and idle policy

Android TV LibraryDreamService and age-filter prefs; Wholphin screensaver configuration.

**User value and assessment:** Good TV polish; OS screensaver delegation may be enough if deliberate. **Rough size:** Medium. **Principles:** Never broaden profile content; no continuous Hosted chatter.

### F-21.19 Clock, finishes-at and elapsed/remaining display options

Android TV clock policy and Plezy system/finish-time widgets; Wholphin ShowClock.

**User value and assessment:** Worth offering for long TV sessions and sleep timers. **Rough size:** Small. **Principles:** No engine change necessary; use locale/hour cycle.

### F-21.20 TV theme music and backdrop/focus personalization

Wholphin theme-song volume; Android TV backdrop/clock/theme/watched; Plezy corner-backdrop/focus-glow/effects.

**User value and assessment:** Optional; add only useful reviewed choices, not every historical toggle. **Rough size:** Medium. **Principles:** Autoplay ambiance/reduced motion/sensory rules require contract reconciliation.

### F-21.21 App-owned game controller mapping and hints

Plezy universal_gamepad and semantic key synthesis; Portico Siri Remote touch is not full controller mapping.

**User value and assessment:** Important for accessibility and TV boxes; validate real controllers. **Rough size:** Small/medium. **Principles:** Standard activation/back must remain equivalent and repeat-safe.

### F-21.22 Phone companion remote with text/navigation

Plezy MobileRemoteScreen offers Remote/Play/More tabs, transport/volume/navigation/search. Portico receiver handoff moves playback, not a general D-pad remote.

**User value and assessment:** Strong household value when TV text entry/focus is painful. **Rough size:** Medium. **Principles:** Use existing authorized receiver context; retain remote command ownership.

### F-21.23 Optional Seerr requests and catalog availability UI

Plezy/Wholphin/Streamyfin request absent titles, seasons, 4K and destination options with permissions.

**User value and assessment:** One of the most valuable ecosystem gaps; scope a minimal request path first. **Rough size:** Medium/large. **Principles:** Explicit opt-in external service; preserve no Hosted private catalog telemetry.

### F-21.24 Trakt/anime trackers/MDBList discovery

Plezy scrobbling/watched sync/catalog rows; Streamyfin popular/plugin lists; Infuse public Trakt.

**User value and assessment:** Useful for migrators; server-owned state must remain authoritative locally. **Rough size:** Medium/large. **Principles:** Opt-in, clear remote write conflicts and privacy; no implicit external sync.

### F-21.25 Search provider choice and richer suggestions

Streamyfin Jellyfin/Marlin/Streamystats choice; Plezy external catalog search. Portico recent/grouped results are server-local.

**User value and assessment:** Consider provider adapter only if server search needs measured improvement. **Rough size:** Medium. **Principles:** No arbitrary unbounded client query scans.

### F-21.26 Native client diagnostics/log sharing for ordinary viewers

Plezy logs copy/upload/clear; Swiftfin local logs; Portico owner server exports cannot explain every UI/device failure.

**User value and assessment:** High support value during Android/Vega pre-release acceptance. **Rough size:** Small/medium. **Principles:** Bounded/redacted diagnostics, no auth/native memory/catalog dump.

### F-21.27 Settings backup/import/reset and safe cache clear

Plezy exports/imports preferences with confirmation and cache-clear/reset actions.

**User value and assessment:** Useful recovery/migration; lower priority than setting-consumer correctness. **Rough size:** Small/medium. **Principles:** Exclude native credentials and viewer-private state; do not export the whole secure store.

### F-21.28 Native licences/credits/support/donation pages

Plezy package licence explorer/donation/About; Android TV licences. Portico About is version/legal footer.

**User value and assessment:** Add clear licence/attribution inspection before promotional donation UI. **Rough size:** Small. **Principles:** No conflict; funding method is Justin’s decision.

### F-21.29 Launch profile prompt and simplified kids workspace

Plezy requireProfileSelectionOnOpen; Swiftfin server parental permissions and local security.

**User value and assessment:** Useful shared-device option; separate from a decorative kids theme. **Rough size:** Medium. **Principles:** Client prompt supplements server policy and cannot weaken PIN/restrictions.

### F-21.30 Android assistant/launcher quick actions

Portico Apple App Intents exist; Android Activity adapter no-op.

**User value and assessment:** Platform parity opportunity after Apple index bug is repaired. **Rough size:** Medium. **Principles:** Narrow opaque identifiers and verified current scope; no whole catalog export.

### F-21.31 SharePlay and native OS multiuser association

Apple tvOS guidance encourages easy multiuser; server Together is not SharePlay.

**User value and assessment:** Optional premium-feeling polish; existing room correctness comes first. **Rough size:** Large. **Principles:** Requires GroupActivities/OS lifecycle/identity adaptation and product approval.

### F-21.32 TV PiP

Apple tvOS public design guidance recognizes PiP; Portico deliberately disables all television PiP.

**User value and assessment:** Record as supported-platform feature omission, not automatic gating bug. **Rough size:** Medium. **Principles:** Conflicts with current full-screen-only TV rule; needs explicit product change.

### F-21.33 Reviewed light/daylight appearance and optional accent

Plezy light/system/OLED and Swiftfin accent.

**User value and assessment:** User comfort can justify a light variant; arbitrary colors are less important. **Rough size:** Medium. **Principles:** AMOLED conflicts with non-black atmosphere; all variants require contrast/focus review.

### F-21.34 External-player escape hatch

Plezy configurable external apps and Android TV selection; Wholphin play-with.

**User value and assessment:** Useful optional fallback, but not substitute for first-party acceptance. **Rough size:** Medium. **Principles:** Explain progress/queue/group limitations; never expose reusable credentials in URLs.

### F-21.35 Standalone file browsing/import mode

Infuse public SMB/NFS/cloud and local transfer workflow; no server required.

**User value and assessment:** Lower priority for launch; keep as a deliberate product boundary rather than claimed parity. **Rough size:** Large. **Principles:** Changes Portico’s thin-client/server-authority architecture.

### F-21.36 Photo libraries/slideshow

Android TV photo player and Wholphin photo-album folders; Portico Gallery is a design gallery.

**User value and assessment:** A tangible non-video library gap, probably separate release scope. **Rough size:** Medium/large. **Principles:** Requires real media-kind/catalog contracts; never reuse development Gallery as a photo app.


## Possible bugs and risks in Portico

All entries are source findings or explicitly labelled risks; none was reproduced on hardware. “Confirmed by reading” confirms the stated code path/mismatch, not every downstream runtime symptom. No Critical issue is asserted.

### B-21.1 Play on has no permanent Enter a TV Code action

When discovery returns nothing or Local Network access is denied, the active-player destination sheet offers AirPlay/Cast/nearby receivers or helper text, but no manual TV-code button. Settings has Link a TV, so the feature exists elsewhere; the contracted peer action is missing exactly where users look for an output.

**Evidence:** `N/src/player/Destinations.tsx:90–136`; `C/presentation/destinations.ts:48–58` emits only AirPlay/Google Cast/Portico device rows. `C/presentation/settings-structure.ts:277` proves the separate settings action. `onboarding/contract.md`, §7 and `visual/contract.md`, §17.4 require the manual action even with no LAN discovery.

**How comparison surfaced it:** Plezy companion discovery/manual-address paths prompted checking the no-nearby recovery.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already known:** Not explicitly listed in Backlog.

**Assessment / possible correction:** Add full-size manual entry without requiring discovery permission; use existing LinkTV flow, keeping caller playback context.

### B-21.2 Programme-sheet Cancel Recording bypasses confirmation

Selecting Cancel on a scheduled programme immediately calls the DVR cancellation method. The same operation in Recordings uses an explicit confirmation, so confirmation behavior depends on entry surface. A stray remote/touch activation can remove a planned recording without a consequence preview.

**Evidence:** `N/src/screens/channels/ProgramSheet.tsx:59–73` calls `dvr.cancel`; `:168` button directly runs `record('cancel')`. `N/src/screens/Recordings.tsx:235` uses ConfirmSheet for cancellation. `media-presentation/contract.md`, §12.2 requires confirmed destructive cancellation.

**How comparison surfaced it:** Plezy cancel-recording title/message/confirmation and Portico’s own Recordings path surfaced the inconsistency.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already known:** Backlog’s older “recording actions not exposed” is stale; this exact confirmation gap is not listed.

**Assessment / possible correction:** Use the same consequence-aware cancellation sheet, including whether partial recorded bytes remain.

### B-21.3 Wide-tablet Downloads loses its primary navigation entry

At tablet width >=700 the shell uses Rail rather than BottomBar. The Rail declares Downloads in its key type but does not include it in primary items, and receives no showDownloads gate. Downloads remains reachable through Settings → Device → storage or a toast; this is a primary-destination gap, not an entirely unreachable screen.

**Evidence:** `N/src/shell/Shell.tsx:128–129`, `:315–324`; `N/src/shell/Rail.tsx:14–24`, `:117`; `N/src/screens/settings/SettingsPage.tsx:283–285`. `navigation-and-input/contract.md`, NAV-5.1 makes Downloads a primary destination on offline-capable clients.

**How comparison surfaced it:** Plezy tablets retain their Downloads destination; width-specific inventory exposed this switch.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already known:** Not listed; Backlog lists tablet settings layout separately.

**Assessment / possible correction:** Offer Downloads in non-TV rail using the same capability gate as bottom bar. Test narrow/wide transitions while Downloads is current.

### B-21.4 Saved Spotlight and Siri index reads a deliberately absent Home row

Home indexes Saved candidates by searching its rows for watchlist/watchlisted. Product explicitly removed My List from Home, so ordinary Saved-only titles have no feed into this index. Each Home index refresh replaces the saved domain with the empty list. This narrows Siri title choices and Spotlight Saved despite the native module promising that integration.

**Evidence:** `N/src/screens/Home.tsx:43–49`; `N/src/bridge/NativeActivity.ts:10–12`; `N/Native/PorticoActivity.m:79–113` deletes domains then indexes received items. `media-presentation/contract.md`, §14.2 forbids Home My List row.

**How comparison surfaced it:** Comparing system-integration claims with actual screen feeders surfaced the stale architecture assumption.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already known:** Not listed. My List naming/no Home shelf is explicitly documented.

**Assessment / possible correction:** Fetch a bounded, viewer-scoped Saved projection independently; do not put My List back on Home merely to repair indexing.

### B-21.5 History and adjacent appended saved lists grow unwindowed

History retains appended pages, rescans/deduplicates all accumulated entries, maps day groups and renders nested grids at full height. Grid explicitly maps every row when scrollEnabled=false. Playlists, resource lists and saved-resource grids have similar accumulated mapping paths. Work grows with visited content rather than visible content, threatening long-session TV/tablet responsiveness.

**Evidence:** `N/src/screens/Saved.tsx:288–320`, `:348–366`; `N/src/ui/Grid.tsx:49–58`; `N/src/screens/title/SavedPages.tsx:140–172`, `:230–236`. My List/favorites do use WindowedGrid, so this is not a claim that all native lists are unbounded.

**How comparison surfaced it:** Plezy sliver/card budgets and Portico O(visible) rule motivated examining every list, not only library grids.

**Confidence:** Confirmed by reading. **Severity:** High. **Already known:** Saved History whole-list rendering is explicitly listed at Backlog line 65; adjacent same-pattern views are additional scope.

**Assessment / possible correction:** Use a single virtualized/windowed owner per screen with stable day headers/occurrence keys. Do not impose an arbitrary maximum history length.

### B-21.6 Published native preferences can be consumed or stored without a reachable control

Native Search reads search.rememberHistory and shell reads notifications.badges, while their authored rows are web-only. cardSizePercent is stored server-side but native fixed metrics do not consume it; showBackdrops similarly has no native preference consumer in inspected screens. These are separate effects: hidden control for a consumed privacy choice, versus unused appearance field.

**Evidence:** `C/presentation/settings-structure.ts:243–250`, `:268`; `N/src/screens/Search.tsx:64`; `N/src/shell/Rail.tsx:39`; `N/src/ui/theme.ts:114–145`. Registry `operations/preferences.go:202`, `:211–215` publishes them.

**How comparison surfaced it:** Competitor settings inventories required mapping each field to both rendering and consumption.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already known:** Poster-size stored/not-applied is known at Backlog line 63. Hidden remember-history/badge controls and backdrop semantics are not explicitly listed.

**Assessment / possible correction:** Make scope/platform expectations explicit; expose native privacy choices and either consume native appearance values or clearly restrict their applicability.

### B-21.7 This device wording actually controls the whole device class

Native quality/method/HDR rows append “This device,” but writes use the server’s profile-device-class scope and mobile/television class. There is no installation ID in that scope. For the same profile/server, the setting is shared by that class, contrary to the row’s wording.

**Evidence:** `C/preferences.ts:5–6`; `C/presentation/settings-structure.ts:216–232`; `N/src/screens/settings/SettingsPage.tsx:134–151`; `N/src/app/server-preferences.tsx:84–96`. Installation-local Reduce Motion is genuinely different and is not implicated.

**How comparison surfaced it:** Plezy explicitly separates account preferences from local preferences; that exposed Portico’s conflation.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already known:** Not listed.

**Assessment / possible correction:** Either label “Phones and tablets”/“Televisions” for the actual scope, or define and implement a true installation override. Do not silently change storage authority in UI.

### B-21.8 Playing a duplicate playlist occurrence starts the first matching item

Playlist tap uses occurrence.media and a sliced list but passes whole playlist container with itemId anchor only. The server finds the first matching item anchor. Selecting the second copy of a repeated movie/song cannot express that occurrence, even though remove/reorder uses occurrence IDs.

**Evidence:** `N/src/screens/title/SavedPages.tsx:150–168`; `N/src/app/container-play.ts:59–65`; `N/src/player/usePlayer.ts:439–452`; `apps/portico-server/server/internal/playbackv1/queue_build.go:269–270` selects first matching item, and `:332–333` supports positional anchor.

**How comparison surfaced it:** Plezy playlist ordering and shared contract’s duplicate-entry law prompted tracing item versus occurrence identity.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already known:** Not listed.

**Assessment / possible correction:** Carry source occurrence/absolute position through container selector. Preserve authorization filtering and playlist revision; an array index into a partial loaded page is insufficient.

### B-21.9 Pre-play subtitles omit unknown-language tracks and merge distinct same-language choices

PlaybackChoices filters subtitle tracks to non-empty language !=und, deduplicates by language, and writes a language intent. A valid track labelled unknown has no pre-play row; normal and SDH/forced tracks sharing English become one indistinguishable choice. The row may also vanish when there is only one audio and no known subtitle language.

**Evidence:** `N/src/screens/title/PlaybackChoices.tsx:40–50`, `:57–66`. In-player subtitle menu is richer and does not fix pre-play information loss.

**How comparison surfaced it:** Swiftfin selects stream identity; Plezy displays track labels and SDH policies, exposing the information discarded in this native chooser.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already known:** Not listed. Version picker omission is intentional source comment, but this subtitle loss is not listed.

**Assessment / possible correction:** Use stable track/resource/source identity and display title/kind; retain Automatic/Off. Label defaults as intent rather than promised effective output.

### B-21.10 Vega exposes unsupported speed and audio-effect preferences

The shared settings context does not pass playbackSpeed/audioEffects capabilities, so both default to supported. Vega README explicitly says supportedRates=[1] and no gapless/crossfade edges; Vega native listening override returns false. The settings renderer therefore exposes video/book speed, gapless, crossfade and leveling choices without target-specific evidence they can take effect.

**Evidence:** `N/src/screens/settings/SettingsPage.tsx:42–55`; `C/presentation/settings-structure.ts:190–201`, `:233–237` capability gates; `V/README.md`, Playback; `V/src/overrides/native/bridge/native-listening-kinds.ts:13`. Leveling effectiveness specifically remains unestablished; speed and gapless/crossfade mismatch is explicit.

**How comparison surfaced it:** Competitor capability/platform gates and port differences prompted checking whether shared settings are actually gated.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already known:** Vega log notes speed should not be offered; Backlog does not enumerate this current settings-context omission.

**Assessment / possible correction:** Pass actual UX capabilities from the target runtime. Hide unsupported controls or describe supported fallback honestly; do not advertise preference persistence as effect.

### B-21.11 Saved View Play and Shuffle only act on the loaded prefix

SavedResourceScreen supplies no container for a view. usePlayList filters loaded media and locally shuffles it; it does not ask the server to play the complete saved query. After one page, “Play”/“Shuffle” can imply the full resource while selecting only the visited subset.

**Evidence:** `N/src/screens/title/SavedPages.tsx:24–36`, `:199–209`, `:221`. Named collection/playlist containers use whole server selectors and are not implicated.

**How comparison surfaced it:** Portico’s correctness-over-limits rule and competitor random/shuffle UX surfaced the distinction between query and loaded array.

**Confidence:** Confirmed by reading. **Severity:** Medium. **Already known:** Not listed.

**Assessment / possible correction:** Add a query/view selector with revision/authorization semantics, or clearly label “Play loaded items.” Avoid loading all view results to hide the limitation.

### B-21.12 Hosted owners are blocked from native metadata edit by authority gate

Metadata edit availability is guessed from role=owner AND authority=local AND non-TV. A Hosted owner cannot reach native edit from detail, show or item menu even where the server resource could authorize it. The code establishes the client restriction; whether it is desired is an open policy decision.

**Evidence:** `N/src/player/EntryMenu.tsx:49`; `N/src/screens/Show.tsx:64`; `C/presentation/detail-actions.ts:105` computes canEdit from owner/local/platform. Native Server pages are owner role based (`SettingsPage.tsx:41–44`), making the authority-specific asymmetry visible.

**How comparison surfaced it:** Plezy backend/admin adapter checks and Swiftfin native admin edit prompted comparing actual resource permission with authentication method.

**Confidence:** Probable. **Severity:** Medium. **Already known:** Not listed.

**Assessment / possible correction:** Reconcile intended metadata permission first, then capability-gate resource actions. Do not assume direct authentication is required simply because current UI checks it.

### B-21.13 Saved hook can skip selecting a replacement service with the same route

useSavedService recreates service on api/scope change, resets snapshot, but its “already selected” ref remembers only the JSON route. If service changes while the route remains My List/favorites, the select effect returns early. The replacement service stays idle until a different route/retry path occurs. usePersonalService and useContent already include service identity in their equivalent guard.

**Evidence:** `N/src/screens/Saved.tsx:50–73`; compare `:95–99` and `N/src/app/content.ts:31–37`. `N/App.tsx:27` mounts Shell without an identity key; exact source transition triggering this requires runtime validation.

**How comparison surfaced it:** Cross-client server/profile retention checks and Portico’s own corrected sister hooks surfaced it.

**Confidence:** Probable. **Severity:** Medium. **Already known:** Not listed.

**Assessment / possible correction:** Key the selection guard by service and route, then verify token/API replacement and profile/server transition outcomes. Do not claim a demonstrated data leak.

### B-21.14 Async item deep links have no visible intent/viewer fence

Shell captures a navigator, starts item/detail lookup, then plays or pushes on completion without checking that the viewer/route/intent is still the one that received the link. Duplicate links use push instead of coalescing. A profile/server switch or a newer link while lookup is pending can publish obsolete navigation intent.

**Evidence:** `N/src/shell/Shell.tsx:258–268`; item/play deep-link handlers have no local generation/abort check. Lower core authorization can reject content, so the precise runtime consequence is not established. NAV-11 and NAV-15 require fenced/coalesced pending intents.

**How comparison surfaced it:** System shelf/deep-link comparison and Portico navigation contract highlighted late-result handling.

**Confidence:** Probable. **Severity:** Medium. **Already known:** Not listed.

**Assessment / possible correction:** Use one scoped pending intent owner, latest generation and semantic route coalescing. Verify cold/warm links with offline route, switch and late response.

### B-21.15 Back from filters commits live changes rather than restoring a draft

Filter values and Clear call onChange directly into Library predicates. Sheet has Done and onClose but no saved draft/rollback. Therefore Back/close keeps changes already made, while NAV-6 says cancel/Back restores last committed filters. This is a confirmed contract mismatch and may be intentional live-filter UX, not necessarily a wrong design.

**Evidence:** `N/src/screens/BrowseFilters.tsx:37`, `:54–71`; `N/src/screens/Library.tsx:79`; `navigation-and-input/contract.md`, NAV-6.7.

**How comparison surfaced it:** Plezy filter confirmation/control semantics prompted examining cancel versus apply.

**Confidence:** Confirmed by reading. **Severity:** Low. **Already known:** Not listed.

**Assessment / possible correction:** Choose live-apply with explicit product law, or implement staged Apply/Cancel. Do not silently treat the dismiss gesture as approval of hidden draft changes.

### B-21.16 TV long-press menus lack a proven visible accessible alternative on some card paths

Recommendation cards expose Not interested through long press; playlist manageable entries show reorder/remove only through long press. Shared Card/ListRow supports that event, but a visible More button/custom accessible action is not established on these exact call sites. Real remote hold/VoiceOver/Switch Control behavior must be checked before declaring these paths complete.

**Evidence:** `N/src/screens/Home.tsx` recommendation card path; `N/src/screens/title/SavedPages.tsx:167–168`; `N/src/screens/Saved.tsx:391–392`; `accessibility-and-language/contract.md`, A11Y-4.9.

**How comparison surfaced it:** Plezy focusable popup/menu and explicit action affordances highlighted alternate-input reachability.

**Confidence:** Speculative — needs a runtime check. **Severity:** Medium. **Already known:** tvOS Not interested reachability is explicitly listed at Backlog line 9. Playlist management alternative is additional scope.

**Assessment / possible correction:** Add a visible/semantic alternate where needed; validate Siri Remote, ordinary Android remote, Vega long-select and VoiceOver/TalkBack. No general runtime focus trap was reproduced.


## Where Portico is stronger

1. **Catalog scale is an explicit shared-client law.** Library browse, alpha jump, server selectors and WindowedGrid avoid asking phones/TVs to load, sort or rank a whole catalog. The exceptions in B-21.5 should be repaired without abandoning that architecture.
2. **Shared semantic authoring is unusually disciplined.** Settings, tab order, card captions, item action IDs and media geometry have one place to change. Separate official Jellyfin clients demonstrate how much product divergence this can prevent.
3. **Native owner operation is broad.** The phone/tablet Server pages include configuration, state, jobs, history, backups and diagnostics. Swiftfin is a credible benchmark for some native admin capabilities, but Portico covers more first-party server operations in its authored structure.
4. **Server-backed Saved Views can become personal Home rows.** This makes customization meaningful beyond theme and card size, while preserving current permissions and library truth.
5. **Whole-container playback and relative playlist mutation do not need the whole container loaded.** Plezy’s full-load editing gate illustrates a trade-off Portico can avoid. Occurrence-aware playback still needs fixing.
6. **Downloads distinguish preparation, transfer, local availability and authorization.** Users should never be told a download is ready merely because the server prepared bytes. The offline entry path is implemented separately from online sign-in.
7. **Music and audiobooks have deliberate different chrome.** Book speed/skip/chapter/sleep and music gapless/leveling/lyrics are not shoehorned into a generic video detail screen. Vega capability mismatches must be resolved before advertising those effects across all targets.
8. **Privacy boundaries are visible in the design.** No implicit Hosted media history, no push dependency, narrow shelf token, account/server separation and scoped personal state are strengths even when they make third-party integrations harder.
9. **TV/input decisions are written down.** Native focus, root Back, large actions, fixed player grammar, manual code fallback and reduced-motion behavior are explicit requirements. Competitor parity should strengthen those outcomes, not silently override them.
10. **Errors and refresh are supposed to preserve the product frame.** Screen boundaries, connection status, retained projections and meaningful empty states are present. Source review supports the intent; device/session transitions still require acceptance.

These are architectural advantages, not claims that every physical target is polished or that the current release meets its performance/accessibility contracts.

## Doc/code disagreements

| Written claim / requirement | Actual source | Consequence / assessment |
|---|---|---|
| NAV-5.1 replaces a primary root and clears former secondary history | Roots keeps per-section pushed stack and returning restores it (`N/src/shell/Roots.tsx:5`, `Shell.tsx:177`) | Deliberate source behavior needs current contract decision. Reuse of one LibraryRoot adds a separate per-library continuity limitation. |
| Settings authoring comment expects two-pane iPad | `N/src/screens/Settings.tsx:27` chooses PhoneSettings for every non-TV | Known Backlog tablet gap. Do not advertise wide rail as two-pane Settings. |
| Native Saved/Search NOTES say screens unwired/placeholder, playlist route absent, history clear/playlists not bound | `N/src/shell/Shell.tsx:94–107` registers real screens; `Saved.tsx:103` and `:322` clear history; `SavedPages.tsx:90–111` move/remove | NOTES are obsolete. Their remaining whole-history warning is still real. |
| NOTES-live says tune stub/recording absent/lead import needed | Current `channels/ChannelsScreen.tsx`, `ProgramSheet.tsx:63–69`, `Recordings.tsx` have recording/tune UI | Older notes and Backlog absence statements should not be used as current acceptance status. |
| Media presentation §7.3/§15 requires immediate Save/favorite/playlist/rate when authorized | `C/presentation/detail-actions.ts:1` quick row only My List/favorite/watched; rate/playlist in More | Explicit authored model contradicts newer contract. Resolve desired density, not just a platform styling patch. |
| Media presentation §7.3/§15.1 requires version/quality choice before play | `N/src/screens/title/PlaybackChoices.tsx:12` says Version joins later and renders only audio/subtitle language | Source intentionally defers part of current contract. No version selector was established in pre-play UI. |
| Onboarding §7 / Visual §17.4 requires permanent manual TV-code peer in Play on | `N/src/player/Destinations.tsx:90` omits it; Settings LinkTV exists | B-21.1. This is a concrete missing action, independent of discovery quality. |
| Profile/device local settings wording | Quality/HDR/method actually store profile-device-class; local preferences genuinely installation-scoped | B-21.7. Words need to identify the actual scope. |
| Rules: O(visible), bounded lists | Saved History, resource/playlist accumulated views map all entries | Known history plus adjacent B-21.5. |
| Rules: hide only server-disabled features, not platform catch-up | TV settings intentionally Dashboard-only; metadata local-owner/non-TV gating; downloads TV exclusion | Some are legitimate documented platform adaptations. The general rule is too broad unless capability/permission exception language is explicit. Hosted-owner edit needs a decision. |
| Native Activity promises Saved Spotlight/Siri | Home feeder looks for removed watchlist row | B-21.4; supported integration exists but its input is stale. |
| Settings capabilities allow speed/audio-effect omission | Shared native context leaves defaults true even on Vega | B-21.10. Capability plumbing, not shared screen existence, decides honest controls. |
| Generic settings-destinations vocabulary | `C/settings-destinations.ts:2` lists personal/listening/account/about/libraries/etc.; actual native Settings addresses use profile/playback/languages/quality/server-* | The two vocabularies should be reconciled/clearly scoped. Not every constant is a registered native screen. |
| Rating state contract says integer 0–10 | Native RateSheet describes server 0.5–5 and only renders 1–5 whole stars (`N/src/screens/title/TitleSheets.tsx:10–31`) | Contract/data/presentation scale needs reconciliation; UI is less granular than its own half-star claim. Do not infer corrupted rating writes without tracing server storage (outside this domain). |
| Playlist/collection art is server-declared presentation family | Playlist hero hardcodes square, saved-resource hero poster (`N/src/screens/title/SavedPages.tsx:121`, `:216`); media children use own kind | Family handling is incomplete in this renderer. A video playlist can be shown as a square hero. This is a confirmed presentation mismatch, with user impact needing screenshots. |
| TV focus cannot change durable selection on mere focus | TV settings column calls setAddress onFocus and marks found page selected (`N/src/screens/Settings.tsx:173`) | Deliberate preview-style settings behavior; decide whether “selected page” is permissible preview or change contract. No mutation occurs just by focus in this path. |
| Product card art restricts landscape episodes to show workspace | Architecture's Rules explicitly allows Search Episodes stills; current Search has its own shape rules | Recorded October design and September normative baseline need one current exception/reference. Do not auto-change Search based on the older general sentence. |

The port logs are dated evidence, not a promise that devices were accepted. The architecture’s “Apple app” label and “ports in flight/not merged” status also need contextual interpretation against the working tree as audited; this review did not mutate branches or attempt to reconcile deployment state.

## Open questions for Justin

1. Should settings labelled This device truly be installation-specific, or should the UI explain that all phones/tablets or all TVs in this profile share those choices?
2. Which navigation rule is current: per-tab stack retention, or clearing pushed history when switching roots? Should each named library retain independent state?
3. Is direct/local authentication intentionally required for metadata editing, even for a Hosted owner with server permission?
4. Are native rate/playlist/favorite actions meant to be immediately visible according to the contract, or does the authored More-heavy row supersede it?
5. Which appearance choices belong in Portico: reviewed light/system modes, density/list views, accent, or the fixed theatre palette only? AMOLED would require a deliberate contract change.
6. Should unwatched spoiler protection mask art/summary/title by default, optionally, or only in show detail? Should “show spoilers” be per episode, per show, or temporary per visit?
7. How much native container authoring should launch include: create/rename/delete/share, full collections, saved query creation on TV, or browsing plus playlist move/remove?
8. Should a Saved View Play/Shuffle cover the entire current query, and which revision/sort snapshot should define it?
9. Should a TV be a receiver that cannot offer PiP/external output, or should tvOS PiP become a supported product option?
10. Is there a preferred TV idle outcome: OS screensaver, profile-safe Portico artwork, pause/exit, or no in-app idle surface? Should the clock/theme music be configurable?
11. Does the launch target require merged multiple-server discovery, or is server switching an explicit boundary worth explaining in onboarding?
12. Which external ecosystem is most valuable first: Seerr requests, Trakt, anime trackers, MDBList, or none? Should those integrations be client-owned or server-owned?
13. Is a general phone companion remote wanted in addition to playback handoff and TV-code approval?
14. Which tablet environments are acceptance targets: narrow Split View, Stage Manager resizing, keyboard/trackpad, external display and rotation? Is a dedicated second-window player wanted?
15. Should native clients have ordinary-viewer log sharing/settings export/cache reset, separately from owner server diagnostics? What support destination should an export use?
16. Should profile selection be requested each app launch on shared devices, and is a simplified kids shell part of launch scope?
17. Is the native star UI deliberately whole-star only despite the half-star server description, and which stored rating scale is canonical?
18. Should native Home/system integrations index a bounded My List independently of Home? What titles should Siri have permission to offer on a locked/shared device?
19. Which existing product contract exceptions authorize Search episode stills, fixed TV-only exclusions and focus-to-preview Settings? Those decisions need current traceable ownership.
20. Which physical Apple/Android/Fire/Vega devices, remote generations and accessibility modes will define acceptance? The existing port logs explicitly do not establish that boundary.

No response was needed to complete this audit; these remain product decisions and follow-up validation questions.

## Evidence boundary

This is a static review of the working trees supplied on 11 October 2026. It establishes route/control/storage/gate paths and documented architectural choices. It does not establish rendered visual density, remote focus reachability, text scaling at maximum sizes, VoiceOver/TalkBack/Switch Control conformance, haptic feel, OS keyboard dictation, actual iPad multitasking behavior, downloads in background, Top Shelf/Watch Next publication timing, launcher capabilities, physical CEC/controller handling or performance at library scale.

No player decoder/delivery/transcode internals were audited. Source calls and visible tuning choices are indexed only as user experience. Music edge behavior, PiP codec support, subtitle burn-in fidelity, cast delivery and receipt cryptographic correctness need their own domain evidence. No Portico server/Hosted endpoint was contacted; no builds/tests/installs or repository changes were performed.

Plezy was read most deeply, including the English catalogue end to end and UI/platform/context paths. Swiftfin and Android TV settings were comprehensively indexed; specialized admin/editor subforms, every legacy Android TV browsing adapter, every device-specific native module and every comparison test were not fully traced end to end. Wholphin and Streamyfin were lighter source comparisons focused on settings, Home/detail/actions/integrations. The appended source indices distinguish screen modules and model bindings from proven reachable features. A setting string or exported component is never a runtime acceptance claim.

Infuse/Plex/Apple evidence is public documentation fetched as text on the review date. Infuse feature claims do not establish usability or paid-tier behavior on every device. Public sources used here: [Infuse](https://firecore.com/infuse), [Plex big-screen customization](https://support.plex.tv/articles/customizing-the-apps/), [Apple tvOS design](https://developer.apple.com/design/human-interface-guidelines/designing-for-tvos), [Apple iPadOS design](https://developer.apple.com/design/human-interface-guidelines/designing-for-ipados). The attempted specific Firecore iCloud support article returned 404; iCloud claims are supported only by the fetched feature page. No precise Netflix feature inventory or current Plex mobile settings inventory was established, so they remain convention references and open coverage limits, rather than asserted parity.

This audit recommends no source edits by itself. The architecture-first conclusion is to preserve bounded server-owned catalog truth and shared authoring, repair concrete UI capability/identity mismatches, then choose the personalization/ecosystem features deliberately. Counts: **55 A entries, 36 F entries, 16 B entries**.

## Source index: screen and sub-screen modules

This index names the actual modules behind the form-factor inventory. It is not a count of independent routes: forms, overlays, reusable lists and abstract helper files also live in screens directories. Native root route reachability was checked against `N/src/shell/Shell.tsx:94–107`; many workflow files render as sheets from those roots. Plezy's screen-directory files were indexed for page types, action/permission/platform branches and settings bindings; its player internals remain excluded. Full code paths were traced for findings, while this declaration/action index is the boundary for lesser-used screens.

### Portico native screens, sheets and helpers

| Module | Exported surface(s) / helper identity | UI vocabulary sample, where present | Evidence |
|---|---|---|---|
| `AccountFeed.tsx` | AccountFeed | `account.portico`, `web.account.notif.markAll`, `action.tryAgain`, `notifications.loading`, `web.account.notif.empty`, `web.account.notif.emptyBody` | `N/src/screens/AccountFeed.tsx:21` |
| `BrowseFilters.tsx` | BrowseBar, AlphabetRail | `lib.sortChip`, `filter.group.title`, `filter.group.activity`, `filter.group.file`, `sort.title`, `filter.title` | `N/src/screens/BrowseFilters.tsx:23` |
| `DeleteAccount.tsx` | DeleteAccountSheet | `account.delete.changed`, `account.delete.done`, `account.delete.mismatch`, `account.delete.title`, `account.delete.action`, `account.delete.checking` | `N/src/screens/DeleteAccount.tsx:17` |
| `Detail.tsx` | DetailScreen | `title.rated`, `title.stars`, `entry.rate`, `title.playFromStart`, `entry.refreshQueued`, `entry.refreshFailed` | `N/src/screens/Detail.tsx:41` |
| `Downloads.tsx` | DeviceDownloads, DownloadsScreen | `downloads.state`, `downloads.action`, `downloads.openFailed`, `action.play`, `action.pause`, `action.tryAgain` | `N/src/screens/Downloads.tsx:24` |
| `Entity.tsx` | EntityScreen | `title.resumeChapter`, `entry.resume`, `entry.play`, `title.shuffle`, `title.watchlist`, `title.finished` | `N/src/screens/Entity.tsx:43` |
| `Gallery.tsx` | Gallery | — | `N/src/screens/Gallery.tsx:22` |
| `Home.tsx` | HomeScreen, Hero, useSectionCards, sectionTitle, SectionView, RootHeaderActions, HeaderUtilities | `web.home.serverFallback`, `web.shell.home`, `error.changed`, `action.refresh`, `home.customize.action`, `action.details` | `N/src/screens/Home.tsx:39` |
| `HomeCustomise.tsx` | customiseLists, customiseSavePayload, addViewRow, removeViewRow, HomeCustomise | `confirm.resetHome.title`, `confirm.resetHome.body`, `action.reset`, `home.customize.action`, `home.customize.subtitle`, `action.resetToDefault` | `N/src/screens/HomeCustomise.tsx:19` |
| `HomeEmpty.tsx` | HomeEmpty | `web.empty.memberTitle`, `web.empty.memberBody`, `web.empty.ownerTitle`, `web.empty.ownerBody`, `home.empty.openConsole`, `home.empty.ownerComputer` | `N/src/screens/HomeEmpty.tsx:21` |
| `HomeRow.tsx` | HomeRowScreen | `action.tryAgain` | `N/src/screens/HomeRow.tsx:24` |
| `Invitation.tsx` | JoinServer, InvitationScreen | `web.directJoin.error.expired`, `web.directJoin.error.tooMany`, `web.join.title`, `web.join.unavailable`, `action.close`, `invitation.joined` | `N/src/screens/Invitation.tsx:31` |
| `InviteSheet.tsx` | InviteSheet | `invite.shareMessage`, `invitation.thisServer`, `invite.readyTitle`, `invite.title`, `invite.readyBody`, `invite.body` | `N/src/screens/InviteSheet.tsx:14` |
| `Library.tsx` | LibraryScreen | `lib.sortField`, `lib.count`, `lib.fallbackTitle`, `web.library.scanning`, `library.clearFilters`, `lib.optionsLoadFailed` | `N/src/screens/Library.tsx:48` |
| `LibraryTabs.tsx` | GenreCard, LibraryPlaylists | `title.songCount`, `library.playlists.loadFailed`, `action.tryAgain`, `web.saved.empty.playlists`, `web.saved.empty.playlistsBody`, `count.items` | `N/src/screens/LibraryTabs.tsx:23` |
| `LinkTV.tsx` | LinkTVSheet | `linkTV.signInFirst`, `linkTV.approved`, `linkTV.yourTV`, `linkTV.declined`, `settings.linkTV`, `action.done` | `N/src/screens/LinkTV.tsx:26` |
| `MetadataEditor.tsx` | repairFieldProblem, repairFieldMessage, MetadataEditorSheet, matchingSearchCommand, matchingStatusKey, isTransientMatchingStatus, chunkBulkTargets | `metadataEditor.titleRequired`, `metadataEditor.wholeNumber`, `metadataEditor.tooSmall`, `metadataEditor.tooLarge`, `metadataEditor.tooLong`, `metadataEditor.badDate` | `N/src/screens/MetadataEditor.tsx:38` |
| `Notifications.tsx` | NotificationsScreen, FeedbackSheet | `notifications.view.unread`, `notifications.view.all`, `notifications.view.archived`, `notifications.title`, `notifications.fromServer`, `notifications.markAllRead` | `N/src/screens/Notifications.tsx:15` |
| `Person.tsx` | PersonScreen | `title.lifespan`, `title.born`, `title.creditsAll`, `title.creditsCast`, `title.creditsCrew`, `error.title.load` | `N/src/screens/Person.tsx:76` |
| `Profiles.tsx` | useDirectAccount, LocalProfilesSection, PINRecoverySheet | `profile.pictureRejected`, `profile.limitsHelp`, `profile.removePin`, `profiles.title`, `profile.limitReached`, `profile.main` | `N/src/screens/Profiles.tsx:24` |
| `Recordings.tsx` | RecordingsScreen | `recordings.minutes`, `recordings.showCountUnwatched`, `recordings.showCount`, `recordings.kept`, `recordings.conflictsBody`, `recordings.newEpisodes` | `N/src/screens/Recordings.tsx:22` |
| `Saved.tsx` | SavedScreen | `lib.sortField`, `saved.title`, `saved.sort`, `saved.retry`, `action.refresh`, `saved.loadFailed` | `N/src/screens/Saved.tsx:128` |
| `Search.tsx` | SearchScreen, searchable | `search.placeholder`, `route.search`, `web.search.someOf`, `web.search.results`, `web.search.recent`, `web.search.clearRecent` | `N/src/screens/Search.tsx:58` |
| `Security.tsx` | ProfileLimitsSheet, DevicesSection, TwoFactorSection, ChangePasswordSheet, SignOutEverywhereSheet | `account.wrongPassword`, `limits.saved`, `limits.noLimit`, `limits.title`, `limits.subtitle`, `limits.save` | `N/src/screens/Security.tsx:51` |
| `ServerMembership.tsx` | ServerMembershipSection | `invitation.thisServer`, `membership.title`, `membership.ownerHelp`, `membership.leave`, `membership.transfer`, `membership.leaveConfirm` | `N/src/screens/ServerMembership.tsx:13` |
| `Settings.tsx` | SettingsScreen, SettingsPageScreen, AccountScreen, AboutScreen | `settings.switchProfile`, `settings.switchServer`, `notifications.title`, `settings.unreadCount`, `feature.watchTogether`, `settings.inGroup` | `N/src/screens/Settings.tsx:28` |
| `SettingsPages.tsx` | useDeviceNoun, WebHandoff, SignOutConfirm, ServersScreen | `link.openFailed`, `settings.openOnWeb`, `settings.signOutConfirm.title`, `servers.fallbackName`, `action.signOut`, `servers.switch` | `N/src/screens/SettingsPages.tsx:26` |
| `Show.tsx` | ShowScreen | `title.episodes`, `title.markSeasonUnwatched`, `title.markSeasonWatched`, `entry.editMetadata`, `title.showSettings`, `title.starring` | `N/src/screens/Show.tsx:49` |
| `Together.tsx` | TogetherScreen | `entry.group.together`, `web.together.eyebrowFallback`, `web.together.goAhead`, `web.together.tagline`, `web.together.taglineBody`, `web.together.startTitle` | `N/src/screens/Together.tsx:34` |
| `auth/AccountHelp.tsx` | AccountHelp | `auth.checkEmail`, `auth.codeSentTo`, `auth.code`, `auth.createAccount`, `auth.sendCodeAgain`, `auth.createYourAccount` | `N/src/screens/auth/AccountHelp.tsx:19` |
| `auth/Activation.tsx` | Activation | `activation.title`, `activation.lead`, `activation.codeRefreshes`, `launch.approvedConnecting`, `activation.declined`, `activation.getNewCode` | `N/src/screens/auth/Activation.tsx:19` |
| `auth/AuthFlow.tsx` | AuthFlow | — | `N/src/screens/auth/AuthFlow.tsx:25` |
| `auth/AuthFrame.tsx` | AuthFrame | `action.back`, `action.dismiss` | `N/src/screens/auth/AuthFrame.tsx:13` |
| `auth/DirectProfiles.tsx` | DirectProfiles | `auth.enterProfilePin`, `auth.whosWatching`, `pin.fourDigit`, `action.tryAgain`, `auth.loadingProfiles` | `N/src/screens/auth/DirectProfiles.tsx:15` |
| `auth/DirectSignIn.tsx` | DirectSignIn, DirectStepScreen | `auth.serverAddress`, `auth.signInToServer`, `action.continue`, `quickConnect.usePassword`, `auth.username`, `auth.password` | `N/src/screens/auth/DirectSignIn.tsx:21` |
| `auth/HostedChooser.tsx` | HostedChooser | `web.chooser.current`, `web.chooser.lastSeen`, `web.chooser.neverSeen`, `web.chooser.unreachableBody`, `web.chooser.unreachableBodyNever`, `web.chooser.unreachableTitle` | `N/src/screens/auth/HostedChooser.tsx:19` |
| `auth/HostedSignIn.tsx` | HostedSignIn, Legal | `auth.enterVerificationCode`, `auth.chooseUsername`, `auth.welcomeBack`, `auth.welcomeHome`, `auth.signInWithAccountTitle`, `auth.accountLead` | `N/src/screens/auth/HostedSignIn.tsx:20` |
| `auth/OfflineDownloads.tsx` | OfflineDownloadsScreen | `feature.downloads`, `auth.offlineDownloadsTitle`, `action.signIn`, `auth.offlineDownloadsLede` | `N/src/screens/auth/OfflineDownloads.tsx:20` |
| `auth/PorticoLanding.tsx` | PorticoLanding | `web.chooser.lastSeen`, `web.chooser.neverSeen`, `web.chooser.unreachableBody`, `web.chooser.unreachableBodyNever`, `auth.signOutOfAccount`, `device.approval.titleThisDevice` | `N/src/screens/auth/PorticoLanding.tsx:27` |
| `auth/ProfileAvatar.tsx` | ProfileAvatar, useProfilePictures | — | `N/src/screens/auth/ProfileAvatar.tsx:15` |
| `auth/QuickConnect.tsx` | QuickConnect | `quickConnect.yourServer`, `quickConnect.title`, `quickConnect.lede`, `quickConnect.failed`, `activation.codeRefreshes`, `launch.approvedConnecting` | `N/src/screens/auth/QuickConnect.tsx:19` |
| `auth/SavedServerUnavailable.tsx` | SavedServerUnavailable | `savedServer.title`, `savedServer.titleGeneric`, `savedServer.lead`, `savedServer.checking`, `savedServer.retryIn`, `savedServer.help` | `N/src/screens/auth/SavedServerUnavailable.tsx:18` |
| `auth/chooser-presence.ts` | chooserFailureView | — | `N/src/screens/auth/chooser-presence.ts:13` |
| `auth/sign-in-steps.ts` | firstDirectStep, directStepAfter, accountScreen, reachableDirectSteps | — | `N/src/screens/auth/sign-in-steps.ts:13` |
| `channels/ChannelList.tsx` | ChannelList, ChannelTimeline | `guide.noGuide`, `guide.next`, `channels.watchChannel`, `channels.moreFor`, `channels.watch`, `guide.tileFailed` | `N/src/screens/channels/ChannelList.tsx:18` |
| `channels/ChannelsScreen.tsx` | ChannelsScreen | `channels.title`, `action.tryAgain`, `channels.noSourcesTitle`, `channels.noSourcesOwner`, `channels.noSourcesMember`, `channels.source` | `N/src/screens/channels/ChannelsScreen.tsx:28` |
| `channels/GuideGrid.tsx` | GuideGrid | `guide.cellTime`, `guide.noInformation`, `guide.live`, `guide.new`, `guide.premiere`, `guide.onNowLeft` | `N/src/screens/channels/GuideGrid.tsx:68` |
| `channels/ProgramSheet.tsx` | ProgramSheet | `recordings.minutes`, `recordings.none`, `recordings.keepLatest`, `recordings.keepAll`, `guide.scheduledBody`, `guide.seriesSet` | `N/src/screens/channels/ProgramSheet.tsx:21` |
| `channels/Reminders.tsx` | ChannelReminderHost | `guide.reminder`, `channels.watch` | `N/src/screens/channels/Reminders.tsx:11` |
| `channels/dvr-groups.ts` | groupRecordings, seriesKey, isExceptionBadge, dvrPaddings, dvrLimits | — | `N/src/screens/channels/dvr-groups.ts:8` |
| `channels/format.ts` | useGuideFormat, useNow, GEOMETRY, initials, tintIndex, episodeLine | `guide.today`, `guide.tomorrow`, `guide.yesterday`, `guide.range`, `guide.minutesLeft`, `guide.timeLeft` | `N/src/screens/channels/format.ts:10` |
| `channels/now-store.ts` | createNowStore | — | `N/src/screens/channels/now-store.ts:17` |
| `channels/parts.tsx` | channelReason, channelDimmed, ChannelLogo, LibraryMark, ChannelName, channelA11y | `channels.custom.name`, `channels.channel` | `N/src/screens/channels/parts.tsx:17` |
| `server/Dashboard.tsx` | NowPlayingPanel, StatePermissionsNotice, AlertsPanel, HealthPanel, JobsPanel, StatisticsPanel | `action.tryAgain`, `web.nowPlaying.title`, `web.nowPlaying.empty`, `web.nowPlaying.untitled`, `web.nowPlaying.open`, `web.nowPlaying.stop` | `N/src/screens/server/Dashboard.tsx:27` |
| `server/LibrariesPanel.tsx` | LibrariesPanel | `web.libraries.sourceUnavailable`, `server.libraries.needAttention`, `server.libraries.warnings`, `web.libraries.loadFailed`, `action.tryAgain`, `web.console.nav.librariesLede` | `N/src/screens/server/LibrariesPanel.tsx:25` |
| `server/LivePanels.tsx` | LiveSourcesPanel, TunersPanel, RecordingRulesPanel, RecordingStoragePanel, LibraryChannelsPanel | `server.live.channelCount`, `server.live.programCount`, `server.live.tunerCount`, `server.live.guideUpdated`, `web.console.refreshFailed`, `web.liveTv.sourceSettings` | `N/src/screens/server/LivePanels.tsx:40` |
| `server/PeoplePanels.tsx` | AccountsPanel, InvitationsPanel, APIKeysPanel | `server.people.libraryGone`, `action.tryAgain`, `web.people.accountsTitle`, `web.people.accountsLede`, `server.people.porticoAccount`, `server.people.allLibraries` | `N/src/screens/server/PeoplePanels.tsx:26` |
| `server/PlayHistory.tsx` | PlayHistoryPanel | `server.plays.allLibraries`, `server.plays.allUsers`, `action.tryAgain`, `server.plays.filterLibrary`, `server.plays.removedLibrary`, `server.plays.filterUser` | `N/src/screens/server/PlayHistory.tsx:16` |
| `server/SchedulePanels.tsx` | WindowsPanel, ScheduledJobsPanel | `settings.server.windows`, `settings.server.windowsLede`, `web.maintenance.windowName`, `web.dvr.on`, `web.maintenance.days`, `web.maintenance.startsAt` | `N/src/screens/server/SchedulePanels.tsx:18` |
| `server/Status.tsx` | TranscodeStatusPanel, RemoteStatusPanel, CertificateStatusRows, AddressesRows, AccountConnectionPanel, IdentityRows, UpdatesPanel | `action.tryAgain`, `settings.server.transcodeStatus`, `settings.server.remote.checking`, `server.remote.checkAgain`, `web.general.status`, `server.cert.name` | `N/src/screens/server/Status.tsx:24` |
| `server/StoragePanels.tsx` | DiskUsagePanel, RetentionRows, BackupsPanel, DeletedTitlesPanel, RemoteSourcesPanel | `action.tryAgain`, `settings.server.diskUsage`, `web.maintenance.storageLede`, `web.maintenance.totalLabel`, `web.maintenance.truncatedHelp`, `web.maintenance.regenerableHelp` | `N/src/screens/server/StoragePanels.tsx:23` |
| `server/Troubleshooting.tsx` | DetailWindowRow, MessageLogPanel, RecordsPanel, CapabilitiesPanel, SupportExportPanel, FeedbackPanel | `settings.server.detailWindow`, `settings.server.detailWindow.until`, `settings.server.detailWindow.help`, `action.tryAgain`, `web.logs.messages`, `web.logs.messagesLede` | `N/src/screens/server/Troubleshooting.tsx:18` |
| `settings/LanguageListPicker.tsx` | LanguageListPicker | `preferences.language.empty`, `preferences.language.moveUp`, `preferences.language.moveDown`, `action.remove`, `preferences.language.add` | `N/src/screens/settings/LanguageListPicker.tsx:17` |
| `settings/ResetRecommendations.tsx` | ResetRecommendationsRow | `settings.resetRecommendations.done`, `settings.resetRecommendations`, `settings.resetRecommendations.help`, `settings.resetRecommendations.confirmTitle`, `settings.resetRecommendations.confirmBody`, `settings.resetRecommendations.confirm` | `N/src/screens/settings/ResetRecommendations.tsx:16` |
| `settings/ServerForms.tsx` | ServerFormsProvider, useServerForm, useFormsAbsent, useServerDeps, useReloadForms, useInlineForm, TypedRow | `action.done`, `settings.save.afterRestart`, `action.tryAgain`, `settings.save.failed`, `settings.save.unsaved`, `settings.save.saved` | `N/src/screens/settings/ServerForms.tsx:21` |
| `settings/ServerPage.tsx` | ServerPageScope, ServerPageBody | — | `N/src/screens/settings/ServerPage.tsx:68` |
| `settings/SettingsPage.tsx` | useSettingsStructure, SettingsPageBody | `preferences.unavailableTitle`, `preferences.unavailableNetwork`, `preferences.unavailableServer`, `action.tryAgain`, `settings.thisDevice`, `account.signedInAs` | `N/src/screens/settings/SettingsPage.tsx:33` |
| `settings/language-list.ts` | MAX_LANGUAGES, COMMON_LANGUAGES, languageName, availableLanguages, addLanguage, removeLanguage, moveLanguage | — | `N/src/screens/settings/language-list.ts:8` |
| `settings/language-picker-handoff.ts` | closeLanguageMenu | — | `N/src/screens/settings/language-picker-handoff.ts:20` |
| `title/EpisodePanel.tsx` | EpisodePanel | `title.resumeRemaining`, `entry.resume`, `entry.play`, `title.playFromStart`, `entry.markUnwatched`, `entry.markWatched` | `N/src/screens/title/EpisodePanel.tsx:31` |
| `title/Episodes.tsx` | useSlot, episodeTitle, episodeMeta, formatAirDate, EpisodeTintContext, EpisodeBrowser, EpisodeStillPlaceholder | `title.airedOn`, `title.episodeDetails`, `entry.play`, `title.aboutEpisode` | `N/src/screens/title/Episodes.tsx:36` |
| `title/PlaybackChoices.tsx` | PlaybackChoices | `mediaInfo.track`, `title.subtitlesAutomatic`, `title.subtitlesOff`, `title.playbackChoices`, `title.audio`, `title.subtitles` | `N/src/screens/title/PlaybackChoices.tsx:23` |
| `title/RowGuide.tsx` | RowGuide | — | `N/src/screens/title/RowGuide.tsx:10` |
| `title/SavedPages.tsx` | PlaylistScreen, SavedResourceScreen | `title.shuffle`, `web.profile.moveUp`, `web.profile.moveDown`, `web.saved.removeFromPlaylist`, `title.kind`, `count.items` | `N/src/screens/title/SavedPages.tsx:47` |
| `title/SeasonSwitcher.tsx` | SeasonSwitcher | `title.episodes`, `title.seasons`, `title.jumpTo`, `title.seasonOptions`, `title.go`, `title.jumpToSeason` | `N/src/screens/title/SeasonSwitcher.tsx:20` |
| `title/TitleDetails.tsx` | TitleDetails | `title.details`, `title.genres`, `title.files`, `mediaInfo.audio`, `mediaInfo.subtitles`, `mediaInfo.location` | `N/src/screens/title/TitleDetails.tsx:16` |
| `title/TitleHero.tsx` | TitleHero, StatusBarScrim | `title.moreActions`, `title.more`, `title.synopsis`, `action.back` | `N/src/screens/title/TitleHero.tsx:66` |
| `title/TitleSheets.tsx` | RateSheet, MediaInfoSheet | `title.rate`, `title.stars`, `title.clearRating`, `title.mediaInformation`, `action.done` | `N/src/screens/title/TitleSheets.tsx:14` |
| `title/TrackList.tsx` | TrackList | `title.shownOfTotal`, `action.showMore` | `N/src/screens/title/TrackList.tsx:15` |
| `title/TrackWindow.tsx` | TrackWindowList, residentFrom | — | `N/src/screens/title/TrackWindow.tsx:29` |
| `title/episode-credits.ts` | loadEpisodeCast | — | `N/src/screens/title/episode-credits.ts:18` |
| `title/facts.ts` | episodeCode, seasonLabel, orderSeasons, nextUpIn, resumeFraction, factsLine, namesIn | `title.episodeAbsolute`, `title.episodeCode`, `title.episodeOnly`, `title.specials`, `title.season` | `N/src/screens/title/facts.ts:11` |
| `title/language.ts` | useLanguageName | — | `N/src/screens/title/language.ts:8` |
| `title/layout.ts` | heroLayout, episodeArt, mosaicPaths | — | `N/src/screens/title/layout.ts:17` |
| `title/saved-hooks.ts` | usePlaylist, useSavedResource | — | `N/src/screens/title/saved-hooks.ts:9` |

### Plezy screen directory: all page modules and supporting screen families

All modules below share Flutter source, with per-form-factor branches rather than one separately named file per phone/tablet/TV. Backend gates matter: catalog discovery is not local library detail, Plex match is not a Jellyfin-identification API, and borrow-connection is not creation of a server user. `video_player/parts` helpers are excluded because they primarily concern player lifecycle/engine coordination; player-screen and chrome settings remain inventoried above.

| Module | Declared page/surface or supporting helper | Feature vocabulary sample | Evidence |
|---|---|---|---|
| `actor_media_screen.dart` | ActorMediaScreen | `discover.noContentAvailable`, `discover.titleCount` | `plezy/lib/screens/actor_media_screen.dart:25` |
| `auth/plex_pin_auth_flow.dart` | PlexPinAuthFlow | `auth.authenticationTimeout`, `addServer.pinExpired`, `addServer.couldNotReachServer`, `auth.signInWithPlex`, `auth.showQRCode` | `plezy/lib/screens/auth/plex_pin_auth_flow.dart:39` |
| `auth_screen.dart` | AuthScreen | `auth.localDataRecoveryRequired`, `serverSelection.noServersFoundForAccount`, `profiles.failedToLoadHomeUsers`, `serverSelection.failedToLoadServers`, `app.title` | `plezy/lib/screens/auth_screen.dart:37` |
| `base_media_list_detail_screen.dart` | for, BaseMediaListDetailScreen, StandardItemLoader | `errors.noClientAvailable` | `plezy/lib/screens/base_media_list_detail_screen.dart:20` |
| `catalog_item_detail_screen.dart` | CatalogItemDetailScreen | `explore.watchlistUpdateFailed`, `explore.checkingLibrary`, `explore.notInLibrary`, `explore.libraryCheckFailed`, `explore.inTheseLibraries` | `plezy/lib/screens/catalog_item_detail_screen.dart:64` |
| `catalog_search_screen.dart` | CatalogSearchScreen | `explore.searchHint`, `explore.searchPrompt`, `explore.searchFailed`, `explore.searchEmpty` | `plezy/lib/screens/catalog_search_screen.dart:22` |
| `collection_detail_screen.dart` | CollectionDetailScreen | `collections.empty`, `collections.collection`, `common.play`, `common.shuffle`, `common.delete` | `plezy/lib/screens/collection_detail_screen.dart:47` |
| `companion_remote/mobile_remote_screen.dart` | MobileRemoteScreen | `companionRemote.title`, `common.disconnect`, `companionRemote.remote.disconnectConfirm`, `companionRemote.remote.reconnecting`, `companionRemote.remote.attemptOf` | `plezy/lib/screens/companion_remote/mobile_remote_screen.dart:23` |
| `discover_screen.dart` | DiscoverScreen | `errors.unableToLoad`, `discover.title`, `discover.continueWatching`, `common.logout`, `messages.logoutConfirm` | `plezy/lib/screens/discover_screen.dart:70` |
| `downloads/downloads_options.dart` | DownloadsOptionsTab, DownloadsTabOptionsMixin, DownloadsLibrarySection, DownloadsLibrarySectionHeader | `libraries.groupings.movies`, `libraries.groupings.shows`, `libraries.groupings.seasons`, `libraries.groupings.episodes`, `libraries.groupings.albums` | `plezy/lib/screens/downloads/downloads_options.dart:35` |
| `downloads/downloads_screen.dart` | DownloadsScreen, DownloadsScreenState | `downloads.manage`, `downloads.tvShows`, `downloads.movies`, `downloads.music`, `downloads.title` | `plezy/lib/screens/downloads/downloads_screen.dart:41` |
| `downloads/sync_rules_screen.dart` | SyncRulesScreen | `downloads.activeSyncRules`, `downloads.noSyncRules`, `downloads.syncAllItems`, `downloads.syncUnwatchedItems`, `downloads.keepNUnwatched` | `plezy/lib/screens/downloads/sync_rules_screen.dart:21` |
| `explore_screen.dart` | ExploreScreen, ExploreScreenState | `explore.selectSource`, `explore.title`, `common.refresh`, `explore.searchHint`, `explore.emptyTitle` | `plezy/lib/screens/explore_screen.dart:50` |
| `focusable_detail_screen_mixin.dart` | must, FocusableDetailScreenMixin | `accessibility.rowPosition`, `accessibility.rowColumnPosition` | `plezy/lib/screens/focusable_detail_screen_mixin.dart:20` |
| `hub_detail_screen.dart` | HubDetailScreen | `libraries.sort`, `hubDetail.title`, `hubDetail.releaseYear`, `hubDetail.dateAdded`, `hubDetail.rating` | `plezy/lib/screens/hub_detail_screen.dart:38` |
| `libraries/alpha_jump_bar.dart` | AlphaJumpBar | `accessibility.alphabetNavigation` | `plezy/lib/screens/libraries/alpha_jump_bar.dart:19` |
| `libraries/alpha_jump_helper.dart` | AlphaJumpHelper | — | `plezy/lib/screens/libraries/alpha_jump_helper.dart:15` |
| `libraries/alpha_scroll_handle.dart` | AlphaScrollHandle | `accessibility.alphabetNavigation`, `accessibility.alphabetScrollHint` | `plezy/lib/screens/libraries/alpha_scroll_handle.dart:14` |
| `libraries/content_state_builder.dart` | SliverErrorState, SliverEmptyState, ContinuationStatusSliver, ContentStateBuilder | `errors.unableToLoad`, `common.retry` | `plezy/lib/screens/libraries/content_state_builder.dart:9` |
| `libraries/filters/filter_operator_row.dart` | FilterClearRow, FilterIncludeExcludeRow, FilterBooleanRow | `libraries.all`, `libraries.advancedFilters.exclude`, `libraries.advancedFilters.include`, `libraries.advancedFilters.any`, `libraries.advancedFilters.yes` | `plezy/lib/screens/libraries/filters/filter_operator_row.dart:30` |
| `libraries/filters/filter_range_page.dart` | FilterNumberPage, FilterDatePage | `libraries.advancedFilters.from`, `libraries.advancedFilters.to` | `plezy/lib/screens/libraries/filters/filter_range_page.dart:20` |
| `libraries/filters/filter_summary.dart` | FilterValueNames | `libraries.advancedFilters.no`, `libraries.advancedFilters.yes`, `libraries.advancedFilters.valueCountExcluded`, `libraries.advancedFilters.valueCount`, `libraries.advancedFilters.not` | `plezy/lib/screens/libraries/filters/filter_summary.dart:11` |
| `libraries/filters/filter_text_page.dart` | FilterTextPage | `libraries.advancedFilters.matchContains`, `libraries.advancedFilters.matchNotContains`, `libraries.advancedFilters.matchIs`, `libraries.advancedFilters.matchIsNot`, `libraries.advancedFilters.matchBeginsWith` | `plezy/lib/screens/libraries/filters/filter_text_page.dart:17` |
| `libraries/filters/filter_value_page.dart` | FilterValuePage | `libraries.advancedFilters.searchValues`, `libraries.advancedFilters.noValues` | `plezy/lib/screens/libraries/filters/filter_value_page.dart:22` |
| `libraries/filters_bottom_sheet.dart` | FiltersBottomSheet | `errors.unableToLoad`, `libraries.filters`, `libraries.clearAll` | `plezy/lib/screens/libraries/filters_bottom_sheet.dart:37` |
| `libraries/folder_tree_item.dart` | FolderTreeItem | `common.play`, `common.shuffle` | `plezy/lib/screens/libraries/folder_tree_item.dart:30` |
| `libraries/folder_tree_view.dart` | FolderTreeView, FolderTreeViewState | `libraries.folders`, `messages.musicNotSupported`, `common.retry`, `libraries.noFoldersFound` | `plezy/lib/screens/libraries/folder_tree_view.dart:22` |
| `libraries/libraries_screen.dart` | LibrariesScreen, contract | `libraries.tabs.recommended`, `libraries.tabs.browse`, `libraries.tabs.collections`, `libraries.tabs.playlists`, `name` | `plezy/lib/screens/libraries/libraries_screen.dart:45` |
| `libraries/library_alpha_bar_strategy.dart` | LibraryAlphaBarStrategy, PlexAlphaBarStrategy, MediaBrowserAlphaBarStrategy | — | `plezy/lib/screens/libraries/library_alpha_bar_strategy.dart:17` |
| `libraries/library_alpha_scroll_metrics.dart` | LibraryAlphaScrollMetrics | — | `plezy/lib/screens/libraries/library_alpha_scroll_metrics.dart:6` |
| `libraries/library_browse_grouping.dart` | Screen helper/functions | — | `plezy/lib/screens/libraries/library_browse_grouping.dart:1` |
| `libraries/library_quick_picker_sheet.dart` | LibraryQuickPickerSheet | `libraries.selectLibrary` | `plezy/lib/screens/libraries/library_quick_picker_sheet.dart:11` |
| `libraries/library_server_label.dart` | LibraryServerLabel | — | `plezy/lib/screens/libraries/library_server_label.dart:57` |
| `libraries/sort_bottom_sheet.dart` | SortBottomSheet | `libraries.sortBy`, `common.clear` | `plezy/lib/screens/libraries/sort_bottom_sheet.dart:16` |
| `libraries/state_messages.dart` | StateMessageWidget, EmptyStateWidget, ErrorStateWidget | `common.retry` | `plezy/lib/screens/libraries/state_messages.dart:10` |
| `libraries/tabs/base_library_tab.dart` | for, BaseLibraryTab, that, BaseLibraryTabState | — | `plezy/lib/screens/libraries/tabs/base_library_tab.dart:18` |
| `libraries/tabs/library_browse_tab.dart` | LibraryBrowseTab, items, LoadedFiltersAndSorts | `libraries.thisLibraryIsEmpty`, `libraries.content`, `libraries.groupings.movies`, `libraries.groupings.shows`, `libraries.groupings.seasons` | `plezy/lib/screens/libraries/tabs/library_browse_tab.dart:73` |
| `libraries/tabs/library_collections_tab.dart` | LibraryCollectionsTab | `libraries.noCollections`, `collections.title` | `plezy/lib/screens/libraries/tabs/library_collections_tab.dart:13` |
| `libraries/tabs/library_playlists_tab.dart` | LibraryPlaylistsTab | `playlists.noPlaylists`, `playlists.title` | `plezy/lib/screens/libraries/tabs/library_playlists_tab.dart:14` |
| `libraries/tabs/library_recommended_tab.dart` | LibraryRecommendedTab | `libraries.noRecommendations`, `libraries.tabs.recommended` | `plezy/lib/screens/libraries/tabs/library_recommended_tab.dart:33` |
| `libraries/tabs/paginated_card_grid_tab.dart` | PaginatedCardGridTabState | — | `plezy/lib/screens/libraries/tabs/paginated_card_grid_tab.dart:29` |
| `livetv/guide_search_sheet.dart` | GuideSearchSheet | `liveTv.searchGuide`, `liveTv.searchHint`, `liveTv.searchNoResults`, `liveTv.channelsSection`, `liveTv.programsSection` | `plezy/lib/screens/livetv/guide_search_sheet.dart:30` |
| `livetv/live_tv_actions_mixin.dart` | LiveTvActionsMixin | — | `plezy/lib/screens/livetv/live_tv_actions_mixin.dart:17` |
| `livetv/live_tv_refresh_mixin.dart` | LiveTvRefreshMixin | — | `plezy/lib/screens/livetv/live_tv_refresh_mixin.dart:27` |
| `livetv/live_tv_screen.dart` | LiveTvScreen | `liveTv.guideReloadRequested`, `liveTv.guideReloadFailed`, `liveTv.dvrAdminRequired`, `liveTv.rulesProcessRequested`, `liveTv.rulesProcessFailed` | `plezy/lib/screens/livetv/live_tv_screen.dart:43` |
| `livetv/live_tv_server_iteration.dart` | Screen helper/functions | — | `plezy/lib/screens/livetv/live_tv_server_iteration.dart:1` |
| `livetv/live_tv_show_schedule_screen.dart` | LiveTvShowScheduleScreen | `liveTv.recordShow`, `liveTv.noPrograms`, `discover.minutesLeft`, `liveTv.startingInMinutes`, `liveTv.dayAtTime` | `plezy/lib/screens/livetv/live_tv_show_schedule_screen.dart:26` |
| `livetv/livetv_recording_actions.dart` | Screen helper/functions | `liveTv.recordNotAvailable`, `liveTv.dvrAdminRequired`, `liveTv.recordingFailed`, `liveTv.recordingScheduled`, `liveTv.alreadyScheduled` | `plezy/lib/screens/livetv/livetv_recording_actions.dart:1` |
| `livetv/livetv_styles.dart` | StatusPill | — | `plezy/lib/screens/livetv/livetv_styles.dart:14` |
| `livetv/program_details_sheet.dart` | Screen helper/functions | `common.play`, `liveTv.watchChannel`, `liveTv.manageRecording`, `liveTv.record`, `liveTv.live` | `plezy/lib/screens/livetv/program_details_sheet.dart:1` |
| `livetv/record_options_sheet.dart` | RecordOptionsSheet | `liveTv.saveTo`, `liveTv.editRule`, `liveTv.recordOptions`, `common.cancel`, `common.save` | `plezy/lib/screens/livetv/record_options_sheet.dart:36` |
| `livetv/reorder_favorites_sheet.dart` | ReorderFavoritesSheet | `liveTv.reorderFavorites` | `plezy/lib/screens/livetv/reorder_favorites_sheet.dart:18` |
| `livetv/tabs/guide_tab.dart` | GuideTab, GuideTabState | `liveTv.today`, `liveTv.tomorrow`, `liveTv.midnight`, `liveTv.overnight`, `liveTv.morning` | `plezy/lib/screens/livetv/tabs/guide_tab.dart:40` |
| `livetv/tabs/recordings_tab.dart` | RecordingsTab, RecordingsTabState | `liveTv.recordingFailed`, `liveTv.editRuleAction`, `common.delete`, `liveTv.dvrAdminRequired`, `liveTv.noScheduledRecordings` | `plezy/lib/screens/livetv/tabs/recordings_tab.dart:26` |
| `livetv/tabs/whats_on_tab.dart` | WhatsOnTab, WhatsOnTabState | `liveTv.noPrograms` | `plezy/lib/screens/livetv/tabs/whats_on_tab.dart:26` |
| `main_screen.dart` | ProfileSelectionResumeGate, ContentRefreshResumeGate, MainScreen, via | `id`, `update.available`, `common.later`, `common.pressBackAgainToExit`, `libraries.noLibrariesFound` | `plezy/lib/screens/main_screen.dart:150` |
| `media_detail/action_buttons.dart` | Screen helper/functions | `common.resume`, `common.play`, `mediaMenu.playVersion`, `tooltips.playTrailer`, `tooltips.shufflePlay` | `plezy/lib/screens/media_detail/action_buttons.dart:1` |
| `media_detail/playback_tracks_status.dart` | Screen helper/functions | `common.off`, `videoControls.tracksButton` | `plezy/lib/screens/media_detail/playback_tracks_status.dart:1` |
| `media_detail_screen.dart` | MediaDetailScreen | `mediaMenu.rate`, `downloads.manageSyncRule`, `downloads.editSyncRule`, `downloads.removeSyncRule`, `downloads.deleteDownload` | `plezy/lib/screens/media_detail_screen.dart:285` |
| `metadata_edit_screen.dart` | MetadataEditScreen, ArtworkPickerDialog | `metadataEdit.metadataUpdateFailed`, `metadataEdit.metadataUpdated`, `metadataEdit.screenTitle`, `common.save`, `metadataEdit.notSet` | `plezy/lib/screens/metadata_edit_screen.dart:32` |
| `music/album_detail_screen.dart` | AlbumDetailScreen | `messages.noItemsAvailable`, `downloads.deleteDownload`, `downloads.deleteConfirm`, `downloads.downloadDeleted`, `downloads.tracksQueued` | `plezy/lib/screens/music/album_detail_screen.dart:44` |
| `music/artist_detail_screen.dart` | ArtistDetailScreen | `messages.noItemsAvailable`, `libraries.groupings.albums`, `music.discography.singlesAndEps`, `music.discography.live`, `music.discography.compilations` | `plezy/lib/screens/music/artist_detail_screen.dart:34` |
| `music/now_playing_screen.dart` | NowPlayingScreen, name | `messages.playbackFailed`, `messages.errorLoading`, `music.sleepTimer`, `common.off`, `music.sleepTimerMinutes` | `plezy/lib/screens/music/now_playing_screen.dart:59` |
| `music/queue_sheet.dart` | QueueSheet, QueueList | `music.queue`, `music.trackCount`, `common.shuffle`, `music.clearQueue`, `messages.noItemsAvailable` | `plezy/lib/screens/music/queue_sheet.dart:28` |
| `playlist/playlist_detail_screen.dart` | PlaylistDetailScreen, with | `playlists.emptyPlaylist`, `common.play`, `common.shuffle`, `playlists.delete`, `playlists.deleteConfirm` | `plezy/lib/screens/playlist/playlist_detail_screen.dart:39` |
| `playlist/playlist_item_card.dart` | PlaylistItemCard, PlaylistItemCardState | `playlists.removeItem`, `discover.tvShow`, `discover.movie` | `plezy/lib/screens/playlist/playlist_item_card.dart:25` |
| `plex_match_screen.dart` | PlexMatchScreen | `errors.searchFailed`, `matchScreen.matchApplied`, `matchScreen.matchFailed`, `matchScreen.match`, `matchScreen.fixMatch` | `plezy/lib/screens/plex_match_screen.dart:29` |
| `profile/add_local_profile_screen.dart` | AddLocalProfileScreen | `profiles.pinsDontMatch`, `profiles.newProfile`, `profiles.profileNameLabel`, `profiles.profileNameHint`, `profiles.pinProtectionOptional` | `plezy/lib/screens/profile/add_local_profile_screen.dart:25` |
| `profile/borrow_connection_screen.dart` | BorrowConnectionScreen | `profiles.borrowLoadFailed`, `profiles.borrowEmpty`, `profiles.borrowEmptySubtitle`, `profiles.borrowAddTo`, `profiles.borrowExplain` | `plezy/lib/screens/profile/borrow_connection_screen.dart:43` |
| `profile/pin_entry_dialog.dart` | PinEntryDialog | `common.cancel`, `profiles.setPinTitle`, `profiles.confirmPinTitle` | `plezy/lib/screens/profile/pin_entry_dialog.dart:17` |
| `profile/pin_status_row.dart` | PinStatusRow | `profiles.pinSet`, `profiles.changePin`, `profiles.removePin` | `plezy/lib/screens/profile/pin_status_row.dart:10` |
| `profile/profile_detail_screen.dart` | ProfileDetailScreen | `profiles.profileRenamed`, `profiles.pinsDontMatch`, `profiles.removeConnectionTitle`, `profiles.removeConnectionMessage`, `profiles.removeConnection` | `plezy/lib/screens/profile/profile_detail_screen.dart:43` |
| `profile/profile_name_field.dart` | ProfileNameField | — | `plezy/lib/screens/profile/profile_name_field.dart:9` |
| `profile/profile_switch_screen.dart` | ProfileSwitchScreen | `screens.switchProfile`, `messages.noProfilesAvailable`, `messages.contactAdminForProfiles`, `profiles.addPlezyProfile`, `profiles.incorrectPinTryAgain` | `plezy/lib/screens/profile/profile_switch_screen.dart:38` |
| `profile/profile_teardown.dart` | SessionTeardownScope | `errors.failedToDeleteProfile`, `profiles.signOutPlexTitle`, `profiles.signOutPlexMessage`, `profiles.signOut`, `profiles.signOutPlexDeleteDownloads` | `plezy/lib/screens/profile/profile_teardown.dart:37` |
| `search_screen.dart` | SearchScreen | `errors.searchUnavailable`, `errors.searchFailed`, `messages.searchPartialResults`, `libraries.groupings.all`, `search.people` | `plezy/lib/screens/search_screen.dart:39` |
| `settings/about_screen.dart` | AboutScreen | `app.title`, `about.title`, `about.versionLabel`, `about.appDescription`, `about.openSourceLicenses` | `plezy/lib/screens/settings/about_screen.dart:12` |
| `settings/account_preferences_detail_screen.dart` | AccountPreferencesDetailScreen, AccountPreferencesBody | `accountPreferences.unavailable`, `accountPreferences.loadFailed`, `accountPreferences.storedOnAccount`, `accountPreferences.groups.audioAndSubtitles`, `accountPreferences.groups.libraryDisplay` | `plezy/lib/screens/settings/account_preferences_detail_screen.dart:51` |
| `settings/account_preferences_screen.dart` | AccountPreferencesScreen | `accountPreferences.sectionTitle`, `accountPreferences.noAccounts`, `accountPreferences.noAccountsHint`, `accountPreferences.pickAccount` | `plezy/lib/screens/settings/account_preferences_screen.dart:21` |
| `settings/add_connection_screen.dart` | AddConnectionScreen | `addServer.signInWithPlexCard`, `addServer.signInWithPlexCardSubtitleScoped`, `addServer.signInWithPlexCardSubtitle`, `addServer.connectToMediaBrowserCard`, `addServer.connectToMediaBrowserCardSubtitleScoped` | `plezy/lib/screens/settings/add_connection_screen.dart:27` |
| `settings/add_jellyfin_screen.dart` | AddJellyfinScreen | `addServer.enterMediaBrowserUrlError`, `addServer.couldNotReachServer`, `addServer.signInFailed`, `auth.quickConnectExpired`, `addServer.quickConnectFailed` | `plezy/lib/screens/settings/add_jellyfin_screen.dart:86` |
| `settings/add_plex_account_screen.dart` | AddPlexAccountScreen | `addServer.failedToRegisterAccount`, `addServer.addPlexTitle`, `auth.signInWithPlex`, `auth.showQRCode` | `plezy/lib/screens/settings/add_plex_account_screen.dart:37` |
| `settings/appearance_settings_screen.dart` | AppearanceSettingsScreen | `settings.appearance`, `settings.display`, `settings.libraryAndCards`, `settings.showEpisodeNumberOnCards`, `settings.showEpisodeNumberOnCardsDescription` | `plezy/lib/screens/settings/appearance_settings_screen.dart:22` |
| `settings/async_form_state_mixin.dart` | AsyncFormStateMixin | — | `plezy/lib/screens/settings/async_form_state_mixin.dart:23` |
| `settings/connection_persistence.dart` | Screen helper/functions | — | `plezy/lib/screens/settings/connection_persistence.dart:1` |
| `settings/edit_jellyfin_connection_screen.dart` | EditJellyfinConnectionScreen | `addServer.couldNotReachServer`, `connections.editMediaBrowserTitle`, `connections.editMediaBrowserIntro`, `addServer.serverUrls`, `addServer.required` | `plezy/lib/screens/settings/edit_jellyfin_connection_screen.dart:19` |
| `settings/external_player_screen.dart` | ExternalPlayerScreen | `externalPlayer.title`, `externalPlayer.useExternalPlayer`, `externalPlayer.useExternalPlayerDescription`, `externalPlayer.selectPlayer`, `externalPlayer.customPlayers` | `plezy/lib/screens/settings/external_player_screen.dart:22` |
| `settings/general_settings_screen.dart` | GeneralSettingsScreen | `settings.general`, `settings.languageAndRegion`, `settings.startup`, `settings.requireProfileSelectionOnOpen`, `settings.requireProfileSelectionOnOpenDescription` | `plezy/lib/screens/settings/general_settings_screen.dart:22` |
| `settings/hotkey_recorder_widget.dart` | HotKeyRecorderWidget | `hotkeys.recordingShortcut`, `hotkeys.pressToRecord`, `hotkeys.setShortcutFor`, `hotkeys.currentShortcut`, `hotkeys.clearShortcut` | `plezy/lib/screens/settings/hotkey_recorder_widget.dart:13` |
| `settings/keyboard_shortcuts_screen.dart` | KeyboardShortcutsScreen | `settings.keyboardShortcuts`, `common.reset`, `settings.resetToDefault`, `settings.resetShortcutsConfirm`, `settings.shortcutsReset` | `plezy/lib/screens/settings/keyboard_shortcuts_screen.dart:18` |
| `settings/licenses_screen.dart` | MergedLicenseEntry, LicensesScreen | `screens.licenses`, `licenses.licensesCount`, `licenses.relatedPackages`, `licenses.licenseNumber`, `licenses.license` | `plezy/lib/screens/settings/licenses_screen.dart:9` |
| `settings/logs_screen.dart` | LogsScreen | `app.title`, `messages.logsCleared`, `messages.logsCopied`, `messages.logsUploaded`, `messages.logId` | `plezy/lib/screens/settings/logs_screen.dart:99` |
| `settings/mpv_config_line_editor.dart` | MpvConfigLineEditor | `mpvConfig.removeLine`, `mpvConfig.lineHint`, `mpvConfig.addLine` | `plezy/lib/screens/settings/mpv_config_line_editor.dart:28` |
| `settings/mpv_config_screen.dart` | MpvConfigScreen | `settings.saveFailed`, `mpvConfig.saveAsPreset`, `mpvConfig.presetName`, `mpvConfig.presetNameHint`, `mpvConfig.presetSaved` | `plezy/lib/screens/settings/mpv_config_screen.dart:29` |
| `settings/playback_settings_screen.dart` | PlaybackSettingsScreen | `settings.videoPlayback`, `settings.player`, `settings.videoAndDisplay`, `settings.audio`, `settings.quality` | `plezy/lib/screens/settings/playback_settings_screen.dart:26` |
| `settings/quick_connect_flow_mixin.dart` | QuickConnectFlowMixin | — | `plezy/lib/screens/settings/quick_connect_flow_mixin.dart:16` |
| `settings/seerr_connect_screen.dart` | SeerrConnectScreen | `addServer.required`, `seerr.noPlexTokenForReauth`, `auth.quickConnectExpired`, `addServer.couldNotReachServer`, `seerr.connectTitle` | `plezy/lib/screens/settings/seerr_connect_screen.dart:36` |
| `settings/seerr_settings_screen.dart` | SeerrSettingsScreen | `seerr.disconnectConfirm`, `seerr.disconnectConfirmBody`, `common.disconnect`, `seerr.title`, `seerr.email` | `plezy/lib/screens/settings/seerr_settings_screen.dart:18` |
| `settings/services_settings_screen.dart` | ServicesSettingsScreen | `services.title`, `services.hubSubtitle`, `services.integrations`, `settings.discordRichPresence`, `settings.discordRichPresenceDescription` | `plezy/lib/screens/settings/services_settings_screen.dart:23` |
| `settings/settings_screen.dart` | SettingsScreen | `settings.title`, `settings.about`, `settings.aboutDescription`, `settings.general`, `settings.generalDescription` | `plezy/lib/screens/settings/settings_screen.dart:67` |
| `settings/settings_utils.dart` | DialogOption | `settings.systemTheme`, `settings.lightTheme`, `settings.darkTheme`, `settings.oledTheme`, `settings.saveFailed` | `plezy/lib/screens/settings/settings_utils.dart:30` |
| `settings/subtitle_styling_screen.dart` | SubtitleStylingScreen | `common.no`, `common.yes`, `subtitlingStyling.overrideScale`, `subtitlingStyling.overrideForce`, `subtitlingStyling.overrideStrip` | `plezy/lib/screens/settings/subtitle_styling_screen.dart:13` |
| `settings/tracker_account_settings_body.dart` | TrackerSettingsToggle, TrackerAccountSettingsBody | `settings.behavior`, `services.libraryFilter.title`, `common.disconnect` | `plezy/lib/screens/settings/tracker_account_settings_body.dart:17` |
| `settings/tracker_connect_launcher.dart` | Screen helper/functions | `services.connectFailed` | `plezy/lib/screens/settings/tracker_connect_launcher.dart:1` |
| `settings/tracker_library_filter_screen.dart` | TrackerLibraryFilterScreen | `services.libraryFilter.subtitleAllSyncing`, `services.libraryFilter.subtitleNoneSyncing`, `services.libraryFilter.subtitleBlocked`, `services.libraryFilter.subtitleAllowed`, `services.libraryFilter.title` | `plezy/lib/screens/settings/tracker_library_filter_screen.dart:20` |
| `settings/tracker_service_info.dart` | TrackerServiceInfo | `trakt.title` | `plezy/lib/screens/settings/tracker_service_info.dart:23` |
| `settings/tracker_settings_screen.dart` | TrackerConfig, TrackerSettingsScreen | `services.names.mal`, `services.names.anilist`, `services.names.simkl`, `services.names.mdblist`, `services.simklReconnect.title` | `plezy/lib/screens/settings/tracker_settings_screen.dart:96` |
| `settings/trakt_settings_screen.dart` | TraktSettingsScreen | `trakt.title`, `trakt.disconnectConfirm`, `trakt.disconnectConfirmBody`, `common.disconnect`, `trakt.connectedAs` | `plezy/lib/screens/settings/trakt_settings_screen.dart:30` |
| `video_player_screen.dart` | VideoPlayerScreen, VideoPlayerScreenState, at | `watchTogether.leaveSessionQuestion`, `watchTogether.leaveSessionConfirm`, `watchTogether.leave`, `videoControls.playbackResumed`, `videoControls.playbackPaused` | `plezy/lib/screens/video_player_screen.dart:468` |

### Additional comparison surface families

| App | Surface families inspected/indexed | Boundary |
|---|---|---|
| Swiftfin | ConnectToServer/ServerConnection/EditServerConnection/SelectUser/QuickConnect; ItemView/ItemContentGroup/Filter; Customize/LocalUser/Security/VideoPlayer/Quality/DeviceProfile/Experimental/Debug; AdminDashboard users/devices/sessions/tasks/triggers/activity/logs/API keys/backups; About/Gesture editor | Shared SwiftUI components and platform gates read; every nested admin mutation was not end-to-end traced. |
| jellyfin-androidtv | Startup server/user picker; home/browsing/itemdetail; settings main/authentication/theme/backdrop/clock/watched/subtitles/home/library/live/screensaver/playback/media segments/next-up/photo/telemetry/developer/about/licences; dream/provider/search integration | All settings files indexed via bindings; older legacy browsing/player adapters sampled, not uniformly audited. |
| Wholphin | Home row settings/destinations/favorites; Seerr Discover/Requests/details/request seasons; movie/show/episode/person/collection/folder/music/photo-album/live/recordings/playlists; app/user/subtitle/experimental/screensaver preferences | TV-only source ideas, not phone/tablet parity claim. |
| Streamyfin | Expo auth/tab settings and TV variants; appearance/hide-libraries/audio-subtitle/network/playback/segments/music/logs/plugins; Seerr/Marlin/Streamystats/KefinTweaks; gesture/Chromecast/storage/native chrome settings; complete Settings model | Settings type indexed completely; stored fields without UI references explicitly labelled model-only. |

## Resolved suspicions and caution against false positives

- Android PiP is **not** classified as a broken platform gate just because `hasPictureInPicture` says all handhelds. Native `PorticoPictureInPicture.supported` checks Android API >=26, the OS PiP feature and absence of Leanback, and announces unavailable; JavaScript hides the affordance on that event (`N/android/app/src/main/java/tv/getportico/portico/native/PorticoPlayerView.kt:202–208`, `N/src/player/VideoPlayer.tsx:338–340`). Event timing still requires runtime validation, but the source has a capability path.
- TV downloads/cast sender omissions are not automatically bugs: contracts explicitly allow no protected offline storage on TV, and TV receiver versus handheld sender is a deliberate distinction. Fire/Vega system Home omission is documented; Vega privacy avoids third-party personalization identifiers (`V/README.md`, Not provided).
- Search/playlist/history/DVR screens are **not** currently unwired merely because old NOTES say so. Registration and actions were traced in the current tree.
- A hidden non-current root using `display:none` or an unreachable focus guide is intended. A focus-trap allegation requires an actual visible target with no eligible directional path; this audit did not reproduce one.
- A static field parser or generated settings union is not itself a screen. Tables distinguish model-only fields, platform-gated controls, owner-only panels and player utilities.
- Shared native and canonical server `settings-structure.ts` copies were compared with `cmp` and matched in this working tree. No general shared-package drift claim is made from that one-file check.
