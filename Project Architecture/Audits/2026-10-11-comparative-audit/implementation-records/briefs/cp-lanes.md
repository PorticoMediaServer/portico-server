# Completion pass — lane scopes

Read `cp-common.md` first. For every audit ID, read its row in the report's Appendix D and the owning lane report it links before changing code; the gap described there is the work.

## cp-browse-parity — "same wide sort and filter in every grouping" (Justin's decision)
- Server fields/pivots and client toolbars for: Home row "See all" pages (recommendation shelves), personal collections, a book's parts/discs, non-season episode groups. Same wide sort/filter set as every other grouping.
- Native My List, Favorites, History and music Playlists: the full wide toolbar (not the narrower list controls).
- A group's own order (playlist position, collection order, episode order, album-type order) as a server sort, so it survives alongside filters where meaningful; Person Cast/Crew keeps the toolbar (add a role operand to the person predicate).
- Cross-library lists (My List/Favorites span libraries): a bounded cross-library browse surface so sort/filter work across libraries rather than one library at a time — keep it bounded and flat-cost.
- B-07.10 authoritative letter anchors (article-stripped sort-key facts from the server); B-18.23 web disc headings from `entry.discNumber`.
- Group choices remembered per group the same way on web and native if that is cheap; otherwise record.

## cp-queues-playlists — whole-view playback and playlist correctness
- B-21.11: every first-party Play / Shuffle / Play-from-here handler (web, native phone/tablet, TV, Vega) forwards the server's query/saved-view selectors instead of loaded entries or the first 500. Sorted and filtered lists play exactly the view.
- B-21.8: duplicate playlist occurrences: occurrence/revision-anchored selectors so the tapped copy plays.
- B-18.29: occurrence-safe Undo for removing an entry from a playlist or collection (web and native), consistent with the existing receipt-based Undo.
- Offline next-episode queue on native (downloaded episodes continue to the next downloaded episode offline).
- Native fade/visual skip for large windowed grids only if it falls out naturally; otherwise leave.

## cp-owner-parity — owner tools and freshness
- Native phone/tablet owner tools still missing: playback problems panel (with title identity), Watching-now pause/resume/seek (with a real scrubber once duration is available), episode-order/offset controls, owner merge covering songs, audiobook files and extras (and the general audio/provider merge gap on the server if small).
- Server: publish authoritative duration on the admin control document; title identity on playback-problem rows; admin session duration — then use them on web and native.
- **List-change events**: owner and viewer list families that currently re-read on a 30-second clock (web) or only on pull (native paged keys, tokens, reports, attention lists, download usage beyond page 1) get server change events/hints, and the clients refresh quietly from them. Remove the 30-second clock where an event now exists. Watch Together joins the freshness hub.
- Web owner: Library Channel builder saves and mount commands showing a false conflict; per-source import-limit readback; tuner person names; subtitle-provider credentials UI (server + web, and phone/tablet owner panel if it fits the owner tools pattern).
- B-18.9: the Administrator role must grant what it says in the web app (and native owner panels): define what an administrator can do versus the owner, consistent with the server's existing role/scope model; don't widen beyond the server's authority.
- B-18.27: reconcile dormant/inert controls and catalogue cleanup.
- Native shared phone page declarations: adopt the canonical ones so local adapters can go.

## cp-native-playback — native and shared playback gaps
- B-21.9: native pre-play subtitle choices keep exact discovery/resource identity and unknown-language alternatives; remembered-choice clear controls; standalone subtitle PUT retired: move native/service/reservation/social consumers to the coordinated session PATCH, then delete the duplicate route (server + clients).
- Offline styled-subtitle fonts: the engines are now Portico's own mpv build (`third_party/media-engine/` in native; see `_program/reports/own-mpv.md`) — make downloaded fonts actually used for local files on Android and Apple (e.g. `sub-fonts-dir` / embedded fonts), including the AVPlayer/ExoPlayer paths if they render ASS.
- B-10.11: Apple limited-bitrate listening: the server's converted AAC target and its selection for the own audio renderer.
- B-11.27: Live TV pre-emption countdown: when a recording needs a tuner a viewer is using, the server already sends `channel.reservation-required` with `reservationRequiredAtMs`; every player (web, native phone/tablet, TV, Vega) shows a clear countdown and what happens next, and handles the stop gracefully.
- B-14.20: Android download transfers resume while the process is dead (installation-owned platform job, e.g. WorkManager), B-14.21 trusted/monotonic receipt clock for expiry.
- B-19.10: an explicit layout choice for TV boxes that don't report "television" (setting and sensible detection); B-19.19 native search keyboard and Android adjustResize follow-ups.
- Chapter and trickplay imagery on native if the server already provides it.

## cp-web-playback — web player and navigation gaps
- Forced-only subtitle control; Up next with artwork and countdown; drawn (libass/bitmap) subtitles while in picture-in-picture and when casting, where technically possible (explain if not); MP3/ADTS/Ogg deep resume; audio output-latency compensation.
- Saved lists, search and person pages retained after Back (scroll and state); the five remaining "refresh required" notices replaced with quiet updates.
- B-17.19: media keys and lock-screen controls for music via Media Session with a real media element where Web Audio alone doesn't count; make it robust in code (device confirmation comes later).
- B-18.30: episode panel on a direct visit: air date, guest/director/writer links.
- iPhone web fullscreen/PiP limitations: improve what's possible, document the rest.

## cp-polish — quality, accessibility, strings, infra
- B-16.4, B-16.5 WindowedGrid end-range discontinuities and memoised cell renderers (web; native equivalent if it shares the issue).
- B-16.7: one long-poll per browser across tabs on plain HTTP too (e.g. BroadcastChannel/leader election), not one per tab.
- B-16.10: descriptive parser salvage across the remaining client-core parsers (unknown enum values degrade, strict authority fields stay strict).
- B-16.12: declare the browser floor (build target + documented support) and degrade older engines explicitly.
- B-16.14: navigation announcements for rapid navigation and identical headings.
- B-16.17: forced-colours and font-scale styling in primitives; accessibility labels; native font scaling/density where code can do it.
- B-16.23: enforce React hook dependency rules (lint) on web and native and fix what it finds properly.
- B-16.25 and B-25.21: one pinned Node version for CI and the container; exact cross-repository dependency revisions recorded and checked instead of moving default branches; native release workflows. B-15.19: resolve the source-map-js advisory. B-19.15: test harness without React internals where feasible, Vega checks in CI.
- B-18.22: remaining English strings (web metadata fallback labels, shared SubtitleService/queue-player messages, native owner job/alert tables, share/merge/Undo/time-help/dashboard and Vega transport keys) into the catalogue.
- B-08.12: conditional responses compare the validator before composing the full response.
- Optional generated consumer declarations: implement or retire each per scope.
- Native: compressed scrolling, memoised cells, deep-list refresh as listed in Appendix 0 viewer gaps.
