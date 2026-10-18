# 17 — The web player

Video and audio playback in the browser: engine choice, HLS and Media Source handling, direct play, capability detection, subtitles, track switching, seeking, buffering and error recovery, player chrome and controls, keyboard and media-key integration, Picture in Picture, fullscreen, casting from the browser, and stats.

This is a static source review. Nothing here was run. Where a finding depends on how a browser behaves at run time, it says so.

Paths: Portico paths are relative to `/home/justin/projects/Portico`. Comparison paths start with the repository name (`jellyfin-web/…`, `jellyfin-vue/…`, `audiobookshelf/…`, `plezy/…`).

## Scope and sources

**Portico, read in full**

- Docs: `portico-internal/Project Architecture/04 Playback and Media.md`, `06 Clients.md`, `Backlog.md`, `Specs/Spec — Playback Protocol v1.md`, `portico-internal/Product Contracts/playback/contract.md`.
- Web player: everything under `apps/portico-server/web/src/player/` (`Player.tsx`, `engine.tsx`, `options.ts`, `Timeline.tsx`, `PostPlay.tsx`, `SkipPrompt.tsx`, `PlaybackInfo.tsx`, `PlayOn.tsx`, `Lyrics.tsx`, `LiveCaptions.tsx`, `dolby-vision.ts`, `page-visibility.ts`, `owner-stop.ts`).
- Web bridge: `apps/portico-server/web/src/bridge/useWebPlaybackAdapter.ts` (the adapter that is actually wired), `web-playback-binding.ts`, `player-audio-mapping.ts`, `useSystemListening.ts`, `system-listening-state.ts`, `sync-rate.ts`, `client-profile.ts`, `cast.ts`, `DecodeAudioRender.ts`, and the audio engine under `bridge/audio/` (`engine.ts`, `track.ts`, `decoder.ts`, `bytes.ts`, `converted.ts`, `stretch.ts`, `capabilities.ts`, `mp3.ts`, `demux.ts`, `types.ts` in full; `resampler.ts`, `flac-decoder.ts`, `flac.ts`, `mp4.ts`, `ogg.ts` headers and seek paths only).
- `apps/portico-server/web/src/playback-v1/web-media-player.ts` (a second web adapter that is not wired).
- Client core: `packages/client-core/src/index.ts` (`PlaybackService`, lines 561–1666), `client-profile.ts`, `playback-v1/legacy-bridge.ts`, `playback-v1/timeline.ts`, `playback-v1/sessions.ts`, `playback-v1/v1-queue-player.ts`, `playback-v1/audio-render.ts`, `audio-effects.ts`, `player-offers.ts`, `subtitles.ts`, `segment-markers.ts`, `player-chapters.ts`, `post-play.ts`, `playback-messages.ts`; `audio-selection.ts` first third.
- Patched hls.js: `apps/portico-server/third_party/hls.js/README.md`, `anchored-fragment-boundary.patch`, `provenance.json`, and four source files read out of the vendored tarball (`stream-controller.ts`, `buffer-controller.ts`, `is-supported.ts`, `config.ts`) to check exact behaviour.
- Server, only to understand what the player talks to: `server/internal/playback/hls_timeline.go`, `hls_copy_timeline.go` (header), `hls_restart.go`, `hls_bad_network.go` (header), `route_failures.go`.
- Tests: names of everything in `web/test`; `playback-traffic.test.ts`, `playback-completion.test.ts`, `audio-render.test.ts` opened.

**Comparison, read**

- jellyfin-web: `src/plugins/htmlVideoPlayer/plugin.js` (all 2,419 lines), `src/components/htmlMediaHelper.js` (all), `src/components/playback/playbackmanager.js` (track switching, seek and `changeStream`, progress timer, error handling, bitrate detection, unload), `src/scripts/browserDeviceProfile.js` (first 330 lines in full, then every browser-specific branch and its comment), `src/apps/legacy/controllers/playback/video/index.js` (keyboard map, pointer handling, up-next trigger, trickplay bubble), `src/plugins/htmlAudioPlayer/plugin.js` (normalisation, Safari notes), media session subscriber and subtitle settings (option list).
- jellyfin-vue: `packages/frontend/src/components/Playback/PlayerElement.vue`, greps of `store/player-element.ts`, `store/playback-manager.ts`, `MusicVisualizer.vue`.
- Audiobookshelf: `client/players/PlayerHandler.js` (progress sync), `LocalAudioPlayer.js` (outline).
- Plezy: `README.md` playback feature list only (Plezy is a Flutter app on mpv, not a browser player).

**Not reached**

- `jellyfin-web` Chromecast plugin, book/PDF/photo/YouTube players, `playerstats.js` body, `upnextdialog.js` body.
- jellyfin-desktop (mentioned only as an architectural alternative).
- Portico's Cast receiver page and `client-core/src/cast/*` beyond the web sender file (domain 13).
- Plex Web: nothing was fetched this session. Statements about Plex are from general knowledge of its public documentation and are marked as such.
- Portico's `listening.ts` was skimmed, not read line by line.

## How Portico does it today

### One player, one element, two engines

The web app mounts a single `<video>` element for everything: films, episodes, live channels, and, when the music engine is off, songs and audiobooks (`web/src/player/Player.tsx:427`). The element is re-created (React `key`) whenever the playing resource changes (`web/src/bridge/web-playback-binding.ts:3`).

Two engines sit behind it:

1. **Video and ordinary audio**: the `<video>` element, fed either by a plain file URL (direct play) or by hls.js (HLS, the segmented streaming format). hls.js is a private build, version 1.7.2 plus one small patch (`third_party/hls.js`).
2. **Music and audiobooks with gapless, crossfade or loudness normalisation on** (gapless is on by default): Portico's own audio engine, `DecodeAudioRender`. It downloads the original file with HTTP Range requests, splits it into packets with its own demuxers, decodes with the browser's WebCodecs `AudioDecoder` (or a JavaScript FLAC decoder), and schedules the decoded sound on a Web Audio `AudioContext`. The `<video>` element is idle in this mode.

### Who decides what

"Thin client" is taken seriously. The browser publishes a **capability profile** once per sign-in and again when the display or audio device changes (`web/src/bridge/client-profile.ts:70`). The profile is built by asking the browser, not by recognising the browser's name: `canPlayType`, `MediaSource.isTypeSupported` (or Safari's `ManagedMediaSource`), and `navigator.mediaCapabilities.decodingInfo` for 4K and HDR (`packages/client-core/src/client-profile.ts:337-417`). The only browser-name sniffing in the video path is one flag, "this engine tone-maps HDR well on a standard screen", true for Safari and for Chrome on macOS (`web/src/bridge/client-profile.ts:48`).

The server then chooses direct play, a remux to HLS, or a conversion, and the player plays the one URL it is given. The declared container list for direct play in a browser is MP4-family (`mp4`, `m4v`, `mov`) and WebM only (`client-profile.ts:396-397`). A Matroska (MKV) file is never direct-played in a browser; it is remuxed to HLS.

### Which adapter is live

There are two web adapters in the tree:

| File | Status |
|---|---|
| `web/src/bridge/useWebPlaybackAdapter.ts` (604 lines) driven by `PlaybackService` in `client-core/src/index.ts`, speaking Playback v1 through `playback-v1/legacy-bridge.ts` | **Wired.** This is what plays. |
| `web/src/playback-v1/web-media-player.ts` (265 lines) driven by `PlaybackSessionController` in `playback-v1/sessions.ts` with `TimelineReporter` | **Not wired.** Its own header says so (`web-media-player.ts:2-3`). It has a unit test; the wired adapter does not. |

Everything below describes the wired path unless it says otherwise.

### Starting, seeking and the timeline

- The server publishes a complete VOD playlist up front on a fixed six-second grid (or on the file's own keyframes for a remux), and produces segments on demand (`server/internal/playback/hls_timeline.go:14-17, 56-86`). A seek is just a request for a later segment; the server moves its converter.
- The player models every seek, including the starting position, as a "pending seek" that only clears when the element reports it has arrived within half a second of the target (`useWebPlaybackAdapter.ts:317-360`, `index.ts:1351-1366`). A seek that does not land in 15 seconds becomes a visible seek error with a Try again button (`index.ts:1334`).
- hls.js is always started at position 0; the resume position is applied afterwards as a pending seek (`useWebPlaybackAdapter.ts:205`).
- A start that does not become ready in 20 seconds fails (`index.ts:929`), except while the tab is hidden, where it waits for the tab to be shown (`index.ts:928-944`).

### Progress reports

The element's `timeupdate` (about four times a second) feeds `PlaybackService.fact` (`Player.tsx:201-204, 433`). The service checkpoints when the state changes or the position has moved three seconds (`index.ts:1438-1444`). The v1 bridge then sends a timeline report at once for a state change or a jump, and otherwise every ten seconds (`legacy-bridge.ts:138-141, 160-170, 226-229`). A report carries generation, state, position and a fixed `rate: 1` (`legacy-bridge.ts:184`). Stopping the player sends a final position with the session `DELETE`. Nothing is sent when the tab is closed or reloaded.

### Error handling

A home server "sleeps, reboots and sits behind bad Wi-Fi" (`useWebPlaybackAdapter.ts:500-503`), so the adapter retries quietly:

- Server says no (HTTP 401, 403, 404, 410 on a segment or playlist): stop with "This stream is no longer available" (`:263-267`).
- Other network failure: restart loading with back-off of 1, 2, 4, 8 then 10 seconds. The viewer sees "Reconnecting…" only once the buffer is empty. After 90 seconds of empty buffer, fail (`:504-537`).
- No progress for eight seconds while playing: treated the same way (`:448-450`).
- Media (decode) error in hls.js: recover twice, the second time swapping the audio codec hint (`:271-277`).
- The engine cannot play what it was given: tell the server which route failed (`POST /v1/playback/route-failures`). If the server has a more compatible route, ask for the title again from the same position, up to three times per title (`index.ts:1453-1468`). The server remembers a failed route per file and sign-in for seven days (`server/internal/playback/route_failures.go:20-23`).
- The server ended the session (owner's stop, transfer, lapsed lease): an error card with the reason; an owner's stop cannot be retried (`index.ts:718-723`, `player/owner-stop.ts`).

### Subtitles

The server turns every text subtitle into one JSON document of plain-text cues (start, end, text, whole-cue italic, top-of-screen) (`client-core/src/subtitles.ts:82-100`). The web app builds `VTTCue` objects from it on a text track it creates itself (`player/options.ts:113-145`). Styling tags from ASS/SSA subtitles are dropped. Image subtitles (PGS, VobSub) are burned into the picture by the server, which means a full video conversion; the player warns first on 4K or HDR titles (`Player.tsx:292-311`). The viewer can choose text size and background (`Player.tsx:142-144`, `Player.module.css:135`), shift timing in half-second steps (`Player.tsx:553`), and search an online provider from the menu.

While subtitles are on, the cue document is re-requested every 25 seconds "to stay authorised" (the server answers 304 when unchanged), and cues are removed if that fails for 75 seconds (`options.ts:164-196, 214`).

### Chrome and controls

Fixed layout: play/pause, skip back and forward (lengths from preferences), previous and next, a seek bar with buffered range, chapter ticks, intro/credits tint and trickplay hover thumbnails, volume, and option boxes for Audio, Subtitles, Quality, Speed, Sleep, Queue and Chapters. A More menu holds Playback info, Watch Together, Report a problem and the shortcut list. Skip intro/recap/credits prompts honour ask/auto/off. A post-play card counts down to the next queue entry. Keyboard: Space/K, ←/J, →/L, M, F, ?, Esc, plus ↑/↓ and G on live channels (`Player.tsx:212-235`). The chrome hides after 2.8 seconds of no pointer movement and the cursor hides with it.

Media Session (the browser's "now playing" card and hardware media keys) is wired for songs and audiobooks only (`bridge/useSystemListening.ts:12`).

### The audio engine in one paragraph

Each track is a "deck". A deck decodes at most 30 seconds ahead of the playhead into one-second `AudioBuffer`s scheduled back to back on the context's sample clock (`DecodeAudioRender.ts:19-23`, `bridge/audio/engine.ts`). The server supplies exact trim (how many leading and trailing frames to drop) and loudness gain, so the engine outputs exactly the track's frames. Near the end of a track the queue prepares the next one; the engine buffers ten seconds of it, commits about two seconds before the edge, and schedules it at the exact sample after the current track ends (gapless) or overlapped with equal-power curves (crossfade). Speed changes for audiobooks use the engine's own pitch-preserving time-stretch (`bridge/audio/stretch.ts`). Compressed bytes are cached in a 64 MB in-memory cache shared by all tracks (`bridge/audio/bytes.ts:40`). Which codecs go through WebCodecs is decided by a table of measured results per browser engine: Chromium has MP3, AAC, FLAC and Opus; Safari has MP3, AAC and Opus with FLAC in JavaScript; Firefox has nothing in the table (`bridge/audio/track.ts:28-33`).

## How the others do it

### jellyfin-web (the official Jellyfin web client)

- **Engine choice is per browser.** Native HLS on iOS always, and on desktop Safari for ordinary titles; hls.js everywhere else, including Chromium, where a comment notes Chromium 141 gained native HLS but it "does not support switching HDR/SDR playlists" (`jellyfin-web/src/components/htmlMediaHelper.js:41-75`). hls.js is the stock npm package.
- **The client builds a large device profile by browser name.** `browserDeviceProfile.js` is 1,654 lines of `canPlayType` probes overridden by named exceptions: Firefox's MKV support "is buggy" and is turned off; Safari only accepts the `hvc1` HEVC tag; "fMP4 in Firefox 149 is largely broken"; WebM "is unreliable on safari 17"; and so on (`jellyfin-web/src/scripts/browserDeviceProfile.js:208-232, 877-881, 752-754, 1464-1476`). MKV is direct-played wherever the browser's own `canPlayType` test for Matroska passes, and on Edge for Windows regardless.
- **Direct play is `<video src>` with `#t=seconds`** for the start position, plus a fallback that seeks when metadata arrives (`plugin.js:600-603`, `htmlMediaHelper.js:135-168`).
- **Seeking**: if the stream is HLS or seekable, set `currentTime`. Otherwise `changeStream`: ask the server for a new stream URL starting at that position and swap the source (`playbackmanager.js:1712-1783`). The same `changeStream` is used to change audio track, subtitle burn-in and bitrate.
- **Errors**: any HTTP error status from hls.js ends that attempt with a server error; fatal network errors call `startLoad()`; media errors go through recover, then swap-audio-codec-and-recover, then give up, with three-second spacing (`htmlMediaHelper.js:77-110, 262-340`). Above that, `onPlaybackError` retries the title with direct play disabled, then with stream copy disabled (`playbackmanager.js:3496-3532`). Nothing is remembered for next time.
- **Subtitles** have five render paths: the browser's own text tracks; a custom HTML overlay (for appearance control and for browsers whose native tracks are unreliable); a libass WebAssembly renderer in a worker for ASS/SSA with embedded and fallback fonts; and `libbitsub` for PGS and VobSub image subtitles drawn on a canvas (`plugin.js:1395-1547, 1552-1704`). A secondary subtitle track can be shown at the same time. Offset is adjustable live from a slider and from the G and H keys.
- **Audio track switching** uses `HTMLMediaElement.audioTracks` where the browser has it, otherwise a stream change (`playbackmanager.js:1341-1362`, `plugin.js:896-940`).
- **Chrome**: aspect ratio modes (auto, cover, fill), brightness, playback rates 0.5× to 4×, Picture in Picture including Safari's and Windows' non-standard forms, AirPlay button, stats overlay, trickplay bubble with chapter name, "Up next" overlay 30–40 seconds before the end of an episode, a keyboard map of about 25 keys including volume, frame step, speed, chapter jump, 0–9 percent seek and subtitle offset.
- **Progress**: a report every ten seconds and on pause, unpause and volume change (`playbackmanager.js:3348-3352, 3659-3677`), and a best-effort stop report on `beforeunload` (`:4428-4434`).
- **Audio**: one `<audio>` element per track, optional gain node for normalisation, a volume fade on stop. No gapless, no crossfade.
- **Bitrate**: an optional bitrate test before playback sets the maximum streaming bitrate automatically (`playbackmanager.js:2608-2632`).

### jellyfin-vue (Jellyfin's alternative web client)

One media element that is moved between a full-screen page and a mini player with Vue `Teleport`, so video keeps playing while browsing. Stock hls.js with its worker. ASS through JASSUB, PGS through `libpgs`, a custom external-subtitle overlay. Web Audio is attached to the element so a spectrum visualiser (`audiomotion-analyzer`) can run. Media Session is set for every media type.

### Audiobookshelf (web client)

A plain `<audio>` element for direct play and hls.js for transcodes. Progress is synced every ten seconds (first sync after twenty), with listening time counted separately from position, and a toast after four failed syncs (`audiobookshelf/client/players/PlayerHandler.js:267-330`).

### Plezy and jellyfin-desktop (the "leave the browser" alternative)

Both embed mpv. That buys full ASS rendering, audio passthrough to a receiver, HDR handled by the player, every container, and extras such as shader presets and 0.25×–8× speed (`plezy/README.md:117-133`). The cost is a desktop app to install and maintain. Portico's backlog already lists a desktop companion app.

### Plex Web (from public documentation, not checked this session)

Automatic quality that adapts to bandwidth unless the viewer picks "Play Original"; a setting for when to burn subtitles (only image formats, always, never); a documented keyboard shortcut list; no ASS rendering in the browser.

## Architectural differences

### A-17.1 A private build of hls.js, against the stock package
- **Portico:** hls.js 1.7.2 plus one 10-line patch to `src/utils/level-helper.ts`, built from source by a script that verifies hashes; the app depends on the local tarball (`third_party/hls.js/README.md`, `anchored-fragment-boundary.patch`, `web/package.json:21`). The patch keeps the first loaded fragment's duration anchored when it is measured to be continuous with its neighbour. The patch's own comment still calls it an "Isolated diagnostic candidate".
- **Jellyfin / others:** jellyfin-web, jellyfin-vue and Audiobookshelf all use the npm package unmodified.
- **Why it matters:** every hls.js upgrade means re-applying the patch, rebuilding and re-qualifying. The provenance record is unusually careful (source, patch and output hashes, reproducible rebuild), so the supply-chain side is well handled. The open risk is knowledge: the README says what the patch does but not which symptom it fixed, which server behaviour makes it necessary, or how to tell if a future hls.js no longer needs it.
- **Assessment:** **Trade-off (subjective).** Deliberate. The fork is as small as a fork can be. To keep it cheap: write down the failing case as a test fixture, and offer the patch upstream. Dropping the fork is small if the server can avoid the condition, unknown otherwise.

### A-17.2 hls.js on every browser that supports it, including Safari
- **Portico:** `Hls.isSupported()` decides; native HLS is used only when that is false (`useWebPlaybackAdapter.ts:181-189`). hls.js 1.7 counts Safari's `ManagedMediaSource` as support (`third_party` tarball `src/is-supported.ts:26-46`, `src/utils/mediasource-helper.ts:3-14`), so desktop Safari and iPhone Safari 17.1+ both run hls.js.
- **Jellyfin:** native HLS on iOS always ("hls.js is only in beta. needs more testing") and on Safari for ordinary titles; hls.js forced on Safari only for VP9 (`jellyfin-web/src/components/htmlMediaHelper.js:31-75`).
- **Why it matters:** one engine everywhere means one set of behaviours to reason about, and it is what makes Portico's seek and timeline model uniform. The costs on Apple browsers are real: hls.js turns off remote playback when it uses `ManagedMediaSource` (see B-17.6), the browser's own HLS stack is usually the better tested path for HEVC, HDR and Dolby audio on Apple hardware, and battery use is higher.
- **Assessment:** **Unclear.** Needs a device pass on Safari (macOS and iPhone) before deciding. Switching Safari to native HLS is medium: the adapter already has the native branch, but audio-rendition switching, the subtitle time mapping and live-channel clock all have hls.js-only code paths.

### A-17.3 Capability detection by asking the browser, not by naming it
- **Portico:** probes only (`client-core/src/client-profile.ts:337-417`); 4K is granted only if `decodingInfo` says supported and smooth; a probe that throws or hangs contributes nothing (1.5-second cap per question).
- **Jellyfin:** probes plus dozens of named overrides, each a past bug (`browserDeviceProfile.js`).
- **Why it matters:** Portico's profile will not rot as browsers change, and a new browser works without a release. But browsers do lie. Jellyfin's file is a catalogue of those lies: Safari passes the 10-bit H.264 test "but playback is failing" (`browserDeviceProfile.js:1162-1163`); Safari only plays HEVC tagged `hvc1`; Firefox 149 fragmented MP4 is broken; AV1 on phones is too slow in software. Portico has none of these and relies on the route-failure ladder (A-17.12) to recover after the fact, at the price of one failed start per file.
- **Assessment:** **Trade-off (subjective), leaning Portico ahead** for maintainability. A small, dated exception table for known liars (the handful above) would remove most first-play failures. Small.

### A-17.4 Matroska is never direct-played in a browser
- **Portico:** direct containers are MP4-family and WebM (`client-profile.ts:396-397`). MKV always becomes an HLS remux.
- **Jellyfin:** direct-plays MKV where `canPlayType('video/x-matroska')` passes, and on Edge on Windows; refuses on Firefox (`browserDeviceProfile.js:208-232`).
- **Why it matters:** most personal libraries are MKV. Portico's choice costs the server a remux process and temporary disk for nearly every browser play, but gives exact seeking, working audio-track choice and subtitles on one path. Jellyfin's choice costs nothing on the server but depends on Chromium's Matroska demuxer and loses audio-track switching in most browsers.
- **Assessment:** **Trade-off (subjective).** Portico's is the safer default. Worth an owner-visible note that browsers remux MKV by design, so the Dashboard's "converting" count is not read as a fault.

### A-17.5 HDR is sent untouched only to an HDR screen or a browser known to tone-map
- **Portico:** HDR10 is declared only if the browser says it decodes PQ and either the display reports high dynamic range or the engine is Safari or Chrome on macOS (`client-profile.ts:363-371`, `web/src/bridge/client-profile.ts:48`). Otherwise the server tone-maps (a full video conversion).
- **Jellyfin:** allows HDR on desktop Chrome on any OS, Edge 121+, Safari, and Firefox on macOS (`browserDeviceProfile.js:246-264`).
- **Why it matters:** Portico avoids the washed-out grey picture some Windows GPUs give. The price is a 4K tone-mapping conversion on the server for every Windows Chrome viewer with a standard screen, which is the heaviest job a weak box can be asked to do.
- **Assessment:** **Trade-off (subjective).** Deliberate (comment cites COMPAT-05). An owner switch ("let Chrome tone-map HDR") would let a weak server opt out.

### A-17.6 Seeking is always a local seek; the server follows segment requests
- **Portico:** the player sets `currentTime`; hls.js asks for the segment; the server restarts its converter there if needed. No "change stream" call on seek.
- **Jellyfin:** local seek for HLS; `changeStream` (new URL, new source) for non-seekable transcodes (`playbackmanager.js:1700-1716`).
- **Why it matters:** Portico's timeline never has an offset between element time and title time (Jellyfin carries `transcodingOffsetTicks` through subtitles, buffered ranges and progress). That removes a whole class of off-by-offset bugs.
- **Assessment:** **Portico ahead.** Note the spec still describes a server seek by `PATCH` for conversions (see Doc/code disagreements).

### A-17.7 Every seek is a tracked command that must be confirmed
- **Portico:** pending seek, 0.5-second landing tolerance, 15-second timeout, explicit failed-seek state with retry (`index.ts:1328-1379`, `useWebPlaybackAdapter.ts:317-360, 461-472`).
- **Jellyfin:** sets `currentTime` and moves on.
- **Why it matters:** Portico never reports a position the picture has not reached, and a seek into an unproduced range cannot silently write a wrong resume point. The cost is more state and one more way to fail: a slow but healthy seek (a weak server restarting a 4K conversion) becomes "That position is not available to seek yet" after 15 seconds, and the player is paused.
- **Assessment:** **Trade-off (subjective).** The correctness idea is good. The timeout should probably wait while segments are answering "still preparing" rather than fail.

### A-17.8 The start position is a seek after load, not hls.js's start position
- **Portico:** hls.js `startPosition` is 0 except after a network-route change (`useWebPlaybackAdapter.ts:205`).
- **Jellyfin:** `startPosition: playerStartPositionTicks / 1e7` (`plugin.js:568`). Portico's own unwired adapter does the same (`web-media-player.ts:135`).
- **Why it matters:** see B-17.5. Resuming a converted title asks for segment 0 first.
- **Assessment:** **Portico behind.** Small fix in the adapter.

### A-17.9 Buffer targets are fixed by time, not by bitrate
- **Portico:** 30 seconds ahead (20 on devices reporting 2 GB of memory or less), up to 60, 32 MB size hint, 30 seconds kept behind (`useWebPlaybackAdapter.ts:199-211`).
- **Jellyfin:** 30 seconds, dropped to 6 seconds when the bitrate cap is 25 Mbps or more, with a comment and an hls.js issue number explaining that browsers choke on huge high-bitrate buffers (`plugin.js:555-563`); back buffer unlimited.
- **Why it matters:** in hls.js the time target is a floor; the size hint only extends it. Thirty seconds of an 80 Mbps remux is about 300 MB ahead plus 300 MB behind, which is beyond what Chromium lets a page hold in one source buffer. hls.js then has to recover from a quota error. See B-17.14.
- **Assessment:** **Portico behind** on high-bitrate streams, **ahead** on back-buffer memory. Small: scale the forward target by the session's known bitrate.

### A-17.10 Fragment load timeout of 15 seconds, and an 8-second stall watchdog
- **Portico:** one load policy for manifests, playlists and fragments: 8 seconds to first byte, 15 seconds total, two retries (`useWebPlaybackAdapter.ts:191-198`). Separately, eight seconds without the picture advancing triggers a reload (`:448-450`).
- **Jellyfin:** hls.js defaults (fragments: 10 seconds to first byte, 120 seconds total) and a 20-second manifest timeout; no application watchdog.
- **Why it matters:** see B-17.4. On a slow link or a slow server the Portico settings turn "buffering slowly" into "failed after 90 seconds".
- **Assessment:** **Portico behind** for slow links. Small.

### A-17.11 Quiet reconnection instead of immediate failure
- **Portico:** network trouble is retried for as long as there is buffer, the viewer is told only when the buffer runs dry, and playback ends only after 90 seconds with nothing to play (`useWebPlaybackAdapter.ts:500-537`). A network-route change (the app moving between a server's addresses) tears the engine down and resumes at the same position (`:492-499`).
- **Jellyfin:** any HTTP error status is fatal for that attempt; other fatal network errors get one `startLoad()` each time.
- **Why it matters:** a server restart or a Wi-Fi blip mid-film is survivable in Portico without touching anything.
- **Assessment:** **Portico ahead** in intent. The mechanism needs the fixes in B-17.4.

### A-17.12 Engine failures are reported to the server, which remembers them
- **Portico:** `POST /v1/playback/route-failures`; the next plan for that file and sign-in skips the failed route for seven days (`index.ts:1453-1468`, `route_failures.go:14-23`).
- **Jellyfin:** client-side retry with direct play then stream copy disabled; forgotten at once.
- **Why it matters:** the second play of a troublesome file starts cleanly in Portico. Two limits: the memory is per file, so every episode of a season encoded the same way fails once; and a single transient decode hiccup closes direct play for that file for a week.
- **Assessment:** **Portico ahead.** Consider generalising a failure to the codec/profile/container combination for that device after it repeats on two or three files.

### A-17.13 Subtitles are one server-normalised cue document
- **Portico:** JSON cues, plain text, whole-cue italic and top placement only; the server strips ASS override tags (`subtitles.ts:82-100`).
- **Jellyfin:** also fetches JSON track events for its manual text track, but renders ASS with libass and image subtitles with libbitsub.
- **Why it matters:** Portico's renderer is tiny and identical in every browser. Anime and other styled subtitles lose positioning, fonts, colours, karaoke and on-screen sign translation; overlapping signs collapse into stacked centred lines.
- **Assessment:** **Portico behind** for styled subtitles; **deliberate** (spec §5.5 names plain WebVTT as the default for ASS). See F-17.1.

### A-17.14 Image subtitles force a video conversion
- **Portico:** PGS/VobSub are burned in (`client-profile.ts:90` declares no bitmap support for web); the text track of the same language is preferred automatically when there is one (`Player.tsx:296-303`).
- **Jellyfin:** canvas rendering in the browser (opt-in setting), else burn-in.
- **Why it matters:** Blu-ray rips commonly have only PGS. Turning subtitles on converts the whole film, at whatever speed the server can manage, and loses direct play, HDR passthrough and often 4K.
- **Assessment:** **Portico behind.** See F-17.2. The spec already allows client rendering of image subtitles (§5.5, D-FEAT-3).

### A-17.15 Subtitle access is re-authorised every 25 seconds
- **Portico:** the cue document is re-fetched every 25 seconds with a validator; cues are removed after 75 seconds without a successful check. The subtitle catalogue and plan are also re-read every 25 seconds while subtitles are on or the menu is open (`options.ts:164-214`, `subtitles.ts:147`).
- **Jellyfin:** fetches the subtitle once.
- **Why it matters:** three small requests every 25 seconds per viewer with subtitles on. At a couple of hundred browsers that is steady background traffic for something that almost never changes. It also means subtitles disappear during a network outage that the video buffer rides through.
- **Assessment:** **Portico behind** on cost; the revocation intent is understandable but media segments on the same grant are already cut off by the server when access ends. The device event stream could carry "subtitles changed" instead. Medium.

### A-17.16 Subtitle timing offset is stored on the server
- **Portico:** each half-second nudge is a `PUT` that updates the selection, then the document is reloaded (`Player.tsx:553`).
- **Jellyfin:** offset is local to the current playback, adjusted live from a slider or keys, and forgotten.
- **Why it matters:** Portico remembers the fix for a badly timed file, which is better. The step is coarse and each step is a round trip.
- **Assessment:** **Portico ahead** on persistence, **behind** on fineness and feel. Small: apply locally at once, save after the viewer stops adjusting, allow 0.1-second steps.

### A-17.17 Audio track choice means asking for the title again
- **Portico:** built to switch inside the HLS manifest (`audio-selection.ts`, `player-audio-mapping.ts`, `useWebPlaybackAdapter.ts:47-139`), but no client ever installs an audio plan, so that path never runs. What runs is "play the title again at this position with that track named" (`index.ts:858-864`). See B-17.1 for why even that does not work today.
- **Jellyfin:** `audioTracks` on the element where available, else `changeStream`.
- **Why it matters:** re-opening costs a rebuffer of a few seconds. The in-manifest switch would be seamless.
- **Assessment:** **Portico behind** today; **ahead** on paper once the plan is wired.

### A-17.18 Quality is manual only, and "Automatic" means "whatever the server plans"
- **Portico:** no adaptive bitrate. The Quality box lists Automatic, prepared versions and conversion rungs the server offers (`Player.tsx:558-568`). A client-side automatic chooser exists in client core (`playback-v1/quality.ts`) with no caller in the web app.
- **Jellyfin:** optional bitrate test before each play picks a cap; manual bitrate menu; a change is a `changeStream`.
- **Plex (public docs):** adapts automatically unless told to play original.
- **Why it matters:** this is a recorded Portico principle. The consequence for a remote viewer on a weak link is buffering until they find the Quality box.
- **Assessment:** **Trade-off (subjective), deliberate.** The spec's own answer (a client-side "Auto" that picks a limit from the client's bandwidth estimate, §5.2) is half built. See F-17.9.

### A-17.19 Progress reporting has no unload path
- **Portico:** nothing on `pagehide`, `beforeunload` or `visibilitychange`.
- **Jellyfin:** stop report on `beforeunload`.
- **Why it matters:** see B-17.7.
- **Assessment:** **Portico behind.** Small.

### A-17.20 Sessions have a lease; a lapsed lease is an error card
- **Portico:** the server ends a session after 120 seconds without a report or media request. The wired web path shows "The stream disconnected. Retry to reconnect." (`index.ts:718-723`, `playback-messages.ts`).
- **Jellyfin:** no lease. After a laptop wakes, the element usually just carries on, or the transcode is restarted by the next segment request.
- **Why it matters:** closing a laptop lid for five minutes and reopening it ends in an error card in Portico. One click recovers, at the right position.
- **Assessment:** **Portico behind** in feel. The spec says the client should start a new session by itself (§17 clarification 16) and the unwired controller does (`sessions.ts:238-243`). Small once ported.

### A-17.21 The player owns intent; the element only reports facts
- **Portico:** "UI intent is the sole command authority. Native events report facts, never autoplay intent" (`index.ts:645`). If the element pauses but intent says playing, the adapter plays it again.
- **Jellyfin:** the element is the truth; a pause from anywhere is a pause.
- **Why it matters:** Portico's model makes remote control, group sync and recovery predictable. It also means a pause that did not come through Portico's own buttons is undone. See B-17.3.
- **Assessment:** **Trade-off (subjective).** Keep the model; teach it that an element pause that Portico did not cause is the viewer's or the system's intent.

### A-17.22 Media Session only for audio
- **Portico:** metadata and handlers for songs and audiobooks (`useSystemListening.ts:12`).
- **Jellyfin, jellyfin-vue:** every media type.
- **Why it matters:** for video, the browser's media hub and the OS show the page title, there are no seek or next buttons, and play/pause falls to the browser's default action on the element (B-17.3).
- **Assessment:** **Portico behind.** Small.

### A-17.23 Music decoded by Portico's own engine, not the media element
- **Portico:** Range fetch, own demuxers, WebCodecs or JavaScript decode, sample-accurate scheduling, server-supplied trim and gain, own resampler and time-stretch.
- **Jellyfin:** `<audio src>`; optional gain node. No gapless or crossfade in the browser.
- **Finamp / Plexamp:** native engines; Plexamp is the stated target.
- **Why it matters:** this is the only browser player among those compared that can be sample-exact across track boundaries. It is also about 1,800 lines of container and codec code that Portico now owns, tied to a per-browser table measured on Chrome 152 and Safari 27.
- **Assessment:** **Portico ahead**, with ownership cost. The conformance fixtures (spec §18.8, `web/audio-conformance.html`) are the right safety net; they need to be re-run on each browser release and on Firefox.

### A-17.24 What happens when WebCodecs is missing
- **Portico:** the browser then declares only PCM and FLAC (JavaScript decoder) as decodable (`capabilities.ts:31-39`), and the server converts everything else to FLAC (or Opus under a bitrate limit). Firefox is in this state because its column in the measured table is empty (`track.ts:31`), whether or not Firefox has an `AudioDecoder`.
- **Jellyfin:** not applicable; the element plays what the browser plays.
- **Why it matters:** a Firefox listener's MP3s are converted to FLAC on the server (several times the bytes) and decoded in JavaScript on the page's main thread. It works and stays gapless; it is the most expensive path. Remote listeners pay in bandwidth.
- **Assessment:** **Trade-off.** Run the fixtures on Firefox and fill in its row. Small.

### A-17.25 All audio decode, resample and stretch work runs on the main thread
- **Portico:** no worker and no audio worklet; decoded audio is scheduled up to 30 seconds ahead so a busy page does not cause a gap (`DecodeAudioRender.ts:19`, `engine.ts:89-109`).
- **Others:** hls.js uses a worker for transmuxing; jellyfin-vue uses workers for subtitles.
- **Why it matters:** scheduling ahead is a sound way to avoid underruns without a worklet. The cost is UI jank while a deck fills (JavaScript FLAC decode of 24-bit/96 kHz, a 64-tap resampler per sample, time-stretch correlation), most visible right after a seek.
- **Assessment:** **Trade-off (subjective).** Moving `Deck.pump` into a worker is medium and can wait for a measured problem.

### A-17.26 Position for music comes from the audio clock, without output latency
- **Portico:** `context.currentTime` (`DecodeAudioRender.ts:222-228`, `engine.ts:83-86`).
- **Others:** the element's `currentTime`, which browsers already compensate.
- **Why it matters:** on Bluetooth headphones the reported position (and synced lyrics) lead the sound by the output latency, typically 150–300 ms.
- **Assessment:** **Portico slightly behind.** Small: subtract `outputLatency` where the browser reports it.

### A-17.27 Video stops when you leave the player
- **Portico:** "Video is full-screen only; leaving it stops playback" (`Player.tsx:50-53`). Audio collapses to a mini bar.
- **jellyfin-vue:** mini video player while browsing. **jellyfin-web:** leaving stops, like Portico.
- **Why it matters:** no browse-while-watching in the tab. Picture in Picture does not help because the player surface still covers the app.
- **Assessment:** **Trade-off (subjective), deliberate.** See F-17.12.

### A-17.28 Up next appears only after the title has fully ended
- **Portico:** the post-play card renders at `phase === 'ended'` (`PostPlay.tsx:43`), with a server-set countdown.
- **Jellyfin:** an overlay 30–40 seconds before the end for episodes longer than ten minutes (`video/index.js:660-677`). Plex and Netflix behave similarly.
- **Why it matters:** bingeing in Portico means sitting through the credits or pressing Skip credits, then waiting for the countdown.
- **Assessment:** **Portico behind.** Portico has credits markers, which are a better trigger than Jellyfin's fixed seconds. See F-17.5.

### A-17.29 A click anywhere on the picture toggles play, for mouse and touch alike
- **Portico:** the tap layer calls `toggle()` on click and fullscreen on double-click (`Player.tsx:442`).
- **Jellyfin:** mouse click is delayed 300 ms so a double-click is not also two pauses; a touch only shows or hides the controls (`video/index.js:1795-1837`).
- **Why it matters:** B-17.11 and B-17.12.
- **Assessment:** **Portico behind.** Small.

### A-17.30 Keyboard map is short and matched on the typed character
- **Portico:** eight keys for video, matched on `e.key` in lower case, with no check for Ctrl/Cmd/Alt (`Player.tsx:214-231`).
- **Jellyfin:** about 25, matched on physical key codes, modified keys skipped.
- **Why it matters:** B-17.9 (shortcuts hijack browser shortcuts) and F-17.6 (missing keys).
- **Assessment:** **Portico behind.** Small.

### A-17.31 Volume is linear and saved per browser
- **Portico:** slider value goes straight to `element.volume`; saved in `localStorage` under one key for all profiles (`engine.tsx:105, 181-200`).
- **Jellyfin:** cubic curve so the slider feels even (`plugin.js:2223-2235`); also saved locally.
- **Why it matters:** with a linear slider most of the audible change is in the bottom fifth.
- **Assessment:** **Portico slightly behind.** Trivial.

### A-17.32 Skip prompts are server-authorised and reported
- **Portico:** markers come with the session's offers; automatic skip only when the server marks a marker safe; each skip is reported for analytics (`SkipPrompt.tsx`, `index.ts:1155-1159`).
- **Jellyfin:** media segments API with client-side per-type actions.
- **Assessment:** **Portico ahead** in design.

### A-17.33 A pre-play read before every video start
- **Portico:** before starting a film or episode the web app reads `/playback-options` to see whether a Dolby Vision colour warning is needed (`engine.tsx:228-245`), then starts the session.
- **Jellyfin:** `PlaybackInfo` is the start.
- **Why it matters:** one extra sequential round trip before every video play, on every title, for a warning that applies to a few files.
- **Assessment:** **Portico slightly behind.** Small: let the session start carry the warning, or read options and start in parallel.

### A-17.34 Changing quality, version or audio restarts as a new one-item queue
- **Portico:** these all call `engine.play(itemId, position, …)`, which creates a fresh queue holding only that title (`Player.tsx:347-364`, `engine.tsx:220-226`, `v1-queue-player.ts:275-277, 322-332`).
- **Jellyfin:** `changeStream` swaps the source and keeps the play queue.
- **Why it matters:** B-17.2.
- **Assessment:** **Portico behind.** The protocol has `PATCH` for exactly this.

### A-17.35 Two web adapters, and tests on the one that is not shipped
- **Portico:** `useWebPlaybackAdapter.ts` (wired, no behavioural test) and `web-media-player.ts` (unwired, tested by `web/test/web-media-player.test.ts`). They differ in start position, error ladder, subtitle delivery (`<track>` elements against a scripted text track), and reporting richness. Some web tests assert on source text with regular expressions rather than behaviour (`web/test/playback-traffic.test.ts`).
- **Jellyfin:** one plugin.
- **Why it matters:** the migration to the new controller is half done. Fixes land in one adapter and not the other; the confident tests cover code nobody runs.
- **Assessment:** **Portico behind** until the migration finishes. Finishing it resolves B-17.1, B-17.2, B-17.5, B-17.8 and several doc disagreements at once. Large, but it is already planned work.

### A-17.36 Timeline reports are sparse
- **Portico (wired):** generation, state, position, `rate: 1` (`legacy-bridge.ts:184`).
- **Spec and unwired reporter:** also buffered time, bandwidth, dropped frames, volume, mute, audio and subtitle track (`playback-v1/timeline.ts:189-207`).
- **Jellyfin:** play state, position, volume, mute, tracks, play method, repeat mode, queue.
- **Why it matters:** the owner's Now Playing cannot show a web viewer's bandwidth or buffer health, and a group cannot see a member's real speed.
- **Assessment:** **Portico behind** the spec. Resolved by A-17.35.

### A-17.37 Trickplay tiles are fetched through the authorised artwork cache
- **Portico:** tile sheets load as blobs through the same cache as posters (`Timeline.tsx:94-108`), with the chapter title above the thumbnail.
- **Jellyfin:** plain image URLs with the API key in the query string.
- **Assessment:** **Portico ahead** (no credential in URLs), at the cost of a fetch-to-blob per sheet.

### A-17.38 Media URLs never carry cookies and never leave their origin
- **Portico:** hls.js requests are forced to `withCredentials = false` and any request to another origin throws (`useWebPlaybackAdapter.ts:216-220`); audio fetches use `credentials: 'omit'`, `cache: 'no-store'`, `redirect: 'error'` (`bytes.ts:87`).
- **Jellyfin:** `withCredentials` follows a web setting; tokens ride in query strings.
- **Assessment:** **Portico ahead.**

### A-17.39 Playback speed range
- **Portico:** 0.5× to 2× for video and audiobooks; none for music (`Player.tsx:75`, `listening.ts:11`). Spec §12 says audiobooks go to 3.0×.
- **Jellyfin:** 0.5× to 4×. **Plezy:** 0.25× to 8×.
- **Assessment:** **Portico behind** its own spec for audiobooks. Trivial.

### A-17.40 Fullscreen targets the player container only
- **Portico:** `stage.requestFullscreen()` with no fallback (`Player.tsx:236-243`).
- **Jellyfin:** Screenfull, then `webkitEnterFullscreen()` on the video for iPhone (`playbackmanager.js:1456-1472`).
- **Why it matters:** B-17.13.
- **Assessment:** **Portico behind** on iPhone.

### A-17.41 Aspect, zoom and brightness
- **Portico:** `object-fit: contain`, fixed (`Player.module.css:4`).
- **Jellyfin:** auto / cover / fill, and a brightness control.
- **Assessment:** **Portico behind** by a small feature. See F-17.7.

### A-17.42 Subtitle appearance options
- **Portico:** size (four steps) and background (three) (`Player.tsx:142-144`), stored as server preferences so they follow the profile.
- **Jellyfin:** size, weight, font, text colour, background colour, drop shadow, vertical position, and a styling mode; stored per browser.
- **Assessment:** **Portico behind** on range, **ahead** on following the profile.

### A-17.43 Casting from the browser
- **Portico:** one Portico Cast receiver; the sender pairs with a short-lived code and hands over the item and position; the local player stops once the TV has started (`web/src/bridge/cast.ts`, `Player.tsx:487`). Cast is offered only when the server is reached over HTTPS, because a Cast device will not load plain HTTP (`cast.ts:51-54`). AirPlay is the Safari system picker.
- **Jellyfin:** Chromecast plugin that sends the server address and an access token to its receiver.
- **Assessment:** **Portico ahead** on credential handling (covered in domain 13). The HTTPS requirement means a plain-HTTP home server has no Cast button at all, with no explanation in the player.

## Missing features and capabilities to consider

Sizes are rough: small (days), medium (a week or two), large (more).

### F-17.1 Styled ASS/SSA subtitles drawn in the browser
- **What:** render ASS with libass compiled to WebAssembly, in a worker, on a canvas over the video, with the file's embedded fonts and a fallback font set.
- **Who:** jellyfin-web (`@jellyfin/libass-wasm`, `plugin.js:1395-1469`, with memory and glyph limits, a render-ahead cache and a legacy worker), jellyfin-vue (JASSUB). Plezy and jellyfin-desktop get it free from mpv.
- **Value:** anime. Without it, typesetting, sign translations, karaoke and positioned dialogue are lost, and the only way to see them is burn-in (a full conversion).
- **Size:** medium on the client; the server must also serve the original ASS and font attachments instead of only the flattened cue document.
- **Conflict:** none with principles; it adds a WebAssembly dependency and a font-serving route.
- **Opinion:** the most valuable player feature Portico lacks, given anime is a first-class library kind in the spec.

### F-17.2 Image subtitles (PGS, VobSub) drawn in the browser
- **What:** decode the subtitle bitmap stream in a worker and paint it on a canvas.
- **Who:** jellyfin-web via `libbitsub` (range requests, prefetch window, debanding for VobSub, `plugin.js:442-463, 1474-1547`); jellyfin-vue via `libpgs`.
- **Value:** Blu-ray rips keep direct play, 4K and HDR with subtitles on. Removes Portico's biggest hidden conversion trigger.
- **Size:** medium. Spec §5.5 already has the capability flag (`render: client` for PGS/VobSub).
- **Opinion:** high value for weak servers. Second only to F-17.1.

### F-17.3 A second subtitle track at the same time
- **Who:** jellyfin-web ("SecondarySubtitles", `plugin.js:672-674, 1573-1587`).
- **Value:** language learners. **Size:** small once cues are drawn by Portico (they already are). **Opinion:** nice to have.

### F-17.4 Fuller subtitle appearance and finer timing control
- **What:** text colour, font, weight, edge style (outline or drop shadow), vertical position; offset in 0.1-second steps with a slider and keys.
- **Who:** jellyfin-web settings (`selectFont`, `selectTextWeight`, `selectDropShadow`, `sliderVerticalPosition`, `inputTextColor`), G/H keys for offset.
- **Value:** readability and accessibility (edge style and colour are standard caption preferences on every TV platform). **Size:** small. **Opinion:** worth doing with the accessibility contract in mind.

### F-17.5 Up next during the credits
- **What:** show the next-episode card when the credits marker starts (or N seconds before the end when there is no marker), with the picture shrinking or continuing behind it.
- **Who:** jellyfin-web (fixed 30–40 seconds before the end), Plex, Netflix.
- **Value:** the core binge interaction. **Size:** small to medium; Portico already has credits markers, the queue's next entry and a server post-play policy. **Opinion:** do it.

### F-17.6 A fuller keyboard and pointer map
- **What Jellyfin has that Portico lacks:** ↑/↓ volume, 0–9 jump to tenths, Home/End, `,` and `.` single-frame step, `<` and `>` speed down and up, Shift+N / Shift+P next and previous, PageUp/PageDown next and previous chapter, G/H subtitle offset (`video/index.js` `onKeyDown`). Common elsewhere: C to toggle captions, I for Picture in Picture, mouse wheel for volume.
- **Size:** small. **Opinion:** cheap and expected by keyboard users. Fix B-17.9 and B-17.10 at the same time.

### F-17.7 Fit modes and brightness
- **What:** fill or crop to remove letterbox bars on ultrawide or 4:3 screens; a brightness control.
- **Who:** jellyfin-web (`plugin.js:2095-2111, 2260-2295`), Plezy.
- **Size:** small (CSS `object-fit` and `filter`). **Opinion:** fit modes yes; brightness is marginal in a browser.

### F-17.8 Media Session for video
- **What:** title, show and artwork in the OS and browser media controls; play, pause, seek and next/previous handlers; position state.
- **Who:** jellyfin-web, jellyfin-vue.
- **Size:** small (the audio path already does it). **Opinion:** needed anyway to fix B-17.3 properly.

### F-17.9 Client-side "Automatic" quality
- **What:** pick a bitrate limit from a measured throughput before starting, and offer to step down when the buffer keeps emptying (ask, never switch silently, to respect the principle).
- **Who:** jellyfin-web bitrate test (`playbackmanager.js:2608-2632`); Plex adapts continuously.
- **State in Portico:** `playback-v1/quality.ts` implements the decision; the spec (§5.2) describes it; no web code calls it.
- **Conflict:** the principle is "quality changes only when a person asks". A measured starting choice under an explicit "Automatic" setting, and a prompt rather than a silent switch mid-play, both fit it.
- **Size:** medium. **Opinion:** worth finishing for remote viewers.

### F-17.10 Richer playback info
- **What Jellyfin shows that Portico does not:** conversion progress and speed, target codecs, the player's and the video's pixel sizes, corrupted frames, the stream protocol, audio sink, and SyncPlay timing stats. Portico shows delivery and reasons, codecs, size, bitrate, HDR, dropped/total frames and buffer seconds.
- **Also useful:** hls.js bandwidth estimate, the audio engine's path (WebCodecs, JavaScript FLAC, converted), and a "copy details" button for bug reports.
- **Size:** small. **Opinion:** the "copy details" button pays for itself the first time someone reports a problem.

### F-17.11 Browse while watching
- **What:** a mini video player, or Document Picture in Picture (a small always-on-top window that can hold Portico's own controls and subtitles).
- **Who:** jellyfin-vue's mini player.
- **Conflict:** `06 Clients.md` says the player layout is fixed and Player.tsx says video is full-screen only. This would be a deliberate change.
- **Size:** medium. **Opinion:** Justin's call; the element is already mounted once in the shell, which is the hard part.

### F-17.12 Touch behaviour for phones and tablets
- **What:** tap shows controls; double-tap left or right seeks; landscape orientation lock in fullscreen.
- **Who:** jellyfin-web (tap toggles the controls, orientation helper `playbackorientation.js`); double-tap seek is standard in YouTube and Plex.
- **Size:** small. **Opinion:** needed for the mobile web to feel right (see B-17.11).

### F-17.13 Direct play of MKV in Chromium as an owner option
- **What:** let the owner allow `<video src>` for Matroska on Chrome and Edge where `canPlayType` passes.
- **Who:** jellyfin-web default.
- **Value:** removes the remux process for most LAN browser plays on a weak box. **Cost:** loses in-player audio switching for that play and depends on Chromium's demuxer.
- **Size:** small on the client (one container in the profile). **Opinion:** worth an experiment behind a switch.

### F-17.14 Audio extras
- **What:** an equaliser, a visualiser, an output-device picker, and volume above 100 %.
- **Who:** jellyfin-vue (spectrum visualiser); Plexamp (EQ, visualisers, device picker).
- **State in Portico:** the audio engine already ends in a Web Audio gain node, so EQ (biquad filters) and an analyser are small additions.
- **Opinion:** low priority, but nearly free given the engine.

### F-17.15 Wider speed range and finer steps
- **What:** up to 3× for audiobooks (the spec's own figure), 0.05–0.1 steps.
- **Who:** jellyfin-web to 4×, Plezy to 8×. **Size:** trivial.

### F-17.16 mpv-class extras (for the planned desktop app, not the browser)
Frame-accurate stepping, screenshots, A–B repeat, audio passthrough, shader presets. Plezy and jellyfin-desktop have them because mpv does. Listed for completeness; a browser cannot do passthrough at all.

### Considered and not needed
- **Screen wake lock:** browsers already keep the screen awake while a video with sound plays. Jellyfin-web does not use the Wake Lock API either.
- **Multi-tab conflict handling:** by contract a tab is its own device and playback is independent per device (P01), so two tabs playing is allowed. The real multi-tab issue is slots held by closed tabs (B-17.7).

## Possible bugs and risks in Portico

None of these was reproduced. "Confirmed by reading" means the code path is unambiguous in the source; the visible effect still needs a browser to see.

### B-17.1 Quality, prepared-version and audio-track choices never reach the server
- **What goes wrong:** choosing a quality rung, a prepared version or a different audio track in the player (or on the title page before playing) restarts playback at the same position with the same stream. The menu then shows the new choice as selected.
- **Evidence:** every such choice ends in `PlaybackService.play(itemId, position, prepared, quality, audioStream)` (`client-core/src/index.ts:1092-1095`), which calls `queue.playItem(...)`. The only queue implementation, `V1QueuePlayer.playItem`, names those three parameters with a leading underscore and ignores them: `return this.playIds([itemId], {startSeconds})` (`playback-v1/v1-queue-player.ts:275-277`). The session start it then sends carries only state and position (`:358-361`). The server accepts `quality`, `audio` and `versionId` on start (`server/internal/playbackv1/sessions.go:78-97`), so the gap is in the client. For audio, the label is wrong afterwards because `installAudioTracks` marks the *requested* stream as selected (`index.ts:853-855`). For quality, the box shows the chosen rung from local state (`Player.tsx:339, 355`).
- **Related:** the seamless in-manifest audio switch is unreachable because nothing calls `installAudioPlan` (`index.ts:839`; no caller in web or native), so `applyAudio` always returns at its first line (`useWebPlaybackAdapter.ts:50`). The profile also tells the server the browser can switch embedded audio tracks (`web/src/bridge/client-profile.ts:47`), but no web code ever enables an element audio track.
- **How the comparison surfaced it:** Jellyfin's `setAudioStreamIndex` and bitrate change both go through `changeStream` with the new parameters; looking for Portico's equivalent led to the dropped arguments.
- **Confidence:** Confirmed by reading. **Severity:** High. **Backlog:** not listed. The React Native app calls the same core path (`apps/native/src/player/usePlayer.ts:258-263`), so it is likely affected too.

### B-17.2 Changing quality, version or audio, pressing Replay, or an automatic fallback throws away the queue
- **What goes wrong:** while watching episode 3 from a season queue, any of those actions creates a new queue containing only episode 3. Up next no longer has a next episode.
- **Evidence:** `engine.play` → `startSingle` → `playItem` → `playIds([itemId])` → `client.create(...)` (`web/src/player/engine.tsx:220-226`, `v1-queue-player.ts:318-332`). Replay goes `service.resume()` → `this.play(itemId, …)` when the phase is `ended` (`index.ts:1311-1314`, `PostPlay.tsx:44-48`). The route-failure fallback calls `this.play(...)` too (`index.ts:1466`). `startSingle` also resets the on-screen identity to "Now playing" with no kind or library until the item is read again (`engine.tsx:221`).
- **Confidence:** Confirmed by reading. **Severity:** High for series viewing. **Backlog:** not listed.

### B-17.3 A pause that does not come from Portico's own buttons is undone at once (video)
- **What goes wrong:** pressing the keyboard's media play/pause key, a headset button, the pause button in the browser's media hub, or the pause button in the Picture in Picture window pauses the `<video>` element; Portico immediately plays it again. The same happens when the operating system pauses media (headphones unplugged in Safari, an interruption).
- **Evidence:** the element's `pause` event calls `fact()` (`Player.tsx:438`), which updates the snapshot and re-runs the adapter's `apply`. Intent is still "playing", so `apply` reaches `else if (el.paused) { void el.play() … }` (`useWebPlaybackAdapter.ts:428-438`). For songs and audiobooks, Media Session handlers route these controls to `service.pause()` (`useSystemListening.ts:17`), so audio is fine; the hook returns early for every other kind (`:12`), leaving video with the browser's default action on the element.
- **How the comparison surfaced it:** Jellyfin treats the element as the truth and registers Media Session for all media.
- **Confidence:** Confirmed by reading the logic; the per-browser effect is Probable. **Severity:** High (media keys and the PiP pause button are everyday controls). **Backlog:** not listed.

### B-17.4 Slow delivery is turned into failure: the stall watchdog and a 15-second fragment limit abort downloads that would have finished
- **What goes wrong:** when a segment takes longer than about 8–10 seconds to arrive and the buffer is empty (a high-bitrate remux over a modest link, a weak server converting slower than real time, a remote viewer), playback does not buffer slowly. It restarts the download repeatedly and ends with an error after 90 seconds.
- **Evidence, three pieces that combine:**
  1. Fragment loads are capped at 15 seconds in total (`useWebPlaybackAdapter.ts:191-198, 215`); hls.js's own default is 120 seconds (`third_party` tarball `src/config.ts:598-599`). hls.js loads a fragment whole, so a fragment that needs 16 seconds can never arrive. Copied streams may have segments up to 30 seconds long (`server/internal/playback/hls_copy_timeline.go:70-73`).
  2. Eight seconds with no movement while not paused calls `recover(() => hls.startLoad(position))` (`:450`).
  3. `recoveryTick` calls the retry on every tick (1, 2, 4, 8, then every 10 seconds) until the buffer holds more than two seconds (`:516-531`). In hls.js, `startLoad` begins with `stopLoad()`, which aborts the fragment in flight (`src/controller/stream-controller.ts:141-147`, `base-stream-controller.ts:176-187`). So each tick throws away the partial download.
- **Second effect:** recovery only ends when more than two seconds are buffered ahead (`:457, 527`). Near the end of a title, or on a link that keeps the buffer under two seconds, it never ends, and `startLoad` keeps firing every ten seconds during otherwise working playback.
- **How the comparison surfaced it:** Jellyfin leaves hls.js's load policy alone and has no watchdog; its one buffer scar goes the other way (shrink the buffer for high bitrates).
- **Confidence:** Confirmed by reading (both sides of the interaction are in source). Size of the effect in practice: Probable. **Severity:** High for remote and weak-server use, which Portico's principles explicitly include. **Backlog:** not listed.

### B-17.5 Resuming an HLS title loads the start of the title first
- **What goes wrong:** hls.js is created with `startPosition: 0` unless the adapter is resuming after a route change (`useWebPlaybackAdapter.ts:205`). The resume position is applied later as a seek once the element has metadata (`:328-360`). So the first request is for segment 0, which makes the server start its converter at the beginning, and the seek then makes it restart at the resume point.
- **Evidence:** as cited; compare `web-media-player.ts:135` (the unwired adapter passes the real start) and `jellyfin-web/src/plugins/htmlVideoPlayer/plugin.js:568`.
- **Effect:** slower starts for every resumed remux or conversion and a wasted converter start on the server. For MPEG-TS segments the whole first segment must arrive before the seek can happen.
- **Confidence:** Confirmed by reading on the client; the server-side double start is Probable. **Severity:** Medium. **Backlog:** not listed.

### B-17.6 AirPlay is offered in Safari but cannot work for HLS sessions
- **What goes wrong:** the Play on menu shows AirPlay whenever Safari exposes the picker (`player/PlayOn.tsx:15`). For HLS sessions Portico uses hls.js, which on Safari 17+ uses `ManagedMediaSource` and sets `media.disableRemotePlayback = true` (`third_party` tarball `src/controller/buffer-controller.ts:325-331`, `src/utils/mediasource-helper.ts:3-14`). A media-source-backed element has nothing an Apple TV can fetch.
- **Works:** direct-play sessions (a plain URL with no cookie needed).
- **How the comparison surfaced it:** Jellyfin keeps Safari on native HLS, partly for this.
- **Confidence:** Probable (needs a Safari check). **Severity:** Medium. **Backlog:** not listed.

### B-17.7 Closing or reloading the tab leaves the session running on the server
- **What goes wrong:** there is no `pagehide`/`beforeunload` handling for playback anywhere in `web/src` (the only `beforeunload` is a settings-form warning). The web `PlaybackService` is built without a journal (`engine.tsx:119`), so nothing is cleaned up on the next load either. The session stays "playing" until its 120-second lease lapses; a converter keeps running until the two-minute abandonment check (`hls_bad_network.go:36`); the stream slot stays counted, so an account capped at one stream can be refused when the viewer reloads and presses play again; and up to about ten seconds of progress is lost because steady progress is only sent every ten seconds (`legacy-bridge.ts:160-170`).
- **How the comparison surfaced it:** jellyfin-web's `beforeunload` → `onAppClose` (`playbackmanager.js:3809-3817, 4428-4434`).
- **Fix shape:** on `pagehide`, send the timeline or `DELETE` with `fetch(..., {keepalive: true})`.
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-17.8 A lapsed lease (laptop sleep, long suspend) ends in an error card
- **What goes wrong:** when a timeline report finds the session gone, the bridge reports "server ended" and the service fails the play with the end reason (`legacy-bridge.ts:214, 254-267`, `index.ts:718-723`). The viewer must press Try again.
- **Spec:** "A lapsed lease makes the client start a new session at the last reported position, with `replacesSessionId` set" (§17 clarification 16). Only the unwired controller does this (`playback-v1/sessions.ts:232-244`).
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-17.9 Player shortcuts fire with Ctrl, Cmd or Alt held
- **What goes wrong:** the key handler never looks at modifier keys (`Player.tsx:214-231`). Ctrl/Cmd+L (address bar) seeks forward and is blocked by `preventDefault`; Ctrl/Cmd+F (find) toggles fullscreen; Ctrl/Cmd+K, +J and +M are taken the same way. Letters are matched in lower case only, so nothing works with Caps Lock on.
- **How the comparison surfaced it:** Jellyfin's handler starts with "Skip modified keys" and matches physical key codes.
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** not listed.

### B-17.10 Shortcuts stop working after using a slider
- **What goes wrong:** the handler returns for any event whose target is inside an `input` (`Player.tsx:217`). The volume slider and the seek bar are range inputs, and they keep focus after a click. Until focus moves, Space, K, F and M do nothing and the arrow keys change the slider instead. The later `[type=range]` checks on the arrow cases (`:221-222`) can never be reached.
- **Related, speculative:** after clicking a transport button, Space reaches both the document handler and, in Firefox, the button's own click on key-up, which would toggle twice. Jellyfin carries a comment about exactly this ("Trick Firefox with a null element to skip next click").
- **Confidence:** Confirmed by reading (first part); Speculative (Firefox double toggle). **Severity:** Low. **Backlog:** not listed.

### B-17.11 On a touch screen, tapping the picture pauses
- **What goes wrong:** the tap layer's `onClick` is `engine.toggle()` for every pointer type (`Player.tsx:442`). With the controls hidden, the natural "tap to show controls" also pauses the film.
- **How the comparison surfaced it:** jellyfin-web branches on pointer type; touch only toggles the controls (`video/index.js:1801-1810`).
- **Confidence:** Confirmed by reading. **Severity:** Medium on phones and tablets. **Backlog:** not listed.

### B-17.12 Double-click for fullscreen also pauses and resumes
- **What goes wrong:** a double-click delivers two `click` events and one `dblclick`. The tap layer toggles play twice and then toggles fullscreen (`Player.tsx:442`). The viewer sees a pause blip; the server receives a paused and a playing report.
- **How the comparison surfaced it:** jellyfin-web delays the single-click action 300 ms and cancels it on the second click (`video/index.js:1812-1826`).
- **Confidence:** Confirmed by reading. **Severity:** Low. **Backlog:** not listed.

### B-17.13 The Fullscreen button does nothing on iPhone
- **What goes wrong:** `toggleFullscreen` calls `stage.requestFullscreen` only if it exists and says nothing otherwise (`Player.tsx:236-243`). iPhone Safari has not offered element fullscreen; the usual fallback is `video.webkitEnterFullscreen()`, which Jellyfin uses (`playbackmanager.js:1462-1471`). In that native fullscreen Portico's own controls and its scripted subtitles would follow different rules, so this needs a design decision, not only a fallback call.
- **Confidence:** Probable (needs an iPhone). **Severity:** Medium on iPhone. **Backlog:** not listed.

### B-17.14 Buffer targets ignore bitrate
- **What could go wrong:** the forward target is at least 30 seconds whatever the bitrate, and 30 seconds are kept behind (`useWebPlaybackAdapter.ts:208-211`). For a 60–100 Mbps 4K remux that is several hundred megabytes in the source buffer. Browsers cap this; hls.js then reacts to a quota error by shrinking its targets. Jellyfin's comment records stalls from this on Chrome, Edge and Firefox and caps the buffer at six seconds for such streams.
- **Confidence:** Speculative, needs a run with a high-bitrate remux. **Severity:** Medium. **Backlog:** not listed.

### B-17.15 A session that a newer action supersedes is never stopped; a fast double "Next" can end in an error
- **What goes wrong:** `V1QueuePlayer.play` and `advance` drop a late answer when a newer action has started (`if (generation !== this.generation) return;`, `v1-queue-player.ts:333, 459`). The session the server created for the dropped answer is neither adopted nor stopped. It occupies a stream slot and appears in Now Playing until its lease lapses. The native-listening path handles the same race explicitly ("it ends now rather than lapsing with its lease", `:392-397`); the ordinary path does not. `PlaybackService.startPlayback` also handles it (`index.ts:1250-1253`), but the web never uses that path.
- **Double Next:** two quick presses send two `:advance` requests with the same queue revision. The first succeeds and replaces the playing session on the server; its answer is dropped. The second gets 412. The player is left holding the old session, which the server has ended, so the next media or timeline request fails and the error card appears. Retry then starts the queue's new current entry at `recoveryTarget()`, which is the previous title's position (`v1-queue-player.ts:605-610`, `index.ts:1163-1166`).
- **Confidence:** Confirmed by reading for the orphan session; Probable for the double-Next sequence and the wrong retry position. **Severity:** Medium. **Backlog:** not listed.

### B-17.16 Reported speed is always 1; other speeds cause extra reports
- **What goes wrong:** the bridge sends `rate: 1` in every report (`legacy-bridge.ts:184`). Its "steady playback" test assumes position advances one second per second (`:169`), so at 2× or 0.5× the test fails every couple of seconds and a report is sent each time instead of every ten seconds. The extrapolated position at send time also assumes 1× (`:181`).
- **Confidence:** Confirmed by reading. **Severity:** Low. **Backlog:** not listed.

### B-17.17 Resuming deep into a long MP3 or FLAC reads everything before the position
- **What goes wrong:** for containers without a seek method (MP3, FLAC, ADTS, Ogg), `startAt` walks every packet from the start (`bridge/audio/track.ts:149-169`); it does not decode them but it does fetch them, 256 KB at a time, one request after another (`bytes.ts:9, 76-98`). A ten-hour single-file MP3 audiobook resumed near the end fetches hundreds of megabytes before the first sound. The seek limit is 15 seconds and the start limit is 20 (`index.ts:929, 1334`), so over a remote link the resume fails outright. The 64 MB byte cache also means the prefix is not kept.
- **Backlog:** the mechanism is listed ("MP3 and FLAC still decode from the start"). The consequence for audiobook resume over a slow link, and the timeouts turning it into a failure, are not. FLAC's seek table is already parsed and unused (`flac.ts:64-81`).
- **Confidence:** Confirmed by reading for the mechanism; Probable for the failure. **Severity:** Medium (audiobooks are first-class).

### B-17.18 The music engine on iPhone and iPad Safari
- **What could go wrong:** the engine plays through a bare `AudioContext` with no playing media element. On iOS that kind of audio follows the ring/silent switch and is suspended when the page goes to the background unless the page sets `navigator.audioSession.type = 'playback'`. Jellyfin sets it with the comment "This avoids the AudioContext being suspended when Safari is put into background" (`jellyfin-web/src/plugins/htmlAudioPlayer/plugin.js:172-175`). Portico never does (no `audioSession` anywhere in `web/src`). The engine also has no `statechange` listener, and its transport only reacts to the `suspended` state (`DecodeAudioRender.ts:143-151`), not iOS's `interrupted` state after a call.
- **Confidence:** Speculative, needs devices. **Severity:** Medium (music that stops when the phone locks). **Backlog:** not listed.

### B-17.19 Media keys and lock-screen controls for music depend on Web Audio counting as "media"
- **What could go wrong:** in rendering mode the `<video>` element has no source (`useWebPlaybackAdapter.ts:395`), so the only thing making sound is Web Audio. Browsers have historically attached hardware media keys and the now-playing card to media elements, not to Web Audio. Portico sets Media Session metadata and handlers, but whether they are honoured without an active element varies by browser and version. `system-listening-state.ts` already notes that the element is dormant.
- **Confidence:** Speculative. **Severity:** Medium if it fails. **Backlog:** not listed. A common remedy is a silent looping media element alongside the context.

### B-17.20 Subtitles disappear during an outage the video survives
- **What goes wrong:** cues are cleared when the 25-second revalidation has failed for 75 seconds (`options.ts:146-151, 171-196`), while the video can play on from buffer and the reconnect logic waits up to 90 seconds with an empty buffer.
- **Confidence:** Confirmed by reading. **Severity:** Low. **Backlog:** not listed.

### B-17.21 In Firefox, subtitles can sit under the controls
- **What goes wrong:** cues are lifted above the controls with `::-webkit-media-text-track-container` (`Player.module.css:136-139`), which the file's own comment says only WebKit and Blink honour.
- **Confidence:** Confirmed by reading; visual effect Probable. **Severity:** Low. **Backlog:** not listed.

### B-17.22 The seek bar has two overlapping ways to seek
- **What could go wrong:** an invisible range input covers the whole bar and calls `onSeek` on every change (`Timeline.tsx:63-73`), and the bar underneath captures the pointer and calls `onSeek` on release (`:35-49`). The component's comment says a drag "commits once on release". A click may therefore issue two seeks to slightly different positions (the range snaps to half seconds), and how a drag behaves depends on how each browser's native slider interacts with pointer capture.
- **Confidence:** Speculative. **Severity:** Low (each extra seek on a conversion can move the server's converter). **Backlog:** not listed.

### B-17.23 Automatic level switching in hls.js is left on
- **Latent trap:** the hls.js configuration sets `capLevelToPlayerSize: true` and does not pin a level (`useWebPlaybackAdapter.ts:201-221`). Today each presentation has one rendition, so nothing switches. If a manifest ever carries more than one, hls.js will change quality by itself, against the "quality changes only when a person asks" rule, and nothing would flag it.
- **Confidence:** Confirmed by reading (as a latent condition). **Severity:** Low. **Backlog:** not listed.

### B-17.24 The subtitle time mapping can rebuild every cue on a fragment change
- **What could go wrong:** on each `FRAG_CHANGED` the adapter computes `startPTS − start` and, if its string form differs from the last one, fires an event that clears and re-adds every cue (`useWebPlaybackAdapter.ts:228-240`, `options.ts:127-145, 207`). If the difference wobbles by tiny amounts between fragments (likely for copied streams cut on source keyframes), that is up to 20,000 cue objects rebuilt on the main thread every few seconds, and the visible line blinks.
- **Confidence:** Speculative; depends on what hls.js reports for these streams. **Severity:** Low to Medium. **Backlog:** not listed. A tolerance (ignore changes under a few milliseconds) would remove the risk.

### B-17.25 Design risk: the adapter that ships has no behavioural test
- `useWebPlaybackAdapter.ts` is 604 lines of interleaved state (pending seek, route resume, recovery, audio binding) written densely, and nothing in `web/test` or `packages/client-core/test` drives it. The tested adapter is the unwired one. Several findings above (B-17.3, B-17.4, B-17.5) are the kind a fake-element test would catch.
- **Severity:** Medium as a risk multiplier. **Backlog:** not listed.

### B-17.26 Small items
- **Sleep timer on video shows no selection.** The menu ticks "Off" or nothing (`Player.tsx:572`), and the box says only "On". "End of this item" is computed from the remaining time at 1× (`:376`).
- **Speed for audiobooks stops at 2×** though the spec says 3.0 (`listening.ts:11`, spec §12).
- **The shortcut list omits** the live-channel keys (↑/↓, G) it implements (`PostPlay.tsx:136`).
- **Fullscreen icon** is read from `document.fullscreenElement` during render with no `fullscreenchange` listener (`Player.tsx:492`); it corrects itself on the next one-second re-render.
- **Confidence:** Confirmed by reading. **Severity:** Low.

### Looked for and not found
Listener leaks across plays (every listener added in `useWebPlaybackAdapter.ts:565-577`, `options.ts:205-214` and `DecodeAudioRender.ts:68-70` is removed in the matching cleanup); double end reports (the bridge de-duplicates "ended" per generation, `legacy-bridge.ts:129-137`); progress after teardown (reports are fenced by intent, session and generation); a replay loop at the end of a file (`web-playback-binding.ts:12-16` guards the browser's final pause); unbounded recovery (90-second cap, though see B-17.4).

## Where Portico is stronger

- **One timeline.** Element time is title time for direct play, remux and conversion alike. Jellyfin still threads a transcoding offset through subtitles, buffered ranges and progress.
- **Fencing everywhere.** Every late event is checked against the intent, session, generation and DOM node that produced it (`web-playback-binding.ts`, `useWebPlaybackAdapter.ts:166-172`). Stale `timeupdate` events from a previous stream cannot write progress. Jellyfin relies on "is changing stream" flags.
- **Honest resume points.** A position is only reported after the picture has reached it; an unconfirmed start never overwrites a bookmark (`index.ts:1519-1531`, the `bookmarkProtected` flag).
- **Recovery that assumes a home server.** Quiet retry while there is buffer, a single "Reconnecting…" state, route changes handled without losing position, and a clear split between "the server said no" and "the server did not answer". The idea is better than Jellyfin's; the tuning needs B-17.4.
- **The server learns from engine failures** for a week per file and sign-in, so a wrong capability claim costs one retry, once.
- **Probe-based capability profile** with recorded evidence per codec ("declared" or "probed"), bounded and validated before sending, republished when the screen's HDR state or the audio device changes. No user-agent table to maintain.
- **Sample-exact music in a browser**: server-measured trim and loudness, exact frame counts, equal-power crossfade, same-album gapless joins, pitch-preserving speed for audiobooks, a windowed-sinc resampler, and conformance fixtures with known answers. None of the compared web clients attempts this.
- **Memory bounds are explicit**: 30 seconds decoded ahead per deck, a 64 MB compressed-byte cache cleared when the viewer changes (`app/session.tsx:322`), hls.js back buffer capped.
- **Capability URLs**: no cookies, same-origin enforced, no credentials in query strings, `no-store` fetches for audio and subtitles.
- **Skip markers** that the server authorises for unattended skipping and that are reported back for analytics.
- **Subtitle choices are server state**: selection, offset and appearance follow the profile; a language with both text and image tracks picks the text one; a burn-in on 4K/HDR asks first.
- **Autoplay refusals are handled** for both engines with a plain "Press play" message rather than a silent stall (`useWebPlaybackAdapter.ts:433-438`, `DecodeAudioRender.ts:143-151`).
- **Hidden-tab starts wait** instead of failing (`index.ts:928-944`).
- **Owner stops are final and explained**, with the owner's message, and cannot be retried from the player.
- **hls.js supply chain**: pinned commit, recorded hashes, reproducible rebuild, no lifecycle scripts.

## Doc/code disagreements

1. **Server seek.** Spec §5.7 and §17 clarification 4 say a seek in a converted presentation is `PATCH {seek}`. The wired web player never sends it; a seek is a segment request and the server follows (`hls_timeline.go:56-63` describes exactly that). Only the unwired controller implements the `PATCH` (`sessions.ts:304-309`).
2. **Timeline contents.** Spec §6 lists rate, buffered time, bandwidth, dropped frames, volume, mute and track ids. The wired bridge sends generation, state, position and a constant rate (`legacy-bridge.ts:184`). Spec §14's `bandwidthKbps (from timeline)` cannot be filled for web viewers.
3. **Paused cadence.** Spec §6: 30 seconds while paused. The bridge re-arms the same interval (10 seconds) in every state (`legacy-bridge.ts:226-229`).
4. **Lapsed lease.** Spec §17 clarification 16: the client starts a new session by itself. Wired path: error card (B-17.8).
5. **"No playback feature polls"** (spec §15). The player polls the subtitle catalogue and plan every 25 seconds while subtitles are on (`subtitles.ts:147`), revalidates the cue document every 25 seconds (`options.ts:214`), and refreshes lyrics every 30 seconds (`Lyrics.tsx:11`).
6. **Automatic quality.** Spec §5.2: "Auto" is a client-side choice from the client's bandwidth estimate. The decision function exists (`playback-v1/quality.ts`); the web app never calls it and sends no quality at all (and see B-17.1).
7. **Subtitle offset.** Spec §5.5: `subtitleOffsetMs` is client-side for sidecars. The web sends each change to the server (`Player.tsx:553`).
8. **Session changes.** Spec §5.7 describes `PATCH` for audio, subtitles, quality and version with a new generation. The wired path uses a separate subtitle-selection route for subtitles and "play again" for the rest.
9. **`04 Playback and Media.md`, AirPlay:** "the system picker, where the platform has one". On Safari the picker is offered but hls.js disables remote playback for HLS sessions (B-17.6).
10. **`web-media-player.ts:2-3`** says the file is "Not wired to screens yet", which is accurate, but nothing in the architecture docs says the web runs on the legacy `PlaybackService` adapter with a v1 bridge. `06 Clients.md` describes `src/bridge` only as holding "HLS (a patched hls.js…) and WebCodecs audio adapters".
11. **Backlog wording.** "MP3 and FLAC still decode from the start to reach a position": the code skips packets without decoding them (`track.ts:147-148`) but still fetches every byte before the position (B-17.17).
12. **Audiobook speed.** Spec §12: 0.5–3.0. Code: 0.5–2 (`listening.ts:11`).
13. **`Timeline.tsx:9-11`** says a drag "commits once on release"; the range input's `onChange` also seeks (B-17.22).
14. **hls.js README** says the patch is narrow and safe; the patch body still labels itself "Isolated diagnostic candidate" (`anchored-fragment-boundary.patch`).
15. **Image subtitles.** Spec §5.5 puts client rendering of PGS/VobSub first in the precedence order; the web profile declares none (`client-profile.ts:90, 111`). Allowed by the spec, but `04 Playback and Media.md` states burn-in as the rule without noting it is a client limitation.

## Open questions for Justin

1. **Finish the move to `PlaybackSessionController` and `WebMediaPlayer`, or fix the wired adapter in place?** Most High findings (B-17.1, B-17.2, B-17.5, B-17.8) and half the doc disagreements disappear with the new controller, but it has never driven a screen.
2. **Should Safari use its own HLS player?** It would restore AirPlay for HLS and use Apple's tested path, at the cost of a second engine behaviour to support.
3. **Is a pause from the system or a hardware key the viewer's intent?** The "UI intent is the sole authority" rule needs an answer for element pauses Portico did not cause.
4. **Styled ASS and image subtitles in the browser (F-17.1, F-17.2):** in scope for release? They add a WebAssembly renderer and font serving, and remove the biggest hidden causes of full conversions.
5. **How slow may playback be before it is declared failed?** Today: 15 seconds per fragment, 8 seconds without movement, 90 seconds empty. For remote viewers and weak servers these may need to be far more patient, or tied to "the server is still answering".
6. **Should a client-side "Automatic" quality exist at launch** (spec §5.2), given there is no adaptive bitrate by principle?
7. **Subtitle re-authorisation every 25 seconds:** is revoking a subtitle within about a minute worth the polling, when media segments are already fenced?
8. **MKV in Chromium:** keep "always remux", or allow direct play behind an owner switch to spare weak servers?
9. **HDR on Windows Chrome with a standard screen:** keep converting on the server, or offer an owner switch to trust the browser's tone-mapping?
10. **Video outside the full-screen player** (mini player or Document Picture in Picture): is the fixed-layout rule meant to exclude this?
11. **The hls.js patch:** is the failing case written down anywhere? If not, capture it as a fixture before the next upgrade, and consider offering it upstream.
12. **Firefox music:** run the conformance page there and enable WebCodecs, or accept server FLAC conversion for Firefox listeners?
13. **Up next during credits:** acceptable within the fixed player layout?

## Evidence boundary

- Nothing was executed. No browser, no server, no tests.
- Browser behaviour that the findings depend on was not observed: what Chrome, Safari and Firefox do with media keys and Picture in Picture controls when no Media Session handlers are set; whether Web Audio alone drives media keys; iPhone fullscreen and background audio rules; how much a source buffer may hold; how a native range input interacts with pointer capture.
- hls.js behaviour was read from the vendored 1.7.2 source for four files only. Whether `startPTS − start` varies between fragments for Portico's streams, and what the patch changes in practice, could not be established.
- Server behaviour was read only where the player's design depends on it. Whether a session start already positions the converter, how the server answers a segment that is not ready (hold or 503), and how stream caps count orphan sessions were not traced; domain 02 covers the protocol.
- No performance numbers: start time, seek time, memory on long 4K streams, main-thread cost of the audio engine and of rebuilding cues are all unmeasured.
- Plex Web statements are from memory of public documentation and were not re-checked.
- The React Native player was opened only to confirm that it calls the same `play(itemId, position, …, quality)` path as the web (relevant to B-17.1).

## Outside my domain

- **B-17.1 is in client core**, so the Apple and Android apps probably drop quality and audio-track choices the same way. Worth a check by whoever owns the native player domain.
- **User-facing strings in the player are hard-coded English** in many places (`Player.tsx` "Playback stopped", "Try again", "Up next", sleep labels, `SkipPrompt.tsx` verbs, `PostPlay.tsx`), beside others that use the catalogue. That belongs to the language and accessibility domain.
- **Server segment length for copied streams** can reach 30 seconds (`hls_copy_timeline.go:70-73`). Combined with the web's 15-second fragment limit this is a cross-domain interaction (B-17.4).
- **Cast needs HTTPS** (`web/src/bridge/cast.ts:51-54`): on a plain-HTTP home server the Cast choice is simply absent. Domain 13 may want to say so in the UI.
