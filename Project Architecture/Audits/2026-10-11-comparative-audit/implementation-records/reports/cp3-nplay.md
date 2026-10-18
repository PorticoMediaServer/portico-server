# cp3-nplay: native playback

Branches: `audit-implementation/cp3-nplay` in `server-cp3-nplay` and `native-cp3-nplay`. `audit-implementation/main` is merged into both: server at 314d250d, native at 401fd5d, plus one test fix after it. Contracts are regenerated with `apigen`. The shared packages are re-mirrored, and `--check` says they are identical to server 314d250d. No subagents or helpers were used. Nothing was pushed.

## 1 Outcome

All thirteen N items are coded, along with both of Justin's additions (volume, and the Vega HDR/surround setting).

Kotlin compiles: `_program/native-ktcheck.sh` exits 0. TypeScript type-checks on web, native and Vega. Go builds, and the touched Go tests pass.

The Apple code (Objective-C) could not be compiled on this machine. Section 8 lists what to check on the Mac.

What changed for viewers:
- Native players show the Up Next card while the credits run. "Start the next episode during the credits" is honoured, and it stays off by default.
- Audiobooks go to 3× on native.
- A player chosen inside the player cannot leak into the next fresh playback on a slow link.
- "Too slow" names the quality that would fit, on web and native.
- A running session is told when the account's allowed hours close.
- The tuner countdown runs on server time.
- iOS downloads keep their bytes when the transfer address changes.
- Downloads carry chapter thumbnails and scrubbing previews.
- The old offline players are gone.
- Android mpv:
  - deinterlaces;
  - draws Dolby Vision profile 5;
  - finds Fire OS receivers;
  - picks subtitle fonts by the script the lines are in.
- Android downloads survive a restart of the device.
- Session-family retirement is decided in the shared code.

## 2 Decisions I made

- **N1:** the web rules (`upNextOffer`, `upNextCountdown`) moved to `client-core/src/up-next.ts`, so web and native share one rule set. The native card takes TV focus when it appears, as the Skip button does.
- **N10:** a fresh playback still waits up to 4 s for the Settings' player to reach the server. If the session started first, planned from the stale player, it is re-planned in place once the server has the setting. The wait for that is up to 30 s. The re-plan uses the existing `replanEngine` path, and only runs when the setting the server last received differs.
- **N18:** the server sends its clock (`linear.nowMs`) with a tuner deadline. The client moves the deadline onto the device clock when the response arrives, so every player and the automatic stop need no other change.
- **N20:** the server sweeps once a minute. It reads the accounts with running sessions (at most 256) and checks the schedule once per account. Sessions outside their hours end with reason `outside_allowed_hours`, which the existing `session.updated` event carries. All session kinds end, including Live TV, because admission refuses them all.
- **N16:** libass in our engine has no way to look up system fonts. Instead, Android reads the first few subtitle lines (mpv `sub-text`). When they are in a script the chosen font cannot draw, it links that script's system font and opens the track again, once per choice.
- **N17:**
  - The job is persisted, with `RECEIVE_BOOT_COMPLETED`.
  - On Android 14 and later, a download the person starts is scheduled as a user-initiated data transfer (`RUN_USER_INITIATED_JOBS`), with the existing download notification.
  - If the system refuses that, the ordinary job is scheduled instead.
- **N12:** inside the resume data, the download address and both archived requests are rewritten to the new address. This handles the plain and keyed-archive forms. The server's ETag is the file's SHA-256 under any address, so If-Range holds. Data that cannot be read, or that the system refuses, starts the transfer again, as before.
- **N13:**
  - Previews are fetched under the transfer address through three new grant routes.
  - Chapter images are listed only where one exists (`playback.ChapterThumbnails`, now exported).
  - The trickplay set is listed only when it is current and made from the downloaded file.
  - `BUNDLE_COMPLETE` is now 4, so existing downloads fetch their previews on the next pass.
  - `useArtwork` passes `file://` paths straight through.
- **Volume (Justin):** native, TV and Vega already had no app-level volume or mute. The engines' volume is used only for gain, crossfade and audio-focus ducking. The iPhone/iPad slider is the system volume (`MPVolumeView`), so it was kept. Nothing to remove.
- **Vega HDR/surround (Justin):** two switches, both off by default:
  - "This TV supports HDR" claims HEVC 10-bit in HDR10 and HLG, for the display too.
  - "This TV supports surround sound" sets the output to 8 channels and lists AAC 5.1, AC-3 5.1 and E-AC-3 7.1.
  - They appear only where the device cannot detect these things (`declaredOutputs`, which is Vega only).
  - On Vega, "Turn off HDR" stays hidden.
- **`paused-load.ts` (coordinator's request): not removed.** Removing it means carrying a paused start through `create`/`startPlayback` and the native queue owner. That is not a clean removal. It still works and has its own test.

## 3 Audit items closed

| Item | Change | Test |
|---|---|---|
| N1 | `client-core/up-next.ts`; native `UpNextCredits.tsx` (card, early start only with the setting on, autoplay on and `automaticSafe`; advances with `advanceFromCredits`); while it shows, the Skip button for credits is off | web `up-next-credits.test.ts` (moved import) |
| N2 | JS `ratesFor(kind)` (audiobooks use `audiobookRates`); Android playback controller and listening owner `BOOK_RATES`; Apple audio render, AVPlayer (with `canPlayFastForward`) and listening owner accept 2.5× and 3× for books | Kotlin compile check |
| N10 | `engineSettingStore.published()`; re-plan effect in `usePlayer` | `engine-setting-store.test.ts` (new case) |
| N12 | `PorticoDownloadRetargetResume` in `PorticoDownloadAttempt.h`, used in `beginTransfer` | Mac: `test/native-download-attempt-tests.m` (new cases) |
| N13 | server bundle `trickplay` + chapter `image`; routes `get_download_trickplay_thumbnails`, `get_download_trickplay_tile`, `get_download_chapter_image`; client-core parse; native fetch, `offlineTrickplay`/`offlineChapters`; `TrickplaySet.tileUrls` | core `downloads.test.ts`, native `offline-extras.test.ts` |
| N14 | removed `PorticoOfflinePlayerActivity`, `PorticoDownloads.play`/`stop` and the `PorticoDownloadPlayback` event (Android + Apple), the JS `play` and the event listener | `offline-ownership.test.ts` moved onto `localMedia` |
| N15 | mpv `deinterlace=auto`, `softwareDeinterlace: true`; `gpu-next` output for DV profile 5 (`dolby-vision-profile` from the track list), never direct output for it; Fire OS: HDMI plug broadcast and `external_surround_sound_enabled` observed, route `hdmi` from the plug | Kotlin compile check |
| N16 | `PorticoMpvFonts.scriptOf/selectFor`, `SubtitleText` event, `fontForText` | Kotlin compile check |
| N17 | persisted job, user-initiated on API 34+, manifest permissions | Kotlin compile check |
| N18 | server `LinearView.NowMS`; `parseChannelSession` converts the deadline | `v1-channel-control.test.ts` (new case) |
| N20 | `EndOutsideHours` sweep; `OutsideHours` wired from `access.Enforcer`; `player.ended.outsideHours*` strings; web StateView; native `endedCopy`; `sessionEndMessage` also names `paused_timeout` | `TestEndOutsideHoursEndsTheAccountsSessionsWithTheReason` |
| N21 | `PlaybackApiError.reason`; `connectionTooSlow()` in client-core; web and native messages | `quality-too-slow.test.ts` |
| N22 | `sameSignIn` in `viewer-selection.ts` (server + family); native local guard removed | `viewer-selection.test.ts` (new case) |

## 4 API changes

- `linear.nowMs` (optional, sent with `reservationRequiredAtMs`).
- Download bundle:
  - `trickplay` (id, geometry, interval, frameCount, durationSeconds, sourceOffsetSeconds, thumbnails, tiles);
  - `chapters[].image`.
- New grant routes, all scope Open and lane media:
  - `GET /v1/downloads/artifacts/{grant}/trickplay/{set}/thumbnails.vtt`
  - `GET /v1/downloads/artifacts/{grant}/trickplay/{set}/tiles/{tile}` (`N.jpg`)
  - `GET /v1/downloads/artifacts/{grant}/chapters/{source}/{index}/image`
- Each new route is added to the restriction matrix and the lanes table.
- New session end reason: `outside_allowed_hours`.

## 5 Schema changes

None.

## 6 Settings added

Vega device settings `tvHdr` and `tvSurround` (device-kept, default off, shown only with `declaredOutputs`).

## 7 Tests run

- client-core tests (viewer-selection, quality-too-slow, v1-channel-control, downloads, settings-scope, trickplay): **61 pass**.
- Type-checks and lints:
  - client-core `tsc`: clean.
  - web: `tsc` clean, `lint:hooks` clean, `lint:strings` clean, `up-next-credits` 3 pass.
  - native: `tsc` (app and tests) clean, `lint:hooks` clean, `lint:strings` clean.
- native tests (engine-setting-store, offline-extras, offline-ownership, session-intents, session-ended): **71 pass**. One expectation in `offline-ownership` was updated from main's `bundle 3` to `4`.
- Vega: `tsc` clean, `capability-profile` **9 pass**.
- Go:
  - `go build ./...`;
  - `go test ./internal/playbackv1 -run 'TestEndOutsideHours|TestPausedTimeout'`, `./internal/downloads -run Bundle`, `./internal/httpapi -run 'Restriction|Lane'`: all ok.
- Kotlin: `native-ktcheck.sh` exit 0, after both merges.

## 8 Device and Mac checks

**Mac: compile these Objective-C files**
- `PorticoDownloads.m`: the AVKit import was removed and AVFoundation imported.
- `PorticoDownloadAttempt.h`: the retarget helper.
- `PorticoListeningOwner.m`: `ListeningBookRate`.
- `PorticoAudioRender.m`, `PorticoPlaybackController.m`.
- Then run `test/native-download-attempt-tests.m`. It should print "PASS … moved address".

**iPhone downloads**
- Start a large download, leave it until its address has to be renewed (or force a new grant), then continue. It should carry on from its bytes; check that `bytesDone` does not reset.
- A finished download should play only in Portico's player, with chapter thumbnails in Chapters and a preview card when scrubbing, in flight mode.

**Up Next (phone, TV, Fire TV, Vega)**
- Use an episode with a sure credits marker, autoplay on, and the setting off. The card shows "Starts in m:ss" and nothing starts early.
- Turn the setting on. The card counts the post-play countdown and the next episode starts. Pause holds the count.
- On TV, Select on the card plays the next episode.

**Audiobooks**
- 2.5× and 3× are in the Speed menu on iPhone and Android, and play at that speed (listening owner too).
- Music still stops at 2×.

**Player choice on a slow link**
- Choose mpv in the player, close it, then on a throttled link start another title.
- If it first opens in mpv, it should switch to the Settings player in place within a few seconds of the capability PUT.

**Allowed hours**
- Give a member hours that end in 2 minutes and play.
- Within about a minute of the end, every player shows "Viewing hours are over" (web, phone, TV, Vega).

**Tuner countdown**
- With "Stop watching" set, put a device clock 5 minutes off. The countdown still matches the server.

**Android mpv**
- Play interlaced MPEG-2 or H.264 1080i (DVD rip, broadcast recording). There should be no combing, and check that the CPU keeps up on a TV box.
- Play a Dolby Vision profile 5 file with Player = mpv. The colours should be correct, not green or purple.
- Play an untagged Arabic or Chinese text track. After the first line, the glyphs should be right (one short redraw is expected).

**Fire TV**
- With a receiver over HDMI and Fire OS "Surround sound: best available" set, the capability document should list Dolby passthrough and route `hdmi`.
- Toggling the setting should republish.

**Android 14+ downloads**
- Starting a download shows the download notification. A 2 GB transfer should not stop at 10 minutes with the app swiped away.
- Reboot mid-download. It should carry on once a network is there (`adb shell dumpsys jobscheduler | grep 4102`).

**Vega Settings**
- "What this TV can show and play" shows two switches.
- Turn HDR on: an HDR title is no longer tone-mapped. Turn surround on: 5.1 stays 5.1.

**Sign-in**
- Restore a saved sign-in whose token was refreshed. There should be no logout of the family (DELETE /v1/sessions/current is not sent).

## 9 Not finished

- `paused-load.ts` is not removed (see section 2).
- Mixed-script lines on Android mpv: the font follows the main non-Latin script. Arabic plus Latin on one track can still box the Latin letters when the Arabic face lacks them. True per-glyph fallback would need a libass patch or a font provider in the engine.
- N13: the web player has no offline downloads, so nothing applies there. Chapter images for audiobooks are not carried, because books have none.
- No render tests for the native Up Next card.

## Commits

portico-server:
- 49deea52: shared credits Up Next, too-slow quality, family retirement, tuner server time, allowed hours.
- 40c4d4eb: Vega HDR/surround settings.
- ab2a0175: download previews.
- 3b33fe26, 314d250d: merges of main.

portico-react-native:
- cedf487: native player follow-ups.
- ecec9b5: Vega declared outputs.
- 3e3e63b: old offline players retired.
- 93d0756: Android download job.
- 81abb84: Android mpv deinterlacing, DV5, Fire OS.
- 69a2288: subtitle font by script.
- 97af201: iOS resume retarget.
- a88adc6: offline previews.
- abcfc5a, 401fd5d: merges of main.
- f0909c4: test expectation.
