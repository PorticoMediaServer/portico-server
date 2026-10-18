# 24 — Community demand and architectural lessons

## Scope and sources

This is a read-only comparative audit dated 11 October 2026. Its central finding is that Portico already addresses much of Jellyfin's highest-voted demand in its architecture and source. The largest adoption risks are unfinished acceptance on household devices, migration friction, version identity, remote connectivity, and the long-term promises made at release. A source-backed feature is not yet a demonstrated product advantage.

The shared brief was read in full. All eight Portico architecture documents and `Backlog.md` were read, followed by selected saved-state, social-playback and product-decision contracts. Source review covered catalogue personal state and search, playlist data shapes, inventory and missing reconciliation, database write scheduling, delivery decisions, subtitle timing, markers, backup scheduling, downloads and shared client presentation. Comparison review covered Jellyfin's local release/blog history, version file and scanner proposal; Plezy's README, backend capability/interface code, website FAQ, relay and release history; and Navidrome/Audiobookshelf feature inventories. This researcher did not run servers, builds, tests, browser automation, MCP tools or requests to Portico infrastructure. No subagents were used. Host: `ehler-homelab-mini-1`, the personal Ubuntu workspace. The requested context refresh failed because its lock path is read-only in this sandbox; existing project and verification guidance was accessible.

### Public sampling and its limits

- **Jellyfin Fider:** public HTTP responses from [most wanted](https://features.jellyfin.org/?view=most-wanted&limit=500) and the [posts API](https://features.jellyfin.org/api/v1/posts?view=most-wanted&limit=500), plus completed/planned/started/declined views. Captured 500 active ranked requests, 301 completed records, 43 planned, 39 started and 440 declined. The active top 120 are individually linked below with exact snapshot votes, creation dates and tracker status. The active ranking excludes completed requests, so a separate completion ledger is essential.
- **GitHub:** unauthenticated REST search of open issues, ordered separately by comments and total reactions, 30 records per order for `jellyfin/jellyfin`, `jellyfin-web`, `jellyfin-androidtv`, `Swiftfin` and `edde746/plezy`. This is ten overlapping ranked samples, not a census of every issue. Total open issue search counts were respectively 425, 301, 295, 84 and 235. Reactions include multiple emoji types; they are not all votes in favour. Comments include maintainer/bot activity and cannot be interpreted as distinct affected users.
- **Plezy closed issues:** a direct REST page of 100 closed records ordered by comments, containing 94 issues after excluding pull requests. Sorting that subset by reactions is explicitly a **subset ranking**. The dedicated search initially hit a rate limit; a later successful request captured the global top 30 closed issues by reactions from **1,736** closed issues. Both sample boundaries are retained in the appendix.
- **Releases:** 100 Jellyfin and 85 Plezy release objects captured. Relevant release bodies and local blog posts were examined. Stable Jellyfin was **12.2**, published 5 October 2026; the supplied server checkout's `13.0.0` is development, not the latest stable release. Plezy **2.23.0** was published during the audit date, 11 October 2026. Public snapshots can change even while a report is being written.
- **Sentiment:** dated Reddit, Hacker News and Lemmy threads selected for migration, device and connectivity experiences. These are self-selected anecdotes. Their vote counts measure attention within a thread, not market share, defect prevalence or a representative survey. Positive and negative accounts are both retained. No fabricated adoption percentages appear here.

Public HTTP data and short extraction notes are staged under `/tmp/portico-audit24` for this run; the deliverable below is self-contained and links primary public sources. Portico references are relative to `/home/justin/projects/Portico`; comparison references use the supplied clone names. No private source was sent to external search.

### Reading the findings

`L-24.x` means architectural lesson; `F-24.x` means significant feature/adoption opportunity; `Q-24.x` means a concrete question prompted by another project's bug history. Q entries distinguish source findings, probable risks and runtime acceptance questions. They are not a list of reproduced Portico bugs. **Already avoids / exposed / unknown** describes the particular failure mechanism, not a verdict on the entire product. Size estimates are qualitative implementation effort including UI, persistence and operational acceptance: small, medium or large. They are not schedules.

## How Portico does it today

Portico is one Go server with one SQLite state folder, shared TypeScript client behaviour, a React web application and React Native clients. Hosted identity/discovery/certificates are separate from media traffic. Direct local username/password identity remains first-class. Google/Apple Hosted sign-in is different from allowing an owner to attach an arbitrary OpenID Connect provider such as Authentik or Authelia. The latter was not located in the audited server identity implementation.

The server composes Home, browse, search and recommendation documents. Narrow normalized catalogue tables hold facts; maintained derived tables hold browse keys, counts, search documents and recommendation features. Requests use consistent snapshots and exact revision keys. Clients page and render. This design directly addresses the class of client enumeration and collection materialization costs that Jellyfin has spent its last two major cycles repairing. Evidence: `Project Architecture/02 Server.md`, `03 Catalogue and Metadata.md`, `05 Recommendations.md`; `server/internal/catalog/search.go:22`, `:129`, `:209`; `server/internal/dbwork/gate.go:24`.

Scanning has a durable inventory boundary. File paths are locators rather than logical media identity. Sources have generations, root incarnations and physical identity; missing reconciliation requires completed inventories, fresh ancestor checks, two observations and elapsed grace. Source removal and forgetting retain logical history. This is substantially more conservative than treating an inaccessible folder as empty. The source implementation is visible at `server/internal/ingestion/inventory.go:248–343` and the inventory contract. However, unchanged-directory optimization presents a separate question about in-place file replacement; see Q-24.3.

Personal state distinguishes progress, watched, rating, favourites, My List and Continue Watching dismissals. Dismissals are tied to a playback identity rather than achieved by erasing progress (`server/internal/catalog/personal.go:73`). Personal half-star ratings exist for playable items (`:267–276`); container state supports saved flags but has no rating field (`container_personal.go:19–31`). Contrary to a stale backlog item, books **can** be watchlisted in current source (`container_personal.go:54–63`). Playlists have named viewer/editor shares and normalized entry rows, including duplicate occurrences. Saved views are dynamic queries, but that is not automatically equivalent to a dynamic playlist playable as a continuously maintained queue.

Playback chooses among direct play, remux and conversion from declared client capabilities. Decisions contain reason codes, source facts and Dolby Vision profile information. HLS is the video transport; text/bitmap/styled subtitle handling, source-relative timestamps, exact source revisions and session generations are explicit. `segmentmarkers` publishes measured or owner-approved skip safety. Chapters, trickplay, lyrics, downloads, Live TV/DVR, Library Channels, Watch Together, playback receivers, backup/restore and diagnostics already have source surfaces. A demand report that simply labels all these absent would mislead Justin. Marker publication/detection does not imply robust repeated-intro recognition: current detectors are conservative chapter/luminance candidates, and cross-episode audio matching was not found (L-24.40/F-24.25).

The deliberate limits matter: port forwarding for remote media, with a possible paid relay later; no push notifications or server email; one metadata provider per library; English launch; no external code contributions; fixed authored player layout; owner decides quality. These are assessable trade-offs, not omissions to be silently “corrected.” Package names, feature strings and a platform capability profile do not establish a released app on that platform. Android/Fire TV/Vega source and builds are not physical-device acceptance. Roku implementation is outside this audit's code scope.

## How the others do it

### Jellyfin: the relevant history, rather than a frozen 10.8 comparison

| Release / date | What changed and why it matters here |
|---|---|
| [10.9.0, 11 May 2024](https://jellyfin.org/posts/jellyfin-release-10.9.0/) | Trickplay previews and lyrics; SQLite pooling; AV1 encoding; DLNA moved out of core into a plugin; UPnP automatic port mapping removed. These combine visible features with optional-service isolation and network/security simplification. Explicit client support was still needed for new server features. |
| [10.10.0, 26 October 2024](https://jellyfin.org/posts/jellyfin-release-10.10.0/) | Media-segment storage/API made intro/credits skipping a server contract with plugin-provided detection; keyframe trickplay extraction reduced expensive generation at a precision trade-off; wider HDR tone mapping, client-side PGS, lyric providers/editor. Plugin existence and client support remained separate conditions. |
| [10.11.0, 19 October 2025](https://jellyfin.org/posts/jellyfin-release-10.11.0/) | Legacy `library.db` moved into EF Core-managed `jellyfin.db`; relationships and cleanup made migrations potentially hours long. New startup status and internal backups accompanied it. Recommended upgrade predecessor 10.10.7, no simple rollback without full restore, ARM32 support ended. Migration failures and query performance were distinct problems. |
| [State of the Fin, 6 January 2026](https://jellyfin.org/posts/state-of-the-fin-2026-01-06/) | Separate general, performance and migration tracking; watched-state preservation on file replacement/rename needed fixes. Client enumeration/filtering remained a performance focus. Qt6 desktop migration introduced another memory/leak investigation and saved-profile migration limits. |
| [State of the Fin, 24 May 2026](https://jellyfin.org/posts/state-of-the-fin-2026-05-24/) | Confirmed new version naming, performance PR [16062](https://github.com/jellyfin/jellyfin/pull/16062), API freeze before release, scanner work targeted at 13.0. Maintainer burnout, review burden and contribution scope are explicitly discussed. |
| [12.0, September 2026](https://jellyfin.org/posts/jellyfin-release-12.0/) | Normalized playlist/collection membership, faster Home/Next Up/music, alternate episode versions, richer books/comics, similarity/search extension interfaces, Modern web layout. Legacy authorization disabled by default; old no-op routes removed; plugin interfaces and .NET target changed. Built-in TLS removal was postponed, not completed. |
| [12.1](https://github.com/jellyfin/jellyfin/releases/tag/v12.1), [12.2](https://github.com/jellyfin/jellyfin/releases/tag/v12.2) | Stable follow-up releases on 15 September and 5 October 2026. Never mistake master `13.0.0` for the deployed stable family. |

EF Core is an object-relational mapping library: it centralizes how code reads/writes relational data. Jellyfin's lesson is centralized schema/access ownership and integrity, not that Go software must adopt an ORM. The [SQLite concurrency post](https://jellyfin.org/posts/SQLite-locking/) describes over-scheduling and long transactions, then retry/serialization strategies. Its casual description of WAL allowing parallel writes should **not** be copied literally: SQLite still has one simultaneous writer. Portico's explicit gate is the relevant comparison.

The [13.0 scanner proposal](https://github.com/jellyfin/jellyfin-meta/discussions/125), created 29 April 2026, separates filesystem enumeration, identification, aggregation, enrichment, writeback, persistence and post-processing. It calls out entangled metadata/scanning, parallel integrity races and stopping filesystem monitoring for the duration of scans. It is a **proposal**, not shipped behaviour. It proposes path matching for item identity; Portico's durable physical/logical distinction should not be discarded just because another maintainer proposes a simpler path-based system.

### Plezy: one application, several backends, several real players

Plezy's attraction is a coherent application across desktop, phone and TV, broad direct-play support and practical extras around playback. Its [README](https://github.com/edde746/plezy) includes downloads/sync rules, offline state sync-back, versions, ASS/SSA, subtitle/audio offsets, frame-rate matching, audio passthrough, gapless music, lyrics, Live TV/DVR, companion remote, tracker/Seerr integration, metadata editing and localization. Its platform footnotes are critical: Android defaults to ExoPlayer; mpv shader features are not the Apple player; tvOS background audio/download limits differ. “mpv everywhere” is an inaccurate shorthand.

The public release sequence is unusually fast. [1.0.0](https://github.com/edde746/plezy/releases/tag/1.0.0) was published 22 October 2025; [2.0.0](https://github.com/edde746/plezy/releases/tag/2.0.0) added Jellyfin and tvOS on 11 May 2026; [2.13.0](https://github.com/edde746/plezy/releases/tag/2.13.0) added Emby on 10 August 2026. The [Emby request #1030](https://github.com/edde746/plezy/issues/1030) opened 12 May and closed 5 August: about 90 days from request to published release, not “added overnight.” The snapshot contains 85 releases in roughly a year; speed has also produced repeated hardware, lifecycle and backend fixes.

The abstraction cost is visible rather than speculative. `plezy/lib/media/media_server_client.dart` defines a common application API; `server_capabilities.dart:1–5` explicitly describes app implementation capabilities, not everything the backend protocol could support. Plex/Jellyfin/Emby differ in ratings, favourites, Continue Watching removal, subtitle marketplace, thumbnails, hubs, playlists and Live TV operations. Releases 2.1–2.3 immediately repaired Jellyfin paging, filters, collections, versions, direct-play preference and subtitles. Later releases repaired original-quality bitrate caps, unaired placeholders, EPG/tuner latency and auth. Shared screens cannot eliminate backend semantic differences.

Watch Together uses a [WebSocket control relay](https://plezy.app/#faq); it exchanges identifiers and timing, **not media bytes or media-server credentials**. Every participant still needs access to the same media on the same server. Custom relays are supported. Current relay/client contracts require authenticated room resumption and capability checks for safe host transfer (`plezy/server/main.go:2004–2035`; README Watch Together). This differs from Portico's server-owned social rooms. Do not confuse either with a paid remote-media relay that solves CGNAT.

The prompt's “donation model” needs correction. The current website FAQ (`plezy/website/src/lib/content/faqs.ts:25–28`) says GitHub binaries are free and App Store/Play Store distribution is a one-time purchase supporting development, fees and test devices. A donation-only business model is **not established** by the current sources. No revenue or relay-cost figures were found.

### What users say, with date and evidence boundaries

| Dated source | Signal | Implication for Portico |
|---|---|---|
| [Server choice, 23 February 2024](https://www.reddit.com/r/selfhosted/comments/1ay7im5/do_you_run_plex_emby_or_jellyfin/) | 217-vote question; 236-vote reply values free/self-hosted operation while the OP prefers Emby's rounded experience. | Convenience and autonomy were competing preferences before later Plex policy changes. |
| [Sharing family VHS library, 16 July 2024](https://www.reddit.com/r/selfhosted/comments/1e4ws90/sharing_jellyfin_with_family/) | Wants relatives to use the TV app without understanding the owner's VPN; replies disagree about exposure versus VPN complexity. | Home-media folder browsing and simple supported remote connection are linked adoption needs. |
| [Plex/Jellyfin TV thread, 3 December 2024](https://www.reddit.com/r/selfhosted/comments/1h5b75v/plex_vs_jellyfin/) | 153-vote question contains an incorrect assumption about client availability; replies correct it and still debate Cast reliability. | Client installation discoverability is part of availability; anecdotes may contain factual errors. |
| [r/selfhosted, 4 January 2025](https://www.reddit.com/r/selfhosted/comments/1htg2ya/plex_vs_jellyfin/) | OP preferred Plex usability after first blaming Jellyfin for a media problem; a pro-Jellyfin reply had 313 votes, a privacy/local-authority reply 177. | Diagnose media/setup/client/server causes separately; privacy appeal does not erase convenience demand. |
| [Plex→Jellyfin migration, 21–22 March 2025](https://www.reddit.com/r/selfhosted/comments/1jgssoe) | Replies identify inconsistent family/profile switching and different app behaviours as blockers. | Shared behaviour is valuable only if switching is actually reachable and reliable on the television. |
| [Plex lifetime thread, 17 August 2025](https://www.reddit.com/r/selfhosted/comments/1mt40wd/plex_lifetime_user_here_any_real_advantages_in/) | 393-vote post; a privacy reply had 707 votes. Others retain Plex/Emby for family devices or remote access, or pair Jellyfin with paid Infuse. | Both owner freedom and household reliability are real preferences. Existing subscriptions change migration incentives. |
| [Remote family setup, 9 September 2025](https://www.reddit.com/r/selfhosted/comments/1ncndym) | 124-vote question; Tailscale suggestions immediately raise installation/account requirements for relatives. | A server owner accepting network work is not consent from every viewer to install a VPN. |
| [Plezy update, 14 January 2026](https://www.reddit.com/r/PleX/comments/1qcdxot/plezy_update_3_months_later_downloads_watch/) | 967-vote launch/update post foregrounds downloads, Watch Together and HDR/DV. | Reliable playback and offline portability can attract users without replacing their existing server first. |
| [Jellyfin migration regret, 6 June 2026](https://www.reddit.com/r/jellyfin/comments/1ty4zhq/beginning_to_regret_migrating_away_from_plex/) | 114-vote question: easy Docker setup followed by HEVC browser/transcode and desktop audio-only problems; replies disagree about configuration difficulty. | Installation success is not playback readiness. Show the actual decode/encode path and a useful recovery action. |
| [Finzy fork introduction, 7 June 2026](https://www.reddit.com/r/jellyfin/comments/1tyxdor/finzy_a_crossplatform_jellyfin_player_mac_windows/) | 35-vote post explicitly sought the same phone/Android TV interface; replies praise quick startup. | Cross-device consistency is an independent product benefit; a fork's praise is not a Portico benchmark. |
| [Why Plezy?, 12 August 2026](https://www.reddit.com/r/PleX/comments/1vmrhw9/why_is_everyone_suddenly_talking_about_plezy/) | 264-vote question comparing Plex's own client with Plezy. | Users can reject a first-party client while retaining its server. Portico needs a deliberate third-party-client stance. |
| [Windows HDR client, 30 September 2026](https://www.reddit.com/r/jellyfin/comments/1wtvyni/windows_client_for_4k_hevc_hdr_support/) | Different HDR results across clients; the poster reported Plezy worked without further setup. | Colour correctness is device/player-specific; a codec checkbox is insufficient evidence. |
| [Best clients, 3 October 2026](https://www.reddit.com/r/jellyfin/comments/1www19x/best_jellyfin_clients/) | Plezy praise, Wholphin speed, Streamyfin on phone, Moonfin across devices; another user uses a proxy rewrite to obtain direct play. | No universal best client emerges. Friction from capability negotiation is enough to motivate workarounds. |
| [HN Jellyfin 12.0 discussion](https://news.ycombinator.com/item?id=49607774) | One commenter uses Plezy on iOS/iPadOS/tvOS while noting uncertain backend parity. | One polished cross-backend client can make server choice easier; this single comment is not adoption data. |
| [Lemmy remote access discussion](https://lemmy.world/post/29026765) | Participants distinguish their own Tailscale success from asking relatives to install it; others prefer a public URL/reverse proxy. | Offer supported connection patterns and explain CGNAT honestly. Thread timestamps are relative; exact dates were not established. |

Infuse draws repeated praise for Apple codec support and polish, but some users dislike its server integration/navigation. Streamyfin is a notable React Native alternative; Findroid illustrates the value of a native Android experience; Swiftfin has an independent iOS/tvOS roadmap and long-standing download/Cast requests. Wholphin and Moonfin compete particularly on television speed/navigation. Finamp and Symfonium demonstrate demand for a specialised music experience, offline listening and car integration. Fladder appears in desktop client comparisons. **Void** has a [native Android alpha announcement dated 21 August 2025](https://www.reddit.com/r/JellyfinCommunity/comments/1mw8crh) and appears in [May 2026 client comparisons](https://www.reddit.com/r/jellyfin/comments/1tfz6bg/looking_for_the_best_jellyfin_client_setup_in_2026/). This establishes another native-client demand signal, not current release maturity or measured superiority. Client-specific forum claims are not proofs of those projects' current release status.

### Other architectural peers and what can actually be inferred

**Stump.** [Stump](https://github.com/stumpapp/stump) is a Rust/Axum/SeaORM/React comics, manga and book server with OPDS interoperability. Its [release stream](https://github.com/stumpapp/stump/releases) and roadmap are stronger evidence than merely recognizing its name. The repository snapshot shows a push on 11 October 2026 and [v0.1.10](https://github.com/stumpapp/stump/releases/tag/v0.1.10) on 21 September 2026: positive current-activity evidence. It is a reading peer rather than a proven general video replacement. Lesson: standards-based catalogue access can bring external readers without building every device app. Portico could evaluate OPDS if books become launch scope; there is no case here for importing Stump's stack wholesale.

**Dim.** [Dim](https://github.com/Dusk-Labs/dim) remains publicly accessible, with a Rust-oriented media-server design and open tracker. The REST snapshot says unarchived, last repository push 21 September 2025, and [latest published release v0.3.0-rc6](https://github.com/Dusk-Labs/dim/releases/tag/v0.3.0-rc6) dated 19 October 2021. That is weak current shipping evidence; it does **not** prove there is no untagged development or security maintenance. No fresh maintainer roadmap was established. Lesson: an attractive new implementation and thousands of stars are weak evidence of sustained household usability. Do not label it dead or successful without maintenance evidence.

**Kyoo.** [Kyoo](https://github.com/zoriya/kyoo) is the closest philosophical peer: built-in features, React Native client, low-maintenance matching, intro detection and explicit OIDC. Its v5 architecture uses PostgreSQL and separable services. The snapshot records a 10 October 2026 push and [v5.2.1](https://github.com/zoriya/Kyoo/releases/tag/v5.2.1) on 27 August 2026, so current activity is established. The README openly marks downloads and tracker sync as not yet reimplemented after the rewrite, with Android TV still planned. Lesson: a technically cleaner rewrite can leave users with temporarily fewer capabilities. Portico's one-process/SQLite design offers lower setup ceremony; Kyoo's service/standard identity approach offers different integration and scale choices. Neither proves the other's performance.

**Streama.** [Streama releases](https://github.com/streamaserver/streama/releases) include 1.11.0 on 2 January 2026 and a preceding security fix for subtitle-download SSRF/path traversal. It should not be dismissed as abandoned solely from its older reputation. Its browser-oriented sharing/library product is narrower than Jellyfin's client estate. Lesson: external URL/subtitle imports are security boundaries even when the feature feels like a small convenience; maintenance and password recovery matter beyond launch aesthetics.

**Navidrome.** The supplied Go/SQLite peer advertises large music libraries, low resource use, robust compilation/multi-disc metadata, transcoding and compatibility with Subsonic/OpenSubsonic clients (`navidrome/README.md:65` onward). Its official [client directory](https://www.navidrome.org/apps/) listed **101 applications** at the snapshot. Compatibility lets it satisfy mobile/car/offline needs through an ecosystem rather than owning every app. This is evidence of breadth, not proof all 101 are maintained or equally good. Portico should preserve normalized music identity and evaluate a compatibility surface only if it can express Portico's authority/restriction rules faithfully.

**Audiobookshelf.** The supplied `audiobookshelf/readme.md` prioritizes podcasts, multi-file books, per-user progress, chapters, metadata editing, M4B merging, RSS and daily backups. These are coherent listener workflows rather than generic video features renamed. It also exposes operational constraints: websocket proxy requirements, an iOS TestFlight capacity limit and a frontend rewrite pause. Lesson: specialist completeness, import paths and routine backup can matter more than adding every media category; store distribution limits can become adoption bottlenecks independent of code quality.

**Immich.** Its [stable-release announcement, 1 October 2025](https://immich.app/blog/v2.0.0-release), reported 271 releases and 1,558 contributors before stable, then explicitly promised semantic versioning and 2.x mobile/server compatibility. Its [Sync v2 account](https://immich.app/blog/sync-v2) describes rebuilding mobile sync around scale and independently updating clients. Inference: mobile utility, continuous delivery and resolving sync debt made a compelling alternative, but counts alone do not prove causality. For Portico, the useful lesson is to make compatibility and recovery part of stable status, and to introduce any telemetry plan transparently. ML search is a product capability with index/model/update costs, not a substitute for catalogue correctness.

## Architectural lessons

### L-24.1 Centralize persistence before shipping years of user data

- **Evidence:** Jellyfin [10.11](https://jellyfin.org/posts/jellyfin-release-10.11.0/) describes a multi-year EF Core conversion delayed by sprawling library access and inconsistent records.
- **Portico:** **Already avoids much of the mechanism** through `compactcatalog` fact writes, `catalog` reads, relational tables and an immutable-after-release baseline (`server/internal/persistence/README.md:13`).
- **Why it matters:** corruption and one-off fixes become much harder to migrate once they are household history. An ORM alone does not enforce ownership or cheap queries.
- **Assessment:** **Portico ahead in stated architecture; runtime unproven.** Keep the boundary and require changes to use it. Small governance cost now; large retrofit later. I would revise this if significant direct writers bypass the canonical fact API.

### L-24.2 A connection pool is not a concurrency policy

- **Evidence:** [SQLite locking discussion](https://jellyfin.org/posts/SQLite-locking/) identifies over-scheduled scans and long transactions; [#15101](https://github.com/jellyfin/jellyfin/issues/15101) accumulated 230 comments.
- **Portico:** **Already avoids uncontrolled writers** with a single physical write gate, foreground priorities and aging background work (`dbwork/gate.go:19–26`). Storage/network work must stay outside it.
- **Assessment:** **Portico ahead on explicit policy.** Preserve measured transaction durations and leak diagnostics. Medium acceptance work on slow disks, LXC/ZFS and overloaded containers. A stalled holder still blocks everybody; serialization cannot repair hostile storage latency.

### L-24.3 Bounded pages must reach all the way down to storage

- **Evidence:** Jellyfin [12.0](https://jellyfin.org/posts/jellyfin-release-12.0/) replaces whole serialized playlist/collection lists with membership rows so counting, paging and edits stop materializing everything.
- **Portico:** **Already avoids this storage shape:** `catalog_playlist_entries` and `catalog_collection_members`, with page indexes (`persistence/migrations/0001_baseline.sql:225–260`).
- **Assessment:** **Portico ahead.** Keep normalized membership and occurrence IDs; do not regress into JSON arrays for convenience. Small ongoing review requirement. Remaining container grids and exact counts deserve independent scale measurement; normalized storage does not automatically make every query indexed.

### L-24.4 Database modernization can expose, rather than cure, bad queries

- **Evidence:** January 2026 Jellyfin performance tracking and [PR #16062](https://github.com/jellyfin/jellyfin/pull/16062) follow the 10.11 rewrite; 12.0 addresses reads without claiming scanner optimization.
- **Portico:** **Exposed in residual paths:** Backlog names container OFFSET paging, sparse-filter counts, broad prefixes, linear shuffle and repeated metadata sweeps; the music-artist sweep entry is stale because current source uses bounded cursor windows. Maintained projections are the right foundation.
- **Assessment:** **Trade-off / partially ahead.** Medium work should target actual outliers. The relevant acceptance question is response/resource bounds under large real data, not whether the database library is fashionable. Would change my view after a trace proves these costs negligible at target scale.

### L-24.5 “Stable” is an upgrade and compatibility promise

- **Evidence:** Jellyfin dropped the constant `10.` prefix after a major database rewrite looked like a minor release. [Immich stable](https://immich.app/blog/v2.0.0-release) explicitly ties stable to semantic versioning and mobile/server compatibility.
- **Portico:** **Unknown public promise.** Versioned API routes and generated schemas exist, but no complete release compatibility/LTS policy was established in the audited scope.
- **Assessment:** **Portico behind on a published promise, appropriately pre-release.** Small policy decision, medium enforcement. Make data migration, API break and client-store lag rules visible before 1.0. The first release should not promise every current behaviour forever.

### L-24.6 Startup work needs progress and a deliberate operator path

- **Evidence:** Jellyfin 10.11's long migrations prompted a startup/log interface; [12.0](https://jellyfin.org/posts/jellyfin-release-12.0/) adds `--mode MigrateSystem` to upgrade and exit.
- **Portico:** **Already avoids intended bulk startup rewrites**, but **exposed** if new indexes/backfills violate its additive policy. “Add index” can itself touch every existing row; the policy deserves a concrete implementation interpretation.
- **Assessment:** **Portico ahead in intent / unclear in future execution.** Medium work to define resumable backfills and startup/readiness states. Never advertise “upgrades in seconds” without including index creation and failure recovery. A displayed building state is more honest than appearing dead.

### L-24.7 Backup portability and downgradeability are separate features

- **Evidence:** Jellyfin 10.11 backup is limited to compatible/original environments and requires full restore for downgrade; newer schema migrations remain a rollback boundary.
- **Portico:** **Partly already avoids lock-in** with plain SQLite/files and staged restore (`backup/apply.go`, `copy.go`, `schedule.go:25`). Pre-baseline development states are intentionally rejected.
- **Assessment:** **Portico ahead in inspectability, unknown portability acceptance.** Medium restore drills across path/OS changes and server versions. A copyable file format does not prove every external media root, certificate, state reference or native-client record migrates. Define the supported restore matrix.

### L-24.8 Inventory should survive metadata failure

- **Evidence:** Jellyfin's [scanner rewrite proposal](https://github.com/jellyfin/jellyfin-meta/discussions/125) disentangles enumeration, matching, enrichment and persistence after errors/races in recursive scanning.
- **Portico:** **Already avoids coupling** through committed inventory pages and later revision-fenced analysis (`ingestion/INVENTORY_CONTRACT.md:45` onward; `ingestion/analysis.go:302`).
- **Assessment:** **Portico ahead.** Preserve provisional visibility and independent failure reporting. Medium operational UI work if users cannot tell “file seen” from “metadata matched” from “analysis done.” Adoption should not wait on a provider outage before an owner can play a known file.

### L-24.9 A disconnected mount is not an empty collection

- **Evidence:** Jellyfin [zero-item scan warning request #17214](https://github.com/jellyfin/jellyfin/issues/17214), [playlist loss #12008](https://github.com/jellyfin/jellyfin/issues/12008), and the scanner proposal's explicit deletion discussion make destructive reconciliation an architectural concern. Mount-specific prevalence was not quantified here.
- **Portico:** **Already avoids the direct empty-list mechanism:** root identity, authoritative completed run, ancestor verification, two observations and grace (`ingestion/inventory.go:271–330`).
- **Assessment:** **Portico ahead by reading.** Keep reversible missing/trash separate from deleting logical history. Medium failure-injection acceptance across NFS/SMB/rclone/remounts. Would revise if a legacy cleanup route bypasses these protections.

### L-24.10 File identity, work identity and edition identity are different

- **Evidence:** version requests [#174, 266 votes](https://features.jellyfin.org/posts/174/better-handling-of-movie-versions-in-multiple-directories), [#2537, 104](https://features.jellyfin.org/posts/2537/display-movie-version-like-extended-directors-cut-etc), and episode versions shipped in 12.0 after years of demand.
- **Portico:** **Exposed at automatic grouping**; durable physical identity and explicit version association already exist. Backlog says a second same-title file becomes a second title. The inventory contract deliberately refuses to infer equivalence from filename alone.
- **Assessment:** **Trade-off.** Large work to represent editions/cuts, multiple files, versions and progress safely. Keep conservative association, then add explainable matching/manual merge rather than folding unrelated works. Shortcuts here become permanent identity debt.

### L-24.11 Replacing a file must invalidate more than codec facts

- **Evidence:** Jellyfin 10.11 explicitly regenerates chapters/trickplay/subtitles/attachments on replacement; January 2026 reports watched-state repair work.
- **Portico:** **Exposed** where incremental directory skipping fails to observe a byte replacement; **already equipped** with source/revision fences for admitted changes. See Q-24.3.
- **Assessment:** **Portico behind on this known edge / ahead in downstream fencing.** Medium scanner correction/acceptance. File facts, subtitle extraction, intro detection, artwork and download preparations should share the same invalidation cause while logical progress survives.

### L-24.12 Implement the server feature and the client contract together

- **Evidence:** Fider marks [intro skip #45](https://features.jellyfin.org/posts/45/skip-intro-outro-credits-feature) completed while [Swiftfin #1525](https://github.com/jellyfin/Swiftfin/issues/1525) still asks for its skip button; lyrics and downloads similarly vary by client.
- **Portico:** **Already avoids some semantic fragmentation** through shared `client-core`, authored structure and common markers, but **exposed** to native renderer/store delivery differences.
- **Assessment:** **Portico ahead in structure, unknown acceptance.** Medium per-platform coverage. Publish a server × client × device matrix. A completed backend ticket is not completion for a household. This is especially important for Cast and downloads.

### L-24.13 One UI framework does not make one playback engine

- **Evidence:** Plezy has mpv, ExoPlayer and Apple-specific routes; [#2302](https://github.com/edde746/plezy/issues/2302) reports mpv failure where ExoPlayer works, and [#2255](https://github.com/edde746/plezy/issues/2255) reports stutter across options.
- **Portico:** **Exposed** to AVPlayer/Media3/Vega/browser differences despite shared React Native screens.
- **Assessment:** **Trade-off.** Large physical playback acceptance, not necessarily large shared-code change. Keep engine facts separate from app styling, and make fallback understandable. A wholesale mpv adoption would add distribution/security/hardware responsibilities; demand does not prove it is the best fix everywhere.

### L-24.14 Capability evidence must describe the actual route

- **Evidence:** Jellyfin Android TV [#5093](https://github.com/jellyfin/jellyfin-androidtv/issues/5093) reports unnecessary HEVC/DV conversion after an update; Plezy releases repeatedly fix DV, AV1, passthrough and frame pacing.
- **Portico:** **Partly already avoids generic labels** with per-stream/profile/container/range checks (`playback/delivery_decision.go:159`, `delivery_plan.go:652`). Actual display/receiver capability remains unknown until measured.
- **Assessment:** **Portico ahead in traceability / exposed operationally.** Medium profiles, large hardware coverage. Store the effective decoder and sink path, not just “device supports HEVC.” A receiver, OS change or subtitle mode can change the admissible tuple.

### L-24.15 Subtitle choice is a resource and timing operation

- **Evidence:** Server [#2547](https://github.com/jellyfin/jellyfin/issues/2547), 105 reactions/151 comments, and web [#4346](https://github.com/jellyfin/jellyfin-web/issues/4346), 36/65, span years of seek/resume desync.
- **Portico:** **Already addresses timing scope** with source-relative WebVTT, selected offset and generation/revision fences (`subtitles/manifest.go:49–103`; `playbackv1/sessions_change.go:144`). Rendering/seek acceptance is **unknown**.
- **Assessment:** **Portico ahead in protocol shape.** Medium fixture/device matrix. Test external text, ASS fonts, PGS, negative offsets, fullscreen/PiP and quality changes. A good-looking subtitle settings sheet does not establish synchronization.

### L-24.16 Observability must not perturb playback

- **Evidence:** Plezy [2.20.0](https://github.com/edde746/plezy/releases/tag/2.20.0) reduces logging cost and fixes an overlay delaying the playback it measured.
- **Portico:** **Exposed / unknown:** rich diagnostics exist; Apple dropped-frame/buffer reporting is known incomplete in Backlog.
- **Assessment:** **Trade-off.** Small bounded reporting design, medium validation under weak CPU. Counters should be sampled/aggregated without frame-by-frame bridge churn or giant logs. Useful logs are an adoption lever, but permanently high debug overhead can recreate the reported defect.

### L-24.17 Resource preparation should be optional, incremental and visible

- **Evidence:** [pre-transcoding #570](https://features.jellyfin.org/posts/570/pre-transcoding), 979 votes, remains open; trickplay's keyframe option appeared after multi-day generation complaints.
- **Portico:** **Already offers preparations** and artifact leases, with background priority and analysis policy warnings. **Exposed** to total disk/IO cost at multi-million scale.
- **Assessment:** **Portico ahead in capability.** Medium operational budgets, cancellation/restart and progress reporting. Keep quality under owner control; a low-resolution preview is a separate artifact policy, not permission to lower a playback request silently.

### L-24.18 Quick sign-in has to include real televisions and weak networks

- **Evidence:** [QR Quick Connect #2637](https://features.jellyfin.org/posts/2637/quick-connect-with-qr-code), 227 votes; [Swiftfin Chromecast #271](https://github.com/jellyfin/Swiftfin/issues/271), 43 reactions; Plezy [offline manual Plex #1447](https://github.com/edde746/plezy/issues/1447), 12.
- **Portico:** **Already has** TV QR/code flows and local direct identity (`native/src/screens/auth/QuickConnect.tsx:61`). Google/Apple flows and physical Cast remain unaccepted in Backlog.
- **Assessment:** **Portico ahead on intended low-ceremony flow.** Medium failure/retry acceptance. Test expired/approved codes, server restarts and no Internet. Avoid equating Hosted-account failure with invalid local credentials.

### L-24.19 A shared backend abstraction needs explicit semantic differences

- **Evidence:** `plezy/lib/media/server_capabilities.dart` distinguishes ratings/favourites, resume removal, thumbnails, hubs and subtitle search; early Jellyfin releases fixed adapters rapidly.
- **Portico:** **Already avoids the current cross-backend cost** by owning one server protocol. A future Plex/Jellyfin import or client adapter would be **exposed** to it.
- **Assessment:** **Trade-off.** Large if Portico becomes a multi-backend client. Keep feature and runtime availability separate, and represent unsupported semantics directly. Importing watched state is a much narrower and safer undertaking than maintaining live compatibility with three server APIs.

### L-24.20 Multi-server identity is not a unified catalogue

- **Evidence:** [#47](https://features.jellyfin.org/posts/47/multiple-servers-support), 178 votes; [#407](https://features.jellyfin.org/posts/407/aggregate-multiple-servers-in-client-ui), 83; Plezy 2.23 fixes one slow server blocking other results.
- **Portico:** **Already has** known-server selection and Hosted directory; cross-server search/merged rows were **not established**. Contracts explicitly scope saved state and social playback to the selected server.
- **Assessment:** **Trade-off, deliberate authority boundaries.** Large federation/fan-out effort. If added, return independent server results/failures and preserve server-qualified identity. Do not claim central sign-in already satisfies this demand.

### L-24.21 Offline mode is a product state, not a download button

- **Evidence:** Fider [#218](https://features.jellyfin.org/posts/218/support-offline-mode-on-android-mobile), 1,820 votes, is started partly because raw-file download is not a managed offline library; Swiftfin downloads has 145 reactions.
- **Portico:** **Already models** preparation, scoped local records, authorization-unverified states, resume replay and reconciliation in contracts/source. **Unknown** full phone acceptance and automatic sync-rule parity.
- **Assessment:** **Portico ahead in design.** Large mobile/offline lifecycle acceptance. Test OS kill, storage pressure, account switch, expiry, network return and conflicting explicit watched actions. Do not remove useful offline bytes merely because a transient request failed.

### L-24.22 Social playback needs reconnect identity and an expiry model

- **Evidence:** Jellyfin [#17078](https://github.com/jellyfin/jellyfin/issues/17078) reports zombie SyncPlay groups after silent websocket drops; Plezy's current relay requires authenticated resumption, not rejoining a reused room code.
- **Portico:** **Already distinguishes** server-issued group invitations, participants, queues, timeline revision and scoped receipts; liveness/drift on real networks is **unknown**.
- **Assessment:** **Portico ahead in authority isolation.** Medium reconnect/timing acceptance. Retain the separation from Hosted and remote-media relay. A short room code should never become a long-lived recovery credential.

### L-24.23 Privacy claims need to explain each boundary

- **Evidence:** privacy dominates several migration threads; Plezy's FAQ discloses relay-visible identifiers/title/timing. Immich discussed transparent future usage data rather than silently introducing it.
- **Portico:** **Already separates** local media/history from Hosted, but has permanent owner-visible play history by deliberate decision (`catalog/play_history.go:18`; architecture §Other services).
- **Assessment:** **Trade-off, deliberate.** Small clear notice, medium redaction review. “Private” should describe private from whom: Hosted, third-party providers, relay operator, household owner, other profiles. Owner history and viewer history are different privacy expectations.

### L-24.24 Optional integrations can become update liabilities

- **Evidence:** Jellyfin moved DLNA to a plugin in 10.9, changed database/plugin interfaces in 10.11/12.0, and deprecates APIs over a release cycle.
- **Portico:** **Unknown extension stance in code; deliberate no external contributions** in brief. DLNA is planned off-by-default with whole-server exposure.
- **Assessment:** **Trade-off.** Decide before 1.0 whether extensions are out-of-process APIs, supported plugins or intentionally absent. Large in-process ABI/security commitment; medium narrow external API. Built-in modules also need disabling and independent failure containment.

### L-24.25 Authentication cleanup is an ecosystem migration

- **Evidence:** 10.11 offered a legacy-auth-off test option; 12.0 disabled deprecated mechanisms by default and changed QuickConnect GET to POST. Plezy's release history records compatibility repairs.
- **Portico:** **Already uses** device-bound modern sessions, `/v1` contracts and shared parsers; older store clients/custom integrations remain **exposed** to future tightening.
- **Assessment:** **Portico ahead at a clean starting point.** Small written sunset rules, medium compatibility fixtures. Provide advance diagnostics for obsolete clients and a security exception path only if Justin intends one. A route alias is not a permanent promise unless published as such.

### L-24.26 Search should fail by section, not erase everything

- **Evidence:** web [#1752](https://github.com/jellyfin/jellyfin-web/issues/1752) asks about 24-result limits; Plezy 2.23 prevents one unresponsive backend blocking other servers; [fuzzy request #3703](https://features.jellyfin.org/posts/3703/fuzzy-search-to-jellyfin), 90 votes, identifies typo tolerance.
- **Portico:** **Already avoids all-or-nothing sections** with per-group budget/status and complete cursors (`catalog/search.go:22–25`, `:64–74`); it uses token-prefix, not demonstrated fuzzy search.
- **Assessment:** **Portico ahead on completeness/failure isolation; behind on fuzzy capability.** Medium optional suggestion index. Keep exact results correct and restriction-aware; fuzzy suggestions must not expose hidden titles or silently replace the literal query.

### L-24.27 Music users notice grouping and continuity more than generic feature count

- **Evidence:** gapless demand 649 votes, multi-disc [server #5605](https://github.com/jellyfin/jellyfin/issues/5605) 71 comments, album grouping [#6694](https://github.com/jellyfin/jellyfin/issues/6694) 42, and specialised Finamp/Symfonium use.
- **Portico:** **Already prioritizes** native gapless, gain/trim, MusicBrainz and album/work identity; smart radio and listening similarity are known open work.
- **Assessment:** **Portico ahead in ambitions, unknown perceptual/device acceptance.** Medium metadata fixtures and large DSP/recommendation optional work. Preserve compilations, disc/track order and different releases; never “fix” duplicates by collapsing legitimate editions.

### L-24.28 Accessibility choices can be playback requirements

- **Evidence:** Android TV [subtitle/audio sync #2479](https://github.com/jellyfin/jellyfin-androidtv/issues/2479), 32 reactions/38 comments; [dual subtitles #4394](https://github.com/jellyfin/jellyfin-androidtv/issues/4394), 37 reactions; Plezy text-size and subtitle placement requests.
- **Portico:** **Has partial controls** and shared structure; dual subtitles, broad subtitle style parity and learning-language workflows were **not established**.
- **Assessment:** **Trade-off.** Medium controls, potentially large dual-renderer work. These are not merely cosmetic preferences for viewers with hearing, vision or language needs. Fixed player layout should still support readable focus and text across devices.

### L-24.29 Remote access combines discovery, reachability and authorization

- **Evidence:** dated migration threads accept Plex because family opens one app; Tailscale suggestions require additional software/accounts. CGNAT cannot be solved by knowing the public IP.
- **Portico:** **Ahead on discovery/certificates**, **exposed on reachability** with deliberate port forwarding and no current media relay (`Project Architecture/07 Hosted Services.md`).
- **Assessment:** **Trade-off, deliberate.** Large relay service or medium documented overlay/VPS path. Show CGNAT/double-NAT diagnosis, not an attractive URL that cannot be reached. Relay funding, privacy and bandwidth limits are product decisions.

### L-24.30 Client availability is partly controlled by stores and maintainers

- **Evidence:** January 2026 Jellyfin describes Samsung review failure, Roku publishing blackout and Swiftfin tvOS resync; Audiobookshelf's iOS beta capacity is full.
- **Portico:** **Exposed** across new native ports and future TV distribution. Platform names in sign-in/profile code are not store availability.
- **Assessment:** **Portico behind on demonstrated distribution, expected pre-release.** Large sustained operational effort. Publish tested OS/device/store availability and alternative installation paths. An accepted build does not promise the next security fix can reach every user immediately.

### L-24.31 A contribution model must budget the review queue

- **Evidence:** Jellyfin May 2026 explicitly describes burnout and features needing scope/design discussion; Audiobookshelf pauses Vue frontend PR review during rewrite.
- **Portico:** **Already makes a deliberate different choice:** no external code contributions. Bug reports/docs/testing/translation channels were not fully established here.
- **Assessment:** **Trade-off.** Small policy, medium triage infrastructure. External code refusal can protect coherence but concentrates maintenance and reduces volunteer TV/localization throughput. Preserve a useful evidence intake path even if code remains sole-author-controlled. I would change this assessment with a viable long-term support/delegation plan.

### L-24.32 Fast releases need narrow blast radius and recovery

- **Evidence:** Plezy's 85-release sequence delivers features quickly but has recurring playback regressions; Jellyfin's hoped-for six-month cycle slipped badly for 10.11.
- **Portico:** **Unknown public cadence/support policy**; signed build manifests and release snapshots are present.
- **Assessment:** **Trade-off.** Medium canary/release notes and rollback discipline. Separate security fixes from large architectural releases; describe what changed for each affected client. Avoid both a promise of constant feature speed and unsupported indefinite LTS. Frequency alone is neither quality nor failure.

### L-24.33 Maintenance scheduling must preserve active playback

- **Evidence:** Jellyfin 12.0 moves heavy maintenance out of scans; Plezy separates logging/overlay work from rendering. Portico background-first principles are deliberate.
- **Portico:** **Already prioritizes** work classes and bounded batches, but **exposed** to expensive background loops identified in Backlog.
- **Assessment:** **Portico ahead in design.** Medium idle/loaded acceptance. Track foreground tail latency, starvation prevention and resource recovery rather than aggregate scan speed alone. Weak devices still need eventual background completion; “deprioritized” requires evidence it does not become effectively paused forever.

### L-24.34 Completed tracker status is not the whole current truth

- **Evidence:** [backup #1603](https://features.jellyfin.org/posts/1603/backup-and-restore) remains started with 143 votes while [#299](https://features.jellyfin.org/posts/299/easy-import-export-jellyfin-settings) is completed with 899; watch-together and still-watching have overlapping records with different states.
- **Portico:** **Unknown demand-intake workflow.** This report cross-checks completion against releases and actual source instead of counting every open ticket as absence.
- **Assessment:** **Unclear / small process opportunity.** Link duplicates and mark client/backend/version scope. A public backlog should say “shipped on web,” “requires plugin” or “planned,” rather than creating years of ambiguous expectations.

### L-24.35 Local metadata and provider ordering are deliberate product choices

- **Evidence:** Jellyfin's scanner proposal preserves provider ordering and per-type options. Portico's catalogue document says one provider “as Plex and Jellyfin have it,” but Jellyfin supports a richer ordered provider/plugin model.
- **Portico:** **Deliberately different:** one provider per library with local overrides/ownership. Known crew/local NFO and provider-change details deserve current-source reconciliation.
- **Assessment:** **Trade-off.** One provider makes provenance simpler; chains broaden coverage at operational/debugging cost. Large chain adoption, small correction to the comparative statement in docs. Demand for more metadata does not justify silent provider mixing or overriding owner locks.

### L-24.36 Successful narrow peers exploit workflows and standards

- **Evidence:** Navidrome's OpenSubsonic ecosystem, Audiobookshelf's podcast/RSS/chapter workflows and Immich's mobile sync promise demonstrate different successful shapes. None relies solely on a new language or attractive Home page.
- **Portico:** **Strong breadth and shared first-party intent**, but **exposed** to supporting video, music, books, DVR and every TV at once.
- **Assessment:** **Trade-off.** Medium launch focus, large optional protocol adapters. Decide which complete household journey Portico can guarantee at 1.0. The evidence would favour broader launch only after independently usable end-to-end workflows are accepted on the named devices.

### L-24.37 Demand can regress when an implementation violates query budgets

- **Evidence:** [tag/genre search #276](https://features.jellyfin.org/posts/276/search-by-tag-genre), 256 votes, has a maintainer response dated 30 December 2024 saying tag search added for 10.9 was reverted in 10.10 because of search performance.
- **Portico:** **Already has** indexed browse attributes, maintained search documents and group budgets; free-text tag/genre equivalence remains uncertain.
- **Assessment:** **Portico ahead in the intended substrate / unknown demand completion.** Medium work to index and expose the intended semantics rather than joining expansive metadata on every search. This is unusually direct evidence that satisfying a feature request once does not mean it can safely remain shipped. A benchmark of narrow and broad queries under restrictions would change the assessment.

### L-24.38 Maintainers decline some popular requests deliberately

- **Evidence:** [federation #184](https://features.jellyfin.org/posts/184/federated-servers) has 381 votes but is declined in favour of external aggregation; [upload #543](https://features.jellyfin.org/posts/543/upload-file-from-the-app-or-site), 116, is outside server scope. Pre-transcoding's open 979-vote record has a 2025 response preferring external tools/plugin work.
- **Portico:** **Deliberately broader** on preparations, diagnostics and integrated first-party flows; narrower on contributions and optional integrations.
- **Assessment:** **Trade-off.** Demand should inform Justin's decisions, not become an automatic roadmap. Small explicit scope statements; large implementation if scope expands. Leave a supported export/integration boundary where appropriate. “Open” does not promise implementation, and “declined” can mean duplicate or bug reclassification rather than rejection of user value.

### L-24.39 Platform access can block a feature despite votes and shared code

- **Evidence:** [VIDAA #1615](https://features.jellyfin.org/posts/1615/jellyfin-client-vidaa-os), 457 votes/119 comments, has a September 2023 maintainer response describing corporate-registration barriers. [Header auth #471](https://features.jellyfin.org/posts/471/header-authentication), 168 declined votes, cites making authentication work across TV operating systems.
- **Portico:** **Exposed** to store/business/device constraints on planned TVs and any browser-dependent authentication adapter. Current VIDAA requirements were not independently verified; the response is dated historical evidence.
- **Assessment:** **Trade-off / unknown feasibility.** Medium feasibility work before large port investment. Shared React Native UI reduces code duplication, but cannot grant SDK/store access or a browser-login route on every TV. Device-specific evidence can justify a bridge/companion workflow or choosing a streaming box instead.


### L-24.40 A skip-marker API is not high-quality automatic intro detection

- **Evidence:** completed intro request #45 has 1,089 votes. The supplied [Intro Skipper plugin](https://github.com/intro-skipper/intro-skipper) compares shared audio across a season with Chromaprint (`IntroSkipper/Analyzers/ChromaprintAnalyzer.cs:128–177`), preserving user-supplied results and candidate provenance.
- **Portico:** **Already has** marker publication, owner editing and a conservative automatic-safe policy. Actual detection is chapter-label plus low-confidence luminance hypotheses: intro .3, recap .2, commercial .25, credits .35 (`mediaanalysis/detectors.go:43`, `:51–98`); automatic skip requires .85 plus measured provenance or owner approval (`segmentmarkers/markers.go:44–70`). No cross-episode audio-comparison detector was found in mediaanalysis.
- **Assessment:** **Portico ahead on conservative safety, behind on this detector capability.** Large bounded season-comparison/indexing work if wanted; do not increase confidence numbers without new measurement. A user may reasonably expect automatic introductions to be found without editing each file, but the present code deliberately refuses to turn weak guesses into unattended skips.


## Ranked demand table

The following is a **vote-ranked active-demand ledger**, not a recommended implementation order. Rank follows the fetched most-wanted view; the last column is my judgement about Portico priority. Existing features still appear because their demand supports launch acceptance and advertising. “Not located” is the result of source searches, not proof that no alternate implementation exists. “Unknown/uncertain” means a related implementation was found but equivalence was not established.

Votes are exact public snapshot counts, not population estimates. Status is Fider's own `open`, `planned` or `started`; “planned” is not a date commitment and “started” can mean one client only. Every row links the actual request and records creation date. Product references in the peer column rely on the README/release/capability evidence above; “JF request” deliberately does not assert Jellyfin has it. Sizes refer to remaining parity/acceptance effort, even when the feature exists.

Evidence keys are expanded immediately below the table into exact Portico paths/lines and grep boundaries. `G-...` means the specific feature was searched in server/shared/native code and was not substantively located; spelling collisions such as subtitle cues, DVR matching “VR,” theme CSS or guest actors do not count as implementations.

| Rank | Feature / request | Votes; status; created | Peer capability evidence | Portico status and evidence | Size | Opinion |
|---:|---|---|---|---|---|---|
| 1 | [Support offline mode on Android mobile](https://features.jellyfin.org/posts/218/support-offline-mode-on-android-mobile) | 1820; started; 2019-07-30 | Plezy managed downloads | Implemented offline subsystem; phones unaccepted [D] | M | Highest-priority acceptance, not a new feature |
| 2 | [Add an option to remove an item from "Continue Watching"](https://features.jellyfin.org/posts/517/add-an-option-to-remove-an-item-from-continue-watching) | 1727; planned; 2020-03-26 | Plex/Emby via Plezy capabilities | Implemented independent dismissal [PS] | S | Keep progress intact; advertise after acceptance |
| 3 | [Watchlist (like Netflix)](https://features.jellyfin.org/posts/576/watchlist-like-netflix) | 1295; planned; 2020-04-27 | Plezy trackers; Plex watchlist | Implemented My List including books/shows [PS] | S | Naming deliberate; keep it easy to reach |
| 4 | [Support for OIDC / OAuth SSO](https://features.jellyfin.org/posts/230/support-for-oidc-oauth-sso) | 1198; planned; 2019-08-03 | Kyoo OIDC; JF plugins | Apple/Google Hosted SSO; arbitrary owner OIDC not located [ID] | L | Strong homelab demand; optional standards adapter |
| 5 | [Remove pagination/use lazy loading for library view](https://features.jellyfin.org/posts/216/remove-pagination-use-lazy-loading-for-library-view) | 1180; started; 2019-07-29 | Plezy lazy browse | Paged/windowed UI already present [UI] | M | Bound server work as well as rendering |
| 6 | [Add support for two factor authentication (2FA)](https://features.jellyfin.org/posts/26/add-support-for-two-factor-authentication-2fa) | 1104; planned; 2019-07-29 | Portico local/Hosted | TOTP source present [ID] | M | Accept recovery and TV second-factor paths |
| 7 | [Pre-transcoding](https://features.jellyfin.org/posts/570/pre-transcoding) | 979; open; 2020-04-24 | Portico; Plex optimize family | Prepared versions implemented [PREP] | M | Promote for weak hardware; disk budgets matter |
| 8 | [Audiobook support](https://features.jellyfin.org/posts/243/audiobook-support) | 858; started; 2019-08-11 | Audiobookshelf; Plezy audio | Books/audiobook listening implemented [AUDIO] | M | Complete chapters/progress rather than generic relabelling |
| 9 | [Watched History](https://features.jellyfin.org/posts/633/watched-history) | 830; planned; 2020-05-23 | Plex; Portico | Personal and owner history implemented [HIST] | M | Preserve privacy distinction and import history |
| 10 | [Offline Sync Feature](https://features.jellyfin.org/posts/1341/offline-sync-feature) | 818; planned; 2021-10-08 | Plezy offline sync | Download/reconcile model present; acceptance unknown [D] | L | Offline conflicts are the hard part |
| 11 | [MySQL server back end](https://features.jellyfin.org/posts/315/mysql-server-back-end) | 664; started; 2019-11-05 | Kyoo PostgreSQL | No MySQL/Postgres server backend located; deliberate SQLite [DB] | L | Keep SQLite unless evidence demands another engine |
| 12 | [Gapless Playback](https://features.jellyfin.org/posts/181/gapless-playback) | 649; open; 2019-07-29 | Plezy/Navidrome clients | Gapless implementation present; physical acceptance unknown [AUDIO] | M | Perceptual transitions are launch quality |
| 13 | [Add support for Dynamic (Smart) Playlists](https://features.jellyfin.org/posts/49/add-support-for-dynamic-smart-playlists) | 588; planned; 2019-07-29 | Navidrome smart playlist ecosystem | Saved views present; dynamic playable playlist not located [SAVED] | M | High-value extension of existing query model |
| 14 | [Trailers Plugin](https://features.jellyfin.org/posts/55/trailers-plugin) | 585; open; 2019-07-29 | Plezy extras | Local trailer/extras shape present; marketplace plugin not located [EXTRA] | M | Local extras first; external fetch is separate |
| 15 | [Temporary direct file sharing links](https://features.jellyfin.org/posts/72/temporary-direct-file-sharing-links) | 581; open; 2019-07-29 | Request, not JF completion evidence | Anonymous expiring media-share links not located [G-sharelinks] | M | Decide guest authority before adding |
| 16 | [Podcast Support](https://features.jellyfin.org/posts/48/podcast-support) | 565; open; 2019-07-29 | Audiobookshelf | Podcast ingestion/subscription not located [G-podcasts] | L | Worth a deliberate listening roadmap, optional at launch |
| 17 | [Support download of transcoded files](https://features.jellyfin.org/posts/57/support-download-of-transcoded-files) | 520; planned; 2019-07-29 | Plezy quality/version downloads | Prepared/conversion download options present [D] | M | Preserve exact version and requested quality |
| 18 | [Jellyfin Apple TV Support](https://features.jellyfin.org/posts/612/jellyfin-apple-tv-support) | 464; started; 2020-05-11 | Swiftfin; Infuse; Plezy | Apple TV source present; distribution acceptance unknown [UI] | L | A household gate, not solved by a server release |
| 19 | [Jellyfin client  Vidaa OS](https://features.jellyfin.org/posts/1615/jellyfin-client-vidaa-os) | 457; open; 2022-05-17 | JF request only | VIDAA app not located [G-vida] | L | Defer unless target households supply devices |
| 20 | [Personal ratings](https://features.jellyfin.org/posts/1134/personal-ratings) | 455; planned; 2021-04-11 | Plex numeric; Plezy backend-dependent | Half-star playable ratings; containers lack rating field [PS] | M | Finish container parity; preserve distinct favourites |
| 21 | [Add a folder view to libraries](https://features.jellyfin.org/posts/224/add-a-folder-view-to-libraries) | 440; open; 2019-07-31 | Plezy folder playback; JF | Owner source inventory; viewer filesystem folder browser not located [G-folderbrowse] | M | Valuable for home media; do not leak owner paths |
| 22 | [Automatically test hardware transcoding](https://features.jellyfin.org/posts/450/automatically-test-hardware-transcoding) | 424; planned; 2020-02-16 | JF request; Portico probe | Hardware detection/probe surfaces present; sample encode acceptance unknown [HW] | M | A real short encode test beats a capability label |
| 23 | [Option to kill a stream from a user](https://features.jellyfin.org/posts/301/option-to-kill-a-stream-from-a-user) | 402; open; 2019-10-16 | Portico; other admin systems vary | Admin session terminate route present [STOP] | S | Accept terminal-stop behaviour; no auto-restart |
| 24 | [IPTV Channel Groupings](https://features.jellyfin.org/posts/186/iptv-channel-groupings) | 377; planned; 2019-07-29 | Plezy grouped guide | Channel group-name projection/filter implemented; UI acceptance unknown [LIVE] | M | Verify provider categories and group persistence |
| 25 | [Allow Jellyfin to display missing episodes and series](https://features.jellyfin.org/posts/572/allow-jellyfin-to-display-missing-episodes-and-series) | 344; started; 2020-04-26 | JF partial; 12 plugin expansion | Missing/unaired provider placeholders not located [G-missingEpisodes] | M | Show separately from actual playable catalogue |
| 26 | ["Keep Original Title"-option.](https://features.jellyfin.org/posts/32/keep-original-title-option) | 317; open; 2019-07-29 | JF request | Original-title fact/display present; prefer-title setting unclear [TITLE] | S | Avoid destructive canonical-title replacement |
| 27 | [SyncPlay "invite to watch" link](https://features.jellyfin.org/posts/971/syncplay-invite-to-watch-link) | 267; open; 2020-12-07 | Plezy room code | Group invite code present; external deep-link completion unknown [SOC] | M | Usable invite links are worth completing |
| 28 | [Better Handling of Movie Versions in Multiple Directories](https://features.jellyfin.org/posts/174/better-handling-of-movie-versions-in-multiple-directories) | 266; open; 2019-07-29 | JF movies; Plezy switches | Explicit version association; automatic cross-directory merge incomplete [INV] | L | Keep identity conservative; add reviewable merge |
| 29 | [Allow global defaults](https://features.jellyfin.org/posts/469/allow-global-defaults) | 259; open; 2020-02-27 | JF request | Per-viewer defaults exist; global enforced defaults not established [PREF] | M | Distinguish defaults from enforced policy |
| 30 | [Search by Tag/Genre](https://features.jellyfin.org/posts/276/search-by-tag-genre) | 256; open; 2019-09-14 | JF clickable tags/filters | Title token-prefix search; browse genre/tags; free-text genre search uncertain [SEARCH] | M | Expose filters clearly; no unsupported global claim |
| 31 | [Samsung TV Support](https://features.jellyfin.org/posts/335/samsung-tv-support) | 253; started; 2019-11-22 | JF Tizen review; Plex/Emby | Tizen profile exists; installable app not established [TV] | L | Store/platform work exceeds a codec profile |
| 32 | [Add images by URL](https://features.jellyfin.org/posts/270/add-images-by-url) | 252; open; 2019-09-04 | JF request | Artwork edit/preview exists; remote URL import not established [META] | M | Nice convenience with strict fetch boundary |
| 33 | [Support Google Assistant and Alexa](https://features.jellyfin.org/posts/275/support-google-assistant-and-alexa) | 243; open; 2019-09-13 | Requested integrations | System dictation exists; Alexa/Assistant integration not located [VOICE] | M | Defer pending specific household workflows |
| 34 | [Better internet streaming using multiple parallel connections (or threads)](https://features.jellyfin.org/posts/2171/better-internet-streaming-using-multiple-parallel-connections-or-threads) | 231; planned; 2023-05-10 | JF request | Parallel multi-connection media transfer not established [NET] | L | Measure stalls first; avoid speculative network complexity |
| 35 | [Create a sign up page for new users](https://features.jellyfin.org/posts/494/create-a-sign-up-page-for-new-users) | 227; open; 2020-03-14 | Hosted registration differs | Invited/local profiles; public self-signup not established [ID] | M | Preserve owner admission; request link could help |
| 36 | [Quick Connect with QR code](https://features.jellyfin.org/posts/2637/quick-connect-with-qr-code) | 227; planned; 2024-04-21 | Portico; JF request | TV QR/code UI implemented [QR] | S | High-value sign-in acceptance |
| 37 | [When "Play next episode automatically" is selected, begin buffering the next episode](https://features.jellyfin.org/posts/400/when-play-next-episode-automatically-is-selected-begin-buffering-the-next-episode) | 222; open; 2020-01-15 | JF request | Audio prebuffer present; video successor prebuffer not established [AUDIO] | M | Trade startup latency against wasted conversion work |
| 38 | [Download a batch not only single media file](https://features.jellyfin.org/posts/655/download-a-batch-not-only-single-media-file) | 219; started; 2020-06-02 | Plezy season/album downloads | Batch download preparation types present [D] | M | Make batch retry and partial failures understandable |
| 39 | [Per user system language](https://features.jellyfin.org/posts/370/per-user-system-language) | 219; open; 2019-12-13 | Plezy 21 translations plus English | Locale/preferences scaffold; English launch deliberate [I18N] | L | Translation and RTL need real acceptance |
| 40 | [Please add "stats for nerds" to the Android TV app](https://features.jellyfin.org/posts/272/please-add-stats-for-nerds-to-the-android-tv-app) | 219; planned; 2019-09-09 | Plezy overlay; JF request | Playback info present; Apple buffer/frame details known gap [STATS] | M | Diagnostics should be cheap and actionable |
| 41 | [Metadata in multiple languages](https://features.jellyfin.org/posts/610/metadata-in-multiple-languages) | 212; open; 2020-05-10 | JF request | Library metadata language present; simultaneous multilingual variants uncertain [META] | L | Distinct from translating UI strings |
| 42 | [Android Support for FLAC Playback](https://features.jellyfin.org/posts/257/android-support-for-flac-playback) | 210; open; 2019-08-26 | Plezy/Finamp/Navidrome clients | FLAC native/audio plan support present [AUDIO] | M | Accept format/seek behaviour on actual Android |
| 43 | [Give the option to combine next up and continue watching](https://features.jellyfin.org/posts/1055/give-the-option-to-combine-next-up-and-continue-watching) | 203; open; 2021-02-16 | Plex-like unified journey | Composed continue/show projections exist; exact dedupe acceptance unknown [HOME] | M | One clear resume/next action reduces confusion |
| 44 | [Add user group support (was: users within users)](https://features.jellyfin.org/posts/493/add-user-group-support-was-users-within-users) | 203; open; 2020-03-13 | JF request | Local/subordinate profiles exist; reusable permission-group abstraction not located [G-groups] | M | Policy templates before complex group hierarchy |
| 45 | [IMBD Rating and Rotten Tomatoes Audiance Rating and Fresh rating on Movies and TV Shows](https://features.jellyfin.org/posts/463/imbd-rating-and-rotten-tomatoes-audiance-rating-and-fresh-rating-on-movies-and-tv-shows) | 196; open; 2020-02-25 | Plex rating sources; Plezy displays | Rating provider fields/render labels present; provider acquisition coverage uncertain [META] | M | Respect licensing and provider availability |
| 46 | [Hide TV shows with no video files in folders](https://features.jellyfin.org/posts/41/hide-tv-shows-with-no-video-files-in-folders) | 191; planned; 2019-07-29 | JF planned | Availability derivation present; exact empty-show presentation uncertain [INV] | S | Do not confuse empty with temporarily unavailable |
| 47 | [Improve Subtitle Appearance/Options](https://features.jellyfin.org/posts/161/improve-subtitle-appearance-options) | 188; open; 2019-07-29 | Plezy ASS/styling | Subtitle policy/style surfaces partial; web-only styling row [SUB] | M | Readability and TV parity matter |
| 48 | [Editing Metadata for multiple items at once](https://features.jellyfin.org/posts/144/editing-metadata-for-multiple-items-at-once) | 186; open; 2019-07-29 | Portico; requested JF | Bulk metadata edit service present [META] | M | Ensure partial conflicts and lock preservation |
| 49 | [Subtile offset on android TV](https://features.jellyfin.org/posts/3036/subtile-offset-on-android-tv) | 180; open; 2024-12-20 | Plezy offset; JF TV request | Session subtitle offset present [SUB] | M | Accept renderer application, not just persisted value |
| 50 | [Multiple Servers Support](https://features.jellyfin.org/posts/47/multiple-servers-support) | 178; open; 2019-07-29 | Plezy multi-backend | Known servers/directory implemented; aggregate library not established [MULTI] | M | Server selection is already useful |
| 51 | [Add Option to Flatten Single-Season TV Shows (Skip Season Level)](https://features.jellyfin.org/posts/3352/add-option-to-flatten-single-season-tv-shows-skip-season-level) | 174; open; 2025-06-12 | Plezy navigation; JF request | Hide-seasons settings present; conditional single-season auto-flatten unclear [SHOW] | S | Easy control; don't guess every show structure |
| 52 | [Filter videos by audio stream language](https://features.jellyfin.org/posts/39/filter-videos-by-audio-stream-language) | 172; open; 2019-07-29 | JF 12 language filters | Indexed audioLanguage browse predicate and facet present [SEARCH] | M | Useful before download/play selection |
| 53 | [cuesheet support](https://features.jellyfin.org/posts/1190/cuesheet-support) | 171; open; 2021-05-18 | JF request | Music CUE track ingestion not located; subtitle cues are unrelated [G-cuesheet] | L | Niche but foundational audio identity work |
| 54 | [New or Improved Home Secreen Sections](https://features.jellyfin.org/posts/1986/new-or-improved-home-secreen-sections) | 170; planned; 2023-02-11 | Plex; Plezy hubs | Rich personal/discover Home rows implemented [HOME] | M | Already a potential differentiator |
| 55 | [Jellyfin VR Oculus Quest](https://features.jellyfin.org/posts/891/jellyfin-vr-oculus-quest) | 167; open; 2020-10-15 | JF request | VR/Quest client not established [G-vr] | L | Low launch priority absent target hardware |
| 56 | [Calibre Library Access Plugin](https://features.jellyfin.org/posts/219/calibre-library-access-plugin) | 165; open; 2019-07-30 | Calibre/OPDS peers | Calibre tag reading present; live Calibre library integration not located [G-calibre] | M | OPDS may be a cheaper useful boundary |
| 57 | [watch together (watch with multiples accounts)](https://features.jellyfin.org/posts/389/watch-together-watch-with-multiples-accounts) | 164; open; 2020-01-05 | JF request differs from SyncPlay | Watch Together participants; same-screen co-watch attribution not established [SOC] | M | Model multiple viewers without fake devices |
| 58 | [Flattening TV Shows](https://features.jellyfin.org/posts/8/flattening-tv-shows) | 161; open; 2019-07-29 | JF/Plezy variants | Hide seasons/absolute numbering present [SHOW] | S | Keep per-show control |
| 59 | [Multiple account switch](https://features.jellyfin.org/posts/2348/multiple-account-switch) | 156; started; 2023-10-04 | Plezy profiles; JF client-specific | Viewer-selection/remembered accounts present; TV PIN acceptance unknown [ID] | M | Household adoption gate |
| 60 | [Collection Sort Options](https://features.jellyfin.org/posts/1577/collection-sort-options) | 148; open; 2022-04-25 | Plezy/JF collections | Ordered normalized collections; per-collection sort controls uncertain [SAVED] | M | Preserve curator order and deterministic defaults |
| 61 | [Add 4k identifier to media](https://features.jellyfin.org/posts/1738/add-4k-identifier-to-media) | 146; open; 2022-08-24 | Plezy badges | Resolution facts and card-caption model present [TITLE] | S | Don't imply HDR from resolution alone |
| 62 | [Backup and restore](https://features.jellyfin.org/posts/1603/backup-and-restore) | 143; started; 2022-05-10 | JF 10.11; Portico | Backup/restore present [BACK] | M | Restore acceptance has more value than another backup button |
| 63 | [Audio crossfading between tracks](https://features.jellyfin.org/posts/140/audio-crossfading-between-tracks) | 139; open; 2019-07-29 | Plezy/audio specialist clients | Crossfade implementation present [AUDIO] | M | Ensure gapless and crossfade modes stay distinct |
| 64 | [Filters and Category more friendly/ Polarized  user, and add more options.](https://features.jellyfin.org/posts/462/filters-and-category-more-friendly-polarized-user-and-add-more-options) | 138; open; 2020-02-25 | Plezy multi-value filters | Typed browse filter tree present; show facet gaps known [SEARCH] | M | Finish media-kind parity; preserve correct totals |
| 65 | [Duplicate finder](https://features.jellyfin.org/posts/31/duplicate-finder) | 135; open; 2019-07-29 | JF request | Duplicate finder not located [G-duplicatefinder] | M | Review-only duplicate report before any merge/delete |
| 66 | [Option to make “Recently Added” series items open the full series page](https://features.jellyfin.org/posts/4021/option-to-make-recently-added-series-items-open-the-full-series-page) | 132; open; 2026-07-07 | JF web bug/request | Recently aired one-show projection documented; Recently Added click target uncertain [HOME] | S | Consistent show target plus explicit episode play |
| 67 | [Limit selectable video quality server-side](https://features.jellyfin.org/posts/431/limit-selectable-video-quality-server-side) | 131; open; 2020-02-07 | JF request | Delivery settings/admin bitrate rejection present; exact user-quality ladder policy unclear [HW] | M | Owner cap and automatic load downgrade are different |
| 68 | [Let the user hide a movie or tv show](https://features.jellyfin.org/posts/1072/let-the-user-hide-a-movie-or-tv-show) | 131; open; 2021-02-28 | JF request | Not Interested affects recommendations; hide-from-library not located [PS] | M | Do not silently repurpose recommendation feedback |
| 69 | [Default user settings on user creation](https://features.jellyfin.org/posts/363/default-user-settings-on-user-creation) | 130; open; 2019-12-10 | JF request | Client defaults exist; admin new-user template not established [PREF] | M | Useful low-ceremony household onboarding |
| 70 | [Show video preview on thumbnail hover](https://features.jellyfin.org/posts/139/show-video-preview-on-thumbnail-hover) | 128; open; 2019-07-29 | JF request | Hover motion preview not located; trickplay is separate [G-hoverpreview] | M | Optional; preserve reduced motion/data usage |
| 71 | [Audio Volume : Ability to increase volume to more than 100%](https://features.jellyfin.org/posts/150/audio-volume-ability-to-increase-volume-to-more-than-100) | 125; open; 2019-07-29 | Plezy downmix/normalization | Gain/preamp audio plan; general >100% video volume UI uncertain [AUDIO] | S | Offer clear clipping-safe controls |
| 72 | [Non Seasonal/One running season option](https://features.jellyfin.org/posts/332/non-seasonal-one-running-season-option) | 124; open; 2019-11-22 | Portico; JF request | Hide seasons/absolute/ranges present [SHOW] | S | Keep useful anime structure controls |
| 73 | [Add movie settings (plex like feature)](https://features.jellyfin.org/posts/459/add-movie-settings-plex-like-feature) | 122; planned; 2020-02-24 | Plezy pre-play selectors | Playback options/renditions before selecting offer present [PLAY] | M | Avoid needless initial transcode |
| 74 | [A richer ePub reader. (Also pdf support)](https://features.jellyfin.org/posts/792/a-richer-epub-reader-also-pdf-support) | 121; started; 2020-08-09 | Stump; JF 12 readers | Audiobooks present; ePub annotations/reader not located [G-reader] | L | Separate book listening from reading scope |
| 75 | [Avoid spoilers for unwatched items](https://features.jellyfin.org/posts/137/avoid-spoilers-for-unwatched-items) | 118; open; 2019-07-29 | JF request | Viewer spoiler-hiding policy not located [G-spoilers] | M | Good low-risk UX if restriction-aware |
| 76 | [Are you still watching?](https://features.jellyfin.org/posts/3439/are-you-still-watching) | 111; open; 2025-08-23 | JF Android TV 0.19/web12; Plezy | Post-play still-watching mode present [POST] | M | Accept timeout/resume and accessibility |
| 77 | [Double tap to fast forward or rewind](https://features.jellyfin.org/posts/131/double-tap-to-fast-forward-or-rewind) | 109; open; 2019-07-29 | JF request | Double-tap video seek gesture not established [G-doubletap] | S | Small optional mobile usability improvement |
| 78 | [Setting to only allow 60 seconds (or entered value) of transcoding](https://features.jellyfin.org/posts/474/setting-to-only-allow-60-seconds-or-entered-value-of-transcoding) | 106; started; 2020-02-29 | JF throttle request | Bounded HLS window/producer design present [PLAY] | M | Prevent filling disk for abandoned streams |
| 79 | [Seamless access to other jellyfin servers](https://features.jellyfin.org/posts/716/seamless-access-to-other-jellyfin-servers) | 105; open; 2020-07-06 | Plezy aggregates connected servers | Per-server identities retained; integrated federation not located [MULTI] | L | Federation is a separate explicit scope decision |
| 80 | [Add an option to tag movies as being part of a TV show](https://features.jellyfin.org/posts/2209/add-an-option-to-tag-movies-as-being-part-of-a-tv-show) | 105; open; 2023-06-21 | JF request | Movie-in-series chronology/link feature not established [G-tvfilm] | M | Useful anime/franchise support, not launch-critical |
| 81 | [Chromecast support on iOS](https://features.jellyfin.org/posts/466/chromecast-support-on-ios) | 105; open; 2020-02-26 | Swiftfin request; Portico | Cast native bridge/source exists; physical acceptance open [CAST] | M | Do one complete physical connection/seek/reconnect journey |
| 82 | [Display movie version like Extended, Director's Cut etc.](https://features.jellyfin.org/posts/2537/display-movie-version-like-extended-directors-cut-etc) | 104; open; 2024-01-14 | Plezy version details; JF request | Version/files UI types present; edition provenance/labels incomplete [PLAY] | M | Label cuts separately from technical encodes |
| 83 | [Dynamic (Smart) Collections defined by custom queries](https://features.jellyfin.org/posts/593/dynamic-smart-collections-defined-by-custom-queries) | 104; open; 2020-05-01 | Plex ecosystem; JF request | Saved query views exist; maintained smart collection not located [SAVED] | M | Reuse typed query AST, not arbitrary SQL |
| 84 | [Add Chat to SyncPlay](https://features.jellyfin.org/posts/740/add-chat-to-syncplay) | 104; open; 2020-07-20 | JF request | Group chat not located [G-chat] | M | Control sync is separate from communications |
| 85 | [Volume setting for Theme Songs](https://features.jellyfin.org/posts/1749/volume-setting-for-theme-songs) | 103; open; 2022-09-05 | Plezy open request | Theme-song feature/volume not located [G-theme] | S | Optional atmosphere; never autoplay without choice |
| 86 | [Add filter to identify "unmatched" titles](https://features.jellyfin.org/posts/665/add-filter-to-identify-unmatched-titles) | 101; open; 2020-06-07 | Plezy/JF request | Metadata review/unmatched owner surfaces present [META] | S | Complete fixes and explain why matching failed |
| 87 | [Add options for real-time upscaling](https://features.jellyfin.org/posts/729/add-options-for-real-time-upscaling) | 100; open; 2020-07-14 | Plezy mpv shaders | Real-time video upscaling shader feature not located [G-upscaling] | L | Power-user optional; hardware/support costs substantial |
| 88 | [Naming parity with Plex](https://features.jellyfin.org/posts/2560/naming-parity-with-plex) | 99; open; 2024-01-28 | JF request; Kyoo naming goal | Filename parser exists; Plex naming compatibility not exhaustively established [INV] | M | Migration fixtures are worth more than assertions |
| 89 | [Add Sonic Analysis Support for Music Library (similar to Plexamp)](https://features.jellyfin.org/posts/3245/add-sonic-analysis-support-for-music-library-similar-to-plexamp) | 98; open; 2025-04-07 | Plexamp; specialised ecosystem | Sonic/listening-similarity model not located; backlog music mixes [G-sonicanalysis] | L | Strong music opportunity after basic listening acceptance |
| 90 | [Adjust audiobook playback speed](https://features.jellyfin.org/posts/590/adjust-audiobook-playback-speed) | 96; open; 2020-04-30 | Plezy/Audiobookshelf | Audiobook speed native/shared controls present [AUDIO] | S | Keep pitch/route correctness |
| 91 | [Support for edl/comskip files](https://features.jellyfin.org/posts/180/support-for-edl-comskip-files) | 96; open; 2019-07-29 | JF request | Commercial marker kind exists; EDL/comskip importer not located [G-comskip] | M | Import is distinct from detecting or skipping markers |
| 92 | [Set second preferred language for subtitles](https://features.jellyfin.org/posts/666/set-second-preferred-language-for-subtitles) | 95; open; 2020-06-07 | JF request; Plezy choices | Subtitle preference system; ordered second-language fallback not established [SUB] | M | Worth explicit fallback order, not random default track |
| 93 | [Rebranding](https://features.jellyfin.org/posts/1919/rebranding) | 95; started; 2023-01-11 | JF branding/custom CSS | Server naming exists; arbitrary client branding/theme system not established [G-branding] | M | Defer if it fragments support and design |
| 94 | [Modular homescreen](https://features.jellyfin.org/posts/1554/modular-homescreen) | 93; open; 2022-04-08 | Plex/Plezy; JF request | Home layout/reorder/hide/saved rows present [HOME] | S | Useful owner/viewer choice already present |
| 95 | [Add a scheduled task for backups](https://features.jellyfin.org/posts/3546/add-a-scheduled-task-for-backups) | 91; planned; 2025-10-25 | JF scheduled-task request | Backup schedule implemented [BACK] | S | High-value operational default; verify real retention |
| 96 | [Gracefully fallback on software encoding when hardware encoding crashes](https://features.jellyfin.org/posts/318/gracefully-fallback-on-software-encoding-when-hardware-encoding-crashes) | 91; planned; 2019-11-07 | JF failover request | Hardware-failure software fallback documented/source routes [HW] | M | Accept original requested quality and load consequences |
| 97 | [Photo timeline](https://features.jellyfin.org/posts/596/photo-timeline) | 91; open; 2020-05-01 | Immich | Photo timeline not located [G-phototimeline] | L | Avoid diluting media launch; reserve kind extension |
| 98 | [Fuzzy search to Jellyfin](https://features.jellyfin.org/posts/3703/fuzzy-search-to-jellyfin) | 90; open; 2026-01-07 | Kyoo trigrams; JF request | Token-prefix search; fuzzy matcher not located [SEARCH] | M | Helpful bounded suggestions, after exact query correctness |
| 99 | [Media codec, resolution, tags and audio flags](https://features.jellyfin.org/posts/1015/media-codec-resolution-tags-and-audio-flags) | 89; open; 2021-01-22 | Plezy detailed File Info | Stream facts/file info/playback reason shape present [STATS] | S | Useful to power users and support |
| 100 | [Message to all users](https://features.jellyfin.org/posts/1535/message-to-all-users) | 89; open; 2022-03-22 | JF request | Notification broadcast API/type present [NOTIFY] | S | In-app only deliberate; check effective audience |
| 101 | [Share playlists](https://features.jellyfin.org/posts/173/share-playlists) | 89; open; 2019-07-29 | JF ACL work; Portico | Named viewer/editor playlist shares implemented [SAVED] | M | Respect each viewer's library restrictions |
| 102 | [Add genres and recommendations to home screen](https://features.jellyfin.org/posts/3501/add-genres-and-recommendations-to-home-screen) | 88; open; 2025-10-01 | Plex; Plezy hubs | Genre/personal recommendation Home families present [HOME] | S | Already a strong potential adoption claim |
| 103 | [Group shows into collections](https://features.jellyfin.org/posts/333/group-shows-into-collections) | 87; open; 2019-11-22 | JF 12 collections; Plezy | Normalized collection members exist; show-container membership UX uncertain [SAVED] | M | Prefer explicit curated grouping |
| 104 | [Anonymous, guest access](https://features.jellyfin.org/posts/1313/anonymous-guest-access) | 87; open; 2021-09-14 | JF request | Guest independent anonymous authority not located [G-guest] | M | Decide privacy/expiration first |
| 105 | [Local Recommendation Engine for Jellyfin](https://features.jellyfin.org/posts/2737/local-recommendation-engine-for-jellyfin) | 86; open; 2024-06-15 | Portico; JF plugin/proposal | Local content/taste recommendation engine present [REC] | M | Verify discovery quality; no cloud library upload |
| 106 | [Local .actors folder support for thumbnails](https://features.jellyfin.org/posts/770/local-actors-folder-support-for-thumbnails) | 86; open; 2020-07-30 | Kodi-style request | Local metadata sidecars; .actors portrait import not located [G-actorsfolder] | S | Good curated-library interoperability if safe |
| 107 | [Limit the number of simultaneous streams per user](https://features.jellyfin.org/posts/1444/limit-the-number-of-simultaneous-streams-per-user) | 85; open; 2022-01-05 | JF per-user policy, more control requested | Global conversion limits and per-account MaxStreams admission implemented; exact per-user conversion-only quota unknown [LIMIT] | M | Prevent one user monopolizing conversion without quality changes |
| 108 | [Better control of transcoding and downloading options](https://features.jellyfin.org/posts/1881/better-control-of-transcoding-and-downloading-options) | 85; open; 2022-12-21 | JF request; Plezy options | Per-account stream/remote-bitrate controls and delivery/download policy exist; finer conversion/download knobs uncertain [LIMIT] | M | Prefer explainable permission/policy knobs |
| 109 | [Default Library Order](https://features.jellyfin.org/posts/1359/default-library-order) | 84; open; 2021-10-28 | JF request | Home/library authored order and preferences; owner default library order unclear [HOME] | S | Small household onboarding convenience |
| 110 | [Aggregate multiple servers in client UI](https://features.jellyfin.org/posts/407/aggregate-multiple-servers-in-client-ui) | 83; open; 2020-01-17 | Plezy unified browsing | Server selector present; merged multi-server catalogue not located [MULTI] | L | Only pursue with explicit latency/privacy semantics |
| 111 | [Cluster Support](https://features.jellyfin.org/posts/873/cluster-support) | 83; open; 2020-10-02 | Kyoo multi-service; replicas WIP | No cluster deployment backend located; deliberate one process/SQLite [DB] | L | Out of 1.0 scope unless scale evidence forces it |
| 112 | [More intuitive play/resume when clicking on a TV show/movie](https://features.jellyfin.org/posts/331/more-intuitive-play-resume-when-clicking-on-a-tv-show-movie) | 83; open; 2019-11-22 | Plex/Plezy/JF bugs | Show next-up projection present; every play entry point unverified [SHOW] | M | High-value regression acceptance |
| 113 | [Raspberry Pi client image](https://features.jellyfin.org/posts/222/raspberry-pi-client-image) | 83; planned; 2019-07-30 | JF request | Dedicated Pi appliance client image not located [G-pi] | M | Distribution work; generic web/client may suffice |
| 114 | [Custom Carousel Options on Home Screen](https://features.jellyfin.org/posts/1439/custom-carousel-options-on-home-screen) | 82; open; 2021-12-31 | Plex; JF request | Home row customization and saved query rows implemented [HOME] | S | Avoid whole-library per-load computation |
| 115 | [Local social features](https://features.jellyfin.org/posts/349/local-social-features) | 79; open; 2019-12-05 | JF request | Owner-controlled opt-in shared-activity recommendations present; social graph excluded [REC] | M | Keep privacy explicit; don't add social obligations |
| 116 | [Make loading animation / Display loading percentage](https://features.jellyfin.org/posts/711/make-loading-animation-display-loading-percentage) | 79; open; 2020-07-03 | JF startup UI; Plezy retry | Phase/status/building/error models present; per-resource percent varies [INV] | M | Honest stage progress beats invented percentages |
| 117 | [Photo album feature](https://features.jellyfin.org/posts/406/photo-album-feature) | 79; open; 2020-01-17 | Immich sharing | Photo album collaboration not located [G-photos] | L | Separate photo product scope |
| 118 | [Make a UI to see all actors](https://features.jellyfin.org/posts/1187/make-a-ui-to-see-all-actors) | 78; open; 2021-05-15 | JF request; Plezy people search | People search/credit pages; all-actors browse destination uncertain [SEARCH] | M | Useful query surface; apply restrictions |
| 119 | [Display all episode titles when multiple episodes are stored in one file](https://features.jellyfin.org/posts/1048/display-all-episode-titles-when-multiple-episodes-are-stored-in-one-file) | 77; open; 2021-02-13 | JF request | Multi-episode file display/segmentation not established [G-multiEpisode] | M | Identity/timeline issue, not only a label |
| 120 | [CinemaMode Plugin](https://features.jellyfin.org/posts/56/cinemamode-plugin) | 77; open; 2019-07-29 | Plezy/Plex requests | Local extras types; Cinema Mode/preroll feature not located [G-preroll] | M | Optional curated experience after playback stability |
### Portico evidence-key registry for the demand table

All short paths in this registry are prefixed with `apps/portico-server/` unless stated otherwise. These references document actual source surfaces, not passing acceptance tests.

| Key | Source evidence and interpretation |
|---|---|
| PS | `server/internal/catalog/personal.go:66` (independent saved/rating fields), `:73` (continue dismissal keyed to playback), `:267` (half-star validation); `container_personal.go:19`, `:54` (container flags, book My List). |
| D | `server/internal/downloads/preparations.go:20`, `:29`, `:40` (per-target visibility, idempotent batch submission, foreground transfer); `packages/client-core/src/downloads.ts:35`, `:373`; `apps/portico-react-native/apps/native/Native/PorticoDownloads.m:8`. |
| PREP | `server/internal/httpapi/prepared_media.go:88`; `packages/client-core/src/prepared-media.ts:105`, `:120` (prepared versions and revision-fenced deletion). |
| ID | `server/internal/identity/two_factor.go:31`; `direct_profiles.go:508`; `packages/client-core/src/identity-devices.ts:88`; native `src/app/session.tsx:7` and `src/bridge/profile-trust.ts:2`; architecture 02/07 for local versus Hosted identity. Generic LDAP/OIDC was searched; Apple/Google Hosted OIDC is not an arbitrary owner-configured provider. |
| DB | `server/internal/dbwork/gate.go:19–26`, `write.go`; `server/internal/persistence/README.md:13–47`. MySQL/PostgreSQL strings were absent in the audited server/shared/native scopes; Hosted PostgreSQL is a different service. |
| UI | Native `src/ui/Grid.tsx:73`, `src/screens/Person.tsx:14`; `Project Architecture/06 Clients.md`, plus actual Android native source. Presence is not a shipping storefront. |
| AUDIO | Native `Native/PorticoAudioRender.m:21`, `Native/PorticoListeningOwner.h:23`, `PorticoListeningOwner.m:744`; `packages/client-core/src/playback-v1/audio-render.ts:106`; architecture 04 audio plan. |
| HIST | `server/internal/catalog/play_history.go:18`; `server/internal/identity/direct_profiles.go:508`; `Project Architecture/02 Server.md` owner history policy. |
| SAVED | `catalog/playlist_types.go:13–31`; `persistence/migrations/0001_baseline.sql:218–260`, `:1294`; `catalog/home_views.go`; `packages/client-core/src/saved.ts:370`. No dynamic-query playlist definition was found in the playlist model. |
| EXTRA | `packages/client-core/src/detail.ts:8` local trailer/featurette/deleted-scene types; `presentation/card-shape.ts:12`. Online marketplace and automatic prerolls are different features. |
| HW | `playback/delivery_decision.go:159`; `delivery_plan.go:453`, `:652`, `:939`; `httpapi/delivery_settings_adapter.go:39`; `packages/client-core/src/telemetry.ts:17`. |
| STOP | `server/internal/httpapi/playback_v1_routes.go:151–172`: admin session terminate with optional message, idempotent. |
| LIVE | `server/internal/livechannels/directory.go:138–139`: `group_name` search and exact group filter; channel source/group projections in the same file. |
| TITLE | `packages/client-core/src/detail.ts:37`, `presentation/title-page.ts`; native `src/screens/Show.tsx:6`; `catalog/browse_contract.go:134–138` resolution and dynamic range. |
| SOC | `server/internal/social/invites.go:178`; `transport.go:481–485`; `store.go:94`; `packages/client-core/src/group-session.ts:88`; social contract defines server scope and explicit exclusions. |
| INV | `server/internal/ingestion/INVENTORY_CONTRACT.md:20–35`; `ingestion/incremental.go:47`, `inventory.go:248–343`; `storage/inventory_page.go:46`, `:280`; version-association contract and file identity. |
| PREF | `packages/client-core/src/music-preferences.ts:32`; native `src/app/preferences.ts:21`; `presentation/settings-structure.ts`. Client defaults were found; owner-enforced/default templates are not thereby proven. |
| SEARCH | `server/internal/catalog/search.go:22`, `:129`, `:209`, `:279`; `browse_contract.go:139–140` and `browse_facets.go:55` establish audio-language filtering. Genre filtering/search text are separate. |
| TV | `packages/client-core/src/client-profile.ts:211` Tizen profile; native `src/screens/LinkTV.tsx:19` platform names. Neither proves a Tizen store package. |
| META | `server/internal/metadata/repair_bulk.go:20–54`; `screen_review.go`; `artwork_preview.go`; `packages/client-core/src/metadata-review.ts:10`, `library-management.ts:70`; local/provider prose in architecture 03 is checked separately. |
| VOICE | Native `Native/PorticoSearchKeyboard.m:7`; Android `PorticoSearchKeyboard.kt:19`. Platform dictation, not a new Alexa skill. |
| NET | Architecture 02/07 networking; no dedicated multi-connection transport parity was established. |
| QR | Native `src/screens/auth/QuickConnect.tsx:3`, `:61–63`. |
| I18N | Architecture 06 shared i18n; `packages/i18n/src/catalog/regional.ts`; localized preference structures. Deliberate English launch, not broad language coverage. |
| STATS | `packages/client-core/src/presentation/playback-info.ts`; native Backlog gaps; `delivery_plan.go:278` detailed stream trace. |
| MULTI | `packages/client-core/src/known-servers.ts:22`; native `src/bridge/known-servers.ts:1`; saved/download contract excludes cross-server aggregation. |
| SHOW | `packages/client-core/src/show-workspace.ts:6`; native `src/screens/Show.tsx:79`; `server/internal/catalog/show_settings.go`. |
| HOME | `server/internal/catalog/home.go:15–30`, `home_views.go`; `packages/client-core/src/home.ts:21`; architecture 03/05 row composition and library tabs. |
| SUB | `server/internal/subtitles/manifest.go:49–103`; `playbackv1/sessions_change.go:105`, `:144`; `packages/client-core/src/presentation/settings-structure.ts:206` web subtitle appearance row. |
| PLAY | `packages/client-core/src/playback-v1/types.ts:49`; `presentation/title-files.ts:7`; `server/internal/playback/hls_timeline.go:15`, `:63`; `playback/hls.go:478`. |
| POST | `packages/client-core/src/post-play.ts:6`; native `src/player/usePlayer.ts:500`; still-watching advance is an implemented mode. |
| BACK | `server/internal/backup/schedule.go:13`, `:25–30`; `copy.go`, `apply.go`. |
| CAST | Native `Native/PorticoCast` modules and receiver directory; architecture 04, Backlog physical Cast acceptance and sidecar track gap. |
| REC | `server/internal/catalog/rec_engine.go:104`, `:498`, `:787`; architecture 05 for maintained facets, opt-in community signals and pending enrichment dataset publication. |
| LIMIT | `server/internal/operations/settings.go:84`, `:135`, `:340`; `httpapi/delivery_settings_adapter.go:39`: global conversion limits. Per-account limits are implemented at `access/limits.go:76–102`, `access/enforcement.go:105–120`, wired by `httpapi/access_admission.go:147`. `playbackv1/admission_gate.go:5–16` serializes same-account admission through session creation; `playback/v1_session_helpers.go:19–37` also checks owner caps. A separate per-user **conversion-only** quota was not established. |
| NOTIFY | `packages/client-core/src/notifications.ts:71`; in-app broadcast result. This is not push/email. |

**Negative/uncertain grep boundary.** Searches used `rg -n -i` across production server `internal`, canonical shared `client-core/src`, web `src`, native `src`/`Native`/Android application source and selected Vega source; tests, generated files, third-party vendoring and build artifacts were excluded. Terms included `smart_playlist|smartPlaylist|playlist.*query`, `shareLink|share_token`, `podcast`, `Vidaa`, `folderView`, `missingEpisodes|unaired`, `cueSheet|cuesheet|\.cue`, `user_group|profile_group`, `duplicateFinder`, `hover.*preview`, `spoiler`, `doubleTap`, `groupChat|chatMessage`, `themeSong|themeMedia`, `sonic|listeningSimilarity`, `comskip|\.edl`, `photoTimeline`, `fuzzy|levenshtein`, `\.actors`, `cluster`, `multiEpisode`, bounded `epub|OPDS|ebook`, `branding|customCSS`, `cinemaMode|pre-roll`, `Raspberry`, `photoAlbum`, `Trakt|Simkl|scrobbl`, `plex.*import|jellyfin.*import`, `externalPlayer|VLC`, `CarPlay|AndroidAuto`, and `secondarySubtitle`. Searches that find incidental platform lists, media-kind exclusions, decoder audio preroll or JSON-array substrings are treated as **not establishing the requested feature**, not zero-result claims. Roku code was not searched. A missing entry could exist in an unreviewed path; these are candidate gaps for Justin's review.

### Completed demand that an active top-100 list would miss

The completion ledger below uses current tracker state, not an inference from age. Status can be stale or cover only one platform. A completion response sometimes points forward to a release rather than proving the feature was already broadly deployed on the response date.

| Feature / request | Votes | Tracker status | What completion really establishes |
|---|---:|---|---|
| [Skip intro/outro/credits feature](https://features.jellyfin.org/posts/45/skip-intro-outro-credits-feature) | 1089 | completed | 10.10 media segments plus provider plugin and client support; not every client has skip UI |
| [Easy Import/Export Jellyfin Settings](https://features.jellyfin.org/posts/299/easy-import-export-jellyfin-settings) | 899 | completed | 10.11 internal backup/restore, not arbitrary cross-OS migration |
| [Add lyrics to songs](https://features.jellyfin.org/posts/285/add-lyrics-to-songs) | 550 | completed | 10.9 server lyrics; client implementation varies |
| [List all collections that a movie belong to in movie details](https://features.jellyfin.org/posts/540/list-all-collections-that-a-movie-belong-to-in-movie-details) | 517 | completed | 12.x collection membership on item detail |
| ["Are you still watching?" prompt](https://features.jellyfin.org/posts/814/are-you-still-watching-prompt) | 496 | completed | Android TV 0.19 prompt; another overlapping request remains open |
| [H265 Playback](https://features.jellyfin.org/posts/701/h265-playback) | 487 | completed | HEVC support is route/device dependent |
| [Allow multiple clients to view a stream](https://features.jellyfin.org/posts/172/allow-multiple-clients-to-view-a-stream) | 387 | completed | SyncPlay exists; client coverage and drift remain separate |
| [Home Assistant integration](https://features.jellyfin.org/posts/27/home-assistant-integration) | 355 | completed | Home Assistant integration, external ecosystem |
| [Remember subtitle and audio track choice between episodes](https://features.jellyfin.org/posts/194/remember-subtitle-and-audio-track-choice-between-episodes) | 335 | completed | Persisted episode audio/subtitle preference, not guaranteed language selection in every player |
| [Multiple versions drop down on TV series](https://features.jellyfin.org/posts/251/multiple-versions-drop-down-on-tv-series) | 276 | completed | 12.0 alternate episode versions, years after movies |
| [iOS app make download option function.](https://features.jellyfin.org/posts/330/ios-app-make-download-option-function) | 249 | completed | Official iOS wrapper 1.7 download, distinct from Swiftfin downloads |
| [WebOS (LG) client](https://features.jellyfin.org/posts/42/webos-lg-client) | 229 | completed | webOS wrapper availability; version/device support varies |
| [Restart buton](https://features.jellyfin.org/posts/339/restart-buton) | 205 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Video timeline scrub thumbnails](https://features.jellyfin.org/posts/160/video-timeline-scrub-thumbnails) | 163 | completed | Trickplay server generation and client previews |
| [HDR playback on Android TV](https://features.jellyfin.org/posts/402/hdr-playback-on-android-tv) | 159 | completed | Compatible Android TV HDR path; not a universal hardware promise |
| [Account based stream limit](https://features.jellyfin.org/posts/59/account-based-stream-limit) | 141 | completed | Per-account limits shipped; later request asks for more granularity |
| [Playback speed](https://features.jellyfin.org/posts/176/playback-speed) | 121 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Non-admin users download subtitles](https://features.jellyfin.org/posts/185/non-admin-users-download-subtitles) | 118 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Make Tags into Clickable Buttons](https://features.jellyfin.org/posts/756/make-tags-into-clickable-buttons) | 115 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [HDR To SDR Tonemapping](https://features.jellyfin.org/posts/640/hdr-to-sdr-tonemapping) | 102 | completed | HDR to SDR conversion exists; tone-map correctness remains route specific |
| [Download Option for Android App](https://features.jellyfin.org/posts/323/download-option-for-android-app) | 98 | completed | Raw Android file download; managed offline demand still started |
| [Content Type: Home Video](https://features.jellyfin.org/posts/579/content-type-home-video) | 91 | completed | Home video library category, no claim of automatic online metadata |
| [Parental Controls "Allow items with tags"](https://features.jellyfin.org/posts/235/parental-controls-allow-items-with-tags) | 86 | completed | Allowed-tag parental controls, scope still needs checking |
| [Manually add subtitles](https://features.jellyfin.org/posts/324/manually-add-subtitles) | 66 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Remember audio/subtitle preferences in Android TV app](https://features.jellyfin.org/posts/2210/remember-audio-subtitle-preferences-in-android-tv-app) | 62 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Hardware acceleration](https://features.jellyfin.org/posts/75/hardware-acceleration) | 62 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [DTS and DTSHD DTSX](https://features.jellyfin.org/posts/312/dts-and-dtshd-dtsx) | 61 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Add Dolby Vision tag whenever applicable](https://features.jellyfin.org/posts/2314/add-dolby-vision-tag-whenever-applicable) | 55 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Add support to remotely read pdfs or epub](https://features.jellyfin.org/posts/483/add-support-to-remotely-read-pdfs-or-epub) | 55 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Music Shuffle All](https://features.jellyfin.org/posts/625/music-shuffle-all) | 50 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Auto-scroll lyrics if it contains timestamps](https://features.jellyfin.org/posts/2661/auto-scroll-lyrics-if-it-contains-timestamps) | 48 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [F-Droid port for Jellyfin Android app](https://features.jellyfin.org/posts/731/f-droid-port-for-jellyfin-android-app) | 48 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Handle movies with multiple/split files as one item](https://features.jellyfin.org/posts/198/handle-movies-with-multiple-split-files-as-one-item) | 47 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Discogs metadata](https://features.jellyfin.org/posts/515/discogs-metadata) | 45 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Allow Custom Chapter Image Intervall](https://features.jellyfin.org/posts/16/allow-custom-chapter-image-intervall) | 44 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Automatic extraction of subtitles](https://features.jellyfin.org/posts/288/automatic-extraction-of-subtitles) | 44 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [OLED friendly screensaver](https://features.jellyfin.org/posts/2844/oled-friendly-screensaver) | 42 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Quickly connect and login without password](https://features.jellyfin.org/posts/541/quickly-connect-and-login-without-password) | 42 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Support external audio tracks](https://features.jellyfin.org/posts/177/support-external-audio-tracks) | 40 | completed | Completed request; see maintainer response and check client/version scope before treating as parity |
| [Disable transcoding](https://features.jellyfin.org/posts/338/disable-transcoding) | 40 | completed | Per-user conversion permission, may fail unsupported original playback |
## Significant features and capabilities to consider

These are prioritised opportunities, including completion of implemented capabilities where user demand makes acceptance unusually valuable. They are not recommendations to abandon deliberate principles. The 120-row ledger retains lower-priority requests so they do not disappear merely because I prefer another direction.

### F-24.1 Import household state from Plex/Jellyfin

**Value:** start with the same files without starting every viewer's progress, favourites, ratings and My List from zero. Migration threads discuss parallel servers, Plexyfin and WatchState because users have years of state. No Plex/Jellyfin import implementation was located in the audited code. **Size: large**, touching catalogue external-ID mapping, personalstate, profile mapping, history and dry-run/reporting. Prefer a local read-only export importer with explicit identity resolution and an undoable batch; never import old access tokens. **Opinion:** one of the highest-value adoption investments. Would lower priority if launch intentionally targets first-time server owners rather than migrations.

### F-24.2 A reviewable versions/editions/merge workflow

Demand is explicit: versions across directories 266 votes, edition labels 104, episode versions 276 in the completion ledger. Portico's source already distinguishes physical assets and logical items, and permits explicit association; automatic same-title grouping is still a known gap. **Size: large**, ingestion/catalogue, metadata identity, title files, playback preferences, source timelines and progress. Separate “same cut, different encode” from “different cut.” **Opinion:** fix the user-facing workflow before release; preserve conservative identity rather than silently merging by filename/provider ID alone. Evidence of different edits needing separate resume would strengthen the need for edition-level timeline identity.

### F-24.3 Complete physical TV acceptance and storefront availability

Apple TV demand 464 votes, VIDAA 457, Samsung 253; Android TV playback rewrite 214 reactions. These counts establish importance across platforms, not market shares. Portico already has Apple/Android/Fire/Vega code, but builds are not store/device acceptance. **Size: large** sustained platform work. Prioritize the named target households: Apple TV and Android/Google TV/Fire TV first; then decide webOS/Tizen, with Roku assessment delegated to its separate audit. **Opinion:** polished first-party TV use is more valuable than many additional server checkboxes. Only real supported-device cohorts can change the order.

### F-24.4 Arbitrary owner-configured OIDC/SSO

Jellyfin request #230 has 1,198 votes and has been planned since 2019. Kyoo advertises arbitrary OIDC; Jellyfin has plugin-based approaches. Portico's Google/Apple Hosted login does not satisfy homelab federation with Authentik/Authelia/Keycloak. **Size: large**, identity trust, subject mapping, recovery, TV sign-in and logout. **Opinion:** a strong optional integration, preserving the deliberate correct-local-password path and sole-owner recovery. It should not make Hosted or an external IdP mandatory for local viewing. Demand from existing account ecosystems would raise its launch priority.

### F-24.5 Managed remote reachability beyond port forwarding

Public-IP discovery and certificates do not penetrate CGNAT. Families repeatedly stay on Plex because they can open a client without VPN instructions. Portico's paid relay possibility is already deliberate. **Size: large**, relay capacity/funding, route selection, privacy, quotas, abuse control, diagnostics and failover; supported overlay/VPS documentation is a medium interim option. **Opinion:** essential for broad remote-family adoption, optional for a clearly local-first launch. Do not promise relay economics from Plezy's control relay; bandwidth differs by orders of magnitude. A reliable local-only launch message can legitimately postpone it.

### F-24.6 Smart playlists as playable saved queries

Fider #49 has 588 votes; smart collections #593 has 104. Portico already has typed browse queries/saved Home views and normalized explicit playlist entries. No persistent dynamic-query playlist model was located. **Size: medium**, query persistence, queue materialization/order/revision semantics, sharing and editing UI. Navidrome clients demonstrate practical smart-playlist demand. **Opinion:** reuse saved-view query semantics, preserving explicit playlists and duplicate occurrences separately. Arbitrary SQL is not needed to satisfy most users and would make restrictions/support harder.

### F-24.7 Accept managed offline use and consider automatic sync rules

The largest active request is Android managed offline (1,820), with offline sync 818 and transcoded downloads 520. Portico has preparations, scoped download records and reconciliation. Plezy adds per-show rules and automatic removal/replenishment. Automatic rule parity was not established here. **Size: large acceptance plus medium rule UI/scheduler**. **Opinion:** first accept OS termination, long travel/no-server use and reliable sync-back; then offer Wi-Fi/storage/episode policy. A raw download success is inadequate. Android interrupted-transfer repairs in Plezy 2.23 are particularly relevant acceptance scenarios.

### F-24.8 Music radio, sonic similarity and specialist continuity

Gapless has 649 votes, crossfade 139, sonic analysis 98. Portico already implements gapless/crossfade/gain/lyrics, while Backlog defers radio/mixes pending a listening-similarity source. **Size: large** if analysing audio vectors; medium if licensed/open recommendation data suffices. Touch recommendation facets, incremental analysis/storage and shared queues. **Opinion:** complete compilations, multi-disc grouping and phone/background listening first; radio is the clearest next music differentiator. Would favour local sonic analysis if its runtime/model/license cost fits weak boxes and privacy principles.

### F-24.9 Accept per-person resource policies and expose finer controls where useful

Jellyfin has existing per-account limits yet #1444 asks for more control; Plezy exposes codec/channel preferences. **Portico already implements per-account MaxStreams, remote bitrate, access schedule and content limits** (`access/limits.go:76–102`, `enforcement.go:105–129`), with an admission gate to avoid concurrent-start races (`playbackv1/admission_gate.go:5–16`). Global conversion limits and owner account/server caps also exist. **Size: medium acceptance/UI work**, then medium optional conversion-only/download/DVR budget refinement. **Opinion:** preserve and verify this implementation rather than adding a duplicate limiter. Decide how conversion-only quotas differ from all-stream caps, and whether recovery/replan consumes a new slot. Owner-requested bitrate constraints differ from automatic load-driven quality reduction.

### F-24.10 Better subtitle controls across every player

Subtitle appearance 188 votes, Android TV offset 180, dual subtitles 37 reactions. Portico has source-relative offsets and partial style controls; broad native parity/dual-track rendering was not established. **Size: medium**, potentially large for dual ASS/PGS. Touch subtitlevideo, client profiles, session changes and native renderers. **Opinion:** offsets, readable TV size/position and forced/default/fallback selection belong before dual-track learning features. Caption accessibility makes this more than decoration. Device tests can determine whether server conversion or native rendering offers the better route.

### F-24.11 External player handoff with progress sync

Jellyfin Android TV #4465 has 28 reactions; Plezy launches external players and recently adds Apple TV Infuse handoff. No comparable external-player handoff implementation was located. **Size: medium/large**, launch intents/URLs, scoped play grants, return progress and authority handoff. **Opinion:** a useful escape hatch for hardware/codec edges, especially desktop/Android. It must not expose permanent session tokens or turn two engines into simultaneous progress authorities. A complete first-party engine matrix would reduce urgency, but not eliminate power-user value.

### F-24.12 A desktop application or deliberate companion path

Plezy's desktop breadth and native codecs attract users who dislike browser limitations; Swiftfin macOS request has 41 reactions. Portico Backlog already plans a desktop companion. **Size: large** packaging/player lifecycle across macOS/Windows/Linux, or medium if a narrow companion launches a tested external player. **Opinion:** choose the product shape explicitly. A browser wrapper does not automatically provide hardware direct play or HDR, and a bundled mpv engine creates its own update burden. Evidence from actual intended desktop users should decide platform order.

### F-24.13 Fuzzy and alternative-title search

Fider #3703 has 90 votes; Portico uses FTS token-prefix queries and exact/prefix ranking. **Size: medium**, bounded typo/alias candidate indexing, ranking, per-user visibility and UI suggestions. **Opinion:** worthwhile after exact-result completeness. Keep the literal query visible and avoid replacing it silently with a popular near-match. Test accented/non-Latin titles and voice-input mistakes; simply enabling an approximate matcher across the full catalogue could violate bounded-query principles.

### F-24.14 Household profile switching, PINs and TV identity

Multi-account switching has 156 votes; Android TV PIN request 56 reactions; Swiftfin Apple TV-user mapping 17. Portico has viewer-selection trust and remembered-account source, but actual PIN/platform-user mapping parity is not established. **Size: medium**, identity, vault scope, remote interaction and offline separation. **Opinion:** accept fast switching before calling the product family-friendly. The television should not become a hidden owner-admin session merely because the owner approved its code; deliberate role semantics must be obvious.

### F-24.15 Viewer folder browsing and home videos

Folder browsing has 440 votes. Plezy supports folder playback, and Jellyfin supports home video libraries. Portico has owner source inventory but no equivalent viewer folder-browser path was located. **Size: medium/large**, safe logical hierarchy, metadata-free cards, privacy, browse counts and queue order. **Opinion:** useful for personal recordings, exercise videos and unmatchable material. Publish display names and opaque locations rather than physical owner paths. A deliberate films/music/audiobooks-only launch can postpone it honestly.

### F-24.16 Podcast subscription and RSS workflows

565 votes remain on podcast support; Audiobookshelf has search, subscription downloads, automatic episodes and RSS for books/podcasts. A `podcast` token in Portico's audio-kind exclusion does not implement this. **Size: large**, feeds, bounded fetches, enclosure identity, episode retention, upload/storage policy and queues. **Opinion:** a coherent listening expansion worth planning, lower than playback acceptance. If implemented, prevent feed/enclosure URLs from becoming an unconstrained server-side fetch feature.

### F-24.17 Trakt/ListenBrainz/Simkl/anime tracking export

Plezy supports several watchlist/rating/scrobble services; music peers support Last.fm/ListenBrainz. None was substantively located in Portico. **Size: medium**, opt-in credentials, stable ID matching, durable outbox/rate limits, completion versus progress semantics and conflict policy. **Opinion:** high migration/retention value for users who keep state outside one server. Default off; no cloud export implied by Hosted login. Portico's local identity and metadata overrides should remain valid without external IDs.

### F-24.18 Request/discovery integration with Seerr

Plezy directly requests films/seasons/4K destinations and shows status from Seerr; this fills a separate acquisition/discovery workflow. No Seerr/Sonarr/Radarr integration was established in Portico—the broad grep had JSON-array false positives. **Size: medium**, owner-controlled endpoint/key, per-user allowed actions, external-ID mapping and UI. **Opinion:** optional integration or deep link first; keep media server and acquisition automation separable. A standalone request surface may be better than expanding Portico's own administrative responsibilities.

### F-24.19 Cross-server search with independent partial results

178 votes for multiple servers, 105 for seamless access and 83 for aggregate libraries support this demand. Portico already has server identities/selection but deliberately scopes core actions to one server. Plezy illustrates fan-out latency and stale-identity costs. **Size: large**, shared search orchestration, qualified item identity, merge/rank rules and authorization. **Opinion:** implement search before unified persistent collections if pursued. One slow server must not hold other results hostage; never deduplicate different edits solely by provider ID.

### F-24.20 Internationalization beyond regional English

Per-user language has 219 votes; simultaneous metadata languages 212. Plezy advertises English plus 21 translations. Portico's US/Canadian/British spelling mechanism is a good foundation but launch English-only is deliberate. **Size: large**, translation workflow, pluralization, RTL layout, keyboard/TV focus, metadata fallback and maintenance. **Opinion:** ask for specific launch markets rather than collecting nominal languages. Closed code contributions need not imply closed translation/testing intake, if Justin wishes to accept those contributions.

### F-24.21 Explainable startup/scan/playback readiness

Hardware self-test has 424 votes; loading progress 79; Jellyfin startup UI and Plezy retry screens show the practical benefit. Portico has stages and reason codes, so much is completion/documentation. **Size: medium** owner/client UX. **Opinion:** make “file found,” “metadata still working,” “source offline,” “needs conversion,” “conversion unavailable” and “retry in progress” visibly different. This lowers support burden without adding decorative technical jargon to normal viewing.

### F-24.22 Reading/OPDS and book portability

Audiobook demand does not automatically ask for an eBook reader, but Fider rich reader 121, Calibre 165 and completed readers demonstrate a separate audience. Stump/Jellyfin/Audiobookshelf provide reading workflows. Bounded searches did not locate a Portico ePub/OPDS reader. **Size: large** reader; medium read-only OPDS export if authority can be preserved. **Opinion:** optional, distinct scope. Portico's generic progress units and kind ranges leave room; they do not establish finished book support.

### F-24.23 Optional customization and home integrations

Theme music, skins, branding, hover preview, voice assistants and Home Assistant are genuine requests even if they conflict with fixed first-party design priorities. Jellyfin supports custom CSS/themes; Plezy supports mpv shaders and TV layout choices. Portico has authored Home rows, not a Kodi-style skin/plugin system. **Size: small to large** depending on surface. **Opinion:** distinguish supported settings, external automation API and full code/theme extension. Keep reduced-motion/accessible defaults and predictable troubleshooting. Demand evidence from intended users could justify more scope.

### F-24.24 A supported integration/export contract

Jellyfin's ecosystem gains clients/plugins but pays for API cleanup; Navidrome's compatibility yields many clients. Portico already generates schemas but shared client code is still moving onto generated contracts. **Size: medium**, SDK/documentation, read/export credentials, deprecation policy and executable compatibility fixtures maintained elsewhere. **Opinion:** even without plugins/external contributions, support owner scripts, safe export and independent clients explicitly if that is intended. If not, say so before 1.0; open source alone is not an API stability promise.

### F-24.25 Reliable repeated-intro/credits detection across episodes

Portico already exposes markers and skip controls, but its current automatic candidates are deliberately weak chapter/luminance heuristics; they do not establish repeated-audio detection comparable to Intro Skipper. Demand is high even though the Jellyfin **segment API** request is marked completed (1,089 votes). **Size: large**, incremental per-source fingerprints, bounded season comparisons, provenance/revision invalidation, owner correction and measured false-positive control. **Opinion:** a valuable next detector behind an already good marker contract, especially for episodic households. Preserve manual/owner-approved markers and exact source timelines. Do not claim broad automatic intro-skip parity from presence of the marker type. Would postpone if the launch promise is explicitly manual/chapter-supported skipping.


## Questions for Portico derived from recurring bugs

These are scoped investigation/acceptance questions. Severity rates the consequence **if the failure occurs**, not a demonstrated current defect. Confidence labels attach to the source observation or risk statement; none means runtime reproduction. Unless explicitly stated, Backlog does not list this exact scenario. Package paths below are under `apps/portico-server/server/internal`; native/shared paths are stated separately.

### Q-24.1 Can scan writes block ordinary viewing on slow storage?

**External evidence:** [Jellyfin #15101](https://github.com/jellyfin/jellyfin/issues/15101), 230 comments, and [#13708](https://github.com/jellyfin/jellyfin/issues/13708), page loads blocked during scans. **Check:** `dbwork`, `ingestion`, `storage`. **Read finding:** the gate serializes physical writers with aging/priorities (`dbwork/gate.go:17–26`); absent filesystem work in the missing-publication transaction is explicit (`ingestion/inventory.go:271`). This mitigates the reported mechanism, but a slow commit/holder can still delay everyone. **Confidence: Confirmed by reading (mitigation), speculative runtime exposure. Severity: High. Backlog:** global performance targets are unestablished; this exact storage matrix is not recorded. Accept an active scan plus resume/save on LXC/ZFS/slow NAS and capture transaction hold/recovery, not only mean throughput.

### Q-24.2 Can an unmounted or replaced root erase personal state?

**External evidence:** [#17214](https://github.com/jellyfin/jellyfin/issues/17214), a scan silently completing with zero items, and [moving library locations](https://github.com/orgs/jellyfin/discussions/6924), metadata loss on re-add. **Check:** `ingestion`, `catalog`, `mounts`, `storage`. **Read finding:** root physical identity and incarnation, completed inventory authority, ancestor recheck, grace and two observations appear in `ingestion/inventory.go:248–330`; source/forget operations retain logical history per inventory contract. **Confidence: Confirmed by reading that safeguards exist; unknown end-to-end coverage. Severity: High. Backlog:** not this precise deletion scenario. Try remount at the same path, empty backing directory, permission denial and recovered source. Look for any legacy cleanup path bypassing inventory authority.

### Q-24.3 Does an in-place media overwrite remain invisible to incremental scans?

**External evidence:** Jellyfin 10.11 regenerates extracted facts on file replacement, then January 2026 repairs watched-state retention. **Check:** `ingestion`, `storage`, `scanevents`, `mediaanalysis`. **Read finding:** `ingestion/incremental.go:47–74` skips a directory when its own snapshot identity/revision matches; `storage/inventory_page.go:46–49` hashes object identity/change token/size/mtime; `:280` returns the directory snapshot. The skipped path only schedules known child directories (`incremental.go:84–113`). An ordinary overwrite can change the file without changing its parent-directory revision, so the skipped directory does not re-stat that file. Automatic integrity scans eventually rewalk after seven days (`:37–39`). **Confidence: Probable source-derived bug; no runtime reproduction. Severity: Medium. Backlog: yes, replaced file not re-read.** Scope is incremental observation, not claiming stale bytes are always playable. A separate watcher/cache-invalidating path could mitigate some events; it was not established here. Accept overwrite with stable filename and no directory-entry change, then verify facts/artifacts and logical progress.

### Q-24.4 Do merge/associate operations preserve edition-specific progress?

**External evidence:** versions demand and Jellyfin 12.0's resume following the selected episode version. **Check:** `catalog` inventory association, `personalstate`, `playbackv1`, `progress`; shared `title-files`. **Read finding:** the inventory contract keeps distinct copies separate until owner association and protects logical history; playback options contain version IDs (`client-core/playback-v1/types.ts:49`). Same-title auto-grouping is not established. **Confidence: Confirmed source capability; unknown edition/timeline semantics. Severity: High for accidental identity collapse. Backlog: yes, second file becomes second title.** Test theatrical/extended cuts, different episode ordering, split files and version removal. A single provider ID proves the work, not identical timestamps across cuts.

### Q-24.5 Can malformed local metadata exhaust the server?

**External evidence:** [Jellyfin #17860](https://github.com/jellyfin/jellyfin/issues/17860) links scan memory pressure to huge malformed NFO title content; older memory threads contain competing plugin/cache explanations. **Check:** `localmetadata`, `metadata`, `metadataprovider`, `storage`, `supervise`. **Read finding:** Portico's inventory/analysis contract promises bounded sidecars and outputs; central supervision contains background panics. That does not establish memory allocation bounds in every parser. **Confidence: Speculative — needs bounded-input/path review and later runtime acceptance. Severity: High. Backlog:** hostile-input route rotation exists, not an NFO allocation matrix. Trace byte/read, XML nesting, credit/title count and provider payload caps independently; no execution was done here.

### Q-24.6 Are residual collection/people pages really independent of library size?

**External evidence:** Jellyfin 12.0 normalizes collections/playlists and fixes Home/read query costs. **Check:** `catalog`, `compactcatalog`. **Read finding:** `catalog/content_entities.go:350–354` still issues `LIMIT ... OFFSET`; `search.go:718–738` people search uses an offset cursor and exact restricted count. These are confirmed shapes, not measured slowness. **Confidence: Confirmed by reading (design cost), runtime severity unknown. Severity: Medium. Backlog: yes for container grids; people-search cost is not the same listed item.** Measure late pages and sparse restrictions at intended scale. Do not present signed opaque cursors as proof of keyset SQL internally.

### Q-24.7 Does resume/seek preserve A/V sync through every remux/conversion route?

**External evidence:** [Jellyfin web #7546](https://github.com/jellyfin/jellyfin-web/issues/7546) audio-transcode HLS timeout; 12.0 notes A/V desync repairs; Plezy [Live seek #2100](https://github.com/edde746/plezy/issues/2100). **Check:** `playback`, `playbackv1`, `decoder`, native player bridges. **Read finding:** the fixed six-second source grid and absolute source timestamps are deliberate (`playback/hls_timeline.go:14–17`, `:63`; `hls.go:478`). **Confidence: Confirmed timing design, unknown device behaviour. Severity: High. Backlog:** physical playback acceptance and HLS requirements remain open broadly. Try seek before first buffered segment, nonzero container PTS, remux video/transcode audio, quality change and rapid repeated seeking; distinguish source clock from engine-window clock.

### Q-24.8 Can subtitles become stale or desynchronized after a seek?

**External evidence:** server #2547 and web #4346 are the strongest recurring subtitle examples. **Check:** `subtitles`, `subtitlevideo`, `playbackv1`, native subtitle renderers. **Read finding:** selected offset reads are bound to session generation/resource revision (`subtitles/manifest.go:49–57`); WebVTT has source-relative timestamps and a timestamp map (`:75–103`); session changes carry offset through reapplication (`sessions_change.go:144–146`). **Confidence: Confirmed safeguards; speculative runtime bug. Severity: High for inaccessible dialogue. Backlog:** no exact scenario. Test positive/negative offset, resume mid-cue, rapid track switch, fragmented HLS and quality/version changes. Preserve exact styling/forced semantics when a renderer changes.

### Q-24.9 Does ASS/PGS selection cause unnecessary burn-in or unusable rendering?

**External evidence:** Jellyfin subtitle burn/render work across 10.10–12.0 and Android TV #2479/#4234; Plezy chooses different engines and ships subtitle fixes frequently. **Check:** `subtitles`, `subtitlevideo`, `playback` capability/delivery decisions, `decoder`, shared client profile. **Read finding:** delivery reason codes and client renderability are modeled; native appearance parity is not demonstrated. **Confidence: Speculative — physical acceptance required. Severity: High for anime/accessible playback. Backlog:** Cast sidecars and broad physical acceptance, not all ASS/PGS cases. Test embedded fonts, positioning, animation, multiple sign/dialogue tracks, PGS forced-only and crop/black bars. Server burn-in is sometimes correct, but weak hardware must explain its cost.

### Q-24.10 Are HDR/DV colours correct, not merely playable?

**External evidence:** Jellyfin Android TV #49 (127 comments), #5093, Swiftfin [#916](https://github.com/jellyfin/Swiftfin/issues/916), and Plezy DV fixes. **Check:** `playback`, `decoder`, client capability publisher, native player/display modules. **Read finding:** DV profile/fallback and approximate-colour reason exist (`delivery_decision.go:126`, `:159`; `delivery_plan.go:652–657`). **Confidence: Confirmed model; unknown visual acceptance. Severity: High. Backlog:** physical media acceptance required. Compare profile 5/7/8, HDR10+, HLG and SDR fallback on actual screens/receivers; validate dynamic metadata/sample entries through remux. A checkbox or successful first frame cannot establish colour correctness.

### Q-24.11 Does direct-play admission match the real decoder/audio sink?

**External evidence:** Android TV #281 HD passthrough, #3900 multichannel AAC→stereo, #5368 DV+TrueHD conversion, Plezy #2621 older AVI playback. **Check:** `playback` delivery decisions, `decoder`, native `PorticoCapabilities`/player modules and shared `client-profile.ts`. **Read finding:** source codec/profile/range checks are detailed; Android media-codec facts and route-specific sink changes need acceptance. **Confidence: Speculative runtime risk. Severity: High. Backlog:** Android/Fire/Vega physical coverage unestablished. Try TV speakers versus AVR, Bluetooth, optical/ARC/eARC, DTS variants, AAC 7.1, interlace and rotation. Codec support must include the transport/container and audio route.

### Q-24.12 Does a hardware failure preserve requested quality and recover cleanly?

**External evidence:** [Jellyfin #4389](https://github.com/jellyfin/jellyfin/issues/4389), 39 comments, and failover demand 91 votes. **Check:** `decoder`, `mediaexec`, `playback`, `operations`. **Read finding:** hardware/software routes, rejection diagnostics and owner conversion limits exist; architecture explicitly promises software fallback without lowering quality. **Confidence: Confirmed intent/source decision shape; unknown failure recovery. Severity: High. Backlog:** not this exact failure chain. Test exhausted hardware sessions, unsupported pixel formats, driver exit and disk pressure. Fallback can overload a weak CPU; report/refuse honestly rather than changing quality under load.

### Q-24.13 Do per-account limits count concurrent starts, replans and Live TV exactly once?

**External evidence:** account-limit demand and shared-server QoS requests. **Check:** `access`, `httpapi` admission, `playbackv1`, `playback` owner policy and `downloads`. **Read finding:** `access/enforcement.go:105–120` counts legacy and v1 sessions while excluding duplicate presentation rows; `access/limits.go:76–102` defines per-member streams/bitrate/schedule. `httpapi/access_admission.go:147` invokes it. Same-account starts are gated through row creation (`playbackv1/admission_gate.go:5–16`, `sessions.go:675`, `channels.go:160`); owner account/server caps are also checked (`playback/v1_session_helpers.go:19–37`). **Confidence: Confirmed by reading (mitigation), unknown all-path/runtime coverage. Severity: Medium. Backlog:** no exact scenario. Verify two simultaneous starts, audio/video/Live TV coexistence, quality replan, zap, end/expiry and failed startup. A dedicated per-user conversion-only quota remains unestablished; do not label existing stream limits absent.

### Q-24.14 Can late progress/favourite writes change the wrong viewer?

**External evidence:** [server #14981](https://github.com/jellyfin/jellyfin/issues/14981), favourite loss during music playback; web #6386; Plezy 2.23 fixes previous Plex user's Home rows. **Check:** `personalstate`, `catalog`, shared viewer selection/runtime and native account vault. **Read finding:** complete scope keys and authority transition fences are normative; `container_personal.go:99–108` writes scoped saved state transactionally. Last-write-wins saved flags differ from explicit offline conflict semantics. **Confidence: Confirmed scoped writes; unknown concurrent/offline switching. Severity: High for privacy/wrong-user state. Backlog:** selected recommendation/Apple stale-response scenarios exist. Try changing viewer while a write/read completes, with two devices offline and old server IDs reused.

### Q-24.15 Do Next Up and Recently Added choose the intended episode?

**External evidence:** [web #8496](https://github.com/jellyfin/jellyfin-web/issues/8496), plays latest episode rather than next; [server #15180](https://github.com/jellyfin/jellyfin/issues/15180), Continue Watching disappears. **Check:** `catalog` Home/episodic projections, `compactcatalog`, shared post-play/title actions. **Read finding:** server-composed Home and show workspace exist; all entry-point semantics were not traced. **Confidence: Speculative runtime/product regression question. Severity: Medium. Backlog:** recently-aired ordering and recommendation acceptance, not all play buttons. Cover specials, split episodes, completed rewatches, newly added older seasons, unavailable next episode and a dismissed continue item. Click target and Play target may legitimately differ, but must be predictable.

### Q-24.16 Can inaccessible titles leak through people/search/recommendations?

**External evidence:** Jellyfin security hardening reports and 12.0 parental-control fixes. **Check:** `catalog`, `contentaccess`, `identity`, `social`, `downloads`. **Read finding:** `search.go:502–504` applies entity restrictions; people search explicitly counts only people with a visible eligible credited item (`:736–738`); Home carries effective restrictions (`home.go:25–30`). **Confidence: Confirmed inspected mitigation; unknown complete route coverage. Severity: High. Backlog:** hostile-input checks do not prove authorization coverage. Check counts, suggestions, artwork, title relationships, versions, deep links, playlists and group invitations, including cached and unavailable records.

### Q-24.17 Do proxy failures degrade cleanly without zombie groups?

**External evidence:** [Jellyfin #13309](https://github.com/jellyfin/jellyfin/issues/13309), proxy Cast/SyncPlay; [#17078](https://github.com/jellyfin/jellyfin/issues/17078), silent websocket drops. **Check:** `httpapi`, `social`, `networking`, shared event transport. **Read finding:** Portico social routes use server-sent event heartbeats and gap resynchronization (`httpapi/social_routes.go:451–497`; `social/events.go:12–24`), not simply the same websocket path. **Confidence: Confirmed transport difference; unknown proxy/liveness acceptance. Severity: Medium. Backlog:** not exact. Test buffering proxy, idle timeout, killed process, laptop sleep, mobile handover and stalled events; expire membership independently from a polite socket-close frame.

### Q-24.18 Does Watch Together drift or change authority during recovery?

**External evidence:** web [#6210](https://github.com/jellyfin/jellyfin-web/issues/6210), transcode desync; Android TV #538 SyncPlay absent; Plezy authenticated resume/host transfer changes. **Check:** `social`, `playbackv1`, shared `group-session`/social runtime and native player time source. **Read finding:** readiness/lag states are explicit; buffering one participant does not pause everyone (`social/transport.go:481–485`). **Confidence: Confirmed policy; unknown clock/control acceptance. Severity: Medium. Backlog:** no equivalent full network matrix. Test host loss, seek races, session regeneration, device sleep and different transcode startup times. Media access must be independently checked for each participant before a group timeline update.

### Q-24.19 Can Cast bootstrap, subtitles and session takeover survive reconnect?

**External evidence:** Plezy #126 Cast 34 reactions, Swiftfin #271 43; Jellyfin Cast [#863](https://github.com/jellyfin/jellyfin-chromecast/issues/863) restrictive HEVC HDR profile. **Check:** `social` Cast bootstrap/receiver grants, `receiver/cast`, native Cast bridge and delivery profile. **Read finding:** one Portico receiver and scoped bootstrap exist; Backlog explicitly says sidecar subtitles are not offered as CAF tracks and physical Cast has not been accepted. **Confidence: Confirmed documented gap; speculative reconnect risk. Severity: Medium. Backlog: yes.** Verify local/remote URL reachability from the receiver itself, expired grants, sender disappearance, quality seek and other device takeover. Browser playback success does not establish receiver network access.

### Q-24.20 Are metadata locks/provenance preserved through refresh and provider changes?

**External evidence:** [server #6709](https://github.com/jellyfin/jellyfin/issues/6709), custom posters reverting; #5236 language mismatch; #5778 unchecked providers still queried. **Check:** `metadata`, `localmetadata`, `metadataprovider`, `compactcatalog`. **Read finding:** `metadata/screen_publish.go:596–667` has a specific local-NFO department/online-crew coexistence rule, including `creditsOnline`; this is more advanced than the stale backlog description. **Confidence: Confirmed current repair path; unknown all providers/locks. Severity: Medium. Backlog:** yes for old crew issue/provider-change stale details, not proof it still reproduces. Test local-only, provider switch/no-match, locked fields and old stale credit rows. Never conclude deliberate single-provider policy guarantees no stray provider access.

### Q-24.21 Do music albums, discs and multi-file books retain their structure?

**External evidence:** Jellyfin #5605, #6694 and [multi-part book #10668](https://github.com/jellyfin/jellyfin/issues/10668), 19 reactions. Plezy [audiobook discussion #2619](https://github.com/edde746/plezy/discussions/2619), 9 October 2026, explicitly says playable audio lacks chapters.

**Check:** `metadata`, `audiofacts`, `catalog`, `compactcatalog`, `playbackv1` listening queues. **Read finding:** Portico has separate book/album containers and native listening ownership; generic audio playability is not a complete book workflow. **Confidence: Speculative acceptance question. Severity: Medium. Backlog:** music similarities/agents are mentioned. Test disc tags, compilation artists, MusicBrainz releases, loose tagged tracks, book parts/chapter edits and file moves. Avoid implying Plezy has full audiobook semantics merely because its README includes music.

### Q-24.22 Can offline transfers resume after OS interruption without corrupt receipts?

**External evidence:** Plezy 2.23 repairs Android resume/restart, wrong progress percentages, partial numbered files and repeated episode re-downloads. **Check:** `downloads`, `preparedmedia`, `artifactlease`, shared downloads service, native `PorticoDownloads`. **Read finding:** idempotent batch submission and per-target visibility exist (`downloads/preparations.go:20–40`); local transfer lifecycle was not exhaustively traced. **Confidence: Confirmed server protection; speculative native-transfer risk. Severity: High for offline usability. Backlog:** general phone/download acceptance, not each scenario. Test >4 GiB files, pause/resume, force stop, custom folder, storage full and checksum failure; compare receipts against the exact version/quality, not title ID alone.

### Q-24.23 Does offline reconciliation preserve explicit user intent?

**External evidence:** offline requests distinguish managed playback/progress from file export; Plezy [discussion #857](https://github.com/edde746/plezy/discussions/857) asks for externally synced-media integration. **Check:** `personalstate`, `downloads`, native storage and shared offline journal. **Read finding:** saved/download contract §4 demands ancestor-aware conflicting explicit edits and visible resolution; source personal rating uses nullable half-star values instead of the contract's integer scale. **Confidence: Confirmed document/schema mismatch; unknown reconciliation behaviour. Severity: Medium. Backlog:** no precise rating-scale entry. Test Mark Unwatched versus playback completion, clear-history versus resume, two-device conflict and expired membership. An arrival-time winner may be wrong even with an idempotency key.

### Q-24.24 Do Live TV startup/guide failures remain isolated and explainable?

**External evidence:** [server #9813](https://github.com/jellyfin/jellyfin/issues/9813), slow start; 12.0 Schedules Direct error-code fix and incremental XMLTV imports; Plezy tuner-timeout fixes. **Check:** `livechannels`, `linearinput`, `linearbuffer`, `recordingaccess`, `recordingmedia`. **Read finding:** channel directory includes provider/group state and guide availability; source sharing/buffer architecture exists. **Confidence: Speculative runtime question. Severity: Medium. Backlog:** integrations not fully exercised. Cover unreachable tuner, locked provider credentials, stale guide, DST, timezone mismatch, schedule change and partial XMLTV refresh. A queued timer does not prove a recording will finish.

### Q-24.25 Does timeshift seek map to the content the viewer sees?

**External evidence:** Plezy #2100, a backwards UI skip reopening ahead of the actual playhead. **Check:** `linearbuffer`, `linearinput`, `playbackv1`, native player timeline adapters. **Read finding:** Portico uses closed segments and a shared buffer, unlike the particular Plex reopen scheme in that report; correctness at live edge/window rollover was not established. **Confidence: Speculative — different architecture. Severity: Medium. Backlog:** no exact bug. Accept pause for several minutes, short backward skips, source reconnect, discontinuity and the oldest retained segment. Make source time, buffer offset and renderer position explicit in traces.

### Q-24.26 Can a long backup pin enough WAL to threaten free space?

**External evidence:** Jellyfin backup/scan coordination and migration/free-space guards; large-library upgrades amplify disk risks. **Check:** `backup`, `dbwork` WAL/reader-pressure/checkpoint code, `operations`. **Read finding:** `backup/copy.go:16–20`, `:35–41` intentionally pins a read snapshot while stepping 16 pages at a time (`:71`). That avoids restarting the copy on commits, but a snapshot also retains WAL frames. This is a legitimate trade-off, not proof of an unbounded WAL leak. **Confidence: Confirmed snapshot design; speculative pressure outcome. Severity: High if disk is exhausted. Backlog:** physical WAL/performance targets unestablished. Accept backup under continuous writes on near-full storage and verify bounded pressure, cancellation, completed-file integrity and recovery.

### Q-24.27 Are native/client teardown and background mode leaks bounded?

**External evidence:** Swiftfin #929 stutter, #937 AirPlay audio delays; Plezy late-film #2314 audio cut-outs; desktop Qt6 migration memory concerns. **Check:** native player/audio ownership modules, shared playback generation/lease lifecycles; server `artifactlease` and `mediaexec`. **Read finding:** source has explicit listening owner and session-generation structures, but no long-session runtime evidence was collected. **Confidence: Speculative. Severity: High for repeated playback failure. Backlog:** physical acceptance open. Run later acceptance through two-hour films, repeated next/quality switches, PiP, foreground/background and output-route changes; watch memory, decoder processes, pins and source descriptors. A five-minute smoke test cannot establish this class.

### Q-24.28 Does security tightening break older released clients predictably?

**External evidence:** Jellyfin legacy authorization disabling and removed routes, Swiftfin 10.11 adaptation, Plezy backend auth fixes. **Check:** `httpapi`/apikit registry, `identity`, generated contracts, shared parsers and update messaging. **Read finding:** bounded parsers intentionally drop unknown content kinds; the API surfaces carry explicit revisions. That is not a complete compatibility policy for renamed fields, new required values or expired session formats. **Confidence: Confirmed design scaffolding; speculative compatibility risk. Severity: High for all-client lockout. Backlog:** client-core migration to generated types remains open. Test oldest supported storefront client against newer server and vice versa, with a clean recovery message rather than unexplained sign-out.

### Q-24.29 Can logs/support export/stream URLs leak durable credentials?

**External evidence:** [server #10808](https://github.com/jellyfin/jellyfin/issues/10808), copying stream URL leaks session key; #13991/#13992 request authentication and granular API permissions. Open reports do not establish current Jellyfin exploitability. **Check:** `diagnostics`, `servicelog`, `telemetry`, `identity`, `httpapi`, media grants and native Report a problem. **Read finding:** Portico has bounded support export and device/scoped session contracts; redaction of every actual field was not established. **Confidence: Speculative — a focused security review belongs to the security domain. Severity: High. Backlog:** no exact entry. Inspect query strings, crash/FFmpeg arguments, imported URLs, room codes and attached file paths; distinguish short-lived media grants from permanent refresh/API secrets.

### Q-24.30 Are hardware/OS support promises validated against packaged dependencies?

**External evidence:** [server #15148](https://github.com/jellyfin/jellyfin/issues/15148), native SQLite illegal instruction on CPUs without SSE4.1; 10.11 drops ARM32; Plezy Linux Wayland requirement and package dependency issues. **Check:** `thirdparty/sqlite`, `decoder`, server release/FFmpeg packaging, native platform minimums. **Read finding:** Portico uses a qualified FFmpeg bundle and declares platform profiles; this audit did not verify every distributed binary's CPU instructions or sandbox availability. **Confidence: Speculative packaging risk. Severity: High on affected supported hosts. Backlog:** weak-hardware goals and platform acceptance remain unestablished. Publish concrete minimums and check old CPUs/container kernels; “Go binary” does not guarantee every media/helper/native dependency shares its portability.

### Q-24.31 Will a viewer understand why a detected intro does not automatically skip?

**External evidence:** Intro Skipper's repeated-audio detection and widespread 10.10 media-segment adoption. **Check:** `mediaanalysis`, `segmentmarkers`, `catalog` marker projection and shared post-play/skip UI. **Read finding:** segment production is wired (`cmd/server/main.go:514–521`, `mediaanalysis/produce.go:279–304`), but chapter-title markers at .85 are explicitly excluded from measured-detector safety; luminance candidates are only .2–.35 (`detectors.go:43`, `:75–98`). `segmentmarkers/markers.go:52–70` requires qualifying provenance/confidence or owner approval. **Confidence: Confirmed by reading, deliberate product behaviour rather than bug. Severity: Low for UI surprise; Medium if marketed as automatic detector parity. Backlog:** no specific automatic-detector parity item was located. Check what candidate/review/manual-skip UI shows and whether edits preserve timeline/version identity; no detection-accuracy/runtime test was run.


## What a pre-release project should decide before 1.0

The projects' histories argue for explicit promises, not copying every option. These decisions are Justin's, especially where they change deliberate scope. Nothing here authorizes a code change or rollout.

| Decision | Evidence of why it matters | A concrete Portico decision to record | Opinion / effort |
|---|---|---|---|
| **API stability** | Jellyfin 12 removes aliases/no-op routes and deprecates replacements; independent clients cannot all update together. | Which `/v1` operations, fields, enum values and error semantics are public promises? What grace is given before removal, and are undocumented routes unsupported? | Publish one supported surface and minimum deprecation interval. Small policy, medium fixtures/docs. |
| **Plugin/extension stance** | DLNA isolation and new recommendation/search interfaces help Jellyfin, while database/plugin changes require rebuilds. | No plugins, out-of-process integrations, or a supported in-process plugin API? Are custom themes/scripted automation separate? | Prefer a narrow external boundary until demand justifies plugin ABI support. Large if in-process. |
| **Database upgrade policy** | 10.11's migration rewrites and corrupt legacy records require hours and special recovery; 12 changes prevent rollback. | Is the baseline frozen at 1.0? How are index builds/backfills resumable? Which prior versions can upgrade directly? Which backups support rollback? | Keep additive/fact/derivation discipline; spell out index/backfill exceptions. Medium operational design. |
| **Client compatibility** | Immich explicitly promises major-family compatibility; Swiftfin/TV stores can lag. | Oldest supported client/server combination, unknown fields/kinds, capability negotiation and useful rejection/update messages. | A tested compatibility matrix beats “same code” assurance. Medium. |
| **Version naming** | Jellyfin's 10.x numbering hid the scale of breaking work. | Does major mean incompatible schema/API/product behaviour, and what are patch/minor promises? How are nightly/RC/beta builds labelled? | Choose understandable rules now; avoid unexplained resets later. Small. |
| **Release cadence and gates** | Six-month hopes slipped for Jellyfin; Plezy ships many releases but still repairs regressions. | Stable/preview channels, security patch procedure, minimum device acceptance, who owns release notes and staged rollout. | Promise gates before dates; avoid an inflexible cadence on a sole maintainer. Medium. |
| **LTS/support lifetime** | New runtime/CPU requirements retire devices; stable branches cannot last indefinitely for free. | Which releases receive security fixes, how long owners may defer upgrades, and when old OS/CPU/client support ends. | A short explicit support promise is better than implicit forever. Small decision, ongoing cost. |
| **Telemetry and diagnostics** | Privacy motivates switches; Immich plans transparent discussion; support still needs useful evidence. | What stays local, what Hosted sees, what Report a problem exports, optional usage metrics if any, retention/deletion and relay privacy. | Keep opt-in export and bounded redaction; explain owner-visible history separately. Small policy, medium audit. |
| **Contribution/governance** | Jellyfin review burden/burnout; Audiobookshelf pauses frontend review during rewrite. | No external code contributions is deliberate. Will tests, translations, device reports, documentation, funding or distribution help be accepted? | Preserve a useful non-code evidence channel. Small policy, medium triage. |
| **Bug/feature intake** | Fider duplicates/stale status make “completed” ambiguous; users report clients in server trackers. | Where requests, support, security reports and device bugs go; expected evidence; per-client scope; how decisions/duplicates are recorded. | One clear intake model reduces noise without silencing demand. Medium. |
| **Remote access service boundary** | Certificates/discovery do not defeat CGNAT; Plezy relay handles only control messages. | Exact free/local/remote support, relay roadmap/funding, fallback, route privacy and whether self-hosted alternatives are supported. | Say port forwarding explicitly at launch. Large service if changed. |
| **Privacy at home** | Portico intentionally keeps owner play history even when a viewer clears theirs; family expectations differ. | Onboarding disclosure of owner/admin visibility, shared-activity opt-in, downloaded-device scope and household guest/PIN policy. | Preserve the decision, communicate its consequences. Small copy/medium verification. |
| **Media/client scope** | Broad inherited media categories can remain rough; peers succeed with specialised listening/reading workflows. | Which workflows and platforms are launch-supported versus experimental, and what unsupported media looks like. | Scope support to accepted devices/workflows. Small list, large acceptance work. |
| **Trust and recovery** | Hosted/local identity, SSO, TV code and stored client credentials have different failure modes. | Supported operation during Hosted/IdP outage, owner reset, certificate expiry and signing-key changes; data/credential restore limits. | Make a local recovery path reviewable without relaxing authorization. Medium. |
| **Distribution and hardware minimums** | ARM32/SSE4.1/Wayland/store restrictions can invalidate a “cross-platform” claim. | Supported OS/CPU/kernel/storage/GPU, container privileges, NAS packages and how media-tool security updates ship. | Publish actual qualified bundles and supported installers. Medium ongoing maintenance. |

### Suggested 1.0 acceptance journeys, derived from the evidence

A feature total will not establish these. The same representative household should be able to: install a supported package with read-only media access; scan a real mixed library with missing/unmatched progress visible; sign into its TV without typing a long password; switch viewers without leaking the previous viewer's rows; play/subtitle/seek/resume representative formats; finish and receive the correct next episode; download a season, lose all connectivity and reconcile progress later; survive an offline mount without losing saved state; recover a backup; and understand why remote access fails under CGNAT. These are suggested release gates, **not tests run here**.

Keep a small permanent regression-media corpus covering ordinary encodes and scars: ASS/fonts, PGS, forced subtitles, negative timestamps, MPEG-TS recordings, interlace/rotation, DV profiles, multichannel AAC/TrueHD/DTS, MP3/FLAC seeks, multi-disc music, multi-file books, alternate cuts and ambiguous metadata. Jellyfin [meta #48](https://github.com/jellyfin/jellyfin-meta/issues/48) explicitly discusses a repeatable test media library. Test fixtures should have clear redistribution rights. Portico's synthetic fixtures and qualified toolchain are helpful foundations but cannot represent every physical decoder/display route.

## Adoption levers

### Migration/import tools

Make parallel installation safe and reversible: point at the same read-only files, map existing users explicitly and preview imported state/identity collisions. Prioritize progress/watched, saved intent, ratings, custom metadata/artwork and edition links before raw historical telemetry. Exclude source credentials. Report unresolved identities rather than choosing a fuzzy title match and overwriting state. Export/import plain local data aligns with Portico's open-data principle and offers an exit path as well as an entry path.

Naming compatibility is a migration issue. A movie name accepted by Plex but not recognised by the new server feels like lost media even if the file is still present. Include a documented “found but unmatched” route and owner fixes. Kyoo's naming-flexibility goal and Jellyfin naming-parity request support treating ambiguous parsing as a first-class workflow rather than blaming users immediately.

### Docker/NAS/binary packaging

Portico already has a Dockerfile/release path; “no Docker in production” describes Hosted operations, not a lack of an owner Docker image. Give owners a pinned image/tag, state/media path diagram, UID/GID/read-only examples, hardware-device access notes and a clear backup/update path. A NAS community package can remove substantial setup work, but claims about Synology/QNAP/Unraid/TrueNAS availability were not verified here. Document what is supported versus community-maintained; do not promise every package at launch.

Short hardware qualification is unusually valuable: the 424-vote self-test request and June 2026 migration thread show owners cannot reliably infer a working transcode path from the selected setting. Report the actual toolchain/driver/encode test and decoder sandbox status. Do not require broad container privilege merely to avoid writing troubleshooting instructions; retain explicit sandbox-required behaviour where the owner selected it.

### Documentation, demo and diagnostics

A useful launch site should show a complete install→first-library→first-TV-play journey, remote access/CGNAT examples, how to see an unmatched item, hardware acceleration checks, subtitle compatibility and restore. A legal-media or synthetic populated demo can lower evaluation cost; this audit deliberately did **not** contact Portico's existing demo. Separate demonstrations of attractive UI from claims about source media codecs/HDR/scale.

Write short recovery guidance tied to reason codes. “Why converted,” “source unavailable,” “waiting for analysis,” “client needs update” and “download needs reconciliation” should lead to an owner/viewer action. Keep confidential technical detail in optional diagnostics; no need to make every family viewer learn FFmpeg or proxies. The public issue sample shows exact media/device/log evidence is much more useful than a generic error toast.

### Which TV platforms matter, and what the evidence can support

| Platform group | Demand evidence in this sample | Portico reading | Practical priority judgement |
|---|---|---|---|
| **Android TV / Google TV / Fire TV** | Jellyfin playback rewrite 214 reactions; HDR 127 comments; direct-play override 49 reactions; SyncPlay 64; Plezy Android TV closed request 39 reactions and repeated Fire/Shield/Onn/Bravia issues. | Shared native source and Android/Fire flavours exist; physical acceptance outstanding. | First acceptance tier with Apple TV. Broad hardware diversity means at least several representative boxes/sinks, not just emulator parity. |
| **Apple TV / tvOS** | Fider 464 votes; Swiftfin TV-user mapping 17 reactions/AirPlay 16 comments; Plezy tvOS TestFlight closed issue 69 comments; repeated migration threads use Infuse. | Apple is the reference UI and has AVPlayer/Top Shelf source, but source/build is not store/hardware acceptance. | First acceptance tier. Complete family switching, audio/HDR/subtitles and signed distribution. |
| **Samsung Tizen** | Fider 253 votes; Jellyfin 2026 reports store-review failure. | Profile/platform names found; installable first-party package not established. | Strong second-tier investigation; browser/native/partner client strategy must be explicit. |
| **LG webOS** | Completed client request 229 votes; recent 12.0 codec notes distinguish webOS generation. | Profile/capability evidence is not an app-store release. | Strong second-tier; older TVs have different browser/codec limitations. |
| **Hisense VIDAA** | Active 457-vote request. | No implementation located. | Real unsatisfied niche, possibly valuable; rank by intended households and feasible distribution, not count alone. |
| **Roku** | Migration discussions mention coverage/polish; Jellyfin has regular Roku releases and store blackout constraints. | Roku code expressly outside this review. | Important household platform to assess in its dedicated audit; no code/parity verdict here. |
| **Vega OS** | Jellyfin [meta #110](https://github.com/jellyfin/jellyfin-meta/issues/110) requests support; Portico has a new port. | Vega implementation exists; actual newer Amazon devices need acceptance. | Track platform change rather than assuming Fire OS tests cover Vega. |
| **Xbox / other TV browser / VR** | Jellyfin Xbox 2026 gamepad/server-version constraint; Quest request 167 votes. | No dedicated implementation established by this researcher. | Optional later scope unless launch users supply a clear need. |

These are evidence-based priorities, **not installed-base statistics**. One long-standing request can accumulate votes because distribution is hard, while a widely available platform generates fewer unmet requests. The evidence cannot tell Justin the percentage of prospective Portico users on any television OS.

### Client availability and trust

Make supported-client installation discoverable from the server and launch documentation. State native versus web wrapper, source-only versus signed store app, and beta versus supported. Mention known codec/OS restrictions before a family member installs an incompatible client. If Portico intentionally supports independent clients, publish the API and compatibility promise; if first-party only, do not imply the entire Jellyfin/Infuse ecosystem automatically works against its protocol.

Funding also affects trust. Plezy's present free-GitHub/paid-store model funds distribution/testing; Jellyfin is volunteer-driven; Immich accepts support without making normal self-hosted features paid. Portico's free/noncommercial plus possible paid relay stance can be appealing, but the relay's scope/cost needs plain wording. No project financial sustainability or actual revenue was measured here.

## Where Portico is stronger

These are advantages in **source/design**, to be advertised only at the level actually accepted:

- Independent Continue Watching removal, My List, local ratings and owner/personal history directly address multiple requests above 800 votes. Portico need not reimplement them to gain value; making them reliable and obvious is the task.
- Central normalized catalogue/membership storage, maintained browse/count/search projections and explicit database priorities anticipate Jellyfin's recent major repair themes. Preserve those boundaries as new features arrive.
- Root identity, completed inventory authority and reversible missing state are unusually careful about unavailable storage. A server should preserve an owner's history when a NAS sleeps or a mount fails.
- Shared client behaviour/authored structure can give household members predictable navigation and semantics across platforms. The evidence makes this worth keeping even though media engines stay platform-specific.
- Server-side explanations, exact revisions and explicit source/version choices can make support more actionable than opaque client decisions. Keep explanations intelligible and diagnostic work bounded.
- Local identity, open SQLite/files, optional Hosted account use and strict separation from library/history provide a credible response to privacy/local-control demand. Explain owner visibility rather than making a blanket privacy claim.
- Integrated recommendations, personal feedback, prepared versions, backup scheduling, lyrics, listening and social playback already cover a breadth of expensive requested capabilities. Resist mistaking their existence for proof of successful device journeys.
- Conservative no-silent-quality-change and no-unapproved automatic marker skip policies respect owners/viewers. Some competitors' adaptive choices are convenient; Portico's deliberate approach remains legitimate if capacity failures and alternatives are clear.

## Doc/code disagreements

| Document statement / known-gap claim | Current evidence | Conclusion and confidence |
|---|---|---|
| Backlog says audiobook Watchlist/“Want to listen” remains open. | `catalog/container_personal.go:54–63` permits `book` watchlisted; same read/write path stores container flags. | **Stale backend-gap claim confirmed by reading.** Actual UI/device acceptance still needs separate checking; do not count book My List as absent. |
| Backlog says `musicArtistStep` groups every album every step. | `metadata/music_artist.go:70–108` walks a bounded artist window using a cursor and limited query. | **Current source differs from backlog.** Do not repeat whole-library sweep as a current bug without tracing another path. |
| Backlog says local NFO cast suppresses online crew and leaves stale credits. | `metadata/screen_publish.go:596–667` specifically computes provider departments beside local fields, including show `creditsOnline`. | **Repair path now exists.** It invalidates the blanket “no coexistence” claim, but does not prove historical rows cleaned or every provider correct. |
| Architecture 02 says all lists use cursors rather than offsets and cost is bounded. | `catalog/content_entities.go:350–354` uses OFFSET; people search uses offset/count at `search.go:718–738`. | **Document overgeneralization confirmed.** Opaque external cursor does not establish keyset internal paging; Backlog already calls out container grids. |
| Persistence README says old derived rows keep serving during rebuild; architecture 03 says readers answer `ErrBuilding` until catch-up. | Both are explicit prose in `persistence/README.md` migration rule 3 and architecture 03 derived-data section. | **Documentation disagreement; runtime resolution not traced here.** Decide whether serving old revision is allowed per domain and mark the state consistently. |
| Saved-state contract §4.1 says rating integer 0..10 with zero unset. | `personal.go:267–276` and baseline `:345` use nullable REAL 0.5..5 in half-star steps. | **Normative shape disagreement confirmed.** Could be deliberate updated UI semantics; conversion/contract decision needed, not an assumed runtime failure. |
| Catalogue architecture says one provider per library “as Plex and Jellyfin have it.” | Jellyfin scanner proposal preserves ordered provider/plugin selection; Portico one-provider rule is deliberate. | **Comparative prose inaccurate.** This is a legitimate difference, not reason to add provider chains automatically. |
| Client/Backlog prose still describes Android/other ports as in-flight/unmerged. | Actual native Android/Fire/Vega sources exist; current project guidance says builds are verified but physical acceptance outstanding. | **Stale planning versus source/build status.** No merge/build was run here; identify shipping status from the current release manifest, not old prose. |
| Backlog lists Cast sidecars not offered as CAF tracks. | Receiver declares VTT capability (`receiver/cast/receiver.js:232`) and reads CAF tracks (`:566`); code search did not establish publishing presentation sidecars as new CAF tracks. | **Gap remains plausible and explicitly known.** Capability declaration/track selection is not sidecar track construction; physical acceptance required. |

No documents or backlog were edited. The disagreements above are recorded for Justin to resolve in the canonical project documentation.

## Open questions for Justin

No answer is needed to complete this read-only report; these are decisions for the architecture review.

1. Is 1.0 intended chiefly for new local users, existing Plex/Jellyfin households, or remote multi-household sharing? The migration/relay/TV priority changes substantially.
2. Which actual TV/device families will be supported at launch, and which will be explicitly experimental? Will webOS/Tizen use first-party packages or a supported independent-client path?
3. What public API/schema/client compatibility and support-lifetime promises should 1.0 make?
4. Should extension support mean narrow external APIs, an integration adapter layer, plugins, themes or none? No external code contributions does not answer this separately.
5. Is arbitrary owner OIDC desired while preserving local authentication and recovery?
6. Should version association automatically group technical encodes, editions/cuts, or only owner-approved relationships? How should resume work across different durations/cuts?
7. Which finer resource policies should supplement the implemented per-account stream/bitrate limits, and how should conversion-only, download and DVR budgets differ?
8. Is a smart playlist a snapshot or a continuously changing query? Can another profile share it without seeing inaccessible results/counts?
9. Should cross-server search/aggregate Home be supported while saved state and Watch Together stay server-scoped?
10. Which specialised listening features—podcasts, radio, sonic similarity, tracking export, car interfaces—matter enough to promise at launch?
11. Are translations/device-testing/documentation contributions welcome despite no external code contributions?
12. How should onboarding explain permanent owner play history, optional shared activity and the external information boundary for Hosted, metadata providers and future relay?
13. Should the rating contract use 0..10 integers or nullable half-star 0.5..5 values? Which is canonical for import/export/offline state?
14. What is the explicit standard for proving weak-hardware and large-library claims before they appear in launch messaging?
15. Can public bug/feature status distinguish implemented backend, implemented UI, store availability and accepted hardware, so “completed” has an honest scope?

## Additional maintainer-response evidence

Of the top 120 active requests, **83 were created before 2021**, including **47 in 2019**. This is long-lived demand, but the response ledger below explains why age/open state cannot be interpreted uniformly. The observations paraphrase actual response text and preserve dates.

| Request | Response date | Maintainer signal | Lesson for Portico |
|---|---|---|---|
| Android offline #218, 1,820 votes | 29 April 2020 | Partial raw original-file download existed; managed offline work remained. | Define completed offline scope precisely. |
| OIDC #230, 1,198 | 18 October 2024 | Authentication-plugin approach, on radar. | SSO should be an explicit trust adapter, including TV clients. |
| Pre-transcoding #570, 979 | 4 November 2025 | Core implementation not currently planned; external FFmpeg plus versions, possible plugin. | Portico's built-in preparation is a real difference, not a Jellyfin parity omission. |
| Audiobooks #243, 858 | 3 September 2022 | Basic book-library support exists, but richer support unfinished. | One playable audio file does not complete chapters/series/parts. |
| External database #315, 664 | 2 June 2025 | EF Core precedes external DB support; PostgreSQL likely before MySQL. | Future capability enabled by a refactor is not shipped deployment support. |
| Apple TV #612, 464 | 27 January 2022 | Native client in testing; points at Swiftfin and independent clients. | Product availability includes both first-party and ecosystem paths. |
| VIDAA #1615, 457 | 26 September 2023 | Registration access blocks open-source project; recommends a streaming device. | Check platform access before planning a port; current rules unknown. |
| Hardware self-test #450, 424 | 7 July 2026 | FFmpeg capability-recognition PRs are groundwork. | Groundwork is different from a representative successful encode test. |
| Missing episodes #572, 344 | 14 March 2023 | Supported through TVDB only at that time. | Provider-dependent capabilities need visible scope. |
| Tag search #276, 256 | 30 December 2024 | Added then reverted for performance. | Build the index and bound query work before restoring user-facing search. |
| Samsung #335, 253 | 15 April 2021 | Sideloadable app development, future store notification. | Sideloadable and store-supported are different adoption states. |
| Batch downloads #655, 219 | 22 February 2023 | Initial server support targeted 10.9; clients still must add it. | Shared contract plus per-platform acceptance. |
| Hide empty shows #41, 191 | 29 May 2026 | Scanner rewrite should address it in 13.0. | Do not confuse a proposal's hoped-for outcome with current behaviour. |
| Account switch #2348, 156 | 5 October 2023 | Android TV already supports remembering/switching users. | Open cross-platform requests may coexist with one-client completion. |
| Backup #1603, 143 | 14 June 2025 | Coming in 10.11; record still started at snapshot. | Release notes supersede stale tracker state for shipped capability. |
| Transcode-ahead #474, 106 | 4 March 2020 | Available since 10.5, despite started tracker state. | Reconcile responses with implementation rather than counting gaps mechanically. |
| iOS Cast #466, 105 | response includes 2021/2022 updates | Added temporarily in Swiftfin then removed; alternatives suggested. | Device capability can regress or move to another client. |
| SyncPlay chat #740, 104 | 28 July 2020 | Maintainers disagree about scope and prefer leaving discussion open. | A roadmap should show unresolved product judgement, not invented consensus. |

### Highest-voted declined records

Declined means a tracker decision, not necessarily that nobody wants it. Portico should retain the demand and make its own choices. The following are the top 15 from the fetched declined view.

| Request | Votes | Status | Response interpretation |
|---|---:|---|---|
| [Federated servers](https://features.jellyfin.org/posts/184/federated-servers) | 381 | declined | Prefer an external aggregator/plugin rather than core federation |
| [Header Authentication](https://features.jellyfin.org/posts/471/header-authentication) | 168 | declined | Cross-client/TV authentication difficulty; no current implementation plan |
| [Upload file from the app or site](https://features.jellyfin.org/posts/543/upload-file-from-the-app-or-site) | 116 | declined | File/library management outside Jellyfin server scope |
| [A support to report bad movie](https://features.jellyfin.org/posts/1254/a-support-to-report-bad-movie) | 84 | declined | Bad-media reporting deferred to third-party tools |
| [External media channels (i.e. Netflix, Audible, Tidal, Spotify, etc. integration)](https://features.jellyfin.org/posts/1022/external-media-channels-i-e-netflix-audible-tidal-spotify-etc-integration) | 60 | declined | Commercial external-service channel integrations not planned |
| [More information on "Playback Error" to simplify troubleshooting](https://features.jellyfin.org/posts/1731/more-information-on-playback-error-to-simplify-troubleshooting) | 45 | declined | Reclassified as a bug, not rejected troubleshooting value |
| [auto delete watched content](https://features.jellyfin.org/posts/294/auto-delete-watched-content) | 44 | declined | Automatic watched-media deletion considered too error-prone |
| [Add feature to allow Jellyfin to utilize certificates from Let's Encrypt](https://features.jellyfin.org/posts/70/add-feature-to-allow-jellyfin-to-utilize-certificates-from-lets-encrypt) | 40 | declined | Prefer reverse proxy to integrating all certificate management |
| [Wake on WanLan](https://features.jellyfin.org/posts/588/wake-on-wanlan) | 29 | declined | Wake-on-LAN outside server scope |
| [Remove From Continue Watching/Next Up Button](https://features.jellyfin.org/posts/4116/remove-from-continue-watching-next-up-button) | 26 | declined | Duplicate of continue-removal demand |
| [Request shows, movies, music from sonarr, radarr, lidarr](https://features.jellyfin.org/posts/604/request-shows-movies-music-from-sonarr-radarr-lidarr) | 24 | declined | Acquisition requests outside core; external tools/plugin possible |
| [Chose resolution and bitrate before play a media](https://features.jellyfin.org/posts/2514/chose-resolution-and-bitrate-before-play-a-media) | 23 | declined | Already available in most clients; report lacked context |
| [Playlists.](https://features.jellyfin.org/posts/1274/playlists) | 21 | declined | Multiple feature requests need separate records |
| [Support Widevine DRM](https://features.jellyfin.org/posts/750/support-widevine-drm) | 21 | declined | DRM conflicts with project stance |
| [Client on HarmonyOS NEXT](https://features.jellyfin.org/posts/2781/client-on-harmonyos-next) | 20 | declined | No hardware/low demand; future platform partnership possible |
## Supplemental demand beyond the active top 120

These rows capture playback/music/parental-control nuances requested in the brief. Rank is within the 500-record active snapshot. They are additional demand, not a second artificially boosted ranking. Product availability is only claimed where sourced above; otherwise the row remains a request.

| Rank | Request | Votes / status | Portico source/status | Effort and opinion |
|---:|---|---|---|---|
| 131 | [Be able to search and filter by language (movies/series)](https://features.jellyfin.org/posts/690/be-able-to-search-and-filter-by-language-movies-series) | 73 / open | Audio-language predicate already exists [SEARCH]; subtitle-language parity uncertain | M; complete visible language filters |
| 142 | [On-Demand Subtitle Search](https://features.jellyfin.org/posts/3385/on-demand-subtitle-search) | 68 / open | Subtitle subsystem exists; external marketplace parity not established [SUB] | M; useful optional provider integration |
| 145 | [Multiple versions of Music](https://features.jellyfin.org/posts/994/multiple-versions-of-music) | 66 / open | Music release identity exists; alternate recording/version UI not fully traced [AUDIO] | L; keep releases/masters distinct |
| 156 | [Add Force Transcoding setting](https://features.jellyfin.org/posts/1699/add-force-transcoding-setting) | 63 / open | Delivery options/capability rejection present; explicit force-codec control uncertain [PLAY] | M; a useful hardware escape hatch |
| 173 | [Option to maintain HDR when transcoding](https://features.jellyfin.org/posts/1694/option-to-maintain-hdr-when-transcoding) | 55 / planned | HDR ranges/plan modeled [HW]; HDR-preserving re-encode acceptance unknown | L; hardware/output-route acceptance |
| 189 | [Support for composer tags (music)](https://features.jellyfin.org/posts/1933/support-for-composer-tags-music) | 52 / planned | Composer tag is read at assets/audio_tags.go:67; composer browse UX uncertain | M; significant classical music usability |
| 216 | [Allow typos/misspellings in search](https://features.jellyfin.org/posts/1141/allow-typos-misspellings-in-search) | 46 / open | Prefix search, no fuzzy matcher found [SEARCH] | M; same demand as #3703, do not double-count users |
| 228 | [Cache transcoded streams](https://features.jellyfin.org/posts/1821/cache-transcoded-streams) | 44 / open | Prepared artifacts and exact revisions exist [PREP]; reusable transient-conversion policy not traced | M; correct source/policy cache key before reuse |
| 246 | [Group music releases by type (Album, Single, Live, Compilation, etc).](https://features.jellyfin.org/posts/2220/group-music-releases-by-type-album-single-live-compilation-etc) | 42 / open | Release-type tag read in assets/audio_tags.go:67; albumType rendering is known open | M; useful music browsing completeness |
| 256 | [Transcode test](https://features.jellyfin.org/posts/223/transcode-test) | 40 / open | Hardware probes/diagnostics present [HW]; sample throughput qualification unknown | M; acceptance/readiness rather than a benchmark claim |
| 258 | [Add a music equalizer](https://features.jellyfin.org/posts/736/add-a-music-equalizer) | 40 / open | Gain/normalization present [AUDIO]; equalizer not located in audited source | M; optional DSP, care with passthrough |
| 319 | [Transcode only if resolution isn't present](https://features.jellyfin.org/posts/2211/transcode-only-if-resolution-isnt-present) | 32 / open | Versions and offers exist [PLAY]; automatic matching direct version/rung unknown | M; avoid converting 4K when correct 1080p already exists |
| 325 | [Filtering by HDR/SDR movies](https://features.jellyfin.org/posts/1323/filtering-by-hdr-sdr-movies) | 32 / open | dynamicRange predicate present at browse_contract.go:137 | S/M; verify facets and per-source semantics |
| 341 | [Option to Always Transcode with Preferred Codec (e.g., AV1 or HEVC)](https://features.jellyfin.org/posts/3269/option-to-always-transcode-with-preferred-codec-e-g-av1-or-hevc) | 30 / open | AV1 client capability/probe exists; arbitrary preferred output codec parity unknown | L; decoder support is not encoder support |
| 352 | [Parental Control to Limit Screentime](https://features.jellyfin.org/posts/699/parental-control-to-limit-screentime) | 29 / open | Access schedule exists at access/limits.go:91; cumulative daily time-budget feature not established | M; a different parental policy, not a missing schedule |
| 363 | [Explicit Music Indicator/Filter](https://features.jellyfin.org/posts/916/explicit-music-indicator-filter) | 28 / open | General restrictions exist; explicit-music field/inheritance not established | M; avoid using subjective discovery tags as safety metadata |
| 387 | [Allowed Tags/Whitelist to Work in Conjunction with Maximum Parental Rating](https://features.jellyfin.org/posts/2732/allowed-tags-whitelist-to-work-in-conjunction-with-maximum-parental-rating) | 26 / open | Tag/rating access checks present [ID/LIMIT]; combination semantics require focused review | M; accept AND/allowlist precedence explicitly |
| 406 | [Improve Parental Control Granularity](https://features.jellyfin.org/posts/2580/improve-parental-control-granularity) | 25 / open | Per-account restrictions and library access exist; requested per-library/genre combinations not fully traced | M; check complete exposure, do not duplicate policy |

## Ranked issue-tracker evidence appendix

This appendix preserves **every unique issue in each pair of open top-30 samples**, rather than showing only convenient examples. C-rank is its rank in the comment-sorted sample; R-rank is its rank in the reaction-sorted sample. A dash means it did not appear in that sample, not zero comments/reactions. Counts/status are the snapshot, not claims the issue reproduces on the latest stable release. Old open reports are especially weak proof of a current defect. Total reactions includes all emoji types. Issue descriptions were used selectively for the recurring classes; every linked issue's complete comment history was not reviewed.

### Jellyfin server: 51 unique ranked open issues; 425 open search matches

| Issue | Created | Comments | Reactions | C-rank | R-rank |
|---|---|---:|---:|---:|---:|
| [#2547 Subtitles load after a while and can be out of sync when resuming](https://github.com/jellyfin/jellyfin/issues/2547) | 2020-03-09 | 151 | 105 | 2 | 1 |
| [#3016 Improve Prometheus Exporter with JF Metrics](https://github.com/jellyfin/jellyfin/issues/3016) | 2020-04-30 | 9 | 63 | — | 2 |
| [#14981 Song favorites get lost during playback](https://github.com/jellyfin/jellyfin/issues/14981) | 2025-10-11 | 27 | 27 | 22 | 3 |
| [#2451 Renaming Media Library causes a rescan](https://github.com/jellyfin/jellyfin/issues/2451) | 2020-02-23 | 6 | 25 | — | 4 |
| [#13991 GetUser() endpoint (UserController) is fully unauthenticated](https://github.com/jellyfin/jellyfin/issues/13991) | 2025-04-23 | 1 | 25 | — | 5 |
| [#15148 SQLite core dump on systems without SSE4.1](https://github.com/jellyfin/jellyfin/issues/15148) | 2025-10-23 | 62 | 24 | 4 | 7 |
| [#10808 Refactor "Copy Stream URL" to not leak the user's session API key](https://github.com/jellyfin/jellyfin/issues/10808) | 2024-01-04 | 5 | 24 | — | 6 |
| [#13992 Granular apikey permissions](https://github.com/jellyfin/jellyfin/issues/13992) | 2025-04-23 | 1 | 23 | — | 8 |
| [#15101 [Group]  "Database Locked" on LXC containers using ZFS as storage](https://github.com/jellyfin/jellyfin/issues/15101) | 2025-10-21 | 230 | 20 | 1 | 9 |
| [#5605 Multi disc music albums are displayed as separate albums](https://github.com/jellyfin/jellyfin/issues/5605) | 2021-03-24 | 71 | 19 | 3 | 11 |
| [#10668 [Issue]: Multi-Part Audiobook being treated as folder with indivdual files instead of one book](https://github.com/jellyfin/jellyfin/issues/10668) | 2023-12-03 | 21 | 19 | — | 10 |
| [#13984 Video streams unauthenticated (VideosController, HlsSegmentController, LiveTVController)](https://github.com/jellyfin/jellyfin/issues/13984) | 2025-04-23 | 3 | 19 | — | 12 |
| [#13983 GetAttachment() (VideoAttachmentsController) endpoint is unauthenticated](https://github.com/jellyfin/jellyfin/issues/13983) | 2025-04-23 | 0 | 19 | — | 13 |
| [#5778 Jellyfin fetching metadata from TheAudioDb and MusicBrainz when providers are unchecked in library settings](https://github.com/jellyfin/jellyfin/issues/5778) | 2021-04-11 | 21 | 18 | — | 15 |
| [#13982 Authentication and user ids are decoupled](https://github.com/jellyfin/jellyfin/issues/13982) | 2025-04-23 | 0 | 18 | — | 14 |
| [#4389 [feature/bug] Implement (hardware)transcoder failover/prioritisation](https://github.com/jellyfin/jellyfin/issues/4389) | 2020-10-29 | 39 | 17 | 9 | 16 |
| [#2570 Populate existing metadata fields for songs and use already embedded tags](https://github.com/jellyfin/jellyfin/issues/2570) | 2020-03-06 | 4 | 15 | — | 17 |
| [#13990 GetUsers() endpoint (UserController) only requires user privileges](https://github.com/jellyfin/jellyfin/issues/13990) | 2025-04-23 | 1 | 15 | — | 18 |
| [#2520 EF Core Migration - Metadata I18N Support](https://github.com/jellyfin/jellyfin/issues/2520) | 2020-03-05 | 4 | 14 | — | 19 |
| [#13987 Most endpoints in DashboardController are unauthenticated](https://github.com/jellyfin/jellyfin/issues/13987) | 2025-04-23 | 1 | 14 | — | 21 |
| [#13988 All (raw) image endpoints in ImageByNameController, ImageController & RemoteImageController are unauthenticated](https://github.com/jellyfin/jellyfin/issues/13988) | 2025-04-23 | 0 | 14 | — | 20 |
| [#6709 Some custom primary images/art seem to have automatically reverted or replaced with defaults](https://github.com/jellyfin/jellyfin/issues/6709) | 2021-10-17 | 39 | 13 | 8 | 24 |
| [#15180 'Continue Watching' not displaying on home screen](https://github.com/jellyfin/jellyfin/issues/15180) | 2025-10-24 | 32 | 13 | 15 | 23 |
| [#3356 Actors with the same name appear as one person](https://github.com/jellyfin/jellyfin/issues/3356) | 2020-06-16 | 27 | 13 | 24 | 26 |
| [#17877 [12.0] Random shows locked on "watched" status](https://github.com/jellyfin/jellyfin/issues/17877) | 2026-09-08 | 23 | 13 | — | 22 |
| [#1481 Metadata is file based, not library based](https://github.com/jellyfin/jellyfin/issues/1481) | 2019-06-21 | 10 | 13 | — | 25 |
| [#16241 SyncPlay group causes erratic playback (auto-pauses, skips, desync)](https://github.com/jellyfin/jellyfin/issues/16241) | 2026-02-14 | 3 | 13 | — | 27 |
| [#13930 Fresh Jellyfin install unreachable on IPv6 single-stack network](https://github.com/jellyfin/jellyfin/issues/13930) | 2025-04-15 | 21 | 12 | — | 29 |
| [#13986 All endpoints in AudioController are unauthenticated](https://github.com/jellyfin/jellyfin/issues/13986) | 2025-04-23 | 0 | 12 | — | 28 |
| [#13985 "Gets subtitles in a specified format" and "Gets subtitles in a specified format" (SubtitleController) endpoints are unauthenticated](https://github.com/jellyfin/jellyfin/issues/13985) | 2025-04-23 | 0 | 11 | — | 30 |
| [#6694 Music (Songs) not added / parsed correctly into Albums](https://github.com/jellyfin/jellyfin/issues/6694) | 2021-10-12 | 42 | 9 | 6 | — |
| [#15272 [10.11.1] Scanning music library reset the recent added order](https://github.com/jellyfin/jellyfin/issues/15272) | 2025-10-28 | 31 | 9 | 17 | — |
| [#5236 Metadata in English, not in selected language](https://github.com/jellyfin/jellyfin/issues/5236) | 2021-02-14 | 38 | 8 | 10 | — |
| [#12255 [Issue]: Metadata fetch timeout - IPv6 connectivity broken](https://github.com/jellyfin/jellyfin/issues/12255) | 2024-07-11 | 27 | 6 | 23 | — |
| [#17989 Dashboard fails to load with "r.map is not a function" error](https://github.com/jellyfin/jellyfin/issues/17989) | 2026-09-11 | 25 | 6 | 26 | — |
| [#15283 Music Artists Not Correct on Tracks with Multiple 'artist' Entries](https://github.com/jellyfin/jellyfin/issues/15283) | 2025-10-28 | 31 | 5 | 18 | — |
| [#10914 [Issue]: DVBSUB Subtitles not selectable when watching Live TV](https://github.com/jellyfin/jellyfin/issues/10914) | 2024-01-26 | 24 | 5 | 29 | — |
| [#8687 [Issue]: Server Crash "An invalid IP address was specified. (Parameter 'address')"](https://github.com/jellyfin/jellyfin/issues/8687) | 2022-11-03 | 33 | 4 | 14 | — |
| [#12408 Movie favourites are randomly unfavourited](https://github.com/jellyfin/jellyfin/issues/12408) | 2024-08-07 | 33 | 4 | 13 | — |
| [#12764 Duplicate music albums shown in music library on the artist page if this artist already was in another library](https://github.com/jellyfin/jellyfin/issues/12764) | 2024-10-02 | 28 | 4 | 21 | — |
| [#11667 [Issue]: Cannot play videos at random nor create playlist](https://github.com/jellyfin/jellyfin/issues/11667) | 2024-05-15 | 25 | 4 | 25 | — |
| [#18066 Migration ChangeOwnerIdToGuid never completes and keeps one CPU core at 100%](https://github.com/jellyfin/jellyfin/issues/18066) | 2026-09-15 | 25 | 4 | 27 | — |
| [#15849 Artwork randomly disappearing](https://github.com/jellyfin/jellyfin/issues/15849) | 2025-12-21 | 25 | 2 | 28 | — |
| [#12008 [Issue]: All Library Scan causes playlists to disappear](https://github.com/jellyfin/jellyfin/issues/12008) | 2024-06-07 | 50 | 1 | 5 | — |
| [#13712 Videos from Google Pixel 4a shot in portrait are played in wrong orientation](https://github.com/jellyfin/jellyfin/issues/13712) | 2023-06-14 | 34 | 1 | 12 | — |
| [#9623 [Issue]: Music artists with the same name are merged while MBIDs are ignored](https://github.com/jellyfin/jellyfin/issues/9623) | 2023-04-09 | 29 | 1 | 20 | — |
| [#9813 Jellyfin taking a long time to start live TV playback.](https://github.com/jellyfin/jellyfin/issues/9813) | 2023-05-25 | 41 | 0 | 7 | — |
| [#14283 Path too long for metadata saver](https://github.com/jellyfin/jellyfin/issues/14283) | 2025-06-11 | 35 | 0 | 11 | — |
| [#1757 Create Albums with metadata, not by file structure](https://github.com/jellyfin/jellyfin/issues/1757) | 2019-09-14 | 31 | 0 | 16 | — |
| [#14689 Media playback randomly starts over again or jumps back - STILL](https://github.com/jellyfin/jellyfin/issues/14689) | 2025-08-22 | 29 | 0 | 19 | — |
| [#16687 DOVIWithHDR10Plus files transcode to SDR instead of falling back to DOVIWithHDR10 Direct Play](https://github.com/jellyfin/jellyfin/issues/16687) | 2026-04-23 | 24 | 0 | 30 | — |
### Jellyfin web: 47 unique ranked open issues; 301 open search matches

| Issue | Created | Comments | Reactions | C-rank | R-rank |
|---|---|---:|---:|---:|---:|
| [#4346 [Issue]: Sub title desync JF 10.8.9](https://github.com/jellyfin/jellyfin-web/issues/4346) | 2023-01-27 | 65 | 36 | 1 | 1 |
| [#3967 Recommendations for First Time Contributors](https://github.com/jellyfin/jellyfin-web/issues/3967) | 2022-09-29 | 5 | 17 | — | 2 |
| [#4698 Implement playlist ACL Interface](https://github.com/jellyfin/jellyfin-web/issues/4698) | 2023-06-27 | 2 | 15 | — | 3 |
| [#516 Security issue with access token](https://github.com/jellyfin/jellyfin-web/issues/516) | 2019-08-16 | 6 | 14 | — | 4 |
| [#7546 [Regression] Web browser HLS playback times out when audio transcoding required - worked in 10.10.7, broken in 10.11.6](https://github.com/jellyfin/jellyfin-web/issues/7546) | 2026-01-28 | 11 | 12 | — | 6 |
| [#5182 Update high level documentation for new contributors](https://github.com/jellyfin/jellyfin-web/issues/5182) | 2024-02-04 | 2 | 12 | — | 5 |
| [#6728 Subtitles disappear when fullscreen](https://github.com/jellyfin/jellyfin-web/issues/6728) | 2025-04-08 | 22 | 11 | 5 | 7 |
| [#8496 Playing a show from "Recently Added" plays latest episode instead of next episode.](https://github.com/jellyfin/jellyfin-web/issues/8496) | 2026-09-16 | 7 | 11 | — | 8 |
| [#7383 Default to channel logos in Live TV](https://github.com/jellyfin/jellyfin-web/issues/7383) | 2025-12-01 | 0 | 10 | — | 9 |
| [#3314 [webOS/TV] No sound when increasing playback speed](https://github.com/jellyfin/jellyfin-web/issues/3314) | 2021-12-16 | 12 | 8 | 27 | 12 |
| [#3783 Subtitle offset text field is clamped to slider min/max values](https://github.com/jellyfin/jellyfin-web/issues/3783) | 2022-07-30 | 7 | 8 | — | 11 |
| [#3086 theme song / theme videos volume adjustments](https://github.com/jellyfin/jellyfin-web/issues/3086) | 2021-10-22 | 3 | 8 | — | 10 |
| [#558 Inconsistent size of subtitles](https://github.com/jellyfin/jellyfin-web/issues/558) | 2019-11-01 | 22 | 7 | 4 | 13 |
| [#6210 SyncPlay desync when transcoding](https://github.com/jellyfin/jellyfin-web/issues/6210) | 2024-10-16 | 9 | 7 | — | 14 |
| [#1425 Subtitles dark background box](https://github.com/jellyfin/jellyfin-web/issues/1425) | 2020-06-17 | 18 | 6 | 8 | 15 |
| [#7176 Play All and Shuffle of mixed Home Videos and Photos library doesn't play videos if images present](https://github.com/jellyfin/jellyfin-web/issues/7176) | 2025-10-08 | 10 | 6 | — | 17 |
| [#1752 Search results are limited to 24.](https://github.com/jellyfin/jellyfin-web/issues/1752) | 2020-07-23 | 20 | 5 | 6 | 18 |
| [#2480 Subtitles in PIP](https://github.com/jellyfin/jellyfin-web/issues/2480) | 2021-03-04 | 18 | 5 | 10 | 24 |
| [#7405 HLS Media Errors only in Webbrowsers](https://github.com/jellyfin/jellyfin-web/issues/7405) | 2025-12-09 | 18 | 5 | 9 | 21 |
| [#5208 KDE/Plasma Media Player controls only partly work with Jellyfin in the browser](https://github.com/jellyfin/jellyfin-web/issues/5208) | 2024-02-16 | 16 | 5 | 15 | 25 |
| [#7341 webOS playing Dolby Vision direct fails to display on 10.11.3](https://github.com/jellyfin/jellyfin-web/issues/7341) | 2025-11-19 | 14 | 5 | 20 | 16 |
| [#2526 Web or Apps lose path on updates (chrome reload/back; refresh metadata; etc)](https://github.com/jellyfin/jellyfin-web/issues/2526) | 2021-03-26 | 13 | 5 | 21 | 19 |
| [#7416 (10.11.3) Jellyfin Drops frames in Firefox when switching tabs or pausing video stream.](https://github.com/jellyfin/jellyfin-web/issues/7416) | 2025-11-20 | 12 | 5 | 29 | 22 |
| [#8007 No trailers are playing in Safari macOS](https://github.com/jellyfin/jellyfin-web/issues/8007) | 2026-01-18 | 7 | 5 | — | 23 |
| [#6679 Unable to zoom in when reading books](https://github.com/jellyfin/jellyfin-web/issues/6679) | 2025-03-29 | 6 | 5 | — | 20 |
| [#723 Subtitles dim screen (like video controls do)](https://github.com/jellyfin/jellyfin-web/issues/723) | 2020-01-21 | 22 | 4 | 3 | 27 |
| [#8190 `r.map is not a function` when accessing Dashboard](https://github.com/jellyfin/jellyfin-web/issues/8190) | 2026-07-10 | 11 | 4 | — | 29 |
| [#8484 Regression in 12.0: physical Pause key no longer works on Samsung Tizen](https://github.com/jellyfin/jellyfin-web/issues/8484) | 2026-09-14 | 3 | 4 | — | 30 |
| [#6335 Branding page enhancements](https://github.com/jellyfin/jellyfin-web/issues/6335) | 2024-11-21 | 0 | 4 | — | 28 |
| [#7444 SRT subtitles not shown in the PIP window](https://github.com/jellyfin/jellyfin-web/issues/7444) | 2025-12-24 | 15 | 3 | 16 | — |
| [#35 Chromecast connection in UI is lost when switching pages](https://github.com/jellyfin/jellyfin-web/issues/35) | 2019-01-14 | 13 | 3 | 23 | — |
| [#2582 Book Position Not Remembered](https://github.com/jellyfin/jellyfin-web/issues/2582) | 2021-04-11 | 13 | 3 | 22 | — |
| [#1417 Rework NativeShell/AppHost](https://github.com/jellyfin/jellyfin-web/issues/1417) | 2020-06-16 | 6 | 3 | — | 26 |
| [#5246 Firefox / Chromium on Linux can't playback all files, Chrome working](https://github.com/jellyfin/jellyfin-web/issues/5246) | 2024-03-01 | 16 | 2 | 14 | — |
| [#2483 Cannot send request when too many songs are selected](https://github.com/jellyfin/jellyfin-web/issues/2483) | 2020-10-30 | 12 | 2 | 25 | — |
| [#4296 Gamepad support broken on Steam Deck](https://github.com/jellyfin/jellyfin-web/issues/4296) | 2023-01-22 | 16 | 1 | 12 | — |
| [#5037 [Issue]: Playback error on folders list (Photos library with home videos)](https://github.com/jellyfin/jellyfin-web/issues/5037) | 2023-07-04 | 16 | 1 | 13 | — |
| [#88 Revamp initial setup dialog to be more useful](https://github.com/jellyfin/jellyfin-web/issues/88) | 2019-01-03 | 12 | 1 | 28 | — |
| [#6386 Favourite button/icon in the queue view not showing updates](https://github.com/jellyfin/jellyfin-web/issues/6386) | 2023-12-17 | 22 | 0 | 2 | — |
| [#2750 Error 414 (request-URI Too Tong) when increased the library page size to 250 (8925 characters)](https://github.com/jellyfin/jellyfin-web/issues/2750) | 2021-06-17 | 19 | 0 | 7 | — |
| [#1012 Errors in editing library should produce an error message](https://github.com/jellyfin/jellyfin-web/issues/1012) | 2020-03-09 | 17 | 0 | 11 | — |
| [#5634 Bug: 3D movie sbs playback, inf bar and subtitle does not look good in v10.9.2](https://github.com/jellyfin/jellyfin-web/issues/5634) | 2024-05-30 | 15 | 0 | 17 | — |
| [#3441 Current playlist shortcut disapear when connected to DLNA server](https://github.com/jellyfin/jellyfin-web/issues/3441) | 2020-09-20 | 14 | 0 | 18 | — |
| [#4077 The homepage only displays the backdrops from libraries of the "movie" type](https://github.com/jellyfin/jellyfin-web/issues/4077) | 2022-10-20 | 14 | 0 | 19 | — |
| [#2426 Can't edit scheduled task trigger](https://github.com/jellyfin/jellyfin-web/issues/2426) | 2021-02-17 | 12 | 0 | 24 | — |
| [#4915 Starting playback on Chromecast doesn't work from the movie / episode page](https://github.com/jellyfin/jellyfin-web/issues/4915) | 2021-03-10 | 12 | 0 | 26 | — |
| [#3782 eBooks title wrong location](https://github.com/jellyfin/jellyfin-web/issues/3782) | 2022-07-29 | 11 | 0 | 30 | — |
### Jellyfin Android TV / Fire TV: 48 unique ranked open issues; 295 open search matches

| Issue | Created | Comments | Reactions | C-rank | R-rank |
|---|---|---:|---:|---:|---:|
| [#1057 The Playback Rewrite](https://github.com/jellyfin/jellyfin-androidtv/issues/1057) | 2021-07-31 | 16 | 214 | 19 | 1 |
| [#538 Add support for Jellyfin SyncPlay](https://github.com/jellyfin/jellyfin-androidtv/issues/538) | 2020-08-04 | 15 | 64 | 22 | 2 |
| [#3509 PIN protection for users](https://github.com/jellyfin/jellyfin-androidtv/issues/3509) | 2024-04-13 | 7 | 56 | — | 3 |
| [#494 Force "direct play" in client](https://github.com/jellyfin/jellyfin-androidtv/issues/494) | 2020-06-01 | 22 | 49 | 11 | 4 |
| [#1150 Video Backdrop and Theme song playback](https://github.com/jellyfin/jellyfin-androidtv/issues/1150) | 2021-09-22 | 28 | 38 | 9 | 5 |
| [#4394 Dual Subtitles](https://github.com/jellyfin/jellyfin-androidtv/issues/4394) | 2025-01-10 | 2 | 37 | — | 6 |
| [#2479 Adding audio / subtitles sync menu in ExoPlayer (adjust audio / subtitles sync functionality) ](https://github.com/jellyfin/jellyfin-androidtv/issues/2479) | 2023-02-07 | 38 | 32 | 3 | 7 |
| [#654 show titles in poster view "all media"](https://github.com/jellyfin/jellyfin-androidtv/issues/654) | 2020-12-14 | 20 | 31 | 13 | 8 |
| [#608 List view](https://github.com/jellyfin/jellyfin-androidtv/issues/608) | 2020-10-26 | 7 | 30 | — | 9 |
| [#4465 Add option to play via external player directly from movie/episode screen](https://github.com/jellyfin/jellyfin-androidtv/issues/4465) | 2025-02-17 | 8 | 28 | — | 10 |
| [#1304 3D movie playback](https://github.com/jellyfin/jellyfin-androidtv/issues/1304) | 2021-12-21 | 5 | 23 | — | 11 |
| [#2867 Add a fallback to AC3/EAC3 option](https://github.com/jellyfin/jellyfin-androidtv/issues/2867) | 2023-07-02 | 7 | 22 | — | 12 |
| [#3170 Deep Link Support](https://github.com/jellyfin/jellyfin-androidtv/issues/3170) | 2023-11-08 | 5 | 22 | — | 13 |
| [#49 Android TV App doesn't support HDR](https://github.com/jellyfin/jellyfin-androidtv/issues/49) | 2019-02-20 | 127 | 21 | 1 | 14 |
| [#3122 Feature to force transcode.](https://github.com/jellyfin/jellyfin-androidtv/issues/3122) | 2023-10-15 | 6 | 21 | — | 15 |
| [#281 HD Audio Passthrough on Nvidia Shield](https://github.com/jellyfin/jellyfin-androidtv/issues/281) | 2020-01-31 | 50 | 20 | 2 | 16 |
| [#5093 [0.19.2] Regression: Jellyfin Android TV 0.19.2 triggers unnecessary transcoding for HEVC Dolby Vision video (was Direct Play in previous versions)](https://github.com/jellyfin/jellyfin-androidtv/issues/5093) | 2025-11-03 | 34 | 18 | 5 | 20 |
| [#4177 Add "Skip" button to player overlay during media segments](https://github.com/jellyfin/jellyfin-androidtv/issues/4177) | 2024-11-06 | 13 | 18 | 26 | 19 |
| [#2289 Configurable default language for audio/subtitle](https://github.com/jellyfin/jellyfin-androidtv/issues/2289) | 2022-11-24 | 8 | 18 | — | 17 |
| [#3218 Selectable Audio and Subtitles Tracks Prior to Playback Start, and Context Awareness](https://github.com/jellyfin/jellyfin-androidtv/issues/3218) | 2023-12-10 | 4 | 18 | — | 18 |
| [#2333 Stack subtitles with overlapping timestamps](https://github.com/jellyfin/jellyfin-androidtv/issues/2333) | 2022-12-10 | 10 | 16 | — | 21 |
| [#1897 Ability to trigger library scan](https://github.com/jellyfin/jellyfin-androidtv/issues/1897) | 2022-07-27 | 5 | 15 | — | 22 |
| [#2646 Option to disable per-episode thumbnails and use series thumbnail instead](https://github.com/jellyfin/jellyfin-androidtv/issues/2646) | 2023-04-06 | 4 | 14 | — | 23 |
| [#5066 On-Demand Subtitle Search](https://github.com/jellyfin/jellyfin-androidtv/issues/5066) | 2025-10-29 | 0 | 14 | — | 24 |
| [#4234 Subtitles rendered above bottom black bar on movies with Aspect Ratio 2.35:1](https://github.com/jellyfin/jellyfin-androidtv/issues/4234) | 2024-11-20 | 30 | 13 | 8 | 26 |
| [#3924 Option to hide OSD when pausing](https://github.com/jellyfin/jellyfin-androidtv/issues/3924) | 2024-08-22 | 11 | 13 | — | 27 |
| [#5517 HDR10+ and other HDR formats trigger transcode/remux due to `VideoRangeTypeNotSupported` — Atmos and lossless audio metadata stripped as side effect](https://github.com/jellyfin/jellyfin-androidtv/issues/5517) | 2026-04-07 | 10 | 13 | — | 28 |
| [#3547 Time Sensitive Seeking](https://github.com/jellyfin/jellyfin-androidtv/issues/3547) | 2024-05-02 | 2 | 13 | — | 25 |
| [#72 Subtitle default option per user](https://github.com/jellyfin/jellyfin-androidtv/issues/72) | 2019-03-11 | 13 | 12 | 24 | 29 |
| [#1149 Improved home page refresh logic or manual refresh button](https://github.com/jellyfin/jellyfin-androidtv/issues/1149) | 2021-09-20 | 13 | 12 | 28 | — |
| [#690 Downloadable Subtitles Like Plex in Jellyfin TV App](https://github.com/jellyfin/jellyfin-androidtv/issues/690) | 2021-01-10 | 8 | 12 | — | 30 |
| [#5190 v0.19.4 very slow compared to v0.18.11](https://github.com/jellyfin/jellyfin-androidtv/issues/5190) | 2025-11-26 | 31 | 11 | 7 | — |
| [#3934 Issue with playing vertically recoded phone videos - None of them plays in Android TV or Amazon Fire TV Stick](https://github.com/jellyfin/jellyfin-androidtv/issues/3934) | 2024-08-26 | 28 | 9 | 10 | — |
| [#3900 AAC 6/8-channel audio only outputs as Stereo PCM](https://github.com/jellyfin/jellyfin-androidtv/issues/3900) | 2024-08-15 | 35 | 4 | 4 | — |
| [#5368 DolbyVision + TrueHD gets Transcoded with compatible hardware in 19.6 (worked in 18.11)](https://github.com/jellyfin/jellyfin-androidtv/issues/5368) | 2026-01-27 | 19 | 4 | 15 | — |
| [#3717 Respect the user's DisplayMissingEpisodes setting in TV Show libraries](https://github.com/jellyfin/jellyfin-androidtv/issues/3717) | 2024-07-03 | 12 | 4 | 30 | — |
| [#1035 Significant stuttering/dropping frames playback of content after 15-30 minutes of play](https://github.com/jellyfin/jellyfin-androidtv/issues/1035) | 2021-07-25 | 20 | 2 | 12 | — |
| [#3154 On seek video get paused on Fire TV](https://github.com/jellyfin/jellyfin-androidtv/issues/3154) | 2023-10-31 | 17 | 2 | 16 | — |
| [#700 Folder view does not work on Firestick 4k](https://github.com/jellyfin/jellyfin-androidtv/issues/700) | 2021-01-27 | 16 | 2 | 18 | — |
| [#867 There is no volume setting built in to the app](https://github.com/jellyfin/jellyfin-androidtv/issues/867) | 2021-05-09 | 13 | 2 | 25 | — |
| [#990 Add support for direct play of H.265/HEVC and Opus (Amazon Fire TV Stick)](https://github.com/jellyfin/jellyfin-androidtv/issues/990) | 2021-06-30 | 12 | 2 | 29 | — |
| [#5593 Android TV: DTS-HD MA not passed through (Kodi works on same Shield/AVR chain)](https://github.com/jellyfin/jellyfin-androidtv/issues/5593) | 2026-05-17 | 16 | 1 | 20 | — |
| [#5094 Some DV file playback fine, other are black](https://github.com/jellyfin/jellyfin-androidtv/issues/5094) | 2025-11-03 | 13 | 1 | 27 | — |
| [#470 Playlist limited to 150 items](https://github.com/jellyfin/jellyfin-androidtv/issues/470) | 2020-05-01 | 33 | 0 | 6 | — |
| [#3986 [BUG] Jellyfin pauses before next song](https://github.com/jellyfin/jellyfin-androidtv/issues/3986) | 2024-09-12 | 19 | 0 | 14 | — |
| [#4435 Switched channels with Opus audio](https://github.com/jellyfin/jellyfin-androidtv/issues/4435) | 2025-02-03 | 17 | 0 | 17 | — |
| [#5358 Fire TV Cube playback issues](https://github.com/jellyfin/jellyfin-androidtv/issues/5358) | 2026-01-26 | 15 | 0 | 21 | — |
| [#4103 Video Freezing While Audio Keeps Playing](https://github.com/jellyfin/jellyfin-androidtv/issues/4103) | 2024-10-23 | 14 | 0 | 23 | — |
### Swiftfin: 41 unique ranked open issues; 84 open search matches

| Issue | Created | Comments | Reactions | C-rank | R-rank |
|---|---|---:|---:|---:|---:|
| [#57 Local Downloads](https://github.com/jellyfin/Swiftfin/issues/57) | 2021-06-11 | 22 | 145 | 2 | 1 |
| [#271 iOS - Chromecast](https://github.com/jellyfin/Swiftfin/issues/271) | 2022-01-08 | 21 | 43 | 3 | 2 |
| [#215 Swiftfin on M1 Macs / macOS](https://github.com/jellyfin/Swiftfin/issues/215) | 2021-11-05 | 5 | 41 | 19 | 3 |
| [#812 Support for custom headers?](https://github.com/jellyfin/Swiftfin/issues/812) | 2023-08-04 | 4 | 31 | 24 | 4 |
| [#390 Support for SharePlay](https://github.com/jellyfin/Swiftfin/issues/390) | 2022-03-13 | 7 | 29 | 16 | 5 |
| [#688 [Feature Request] SyncPlay](https://github.com/jellyfin/Swiftfin/issues/688) | 2023-01-28 | 11 | 28 | 8 | 6 |
| [#1189 tvOS - Support Deep Links](https://github.com/jellyfin/Swiftfin/issues/1189) | 2024-08-15 | 4 | 26 | 23 | 7 |
| [#258 tvOS mapping Apple TV users to Jellyfin users](https://github.com/jellyfin/Swiftfin/issues/258) | 2022-01-06 | 11 | 17 | 7 | 8 |
| [#937 [tvOS] Audio Delays for AirPlay Speakers](https://github.com/jellyfin/Swiftfin/issues/937) | 2023-12-21 | 16 | 11 | 5 | 9 |
| [#1525 Skip button for media segments (intro/outro)](https://github.com/jellyfin/Swiftfin/issues/1525) | 2025-04-29 | 1 | 10 | — | 10 |
| [#916 Flickering black screen every two seconds](https://github.com/jellyfin/Swiftfin/issues/916) | 2023-11-17 | 4 | 9 | 28 | 12 |
| [#1241 Add Secondary Subtitles support](https://github.com/jellyfin/Swiftfin/issues/1241) | 2024-09-23 | 0 | 9 | — | 11 |
| [#673 Random order play](https://github.com/jellyfin/Swiftfin/issues/673) | 2023-01-06 | 10 | 8 | 11 | 13 |
| [#1509 Cinema Mode/Pre Rolls](https://github.com/jellyfin/Swiftfin/issues/1509) | 2025-04-23 | 3 | 8 | — | 15 |
| [#841 Atmos EAC support](https://github.com/jellyfin/Swiftfin/issues/841) | 2023-09-14 | 0 | 8 | — | 14 |
| [#929 Apple TV - Fully-compatible video stutters and drops keyframes](https://github.com/jellyfin/Swiftfin/issues/929) | 2023-11-26 | 13 | 7 | 6 | 16 |
| [#1892 Custom Subtitles Renderer for AVPlayerLayer [Swift library already available]](https://github.com/jellyfin/Swiftfin/issues/1892) | 2026-01-06 | 9 | 7 | 12 | 17 |
| [#1439 Add option to make LiveTV a primary header element.](https://github.com/jellyfin/Swiftfin/issues/1439) | 2025-02-21 | 4 | 6 | 27 | 18 |
| [#867 Shortcuts and Automation](https://github.com/jellyfin/Swiftfin/issues/867) | 2023-09-30 | 4 | 5 | 21 | 19 |
| [#1444 Very slow initial loading time for media playback](https://github.com/jellyfin/Swiftfin/issues/1444) | 2025-03-03 | 10 | 4 | 9 | 21 |
| [#335 Enable Top Shelf items on AppleTV](https://github.com/jellyfin/Swiftfin/issues/335) | 2022-01-21 | 4 | 4 | 22 | 22 |
| [#842 Live TV Guide](https://github.com/jellyfin/Swiftfin/issues/842) | 2023-09-14 | 4 | 4 | 29 | 23 |
| [#762 Sleeptimer](https://github.com/jellyfin/Swiftfin/issues/762) | 2023-04-30 | 0 | 4 | — | 20 |
| [#545 Figure Out Shared Groups - Widgets, Top Shelf](https://github.com/jellyfin/Swiftfin/issues/545) | 2022-08-29 | 3 | 3 | — | 27 |
| [#745 Framerate not switching to movie framerate](https://github.com/jellyfin/Swiftfin/issues/745) | 2023-04-10 | 3 | 3 | 30 | 24 |
| [#1575 HomeView Carousel](https://github.com/jellyfin/Swiftfin/issues/1575) | 2025-06-15 | 3 | 3 | — | 28 |
| [#2275 Incorrect Audio Track Selected](https://github.com/jellyfin/Swiftfin/issues/2275) | 2026-09-08 | 2 | 3 | — | 25 |
| [#609 Support for playlist](https://github.com/jellyfin/Swiftfin/issues/609) | 2022-10-19 | 0 | 3 | — | 26 |
| [#2330 Enable automatic PiP mode when closing the app](https://github.com/jellyfin/Swiftfin/issues/2330) | 2026-09-24 | 0 | 3 | — | 29 |
| [#1231 Remote play not working ](https://github.com/jellyfin/Swiftfin/issues/1231) | 2024-09-09 | 4 | 2 | 20 | — |
| [#1079 Add Theme songs](https://github.com/jellyfin/Swiftfin/issues/1079) | 2024-05-28 | 3 | 2 | — | 30 |
| [#1536 Implement Toast](https://github.com/jellyfin/Swiftfin/issues/1536) | 2025-05-13 | 27 | 1 | 1 | — |
| [#723 Multi Channel Audio through Standard Video Player does not work](https://github.com/jellyfin/Swiftfin/issues/723) | 2023-03-12 | 10 | 1 | 10 | — |
| [#2174 VLC Fails to Play Out of Order Subtitles](https://github.com/jellyfin/Swiftfin/issues/2174) | 2026-08-04 | 18 | 0 | 4 | — |
| [#1568 Automate Swiftfin App Store Screenshots](https://github.com/jellyfin/Swiftfin/issues/1568) | 2025-06-12 | 8 | 0 | 15 | — |
| [#1835 Load media sources in background](https://github.com/jellyfin/Swiftfin/issues/1835) | 2025-12-05 | 8 | 0 | 14 | — |
| [#2264 Seeking fails on direct-play MPEG-TS recordings (VLC player)](https://github.com/jellyfin/Swiftfin/issues/2264) | 2026-09-02 | 8 | 0 | 13 | — |
| [#2237 Swiftfin Player fails over Tailscale while Native Player works](https://github.com/jellyfin/Swiftfin/issues/2237) | 2026-08-19 | 6 | 0 | 17 | — |
| [#1784 Local Downloads - `DownloadManager` and Metadata Storage](https://github.com/jellyfin/Swiftfin/issues/1784) | 2025-10-28 | 5 | 0 | 18 | — |
| [#2155 tvOS: Swiftfin ignores library display settings configured on Jellyfin Server](https://github.com/jellyfin/Swiftfin/issues/2155) | 2026-07-30 | 4 | 0 | 25 | — |
| [#2201 Native (AVPlayer) engine causes 2-4x higher server CPU than VLCKit during Direct Play, causing stutter on remote playback](https://github.com/jellyfin/Swiftfin/issues/2201) | 2026-08-11 | 4 | 0 | 26 | — |
### Plezy: 53 unique ranked open issues; 235 open search matches

| Issue | Created | Comments | Reactions | C-rank | R-rank |
|---|---|---:|---:|---:|---:|
| [#126 Google Cast support](https://github.com/edde746/plezy/issues/126) | 2025-11-22 | 21 | 34 | 3 | 1 |
| [#791 TV Theme Music (Support Local Media Assets)](https://github.com/edde746/plezy/issues/791) | 2026-04-02 | 2 | 19 | — | 2 |
| [#1447 Add an offline local access mode with manually configured Plex server address](https://github.com/edde746/plezy/issues/1447) | 2026-06-28 | 2 | 12 | — | 3 |
| [#970 Plex Preroll support](https://github.com/edde746/plezy/issues/970) | 2026-05-03 | 5 | 11 | 21 | 4 |
| [#1565 tvOS - Multiple Profiles](https://github.com/edde746/plezy/issues/1565) | 2026-07-13 | 4 | 9 | 26 | 6 |
| [#367 Background play](https://github.com/edde746/plezy/issues/367) | 2026-02-03 | 1 | 9 | — | 5 |
| [#502 FONT SIZE settings control for side and selected item](https://github.com/edde746/plezy/issues/502) | 2026-02-21 | 0 | 7 | — | 7 |
| [#689 Add subtitle styling into player](https://github.com/edde746/plezy/issues/689) | 2026-03-13 | 0 | 7 | — | 8 |
| [#782 Support for mpv scripts (e.g. auto-VSR)](https://github.com/edde746/plezy/issues/782) | 2026-04-01 | 10 | 6 | 4 | 11 |
| [#72 [Feature Request] FCast support](https://github.com/edde746/plezy/issues/72) | 2025-11-07 | 1 | 6 | — | 9 |
| [#401 Feature Request: Content Screensaver](https://github.com/edde746/plezy/issues/401) | 2026-02-07 | 0 | 6 | — | 10 |
| [#95 [Feature Request] Subtitle font change](https://github.com/edde746/plezy/issues/95) | 2025-11-12 | 4 | 5 | 28 | 16 |
| [#1795 Plezy Settings Cloud Backup](https://github.com/edde746/plezy/issues/1795) | 2026-08-05 | 3 | 5 | — | 13 |
| [#2174 In-Player Subtitle Position and Size Controls](https://github.com/edde746/plezy/issues/2174) | 2026-08-30 | 3 | 5 | — | 15 |
| [#1570 [Seerr] Browse by cast members suggestions](https://github.com/edde746/plezy/issues/1570) | 2026-07-14 | 2 | 5 | — | 12 |
| [#1517 Search by Actor/Director](https://github.com/edde746/plezy/issues/1517) | 2026-07-10 | 0 | 5 | — | 14 |
| [#1652 Home Screen: Merge Sections Across Multiple Libraries](https://github.com/edde746/plezy/issues/1652) | 2026-07-22 | 5 | 4 | 23 | 25 |
| [#1035 Jellyfin Pin Collections to Home and allow ordering](https://github.com/edde746/plezy/issues/1035) | 2026-05-13 | 2 | 4 | — | 18 |
| [#823 Control the player from external sources like Home Assistant.](https://github.com/edde746/plezy/issues/823) | 2026-04-08 | 1 | 4 | — | 19 |
| [#831 Audio drops when resuming, skipping forward or skipping backwards on media files with AC3 5.1](https://github.com/edde746/plezy/issues/831) | 2026-04-09 | 1 | 4 | — | 20 |
| [#938 Local Extras and Trailers](https://github.com/edde746/plezy/issues/938) | 2026-04-28 | 1 | 4 | — | 17 |
| [#1980 Allow users to select which Plex servers are connected](https://github.com/edde746/plezy/issues/1980) | 2026-08-17 | 1 | 4 | — | 21 |
| [#2537 Option to completely disable "Play Next" prompt at end of episode](https://github.com/edde746/plezy/issues/2537) | 2026-09-30 | 1 | 4 | — | 22 |
| [#1146 Notification for new episode (from official Plex app)](https://github.com/edde746/plezy/issues/1146) | 2026-05-26 | 0 | 4 | — | 23 |
| [#1780 Feature Request: Detect Existing Local Files & Refresh Offline Downloads](https://github.com/edde746/plezy/issues/1780) | 2026-08-03 | 0 | 4 | — | 24 |
| [#1735 Slow UI Performance](https://github.com/edde746/plezy/issues/1735) | 2026-07-31 | 4 | 3 | 24 | 26 |
| [#1315 Large skip option for ignore chapter](https://github.com/edde746/plezy/issues/1315) | 2026-06-11 | 2 | 3 | — | 28 |
| [#1023 Jellyfin Quick Connect Approval](https://github.com/edde746/plezy/issues/1023) | 2026-05-12 | 0 | 3 | — | 27 |
| [#2367 Improve media info previews for TV shows and add visibility options](https://github.com/edde746/plezy/issues/2367) | 2026-09-15 | 0 | 3 | — | 30 |
| [#2595 Atmos support for EAC3 Music](https://github.com/edde746/plezy/issues/2595) | 2026-10-06 | 0 | 3 | — | 29 |
| [#2515 (Re-)Enable Season Posters as Tabs (and/or Seasons as Tabs Period) for TV Target Platforms](https://github.com/edde746/plezy/issues/2515) | 2026-09-29 | 6 | 2 | 14 | — |
| [#863 Support custom aspect ratios for Constant Image Height (CIH)](https://github.com/edde746/plezy/issues/863) | 2026-04-14 | 5 | 2 | 18 | — |
| [#1321 Option to Disable Codecs/Force Transcode](https://github.com/edde746/plezy/issues/1321) | 2026-06-12 | 7 | 1 | 9 | — |
| [#1120 Unified Library not functioning Andriod](https://github.com/edde746/plezy/issues/1120) | 2026-05-24 | 6 | 1 | 11 | — |
| [#1385 Expand Plezy Companion Remote to Support Mobile-to-Mobile Control](https://github.com/edde746/plezy/issues/1385) | 2026-06-20 | 5 | 1 | 17 | — |
| [#2200 2.18.0 FireTV 4k & LG OLED - App now allows passthrough for DTS-HD MA, unsupported by LG OLED](https://github.com/edde746/plezy/issues/2200) | 2026-08-31 | 5 | 1 | 19 | — |
| [#1480 Feature Request: Fast scrubbing gesture on Apple TV (press + swipe)](https://github.com/edde746/plezy/issues/1480) | 2026-07-03 | 4 | 1 | 30 | — |
| [#1510 Allow Media Play/Pause button to start the playback of focused item](https://github.com/edde746/plezy/issues/1510) | 2026-07-08 | 4 | 1 | 29 | — |
| [#2302 Multiple issues with MPV](https://github.com/edde746/plezy/issues/2302) | 2026-09-09 | 31 | 0 | 1 | — |
| [#2255 Intermittent playback stuttering since Plezy 2.18.0](https://github.com/edde746/plezy/issues/2255) | 2026-09-06 | 26 | 0 | 2 | — |
| [#2314 [Android] Audio cut outs](https://github.com/edde746/plezy/issues/2314) | 2026-09-10 | 9 | 0 | 5 | — |
| [#1963 Linux playback causing crash.](https://github.com/edde746/plezy/issues/1963) | 2026-08-16 | 7 | 0 | 6 | — |
| [#2100 Live TV skip-back can seek forward after a time-shift reopen](https://github.com/edde746/plezy/issues/2100) | 2026-08-24 | 7 | 0 | 7 | — |
| [#2621 AVI file stopped](https://github.com/edde746/plezy/issues/2621) | 2026-10-09 | 7 | 0 | 8 | — |
| [#1709 [Bug] Screen flickering when playing AV1 video with Opus 5.1 audio on Fire Stick (ExoPlayer)](https://github.com/edde746/plezy/issues/1709) | 2026-07-29 | 6 | 0 | 10 | — |
| [#2431 [Bug] Seek drops HW decoding to SW when video starts from a resume point via Android Intent](https://github.com/edde746/plezy/issues/2431) | 2026-09-22 | 6 | 0 | 12 | — |
| [#2574 Jellyfin STRM playback fails](https://github.com/edde746/plezy/issues/2574) | 2026-10-04 | 6 | 0 | 13 | — |
| [#2616 tvOS - Plezy unable to reconnect to Plex server](https://github.com/edde746/plezy/issues/2616) | 2026-10-09 | 6 | 0 | 15 | — |
| [#1009 Display show/movie logo on player details screen](https://github.com/edde746/plezy/issues/1009) | 2026-05-11 | 5 | 0 | 16 | — |
| [#2264 5.1 not working on iPadOS](https://github.com/edde746/plezy/issues/2264) | 2026-09-06 | 5 | 0 | 22 | — |
| [#2401 Plezy UI is displayed upside down on Bouygues TV box](https://github.com/edde746/plezy/issues/2401) | 2026-09-20 | 5 | 0 | 20 | — |
| [#847 Fix live tv judder with MPV on nvidia shield](https://github.com/edde746/plezy/issues/847) | 2026-04-12 | 4 | 0 | 25 | — |
| [#1966 Volume issues / low volume](https://github.com/edde746/plezy/issues/1966) | 2026-08-16 | 4 | 0 | 27 | — |

### Plezy: globally most-reacted closed issues

The later successful REST search covered the top 30 by reactions out of 1,736 closed issues. Closed means a tracker lifecycle state; without a release or maintainer resolution it does not prove implementation, fixed runtime behaviour or universal client support. The separate 94-issue comment-sorted closed subset was used for regressions such as late audio cut-outs, stutter, posters and connectivity.

| Issue | Created | Closed | Comments | Reactions |
|---|---|---|---:|---:|
| [#67 Support for AndroidTV](https://github.com/edde746/plezy/issues/67) | 2025-11-06 | 2025-12-10 | 21 | 39 |
| [#199 Live TV Support](https://github.com/edde746/plezy/issues/199) | 2025-12-27 | 2026-02-14 | 58 | 15 |
| [#44 [request] picture-in-picture for background play](https://github.com/edde746/plezy/issues/44) | 2025-11-05 | 2026-03-02 | 25 | 14 |
| [#963 tvOS TestFlight](https://github.com/edde746/plezy/issues/963) | 2026-05-02 | 2026-05-25 | 69 | 11 |
| [#2244 Stop dimming the media area  when hovering over the sidebar](https://github.com/edde746/plezy/issues/2244) | 2026-09-04 | 2026-09-18 | 1 | 11 |
| [#957 Jellyfin Support Beta](https://github.com/edde746/plezy/issues/957) | 2026-05-02 | 2026-05-11 | 2 | 10 |
| [#281 feat: Support Jellyfin servers](https://github.com/edde746/plezy/issues/281) | 2026-01-17 | 2026-05-02 | 4 | 9 |
| [#78 Feature Request: Watch Together](https://github.com/edde746/plezy/issues/78) | 2025-11-09 | 2025-12-19 | 1 | 9 |
| [#139 Download Media for Offline Use](https://github.com/edde746/plezy/issues/139) | 2025-11-30 | 2025-12-13 | 0 | 8 |
| [#24 Better Linux builds](https://github.com/edde746/plezy/issues/24) | 2025-11-04 | 2025-12-26 | 4 | 8 |
| [#213 Selecting download quality](https://github.com/edde746/plezy/issues/213) | 2026-01-04 | 2026-04-08 | 1 | 7 |
| [#1030 Emby Support as well](https://github.com/edde746/plezy/issues/1030) | 2026-05-12 | 2026-08-05 | 7 | 7 |
| [#43 Feature request: Skip intro/credits](https://github.com/edde746/plezy/issues/43) | 2025-11-04 | 2025-11-10 | 2 | 6 |
| [#343 Downloading subtitles from Opensubtitles?](https://github.com/edde746/plezy/issues/343) | 2026-01-27 | 2026-04-01 | 4 | 6 |
| [#84 [Feature Request]Let me choose audio track / subs BEFORE starting the video. ( like plex's native player )](https://github.com/edde746/plezy/issues/84) | 2025-11-10 | 2026-04-08 | 6 | 5 |
| [#617 Trakt integration](https://github.com/edde746/plezy/issues/617) | 2026-03-03 | 2026-04-19 | 5 | 5 |
| [#203 Android no longer apk](https://github.com/edde746/plezy/issues/203) | 2025-12-29 | 2025-12-29 | 8 | 5 |
| [#432 Plezy Watchlist (unrelated to Plex Watchlist)](https://github.com/edde746/plezy/issues/432) | 2026-02-10 | 2026-07-10 | 12 | 5 |
| [#1029 Add "favorite" feature](https://github.com/edde746/plezy/issues/1029) | 2026-05-12 | 2026-07-04 | 1 | 5 |
| [#1676 Don't show UI when seeking on Android TV](https://github.com/edde746/plezy/issues/1676) | 2026-07-25 | 2026-07-29 | 1 | 5 |
| [#635 feat: Show Clock on Pause](https://github.com/edde746/plezy/issues/635) | 2026-03-05 | 2026-08-09 | 7 | 5 |
| [#2042 Android TV has truncated movie info](https://github.com/edde746/plezy/issues/2042) | 2026-08-20 | 2026-08-23 | 1 | 5 |
| [#1827 Play Next Countdown](https://github.com/edde746/plezy/issues/1827) | 2026-08-07 | 2026-08-27 | 1 | 5 |
| [#1646 Background App Refresh: AppleTV, iOS, iPadOS](https://github.com/edde746/plezy/issues/1646) | 2026-07-21 | 2026-08-31 | 2 | 4 |
| [#1877 Feature Request: Webview / OAuth / Custom Header support for Seerr servers behind reverse proxy](https://github.com/edde746/plezy/issues/1877) | 2026-08-12 | 2026-09-03 | 2 | 4 |
| [#1751 Please add Mdblist as a tracker.](https://github.com/edde746/plezy/issues/1751) | 2026-07-31 | 2026-08-05 | 0 | 4 |
| [#1640 Linux: MPV loadfile command fails with Jellyfin (Ubuntu 24.04, Jellyfin 10.11.11)](https://github.com/edde746/plezy/issues/1640) | 2026-07-20 | 2026-08-11 | 2 | 4 |
| [#2440 Transcoding Downloads](https://github.com/edde746/plezy/issues/2440) | 2026-09-23 | 2026-09-23 | 1 | 4 |
| [#389 Support for TheIntroDB API for Skip Intro/Outro Management](https://github.com/edde746/plezy/issues/389) | 2026-02-06 | 2026-02-22 | 2 | 4 |
| [#234 Season, epsiode details in thumbnails](https://github.com/edde746/plezy/issues/234) | 2026-01-14 | 2026-04-23 | 4 | 3 |

### Meta/governance and Plezy discussion evidence

[Jellyfin meta #26](https://github.com/jellyfin/jellyfin-meta/issues/26) records earlier EF Core/API-v2 planning, while [#29](https://github.com/jellyfin/jellyfin-meta/issues/29) questions broad media-kind scope. [#20](https://github.com/jellyfin/jellyfin-meta/issues/20) discusses project-wide error reporting and [#48](https://github.com/jellyfin/jellyfin-meta/issues/48) repeatable test media. [#35](https://github.com/jellyfin/jellyfin-meta/issues/35) addresses nondefault-port discovery using DNS SRV; [#103](https://github.com/jellyfin/jellyfin-meta/issues/103) requests proxy authentication headers. These complement the detailed [13.0 scanner proposal #125](https://github.com/jellyfin/jellyfin-meta/discussions/125); open planning records are not a current release promise.

Plezy discussion [#857, 13 April 2026](https://github.com/edde746/plezy/discussions/857) asks to recognize externally synced files as offline media, showing why some users want a manifest/interoperability boundary rather than only built-in downloads. [#2619, 9 October 2026](https://github.com/edde746/plezy/discussions/2619) asks for audiobook chapters despite audio already playing. [#2529, 29 September 2026](https://github.com/edde746/plezy/discussions/2529) praises the app but cannot determine what Home layout does across several libraries. [#2639, 11 October 2026](https://github.com/edde746/plezy/discussions/2639) asks whether ExoPlayer will be discontinued and whether mpv supports HDR; a question is **not evidence of a maintainer plan to discontinue it**. No separate comprehensive maintained Plezy roadmap with promised dates was established.

Two 2024 HN threads strengthen the process lessons: [10.9 trickplay activation, 12–14 May](https://news.ycombinator.com/item?id=40334173) shows users need clear activation steps even when a feature works; [funding/governance, 22 July](https://news.ycombinator.com/item?id=41031998) contains debate about donor expectations and distributing funds to client projects, including maintainer participation. It shows funding and development capacity are distinct, not that money cannot help. No legal or financial conclusion is drawn from commenters' speculation.

### Current stable point-release scars worth preserving as acceptance cases

| Public release evidence | Failure/repair class | Portico package/question |
|---|---|---|
| [12.1, 47 changes](https://github.com/jellyfin/jellyfin/releases/tag/v12.1) | Folder child sets retained too long, UserViews memory/counts, optimisation memory and pre-migration backup integrity | `ingestion`, `catalog`, `backup`, `dbwork`; Q-24.5/6/26/27 |
| 12.1 | Forced-only preferences overriding remembered tracks; device revocation not ending sessions; SyncPlay error/wait handling | `subtitles`, `identity`, `social`, `access`; Q-24.8/14/16/18 |
| 12.1 | Vulkan tone-map colour-range conversion and source/EPG corner cases | `decoder`, `playback`, `livechannels`; Q-24.10/24/25 |
| [12.2, 51 changes](https://github.com/jellyfin/jellyfin/releases/tag/v12.2) | Malformed FLAC hang, trickplay context disposal, orphan values and partial extras indexes | `audiofacts`, `mediaanalysis`, `catalog`, `compactcatalog`; Q-24.5/6/21/27 |
| 12.2 | Segment cleaner deleting ahead of current MKV remux playback | `playback`, `artifactlease`, `linearbuffer`; Q-24.7/25 |
| 12.2 | Missing/empty database on an already configured server must fail startup, not silently look new | `persistence`, `identity`, `backup`; Q-24.2/26/28 |
| 12.2 | Case/multiscript search, inherited parental ratings, library-renaming preferences and subtitle/video format details | `catalog`, `identity`, shared preferences and native player; Q-24.14/16/20 |

These are release-noted repairs, not accusations of the same Portico bugs. They illustrate why migration/read/scanning/media correctness is continuing maintenance even after the big refactor ships.

## Evidence boundary

The audit establishes source shapes, actual tracker counts/status, release chronology, selected maintainer positions and dated community reports. It establishes **no runtime reproduction, measured Portico throughput, physical decoder/display correctness, security exploitability, shipped store availability, contributor productivity or market share**. No Portico test/server/client was run; no production/demo/Hosted endpoint was contacted. No comparison repository code was executed. Only this Markdown output was written under Portico; research scripts/public snapshots live in `/tmp`.

The top 120 active feature requests and 40 completed/15 declined records are individually recorded, plus 18 supplementary active requests. 500 active requests were captured, not all 500 deeply analysed or all several thousand comments read. The open issue appendix contains 240 unique issue records across five ranked sample unions, plus 30 globally ranked Plezy closed issues; selected bodies, release resolutions and discussion texts informed the recurring classes. Sampling cannot establish relative defect rates across products with different user/device populations or reporting rules. Reddit/HN/Lemmy accounts can misattribute media/setup failures to software; corrections and competing experiences are retained.

Jellyfin code API/EF details beyond the release/scanner evidence, Plezy's every native player branch, complete independent-client roadmaps, paid-service finances, present VIDAA business-registration rules, every NAS/store package and every security advisory were not exhaustively reviewed. Dim's stale release/push evidence supports caution, not a definitive declaration of abandonment. LTS and compatibility recommendations are this researcher's proposed decisions, not assertions of guarantees the peers never made. Roku implementation is intentionally outside this domain.

Source line numbers refer to the working trees read during this run. Reported HEADs are orientation, **not proof the trees were clean or identical to those commits**: Portico server `82aaa54f89efdc2e029b635746627cd8c8b71960`, native `a3d8e7cd14c16229715a07c67f43cb8364bae743`; comparison Jellyfin `a45d1415d4da7c9b69d6e7a6895fc63d2d68e8f1`, Plezy `ce361d90804689b462a08fe995419a00af7ccb13`. Public stable releases were checked separately from clone HEADs. Source files could change during a multi-researcher audit; findings should be reconciled against the final reviewed revision before implementation.

**Index:** 40 L entries, 25 F entries, 31 Q entries; no separately numbered B entries because this domain intentionally records bug-derived questions. Q-24.3 is the strongest probable existing Portico bug identified here (Medium, already in Backlog). Q-24.23 also records a confirmed contract/schema rating disagreement, not a reproduced runtime defect. Every Q names the relevant package and distinguishes the inspected mitigation from unestablished acceptance.
