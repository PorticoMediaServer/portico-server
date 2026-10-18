# Lane: playback-core — one capability document, the planner, quality, sessions

Worktree: `/home/justin/projects/Portico/.worktrees/audit-implementation/server-playback-core` (branch `audit-implementation/playback-core`). You can commit.
Audit documents: 02, 20 (capability and negotiation parts), 17 (protocol parts); verification V02. Decision register D-04, D-05, D-13, D-19, D-55.

## What to deliver
1. **One capability document.** Two exist today with a lossy translation between them. Keep one (the richer), make it the wire format, delete the other and the translation. It must describe per engine what the device can really do: containers, video codecs with profile, level, bit depth and resolution, dynamic range (SDR, HDR10, HLG, Dolby Vision profiles), whether the device tone-maps HDR itself, audio codecs with channel counts, **audio passthrough**, and which subtitle kinds the engine renders itself: use the field `subtitleRendering` with values `text`, `ass`, `pgs`, `vobsub`, `dvb` (the subtitles lane uses the same names). A device may send one profile per engine: `native` and, where present, `mpv`, plus the viewer's engine setting (`automatic`, `native`, `mpv`).
2. **The plan names the engine.** With the setting on Automatic the server picks: native when the native engine can play the file directly or with a plain remux; mpv when the alternative is a video conversion, lossy audio conversion or burned-in subtitles and the mpv profile can play it directly; otherwise a conversion for the native engine.
3. **Planner correctness.** Fix the confirmed negotiation bugs: passthrough always refused (B-02.2 / B-20.1), device tone mapping never declared (B-02.4 / B-20.2), the rest of B-02.x. Lossless audio must not silently become 384 kbps AAC when a better target the device accepts exists.
4. **Quality.** Default Original. Fixed qualities below it. `automatic` is a value the client may request: it starts at Original and the client may ask to step down or up; each step is a re-plan of the same session at the current position. Home and remote have separate saved defaults. Owner ceilings, member caps and the upload budget always apply. A client-reported throughput estimate may be attached to a session so the server can refuse a start that cannot work.
5. **Changing quality, version, audio or subtitle during playback reaches the server and keeps the queue** (B-02.6 / B-17.1, server side): one re-plan operation on the session.
6. **Remove the legacy session-start route and any other legacy playback routes** (D-13, B-02.7). Playback Protocol v1 is the only path. Remove the code and tests that exist only for them.
7. **Remembered track choices** (the specification promises it): a profile's last audio and subtitle language choice per show (and per film) is applied at the next start.
8. **Capability overrides** (D-55): a short viewer list — force stereo, disable passthrough, disable HDR — carried in the capability document.

## Boundaries
- The **conversion-outputs** lane owns FFmpeg command construction, encoders, hardware, HLS packaging and burn-in, and is adding HEVC, HDR-preserving, tone-mapped and multichannel or lossless audio targets. Do not edit those. Make the planner able to choose such targets from the capability document against whatever target type exists now; a later lane does the final wiring.
- The **livetv** lane owns `livechannels`, `linearinput`, `linearbuffer`, the recording packages and `playback/channel_sessions.go`.
- The **subtitles-text** lane owns subtitle parsing, extraction and delivery.
- Update `packages/client-core` capability types, protocol services and parsers to the new document. The web and native capability probes and player screens are adapted by client lanes afterwards: list exactly what each must now send and call.
