# Playback and media

The server decides the source, engine and delivery. Clients supply measured capabilities and user choices, play the presentation and report observations. [Playback Protocol](Specs/Spec%20%E2%80%94%20Playback%20Protocol%20v1.md) describes the current API boundary. It is pre-release despite the existing v1 paths.

## One document and two engines

A device publishes one capability document at `PUT /v1/me/devices/current/capabilities`. It contains display/audio-output facts, the Player setting, overrides and profiles for each available engine. There is no second translated client profile.

Automatic (default) uses native for original playback or plain remux. It uses mpv when that avoids video conversion, lossy audio or subtitle burn-in and mpv can play the original. Native and mpv are explicit choices; a necessary fallback is explained in the plan. The presentation names the selected engine. Compatibility includes codecs, bit depth, container/transport, receiver audio, HDR and subtitle rendering.

mpv implementations exist for iPhone/iPad/Apple TV and Android/Android TV/Fire TV. The engine is Portico's own build from pinned source (`third_party/media-engine/` in the client repository): FFmpeg 8.1.2 (the server's pin), mpv 0.41.0, libass, libplacebo and the rest, each a separate dynamic library, LGPL-2.1-or-later throughout. It replaced MPVKit and media-kit's libmpv (15 Oct). Both engines implement `apps/native/Native/ENGINE-CONTRACT.md`. They are source implementations requiring real builds and device acceptance. mpv PiP/live/music are not delivered. Android mpv declares no bitstream passthrough, interlacing or Dolby Vision profile 5; HDR with mpv-drawn subtitles is tone-mapped. Vega uses its native implementation, text subtitles, SDR and stereo. Per-engine display ranges and multiple rate/resolution limits per codec remain gaps. Sources: [playback core](../../_program/reports/playback-core.md), [native playback](../../_program/reports/native-playback.md), [Apple mpv](../../_program/reports/apple-mpv.md), [Android mpv](../../_program/reports/android-mpv.md), [Vega](../../_program/reports/vega.md).

## Sessions and quality

Web and native use PlaybackService, PlaybackSessionController and platform engines. Start/change/stop/timeline calls use the current session API. Version, quality and server track changes replan the same session and retain its queue, position and next entry. Rejected replans retain working authority. A Player (engine) change on native re-plans the same session in place too (`apps/native/src/player/engine-replan.ts`).

Player choices (Justin, 15 Oct): what is set in Settings applies to every playback; a change made inside the player (audio or subtitle track, quality, speed, caption look, Forced only, the player engine) lasts for that playback only. It carries through autoplay and queue continuation and is gone at the next fresh start. Nothing is remembered per title and there is no "forget last choice" control ([cp-followups](../../_program/reports/cp-followups.md)).

Original is the saved default at home and away. Fixed lower choices impose explicit ceilings. Optional Automatic begins at Original, then steps down or up using client throughput, buffering and stalls. It is a viewer choice, never a server-load response. Session quality can override the saved starting value; owner/member ceilings still constrain delivery. Direct-file playback lacks fragment throughput evidence and may react only to stalls. Native start-throughput wiring and useful too-slow error hints remain report gaps.

## Conversion and subtitles

Ordinary HLS planning now selects and persists executable converter targets. Supported paths include H.264, HEVC 8/10-bit and available AV1; HDR10/HLG preservation or CPU tone mapping; multichannel AAC/AC-3/E-AC-3/Opus/FLAC and compatible copied audio. HEVC/AV1 and applicable lossless audio use fragmented MP4. Upstream E-AC-3 eight-channel encoding is unavailable. Dolby Vision requires a compatible base or qualified reshape; full mastering metadata is not captured. Prepared recipes remain H.264 SDR/AAC. Live publishing remains MPEG-TS, so fMP4-only targets cannot be offered there. GPU/HDR/receiver evidence is still required. See [conversion outputs](../../_program/reports/conversion-outputs.md) and later [planner wiring](../../_program/reports/planner-wiring.md).

Discovered subtitle tracks become selectable resources before extraction. The server can deliver WebVTT, ASS/SSA originals/fonts, PGS, paired VobSub and DVB capsules under grants. Device rendering is preferred when the engine supports it; burn-in is the fallback. The web contains WebAssembly libass and its own image-subtitle workers. Browser execution, real sample fidelity and font coverage remain incomplete. Picture in Picture shows no drawn subtitles: a burned-in subtitle stays with the picture, a drawn one returns when the picture comes back, and opening Picture in Picture never triggers a conversion (Justin, 15 Oct). Native/mpv rendering has separate declared limits. See [subtitle text](../../_program/reports/subtitles-text.md) and [web subtitles](../../_program/reports/web-subtitles.md).

## Listening and offline

Music/audiobooks use tags, chapters, lyrics and the shared listening/queue authority. Local lyric discovery is automatic before an explicit choice. Audiobook speed reaches 3× on server/web; engine acceptance remains required. Audio work adds gapless resampling, bounded seek/fallback paths, native gain and priming changes. Accurate gapless/deep-seek/output-latency behaviour still needs real fixtures and device evidence. See [music/audiobooks](../../_program/reports/music-audiobooks.md) and [audio engines](../../_program/reports/audio-engines.md).

Downloads carry a local title/artwork/subtitle bundle and signed permission receipt. Source replacement, title removal or server cache retirement ends the server claim without revoking valid device bytes. Access revocation and receipt expiry are separate. Ordinary sign-out keeps protected downloads; the viewer can explicitly remove them. Styled/image originals, companions/fonts, local player consumption, next-episode playback and clock rollback are not fully delivered. No server snapshot of an original is made merely to keep a download alive. [Downloads](../../_program/reports/downloads.md), [download review](../../_program/reports/review-downloads.md) and [native sync](../../_program/reports/native-sync.md) give exact limits.

## Live and social

Live viewers and DVR share one upstream channel acquisition with private readers/conversions. Recordings stay profile-private; completed same-airing payloads use shared storage ownership. Active leader cancellation is not seamless. Watch Together has distinct command/presence fences and a brief readiness hold; casting/handoff retains independent receiver playback. Paused-first-frame, receiver control delivery and physical acceptance remain gaps. See [Live TV](../../_program/reports/livetv.md) and [social](../../_program/reports/social.md).

## Current independent review

The independent [pipeline review](../../_program/reports/review-playback-pipeline.md) is in progress with reproduced defects. Rich-path implementation reports do not waive those review findings.

Vega sign-in, restore, control and event streams work through its origin-bound transport; the residual redirect risk was accepted by Justin on 14 Oct ([vega-transport](../../_program/reports/vega-transport.md)). The same follow-up supplies Apple scene/reinstall/admission and inactive-page source fixes; earlier native reports' gap lists must be read with it.
