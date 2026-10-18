# Lane report: web-subtitles

Worktree `/home/justin/projects/Portico/.worktrees/audit-implementation/server-web-subtitles`, branch `audit-implementation/web-subtitles`: 10 commits on top of `b5f3324` (`7d832c5` … `3bafe43`), then `audit-implementation/main` merged in (`e890360`, with the subtitles and playback core lanes) and the wiring finished on top of it. Nothing pushed. No full build or full suite was run (rule 3). No subagent and no helper model was used.

**Nothing in this lane has been seen in a browser.** The image decoders, the overlay, both renderers' page-side logic, the capability declaration, the shared client code and the leftovers are covered by tests that run here. What is *not* covered by anything here is the real libass engine drawing on a real canvas, the image worker loading in a built app, and the whole path from a server answer to pixels. Section 8 is the browser list; section 9 says what is unproven.

**Two things need the owner's eye** (decisions 1 and 3): the libass build contains one LGPL library, and the server's content security policy was loosened by one token.

## 0. After the merge with the subtitles and playback core lanes

`audit-implementation/main` is merged. The one conflict (`web/src/bridge/client-profile.ts`) keeps the new capability document and writes this browser's `subtitleRendering` into it. `packages/contracts` is the base's, untouched. The content security policy still carries `'wasm-unsafe-eval'`.

**In code, styled and image subtitles are now drawn end to end on the web**: the browser declares the kinds; choosing a track asks the session for its original; the server answers with `renderer: "client"` and the original's addresses; the page reads them and draws. Still not seen in a browser.

What was finished on top of the merge:

- **Subtitle choices are made on the playing session** (`PATCH /v1/playback/sessions/{id}` through `PlaybackService.changePlayback`), not only on the subtitle route. A track the browser draws is asked for with `delivery: "embeddedClient"`; an image track that failed to draw here is asked for with `delivery: "burn"`; a track always carries its saved timing. The session therefore knows the track and its delivery, so a quality or audio change and a re-plan after an engine failure come back with the same subtitle drawn the same way, and the web now feeds the "remembered subtitle choice" for the next episode. The Subtitles menu, the C key, the pick made on the title page, the timing nudge, the automatic switch to the original, and the fall-back after a drawing failure all go through this one function (`select` in `web/src/player/options.ts`; the change itself is `subtitleSessionChange` in `web/src/player/subtitles/selection.ts`). The subtitle route remains only as the fallback when there is no session to change or the session refuses.
- **Shared client code**: `PlaybackChoices.subtitleDelivery` and `.subtitleOffsetMs`, sent by `V1QueuePlayer.changePlayback`; `PlaybackService.changesInPlace()`. The stand-in `chooseSubtitleRendering` is removed.
- **Two small server changes in `internal/playbackv1`** (the hand-off both lanes' reports named):
  1. `choice.SubtitleBesideStream` is now true for a track that has an original (styled or image), in `present` and in the plan preview. An engine that lists the kind is no longer given a burn-in beside a repackaged or converted stream; engines that list only `text` are unaffected.
  2. Leaving a subtitle no longer re-plans the stream when the client drew it itself from the original beside the stream (`subtitleReplans`): such a track never weighed in the plan, so turning it off or switching to a text track keeps the bytes. Tested in `subtitle_replans_test.go`.
- A track whose original the server hands over again after it already failed to draw here is not retried.

Run after the merge, all passing: `npx tsc --noEmit` in `web` and `packages/client-core`; `lint:strings`, `lint:design --strict`; web tests `subtitle-images`, `subtitle-overlay` (12), `up-next-credits`, `native-hls`, `subtitle-surface`, `web-media-player`, `web-playback`, `now-playing`, `entity-play`, `container-play`, `sign-in-gates`, `capabilities`; client-core tests `subtitle-originals`, `subtitles`, `subtitle-presentation`, `subtitle-polling`, `mu4-bitmap-subtitles`, `playback-v1`, `playback-v1-core`, `v1-queue-player` (34, one new), `playback-choices`, `client-profile`, `engine-failure`, `listening`, `audiobook-rates`, `i18n`. Go: `internal/playbackv1` and `internal/playback` fail only the four tests the playback core report lists as failing on the base (`TestLargeContainersListInIndexOrder`, `TestDecisionMatrixAcrossDevices`, `TestDeliveryRouteSelectionAndExecutableCodecPolicy`, `TestMPEGTSSeekRestartsOnIndexedPacket`).

Still open after this: see section 9; and three points specific to the wiring.

- **A styled track selected by the server at the start** (remembered or automatic) still starts as text and is switched to its original by the page with one extra request; only image tracks are handed over as originals at the start. Making the planner do it for styled tracks is a change in `applySubtitles`.
- **A device-drawn track the server chose by itself** (no `embeddedClient` on the session) still reloads the stream once when it is turned off; one the viewer chose does not.
- **The hook that ties this together (`usePlayerSubtitles`) has no test of its own**: its parts do (the session change, the overlay, the renderers, the shared client functions), the React glue does not.

Sections 2 to 9 below were written before the merge. Where they say the server pieces were missing (sections 7 and 9, "not connected end to end"), this section replaces them.

## 1. Outcome

- **Styled subtitles (ASS/SSA) are drawn in the browser by libass** compiled to WebAssembly, in a worker, on a canvas over the picture. The script is fetched and decoded by the page (in the encoding the server names, so legacy-encoded files are not garbled) and handed to the engine as text. Fonts attached to the file are fetched from the server's font list; fonts embedded in the script's own `[Fonts]` section are left to libass, which the upstream build sets up to extract them (not verified here; browser test 2); for scripts the default font lacks, a fallback font is added only when the subtitle's text actually uses that script (Arabic, Hebrew, Thai, Devanagari, Bengali, Tamil, Chinese and Japanese). The engine's files are requested the first time a styled track is shown, never before.
- **Image subtitles are decoded by Portico's own code**, in a worker: Blu-ray PGS (`.sup`: presentation, window, palette and object segments, run-length decoding, objects split across segments, cropping, forced flags, windows, palette-only updates, epochs), DVD VobSub (`.idx` + `.sub`: index parsing, program-stream packets, subpicture units across packs, two-field 2-bit run-length data, colour/contrast/area/timing commands, custom colours, forced display) and DVB (ETSI EN 300 743 from the server's Matroska file: page, region, colour table, object and display definition segments, 2/4/8-bit pixel strings, map tables, default colour tables, non-modifying colour, a small Matroska reader that copes with open-ended segments). Pictures are decoded only when a cue is about to be shown. Every size, count and offset is bounded; a broken part is skipped and the rest kept.
- **One overlay** (`web/src/player/subtitles/overlay.ts`) keeps any drawn subtitle in place and in time: the canvas sits exactly on the picture (styled) or on the whole player including the black bars (image subtitles authored for the full frame of a since-cropped film land where they did on the disc), at the screen's pixel density, and follows window size, full screen, the fill/fit mode, a change of the video's size and a move to a screen of another density. The renderer is told the subtitle's time on every video frame while playing and on pause, waiting, seek and speed change. Subtitle time uses the same stream offset the text subtitles use (from the media player port) and the viewer's timing correction, which moves drawn subtitles at once. Choosing another track, turning subtitles off, a new title or closing the player releases the renderer and its worker.
- **One small renderer interface** (`web/src/player/subtitles/types.ts`): a renderer takes a subtitle as bytes or as an address, a canvas and the clock. Styled and image renderers implement it; each is a separate chunk loaded on first use.
- **The browser tells the server what it draws**, in one place (`web/src/player/subtitles/declaration.ts`): `text`, plus `ass` where WebAssembly runs, plus `pgs`, `vobsub`, `dvb`. iPhone declares only `text` (see decision 6).
- **Selecting a track asks for its original** when the browser draws that kind, so the server leaves the video alone. A track the server selected by itself as text or burn-in is switched to its original once. If drawing fails on this device, the same track is asked for the ordinary way (text for styled, burn-in for image), the viewer is told in one line, and it is not retried in that viewing. The Subtitles menu no longer warns about conversion, prefers a text track, or applies the owner's "no burn-in" switch for an image track the browser draws.
- **Viewer caption style**: text subtitles are unchanged (size and background apply through cue styling). Drawn subtitles keep their authored look; the caption size setting nudges their overall size (0.9× to 1.25×, about the bottom centre) and a new **Position** control in the Subtitles menu moves them up or down the picture (kept on the device).
- **Leftovers**: audiobooks play at up to 3× (server, shared client, settings, player). The next title is offered in a corner card while the closing credits run. iPhone uses the system's HLS player; Safari on Mac and iPad hands a stream to its own HLS player when hls.js cannot play its codecs or cannot recover, before anything is reported as a failure. Deep resume for MP3, ADTS and Ogg was left to the audio-engines lane (decision 12).

## 2. Decisions I made

1. **The libass build is not purely permissive, and I vendored it.** The instruction was to fetch a permissively licensed build and check everything bundled. The check found that every published WebAssembly build of libass (`jassub`, `@jellyfin/libass-wasm`, `libass-wasm` all declare the same licence expression) contains **FriBidi under LGPL-2.1-or-later**, because libass cannot be built without it. Everything else is MIT, ISC, FreeType Licence, Zlib, BSD, BSL or public domain. LGPL-2.1-or-later may be distributed inside a GPL-3.0-or-later work such as Portico; the notices ship beside the files in every build and the corresponding source is the pinned upstream commit. If LGPL code in the web app is not acceptable, styled subtitles in the browser cannot be done with libass at all; removing it is deleting `third_party/libass-wasm`, the `ass` line in `declaration.ts` and `ass-renderer.ts`.
2. **`@jellyfin/libass-wasm` 4.2.4, not `jassub`.** `jassub` 2.5.20 was published three days before this work and has four runtime dependencies that would each need vendoring and review. The Jellyfin build is nine months old, has no dependencies, was published by the project's CI with an npm provenance attestation, and is what jellyfin-web ships. Its tarball's SHA-512 matched the registry before anything was copied. Kept: the tarball, four files from it byte for byte, its `LICENSE` and `COPYRIGHT`, `provenance.json` and `verify.py`. The asm.js fallback worker (4.8 MB) is not shipped.
3. **Content security policy: `'wasm-unsafe-eval'` added to `script-src`** (`server/internal/httpapi/router.go`). No browser compiles WebAssembly without it. It does not allow `eval`. This is the only security-relevant server change.
4. **Fallback fonts were copied from this build host's Ubuntu packages, not downloaded**: six Noto Sans fonts (OFL 1.1, `fonts-noto-core`) and Droid Sans Fallback (Apache 2.0, `fonts-droid-fallback`) for Chinese and Japanese, 4.7 MB in all, with package versions and digests recorded. **Korean is not covered** (the Noto CJK font that has it is 19 MB). The better long-term answer is the server offering its own fonts to the browser, the same ones burn-in uses (section 7).
5. **The default font is the build's own Liberation Sans** (metric-compatible with Arial, which most styled subtitles ask for). The package's notice file does not list it; I identified it and its OFL licence from the font's own name table.
6. **iPhone does not declare drawn subtitles.** Full screen on iPhone is the system's video player, which shows the video and nothing drawn over it, and full screen is how video is watched there. So on iPhone styled tracks stay text and image tracks stay burned in. iPad and every desktop browser draw.
7. **Styled subtitles are drawn at no more than 1440 lines** and scaled up beyond that: libass compositing in the browser is CPU work proportional to pixels. Any subtitle canvas is capped at a 4K frame.
8. **Drawn subtitles are not lifted above the controls** the way text cues are; they stay where they were authored (as mpv, Plex and Jellyfin do). The Position control is the viewer's way to move them.
9. **Position is kept on the device** (`localStorage`), not as a profile preference: adding a preference means the server registry, the settings structure and every native app, which is outside this lane.
10. **A track the viewer picks is honoured** when the browser draws it: the "show the text track of the same language instead" rule now applies only to image tracks that would be burned in.
11. **Access is not re-checked on a timer for drawn subtitles.** The original is read once per selection; the subtitle service's own 25-second plan read is what notices a revoked viewer, and the overlay is cleared when the plan goes.
12. **MP3, ADTS and Ogg deep resume left alone.** The lane brief lists it, but its boundaries say to keep out of the music engine, and the audio-engines lane's brief includes deep seeking in that engine; its report says bounded MP3 synchronisation and unindexed deep-seek hand-off are in progress. Doing it here would have collided.
13. **Audiobook speeds are 0.5 to 2 in quarter steps, then 2.5 and 3.** Music and video stay at 2. The web engine's time-stretcher was already written for 0.5–3.0.
14. **Up next during credits never plays anything by itself.** It appears while a closing credits (or outro) marker runs, or for the last 30 seconds of an episode of ten minutes or more with no marker; never for films without a marker, never for opening credits, never when credits prompts are off or the server wants the "still watching?" check. "Play now" goes to the next title and records the credits as skipped; "Watch credits" puts it away. The countdown stays with the end-of-title card. While the card is up it replaces the "Skip credits" button.
15. **Native HLS**: iPhone always (it has only ManagedMediaSource, through which hls.js cannot offer AirPlay; iPhones before iOS 17.1 already used this path). Safari on Mac and iPad keep hls.js first and fall back to Safari's player once per load. Chrome, Edge, Firefox and Android never use a native HLS player, even where one now exists.
16. **The vendored folder is `files/`, not `dist/`**: `.dockerignore` excludes every `dist` directory, which would have silently dropped the engine from container builds.
17. **Shared client code reads the server's `deliveries` list tolerantly in its own file** (`packages/client-core/src/subtitle-originals.ts`) instead of editing `subtitles.ts`, which the subtitles-text lane is rewriting. It compiles and is tested against both the old and the new shapes.

## 3. Bugs

Documents 04 and 17 were worked through by the subtitles-text and web-player lanes; their reports carry the line per bug. Only the entries this lane changed are listed. Everything else in those documents: untouched here.

| ID | Result |
|---|---|
| A-04.9, A-17.13, F-17.1 | Done on web: styled subtitles drawn by libass with the file's fonts. |
| A-04.10, A-17.14, F-17.2 | Done on web: PGS, VobSub and DVB drawn by Portico's decoders. |
| B-04.14 | Web part done (attached fonts used; fallback fonts for seven scripts). Burn-in fonts are conversion-outputs'. |
| B-04.26 | Fixed on web for styled tracks drawn as originals (libass lays out overlapping signs). Text cues unchanged. |
| B-04.39, B-17.24 | Not changed: the text cue set is still rebuilt when the stream offset changes. For drawn subtitles the offset is one number. |
| B-17.6 | Improved: on iPhone the system HLS player is used, so AirPlay is offered for converted streams. Safari on Mac and iPad unchanged (not offered through hls.js). Needs devices. |
| B-17.17 | Not fixed here: MP3/ADTS/Ogg belongs to audio-engines (decision 12). |
| B-17.19 | Not fixed: unchanged from the web-player lane. |
| B-17.26, A-17.39 | Audiobook speed to 3×: fixed (server, client-core, settings, web player). |
| A-17.28, F-17.5 | Done: up next during credits. |

## 4. API changes

No route added or removed.

- **Preference domain**: `audiobooks.defaultSpeed`, and `bookRate` in `GET/PUT /v1/listening/preferences`, accept `2.5` and `3` as well as the earlier values. `music.defaultSpeed` and `playback.defaultSpeed` unchanged.
- **Response header**: `Content-Security-Policy` `script-src` gains `'wasm-unsafe-eval'`.
- **Capability document** published by the web app: `engines.native.subtitleRendering` (or, on the older document, `subtitles.styled` / `subtitles.bitmap`) now lists what the browser draws.
- **Used, defined by subtitles-text**: `deliveries` on subtitle resources, `rendering` on subtitle selection, plan `renderer: "client"`, `GET /v1/media/{grant}/subtitles/{resource}/{revision}/original.{ass,ssa,sup,idx,sub,mks}`, `…/fonts`, `…/fonts/{n}`.
- **client-core additions** (`subtitle-originals.ts`, exported from the index): `subtitleOriginals`, `drawnRenderingFor`, `selectedSubtitleOriginal`, `chooseSubtitleRendering`, `readSubtitleFile`, `readSubtitleFontList`, types `DrawnSubtitleFormat`, `SubtitleOriginal`. `listening.ts`: `audiobookRates`.
- **Web app static paths**: `/vendor/libass-wasm-4.2.4/*`, `/vendor/subtitle-fonts-1/*` (with their licence files).

## 5. Schema changes

None.

## 6. Owner or viewer settings added or wired

| Setting | Type | Default | Takes effect | Screen |
|---|---|---|---|---|
| `audiobooks.defaultSpeed` (existing) | choice, now up to 3× | 1 | audiobook playback speed | Settings (structure updated) and the player's Speed menu |
| `playback.subtitleSize` (existing) | choice | medium | now also the overall size of drawn subtitles (0.9 / 1 / 1.12 / 1.25) | existing |
| Subtitle position (device, `portico.player.subtitle-raise.v1`) | −10 to +30 % of picture height, steps of 2 | 0 | drawn subtitles only | player › Subtitles › Position (Up, Down, Reset), shown while a drawn track is on |

## 7. Needed from others

**Whoever merges (exact steps)**
1. Until the subtitles-text lane's `packages/client-core/src/subtitles.ts` is in, the plan parser refuses `renderer: "client"`, so nothing is drawn and everything behaves as before. No change of mine is needed after that merge.
2. `web/src/bridge/client-profile.ts` will conflict with playback-core on one line in `browserClientProfile`: keep their line and keep the wrapper, `.then(([profile, audioDecode]): CapabilityDocument => declareSubtitleRendering({...}))`. `declareSubtitleRendering` already writes `engines.<name>.subtitleRendering`.
3. `web/src/player/Player.tsx` is also edited by audio-engines (two other lines) and `Dockerfile` / `scripts/container-release.mjs` may be edited by ops-packaging; my changes there are two `COPY` lines and two folder names.

**playback-core (server)**: the planner draws on the device only for a track inside the original file being played; `choice.SubtitleBesideStream` is hard-coded `false` (`server/internal/playbackv1/sessions.go:689`). For the web it must be true whenever the track has an original delivery, for direct, repackaged and converted streams alike; otherwise the server still burns in what the browser declared. Also: automatic selection should pick the original for a device that lists the kind (the web corrects a text or burn-in selection itself, once, at the cost of a second request).

**subtitles-text / conversion-outputs (server)**: a route listing the server's fallback fonts (those burn-in uses) for the browser, so a styled subtitle looks the same drawn or burned and Korean and other scripts are covered. The web seam is `fallbackFontsFor` in `web/src/player/subtitles/fallback-fonts.ts` (or `StyledRendererOptions.fallbackFonts`).

**hosted**: the Hosted web app's own content security policy needs `'wasm-unsafe-eval'`, and its build must serve `/vendor/…`. Originals and fonts are fetched cross-origin from the server exactly as cue documents are today.

**ops-packaging**: attach the libass-wasm source (upstream repository at the commit in `third_party/libass-wasm/provenance.json`, with submodules) to releases the way the FFmpeg source is attached; that is the LGPL obligation. A first real `vite build` should show `vendor/libass-wasm-4.2.4/` (4 files and 3 notices) and `vendor/subtitle-fonts-1/` (7 fonts and 2 notices) in the output.

**native lanes**: `audiobookRates` (to 3×) is in the shared package; native speed menus and engines need to offer and honour 2.5× and 3×. The settings structure row already lists them.

**web-app lane**: `settings-structure.ts` gained `audiobookSpeeds`. `THIRD-PARTY-NOTICES.md` says the Manrope licence is "beside the distributed font files", but `web/src/ui/fonts/` holds only the font. `PostPlay.tsx` still has hard-coded English; the new card uses catalogue strings (`packages/i18n/src/catalog/en-US/subtitles.ts`).

**audio-engines**: MP3, ADTS and Ogg deep resume (B-17.17) stays with you.

**Documents to correct**: `Spec — Playback Protocol v1` §5.5 and `04 Playback and Media.md` (the web renders ASS, PGS, VobSub and DVB itself; iPhone does not); §12 (audiobook 3× now delivered on server and web); `06 Clients.md` (subtitle overlay, native HLS policy); `SECURITY.md` or wherever the policy header is described (`'wasm-unsafe-eval'`); the Backlog entries for client subtitle rendering and up next.

## 8. Test notes for other environments

**Run here, all passing**: `npx tsc --noEmit` in `web` and `packages/client-core`; `lint:strings` and `lint:design --strict` in `web`; `python3 -I third_party/libass-wasm/verify.py`; `go build` of `internal/httpapi`, `internal/operations`, `internal/catalog`; `go test ./internal/operations -run 'TestAudiobookSpeed|Preference'`. Web tests: `subtitle-images` (11), `subtitle-overlay` (11), `up-next-credits`, `native-hls` (5), and unchanged `subtitle-surface`, `web-media-player`, `web-playback`, `now-playing`, `entity-play`, `container-play`, `playback-traffic`, `player-shortcut-key`, `live-captions`, `dolby-vision`, `sign-in-gates`, `capabilities`. client-core tests: `subtitle-originals` (5), `audiobook-rates`, `subtitles`, `subtitle-presentation`, `subtitle-polling`, `mu4-bitmap-subtitles`, `i18n`, `playback-settings`, `listening`, `v1-native-listening`. The Vite plugin was exercised without a build (it emits the 16 files and serves them in development; a path with `..` is refused).

**Failing here and not mine, as far as I can tell**: `go test ./internal/catalog -run Listening` fails `TestListeningMembershipVisibilityUsesIndexedPaths` ("disc path lost index"), a SQLite query-plan assertion on catalogue tables this lane does not touch. I did not run it on the base commit to prove it was already failing.

**Not run**: any build, any browser, the whole web or client-core suites, `go vet`.

**Browser pass.** Needs the merged server (subtitles-text and playback-core in, `SubtitleBesideStream` on). Chrome, Firefox and Safari on a desktop; Safari on iPad and iPhone. In every case "pass" includes: Playback info says the video is played directly or repackaged, not converted, and the network panel shows no new stream after the subtitle is chosen.

1. **Engine loads only when needed.** Play a film with SRT only: no request to `/vendor/`. Choose an ASS track: one request each for `subtitles-octopus.js`, `subtitles-octopus-worker.js`, `subtitles-octopus-worker.wasm`, `default.woff2`, then `original.ass`, `fonts`, each `fonts/N`. Console: no content-security-policy error. Fail: "Refused to compile or instantiate WebAssembly".
2. **Styled look.** An anime episode with typeset signs, karaoke and attached fonts (any fansub MKV with `Attachment` font streams). Also one `.ass` sidecar that carries its fonts inside the script (a `[Fonts]` section). Pass: signs sit on the signs, in their own typefaces and colours, karaoke sweeps, dialogue at the bottom. Compare one frame with mpv. Fail: everything in one plain font, or centred text lines.
3. **Scripts.** ASS files without attached fonts in Japanese, Chinese, Arabic, Hebrew, Thai, Hindi. Pass: readable letters, Arabic and Hebrew right to left and joined, and a request for exactly the matching font under `/vendor/subtitle-fonts-1/`. Korean: empty boxes are expected (known gap). A legacy-encoded file (Windows-1251 Russian ASS): readable, not garbage.
4. **In step.** With an ASS track and then a PGS track: pause (subtitle stays, animation stops), seek forward and back (right line within a frame or two), speed 0.5× and 2× (stays in step for five minutes), frame-step while paused, let the buffer run dry on a throttled link (subtitle holds, does not run ahead), G/H timing nudges move it at once and the value survives a reload.
5. **Converted and repackaged streams.** An MKV in Safari (repackaged through hls.js) with ASS and with PGS, resumed from the middle: subtitles on time from the first line (this is where the stream offset matters). A title played at 720p quality with a PGS track: drawn, on time.
6. **Geometry.** Resize the window, enter and leave full screen, toggle fill/fit (Z), move the window to a screen of another pixel density, rotate an iPad. Pass each time: sharp, on the picture, no stretch. A 2.39:1 film (1920×800) with full-frame 1920×1080 PGS: lines that were in the black bar on the disc sit in the bar. A 4:3 title with VobSub: not stretched sideways.
7. **PGS** from a Blu-ray remux (`.sup` sidecar and embedded): text edges clean, colours right (white is white, not pink or green), fades are fades, two-line and two-position cues, forced-only track. Long film: memory steady after ten minutes of seeking about.
8. **VobSub** from a DVD rip (`.idx` + `.sub`, and embedded in MKV): four-colour subtitles with outline, right colours, right timing at the start and after an hour, both lines of the interlace present (no missing alternate rows).
9. **DVB** from a recorded broadcast (`.ts` with DVB subtitles): boxes and text in the right place and colours, cleared when speech stops.
10. **Release.** Switch ASS → PGS → SRT → Off, then play the next episode. Pass: one worker at most in the browser's task manager at any time, none after Off; no stale subtitle over the next title.
11. **Failure.** Block `/vendor/libass-wasm-4.2.4/subtitles-octopus-worker.wasm` in the network panel and choose an ASS track. Pass: within 45 seconds, one line "…could not be drawn on this device…", the same track as plain text. For PGS, block `original.sup`: the track is burned in (stream reloads) or, with conversion turned off for the account, a plain error. No loop of requests.
12. **Caption settings.** Change caption size in Settings: text cues change as before; a drawn track grows or shrinks slightly about the bottom centre. Subtitles › Position Up/Down/Reset moves a drawn track and is remembered after a reload.
13. **iPhone.** ASS track: plain text; PGS: burned in (expected). Full screen shows subtitles. AirPlay is offered for a converted stream and plays on an Apple TV. Resume from the middle starts at the resume point. Live TV still tunes and can be paused.
14. **Safari fallback.** A stream hls.js refuses in Safari on a Mac (for example Dolby Vision profile 5 in fMP4): plays through Safari's own player without an error card; the server records no route failure.
15. **Up next.** An episode with a credits marker and a next episode: at the credits a corner card names the next episode; Play now starts it; on another episode Watch credits removes it and the end-of-title countdown still appears. A film with nothing queued: no card. With "credits: skip automatically": no card, credits are skipped as before.
16. **Audiobook speed.** Speed menu lists 2.5× and 3×; speech stays at pitch and intelligible; the choice survives a reload and appears in Settings.
17. **Picture in Picture** with a drawn subtitle: the subtitle stays in the page, not in the floating window (known limit).

## 9. Not finished

- **Unproven in a browser**: everything in section 8. The specific risks: the vendored wrapper's canvas-only mode driven by my clock (I read its source; I have not run it); the image worker loading as a module worker in the built app; `createImageBitmap` in workers on Safari; WebAssembly under the new policy; the engine's own console noise.
- **Not connected end to end until merged**: the plan parser (subtitles-text) and `SubtitleBesideStream` (playback-core), section 7. Before those, the server never answers `renderer: "client"`.
- **Korean** and any script outside the seven: no fallback font. Fallback fonts are regular weight only (bold and italic are synthesised).
- **iPhone** draws nothing itself (decision 6). **Picture in Picture** and **casting** do not carry a drawn subtitle; nothing switches to text or burn-in when they start.
- **DVB**: only pixel-coded objects. Character-coded objects and the newer progressive pixel coding are skipped. **VobSub**: colour changes inside one subtitle (`CHG_COLCON`) are ignored. **PGS/VobSub/DVB forced-only display**: the flags are decoded and exposed; there is no viewer control for "forced only".
- **No real sample files were decoded**: the decoder tests read samples generated by encoders I wrote from the same understanding of the formats. A misreading of a format would pass both.
- **Viewer background and colour settings** do not apply to drawn subtitles (by design for styled; image subtitles simply have none).
- **Up next during credits**: no artwork on the card, no countdown.
- **Safari on Mac and iPad** still cannot AirPlay a converted stream (hls.js stays first there).
- **MP3, ADTS and Ogg deep resume**: not done here (decision 12).
- **Hard-coded English** elsewhere in `Player.tsx` and `PostPlay.tsx` is untouched; the subtitle hook's two older error sentences are still literals.
