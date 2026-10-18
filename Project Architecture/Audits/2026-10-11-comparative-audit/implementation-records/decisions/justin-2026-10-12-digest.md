# Justin's decisions on the final report — 12 Oct 2026 (digest; verbatim text is in the session transcript jsonl, the message beginning "So I've finally reviewed your final report")

Status: discussion round. He will send ONE more reply, then wants work to start.

## Decided
- Scanning/naming (line 54, D-02, D-03, Bet 4, D-15): Plex is the gold standard; far more tolerant; minor misspellings handled; folder AND file both feed naming; file (name + embedded metadata) wins on conflict (S02E01 in "Season 1" -> season 2). Season folders or none; "E01"/"01" inside "Season 2". Infer when decently confident; unmatched only at no confidence. Tolerant of bad data with guards against malicious input. Extend past Plex where there is an obvious win.
- Players (line 55, D-04, Bet 6): native player stays primary/default experience. Open to bundling MPV as optional "advanced" player and to an "Automatic" setting (native first, MPV when better), possibly as the default — wants my opinion. Best client experience beats server efficiency. Owner can always set transcoding limits.
- Conversions (D-17): must keep multichannel audio and HDR; HEVC/HDR output; wide audio outputs; tone mapping where HDR unsupported. Deepen player capability profiles (every native player + MPV). "It just plays", Plex level.
- Subtitles (D-18): wide support without transcoding; ASS rendered on device wherever possible, otherwise burned in; server capable of high-quality burn-in; consistent experience across devices. Implementation up to me.
- API (line 56, D-06, Bet 9): full local API, API keys easily generated, webhooks, MCP server. Every action available via API. First-party clients consume the same API third parties would. Designed so rogue clients cannot bring the system down. Sonarr-style "paths changed" notifications for limited scans. Third-party ecosystem is a goal now. Jellyfin-compatible layer: not now.
- Plain HTTP (line 56, Bet 10): never reject, from anywhere. No detection of Tailscale/proxy; no hard-coded restrictions on where the service loads. At most a minor note on the sign-in screen. "Users are allowed to be stupid."
- D-01 / Bet 5: Plex model. Title and file are separate; versions/files come and go; the item and its history stay; removed only when all copies are gone.
- Trash (Bet 10 correction): NOT a folder. Delete in Portico = delete the file on disk. File gone from disk = title kept and marked missing (Plex "trash") so a replacement file re-attaches. Never copy to a trash folder/volume.
- D-05: one capability document.
- D-07: support reverse proxy, Tailscale, IPv6, Windows/Mac/Linux/Docker at launch. CGNAT not blocked. Relay built later (paid), not day one. Stay out of the way of owner-configured routes. DISCUSS FIRST if this means dramatic product/architecture changes.
- D-08: push changes to open screens (~30 s scale), quiet refresh, no immediate invalidation, failures not shown immediately; Plex-style fade-in.
- D-09: decide every major architectural detail now; one API baseline; no backwards compatibility required today; servers attempt to serve any client but old APIs are not maintained.
- D-10: ready = server on Windows/Mac/Linux/Docker; clients on iOS/tvOS, Android, Android TV, Fire TV, Vega OS Fire TV, Roku; consistent features; all current features + today's decisions.
- Bet 1 / D-11: keep server-composed screens; optional bounded API for third parties to request other content; a feature is done only when every first-party client has it, unless technically impossible there (then hide it).
- Bet 2: wants discussion (is brittleness user-facing?).
- Bet 3 / D-14: server may NEVER answer "being prepared" for browse/search; serve old content until new is ready; background work chunked into frequent checkpoints.
- Bet 7 (D-20..23): good defaults the owner can override; avoid a wall of switches; combine into profile drop-downs where sensible, granular numbers where sensible (between Plex and Jellyfin). Responsiveness wins: web UI/console must stay reachable under load. Inert settings: WIRE them, don't remove (remove only if never going to be used).
- Bet 8: no light mode yet. Remember tabs/sort/filter: yes. NEVER themes, custom CSS or plugins.
- Bet 9: he writes the code; no plugins ever; outsiders talk to the server through API/MCP.
- Bet 11: agree; fix all listed defects; deliberate trade-offs stand.
- Bet 12: movies, TV, anime, music, audiobooks, Live TV, DVR, downloads, personal state, discovery all needed at release. Watch Together and casting: fix obvious defects only, low priority. Live TV: one tuner per channel, separate FFmpeg lane per viewer allowed. Downloads may survive server cleanup (metadata must be on device). CarPlay/Android Auto out of scope. Audiobook/music defaults: my leeway. Watch Together: bounded wait after seek yes.
- Bet 13: React Native stands; Roku exists (apps/portico-roku, branch codex/roku-request-resilience, NOT covered by the audit); parity on non-RN clients too. Never let lack of device testing stop work.
- D-28 webhooks: yes, local server only, owner-focused, no Hosted load. No push notifications, no email.
- D-30 import/export: no for now. Trakt maybe later.
- D-31: build our own; avoid dependencies.
- D-32: no outside contributions today; revisit after a few months with users.
- Anything unanswered: take a conservative approach that does not drastically change the product's shape.

## Durable product positions (also in memory)
No plugins/themes/custom CSS ever. No push/email. Own code over dependencies. API + webhooks + MCP for openness. Users may make unsafe choices; caution lightly, never block.
