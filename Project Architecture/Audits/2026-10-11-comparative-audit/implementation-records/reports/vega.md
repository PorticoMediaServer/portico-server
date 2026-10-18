# Lane report: vega

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/native-vega`, branch `audit-implementation/vega` (repository `portico-react-native`), 14 commits over `4f64baf` (`d5d6007` … `6bfb3c0`), all by Justin Ehler, nothing pushed. `packages/` is untouched. No subagent and no helper model was used.

**Nothing here was built for Vega or run on a Vega device or the SDK's virtual device.** What was checked: `npx tsc --noEmit` and `npx tsc -p test/tsconfig.json --noEmit` in `apps/vega` (clean); `npx tsc --noEmit` and `npx tsc -p test/tsconfig.json --noEmit` in `apps/native` (clean); `npm run lint:strings` in both (exit 0; Vega 0 hard-coded strings; native has the one known over-baseline line in `screens/Downloads.tsx`, not mine); the Vega app's new test suite, `npm test` in `apps/vega` (7 files, 61 tests, about ten seconds, all pass); and eight native test files next to the shared files I touched, one at a time (all pass, including `feedback-state`, which was failing before this lane). The Vega tests run the real bridge code against stand-ins for the media element, its media source and a server; they prove the logic, not the device.

## 1. Outcome

The Vega app already drew the shared screens (it imports `apps/native/App.tsx`), so every screen that landed today reached it for free. What it lacked was underneath: the parts Vega supplies itself (its "native modules" in JavaScript, its player, its description of the device). Those are now level with the Android TV and Apple TV apps, and what Vega cannot do is hidden by what the device says of itself, not by checks for Vega in the screens.

- **The player is a full engine of the shared playback path.** It was already driven by the shared playback service and session controller; it now keeps the whole engine contract (`apps/native/Native/ENGINE-CONTRACT.md`). It declares what it does not do (no Picture in Picture, no AirPlay, text subtitles only), so the player screen no longer offers a Picture in Picture button that did nothing.
- **Automatic quality works.** The engine measures the connection as it fetches the stream and reports it with how much is buffered, which is what Automatic steps down and back up on. Before, Vega reported neither, and Automatic could never move.
- **Re-plans in place** (quality, version, audio, subtitles) need nothing of Vega's own: they are the shared session change, and the engine opens the new presentation where the old one was.
- **Live channels play.** Vega's stream reader could not follow a live playlist (it slides, and restarts its clock at a programme change) and did not know a channel's timeline. It now does both, including pausing and seeking back inside the channel's window.
- **Subtitles** are drawn in the profile's caption size and background (the player's Subtitles panel changes them at once), honour the track's timing offset (it was silently ignored: the offset arrives as text and Vega read it as a number), are accepted only from the playing session's own address, and go dark 40 seconds after the server stops confirming them, as on the other engines. Styled and image subtitles are burned in by the server: the capability document says the engine draws text only.
- **One honest capability document.** One engine (`native`), no mpv. Its codecs and containers now come from what the device itself answers, on top of the declared table. It claims no HDR and no surround output, because the SDK cannot say what the television shows or what the sound goes to. Settings no longer offers "Turn off HDR" on Vega (it could do nothing).
- **Watch Together and "Play on" arrive as they happen.** The shared code moved its event streams to a native reader on 12 October, and Vega had none: both streams failed to open on Vega. Vega now has the reader, with the same limits and error codes as Apple and Android.
- **The device is called what it is.** Settings, the sign-out confirmation, sign-in approvals and Link a TV said "Apple TV" for a Vega device in places. They say "Fire TV", from one description of the device.
- **Loading states cannot freeze the app.** Vega's own loading ring exists because the shared one stops the app on Vega OS 1.2. The shared loading state (`StateView loading`, used on about twenty screens, including "Finding servers" and the player) still drew the shared ring inside itself. Every ring is now Vega's.
- **The remote's Menu key (≡)** opens the focused control's menu, as on a Fire TV that runs Android.
- **Speed is gone from the player** on Vega (the media element has one speed), and a profile whose default speed is not 1× now plays; before, it was paused at once with "This speed is unavailable".
- **A video is paused when the app leaves the screen**, and the player is told, so it shows Play on return. Music plays on.
- **The route to the server being checked again** no longer leaves a failed player behind: the place is kept, the media is closed, and it opens there when the route is ready.
- **Why a title stopped** is reported in the shared terms (the server's status, a lost connection, or the player refusing the media), so "This can't be played" and the server's "deliver it another way" work on Vega.

### Parity inventory

Android TV / Apple TV ("TV") against Vega. "Shared" means the same screen file runs on Vega and needs nothing of Vega's own. "Fixed" means it was broken or wrong on Vega before this lane.

**Screens**

| Screen | TV | Vega before | Vega now |
|---|---|---|---|
| Welcome, Portico landing, server chooser, saved server unavailable | Yes | Shared; loading states drew the ring that freezes Vega | Shared. **Fixed** (ring) |
| Sign in by code: Portico Account activation, behind the outage gate | Yes | Own copy of the shared file, kept in step by hand | Shared file itself; the copy is deleted |
| Sign in by code: server Quick Connect, "waiting for the owner" | Yes | Own copy, as above | Shared file itself |
| Direct sign-in by address, plain-HTTP note, profiles, PIN | Yes | Shared | Shared |
| "This device can't read its saved sign-in" (reset) | Android TV and Fire TV only | Not shown | Not shown: Vega's records have no separate key to lose |
| Home, Home row "See all", customise Home | Yes | Shared | Shared |
| Library: tabs, filters, sort, remembered tab and sort | Yes | Shared | Shared |
| Title page, show and episodes, versions, missing and unavailable titles, files list | Yes | Shared | Shared |
| Collection, playlist, saved view, person, search | Yes | Shared | Shared |
| Saved: watchlist, favourites, playlists, history | Yes | Shared | Shared |
| Live TV: channels, guide, programme sheet, reminders | Yes | Shared screens; **a channel did not play** | Shared; **fixed** (player) |
| Recordings (DVR) | Yes | Shared | Shared |
| Watch Together: create, join by code, group pill, host states | Yes | Shared screens; **no live events** | Shared; **fixed** (event streams) |
| Receive "Play on" from a phone | Yes | Shared; **no live events**; announced the TV as "Apple TV" until its name was read | Shared; **fixed** |
| Notifications, invitations, server membership | Yes | Shared | Shared |
| Settings and every page (below) | Yes | Shared | Shared |
| Server dashboard (owner) | Yes | Shared | Shared |
| Link a TV (on a phone, naming this device) | Shows "Fire TV" | Showed the raw word "vega" | "Fire TV". **Fixed** |
| Downloads | No TV has it | Hidden | Hidden (the shared rule for televisions) |
| Cast / AirPlay sender | No TV has it | Hidden | Hidden |
| Rows on the television's home screen | Apple TV and Google TV | None | None, by decision: Amazon's route reports viewing to Amazon (README) |
| Finding servers and televisions nearby | Yes | None | None: the SDK has no discovery API. Code or address instead |

**Settings rows (television)**

| Row | TV | Vega now | How it is decided |
|---|---|---|---|
| Profile: identity, profiles, Portico Account, devices, Sign out | Yes | Yes; the sign-out confirmation now says "Fire TV" | Shared; device description |
| Playback: next episode, countdown, still watching, skipping, skip amounts, watched threshold | Yes | Yes | Shared |
| Playback: default speed (video, audiobooks) | Yes | Hidden | `playerSettingsSupport.playbackSpeed` is false |
| Audio and subtitles: languages, subtitle mode | Yes | Yes | Shared |
| Caption size and background | In the player's Subtitles panel | Same; now takes effect | Engine draws the style |
| Quality at home and away (Original, Automatic, fixed) | Yes | Yes; Automatic now works | Shared; engine reports the connection |
| Playback method | Yes | Yes | Shared |
| Player (Automatic, this device's player, mpv) | Where mpv is built in | Hidden | No mpv module in this build |
| Always play in stereo | Yes | Yes | Capability document override |
| Don't pass sound through | Where a receiver is detected | Hidden | No receiver is ever reported on Vega |
| Turn off HDR | Where the device has HDR | Hidden. **Fixed** (was offered and did nothing) | The document claims no HDR |
| Music: lyrics | Yes | Yes | Shared |
| Music: volume levelling, crossfade, gapless | Yes | Hidden | `playerSettingsSupport.audioEffects` is false |
| Appearance: reduce motion, text size | Yes | Yes | Shared |
| Region, Privacy, Reset recommendations | Yes | Yes | Shared |
| This device: name, Receive from other devices, servers | Yes | Yes | Shared; Vega's own device name |
| About: version, Send a report | Yes | Yes | Shared; report says platform `vega` |

**Player abilities**

| Ability | TV | Vega before | Vega now |
|---|---|---|---|
| Shared playback service and session controller | Yes | Yes | Yes |
| Engine named by the server's plan | native or mpv | native | native (one engine) |
| Original, repackaged (HLS fMP4) and converted (HLS TS) streams | Yes | Yes | Yes |
| A file from its address | Yes | Only when the server is HTTPS | Same: the media element refuses plain HTTP. Declared in the document |
| Quality menu: Original, Automatic, fixed; the tick is what is in force | Yes | Menu shown; Automatic never moved | Yes |
| Change quality, version, audio or subtitles in place | Yes | Yes (shared) | Yes |
| Audio tracks of a stream; remembered language | Yes | Yes | Yes |
| Audio track inside an original file | mpv only | No | No; declared, so the server repackages |
| Text subtitles, caption size and background, timing offset | Yes | Fixed look; offset ignored | Yes |
| Styled (ASS) and image subtitles drawn on the device | mpv only | No | No: burned in by the server, as declared |
| Chapters: marks, names, menu | Yes | Yes (server's chapters) | Yes |
| Scrub previews | Yes | Yes (shared) | Yes |
| Skip intro, credits, recap; Up Next; still watching | Yes | Yes | Yes |
| Live channel playback, pause and seek back | Yes | No | Yes |
| Music and audiobooks | Yes, with gapless and crossfade | Yes, without | Same, without (no separate listening owner); hidden in Settings |
| Playback speed | Yes | Speed box shown, every other speed disabled; a default speed stopped playback | Hidden |
| Sleep timer | Yes | Yes | Yes |
| Picture in Picture | Apple TV | Button shown, did nothing | Hidden |
| "This can't be played" and the server's other way to deliver | Yes | Could not tell the cases apart | Yes |
| Reconnecting after a lost route | Yes | Could stay failed | Yes |
| Paused when the app leaves the screen | Yes | Not handled | Yes |
| Remote transport keys and voice requests | Yes | Yes | Yes |
| Display matched to the title (frame rate, dynamic range) | Yes | No | No: the SDK offers no way (section 9) |
| HDR, surround sound, passthrough | Where the device has them | No | No: not claimed (section 9) |
| System caption style (Accessibility) | Yes | No | No: the SDK does not tell an app |

## 2. Decisions I made

1. **Three small files say what a Vega device is, and nothing else checks for Vega.** `bridge/device-platform.ts` (what it is called, what it can send to), `bridge/native-listening-kinds.ts` (speed, audio effects) and the engine's `features` plus the capability document (what the player does). Each is a Vega override of a shared file that already existed for exactly this purpose.
2. **Four overrides became fewer and smaller.** With the device described in one place, Vega's 106-line copy of `setup-codes.ts` and its three wrappers of client-core files are deleted; Vega now runs the shared sign-in code itself. A copy had already needed hand-updating once today.
3. **The loading ring was split into its own shared file** (`apps/native/src/ui/Ring.tsx`; `Feedback.tsx` re-exports it). No behaviour changes on other platforms. It is the only way a loading state inside a shared file gets Vega's ring.
4. **The capability document is probed, with three things held back on purpose.**
   - *No HDR.* The SDK does not say what the television shows, and HDR sent to a screen that does not show it is grey and flat. The server tone-maps. This is the largest quality cost of the lane's decisions and is easy to reverse once a device proves otherwise (section 9).
   - *Dolby audio is never listed as decoded*, whatever the device answers. Its "yes" does not tell decoding from handing on to a receiver, and a television that cannot decode it would be silent. It stays listed for handing on, used only when a receiver is reported (never, today).
   - *Two channels out.* The server mixes surround down.
5. **The SDK's `smooth` answer is ignored.** SDK 0.24 leaves it false for every picture, which would have capped every codec at 1080p. A size the decoders list is taken to play smoothly.
6. **H.264 at 4K is asked again at the level 4K needs.** The shared question names level 4.0, which a device that answers by level refuses.
7. **A channel's segments are placed one after another** (the source buffer's `sequence` mode), and where each one landed is read back from the buffer. If a device refuses that mode, the segments keep their own times and playback is carried across a programme change (at most 0.4 s late). Both are tested.
8. **Throughput is reported only from segments of 64 KB or more, for 15 seconds**, counted from the server's first byte so a segment still being written does not read as a slow connection. A file the media element opens itself (HTTPS direct play) reports its buffer only; Automatic then steps down on stalls alone.
9. **Only the app going to the background pauses a video**, not something the system draws over it. Music plays on.
10. **The Menu key is heard in Vega's own layer.** The shared screens get the remote in the Apple TV's vocabulary, where "menu" means Back; one small shared export (`runFocusedMenu`) lets Vega run the focused control's action.
11. **Request ids on Vega do not replace a `crypto` the runtime may already have.** The Apple and Android entry installs `crypto.randomUUID` by assigning onto the global; on a runtime I cannot inspect, that could stop the app at launch. Vega has its own guarded version.
12. **A test suite for Vega's bridge was added** (`apps/vega/test`, `npm test`), because nothing else can check this code here. It found a real fault before any device did (a seek to a moment a channel has not broadcast yet never finished).

## 3. Bugs

The brief names documents 19 and 21 as background; these are the entries in them, and in document 22, that concern Vega.

| ID | Result |
|---|---|
| B-21.10 (Vega offers speed and audio effects) | Settings was fixed earlier today. **Finished here**: the player's Speed box is hidden, and a default speed set on another device no longer stops playback. |
| B-19.11 (Vega keeps sign-in records as plain files) | Not fixed. The SDK has no keychain; the app's private directory is the protection. `06 Clients.md` should say so. |
| B-22.19 (Vega refuses a redirect only after following it) | **Not fixed.** The SDK's networking gives JavaScript no way to refuse a redirect before it is followed; closing it needs a native HTTP client, which cannot be built or tried here. The new event stream and subtitle fetches refuse a redirected answer the same after-the-fact way. One redirect test on a device is still owed (section 8, I). |
| B-20.1 (audio a receiver could take is converted) | Not present as a defect on Vega: no receiver can be detected, so nothing is claimed. Unchanged. |
| B-20.12 (caption style) | **Fixed on Vega** (size and background). The system's own caption style is not available to an app on Vega. |
| B-25.10 (no automated checks for Vega) | Partly: the bridge now has 61 tests that need no device. Device behaviour is still unautomated. |
| New | Watch Together and receiver event streams could not open on Vega since 12 October. Fixed. |
| New | Loading states drew the ring that stops the app on Vega OS 1.2. Fixed. |
| New | Subtitle timing offset ignored on Vega. Fixed. |
| New | Picture in Picture button shown on Vega. Fixed. |
| New | A profile with a default speed other than 1× was paused with an error on Vega. Fixed. |
| New | "Turn off HDR" offered on Vega with no effect. Fixed. |
| New | A Vega device named "Apple TV" in Settings and before its name was read; "vega" shown raw in Link a TV. Fixed. |
| New | A route re-check could leave Vega's player failed with no recovery. Fixed. |

## 4. API changes

None. No route was added or changed. Client behaviour against the server on Vega: `GET /v1/notifications/events` and `GET /v1/groups/{id}/events` are now opened as streams; subtitle documents are fetched from `/v1/media/subtitles/{grant}/{resource}/{revision}` with `If-None-Match` every 25 seconds; the capability document's `engines.native` may carry `evidence: "probed"`.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

None added. On Vega:

| Name | Change | Takes effect | Screen |
|---|---|---|---|
| `playback.subtitleSize`, `playback.subtitleBackground` | Wired (did nothing on Vega) | Vega's subtitle line | The player's Subtitles panel (shared) |
| `quality.home`, `quality.remote` = Automatic | Wired (could not move on Vega) | Engine reports the connection | Settings › Quality (shared) |
| Turn off HDR (`disableHdr`) | Hidden on Vega | — | Was shown, did nothing |
| `playback.defaultSpeed`, `audiobooks.defaultSpeed` | Not applied on Vega | The player plays at 1× | Hidden in Settings already |

## 7. Needed from others

**Shared packages (`portico-server/packages`), none blocking:**
1. `client-core` `client-profile.ts` `probeWebProfile`: ask the 4K question for H.264 with a level that covers 4K (`avc1.640033`), not `avc1.640028`. Vega works round it locally; the web has the same question.
2. `client-core` `presentation/destinations.ts` `DestinationPlatform`: add `'vega'` (Vega passes `'firetv'`, which gives the same answer).
3. `client-core` `client-profile.ts` `vegaDeclaredProfile`: its comment says Dolby is used "once the probe reports the route"; nothing on Vega can report one. Either say so, or see section 9.
4. No strings are needed: this lane added none.

**`apps/native` (whoever merges):** `DevicePlatform` in `bridge/device-platform.ts` has no `'vega'` member; Vega's override casts. Adding it needs item 2 first.

**Server:** nothing.

**Documents:** `06 Clients.md` and `01 Overview.md` do not mention Vega; they should describe it as a thin variant with its own player, storage and capability document, and record B-19.11 and B-22.19 as known limits. `Native/ENGINE-CONTRACT.md` says "there are two engines"; Vega's is a third implementation of the native one and could be named there.

## 8. Test notes for other environments

**Environment.** A machine with the Vega SDK 0.24 (`vega` on the PATH) and Node 22; a Fire TV on Vega OS 1.2 in developer mode (preferred) or the SDK's virtual device. A Portico server from the integrated programme branch on the same network over plain HTTP, and, for step E3, one reachable over HTTPS. A phone signed in to the same server. For anything about the player, set `"debugConsole": true` and a `debugSink` in `apps/vega/app.json` (README, "Settings in `app.json`") and run `node scripts/debug-sink.mjs`: once video is drawn the virtual device's screenshots stop updating, so the player is read from its trace. Do not ship with it on.

**Zero.** `cd apps/vega && npm install --legacy-peer-deps && npx tsc --noEmit && npm test`, then `node scripts/vega.mjs run --release`. Pass: it builds, installs and launches. This is the first build since the changes; a bundling error here is the first thing to report.

**A. First run.** Launch signed out. Pass: the wordmark, then the sign-in choice; every loading ring turns; while "Finding servers" (or any loading state) is on screen the remote still moves focus. Fail: the app stops responding while a ring is on screen.

**B. Sign in by code.** Choose the Portico Account: a code and QR appear. On the phone, Link a TV, type the code. Pass: the phone shows the television's own name (or "Fire TV") and "Fire TV · Portico 0.1.0" beneath, never "Apple TV" or "vega"; approving signs the television in. Repeat with a server's Quick Connect code. Stop the Portico Account service (or block it) for a minute with the code screen open: the screen keeps the code and says it is waiting, and recovers by itself.

**C. Browse.** Home, a library, change its tab and sort, leave and return (the tab and sort are remembered), a title page, search, Saved. Press the remote's Menu key (≡) on a poster: its menu opens once, on release. Press it on Play or Sign out: nothing happens. Back still goes back. In Settings: no Speed section under Playback; no levelling, crossfade or gapless under Music; under Quality, "Always play in stereo" only (no Player, no "Turn off HDR", no "Don't pass sound through"); the sign-out confirmation says "Fire TV".

**D. Capability document.** After sign-in, `GET /v1/me/devices/current/capabilities` with the television's token (or read it in the owner console). Pass: `client.family` `vega`, one engine `native` named `w3cmedia`, `subtitleRendering: ["text"]`, `embeddedAudioSwitching: false`, every `dynamicRanges` `["sdr"]`, `audioOutput.maxChannels` 2, no `direct` transport from an HTTP server. With the device answering: `evidence: "probed"`, and HEVC at 2160 on a 4K stick. In the trace: a line `capability probe: video …` and, with the debug console on, what the decoder says of HDR10 and HLG (write these down: they decide section 9's first item). If the trace says "the device did not answer", the declared table is in use: report it.

**E. Play.**
1. *Repackaged:* an H.264 or HEVC MKV with AAC. Pass: plays within a few seconds; the server shows no conversion. Seek forward 20 minutes and back.
2. *Converted:* a title this device cannot decode (for example MPEG-2 or VC-1). Pass: plays; the server shows a conversion.
3. *Direct:* from the HTTPS server, an H.264 MP4. Pass: the session's mode is direct. From the HTTP server the same file is repackaged instead.
4. *Resume:* stop at 30 minutes, start again. Pass: opens there, not at the start.
5. *Surround source:* a 5.1 AC-3 or E-AC-3 title. Pass: stereo sound (converted). Fail: silence.
6. *HDR source:* an HDR10 title. Pass: a normal-looking (tone-mapped) picture, not grey.
7. *Leave the app:* press Home while a film plays, come back. Pass: paused, the button says Play, one press continues. Do the same with music: it keeps playing.
8. *Route:* pull the network for 20 seconds during a film. Pass: "Reconnecting", then it continues from the same place. Fail: an error card, or a restart from zero.

**F. Quality.** In the player, Quality lists Original, Automatic and fixed qualities; Original is ticked. Choose 720p: the picture returns within about a second of where it was, Up Next is unchanged, the server shows the same session with the next generation. Choose Automatic, then throttle the link to about 3 Mb/s on a 20 Mb/s title: within about 15 seconds a message says the quality was lowered; remove the throttle: it is raised a step at a time. If nothing moves, look in the trace for `throughputKbps` on `fact progress` lines: absent means the measurement is not reaching the player.

**G. Subtitles.** (1) An SRT track: the line appears. In the Subtitles panel change Size and Background: the line changes at once, including "none" (an edge instead of a box). (2) Set a timing offset: the lines move by it. (3) A styled ASS or a PGS track: a warning, then the picture returns with the subtitles in it (burned in); the server shows a conversion. (4) Turn subtitles off. (5) Start the next episode: the same languages are in force.

**H. Live TV.** Tune a channel. Pass: the picture within a few seconds, at the live position. Leave it ten minutes: no stall, no drift, memory steady. Pause two minutes and resume: it continues from where it paused. Seek back a few minutes and forward again. Stay across the top of the hour (a programme change): at most a brief hitch, then it continues. Fail: a freeze at the programme change (then say whether the trace has `hls channel carried from …` lines: that tells which placement mode the device is in).

**I. Redirect check (B-22.19).** Put a proxy in front of the server that answers `POST /v1/…` with `307` to another host that logs what it receives. Sign in. Report whether the second host received the request body and the `Authorization` header. Either way the app must show a failure and stay signed out.

**J. Watch Together.** Create a group on the phone and join it from the television by code. Pass: play, pause and seek on the phone are followed on the television within about a second, and the reverse; the television shows "Waiting for everyone" while the group holds. Leave it idle 35 minutes in the group: still in step afterwards (the stream is reopened every megabyte or 35 minutes). Fail: the television only catches up every several seconds.

**K. Play on.** With "Receive from other devices" on, on the phone choose Play on and pick the television. Pass: the television asks once, then plays; later sends start within a second or two.

**L. A report sent.** Settings › About › Send a report, confirm. Pass: "sent to …"; the owner's console lists a report from platform `vega`, device class television.

## 9. Not finished

- **Nothing was built for or run on Vega.** Every item above is type-checked and unit-tested against stand-ins only. Treat step Zero as the start of testing.
- **HDR is not played as HDR on Vega.** The server tone-maps every HDR title. To claim it: confirm on a device that an HDR10 title sent as it is looks right on an HDR television and acceptable on a non-HDR one, then remove `sdrOnly` in `apps/vega/src/bridge/capability-profile.ts` and state the display's ranges. Without a way to read the television, a viewer setting would have to say it.
- **No surround sound on Vega.** Stereo only; Dolby is never passed on, because no receiver can be detected. Needs an SDK interface for the audio output, or a viewer setting.
- **Live channels are the least certain code in the lane.** Written against the server's playlist writer and tested against a stand-in buffer, in both placement modes, but never against the device's media source. Section 8, H is its test.
- **The capability probe has never been asked of a real device.** If its answers are wrong the declared table is not a safe fallback by itself (the probe replaces it). Section 8, D is its test.
- **B-22.19 is open** (redirects are refused only after they were followed).
- **Display matching** (frame rate, dynamic range) and the **system's caption style** are not done: the SDK gives no way.
- **Automatic quality cannot measure a file played directly over HTTPS**; it reacts to stalls there.
- **A programme change on a device without `sequence` mode** hitches for up to 0.4 s.
- **No gapless, crossfade, levelling or speed** on Vega (hidden); music has no separate listening owner.
- **No nearby discovery, no home-screen rows, no downloads, no cast**: unchanged, by platform or by decision.
- **The Vega screens themselves were not looked at.** Focus, layout and wording are the shared files'; I changed no screen layout, and verified none on a device.
