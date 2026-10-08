# Fire TV Vega OS port — handoff (3 Oct 2026)

For the agent building Portico for Amazon's **Vega OS** (the Linux-based Fire TV operating system that
replaces Android on new Fire TV devices). Read this whole file, then the Android handoff
(`2026-10-03-android-port-handoff.md`, §0–§2 and §4 for the native contract this lane shares), the app's
README (`apps/portico-react-native/apps/native/README.md`) and `06 Clients.md`. Two other agents work at
the same time: Android/Android TV/Fire OS in `apps/native` (branch `android-port`) and Roku in
`apps/roku` (branch `roku-port`). The previous lead (Fable) reviews your work on return, next to the
Apple TV app, which is the oracle for every television client.

## 0. What this is

Vega OS apps are React Native apps built with Amazon's Vega SDK: Amazon's own React Native distribution
and toolchain (a CLI, a package format, a virtual device for Linux and macOS), with playback through
W3C media interfaces rather than a native player module. Portico's screens are already one React Native
code base (`apps/native/src`), so the job is to run **those same screens** on Vega with a Vega-specific
bridge, and to end with an app that looks and behaves like the Apple TV app.

The exact SDK version, its React Native version and which React Native APIs it carries are facts you
must establish on day one from Amazon's developer documentation (developer.amazon.com, "Vega",
"React Native for Vega") and the installed SDK — this file was written without that, and says so where
it matters. Record what you found at the top of the log.

## 1. Justin's rules (non-negotiable)

Rules 1–12 of the Android handoff §1 apply word for word (no subagents; almost no tests; never idle; no
AI attribution or model names; pre-release, structural fixes; one UI, screens never know the platform;
identical module names, methods and events; US English source text; scale and correctness rules; native
text input only; review your own diff; decide and log). Two more for this lane:

13. **Visual parity with the Apple TV app is the goal**, not "a Vega app that works". Same rail, same
    rows, same card sizes and captions, same focus ring and scale, same player chrome. Where Vega's
    React Native cannot draw something the same way, draw the closest thing and record the difference
    with a screenshot of each.
14. **Do not modify `apps/native/src`.** You read it; you do not write it. If a shared file must change
    for Vega, put the exact change in the log under "Shared changes needed" with the diff, and work
    around it locally (a `.vega.ts` sibling, a shim module in `apps/vega`) in the meantime. The lead
    applies shared changes when merging the lanes.

## 2. Decisions already made (apply exactly)

- **Where it lives:** `apps/portico-react-native/apps/vega`, a Vega SDK project of its own (its own
  `package.json`, the SDK's React Native, its Metro config), whose Metro `watchFolders` include
  `../native/src` and `../../packages`, with resolver aliases so `react-native` resolves to the SDK's
  package for both the app and the shared screens. The shared screens are imported, never copied.
- **The bridge:** `apps/vega/src/bridge/` provides every module the shared `src/bridge` expects
  (`NativeModules.PorticoRuntime`, `PorticoNetwork`, `PorticoCapabilities`, `PorticoPlaybackModule` +
  the `PorticoPlayer` view, `PorticoSystemMedia`, `PorticoSearchKeyboard`, and no-op `PorticoActivity`,
  `PorticoSiriRemote`, `PorticoCast`, `PorticoDownloads`, `PorticoTopShelf`, `PorticoImagePicker`,
  `PorticoSubtitleFiles`, `PorticoAudioRoutePicker`, `PorticoBackdrop`) with the method names, promise
  shapes and event names listed in the Android handoff §4. On Vega these are JavaScript or Vega-native
  implementations, whichever the SDK offers: records through the SDK's secure storage, HTTP through its
  fetch/XHR (redirects refused where the Apple module refuses them), discovery through its mDNS API if
  one exists (otherwise the module answers "no results" and sign-in is by address and code, as the
  Apple TV does without a network), and playback through W3C media.
- **Playback: W3C media.** client-core already declares the platform:
  `vegaDeclaredProfile` in `packages/client-core/src/client-profile.ts` (family `vega`, engine
  `w3cmedia`). The video element plays what the server sends a television: HLS (TS and fMP4) and direct
  play; text tracks for subtitles; the capability profile from what the device reports (codecs, HDR,
  audio channels) merged into that declared table as `appleProfile` does for Apple. The lease/command/
  fact vocabulary the JS player sends and expects is in `apps/native/src/bridge/NativePlaybackCoordinator.ts`
  and `useNativePlayback.ts`; implement `PorticoPlaybackModule` over the media element so
  `src/player/VideoPlayer.tsx` runs unchanged.
- **Listening (music, audiobooks):** the same media element behind the listening half of
  `PorticoPlaybackModule` (queue, prepare-next/commit-next per Playback Protocol v1 §18). Gapless is
  whatever the element gives; log the measured gap. The device's media session (lock screen does not
  exist on a television; the remote's play/pause keys do) through the SDK's media session API if there
  is one.
- **Sign-in:** code flow only (the TV shows a code; a phone or computer approves it at
  web.getportico.tv/device or on the owner's server), and direct sign-in by address, as on Apple TV.
  No providers on the television.
- **Home screen integration** (Vega's equivalent of Top Shelf / Watch Next): phase 4, through whatever
  the SDK provides; a no-op module until then.
- **No store listing, no signing, no Fire OS** (that is the Android lane's `firetv` flavor).

## 3. Where things are, and staying out of the other lanes' way

- Root `/Users/justin/Documents/Codex Projects/Portico/` (Mac) or wherever Justin clones it on Linux.
  `apps/portico-react-native/apps/native` is the app (shared `src/`, Apple `Native/`, `ios/`, `tvos/`;
  the Android lane adds `android/`). `apps/portico-react-native/packages` is the copy of `client-core`,
  `design`, `i18n` (originals in `apps/portico-server/packages`; `node scripts/shared-packages.mjs
  --write` refreshes the copy).
- **Your worktree and branch:**
  `cd apps/portico-react-native && git worktree add "../../.worktrees/portico-react-native-vega" -b vega-port frontend-rework`.
  Work only under `apps/vega` there (plus the SDK's generated files inside it). Never touch
  `apps/native` or `apps/roku`. Commit in the worktree with
  `git -c user.name="Justin Ehler" -c user.email="justin@MBP.local" commit -m "…"`, at least once per
  phase. Write your log in portico-internal (`Audits/2026-10-03-vega-port-log.md`) and leave it
  uncommitted; the lead commits it.
- **Local server for testing:** as the Android handoff §3 says (`.scratch/dev-local.sh`, 127.0.0.1:32500,
  the owner test account in `.scratch/DEV-ACCOUNT.md`, never printed). From the Vega virtual device the
  host address depends on the SDK's networking; find it and write it down.
- **Toolchain:** the Vega SDK (Linux or macOS), Node 22, the Vega virtual device. Record the versions.
- **The reference:** the Apple TV app in the tvOS simulator ("Portico TV"); screenshots of it for every
  screen in §6 go into `portico-internal/Project Architecture/Audits/tv-reference-shots/` (the Roku
  lane uses the same folder; if the shots are already there, use them, don't retake). On Linux without
  the simulator, ask for them in the log and use the measurements in §5.

## 4. What the shared screens need from React Native (verify on day one)

`apps/native/src` uses, from `react-native`: `View`, `Text`, `ScrollView`, `FlatList`,
`VirtualizedList`, `Image`, `Pressable`, `TextInput`, `Switch`, `Animated`, `PanResponder`, `AppState`,
`Linking`, `Share`, `BackHandler`, `NativeModules`, `NativeEventEmitter`, `UIManager`, `Platform`
(`Platform.isTV` must be true), and the television APIs `TVFocusGuideView`, `useTVEventHandler`,
`TVEventControl` (tvOS-only; the shell guards it), `hasTVPreferredFocus` and `requestTVFocus`. From
libraries: `@react-navigation/native` + `native-stack`, `react-native-screens`,
`react-native-safe-area-context`, `react-native-reanimated` (+ `react-native-worklets`),
`react-native-svg` (through the app's `metro/svg-lite.js` shim), `react-native-qrcode-svg`,
`react-native-url-polyfill`, `@bacons/text-decoder`.

For each: does the SDK's React Native provide it, is there a Vega-compatible version of the library, or
does `apps/vega` need a shim that keeps the shared code untouched? Write the table in the log before
building anything. The likely hard ones are focus (`TVFocusGuideView`, `requestTVFocus`), Reanimated,
and native-stack navigation; if focus guides are missing, implement the same behaviour with the SDK's
focus API behind a shim named exactly like the React Native export so `src/ui/Focusable.tsx` and
`src/screens/title/RowGuide.tsx` compile unchanged.

## 5. Parity measurements (the numbers to match)

The Apple TV app renders at 1920×1080 points. From `apps/native/src/ui/theme.ts` and
`packages/design/src/index.ts`:

- Television spacing: xxs 4, xs 8, sm 12, md 20, lg 28, xl 40, xxl 56, xxxl 80; page 80; gutter 24.
- Television metrics: rail 112 collapsed / 340 expanded; poster width 214; square width 250; focus
  scale 1.08; radii from `radii` × `tvRadiusMultiplier` (1.75); type roles from `typeScale.tenFoot`.
- Colours: `colors` in `packages/design` (ink `#071018`, raised `#10212d`, accent `#409fd4`, text
  `#edf5fa` …); the focus ring from `componentSpec.focus`; motion 120/180/320 ms.
- Font: Manrope 400/500/600/700 (`apps/native/assets/*.ttf`).

Since the screens are shared, you get these for free when the shim layer is right; the measurements are
for checking the result against the reference screenshots, not for re-implementing.

## 6. Phases and checkpoints

Each ends with a commit and a log entry. "Checked" means on the Vega virtual device with a screenshot,
or, if the device cannot be driven from the session, an honest "not checked" plus how to check it.

### Phase 0 — SDK facts and a shell that builds
- Install the SDK; create `apps/vega` from its template; wire Metro to `../native/src` and `../../packages`.
- The §4 table in the log; shims for what is missing; `PorticoRuntime`, no-op modules.
- **Checkpoint:** the app launches on the virtual device to the sign-in screen (Activation with the code
  and QR) and `npx tsc --noEmit` passes for `apps/vega` with the shared sources included.

### Phase 1 — sign in and browse everything
- `PorticoNetwork` (what the SDK allows), `PorticoCapabilities` (+ a `vegaProfile(options, facts)` in
  client-core if the SDK reports codec/HDR facts; otherwise the declared table), `PorticoSearchKeyboard`
  (the system keyboard), deep links if the SDK has them.
- **Checkpoint:** code sign-in approved from the web app; Home with hero and rows; every library tab;
  movie and show pages; Search; Saved; Settings with the Dashboard; the rail, focus and Back as on Apple
  TV. Screenshots next to the references for Home, a library, a movie page, a show page, Settings.

### Phase 2 — video
- `PorticoPlaybackModule` + `PorticoPlayer` over the media element; subtitles (embedded and sidecar);
  audio track change; `PorticoSystemMedia` for the remote's keys.
- **Checkpoint:** direct play, an HLS transcode, resume, subtitles, the option boxes, Up Next; the
  server's Now Playing shows the session.

### Phase 3 — listening
- The listening half: queue, §18 transitions, the now-playing pill, remote keys.
- **Checkpoint:** an album plays through (log the gap), an audiobook resumes with its speed.

### Phase 4 — home screen integration and polish
- Vega's home-screen rows (if the SDK offers them); the parity pass over §7 of the Android handoff's
  journeys 1–8 (television ones), fixing differences; a release package built the SDK's way and
  installed on a fresh virtual device.
- **Checkpoint:** `npm test` for the shared tests still passes from `apps/native` (you changed nothing
  there), `npx tsc --noEmit` and the strings lint pass for `apps/vega`, the log's final section is
  written.

## 7. If the virtual device cannot be driven

This runs overnight. If the Vega virtual device cannot be installed, started or driven from the
session, do not stop: establish the §4 facts from the documentation, build the shim layer and the bridge
against the SDK's types, keep the type-check green, and write under each checkpoint what was not run and
the exact commands to run it. Say plainly in the log's first lines whether anything ran on a device.

## 8. The log (required)

`portico-internal/Project Architecture/Audits/2026-10-03-vega-port-log.md`, uncommitted. First: the SDK
and React Native versions, the §4 table, whether a device ran. Then per phase: **Built**, **Decisions
where this handoff was silent**, **Differences from the Apple TV app** (each with reason and
screenshot), **Shared changes needed** (diffs against `apps/native/src` for the lead to apply), **Not
done, and why**, **Functional checks** (with screenshot paths), **Builds and suites**, **Commits**.
Questions for Justin numbered at the end; do not wait for answers.

---

## Prompt to paste

```
You are continuing work on Portico, a self-hosted media server with a Go backend, a web app, and one
React Native app (react-native-tvos) for iPhone, iPad and Apple TV. Your task is to bring that app's
screens, unchanged, to Amazon Fire TV's Vega OS using Amazon's Vega SDK (its React Native distribution,
W3C media for playback), in a new folder apps/vega that imports the shared screens from apps/native/src.
The Apple TV app is the oracle: the goal is visual and behavioural parity with it.

Read, in this order, and follow exactly:
1. portico-internal/Project Architecture/Audits/2026-10-03-vega-port-handoff.md.
2. portico-internal/Project Architecture/Audits/2026-10-03-android-port-handoff.md, §0–§2 and §4 (the
   native module contract: names, methods, events).
3. apps/portico-react-native/apps/native/README.md and src/bridge/README.md.
4. portico-internal/Project Architecture/06 Clients.md and Specs/Spec — Playback Protocol v1.md
   (§3, §5, §8, §18).

Rules you must not break: no subagents or helper lanes; almost no tests while building (type-check,
strings lint, a short device check per phase); never idle; no AI attribution or model names anywhere;
never modify apps/native/src (shims and bridges live in apps/vega; shared changes you need go in the
log as diffs); native module names, methods and events identical to the Apple ones; US English source
text; decide and log where the handoff is silent rather than asking.

Work in your own worktree and branch: cd apps/portico-react-native && git worktree add
"../../.worktrees/portico-react-native-vega" -b vega-port frontend-rework. Work only under apps/vega
there; never touch apps/native or apps/roku (other agents). Commit per phase with: git -c
user.name="Justin Ehler" -c user.email="justin@MBP.local" commit -m "…". Write the log at
portico-internal/Project Architecture/Audits/2026-10-03-vega-port-log.md but do not commit in
portico-internal. Establish the Vega SDK facts first and record them; if the Vega virtual device cannot
be driven from your session, keep building and say in the log what was not checked and how to check it.
Start with phase 0.
```
