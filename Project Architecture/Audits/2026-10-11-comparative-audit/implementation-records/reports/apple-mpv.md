# Lane report: apple-mpv

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/native-apple-mpv` (branch `audit-implementation/apple-mpv`, repository `portico-react-native`, 9 commits over `5c31d51`).
Status: COMPLETE as written code. **Nothing here has been compiled or run on an Apple device**: there is no Xcode or Swift compiler on this machine. The Swift was checked only by reading and by a bracket-balance script. The JavaScript is type-checked and tested (see section 8).

## 1. Outcome

- **mpv is a second player engine on iPhone, iPad and Apple TV.** It is its own native module and view (`PorticoMpvPlaybackModule`, `PorticoMpvPlayer`) beside the AVPlayer ones and speaks the same lease, command and fact contract, so the player screen drives either. Files: `apps/native/Native/PorticoMpv*.swift` (controller, libmpv wrapper, network relay, view, module, support), `PorticoMpvBridge.m`, `PorticoEngineClock.h/.m`, `Portico-Bridging-Header.h`.
- **What it plays:** MKV, WebM, AVI, TS and the other containers AVPlayer refuses, directly; H.264 (also 10-bit), HEVC, VP9, AV1, MPEG-2, MPEG-4, VC-1; DTS, DTS-HD, TrueHD, FLAC, Opus and the rest decoded on the device to multichannel PCM; embedded and external subtitles as text, ASS (with the file's attached fonts), PGS, VobSub and DVB, drawn on the device; audio and subtitle track switching inside the engine; speed, exact seeking, resume, chapters, progress through the same fact path as AVPlayer. It also plays the server's HLS streams.
- **System behaviour:** Now Playing and remote commands work for either engine (the system module reads a clock from whichever engine owns the picture); interruptions and route changes are handled; leaving the app pauses and tells JavaScript; on Apple TV the display is asked to match the frame rate and playback waits for the switch to finish.
- **What mpv cannot do is declared:** no Picture in Picture, no AirPlay video, no live channels, no music sessions. The module reports these (`features`), and the player hides Picture in Picture and AirPlay while mpv is active. Channels and music always play in the native engine.
- **The Player setting** (Automatic, the default; this device's player; mpv) is in Settings (Quality page) and in the player's own menu. The player screen uses the engine the server's plan names; otherwise the local rule. **Changing engine mid-title keeps the position, the session and the queue.**
- **The capability document** the app publishes is now version 2 in the server's new shape: one profile per engine (`engines.native`, `engines.mpv`), the viewer's setting, screen, output. The mpv profile has limits by device class.
- **`apps/native/Native/ENGINE-CONTRACT.md`** lists every command, fact, field, rule and error text both engines implement. The Android mpv engine can be written from it.
- Five AVPlayer-engine bugs from document 20 are fixed (section 3).

## 2. Decisions I made

1. **A second module, not a mode.** mpv is `PorticoMpvPlaybackModule` + `PorticoMpvPlayer`; JavaScript holds a lease on one engine at a time. Fact and Picture in Picture events have their own names per engine, because React Native delivers events by name alone.
2. **Video output is Metal through libmpv's own renderer** (`vo=gpu-next`, Vulkan on MoltenVK, a `CAMetalLayer` handed to mpv as its window). libmpv's `mpv_render_context` API has only OpenGL and software back ends, so this is how a Metal-backed view is reached; it is what upstream MPVKit ships. Hardware decoding is VideoToolbox.
3. **HDR: yes on iPhone and iPad (iOS 16+, HDR panel), no on Apple TV.** tvOS gives HDR output only to the system's own video layers, never to a Metal layer. On Apple TV mpv tone-maps HDR and Dolby Vision to standard range on the device. This is the biggest limitation of the lane: an HDR title on an HDR television loses its HDR when it plays in mpv. Automatic prefers the native player whenever it can play or plainly repackage the title, so this bites only when the alternative was a conversion. **The owner may want the planner to prefer a native HDR-preserving conversion over mpv direct for HDR titles on an HDR Apple TV.** A patched mpv build that outputs through `AVSampleBufferDisplayLayer` (as two of the reference clients use) would fix HDR and allow Picture in Picture; I did not depend on a third party's patched binaries.
4. **No bitstream passthrough.** iOS and tvOS give a third-party decoder no way to hand Dolby or DTS to a receiver undecoded. mpv decodes to PCM in as many channels as the output takes (Apple TV can re-encode to Dolby itself, in its own Audio Format setting). Atmos and DTS:X play as their channel bed; the profile claims no object audio.
5. **All of mpv's network reads go through a relay inside the app.** mpv's own HTTP client cannot see the system's trusted certificates on iOS and by default does not verify TLS at all. I did not accept that. mpv is only ever given `http://127.0.0.1:<port>/...`; the relay fetches the real resource with `URLSession` (system TLS validation, the app's transport security rules, certificates the device trusts). It serves only paths under the playback's own media grant, GET and HEAD only, and refuses redirects. It also tells the engine exactly why a fetch failed (HTTP status or connection error), so failures are classified as precisely as AVPlayer's.
6. **Dependency: upstream MPVKit `1.1.0-n9.0.2`** (mpv 0.41, its LGPL product), by Swift Package Manager, pinned to the exact version in both Xcode projects. CocoaPods has no upstream MPVKit pod. The engine files compile to a stand-in when the package is absent, so the app still builds without it (and then has no mpv and no setting).
7. **Engine choice.** In order: no mpv in the build, or music, or a channel: native. A chosen subtitle the server delivers for the device to draw in a kind the native engine cannot draw: mpv (unless the setting is Native). The engine the plan names. Then the setting. Then, on Automatic, the local rule.
8. **The local rule is narrower than the brief's sentence, on purpose.** Without a server-named engine the device can only tell that mpv is needed when it holds a source the native player cannot open: a downloaded MKV, for instance. A stream the server already converted was made for the native player and plays there; choosing mpv for it gains nothing and loses Picture in Picture and AirPlay. The full rule (mpv when the alternative is a video conversion, lossy audio or burned-in subtitles) is the server's, through the plan.
9. **On Automatic, choosing a styled (ASS) or image subtitle moves the title to mpv**, so it is drawn properly on the device instead of flattened to plain text or burned in. That follows "styled and image subtitles are rendered on the device wherever possible", but it means an ASS choice on Apple TV also takes the picture to standard range (decision 3). One line in `engine-media.ts` (`renderingToRequest`) restricts this to image subtitles if the owner prefers.
10. **Live channels do not play in mpv.** Channels are positioned by wall-clock dates from the stream, which mpv does not expose. The command is refused and the player never sends one.
11. **Leaving the app pauses mpv** (the system forbids drawing in the background) and tells JavaScript, so the intent becomes paused. The same fact (`systemPaused`) now also comes from the AVPlayer engine when the system paused it.
12. **Software decoding limits by device class**, from memory and processor count (so Android can use the same rule): light (2.5 GB or less, or two cores: Apple TV HD) leaves HEVC, VP9 and AV1 without hardware to the server; standard (up to 5 GB: Apple TV 4K, most phones) takes them to 1080p30; strong takes them to 4K30. These numbers are estimates, not measurements.
13. **The app now publishes the version 2 capability document** rather than leaving it to a later lane, because without it the server can never choose mpv. The native profile in it says: draws text subtitles only; does not switch audio tracks inside an original file (the app never has); HEVC only under the `hvc1`/`dvh1` tags; tone-maps HDR itself when HEVC is in hardware.
14. **Strings and the settings row are local stand-ins** (`engine-strings.ts`, `EngineSettingRow.tsx`), because `packages/*` are copies here. Both are small and say how to replace them.
15. The audio session for mpv uses the ordinary playback policy, not the long-form video policy AVPlayer uses, since mpv cannot use AirPlay 2 video anyway.

## 3. Bugs

Document 20:

| Bug | Result |
| --- | --- |
| B-20.1 | Not mine (planner, Android probe). The new document sends passthrough as its own list. |
| B-20.2 | Client side fixed: the native profile now says `toneMapsHdr` when HEVC is in hardware. The server side is playback-core's. |
| B-20.3 | Already fixed. |
| B-20.4 | Android. Not in this lane. |
| B-20.5 | Fixed for mpv (it waits for the display switch). **Not fixed for AVPlayer**: it is unconfirmed (needs a television), and gating AVPlayer's start blind risked a start-up delay I could not test. |
| B-20.6 | Already fixed. mpv classifies failures the same way. |
| B-20.7 | Fixed: both engines send `systemPaused`; the player sets its intent to paused. Needs a device to confirm. |
| B-20.8 | Already fixed. |
| B-20.9 | Client side fixed: the native profile states the HEVC tags AVPlayer takes; the server's planner has the matching field. |
| B-20.10, .11, .14, .19, .20, .21 | Android. B-20.11 no longer applies to the published document: every native engine declares text only. |
| B-20.12 | Fixed for mpv (system caption size scales its subtitles and the overlay line). Not fixed for the AVPlayer overlay; left for the native-playback lane, with colour and background. |
| B-20.13 | Already fixed. |
| B-20.15 | Fixed in part: chapters (from the server's playback options, or from mpv) now reach the lock screen. The published document no longer carries the false `trickplay`/`chapters` flags at all. The timeline still draws no chapter marks. |
| B-20.16 | Already fixed. |
| B-20.17 | Fixed: the profile's default video and audiobook speeds are applied. |
| B-20.18 | Fixed: the pre-play check waits 1.5 s at most. |
| B-20.22 | Fixed: both engines give the audio session back when a video is closed. |
| B-20.23 | Fixed: the observer comes off the item it was put on. |

Document 19 (B-19.1 to B-19.19): none is a player-engine bug; they belong to the native-app lane. Not touched.

## 4. API changes

No server routes. Two client behaviours against the server changed:

- `PUT /v1/me/devices/current/capabilities` now receives the version 2 document (`version: 2`, `client`, `engineSetting`, `overrides`, `display`, `audioOutput`, `maxBitrateBps`, `engines.native`, `engines.mpv`, `audioDecode`), matching `server/internal/playback/capability_document.go` in the playback-core worktree. It is sent again when the Player setting changes. The old shape (form, network class, `features`) is no longer sent.
- The player reads `engine` from the session and `deliveries` / `renderer: 'client'` from the subtitle plan, and passes `rendering` when choosing a subtitle. All three are read defensively and do nothing until the shared package carries them (section 7).

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

| Setting | Type | Default | Where it takes effect | Screen |
| --- | --- | --- | --- | --- |
| Player (`preferences.playerEngine`, this device) | Automatic, This device's player, mpv | Automatic | Engine choice in the player; sent to the server as `engineSetting` | Yes: Settings, Quality page, and the player's menu (Player). Shown only on a build that has mpv. |
| `playback.defaultSpeed`, `audiobooks.defaultSpeed` (existing, did nothing on native) | speed | 1 | Each video and audiobook starts at it | Existing rows |

## 7. Needed from others

**Shared packages (`portico-server/packages`):**

1. `client-core` `PlaybackSession`: add `engine?: 'native' | 'mpv'`, copied from the session presentation's `engine` in `toPlaybackSession`. Until then the plan's engine never reaches the player and Automatic uses only the local rule. Then make `planEngineOf` in `apps/native/src/player/engine-media.ts` a typed read.
2. `client-core` capability types: the version 2 document type. Replace the local `CapabilityDocument`/`EngineCapabilities` types (`src/bridge/capability-document.ts`, `mpv-profile.ts`) and the cast in `capability-publisher.ts`.
3. `client-core` subtitles (the subtitles-text lane has this in its worktree): `deliveries`, `renderer: 'client'`, `choose(resource, offsetUs, rendering)`. Then replace the stand-in readers in `engine-media.ts`.
4. `client-core` settings structure: on page `quality`, section `method`, add `{kind: 'composite', id: 'playerEngine', storage: 'device', label: 'settings.row.player', help: 'settings.help.player', scope: 'device', platforms: ['phone', 'tv'], needs: c => !!c.playerEngines, control: choice of automatic / native / mpv}`, and capability `playerEngines`. Then delete `EngineSettingRow.tsx` and the three lines in `SettingsPage.tsx`.
5. `i18n` catalogue: `settings.row.player` "Player"; `settings.help.player` "Automatic uses this device's own player when it can play a title as it is, and mpv when the title would otherwise have to be converted or have its subtitles burned in."; `settings.player.automatic` "Automatic (recommended)"; `settings.player.native` "This device's player"; `settings.player.mpv` "mpv". Then delete `engine-strings.ts`.

**Server, playback-core:**

6. One video entry per codec cannot give different limits by bit depth: 10-bit H.264 is software-only in mpv, 8-bit is hardware. The mpv profile lists `bitDepths: [8, 10]` at the hardware size limit. A 4K 10-bit H.264 file (rare) would be sent to mpv and may stutter on a weaker device.
7. The document has one `display`. From mpv an Apple TV shows standard range only (decision 3). `mpvDisplayRanges()` gives the list; the document has nowhere to put it. If the planner should weigh it, it needs a per-engine display list.
8. When a subtitle choice needs mpv, the plan should name `mpv`. The client moves the title to mpv by itself meanwhile.
9. After a change of the Player setting mid-title the client asks for the title again through `playback.play` (the path a quality change uses). When the one re-plan operation exists, use it here: `usePlayer.ts`, the effect commented "The Player setting changed while a title...".

**Server, subtitles-text:** the client sends `render.url` (the delivery's `url`) and `render.fonts` (from `fontsUrl`) to mpv; for VobSub mpv fetches `original.sub` beside `original.idx`. Both must be under the same media grant. That matches what the lane's report describes.

**Client lanes:**

10. `apps/native/src/app/device.tsx` still publishes the older client profile to `PUT /v1/playback/client-profile`, which playback-core removed. It fails quietly; delete the call (native-app or native-playback lane).
11. Android mpv lane: implement `PorticoMpvPlaybackModule` and `PorticoMpvPlayer` in Kotlin from `ENGINE-CONTRACT.md`. The JavaScript is platform-neutral and needs no change: `src/bridge/player-engines.ts` finds the module by name, and `facts()` feeds the same profile builder (`mpvEngineCapabilities`). Android's native profile is built by the same `nativeEngineCapabilities`.
12. Vega: no change needed; it has no mpv module, so the setting is absent. I could not type-check `apps/vega` (no `node_modules` there).
13. Downloads lane: an offline download of an MKV now plays (in mpv) on Apple. Subtitles travelling with downloads can be given to mpv as `render.url` with a `file:` address inside the Downloads folder; the engine accepts that.

**Documents to correct:** `06 Clients.md` and `04 Playback and Media.md` say the player on each client is one thin platform engine; there are now two. The Product Contract for Picture in Picture and AirPlay should say they are not offered while mpv is active. `apps/native/README.md` "Known gaps" still says subtitle selection is a placeholder, which is stale (not from this lane).

## 8. Test notes for other environments

### What was checked here

- `npx tsc --noEmit` and `npx tsc -p test/tsconfig.json --noEmit` in `apps/native`: clean.
- `npm run lint:strings`: clean (0 hard-coded strings).
- New tests, all passing: `test/engine-choice.test.ts` (6), `test/engine-switch.test.ts` (5), `test/mpv-profile.test.ts` (7, including the server's admission rules restated), `test/engine-media.test.ts` (7). Existing `v1-resume`, `local-playback`, `playback-failure-policy`, `capability-profile` tests still pass. The full suite was not run.
- Run one: `cd apps/native && node --experimental-strip-types --import ./test/register.mjs --test test/engine-switch.test.ts`.

### Building (a Mac with Xcode 16 or later)

1. `cd apps/native && npm ci --legacy-peer-deps`.
2. `cd ios && pod install`, then `xcodebuild -resolvePackageDependencies -workspace Portico.xcworkspace -scheme Portico` (fetches MPVKit `1.1.0-n9.0.2` from `https://github.com/mpvkit/MPVKit.git`; a large download). Same in `tvos/`.
3. Build the `Portico` scheme. **Expect Swift compile errors on the first build**: about 2,300 lines of Swift were written without a compiler. The likeliest places: libmpv C types in `PorticoMpvCore.swift` (enum comparisons, pointer conversions), and Objective-C types seen through `Portico-Bridging-Header.h` in `PorticoMpvController.swift` (`PorticoSubtitleSelection`, `PorticoPlaybackController.shared`).
4. If the package product does not link: in Xcode, target Portico, General, Frameworks and Libraries, confirm `MPVKit` is listed. The project files were edited by hand (`XCRemoteSwiftPackageReference "MPVKit"`, `SWIFT_OBJC_BRIDGING_HEADER`, eight new source files in each project).
5. While debugging HDR titles, turn Metal API Validation off in the scheme (Run, Diagnostics).
6. Check the built app's size, and that the MPVKit frameworks raise no privacy-manifest warning on upload.

### Device tests. Pass and fail are stated for each.

Use a server from the integrated programme branch (it must accept the version 2 capability document and name `engine` in the plan). Set Settings, Quality, Player as each test says.

1. **Setting appears.** Settings, Quality shows "Player" with three choices. Fail: no row (the module's `available` is false: the package did not link).
2. **MKV direct, Automatic.** Play an H.264/AAC MKV. Pass: it starts within a few seconds, the server's Now Playing shows it as original (no conversion), the player has no Picture in Picture button. Fail: a conversion session, or an error.
3. **MP4 stays native, Automatic.** Play an H.264/AAC MP4. Pass: Picture in Picture button present (native engine).
4. **Forced mpv.** Player = mpv; play the MP4 from test 3. Pass: plays, no Picture in Picture button, AirPlay absent from the Play on sheet.
5. **Seek, resume, speed.** In mpv: scrub to several places (the picture lands within half a second, no "position not available"), leave and resume (it starts at the saved position, not 0:00), set 1.5x (sound stays in pitch).
6. **Audio tracks.** A file with two audio tracks, direct in mpv: choose the second. Pass: sound changes within a second with no reload and no new session on the server.
7. **Subtitles.** (a) A plain SRT track: white text on a soft black box, inside the picture. (b) An ASS track with styling and attached fonts: positioned, coloured, in its own typeface. (c) PGS and VobSub: images appear, correctly timed. (d) Subtitle timing +0.5 s moves them later. Fail for any: nothing drawn, or the server starts a burn-in conversion.
8. **Engine switch mid-title.** Start an MP4 on Automatic (native). Open the player menu, Player, choose mpv. Pass: the picture returns within a few seconds at the same position (within a second), Up Next is unchanged, the Picture in Picture button disappears. Switch back: same. Also: on Automatic, while a title plays natively, choose a PGS subtitle: the title moves to mpv and draws it.
9. **Multichannel.** Apple TV into a 5.1 or 7.1 receiver: a DTS-HD or TrueHD track plays as multichannel PCM (the receiver shows PCM 5.1/7.1, or Dolby if Apple TV's Audio Format re-encodes). Fail: stereo.
10. **HDR.** iPhone or iPad with an HDR screen: an HDR10 title is visibly HDR in mpv (highlights brighter than the white interface). Apple TV: the same title is correct in colour but standard range (expected; decision 3). A Dolby Vision profile 5 file has correct colours (not green and purple).
11. **Frame-rate matching, Apple TV** (tvOS 17 or later, Match Frame Rate on): play a 23.976 fps title in mpv from a 60 Hz menu. Pass: the television switches, then playback starts; the opening seconds are not lost to a blank screen. On leaving the player the television switches back about two seconds later.
12. **Lock screen and remotes.** In mpv: the lock screen shows the title, artwork, position and a moving progress bar; play, pause, skip and scrub from the lock screen and AirPods work; with no queue, Next and Previous step by chapter in a file that has chapters.
13. **Interruptions.** During mpv playback take a phone call (iPhone) or invoke Siri: playback pauses, and resumes afterwards only if it was playing; sound returns. Unplug headphones: it pauses.
14. **Leaving the app.** In mpv press Home, wait, return. Pass: the picture is there, the button says Play, one press continues. Fail: black picture, a crash on backgrounding, or two presses needed. Repeat with AVPlayer and Picture in Picture off (the B-20.7 check).
15. **Network loss.** Mid-title in mpv switch Wi-Fi off for 15 seconds, then on. Pass: "Reconnecting", then playback continues from the same place. Fail: the title ends, or is marked watched.
16. **TLS.** A server on HTTPS with a certificate the device does not trust: mpv must refuse to play it, like AVPlayer. A server on plain HTTP on the home network must play.
17. **Errors.** A corrupt or zero-length file in mpv: a plain error or recovery through the next route, not a hang. A file whose only video codec mpv cannot decode: the server is asked for another route, not sound over black.
18. **Downloads.** Download an MKV, go offline, play it: it plays in mpv.
19. **Audio session release (B-20.22).** Play music in another app, start a film in Portico, close it. Pass: the other app's music may resume (it does in the Music app).
20. **Memory and heat.** A 4K HEVC remux for 20 minutes on the oldest supported Apple TV 4K and an older iPhone: no dropped-frame stutter, no termination.

## 9. Not finished

- **Not compiled, not run.** Everything native in this lane is untested code. Treat the first build as the start of testing.
- **HDR output on Apple TV in mpv**: not possible with this output path (decision 3). Standard range, tone-mapped.
- **Picture in Picture and AirPlay video in mpv**: not built (declared and hidden).
- **Live channels in mpv**: refused (decision 10).
- **Bitstream passthrough**: not available on the platform.
- **AVPlayer's late display switch (B-20.5)** and **AVPlayer's subtitle overlay ignoring caption style (B-20.12)**: not fixed.
- **Frame-rate matching in mpv needs tvOS 17**; on tvOS 15 and 16 there is no public way to ask without a media item, so mpv does not match there.
- **The plan's engine does not reach the player** until the shared package carries `engine` on the session (section 7, item 1). Until then Automatic means: native, except a download the native player cannot open, or a subtitle that needs mpv. The setting "mpv" works today.
- **Software decoding limits are estimates** (decision 12). They need measuring on real devices.
- **The Settings row and its strings are stand-ins** (section 7, items 4 and 5). English only.
- **`apps/vega` was not type-checked.**
- **Deinterlacing in mpv** is set to automatic and the profile claims interlaced MPEG-2 and VC-1, but it is unverified: check an interlaced DVD rip on a device for combing.
- **Timeline chapter marks and scrub thumbnails** are not drawn (not in this lane).
