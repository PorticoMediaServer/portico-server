# cp-web-playback — web player and navigation gaps

## 1. Outcome

Every item in this lane's brief has a code change, and each change has tests. None has been tried on a real device yet.

- **Up next during the credits** now shows the next title's artwork (the episode's own still, or the poster) and a countdown. When autoplay is on and the server is sure of the credits marker (`automaticSafe`), the next title starts when the viewer's post-play countdown runs out. The server counts that as an automatic advance, so "Still watching?" still applies. In every other case nothing starts early: the card says when the end card will start the next title ("Starts in 1:23"). The end card shows the same artwork.
- **Forced only.** Image subtitles (PGS, VobSub) that mark some lines forced get a "Forced only" switch in the subtitle menu. With it on, only the forced lines (foreign dialogue, signs) are drawn. The choice is kept on this device.
- **Drawn subtitles in Picture in Picture.** On Chrome and Edge, Picture in Picture now opens a window that belongs to the page (document Picture in Picture), and the video and its subtitle canvas move there together, so styled and image subtitles stay on the picture. Safari uses its own window, which shows only the video. While that window is open, the drawn track is asked for the ordinary way: text where the server has the styled track as text, otherwise burned in. The page draws it again when the picture comes back. Safari's own presentation-mode moves (the iPhone player's PiP button, swipe home) are followed too.
- **Casting.** The chosen subtitle track (or "off") now goes to the TV with the title. The receiver starts its session with it, and the server burns in styled and image subtitles because the receiver draws text only. Drawing them on the TV itself was not done: see section 9.
- **MP3, ADTS and Ogg deep resume** is exact. A direct plan whose container has no seek index now carries a `deepStart`: the existing converted route from a given frame, lossless FLAC at the file's own rate. A resume or seek past 30 seconds reads from there. It no longer reads the file's prefix or falls back to ordinary playback, so gapless, crossfade and gains keep working. The server offers it only when converting is allowed and the device decodes FLAC at that rate.
- **Output-latency compensation.** The music position, track edges and the end now follow what the speakers are actually playing. That is the context clock minus the output latency, taken from the browser's output timestamp or else its stated latencies. It is bounded to 0.5 s and smoothed.
- **B-17.19 (media keys and lock screen for music).** Music decoded on the device now plays through a real `<audio>` element, fed from the AudioContext through a MediaStream destination. Media Session therefore has a playing media element in Chrome, Firefox and Safari. If the element can't be set up or refuses to play, the audio goes straight to the speakers as before. A pause the system sends to the element counts as the listener's pause.
- **B-18.30.** On a direct visit, the web episode panel now shows the episode's own air date. The server already sent it; the client was dropping it. Guest stars, directors and writers are links to their person pages. On native, the episode panel's directors and writers are now person cards that open their pages; the cast already had them.
- **Back retention.**
  - Saved lists (My List, Favorites, Playlists), History, collections and saved views are kept in the viewer's freshness pool, the same way library pages are.
  - The last five people visited keep their header, chosen role and credits grid, with its loaded pages.
  - Search already kept its service and words. No change was needed there; this was checked in the code, not in a browser.
- **The "refresh required" notices** (library and its row view, collection, show, saved lists, search) are replaced with quiet re-reads. The content on screen stays put. Up to three tries run over about half a minute, with jittered backoff and only while the tab is in front. The notice appears only if the list is still refused after that.
- **iPhone fullscreen and PiP.** The player now follows Safari's presentation modes. In the iPhone's full-screen player it hands a styled track over as text. It does not burn a track in there, because a new picture would close the full-screen player. The rest is written up in `web/README.md` ("Where the browser draws the picture").

## 2. Decisions I made

- **Countdown during the credits.** The next title starts early only when autoplay is on and the server marks the credits marker `automaticSafe`. This follows Plex, but a low-confidence marker never cuts into the story. The countdown length is the viewer's post-play countdown. The early start is a `completion` advance (new `advanceFromCredits`), so passout counting still applies. In every other case the card only says when the end card will start the next title. This partly overrules the earlier "Up next during credits is an offer, never automatic" default (Appendix E). Overrule it if you want no early start at all.
- **Hand-over rules for Picture in Picture and iPhone full screen.**
  - In the browser's video-only PiP window, the track goes to the server's ordinary delivery, which may be a burn-in conversion. This follows your "otherwise burned in" rule.
  - In the iPhone's full-screen player, only text is asked for. A burn-in reload would end the full-screen player and could loop.
  - Burn-in is refused when the owner has conversion off. In that case the PiP window shows no subtitle, and nothing on screen says so.
- **Cast.** I send the subtitle choice rather than putting libass on the receiver. On a text-only receiver the server burns in whatever cannot be shown as text, which follows your decision. libass on a Chromecast is untestable here and risky on its memory and CPU.
- **Deep start format.** FLAC at the source's own rate, so frame numbers stay identical. It is not offered when the device's FLAC capability cannot take that rate. The cost is more bandwidth than the MP3 itself for the remainder of that one track; the next track is direct again.
- **Two smaller choices.**
  - A direct start the browser cannot decode at all also uses the deep start when the position is past zero.
  - "Forced only" is device-local, like subtitle position, rather than a profile preference. It is shown only when the track has both forced and ordinary lines.
- **Media element path.** The carrier element always carries the audio where it can, rather than playing a silent track beside it. The "silent carrier" alternative the earlier lane rejected as speculative was not used.
- **Quiet re-read bounds.** Three tries, an immediate one, then backoff from 1.5 s capped at 12 s. The count resets after a minute of the list being readable.

## 3. Audit bugs closed and decision gaps closed

| ID / gap | What changed | Test |
| --- | --- | --- |
| B-17.19 | `web/src/bridge/audio/carrier.ts`: `MediaCarrier` routes the engine's output through an `<audio>` element. `DecodeAudioRender` plays and pauses it, and an outside pause becomes the listener's pause. Falls back to the speakers. | `web/test/audio-carrier.test.ts` (2 tests). Device confirmation still pending. |
| B-18.30 | Client parses `item.episode.airDate`. `EpisodePanel` uses it on a direct visit and links guests, directors and writers. Native panel adds crew person cards. | web `tsc` and suite; native `tsc` and suite. No dedicated UI test (the screens have no render harness). |
| Up next artwork and countdown (§4 / App. 0) | Queue window entries carry `posterUrl` and `stillUrl`. New `upNextCountdown` rule. `UpNextCredits` and `PostPlay` show artwork. | `TestPlaybackV1QueueWindowArtwork`; `web/test/up-next-credits.test.ts`; `client-core/test/queue-entry-artwork.test.ts` |
| Forced-only control | The worker reports per-cue forced flags. `ImageSubtitleRenderer.setForcedOnly`, overlay `forcedChoice`/`setForcedOnly`, and a menu row. | `web/test/subtitle-overlay.test.ts` (new forced-only test) |
| Drawn subtitles in PiP and casting | Document PiP window move, a hand-over for system surfaces, and the cast `subtitleTrackId`. | Receiver test (`receiver.test.cjs`); `client-core/test/cast-sender.test.ts`. PiP is device-only. |
| MP3/ADTS/Ogg deep resume (B-17.17 residue) | Server `AudioPlan.DeepStart`; v1 `AudioRender.deepStart`; converted route accepts it; client parse; engine `reader().open`. | `TestPlaybackV1DirectPlanOffersADeepStartForUnindexedFiles`; client-core `audio-render-v2.test.ts`; web `audio-render.test.ts` ("an MP3 started part-way…") |
| Output-latency compensation (A-17.26) | `measureLag` and `heardTime`; `Deck.playedFrames(at)`. | web `audio-render.test.ts` ("the position is what is heard…") |
| Back retention (saved, search, person) | Pooled `useSaved`/`usePersonal`; `screens/people/person-memory.ts`. | `web/test/person-memory.test.ts`; web suite |
| Five refresh-required notices | `web/src/app/quiet-refresh.ts` (`quietRefreshStep`, `useQuietRefresh`), used in Show, Search, Saved, Library (two places), Entity. | `web/test/quiet-refresh.test.ts` |
| iPhone fullscreen/PiP | Follows `webkitpresentationmodechanged`; text-only hand-over in the system player; README section. | Device-only |

Naming note: `web/src/app/quiet-refresh.ts` exports `useQuietRefresh`. An unrelated `useQuietRefresh` (a periodic re-read) also exists in `app/freshness.tsx`. They are imported from different modules; rename one if it confuses.

## 4. API changes

- `QueueEntry` (queue windows, `POST /v1/queues`, window reads): optional `posterUrl` (`/v1/items/{id}/art/poster`, present only when that poster exists, including season, show, album or book inheritance) and `stillUrl` (the item's own still, versioned `/v1/metadata/item/{id}/art/{still|thumbnail}?v=…&w=800`).
  - Cost is two keyed lookups per window entry, inside the existing batched query.
  - `TestPlaybackV1QueueWindowStatementsAreConstant` still passes.
- `presentation.audioRender.deepStart` (direct plans of mp3, adts or ogg only): `{url, container: "flac", codec: "flac", sampleRate, channels, bitDepth}`. `GET /v1/media/{grant}/audio-converted?fromFrame=N` now also serves a direct plan that has a deep start. The stored plan carries the internal `convert` spec for it.
- Contracts and OpenAPI were regenerated with `go run ./cmd/apigen`.
- Cast channel: `load` may carry `subtitleTrackId` (a track id, or `"off"`). The receiver validates it (`[A-Za-z0-9._:-]{1,128}`) and starts its session with `subtitles: {trackId}`.
- Client-core:
  - `QueueController.advanceFromCredits?` (V1QueuePlayer: a `completion` advance).
  - `CastChoices` and `castChoiceFields`; `CastController.start(..., choices?)`.
  - `AudioDeepStart` type.
  - `DetailEpisodeInfo.airDate`.
  - `QueuePlaybackView.items[].posterUrl/stillUrl`.
  - The unused `QueueEntry.artwork?: unknown` was replaced by the two typed fields.

## 5. Schema changes

None.

## 6. Settings added

No registry settings. One device-local value: `localStorage` `portico.player.subtitle-forced-only.v1` (Forced only).

## 7. Tests run

- `go build ./...`: pass.
- `go test ./internal/httpapi -run 'TestPlaybackV1QueueWindowArtwork|TestPlaybackV1QueueWindowStatementsAreConstant|TestPlaybackV1QueuePlayAdvanceAndEdit|TestPlaybackV1DirectPlanOffersADeepStart|TestPlaybackV1AudioPresentationSays|TestPlaybackV1AudioRouteCaps'`: pass.
- `go test ./internal/playback ./internal/playbackv1`: pass.
- `go run ./cmd/apigen`: regenerated.
- `node --test server/receiver/cast/test/receiver.test.cjs`: 21/21 pass.
- `packages/client-core`:
  - `npm test`: 1950/1950 pass, on the final code. The first run caught one hand-written `/v1/` literal, which now uses `apiRoot`.
  - `tsc --noEmit`: pass.
- `web`:
  - `tsc --noEmit`: pass.
  - `npm run lint:strings` and `lint:design --strict`: clean.
  - `npm test`: 554/554 pass, rerun at the final commit.
  - `npm run build --configLoader runner`: pass.
- Native `apps/native` after the mirror:
  - `tsc --noEmit`: pass.
  - `npm test`: 665/665.
  - `lint:strings`: clean.
- Vega after the mirror:
  - `npm test`: 87/87.
  - `tsc --noEmit`: pass.
- Browser smoke run, headless Chrome 154 on mini-1 (`web/test/integration/browser-smoke.mjs`): production web build plus the server binary from this branch, fresh state. All 13 groups pass: first-run, subtitles (SRT cue, ASS and PGS pixels through the new picture wrapper, both widths), browse, groups, freshness, console, media (next episode advances; music plays and advances through the new `<audio>` carrier), profiles, tokens, downloads, csp, the screen walk and sign-out.
  - Evidence: `_program/scratch/cp-web-playback/smoke/` (screenshots, `actions-*.json`, `summary.json`). The state, media and Chrome profile were deleted afterwards, along with my GOCACHE.
  - The run does not exercise Picture in Picture, Forced only, the credits countdown, deep starts or latency.

## 8. Device test notes

1. **Up next.** Play an episode whose credits marker is `automaticSafe`, with autoplay on and a 10 s countdown. When the credits start, the card shows the next episode's still and "Playing in 10s" with a filling bar. Pause: the count holds. At 0, the next episode starts, and the owner's Now Playing shows a completion advance. Now repeat until "Still watching?" is due: no card, and the end card asks. With a marker that is not safe, the card shows "Starts in m:ss" and nothing starts early. Turn autoplay off: no count.
2. **Forced only.** Play a Blu-ray PGS full track that has forced foreign-dialogue lines (for example, an English track with forced lines in an alien language). Subtitles menu → Forced only. Only the forced lines show. Reload: the choice is still set.
3. **Picture in Picture in Chrome/Edge.** With ASS or PGS on, press PiP. A small window shows the picture with styled subtitles drawn; resize it and the subtitles stay placed. The main tab shows "Playing in Picture in Picture" with "Back to the player". Close the window: the picture returns, still playing.
4. **Picture in Picture in Safari (macOS or iPad).** With ASS on, enter PiP. The text version of the track appears in the PiP window if the server has text. A PGS track gets a burned-in picture after a reload, if conversion is allowed. Leave PiP: the drawn subtitles come back. Watch for PiP closing on the burn-in reload, and for a loop.
5. **iPhone.** Fullscreen with ASS: the text lines appear in the system player in the system caption style. With PGS: nothing until you exit, and there must be no reload loop. Use the system player's PiP button: the same hand-over as Safari.
6. **Cast.** Choose PGS on the web player, then cast. The TV shows the PGS burned in. Choose Off and cast: no subtitles. Choose SRT: text.
7. **Media keys and lock screen (B-17.19).** In Chrome, Firefox and Safari on macOS, play an album on the device engine (gapless on). Use a keyboard media key, the browser media hub and headset buttons. Each must play or pause, the player's button must follow, and the OS Now Playing must show the title. On an iPhone, lock the screen, use the lock-screen controls, toggle silent mode and take a call. Audio and controls must behave. If the audio is silent on any browser, that browser rejects MediaStream playback: report it, and the fallback branch needs a trigger.
8. **Latency.** Play synced lyrics over Bluetooth headphones (around 150–250 ms latency). Lines should match what you hear, and the position must not jump.
9. **Deep resume.** Use a 1-hour VBR MP3 audiobook and a long Ogg Vorbis file, resumed at 40 minutes on a throttled link. The network panel shows one `audio-converted?fromFrame=…` stream, not the file's prefix. Playback starts promptly at the right sentence, and the end of the track joins the next gaplessly.
10. **Back.** Open My List and scroll; open a title; go Back. The list shows at once at the same place. Do the same for History and a person's credits (scrolled deep), and for Search results.
11. **Quiet re-read.** While a library page is open, force a stale continuation (rescan while paging). No notice should flash, and the list updates in place.

## 9. Not finished

- **Nothing has been run on a device.** PiP (both kinds), Safari presentation modes, iPhone full screen, the carrier element (especially iOS background and lock-screen behaviour with a MediaStream-fed `<audio>`), and real output-latency figures are all untested. The new React wiring (hand-over effect, document PiP move, quiet re-read hook, saved pooling, person memory) has no render test.
- **Cast is burn-in only.** The receiver still draws text only, so styled subtitles arrive burned in, not drawn there.
- **No subtitle carry from native when casting.** Native players don't pass the subtitle choice to Cast yet. The shared `start(..., choices)` is ready for them (cp-native-playback).
- **No native "Forced only".** mpv's `sub-forced-events-only` is the native counterpart (cp-native-playback).
- **Deep start needs a converter.** It requires converting to be allowed. Without that, a deep MP3/ADTS/Ogg resume still uses ordinary playback, as before.
- **Not in this lane.** Watch Together on the freshness hub (§4 list) was not in this lane's brief and was not done.
- **Search scroll restore after Back is not confirmed in a browser.** The service and words were already kept.
- **No rename of the clashing hook.** See the naming note under section 3.

## Commits

portico-server (`audit-implementation/cp-web-playback`):
- 93d08792 Up next artwork/countdown, Forced only, PiP and cast subtitles
- 496a6489 Media element for device music, output latency, MP3/ADTS/Ogg deep start
- c8ab6b4e Quiet re-reads, Back retention for saved lists and people, episode people links
- 308058a7 Safari presentation modes; README on where the browser draws the picture

portico-react-native (`audit-implementation/cp-web-playback`):
- 6a3dff6 Mirror the shared packages
- 3dc1f55 Episode directors and writers as person cards (B-18.30)
