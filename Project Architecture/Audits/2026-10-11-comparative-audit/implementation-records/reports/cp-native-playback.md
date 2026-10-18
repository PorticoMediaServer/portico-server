# cp-native-playback — native and shared playback gaps (15 Oct 2026)

Worktrees: `server-cp-native-playback` and `native-cp-native-playback`, branch `audit-implementation/cp-native-playback`. `audit-implementation/main` has been merged in, including cp-web-playback and cp-polish. No subagents or helpers were used. Nothing has been pushed.

## 1 Outcome

- **Subtitles (B-21.9).** Every subtitle choice is now a change to the playing session (`PATCH /v1/playback/sessions/{id}`). This covers picking a track, Off, a timing nudge and a found track. The separate `PUT /v1/items/{itemId}/playback/{session}/subtitles` route is gone, along with its unused controller and reservation hooks.
  - On the title page, native and web list every subtitle track separately and identify each by its own ID. That includes tracks with no language, SDH and forced tracks, and two tracks in the same language.
  - If the session doesn't offer the chosen track, nothing else is picked silently in its place.
  - New "Forget last choice" control: when the server is reusing the profile's last audio and subtitle choice for a show or film, the viewer can clear it.
- **Live TV tuner conflicts (B-11.27), following your 15 Oct decision.** New owner setting: "When a recording needs a tuner someone is watching".
  - **Keep watching (default).** Live viewing is never interrupted and no viewer is given a deadline. The recording waits for a free tuner. If one frees up it starts late and is kept as a partial recording. If none frees up it fails with its own reason, "Live TV viewing is using every tuner". The owner gets one notice that says, in turn, waiting, then started late or did not record.
  - **Stop watching.** Pre-emption works as before. Every player (web, phone/tablet, TV, Vega) shows the time the tuner will be taken, a countdown in the last minute, and what happens next. At that moment the player stops the channel itself and shows "Live TV stopped", with no retry loop.
- **Downloaded styled subtitles** now use their own attached fonts in mpv on Android and Apple. The fonts are copied into a folder of their own, because mpv reads every file in a fonts folder.
- **Apple listening with a bitrate limit (B-10.11).** When the limit is below a lossless file's bitrate, the server now converts to AAC-LC in ADTS within the limit. Apple's own renderer accepts it. Before, Apple got no gapless plan at all.
- **Android downloads (B-14.20).** A system job (JobScheduler) brings the app back and continues unfinished downloads from their saved bytes while the app is closed.
- **Download expiry clock (B-14.21).** Expiry is now judged by a clock that can't be turned back. It is reset to the server's time whenever the server issues or renews a receipt.
- **Layout (B-19.10).** Android now treats a box with no touchscreen as a TV, and has a Layout setting: Automatic, Television or Touch. The app restarts to apply it.
- **Small native leftovers (B-19.19).** The unused native search view is retired, and the manifest now explains why `adjustResize` stays.
- **Chapter pictures** now show in the native chapter list. Native already had trickplay.
- **Extras from the coordinator.**
  - Casting from native now sends the subtitle on screen, or Off.
  - Native has "Forced only" for image subtitles (mpv `sub-forced-events-only`), remembered on the device.

## 2 Decisions I made

- **Your decision (15 Oct), B-11.27:** tuner pre-emption is an owner setting. The default is that the recording does not start and live viewing is never interrupted. "Stop watching" keeps the countdown on every player. I implemented it as written.
- **B-11.27 extra (you left this to me):** under Keep watching, if a tuner frees up during the programme, the recording starts then and is kept as a partial recording ("incomplete-playable"). The owner's notice changes to "Recording started late". In this mode a viewer is never refused because a recording is due soon, and never given a deadline; live always wins.
- The setting lives in the existing DVR settings document (`/v1/admin/dvr/settings`, `liveTunerConflict`), stored in `dvr_defaults.live_tuner_conflict`. It shows in the shared Settings structure under Recording defaults, on web and on phone/tablet owner pages. TV owner pages are read-only and don't show it.
- **Countdown timing:** the time the tuner goes is shown when it is more than a minute away, with a seconds countdown inside the last minute. It uses the device clock against the server's deadline, so clock skew shifts it slightly. The server's own stop is the authority.
- **Pre-play subtitles:** tracks come from `playback-options` `versions[].subtitles`, which uses catalogue resource IDs (these already include discovered tracks). I did not add a discovery-ID path; a resource ID identifies a track exactly. The old by-language list is used only when the options can't be read.
- **Forget last choice:** the route is `DELETE /v1/items/{itemId}/playback-memory`. Per title only; there is no "forget everything" for a profile.
- **AAC target:**
  - Format is ADTS, which can be read as it streams. MP4 needs a finished file.
  - Priming is FFmpeg's 1024 frames, stated in the plan's trim as `aac-priming`.
  - Bitrate is 192 kbps stereo, or the viewer's limit if lower.
  - Below 16 kbps nothing is converted. The limit is never quietly raised.
  - Rate is the source's own if it is 44.1 or 48 kHz, otherwise 48 kHz.
  - Apple now declares `adts/aac` in its capability document, which it did not claim before.
- **Download job:** uses platform JobScheduler, so no WorkManager dependency. It is not persisted across reboots (that would need `RECEIVE_BOOT_COMPLETED`); after a reboot, the next app launch resumes downloads as before. It needs an unmetered network unless "use mobile data" is on. An expired download address can't be renewed without the app, so that download waits for the app.
- **Receipt clock:** a stored floor that only moves forward (it is only written when it moves by more than a minute), reset to the server's `issuedAt` on every issue or renewal. That reset means a clock that was wrongly far ahead recovers once the device is online again.
- **Layout detection:** the app is laid out for TV if any of these is true: Fire TV build, Leanback, TV UI mode, or no touchscreen on a device that isn't a PC. The setting overrides detection. Changing it restarts the app (start the launch intent, then kill the process), because density is fixed at process start.
- **Search keyboard:** I retired it rather than mounting it. The Apple view was never in the Xcode project. The Android view duplicated React Native's text field. The existing field already opens the system keyboard with dictation on every device. `adjustResize` stays: with edge-to-edge drawing it resizes nothing, but Android 10 and earlier send no keyboard insets without it.
- **Chapter pictures:** read from the session's chapter list (`thumbnailUrl`, first 100) and matched to the title's chapters by start time.
- **Casting:** sends the track actually on screen (`shownId`), so a track mpv reads from the file is included.

## 3 Audit bugs closed and decision gaps closed

| ID | Change | Test |
|---|---|---|
| B-21.9 | Subtitle PUT route deleted (server, contracts, lanes, matrices). `SubtitleService` no longer selects. Native picks tracks, Off, timing and found tracks via session PATCH. Pre-play lists tracks by identity on native and web. Exact-ID rule in `pending-track-choice`. Forget-memory route and controls. | Go: `TestPlaybackV1ClientReadsWithTheV1SessionID` (PATCH fence, PUT gone), `TestSubtitleReplanRoutesHoldToTheMembersCurrentCeiling`, `TestPlaybackV1RemembersTrackChoices` (forget), restriction and scope matrices. Core: `pending-track-choice.test.ts` (new), subtitles and subtitle-presentation tests updated. |
| B-11.27 | Owner setting, allocator policy, DVR worker wait/late/fail and owner notice, `ErrLiveInUse`. Client-core `live-handover.ts` plus automatic stop in `PlaybackService`. Banner and stopped state on web and native (phone, TV, Vega). DVR reason text. | Go: `TestKeepWatchingNeverPreemptsALiveViewer`, `TestSharedChannelPreemptionNotifiesEveryViewerOnce` (stop-watching), `TestRecordingAgainstALiveViewerFollowsTheOwnersSetting` (waits → late start; no tuner → fails with notice; stop-watching → countdown), `TestDVRTemplatesAndTunerConflicts` (setting). Core: v1-channel-control countdown/stop test, administration parse. |
| B-10.11 | `audio_aac` recipe, AAC plan selection, `audio/aac` route, Apple `adts/aac` declared, Apple renderer takes priming from the plan for ADTS. | Go: `TestAudioConversionAACStartsAfterItsPriming` (real FFmpeg; track starts exactly at 1024), `TestPlaybackV1LimitedAppleListeningConvertsToAAC`. Native: capability-profile test. |
| B-14.20 | `PorticoDownloadJob` (JobScheduler) plus `PorticoDownloadCenter.resumeUnfinished`. | Kotlin type-check only. |
| B-14.21 | `receipt-clock.ts`, used for the playable check and renewal timing. | `receipt-clock.test.ts` (new). |
| B-19.10 | Layout setting (shared row, Android preference, restart), no-touchscreen detection, `device-kind.ts` follows the runtime both ways. | Settings structure tests; Kotlin type-check. |
| B-19.19 | Search view retired; `adjustResize` reconciled and documented. | Kotlin type-check. |
| Offline fonts gap (Appendix 0) | Local font gathering in both mpv controllers. | Kotlin type-check; Swift not compiled. |
| Chapter/trickplay imagery gap | Native chapter list shows the server's chapter images; trickplay was already shown. | Type-check. |

## 4 API changes

- **Removed:** `PUT /v1/items/{itemId}/playback/{session}/subtitles`. Use the session PATCH (`subtitles`, `subtitleOffsetMs`). The `X-Playback-Controller-Token` header is no longer read anywhere; its CORS entry is left in place.
- **Added:** `DELETE /v1/items/{itemId}/playback-memory` (`forget_playback_track_memory`, viewer-item access, playback scope, returns 204).
- **Changed:**
  - `GET/PUT /v1/admin/dvr/settings`: `settings.liveTunerConflict` (`keep-watching` | `stop-watching`) and `enumerations.liveTunerConflict`.
  - Audio render plan: converted mode can now be `container: "adts", codec: "aac"` with trim source `aac-priming`. `/audio-converted` serves `audio/aac` for it.
  - DVR recording reason `tuner-in-use-live`.
  - Channel session end on the client: `ended.reason = "tuner_reserved"` (client-side only).
- Contracts regenerated with apigen.

## 5 Schema changes

`dvr_defaults.live_tuner_conflict TEXT NOT NULL DEFAULT 'keep-watching' CHECK(IN('keep-watching','stop-watching'))`, added to the baseline (no migration) and synced with `schema-sync.py`.

## 6 Settings added

- **Owner:** "When a recording needs a tuner someone is watching". Keep watching (the recording does not start) is the default; the other option is Stop watching (the recording takes the tuner). In the DVR document; shown on web and phone/tablet.
- **Device (Android):** Layout: Automatic / Television / Touch.
- **Device (native):** Forced only, in the player's subtitle menu for image tracks drawn by mpv.
- **Per title:** Forget last choice, in the title page's subtitle menu (native) or as a button (web).

## 7 Tests run

| Command | Result |
|---|---|
| `go test ./internal/playbackv1/ ./internal/subtitles/ ./internal/livechannels/... ./internal/administration/ ./internal/persistence/ ./internal/decoder/ ./internal/playback/ ./internal/subtitlevideo/` | ok |
| `go test ./internal/httpapi/` | Everything I touched passes. 2 failures are unrelated: `TestListeningHTTPPreferenceEnvelopeAndCAS` needs a reviewed helper binary on this host; `TestPlaybackV1QueueQueryCapturesBeyond500AndKeepsItsSnapshot` failed under full-suite load and passes on its own (timing). |
| `npm run test:core` | 1965/1965 |
| `web: npm run check` (tokens, design lint, oxlint hooks, strings, tsc, tests) | pass (581 tests) |
| `apps/native: tsc --noEmit`, `npm run lint:hooks`, `npm test` | pass (680) |
| `apps/vega: tsc --noEmit`, `npm test` | pass (87) |
| Android Kotlin type-check (cached Kotlin 2.1.20, the same harness as android-native-review, google flavour) | exit 0 |
| Swift / Objective-C | Not compiled; not possible on Ubuntu. |

## 8 Device test notes (Mac and devices)

**Apple — compile first.** Changed files:
- `Native/PorticoMpvSupport.swift`: new `gather`, the `gathering` counter, `cancel` change.
- `Native/PorticoMpvController.swift`: fonts for local files; `sub-forced-events-only`.
- `Native/PorticoAudioRender.m`: ADTS priming branch.
- `Native/PorticoSearchKeyboard.m`: deleted. It was not in either Xcode project.

**Apple checks:**
1. **AAC under a limit.** Set a listening limit of 256 kbps or lower on iPhone and Apple TV and play a FLAC album. The plan should be converted AAC/ADTS. Check it starts promptly, joins gaplessly, a deep seek (`fromFrame`) is fine, there is no clipped first syllable and no extra 1024-frame lead-in.
   - Also check that AudioFile with an ADTS hint and unknown length (`1<<40`) does not scan the whole stream.
   - A conformance run should include converted ADTS.
2. **Downloaded fonts.** Download an ASS title with attached fonts, go offline, play it in mpv. The fonts must be used, not a fallback.
3. **Forced only.** Turn it on for a PGS track that has forced lines; only those lines should show.

**Android:**
1. **Download job.** Start a large download, swipe the app away (or force-stop it with `adb shell am kill`), and stay on Wi-Fi. The job should continue the download.
   - With "Wi-Fi only" on and mobile data in use, it waits.
   - Check `adb shell dumpsys jobscheduler | grep 4102`.
   - On API 34+, the job may be stopped after about 10 minutes; check it resumes on the next run.
2. **Layout.**
   - A generic box with no touchscreen: Automatic should give the TV layout with focus.
   - Settings → Appearance → Layout → Touch: the app restarts with the touch layout. Television on a tablet: the TV layout.
   - Fire TV and Android TV: unchanged.
3. **Keyboard.** On Android 10 and 14, keyboard padding in forms (sign-in, search) is unchanged.

**All players — tuner conflict.** Use a tuner with one tuner and two profiles.
- **Default setting:** watch channel A, then a recording on channel B comes due. Viewing must not stop. The recording shows "Live TV viewing is using every tuner". The owner gets the notice "Recording is waiting for a tuner". Stop watching: the recording starts and the notice becomes "Recording started late". Watch past the programme's end: the recording fails with the notice.
- **Stop watching:** web, iPhone, Apple TV, Android TV and Vega each show the time, then a countdown, then "Live TV stopped" with Close / Back to guide, and do not reconnect.

**Subtitles:**
- A title with two English tracks and one with no language: each shows on the title page and plays exactly that track.
- Forget last choice clears the remembered language.
- Cast from iPhone with PGS chosen: the TV shows it burned in. With Off: no subtitles.
- Native timing nudges (±0.5 s) still apply.

**Chapters:** the native chapter list shows chapter pictures for a title with chapter images.

## 9 Not finished

- No Swift or Objective-C compile or device run. Everything Apple listed in section 8 is unverified.
- No physical run of the Android job, Layout restart or fonts. No end-to-end AAC playback on a real Apple renderer; only the server-side priming is proven.
- The B-10.11 FFmpeg priming (1024) is measured with FFmpeg's decoder. Core Audio's raw decoder with no priming method is expected to match, but that is not proven.
- The download job does not survive a reboot (by choice, section 2). It does not use API 34 "user-initiated data transfer" jobs, so very long transfers continue in roughly 10-minute job runs.
- Native "Forced only" is shown for every image track mpv draws, not only tracks that mix forced and ordinary lines (the web can tell; native can't). It depends on mpv honouring `sub-forced-events-only` in our 0.41 build.
- The countdown uses the device clock, so there is no skew correction.
- `X-Playback-Controller-Token` is still in the server's CORS allow-list and is still sent by `PorticoListeningOwner.m`. Both are now no-ops; I left them to avoid touching unrelated files.
- The DVR owner notices are English server strings, like the existing notification pattern.

## Commits

**portico-server** (`server-cp-native-playback`, not counting merges of main):
- a67ca446 Retire the standalone subtitle selection route: a subtitle choice is a change of the playing session
- 630ed79f Let the owner choose what a recording does to someone watching its tuner, and keep watching by default
- 41d6e38f Give a device without Opus AAC when its listening bitrate is limited
- 84c8c818 Offer a Layout setting where a device can be laid out for a television or for touch
- 9fd4eda4 Keep the tuner countdown banner on the player's own stacking
- (merges of `audit-implementation/main`: 5b220cb6, d7cae9e3)

**portico-react-native** (`native-cp-native-playback`, not counting merges):
- 44e8bb6 Choose subtitles on the playing session, list pre-play tracks by identity, and count live viewers down before a recording takes their tuner
- 6808c45 Draw a download's styled subtitles in their own fonts on Android and Apple
- 020d600 Accept the server's limited-bitrate AAC on Apple's own audio renderer
- 72db193 Carry downloads on while the app is closed on Android, and judge receipt expiry by a clock that cannot be turned back
- 5440366 Let a device be laid out for a television or for touch, and treat a box with no touchscreen as a television
- 0722840 Bring in the shared Layout setting row and its words
- 515e216 Keep one search field on every device and say why the window keeps adjustResize
- 3c37303 Show each chapter's picture in the player's chapter list where the server has made one
- 653decc Send the subtitle on screen (or Off) with a title cast from a phone or tablet
- 59e5145 Offer Forced only for image subtitles drawn on the device, kept by the device as on the web
- 6b35dcf Record the shared packages' source revision after bringing in main
- (merges of `audit-implementation/main`: 0fbdf3b, e6c4991)

Notes:
- `node_modules` in the server worktree is a private `npm ci` install (git-ignored), not a symlink, so the main checkout was not written to. Native `node_modules` are git-ignored symlinks.
- Native commit 5440366 on its own does not type-check; the mirror commit 0722840 that follows it fixes that.
- GOCACHE has been deleted.
