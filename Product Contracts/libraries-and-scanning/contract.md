# Sources, Libraries, and Scanning Contract

> Current Portico product contract. October decisions supersede the September reconciliation where noted.


Contract ID: `PC-LIBRARY-SCANNING`  
Revision: `portico.october.1`  
Status: **approved**  
Owners: Product, Server Storage, Scanner, Media Analysis, Security, Client Foundations  
Imports: `PC-FOUNDATION` permissions and resource lanes; `PC-GOVERNANCE` lifecycle/exception law; `PC-GLOSSARY` vocabulary  
Coordinates with: `PC-NETWORKING`, `PC-METADATA`, `PC-PLAYBACK`, `PC-SETTINGS-OPERATIONS`  
Applies to: Portico Server, bundled Server Web, Client Core management projections, and every first-party management client

## Scope and non-goals

This contract owns media sources, library composition, inventory, source revisions, scan tiers, scan scheduling, reconciliation, remote-storage behavior, `.strm` descriptors, scan recovery, resource admission, and owner-visible scan state.

It does not define descriptive metadata selection, playback planning, writable cloud file management, provider-specific file synchronization, encrypted-disc decryption, interactive DVD/Blu-ray menus, or a general-purpose rclone/WebDAV file browser. Those boundaries belong to the Metadata, Playback, Security, and Settings contracts.

Portico supports ordinary extracted media files. ISO, `VIDEO_TS`, and `BDMV` sources MAY be catalogued with an explicit unsupported-playback reason, but automatic mounting, decryption, and disc menus are outside this revision.

There are no legacy scan modes in this pre-release contract. `File List Only`, `Basic`, `Complete`, and `Custom` are the only user-facing analysis tiers.

There are no unresolved library/scanning product decisions in this revision. An implementation gap is not permission to invent a fifth tier, make a managed remote look like a local path, or fall back to a legacy scanner.

## 1. Canonical model

### 1.1 Library

A **library** is an owner-configured logical collection with one media kind, one metadata policy, one scan policy, and one or more sources. A library ID is opaque and server-owned. Its name, source paths, remote roots, provider locators, and filesystem identities are mutable evidence rather than identity.

Every library MUST declare a concrete kind. A mixed library MAY exist only as an explicitly supported generic-file experience; a client or scanner MUST NOT silently mix kinds and then guess different hierarchy rules per file.

### 1.2 Source

A **source** is an independently admitted storage origin. Initial source kinds are:

- `local` — a local path or OS-mounted filesystem path;
- `rclone` — a managed rclone remote and root;
- `webdav` — a native WebDAV origin and root.

An OS-mounted SMB, NFS, or user-managed rclone path remains a `local` source with a network-storage classification. Classification controls timeout, health, absence, and scheduling policy; it does not change catalog identity.

Each source owns:

- an opaque source ID;
- library ID and kind;
- display name and normalized root;
- a credential reference, never credential material in ordinary source records;
- health and circuit state;
- current inventory generation/cursor/token;
- current scan tier or library policy projection;
- concurrency and cache budgets;
- last authoritative inventory and last successful progress times.

### 1.3 Object and revision

An inventoried object MUST have a normalized relative path, size when known, modification time when trustworthy, and a **source revision** derived from the strongest provider/filesystem evidence available. Provider object ID, ETag, version ID, checksum, size, and modification time are evidence inputs; no single optional field is universally authoritative.

A path is a locator, not durable media identity. A move or rename SHOULD preserve the canonical media item when stable object identity, exact content evidence, or an unambiguous scanner alias proves continuity. Ambiguous similarity MUST keep items separate for owner review.

A downstream metadata or analysis job MUST carry the exact source revision it was queued for and MUST revalidate that revision before publishing. Work for an older revision is discarded or requeued; it never overwrites newer facts.

## 2. Scan-tier law

Inventory is always the first stage. Analysis tiers govern what may happen after inventory has durably committed.

| Tier | User meaning | Media-content reads | Generated storage | Default |
| --- | --- | --- | --- | --- |
| **File List Only** | Find and organize files using names and source inventory only | Prohibited during scanning | Catalog rows and small indexes only | Available but discouraged |
| **Basic** | Gather the facts needed for reliable matching, playback planning, tracks, chapters, and representative thumbnails | Bounded, targeted reads | Bounded thumbnails, normalized sidecars, and technical facts | **Recommended for rclone and WebDAV; normal default** |
| **Complete** | Perform every enabled deep analysis that benefits discovery and playback features | Sustained and full-file reads permitted | Potentially substantial analysis, image, waveform, fingerprint, and trickplay data | Opt-in with warning |
| **Custom** | Advanced selection of individual operations | Exactly the enabled operations | Depends on selections | Advanced only |

Every surface that offers **Complete** MUST say that it can perform sustained/full-file reads and may require significantly more storage for generated files. Every **Custom** surface MUST identify it as advanced and display the same storage warning before the choices.

The selected tier is a promise and MUST be enforced at the last content-read boundary, not only when work is queued. Changing to a less intensive tier cancels or safely preempts now-prohibited queued/running work. Changing to a more intensive tier schedules missing stages for the current revision without repeating unchanged inventory unnecessarily.

### 2.1 File List Only

File List Only MUST:

- enumerate directory/provider records and read only listing/stat metadata;
- normalize paths and infer media kind, hierarchy, title, year, season/episode, version, and disc/descriptor kind from names and structure;
- publish an inventory-derived catalog promptly;
- perform **zero media-object content reads** during scanning;
- avoid opening media files, remote objects, `.strm` descriptors, NFO/OPF files, embedded tags, embedded artwork, subtitles, lyrics, or sidecars;
- avoid provider range requests and media downloads;
- show that track, codec, duration, chapter, thumbnail, and deep-analysis facts may be unavailable until first playback or a higher tier.

Filesystem directory enumeration and stat calls are not media-content reads. A backend that cannot list without downloading object bodies is not conformant for File List Only.

After inventory commits, File List Only MAY perform descriptive provider matching using only normalized path/name/hierarchy and object-listing evidence. It may retain bounded identity/text decisions, but automatic provider-artwork binary acquisition waits for Basic. This is not permission to open media, NFO/OPF, subtitle, lyric, artwork, `.strm`, or embedded-tag content. Bounded `.plexignore` and `.ignore` reads are scan configuration and are allowed in File List Only. Provider matching remains an independent metadata job under `PC-METADATA`; its network requests and match state are reported separately from source-download bytes.

### 2.2 Basic

Basic MUST first complete the File List Only stage, then MAY perform bounded work that has explicit byte, request, wall-clock, CPU, and concurrency budgets. It SHOULD acquire:

- container, duration, stream, codec, resolution, channel-layout, language, disposition, chapter, and attachment inventory;
- small local NFO/OPF, subtitle, lyric, artwork, and supported embedded-tag evidence;
- one representative thumbnail when no better artwork exists;
- enough seek/range evidence to plan playback reliably;
- stable provider identifiers found in filenames or local metadata.

Basic MUST NOT perform full-file checksums, loudness scans, sonic fingerprints, trickplay/contact-sheet generation, chapter-image sweeps, waveform generation, or content-wide intro/credit detection. It MUST NOT materialize an entire remote object merely because a probe seeks near its end. A bounded range probe may read discontiguous ranges.

Failure to obtain Basic facts MUST leave the inventory item visible with an analysis state and retry policy. It MUST NOT make a valid inventoried item disappear.

### 2.3 Complete

Complete MUST follow a successful Basic stage for the same source revision. It MAY enable:

- full-file technical analysis and exact-seek/keyframe evidence;
- loudness and audio normalization analysis;
- sonic fingerprinting and similarity features;
- intro, recap, commercial, and credit/outro detection;
- trickplay previews, chapter thumbnails, waveforms, and other generated navigation artifacts;
- full-file checksums when required for identity or integrity;
- extraction of embedded covers and attachments not safely obtained by Basic.

Before sustained work, Portico MUST reserve required CPU, memory, temporary storage, output storage, and source bandwidth. A remote object MUST be revision-checked before and after staging; incoherent staging publishes nothing. Temporary materializations are private, bounded, and removed after completion, cancellation, or crash reconciliation.

Complete is never allowed to impede active playback. It is restart-safe and may take hours or days on a large library without holding a database writer, source lock, or UI operation open.

### 2.4 Custom and disk-I/O groups

Custom options MUST be grouped by their impact on disk I/O, not by internal package or FFmpeg terminology.

#### Low disk I/O — directory and small-file reads

- read local NFO/OPF metadata;
- read external subtitle and lyric sidecars;
- discover local artwork and small sidecar assets;
- parse supported filename/provider-ID hints;
- contact enabled metadata providers after durable inventory.

#### Moderate disk I/O — targeted media reads

- probe container and streams;
- read embedded tags, chapters, covers, and attachment indexes;
- generate one representative thumbnail;
- validate bounded seek/range behavior;
- extract selected small embedded covers or attachments.

#### High disk I/O — sustained or full-file work

- full-file checksum or integrity pass;
- trickplay/contact-sheet and chapter-thumbnail generation;
- waveform, loudness, and audio normalization analysis;
- sonic fingerprinting;
- intro, recap, commercial, and credit/outro detection;
- full embedded-attachment extraction;
- explicit owner-enabled analysis of a `.strm` target.

Remote full-object staging is not an independently selectable outcome. It is a derived, revision-fenced execution prerequisite used only when an enabled Complete or Custom operation cannot execute through bounded authenticated reads. The worker MUST revalidate the object revision before and after materialization, publish nothing from incoherent staging, and remove its private temporary copy after completion, cancellation, or crash reconciliation. A client MUST NOT expose a `stageRemoteObject` toggle or imply that staging by itself produces a media fact or artifact.

The serialized Custom profile carries only the owner-selected boolean for each closed, named option. Disk-I/O, network-I/O, generated-storage impact, and dependencies are immutable annotations in `scan-profile.schema.json`; clients and Server code MUST NOT accept mutable copies of those facts. Dependencies are explicit: an option may require `probeStreams`, but enabling one option MUST NOT silently enable unrelated work. The serialized Custom profile validates against `scan-profile.schema.json`.

## 3. Pipeline and publication

The canonical pipeline is:

```text
admit source
  → inventory
  → persist object/source revisions
  → classify and derive provisional hierarchy
  → publish inventory catalog
  → queue Basic work for exact revision
  → atomically publish Basic facts
  → queue Complete/Custom high-I/O work when selected
  → atomically publish generated facts/artifacts
  → reconcile absence only from authoritative evidence
```

Rules:

1. Inventory commits in bounded pages/batches before enrichment.
2. No filesystem walk, provider request, child process, or artifact write occurs while a database write transaction is held.
3. Catalog publication is atomic per bounded item/batch and is fenced by source revision.
4. Metadata matching and media analysis are durable downstream jobs, not recursion inside a directory walk.
5. Each stage is idempotent for `(source, object, sourceRevision, policyRevision, stageRevision)`.
6. Generated artifacts publish by private temporary write, validation, atomic replacement, and durable reference update.
7. A crash or lost response resumes from durable cursors and completed item-stage records.
8. Progress is factual: discovered, indexed, unchanged, queued, analyzed, skipped, failed, and pending are distinct counts.

## 4. Local and mounted sources

### 4.1 Enumeration

Local scanning MUST use bounded, cancellation-aware enumeration and per-source admission. It MUST normalize filesystem paths, reject traversal outside configured roots, and preserve the configured/display path separately from the resolved path.

Native change notifications are acceleration hints only. Scheduled durable reconciliation remains required because watchers overflow, coalesce, miss offline changes, and differ by platform.

Unchanged-directory checkpoints MAY skip traversal only when their evidence remains valid for that source and scan mode. A forced full scan bypasses checkpoints without discarding the last known-good catalog until replacement evidence commits.

### 4.2 Symlinks, links, cycles, and boundaries

- Symlink following is owner-selectable per source and defaults off.
- A followed link MUST resolve beneath an explicitly allowed root or be rejected.
- Directory identity/visited tracking MUST break symlink, junction, bind-mount, and hardlink cycles.
- One physical file exposed through multiple paths MAY become versions of one item only with unambiguous identity evidence; path coincidence is insufficient.
- Special device files, sockets, pipes, and reparse targets that are not regular files/directories are excluded.
- Permission errors are recorded per root/object and do not silently authorize deletion.

### 4.3 Archives and disc structures

General archive expansion during scans is prohibited. ZIP/RAR/7z/tar files are catalogued only when a separately approved media-kind contract supports them. Scanner code MUST NOT recursively unpack attacker-controlled archives.

ISO, `VIDEO_TS`, and `BDMV` MAY appear as one logical item with a truthful playback limitation. Presence never implies decryption, menu, or mount capability.

## 5. Managed rclone

### 5.1 Owner-supplied binary

Portico MUST allow the owner to select an rclone executable. It MUST:

- resolve symlinks to a canonical path;
- require an executable regular file;
- execute only a bounded `rclone version` validation command;
- record version and SHA-256 fingerprint;
- revalidate the fingerprint before each new backend lifecycle;
- fail closed if the binary is replaced;
- never silently download, update, or replace rclone.

### 5.2 Configuration

Portico MUST support both importing an owner-provided configuration and creating/editing a remote through a managed setup flow. The managed flow may use provider-specific forms or a bounded rclone configuration protocol, but it MUST NOT expose a shell, arbitrary command entry, or an unbounded inherited interactive terminal. In every configuration path:

- config and password material live in private application data (`0700` directory and `0600` file or stronger platform equivalent);
- installation is atomic and recoverable;
- credentials never appear in process arguments, logs, diagnostics, library APIs, inventory rows, QR codes, or client state;
- only allowlisted rclone subcommands and flags are constructed by Portico;
- arbitrary RC, shell, backend command, and generic command passthrough are prohibited;
- `RCLONE_ASK_PASSWORD` or equivalent interactive prompting is disabled for background work;
- test/validation completes before the source is committed active;
- credential rotation creates a candidate configuration, validates it, then atomically swaps it without destroying the last working configuration prematurely.

The managed setup lifecycle is `draft → binary_approved → configuration_pending → authorization_pending | validating → ready | needs_action`. Browser/OAuth authorization, when required by a backend, resumes the same opaque setup operation; the browser receives no stored credential or config file. Replaying, abandoning, or opening the flow on a second device cannot create duplicate active remotes or publish a partially validated config. A newer config generation fences every validation, listing, mount, and read started under the older generation.

### 5.3 Inventory and mount lifecycle

Managed rclone scanning MUST use provider/object listing, never a FUSE directory walk. It SHOULD list one directory/page at a time with a durable bounded queue/cursor and MUST support libraries with millions of objects without retaining the full object list in memory.

An owner MUST be able to enable a supervised read-only compatibility mount when the selected rclone backend and host support rclone mount semantics. Scanning and ordinary remote reads do not require it. Its lifecycle is `stopped → starting → healthy | degraded → stopping → stopped`, with supervised restart policy, owner-visible health, private cache, maximum cache size, minimum free-space floor, and explicit unmount on source removal. If the host cannot support the mount, setup reports that capability limitation before the owner enables it; Portico does not pretend that object-listing access is a mounted filesystem.

The mount is never scanner truth. Starting, stopping, or losing it cannot rewrite inventory or prove objects absent. A mount cache uses bounded full-mode/sparse behavior appropriate for seeking and MUST yield to active playback and storage floors.

## 6. Native WebDAV

WebDAV is implemented through native object inventory and ranged reads; it does not require an OS mount.

- HTTPS is required for non-local origins. Any HTTP exception follows `PC-NETWORKING` and is visibly classified as local/insecure.
- Credentials are sent only in authorization headers and are stored through the server secret boundary.
- Redirects MUST remain same-origin unless the owner explicitly reconfigures and revalidates the origin.
- Full inventory SHOULD use breadth-first `PROPFIND Depth: 1`; it MUST NOT require an unbounded `Depth: infinity` response.
- RFC 6578 `sync-collection` tokens SHOULD drive delta inventory where supported. Invalid/expired tokens fall back to a complete inventory without treating unknown objects as deleted.
- Responses, XML depth, entry count, href length, and cursor state are bounded.
- Hrefs are decoded and normalized beneath the configured root; traversal, duplicate alias, and cross-origin hrefs are rejected.
- ETag/version, content length, modification time, and content type form revision evidence.
- Basic analysis and playback SHOULD use byte ranges. If the origin lacks correct range behavior, File List Only still works, while analysis/playback capability reports the limitation honestly.

Setup validates TLS/origin policy, authentication, a bounded root `PROPFIND`, href containment, and advertised range behavior before activation. Editing the origin, root, username, credential, or trust policy creates a candidate configuration generation; it does not replace the last working generation until validation succeeds.

## 7. Remote inventory, caches, and reads

### 7.1 Inventory

Remote inventory MUST be page-transactional: apply a validated page before advancing its durable cursor. Replaying a page is idempotent. A full run is authoritative only after every page/root completes and the terminal generation commits.

Delta inventory touches only objects reported changed/deleted. It MUST NOT rewrite every catalog row in a million-object source. A delta report cannot prove absence outside its explicit deletion records.

The inventory cache stores normalized object evidence, not provider credentials or signed download URLs. Provider URLs are short-lived execution inputs only.

### 7.2 Ranged reads

Remote reads MUST:

- identify one exact object revision;
- use bounded request/byte/time budgets for Basic;
- coalesce sensible adjacent ranges and avoid spawning a process per tiny segment;
- authenticate server-side without exposing provider credentials to FFmpeg, clients, or logs;
- validate status, range, length, and revision behavior;
- cancel cooperatively and close provider bodies promptly;
- separate background-analysis and foreground-playback admission.

First playback MAY perform a bounded just-in-time probe when required facts are missing. This is playback work, not permission for a background File List Only scan to read content.

## 8. `.strm` descriptors

`.strm` is a first-class local descriptor, not a media file and not a remote-storage backend.

- Inventory recognizes it by extension and derives provisional identity from local name/structure without reading it.
- The catalog retains the local descriptor path/revision, never its resolved URL.
- An authorized playback request reads at most 64 KiB, requires exactly one non-empty HTTP(S) locator, rejects URL userinfo and malformed/disallowed origins, and re-applies DNS/IP/redirect SSRF policy at every connection.
- Descriptor contents, query tokens, and resolved locators never enter catalog APIs, logs, diagnostics, search indexes, backups of catalog metadata, or client state.
- Scan never fetches the target URL merely to match metadata.
- Basic/Complete MAY analyze a STRM target only when a distinct owner-enabled option exists; that operation is network-heavy, uses the remote read/proxy security boundary, is revision-fenced to the descriptor, and cannot persist the target locator.
- Failure or change of the target leaves the descriptor inventoried and reports playback/analysis availability separately.

## 9. Scheduling, concurrency, and playback priority

Use PC-FOUNDATION's single resource-lane order. No feature-specific priority list overrides it. Background inventory, enrichment and analysis yield through bounded quanta, per-source admission and durable continuations. A blocked source cannot monopolize healthy-source workers; latency and faults reduce background concurrency, not authorized playback admission.

Scheduled scans are jittered and coalesced by exact source and compatible policy. A stronger request may add stages, but cannot broaden File List Only or Custom permissions without an owner policy change. A forced full inventory and a delta scan are different evidence requirements; coalescing retains the stronger requested coverage. Cancellation fences publication durably and preserves committed inventory. Completion of required cleanup is distinct from acceptance of cancellation.

Native rclone/WebDAV inventory and OS-mounted filesystem enumeration use distinct adapters. All obey the inventory/generation and source-revision rules in the source/catalog amendment below. No filesystem, provider or process call holds a database write transaction.

## 10. Absence, deletion, and reconciliation

A title and its files have separate lifetimes. A complete healthy observation of the exact admitted root and subtree marks absent files missing immediately. A verified paths-changed/watch observation can mark the final removed file missing. No second scan or missing-file grace is required.

Timeout, cancellation, permission loss, disconnected or changed mounts, invalid cursors, partial pages and unhealthy roots cannot prove absence. Protect the affected subtree and keep healthy observations elsewhere. An ordinary suddenly empty root that held media is unavailable until reachability/coverage can be established; verified reported removal is distinct.

A title with no available copies stays marked missing until the owner removes missing titles immediately, after a configured period (default 30 days), or never. Another recognised copy restores the title. Final removal retains supported personal state by recognised/provider identity; per-play history and playlist positions are not all restored.

Deleting in Portico deletes the selected file on disk after preview and confirmation. There is no state-folder trash, trash restore or trash-folder retention. One-version deletion preserves other copies and personal state; deleting the last follows the missing-title policy. A changed file is not substituted into the deletion. Library removal keeps files and runs bounded catalogue cleanup. Removing one source marks its files unavailable while keeping other sources and protected state.

Hidden empty library rows, large show merge transactions, untagged-audio replacement identity and some absorbed-reference repairs remain implementation limits in the identity reports.

## 11. Failure and owner-visible states

Source states are `validating`, `ready`, `inventory_running`, `healthy`, `degraded`, `offline`, `credentials_required`, `binary_changed`, `rate_limited`, `cancelled`, and `removing`.

Scan states are `queued`, `inventory`, `catalog_published`, `basic_pending`, `basic_running`, `complete_pending`, `complete_running`, `reconciling`, `complete`, `complete_with_warnings`, `cancelled`, and `failed`.

The UI MUST distinguish:

- library inventory is usable while enrichment continues;
- a source is offline while last-known catalog remains;
- some objects failed but the run completed with warnings;
- analysis was slowed for playback, or explicitly paused by the owner and will resume;
- credentials, binary approval, range support, or free storage require owner action;
- a destructive cleanup awaits confirmation.

Raw provider stderr, credentials, paths beyond the owner’s necessary scope, URLs, query strings, HTTP bodies, and child-process arguments are diagnostic-only and redacted. Viewer clients never receive storage-source diagnostics.

## 12. Security and privacy

- Source management requires the server owner and an interactive management session.
- Library APIs return credential presence, never credential values.
- Remote URL policy prevents SSRF to loopback, link-local, multicast, broadcast, cloud metadata services, and private ranges unless the configured source is an explicitly permitted local origin.
- Content sniffing, XML parsing, sidecars, artwork, archives, and tool output are bounded and treated as attacker-controlled.
- FFmpeg/FFprobe run with `-nostdin`, protocol allowlists, time/output limits, private working directories, and no provider secret in argv/environment visible to diagnostics.
- Inventory and analysis remain server-local. Hosted Services receives no paths, filenames, titles, media facts, provider locators, or source credentials.
- Logs use opaque source/object IDs and bounded error classes; owner-exported diagnostics apply explicit redaction.

## 13. Conformance

The canonical serialized profile is `scan-profile.schema.json`; scenario fixtures are `conformance.json`. A conformant implementation proves at least:

- File List Only performs zero media/sidecar/descriptor content reads for local, rclone, and WebDAV;
- inventory appears before delayed analysis on a large library;
- Basic stays within configured range/request/time budgets and can create a representative thumbnail;
- Complete follows Basic for the same revision and reserves/stages coherently;
- playback preempts remote/local background analysis without consuming a failure attempt;
- an incomplete/offline inventory never deletes media;
- cursor replay, process restart, cancellation, and source-revision change are safe;
- one blocked source does not block an independent source;
- rclone binary replacement fails closed and credentials never reach argv/logs;
- WebDAV invalid sync token falls back without false deletion;
- `.strm` locator stays private and is validated again at playback;
- symlink cycles terminate and root escape is rejected;
- Custom dependency and I/O-impact presentation is truthful;
- million-object simulation uses bounded memory, pages, queues, and database batches.

Minimum applicability matrix:

| Rule family | Server | Hosted | Web admin | Client Core | Native management clients | Viewer-only clients |
| --- | --- | --- | --- | --- | --- | --- |
| Source/inventory authority | required | not applicable | project status | types/actions | project status | not exposed |
| Scan tiers and warnings | enforce | not applicable | required | canonical enums | required where managed | status only |
| Remote credentials | required | prohibited | write-only | omit secrets | write-only | prohibited |
| Playback priority | required | not applicable | report | status | report | benefit only |
| Absence/destruction | required | not applicable | confirm | canonical actions | confirm where offered | not exposed |
| Failures/progress | required | not applicable | required | canonical states | required | bounded availability |

Approved exceptions use the governance form exactly, with `Contract: PC-LIBRARY-SCANNING#<section>` and a test/fixture that prevents scope expansion.

The informative comparison behind these decisions is in `plex-jellyfin-precedent-review.md`; it cannot override this contract.


## Reconciled architecture boundary

The applicable section of [Domain architecture and failure boundaries](../architecture-amendments.md) completes this contract. It resolves named implementation conflicts; original UX and product choices remain as recorded in [decisions.md](../decisions.md). Old schema/fixture references are historical inputs to be ported, as defined by the current registry, not a second rebuilt API authority.

## Naming, monitoring and tolerance — October decisions

Read source-relative folders, filenames and bounded embedded evidence together. Usable file evidence wins conflicts. S02E01 inside Season 1 is season 2; E01/01 inside Season 2, flat files, episode zero, translated seasons, editions and parts are supported reader inputs. Embedded audio tags outrank path hints. Keep provenance, contradictions and confidence; do not require strict folder/file agreement.

A bad file/probe, NFO row, ignore line or unreadable subfolder leaves usable neighbours and owner diagnostics. Bounds, symlink containment and source custody remain enforced. Disc folders become one unsupported logical entry rather than one title per stream.

`monitorChanges` defaults true. Native watchers debounce into limited scans; bounded periodic fallback covers unavailable watches and network sources. Sidecar changes invalidate affected evidence. Unchanged directory times do not prove unchanged local files. Analysis queues only missing/revision-invalid evidence. Terminal job/change/listing history is bounded. Gentle/Normal/Fast controls pacing; playback slows rather than abandons analysis. Windows/macOS watches and network mounts need native acceptance.

## October scope and evidence

The later scan-loop report covers naming/ignore/sidecar wiring left open in earlier reports. Native watchers/network shares and large-merge/reference correctness still need acceptance or fixes.

Sources: [identity](../../../_program/reports/identity.md), [scan-loop](../../../_program/reports/scan-loop.md), [naming](../../../_program/reports/naming.md). [Owner decisions](../decisions.md) govern. Each existing conformance matrix is required coverage, not proof of execution. Release parity is required for applicable first-party features; truly unsupported platform capabilities are hidden. Roku, CarPlay, Android Auto and light theme are outside this programme. No consumer push or email. No stable API or old-database compatibility is promised before release.
