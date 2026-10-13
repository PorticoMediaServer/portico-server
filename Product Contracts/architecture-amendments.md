# Domain architecture and failure boundaries

Revision: portico.october.1. The [dated decisions](decisions.md) govern. The family contracts retain required product behaviour; the implementation evidence below does not declare all requirements complete. September architecture assumptions are superseded where the October decisions or verified current code differ.

## Identity and local authority

Hosted owns global account sign-in and profile identity. The selected server owns membership, media access and personal state. Account-only work needs no invented server/profile. Stable owner identity is separate from permission revisions and token renewal.

Local sessions use bearer access/rotating refresh credentials and installation records, not possession keys. TV approval carries the approver role. Cast pairing issues separate playback-only member authority. Remote admission uses home networks and account policy on every creation path. Learned terminal Hosted signer revocation retires only affected admitted descendants. Direct/recovery families remain independent.

The owner sees permanent play history even when a viewer pauses/clears personal history. Member deletion retains plays without names. Restricted-profile device powers, failed-attempt retention, country/per-library rating policy and some legacy authority branches remain unfinished. No consumer push or email is approved; residual Hosted mail flows require reconciliation. Sources: [accounts/security](../../_program/reports/accounts-security.md), [review](../../_program/reports/review-accounts-security.md).

## Networking and Hosted coordination

Plain HTTP is accepted from any address. Home networks are the complete locality rule; trusted proxies only supply validated request evidence. TLS is encouraged, never forced. Owner certificates and own-hostname proxies are supported. Hosted publishes TLS candidates only. IPv4/IPv6/overlays are supported designs; CGNAT is not blocked. No sub-path or relay is built.

Current membership is local-server authority, pushed to Hosted's discovery index. There is no seven-day Hosted membership-policy lease, daily policy renewal or thirty-minute presence dependency in the current server. `internal/hosted/current_schedule.go` uses event-driven work and a jittered check-in supplied by Hosted: weekly when reachable by wakes, daily otherwise. Network observations alone do not force a Hosted request. Check-in carries member sequence/digest, departures and an optional root-signed signing certificate. This corrects historic P05 and the older networking contract; it is implementation evidence, not a new owner timing decision.

Route location, pinned server identity, credential validity and membership remain separate facts. Nonce/context/signature proof cannot make HTTP confidential. Quiet transport failure preserves credentials. Shared outage floors and Retry-After must outlive shorter protocol polling caps. Physical proxy/IPv6/browser discovery/fleet acceptance remains unverified. Sources: [networking fixes](../../_program/reports/review-networking.md), [Hosted](../../_program/reports/hosted.md), [Hosted scale](../Project%20Architecture/Specs/Spec%20%E2%80%94%20Hosted%20at%20Scale.md).

## Catalogue, inventory and metadata

Titles own identity/state; files own byte/revision/availability evidence. Versions/editions/parts attach where identity is established. Healthy complete or verified reported absence marks files missing immediately; whole-root failure and partial coverage protect affected prior facts. Retain a no-copy title under immediately/days/never policy. Disk deletion is confirmed physical deletion, not a recoverable state-folder copy.

Folder/file/embedded evidence carries provenance. Usable file evidence wins conflicts; tolerant confidence matching handles yearless/minor-misspelled names. Reject the smallest bad unit while retaining hostile-input bounds and custody. File List Only allows bounded ignore configuration, not ordinary media/sidecar reads. Watchers/limited reports and periodic fallback detect changes; native/network-share acceptance is pending.

One provider per library, TMDB default for all video, local metadata only available. No provider chain or Hosted broker. Manual edits/locks, candidate reasons and publication fences stay. Needs attention covers the existing film/TV/anime queue with exact totals and later MCP actions; music is separate. Current automatic screen matching requires verified details, no contradiction, at least one strong signal, confidence at least 0.78 and a 0.07 margin from incompatible candidates. These are current source constants, not statistical calibration or a new product tuning setting.

Provider merges preserve supported personal state; there is no absorbed-ID redirect. Large show merges remain one transaction. Full closed-reference continuity, untagged audio identity and album/artist/book merging are unfinished. Download reference moves have later fixes. Selected artwork is retained; local usage is distinct from provider-fetched art. Sources: [identity/review](../../_program/reports/review-identity.md), [scan loop](../../_program/reports/scan-loop.md), [providers](../../_program/reports/providers-matching.md), [follow-ups](../../_program/reports/server-followups.md).

## Playback and conversion

One capability document combines engine/display/audio facts, Player setting and overrides. Automatic engine prefers native original/plain remux and uses mpv where it avoids native video conversion, lossy audio or subtitle burn-in. Explicit choices and real supported limits remain. Original is the quality default everywhere; optional Automatic starts at Original and adapts down/up to the connection. Saved quality is a starting choice, owner/member policy a ceiling.

Ordinary planning persists rich conversion targets: compatible HEVC/HDR/multichannel, fMP4 as needed and tone mapping for SDR. Prepared recipes remain H.264 SDR/AAC; live remains TS. Device subtitles include styled/image originals where renderers exist, with burn fallback. Native mpv, web workers, glyph/bitmap/HDR/PiP/Cast fidelity and hardware paths need acceptance. Per-engine display/rate profiles, live two-engine/fMP4 and rich offline recipes remain unfinished. Sources: [playback](../../_program/reports/playback-core.md), [planner](../../_program/reports/planner-wiring.md), [subtitles](../../_program/reports/subtitles-text.md), [clients](../Project%20Architecture/06%20Clients.md).

## Freshness and screens

Browse retains previous relational generations; search builds independent FTS slots. First builds serve committed content. Failed titles have durable retry evidence without stopping healthy work. Protected unpublished restriction classes return empty content rather than private stale data. Ordering, directory, personal command and progress counters are separate. Continuations recover quietly where possible.

One scoped event feed asks visible dependencies to reread. Clients preserve content/focus, coalesce hints, idle briefly and delay transient errors. Covered/background screens do not poll indiscriminately. Appended lists, special screens, browser stream sharing and focus remain reported gaps. The main library sort/filter vocabulary must reach every subgroup; grouping implementation is in progress. Sources: [server freshness](../../_program/reports/freshness-server.md), [client freshness](../../_program/reports/freshness-clients.md), [grouping](../../_program/reports/grouping-sorts.md).

## Personal state and downloads

Integer 0–10 ratings map to half-stars by division by two; unset is distinct from zero. Container marks clear all current members' progress and fence earlier playback, with thirty-second conditional Undo. Owner history is distinct from personal history; explicit watched commands fence stale offline observations.

Claims and receipts have separate lifetimes. Source/title/cache retirement does not revoke valid device bytes. Bundles keep title/artwork/subtitle facts locally. Receipt expiry blocks without deleting; access revocation still fails closed. Ordinary sign-out retains protected copies unless removal is explicit. Offline ordering follows event sequence/observation time mapped to intent order, not maximum position. Styled/image originals/fonts, player extras, offline next episode, clock rollback and some background transfer paths remain unfinished. Sources: [personal state](../../_program/reports/personal-state.md), [downloads](../../_program/reports/downloads.md), [review](../../_program/reports/review-downloads.md).

## Live TV, DVR and social

One upstream channel input serves private viewers/conversions and recording leases. Same-airing recordings share payload storage with profile-private ownership/deletion. Active leader cancellation, guide extensions during capture, fMP4 live, Windows custody and real tuner/load acceptance remain incomplete.

Watch Together has separate command/readiness/presence/queue/governance revisions and a five-second readiness hold after seek/load. Keep the host-only, Start anyway, no-global-buffer-pause and 0.75/3-second correction rules. Cast/handoff retains destination control independently of the phone. Paused initial frame, remote session events and stable native receiver proof keys remain client integration gaps. Historical direct-LAN possession-key protocols are not current implemented sessions. Sources: [Live TV](../../_program/reports/livetv.md), [social](../../_program/reports/social.md), [native sync](../../_program/reports/native-sync.md).

## Settings, operations and release

Settings declare owner, default, bounds, effective state and application mode. Wire useful stored controls. Gentle/Normal/Fast is shared pacing; explicit owner pause lasts one hour. Backup timing does not confine all background jobs. Some worker pause boundaries remain unfinished.

State/backups remain plain. Restore staging is detached/pollable, with safety/rollback copies and replayable session scrubbing. Ordinary sign-ins end; API credentials in the backup return, including later-revoked keys. No universal external security-incarnation guarantee is certified. Minimal startup recovery exists; full recovery console is open. Windows SCM, macOS system daemon, Linux and Docker packaging have source/fixture/cross-compile evidence but need real lifecycle qualification. Release signing needs the independently approved public pin and actual artifact/source distribution.

Later console passes expose version deletion, attention totals, storage/FFmpeg status, reports, Restart/restore progress and remote controls. Native broad administration is web-first. Consumer parity, catalogue strings, SDK/device builds and physical acceptance remain requirements. No watch-state import/export, consumer push/email, plugins/themes/custom CSS, Roku, CarPlay/Android Auto or light theme. Sources: [operations](../../_program/reports/ops-packaging.md), [limits](../../_program/reports/limits-settings.md), [console](../../_program/reports/console.md).

## Latest review follow-ups

[Hosted follow-ups](../../_program/reports/hosted-followups.md) deliver fresh-only format 21 and durable newest-certificate/replica consent fixes. [Freshness review](../../_program/reports/review-freshness.md) completes eight independent fixes; unchanged SQL guards and physical/load acceptance remain. [Native follow-ups](../../_program/reports/leftovers-native.md) correct Apple scene/reinstall/request admission and contain Vega redirects by disabling authenticated operation until a safe native transport exists. Scan/live/pipeline/operations reviews are still in progress; read their appended fix status, not just their original findings.
