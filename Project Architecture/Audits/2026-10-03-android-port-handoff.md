# Android, Android TV and Fire TV (Fire OS) port — handoff (3 Oct 2026)

For the agent building Portico for Android phones, Android TV and Android-based Fire TV. Two other agents
work at the same time on **Vega OS** (`2026-10-03-vega-port-handoff.md`) and **Roku**
(`2026-10-03-roku-port-handoff.md`); §3 says how the three stay out of each other's way. Read this whole file first, then the
Apple app's README (`apps/portico-react-native/apps/apple/README.md`), `06 Clients.md`, and the last
sections of `2026-10-02-frontend-rework-log.md` (how work here is logged and reviewed). The previous lead
(Fable) reviews your work on return, side by side with the iPhone and Apple TV apps, so keep the log in §10.

## 0. What this is, and why now

Portico's clients are the web app and one React Native app for iPhone, iPad and Apple TV
(`react-native-tvos`). Everything a person sees in that app is in `src/` and is meant to be the same on
every platform; `Native/` holds what JavaScript cannot do (the player, the gapless audio engine, the
keychain, discovery, downloads, casting). The product strategy has been "web and Apple only until the UI
settles; later ports copy the tvOS app as the reference". The UI has now been through its rework, and
Justin is moving most of his work to Linux, where he can run and test Android and Android TV while the
Apple apps keep improving from the same shared screens.

So the job is: **the same app, on Android phones, Android TV and Fire TV (Fire OS)**, built so that the
screens stay one code base and only the native layer is platform work. The Apple TV app is the oracle for Android TV; the iPhone app
is the oracle for Android phones. Where Android cannot do what Apple does, do the Android-native thing and
log it; never fork a screen.

## 1. Justin's rules (non-negotiable)

1. **No Luna or Sol lanes, no subagents.** Do it in the lead session.
2. **Tests: almost none while building.** `npx tsc --noEmit` and `npm run lint:strings` at the end of a
   phase, plus a short functional check on the emulator of what you just changed. The existing node test
   suite (`npm test`) runs once at the end of each phase; write new tests only for a new `.android.ts`
   bridge file that has logic of its own. Spend tokens on delivery.
3. **Never idle.** While Gradle or an emulator starts, work on the next piece.
4. **No AI attribution** in commits (no Co-Authored-By, no "Generated with"), and no model or agent names
   in code, files, branches or commit messages.
5. **Pre-release: no sacred cows.** Prefer the structural fix. If a shared file needs a change to serve
   Android cleanly, change it (and keep Apple working); do not bolt a shim on the side.
6. **One UI.** `src/screens`, `src/shell`, `src/ui`, `src/player` and `src/app` are shared. A screen may
   not know which operating system it is on. The only layout axis is `isTV` (television vs handheld), as
   today. `Platform.OS` is allowed in `src/bridge`, `src/ui/theme.ts` and `src/app/i18n.ts` only; a
   platform split that needs more than a line goes in a `*.android.ts` / `*.ios.ts` pair inside
   `src/bridge`.
7. **Native module names, methods and events are identical on both platforms.** The JavaScript in
   `src/bridge` is the contract: implement `NativeModules.PorticoRuntime` with the same method names and
   promise shapes, emit the same event names with the same bodies. Where Android has no counterpart, the
   module exists and answers the way tvOS answers on iPhone-only features (a harmless no-op or `null`),
   so the bridge never branches.
8. **US English source text; regional spelling is the catalogue's job.** Hide what is disabled; never show
   "not available on this platform" rows.
9. **Scale rule:** client cost is O(visible), never O(library). **Correctness over limits.** **Artwork
   shapes** as the Apple app draws them (posters for film/TV/anime, square for music and audiobooks, stills
   only inside a show page and in Search's Episodes section). You inherit these by sharing the screens;
   do not undo them in a platform file.
10. **Native text input only** (the platform keyboard, with its own dictation). No custom keyboards.
11. Review your own diff rather than adding tests. Get the shape right the first time.
12. **Decide and log.** Where this file is silent, decide the way the Apple app would, write the decision
    in the log, and keep going. Do not stop to ask.

## 2. Decisions already made (apply exactly)

- **One app, not a second app.** The Android project goes into the existing React Native app as
  `android/`, beside `ios/` and `tvos/`. The folder is already **`apps/portico-react-native/apps/native`**
  (renamed on 3 Oct, pods reinstalled, iOS build checked); nothing to rename.
- **One Android build serves phones and televisions.** Application id **`tv.getportico.portico`**
  (Apple uses `.ios` and `.tv` only because they are separate bundles). The manifest declares both the
  phone launcher and the Leanback launcher intent, `android.software.leanback` and
  `android.hardware.touchscreen` as not required, and a 320×180 TV banner. At runtime
  `Platform.isTV` (react-native-tvos reads `UiModeManager`) is the television switch, exactly as on Apple.
- **Fire TV (Fire OS) is the same app, as a Gradle product flavor `firetv`.** Fire OS is Android
  without Google Play services: the flavor leaves out the Cast SDK and Credential Manager, and the
  `google` flavor's modules answer the way tvOS does when a feature is absent (`PorticoCast.available`
  false; `PorticoAccountAuth` Google through the web flow in a Custom Tab, the same path Apple-on-Android
  uses). Everything else (Leanback launcher, the banner, D-pad focus, Media3, Keystore, NSD) is shared.
  Fire OS 6/7/8 are API 25/28/30: minSdk 25 for the `firetv` flavor (Fire OS 5 sticks, API 22, are out of
  reach for react-native-tvos 0.83 and are not a target). The capability profile uses
  `fireTvDeclaredProfile`/`androidProfile` with `family: 'firetv'` when `Build.MANUFACTURER` is Amazon.
  The Fire TV remote's Menu key (`KEYCODE_MENU`) opens what long-press Select opens on Apple TV; Back is
  Back. Amazon's Fire TV emulator image exists for Android Studio only in old form, so Fire OS checks are
  done on the Android TV emulator plus a real stick when Justin has one; say in the log which you used.
- **Legacy bridge modules, not TurboModules.** The Apple modules are `RCT_EXPORT_MODULE` classes and
  `requireNativeComponent` views on the interop layer. Android does the same:
  `ReactContextBaseJavaModule` + `ReactPackage`, `SimpleViewManager` for the views. No codegen specs.
  Kotlin, one file per module, under `android/app/src/main/java/tv/getportico/portico/native/`.
- **Player: Media3 (ExoPlayer)** for everything the server can send a phone or a television: direct play
  (the original file, byte ranges), HLS (TS and fMP4), progressive. Media3 `1.10.x`, modules
  `exoplayer`, `exoplayer-hls`, `datasource-okhttp`, `session`, `ui` (for `PlayerView`'s surface only;
  Portico draws its own chrome). The retired project's `packages/player/android` is a reference for the
  lease model, the download service and the media session, not a drop-in (see §5).
- **Music and audiobooks ("the listening owner")** are Media3 too: ExoPlayer's own gapless playback for
  FLAC, ALAC-as-FLAC, AAC, MP3 and Opus, crossfade through a second player, `MediaSession` for the lock
  screen and headphones, a foreground service so playback outlives the activity. The Apple engine decodes
  with Core Audio because AVPlayer cannot do sample-exact joins; ExoPlayer can, so the Android
  `audioDecode` table declares what ExoPlayer decodes and the server converts the rest.
- **Secure records: Android Keystore.** `PorticoRuntime`'s records (credentials, journals, queue records,
  selected viewer, profile trust with its PIN counters) live in app-private storage encrypted with a
  Keystore-held key (an AES key in `AndroidKeyStore`, one file per record, atomic rename on write).
  The same serialisation guarantee as Apple: one record read-modify-write at a time.
- **Network identity and discovery.** `PorticoNetwork`'s device key is an EC P-256 key in the Keystore
  (never exportable); `sha256`, `verify`, the TV grant envelope, Bonjour-style `browse`/`advertise` map to
  `NsdManager` (same service types and TXT fields as the Swift module reads). LAN servers over plain HTTP
  are allowed (network security config: cleartext permitted; remote traffic over TLS as the server
  requires).
- **Sign-in.** Direct sign-in and Portico Account work as on iPhone through the shared screens. Google:
  Credential Manager (Sign in with Google) on phones; Sign in with Apple on Android goes through the web
  flow in a Custom Tab and returns on `portico://account-return` (`PorticoAccountAuth.openBrowser` +
  the deep link, like iOS's `signInApple` fallback). Televisions use the code flow only (the TV shows a
  code; a phone or computer approves it), as on Apple TV.
- **Android TV home screen:** a Portico channel plus Watch Next through `TvContractCompat`, fed the way
  the Top Shelf extension is (`PorticoTopShelf.configure/expiresAt/clear/changed`, two sections, posters
  only). Phase 4; a no-op module until then.
- **Picture in Picture** on phones through the activity (`enterPictureInPictureMode`), reporting the same
  `PorticoPictureInPicture` event. Televisions: none, as on Apple TV.
- **Casting:** Google Cast SDK on phones only (`PorticoCast`: `available/current/choose/send/end`,
  events `PorticoCastSession`, `PorticoCastMessage`); the receiver application id is the same
  `PorticoCastApplicationID` the iOS build reads. Televisions never cast.
- **Downloads** on phones only, like iPhone: `PorticoDownloads` with the same twelve methods, files in
  app-private storage, SHA-256 checked before "ready", a foreground service while transferring.
- **No signing, no Play Store or Amazon Appstore listing, no widgets, no Android Auto, no Wear** in
  this work. Debug
  keystore builds are fine; Release builds embed the bundle so no Metro is needed (as the simulators do).

## 3. Where things are

- Root: `/Users/justin/Documents/Codex Projects/Portico/` on the Mac; on Linux, wherever Justin clones
  it (the repositories are self-contained; `scripts/shared-packages.mjs` expects `../portico-server`).
  - `apps/portico-react-native/apps/apple` (→ `apps/native`): the app. `src/` shared, `Native/` Apple
    modules, `ios/`, `tvos/`, soon `android/`. `test/` runs under node with a `react-native` stand-in
    (`test/react-native-stub.mjs`) — platform-neutral, it keeps working for Android.
  - `apps/portico-react-native/packages`: the copy of `client-core`, `design`, `i18n`. Edit the originals
    in `apps/portico-server/packages`, then `node scripts/shared-packages.mjs --write`.
  - `apps/portico-server`: the Go server and the web app; `packages/client-core/src/client-profile.ts`
    already has `androidDeclaredProfile`, `fireTvDeclaredProfile` and `androidProfile(options, facts)`
    with its `AndroidNativeFacts` shape (decoders, display HDR, audio decoders, passthrough): the
    Kotlin `PorticoCapabilities.facts` must return exactly that shape.
  - `portico-internal/Project Architecture`: `06 Clients.md` (both clients and the rules), `04 Playback
    and Media.md`, `Specs/Spec — Playback Protocol v1.md` (§3 capability profile, §5 sessions, §8
    queues, §13 downloads, §18 rendered audio), `Backlog.md`, this file.
  - The retired project: `/Users/justin/Documents/Codex Projects/Portico (Original Retired Project -
    GPT 5.5 and 5.6)/portico-react-native/` — `packages/player/android/src/main/java/tv/getportico/player/`
    (a Media3 player view, runtime store, downloads with WorkManager and a foreground service, a media
    session service, nearby devices; Kotlin, with unit tests) and `apps/fire-tv/android` (a Gradle setup
    on Media3 1.10.1, compileSdk 36, minSdk 24). Its protocol is older than Playback v1 and its core was
    judged fragile; read it for Android mechanics (surfaces, services, Keystore, NSD), never for the
    model, which is client-core's.
- **Branch and worktree (three agents share this repository at once).** Create your own worktree and
  branch off `frontend-rework` and work only there:
  `cd apps/portico-react-native && git worktree add "../../.worktrees/portico-react-native-android" -b android-port frontend-rework`,
  then `npm install --legacy-peer-deps` in that worktree's `apps/native`. The Vega agent works in
  `apps/vega` (branch `vega-port`) and the Roku agent in `apps/roku` (branch `roku-port`); never create
  or touch those folders. If client-core needs a change, make it in portico-server on `frontend-rework`
  as the one exception, say so in the log, and sync the packages into your worktree. Commit in your
  worktree with `git -c user.name="Justin Ehler" -c user.email="justin@MBP.local" commit -m "…"`,
  at least once per phase, with a plain description. **Do not commit in portico-internal**: write your
  log there and leave it uncommitted; the lead commits it when merging the lanes.
- **Local server for testing.** portico-server runs on Linux and macOS alike:
  `cd apps/portico-server/server && go build -o ../.scratch/portico-server ./cmd/server`, then
  `apps/portico-server/.scratch/dev-local.sh` (127.0.0.1:32500, state in `~/PorticoDev/state`, media in
  `~/PorticoDev/media`; needs ffmpeg/ffprobe on the PATH and the ssh alias the script uses for the TMDB
  token — read the script, do not print the token). The owner test account is in
  `.scratch/DEV-ACCOUNT.md` (localhost only; never print it). **From an Android emulator the host is
  `10.0.2.2`, not `127.0.0.1`:** sign in to `10.0.2.2:32500`. Go tests, if ever needed, run on the
  runner (`scripts/runner-test.sh`), not locally.
- **Toolchain (Linux or Mac):** JDK 17, Android SDK with platform 36 and build-tools, cmdline-tools
  (`sdkmanager`, `avdmanager`), Node 22, Watchman optional. Two AVDs: a phone (Pixel-class, API 35,
  Google APIs so Credential Manager and Cast work) and **Android TV (1080p, API 34)**. `adb` drives the
  emulators (`adb shell input keyevent KEYCODE_DPAD_RIGHT|DPAD_CENTER|BACK`, `adb exec-out screencap -p
  > shot.png`) — that is how you do functional checks and take the screenshots for the log.
- Metro: `npm start` (port 19424); debug builds load from it (`adb reverse tcp:19424 tcp:19424` on a
  phone emulator; the TV emulator is the same host). Release: `cd android && ./gradlew assembleRelease`
  (bundle embedded), install with `adb install -r`.

## 4. The native layer, module by module

The JavaScript side is the contract. For each module: what the Apple one does, what Android does, and in
which phase. Method names are the `RCT_REMAP_METHOD` names; events are `NativeEventEmitter` names.

| Module (`NativeModules.*`) | Apple | Android | Phase |
|---|---|---|---|
| **PorticoRuntime** (`Native/PorticoPlayer.m`) — `newRequestId`, `launchId`, `hostedUrl`, `recordTestEvent`, `loadQueueRecord`/`saveQueueRecord`, `queueRequest`/`cancelQueueRequest`, `networkRequest`, `profileRequest`, `profileInstallation`, `saveProfileTrust`/`listProfileTrust`/`unlockProfileTrust`/`removeProfileTrust`, `loadSelectedViewer`/`saveSelectedViewer`, `loadCredentials`/`saveCredentials`, `loadJournal`/`saveJournal`, `playbackRequestId`, `playbackRandomBytes`, `testLogin` | Keychain-backed records; credentialed HTTP without redirects, cookies or cache (bounded body, exact JSON); profile trust vault with PIN attempt counters (`PorticoProfileTrust.h`: fail closed on corrupt counters, never replace the key behind existing proofs) | Keystore-encrypted record files; OkHttp client with redirects, cookies and cache off, bounded bodies; the same trust vault rules. `hostedUrl` from `BuildConfig`/manifest meta-data (`PorticoHostedURL`, as `Info.plist` has it) | 0 |
| **PorticoNetwork** (`PorticoNetwork.swift`, events `portico.network`) — `sha256`, `verify`, `startMonitoring`, `browse`/`cancelBrowse`, `tvPublicKey`, `openTVGrant`, `forgetTVKey`, `advertiseTV`, `advertisePlayer`, `stopPlayerAdvertisement`, `stopTVAdvertisement` | Device key in the keychain; Bonjour search and advertisement; `NWPathMonitor` reporting one coarse link kind (wifi/wired/cellular) and a keyed network generation that never leaves the device | Keystore EC key; `NsdManager` discovery/registration with the same service types (`_portico._tcp`, `_portico-tv._tcp`, `_portico-player._tcp`) and TXT keys (read the Swift for the field names and limits); `ConnectivityManager` network callback for the link kind | 1 |
| **PorticoCapabilities** — `facts`, event `portico.capabilities` | HDR formats the panel accepts, hardware codecs, current audio output channels | `MediaCodecList` (hardware decoders: AVC, HEVC, AV1, VP9, profiles and levels), `Display.HdrCapabilities`, `AudioManager` output devices and channel counts; emit on display/route change | 1 |
| **PorticoAccountAuth** — `newBinding`, `newRequestId`, `loadContinuation`, `saveContinuation`, `openBrowser`, `signInApple`, `cancel` | Sign in with Apple natively; Google and Portico Account through the browser and `portico://account-return` | Credential Manager for Google; `signInApple` opens the web flow in a Custom Tab (returns through the deep link); continuations in the Keystore store | 1 (direct and code sign-in), 4 (providers) |
| **PorticoPlaybackModule** + **PorticoPlayer** view (`lease`, `pictureInPictureRequest`; events `PorticoPlaybackFact`, `PorticoListeningSnapshot`, `PorticoPictureInPicture`) — `acquire`, `command`, `release`, `adoptListeningSession`, `listeningSessionUpdated`, `purgeAudioCache`, `listeningAdvance`, `listeningRoute`, `listeningAction`, `listeningSnapshot`, `acknowledgeListening`, `listeningPolicy`, `listeningDeadlines`, `listeningCredentials`, `retireListening`, `listeningMetadata`, `listeningSkipIntervals` (+ test hooks) | `PorticoPlaybackController`: AVPlayer under a lease, facts (time, buffering, tracks, bitrate, errors) as events; device-fit caps (never a variant above the screen, 30 s forward buffer on 2 GB devices); a downloaded file plays through the same controller. `PorticoListeningOwner` (1,455 lines): the v1 listening session owner for music and audiobooks — queue, prepare-next/commit-next (§18), credentials, deadlines, Now Playing | Video: ExoPlayer with a `SurfaceView` in the view manager, the same lease and command vocabulary (read `NativePlaybackCoordinator.ts` and `useNativePlayback.ts` for every command and fact the JS sends and expects), track selection by the server's plan, bandwidth caps by screen size. Listening: a `MediaSessionService` hosting a second ExoPlayer, the §18 transitions, gapless by ExoPlayer, crossfade by a second player | 2 (video), 3 (listening) |
| **PorticoAudioRender** (no JS methods; the engine behind the owner) | Core Audio decode, two decks, sample-exact joins, gains baked in | Not ported: ExoPlayer's gapless and `AudioProcessor` gain. `src/bridge/audio-decode.ts` gets an Android table (what ExoPlayer decodes in software and hardware; everything else the server converts to FLAC) | 3 |
| **PorticoSubtitleSelection**, **PorticoAudioSelection** (behind the controller) | Embedded tracks by language, sidecar/uploaded/provider tracks drawn by the app, 304 revalidation of the cue document | ExoPlayer text renderer for embedded tracks; sidecar WebVTT through `MediaItem.SubtitleConfiguration`; the same cue document handling in the module | 2 |
| **PorticoSystemMedia** — `setVideo`, `clear`, `currentRoute`; events `PorticoSystemMediaCommand`, `PorticoSystemMediaRoute` | Now Playing for video (lock screen, Control Centre), interruptions, AirPlay route | `MediaSession` for the video player (phones: lock screen and headset buttons; TV: the remote's play/pause keys), audio focus loss as the interruption event, `MediaRouter` for the route | 2 |
| **PorticoDownloads** — `list`, `start`, `pause`, `remove`, `setMeta`, `storage`, `setCellular`, `readState`, `writeState`, `localFile`, `play`, `stop`; events `PorticoDownloadChanged`, `PorticoDownloadPlayback` | Background URLSession, SHA-256 check, app-private storage excluded from backup | OkHttp downloads in a foreground service (WorkManager to resume after process death), SHA-256 before ready, `noBackup` storage, cellular rule honoured; `supported` only on phones (the JS already says iPhone-only; make it "handheld") | 4 |
| **PorticoCast** — `available`, `current`, `choose`, `send`, `end`; events `PorticoCastSession`, `PorticoCastMessage` | Google Cast SDK, Portico's custom channel | Google Cast SDK for Android, same channel; phones only | 4 |
| **PorticoTopShelf** — `configure`, `expiresAt`, `clear`, `changed` | The Top Shelf extension reads a token from the app group | `TvContractCompat` channel + Watch Next, a `JobService`/WorkManager refresh reading the same small feed with the same token; televisions only | 4 |
| **PorticoSiriRemote** — `setEnabled`; events `PorticoSiriRemoteTouch`, `PorticoSiriRemoteSwipe` | Touch-surface scrubbing on the Siri Remote | No equivalent: the module exists, `setEnabled` is a no-op, no events. D-pad left/right seeks through the shared timeline (as it does on Apple TV without a touch surface) | 0 |
| **PorticoSearchKeyboard** (view: `placeholder`, `onChangeText`) | tvOS system search container with dictation | A plain `EditText` host that opens the system IME (Android TV's keyboard has voice input) and reports `onChangeText`; same props | 1 |
| **PorticoActivity** — `setCurrent`, `indexTitles`, `clear` | Handoff, Spotlight, Siri (iPhone) | No-op (as on tvOS) | 0 |
| **PorticoImagePicker** / **PorticoSubtitleFiles** — `pick` | System pickers | `ACTION_OPEN_DOCUMENT` with the same result shape (phones; televisions answer `null`) | 4 |
| **PorticoAudioRoutePicker** view (`video`), **PorticoVolumeView** view | AirPlay route picker; system volume slider (phones) | `MediaRouteButton` (Cast/route) on phones; the volume view renders nothing (phones use hardware keys through `AudioManager`; the JS already hides it when the view manager is absent) | 4 |
| **PorticoBackdrop** view | A native backdrop view, exported by `NativeViews.ts` but not used by any screen (`src/ui/Canvas.tsx` draws its own `Backdrop`) | Drop the export, or register an empty view manager; do not port | 0 |
| **URL routes** (`tvos/Portico/PorticoURLRoutes.swift`, `ios/.../AppDelegate.swift`) | One allowlist of `portico://` routes: `item/{id}`, `play/{id}`, `account-return`, `downloads`, `account`, universal links for web.getportico.tv/device, /account, /join | The same allowlist in the activity's intent filters (`portico` scheme + `https://web.getportico.tv` app links) and `getInitialURL`; anything else refused | 1 |

Also platform surface outside modules:

- **Keyboard events on TV.** `TVEventControl.enableTVMenuKey` is tvOS-only; guard it (the Shell already
  routes Android back through `BackHandler`). `useTVEventHandler` and `TVFocusGuideView` work on
  Android TV in react-native-tvos; `requestTVFocus` and `hasTVPreferredFocus` do too. Check `Focusable`
  (scale + ring with Reanimated) on the TV emulator first; it is the primitive every control uses.
- **Fonts and icons.** `UIAppFonts` → `android/app/src/main/assets/fonts`; `scripts/make-icons.sh` →
  add adaptive icons and the TV banner from `brand/`.
- **Locale.** `src/app/i18n.ts` reads `SettingsManager` on iOS; use `I18nManager`/`getLocales` from
  React Native on Android for the regional catalogue (en-US/en-GB/en-CA spelling is the catalogue's job).
- **Server-sent events.** `src/bridge/event-stream.ts` adapts `XMLHttpRequest` for streaming; verify on
  Android early (phase 1) that partial `responseText` arrives before completion — if the Android
  networking layer buffers, the module needs an OkHttp streaming reader behind the same interface.
- **Cleartext and TLS.** `network_security_config.xml`: cleartext allowed (LAN servers), system trust
  for TLS; the app never pins public CAs. Profile requests must refuse redirects (Apple:
  `NSURLSession` rejects before sending a password).

## 5. What to take from the retired project, and what not to

Read `packages/player/android/src/main/java/tv/getportico/player/` once before phase 2:

- Take: how `PorticoPlayerView`/`PorticoPlayerViewManager` host ExoPlayer in a React view; the
  `MediaSessionService` shape; `PorticoDownloadForegroundService` + `PorticoDownloadWorkScheduler`
  (WorkManager resume); `PorticoAndroidRuntime` (Keystore record store); `PorticoNearbyDevicesModule`
  (NSD).
- Do not take: its session actor and transport (`PorticoPlaybackSessionActor`,
  `PorticoPlaybackReceiverTransport`, `PorticoContinuationRetryGate`) — Playback Protocol v1 and
  client-core own that model now, and the JS tells the native side what to do.
- Its Gradle setup (`apps/fire-tv/android`) shows a working react-native-tvos Android TV project; ours
  starts from the current `react-native-tvos` template instead (0.83.x), then borrows the Media3 and
  Leanback bits.

## 6. Phases and checkpoints

Do them in order; each ends with a commit per repository and a log entry (§10). "Checked" means you did
it on the emulator and wrote one line about it.

### Phase 0 — bootstrap, first launch
- Add `android/` from the react-native-tvos 0.83 template; application id `tv.getportico.portico`; both
  launcher intents; Hermes; the metro config's `svg-lite` resolver untouched; fonts and icons.
- Kotlin: `PorticoRuntime` (store, request ids, `networkRequest`/`profileRequest`/`queueRequest`,
  trust vault), `PorticoActivity` and `PorticoSiriRemote` as no-ops, `PorticoBackdrop`.
- **Checkpoint:** the app launches on the phone and TV emulators to the sign-in screen with no red box;
  `npx tsc --noEmit` clean; the Apple app unchanged in behaviour.

### Phase 1 — sign in and browse everything
- `PorticoNetwork`, `PorticoCapabilities` (returning client-core's `AndroidNativeFacts`; the publisher
  in `src/bridge/capability-publisher.ts` and `client-profile.ts` chooses `androidProfile` on Android),
  `PorticoSearchKeyboard`, URL routes, `PorticoAccountAuth` for the direct and code flows.
- **Checkpoint, phone:** direct sign-in to `10.0.2.2:32500`; Home, every library tab, a movie page, a
  show page with its episode list, Search, Saved, Settings including the Server pages (owner); the
  capability profile the server shows under Settings › Server › Now Playing names `android`/`exoplayer`.
- **Checkpoint, TV:** code sign-in approved from the web app; the rail, focus and Back behave as on
  Apple TV (rail ↔ content, Back to the rail at a root, Back on the rail leaves); Dashboard visible.
  Build the `firetv` flavor too and install it on the TV emulator: same journey, Cast absent.

### Phase 2 — video
- `PorticoPlaybackModule` video half, `PorticoPlayer` view, subtitle and audio selection,
  `PorticoSystemMedia`, PiP on phones.
- **Checkpoint:** a direct-play file, an HLS transcode (force one with a low quality), subtitles from a
  sidecar and an embedded track, audio track change mid-play, resume, the progress bar on the card,
  the Up Next prompt; on TV the D-pad drives the timeline and the option boxes; the server's Now Playing
  shows the session with its delivery decision.

### Phase 3 — listening
- The listening owner on Media3: queue, prepare-next/commit-next, gapless between two tracks of one
  album, crossfade, the mini player (phone) and now-playing pill (TV), lock screen and headset controls,
  `audio-decode.ts` Android table.
- **Checkpoint:** an album plays through with no audible gap; an audiobook resumes at its position and
  keeps its speed; playback continues with the screen off; Up Next edits apply.

### Phase 4 — the rest of the native surface
- `PorticoDownloads` (phones), `PorticoCast` (phones), `PorticoTopShelf` → TV channel, provider sign-in
  (Google; Apple via the web), pickers, `PorticoAudioRoutePicker`.
- **Checkpoint:** a download completes with the app backgrounded and plays offline; Watch Next on the
  TV home screen shows Continue Watching after a play; a Google sign-in completes on the phone.

### Phase 5 — parity pass and builds
- Walk the iPhone and Apple TV journeys (§7) on the Android emulators side by side and fix what differs.
- Release builds (`assembleRelease`) for both, installed on fresh emulators, used once from sign-in to
  playback with no Metro.
- Run `npm test`, `npx tsc --noEmit`, `npm run lint:strings`. Update the READMEs and `06 Clients.md`
  (an Android section like the Apple one). Write the final log section.

## 7. The journeys to compare (the oracle)

On the Apple TV simulator and the Android TV emulator, then the iPhone simulator and the phone emulator:

1. Cold start, signed out → sign in (code on TV, direct on phone) → Home with the hero and rows.
2. Rail/tab navigation to every top-level destination; Back/Menu behaviour at each depth.
3. A library: Discover → Browse (sort, a filter, a genre chip) → a card's More menu.
4. A movie page: facts, genres as links, cast, pick-before-play dropdowns, Play; the player chrome, the
   option boxes, subtitles, Playback info, Sleep; back out; the progress bar on the card.
5. A show page: the episode list, play an episode, Up Next into the next one.
6. Music: an album → play → mini player/pill → Up Next → lock screen (phone).
7. Search with the platform keyboard; Saved › My List and Favorites; Settings pages; the Server pages
   on the phone, the Dashboard on TV.
8. Sign-out ended by the server → the sign-in screen with server and username filled in.

Photograph each step on both platforms (simulator screenshots on Mac; `adb exec-out screencap` on
Android) and keep them under `portico-internal/Project Architecture/Audits/android-port-shots/`.

## 8. Known Android differences to decide the same way every time

- No touch-surface scrubbing on TV remotes: seek with left/right (the Apple TV without a touch surface
  does the same); long-press accelerates if the shared timeline already supports it.
- No parallax or hover on TV: `Focusable`'s scale and ring are the whole focus language.
- Android TV emulators have no HDR and only software codecs (AVC reliably; HEVC and AV1 may fail): test
  transcode paths there and note anything HDR-related as "real device".
- Back on phones is the system back gesture/button; the navigation bar's Back arrow stays (iPhone has it).
- Keyboard avoidance: `KeyboardAvoidingView` `behavior` is iOS-only in `AuthFrame`; Android uses
  `adjustResize` in the manifest.
- Status bar: translucent, content under it only where the iPhone app allows (never under the status
  bar on scrolled content — see the Saved header fix of 2 Oct).

## 9. Not in scope (do not start these)

Play Store and Amazon Appstore listings and signing; Vega OS and Roku (other agents); Chromecast receiver changes; widgets; Android Auto; Wear OS;
tablets as a distinct layout (phones and tablets share the handheld layout as iPhone and iPad do);
changing any shared screen for Android's sake beyond what §1.6 allows.

## 9a. If the emulator cannot be driven

This work runs overnight. If the emulators cannot be started or driven from this session (no
`adb`, no display, a computer-use tool that will not reach them), **do not stop**: keep building through
the phases, keep `npx tsc --noEmit` and the Gradle build green, and write under each checkpoint exactly
what was not checked and how to check it in five minutes (the adb commands). The review does those checks.

## 10. The log (required — this is what gets reviewed)

Create `portico-internal/Project Architecture/Audits/2026-10-03-android-port-log.md` (leave it
uncommitted) and append per phase, in this order: **Built** (what, which files), **Decisions where this handoff was silent**,
**Differences from the Apple app** (each with the reason), **Not done, and why**, **Functional checks**
(what you did on which emulator, with screenshot paths), **Builds and suites**, **Commits**. Keep it
factual; the review reads the log first, then the diffs, then uses the emulators next to the simulators.
Questions for Justin go in a numbered list at the end of the log; do not wait for answers.

---

## Prompt to paste

```
You are continuing work on Portico, a self-hosted media server with a Go backend, a web app, and one
React Native app (react-native-tvos) for iPhone, iPad and Apple TV. Your task is to build the same app
for Android phones, Android TV and Fire TV (Fire OS, as a product flavor), in the same React Native
project, sharing every screen and porting only the native layer. Two other agents are building Vega OS
and Roku at the same time in their own folders and branches; stay in yours.

Read, in this order, and follow exactly:
1. portico-internal/Project Architecture/Audits/2026-10-03-android-port-handoff.md (the plan, the
   rules, the decisions already made, the module-by-module contract, the phases and checkpoints, and
   what to log).
2. apps/portico-react-native/apps/native/README.md and src/bridge/README.md.
3. portico-internal/Project Architecture/06 Clients.md, then Specs/Spec — Playback Protocol v1.md
   (§3, §5, §8, §13, §18).
4. The last sections of portico-internal/Project Architecture/Audits/2026-10-02-frontend-rework-log.md,
   to see how work is logged and reviewed here.

Rules you must not break: no subagents or helper lanes; almost no tests while building (type-check,
strings lint and a short emulator check per phase; the node suite once per phase); never idle; no AI
attribution or model names anywhere in code, files, branches or commits; one shared UI — screens never
know the operating system, only isTV; native module names, methods and events identical to the Apple
ones; US English source text; decide and log where the handoff is silent rather than asking.

Work in your own worktree and branch: cd apps/portico-react-native && git worktree add
"../../.worktrees/portico-react-native-android" -b android-port frontend-rework, then npm install
--legacy-peer-deps in its apps/native. Never touch apps/vega or apps/roku. Commit there per phase with:
git -c user.name="Justin Ehler" -c user.email="justin@MBP.local" commit -m "…". Write the log in
portico-internal but do not commit there. If the emulators cannot be driven from your session, keep
building and say in the log what was not checked and how to check it.

Do all five phases in order, each to its checkpoint, and keep the log at
portico-internal/Project Architecture/Audits/2026-10-03-android-port-log.md as the handoff's §10
describes. The previous lead will review your work against the Apple TV and iPhone apps side by side.
Start with phase 0.
```
