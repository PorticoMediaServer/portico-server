# Clients

Two clients ship: the **web app** (in portico-server) and the **Apple app** for iPhone, iPad and Apple TV (in portico-react-native). Both are thin. The server composes rows, orders, pages, labels and recommendations, and clients render them. Behavior that isn't rendering lives in the shared client core, so a future port gets it for free. The tvOS app is the reference experience for later platforms.

## Shared packages

Owned by portico-server (`packages/`); portico-react-native carries an identical copy of the parts it uses ([Overview](01%20Overview.md#repositories)).

- **`client-core`** (`@portico/client-core`) is the headless core, in TypeScript with no UI. It covers:
  - session and device registration
  - server connections and sign-in flows (direct, Portico Account, TV code approval)
  - response validation for every document the apps read (Home, content projections, detail, show workspace, search, saved, guide, recommendations)
  - Playback Protocol v1 sessions and queues, downloads, personal state, Not interested with Undo, bulk jobs, and preferences

  It also holds the **authored structure** every client renders (`src/presentation`): the Settings pages and rows (`settings-structure.ts`), a library's tabs (`library-tabs.ts`), the caption under a card (`card-caption.ts`), search chips and section order (`search-model.ts`), title-page facts and per-show settings rows (`title-page.ts`), and the player's Playback info rows (`playback-info.ts`). The owner console's rules and words are in `src/server-admin`. A client never renders a registry or a capability list in its own order.

  Every server response is validated with bounded parsers before use. Unknown content kinds are dropped so a newer server doesn't break an older client. Tests: `npm run test:core`.
- **`design`** holds the tokens (colour, type, spacing, radii, icons) and the component specification (`COMPONENTS.md`, numbers in `src/components.ts`). The web app compiles tokens into CSS, and the Apple app reads them directly.
- **`i18n`** holds the string catalogue and message formatting. US English is the source language; launch is English-only, with more languages planned. Words follow the device's region, like dates, times and numbers do, unless a profile chooses a language in Language & region: the United States (and English with no region) reads the source, Canada reads en-CA, and every other English-speaking region reads en-GB. The two regional catalogues are **derived** from en-US by spelling rules (`i18n/src/catalog/regional.ts`: "favourite", "colour", "cancelled" for both; "-ise" and "programme" for Britain only), so a new string cannot be missed; only spelling is regional, never vocabulary. `lint-strings` rejects raw user-facing strings outside the catalogue.
- **`contracts`** is generated TypeScript for the server API (from apikit). Client-core is moving onto it.

## Web app

`apps/portico-server/web`: React, TypeScript, Vite and TanStack Router ([`web/README.md`](../../apps/portico-server/web/README.md)).

- **Served by every server** from `PORTICO_WEB_DIR` on the server's own port, and built into the Docker image. **Hosted serves a Hosted-only build** at web.getportico.tv (`VITE_HOSTED_WEB`), which signs in through Portico Account and connects to the user's servers.
- `src/ui` is the only place raw colours, fonts and z-index values may appear (`scripts/lint-design.mjs`). `src/bridge` holds the browser transport, storage, HLS (a patched hls.js from `third_party/hls.js`) and WebCodecs audio adapters.
- Server administration is the **Server** heading of Settings (ten pages, one Save per page), drawn by `src/screens/settings` from client-core's structure with panels in `src/screens/server`. The same pages are native on iPhone and iPad; Apple TV lists the Dashboard for an owner session.
- Title cards are links (`href` from the same route rule as navigation), so middle-click, a new tab and Copy link work; one page header puts every page's title at the same height; card size (Small, Medium, Large) scales grids and shelves through `--card-scale`.
- Checks: `npm run check:web` (tokens, design lint, preference-consumer check, string lint, tsc, tests).

## Apple app

`apps/portico-react-native/apps/native`: React Native for iOS and tvOS (react-native-tvos), with native modules for what JavaScript can't do well.

| Folder | Role |
|---|---|
| `src/ui` | The design system; no other folder uses raw colours, `StyleSheet.create` or raw `Pressable`/`Text` |
| `src/app` | Session, content and detail hooks, routes, device preferences |
| `src/shell` | Television rail, handheld bottom tabs, navigation stack, Menu-key policy |
| `src/screens` | Home, Library (tabs authored in client-core: Discover first, then the kind's own), detail pages, Search, Saved, Live TV, Channels, Settings (one screen, with the Server pages for owners) |
| `src/player` | Player engine binding, full-screen chrome, handheld mini bar, TV now-playing pill |
| `src/bridge` | JavaScript bindings to the native modules |
| `Native/` | Objective-C and Swift modules: `PorticoPlayer` and `PorticoPlaybackController` (AVPlayer), `PorticoAudioRender` (gapless decode), `PorticoDownloads`, `PorticoNetwork` (TLS and local discovery), `PorticoAccountAuth` (Sign in with Apple), keychain storage, `PorticoCast`, `PorticoSystemMedia` (Now Playing, remote commands), `PorticoTopShelf`, `PorticoSiriRemote`, `PorticoSearchKeyboard` |
| `ios/`, `tvos/` | Xcode projects `Portico` (bundle ids `tv.getportico.portico.ios` and `.tv`), with the TV Top Shelf extension |

- The app group is `group.tv.getportico.portico`; the Top Shelf reads its token from there. Credentials live in the keychain under `tv.getportico.portico.*` services.
- Build: `npm install --legacy-peer-deps`, `pod install` in `ios/` and `tvos/` (with a UTF-8 locale), then Xcode or `xcodebuild`. Metro runs on port 19424 for debug builds. Checks: `npx tsc --noEmit`, `npm test`, `npm run lint:strings`.

## Android app

The same React Native app (`apps/portico-react-native/apps/native`), for Android phones and tablets, Android TV, and Fire TV. Every screen is shared with the Apple targets; only the native layer is ported, under the same module names, methods and events. Built 3 Oct 2026 on branch `android-port` (not merged); what was built, what differs and what was not checked is in `Audits/2026-10-03-android-port-log.md`.

| Piece | Role |
|---|---|
| `android/` | One Gradle project, application id `tv.getportico.portico`, minimum Android 7 (API 24); phone and Leanback launchers in one manifest |
| Flavors | `google` (Google Play services; includes Google Cast) and `firetv` (Fire OS, minimum API 25; no Play services, no Cast, no home-screen rows) |
| `TelevisionDisplay.kt` | Overrides the density on a television so the shared screens lay out in the tvOS 1920×1080 space |
| `native/` (Kotlin) | `PorticoRuntime` (records sealed by an Android Keystore key, the bounded HTTP transport), `PorticoProfileTrust`, `PorticoNetwork` (NSD discovery, signatures, the TV grant), `PorticoAccountAuth` (browser sign-in through a Custom Tab), `PorticoCapabilities` (`MediaCodecList`), `PorticoPlaybackModule` + `PorticoPlayer` view and `PorticoPlaybackController` (Media3 ExoPlayer; subtitles drawn by the app), `PorticoListeningOwner` + `PorticoListeningService` (music and audiobooks: the v1 owner, a media session service, gapless through one playlist, crossfade through a second player), `PorticoSystemMedia` (video media session), `PorticoDownloads` (phones), `PorticoTopShelf` (televisions: Watch Next and one channel), `PorticoImagePicker`, `PorticoSubtitleFiles`, `PorticoCast` (google flavor) |

- Music and audiobooks declare what ExoPlayer decodes (`audioDecode`: FLAC, AAC, MP3, Opus, Vorbis, PCM); the server converts the rest (ALAC to FLAC).
- Picture in Picture on phones and tablets only. Downloads on phones and tablets only. Cast on phones and tablets in the `google` flavor only.
- Provider sign-in (Google, Apple) goes through the browser and returns by `portico://account-return`.
- Build: JDK 17, `./gradlew :app:assembleGoogleDebug` (Metro on 19424) or `:app:assembleGoogleRelease` (bundle embedded). The release build type is signed with the debug keystore until a release key exists. Checks are the Apple ones: `npx tsc --noEmit`, `npm test`, `npm run lint:strings`.

## Rules both clients follow

- **O(visible).** Windowed lists and grids, bounded pages and caches. No whole-library loads, and no client-side sorting or filtering of a library.
- **The server decides.** Clients never rank, group, hide or re-page what the server published. Unknown row ids fall back to the server's title text.
- **Stale pages are discarded.** A page request is bound to the document, viewer and revision it started from; `stale_continuation` restarts the list visibly.
- **Native text input only.** Each platform's keyboard, with dictation; no custom on-screen keyboards.
- **Sign-in by platform.** iPhone, iPad and web use Apple or Google SSO, Portico Account, or direct sign-in. Codes are for TVs only: the TV shows a code, and a signed-in phone or computer approves it.
- **No push notifications.** Alerts are in-app. When a sign-in ends, the app goes straight to the sign-in screen with the server and username filled in.
- **The player layout is fixed.** Dialogs and menus may change; the layout may not. Pause/Play is the only filled transport control. The option boxes are Audio, Subtitles, Quality, Speed and Sleep, collapsed to one sheet on phones. Music has no 10/30-second skip and no Speed (audiobooks keep both); a song starts in the mini player. More holds Playback info (delivery and why, codecs, size, bitrate, and on the web dropped frames and buffer), Watch Together and Report a problem. On Apple TV a film or episode never shows shuffle or repeat.
- **Card marks.** A watched check and a progress bar; there is no mark for "not watched yet". The caption under a card is one shared rule (an album names its artist, a book its author, an episode its number, date and show).
- **Pushed screens on iPhone and iPad** get a solid navigation bar (Back and the title) once their own header or hero has scrolled away; content never runs under the status bar. Every tab's top bar has Search, the bell and the account button.
- **Apple TV browse header** is two rows: the title (with the count), then tabs with Sort, Filters and Jump to. Filters are a right-hand panel with two levels and no text entry. The rail's focus uses the same ring as content.
- **Hide only what the server disabled**, never because a platform is about to catch up.
