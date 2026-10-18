# Independent themes — Portico comparative architecture audit

Completed report, 11 October 2026. Independent clustering of the original domain evidence; no other reviewer’s clustering was consulted. This report synthesizes static source audits, not runtime verification.

## Reading guide and evidence boundary

All **1,250** entries in A-index.txt are placed once: **1,210 A entries and 40 L-24 lessons**. The primary register in section 1 and the local register in section 2 are exhaustive; references in analysis, process patterns, strengths and contradictions are secondary references and do not create extra placements. There are 15 themes. Themes describe bets, not teams or feature lists: an entry belongs where its main trade-off is explained, even if it also touches another theme.

I read the “How Portico does it today” section of each of the 25 documents, the entire A index and B index, and fuller entries where the index concealed a boundary or conflict. I read Overview, decisions.md and architecture.md, plus relevant passages in the client, playback, catalogue, recommendation and offline contracts. I did not consult another reviewer's themes or verification reports. This is an independent synthesis of the original auditors' evidence, not a second code verification of all claims. No Portico builds or tests, comparison-repository execution, browser, MCP, subagents or Portico endpoints were used. Small read-only Python checks reconciled the report registers and coverage totals. Only this report was written.

“Deliberate” means there is a written product choice or a clearly identified implementation rationale. It does not mean the implementation is complete or the costs have been accepted on real devices. Product Contracts/architecture.md:3 explicitly describes normative rebuild architecture, not existing mechanisms; decisions.md:59 likewise does not certify implementation. Overview is a description that can be stale. Justin's current philosophy and explicit choices take precedence over older contradictory contracts. The required context-sync invocation failed on its read-only lock path; existing local guidance was used.

The most useful organization is around three questions: **what remains under the owner's control; where Portico insists on exactness; and how much behaviour can be shared without sacrificing the actual device experience**. The fifteen bets below separate those questions enough to make decisions concrete. There is no evidence that every member entry was itself consciously chosen: some are costs, omissions or counterexamples to the larger bet.

## 1. The themes

Theme counts include lessons as well as architectural differences. Each theme's member register follows its analysis. Grouping by evidence source within a theme preserves the original domain detail without making domains the report's governing structure.

### T01 — A free local core, with Hosted kept small (72 members)

**The bet.** The owner's server remains a useful, free product in its own right. Hosted supplies identity, names and certificates that simplify connecting, while ordinary media and already established sessions stay local. A copyable state folder and conventional installation are meant to keep ownership tangible rather than dependent on a subscription.

**Counter-position.** Jellyfin puts even more of identity and remote setup on the owner; Plex's central account, discovery and relay make the remote household journey easier but create a larger service dependency (A-23.1–4). Plezy is a client of those backends, not a replacement for their local server or account infrastructure; Infuse can instead own an aggregated catalogue on the device (A-23.5–6). Those products put convenience at different points in the system.

**Buys and costs.** Direct sign-in, local renewal, restrained telemetry, inspectable SQLite and certificate assistance are real structural advantages (A-01.37; A-09.15, A-09.25, A-09.34, A-09.59; A-22.1, A-22.44). Port forwarding still fails for some CGNAT homes; no relay, root-only origins and one selected server constrain deployment and multi-server use (A-09.27, A-09.31; A-22.17–23; A-23.2–6). One state volume also concentrates backup, cache and snapshot capacity (A-09.13, A-09.47).

**Intent and Justin's philosophy.** Strongly deliberate: Overview:41, 50, 52; decisions.md P02/P20/P22 at lines 10, 28, 30. This is the strongest Jellyfin-like freedom bet, with Plex-like connective conveniences. Its polish depends on truthful reachability diagnostics and a clear supported deployment matrix.

**Inconsistent application.** Automatic networking is more capable than the phrase “port forwarding” suggests, but proxy, overlay and Windows paths get unequal treatment (A-09.21–27; A-22.17–18). A remote-access convenience is coupled to claiming even when the owner has a certificate (A-09.31). “Copyable” does not prove cross-OS restore acceptance (L-24.7). Public HTTP is disputed between auditors; do not use the security document alone to characterize it.

**Decisions for Justin.** Which remote journey must launch support: documented direct/proxy/VPN access; direct access plus a separately funded optional relay; or a relay-backed default? Should state-folder simplicity remain absolute, or remain the default with explicit cache/backup destinations? Is one selected server the intended product, or should independently scoped simultaneous servers be added before any merged catalogue?

**Member entries**

#### 01 Identity and access

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-01.39** — One signed-in server at a time on a device

#### 04 Subtitles and analysis

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-04.4** — Subtitles never written beside the media

#### 05 Inventory and naming

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-05.13** — Libraries and folders
- **A-05.35** — Remote storage is a first-class back-end

#### 07 Persistence and queries

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-07.28** — Database engine

#### 09 Server operations

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-09.1** — One self-contained binary that is also its own helper
- **A-09.2** — The web app is a folder on disk, not part of the binary
- **A-09.3** — Windows: a boot-time Scheduled Task running as SYSTEM
- **A-09.4** — macOS: a launcher app that installs a per-user background agent
- **A-09.5** — Linux: packages with a locked-down systemd unit, but no repository
- **A-09.6** — Docker: non-root, an init process, and a built-in health check
- **A-09.7** — Platform coverage
- **A-09.10** — Configuration: environment variables plus one database document, no file
- **A-09.13** — One state folder versus separate config, data, cache and log directories
- **A-09.14** — Moving a server: absolute paths rewritten at start
- **A-09.15** — Nothing encrypted at rest, with a permissions warning instead
- **A-09.20** — One port for HTTP and HTTPS
- **A-09.21** — Plain HTTP is refused to non-private peers at the socket
- **A-09.22** — Listening: one explicit IP, IPv4 by default
- **A-09.23** — Automatic port mapping
- **A-09.24** — Finding the public address
- **A-09.25** — Names and certificates through Hosted
- **A-09.26** — Bringing your own certificate
- **A-09.27** — Reverse proxies and sub-path hosting
- **A-09.28** — Deciding whether a request is local or remote
- **A-09.29** — Controls for remote use
- **A-09.30** — Finding servers on the LAN
- **A-09.31** — Remote access needs a claimed server
- **A-09.32** — HTTP versions and sockets
- **A-09.34** — Talking to the vendor's service
- **A-09.46** — What a backup is
- **A-09.47** — Scheduling, retention and location
- **A-09.55** — One server per state folder
- **A-09.59** — Telemetry and privacy posture

#### 11 Live TV and DVR

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-11.22** — Recording storage is content-addressed and private
- **A-11.40** — Credentials at rest

#### 13 Social playback and receivers

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-13.35** — Cast needs HTTPS, and one shared receiver page

#### 15 Security

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-15.33** — A central account service exists at all
- **A-15.36** — Plain HTTP as a first-class transport
- **A-15.38** — Encryption at rest

#### 16 Web architecture

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-16.18** — The app assumes it is served from the root of the origin
- **A-16.47** — Multi-server on the web

#### 18 Web experience

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-18.30** — Networking

#### 21 Native experience

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-21.4** — Multiple servers are separate navigation universes
- **A-21.5** — Hosted account authorization can succeed without a usable server
- **A-21.8** — Remembered direct servers do not silently retain cross-server login
- **A-21.55** — Direct-file clients move catalog ownership into the app

#### 22 Connectivity

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-22.1** — A direct data plane with an optional account control plane
- **A-22.2** — Bonjour/NSD versus a product-specific UDP discovery request
- **A-22.15** — HTTP remains first-class, public HTTP is refused
- **A-22.16** — A secure-LAN DNS alias replaces the raw IP
- **A-22.17** — Private-address classification excludes Tailscale's IPv4 range
- **A-22.18** — Reverse-proxy origins supported, subpaths refused
- **A-22.19** — No relay is an architectural and product boundary
- **A-22.20** — Remote reachability is not inferred from a certificate or router lease
- **A-22.21** — DNS names are stateful current addresses with membership-label rotation
- **A-22.22** — One active server instead of simultaneous catalogue aggregation
- **A-22.23** — Per-server failure isolation has different meaning
- **A-22.44** — Server-list checks are version probes, not presence polling
- **A-22.45** — Server-side Hosted retries persist and spread fleet recovery
- **A-22.46** — Wakes are signed hints with durable acknowledgement tracking
- **A-22.48** — Explicit URL/interface preferences versus automatic route hints

#### 23 Adjacent competitors

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-23.1** — Optional Hosted identity versus Plex's normal central account
- **A-23.2** — Certificates and discovery are not a relay
- **A-23.3** — Relay availability versus relay quality
- **A-23.4** — Remote-payment enforcement versus infrastructure payment
- **A-23.6** — Infuse's local combined library versus on-demand server mode
- **A-23.25** — Commercial catalogs carry different incentives and obligations

#### 24 Community lessons

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **L-24.29** — Remote access combines discovery, reachability and authorization

#### 25 Engineering

Local ownership, deployment freedom and the boundary of Hosted convenience.

- **A-25.35** — OS installers implement different service models
- **A-25.57** — Free core plus future relay has distinct sustainability economics
- **A-25.58** — Local telemetry versus automatic crash/installation reporting

### T02 — The household profile is the authority and privacy boundary (75 members)

**The bet.** A household account contains profiles, and the selected profile is a real server authority, not just a local avatar. Library visibility, personal state, social membership and private resources should agree about that person across every surface. Hosted proves identity while the local server decides membership and access.

**Counter-position.** Jellyfin's flatter user model and numerous per-user permissions are simpler for independent adults but less naturally household-shaped (A-01.1–8). Plex makes household profiles familiar while central identity simplifies sharing; its broader public social model makes different disclosure choices (A-23.21–24). Plezy inherits backend authority and adds client convenience rather than enforcing a new household policy.

**Buys and costs.** SQL-level restrictions, separate account/viewing credentials, personal subtitles, private DVR and protected offline ownership reduce accidental cross-person sharing (A-01.10, A-01.16; A-04.26; A-11.18, A-11.37; A-14.31). The same-profile receiver rule and dual restriction engines can frustrate households; owner and viewer histories also create a privacy boundary people may misunderstand (A-01.5, A-01.45; A-13.10, A-13.26; A-12.16–18).

**Intent and Justin's philosophy.** Deliberate foundation: decisions.md P01/P02/P15/P20; architecture.md:13, 31 and 65 distinguish identity, authorization and projections. It fits freedom to share selectively and Plex-like profile polish, provided families can predict the policy.

**Inconsistent application.** Account and profile rating ladders disagree (A-01.5–8); authority is sometimes checked by URL prefix or sign-in mode rather than the action's effective permission (A-01.12; A-18.23–24; A-21.48). Hiding a group participant and preserving owner history after erasure need explicit privacy meaning (A-13.23; A-01.45). A Cast token with the pairing person's full role narrows the value of otherwise careful receiver permissions (A-13.32). Broad native account administration is not equivalent to successfully enforcing those boundaries.

**Decisions for Justin.** Keep one profile as the exact receiver/group unit, allow household-account handoff with explicit history attribution, or add a separate receiver identity? Should owner account limits and profile parental controls share one engine with distinct editors? Should privacy mean “no personal history,” “resume but no history,” or “no household aggregate signal,” and what must the screen tell viewers about owner history?

**Member entries**

#### 01 Identity and access

Household scope, effective permissions and who may see or control whose resources.

- **A-01.1** — Accounts with profiles underneath, versus a flat list of users
- **A-01.2** — Three fixed tiers with exactly one owner, versus an administrator flag
- **A-01.4** — New members start with no libraries; Jellyfin users start with all
- **A-01.5** — Two restriction systems in Portico (per profile and per account), one in Jellyfin
- **A-01.6** — Ratings compared by "minimum age" from a small built-in table, versus per-country score tables
- **A-01.7** — One "allow unrated" switch, versus per-type unrated blocking
- **A-01.8** — Blocked labels only, versus blocked tags plus an allow list, inherited from parents
- **A-01.9** — No owner override of a title's rating
- **A-01.10** — Restrictions compiled into catalogue SQL, with one predicate for every surface
- **A-01.11** — Access schedules: per account and enforced at the door, versus per user and enforced on every request
- **A-01.12** — Feature switches enforced by URL prefix
- **A-01.16** — Three kinds of credential with different reach
- **A-01.17** — Every request re-checks the account, profile and device in the database
- **A-01.24** — Built-in TOTP second factor on the server itself
- **A-01.26** — Profile PINs are the only wall between profiles
- **A-01.27** — Switching profile creates a new session; the old profile's playback is ended on the server only if the client asks
- **A-01.31** — Device records keyed by an installation id; two device lists
- **A-01.32** — Session history keeps the reason a session ended
- **A-01.36** — A global account that proves identity, while each server owns membership
- **A-01.37** — What breaks when things are down
- **A-01.38** — Security actions at Hosted do not reach sessions on servers
- **A-01.40** — Invitations by code
- **A-01.41** — Remote sign-in is a server-wide switch
- **A-01.43** — Stream limits count streams, not signed-in devices
- **A-01.45** — The owner's play history outlives the person
- **A-01.48** — Shared-TV behaviour
- **A-01.49** — Remembered accounts for the browser are kept on the server
- **A-01.53** — Avatars

#### 02 Playback decisions and protocol

Household scope, effective permissions and who may see or control whose resources.

- **A-02.28** — Capability URLs instead of tokens in query strings
- **A-02.35** — Two histories and a privacy switch

#### 04 Subtitles and analysis

Household scope, effective permissions and who may see or control whose resources.

- **A-04.15** — Delivery by authorised polling
- **A-04.26** — Personal versus shared subtitles, with history
- **A-04.28** — Who may manage subtitles

#### 06 Metadata and artwork

Household scope, effective permissions and who may see or control whose resources.

- **A-06.8** — A consent gate before the first provider call
- **A-06.36** — Image URLs and who may fetch them
- **A-06.50** — What leaves the server

#### 07 Persistence and queries

Household scope, effective permissions and who may see or control whose resources.

- **A-07.46** — The owner's play history

#### 10 Music and books

Household scope, effective permissions and who may see or control whose resources.

- **A-10.38** — Two histories and maintained play counts; no scrobbling, no statistics

#### 11 Live TV and DVR

Household scope, effective permissions and who may see or control whose resources.

- **A-11.18** — DVR belongs to a profile
- **A-11.37** — Parental controls on live content
- **A-11.47** — Library Channels respect profile restrictions

#### 12 Personal state and discovery

Household scope, effective permissions and who may see or control whose resources.

- **A-12.16** — Two histories: the viewer's and the owner's
- **A-12.17** — Pause watch history (incognito)
- **A-12.18** — No "watched by several people on one screen"

#### 13 Social playback and receivers

Household scope, effective permissions and who may see or control whose resources.

- **A-13.10** — A member is a profile, not a device
- **A-13.11** — How people find and join a group
- **A-13.12** — A queue that not everyone may see
- **A-13.20** — Reach: one server, no third party
- **A-13.23** — A privacy switch that hides a participant
- **A-13.26** — Only the same profile can send to a device
- **A-13.32** — How the TV gets a credential
- **A-13.34** — One controller, with a takeover prompt
- **A-13.41** — How a server is shared
- **A-13.42** — What can be restricted per person
- **A-13.43** — No public or guest links

#### 14 Downloads

Household scope, effective permissions and who may see or control whose resources.

- **A-14.31** — Ownership on a shared device

#### 15 Security

Household scope, effective permissions and who may see or control whose resources.

- **A-15.20** — Where the browser keeps the session
- **A-15.30** — Restrictions are part of the query, and every route is tested
- **A-15.31** — Who the request is "as"

#### 16 Web architecture

Household scope, effective permissions and who may see or control whose resources.

- **A-16.59** — Search and the address bar

#### 17 Web player

Household scope, effective permissions and who may see or control whose resources.

- **A-17.15** — Subtitle access is re-authorised every 25 seconds

#### 18 Web experience

Household scope, effective permissions and who may see or control whose resources.

- **A-18.23** — How permission shows in the UI
- **A-18.24** — People: accounts with profiles, invitations and device approval against users
- **A-18.48** — The account site lives inside the app

#### 19 Native architecture

Household scope, effective permissions and who may see or control whose resources.

- **A-19.14** — Ordinary preferences live in the secure store
- **A-19.41** — Shared devices: an on-device PIN vault, no system user awareness

#### 20 Native players

Household scope, effective permissions and who may see or control whose resources.

- **A-20.18** — Subtitles are re-authorised every 25 seconds

#### 21 Native experience

Household scope, effective permissions and who may see or control whose resources.

- **A-21.7** — Profile authority differs from client-local profile convenience
- **A-21.33** — Offline authorization is a visible scoped state rather than permanent copy
- **A-21.44** — Scoped system shelf feed versus upstream token/catalog fetch
- **A-21.51** — Kids restrictions are server policy, not a separate playful shell

#### 23 Adjacent competitors

Household scope, effective permissions and who may see or control whose resources.

- **A-23.21** — PIN profiles and household boundaries
- **A-23.22** — Sharing restrictions as authoritative query policy
- **A-23.29** — Built-in history versus independent statistics service

#### 24 Community lessons

Household scope, effective permissions and who may see or control whose resources.

- **L-24.23** — Privacy claims need to explain each boundary

### T03 — People choose policy and quality (83 members)

**The bet.** People should be able to express an intention and have it honored without hidden product arbitration. Quality is chosen explicitly, correct credentials do not cause a permanent lockout, and the owner decides ceilings and operating policy. Low ceremony means sensible defaults and a small understandable control set, rather than maximum switches everywhere.

**Counter-position.** Jellyfin exposes many permission and hardware knobs; Plex supplies more automatic bandwidth adjustment and routine convenience (A-01.3; A-02.18–20; A-18.25, A-18.31; A-23.15). Plezy offers more local playback overrides and presets where backend detection or device quirks demand them (A-20.8). Neither automatic network adaptation nor fine controls are inherently wrong; they allocate responsibility differently.

**Buys and costs.** Owner-set limits, typed refusal, delivery preferences, no permanent lockout and manual updates are predictable (A-01.21, A-01.23; A-02.12–13, A-02.31; A-09.11–12, A-09.51). Fixed quality can leave a viewer buffering; very sparse controls can prevent correcting a wrong device claim; no email or automatic update path shifts attention back to the owner. Password simplicity and open first-run setup carry residual risk even when operating exactly as intended (A-01.29; A-15.8–15).

**Intent and Justin's philosophy.** Explicit in Overview:51, Playback and Media:3 and decisions.md P07/P19. Strong freedom/choice alignment, but Justin's slight ease-of-use preference argues for suggested actions and intelligible defaults. It does not authorize silently changing his deliberate HTTP, lockout or at-rest choices.

**Inconsistent application.** “Automatic” is shown without the intended bandwidth policy being connected (A-02.19; A-17.18; A-20.27). Remux is unexpectedly blocked by the transcode switch (A-02.32). Permission edits sign people out, and schedules are treated as session-ending rather than temporary policy (A-01.11, A-01.13). Background analysis stops for playback despite the “never paused” rule (A-05.34; A-23.13), while maintenance-window wording may promise the opposite operating model.

**Decisions for Justin.** For a weak connection, keep manual quality alone, offer a consent-required suggestion, or add a viewer-selected adaptive mode? For background work, guarantee eventual lower-priority progress, or permit explicit source/maintenance pauses? Keep fixed minimal controls, add a few repair overrides, or expose an advanced page? Which policy edits should change access live and which should revoke sessions?

**Member entries**

#### 01 Identity and access

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-01.3** — A few coarse capabilities, versus forty per-user permission switches
- **A-01.13** — Changing someone's permissions signs them out; Jellyfin applies changes live
- **A-01.21** — Cool-off instead of lockout
- **A-01.23** — bcrypt cost 10 with an 8-character minimum, versus PBKDF2 with 210,000 iterations and empty passwords allowed
- **A-01.25** — Password reset: owner resets with forced change and a host-side recovery command, versus a PIN file
- **A-01.29** — First-run setup is open to whoever reaches the server first
- **A-01.30** — A local recovery owner whose password only works from a private network

#### 02 Playback decisions and protocol

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-02.11** — An owner "planning policy"
- **A-02.12** — Viewer delivery preferences with require and never
- **A-02.13** — Refusal is a first-class outcome
- **A-02.18** — Quality changes only when a person asks
- **A-02.19** — "Automatic" quality is specified as client-side, built, and unused
- **A-02.20** — How a ceiling is compared with a file
- **A-02.21** — Two different "is this remote?" answers
- **A-02.31** — Stream limits and conversion caps
- **A-02.32** — Transcoding permission is global, not per member

#### 03 Transcoding

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-03.24** — Master playlist: single variant, no fallbacks
- **A-03.25** — No adaptive bitrate
- **A-03.29** — Temporary storage location

#### 04 Subtitles and analysis

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-04.33** — Marker types and preferences

#### 05 Inventory and naming

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-05.24** — Ignore rules
- **A-05.40** — Order, grouping and visibility of libraries
- **A-05.42** — Per-library options
- **A-05.48** — What kinds of re-scan an owner can ask for

#### 07 Persistence and queries

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-07.21** — Owner-tunable database settings

#### 08 API and events

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-08.30** — Background work that people can see

#### 09 Server operations

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-09.8** — FFmpeg is bundled, pinned and verified
- **A-09.11** — Settings are a typed, revisioned registry
- **A-09.12** — What needs a restart
- **A-09.16** — What first-run setup asks
- **A-09.17** — Who can reach an unconfigured server
- **A-09.19** — Getting back in when the owner is locked out
- **A-09.38** — Scheduled work
- **A-09.49** — How the server restarts itself
- **A-09.51** — Updates
- **A-09.52** — Downgrades

#### 10 Music and books

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-10.31** — Advancing policy lives on the server and in native code

#### 11 Live TV and DVR

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-11.17** — Conflict policy
- **A-11.35** — Favourite and hidden channels
- **A-11.48** — Settings that exist only as documents

#### 12 Personal state and discovery

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-12.5** — Thresholds are per library and per profile; Jellyfin's are server-wide
- **A-12.6** — "Started" is a time floor capped by a percentage; Jellyfin's is a pure percentage plus a minimum length
- **A-12.9** — Very short videos
- **A-12.11** — Rewatching clears the watched mark
- **A-12.20** — One merged row, no option to split
- **A-12.22** — Staleness window is the owner's, per library, and also applies to unfinished films
- **A-12.24** — Specials
- **A-12.25** — No rewatch mode
- **A-12.28** — Row size is the owner's, and the smallest library setting wins
- **A-12.40** — Recently added: per library, one card per work, watched titles included
- **A-12.58** — One registry, two scopes, server-stored
- **A-12.59** — The owner cannot set or lock viewers' preferences
- **A-12.60** — Per-profile language and subtitle defaults

#### 13 Social playback and receivers

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-13.2** — Nobody waits for anybody
- **A-13.7** — Who may control playback
- **A-13.8** — The host role and what happens when the host goes away
- **A-13.18** — Playback speed is a group property in the protocol only
- **A-13.21** — Limits
- **A-13.24** — Per-profile feature switch
- **A-13.31** — "Allow this phone?" on the TV

#### 15 Security

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-15.8** — Password hashing
- **A-15.9** — Password rules
- **A-15.10** — Wrong-password handling
- **A-15.12** — Second factor
- **A-15.14** — First-run setup
- **A-15.15** — Owner recovery

#### 17 Web player

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-17.18** — Quality is manual only, and "Automatic" means "whatever the server plans"
- **A-17.27** — Video stops when you leave the player
- **A-17.41** — Aspect, zoom and brightness
- **A-17.42** — Subtitle appearance options

#### 18 Web experience

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-18.25** — Library options: a dozen choices against forty
- **A-18.26** — Dashboard and server control
- **A-18.31** — Transcoding settings

#### 20 Native players

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-20.8** — No user override when detection is wrong
- **A-20.27** — Quality is one rendition; "Automatic" is the server's choice

#### 21 Native experience

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-21.10** — TV administration deliberately resolves only Dashboard
- **A-21.53** — Device tuning is mostly owner policy or implicit capability

#### 23 Adjacent competitors

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-23.13** — Scheduled heavy analyses versus always-deprioritized work
- **A-23.14** — Stored optimized copies: lifecycle matters more than existence
- **A-23.15** — Fixed user-requested quality versus bandwidth adaptation
- **A-23.17** — Hardware/HDR capability is not a premium permission

#### 25 Engineering

Explicit human choices, defaults, operating policy and the consequences of limited controls.

- **A-25.12** — Configuration uses typed owner documents and environment plumbing
- **A-25.38** — Update notices are owner-configured and background-fetched

### T04 — Make large libraries cheap by doing work before the page opens (109 members)

**The bet.** Large libraries should cost about as much to browse as small ones. Portico stores narrow facts, maintains browse/search/count projections in background, and pages windows all the way into clients and queues. Resource admission and write priority protect already active use rather than assuming every request can run at once.

**Counter-position.** The audits describe more query-time enumeration, offset paging and client materialization in Jellyfin and some of its clients, while also documenting Jellyfin's recent work to reduce those costs (A-07.12–20; L-24.1–4). Plex establishes the desired responsiveness but its internal cost model cannot be inspected here. Plezy's richer locally owned state and specialist servers' smaller domains are different ways to buy responsiveness; neither proves Portico's million-item target.

**Buys and costs.** Counted blocks, exact totals, windowed grids, formula shuffle and prioritized SQLite work are unusually coherent (A-07.13–14, A-07.17, A-07.20; A-10.28–29; A-16.24–25; A-22.34). Maintaining all those projections costs disk, write work and repair machinery. Formula shuffle can make “Play next” surprising; fixed scanner/provider cadence can dominate throughput regardless of available hardware (A-05.3; A-06.4, A-06.14; A-10.29).

**Intent and Justin's philosophy.** Deliberate: Overview:48, 53; Clients:68; architecture.md:27, 73. It serves both weak owner hardware and polished browsing across platforms. It is an architectural direction, not verified capacity.

**Inconsistent application.** Some history/resource lists accumulate or stop at a prefix while ordinary browsing windows correctly (A-19.28; A-21.24, A-21.30). Hard guide/source caps contradict complete-result ambitions (A-11.7). Live input cost grows per viewer (A-11.1), despite another document claiming sharing. Long snapshots, derived batches and startup backfills can put background work back in the foreground's way (L-24.6; A-25.43; B-07.13–14, B-07.18).

**Decisions for Justin.** Keep all exact projection machinery, or simplify specific expensive views only after measurements? Should shuffle use a formula body plus an explicit next-up lane? Should all ordinary lists be windowed before launch, or is a clearly stated smaller supported scale acceptable? Which scale/hardware workload must be accepted before claiming the target?

**Member entries**

#### 03 Transcoding

Work bounds, maintained projections, windows and foreground resource protection.

- **A-03.1** — Throttle by killing FFmpeg versus pausing it
- **A-03.30** — Disk budgets
- **A-03.31** — Segment retention default
- **A-03.33** — Concurrency limits and queueing
- **A-03.41** — Trickplay and still extraction
- **A-03.47** — Consumer NVIDIA session limits
- **A-03.50** — Process priority

#### 04 Subtitles and analysis

Work bounds, maintained projections, windows and foreground resource protection.

- **A-04.2** — When text is extracted: at scan, per stream, versus on demand, per file
- **A-04.36** — Trickplay pipeline
- **A-04.37** — Trickplay geometry
- **A-04.38** — Trickplay storage
- **A-04.41** — How analysis is scheduled
- **A-04.42** — Deep analysis depends on subtitle import
- **A-04.43** — Analysis products nobody reads
- **A-04.47** — Limits

#### 05 Inventory and naming

Work bounds, maintained projections, windows and foreground resource protection.

- **A-05.3** — Scan speed is set by a clock, not by the disk
- **A-05.33** — Scan tiers as promises about disk reads
- **A-05.34** — Analysis stops while anything plays from the same source
- **A-05.36** — How a remote is listed
- **A-05.47** — Hard links and the same file in two places

#### 06 Metadata and artwork

Work bounds, maintained projections, windows and foreground resource protection.

- **A-06.3** — Matching and Fix Match are queued jobs, versus an interactive request
- **A-06.4** — One request a second, one title a second, versus unthrottled provider calls
- **A-06.14** — Episodes: one work row and one request per episode (TMDB)
- **A-06.26** — Local artwork is copied into a capped cache
- **A-06.35** — Pure-Go image code with hard limits, versus a native library
- **A-06.40** — Always fetching TMDB "original" size
- **A-06.42** — Client image loading

#### 07 Persistence and queries

Work bounds, maintained projections, windows and foreground resource protection.

- **A-07.1** — Kinds as data with narrow side tables, versus one wide table with a type string
- **A-07.6** — Shape of per-viewer state
- **A-07.9** — Hierarchy
- **A-07.12** — Derived tables kept by a queue and a worker, versus computing at query time
- **A-07.13** — Totals
- **A-07.14** — Reaching position N
- **A-07.17** — Write ordering: one gate with a priority ladder, versus selectable locking modes
- **A-07.20** — Write-ahead log growth
- **A-07.22** — Page cache and memory mapping
- **A-07.23** — Planner statistics
- **A-07.24** — Reclaiming space
- **A-07.27** — What an upgrade does to derived data
- **A-07.29** — Where the logic lives
- **A-07.30** — Filter model: a boolean tree, versus a flat list of fixed parameters
- **A-07.31** — Which facts can be filtered on
- **A-07.33** — Sorts
- **A-07.34** — The alphabet rail
- **A-07.35** — Filter option lists (facets)
- **A-07.36** — Search matching
- **A-07.38** — Search response shape
- **A-07.41** — Parental restrictions in queries
- **A-07.44** — Orphaned vocabulary and people
- **A-07.47** — Guard rails in tests

#### 08 API and events

Work bounds, maintained projections, windows and foreground resource protection.

- **A-08.19** — Admission control and back-pressure
- **A-08.34** — Response compression

#### 09 Server operations

Work bounds, maintained projections, windows and foreground resource protection.

- **A-09.33** — Overload protection is designed in
- **A-09.53** — Resource controls
- **A-09.54** — When the disk fills
- **A-09.57** — Looking after the database

#### 10 Music and books

Work bounds, maintained projections, windows and foreground resource protection.

- **A-10.28** — The queue is a server resource
- **A-10.29** — Shuffle is a formula, so edits behave differently

#### 11 Live TV and DVR

Work bounds, maintained projections, windows and foreground resource protection.

- **A-11.1** — One input per viewer, not per channel
- **A-11.7** — Hard size limits on sources
- **A-11.31** — Guide API shape
- **A-11.32** — Guide grid rendering
- **A-11.38** — Guide refresh scheduling
- **A-11.45** — Library Channels: one FFmpeg per viewer

#### 12 Personal state and discovery

Work bounds, maintained projections, windows and foreground resource protection.

- **A-12.1** — Watched is computed from intents and container defaults, not stored per item
- **A-12.36** — The server composes Home; clients only draw it
- **A-12.44** — Exact totals and resumable paging on every row
- **A-12.53** — Cost model
- **A-12.55** — Search engine: token-prefix full-text on one title
- **A-12.56** — Search groups and budgets

#### 14 Downloads

Work bounds, maintained projections, windows and foreground resource protection.

- **A-14.18** — Server-wide and per-device concurrency

#### 16 Web architecture

Work bounds, maintained projections, windows and foreground resource protection.

- **A-16.20** — First-load weight
- **A-16.24** — List virtualisation versus pagination
- **A-16.25** — Letter jumps
- **A-16.28** — Image sizes and device pixel ratio
- **A-16.30** — Home is rendered in full

#### 17 Web player

Work bounds, maintained projections, windows and foreground resource protection.

- **A-17.33** — A pre-play read before every video start
- **A-17.37** — Trickplay tiles are fetched through the authorised artwork cache

#### 18 Web experience

Work bounds, maintained projections, windows and foreground resource protection.

- **A-18.6** — A windowed endless grid against numbered pages
- **A-18.16** — Bulk actions as server jobs
- **A-18.39** — Five ways to page

#### 19 Native architecture

Work bounds, maintained projections, windows and foreground resource protection.

- **A-19.26** — Lists: RN's built-in virtualisation plus a custom windowed grid
- **A-19.27** — Random-access paging with placeholders
- **A-19.28** — The Saved History tab and non-windowed screens
- **A-19.31** — No device performance tiers

#### 20 Native players

Work bounds, maintained projections, windows and foreground resource protection.

- **A-20.35** — Start-up makes an extra request to decide whether to warn about Dolby Vision

#### 21 Native experience

Work bounds, maintained projections, windows and foreground resource protection.

- **A-21.20** — Browse capability vocabulary comes from server
- **A-21.23** — Alphabet and jump use bounded browser windows
- **A-21.24** — Whole-container playback and loaded-view playback differ
- **A-21.29** — Playlist mutations can be windowed, but playback loses occurrence identity
- **A-21.30** — Personal history and resources fall outside the windowed browsing pattern

#### 22 Connectivity

Work bounds, maintained projections, windows and foreground resource protection.

- **A-22.29** — Native transport admission is bounded FIFO
- **A-22.30** — Browser and native response memory boundaries differ
- **A-22.33** — No universal selected-server circuit breaker
- **A-22.34** — Server overload is typed and resource-classed

#### 24 Community lessons

Work bounds, maintained projections, windows and foreground resource protection.

- **L-24.1** — Centralize persistence before shipping years of user data
- **L-24.2** — A connection pool is not a concurrency policy
- **L-24.3** — Bounded pages must reach all the way down to storage
- **L-24.4** — Database modernization can expose, rather than cure, bad queries
- **L-24.6** — Startup work needs progress and a deliberate operator path
- **L-24.17** — Resource preparation should be optional, incremental and visible
- **L-24.33** — Maintenance scheduling must preserve active playback
- **L-24.37** — Demand can regress when an implementation violates query budgets

#### 25 Engineering

Work bounds, maintained projections, windows and foreground resource protection.

- **A-25.8** — SQL is owned by domains, not only repositories
- **A-25.9** — Request context is a service view
- **A-25.13** — Pure-Go SQLite enables cross-compilation
- **A-25.19** — Product-specific guards versus general analyzers
- **A-25.22** — Scale is encoded in test design
- **A-25.43** — Additive migrations favor large-library availability

### T05 — Every action has an identity, a revision, and a recovery story (150 members)

**The bet.** A request is a durable intention, not merely a transient HTTP call. Revisions, operation IDs, leases, generations and immutable receipts should make retries, concurrent devices and restart outcomes explicit. Old work must never masquerade as the current action.

**Counter-position.** Jellyfin often uses simpler mutable session state and start/progress/stop calls; permanent tokens avoid the refresh protocol Portico must maintain (A-01.14, A-01.18; A-02.24–29). Plex's smooth handoff/recovery is the experience target, with inaccessible internal guarantees. Plezy contains substantial pragmatic recovery for its own players and stores, but the audits find no equivalent to Portico's leased command/fact native bridge (A-20.3).

**Buys and costs.** Retry-safe refresh, canonical progress writers, resumable scans, staged restore and prepared handoff are strong foundations (A-01.18; A-02.36; A-05.1; A-09.48; A-13.29). They add many wire dialects, state transitions, writer rules and storage dependencies. Narrow certainty can turn a playable approximate seek or a harmless concurrent change into a visible refusal (A-08.11–12; A-12.14–15; A-20.22).

**Intent and Justin's philosophy.** Deliberate and central: architecture.md:31–45 and 51–59; decisions.md R02/R07/R09/R12/R20. This serves polish when recovery is automatic and understandable, and freedom when the owner can inspect what happened. Exact internal guarantees need not become extra user ceremony.

**Inconsistent application.** Offline progress bypasses online intent ordering; container flags use lighter mutation semantics; catch-all revisions mix progress and control (A-12.15, A-12.63; A-14.12, A-14.29; A-13.13). Legacy playback adapters may discard queues or fail to recover a lapsed lease even though a stricter controller exists (A-17.20, A-17.34–36). Handoff starts the target audibly before committing, contrary to paused/unpresented readiness (A-13.29; decisions R07). Signed binary identity also stops short of the final package (A-25.31–33).

**Decisions for Justin.** Where is strict compare-and-swap essential, and where is a scoped simple boolean update acceptable? Should control, observation, progress and authorization get separate revisions? Finish one playback controller, preserve a deliberately smaller live bridge, or retire the unused one? What exact retry/recovery guarantees will independent clients be promised at 1.0?

**Member entries**

#### 01 Identity and access

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-01.14** — Short access token plus rotating refresh token, bound to a device, versus one permanent token
- **A-01.15** — Tokens stored as digests; Jellyfin stores them in plain text
- **A-01.18** — Idempotent refresh with a stored answer
- **A-01.19** — What a client does when its token is refused
- **A-01.20** — Signing out a family from any of its old tokens
- **A-01.28** — TV code: durable, eight characters, approval on the server's own page
- **A-01.51** — Administration writes carry a revision and an operation id

#### 02 Playback decisions and protocol

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-02.8** — Four named routes with a recorded reason for every closed route
- **A-02.23** — The server remembers a failed route
- **A-02.24** — A playback session is its own resource
- **A-02.26** — Lease and cleanup timing
- **A-02.27** — Start can answer "being prepared"
- **A-02.29** — A plan is pinned to the exact file
- **A-02.30** — Seeking
- **A-02.34** — Re-watching clears "watched"
- **A-02.36** — Cross-device ordering of progress writers
- **A-02.47** — Multi-part titles

#### 03 Transcoding

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-03.3** — Copied video: keyframe "at or before" the grid line, cut by the segment muxer
- **A-03.4** — A source that cannot be indexed is converted instead of remuxed
- **A-03.26** — Restart rule on seek
- **A-03.28** — Live output through an HTTP PUT gateway
- **A-03.32** — Idle and end-of-session handling
- **A-03.35** — Hardware failure handling
- **A-03.38** — fMP4 handled by the server, not just FFmpeg
- **A-03.43** — A second, dormant engine
- **A-03.44** — Seek position for a copied stream

#### 04 Subtitles and analysis

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-04.17** — Timing offset is server state
- **A-04.35** — Skips are reported to the server

#### 05 Inventory and naming

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-05.1** — A durable, paged state machine instead of an in-process walk
- **A-05.6** — A scan must see a still tree, or it fails
- **A-05.10** — Protecting the library when a drive disappears
- **A-05.11** — What happens to a file that is really gone
- **A-05.38** — Reporting progress and problems
- **A-05.39** — Deleting media from the UI
- **A-05.45** — Re-reading old problems when the reader improves
- **A-05.46** — A numbered change feed

#### 06 Metadata and artwork

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-06.21** — Optimistic concurrency on edits

#### 07 Persistence and queries

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-07.15** — Paging contract: signed, revision-bound cursors and anchored ranges, versus start index and limit
- **A-07.18** — One snapshot per request
- **A-07.19** — Durability
- **A-07.25** — Integrity checking and repair
- **A-07.26** — Migration mechanics
- **A-07.43** — Backup of the database
- **A-07.49** — Time-bounded cursors

#### 08 API and events

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-08.9** — Error model
- **A-08.10** — Paging: signed, revision-fenced cursors versus offsets
- **A-08.11** — Optimistic concurrency
- **A-08.12** — Idempotency
- **A-08.23** — Durable, resumable event log versus ephemeral push
- **A-08.26** — Keep-alive and stream lifetime
- **A-08.46** — Slow or stalled stream consumers

#### 09 Server operations

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-09.9** — Release integrity: strong provenance, unsigned installers
- **A-09.37** — Activity, history and audit
- **A-09.48** — Restore
- **A-09.50** — Schema migrations
- **A-09.56** — Crash containment
- **A-09.58** — Deleting media

#### 10 Music and books

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-10.24** — A protocol for the next track (prepare, then commit)
- **A-10.30** — One queue, no separate "Next Up" lane, no previous-queue history
- **A-10.41** — Progress is "which file, and where", ordered by intent

#### 11 Live TV and DVR

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-11.3** — Server-side timeshift buffer per session
- **A-11.5** — Guide stored as full-copy generations
- **A-11.11** — Programme identity from start time
- **A-11.16** — Durable tuner allocations with reservations
- **A-11.19** — Scheduling engine
- **A-11.20** — A recording follows the guide rather than a clock time
- **A-11.21** — No retry or resume during a recording
- **A-11.24** — Honest completeness accounting
- **A-11.28** — Retention and disk guards
- **A-11.39** — A new guide generation is treated as a new source by playback
- **A-11.43** — Live session failure handling

#### 12 Personal state and discovery

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-12.2** — An item's own intent always beats a later "mark show watched"
- **A-12.3** — Titles added after a container mark stay unwatched; a title with no "added" time never inherits
- **A-12.14** — One canonical writer per item, with fences, instead of last write wins
- **A-12.15** — Explicit changes are idempotent, revision-checked and have an offline-conflict protocol
- **A-12.19** — Versions share one resume point
- **A-12.63** — Offline progress is reconciled by the server with last-write-wins

#### 13 Social playback and receivers

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-13.3** — Commands take effect "now", not at a scheduled moment
- **A-13.5** — The follower has no protection around a seek
- **A-13.6** — Clock synchronisation
- **A-13.9** — Groups are stored; they survive a restart
- **A-13.13** — Concurrency control on commands
- **A-13.17** — What "ready" means
- **A-13.29** — Two-phase handoff

#### 14 Downloads

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-14.16** — No automatic retry, no "waiting" states

#### 15 Security

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-15.6** — Session model
- **A-15.13** — TV-code sign-in
- **A-15.34** — Trust root and key rotation
- **A-15.39** — Backups
- **A-15.48** — Build and release integrity

#### 16 Web architecture

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-16.13** — Transport: no default deadline, no built-in retry
- **A-16.26** — Scroll position on Back
- **A-16.42** — Error presentation
- **A-16.43** — "Server unreachable" UX
- **A-16.44** — Error boundaries
- **A-16.48** — Multi-tab behaviour
- **A-16.57** — Client-visible concurrency control

#### 17 Web player

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-17.6** — Seeking is always a local seek; the server follows segment requests
- **A-17.7** — Every seek is a tracked command that must be confirmed
- **A-17.8** — The start position is a seek after load, not hls.js's start position
- **A-17.10** — Fragment load timeout of 15 seconds, and an 8-second stall watchdog
- **A-17.11** — Quiet reconnection instead of immediate failure
- **A-17.12** — Engine failures are reported to the server, which remembers them
- **A-17.16** — Subtitle timing offset is stored on the server
- **A-17.19** — Progress reporting has no unload path
- **A-17.20** — Sessions have a lease; a lapsed lease is an error card
- **A-17.21** — The player owns intent; the element only reports facts
- **A-17.34** — Changing quality, version or audio restarts as a new one-item queue
- **A-17.36** — Timeline reports are sparse
- **A-17.38** — Media URLs never carry cookies and never leave their origin

#### 18 Web experience

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-18.22** — Delete
- **A-18.27** — Scheduled work

#### 19 Native architecture

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-19.15** — Hand-written secure storage with "never repair"
- **A-19.18** — Deep links: a native allowlist and a hand-written router
- **A-19.19** — No state restoration after the system kills the app
- **A-19.38** — Errors: two boundaries, catalogue words only, nothing recorded
- **A-19.39** — No log the user can see or send

#### 20 Native players

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-20.3** — A leased, revisioned command-and-fact protocol between JavaScript and the engine
- **A-20.7** — A wrong capability claim is corrected by a remembered failure, not by the viewer
- **A-20.22** — Every seek is exact and verified
- **A-20.36** — Progress is observed twice a second and checkpointed on change

#### 22 Connectivity

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-22.8** — LAN gets a short head start, not a full serial timeout
- **A-22.11** — Fixed per-origin proof deadlines versus phase-specific transport budgets
- **A-22.13** — Platform network changes are hints, not proof of server loss
- **A-22.24** — Durable rotated sessions versus long-lived access tokens
- **A-22.25** — Refresh has a persistent request ID and restart-safe receipt
- **A-22.26** — Refusal is distinct from transient failure
- **A-22.27** — Single-flight is layered across runtime and storage
- **A-22.31** — Mutation recovery belongs to the operation owner
- **A-22.32** — 401 retries apply to all methods once
- **A-22.40** — Online report facts are ordered and bounded
- **A-22.41** — Online reporting and offline downloaded-progress sync are separate
- **A-22.53** — TV activation is a durable authorization operation

#### 23 Adjacent competitors

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-23.20** — Watch Together survives a redesign only if it remains a contract
- **A-23.28** — External events should trigger the narrowest trustworthy scan

#### 24 Community lessons

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **L-24.5** — “Stable” is an upgrade and compatibility promise
- **L-24.7** — Backup portability and downgradeability are separate features
- **L-24.11** — Replacing a file must invalidate more than codec facts
- **L-24.15** — Subtitle choice is a resource and timing operation
- **L-24.18** — Quick sign-in has to include real televisions and weak networks
- **L-24.22** — Social playback needs reconnect identity and an expiry model
- **L-24.25** — Authentication cleanup is an ecosystem migration
- **L-24.32** — Fast releases need narrow blast radius and recovery

#### 25 Engineering

Operation identity, exact revisions, ordering, lifecycle and recovery guarantees.

- **A-25.7** — Narrow callbacks fence cross-domain authority
- **A-25.20** — Panic containment is a process policy
- **A-25.21** — Real database and contract tests are first-class
- **A-25.31** — Source sealing versus reproducible binary builds
- **A-25.32** — Signed server binary manifest is narrower than a signed product
- **A-25.33** — Release signer custody is documented but not enforced locally
- **A-25.39** — Automatic application and rollback are not implemented

### T06 — Contain untrusted data and external work (71 members)

**The bet.** Trust boundaries should sit around dangerous operations, without making ordinary use complicated. Files, URLs, XML, images, subprocesses and discovery hints are treated as untrusted; capabilities and authorization constrain access. Isolation and bounded adapters prevent a bad source from becoming control over the whole host.

**Counter-position.** Jellyfin's extensible host and conventional FFmpeg invocation have broader integration reach but a less confined media process in these audits (A-03.13; A-15.22–29). Plex hides much of the implementation and cannot be ranked on unseen internal confinement. Plezy/mpv and direct-file clients can decode more locally, moving risk and maintenance into the device rather than the server.

**Buys and costs.** Descriptor/bridge inputs, sandboxed tools, pinned outbound DNS, protected image access, path guards and signed identity probes are clear security leads (A-03.13, A-03.45; A-04.44; A-15.25–30; A-22.3–6). Isolation adds process startup, credential fetches and hardware/OS exceptions. Unsupported Windows subtitle extraction and GPU/sandbox mismatches show the practical cost (A-04.48; A-03.11; B-03.4–5).

**Intent and Justin's philosophy.** Deliberate: Playback and Media:35–40; architecture.md:9, 43; documented access boundaries in A-15.1. This is compatible with his security/ease balance: contain risky internals rather than demand stronger passwords or remove HTTP by default. Encryption at rest and lockout belong to separate deliberate choices.

**Inconsistent application.** Several proxy/locality functions apply different trust rules (A-01.42; A-15.17). Vega observes redirects after transmission rather than preventing them (A-22.52). API registry access labels are not always the enforced policy (A-08.16). The release-feed fetch sits outside the normal outbound policy (B-15.17). Existing guard machinery should be made comprehensive before treating its name as proof.

**Decisions for Justin.** Require confinement on every supported server platform, qualify an explicitly reduced boundary on some platforms, or restrict affected capabilities until an adapter exists? Keep authenticated image fetches, or use narrow revocable image credentials to reduce client work? Should all external URL consumers and client-address decisions go through one reviewed policy?

**Member entries**

#### 01 Identity and access

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-01.22** — No list of users before sign-in
- **A-01.42** — "Remote" is decided in two different ways
- **A-01.46** — Credential storage on clients
- **A-01.47** — Clients pin the server's identity key

#### 02 Playback decisions and protocol

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-02.42** — Remote URL sources

#### 03 Transcoding

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-03.13** — Sandbox versus none
- **A-03.45** — Remote sources reach FFmpeg through a private bridge

#### 04 Subtitles and analysis

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-04.39** — Trickplay delivery
- **A-04.44** — Confined decoders for every subtitle and analysis job

#### 05 Inventory and naming

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-05.2** — Every file-system call runs in a child process
- **A-05.27** — Symbolic links
- **A-05.37** — .strm descriptors
- **A-05.43** — Reading local sidecars

#### 08 API and events

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-08.7** — Strict request parsing
- **A-08.8** — No null in responses, tolerant readers
- **A-08.14** — Auth header design
- **A-08.16** — How authorization is expressed
- **A-08.17** — Media URL authority
- **A-08.20** — Sign-in throttling
- **A-08.44** — Security headers on API responses
- **A-08.45** — Unknown routes and methods

#### 10 Music and books

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-10.1** — Tags are read by ffprobe and a whitelist, not by a tag library

#### 11 Live TV and DVR

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-11.13** — Source and stream types
- **A-11.14** — Hostile-input posture

#### 15 Security

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-15.1** — How a route becomes authenticated
- **A-15.2** — Size of the unauthenticated surface
- **A-15.3** — Images need a credential
- **A-15.4** — Media URLs are per-playback capabilities
- **A-15.5** — Credentials in URLs
- **A-15.7** — "Device-bound" means a random id, not a key
- **A-15.11** — Username enumeration
- **A-15.16** — What "local network" is trusted for
- **A-15.17** — Proxy header handling
- **A-15.18** — Security response headers
- **A-15.19** — Cross-origin requests
- **A-15.21** — HTML injection sinks
- **A-15.22** — Running FFmpeg
- **A-15.23** — Building FFmpeg arguments
- **A-15.24** — No owner-to-code-execution features
- **A-15.25** — Server-side request forgery
- **A-15.26** — Image handling
- **A-15.27** — XML and archives
- **A-15.28** — Paths that come from requests
- **A-15.29** — Server-side folder picker
- **A-15.35** — TLS
- **A-15.37** — Server identity pinning
- **A-15.40** — Logging discipline
- **A-15.41** — Rate limiting and overload
- **A-15.42** — Error messages
- **A-15.43** — Hostile-input testing
- **A-15.44** — Runtime profiling endpoint
- **A-15.45** — Native credential storage
- **A-15.46** — Native transport rules
- **A-15.47** — Deep links, WebViews, exported components
- **A-15.50** — Network discovery protocols

#### 16 Web architecture

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-16.11** — Runtime validation of every response versus trusting generated types
- **A-16.23** — Plain-HTTP engineering
- **A-16.27** — Artwork delivery: authorised fetch + object URLs versus plain image URLs
- **A-16.58** — Content-Security-Policy posture and its side effects

#### 19 Native architecture

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-19.16** — All API traffic goes through a bounded native transport
- **A-19.30** — Image authorisation by header

#### 22 Connectivity

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-22.3** — Discovery is an identity hint, never a credential destination by itself
- **A-22.4** — Persistent server identity is key-derived
- **A-22.5** — Manual pairing is trust on first use
- **A-22.6** — Signed account-scoped route documents
- **A-22.7** — Bounded current/previous route evidence
- **A-22.51** — Discovery responder recovery and hard bounds are unusually explicit
- **A-22.52** — Vega cannot enforce the same redirect contract

#### 24 Community lessons

Confinement, access boundaries and safe handling of untrusted inputs.

- **L-24.16** — Observability must not perturb playback

#### 25 Engineering

Confinement, access boundaries and safe handling of untrusted inputs.

- **A-25.30** — Security automation is present but incomplete as a release gate
- **A-25.34** — Container engineering has good defaults, separate release weakness

### T07 — Keep evidence; ask the owner when identity is uncertain (71 members)

**The bet.** Catalogue identity and metadata should be explainable evidence, rather than the latest provider response overwriting yesterday's truth. Portico prefers strong matching signals, explicit owner choices, provenance, locks and undo; it avoids guessing where two works or files might be confused. Local facts should remain meaningful without a provider.

**Counter-position.** Jellyfin's ordered provider chains and broader filename/sidecar handling maximize automatic coverage; Plex's aggregating agents and partial-content matching absorb more messy libraries (A-06.1–2; A-05.7, A-05.15; A-23.7–8). Plezy's meaningful track-choice matching declines ambiguity while remembering useful choices, illustrating that caution need not eliminate convenience (A-04.23). Navidrome treats tags as truth; Audiobookshelf uses richer folder/local evidence for its specialist domain (A-10.3, A-10.39).

**Buys and costs.** Durable evidence, locks/undo, explicit episode ordering, distinct asset/title identity and conservative marker approval are valuable (A-06.5, A-06.15, A-06.20; A-07.2–8; A-04.31; L-24.40). Strict matching can leave many items awaiting intervention, while one provider cannot supply every supplementary field. Narrow parsing, all-or-nothing chapters/guide records and path-sensitive identities can undermine easy adoption (A-06.2, A-06.51; A-05.15, A-05.28; A-11.6, A-11.11).

**Intent and Justin's philosophy.** Mixed: provenance/locks and local evidence are deliberate (decisions P10/P12/R04/R05; architecture.md:73). “One provider” is explicitly recorded in Catalogue and Metadata:66; individual parser limitations are frequently emergent rather than product decisions. This supports freedom and trustworthy polish, but asks more owner labor than Plex-like import.

**Inconsistent application.** Local metadata sometimes wins permanently after the local source disappears (B-06.15). Bulk lyrics selects a first result without the same identity caution (B-10.6). Conservative subtitle import excludes discovered tracks from automatic selection (A-04.1, A-04.20). Stable logical identity is weaker across copies/replacements than within same-disk moves; the documents disagree about rename survival (A-12.4 versus A-05.7).

**Decisions for Justin.** Keep one provider for everything, keep one identity provider with typed supplementary sources, or permit explicit chains? Retain strict no-guess matching but add a usable review inbox, or accept narrow evidence-based automatic fallbacks? Should bad external records be quarantined individually while good records publish? What continuity proof should preserve personal state across a replacement or cross-disk move?

**Member entries**

#### 04 Subtitles and analysis

Identity evidence, provenance, uncertainty and owner-directed reconciliation.

- **A-04.1** — Import-then-play versus "every stream is playable"
- **A-04.3** — Stored form: a JSON cue document versus the original file
- **A-04.5** — Sidecar matching grammar
- **A-04.6** — Character encodings
- **A-04.19** — Subtitle modes and the meaning of "forced"
- **A-04.20** — Auto-selection only sees imported plain-text tracks
- **A-04.23** — No remembered track choice
- **A-04.24** — Track identity
- **A-04.29** — Chapters
- **A-04.31** — Marker model
- **A-04.32** — Detection method
- **A-04.34** — Skip experience

#### 05 Inventory and naming

Identity evidence, provenance, uncertainty and owner-directed reconciliation.

- **A-05.7** — File identity: device + inode + size + time
- **A-05.8** — What a "title" is keyed on
- **A-05.9** — What survives a move, rename, replace or re-download
- **A-05.14** — Movie titles come from the file name, never the folder
- **A-05.15** — A strict episode reader that refuses, versus a permissive one that guesses
- **A-05.16** — Show titles come from the file name, and the folder only checks them
- **A-05.20** — Multi-episode files
- **A-05.21** — Versions and editions
- **A-05.22** — Multi-part files
- **A-05.28** — One bad entry versus the whole scan
- **A-05.31** — Music grouping
- **A-05.32** — Audiobooks
- **A-05.44** — A file is interpreted once

#### 06 Metadata and artwork

Identity evidence, provenance, uncertainty and owner-directed reconciliation.

- **A-06.1** — One provider per library, versus ordered chains with per-field merge
- **A-06.2** — Conservative automatic matching, versus "best result wins"
- **A-06.5** — A durable evidence model, versus overwrite in place
- **A-06.12** — A different year is a hard contradiction
- **A-06.15** — Episode order is an explicit, per-show choice
- **A-06.16** — Absolute-numbered files need TheTVDB or AniList
- **A-06.17** — Specials placement
- **A-06.18** — Language and region
- **A-06.19** — Precedence between local files and the provider
- **A-06.20** — Locks, history and undo
- **A-06.23** — NFO reading is strict and narrow; no NFO writing
- **A-06.25** — Local artwork names recognised
- **A-06.28** — People
- **A-06.38** — One image per role, chosen through a candidate/selection model
- **A-06.44** — Music
- **A-06.49** — Provider responses are validated hard
- **A-06.51** — Finding what did not match
- **A-06.52** — Changing provider re-matches the library
- **A-06.53** — "Local metadata only" is a library-level agent

#### 07 Persistence and queries

Identity evidence, provenance, uncertainty and owner-directed reconciliation.

- **A-07.2** — Integer ids inside, random public ids outside, versus a GUID computed from the path
- **A-07.3** — Files are separate from titles
- **A-07.4** — What happens to identity when files move
- **A-07.5** — Personal state is deleted with the title, versus detached and re-attached
- **A-07.7** — Genres, studios, tags
- **A-07.8** — People

#### 10 Music and books

Identity evidence, provenance, uncertainty and owner-directed reconciliation.

- **A-10.3** — Album identity includes the folder and the year, and albums are never merged
- **A-10.9** — Evidence with provenance, and a three-way local/online policy
- **A-10.34** — Lyrics are curated resources, not discovered files
- **A-10.35** — Strict LRC parsing, line-level only

#### 11 Live TV and DVR

Identity evidence, provenance, uncertainty and owner-directed reconciliation.

- **A-11.6** — All-or-nothing validation of provider data
- **A-11.10** — Channel identity from tvg-id
- **A-11.12** — Series identity

#### 12 Personal state and discovery

Identity evidence, provenance, uncertainty and owner-directed reconciliation.

- **A-12.4** — Personal state follows the catalogue entity; Jellyfin's follows the title's provider identity

#### 18 Web experience

Identity evidence, provenance, uncertainty and owner-directed reconciliation.

- **A-18.18** — The metadata editor
- **A-18.21** — Refresh
- **A-18.47** — How a long-running show is presented

#### 21 Native experience

Identity evidence, provenance, uncertainty and owner-directed reconciliation.

- **A-21.48** — Native metadata repair includes provenance/locks but authentication gates matter

#### 23 Adjacent competitors

Identity evidence, provenance, uncertainty and owner-directed reconciliation.

- **A-23.7** — One chosen provider versus one aggregating agent
- **A-23.8** — Local NFO evidence versus provider-owned descriptions
- **A-23.11** — Voice activity as reusable subtitle alignment evidence

#### 24 Community lessons

Identity evidence, provenance, uncertainty and owner-directed reconciliation.

- **L-24.8** — Inventory should survive metadata failure
- **L-24.9** — A disconnected mount is not an empty collection
- **L-24.10** — File identity, work identity and edition identity are different
- **L-24.14** — Capability evidence must describe the actual route
- **L-24.35** — Local metadata and provider ordering are deliberate product choices
- **L-24.40** — A skip-marker API is not high-quality automatic intro detection

### T08 — Compose once on the server and share client behaviour (63 members)

**The bet.** Product behaviour is composed once and rendered many times. The server decides rows, ordering, filters and playback, while a shared headless client core owns request/state semantics and authored presentation grammar. React web and React Native are adapters to the same product rather than independent products evolving their own rules.

**Counter-position.** Jellyfin's independently developed clients and SDKs permit platform-specific innovation but visibly diverge in feature completion (A-16.1, A-19.1; L-24.12). Plex is the consistency target, though its internal sharing cannot be established here. Plezy's shared Flutter UI also reduces divergence, but composes against several backend semantics and often owns more state in the client (A-21.1–4; L-24.19).

**Buys and costs.** Shared screen grammar, error words, settings structure, show workspace and multi-platform social follower reduce repeated work (A-16.8–9; A-19.8–9, A-19.33; A-21.11, A-21.19, A-21.25). Server composition ties client possibilities to server projections; independent native conventions, offline composition and multi-server aggregation become harder. Copied packages require coordinated versions, not merely equal files (A-25.1–3).

**Intent and Justin's philosophy.** Explicit: Overview:42, 54; Clients:3, 68–70; decisions P13/P21; architecture.md:9, 67. This directly serves his many-platform consistency goal. It does not establish equal codecs, focus, accessibility or device quality.

**Inconsistent application.** Clients still infer Home destinations from row IDs; page/card actions have two capability mappings (A-16.8; A-18.14; A-21.28). Native and web pre-play choices differ (A-18.12; A-21.27). A checksum gate is present despite one document saying none exists, but Hosted apikit actually drifts (A-16.10 versus A-19.9; A-25.3). Native administration is broad even though decisions P18 reserves broad operations to web-first flows.

**Decisions for Justin.** Keep server-authoritative semantics with explicit platform adapters, or allow narrowly declared platform departures? Keep checked copies with pinned source manifests, publish versioned packages, or colocate public clients? Which differences are legitimate platform adaptations and which are parity obligations? Must each backend feature include its first-party consumer before it is described as complete?

**Member entries**

#### 02 Playback decisions and protocol

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-02.1** — One server-made decision versus a menu the client chooses from
- **A-02.25** — One timeline call versus start / progress / stopped / ping
- **A-02.45** — Next episode
- **A-02.46** — Live TV and VOD share the session resource

#### 06 Metadata and artwork

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-06.22** — The editor's field list comes from the server and is narrower

#### 08 API and events

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-08.1** — Screen documents composed by the server, versus one generic item query
- **A-08.38** — Server-supplied strings
- **A-08.47** — Web app delivery

#### 09 Server operations

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-09.35** — Settings and Console are separate ideas; the dashboard is server-composed panels

#### 12 Personal state and discovery

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-12.37** — Home layout is per profile and identical on every device
- **A-12.42** — No merging across servers

#### 13 Social playback and receivers

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-13.1** — Who owns the group clock
- **A-13.4** — How drift is corrected
- **A-13.19** — Client coverage
- **A-13.33** — The TV owns the playback session

#### 16 Web architecture

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-16.1** — One app versus three apps in a long migration
- **A-16.6** — Data layer: hand-rolled services versus a query cache
- **A-16.8** — Server-authored documents versus client-composed screens
- **A-16.9** — Authored presentation structure in the shared core
- **A-16.10** — Shared core distributed by copying a folder
- **A-16.15** — Session and token handling in the browser
- **A-16.16** — One giant session context
- **A-16.50** — Where preferences live
- **A-16.56** — One codebase, two builds (bundled and Hosted-only)

#### 18 Web experience

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-18.1** — Who decides what a screen contains
- **A-18.2** — One Settings screen against a separate admin console
- **A-18.7** — Filters built from the server's vocabulary against a fixed panel
- **A-18.11** — One hero for every type; an episode is a panel
- **A-18.14** — Two action models for one title
- **A-18.51** — "This device" settings are per kind of device

#### 19 Native architecture

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-19.1** — The cross-platform bet: React Native with native modules
- **A-19.2** — One Android binary for phones and televisions
- **A-19.8** — Vega as a shim over the same screens
- **A-19.9** — Shared packages by copy and checksum
- **A-19.10** — Shared code is imported by deep relative path
- **A-19.11** — State management: services with snapshots, not a state library
- **A-19.17** — Navigation: seven kept-alive stacks under a custom navigator
- **A-19.25** — A television is laid out in a 1920×1080 design space
- **A-19.33** — A design system that is enforced, and shared with the web
- **A-19.42** — Platform coverage

#### 20 Native players

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-20.2** — The player screen is React Native; competitors with heavy engines moved it native
- **A-20.29** — The engine never plays "by itself": every state change is a JavaScript intent
- **A-20.34** — Post-play and skip are server policy

#### 21 Native experience

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-21.1** — Server-composed media surfaces versus client-composed hubs
- **A-21.2** — One shared screen implementation across six native targets
- **A-21.3** — Vega emulates Apple React Native semantics
- **A-21.9** — Native owner console is unusually broad
- **A-21.11** — Authored settings layout rather than registry-generated UI
- **A-21.12** — Server device-class settings are labelled as this device
- **A-21.13** — Installation-local and server preference models coexist
- **A-21.14** — Home saved query rows are server-backed resources
- **A-21.19** — Library peers are shared per media kind
- **A-21.25** — One canonical show workspace instead of many detached child screens
- **A-21.28** — Context and detail actions share IDs but differ in actual capabilities

#### 24 Community lessons

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **L-24.12** — Implement the server feature and the client contract together
- **L-24.20** — Multi-server identity is not a unified catalogue
- **L-24.39** — Platform access can block a feature despite votes and shared code

#### 25 Engineering

Server-owned composition, shared behaviour and the cost of keeping adapters aligned.

- **A-25.1** — Split product repositories, colocated server and web
- **A-25.2** — Copied shared packages instead of published versions
- **A-25.3** — Shared Go toolkit is vendored by copying
- **A-25.26** — Client tests are mostly headless behavior harnesses
- **A-25.27** — Native CI is Android-centric, not absent
- **A-25.28** — Roku relies on generated parity and source-driven checks

### T09 — Use platform video players and normalize media on the server (106 members)

**The bet.** Video uses the operating system's player, with the server supplying a stream that player can handle. This keeps the normal client lighter and integrates with OS audio, display and lifecycle behaviour. Portico makes a deliberate exception for listening: custom server facts and client audio rendering chase Plexamp-like continuity where ordinary players are insufficient.

**Counter-position.** Jellyfin has both native-system players and mpv desktop; Swiftfin and Plezy bring VLC/mpv/libass-style breadth onto the device (A-20.1; L-24.13). Infuse puts much more format/HDR processing in the client (A-23.16–18). Plex is the polish and codec experience target, while Plexamp is explicitly the listening standard. These options reduce server conversion at the cost of client binary, engine and qualification ownership.

**Buys and costs.** Smaller clients, platform PiP and system media integration are useful (A-20.31–32, A-20.41). Browser/Apple MKV remux, lossy surround conversion, SDR-only video conversion and subtitle burn-in cost CPU, fidelity and start latency (A-02.9; A-04.9–13; A-20.9, A-20.19). Custom audio buys gapless/crossfade but becomes multiple DSP/trim implementations to qualify (A-10.15–22; A-17.23–25).

**Intent and Justin's philosophy.** Native players are explicit in decisions P21 and Playback and Media:3, 26–30. Fixed H.264/SDR conversion recipes are partly implementation simplification, not a demonstrated permanent product decision. This supports ease, footprint and consistency; it can conflict with broad platform feature richness when server fallback reduces the requested experience.

**Inconsistent application.** Rich capability probes are overwritten by a lossy translated document (A-02.4; A-20.5–6). Android advertises subtitle rendering it does not provide (B-20.11); live and prepared paths ignore much of the normal planner's flexibility (A-03.27, A-03.40; A-11.4). Frame matching, trim and effect behaviour differ by platform. Download options can offer a format the offline OS player cannot decode (A-14.4).

**Decisions for Justin.** Retain system players plus targeted subtitle/codec improvements; add an optional alternative engine at the existing command/fact seam; or make a broad engine the default? Invest first in HEVC/HDR/multichannel server outputs or in local rendering? What format/device corpus establishes “matches Plex” separately from a file merely starting?

**Member entries**

#### 02 Playback decisions and protocol

The division between client engine capability and server compatibility work.

- **A-02.2** — Profile stored per device versus sent with every request
- **A-02.3** — Codec tables plus transports versus a condition language
- **A-02.4** — Two capability documents with a lossy translation between them
- **A-02.5** — Evidence tags on every capability
- **A-02.6** — A conservative baseline for unknown clients
- **A-02.7** — Tolerant of unknown names, strict about version
- **A-02.9** — Conversion output is fixed; Jellyfin's is negotiated
- **A-02.10** — MPEG-TS is tried before fMP4
- **A-02.14** — Jellyfin re-selects an audio track to keep direct play; Portico does not
- **A-02.15** — Dolby Vision and HDR modelling
- **A-02.16** — Passthrough is route-aware
- **A-02.17** — Audio-track switching inside an original file is a declared capability
- **A-02.22** — A client-declared transport class
- **A-02.38** — Default audio track
- **A-02.39** — Default subtitles
- **A-02.40** — Subtitle delivery is decided by the track, not the device
- **A-02.41** — Which version plays by default
- **A-02.43** — Disc folders, ISO, 3D
- **A-02.48** — Stream facts are refreshed at first play

#### 03 Transcoding

The division between client engine capability and server compatibility work.

- **A-03.6** — Converted output is always H.264 8-bit SDR in MPEG-TS
- **A-03.7** — Bitrate policy: fixed average bitrate versus quality target with a cap
- **A-03.8** — No profile, level, GOP or scene-cut control on the on-demand encoder
- **A-03.9** — Forcing keyframes on hardware encoders
- **A-03.10** — Hardware selection: probe everything automatically versus owner picks
- **A-03.11** — GPU decode and GPU filters are all-or-nothing behind one probe
- **A-03.14** — Tone-mapping choices
- **A-03.15** — Dolby Vision
- **A-03.16** — HDR detection
- **A-03.23** — Alternate audio as real HLS renditions
- **A-03.27** — Live TV conversion is software-only and converts more than necessary
- **A-03.40** — Prepared versions: two fixed recipes
- **A-03.48** — Dependence on host GPU drivers
- **A-03.49** — Platform coverage

#### 04 Subtitles and analysis

The division between client engine capability and server compatibility work.

- **A-04.8** — Format coverage
- **A-04.9** — Styled ASS: flatten or burn, never render on the client
- **A-04.10** — Image subtitles: burn-in only
- **A-04.11** — Burn-in needs a pre-extracted copy of the track
- **A-04.13** — Fonts for burn-in
- **A-04.14** — Who decides to burn
- **A-04.16** — HLS subtitle renditions
- **A-04.25** — Switching audio
- **A-04.45** — Live TV captions
- **A-04.48** — Windows servers

#### 10 Music and books

The division between client engine capability and server compatibility work.

- **A-10.15** — Gapless by server-measured facts and on-device decoding
- **A-10.16** — Three engines with three different sources of truth for trim
- **A-10.17** — Every audio file is fully decoded once to measure it
- **A-10.18** — Conversion targets are FLAC and Opus only, made fresh for every request
- **A-10.19** — The audio capability model has no codec profile
- **A-10.20** — Loudness: tags, then server analysis, with peaks; off by default
- **A-10.21** — Normalisation can only turn tracks down on Android
- **A-10.22** — Crossfade
- **A-10.25** — Seeking inside large files

#### 11 Live TV and DVR

The division between client engine capability and server compatibility work.

- **A-11.2** — Always re-segmented HLS, never direct
- **A-11.4** — Narrow copy rule on live: H.264 with stereo AAC only

#### 14 Downloads

The division between client engine capability and server compatibility work.

- **A-14.4** — Quality is a fixed server ladder; the device's abilities are not consulted
- **A-14.5** — One conversion recipe family: H.264 SDR, stereo AAC, 30 fps, no subtitles, no chapters
- **A-14.6** — Software-only, two-thread encodes, one at a time
- **A-14.24** — Two different players for the same file
- **A-14.41** — Items made of several files, or with several versions
- **A-14.42** — Music and audiobooks

#### 17 Web player

The division between client engine capability and server compatibility work.

- **A-17.2** — hls.js on every browser that supports it, including Safari
- **A-17.3** — Capability detection by asking the browser, not by naming it
- **A-17.4** — Matroska is never direct-played in a browser
- **A-17.5** — HDR is sent untouched only to an HDR screen or a browser known to tone-map
- **A-17.9** — Buffer targets are fixed by time, not by bitrate
- **A-17.13** — Subtitles are one server-normalised cue document
- **A-17.14** — Image subtitles force a video conversion
- **A-17.17** — Audio track choice means asking for the title again
- **A-17.23** — Music decoded by Portico's own engine, not the media element
- **A-17.24** — What happens when WebCodecs is missing
- **A-17.25** — All audio decode, resample and stretch work runs on the main thread
- **A-17.40** — Fullscreen targets the player container only
- **A-17.43** — Casting from the browser

#### 19 Native architecture

The division between client engine capability and server compatibility work.

- **A-19.20** — The focus engine is the platform's (except on Vega)
- **A-19.23** — Siri Remote touch surface
- **A-19.32** — Animation engines
- **A-19.51** — Home-screen integration is written natively per platform
- **A-19.53** — The desktop question

#### 20 Native players

The division between client engine capability and server compatibility work.

- **A-20.1** — Platform players fed by the server, versus an engine the app brings with it
- **A-20.4** — Capabilities are a stored device document, not part of each play request
- **A-20.5** — Two capability documents, and the poorer one wins
- **A-20.6** — A declared table plus a probe, with evidence recorded
- **A-20.9** — Lossless and DTS audio become 384 kbps AAC
- **A-20.10** — Passthrough is detected from the HDMI device's encodings, not from the audio sink's own answer
- **A-20.11** — On Apple the system owns the audio output; Portico only declares
- **A-20.12** — Downmixing is the server's job
- **A-20.13** — HDR on an SDR screen: device tone mapping is intended but not delivered
- **A-20.14** — Dolby Vision profile 7 falls back to its base layer by copy; others convert it on the device
- **A-20.15** — No known-defect device list
- **A-20.16** — Frame-rate and dynamic-range matching: tvOS only, automatic, and late
- **A-20.19** — Image and styled subtitles force a video transcode
- **A-20.20** — Audio track choice on a direct-played file restarts the session
- **A-20.28** — Buffering is the engine default with a low-memory cap
- **A-20.30** — Leaving the app: automatic Picture in Picture on phones, nothing on Android TV
- **A-20.31** — System media integration is hand-built rather than inherited
- **A-20.32** — Picture in Picture uses the player layer
- **A-20.37** — Live TV positions are wall-clock dates
- **A-20.38** — Downloads play in the same controller
- **A-20.39** — Vega plays HLS through a loader the port wrote
- **A-20.40** — Licences and distribution
- **A-20.41** — Binary size, battery and maintenance

#### 21 Native experience

The division between client engine capability and server compatibility work.

- **A-21.35** — App voice intentions are distinct from dictation
- **A-21.52** — External players are not a first-party UX option

#### 23 Adjacent competitors

The division between client engine capability and server compatibility work.

- **A-23.16** — Broad client decoding versus server-side fallback
- **A-23.18** — Lossless decode versus transparent final signal path

#### 24 Community lessons

The division between client engine capability and server compatibility work.

- **L-24.13** — One UI framework does not make one playback engine

### T10 — An authored, consistent interface with restrained customization (88 members)

**The bet.** Portico has an authored product character: stable navigation, semantic card geometry, a fixed player grammar and restrained appearance choices. Shared design and text primitives keep that character coherent across screens and input types. Defaults should be understandable without turning every page into an owner's layout project.

**Counter-position.** Jellyfin offers more configurable Home slots, themes, CSS, branding and client-specific controls (A-18.5, A-18.32, A-18.35). Plex is the closer polish reference, but has more mature interaction details and broader language/platform coverage. Plezy offers more layout, theme and input personalization; native specialist clients often integrate accessibility and OS conventions more deeply (A-21.16–22, A-21.34–43).

**Buys and costs.** Shared heroes, a canonical show workspace, meaningful geometry, disciplined tokens and coherent empty states reduce cognitive load (A-18.11, A-18.37, A-18.45; A-19.33; A-21.17, A-21.25). Fixed density, dark-only appearance and English launch omit meaningful accessibility or family preferences, not just cosmetic variety (A-16.33–40, A-16.45–46; L-24.28). TV versus handheld input still needs individual acceptance.

**Intent and Justin's philosophy.** Deliberate core: decisions P13/P14; Clients:71, 74–77; visual/contract.md:24 requires adaptation without changing product character. English launch and individual missing affordances should not be confused with a permanent refusal to localize. The restraint suits his slight ease preference; consistent polish requires essential actions to remain reachable.

**Inconsistent application.** Account/Help/feedback disappear at narrow web widths (B-18.14). Regional formatting and labels bypass the shared catalogue in many screens (A-18.38; B-16.6; B-18.12, B-18.24). Native subtitle appearance ignores product and system preferences (A-20.21). Kept-alive root stacks intentionally preserve history but contradict a newer navigation law (A-21.41); this needs a choice, not automatic cleanup.

**Decisions for Justin.** Keep one reviewed appearance, add accessible light/high-contrast variants, or allow bounded personal styling? Which input repairs are essential parity versus optional gestures? Retain root history or adopt reset-on-root navigation? Should English-only launch require pseudolocale, large text, screen-reader and keyboard/focus acceptance before broader translation?

**Member entries**

#### 04 Subtitles and analysis

The authored interaction grammar, consistency and limits on personalization.

- **A-04.40** — Trickplay on clients

#### 05 Inventory and naming

The authored interaction grammar, consistency and limits on personalization.

- **A-05.19** — Season folders are English only

#### 06 Metadata and artwork

The authored interaction grammar, consistency and limits on personalization.

- **A-06.33** — Originals are discarded; one normalised master is kept
- **A-06.34** — Three fixed sizes, versus any size on demand
- **A-06.37** — Placeholders

#### 09 Server operations

The authored interaction grammar, consistency and limits on personalization.

- **A-09.18** — What a browser sees while the server is starting

#### 16 Web architecture

The authored interaction grammar, consistency and limits on personalization.

- **A-16.4** — Bespoke design system on Radix versus a component kit
- **A-16.5** — CSS approach
- **A-16.17** — Routing: real URLs with validated params versus a hash router
- **A-16.19** — Code splitting and preloading
- **A-16.22** — Browser support: undeclared modern-only versus declared ancient
- **A-16.29** — Placeholders while images load
- **A-16.31** — Input modes: pointer, touch and keyboard only
- **A-16.32** — Is the web app a base for TV and desktop wrappers?
- **A-16.33** — Theming and personalisation
- **A-16.34** — Fonts
- **A-16.35** — Message format
- **A-16.36** — Catalogue loading
- **A-16.37** — Languages and translation workflow
- **A-16.38** — How code reaches the catalogue
- **A-16.39** — Locale-aware formatting
- **A-16.40** — Right-to-left
- **A-16.45** — Accessibility engineering
- **A-16.46** — Page titles and route announcements
- **A-16.60** — URL design details

#### 17 Web player

The authored interaction grammar, consistency and limits on personalization.

- **A-17.22** — Media Session only for audio
- **A-17.28** — Up next appears only after the title has fully ended
- **A-17.32** — Skip prompts are server-authorised and reported

#### 18 Web experience

The authored interaction grammar, consistency and limits on personalization.

- **A-18.3** — Navigation: a permanent rail of libraries against an app bar and drawer
- **A-18.4** — Search: a destination with chips against a field with sections
- **A-18.5** — Home: server-composed rows with an order dialog against user-chosen slots
- **A-18.8** — View modes
- **A-18.9** — Where sort and filter state lives
- **A-18.10** — Library tabs
- **A-18.12** — Pick before play
- **A-18.13** — Technical details: a Files section against a Media Info dialog
- **A-18.19** — Fixing a wrong match
- **A-18.20** — Artwork
- **A-18.32** — Themes, custom CSS and branding
- **A-18.33** — First run
- **A-18.35** — Keyboard and input
- **A-18.36** — Layout modes
- **A-18.37** — Visual language
- **A-18.38** — Languages, and how strings reach the screen
- **A-18.40** — Addresses and links
- **A-18.41** — The browser tab's title
- **A-18.43** — Where Watch Together starts
- **A-18.45** — Loading, empty and error states
- **A-18.46** — Accessibility affordances
- **A-18.50** — Back
- **A-18.52** — No mark for "not watched yet"

#### 19 Native architecture

The authored interaction grammar, consistency and limits on personalization.

- **A-19.3** — How a device is judged to be a television
- **A-19.21** — Focus memory by guides and a one-shot claim
- **A-19.22** — Menu and Back policy
- **A-19.24** — Keyboards, game controllers and pointers
- **A-19.34** — One dark theme
- **A-19.35** — Fonts and icons are bundled
- **A-19.36** — Accessibility lives in the primitives
- **A-19.37** — Internationalisation: own formatter, English only
- **A-19.47** — Minimum OS versions
- **A-19.54** — Android edge-to-edge and the keyboard
- **A-19.55** — Orientation

#### 20 Native players

The authored interaction grammar, consistency and limits on personalization.

- **A-20.17** — One server-normalised subtitle document drawn by the app
- **A-20.21** — Subtitle look is fixed
- **A-20.23** — The timeline has no pictures and no marks
- **A-20.25** — Handheld player has no gestures
- **A-20.26** — No zoom or aspect modes

#### 21 Native experience

The authored interaction grammar, consistency and limits on personalization.

- **A-21.6** — TV setup is a first-class code/QR approval workflow
- **A-21.15** — My List is a destination rather than a Home shelf
- **A-21.16** — Fixed theatre theme versus extensive appearance personalization
- **A-21.17** — Artwork geometry is semantic rather than freely chosen
- **A-21.18** — Watched indicators are a fixed product rule
- **A-21.21** — TV filtering avoids text entry and separates field/value levels
- **A-21.22** — Native view modes lag the web and competition
- **A-21.26** — No native episode spoiler policy established
- **A-21.27** — Pre-play choices omit version/quality and collapse subtitle identity
- **A-21.34** — Native platform text entry is an explicit product rule
- **A-21.36** — Search scope and suggestions differ
- **A-21.37** — Handheld input focuses on visible controls, not gesture customization
- **A-21.38** — Tablet layout adapts width but is not a full tablet workspace
- **A-21.39** — TV focus relies on native geometry plus semantic repair guides
- **A-21.40** — Root Back preserves in-app hierarchy and delegates OS exit
- **A-21.41** — Kept-alive root stacks preserve context but contradict current root law
- **A-21.42** — Audio continuation uses a tiny mini bar or TV shell pill
- **A-21.43** — Fixed player-control grammar with utilities separate from settings
- **A-21.50** — Language design prioritizes English regional completeness

#### 22 Connectivity

The authored interaction grammar, consistency and limits on personalization.

- **A-22.50** — Connection diagnostics remain less specific than failure evidence allows

#### 24 Community lessons

The authored interaction grammar, consistency and limits on personalization.

- **L-24.28** — Accessibility choices can be playback requirements

### T11 — One broad media product, with depth bought selectively (77 members)

**The bet.** One server and one consumer app should handle several household media families and adjacent experiences. Shared catalogue, playback, guide, saved-state and library-channel abstractions provide breadth without deploying a separate application for every use. Portico buys specialist depth selectively rather than adopting every tool's feature model.

**Counter-position.** Jellyfin supports more library/format kinds and plugins but also has uneven specialist UX. Plex combines broad media UX with specialist Plexamp and commercial discovery; Channels focuses deeply on TV/DVR (A-23.23, A-23.32–34). Navidrome/Finamp focus on music, Audiobookshelf on books/podcasts, and Plezy concentrates on a rich client for existing servers. Their narrower workflows can be more complete without being a better whole-product architecture.

**Buys and costs.** First-class anime, books, Live/DVR, Xtream and Library Channels already exist; this is substantial scope (A-05.12, A-05.32; A-11.41, A-11.44; A-21.47). Uniform types and thresholds can miss album roles, untagged book structure, recording episode filing, in-progress viewing, rolling offline rules and car integrations (A-10.14, A-10.37, A-10.40, A-10.45; A-11.23–30; A-14.40–42). Adding an entity kind alone does not supply its lifecycle.

**Intent and Justin's philosophy.** Five families and retained adjacent domains are explicit in decisions P08/P15/P16/P17/P21. The shape is deliberate; individual omissions are often unfinished or unchosen rather than principled exclusions. His strong, consistent feature-set goal argues for complete named journeys, not competitor checklist breadth.

**Inconsistent application.** Live, VOD, downloads and library channels nominally share abstractions but receive different tracks, captions, conversion rules and progress semantics (A-04.45–46; A-11.4, A-11.45; A-14.41–42). Recorded programmes become movies even where guide evidence can label them more usefully (A-11.23). Music and audiobooks share percentage completion despite different listening needs (A-10.37; A-12.10).

**Decisions for Justin.** Which complete journeys are release promises: film/TV first, the five families, or those plus DVR/offline/social? Build missing specialist depth, integrate a specialist through a documented protocol, or explicitly defer it? Should Library Channels use ordinary VOD delivery at schedule offsets, shared broadcast delivery, or a deliberately limited separate mode?

**Member entries**

#### 05 Inventory and naming

Shared media abstractions and the specialist depth they currently leave out.

- **A-05.12** — Library types
- **A-05.17** — Absolute numbering depends on the library kind
- **A-05.25** — Which files count as media
- **A-05.26** — Disc folders and images
- **A-05.49** — Files of the wrong kind inside a library

#### 06 Metadata and artwork

Shared media abstractions and the specialist depth they currently leave out.

- **A-06.27** — Embedded information in video files
- **A-06.29** — Collections
- **A-06.30** — Genres, tags, studios, networks
- **A-06.31** — Ratings
- **A-06.32** — Trailers, theme music, extras
- **A-06.41** — Artwork sources
- **A-06.43** — Portraits belong to titles, not people
- **A-06.45** — Books and audiobooks

#### 07 Persistence and queries

Shared media abstractions and the specialist depth they currently leave out.

- **A-07.10** — Missing and unaired episodes
- **A-07.11** — Collections belong to one library

#### 10 Music and books

Shared media abstractions and the specialist depth they currently leave out.

- **A-10.2** — Nine audio extensions are scanned
- **A-10.4** — Multi-disc folders are recognised by one strict pattern
- **A-10.5** — An artist is a name; there are no sort names and no MusicBrainz artist identity at scan
- **A-10.6** — Multi-artist tracks are not split
- **A-10.7** — Album artist fallback and compilations
- **A-10.10** — Sidecar formats
- **A-10.11** — Cover art discovery and storage
- **A-10.12** — Online matching is evidence-scored and conservative
- **A-10.13** — Artist biographies and images come from Wikidata, Wikipedia and Commons only
- **A-10.14** — No roles beyond artist, and nothing for classical music
- **A-10.26** — Hi-res, bit-perfect, exclusive output, DSD
- **A-10.32** — Instant mix exists on the server but is not offered
- **A-10.33** — Playlists
- **A-10.37** — "Played" is one percentage for everything that is not video
- **A-10.39** — A book is identified by tags, with the folder as a tie-breaker
- **A-10.40** — Audiobook metadata is local only
- **A-10.42** — Chapters come only from the file, and validation is all-or-nothing
- **A-10.43** — Sleep timer and automatic stop
- **A-10.44** — No podcasts, ebooks or comics
- **A-10.45** — Lock screen yes; car, watch, widgets and speakers no
- **A-10.46** — No third-party client ecosystem
- **A-10.47** — Offline music

#### 11 Live TV and DVR

Shared media abstractions and the specialist depth they currently leave out.

- **A-11.8** — Guide sources
- **A-11.9** — Channel-to-guide mapping
- **A-11.15** — Fixed user agent, no per-source request options
- **A-11.23** — Recordings appear as "movies" with only the programme title
- **A-11.25** — Recording format and post-processing
- **A-11.26** — No watching a recording in progress
- **A-11.27** — Cancelling an in-progress recording discards it
- **A-11.29** — Series rule options
- **A-11.30** — No recording without a programme
- **A-11.33** — Guide filters and colour coding
- **A-11.34** — Categories kept as raw strings
- **A-11.36** — Images
- **A-11.41** — Xtream as a first-class source
- **A-11.42** — HDHomeRun support depth
- **A-11.44** — Library Channels: schedule model

#### 12 Personal state and discovery

Shared media abstractions and the specialist depth they currently leave out.

- **A-12.10** — Audiobook rules

#### 13 Social playback and receivers

Shared media abstractions and the specialist depth they currently leave out.

- **A-13.15** — The shared queue stops at "a list"
- **A-13.16** — The server does not know how long the title is
- **A-13.25** — Portico moves playback; the others also drive another device
- **A-13.30** — What travels with a handoff
- **A-13.36** — Cast feature breadth
- **A-13.37** — When the phone goes away
- **A-13.38** — A Cast TV is not a session other devices can drive
- **A-13.40** — The owner's view of who is watching

#### 14 Downloads

Shared media abstractions and the specialist depth they currently leave out.

- **A-14.40** — Platforms covered

#### 18 Web experience

Shared media abstractions and the specialist depth they currently leave out.

- **A-18.44** — Media types

#### 21 Native experience

Shared media abstractions and the specialist depth they currently leave out.

- **A-21.46** — Discovery is local/server media rather than an external ecosystem hub
- **A-21.47** — Native Live/DVR sits beside first-party library channels

#### 23 Adjacent competitors

Shared media abstractions and the specialist depth they currently leave out.

- **A-23.5** — Serverless playback is a different product, not another mount
- **A-23.9** — Intro detection: episode similarity versus scene hypotheses
- **A-23.10** — Credits detection and post-credit preservation
- **A-23.19** — Downloads: selection versus rolling policy
- **A-23.23** — Server-local discovery versus the whole entertainment market
- **A-23.26** — Static collections, saved queries and external curation
- **A-23.30** — Requests model desire for media that does not exist yet
- **A-23.32** — Richer media needs richer progress and authority
- **A-23.33** — Live input types versus TV Everywhere provider login
- **A-23.34** — Library Channels versus a station automation platform

#### 24 Community lessons

Shared media abstractions and the specialist depth they currently leave out.

- **L-24.27** — Music users notice grouping and continuity more than generic feature count
- **L-24.36** — Successful narrow peers exploit workflows and standards

### T12 — Keep ownership bounded, even at the cost of ecosystem reach (100 members)

**The bet.** A small owned implementation and a controlled first-party product are preferred to a large dependency, plugin and contributor ecosystem. Handwritten services, explicit wiring, narrow forks and product-specific checks make key behaviour understandable to one maintainer. Open-source use and forking remain separate from accepting outside code or supporting every integration.

**Counter-position.** Jellyfin distributes work among plugins, public SDKs, third-party clients and contributors; its long tail is broad but comes with migration and compatibility obligations (A-08.31, A-08.42; L-24.24–25). Plex controls its product more tightly but has a mature API/automation ecosystem around it. Plezy relies on mature engines/libraries and multiple backend adapters; Navidrome's OpenSubsonic support buys many clients without owning them (A-10.46; A-23.27).

**Buys and costs.** Few dependencies, explicit FFmpeg/hls.js provenance, exact ownership and invariant tests are useful (A-16.2; A-19.7; A-25.14–19). The owner inherits parser, transport, i18n, test-harness, release, store and support work. No working integration credential, no standard music facade, limited import/export and no translation intake reduce practical user choice (A-08.15; A-10.46; A-12.61–62; A-25.48–49).

**Intent and Justin's philosophy.** Contribution policy is explicit in apps/portico-server/CONTRIBUTING.md:17; Overview:21–22 gives canonical ownership. No plugins and small dependencies are partly explicit, partly inferred from source shape. Crucially A-25.47 finds explicit *public independent-client intent*: “no ecosystem” must not be treated as Justin's settled philosophy. Runtime user freedom and source freedom can coexist with centralized authorship, but integration freedom needs an actual usable boundary.

**Inconsistent application.** Disabled API keys coexist with screens/docs/models suggesting working keys (A-01.33; A-08.15; A-09.45). Generated contracts coexist with handwritten parsers and multiple route/error dialects (A-08.3, A-08.9; A-16.12). Custom tooling omits some ordinary checks and actual device/render coverage (A-25.23–28, A-25.30). Fork ownership is explicit in some components but cross-repo dependency identity is not pinned consistently.

**Decisions for Justin.** Keep in-process plugins absent but support scoped external API/webhooks; support selected standard facades; or intentionally support first-party access only, revising the independent-client promise? Which commodity implementations should stay owned and which should be delegated to mature libraries? Keep sole-owner code intake while accepting reviewed translations/data/docs, or broaden contributions? What ongoing compatibility, store and support commitment is affordable?

**Member entries**

#### 01 Identity and access

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-01.33** — API keys: scoped and hashed in Portico but not usable; unrestricted and plain in Jellyfin
- **A-01.34** — No path for third-party clients
- **A-01.35** — Single sign-on: Apple and Google through Hosted only
- **A-01.50** — Security notices inside the app, no email

#### 03 Transcoding

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-03.12** — Stock FFmpeg versus a patched fork with bundled drivers

#### 04 Subtitles and analysis

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-04.7** — Own parsers versus a subtitle library
- **A-04.27** — Online subtitles

#### 06 Metadata and artwork

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-06.9** — Shipped provider credentials
- **A-06.47** — Attribution is a product requirement

#### 07 Persistence and queries

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-07.48** — Public id format and third-party ecosystem

#### 08 API and events

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-08.2** — Size and granularity of the surface
- **A-08.3** — Where the contract comes from
- **A-08.4** — Is the contract served by the running server?
- **A-08.5** — Versioning scheme
- **A-08.6** — Compatibility and deprecation policy
- **A-08.15** — Credential lifetime and third-party credentials
- **A-08.18** — CORS posture
- **A-08.29** — Remote control of another device
- **A-08.31** — Extensibility by code
- **A-08.32** — Notifications
- **A-08.35** — Reverse proxies and forwarded headers
- **A-08.42** — Ecosystem protocols
- **A-08.43** — What a multi-backend client needs from a server (lessons from Plezy)

#### 09 Server operations

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-09.41** — Alerts and notifications
- **A-09.42** — Metrics for external monitoring
- **A-09.44** — Plugins and branding
- **A-09.45** — API keys

#### 11 Live TV and DVR

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-11.46** — Library Channels are closed to other players

#### 12 Personal state and discovery

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-12.61** — No third-party watch-state sync
- **A-12.62** — No import or export of watch state

#### 13 Social playback and receivers

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-13.39** — AirPlay and DLNA

#### 14 Downloads

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-14.38** — No notifications; polling instead of push

#### 15 Security

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-15.32** — API keys
- **A-15.49** — Credentials shipped in the binary

#### 16 Web architecture

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-16.2** — Dependency surface: 9 packages versus 60
- **A-16.3** — Bleeding-edge toolchain versus conservative one
- **A-16.12** — Two parallel type systems: generated contracts nobody imports
- **A-16.51** — Static analysis
- **A-16.52** — Tests
- **A-16.53** — Development aids
- **A-16.54** — No StrictMode
- **A-16.55** — Extension points

#### 17 Web player

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-17.1** — A private build of hls.js, against the stock package
- **A-17.35** — Two web adapters, and tests on the one that is not shipped

#### 18 Web experience

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-18.28** — Logs and diagnostics
- **A-18.29** — No plugins
- **A-18.34** — Notifications
- **A-18.49** — Feedback is built in

#### 19 Native architecture

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-19.4** — Dependence on the react-native-tvos fork, and how current it is
- **A-19.5** — New Architecture is on, but Portico's own modules are old-style
- **A-19.6** — The native contract is kept in step by hand
- **A-19.7** — Dependency count
- **A-19.43** — Build and release pipeline
- **A-19.44** — Android packaging
- **A-19.45** — Testing: logic under Node, nothing rendered
- **A-19.46** — How television behaviour is proven
- **A-19.48** — No over-the-air JavaScript updates
- **A-19.49** — Polyfills are few and explicit
- **A-19.52** — Apple project hygiene

#### 21 Native experience

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-21.45** — No push alerts is a deliberate product boundary
- **A-21.49** — Owner server diagnostics are rich; ordinary client diagnostics are thin
- **A-21.54** — Donation/about/licence experience differs

#### 22 Connectivity

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-22.49** — Compatibility is document/feature validation, not a system version gate

#### 23 Adjacent competitors

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-23.27** — An API ecosystem is more than an OpenAPI file
- **A-23.31** — Plugins/skins versus an authored first-party contract
- **A-23.35** — Migration is a boundary between products, not a database copy
- **A-23.36** — Platform breadth, ownership and operational promise

#### 24 Community lessons

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **L-24.19** — A shared backend abstraction needs explicit semantic differences
- **L-24.24** — Optional integrations can become update liabilities
- **L-24.30** — Client availability is partly controlled by stores and maintainers
- **L-24.31** — A contribution model must budget the review queue
- **L-24.34** — Completed tracker status is not the whole current truth
- **L-24.38** — Maintainers decline some popular requests deliberately

#### 25 Engineering

Maintainer ownership, external boundaries, dependencies and compatibility commitments.

- **A-25.4** — Broad package DAG instead of formal layers
- **A-25.5** — HTTP composition and catalog are god-package candidates
- **A-25.6** — Explicit wiring instead of framework DI
- **A-25.14** — A local SQLite fork is now a critical maintained dependency
- **A-25.15** — Minimal Go module inventory, substantial owned behavior
- **A-25.16** — Web/native dependency policy is less uniform
- **A-25.17** — Manually approved FFmpeg is a separate component release
- **A-25.18** — hls.js patch provenance is explicit
- **A-25.23** — Media tests are tiered away from default CI
- **A-25.24** — Race detector is release-only in the principal public server workflow
- **A-25.25** — Fuzz seeds exist, continuous fuzzing does not
- **A-25.40** — Stable/beta labels exist before channel semantics are settled
- **A-25.41** — SemVer validation does not define compatibility support
- **A-25.42** — Compatibility checks validate current spec, not prior releases
- **A-25.45** — Release notes are generated, migration guidance is manual
- **A-25.46** — Public docs exist but architecture currency is weak
- **A-25.47** — Public third-party API intent is unusually explicit
- **A-25.48** — Internationalization plumbing precedes community workflow
- **A-25.49** — GPL licensing and contribution governance are different freedoms
- **A-25.50** — Apple distribution needs a distinct license decision
- **A-25.52** — Third-party notices describe inventories rather than ship all notices
- **A-25.53** — Private Hosted services coexist with GPL public clients
- **A-25.54** — Trademark rules preserve official release identity
- **A-25.55** — Sole-owner AI-assisted development centralizes review
- **A-25.56** — Private contracts are knowledge capture, not completion certificates
- **A-25.59** — Issue-led support versus a staffed community
- **A-25.60** — Extension model trades ecosystem reach for bounded ownership

### T13 — Personal discovery stays local and distinct from saved intent (34 members)

**The bet.** Personal discovery is a local product capability, not a cloud social graph. Watching, explicit saved intent, favourites, ratings and exclusions are distinct signals; a maintained profile taste model can power discovery across media. Other members' activity is optional and permission-filtered.

**Counter-position.** Jellyfin's recommendation surfaces are generally simpler and more query/category driven in these documents. Plex has a richer entertainment-market and social graph, while Plexamp adds sonic discovery; neither has the same local-only boundary (A-12.45–54; A-23.12, A-23.23–24). Plezy mainly consumes backend recommendations, and specialist music tools invest in radio and listening-specific discovery.

**Buys and costs.** Typed features, cold start, diversification, feedback/Undo and explicit privacy gates are unusually ambitious and structurally strong (A-06.46; A-12.30, A-12.45–53). The model adds triggers, maintenance and subtle signal interpretation. It does not replace sonic similarity or a “where can I watch this?” catalogue; recommendation explanations and recently played music remain weaker (A-12.29, A-12.47, A-12.54; A-23.12).

**Intent and Justin's philosophy.** Explicit in Recommendations:3, 7–11, 21–31, 54; decisions P15 preserves distinct personal fields. This is Plex-shaped polish with Jellyfin-like private ownership. It should work with local metadata rather than requiring online identity everywhere.

**Inconsistent application.** Container watched marks may not reach the taste model, paused reports move activity, and aggregate counts can disagree with privacy expectations (B-12.6, B-12.12, B-12.22). Discovery rows rotate by UTC rather than the viewer/server day (A-12.52). “My List” is deliberately a destination, yet a system index expects an absent Home row (A-21.15; B-21.4). Threshold and Next Up details must be treated as explicit semantics rather than a popularity proxy.

**Decisions for Justin.** Keep local feature discovery only, add optional sonic features, or expose external discovery/request adapters? Should users get a clear “because…” explanation and more precise feedback before another model is added? Should incognito preserve resume while excluding taste/history? Which manual/container actions count as engagement, and what household aggregation minimum protects small groups?

**Member entries**

#### 02 Playback decisions and protocol

Explicit personal intent, local taste signals and household discovery.

- **A-02.33** — Thresholds
- **A-02.37** — When a play is counted

#### 06 Metadata and artwork

Explicit personal intent, local taste signals and household discovery.

- **A-06.46** — Trending feeds, provider "similar" lists and the Portico title dataset

#### 12 Personal state and discovery

Explicit personal intent, local taste signals and household discovery.

- **A-12.7** — Completion by credits marker
- **A-12.8** — Unknown duration
- **A-12.12** — Play count means "played to the end", counted once per playback
- **A-12.13** — "Last played" moves on every accepted report, including while paused
- **A-12.21** — Next Up is anchored on the last active episode, not the last watched one
- **A-12.23** — Season premieres are exempt from the window
- **A-12.26** — Remove from Continue Watching, with Undo and show-level scope
- **A-12.27** — Marking episodes watched by hand does not "start" a show
- **A-12.29** — Continue Listening is audiobooks only; music has no "recently played"
- **A-12.30** — Four separate personal signals
- **A-12.31** — My List is local to one server and holds only what is in the library
- **A-12.32** — Which kinds carry which flag
- **A-12.33** — My List has no "when I saved it"
- **A-12.34** — Playlists
- **A-12.35** — Collections: two kinds
- **A-12.38** — The hero is "what you were last in the middle of"
- **A-12.39** — No library tiles on Home
- **A-12.41** — The owner cannot arrange or add rows for everyone
- **A-12.45** — A maintained taste model versus per-request category lists
- **A-12.46** — Coverage: every media family, on Home, in libraries, on title pages and as a sort
- **A-12.47** — Explanation of why something is recommended
- **A-12.48** — Feedback controls
- **A-12.49** — Cold start
- **A-12.50** — Restricted (kids) profiles
- **A-12.51** — Community signals are opt-in twice and need at least two people
- **A-12.52** — Daily rotation and exploration
- **A-12.54** — "Discover" means inside the library

#### 18 Web experience

Explicit personal intent, local taste signals and household discovery.

- **A-18.15** — Personal state vocabulary
- **A-18.17** — Two things called "collection"

#### 23 Adjacent competitors

Explicit personal intent, local taste signals and household discovery.

- **A-23.12** — Loudness/trim are not sonic similarity
- **A-23.24** — Public social graph versus opted-in household signal

### T14 — Offline copies remain verified, scoped parts of the server (38 members)

**The bet.** A download remains part of a server-owned media relationship rather than becoming an unrestricted independent copy inside the app. Exact verified bytes, scoped ownership and a time-limited signed authorization allow offline use without pretending revocation can travel through no network. Preparation and transfer are explicit states with known size and integrity.

**Counter-position.** Jellyfin's simple original file download has less ceremony; Streamyfin sends download capabilities and builds a fuller offline experience. Plezy/mpv plays broader originals and carries sidecars; Finamp invests in synchronized offline music (A-14.1–5, A-14.14–26). Plex's rolling desired-content rules are a convenience benchmark. The comparison is app behaviour, not DRM: an owner can still access their original files.

**Buys and costs.** Shared prepared artifacts, visible size, client hashing, signed receipts and per-viewer isolation are valuable (A-14.3, A-14.7, A-14.17, A-14.31; A-21.31–33). Copying and hashing originals twice, per viewer, uses weak-server disk/I/O; short transfer grants and missing resume waste completed work (A-14.2, A-14.13–14). Flat JSON records and media-only payloads limit offline browsing, subtitles and queues (A-14.20–26).

**Intent and Justin's philosophy.** Explicit: decisions P06/P15; saved-state-and-downloads/contract.md:233–239, 268 and 301. Thirty-day verification, protected inactive profiles, and preserving bytes on expiry/unreachability are deliberate. A server-retention event deleting a usable device copy is a distinct implementation outcome, not automatically part of that choice.

**Inconsistent application.** Device lifetime is coupled to server claim retention (A-14.9), sign-out does not follow the written deletion rule (A-14.32), and offline progress follows different rules than online (A-14.27–30). Original format is offered without device compatibility (A-14.4). The signed-out flow can use a second system player (A-14.24), defeating consistent subtitle/queue/resume experience. These gaps hurt the promised polish more than the existence of receipts helps it.

**Decisions for Justin.** Keep expiring server authorization, allow owner-authorized durable exported copies as a separate action, or change app downloads to permanent copies? Decouple preparation retention from valid device receipts? Keep original snapshots or use a lighter immutable/leased source path with equivalent byte integrity? Deliver a grouped offline subset first, or a full offline shell with one player and shared reconciliation?

**Member entries**

#### 04 Subtitles and analysis

Offline artifact identity, scoped lifetime, transfer and local product completeness.

- **A-04.46** — Prepared versions, downloads and offline

#### 14 Downloads

Offline artifact identity, scoped lifetime, transfer and local product completeness.

- **A-14.1** — A download is a stored, stateful claim, not a file request
- **A-14.2** — "Original" is copied into the state folder and hashed twice before it can be fetched
- **A-14.3** — Converted downloads are finished files made ahead of time and shared
- **A-14.7** — Size is known and published before the user commits
- **A-14.8** — Server storage is bounded by an owner ceiling counted in claims
- **A-14.9** — A device copy lives only as long as the server-side claim
- **A-14.10** — Offline permission is a signed, expiring receipt
- **A-14.11** — The receipt is implemented differently from the contract's design
- **A-14.12** — Every state change is fenced and idempotent
- **A-14.13** — Transfer permission is a 10-minute capability address
- **A-14.14** — The server can resume; the apps do not
- **A-14.17** — End-to-end integrity check
- **A-14.19** — Where files live on the device
- **A-14.20** — The device catalogue is two JSON documents
- **A-14.21** — Only the media file travels
- **A-14.22** — Titles are remembered only for single-title requests
- **A-14.23** — Offline browsing is one flat list, reached from an error screen
- **A-14.25** — A downloaded copy is used only from the Downloads screen
- **A-14.26** — Offline playback has no queue
- **A-14.27** — What is recorded while offline
- **A-14.28** — How offline progress is merged
- **A-14.29** — Offline progress is written straight into the tables, bypassing the online rules
- **A-14.30** — Server state is never pulled down to a downloaded copy
- **A-14.32** — Sign-out keeps the files
- **A-14.33** — Who may download
- **A-14.34** — Whole-container requests are expanded by the server
- **A-14.35** — Automatic downloads are one-shot
- **A-14.36** — Network and power rules
- **A-14.37** — Device storage rules
- **A-14.39** — Web is "save a file", two steps, opaque name

#### 16 Web architecture

Offline artifact identity, scoped lifetime, transfer and local product completeness.

- **A-16.49** — Installability and offline shell

#### 18 Web experience

Offline artifact identity, scoped lifetime, transfer and local product completeness.

- **A-18.42** — Downloads in a browser

#### 19 Native architecture

Offline artifact identity, scoped lifetime, transfer and local product completeness.

- **A-19.13** — No local database

#### 21 Native experience

Offline artifact identity, scoped lifetime, transfer and local product completeness.

- **A-21.31** — Downloads distinguish server preparation from protected local bytes
- **A-21.32** — Rolling offline rules are not exposed

#### 22 Connectivity

Offline artifact identity, scoped lifetime, transfer and local product completeness.

- **A-22.47** — Offline launch prioritizes shell/download access differently by platform

#### 24 Community lessons

Offline artifact identity, scoped lifetime, transfer and local product completeness.

- **L-24.21** — Offline mode is a product state, not a download button

### T15 — Freshness follows evidence and change, not a timer (41 members)

**The bet.** Derived product state should become fresh because the evidence changed, not because an arbitrary cache lifetime elapsed. Revision-bound memos and invalidation events let Portico retain correct reuse without trusting stale content. Event-driven workers and sparse Hosted updates keep an idle system quiet.

**Counter-position.** Jellyfin and its clients use more timers, caches and ephemeral WebSocket events; these can show content faster and simplify invalidation at a staleness cost (A-08.21–25; A-16.7, A-16.14). Plex's exact internal freshness policy is not available here; its visible immediacy remains the experience target. Plezy uses practical persisted/cache state and backend-specific refresh rules, while some specialist clients deliberately make cached content the offline default.

**Buys and costs.** Exact Home/cursor revision keys, durable event resumes and conditional provider requests avoid arbitrary stale periods (A-06.6; A-12.43; A-22.38–39, A-22.42). Conservative no-cache screen ownership can still impose repeated loads on Back. Broad revision bumps can invalidate unrelated views, and events that merely nudge all readers create fan-out work (A-07.16; A-08.25; A-16.7).

**Intent and Justin's philosophy.** Explicit in Overview:49, 52 and Recommendations:7, 31; architecture.md:61–67 sets out event/projection semantics. This should improve polish and resource efficiency. “Fresh” does not require discarding a revision-keyed last good view, and it does not forbid time-based validity for external evidence such as provider freshness or certificates.

**Inconsistent application.** Static browse-capability memos have no revision key, guide reads have a time-based reuse exception, and important catalogue/personal changes are not published/consumed across the UI (A-16.14; A-22.37; B-16.19). A directory timestamp misses in-place file edits while later media fences still demand exact facts (A-05.4–5). Long polling and separate event feeds persist despite a unified feed architecture (A-08.21; A-22.43). Hosted outage gates exist but activation can bypass them (B-22.26).

**Decisions for Justin.** Allow immediate display of a bounded, viewer-scoped previous document followed by exact revalidation, or require each screen to load fresh before display? Which revision granularity and invalidation events should be mandatory? Keep intentional transport-time memos as documented exceptions, or replace them with explicit revisions? Wire bounded streaming where it materially reduces local-server chatter, or retain polling with accepted measured cost?

**Member entries**

#### 05 Inventory and naming

Revision-driven reuse, change propagation, event delivery and quiet idle operation.

- **A-05.4** — Change detection: directory fingerprints with a weekly safety net
- **A-05.5** — What triggers a scan

#### 06 Metadata and artwork

Revision-driven reuse, change propagation, event delivery and quiet idle operation.

- **A-06.6** — Freshness windows, a refresh budget and conditional requests
- **A-06.7** — No scheduled refresh
- **A-06.39** — Image language is recorded but not used to choose
- **A-06.48** — Provider failures are durable state

#### 07 Persistence and queries

Revision-driven reuse, change propagation, event delivery and quiet idle operation.

- **A-07.16** — Revision counters are bumped by triggers on almost every row

#### 08 API and events

Revision-driven reuse, change propagation, event delivery and quiet idle operation.

- **A-08.13** — Conditional requests and cache headers
- **A-08.21** — Real-time transport: SSE and long-poll versus one WebSocket
- **A-08.22** — Events as invalidations versus events as data
- **A-08.24** — What is actually pushed
- **A-08.25** — Fan-out mechanism
- **A-08.27** — One stream per installation
- **A-08.28** — Subscriptions and topics

#### 10 Music and books

Revision-driven reuse, change propagation, event delivery and quiet idle operation.

- **A-10.36** — Lyrics are pinned to one exact version of the file

#### 12 Personal state and discovery

Revision-driven reuse, change propagation, event delivery and quiet idle operation.

- **A-12.43** — Freshness by revision, never by time
- **A-12.57** — Recent searches

#### 13 Social playback and receivers

Revision-driven reuse, change propagation, event delivery and quiet idle operation.

- **A-13.14** — Transport and chattiness
- **A-13.27** — Finding devices: server list ∩ local network
- **A-13.28** — How a TV learns it has been asked
- **A-13.44** — A real inbox, in-app only
- **A-13.45** — Notices are raised only by server housekeeping

#### 16 Web architecture

Revision-driven reuse, change propagation, event delivery and quiet idle operation.

- **A-16.7** — No cache between screens versus cached, persisted queries
- **A-16.14** — Live updates: long-poll with two listeners versus a WebSocket that drives the UI
- **A-16.21** — Static asset caching and compression

#### 19 Native architecture

Revision-driven reuse, change propagation, event delivery and quiet idle operation.

- **A-19.12** — Nothing fetched is kept between launches
- **A-19.29** — Images: the built-in Image with Portico's own scheduler
- **A-19.40** — Background work
- **A-19.50** — Server events over XMLHttpRequest

#### 22 Connectivity

Revision-driven reuse, change propagation, event delivery and quiet idle operation.

- **A-22.9** — First verified winner versus two-phase optimization
- **A-22.10** — Last-good precedes LAN quality in ordering
- **A-22.12** — Fresh directory discovery is hedged and rate-limited
- **A-22.14** — Ready connections are not re-evaluated on foreground alone
- **A-22.35** — Retry-After is several policies, not one
- **A-22.36** — Jitter exists for route, event and timeline recovery but not every auth loop
- **A-22.37** — Guide read reuse is bounded but time-based
- **A-22.38** — Subscriber-aware deduplication cancels only orphaned reads
- **A-22.39** — ETags and revisions are application-owned
- **A-22.42** — One cursor-resumable device feed with explicit resync
- **A-22.43** — Native uses short polling rather than core's default SSE

#### 24 Community lessons

Revision-driven reuse, change propagation, event delivery and quiet idle operation.

- **L-24.26** — Search should fail by section, not erase everything

## 2. Local differences

**72 entries.** These are specific grammar, tuning, diagnostics, endpoint or packaging choices for which the evidence does not establish a recurring product bet. Some are defects or gaps; “local” does not mean trivial. They still participate in secondary process patterns where relevant. A repeated implementation error, such as two year readers disagreeing, is not itself a philosophy. The one-line assessment below is the original index wording, retained rather than silently rejudged.

### 01 Identity and access

- **A-01.44** — A member cannot be deleted, only disabled. Portico behind. Small-to-medium: the erasure function exists; it needs an owner route and a confirmation that says what is erased.
- **A-01.52** — Usernames. Portico behind on consistency (see doc/code); trivial to unify.

### 02 Playback decisions and protocol

- **A-02.44** — Pre-rolls. Portico behind, low priority. See F-02.15.

### 03 Transcoding

- **A-03.2** — Absolute timestamps via -output_ts_offset versus -copyts. Trade-off. Portico's is simpler to reason about. One residual difference: the first window (offset 0) can have slightly negative audio timestamps from AAC encoder priming, which FFmpeg shifts for that window only, so a restarted window is ~21 ms off from window zero. Harmless in practice; noted fo
- **A-03.5** — Segment length and grid. Trade-off. Portico's fixed value removes a class of bugs, and Apple recommends 6 s. Worth measuring start and seek latency on a weak box before freezing it; a 4 s grid for conversions (what Portico's Live TV already uses) would be a middle path. Changing it later is easy only if no client assumes
- **A-03.17** — Deinterlacing. Portico behind, modest. Double-rate as an option is small; GPU deinterlace is medium.
- **A-03.18** — Scaling and pixel aspect. Portico behind, small: always emit a scale-to-even, square-pixel step in the software path.
- **A-03.19** — Rotation. Roughly equal in software; Portico behind on GPU paths, where automatic rotation cannot run on GPU frames and the session falls back to software (and counts as a hardware failure, B-03.6). Small.
- **A-03.20** — Audio conversion targets. Trade-off, Portico simpler. Add -ar 48000 when converting (Live TV already does: linear_hls.go:123).
- **A-03.21** — Downmix. Portico ahead on the default, behind on choice. See F-03.9.
- **A-03.22** — Audio copy into HLS. Trade-off. Few HLS players accept DTS/TrueHD; the people who care (AVR passthrough from an Android TV box) mostly direct-play. Trivial.
- **A-03.34** — Failure diagnostics. Portico behind. Small to medium: keep a bounded ring of recent conversion records (arguments with paths redacted, exit status, error tail, speed) independent of the session's lifetime.
- **A-03.36** — Input handling for odd sources. Portico behind, small. +genpts on copy routes and +discardcorrupt for MPEG-TS inputs are cheap.
- **A-03.37** — Stream-copied audio next to converted video. Unclear — Portico has the same mechanism and no trim. See B-03.12.
- **A-03.39** — Session-private output, no reuse. Equal.
- **A-03.42** — Threads. Trivial, but an owner-visible thread cap is a common request on shared boxes.
- **A-03.46** — Rate limiting a remux. Trivial. Portico's Library Channel path already uses -readrate 1 (linear_hls.go:88); adding a modest -readrate to on-demand copies would make the buffer setting exact.

### 04 Subtitles and analysis

- **A-04.12** — Burn-in geometry. Portico behind on the mismatch case (needs a runtime check); trade-off on where the overlay happens. Small fix once confirmed.
- **A-04.18** — No audio delay control. Portico behind. AVPlayer has no direct audio delay; Android ExoPlayer and the web can approximate. Medium, platform-dependent.
- **A-04.21** — SDH detection. Portico behind. Small (carry the flag through subtitles.Track).
- **A-04.22** — Audio default and language matching. Trade-off, with a real bug in the table. No "original language" preference in Portico (F-04.6).
- **A-04.30** — Chapter thumbnails. Portico behind on picture quality (chapter starts are very often black or mid-fade). Small.

### 05 Inventory and naming

- **A-05.18** — Daily shows become "season = year, episode = day of the year". Trade-off; the folder check should not apply to dated files.
- **A-05.23** — Extras. Trade-off. Deliberate ("Sidecar extras are recognised from the owning media folder", extras.go:12).
- **A-05.29** — Provider id hints in names. Portico behind; small.
- **A-05.30** — Which year wins. Portico behind (B-05.7). Two readers with different rules for the same job is also a smell: FilenameTitle and screenFilename disagree on 1917 (2019) and on Movie.2048x1080.
- **A-05.41** — Unicode and letter case. Unclear (nobody does this well). Normalising to NFC when building keys and comparing titles is cheap (golang.org/x/text/unicode/norm is already imported in metadata/screen_match.go).

### 06 Metadata and artwork

- **A-06.10** — Year parsing takes the first year-looking number. Portico behind (a bug, not a choice).
- **A-06.11** — Which ID hints are understood. Portico behind. Small: TMDB's /find/{imdb id} gives the TMDB id; TheTVDB has a remote-id search. See F-06.1.
- **A-06.13** — Title comparison. Portico ahead on alias coverage (it fetches alternative titles for the top four), behind on a few common normalisations. Small additions.
- **A-06.24** — NFO file names looked for. Portico behind, small.

### 07 Persistence and queries

- **A-07.32** — Filtering by name instead of by identity. Portico behind. Small to add id-typed fields beside the text ones.
- **A-07.37** — Search ranking. roughly equal. bm25 over a one-field document mostly measures title length.
- **A-07.39** — Sort-key rules. Trade-off. Portico is better for non-English libraries; Jellyfin is better on numbers and punctuation. Both rules could coexist (F-07.10, F-07.11).
- **A-07.40** — Ordering of non-Latin text. Equal with Jellyfin. The contract's cursor rule mentions binding "locale/collation"; today there is one collation, so nothing needs binding.
- **A-07.42** — Dates and times in storage. Portico behind in uniformity; it is the root of B-07.8.
- **A-07.45** — Removing a library. Portico behind; not in Backlog.md. See F-07.14.

### 08 API and events

- **A-08.33** — Client log upload. Portico ahead (structured, bounded, surfaced in the console).
- **A-08.36** — Health and readiness. Portico ahead. Three endpoints for one question is one or two too many for outsiders.
- **A-08.37** — Capability negotiation. Portico ahead. Make it trustworthy: one advertised limit is already wrong (B-08.5), and limits.ts in the generated contract is two lines long.
- **A-08.39** — Observability of the API itself. Portico ahead internally; behind on standard outward signals (no response-time header, no Prometheus endpoint; F-08.15). Legacy handlers do not return a request id.
- **A-08.40** — Route matching inside the registry. Trivial, noted for completeness.
- **A-08.41** — Identifier and unit conventions. Portico ahead on ids, mixed on units. One rule for time and duration before /v1 is frozen would spare every client a table of exceptions.

### 09 Server operations

- **A-09.36** — Acting on a live session. Level on stopping, behind on a message to one viewer (F-09.21). Whether the owner should be able to drive someone else's player is a product question.
- **A-09.39** — Logs. Trade-off, leaning behind for troubleshooting. Deliberate in spirit (bounded, redacted, time-boxed debug per settings contract §10).
- **A-09.40** — Diagnostics beyond logs. Portico ahead, clearly.
- **A-09.43** — Health endpoints. Portico ahead.

### 10 Music and books

- **A-10.8** — Genres split on fixed characters. Trade-off, minor. Matches Navidrome's default. A per-library "don't split these" list would be small.
- **A-10.23** — Sample-rate changes between tracks. Trade-off. Recreating the context when the queue moves to a different rate and nothing is joined to the old one would restore exactness; small to medium.
- **A-10.27** — Speed. Level on mechanism, behind on range. The spec promised 0.5–3.0 (§12). Small.

### 12 Personal state and discovery

- **A-12.64** — Operating-system home integrations. Unclear (not read). Parity appears to exist.

### 13 Social playback and receivers

- **A-13.22** — Names in the roster are typed by the member. Portico behind, small: take the name from the profile, or at least apply the same text filter.

### 14 Downloads

- **A-14.15** — Background transfer machinery. Portico behind on Android; roughly level on iOS apart from A-14.13/14. Medium work: WorkManager (or DownloadManager) plus the resume fix.
- **A-14.43** — Audit trail. Roughly level; a line for "transferred to device X" would complete it. Small.

### 16 Web architecture

- **A-16.41** — Text the server sends. Portico ahead.
- **A-16.61** — Client diagnostics. Portico behind, mildly. See F-16.13.

### 17 Web player

- **A-17.26** — Position for music comes from the audio clock, without output latency. Portico slightly behind. Small: subtract outputLatency where the browser reports it.
- **A-17.29** — A click anywhere on the picture toggles play, for mouse and touch alike. Portico behind. Small.
- **A-17.30** — Keyboard map is short and matched on the typed character. Portico behind. Small.
- **A-17.31** — Volume is linear and saved per browser. Portico slightly behind. Trivial.
- **A-17.39** — Playback speed range. Portico behind its own spec for audiobooks. Trivial.

### 20 Native players

- **A-20.24** — Television seeking: fixed steps, no acceleration; Siri Remote surface scrubbing is custom. Portico behind on Android TV and Fire TV, roughly at parity on Apple TV. Small: key-repeat counting in the shared player.
- **A-20.33** — Playback speed: seven fixed steps to 2×. Portico slightly behind. Small.

### 22 Connectivity

- **A-22.28** — General transport policies remain surface-owned. Portico behind for policy consistency. Medium for one operation-classed transport that permits explicit long-poll/media exceptions and avoids mutation replay.
- **A-22.54** — Clock tolerance differs between server admission and client route proof. Trade-off (subjective). Short proof validity improves freshness, but nonce uniqueness already prevents replay for this exchange. Medium to define a coherent bounded clock policy and distinct actionable messages.

### 25 Engineering

- **A-25.10** — Error conventions combine typed sentinels and a large transport map. Trade-off (subjective) — Useful public boundary, substantial maintenance load. Medium: per-area mappings and table-driven complete error tests; keep private causes separate from public strings. Do not replace the map with raw err.Error responses.
- **A-25.11** — Plain logs plus local operational records. Trade-off (subjective) — Medium: establish a small structured local logging wrapper with request/operation/build IDs and redaction, keeping sensitive fields out. No hosted collector is required. Reconsider if current support exports already provide sufficient correlation in real failures.
- **A-25.29** — CI does not imply required checks. Unclear — Small/medium: export/document required checks and tag/environment protections, distinguish human review from independent agent review, and bind release authorization to exact SHAs. No remote settings were queried.
- **A-25.36** — Signing and notarization are preview-only promises. Portico behind — Honest prerelease staging, appropriate automatic-update hold. Medium/large platform release work before marketing unattended/store distribution. No evidence signing accounts/secrets have been provisioned was collected.
- **A-25.37** — Native target runtime testing differs from cross-compilation. Portico behind — Medium: startup/readiness/shutdown/package smoke on every shipped target; medium/large hardware qualification separately. Avoid rerunning only host tests and calling that platform validation.
- **A-25.44** — Baseline and squash reset provenance. Trade-off (subjective) — Deliberate pre-release cleanup. Small/medium: retain searchable private history/bundles and provenance links, keep future meaningful small commits. Do not equate a short public history with a young implementation or lack of past review.
- **A-25.51** — FFmpeg distribution chooses GPLv3 and rejects nonfree. Portico ahead — Explicit recipe/license guard is good. Medium ongoing work: audit dependency source completeness and package notices on each release; codec patent obligations are a separate jurisdictional question not settled by source licensing.

## 3. Cross-cutting patterns that are not philosophies

These are overlapping secondary indexes. They do **not** reassign architectural entries, and they do not claim every referenced B finding is independently confirmed. B-index also includes Q-24 acceptance questions; those are unresolved questions, not demonstrated bugs. Deliberate HTTP, simple passwords, no permanent lockout, open data and no push are not defects merely because an auditor placed their residual risk in B.

### P1 — Specified or built, but the user path stops before the effect

Backend/API presence, preference rows, or native modules are repeatedly mistaken for an available end-to-end feature. Examples include Auto quality, subtitle uploads, instant mix, chapter/trickplay utilities, search preferences and system-shelf indexing. This is often unfinished integration rather than a reason to reverse the underlying architecture.

- **A/L evidence:** A-02.19, A-04.40, A-04.43, A-10.32, A-11.48, A-16.19, A-21.11, A-21.27.
- **B evidence:** B-02.6, B-04.24, B-18.3, B-18.27, B-20.15–17, B-21.4, B-21.6, B-21.10.
- **Process implication:** Track a feature by command → consumer → effective result on each supported platform. A server implementation and a consumed field are separate milestones; neither certifies a reachable control.

### P2 — One concept retains two implementations or vocabularies

Identity devices/restrictions, capability profiles, subtitle plans, playback engines/adapters, generated types and API families coexist. Some coexistence is deliberate migration, but the active owner and retirement condition are not consistently obvious.

- **A/L evidence:** A-01.5, A-01.31, A-02.4, A-03.43, A-04.7, A-08.3, A-08.9, A-16.12, A-17.35, A-20.5, A-21.13, A-25.3.
- **B evidence:** B-01.22, B-02.17, B-04.33, B-09.24, B-15.6, B-25.5.
- **Process implication:** Every migration needs an explicit production-path map, the exact supported old/new combinations, one table/wire owner and a removal criterion. Two representations are particularly dangerous when the poorer one silently wins.

### P3 — Settings describe desired behaviour without applying it

The console can save, validate and display fields whose runtime consumers are absent or different. This is more misleading than an honestly missing feature because the owner can reasonably believe an action succeeded.

- **A/L evidence:** A-03.29, A-09.11, A-09.38, A-11.15, A-11.48, A-12.58, A-21.12–13, A-21.53.
- **B evidence:** B-01.4, B-03.10, B-09.9–10, B-09.28, B-11.9, B-18.11, B-20.12, B-20.17, B-21.6–7.
- **Process implication:** Maintain a field matrix with storage scope, authored control, effective consumer and application timing. Consumer checks should prove a useful effect, not just a text reference. Distinguish desired from effective state in readback.

### P4 — Strictness expands one imperfect record into a whole failure

Track limits replace all tracks with none; bad subtitle/chapter/lyric lines erase good material; changing or unreadable directories fail a scan; one guide/provider/parser error rejects a whole document. Unknown optional enum values can do the same to client screens.

- **A/L evidence:** A-04.29, A-04.47, A-05.6, A-05.28, A-05.43, A-06.23, A-06.49, A-08.8, A-10.35, A-10.42, A-11.6–7, A-16.11.
- **B evidence:** B-04.3, B-04.5–7, B-04.10, B-04.23, B-04.28, B-05.2, B-05.4–5, B-05.29, B-06.16–17, B-10.8, B-10.16, B-11.2–7, B-16.10, B-22.1–2.
- **Process implication:** Review the smallest safe rejection unit. Keep hard security and memory bounds; quarantine the failed record or subtree, preserve trustworthy siblings, and expose warning counts. Do not conflate an unsafe input with an unfamiliar optional field.

### P5 — A limit becomes a product truncation instead of a page boundary

Several otherwise bounded systems stop at a fixed prefix or reject oversized scope rather than continue. Complete-result browsing is a principle, so bounded work must eventually visit all eligible members.

- **A/L evidence:** A-04.47, A-07.31, A-11.7, A-18.39, A-19.28, A-21.24, A-21.30, L-24.3.
- **B evidence:** B-04.27, B-11.5, B-18.4, B-21.5, B-21.11.
- **Process implication:** Use continuation frontiers and bounded storage queries. Prove the last page and whole-query playback/selection, rather than testing only a comfortably small first page. A safety maximum may still be valid if it is a declared capability rather than silent incompleteness.

### P6 — Revisions are correct individually but cover too much unrelated activity

Progress, readiness, directory publication and token rotation invalidate browsing/control or recreate screen services. Some counters fence current state; others represent observations. Treating them as one version creates conflict churn.

- **A/L evidence:** A-07.15–16, A-08.13, A-13.13, A-14.12, A-22.39–40.
- **B evidence:** B-07.4–5, B-08.8, B-11.1, B-13.1, B-14.8–9, B-14.13, B-14.17, B-19.2.
- **Process implication:** Name each revision by its semantic owner and enumerate its writers. Separate authorization, membership, content order, command state and observation progress. Replay tests should include harmless concurrent activity.

### P7 — Parallel input paths bypass the canonical rule

Online progress, offline progress, manual/container flags and social/player effects have different arbitration, completion or permission behaviour. A declared shared model can still be undermined by a direct write path.

- **A/L evidence:** A-02.34–37, A-12.1–2, A-12.14–15, A-12.63, A-14.27–30, A-15.1, A-15.30.
- **B evidence:** B-02.7, B-12.1, B-12.3, B-12.5–8, B-12.14, B-14.10–11, B-15.1, B-21.12.
- **Process implication:** Enumerate all writers and authority entry points for each durable fact, then run the same semantic vectors through them. Differences may be deliberate, but they need explicit rules and user wording.

### P8 — A declared capability is broader than the effective implementation

Lossy translation removes valid codec/sink facts, while other profiles advertise ASS/PGS, chapter/trickplay or effects with no working renderer/control. Download quality offerings outrun the recipes that exist.

- **A/L evidence:** A-02.3–7, A-02.16–17, A-14.4–5, A-20.4–6, A-20.17–23, A-21.27, A-21.53, L-24.14.
- **B evidence:** B-02.1–5, B-02.14–15, B-14.4, B-14.12, B-20.8–12, B-20.14–15, B-20.20, B-21.10.
- **Process implication:** Test the published document through every conversion into the effective planner/engine. Separate compiled, declared, probed, rendered and physically qualified capabilities. A positive platform API answer is useful evidence, not an entire playback promise.

### P9 — Producers and consumers disagree at a wire/document seam

Release manifests, secure LAN origins, subtitle format labels, retry headers, events and generated API descriptions do not always describe what the other side accepts. Internal unit correctness cannot settle these seams.

- **A/L evidence:** A-04.3, A-08.3–9, A-08.37, A-09.51, A-22.6, A-22.16, A-22.49, A-25.38, A-25.42, A-25.46.
- **B evidence:** B-04.36, B-08.5, B-08.10–11, B-09.11, B-22.1–2, B-22.23, B-25.1, B-25.17.
- **Process implication:** Generate real producer output and feed it into the actual consumer. Keep the fixture tuple tied to producer/client/server versions. Documentation route coverage is not schema or semantic conformance.

### P10 — Transient failure is sometimes classified as final loss

Careful retry-safe owners coexist with paths that erase sign-in on refusal/storage errors, park provider work after a short outage, or lose download recovery. Different Retry-After parsing and jitter policies amplify storms.

- **A/L evidence:** A-01.19, A-06.48, A-14.13–16, A-14.28, A-22.26–28, A-22.33, A-22.35–36, A-22.54.
- **B evidence:** B-01.9, B-06.12, B-14.2–3, B-14.16, B-16.20, B-22.4–5, B-22.9–12, B-22.25–26.
- **Process implication:** Share failure classes and retry-floor parsing. A timeout or unreadable store is not evidence of revoked authority. Exercise restart, retry and storage failures with the same operation ID and distinguish explicit refusal from an unknown outcome.

### P11 — Event machinery exists, but ordinary screens are not kept current

The unified feed is durable and resumable, yet many changes are not emitted or consumed; long-poll clients and auxiliary feeds bypass part of the intended common transport. A healthy server may still display outdated content until remount.

- **A/L evidence:** A-05.5, A-08.21–28, A-12.43, A-16.14, A-19.50, A-22.37–43, A-22.46.
- **B evidence:** B-08.3–6, B-16.7, B-16.19, B-22.13, B-22.18, B-22.26.
- **Process implication:** Define the minimal resource-change vocabulary and required subscribers, including gap/resync behaviour. Evaluate end-to-end idle/load cost; streaming versus polling is a transport choice, while missed invalidation is a correctness issue.

### P12 — Bounded foreground work hides unbounded or repeated lifecycle cost

Duplicate frames, original snapshots/double hashing, accumulated ledgers/people/claims, per-viewer live work, whole-library requeueing and wide event wakeups spend resources outside the ordinary page test.

- **A/L evidence:** A-04.36–43, A-05.3, A-05.36, A-05.46, A-06.4, A-06.26, A-07.44, A-08.25, A-11.1, A-14.2, A-14.20.
- **B evidence:** B-01.18, B-04.29, B-05.20–22, B-06.20, B-06.24, B-07.15, B-08.6, B-11.21, B-11.23, B-11.26, B-11.31–32, B-13.10, B-14.5, B-14.24–25, B-19.14.
- **Process implication:** Include retained bytes, worker throughput, cleanup, wake frequency and per-session source cost in the budget. Test aged/churned state as well as newly seeded catalogues. Share work only where identity and authority permit it.

### P13 — Shared semantics are bypassed by strings, units or scope labels

Dates use different text layouts; language tables drift; components use browser locale or raw English; device-class settings say “this device.” These look small individually but undermine the central consistency promise.

- **A/L evidence:** A-01.52, A-04.22, A-07.42, A-08.41, A-16.35–41, A-18.38, A-18.51, A-21.12, A-21.50.
- **B evidence:** B-02.17, B-04.15–16, B-07.8–10, B-16.6, B-18.12, B-18.19, B-18.22–24, B-19.16–17, B-20.8, B-21.7.
- **Process implication:** Use semantic IDs and typed units at boundaries; route displayed text/formatting through the shared locale owner. Check the whole scope label against storage effects. Expand language/long-string vectors before adding translations.

### P14 — Proof is attached to a helper or source shape, not the shipping outcome

The unwired player has tests, while the live adapter does not have equivalent behavioural tests. Headless harnesses, regex/SQL guard exclusions and debug/cross-build CI are useful but cannot prove rendering, hardware, packaged dependencies or release identity.

- **A/L evidence:** A-07.47, A-16.51–54, A-17.35, A-19.43–46, A-25.19, A-25.21–30, A-25.37, A-25.56, L-24.12–14, L-24.30.
- **B evidence:** B-17.25, B-19.15, B-25.9–10, B-25.13–17, B-25.19, B-25.21.
- **Process implication:** Tie evidence to the active wiring, exact build and device/route. Keep cheap invariant tests, but add lifecycle/component/package/device evidence at the corresponding boundary. Neither code volume nor test volume is qualification.

### P15 — Recovery and support lose the cause that would make them useful

Some strong diagnostics coexist with immediately deleted conversion errors, path-free scan failures with no owner detail, thin client crash records, generic network failures and missing field-dependency explanation.

- **A/L evidence:** A-03.34, A-05.38, A-08.33, A-08.39, A-09.39–40, A-19.38–39, A-21.49, A-22.50, A-25.11.
- **B evidence:** B-03.11, B-04.8, B-05.30, B-08.2, B-09.23, B-09.29, B-19.6, B-20.6.
- **Process implication:** Keep a bounded, redacted causal record tied to operation/build identity after transient resources disappear. Give consumers simple actions while preserving enough owner evidence to distinguish server, network, decoder, storage and policy failures.

### P16 — Release identity and operational lifecycle stop before the final boundary

Source sealing, signing and package checks are valuable, but later web/packaging bytes, optional signers, differing service models, moving cross-repo branches and missing restart hooks can escape the guarantee. This is a completion/gate issue, not an argument for a different media-server philosophy.

- **A/L evidence:** A-09.3–9, A-09.48–52, A-15.48, A-25.2–3, A-25.31–44.
- **B evidence:** B-09.2–5, B-09.19–20, B-09.27, B-25.2–8, B-25.18–21.
- **Process implication:** Qualify the final artifact and its install → upgrade → stop → restart → restore lifecycle on each shipped OS. Pin cross-repo inputs, require the intended signer and bind tests to the requested release source. Separate known prerelease holds from newly discovered defects.

## 4. Where Portico is clearly ahead

The positive evidence is substantial, especially in the architecture of authority, data access and recovery. The strongest consolidated areas to preserve are:

- **Household authority without cloud-owned media sessions:** account/profile separation, SQL restrictions, local membership, privacy-aware personal resources and signed identity evidence (A-01.1, A-01.10, A-01.36–37, A-01.47; A-04.26; A-12.16–17; A-22.3–6).
- **Large-library foundations:** narrow catalogue tables, distinct files and titles, precomputed totals/position blocks, prioritized writes, WAL management, windowed clients and typed overload (A-07.1–3, A-07.12–14, A-07.17–20; A-08.19; A-16.24–25; A-19.27; A-22.34). These mechanisms deserve preservation; they are not a measured million-item guarantee.
- **Honest, inspectable lifecycles:** operation identity, replay-safe refresh, source-pinned sessions, progress fences, resumable inventory, tuner reservations, completeness accounting and staged restore (A-01.18; A-02.24–29, A-02.36; A-05.1, A-05.10; A-09.48; A-11.16, A-11.19, A-11.24). Specific defects limit some implementations.
- **Safer media processing and remote reads:** sandbox/descriptor boundaries, outbound DNS and private-origin policy, path restrictions, bounded artwork/XML processing and restricted request paths (A-03.13, A-03.45; A-04.44; A-15.22–30). Platform exceptions must remain visible.
- **Shared product behaviour:** server-authored surfaces, shared headless presentation grammar, enforced design primitives, canonical show workspace and a leased native command/fact bridge (A-16.8–9; A-19.33; A-20.3; A-21.1, A-21.11, A-21.25). Source reuse is an advantage even before device parity is proven.
- **Provenance and reversible owner control:** metadata evidence, locks/history/undo, explicit ordering, candidate/selection artwork, personal subtitles and stable playlist occurrences (A-06.5, A-06.15, A-06.20–21, A-06.38; A-04.24, A-04.26; A-12.34; A-21.29).
- **Local discovery and listening ambition:** maintained taste, feedback, cold start, restriction-aware diversified recommendations, explicit personal fields and server-measured trim/gain with crossfade (A-12.30, A-12.45–53; A-10.15, A-10.20, A-10.22; A-17.23). The listening advantage requires device/perceptual acceptance rather than a feature-name claim.
- **Owner operations and controlled distribution:** integrated diagnostics, history/audit, health, non-root container defaults, component provenance and explicit contracts that do not certify completion (A-09.37, A-09.40, A-09.43; A-25.17–22, A-25.34, A-25.56, A-25.58).

### Complete positive-assessment register

**443 entries** explicitly contain “Portico ahead” in the index assessment: **313 without an identified qualification marker and 130 qualified/mixed entries** under the conservative tagging below. This is an index of all original positive assessments, not 443 independently verified wins. Original wording follows each title so that “ahead on safety, behind on breadth,” “ahead in intent,” and unresolved contradictions cannot disappear into a single score. **†** marks a mixed, conditional, implementation-limited or disputed positive assessment. An unmarked entry is still static audit evidence, not runtime proof. A-03.28 and A-09.45 specifically remain disputed and should not be advertised as shipped advantages.

#### T01 — A free local core, with Hosted kept small (28)

- **A-05.13** — Libraries and folders. *Portico ahead on per-source control; the uniqueness rule is a trade-off (it is a database constraint on a legacy column, libraries.root UNIQUE, so possibly accidental).*
- **A-05.35** — Remote storage is a first-class back-end. *Portico ahead — the largest architectural lead in this domain. Deliberate (contract.md §5–7).*
- **A-09.1** — One self-contained binary that is also its own helper. *Portico ahead. Deliberate (02 Server.md, opening paragraph). Keep it.*
- **A-09.6 †** — Docker: non-root, an init process, and a built-in health check. *Portico ahead on the image, behind on documentation and conventions. See F-09.1.*
- **A-09.14** — Moving a server: absolute paths rewritten at start. *Portico ahead in outcome. The spec says "relative to the state folder, never by absolute path"; the code chose absolute-plus-rebase instead (Doc/code disagreements). Either is defensible; the documents should say which.*
- **A-09.15 †** — Nothing encrypted at rest, with a permissions warning instead. *Portico ahead in intent. The check itself has platform problems (B-09.15).*
- **A-09.20** — One port for HTTP and HTTPS. *Portico ahead for owners. Deliberate (decision P21).*
- **A-09.21 †** — Plain HTTP is refused to non-private peers at the socket. *Portico ahead on safety, behind on flexibility. "HTTP stays first-class" is a stated principle, so the classifier should honour the owner's own LAN networks setting.*
- **A-09.23** — Automatic port mapping. *Portico ahead. The architecture and brief describe remote access as "port forwarding, or none", which undersells it. Gateway detection exists for Linux and macOS only (topology.go:111-166), so on Windows the owner must type the router address into the Advanced section before mapping can work.*
- **A-09.24** — Finding the public address. *Portico ahead functionally. Worth one line in the console and privacy notes, and worth deciding whether Portico's own DNS nodes should offer the reflection instead (see Open questions).*
- **A-09.25** — Names and certificates through Hosted. *Portico ahead of Jellyfin, level with Plex. The backlog records that a fresh issuance and a renewal have not been run end to end since the move to systemd.*
- **A-09.26** — Bringing your own certificate. *Portico ahead. One domain only; and on the Linux package the service user usually cannot read /etc/letsencrypt/live/…, which is the example the console shows (B-09.14).*
- **A-09.30 †** — Finding servers on the LAN. *Portico ahead on design; one Docker caveat to document (B-09.26).*
- **A-09.34** — Talking to the vendor's service. *Portico ahead of Plex on restraint. The networking contract still describes a 30-minute heartbeat that the code no longer has (Doc/code disagreements).*
- **A-09.46** — What a backup is. *Portico ahead for its design goals. Deliberate (Spec — Backups).*
- **A-09.47 †** — Scheduling, retention and location. *Portico ahead of Jellyfin on having a schedule at all; behind Navidrome and Audiobookshelf on being able to put backups somewhere else (B-09.18, F-09.31).*
- **A-09.55** — One server per state folder. *Portico ahead.*
- **A-09.59** — Telemetry and privacy posture. *Portico ahead of Plex and Navidrome, level with Jellyfin in spirit. A single "what this server contacts and why" page would turn this into something to advertise.*
- **A-18.30** — Networking. *Portico ahead for the common case. Reverse-proxy users will ask for a base path (F-18.42).*
- **A-21.5** — Hosted account authorization can succeed without a usable server. *Portico ahead. Deliberate independent-authority rule. Keep it; medium UX work for coherent empty/error states and device sign-in, with no need to copy competing server-coupled flow.*
- **A-22.20** — Remote reachability is not inferred from a certificate or router lease. *Portico ahead, deliberate. Keep it; medium to extend evidence to richer user-facing failure reasons without implying external proof covers every client's path.*
- **A-22.44** — Server-list checks are version probes, not presence polling. *Portico ahead on control-plane cost, deliberate Hosted-at-Scale. Small/medium to make explicit refresh and change signals reliable without adding heartbeat traffic.*
- **A-22.45** — Server-side Hosted retries persist and spread fleet recovery. *Portico ahead, deliberate. Keep it; medium to validate crash/restart and queue-drain ordering at scale.*
- **A-22.46** — Wakes are signed hints with durable acknowledgement tracking. *Portico ahead in distributed-work discipline, deliberate. Small for clearer docs; medium for missed wake/private-proxy fallback acceptance.*
- **A-23.1** — Optional Hosted identity versus Plex's normal central account. *Portico ahead for owner autonomy, deliberate (PC1). Preserve local lifecycle and clarify which conveniences need Hosted. Changes are small documentation/acceptance work unless the implementation actually violates it. Plex is ahead in recognition and cross-device setup polish; that would change the*
- **A-23.4** — Remote-payment enforcement versus infrastructure payment. *Portico ahead on simplicity and owner control, deliberate. Keep paid hosted services separate from local core capabilities; medium entitlement/service design if relay launches. Sustainability is unresolved: a generous free core still needs client-store, certificate, support and security fundin*
- **A-23.6** — Infuse's local combined library versus on-demand server mode. *Portico ahead for the intended live huge-library path; deliberate. Medium–large to add an explicit offline catalog subset or multiple-server unified view. Retain on-demand default. Offline travelers or households spanning several servers may prefer the other model.*
- **A-25.58** — Local telemetry versus automatic crash/installation reporting. *Portico ahead — For privacy/control, ahead. Small/medium: clear local-versus-Hosted data inventory, bounded user-reviewed support exports and crash-log instructions; any opt-in remote reporting would need explicit product approval.*

#### T02 — The household profile is the authority and privacy boundary (34)

- **A-01.1** — Accounts with profiles underneath, versus a flat list of users. *Portico ahead for household use; deliberate (PC/identity/contract.md §2.3). Jellyfin's model is simpler to reason about for "separate adults who each want a real login", which Portico covers with separate member accounts.*
- **A-01.10** — Restrictions compiled into catalogue SQL, with one predicate for every surface. *Portico ahead. Worth advertising. The weak spot is feature gating by URL prefix (A-01.12).*
- **A-01.16 †** — Three kinds of credential with different reach. *Portico ahead, with the caveat that in practice the apps throw the account session away after a profile is chosen, so account management happens through the primary profile's viewing session (direct_accounts.go:328-332).*
- **A-01.24** — Built-in TOTP second factor on the server itself. *Portico ahead.*
- **A-01.32** — Session history keeps the reason a session ended. *Portico ahead for diagnosing "why was I signed out". Revoked rows are never pruned (B-01.18).*
- **A-01.36** — A global account that proves identity, while each server owns membership. *Portico ahead. This is the strongest structural idea in the domain.*
- **A-01.37 †** — What breaks when things are down. *Portico ahead of Plex and close to Jellyfin for anything already signed in. The gap is a Portico Account member (not owner) wanting to add a device while Hosted is unreachable: they have no password on the server, so only a TV code from another device helps. Deliberate (PC/identity/contract.m*
- **A-01.40** — Invitations by code. *Portico ahead of Jellyfin.*
- **A-01.43** — Stream limits count streams, not signed-in devices. *Portico ahead; counting streams is what owners mean.*
- **A-01.53** — Avatars. *Portico ahead on privacy.*
- **A-02.28** — Capability URLs instead of tokens in query strings. *Portico ahead (a leaked URL dies with the session). Detailed review belongs to the security auditor.*
- **A-02.35** — Two histories and a privacy switch. *Portico ahead.*
- **A-04.26** — Personal versus shared subtitles, with history. *Portico ahead. No competitor has per-viewer subtitles.*
- **A-06.8** — A consent gate before the first provider call. *Portico ahead on privacy; costs one click. Deliberate (contract §13).*
- **A-06.36** — Image URLs and who may fetch them. *Portico ahead on privacy; watch the per-image database cost on a grid of 60 posters. A short-lived signed image token or a cookie scoped to the image path would allow plain <img> and browser-managed memory. Medium.*
- **A-06.50** — What leaves the server. *Portico ahead.*
- **A-07.46** — The owner's play history. *Portico ahead.*
- **A-10.38 †** — Two histories and maintained play counts; no scrobbling, no statistics. *Portico ahead on privacy design; behind on scrobbling and on showing people their listening (F-10.3, F-10.4).*
- **A-11.37 †** — Parental controls on live content. *Portico ahead in granularity, with one gap.*
- **A-11.47** — Library Channels respect profile restrictions. *Portico ahead.*
- **A-12.16** — Two histories: the viewer's and the owner's. *Portico ahead. Say plainly in the viewer's Privacy screen that the owner still sees plays.*
- **A-12.17** — Pause watch history (incognito). *Portico ahead. Consider a narrower variant: keep resume, skip History and taste.*
- **A-13.12** — A queue that not everyone may see. *Portico ahead. Two leaks undercut it (B-13.9).*
- **A-13.32 †** — How the TV gets a credential. *Portico ahead. Two caveats: the TV's token is a full viewer session with the pairing person's role, not the narrow receiver credential the contract describes (B-13.14), and the contract's encrypted proof-of-possession bootstrap is not what was built (D-13.3).*
- **A-13.34 †** — One controller, with a takeover prompt. *Portico ahead for shared homes. It is more code to get right on real devices (not yet physically tested, per Backlog).*
- **A-13.41** — How a server is shared. *Portico ahead of Jellyfin. Against Plex it lacks delivery and the review-before-send summary the contract describes (§3.1). Content restrictions and limits cannot be attached to the invitation; they are set on the member afterwards, so a new member briefly has unrestricted ratings (F-13.11).*
- **A-14.31 †** — Ownership on a shared device. *Portico ahead on isolation (and its tests encode a real past bug: one profile's renewal deleting another's files, :106-113). Deduplication is an optimisation the contract already permits (§3 rule 4).*
- **A-15.30** — Restrictions are part of the query, and every route is tested. *Portico ahead. This test is worth advertising.*
- **A-15.31** — Who the request is "as". *Portico ahead.*
- **A-18.24** — People: accounts with profiles, invitations and device approval against users. *Portico ahead structurally; missing schedule and tag allow-list (F-18.33).*
- **A-18.48** — The account site lives inside the app. *Portico ahead.*
- **A-19.41 †** — Shared devices: an on-device PIN vault, no system user awareness. *Portico ahead on PIN handling; behind on Apple TV multi-user (F-19.7).*
- **A-21.44** — Scoped system shelf feed versus upstream token/catalog fetch. *Portico ahead. Keep narrow feed, verify refresh/removal on profile changes. Medium system integration per OS, constrained by privacy rather than imitation.*
- **A-23.29 †** — Built-in history versus independent statistics service. *Portico ahead for common built-in ops, Unclear on reporting depth. Medium richer exports/anomaly/report queries, no need for another daemon. Prominent privacy wording is important. Users needing multi-server reports or notifications may still prefer an external tool.*

#### T03 — People choose policy and quality (17)

- **A-01.21** — Cool-off instead of lockout. *Portico ahead; deliberate ("a correct password always signs you in", 02 Server.md).*
- **A-01.30** — A local recovery owner whose password only works from a private network. *Portico ahead. The "private network" test here is a different function from the one used for remote-playback decisions (B-01.14).*
- **A-02.12** — Viewer delivery preferences with require and never. *Portico ahead in expressiveness; it is a power-user feature. Check the clients expose it; I did not trace the settings UI.*
- **A-02.13** — Refusal is a first-class outcome. *Portico ahead. Keep.*
- **A-02.31** — Stream limits and conversion caps. *Portico ahead. Note the tension with "no refusal because of server load": a conversion cap is an owner-set refusal, which is consistent with the rule as written.*
- **A-09.8 †** — FFmpeg is bundled, pinned and verified. *Portico ahead on integrity. One gap to note: the Docker image carries the FFmpeg binaries but no GPU user-space drivers (B-09.26).*
- **A-09.11** — Settings are a typed, revisioned registry. *Portico ahead. Deliberate (settings contract §2.2, §5.4). Three places where the registry and reality disagree are listed as B-09.9, B-09.10 and B-09.28.*
- **A-09.12 †** — What needs a restart. *Portico ahead on hot application; behind on being able to change the port or bind address without touching the service definition (F-09.8).*
- **A-09.17** — Who can reach an unconfigured server. *Portico ahead of Jellyfin, roughly level with Plex in direct deployments. Deliberate.*
- **A-09.19 †** — Getting back in when the owner is locked out. *Portico ahead. One small risk in B-09.32.*
- **A-10.31** — Advancing policy lives on the server and in native code. *Portico ahead; unusual and sensible for audiobooks. For music a passout check that stops an album may surprise people; worth confirming the default.*
- **A-12.58** — One registry, two scopes, server-stored. *Portico ahead.*
- **A-15.9** — Password rules. *Portico ahead. Note the Identity contract still specifies upper/lower/digit rules that the code deliberately does not enforce.*
- **A-15.12** — Second factor. *Portico ahead. See B-15.9 for how the no-lockout rule limits what the second factor buys.*
- **A-15.15** — Owner recovery. *Portico ahead.*
- **A-18.26** — Dashboard and server control. *Portico ahead on insight; missing restart (F-18.37).*
- **A-23.17 †** — Hardware/HDR capability is not a premium permission. *Portico ahead on access, Unclear on actual GPU/HDR breadth. Deliberate free model. Medium acceptance/error reporting, potentially large missing backend paths. Keep clear diagnosis rather than claiming all hardware is interchangeable.*

#### T04 — Make large libraries cheap by doing work before the page opens (60)

- **A-03.1** — Throttle by killing FFmpeg versus pausing it. *Trade-off (subjective), leaning Portico ahead. Deliberate (comment at hls.go:639-642). It only works because of the fixed grid (A-03.2), and it is on by default, which Jellyfin's is not. What would change my view: a measured restart latency on a Raspberry-Pi-class box above ~10 s.*
- **A-03.30 †** — Disk budgets. *Portico ahead in intent. The fixed per-session ceiling conflicts with the owner's own retention setting — B-03.8.*
- **A-03.33** — Concurrency limits and queueing. *Portico ahead. Note the count is of sessions, not running processes, so a paused or throttled viewer still holds a slot — reasonable, but worth knowing.*
- **A-03.47** — Consumer NVIDIA session limits. *Portico ahead in mechanism, but the breaker interaction means the fourth viewer on an older driver can switch hardware off for the first three. Fixing B-03.6 resolves it.*
- **A-03.50** — Process priority. *Portico ahead, and consistent with its "deprioritised, never paused" principle.*
- **A-05.33** — Scan tiers as promises about disk reads. *Portico ahead. Deliberate.*
- **A-05.47** — Hard links and the same file in two places. *Portico ahead — avoids duplicates when a download folder and a library folder share hard links, and avoids probing the same bytes twice.*
- **A-06.35 †** — Pure-Go image code with hard limits, versus a native library. *Portico ahead on safety, behind on speed. A small pool sized to CPU count (two to four) with the same pixel limits would help without giving up the bounds; and on-demand resizes for viewers should not wait behind background downloads.*
- **A-06.42 †** — Client image loading. *Scheduling Portico ahead of everything I read. Behind on a managed on-device disk cache for native (behaviour is whatever the platform defaults are, which I could not establish from source) and on placeholders.*
- **A-07.1** — Kinds as data with narrow side tables, versus one wide table with a type string. *Portico ahead. Deliberate (persistence/README.md rule 6). The cost is more joins when composing a detail page, which Portico pays by precomputing browse rows.*
- **A-07.12 †** — Derived tables kept by a queue and a worker, versus computing at query time. *Portico ahead for the stated 5–10 million target, provided the derived data can be verified and repaired in the field. Today the verifier for the counted blocks is a test-and-diagnostics function (CheckBrowseBlocks, derive_browse_blocks.go:55-72) and there is no owner-facing "check and rebui*
- **A-07.13** — Totals. *Portico ahead.*
- **A-07.14 †** — Reaching position N. *Portico ahead, with the caveat in A-07.12 about verification.*
- **A-07.17** — Write ordering: one gate with a priority ladder, versus selectable locking modes. *Portico ahead. One thing Jellyfin has that Portico does not: an escape hatch. If the gate misbehaves in the field there is no setting to change (A-07.21).*
- **A-07.20** — Write-ahead log growth. *Portico ahead, by a lot. This is the most carefully engineered part of the domain.*
- **A-07.23 †** — Planner statistics. *Portico ahead in intent; it is also the only hand-written reimplementation of ANALYZE, which is a maintenance liability if sampled numbers differ from true ones in a way that changes plans.*
- **A-07.30** — Filter model: a boolean tree, versus a flat list of fixed parameters. *Portico ahead in expressiveness and in being self-describing. Jellyfin covers more individual facts (A-07.31).*
- **A-07.34** — The alphabet rail. *Portico ahead. Edge: a library with many scripts publishes one anchor per distinct first letter, which for Chinese or Korean titles can be thousands of single-title "letters". I did not find a cap.*
- **A-07.36 †** — Search matching. *Portico ahead on engine, behind on what is indexed and on query preprocessing.*
- **A-07.38** — Search response shape. *Portico ahead.*
- **A-07.41** — Parental restrictions in queries. *Portico ahead on read cost; it stores one extra copy of the browse rows per distinct restriction policy per library, which is a storage and write cost that grows with the number of differently restricted profiles.*
- **A-07.47** — Guard rails in tests. *Portico ahead. I did not run them.*
- **A-08.19** — Admission control and back-pressure. *Portico ahead, by a wide margin. Keep, advertise, and publish the limits (F-08.14).*
- **A-09.33 †** — Overload protection is designed in. *Portico ahead in design. Deliberate.*
- **A-09.53 †** — Resource controls. *Portico ahead in approach. The numbers have not been validated on hardware (the brief says performance targets are not yet established).*
- **A-09.54 †** — When the disk fills. *Portico ahead in philosophy, with gaps in telling the owner.*
- **A-09.57** — Looking after the database. *Portico ahead. The scheduled integrity check is environment-only; it would be more discoverable as a setting. Portico has no vacuum, so a database that shrinks never gives space back (F-09.34).*
- **A-10.28** — The queue is a server resource. *Portico ahead for scale and multi-device; deliberate (thin clients). The consequences for shuffle are the next entry.*
- **A-11.31 †** — Guide API shape. *Portico ahead for the grid, behind for discovery (F-11.22).*
- **A-11.32** — Guide grid rendering. *Portico ahead.*
- **A-11.38** — Guide refresh scheduling. *Portico ahead.*
- **A-12.1 †** — Watched is computed from intents and container defaults, not stored per item. *Portico ahead on scale, deliberate (scale principle; tests S/catalog/bulk_jobs_test.go:149-199, 285-310). The precedence choice is the risk, not the mechanism.*
- **A-12.36** — The server composes Home; clients only draw it. *Portico ahead, deliberate (thin clients). Consider letting clients fetch below-the-fold rows lazily (the per-row endpoint exists).*
- **A-12.44** — Exact totals and resumable paging on every row. *Portico ahead.*
- **A-12.53 †** — Cost model. *Portico ahead at scale; unmeasured on weak hardware (see Evidence boundary).*
- **A-12.55 †** — Search engine: token-prefix full-text on one title. *Portico ahead on scale and ranking, behind on recall. See F-12.10.*
- **A-12.56 †** — Search groups and budgets. *Portico ahead on structure, behind on breadth.*
- **A-16.24** — List virtualisation versus pagination. *Portico ahead, clearly, for the stated 5–10 million item goal.*
- **A-16.25** — Letter jumps. *Portico ahead.*
- **A-16.28** — Image sizes and device pixel ratio. *Portico ahead for the "weak server, many viewers" target. A fourth small bucket (about 240) would be the only thing to consider, and only with measurements.*
- **A-17.37** — Trickplay tiles are fetched through the authorised artwork cache. *Portico ahead (no credential in URLs), at the cost of a fetch-to-blob per sheet.*
- **A-18.6** — A windowed endless grid against numbered pages. *Portico ahead on the grid. The rail's narrow availability is the one weak spot (F-18.10).*
- **A-18.16** — Bulk actions as server jobs. *Portico ahead.*
- **A-18.39** — Five ways to page. *Portico ahead where it uses the windowed grid; the Previous / Next lists (My List, Favorites, History) are the outliers and, for someone with a thousand favorites, the weakest lists in the app. Moving them onto the windowed collection is medium.*
- **A-19.27** — Random-access paging with placeholders. *Portico ahead. Two limits to know: B-19.13 (row-height assumption and number precision at millions of rows).*
- **A-21.20** — Browse capability vocabulary comes from server. *Portico ahead. Deliberate O(visible)/correctness principle. Medium guided operator/range UX; preserve query semantics and never load all items to imitate another client.*
- **A-21.23** — Alphabet and jump use bounded browser windows. *Portico ahead. Keep server-authoritative jump; small per-form-factor affordance polish. It would be worse to copy an in-memory whole-list alpha index.*
- **A-21.29** — Playlist mutations can be windowed, but playback loses occurrence identity. *Portico ahead. Keep bounded mutations; small/medium B-21.8 repair. Competitor whole-load gating is a precedent to avoid, not an argument against native editing.*
- **A-22.30 †** — Browser and native response memory boundaries differ. *Portico ahead on specialized/native bounds, behind on shared consistency. Medium to generalize bounded reads, preserving large permitted GET documents.*
- **A-22.34 †** — Server overload is typed and resource-classed. *Portico ahead, deliberate scale principles. Medium to align client priorities and prove physical capacity; static structure is not a 200-browser performance result.*
- **A-25.13** — Pure-Go SQLite enables cross-compilation. *Portico ahead — For portable deployment/build simplicity, ahead; not a proven speed advantage. Preserve unless profiling establishes a compelling alternative. Switching SQLite drivers is large and touches cancellation, custom functions, backup, diagnostics and tests.*
- **A-25.19** — Product-specific guards versus general analyzers. *Portico ahead — Ahead on domain invariants; not uniformly ahead on generic hygiene. Medium: keep domain guards, add cheap targeted general checks and make AST guards complete. B-25.13/14 limit what current guards prove.*
- **A-25.22** — Scale is encoded in test design. *Portico ahead — Deliberate and valuable. Medium: publish reproducible workload/hardware/result evidence and make release-tier invocation explicit. Goals and synthetic proofs remain distinct from household and concurrency acceptance.*
- **L-24.1 †** — Centralize persistence before shipping years of user data. *Portico ahead in stated architecture; runtime unproven. Keep the boundary and require changes to use it. Small governance cost now; large retrofit later. I would revise this if significant direct writers bypass the canonical fact API.*
- **L-24.2** — A connection pool is not a concurrency policy. *Portico ahead on explicit policy. Preserve measured transaction durations and leak diagnostics. Medium acceptance work on slow disks, LXC/ZFS and overloaded containers. A stalled holder still blocks everybody; serialization cannot repair hostile storage latency.*
- **L-24.3** — Bounded pages must reach all the way down to storage. *Portico ahead. Keep normalized membership and occurrence IDs; do not regress into JSON arrays for convenience. Small ongoing review requirement. Remaining container grids and exact counts deserve independent scale measurement; normalized storage does not automatically make every query indexed.*
- **L-24.6 †** — Startup work needs progress and a deliberate operator path. *Portico ahead in intent / unclear in future execution. Medium work to define resumable backfills and startup/readiness states. Never advertise “upgrades in seconds” without including index creation and failure recovery. A displayed building state is more honest than appearing dead.*
- **L-24.17** — Resource preparation should be optional, incremental and visible. *Portico ahead in capability. Medium operational budgets, cancellation/restart and progress reporting. Keep quality under owner control; a low-resolution preview is a separate artifact policy, not permission to lower a playback request silently.*
- **L-24.33 †** — Maintenance scheduling must preserve active playback. *Portico ahead in design. Medium idle/loaded acceptance. Track foreground tail latency, starvation prevention and resource recovery rather than aggregate scan speed alone. Weak devices still need eventual background completion; “deprioritized” requires evidence it does not become effectively paused*
- **L-24.37 †** — Demand can regress when an implementation violates query budgets. *Portico ahead in the intended substrate / unknown demand completion. Medium work to index and expose the intended semantics rather than joining expansive metadata on every search. This is unusually direct evidence that satisfying a feature request once does not mean it can safely remain shipped. A*

#### T05 — Every action has an identity, a revision, and a recovery story (92)

- **A-01.14** — Short access token plus rotating refresh token, bound to a device, versus one permanent token. *Portico ahead on security, at a complexity cost Jellyfin never pays. Deliberate (PC/identity/contract.md §5).*
- **A-01.15 †** — Tokens stored as digests; Jellyfin stores them in plain text. *Portico ahead. One loose end: the plain receipts are meant to be cleared after five minutes, and that clean-up can stop running (B-01.5).*
- **A-01.18** — Idempotent refresh with a stored answer. *Portico ahead. This is the piece that makes rotation survivable on flaky mobile networks.*
- **A-01.20** — Signing out a family from any of its old tokens. *Portico ahead (a correct consequence of rotation).*
- **A-01.28** — TV code: durable, eight characters, approval on the server's own page. *Portico ahead. Two edge cases break it (B-01.5, B-01.6). There is no owner switch to turn TV codes off (F-01.15).*
- **A-01.51** — Administration writes carry a revision and an operation id. *Portico ahead.*
- **A-02.8 †** — Four named routes with a recorded reason for every closed route. *Portico ahead underneath, behind at the surface. Small: map the trace's rejection codes into decision.reasons.*
- **A-02.23** — The server remembers a failed route. *Portico ahead, with a refinement to consider: count failures per (device, codec/container shape) and tell the owner "this device keeps failing HEVC direct play".*
- **A-02.24** — A playback session is its own resource. *Portico ahead. Deliberate (spec §2, §16). The cost is protocol weight for third-party clients: keys, revisions, generations, 202 replays.*
- **A-02.26** — Lease and cleanup timing. *Portico ahead, deliberate (spec §5.8).*
- **A-02.29** — A plan is pinned to the exact file. *Portico ahead for correctness; it means replacing a file mid-play stops playback, which is the right failure.*
- **A-02.36** — Cross-device ordering of progress writers. *Portico ahead. It is intricate; a short written explanation of the rules would help future work.*
- **A-02.47 †** — Multi-part titles. *Portico ahead on paper. I did not verify that clients move from part to part seamlessly or that resume across parts is stored (resume.partIndex is always 0 in options.go:525-527).*
- **A-03.3** — Copied video: keyframe "at or before" the grid line, cut by the segment muxer. *Portico ahead on correctness. The cost is in A-03.4.*
- **A-03.26** — Restart rule on seek. *Portico ahead on robustness (budget, shared waits, abandoned-conversion reclaim). The 12 s versus 24 s threshold is a tuning choice.*
- **A-03.28 †** — Live output through an HTTP PUT gateway. *Portico ahead — sharing one conversion across viewers is the right design for live.*
- **A-03.32** — Idle and end-of-session handling. *Portico ahead — more precise, and the orphan ledger and parent-death signal cover crashes Jellyfin leaves to luck.*
- **A-03.35** — Hardware failure handling. *Portico ahead — invisible fallback is what a normal person wants. The breaker cannot tell a bad GPU from a bad file (B-03.6).*
- **A-03.44** — Seek position for a copied stream. *Portico ahead — exact rather than "most videos". Jellyfin's end-of-file clamp is worth copying as a guard: Portico refuses out-of-range segment numbers, which covers the same case differently.*
- **A-04.17 †** — Timing offset is server state. *Portico ahead on persistence, behind on immediacy. Small: apply the live offset locally for overlay rendering and send it on a debounce.*
- **A-04.35** — Skips are reported to the server. *Portico ahead — this is the raw material for measuring marker quality.*
- **A-05.1 †** — A durable, paged state machine instead of an in-process walk. *Portico ahead on robustness by design; deliberate (contract.md §3). The cost side is the subject of A-05.3 and B-05.1.*
- **A-05.10** — Protecting the library when a drive disappears. *Portico ahead on safety; deliberate (contract.md §10).*
- **A-05.38 †** — Reporting progress and problems. *Portico ahead, with that one gap.*
- **A-05.39** — Deleting media from the UI. *Portico ahead on safety. One trap: the trash lives in the server's state folder, so deleting from another volume copies the whole file there (B-05.25).*
- **A-05.45** — Re-reading old problems when the reader improves. *Portico ahead — a neat mechanism, and exactly what will be needed as the grammar grows. Note it covers only files with issues; files the old reader accepted wrongly are not revisited.*
- **A-05.46** — A numbered change feed. *Portico ahead in durability. Nothing prunes it (B-05.20).*
- **A-06.21** — Optimistic concurrency on edits. *Portico ahead.*
- **A-07.18** — One snapshot per request. *Portico ahead.*
- **A-07.19** — Durability. *Portico ahead (per-class durability is unusual and well reasoned). Note for completeness: with NORMAL, the last few commits before a power cut can be lost, including a "watched" mark; neither product treats that as a problem.*
- **A-07.43** — Backup of the database. *Portico ahead on correctness (a plain file copy of a live WAL database is only safe if nothing writes in between). See B-07.13 for the cost of the pinned snapshot on a large, busy database.*
- **A-08.9 †** — Error model. *Portico ahead of Jellyfin, behind its own Style Guide. The single envelope is the documented target. The migration rides on A-08.3. In the meantime a one-page table of "which family uses which envelope" in the API guide would save outsiders real time (the guide currently says "every JSON error is*
- **A-08.10** — Paging: signed, revision-fenced cursors versus offsets. *Portico ahead for first-party UX and scale; Trade-off for integrators. Deliberate ("Correctness over limits", "Scale"). A snapshot-tolerant export route (keyset order by id, no revision fence, documented as "may miss concurrent changes") would serve sync tools. Small to medium.*
- **A-08.11** — Optimistic concurrency. *Portico ahead. Note the two spellings (expectedRevision in the body with 409 _conflict; If-Match header with 412 revision_mismatch) — one more thing an outside developer has to learn per family.*
- **A-08.12 †** — Idempotency. *Portico ahead, with the same "many dialects" caveat.*
- **A-08.23** — Durable, resumable event log versus ephemeral push. *Portico ahead. Two edge cases in the current implementation are listed as B-08.4 and B-08.5.*
- **A-08.26** — Keep-alive and stream lifetime. *Portico ahead. Small tidy-ups: use one heartbeat interval across the three streams (15 s is the safest), and state it in the guide (06-events.md currently says heartbeat interval is "not specified").*
- **A-08.46** — Slow or stalled stream consumers. *Portico ahead: nothing is lost because the log is durable.*
- **A-09.9** — Release integrity: strong provenance, unsigned installers. *Portico ahead on provenance, level on signing. Deliberate and documented ("signing … before unattended updates").*
- **A-09.37** — Activity, history and audit. *Portico ahead. A tamper-evident audit trail and built-in play history are both beyond stock Jellyfin.*
- **A-09.48 †** — Restore. *Portico ahead, by a wide margin, in design. Three implementation problems undercut it today (B-09.1, B-09.2, B-09.16).*
- **A-09.50 †** — Schema migrations. *Portico ahead on migration discipline, behind on a guaranteed way back (F-09.3). A pre-migration backup is small: the backup service already has a synchronous Create.*
- **A-09.56** — Crash containment. *Portico ahead.*
- **A-09.58 †** — Deleting media. *Portico ahead in intent, with a design risk.*
- **A-10.24 †** — A protocol for the next track (prepare, then commit). *Portico ahead on accounting accuracy; a complexity risk worth runtime testing (see Evidence boundary).*
- **A-10.41** — Progress is "which file, and where", ordered by intent. *Portico ahead on multi-device correctness. The spec's "position is book-relative (positionMs across parts)" (§12) is not how it is stored, which is fine, but it means "X hours left in the book" has to be computed.*
- **A-11.3** — Server-side timeshift buffer per session. *Portico ahead of Jellyfin, level with Plex. The depth and size are configuration values with no owner control I could find (see F-11.24).*
- **A-11.5** — Guide stored as full-copy generations. *Trade-off (subjective), leaning Portico ahead for correctness. Skipping the publish when the fetched bytes are identical to the last ones (a hash compare) would remove most of the cost and is small. It would also avoid B-11.1 in the common case.*
- **A-11.16** — Durable tuner allocations with reservations. *Portico ahead in rigour. The machinery is sound; what it counts (sessions rather than channels, A-11.1) is the weakness.*
- **A-11.19** — Scheduling engine. *Portico ahead.*
- **A-11.24** — Honest completeness accounting. *Portico ahead. One calibration issue in B-11.20.*
- **A-11.28** — Retention and disk guards. *Portico ahead.*
- **A-11.43 †** — Live session failure handling. *Portico ahead in design, with a counter bug.*
- **A-12.14** — One canonical writer per item, with fences, instead of last write wins. *Portico ahead, deliberate (contract §4.3). Completion should probably be accepted from any session.*
- **A-12.15** — Explicit changes are idempotent, revision-checked and have an offline-conflict protocol. *Portico ahead on correctness, deliberate (P15). The container flags skip all of it (last write wins, container_personal.go:16-18), which shows the lighter form is acceptable for booleans; consider it for item-level My List and Favorite too.*
- **A-13.9** — Groups are stored; they survive a restart. *Portico ahead on durability. The write volume is the thing to trim.*
- **A-13.13 †** — Concurrency control on commands. *Portico ahead in intent, behind in effect until B-13.1 is fixed. The fix is to fence commands on playbackRevision (which exists in the schema and the snapshot but is never incremented) rather than on the catch-all revision.*
- **A-13.29** — Two-phase handoff. *Portico ahead. Note the TV must be audibly playing before the phone stops, so both play for a second or two; the contract instead asks for the TV to prepare "paused and unpresented" (doc/code disagreement D-13.4).*
- **A-15.6** — Session model. *Portico ahead. The price is client complexity (refresh races, replay receipts), which the Backlog's "two Apple sign-ins ended without an obvious cause" item shows is not free.*
- **A-15.13** — TV-code sign-in. *Portico ahead. Both share the inherent weakness of the pattern: whoever gets you to approve their code becomes you. On Portico an owner's approval yields an owner session (B-15.13).*
- **A-15.34 †** — Trust root and key rotation. *Portico ahead of anything comparable I can see. The gap is how fast a server that Hosted cannot reach learns of a revocation (B-15.12).*
- **A-15.39** — Backups. *Portico ahead on attack surface (no upload, no archive parsing). The contents are everything needed to become the server and its users (B-15.5).*
- **A-15.48 †** — Build and release integrity. *Portico ahead on pipeline hygiene, behind on signed installers. Loose ends in B-15.19.*
- **A-16.26 †** — Scroll position on Back. *Portico ahead in design, probably behind in effect until B-16.1 is checked.*
- **A-16.42** — Error presentation. *Portico ahead.*
- **A-16.43 †** — "Server unreachable" UX. *Portico ahead, with one risk.*
- **A-16.57** — Client-visible concurrency control. *Portico ahead.*
- **A-17.6** — Seeking is always a local seek; the server follows segment requests. *Portico ahead. Note the spec still describes a server seek by PATCH for conversions (see Doc/code disagreements).*
- **A-17.11 †** — Quiet reconnection instead of immediate failure. *Portico ahead in intent. The mechanism needs the fixes in B-17.4.*
- **A-17.12** — Engine failures are reported to the server, which remembers them. *Portico ahead. Consider generalising a failure to the codec/profile/container combination for that device after it repeats on two or three files.*
- **A-17.16 †** — Subtitle timing offset is stored on the server. *Portico ahead on persistence, behind on fineness and feel. Small: apply locally at once, save after the viewer stops adjusting, allow 0.1-second steps.*
- **A-17.38** — Media URLs never carry cookies and never leave their origin. *Portico ahead.*
- **A-18.22** — Delete. *Portico ahead for single delete; see B-18.13 for bulk.*
- **A-19.15 †** — Hand-written secure storage with "never repair". *Portico ahead on rigour, with one gap: there is no recovery path. Plezy's existence of prefs_recovery.dart and tvos_database_recovery_store.dart shows that field corruption is common enough to need one.*
- **A-20.3** — A leased, revisioned command-and-fact protocol between JavaScript and the engine. *Portico ahead. Worth keeping and advertising to third-party client authors as the reference adapter pattern. If a second engine is ever added, this protocol is the natural seam: an mpv adapter would implement the same commands and facts.*
- **A-20.7 †** — A wrong capability claim is corrected by a remembered failure, not by the viewer. *Portico ahead, with caveats. Worth adding: an owner-visible list of remembered route failures with a clear button, and a per-device "this codec failed N different files" signal. Small.*
- **A-22.8** — LAN gets a short head start, not a full serial timeout. *Portico ahead over sequential fallback; similar to Jellyfin web but with stronger cancellation. Deliberate. Small to tune after physical-network measurements; do not change constants from intuition alone.*
- **A-22.13** — Platform network changes are hints, not proof of server loss. *Portico ahead relative to a global internet gate, and broadly aligned with Plezy. Deliberate contract §11. Small to improve explicit path-state messages; runtime permission/VPN cases remain unverified.*
- **A-22.25** — Refresh has a persistent request ID and restart-safe receipt. *Portico ahead for its chosen auth model, deliberate. Keep it and remove stale five-minute comments. Small for documentation, medium for storage/coordination failure fixtures.*
- **A-22.26** — Refusal is distinct from transient failure. *Portico ahead in the intended path. Keep it; B-22.4–5 show exceptions elsewhere. Small/medium to make classification consistent rather than copied at every bridge.*
- **A-22.27 †** — Single-flight is layered across runtime and storage. *Portico ahead in secure contexts, behind on ordinary HTTP multi-tab fallback. Medium to supply true cross-tab coordination or explicitly isolate credentials by tab.*
- **A-22.31** — Mutation recovery belongs to the operation owner. *Portico ahead in explicit receipt-oriented design, deliberate. Small/medium to audit hidden platform retry and ensure every replayable path retains its key.*
- **A-22.40** — Online report facts are ordered and bounded. *Portico ahead on live sequence discipline. Deliberate October 9 work. Medium to clarify crash durability and user experience at a full fact queue.*
- **A-22.53** — TV activation is a durable authorization operation. *Portico ahead in explicit operation lifecycle, deliberate onboarding contract. Keep it; medium to coordinate failure backoff with HostedGate and verify platform grant installation/storage boundaries.*
- **A-23.20 †** — Watch Together survives a redesign only if it remains a contract. *Portico ahead in preserving intent, Unclear on device reliability. Medium cross-platform acceptance. Keep it if supported without a cloud media-history store. Actual drift, reconnection and platform coverage would change this verdict.*
- **A-25.7** — Narrow callbacks fence cross-domain authority. *Portico ahead — For transactional authorization this is a useful narrow seam. Keep it; small/medium improvement is documenting callback preconditions, rollback/after-commit behavior and cancellation, with seam tests. Reconsider if callbacks start serving as a hidden general event bus.*
- **A-25.21 †** — Real database and contract tests are first-class. *Portico ahead — Strong foundation worth preserving. Medium: maintain a semantic API conformance corpus and separate cheap domain tests from broad composition tests. Do not use the test-line ratio as release qualification.*
- **L-24.7 †** — Backup portability and downgradeability are separate features. *Portico ahead in inspectability, unknown portability acceptance. Medium restore drills across path/OS changes and server versions. A copyable file format does not prove every external media root, certificate, state reference or native-client record migrates. Define the supported restore matrix.*
- **L-24.15** — Subtitle choice is a resource and timing operation. *Portico ahead in protocol shape. Medium fixture/device matrix. Test external text, ASS fonts, PGS, negative offsets, fullscreen/PiP and quality changes. A good-looking subtitle settings sheet does not establish synchronization.*
- **L-24.18** — Quick sign-in has to include real televisions and weak networks. *Portico ahead on intended low-ceremony flow. Medium failure/retry acceptance. Test expired/approved codes, server restarts and no Internet. Avoid equating Hosted-account failure with invalid local credentials.*
- **L-24.22** — Social playback needs reconnect identity and an expiry model. *Portico ahead in authority isolation. Medium reconnect/timing acceptance. Retain the separation from Hosted and remote-media relay. A short room code should never become a long-lived recovery credential.*
- **L-24.25** — Authentication cleanup is an ecosystem migration. *Portico ahead at a clean starting point. Small written sunset rules, medium compatibility fixtures. Provide advance diagnostics for obsolete clients and a security exception path only if Justin intends one. A route alias is not a permanent promise unless published as such.*

#### T06 — Contain untrusted data and external work (52)

- **A-01.46** — Credential storage on clients. *Portico ahead.*
- **A-01.47** — Clients pin the server's identity key. *Portico ahead, especially because plain HTTP is first-class.*
- **A-03.13 †** — Sandbox versus none. *Portico ahead, deliberate (04 Playback and Media.md, "Decoder confinement"). Keep; fix the GPU gaps.*
- **A-03.45 †** — Remote sources reach FFmpeg through a private bridge. *Portico ahead on safety, behind on completeness until B-03.5 is fixed.*
- **A-04.39** — Trickplay delivery. *Portico ahead on formats (the WebVTT thumbnail track is a de-facto standard many web players read). Caching is weaker than it could be (B-04.30).*
- **A-04.44** — Confined decoders for every subtitle and analysis job. *Portico ahead.*
- **A-05.37** — .strm descriptors. *Portico ahead on privacy and safety. Deliberate.*
- **A-08.8 †** — No null in responses, tolerant readers. *Portico ahead as a contract rule. The run-time enforcement is a risk worth softening.*
- **A-08.14** — Auth header design. *Portico ahead.*
- **A-08.16** — How authorization is expressed. *Portico ahead on strength. For auditability, the registry label should be the thing that is enforced (so the generated contract's x-access is a fact, not a comment).*
- **A-08.17** — Media URL authority. *Portico ahead. (Detail belongs to the playback audit.)*
- **A-08.20** — Sign-in throttling. *Portico ahead.*
- **A-08.44** — Security headers on API responses. *Portico ahead. One side effect to keep in mind: the fixed connect-src list in the CSP names Portico's own hosts, so an owner-hosted copy of the web app on another origin needs both PORTICO_ALLOWED_ORIGINS and a CSP that the server does not let them change.*
- **A-08.45** — Unknown routes and methods. *Portico ahead (small, but it is the kind of thing API consumers notice).*
- **A-11.14** — Hostile-input posture. *Portico ahead. Worth advertising. The cost is the strictness discussed in A-11.6; the security goal does not require rejecting whole documents.*
- **A-15.1** — How a route becomes authenticated. *Portico ahead in outcome, level in structure. I opened every raw handler the script could not classify and found none missing a check. But nothing in the test suite asserts "every route answers 401 to an anonymous caller unless it is on a named public list". The restriction matrix test already wal*
- **A-15.2** — Size of the unauthenticated surface. *Portico ahead. Keep it that way.*
- **A-15.3** — Images need a credential. *Portico ahead on security; a real performance trade-off. Deliberate.*
- **A-15.4** — Media URLs are per-playback capabilities. *Portico ahead.*
- **A-15.5** — Credentials in URLs. *Portico ahead. The contract's absolute wording ("secrets never appear in URLs") is not literally true; see Doc/code disagreements.*
- **A-15.11** — Username enumeration. *Portico ahead. One small leak remains in remembered-accounts (B-15.14).*
- **A-15.16 †** — What "local network" is trusted for. *Portico ahead. The residual risk is B-15.11.*
- **A-15.18** — Security response headers. *Portico ahead. Small tightening available (B-15.16).*
- **A-15.19 †** — Cross-origin requests. *Portico ahead. The built-in trust of https://web.getportico.tv is the mechanism behind A-15.33.*
- **A-15.21** — HTML injection sinks. *Portico ahead. Worth a lint rule so it stays true (F-15.16).*
- **A-15.22 †** — Running FFmpeg. *Portico ahead, and this is the single largest security difference. Caveat: where the sandbox is unavailable (Docker default profiles, Windows) the baseline is closer to the others, and the Dockerfile says so honestly (Dockerfile:22-28).*
- **A-15.23** — Building FFmpeg arguments. *Portico ahead. I searched every filter and protocol site and read the main builders; I did not read every line of every builder.*
- **A-15.24 †** — No owner-to-code-execution features. *Portico ahead. Not verified for rclone remote configuration (internal/remotesources, internal/mounts), where option strings can be dangerous; that deserves its own look.*
- **A-15.25** — Server-side request forgery. *Portico ahead. One owner-only exception: the release feed URL uses a plain client (B-15.17).*
- **A-15.26** — Image handling. *Portico ahead. The cost is no SVG channel logos.*
- **A-15.27** — XML and archives. *Portico ahead. (Whether skipping embedded fonts hurts subtitle rendering is domain 04's question.)*
- **A-15.28** — Paths that come from requests. *Portico ahead.*
- **A-15.29** — Server-side folder picker. *Portico ahead. Small ordering nit in B-15.18.*
- **A-15.35** — TLS. *Portico ahead of Jellyfin, level with Plex.*
- **A-15.37** — Server identity pinning. *Portico ahead, with the limits stated.*
- **A-15.40** — Logging discipline. *Portico ahead.*
- **A-15.41 †** — Rate limiting and overload. *Portico ahead, with the bugs in B-15.3 and B-15.6.*
- **A-15.42** — Error messages. *Portico ahead.*
- **A-15.43** — Hostile-input testing. *Portico ahead. The Backlog already notes the rotation weakness.*
- **A-15.45** — Native credential storage. *Portico ahead.*
- **A-15.46** — Native transport rules. *Portico ahead. The Android config is broader than the contract's "HTTP only to local addresses"; the restriction there lives in JavaScript address normalisation, which I did not trace.*
- **A-15.47** — Deep links, WebViews, exported components. *Portico ahead.*
- **A-15.50 †** — Network discovery protocols. *Portico ahead for now. When DLNA arrives it brings unauthenticated media by design; the Backlog's own note ("exposes the whole server when on") is the right warning to keep.*
- **A-16.23** — Plain-HTTP engineering. *Portico ahead in care; it is a real cost of the "HTTP stays first-class" principle and worth knowing that the cross-tab guarantees only exist on HTTPS and localhost.*
- **A-16.58** — Content-Security-Policy posture and its side effects. *Portico ahead (auditor 15 owns the security view).*
- **A-19.16** — All API traffic goes through a bounded native transport. *Portico ahead for safety; watch the ceilings as libraries grow (a Home document or a guide window that passes 8 MB fails outright rather than degrading).*
- **A-19.30** — Image authorisation by header. *Portico ahead. One consequence: every token renewal changes every image's request headers, so each mounted card re-renders (harmless, but work on a weak device).*
- **A-22.3** — Discovery is an identity hint, never a credential destination by itself. *Portico ahead, deliberate. Preserve it; small to expose better user explanation, medium to close ongoing socket/restore bypasses described below.*
- **A-22.4** — Persistent server identity is key-derived. *Portico ahead for remembered-route authentication. Deliberate; replacing it would be large and undesirable without an equally strong trust model.*
- **A-22.6** — Signed account-scoped route documents. *Portico ahead on evidence structure, deliberate; B-22.1 demonstrates why complete wire-boundary fixtures matter. Medium to improve compatibility and canonicalization.*
- **A-22.51** — Discovery responder recovery and hard bounds are unusually explicit. *Portico ahead on bounds/recovery; Trade-off on custom protocol maintenance. Medium to add standards/network fixtures; switching to a library is not automatically better.*
- **A-25.34** — Container engineering has good defaults, separate release weakness. *Portico ahead — Good defensive default container design. Small/medium: fix bundle input per target, document UID/GID/mount behavior and publish image size/digest. Do not weaken init/nonroot to simplify packaging.*

#### T07 — Keep evidence; ask the owner when identity is uncertain (23)

- **A-04.24** — Track identity. *Portico ahead. Worth keeping.*
- **A-04.31 †** — Marker model. *Portico ahead in design. It is the right shape for mixing detectors of different quality.*
- **A-05.7 †** — File identity: device + inode + size + time. *Portico ahead for same-disk renames, behind for cross-disk moves and replacements. Deliberate (contract.md §1.3). A content fingerprint (first/last megabyte + size, as Plex and OpenSubtitles do) or tag ids for music would cover the gap (medium).*
- **A-05.20** — Multi-episode files. *Portico ahead in model. Worth checking with auditor 03/04 what "watched" means when two episodes share one file.*
- **A-05.32 †** — Audiobooks. *Portico ahead of Jellyfin, behind Audiobookshelf on untagged collections (no author/series from folders). Deliberate ("Books and audiobooks: embedded tags and local files", 03 Catalogue and Metadata.md).*
- **A-06.5** — A durable evidence model, versus overwrite in place. *Portico ahead. Keep.*
- **A-06.15 †** — Episode order is an explicit, per-show choice. *Portico ahead in rigour. Not yet a library-level default (Backlog: "Per-show settings not built").*
- **A-06.20** — Locks, history and undo. *Portico ahead. Worth advertising.*
- **A-06.38 †** — One image per role, chosen through a candidate/selection model. *Model Portico ahead (fenced, undoable, auditable); breadth Portico behind (no multiple backdrops, no delete-to-none, no language filter).*
- **A-06.44 †** — Music. *Identity handling Portico ahead (fingerprints, typed credits, no silent overwrite). Artist images and biographies Portico behind in coverage: Commons has photos for far fewer artists than Deezer/Last.fm/AudioDB/Fanart, and only English text.*
- **A-06.49** — Provider responses are validated hard. *Portico ahead on safety; the "fail the whole record" choice for list sizes is a latent trap where truncating would be kinder (B-06.17).*
- **A-06.53** — "Local metadata only" is a library-level agent. *Portico ahead (clearer promise). Backlog notes the web add-library form failed with local-only + en on 25 Sep.*
- **A-07.2** — Integer ids inside, random public ids outside, versus a GUID computed from the path. *Portico ahead on size and on not exposing structure. Deliberate (schema header comment, "Direct Design §4"). See A-07.4 and B-07.3 for the places where the ledger's promise is weaker than it reads.*
- **A-07.3** — Files are separate from titles. *Portico ahead in the data model. (Whether the scanner actually groups versions is another matter: Backlog already lists "a second file of the same title becomes a second title".)*
- **A-07.7 †** — Genres, studios, tags. *Portico ahead on provenance; Unclear on redundancy. I did not trace whether the text columns and the edges can disagree.*
- **A-07.8 †** — People. *Portico ahead in model. Two gaps: nothing ever deletes a person nobody is credited as any more (B-07.15), and the browse filters match people by credited name text rather than by person (A-07.32).*
- **A-10.9** — Evidence with provenance, and a three-way local/online policy. *Portico ahead in principle. Audiobookshelf's ordered, owner-editable precedence is easier for an owner to understand than three modes; worth a look when the audiobook agent is revisited.*
- **A-18.18 †** — The metadata editor. *Portico ahead on design, behind on field coverage (F-18.27).*
- **A-23.8 †** — Local NFO evidence versus provider-owned descriptions. *Portico ahead over verified Plex baseline, Unclear on ecosystem completeness. Medium export/interoperability fixtures, not another parser from scratch. Keep bounds and provenance. Source-level subset compatibility needs explicit documentation; adoption experience would change the verdict.*
- **L-24.8** — Inventory should survive metadata failure. *Portico ahead. Preserve provisional visibility and independent failure reporting. Medium operational UI work if users cannot tell “file seen” from “metadata matched” from “analysis done.” Adoption should not wait on a provider outage before an owner can play a known file.*
- **L-24.9** — A disconnected mount is not an empty collection. *Portico ahead by reading. Keep reversible missing/trash separate from deleting logical history. Medium failure-injection acceptance across NFS/SMB/rclone/remounts. Would revise if a legacy cleanup route bypasses these protections.*
- **L-24.14** — Capability evidence must describe the actual route. *Portico ahead in traceability / exposed operationally. Medium profiles, large hardware coverage. Store the effective decoder and sink path, not just “device supports HEVC.” A receiver, OS change or subtitle mode can change the admissible tuple.*
- **L-24.40 †** — A skip-marker API is not high-quality automatic intro detection. *Portico ahead on conservative safety, behind on this detector capability. Large bounded season-comparison/indexing work if wanted; do not increase confidence numbers without new measurement. A user may reasonably expect automatic introductions to be found without editing each file, but the present*

#### T08 — Compose once on the server and share client behaviour (30)

- **A-02.1** — One server-made decision versus a menu the client chooses from. *Portico ahead for consistency. Deliberate (04 Playback and Media.md opening paragraph; "Thin clients" principle). What would change my view: evidence that device quirks need per-user overrides the server cannot know (see F-02.14).*
- **A-02.25** — One timeline call versus start / progress / stopped / ping. *Portico ahead.*
- **A-02.45** — Next episode. *Portico ahead (same behaviour on every client).*
- **A-02.46** — Live TV and VOD share the session resource. *Portico ahead in uniformity. Not reviewed in depth here.*
- **A-06.22 †** — The editor's field list comes from the server and is narrower. *Server-published schema is Portico ahead (new fields need no client release). Coverage is Portico behind (F-06.28).*
- **A-08.38** — Server-supplied strings. *Portico ahead for multi-user servers with mixed languages (the server never has to know the viewer's language).*
- **A-13.19** — Client coverage. *Portico ahead of Jellyfin, level with Plezy. The native build has no speed nudge (RN/app/together.tsx:44), so it leans entirely on seeks (B-13.3).*
- **A-16.1** — One app versus three apps in a long migration. *Portico ahead. Nothing to adopt. The lesson from Jellyfin is the cost of ever letting a second way of building screens in.*
- **A-16.8** — Server-authored documents versus client-composed screens. *Portico ahead for a first-party, multi-platform product. Keep watching for client-side decisions creeping back in; one small example is web/src/screens/home/Home.tsx:41, which decides "this row continues by date added" by testing the row's id against /recent|added/i.*
- **A-16.9** — Authored presentation structure in the shared core. *Portico ahead. This is the most distinctive architectural asset in my domain.*
- **A-16.15 †** — Session and token handling in the browser. *Portico ahead on security design, behind on simplicity. The 1,086-line SessionProvider with one context value is the riskiest single file in the web app to change; splitting sign-in flows (direct, Portico Account, restore) into separate modules behind the same context would be a medium refac*
- **A-16.50** — Where preferences live. *Portico ahead.*
- **A-16.56** — One codebase, two builds (bundled and Hosted-only). *Portico ahead in capability; treat the Hosted build as the compatibility canary.*
- **A-18.1** — Who decides what a screen contains. *Portico ahead for consistency and scale; deliberate (06 Clients.md). The trade-off shows up as the long missing-features list below: Jellyfin's breadth comes from each page being free to query whatever it likes.*
- **A-18.2** — One Settings screen against a separate admin console. *Portico ahead on findability (search over settings is something Jellyfin lacks). Deliberate (Justin, 2 Oct 2026). Watch the page count: the Server heading is already eleven pages, not the ten the docs say (D-18.1).*
- **A-18.7 †** — Filters built from the server's vocabulary against a fixed panel. *Portico ahead of Jellyfin, behind Plex. Deliberate ("never two number boxes", browse-toolbar.ts:150-153). A custom-range row at the end of each preset list would close most of the gap without changing the model (small, client-core plus each client).*
- **A-19.8 †** — Vega as a shim over the same screens. *Portico ahead on reach, unclear on quality until a physical Fire TV has been used. Keeping it a separate app with overrides, rather than Platform.OS === 'kepler' branches in shared files, is the right shape.*
- **A-19.9** — Shared packages by copy and checksum. *Portico ahead on what is shared; the mechanism is a trade-off. Deliberate (01 Overview.md). A git subtree or a single workspace would remove the two-commit dance; not urgent.*
- **A-19.17** — Navigation: seven kept-alive stacks under a custom navigator. *Portico ahead on feel; needs a bound for weak devices.*
- **A-19.33** — A design system that is enforced, and shared with the web. *Portico ahead. This is why a TV port could reuse every screen.*
- **A-20.2 †** — The player screen is React Native; competitors with heavy engines moved it native. *Portico ahead on consistency and maintenance; behind on tvOS system conveniences. I would keep it. Adopting a native screen would be large and would break the layout rule.*
- **A-20.29** — The engine never plays "by itself": every state change is a JavaScript intent. *Portico ahead in principle; incomplete in coverage. Add a reconciliation rule: when a progress fact says paused while the intent is playing and no seek or route change is pending, either re-apply the intent or adopt the pause. Small.*
- **A-20.34** — Post-play and skip are server policy. *Portico ahead (consistent across devices, and the safe-to-auto-skip flag is unique). A chapter-name fallback for files with "Intro"/"Credits" chapters is a server feature to consider (domain 04).*
- **A-21.1** — Server-composed media surfaces versus client-composed hubs. *Portico ahead. Deliberate thin-client principle. Keep it; adding cross-server composition would be large work in scoped identity, aggregation and presentation, not just an extra shelf. A stronger client customization contract could change the balance.*
- **A-21.9** — Native owner console is unusually broad. *Portico ahead. Deliberate documented native Server pages. Keep and advertise carefully; medium polish for complex phone forms. Swiftfin shows that user/device/task administration is not inherently a web-only task.*
- **A-21.11** — Authored settings layout rather than registry-generated UI. *Portico ahead. Deliberate good architecture. Small to maintain a rendered-versus-consumed matrix; audit B-21.6/10 because field existence alone is insufficient.*
- **A-21.14** — Home saved query rows are server-backed resources. *Portico ahead. Deliberate server/query architecture. Keep; medium to expose equivalent TV creation with guided naming, small to improve discovery of the existing mobile workflow.*
- **A-21.19** — Library peers are shared per media kind. *Portico ahead. Deliberate shared authoring. Keep; small responsive overflow/focus improvements if real long labels or many tabs fail.*
- **A-21.25** — One canonical show workspace instead of many detached child screens. *Portico ahead. Deliberate media contract §9. Keep; medium acceptance coverage for specials, groups, empty seasons and deep-linked unavailable episodes.*
- **L-24.12 †** — Implement the server feature and the client contract together. *Portico ahead in structure, unknown acceptance. Medium per-platform coverage. Publish a server × client × device matrix. A completed backend ticket is not completion for a household. This is especially important for Cast and downloads.*

#### T09 — Use platform video players and normalize media on the server (23)

- **A-02.5** — Evidence tags on every capability. *Portico ahead. Small, useful for support. Keep.*
- **A-02.16 †** — Passthrough is route-aware. *Portico ahead in design, but the fact is lost for v1-publishing devices (B-02.2).*
- **A-02.22** — A client-declared transport class. *Portico ahead in having one model. Note the surprise in B-02.10 (cellular default allowHDR=false forces conversion).*
- **A-02.48** — Stream facts are refreshed at first play. *Portico ahead, with the small consequence that a preview made before the first play can differ from the plan made at start.*
- **A-03.10** — Hardware selection: probe everything automatically versus owner picks. *Portico ahead. "Works without configuration and tells you why not" is a real advantage. Keep it.*
- **A-03.14 †** — Tone-mapping choices. *Portico ahead on defaults, behind on speed and control. Either apply the chosen curve everywhere or say in the setting's description that it applies to software tone mapping only.*
- **A-03.23** — Alternate audio as real HLS renditions. *Portico ahead. One detail to verify at run time: audio renditions are always MPEG-TS on the fixed 6 s grid even when the video is fMP4 on a keyframe-derived timeline (hls_renditions.go:294, :426). The HLS specification permits renditions with different segment boundaries and formats; hls.js an*
- **A-03.40** — Prepared versions: two fixed recipes. *Portico ahead of Jellyfin for having it; the recipe has problems — B-03.14 (30 fps) and F-03.11 (HDR titles cannot be prepared).*
- **A-04.16 †** — HLS subtitle renditions. *Portico ahead in having multi-track renditions at all; behind on attributes. Small: emit DEFAULT/AUTOSELECT=YES for the session's selected track, FORCED=YES for forced tracks, and public.accessibility. characteristics for SDH.*
- **A-04.25** — Switching audio. *Portico ahead on HLS; equal otherwise.*
- **A-10.15 †** — Gapless by server-measured facts and on-device decoding. *Portico ahead in ambition and, where the fixtures cover it, in result. Deliberate (spec §18.1 "Plex and Plexamp are the standard to match").*
- **A-10.20 †** — Loudness: tags, then server analysis, with peaks; off by default. *Portico ahead in mechanism, behind in defaults and modes. Small: an "auto" mode (album gain when the neighbours are the same album, which the queue already knows via albumId), and consider on by default.*
- **A-10.22 †** — Crossfade. *Portico ahead of the open-source field; behind Plexamp's content-aware fades, which need the loudness-over-time data Portico already has the tooling to produce (large).*
- **A-17.3** — Capability detection by asking the browser, not by naming it. *Trade-off (subjective), leaning Portico ahead for maintainability. A small, dated exception table for known liars (the handful above) would remove most first-play failures. Small.*
- **A-17.23** — Music decoded by Portico's own engine, not the media element. *Portico ahead, with ownership cost. The conformance fixtures (spec §18.8, web/audio-conformance.html) are the right safety net; they need to be re-run on each browser release and on Firefox.*
- **A-17.43** — Casting from the browser. *Portico ahead on credential handling (covered in domain 13). The HTTPS requirement means a plain-HTTP home server has no Cast button at all, with no explanation in the player.*
- **A-19.20** — The focus engine is the platform's (except on Vega). *Portico ahead on tvOS and Android TV. On Vega, Portico is in Plezy's position (rebuilding behaviour) but with less machinery and no tests.*
- **A-19.23** — Siri Remote touch surface. *Portico ahead — narrow, native, off by default.*
- **A-20.6 †** — A declared table plus a probe, with evidence recorded. *Portico ahead in design, behind in what survives to the server. Fix with A-20.5.*
- **A-20.14 †** — Dolby Vision profile 7 falls back to its base layer by copy; others convert it on the device. *Portico ahead on honesty and safety; behind on profile 7 picture quality. Medium, server side.*
- **A-20.37 †** — Live TV positions are wall-clock dates. *Portico ahead in model, with one Android gap.*
- **A-20.41** — Binary size, battery and maintenance. *Portico ahead. Any second engine should be opt-in per play ("this file needs the alternative player") so the default keeps these properties.*
- **A-21.35** — App voice intentions are distinct from dictation. *Portico ahead. Keep Apple integration and repair feeder. Medium Android assistant/catalog adapters if justified; avoid claiming unsupported global catalog access.*

#### T10 — An authored, consistent interface with restrained customization (16)

- **A-16.4 †** — Bespoke design system on Radix versus a component kit. *Portico ahead on coherence. The risk is accessibility detail work (see B-16.13 to B-16.17).*
- **A-16.17** — Routing: real URLs with validated params versus a hash router. *Portico ahead for a browser product. See A-16.18 for the sub-path cost.*
- **A-16.35** — Message format. *Portico ahead.*
- **A-16.39 †** — Locale-aware formatting. *Portico ahead in design.*
- **A-16.45** — Accessibility engineering. *Portico ahead on intent, mixed on detail.*
- **A-17.32 †** — Skip prompts are server-authorised and reported. *Portico ahead in design.*
- **A-18.4** — Search: a destination with chips against a field with sections. *Portico ahead. One open point: the address carries the words, which the navigation contract forbids (D-18.5).*
- **A-18.5** — Home: server-composed rows with an order dialog against user-chosen slots. *Portico ahead on the model. Consider two viewer switches Jellyfin and Plezy both have: "show hero" and "hide watched from recently added" (small each, server-side row rule plus one preference).*
- **A-18.12 †** — Pick before play. *Portico ahead on presentation; behind in effect until versions merge (Backlog, "A second file of the same title becomes a second title").*
- **A-18.37** — Visual language. *Portico ahead on polish and coherence. Theme music is the one piece of "atmosphere" Jellyfin and Plex have that Portico lacks (F-18.16).*
- **A-18.40** — Addresses and links. *Portico ahead. Exceptions noted in B-18.25 (some cards are not links) and B-18.17 (a season card's address).*
- **A-18.45** — Loading, empty and error states. *Portico ahead.*
- **A-18.46 †** — Accessibility affordances. *Portico ahead on semantics; the tab title (A-18.41) and theme (A-18.32) are the gaps. A proper review belongs to the accessibility auditor.*
- **A-19.22** — Menu and Back policy. *Portico ahead on tvOS. The Android Menu-as-long-press trick has a real defect (B-19.3).*
- **A-19.47** — Minimum OS versions. *Portico ahead on reach. API 24 vs 23 excludes almost nothing.*
- **A-21.6** — TV setup is a first-class code/QR approval workflow. *Portico ahead. Deliberate code-centric design. Keep it; small UI repair B-21.1 completes the promised path. Cryptographic/idempotency internals are outside this report.*

#### T11 — One broad media product, with depth bought selectively (6)

- **A-06.30 †** — Genres, tags, studios, networks. *Storage Portico ahead; "one studio" Portico behind (co-productions lose all but the first company, so "A24" finds only the films where it is listed first).*
- **A-10.12 †** — Online matching is evidence-scored and conservative. *Portico ahead on correctness, behind on coverage and speed. Deliberate. Worth deciding how the owner experiences a large first import (see Open questions).*
- **A-10.33 †** — Playlists. *Portico ahead on collaboration and scale, behind on smart playlists and M3U (F-10.7, F-10.8).*
- **A-11.41** — Xtream as a first-class source. *Portico ahead for having it; see F-11.20 for what is left on the table.*
- **A-21.47** — Native Live/DVR sits beside first-party library channels. *Portico ahead. Keep channel abstraction and native DVR. Medium guide/favorite/reorder polishing; small cancellation repair. Tuner/timeshift engines excluded.*
- **L-24.27 †** — Music users notice grouping and continuity more than generic feature count. *Portico ahead in ambitions, unknown perceptual/device acceptance. Medium metadata fixtures and large DSP/recommendation optional work. Preserve compilations, disc/track order and different releases; never “fix” duplicates by collapsing legitimate editions.*

#### T12 — Keep ownership bounded, even at the cost of ecosystem reach (14)

- **A-06.47** — Attribution is a product requirement. *Portico ahead (and safer with providers' terms).*
- **A-08.5 †** — Versioning scheme. *Portico ahead in design, deliberate. To make it real: define what bumps level (any additive change clients may rely on), and prefer the per-feature strings in /v1/capabilities for anything optional. Small.*
- **A-08.32 †** — Notifications. *Portico ahead in-app, behind off-app. A generic outbound webhook per notification category would respect "no push, no email from the server" while letting owners wire their own (F-08.2).*
- **A-09.45 †** — API keys. *Portico ahead. I did not trace whether every high-risk route actually refuses a full key; that belongs to the identity audit.*
- **A-16.52 †** — Tests. *Portico ahead on unit tests, behind on rendering/e2e. A small Playwright suite against the dev server and the existing demo fixtures (sign in, Home, scroll a library, open a title, Back, play, switch profile in two tabs) would cover the biggest risks. Medium.*
- **A-18.28** — Logs and diagnostics. *Portico ahead.*
- **A-18.34** — Notifications. *Portico ahead. No push or email by decision.*
- **A-18.49** — Feedback is built in. *Portico ahead. On phones the menu entry is unreachable (B-18.14).*
- **A-19.7** — Dependency count. *Portico ahead on hygiene, and it is clearly deliberate. The honest cost is in A-19.26 (lists), A-19.29 (images) and A-19.45 (tests), where a mature library would bring behaviour Portico has not built.*
- **A-19.49** — Polyfills are few and explicit. *Portico ahead on hygiene. The port log's question 11 (was Download working on iPhone before this?) deserves an answer, because it implies a shared service was never exercised on a device.*
- **A-25.17 †** — Manually approved FFmpeg is a separate component release. *Portico ahead — Keep deliberate qualification. Medium: publish approval evidence and component compatibility/security replacement policy. A feature manifest should record hardware acceptance separately from compile/codec presence.*
- **A-25.18** — hls.js patch provenance is explicit. *Portico ahead — Keep provenance and reproducible rebuild procedure. Small/medium: CI verify artifact/hash and rerun boundary regression on upgrades. Reconsider maintaining the fork when upstream fixes the exact problem.*
- **A-25.47 †** — Public third-party API intent is unusually explicit. *Portico ahead — Strong deliberate intent, not yet a verified compatibility promise. Medium: publish minimal third-party client examples and semantic vectors independent of UI packages. Treat capability negotiation as part of public contract.*
- **A-25.56** — Private contracts are knowledge capture, not completion certificates. *Portico ahead — Keep the separation and exact-source evidence. Medium: map every required outcome to executable/manual evidence with owner/platform/build identity and expiration triggers. Do not publish private provenance by accident.*

#### T13 — Personal discovery stays local and distinct from saved intent (18)

- **A-02.37** — When a play is counted. *Portico ahead for most uses (recommendations, "most played").*
- **A-06.46** — Trending feeds, provider "similar" lists and the Portico title dataset. *Portico ahead.*
- **A-12.7** — Completion by credits marker. *Portico ahead of Jellyfin, level with Plex. Open question whether unapproved high-confidence markers should count.*
- **A-12.12 †** — Play count means "played to the end", counted once per playback. *Portico ahead in meaning, behind in use.*
- **A-12.23 †** — Season premieres are exempt from the window. *Portico ahead in intent; the rule needs a recency bound.*
- **A-12.26** — Remove from Continue Watching, with Undo and show-level scope. *Portico ahead.*
- **A-12.30** — Four separate personal signals. *Portico ahead.*
- **A-12.34 †** — Playlists. *Portico ahead on integrity, behind on smart and import.*
- **A-12.35** — Collections: two kinds. *Portico ahead. Neither kind can become a Home row (F-12.7).*
- **A-12.45** — A maintained taste model versus per-request category lists. *Portico ahead, by a wide margin. The cost is the amount of machinery to keep correct (triggers, jobs, overlays, memos).*
- **A-12.46** — Coverage: every media family, on Home, in libraries, on title pages and as a sort. *Portico ahead.*
- **A-12.48** — Feedback controls. *Portico ahead. Not interested hides one title; there is no "less like this" for a genre or person.*
- **A-12.49** — Cold start. *Portico ahead. No onboarding "pick a few you like" step (F-12.14).*
- **A-12.50** — Restricted (kids) profiles. *Portico ahead.*
- **A-12.51** — Community signals are opt-in twice and need at least two people. *Portico ahead; raise the minimum or hide the row for the people who form the minimum.*
- **A-12.52** — Daily rotation and exploration. *Portico ahead; use the server's local day.*
- **A-18.15** — Personal state vocabulary. *Portico ahead. Deliberate (Spec — Title Pages §1: "Like/Dislike does not exist").*
- **A-23.24** — Public social graph versus opted-in household signal. *Portico ahead for minimizing disclosure, Trade-off for social value. Large social service, medium opt-in export/scrobble. Prefer optional connectors. Explicit user demand could justify social layers, but not default disclosure.*

#### T14 — Offline copies remain verified, scoped parts of the server (9)

- **A-14.3 †** — Converted downloads are finished files made ahead of time and shared. *Trade-off (subjective), leaning Portico ahead on the engineering and behind on what a non-owner can actually do. Deliberate ("downloads never widens who may order encoding work", S/api/downloads.md:55-60). A per-profile "may request conversions" switch (Jellyfin's EnableSyncTranscoding*
- **A-14.7** — Size is known and published before the user commits. *Portico ahead. Keep.*
- **A-14.8** — Server storage is bounded by an owner ceiling counted in claims. *Portico ahead in having a limit at all; the default and the snapshot design (A-14.2) undermine it. Consider a non-zero default expressed as a share of the state volume.*
- **A-14.12 †** — Every state change is fenced and idempotent. *Portico ahead in intent; the progress counter and the fence should be two numbers.*
- **A-14.17 †** — End-to-end integrity check. *Portico ahead. Keep the client check. The verification after a background completion on iOS has a timing risk (B-14.18).*
- **A-14.33** — Who may download. *Portico ahead in rigour. Missing: per-library "no downloads", per-profile count/byte limits, a cap on simultaneous transfers per user, and an owner view of who holds what (the data exists: Usage with scope=server, service.go:219-276).*
- **A-14.34 †** — Whole-container requests are expanded by the server. *Portico ahead in design; the fence is too strict.*
- **A-21.31** — Downloads distinguish server preparation from protected local bytes. *Portico ahead. Deliberate saved-download contract. Keep; medium improvement for grouping/batch management; engine transfer integrity is outside this UI review.*
- **L-24.21 †** — Offline mode is a product state, not a download button. *Portico ahead in design. Large mobile/offline lifecycle acceptance. Test OS kill, storage pressure, account switch, expiry, network return and conflicting explicit watched actions. Do not remove useful offline bytes merely because a transient request failed.*

#### T15 — Freshness follows evidence and change, not a timer (10)

- **A-06.6 †** — Freshness windows, a refresh budget and conditional requests. *Portico ahead in design. Two side effects are bugs today: a language change is not a reason to refetch (B-06.9), and a person's Refresh on a show cannot get past the window (B-06.10). The freshness key should include the language, and "a person asked" should always bypass it.*
- **A-06.48** — Provider failures are durable state. *Portico ahead in visibility. The parking rule is too eager for long outages (B-06.12).*
- **A-08.21** — Real-time transport: SSE and long-poll versus one WebSocket. *Portico ahead for a self-hosted product. Deliberate (Style Guide amendment 8). The remaining untidiness is the number of channels: the Style Guide says "one /v1/events endpoint", the code has five long-lived routes, and a receiver device keeps two open (apps/portico-react-native/apps/native/src*
- **A-08.22** — Events as invalidations versus events as data. *Portico ahead on correctness; deliberate. Fine as long as the refetches are cheap (they are conditional and bounded).*
- **A-08.28** — Subscriptions and topics. *Portico ahead for simplicity. Revisit only if event volume grows.*
- **A-12.43** — Freshness by revision, never by time. *Portico ahead, deliberate. Remove the unused TTL field or decide what it means.*
- **A-13.44 †** — A real inbox, in-app only. *Portico ahead in-app, behind on reach. A single owner-configured outbound webhook would cover the away case without push infrastructure or server email (F-13.8); it does touch the recorded "no push or server email" decision, which is Justin's call.*
- **A-22.38** — Subscriber-aware deduplication cancels only orphaned reads. *Portico ahead in this narrowly implemented surface. Medium to generalize with scope/revision-aware keys instead of global endpoint caching.*
- **A-22.42** — One cursor-resumable device feed with explicit resync. *Portico ahead in protocol explicitness, deliberate. Medium to verify subscriptions honor resync and native auxiliary feeds use the same route/auth boundary.*
- **L-24.26 †** — Search should fail by section, not erase everything. *Portico ahead on completeness/failure isolation; behind on fuzzy capability. Medium optional suggestion index. Keep exact results correct and restriction-aware; fuzzy suggestions must not expose hidden titles or silently replace the literal query.*

#### L — Local positive differences (11)

- **A-03.21 †** — Downmix. *Portico ahead on the default, behind on choice. See F-03.9.*
- **A-06.13 †** — Title comparison. *Portico ahead on alias coverage (it fetches alternative titles for the top four), behind on a few common normalisations. Small additions.*
- **A-08.33** — Client log upload. *Portico ahead (structured, bounded, surfaced in the console).*
- **A-08.36** — Health and readiness. *Portico ahead. Three endpoints for one question is one or two too many for outsiders.*
- **A-08.37** — Capability negotiation. *Portico ahead. Make it trustworthy: one advertised limit is already wrong (B-08.5), and limits.ts in the generated contract is two lines long.*
- **A-08.39 †** — Observability of the API itself. *Portico ahead internally; behind on standard outward signals (no response-time header, no Prometheus endpoint; F-08.15). Legacy handlers do not return a request id.*
- **A-08.41** — Identifier and unit conventions. *Portico ahead on ids, mixed on units. One rule for time and duration before /v1 is frozen would spare every client a table of exceptions.*
- **A-09.40** — Diagnostics beyond logs. *Portico ahead, clearly.*
- **A-09.43** — Health endpoints. *Portico ahead.*
- **A-16.41** — Text the server sends. *Portico ahead.*
- **A-25.51** — FFmpeg distribution chooses GPLv3 and rejects nonfree. *Portico ahead — Explicit recipe/license guard is good. Medium ongoing work: audit dependency source completeness and package notices on each release; codec patent obligations are a separate jurisdictional question not settled by source licensing.*

## 5. Contradictions between documents

These are source-report conflicts, not runtime verdicts. The full entries were opened for the main conflicts below. Resolve behaviour from the active wiring before changing the design or advertising a strength; do not choose whichever assessment sounds more favorable.

### C1 — Live inputs and conversions: shared per channel or per viewer?

**A-03.28** explicitly says one FFmpeg per channel, with output held for all viewers, and rates Portico ahead. **A-11.1** explicitly says each session allocates its own tuner, upstream connection, FFmpeg and buffer, and rates it behind; A-11.45 repeats the per-viewer structure for Library Channels. Playback and Media:45 describes sharing. These are incompatible descriptions of the same live architecture. **Resolution needed:** follow the allocation/input/buffer ownership from two session starts for one channel; distinguish a shared raw source from shared conversion. Tuner capacity and weak-server cost decisions depend on the answer. The positive register marks A-03.28 disputed.

### C2 — API keys: implemented access, stored scaffolding, or deliberately disabled?

**A-09.45** rates scoped hashed keys ahead and discusses their use on high-risk routes. **A-01.33/B-01.3** say keys can be created but never authenticate. **A-08.15/A-15.32** describe a deliberately disabled path; document 08's “How Portico does it today” additionally says creation returns `api_keys_disabled`, conflicting with document 01's creation claim. A-25.47 finds explicit independent-client API intent. **Resolution needed:** name the actually registered create/authenticate routes and reachable UI, separately from dormant store code and future OAuth intent. Do not present model quality as working integration support or infer that third-party clients are permanently prohibited.

### C3 — Public HTTP: accepted by the server or disconnected before HTTP handling?

**A-15.36** says the server always listens for HTTP and does not discourage public-IP browser sign-in. **A-09.21** says non-private cleartext sockets are closed before a request is read. **A-22.15** reports public HTTP rejection in the client/server model. Those findings can only coexist if listener wiring, a fronting proxy or specific modes differ; “an HTTP handler exists” is not proof of public reachability. **Resolution needed:** trace direct listener selection and proxy/bind modes. Preserve Justin's first-class local HTTP decision; settle the public exposure separately. Do not count the deliberate local choice as a bug.

### C4 — Shared-client checksum enforcement: missing or already in CI?

**A-16.10** says nothing in CI checks copied packages and recommends adding a check. **A-19.9** says `scripts/shared-packages.mjs` runs in CI; **A-25.2** describes its hashing selection and identifies the remaining upstream-version problem. Document 25 also reports actual Hosted apikit drift (A-25.3/B-25.5), which is a different copy. **Resolution needed:** correct the web audit's checker-presence claim, then distinguish selected-file equality from pinned canonical-source identity and from the Go copy's incomplete gate. This changes both the assessment and proposed work size.

### C5 — Same-disk movie renames: continuity or lost personal state?

**A-05.7** describes exact device/inode-based relocation preserving the asset and item; **A-07.4** distinguishes same-disk continuity from cross-disk moves. **A-12.4/B-12.18** infer that a renamed/moved movie loses state from path-based entity keys, while explicitly admitting the scanner path was not traced. **Resolution needed:** separate same-filesystem rename/relocation, copy to another disk, replacement under a new name, root change and final purge. The probable broad claim must not erase the scanner's reported stronger path, while that stronger path must not be generalized to replacement/copy. A-07.5 still independently raises state deletion after purge.

### C6 — Apple offline originals: automatically compatible or offered unchanged?

**A-20.38** says Apple downloads are always MP4-family conversions/remuxes because AVPlayer requires compatible media. **A-14.4/B-14.12** say options ignore the device and offer Original even for an MKV, with no compatibility remux. **Resolution needed:** distinguish “must be compatible to play” from “is made compatible by download preparation.” The report's output-contract claim cannot be inferred from the player limitation. This decides whether originals need device checks, remux, refusal or an alternative offline engine.

### C7 — Refresh-receipt lifetime: five minutes or successor use/family horizon?

**A-01.15** says plain replay receipts are meant to clear after five minutes. **A-01.18** instead describes replay until successor use; **A-22.25** explicitly says receipts survive restart until successor use/horizon and calls the five-minute comments stale. **A-15.38/B-15.5** assess cleartext credential storage more broadly. **Resolution needed:** distinguish TV-code grant expiry from refresh receipts and other replay envelopes, then publish each intended lifecycle. A cleanup defect concerning approved TV codes (B-01.5/B-15.2) does not by itself prove the intended lifetime of refresh receipts. The retention/security cost needs the right model.

### C8 — Which artwork size actually ships?

**A-06.34** reports 400/800/1920 variants, consistent with A-16.28. Document 19's “How Portico does it today — Lists and images,” associated with **A-19.29**, says the scheduler chooses all three but today's server only serves ≤400 and 1920. **Resolution needed:** compare the actual versioned artwork handler with prebuilt and lazy resize storage, not just the requested client buckets. This may be a stale observation or two image routes; either way the shared resolution/cost discussion needs one answer.

### C9 — Native events: one SSE feed or intentional short long-polling?

Document 19's state/refresh walkthrough and **A-19.50** describe native server events over XHR/SSE. Document 08's walkthrough says no first-party client uses unified-feed SSE; **A-22.43** says native intentionally uses ten-second polls. B-22.18 identifies separate Watch Together/receiver XHR streams. **Likely scope reconciliation:** unified device feed polls; auxiliary feeds stream. Preserve that distinction in the final report rather than implying all native event traffic shares one transport or route-verification path. Confirm each consumer's opener and authority owner before costing an SSE migration.

### C10 — Native chapters: offered utility or absent player feature?

Document 21's “How Portico does it today” lists chapter utilities in the native player. **A-20.23/B-20.15** explicitly say there is no chapter list or ticks and that capabilities claim them; document 04's client coverage finds no native chapter-service use. **A-21.43** assesses the fixed control grammar, not successful decoder/utility execution. **Resolution needed:** show the reachable chapter action and consumer, or narrow the UI summary to implemented controls. A shared component, schema flag or utility vocabulary is insufficient to mark parity.

### Different assessments that should retain their scope

- **A-09.36 versus A-13.40:** “message to one viewer” is ambiguous. Terminate-with-message and message-without-stopping are different actions; document 13 supports the former and lacks the latter. Reword the operations gap rather than conclude that no message can be sent.
- **A-09.9/A-15.48 versus A-25.31–33:** strong sealed binary provenance and weak final-package signing can both be true. Document 25 narrows the authenticated bytes. Retain the provenance advantage while withholding a whole-product integrity claim.
- **A-19.7 versus A-16.2/A-25.15–16:** “few dependencies” can be a hygiene advantage and still create more owned implementation/qualification cost. Counts from one stack do not establish whole-product simplicity.
- **A-12.43 versus A-22.37/A-25.38:** revision-keyed derived results, a guide read memo and external update-feed freshness are different cache classes. A transport memo exception needs recording; external evidence lifetimes need not violate the derived-data rule.
- **A-04.15/A-20.18 versus A-17.15:** the native subtitle expiry description is forty seconds, while the web walkthrough gives a seventy-five-second failure threshold. Treat this as platform-specific behaviour requiring consistent UX, not evidence that either auditor's timer is necessarily wrong.
- **A-04.31/A-20.34 versus A-23.9–10/L-24.40:** a strong marker/approval protocol and weak automatic intro/credits detection are independent assessments. Do not market the protocol as detector accuracy.
- **A-10.33/A-12.34 versus A-21.29:** collaborative playlists and stable entry IDs can be ahead while selecting a duplicate occurrence starts the wrong item. Integrity of mutation and integrity of playback selection are separate boundaries.

### Written product conflicts relevant to the themes

These do not necessarily represent disagreement between auditors, but they must be made explicit before a design choice:

- **One provider / anime default:** Catalogue and Metadata:66 and A-06.1 describe one provider and TMDB anime default; decisions P10 retains selectable-fallback wording and AniList default. L-24.35 also warns against treating Jellyfin as one-provider: the audit describes chains. Reconcile current intent, default and the meaning of “fallback” without silently mixing providers.
- **Background scheduling:** Overview:51 says never paused; A-05.34 finds source playback pauses analysis; A-09.38/B-09.28 find maintenance-window wording mostly unimplemented; A-23.13 leaves the policy unsettled. These are three distinct scheduling promises.
- **Offline expiry and sign-out:** decisions P06 and the offline contract preserve bytes on expiry/unreachability but delete on learned revocation/sign-out. A-14.9/A-14.32 describe different lifecycle outcomes. Do not use that contradiction to discard the explicit thirty-day choice.
- **Native console / navigation:** decisions P18 is web-first for broad operations; A-21.9 describes broad native panels. A-21.41 reports kept-alive roots versus a newer reset law. Label deliberate source behaviour separately from the governing contract.
- **Password and device-binding language:** A-15.9 and A-15.7 identify old composition/proof-of-possession promises that the implementation does not meet. Justin explicitly retains simple passwords; a random installation ID is not a device key. Correct stale promises and assess the actual boundary without reclassifying his simple-password policy as a defect.

### C11 — Which playback/control API is the current product boundary?

Documents 02, 08 and 17 describe Playback v1, a live legacy bridge and an unwired newer controller (**A-02.24–27, A-08.3, A-17.35**). Document 23's walkthrough and “Doc/code disagreements” identify API README:34 and existing `/v2` controller/lane/occurrence control routes, and warn against calling remote-controller architecture absent. **A-13.25/A-13.38** nevertheless describe handoff without continued remote control. **Resolution needed:** record each protocol's actual registered routes, client consumers and migration status, including whether v2 is scaffolding or the current consumer path. The existence of v2 routes does not prove the UI uses them; a v1-only audit likewise cannot settle the current control architecture. API breadth, adapter retirement and the size of adding remote control must be reviewed against the same source/wiring snapshot.

## 6. Coverage

Each cell counts primary members only. Columns T01–T15 refer to the names in section 1; **Local** is section 2. Document 24 contributes L-24 lessons rather than A entries. Overlapping B/process references and the positive-assessment register are excluded from counts.

| Document | T01 | T02 | T03 | T04 | T05 | T06 | T07 | T08 | T09 | T10 | T11 | T12 | T13 | T14 | T15 | Local | Total |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| 01 Identity and access | 1 | 28 | 7 | 0 | 7 | 4 | 0 | 0 | 0 | 0 | 0 | 4 | 0 | 0 | 0 | 2 | 53 |
| 02 Playback decisions and protocol | 0 | 2 | 9 | 0 | 10 | 1 | 0 | 4 | 19 | 0 | 0 | 0 | 2 | 0 | 0 | 1 | 48 |
| 03 Transcoding | 0 | 0 | 3 | 7 | 9 | 2 | 0 | 0 | 14 | 0 | 0 | 1 | 0 | 0 | 0 | 14 | 50 |
| 04 Subtitles and analysis | 1 | 3 | 1 | 8 | 2 | 2 | 12 | 0 | 10 | 1 | 0 | 2 | 0 | 1 | 0 | 5 | 48 |
| 05 Inventory and naming | 2 | 0 | 4 | 5 | 8 | 4 | 13 | 0 | 0 | 1 | 5 | 0 | 0 | 0 | 2 | 5 | 49 |
| 06 Metadata and artwork | 0 | 3 | 0 | 7 | 1 | 0 | 19 | 1 | 0 | 3 | 8 | 2 | 1 | 0 | 4 | 4 | 53 |
| 07 Persistence and queries | 1 | 1 | 1 | 23 | 7 | 0 | 6 | 0 | 0 | 0 | 2 | 1 | 0 | 0 | 1 | 6 | 49 |
| 08 API and events | 0 | 0 | 1 | 2 | 7 | 8 | 0 | 3 | 0 | 0 | 0 | 13 | 0 | 0 | 7 | 6 | 47 |
| 09 Server operations | 29 | 0 | 10 | 4 | 6 | 0 | 0 | 1 | 0 | 1 | 0 | 4 | 0 | 0 | 0 | 4 | 59 |
| 10 Music and books | 0 | 1 | 1 | 2 | 3 | 1 | 4 | 0 | 9 | 0 | 22 | 0 | 0 | 0 | 1 | 3 | 47 |
| 11 Live TV and DVR | 2 | 3 | 3 | 6 | 11 | 2 | 3 | 0 | 2 | 0 | 15 | 1 | 0 | 0 | 0 | 0 | 48 |
| 12 Personal state and discovery | 0 | 3 | 13 | 6 | 6 | 0 | 1 | 2 | 0 | 0 | 1 | 2 | 27 | 0 | 2 | 1 | 64 |
| 13 Social playback and receivers | 1 | 11 | 7 | 0 | 7 | 0 | 0 | 4 | 0 | 0 | 8 | 1 | 0 | 0 | 5 | 1 | 45 |
| 14 Downloads | 0 | 1 | 0 | 1 | 1 | 0 | 0 | 0 | 6 | 0 | 1 | 1 | 0 | 30 | 0 | 2 | 43 |
| 15 Security | 3 | 3 | 6 | 0 | 5 | 31 | 0 | 0 | 0 | 0 | 0 | 2 | 0 | 0 | 0 | 0 | 50 |
| 16 Web architecture | 2 | 1 | 0 | 5 | 7 | 4 | 0 | 9 | 0 | 19 | 0 | 8 | 0 | 1 | 3 | 2 | 61 |
| 17 Web player | 0 | 1 | 4 | 2 | 13 | 0 | 0 | 0 | 13 | 3 | 0 | 2 | 0 | 0 | 0 | 5 | 43 |
| 18 Web experience | 1 | 3 | 3 | 3 | 2 | 0 | 3 | 6 | 0 | 23 | 1 | 4 | 2 | 1 | 0 | 0 | 52 |
| 19 Native architecture | 0 | 2 | 0 | 4 | 5 | 2 | 0 | 10 | 5 | 11 | 0 | 11 | 0 | 1 | 4 | 0 | 55 |
| 20 Native players | 0 | 1 | 2 | 1 | 4 | 0 | 0 | 3 | 23 | 5 | 0 | 0 | 0 | 0 | 0 | 2 | 41 |
| 21 Native experience | 4 | 4 | 2 | 5 | 0 | 0 | 1 | 11 | 2 | 19 | 2 | 3 | 0 | 2 | 0 | 0 | 55 |
| 22 Connectivity | 15 | 0 | 0 | 4 | 12 | 7 | 0 | 0 | 0 | 1 | 0 | 1 | 0 | 1 | 11 | 2 | 54 |
| 23 Adjacent competitors | 6 | 3 | 4 | 0 | 2 | 0 | 3 | 0 | 2 | 0 | 10 | 4 | 2 | 0 | 0 | 0 | 36 |
| 24 Community lessons | 1 | 1 | 0 | 8 | 8 | 1 | 6 | 3 | 1 | 1 | 2 | 6 | 0 | 1 | 1 | 0 | 40 |
| 25 Engineering | 3 | 0 | 2 | 6 | 7 | 2 | 0 | 6 | 0 | 0 | 0 | 27 | 0 | 0 | 0 | 7 | 60 |
| **Total** | 72 | 75 | 83 | 109 | 150 | 71 | 71 | 63 | 106 | 88 | 77 | 100 | 34 | 38 | 41 | **72** | **1250** |

**Coverage reconciliation:** 1,178 themed + 72 local = 1,250. There are no unassigned architectural/lesson IDs and no duplicate primary placements. The supplied index's 25 document counts agree with the source-heading inventory. The complete positive register contains 443 unique indexed positive assessments. Process patterns are illustrative cross-cutting groups, not an independent exhaustive verification of 619 B/Q items.

### Placement rules and practical use

- Large repeated mechanisms go to the bet they serve: SQL restrictions primarily express household authority; viewer-scoped artifact lifetime expresses offline authority; windowing primarily expresses scale. Mentioning revisions does not automatically put an entry under T05.
- An absence is a theme member when it is a concrete cost or inconsistency of that bet: no local offline catalogue tests thin-client/server dependence; lossless surround conversion tests the platform-player split. “Behind” by itself is not a philosophy.
- One-off parser bugs, encoder tuning, naming conventions and packaging details remain local unless the evidence establishes a wider policy. Those same entries can expose a process pattern without being forced into a theme.
- Lessons are assigned by their architectural proposition, not by competitor demand volume. Requested features and historical community complaints are evidence for a decision, not an automatic roadmap.
- Resolve the conflicting production-path claims in section 5 first. Then decide whether each cost is accepted, the policy needs a narrow exception, or the implementation has failed its own policy. The recurring actionable middle course is narrower rejection, finer revisions, common consumers and better recovery—not wholesale replacement of the bets.

### Source orientation

Entry IDs resolve to the corresponding numbered domain document in this audit directory; titles in the member lists retain the supplied index wording. Portico principle references above are relative to `/home/justin/projects/Portico`: `portico-internal/Project Architecture/` for Overview, Catalogue and Metadata, Playback and Media, Clients and Recommendations; `portico-internal/Product Contracts/` for decisions.md, architecture.md and family contracts. Numerical line citations are source-reading locations, not proof that implementation meets them. Comparator claims are taken from the audit documents' descriptions only; current product prices, law, licenses, store release availability and vendor policies were not rechecked.
