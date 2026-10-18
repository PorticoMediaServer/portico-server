# Lane report: web-player

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/server-web-player`, branch `audit-implementation/web-player`, 10 commits on top of `0e03d99` (`2699787` … `6a1835d`). Nothing pushed. No full build or full suite was run (rule 3); what was run is listed in section 8.

**Nothing in this lane has been played in a real browser.** Everything below is verified by type-checks and by tests that drive the real playback service, queue player, session controller and media element port against a fake server, a fake `<video>` and a fake hls.js. Section 8 says what to check in browsers.

## 1. Outcome

**One web playback path.** The older player adapter (`web/src/bridge/useWebPlaybackAdapter.ts`, 604 lines, no behavioural test) is deleted, with the code that existed only for it (`web-playback-binding.ts`, `player-audio-mapping.ts` and their tests). The web no longer uses the legacy bridge (`V1PlaybackApi`). What plays now is:

```
PlaybackService            what the screens read and command (shared with the native apps)
WebPlayback                web/src/playback-v1/web-playback.ts: relays intent down, facts up
PlaybackSessionController  the server session: timeline, re-plan, lease, server end
WebMediaPlayer             the <video> element and hls.js (one element for the player's life)
```

- **Protocol v1 only.** Sessions start, change and stop through `SessionsClient`/`PlaybackSessionController`; progress is the controller's timeline (position, state, rate, buffer, bandwidth, dropped frames, volume, mute, generation), 10 s playing and 30 s paused. The old session start, progress and stop calls are removed from `HttpLocalApi`.
- **Quality, version and audio changes reach the server and keep the queue.** They are a `PATCH` of the playing session (same session, same queue entry, next generation), loaded where the picture is. The Audio menu ticks what was committed. Replay and an engine-failure retry restart the current queue entry in place instead of making a one-title queue.
- **Engine failures are reported** (`POST /v1/playback/route-failures`) and the title comes back on the next route up, in the same queue, at the same position. With no route left the viewer gets the "cannot be played here" card.
- **A lapsed lease** (laptop asleep, long outage) starts the same queue entry again at the last position, playing or paused as it was, with no error card. An owner's stop or a transfer is still final.
- **A resume loads from the resume point** (hls.js `startPosition`), not from the start of the title followed by a seek.
- **Slow is not failed.** No wall-clock limit on a fragment that is receiving bytes; nothing restarts a download in progress. A download that receives nothing for 20 s is restarted; playback is given up only after 90 s with an empty buffer and no sign of life (bytes, or the server's "still preparing"). The service's start and seek windows (30 s of silence on web) begin again whenever media arrives, for video and for the music engine.
- **A pause from outside the page is the viewer's** (media key, headset, browser media hub, Picture in Picture window, the system): it becomes intent and is not undone. Media Session is registered for every title, not only music.
- **Closing or reloading the tab** stops the session with its position in a request that outlives the page (frees the stream slot and any conversion at once). Hiding the tab only reports the position.
- **Quality menu**: Original, Automatic, then the fixed qualities below the source (4K, 1080p, 720p, 480p, 360p). A choice holds for the viewing (it carries to the next episode) and a new play starts from the saved default again. **Automatic** measures throughput from fragment downloads, starts at Original, steps down on a sustained shortfall or repeated stalls and back up one step after sustained headroom, with a hold between changes and a longer wait after a step up that stalls. It says so in one line when it changes.
- **Saved defaults**: Settings › Quality & data › Default quality already has one row for home and one for away; the choices are now ordered Original, Automatic, 4K, 1080p, 720p, 480p and the help text explains them. The player reads the row for the network the session is on.
- **Honest error card**: "This can't be played here" (plain reason, Back, Report a problem) when trying again cannot help; "Playback stopped" with Try again otherwise. The queue's own refusals ("can't be played on this device", "converting is turned off", stream limit) are no longer flattened into "Playback could not continue".
- **Keyboard**: Space/K, ←/J, →/L, ↑/↓ volume (channel up/down on live), M, F, I (Picture in Picture), C (subtitles on/off), G/H (subtitle timing ∓0.1 s; G is the guide on live), Z (fill/fit), 0–9, Home, End, `<`/`>` speed, Shift+N/P, Page Up/Down chapters, `,`/`.` frame step while paused, ?, Esc. Shortcuts keep working after a slider is used; Space on a focused button no longer double-toggles. The shortcut list shows all of them, in catalogue strings.
- **Pointer**: a mouse click toggles after a moment in which a second click makes it a double-click (fullscreen), so no pause blip; a finger tap shows or hides the controls and never pauses; a double tap on the left or right third skips, in the middle goes fullscreen; the mouse wheel changes volume.
- **Comforts**: fullscreen falls back to the system's full-screen video on iPhone and says so where there is none; the fullscreen button follows `fullscreenchange`; landscape lock on touch devices in fullscreen; fill-the-screen fit mode; squared volume curve; subtitle timing in 0.1 s steps applied at once and saved after adjusting stops; subtitles survive an outage the video survives; cues sit above the controls in Firefox; Playback info follows a re-plan and has Copy details; the video sleep timer shows its selection and remaining time; a fault while drawing the player stops at the player (error boundary with Try again).
- **Music engine**: a FLAC is started part-way from its frames' own positions (seek table or a bounded, verified search) instead of reading the whole file before the position; reading a file counts as progress; the audio session is declared as playback on iPhone/iPad and an interrupted context is started again.
- **Seam for on-device subtitle rendering**: `web/src/player/subtitle-surface.ts` (`SubtitleSurface`) is the single place a subtitle track is attached to the element. The port only tells it where cues sit on the element's clock (`timeline(offset)`); the cue document is drawn through `show`/`clear`. A styled or image renderer attaches there.

## 2. Decisions I made

1. **`PlaybackService` stays as the screen-facing store.** It is shared with the native apps (copied package), the queue player, listening, live channels and the music engine. Replacing it on web would have meant rewriting all of those; the audit's defects were in the adapter and the bridge beneath it, which is what was replaced. `legacy-bridge.ts` stays in `client-core` only because the native apps still construct `V1PlaybackApi`; its header says so.
2. **A web seek is always local.** The controller's `PATCH {seek}` for converted streams is switched off on web (`serverSeek: () => false`): the server publishes the whole timeline and follows segment requests, which the audit called the better design. Spec §5.7 and §17.4 still describe the `PATCH`.
3. **One `<video>` element for the life of the player** instead of a new one per session. Stale events are fenced by generation in the port. Picture in Picture, fullscreen and autoplay permission now survive a change of title or quality.
4. **Silence windows**: 20 s without a byte restarts a download; 90 s empty with no sign of life gives up; 30 s for the service's start and seek windows, restarted by any arriving byte or "still preparing" answer. A server that holds a request open silently for longer than that still fails; it has to answer something.
5. **Automatic's thresholds**: down after 10 s of measured shortfall or two stalls in 30 s, to the best step the connection carries with a fifth to spare; up one step after 30 s at half as much again as that step needs; 15 s minimum between changes; the wait before going up doubles (to at most 10 min) after a step up that stalls. Direct play has no fragment measurements, so there it reacts to stalls only.
6. **Fixed qualities** are 4K 40 Mbps, 1080p 8, 720p 4, 480p 1.5, 360p 0.8, offered only below the source (a step at the source's own height only when the source's bit rate is 1.5× the step's). One table: `packages/client-core/src/playback-v1/choices.ts`.
7. **Prepared (optimised) copies are no longer a separate row in the player's Quality menu.** Protocol v1 has no selector for one; the planner picks a prepared copy when it fits the requested quality. The title page's "prepared" pick is sent as the fixed quality of that copy's height.
8. **A quality picked in the player holds for the viewing** (the server already carries it to the next queue entry) and is dropped when something else is started.
9. **Closing the tab stops the session; a page kept for Back (bfcache) or a hidden tab only reports.** A live channel's session is not stopped on close (its own control owns it; the lease covers it).
10. **iPhone fullscreen uses the system's full-screen video player** (its own controls; the subtitle lines are real text-track cues, so they show there).
11. **AirPlay is offered only when the element can be handed to a receiver** (`disableRemotePlayback` not set by hls.js): direct play and native HLS yes, hls.js on a media source no. Whether Safari should use native HLS to get AirPlay for converted streams is still open (audit question 2).
12. **Subtitles stay on screen through an outage**; they are cleared only when the server refuses the cue document (401/403/404/410). The 25 s re-check is unchanged.
13. **Volume slider is squared**, not cubic: half the slider is about half as loud.
14. **Frame step assumes 24 frames a second** (the file's rate is not known to the page) and moves the element directly while paused.
15. **End key seeks to five seconds before the end**, not to the end, so a slip does not finish the title.
16. **A queued Next is dropped if the viewer closes the player before it runs**; a second Next pressed while the first is in flight runs after it, from the queue it left.
17. **Ends this device caused itself** (`replaced`, `completed`, `ended`) are ignored as server news, so a notice that overtakes the answer carrying the next session does not fail playback.
18. **A stream the subtitle route re-planned** (burn-in chosen through `/v1/items/{id}/playback/{session}/subtitles`) is loaded as the service was given it when the v1 session itself did not move, and the lease is kept from the service's readings. The proper path is a session `PATCH {subtitles}`; see section 7.

## 3. Bugs

Document 17 (and V09):

| ID | Result |
|---|---|
| B-17.1 | Fixed. Quality, version and audio are a session `PATCH`; choices picked before Play travel with the start. Shared code, so the native queue path gets it too (`V1PlaybackApi.change`). |
| B-17.2 | Fixed. Re-plan keeps the session; Replay and fallback restart the current entry in place. |
| B-17.3 | Fixed. Unsolicited pause/play is intent; Media Session for all titles. |
| B-17.4 | Fixed. Progress-based loading and recovery; recovery ends at the end of a title or once the picture moves. |
| B-17.5 | Fixed. |
| B-17.6 | Fixed as honest gating (AirPlay not offered for media-source streams). Needs Safari + Apple TV to confirm (section 8). |
| B-17.7 | Fixed (`pagehide` keepalive `DELETE` with position; flush on hidden). |
| B-17.8 | Fixed. |
| B-17.9 | Already fixed (fix log); kept. |
| B-17.10 | Fixed. |
| B-17.11 | Fixed. |
| B-17.12 | Fixed. |
| B-17.13 | Fixed (system full-screen fallback, plain notice where none). Needs an iPhone. |
| B-17.14 | Fixed by sizing buffers from the stream's bit rate (120 MB budget, 40 MB on ≤2 GB devices). Not measured. |
| B-17.15 | Fixed (moves serialised, overtaken sessions stopped, retry position bound to its title). |
| B-17.16 | Fixed (real rate in every report; cadence from the reporter). |
| B-17.17 | Partly fixed. FLAC no longer reads the prefix; a slow start or seek no longer fails while bytes arrive. MP3, ADTS and Ogg still read everything before the position (see section 9). |
| B-17.18 | Fixed by code (audio session type, interruption handling). Needs devices. |
| B-17.19 | Not fixed: speculative, needs measurement per browser before adding a silent media element. |
| B-17.20 | Fixed. |
| B-17.21 | Fixed (cues lifted by line where the cue box cannot be moved with CSS). Needs Firefox. |
| B-17.22 | Fixed (the range input takes no pointer events). |
| B-17.23 | Fixed (no size capping; a multi-rendition manifest is pinned to its best level). |
| B-17.24 | Fixed (20 ms tolerance). |
| B-17.25 | Fixed: the shipped path is the tested one. |
| B-17.26 | Sleep selection and "end of title" at speed: fixed. Shortcut list: fixed. Fullscreen icon: fixed. Audiobook speed to 3×: not fixed (the server's listening preferences accept only up to 2×; music-audiobooks lane). |
| N-V09.1 | Fixed, and tested with the wired combination. |

Document 16: A-16.44 (no error boundary around the player) fixed. Nothing else in 16 is player code.

## 4. API changes

No server route was added or changed. Client side:

- Removed: `HttpLocalApi.createPlayback`, `.progressPlayback`, `.stopPlayback` (the legacy `POST /v1/playback/sessions` body `{itemId, quality: "auto", requestId}`, `POST …/progress`, and the plain `DELETE`). `HttpLocalApi` no longer implements `PlaybackApi`. The web calls no legacy playback route; the server lane can delete `POST /v1/playback/sessions/{id}/progress` and the legacy start shape as far as the web is concerned.
- Added: `HttpLocalApi.farewell(path, method, body)` (keepalive request).
- Still used by the player, unchanged: `/v1/items/{id}/playback-offers`, `/v1/items/{id}/playback-options` (Dolby Vision pre-check, durations), `/v1/items/{id}/playback/{session}/subtitles`, chapters, trickplay, `/v1/playback/route-failures`.

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

- No new keys. **Default quality** for home and away is the existing registry composite (`quality.home`, `quality.away`, and `quality.cellular` on phones) over `quality.<network>.mode` and `maxVideoHeight`; screen: Settings › Quality & data. Now read by the web player to tick the menu and to turn Automatic on, and reordered Original first.
- Device-local, no screen: fill/fit (`localStorage` `portico.player.fill.v1`), toggled from More and with Z.

## 7. Needed from others

**playback-core (server)**
- The saved default for **away** still ships as `automatic` with a 20 Mbps cap on the `unknown` network (registry defaults in `server/internal/operations/preferences.go`), which reads as "4K" in settings. The decision is Original everywhere by default.
- The saved default is enforced by the server as a **ceiling** (delivery policy). So choosing Original in the player while the saved default is 720p is still capped at 720p. The saved default must become a starting value the session's own `quality` can override; owner and member caps stay ceilings.
- When `automatic` becomes a server quality value, the single place to re-point is `qualityRequest()` in `packages/client-core/src/playback-v1/choices.ts` (today Automatic sends `original`, then `limit` steps). The step table `fixedQualities` is there too.
- A throughput estimate for the server: the web has it (`AutomaticQuality.estimate`, and `bandwidthKbps` in every timeline report).
- Burn-in subtitle choices should be a session `PATCH {subtitles}`; the legacy subtitle route changes the media session without moving the v1 generation (decision 18).
- The new capability document: `web/src/bridge/client-profile.ts` is untouched by this lane and is where the web's probe is built.

**Every place that talks the protocol** (all behind `client-core` services, as asked): `packages/client-core/src/playback-v1/sessions.ts` (`SessionsClient`, `SessionChange`, `StartOptions`), `choices.ts` (`qualityRequest`, `audioTrackId`: audio ids are `a<stream index>`), `v1-queue-player.ts` (`startChoices`, `changePlayback`), `queue.ts` (`create … startPlayback.quality`), `timeline.ts` (report shape), and `web/src/playback-v1/web-playback.ts` `makeTransport()` (the only web caller: start, change, stop, route failure).

**web-app lane**
- I made three small edits in title-page files so picks before Play are sent: `web/src/screens/detail/PlaybackChoices.tsx` (adds `choices`), `Detail.tsx` and `EpisodePanel.tsx` (pass it to `player.play`). `player.play` gained a sixth argument, `choices`.
- `settings-structure.ts`: `limitChoices` reordered; two help strings and one section title changed in `packages/i18n/src/catalog/en-US/settings.ts`.
- Hard-coded English remains in parts of `Player.tsx` I did not touch (Audio, Subtitles, Speed labels, some notices) and in `client-core` `playback-messages.ts` and the queue player's messages.

**native lanes** (`packages/client-core` is copied there)
- `V1QueuePlayerOptions.api` is now the interface `QueueSessionTransport`; `V1PlaybackApi` satisfies it and gained `change()`, so quality/audio/version changes and in-place restarts work on native through the same queue player. `QueueController.playItem` has a sixth argument and there is a new optional `changePlayback`. `PlaybackService.play` has a sixth argument; new `changePlayback`, `playbackChoices`, `installPresentation`, `engineProgress`.
- `HttpLocalApi` lost the three legacy playback methods; native does not call them (it wraps `V1PlaybackApi`).
- Native player screens should call `service.changePlayback({quality | versionId | audioTrackId})` and drop the prepared/quality-string arguments, as the web did. When native moves to `PlaybackSessionController` too, `legacy-bridge.ts` can be deleted.

**music-audiobooks lane**: audiobook speeds above 2× (B-17.26) need the server's listening preferences to accept them; MP3/ADTS/Ogg part-way starts need a server-side seek index (or exact Xing/CBR handling) to stop reading the prefix; output latency is not subtracted from the music position (A-17.26).

**subtitles-text / the later web subtitle lane**: attach renderers at `SubtitleSurface`; `WebMediaPlayer` no longer creates `<track>` elements for sidecars (`selectSidecarSubtitle` only records the id for the timeline).

**Documents to correct**: `Spec — Playback Protocol v1.md` §5.7 and §17 clarification 4 (web seeks are local); §12 (audiobook speed 3.0 not delivered); `04 Playback and Media.md` (AirPlay: only for streams the receiver can fetch; quality menu wording; Automatic); `06 Clients.md` (the web path is service → controller → port; `src/bridge` no longer holds an HLS adapter); `web-media-player.ts` is no longer "not wired"; Backlog entry on MP3/FLAC part-way starts (FLAC done).

## 8. Test notes for other environments

Run here (all pass): `npx tsc --noEmit` in `web` and `packages/client-core`; `npm run lint:strings` and `lint:design --strict` in `web`; the preference-consumer check; web tests `web-media-player`, `web-playback`, `subtitle-surface`, `now-playing`, `player-shortcut-key`, `playback-traffic`, `entity-play`, `container-play`, `cast-sender`, `dolby-vision`, `live-captions`, `audio-*`; client-core tests `playback-v1`, `playback-v1-core`, `v1-queue-player`, `v1-timeline-cadence`, `playback-choices`, `playback-progress-windows`, `engine-failure`, `seek-reliability`, `playback-recovering`, `services`, `listening`, `v1-native-listening`, `v1-channel-control`, `startup-visibility`, `audio-selection`, `subtitle-presentation`, `playback-settings`, `i18n`. Not run: the whole web or client-core suites, any build, any browser.

Browser pass (Chrome, Firefox, Safari on macOS; Safari on iPhone and iPad; a server with a film library, a season, a music library and live TV). Pass means the stated result with no error card unless one is expected.

1. **Start and resume**: play a half-watched remux (MKV). Network panel: the first media request is the segment at the resume point, not `seg0`. Pass: picture starts at the resume point.
2. **Quality**: in a season queue, pick 720p. Pass: a `PATCH /v1/playback/sessions/{id}`, picture continues within a second of where it was, Up next still lists the next episode, Playback info says converting. Pick Original: back to the file. Fail: a `POST /v1/queues`, or Up next empty.
3. **Audio track** on a two-track file: same checks; the menu ticks the new track.
4. **Replay** at the end of episode 3: episode 3 again, Up next still episode 4.
5. **Media keys / Picture in Picture**: pause from the keyboard's media key, from the browser's media hub, and from the PiP window. Pass: stays paused, Portico's button shows Play. Fail: resumes by itself.
6. **Slow link**: throttle to 1.5 Mbps on a 20 Mbps remux with quality Original. Pass: buffers, "Buffering…", keeps going for minutes; no "Playback stopped". With quality Automatic: within about 15 s a line says the quality was lowered, and the picture continues; un-throttle and after about a minute it is raised one step at a time.
7. **Server restart mid-film**: "Reconnecting…" once the buffer is empty, playback resumes when the server is back (within 90 s), same position.
8. **Sleep**: play, close the laptop lid for five minutes, open. Pass: brief "Reconnecting…", then playback continues at the same position (or stays paused if it was), no error card.
9. **Tab close**: with an account limited to one stream, play, reload the tab, press play again. Pass: plays. Owner's Now Playing shows the old session gone at once.
10. **Engine failure**: a file the browser claims and cannot decode (10-bit H.264 in Safari). Pass: one retry by itself on a converted route. A file with conversion turned off for the account: the "This can't be played here" card with the reason, Back and Report a problem.
11. **Touch (iPhone, iPad, Android Chrome)**: tap shows controls, tap again hides them, never pauses; double tap right skips forward; fullscreen button works on iPhone (system player, subtitles visible).
12. **Keyboard**: every key in the ? list; click the volume slider, then Space still toggles and ←/→ move the slider only while it has focus; Ctrl/Cmd+L still reaches the address bar.
13. **Subtitles**: text track on, G/H nudges move the lines at once; pull the network for 60 s while the buffer plays: lines stay. Firefox: lines sit above the controls. A PGS track (burn-in): picture reloads with the subtitle burned in and playback continues past two minutes (lease kept).
14. **Safari AirPlay**: direct-play MP4 offers AirPlay and it plays on an Apple TV; an MKV (hls.js) does not offer it.
15. **Music**: gapless album still gapless; seek deep into a one-hour FLAC over a slow link starts within a few seconds (network panel: a handful of range requests, not the whole prefix); iPhone: music continues with the silent switch on and after locking; after a phone call ends, it resumes.
16. **Live TV**: tune, pause, go live, channel up/down with arrows, G opens the guide; captions toggle still works.
17. **Watch Together**: with a group, the media-key pause goes to the group like the button does.
18. **High bit rate**: an 80 Mbps 4K remux on Chrome for ten minutes: no repeated stalls, `chrome://media-internals` shows no quota errors.

## 9. Not finished

- **Not run in any browser** (see top). The risk is concentrated in real hls.js and element behaviour the fakes only model: whether a pause event is fired when hls.js rebuilds buffers (would read as the viewer's pause), `playingDate` mapping on live streams, native HLS start positions on iOS.
- **MP3, ADTS and Ogg** music/audiobook files still read everything before the position when started part-way (B-17.17). It no longer fails while bytes arrive, but a deep resume over a slow link is slow. Exact frame positions are not recoverable from a VBR MP3 without reading it or a server index.
- **Audiobook speed above 2×**, **output-latency compensation** for the music position, and a **silent element for media keys with the music engine** (B-17.19): not done.
- **Up next during the credits** (A-17.28): not done. The Skip credits prompt and the post-play countdown are unchanged.
- **Automatic on direct play** reacts to stalls only; there is no throughput measurement without fragments.
- **The saved default's two server-side problems** in section 7 (away default, default acting as a ceiling) make the menu's tick honest about the server but not yet what the decision asks for on remote connections.
- **A live channel's session is not ended when the tab closes**; it lapses with its lease.
- **The Dolby Vision pre-play read** still runs before each video start (A-17.33).
- **Safari stays on hls.js**; native HLS for Safari (AirPlay for converted streams) was not attempted.
- `legacy-bridge.ts` and `PlaybackService`'s own progress/cleanup code remain in `client-core` for the native apps.
