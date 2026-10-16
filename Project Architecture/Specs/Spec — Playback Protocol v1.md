# Portico playback protocol — current specification

October programme reconciliation. Existing `/v1` paths are pre-release; release establishes the stable API. The generated `server/api/portico.openapi.json` and current typed client services own exact wire fields. This specification explains behaviour and lifecycle without maintaining a second JSON schema.

## 1. Authority and model

The local server selects authorised titles/files, engine and delivery. A title is separate from versions and parts. A session owns a presentation, media generation, lease, progress authority and optional queue link. Independent devices may play independently. A user-triggered handoff is explicit and fenced. Load estimates do not silently lower fixed quality or refuse an otherwise valid start; real permissions, source failure, owner caps and compatibility still apply.

Sessions bind to authenticated installation/device records. Access/refresh tokens are bearers, with installation ID on refresh; no device possession key exists. Cast pairing proofs are a separate authority boundary.

## 2. Capabilities

`PUT /v1/me/devices/current/capabilities` replaces one capability document; GET reads it. It contains display and active audio-output facts, Player choice, stereo/passthrough/HDR overrides and available engine profiles. Each profile carries supported codecs, limits, containers/transport and subtitle rendering. There is no second translated profile or old capability publication route.

Automatic is the Player default. Native plays original/plain remux where possible. mpv is selected when it avoids native video conversion, lossy audio or burn-in and can play the file. Explicit Native/mpv choices remain, with necessary fallback explained by the plan. The presentation names the engine. Republish on relevant output/settings changes.

mpv is implemented on Apple and Android/Fire clients with Portico's own engine build (`third_party/media-engine/` in the client repository, FFmpeg at the server's pin), but native linkage/device acceptance remains required. Android mpv declares no passthrough, interlacing or DV profile 5; mpv PiP/live/music are not delivered. Vega declares SDR/stereo/text and hides unsupported features. Per-engine display ranges and richer codec rate/resolution limits remain gaps.

## 3. Reads and choices

`GET /v1/items/{itemId}/playback-options` provides current versions, audio/subtitle choices, quality options and the viewer's Settings preferences.

Player choices (Justin, 15 Oct): Settings decide every start. A change made in the player (track, quality, speed, caption look, Forced only, the engine) lasts for that playback only: it is carried through autoplay, completion and queue continuation and is gone at the next fresh start or new queue. Nothing is remembered per title; there is no "forget last choice" route. A plan preview explains direct/remux/conversion and engine choice. Current source facts/permissions decide choices, not a client reconstruction of server policy.

Version/track choices before Play must reach the start. Native's pending-version hook remains an integration item. Multipart version grouping/best-playable version selection and absorbed-ID references need identity/playback review; generic height/size order does not prove the best playable choice.

Markers, chapters and discovered subtitles are independent resources. Subtitle discovery does not require viewer import or eager extraction. Exact search/download/marker routes are in the generated registry, not the old colon-route examples.

## 4. Sessions

| Operation | Route | Behaviour |
|---|---|---|
| Start | POST /v1/playback/sessions | Start under current choices/capabilities; media preparation may return a retryable 202. This does not apply to browse/search. |
| Read | GET /v1/playback/sessions/{id} | Read current owned session and presentation. |
| Change | PATCH /v1/playback/sessions/{id} | Change state/quality/version/tracks or report player failure; preserve queue/current entry. Replan bytes under a new generation where needed. |
| Stop | DELETE /v1/playback/sessions/{id} | Idempotent close with final evidence under the current contract. |
| Timeline | POST /v1/playback/sessions/{id}/timeline | Report position/state/rate/observations and renew the lease. |

Rejected starts/replans must preserve the working grant, queue and progress authority. Replay/reads recheck profile ownership and current visibility. Remote controllers retain the playing device's engine/network context. Use relevant revisions; unrelated progress must not conflict with a semantic command. Reconcile an ambiguous mutation instead of inventing a new intent.

Web and native use PlaybackService and PlaybackSessionController. Local seek is used when the player/range permits; server production restart is requested only when needed. Timeline observations include local track choices and failure facts. A native Player (engine) change re-plans the same session in place (`PATCH` with a replan), keeping the queue entry, choices and position.

## 5. Quality and conversion

Original is the saved default on every network. Necessary compatibility conversion preserves fidelity as far as the admitted path allows. Fixed lower choices have explicit codec-aware ceilings. A session overrides its saved starting choice; owner/member caps remain ceilings.

Automatic is optional, begins at Original and steps down/up with throughput, buffer and stalls. The server retains the explicit intent and receives client steps. It is not imposed by load. Direct files without fragment throughput evidence may react only to stalls. Native start throughput/error hints remain handoffs.

The ordinary planner uses converter inventory and stores executable targets with media generation. Paths include H.264, HEVC 8/10-bit, available AV1, HDR10/HLG preservation, CPU tone mapping and compatible copied or multichannel encoded audio. HEVC/AV1 and applicable lossless audio use fMP4. E-AC-3 eight-channel encode is unavailable. Unsupported DV requires compatible base/qualified reshape; uncaptured mastering metadata cannot be promised.

Rich ordinary playback is distinct from current H.264 SDR/AAC prepared recipes and MPEG-TS live publication. Rich offline recipes, live fMP4 and full two-engine live presentation remain unfinished. Hardware codecs/HDR/audio must be qualified on real devices.

## 6. Subtitles

Prefer original device rendering where supported, including beside remuxed/converted streams. The web draws ASS/SSA, PGS, VobSub and DVB itself; native players draw what their engine declares; iPhone Safari's full-screen player takes a styled track as text. Subtitles are chosen by session change, never by a separate selection route. Otherwise use appropriate text delivery or high-quality burn-in. Track Off, Automatic, forced and explicit choices remain distinct. A subtitle-triggered replan preserves the session/queue.

The server delivers WebVTT, ASS/SSA originals and fonts, PGS SUP, paired VobSub and DVB capsules with grant-scoped range reads. Web has WebAssembly libass and image workers. Native/mpv follow declared render capability. Unsupported renderers are hidden; do not advertise a kind merely because an engine library exists.

Picture in Picture shows no drawn subtitles (Justin, 15 Oct): a burned-in subtitle stays with the picture, a drawn one returns with it, and opening Picture in Picture never asks the server for a conversion. Glyph coverage, real bitmap samples, offsets/seeks, HDR preview/burn, Cast fallback and offline original/companion/font packaging remain acceptance/integration tasks. Device rendering source code is not proof of consistent subtitles everywhere.

## 7. Queues and listening

Queue selectors, windows and prepare/commit-next operations are in the current registry. Stable entry IDs retain duplicate playlist occurrences. Changes/retries do not collapse a queue to one title. Audio successor preparation is not commit; bounded deadlines and exact generation/transition fences govern gapless/crossfade. A late successor must not hold outgoing audio forever.

Music and audiobooks retain local tag/lyric/chapter evidence. Normalisation is off by default; unknown gain uses unity and amplification needs measured peak. Audiobook speed reaches 3× on server/web. Exact resume in MP3, ADTS and Ogg needs no conversion: a per-file seek index (`GET /v1/media/{grant}/audio-index`, named by the direct plan as `seekIndex`) lets the client read from the right byte (Justin, 15 Oct). Pause comfort uses 15 seconds after five minutes and 30 after one hour. Source facts still perform full background decode rather than a general packets-first shortcut. Real priming, deep resume, resampling, latency and gapless fixtures remain required.

## 8. Live, DVR and offline

One upstream live channel input serves private readers/conversions. Tuner use counts channels, not viewers. Shared same-airing recordings remain private per profile with independent deletion. Capture interruption/leader cancellation and platform physical locks have disclosed limits.

Download claims and offline receipts have separate lifetime. Source/title/cache tidying does not revoke valid local bytes. Bundles carry local titles/artwork/text subtitles and other facts; styled/image originals, local extra consumption, next episode, thumbnails and clock/background-transfer qualification remain unfinished. Ordinary sign-out keeps protected downloads unless removal is selected. Access revocation and exact receipt expiry remain enforced.

## 9. Social and remote control

Watch Together is server-owned with host commands, explicit invitations and the contracted 0.75/3-second correction bands. Separate presence/readiness/queue/governance fences avoid false conflicts. Seek/title selection holds the timeline for up to five seconds awaiting readiness, preserving intent; one buffering member does not pause everyone. Host loss pauses at two minutes and ends at ten.

Current receiver/handoff routes retain the committed destination session for control. Do not add the retired generic device-command/first-playing handoff protocol from this specification's former examples. Cast uses narrow paired playback authority and installation-private controller proof. Paused first-frame and destination session-change delivery remain client gaps.

## 10. Events and removed surfaces

One `/v1/events` feed carries scoped hints and session/queue changes, with bounded long-poll fallback. Retired dual capability and legacy start/progress/stop paths are not alternate protocols. Some internal seams and the experimental finite normaliser remain; registry cleanup is in progress. Exact removals and remaining consumers belong to the source reports and generated route inventory.

## 11. Evidence and remaining work

Sources: [playback core](../../../_program/reports/playback-core.md), [playback review](../../../_program/reports/review-playback-core.md), [planner wiring](../../../_program/reports/planner-wiring.md), [conversion](../../../_program/reports/conversion-outputs.md), [native playback](../../../_program/reports/native-playback.md), [web player](../../../_program/reports/web-player.md), [subtitle server](../../../_program/reports/subtitles-text.md), [web subtitles](../../../_program/reports/web-subtitles.md), [audio engines](../../../_program/reports/audio-engines.md), [downloads](../../../_program/reports/downloads.md), [Live TV](../../../_program/reports/livetv.md), [social](../../../_program/reports/social.md).

Follow each report's cold steps. No browser/native SDK build, physical tuner/GPU/receiver acceptance, full suite or deployment is claimed here. The [Backlog](../Backlog.md) groups unfinished work. This specification does not certify original outcome fixtures that have not been ported/executed.
