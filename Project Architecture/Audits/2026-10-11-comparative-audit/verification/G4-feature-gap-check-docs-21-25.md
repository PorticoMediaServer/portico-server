# G4 — Feature-gap verification, documents 21–25

## Summary

All **173 F entries** were checked: **45 Absent, 113 Partly present, 0 Present, 13 Planned, 2 Excluded by decision**. Every document has its own table below (21: 36; 22: 22; 23: 65; 24: 25; 25: 25). No entry is wholly Present in its full bundled scope; that does not validate blanket claims that its foundations are missing. The two recorded exclusions are **F-21.32** (television PiP) and **F-23.59** (commercial content/rentals under the current free, non-commercial product model).

The clearest incorrect subclaim is **F-23.19**'s suggestion that recurring refill/removal is not established: native already durably requests the next episode after completion and can delete the watched download. **F-21.9/F-24.7** should be scoped to keep-N show rules and acceptance, not new automatic-download plumbing. Important narrowing elsewhere: **F-21.17** already has permanent manual TV linking in Settings, but lacks its required peer action in the playback destination sheet; **F-24.14** has working source paths for profile switching and PIN entry, leaving launch/OS-user association and acceptance; **F-24.9** already has account stream/bitrate/schedule/content policies and both client management surfaces. These last two first-pass entries themselves acknowledge some foundations: the correction is to the decision scope, not an accusation that their full text denied all implementation.

The highest-value partial cases are source/version association versus automatic grouping and preplay choice (**F-21.2/F-23.60/F-24.2**), native exact subtitle/style parity (**F-21.3/F-24.10**), native saved-resource editing (**F-21.13**), routed event/retry/diagnostic consistency (**F-22.2/F-22.3/F-22.14**), and release completeness (**F-25.2/F-25.7/F-25.8/F-25.18**). Native has a real shared ten-second **long-poll**, not merely a periodic empty poll; the remaining event gap is unifying separate XHR feeds. Timed Library Channel blocks, fallback/overrun policy, overlay configuration and scoped logo delivery also exist (**F-23.43**). Many original M estimates cover coding one surface; consistent delivery and qualification across the fleet is often L or XL. Plans and contracts are obligations, never proof of delivery; radio, relay, desktop companion and DLNA are already recorded work, and the current DLNA contract specifically rejects the old backlog's anonymous whole-server interpretation.

## Reading the tables

Paths are relative to `/home/justin/projects/Portico`. Prefixes: **S** = `apps/portico-server`; **N** = `apps/portico-react-native/apps/native`; **RN** = `apps/portico-react-native`; **V** = `apps/portico-react-native/apps/vega`; **PA** = `portico-internal/Project Architecture`; **PC** = `portico-internal/Product Contracts`; **Site** = `getportico-tv`. These are path abbreviations, not separate evidence sources.

**All** means server, web, iOS, tvOS, Android, Android TV, Fire TV and Vega. **Clients** means those seven client platforms; **TV** means tvOS, Android TV, Fire TV and Vega; **Handheld** means iOS and Android. Platform columns describe delivery scope, not verified support. Sizes estimate the remaining work including integration: S hours, M days, L at least a week, XL a substantial project. Duplicates identify the same capability or an explicitly overlapping bundle; they do not imply identical scope. Planned means a recorded obligation/plan with no delivery of the particular capability established. Partly present takes precedence when usable constituent implementation exists.

Context-sync was not run because this lane permits writes only to this report; canonical project/verification guidance was read directly. Execution host: `ehler-homelab-mini-1` (personal Ubuntu).

## Document 21 — Native UX and feature parity

| ID | Claimed gap | Verdict | What exists / where (path) | Canonical name | Duplicates | Size | Platforms | Fit |
|---|---|---|---|---|---|---|---|---|
| F-21.1 | Spoiler masking with explicit reveal | **Planned** | No profile spoiler preference/renderer found; PC/media-presentation/contract.md:161 requires episode-summary policy. Search covered spoiler/hideSummary/maskSummary. | Spoiler masking | F-18.49 | M (agrees) | All | Polish and choice; implement promised policy before extra domains. |
| F-21.2 | Pre-play version and quality selector | **Partly present** | N/src/screens/title/PlaybackChoices.tsx:13 defers Version; :39 selects current/first source. Audio/subtitle preplay and in-player quality exist (N/src/player/PlayerOptions.tsx:154); preplay version/quality missing. | Pre-play source and quality selection | F-18.19, F-23.60, F-24.2 | M UI; L ingestion | All | Choice fits; retain exact server source identity. |
| F-21.3 | Exact subtitle preselection and SDH/audio-description preferences | **Partly present** | N/src/screens/title/PlaybackChoices.tsx:41 deduplicates subtitle languages; :45 sends language. S/packages/client-core/src/playback-v1/types.ts:35 retains exact ID/SDH. Exact preplay subtitle ID and SDH/description preferences missing. | Accessible track preferences | F-02.8, F-04.6, F-18.51 | M (agrees) | All | High accessibility value; avoid ambiguous same-language choices. |
| F-21.4 | Native grid/list/card-density options | **Partly present** | S/packages/client-core/src/presentation/settings-structure.ts:622 stores card percentage; PA/Backlog.md:62 records native non-consumption. Native grids/song lists exist; adjustable native card grid and generic list switch absent. | Native browse density and list mode | F-18.9 | M (agrees) | Clients | Choice with stable geometry; bounded lists mandatory. |
| F-21.5 | Two-pane tablet Settings and regular-width media workspaces | **Planned** | PA/Backlog.md:69 explicitly lists iPad two-pane Settings; N/src/screens/Settings.tsx:59 uses stacked pages rather than master/detail workspace. | Tablet two-pane workspaces | — | M settings; L all workspaces | iOS, Android, web | Strong polish fit; preserve narrow-window fallback. |
| F-21.6 | Handheld gesture controls with visible alternatives | **Partly present** | N/src/player/Timeline.tsx:23 implements drag seek and :37 accessible adjustment; N/src/player/VideoPlayer.tsx:446 exposes controls. No double-tap seek, brightness/volume swipe, pinch/hold-speed map found. | Handheld player gestures | F-17.12, F-20.10 | M (agrees) | Handheld; web touch | Ergonomic choice; buttons and accessible alternatives required. |
| F-21.7 | Keyboard shortcuts and pointer-complete tablet navigation | **Planned** | PC/navigation-and-input/contract.md:190 requires shortcuts with text-entry fencing; native focus/button primitives exist but no app keyboard command map/editor found. | Native keyboard and pointer navigation | F-17.6, F-18.53, F-19.10 | M core; L device acceptance | iOS, Android; web overlap | Consistency and accessibility; OS keys stay reserved. |
| F-21.8 | Multi-server merged Home/search/library source selection | **Partly present** | S/packages/client-core/src/server-connections.ts:1 and N/src/app/session.tsx retain/select known servers; no merged Home/search catalogue owner found. Selection is one server at a time. | Merged multi-server browsing | F-12.25, F-22.17, F-24.19 | XL (raise L) | All | Freedom benefit; explicit cross-server identities and partial errors needed. |
| F-21.9 | Rolling keep-N offline episode rules | **Partly present** | N/src/bridge/offline-downloads.ts:359 persists next-after intent; :368 requests next episode; :565 auto-next/delete-watched. N/src/screens/Downloads.tsx:175 exposes Wi-Fi/storage rules. Missing per-show keep-N and owned-rule reconciliation. | Rolling offline episode rules | F-14.11, F-23.19, F-24.7 | L (M/L fair) | All; transfers Handheld | Strong travel fit; revision-driven refill, transparent deletion ownership. |
| F-21.10 | Grouped and batch download management | **Partly present** | N/src/screens/Downloads.tsx:43 offers per-file pause/resume/remove; :128 displays batch request progress. Local list is flat; no group selection or batch pause/retry/delete found. | Grouped batch download management | F-14.18 | M/L (raise M for all clients) | All; local Handheld | Polish fit; retain separate local/server ownership. |
| F-21.11 | Android background-download health screen | **Absent** | N/android/app/src/main/java/tv/getportico/portico/native/PorticoDownloads.kt:24 gates TV downloads; no standby/Data Saver/battery health screen found in native/settings/string searches. | Android background health diagnostics | F-22.19 | M (S/M optimistic) | Android, Android TV, Fire TV; Vega separate APIs | Explain actual restrictions; avoid blanket exemption requests. |
| F-21.12 | Download Live Activity / local completion entry | **Partly present** | N/android/app/src/main/java/tv/getportico/portico/native/PorticoDownloadCenter.kt:354 creates foreground-transfer notification; N/Native/PorticoDownloads.m:209 deliberately shows completion in-app/toast only. No iOS ActivityKit/Live Activity or persistent OS completion entry found. | Download OS progress surfaces | — | M per OS; L parity | iOS, Android; TV capability limits | Optional polish; iOS completion notifications are a recorded exclusion in code, Android foreground progress is already real; Live Activity needs separate product scope. |
| F-21.13 | Complete playlist/collection native CRUD and sharing | **Partly present** | N/src/player/EntryMenu.tsx:157 creates playlists, :212 creates collections; N/src/screens/title/SavedPages.tsx:95 reorders and :111 removes occurrences. Rename/delete/share and full collection entry management not exposed there. | Native saved-resource editing and sharing | F-12.16, F-18.29, F-18.31 | M/L (M wiring plausible) | All | High consistency value; occurrence IDs and server rights already available. |
| F-21.14 | TV creation of Saved Views | **Partly present** | N/src/screens/BrowseFilters.tsx:92 returns inline TV toolbar without Save; :113 handheld Save and :125 naming sheet exist. Server query resources already exist. | TV saved-view creation | — | S/M (agrees) | TV; server/web/Handheld exist | Expose existing strength consistently; remote naming needs native keyboard. |
| F-21.15 | Broader context actions and consistent immediate detail actions | **Partly present** | N/src/player/EntryMenu.tsx:87 dispatches play/state/download/metadata/playlist; :118 Details exists. It lacks rating/collection/file-info dispatch; N/src/screens/Detail.tsx:126 exposes refresh elsewhere. | Native context-action parity | F-18.12, F-18.14, F-18.27 | M (S/M optimistic) | Clients; server availability | Consistency fit; do not equate union members with reachable actions. |
| F-21.16 | Native share links and share-sheet actions | **Partly present** | N/src/screens/InviteSheet.tsx:37 and Together.tsx:200 use Share.share; N/src/app/routes.ts:25 includes links. No normal media-item share-sheet caller found. | Native item link sharing | F-18.54 | S/M (agrees) | All; native share Handheld | Household ease; route link confers no media permission. |
| F-21.17 | Play-on-device from detail and permanent TV-code entry | **Partly present** | N/src/screens/LinkTV.tsx:74 has manual code; N/src/screens/settings/SettingsPage.tsx:139 exposes it permanently in Settings. N/src/player/CastBar.tsx:65 controls active playback; detail page has no equivalent Play-on action. N/src/player/Destinations.tsx:89 lacks the permanent code peer required by PC/onboarding/contract.md:184; reuse the existing code service. | Detail device playback and manual TV linking | F-13.18 | M (raise S for full scope) | All | Strong ease/consistency fit; keep manual entry independent of discovery. |
| F-21.18 | Screensaver with profile-safe art and idle policy | **Absent** | No screensaver/DreamService/idle-art implementation found; native TV renderer is ordinary product shell. Needs idle, visibility and restricted-art policy. | Profile-safe TV screensaver | F-18.52, F-19.9 | M/L (M optimistic) | TV; server art policy | TV polish; no new Hosted periodic traffic. |
| F-21.19 | Clock, finishes-at and elapsed/remaining display options | **Partly present** | N/src/player/Timeline.tsx:72 displays elapsed and :73 remaining; :12 supports live wall-clock labels. No finishes-at/system-clock policy or display toggle found. | Playback clock and finish-time options | — | S UI; M parity | Clients | Small useful choice; use locale/hour cycle and playback speed. |
| F-21.20 | TV theme music and backdrop/focus personalization | **Partly present** | S/packages/client-core/src/presentation/settings-structure.ts:244 has web backdrop option and :247 native Reduced Motion. No theme-song player/focus-glow preference found. | TV ambiance personalization | F-18.16, F-24.23 | M/L (M optimistic) | Clients; server theme assets | Optional choice; reconcile sensory/Reduced Motion and fixed appearance rules. |
| F-21.21 | App-owned game controller mapping and hints | **Planned** | N/Native/PorticoSiriRemote.m:48 handles microGamepad D-pad touch only; PC/navigation-and-input/contract.md:21 requires controller equivalence. No full controller action map/hints found. | Game-controller navigation | F-16.7, F-19.10 | M/L (raise S/M) | Clients; TV emphasis | Accessibility and platform consistency; hardware mapping varies. |
| F-21.22 | Phone companion remote with text/navigation | **Partly present** | S/packages/client-core/src/remote-playback.ts:1 and N/src/player/CastBar.tsx:65 provide authorized playback control. No D-pad/text/search commands or general companion screen found. | Phone companion navigation remote | F-08.7, F-13.5, F-16.11 | L (raise M) | All; Handheld controller, TV receiver | Good household choice; scope and command ownership need design. |
| F-21.23 | Optional Seerr requests and catalog availability UI | **Absent** | No Seerr/Sonarr/Radarr adapter/routes/UI found in server/API/core/web/native/i18n searches; acquisition remains external. | Seerr request integration | F-08.10, F-12.5, F-23.6, F-24.18 | L (M/L fair) | All | Opt-in freedom; keep credentials/catalogue private and Hosted cheap. |
| F-21.24 | Trakt/anime trackers/MDBList discovery | **Absent** | No Trakt/Simkl/ListenBrainz/Last.fm/MDBList/anime-tracker connector found. Local personal-state routes are not remote synchronization. | External tracking and discovery adapters | F-08.4, F-10.6, F-12.1, F-23.7, F-24.17 | L; XL multi-service (M/L low) | All | Optional freedom; durable outbox, matching and conflict policy needed. |
| F-21.25 | Search provider choice and richer suggestions | **Partly present** | N/src/screens/Search.tsx:1 uses server search; server metadata providers are enrichment, not alternate search services. Grouped local results/recent queries exist; external provider selection absent. | Search-provider adapters | F-07.5, F-07.8, F-23.57 | L (raise M) | All | Only adopt for measured value; server-bound ranking and permissions remain. |
| F-21.26 | Native client diagnostics/log sharing for ordinary viewers | **Partly present** | S/server/internal/httpapi/admin_diagnostics.go:148 accepts bounded viewer client-log uploads; N/src/screens/server/Troubleshooting.tsx:141 exports owner server JSON. No native viewer local ring-log collector/copy/share/clear UI found. | Viewer client diagnostics export | F-16.13, F-19.1, F-19.2, F-22.10, F-22.21 | M (S/M fair) | All; device adapters Clients | High support polish; redact credentials, paths and viewer data. |
| F-21.27 | Settings backup/import/reset and safe cache clear | **Partly present** | S/packages/client-core/src/preferences.ts:1 persists scoped preferences; SettingsPage.tsx:138 resets recommendations. No preference export/import/default reset/cache-clear actions in authored settings. | Preference portability and safe cache reset | — | M (S/M low for full parity) | Clients; server profiles | Recovery choice; exclude secure credentials and private journals. |
| F-21.28 | Native licences/credits/support/donation pages | **Partly present** | N/src/screens/settings/SettingsPage.tsx:288 renders About/legal footer; S/THIRD-PARTY-NOTICES.md:1 holds notices outside app. No browsable native dependency-license/support/donation page found. | Native legal and support pages | F-25.24 | S/M (raise S) | Clients; Site | Licences/support aid polish; funding is an owner decision. |
| F-21.29 | Launch profile prompt and simplified kids workspace | **Partly present** | N/src/screens/auth/DirectProfiles.tsx:81 has Who’s watching/PIN; N/src/screens/Profiles.tsx:3 manages profiles. No always-prompt-on-launch preference or simplified kids shell found. | Launch profile prompt and kids workspace | F-01.18, F-01.19, F-10.38, F-24.14 | M (agrees) | All | Household choice; prompt supplements existing server restrictions. |
| F-21.30 | Android assistant/launcher quick actions | **Partly present** | N/src/bridge/NativeActivity.ts:34 enables Apple only; :44 indexes bounded lists. Android Top Shelf/launcher code exists separately, but no equivalent assistant activity indexing wired. | Assistant and launcher actions | F-10.40, F-19.8 | M/L (M per platform) | iOS, Android; TV launch adapters | Consistency benefit; narrow opaque current-scope IDs only. |
| F-21.31 | SharePlay and native OS multiuser association | **Absent** | No GroupActivities/SharePlay/OS-user association adapter found; server Together exists, a different carrier. | SharePlay and OS user association | F-13.17, F-19.7, F-24.14 | L/XL (L fair) | iOS, tvOS; other clients need analogous behavior | Platform polish, inherently Apple-specific; retain common Together path. |
| F-21.32 | TV PiP | **Excluded by decision** | PA/06 Clients.md:62 explicitly confines PiP to phones/tablets; PC/media-presentation/contract.md:266 requires television video to end on closing full-screen except receiver handoff. | TV picture in picture | — | M per supported OS; L parity | TV | Recorded product boundary; changing it needs a decision, OS reach differs. |
| F-21.33 | Reviewed light/daylight appearance and optional accent | **Absent** | N/src/ui/theme.ts:213 uses one design theme; no light palette/accent selector found. PC/visual/contract.md:90 mandates accepted atmospheric background; it does not expressly prohibit a reviewed light variant. | Light theme and accent choice | F-16.4, F-18.45, F-23.56 | L (raise M for all clients) | Clients | User comfort fits choice; reconcile accepted palette/contrast before expansion. |
| F-21.34 | External-player escape hatch | **Absent** | No external-player selection/launch/return-progress adapter found. Local download playback via Portico/system view is not arbitrary external-engine handoff. | External player handoff | F-20.18, F-24.11 | L (raise M for lifecycle/progress) | All; player-specific OS adapters | Optional freedom; scoped grants and one progress authority essential. |
| F-21.35 | Standalone file browsing/import mode | **Absent** | No standalone file/import/NAS catalogue path found; N/src/bridge/offline-downloads.ts handles server-issued artifacts only. | Standalone NAS or local player mode | F-23.10, F-23.11 | XL (raise L) | Clients; independent catalogue authority | Major thin-client/server-authority change, not a UI option. |
| F-21.36 | Photo libraries/slideshow | **Absent** | No photo library/slideshow handler or renderer found; N/src/screens/Gallery.tsx:85 is developer UI gallery, not media. Reserved kinds alone do not deliver photo indexing. | Photo and home-video libraries | F-23.39, F-24.15 | XL (raise M/L) | All | Useful freedom; new catalogue/presentation/privacy contracts required. |

## Document 22 — Connectivity and resilience

| ID | Claimed gap | Verdict | What exists / where (path) | Canonical name | Duplicates | Size | Platforms | Fit |
|---|---|---|---|---|---|---|---|---|
| F-22.1 | A user-editable set of addresses for one pinned server | **Partly present** | S/packages/client-core/src/server-connections.ts:7 stores paired origins; route-connection.ts:73 remembers up to eight. known-servers.ts:11 holds one displayed address per entry. No add/remove/order UI for one pin found. | Editable pinned-server addresses | F-21.8 | M (agrees) | All | Freedom/ease fit; keep verified identity independent of address. |
| F-22.2 | A common operation-classed request policy | **Partly present** | N/Native/PorticoPlayer.m:122 sets 15-second budgets, :163 bounds responses; S/packages/client-core/src/route-connection.ts:154 retries reads only. Core index.ts:338 general requests lack one operation-class policy; browser/native/Vega defaults differ. | Operation-class request policy | — | L (M/L fair) | All | Strong consistency; avoid mutation replay without receipts. |
| F-22.3 | Shared Retry-After parsing and preservation everywhere | **Partly present** | S/packages/client-core/src/index.ts:134 parses seconds/date and :405 uses it; playback-v1/http.ts:42 accepts numeric only; console-transport.ts:18 error shape loses retry floor; server-connections.ts:262 wraps refresh errors. | Consistent Retry-After handling | — | M (S/M fair) | All | Polish and resilient low-load behavior; helper exists, consumers need repair. |
| F-22.4 | Small control requests reserved ahead of browsing | **Partly present** | N/Native/PorticoPlayer.m:136 admits FIFO and :149 takes first waiting request; S/server/internal/httpapi/route_lanes.go:394 reserves server auth lane. Missing client priority/control reservation. | Client control-request reservation | — | M/L (M core plausible) | Clients; server already lanes | Consistency and playback-first discipline; bounded fairness required. |
| F-22.5 | A bounded “return to LAN” preference check | **Partly present** | S/packages/client-core/src/route-connection.ts:95 reprobes on network change; :86 skips a ready foreground connection; :113 puts last-good before LAN. No healthy-route foreground optimization or seamless replacement owner found. | Return-to-LAN route optimization | — | M (agrees) | All | Ease fit; local event-based check respects cheap Hosted. |
| F-22.6 | Direct-DNS rebinding diagnosis and safe recovery | **Absent** | S/packages/client-core/src/remote-access.ts:30 offers NAT/proxy guidance; no DNS-rebinding error class/narrow-domain help found in bridge/presenter/i18n searches. | DNS-rebinding recovery diagnostics | F-23.51 | M (agrees) | All | High support value; preserve DNS safeguards and verified fallback. |
| F-22.7 | Explicit VPN/Tailscale route policy | **Partly present** | S/server/internal/networking/remote_manager.go:643 publishes VPN/private-proxy manual routes; S/packages/client-core/src/route-identity.ts:42 excludes 100.64/10 and overlay names from HTTP-private classification. HTTPS overlay works; explicit HTTP overlay policy absent. | VPN and overlay route support | F-09.5, F-23.51 | M (S/M low for all layers) | All | Core self-hosting freedom; must reconcile HTTP trust consistently. |
| F-22.8 | Forgiving, protocol-safe common-port candidates | **Partly present** | N/src/app/direct-connect.ts:21 guesses HTTP/HTTPS only; no 32500 insertion/bounded port race. Existing explicit host:port works and identity probe follows. | Forgiving common-port connection input | — | S/M (S narrow fix) | Clients | Ease fit; never invent insecure WAN downgrade. |
| F-22.9 | Release-era compatibility negotiation | **Partly present** | S/server/internal/httpapi/router.go:816 returns public version/build; :751 registers identity proof. No required/optional-feature negotiation or supported release window; PC/scope-completion.md:15 explicitly requires compatibility outcomes. | Release compatibility negotiation | F-16.15, F-19.14, F-25.1, F-25.11 | L (M/L fair) | All | High consistency value before stable release. |
| F-22.10 | Explicit network causes in a connection diagnostics view | **Partly present** | S/packages/client-core/src/route-connection.ts:3 reports phase/discovery/identity mismatch; N/src/bridge/server-connections.ts:109 exposes local-permission denial. DNS/TLS/connect/slow/captive distinctions and dedicated view absent. | Connection-cause diagnostics | F-20.26, F-21.26, F-22.21, F-23.51 | L (raise M across bridges) | All | Polish without clutter; details should be on demand and redacted. |
| F-22.11 | Optional per-network address preference | **Absent** | RouteSnapshot.network in S/packages/client-core/src/route-connection.ts:3 is a change marker, not named Wi-Fi/cellular route policy; no SSID/route-preference setting found. | Per-network address preferences | — | M/L (M plausible UI) | Clients | Optional freedom; network identity cannot substitute for trust. |
| F-22.12 | Offline cached navigation snapshots | **Partly present** | N/src/screens/auth/OfflineDownloads.tsx:1 and bridge/offline-downloads.ts retain download identity/art/progress; no durable last-known Home/catalogue navigation snapshots found. | Offline navigation snapshots | F-14.6, F-14.8, F-19.3 | L/XL (L baseline fair) | Clients | Separate explicitly stale offline mode needed to reconcile fresh-derived-results principle. |
| F-22.13 | Crash-safe final facts from ordinary online playback | **Partly present** | S/packages/client-core/src/playback-v1/legacy-bridge.ts:31 stores report queue in memory; :175 enqueues exact reports. Core index.ts:1640 persists cleanup/checkpoint journal; N/src/bridge/offline-downloads.ts:553 persists offline progress. Missing durable exact final/marker facts for ordinary UI v1 reports; do not call all playback journaling absent. | Durable final online playback facts | F-02.23, F-14.12 | L (M/L fair) | All | Correctness fit; bound viewer-owned replay and distinguish leases from facts. |
| F-22.14 | Unified native authenticated event transport | **Partly present** | N/src/bridge/useNativePlayback.ts:100 owns EventsClient with waitSeconds=10; this is server long-poll, not periodic empty polling. N/src/bridge/event-stream.ts:26 separately opens raw URL XHR for other feeds; no unified routed refresh/subscription owner. | Unified native event transport | F-08.6, F-16.1 | L (M/L fair) | Clients; server event API exists | Consistency and quiet-server fit; preserve cursor/resync/cancellation. |
| F-22.15 | Per-selected-server overload coordination | **Partly present** | S/packages/client-core/src/hosted-gate.ts:31 shares Hosted failure floors; playback-v1/legacy-bridge.ts:210 honors per-report delay. No analogous selected-server coordinated gate across reads/features found. | Selected-server overload coordination | — | M/L (M core plausible) | All | Supports weak boxes; reserve auth/playback and distinguish lane refusal. |
| F-22.16 | Captive portal and split-horizon troubleshooting | **Partly present** | S/packages/client-core/src/route-identity.ts:96 probes API and signed nonce; route-connection.ts:167 detects non-JSON gateway failure. No portal/HTML/split-horizon diagnostic or repair view found. | Captive-portal and split-DNS recovery | F-22.6, F-23.51 | M (S/M fair) | All | Ease fit; keep saved credentials, never send them to unverified redirect. |
| F-22.17 | Per-server credential retention for quick switches | **Partly present** | S/packages/client-core/src/known-servers.ts:3 explicitly stores no credentials; server-connections.ts:17 storage reads one selected record. Remembered address switcher exists; independent protected server families absent. | Retained multi-server credentials | F-01.23, F-21.8 | L (M/L fair) | Clients | Freedom and fast household switching; no merged catalogue required. |
| F-22.18 | Owner-defined published URLs by subnet | **Partly present** | S/server/internal/networking/remote_manager.go:501 publishes manual routes; no requester-subnet URL mapping found in networking handlers/settings/OpenAPI. Existing owner access URLs apply more broadly. | Subnet-specific published routes | — | L (M/L fair) | All | Advanced freedom; leave automatic simple defaults. |
| F-22.19 | Android background-network restrictions explained | **Absent** | No Android standby/Data Saver/notification-health presenter or setting links found; generic transfer/network errors remain. N/android/app/src/main/java/tv/getportico/portico/native/PorticoDownloads.kt:24 honestly excludes TV downloads. | Android background health diagnostics | F-21.11 | M (agrees) | Android, Fire TV; Vega distinct SDK | Support polish; show only relevant observed restrictions. |
| F-22.20 | A server-identity pairing QR distinct from TV activation | **Partly present** | S/packages/client-core/src/setup-qr.ts:5 draws verification URL QR; N/src/bridge/setup-codes.ts binds activation. No distinct pin/fingerprint/routes QR encoder/parser/scanner found. | Server-identity pairing QR | — | M (agrees) | All; scanner Handheld | Simpler private HTTP/VPN pairing; no secrets in QR. |
| F-22.21 | Backoff/deadline diagnostics without raw private destinations | **Partly present** | S/packages/client-core/src/route-connection.ts:3 exposes phase/revision/network/discovery; N/src/screens/server/Troubleshooting.tsx:141 exports server support report. No queued-vs-network timing/retry-floor/last-proof export found. | Redacted route and retry diagnostics | F-19.2, F-20.26, F-21.26, F-22.10 | M (S/M fair) | All | Useful low-drama troubleshooting; no raw private destination telemetry. |
| F-22.22 | Optional relay or assisted remote access | **Planned** | PA/Backlog.md:99 plans paid relay; PC/scope-completion.md:39 retains future separate paid scope. No current media relay; assisted direct mapping does exist. | Paid remote-access relay | F-09.19, F-23.50, F-24.5 | XL (raise L) | All | Future owner choice; separate economics/privacy from cheap Hosted. |

## Document 23 — Competitor feature survey

| ID | Claimed gap | Verdict | What exists / where (path) | Canonical name | Duplicates | Size | Platforms | Fit |
|---|---|---|---|---|---|---|---|---|
| F-23.1 | A reviewed migration wizard for Plex/Jellyfin/Emby | **Absent** | No Plex/Jellyfin/Emby importer, identity review map or state/artwork migration route found; S/server/internal/backup/backup.go:1 is Portico-format backup, not migration. | Household state migration | F-08.5, F-12.4, F-24.1 | XL (raise L for full wizard) | All | Highest adoption/freedom value; staged consent and undo required. |
| F-23.2 | Interoperable export of facts, artwork, collections and personal state | **Partly present** | S/server/internal/backup/backup.go:1 preserves own database/files; httpapi/console.go:320 exports diagnostics; PA/Backlog.md:38 records old CSV history route. No product-neutral facts/art/list/profile bundle. | Portable library and personal-state export | F-06.15, F-08.5, F-10.8, F-12.4, F-24.24 | L/XL (M/L optimistic) | All | Open-data fit; keep private history separate from shared metadata. |
| F-23.3 | External-state reconciliation for coexistence, not only one-time migration | **Absent** | No WatchState/JellyPlex-Watched/state coexistence adapter found; local personalstate and offline reconciliation are not external-server sync. | External state synchronization | F-08.4, F-12.1, F-21.24, F-23.7, F-24.17 | L (raise M) | All | Trial adoption fit; one-way import first, deterministic conflicts. |
| F-23.4 | Authenticated outbound webhooks | **Absent** | No webhook destination/outbox/signature route, preference or consumer found across API/server/core/clients. Existing event feeds are inbound subscriptions. | Authenticated outbound webhooks | F-08.2, F-09.26, F-18.40, F-23.49 | L (raise M for durable egress) | Server; client opt-in web/Handheld/TV | Freedom without email infrastructure; bounded signed delivery and privacy scopes. |
| F-23.5 | Import/rename/replace notifications with targeted scans | **Partly present** | S/server/internal/httpapi/router.go:995 queues library scan with empty source/path selector at :1001; source scans/inventory exist. Missing file/subtree completion webhook and path translation/idempotent receipts. | Targeted change-notification scans | F-05.2, F-08.9, F-23.48 | M/L (M narrow API plausible) | Server; owner clients All | Direct scale benefit; invalidate exact source revisions after completed writes. |
| F-23.6 | Seerr adapter and request lifecycle | **Absent** | No Seerr acquisition/request/approval/available state or adapter found; local My List does not acquire absent media. | Seerr request integration | F-08.10, F-12.5, F-21.23, F-24.18 | L adapter; XL native portal (raise M) | All | Optional choice and household ease; keep acquisition external. |
| F-23.7 | Opt-in Trakt/Simkl and music scrobbling | **Absent** | No Trakt/Simkl/Last.fm/ListenBrainz/scrobble handler, service or opt-in settings found in word-boundary searches. | External tracking and discovery adapters | F-08.4, F-10.6, F-12.1, F-21.24, F-23.3, F-24.17 | L per protocol; XL bundle (raise M) | All | Freedom fit; per-person consent, durable idempotent outbox. |
| F-23.8 | OpenSubsonic compatibility for music clients | **Absent** | No Subsonic/OpenSubsonic protocol adapter or routes found; S/server/api/README.md:1 documents Portico’s own API. | OpenSubsonic music API | F-08.8, F-10.42 | L/XL (M/L low for full parity) | Server; all clients benefit via ecosystem | Strong freedom; explicit auth/extensions/paging compatibility needed. |
| F-23.9 | A documented integration target for Infuse and other multi-server players | **Partly present** | PC/scope-completion.md:15 requires independently usable API semantics. Own JSON/YAML contracts exist; no Infuse reference adapter/vendor integration found. | Supported external API and SDK | F-08.3, F-08.16, F-23.62, F-24.24, F-25.25 | L (M for recipes only) | All; external players | Choice fit; vendor adoption cannot be guaranteed by API publication. |
| F-23.10 | Open-file/open-URL playback without a server | **Absent** | N/src/bridge/offline-downloads.ts:513 plays authorized saved artifacts; no arbitrary file-provider/URL input into a separate local session found. | Standalone NAS or local player mode | F-21.35, F-23.11 | L native; XL consistent catalogue (M low) | Clients | Useful freedom but distinct local authority/lifecycle required. |
| F-23.11 | Full player-only NAS/cloud library mode | **Absent** | S/server/internal/remotesources owns remote storage; native clients remain server-backed. No app-owned SMB/NFS/cloud scanner/catalogue found. | Standalone NAS or local player mode | F-21.35, F-23.10 | XL (raise L) | Clients | Major thin-client/scale architecture change, not routine parity work. |
| F-23.12 | A broader client video/subtitle engine where format tests justify it | **Partly present** | N/Native/PorticoPlayer.m:24 attaches AVPlayer; N/android/app source uses Media3; server subtitle burn-in/rendering exists (S/server/internal/decoder/burn_in.go:35). No mpv-class cross-platform ASS/PGS direct renderer established. | Broad direct-play and subtitle engines | F-04.1, F-04.2, F-17.1, F-17.2, F-20.1, F-20.3 | XL (raise L for fleet) | All | Choice and lower server work; engine decision must follow corpus/device evidence. |
| F-23.13 | Explicit opt-in adaptive quality or actionable quality suggestions | **Partly present** | S/packages/client-core/src/presentation/settings-structure.ts:593 maps requested limits; native PlayerOptions.tsx:171 changes quality. No bandwidth estimator/adaptive ladder/suggestion owner found. PA/Specs/Spec — Playback Protocol v1.md:152 forbids server autonomous lowering, permits explicit client Auto. | Adaptive quality and suggestions | F-02.4, F-03.17, F-17.9, F-20.21 | M suggestions; XL adaptation (L fair) | All | Suggestions fit ease; opt-in adaptation needs precise consent, never load-driven quality loss. |
| F-23.14 | Automatic subtitle synchronization with source/edition safeguards | **Partly present** | N/src/player/PlayerOptions.tsx:267 adjusts source-relative timing; S/server/internal/httpapi/subtitle_routes.go owns choices. No audio/reference alignment/drift analysis found. | Automatic subtitle alignment | F-04.21 | L (M/L fair) | Server; Clients review/accept | Choice fit; preserve originals and edition/source safeguards. |
| F-23.15 | Repeated-intro matching and trusted marker import | **Partly present** | S/server/internal/mediaanalysis/detectors.go:24 derives chapter markers and :75 low-confidence luminance intro; httpapi/media_analysis.go:105 edits markers. No repeated-season audio matcher/trusted import workflow found. | Repeated-intro detection and marker import | F-04.13, F-04.15, F-24.25 | L detector; M/L adapter (agrees) | All | Polish fit; reviewed provenance and bounded background comparison. |
| F-23.16 | Credits with mid/post-credit scene boundaries | **Partly present** | S/server/internal/mediaanalysis/detectors.go:98 extends a credits hypothesis to duration; marker/skip UI exists. No explicit mid/post-credit scene boundaries/classifier found. | Credits scene-aware detection | F-04.14, F-24.25 | L (agrees) | All | Polish only if story content is protected against false skips. |
| F-23.17 | Accurate commercial detection and a recording-focused review UI | **Partly present** | S/server/internal/mediaanalysis/detectors.go:39 recognizes chapter commercial labels and :85 paired black boundaries; httpapi/media_analysis.go:105 edits markers. No Comskip/specialist recording review surface found. | Commercial detection and review | F-11.15 | L adapter; XL native accuracy (M low) | All; server detector | Reversible skipping fits freedom; destructive cutting is separate decision. |
| F-23.18 | Durable optimization rules, rather than one conversion at a time | **Partly present** | S/packages/client-core/src/downloads-service.ts:1 supports preparation service; no persistent on-add/query/scheduled conversion rule engine found. Next-episode download intents are narrower. | Standing media optimization rules | F-03.11 | L (raise M) | All | Ease for weak servers; bounded changed-media selection and storage accounting. |
| F-23.19 | Rolling offline policies with storage-aware refill | **Partly present** | N/src/bridge/offline-downloads.ts:359 persists/refills next episode, :565 removes watched; Downloads.tsx:175 exposes Wi-Fi/storage controls. Missing recurring per-show keep-N/charging/owned-file eviction policy. | Rolling offline episode rules | F-14.11, F-21.9, F-24.7 | L (M/L fair) | All; local transfers Handheld | Useful travel polish; first-pass claim of no recurring refill is too broad. |
| F-23.20 | Disc structures and extras interoperability | **Partly present** | S/packages/client-core/src/detail.ts:8 defines extras and :37 edition facts; no ISO/BDMV/VIDEO_TS catalogue/menu adapter found. File extras exist; disc navigation does not. | Disc structures and extras | F-02.13, F-03.19, F-05.11, F-05.13 | XL full menus; L indexing (L fair) | All | Owner choice; document unencrypted format and platform limits. |
| F-23.21 | Audio-derived musical similarity | **Absent** | PA/Backlog.md:17 plans radio/mixes but leaves the similarity source undecided; it does not select an audio-derived engine. Generic analysis/AcoustID fingerprints are not sonic embeddings; no sonic model/index found. | Audio-derived music similarity | F-10.5, F-24.8 | XL (raise L) | Server; all Clients | Good optional music depth; weak-hardware/model licence cost must fit. |
| F-23.22 | Artist/album/track radio and personal mixes | **Planned** | PA/Backlog.md:17 plans radio/mixes; PC/scope-completion.md:7 requires server-defined instant-mix continuation. No radio/mix implementation found; shuffle is already separate. | Music radio and instant mixes | F-10.4, F-24.8 | L metadata; XL sonic (M low) | All | Strong ease/choice fit; bounded reproducible authorized membership. |
| F-23.23 | Guest DJ and sonic journeys | **Absent** | Shared queues/occurrence IDs exist; no seed-to-seed sonic journey or DJ strategy found in core/server/native/music searches. | Guest DJ and sonic journeys | F-23.21, F-23.22 | L after prerequisites; XL total (M low) | All | Optional exploration; never silently rewrite explicit queues. |
| F-23.24 | Equalizer, AutoEQ, crossfeed and output-specific DSP | **Absent** | Gain/crossfade/gapless are present; no EQ/AutoEQ/crossfeed/parametric output profile found in JS/Apple/Android/Vega searches. | Audio equalizer and output DSP | F-10.26 | L EQ; XL fleet/DSP (M low) | Clients; server capability/reporting | Choice fit; processing must be visible and avoid clipping. |
| F-23.25 | Sample-rate matching and honest output diagnostics | **Partly present** | S/server/internal/playbackv1/transitions.go:496 publishes source/render sample rate; N/src/player/PlayerOptions.tsx:312 shows playback-info rows. No complete actual-device mixer/DAC-rate matching or bit-perfect path established. | Sample-rate matching and output diagnostics | F-10.26, F-20.22 | L diagnostics; XL exclusive output (agrees) | Clients; server facts | Truthful polish; OS/output-specific and may conflict with mixing/crossfade. |
| F-23.26 | Headless listening receiver and remote-control UX | **Partly present** | S/packages/client-core/src/remote-playback.ts:121 implements authorized receiver lifecycle. No supported headless audio appliance/service/package found. | Headless music receiver | F-13.5, F-21.22 | L (M/L fair) | Server; Handheld controller; new appliance | Freedom fit; a screenless receiver needs output/reconnect qualification. |
| F-23.27 | CarPlay and Android Auto | **Absent** | No CarPlay scene/entitlement/browser or AndroidAuto car browser found. Ordinary Now Playing/media session is already present. | CarPlay and Android Auto | F-10.39, F-19.16 | L per OS (raise M) | iOS/CarPlay, Android/Auto; server browse | High music/audiobook polish; inherently platform-specific surface. |
| F-23.28 | Wearable control/offline listening | **Absent** | No WatchConnectivity/watchOS/Wear OS app or offline receiver found; phone OS transport controls do not create wearable client. | Wearable listening and controls | F-10.40 | L controls; XL independent offline (raise M) | iOS/Watch, Android/Wear; server scopes | Useful choice; new platform support cost and receipt lifecycle. |
| F-23.29 | Classical compositions, performances and richer credits | **Partly present** | Music metadata/tag/provider plumbing exists (S/server/internal/metadata/musicbrainz.go:113), but no composition/performance/conductor model/browse graph found. | Classical music graph and credits | F-10.12, F-10.15 | XL (raise L) | All | Freedom for specialist listeners; maintain identities incrementally. |
| F-23.30 | Rich music editorial presentation and metadata completeness | **Partly present** | S/server/internal/metadata/musicbrainz.go:113 enriches releases; PA/Backlog.md:21 records related-artist work and :35 unrendered label/albumType/country/years. No complete editorial service/linked credits display. | Music editorial and metadata presentation | F-06.25, F-10.12 | M existing fields; XL licensed feed (agrees) | All | Render existing facts first; licence/provenance must fit free product. |
| F-23.31 | Personal listening recap, streaks and exploration summaries | **Partly present** | S/server/internal/catalog/play_history.go:1 persists observed plays; httpapi/play_history.go:47 summarizes owner history. No personal recap/streak/exploration report surface found. | Personal listening recap | F-10.9, F-18.58 | L (raise M for maintained reports) | All | Optional local polish; privacy/reset/date coverage must be honest. |
| F-23.32 | Music-aware transitions and visualizations | **Partly present** | S/server/internal/operations/preferences.go:195 stores crossfade; media-analysis supports waveform jobs. No music-envelope/album-aware Sweet-Fades strategy found; do not call all transition/visualization plumbing absent. | Music-aware transitions and visualizations | F-10.25 | L (raise M for audio policy) | All | Choice fit; preserve gapless albums and disclose output processing. |
| F-23.33 | Reviewed audiobook metadata agent | **Partly present** | S/server/internal/metadata/library_agent.go:114 advertises books with MusicBrainz/CAA; :235 enables provider policy. Worker musicbrainz.go:113 is album/recording-shaped, not a reviewed narrator/book-series matcher. PA/Backlog.md:24 records archived unreviewed work; PC/scope-completion.md:7 explicitly defers remote audiobook enrichment. | Audiobook metadata enrichment | F-06.24, F-10.32 | L review/provider (M optimistic) | All | Freedom fit; displayed agent description overstates specialized book enrichment. |
| F-23.34 | Audiobook chapter editor and personal bookmarks | **Partly present** | S/server/internal/httpapi/media_analysis.go:105 supports generic chapter marker edits; existing book chapters play. No private position+note bookmark route/UI found; PA/Specs/Spec — Playback Protocol v1.md:223 specifies it. | Audiobook chapter editing and bookmarks | F-10.29, F-10.31 | L (raise M for book clocks/UI) | All | Local listening polish; shared chapter facts and private notes need separate scopes. |
| F-23.35 | Podcast subscriptions and automatic episode acquisition | **Absent** | Only podcast exclusion token in S/web/src/player/dolby-vision.ts:46; no RSS feed/enclosure ingestion, subscriptions or retention service found. | Podcast subscriptions and acquisition | F-10.34, F-24.16 | XL (raise L) | All | Adjacent freedom, substantial new scheduled trust/storage domain. |
| F-23.36 | Private RSS publication of audiobooks/podcasts | **Absent** | No authenticated RSS publication/feed grant route found. Existing audio streaming is not a podcast-feed endpoint. | Private RSS audio feeds | F-24.16 | L (raise M for grant lifecycle) | Server; client setup All; external feed readers | Choice fit; revocable scoped bearer URLs and limitations must be explicit. |
| F-23.37 | EPUB/PDF/comics with format-specific progress | **Absent** | S/server/internal/persistence/README.md:64 reserves future kinds, :76 mentions page progress; no EPUB/PDF/comic parser or first-party reader workflow found. | Reading libraries and progress | F-10.35, F-18.47, F-24.22 | XL (raise L) | All | New product scope; time-based audiobook progress cannot substitute. |
| F-23.38 | OPDS and page-streaming interoperability | **Absent** | No OPDS/page-streaming endpoints/models found. Reserved book/progress types do not implement a reader catalogue. | OPDS and reader interoperability | F-24.22 | L adapter after reading model (M low) | Server; external readers; setup Clients | Freedom fit; large catalogues must page with explicit grant boundaries. |
| F-23.39 | Read-only photo/home-video library | **Absent** | S/server/internal/persistence/README.md:64 reserves photo kinds; no photo/home-video kind scanner, albums or slideshow found. Developer Gallery is unrelated. | Photo and home-video libraries | F-18.47, F-21.36, F-24.15 | XL (raise M/L) | All | Useful private-media freedom; new privacy/scale/presentation scope. |
| F-23.40 | Full mobile photo backup and intelligent photo management | **Absent** | No camera-backup/upload/faces/maps/dedup/semantic-photo service found; read-only media sources and image-picker artwork upload are narrower. | Mobile photo backup and management | F-23.39 | XL (agrees very large) | All; uploads Handheld | Large personal-cloud product change; originals/trust/background lifecycle central. |
| F-23.41 | TV Everywhere source integration | **Absent** | Existing live source adapters include M3U/Xtream/HDHomeRun; no provider-login TV Everywhere source found in server/API/client settings. | TV Everywhere integration | — | XL built-in; L bridge (L fair) | All; server source adapter | Freedom via optional external bridge; upstream DRM/provider changes limit consistency. |
| F-23.42 | Library Channel export to M3U/XMLTV or virtual tuners | **Partly present** | S/server/internal/httpapi/library_channels.go:165 serves own protocol; livechannels/library/guide.go:105 publishes guide/logo facts. No outward M3U/XMLTV/virtual-tuner route found; HLS playback manifests are not channel-list export. | Library Channel IPTV export | — | L (raise M for scopes/interop) | Server; all Clients configure; external IPTV | Good ecosystem choice; scoped revocable legacy grants required. |
| F-23.43 | Advanced station clocks, filler, bumpers and channel graphics | **Partly present** | S/server/internal/livechannels/library/model.go:178 has timed blocks/priority/fallback/overrun and :195 overlay config; playbackruntime/linear.go:890 issues scoped logo URL. Missing dedicated filler/bumper/advanced station-clock sequences; rendering acceptance unestablished. | Advanced station scheduling and graphics | F-11.25 | L remaining features (M/L fair) | All | Enthusiast depth; existing clocks/graphics should not be rebuilt. |
| F-23.44 | Kometa-style external collection and metadata rules | **Partly present** | S/server/internal/catalog/resources.go:514 stores validated saved-view queries; static collections exist. No Kometa/rule-import/external-list adapter found. | External curation rule adapters | F-06.5, F-18.30 | L (raise M for durable adapter) | All | Freedom fit; preserve manual fields and incrementally maintained membership. |
| F-23.45 | Reversible overlays and artwork treatments | **Absent** | No automatic metadata-badge/quality poster treatment generator found. Video subtitle/OS overlay matches are unrelated. Existing selected artwork remains separate. | Reversible poster treatments | — | M/L (M narrow badges plausible) | All | Optional customization; derivative artwork and restrained readable defaults. |
| F-23.46 | Shared smart collections as curated entities | **Partly present** | S/server/internal/catalog/resources.go:514 restricts queries to views, :533 restricts visibility to collections. Live saved views already exist; shared curated query entity/art/publishing model missing. | Shared smart collections | F-18.30, F-24.6 | L (raise M for authority/schema) | All | Reuse query engine; clarify whose personal predicates define shared membership. |
| F-23.47 | Richer history queries, exports, and third-party stats adapters | **Partly present** | S/server/internal/httpapi/play_history.go:18 serves history and :47 summaries; PA/Backlog.md:52 lacks title search/per-row delete/who-watched and :38 current CSV integration. No Tautulli/Jellystat adapter found. | History query and export depth | F-12.17, F-18.58 | M/L (M fair) | All | Operational polish; bounded indexed questions and viewer privacy. |
| F-23.48 | External conversion worker integration and in-place replacement contract | **Partly present** | Prepared artifacts exist; PA/Backlog.md:30 reports in-place replacement facts not reread. No Tdarr/Unmanic completion/replacement adapter found; read-only source rule does not authorize Portico rewriting originals. | External conversion-worker integration | F-23.5 | L connector; XL distributed encoders (agrees) | Server; admin Clients | External owner choice fits; exact invalidation before adding worker orchestration. |
| F-23.49 | Off-app operational/request alerts through optional adapters | **Absent** | PA/06 Clients.md:73 excludes app push; PA/07 Hosted Services.md:27 excludes alert email. No external operational/request alert webhook found. Optional third-party adapters are not themselves explicitly banned. | Off-app alerts through adapters | F-09.26, F-13.8, F-23.4 | L external adapter (raise M) | Server; all Clients configure; external destinations | Choice-compatible external webhooks; built-in email/push would change recorded decision. |
| F-23.50 | Optional paid relay with a sustainable quality contract | **Planned** | PA/Backlog.md:99 and PC/scope-completion.md:39 record future separate paid relay; no media relay implementation found. | Paid remote-access relay | F-09.19, F-22.22, F-24.5 | XL (raise L) | All | Ease/freedom fit only with separate sustainable privacy/capacity contract. |
| F-23.51 | Guided remote-access and failure diagnosis | **Partly present** | S/packages/client-core/src/remote-access.ts:30 distinguishes CGNAT/proxy/IPv6/mapping failures and :28 has owner-triggered check; native/web settings expose networking. Missing comprehensive DNS/hairpin/VPN/client cause diagnosis and acceptance. | Guided remote-access diagnosis | F-09.5, F-22.6, F-22.7, F-22.10, F-22.16, F-24.21 | L (raise M for device/network evidence) | All | High ease value; never equate directory/certificate with reachability. |
| F-23.52 | A desktop playback application/companion | **Planned** | PA/Backlog.md:95 plans macOS/Windows desktop companion; browsers work. No shipped desktop playback application established; server tray/installer scope differs from broad codec player. | Desktop playback companion | F-09.11, F-09.12, F-14.26, F-16.14, F-24.12 | XL full player; L tray (M/L low) | Server, web; macOS/Windows; Linux undecided | Choice fit; define companion versus player before estimating. |
| F-23.53 | Additional living-room and Vision platform reach | **Partly present** | Apple/Android/Fire/Vega clients exist; PC/scope-completion.md:37 retains later Tizen/webOS qualification. No qualified Vision/console/vendor-TV release established; Roku excluded from this review. | Expanded platform coverage and acceptance | F-19.15, F-24.3, F-25.15 | XL (raise L) | Clients plus Tizen/webOS/Vision; server contracts | Consistency is primary goal; code and storefront/device evidence are distinct. |
| F-23.54 | Non-English UI and localization acceptance | **Partly present** | PA/06 Clients.md:19 states English launch/more planned; S/packages/i18n/src/catalog/regional.ts:1 derives en-CA/en-GB. No other translated catalogue/RTL acceptance found. | Non-English localization | F-16.3, F-19.19, F-24.20, F-25.20 | XL fleet; M translation infrastructure (agrees) | All | Strong household consistency; chosen locales need real device/layout acceptance. |
| F-23.55 | Controlled provider/plugin extension points | **Absent** | Built-in provider selection/registry exists; no runtime plugin loader or owner external-provider protocol found. No-code-contribution rule is not a prohibition on owner integrations. | Provider and plugin extension protocols | F-09.25 | XL plugins; L external providers (M low) | Server; all Clients capability UI | Freedom benefit; resource/trust bounds and one-provider-per-library policy retained. |
| F-23.56 | Optional themes/skins and authorable layouts | **Partly present** | Authored Home arrangement/appearance already exist (S/packages/client-core/src/presentation/settings-structure.ts:241). No skin/layout engine found; PC/visual/contract.md:350 keeps fixed structure. | Themes and authored-layout customization | F-16.4, F-16.5, F-18.45, F-21.33, F-24.23 | L limited theme; XL skins (M low) | Clients | Limited choice fits; arbitrary skins require revisiting consistency/visual contract. |
| F-23.57 | Universal availability and absent-title discovery | **Partly present** | Local Home/Discover recommendations exist; no universal regional availability/absent-title catalogue adapter found. Metadata providers enrich identified library media, not streaming availability. | External availability and absent-title discovery | F-07.8, F-12.6, F-21.25 | XL regional service; L adapter (M low) | All | Optional freedom; explicit external/local separation and cheap Hosted. |
| F-23.58 | Public lists, reviews, discussions and a cross-server social graph | **Partly present** | S/packages/client-core/src/social-playback.ts:1 provides Together; PC/sharing-and-social-playback/contract.md:24 excludes public social profiles from current contract scope. No public lists/reviews/discussion/cross-server graph found. | Public social graph and reviews | F-13.12, F-18.21 | XL (raise L) | All; new external/Hosted service | Major privacy/cost expansion; existing household social benefit is real. |
| F-23.59 | Ad-supported content and rentals | **Excluded by decision** | PA/01 Overview.md:3 defines free non-commercial personal media server; PC/decisions.md:30 keeps paid relay as sole separate paid scope. No commercial content distribution found; this would overturn current product model. | Commercial content and rentals | — | XL (agrees very large) | All; new licensed service | Conflicts with free/non-commercial/private-library identity; not a launch defect. |
| F-23.60 | Reliable edition/version grouping at ingestion | **Partly present** | S/server/internal/httpapi/inventory.go:279 calls AssociateInventoryVersion; detail.ts:37 carries edition and sources. PA/Backlog.md:29 reports duplicate files instead of versions. Automatic grouping and complete preplay/cut timeline workflow missing. | Edition and version grouping | F-05.5, F-05.6, F-18.19, F-21.2, F-24.2 | L (M/L fair) | All | High owner-choice value; do not merge different edits by title alone. |
| F-23.61 | First-party app feature completion and acceptance | **Partly present** | PA/Backlog.md:62 lists native/history/recording omissions, :70 dropped-frame/buffer gap, :82 Cast acceptance. Existing feature modules/UI are substantial; full reachable parity and physical fleet evidence absent. | First-party feature and device qualification | F-05.27, F-21.13, F-24.3, F-25.2, F-25.15 | XL (raise L fleet) | All | Highest consistency/polish priority; advertised capabilities need per-platform evidence. |
| F-23.62 | Published compatibility policy, SDK recipes and example adapters | **Partly present** | S/server/api/README.md:1 and JSON/YAML contracts document own API; PC/scope-completion.md:15 requires independent semantics. No stable release-window policy/published reference adapter/SDK release established. | Supported external API and SDK | F-08.3, F-08.16, F-23.9, F-24.24, F-25.1, F-25.25 | L (raise M for support window) | All; third-party clients | Freedom fit; API stability is a real support commitment. |
| F-23.63 | DLNA with a clearly documented permission boundary | **Planned** | PA/Backlog.md:94 plans DLNA; PC/scope-completion.md:17 requires authenticated scoped directory/expiring renderer capabilities. S/server/api/social.md:437 calls it omitted. Comments/lane prefix are not implementation; newer contract overrides whole-server backlog wording. | Scoped DLNA interoperability | F-09.20, F-10.41, F-13.16 | L/XL (M/L low) | Server; all Clients configure/control; legacy devices | Freedom with explicit renderer limits; anonymous full-library exposure is not authorized. |
| F-23.64 | Optional player enhancements with truthful quality claims | **Partly present** | S/server/internal/decoder/hardware.go:522 has deinterlace/scale paths; no named client AI upscaler found. Existing scaling must not be described as absent enhancement. | Optional video enhancement filters | F-03.10 | XL AI fleet; M specific filter (agrees) | All; server filters/device renderers | Low priority until viewing evidence; preserve original rendering and truthful claims. |
| F-23.65 | Opt-in cinema trailers/pre-roll and richer extras presentation | **Partly present** | S/packages/client-core/src/detail.ts:8 defines trailer/featurette/behind-the-scenes and :129 parses extras. No automatic cinema/pre-roll queue owner or external trailer supplier found. | Cinema pre-roll and extras presentation | F-02.15, F-06.18, F-18.17 | L (raise M for full parity) | All | Optional theater choice; immediate main-feature play and separate eligibility/state retained. |

## Document 24 — Community demand

| ID | Claimed gap | Verdict | What exists / where (path) | Canonical name | Duplicates | Size | Platforms | Fit |
|---|---|---|---|---|---|---|---|---|
| F-24.1 | Import household state from Plex/Jellyfin | **Absent** | No external household export/import mapping workflow found in server/API/core/clients; S/server/internal/backup/backup.go:1 handles Portico’s own format. Existing scanning imports files, not old viewer investment. | Household state migration | F-08.5, F-12.4, F-23.1 | XL (L understated) | All | Highest migration/freedom value; staged matching and undoable batches. |
| F-24.2 | A reviewable versions/editions/merge workflow | **Partly present** | S/server/internal/httpapi/inventory.go:279 associates physical object to logical item; metadata_repair.go:91 previews merge; N/src/screens/title/PlaybackChoices.tsx:13 defers Version. Automatic grouping/cut timelines/full review UI incomplete; PA/Backlog.md:29 already records it. | Edition and version grouping | F-05.5, F-05.6, F-18.19, F-21.2, F-23.60 | L/XL (L fair) | All | Owner choice fits; distinct cuts need conservative identity/progress. |
| F-24.3 | Complete physical TV acceptance and storefront availability | **Partly present** | Existing tvOS/Android TV/Fire/Vega sources and debug builds are real; PC/scope-completion.md:37 requires individual acceptance. Source cannot establish store listings or actual physical results. | Expanded platform coverage and acceptance | F-23.53, F-23.61, F-25.5, F-25.15 | XL (raise L) | Clients; extra vendor TVs | Central polish/consistency goal; list code-ready and qualified separately. |
| F-24.4 | Arbitrary owner-configured OIDC/SSO | **Absent** | No arbitrary IdP issuer/discovery/client/subject mapping or LDAP adapter found; S/server/api/identity.openapi.yaml and native account adapters implement Portico/Google/Apple, not owner federation. | Owner-configured OIDC or LDAP login | F-01.9, F-15.17 | L/XL (L fair) | All | Strong self-hoster freedom; preserve independent local recovery/password path. |
| F-24.5 | Managed remote reachability beyond port forwarding | **Planned** | PA/Backlog.md:99 and PC/scope-completion.md:39 plan future paid relay. S/packages/client-core/src/remote-access.ts:30 has owner-triggered NAT/IPv6/proxy advice; mapping is not CGNAT traversal. | Paid remote-access relay | F-09.19, F-22.22, F-23.50 | XL (raise L) | All | Ease value; direct choices retained and relay economics separately governed. |
| F-24.6 | Smart playlists as playable saved queries | **Partly present** | S/server/internal/catalog/resources.go:514 persists typed query views but rejects collection query definitions; native SavedPages.tsx:183 can present/play views. Explicit playlist occurrences exist; persistent dynamic-query playlist entity missing. | Smart playlists | F-07.22, F-10.7, F-12.15, F-23.46 | L (raise M for queue semantics) | All | Reuse query engine; retain duplicates and stable playback revisions. |
| F-24.7 | Accept managed offline use and consider automatic sync rules | **Partly present** | N/src/bridge/offline-downloads.ts:359 persists/refills next, :565 deletes watched, :553 journals progress; Downloads.tsx:175 exposes policy. Keep-N show rules/charging and physical termination/travel acceptance remain. | Rolling offline episode rules | F-14.11, F-21.9, F-23.19 | L rules; XL qualification (agrees) | All; local Handheld | Complete promised travel reliability first; automatic deletion/refill must be explicit. |
| F-24.8 | Music radio, sonic similarity and specialist continuity | **Partly present** | Shared gain/gapless/crossfade/queue infrastructure exists; PA/Backlog.md:17 plans radio/mixes with similarity undecided; PC/scope-completion.md:7 retains instant-mix obligation. No radio generator/sonic engine found. | Music radio and instant mixes | F-10.4, F-10.5, F-23.21, F-23.22 | L metadata; XL sonic (L low for fleet) | All | High music polish; incremental bounded similarity and queue ownership. |
| F-24.9 | Accept per-person resource policies and expose finer controls where useful | **Partly present** | S/server/internal/access/limits.go:96 stores MaxStreams/schedule/bitrate/content; enforcement.go:105 counts streams; playbackv1/admission_gate.go:5 serializes starts. S/web/src/screens/server/People.tsx:64 and N/src/screens/server/PeoplePanels.tsx:76 expose limits. Missing finer conversion/download/DVR budgets and acceptance, not basic account policies. | Per-person resource policy depth | F-01.1, F-02.9, F-18.34 | M/L remaining; not new limiter (agrees) | All | Strong consistency; owner-selected limits differ from predictive quality degradation. |
| F-24.10 | Better subtitle controls across every player | **Partly present** | N/src/player/PlayerOptions.tsx:267 offers source-relative ±0.5s; S/packages/client-core/src/presentation/settings-structure.ts:206 gates appearance to web. Native PorticoPlayer.m:54 hardcodes overlay font/layout; dual tracks absent. | Subtitle control parity | F-04.7, F-04.9, F-17.3, F-17.4, F-18.50, F-20.12, F-21.3 | L native styling; XL dual renderer (M low) | All | Accessibility/polish priority; qualify style/timing independently per renderer. |
| F-24.11 | External player handoff with progress sync | **Absent** | No external-player launcher/settings/return-progress authority found in native/web/core or API. Existing system local-download view is narrower. | External player handoff | F-20.18, F-21.34 | L (M/L fair) | All; OS/player-specific adapters | Optional freedom; scoped grants, limitation copy and one writer required. |
| F-24.12 | A desktop application or deliberate companion path | **Planned** | PA/Backlog.md:95 plans macOS/Windows companion; no desktop playback app/codec shell found. Existing server installers are not a desktop media client. | Desktop playback companion | F-09.11, F-09.12, F-14.26, F-16.14, F-23.52 | XL full app; L companion (agrees) | Server, web; macOS/Windows; Linux undecided | Choice benefit; define player/output scope before allocating platform cost. |
| F-24.13 | Fuzzy and alternative-title search | **Partly present** | S/server/internal/catalog/search.go:146 uses FTS exact/prefix boosts, :279 echoes token_prefix. Provider original titles/synonyms exist upstream but no alias/typo candidate index used on search path found. | Fuzzy and alternative-title search | F-07.5, F-07.9, F-12.10 | L (raise M if complete aliases) | All | Ease fit; bounded visible candidates and literal-query transparency. |
| F-24.14 | Household profile switching, PINs and TV identity | **Partly present** | N/src/screens/auth/DirectProfiles.tsx:81 renders PIN selector; Profiles.tsx:3 manages PIN recovery; session.tsx:791 chooses profiles. Missing OS multiuser mapping/optional launch prompt and actual TV-role acceptance; profiles/PIN are not absent. | Household profiles and OS user mapping | F-01.18, F-01.19, F-19.7, F-21.29, F-21.31 | M OS integration; L acceptance (M narrow fair) | All; OS mapping tvOS-specific | Family ease and consistency; viewer scope must fence offline and admin surfaces. |
| F-24.15 | Viewer folder browsing and home videos | **Partly present** | S/server/internal/catalog/inventory.go:204 preserves storage identity; httpapi/inventory.go:264 exposes owner operations. No viewer opaque folder hierarchy/home-video/photo library route found. | Viewer folder and home-video browsing | F-18.11, F-18.47, F-21.36, F-23.39 | L folder browse; XL media domain (M/L low) | All | Freedom fit; never expose physical owner paths in viewer hierarchy. |
| F-24.16 | Podcast subscription and RSS workflows | **Absent** | No RSS subscription/feed fetch/episode retention/acquisition or private feed publication found; S/web/src/player/dolby-vision.ts:46 only excludes a podcast token. | Podcast subscriptions and acquisition | F-10.34, F-23.35, F-23.36 | XL (raise L) | All | Coherent adjacent expansion; explicit external-fetch/storage trust model. |
| F-24.17 | Trakt/ListenBrainz/Simkl/anime tracking export | **Absent** | No Trakt/ListenBrainz/Simkl/Last.fm/anime tracker adapters or opt-in credential settings found; personal state remains local-server authoritative. | External tracking and discovery adapters | F-08.4, F-10.6, F-12.1, F-21.24, F-23.3, F-23.7 | L per service; XL bundle (raise M) | All | User freedom/portability; outbound data consent cannot follow implicitly from Hosted login. |
| F-24.18 | Request/discovery integration with Seerr | **Absent** | No request/approval/acquisition/available adapter or external-ID request UI found across server/OpenAPI/core/web/native/i18n. | Seerr request integration | F-08.10, F-12.5, F-21.23, F-23.6 | L (raise M for full lifecycle) | All | Good opt-in household ease; keep acquisition responsibilities separate. |
| F-24.19 | Cross-server search with independent partial results | **Partly present** | S/packages/client-core/src/known-servers.ts:3 retains address list without independent credentials; server-connections.ts:17 one selected record. No aggregate search fan-out/partial results found. | Merged multi-server browsing | F-01.23, F-12.25, F-21.8, F-22.17 | XL (raise L) | All | Freedom benefit; one failed server cannot hold others hostage or leak profile scope. |
| F-24.20 | Internationalization beyond regional English | **Partly present** | S/packages/i18n/src/catalog/regional.ts:1 derives English variants; PA/06 Clients.md:19 explicitly plans more languages after English launch. No non-English catalogue/RTL fleet acceptance found. | Non-English localization | F-16.3, F-19.19, F-23.54, F-25.20 | XL (agrees) | All | Consistency/freedom fit; regional spelling is a real foundation, not full translation. |
| F-24.21 | Explainable startup/scan/playback readiness | **Partly present** | S/server/internal/httpapi/router.go:816 publishes setup/build status; library scan jobs :1008 report progress; S/packages/client-core/src/playback-messages.ts:12 has source/conversion reasons; remote-access.ts:30 gives NAT guidance. Missing single explainable stage/cause UX and device acceptance. | Guided remote-access diagnosis | F-22.10, F-23.51 | L (raise M for all readiness states) | All | Strong ease/polish; expose actionable causes without technical clutter. |
| F-24.22 | Reading/OPDS and book portability | **Absent** | S/server/internal/persistence/README.md:76 reserves page/fraction progress; no actual EPUB/OPDS/comic reader/scanner/export found. | Reading libraries and progress | F-10.35, F-18.47, F-23.37, F-23.38 | XL reader; L OPDS after model (agrees) | All; external reader APIs | Distinct product scope; generic progress is not delivered portability. |
| F-24.23 | Optional customization and home integrations | **Partly present** | S/packages/client-core/src/presentation/settings-structure.ts:241 has appearance/Home choices; N/src/bridge/NativeActivity.ts:34 Apple intents exist. No custom CSS/skin/automation/theme-music general engine found. | Themes and authored-layout customization | F-16.4, F-16.5, F-18.16, F-21.20, F-21.33, F-23.55, F-23.56 | M narrow tokens; XL skins/integrations (S–L too vague) | All | Supported choices fit freedom; reconcile fixed authored visual/Reduced Motion rules. |
| F-24.24 | A supported integration/export contract | **Partly present** | S/server/api/README.md:47 allows pre-release changes; JSON plus authored YAML, core routes and types exist. PC/scope-completion.md:15 requires independent supported API; SDK releases/semantic examples/stable policy/portable export remain incomplete. | Supported external API and SDK | F-08.3, F-08.16, F-23.2, F-23.9, F-23.62, F-25.25 | L (raise M for support/export) | All; external consumers | Core freedom/open-data promise; generated schemas alone do not fulfill it. |
| F-24.25 | Reliable repeated-intro/credits detection across episodes | **Partly present** | S/server/internal/mediaanalysis/detectors.go:24 uses chapter labels, :75 intro luminance and :98 credits end-region. Marker/skip UI exists; repeated-season audio detector/scene-aware credit boundaries missing. | Repeated-intro detection and marker import | F-04.13, F-04.14, F-04.15, F-23.15, F-23.16 | L/XL (L fair) | All | Strong polish if trustworthy; preserve manual/approved exact-timeline markers. |

## Document 25 — Engineering and project model

For this document, the client feature is often already real; the claimed gap is its release gate, inventory, policy or qualification. No CI job, package, signer or physical device was executed/queried.

| ID | Claimed gap | Verdict | What exists / where (path) | Canonical name | Duplicates | Size | Platforms | Fit |
|---|---|---|---|---|---|---|---|---|
| F-25.1 | A release compatibility contract | **Partly present** | S/server/internal/httpapi/router.go:816 returns version/build; S/server/api/README.md:47 explicitly permits pre-release breaks. PC/scope-completion.md:15 requires compatibility outcomes; no release-era minimum/window/deprecation/downgrade policy found. | Release compatibility negotiation | F-16.15, F-19.14, F-22.9, F-23.62, F-25.11 | L (M policy alone plausible) | All | Consistency/ease priority before stable v1; no perpetual pre-release aliases required. |
| F-25.2 | One enforceable release qualification ledger | **Partly present** | S/.github/workflows/release.yml:59 reruns CI and scripts/build-server-release.py:158 seals source/artifact identities; PC/conformance.md:3 requires exact build/review/behavior/client evidence. No machine-enforced all-platform ledger/exceptions/final evidence binding found. | Release qualification ledger | F-23.61, F-24.3, F-25.4, F-25.15 | L (M/L fair) | All plus Hosted/operator components | Core polish/consistency; missing evidence must remain unqualified. |
| F-25.3 | Pinned cross-repository source tuples | **Partly present** | RN/scripts/shared-packages.mjs:38 hashes selected copies; native CI :24 checks server default branch without ref. S/scripts/build-server-release.py:128 snapshots dependencies; no server/native/Hosted source tuple pin/version check found. | Pinned cross-repository source tuple | — | M (S/M fair) | All; Hosted/build inputs | Reproducible consistency; preserve standalone repository builds. |
| F-25.4 | Media and scale jobs required for release | **Partly present** | S/.github/workflows/ffmpeg-toolchain.yml:56 runs full bundle proof and :70 two media tests; S/scripts/build-server-release.py:148 race-tests server. S/server/internal/testtier/testtier.go:11 skips media by default; no all/performance tier release gate found. PA/Backlog.md:57 records full-suite timeout. | Release media and scale gates | F-25.2 | L (raise M if timeout/benchmarks unresolved) | All; qualified server hardware | Scale/polish fit; tie costly evidence to actual release source/bundle. |
| F-25.5 | Apple and Vega CI, release-mode Android validation | **Partly present** | RN/.github/workflows/ci.yml:50 assembles Android debug; no Apple xcodebuild/native-test/Vega lane or Android release-mode packaging lane found. Existing JS/core/type checks are real. | Native platform CI qualification | F-19.13, F-23.53, F-24.3 | L (M/L fair) | iOS, tvOS, Android, Android TV, Fire TV, Vega | Consistency obligation; compile/store/media/device results separate. |
| F-25.6 | Installed-package smoke matrix | **Planned** | PC/scope-completion.md:29 requires install/restart/upgrade/uninstall evidence; S/scripts/package-release.sh:18 checks packaging inputs but no install/upgrade/state-preservation smoke matrix found. | Installed-package smoke matrix | F-09.11, F-09.12 | L (raise M for every OS) | Server packages Linux/macOS/Windows; web bundled | Low-ceremony reliability fit; package presence is not install acceptance. |
| F-25.7 | Complete signed final-artifact inventory | **Partly present** | S/scripts/build-server-release.py:158 signs binary/source/toolchain digest; package-release.sh:23 adds web after sealing; package-macos.sh:31 changes binaries with codesign. Release workflow :215 emits checksums/attestations, but no independently trusted complete final installer/web/service signature inventory found. | Signed final-artifact inventory | F-15.15, F-25.8, F-25.18 | L (raise M for trust/install changes) | All; server packages/container/static assets | Release integrity fits polish; re-sign exact final bytes after OS mutation. |
| F-25.8 | Official signer mode with protected authorization | **Partly present** | S/.github/workflows/release.yml:55 requires secret key before release; build-server-release.py:35 still generates a fallback key when absent. server/api/release-signing.md:3–9 requires isolation/review/rotation and admits unfinished enforcement. No protected separate signer/approved key identity gate found. | Protected official release signer | F-15.15, F-25.7 | L (raise M for custody/protected gate) | All release artifacts; build infra | Real lifecycle assurance; development convenience cannot imply official authenticity. |
| F-25.9 | One update-feed schema and channels contract | **Partly present** | S/scripts/write-public-update-manifest.mjs:23 writes flat schema/channel/version; server/internal/administration/maintenance.go:491 expects channels map and :526 refuses incompatible shape. Update notice/check path exists; generator-to-consumer schema/channel fixtures absent. | Unified update-feed contract | F-09.33, F-16.12 | M (S/M fair) | Server; all Clients update notice; Site | Immediate ease/polish gain; manual installation remains deliberate. |
| F-25.10 | Public upgrade and breaking-change notes | **Partly present** | S/.github/workflows/release.yml:228 generates release notes and :230 publishes generic preview guidance. No required curated platform/schema/client/rollback note fields or check found. | Upgrade and breaking-change notes | F-25.1, F-25.22 | S/M per release (agrees) | All; server schema/platforms | Ease and honest consistency; generated commits alone do not explain upgrade implications. |
| F-25.11 | Prior-version API/semantic conformance gate | **Absent** | S/.github/workflows/ci.yml:51 checks current generated contracts; no supported-release OpenAPI diff/old-client semantic vectors found. PC/scope-completion.md:15 requires public semantic/version fixtures but does not specify a prior-release gate/window. | Prior-release semantic conformance gate | F-22.9, F-25.1 | L initial; M ongoing (M low initial) | All | Consistency fit once compatibility promised; pre-release breaks remain authorized. |
| F-25.12 | Broader vulnerability and dependency qualification | **Partly present** | S/.github/workflows/security.yml:18 runs gitleaks/CodeQL; Dependabot configuration exists. No govulncheck/reachable-vulnerability/dependency-review/native-sanitizer gate found in workflows/scripts. | Dependency security qualification | F-15.16 | M/L (S/M low for native fleet) | All plus build infra | Practical security without UX burden; classify reviewed reachable findings, not dependency counts. |
| F-25.13 | Scheduled parser fuzzing and minimized corpus ownership | **Partly present** | S/server/internal/httpapi/fuzz_test.go:34–156 defines five fuzz targets with seeds. No scheduled -fuzz campaign/minimized-corpus ownership native parser lane found. Ordinary go test runs seeds only. | Scheduled parser fuzzing | F-15.16 | L (raise M for native corpus/resource bounds) | Server; native parser adapters; Clients indirectly | Hostile-input/weak-hardware fit; bounded triaged campaigns. |
| F-25.14 | Real rendered UI and small end-to-end journeys | **Partly present** | S/web/test/helpers/component-harness.mjs:4 explicitly models callbacks, not render/focus/scheduling; web/test/integration/setup-flow.mjs:63 exercises API setup. No browser/RN rendered journey gate found in package/workflow searches. | Rendered UI and end-to-end journeys | F-16.16, F-19.12 | L/XL (M/L fair) | Clients; disposable server fixture | Essential polish/accessibility; small meaningful journeys beat mirrored unit snapshots. |
| F-25.15 | Physical-device acceptance matrix | **Planned** | PC/scope-completion.md:37 and governance.md:15 explicitly require physical boundary evidence while current native testing is simulator-only. Dated handoff logs exist; no complete release-bound matrix/results established. | Physical-device acceptance matrix | F-23.53, F-23.61, F-24.3, F-25.2 | XL ongoing (raise L) | All; real outputs/weak hosts | Central consistency promise; never infer HDMI/HDR/background from shared builds. |
| F-25.16 | An architecture dependency/cohesion map | **Partly present** | PC/architecture.md:25 assigns domain/table ownership and :9 bounded adapters; Go internal/package boundaries exist. No maintained actual import/callback/state-lifetime graph/forbidden-edge AST gate found in scripts/CI. | Architecture ownership and dependency map | — | M/L (M narrow map plausible) | Server, core/web/native/Vega; Hosted | Supports sustained polish/scale; no framework/microservice rewrite implied. |
| F-25.17 | Test cost and flakiness observability | **Partly present** | S/scripts/runner-test.sh:10 supports -json, :43 separate performance tier, :219 timeout; PA/Backlog.md:57–59 records expensive/full-suite costs. No maintained duration/skip/retry dashboard or flake ownership gate found. | Test duration and flakiness reporting | — | M (S/M fair) | All test lanes/build infra | Fast feedback fit; do not hide flaky correctness failures as permanent optional work. |
| F-25.18 | Release SBOM and notice completeness verification | **Partly present** | S/.github/workflows/release.yml:181 enables container SBOM and :218 scans release artifacts; S/scripts/verify-ffmpeg-manifest.py:12 checks licence presence; package-release.sh:24 includes notices. No unpacked final Go/JS/native/font/FFmpeg completeness reconciliation found. | Release SBOM and notice completeness | F-15.15, F-25.7 | L (raise M for inventory verification) | All packaged platforms/container/JS/media bundle | Open-source polish/maintainability; existing SBOM infrastructure is not absent. |
| F-25.19 | A maintained third-party patch register | **Partly present** | S/server/internal/thirdparty/sqlite/PORTICO.md:3 pins upstream/patch rationale/upgrade checks; third_party/hls.js/README.md:3 pins source/provenance; third_party/ffmpeg/sources.lock.json:15 pins recipes/patches. No unified owner/retirement/upgrade register across components found. | Third-party patch maintenance register | — | M (S/M fair) | All; server SQLite/FFmpeg/web HLS/native libraries | Good existing practice; remaining work is consolidation and responsibility, not recreating provenance. |
| F-25.20 | Community translation intake without general code PRs | **Partly present** | S/packages/i18n/src/index.ts:16/69 provides catalogues/fallback; PA/06 Clients.md:19 plans more languages. RN/CONTRIBUTING.md:17 rejects external PRs; no community translation import/review/licence channel found. | Non-English localization | F-16.3, F-19.19, F-23.54, F-24.20 | L intake; XL sustained locales (M low) | All; translator workflow | Choice/household reach fits; intake policy must explicitly distinguish translation from code. |
| F-25.21 | An explicit Apple distribution-rights policy | **Absent** | RN/LICENSE:1 is GPL text; no Apple-specific extra grant/dual-licence/dependency-rights inventory/policy found in native/public docs. This is absence of recorded policy, not a finding that distribution is unlawful. | Apple distribution-rights policy | — | M inventory; decision effort unbounded (M tentative) | iOS, tvOS, future Apple desktop/Vision | Owner/legal product decision; consistency and licence freedom both matter. |
| F-25.22 | Release-bound documentation verification | **Partly present** | Site/scripts/check-build.mjs:8 checks required pages and :69 internal links/build identity; S/server/api/README.md:47 still has pre-release guidance and PA/Backlog.md is dated. No current artifact/platform/API/install-command applicability gate found. | Release-bound documentation verification | F-25.10 | M/L (S/M narrow checks fair) | All; Site/build inventories | Ease and truthful reach; existing site/link checking should be extended. |
| F-25.23 | Succession and review capacity plan | **Absent** | PC/governance.md:15 requires independent reviewers; S/server/api/release-signing.md:7 requires separate recovery custody. No named substitute operator/release/security authority or maintained succession process found in reviewed project/public guidance. | Succession and independent review capacity | — | M planning; ongoing human commitment (agrees) | All plus Hosted/key custody/operator roles | Sustainability supports trust; does not imply opening product/code authority. |
| F-25.24 | Voluntary funding and community support routes | **Partly present** | S/CONTRIBUTING.md:7 welcomes bug/feature/compatibility/docs issues and :22 directs private security reports; Site/src/pages/developers.astro:21 links issue guidance. No voluntary donations/sponsorship/maintenance-cost/community-moderation plan found. | Funding and community support routes | F-21.28 | S/M setup; ongoing moderation (agrees) | All; Site/issues/community | Free-product sustainability choice; existing feedback channels are present. |
| F-25.25 | Externally usable integration SDK/examples | **Partly present** | S/packages/contracts and server/api JSON/YAML contain generated/authored contracts; server/api/README.md:18 promises independent third-party endpoints. PC/scope-completion.md:15 requires public semantic fixtures; no released standalone SDK/reference authenticated adapter found. | Supported external API and SDK | F-08.3, F-08.16, F-23.9, F-23.62, F-24.24 | L (raise M for independent semantic fixtures) | All; external client developers | Strong freedom/thin-client fit; current internal package does not prove usable external integration. |

## Partly present — details

The implementation facts and missing boundaries below are static findings. A platform named in delivery scope is not certified by this review. Broad bundled proposals must be split before planning: implemented foundations, missing capability, and qualification are different work items.

### F-21.2 — Pre-play source and quality selection

N/src/screens/title/PlaybackChoices.tsx:13 defers Version; :39 selects current/first source. Audio/subtitle preplay and in-player quality exist (N/src/player/PlayerOptions.tsx:154); preplay version/quality missing. The small native task is offer/source selection and carrying the exact source into session creation. The larger task is scanner identity and edition-level timelines; exposing a selector cannot repair duplicate titles at ingestion.

### F-21.3 — Accessible track preferences

N/src/screens/title/PlaybackChoices.tsx:41 deduplicates subtitle languages; :45 sends language. S/packages/client-core/src/playback-v1/types.ts:35 retains exact ID/SDH. Exact preplay subtitle ID and SDH/description preferences missing. The in-player subtitle picker is more exact than the preplay picker. Retain stream identity from the offer through the pending choice; a language match cannot distinguish English SDH, forced dialogue and commentary. Accessibility preferences require corresponding server selection policy and client exposure.

### F-21.4 — Native browse density and list mode

S/packages/client-core/src/presentation/settings-structure.ts:622 stores card percentage; PA/Backlog.md:62 records native non-consumption. Native grids/song lists exist; adjustable native card grid and generic list switch absent.

### F-21.6 — Handheld player gestures

N/src/player/Timeline.tsx:23 implements drag seek and :37 accessible adjustment; N/src/player/VideoPlayer.tsx:446 exposes controls. No double-tap seek, brightness/volume swipe, pinch/hold-speed map found.

### F-21.8 — Merged multi-server browsing

S/packages/client-core/src/server-connections.ts:1 and N/src/app/session.tsx retain/select known servers; no merged Home/search catalogue owner found. Selection is one server at a time.

### F-21.9 — Rolling offline episode rules

N/src/bridge/offline-downloads.ts:359 persists next-after intent; :368 requests next episode; :565 auto-next/delete-watched. N/src/screens/Downloads.tsx:175 exposes Wi-Fi/storage rules. Missing per-show keep-N and owned-rule reconciliation. This is an actual continuing workflow, not just a dormant setting: end observation records a durable next-after intent, reconnection continues it, accepted preparations become local wants, and optional deletion occurs after the ended observation. It does not retain a configured N episodes per show or reconcile a show-owned desired set.

### F-21.10 — Grouped batch download management

N/src/screens/Downloads.tsx:43 offers per-file pause/resume/remove; :128 displays batch request progress. Local list is flat; no group selection or batch pause/retry/delete found.

### F-21.12 — Download OS progress surfaces

N/android/app/src/main/java/tv/getportico/portico/native/PorticoDownloadCenter.kt:354 creates foreground-transfer notification; N/Native/PorticoDownloads.m:209 deliberately shows completion in-app/toast only. No iOS ActivityKit/Live Activity or persistent OS completion entry found. Android foreground-service progress is required transport infrastructure; it is not evidence of an iOS Live Activity or a persistent finished-download entry. The Apple download module explicitly cites Justin’s no-notification rule. A local completion notification should therefore not be silently introduced as routine parity wiring.

### F-21.13 — Native saved-resource editing and sharing

N/src/player/EntryMenu.tsx:157 creates playlists, :212 creates collections; N/src/screens/title/SavedPages.tsx:95 reorders and :111 removes occurrences. Rename/delete/share and full collection entry management not exposed there. Create/add already exist in entry sheets, and playlist remove/reorder already use stable occurrence IDs. Do not implement a second saved-resource service. Finish naming/deletion/sharing and collection management on the existing server-backed services, then verify phone and TV reachability.

### F-21.14 — TV saved-view creation

N/src/screens/BrowseFilters.tsx:92 returns inline TV toolbar without Save; :113 handheld Save and :125 naming sheet exist. Server query resources already exist.

### F-21.15 — Native context-action parity

N/src/player/EntryMenu.tsx:87 dispatches play/state/download/metadata/playlist; :118 Details exists. It lacks rating/collection/file-info dispatch; N/src/screens/Detail.tsx:126 exposes refresh elsewhere.

### F-21.16 — Native item link sharing

N/src/screens/InviteSheet.tsx:37 and Together.tsx:200 use Share.share; N/src/app/routes.ts:25 includes links. No normal media-item share-sheet caller found.

### F-21.17 — Detail device playback and manual TV linking

N/src/screens/LinkTV.tsx:74 has manual code; N/src/screens/settings/SettingsPage.tsx:139 exposes it permanently in Settings. N/src/player/CastBar.tsx:65 controls active playback; detail page has no equivalent Play-on action. N/src/player/Destinations.tsx:89 lacks the permanent code peer required by PC/onboarding/contract.md:184; reuse the existing code service. Settings linking is reachable independently of discovery. The actual contract discrepancy is precise: Destinations renders AirPlay, Cast, discovered players or an empty/looking state, without Enter a TV Code. That missing entry and detail-page Play on can reuse the existing approver/receiver machinery.

### F-21.19 — Playback clock and finish-time options

N/src/player/Timeline.tsx:72 displays elapsed and :73 remaining; :12 supports live wall-clock labels. No finishes-at/system-clock policy or display toggle found.

### F-21.20 — TV ambiance personalization

S/packages/client-core/src/presentation/settings-structure.ts:244 has web backdrop option and :247 native Reduced Motion. No theme-song player/focus-glow preference found.

### F-21.22 — Phone companion navigation remote

S/packages/client-core/src/remote-playback.ts:1 and N/src/player/CastBar.tsx:65 provide authorized playback control. No D-pad/text/search commands or general companion screen found.

### F-21.25 — Search-provider adapters

N/src/screens/Search.tsx:1 uses server search; server metadata providers are enrichment, not alternate search services. Grouped local results/recent queries exist; external provider selection absent.

### F-21.26 — Viewer client diagnostics export

S/server/internal/httpapi/admin_diagnostics.go:148 accepts bounded viewer client-log uploads; N/src/screens/server/Troubleshooting.tsx:141 exports owner server JSON. No native viewer local ring-log collector/copy/share/clear UI found. The ordinary-viewer upload endpoint disproves any blanket assertion that the server has no client-log intake. It does not supply an app-owned redacted local log ring or a person-facing copy/share/clear screen. Owner support reports diagnose the server rather than the device’s complete lifecycle.

### F-21.27 — Preference portability and safe cache reset

S/packages/client-core/src/preferences.ts:1 persists scoped preferences; SettingsPage.tsx:138 resets recommendations. No preference export/import/default reset/cache-clear actions in authored settings.

### F-21.28 — Native legal and support pages

N/src/screens/settings/SettingsPage.tsx:288 renders About/legal footer; S/THIRD-PARTY-NOTICES.md:1 holds notices outside app. No browsable native dependency-license/support/donation page found.

### F-21.29 — Launch profile prompt and kids workspace

N/src/screens/auth/DirectProfiles.tsx:81 has Who’s watching/PIN; N/src/screens/Profiles.tsx:3 manages profiles. No always-prompt-on-launch preference or simplified kids shell found.

### F-21.30 — Assistant and launcher actions

N/src/bridge/NativeActivity.ts:34 enables Apple only; :44 indexes bounded lists. Android Top Shelf/launcher code exists separately, but no equivalent assistant activity indexing wired.

### F-22.1 — Editable pinned-server addresses

S/packages/client-core/src/server-connections.ts:7 stores paired origins; route-connection.ts:73 remembers up to eight. known-servers.ts:11 holds one displayed address per entry. No add/remove/order UI for one pin found.

### F-22.2 — Operation-class request policy

N/Native/PorticoPlayer.m:122 sets 15-second budgets, :163 bounds responses; S/packages/client-core/src/route-connection.ts:154 retries reads only. Core index.ts:338 general requests lack one operation-class policy; browser/native/Vega defaults differ.

### F-22.3 — Consistent Retry-After handling

S/packages/client-core/src/index.ts:134 parses seconds/date and :405 uses it; playback-v1/http.ts:42 accepts numeric only; console-transport.ts:18 error shape loses retry floor; server-connections.ts:262 wraps refresh errors. This is consumer repair, not implementation of another Retry-After helper. Playback HTTP-date handling, console error fields and wrapped renewal errors are separate loss points; carrying a delay in one error type does not prove all callers honor it.

### F-22.4 — Client control-request reservation

N/Native/PorticoPlayer.m:136 admits FIFO and :149 takes first waiting request; S/server/internal/httpapi/route_lanes.go:394 reserves server auth lane. Missing client priority/control reservation.

### F-22.5 — Return-to-LAN route optimization

S/packages/client-core/src/route-connection.ts:95 reprobes on network change; :86 skips a ready foreground connection; :113 puts last-good before LAN. No healthy-route foreground optimization or seamless replacement owner found. Network-change re-evaluation already exists. Foreground-only re-evaluation skips a ready connection and last-good priority can keep a remote route preferred. The proposal must preserve a healthy stream until a replacement proves identity; a simple periodic Hosted probe would contradict the economy requirement.

### F-22.7 — VPN and overlay route support

S/server/internal/networking/remote_manager.go:643 publishes VPN/private-proxy manual routes; S/packages/client-core/src/route-identity.ts:42 excludes 100.64/10 and overlay names from HTTP-private classification. HTTPS overlay works; explicit HTTP overlay policy absent.

### F-22.8 — Forgiving common-port connection input

N/src/app/direct-connect.ts:21 guesses HTTP/HTTPS only; no 32500 insertion/bounded port race. Existing explicit host:port works and identity probe follows.

### F-22.9 — Release compatibility negotiation

S/server/internal/httpapi/router.go:816 returns public version/build; :751 registers identity proof. No required/optional-feature negotiation or supported release window; PC/scope-completion.md:15 explicitly requires compatibility outcomes.

### F-22.10 — Connection-cause diagnostics

S/packages/client-core/src/route-connection.ts:3 reports phase/discovery/identity mismatch; N/src/bridge/server-connections.ts:109 exposes local-permission denial. DNS/TLS/connect/slow/captive distinctions and dedicated view absent.

### F-22.12 — Offline navigation snapshots

N/src/screens/auth/OfflineDownloads.tsx:1 and bridge/offline-downloads.ts retain download identity/art/progress; no durable last-known Home/catalogue navigation snapshots found.

### F-22.13 — Durable final online playback facts

S/packages/client-core/src/playback-v1/legacy-bridge.ts:31 stores report queue in memory; :175 enqueues exact reports. Core index.ts:1640 persists cleanup/checkpoint journal; N/src/bridge/offline-downloads.ts:553 persists offline progress. Missing durable exact final/marker facts for ordinary UI v1 reports; do not call all playback journaling absent. The exact JS v1 report queue is the missing crash-durability boundary. Existing cleanup/checkpoint journals and native listening ownership should be retained. This review does not claim every native engine has the same fact-loss behavior or that cleanup journals preserve the exact sequence and marker facts.

### F-22.14 — Unified native event transport

N/src/bridge/useNativePlayback.ts:100 owns EventsClient with waitSeconds=10; this is server long-poll, not periodic empty polling. N/src/bridge/event-stream.ts:26 separately opens raw URL XHR for other feeds; no unified routed refresh/subscription owner. EventsClient is a request held by the server for up to ten seconds, with cursor/reconnect semantics; it is not an arbitrary ten-second polling timer. Separate Together/receiver XHR adapters open their supplied URL directly. One routed authenticated owner must cover those paths, including cancellation, refresh, byte bounds and subscription sharing.

### F-22.15 — Selected-server overload coordination

S/packages/client-core/src/hosted-gate.ts:31 shares Hosted failure floors; playback-v1/legacy-bridge.ts:210 honors per-report delay. No analogous selected-server coordinated gate across reads/features found.

### F-22.16 — Captive-portal and split-DNS recovery

S/packages/client-core/src/route-identity.ts:96 probes API and signed nonce; route-connection.ts:167 detects non-JSON gateway failure. No portal/HTML/split-horizon diagnostic or repair view found.

### F-22.17 — Retained multi-server credentials

S/packages/client-core/src/known-servers.ts:3 explicitly stores no credentials; server-connections.ts:17 storage reads one selected record. Remembered address switcher exists; independent protected server families absent.

### F-22.18 — Subnet-specific published routes

S/server/internal/networking/remote_manager.go:501 publishes manual routes; no requester-subnet URL mapping found in networking handlers/settings/OpenAPI. Existing owner access URLs apply more broadly.

### F-22.20 — Server-identity pairing QR

S/packages/client-core/src/setup-qr.ts:5 draws verification URL QR; N/src/bridge/setup-codes.ts binds activation. No distinct pin/fingerprint/routes QR encoder/parser/scanner found.

### F-22.21 — Redacted route and retry diagnostics

S/packages/client-core/src/route-connection.ts:3 exposes phase/revision/network/discovery; N/src/screens/server/Troubleshooting.tsx:141 exports server support report. No queued-vs-network timing/retry-floor/last-proof export found.

### F-23.2 — Portable library and personal-state export

S/server/internal/backup/backup.go:1 preserves own database/files; httpapi/console.go:320 exports diagnostics; PA/Backlog.md:38 records old CSV history route. No product-neutral facts/art/list/profile bundle.

### F-23.5 — Targeted change-notification scans

S/server/internal/httpapi/router.go:995 queues library scan with empty source/path selector at :1001; source scans/inventory exist. Missing file/subtree completion webhook and path translation/idempotent receipts.

### F-23.9 — Supported external API and SDK

PC/scope-completion.md:15 requires independently usable API semantics. Own JSON/YAML contracts exist; no Infuse reference adapter/vendor integration found. The product already supplies a documented public API foundation. The current contract requires an independently usable boundary but does not promise that Infuse or any vendor will adopt Portico. A reference external client is the reviewable deliverable; vendor coordination and acceptance remain separate.

### F-23.12 — Broad direct-play and subtitle engines

N/Native/PorticoPlayer.m:24 attaches AVPlayer; N/android/app source uses Media3; server subtitle burn-in/rendering exists (S/server/internal/decoder/burn_in.go:35). No mpv-class cross-platform ASS/PGS direct renderer established.

### F-23.13 — Adaptive quality and suggestions

S/packages/client-core/src/presentation/settings-structure.ts:593 maps requested limits; native PlayerOptions.tsx:171 changes quality. No bandwidth estimator/adaptive ladder/suggestion owner found. PA/Specs/Spec — Playback Protocol v1.md:152 forbids server autonomous lowering, permits explicit client Auto. The Playback Protocol v1 spec expressly permits client Auto to choose/update a viewer-requested limit, while prohibiting the server from autonomously lowering quality. Thus all adaptive behavior is not categorically excluded. A bandwidth estimator, consent semantics and correct replanning still need implementation; suggestions are a narrower scope.

### F-23.14 — Automatic subtitle alignment

N/src/player/PlayerOptions.tsx:267 adjusts source-relative timing; S/server/internal/httpapi/subtitle_routes.go owns choices. No audio/reference alignment/drift analysis found.

### F-23.15 — Repeated-intro detection and marker import

S/server/internal/mediaanalysis/detectors.go:24 derives chapter markers and :75 low-confidence luminance intro; httpapi/media_analysis.go:105 edits markers. No repeated-season audio matcher/trusted import workflow found. Marker editing/import-like manual data is not repeated-intro detection. Current chapter and luminance results are deliberately unapproved hypotheses; trusted imported intervals need source identity, provenance and review rather than automatic promotion of ambiguous candidates.

### F-23.16 — Credits scene-aware detection

S/server/internal/mediaanalysis/detectors.go:98 extends a credits hypothesis to duration; marker/skip UI exists. No explicit mid/post-credit scene boundaries/classifier found.

### F-23.17 — Commercial detection and review

S/server/internal/mediaanalysis/detectors.go:39 recognizes chapter commercial labels and :85 paired black boundaries; httpapi/media_analysis.go:105 edits markers. No Comskip/specialist recording review surface found.

### F-23.18 — Standing media optimization rules

S/packages/client-core/src/downloads-service.ts:1 supports preparation service; no persistent on-add/query/scheduled conversion rule engine found. Next-episode download intents are narrower.

### F-23.19 — Rolling offline episode rules

N/src/bridge/offline-downloads.ts:359 persists/refills next episode, :565 removes watched; Downloads.tsx:175 exposes Wi-Fi/storage controls. Missing recurring per-show keep-N/charging/owned-file eviction policy. The native download lane supplies the recurring next-episode behavior the first pass did not trace. Keep-N ownership, charging policy and richer refill/eviction reconciliation are still legitimate gaps. Server next-N selection and the device’s persistent next-after intents should be reused.

### F-23.20 — Disc structures and extras

S/packages/client-core/src/detail.ts:8 defines extras and :37 edition facts; no ISO/BDMV/VIDEO_TS catalogue/menu adapter found. File extras exist; disc navigation does not.

### F-23.25 — Sample-rate matching and output diagnostics

S/server/internal/playbackv1/transitions.go:496 publishes source/render sample rate; N/src/player/PlayerOptions.tsx:312 shows playback-info rows. No complete actual-device mixer/DAC-rate matching or bit-perfect path established.

### F-23.26 — Headless music receiver

S/packages/client-core/src/remote-playback.ts:121 implements authorized receiver lifecycle. No supported headless audio appliance/service/package found.

### F-23.29 — Classical music graph and credits

Music metadata/tag/provider plumbing exists (S/server/internal/metadata/musicbrainz.go:113), but no composition/performance/conductor model/browse graph found.

### F-23.30 — Music editorial and metadata presentation

S/server/internal/metadata/musicbrainz.go:113 enriches releases; PA/Backlog.md:21 records related-artist work and :35 unrendered label/albumType/country/years. No complete editorial service/linked credits display.

### F-23.31 — Personal listening recap

S/server/internal/catalog/play_history.go:1 persists observed plays; httpapi/play_history.go:47 summarizes owner history. No personal recap/streak/exploration report surface found.

### F-23.32 — Music-aware transitions and visualizations

S/server/internal/operations/preferences.go:195 stores crossfade; media-analysis supports waveform jobs. No music-envelope/album-aware Sweet-Fades strategy found; do not call all transition/visualization plumbing absent.

### F-23.33 — Audiobook metadata enrichment

S/server/internal/metadata/library_agent.go:114 advertises books with MusicBrainz/CAA; :235 enables provider policy. Worker musicbrainz.go:113 is album/recording-shaped, not a reviewed narrator/book-series matcher. PA/Backlog.md:24 records archived unreviewed work; PC/scope-completion.md:7 explicitly defers remote audiobook enrichment. A current agent descriptor and provider-enable policy do exist for audiobook libraries. The inspected enrichment worker handles release/recording-shaped music work; that is not proof of an edition/narrator/series-aware book provider. The backlog’s archived work is not active code, and the contract explicitly defers remote audiobook enrichment.

### F-23.34 — Audiobook chapter editing and bookmarks

S/server/internal/httpapi/media_analysis.go:105 supports generic chapter marker edits; existing book chapters play. No private position+note bookmark route/UI found; PA/Specs/Spec — Playback Protocol v1.md:223 specifies it.

### F-23.42 — Library Channel IPTV export

S/server/internal/httpapi/library_channels.go:165 serves own protocol; livechannels/library/guide.go:105 publishes guide/logo facts. No outward M3U/XMLTV/virtual-tuner route found; HLS playback manifests are not channel-list export.

### F-23.43 — Advanced station scheduling and graphics

S/server/internal/livechannels/library/model.go:178 has timed blocks/priority/fallback/overrun and :195 overlay config; playbackruntime/linear.go:890 issues scoped logo URL. Missing dedicated filler/bumper/advanced station-clock sequences; rendering acceptance unestablished. The station model already has weekday/time blocks, priorities, fallback rules, cursor/block-start anchoring and finish/cut/fit overrun. Logo visibility is checked separately before delivery. Filler/bumper sequencing and actual on-screen graphic parity still require work/acceptance; existence of overlay fields alone does not prove every renderer consumes them.

### F-23.44 — External curation rule adapters

S/server/internal/catalog/resources.go:514 stores validated saved-view queries; static collections exist. No Kometa/rule-import/external-list adapter found.

### F-23.46 — Shared smart collections

S/server/internal/catalog/resources.go:514 restricts queries to views, :533 restricts visibility to collections. Live saved views already exist; shared curated query entity/art/publishing model missing. The server intentionally splits query-backed views from collections carrying shared visibility. A shared smart collection therefore needs reconciled identity, sharing, artwork and personal-predicate semantics rather than an additional query engine. Existing live browse definitions substantially reduce the query implementation cost.

### F-23.47 — History query and export depth

S/server/internal/httpapi/play_history.go:18 serves history and :47 summaries; PA/Backlog.md:52 lacks title search/per-row delete/who-watched and :38 current CSV integration. No Tautulli/Jellystat adapter found.

### F-23.48 — External conversion-worker integration

Prepared artifacts exist; PA/Backlog.md:30 reports in-place replacement facts not reread. No Tdarr/Unmanic completion/replacement adapter found; read-only source rule does not authorize Portico rewriting originals.

### F-23.51 — Guided remote-access diagnosis

S/packages/client-core/src/remote-access.ts:30 distinguishes CGNAT/proxy/IPv6/mapping failures and :28 has owner-triggered check; native/web settings expose networking. Missing comprehensive DNS/hairpin/VPN/client cause diagnosis and acceptance.

### F-23.53 — Expanded platform coverage and acceptance

Apple/Android/Fire/Vega clients exist; PC/scope-completion.md:37 retains later Tizen/webOS qualification. No qualified Vision/console/vendor-TV release established; Roku excluded from this review.

### F-23.54 — Non-English localization

PA/06 Clients.md:19 states English launch/more planned; S/packages/i18n/src/catalog/regional.ts:1 derives en-CA/en-GB. No other translated catalogue/RTL acceptance found.

### F-23.56 — Themes and authored-layout customization

Authored Home arrangement/appearance already exist (S/packages/client-core/src/presentation/settings-structure.ts:241). No skin/layout engine found; PC/visual/contract.md:350 keeps fixed structure.

### F-23.57 — External availability and absent-title discovery

Local Home/Discover recommendations exist; no universal regional availability/absent-title catalogue adapter found. Metadata providers enrich identified library media, not streaming availability.

### F-23.58 — Public social graph and reviews

S/packages/client-core/src/social-playback.ts:1 provides Together; PC/sharing-and-social-playback/contract.md:24 excludes public social profiles from current contract scope. No public lists/reviews/discussion/cross-server graph found.

### F-23.60 — Edition and version grouping

S/server/internal/httpapi/inventory.go:279 calls AssociateInventoryVersion; detail.ts:37 carries edition and sources. PA/Backlog.md:29 reports duplicate files instead of versions. Automatic grouping and complete preplay/cut timeline workflow missing. An owner can explicitly associate an inventory asset as another version, and edition labels can reach detail facts. That disproves complete absence of grouping infrastructure. The known same-title ingestion behavior and deferred native Version control leave automatic grouping and consistent selection genuinely unfinished.

### F-23.61 — First-party feature and device qualification

PA/Backlog.md:62 lists native/history/recording omissions, :70 dropped-frame/buffer gap, :82 Cast acceptance. Existing feature modules/UI are substantial; full reachable parity and physical fleet evidence absent.

### F-23.62 — Supported external API and SDK

S/server/api/README.md:1 and JSON/YAML contracts document own API; PC/scope-completion.md:15 requires independent semantics. No stable release-window policy/published reference adapter/SDK release established.

### F-23.64 — Optional video enhancement filters

S/server/internal/decoder/hardware.go:522 has deinterlace/scale paths; no named client AI upscaler found. Existing scaling must not be described as absent enhancement.

### F-23.65 — Cinema pre-roll and extras presentation

S/packages/client-core/src/detail.ts:8 defines trailer/featurette/behind-the-scenes and :129 parses extras. No automatic cinema/pre-roll queue owner or external trailer supplier found.

### F-24.2 — Edition and version grouping

S/server/internal/httpapi/inventory.go:279 associates physical object to logical item; metadata_repair.go:91 previews merge; N/src/screens/title/PlaybackChoices.tsx:13 defers Version. Automatic grouping/cut timelines/full review UI incomplete; PA/Backlog.md:29 already records it.

### F-24.3 — Expanded platform coverage and acceptance

Existing tvOS/Android TV/Fire/Vega sources and debug builds are real; PC/scope-completion.md:37 requires individual acceptance. Source cannot establish store listings or actual physical results.

### F-24.6 — Smart playlists

S/server/internal/catalog/resources.go:514 persists typed query views but rejects collection query definitions; native SavedPages.tsx:183 can present/play views. Explicit playlist occurrences exist; persistent dynamic-query playlist entity missing.

### F-24.7 — Rolling offline episode rules

N/src/bridge/offline-downloads.ts:359 persists/refills next, :565 deletes watched, :553 journals progress; Downloads.tsx:175 exposes policy. Keep-N show rules/charging and physical termination/travel acceptance remain.

### F-24.8 — Music radio and instant mixes

Shared gain/gapless/crossfade/queue infrastructure exists; PA/Backlog.md:17 plans radio/mixes with similarity undecided; PC/scope-completion.md:7 retains instant-mix obligation. No radio generator/sonic engine found.

### F-24.9 — Per-person resource policy depth

S/server/internal/access/limits.go:96 stores MaxStreams/schedule/bitrate/content; enforcement.go:105 counts streams; playbackv1/admission_gate.go:5 serializes starts. S/web/src/screens/server/People.tsx:64 and N/src/screens/server/PeoplePanels.tsx:76 expose limits. Missing finer conversion/download/DVR budgets and acceptance, not basic account policies. Basic limits already reach server enforcement and both web/native account forms. The start gate prevents two overlapping starts from both observing the previous stream count. New conversion-only, download or DVR resource budgets are optional policy extensions; device/race acceptance is verification work, not a reason to build another MaxStreams limiter.

### F-24.10 — Subtitle control parity

N/src/player/PlayerOptions.tsx:267 offers source-relative ±0.5s; S/packages/client-core/src/presentation/settings-structure.ts:206 gates appearance to web. Native PorticoPlayer.m:54 hardcodes overlay font/layout; dual tracks absent.

### F-24.13 — Fuzzy and alternative-title search

S/server/internal/catalog/search.go:146 uses FTS exact/prefix boosts, :279 echoes token_prefix. Provider original titles/synonyms exist upstream but no alias/typo candidate index used on search path found.

### F-24.14 — Household profiles and OS user mapping

N/src/screens/auth/DirectProfiles.tsx:81 renders PIN selector; Profiles.tsx:3 manages PIN recovery; session.tsx:791 chooses profiles. Missing OS multiuser mapping/optional launch prompt and actual TV-role acceptance; profiles/PIN are not absent. Profile PIN entry and switching are real reachable flows, not merely catalogue strings. OS-user association and a configurable always-show-profile-on-launch policy remain absent in the reviewed implementation. Physical TV role/focus and offline separation cannot be accepted from source review alone.

### F-24.15 — Viewer folder and home-video browsing

S/server/internal/catalog/inventory.go:204 preserves storage identity; httpapi/inventory.go:264 exposes owner operations. No viewer opaque folder hierarchy/home-video/photo library route found.

### F-24.19 — Merged multi-server browsing

S/packages/client-core/src/known-servers.ts:3 retains address list without independent credentials; server-connections.ts:17 one selected record. No aggregate search fan-out/partial results found.

### F-24.20 — Non-English localization

S/packages/i18n/src/catalog/regional.ts:1 derives English variants; PA/06 Clients.md:19 explicitly plans more languages after English launch. No non-English catalogue/RTL fleet acceptance found.

### F-24.21 — Guided remote-access diagnosis

S/server/internal/httpapi/router.go:816 publishes setup/build status; library scan jobs :1008 report progress; S/packages/client-core/src/playback-messages.ts:12 has source/conversion reasons; remote-access.ts:30 gives NAT guidance. Missing single explainable stage/cause UX and device acceptance.

### F-24.23 — Themes and authored-layout customization

S/packages/client-core/src/presentation/settings-structure.ts:241 has appearance/Home choices; N/src/bridge/NativeActivity.ts:34 Apple intents exist. No custom CSS/skin/automation/theme-music general engine found.

### F-24.24 — Supported external API and SDK

S/server/api/README.md:47 allows pre-release changes; JSON plus authored YAML, core routes and types exist. PC/scope-completion.md:15 requires independent supported API; SDK releases/semantic examples/stable policy/portable export remain incomplete.

### F-24.25 — Repeated-intro detection and marker import

S/server/internal/mediaanalysis/detectors.go:24 uses chapter labels, :75 intro luminance and :98 credits end-region. Marker/skip UI exists; repeated-season audio detector/scene-aware credit boundaries missing.

### F-25.1 — Release compatibility negotiation

S/server/internal/httpapi/router.go:816 returns version/build; S/server/api/README.md:47 explicitly permits pre-release breaks. PC/scope-completion.md:15 requires compatibility outcomes; no release-era minimum/window/deprecation/downgrade policy found.

### F-25.2 — Release qualification ledger

S/.github/workflows/release.yml:59 reruns CI and scripts/build-server-release.py:158 seals source/artifact identities; PC/conformance.md:3 requires exact build/review/behavior/client evidence. No machine-enforced all-platform ledger/exceptions/final evidence binding found.

### F-25.3 — Pinned cross-repository source tuple

RN/scripts/shared-packages.mjs:38 hashes selected copies; native CI :24 checks server default branch without ref. S/scripts/build-server-release.py:128 snapshots dependencies; no server/native/Hosted source tuple pin/version check found.

### F-25.4 — Release media and scale gates

S/.github/workflows/ffmpeg-toolchain.yml:56 runs full bundle proof and :70 two media tests; S/scripts/build-server-release.py:148 race-tests server. S/server/internal/testtier/testtier.go:11 skips media by default; no all/performance tier release gate found. PA/Backlog.md:57 records full-suite timeout.

### F-25.5 — Native platform CI qualification

RN/.github/workflows/ci.yml:50 assembles Android debug; no Apple xcodebuild/native-test/Vega lane or Android release-mode packaging lane found. Existing JS/core/type checks are real.

### F-25.7 — Signed final-artifact inventory

S/scripts/build-server-release.py:158 signs binary/source/toolchain digest; package-release.sh:23 adds web after sealing; package-macos.sh:31 changes binaries with codesign. Release workflow :215 emits checksums/attestations, but no independently trusted complete final installer/web/service signature inventory found. The signed manifest authenticates a sealed server binary and toolchain digest. Packaging subsequently adds web and notices, and macOS codesign can alter binary bytes. A final complete inventory must authenticate those final outputs and be verified by an independent pinned trust root; a checksum list or same-feed key is a different assurance.

### F-25.8 — Protected official release signer

S/.github/workflows/release.yml:55 requires secret key before release; build-server-release.py:35 still generates a fallback key when absent. server/api/release-signing.md:3–9 requires isolation/review/rotation and admits unfinished enforcement. No protected separate signer/approved key identity gate found. The public release workflow already fails if its signing secret is empty. The uncovered boundary is an approved protected signer identity/job: the generic sealing script can generate a key, and the custody document explicitly says enforcement is unfinished. No assertion is made about actual secret custody or repository environment protection outside these files.

### F-25.9 — Unified update-feed contract

S/scripts/write-public-update-manifest.mjs:23 writes flat schema/channel/version; server/internal/administration/maintenance.go:491 expects channels map and :526 refuses incompatible shape. Update notice/check path exists; generator-to-consumer schema/channel fixtures absent.

### F-25.10 — Upgrade and breaking-change notes

S/.github/workflows/release.yml:228 generates release notes and :230 publishes generic preview guidance. No required curated platform/schema/client/rollback note fields or check found.

### F-25.12 — Dependency security qualification

S/.github/workflows/security.yml:18 runs gitleaks/CodeQL; Dependabot configuration exists. No govulncheck/reachable-vulnerability/dependency-review/native-sanitizer gate found in workflows/scripts.

### F-25.13 — Scheduled parser fuzzing

S/server/internal/httpapi/fuzz_test.go:34–156 defines five fuzz targets with seeds. No scheduled -fuzz campaign/minimized-corpus ownership native parser lane found. Ordinary go test runs seeds only.

### F-25.14 — Rendered UI and end-to-end journeys

S/web/test/helpers/component-harness.mjs:4 explicitly models callbacks, not render/focus/scheduling; web/test/integration/setup-flow.mjs:63 exercises API setup. No browser/RN rendered journey gate found in package/workflow searches.

### F-25.16 — Architecture ownership and dependency map

PC/architecture.md:25 assigns domain/table ownership and :9 bounded adapters; Go internal/package boundaries exist. No maintained actual import/callback/state-lifetime graph/forbidden-edge AST gate found in scripts/CI.

### F-25.17 — Test duration and flakiness reporting

S/scripts/runner-test.sh:10 supports -json, :43 separate performance tier, :219 timeout; PA/Backlog.md:57–59 records expensive/full-suite costs. No maintained duration/skip/retry dashboard or flake ownership gate found.

### F-25.18 — Release SBOM and notice completeness

S/.github/workflows/release.yml:181 enables container SBOM and :218 scans release artifacts; S/scripts/verify-ffmpeg-manifest.py:12 checks licence presence; package-release.sh:24 includes notices. No unpacked final Go/JS/native/font/FFmpeg completeness reconciliation found. SBOM/provenance actions and FFmpeg licence checks are already wired. The remaining requirement is to reconcile the unpacked delivered contents with Go, JS, native, media, font/icon and licence inventories. No SBOM was generated or inspected during this audit, so completeness cannot be asserted.

### F-25.19 — Third-party patch maintenance register

S/server/internal/thirdparty/sqlite/PORTICO.md:3 pins upstream/patch rationale/upgrade checks; third_party/hls.js/README.md:3 pins source/provenance; third_party/ffmpeg/sources.lock.json:15 pins recipes/patches. No unified owner/retirement/upgrade register across components found. SQLite, hls.js and FFmpeg each already record substantial upstream and patch provenance. The missing unified register is a maintenance coordination feature: who owns upgrades, which targets are qualified, and when a patch can retire. It should reference the existing per-component records instead of duplicating them.

### F-25.20 — Non-English localization

S/packages/i18n/src/index.ts:16/69 provides catalogues/fallback; PA/06 Clients.md:19 plans more languages. RN/CONTRIBUTING.md:17 rejects external PRs; no community translation import/review/licence channel found.

### F-25.22 — Release-bound documentation verification

Site/scripts/check-build.mjs:8 checks required pages and :69 internal links/build identity; S/server/api/README.md:47 still has pre-release guidance and PA/Backlog.md is dated. No current artifact/platform/API/install-command applicability gate found. The site checker already verifies generated pages, local links, assets and its commit identity. It does not execute documented installation commands or bind each download/API/platform claim to a qualified release. The improvement is release applicability, not adding the first documentation checker.

### F-25.24 — Funding and community support routes

S/CONTRIBUTING.md:7 welcomes bug/feature/compatibility/docs issues and :22 directs private security reports; Site/src/pages/developers.astro:21 links issue guidance. No voluntary donations/sponsorship/maintenance-cost/community-moderation plan found.

### F-25.25 — Supported external API and SDK

S/packages/contracts and server/api JSON/YAML contain generated/authored contracts; server/api/README.md:18 promises independent third-party endpoints. PC/scope-completion.md:15 requires public semantic fixtures; no released standalone SDK/reference authenticated adapter found.

## Evidence boundary

This was a read-only review of the active working trees on 11 October 2026. Only this output file was created/edited. No Portico builds, test suites, formatter, installer, server, browser, MCP tool, subagent or endpoint was run; no `.private` data, demo server or Hosted endpoint was accessed. No throw-away experiment was necessary. Existing tests and CI scripts were read as source, not executed or counted as passing results.

For every entry I read its full feature text, searched the complete F-index for matching/overlapping capabilities, and searched the relevant server handlers/route wiring, authored OpenAPI YAML plus generated `server/api/portico.openapi.json`, client-core (including settings-structure/preferences), web, native JS/Apple/Android adapters, Vega bridge sources, and i18n catalogue. Plans were checked in Backlog, architecture/Specs and Product Contracts. Searches used capability names and implementation vocabulary: e.g. spoiler/hideSummary/maskSummary; SDH/description/track IDs; keep-N/autoNext/deleteWatched/nextAfter/storage; webhook/outbox/Seerr/Sonarr/Radarr; Trakt/Simkl/ListenBrainz/Last.fm/scrobble/Subsonic; RSS/podcast/enclosure/OPDS/EPUB/comic/photo/slideshow; GameController/gamepad/keycodes/gestures; deadlines/Retry-After/FIFO/route pin/network/credential; release/signing/provenance/SBOM/fuzz/conformance/compatibility. Word-boundary ecosystem searches removed false positives such as `closeErr`, RSS-as-memory, `republish`, and unrelated rendering overlays. Negative results establish no implementation found in those active scopes, not a mathematical proof that no differently named or external adapter can exist.

For the native-relevant core seams cited here, I independently compared canonical and copied source bytes for preferences, settings structure, legacy v1 reports, route connection, known/remembered servers, v1 track types, remote playback and saved state; those inspected copies were identical. This was not execution of the copy-checking script or a comprehensive cross-repository build proof. Test-only fixtures, generic kind tokens, unused preference storage, comments and API declarations were not credited as delivered UI. Where the implementation boundary remained conditional, the row says so.

Presence means an implementation path exists, not runtime acceptance. No network/DNS/TLS/OS-background behavior, physical playback/output/focus/accessibility, vendor integration, store listing, performance/scale target, branch protection, signing-secret custody, release artifact or actual SBOM completeness was independently established. Physical/release work could have evidence outside the reviewed trees; absence of a complete applicable ledger here is not a claim that nobody ever tested a device. Competitor features and community demand were taken as the entries’ comparison premise; I did not re-verify current public vendor claims or licensing terms. F-23.12/F-23.64 are conditional enhancement proposals, not demonstrated comparative image/format superiority. F-25.21 records a missing distribution policy, not legal advice or a determination of infringement.

Current contracts can override older architecture/backlog wording, and recorded deferrals do not authorize deleting their retained capability. This matters especially for DLNA (scoped authenticated contract versus old whole-server backlog), audiobook enrichment (explicit remote deferral), English launch versus later localization, and future paid relay. F-23.59 is excluded by the current free/non-commercial product model, not a claim that Justin can never change that model. Bundled entries with a real unfinished constituent stay Partly present even when a subclaim is refuted. Estimates concern the remaining consistent delivery, and should be split by missing feature versus acceptance before scheduling. Roku source/runtime was not audited; its presence or absence is not inferred. Coverage is complete: no F entry in documents 21–25 was skipped.
