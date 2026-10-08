# Android, Android TV and Fire TV port — log (3 Oct 2026)

The work the handoff `2026-10-03-android-port-handoff.md` asks for, phase by phase. Worktree
`.worktrees/portico-react-native-android`, branch `android-port` (off `frontend-rework` at `a49bb83`).
This file is uncommitted, as asked.

**Status at the end (3 Oct, ~06:50):** all five phases are built and committed on `android-port`.
Start with **Changes outside the app** and **Questions for Justin** at the end: five small
client-core commits in portico-server change the Apple and web clients too, and two server defects
were found and left alone.

**Read first.** Justin's two notes during the night (3 Oct, ~02:20): run to completion without asking;
minor changes to the server or Hosted are allowed if they unblock the port, documented and highlighted.
Anything done under the second note is listed under **Changes outside the app** at the end of this file.

## Environment used (this Mac)

- Apple M4 Pro. JDK 17 is Homebrew's (`/usr/local/opt/openjdk@17`, an x86_64 build under Rosetta); the
  React Native Gradle plugin pins the toolchain to 17, and the only arm64 JDK here is Android Studio's 21.
  Builds run with `JAVA_HOME` set to the 17 and `ANDROID_HOME=~/Library/Android/sdk`. On Linux a native
  JDK 17 needs neither workaround.
- The SDK had platform 36, build-tools 36.0.0, NDK 27.1 and two arm64 system images, and no
  `cmdline-tools` (no `sdkmanager`/`avdmanager`). Nothing was downloaded: the two AVDs were written by
  hand (`~/.android/avd/Portico_Android_TV.avd`, `Portico_Android_Phone.avd`) from the installed images:
  - **Portico_Android_TV**: Android TV, **API 36**, 1920×1080 at xhdpi (the handoff asked for API 34).
  - **Portico_Android_Phone**: Google Play image, **API 37, 16 KB pages**, 1080×2400 at 420 dpi (the
    handoff asked for API 35 Google APIs). It is a production image: adb must be authorised on screen
    once ("Allow USB debugging?"), which I accepted for this Mac's own key through the emulator's
    control service (`scratchpad/emu-touch.mjs`; the emulator console's `event` commands do not reach
    this image).
- Both emulators run headless (`emulator -avd … -no-window -no-audio -gpu host`); checks are `adb`
  screenshots and key events. **Start them one at a time**: launched together, neither attached to adb.
- **Metro and the dev server (from 04:35).** Background shell commands in this session are stopped
  after two hours. Metro now runs from the app's preview launcher (entry `metro-android-port` in
  `Portico/.claude/launch.json`, port 19424). The launcher cannot read
  `apps/portico-server/.scratch/dev-local.sh` (macOS: "Operation not permitted"), so the dev server
  could not be moved the same way. I stopped the copy I had started; a `portico-server` process was
  listening on 32500 again at 04:35:48 (same state folder). It is the Vega lane's: its log is
  `~/PorticoDev/logs/dev-local-vega-lane.log`. I used it, read its log, and did not manage it. If it is gone at review: `sh apps/portico-server/.scratch/dev-local.sh`.
- The retired project is not at the path the handoff gives. A copy of its Android player package is at
  `~/Downloads/Portico-TV-Frontend-Remediation-Final/react-native/packages/player/android/`; that is
  what "the retired project" means below.

## Phase 0 — bootstrap, first launch

### Built

| What | Files |
|---|---|
| The Android project, from the react-native-tvos 0.83.0 template: application id `tv.getportico.portico`, namespace the same, compile/target SDK 36, min SDK 24 (`firetv` flavor 25), Hermes, New Architecture on (the modules run through its interop layer), all four ABIs by default | `android/build.gradle`, `settings.gradle`, `gradle.properties`, `app/build.gradle` |
| Flavors `google` and `firetv` (dimension `store`), both debuggable variants declared to the React plugin; Metro port 19424 (`reactNativeDevServerPort`); `-PporticoHostedUrl` → manifest meta-data `PorticoHostedURL` (the counterpart of `PORTICO_HOSTED_URL`) | `app/build.gradle`, `gradle.properties`, `AndroidManifest.xml` |
| One activity with the phone launcher and the Leanback launcher, touchscreen and leanback not required, `adjustResize`, `singleTask`; no backup or device transfer of app data; network security config (cleartext allowed for LAN servers, system trust anchors only) | `AndroidManifest.xml`, `res/xml/network_security_config.xml`, `res/xml/data_extraction_rules.xml` |
| Launch screen (ink with the white wordmark, as `LaunchScreen.storyboard`), always-dark theme, edge-to-edge on every Android version | `res/values/styles.xml`, `values-v31/styles.xml`, `res/drawable/launch_screen.xml`, `colors.xml` |
| Icons from `brand/`: adaptive icon (navy gradient as a drawable, the mark at 80 dp of the 108 dp canvas), pre-adaptive PNGs for Android 7, the 320×180 dp TV banner (wordmark on the navy, the tvOS icon's layers flattened), the launch logo | `scripts/make-icons.sh` (now `[apple|android]`, no argument writes both; the Apple section is unchanged), `res/mipmap-*`, `res/drawable-*` |
| Fonts: the four Manrope files are copied from `assets/` at build time under the names the theme uses (`Manrope-SemiBold.ttf`…); nothing is duplicated in git | `app/build.gradle` (`porticoFonts`) |
| **PorticoRuntime**: `launchId`, `hostedUrl`, `newRequestId`, `playbackRequestId` and `playbackRandomBytes` (synchronous), `queueRequest`/`networkRequest`/`profileRequest`/`cancelQueueRequest`, `load`/`saveQueueRecord`, `load`/`saveCredentials`, `load`/`saveJournal`, `load`/`saveSelectedViewer` (with the quarantine flag), `profileInstallation`, `save`/`list`/`unlock`/`removeProfileTrust`, `recordTestEvent` and `testLogin` (debug builds) | `native/PorticoRuntime.kt` |
| The secure record store: one file per record under the no-backup directory, AES-256-GCM with a key that never leaves the Android Keystore, the record's service and account bound into the seal, atomic rename, read-back before "saved", missing and unreadable kept apart | `native/PorticoRecordStore.kt` |
| The bounded transport: one OkHttp client, no redirects, no cookies, no cache, 12 in flight and 64 waiting, 1 MB bodies (8 MB for API GETs), 15 s, strict UTF-8, the Apple codes and messages | `native/PorticoTransport.kt` |
| The profile trust vault, rule for rule from `PorticoProfileTrust.h`: PBKDF2-HMAC-SHA256 (200,000 rounds) over PIN + device key + scope, AES-256-CBC + HMAC, attempt counters charged before the decrypt, corrupt counters fail closed, the device key never replaced behind existing proofs, expired entries pruned | `native/PorticoProfileTrust.kt` |
| **PorticoActivity** and **PorticoSiriRemote** as no-ops; the base class for event-emitting modules (`addListener`/`removeListeners`) | `native/PorticoActivity.kt`, `PorticoSiriRemote.kt`, `PorticoEventModule.kt` |
| **PorticoBackdrop**: the unused export is dropped from `NativeViews.ts` (no screen imports it; `ui/Canvas` draws its own). No Android view manager | `src/bridge/NativeViews.ts` |
| The regional catalogue reads Android's locale (`I18nManager.getConstants().localeIdentifier`) | `src/app/i18n.ts` |
| The television design space (see Decisions) | `TelevisionDisplay.kt`, `MainApplication.kt`, `MainActivity.kt` |

Pulled forward from phase 1, because the sign-in screens call them as they mount (the TV screen asks for
its activation key, the phone screen for its account continuation):

| What | Files |
|---|---|
| **PorticoNetwork**, whole: `sha256`, `verify` (Ed25519), `startMonitoring` (the `portico.network` event: a keyed generation that stays on the device, `online`, one coarse `transport`), `browse`/`cancelBrowse` over `NsdManager` with the three service types and the Swift module's TXT fields and limits, `tvPublicKey`/`openTVGrant`/`forgetTVKey` (X25519, HKDF-SHA256, AES-GCM), `advertiseTV`, `advertisePlayer` and their stops | `native/PorticoNetwork.kt` |
| **PorticoAccountAuth**: `newBinding`, `newRequestId`, `load`/`saveContinuation`, `openBrowser` (a Custom Tab; the return is taken from the activity's incoming link), `cancel`; `signInApple` rejects and `canSignInApple` is false | `native/PorticoAccountAuth.kt` |
| The `portico://` route allowlist, ported from `PorticoURLRoutes.swift` route for route, with the app-link translation (`/account`, `/device`, `/join`, `/together/join`, `/app/account-return`); applied to the launch intent and to every new intent before React Native sees it | `PorticoUrlRoutes.kt`, `MainActivity.kt`, `AndroidManifest.xml` |

### Decisions where the handoff was silent

1. **A television is measured in the 1920×1080 design space.** tvOS reports a television as 1920×1080
   points and the shared screens are designed there. Every Android television reports 960×540 dp, so the
   same screens drew at twice their size (first screenshot of the night). Scaling in JavaScript would
   have put a second scale into every `isTV` number in every screen. Instead the app gives a television
   a density where one unit is 1/1920 of the screen's width: the application's and the activity's
   resources carry that density, and React Native's display metrics are set from them before it starts.
   React Native otherwise takes density from the display itself and re-reads it when the font scale may
   have changed (`enableFontScaleChangesUpdatingLayout`); on a television that one flag is turned off
   (through `ReactNativeFeatureFlags.dangerouslyForceOverride`, an internal API — **check it at every
   React Native upgrade**). Result: on a 1080p panel one unit is one pixel, as on an Apple TV HD; React
   picks @1x images there and @2x on a 4K UI. Phones and tablets are untouched. Cost: a television does
   not re-lay-out if the system font scale changes while the app runs (it does at the next start).
2. **Curve25519 is done in a library, and its keys are sealed by the Keystore, not held in it.** The
   handoff says the device key is "an EC P-256 key in the Keystore". The wire protocol is
   `tv-x25519-v1` (the server encrypts the grant to an X25519 key) and route identity is Ed25519; the
   Keystore holds neither curve on the Android versions the app supports. So the X25519 private key is
   32 bytes in the record store (sealed by the Keystore's AES key, never exported from the app), and the
   math is BouncyCastle's lightweight API (`bcprov-jdk18on` 1.86; no JCA provider is registered). The
   same library gives a byte-oriented PBKDF2 for the trust vault: the platform's `PBEKeySpec` takes
   characters, and the vault's key material is binary.
3. **Kotlin package `tv.getportico.portico.native`**, the directory the handoff names. `native` is a
   Java keyword, so no Java source can import the package; the app has no Java source. If one is ever
   added, rename the package.
4. **Request identifiers are lowercase UUIDs** (the JVM's form). The bridge already lowercases the
   Apple ones where the core validates them.
5. **Storage runs on its own thread**, in order (`portico-records`). Android's bridge modules share one
   thread; a PIN check must not hold up the player.
6. **`testLogin`** (debug builds) is read from `cache/portico-test-login.json` in the app's own storage
   (`adb shell run-as tv.getportico.portico` can place it); Android has no launch environment.
7. **Release builds are signed with the debug keystore** (the handoff: no signing in this work).
8. **Deep links: the manifest takes every `portico://` link and the activity keeps only the allowlisted
   ones**, as the Apple app delegate does. A provider sign-in's return (`portico://account-return`) goes
   to the `openBrowser` call that is waiting for it and is not also shown to JavaScript's `Linking`,
   which is what `ASWebAuthenticationSession` does on iPhone.
9. **OkHttp 4.12.0** is declared directly (React Native brings 4.9.2); Media3's OkHttp data source needs
   4.12 in phase 2 anyway.

### Differences from the Apple app

- **`TVEventControl.enableTVMenuKey` needed no guard.** React Native's own implementation does nothing
  when its native emitter is absent, and Android has none. Back on Android TV arrives through
  `BackHandler`, which the Shell already handles.
- **`profileRequest` allows plain HTTP only to the device itself**, exactly as on Apple (passwords and
  PINs never travel in clear to another host). On the emulator the dev server is therefore reached as
  `127.0.0.1:32500` through `adb reverse tcp:32500 tcp:32500`, not as `10.0.2.2` (see phase 1).
- **A redirect** is refused for 301, 302, 303, 307 and 308 with a `Location`, which is what
  `NSURLSession` would have followed.
- **Request paths are checked after normalisation** (`/v1/../x` is `/x`), which is stricter than the
  Apple prefix test.
- **"When unlocked" and "after first unlock"** have no Android counterpart; records are readable
  whenever the app runs. They never leave the device (no backup, no transfer).
- **The launch screen's wordmark is 200 dp wide.** On a television that is 200 px of 1920, smaller than
  the Apple TV's; it is on screen for a fraction of a second. Noted for phase 5.

### Not done, and why

- **App links are declared but not verified.** `autoVerify` needs
  `https://web.getportico.tv/.well-known/assetlinks.json` naming the app's signing certificate, and there
  is no release signing yet. Until then an https link opens the browser; `portico://` links work.
- **Nearby discovery and advertisement are unchecked**: the emulator's network does not carry mDNS to
  the Mac's LAN. Real device (§ phase 1).

### Functional checks

- Android TV emulator (API 36), debug build from Metro: launches to **"Activate Portico on this TV"**
  with the QR code, a live code from web.getportico.tv (so `tvPublicKey`, the record store, the TLS
  transport and the fonts all work), focus ring on the first button, no red box.
  `android-port-shots/phase0-tv-sign-in.png`. The code survived a reinstall-in-place, so the record
  store round-trips through the Keystore.
- Phone emulator (API 37, 16 KB pages): launches to **"Welcome home."** with the Google and Apple
  buttons the account service lists, no red box. `android-port-shots/phase0-phone-sign-in.png`.
- The two red boxes on the way: the 2× television layout (decision 1) and "The account authorization
  module is missing" on the phone (why PorticoAccountAuth came forward).

### Builds and suites

- `./gradlew :app:assembleGoogleDebug -PreactNativeArchitectures=arm64-v8a`: passes (first build 6 m 52 s,
  later ones 10–15 s). `assembleFiretvDebug` and Release are built in phases 1 and 5.
- `npx tsc --noEmit` clean; `npm run lint:strings` clean; `npm test` 217 of 217.
- The Apple apps were not rebuilt: the shared changes are the dropped unused export and one
  Android-only branch in `i18n.ts`.

### Commits

- portico-react-native `8c13ae3` (branch `android-port`).

## Phase 1 — sign in and browse everything

### Built

| What | Files |
|---|---|
| **PorticoCapabilities**: `facts` answers client-core's `AndroidNativeFacts` (plus `deviceName`, `appVersion`, `manufacturer`): video decoders from `MediaCodecList` (the chip's decoder wins over a software one; widths, heights, frame rate, bit rate, level, 10-bit, HDR formats, Dolby Vision profiles), audio decoders and their channels, the display's HDR types and largest mode, what the HDMI sink takes as a bitstream, the output route and its channels. Event `portico.capabilities` on an audio-device or display change | `native/PorticoCapabilities.kt` |
| The capability profile and the client profile are the Android ones on Android: one bridge function chooses `androidProfile`/`appleProfile` and `androidCapabilityProfile`/`appleCapabilityProfile` | `src/bridge/device-profile.ts`, `audio-decode.ts`, `capability-publisher.ts`, `client-profile.ts` |
| One bridge module that names the device: `devicePlatform` (`tvos`, `ios`, `androidtv`, `firetv`, `android`), `installationPlatform` (an iPad says `ipados`), `androidFamily`, `deviceModelName`, `deviceNoun`, `isTablet` | `src/bridge/device-platform.ts`; used by `app/session.tsx`, `app/device.tsx`, `app/receiver.tsx`, `app/cast.tsx`, `bridge/setup-codes.ts`, `player/Destinations.tsx`, `shell/Shell.tsx`, `shell/ShellFrame.tsx`, `screens/SettingsPages.tsx` |
| Device nouns for Android in the catalogue: `device.noun.phone`, `.tablet`, `.androidTV`, `.fireTV` | portico-server `packages/i18n/src/catalog/en-US.ts` (see Changes outside the app) |
| **PorticoSearchKeyboard** view: an `EditText` host with `placeholder` and `onChangeText` | `native/PorticoSearchKeyboard.kt` |
| A television's Menu key is delivered as Select held past the long-press delay, so each control's own long-press action answers it | `MainActivity.kt` |
| The television text field opens its hidden input on demand (see Differences) | `src/ui/Field.tsx`, `src/ui/theme.ts` (`inputFocusOnDemand`) |
| A scrolling form pads for the keyboard wherever the window is not resized for it | `src/ui/theme.ts` (`keyboardAvoidance`), `screens/auth/AuthFrame.tsx` |
| Artwork follows the verified route (see Found on the way) | `src/app/content.ts`, `src/ui/Artwork.tsx` |
| A television is a television in the People page's device list whichever client announced it | `screens/server/PeoplePanels.tsx` |
| Event modules declare `addListener`/`removeListeners` themselves: React Native exports only the methods a module's own class declares, not inherited ones | `native/PorticoNetwork.kt`, `PorticoCapabilities.kt`, `PorticoSiriRemote.kt`, `PorticoEventModule.kt` |

PorticoNetwork, the URL routes and PorticoAccountAuth's direct and code flows landed with phase 0.

### Found on the way and fixed (shared code, every platform)

- **Artwork stayed blank after a cold start that resumed a saved sign-in.** The app shows the remembered
  sign-in at once while the server route is still being verified. In that window client-core's
  `mediaURL` answers `''` (it never names an unverified origin); `useArtwork` wrapped that in
  `{uri: ''}`, **cached it**, and nothing asked again once the route was ready. On Android an empty
  address is answered with a transparent 1×1 image *and an `onLoad`*, so the card took its placeholder
  away and showed nothing; on Apple the same race leaves the titled placeholder up. Fix: `useArtwork`
  follows the route's origin (no source, and nothing cached, until it is ready; asked again the moment
  it is), and `Artwork` treats an object source with no address as no image.
  This took most of the phase to find: the image views were present, sized, opaque and "loaded", and
  the cause only showed when the bitmap inside turned out to be 1×1.
- **The TV text field could not be opened twice after a refused focus.** React Native records a focus
  request as granted before the native view answers. `Field` now clears a stale record before asking.

### Decisions where the handoff was silent

1. **Platform words.** A device announces itself as `androidtv`, `firetv` (Android TV on Amazon
   hardware, by `Build.MANUFACTURER`) or `android`. The account service accepted `androidtv` for a TV
   code (a new code was issued), and the server's Link a TV sheet already had names for all three.
2. **Profile names are `ffprobe`'s.** The server refuses a file whose profile is not in the client's
   list, comparing the file's `ffprobe` profile lowercased with underscores. So the Kotlin facts say
   `main_10`, `high_10`, `constrained_baseline`, `profile_2`, never Android's constant names. (The
   Apple declared profile lists none, so nothing is refused on profile there.)
3. **Android tablets** take the layout an iPad takes: `isTablet` is true at 600 dp or more across the
   short side, and the Shell uses it where it read `isPad`.
4. **Device nouns:** "this phone", "this tablet", "this Android TV", "this Fire TV".
5. **Until the listening owner exists** (phase 3) the Android capability profile publishes an empty
   `audioDecode` and `gapless`/`crossfade`/`pip` false, so the server plans ordinary audio.
6. **Menu key on every television**, not only Fire TV: it is the same key code, and Android TV remotes
   that have one mean the same thing by it. In debug builds this takes the key from React Native's
   developer menu on a television (the phone keeps it).
7. **The TV code was approved from the Android phone app** (Settings › Link a TV, opened with
   `portico://device`), not from the web app as the checkpoint says: it exercises both apps and the
   same server route, and typing the test account into a browser would have put it in this session's
   transcript.

### Differences from the Apple app

- **TV text field.** On tvOS the hidden input becomes first responder wherever it sits. On Android a
  view inside a container that refuses focus cannot take it, so the container lets focus in while the
  keyboard is in use and hands focus back to the field's button afterwards (unless Next moved to
  another field). One flag in `theme.ts`; tvOS takes the old path unchanged.
- **The Android TV keyboard is a panel, not a full screen**, so the form scrolls to keep the field
  above it. The panel has the system's voice-input key.
- **Edge to edge means no window resize for the keyboard** on any Android version, so the handoff's
  "Android uses `adjustResize`" does not hold; forms pad as they do on iPhone.
- **`profileRequest` and plain HTTP**: sign-in to the dev server uses `127.0.0.1:32500` through
  `adb reverse tcp:32500 tcp:32500`, not `10.0.2.2`, because passwords only travel in clear to the
  device itself (the Apple rule, kept).
- **Search uses React Native's text input** on every platform; the `PorticoSearchKeyboard` view exists
  on Android for the contract but no screen mounts it (none does on Apple either).

### Not done, and why

- **Approval from the web app** (decision 7): not exercised. To check: open `http://127.0.0.1:32500/device`
  signed in as the owner and type the TV's code.
- **Fire OS itself**: the `firetv` flavor was run on the Android TV emulator (as the handoff says to);
  `firetv` as a platform word needs Amazon hardware to appear.
- **Cast absent on `firetv`**: nothing to see yet; Cast is phase 4 and no flavor has it.
- **Nearby discovery** (`browse`, `advertiseTV`, `advertisePlayer`): written, unchecked. The emulator's
  network carries no mDNS. Real device.
- **`PorticoSearchKeyboard`** is unexercised (no screen uses it).

### Functional checks

Phone emulator (API 37), debug build:

- Direct sign-in to `127.0.0.1:32500` with the dev owner account; Home with the hero, Continue rows and
  the tab bar. `phase1-phone-home.png`.
- Library: Discover, the Shows grid with the letter rail and filter chips, switching library to Movies.
  `phase1-phone-library.png`, `-library-browse.png`, `-movies.png`.
- A movie page (facts, Play, My List, cast with initials, More like this, Details, Files) and the
  navigation bar once the hero has scrolled away. `phase1-phone-movie-page.png`, `-movie-page-2.png`.
- A show page with its season and episode list, and an episode's details sheet.
  `phase1-phone-show-page.png`, `-show-episodes.png`.
- Search with the system keyboard (and its dictation key): "dune" gives Movies · 3 and an audiobook.
  `phase1-phone-search.png`.
- Saved › My List. `phase1-phone-saved.png`.
- Settings lists the Server pages for the owner (Dashboard, Play history, Libraries … General).
- Back at the root leaves the app; the launcher shows the adaptive icon.
- Eight cold starts after the artwork fix: every image on Home in seven; one card blank once (see
  Questions 2).

Android TV emulator (API 36), debug build:

- Sign in directly to a server: the address field opens the system keyboard, Return submits, the code
  screen appears. Approved from the phone ("sdk_google_atv64_arm64 · Android TV · Portico 1.0",
  `phase1-phone-link-tv-approve.png`); the TV lands on Home. `phase1-tv-home.png`.
- D-pad: Resume → a card → the next card; Left twice reaches the rail (it expands and pushes the
  page, `phase1-tv-rail.png`); Movies opens with focus on the first card (`phase1-tv-library.png`);
  Back returns focus to the rail; Back on the rail leaves the app.
- Menu on a focused card opens its More menu. `phase1-tv-menu-key-more.png`.
- The event stream: `/v1/notifications/events` delivers `hello`, `receiver.inbox_changed` and a
  heartbeat every 20 s at `readyState 3`, so XMLHttpRequest streams on Android and no OkHttp reader is
  needed.
- `firetv` flavor (min SDK 25) installed over the google build: the sign-in is kept, Home, Settings
  with the Dashboard row. `phase1-firetv-flavor-settings.png`.

The server's record of both devices (read from the dev database): family `android` and `androidtv`,
platform `android`, engine `exoplayer`, evidence `mixed`; H.264 to 4096×2304 with
`baseline, constrained_baseline, constrained_high, high, main`, HEVC `main`, VP9 `profile_0, profile_2`,
AV1 `main`; v1 form `phone` and `tv`.

### Builds and suites

- `assembleGoogleDebug` and `assembleFiretvDebug` pass.
- `npx tsc --noEmit` clean; `npm run lint:strings` clean; `npm test` 217 of 217.
- The Apple apps were not rebuilt. Shared files changed in behaviour for Apple: `useArtwork` (no
  source before the route is ready), `Artwork` (an empty address is not mounted), `Field` on tvOS
  (explicit `onFocus`/`onBlur` wrappers that call the same handlers). Worth one launch of each
  simulator at review.

### Commits

- portico-react-native `9c80028` (branch `android-port`).
- portico-server `c75d5fd` (branch `frontend-rework`): the four device nouns.

## Phase 2 — video

### Built

Native (Kotlin, `android/app/src/main/java/tv/getportico/portico/native/`), on **Media3 1.10.1**
(`media3-exoplayer`, `-exoplayer-hls`, `-datasource-okhttp`, `-session`):

- `PorticoPlaybackController.kt` — the one video player, a port of `Native/PorticoPlaybackController.m`:
  the lease, full validation of a command before anything changes (same refusals, same wording), the
  binding and revision rules, pending seek with a correlated `seekApplied`/`seekFailed`, the route
  suspend/resume rules (`verifying` unloads, `ready` reloads at the retained position, ten seconds to
  get there), linear (channel) time through the window start, facts stamped exactly as on Apple
  (`instanceId`, `itemInstanceId`, `binding`, `factSequence`, `lastCommandRevision`, `seeking`, the
  rate facts), failures classified as network / `decode_error` / `source_not_supported` / `engine_error`.
- `PorticoPlaybackModule.kt` — `acquire`, `command`, `release`; events `PorticoPlaybackFact` and
  `PorticoPictureInPicture`. The listening methods arrive with phase 3.
- `PorticoPlayerView.kt` — the `PorticoPlayer` view (`lease`, `pictureInPictureRequest`): a
  `SurfaceView` letterboxed to the picture's shape with the subtitle line over it; Picture in Picture.
- `PorticoSubtitleSelection.kt` — the server's cue document, read and re-checked every 25 seconds
  (ETag/304), authorisation lapsing 40 seconds after the last good answer, the address checked against
  the session's own media grant. A port of the Apple file with the same limits.
- `PorticoAudioSelection.kt` — HLS audio renditions mapped by name, with the `audioMapped`,
  `audioObserved`, `audioApplied`, `audioFailed` facts.
- `PorticoSystemMedia.kt` — `setVideo`, `clear`, `currentRoute`; a Media3 `MediaSession` whose player
  is a forwarding proxy: every system command (media keys, headset, Assistant, a Bluetooth remote)
  becomes a `PorticoSystemMediaCommand` for JavaScript and never moves the engine directly. Title,
  artwork (fetched with the bearer), audio-focus loss and "headphones unplugged" → `pause`.
- `PorticoAudioRoutePicker.kt` — the view exists and draws nothing (see Differences).

Shared JavaScript:

- `src/bridge/device-platform.ts`: `playOnPlatform`, `hasAirPlay`. `src/player/Destinations.tsx` asks
  them instead of reading `Platform` (one `Platform` use fewer outside the bridge).
- `src/bridge/audio-decode.ts`: Android declares `pip` on phones and tablets.
- `src/bridge/useNativePlayback.ts`: the event feed's long-poll asks for 10 seconds (see next section).

### Found on the way and fixed (shared code, every platform)

1. **A quiet event feed reloaded the playing stream about every 30 seconds.** The device's one event
   feed long-polls `/v1/events?waitSeconds=25`. Every API request goes through the native bounded
   transport, which ends a request at 15 seconds (Apple: `timeoutIntervalForRequest` and
   `…ForResource` = 15; Android: the same contract). A quiet poll therefore always failed; the route
   layer reads a failed request as a lost route, re-verifies, and the player receives `verifying` →
   unload → `ready` → reload. Measured on the phone: a failure every ~32 s, each one a reload. After a
   title had ended the reload asked for a retired session (404) and the Finished card was replaced
   by "Playback stopped". Fix: the feed is created with `waitSeconds: 10`
   (`src/bridge/useNativePlayback.ts`). 75 seconds of playback afterwards: no failure, no reload.
   **The Apple transport has the same 15-second limit and the same JavaScript, so I expect the Apple
   apps had this too; I could not run them to confirm.** (Question 4.)
2. **Every title that played to its end stayed in the cleanup journal for ever.** Cleanup sends a
   checkpoint and then `DELETE /v1/playback/sessions/{id}`. For a session that already ended the server
   answers `410 session_ended` (retry: never); the client treated that as a failure and kept the entry.
   Four had accumulated here; any later error then showed "Finishing a previous playback before
   starting again", and at 32 entries playback refuses to start. Fix in client-core
   (`PlaybackService.tryCleanup`): 404 and 410 on the stop mean it is done. portico-server `94592e8`.

### Decisions where the handoff was silent

- **Subtitles are always drawn by the app**, from the server's cue document, on every delivery
  (direct, remux, transcode). ExoPlayer's own text tracks are switched off. One renderer, one look,
  and the same rule as Apple.
- **No `media3-ui`.** The player chrome is the shared React Native one; the native view is a surface
  and a line of text.
- **ExoPlayer reports a finished seek in two ways** (a state change when it had to buffer, only a
  position discontinuity when the target was already buffered). Both are observed, so a seek inside the
  buffer is acknowledged too.
- **Device fit, not quality:** the track selector is capped at the panel's real size (the largest
  display mode, so a 1080p interface on a 4K panel may still use the panel). The viewer's quality
  choice stays with the server's plan.
- **Low-memory devices** (2.5 GB or less) get a smaller buffer (15–30 s).
- **Picture in Picture on Android shrinks the whole activity**, not a view. While it is active a
  second video stage is put over the activity's content so the small window shows only the picture
  (and the app's subtitles). States are reported with the Apple names. On Android 12+ leaving the app
  while a film plays enters it automatically; before 12 the activity asks on `onUserLeaveHint`.
  Coming back to the app restores the full player.
- **A device without Picture in Picture says so as soon as the player owns the video** (every
  television, `FEATURE_LEANBACK`), so the shared player never shows the button there. tvOS has
  Picture in Picture; Android TV and Fire TV in this port do not.
- **System media session id** `portico.video`; play, pause, seek, skip ±, next and previous are
  offered; all are forwarded.
- **`audioRender` in a video command is refused** ("Unsupported audio render context."): rendered
  audio belongs to the listening owner (phase 3).

### Differences from the Apple app

- **No AirPlay; "Play on" has no system picker row on Android.** `PorticoAudioRoutePicker` is an empty
  view so the shared sheet mounts. `playOnDestinations` now receives `android` / `androidtv` /
  `firetv` (client-core, portico-server `849cdbf`). Cast arrives in phase 4.
- **No `PorticoVolumeView`.** The shared code hides the control when the view is absent; the device's
  own volume keys and the system panel are the Android way.
- **No lock-screen or notification controls for video.** The session exists and answers media
  keys (checked), but Android shows a media notification only for a session inside a
  `MediaSessionService`. The phase 3 service is the listening owner's alone: a video in the
  background is either in Picture in Picture (which has its own controls) or paused.
- **Timeline step on television:** two D-pad presses moved the target about 20 seconds here. I did not compare the
  hold-to-accelerate behaviour with the Siri Remote.
- `audioUnavailable` (the Apple 422 path for a rendition the server withdrew) is not ported; a
  rendition that never maps reports `audioFailed` after 15 seconds.

### Not done, and why

- **Trick-play thumbnails, chapters, skip-intro markers**: shared JavaScript; not looked at separately.
- **HDR, Dolby Vision, surround passthrough**: nothing in the demo library exercises them and the
  emulator cannot show them. The capability profile (phase 1) declares only what `MediaCodecList`
  reports. To check: a real Android TV with an HDR file, the server's Now Playing decision, and
  `adb shell dumpsys media.metrics`.
- **Fire TV**: `assembleFiretvDebug` builds; no Fire OS device or emulator was run.
- **Downloads playing offline**: the local-file rule is ported (`noBackupFilesDir/Downloads` only);
  it is exercised in phase 4.

### Functional checks

Phone = `Portico_Android_Phone` (API 37), television = `Portico_Android_TV` (API 36), dev server on
`127.0.0.1:32500` through `adb reverse`. Server state read from `~/PorticoDev/state/state/server.sqlite`
(`playback_v1_sessions`, `playback_delivery_plans`).

| Check | Result |
| --- | --- |
| Direct play, MP4 and MKV (phone, television) | Plays; server: `direct`, video `direct`, audio `direct` |
| Resume | `portico://play/…` restarted at the saved position (`startPositionMs` matched to the millisecond) |
| Played to the end | `ended` reported, Finished card (Replay, Close), item marked watched, next episode offered on the show page |
| Up Next | Played S1 E2 from the season list: at the end the Up Next card ("…And the Bag's in the River", "Playing in 5s", Play now / Cancel) and then the next episode started by itself (`phase2-phone-up-next.png`) |
| HLS transcode | 720p file with Playback method "Always convert": `stream`, video `transcode` to h264, audio `copy`, `mpegts_hls`; plays, seeks, resumes (see the note on quality below) |
| Sidecar subtitle | `.en.srt` beside the file: "Sidecar line 11 at 30 seconds" on screen at 0:30 (`phase2-phone-sidecar-subtitle.png`) |
| Embedded subtitle | SubRip track inside the MKV, over an HLS transcode (`phase2-phone-embedded-subtitle-hls.png`) |
| Audio track change mid-play | The player restarts the session at the same position (7.848 s → 7.848 s). **The chosen track is not sent** (shared code, see Question 5) |
| Timeline tap, media keys | Seek exact; pause / play / rewind 10 s through the system media session |
| Picture in Picture (phone) | Home while playing → small window with only the picture, playback continues; opening the app restores the full player (`phase2-phone-picture-in-picture.png`) |
| Progress bar on the card | Continue Watching cards on the television Home (`phase2-tv-home-progress.png`) |
| Television D-pad | Centre pauses and shows the chrome; left/right scrub the timeline, centre commits; down reaches transport, then the option boxes; centre opens a box (Subtitles panel, `phase2-tv-player-options.png`); Back closes |
| Server Now Playing | Dashboard on the phone while the television played: "Interstellar · dev · on sdk_google_atv64_arm64 · Playing · Direct play · Home network" (`phase2-phone-dashboard-watching-now.png`) |

Notes from the checks:

- **The player's Quality row does nothing under protocol v1** (shared code, Question 5), and the
  **"On this network" quality applies only to wired or unknown links**: a phone on the server's own
  Wi-Fi is classed `wifi` by the server and reads the "Away from home" lane (Question 6). With a
  480p ceiling on that lane the server still chose direct play (`direct_play_preferred`,
  `planning_maximum_fidelity`); only "Always convert" forced the transcode.
- `uiautomator dump` returns nothing while anything animates (a spinner, a playing video). Checks
  during playback are screenshots, media keys and the server's session rows.
- The demo library had no file above 360p and no sidecar on disk (two 720p catalog entries pointed at
  files that are gone). I generated one synthetic file and one sidecar (see Changes outside the app).
- The Finished card can still be replaced by "Playback stopped" if the route really changes after a
  title ended (the player reloads a retired session). The Apple controller has the same rule; left as is.

### Builds and suites

- `assembleGoogleDebug` and `assembleFiretvDebug` pass (arm64).
- `npx tsc --noEmit` clean; `npm run lint:strings` clean; `npm test` 217 of 217.
- No new test file: nothing in this phase is a `.android.ts` bridge with logic of its own.
- The Apple apps were not rebuilt. Shared behaviour changed for Apple: the event feed's 10-second
  long-poll, the cleanup rule in client-core, and `Destinations.tsx` reading the platform from the
  bridge (same result on Apple).

### Commits

- portico-react-native `ca4d85d` (branch `android-port`).
- portico-server `849cdbf` and `94592e8` (branch `frontend-rework`).

## Phase 3 — listening

### Built

Native (Kotlin):

- `PorticoListeningOwner.kt` — the Playback v1 listening owner, a port of
  `PorticoListeningSessionOwner` in `Native/PorticoListeningOwner.m`: adoption with the same checks,
  the journal (session, `seq`, a commit in flight, deadlines) in the sealed record store, the loop
  (one request at a time, backoff 1/2/5/10/30 s, the 120-second lease), the timeline report, PATCH
  for pause and resume, `:advance` with the 412 "take the new revision and ask once more" rule,
  `:prepare-next` and `:commit-next` with the lost-commit replay, the policy (effects, preferred
  rates, autoplay, sleep and passout timers, the end-of-chapter timer), route changes, metadata and
  artwork, and the snapshot JavaScript validates. Same methods, same error texts.
- `PorticoAudioPlan.kt` — the version 2 audio plan checks (which files this engine decodes, the
  same-origin address, the gain rule).
- `PorticoPlaybackController.kt` — the owner's hold on the player (`listeningOwner`,
  `listeningRoute`, `listeningSnapshot`, `resetListeningEngine`, `clearListeningOwner`, and the
  rule that a JavaScript command under a listening owner may only attach), and the plan's edges:
  - **gapless**: the reserved track is queued behind the current one in the same ExoPlayer, held by
    `pauseAtEndOfMediaItems` until the server confirms the commit. ExoPlayer joins the two;
  - **crossfade**: a second ExoPlayer starts the next track under the end of this one (equal-power
    curves) and then becomes the player;
  - the facts the owner expects: `audioPrepared`, `audioCommit` (at `crossfade + 2` seconds from the
    end), `audioBoundary`, `audioPreparationFailed`.
- `PorticoListeningSession.kt` — `PorticoListeningService` (a Media3 `MediaSessionService`,
  `foregroundServiceType="mediaPlayback"`) and the session's player: a `SimpleBasePlayer` that plays
  nothing, shows what the owner reports (title, artist, album, artwork, position, rate) and turns
  every press into the owner's intent (pause, resume, seek, skip, next, previous, stop, rate).
- `PorticoPlaybackModule.kt` — the listening methods, with the Apple names: `adoptListeningSession`,
  `listeningSessionUpdated`, `purgeAudioCache`, `listeningAdvance`, `listeningRoute`,
  `listeningAction`, `listeningSnapshot`, `acknowledgeListening`, `listeningPolicy`,
  `listeningDeadlines`, `listeningCredentials`, `retireListening`, `listeningMetadata`,
  `listeningSkipIntervals`; event `PorticoListeningSnapshot`.
- Manifest: `FOREGROUND_SERVICE`, `FOREGROUND_SERVICE_MEDIA_PLAYBACK`, `WAKE_LOCK`; the service.

Shared JavaScript:

- `src/bridge/audio-decode.ts`: the Android `audioDecode` table (FLAC, AAC in MP4, MP3, Opus and
  Vorbis in Ogg, PCM in WAV), narrowed to the decoders `MediaCodecList` reports; `gapless` and
  `crossfade` are now declared.

### Found on the way and fixed (shared code, every platform)

1. **A finished track stopped the queue.** `V1QueuePlayer` forwarded every `session.updated` event to
   the native owner, including the one for a track the owner itself had just reported finished. The
   owner then read its own completed session as "the server ended playback" and went terminal
   instead of advancing. It is a race with the owner's next loop turn; the event feed used to be so
   slow (item 1 of phase 2) that the owner usually won. **With the feed fixed, the event wins, so
   this needed fixing for Apple as well**: client-core no longer forwards an end whose reason is
   `ended` or `completed` (portico-server `7f18ba1`). The Android owner also ignores it natively.
2. **Playback cleanup still stuck after `94592e8`.** The v1 client throws `PlaybackApiError`, not
   `ApiError`, so the `instanceof` test never matched. Cleanup now reads the status from either
   (portico-server `729f626`). Checked: six stuck entries drained.
3. **Album, artist and book pages drew nothing on television.** `TrackWindowList` gave its
   `VirtualizedList` `flex: 1`; on television the list library wraps the scroll view in a style-less
   focus guide that shrink-wraps, so the list was one pixel tall (`ReactScrollView 0,0-1808,1` in the
   view dump). `PageList` and `WindowedGrid` already measure their slot for this reason; the track
   list now does the same (`src/screens/title/TrackWindow.tsx`). **Same JavaScript on tvOS: please
   check an album page on Apple TV.**
4. **"The player could not accept a control change" when something played right after music.**
   When the native owner retires it releases the player's lease (as on Apple), but
   `NativePlaybackCoordinator` kept using it for the next, JavaScript-played item. It now takes a
   fresh lease when a listening owner goes away (`src/bridge/NativePlaybackCoordinator.ts`).

### Decisions where the handoff was silent

- **One ExoPlayer for both paths.** On Apple the plan goes to a Core Audio engine and everything
  else to AVPlayer. Here ExoPlayer plays both; "rendering" means: the plan's file (the original or
  the server's FLAC/Opus), its gain, and the edges.
- **Nothing crosses into the next track without the server's commit.** The queued track waits behind
  `pauseAtEndOfMediaItems`. A commit that arrives after the track ran out crosses at once (a short
  gap); one that never arrives drops the queued track, and the track ends and advances as usual.
- **Trim is ExoPlayer's.** The plan's `trim.startFrames/endFrames` are not applied by hand: ExoPlayer
  reads the same encoder delay and padding from the file (LAME/Xing, MP4 edit lists, `iTunSMPB`).
- **Gain is the player's volume**, so it can lower but not raise: a positive gain is clamped to 1
  (see Differences).
- **`audioPrepared`** is sent when the current track is fully buffered (ExoPlayer then loads the
  next), or three seconds after the track was queued.
- **The system session is a facade, not the ExoPlayer.** The crossfade swaps players, and the queue
  is the server's: a `SimpleBasePlayer` that reads the owner keeps the session stable across both.
  It lists one stand-in item before and after the current one so the system offers Previous and Next.
- **No video session while a listening owner holds the player.** The shared player publishes "video"
  for a moment at a track change (the new item's kind is not known yet). On Android a second session
  takes the media-button routing with it when it disappears; `PorticoSystemMedia` therefore stays
  closed while listening is active.
- **Audio focus.** A moment's loss (a notification, navigation) is ExoPlayer's own to wait out; a
  lasting loss is a pause the listener can undo (PATCH `paused`). Headphones unplugged pauses.
- **`purgeAudioCache`** resolves at once: the player keeps no audio on disk.
- **No `POST_NOTIFICATIONS` request.** Media-session notifications are exempt on Android 13+, and the
  controls appear in the system media area without it (checked, API 37).
- **Swiping the app away stops the music** (the React host is torn down, the owner retires and sends
  DELETE with the position). Playback with the screen off or the app in the background continues.

### Differences from the Apple app

- **Volume leveling cannot amplify.** A track quieter than the reference stays at its own level;
  louder tracks are lowered. An `AudioProcessor` gain stage would fix it; not done.
- **Joins are ExoPlayer's, not sample-counted by Portico.** FLAC (and the server's FLAC conversion)
  joins exactly; MP3 and AAC depend on the file's own gapless information. There is no §18.8 fixture
  run on Android, and no conformance harness.
- **ALAC is always converted** (to FLAC) on Android; Ogg Opus and Vorbis play directly, which Apple
  converts.
- **The crossfade starts when the commit and the fade length allow**: measured 3.6 s for a 4 s
  setting (the engine looks twice a second and the second player needs a moment to start).
- **No lock-screen artwork cache, no `route` name for the built-in speaker** (the snapshot's `route`
  names Bluetooth, wired, USB and HDMI outputs only).
- **Stop** on a headset or the notification retires the session (as Apple's stop command).

### Not done, and why

- **Audiobooks under the native owner were not exercised.** Every audiobook in the dev library is a
  single file whose queue entry kind is `audiobook`; the app hands only `song`, `track` and
  `audiobook_file` to the native owner (`native-listening-kinds.ts`), so these play through the
  JavaScript path on every platform. The owner's book code (preferred book rate, end-of-chapter
  timer) is ported but unverified. To check: a multi-file audiobook.
- **Nothing was heard.** The emulators run without audio. Gapless and crossfade are verified by the
  server's sessions, the engine's positions and the system session, not by ear. To check on a
  device: an album ripped gapless (FLAC), then the same with a 4-second crossfade and shuffle.
- **Lock screen**: the emulator image has no keyguard. The system media controls (the same surface)
  are shown in the notification shade.
- **Sleep timer, passout timer, end-of-chapter**: ported, not exercised.
- **Android Auto / Wear**: the session is exported and would answer; no browse tree
  (`MediaLibraryService`) is offered. Not asked for.

### Functional checks

| Check | Result |
| --- | --- |
| Album → Play (phone) | Native owner adopts; timeline `seq` rises every 10 s; mini player shows the track; system session `PLAYING` with title, artist, album, artwork; service in the foreground (`mediaPlayback`) |
| Gapless edge | `:prepare-next` 200 ~40 s before the end, `:commit-next` 200 two seconds before it, old session `completed`, new session reporting from 0.4 s, app and system session on "Rumour Has It" |
| Crossfade (4 s, shuffled so the tracks are not neighbours) | `crossfade start seconds=3.6`, second player primary at 3.47 s with full volume, boundary, session continues |
| Screen off | `mWakefulness=Asleep`; position and timeline went on (39 s → 51 s), ExoPlayer wake lock held |
| Headset / remote keys | Pause, play, rewind (−10 s), next, previous, stop through the session. Fast-forward: see note |
| System media controls | Title, artist, artwork, scrubber, previous/pause/next (`phase3-phone-system-media-controls.png`) |
| Full player (phone) | `phase3-phone-music-player.png` |
| Up Next edits | Removed the next track in Up Next; Next then met 412, took the new revision, asked again, got `queue_ended` and paused |
| Next / Previous | Next while playing and while paused: `:advance` 200 and the next track (paused stays paused) |
| Music, then another item | Starts on a fresh lease, no error |
| Television | Album page, Play, full player, Back to the page with the now-playing pill, play/pause and stop from the remote (`phase3-tv-music-player.png`, `phase3-tv-now-playing-pill.png`; the pill sits under the debug banner in the shot) |
| Audiobook (JavaScript path) | Plays; from `portico://play/…` it resumed at its position (13.3 s) |

Notes from the checks:

- **Fast-forward in debug builds.** React Native keeps the fast-forward key-down for its developer
  menu when developer support is on, so the session sees only the key-up and ignores it. Sent through
  the system (`cmd media_session dispatch fast-forward`) it seeks 30 seconds. Release builds do not
  intercept it.
- **Resume from the audiobook's page started at 0:00** although the server's session started at
  33.4 s: the first command JavaScript sends already carries `pendingSeek 0` (Question 9). Shared
  code; from a deep link the same book resumed correctly.
- Changing "Play the next episode automatically" or a Music setting takes effect for listening
  after the app restarts (the listening preferences are read once).

### Builds and suites

- `assembleGoogleDebug` and `assembleFiretvDebug` pass (arm64).
- `npx tsc --noEmit` clean; `npm run lint:strings` clean; `npm test` 217 of 217.
- No new test file (no `.android.ts` bridge with logic of its own).
- The Apple apps were not rebuilt. Shared behaviour changed for Apple: items 1–4 above.

### Commits

- portico-react-native `56945a6` (branch `android-port`).
- portico-server `7f18ba1` and `729f626` (branch `frontend-rework`).

## Phase 4 — the rest of the native surface

### Built

- **`PorticoDownloads`** (phones and tablets; `PorticoDownloadCenter.kt`, `PorticoDownloads.kt`): the
  twelve Apple methods (`list`, `start`, `pause`, `remove`, `setMeta`, `storage`, `setCellular`,
  `readState`, `writeState`, `localFile`, `play`, `stop`) and both events. Files, the manifest and the
  JavaScript state document live in `noBackupFilesDir/Downloads` (the same folder the player accepts a
  local file from). A transfer is an OkHttp request that belongs to the process, with a foreground
  service (`PorticoDownloadService`, type `dataSync`) keeping the process alive while one runs. A
  partial file is continued with `Range` only under the grant it was fetched with. SHA-256 is checked
  before a file is called ready; the grant address is dropped once it is. "Never use mobile data"
  suspends a transfer in flight. The same failure codes as Apple (`grant_expired`, `server_refused`,
  `verification_failed`, `storage_full`, `network`, `transfer_failed`).
- **`PorticoOfflinePlayerActivity`**: the plain full-screen player `play()` opens (the launch flow's
  offline Downloads, where Portico's player is not mounted): the system `VideoView` and its controls,
  position reported every five seconds as `PorticoDownloadPlayback`.
- **`PorticoTopShelf`** (televisions; `PorticoTopShelf.kt`): `configure`, `expiresAt`, `clear`,
  `changed`. The same feed (`GET /v1/topshelf`) with the same token, drawn as the system's **Watch
  Next** row (the feed's `continue` section, up to ten, with progress) and **one Portico channel** (the
  next row of the viewer's Home, under its own title, up to ten). The token is sealed in the record
  store; posters are fetched with it, shrunk, cached, and served to the launcher by a read-only
  provider (`PorticoShelfArtProvider`). A `JobService` reads the feed again every six hours.
- **`PorticoImagePicker`** and **`PorticoSubtitleFiles`** (`PorticoPickers.kt`): `pick()` through the
  system document picker, with the Apple result shapes and limits (a picture at most 1024 pixels as
  JPEG; one SRT/VTT/ASS/SSA/SUP file or a matching IDX + SUB pair, 4 MiB text, 32 MiB binary). A
  television answers `null` and "unavailable", as tvOS does.
- **`PorticoCast`** (google flavor only, phones and tablets; `app/src/google/…/PorticoCast.kt`): the
  five methods and two events over `urn:x-cast:tv.getportico.cast`, with the Cast SDK's own chooser.
  The receiver id is the manifest's `PorticoCastApplicationID` (Gradle property
  `porticoCastApplicationId`); with none, `available` is false. The `firetv` flavor has no Cast SDK
  and no module (`app/src/firetv/…/PorticoFlavor.kt`).
- Shared JavaScript: downloads are offered on every handheld (`offline-downloads.ts`:
  `supported: !!native && !Platform.isTV`); `src/bridge/random-id.ts` (see next section).

### Found on the way and fixed (shared code, every platform)

1. **"Download…" answered "Portico can't reach your server".** No request had failed: the downloads
   service makes its operation id with client-core `randomId()`, which needs `crypto.randomUUID` or
   `crypto.getRandomValues`, and Hermes has neither. The call threw a `TypeError` before anything was
   sent. The app now installs `crypto.randomUUID` at startup from the native UUID source it already
   uses for playback request ids (`src/bridge/random-id.ts`, imported by `index.js`), so every shared
   service that makes its own ids works. **The Apple apps run the same engine and the same code: I
   expect Download fails the same way on iPhone today** (Question 11).

### Decisions where the handoff was silent

- **No WorkManager.** A foreground service keeps a running transfer alive; after the process is
  killed the download is shown as paused and "Continue" asks for a fresh grant (which the app must
  do anyway: grants are short-lived). A worker could not have resumed with an expired grant.
- **The foreground service's notification** says "Downloading for offline viewing" (channel
  "Downloads", low importance). Android requires one; no notification is posted when a download
  finishes (the app shows it).
- **The plain offline player is the system `VideoView`**, not Media3's UI module (which the app does
  not ship). It plays what the platform plays; Portico's own player is used whenever the app is
  signed in.
- **Home screen rows through the platform `TvContract`** (API 26+), not the `tvprovider` library: no
  new dependency. The channel is the app's first, so the system shows it without asking.
- **Artwork shape on the channel** follows the feed section's `shape` (`square` for music and
  audiobooks, poster otherwise).
- **Progress on Watch Next**: the feed gives a percentage; a nominal 100-second length carries it to
  the launcher's progress bar.
- **Cast needs a `FragmentActivity` and Google Play services**; anything else answers unavailable.

### Differences from the Apple app

- **Provider sign-in goes through the browser** (a Custom Tab and `portico://account-return`), for
  Google as for Apple. Credential Manager is not used: it needs a Google OAuth client for the Android
  app and a Hosted endpoint that accepts its ID token, and Hosted is outside this port.
  `canSignInApple` is false; Sign in with Apple is the web flow.
- **`PorticoAudioRoutePicker`** draws nothing. Cast is chosen through `PorticoCast.choose()`; there
  is no AirPlay.
- **A download asked for with the app already in the background does not start until the app
  returns**: the "prepare, then fetch the grant" steps are JavaScript timers, which React Native
  pauses on Android when the app is not in front. Once the transfer has started it finishes in the
  background (checked). iOS has the same split (JavaScript is suspended; the URL session continues).
- **Fire TV has no home-screen rows.** `TvContract` rows are an Android TV launcher feature; the
  module is the same and its rows are simply not shown on Fire OS.
- The Downloads list shows a placeholder for a download's artwork (the address needs the server;
  not cached with the file). I did not check what iPhone shows.

### Not done, and why

- **Google sign-in was not exercised**: it needs a real Google account and Hosted Services, and I do
  not create accounts or enter real credentials. To check: phone → Sign in with a Portico Account →
  Google, in the Custom Tab; the app should return through `portico://account-return`.
- **Cast was not exercised**: no receiver, and no `porticoCastApplicationId` in this build. It
  compiles in the google flavor and reports unavailable. To check: build with
  `-PporticoCastApplicationId=<the iOS id>` and cast to a Chromecast on the same network.
- **Selecting a Play Next card from the launcher** was not driven (the launcher cannot be navigated
  reliably by blind key presses). Its intent is `portico://play/<id>`, which `adb shell am start`
  opens and plays on the television (phase 2).
- **The pickers were not driven** (the system document picker is another app's interface). To check:
  Profiles → change picture; player → Subtitles → Add.
- **Downloads resume after a process death, pause/continue, the mobile-data rule, a failed checksum**:
  written to the Apple rules, not exercised.

### Functional checks

| Check | Result |
| --- | --- |
| Download (phone) | More actions → Download… → Original: `ready`, 6,727,618 bytes, SHA-256 matched, file under `no_backup/Downloads/<id>/`, no grant address left in the manifest (`phase4-phone-downloads.png`) |
| Plays offline | With the phone's route to the server removed, the download played in Portico's player from the Downloads screen |
| Completes in the background | A 502 MiB file: Home pressed as the transfer began; four seconds later `ready`, 502 / 502 MiB, with the launcher in front |
| Watch Next (television) | After approval the feed was read (`continue: 3`, …) and the Android TV home screen's Play Next row shows Interstellar, Breaking Bad and Dune with their posters (`phase4-tv-home-play-next.png`) |
| Portico channel (television) | "Portico: Continue Listening" with two items on the home screen (`phase4-tv-home-portico-channel.png`) |
| Google build with Cast, Fire TV build without | Both assemble; the google build starts on phone and television; Cast reports unavailable (no receiver id) |

Notes from the checks:

- A blind tap of mine on the phone opened the profile switcher and ended the session that had approved
  the television, which signed the television out. I linked it again (Settings → Link a TV on the
  phone, the code from the television). The rule I work to since: no tap on a coordinate that was not
  just read from the screen.
- The first title I tried to download answered "The original file is not reachable" for every
  quality: it was the regenerated `Dune (2021) - 720p.mkv` (its catalog entry predates the file).
  Other titles download.

### Builds and suites

- `assembleGoogleDebug` and `assembleFiretvDebug` pass (arm64); the google build now includes
  `play-services-cast-framework:22.1.0`.
- `npx tsc --noEmit` clean; `npm run lint:strings` clean; `npm test` 217 of 217.
- The Apple apps were not rebuilt. Shared behaviour changed for Apple: `crypto.randomUUID` is
  installed at startup (`index.js`).

### Commits

- portico-react-native `d18b40b` (branch `android-port`).

## Phase 5 — parity pass and builds

### Built

- **Release builds**: `assembleGoogleRelease` (36.9 MB) and `assembleFiretvRelease` (34.9 MB), arm64,
  the JavaScript bundle embedded (Hermes bytecode), no Metro.
- **Documentation**: `apps/native/README.md` (an Android section: flavors, build, emulator notes,
  deep links, signing), `src/bridge/README.md` (which files may know the platform, and the "absent
  module means absent feature" rule), `06 Clients.md` (the "Android app (planned)" paragraph is now
  an "Android app" section like the Apple one; uncommitted, as the rest of portico-internal).

### Decisions where the handoff was silent

- **"Fresh emulators" are fresh installs.** This Mac has no `avdmanager` (see Environment), so I did
  not create new devices: the debug app was uninstalled from both emulators (which removes its
  data, its Keystore-sealed records and its sign-in) and the release build installed in its place.
- **One ABI was built** (`-PreactNativeArchitectures=arm64-v8a`), the emulators' own. The project
  still lists all four.
- **Release is signed with the debug keystore** (the template's setting). A release key is yours to
  create; nothing in the build depends on which key signs it except the App Links check
  (`assetlinks.json` needs the release key's fingerprint).

### Differences from the Apple app (found on the walk)

- **The Apple side of the side-by-side walk was not run.** The simulators were not started in this
  session; the screenshots below are Android only, named so they can be laid beside the Apple ones.
- **Television sign-in ended by the server** returned the television to the first screen ("Activate
  Portico on this TV"), not to a sign-in screen with the server filled in. Seen when the session
  that had approved it was ended (phase 4). I did not compare with Apple TV.
- **After Stop from a remote or headset**, the television's now-playing pill stays, reading
  "Paused" (`phase5-tv-11-now-playing-pill.png`), although the session was ended. Shared screen
  code; the Apple owner maps Stop the same way, so it probably shows the same. Not changed.
- One Continue Watching card on the television's Home had no picture after the fresh install
  (`phase5-tv-03-home.png`, "Dune"): it is the regenerated 720p file, whose catalog entry has no
  artwork of its own. Not the fade problem of Question 2.

### Not done, and why

- Journey 3 (a library's Browse: sort, filter, genre chip, a card's More menu) and journey 8 as
  written were not walked again on the release build; they were covered on the debug build in
  phase 1, and nothing native changed under them since.
- No Fire OS device or emulator: the `firetv` build was assembled (debug and release) and never run.
- No real phone, tablet or television: nothing here was seen on hardware. HDR, surround passthrough,
  Bluetooth routes, a tablet layout and real remotes are untested.

### Functional checks (release build, no Metro)

| Step | Phone | Television |
| --- | --- | --- |
| 1. Cold start, signed out → sign in → Home | Direct sign-in typed; Home with hero and rows (`phase5-phone-01`, `-02`) | Code approved from the phone; Home (`phase5-tv-01`, `-02`, `-03`) |
| 4. A movie page, Play, the player | `phase5-phone-03`, `-04`; server: direct play | `phase5-tv-04`, `-05`, `-06`; server: direct play, form `tv` |
| 5. A show page | `phase5-phone-05` | `phase5-tv-07` |
| 6. Music: album → play → mini player / pill | `phase5-phone-06`; system session playing | `phase5-tv-08`, `-09`, `-10`, `-11` |
| 2, 7. Tabs, Search with the system keyboard, Saved, Downloads, Settings | `phase5-phone-07-tab-*`, `-08`, `-09` | the rail in every television shot |

### Builds and suites

- `assembleGoogleRelease`, `assembleFiretvRelease`, `assembleGoogleDebug`, `assembleFiretvDebug`: pass.
- `npx tsc --noEmit` clean; `npm run lint:strings` clean; `npm test` 217 of 217.
- The Apple apps were not built at any point in this work. Everything shared that changed is listed
  per phase under "Found on the way" and under Changes outside the app; the ones a reviewer should
  look at first on Apple are the event feed's 10-second poll, the track-end event guard, the
  track list's height on television, the coordinator's fresh lease, and `crypto.randomUUID`.

### Commits

- portico-react-native `473963e` (branch `android-port`; six commits from `8c13ae3`).
- portico-server, branch `frontend-rework`: `c75d5fd`, `849cdbf`, `94592e8`, `7f18ba1`, `729f626`.
- portico-internal: nothing committed (this log, `06 Clients.md` and `Audits/android-port-shots/`
  are uncommitted changes there).

### State left behind

- Both emulators run the **release** build, signed in to the dev server (phone: the dev owner;
  television: linked from the phone). The phone holds two small downloads and one 503 MiB one.
- Metro (`metro-android-port`) was started from the app's preview launcher and is stopped at the
  end of this session. The dev server on 32500 is the Vega lane's process.
- Profile `dev` settings are as they were found (see Changes outside the app, items 5 and 9).

## Changes outside the app

**For Justin's attention.** Each of these is outside `apps/portico-react-native`.

1. **portico-server `c75d5fd` on `frontend-rework`** (the exception the handoff allows): four strings in
   `packages/i18n/src/catalog/en-US.ts` (`device.noun.phone`, `.tablet`, `.androidTV`, `.fireTV`). No
   server code. Synced into the worktree's `packages/` with `scripts/shared-packages.mjs --write`.
2. **portico-server `849cdbf` on `frontend-rework`** (same exception): client-core
   `presentation/destinations.ts` — `DestinationPlatform` gains `android` and `firetv`, so "Play on"
   can be asked for an Android phone and a Fire TV. Types and one table; no behaviour change for the
   existing platforms.
3. **portico-server `94592e8` on `frontend-rework`** (same exception): client-core
   `PlaybackService.tryCleanup` — a stop answered 404 or 410 completes the cleanup. Changes behaviour
   on every client that uses client-core (web, Apple, Android): finished sessions no longer pile up in
   the journal. Seven lines.
4. **Dev library, `~/PorticoDev/media/Movies/Dune (2021)/`** (demo data): added
   `Dune (2021) - 720p.mkv` (90 s synthetic test pattern, 1280×720 H.264 + AAC, made with ffmpeg; the
   catalog already had an entry of this name pointing at a missing file) and
   `Dune (2021) - 720p.en.srt` (30 numbered lines). Then "Scan now" on Movies. Nothing removed.
5. **Dev server viewer settings** (profile `dev`): Quality "On this network" and "Away from home", and
   "Playback method", were changed for the transcode check and **set back** to Original / 1080p /
   Automatic.
6. **`Portico/.claude/launch.json`**: one entry added, `metro-android-port` (the folder is ignored by Git).
7. **portico-server `7f18ba1` on `frontend-rework`** (same exception): client-core
   `playback-v1/v1-queue-player.ts` — a `session.updated` whose data says the session ended as
   `ended` or `completed` is not forwarded to the native listening owner. **Changes Apple**: without
   it (and with the event feed now prompt) the Apple owner would stop at the end of each track
   whenever the event beat its loop.
8. **portico-server `729f626` on `frontend-rework`** (same exception): client-core cleanup reads the
   HTTP status of either API's error class. Completes item 3.
9. **Dev server viewer settings** (profile `dev`), phase 3: "Play the next episode automatically"
   turned on for the album checks and **set back to off**; Crossfade set to 4 seconds and **set back
   to Off**. Gapless playback was toggled off and on again (it was on).

10. **Dev library** (phase 4): a 503 MiB synthetic file `Movies/Big Buck Bunny (2008)/` was added for
    the background-download check and **deleted again** (the catalog keeps an entry for a file that
    is gone until the next scan). The phone emulator still holds its downloaded copy.
11. **The television emulator was signed out and linked again** (see phase 4, notes).

No server Go code, schema or Hosted change so far. Two server defects were found and **not** fixed
(Questions 7 and 8); the dev server on this Mac is the Vega lane's process
(`~/PorticoDev/logs/dev-local-vega-lane.log`), which I only read.

## Questions for Justin

1. (Phase 0) The television design space depends on one internal React Native flag override. The
   alternative is a JavaScript scale in `theme.ts` plus a pass over every `isTV` literal. I chose the
   native one because it keeps the screens identical; say if you would rather not depend on the flag.
2. (Phase 1) Once in eight cold starts one Home card showed its raised background with no image and no
   title (the image had loaded; its fade had not shown). Not reproduced in the next five. If it
   recurs, the likely cause is a re-render landing while the native-driven fade runs; the cheap
   guard is to set the value to 1 when the fade ends.
3. (Phase 1) "Report a problem" (`operations/feedback.go`) accepts only `web`, `ios` and `tvos` as a
   diagnostic's platform. An Android report would be refused. Not changed yet; it is a one-line
   server change if you want Android reports accepted.
4. (Phase 2) The event feed's 25-second long-poll could never finish inside the native transport's
   15 seconds, and each failure reloaded the playing stream. I shortened the poll to 10 seconds for
   every platform. Please confirm on Apple TV that playback used to hiccup about every 30 seconds
   while nothing else was happening (or tell me it did not, and I will look for why Apple differed).
   The alternative fix is a longer limit for this one request in both native transports.
5. (Phase 2) Under protocol v1 the player's **Quality** row and an **audio track** change both restart
   the title through `V1QueuePlayer.playItem`, which ignores the quality and the audio stream it is
   given (`_quality`, `_audioStream`). The restart happens; the choice is lost. This is shared code,
   so iPhone and Apple TV behave the same. Is wiring these to the v1 session commands (`setQuality`,
   the audio track) planned, or should the rows be hidden until it is?
6. (Phase 2) The server classes a phone on the server's own Wi-Fi as `wifi`, and reads the quality
   lane the settings page calls "Away from home — on Wi-Fi or a wired connection anywhere else".
   "On this network" is used only for wired or unknown links on the LAN
   (`playback/delivery_policy.go`, `ResolveNetworkClass`). Intended?
7. (Phase 3, server) **`:advance` answers 500 after a gapless or crossfaded edge, and the queue has
   moved anyway.** A session started by `:commit-next` is stored with `media_generation = 0`
   (`playback_v1_sessions`, `start_key` `commit.pn_…`). The next `:advance` builds
   `SessionReplacement{Generation: 0}` (`playbackv1/sessions.go:667`) and `playback.create` refuses
   it (`service.go:181`, "invalid playback JSON"). Log: `request … failed (500 internal): invalid
   playback JSON`. Seen with Next after an edge: each try moved the queue one entry and started
   nothing. A second edge (another commit) is unaffected, so an album plays through; a skip after an
   edge does not. Apple uses the same calls. Not fixed: it needs a server build and the running dev
   server is another lane's.
8. (Phase 3, server/product) With "Play the next episode automatically" off, **music stops after
   every track** (the listening preference `autoplayNext` is the same setting). Intended for music?
9. (Phase 3, shared) "Resume" on an audiobook's page starts at 0:00 while the server's session starts
   at the saved position; the first player command already asks for position 0. From a deep link the
   same book resumes correctly. I did not find the caller; is this known on iPhone?
10. (Phase 3, shared) Single-file audiobooks (queue entry kind `audiobook`) are not handed to the
    native listening owner; only `audiobook_file` is. They play through JavaScript, without the
    lock-screen session. Intended?
11. (Phase 4, shared) `crypto` does not exist in Hermes, so `new DownloadsService(api)` threw on its
    first request ("can't reach your server"). I install `crypto.randomUUID` at startup for every
    native platform. Was Download working on iPhone before this? If it was, something else provides
    `crypto` there and I would like to know what.
12. (Phase 4) Provider sign-in on Android uses the browser for Google as well as Apple. Credential
    Manager (the one-tap Google sheet) needs an Android OAuth client and a Hosted endpoint for its ID
    token. Do you want that built (it touches Hosted)?

