# Lane report: native-playback

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/native-native-playback`, branch `audit-implementation/native-playback` (repository `portico-react-native`), 11 commits over `0e4fdcf` (`72cde11` … `bd8afae`). Nothing pushed. No subagent and no helper model was used; all work is mine.

**Nothing here has been built with Xcode or Gradle or run on a device or simulator.** What was checked: `npx tsc --noEmit` and `npx tsc -p test/tsconfig.json --noEmit` in `apps/native` (clean for every file of this lane), `npx tsc --noEmit` in `apps/vega` (clean for the same), `npm run lint:strings` (nothing of this lane's flagged), every test file in `apps/native/test` run one at a time, the Kotlin sources through the Kotlin 2.1.20 compiler for both flavours (type-checked, never linked), and a bracket-balance pass over the Swift and Objective-C I touched. The Swift and Objective-C were **not compiled**. Section 8 says how to test each thing on a device.

## 1. Outcome

- **One playback path.** Video on iPhone, iPad, Apple TV, Android, Android TV and Fire TV now plays through the shared playback service and the shared session controller, as the web does. The older bridge (`legacy-bridge`, `V1PlaybackApi`) is no longer used by the app or its tests. New file: `apps/native/src/player/native-playback.ts`. The engines stay driven by the coordinator; the session controller is told what the engine in use reports and owns the server's side: the timeline and lease, every change to the session, the stop with the last position, a session the server ended, and a lapsed lease (the queue entry starts again where it was instead of an error).
- **Changes in place.** Quality, version, audio track and subtitle track are each one change to the playing session: same session, same queue entry, Up Next untouched, the picture continues where it was. A player failure is reported the same way and the title comes back another way in place.
- **One capability document.** The app publishes the single document: a profile for the platform's player, a profile for mpv where the build has it, the screen, the audio output, what the music engine decodes, the Player setting and the three overrides. The older client profile and its publisher are deleted (including the Vega app's).
- **The server's plan names the engine.** Automatic follows the plan. Native and mpv are the viewer's override. Changing the setting mid-title republishes, then starts the current queue entry again where it stands: position, queue, Up Next and a pause are kept.
- **Quality.** The menu is the server's for the title: Original (the default), Automatic, then the fixed qualities below the picture. The tick is what the server put in force. Automatic measures the connection on the device (each engine now reports it) and asks the server to step down and back up. Saved defaults for home and away are the shared settings rows.
- **Subtitles.** A choice is a session change, so the server decides delivery from what this device's engines draw: plain cues, the original handed to mpv (the title moves to mpv on Automatic), or burn-in as the last resort. Every engine draws text in the profile's caption size and background on top of the system's caption style. The player's Subtitles panel has Size and Background.
- **Downloads.** The offline player offers the subtitle files stored with a download, shows its chapters, and offers Skip for its markers. A download the platform player cannot open goes to mpv by what the download says of its own file.
- **Remembered tracks.** The Audio menu ticks the track the server chose (asked for, or the language last picked for the show). mpv playing a file as it is starts on that track.
- **Chapters and scrub previews.** The timeline has a mark at each chapter and, while scrubbing, a card with the server's preview picture and the chapter name. The menu has Chapters and, for a title with several, Version.
- **Player bugs.** See section 3. In short: Apple TV and Android televisions hold a title until the display has matched it; Android no longer fails every seek in a file with no index; the crossfade survivor is no longer the film player; mpv on Android has a face for Arabic, Hebrew, Thai, Indic and other scripts; mpv on Apple opens the second file of an external VobSub track; skipped markers are reported again (they were silently dropped on native).

## 2. Decisions I made

1. **The session controller observes the engine; it does not drive it.** The coordinator and the two engine modules keep their lease, command and fact contract unchanged. The controller's player port is fed from engine facts. This is the smallest design that puts native on the shared path without rewriting two unbuildable engines.
2. **mpv's profile is the native repository's own builder, not the shared `mpvEngine`.** The local one knows the device (hardware decoder limits, device class, Android's missing deinterlacer and Dolby Vision profile 5, no passthrough). The shared one is cruder and would overclaim. The document is validated with the shared `capabilityDocumentProblem`; if it fails the mpv profile is left out.
3. **The platform player declares `embeddedAudioSwitching: false`** on both platforms, and text subtitles only. It does not choose between audio tracks inside an original file; saying so makes the server repackage with the chosen track instead of playing the wrong one.
4. **An audio choice on a server title is always a session change**, also in mpv, which could switch in place. Reason: only a session change is remembered for the next episode. Cost: a one to two second reload in mpv. In-place switching is kept for downloads. See section 7 for the server change that would remove the reload.
5. **A Player-setting change mid-title restarts the current queue entry** (a new session on the same entry). The server has no operation that re-plans a session only because the capability document changed.
6. **Android: a streamed file that cannot seek is reported as a player failure** (`source_not_supported`), so the server repackages it and remembers. If the owner has turned repackaging off, the viewer now gets "can't be played" where before they got a title that played from the start and refused every seek.
7. **Android televisions: the app votes for the frame rate itself** and waits for the switch, and Media3's own seamless-only request is turned off there. The system's "match content frame rate" setting still decides. An HDR title on an HDR television is also held about 1.5 s after its first picture. On Android 11 televisions every start waits up to 1 s (no setting to read).
8. **Apple TV: the display is matched before `ready`**, waiting for the system's mode-switch notifications, 10 s at most, 0.4 s when no switch begins. With Match Content off nothing is waited for.
9. **System caption colours win only where the system says they override** (Apple: the style's "video override" behaviour; Android: captions switched on with a style that names the colour). The system caption size always multiplies. Otherwise Portico's two settings decide.
10. **Caption Size and Background are offered in the player's Subtitles panel**, because the shared settings structure lists them for the web only and I may not edit it. They write the same profile preferences.
11. **Fallback fonts on Android are chosen by the subtitle track's language**, one face at a time. This build's libass cannot ask Android for a per-letter fallback. Mixed-script lines and untagged tracks remain a gap (section 9).
12. **Subtitle documents are accepted over plain HTTP wherever the media itself is** (same scheme, host, port and grant). The private-address restriction is removed from both native cue readers, per the programme's rule on plain HTTP.
13. **Throughput is reported only when really measured**: AVPlayer's observed bitrate, Media3's bandwidth callbacks (not its pre-transfer guess, and only while fresh), mpv's cache speed only while the reader is not resting. Files played from a download report none.
14. **Android passthrough also asks Media3's own `AudioCapabilities`**, which reads the HDMI plug report and Fire OS's surround setting. DTS-HD is now named `dts` (the probe said `dtshd`, a name the server never matches).
15. **The capability document is published only after the device has read the viewer's saved settings** (2 s at most), so defaults never overwrite what the server holds.
16. Nine player sentences are staged in `apps/native/src/player/player-words.ts` because the catalogue is a copy here (section 7).

## 3. Bugs

| ID | Result |
|---|---|
| B-20.1 | Fixed on the client: passthrough is its own list beside decoding, with the active route; DTS-HD naming corrected. Server side was playback-core's. |
| B-20.2 | Already fixed (shared `appleProfile` sets `toneMapsHdr`); kept. |
| B-20.3 | Already fixed; now also accepted over plain HTTP to any host the media itself uses. |
| B-20.4 | Already fixed (android-mpv). |
| B-20.5 | **Fixed** for AVPlayer on Apple TV and for Media3 on Android televisions (decisions 7, 8). Needs a television to confirm. |
| B-20.6 | Already fixed. |
| B-20.7 | Already fixed. |
| B-20.8 | Already fixed. |
| B-20.9 | Already fixed (`codecTags` in the shared Apple table). |
| B-20.10 | Already fixed. |
| B-20.11 | Fixed: the published native profile is text only on every platform, now from the shared table. |
| B-20.12 | **Fixed** on all four engines: Portico's size and background, and the system's caption style. |
| B-20.13 | Already fixed. |
| B-20.14 | Fixed: shared `androidProfile` caps software decoders; the app narrows further by device class. |
| B-20.15 | **Fixed**: the false flags are gone, and chapters and scrub previews are now drawn. |
| B-20.16, .17, .18 | Already fixed. |
| B-20.19 | **Fixed** by recovery (decision 6) plus a wider transport-stream seek search. Not reproduced: needs a cue-less MKV on a device. |
| B-20.20 | Already fixed; the probe now also asks the player's own audio output. |
| B-20.21 | **Fixed**: the crossfade player is built like the main one, and a film gets a fresh player. |
| B-20.22, .23 | Already fixed. |
| N-V02.1 | Already fixed in the shared service; native now uses that path. |
| B-14.12 | Fixed for playing: a download the platform player cannot open goes to mpv. No note in the download menu (not the player). |
| New | Marker skips were never reported on native: the local-playback wrapper dropped `markerSkipped`. Fixed. |
| New | External VobSub on Apple mpv could not open its `.sub` file. Fixed as on Android. |

## 4. API changes

No server route. Client behaviour against the server:
- `PUT /v1/me/devices/current/capabilities`: the shared document type; `overrides` now carry the viewer's switches.
- `PUT /v1/playback/client-profile` is no longer called (it was removed on the server).
- `PATCH /v1/playback/sessions/{id}` is now used for quality, version, audio, subtitles (`subtitles.trackId` is the subtitle resource id, `null` for off) and `failure`.
- `GET /v1/items/{id}/playback-options` feeds the quality menu, versions and chapters; `GET /v1/items/{id}/trickplay` feeds scrub previews.

Engine contract (`apps/native/Native/ENGINE-CONTRACT.md`, updated): `progress` facts may carry `throughputKbps` and `bufferedSeconds`; the subtitle command gains `style: {scale, background}`; `documentUrl` may be a `.vtt` file inside Downloads for a download; display matching is every engine's.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

| Name | Type | Default | Takes effect | Screen |
|---|---|---|---|---|
| Player (`playerEngine`, this device) | Automatic, This device's player, mpv | Automatic | `engineSetting` in the capability document | Settings › Quality (shared row, shown where mpv is built in) and the player menu |
| Always play in stereo (`forceStereo`, this device) | switch | off | capability document `overrides` | Settings › Quality, shared row |
| Don't pass sound through (`disablePassthrough`) | switch | off | same | shared row; shown only when a receiver is detected |
| Turn off HDR (`disableHdr`) | switch | off | same | shared row; hidden on a device with no HDR |
| `quality.home`, `quality.remote` | Original, Automatic, fixed | Original | the server, at each start | shared preference rows (not changed by me) |
| `playback.subtitleSize`, `playback.subtitleBackground` (existing, did nothing on native) | choice | medium, translucent | every engine's subtitle text | the player's Subtitles panel |

## 7. Needed from others

**Whoever merges this with native-sync.** Both lanes were told to replace the Player stand-ins. Expect conflicts in: `apps/native/src/screens/settings/SettingsPage.tsx` (I added three capabilities and four device rows, and removed the stand-in row), `src/app/preferences.ts` (three override fields, a loaded gate), `src/app/device.tsx` (the old publisher call removed), `src/player/VideoPlayer.tsx` and `PlayerOptions.tsx` (`word(...)` and `engineText(...)` replaced by `i18n.t`), and the deleted `EngineSettingRow.tsx`, `engine-strings.ts`, `bridge/client-profile.ts`. Take this lane's side for player files.

**The mirror dropped the downloads lane's shared code.** `main-server` does not yet contain the downloads lane's `packages/client-core` changes, so `--write` removed them. `src/bridge/offline-downloads.ts`, `src/app/downloads.tsx`, `src/screens/Downloads.tsx` and three download test files no longer type-check or pass. Not mine to fix: merge the server downloads lane, then mirror again. My offline code reads the bundle defensively and works either way.

**Shared packages (`portico-server/packages`):**
1. `client-core` `playback-v1/v1-queue-player.ts` `startChoices`: pass `throughputKbps` to a start, so Automatic can begin on the right step and a too-slow fixed quality is refused at start. Native has the estimate (`NativePlayback.throughputKbps`).
2. `client-core` `playback-v1/http.ts` `PlaybackApiError`: carry `error.reason`. `connection_too_slow` names the quality that fits; without it native says only "try Automatic".
3. `client-core` `presentation/settings-structure.ts`: remove `platforms: ['web']` from the subtitle appearance section (native now honours both).
4. `client-core` `index.ts` `PlaybackSession`: an `engine` field is no longer needed by native (it reads the v1 session); no change required.
5. `i18n` catalogue, then delete `apps/native/src/player/player-words.ts`: `player.opt.chapters` "Chapters"; `player.chapter.numbered` "Chapter {number}"; `player.opt.versionChangeFailed` "The version couldn't be changed. Playback continues as it was."; `player.subtitles.changeFailed` "The subtitles couldn't be changed."; `player.subtitles.drawnHere` "Drawn by this device"; `player.quality.tooSlow` "Your connection is too slow for that quality. Try {quality}, or Automatic."; `player.quality.tooSlowAny` "Your connection is too slow for that quality. Try Automatic."; `player.quality.automaticNow` "Automatic · {quality}"; `player.quality.fixedHelp` "Up to {height}p; converted only when the file is larger". Native also uses four `web.player.quality.*` strings as they are.
6. `legacy-bridge.ts` can be deleted from `client-core`: no client uses it.

**Server, playback-core:**
7. Remember the audio language from the timeline's `audioTrackId` (or a no-re-plan flag on `PATCH`), so mpv can switch tracks in place and still be remembered (decision 4).
8. A `PATCH` that re-plans because the device's capability document changed, so a Player-setting change keeps the same session (decision 5).
9. Still open from the mpv lanes: per-engine display ranges; one limit per codec.

**Downloads lane:** carry styled and image subtitle originals; the player already offers any form mpv draws.

**A libmpv of our own for Android** (newer libass with its Android font provider, libplacebo, the S/PDIF muxer, deinterlacing) would close the font gap and three declared "no"s.

**Documents to correct:** `06 Clients.md` and `04 Playback and Media.md` (native is on the shared session path; two engines; Automatic quality is measured on the device); `Spec — Playback Protocol v1` §5.5 (device-drawn subtitles on native); the Accessibility contract A11Y-10.5 can be marked met for subtitle appearance; `apps/native/README.md` known gaps (I removed the stale subtitle line).

## 8. Test notes for other environments

Server: a build of the integrated programme branch. Build the app first (iOS, tvOS, both Android flavours). **Expect compile errors on the first Apple build**: about 250 lines of Objective-C and Swift were written without a compiler, in `PorticoPlaybackController.m` (`matchDisplayForItem:`, `addConnectionFacts:`), `PorticoPlayer.m` (`styleSubtitle:`), `PorticoSubtitleSelection.m` (`readLocal:`), `PorticoMpvController.swift` (`applyCaptionStyle`, `closeReferences`).

1. **Capability document.** Sign in, then `GET /v1/me/devices/current/capabilities` with the device's token. Pass: `version: 2`, `engines.native`, `engines.mpv` on a build with mpv, `engineSetting` as set, `overrides` all false. Turn on "Always play in stereo": within two seconds the stored document shows `forceStereo: true`.
2. **In-place quality.** Start episode 2 of a season. In the player choose 720p. Pass: the picture resumes within a second of where it was, Up Next still lists episode 3, the server shows the same session id with generation + 1, the Quality box says 720p. Choose Original: same checks. Fail: a new queue or an empty Up Next.
3. **Automatic.** Choose Automatic, throttle the link to about 3 Mb/s on a 20 Mb/s title. Pass: within about 15 s a message says the quality was lowered and playback continues; remove the throttle and within about a minute it is raised a step at a time. Fail: nothing happens (then check the engine sends `throughputKbps`: in a development build log the `progress` facts).
4. **Engine from the plan.** With mpv built in and Player on Automatic: an H.264/AAC MP4 plays in the platform player (Picture in Picture button present); an MKV with DTS plays in mpv with no conversion on the server. Change Player to "This device's player" mid-title: the picture returns at the same second, paused if it was paused, Up Next unchanged.
5. **Subtitles.** (a) SRT: appears; in the Subtitles panel press Size and Background: the line changes at once. (b) Set large captions in the system's Accessibility settings: the line grows on both engines. (c) On Automatic choose a PGS or styled ASS track on a title playing in the platform player: the title moves to mpv and draws it, the server shows `subtitle.delivery: "device"`, no burn-in conversion. (d) Player set to "This device's player", same track: burned in, after the warning. (e) Turn subtitles off: returns to the platform player.
6. **Remembered tracks.** Pick Japanese audio and English subtitles on episode 1; start episode 2. Pass: both are in force and ticked in the menus.
7. **Chapters and previews.** A film with chapters and scrub previews made on the server. Pass: marks on the timeline; dragging (phone) or scrubbing on the remote's surface (Apple TV) shows a small picture and the chapter name; Menu › Chapters lists them and jumps.
8. **Offline.** Download a film with an SRT track, chapters and an intro marker; go to Airplane Mode; play it. Pass: Subtitles lists the track and draws it; Chapters lists; Skip appears in the intro. Download an MKV on an iPhone: it plays in mpv.
9. **Apple TV display matching (the least certain Apple change).** Match Frame Rate and Match Dynamic Range on; play a 24 fps HDR film from a 60 Hz SDR menu in the platform player. Pass: the television switches, then playback starts at the resume point with nothing lost. Fail: a start delayed by the full 10 s (the switch notifications did not arrive), or the old behaviour. Repeat with both settings off: no added delay.
10. **Android television display matching.** Android TV 12+, "Match content frame rate: Always", a 23.976 fps title in the platform player. Pass: the television switches and playback starts after it. With "Never": starts at once. An HDR10 title: starts about 1.5 s after the first picture, opening seconds intact. Fail: black picture, or a start that never comes.
11. **Unseekable file (Android).** An MKV written without a cue index, played directly, resumed at 60 s. Pass: a brief pause, then it plays from 60 s as a repackaged stream; later plays go straight there. Fail: "That position is not available".
12. **Crossfade then film (Android).** With crossfade on, let two songs crossfade, then play a film. Pass: plays normally; pausing another app's audio behaves as for any film.
13. **Fonts (Android, mpv).** An ASS track with no attached fonts in Arabic, Hebrew, Thai, Hindi and Japanese, each tagged with its language. Pass: glyphs, not boxes. Note any script that shows boxes and the files under `/system/fonts`.
14. **External VobSub (Apple, mpv).** A direct-played MKV with an `.idx`/`.sub` sidecar chosen. Pass: images appear.
15. **Passthrough (Fire TV with a receiver).** Read the stored document: `engines.native.audioPassthrough` lists `ac3`/`eac3`. Switch to Bluetooth headphones: the list is empty within a few seconds.
16. **Lease.** Pause a film, put the phone away for five minutes, come back and press Play. Pass: "Reconnecting" briefly, then it plays from the same place. Fail: an error card.
17. **Vega.** Type-checked only. Confirm it still plays and that the capability document is stored for it.

Run one JavaScript test: `cd apps/native && node --experimental-strip-types --import ./test/register.mjs --test test/native-playback.test.ts`.

## 9. Not finished

- **Nothing was compiled for Apple or built for Android, and nothing ran on a device.** Treat the first builds as the start of testing.
- **Throughput is not sent with a start**, only with changes (needs shared change 7.1). Automatic therefore always starts at Original.
- **`connection_too_slow` cannot name the quality that fits** (needs 7.2).
- **Fallback fonts on Android are by language only.** A line mixing two non-Latin scripts, digits or Latin words inside some scripts' faces, and a track with no language tag can still show boxes. On Apple I changed nothing: I believe MPVKit's libass uses the system's fonts with per-letter fallback, but I could not verify it here (test 13 applies to Apple too).
- **mpv's HDR direct-output switch on Android** (the android-mpv lane's "least certain part") is unchanged and still unproven.
- **A Player-setting change makes a new session** on the same queue entry, not a re-plan of the same session (decision 5).
- **Caption style on Vega**: the command carries it; I did not change Vega's own subtitle drawing.
- **Settings rows for caption size and background** are in the player, not in Settings (needs 7.3).
- **Chapter thumbnails** are parsed but not shown in the chapter list (names and times only).
- **Download-related type errors and test failures from the mirror** remain (section 7). Tests failing on the mirror commit itself and not mine: `content-forward`, `feedback-state`, `container-enrollment`, `offline-groups`, `offline-ownership`.
- The `NativePlayback` path has ten tests against the shared fake server; the React hook (`usePlayer.ts`) that wires it has none.
