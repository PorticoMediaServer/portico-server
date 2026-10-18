# Portico comparative audit — final report

**Date:** 11 October 2026 · **Scope:** Portico server, web app, shared client packages and React Native apps (including Vega), compared with Jellyfin's official projects, Plezy and others · **Status:** read-only audit; nothing in Portico's code was changed.

This is the one document to read. Everything else in this folder is supporting detail that it points to by ID.

## How to read this

- **Part 1 (10 minutes)** is the executive summary: what the audit found, and the decisions that matter most before release.
- **Part 2 (30 minutes)** groups the roughly 1,250 architectural differences into 13 underlying design bets. Each one states the bet, what it buys, what it costs, how the others chose, where Portico applies it inconsistently, and the decision it puts in front of you.
- **Part 3** is the decision register: every decision as a numbered item (`D-01` …) with options and my recommendation, so you can answer by number and hand it back.
- **Part 4** covers missing features, with the second reviewer's check of each claimed gap.
- **Part 5** lists bugs separately, with the second reviewer's verdict on each.
- **Part 6** says what this audit could not establish.

**IDs.** `A-07.12` means architectural difference 12 in domain document 07. `F-` is a missing feature, `B-` a possible bug, `N-` a new finding from the second review, `D-` a decision in this report. The domain documents (`01-…md` to `25-…md`) hold the full text and code references for every ID.

**How much to trust it.** All of this is source reading. Nothing was run on a device, and no performance was measured. Twenty Claude Opus lanes and five GPT 6.1 Sol lanes wrote the domain documents. A second pass by GPT 6.1 Sol then tried to refute 425 of the 588 bug claims and checked all 709 "missing feature" claims against the code. Where the two models disagree, this report says so. I resolved two contradictions between auditors myself by reading the code, and say where.

**Your stated philosophy, as I applied it.** Portico values user freedom and choice in how people run it (as Jellyfin does), with product polish, a very slight preference for ease of use over customisability, and a strong feature set delivered consistently across many platforms (as Plex does). Plex is the closer experience target. On security, some user-facing choices deliberately trade security for ease; those are presented as decisions, not defects.

---

# Part 1 — Executive summary

## The short version

Portico's foundations are, in most respects, more deliberate than Jellyfin's and in several places structurally ahead of anything compared. Of the 1,250 differences, the auditors assessed about 390 as Portico ahead, 320 as Portico behind and 350 as genuine trade-offs. The "ahead" entries cluster in the parts that are hardest to retrofit: the data model, access control, playback sessions, recovery, and the shared client core. The "behind" entries cluster in parts that are comparatively cheap to change: how tolerant the server is of messy real-world input, how much playback work is pushed onto the server, features that are built but not connected, and the release and installation lifecycle.

That is a good position to be in before release. It also means the remaining work is less about architecture than about three habits that show up across every domain.

**1. Portico refuses where others guess.** One imperfect row in a guide file, one odd line in a subtitle, one folder that changed during a scan, one unreadable sub-folder, one unfamiliar value from a newer server: in each case Portico rejects the whole thing where Jellyfin and Plex skip the bad part and carry on. This is the single most common root cause of the bug claims (pattern P4 in Part 2). It protects against hostile input, which is good, but it also makes ordinary untidy libraries fail. For someone arriving from Plex or Jellyfin with an existing library, this is likely to be the first thing they notice.

**2. Things are built but not connected.** Automatic quality, API keys, instant mix, the generated API contract, a second web player with the tests, several owner settings that save but do nothing. Forty-two of the verified bug claims are really unfinished features. None of these needs a design change; each needs finishing or removing.

**3. One idea, two implementations.** Two capability documents (the poorer one wins, which is the root of several High playback bugs), two "is this request local?" tests, two restriction systems, two session-start routes (the legacy one skips policy), two web player adapters. Where two exist, the bugs are at the seam.

## Where Portico is clearly ahead

Worth protecting, and worth saying publicly once proven on devices:

- **Access control built into the queries.** Restrictions are part of every catalogue read, not a filter applied afterwards, and nothing in the library is reachable without a credential. Jellyfin's main branch still has unauthenticated stream and image endpoints.
- **Sandboxed media processing.** FFmpeg runs confined and never receives a user-controlled string. None of Jellyfin, Plex or Emby does this.
- **Short-lived, rotating, hashed tokens** against Jellyfin's permanent plain-text tokens, with session families and retry-safe refresh.
- **A data model built for scale.** Kinds as data, files separate from titles, maintained totals and positions, bounded page work, one write gate with priorities. Jellyfin spent its last two release cycles migrating away from the problems this avoids.
- **Server-composed screens and a shared client core.** This is what makes consistency across many platforms achievable for one person. Jellyfin's separate client teams are its most-cited weakness.
- **Evidence, locks, history and undo for metadata**, against overwrite-in-place.
- **Automatic port mapping, names and certificates** with no relay and no cloud dependency for playback. Jellyfin has none of this; the architecture docs undersell it.
- **A maintained taste engine** across every media type, private by construction.
- **Durable everything:** scans, recordings, downloads, groups and TV sign-in all survive a restart.

## Where Portico is behind, in order of how much it matters

1. **Taking in a real library.** Strict naming (the file must name the show, and the folder must agree), no folder-name authority, hidden files catalogued, second copies becoming second titles, a scan paced at about three steps a second, scans that fail if anything changes mid-scan, conservative matching with no "needs attention" list, and a one-request-a-second provider pace. Individually defensible; together they make the first hour with Portico worse than the first hour with Plex. (Themes 4 and 5.)
2. **Playback breadth on devices.** Platform players only, so the server remuxes or converts what Plezy, Infuse and Jellyfin's Android clients play directly. Conversions are always H.264 8-bit SDR. DTS and TrueHD become 384 kbps AAC. Image and styled subtitles force a full video conversion on every client. For a product that targets weak servers, the server is doing the most work exactly where others do none. (Theme 6.)
3. **Openness.** No working API key, no webhook, no import or export of watch state, no reverse-proxy or sub-path support, plain HTTP refused to Tailscale addresses, one provider, few owner-tunable limits. This is where the product is furthest from "freedom in the style of Jellyfin". Portico's own documents state an intent to support independent clients, so this looks unfinished, not decided. (Themes 9 and 10.)
4. **Open screens do not stay current.** The event system exists and is durable, but watch state and library changes are not pushed, and the web app long-polls with two listeners. On a second device this is the most visible difference from Plex. (Theme 3.)
5. **Release and platform lifecycle.** A Windows server cannot add a local library (the one Critical finding), Linux package upgrades leave the service stopped, restore relies on a service manager that Windows and plain Docker do not have, there is no Apple build in CI, no crash reporting, and no physical-device acceptance yet. (Theme 13.)
6. **Depth in the specialist areas.** Live TV opens one input per viewer where the docs say per channel; downloads carry only the media file and can delete themselves when the server tidies up; audiobooks lose their place at 95%; music has no CarPlay or Android Auto. (Theme 12.)

## The decisions that matter most before release

These are the ones that are cheap to decide now and expensive to change once real libraries and third parties depend on them. Full options are in Part 3.

| # | Decision | Why now |
|---|---|---|
| D-01 | **What identifies a title, and where a person's watch state attaches.** Today it follows the file's catalogue row; an upgraded copy starts fresh and a deleted-then-restored title loses its history. | It is a schema decision. It also decides whether "versions" and Plex/Jellyfin import are possible. |
| D-02 | **Tolerant intake.** Should Portico keep hard safety bounds but reject the bad row instead of the whole file, folder or document, and report what it skipped? | It changes the first-run experience more than any feature would. |
| D-03 | **Who names a title: the folder or the file?** Every other product and Sonarr/Radarr assume the folder. | Existing libraries are organised that way. |
| D-04 | **Playback strategy.** Stay on platform players only, add targeted on-device abilities (subtitle rendering, Android audio decoding), or add an optional second engine? And should conversions be able to keep HEVC, HDR and multichannel audio? | It sets server load, quality and the device test matrix. |
| D-05 | **One capability document.** The lossy translation between two is behind at least five High bugs. | Every new client inherits whichever shape ships. |
| D-06 | **Is a third-party ecosystem a goal for 1.0, later, or never?** Tokens, webhooks, a compatibility layer and the public-contract promise all follow from this one answer. | An API is a promise once people build on it. |
| D-07 | **Which ways of running Portico are supported:** reverse proxy, Tailscale or CGNAT, IPv6, Windows, Docker without a service manager? | Several High bugs are release blockers only for configurations you have not yet said you support. |
| D-08 | **How "fresh" should feel.** Push changes to open screens? Show the last known screen instantly and then revalidate? Split the revision counters so a progress tick does not invalidate a browse cursor? | It is the most visible polish gap, and the revision design is easier to change before clients depend on it. |
| D-09 | **The frozen-at-first-release details:** sort-key rules, search scope, database space reclamation, one time format, the oldest client a server must serve. | Each is trivial now and a migration later. |
| D-10 | **What "ready to release" means:** which devices, which platforms, which evidence. | Nothing in this audit or the existing tests establishes device behaviour. |

## Bugs in one paragraph

Of 588 possible bugs, the second review re-checked 425 and refuted only two outright, but found roughly half were narrower than first described. After review there is **one Critical** (Windows local libraries), **69 High** (26 confirmed exactly as claimed, 38 confirmed but narrower, 5 needing a device or runtime check), **298 Medium** and 55 Low; 163 Low-severity claims were not re-checked. The second reviewer classed about 233 as genuine defects, 86 as latent design risks, 44 as deliberate trade-offs, 42 as unfinished features and 15 as missing features mislabelled as bugs. It also found five new problems, two of them High. Thirteen of the High items are small (hours each). Part 5 has the full Critical and High list; `BUG-REGISTER.md` has all 588.

## Features in one paragraph

Of 709 features the first pass called missing, the second review found **436 partly present**, 200 absent, 51 already planned, 21 excluded by one of your recorded decisions, and 1 simply wrong. So most of the gap is unfinished work on foundations that exist, not new invention. Portico's source already implements about 14 of Jellyfin's 25 most-voted feature requests. The second reviewer also raised most size estimates, because a feature is only delivered once it is on every platform. The largest genuinely absent items are import from Plex and Jellyfin, on-device rendering of styled and image subtitles, HEVC and HDR conversion output, CarPlay and Android Auto, and a way for outside tools to connect.

## What I would do first

1. **Answer D-01 to D-10.** Most of the larger work waits on them.
2. **Fix the small confirmed High defects** (Part 5, "Small and safe"): they need no decision.
3. **Collapse the duplicates:** one capability document, one local-or-remote test, one session-start route, one web player adapter. This removes whole families of bugs.
4. **Finish or remove the half-built features** so the product stops promising things it does not do.
5. **Then** intake tolerance and the identity change, which are the two largest pieces of real work, followed by playback breadth.

---

# Part 2 — The thirteen design bets

You suspected the differences boil down to a few larger philosophies. They do. I grouped them independently, the second reviewer grouped them independently (15 themes, in `verification/T-independent-themes.md`, which also assigns every one of the 1,250 entries to a theme), and the two groupings agree closely. The 13 below merge them. For each, the T-numbers in brackets point to the second reviewer's theme, where the complete member list lives.

Each bet has the same shape: **the bet**, **how others chose**, **what it buys**, **what it costs**, **where Portico contradicts itself**, and **the decision**. "Contradicts itself" matters most: those are the places where the fix does not require changing your mind about anything.

## Bet 1 — The server composes; clients render [T08]

**The bet.** The server decides what a screen contains: rows, order, pages, labels, filters, recommendations, and how a file should be played. A shared headless package (`client-core`) owns behaviour and the authored structure of settings, tabs and captions. Web and native apps draw what they are given.

**How others chose.** Jellyfin exposes one generic item query and lets each client build its own screens (A-08.1). That is why third-party Jellyfin clients exist, and also why its official clients disagree with each other. Plezy composes its own hubs on the device and adapts three different servers behind one model (A-21.1). Plex is the consistency target but cannot be read.

**What it buys.** One person can keep seven platforms consistent. A new port gets Home, browse, search, the show page and settings almost for free (A-16.8, A-16.9, A-19.8, A-21.11, A-21.25). Clients never load a whole library. Labels and errors read the same everywhere.

**What it costs.** Every feature needs a server change and then a consumer on each client before it exists; lane 24 found this is exactly where Jellyfin features stall for years (L-24.12). A client cannot do anything the server did not anticipate, so multi-server merging, offline browsing and platform-specific layouts are harder. Outsiders cannot build a different experience on the same API (A-08.42, A-10.46).

**Where Portico contradicts itself.** Clients still guess a Home row's "See all" destination from its id (B-18.28). The same title has two different action lists depending on whether you open its page or its card menu (A-18.14, A-21.28). Web and native offer different choices before play (A-18.12, A-21.27). The native apps expose broad owner administration although an earlier decision reserved that for web.

**Assessment.** Portico ahead, and the right bet for your goals. It is the main reason consistency across platforms is realistic. Keep it; fix the leaks.

**Decision.** D-11: which differences between platforms are legitimate adaptations, and which are parity obligations? And should a server feature only be called done when every first-party client consumes it?

## Bet 2 — Every action is a durable, numbered, recoverable thing [T05]

**The bet.** A request is an intention with an identity, not a transient call. Sessions, scans, downloads, recordings, groups, TV sign-in and token refresh are stored state machines with revisions, leases and receipts. A retry cannot do the work twice, and old work can never pass for current work.

**How others chose.** Jellyfin keeps mutable in-memory session state with start, progress and stop calls, and permanent tokens that need no refresh protocol (A-01.14, A-02.24–29). It is far simpler and it loses things on restart. Plezy has pragmatic recovery inside its player but nothing like Portico's leased command channel between JavaScript and the native engine (A-20.3).

**What it buys.** Scans resume (A-05.1). Refresh is retry-safe (A-01.18). Progress from two devices is ordered instead of last-write-wins (A-02.36, A-12.14). Restore is staged (A-09.48). Recordings report honestly how complete they are (A-11.24). This is the largest group of "Portico ahead" findings.

**What it costs.** Complexity, and brittleness when the bookkeeping is too coarse. One catch-all revision number per resource means unrelated activity invalidates real work: a Watch Together heartbeat makes the host's command fail with a conflict (B-13.1, confirmed High); a playback progress tick invalidates browse cursors and Home (B-07.4, B-12.20); a token renewal rebuilds services on every mounted native screen (B-19.2). The second reviewer calls this pattern P6. Simple things also become heavy: an "original" download is copied into the state folder and hashed twice before a byte is sent (A-14.2), where every other product serves the file.

**Where Portico contradicts itself.** Offline progress is written straight into the tables, bypassing the ordering rules the online path enforces (A-14.29, B-14.10). A legacy session-start route skips member limits and never appears in Now Playing (B-02.7, High). The web app ships the older player adapter, without behavioural tests, while the stricter controller with the tests is not wired in (A-17.35, B-17.25).

**Assessment.** Portico ahead in design. The costs are mostly granularity and leftover parallel paths, not the idea.

**Decisions.** D-08 (split the revision counters). D-12: finish the move to the new web playback controller, or repair the shipped one? D-13: remove the legacy session-start route?

## Bet 3 — Do the work before the page opens, and stay fresh by revision [T04, T15]

**The bet.** A library of millions must feel like a small one. Totals, sort positions, search documents, facets and recommendation features are maintained in the background when the library changes. A page does bounded, indexed work. Nothing is cached by time; a result may be reused only when keyed by the exact revisions it read.

**How others chose.** Jellyfin computes at query time over one wide table, with offset paging and timers. Its 10.11 release was largely a painful migration to fix the resulting slowness and locking (L-24.1 to L-24.4). Navidrome and Audiobookshelf are simply smaller problems.

**What it buys.** Exact totals, jump-to-letter, and stable paging at any size (A-07.12 to A-07.14, A-16.24, A-19.27). One snapshot per request. A write gate that keeps playback ahead of a scan (A-07.17). The auditors rate this ahead of everything compared, with the caveat that no capacity has been measured.

**What it costs.**
- **Rebuilds take features offline.** Raising a derivation version makes browse or search answer "being prepared" for the whole rebuild (B-07.2, High). The second reviewer confirms it and notes the persistence README promises the opposite.
- **One bad entity can stall the workers** that maintain the second tier of derived data (B-07.1, confirmed High).
- **Lifecycle cost hides outside the page test.** Scan bookkeeping is never pruned (B-05.20, raised to High); every scan re-queues every file (B-05.21); ledgers and vocabularies grow without bound (B-07.15).
- **"Fresh" is not the same as "current".** The event feed is durable and resumable, but watch-state and library changes are not published to open screens (A-08.24, A-16.14). Each screen also refetches on Back because nothing is kept between screens (A-16.7). A screen can be exactly correct when loaded and stale a minute later.
- **Idle chatter.** Native apps long-poll every 10 seconds and web every 25, each with database reads (A-22.43), which sits awkwardly beside "an idle server does no polling work".

**Where Portico contradicts itself.** Some lists ignore the windowing rule and load or truncate: native History (B-21.5), Genres and Series tabs capped at 200 (B-18.4), Saved View playback covering only what was loaded (B-21.11). Guide reads are cached by time (A-22.37). The browse-capabilities cache has no revision key at all (B-16.19).

**Assessment.** Portico ahead, and worth the complexity for the stated scale goal. The gaps are operational (rebuild availability, pruning) and experiential (live refresh).

**Decisions.** D-08 again: push changes to open screens, and allow showing the last known document instantly before revalidating? D-14: should a rebuild keep serving the old rows until the new ones are ready? D-09: the details frozen at first release.

## Bet 4 — Refuse, don't guess [T07, pattern P4]

**The bet.** When input is uncertain, Portico declines to act instead of guessing: a title with no year never auto-matches; an episode file the reader cannot parse is refused; a provider record that fails validation is rejected; a guide file with one bad programme is rejected whole. Metadata is evidence with provenance, locks, history and undo, not the latest response overwriting the last.

**How others chose.** Jellyfin and Plex guess generously and let you fix mistakes afterwards: "best result wins" matching (A-06.2), dozens of filename patterns (A-05.15), skip-the-bad-row parsing (A-11.6). Their libraries fill up quickly and contain some wrong matches.

**What it buys.** Fewer wrong matches. A real audit trail and undo for metadata (A-06.5, A-06.20), which neither Jellyfin nor Plex has. Strong protection against hostile files (A-11.14, A-15.27).

**What it costs.** This bet is behind more High bug claims than any other.
- **Naming.** Movie and show titles come from the file name, never the folder (A-05.14, A-05.16). A show whose files disagree with its folder is rejected (B-05.11); flat show folders are only understood inside a `Season N` folder (B-05.10, confirmed High). `1920x1080` is read as season 1920 episode 1080 (B-05.12). The auditor ran Jellyfin's naming tests through a copy of Portico's readers to find these.
- **Scans.** A folder that changes during a scan fails the scan, and analysis never starts (B-05.2). One unreadable sub-folder stops every scan of that source (B-05.4, confirmed High).
- **Matching.** Yearless titles never match and there is no list of what was left behind (B-06.2). Titles that look like years confuse the parser (B-06.1).
- **Live TV.** A standard XMLTV file with a `DOCTYPE` line is rejected (B-11.2, confirmed High). One odd programme, a missing stop time or a gzip file rejects the guide (B-11.3 to B-11.6). One imperfect entry rejects an M3U playlist (B-11.7).
- **Subtitles, lyrics, chapters.** One odd line rejects an ASS script (B-04.10), an LRC file (B-10.8) or all of a book's chapters (B-10.16).
- **Clients.** The strict response parsers reject a whole document on one unknown enum value, so a newer server can break an older app (B-16.10).

The second reviewer lowered several of these from High to Medium on impact, and noted that some rejections are written policy (bounded whole-document rejection, DOCTYPE refusal). It agreed with the pattern.

**Where Portico contradicts itself.** The bulk lyrics run publishes the first search hit unverified (B-10.6), the opposite of the matching caution everywhere else. A local NFO keeps ownership of a field after the NFO is deleted (B-06.15).

**Assessment.** Trade-off, but currently set too far toward refusal for a product with a slight preference for ease of use. The useful distinction is between *unsafe* input and *untidy* input. Keep every size, depth and time bound. Change the unit of rejection from the document to the row, publish the good rows, and show the owner what was skipped and why. That preserves the security posture and removes most of the failures above.

**Decisions.** D-02 (tolerant intake), D-03 (folder or file), D-15: keep conservative matching but add a "needs attention" list and accept folder and IMDb id hints? D-16: stay with one provider for everything, or one *identity* provider plus typed supplementary sources for ratings and artwork?

## Bet 5 — Identity follows the file and the server's own rows

**The bet.** A film is the catalogue row its file was scanned into; a person's watch state hangs off that row. Files are identified by device, inode, size and time. Shows are keyed by title and year, episodes by number, albums by folder and year, live programmes by channel and start time.

**How others chose.** Jellyfin derives ids from the path but keeps user data keyed so it can re-attach by provider identity. Plex groups files that match the same provider record into versions of one title. Navidrome derives ids from tags.

**What it buys.** Stable ids that do not change when metadata does (A-07.2). Files cleanly separated from titles (A-07.3). An ordinary rename or move on the same disk keeps everything.

*A correction worth knowing:* two first-pass lanes claimed a renamed or moved film loses its watch state. The second reviewer traced the scanner and found move detection; that claim was wrong. What remains true is narrower and still important.

**What it costs.**
- **An upgraded copy is a new title.** Replace `Film (2021).mp4` with a better encode under a different name and Portico catalogues a second film; the watch state stays on the old one (B-05.8, High, already in the Backlog). There is nothing for the Versions control to list.
- **Delete and re-add loses history.** Personal state cascade-deletes with the title (A-07.5); Jellyfin detaches and re-attaches it.
- **No import or export.** Because state is keyed to Portico's own rows, there is no path to bring watch history from Plex or Jellyfin (A-12.62) or to sync with Trakt (A-12.61). Lane 24 ranks import as the strongest adoption lever.
- **Fragile keys elsewhere.** Show identity by title and year splits or merges shows (B-05.27). Albums split across common layouts (B-10.17). A recording follows a programme identity that is only channel plus start time when the guide has no ids (A-11.11). The device number in file identity may change across reboots on network mounts (B-05.18, unsettled).

**Assessment.** Portico behind, and this is the most important schema decision in the audit, because it is the one that gets expensive after release. The research lane's framing is right: file identity, work identity and edition identity are three different things (L-24.10). Portico has the first and needs the second.

**Decision.** D-01: add a work identity (provider id where known, Portico's own otherwise) that watch state attaches to, with files as versions of a work? What should survive a delete and re-add, and for how long?

## Bet 6 — Platform players, with the server as the compatibility layer [T09]

**The bet.** Video plays through each platform's own player (AVPlayer, ExoPlayer, the browser). When a device cannot play a file as it is, the server remuxes or converts it. The app stays small and gets system features (Picture in Picture, AirPlay, lock-screen controls) for free. Music is the deliberate exception: Portico's own decode engines chase Plexamp-grade gapless playback.

**How others chose.** Plezy embeds mpv everywhere and direct-plays almost anything. Swiftfin ships two engines (VLC and AVPlayer). Jellyfin's Android clients add FFmpeg audio decoding and on-device ASS and PGS subtitle rendering. Infuse built its own engine and is the quality bar on Apple TV. All of them push work to the device to spare the server.

**What it buys.** Small binaries, correct system integration, no codec builds to maintain and no GPL/LGPL engine to clear through app stores (A-20.31, A-20.41).

**What it costs.** Portico is the only product compared whose Apple client cannot play an MKV without the server (A-20.1). Beyond that:
- **Conversions are always H.264 8-bit SDR in MPEG-TS** at a fixed ladder bitrate. No HEVC output, no HDR-preserving output (A-02.9, A-03.6, A-03.7).
- **Lossless audio (DTS-HD, TrueHD) is converted to lossy 384 kbps AAC 5.1** (A-20.9), which the spec says should not happen. The second reviewer found an E-AC-3 5.1 path does exist for qualifying devices, so how often AAC is the result needs a device check.
- **Image and styled subtitles always burn in**, forcing a full video conversion on web, Apple and Android (A-04.9, A-04.10, A-17.14, A-20.19). The spec promises client rendering. Three lanes found this independently.
- **Live TV** copies only H.264 with stereo AAC; everything else converts (A-11.4).
- **The device's real abilities are lost in translation.** Native apps probe rich capabilities, but the server plans from a poorer second document. Passthrough audio is always refused (B-02.2 / B-20.1) and the device's own HDR tone-mapping is never declared (B-02.4 / B-20.2), so the server does work the device could do. Both confirmed and raised to High.
- **No automatic quality.** The "Automatic" option exists in the spec and half in code, and nothing calls it (A-02.19). Quality changes only when a person asks, by principle, but there is also no bandwidth test to pick a sensible start.

For a product whose stated target includes weak servers, this is the bet in most tension with its own goals: the server does the most work in exactly the cases where competitors do none.

**Where Portico contradicts itself.** The music path proves Portico is willing to own a decode engine when quality demands it. Android declares ASS and PGS rendering it does not have (B-20.11). Refresh-rate matching exists on tvOS only (A-20.16). Quality, version and audio choices made in the player never reach the server and discard the queue (B-02.6 / B-17.1, High, confirmed by both models from both ends).

**Assessment.** Trade-off, but the current setting is costly. The native-players auditor's ordering is sensible and avoids the big decision for now: (1) fix the capability translation; (2) add FFmpeg audio decoding on Android; (3) render image and styled subtitles on the device, starting with web; (4) add a lossless or E-AC-3 audio target and HEVC/HDR conversion output; and only then (5) decide whether an optional second engine is still needed.

**Decisions.** D-04 (strategy), D-05 (one capability document), D-17 (conversion outputs), D-18 (subtitle rendering and the import-before-play model), D-19 (automatic quality).

## Bet 7 — Fixed budgets and built-in limits; the owner rarely chooses [T03, pattern P5]

**The bet.** Portico protects weak hardware with budgets it sets itself: background work is deprioritised and paced, never paused; requests are admitted or refused by class; sizes, counts and durations are capped. The owner gets few knobs, and quality never changes unless a person asks.

**How others chose.** Jellyfin exposes almost everything (scan concurrency, encoder choice, thread counts, forty library options) and lets the owner get it wrong. Plex hides most of it but adapts automatically.

**What it buys.** A server that should stay responsive on a small box. Typed overload responses that clients respect (A-08.19, A-22.34). No way for an owner to misconfigure the encoder.

**What it costs.** The limits are constants, and several bite ordinary use.
- **Pace.** Scans run at about three steps a second per source, however fast the disk (B-05.1, High; the second reviewer calls it a small fix). Metadata runs at one provider request and one title a second, so a first import of a large library takes days (B-06.11).
- **Caps.** All local artwork shares one 128 MiB folder, after which covers are dropped (B-06.4 / B-10.2, High). 256 connections (B-09.7). 4 GiB per conversion session (B-03.8). 16 MiB and 2,000 channels for a guide source (B-11.5), which rules out most IPTV line-ups. 32 tracks per file; over that, none are used (B-04.5).
- **Clocks.** Fixed timeouts turn "slow" into "failed": 90 seconds to extract up to eight subtitle tracks (B-04.4, High); 15 seconds per video fragment and 8 seconds of stall on the web (B-17.4, High); 10 seconds for a restore's integrity check (B-09.1, High); 10 minutes for a download address, shorter than an OS pause (B-14.3, High).
- **No overrides.** When capability detection is wrong there is nothing the viewer can do (A-20.8). The owner cannot tune the database, choose an encoder or raise a cap.

**Where Portico contradicts itself.** "Deprioritised, never paused" is the principle, yet analysis stops while anything plays from the same source (A-05.34). Several owner settings exist, save and do nothing: the conversion temp folder (B-03.10 / B-09.9), several Live TV settings (B-11.9), device approval (B-01.4). The second reviewer calls this pattern P3 and notes it is worse than an honestly missing feature.

**Assessment.** This is where Portico is furthest from "freedom and choice". The slight preference for ease of use argues for good defaults, not for the absence of the setting. My recommendation is one rule applied everywhere: every built-in limit is a default the owner can raise, shown with its current usage; every timeout is tied to "is the other side still making progress" instead of a wall-clock constant.

**Decisions.** D-20 (limits become owner-adjustable defaults), D-21 (scan and provider pace), D-22 (timeouts tied to progress), D-23 (remove or wire the inert settings).

## Bet 8 — An authored interface with few choices [T10]

**The bet.** Portico has one look and one way of working: a fixed dark theme, fixed card geometry (posters for film and TV, squares for music and books), a fixed player layout, server-ordered Home, and one Settings structure shared by every client. Customisation is deliberately restrained.

**How others chose.** Jellyfin offers themes, custom CSS, branding, per-tab view modes and remembered sort and filter. Plezy offers themes, AMOLED mode, accent colours, density, per-library view modes, spoiler hiding, gesture and shortcut editors and a long playback settings list. Plex sits in between and is the polish target.

**What it buys.** Coherence. One hero for every type, a single show workspace, consistent empty and error states, a design system that lint rules enforce and that web and native share (A-18.37, A-18.45, A-19.33, A-21.25). The auditors rate the visual language ahead.

**What it costs.** Some of what is missing is taste; some is function.
- **Things a person expects to be remembered are not.** A library always opens on Discover with no remembered tab, sort or filter (A-18.9, A-18.10). The last audio and subtitle choice is not remembered per title or show, although the spec promises it (A-04.23, F-02.6).
- **Accessibility gaps that are not cosmetic.** Dark only, English only at launch, no main landmark or skip link on web (B-16.13), a static page title (B-16.14), cards that do not announce watched state (B-16.15), subtitles on native that ignore both Portico's and the system's caption style (B-20.12).
- **Player comforts.** No zoom or aspect modes, no gestures on phones, no scrub thumbnails or chapter marks on native, a short keyboard map on web (A-20.23 to A-20.26, A-17.30).
- **No spoiler protection** for unwatched episodes, which lane 21 ranks as the most valuable native gap against Plezy.

**Where Portico contradicts itself.** Settings labelled "This device" actually apply to every device of that class (B-18.19, B-21.7; found on web and native independently). About fifteen web screens bypass the shared language catalogue (B-16.6, B-18.12). At phone width the web app loses Switch profile, Switch server and Feedback (B-18.14). Roughly 530 strings in the catalogue are unused (B-18.27).

**Assessment.** The restraint fits "a very slight preference for ease of use over customisability". Today the preference is strong, not slight. I would keep the fixed look and add the small set of choices that are about memory and accessibility, not decoration.

**Decisions.** D-24: which appearance options belong (a light or high-contrast theme; nothing; bounded styling)? D-25: remember tab, sort and track choices? D-26: spoiler protection, and view modes on native? D-27: fix the "This device" label or make the settings truly per device?

## Bet 9 — A small, owned codebase over ecosystem reach [T12]

**The bet.** Portico prefers code it wrote and controls: its own subtitle and tag parsing, image pipeline, audio engines, design system, transport and navigator, with very few dependencies and narrow forks of hls.js and the SQLite driver. There are no plugins. Code contributions are closed. The API is designed for first-party clients.

**How others chose.** Jellyfin spreads work across plugins, SDKs, third-party clients and hundreds of contributors, and pays for it in compatibility breaks and uneven quality (L-24.24, L-24.25). Navidrome adopted the Subsonic protocol and inherited dozens of clients without writing one, and has just added a Jellyfin-compatible music API. Plezy leans on mpv and mature libraries.

**What it buys.** A small attack surface (no owner-to-code-execution feature exists at all, A-15.24). Explicit provenance for FFmpeg and hls.js. A codebase one person can hold in their head, with guard tests that encode the project's own rules (A-25.19, A-25.21).

**What it costs.**
- **No way in for anyone else.** API keys have storage, listing and revoking, but creating one is disabled and nothing authenticates with one (B-01.3, confirmed, classed as an unfinished feature). There are no webhooks, no scrobbling, no Trakt, no Sonarr/Radarr "this path changed" hook (A-23.28), no Seerr link, no Subsonic or DLNA. Jellyfin's ecosystem (Jellyseerr, Jellystat, Streamyfin, Finamp, Infuse support) exists because of an open API with a working credential.
- **Owned parsers have coverage gaps.** Tags are read through ffprobe and a whitelist, so multi-value artists, sort tags and some ReplayGain forms are missed (A-10.1). No character-set detection, so older subtitle files show as garbage (B-04.9). NFO reading is narrow (A-06.23).
- **Two forks to carry** (the SQLite driver and hls.js), and three separate gapless decode engines to qualify.
- **The typed contract is mostly unused.** Only about 66 of roughly 566 API operations are on the generated registry, and the generated TypeScript package has no importers; clients use about 221 hand-written path literals (A-08.3, A-16.12). Two lanes counted this independently.
- **No community intake** even for translations, which matters once a second language is wanted (A-25.48).

**Where Portico contradicts itself.** This is the important one. Portico's own documents state an intent to support independent clients (A-25.47), the API guide is written for outsiders, and a key-management screen exists. So "closed" is not a recorded decision; it is an unfinished one. It is also the bet most at odds with "freedom in the style of Jellyfin".

**Assessment.** I would separate three things that are currently bundled: *who writes Portico's code* (you; keep it), *whether there are in-process plugins* (no; keep it, for security), and *whether outsiders can talk to a Portico server* (currently no; I think this should be yes). A scoped personal access token plus one outbound webhook would cover most of what Jellyfin's plugins are actually used for, without a plugin system. A compatibility layer is a bigger and separate question.

**Decisions.** D-06 (ecosystem stance), D-28 (webhook), D-29 (a Jellyfin-compatible or Subsonic layer), D-30 (import and export of watch state; tracker sync), D-31 (which commodity pieces to keep owning and which to borrow), D-32 (accept translations and documentation from outside?).

## Bet 10 — A free local server with a small Hosted service, run one way [T01]

**The bet.** The owner's server is complete on its own: one binary, one state folder of plain SQLite and files, copyable to another machine. Hosted adds accounts, a server directory, names and certificates, and nothing else: no relay, no metadata, no media. Remote access is automatic port mapping or the owner's own forwarding. A service manager restarts the server.

**How others chose.** Plex has a mandatory cloud account, a relay, and now charges for remote streaming, which is the top reason people leave it (lane 23). Jellyfin has no cloud at all and leaves reverse proxies, certificates and Tailscale to the owner, which is the top reason families give up on it (lane 24). Portico sits deliberately between them.

**What it buys.** The best remote-access story of the three on paper: automatic mapping over four protocols, real certificates, no monthly fee and no dependency on Hosted for playback (A-09.23 to A-09.25). Already signed-in devices keep working when Hosted is down (A-01.37). Clients verify a server's cryptographic identity before sending credentials to a remembered address (A-22.3, A-22.4), which neither Jellyfin nor Plezy does.

**What it costs.**
- **Only one way of running it is really supported.** Behind a TLS-terminating reverse proxy the bundled web app's writes are refused unless an allow-list variable is set (B-08.1), rate limits treat everyone behind the proxy as one client (B-08.7 / B-15.3), and sub-path hosting is not possible (A-16.18). Plain HTTP is refused to Tailscale, carrier-grade NAT and global IPv6 LAN addresses because "private" is hard-coded (B-09.8, confirmed). The default listener is IPv4 only (B-09.21), which removes the only direct route for owners behind CGNAT.
- **No answer for CGNAT** until a relay exists.
- **One volume for everything.** Cache, prepared media, backups and the trash for deleted media all live in the state folder beside the database (A-09.13, B-09.13, B-09.18). Deleting a 60 GB film copies it onto the state volume, where it is never purged automatically.
- **Restart depends on the platform.** After a restore the server exits and waits to be started; on Windows and plain Docker nothing starts it (B-09.2, High). A `.deb` or `.rpm` upgrade leaves the service stopped (B-09.3 / B-25.20, High, small fix).
- **Windows is advertised and not ready.** A Windows server cannot add a local library because file identity is unimplemented there (B-05.28, Critical), cannot extract embedded subtitles (B-04.31), and runs as a scheduled task with awkward defaults (B-09.4, B-09.5).
- **One server at a time** on a device; no merged view across servers (A-22.22).

*A contradiction I resolved by reading the code:* the security lane said the server does nothing to stop a browser signing in over plain HTTP at a public address. The operations lane said public peers are disconnected at the socket. The operations lane is right (`internal/networking/certificate_listener.go`, `localHTTPConnection`): plain HTTP is accepted only from loopback, private and link-local peers. The exception is when the peer address is hidden by Docker's bridge or a proxy, where everyone looks private (B-15.11).

**Where Portico contradicts itself.** The Server architecture document describes remote access as "port forwarding, or none", underselling what exists. "Plain HTTP stays first-class" is a principle, but the owner's own "home networks" setting does not decide who may use it.

**Assessment.** Portico ahead in concept and the closest of the three to your stated philosophy. The gap is the supported-deployment matrix: reverse proxies and Tailscale are how the Jellyfin audience actually runs things, and they are the audience most likely to try Portico first.

**Decisions.** D-07 (supported deployments), D-33 (may cache, backups and trash live outside the state folder?), D-34 (should the server restart itself?), D-35 (relay, and several servers at once).

## Bet 11 — The household is the authority; secure inside, relaxed at the door [T02, T06]

**The bet.** An account contains profiles, and the selected profile is a real server-side authority that every surface respects. Hosted proves who someone is; each server decides what they may do. Dangerous operations are contained by construction. At the front door, ease wins: a correct password always signs in, there is no lockout, password rules are light, plain HTTP is allowed on the LAN, nothing is encrypted at rest.

**How others chose.** Jellyfin has a flat user list with about forty per-user switches, permanent tokens stored in plain text, and several endpoints with no authentication. Plex has managed users and PINs under a mandatory central account.

**What it buys.** The auditors' longest "ahead" list. Restrictions compiled into SQL (A-01.10). Device-scoped rotating tokens stored as digests (A-01.14, A-01.15). Media URLs that are per-playback grants (A-15.4). Sandboxed FFmpeg (A-15.22). Server-side request forgery, path, XML and image defences that the security lane checked class by class against Jellyfin's published advisories (A-15.25 to A-15.29). Local membership that Hosted cannot override (A-01.36).

**Genuine defects** (both models agree these are bugs, not choices):
- The owner's "remote sign-in: owner only / off" policy is enforced on the legacy sign-in route and not on the one clients actually use (B-01.2 / B-15.1, confirmed High).
- Starting playback without an idempotency key takes a legacy path that skips member limits (B-02.7, High).
- A television used as a Cast target holds a full session with the pairing person's role, which can be owner, and that credential passes the owner admin-read path (B-13.14, raised to High).
- When a server learns a Hosted signing key was revoked, it rejects new assertions but does not end sessions already issued under it (N-V01.1, new, High).
- One approved but uncollected TV code permanently breaks a clean-up job (B-01.5 / B-15.2).
- Three different parsers decide the client's address from proxy headers (B-15.6).
- The Server document says a device registers a key. No such key exists; tokens are bearer tokens tied to a random installation id (B-15.7). The second reviewer classes this as a documentation error. It is yours to decide whether to build the key or correct the claim.

**Deliberate trade-offs** (working as intended; the residual risk is stated so you can confirm each):
- **No lockout, light password rules** (A-01.21, A-01.23). Risk: slow online guessing. A possible low-friction mitigation is to delay only unfamiliar devices, though a stricter sign-in mode is currently excluded by a recorded decision.
- **Plain HTTP on the LAN** (A-15.36). Risk: anyone on the same network can read passwords and tokens.
- **Nothing encrypted at rest** (A-15.38, B-15.5). Risk: a backup contains working sessions and two-factor seeds. A restore brings revoked sessions back (B-09.19). Low-friction mitigation: leave live secrets out of backups, or sign everyone out on restore.
- **First-run setup open to whoever reaches the server first** (A-01.29, B-15.4). Risk: on a LAN, any web page a household member has open could claim an unconfigured server, because the Host check that would stop it is not active. This one sits on the line between trade-off and defect.
- **Approving a TV code gives that TV the approver's role** (B-15.13), decided on 2 October.

**Costs elsewhere.** Two restriction systems with different rating scales, so "PG" hides "TV-PG" (A-01.5, B-01.10). A thin ratings table against Jellyfin's 44 country files. Changing someone's permissions signs them out everywhere (A-01.13). A member cannot be deleted, only disabled (A-01.44). The owner cannot see all members' devices.

**Assessment.** Portico well ahead on construction. The trade-offs are coherent with your philosophy. The defects above are worth fixing regardless.

**Decisions.** D-36: confirm the five trade-offs as decisions, with or without the mitigations. D-37: device keys — build, or correct the document? D-38: the first-run window. D-39: one restriction engine, and how far parental controls go. D-40: should a permission change apply live instead of signing the person out?

## Bet 12 — One product for every kind of media, with depth chosen selectively [T11, T13, T14]

**The bet.** One server and one app cover film, TV, anime, music, audiobooks, Live TV and DVR, owner-scheduled channels, downloads, Watch Together and personal discovery. Depth is bought where it matters most (music playback, recommendations) and kept shallow elsewhere.

**How others chose.** Jellyfin is broad and uneven. Plex pairs a broad product with a specialist music app. The successful newer projects are narrow: Navidrome for music, Audiobookshelf for books, Channels for DVR, each deeper than Portico in its own area.

**What it buys.** A household installs one thing. Shared playback, state and restrictions across all media. The recommendation engine and Library Channels are original.

**Where each area stands** (detail in the domain documents):
- **Music (doc 10).** Ahead on the gapless design and the server-owned queue. Behind on tag reading, album grouping, lyrics that do not appear on their own (A-10.34), and the absence of CarPlay, Android Auto, radio and scrobbling. "Play next" goes to the end of the queue when shuffle is on (B-10.3).
- **Audiobooks (doc 10).** A file is marked finished and its resume point reset to zero at 95% (B-10.1, confirmed High, small fix). No bookmarks, no metadata source, narrator read from one tag.
- **Live TV and DVR (doc 11).** *A second contradiction I resolved by reading the code:* the architecture document and one auditor say live input is shared per channel; another auditor says per viewer. The work is keyed by playback session (`internal/playback/channel_sessions.go`), so it is per viewer. Two people on one channel use two tuners, and a recording uses a third (A-11.1). The latest Live TV contract records sharing as a future capability, so this is a known limit; it is the architecture document that is wrong. Every guide refresh retunes every live viewer (B-11.1, confirmed High). The first upstream error ends a recording for good (B-11.11). DVR is private per profile, so two people recording the same programme record it twice (B-11.26; a contract choice). Recordings appear as title-only "movies" (A-11.23).
- **Downloads (doc 14).** Strong on integrity and ownership. A downloaded file is deleted when the server's retention for the claim lapses (B-14.1, confirmed High). A failed transfer restarts from zero although the server supports resume (B-14.2). Only the media file travels: no subtitles, artwork or metadata (A-14.21). Four of the six quality options offered cannot actually be encoded (B-14.4, confirmed High). Offline plays reach no history (B-12.5).
- **Watch Together and casting (doc 13).** Nobody waits for anybody after a seek, and a member's heartbeat invalidates the host's command (B-13.1, B-13.2, High). Cast stops when the phone disconnects (B-13.7). "Play on" moves playback but cannot then control the other device.
- **Personal state and discovery (doc 12).** Ahead on the taste engine, two histories and the composed Home. "Mark show watched" skips episodes that are in progress, because an item's own state beats a later container mark; the code and a test encode this, the button's wording does not (B-12.1). No tracker sync, no import, no "watched together".

**Assessment.** The breadth is a real differentiator and matches the Plex-style goal. The risk is promising eleven journeys and delivering six well. The research lanes' advice is to name the complete journeys you will stand behind at release and mark the rest as early.

**Decisions.** D-41: which journeys are release promises? D-42: Live TV — share input per channel, and is the DVR household-wide or per profile? D-43: downloads — should a device copy survive server tidying, and what travels with it? D-44: audiobook and music rules, and is the car in scope? D-45: Watch Together — allow a short bounded wait after a seek?

## Bet 13 — Many platforms from one shared app; proof still to come

**The bet.** One React Native app with native modules covers iPhone, iPad, Apple TV, Android phones and tablets, Android TV and Fire TV, with Vega as a thin variant, all sharing `client-core` with the web app. The server, web app and shared packages live in one repository; the native apps keep a checked copy.

**How others chose.** Plezy ships phone, tablet, TV and desktop (Windows, macOS, Linux) apps from one Flutter codebase, with a full store and package-manager release pipeline. Jellyfin has a separate team and codebase per client. Streamyfin is the closest technical sibling (React Native with Expo).

**What it buys.** Portico gets each platform's real focus engine on TV, which Plezy had to rebuild by hand (A-19.20). Bounded-memory paging with jump-to-letter is ahead of every compared client (A-19.27). The dependency count is tiny (A-19.7). Shared screens mean a fix lands everywhere.

**What it costs, and what is unproven.**
- **Nothing has been accepted on a physical device.** The Android, Fire TV and Vega ports are a week old. Emulator checks are explicitly not accepted as proof. Five High or Medium player claims cannot be settled without a device (B-20.4 video continuing after leaving the app on Android, B-20.5, B-20.7, B-20.9, B-20.19).
- **No Apple build in CI, no release pipeline, no crash reporting, no log a user can send** (A-19.38, A-19.39, A-19.43, A-19.46). Lane 19 recommends crash and log reports sent to the owner's own server, as jellyfin-androidtv does, which fits the privacy stance.
- **Cold-start deep links are dropped**: Top Shelf, Watch Next, Siri and universal links (B-19.1).
- **Dependence on a small fork** (react-native-tvos), with Vega holding the shared code to an older React Native version (A-19.4).
- **Release engineering stops short of the final artifact** (pattern P16): the Windows ZIP is built from the wrong path (B-25.3), the Docker release has no qualified FFmpeg input (B-25.4), manual release CI can test a different revision from the one packaged (B-25.19), the update feed's shape does not match what the server reads (B-09.11 / B-25.1), and installers are unsigned.
- **No compatibility promise.** Nothing defines the oldest app a server must serve or the oldest server an app must tolerate, which matters for TV apps that update slowly (A-08.6, A-25.41).
- **An unanswered licence question:** whether GPL-3.0-or-later code can be distributed through Apple's App Store as it stands (A-25.50, B-25.12). The second reviewer left it unsettled. It needs a deliberate answer before submission; I am not able to give legal advice on it.

**Where Portico contradicts itself.** One web auditor said nothing checks the copied shared packages; two others found the check runs in CI. The check exists. What does drift is the separate Go toolkit copy in Hosted (B-25.5). The Overview document still says only the web and Apple apps ship.

**Assessment.** The cross-platform bet is sound and is the right one for a single developer chasing Plex-style reach. What is missing is proof and pipeline, not architecture.

**Decisions.** D-10 (definition of ready), D-46 (client and server compatibility window), D-47 (crash and log reports to the owner's server), D-48 (the Apple licence question), D-49 (keep copied packages, publish them, or one repository).

## Recurring patterns that are not philosophies

The second reviewer identified sixteen implementation habits that recur across domains (`verification/T-independent-themes.md`, section 3). These six account for most of the bug list, and each suggests a process change more than a design change:

| Pattern | What it looks like | Suggested habit |
|---|---|---|
| **Built but not connected** (P1) | Automatic quality, API keys, instant mix, native chapters and scrub previews, search history | Track a feature as command → consumer → visible effect on each platform |
| **Two implementations of one idea** (P2) | Capability documents, local-or-remote tests, session-start routes, web player adapters, restriction engines | Every migration names the production path and a removal date |
| **Settings saved but not applied** (P3) | Conversion temp folder, Live TV options, device approval, poster size | A check that every stored setting has a consumer with an effect |
| **One bad record fails the whole** (P4) | Guides, playlists, subtitles, chapters, scans, client documents | Reject the smallest safe unit; report what was skipped |
| **A declared capability is wider than the real one** (P8) | Android subtitle rendering, download quality options, passthrough | Test the declaration through to the engine that must honour it |
| **Tests on the helper, not the shipped path** (P14) | The unwired web player has the tests; TV behaviour is proven by hand | Attach evidence to the wiring that ships |

---

# Part 3 — Decision register

Answer by number. The numbers are stable labels, not an order: they were assigned as the report was written, so they do not run in sequence within each table. "Rec." is my recommendation; you are the authority. Sizes are rough: **S** hours, **M** days, **L** a week or more, **XL** a project. "Blocks" lists what waits on the answer.

## A. Decide before release (expensive to change later)

| # | Decision | Options | Rec. | Size | Blocks |
|---|---|---|---|---|---|
| **D-01** | **Title identity and where watch state attaches** (Bet 5) | (a) Leave as is. (b) Add a *work* identity that state attaches to; files become versions of a work; state survives delete and re-add for a grace period. (c) As (b), keyed by provider id where known so state can also be imported and exported. | **(c)**. It is the one schema change that unlocks versions, upgrade continuity, import and tracker sync together. | L–XL | B-05.8, A-07.5, F-12.4, versions, import |
| **D-02** | **Tolerant intake** (Bet 4) | (a) Keep whole-document rejection. (b) Keep all safety bounds; reject the smallest unit (row, line, file, sub-folder); publish the rest; show the owner what was skipped. | **(b)**, as one policy applied to guides, playlists, subtitles, lyrics, chapters, NFOs, scans and client response parsing. | L (many small pieces) | B-11.2–7, B-05.2, B-05.4, B-04.10, B-10.8, B-10.16, B-16.10 |
| **D-03** | **Who names a title: folder or file?** | (a) File only (today). (b) Folder is the authority, file is the fallback, as Plex, Jellyfin, Kodi and Sonarr/Radarr assume. (c) Either, with the stronger evidence winning and conflicts shown for review. | **(b)** for shows and films; keep the strict reader as the fallback. | M–L | B-05.10, B-05.11, A-05.14–16 |
| **D-04** | **Playback strategy** (Bet 6) | (a) Platform players only; improve server outputs. (b) Platform players plus targeted on-device abilities: subtitle rendering, Android FFmpeg audio. (c) Optional second engine (mpv) on some platforms. (d) Universal engine by default. | **(b) now**, revisit (c) after measuring how many conversions remain. Avoids the licence and maintenance cost of an engine until it is proven necessary. | L | F-20.1–5, A-04.9–10, device matrix |
| **D-05** | **One capability document** | (a) Keep two with a translation. (b) Make the richer planner profile the wire format. (c) Grow v1 with the missing fields and delete the other. | **(b) or (c)**; either is fine as long as one is deleted. | M | B-02.1–5, B-20.1–2, B-20.8, B-20.10 |
| **D-06** | **Third-party ecosystem stance** (Bet 9) | (a) First-party only; remove the API-key screen and the independent-client wording. (b) Scoped personal access tokens and a published, versioned subset of the API. (c) As (b) plus OAuth later. | **(b)**. No plugins, but a way in. It is the cheapest large step toward "freedom in the style of Jellyfin". | M | B-01.3, D-28, D-29, D-30 |
| **D-07** | **Supported ways to run Portico** (Bet 10) | For each, choose supported / documented workaround / unsupported: reverse proxy on its own hostname; sub-path hosting; Tailscale or other overlay; carrier-grade NAT; IPv6-only or dual-stack; Windows; Docker without a service manager; Synology, Unraid, TrueNAS. | Support reverse proxy on a hostname, Tailscale as a "home network", dual-stack listening and Docker. State sub-path as unsupported. Decide Windows explicitly (see D-10). | M each | B-08.1, B-08.7, B-09.8, B-09.21, B-15.3, B-15.6 |
| **D-08** | **How fresh should feel** (Bets 2, 3) | Three separate choices. (i) Push watch-state and library changes to open screens? (ii) Paint the last known document instantly, keyed by its revision, then revalidate? (iii) Split the catch-all revision into separate counters (content order, personal state, commands, observations)? | **Yes to all three.** (ii) does not break the "no time-based caches" rule: it is keyed by revision, not by time. (iii) removes a family of conflicts. | M–L | B-07.4, B-13.1, B-08.8, A-16.7, A-16.14, F-08.6 |
| **D-09** | **Details frozen at first release** | (i) Sort keys: natural number order and skipping leading punctuation. (ii) Search: match original titles, artists and years; punctuation-insensitive; CJK substrings. (iii) Database space reclamation: must be chosen before the first release database exists. (iv) One time and duration convention across the API. (v) Rating scale: 0–10 integers or half-stars. | Do (i), (ii) punctuation and original titles, (iii) incremental, (iv), and pick (v). All are cheap now and a rebuild or migration later. | S–M each | B-07.6, B-07.7, F-07.5, F-07.10, F-07.15, A-08.41 |
| **D-10** | **Definition of ready** (Bet 13) | Which platforms are advertised at release; which physical devices define acceptance; which evidence must exist at the tagged source. Specifically: is Windows in or out for the first release? | Name a small device set (one Apple TV, one iPhone, one Fire TV Stick, one Chromecast with Google TV, one Vega device). Hold Windows back unless B-05.28 and B-04.31 are fixed. | — | B-05.28, B-04.31, B-09.2–5, release claims |

## B. Architecture clean-up (no change of mind required)

| # | Decision | Options | Rec. | Size |
|---|---|---|---|---|
| **D-11** | Platform parity rule | (a) A feature is done when the server has it. (b) Done when every first-party client consumes it, with a short written list of allowed platform differences. | **(b)** | process |
| **D-12** | Web playback path | (a) Fix the shipped legacy adapter. (b) Finish the move to the newer controller that already has the tests. | **(b)**; it removes B-17.1, B-17.2, B-17.5, B-17.8 together. Fix N-V09.1 in the same change. | M–L |
| **D-13** | Legacy session-start and sign-in routes | Remove, or keep and apply the same policy checks | **Remove** once clients are confirmed off them; until then apply the checks. | S–M |
| **D-14** | Derived-data rebuild availability | (a) Readers wait ("being prepared"). (b) Old rows keep serving until new ones are ready. | **(b)** eventually; the second reviewer rates it a substantial change. For release, at least show progress and make failures non-blocking (B-07.1). | L |
| **D-23** | Inert settings | Wire each, or remove it from the console until it works | **Remove now, wire later**; list in Part 5. | S |

## C. Intake and metadata

| # | Decision | Options | Rec. | Size |
|---|---|---|---|---|
| **D-15** | Matching strictness | (a) Keep "two signals or ask". (b) Keep it, add a "needs attention" inbox, and accept folder-name and IMDb id hints as a second signal. (c) Relax to best-result-wins. | **(b)** | M |
| **D-16** | Provider model | (a) One provider for everything (today). (b) One *identity* provider plus typed supplementary sources (critic ratings, extra artwork, biographies). (c) Ordered chains as Jellyfin. | **(b)**. Note `decisions.md` still describes a selectable fallback and an AniList default for anime, which conflicts with the Catalogue document; one of them needs correcting. | M |
| **D-21** | Scan and provider pace | Fixed pace (today), or as fast as the storage and provider allow within a politeness limit, with an owner "gentle / normal / fast" choice | **Adaptive with a three-way choice.** What first-import time is acceptable for 2,000 films is the number that sets it. | S–M |
| **D-50** | Real-time monitoring and "paths changed" | (a) Neither. (b) File watching for local folders. (c) An API call that Sonarr/Radarr-style tools can use to say what changed. | **(c) first** (needs D-06), then (b). | M |
| **D-51** | Ignore rules and hidden files | Hide dot-files and samples by default; honour `.ignore` / `.plexignore` | **Yes** | S |
| **D-52** | Writing beside the media | Should Portico ever write NFO, artwork or downloaded subtitles next to the files, as an explicit owner option? | **Optional, off by default.** It fits "open data" and helps people leave as easily as they arrive. | M |
| **D-53** | Removing a library | No route exists today (it is planned). What happens to history, My List and playlists that point into it? | **Add it before release**; keep personal state for the D-01 grace period. | L |

## D. Playback and media

| # | Decision | Options | Rec. | Size |
|---|---|---|---|---|
| **D-17** | Conversion outputs | (a) H.264 SDR only for release. (b) Shape the planner now so HEVC and HDR-preserving output can be added. (c) Build HEVC/HDR output and a lossless or E-AC-3 audio target before release. | **(b) for release, (c) soon after.** Fix the 384 kbps AAC downmix of lossless audio first; it is the most audible. | M–L |
| **D-18** | Subtitles | (i) Should a discovered text track be selectable and auto-selectable at once, with import as an invisible cache step? (ii) Render ASS and image subtitles on the device, at least on web? (iii) Detect legacy character sets? | **Yes to (i) and (iii); (ii) on web first**, then Android. Apple needs D-04. | M–L |
| **D-19** | Automatic quality | (a) Manual only. (b) A bandwidth test that picks the *starting* quality; changes still only when asked. (c) A viewer-selected "adapt to my connection" mode. | **(b)**; it respects the principle and fixes remote first-play. | M |
| **D-20** | Built-in limits | (a) Constants. (b) Every limit is a default the owner can raise, shown with current usage. | **(b)**, starting with artwork (128 MiB), guide size, connection cap and conversion session size. | M |
| **D-22** | Timeouts | Wall-clock constants, or tied to "the other side is still making progress" | **Progress-based** for extraction, web fragments, restore checks and downloads. | M |
| **D-54** | FFmpeg strategy | Unpatched upstream (today); carry a small patch set; or allow jellyfin-ffmpeg as an alternative binary | **Stay upstream**, but fix the hardware keyframe and probe issues (B-03.1, B-03.3) and allow the owner to point at another build. | M |
| **D-55** | Capability overrides | Give viewers a short "if something looks or sounds wrong" list, or keep automatic only | **A short list** (force stereo, disable passthrough, disable HDR). It is the escape hatch when detection is wrong. | S–M |

## E. Openness and running the server

| # | Decision | Options | Rec. | Size |
|---|---|---|---|---|
| **D-28** | Outbound webhook | Does "no push, no email" rule out one owner-configured webhook for events and alerts? | **Allow it.** It lets "disk nearly full" or "backup failed" reach someone, and covers most plugin use. | M–L |
| **D-29** | Compatibility layer | (a) None. (b) Subsonic for music. (c) A Jellyfin-compatible subset so Infuse, Kodi, VLC and Plezy work. | **Not for release.** Revisit (b) after D-06; (c) is large and binds you to someone else's API. | L–XL |
| **D-30** | Import, export and trackers | One-time import of watch state from Plex and Jellyfin; export; Trakt-style sync | **Import soon after release** if D-01(c) is chosen; it is the strongest adoption lever in the research, but the second reviewer sizes it as a project. Sync later. | XL |
| **D-31** | Build or borrow | For each owned piece decide keep or replace: tag reading, charset detection, subtitle parsing, image decoding | **Borrow tag reading and charset detection**; keep the rest. | M |
| **D-32** | Outside contributions | Stay closed entirely, or accept translations and documentation | **Accept translations** through a translation tool, with a clear rights grant. | process |
| **D-33** | State folder shape | Strictly one folder, or one folder by default with optional separate destinations for cache, backups and trash | **Optional destinations.** Put the trash beside the media it came from. | M |
| **D-34** | Restart ownership | Rely on a service manager everywhere, or let the server restart itself and offer Restart in the console | **Self-restart**; it fixes Windows and plain Docker at once. | M |
| **D-35** | Relay and multi-server | Design an extension point for an optional paid relay now? Support several signed-in servers at once? | **Leave the seam, build neither yet.** | — |
| **D-56** | Pre-upgrade backup | May a start with migrations to apply copy the database first? | **Yes**; it is the one justified exception to "no library-sized startup work". | L (second reviewer; the first pass said small) |
| **D-57** | Update notices | Ship a default update feed so every server can say a newer version exists | **Yes, opt-out**, and fix the feed shape (B-09.11). | S |

## F. People, privacy and security

| # | Decision | Options | Rec. | Size |
|---|---|---|---|---|
| **D-36** | Confirm the deliberate trade-offs | No lockout; light password rules; LAN HTTP; nothing encrypted at rest; TV code confers the approver's role | **Confirm all five**, with two mitigations that cost the user nothing: keep live secrets out of backups (or sign everyone out on restore), and give Cast and TV-code sessions member-level authority unless the owner asks for more. | S–M |
| **D-37** | Device keys | Build proof of possession (at least on refresh), or correct the document | **Correct the document for release**; consider real binding later. | S |
| **D-38** | First-run setup window | (a) Open to anyone who reaches the server. (b) Open on the LAN with a working Host check; from a public address require a token printed in the log. | **(b)**. Note: "setup protection options" are currently excluded by a recorded decision, so (b) reopens it. At minimum, make the Host check work. | S–M |
| **D-39** | Restrictions and parental controls | One rating engine for accounts and profiles; country tables; per-library "block unrated"; an allow-list kids mode; viewing schedules that pause instead of signing out | **One engine first.** Then the allow-list kids mode, which the identity lane rates the most valuable. | M–L |
| **D-40** | Permission changes | Sign the person out (today), or apply live | **Live**; sign-out only when access is removed entirely. | M |
| **D-58** | Deleting a member; erasure and play history | Add delete. On erasure, keep plays with the name, or blank the name? | **Add delete; blank the name.** | M |
| **D-59** | What viewers are told about owner history | The owner always sees every play, even when a viewer pauses or clears their own | **Say so plainly** in the privacy screen. | S |

## G. Scope of the specialist areas

| # | Decision | Options | Rec. | Size |
|---|---|---|---|---|
| **D-41** | Release promises | Which journeys are complete at release, and which are labelled early: film and TV; music; audiobooks; Live TV and DVR; downloads; Watch Together; Library Channels | **Film, TV and music as complete; the rest labelled early.** | — |
| **D-42** | Live TV | (i) Share one input per channel? (ii) Household DVR, or private per profile? (iii) What line-up size must work: an antenna, or 10,000-channel IPTV? | **(i) share the raw input first; (ii) private by default with an owner "share" option; (iii) decide, because it sets the limits.** | L |
| **D-43** | Downloads | (i) Should a device copy ever disappear because the server tidied up? (ii) Subtitles, artwork and titles with every download? (iii) Resume instead of restart? (iv) Standing rules ("next three unwatched")? | **(i) no; (ii) yes; (iii) yes; (iv) later.** | M–L |
| **D-44** | Music and audiobooks | Audiobooks finish on time remaining, not 95%; local lyrics appear automatically; "Play next" under shuffle goes next; CarPlay and Android Auto before release? | **Yes to the first three. Car: decide; it is the largest missing piece against Plexamp.** | S–L |
| **D-45** | Watch Together | Allow a short bounded wait after a seek or title change? May a member take the host role? | **Yes to a bounded wait**; it removes most sync problems. | M |
| **D-60** | "Mark show watched" | Should it also finish episodes in progress, and have a real Undo? | **Yes to both.** | S–M |
| **D-61** | Rewatching | A rewatch makes the title unwatched until it completes (contract today) | **Keep watched and show progress**, as Plex does, now that play counts exist. Your call; the contract currently says otherwise. | S |

## H. Shipping

| # | Decision | Options | Rec. | Size |
|---|---|---|---|---|
| **D-46** | Compatibility window | The oldest app a server must serve; the oldest server an app must tolerate; what counts as a breaking change | **Write it down before release**, even if short. TV apps update slowly. | process |
| **D-47** | Crash and log reports | None; to the owner's own server; forwardable to Portico by the owner | **To the owner's server**, forwardable by choice. | M |
| **D-48** | Apple distribution and the GPL | Needs a deliberate answer on the licence grant for code you own and a review of dependency licences | **Get advice before submission.** I cannot give legal advice. | — |
| **D-49** | Shared packages | Checked copies (today); published versioned packages; or one repository for public clients | **Keep copies, pin the source revision.** | S |
| **D-24** | Appearance | Fixed dark theme only; add a light or high-contrast theme; bounded styling | **Add high-contrast and honour system text size** before any theme work. | M |
| **D-25** | Remembered choices | Library tab and sort; audio and subtitle per show | **Yes to both.** | S–M |
| **D-26** | Spoiler protection and native view modes | Build, or leave out | **Spoiler protection yes** (optional); view modes later. | M |
| **D-27** | "This device" settings | Make them truly per device, or relabel | **Relabel now** ("all phones", "all TVs"). | S |

---

# Part 4 — Missing features

## What the second review changed

The first pass listed 709 features that competitors have and Portico supposedly lacks. The second reviewer looked for each one in the server, the API, `client-core`, the web app, the native apps and the plans (`verification/G1` to `G4`).

| Verdict | Count | Meaning |
|---|---|---|
| **Partly present** | 436 | Something real exists: server only, one client only, built but not exposed, or a narrower version |
| **Absent** | 200 | As claimed |
| **Planned** | 51 | Absent, but already in the Backlog, a spec or a contract |
| **Excluded by decision** | 21 | A recorded decision currently rules it out |
| **Present** | 1 | The claim was wrong (kids' restrictions already apply to audiobooks) |

Two things follow.

**Most of the gap is unfinished, not missing.** Six in ten claimed gaps have a real foundation in the code already. The second reviewer also found about 25 claims with a wrong component, where part of the "missing" feature exists: local trailers, date-named episodes, disc-folder parsing, cross-library Saved collections, owner check-and-repair APIs, XMLTV for HDHomeRun on the server, next and previous in a Watch Together queue, subtitle search and download in the web player, access schedules, an all-devices API, an E-AC-3 5.1 conversion path, and automatic download of the next episode with removal of the watched one. These should not start new subsystems.

**The first pass's sizes were optimistic.** Where an auditor wrote "small", the second reviewer usually wrote "L" (a week or more), because it counted publication, permissions, recovery, every client's UI and device acceptance. That is the honest cost of the consistency goal: a feature is not delivered until it is on eight surfaces. Sizes below are the second reviewer's.

## Against real demand

Lane 24 ranked 120 feature requests by evidence, mostly votes on Jellyfin's own request tracker. Of Jellyfin's 25 most-voted requests, Portico's source already implements about 14 and partly implements 5:

- **Already built** (subject to device acceptance): offline downloads on phones, remove from Continue Watching, a watchlist, lazy-loading library views, two-factor sign-in, pre-converted versions, audiobooks, watch history, gapless music, downloading converted files, an Apple TV app, stopping someone's stream, IPTV channel groups, QR sign-in for TVs.
- **Partly built:** owner-chosen single sign-on (Apple and Google only), smart playlists (saved views exist; not playable as playlists), trailers (local only), personal ratings (not on shows or albums), hardware-conversion self-test.
- **Not built:** temporary share links (excluded by decision), podcasts, a folder view, missing-episode display, a MySQL back end (deliberately not), a VIDAA TV app.

This is a strong position that the product does not yet advertise. Lane 23's survey of Plex, Infuse and Emby reached the same conclusion from the other side: the differentiators people pay Plex for (hardware conversion, downloads, skip markers, Watch Together, remote access) are all free in Portico's design.

## The shortlist

About forty capabilities, chosen for demand and fit. Duplicate claims across documents are merged under the second reviewer's canonical name; the IDs lead back to the detail. **Today** is the second reviewer's verdict. **Fit** is against your philosophy: *F* freedom and choice, *P* polish and ease, *C* consistency across platforms.

### Getting people in and letting tools connect

| Capability | Who has it | Today | Size | Fit | My view |
|---|---|---|---|---|---|
| **Import watch state from Plex and Jellyfin** (F-24.1, F-23.1, F-12.4, F-08.5) | Third-party tools for both; no server does it well | Absent | XL | F, P | The strongest adoption lever in the research. Needs D-01 first. |
| **Scoped access tokens for scripts and tools** (F-08.1, F-01.8) | Jellyfin, Plex | Partly: storage exists, creation disabled | M–L | F | Do it. Everything else in this group depends on it. |
| **Outbound webhook** (F-08.2) | Plex, Jellyfin plugin | Absent | M–L | F | Do it; covers most plugin uses and reaches an owner who is not looking at the console. |
| **"These paths changed" scan call** (F-23.5, F-05.2) | Plex, Jellyfin, used by Sonarr/Radarr | Partly | L | F | High value for the homelab audience. |
| **Real-time folder monitoring** (F-05.1) | Jellyfin, Plex | Partly | L | P | After the scan call. |
| **Tracker sync: Trakt, Simkl, Last.fm, ListenBrainz** (F-12.1, F-10.6) | Plezy, Infuse, Jellyfin plugin, Navidrome | Absent | L each | F | Later; per profile, by consent. |
| **Requests through Seerr** (F-23.6) | Streamyfin, Wholphin | Absent | L | F | Later; keep acquisition outside Portico. |
| **Owner-chosen single sign-on (OIDC)** (F-24.4, F-01.9) | Jellyfin plugin; 1,198 votes | Excluded for first release | L–XL | F | Your recorded decision stands; revisit after release. |

### Library and metadata

| Capability | Who has it | Today | Size | Fit | My view |
|---|---|---|---|---|---|
| **Versions and editions as one title** (F-24.2, F-05.5, F-02.1) | Plex, Jellyfin | Partly: manual association exists | L–XL | P | Part of D-01. |
| **Watch state that survives replace and re-add** (F-07.1) | Jellyfin, Plex | Partly | L–XL | P | Part of D-01. |
| **A "needs attention" list for unmatched titles** (F-06.3) | Plex, Jellyfin | Partly | L | P | Do it; it makes conservative matching livable. |
| **Folder-name and IMDb id hints** (F-06.1) | Plex, Jellyfin | Partly | M–L | P | Do it with D-03. |
| **Ignore rules and hidden-file handling** (F-05.4) | Jellyfin, Plex | Absent | M | P | Do it; small and visible. |
| **Remove a library** (F-07.14, F-18.1) | Everyone | Planned | L | P | Needed before release. |
| **Automatic collections** (F-06.5) | Plex, Jellyfin | Partly | L | P | Worth having. |
| **Smart playlists from saved views** (F-24.6) | Plex, Navidrome; 588 votes | Partly | L | F, P | A natural extension of what exists. |
| **Search that finds original titles, artists and years** (F-07.5, F-12.10) | Plex | Absent / partly | L | P | Decide scope under D-09. |
| **Full TV artwork: season posters, logos, cast photos** (F-06.7) | Plex, Jellyfin | Partly | L | P | Visible polish gap on every show page. |
| **Missing and upcoming episodes** (F-06.19) | Jellyfin, Plex; 344 votes | Partly | L–XL | — | Your call; not obviously a personal-library job. |

### Playback

| Capability | Who has it | Today | Size | Fit | My view |
|---|---|---|---|---|---|
| **Styled (ASS) and image subtitles drawn on the device** (F-04.1, F-17.1, F-20.3) | Jellyfin web and Android, Plezy, Infuse | Planned (the spec promises it) | L web and Android; XL everywhere | P, C | The biggest hidden cause of full conversions. Web first. |
| **HEVC and HDR-preserving conversion** (F-02.2, F-03.1) | Jellyfin, Plex | Absent | L first path; XL full | P | After the capability document is fixed. |
| **Multichannel and lossless audio targets** (F-20.4) | Jellyfin, Plex | Partly: an E-AC-3 5.1 path exists | L | P | Check on a device how often AAC is still the result. |
| **FFmpeg audio decoding on Android** (F-20.2) | Every Jellyfin Android client | Absent | L | P | Removes most audio conversions on Android. |
| **Starting quality from a bandwidth test** (F-02.4) | Jellyfin, Plex | Partly: specified, half built, never called | L | P | D-19. |
| **Remembered audio and subtitle choice** (F-02.6, F-04.5) | Plex, Plezy, Jellyfin | Partly | L | P | Spec already promises it. |
| **Legacy subtitle character sets** (F-04.3) | Jellyfin, mpv | Partly | M–L | P | Do it. |
| **Intro detection by repeated audio across episodes** (F-04.13) | Plex, the Jellyfin intro-skipper plugin | Absent | XL | P | A real differentiator once done; expensive. |
| **Scrub previews and chapter marks on native** (F-04.17, F-20.x) | Plezy, Swiftfin, Jellyfin Android TV | Partly: server and web only | L | C | A consistency debt. |
| **Refresh-rate matching on Android TV** (F-20.5) | Plezy, Wholphin, Jellyfin Android TV | Absent | L | C | Exists on Apple TV only today. |
| **External player hand-off** (F-24.11, F-20.18) | Plezy, Streamyfin, Jellyfin Android | Absent | L–XL | F | The pragmatic answer for power users if D-04 stays on platform players. |

### Apps

| Capability | Who has it | Today | Size | Fit | My view |
|---|---|---|---|---|---|
| **Open screens that update themselves** (F-16.1, F-08.6) | Jellyfin, Plex | Partly | L | P | D-08. |
| **Spoiler protection for unwatched episodes** (F-21.1, F-18.49) | Plezy, Infuse | Planned (the contract requires it) | L | P | Do it. |
| **Crash and log reports to the owner's server** (F-19.1) | Jellyfin Android TV | Partly: storage exists | L | P | D-47. |
| **CarPlay and Android Auto** (F-10.39) | Plexamp, Finamp, Jellyfin Android | Absent | L each | P, C | The largest gap against "Plexamp-grade". D-44. |
| **Search inside a library** (F-18.2) | Jellyfin, Plex | Partly | M–L | P | The contract requires it. |
| **A desktop app** (F-24.12) | Plezy, Jellyfin | Planned | L–XL | C | Already on your list. |

### Specialist areas

| Capability | Who has it | Today | Size | Fit | My view |
|---|---|---|---|---|---|
| **Local lyrics that simply appear** (F-10.1) | Jellyfin, Navidrome, Finamp | Partly: reader exists, nothing publishes | M | P | Do it. |
| **Radio and mixes** (F-10.4, F-23.22) | Plexamp, Finamp | Planned; a server instant mix exists unused | L | P | Offer what exists first. |
| **Audiobook bookmarks and a metadata source** (F-10.29 to F-10.32) | Audiobookshelf | Planned / absent | M–L each | P | If audiobooks are a release promise. |
| **Tolerant guide import with diagnostics** (F-11.4) | Jellyfin, Plex | Absent | L | P | D-02. |
| **Recordings that survive a hiccup** (F-11.8) | Jellyfin, Channels | Partly | L | P | If DVR is a release promise. |
| **Downloads that resume, with subtitles and artwork** (F-14.2, F-14.4) | Plezy, Streamyfin, Plex | Partly / planned | L each | P, C | D-43. |
| **Watch Together that waits after a seek, and advances a queue** (F-13.1, F-13.4) | Jellyfin SyncPlay, Plezy | Partly | L each | P | D-45. |
| **An allow-list kids mode** (F-01.4) | Jellyfin | Absent | M–L | P | The simplest strong parental control. |

## Things currently ruled out by a recorded decision

The second reviewer found 21 claimed gaps that an existing decision excludes. Listed so that nothing gets built by accident, and so you can reopen any of them deliberately:

- **Public or guest share links** (F-01.21, F-08.11, F-10.43, F-18.57). 581 votes on Jellyfin's tracker.
- **Sub-path hosting** (F-08.17, F-09.4). Consistent with my D-07 recommendation.
- **Owner-chosen single sign-on** (F-01.9, F-15.17), for the first release.
- **A light theme and accent colours** (F-16.4, F-18.45). My D-24 suggests high contrast and system text size instead.
- **A stricter sign-in mode, setup protection options, and a passphrase for external backups** (F-15.4, F-15.6, F-15.9). Note that my recommendation on D-38 (protect first-run setup from public addresses) would reopen F-15.6; that is yours to weigh.
- **A limit on signed-in devices per member** (F-01.14); **a hero when nothing is in progress** (F-12.21); **merged Continue Watching across servers** (F-12.25); **household-wide "Play on"** (F-13.18); **presence outside the app** (F-13.19); **download storage location and export** (F-14.16); **Picture in Picture on TVs** (F-21.32); **ad-supported content and rentals** (F-23.59).

## What competitors' users dislike, as openings

From lanes 23 and 24, with sources in those documents:

- **Plex:** the mandatory cloud account, paying for remote streaming, advertising and streaming clutter in the app, and a privacy incident with shared viewing summaries. Portico's design answers all four. The matching obligation is to be plain about what the owner can see (D-59).
- **Jellyfin:** inconsistent clients, remote access left to the owner, hardware conversion that is hard to set up, weak music and recommendations, and slow large libraries. Portico's design answers these too, on paper. Proof on devices and at scale is what turns it into a claim.
- **Infuse:** shows that many people want a great player and no server at all. That is a different product, but it is why external player hand-off and a compatibility layer keep appearing in the research.

---

# Part 5 — Bugs

Listed separately from the design discussion, as you asked. The complete list of all 588 claims with both reviewers' verdicts is in **`BUG-REGISTER.md`** in this folder; this part covers what needs your attention.

## The numbers

| After the second review | Count | Of which |
|---|---|---|
| **Critical** | 1 | Confirmed (narrower): Windows local libraries |
| **High** | 69 | 26 confirmed exactly, 38 confirmed but narrower, 5 need a device or runtime check |
| **Medium** | 298 | 109 confirmed, 164 narrower, 25 unsettled |
| **Low** | 57 | including the 2 claims refuted outright |
| Low, not re-checked | 163 | left as the first auditor wrote them |

**By kind**, for the 425 re-checked: about 233 genuine defects, 86 latent design risks (nothing breaks today), 44 deliberate trade-offs, 42 unfinished features, 15 missing features that were mislabelled as bugs, and 5 documentation errors. Only eight are already in `Backlog.md`.

**How the first pass held up.** It was reliable about *whether* a problem exists (2 of 425 wrong) and tended to overstate *how far it reaches* (about half narrower). Treat any first-pass severity that was not re-checked as an upper bound.

**What "confirmed" means here.** Two different models read the same code and reached the same conclusion. It does not mean the bug was reproduced. About one claim in ten can only be settled on a device or a running server.

## Critical and High, after review

Duplicates found by different lanes are merged and shown as `A = B`. "Deliberate trade-off" means the behaviour is written policy; it stays on this list because the impact is high and you may want to revisit the policy.

**Running the server, install and release**

| ID | What goes wrong | Second review | Class | Size |
|---|---|---|---|---|
| B-09.1 | Restore, and the on-demand integrity check, run under a 10-second request budget | Confirmed, narrower | Defect | M |
| B-09.2 | After a restore the server exits and nothing may start it again | Confirmed, narrower | Defect | M |
| B-09.3 = B-25.20 | Linux packages: a fresh install is not started, and an upgrade leaves the service stopped (deb) or stopped and disabled (rpm) | Confirmed, narrower | Defect | S |
| B-09.4 | Windows: Scheduled Task defaults will stop the server after three days and on battery | Unsettled — needs runtime | Defect | S |
| B-09.13 | Deleted media is moved into the state folder and never leaves on its own | Confirmed, narrower | Defect | M |
| B-09.35 | Moving only the database file does not work, and a mismatched restore is not rolled back | Confirmed, narrower | Defect | M |
| B-25.2 | Release and Makefile vet still reject the maintained driver | Unsettled — needs runtime | Defect | S |
| B-25.3 | Windows portable ZIP output is relative to its payload directory | Confirmed | Defect | S |
| B-25.4 | Docker release job omits its required FFmpeg input | Confirmed | Defect | M |
| B-25.19 | Manual release can test caller branch rather than requested tag | Confirmed | Defect | S |

**Libraries, scanning and metadata**

| ID | What goes wrong | Second review | Class | Size |
|---|---|---|---|---|
| **B-05.28** (Critical) | On Windows the server cannot add a library | Confirmed, narrower | Defect | M |
| B-05.1 | Scan speed has a hard ceiling of about three steps per second per folder source | Confirmed, narrower | Defect | S |
| B-05.2 | Any folder changing during a scan fails the whole scan — and then nothing gets analysed | Confirmed, narrower | Deliberate trade-off | M |
| B-05.3 | A file that changes after it was first seen stays stale — and probably unplayable — for up to a week | Confirmed, narrower | Defect | M |
| B-05.4 | One unreadable sub-folder stops every scan of that source | Confirmed | Defect | M |
| B-05.8 | A second file for a movie is a second movie; an upgraded file loses the title's personal state | Confirmed, narrower | Unfinished feature | L |
| B-05.10 | Flat show folders, and the show folder's year, are only understood inside a Season N folder | Confirmed | Defect | M |
| B-05.11 | The folder-must-match rule rejects correctly organised shows | Confirmed, narrower | Defect | M |
| B-05.20 | Scan bookkeeping is never pruned | Confirmed, narrower | Design risk | M |
| B-06.3 | The "is this slot already taken?" check for provider posters never matches | Confirmed | Defect | S |
| B-06.4 = B-10.2 | Local artwork stops being accepted after 128 MB in total | Confirmed, narrower | Defect | M |
| B-06.24 | Adding one episode re-fetches every episode of the show; the freshness rules do not cover episodes | Confirmed | Defect | M |
| B-07.1 | One permanently failing entity stops all second-tier derived workers | Confirmed | Defect | M |
| B-07.2 | Raising a derivation version takes browsing or search offline for the whole rebuild | Confirmed, narrower | Deliberate trade-off | L |

**Playback, conversion and subtitles**

| ID | What goes wrong | Second review | Class | Size |
|---|---|---|---|---|
| B-02.1 | Native devices are told HEVC may travel in MPEG-TS HLS | Confirmed, narrower | Defect | M |
| B-02.2 = B-20.1 | Passthrough codecs are always rejected for v1-publishing devices | Confirmed, narrower | Defect | M |
| B-02.4 = B-20.2 | "The device tone-maps HDR itself" is never sent | Confirmed | Defect | M |
| B-02.6 = B-17.1 | The player's Quality menu, version choice and audio re-plan never reach the server | Confirmed, narrower | Defect | M |
| B-02.7 | A start without an Idempotency-Key takes the legacy path and skips caps | Confirmed, narrower | Defect | M |
| B-02.12 | Turning on a burned-in subtitle re-plans without the session's ceilings | Confirmed | Defect | M |
| B-03.1 | NVIDIA, Quick Sync and AMD encodes probably do not cut on the six-second grid | Confirmed, narrower | Defect | M |
| B-03.4 | Software HDR conversions fail inside the Linux sandbox when Vulkan (or OpenCL) was detected | Confirmed, narrower | Defect | M |
| B-04.1 | Sidecar subtitle files are never imported automatically, so they are never auto-selected | Confirmed, narrower | Unfinished feature | M |
| B-04.4 | The 90-second budget covers up to eight full reads of the file | Confirmed, narrower | Defect | M |
| B-04.31 | On a Windows server no embedded subtitle can be used | Confirmed, narrower | Defect | S |
| B-17.4 | Slow delivery is turned into failure: the stall watchdog and a 15-second fragment limit abort downloads that would have finished | Confirmed, narrower | Defect | M |
| B-20.3 | App-drawn subtitles are refused when the server is reached over plain HTTP | Confirmed | Defect | S |
| B-20.4 | On Android a video keeps playing after the app is left when Picture in Picture does not take over | Unsettled — needs runtime | Defect | M |

**Identity, security and connectivity**

| ID | What goes wrong | Second review | Class | Size |
|---|---|---|---|---|
| B-01.2 = B-15.1 | "Remote sign-in: owner only / off" is not applied on the sign-in routes the apps use | Confirmed | Defect | M |
| B-15.12 | A stolen Hosted signing key vouches for any Portico Account until each server hears otherwise | Confirmed, narrower | Design risk | L |
| B-22.1 | Secure LAN direct names invalidate signed route documents | Confirmed | Defect | M |
| B-22.2 | Multiple secure LAN addresses collapse to duplicate signed origins | Confirmed | Defect | S/M |
| B-22.3 | Plain-HTTP tab-local locks cannot protect a shared rotating credential | Confirmed | Defect | M |
| B-22.4 | A temporary protected-store read failure is classified as sign-out | Confirmed | Defect | M |
| B-22.18 | Auxiliary native streams bypass route selection and verification | Confirmed | Defect | M |
| B-22.19 | Vega rejects redirects after credential transmission can occur | Confirmed, narrower | Defect | M/L |

**Live TV, music and audiobooks**

| ID | What goes wrong | Second review | Class | Size |
|---|---|---|---|---|
| B-10.1 | An audiobook file is marked finished, and its resume point reset to zero, at 95% | Confirmed | Defect | S |
| B-11.1 | Every guide refresh retunes every live viewer of that source | Confirmed | Defect | M |
| B-11.2 | A standard XMLTV file with a DOCTYPE line is rejected | Confirmed | Defect | S |
| B-11.8 | HDHomeRun sources cannot have a guide, so they cannot record | Confirmed, narrower | Unfinished feature | M |
| B-11.10 | A few minutes of padding overlap fails an entire recording | Confirmed, narrower | Deliberate trade-off | M |
| B-11.11 | The first upstream error ends a recording for good | Confirmed, narrower | Unfinished feature | L |
| B-11.13 | A one-time recording is lost if the provider corrects the start time | Confirmed, narrower | Deliberate trade-off | M |
| B-11.14 | HLS sources with several quality variants: recordings take all of them, live takes the first listed | Unsettled — needs runtime | Design risk | M |

**Downloads, Watch Together and casting**

| ID | What goes wrong | Second review | Class | Size |
|---|---|---|---|---|
| B-13.1 | Heartbeats and readiness reports invalidate the host's commands | Confirmed | Defect | M |
| B-13.2 | A slow seek is restarted every three seconds and never finishes | Confirmed, narrower | Defect | M |
| B-13.7 | Casting stops when the phone disconnects | Confirmed, narrower | Defect | S |
| B-13.14 | The Cast TV holds a full session with the pairing person's role | Confirmed | Defect | M/L |
| B-14.1 | Downloads delete themselves from the device after the server's retention period | Confirmed | Defect | M |
| B-14.2 | "Continue" and "Try again" restart from byte zero on both platforms | Confirmed, narrower | Defect | M |
| B-14.3 | Any pause longer than ten minutes inside the OS downloader fails the transfer | Confirmed, narrower | Defect | M |
| B-14.4 | Four of the six offered quality rungs can never be produced | Confirmed | Unfinished feature | M |
| B-14.6 | A download that finishes while the app is not running online cannot be played offline | Confirmed, narrower | Defect | M |

**Web and native apps**

| ID | What goes wrong | Second review | Class | Size |
|---|---|---|---|---|
| B-19.18 | No scene manifest in the Apple apps | Unsettled — needs runtime | Unfinished feature | M |


**Plain-language notes on the tersely titled ones**

- **B-22.1 / B-22.2:** when a claimed server offers HTTPS on the LAN, the address list Hosted signs for it contains entries the client's own check rejects, so the client discards the *whole* list, including valid remote addresses. A new device signing in through a Portico Account cannot connect. A server with two LAN addresses hits a second, independent version of the same failure.
- **B-22.3:** on a plain-HTTP LAN server, two browser tabs renewing their sign-in at the same moment can each invalidate the other, signing both out.
- **B-22.4:** if the device's secure storage is briefly unreadable, the client concludes the sign-in was replaced and deletes it. This is a candidate cause for the unexplained Apple sign-outs in the Backlog, though nobody has shown that.
- **B-22.18:** the Watch Together and receiver event streams on native open their own connections and skip the route selection and server-identity check everything else uses.
- **B-02.1:** the capability translation tells the server Apple devices can take HEVC in an MPEG-TS stream, and the server prefers that route. Whether AVPlayer then fails needs one device test.
- **B-06.3:** a routine metadata refresh can replace a poster the owner chose from local artwork, and re-downloads artwork it already has.
- **B-25.4:** the Docker release job cannot build from a clean tag because it needs a generated FFmpeg bundle that no earlier step hands to it.

## Small and safe: fix without waiting for any decision

Thirteen High items (fourteen IDs) the second reviewer sized at hours, none of which depends on a decision in Part 3. Two of them, B-09.4 and B-25.2, are probable and need a quick check first:

| ID | Fix |
|---|---|
| B-05.1 | Remove the fixed cadence floor so a scan runs as fast as the storage allows |
| B-06.3 | Compare the artwork slot id with the right type |
| B-09.3 = B-25.20 | Start and restart the service in the Linux package scripts |
| B-09.4 | Set the Windows task's time limit and battery options (needs a Windows check) |
| B-10.1 | Do not reset an audiobook's resume point at 95% |
| B-11.2 | Accept the standard XMLTV `DOCTYPE` line while still refusing entity expansion |
| B-13.7 | Keep Cast playing when the last sender disconnects |
| B-20.3 | Allow the subtitle document over LAN HTTP, as the rest of the app does |
| B-04.31 | Enable embedded-subtitle extraction on Windows (only matters if Windows ships) |
| B-22.2 | De-duplicate signed LAN addresses |
| B-25.3 | Fix the Windows ZIP output path |
| B-25.19 | Make manual release CI test the tag it packages |
| B-25.2 | Align the release vet step with the maintained SQLite driver (confirm it currently fails) |

## New problems the second review found

| ID | Severity | What |
|---|---|---|
| N-V01.1 | High | When a server learns a Hosted signing key was revoked, it rejects new sign-ins from that key but does not end sessions already issued under it |
| N-V02.1 | High | Playback started from a queue (web), and the next item in a queue (native), can skip the "this route failed before, try another" memory, so a file that failed one way is tried the same way again |
| N-V09.1 | Medium | The web player path that ships cannot report an engine failure, so the advertised automatic fallback cannot happen. Fix together with B-17.2, or enabling fallback will start discarding queues |
| N-V03.1 | Medium | Mismatched read limits disable even very small image-subtitle sidecar files |
| N-V08.1 | Medium | Downloading a very large show or playlist silently stops enrolling items after fifty pages (about 5,100) |
| N-V11.1 | Medium | On the legacy playback path, two simultaneous starts can both pass a member's stream limit |
| N-V11.2 | Low–Medium | Analysis is written to stop for playback, contradicting "never paused"; on a busy source it may starve |

## High claims that need a device or a running server

| ID | Claim | The cheapest check |
|---|---|---|
| B-20.4 | Android video keeps playing after leaving the app | Press Home during playback on one Android TV and one phone |
| B-09.4 | Windows task stops after three days or on battery | Inspect the installed task on a Windows machine |
| B-11.14 | Multi-variant HLS sources: recordings capture every variant | Record from one multi-variant IPTV source |
| B-19.18 | Newer Apple SDKs may require a scene manifest | Build against the current SDK and read the warnings |
| B-25.2 | Release vet rejects the SQLite driver | Run the release vet step |
| B-02.1, B-22.19 | (confirmed in source; impact unproven) | One HEVC MKV on an Apple TV; one redirect test on a Vega device |

Also unsettled at Medium: B-20.5, B-20.7, B-20.9, B-20.19 (native player behaviours), B-05.18 and B-05.19 (file identity and timestamps on network mounts), B-13.3 and B-14.12.

## High-impact behaviours that are written policy

These four are on the High list but classed as deliberate. Each maps to a decision in Part 3.

| ID | Policy | Decision |
|---|---|---|
| B-05.2 | A scan that sees a folder change fails conservatively | D-02 |
| B-07.2 | Readers wait while a derived table rebuilds | D-14 |
| B-11.10 | Recording padding that overlaps another recording fails the recording | D-42 |
| B-11.13 | A recording follows programme identity, so a programme with no stable id that moves is lost | D-42 |

Elsewhere, the second reviewer reclassified these first-pass "bugs" as decisions working as intended: rewatching clears the watched mark (B-02.8, required by the contract; D-61); a restore brings back revoked sessions (B-09.19; D-36); bulk delete has no confirmation because it goes to a recoverable trash (B-18.13); weak luminance-based skip markers are shown but cannot auto-skip (B-04.21); and the security trade-offs in Bet 11.

## What the second review got the first pass to drop or narrow

- **Refuted:** B-03.7 (a keyframe burst when joining a live stream; the pinned FFmpeg rebases time, so it does not happen) and B-18.8 (owners signed in with a Portico Account being denied metadata tools; sign-in now issues local authority).
- **Substantially narrowed:** a renamed or moved film does **not** lose its watch state (B-12.18); HDHomeRun guides **do** work through explicit mapping, what is missing is the setup screen (B-11.8); a failed full conversion does not retry automatically (part of B-02.16); remote-source GPU failures apply only to one narrow case (B-03.5).
- **Lowered from High to Medium:** the parsing-strictness claims in Live TV (B-11.3, .4, .5, .7), several metadata claims (B-06.1, .2, .7, .11, .21), "mark show watched" (B-12.1), the connection cap (B-09.7), several release-signing gaps (B-25.6, .8, .9, .10, .12), and a group of web and connectivity claims (B-17.2, B-17.3, B-18.1, B-18.5, B-18.13, B-21.5, B-22.5, B-22.6, B-22.20, B-08.1).
- **Raised to High:** passthrough refused (B-02.2), tone-mapping never declared (B-02.4), the legacy start path skipping limits (B-02.7), burn-in ignoring ceilings (B-02.12), the artwork slot check (B-06.3), unpruned scan bookkeeping (B-05.20), foreign-database restore (B-09.35), the Linux upgrade (B-25.20) and the Cast session's authority (B-13.14).

## Medium: the ones I would pull forward

Of the 298, these are the ones a new user is most likely to hit, or that are cheap relative to their effect. All are in the register with sizes.

| ID | What | Why it stands out |
|---|---|---|
| B-07.6 | Titles such as *M\*A\*S\*H*, *S.W.A.T.* and one-letter titles cannot be found by search | Frozen-at-release detail (D-09) |
| B-06.1 / B-05.7 | Titles containing a year (*1917*, *Blade Runner 2049*) parse wrongly | Common, visible on day one |
| B-06.2 | Yearless titles never auto-match, with no list of what was left | D-15 |
| B-05.6 / B-05.16 | Hidden files, samples and bare trailers become titles | Clutter in every imported library |
| B-04.9 | Subtitles in older character sets show as garbage | Very common outside English |
| B-18.5 = F-07.14 | A library cannot be removed | D-53 |
| B-12.1 | "Mark show watched" skips episodes in progress | D-60 |
| B-10.3 / B-10.4 | "Play next" and edits misbehave under shuffle | D-44 |
| B-16.8 = B-22.8 | Most browser requests have no deadline | Hangs instead of errors |
| B-16.3 | Only the main bundle is cached as immutable; other hashed files are re-validated every load | Easy win on slow links |
| B-08.1 = B-15.8 | The web app's writes are refused behind a TLS reverse proxy | Release blocker if proxies are supported (D-07) |
| B-09.8 | Plain HTTP refused to Tailscale and CGNAT addresses | D-07 |
| B-19.1 | Top Shelf, Watch Next, Siri and universal links are dropped on a cold start | Breaks every system integration |
| B-17.2 / B-17.3 | Changing quality discards the queue; a pause from the system or a media key is undone | Daily annoyances |
| B-14.14 | Signed-out and other-profile downloads are orphaned on the device | Storage leak |

## Settings that save but do nothing (D-23)

Remove from the console or wire up: the conversion temporary folder and background-session limit (B-03.10 / B-09.9); several Live TV owner settings (B-11.9); device approval (B-01.4); the "secure connections" choice the server rejects (B-09.10); "remember search history" (B-18.3); the "started" percentage (B-12.11); poster size on Apple (Backlog); and the API-key screen (B-01.3) unless D-06 says finish it.

## Questions other products' histories pose

A separate lane took 44 questions derived from Jellyfin's and Plex's recurring bug classes and answered each from Portico's code (`verification/V11-…md`): Portico handles 2, is exposed on 19, partly exposed on 19, and 3 need a runtime check. The exposures it rates High are already covered above (capability translation, subtitle timing after a seek, offline transfer resume, Watch Together recovery, Cast reconnect, live TV failure isolation, connection-state truthfulness after an address change, and audiobook progress). Two reassurances worth stating: an unmounted or replaced library root does **not** erase personal state, and malformed local metadata cannot exhaust the server.

---

# Part 6 — What this audit did not establish

- **Nothing was run.** No Portico server, client, build or test suite was executed. Every finding is from reading source. The second reviewer ran a handful of copied pure functions (filename parsing, a file-overwrite experiment) in a scratch directory; those show the behaviour of the copy, not the product.
- **No device behaviour.** Nothing here says how Portico looks, sounds or performs on an Apple TV, a Fire TV Stick, an Android phone or a Vega device. The native ports are a week old and have no physical acceptance.
- **No performance or capacity.** The scale design is assessed as a design. The million-item and hundred-stream targets remain unestablished, as your own notes already say.
- **Closed products are from public information.** Plex, Infuse, Emby, Plexamp and Channels were compared from documentation and reviews, and some first-pass statements about them were from the auditors' memory, not fetched sources. The comparison with Jellyfin, Plezy, Streamyfin, Swiftfin, Wholphin, Finamp, Navidrome and Audiobookshelf is from their code as cloned on 11 October 2026.
- **Coverage gaps the auditors named.** Hosted's own Go code was read only where it shapes the server; the Roku client was out of scope; the Cast receiver, the Vega shims and parts of the native listening engines were skimmed; about 430 non-catalogue database tables and most triggers were not read line by line.
- **163 Low-severity bug claims were never re-checked**, and the 1,250 architectural differences were clustered and spot-checked but not individually verified by a second model. Where two auditors contradicted each other I have said so; the second reviewer lists ten such contradictions (`verification/T-independent-themes.md`, section 5). I resolved two by reading code (plain HTTP is refused to public peers at the socket; live TV input is per viewer), and the verification lanes settled two more (API keys cannot authenticate; a same-disk rename keeps watch state). The other six are open.
- **Two models are not a proof.** "Confirmed" means two independent readings agree. Both can be wrong in the same way.
- **I am not able to give legal advice** on the App Store and GPL question, provider terms of use, or what a paid relay would mean for the metadata licences. Those need a person qualified to answer.

## What is in this folder

| File | What it is |
|---|---|
| `00-FINAL-REPORT.md` | This document |
| `BUG-REGISTER.md` | All 588 bug claims with both reviewers' verdicts, class, severity and size |
| `01-…md` to `25-…md` | The 25 domain documents: full walkthroughs of how Portico and the others work, every A-, F- and B- entry with code references, doc-versus-code disagreements, and open questions |
| `verification/V01…V10-…md` | The second reviewer's independent trace of each re-checked bug claim, with fix directions |
| `verification/V11-…md` | Answers, from Portico's code, to 73 questions and lessons drawn from other products' histories |
| `verification/G1…G4-…md` | The second reviewer's check of all 709 "missing feature" claims: what actually exists, where, and what is left |
| `verification/T-independent-themes.md` | The second reviewer's independent grouping of every architectural difference into 15 themes, with full member lists, 16 recurring patterns, a consolidated "Portico ahead" register and the contradictions between documents |

Nothing in Portico's repositories was changed. These files are new and uncommitted in `portico-internal`.

## Handing this back

When you have worked through Part 3, the most useful reply is a list of decision numbers with your choice (for example "D-01 c, D-02 b, D-03 b, D-07: proxy yes, sub-path no, Windows out"), plus any of the "small and safe" fixes you want held back. From that I can turn the answers into an ordered implementation plan, with the bug IDs each decision unblocks.
