# 19 — Native app engineering architecture

Comparative audit, 11 October 2026. **Static source review only**: nothing was built, installed, run or measured. Every statement about behaviour is a reading of the code, not an observation on a device.

Paths: Portico paths are relative to `/home/justin/projects/Portico`. `native/` below is short for `apps/portico-react-native/apps/native/`, `vega/` for `apps/portico-react-native/apps/vega/`, `core/` for `apps/portico-react-native/packages/client-core/src/`. Comparison paths start with the project name (`plezy/…`, `streamyfin/…`).

Jargon used once and then assumed:

- **RN** — React Native: screens written in TypeScript/React, drawn with the platform's real native views.
- **react-native-tvos** — the community fork of RN that adds Apple TV and Android TV support. Portico installs it under the name `react-native`.
- **New Architecture** — RN's current internals: *Fabric* (the renderer), *TurboModules* (native modules loaded lazily with typed specs), *bridgeless* (no old message bridge). The **interop layer** lets old-style modules keep working under it.
- **Hermes** — the JavaScript engine RN ships; the app bundle is pre-compiled to Hermes bytecode.
- **Native module** — a piece of Swift/Objective-C/Kotlin the JavaScript can call.
- **Focus engine** — the part of a TV platform that decides which control is highlighted and where the highlight goes when a direction is pressed.
- **Virtualised list** — a list that only creates the rows near the visible area.

## Scope and sources

**Portico, read in full or nearly so:** `06 Clients.md`, `01 Overview.md`, `Backlog.md`, the Android port log (all of phases 0–1, 4–5, the changes and questions; headings only for 2–3, which are the player's) and the first 200 lines of the Vega port log; `native/package.json`, `index.js`, `App.tsx`, `metro.config.js`, `babel.config.js`; `src/shell/*` (Shell, Rail, Roots, ScreenBoundary, ErrorBoundary, ShellFrame, rail-expansion); `src/ui` (Focusable, FocusHalo, theme, Text, Grid, Shelf, Artwork, ArtworkPipeline, Card, Page, the head of Sheet); `src/app` (the first 600 lines of `session.tsx` and its phase logic, content, detail, browse, browse-page-source, section-window, paging, preferences, refresh, i18n, routes, open, debug, device-events); `src/bridge` (README, queue-storage, credentials, profile-trust, known-servers, last-server, direct-memory, event-stream, device-platform, random-id, NativeViews, NativeActivity, NativeSiriRemote); `Native/PorticoPlayer.m` (which also holds the iOS `PorticoRuntime`, the HTTP transport and the Keychain code), `PorticoSecureStorageRead.h`, both `AppDelegate.swift`, `PorticoIntents.swift` (link hand-off), both `Info.plist`, entitlements, Podfiles and Xcode build settings; Android `MainApplication.kt`, `MainActivity.kt`, `TelevisionDisplay.kt`, `AndroidManifest.xml`, `app/build.gradle`, `gradle.properties`, `PorticoPackage.kt`, `PorticoEventModule.kt`, `PorticoRecordStore.kt`, parts of `PorticoRuntime.kt` and `PorticoProfileTrust.kt`, and a lifecycle survey (grep) of every Kotlin module; Vega `package.json`, `manifest.toml`, `metro.config.js`, file inventory; `scripts/shared-packages.mjs`; `.github/workflows/*`; `test/` harness (`hook-driver.ts`, `register.mjs`, `resolve.mjs`, the stub). I also read the parts of react-native-tvos 0.83.10-1 in `node_modules` that Portico depends on (`Pressability.js`, `ReactAndroidHWInputDeviceHelper.java`, `DisplayMetricsHolder.kt`, `ReactRootView.java`, `ReactHostImpl.kt`, `DefaultNewArchitectureEntryPoint.kt`).

**Checked mechanically:** the shared-package copy against portico-server (`diff -rq` of `client-core/src`, `design/src`, `i18n/src|scripts|baselines`): **no differences** in the working trees today. Exported method names of every iOS module against its Kotlin twin: **they match** (iOS has two extra test-only methods; Cast exists only in the `google` flavor).

**Comparison projects read:** Plezy (`pubspec.yaml`, `lib/focus/*`, `lib/database/tables.dart`, `lib/services/{device_performance,image_cache_service,api_cache,log_upload_service,tvos_system_navigation_service,apple_tv_remote_touch_service}.dart`, `lib/utils/platform_detector.dart`, `tvos/`, `scripts/`, `.github/workflows/build.yml`); Streamyfin (`package.json`, `app/_layout.tsx` query setup, `modules/`, `providers/`, `utils/atoms/`, `plugins/`, `eas.json`, `docs/tv-focus-guide.md`); Swiftfin (project layout, Swift package list, deployment targets, workflows); jellyfin-androidtv (`gradle/libs.versions.toml`, `app/build.gradle.kts`, package layout, `telemetry/`, `integration/`, manifest); Wholphin (version catalogue, build file); jellyfin-android (version catalogue, flavors, package layout).

**Not covered:** the player engines (auditor 20), screen-by-screen UX (21), connection and discovery logic (22). The second half of `session.tsx`, most of `src/screens`, `PorticoListeningOwner`, `PorticoPlaybackController`, `PorticoDownloads` and the Vega shims' internals were not read line by line. Finamp was not opened. Nothing about binary size, start-up time, memory or frame rate was measured for any project; where those are discussed the reasoning is architectural.

## How Portico does it today

### One JavaScript app, three native shells

One React Native app (`native/`) draws every screen for iPhone, iPad, Apple TV, Android phones and tablets, Android TV and Fire TV. A second, thin app (`vega/`) runs **the same screen files** on Amazon's Vega OS.

- **Framework:** `react-native` is aliased to `react-native-tvos@0.83.10-1`, React 19.2.8 (`native/package.json`). New Architecture and Hermes are on (`android/gradle.properties`: `newArchEnabled=true`, `hermesEnabled=true`). There is **no Expo**.
- **Third-party libraries: thirteen.** Navigation (`@react-navigation/native`, `native-stack`, `react-native-screens`, `safe-area-context`), animation (`react-native-reanimated` 4.6 + `worklets`), drawing (`react-native-svg`, `qrcode-svg`), two polyfills (`@bacons/text-decoder`, `react-native-url-polyfill`) and `semver`. No storage library, no image library, no list library, no query/state library, no crash reporter, no analytics.
- **Apple:** two Xcode projects (`ios/`, `tvos/`) share one folder of Objective-C/Swift (`Native/`, 5,700 lines). tvOS adds a Top Shelf extension. Deployment target 15.1 for both. CocoaPods; the only extra pod is Google Cast on iOS.
- **Android:** one Gradle project, one application id for phones *and* televisions, two flavors (`google`, `firetv`), 8,000 lines of Kotlin under `native/` (a Kotlin package literally named `native`). Dependencies: OkHttp, Media3, Custom Tabs, BouncyCastle, Cast (google flavor only). Minimum Android 7 (API 24; Fire flavor 25), compile/target 36.
- **Vega:** its own `package.json` on stock `react-native@0.83.0` plus Amazon's `react-native-kepler`. `vega/metro.config.js` reads the screens from `../native/src` in place and swaps three things at bundle time: `react-native` becomes a shim file (`src/shims/react-native.ts`), Reanimated becomes a 120-line JavaScript stand-in, and any shared file can be replaced by a same-path file under `src/overrides`. The "native modules" on Vega are **JavaScript** (`vega/src/bridge`, 2,300 lines), including cryptography (`@noble/*`), an HLS client (`hls.ts`) and the record store (plain files). About 3,800 lines in all are Vega-specific.

### The native boundary

JavaScript reaches native code through eleven modules with the same names, methods and events on Apple and Android (`src/bridge/README.md`; `PorticoPackage.kt`). They are **old-style bridge modules** (`RCT_EXPORT_MODULE`, `@ReactMethod`) accessed as `NativeModules.PorticoX`, running through the New Architecture's interop layer. There are no TurboModule specs and no generated bindings. Views (`PorticoPlayer`, the route picker, the volume slider) are old-style view managers via `requireNativeComponent`; their events go through module events because old view events crashed a release build (`src/app/legacy-events.ts`).

`PorticoRuntime` is the workhorse: identifiers, random bytes, **all HTTP for the API** (a bounded transport: one session, 12 requests in flight, 64 waiting, 1 MB bodies or 8 MB for API GETs, 15 s, no redirects, cookies or cache — `Native/PorticoPlayer.m:87-191`, `PorticoTransport.kt`), and **all durable storage**.

### Storage

There is no database and no general key-value store. Everything the app remembers between launches is a small JSON record (at most 256 KB) in a secure store:

| Platform | Where | Protection |
|---|---|---|
| iPhone, iPad, Apple TV | Keychain generic passwords under `tv.getportico.portico.{queues,credentials,playback,…}` (`PorticoPlayer.m:220-331`) | `AfterFirstUnlockThisDeviceOnly` |
| Android, Android TV, Fire OS | One file per record in the no-backup folder, sealed with AES-256-GCM by one Android Keystore key (`PorticoRecordStore.kt`) | Key never leaves the Keystore; no backup or device transfer |
| Vega | Files in the app's private `/data`, two slots per record with a checksum (`vega/src/bridge/storage.ts`) | None beyond the app sandbox |

`src/bridge/queue-storage.ts` (`NativeQueueStore`) gives JavaScript one read-modify-write per key, serialised per key inside the JS runtime. It holds the device preferences (`preferences:v1`), the list of known servers, the last server per account, the last direct sign-in's address and username, and the playback queue id. Credentials, the selected viewer, the playback journal and the PIN-protected profile vault have their own methods with read-back verification and a "quarantine" flag. The rule everywhere is **a read never repairs or overwrites what it cannot open**.

Downloads keep their own manifest and files (phones and tablets only). Nothing else is cached to disk by the app; images use the platform HTTP cache.

### State and data

- **Session:** one module-level `ViewerService` (client-core) plus a React context (`src/app/session.tsx`, 1,160 lines) that sequences sign-in, restore, renewal and sign-out. A remembered sign-in is shown immediately ("the app always opens into the shell") while it is verified in the background; until then the API object points at a placeholder origin (`session.tsx:217-219, 311-316`).
- **Content:** each screen creates a client-core service (`LibraryContentService`, `DetailService`, `ShowWorkspaceService`, a windowed collection…) inside a hook and binds its snapshot to React (`src/app/content.ts`, `detail.ts`, `browse.ts`, `section-window.ts`). Services validate every response and fence stale pages by document, viewer and revision. Nothing is persisted; Home keeps the last document in memory per viewer for instant re-show.
- **Small shared stores:** hand-written external stores read with `useSyncExternalStore` (the exit gate, connection status, device events, rail expansion) so that frequent changes do not re-render screens.
- **Refresh:** pull-to-refresh on touch; on every platform a screen that comes back after five minutes refreshes itself (`src/app/refresh.tsx`).
- **Events:** one server-sent-events feed, read through `XMLHttpRequest` because RN's `fetch` cannot stream (`src/bridge/event-stream.ts`); the connection is cut and resumed after 1 MB of text.

### Navigation and input

- **Structure:** `@react-navigation` native stack. A custom navigator (`src/shell/Roots.tsx`) keeps seven "roots" alive (Home, Library, Saved, Channels, Downloads, Search, Settings), each with its own stack that can show any page; hidden roots are `display: none`.
- **Links:** every incoming URL passes a native allowlist first (`PorticoURLRoutes.swift`, `PorticoUrlRoutes.kt`), then a hand-written regular-expression router in `Shell.tsx:256-297`. Universal links, Handoff, Spotlight and Siri become `portico://` routes on iOS; Android app links are declared but unverified (no release signing key yet).
- **Back / Menu:** `BackHandler` pops the stack, then moves focus to the rail, and only then leaves the app; on tvOS `TVEventControl` switches the Menu key between "ours" and "exit" from a tiny store that tracks stack depth and rail focus (`Shell.tsx:71-90, 160-176, 369-379`). On Android televisions the remote's Menu key is re-sent as a held Select so that "long-press" actions answer it (`MainActivity.kt`, `dispatchKeyEvent`).
- **Focus:** the platform's own focus engine does the moving (UIKit's on tvOS, Android's on Android TV). Portico shapes it with react-native-tvos's `TVFocusGuideView`: one guide per screen that leaves the engine while covered (`ScreenBoundary.tsx`), one per shelf with a right-edge trap (`Shelf.tsx`), one for the rail whose destination is the selected item (`Rail.tsx`). A time-boxed "entry focus" claim (`useEntryFocus`, `theme.ts`) gives a late-loading screen its first focus once and never again. On Vega the SDK's focus does not re-home when a view disappears, so `vega/src/shims/focus.tsx` rebuilds that from focus/blur events and timers.
- **One interaction primitive:** `src/ui/Focusable.tsx` wraps `Pressable` for every card, button and row (focus scale, ring, hover on iPad, press). Screens never use `Pressable`, `Text`, raw colours or `StyleSheet.create` directly.
- **Siri Remote touch surface:** a native module reports absolute finger position for scrubbing (`NativeSiriRemote.ts`).
- **Television layout space:** screens are designed at 1920×1080 units, which is what tvOS reports. Android televisions report 960×540, so `TelevisionDisplay.kt` overrides the density for the whole process and turns off one RN re-read through an internal feature-flag API. Vega does the same with a scaled canvas in its `react-native` shim.

### Lists and images

- **Lists:** RN's built-in `FlatList`/`VirtualizedList` only. `WindowedGrid` (`src/ui/Grid.tsx:116-195`) is a grid whose row count comes from the server's total, with placeholder cells for pages not yet loaded; it reports the visible range to a client-core windowed collection that fetches, prefetches and evicts pages (5 pages of 60 on TV, 8 on handheld). This is random access: a letter jump scrolls straight to an offset. Shelves are horizontal `FlatList`s with `windowSize` 3.
- **Images:** RN's built-in `Image`, with the bearer token as a request header (`src/app/content.ts:53-86`). A client-core scheduler decides *when* each image may mount (visual order, 6 at a time, 12 s timeout, backoff retries — `src/ui/ArtworkPipeline.ts:36-69`). One size bucket per rendered size (400/800/1920, though today's server only serves ≤400 and 1920). Placeholder is the title in type on a slate tile; no blurhash. Disk cache is the platform's: `URLCache` 32 MB memory / 256 MB disk on iOS, 128 MB on tvOS (`AppDelegate.swift`); Fresco's defaults on Android.

### Design, text, accessibility

Tokens (colour, type, spacing, radii, icons) come from the shared `design` package that the web also compiles. One dark theme. Manrope in four weights is bundled. Strings come from the shared `i18n` package with Portico's own ICU message formatter; a lint rejects raw strings (three baseline exceptions). English only at launch. Dynamic Type is honoured on handhelds with no cap (except tiny badges at 2×); televisions get an in-app "Text size: large" (1.2×). Reduce Motion follows the system and an in-app switch. Accessibility labels and roles are set in the primitives (175 uses across 51 of 130 component files).

### Build, release, testing

- **CI** (`.github/workflows/ci.yml`): shared-package identity check, `tsc`, string lint, `node --test` (about 240 tests), and an Android `assembleDebug`. Security workflow: gitleaks and CodeQL. Dependabot weekly.
- **Not in CI:** any iOS/tvOS build, ESLint, the Vega app, any device or simulator test.
- **Release:** none yet. Android release builds are signed with the debug keystore, not minified, and carry all four CPU architectures. No signing team in the Xcode projects. No over-the-air updates.
- **Tests:** Node's test runner over the real bridge and hook code with `react-native` replaced by a stub (`test/resolve.mjs`). Hooks run under a hand-written driver that installs itself as React's dispatcher (`test/hook-driver.ts`). No component rendering, no end-to-end tests. Television behaviour was proven by hand on emulators with screenshots (the port logs).

## How the others do it

### Jellyfin's official clients — one project per platform

Jellyfin has no shared client code beyond per-language SDKs generated from the server's OpenAPI.

- **jellyfin-androidtv** (Android TV, Fire TV): Kotlin. Leanback (Google's older TV toolkit) being migrated to Compose; Koin for dependency injection; repositories over `jellyfin-sdk-kotlin`; Coil for images; Timber logging; **ACRA crash reports sent to the user's own Jellyfin server** (`jellyfin-androidtv/app/src/main/java/org/jellyfin/androidtv/telemetry/TelemetryService.kt`); WorkManager for the home-screen channel (`integration/LeanbackChannelWorker.kt`); a system screensaver (`integration/dream/`); a content provider for Android TV's global search (`integration/MediaContentProvider.kt`). Minimum API 23, R8 minification on (`app/build.gradle.kts:56`), fastlane for publishing, Kotest unit tests.
- **jellyfin-android** (phones): a WebView around jellyfin-web with native ExoPlayer, downloads (Room), Cast and Android Auto. Two flavors, `libre` and `proprietary`, the latter carrying Google Play services (`app/build.gradle.kts:78-84,173-174`) — the same split Portico makes with `google`/`firetv`.
- **Swiftfin** (iPhone, iPad, Apple TV): SwiftUI. 640 Swift files in `Shared/`, 110 iOS-only, 24 tvOS-only. Coordinators for navigation, view models, Factory for injection, CoreStore (Core Data) for servers and users, `Defaults` for settings, Nuke + BlurHashKit for images, `CollectionVGrid`/`CollectionHStack` (UICollectionView-backed) for paging lists, Pulse for an in-app network/log viewer. Deployment targets **iOS 18.6 and tvOS 26.1**. fastlane + a TestFlight workflow.
- **Wholphin** (third-party Android TV): Compose for TV, Hilt, Room, DataStore, Coil, ACRA, Media3 + mpv, baseline profiles (`profileinstaller`), ABI splits, R8, Robolectric tests.

### Plezy — Flutter, one renderer everywhere

One Dart codebase (989 files, about 266,000 lines) for iOS, Android, Android TV/Fire TV, **tvOS**, macOS, Windows and Linux. Flutter draws every pixel itself, so nothing on screen is a platform control.

- **tvOS is reached with a custom Flutter engine** pinned by version and checksum (`plezy/tvos/engine.version`, `engine.sha256`, `tvos/Podfile:24-79`). Flutter does not officially support tvOS.
- **Focus is its own subsystem** (`plezy/lib/focus/`, 24 files): key classification, a focus-memory tracker, an input-mode tracker (pointer vs keys), long-press-on-Select, key-repeat handling, a boundary that stops covered routes stealing focus (`covered_route_focus_boundary.dart`), and — because the UIKit focus engine is not in play — a Dart re-creation of how far a Siri Remote swipe moves focus, tuned against measurements of the real engine (`lib/services/apple_tv_remote_touch_service.dart:22-45`).
- **State:** `provider` (ChangeNotifier) with 18 providers and 135 files of services.
- **Local database:** Drift (SQLite) with tables for an API response cache, downloads, sync rules, connections, profiles and offline watch progress (`lib/database/tables.dart`).
- **Images:** `cached_network_image` with a disk cache of 3,000 objects and 14 days, concurrency 6 (3 on weak devices), credentials stripped from cache keys (`lib/services/image_cache_service.dart:20-58`).
- **Weak devices:** a "reduced effects" tier chosen automatically on 32-bit, low-RAM or ≤2.2 GB Android devices, which also caps the GPU cache (`lib/services/device_performance.dart:18-60`).
- **TV detection:** leanback or Fire TV system features, an automotive exclusion, and a user "force TV mode" switch (`lib/utils/platform_detector.dart:13-130`).
- **Also:** 22 languages (slang), Sentry crash reporting to its own host, a log buffer with "upload and get a Log ID" (`lib/services/log_upload_service.dart`), gamepad and keyboard-shortcut services, an assistive-technology service, Android Automotive restrictions, auto-update on desktop, Discord presence.
- **Pipeline and tests:** a release workflow that builds signed, attested artefacts for every platform (split-per-ABI APKs, IPA, notarised DMG, Windows x64/arm64, Linux); 664 test files; Maestro end-to-end flows against a real Jellyfin; performance bench scripts; about forty repository checks (hard-coded strings, shrinker rules, tvOS test wiring…).

### Streamyfin — the closest sibling (React Native)

React Native **with Expo** (SDK 57) on `react-native-tvos@0.86.0-2`, three minor versions ahead of Portico. 85 runtime dependencies.

- **Routing:** expo-router (files are routes; deep links come for free).
- **Data:** TanStack Query with the cache **persisted to MMKV for 24 hours** (`streamyfin/app/_layout.tsx:301-321`), jotai atoms for settings and UI state, the official `@jellyfin/sdk`, axios.
- **Lists and images:** FlashList and expo-image (blurhash placeholders, disk cache).
- **Native code:** eleven local Expo modules (`modules/`: mpv player, ExoPlayer, background downloader, Top Shelf cache, TV recommendations, TV search, tvOS user profile…), Nitro modules, config plugins that patch the native projects at prebuild.
- **TV:** a **separate build** (`EXPO_TV=1`) from the same source; TV-specific hooks and modals (`useTV*`), and a written focus guide (`docs/tv-focus-guide.md`) whose advice includes "prefer `ScrollView` over `FlatList` for TV rows when focus must be reliable" and "use `TVFocusGuideView` destinations, held in state not refs". tvOS builds carry the **user-management entitlement** so each Apple TV user gets their own data (`plugins/withTVUserManagement.ts`).
- **Also:** Sentry, i18next + Crowdin, Jest tests (dozens of hook tests), an `e2e/` folder, EAS build profiles, nativewind (Tailwind) for styling.

### Others in one line each

- **Plex, Infuse, Emby** (closed): native per platform or proprietary cross-platform toolkits; not comparable from source.
- **Finamp** (Flutter, music): not opened for this audit.

## Architectural differences

### A-19.1 The cross-platform bet: React Native with native modules
- **Portico:** one TypeScript app drawn with each platform's real views, plus about 5,700 lines of Objective-C/Swift and 8,000 of Kotlin for what JavaScript cannot do well (players, storage, network, discovery, system media, home-screen rows).
- **Jellyfin:** fully native, one separate project per platform (Kotlin for TV, a WebView shell for phones, SwiftUI for Apple). **Plezy:** Flutter, one renderer for seven platforms. **Streamyfin:** React Native + Expo.
- **Why it matters:**

  | Cost | RN + native modules (Portico) | Flutter (Plezy) | Native per platform (Jellyfin) | WebView shell (jellyfin-android) |
  |---|---|---|---|---|
  | TV focus quality | Platform engine on tvOS and Android TV; hand-built on Vega | Self-built everywhere, incl. Siri Remote swipes | Platform engine, best fit | Poor (web spatial navigation) |
  | Weak sticks | JS thread + native views; lists are the risk | Skia/Impeller GPU memory is the risk (Plezy caps it) | Best | Worst |
  | Platform features | Any, by writing a module twice (three times with Vega) | Any, by writing a plugin per platform | Direct | Through a bridge |
  | Desktop | Not covered by this stack as built | Free | Separate project | Electron/Qt project |
  | Shared with the web app | `client-core`, `design`, `i18n` shared as source | Nothing | Nothing | Everything |
  | People needed | One JS codebase + two native ones in sync | One Dart codebase + thin native | One team per platform | One web team |

- **Assessment: Trade-off (subjective), and I think Portico chose well for its goals.** It is deliberate (`06 Clients.md`: "ports of the tvOS app"). The decisive advantages are real platform focus on the two biggest TV platforms and sharing the headless core with the web app, which no competitor does. The costs are the native layer written two or three times and a dependence on the react-native-tvos fork (A-19.4). What would change my view: measured scroll and focus performance on a Fire TV Stick or Chromecast that cannot be fixed inside RN.

### A-19.2 One Android binary for phones and televisions
- **Portico:** one application id and one APK per flavor; `Platform.isTV` picks the layout at run time; one manifest declares both launchers (`AndroidManifest.xml`); modules differ by device (`PorticoPackage.kt`: Top Shelf on TVs, Downloads on handhelds).
- **Jellyfin:** two different apps (`jellyfin-androidtv`, `jellyfin-android`). **Streamyfin:** two builds from one source (`EXPO_TV=1`). **Plezy:** one binary, run-time detection.
- **Why it matters:** one listing and one install for the owner; but the phone build carries TV code and the reverse, Google Play reviews TV and phone quality together, and a TV-only crash blocks a phone release.
- **Assessment: Trade-off.** Deliberate (build.gradle comment "One application for phones and televisions"). Splitting later is small (a third flavor dimension). Apple is already split into two targets.

### A-19.3 How a device is judged to be a television
- **Portico:** a single signal, `UiModeManager.currentModeType == TELEVISION`, both in `TelevisionDisplay.kt` and inside RN's `Platform.isTV`. No override.
- **Plezy:** leanback *or* Fire TV system feature, minus automotive, plus a user "force TV" setting (`plezy/lib/utils/platform_detector.dart:13-130`).
- **Why it matters:** cheap "Android TV boxes" that run tablet Android report a normal UI mode. Portico would give them the touch layout, which draws **no focus ring** (`theme.ts`: `focusRing: 0` off TV), so a remote-only box is unusable with no way out.
- **Assessment: Portico behind** (small). A "Use the TV layout" switch plus a second signal would close it; the density override would have to follow the same decision.

### A-19.4 Dependence on the react-native-tvos fork, and how current it is
- **Portico:** `react-native-tvos@0.83.10-1`; Vega is pinned to stock 0.83.0 by Amazon's SDK. Upgrades are manual (no Expo).
- **Streamyfin:** `react-native-tvos@0.86.0-2` with Expo SDK 57, upgraded through Expo's tooling.
- **Why it matters:** every TV feature Portico relies on (`TVFocusGuideView`, `TVEventControl`, the snap-to-focus scroll props, TV press handling in `Pressable`) exists only in the fork, which is maintained by a very small group. Vega fixes the *shared* code to whatever RN version Amazon ships, so the Apple/Android app cannot use APIs newer than Vega's without an override file.
- **Assessment: Trade-off, with a real concentration risk.** There is no alternative for tvOS in RN. Worth writing down an upgrade routine (the port log already lists one internal API to re-check at each upgrade) and deciding whether Vega is allowed to hold the others back.

### A-19.5 New Architecture is on, but Portico's own modules are old-style
- **Portico:** Fabric, bridgeless and Hermes are enabled; all eleven modules and three views are legacy bridge modules behind the interop layer ("Bridge modules by design", `PorticoPackage.kt`). A callable-module shim swallows legacy view events (`legacy-events.ts`). On Vega the legacy `NativeModules` object throws, so a shim serves Portico's JS modules.
- **Streamyfin:** Expo Modules API and Nitro modules (typed, generated). **Plezy:** platform channels.
- **Why it matters:** the interop layer is a compatibility path; RN has been removing legacy pieces release by release. Old-style modules get no typed spec, so nothing checks that the Swift, Kotlin and Vega-JS implementations agree with what the TypeScript calls.
- **Assessment: Portico behind** on durability, fine for today. Converting to TurboModule specs is medium work (one TypeScript spec per module, codegen, mechanical changes to 11 modules × 2 platforms) and would turn "same names by convention" into a compile-time contract.

### A-19.6 The native contract is kept in step by hand
- **Portico:** method and event names match across iOS and Android today (checked). Differences are recorded in prose (the port log): UUID letter case, "after first unlock" having no Android twin, redirect handling, path normalisation.
- **Others:** Flutter/Expo plugins have one Dart/TS interface per plugin; native Jellyfin apps have no such contract to keep.
- **Why it matters:** three implementations (Apple, Android, Vega-JS) of security-sensitive behaviour — the PIN vault, the TV grant envelope, the bounded transport — must stay byte-compatible. Today only reviewers enforce that.
- **Assessment: Trade-off.** Shared conformance tests (the same vectors run against all three vault and envelope implementations) would be a small, high-value addition.

### A-19.7 Dependency count
- **Portico:** 13 runtime npm packages; 5 Gradle libraries; 1 extra pod.
- **Streamyfin:** 85 npm packages. **Plezy:** about 45 pub packages plus four vendored forks. **Swiftfin:** 25 Swift packages. **jellyfin-androidtv:** about 40 libraries.
- **Why it matters:** fewer upgrades, a smaller supply chain, and no behaviour Portico does not understand; in exchange Portico wrote and must maintain its own storage, transport, image scheduling, paging, message formatting and test harness.
- **Assessment: Portico ahead** on hygiene, and it is clearly deliberate. The honest cost is in A-19.26 (lists), A-19.29 (images) and A-19.45 (tests), where a mature library would bring behaviour Portico has not built.

### A-19.8 Vega as a shim over the same screens
- **Portico:** Vega shares every screen file unchanged; platform differences live in a `react-native` shim, a Reanimated stand-in, a rebuilt focus layer and JS "native" modules (`vega/metro.config.js`, `vega/src/shims`, `vega/src/bridge`).
- **Others:** none of the compared projects supports Vega OS.
- **Why it matters:** no fork to keep in step — a screen change reaches Vega for free. But the shim imitates tvOS behaviours (focus re-homing, focus-following scroll snap, preferred focus) with timers, and animation runs on the JS thread. The port log itself calls focus "the part most likely to show an edge on real hardware".
- **Assessment: Portico ahead** on reach, **unclear** on quality until a physical Fire TV has been used. Keeping it a separate app with overrides, rather than `Platform.OS === 'kepler'` branches in shared files, is the right shape.

### A-19.9 Shared packages by copy and checksum
- **Portico:** `client-core`, `design` and `i18n` are owned by portico-server; the RN repo holds a byte-identical copy checked by `scripts/shared-packages.mjs` in CI. The copies are identical today.
- **Others:** published SDK packages (`@jellyfin/sdk`, `jellyfin-sdk-kotlin`, `jellyfin-sdk-swift`), generated from OpenAPI and versioned.
- **Why it matters:** Portico shares *behaviour* (paging, fencing, presentation rules), not just types — far more than an SDK. The copy means every client-core change is two commits in two repositories, and CI in the RN repo checks against portico-server's default branch, so work on a server feature branch fails the RN check until both land.
- **Assessment: Portico ahead** on what is shared; the mechanism is a **trade-off**. Deliberate (`01 Overview.md`). A git subtree or a single workspace would remove the two-commit dance; not urgent.

### A-19.10 Shared code is imported by deep relative path
- **Portico:** `import … from '../../../../packages/client-core/src/presentation/index'` throughout `native/src`.
- **Others:** package names (`@jellyfin/sdk`, Dart package imports).
- **Why it matters:** any file may reach into any internal file of client-core, so client-core has no enforceable public surface, and moving a file breaks apps. Vega's override mechanism depends on these exact paths.
- **Assessment: Portico behind** (small, cosmetic until client-core is refactored). A path alias (`@portico/client-core`) resolved by Metro and `tsc` is a small change.

### A-19.11 State management: services with snapshots, not a state library
- **Portico:** client-core classes expose `subscribe`/`getSnapshot`; hooks create one per screen and mirror its snapshot into React state; a handful of tiny external stores avoid broad re-renders. One large context (`SessionValue`, about 40 fields) carries the session.
- **Streamyfin:** TanStack Query (cache, dedupe, retries, background refetch) + jotai. **Plezy:** provider/ChangeNotifier. **Swiftfin:** view models with a state macro. **androidtv:** Koin + ViewModel + Flow.
- **Why it matters:** Portico's approach keeps all protocol rules in code shared with the web and has no cache-staleness timers (the "Fresh" principle). But each screen instance owns a private service, so two screens showing the same thing fetch it twice, and anything the session context publishes re-renders every consumer (see B-19.2).
- **Assessment: Trade-off (subjective).** Deliberate ("Shared behaviour lives in client-core"). I would keep it. The one structural weakness is the wide session context; splitting identity (stable) from token (rotating) would remove a class of accidental re-creation.

### A-19.12 Nothing fetched is kept between launches
- **Portico:** content is memory-only. A cold start shows the shell, then waits for the server.
- **Streamyfin:** the query cache is persisted for 24 hours, so the last Home and library pages draw at once and work offline. **Plezy:** an `ApiCache` table in SQLite, plus pinned metadata for downloads. **Swiftfin/androidtv:** nothing persisted.
- **Why it matters:** perceived start-up on a slow link or a sleeping server, and whether anything but Downloads is browsable offline.
- **Assessment: Trade-off.** Portico's "Fresh" principle forbids *time-based* caches, not revision-keyed ones: a persisted Home document shown instantly and replaced when its revision differs would be consistent with the principle. Medium work (a bounded on-disk store, viewer-scoped, cleared on sign-out). See F-19.3.

### A-19.13 No local database
- **Portico:** none. Records are individual JSON blobs ≤256 KB; downloads have a manifest file.
- **Plezy:** Drift/SQLite, 13 tables. **Swiftfin:** Core Data via CoreStore. **Wholphin, jellyfin-android:** Room. **Streamyfin:** MMKV.
- **Why it matters:** Portico cannot hold anything large or queryable on the device — fine while clients are thin, limiting for offline libraries (many downloads with metadata, offline play history, sync rules).
- **Assessment: Trade-off.** Consistent with "thin clients". The pressure will come from downloads (auditor 14's domain); the 256 KB record cap is the first wall.

### A-19.14 Ordinary preferences live in the secure store
- **Portico:** `preferences:v1` (reduce motion, speeds, text size), known servers and the last sign-in's username are Keychain items / Keystore-sealed files (`preferences.ts`, `queue-storage.ts`).
- **Others:** `UserDefaults`/`Defaults`, SharedPreferences/DataStore, MMKV, shared_preferences; only tokens go to secure storage.
- **Why it matters:** simpler (one store), and nothing leaks into backups. But secure stores fail in ways plain files do not (locked device, lost key), a Keychain read is far slower than a defaults read, and on iOS the values survive deleting the app (B-19.12). The theme waits on this read at every launch (`PreferencesProvider`), so "large text" and "reduce motion" apply a moment after first paint.
- **Assessment: Portico behind** (small). Non-secret device preferences belong in the plain platform store.

### A-19.15 Hand-written secure storage with "never repair"
- **Portico:** own Keychain code, own Keystore+AES-GCM file format, own two-slot files on Vega; writes are read back before "saved"; missing and unreadable stay different all the way to JavaScript; an unreadable record is never overwritten by a read.
- **Others:** `expo-secure-store`, `keychain-swift`, a credential vault class in Plezy with a `prefs_recovery` service; androidtv keeps accounts in a preferences store.
- **Why it matters:** Portico's rules are stricter and better documented than any library's. The flip side of "never repair" is that some failures are permanent (B-19.4).
- **Assessment: Portico ahead** on rigour, with one gap: there is no recovery path. Plezy's existence of `prefs_recovery.dart` and `tvos_database_recovery_store.dart` shows that field corruption is common enough to need one.

### A-19.16 All API traffic goes through a bounded native transport
- **Portico:** JavaScript never uses `fetch` for the API; `PorticoRuntime.networkRequest` enforces caps on concurrency (12 + 64 queued), size (1 MB, 8 MB for GETs), time (15 s), and refuses redirects, cookies and caching.
- **Others:** `fetch`/axios (Streamyfin), `package:http` with a shared client (Plezy), SDK HTTP clients (Jellyfin apps). No one else caps response size.
- **Why it matters:** a hostile or broken server cannot exhaust the client. The fixed 15 s ceiling also means no request can be a long poll (the port log had to shorten the event poll to 10 s) and the 8 MB ceiling is a hard limit on any single document.
- **Assessment: Portico ahead** for safety; watch the ceilings as libraries grow (a Home document or a guide window that passes 8 MB fails outright rather than degrading).

### A-19.17 Navigation: seven kept-alive stacks under a custom navigator
- **Portico:** `Roots.tsx` uses React Navigation's tab state machine without its tab bar; each root has a full native stack; hidden roots stay mounted.
- **Streamyfin:** expo-router file routes with native bottom tabs. **Plezy:** Flutter Navigator with nested navigators. **Swiftfin:** coordinators. **androidtv:** fragments, moving to Navigation 3.
- **Why it matters:** returning to a destination shows it exactly as left, with no refetch — a quality touch several competitors lack. The price is memory: every root and every pushed page stays mounted with live subscriptions (B-19.14).
- **Assessment: Portico ahead** on feel; needs a bound for weak devices.

### A-19.18 Deep links: a native allowlist and a hand-written router
- **Portico:** links are filtered natively, then matched by regular expressions in `Shell.tsx`; React Navigation's own `linking` configuration is not used. Secrets ride in URL fragments and are held in memory by the receiving screen.
- **Streamyfin:** expo-router maps URLs to routes automatically. **androidtv:** search and view intents into fragments.
- **Why it matters:** the allowlist is stricter than a generic mapping (only seven route shapes exist), which is good security. But routing is tied to the shell's lifecycle, which is where B-19.1 comes from.
- **Assessment: Trade-off.** Keep the allowlist; give links a queue that survives until the session is usable.

### A-19.19 No state restoration after the system kills the app
- **Portico:** `MainActivity.onCreate` passes `null` to the superclass on purpose; navigation state is not saved; the app reopens at Home.
- **Others:** the same in practice for Plezy, Swiftfin and Streamyfin. jellyfin-androidtv restores its fragment back stack in some paths.
- **Why it matters:** Fire TV sticks kill background apps aggressively; pressing Home during browsing and coming back often means starting again at Home.
- **Assessment: Trade-off**, parity with peers. React Navigation can persist and restore state cheaply (small work), but restored screens must tolerate items that no longer exist.

### A-19.20 The focus engine is the platform's (except on Vega)
- **Portico:** UIKit's focus engine on tvOS and Android's `FocusFinder` on Android TV, steered with focus guides.
- **Plezy:** Flutter's focus tree on every platform, with a Dart model of Siri Remote swipe distance and repeat timing because the system engine is not involved. **jellyfin-androidtv / Wholphin:** Leanback and Compose focus. **Swiftfin:** SwiftUI focus.
- **Why it matters:** on Apple TV, swipes, inertia, the focus sound and game-controller input behave like every other app with no code. Plezy needed a probe app and 101 measured swipe sessions to approximate that.
- **Assessment: Portico ahead** on tvOS and Android TV. On Vega, Portico is in Plezy's position (rebuilding behaviour) but with less machinery and no tests.

### A-19.21 Focus memory by guides and a one-shot claim
- **Portico:** `autoFocus` guides remember the last focused child per screen, shelf and rail; `useEntryFocus` claims focus once within a time window when data arrives; the rail is remounted when the player closes because a hidden guide kept a stale destination (`Shell.tsx:387-396`); a module-level flag stops a late-loading screen stealing focus while the viewer is browsing the rail (`theme.ts`, `railBrowsing`).
- **Plezy:** explicit `FocusMemoryTracker` keyed by item id, and `CoveredRouteFocusBoundary` for the same "covered screen steals focus" problem Portico solves with `focusable={focused}` on the screen's guide. **Streamyfin:** documents the same pitfalls.
- **Why it matters:** Portico's memory is positional and implicit (the guide's own), so when a list's contents change under it the remembered child can be gone; Plezy's is keyed by identity and prunes.
- **Assessment: Trade-off.** Portico's is less code and uses the platform; the timers and the remount are scars that show where it is fragile. An automated TV focus test would protect it (F-19.12).

### A-19.22 Menu and Back policy
- **Portico:** Back pops, then focuses the rail, then exits; the tvOS Menu key is handed to or taken from the system from a store that updates on every focus move. Android TV's Menu key is turned into a synthetic long press of Select.
- **Plezy:** a dedicated channel into its custom engine to pass Menu through (`tvos_system_navigation_service.dart`), with ordering logic for in-flight updates. **androidtv:** default Android back.
- **Why it matters:** "Back never dumps you out of the app unexpectedly" is an App Store review point on tvOS and a common complaint on Android TV.
- **Assessment: Portico ahead** on tvOS. The Android Menu-as-long-press trick has a real defect (B-19.3).

### A-19.23 Siri Remote touch surface
- **Portico:** a small module exposes absolute touch position and swipes only while enabled, for scrubbing.
- **Plezy:** synthesises all navigation from raw touches. **Swiftfin:** SwiftUI gestures.
- **Assessment: Portico ahead** — narrow, native, off by default.

### A-19.24 Keyboards, game controllers and pointers
- **Portico:** hover states for iPad pointers in `Focusable`; game controllers work on tvOS only because the system maps them to focus; no keyboard shortcuts; no key mapping beyond play/pause and the PIN pad.
- **Plezy:** `gamepad_service.dart`, `keyboard_shortcuts_service.dart` with a hotkey recorder, an input-mode tracker that switches the UI between pointer and key navigation.
- **Why it matters:** iPad with a hardware keyboard, Android TV with a keyboard or gamepad, and any future desktop build.
- **Assessment: Portico behind**, matching its current platforms' needs. See F-19.10.

### A-19.25 A television is laid out in a 1920×1080 design space
- **Portico:** Android TV's density is overridden process-wide and Vega's window is wrapped in a scaled canvas so that the tvOS numbers work unchanged.
- **Others:** native Android apps use dp at 960×540; Flutter uses logical pixels; Streamyfin scales in JavaScript.
- **Why it matters:** one set of TV numbers for every platform, which is why the port could share every screen. The Android method reaches into RN internals and changes density for everything in the process, not only RN (B-19.8).
- **Assessment: Trade-off.** Deliberate and documented (port log, decision 1 and question 1). I think it is the right call for parity; it needs a guard test at every RN upgrade.

### A-19.26 Lists: RN's built-in virtualisation plus a custom windowed grid
- **Portico:** `VirtualizedList` with tuned windows (`windowSize` 3 on TV), `removeClippedSubviews` off because TV focus needs neighbours, and a tvOS workaround that gives lists a measured height.
- **Streamyfin:** FlashList (recycles cells) on phones, and advises plain `ScrollView` for TV rows. **Swiftfin:** UICollectionView-backed grids. **androidtv:** RecyclerView/Leanback. **Plezy:** Flutter slivers.
- **Why it matters:** `VirtualizedList` creates and destroys React components as you scroll rather than recycling native cells; on weak sticks this is the usual source of dropped frames and blank flashes in RN TV apps.
- **Assessment: Unclear** without device measurement. Portico's windows are conservative and well reasoned. FlashList's recycling and TV focus have their own conflicts, so I would not switch blind.

### A-19.27 Random-access paging with placeholders
- **Portico:** the grid knows the total and can jump anywhere; pages are fetched by offset pinned to a revision, with eviction, and a `stale_continuation` restart.
- **Jellyfin clients:** `StartIndex`/`Limit` infinite scroll that grows a list in memory; letter jumps re-query. **Plezy:** paged loads into growing lists.
- **Why it matters:** memory stays flat at any library size and "jump to T" is instant — this is the client half of Portico's scale principle, and no compared client does it.
- **Assessment: Portico ahead.** Two limits to know: B-19.13 (row-height assumption and number precision at millions of rows).

### A-19.28 The Saved History tab and non-windowed screens
- **Portico:** six screens use `WindowedGrid`; seventeen use a plain scrolling `Page`; `Backlog.md` already lists Saved History as rendering its whole list.
- **Others:** uniformly paged lists.
- **Assessment: Portico behind** in that one known spot (already in the backlog).

### A-19.29 Images: the built-in `Image` with Portico's own scheduler
- **Portico:** no image library. A scheduler orders loads by screen position, limits them to six, times out and retries; the platform HTTP cache is the only disk cache (256 MB iOS, 128 MB tvOS, Fresco's defaults on Android — around 40 MB on most devices).
- **Streamyfin:** expo-image (SDWebImage/Glide underneath): memory + disk cache, blurhash placeholders, priorities, prefetch. **Swiftfin:** Nuke + blurhash. **androidtv/Wholphin:** Coil. **Plezy:** disk cache of 3,000 objects for 14 days, concurrency 6 or 3 by device tier.
- **Why it matters:** visual-order loading is something none of the libraries do, and it shows on a slow server. What Portico lacks: a guaranteed disk cache size on Android, decoded-size downsampling control, prefetch of the next row, and any colour or blur placeholder. The scheduler also keeps its own idea of what is "cached" that can disagree with the platform cache (B-19.5).
- **Assessment: Trade-off.** Keep the scheduler; consider putting a real image library *under* it (medium work, touches `Artwork.tsx` only). At minimum set Fresco's disk cache size explicitly so Android TV behaves like Apple TV.

### A-19.30 Image authorisation by header
- **Portico:** `Authorization: Bearer` on every image request; URLs carry no secret.
- **Plezy:** Plex puts the token in the URL, so Plezy redacts cache keys to keep tokens out of the on-disk cache index (`image_cache_service.dart:60-78`). **Jellyfin clients:** image endpoints are mostly unauthenticated.
- **Assessment: Portico ahead.** One consequence: every token renewal changes every image's request headers, so each mounted card re-renders (harmless, but work on a weak device).

### A-19.31 No device performance tiers
- **Portico:** the same effects, window sizes and concurrency on an Apple TV 4K and a 1 GB Fire TV Stick; only `Platform.isTV` varies them.
- **Plezy:** an automatic "reduced" tier (32-bit, low-RAM, ≤2.2 GB) that trims effects, halves image concurrency and caps GPU memory, with a user override.
- **Why it matters:** the docs say targets "span weak boxes"; the rail's animated push, focus halos with 28-px shadows, and six concurrent image decodes are exactly what a weak stick struggles with.
- **Assessment: Portico behind** (unmeasured). A tier flag in `theme.ts` fed by a native "low RAM / 32-bit" fact is small; deciding what it turns off needs a device.

### A-19.32 Animation engines
- **Portico:** Reanimated 4 worklets (UI thread) for focus scale, rail and sheets; core `Animated` with the native driver for image fades; a JavaScript-thread stand-in on Vega.
- **Others:** Flutter's own; SwiftUI; Compose; Streamyfin also Reanimated.
- **Assessment: Trade-off.** Sound on Apple/Android. On Vega every animation frame is JS work — the first thing to check on a Fire TV Stick.

### A-19.33 A design system that is enforced, and shared with the web
- **Portico:** only `src/ui` may use raw colours, `StyleSheet.create`, `Pressable` or `Text`; tokens and component measurements come from the `design` package the web also compiles.
- **Streamyfin:** Tailwind classes via nativewind, per-component. **Plezy:** Flutter themes plus a focus theme. **Swiftfin:** SwiftUI styles.
- **Assessment: Portico ahead.** This is why a TV port could reuse every screen.

### A-19.34 One dark theme
- **Portico:** fixed dark palette; no light mode, OLED black or accent choice.
- **Plezy:** theme provider with a colour picker and OLED mode. **Streamyfin:** accent colour derived from artwork. **Swiftfin:** system appearance and accent.
- **Assessment: Trade-off (subjective)** — a product decision outside this domain; architecturally the token layer would make it small.

### A-19.35 Fonts and icons are bundled
- **Portico:** Manrope (four files, copied under different names on Android at build time), icons as SVG paths through a slimmed `react-native-svg` (`metro/svg-lite.js`).
- **Swiftfin:** system font and SF Symbols. **Plezy:** Material Symbols font plus bundled CJK fallback fonts for Linux.
- **Why it matters:** identical look everywhere; but Manrope has no CJK, Arabic, Hebrew or Thai glyphs, so those titles fall back to the system font mid-line, and bundled fonts ignore the system "bold text" setting.
- **Assessment: Trade-off.** Relevant when languages are added.

### A-19.36 Accessibility lives in the primitives
- **Portico:** labels/roles in `Card`, `Button`, `ListRow`, etc.; Dynamic Type uncapped on handhelds, off on TV (in-app 1.2× instead); Reduce Motion honoured; the rail hides invisible labels from assistive technology. No `announceForAccessibility`, no check for a running screen reader, a few hard-coded English labels (`Artwork.tsx:61`).
- **Plezy:** an `assistive_technology_service` that changes behaviour when a screen reader is active. **Swiftfin:** SwiftUI's built-in accessibility. **androidtv:** TalkBack through Leanback defaults.
- **Why it matters:** VoiceOver on tvOS drives the same focus engine, so Portico inherits correct navigation; what is missing is spoken feedback for changes that are only visual (toasts use a live region; loading states and focus-claim jumps do not).
- **Assessment: Trade-off** — better than most RN apps, behind fully native ones. Auditor for accessibility will go deeper.

### A-19.37 Internationalisation: own formatter, English only
- **Portico:** shared catalogue (about 1,730 strings) and a hand-written ICU MessageFormat; regional English derived by rule; a lint with three exceptions. Plurals use `Intl.PluralRules` with an English fallback.
- **Plezy:** 22 languages via slang. **Streamyfin:** i18next + Crowdin. **Jellyfin:** Weblate. **Swiftfin:** generated strings from translation files.
- **Assessment: Portico behind** on languages (a recorded launch decision), **ahead** on discipline. Three things to settle before adding a language: no translation workflow, no right-to-left handling (styles use `left`/`marginLeft`; `supportsRtl="true"` is set in the Android manifest anyway), and whether Hermes provides `Intl.PluralRules` on every platform (B-19.17).

### A-19.38 Errors: two boundaries, catalogue words only, nothing recorded
- **Portico:** a per-screen error boundary and a root one; the screen never shows a raw error; details go to `console.error`. No global JavaScript error handler in `native/` (Vega's debug file installs one), no native crash handler, no crash upload.
- **Plezy / Streamyfin:** Sentry with symbol upload. **androidtv / Wholphin:** ACRA; androidtv posts the report to the user's own server. **Swiftfin:** Pulse log store.
- **Why it matters:** when the app crashes on someone's Fire TV there is no trace anywhere — not on the device, not on their server, not with Portico.
- **Assessment: Portico behind.** The androidtv model (crash and client logs go to *the owner's server*, never to a third party) fits Portico's principles exactly. See F-19.1.

### A-19.39 No log the user can see or send
- **Portico:** a 12-line in-memory trace in development builds only (`src/app/debug.ts`).
- **Plezy:** an in-app log viewer and "upload logs, get a Log ID". **Swiftfin:** Pulse. **Streamyfin:** a log file with export. **androidtv:** Timber plus server-side client logs.
- **Assessment: Portico behind.** See F-19.2.

### A-19.40 Background work
- **Portico:** iOS background `URLSession` for downloads; an Android foreground service for a running download (no WorkManager, by a written decision); a `JobService` that refreshes TV home-screen rows every six hours; no background fetch.
- **Others:** WorkManager (androidtv, Wholphin), expo-background-task and a Live Activity for downloads (Streamyfin), a forked `background_downloader` with resume fixes (Plezy).
- **Assessment: Trade-off**; details belong to the downloads auditor. Note the six-hour job is periodic device-to-server chatter that exists only on Android TV.

### A-19.41 Shared devices: an on-device PIN vault, no system user awareness
- **Portico:** a profile "trust vault" (PBKDF2 200,000 rounds, attempt counters charged before decryption, lockouts) implemented three times; one signed-in viewer per installation.
- **Streamyfin:** tvOS user-management entitlement, so each Apple TV user has separate app data. **Plezy:** profiles and connections in its database with a profile picker. **androidtv:** multiple servers and users with auto-login preferences.
- **Assessment: Portico ahead** on PIN handling; **behind** on Apple TV multi-user (F-19.7).

### A-19.42 Platform coverage

| Platform | Portico | Plezy | Swiftfin | jellyfin-androidtv / -android | Streamyfin | Wholphin |
|---|---|---|---|---|---|---|
| iPhone | Yes | Yes | Yes | — | Yes | — |
| iPad (multitasking) | Yes; multitasking unverified (B-19.7) | Yes | Yes | — | Yes | — |
| Apple TV | Yes (reference) | Yes (custom engine) | Yes | — | Yes | — |
| Android phone/tablet | New, unvalidated | Yes | — | Yes (WebView) | Yes | — |
| Android TV | New, unvalidated | Yes | — | Yes | Yes | Yes |
| Fire TV (Fire OS) | New, unvalidated | Yes | — | Yes | Yes | Yes |
| Fire TV (Vega OS) | New, unvalidated | No | No | No | No | No |
| Roku | Separate project, out of scope | No | No | Separate official app | No | No |
| webOS / Tizen / Xbox | No | No | No | Separate official apps | No | No |
| macOS / Windows / Linux | No (companion planned) | Yes | Mac via iPad app | Separate desktop app | No | No |
| visionOS | No | No | Compatible iPad app | — | No | — |
| CarPlay / Android Auto | No | Android Automotive restrictions only | No | Android Auto (phone app) | No | No |
| Watch / widgets | No | No | No | No | Download Live Activity | No |

- **Assessment:** Portico's reach from one screen codebase is second only to Plezy's, and it is the only one on Vega. Everything except Apple is days old and unproven on hardware.

### A-19.43 Build and release pipeline
- **Portico:** CI type-checks, lints strings, runs Node tests and assembles Android debug builds. No Apple build, no signing, no store upload, no release workflow, no update channel.
- **Plezy:** a protected-branch release workflow producing signed, attested builds for every platform, symbol upload, notarisation, auto-update feeds. **Streamyfin:** EAS profiles for phone and TV. **Swiftfin, androidtv:** fastlane and store workflows.
- **Assessment: Portico behind**, as expected before a first release. The first missing piece is a macOS runner job that at least compiles the iOS and tvOS targets: today a Swift or Objective-C error is found only when someone opens Xcode, and the Android port changed shared files without the Apple apps being rebuilt (stated in the port log).

### A-19.44 Android packaging
- **Portico:** R8 off (`enableProguardInReleaseBuilds = false`), all four ABIs in one APK, release signed with the debug key, no baseline profile. The port log's 36.9 MB release figure is for arm64 only.
- **androidtv / Wholphin:** R8 on; Wholphin adds ABI splits and baseline profiles. **Plezy:** split-per-ABI, obfuscation, symbol maps.
- **Why it matters:** without shrinking, the whole of BouncyCastle (several megabytes of bytecode, used for three primitives) and all of Media3 ship; four ABIs roughly triple the native libraries; no baseline profile means slower first launches on weak sticks, which is exactly where Hermes + RN start-up is felt.
- **Assessment: Portico behind** (small to medium; R8 needs keep rules for RN and the modules).

### A-19.45 Testing: logic under Node, nothing rendered
- **Portico:** about 240 Node tests over real bridge and hook code with a stub `react-native`; a custom hook driver that plugs into React's private dispatcher; two native test files (`test/porticourlroutes-tests.swift`, `test/native-download-attempt-tests.m`) that no workflow runs.
- **Streamyfin:** Jest with React Native Testing Library-style hook tests. **Plezy:** 664 test files, widget tests, Maestro flows, perf benches. **androidtv:** Kotest + MockK. **Wholphin:** Robolectric.
- **Why it matters:** Portico's tests are fast and test real ownership/fencing logic, but nothing ever mounts a component, so focus wiring, list windows, navigation and the theme are unprotected. The hook driver depends on a React internal whose name says not to (B-19.15).
- **Assessment: Portico behind** on breadth, **ahead** on speed and on testing the shared core once for all clients.

### A-19.46 How television behaviour is proven
- **Portico:** by hand, on emulators, recorded as screenshots in the port logs; no physical device yet.
- **Plezy:** Maestro flows in CI, a check that tvOS tests are wired, and measured tuning on an Apple TV 4K. **Streamyfin:** written focus rules. **androidtv:** years of field use.
- **Assessment: Portico behind.** The docs say acceptance on physical devices is not established; that is the largest open item in this domain.

### A-19.47 Minimum OS versions
- **Portico:** iOS/tvOS 15.1; Android 7 (API 24), Fire OS flavor API 25; Vega OS 1.2.
- **Swiftfin:** iOS 18.6, tvOS 26.1. **androidtv, Wholphin, jellyfin-android:** API 23. **Streamyfin:** Expo's floor (iOS 15.1+).
- **Why it matters:** Portico reaches the Apple TV HD (2015) and old iPads that Swiftfin has dropped — good for a server owner's family hardware — at the cost of testing old OS behaviour and forgoing newer APIs.
- **Assessment: Portico ahead** on reach. API 24 vs 23 excludes almost nothing.

### A-19.48 No over-the-air JavaScript updates
- **Portico:** the bundle is embedded; changes ship as store builds.
- **Others:** none of the compared apps use CodePush/EAS Update either; Plezy has a desktop auto-updater and an in-app update notice.
- **Assessment: Parity.** For a client that must match a self-hosted server's version, a simple "this server needs a newer app" notice matters more than OTA (see F-19.14).

### A-19.49 Polyfills are few and explicit
- **Portico:** `TextDecoder`, `URL`, and `crypto.randomUUID` installed from the native UUID source (`src/bridge/random-id.ts`, added during the Android port after Downloads failed without it).
- **Assessment: Portico ahead** on hygiene. The port log's question 11 (was Download working on iPhone before this?) deserves an answer, because it implies a shared service was never exercised on a device.

### A-19.50 Server events over `XMLHttpRequest`
- **Portico:** SSE read incrementally through XHR, restarted every 1 MB.
- **Jellyfin clients:** a WebSocket. **Plezy:** WebSocket/long-poll per backend.
- **Assessment: Trade-off.** Works without a dependency; depends on RN delivering partial XHR text on every platform (confirmed on Android in the port log, done differently on Vega).

### A-19.51 Home-screen integration is written natively per platform
- **Portico:** tvOS Top Shelf extension reading a token from the app group; Android TV Watch Next and one channel through `TvContract` with no support library; nothing on Fire OS; a record-only stub on Vega.
- **androidtv:** tvprovider + WorkManager, plus global search. **Streamyfin:** Top Shelf target and a `tv-recommendations` module. **Plezy:** `system_shelf_service`.
- **Assessment: Parity**, minus Android TV global search (F-19.8).

### A-19.52 Apple project hygiene
- **Portico:** no scene manifest (`UIApplicationSceneManifest`) in either `Info.plist`; iPad lists three orientations; no development team or signing style in the projects; ATS allows local networking only.
- **Swiftfin:** SwiftUI app lifecycle (scenes by construction), all orientations.
- **Assessment: Unclear** — two items here may block a store build (B-19.7, B-19.18).

### A-19.53 The desktop question
- **Portico:** a per-user desktop companion for macOS and Windows is planned (`Backlog.md`), and the web app already runs in any desktop browser.
- **Plezy:** ships macOS, Windows and Linux from the same Flutter code with mpv, window management, keyboard shortcuts and auto-update. **Jellyfin:** a separate mpv-based desktop project.
- **What Plezy's desktop support implies:** a real desktop player is mostly *not* UI — it is a native video pipeline (mpv), window and fullscreen handling, media keys, display-mode switching, updates and installers per OS. Plezy carries a patched Flutter engine on Windows and vendored fixes for two Linux/Windows preference plugins to get there. Portico's RN screens would not carry over cheaply (React Native for macOS/Windows are different forks again, with no TV-fork features), whereas the web app plus `client-core` already cover desktop UI.
- **Assessment: Trade-off.** If the companion's job is "tray app that helps the server and opens the web app", the current plan is cheap and sound. If it is meant to be a player that beats the browser (HDR, passthrough audio, any codec), the realistic route is a thin native shell around mpv driven by `client-core`, not a port of the RN app.

### A-19.54 Android edge-to-edge and the keyboard
- **Portico:** edge-to-edge on every Android version, so the window never resizes for the keyboard and forms pad themselves (`theme.ts`, `keyboardAvoidance`); the manifest still says `adjustResize`.
- **Others:** per-screen handling.
- **Assessment: Parity**; the manifest attribute is now misleading (see Doc/code disagreements).

### A-19.55 Orientation
- **Portico:** iPhone portrait and both landscapes; iPad the same three (no upside-down).
- **Swiftfin, Plezy:** all four on iPad.
- **Assessment: Portico behind** by one line of the plist (B-19.7).

## Missing features and capabilities to consider

Sizes are rough: **small** = days, **medium** = a week or two, **large** = more.

### F-19.1 Crash and client-error reports sent to the owner's own server
- **What:** catch fatal JavaScript errors and native crashes, keep the report on the device, and post it to the signed-in server on next launch, where the owner sees it in Troubleshooting.
- **Who has it:** jellyfin-androidtv (ACRA → the Jellyfin server's client-log endpoint, behind a preference; `TelemetryService.kt`), Wholphin (ACRA), Plezy and Streamyfin (Sentry, a third party).
- **Value:** today a crash on a relative's Fire TV leaves no evidence anywhere. **Size:** medium (a global JS handler, a native handler per platform, one server route — Portico already has "Report a problem"). **Principles:** fits "hosted stays cheap" and "no third parties" if it goes only to the owner's server. **Opinion:** the most valuable item in this list before a public release.

### F-19.2 A log the person can see, copy and attach
- **What:** a bounded in-memory/on-disk log of app events, visible in Settings and attachable to a problem report.
- **Who:** Plezy (viewer + "upload, get a Log ID", `log_upload_service.dart`), Swiftfin (Pulse, including network requests), Streamyfin (log file export), androidtv (Timber + ACRA attaches 250 log lines).
- **Size:** small to medium. **Opinion:** do it with F-19.1; the two share plumbing.

### F-19.3 Last-known Home and library pages kept on the device
- **What:** persist the last validated document per viewer and show it immediately at launch, replacing it when the server's revision differs.
- **Who:** Streamyfin (24-hour persisted query cache), Plezy (API cache table).
- **Value:** instant first paint; something to look at when the server is asleep or the link is slow. **Size:** medium. **Principles:** allowed if keyed by revision and viewer, never by time; needs a decision on whether showing possibly-stale rows for a second is acceptable under "Fresh". **Opinion:** worth it for television start-up; decide after measuring cold start on a stick.

### F-19.4 Colour or blur placeholders for artwork
- **Who:** Streamyfin and Swiftfin (blurhash from Jellyfin's API), Plezy.
- **Size:** small on the client once the server publishes a dominant colour (already in `Backlog.md` as a server item). **Opinion:** the title-in-type placeholder is distinctive and good; a server-published tint behind it would be enough. Blurhash decoding on the JS thread is not worth it.

### F-19.5 A reduced-effects tier for weak devices
- **Who:** Plezy (`device_performance.dart`).
- **What it would touch:** `theme.ts` (a tier flag), `ArtworkPipeline.ts` (concurrency), `FocusHalo`/rail animation, list window sizes; a native fact for "low RAM / 32-bit". **Size:** small to build, needs devices to tune. **Opinion:** build the switch now, tune later.

### F-19.6 A "use the TV layout" override
- **Who:** Plezy (force-TV setting plus multiple detection signals).
- **Size:** small, but the Android density override must follow the same decision at process start. **Opinion:** cheap insurance for odd Android boxes.

### F-19.7 Apple TV multi-user support
- **What:** the `com.apple.developer.user-management` entitlement (`runs-as-current-user`) so each tvOS user has their own app data and sign-in.
- **Who:** Streamyfin (`plugins/withTVUserManagement.ts`, `modules/tv-user-profile`).
- **Value:** on a family Apple TV, switching the system user switches the Portico profile with no PIN screen. **Size:** small (an entitlement and a provisioning change; storage is already per-app). **Conflict:** interacts with Portico's own profile switcher and the Top Shelf app group. **Opinion:** worth a decision; it is the platform's intended answer to shared TVs.

### F-19.8 Android TV global search and voice
- **What:** a search content provider so the launcher's search and Assistant find Portico titles.
- **Who:** jellyfin-androidtv (`integration/MediaContentProvider.kt`, search intents in its manifest); Streamyfin has a `tv-search` module.
- **Size:** medium (provider, a bounded search call, deep link — which first needs B-19.1 fixed). **Opinion:** nice to have after launch.

### F-19.9 A screensaver
- **Who:** jellyfin-androidtv (a system `DreamService` plus an in-app screensaver showing library art).
- **Size:** small to medium. **Opinion:** low priority; tvOS has its own.

### F-19.10 Keyboard shortcuts and controller mapping
- **Who:** Plezy (configurable shortcuts, gamepad service).
- **Value:** iPad with a keyboard (space, arrows, F for full screen), Android TV with a gamepad or keyboard. **Size:** small for a fixed set in the player. **Opinion:** add a fixed set for the player only.

### F-19.11 Shrinking, ABI splits and baseline profiles on Android
- **Who:** Wholphin (R8, splits, `profileinstaller`), androidtv (R8), Plezy (split-per-ABI).
- **Value:** smaller downloads and faster cold start on the weakest hardware. **Size:** small to medium. **Opinion:** required before a store release anyway.

### F-19.12 Automated television journeys and component tests
- **Who:** Plezy (Maestro flows against a real server, run in CI), Streamyfin (Jest component/hook tests).
- **What for Portico:** a handful of Maestro (or equivalent) flows on the tvOS simulator and an Android TV emulator: sign in, move through the rail, open a title, Back to the same card, play. **Size:** medium to set up, small per journey. **Opinion:** the focus design relies on timers and platform quirks (A-19.21); without automation every RN upgrade is a manual walk of three platforms.

### F-19.13 Apple builds and the existing native tests in CI
- **What:** a macOS job that builds `ios` and `tvos` and runs `porticourlroutes-tests.swift` and `native-download-attempt-tests.m`.
- **Size:** small. **Opinion:** do first; it is the cheapest protection against shared-code changes breaking Apple unnoticed.

### F-19.14 "This server needs a newer app" and update notices
- **Who:** Plezy (`update_service.dart`, desktop auto-update), Swiftfin and Streamyfin check server versions (`useServerVersion.ts`).
- **Value:** self-hosted servers and store-delivered apps drift apart; today an incompatible pair shows a generic failure (`portico-failure.ts` has an `incompatible` case for sign-in only). **Size:** small. **Opinion:** worth having at launch.

### F-19.15 Mac and Vision Pro availability of the iPad app
- **What:** allow the iPad build to run as "Designed for iPad" on Apple-silicon Macs and visionOS.
- **Who:** Swiftfin is usable this way. **Size:** small (store setting plus a pass over window resizing and keyboard/pointer). **Opinion:** a cheap desktop presence on Mac that needs none of the planned companion; test resizing first.

### F-19.16 Android Auto and CarPlay for music and audiobooks
- **Who:** jellyfin-android (Android Auto), Plezy (automotive driver-distraction rules).
- **State in Portico:** Android already runs a Media3 `MediaSessionService`; browsing in a car needs a `MediaLibraryService` and a browse tree. CarPlay needs an entitlement from Apple. **Size:** medium each. **Opinion:** later; audiobooks make it more relevant than for a video-only product.

### F-19.17 Typed module specs and shared conformance vectors
- **What:** one TypeScript spec per native module (TurboModule codegen) and one set of test vectors (vault envelopes, TV grant, transport refusals) run against the Swift, Kotlin and Vega-JS implementations.
- **Size:** medium. **Opinion:** the vectors are the valuable half and can come first.

### F-19.18 Restore the last screen after the system kills the app
- **Size:** small with React Navigation's state persistence, plus care for vanished items. **Opinion:** worthwhile on Fire TV, where process death is routine.

### F-19.19 A translation workflow and right-to-left readiness
- **Who:** Crowdin (Streamyfin), Weblate (Jellyfin), slang with 22 locales (Plezy).
- **Opinion:** not needed for an English launch; decide the workflow before the catalogue grows further, and replace `left`/`right` style properties with `start`/`end` in `src/ui` while the surface is small.

### F-19.20 Freeze or cap covered screens
- **What:** stop covered stack screens and hidden roots from rendering (react-freeze through `react-native-screens`), and cap stack depth.
- **Size:** small. **Opinion:** see B-19.14; test with TV focus guides before enabling.

### F-19.21 An explicit Android image disk cache and next-row prefetch
- **What:** configure Fresco's disk cache to match Apple's 128–256 MB; prefetch the first images of the row below the viewport.
- **Size:** small. **Opinion:** do the cache size now; prefetch after measuring.

### F-19.22 Smaller platform extras seen elsewhere
- A download **Live Activity** on iPhone (Streamyfin, `targets/StreamyfinDownloadActivity`); **settings export/import** (Plezy); an **in-app network inspector** for support (Swiftfin's Pulse); **Discord presence** (Plezy). Recorded for completeness; I would not prioritise any of them.

## Possible bugs and risks in Portico

None of these is in `Backlog.md` unless stated. "Confirmed by reading" means the code path is unambiguous in the source; none was reproduced.

### B-19.1 A link that launches the app from cold is thrown away
- **What goes wrong:** selecting a Top Shelf card, an Android TV Watch Next card, a Spotlight or Siri result, a universal link or `portico://device` **while Portico is not running** opens the app at Home and nothing else happens.
- **Evidence:** the shell reads the launch URL once, on mount (`native/src/shell/Shell.tsx:293-297`). The handler returns at once unless the API has a real origin (`Shell.tsx:259`, `hasRealOrigin`). At a cold start with a remembered sign-in the shell mounts immediately with the **placeholder** API (`native/src/app/session.tsx:311-316`, `217-219`, phase at `1099`), and the real API is only set after the restore's network round trips (`session.tsx:456-461`). Nothing replays the URL later. On iOS, Handoff/Spotlight/Siri/universal links are re-sent after a fixed two-second delay (`native/ios/Portico/PorticoIntents.swift:13-19`), which only helps if the restore finishes inside two seconds.
- **How the comparison surfaced it:** expo-router (Streamyfin) queues the initial URL inside the router; Portico's hand-written router is tied to shell state.
- **Confidence:** Confirmed by reading. The port log's Watch Next check used `adb … am start` with the app already running, which takes the other path (`onNewIntent`). **Severity:** Medium (home-screen cards are a primary TV entry point; Fire and Android TV kill apps often).

### B-19.2 Every access-token renewal rebuilds the data services of every mounted screen
- **What goes wrong:** about every 13 minutes in the foreground (15-minute tokens renewed 120 s early, `core/session-refresh.ts:76`), each mounted Library Discover tab, entity page, title page, show page, Search, Saved and person page throws away its service and loads again. On screen that is a return to the loading state; on television the focused card unmounts and focus moves.
- **Evidence:** renewal publishes a **new session object** for the same viewer (`core/index.ts:535-541`, comment: "no new generation, so nothing viewer-scoped restarts"). The hooks key their scope on the object's identity: `useMemo(() => viewerScope(session, api), [session, api])` then `useMemo(() => new LibraryContentService(…), [api, scope])` (`native/src/app/content.ts:17-18`; same pattern in `detail.ts:15-16` and `:45`, `section-window.ts:13-19`, `screens/Search.tsx:39`, `Saved.tsx:51`, `Person.tsx:80` and others). `viewerScope` returns a fresh object each time, so the service is recreated, the old one disposed, the snapshot reset to `idle` (`core/library-content.ts:279`) and the route selected again. The main browse grid is unaffected (`browse.ts` keys on `api` only).
- **Why it matters beyond the flash:** all seven kept-alive roots and every stacked page refetch at the same moment.
- **Confidence:** Confirmed by reading for the re-creation; the visible effect is Probable (needs one 15-minute session on a simulator). **Severity:** Medium. **Fix shape:** key on `scope.viewerId` and `scope.serverId` (strings), or have `viewerScope` return a cached object per value.

### B-19.3 On Android TV and Fire TV the remote's Menu key presses the focused control
- **What goes wrong:** Menu is re-sent as Select-down, then Select-up 800 ms later (`native/android/app/src/main/java/tv/getportico/portico/MainActivity.kt`, `dispatchKeyEvent`). react-native-tvos turns key-down into a 500 ms long-press timer and, on key-up, calls `onPress` **unless a long-press handler exists and fired** (`node_modules/react-native/Libraries/Pressability/Pressability.js:442-487`). `Focusable` passes `onLongPress` through only when the caller gave one. So on every control without a long-press action — Sign out, Delete, Play, a settings row — pressing Menu activates it after 0.8 s.
- **Related edge:** if focus moves during the 0.8 s, the original control never receives its key-up and its long-press timer still fires.
- **How the comparison surfaced it:** Plezy and jellyfin-androidtv treat Menu as its own key (context menu), never as a synthetic Select.
- **Confidence:** Confirmed by reading (both files). The port log tested Menu only on a card, which has a long-press action. **Severity:** Medium (unexpected activation of destructive buttons). **Fix shape:** deliver Menu as its own TV event and let `Focusable` map it to `onLongPress` when present, otherwise ignore it.

### B-19.4 If the Android Keystore key is lost, the app can never read or save most of its records again
- **What goes wrong:** every record file is sealed by one Keystore key. If the key disappears while the files remain (a known field failure on some devices after OS updates or Keystore resets), `read` returns "unreadable" for every record (`PorticoRecordStore.kt`, `read` → `key(create = false)`), and "unreadable" is never repaired by design (`PorticoRuntime.kt:149-160`). A plain save still works (it creates a new key), so sign-in succeeds. But every read-modify-write record reads first and aborts on failure (`native/src/bridge/queue-storage.ts`): device preferences, known servers, last server, last sign-in and the queue id can never be written again. The installation id is also unreadable and is not regenerated (`PorticoProfileTrust.kt:47-58`), so `installationReady` gives up (`session.tsx:43-49`, "the session just can't be renewed") and the person is asked to sign in again roughly every 15 minutes, indefinitely, with no message that says why. Only clearing the app's data fixes it.
- **Evidence:** as cited. The same "never repair" rule on iOS is safer because Keychain items do not depend on a second key the app holds.
- **How the comparison surfaced it:** Plezy carries explicit recovery code for corrupted local stores (`lib/services/prefs_recovery.dart`, `lib/database/tvos_database_recovery_store.dart`).
- **Confidence:** Confirmed by reading for the logic; how often the trigger happens is Speculative. **Severity:** Medium (rare, but permanent and invisible). **Fix shape:** when the key is missing and record files exist, treat the store as lost: clear it, start fresh and say so once.

### B-19.5 An image that has loaded once is never retried and gets no placeholder if it later fails
- **What goes wrong:** `ArtworkPipeline` remembers every image that has ever decoded in a module-level `Set` (`native/src/ui/ArtworkPipeline.ts:29`). For those, `Artwork` starts with the placeholder hidden and full opacity (`native/src/ui/Artwork.tsx:46-53`) and skips the scheduler entirely (`ArtworkPipeline.ts:123, 138`). If the platform cache has since evicted the bytes and the reload fails (server asleep, offline), `failed()` finds no attempt to settle (`ArtworkPipeline.ts:162`), so the card stays an empty tile with no title, no retry and no "artwork unavailable" label for the rest of the session.
- **Also:** the set is never trimmed, although the scheduler's own cache is capped at 800 (`ArtworkPipeline.ts:62-65`) and the retry counter at 2,000 (`:41`) — slow memory growth over a long browsing session. And a retry that succeeds is cached by the platform under the retry URL (`?r=1`, `Artwork.tsx:20-23`), so the next mount asks for the plain URL and downloads it again.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### B-19.6 An uncaught JavaScript error outside rendering ends the app with no record
- **What goes wrong:** the two error boundaries catch render errors only. An exception in an event handler, a timer or a native-event callback is fatal in a release build of React Native, and `native/index.js` / `App.tsx` install no global handler (only `vega/src/debug.tsx:89-90` does). The app closes to the home screen and nothing is written anywhere.
- **Confidence:** Confirmed by reading (absence of a handler). **Severity:** Medium as a risk multiplier: it turns any small bug into an unexplained crash. See F-19.1.

### B-19.7 The iPad build lists three orientations, which App Store validation normally rejects for a multitasking app
- **Evidence:** `native/ios/Portico/Info.plist:89-95` (`UISupportedInterfaceOrientations~ipad`: portrait, landscape left, landscape right) and no `UIRequiresFullScreen`. Apple's long-standing upload check (ITMS-90474) requires all four orientations on iPad unless the app opts out of multitasking.
- **Confidence:** Probable (not uploaded). **Severity:** Medium only because it blocks an upload; the fix is one line.

### B-19.8 The Android TV density override has gaps and wide side effects
- **Gap:** React Native re-reads display metrics from the system, unconditionally, when the display rotation is not the one it last saw (`node_modules/react-native/ReactAndroid/src/main/java/com/facebook/react/ReactRootView.java:975-985`). That path is not covered by the one flag `TelevisionDisplay.keep` turns off, and `PixelUtil` converts with the *screen* density it overwrites. On a television or projector that reports a non-zero rotation, the layout would return to double size.
- **Side effects by design:** the override changes density for the whole process, not only RN: anything native drawn from the activity's context measures in the design space (the native subtitle view, the search `EditText`, any dialog), while `Configuration.screenWidthDp` still says 960 — two facts that no longer agree for any library that multiplies them. Font-scale changes are ignored until restart (documented).
- **Fragility:** `ReactNativeFeatureFlags.dangerouslyForceOverride` and `ReactNativeFeatureFlagsOverrides_RNOSS_Stable_Android` are internal. Today the replacement set equals what RN loads for the stable channel (`DefaultNewArchitectureEntryPoint.kt`, `ReleaseLevel.STABLE`), so nothing else changes — but only by coincidence of version.
- **Confidence:** the rotation gap is Speculative (needs a device that reports rotation); the rest Confirmed by reading. **Severity:** Low. The port log's question 1 already asks Justin whether to keep this dependency; it is not in `Backlog.md`.

### B-19.9 On Apple, storage, PIN hashing and request submission share one queue
- **What:** `PorticoRuntime` declares no `methodQueue` (`native/Native/PorticoPlayer.m:193-199`), so React Native runs all its asynchronous methods on one serial queue: Keychain reads and writes, the 200,000-round PBKDF2 of a PIN unlock (`:284-294`), and the hand-off of every API request (`:229-246`). While a PIN is checked or a Keychain call stalls, no new request starts. Android deliberately moved storage to its own thread for this reason (port log, phase 0 decision 5: "a PIN check must not hold up the player").
- **Confidence:** Probable (queue behaviour of interop modules not verified at run time). **Severity:** Low. It is a same-name, different-behaviour divergence between the platforms.

### B-19.10 A television that does not report "television" gets a touch layout with no visible focus
- **What:** see A-19.3. `Platform.isTV` false → `focusRing: 0`, no rail focus guides, the bottom tab bar, downloads module instead of home-screen rows, and no density override.
- **Confidence:** Confirmed by reading; which devices do this is known in general (generic Android boxes) but not tested. **Severity:** Low to Medium depending on how many owners use such boxes.

### B-19.11 Vega keeps sign-in records as plain files
- **What:** Vega has no keychain; credentials and the device key are ordinary files in the app's private storage (`vega/src/bridge/storage.ts`; port log item 9). The PIN vault's protection there rests on the PIN alone plus a device key stored beside it.
- **Confidence:** Confirmed by reading and by the port log. **Severity:** Low (sandboxed consumer device). Recorded because `06 Clients.md` does not mention Vega at all.

### B-19.12 On Apple, deleting the app does not sign it out
- **What:** Keychain items outlive the app on iOS and tvOS, and Portico never clears them on first launch. After delete-and-reinstall the app opens signed in as the previous viewer, with the old installation id, PIN vault and preferences. The "quarantined" marker that protects against an unverified credential is in `NSUserDefaults` (`native/Native/PorticoPlayer.m:300-312`), which *is* deleted with the app — so a credential that was quarantined before deletion is trusted again after reinstall. Android behaves the opposite way (everything is removed).
- **Confidence:** Confirmed by reading (standard platform behaviour; not exercised). **Severity:** Low to Medium (privacy on a device that changes hands; a surprising platform difference). **Fix shape:** a first-launch marker in `UserDefaults`; when absent, clear Portico's Keychain services.

### B-19.13 The windowed grid assumes equal row heights and exact pixel arithmetic at any size
- **Row height:** `WindowedGrid` measures row 0 once and uses that height for every row's position (`native/src/ui/Grid.tsx:129, 152-155, 171`). `Card` lets a title take two lines for books (`Card.tsx`, `titleLines`) inside a caption block that only has a *minimum* height, so a row with a wrapped title can be taller than row 0. Each such row shifts everything below it from where the list believes it is, which shows as letter jumps landing short and blank bands while scrolling far.
- **Precision:** list offsets pass through React Native's layout engine as 32-bit floats, which are exact only up to about 16.7 million. A television row is roughly 440 units tall, so beyond roughly 38,000 rows (about 230,000 titles at six across) positions start rounding to 2 units, and at five million titles to tens of units. Native grids (RecyclerView, UICollectionView) and Flutter do not position by one giant offset and have no such limit.
- **Confidence:** Speculative for both (needs a library with long titles, and a very large library, on a device). **Severity:** Low today; Medium against the stated 5–10 million goal. Not in `Backlog.md`.

### B-19.14 Mounted screens are never released or frozen
- **What:** seven roots stay mounted once visited (`native/src/shell/Roots.tsx`), each stack keeps every pushed page mounted, there is no depth limit (title → person → title → …), and screens are not frozen while covered (no `enableFreeze`). Each keeps its services, subscriptions and decoded images, and re-renders on session changes (B-19.2).
- **Confidence:** Confirmed by reading; impact on memory is unmeasured. **Severity:** Low on Apple TV, potentially Medium on 1–1.5 GB sticks, where Android will kill the app (and B-19.1 then applies on the way back in).

### B-19.15 The test harness leans on a React internal, and native tests are not run
- **What:** `native/test/hook-driver.ts:25-29` installs itself through `__CLIENT_INTERNALS_DO_NOT_USE_OR_WARN_USERS_THEY_CANNOT_UPGRADE`; a React upgrade can silently change hook semantics the driver imitates. `test/porticourlroutes-tests.swift` and `test/native-download-attempt-tests.m` exist but no script or workflow runs them. ESLint is configured (`npm run lint`) but not in CI; nothing in CI touches `vega/`.
- **Confidence:** Confirmed by reading. **Severity:** Low (process risk).

### B-19.16 User-visible English outside the catalogue
- **Evidence:** `native/src/app/session.tsx:325` ("Saved server credentials could not be read. Sign in again."), the `ShownError` messages at `session.tsx:367, 402-409`, filter and sort names in `native/src/app/browse.ts` (`fieldNames`, `quickFilterLabel`), and the accessibility label in `native/src/ui/Artwork.tsx:61`. The string lint passes because these are not JSX text.
- **Confidence:** Confirmed by reading. **Severity:** Low while the app is English-only; they also bypass the regional-spelling rule today ("Favorites").

### B-19.17 Plural rules may silently fall back to English on devices
- **What:** `packages/i18n/src/messageformat.ts:111-121` uses `Intl.PluralRules` and falls back to "one/other" when it is missing. Hermes has historically lacked `Intl.PluralRules`. If it is missing on any of Hermes-iOS, Hermes-Android or Vega, a future Polish, Russian or Arabic catalogue would pluralise correctly on the web and wrongly in the apps, with no error.
- **Confidence:** Speculative (depends on the Hermes build; not checked). **Severity:** Low now, Medium when the first non-English language ships.

### B-19.18 No scene manifest in the Apple apps
- **What:** neither `Info.plist` declares `UIApplicationSceneManifest`; both apps use the classic app-delegate window. Apple has announced that UIKit apps built with the SDK after iOS/tvOS 26 must adopt the scene life cycle. The Podfiles already mention Xcode 27.
- **Confidence:** Speculative — I could not verify what the current SDK enforces or what react-native-tvos 0.83 does about it, and the Apple apps were not built in the last port session. **Severity:** High if it applies (the app would not launch when built with that SDK), nil if not. One launch from an Xcode 27 build settles it.

### B-19.19 Small contract leftovers
- `pictureInPictureSupported` is exported by the iOS runtime only (`PorticoPlayer.m:203`) and read by nothing; JavaScript decides Picture in Picture by `!Platform.isTV` (`device-platform.ts`), so a handheld without PiP offers it and then reports "unavailable".
- `PorticoSearchKeyboard` is implemented on both platforms and mounted by no screen (port log).
- The Android manifest still declares `adjustResize` although edge-to-edge makes it inert.
- **Confidence:** Confirmed by reading. **Severity:** Low.

## Where Portico is stronger

- **Real platform focus on Apple TV and Android TV.** Plezy had to measure and re-create Siri Remote behaviour; Portico gets it from the system.
- **A shared headless core with the web app.** Paging, fencing, validation, presentation rules and strings are written once. No compared project shares logic between its web and native clients.
- **Thirteen dependencies.** The smallest supply chain of any client compared, by a wide margin.
- **Random-access, bounded-memory browsing** with jump-to-letter, backed by revision-pinned pages. Competitors grow lists in memory.
- **A bounded, cookie-less, redirect-refusing transport** and bearer-header image requests. Nobody else caps what a server can send.
- **Storage discipline:** read-back verification, missing-versus-unreadable kept distinct, nothing in backups, a PIN vault that charges attempts before decrypting.
- **An enforced design system** shared with the web, which is what made "every screen shared" true across four native shells.
- **Kept-alive destinations:** returning to a tab shows it as left, without a refetch.
- **Per-screen error containment** with catalogue-only wording.
- **Reach:** the only client here on Vega OS, and it still supports the 2015 Apple TV.
- **Honest records:** the port logs state exactly what was and was not seen on a device, which made this audit much easier.

## Doc/code disagreements

- **`06 Clients.md` opens with "Two clients ship: the web app and the Apple app"** and has no Vega section; the repository README and code say six native targets plus Vega. `01 Overview.md` likewise describes only Apple.
- **`06 Clients.md` says the Android work is "on branch `android-port` (not merged)"**; the checked-out branch (`codex/android-testing-performance`) and `main` contain it, with CI building it.
- **`Backlog.md` (Clients): "`src/app/compat.ts` can go once client-core covers the newer fields."** The file is already gone; the compatibility layer now lives in client-core (`core/presentation/content-compat.ts`).
- **`06 Clients.md` Apple build line** says `npm install --legacy-peer-deps`; the README and CI use `npm ci`.
- **`native/src/bridge/README.md`** says the files "were carried over unchanged from the original host"; several were changed for Android (the README itself lists them further down).
- **`core/index.ts:534`** documents token rotation as "nothing viewer-scoped restarts"; the native hooks restart on it (B-19.2).
- **`AndroidManifest.xml` `windowSoftInputMode="adjustResize"`** versus the port log's finding that edge-to-edge makes forms pad instead.
- **`06 Clients.md` lists `PorticoSearchKeyboard`** among the native modules without saying no screen uses it.
- **Android port log, question 3:** `apps/portico-server/server/internal/operations/feedback.go:25` still accepts only `web`, `ios` and `tvos` as a diagnostic's platform. I did not trace whether the Android app sends a platform there (outside my domain), but the question is still open in the code.

## Open questions for Justin

1. **Is Vega allowed to hold the other platforms to its React Native version**, or may the main app move ahead with override files covering the gap?
2. **Crash and log reports to the owner's server (F-19.1, F-19.2):** wanted before release? If so, should the owner be able to forward one to Portico, or do they stay on the server only?
3. **A persisted, revision-keyed copy of Home (F-19.3):** does showing last-known rows for a moment at launch fit "Fresh", or is the rule "never show anything unconfirmed"?
4. **One Android app for phones and TVs, or two listings?** It affects store review, the TV-detection override (F-19.6) and release cadence.
5. **Apple TV system users (F-19.7):** should a tvOS user switch change the Portico sign-in, or is Portico's own profile switcher the only model?
6. **The Android TV density override:** keep the dependence on an internal React Native flag (port log question 1), or move the scale into `theme.ts`?
7. **What should the Menu key do on Android TV/Fire TV** on a control that has no "More" action — nothing, or something global?
8. **Should deleting the app on iPhone/Apple TV sign it out** (B-19.12)?
9. **Desktop companion scope (A-19.53):** helper for the server and web app, or a player? The answer decides whether any of the React Native work is relevant to it.
10. **Minimum OS versions:** is supporting iOS/tvOS 15 and Android 7 a goal (old family hardware) or an accident of the template defaults?
11. **Physical acceptance:** which devices define "works" for release — at least one Fire TV Stick (Fire OS), one Vega device, one Chromecast with Google TV, one Apple TV HD?
12. **Is convert-to-TurboModules worth scheduling**, or is the interop layer acceptable until React Native forces the issue?

## Evidence boundary

- Nothing was built or run. Every bug above is a reading of source; none was reproduced. B-19.1, B-19.2 and B-19.3 each need a five-minute device or simulator check.
- No performance, memory, start-up or binary-size figure in this document is a measurement, for Portico or for any other project. Statements about weak devices are architectural expectations.
- I did not verify third-party platform facts that have moved since my reference knowledge: what the current Apple SDK enforces about scenes (B-19.18), App Store validation rules for iPad orientations (B-19.7), or which `Intl` parts the bundled Hermes provides (B-19.17).
- React Native internals were read from the installed `react-native-tvos@0.83.10-1` in `node_modules`; behaviour of the New Architecture interop layer for module queues (B-19.9) was inferred, not traced.
- The comparison projects were read at the depth listed under Scope; claims about them are limited to what the cited files show. Closed-source clients were not compared.
- The second half of `session.tsx`, most screen files, the listening and playback controllers, downloads internals and the Vega shims were not read line by line; bugs there are outside what this review could find.

## Outside my domain

- **iOS App Transport Security** (`ios/Portico/Info.plist`: arbitrary loads off, local networking on) appears to block plain-HTTP connections to a server reached by a public host name, which sits awkwardly beside "plain HTTP stays first-class". For the connection auditor (22).
- **The 15-second transport ceiling** forced the event long-poll down to 10 s for every platform (Android port log, question 4); the Apple-side question in that log is still unanswered. For the playback/connection auditors.
- **`PorticoListeningService` is exported** with the Media3 session action, as Android requires for media controllers; whether its session rejects unknown controllers is for the security or playback auditor.
- **The Android TV home-screen job runs every six hours** per device against the server; trivial load, but it is periodic chatter of the kind the principles discourage.
