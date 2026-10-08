# Fire TV Vega OS port — log (3 Oct 2026)

Worktree `.worktrees/portico-react-native-vega`, branch `vega-port` (from `frontend-rework` at `a49bb83`).
Everything is under `apps/portico-react-native/apps/vega`. `apps/native` and `apps/roku` were not touched.

**Did anything run on a device? Yes.** The Vega virtual device (Vega OS 1.2, `vvrp-tv-arm64`) is
installed on this Mac and can be built for, installed to, launched, keyed and photographed from the
session. No physical Fire TV was used. **One limit that shapes everything about playback:** from
the moment any media plays, the virtual device's screenshots are stale until the app is restarted.
Browsing was checked by eye; playback was checked by trace, never by eye.

**No Apple TV reference shots.** `Audits/tv-reference-shots/` does not exist and this Mac has no
simulator runtimes installed (`xcrun simctl list runtimes` is empty), so the Apple TV app could not
be photographed. Parity below is judged against the handoff's §5 measurements and the shared
code's own rules. **Request:** reference shots of the §6 screens from the tvOS simulator.

## For Justin first

**Where it stands (branch `vega-port`, five commits, `78511f9` → `6cf74b9`):** the shared
screens run unchanged on Vega. Sign-in, Home, libraries, title pages, Search, Saved and Settings
work and were looked at. Video, music and subtitles play, by trace. `apps/native/src` and
`packages` are untouched; the shared tests pass (217 of 217).

1. **Nothing outside `apps/vega` was changed in the code.** No server, Hosted or shared-source
   edits. Two things were done *to the running dev server*, both reversible and both listed in the
   phases: it was restarted once with the documented script after it had stopped (phase 1,
   Decisions 6), and the dev profile's "autoplay next" was turned on for one test and put back
   (phase 3, Decisions 3; the preference's revision went from 13 to 15, its value is as it was).
   One sort order in the Movies library was changed and put back.
2. **I have not seen a single frame of video, nor the player while anything plays** (above).
   Everything said about playback rests on the app's trace and the device's media log. Someone
   needs to look at it on a device with a screen: `vega virtual-device start` without the script's
   `--gui-off`, or a Fire TV. This is the largest gap in the port.
3. **Vega's media element will not open an `http:` address, and has no HLS client.** So playback
   from a LAN server needed an HLS client of the app's own over media source extensions
   (`src/bridge/hls.ts`). It works with what the server writes today; it is new code on the
   critical path and has seen one library of H.264/AAC test files (phase 2).
4. **Three things Vega cannot do that the Apple TV app does:** change playback speed (audiobooks
   included); gapless or crossfaded music (0.61 s between tracks, measured); play a file as it is
   from an HTTP server (everything is remuxed or converted to HLS). Phase 2 and 3, Differences.
5. **Amazon's Reanimated build is not used.** Styles it drove landed late or on the wrong control.
   The shared code's use of that API is served by 180 lines in `apps/vega`; motion runs on the
   JavaScript thread. Fine on the virtual device; unmeasured on a Fire TV Stick (phase 1,
   Differences 1).
6. **Focus is rebuilt, not native.** tvOS re-homes focus when the focused view goes away; Vega
   does not, and the shared screens depend on it everywhere. `src/shims/focus.tsx` reconstructs
   it from focus and blur events, with timers. It passed every journey I walked; it is the part
   most likely to show an edge on real hardware (phase 1, Differences 9).
7. **A shared-code finding that matters beyond Vega:** a restore of the saved sign-in that fails
   at launch signs the device out at the server (phase 1, Shared changes 8). Read from the code,
   consistent with two sign-outs here; worth the lead's look for Apple too.
8. **The lanes collided once** over emulator ports; the Vega device now runs on its own
   (Toolchain facts 4).
9. **Vega has no keychain.** Sign-in records are files in the app's private storage (phase 0,
   Differences 2).

## SDK facts (established on this machine, 3 Oct)

| Fact | Value |
|---|---|
| Vega SDK | 0.24.9914 (`~/vega/sdk/vega-sdk/main/0.24.9914`), Vega CLI 1.3.4 |
| React Native | 0.83.0, with `@amazon-devices/react-native-kepler` 4.0.1 as the platform (`Platform.OS === 'kepler'`), React 19.2.0, Hermes |
| How the SDK's React Native ships | The runtime is on the device as a system bundle; the app bundle carries stubs. New architecture only: **the legacy `NativeModules` object throws when read** (`__fbBatchedBridgeConfig is not set`). TurboModules and `TurboModuleRegistry.get` work. |
| OS target | Vega OS 1.2 (`manifest.toml` `min`/`target`); runtime module `react_native_kepler_4` |
| Template | `vega project generate -t helloWorld` ("targets RN v0.83") |
| Virtual device | `vvrp-tv-arm64`, "OS 1.2 (TV Ship/101520710)", aarch64, 1920×1080 at 320 dpi |
| Layout size | **960×540 points at pixel ratio 2** (`Dimensions.get('window')`), not 1920×1080 at 1 |
| Node | 22.22.2 |
| Package format | `.vpkg`, built for aarch64, armv7 and x86_64 in one build; Debug build ≈ 30 s, Release ≈ 33 s |
| Host address from the virtual device | `10.0.2.2` (it is an Android-emulator fork); confirmed in phase 1 against the local server on port 32500 |
| JavaScript runtime | No global `crypto`; `TextDecoder`, `Intl`, `BigInt`, `AbortController`, `Headers`, `Response` present; `Intl` reports `en-US`, time zone `UTC` |
| App storage | `/data` (private, persistent) through `@amazon-devices/kepler-file-system`: read, write, remove, exists; **no rename, no directories API**. The built-in `AsyncStorage` is deprecated and throws on this OS ("Application data root path is not set"). |
| Crypto | `@amazon-devices/keplercrypto`: system random, native PBKDF2 (the Web Crypto layer caps output at one digest; the lower-level API gives 64 bytes) |
| Network state | `@amazon-devices/keplerscript-netmgr-lib` (a NetInfo-shaped API) |
| Service discovery (mDNS) | **None in the SDK.** |
| Playback | `@amazon-devices/react-native-w3cmedia` 2.3.2 (phase 2) |
| Remote events | One event per press through `useTVEventHandler`, with the SDK's names (`playpause`, `back`, `skip_backward`…) and `eventKeyAction` undefined for an injected short press |
| Focus | Native spatial navigation between focusable views, including inside a scaled container; the nearest candidate is judged by its center, so a wide control loses to a small one beside it. `TVFocusGuideView` (with `autoFocus`, `destinations`, `trapFocus*`), `requestTVFocus`, `hasTVPreferredFocus` exist. **A view with `hasTVPreferredFocus` asks for focus again on every update** (phase 0, Differences 4). **A guide's `autoFocus` memory outlives the control it points at**, and **`focusable={false}` on a guide does not put its contents out of reach** (phase 1, Differences 3 and 4). |
| Text entry | The system keyboard is a separate full-screen window, raised when a `TextInput` takes focus. The app must ask for it in the manifest (`[[wants.service]] id = "com.amazon.inputmethod.service"`); without that line nothing appears. Select types the highlighted key; "Go"/"Done" submits. |
| Scrolling | `ScrollView`/`FlatList` are the SDK's. **`snapToAlignment="item"` with `snapToItemPadding` (react-native-tvos) stops the app** (native abort on the UI thread, no JavaScript error). Focus moving to a control off screen scrolls it into view. |
| Animation | React Native's `Animated` works, native driver included. Two things do not: Amazon's Reanimated build (phase 1, Differences 1), and **rotating a view that has both rounded corners and border sides of different colors, which stops the app responding** (phase 1, Differences 2). While anything animates, the device log gets one "Application data root path is not set" line per frame; harmless, but it buries everything else. |
| Uncaught JavaScript errors | A Release build ends without a word in the device log. A native abort leaves a crash record (`vega device copy-logs --artifact SYSTEM_TOMBSTONE/acr`) with addresses only; the runtime libraries can be pulled from the device (`/usr/lib/libkeplerscript-4.so.4`) and their exported symbols give the neighborhood. |

### Toolchain facts that cost time (all handled by `apps/vega/scripts/vega.mjs`)

1. **Spaces in the path break the build.** The SDK's tools pass paths to a shell unquoted
   (`execSync(\`${bin} build -d ${dir}\`)`, `shelljs.exec(cmd.join(' '))`), and this checkout is under
   `Codex Projects`. The script mirrors `apps/vega`, `apps/native/{src,assets,App.tsx,package.json}`
   and `packages` to `~/.cache/portico-vega-stage` and builds there; on a path without spaces it
   builds in place.
2. **An Intel `bash` in `/usr/local/bin`** comes first on this Mac's PATH. The SDK's wrappers are
   `#!/usr/bin/env bash`, so every tool ran under Rosetta and packaging failed with "macOS x86_64
   binary not available". The script puts `/bin` first.
3. **`vega virtual-device start` kills a healthy device** when its readiness probe fails (it did
   under 2): "Deadline reached (240s) but virtual device unresponsive", then a kill signal.
4. **The virtual device and Android emulators share ports** (console 5554+, device 5555+, `adb`
   server 5037). The script starts the device with `--vda-port 5701 --gui-off` and sets
   `ANDROID_ADB_SERVER_PORT=5039` for every SDK command. Screenshots come from the emulator console
   on port 5700 (`screenrecord screenshot`), authorized with the same token file the SDK's tools use.
5. **The device's own `screenshooter` ends the developer shell**; it is not used.
6. **JavaScript console output from the app's own code does not reach `loggingctl`** in a Release
   build (the SDK's own warnings do). Diagnostics were done by drawing values on the screen.
7. **A project `resolveRequest` replaces, not wraps, the SDK's `react-native` → Vega renaming.** With
   one, libraries got stock React Native and the bundle failed; `metro.config.js` does the renaming.

## What the shared screens need from React Native (handoff §4)

"SDK" = provided by `@amazon-devices/react-native-kepler` or an Amazon build of the library. Versions
are what `vega project install` resolves for OS 1.2.

| Needed | On Vega | In `apps/vega` |
|---|---|---|
| `Text`, `VirtualizedList`, `Image`, `Switch`, `Animated`, `Easing`, `PanResponder`, `AppState`, `Linking`, `Share`, `BackHandler`, `Keyboard`, `KeyboardAvoidingView`, `AccessibilityInfo`, `RefreshControl`, `StatusBar`, `registerCallableModule` | SDK | Re-exported unchanged |
| `Platform` | SDK; `OS` is `'kepler'`, `isTV` true on the TV profile, `Version` numeric | Unchanged. Every `Platform.OS === 'ios'` branch in the shared code is correctly false. |
| `NativeModules` | SDK's is the legacy bridge and throws | Shim: Portico's modules, then `TurboModuleRegistry.get` |
| `NativeEventEmitter` | SDK | Shim: a Portico module's own events, else the SDK's |
| `requireNativeComponent`, `UIManager.hasViewManagerConfig` | SDK | Shim: Portico's views (`PorticoPlayer`, `PorticoBackdrop`; the route picker and volume view draw nothing) |
| `Pressable` | SDK | Shim: `hasTVPreferredFocus` as one request for focus (phase 0, Differences 4); `tvParallaxProperties` dropped; tells the focus guides and snapping scroll views around it when it takes focus (phase 1) |
| `useTVEventHandler` | SDK, other names | Shim: Apple TV names (`playPause`, `menu` for Back, `rewind`, `fastForward`), one event per press, `long…` for a held key |
| `TVEventControl` | Absent (tvOS only). **The shell does not guard it** (`Shell.tsx:373`, `DirectProfiles.tsx:48`), contrary to the handoff. | Shim: methods that do nothing |
| `TVFocusGuideView` | SDK (`autoFocus`, `destinations`, `trapFocus*`, `enabled`) | Shim (`focus.tsx`): `focusable={false}` puts the contents out of reach; `autoFocus` chooses its destination from the controls in reach now (phase 1, Differences 3 and 4) |
| `TextInput` | SDK | Shim (`text-input.tsx`): `enablesReturnKeyAutomatically` without the SDK's failure; a field's hidden input exists only while it is typed into (phase 1, Differences 5) |
| `ScrollView`, `FlatList`, `View` | SDK | Shim (`snap.tsx`): the tvOS focus-following snap (`snapToAlignment="item"`, `snapToItemPadding`, `scrollSnapOffset`, `scrollSnapAlign`) done from focus events (phase 1, Differences 6) |
| `requestTVFocus`, `hasTVPreferredFocus` | SDK | Used as is (through the shimmed `Pressable`) |
| `useWindowDimensions`, `Dimensions`, `PixelRatio`, `StyleSheet.hairlineWidth`, `Modal` | SDK, on a 960×540 @2 canvas | Shim: the 1920×1080 design canvas (Differences 1) |
| `@react-navigation/native` 7.4.1 (Apple) | Amazon build of 7.1.22 | Aliased in Metro and `tsconfig` |
| `@react-navigation/native-stack` 7.19.1 | Amazon build of 7.8.1 | Aliased |
| `react-native-screens` 4.23.0 | Amazon build `3.0.9000000001` (of 3.25); **not in the OS 1.2 profile** but required by native-stack and works | Installed pinned; aliased |
| `react-native-safe-area-context` 5.5.x | Amazon build of 5.2.0 | Aliased |
| `react-native-reanimated` 4.6.0 + `react-native-worklets` 0.12.2 | Amazon builds of 4.1.3 and 0.6.0 exist | **Not installed.** Shim (`reanimated.tsx` over `motion.ts`): the part of the API the shared code uses, stepped in JavaScript (phase 1, Differences 1) |
| `react-native-svg` 15.15.5 (through `metro/svg-lite.js`) | Amazon build of 15.11.1 | Aliased to the package itself (the Apple app's size shim is not used; bundle size is not a constraint here) |
| `react-native-qrcode-svg` 6.3.22 | Amazon build exists but is not in the OS 1.2 profile | The community package (plain JavaScript) over the Amazon SVG build |
| `react-native-url-polyfill` | Not built for Vega; reads `NativeModules` as it loads | The community package, resolved against the shim so that read is safe |
| `@bacons/text-decoder` | Plain JavaScript | Installed as is |

`npx tsc --noEmit` over `apps/vega` **and every shared source file** passed with one error on the
first run (`tvParallaxProperties` on `Pressable`), fixed by a type declaration in `apps/vega/types`.

---

## Phase 0 — SDK facts and a shell that builds

### Built

- `apps/vega` from the SDK's template: `package.json`, `manifest.toml` (id
  `tv.getportico.portico.vega`, component `tv.getportico.portico.vega.main`), `app.json`, `index.js`,
  `babel.config.js`, `metro.config.js`, `tsconfig.json`, `README.md`, `strings-baseline.json` (empty).
- `metro.config.js`: watches `../native` (sources and assets only) and `../../packages`; resolves
  `react-native` to the shim for everything outside `node_modules`; aliases community package names
  to the Amazon builds; replaces a shared file by the file at the same path under `src/overrides`.
- `src/shims/react-native.ts`, `canvas.tsx`, `pressable.tsx`; `src/Root.tsx`.
- `src/bridge/`: `runtime.ts` (**PorticoRuntime**, complete), `transport.ts` (the bounded HTTP
  transport), `storage.ts` (records), `profile-trust.ts` (the trust vault, a port of
  `PorticoProfileTrust.h`), `kdf.ts`, `network.ts` (**PorticoNetwork**: `sha256`, `verify`,
  `tvPublicKey`, `openTVGrant`, `forgetTVKey`, monitoring; discovery answers "nothing found"),
  `modules.ts` (**PorticoCapabilities** with identity facts only so far, **PorticoAccountAuth**,
  **PorticoTopShelf** as a record store, and the no-op **PorticoSiriRemote**, **PorticoActivity**,
  **PorticoCast**, **PorticoDownloads**, **PorticoImagePicker**, **PorticoSubtitleFiles**),
  `system-media.ts` and `playback.ts` (stubs that refuse to play, for phase 2), `events.ts`,
  `encoding.ts`, `random.ts`.
- `scripts/vega.mjs`: `build`, `install`, `launch`, `stop`, `run`, `key`, `type`, `shot`, `logs`,
  `device start|stop|status`, `start`, `clean`.

### Decisions where the handoff was silent

1. **`react-native` is a shim file, and overrides live at mirrored paths.** The handoff asked for a
   resolver alias and "a `.vega.ts` sibling". A sibling would have to live in `apps/native`, which
   this lane may not write, so `src/overrides/<same relative path>` is the sibling.
2. **Records are files in `/data`, two slots per record.** The file API cannot rename, so a plain
   write is not atomic. A write goes to the slot not holding the current version, with a sequence
   number and SHA-256, and is read back before the older slot is removed. Record names are the Apple
   service and account, hashed. Read-back verification applies to every record (Apple does it for
   PIN counters and the viewer selection).
3. **Cryptography in audited JavaScript** (`@noble/curves`, `@noble/hashes`, `@noble/ciphers`) for
   X25519, Ed25519, HKDF, AES-GCM, AES-CBC, HMAC and SHA-256, so the envelope formats are byte-for-byte
   the Apple ones. Random numbers come only from the SDK's generator; there is no fallback.
4. **The vault's PBKDF2 (200,000 rounds) runs natively**, behind a self-check: once per launch the
   SDK's result for a short derivation must equal the JavaScript implementation's, else JavaScript
   is used. Measured on the virtual device: 50 ms native; about a minute in JavaScript.
5. **Request identifiers are upper-case UUIDs**, as `NSUUID` writes them.
6. **`hostedUrl` comes from `app.json`** (`portico.hostedUrl`; empty means the shared default,
   `https://web.getportico.tv`), in place of the Xcode build setting.
7. **`advertiseTV`/`advertisePlayer` validate as Apple does and resolve `false`** (nothing is
   advertised); `browse` resolves an empty list.
8. **The shared `legacy-events.ts` is not imported** by the Vega entry: it is an Apple workaround
   for old-style view events.
9. **Fonts** are copied at build time from `../native/assets` into `assets/fonts` under the family
   names the design system uses (`Manrope-Regular.ttf` …); the copies are not committed.
10. **`PorticoTopShelf` keeps its record now** rather than doing nothing, so the app does not mint a
    new shelf token on every launch before phase 4.
11. **Screenshots for the log are Release builds** (a Debug build draws its warning banner over the app).

### Differences from the Apple TV app

1. **Layout canvas.** Vega lays out at 960×540 @2; the app is laid out on a 1920×1080 canvas and
   scaled (one canvas point = one pixel at 1080p). Without it everything was twice as large
   (`vega-port-shots/p0-05-unscaled-before-canvas.png`); with it the Activation screen matches the
   §5 numbers (`p0-01-activation.png`). The shim reports the canvas as the window, a pixel ratio of
   1 and a one-pixel hairline, and wraps `Modal` contents in the canvas. Focus navigation works
   inside the scaled container (checked). Libraries in `node_modules` still see 960×540.
2. **No keychain.** Records are in the app's private storage, readable by nothing else on the
   device but not hardware-protected. The TV sign-in private key and the vault's device key are
   among them.
3. **Redirects are refused after the fact.** The SDK's networking follows redirects and offers no
   way to refuse one first; the transport discards any response whose final address is not the one
   asked for. A request (with its headers) can therefore reach a redirect target before being
   refused. To be revisited in phase 1 against the SDK's network stack.
4. **`hasTVPreferredFocus`.** On Vega the native view asks for focus on every update while the prop
   is true. The shared `Focusable` re-renders on focus change, so the preferred button took focus
   back the instant it lost it and re-rendered in a loop (`p0-04-preferred-focus-loop-before-fix.png`:
   one press, dozens of focus events). Taking the prop away again does not withdraw it either. The
   shim's `Pressable` therefore never gives the native view the prop: a preference is one
   `requestTVFocus()` when the control appears, and one more each time the preference turns on.
   (Corrected in phase 1; the phase 0 commit passed the prop for the first commit only.)
5. **No nearby discovery** (no mDNS API): the TV is not advertised to a phone on the same network,
   and servers are not found automatically. Code and address sign-in are unaffected.
6. **Remote events** arrive once per press with different names; the shim translates. A held key's
   `long…` events are synthesized after 500 ms. Not yet checked with a physical remote.

### Shared changes needed

None applied. Two findings for the lead (no diff needed for Vega, since the shim covers both):

- `src/shell/Shell.tsx:373–375` and `src/screens/auth/DirectProfiles.tsx:48–49` call
  `TVEventControl.enableTVMenuKey()` unguarded. The Android handoff says the shell guards it; it
  does not. On react-native-tvos for Android the export exists, so this may be harmless there.
- `src/ui/Focusable.tsx` calls `setState` on every focus event even when nothing changed
  (`update` always builds a new object). Harmless on Apple; on any platform that re-applies
  `hasTVPreferredFocus` on update it loops. Returning `prev` when the patch changes nothing would
  make it robust everywhere:

  ```diff
  -      setState(prev => {
  -        const next = {...prev, ...patch};
  +      setState(prev => {
  +        if ((Object.keys(patch) as (keyof FocusableState)[]).every(key => prev[key] === patch[key])) return prev;
  +        const next = {...prev, ...patch};
  ```

Phase 1 will add the platform-name and capability-profile overrides, with their diffs.

### Not done, and why

- `PorticoCapabilities.facts` returns identity only; codec and HDR facts are phase 1.
- `PorticoPlaybackModule`, `PorticoPlayer`, `PorticoSystemMedia`: stubs until phases 2 and 3.
- `PorticoSearchKeyboard`: no screen uses it (Search uses `TextInput`); nothing to provide.
- Home-screen rows: phase 4.

### Functional checks (virtual device, Release build)

- The app launches to the Activation screen with a code from `web.getportico.tv` and its QR code:
  `vega-port-shots/p0-01-activation.png`. This exercises the HTTP transport, the X25519 key, storage
  and the SVG and font paths.
- The same code is shown after the app is stopped and launched again (the journal persists).
- Right moves focus to "Sign in directly to a server", Left returns: `p0-02-activation-focus-right.png`.
- Select on "Sign in with a Portico Account" opens the account form; Up and Down move between Back
  and the field; Back returns.
- On-device checks of the bridge (`p0-03-device-checks.png`): system random and UUIDs; an unknown
  module reads as undefined; a record written in one launch is read in the next; removal; the
  published PBKDF2 test vector; 200,000 rounds in 50 ms; `PorticoNetwork.sha256`.

### Builds and suites

- `npx tsc --noEmit` in `apps/vega` (with all shared sources): clean.
- `npm run lint:strings` in `apps/vega`: 0 hard-coded strings; raw-error presentation clean.
- `node scripts/vega.mjs build` and `build --release`: packages for aarch64, armv7, x86_64.
- The shared node suite (`npm test` in `apps/native`) was not run: nothing there changed.

### Commits

- `vega-port` `78511f9`: "Vega OS app: SDK project, react-native shim, bridge foundation, device tooling"

---

## Phase 1 — sign in and browse everything

### Built

- **The app signs in and browses.** Direct sign-in to the local server (`10.0.2.2:32500`): address
  typed on the system keyboard, the server's identity proof verified, a Quick Connect code shown and
  approved, capability profile published, then Home, the libraries, movie and show pages, Search,
  the rail, focus and Back.
- `src/shims/snap.tsx`: the tvOS focus-following snap for rows and pages.
- `src/shims/focus.tsx`: `TVFocusGuideView` with the Apple TV's meaning of `focusable={false}` and
  `autoFocus`, and a home for focus when the focused control goes away.
- `src/shims/text-input.tsx`, `typing.ts`: the field's hidden input, what waits while the keyboard
  is up, and a guard for an SDK `TextInput` fault.
- `src/shims/motion.ts` and `reanimated.tsx`: the Reanimated API the shared code uses
  (`useSharedValue`, `useAnimatedStyle`, `withTiming`, `withSpring`, `withSequence`,
  `interpolateColor`, `runOnJS`, `Easing`, `Animated.View`/`Text`/`Image`/`ScrollView`,
  `createAnimatedComponent`), without the library. `Animated`/`Easing` from `react-native` are the
  SDK's own again.
- `src/overrides/native/ui/Feedback.tsx`: the loading ring, drawn so that Vega can turn it.
- `src/overrides/native/bridge/{client-profile,capability-publisher,setup-codes}.ts` and
  `src/overrides/packages/client-core/src/{installation,identity-client,remote-playback}.ts`: the
  Vega capability profile, and `vega` / "Fire TV" where the shared files hard-code `tvos` /
  "Apple TV".
- `src/bridge/capability-profile.ts`, `src/platform.ts`; `PorticoCapabilities.facts` answers model,
  OS version, device name (bounded to 1.5 s) and app version.
- `src/debug.tsx`: the on-screen console (`portico.debugConsole`: `false`, `true`, `"errors"`),
  including uncaught errors, which a Release build otherwise ends on silently.
- `manifest.toml`: `com.amazon.inputmethod.service` (the system keyboard).
- `package.json`, `babel.config.js`, `metro.config.js`: Reanimated and worklets removed.

### Decisions where the handoff was silent

1. **Reanimated is not in the app** (Differences 1). The handoff lists it as required and "confirm
   the version"; Amazon's build is the wrong tool on this OS, and the shared code's use of it is
   small enough to serve from 180 lines.
2. **Spring animations ease out over 350 ms** (`withSpring`). The shared code springs only a sheet
   released from a drag, which a television never does.
3. **A field's hidden input is mounted only while it is typed into** (Differences 5), and Back while
   the keyboard is up closes the keyboard and nothing else.
4. **A guide's destination** is, among the controls in reach now: the one focused most recently
   (a history of 24); else one with the same accessible name as the last (a list that drew its
   card again); else the first that asks for preferred focus; else the first that mounted. tvOS's
   "first" is the first in the view tree; mount order is the same for a screen that appears whole.
5a. **A request for focus made under the keyboard waits** until typing ends, and the last one made
   wins (the first search result); with none, focus returns to the field.
5. **Sign-in approval was done through the server's API**, not the web page: the session's browser
   had no signed-in server page, and typing the owner password there would have put it in this
   session. A script signs the owner in, reads the pending request (`/v1/quick-connect/review`),
   approves it and signs the owner out again; it prints no secrets. The server's own description of
   the device at approval was `deviceName "Fire TV"`, `platform "vega"`, `appVersion "0.1.0"`.
   **So "approved from the web app" in the checkpoint is not what was checked.**
6. **The local server was restarted once by this lane** (04:35, with the documented
   `.scratch/dev-local.sh`): it had stopped, and nothing of mine had stopped it.
7. **Codec and HDR facts stay client-core's declared Vega table** (`vegaDeclaredProfile`) until
   phase 2 has a player to ask.

### Differences from the Apple TV app

1. **Motion is stepped in JavaScript, not on the UI thread.** With Amazon's Reanimated (4.1.3)
   loaded, styles it drove were applied late or to the wrong control: the focus ring stayed on the
   card focus had left while the scale moved on to the next. The library is out; values written
   with its API are stepped once a frame by `motion.ts`, and only the component showing a value
   renders again. Durations and curves are the shared ones. On the virtual device focus scaling,
   the ring and the rail opening look right and keys stay responsive
   (`vega-port-shots/p1-02-home.png`, `p1-03-home-rail-open.png`). **Not measured on hardware:** a
   Fire TV Stick is far slower than this Mac; if focus transitions stutter there,
   `portico.animations: false` makes them arrive at once.
2. **The loading ring is drawn differently, to look the same.** The shared ring is one rounded view
   with one border side in the accent color, rotated. On Vega, rotating a view that has both
   rounded corners and border sides of different colors stops the app responding within a second
   (either property alone is fine; so is a plain view, the skeleton pulse and focus scaling). The
   override draws a still track and a quarter-circle path that turns over it: same size, stroke,
   colors and speed. This was the cause of the "direct sign-in hangs" seen at the start of phase 1,
   which for a while was wrongly put down to animation in general.
3. **`TVFocusGuideView focusable={false}`.** On Vega the prop makes the guide's own view
   unfocusable and says nothing about its contents. The shim makes it mean what it means on tvOS:
   a `Pressable` inside is not focusable, a `TextInput` inside is not mounted.
4. **`TVFocusGuideView autoFocus`.** The SDK remembers the last focused child natively and keeps
   sending focus there after that control is hidden (the shell keeps Home alive, not displayed,
   while Search shows) or gone. Entering Search from the rail did nothing at all. The shim chooses
   the destination itself and hands it to the native guide, which then does as tvOS does.
5. **Text fields.** The shared `Field` is a button with a 1×1 real input beside it. On Vega that
   input took focus when focus moved past it, invisibly. It is now mounted only from the button's
   press until typing ends, when focus returns to the button
   (`vega-port-shots/p1-10-keyboard.png`). The keyboard itself is Amazon's full-screen grid, not the
   tvOS one; dictation is the remote's microphone button.
6. **Row and page snapping** is done from focus events, after the system's own scroll-into-view,
   rather than inside the scroll. The resting positions are the tvOS ones: a row's focused card one
   page gutter from the left (`p1-06-row-snap.png`), a marked section at its offset from the top
   (`p1-08-show-episodes.png`). There may be a visible second movement where the system's scroll
   and the snap disagree; not seen on the virtual device, to be looked at on hardware.
7. **Home's page** has no item snapping on Apple TV either (`PageList`); the vertical resting
   position after Down is the system's and will differ by some pixels from tvOS.
8. **The platform is `vega` and the device "Fire TV"** in every request and in the fallback device
   name. The web app and the server do not know `vega` yet (see Shared changes).
9. **Focus that is nowhere.** On tvOS the focus engine re-homes focus when the focused view goes
   away. On Vega nothing does: after a page was pushed, a sheet closed or the keyboard dismissed,
   the remote moved nothing, or focus fell to the rail. A screen's guide now asks for focus when it
   becomes the front screen while no control holds it (retrying for 2.5 s, since a request made
   mid-transition is silently dropped), and puts right a wrong first landing within 0.7 s. Back
   from a movie, a show, a person and the episode sheet each return to the control that opened
   them, at the same scroll position. **This is the least Apple-like part of the port in its
   mechanics** (timers where tvOS has an engine) and the first thing to watch on real hardware.
10. **Back while the keyboard is up** closes the keyboard only. Vega delivers that press to the app
    as well (twice: a remote event and a hardware Back, the second sometimes after the keyboard has
    gone); both are withheld for 0.5 s after typing ends.

### Shared changes needed

None applied; `apps/native/src` and `packages` are untouched. Each override below stands in for a
change the shared file should take, after which the override can go.

1. **One source for the platform name.** `app/session.tsx:46`, `app/device.tsx:37`,
   `app/receiver.tsx:47` and `bridge/setup-codes.ts` write `Platform.isTV ? 'tvos' : 'ios'` (and
   "Apple TV"). Suggested: a `bridge/platform.ts` exporting `platformId` and `deviceKindName`,
   which a port overrides in one file instead of four.
2. **`bridge/client-profile.ts`, `bridge/capability-publisher.ts`**: take the profile builder from
   the platform file too (Vega's is `vegaDeclaredProfile`).
3. **`ui/Feedback.tsx` `Ring`**: nothing to change for Apple. If the lead prefers one
   implementation, the track-plus-arc drawing in the override is correct on both.
4. **`ui/Focusable.tsx`**: the `setState` guard from phase 0 still stands.
5. **`screens/Home.tsx:162` with `client-core/presentation/home-hero.ts:66`** (seen on Vega, not
   Vega's): `heroDuration` returns `''` under 120 s, but the hero label uses a remaining time from
   60 s, so a title with 60–119 s left reads "Resume ·  left". Seen on Home early in phase 1; no
   screenshot was kept, and the hero item has since moved past that window.

   ```diff
   -    playLabel: left >= 60 ? t('entry.resumeRemaining', {remaining: options.duration(left)}) : t(progress > 0 ? 'entry.resume' : 'action.play'),
   +    playLabel: left >= 60 && options.duration(left) ? t('entry.resumeRemaining', {remaining: options.duration(left)}) : t(progress > 0 ? 'entry.resume' : 'action.play'),
   ```
6. **The events long poll outlives its transport** (seen on Vega, not Vega's): `EventsClient` asks
   `/v1/events?waitSeconds=25`, and the native transport's limit is 15 s on Apple
   (`PorticoPlayer.m:122`) as here, so every quiet poll ends as a transport error after 15 s and is
   retried. Events still arrive. Suggested: `waitSeconds: 10` where the client is built
   (`bridge/useNativePlayback.ts:98`).
7. **`web` and the server**: a label for platform `vega` ("Fire TV") in the devices list (Settings
   shows the raw word "vega" under the device, `p1-15-settings.png`), and `vega` in the feedback
   platform list (`feedback.go`), when the lead merges.
8. **A failed restore at launch signs the device out at the server** (read from the code, and
   consistent with two sign-outs seen here; not reproduced on purpose). `resumeSaved` restores the
   saved sign-in through `ViewerSelectionCommit.commit`. At launch there is no current viewer, so
   when the commit fails for any reason after it starts (its 20 s limit, `/v1/me` answering an
   error once, a storage write failing), its `finally` treats the saved session as an unused
   candidate and calls `revoke`, which is `logout()`: `DELETE /v1/sessions/current`. The server
   logs `authorization family revoked … reason=explicit_signout`, and the next launch shows the
   sign-in screen. The comment at `session.tsx:483` says the opposite is intended ("Nothing is
   discarded here"). Here it happened at 04:45:01 and 05:14:08, both launches in the 35 minutes
   when the turning ring could stall the app (Differences 2); none in the 20-odd launches since.
   Suggested, in `app/session.tsx`:

   ```diff
              revoke: async record => {
   +            // A saved sign-in being restored is not an unused candidate: failing to restore it must not end it.
   +            if (established && record.session.accessToken === candidate.session.accessToken) return;
                try {
   ```
   On Apple this would need a stalled launch or one failed `/v1/me`; worth the lead's look.

### Not done, and why

- **The Dashboard pane was opened but not walked** (focus was not moved into its cards), nor the
  other Settings sections, Music, Anime, Live TV or the Audiobooks library's own screens.
- **Two focus rings at once were seen** (`p1-17-two-focus-rings.png`: an audiobook page reached
  by an unplanned key sequence, Resume and the bookmark button both ringed, and still so two key
  presses later). A deliberate repeat from a fresh launch on a movie page showed one ring moving
  cleanly (Resume → bookmark → heart). Not explained, not reproduced on purpose; to watch for.
- **Password sign-in and Portico Account sign-in were not exercised** (the first needs the owner
  password typed in this session, the second a real account).
- **Deep links** (`portico://item/<id>`): not looked at yet.
- **Redirect refusal** (phase 0, Differences 3): unchanged.
- **A physical remote**: the virtual device's keys are injected; long-press timing is unverified.

### Functional checks (virtual device, Release build)

- Direct sign-in: address typed, code shown (`p1-01-direct-code.png`), approved, Home.
- Home: hero with backdrop, eyebrow, title, facts, progress, Resume and Details; Continue Watching
  and Recently added rows; focus ring, scale and glow on the focused card (`p1-02-home.png`).
- Rail: opens with labels and pushes the page (`p1-03-home-rail-open.png`), closes when focus
  returns to the page.
- Libraries: Movies (`p1-04-library-movies.png`), TV Shows (`p1-09-library-tv.png`), Audiobooks;
  tabs, shelves, row snapping (`p1-06-row-snap.png`).
- Movie page (`p1-05-movie-page.png`); Back returns to the same card at the same scroll position.
- Show page (`p1-07-show-page.png`), episode list with the still and details pane
  (`p1-08-show-episodes.png`).
- Search (`p1-12-search-results.png`): Right from the rail lands on the field; Select raises the
  keyboard (`p1-10-keyboard.png`); typed text searches as it is typed; "Search" on the keyboard
  closes it with focus on the first result; Back closes it with focus on the field and the page
  still there; Down reaches the chips and the results; a result opens and Back returns to it.
- A person page from Search (`p1-14-person-page.png`).
- Saved with its tabs and sort chips (`p1-13-saved.png`); Settings, Profile & account with the
  device list (`p1-15-settings.png`); the Dashboard pane (`p1-16-dashboard.png`).
- The episode sheet from Home's Continue Watching: opens with focus on Resume, Down moves within
  it, Back closes it onto the show page's Resume, Back again returns Home onto the same card.
- The session survives stopping and starting the app, installing a new build over it, and six
  minutes stopped. (Two sign-outs earlier in the phase: Shared changes 8.)

### Builds and suites

- `npx tsc --noEmit` in `apps/vega` (with all shared sources): clean.
- `npm run lint:strings` in `apps/vega`: clean.
- `node scripts/vega.mjs build --release`: packages for aarch64, armv7, x86_64.
- The shared node suite was not run: nothing shared changed.

### Commits

- `vega-port` `a9972e0`: "Vega OS app: sign-in and browsing; focus, snapping, text entry and motion shims"

---

## Phase 2 — video

### Built

- `src/bridge/playback.ts`: **PorticoPlaybackModule** (`acquire`, `command`, `release`) and the
  **PorticoPlayer** view over the SDK's W3C media element, with the Apple controller's contract:
  one lease, full-state commands validated in the Apple order and words, facts with the binding,
  a rising sequence and the last command revision. Facts: `ready`, `firstFrame`, `progress` (every
  0.5 s), `rateStatus`, `stalled`, `seekApplied`, `seekFailed`, `ended`, `error` (with
  `networkError`, `engineCode`, `errorDetail`), `subtitleStatus`, `audioMapped`, `audioObserved`,
  `audioApplied`, `audioUnavailable`, `audioFailed`. Rate facts as Apple's (`supportedRates: [1]`).
- `src/bridge/hls.ts`: an HLS client over media source extensions, for what the Portico server
  writes (one fMP4 video variant, audio renditions in MPEG-TS or fMP4, VOD or growing playlists).
- `src/bridge/subtitles.ts`: the server's subtitle document, cue lookup, and the label the view
  draws, laid out as the Apple view's.
- `src/bridge/system-media.ts`: **PorticoSystemMedia** (`setVideo`, `clear`, `currentRoute`; event
  `PorticoSystemMediaCommand`), fed by the system's media-control requests.
- `capability-profile.ts`: transports by origin (HTTP: `hls_fmp4` only).
- `manifest.toml`: the media services, the media category and the playback-server interface.
- `transport.ts`: a request that asks the server to wait is given that long.
- `debug.tsx`, `scripts/debug-sink.mjs`: trace lines posted to the development machine.
- The key names seen while something plays (`play`, `forward`, `enter`) mapped to tvOS names.

### Decisions where the handoff was silent

1. **The handoff says "W3C media for playback"; it does not say the element cannot open a Portico
   server's address.** On Vega OS 1.2 the element refuses `http:` outright (device log:
   "isUriSchemeSecure Got an insecure protocol/scheme http, return error"), and it has no HLS
   client for any scheme. So everything from a LAN server goes through media source extensions
   with a client of the app's own. Amazon's route is a patched Shaka Player; I wrote a small
   client for the server's own output instead (300 lines, nothing to download or patch, no
   adaptive logic to disagree with the server's plan). It is not a general HLS player.
2. **From an HTTP origin only `hls_fmp4` is declared**, so the server remuxes or transcodes
   everything into HLS for this client, files included. From HTTPS, `direct` is declared too and a
   file plays from its address. `hls_ts` is never declared. **(Wrong, and corrected in phase 3,
   Decisions 2: without `hls_ts` nothing that needs converting plays.)**
3. **The stream's clock.** The server's segments start one second in (video fMP4 and audio TS
   alike), and a media source keeps the timestamps it is given. The offset is measured from the
   first video segment appended and removed from every position reported, added to every seek.
4. **The audio renditions are MPEG-TS** (`audio-0-000000.ts`) beside fMP4 video. The media source
   accepted a `video/mp2t` buffer for them, so nothing is transmuxed.
5. **`stalled` is reported only when playback runs dry**, not while a seek or the first picture is
   awaited (the element says "waiting" for all three; Apple's stall notification means the first).
6. **A seek to where the element already is** is answered at once (the element says nothing).
7. **The transport keys.** With the media category declared, the system asks the app (through the
   element's control handler) instead of acting on the element; each request becomes a
   `PorticoSystemMediaCommand`. Fast Forward and Rewind are sent as `skip` by the amounts in
   `setVideo` (the profile's), as Apple's remote commands are. Play/Pause also arrives as a key,
   which the shared player already handles and de-duplicates.
8. **The events long poll** (phase 1, Shared changes 6) is now given its 25 s by the transport:
   left alone, each 15 s failure flipped the route to "verifying" and back, and the player paused
   and resumed whatever was playing. The shared change is still worth making for Apple.
9. **Trace by network.** The player cannot be photographed on the virtual device (below), so
   `portico.debugSink` posts the debug lines to the development machine. Off unless configured;
   nothing ships with it set.

### Differences from the Apple TV app

1. **No speed control.** The element has no playback rate. `supportedRates` is `[1]`; asking for
   another rate gets Apple's "This speed is unavailable…" in `rateError`.
2. **Every title from an HTTP server is HLS**, so nothing is played from the file directly: more
   work for the server (a remux at least) than for an Apple TV on the same network.
3. **Subtitles are measured against the view, not the picture inside it**: identical for a
   picture as wide as the screen, 5% of the bars off for a narrower one. System font, as Apple's.
4. **No AirPlay, no picture in picture, no external route**: `currentRoute` is always "none" and
   `PorticoPictureInPicture` is never sent. The shared player's PiP button is still drawn (it is
   in the screenshots); it does nothing here. It should be hidden for this platform (Shared
   changes 1).
5. **Audio decode**: nothing beyond the declared table; AC-3 and E-AC-3 in TS through the media
   source are untried.

### Shared changes needed

1. `player/VideoPlayer.tsx`: the picture-in-picture button should be hidden where the platform
   has none (it keys on `Platform.isTV`/iOS today; Vega needs a capability, for example
   `UIManager.hasViewManagerConfig('PorticoPictureInPicture')` or a bridge constant).
2. The speed control should be hidden when `supportedRates` is a single rate ("hide only disabled
   features"). Not checked whether it already is: the player could not be photographed.

### Not done, and why

- **The picture was never seen.** From the first video frame the virtual device's screenshot
  (emulator console) returns the same frozen image until the app is restarted, even after the
  player is closed; the on-device `screenshooter` fails with a permission error. The app is not
  frozen: its trace, the remote's keys and the media pipeline's log all carry on. So: no
  screenshot of video, of the player's controls over video, or of the options sheets. **To
  check:** start the device with its window (`vega virtual-device start`, without the script's
  `--gui-off`) or use a Fire TV, play a film, and look.
- **Subtitles chosen in the player, audio track changes, the Quality sheet, post-play and the
  next-episode countdown were not exercised**: they need the player's sheets, which I could not
  see to steer. Subtitles were checked below the player (Functional checks). In every command the
  shared player sent, `audio` was null, so the audio mapping code has not run at all.
- **Seeking by scrubbing** (Left/Right on the bar) was not tried; skip and resume were.
- **HEVC, HDR, surround, a transcode in progress (the 503 retry), live TV (growing playlists),
  an HTTPS origin (direct files)**: written for, not run. The dev library is H.264/AAC.
- **The redirect check** (phase 0, Differences 3) does not cover the HLS client's own requests.

### Functional checks (virtual device, Release build; by trace unless a file is named)

- Interstellar from Home: session created, lease acquired, HLS opened, clock offset 1.000 s,
  `ready`, resume seek applied at 203.53 s, `firstFrame` 0.7 s later, `progress` in real time
  (208.6 → 226.2 over 17.6 s). The device log shows `keplervideosink` and `novaaudiosink` running.
- A title from the beginning: `seekApplied` at 0, `firstFrame`, progress from 1.45 s.
- Play/Pause on the remote: `system media toggle`, key, command `paused=true`, fact `paused`;
  again: resumed.
- Fast Forward: `system media skip`, command with a seek +30 s, `seekApplied` at 42.40.
- The end of a 45 s title: `ended` at 45.00, command `paused=true`, the session and queue updated.
- Back from the player: `release`, and the next launch is signed in on Home.
- Subtitles, in a probe screen with the player view and a sample document served from the Mac:
  fetched, cues chosen by time, drawn at the bottom (`p2-01-subtitle-cue.png`, two lines, italic)
  and at the top (`p2-02-subtitle-top-cue.png`); nothing between cues.

### Builds and suites

- `npx tsc --noEmit`: clean. `npm run lint:strings`: clean. Release package built.
- The shared node suite was not run: nothing shared changed.

### Commits

- `vega-port` `470cd34`: "Vega OS app: video playback over the W3C media element, HLS through media source extensions, subtitles, transport keys"

---

## Phase 3 — listening

### Built

- `src/overrides/native/bridge/native-listening-kinds.ts`: no kind is handed to a native listening
  owner on Vega. Music and audiobooks play through `PorticoPlaybackModule` like video, and the
  shared queue player (`V1QueuePlayer`) keeps their timeline, lease and queue itself.
- `hls.ts`: sound-only streams (one track, MPEG-TS or fMP4), and a stream reopened at its end.
- `capability-profile.ts`: `hls_ts` is declared again (see Decisions 2).
- `debug.tsx`: the server's error code and words for a refused request; the sink gets every line
  while the overlay shows only errors.

### Decisions where the handoff was silent

1. **No listening owner.** The Apple owner (`PorticoListeningOwner`, 1,455 lines) exists so that
   listening survives JavaScript being suspended, and to make gapless and crossfade edges with a
   second render engine. A Vega app's JavaScript runs while it plays and there is one media
   element. The shared code already plays a listening session in JavaScript when no owner takes it
   (`v1-queue-player.ts:377`), so that path is used: one override, no port of the owner. The
   `listening…` methods of `PorticoPlaybackModule` exist with the Apple names and refuse or do
   nothing; none is called.
2. **`hls_ts` must be declared.** The server converts only into HLS-TS (`delivery_plan.go`, route
   4: music, and any video that needs transcoding); with only `hls_fmp4` every song answered
   `422 unsupported_media`. Phase 2 had dropped `hls_ts` on the reading that media source
   extensions take MP4 only; they take MPEG-TS here. Corrected.
3. **The dev profile's "autoplay next" was off** (`autoplayNext: false`, revision 13), so an album
   stopped after each track, correctly. For one test I turned it on through the API and put it
   back (it is now revision 15, off). Nothing else on the server was changed.

### Differences from the Apple TV app

1. **No gapless, no crossfade, no loudness normalization** (§18): `features.gapless` and
   `crossfade` are declared false and `audioDecode` is empty, so the server plans listening as
   ordinary playback. **Measured gap between tracks: 0.61 s** on the virtual device against the
   local server (`ended` at 89.96 s, next track's command at 90.18, `ready` at 90.46, first sound
   at 90.57). On a Fire TV Stick over Wi-Fi expect more.
2. **No speed for audiobooks** (the element has no playback rate): the book rate preference has no
   effect, and the Speed control should not be offered (phase 2, Shared changes 2).
3. **Everything is converted to AAC in HLS-TS** from an HTTP server, FLAC included (no original
   quality: the element will not open the file's address, and §18.1's decode plan needs an audio
   engine of the app's own).
4. **"Next"/"Previous" asked of the system** (a voice request; Fire TV remotes have no such keys)
   does nothing: `PlaybackService.systemListeningAction` only addresses a native owner. The
   player's own Next and Previous buttons use the queue and work the same either way.
5. **Listening stops when the app is no longer in front**, as video does. Apple TV music carries
   on behind the screen saver; whether a Vega app may keep playing in the background was not
   looked into.

### Shared changes needed

1. `client-core` `PlaybackService.systemListeningAction` (`index.ts:822`): fall back to the queue
   controller when there is no native owner.

   ```diff
   -  systemListeningAction(action:'next'|'previous'){return this.nativeV1?.advance(action)??Promise.resolve();}
   +  systemListeningAction(action:'next'|'previous'){
   +    if(this.nativeV1)return this.nativeV1.advance(action);
   +    const queue=this.getQueueController(),header=queue?.getSnapshot().queue;
   +    if(!queue||!header)return Promise.resolve();
   +    return action==='next'?queue.next(header.id,header.revision):queue.previous();
   +  }
   ```
   (Written from the signatures read; not compiled.)
2. `bridge/native-listening-kinds.ts`: the list of native kinds should come from the platform file
   proposed in phase 1 (Shared changes 1), empty where there is no owner.

### Not done, and why

- **The listening screen was not seen while anything played.** The virtual device's screenshots
  return stale frames (sometimes older than the one before) from the moment any media plays, sound
  alone included, until the app is restarted. `p3-01-listening-player-not-following.png` is one
  such frame, kept as the evidence: it shows "Getting your queue ready…" while the trace of the
  shared service says `phase=ready intent=playing` and positions advance. So the player's state
  on screen (position, Pause icon, artwork) is unverified by eye, here and in phase 2.
- **Audiobooks** were not played after the change (the earlier attempt, before it, failed at the
  hand-over as expected). Same path as music; resume position and chapter timers are the shared
  code's. **Shuffle, repeat, the queue list, the sleep timer, the mini player** were not exercised.
- **A stale session from my own earlier failure** (`ps_D_GHiM`, answered `410 session_ended` at
  every launch, with the player then saying "Finishing a previous playback before starting
  again") went away only when the app's data was cleared. I did not find what should retire a
  journal entry whose session the server says has ended; if it is nothing, it is a shared bug
  (the journal is the shared code's). Worth one look: `PorticoRuntime.saveJournal`'s reader.

### Functional checks (virtual device, Release build, by trace)

- An album from Music (`p3-00-album-page.png`): queue created (201), session created, HLS-TS
  opened, `ready`, `firstFrame`, positions in real time for a minute, `ended` at 59.98.
- With autoplay on: the next track by itself, 0.61 s later, and so on.
- With autoplay off: stops at the end of the track (the preference's meaning).
- The shared service through it all: `phase=ready intent=playing`, every fact accepted.
- A fresh install: sign in, approve, play music. No sign-out, no error.

### Builds and suites

- `npx tsc --noEmit`: clean. `npm run lint:strings`: clean. Release package built.

### Commits

- `vega-port` `b57fa88`: "Vega OS app: music and audiobooks through the same player; HLS-TS and sound-only streams"

---

## Phase 4 — home screen, parity pass, package

### Built

- Nothing new in code. `README.md`: what the platform does not provide.

### Decisions where the handoff was silent

1. **No rows on the Fire TV home screen.** What the SDK offers is content personalization
   (`@amazon-devices/kepler-content-personalization` 1.3.3: entitlements, customer lists, playback
   events), which reports what a customer watches to Amazon, by Amazon catalog identifier, for
   Fire TV's own Continue Watching. A personal library has no catalog identifiers, and reporting
   viewing to a third party is against Portico's stance. Not integrated; `PorticoTopShelf` keeps
   the shared code's record (so no token is minted per launch) and draws nothing. Question 4.
2. **"A fresh virtual device"** was a fresh install on the same virtual device (the app
   uninstalled, so no data): a second device would have needed its own ports and image, for the
   same evidence.

### The parity pass (Android handoff §7, television journeys)

| # | Journey | On Vega |
|---|---|---|
| 1 | Cold start, signed out, code sign-in, Home with hero and rows | Seen (`p1-01`, `p1-02`). Approval through the API, not the web page (phase 1, Decisions 5). |
| 2 | Rail to every destination; Back at each depth | Home, Search, Saved, Movies, TV Shows, Music, Settings seen; Anime, Audiobooks' own screens and Live TV not opened. Back: title page → the card it came from; sheet → its opener; section root → the rail (`p4-03`, and the rail in the frame after). Back on the rail (leaving the app) not tried. |
| 3 | Library: Discover, Browse, sort, a filter, a genre chip, a card's More menu | Discover and Browse seen (`p1-04`, `p4-01`); Sort sheet opens on the current choice, applies, returns focus to its button (`p4-02`, `p4-03`). Filters, genre chips and the More menu (a long press, which my key tool cannot make) not tried. |
| 4 | Movie page; Play; player chrome, option boxes, subtitles, Playback info, Sleep; back; progress on the card | Page seen (`p1-05`). Play, pause, skip, end: by trace. Chrome and option boxes: not seen. Progress bars are drawn on the Continue Watching cards; not compared before and after a play. |
| 5 | Show page, episode list, play an episode, Up Next | Page, list and the episode sheet seen (`p1-07`, `p1-08`). Playing an episode and Up Next not tried. |
| 6 | Music: album, play, mini player, Up Next | Album page seen (`p3-00`); play and the next track by trace. Mini player and the queue list not seen. |
| 7 | Search with the platform keyboard; Saved; Settings; Dashboard | Seen (`p1-10`, `p1-12`, `p1-13`, `p1-15`, `p1-16`). Favorites tab and the other Settings sections not opened. |
| 8 | Sign-in ended by the server: the sign-in screen with the server filled in | Seen twice, unplanned (phase 1, Shared changes 8): the sign-in screen came up with the server's address filled in. Not provoked on purpose (it would have ended other lanes' sessions on the shared dev account). |

Differences found in the pass and fixed are in the phases above. None was found in the pass
itself that is still open, apart from what could not be seen.

### Not done, and why

- **Everything listed as "not seen" or "not tried" in the table.**
- **A physical Fire TV**: remote timing (the 500 ms hold for a long press is synthesized), real
  decode (HEVC, HDR, Dolby audio), performance of JavaScript-thread motion, memory.
- **A Portico Account (Hosted) sign-in**: the activation screen and its code were seen in phase 0;
  no account was signed in.
- **Signing, store listing**: out of scope by the handoff.

### Functional checks

- Release package: `node scripts/vega.mjs build --release` → `build/{aarch64,armv7,x86_64}-release/portico-vega_<arch>.vpkg` (3.6 MB each).
- Fresh install (app uninstalled first), direct sign-in, approval, an album played: no error.

### Builds and suites

- `npx tsc --noEmit` in `apps/vega` (with every shared source): clean.
- `npm run lint:strings` in `apps/vega`: 0 hard-coded strings; raw-error presentation clean.
- `npm test` in `apps/native`: **217 tests, 217 pass.** This worktree's `apps/native` has no
  `node_modules`, so for the run a link to the main checkout's was made and removed afterwards
  (without it, 7 files fail on `react` and `rolldown` not being found; 191 pass). `git status`
  in `apps/native` is clean.

### Commits

- `vega-port` `6cf74b9`: "Vega OS app: note what the platform does not provide"

---

## Questions for Justin

1. **Who looks at playback?** I could not (For Justin first, 2). Do you want to run the virtual
   device with its window, or is there a Fire TV to put the package on?
2. **Speed and gapless.** Vega's media element offers neither. Accept the difference for this
   platform, or is an audio engine of the app's own (decoding PCM in JavaScript or a native
   module, as §18.1 expects) worth a later phase? It would also restore original-quality music.
3. **HTTP and files.** From a LAN server everything is HLS. If Portico servers are to be reached
   over HTTPS on the LAN one day, files play directly with no further work here.
4. **Fire TV home screen.** Not integrated, because it means reporting viewing to Amazon by
   catalog identifier. Is that the decision you want recorded, or should it be looked at again
   when (if) Amazon offers something local?
5. **Storage without a keychain** (phase 0, Differences 2): acceptable as is, or should the
   records be sealed with a key from the device's key service if a public API for it appears?
6. **The shared changes** (each phase lists them): the platform file in place of hard-coded
   `tvos`, hiding picture in picture and Speed where a platform has none, the `Focusable` state
   guard, `systemListeningAction` without an owner, the events long poll, and the launch-restore
   sign-out. I applied none, as told; the overrides in `apps/vega/src/overrides` stand in for the
   first two and can go when they land.
7. **Apple TV reference shots**: can a tvOS simulator runtime be installed again so the §6 screens
   can be photographed into `Audits/tv-reference-shots/`?
8. **Lane isolation**: the Android lane's tools and the Vega virtual device share default ports.
   Mine avoids theirs; if their scripts ever "kill all emulators" it will still stop the Vega
   device. Worth a line in their notes.
