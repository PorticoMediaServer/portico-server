# Playback contract

Contract ID: PC-PLAYBACK. Revision: portico.reconciled.1.

These are the product invariants every playback implementation keeps. The mechanism (sessions, offers, queues, delivery) is [Playback Protocol v1](../../Project%20Architecture/Specs/Spec%20%E2%80%94%20Playback%20Protocol%20v1.md).

**P01 — Independent playback.** The same account/profile may play independently on multiple devices. A new intent MUST NOT stop another controller's playback. Handoff is explicit, authorized and fenced.

**P02 — Owner policy and best effort.** Server Settings MUST provide transcoding enabled/disabled, optional per-account stream caps across that account's profiles/devices, and an optional total server stream cap. Defaults are unlimited stream counts. Predicted CPU/GPU/memory/network/storage capacity MUST NOT reject playback start or terminate existing streams. Attempt delivery and serve best-effort despite overload. Actual execution, source, allocation or disk failure is reported honestly. Codec/capability compatibility and access authority remain real requirements; a load estimate is not a compatibility failure.

**P03 — Server responsibility.** The server selects authorized media sources, delivery plans, transformations and fallback offers. Clients supply user intent and measured/declared capabilities, execute plans and report observations. They MUST NOT manufacture server stream membership, infer permission from capability claims, or send arbitrary encoder arguments.

**P04 — Fidelity and honesty.** Preserve compatible source streams whenever consistent with user quality policy and client capability. Identify necessary conversions explicitly. Do not claim preserved HDR, Atmos, surround, styled subtitles, gapless audio or hardware decoding without evidence for the actual path. Unsupported output choices get a specific explanation and compatible alternative where available.

**P05 — Cross-client contract.** A third-party client MUST be able to negotiate, attach, control, renew and stop playback using documented HTTP/API schemas without Portico TypeScript, a proprietary HLS loader, or private engine field mutations. First-party control hints may improve scheduling, but standard HLS playback and seeking MUST remain useful without those hints.

**P06 — No hidden scope loss.** Movies, TV, anime, music and local audiobooks remain in scope. Direct/remux/transcode, alternate audio, subtitles, lyrics, chapters, multipart media, live/timeshift and offline playback have distinct acceptance gates. A staged implementation does not remove these requirements.
