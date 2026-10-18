# 23 — Closed-source and adjacent competitors: public feature and architecture survey

## Scope and sources

**Audit date: 11 October 2026.** This is a read-only public-information survey, with Portico source inspection to prevent false gaps. The central choices are whether Portico should be a better personal server, a broader media platform, or also a great independent player; how much central infrastructure it should own; and which ecosystem interfaces it needs before users will move.

The competitors' private implementation details are unknown. Documentation establishes advertised behavior and published contracts, not internal algorithms, performance, or device reliability. Forum reports establish that a person experienced or disliked something; they do not establish its prevalence. “No evidence found” means a bounded search, not mathematical proof of absence. Prices are USD examples, excluding tax and regional differences, checked on the audit date; subscription entitlements can change.

I read the shared brief in full, all eight architecture documents, all of Backlog including Planned features, the headings of every Product Contract, client-core's settings structure, `server/api/README.md`, the generated `portico.openapi.json` path inventory, and the broader `openapi.yaml`. I inspected implementation in identity profiles, scan routing, local NFO parsing, music metadata inputs, audio measurement/planning, prepared-media work, and deep media-analysis production and detectors. This is not an end-to-end review of every Portico domain; those belong to the other auditors.

No builds, installs, tests, server launches, Git mutations, browser/computer-use/MCP calls, or requests to Portico infrastructure were made. Only this output file was written under Portico; public-page fetches and research scratch data went to `/tmp`. Execution host is `ehler-homelab-mini-1`. Required context-sync was attempted but failed on its lock file outside the writable sandbox. Existing project and verification guidance was readable; its freshness could not be established.

**Report inventory:** 36 architectural differences (A-23), 65 numbered capability proposals (F-23), no confirmed/suspected bug entries (B-23), 128 feature-matrix rows, and 152 linked public sources.

### Terms used in this survey

- **NAS:** network-attached storage, usually a small file-serving appliance.
- **Direct play / remux / transcode:** play original bytes; change their container without re-encoding; or encode audio/video into another format.
- **NAT / CGNAT:** router address translation; carrier-managed translation that can prevent ordinary inbound port forwarding.
- **Relay:** a vendor/server tunnel carrying a connection when the client cannot reach the home server directly.
- **NFO:** local XML metadata sidecars following media-center conventions; supported tags vary by product.
- **HDR / SDR / tone mapping:** high versus standard dynamic range, and conversion between them. Codec decoding and display output support are separate.
- **Object audio / passthrough / LPCM:** positioned sound objects such as Atmos; forwarding an encoded stream to a receiver; or decoded uncompressed audio channels. Lossless channel decoding does not guarantee preservation of object metadata.
- **OPDS:** an open publication catalog/distribution protocol used by ebook/comic readers; it does not inherently synchronize reading progress.

### Reading key and local evidence map

Paths below are relative to `/home/justin/projects/Portico`. The identifiers are used throughout the matrix and findings to make the verification boundary visible.

| Key | Evidence actually inspected | What it supports |
|---|---|---|
| PC1 | `portico-internal/Project Architecture/01 Overview.md:46`; `02 Server.md:13` | Free, non-commercial personal server; Go/SQLite; bounded indexed reads; portable state; exact-revision freshness; owner choice |
| PC2 | `02 Server.md:47`; `07 Hosted Services.md:7`; `server/api/README.md:62` | Local accounts, Hosted account assertions, account/profile distinction, server membership authority, device sessions |
| PC3 | `02 Server.md:56`; `07 Hosted Services.md:33` and `:47` | Names/certificates/directory; direct connections; free port forwarding; no present relay; possible paid relay |
| PC4 | `03 Catalogue and Metadata.md:60`; `apps/portico-server/server/internal/localmetadata/video_parse.go:49` | One selected screen provider, local-only mode, NFO support, bounded strict XML, manual fields and artwork |
| PC5 | `04 Playback and Media.md:15`; `apps/portico-server/server/internal/playback/audio_plan.go:35`; `internal/audiofacts/facts.go:1` | Direct/remux/convert, prepared media, hardware support, gapless trim measurements, conversion ceilings |
| PC6 | `05 Recommendations.md:13` and `:35` | Maintained content/taste features, diversity, For you, opt-in household activity, My List/saved views |
| PC7 | `06 Clients.md:32`, `:50`, `:66`; `packages/client-core/src/presentation/settings-structure.ts:153`, `:232` | Shared settings, intro/credits/recap preferences, lyrics, gain, crossfade; Apple/Android engine choices and platform qualifications |
| PC8 | `04 Playback and Media.md:42`; `server/api/social.md`; `server/internal/httpapi/social_routes.go` | Live TV, DVR, Library Channels, Watch Together, Cast/remote playback |
| PC9 | `02 Server.md:66`; settings structure server panels; `server/api/openapi.yaml:84` | Owner history, jobs, diagnostics, settings, backups, in-app alerts, API administration |
| PC10 | `server/internal/persistence/migrations/0001_baseline.sql:5` and `:17` | Actual kinds are movie/show/season/episode, artist/album/song, book/audiobook_file, collection/extra/disc/author; reading/photos numbers reserved but not inserted |
| PC11 | `server/internal/mediaanalysis/detectors.go:51`; `produce.go:313`, `:362`; `markers.go:115` | Heuristic marker candidates require review; loudness/waveform/spectral signatures exist; owner approval/editing |
| PC12 | `server/internal/httpapi/router.go:995`; `inventory.go:198` | Whole-library and source-scoped scans exist; inspected handlers do not accept a file or directory path to limit that scan |
| PC13 | `server/internal/identity/direct_profiles.go:14`, `:43`, `:112` | Four-digit PINs, profile ordering/library policy; eight profiles per direct account |
| PC14 | `server/internal/metadata/music_inputs.go:153`, `:168`; `metadataprovider/fingerprint.go:82` | AcoustID integration expects compatible embedded Chromaprint evidence; generic spectral analysis is not equivalent |
| PC15 | `Project Architecture/Backlog.md:13`, `:19`, `:61`, `:93` | Known work: music radio/mixes, audiobook agent, version grouping, platform UI completion, desktop, DLNA, relay |

For server code keys PC5–PC14, omitted prefix is `apps/portico-server/`. For packages, omitted prefix is also `apps/portico-server/`. Full citations are provided where a distinction needs exact lines.

**Absence searches:** I searched non-test `.go`, `.ts`, `.tsx`, `.sql`, `.kt`, `.swift`, `.m`, and `.mm` in server `internal`, client-core `src`, and the native app, excluding dependency/build/generated directories. There were no named implementation matches for `trakt`, `simkl`, `scrobbl`, `webhook`, `sonarr`, `radarr`, `bazarr`, `overseerr`, `jellyseerr`, `kometa`, `sonic`, `equalizer`, `carplay`, `androidauto`, `subsonic`, `roon`, `epub`, `cbz`, or `comskip`. `podcast` appeared in one defensive audio-kind list, not in an ingest/library workflow. `smb` appeared in the server rclone mount configuration, not in an independent native player. `dlna` appeared in generic route/fallback comments, and Backlog explicitly plans it. `photo` matches include profile and cast portraits; they do not establish a photo library. Later checks also found Portico trailer extra types and server deinterlacing/scaling, while no automatic cinema-pre-roll or AI-upscaler path was found. Later checks used `equaliz`, `parametric`, `sample.?rate.?match`, `bit.?perfect`, `headless`, `sonos`, `nfo`, `export`, and source/scan route searches. These checks are denoted **K** in the matrix/findings. Names can differ, so uncertain depth is explicitly marked.

## How Portico does it today

Portico is already broad. It is not merely movies, TV, and a basic video player. The current architecture describes anime libraries, music, audiobooks, remote/mounted sources including rclone and WebDAV, NFO/local metadata, manual edits and Fix Match backend, personal saved state, collections/playlists/saved queries, recommendation rows, Live TV/DVR, synthetic Library Channels, downloads, Watch Together, playback handoff, Cast, and an owner console. A competitor feature is not “missing” just because its product uses a different name.

The server stores facts and maintains derived search, browse, counts, relationships, and recommendation tables. Client pages use server-composed documents. Request work is intended to depend on visible results, while bulk updates happen incrementally in the background. Exact revision keys are meant to prevent stale results after edits. These are architectural aims and source mechanisms, not proof of 5–10-million-item device performance (PC1/PC6).

Screen metadata comes from one chosen provider per library, with local sidecars and owner overrides. TMDB, TheTVDB, and AniList are choices; AniList does not name episodes. Music uses MusicBrainz and compatible AcoustID evidence. Books are presently audio book containers and files, with local/tag metadata. The baseline reserves reading and photo kind numbers without instantiating those kinds. An extensible entity table does not itself implement an EPUB reader or a mobile photo backup app (PC4/PC10/PC14).

The server decides delivery from declared client capabilities: original file, remux, or conversion. Apple uses AVPlayer plus custom native audio modules; Android uses Media3/ExoPlayer and its listening service; web uses HLS and WebCodecs audio. Hardware acceleration, gain normalization, gapless, crossfade, lyrics, chapter/seek information, and prepared conversions are already represented. Actual codec/HDR/audio-output equivalence to Infuse or Plexamp is unestablished. The settings value “automatic” is a delivery/ceiling policy, not evidence of Plex-style ongoing bandwidth adaptation (PC5/PC7).

Background media analysis has real implementations for loudness, waveforms, fingerprints, trickplay, and marker candidates. The inspected intro/recap/commercial/credits detector uses chapter labels and sampled luminance. It explicitly treats results as unapproved hypotheses. This is substantially different from Plex's across-episode audio matching or cloud-assisted credits data; a Skip Intro preference does not demonstrate effortless accurate automatic detection (PC11).

A server-local account can create multiple PIN profiles and use direct authentication without a central identity. Optional Hosted account assertions simplify sign-in/discovery. Hosted publishes names and certificates; media normally goes straight to the user's server. It is not a media host, metadata aggregator, or relay today. Plain HTTP is a deliberate choice, while certificates are available for claimed servers and custom HTTPS is supported (PC2/PC3/PC13).

The current API reference describes a `/v2` controller/lane/occurrence playback control plane, whereas overview architecture still emphasizes Playback Protocol v1. The generated JSON requested in the prompt is a narrower route set than `openapi.yaml`; I did not infer gaps from that JSON alone. See Doc/code disagreements.

## How the others do it — per-product profiles

### Jellyfin: the free server baseline, from documentation

Jellyfin uses server-local users and has no vendor account or subscription gate for hardware transcoding, remote playback, or DVR. Owners supply networking, HTTPS/reverse proxy or VPN, and platform configuration. Its documentation describes a modified FFmpeg pipeline with Intel, NVIDIA, AMD, Apple, and Rockchip acceleration, including supported tone-mapping paths. That extensive matrix also documents why configuration is hard: driver, container-device, codec, and partial-acceleration combinations matter. [Jellyfin networking][J-network] [Jellyfin GPU pipelines][J-hardware]

Libraries cover video, music, photos, and books, though “supported file type” is not synonymous with a polished reader or dedicated music discovery experience. Official documentation lists NFO handling, naming conventions, local assets, plugins, Trakt, and webhook integration. Community clients and plugins fill many specialist roles; this creates choice and varying capability by device. Remote server setup remains an owner operation rather than an account-based relay service. [Jellyfin library types][J-libraries] [Jellyfin books formats][J-books] [Jellyfin music metadata][J-music] [Jellyfin plugins][J-plugins]

The official 7 September 2026 release documents improved relational playlists/collections, episode versions, book/comic metadata/readers, and selectable recommendation sources including ListenBrainz. Older format/client docs need that update when judging feature depth. [Jellyfin 12.0 release][J12]

Strengths: no feature paywall, local authority, extensibility, many community clients. Complaints to investigate are client inconsistency, remote setup, GPU configuration, music depth, and large-library responsiveness; no prevalence or current universal failure is established here. Portico's shared client behavior and maintained projections are plausible differentiators, but require device and scale acceptance.

### Plezy: a client reference, not another server architecture audit

Plezy is in the brief as a Flutter client for Plex/Jellyfin/Emby with an mpv player, downloads, companion remote, and Watch Together support. I did not re-audit its code or independently verify its current product matrix; other lanes do that. Its strategic relevance is that a strong interoperable client can compete without replacing a household's server. Portico cannot assume server ownership is the only route to adoption. Infuse and Symfonium independently establish that pattern from public sources.

### Plex Media Server, Plex Web, and mobile/TV apps

**Architecture and authority.** The owner installs PMS and stores media locally. Central Plex accounts unify server discovery, sharing, product entitlements, hosted Web, Discover, Watchlist, and social features. This is a cloud-dependent control layer, not a claim that Plex hosts every personal file. LAN exceptions and previously configured access can work without normal online sign-in; that is not the same as Portico's first-class local account and profile lifecycle. Server libraries and many playback operations are now covered by a published PMS API with explicit version headers. [Plex account requirements][P-account] [Plex published PMS API][P-api]

**Remote connections.** PMS attempts automatic router mapping using UPnP/NAT-PMP, with manual forwarding where necessary. Secure `plex.direct` names and certificates support browser-trusted direct connections; DNS rebinding protection and NAT loopback can interfere. Relay falls back when a direct connection fails: the official July 2025 article says end-to-end encrypted tunneling, a 2 Mbps maximum for both pass types, and no downloads over Relay. A paid entitlement is not a guarantee of high-bitrate relay. [Plex remote setup][P-remote] [Plex secure connections][P-secure] [Plex Relay][P-relay]

**Library and analysis.** Scanners identify files; agents populate metadata. Modern Plex Movie/Series/Music agents aggregate information from multiple services. Legacy configurable agent-source priority remains documented, but legacy-agent visibility changed around PMS 1.43; do not confuse it with the modern agent architecture. Music sources include MusicBrainz and AllMusic. Scheduled analysis includes chapter/preview imagery, intro/credits detection, loudness, voice activity for subtitle timing, and sonic features. Public docs expose these jobs; their private implementation cannot be reconstructed from a feature list. [Plex metadata agents][P-agents] [Plex scheduled tasks][P-tasks] [Plex published PMS API][P-api]

**Playback.** Direct Play, remux, software/hardware transcode, HDR-to-SDR tone mapping, stored optimized copies, subtitles, intro/credits controls, and mobile/desktop downloads are mature product surfaces. Plex Pass gates hardware acceleration and tone mapping in the cited docs; CPU conversion is not equivalent to a paid feature. Auto quality is an opt-in connection-speed response for converted streams, not automatic modification of a direct-play original. September 2026 transcoder docs also label HEVC encoding and optimization experimental. Player capabilities differ across apps and TV firmware. [Plex hardware acceleration][P-hardware] [Plex HDR tone mapping][P-tone] [Plex Media Optimizer][P-optimize] [Plex auto quality][P-auto] [Plex transcoder settings][P-transcoder]

**Users.** Plex Home offers managed users, four-digit PINs, fast switching, and a 15-member household limit. Managed users do not independently sign in with email/password. Full-account membership in Home and sharing restrictions have entitlement nuances. Friends can receive selected libraries and restrictions on rating/labels and other allowed activities. Portico already has comparable basic account/profile, invitation, library-policy, and PIN concepts; its exact policy model should be compared, not replaced by “add Plex Home.” [Plex Home][P-home] [Plex share restrictions][P-sharing]

**Commercial/discovery layer.** Plex adds ad-supported on-demand content and FAST channels, rentals (launched February 2024), universal streaming availability/search, Watchlist, and social activity. Some titles link outward to another service; discovery does not mean Plex can decrypt and play Netflix locally. PMS and the commercial catalog are distinct systems sharing an app. [Plex rentals launch][P-rentals] [Plex Discover][P-discover] [Plex Universal Watchlist][P-watchlist]

**Strengths.** Broad TV/device availability, easy account-based sharing, practical direct access and fallback, rich background analysis, specialist Plexamp, and a large automation ecosystem. The existence of Tautulli, Kometa, request services, and watch-state sync tools is part of Plex's practical value, even when Plex did not build those tools.

**Known complaints.** Cloud sign-in dependency, streaming clutter, entitlement surprises, missing functions during app rewrites, slow download work or format differences, and uncertainty about data use. These are not proof Plex sells personal library records: Plex explicitly says it does not sell/share personal-server viewing data in its March 2025 announcement. Discover/cloud watch state and advertising data require separate examination (see dislikes).

#### Plex changes that materially affect a 2026 comparison

| Date | Verified change and boundary |
|---|---|
| 7 Feb 2024 | Rental offering launched; location/device entitlement varies. [Plex rentals launch][P-rentals] |
| 6 Sep 2024 | Subtitle auto-sync announced; SRT and voice-analysis restrictions are documented. [Plex subtitle auto-sync][P-subs] |
| 22 Nov 2024 / 22 Jan 2025 | Redesigned mobile preview, then Apple TV preview. These were previews, not universal rollout. [Plex preview announcement][P-redesign] |
| 31 Mar 2025 | Mobile redesign began public rollout; official blog archive records date. [Plex mobile launch][P-mobile] |
| 29 Apr 2025 | Remote personal-video entitlement introduced. New mobile local playback no longer needs the old one-time unlock. Plex Pass became $6.99/month, $69.99/year, $249.99 lifetime at that time. Remote Watch Pass introductory $1.99/month or $19.99/year. Owner Plex Pass covers shared remote users; a viewer's Remote Watch Pass covers that viewer. [Plex 2025 changes][P-price25] [Plex remote requirements][P-entitlement] |
| 8 Sep 2025 | Plex announced a security incident involving account data, including hashed passwords and authentication data. This is a central-account risk, not evidence of stolen media files. [Plex security notice][P-incident25] |
| 29 Apr 2026 | Announcement updated for Fire TV/smart TV/console paywall expansion. Actual enforcement must be read per app/version. [Plex 2025 changes][P-price25] |
| 19 May / 1 Jul 2026 | Lifetime increased to **$749.99**; new recurring five-year option **$249.99**. Existing lifetime holders retained benefits; monthly/annual unchanged in that update. Reporting $249.99 as today's lifetime price is wrong. [Plex 2026 pricing][P-price26] |
| 3 / 10 Jun 2026 | Social-platform announcement followed by public-discussions launch. Predictive ratings and upgraded Lists in the announcement were “coming soon”; no blanket assumption that every promised function shipped. [Plex social announcement][P-social26] [Plex discussions launch][P-discussions] |
| 28 Sep 2026 | Official 2026.19 mobile/TV notes: show-specific preferred audio/subtitle languages, changed library navigation, and low-end grid improvements. [Plex September release][P-release26] |
| Sep 2026 | Official forum discussion indicates remote requirements reaching third-party clients including Infuse; owner-pass exception remains important. Support page lag/wording makes exact rollout coverage a verification boundary. [Plex third-party rollout discussion][P-thirdparty] |

The May 2026 pricing roadmap promised restored music/photos in redesigned mobile/TV apps, mobile playlist editing, NFO support, broader management, and IPv6. Those promises should not be recorded as absent forever or as shipped without a later release record. The API **has** shipped: the developer portal documents its versioned public release. [Plex 2026 pricing][P-price26] [Plex published PMS API][P-api]

### Plexamp in detail: the music experience to beat

Plexamp is a specialized player backed by PMS rather than a standalone NAS file browser. Basic streaming became free in July 2023; remote personal-music streaming remains described as free in Plex's current free/paid page. The 2025 video remote paywall must not be generalized to all music. Its Apple/Android/desktop availability and concentrated music UI avoid making listeners navigate a video-first app. [Plexamp free launch][PA-free] [Plex free versus paid][P-free]

The published feature split assigns mixes, sonic similarity, Sonic Adventure, Guest DJ, downloads, lyrics, autoplay, home customization, ten-band EQ, radio, sample-rate matching, and headless Raspberry Pi operation to Plex Pass, while basic playback, visualizers, casting, and car integrations have free portions. Treat that as the vendor's feature split; I did not install or exercise it. [Plexamp feature split][PA-features]

**Sonic analysis** runs locally against actual audio using a neural network, placing songs in a multidimensional similarity space. It is not AcoustID identification, tags, genre overlap, or ReplayGain. Obscure/local recordings can participate even when no online metadata exists. The cited support page warns of hours/days of CPU work and platform restrictions; some platform language is visibly old and should not be presumed unchanged on all 2026 processors. [Plex sonic analysis][PA-sonic]

**Radio and queue invention** matter as much as decoding. Artist/album/track radio, mix builders, similar items, Guest DJ insertion, and Sonic Adventure provide listening paths through a collection. Aural Fixations summarizes listening habits. Downloads and cached listening reduce mobile-network friction. Sweet Fades are music-aware transitions; Portico's fixed crossfade is not proven equivalent. Ordinary shuffle, repeat, gapless, gain leveling, and lyrics are already Portico features (PC5/PC7), so the gap is intelligent selection and specialist depth.

**Audio and endpoints.** Headless operation, remote control, car UIs, EQ, sample-rate behavior, and visualizers give Plexamp reach into daily listening. Roon and Symfonium set an even higher output/DSP bar. File decoding, lossless PCM, a bit-perfect hardware path, and preservation of Atmos objects are different claims. A technically correct server FLAC stream can still be resampled by the device. Portico should expose what it knows rather than advertise unspecified audiophile parity.

**Caveats.** Proprietary analysis/model, premium features, opaque recommendation taste, platform availability, and a separate app that is inconvenient on a TV lacking Plexamp. A 2025 forum complaint about removed music on redesigned Roku illustrates that splitting a specialist app can leave a platform stranded. [Plex missing music report][P-musiccomplaint]

### Infuse / Firecore

**Core architecture.** Infuse can be a complete player/library without a media server: it reads local files, NAS shares over SMB/NFS/FTP/WebDAV, and supported cloud drives; fetches descriptive data/art from TMDB; and decodes on the Apple device. No mandatory Firecore server installation is required. It can instead connect to Plex/Emby/Jellyfin, using their metadata and watched state. Apple purchase/iCloud services and optional Trakt/cloud connections are dependencies, but local file playback is not the same cloud-account architecture as Plex. [Infuse product and prices][I-main] [Infuse server modes][I-server]

**Two catalog modes.** Library Mode builds a local combined catalog across sources, with filters and offline browsing. Direct Mode fetches server records as needed, reduces client storage, and avoids full catalog synchronization; Firecore recommends it for larger libraries. These are specifically media-server connection modes, not synonyms for SMB transport. InfuseSync is recommended for Library Mode with Emby/Jellyfin. There is still a source refresh schedule in the cited article. [Infuse server modes][I-server]

**Engine.** The vendor lists many containers and subtitle formats, DVD/Blu-ray folder/disc-image handling, HDR/Dolby Vision, and lossless audio decoding. Exact Dolby Vision profile, enhancement-layer treatment, device generation, subtitle styling, and output route matter. The May 2026 Audio Options article supports E-AC3 Atmos and multichannel LPCM, explains TrueHD/DTS-HD decoding to PCM, and says TrueHD Atmos/DTS:X flexibility still depends on Apple. Do not translate “TrueHD decoding” to “TrueHD Atmos bitstream.” [Infuse store formats][I-formats] [Infuse audio output][I-audio]

**2026 correction.** Infuse 8.5, dated 30 July 2026 in official notes, added transcoding options for Emby/Jellyfin/Plex. Subsequent notes cover version selection, track changes during conversion, unstable-network handling, and background downloads. Infuse itself does not thereby become a general server-side transcoder; it can ask an attached server to convert when useful. “Never transcodes” is now misleading. [Infuse release history][I-releases]

**Sync.** iCloud carries saved shares, layout, matches, selected tracks, subtitles, playlists, and collections; Pro adds watched/progress/rating state. Same iCloud identity is required. Plex/Emby Connect shares need per-device setup. Trakt outbound sync is available in free Infuse; two-way standalone sync is Pro. With a media-server connection, the server remains watched-state authority and Trakt history is not pulled back. [Infuse iCloud sync][I-cloud] [Infuse Trakt sync][I-trakt]

**Pricing.** Current public product page: $1.99/month, $16.99/year, or $99.99 lifetime. Pro covers additional formats/audio, cloud support and advanced sync. The App Store listing and family-sharing support expose nuances; a free install is not the full engine entitlement. [Infuse product and prices][I-main] [Infuse purchases and sharing][I-purchase]

Strength: a polished powerful Apple player for people who already have storage, and a way to improve Plex/Jellyfin playback without migrating. Complaints: subscription/lifetime price, Apple-only reach, initial metadata/artwork fetch delay, local rescan/sync complexity, and output constraints inherited from Apple. Direct Mode addresses a real catalog-sync burden, rather than proving every user should maintain a local catalog.

### Emby Server, apps, and Premiere

Emby remains a local-server product with local users, parental policies, file libraries, provider metadata, apps, transcoding, DVR, and plugins. Optional Emby Connect maps an online identity to local server users and locates servers. Official docs explicitly say it is not a streaming relay; clients connect directly. Cloud-account dependency is consequently less absolute than Plex's normal account workflow. [Emby Connect boundary][E-connect] [Emby architecture and apps][E-about]

Premiere is a server-installed key with device/household terms, not merely a purchase of a player UI. Published prices are $4.99/month, $54/year, $119 lifetime, with a standard 30-device household limit and extended options. The feature matrix separately distinguishes server-level features, browser playback, mobile unlock, desktop/console playback, and some TV rules. Hardware, HDR tone mapping, DVR, downloads/conversion, intro skipping, webhooks, CoverArt, backup plugin, LDAP, and several client options are premium in the matrix. Exact app and version matter. [Emby Premiere pricing][E-price] [Emby feature matrix][E-matrix]

Strengths: no commercial streaming catalog, local-user/server model, broad apps, useful metadata/plugins, and established DVR. Complaint opportunity: confusing unlock-versus-server-pass/device coverage, paid essentials, and proprietary development. Emby's own history article dates the move from open source to a proprietary model to 2018; old MediaBrowser code is not evidence of today's server. Its private engine and performance internals were not audited. [Emby proprietary transition][E-history]

### Channels DVR

Channels is a television-first local DVR server plus polished clients, subscribed at $8/month or $80/year in current official docs. It supports HDHomeRun, remote recording/live playback, series passes, commercial detection, personal media, and Virtual Channels. TV Everywhere logs into supported providers to access network streams; it does not guarantee every channel in a paid cable bundle or bypass DRM. Cloud subscription/guide/authentication services augment locally stored recordings. Remote playback requires actual connectivity, historically port forwarding; no universal vendor relay was established. [Channels subscription][C-price] [Channels TV Everywhere][C-tve] [Channels Virtual Channels][C-virtual] [Channels remote setup discussion][C-remote]

Its strength is television operations: guide, recurring recording, time shifting, tuners, and commercial-skip workflows as the core product rather than an annex to films. Portico already has these broad categories; the comparison should examine conflict handling, chase play, channel surfing, and commercial review depth. The January 2026 enhanced commercial detector was announced as prerelease/opt-in, with preview and enabled modes; this is not evidence that all installs use it. [Channels enhanced detection beta][C-commercial]

Known traps: subscription plus separate TV-provider costs, channel disappearances, provider-login automation breakage, DRM, and maintenance of a specialist client fleet. July/August 2026 forum threads cite removal of CNN-family streams due to DRM and Spectrum authorization problems. Those are source-availability failures, not evidence Channels can always recover them. [Channels TVE/DRM failures][C-complaint]

### Kodi

Kodi is a media center/player, not principally a household HTTP server. Each installation has a local SQLite library, scrapers, artwork cache, settings, skin, and add-ons. Shared MySQL/MariaDB libraries are an optional arrangement; SMB/NFS/file shares remain the source. PVR functionality is commonly a front end to an external backend. No vendor account or relay is necessary for the basic local application. [Kodi database model][K-db] [Kodi syncing and sharing][K-share] [Kodi add-ons][K-addons]

Skins can reshape navigation/density almost completely. Add-ons include metadata, PVR, subtitles, music and other integrations; this is flexible executable extension code, unlike Stremio's usual remote add-on protocol. That flexibility creates setup, maintenance and trust burdens. Not all add-on content is legal or supported; neither Kodi's capabilities nor piracy-associated addons establish Portico should distribute such integrations.

NFO is the major interoperability reference. It is an ecosystem XML convention with different tags for movies, shows, episodes and music, not one universally enforced schema. Kodi movie NFO documents identities, sets, cast/art, playcount/lastplayed, and resume information. Portico reads a bounded subset. Metadata import/export should not confuse public title facts with one viewer's private watched state. [Kodi NFO convention][K-nfo] [Kodi movie NFO tags][K-movie]

Strength: open local player, flexibility, no obligatory server, community skins, disc/remux playback on suitable hardware, reusable sidecars. Complaints/traps: configuration sprawl, skin-dependent discoverability, broken add-ons, device-specific passthrough, shared-database maintenance, and unclear separation of file/library modes. These are architectural burdens, not measured current defect rates.

### Apple TV app, Netflix, Disney+: UX conventions only

These references are useful for Continue Watching, profile selection/PINs, clear resume versus restart, subtitle accessibility, predictable next-episode countdown, “still watching,” download visibility, progress, and library/streaming-service separation. They are managed commercial catalogs with device certification, DRM, content licenses, and different telemetry/economics; their private recommendation/transcoding infrastructure is not a fair implementation blueprint for a one-owner self-hosted server.

Apple documents personal recommendations, offline purchases/Originals, subtitle appearance, and profile passcodes. Netflix documents inactivity prompts and hiding activity to remove recommendation/Continue Watching signals. Disney+'s 2026 help page on Hulu linking demonstrates importing profiles/history/My Stuff with exceptions and parental boundaries. This is a useful adoption lesson: migration is a product feature, and permission semantics survive the move. Do not infer universal DRM-free file import or ongoing GroupWatch from obsolete summaries. [Apple TV personalization][UX-apple] [Netflix inactivity convention][UX-netflix] [Netflix hidden history convention][UX-history] [Disney/Hulu profile migration][UX-disney]

### Adjacent ecosystem: product profiles and Portico integration implications

| Tool | Public model and strengths | Dependencies, costs, and Portico implication |
|---|---|---|
| Overseerr / Jellyseerr / **Seerr** | Request portal: discover titles absent from the library, user quotas/approvals, service routing, notifications. Official merger announced 10 Feb 2026; Seerr now combines Plex/Jellyfin/Emby support. [Seerr features][S-seerr] [Seerr merger announcement][S-seerrmerge] | Self-hosted portal plus media-server and Sonarr/Radarr credentials. Existing-instance automatic migration has backup/permissions requirements. Portico has My List, not a verified acquisition-request state machine. Adapter support is a smaller strategic step than cloning the product. |
| Tautulli | Separate Plex activity/history/stats service, reports, notifications, scripts, metadata exports, API. [Tautulli overview][S-tautulli] [Tautulli API][S-tautapi] | Reads privileged PMS information and retains its own history. Portico already records owner play history and stats; gap is integration/report depth, not “add analytics.” |
| Jellystat | Separate Jellyfin/Emby statistics service; published releases include session details, adaptive polling, webhooks, materialized-view fixes. [Jellystat releases][S-jellystat] | Additional service/database and sensitive playback history. Portico's maintained counts can eliminate a separate collector for common questions. API contract/exports still matter. |
| Trakt / Simkl | Cloud watch history, lists, progress/discovery/social tracking and app ecosystems. Simkl explicitly spans TV/anime/movies. [Trakt scrobbler announcement][S-trakt] [Simkl tracking ecosystem][S-simkl] | Optional account and sometimes premium sync; remote personal-viewing data leaves the server by user choice. Need per-profile mapping, retry deduplication, deletion/unwatch semantics, and provider ID mappings. |
| Sonarr / Radarr | Organize/rename/import episodes/films and emit events on import/upgrade/rename/deletion. They can write Kodi-compatible metadata/assets. [Sonarr events and metadata][S-sonarr] [Radarr events and metadata][S-radarr] | They own acquisition/file organization, not playback. Portico should accept authenticated completion notifications and scan only the affected trusted path. “On Grab” is too early. File path translation between containers is essential. |
| Bazarr | Manages subtitle languages/providers and can align downloaded text to audio or embedded subtitles. [Bazarr subtitle processing][S-bazarr] | Extra worker/credentials and filesystem writes; Portico already discovers sidecars. Integration can be file-change invalidation, not embedded downloader code. Optional timing alignment is a real gap. |
| Kometa, formerly Plex Meta Manager | Declarative collection/playlist/metadata/overlay automation, external lists and reusable defaults. [Kometa overview][S-kometa] [Kometa rule files][S-kometafiles] | API/artwork edits, external metadata and scheduling. Portico has collections/saved views, but no verified Kometa adapter or overlay automation. Offer stable authoring/export APIs and preserve original art. |
| Tdarr / Unmanic | Library optimization pipelines, workers/plugins, health checks; Tdarr documents mapped and unmapped nodes, with Pro constraints. [Tdarr worker types][S-tdarr] [Unmanic workflows][S-unmanic] | May replace/remux original files. They are not live-stream transcoding substitutes. Portico needs robust in-place file replacement and invalidation; distributed live encoding is a separate larger problem. |
| ErsatzTV / Tunarr | User-programmed linear TV from files/server libraries, schedules/filler, M3U/XMLTV and HDHomeRun-compatible output. [ErsatzTV playout instructions][S-ersatz] [Tunarr streams and sources][S-tunarr] [Tunarr XMLTV export][S-tunarrepg] | Persistent playout and transcoding resources. Portico already implements Library Channels and can ingest external M3U/XMLTV. Channel export and advanced station scheduling remain strategic options. |
| Audiobookshelf | Dedicated audiobook/podcast server; metadata lookup, chapter editing, merge-to-M4B/tagging, progress/downloads, RSS, basic ebooks/send-to-device. [Audiobookshelf introduction][S-abs] | Local server with optional providers; companion apps' maturity varies. Portico already has book/author/series/audio playback. Podcast subscriptions, richer book metadata and specialist chapter workflows are the delta. |
| Kavita / Komga | Dedicated ebook/comic/manga servers with format-specific readers, progress, reading lists, ComicInfo/EPUB metadata, OPDS/page streaming. Kavita offers optional premium enrichment. [Kavita product][S-kavita] [Komga product][S-komga] [Komga OPDS client matrix][S-opds] | Reading progress is page/chapter/location, not media seconds. Adding an ebook extension to a video scanner is insufficient. External-reader OPDS is an interoperability option. |
| Immich / PhotoPrism | Personal photo/video managers: external libraries, indexing, albums, metadata/search; Immich adds mobile backup, faces, semantic search, maps, Live Photos and partner sharing. PhotoPrism supports originals and WebDAV workflows. [Immich features][S-immich] [PhotoPrism documentation][S-photo] | Much more than a slideshow: privacy-sensitive faces/GPS, duplicate handling, uploads and machine-learning jobs. Consider read-only display/integration first; a full backup replacement competes with a specialist product. |
| Stremio | Catalog/stream-source aggregator with official/community remote addons and account sync; guest mode is documented. Local files/HTTP are supported paths. [Stremio features and guest/addon model][S-stremio] | A remote catalog/source protocol differs from running arbitrary server plugins. Add-on trust, provider durability, and jurisdiction/content legality remain separate concerns. Users can discover before owning a server. |
| VidHub / SenPlayer / Fileball (listing now Filebar) | Apple-oriented local/NAS/cloud/multi-server players and libraries; official/app-store sources list SMB/WebDAV and Plex/Emby/Jellyfin. [VidHub product scope][S-vidhub] [SenPlayer store description][S-sen] [Fileball/Filebar store description][S-fileball] | Sources prove advertised scope, not claimed 8K/Atmos/frame-rate reliability. They broaden the “just a player” market. Small publishers, changing names, regional drives and closed engines create support/dependency risk. |
| nPlayer / VLC | File/network-first players, broad formats/subtitles and local playback; nPlayer offers paid editions, VLC is free/open. [nPlayer features][S-nplayer] [VLC features][S-vlc] | They solve “open this file/URL” without library administration. Portico can add an open-file action separately from a fully synchronized serverless catalog. |
| Symfonium | Paid Android client combining Plex/Jellyfin/Emby/OpenSubsonic/local/SMB/cloud and audiobook sources, offline caching, car/cast and advanced DSP. [Symfonium product features][S-sym] [Symfonium backend feature matrix][S-symproviders] | No subscription in current vendor description; backend fields differ. Full client sync/offline customization differs from Portico's thin live projections. An OpenSubsonic compatibility surface would immediately widen client choice. |
| Roon | $14.99/month, $12.49/month billed annually, or $829.99 lifetime. Paid local music server plus remotes/RAAT endpoints and ARC; rich credits/composition/version links, transparent audio signal path and DSP. [Roon pricing/product scope][S-roon] [Roon signal path][S-roonpath] [Roon ARC FAQ][S-roonarc] | Vendor accounts/licensing and optional streaming subscriptions; separate endpoint ecosystem. ARC uses the home server. Roon 2.0's always-online requirement was removed in 2.0.28 (Feb 2024); repeating that old complaint as current is wrong. [Roon offline restoration][S-roonoffline] |

## Architectural differences

Sizes describe engineering scope, not a schedule: **small** is one bounded interface or presentation change; **medium** adds a subsystem or several clients; **large** changes authority, storage, protocol, or platform strategy. Closed-source internal cost estimates are inferences. Each assessment is my opinion; Justin's product decisions govern.

### A-23.1 Optional Hosted identity versus Plex's normal central account

- **Portico:** Local account/profile authority remains first-class; optional Hosted assertions connect an account to servers (PC2).
- **Plex / Emby / Infuse:** Plex's normal sharing and app sign-in use its account service; Emby Connect is optional discovery; Infuse can read files without a media-server account. [Plex account requirements][P-account] [Emby Connect boundary][E-connect] [Infuse product and prices][I-main]
- **Why it matters:** A cloud outage or changed vendor entitlement should not prevent a household from administering its own users. Local exceptions that bypass authentication are a worse substitute for local identity.
- **Assessment: Portico ahead** for owner autonomy, deliberate (PC1). Preserve local lifecycle and clarify which conveniences need Hosted. Changes are small documentation/acceptance work unless the implementation actually violates it. Plex is ahead in recognition and cross-device setup polish; that would change the practical verdict for nontechnical households.

### A-23.2 Certificates and discovery are not a relay

- **Portico:** Hosted offers a signed directory, direct names and DNS-01 certificates, but not transport around NAT (PC3).
- **Plex:** `plex.direct`/certificates support secure direct access; Relay can bridge an unavailable direct route. [Plex secure connections][P-secure] [Plex Relay][P-relay]
- **Why it matters:** “Your server has a trusted hostname” does not solve carrier-grade NAT, blocked inbound ports, or insufficient uplink. Both products must explain DNS rebinding and route selection.
- **Assessment: Trade-off (subjective).** Deliberate cheap Hosted boundary. Keep certificate assistance; add a relay only as a **large** separately costed transport service. The free direct path should remain independent. A target audience on CGNAT would make this a launch-critical gap.

### A-23.3 Relay availability versus relay quality

- **Portico:** Port forwarding or no remote reach today; a future paid relay must define capacity and quality (PC3/PC15).
- **Plex:** Official Relay caps streams at 2 Mbps and does not carry Downloads; it is a fallback rather than original-quality remote transport. [Plex Relay][P-relay]
- **Why it matters:** Users can mistake successful connectivity for successful quality. Remote entitlement, direct reach, relay limits and encoding capability are separate states.
- **Assessment: Unclear.** Portico lacks fallback convenience but can offer a clearer paid service. **Large** work in tunnel operations, congestion, grants, accounting and UI. Preserve original quality until the viewer opts into a constrained rendition; show relay status and limits. Do not copy Plex's quality cap without a deliberate business decision.

### A-23.4 Remote-payment enforcement versus infrastructure payment

- **Portico:** Free software/direct access; only possible future relay is paid (PC1/PC3, decisions P22).
- **Plex / Emby:** Plex gates remote personal video even when its servers do not transport the media; Emby monetizes server/client capabilities. [Plex remote requirements][P-entitlement] [Emby feature matrix][E-matrix]
- **Why it matters:** Charging for bandwidth supplied versus permission to use one's own bandwidth creates a very different trust proposition. Entitlements also enter login/playback failures and third-party compatibility.
- **Assessment: Portico ahead** on simplicity and owner control, deliberate. Keep paid hosted services separate from local core capabilities; **medium** entitlement/service design if relay launches. Sustainability is unresolved: a generous free core still needs client-store, certificate, support and security funding.

### A-23.5 Serverless playback is a different product, not another mount

- **Portico:** Server-mounted SMB/rclone/WebDAV feeds its authoritative catalog. Native apps are server clients (PC7; mounts configuration; K).
- **Infuse / Kodi / VLC / VidHub:** Client reads a share/file directly, owns browsing/playback and sometimes local metadata. [Infuse product and prices][I-main] [Kodi syncing and sharing][K-share] [VLC features][S-vlc] [VidHub product scope][S-vidhub]
- **Why it matters:** A NAS owner may want an excellent TV player without another daemon. Thin clients currently lack independent catalog, source credentials, identity and saved-state authority.
- **Assessment: Trade-off (subjective).** **Large** for a full player-only library/sync product; **medium** for open-file/URL playback. Keep explicit modes so local files do not quietly acquire server restrictions or corrupt server history. If Portico's target is dedicated self-hosters, omit the full mode; if player acquisition is the entry point, it deserves a separate architecture.

### A-23.6 Infuse's local combined library versus on-demand server mode

- **Portico:** Bounded server projections, clients cache visible pages; server sorting/filtering is authoritative (PC1/PC7).
- **Infuse:** Library Mode combines source catalogs locally; Direct Mode asks the server for needed metadata without full pre-caching. [Infuse server modes][I-server]
- **Why it matters:** Offline browse and cross-source blending trade memory/sync cost against instantaneous updates and scale. Infuse itself offers both because neither dominates.
- **Assessment: Portico ahead** for the intended live huge-library path; deliberate. **Medium–large** to add an explicit offline catalog subset or multiple-server unified view. Retain on-demand default. Offline travelers or households spanning several servers may prefer the other model.

### A-23.7 One chosen provider versus one aggregating agent

- **Portico:** One online screen provider per library, plus local evidence/overrides (PC4); documented as deliberate.
- **Plex / Jellyfin / Kodi:** Plex's modern agent can aggregate several upstream sources; legacy agent source priorities and Jellyfin provider/plugin arrangements are not the same as one upstream. [Plex metadata agents][P-agents] [Jellyfin plugins][J-plugins] [Kodi NFO convention][K-nfo]
- **Why it matters:** “One agent” and “one provider” are different boundaries. A single-provider policy simplifies provenance and consistency but may lose regional metadata, missing credits/art or music editorial enrichment.
- **Assessment: Trade-off (subjective).** Do not copy a fallback chain casually; **large** provenance/refresh policy change. Preserve the deliberate rule but document the coverage cost. A catalog where the chosen provider is consistently incomplete would justify reconsideration.

### A-23.8 Local NFO evidence versus provider-owned descriptions

- **Portico:** Strict bounded XML input with local evidence and locks; movie/show/episode roots and edition fields parsed (PC4).
- **Kodi / Emby / Jellyfin / Plex:** Kodi NFO is a broad interchange convention, adopted by other servers and *arr writers. Plex's May 2026 roadmap still listed NFO support. [Kodi movie NFO tags][K-movie] [Sonarr events and metadata][S-sonarr] [Radarr events and metadata][S-radarr] [Plex 2026 pricing][P-price26]
- **Why it matters:** Owners preserve curated art/descriptions and titles missing from public providers. Full round trip is more useful than partial import; playback state is separate private evidence.
- **Assessment: Portico ahead** over verified Plex baseline, **Unclear** on ecosystem completeness. **Medium** export/interoperability fixtures, not another parser from scratch. Keep bounds and provenance. Source-level subset compatibility needs explicit documentation; adoption experience would change the verdict.

### A-23.9 Intro detection: episode similarity versus scene hypotheses

- **Portico:** Chapter/luminance candidates with low confidence and owner approval; not automatic-safe by detector claim (PC11).
- **Plex:** Matches repeated early audio across episodes within a season, with documented exclusions and imperfect accuracy. [Plex intro detection][P-intro]
- **Why it matters:** A button can exist while the owner's usable marker set is nearly empty. Both false skips and endless manual review are product costs.
- **Assessment: Portico behind** in demonstrated automation depth. **Large** season-wise matching and sample evaluation; **medium** external marker import. Keep reviewed marker safety. Accurate recall on a varied held-out corpus would change the judgment, not the presence of an enum or preference.

### A-23.10 Credits detection and post-credit preservation

- **Portico:** End-title luminance creates a candidate extending to duration; owner review/editing exists (PC11).
- **Plex:** Credits data can include extra-scene boundaries, post-play minimization, watched threshold policy, and hash-keyed cloud reuse. [Plex credits detection][P-credits]
- **Why it matters:** Skipping everything after the first credits can erase story content. Cloud reuse saves repeated CPU work but creates a new central dataset and privacy boundary.
- **Assessment: Portico behind** on demonstrated segmentation; **Trade-off** on cloud submission. **Large** richer detector/model, **medium** multi-interval markers/import and post-play behavior. Avoid automatic upload by default. Library-specific accuracy and privacy requirements determine whether a central service is worthwhile.

### A-23.11 Voice activity as reusable subtitle alignment evidence

- **Portico:** Subtitle offsets exist, including web ±0.5-second controls; no named voice-activity/automatic sync path found (K; `web/src/player/Player.tsx:553`).
- **Plex / Bazarr:** Plex analyzes primary audio, then aligns compatible external SRT during playback. Bazarr can analyze audio/embedded subtitles after download. [Plex subtitle auto-sync][P-subs] [Bazarr subtitle processing][S-bazarr]
- **Why it matters:** Timing fixes should survive seeks and differing cuts without rewriting original evidence invisibly. Manual offset is not drift or frame-rate correction.
- **Assessment: Portico behind.** **Medium–large** background alignment stages and player selection UI, compatible with measured/provenance architecture. Keep an offset fallback, disable alignment for mismatched cuts, and make accessibility captions optional inputs rather than stripping them. Demand real multilingual/edition tests.

### A-23.12 Loudness/trim are not sonic similarity

- **Portico:** Measures gain, raw frames, encoder delay and padding; has generic spectral fingerprints, not a sonic music model (PC5/PC11/PC14).
- **Plexamp:** Neural audio features power similarity and radio even without useful tags. [Plex sonic analysis][PA-sonic]
- **Why it matters:** Matching the song's identity, maintaining its perceived level, and choosing a musically related next song solve different problems.
- **Assessment: Portico behind** for music discovery, while its existing trim/gain foundation deserves credit. **Large** model choice, background feature store/index, radio ranking and QA; compatible if compute is optional and bounded. Tag-only radio may deliver useful value sooner. Do not describe spectral-signatures as equivalent to Plex sonic analysis.

### A-23.13 Scheduled heavy analyses versus always-deprioritized work

- **Portico:** Principle says background work is deprioritized, never paused; source has scan policy tiers and deep-analysis cancellation when playback is busy (`ingestion/deep_analysis.go:45`).
- **Plex:** Owner maintenance windows and per-feature on-add/scheduled choices control analysis. [Plex scheduled tasks][P-tasks] [Plex sonic analysis][PA-sonic]
- **Why it matters:** Full waveform/loudness/voice/model scans consume energy and remote-file bandwidth. On a continuously used source, cancellation/retry can differ sharply from “slow but steady.”
- **Assessment: Unclear.** Source/document reconciliation needed before advertising the rule. **Small** wording if intentional; **medium** scheduling/accounting if behavior must change. I did not establish starvation or a bug. Preserve owner-visible queues, costs and progress regardless of policy.

### A-23.14 Stored optimized copies: lifecycle matters more than existence

- **Portico:** Prepared-media jobs, leases, quotas/free-space guards and conversion APIs already exist (PC5; `preparedmedia/worker.go:101`).
- **Plex / Emby / Tdarr:** Plex optimizes selected query results, Emby offers scheduled/on-add content conversion, Tdarr replaces or creates files through workflows. [Plex Media Optimizer][P-optimize] [Emby Premiere pricing][E-price] [Tdarr worker types][S-tdarr]
- **Why it matters:** “Convert once” can avoid live CPU, but selection rules, stale-original invalidation, rendition choice, subtitles and cleanup are the actual product.
- **Assessment: Trade-off (subjective).** Portico is not missing optimized versions wholesale. **Medium** persistent conversion rules/UI if wanted; **large** distributed optimization. Preserve originals and explicit storage cost. Automatic version selection and sustained policies need specialist validation.

### A-23.15 Fixed user-requested quality versus bandwidth adaptation

- **Portico:** Quality changes only when requested; network ceilings are settings, no ongoing auto-adaptation proven (PC1/PC5/PC7).
- **Plex / Infuse:** Plex opt-in auto quality changes converted video/audio with bandwidth. Infuse 8.5 adds server-transcoding options; direct files still depend on network buffering. [Plex auto quality][P-auto] [Infuse release history][I-releases]
- **Why it matters:** Mobile users prefer continuous playback; enthusiasts prefer no silent downgrade. Server overload and weak client bandwidth are different causes.
- **Assessment: Trade-off (subjective), deliberate.** **Large** real adaptive ladder/session switching, **small–medium** quality suggestions requiring consent. An explicit viewer-selected adaptive mode could preserve owner-choice values, but conflicts with a literal prohibition and needs Justin's decision.

### A-23.16 Broad client decoding versus server-side fallback

- **Portico:** AVPlayer/Media3/HLS capabilities inform server conversion; custom audio handles gapless (PC5/PC7).
- **Infuse / Kodi / VLC:** A broad local engine handles containers/subtitles/audio without making the NAS convert. [Infuse store formats][I-formats] [Infuse audio output][I-audio] [VLC features][S-vlc]
- **Why it matters:** A player improvement can eliminate many transcodes, reduce server cost and preserve subtitle styling. Licensing/platform constraints remain even with a universal decoder.
- **Assessment: Unclear** until a format/device acceptance corpus exists. **Large** integrating and maintaining a broader video engine across platforms. Prefer targeted client improvements when the actual missing codec/subtitle is known; don't advertise parity from FFmpeg capability alone.

### A-23.17 Hardware/HDR capability is not a premium permission

- **Portico:** Hardware and tone-map policy are owner capabilities/settings, with fallback rather than a subscription unlock (PC5).
- **Plex / Emby / Jellyfin:** Plex/Emby premium gates contrast with Jellyfin's free GPU pipelines. [Plex hardware acceleration][P-hardware] [Plex HDR tone mapping][P-tone] [Emby feature matrix][E-matrix] [Jellyfin GPU pipelines][J-hardware]
- **Why it matters:** Weak hardware particularly benefits from a GPU. Platform capability, driver availability and entitlement should be separately explained.
- **Assessment: Portico ahead** on access, **Unclear** on actual GPU/HDR breadth. Deliberate free model. **Medium** acceptance/error reporting, potentially **large** missing backend paths. Keep clear diagnosis rather than claiming all hardware is interchangeable.

### A-23.18 Lossless decode versus transparent final signal path

- **Portico:** Delivery diagnostics include codec/bitrate and audio trim/gain facts; a complete DAC/OS signal path is not established (PC5/PC7; K).
- **Roon / Infuse / Symfonium:** Roon explains transformations/output devices; Infuse documents LPCM/Atmos limits; Symfonium exposes high-resolution DSP/output controls. [Roon signal path][S-roonpath] [Infuse audio output][I-audio] [Symfonium product features][S-sym]
- **Why it matters:** Resampling, gain, crossfade and OS mixing can change output despite an original or FLAC source.
- **Assessment: Portico behind** on audiophile transparency. **Medium** output facts/diagnostics; **large** exclusive mode, rate matching and DAC integration. Keep honest “unknown after OS handoff” states. Accurate route reporting is more valuable than an unverified bit-perfect badge.

### A-23.19 Downloads: selection versus rolling policy

- **Portico:** Container requests support all/unwatched/next and keepNext, with bounded capture/admission. The inspected request advances only while capturing/admitting (`downloads/requests.go:320`).
- **Plex / Plexamp / Symfonium:** Downloads/caching have distinct app/premium models; Plex's May 2026 roadmap promised automatic new-episode downloads. [Plex 2026 pricing][P-price26] [Plexamp feature split][PA-features] [Symfonium product features][S-sym]
- **Why it matters:** Selecting the next five episodes once is not a durable rule that removes watched files and replenishes them forever. Client disk, offline authority and new media revisions complicate that promise.
- **Assessment: Unclear** on full rolling lifecycle; do not claim Portico lacks next-N selection. **Medium–large** persistent desired-state policy across server/client. Good mobile value if bounded and explicit; existing protocol auditors should establish completion/refill semantics first.

### A-23.20 Watch Together survives a redesign only if it remains a contract

- **Portico:** Explicit server timeline, group/queue/permission/handoff contracts and routes (PC8).
- **Plex:** Watch Together remains documented, but a 25 Feb 2025 notice removes it from most redesigned apps and retains Web for the foreseeable future. [Plex Watch Together notice][P-together]
- **Why it matters:** A feature checklist that says both “yes” hides a major app-coverage difference. Group roles/reconnect/individual tracks matter as much as synchronizing pause.
- **Assessment: Portico ahead** in preserving intent, **Unclear** on device reliability. **Medium** cross-platform acceptance. Keep it if supported without a cloud media-history store. Actual drift, reconnection and platform coverage would change this verdict.

### A-23.21 PIN profiles and household boundaries

- **Portico:** Account sessions select ordered PIN profiles, library rules and trust tokens; eight profiles per direct account (PC13/API README).
- **Plex:** Fifteen members per Home, managed users and full-account switching with different login/entitlement semantics. [Plex Home][P-home]
- **Why it matters:** Limits count different things; eight profiles per account is not a global eight-user server limit. A profile PIN is a household convenience, not replacement for account security.
- **Assessment: Trade-off (subjective).** **Small** explicit capacities/UI; **medium** alternative household model. Portico should retain its clear account/profile distinctions and test fast switching while media/downloads continue. Large families or separate adults may prefer different semantics.

### A-23.22 Sharing restrictions as authoritative query policy

- **Portico:** Server membership and content/library fences scope reads and streaming; sharing saved resources exists (PC2/PC6/PC8).
- **Plex / Emby:** Library shares, managed users and content restrictions are established; Plex labels/ratings and Emby parental controls offer product-specific policies. [Plex share restrictions][P-sharing] [Emby feature matrix][E-matrix]
- **Why it matters:** Restrictions must cover search art, recommendations, playlists, download grants and channel guides, not just hidden cards.
- **Assessment: Unclear** on equivalent rule breadth. **Medium** if adding allow/deny labels or schedules; retain restrictions in indexed queries. No wholesale sharing gap was found. Exact parent/household policy needs should guide changes, not copying another service's terms.

### A-23.23 Server-local discovery versus the whole entertainment market

- **Portico:** Discover ranks accessible owned libraries; My List is saved personal media, not a cloud availability database (PC6).
- **Plex / Apple:** Universal search/Watchlist and provider availability span content not on the server. [Plex Discover][P-discover] [Plex Universal Watchlist][P-watchlist]
- **Why it matters:** Owned-title IDs and abstract works need separate modeling when a wanted title has no file. Regions, service deep links, changing rights and unavailable items introduce new state.
- **Assessment: Trade-off (subjective), deliberate scope.** **Large** availability/commercial dataset service; **medium** external-work request portal. Keep local discovery uncluttered. A family seeking one “where can I watch?” app might choose Plex irrespective of server quality.

### A-23.24 Public social graph versus opted-in household signal

- **Portico:** Community activity is server-scoped/opt-in and recommendations work with local identity; owner's audit history is independently retained (PC6/PC9).
- **Plex / Trakt / Simkl:** Cross-service cloud watch state, friends, reviews/lists and discussions can share taste beyond a household. [Plex social announcement][P-social26] [Plex cloud watched-state sync][P-sync] [Trakt scrobbler announcement][S-trakt] [Simkl tracking ecosystem][S-simkl]
- **Why it matters:** Publication, account linkage, delete/retract and children's visibility are new authorities; “private recommendations” and “public activity” cannot be one checkbox.
- **Assessment: Portico ahead** for minimizing disclosure, **Trade-off** for social value. **Large** social service, **medium** opt-in export/scrobble. Prefer optional connectors. Explicit user demand could justify social layers, but not default disclosure.

### A-23.25 Commercial catalogs carry different incentives and obligations

- **Portico:** Free non-commercial owner libraries; no verified ad/rental catalog (PC1/K).
- **Plex / Netflix / Disney+:** Catalog licensing, ads/entitlements, regional availability, DRM and commercial discovery are part of their platforms. [Plex rentals launch][P-rentals] [Plex free versus paid][P-free] [Disney/Hulu profile migration][UX-disney]
- **Why it matters:** Adding free channels is not just another metadata row: third-party account/DRM/rights/support dependencies follow. Revenue can encourage promotional surfaces over owner navigation.
- **Assessment: Trade-off (subjective).** Deliberate personal-server scope is valuable. **Large**, potentially organizational/legal rather than normal coding, to become a distributor. Owner-added legitimate sources remain a different feature. Only a changed business goal would reverse my opinion.

### A-23.26 Static collections, saved queries and external curation

- **Portico:** Static collections plus saved views run through the browse engine; views can appear on Home (`catalog/resources.go:340`; PC6).
- **Plex / Kometa:** Smart collections automatically match filters, can publish rows, and external lists drive named collections/metadata/overlays. [Plex collections][P-collections] [Kometa rule files][S-kometafiles]
- **Why it matters:** Saved query semantics are already present; the delta is a shared curated entity with artwork, ordered membership and imported external rules. Calling “smart views” missing would be false.
- **Assessment: Trade-off (subjective).** **Medium** presentation/conversion between a view and named collection; **medium** adapter. Preserve one query engine. Demand for curated seasonal/award rows supports doing this, while pure personal browsing may not.

### A-23.27 An API ecosystem is more than an OpenAPI file

- **Portico:** Generated route schemas plus broader product API; API settings and events exist, but named adapters not found (PC9/PC12/K).
- **Plex / Jellyfin:** Published PMS API, Jellyfin plugin/client ecosystem, Tautulli/Kometa/request adapters and many established consumers. [Plex published PMS API][P-api] [Jellyfin plugins][J-plugins] [Tautulli API][S-tautapi]
- **Why it matters:** An external developer needs authentication, pagination, event semantics, stable IDs, examples, revision conflicts and compatibility policy. Pre-release route deletion is reasonable today but costly after integrations ship.
- **Assessment: Portico behind** on ecosystem adoption, ahead on potential contract discipline. **Medium** adapter/SDK/reference integration work; **large** compatibility program. Keep the API and pick a small set of supported integrations rather than promise automatic Plex compatibility.

### A-23.28 External events should trigger the narrowest trustworthy scan

- **Portico:** Source/library scan APIs exist; inspected handlers take mode/source, not a changed file/path (PC12).
- **Sonarr / Radarr / Bazarr / Plex:** Import/upgrade/rename/delete events and server scanning interfaces let automation communicate changes after completion. [Sonarr events and metadata][S-sonarr] [Radarr events and metadata][S-radarr] [Plex published PMS API][P-api]
- **Why it matters:** A whole-library walk after every episode scales poorly; on-grab notifications race files that do not exist yet. Raw paths also risk escaping configured roots.
- **Assessment: Portico behind** on verified targeted ingestion interface. **Medium** path-to-source mapping, deduplicated subtree/item queue, authentication/idempotency. Strong fit with O(changed) principles. Existing scanner internals may reduce size, but an event enum alone is not an external integration.

### A-23.29 Built-in history versus independent statistics service

- **Portico:** Permanent owner play history and maintained play counts coexist with private-viewer history controls (PC9).
- **Plex / Tautulli / Jellystat:** Separate statistics collectors persist events and expose richer charts/reports/scripts and APIs. [Tautulli overview][S-tautulli] [Tautulli API][S-tautapi] [Jellystat releases][S-jellystat]
- **Why it matters:** Independent collection survives limited vendor UI, but adds duplicate data/credentials and retention ambiguity. A viewer clearing personal history may not erase admin logs.
- **Assessment: Portico ahead** for common built-in ops, **Unclear** on reporting depth. **Medium** richer exports/anomaly/report queries, no need for another daemon. Prominent privacy wording is important. Users needing multi-server reports or notifications may still prefer an external tool.

### A-23.30 Requests model desire for media that does not exist yet

- **Portico:** My List stores accessible titles; no named request-service connector/workflow found (PC6/K).
- **Seerr:** External titles, request/approval/quota state, existing-availability checks and service automation. [Seerr features][S-seerr]
- **Why it matters:** Requested, approved, downloading, failed and available are not watched-state flags. A no-file work needs a stable ID and permission-safe lifecycle.
- **Assessment: Portico behind** for household acquisition workflow. **Medium** connector; **large** native portal. My opinion is support Seerr first. No conflict with self-hosting if explicit and optional; the no-push/no-email rule limits expected request alerts.

### A-23.31 Plugins/skins versus an authored first-party contract

- **Portico:** Thin clients share authored navigation/settings and design rules; no generic runtime plugin engine found (PC7/K).
- **Kodi / Jellyfin / Emby:** Executable add-ons/plugins and Kodi skins let users replace providers, integrations or UI. [Kodi add-ons][K-addons] [Kodi skins][K-skins] [Jellyfin plugins][J-plugins] [Emby feature matrix][E-matrix]
- **Why it matters:** User flexibility can cure a small team's blind spots; arbitrary execution increases trust, versioning and support cost. Skin-specific behavior undermines a predictable shared product.
- **Assessment: Trade-off (subjective), deliberate consistency.** **Large** safe plugin framework; **medium** declarative themes/extension endpoints. Keep external integrations viable even without plugins. A controlled provider protocol is more plausible than opening every internal API.

### A-23.32 Richer media needs richer progress and authority

- **Portico:** Audio-book containers/files exist; reading/photos are reserved kinds only (PC10).
- **Audiobookshelf / Kavita / Komga / Immich:** Podcast feeds, reading locations/pages, photo capture times/albums/people, uploads and specialist clients have different semantics. [Audiobookshelf introduction][S-abs] [Kavita product][S-kavita] [Komga product][S-komga] [Immich features][S-immich]
- **Why it matters:** A generic item schema does not settle page progress, multi-volume series, duplicate photos, GPS permissions or mobile backup conflicts.
- **Assessment: Trade-off (subjective).** **Large** for each specialist domain; **medium** interoperable read-only catalogs. Preserve extensibility, but don't assume kinds-as-data makes a full library type cheap. A core promise to organize all household media would change priorities.

### A-23.33 Live input types versus TV Everywhere provider login

- **Portico:** HDHomeRun/M3U/Xtream/XMLTV, buffered live streams and DVR (PC8).
- **Channels:** Provider-login TV Everywhere adds source channels with licensing/DRM and changing website authentication. [Channels TV Everywhere][C-tve] [Channels TVE/DRM failures][C-complaint]
- **Why it matters:** This is a connector and maintenance commitment, not a missing DVR engine. Access to a provider app does not guarantee an ingestable legal stream.
- **Assessment: Trade-off (subjective).** **Large** native TVE maintenance; **small–medium** documented external Channels/M3U integration. Prefer interoperability unless TV subscription replacement is a target. Third-party instability would remain even after a flawless implementation.

### A-23.34 Library Channels versus a station automation platform

- **Portico:** Internal scheduled shared channels from browse filters (PC8).
- **Channels / ErsatzTV / Tunarr:** Virtual channels and specialist scheduling, filler, stream outputs and XMLTV allow use by other servers/players. [Channels Virtual Channels][C-virtual] [ErsatzTV playout instructions][S-ersatz] [Tunarr streams and sources][S-tunarr]
- **Why it matters:** Choosing what plays is distinct from a full clocked station with logos, bumpers, filler and interoperable tuner/guide output.
- **Assessment: Unclear** on detailed parity; not a wholesale missing feature. **Medium** M3U/XMLTV export/advanced schedule; **large** broadcast-style playout. Keep current built-in simplicity and allow external channels. Actual scheduling needs determine value.

### A-23.35 Migration is a boundary between products, not a database copy

- **Portico:** Local facts and portable own-state backup; no named Plex/Emby importer found (PC1/K).
- **Jellyfin / WatchState / JellyPlex-Watched / Kodi:** API watch/user migration scripts, external-ID/path matching, and sidecar export/import support adoption. Direct modern Emby DB migration is explicitly unsupported by Jellyfin. [Jellyfin migration boundary][J-migrate] [WatchState project][M-watchstate] [JellyPlex-Watched project][M-jellyplex] [Kodi movie NFO tags][K-movie]
- **Why it matters:** Years of history/poster choices/collections are more valuable than a rescan. Source and destination IDs, editions, users and paths do not match automatically.
- **Assessment: Portico behind.** **Large** complete migration wizard, **medium** watched-state import first. Strong fit with open data if dry-run mappings, receipts, resumability and non-destructive review exist. Password hashes/cloud tokens must not be transplanted as authority.

### A-23.36 Platform breadth, ownership and operational promise

- **Portico:** Web/Apple reference UI, Android/TV/Fire/Vega ports, desktop/DLNA planned, GPL/open source but no external code contributions per brief; physical acceptance outstanding (PC7/PC15).
- **Plex / Emby / Infuse / Kodi:** Plex/Emby have broad commercial app fleets; Infuse concentrates on Apple/Mac/Vision; Kodi emphasizes community platform/player flexibility. [Plex apps and devices][P-platforms] [Emby architecture and apps][E-about] [Infuse product and prices][I-main]
- **Why it matters:** A family will reject a technically strong server if one living-room device or daily car workflow is missing. Open licensing, ability to contribute, and supported maintained clients are separate promises.
- **Assessment: Portico behind** in demonstrated app maturity/breadth, **Trade-off** on contribution policy. **Large** store/device support program; **medium** desktop player/third-party connector. Target a supported platform set honestly; all-platform claims should follow physical qualification rather than build success.

## Missing features and capabilities to consider

### Feature matrix

**Legend:** **Y** = public documentation or inspected Portico path establishes the capability category; **P** = partial/platform-dependent or depth unverified; **Plan** = explicitly in Portico Backlog or vendor roadmap, not counted as shipped; **K−** = no implementation found by the bounded searches above; **—** = outside the verified product scope, not a claim of impossibility; **?** = public evidence insufficient. **PP** = Plex Pass; **EP** = Emby Premiere; **Pro** = Infuse Pro. A “Y” never establishes performance, correctness, all-client availability, or identical semantics. Jellyfin column is from documentation, not code. Sources in a row support the competitor claims; Portico evidence is PC keys or K.

#### Library and metadata

| Capability | Portico | Plex | Emby | Jellyfin docs | Infuse | Notable others / evidence |
|---|---|---|---|---|---|---|
| Movies, shows, music libraries | Y PC4/10 | Y; redesigned app coverage varies | Y | Y | Video; no general music server | Kodi; [Plex metadata agents][P-agents] [Emby architecture and apps][E-about] [Jellyfin library types][J-libraries] [Infuse server modes][I-server] |
| Explicit anime workflow/provider | Y AniList option PC4 | P TV metadata/custom ecosystem | P | P providers/plugins | P server/TMDB | Simkl explicitly tracks anime; [Simkl tracking ecosystem][S-simkl] |
| Local-only metadata | Y PC4 | P personal/embedded assets, modern agent scope differs | Y local/NFO | Y NFO/local assets | Y local files/TMDB toggle scope | Kodi sidecars; [Plex metadata agents][P-agents] [Kodi NFO convention][K-nfo] [Jellyfin movies/NFO/versions][J-movies] |
| NFO ingest | Y subset PC4 | Plan May 2026; later completeness not verified | Y supported ecosystem | Y | P local XML/server metadata | Kodi reference format; [Kodi NFO convention][K-nfo] [Plex 2026 pricing][P-price26] [Jellyfin movies/NFO/versions][J-movies] |
| NFO metadata export/round trip | K− generic exporter; DVR sidecars exist | ? | P metadata savers | Y configurable NFO | ? | Kodi export; *arr writers; [Kodi NFO convention][K-nfo] [Sonarr events and metadata][S-sonarr] [Radarr events and metadata][S-radarr] |
| Provider choices | Y one screen provider PC4 | Modern agent aggregates, legacy priority | Providers/plugins | Providers/plugins | TMDB or connected-server facts | [Plex metadata agents][P-agents] [Jellyfin plugins][J-plugins] |
| Provider provenance/locks | Y PC4 | P edit fields/locks | P edits | P metadata editor | P manual match/local art | Roon category-wise metadata merge; [Roon metadata graph][S-roonmeta] |
| Manual match/artwork correction | Y backend; UI backlog PC15 | Y | Y | Y | Y | Existing curation must migrate; [Plex metadata agents][P-agents] [Infuse iCloud sync][I-cloud] |
| Alternate versions/editions | P schema/parser; grouping backlog PC15 | Y versions; editions distinct state | Y multi-version | Y multi-version naming | Y server version picker | [Plex multiple editions][P-editions] [Jellyfin movies/NFO/versions][J-movies] [Infuse release history][I-releases] |
| Collections | Y PC4/6 | Y | Y | Y | Y | Kometa automation; [Plex collections][P-collections] [Infuse iCloud sync][I-cloud] [Kometa rule files][S-kometafiles] |
| Dynamic filtered saved rows | Y saved views PC6 | Y smart collections | P | P filtered views/plugins | P filters in Library Mode | Portico is not missing saved queries; [Plex collections][P-collections] [Infuse server modes][I-server] |
| Cross-media curated franchise links | P relation/query scope | Y same-name linked collections | ? | P collections | P source/mode dependent | [Plex collections][P-collections] |
| Automatic external-list/award collections | K− | Third-party Kometa | ? | Community tools | ? | Kometa/Wikidata curation; [Kometa overview][S-kometa] |
| Poster overlays / CoverArt | K− automated treatment | Kometa | EP CoverArt | Community tools | — | [Kometa rule files][S-kometafiles] [Emby Premiere pricing][E-price] |
| Other videos/home movies | P actual kind/source workflow needs check | Y Other Videos | Y | Y mixed/video libraries | Y files/home videos | Kodi; [Plex metadata agents][P-agents] [Emby architecture and apps][E-about] [Jellyfin library types][J-libraries] [Infuse product and prices][I-main] |
| Broad local folder browsing alongside library | P owner sources; not player-only | P | P folders | P | Y | VLC/nPlayer/Kodi; [Infuse NAS/file protocols][I-share] [nPlayer features][S-nplayer] [VLC features][S-vlc] |
| Portable metadata/art choices independent of server DB | P NFO input/state folder; generic export K− | P external export tools | P NFO | P NFO | P iCloud/local metadata | Kodi/tinyMediaManager conventions; [Kodi NFO convention][K-nfo] [Plex NFO Exporter][M-plexexport] |

#### Playback and player

| Capability | Portico | Plex | Emby | Jellyfin docs | Infuse | Notable others / evidence |
|---|---|---|---|---|---|---|
| Direct play/remux/transcode | Y PC5 | Y | Y | Y | Direct engine; 8.5 server conversion controls | [Plex transcoder settings][P-transcoder] [Jellyfin transcoding][J-transcode] [Infuse release history][I-releases] |
| Hardware encoding | Y capability; physical acceptance open | PP | EP, named exceptions | Y free | Attached server entitlement | [Plex hardware acceleration][P-hardware] [Emby feature matrix][E-matrix] [Jellyfin GPU pipelines][J-hardware] |
| HDR-to-SDR tone map | Y policy/paths; parity unverified | PP | EP | Y supported paths | Device/server dependent | [Plex HDR tone mapping][P-tone] [Jellyfin GPU pipelines][J-hardware] [Infuse audio output][I-audio] |
| HEVC output conversion | P exact device/backend match not reviewed | Experimental in Sep 2026 docs | P platform | P GPU/codec | Server conversion menu | [Plex transcoder settings][P-transcoder] [Jellyfin GPU pipelines][J-hardware] [Infuse release history][I-releases] |
| Original-quality preference/network ceilings | Y PC7 | Y | Y | Y | Y file/server options | [Plex auto quality][P-auto] [Infuse release history][I-releases] |
| Ongoing automatic bandwidth adaptation | K−; deliberate fixed choice PC1 | Y opt-in converted streams | ? | Client-dependent ? | P buffering/server auto transcode | [Plex auto quality][P-auto] [Infuse release history][I-releases] |
| Quality suggestions requiring viewer action | P no equivalent proven | Y selected apps | ? | ? | P conversion controls | [Plex Android TV settings][P-tvsettings] |
| Prepared optimized media | Y PC5 | Y Media Optimizer | EP content conversion | P external workflows | Uses server versions | [Plex Media Optimizer][P-optimize] [Emby Premiere pricing][E-price] |
| Scheduled/query-based evergreen optimization | P not established | Y optimized selection workflows | EP scheduled/on-add | External Tdarr/Unmanic | — | [Plex Media Optimizer][P-optimize] [Emby Premiere pricing][E-price] [Unmanic workflows][S-unmanic] |
| Audio/subtitle selection and preferred languages | Y PC7 | Y; per-show preferences Sep 2026 | Y | Y | Y | [Plex September release][P-release26] [Infuse release history][I-releases] |
| Manual subtitle timing adjustment | Y web/UI and session fields | Y | P | P clients | Y | Portico `Player.tsx:553`; [Plex subtitle auto-sync][P-subs] [Infuse settings][I-settings] |
| Automatic subtitle alignment | K− | PP voice + external SRT | ? | External tools/plugin workflows | ? | Bazarr ffsubsync; [Plex subtitle auto-sync][P-subs] [Bazarr subtitle processing][S-bazarr] |
| Styled/bitmap subtitle without server burn-in | P device renderer capability | P clients | P clients | P clients | Broad engine support | Kodi/VLC; [Infuse store formats][I-formats] [VLC features][S-vlc] |
| Automatic intro matching | P heuristics/review PC11 | PP audio-season matching | EP | Intro Skipper plugin | P server markers/engine notes | [Plex intro detection][P-intro] [Emby feature matrix][E-matrix] [Intro Skipper README][J-intro] [Infuse release history][I-releases] |
| Skip intro/credits/recap UI | Y preferences/markers PC7/11 | Intro/credits PP; coverage varies | EP intro | P clients/plugins | Y notes; exact coverage varies | [Plex intro detection][P-intro] [Plex credits detection][P-credits] [Infuse release history][I-releases] |
| Credits including mid/post-credit scenes | P no detector parity proven | PP documented extra scenes | ? | Plugin-dependent | Uses server/markers | [Plex credits detection][P-credits] |
| Commercial detection/review | P heuristic candidates PC11 | PP DVR/agent detection | ? | External workflows | — | Channels specialist; [Channels enhanced detection beta][C-commercial] [Plex Series agent settings][P-seriesagent] |
| Trickplay/chapters/thumbs | Y PC11 | Y background analysis | Y client/server scope | Y supported docs | Chapters; server feature support varies | [Plex published PMS API][P-api] [Jellyfin transcoding][J-transcode] [Infuse store formats][I-formats] |
| PiP, background audio, system controls | Y documented PC7; devices qualified separately | P apps | P apps | P clients | Y supported devices | [Infuse store formats][I-formats] [Emby feature matrix][E-matrix] |
| Speed/sleep/skip intervals | Y PC7, music intentionally no speed | P apps | P apps | P clients | P controls | Dedicated audiobook clients; [Audiobookshelf introduction][S-abs] |
| Offline downloads | Y PC5/7; client UI backlog | PP | EP | P third-party clients | Y supported platforms; Pro scope | [Plexamp feature split][PA-features] [Emby feature matrix][E-matrix] [Infuse NAS/file protocols][I-share] |
| All/unwatched/next-N container selection | Y `downloads/requests.go:89` | P app/roadmap | P sync | P clients | P downloads | Do not mislabel next-N absent |
| Rolling auto-download/replenish/evict | ? selection exists, lifecycle not proven | Plan improvements May 2026 | P sync | Client tools | P latest background work | Symfonium automatic caching; [Plex 2026 pricing][P-price26] [Symfonium product features][S-sym] |
| Watch Together | Y contract/routes PC8 | Web retained; redesigned apps retire feature | ? native equivalent | Third-party clients/tools | ? | Plezy brief baseline; [Plex Watch Together notice][P-together] |
| Play-on/remote control | Y PC8 | Y app-dependent | Y app-dependent | P clients | AirPlay/Cast, source-dependent | Plexamp/headless/Kodi; [Plexamp feature split][PA-features] [Infuse product and prices][I-main] |
| Deinterlacing/scaling and AI enhancement | P server deinterlace/scale; AI K− | P enhancements/roadmap | P server processing | P server processing | AI upscaling advertised; no accuracy test | [Infuse product and prices][I-main] |
| Cinema trailer/pre-roll sequencing | K− automatic workflow; extras Y | Y selected apps | EP Cinema Intros | P external/client tools | Extras, exact sequencing ? | [Plex cinema extras][P-extras] [Emby Premiere pricing][E-price] |
| DVD/Blu-ray folders / ISO menus | K− full workflow; not inferred from FFmpeg | P no full menu parity established | P naming/playback limits | P supported docs/clients | Y supported structures; menu scope varies | Kodi/nPlayer/VLC; [Infuse store formats][I-formats] [nPlayer features][S-nplayer] [VLC features][S-vlc] |
| Open arbitrary local file/URL without server | K− | P some apps, not general serverless catalog | — | Third-party players | Y | VLC/nPlayer/Stremio; [Infuse NAS/file protocols][I-share] [VLC features][S-vlc] [Stremio features and guest/addon model][S-stremio] |

#### Music

| Capability | Portico | Plex / Plexamp | Emby | Jellyfin docs | Infuse | Notable others / evidence |
|---|---|---|---|---|---|---|
| MusicBrainz identity and embedded tags | Y PC4/14 | Y aggregated music agent | Y | Y | — general music library | [Plex metadata agents][P-agents] [Jellyfin music metadata][J-music] |
| Gapless trim/gain normalization | Y PC5/7 | Y Plexamp | P client-specific | P clients | Audio in video | [Plexamp feature split][PA-features] [Symfonium product features][S-sym] |
| Crossfade | Y fixed seconds PC7 | Plexamp Sweet Fades | P | P clients | — music queue | Music-aware fades differ; [Plexamp fade discussion][PA-fades] |
| Lyrics/synced lyrics | Y settings/resource PC7 | PP lyrics | EP user lyrics in supported apps | P local/client docs | — | [Plexamp feature split][PA-features] [Emby feature matrix][E-matrix] [Jellyfin music metadata][J-music] |
| Audio-content similarity model | K− PC11 not sonic | PP | ? | Specialist/community tools | — | [Plex sonic analysis][PA-sonic] |
| Artist/track/album radio and mixes | Plan PC15 | PP rich selection | P | P community clients | — | Symfonium mixes/Roon; [Plex specialist-app rationale][PA-specialist] [Symfonium product features][S-sym] |
| Adaptive Guest DJ / sonic journey | K− | PP | — verified | — core verified | — | [Plexamp feature split][PA-features] |
| EQ / AutoEQ / parametric DSP | K− | PP ten-band EQ | ? | Client-dependent | Video audio controls | Symfonium/Roon DSP; [Symfonium product features][S-sym] [Roon pricing/product scope][S-roon] |
| Sample-rate matching / exclusive DAC path | K− verified end-to-end | PP; platform/experimental limits | ? | Client-dependent | Apple video output constraints | Roon/Symfonium; [Plexamp output update][PA-rate] [Roon signal path][S-roonpath] |
| Headless music endpoint | K− | PP Raspberry Pi | ? | External clients | — | Roon endpoints; [Plexamp feature split][PA-features] [Roon ARC FAQ][S-roonarc] |
| CarPlay / Android Auto | K− named app integration | Plexamp available | EP | Third-party music clients | — | Symfonium; [Emby feature matrix][E-matrix] [Symfonium product features][S-sym] |
| Wearable listening/control | K− | P platform ecosystem, not blanket | EP Apple Watch | Third-party clients | — | Symfonium Wear OS; [Emby feature matrix][E-matrix] [Symfonium product features][S-sym] |
| OpenSubsonic API | K− | — native | — native | — native core | — | Navidrome + Symfonium; [Symfonium backend feature matrix][S-symproviders] |
| Classical composition/performance graph | P credits/tags, no model verified | P metadata | P metadata | P tags | — | Roon explicit graph; [Roon metadata graph][S-roonmeta] |
| Rich editorial biographies/reviews/moods | P MusicBrainz/Wikidata, fields/rendering backlog | Y AllMusic/music agent | P providers | P providers/plugins | — | Roon enriched links; [Plex metadata agents][P-agents] [Roon metadata graph][S-roonmeta] |
| Listening dashboards/year recap | P owner counts/history PC9 | Plexamp Aural Fixations | P | Jellystat/clients | — | Tautulli; [Plexamp feature split][PA-features] [Tautulli overview][S-tautulli] |
| Visualizers/interactive waveform music view | P waveform data PC11, UI depth unknown | Y | ? | Client-dependent | — | VLC/other players; [Plexamp feature split][PA-features] |

#### Audiobooks, books, podcasts, and photos

| Capability | Portico | Plex | Emby | Jellyfin docs | Infuse | Notable others / evidence |
|---|---|---|---|---|---|---|
| Audiobook grouping, progress, author/series | Y PC10; agent backlog | P music/third-party apps | P books/audio | Y books/audio | — specialist model | ABS specialist; [Jellyfin books formats][J-books] [Audiobookshelf introduction][S-abs] |
| Audiobook provider lookup/cover enrichment | Plan agent PC15 | Third-party agents/tools | P providers | P plugins | — | ABS multiple sources; [Audiobookshelf introduction][S-abs] |
| Chapter lookup/editor, merge/tag tools | P analysis marker editing; book editor not proven | External tools | ? | External tools | Chapters only | ABS M4B/chapter workflows; [Audiobookshelf introduction][S-abs] |
| Podcast RSS subscribe/auto-download | K− | No verified current podcast service | EP matrix | P manually stored media/plugins | — | ABS; [Emby feature matrix][E-matrix] [Audiobookshelf introduction][S-abs] |
| RSS publication for books/podcasts | K− | — | ? | ? | — | ABS open RSS; [Audiobookshelf introduction][S-abs] |
| EPUB/PDF/comic library | K− PC10 | — core verified | P books depth unverified | Y formats; 12.0 improves reader/book metadata [Jellyfin 12.0 release][J12] | — | Kavita/Komga; [Jellyfin books formats][J-books] [Kavita product][S-kavita] [Komga product][S-komga] |
| OPDS/page streaming | K− | — | ? | ? | — | Kavita/Komga external readers; [Komga OPDS client matrix][S-opds] [Kavita OPDS capabilities][S-kavitaopds] |
| Ebook reading locations and device sync | K− | — | ? | P clients | — | Komga KOReader sync limits documented; [Komga KOReader progress limits][S-koreader] |
| Photo viewing/library/slideshow | K− PC10 | PMS/Plex Photos; redesign coverage varies | Y | Y photos | — not photo manager | Immich/PhotoPrism; [Plex free versus paid][P-free] [Emby architecture and apps][E-about] [Jellyfin library types][J-libraries] |
| Mobile camera backup | K− | No blanket current guarantee | EP Camera Upload | ? | — | Immich; [Emby feature matrix][E-matrix] [Immich features][S-immich] |
| Faces/map/semantic photo search | K− | No verified equivalent current scope | ? | — core verified | — | Immich/PhotoPrism; [Immich features][S-immich] [PhotoPrism documentation][S-photo] |
| Read-only external photo folders | K− photo type | P photos | P photos | P photos | — | Immich external libraries; PhotoPrism originals; [Immich features][S-immich] [PhotoPrism indexing/originals][S-photooriginals] |

#### Live TV and DVR

| Capability | Portico | Plex | Emby | Jellyfin docs | Infuse | Notable others / evidence |
|---|---|---|---|---|---|---|
| HDHomeRun / M3U / XMLTV | Y; also Xtream PC8 | Tuners; arbitrary M3U generally external bridge | Y source-specific | Y tuners/M3U/XMLTV | Not a DVR | Channels; [Plex free versus paid][P-free] [Emby architecture and apps][E-about] [Jellyfin Live TV][J-live] [Channels subscription][C-price] |
| Recurring recording rules and retention | Y PC8 | PP DVR | EP DVR | Y DVR | — | Channels Series Pass; [Emby feature matrix][E-matrix] [Jellyfin Live TV][J-live] [Channels subscription][C-price] |
| Timeshift/shared live input | Y PC8 | P live client/server | P | P | — | Channels TV focus; [Channels subscription][C-price] |
| TV Everywhere provider account connector | K− | — verified native | ? | External source | — | Channels; [Channels TV Everywhere][C-tve] |
| Commercial editor / improved detector | P owner markers, heuristic accuracy | PP detection | ? | External Comskip/plugins | — | Channels prerelease enhanced detector; [Channels enhanced detection beta][C-commercial] |
| Linear channels from own files | Y Library Channels PC8 | External Tunarr/ErsatzTV | External/native plugin scope ? | External tools | — | Channels Virtual Channels; [Channels Virtual Channels][C-virtual] [Tunarr streams and sources][S-tunarr] |
| Filler/bumpers/detailed station clock | ? detailed parity not established | External | External | External | — | ErsatzTV; [ErsatzTV playout instructions][S-ersatz] |
| Export M3U/XMLTV/virtual tuner | K− outward interface; ingest Y | Consumes tuner | Consumes sources | Consumes sources | P playlist/player | Tunarr XMLTV/HDHR; [Tunarr XMLTV export][S-tunarrepg] [Tunarr streams and sources][S-tunarr] |

#### Discovery and social

| Capability | Portico | Plex | Emby | Jellyfin docs | Infuse | Notable others / evidence |
|---|---|---|---|---|---|---|
| Personalized owned-library rows | Y PC6 | Y | P recommendation depth | P related/latest; 12.0 provider choices [Jellyfin 12.0 release][J12] | P categories/server rows | Roon/Symfonium music; [Plex Discover][P-discover] [Symfonium product features][S-sym] |
| Not interested / reset taste | Y PC6 | P cloud recommendation policy | ? | ? | ? | Netflix feedback convention; [Netflix hidden history convention][UX-history] |
| Household also-watched signal | Y opt-in PC6 | P social/server rows | ? | Plugin/client-dependent | Uses source catalogs | [Plex cloud watched-state sync][P-sync] |
| Universal streaming availability/search | K− | Y | — | — core | — | Apple TV/Stremio catalog scope differs; [Plex Discover][P-discover] [Stremio features and guest/addon model][S-stremio] |
| External work Watchlist before ownership | K− My List not same model | Y | ? | Request services | P Trakt lists | Seerr; [Plex Universal Watchlist][P-watchlist] [Seerr features][S-seerr] |
| Requests/approvals/quotas | K− | Seerr/Overseerr | Seerr | Seerr/Jellyseerr | — | [Seerr features][S-seerr] |
| Public ratings/reviews/discussions | K− deliberate private product | Social rollout Jun 2026 | — verified | — core verified | Trakt optional | Trakt/Simkl; [Plex discussions launch][P-discussions] [Trakt scrobbler announcement][S-trakt] [Simkl tracking ecosystem][S-simkl] |
| Optional external watch-state sync | K− | Plex cloud or third-party tools | Plugins/tools | Trakt plugin/tools | Y Trakt/iCloud with caveats | [Plex cloud watched-state sync][P-sync] [Jellyfin plugins][J-plugins] [Infuse Trakt sync][I-trakt] |
| Ad-supported/rental catalogs | K− deliberate scope | Y | — | — | — | Commercial rights required; [Plex free versus paid][P-free] [Plex rentals launch][P-rentals] |

#### Users, sharing, and remote access

| Capability | Portico | Plex | Emby | Jellyfin docs | Infuse | Notable others / evidence |
|---|---|---|---|---|---|---|
| Local username/password account | Y PC2 | P offline/LAN exceptions; normal account central | Y | Y | NAS/server credentials | [Plex account requirements][P-account] [Emby Connect boundary][E-connect] [Jellyfin users and parental controls][J-users] [Infuse NAS/file protocols][I-share] |
| Optional central sign-in/directory | Y Hosted PC2 | Normal Plex model | Y optional Connect | No vendor directory | Server online accounts/iCloud | [Emby Connect boundary][E-connect] [Infuse server modes][I-server] |
| Managed profiles/PIN/fast switching | Y PC13 | Y Home | P app/users | P Quick Connect/local users | P system/Plex profile switch | [Plex Home][P-home] [Infuse server modes][I-server] [Jellyfin users and parental controls][J-users] |
| Library/content restrictions | Y PC2 | Y shares/ratings/labels | Y | Y | Folder/rating settings | [Plex share restrictions][P-sharing] [Emby feature matrix][E-matrix] [Jellyfin users and parental controls][J-users] [Infuse product and prices][I-main] |
| Direct certificates/vendor hostname | Y claimed PC3 | Y `plex.direct` | Owner configuration | Owner configuration | Source HTTPS | [Plex secure connections][P-secure] [Jellyfin networking][J-network] |
| Free remote personal video | Y when reachable | Entitlement required, rollout-specific | P app unlock/EP; no video-only remote pass | Y when reachable | Pro/client/server entitlement | [Plex remote requirements][P-entitlement] [Emby feature matrix][E-matrix] [Infuse purchases and sharing][I-purchase] |
| Vendor relay fallback | Plan PC15 | Y 2 Mbps, downloads excluded | No Connect relay | No official vendor relay | Uses source reachability | [Plex Relay][P-relay] [Emby Connect boundary][E-connect] |
| Easy VPN/CGNAT setup guidance | ? acceptance not established | Relay/remote troubleshooting | Owner network | Owner network/VPN | NAS/server network | [Jellyfin networking][J-network] [Plex remote setup][P-remote] |
| Viewer privacy vs owner permanent history | Y separate PC9 | P vendor/server/tool stores | P server/tool stores | P logs/plugins | Sync destination caveats | Must disclose boundaries; [Tautulli API][S-tautapi] [Infuse Trakt sync][I-trakt] |

#### Administration, operations, integrations, and ecosystem

| Capability | Portico | Plex | Emby | Jellyfin docs | Infuse | Notable others / evidence |
|---|---|---|---|---|---|---|
| Owner now-playing, history, health | Y PC9 | PMS/Dash/Tautulli | Y | Y dashboard + Jellystat | No server admin | [Plex Dash release notes][PA-dash] [Tautulli overview][S-tautulli] [Jellystat releases][S-jellystat] |
| Backup/restore with ordinary files | Y PC1/9 | P PMS files/database | EP plugin for managed UI | Y docs; migration caveats | iCloud/local app state | [Plex state directory][P-data] [Emby feature matrix][E-matrix] [Jellyfin migration boundary][J-migrate] |
| Documented integration API | Y multiple specs PC9 | Y published/versioned | Y ecosystem | Y API | 2026 release notes mention API additions | [Plex published PMS API][P-api] [Jellyfin plugins][J-plugins] [Infuse release history][I-releases] |
| Outbound event webhooks | K−; SSE events are not webhooks | PP | EP | Webhook plugin | ? | [Plex webhooks][P-hooks] [Emby feature matrix][E-matrix] [Jellyfin plugins][J-plugins] |
| *arr targeted scan connector | K−; source/library scans Y PC12 | Published API + connectors | Existing ecosystem | Existing ecosystem | Reads connected server | [Plex published PMS API][P-api] [Sonarr events and metadata][S-sonarr] [Radarr events and metadata][S-radarr] |
| Request-service adapter | K− | Seerr | Seerr | Seerr | — | [Seerr features][S-seerr] |
| Tautulli/Jellystat compatibility | K− APIs not drop-in | Tautulli | Jellystat documented ecosystem | Jellystat | — | [Tautulli API][S-tautapi] [Jellystat releases][S-jellystat] |
| Kometa/external authoring adapter | K− | Y Kometa | Different tooling | Different tooling | — | [Kometa rule files][S-kometafiles] |
| Watch-state migration/sync adapters | K− | WatchState/JellyPlex | Same | Same | Trakt/source sync | [WatchState project][M-watchstate] [JellyPlex-Watched project][M-jellyplex] |
| Original metadata/art/collection export | P own backup; interoperable exporter K− | Tautulli/third-party exports | P savers/plugins | NFO/API tools | P local/iCloud | [Tautulli API][S-tautapi] [Plex NFO Exporter][M-plexexport] [Kodi NFO convention][K-nfo] |
| Distributed library conversion workers | K− built-in | External Tdarr | External Tdarr/Unmanic | External workflows | — | [Tdarr worker types][S-tdarr] [Unmanic workflows][S-unmanic] |
| DLNA/UPnP serving | Plan PC15 | Y official scope | Y | Plugin in newer ecosystem | Client can read UPnP/DLNA | [Emby architecture and apps][E-about] [Jellyfin plugins][J-plugins] [Infuse NAS/file protocols][I-share] |
| Arbitrary server plugins / provider extension API | K− generic runtime | Modern provider API; legacy history | Y plugins | Y plugins | Closed engine/API surface | [Plex published PMS API][P-api] [Jellyfin plugins][J-plugins] [Emby feature matrix][E-matrix] |
| UI skins/themes | P authored appearance; no skins | P themes/app design | EP themes | Community CSS/clients | P app settings | Kodi skins; [Kodi skins][K-skins] |
| Email/push owner/request notifications | Deliberate no server push/email PC7 | Tautulli ecosystem | Webhooks/tools | Webhook tools | Platform sync notifications | [Tautulli overview][S-tautulli] [Seerr features][S-seerr] |

#### Apps, platforms, and monetization/licensing

| Capability | Portico | Plex | Emby | Jellyfin docs | Infuse | Notable others / evidence |
|---|---|---|---|---|---|---|
| Web, iOS/iPadOS, Apple TV | Y code/docs; acceptance open | Y evolving app coverage | Y | Y official/community | Y | [Plex apps and devices][P-platforms] [Emby architecture and apps][E-about] [Jellyfin client inventory][J-clients] [Infuse product and prices][I-main] |
| Android/Android TV/Fire TV | Ports; physical parity open PC7 | Y | Y | Y | — | Symfonium/Kodi/VLC; [Symfonium product features][S-sym] |
| Vega OS | Port PC7/brief | ? latest device-specific support | ? | ? | — | Do not infer Fire OS compatibility |
| Dedicated Windows/macOS/Linux playback | Plan desktop companion PC15 | Y desktop/HTPC | Y apps | Y desktop/community | macOS Y | Kodi/VLC; [Plex apps and devices][P-platforms] [Emby feature matrix][E-matrix] [Jellyfin client inventory][J-clients] |
| Samsung/LG/console fleet | K− verified first-party scope | Y | Y | P apps/community/sideload | — | [Plex apps and devices][P-platforms] [Emby architecture and apps][E-about] [Jellyfin client inventory][J-clients] |
| Vision Pro app | K− verified | ? | ? | Community clients | Y | [Infuse product and prices][I-main] |
| Car/wearable/headless listening | K− | Plexamp | EP | Community clients | — | Symfonium/Roon; [Symfonium product features][S-sym] [Plexamp feature split][PA-features] |
| Broad UI translations | English/regions now, more planned PC7 | Y commercial fleet | Y | Y community | Many locales | Exact quality not audited; [Infuse store formats][I-formats] [Jellyfin client inventory][J-clients] |
| Free open-source server/client core | Y brief GPL, no external code contributions | Proprietary | Proprietary since 2018 | Y GPL/community | Proprietary | Kodi/VLC open; [Emby proprietary transition][E-history] [Jellyfin free/open model][J-about] [VLC features][S-vlc] |
| Lifetime/recurring entitlement | None core; relay possible | PP/RWP; 2026 prices above | EP/key/device terms | None | Pro recurring/lifetime | Roon paid; Symfonium one-time; [Plex 2026 pricing][P-price26] [Emby Premiere pricing][E-price] [Infuse purchases and sharing][I-purchase] [Roon pricing/product scope][S-roon] |
| Paid hosted enrichment/service alongside free core | Dataset intended free/non-commercial PC6 | Cloud/ads/passes | Premiere | Donations/no feature paywall | App engine/sync premium | Kavita+; [Kavita product][S-kavita] |
| User contribution/extensibility governance | No external code contributions per brief | Vendor/private core + APIs | Vendor/private core + plugins | Community contributions | Vendor/private app | Participation policy is independent of GPL rights |

### Numbered gaps and capability proposals

These include deliberate exclusions and known backlog items, not just new omissions. The matrix also contains **unknown depth** cells that are questions rather than asserted gaps. The prioritization is opinion, not authorization to implement. No runtime failure has been established.

#### Adoption, portability, and integration

### F-23.1 A reviewed migration wizard for Plex/Jellyfin/Emby

Import a user's investment, not merely rescan their folders: libraries, account/profile mappings, watched/progress/rating state, posters, manual metadata and collections. Jellyfin documents API migration scripts, WatchState and JellyPlex-Watched synchronize play state, and Plex export tools preserve metadata (see migration section). Portico: no named importer found (K). **Value:** reduces the biggest switching cost. **Size:** large. **Conflict:** none with open data; credentials and user mapping need new consent. **Opinion:** highest-value adoption feature; start with a dry-run map and staged imports, never wholesale database conversion. [Jellyfin migration boundary][J-migrate] [WatchState project][M-watchstate] [JellyPlex-Watched project][M-jellyplex]

### F-23.2 Interoperable export of facts, artwork, collections and personal state

Portico backups already preserve its own state, and NFO ingest/DVR sidecars exist. What I did not find is a general export package another product can consume (K/PC4/9). Kodi sidecars and Tautulli metadata exports set precedents. **Value:** user confidence and recovery when Portico is no longer desired. **Size:** medium–large. **Conflict:** none, but private per-profile history must be separated from shared NFO. **Opinion:** pair export with import; version the package and include external IDs, provenance and art digests, not just display names. [Kodi NFO convention][K-nfo] [Tautulli API][S-tautapi]

### F-23.3 External-state reconciliation for coexistence, not only one-time migration

WatchState/JellyPlex-Watched let two servers coexist while a household tries a replacement. Portico lacks a verified adapter (K). **Value:** safe trial adoption and different apps using the same storage. **Size:** medium. **Conflict:** external edits must not silently erase current local progress. **Opinion:** implement explicit one-way/import-only first, then optional bidirectional synchronization with deterministic conflicts. Preserve last-played timestamps and don't collapse cuts/episode orders by title. Separate “unwatched” from “source has no record.” [WatchState project][M-watchstate] [JellyPlex-Watched project][M-jellyplex] [WatchState backend limitations][M-limitations]

### F-23.4 Authenticated outbound webhooks

Plex Pass, Emby Premiere and Jellyfin's Webhook plugin support notifications/automation. Portico has SSE invalidations but no named outbound webhook implementation found (K). **Value:** home automation, statistics, request status and watch-state tools. **Size:** medium. **Conflict:** potential external data disclosure, arbitrary egress and no-server-email principle; none requires email. **Opinion:** add an optional durable outbox with narrow event schemas, signatures, retry IDs and per-profile scope. Revalidate destination trust and never include credentials or unrestricted source paths. [Plex webhooks][P-hooks] [Emby feature matrix][E-matrix] [Jellyfin plugins][J-plugins]

### F-23.5 Import/rename/replace notifications with targeted scans

Sonarr/Radarr expose completion, upgrade, rename and deletion events. Portico source/library scans exist; inspected routes have no path-limited argument (PC12/K). **Value:** one new episode should not walk millions of items. **Size:** medium. **Conflict:** none; this directly serves incremental scale. **Opinion:** high priority: trusted source-relative file/subtree IDs, path translation, idempotent coalescing, status receipts and permission-scoped credentials. Wait for actual import completion, then invalidate metadata/subtitle/analysis state for that file. Don't treat a delete-for-upgrade event as permanent title loss. [Sonarr events and metadata][S-sonarr] [Radarr events and metadata][S-radarr]

### F-23.6 Seerr adapter and request lifecycle

Seerr is the current unified request ecosystem rather than three equally current independent projects. Portico has no verified adapter or requested/approved/acquiring/available domain (K). **Value:** family members request absent titles without messaging the owner manually. **Size:** medium adapter; large full native portal. **Conflict:** external-work IDs and notifications are new; no push/email remains deliberate. **Opinion:** support an adapter first, with availability events and constrained service accounts. My List should retain its meaning rather than become an acquisition queue implicitly. [Seerr features][S-seerr] [Seerr merger announcement][S-seerrmerge]

### F-23.7 Opt-in Trakt/Simkl and music scrobbling

Infuse/Jellyfin tools offer external watch tracking; Plex/Simkl ecosystems support cloud histories. Scrobbling means reporting listening/watching to a history service. No Portico implementation found (K). **Value:** portable history and existing recommendation/list habits. **Size:** medium per protocol. **Conflict:** deliberate external disclosure, Hosted privacy boundary and kids policies. **Opinion:** worthwhile user-controlled connector; keep tokens per person, retries idempotent, and outbound history disabled by default. Music Last.fm/ListenBrainz would be analogous proposals, but their current API details were not audited here. [Infuse Trakt sync][I-trakt] [Jellyfin plugins][J-plugins] [Simkl tracking ecosystem][S-simkl]

### F-23.8 OpenSubsonic compatibility for music clients

Symfonium and other music players speak OpenSubsonic/Navidrome; Portico's own API is not interchangeable and no adapter was found (K). **Value:** immediate car, offline, DAC and specialist client options without writing every client feature. **Size:** medium–large, depending on extension coverage. **Conflict:** shared-client/thin-client preference, but a separate public projection can preserve server authority. **Opinion:** strong ecosystem option if music is a launch promise. Specify supported extensions, playlist mutation, authentication and pagination; don't claim compatibility from a handful of endpoints. [Symfonium backend feature matrix][S-symproviders]

### F-23.9 A documented integration target for Infuse and other multi-server players

Infuse supports Plex/Emby/Jellyfin, and Symfonium supports known protocols; Portico support is not automatic (K). **Value:** users can keep a favorite player while trying a new server. **Size:** medium API/client adapter plus vendor coordination; potentially large for full playback/download parity. **Conflict:** none if authority and capability negotiation remain explicit. **Opinion:** choose one reference third-party integration and publish practical examples, source/version selection, grant renewal and watched-state contracts. Avoid impersonating Plex account or entitlement APIs. [Infuse server modes][I-server] [Symfonium backend feature matrix][S-symproviders]

#### Playback and mobile robustness

### F-23.10 Open-file/open-URL playback without a server

VLC, nPlayer and Infuse solve immediate file playback. No Portico equivalent was found (K). **Value:** travel, USB files, debugging and a simple entry point for new users. **Size:** medium on native apps, smaller limited web-file support. **Conflict:** server-first authority; keep a visibly separate local session with local progress. **Opinion:** more manageable than a full player-only catalog. Ensure share-sheet/file-provider permissions, correct cleanup, subtitles and supported formats; don't require a fake server to play a local MP4. [VLC features][S-vlc] [nPlayer features][S-nplayer] [Infuse NAS/file protocols][I-share]

### F-23.11 Full player-only NAS/cloud library mode

Infuse, VidHub and similar clients scan shares, enrich metadata and maintain local state. Portico's SMB is currently a server source option, not an app-owned library (K). **Value:** NAS owners avoiding a media-server install. **Size:** large: sources, credential vault, catalog/indexing, metadata, progress and sync authority. **Conflict:** thin-client/server-first principle and huge-library goals. **Opinion:** a genuine strategic alternative, not a routine feature. Consider a local read-only subset first; require Justin's target-user decision before building a parallel server inside every client. [Infuse NAS/file protocols][I-share] [VidHub product scope][S-vidhub]

### F-23.12 A broader client video/subtitle engine where format tests justify it

Infuse/Kodi/VLC minimize server conversions for demanding remuxes, image subtitles and styled ASS. Portico already converts unsupported input; this is a potential direct-play depth gap, not missing playback (PC5/7). **Value:** lower server load and better preservation of original video/subtitle appearance. **Size:** large across native platforms. **Conflict:** binary size/licensing/maintenance; none with owner choice. **Opinion:** conditional: benchmark a shared acceptance corpus before choosing mpv/libVLC/custom FFmpeg or improving native paths. Format lists alone do not prove superiority. [Infuse store formats][I-formats] [VLC features][S-vlc]

### F-23.13 Explicit opt-in adaptive quality or actionable quality suggestions

Plex adapts converted streams to changing bandwidth and also offers suggestions. Portico deliberately keeps viewer-requested quality (PC1/K). **Value:** uninterrupted cellular/weak Wi-Fi playback. **Size:** large adaptive pipeline, small–medium suggestions. **Conflict:** literal quality-only-on-request rule. **Opinion:** preserve fixed-original default; suggestions are the easiest fit. A user explicitly choosing “adapt to my connection” could count as ongoing consent only if Justin accepts that interpretation. Never confuse GPU pressure with network congestion. [Plex auto quality][P-auto] [Plex Android TV settings][P-tvsettings]

### F-23.14 Automatic subtitle synchronization with source/edition safeguards

Manual subtitle timing exists in Portico; automated audio-based alignment was not found (K). Plex uses stored voice activity, Bazarr uses audio/embedded reference alignment. **Value:** common subtitle repair without stopping to guess offsets. **Size:** medium–large. **Conflict:** analysis cost on weak hardware, policy-gated remote reads. **Opinion:** useful optional analysis; preserve originals, provenance and manual offset, expose confidence and reject conflicting edits/cuts. Support language-dependent audio selection and rate drift, not just a single global offset. [Plex subtitle auto-sync][P-subs] [Bazarr subtitle processing][S-bazarr]

### F-23.15 Repeated-intro matching and trusted marker import

Portico's chapter/luminance candidates require review (PC11). Plex finds repeated season audio; Jellyfin Intro Skipper is another documented approach. **Value:** usable Skip Intro at household scale without marking every episode. **Size:** large detector; medium import/interop. **Conflict:** background cost and false-skip safety. **Opinion:** prioritize trustworthy marker import plus season matching. Distinguish detected, imported, edited and approved evidence; short, changing, silent and anime openings need a deliberate acceptance corpus. Do not “fix” this by approving heuristic candidates automatically. [Plex intro detection][P-intro] [Intro Skipper README][J-intro]

### F-23.16 Credits with mid/post-credit scene boundaries

Portico detects an end-region hypothesis, but no equivalent extra-scene classifier was established (PC11). Plex documents extra-scene-aware skipping and post-play behavior. **Value:** fast episode transitions without losing story content. **Size:** large analysis, medium richer marker/UI model. **Conflict:** false skips; optional central-marker service changes cheap Hosted/privacy scope. **Opinion:** useful, but preserving scenes outranks aggressive skipping. Reuse/import markers locally first; treat cloud matching as a separate optional service with explicit retention and disclosure. [Plex credits detection][P-credits]

### F-23.17 Accurate commercial detection and a recording-focused review UI

Portico has commercial marker types/candidates; Comskip integration and specialist review parity were not found (K/PC11). Channels offers established commercial workflows and an opt-in enhanced detector in 2026. **Value:** practical TV watching and correcting bad skips. **Size:** medium external detector adapter, large native accuracy work. **Conflict:** analysis cost; automatic deletion must be separately opt-in. **Opinion:** prioritize reversible skip intervals and easy review over destructive cutting. Broadcast fades, sports and interrupted recordings require tests. [Channels enhanced detection beta][C-commercial] [Channels commercial settings][C-commercialsettings]

### F-23.18 Durable optimization rules, rather than one conversion at a time

Prepared artifacts already exist (PC5). Plex Media Optimizer and Emby on-add/scheduled conversion offer policy-based preparation. **Value:** small servers can prepare remote/mobile-friendly media before viewers arrive. **Size:** medium rules/UI and lifecycle. **Conflict:** storage and energy costs, not a mandatory quality downgrade. **Opinion:** worthwhile if rules select bounded changed media and expose estimated bytes. Reuse the same prepared artifact/lease subsystem and invalidate by source revision; never silently replace originals. [Plex Media Optimizer][P-optimize] [Emby Premiere pricing][E-price]

### F-23.19 Rolling offline policies with storage-aware refill

Portico supports next-N/all/unwatched selection; the inspected capture/admission stages don't establish recurring refill/eviction after watching (PC5; requests lines 320–404). Symfonium automatic caching and Plex's download roadmap show demand. **Value:** a commute-ready queue that stays ready. **Size:** medium–large. **Conflict:** client storage, token expiry and silent network use. **Opinion:** verify existing download lane first; then express desired state separately from one-time receipts, with Wi-Fi/charging choices and visible eviction reasons. No claim that next-N is missing. [Symfonium product features][S-sym] [Plex 2026 pricing][P-price26]

### F-23.20 Disc structures and extras interoperability

Infuse lists ISO/BDMV/VIDEO_TS and broad subtitle support; Kodi/VLC/nPlayer users expect file structures, extras and editions to remain usable. Portico has an extra kind and NFO edition parsing, but full disc-menu playback is not established (PC10/K). **Value:** film enthusiasts avoid remuxing everything. **Size:** large full menus/decryption-dependent paths; medium recognized titles/chapters/extras. **Conflict:** maintenance and format/rights complexity. **Opinion:** document boundaries first, support useful unencrypted structures if demand warrants, and never imply bypassing DRM is part of the feature. [Infuse store formats][I-formats] [VLC features][S-vlc]

#### Music depth

### F-23.21 Audio-derived musical similarity

Plexamp sonic features work beyond tags; Portico generic fingerprints and AcoustID don't perform that role (PC11/14/K). **Value:** obscure and local music becomes discoverable. **Size:** large model/index/analysis pipeline. **Conflict:** weak hardware, energy and model licensing; optional background analysis fits maintained derivations. **Opinion:** important only if Plexamp-level music is a core promise. Select a distributable model, version embeddings, define incremental candidate indexes, and measure recommendations with listener judgments rather than nearest-neighbor demos. [Plex sonic analysis][PA-sonic]

### F-23.22 Artist/album/track radio and personal mixes

Already planned in Backlog (PC15), unlike absent basic shuffle/queues. Plexamp/Symfonium let a seed create a varied continuous listening session. **Value:** people listen more of their library without queue management. **Size:** medium metadata-only first version; large sonic-enhanced depth. **Conflict:** correctness and bounded candidate diversity; no necessary cloud service. **Opinion:** a very high-value music gap. Use deterministic explainable seed/rules, avoid repeating one artist, respect dislikes and current queue ownership, and make radio distinguishable from shuffle. [Plex specialist-app rationale][PA-specialist] [Symfonium product features][S-sym]

### F-23.23 Guest DJ and sonic journeys

Plexamp changes a queue with related tracks and creates a sonic route between chosen seeds. No equivalent Portico queue-generating strategy found (K). **Value:** exploratory listening and rediscovery. **Size:** medium once similarity/radio exists, otherwise large. **Conflict:** explicit queues should not change unexpectedly. **Opinion:** valuable second-stage music feature, not a launch blocker. Viewer chooses the DJ and can inspect/remove generated entries; preserve entry occurrence IDs so inserted songs don't corrupt progress or remote controls. [Plexamp feature split][PA-features]

### F-23.24 Equalizer, AutoEQ, crossfeed and output-specific DSP

Portico gain/crossfade/gapless already exist, but named EQ/DSP absent (K). Plexamp ten-band EQ and Symfonium/Roon tuning provide concrete user value. **Value:** headphones, hearing preferences and room/output correction. **Size:** medium basic EQ, large calibrated/parametric/device-specific DSP. **Conflict:** modifies audio intentionally; no issue with owner choice when visible. **Opinion:** basic per-device EQ is plausible, advanced room correction optional. Prevent clipping, report processing in Playback info, persist by output route rather than changing every household device. [Symfonium product features][S-sym] [Roon signal path][S-roonpath]

### F-23.25 Sample-rate matching and honest output diagnostics

No complete Portico rate-matching/bit-perfect path found (K). Plexamp's September 2024 notes describe experimental Android output on some DACs; Roon's signal path separates lossless and enhanced processing. **Value:** enthusiasts trust actual audio transformations. **Size:** medium reporting, large platform-exclusive output. **Conflict:** sample-rate switching can oppose crossfade/mixing. **Opinion:** add truthful reporting first. Avoid a universal bit-perfect claim; state source, conversion, gain, mixer rate, route and unknown downstream steps. Capability tests must use real hardware. [Plexamp output update][PA-rate] [Roon signal path][S-roonpath]

### F-23.26 Headless listening receiver and remote-control UX

Plexamp can run on a Raspberry Pi; Roon has network endpoints. Portico already has Play on and controller/renderer separation, but no documented headless music appliance (K/PC8). **Value:** a stereo without a screen, controlled from phone. **Size:** medium–large receiver packaging/output lifecycle. **Conflict:** none fundamental; supported hardware burden. **Opinion:** good fit for the newer protocol once audio route reliability exists. Define discovery, trust, queue ownership and reconnect explicitly; don't treat a browser left open on a Pi as a supported appliance. [Plexamp feature split][PA-features] [Roon ARC FAQ][S-roonarc]

### F-23.27 CarPlay and Android Auto

Plexamp, Emby Premiere and Symfonium provide car-specific browse/control. No named Portico integration found across native sources (K). **Value:** daily music/audiobook listening without handheld UI. **Size:** medium per platform, plus entitlement/design approval. **Conflict:** no core server change, but thin-client paged browsing must fit car constraints. **Opinion:** among the highest-value missing music/audiobook app features. Respect car interaction limits and offline availability; ordinary Now Playing commands do not establish a browsable car app. [Emby feature matrix][E-matrix] [Symfonium product features][S-sym]

### F-23.28 Wearable control/offline listening

Emby's matrix lists Apple Watch; Symfonium lists Wear OS. No Portico watch app established (K). **Value:** exercising and easy remote playback. **Size:** medium controls, large independent downloads/library. **Conflict:** platform breadth and maintenance. **Opinion:** lower than car support unless the target audience strongly uses watches. A small controller can reuse server authority without pretending the watch is a full library client; independent offline receipts and token expiry need separate design. [Emby feature matrix][E-matrix] [Symfonium product features][S-sym]

### F-23.29 Classical compositions, performances and richer credits

Portico's metadata/credits exist; a composition/performance graph was not found (K). Roon explicitly distinguishes artists, composers, performances, compositions and album instances. **Value:** classical/jazz browsing, multiple recordings, conductors and session credits. **Size:** large data/model/UI work. **Conflict:** provider availability and incremental indexing, not ownership. **Opinion:** preserve tags and external IDs now; implement graph depth only if this is the target music audience. “Artist = composer” or title text matching will lose meaning during import. [Roon metadata graph][S-roonmeta]

### F-23.30 Rich music editorial presentation and metadata completeness

Plex Music includes AllMusic/MusicBrainz; Roon adds linked editorial context. Portico supports music facts but Backlog notes several fields not rendered and related artists/albums work pending (PC4/15). **Value:** discovery by collaborators, label, release type, original year and credible descriptions. **Size:** medium rendering/provider-compatible facts; large licensed editorial service. **Conflict:** free/non-commercial/provider policy. **Opinion:** render existing facts first; verify license and provenance before adding a service. Do not mistake MusicBrainz's IDs for a full commercial editorial feed. [Plex metadata agents][P-agents] [Roon metadata graph][S-roonmeta]

### F-23.31 Personal listening recap, streaks and exploration summaries

Portico play counts/history exist (PC9), so this is a personal product/reporting gap rather than missing telemetry. Plexamp Aural Fixations and Tautulli illustrate summaries. **Value:** users understand taste, rediscover neglected music and enjoy an annual recap. **Size:** medium maintained aggregates/UI. **Conflict:** privacy pause/reset semantics and scan cost. **Opinion:** optional local reports can be delightful; base them on observed listening, expose covered dates, and avoid treating seeks or endless background radio as engaged listening. [Plexamp feature split][PA-features] [Tautulli overview][S-tautulli]

### F-23.32 Music-aware transitions and visualizations

Portico already has waveform data and fixed crossfade (PC7/11). Plexamp Sweet Fades decides transitions differently and preserves complete album listening; its staff forum explanation rejects forced album fades. **Value:** smooth mixes without ruining gapless albums. **Size:** medium envelope/queue policy, medium visualization UI. **Conflict:** lossless/exclusive path may disable processing. **Opinion:** lower priority than radio; don't claim parity from a seconds slider. Mark this as depth to validate, not established absence of every visualization. [Plexamp fade discussion][PA-fades] [Plexamp album fade rule][PA-fadesalbum]

#### Audiobooks, reading, and photos

### F-23.33 Reviewed audiobook metadata agent

Already listed as unmerged/unreviewed in Backlog (PC15); present book metadata is local/tag-based. Audiobookshelf offers provider lookup and cover enrichment. **Value:** usable books without manually tagging every file. **Size:** medium after existing work review. **Conflict:** no new user keys/local-only must remain honored, and provider access must be permitted. **Opinion:** high value for a declared audiobook library. Evaluate a provider with correct editions/narrators/series; don't import a movie-style matcher into book identity blindly. [Audiobookshelf introduction][S-abs]

### F-23.34 Audiobook chapter editor and personal bookmarks

Portico chapters and generic analysis-marker editing exist, but a specialist book chapter lookup/merge workflow and persistent listener bookmarks were not established by searched surfaces. ABS offers chapter tooling; bookmark depth is a Portico proposal, not claimed competitor parity. **Value:** long books, multi-file editions and memorable passages. **Size:** medium. **Conflict:** shared chapter facts versus private bookmarks require separate authority. **Opinion:** useful after reliable book clocks/offline reconciliation; preserve original file timeline and record chapters across files consistently. Confirm with the audiobook auditor before asserting complete absence. [Audiobookshelf introduction][S-abs]

### F-23.35 Podcast subscriptions and automatic episode acquisition

Only a defensive podcast kind string was found; no RSS ingest/library workflow (K/PC10). ABS documents feed discovery and auto-download, Emby lists Podcasts in Premiere. **Value:** regular listening, offline queues and retention. **Size:** large new ingestion/library domain. **Conflict:** scheduled external traffic, feed redirects/credentials, retention and source trust. **Opinion:** reasonable adjacent expansion, not free audiobook parity. Distinguish feed GUIDs from filenames, support changed episodes, and keep feed fetches out of page requests. [Audiobookshelf introduction][S-abs] [Emby feature matrix][E-matrix]

### F-23.36 Private RSS publication of audiobooks/podcasts

ABS can expose RSS for external podcast apps. No Portico equivalent found (K). **Value:** users keep their preferred listening app. **Size:** medium. **Conflict:** long-lived URLs become bearer credentials and public HTTP remains owner choice. **Opinion:** potentially cheaper than another client, with revocable scoped feed grants and expiry explanations. A feed app may not support Portico's progress or fine-grained restrictions; disclose that limitation. Never publish every library through an unscoped feed. [Audiobookshelf introduction][S-abs]

### F-23.37 EPUB/PDF/comics with format-specific progress

Portico reserves reading kinds but current `book` is listening-oriented (PC10). Kavita/Komga offer dedicated readers; Jellyfin documents book extensions. **Value:** one personal library for readers. **Size:** large scanners/renderers/locations/accessibility across clients. **Conflict:** scope expansion and archive/untrusted-document parsing. **Opinion:** don't start by reusing video seconds or a poster grid. A specialist external reader integration is a more credible first step than a full multi-platform reader in a media-server launch. [Kavita product][S-kavita] [Komga product][S-komga] [Jellyfin books formats][J-books]

### F-23.38 OPDS and page-streaming interoperability

OPDS is a catalog/distribution protocol for ebook/comic readers, not a general progress-sync guarantee. Kavita/Komga document client-specific capabilities and KOReader limitations. No Portico interface found (K). **Value:** immediate established reading apps if reading libraries are added. **Size:** medium server adapter; progress protocol separate. **Conflict:** permissions, grant scope and huge catalogs must page correctly. **Opinion:** useful after a reading model, not before; document version/extensions and exact sync granularity. [Komga OPDS client matrix][S-opds] [Kavita OPDS capabilities][S-kavitaopds] [Komga KOReader progress limits][S-koreader]

### F-23.39 Read-only photo/home-video library

Plex/Emby/Jellyfin support photos; Immich/PhotoPrism specialize. Portico has reserved photo kinds but no actual photo library established (PC10/K). **Value:** private TV slideshows, albums and family videos. **Size:** medium–large for read-only indexing/display. **Conflict:** mixed layouts and people/location privacy; scale may mean millions of assets. **Opinion:** a worthwhile question, not an obvious must-have. Start with folder/albums and interoperable external-library integration; keep photos out of movie recommendations and owner play-history assumptions. [Immich features][S-immich] [PhotoPrism indexing/originals][S-photooriginals]

### F-23.40 Full mobile photo backup and intelligent photo management

Immich offers camera backup, motion photos, faces, maps, duplicates and semantic search; this is far beyond “photo playback.” No Portico implementation found (K). **Value:** replace a paid photo cloud. **Size:** very large ongoing product. **Conflict:** upload authority, privacy, machine-learning cost, backups and broad mobile background constraints. **Opinion:** likely a distraction unless Justin explicitly wants a personal-cloud suite. Integrate read-only catalogs or casting before competing with specialist backup software; photos require exceptionally trustworthy originals handling. [Immich features][S-immich]

#### Television, collections, and operations

### F-23.41 TV Everywhere source integration

Channels supports provider-login channels; Portico accepts HDHomeRun/M3U/Xtream but no TVE connector found (K/PC8). **Value:** owned subscription channels in one guide/DVR. **Size:** large native maintenance, medium documented external bridge. **Conflict:** upstream licensing, credentials, website automation and DRM fragility. **Opinion:** prefer Channels interoperability. Don't promise recovery of a channel whose provider adopted inaccessible DRM; supported provider/version lists and source diagnostics are essential. [Channels TV Everywhere][C-tve] [Channels TVE/DRM failures][C-complaint]

### F-23.42 Library Channel export to M3U/XMLTV or virtual tuners

Portico already builds Library Channels; no outward M3U/XMLTV/HDHomeRun-compatible product route found (K). Tunarr exposes interoperable streams/guides. **Value:** existing IPTV devices and other servers can consume Portico channels. **Size:** medium. **Conflict:** unauthenticated legacy clients and shared channel policy. **Opinion:** good ecosystem expansion if grants, guide restrictions and tuner limits are explicit. Provide scoped revocable source URLs; “on LAN” must not silently authorize the whole server. [Tunarr streams and sources][S-tunarr] [Tunarr XMLTV export][S-tunarrepg]

### F-23.43 Advanced station clocks, filler, bumpers and channel graphics

Portico's schedule/filter channel core exists; precise filler/clock/branding parity was not established. ErsatzTV provides detailed playout instructions. **Value:** curated cartoon blocks, themed evenings and real lean-back TV. **Size:** medium–large. **Conflict:** persistent generation/transcoding and shared versus per-profile rights. **Opinion:** optional enthusiasts' depth. First compare current scheduling with the dedicated channel auditor; add only proven deltas, and keep simpler repeat/shuffle channels easy to create. [ErsatzTV playout instructions][S-ersatz]

### F-23.44 Kometa-style external collection and metadata rules

Portico has saved queries/static collections, not a verified rule importer/adapter (K). Kometa automates lists, metadata, art, and playlists. **Value:** awards, franchises, seasonal collections and repeatable curation. **Size:** medium adapter/declarative engine. **Conflict:** external datasets/licenses and accidental override of manual fields. **Opinion:** prioritize a stable authoring API and exportable rule format, perhaps a Kometa provider, rather than copying its whole scheduler. Version rules and maintain membership incrementally. [Kometa rule files][S-kometafiles] [Kometa overview][S-kometa]

### F-23.45 Reversible overlays and artwork treatments

Kometa and Emby CoverArt generate badges/treatments from quality/ratings/format. No Portico automation found (K). **Value:** power users identify HDR/4K/audio/collections at a glance. **Size:** medium. **Conflict:** uncluttered poster design and copyrighted rating/art sources; changing bytes loses original choices unless preserved. **Opinion:** optional owner styling, rendered/generated as a derivative of original art. Mobile/TV overlays need readability tests. Don't force metadata inspection text into every poster by default. [Kometa rule files][S-kometafiles] [Emby Premiere pricing][E-price]

### F-23.46 Shared smart collections as curated entities

Portico already stores live browse definitions, so “add smart filters” is not a gap. Saved collections reject a query definition while views accept one (`catalog/resources.go:514`). Plex combines filtering with collection artwork/publishing. **Value:** one shared curated shelf that updates automatically. **Size:** medium schema/presentation reconciliation. **Conflict:** avoid two independent query engines and cross-viewer personalization leaks. **Opinion:** extend/alias existing saved views where appropriate. Carefully define whether watched-state predicates mean owner, viewer or no personal state. [Plex collections][P-collections]

### F-23.47 Richer history queries, exports, and third-party stats adapters

Portico's owner history/summary exists; Backlog explicitly lacks title search, per-row delete, title-page “who watched,” and current-history CSV integration (PC9/15). Tautulli/Jellystat offer deeper reporting. **Value:** troubleshoot formats, understand demand and manage retention. **Size:** medium bounded indexed reports/adapter. **Conflict:** viewer privacy and permanent admin-log decision. **Opinion:** implement requested operational questions and export contracts, not duplicated analytics infrastructure. Add indexes/maintained summaries and expose covered time windows honestly. [Tautulli API][S-tautapi] [Jellystat releases][S-jellystat]

### F-23.48 External conversion worker integration and in-place replacement contract

Tdarr/Unmanic replace/remux media through plugin workers. Portico has prepared conversions and source-revision mechanisms, but no named automation adapter (K); Backlog reports replacement-scan problems (PC15). **Value:** owners optimize storage without confusing the server. **Size:** medium connector/acceptance, large distributed built-in encoding. **Conflict:** original preservation and analysis invalidation; physical paths may stay identical. **Opinion:** document a completion/invalidation contract and verify existing scanning fixes before adding workers. A new byte stream must retire old duration/chapters/markers/gain/download artifacts. [Tdarr worker types][S-tdarr] [Unmanic workflows][S-unmanic]

### F-23.49 Off-app operational/request alerts through optional adapters

Plex/Tautulli and Seerr can notify owners/users without an app open. Portico intentionally excludes server email and push (PC7). **Value:** failed disk/recordings and fulfilled requests are noticed promptly. **Size:** medium webhooks/external adapter; large own push infrastructure. **Conflict:** explicit product principle. **Opinion:** retain the no-email/no-push default and support user-operated webhook destinations if desired. Deliberate exclusion remains a difference worth recording; a recorder used for irreplaceable live broadcasts may justify rethinking alert delivery. [Tautulli overview][S-tautulli] [Seerr features][S-seerr]

#### Reach, customization, and strategic scope

### F-23.50 Optional paid relay with a sustainable quality contract

Explicit Backlog plan (PC15), not a newly discovered feature. Plex provides constrained fallback, while Emby Connect doesn't relay. **Value:** CGNAT/blocked ports/nontechnical remote setup. **Size:** large service/protocol/operations. **Conflict:** Hosted stays cheap and fixed quality; paid transport can address cost if terms are clear. **Opinion:** strategically valuable, but estimate per-hour egress, concurrent throughput and support before promising unlimited lifetime service. Preserve direct access and original-quality choices. [Plex Relay][P-relay] [Emby Connect boundary][E-connect]

### F-23.51 Guided remote-access and failure diagnosis

Certificates alone don't fix NAT or upload capacity. Jellyfin docs expose setup work; Plex offers mapping/relay/troubleshooting. Portico networking settings exist, but comparable real-world setup acceptance wasn't established (PC3). **Value:** fewer abandoned installs and fewer support messages. **Size:** medium diagnostics/guides. **Conflict:** no periodic Hosted chatter; use owner-triggered probes and event-based checks. **Opinion:** high priority even without relay. Explain local versus remote URL, double/CGNAT, DNS rebinding, VPN/reverse proxy and certificate problems separately. Never label mere directory presence as reachability. [Jellyfin networking][J-network] [Plex remote setup][P-remote] [Plex secure connections][P-secure]

### F-23.52 A desktop playback application/companion

Backlog explicitly plans macOS/Windows companion; browsers already work (PC15). Plex/Emby/Kodi/VLC and Jellyfin desktop demonstrate demand. **Value:** keyboard playback, local engine formats, downloads/system controls and home theater output. **Size:** medium–large. **Conflict:** maintenance/platform spread, not server authority. **Opinion:** distinguish “server tray/installer companion” from “desktop player” in requirements. An app shell around browser playback won't achieve broad codec/output improvements automatically. Linux scope is an open decision. [Plex apps and devices][P-platforms] [Emby feature matrix][E-matrix] [Jellyfin client inventory][J-clients] [VLC features][S-vlc]

### F-23.53 Additional living-room and Vision platform reach

Verified Portico scope includes new ports, not a qualified Samsung/LG/console/Vision fleet (PC7/K). Plex/Emby broadly cover TVs; Infuse offers Vision. **Value:** households use devices they already own. **Size:** large ongoing platform program. **Conflict:** sole-owner support burden. **Opinion:** prioritize actual target-device demand; third-party protocol support can fill gaps sooner. Roku is excluded from this runner by the brief, so no absence claim about its existing checkout. Build output is not device acceptance. [Plex apps and devices][P-platforms] [Emby architecture and apps][E-about] [Infuse product and prices][I-main]

### F-23.54 Non-English UI and localization acceptance

Architecture says English launch with regional spelling and other languages planned (PC7). Infuse lists many locales and Jellyfin uses community translations. **Value:** household accessibility and international adoption. **Size:** medium infrastructure/translation, large sustained review across UI/device input. **Conflict:** none, but raw translated strings and RTL can violate authored layout bounds. **Opinion:** design exists; choose actual launch locales and test truncation, plurals, metadata language versus UI language, dictation and bidirectional text. This is not a claim that i18n architecture is missing. [Infuse store formats][I-formats] [Jellyfin client inventory][J-clients]

### F-23.55 Controlled provider/plugin extension points

Jellyfin/Emby plugins and Kodi addons address niches; Plex now publishes metadata-provider API surfaces. Portico lacks a generic runtime extension engine (K). **Value:** owners solve obscure metadata/integration needs without waiting for Justin. **Size:** large plugins, medium external provider/service protocol. **Conflict:** no external code contributions, consistent authored product, trust and resource bounds. **Opinion:** consider external declarative/provider protocols first. Open-source license does not itself require accepting patches or running arbitrary owner code inside the server. [Plex published PMS API][P-api] [Jellyfin plugins][J-plugins] [Kodi add-ons][K-addons]

### F-23.56 Optional themes/skins and authorable layouts

Portico already allows Home arrangement and some appearance settings, so basic customization exists. Kodi skins and Emby premium themes go further. **Value:** accessibility, display density, household tastes. **Size:** medium themes, large arbitrary skins. **Conflict:** first-party consistency and fixed player geometry. **Opinion:** limited tokens/themes and row controls can satisfy most demand. Record full skins as a deliberate alternative; only user research should overturn the authored UI contract. [Kodi skins][K-skins] [Emby feature matrix][E-matrix]

### F-23.57 Universal availability and absent-title discovery

Plex searches beyond owned libraries; Portico's recommendation engine is already present but locally scoped (PC6/K). **Value:** answer where a wanted film can be watched, or requested. **Size:** large current regional availability service; medium links/external request catalogs. **Conflict:** data/service licensing, cheap Hosted and no clutter. **Opinion:** prefer opt-in external discovery or Seerr integration. Keep local and external results visibly distinguished; never silently substitute an ad stream for a user's local title. [Plex Discover][P-discover] [Plex Universal Watchlist][P-watchlist]

### F-23.58 Public lists, reviews, discussions and a cross-server social graph

Plex's 2026 social rollout, Trakt and Simkl extend discovery beyond a household. No equivalent Portico implementation found (K). **Value:** trusted recommendations/community. **Size:** large service, moderation/privacy and permission work. **Conflict:** privacy, local authority and Hosted cost. **Opinion:** optional external connectors are the better first step. Portico's opt-in household “also watched” already provides some social benefit; do not call all social playback missing. Clearly separate publication from internal owner logging. [Plex social announcement][P-social26] [Plex discussions launch][P-discussions] [Simkl tracking ecosystem][S-simkl]

### F-23.59 Ad-supported content and rentals

Plex has both; Portico deliberately does not act as a commercial distributor (PC1/K). **Value:** immediate catalog for users with no personal files and possible funding. **Size:** very large business/platform change. **Conflict:** free non-commercial identity, licensing/DRM/privacy and uncluttered navigation. **Opinion:** do not adopt by default. Keep the difference on record because some viewers pick a free content service, not a server. Owner-added lawful live sources are a separate capability Portico already has. [Plex free versus paid][P-free] [Plex rentals launch][P-rentals]

### F-23.60 Reliable edition/version grouping at ingestion

Backlog reports same-title alternate files becoming duplicate titles; NFO parser accepts edition labels and the product has source/version surfaces (PC4/15). Plex treats editions differently from technical versions; Infuse offers explicit server version choice. **Value:** select 4K/1080p or theatrical/extended without losing progress. **Size:** medium–large scanner identity/UI semantics. **Conflict:** need stable identities, not title-only deduplication. **Opinion:** important completeness work already known; inspect current ingestion fixes before declaring a live bug. Migrate editions independently and preserve per-cut duration/resume. [Plex multiple editions][P-editions] [Infuse release history][I-releases]

### F-23.61 First-party app feature completion and acceptance

Backlog lists native playlist/history/recording gaps, Cast subtitle handling and performance-info omissions; new ports lack physical acceptance (PC7/15). Competitors' broad app listings do not prove parity either. **Value:** one household experience that behaves consistently. **Size:** medium per missing surface, large qualification fleet. **Conflict:** none with shared structure, but platform limitations need honest UI. **Opinion:** prioritize completing and physically verifying existing promised features before adding new media domains. Treat “has a module” separately from reachable user actions. No tests were run here.

### F-23.62 Published compatibility policy, SDK recipes and example adapters

Portico publishes schemas and a complete product API, but pre-release rules allow deletion/restructuring; no named ecosystem adapter found (K/API README). Plex's versioned API and established tools show the benefit of stable external contracts. **Value:** others can build players/request/statistics tools with confidence. **Size:** medium documentation/fixtures/SDK/reference adapter; ongoing version support. **Conflict:** no need to preserve pre-release bugs, but post-release commitments consume attention. **Opinion:** pick a supported external version and realistic examples before release. Define token scopes, event ordering, cursors, rate limits, media grants, and deprecation policy. [Plex published PMS API][P-api] [Tautulli API][S-tautapi]

### F-23.63 DLNA with a clearly documented permission boundary

Explicitly planned in Backlog; generic route comments don't implement it (PC15/K). Emby and other players support the ecosystem. **Value:** older TVs/receivers without a maintained app. **Size:** medium–large discovery, browse and playback profiles. **Conflict:** the brief/backlog says whole-server exposure when enabled, whereas reconciled decisions describe an authenticated capability-limited boundary; this must be settled. **Opinion:** preserve off-by-default, favor scoped interoperability, and make source/access restrictions visible before enablement. See doc disagreements; no real exposure was established. [Emby architecture and apps][E-about] [Infuse NAS/file protocols][I-share]

### F-23.64 Optional player enhancements with truthful quality claims

Infuse advertises AI video upscaling; its store notes also include higher-quality deinterlacing. Portico has real server deinterlacing/scaling paths (`server/internal/decoder/hardware.go:522`) but no named client AI upscaler found. **Value:** improve older/interlaced material on large displays. **Size:** medium specific filter, large licensed/model-based cross-platform pipeline. **Conflict:** intentional image processing and extra device compute, not a reason to change server quality silently. **Opinion:** low priority until actual viewing tests justify it. Preserve original rendering as a choice and distinguish ordinary scaling from inferred detail; marketing claims do not establish reconstruction accuracy. [Infuse product and prices][I-main]

### F-23.65 Opt-in cinema trailers/pre-roll and richer extras presentation

Portico has trailer/featurette/behind-the-scenes extra types (`packages/client-core/src/detail.ts:8`), so extras themselves are not missing. I found no automatic cinema-trailer/pre-roll sequence in the searched implementation. Plex's Extras settings choose local/upcoming trailers and pre-roll clips; Emby offers Cinema Intros. **Value:** home-theater atmosphere and discovery. **Size:** medium queue/presentation, larger licensed online trailer supply. **Conflict:** avoid automatic promotional clutter and unintended watch-state writes. **Opinion:** optional enthusiast feature; preserve immediate main-feature playback and separately scope parental eligibility for every inserted clip. [Plex cinema extras][P-extras] [Emby Premiere pricing][E-price]

## What users dislike — opportunities and traps

This section separates verified policy changes from anecdotal dissatisfaction. A support post is not a survey. A limitation can also be a reasonable choice that benefits another user. Portico should avoid promising it has solved a complaint without testing the relevant boundary.

### Plex

**Cloud authority and changed entitlements.** Users experience remote playback on their own uplink as ownership, so a new paid requirement feels different from paying for a vendor's bandwidth. The April 2025 forum thread records objections; official policy preserves remote access for shared users when the owner has Plex Pass. Portico can advertise free reachable direct access, but should explain that “free” does not promise a free future high-bandwidth relay. [Plex paywall objections][P-paycomplaint] [Plex remote requirements][P-entitlement]

**Price trust.** A 2025 lifetime-price comparison is obsolete after July 2026. Existing lifetime holders retained access, which must be said fairly. The opportunity is predictable scope and sustainable pricing; the trap is promising a lifetime infrastructure benefit the project cannot fund. Publish exactly which local capabilities never need payment and which optional hosted services have ongoing costs. [Plex 2026 pricing][P-price26]

**Clutter and redesign regressions.** The commercial and personal catalog share navigation. Some people value that; others want only their files. A May 2025 complaint documents lost Roku music; the original specialist-app explanation and later restoration roadmap show changing direction, not one settled universal behavior. Watch Together retirement is explicitly documented. Portico should preserve capability access through UI changes and allow unwanted rows/destinations to be hidden without silently disabling library types. [Plex missing music report][P-musiccomplaint] [Plex specialist-app rationale][PA-specialist] [Plex Watch Together notice][P-together]

**Privacy communication.** Plex's November 2023 clarification says Discover Together is opt-in, explains that saving the shown onboarding values can make activity visible to friends, and acknowledges default weekly emails surprised people. Marketing unsubscribe and transactional/social email preferences differed. This historical incident is relevant even though the feature predates the requested 2024–2026 window. Treat deliberate cloud watch-state sync and social activity as separate from claims about PMS data collection. [Plex social privacy clarification][P-socialprivacy] [Plex cloud watched-state sync][P-sync]

**Incidents.** Official September 2025 notice identifies accessed account information including hashed passwords and authentication data. An official support discussion also records server re-claim friction following the August 2022 incident. This does not prove a personal-file breach. Central accounts concentrate risk; Portico Hosted also has that responsibility, even with less collected data. Test recovery without taking local owner control hostage. [Plex security notice][P-incident25] [Plex reclaim discussion][P-incident22]

**Privacy-policy interpretation.** Plex's March 2025 policy/announcement discusses additional data uses and consent while explicitly excluding sale/sharing of personal-server media/use. Do not turn user suspicion into a factual allegation that Plex sells the titles of their local library. The genuine opportunity is a short, accurate map of what leaves a Portico server and why, including metadata lookups and optional connectors. [Plex privacy policy revision][P-policy25]

### Jellyfin

**Setup friction is documented, not merely reputation.** Official setup/networking/GPU documentation sends owners through network exposure, reverse proxies/certificates/VPN choices and device/driver/container setup. The freedom is valuable, but families who only want playback can struggle before reaching it. Portico's certificate/directory assistance can help; it still doesn't remove NAT/GPU/permissions work automatically. [Jellyfin networking][J-network] [Jellyfin setup wizard][J-setup] [Jellyfin GPU pipelines][J-hardware]

**Client inconsistency.** Official/community clients intentionally differ in engines/platform focus, so codec, subtitles, downloads, music, input and administration support vary. That architecture presents an opportunity for Portico's shared behavior, but matching React Native code is not evidence of equal physical rendering, decoder output or remote handling. Third-party clients are also a Jellyfin strength, not merely fragmentation. [Jellyfin client inventory][J-clients]

**Music/recommendation depth.** Old summaries saying Jellyfin has only weak tag-based recommendations are now unsafe. The September 2026 release documents selectable recommendation sources and a ListenBrainz similar-artist source. That doesn't prove Plexamp-level radio or consistent client DSP. Compare current APIs/features, not a stale “no recommendations” slogan. [Jellyfin 12.0 release][J12]

**Large libraries and upgrades.** Historical complaints are plausible areas to measure, not proof Portico is faster. Jellyfin 12.0 specifically documents relational playlist/collection rows and faster major browse surfaces; its upgrade also needs backup/scan and affects old plugins/clients. Portico's O(visible) aim is still worth preserving, but comparative latency/correctness must be measured on the same corpus. [Jellyfin 12.0 release][J12]

### Emby

The matrix's split between free browsing, mobile unlock, licensed server Premiere, supported device limits and app-specific rules produces understandable confusion. A May 2023 forum thread captures a paying user's frustration; the team explains the license belongs to the server, not an unlimited user identity across other people's servers. It is dated evidence, not today's device-policy proof. [Emby license confusion][E-complaint] [Emby feature matrix][E-matrix]

Premium hardware/tone mapping, DVR/downloads, intro skip and other functions are a legitimate paid-business choice. Portico can differentiate with a free complete local core, but should not hide necessary technical capabilities behind a future hosted account or ambiguous store purchase. Proprietary development also limits audit/fork options, while vendor continuity may benefit users who want official support. [Emby proprietary transition][E-history]

### Infuse

Price dissatisfaction is real but not uniform: a February 2026 thread argues about lifetime price, regional exchange/tax differences and subscription avoidance; others consider daily use worth the cost. The product offers a lifetime option, so “subscription-only” is false. Portico should distinguish free software from Apple's store/platform costs and avoid a no-cost promise dependent on unpriced codec licenses. [Infuse price discussion][I-pricecomplaint] [Infuse purchases and sharing][I-purchase]

Metadata/artwork fetch slowness can hurt a player with excellent decode performance. November 2025 and March 2026 threads report delays; the latter has another user with a similar-size library reporting no issue. One reporter's “6000k episodes” string should not be interpreted as a verified six-million-item benchmark. Direct Mode reduces one class of sync work, but doesn't prove every slow setup is fixed. Portico's centralized projections can avoid repeated full-device catalog work; network/metadata limits remain. [Infuse fetching-details report][I-slow25] [Infuse performance discussion][I-slow26] [Infuse server modes][I-server]

Apple platform/output constraints matter. Infuse's TrueHD/DTS-HD decoding to LPCM is valuable, but not full TrueHD Atmos/DTS:X object passthrough. A user can prefer a Shield/Kodi-style setup despite a polished Apple UI. Portico needs equally precise route/codec explanations. [Infuse audio output][I-audio]

### Plexamp, Channels, Kodi, and the surrounding tools

| Product/group | Dislike or inherent burden | Portico opportunity / trap |
|---|---|---|
| Plexamp | Paid specialist discovery/output features, opaque similarity, separate app/platform gaps; historical fade/sleep/reconnect reports | Keep music first-class; don't remove TV listening before an alternative exists. Document lossless versus processed output and keep fixed album gapless behavior. [Plexamp feature split][PA-features] [Plexamp album fade rule][PA-fadesalbum] |
| Channels | Subscription plus provider costs; website login/DRM changes can remove channels; detecting ads is imperfect | Strong guide/recording failure UX; do not promise universal TVE access or copy destructive ad cutting. [Channels subscription][C-price] [Channels TVE/DRM failures][C-complaint] |
| Kodi | Configuration and skin/add-on maintenance, shared-database setup, variable device/audio output | Predictable first-party UX is valuable. Avoid abandoning interoperability merely to preserve visual control. Treat third-party executable code as an explicit trust boundary. [Kodi add-ons][K-addons] [Kodi syncing and sharing][K-share] |
| Seerr / *arr / Bazarr | Several daemons, tokens, path mappings and notification policies; acquisition status can race scanning | Publish one reliable completion API and reusable adapter examples; avoid embedding every acquisition service in the media server. [Seerr features][S-seerr] [Sonarr events and metadata][S-sonarr] |
| Tautulli / Jellystat | Additional privileged history store; missed/duplicate activity and retention alignment | Built-in history is valuable; make exports and event provenance clear. Query/graph depth remains an opportunity. [Tautulli API][S-tautapi] [Jellystat releases][S-jellystat] |
| Trakt / Simkl | Cloud reliance, external account/premium terms, metadata matching and changing service behavior | Optional portable state; never link a whole household to one user's scrobbling token or publish kids' activity implicitly. [Infuse Trakt sync][I-trakt] [Simkl tracking ecosystem][S-simkl] |
| Tdarr / Unmanic | File mutation, worker/cache/path complexity, optional paid distributed features | Be resilient to original changes; keep prepared copies separate. Distributed jobs should not turn background optimization into media loss. [Tdarr worker types][S-tdarr] [Unmanic workflows][S-unmanic] |
| ErsatzTV / Tunarr | Another scheduler/transcoder, metadata and program schedule mismatches | Library Channels already reduce setup; prioritize predictable channel behavior before broadcast-style customization. [ErsatzTV playout instructions][S-ersatz] [Tunarr streams and sources][S-tunarr] |
| Audiobookshelf | Dedicated extra server; companion-app maturity/format/sync boundaries | Portico can unify common listening, but specialist workflows remain real gaps. [Audiobookshelf introduction][S-abs] |
| Kavita / Komga | Separate library, reader-extension and progress-sync limitations | Interoperability beats pretending an EPUB is a video. KOReader chapter-level loss demonstrates why sync precision must be stated. [Komga KOReader progress limits][S-koreader] |
| Immich / PhotoPrism | Operational backup/storage/privacy/ML complexity despite impressive features | A photo display integration can be valuable; replacing a photo backup app creates a much larger reliability promise. [Immich features][S-immich] [PhotoPrism indexing/originals][S-photooriginals] |
| Stremio | Add-on/source/account dependencies and changing provider availability | Separate catalogs from stream authority and clearly identify source trust. Remote addons do not remove all security/privacy risk. [Stremio features and guest/addon model][S-stremio] |
| VidHub/SenPlayer/Fileball/nPlayer/VLC | Variable polish/protocol/output/platform scope, small-vendor continuity or file-first rather than household-library behavior | A simple open-file path helps; don't infer every listed format or marketing resolution works on every hardware route. [SenPlayer store description][S-sen] [Fileball/Filebar store description][S-fileball] [nPlayer features][S-nplayer] |
| Symfonium | A configurable client has full-sync/provider-field complexity and Android-only reach | Server APIs can unlock specialist clients; thin clients cannot automatically provide their offline/customization depth. [Symfonium backend feature matrix][S-symproviders] |
| Roon | Higher subscription/lifetime cost and endpoint complexity; rich metadata needs vendor services | Transparent audio/credits is a useful bar. Do not repeat obsolete always-online claims: offline behavior was restored in February 2024. [Roon pricing/product scope][S-roon] [Roon offline restoration][S-roonoffline] |

## Possible bugs and risks in Portico

**No B-23 entries.** This lane did not establish a runtime bug, and it would be misleading to manufacture one from a competitor's checkbox. The following are questions for the relevant code/device auditors, not claims of broken behavior:

- Does local-account login, profile management and already authorized playback work through a Hosted outage, expired directory/certificate, and an unclaimed server? Does recovery preserve a local owner rather than require an insecure network bypass? Compare Plex re-claim friction (PC2/3).
- Do direct names and certificate renewal handle DNS rebinding protection, NAT loopback, CGNAT, local IP changes and multi-server accounts without reporting a directory entry as reachable? Does a connection state clearly identify remote versus direct-local paths (PC3)?
- Do marker skips preserve mid/post-credit scenes, manual edits, changed files and language/cut differences? The current detector's low-confidence semantics are explicit; confirm UI doesn't authorize candidates by preference alone (`mediaanalysis/detectors.go:51`).
- When a background decode is cancelled because playback becomes busy, does it resume work without repeated expensive rereads/starvation? What exactly does “never paused” mean against `ingestion/deep_analysis.go:45`? No starvation reproduced.
- Do subtitle offsets and any future alignment survive seek, session/rendition changes, timestamp discontinuity, HLS conversion and offline download? Distinguish source subtitle revision from presentation timing (`playbackv1/subtitles.go:35`).
- When Tdarr/*arr overwrites or atomically replaces a file at the same path, do duration, stream facts, thumbnails, loudness/trim, markers, subtitles and prepared downloads all invalidate correctly? Backlog already records a replacement problem; this lane did not verify it remains current.
- Do source/version selection and queued next episode honor the intended cut/resolution instead of replaying an old candidate? Infuse/Plex 2026 notes show that version and asynchronous player replacement are recurring product scars. See `I-releases` and `P-release26`.
- Do profile switches isolate active playback ownership, download cache, remote controls, artwork/history and pending offline writes? A PIN does not make a device-wide cache per-profile automatically (PC13).
- Are restricted items and their titles/art absent from shared queries, group metadata, notifications, guide exports and statistic APIs, including while restrictions change mid-page? Saved views using watched predicates must bind a viewer explicitly (PC2/6).
- Do long audiobook clocks, multi-file chapters, speed changes and changed sources preserve progress across online/offline clients? Can one-time next-N selection be confused with an evergreen refill policy (`downloads/requests.go:320`)?
- Does platform output preserve the advertised HDR/Dolby Vision and E-AC3/TrueHD/DTS behavior, and do diagnostic labels distinguish decode, PCM, object audio and passthrough? Test physical HDMI/DAC routes, not emulator codec enums.
- Can a future webhook/import path be abused for SSRF, root-path escape, credential disclosure or unrestricted scans? Can imported passwords/tokens accidentally become native authority? These are interface-design questions, not present vulnerabilities.

## Where Portico is stronger

1. **Owner authority with optional convenience.** Direct local accounts, PIN profiles and a server-authoritative membership model can avoid normal cloud-login dependence while still helping with certificates and discovery. Keep this credible under outages, not merely in a happy-path diagram.
2. **A free complete local core.** Hardware/transcode/download/DVR categories are not subscription permissions in the stated model. “No payment for your own reachable uplink” is a concrete distinction from Plex's changed remote-video policy.
3. **A coherent multi-media product.** Music/audiobooks/DVR/Library Channels/Watch Together already have domain models. Splitting everything into specialist apps can strand a TV platform; Portico has an opportunity to keep basic functionality consistently reachable.
4. **Built-in administration and history.** Owner operations, plain backups, diagnostics, retained play history and counts cover needs that often send Plex/Jellyfin owners to another daemon. Rich export and retention explanations can strengthen this without needless data duplication.
5. **Intentional large-library mechanics.** Maintained derived data, bounded reads, server sorting and visible-window clients are promising. Advertise these as an architecture until the agreed workload/device targets pass; Jellyfin's current improvements must be included in comparisons.
6. **Open, portable state and local evidence.** SQLite/files, local-only providers, NFO, selected art and manual provenance are valuable for users who want their curation to survive a vendor. Interoperable export would turn a good storage architecture into a visible trust advantage.
7. **Honest analysis safety.** Low-confidence candidates are kept separate from approved skipping. That is preferable to silently losing post-credit scenes. Accuracy and manageable review effort still need improvement.
8. **Shared behavior and authored presentation.** A setting's meaning and row ordering can stay coherent across platforms. Device acceptance, accessibility and format output must prove the benefit; shared code itself isn't the result.
9. **Hosted with a narrow remit.** Account/directory/certificate service without personal-media storage is a strong privacy/economic boundary. New relay/social/metadata services should be evaluated against it explicitly rather than creep in under an “integration” name.

## Doc/code disagreements and stale-public-source traps

| Observation | Evidence and boundary | Consequence for this report |
|---|---|---|
| Playback overview is still predominantly v1 | `04 Playback and Media.md:5` versus API README's `/v2` control model (`:34`) and existing control routes | Do not describe Portico as only the old session/offers API or claim remote controller architecture absent. No runtime use-path completeness established. |
| Generated JSON is not the complete route inventory | `portico.openapi.json` paths versus `api/README.md:7` describing `openapi.yaml` as complete | Gap searches also inspected full YAML/router; no endpoint absence inferred from JSON alone. |
| “One provider, as Plex and Jellyfin have it” is inaccurate comparator wording | `03 Catalogue and Metadata.md:66`; Plex modern agents aggregate and legacy sources have priority; Jellyfin has providers/plugins | Portico's deliberate rule is real, but the analogy should be corrected. `screen_review.go:237` and `administration_policy.go:84` reject >1 provider in the inspected current writes. |
| Older contracts still retain default/fallback choices | Decisions P10 (`Product Contracts/decisions.md:18`) says TMDB/TVDB selectable fallback and AniList default, while Oct overview/library agent describes one selected provider with TMDB anime default | Treat later user/architecture instructions as current intent; coordinate metadata audit to reconcile normative contracts. Do not call an accepted later choice an implementation bug. |
| Background “never paused” has a source-level exception or different meaning | Overview principle and `04 Playback and Media.md:19` versus `ingestion/deep_analysis.go:45`, cancelling when playbackBusy | Wording needs reconciliation; it could intentionally avoid source contention rather than globally pause work. No starvation proved. |
| DLNA planned exposure model conflicts between documents | `Backlog.md:95` says off by default, whole server exposed when on; decisions R19 (`Product Contracts/decisions.md:54`) says authenticated capability-limited boundary | No actual DLNA implementation/exposure proved. Resolve before interoperability design. |
| Backlog is dated and internally contains superseded details | Dated 2 Oct; native ports/demo/metadata fixes continued later, architecture says NFO cast + online crew while backlog retains older issue | Mark known gaps as backlog context, not today's confirmed bugs. User specifically forbade checking the demo. |
| Architecture says “Playback matches or exceeds Plex” | `04 Playback and Media.md:3`; current public Plex/Infuse features and unqualified Portico devices | This is an ambition, not an established comparison result. Add feature/device/analysis acceptance criteria before making an external claim. |
| AcoustID claims need algorithm precision | Overview music fingerprint description versus `music_inputs.go:155` and spectral analysis `produce.go:453` | Compatible embedded Chromaprint evidence can be used; private spectral signatures are neither AcoustID data nor a sonic model. No automatic generated AcoustID parity assumed. |
| Plex support pages can contradict themselves or lag rollout | Auto-Sync page lists both “never” and “scheduled+added” as defaults; Home page retains old mobile-unlock text; remote requirements page dated Apr 2026 lags Sep third-party discussion | Avoid asserting an exact default or universal app entitlement from older prose. Cite date and latest release/policy evidence separately. |
| Plex 2026 pricing and Infuse conversion change older comparisons | July lifetime increase; Infuse 8.5 server conversion controls | Use current prices and qualified “direct engine plus optional connected-server conversion.” |
| Roon internet requirement was reversed | Official 2.0.28 release restores offline behavior | Do not copy old always-online criticism as current. |
| Emby documentation host changed | `support.emby.media/...` indexed matrix failed direct HTTP; `https://emby.media/support/articles/Premiere-Feature-Matrix.html` fetched successfully | Use current resolved official URL in sources. Search snippets alone were checked against live matrix. |
| Jellyfin 2026 release updates reputation | Release dated 7 Sep documents performance, versions, books and recommendation source work | Historical large-library/music complaints remain opportunities to test, not current blanket deficiencies. |

## Open questions for Justin — market positioning and migration

These are recorded questions, not requests for a reply during this run.

### Target user and reason to choose Portico

- Is the primary owner a skilled homelab operator, a NAS enthusiast, a family replacing Plex, a movie-remux enthusiast, a music collector, or a household recorder? Who is the primary viewer? The owner can tolerate ten setup steps while their family cannot tolerate one playback failure.
- What is the one-sentence reason to choose Portico over Plex: **“your whole personal library, free direct remote access, with local control and no commercial clutter”**? That is a plausible proposed position, not an approved brand promise. Which words are actually committed?
- What is the one-sentence reason to choose Portico over Jellyfin: **“a consistent first-party experience and easier operation, designed for very large libraries”**? This requires physical-device, onboarding and comparative performance proof. Open source/free alone are not differentiators.
- Should a player-only user be a customer at all? Infuse/Symfonium demonstrate they can prefer a client over migrating a server. Is supporting their existing player a better adoption path than requiring every household to switch both pieces?
- Is “Plexamp-quality music” a release requirement, or only reliable music playback? Gapless/gain/lyrics/queues are present; radio, car UX and output transparency determine whether listeners experience parity.
- Are audiobooks a serious promise, and do they justify a separate listening surface or app? Are books/comics/photos truly in scope or should specialist servers stay integrated companions?
- Does “no external contributions” describe launch governance or permanent policy? Who will maintain growing codec/device/provider/API breadth if integrations succeed?

### Paid/hosted services and trust

- Which ongoing costs can remain free: account recovery/email, certificate issuance, DNS, app-store membership, metadata credentials, bandwidth, support and security response? No periodic chatter reduces traffic but doesn't remove fixed or incident costs.
- Can a paid relay charge for transported GB/hour/concurrency instead of permission to use direct access? What is the minimum usable original-quality ceiling, and how will caps/regions/outages be visible before a purchase?
- Would subscriptions, donations, optional managed setup, paid support or a hosted relay fund maintenance without changing the free local core? Which are compatible with the non-commercial project's actual licenses/provider terms? This report does not establish legal permission for a new commercial offering.
- Are any lifetime **infrastructure** promises affordable under worse-case usage and carrier prices? Plex's pricing history is a warning about liabilities, not proof all recurring charges are bad.
- What will Portico tell viewers about the owner's permanent play history, even when the viewer pauses/clears personal History? A privacy advantage must accurately describe admin visibility, metadata-provider requests and optional scrobbling.
- If quality never changes except on a user's request, is a deliberately selected adaptive mode allowed? If not, quality suggestions and buffering diagnosis become more important.

### Migration paths: existing tools and a concrete adoption contract

| Existing path/tool | What it can preserve | What it cannot safely be presumed to preserve | Source |
|---|---|---|---|
| Jellyfin migration docs | Link API-based watched/user scripts from Plex/Emby/Jellyfin | Direct modern Emby database import; paths/password authority/art/collections aren't universal | [Jellyfin migration boundary][J-migrate] |
| JellyPlex-Watched | Cross-server watched state with provider-ID/filename matching and configurable user mapping | A full metadata/art/user-auth migration, every edition/episode-order match, current Portico support | [JellyPlex-Watched project][M-jellyplex] |
| WatchState | Local sync/import/export of Plex/Jellyfin/Emby play state; webhook/scheduled workflows | Real-time progress equivalence across every backend; docs note Plex event limits; Portico adapter absent | [WatchState project][M-watchstate] [WatchState backend limitations][M-limitations] |
| Tautulli exporter | Movie/show/season/music/collection/playlist metadata and artwork resources in export formats | Destination schema/identity mapping, full personal state or user credentials | [Tautulli exporter guide][M-tautexport] |
| WebTools-NG ExportTools | Metadata/media info and chosen posters/background art; export settings and known CSV issues documented | A reliable full automatic migration with every field; separator/field/agent caveats matter | [WebTools-NG ExportTools][M-webtools] |
| Plex NFO Exporter | Metadata/posters/background images written beside files for another server to read | Complete private state, passwords, exact locks/source provenance, all server entities | [Plex NFO Exporter][M-plexexport] |
| Kodi NFO/export/import | Shared title facts/art pointers and some watched/resume tags, with explicit import choices | Per-person household authority; ambiguous/mixed-provider IDs or distinct cuts | [Kodi NFO convention][K-nfo] [Kodi movie NFO tags][K-movie] |
| Trakt/Simkl import/sync ecosystem | Portable external-ID watch history/lists, depending on account/entitlements/tool | Local posters/manual locks, byte sources, fine-grained resume, all editions, user passwords | [Infuse Trakt sync][I-trakt] [Simkl tracking ecosystem][S-simkl] |
| Seerr migration | Its own request-service database/config migration from Overseerr/Jellyseerr | A migration of the underlying media server's library/users | [Seerr migration guide][S-seerrmigration] |

These are documented paths, not tested or recommended installations. No third-party scripts were run. Their current releases, security and completeness need a dedicated implementation review before use.

A Portico wizard could follow a reviewable sequence: connect read-only to the old server; inventory libraries/users/external IDs; map source roots and destination profiles; show counts and ambiguous identities; scan existing files without copying; import shared facts/art/collections; then import each person's state with timestamp/conflict rules; reconcile totals; offer optional coexistence sync; provide a reversible import receipt and an export. The wizard should work against bounded pages and durable stages so a million-item catalog is not one transaction or one browser JSON object.

The source identity of a title needs more than “name/year.” Provider IDs are typed by provider and media type. Episodes need a declared order and number; special/anime/absolute/DVD order can disagree. Editions/versions need cut and source information. Music has release-group, release, recording and track identities; books need narrator/edition/file/chapter clocks. An unknown match should stay unresolved, not be guessed into a wrong watched record.

Questions that only Justin can settle:

- What must migrate for “it just works”: watched/unwatched, in-progress seconds, ratings, favorites/My List, play dates/counts, collection order/posters, manual locked descriptions, playlists with duplicate entries, or every source fact?
- Will import preserve source dates and manual art by default, or prefer freshly fetched metadata? Can a user select per-field rules and see provenance?
- Should imports create accounts/profiles and invite users, or require explicit destination profiles? Imported names/settings can be copied; password hashes, PIN hashes, server tokens and SSO identities must not silently create authority.
- Should old/new servers coexist? Which wins when one client marks unwatched, seeks backward or removes a rating after another finished playback?
- Is initial adoption read-only against media folders, including export sidecars? Owners may want Portico to leave a working Plex setup completely intact until the family accepts it.
- What are the acceptance counts: matched/unmatched/ambiguous works, migrated state/art/collections, skipped restricted items, resumed interrupted stages? Will the wizard produce a human-readable reconciliation report?

## Evidence boundary

No competitor or Portico was installed, executed, signed into, benchmarked, or played. Closed-source implementation internals, database query plans, decoder/model internals, and private backend costs are unknown. The survey uses official public sources where possible and dated forums only for complaints or unresolved rollout details. Product webpages and store listings establish advertising, not device acceptance. Static Portico review supports specific paths and bounded absence searches; it does not prove every UI is reachable or that a current backlog issue remains unfixed.

The review did not audit Roku code, Plezy code, every specialist server's source, provider legal terms, purchase flows, plugin execution security, or exact current price in every region. Apple TV/Netflix/Disney+ are UX references only. End-to-end import coverage, true HDR/object-audio output, offline behavior under revocation, relay throughput, server capacity, performance at millions of entities, recommendation accuracy and physical Android/Fire/Vega parity remain unestablished.

Failed/redirected public URLs and undated sources are identified below rather than silently treated as verified. `docs.emby.media` produced a TLS hostname mismatch during plain HTTP fetch; equivalent `emby.media` pages were fetched successfully without bypassing certificate validation. Plex download-page redirects looped in HTTP; its Apps & Devices page was fetched instead. The old `plex-dash` support slug resolved to connectivity tips, so Dash capabilities are cited to its official release thread. “Undated; retrieved 2026-10-11” is a date boundary, not an inferred publication date. Documentation can change after retrieval. For mutable pages the cited current page plus its publication/last-modified date is the evidence; no immutable web archive was created.

## Public source register

All sources were searched or fetched on **11 October 2026**. Dates in the register are publication/visible update dates or structured page metadata where available. “Live, undated” means no reliable article date was exposed; the retrieval date is the only established date. A source may redirect to its maintained equivalent. Repeated linked source names below use reference-style Markdown definitions so every claim/table cell resolves to the actual URL.

| Source | Publication/update boundary |
|---|---|
| [Plex Relay][P-relay] | 2025-07-14 update |
| [Plex secure connections][P-secure] | 2025-07-24 update |
| [Plex remote setup][P-remote] | 2026-09-08 structured update/publication; retrieved 2026-10-11 |
| [Plex remote requirements][P-entitlement] | 2026-04-10 visible update |
| [Plex 2025 changes][P-price25] | 2025-03-19; updated 2026-04-29 |
| [Plex 2026 pricing][P-price26] | 2026-05-19; updated 2026-07-01 |
| [Plex preview announcement][P-redesign] | 2024-11-22; updated 2025-01-22 |
| [Plex mobile launch][P-mobile] | 2025-03-31 |
| [Plex social announcement][P-social26] | 2026-06-03 |
| [Plex discussions launch][P-discussions] | 2026-06-10 |
| [Plex September release][P-release26] | 2026-09-28 |
| [Plex third-party rollout discussion][P-thirdparty] | September 2026; entitlement wording unresolved |
| [Plex security notice][P-incident25] | 2025-09-08 |
| [Plex reclaim discussion][P-incident22] | 2022-09-19; discusses August 2022 incident |
| [Plex privacy policy revision][P-policy25] | March 2025 revision |
| [Plex social privacy clarification][P-socialprivacy] | 2023-11-28; historical |
| [Plex paywall objections][P-paycomplaint] | 2025-03-19 |
| [Plex missing music report][P-musiccomplaint] | 2025-05-21; anecdotal |
| [Plex account requirements][P-account] | 2025-09-03 structured update/publication; retrieved 2026-10-11 |
| [Plex published PMS API][P-api] | Live API versions 1.0–1.2.x; no article date |
| [Plex metadata agents][P-agents] | 2025-11-18 update |
| [Plex scheduled tasks][P-tasks] | 2025-07-31 update |
| [Plex hardware acceleration][P-hardware] | 2025-09-11 update |
| [Plex HDR tone mapping][P-tone] | 2024-08-22 update |
| [Plex transcoder settings][P-transcoder] | 2026-09-30 update |
| [Plex Media Optimizer][P-optimize] | 2026-09-30 structured update/publication; retrieved 2026-10-11 |
| [Plex auto quality][P-auto] | 2022-01-05 structured update/publication; retrieved 2026-10-11 |
| [Plex Android TV settings][P-tvsettings] | 2024-06-24 structured update/publication; retrieved 2026-10-11 |
| [Plex Home][P-home] | Live page includes outdated mobile-unlock text |
| [Plex share restrictions][P-sharing] | 2026-02-21 update |
| [Plex rentals launch][P-rentals] | 2024-02-07 |
| [Plex Discover][P-discover] | 2025-11-07 update |
| [Plex Universal Watchlist][P-watchlist] | 2025-07-10 update |
| [Plex cloud watched-state sync][P-sync] | 2025-03-24 update |
| [Plex intro detection][P-intro] | 2024-08-19 update |
| [Plex credits detection][P-credits] | 2024-08-19 update |
| [Plex subtitle auto-sync][P-subs] | 2024-09-18 update; conflicting default prose |
| [Plex Watch Together notice][P-together] | 2025-02-25 retirement notice |
| [Plex webhooks][P-hooks] | 2025-05-21 structured update/publication; retrieved 2026-10-11 |
| [Plex multiple editions][P-editions] | 2026-07-07 structured update/publication; retrieved 2026-10-11 |
| [Plex collections][P-collections] | 2024-12-27 update |
| [Plex Series agent settings][P-seriesagent] | 2025-04-13 update |
| [Plex free versus paid][P-free] | 2025-04-29 structured update/publication; retrieved 2026-10-11 |
| [Plex state directory][P-data] | 2025-07-25 update |
| [Plex apps and devices][P-platforms] | Live, undated; retrieved 2026-10-11 |
| [Plexamp feature split][PA-features] | Live; search index supplied feature text, plain HTML JS shell |
| [Plexamp free launch][PA-free] | 2023-07-18; baseline still referenced |
| [Plex sonic analysis][PA-sonic] | Live; platform prose includes legacy details |
| [Plex specialist-app rationale][PA-specialist] | 2024-09-12; later music restoration roadmap supersedes scope |
| [Plex Dash release notes][PA-dash] | Rolling thread; 2.0 section November 2023, 2.0.1 December 2023 |
| [Plexamp output update][PA-rate] | 2024-09-21 |
| [Plexamp fade discussion][PA-fades] | 2022-09-26; historical, not current compatibility guarantee |
| [Plexamp album fade rule][PA-fadesalbum] | 2024-11-04; staff clarification |
| [Infuse product and prices][I-main] | Live, undated; retrieved 2026-10-11 |
| [Infuse store formats][I-formats] | Live Apple store listing; version/device-specific |
| [Infuse server modes][I-server] | 2026-05-29 live update; index had older date |
| [Infuse audio output][I-audio] | 2026-05-06 live update |
| [Infuse iCloud sync][I-cloud] | 2026-05-06 live update; index had older date |
| [Infuse Trakt sync][I-trakt] | 2026-05-06 live update |
| [Infuse release history][I-releases] | Rolling; 8.5 dated 2026-07-30, later September 2026 fixes |
| [Infuse purchases and sharing][I-purchase] | Live, undated; retrieved 2026-10-11 |
| [Infuse NAS/file protocols][I-share] | 2025-01-05 indexed update; live may be newer |
| [Infuse settings][I-settings] | Live, undated; retrieved 2026-10-11 |
| [Infuse price discussion][I-pricecomplaint] | 2026-02-19; anecdotes |
| [Infuse fetching-details report][I-slow25] | 2025-11-20; anecdote, network alternatives discussed |
| [Infuse performance discussion][I-slow26] | 2026-03-22; conflicting user experiences |
| [Emby Connect boundary][E-connect] | Live, undated; retrieved 2026-10-11 |
| [Emby architecture and apps][E-about] | Live, undated; retrieved 2026-10-11 |
| [Emby Premiere pricing][E-price] | Live, undated; retrieved 2026-10-11 |
| [Emby feature matrix][E-matrix] | Live; old support.emby.media URL returns 404 |
| [Emby proprietary transition][E-history] | 2024-11-22; historical 2018 transition |
| [Emby license confusion][E-complaint] | 2023-05-02; dated anecdote/team reply |
| [Channels subscription][C-price] | Live, undated; retrieved 2026-10-11 |
| [Channels TV Everywhere][C-tve] | Live; source marked Beta |
| [Channels Virtual Channels][C-virtual] | Live, undated; retrieved 2026-10-11 |
| [Channels remote setup discussion][C-remote] | 2021-08-05; historical port-forwarding evidence, current relay not established |
| [Channels enhanced detection beta][C-commercial] | 2026-01-14; prerelease/opt-in |
| [Channels commercial settings][C-commercialsettings] | 2022-12-09; historical settings rationale |
| [Channels TVE/DRM failures][C-complaint] | July–August 2026; user reports/release quotations |
| [Kodi database model][K-db] | Live wiki; some version examples old |
| [Kodi syncing and sharing][K-share] | Live, undated; retrieved 2026-10-11 |
| [Kodi NFO convention][K-nfo] | Live, undated; retrieved 2026-10-11 |
| [Kodi movie NFO tags][K-movie] | Live, undated; retrieved 2026-10-11 |
| [Kodi add-ons][K-addons] | Live, undated; retrieved 2026-10-11 |
| [Kodi skins][K-skins] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin networking][J-network] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin GPU pipelines][J-hardware] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin setup wizard][J-setup] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin library types][J-libraries] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin books formats][J-books] | Live format docs; supplement with 2026 release |
| [Jellyfin music metadata][J-music] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin movies/NFO/versions][J-movies] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin plugins][J-plugins] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin transcoding][J-transcode] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin migration boundary][J-migrate] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin users and parental controls][J-users] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin Live TV][J-live] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin client inventory][J-clients] | Live, undated; retrieved 2026-10-11 |
| [Jellyfin free/open model][J-about] | Live, undated; retrieved 2026-10-11 |
| [Intro Skipper README][J-intro] | Live maintainer README; not executed/source-audited |
| [Jellyfin 12.0 release][J12] | 2026-09-07 |
| [Seerr features][S-seerr] | Live, undated; retrieved 2026-10-11 |
| [Seerr merger announcement][S-seerrmerge] | 2026-02-10 |
| [Seerr migration guide][S-seerrmigration] | Live, undated; retrieved 2026-10-11 |
| [Tautulli overview][S-tautulli] | Live, undated; retrieved 2026-10-11 |
| [Tautulli API][S-tautapi] | Live, undated; retrieved 2026-10-11 |
| [Jellystat releases][S-jellystat] | Rolling; release dates not all exposed with year in index |
| [Trakt scrobbler announcement][S-trakt] | 2020-08-24; historical protocol existence, premium/current details unverified |
| [Simkl tracking ecosystem][S-simkl] | Live, undated; retrieved 2026-10-11 |
| [Sonarr events and metadata][S-sonarr] | Live, undated; retrieved 2026-10-11 |
| [Radarr events and metadata][S-radarr] | Live, undated; retrieved 2026-10-11 |
| [Bazarr subtitle processing][S-bazarr] | Live, undated; retrieved 2026-10-11 |
| [Kometa overview][S-kometa] | Live; docs.kometa.wiki did not resolve |
| [Kometa rule files][S-kometafiles] | Nightly docs explicitly; stable naming may differ |
| [Tdarr worker types][S-tdarr] | Includes mapped/unmapped and Pro restrictions |
| [Unmanic workflows][S-unmanic] | Live, undated; retrieved 2026-10-11 |
| [ErsatzTV playout instructions][S-ersatz] | Live, undated; retrieved 2026-10-11 |
| [Tunarr streams and sources][S-tunarr] | Live, undated; retrieved 2026-10-11 |
| [Tunarr XMLTV export][S-tunarrepg] | Live, undated; retrieved 2026-10-11 |
| [Audiobookshelf introduction][S-abs] | Live; companion apps labelled beta |
| [Kavita product][S-kavita] | Live, undated; retrieved 2026-10-11 |
| [Komga product][S-komga] | Live, undated; retrieved 2026-10-11 |
| [Komga OPDS client matrix][S-opds] | Live, undated; retrieved 2026-10-11 |
| [Komga KOReader progress limits][S-koreader] | Live, undated; retrieved 2026-10-11 |
| [Kavita OPDS capabilities][S-kavitaopds] | Live, undated; retrieved 2026-10-11 |
| [Immich features][S-immich] | Live, undated; retrieved 2026-10-11 |
| [PhotoPrism documentation][S-photo] | Live, undated; retrieved 2026-10-11 |
| [PhotoPrism indexing/originals][S-photooriginals] | Live, undated; retrieved 2026-10-11 |
| [Stremio features and guest/addon model][S-stremio] | Live page; some offline statements look legacy |
| [VidHub product scope][S-vidhub] | 2024-04-24 published; 2026-08-05 update |
| [SenPlayer store description][S-sen] | Live store listing; marketing/device claims untested |
| [Fileball/Filebar store description][S-fileball] | Live listing name Filebar; publisher site fileball.app |
| [nPlayer features][S-nplayer] | Live, undated; retrieved 2026-10-11 |
| [VLC features][S-vlc] | Live, undated; retrieved 2026-10-11 |
| [Symfonium product features][S-sym] | Live page cites v15.0.0 2026-06-10 |
| [Symfonium backend feature matrix][S-symproviders] | Live, undated; retrieved 2026-10-11 |
| [Roon pricing/product scope][S-roon] | Live page; hardware/music-service subscriptions separate |
| [Roon ARC FAQ][S-roonarc] | Live, undated; retrieved 2026-10-11 |
| [Roon signal path][S-roonpath] | Live, undated; retrieved 2026-10-11 |
| [Roon metadata graph][S-roonmeta] | Live, undated; retrieved 2026-10-11 |
| [Roon offline restoration][S-roonoffline] | 2024-02; official release |
| [WatchState project][M-watchstate] | Live README; not run or code-audited |
| [WatchState backend limitations][M-limitations] | Live maintainer guide |
| [JellyPlex-Watched project][M-jellyplex] | Live README; not run or code-audited |
| [Plex NFO Exporter][M-plexexport] | Live README; not run or security-reviewed |
| [WebTools-NG ExportTools][M-webtools] | Wiki; indexed publication 2023, current revision date unverified |
| [Tautulli exporter guide][M-tautexport] | Live maintainer guide |
| [Apple TV personalization][UX-apple] | Live official English-rendered help under regional URL |
| [Netflix inactivity convention][UX-netflix] | Live, undated; retrieved 2026-10-11 |
| [Netflix hidden history convention][UX-history] | Live, undated; retrieved 2026-10-11 |
| [Disney/Hulu profile migration][UX-disney] | 2026-era help; exact visible date not established |
| [Plex cinema extras][P-extras] | Live, undated; retrieved 2026-10-11 |

### Resolved citation URLs

[P-relay]: https://support.plex.tv/articles/216766168-accessing-a-server-through-relay/
[P-secure]: https://support.plex.tv/articles/206225077-how-to-use-secure-server-connections/
[P-remote]: https://support.plex.tv/articles/200289506-remote-access/
[P-entitlement]: https://support.plex.tv/articles/requirements-for-remote-playback-of-personal-media/
[P-price25]: https://www.plex.tv/blog/important-2025-plex-updates/
[P-price26]: https://www.plex.tv/blog/new-lifetime-plex-pass-pricing/
[P-redesign]: https://www.plex.tv/blog/the-new-plex-experience/
[P-mobile]: https://www.plex.tv/blog/its-go-time-the-new-plex-experience-is-here/
[P-social26]: https://www.plex.tv/nl/blog/plex-introduces-a-social-platform-for-entertainment-discovery-across-streaming-services/
[P-discussions]: https://www.plex.tv/blog/let-the-discussions-begin/
[P-release26]: https://forums.plex.tv/t/plex-for-mobile-tvs-android-mobile-ios-android-tv-tvos-fire-tv/909610/59
[P-thirdparty]: https://forums.plex.tv/t/infuse-devs-remote-streaming-will-require-a-plex-pass-remote-watch-pass-including-3rd-party-apps/943183
[P-incident25]: https://forums.plex.tv/t/important-notice-of-security-incident/930523
[P-incident22]: https://forums.plex.tv/t/cannot-reach-my-server/811168
[P-policy25]: https://www.plex.tv/en-gb/about/privacy-legal/revision-march-2025/
[P-socialprivacy]: https://forums.plex.tv/t/discover-together-public-release/857227/3
[P-paycomplaint]: https://forums.plex.tv/t/remote-streaming-will-be-a-plex-pass-feature/909369
[P-musiccomplaint]: https://forums.plex.tv/t/review-of-the-new-plex-app-music-missing/919519
[P-account]: https://support.plex.tv/articles/200878643-sign-in-to-your-plex-account/
[P-api]: https://developer.plex.tv/pms/
[P-agents]: https://support.plex.tv/articles/200241558-agents/
[P-tasks]: https://support.plex.tv/articles/201553286-scheduled-tasks/
[P-hardware]: https://support.plex.tv/articles/115002178853-using-hardware-accelerated-streaming/
[P-tone]: https://support.plex.tv/articles/hdr-to-sdr-tone-mapping/
[P-transcoder]: https://support.plex.tv/articles/200250347-transcoder/
[P-optimize]: https://support.plex.tv/articles/214079318-media-optimizer-overview/
[P-auto]: https://support.plex.tv/articles/115007570148-automatically-adjust-quality-when-streaming/
[P-tvsettings]: https://support.plex.tv/articles/settings-android-tv/
[P-home]: https://support.plex.tv/articles/203815766-what-is-plex-home/
[P-sharing]: https://support.plex.tv/articles/204232573-restricting-the-shares/
[P-rentals]: https://www.plex.tv/de/blog/welcome-to-rental-land-on-plex/
[P-discover]: https://support.plex.tv/articles/discover/
[P-watchlist]: https://support.plex.tv/articles/universal-watchlist/
[P-sync]: https://support.plex.tv/articles/sync-watch-state-and-ratings/
[P-intro]: https://support.plex.tv/articles/skip-content/
[P-credits]: https://support.plex.tv/articles/credits-detection/
[P-subs]: https://support.plex.tv/articles/auto-sync-subtitles/
[P-together]: https://support.plex.tv/articles/frequently-asked-questions-watch-together/
[P-hooks]: https://support.plex.tv/articles/115002267687-webhooks/
[P-editions]: https://support.plex.tv/articles/multiple-editions/
[P-collections]: https://support.plex.tv/articles/201273953-collections/
[P-seriesagent]: https://support.plex.tv/articles/advanced-setting-plex-tv-series-agent/
[P-free]: https://support.plex.tv/articles/202526943-plex-free-vs-paid/
[P-data]: https://support.plex.tv/articles/202915258-where-is-the-plex-media-server-data-directory-located/
[P-platforms]: https://www.plex.tv/apps-devices/
[PA-features]: https://plexamp.com/
[PA-free]: https://www.plex.tv/en-au/blog/free-bird-plexamp-spreads-its-wings-for-every-music-lover/
[PA-sonic]: https://support.plex.tv/articles/sonic-analysis-music/
[PA-specialist]: https://www.plex.tv/blog/the-future-of-plex-focused-streamlined-and-ready-for-feedback/
[PA-dash]: https://forums.plex.tv/t/plex-dash-release-notes/579489
[PA-rate]: https://forums.plex.tv/t/plexamp-release-notes/221280/76
[PA-fades]: https://forums.plex.tv/t/plexamp-sweet-fades-query/812480
[PA-fadesalbum]: https://forums.plex.tv/t/plexamp-sweet-fades/894454
[I-main]: https://firecore.com/infuse
[I-formats]: https://apps.firecore.com/infuse-pro
[I-server]: https://support.firecore.com/hc/en-us/articles/360006462093-Streaming-from-Plex-Emby-and-Jellyfin
[I-audio]: https://support.firecore.com/hc/en-us/articles/217735707-Audio-Options
[I-cloud]: https://support.firecore.com/hc/en-us/articles/115000070773-iCloud-Sync
[I-trakt]: https://support.firecore.com/hc/en-us/articles/4405098078743-Trakt-Sync
[I-releases]: https://firecore.com/releases
[I-purchase]: https://support.firecore.com/hc/en-us/articles/360046954753-Purchases-and-Family-Sharing
[I-share]: https://support.firecore.com/hc/en-us/articles/215090977-Streaming-From-a-Mac-PC-or-NAS
[I-settings]: https://support.firecore.com/hc/en-us/articles/360015608854-Settings-Overview
[I-pricecomplaint]: https://community.firecore.com/t/inquiry-about-purchasing-the-software-and-its-price/59047
[I-slow25]: https://community.firecore.com/t/is-there-some-kind-of-rate-limiting-when-fetching-details-or-artwork/58089
[I-slow26]: https://community.firecore.com/t/best-practices-for-a-plex-powered-infuse/59316
[E-connect]: https://emby.media/connect.html
[E-about]: https://emby.media/about.html
[E-price]: https://emby.media/premiere.html
[E-matrix]: https://emby.media/support/articles/Premiere-Feature-Matrix.html
[E-history]: https://emby.media/community/topic/133968-the-history-and-evolution-of-emby/
[E-complaint]: https://emby.media/community/topic/118350-firetv-not-activated-on-premiere/
[C-price]: https://getchannels.com/docs/getting-started/quick-start-guide/subscription/
[C-tve]: https://getchannels.com/docs/channels-dvr-server/tv-everywhere/about/
[C-virtual]: https://getchannels.com/docs/channels-dvr-server/how-to/virtual-channels/
[C-remote]: https://community.getchannels.com/t/installing-channels-dvr-server-on-mac/28340
[C-commercial]: https://community.getchannels.com/t/beta-enhanced-commercial-detection/45472
[C-commercialsettings]: https://community.getchannels.com/t/commercial-detection-advanced-settings/34396
[C-complaint]: https://community.getchannels.com/t/missing-spectrum-tve-channels-cnn-tcm-authorization-tuner-errors-in-columbus-oh/46810/9
[K-db]: https://kodi.wiki/view/Databases
[K-share]: https://kodi.wiki/view/Syncing_and_sharing
[K-nfo]: https://kodi.wiki/view/NFO_files
[K-movie]: https://kodi.wiki/view/NFO_files/Movies
[K-addons]: https://kodi.wiki/view/Add-ons
[K-skins]: https://kodi.wiki/view/Skins
[J-network]: https://jellyfin.org/docs/general/post-install/networking/
[J-hardware]: https://jellyfin.org/docs/general/post-install/transcoding/hardware-acceleration/
[J-setup]: https://jellyfin.org/docs/general/post-install/setup-wizard/
[J-libraries]: https://jellyfin.org/docs/general/server/libraries/
[J-books]: https://jellyfin.org/docs/general/server/media/books/
[J-music]: https://jellyfin.org/docs/general/server/media/music/
[J-movies]: https://jellyfin.org/docs/general/server/media/movies/
[J-plugins]: https://jellyfin.org/docs/general/server/plugins/
[J-transcode]: https://jellyfin.org/docs/general/post-install/transcoding/
[J-migrate]: https://jellyfin.org/docs/general/administration/migrate/
[J-users]: https://jellyfin.org/docs/general/server/users/adding-managing-users/
[J-live]: https://jellyfin.org/docs/general/server/live-tv/index.html
[J-clients]: https://jellyfin.org/downloads/clients/
[J-about]: https://jellyfin.org/docs/general/about/
[J-intro]: https://github.com/intro-skipper/intro-skipper
[J12]: https://jellyfin.org/posts/jellyfin-release-12.0/
[S-seerr]: https://docs.seerr.dev/
[S-seerrmerge]: https://docs.seerr.dev/blog/seerr-release/
[S-seerrmigration]: https://docs.seerr.dev/migration-guide/
[S-tautulli]: https://tautulli.com/
[S-tautapi]: https://docs.tautulli.com/extending-tautulli/api-reference
[S-jellystat]: https://github.com/CyferShepard/Jellystat/releases
[S-trakt]: https://forums.trakt.tv/t/plex-scrobbler/84
[S-simkl]: https://simkl.com/en
[S-sonarr]: https://wiki.servarr.com/sonarr/settings
[S-radarr]: https://wiki.servarr.com/radarr/settings
[S-bazarr]: https://wiki.bazarr.media/Additional-Configuration/Settings/Subtitles/
[S-kometa]: https://www.kometa.wiki/en/latest/
[S-kometafiles]: https://kometa.wiki/en/nightly/files/overview/
[S-tdarr]: https://docs.tdarr.io/docs/nodes/nodes/
[S-unmanic]: https://docs.unmanic.app/docs/using_unmanic/getting_started/
[S-ersatz]: https://ersatztv.org/docs/scheduling/sequential/playout/
[S-tunarr]: https://tunarr.com/
[S-tunarrepg]: https://tunarr.com/configure/channels/epg/
[S-abs]: https://audiobookshelf.org/docs/documentation/introduction/
[S-kavita]: https://www.kavitareader.com/
[S-komga]: https://komga.org/
[S-opds]: https://komga.org/docs/guides/opds/
[S-koreader]: https://komga.org/docs/guides/koreader/
[S-kavitaopds]: https://wiki.kavitareader.com/guides/features/opds/
[S-immich]: https://immich.app/features
[S-photo]: https://docs.photoprism.app/
[S-photooriginals]: https://docs.photoprism.app/user-guide/library/originals/
[S-stremio]: https://www.stremio.com/features
[S-vidhub]: https://vidhub.okaapps.com/what-does-vidhub-do/
[S-sen]: https://apps.apple.com/us/app/senplayer-media-player/id6443975850
[S-fileball]: https://apps.apple.com/us/app/filebar-media-player/id1558391784
[S-nplayer]: https://nplayer.com/
[S-vlc]: https://www.videolan.org/vlc/features.html
[S-sym]: https://www.symfonium.app/
[S-symproviders]: https://docs.symfonium.app/wiki/providers/supported-features-by-media-providers/
[S-roon]: https://roon.app/en/pricing
[S-roonarc]: https://help.roonlabs.com/portal/en/kb/articles/arc
[S-roonpath]: https://help.roonlabs.com/portal/en/kb/articles/signal-path
[S-roonmeta]: https://help.roonlabs.com/portal/en/kb/articles/metadata-model
[S-roonoffline]: https://community.roonlabs.com/t/roon-2-0-28-is-live/263614
[M-watchstate]: https://github.com/arabcoders/watchstate
[M-limitations]: https://github.com/arabcoders/watchstate/blob/master/guides/backend-limitations.md
[M-jellyplex]: https://github.com/luigi311/JellyPlex-Watched
[M-plexexport]: https://github.com/van-geaux/plex_nfo_exporter
[M-webtools]: https://github.com/WebTools-NG/WebTools-NG/wiki/ExportTools-Settings
[M-tautexport]: https://github.com/Tautulli/Tautulli/wiki/Exporter-Guide
[UX-apple]: https://support.apple.com/et-ee/guide/tvapp/atvbed18c34e/web
[UX-netflix]: https://help.netflix.com/en/node/114059
[UX-history]: https://help.netflix.com/en/node/22205
[UX-disney]: https://help.disneyplus.com/article/disneyplus-hulu-via-disney-plus/
[P-extras]: https://support.plex.tv/articles/202920803-extras/
