# 14 — Downloads and offline use

> Comparative audit, 11 October 2026. Read-only static source review of the working trees as they stood that day. Nothing here was run; every "bug" is a reading of the code, with a stated confidence.

Path shorthand used throughout (all relative to `/home/justin/projects/Portico`):

| Short | Full |
|---|---|
| `S/` | `apps/portico-server/server/` |
| `CC/` | `apps/portico-server/packages/client-core/src/` |
| `W/` | `apps/portico-server/web/src/` |
| `N/` | `apps/portico-react-native/apps/native/` |
| `PC/` | `portico-internal/Product Contracts/` |

Comparison paths are written `jellyfin/…`, `plezy/…`, `streamyfin/…`, `finamp/…`, `jellyfin-android/…`, `audiobookshelf/…`, `navidrome/…`.

## Scope and sources

**Domain.** Asking the server for a download, the server preparing it, moving the bytes to a device, storing them, browsing and playing with no network, sending watch progress back, automatic downloads, and storage management on both sides.

**Portico material read in full**

- Contract and docs: `PC/saved-state-and-downloads/contract.md` (all 349 lines), the downloads paragraphs of `PC/architecture-amendments.md`, `PC/decisions.md` (P06, P15, R08, R09), `PC/conformance.md` (CF32–CF40, CF60), `S/api/downloads.md`, `Project Architecture/04 Playback and Media.md`, `06 Clients.md`, `Backlog.md`.
- Server: every non-test file in `S/internal/downloads` (types, service, policy, preparations, worker, source_snapshot, grants, receipts, progress, requests, targets, options), `S/internal/preparedmedia` (downloads_access, custody, types, and the worker/artifacts/service parts that decide concurrency, recipes and deletion), `S/internal/decoder/prepared.go`, `S/internal/artifactlease`, `S/internal/diskspace`, `S/internal/httpapi/downloads_routes.go`, `download_requests.go`, the download rows of `route_lanes.go`/`admission.go`, the download tables and indexes in `S/internal/persistence/migrations/0001_baseline.sql`, the wiring in `S/cmd/server/main.go`, and — for comparison with offline progress — the online progress path in `S/internal/playback/personal_progress.go`, `S/internal/playback/service.go` and `S/internal/catalog/personal_activity.go`.
- Shared client logic: `CC/downloads.ts`, `CC/downloads-service.ts`, `CC/download-requests.ts`, `CC/prepared-media.ts` (first part), `CC/storage-management.ts` (it is about managed remote mounts, not downloads).
- Web: `W/screens/downloads/Downloads.tsx`, the download sheet in `W/app/downloads.tsx` (skimmed).
- Native app: `N/src/bridge/offline-downloads.ts` (all), `N/src/app/downloads.tsx`, `N/src/screens/Downloads.tsx`, `N/src/shell/DownloadsGate.tsx`, `N/src/screens/auth/OfflineDownloads.tsx`, `N/src/app/download-toasts.ts`, `N/src/bridge/local-playback.ts`, the offline parts of `N/src/app/session.tsx`, `N/Native/PorticoDownloads.m` (all), `N/Native/PorticoDownloadAttempt.h`, the local-file gate in `N/Native/PorticoPlaybackController.m`, Android `PorticoDownloadCenter.kt` (all) and the surface of `PorticoDownloads.kt`. Vega: confirmed the module is a stub that rejects.

**Comparison material read**

- **Jellyfin server:** `jellyfin/Jellyfin.Api/Controllers/LibraryController.cs` (`GetDownload`, `GetFile`, `LogDownloadAsync`), `BaseItem.CanDownload`, the `EnableContentDownloading` / `EnableSyncTranscoding` permissions.
- **Plezy:** `lib/database/tables.dart` (whole schema), outline and key sections of `lib/services/download_manager_service.dart` (4,517 lines), `sync_rule_executor.dart`, `offline_watch_sync_service.dart`, `download_storage_service.dart`, `downloaded_video_source.dart`, `offline_mode_provider.dart`, `jellyfin_client/parts/images_downloads.dart`, `media/download_resolution.dart`, `background_download_warning_banner.dart`.
- **Streamyfin:** `providers/Downloads/README.md`, `types.ts`, `utils/profiles/download.ts`, `hooks/useTwoWaySync.ts`, `hooks/usePlaybackManager.ts` (offline adjacent items), `modules/background-downloader/README.md`.
- **Finamp:** documentation comments of `lib/services/downloads_service.dart`, `offline_listen_helper.dart`, download transcoding settings in `finamp_models.dart`.
- **jellyfin-android:** `downloads/DownloadWorker.kt`, `DownloadMethod.kt` and the file list of `downloads/`.
- **Swiftfin:** searched; the current tree has no offline download feature.
- **Audiobookshelf server:** `server/managers/PlaybackSessionManager.js` (`syncLocalSession`).
- **Navidrome:** `core/archiver.go` signatures and `conf/configuration.go` download options.
- **Closed products (Plex, Infuse, Netflix):** from general product knowledge only. I did not fetch their documentation in this session, so statements about them are marked "from memory" and should be treated as prompts, not evidence.

**Not reached:** Plezy's `download_provider.dart` (2,879 lines) and its downloads screens line by line; Streamyfin's hooks line by line; Finamp's backend file; the Audiobookshelf mobile app (not cloned); the two Portico OpenAPI YAML files line by line (I relied on `downloads.md` and the Go and TypeScript code, which are the truth); Portico test files except where they settled a question; the Roku app (out of scope).

## How Portico does it today

### The idea in one paragraph

A download in Portico is not "fetch this file". It is a **claim** (the code calls it a *preparation*) that one viewer makes on one title at one quality. The server works on the claim in the background until it is `ready`, then hands out a short-lived web address (a *grant*) for the bytes, and finally issues a signed note (an *offline receipt*) that says this exact viewer may play this exact file for the next 30 days. The app refuses to play a downloaded file without a valid receipt. Every step is idempotent (safe to repeat) and fenced by a revision number.

### Server, step by step

1. **Menu.** `GET /v1/items/{id}/download-options` lists `original` plus the quality ladder rungs no taller than the source (2160p … 360p), each with a size, whether it is available and why not (`S/internal/downloads/options.go:80-200`). Audio items get one rung, "AAC 192 kbps".
2. **Claim.** `POST /v1/downloads/preparations` takes one title, a list of up to 100 titles, or "the episode after X" (`S/internal/downloads/preparations.go:32`, `targets.go:159`). A whole show, season, album, book or playlist goes through a second, durable endpoint, `POST /v1/downloads/requests`, which the worker expands 20 members at a time with a policy of `all`, `unwatched` or `next N` (`requests.go:117-455`). A unique index allows one live claim per viewer, title and quality (`0001_baseline.sql:3798`).
3. **Preparing `original`.** The worker **copies the whole library file** into `<state>/downloads/source-snapshots/<claim id>.source`, hashing as it copies (`source_snapshot.go:23-120`), then **reads the copy again** in 32 MiB windows to recompute the SHA-256 and compare (`worker.go:491-566`). Only then is the claim `ready`. The copy is kept for as long as the claim is live.
4. **Preparing a rung.** The claim points at a *prepared version*: a complete H.264 + stereo AAC MP4 that the prepared-media producer encoded and stored. If none exists, an **owner's** claim queues the encode; anyone else's claim ends as `unavailable: optimized_version_unavailable` (`worker.go:359-456`, `preparedmedia/downloads_access.go:61`). Prepared versions are shared: every viewer who asks for the same rung of the same title gets the same file.
5. **Transfer.** `POST …/{id}/grant` mints a token. `GET /v1/downloads/artifacts/{token}` serves the bytes with `Range`, `ETag: "sha256-…"` and `If-Range` support through Go's `http.ServeContent` (`httpapi/downloads_routes.go:206-231`). No sign-in token is read on that route; the address is the permission. A grant must be first used within 10 minutes and can be reused for 10 minutes after first use (`types.go:105-108`, `grants.go:150-155`).
6. **Receipt.** `POST /v1/downloads/receipts` signs, with an Ed25519 key the server generates and keeps in SQLite, a statement naming the viewer, title, quality, file hash and size, valid 30 days (`receipts.go:241-346`). `POST …/revalidate` renews it; a refusal means "delete the file". A revocation list endpoint exists as well.
7. **Offline progress.** `POST /v1/downloads/progress` accepts up to 100 observations (`itemId`, position, optional `watched`, the device's timestamp). The newest timestamp wins against the last offline observation and the last online activity (`progress.go:66-189`).
8. **Housekeeping.** A ready claim expires `retentionDays` (default 30) after its last grant; expiry revokes its receipts (`worker.go:122-160`). The owner can cap total claimed bytes with `maxPreparedBytes` (default unlimited) (`service.go:281-297`). Snapshot files with no live claim are deleted (`source_snapshot.go:158-195`).

One background goroutine does all of this, one claim at a time (`worker.go:64-116`).

### Clients

- **Shared logic (`client-core`).** `DownloadsService` lists claims, polls active ones every 5 seconds, sends requests and actions, and follows whole-container requests it started (`CC/downloads-service.ts`). Strict parsers reject any response that does not match the contract (`CC/downloads.ts`).
- **iPhone, iPad, Android phone and tablet.** `OfflineDownloads` (`N/src/bridge/offline-downloads.ts`) remembers what this device "wants", and when a wanted claim turns `ready` it asks for a grant and hands the address to the native module. iOS uses a background `URLSession` download task (`N/Native/PorticoDownloads.m`); Android uses OkHttp on a two-thread pool kept alive by a foreground service (`PorticoDownloadCenter.kt`). Both verify the SHA-256 of the finished file, then file it under `Downloads/<claim id>/<claim id>.<ext>` in app-private storage that is excluded from backup. A small JSON manifest lists entries; a second JSON document holds settings, the queue of unsent progress, resume points, receipt keys and outstanding "wants".
- **Playing offline.** A file is playable only if it has a receipt whose signature verifies against a remembered key, whose viewer equals the saved viewer, and whose expiry is in the future by the device clock (`offline-downloads.ts:508-515`). In the signed-in shell it plays in Portico's own player as a bare local file (`local-playback.ts`); from the "server unavailable" launch screen it plays in the system player.
- **Settings.** Wi-Fi only (on by default), remove after watching, download the next episode after finishing one, and a device storage limit (`N/src/screens/Downloads.tsx:172-184`).
- **Web.** A list of claims with a "Save" button that navigates the browser to the grant address (`W/screens/downloads/Downloads.tsx:45-53`). No offline support in the browser.
- **TVs and Vega.** No downloads (`offline-downloads.ts:116`, Vega's module rejects).

### What does not exist

No sidecar subtitles, artwork, chapters, intro/credit markers or trickplay travel with a download. There is no offline library view beyond one flat list, no offline search, no queue or next-episode when playing a download, no standing "keep N episodes" rule, no way to mark something watched while offline, and no step that pulls the server's newer watch state down to a downloaded copy.

## How the others do it

### Jellyfin (server and official clients)

- **Server.** One endpoint: `GET /Items/{id}/Download` returns the library file itself with its original file name and HTTP range support (`jellyfin/Jellyfin.Api/Controllers/LibraryController.cs:669-720`). It needs the per-user permission `EnableContentDownloading`, and writes an activity-log line "user is downloading X". There is no queue, no stored state, no hashing, no expiry. A second permission, `EnableSyncTranscoding`, says whether the user may make the server convert for a download; clients that want a smaller file simply download from the ordinary transcoding stream endpoint. The old Emby-era "Sync" service (server-managed conversion jobs per device) is not in Jellyfin's core.
- **jellyfin-android.** A WorkManager job per download with a network constraint chosen in settings: Wi-Fi only (default), mobile data, or mobile and roaming (`jellyfin-android/…/downloads/DownloadWorker.kt:36-38`), a Room table of downloaded files, and a `ContentRange` helper for resuming.
- **Swiftfin.** No offline downloads in the current source.
- **jellyfin-web.** A "Download" menu item that opens the endpoint above.

### Plezy (the fullest open-source implementation here)

- **What is downloaded.** Always the original file (it plays anything through mpv, so it never needs a conversion). Items split across several files are fetched whole. Alongside the video it fetches external subtitle files, poster/logo/backdrop artwork and chapter thumbnails (`plezy/lib/services/download_manager_service.dart:3157-3392`, `media/download_resolution.dart`).
- **Transfer.** The `background_downloader` plugin (background `URLSession` on iOS, WorkManager on Android): native pause/resume, automatic rescheduling of tasks killed with the app, a free-space check before each task, one download at a time, a Wi-Fi-only rule that is re-applied to queued tasks when the setting changes, and a circuit breaker that stops the queue when storage fills (`…:918-1100`, `:2642-2760`).
- **Local database.** A real SQLite schema (`plezy/lib/database/tables.dart`): `DownloadQueue`, `DownloadedMedia` (status, bytes, file paths, retry count, native task id, library id and title), `DownloadOwners` (which profile may see a physical file — files are stored once and shared), `SyncRules`, `SyncRuleDownloads`, `DownloadCollections`, `OfflineWatchProgress`, and `ApiCache` rows that are **pinned** so the ordinary title pages keep working offline.
- **Storage.** App folder by default, or any folder the user picks including an SD card through Android's storage-access framework; human-readable names such as `Show (2019)/S01E02 - Title [1a2b3c4d].mkv` (`download_storage_service.dart:105-340`).
- **Offline mode.** The whole app switches to an offline state when no visible server is reachable (not when the OS says "no internet") (`offline_mode_provider.dart:66-74`); downloads are shown as a tree grouped by library, show, season and collection, with sort and filter.
- **Automatic downloads.** Persistent *sync rules*: keep N unwatched episodes of a show or season, or mirror a playlist or collection, with a 30-minute (Wi-Fi) or 3-hour (cellular) cooldown and cleanup when the rule is removed (`sync_rule_executor.dart:37-65`).
- **Watch state.** A durable queue of offline actions (`progress`, `watched`, `unwatched`), pushed when a server comes back and then followed by a pull of the server's state for downloaded items; failed pushes are kept, never dropped (`offline_watch_sync_service.dart:37-126`, `:233-300`, `:842-850`).
- **Housekeeping.** Start-up reconciliation between native tasks and database rows, repair of missing artwork and subtitles, a banner that warns when a phone maker's battery settings will kill background downloads.

### Streamyfin (React Native, closest technology to Portico's app)

- Stores the **whole item record and media-source record** for each download, plus trickplay sheets, external subtitles, cover images and intro/credit/recap segments (`streamyfin/providers/Downloads/types.ts`, `README.md`).
- Quality: the user picks a bitrate; the app sends a *download device profile* and downloads the server's on-the-fly MP4 transcode (`utils/profiles/download.ts`).
- Background downloads through a native module (`NSURLSession` on iOS, the system `DownloadManager` on Android), with an iOS Live Activity for progress.
- Offline next/previous episode from the downloaded set (`hooks/usePlaybackManager.ts:21-90`).
- Two-way progress sync per item by comparing `LastPlayedDate`: the newer side wins, in either direction (`hooks/useTwoWaySync.ts`).

### Finamp (offline-first music)

- Downloads form a graph: an album, playlist, artist or the whole library is a *required* node whose children (tracks, images) follow from it. Re-syncing a node picks up tracks added to a playlist or album later (`finamp/lib/services/downloads_service.dart:431-548`).
- Optional transcoded downloads with a chosen codec and bitrate, several download locations, a "repair downloads" action, an explicit offline-mode switch, and a log of offline listens that is submitted later or can be exported as a file (`offline_listen_helper.dart`).

### Audiobookshelf, Navidrome

- **Audiobookshelf** syncs whole offline *listening sessions* (position, time listened, device) so offline listening appears in history and statistics; progress is updated when the session is newer than what the server holds (`audiobookshelf/server/managers/PlaybackSessionManager.js:127-280`).
- **Navidrome** has no app-side offline feature of its own, but the server can zip an album, artist or playlist for download, optionally transcoding each track, with sensible file names and cover art (`navidrome/core/archiver.go:50-249`), behind an `EnableDownloads` switch.

### Plex, Infuse, Netflix (from memory, not checked this session)

- **Plex Downloads** (formerly Sync): per-item quality choice (original or converted), "unwatched only", "limit to N", automatic removal of watched items, Wi-Fi-only, subscription-style downloads of new episodes, and a per-shared-user "allow downloads" switch.
- **Infuse:** downloads the original with its metadata and artwork; resumes; can keep watched state in iCloud.
- **Netflix "smart downloads":** delete a watched episode and fetch the next one, only on Wi-Fi; "downloads for you".

## Architectural differences

Entries are grouped: server model (1–12), transfer (13–18), on-device storage and catalogue (19–25), offline playback and watch state (26–31), policy and automation (32–39).

### A-14.1 A download is a stored, stateful claim, not a file request
- **Portico:** Four calls and four tables before a byte moves: options → preparation → grant → transfer → receipt (`S/api/downloads.md:15-21`; tables `download_preparations`, `download_grants`, `download_receipts`, `download_requests` in `0001_baseline.sql:2513-2560, 3566-3584`).
- **Jellyfin / Plezy / others:** One authenticated `GET` of the file (`jellyfin/…/LibraryController.cs:669`). The server keeps nothing. All state lives in the client.
- **Why it matters:** Portico can show a download on every device of a profile, cap storage, revoke, and survive restarts. It also means a download cannot start until a background worker has run, and the server carries per-download rows and (for originals) files.
- **Assessment:** **Trade-off (subjective).** Deliberate (`PC/saved-state-and-downloads/contract.md` §7–§8). The stateful model earns its keep for *converted* downloads. For *original* downloads it currently costs far more than it gives (see A-14.2). Moving originals to a light path would be medium work in `internal/downloads` and no client change.

### A-14.2 "Original" is copied into the state folder and hashed twice before it can be fetched
- **Portico:** The worker copies the full source file to `<state>/downloads/source-snapshots/<claim>.source` while hashing (`source_snapshot.go:23-120`), then re-reads the copy to hash it again (`worker.go:244-251, 525-566`). The copy is per claim, so two viewers downloading the same film make two copies (`source_snapshot.go:105`). It stays until the claim leaves `queued/running/paused/ready` (`source_snapshot.go:185`), i.e. normally for `retentionDays`.
- **Jellyfin / Plezy / Streamyfin:** Serve the library file directly. No copy, no hash.
- **Why it matters:** For a 60 GB remux on a small box: read 60 GB, write 60 GB, read 60 GB again, and need 62 GB free on the *state* volume (often a small SSD, not the media array) — all before the first byte reaches the phone. A family of four downloading the same film holds 240 GB of copies for a month. The copy also runs inside the single worker pass while holding the worker's lock, so every other claim, container request and expiry waits (`worker.go:64-67`, `source_snapshot.go:24`).
- **Assessment:** **Portico behind.** The motive is sound and written down in the code comment — a library path is mutable, so the hash a receipt vouches for must describe bytes that cannot change (`source_snapshot.go:18-22`). But the cost model conflicts with "runs well on weak hardware". Cheaper designs that keep the guarantee: (a) hash the library file in place and serve it directly while its size and modified time still match, failing the transfer with `source_changed` if they change (playback already trusts size + mtime through `OpenPlayback`); (b) keep the snapshot but key it by content (one copy per asset revision, shared by all viewers, reference-counted); (c) use a reflink/hard link when the state folder and library share a filesystem. Option (a) is the smallest: medium work, server only. See B-14.5.

### A-14.3 Converted downloads are finished files made ahead of time and shared
- **Portico:** A rung resolves to a stored *prepared version* (`preparedmedia`), encoded once and reused by every viewer; a claim just points at it (`worker.go:405-428`). Only the owner's request can start an encode (`worker.go:360`, `downloads_access.go:66`).
- **Streamyfin / Finamp / Jellyfin:** The client downloads the live transcode stream; the encode exists only for that transfer and is thrown away. **Plezy:** never converts. **Plex (from memory):** converts per request on the server, any permitted user can trigger it.
- **Why it matters:** Portico's model gives exact sizes, a real hash, resumable transfers and zero repeated CPU for the second viewer. It also means a non-owner can never get a smaller file unless the owner prepared that title at that rung beforehand; in practice a guest sees "No optimized copy at this quality exists yet. Ask the server owner…" on almost everything.
- **Assessment:** **Trade-off (subjective)**, leaning **Portico ahead** on the engineering and **behind** on what a non-owner can actually do. Deliberate ("downloads never widens who may order encoding work", `S/api/downloads.md:55-60`). A per-profile "may request conversions" switch (Jellyfin's `EnableSyncTranscoding`) would be small on top of what exists.

### A-14.4 Quality is a fixed server ladder; the device's abilities are not consulted
- **Portico:** `download-options` takes no device profile. It offers `original` whatever the container and codecs, and every rung at or below the source height (`options.go:151-198`). The app shows the list as given (`N/src/app/downloads.tsx:133-141`).
- **Streamyfin:** sends a download device profile so the server decides copy versus convert per stream (`streamyfin/utils/profiles/download.ts`). **Plezy:** plays everything with mpv, so the question does not arise.
- **Why it matters:** Portico's iOS player is AVPlayer (`06 Clients.md:44`), which cannot open Matroska and cannot decode several common audio formats. A user can download a 20 GB "Original" MKV to an iPhone that cannot play it. The contract anticipated this: "the server MAY remux without loss when the original container is not usable by the device; it MUST disclose that compatibility remux" (`contract.md` §7). No such remux exists.
- **Assessment:** **Portico behind.** See B-14.12 and F-14.14. Medium: pass the same capability document playback already sends, mark options `playableOnThisDevice`, and add a lossless-remux recipe.

### A-14.5 One conversion recipe family: H.264 SDR, stereo AAC, 30 fps, no subtitles, no chapters
- **Portico:** `-c:v libx264 -preset medium -profile:v high -level:v 4.0 -r 30 -crf 21`, `-c:a aac -b:a 192k -ac 2`, `-sn -dn -map_metadata -1`, chapters dropped, fragmented MP4 (`S/internal/decoder/prepared.go:22-48`). HDR sources are refused (`preparedmedia/worker.go:663`). Output is validated to be ≤ 30.1 fps and H.264 level ≤ 4.0 (`worker.go:679-705`).
- **Others:** Jellyfin's transcoder follows the client profile (HEVC, surround, original frame rate, subtitle burn-in or sidecar). Finamp lets the user pick codec and bitrate for music.
- **Why it matters:** A converted download of a 23.976 fps film is re-timed to 30 fps (repeated frames, visible judder); 5.1 becomes stereo; subtitles vanish because none are embedded and none are fetched separately (A-14.21); an HDR title can only be downloaded as the original.
- **Assessment:** **Portico behind** on output quality; deliberate in keeping recipes immutable and few (`preparedmedia/types.go:48-58`). The frame-rate clamp looks unintended for film content. Mostly the transcoding auditor's domain; listed here because it is the only shape a smaller download can take.

### A-14.6 Software-only, two-thread encodes, one at a time
- **Portico:** `-threads 2`, `libx264 medium`, one job per pass through a single loop (`decoder/prepared.go:23,36`, `preparedmedia/worker.go:120-157`); background jobs do not take the interactive playback slot (`decoder/prepared.go:75-77`).
- **Others:** Jellyfin uses whatever hardware encoder playback uses.
- **Why it matters:** Predictable load on a weak box (good, and consistent with "background work is deprioritized"), but a two-hour film at 1080p on a small CPU is many hours, and a season is days. There is no ETA until bytes flow, and the estimate is bitrate × duration.
- **Assessment:** **Trade-off (subjective).** Consistent with Portico's principles. Worth showing the owner an honest time estimate before a whole-show conversion is queued.

### A-14.7 Size is known and published before the user commits
- **Portico:** exact for originals and prepared rungs, `estimated: true` otherwise (`options.go:27-46`, `targets.go:96-116`).
- **Others:** Streamyfin estimates from bitrate; Plezy shows the source size; Jellyfin web shows nothing.
- **Assessment:** **Portico ahead.** Keep.

### A-14.8 Server storage is bounded by an owner ceiling counted in claims
- **Portico:** `downloads.maxPreparedBytes` (default 0 = unlimited) is checked inside the admitting transaction (`service.go:281-297`); `/v1/downloads/usage` reports per-profile and server totals. Free disk space is also checked before a snapshot copy with a 2 GiB floor (`source_snapshot.go:26, 59`, `diskspace.go`).
- **Others:** nothing comparable (they store nothing).
- **Why it matters:** The ceiling counts *claims*, and deliberately double-counts a shared prepared file (`service.go:188-191`), while snapshot copies are genuinely one per claim. The default is unlimited, so out of the box the only brake is the 2 GiB floor.
- **Assessment:** **Portico ahead** in having a limit at all; the default and the snapshot design (A-14.2) undermine it. Consider a non-zero default expressed as a share of the state volume.

### A-14.9 A device copy lives only as long as the server-side claim
- **Portico:** When a ready claim has had no grant for `retentionDays`, it becomes `expired` and its receipts are revoked (`worker.go:122-160`). Revalidation then answers `refused: revoked`, and the app deletes the file (`offline-downloads.ts:476`, `:113`). Renewing a receipt does **not** refresh the claim's clock (`receipts.go:353-473` never touches `used_ms`).
- **Everyone else:** a downloaded file stays until the user deletes it (Plex, from memory, can auto-delete *watched* items, by user choice).
- **Why it matters:** With defaults, every download disappears from the phone about 30 days after it was fetched, the next time the app talks to the server. The contract says the opposite: "Server artifact retention and client offline retention are separate" (§8 rule 7).
- **Assessment:** **Portico behind**, and I believe unintended as a product outcome even though the code and `downloads.md:261-263` describe it. See B-14.1.

### A-14.10 Offline permission is a signed, expiring receipt
- **Portico:** Ed25519-signed receipt per download, 30 days, renewed on contact, revocable per receipt, with a published revocation list (`receipts.go`). The app will not play without one (`offline-downloads.ts:508-515`).
- **Jellyfin clients, Plezy, Streamyfin, Finamp:** none. A downloaded file is a file. Removing a user on the server does nothing to copies already on their phone.
- **Plex (from memory):** downloads stop working when access is removed and the app next checks in; no public cryptographic receipt.
- **Why it matters:** The owner can actually withdraw offline access (lost device, ended share, child's profile). The price is a hard dependency: a device that cannot reach the server for 30 days loses playback, and a receipt problem makes paid-for-in-time downloads unplayable (B-14.6, B-14.16).
- **Assessment:** **Trade-off (subjective).** Deliberate and decided (P06, `contract.md` §10, §16). It is honest that this is not DRM. If kept, the failure paths around it need to be much more forgiving than they are today.

### A-14.11 The receipt is implemented differently from the contract's design
- **Portico (code):** envelope `{receiptId, keyId, payload(base64url JSON), signature, claims}`; fields `issuedAt`/`expiresAt`; signing key is a dedicated key in `download_receipt_keys`; the client pins keys by id from `GET /v1/downloads/receipt-keys` (`receipts.go:45-72, 121-143`; `offline-downloads.ts:484-494`).
- **Portico (contract §10.1):** fields `lastVerifiedAt`/`verifyBy`, the server's *identity* key with a pinned fingerprint, RFC 8785 canonical JSON, `authorizationRevision`, media-version id, four named revalidation outcomes.
- **Assessment:** **Unclear** which is intended to win. The implemented envelope is simpler and avoids canonicalisation pitfalls (I think it is the better wire design). The contract's insistence on the pinned identity key is stronger against a swapped server. Recorded under Doc/code disagreements.

### A-14.12 Every state change is fenced and idempotent
- **Portico:** `operationId` + `expectedRevision` on every action; conflicts return the current row (`preparations.go:211-312`). Batches report per-item rejections with reason codes.
- **Others:** none (single-client state).
- **Why it matters:** Robust against double taps and retries. But the same `revision` is also bumped by the worker for every 32 MiB hashed and every pass over a converting claim (`worker.go:546`, `:447`), so it moves several times a second while work is in progress (B-14.13).
- **Assessment:** **Portico ahead** in intent; the progress counter and the fence should be two numbers.

### A-14.13 Transfer permission is a 10-minute capability address
- **Portico:** token in the path, 10 minutes to first use, 10 minutes of reuse after first use (`types.go:103-108`); nothing else is checked on the transfer route except that the viewer still may see and download the title (`grants.go:174-184`).
- **Jellyfin / Plezy / Streamyfin:** the normal long-lived access token (header or `api_key` in the query) on a stable URL.
- **Why it matters:** A leaked Portico address is worth ten minutes; a leaked Jellyfin URL is worth the session. But background download systems (iOS `URLSession`, Android `DownloadManager`/WorkManager, browsers) all assume the URL stays valid across pauses, network changes and retries. A ten-minute address fights every one of them (B-14.2, B-14.3).
- **Assessment:** **Trade-off**, currently hurting. Deliberate (`contract.md` §8 rule 6, §14). The security goal can be kept with a longer-lived, download-scoped credential bound to the claim and device (hours or days, revocable with the claim), or by letting an expired grant be *renewed* for the same artifact so partial bytes stay valid (the contract's own CF36: "Renew only exact artifact under current owner").

### A-14.14 The server can resume; the apps do not
- **Portico server:** `Accept-Ranges`, strong `ETag` from the content hash, `If-Range` (`downloads_routes.go:220-228`).
- **Portico apps:** both platforms throw away the partial file whenever the grant address differs from the one the partial was fetched with (`PorticoDownloads.m:288-305`; `PorticoDownloadCenter.kt:138-141`), and JavaScript always asks for a brand-new grant before continuing (`offline-downloads.ts:412`, `CC/downloads-service.ts:212-214`). So "Continue" restarts from zero.
- **Plezy:** native pause/resume and automatic rescheduling (`download_manager_service.dart:973-1100, 3480-3530`). **jellyfin-android:** `ContentRange` resume in a WorkManager job.
- **Assessment:** **Portico behind.** The artifact's identity is its SHA-256, not its URL; a partial should be continued with `Range` + `If-Range: "sha256-…"` under any grant for the same claim. See B-14.2.

### A-14.15 Background transfer machinery
- **Portico iOS:** background `URLSession`, `discretionary = NO`, 2 connections per host, cellular decided per request (`PorticoDownloads.m:129-152`).
- **Portico Android:** plain OkHttp calls on an in-process thread pool, kept alive by a `dataSync` foreground service with a fixed "download in progress" notification (`PorticoDownloadCenter.kt:54, 350-384`). No WorkManager, no system DownloadManager. A transfer left running when the process dies is shown as paused on next launch (`:82`).
- **Plezy:** `background_downloader` (WorkManager on Android) with a holding queue, free-space check, killed-task rescheduling and a warning banner for phones whose makers kill background work. **Streamyfin:** system `DownloadManager`. **jellyfin-android:** WorkManager with network constraints.
- **Why it matters:** On Android, Portico downloads stop when the process is killed (swiped away, low memory, aggressive battery savers) and nothing restarts them. Foreground `dataSync` services are also time-limited on recent Android versions (B-14.20).
- **Assessment:** **Portico behind** on Android; roughly level on iOS apart from A-14.13/14. Medium work: WorkManager (or `DownloadManager`) plus the resume fix.

### A-14.16 No automatic retry, no "waiting" states
- **Portico:** a network error ends the attempt as `failed: network`; the user taps "Try again" (`PorticoDownloads.m:236-239`; `PorticoDownloadCenter.kt:277-286`; `N/src/screens/Downloads.tsx:44`). On Android, moving to cellular with Wi-Fi-only pauses everything and nothing resumes when Wi-Fi returns (`PorticoDownloadCenter.kt:86-90`). The app's own states are just `downloading | paused | failed | ready` (`offline-downloads.ts:18`).
- **Contract:** `waiting-for-server`, `queued`, `transferring`, `verifying`, `waiting-for-wifi`, `waiting-for-storage`, `server-unreachable`, `failed-retryable`, `failed-terminal`, `authorization-unverified`, `deleting` (§9).
- **Plezy:** app-level auto-retry with a retry count, requeue on system cancel, a storage-full circuit breaker (`…:2683-2860`).
- **Assessment:** **Portico behind.** Medium.

### A-14.17 End-to-end integrity check
- **Portico:** the server publishes a SHA-256; both apps hash the finished file and delete it on mismatch (`PorticoDownloads.m:183-190`; `PorticoDownloadCenter.kt:288-299`). The partial is written to a private temporary place and promoted by rename.
- **Others:** none verify content; they trust HTTP length.
- **Why it matters:** Catches truncation and corruption. Costs one full read of the file on the phone after download, and (today) two full reads plus a copy on the server.
- **Assessment:** **Portico ahead.** Keep the client check. The verification after a background completion on iOS has a timing risk (B-14.18).

### A-14.18 Server-wide and per-device concurrency
- **Portico:** transfer route in a "bulk-transfer" lane of 16 concurrent requests with no queueing (`httpapi/admission.go:77`); 2 at a time per device (`PorticoDownloads.m:141`, `PorticoDownloadCenter.kt:54`); up to 200 *running* preparations per viewer but one worker processing 8 per pass (`types.go:98-101`, `worker.go:76`).
- **Plezy:** one download at a time ("Plex servers can also reject concurrent media downloads").
- **Assessment:** **Trade-off.** No per-user or per-device transfer cap and no bandwidth cap for downloads exist on the server; the lane protects the box. Fine for now.

### A-14.19 Where files live on the device
- **Portico:** fixed app-private location (`Application Support/Downloads` on iOS, `noBackupFilesDir/Downloads` on Android), excluded from backups, one folder per claim, file named `<claim id>.<ext>` (`PorticoDownloads.m:73-83`; `PorticoDownloadCenter.kt:93-95`; `targets.go:151`). No SD card, no user folder, nothing visible in the Files app.
- **Plezy:** app folder or a user-chosen folder (including SD card) with readable show/season/file names. **Finamp:** several named download locations. **Streamyfin:** app documents.
- **Why it matters:** Portico's choice is the safe one for the receipt model (files outside the app could be played by anything). It rules out the two things Android users ask for most: SD-card storage and opening the file in another player. The contract allows one removable root on Android (§13).
- **Assessment:** **Trade-off (subjective).** Deliberate (§14). An SD-card *app-specific* directory (`getExternalFilesDirs`) would keep privacy and add capacity: small-to-medium.

### A-14.20 The device catalogue is two JSON documents
- **Portico:** `manifest.json` (entries: id, itemId, title, subtitle, quality, artwork **URL**, sizes, state, and a `meta` string holding library id, kind, owner and the receipt) rewritten whole on every change (`PorticoDownloads.m:112-117`), and `state.json` (settings, unsent progress, resume points, keys, wants) capped at 1 MiB (`:418-427`).
- **Plezy:** Drift/SQLite with eight download-related tables. **Finamp:** Isar node graph. **Streamyfin:** MMKV holding whole item records.
- **Why it matters:** Fine for tens of downloads. With hundreds (a music library, several shows) each receipt renewal rewrites the whole manifest once per entry (B-14.9), the unsent-progress and wants maps share one 1 MiB document, and nothing is queryable. The contract deliberately wants a *minimal* catalogue (§9 rule 5), which this is.
- **Assessment:** **Portico behind** for scale, **deliberate** for minimalism. If offline browsing grows (F-14.6, F-14.8), a small SQLite file on the device is the natural step.

### A-14.21 Only the media file travels
- **Portico:** no sidecar subtitles, artwork files, chapter list, intro/credit markers, trickplay images, or descriptive metadata are stored with a download. Artwork is a remembered URL (`offline-downloads.ts:414-420`). Converted files have subtitles and chapters stripped (A-14.5).
- **Plezy:** external subtitles, poster/logo/backdrop, chapter thumbnails, pinned metadata. **Streamyfin:** all of that plus trickplay and skip segments. **Finamp:** images and full item metadata.
- **Why it matters:** Offline, a Portico download plays with whatever is embedded in the file and shows whatever image the OS cache still holds. A film whose subtitles are a separate `.srt` has no subtitles on the plane.
- **Assessment:** **Portico behind.** The single most visible functional gap. See F-14.4 to F-14.7.

### A-14.22 Titles are remembered only for single-title requests
- **Portico:** a single title keeps the name, year and artwork URL the screen had when the user tapped Download; members of a season/show/album request and auto-next episodes are stored with an empty title and fall back to the file name, which is the claim id (`offline-downloads.ts:289-302, 338, 377`; `N/src/app/downloads.tsx:92-94`). The server's preparation record carries no title at all (`types.go:148-166`), which is why the web list also shows ids (`W/screens/downloads/Downloads.tsx:32-34`).
- **Others:** every download carries its item record.
- **Assessment:** **Portico behind**; acknowledged in code comments ("deferred-to-contract"). See B-14.7. Small on the server (publish title/parent/number/artwork ref on the preparation), small on the clients.

### A-14.23 Offline browsing is one flat list, reached from an error screen
- **Portico:** with no server, the launch flow shows "server unavailable" with a "Watch downloads" button that opens a flat list (`N/src/screens/auth/SavedServerUnavailable.tsx:68`, `OfflineDownloads.tsx`). No grouping by show or album, no search, no sort, no title page. The list is not virtualised (`N/src/screens/Downloads.tsx:61-80`).
- **Plezy:** a real offline mode: the normal app, limited to downloaded items, with a tree by library → show → season, collections, sort and filter, and working title pages from pinned metadata. **Streamyfin:** downloads grouped as movies / series → seasons. **Finamp:** an offline switch that filters the whole app.
- **Assessment:** **Portico behind.** Conflicts mildly with "thin clients: the server composes rows" — offline there is no server, so some client-side composition is unavoidable. Large if done as a true offline shell; medium as a grouped, searchable downloads screen.

### A-14.24 Two different players for the same file
- **Portico:** signed in, a download plays in Portico's player as a bare local source; from the launch flow it plays in the system player (`AVPlayerViewController` / a plain Android video view) because Portico's player is not mounted there (`N/src/screens/Downloads.tsx:33-36`; `PorticoDownloads.m:448-483`).
- **Others:** one player everywhere.
- **Why it matters:** In the truly-offline case — the one downloads exist for — the user gets the lesser player: no Portico subtitle rendering, no skip, no next episode.
- **Assessment:** **Portico behind.** Mount the player in the offline flow; medium.

### A-14.25 A downloaded copy is used only from the Downloads screen
- **Portico:** the title page has no "Downloaded" state and its Play button always streams (`N/src/screens/Detail.tsx` has no reference to the offline store; `PlayerHost.tsx:47-50` is the only local entry point).
- **Plezy:** `resolveDownloadedVideoSource` plays the local file when the downloaded version matches the requested one, online or offline, and shows download status icons on items (`plezy/lib/services/downloaded_video_source.dart:16-35`, `widgets/download_status_icon.dart`). **Streamyfin, Finamp:** same idea.
- **Why it matters:** On mobile data a user who downloaded a film to save data will stream it anyway unless they remember to start it from Downloads.
- **Assessment:** **Portico behind.** Small-to-medium in the app.

### A-14.26 Offline playback has no queue
- **Portico:** "no queue, no server session, no network" (`N/src/bridge/local-playback.ts:19-23`). One file plays and stops.
- **Streamyfin:** next/previous from the downloaded set. **Plezy, Finamp:** full local queues.
- **Why it matters:** A downloaded season does not advance to the next episode; a downloaded album plays one track. For music and multi-file audiobooks this makes offline listening impractical.
- **Assessment:** **Portico behind.** Medium (needs the ordering metadata of F-14.6).

### A-14.27 What is recorded while offline
- **Portico:** one record per title — the latest position, plus `watched: true` when playback ended or passed a hard-coded 92% (`offline-downloads.ts:546-568`). Nothing else: no explicit mark watched/unwatched, no rating, no My List change, no listening time.
- **Contract:** an outbox of operations with a device sequence and base revision, including explicit actions (§4.4, §9 rule 6, §10.3).
- **Plezy:** a queue of `progress` / `watched` / `unwatched` actions using the *server's* watched threshold. **Audiobookshelf:** complete listening sessions. **Finamp:** a log of listens.
- **Assessment:** **Portico behind** its own contract; the simplification is workable for video but loses history (B-14.10).

### A-14.28 How offline progress is merged
- **Portico:** newest device timestamp wins, compared with the last offline observation accepted for that title and with the last *online* activity; each entry gets an explicit outcome and the server's current value (`progress.go:136-183`). A timestamp more than 24 h in the future is refused.
- **Contract / amendments:** "Offline event ordering is monotonic by sequence, not maximum position… not client timestamps as authority" (`architecture-amendments.md:71`; decision R09 marks this "Future offline integration").
- **Streamyfin:** newest `LastPlayedDate` wins, either direction. **Plezy:** push everything, server decides; then pull. **Audiobookshelf:** session newer than progress wins.
- **Assessment:** **Trade-off.** Portico's per-entry outcomes are the best-designed reply of the group. Timestamps are what everyone uses in practice; the contract's sequence model is stricter than anyone ships. Decide which is the plan and align the document.

### A-14.29 Offline progress is written straight into the tables, bypassing the online rules
- **Portico:** `applyProgress` upserts `progress`, `progress_activity`, optionally `personal_watched_intents` and `personal_items` (`progress.go:201-229`). The online path does much more: start and completion thresholds, credits-based completion, play history and play counts, the viewer's watch history, explicit-action fences, "tracks keep no resume point", clearing the position on completion (`S/internal/catalog/personal_activity.go:60-143`).
- **Others:** replay through the same endpoints as live playback (Plezy uses the ordinary scrobble/timeline calls).
- **Assessment:** **Portico behind.** `downloads.md:230-231` claims "There is no second kind of progress", but there is. See B-14.10 and B-14.11.

### A-14.30 Server state is never pulled down to a downloaded copy
- **Portico:** the resume point used for a download is the device's own map (`offline-downloads.ts:522, 532`); the server's answer to a progress sync is parsed and discarded (`:586`).
- **Plezy / Streamyfin:** pull watch state for downloaded items when online.
- **Assessment:** **Portico behind.** Small: seed/refresh the local resume map from the progress reply or from a per-item read at revalidation time.

### A-14.31 Ownership on a shared device
- **Portico:** each entry belongs to one `(authority, account, profile, server)` taken from its signed receipt; other viewers neither see nor can play it; unsent progress is filed per owner and only ever sent as that owner (`offline-downloads.ts:64-68, 147-177, 546-594`). Physical files are per claim, so two profiles downloading the same film store it twice.
- **Plezy:** one physical file shared across profiles with an owners table.
- **Assessment:** **Portico ahead** on isolation (and its tests encode a real past bug: one profile's renewal deleting another's files, `:106-113`). Deduplication is an optimisation the contract already permits (§3 rule 4).

### A-14.32 Sign-out keeps the files
- **Portico (code):** "Signing out withdraws the offline list too: the files stay, but nobody is named as their owner until that viewer signs in again" (`N/src/app/session.tsx:1007-1009`).
- **Portico (contract):** "Sign out removes the account's local protected downloads and scoped state" (§3 rule 6, §11).
- **Plezy:** keeps downloads across logout and carefully carries their metadata over.
- **Assessment:** **Unclear** which Justin wants; the code's behaviour is friendlier, the contract's is tidier. Today's middle ground leaks storage (B-14.14).

### A-14.33 Who may download
- **Portico:** a per-profile `allowDownloads` restriction, re-checked at claim, grant, transfer, receipt and renewal (`policy.go:14-30`; five call sites). Conversions: owner only. Library and content-rating visibility are re-checked each time, and a title that became invisible answers exactly like a missing one (`preparations.go:78-89`).
- **Jellyfin:** `EnableContentDownloading` per user, checked once; plus `EnableSyncTranscoding`. No per-library download switch.
- **Assessment:** **Portico ahead** in rigour. Missing: per-library "no downloads", per-profile count/byte limits, a cap on simultaneous transfers per user, and an owner view of who holds what (the data exists: `Usage` with `scope=server`, `service.go:219-276`).

### A-14.34 Whole-container requests are expanded by the server
- **Portico:** a durable request row; the worker captures members 20 at a time in keyset order under a "fence" (a digest of the library and viewer revisions) and then admits them (`requests.go:284-455`). No size limit.
- **Others:** the client enumerates episodes and queues each one.
- **Why it matters:** Matches Portico's scale rule (bounded work per step, no client-side library walks). The fence makes capture all-or-nothing against any change (B-14.17).
- **Assessment:** **Portico ahead** in design; the fence is too strict.

### A-14.35 Automatic downloads are one-shot
- **Portico:** the policy `next N` selects N unwatched episodes once (`requests.go:98-111, 359-366`). Separately the app, when a downloaded episode *finishes playing on that device*, asks for the one after it (`offline-downloads.ts:359-384`). Nothing re-evaluates later: new episodes of a show are not fetched, watched ones are not replaced unless played to the end on this device, playlists are not mirrored.
- **Plezy:** persistent sync rules (A above). **Plex / Netflix (from memory):** standing rules.
- **Assessment:** **Portico behind.** See F-14.11. The server already has every ingredient (durable requests, `nextAfterMediaId`, unwatched filter).

### A-14.36 Network and power rules
- **Portico:** one switch, Wi-Fi only, default on; applied per request on iOS and by pausing on Android (`PorticoDownloads.m:147-152, 380-407`; `PorticoDownloadCenter.kt:86-90, 205-210`).
- **jellyfin-android:** Wi-Fi only / mobile / mobile + roaming as WorkManager constraints. **Plezy:** Wi-Fi-only re-applied to already queued tasks.
- **Assessment:** **Roughly level**, minus roaming and charging-only. Small.

### A-14.37 Device storage rules
- **Portico:** a storage limit (none, 5–100 GB), 500 MiB of headroom, in-flight bytes counted before starting more (`offline-downloads.ts:386-402`); "remove after watching" acts when playback reaches the natural end (`:563-567`). No automatic eviction.
- **Plezy:** native free-space check per task, storage-full circuit breaker. **Plex (from memory):** remove watched.
- **Assessment:** **Roughly level.** The limit and usage figure are device-wide across profiles while the list is per profile (B-14.14).

### A-14.38 No notifications; polling instead of push
- **Portico:** no completion notifications by rule (`PorticoDownloads.m:209`; `N/src/app/download-toasts.ts`), except Android's mandatory foreground-service notice. The app learns that a server-side preparation finished only by polling every 5 s while in the foreground (`CC/downloads-service.ts:83-124`; `N/src/app/downloads.tsx:25-30`).
- **Plezy / Streamyfin:** notifications, Live Activity.
- **Why it matters:** A conversion that takes an hour completes on the server, but the transfer only starts when the user next opens the app.
- **Assessment:** **Deliberate** (recorded decision: no push notifications). The consequence for long conversions is worth stating in the UI ("open Portico again to fetch it").

### A-14.39 Web is "save a file", two steps, opaque name
- **Portico:** request → wait for `ready` → "Save" navigates to the grant; `Content-Disposition` is `<claim id>.<ext>` (`downloads_routes.go:223`, `targets.go:118-152`). No service worker, no offline playback in the browser (deliberate, §13).
- **Jellyfin web:** immediate download with the original file name. **Navidrome:** zip of an album or playlist with tidy names.
- **Assessment:** **Portico behind** for the simple "give me the file" case. Small: a readable download file name; larger: an immediate path for originals (A-14.2).

### A-14.40 Platforms covered
- **Portico:** phones and tablets only; no TV (deliberate, §13), no desktop app yet (planned in `Backlog.md`), no Vega.
- **Plezy:** phones, tablets and desktop (not tvOS). **Finamp:** mobile and desktop.
- **Assessment:** **Deliberate.** When the desktop companion arrives, laptops are a prime offline device.

### A-14.41 Items made of several files, or with several versions
- **Portico:** refused: `Parts != 1` → `source_unavailable` (`targets.go:66-78`). The count is of *all* asset links of the title, so a title with two versions is refused the same way.
- **Plezy:** downloads every part of a stacked item and lets the user pick the version (`download_manager_service.dart:2187-2420`, `utils/download_version_utils.dart`).
- **Assessment:** **Portico behind**; also a latent trap for the planned "second file becomes a second version" work in `Backlog.md` (B-14.26).

### A-14.42 Music and audiobooks
- **Portico:** per track or per file, original or one AAC 192 kbps recipe; albums, books and playlists through container requests. No offline queue, no gapless, no playlist structure offline.
- **Finamp:** offline-first: collections as first-class downloads that re-sync, chosen codec/bitrate, offline listens.
- **Assessment:** **Portico behind**; music offline is effectively not usable yet (A-14.26). Large to do properly.

### A-14.43 Audit trail
- **Portico:** audit rows for prepare, each action, revoke and settings (`preparations.go:183, 302`; `receipts.go:527`). Nothing records the transfer itself.
- **Jellyfin:** an activity-log entry per download naming user, item, client and device.
- **Assessment:** **Roughly level**; a line for "transferred to device X" would complete it. Small.

## Missing features and capabilities to consider

Sizes: **small** = days in one place; **medium** = a week or two across server and app; **large** = a new subsystem.

### F-14.1 A light path for original-quality downloads
Serve the library file directly (hash once in place, refuse the transfer if size or modified time changes), or keep one shared, content-addressed snapshot per asset instead of one per claim. Who has it: everyone else serves the file directly. Value: originals start in seconds instead of after a full copy; a weak box does not need the film's size free on its state volume. Size: medium, server only. Conflict: none with the principles; it serves "weak hardware". Opinion: the highest-value server change in this domain.

### F-14.2 True resume of an interrupted transfer
Continue a partial file with `Range` + `If-Range: "sha256-…"` under any grant for the same claim; treat the hash, not the URL, as the file's identity. Who: Plezy (native resume), jellyfin-android (`ContentRange`). Value: large files over poor links actually finish. Size: small on Android (the code path exists, `PorticoDownloadCenter.kt:241-257`), medium on iOS (a background download task cannot be re-pointed at a new URL; options are a longer-lived download credential or app-managed ranged data tasks). Opinion: must-have before release.

### F-14.3 Automatic retry and honest waiting states
Retry with backoff on network loss; hold `waiting for Wi-Fi`, `waiting for server`, `waiting for space` instead of `failed`; resume by itself when the condition clears. Who: Plezy, system download managers. Size: medium. The contract already names these states (§9).

### F-14.4 Subtitles with the download
Fetch the title's sidecar subtitle files (and, for converted files, the embedded text tracks extracted by the server) and offer them in the offline player. Who: Plezy, Streamyfin. Size: medium (server: list subtitle artifacts with the claim; app: store and wire into the player). Opinion: without this a large share of libraries are unwatchable offline for viewers who need subtitles.

### F-14.5 Artwork with the download
Store the poster (and for episodes the show poster and still) as files beside the media. Who: Plezy (hash-named, shared across items), Streamyfin, Finamp. Size: small.

### F-14.6 A metadata snapshot with the download
Title, show/season/episode numbers, year, runtime, overview, track/disc numbers, album and artist, plus the ordering needed for "next". Who: all three open-source clients (Streamyfin stores the whole item record). Size: small on the server (publish it on the preparation or the receipt response), small on the app. This is the missing piece behind B-14.7, A-14.23 and A-14.26.

### F-14.7 Chapters, intro/credit markers and trickplay offline
Who: Streamyfin (segments and trickplay sheets), Plezy (chapter thumbnails). Size: medium. Opinion: after F-14.4–F-14.6.

### F-14.8 A real offline library view
Group downloads by show → season, album, book; search and sort; an offline banner; reach it from the normal shell rather than only from the "server unavailable" screen. Who: Plezy (tree view, filters, collections), Streamyfin (series grouping), Finamp (offline switch). Size: medium for a grouped Downloads screen; large for a full offline shell. Tension with "thin clients" noted in A-14.23.

### F-14.9 Offline queue: next episode, album and audiobook playback
Who: Streamyfin, Plezy, Finamp. Size: medium, depends on F-14.6.

### F-14.10 Use the downloaded copy when online, and show download state on titles
A "Downloaded" badge on cards and title pages; Play uses the local file when the downloaded quality is what would be played. Who: Plezy (strict version matching), Streamyfin, Finamp. Size: small-to-medium.

### F-14.11 Standing download rules
"Keep the next N unwatched episodes of this show", "download new episodes", "keep My List / this playlist downloaded", re-evaluated when the app is online, with removal of watched episodes. Who: Plezy sync rules; Plex and Netflix (from memory). Size: medium — the server already expands containers durably and filters unwatched; what is missing is a stored rule and a trigger on reconnect. No conflict with "no periodic chatter" if evaluation is tied to app foreground/connect.

### F-14.12 Offline actions outbox
Mark watched/unwatched, My List, rating and reaction while offline, sent later with the conflict rules the contract already specifies (§4.4). Who: Plezy (watched/unwatched). Size: medium.

### F-14.13 Pull newer server state to downloaded items
So a film half-watched on the TV resumes at the right place on the plane. Who: Plezy, Streamyfin. Size: small.

### F-14.14 Device-aware options and a lossless compatibility remux
Tell the user which options this device can play; offer "Original (repackaged for this device)" when only the container is the problem. Who: Streamyfin via device profiles. Size: medium. The contract already requires disclosure of such a remux (§7).

### F-14.15 Let chosen non-owners request conversions; faster recipes
A per-profile permission (Jellyfin's `EnableSyncTranscoding`), plus recipes that keep the source frame rate, optionally keep 5.1, and use a hardware encoder when playback has one configured. Size: small for the permission; medium for recipes (new immutable recipe ids). Tension: "server load never lowers quality" is untouched; "owner choice" argues for the switch.

### F-14.16 Storage location and export
App-specific SD-card directory on Android; optionally "export a copy" to Files/another app as an explicit action for viewers the owner allows. Who: Plezy (any folder), Finamp (several locations). Size: small for SD; export conflicts with the receipt model and needs a decision (contract §14 puts share/export out of scope).

### F-14.17 More network and power rules
Allow on mobile data but not roaming; only while charging; a per-download "use mobile data this once". Who: jellyfin-android (roaming), Netflix (from memory). Size: small.

### F-14.18 Queue management
Reorder, pause all / resume all, per-item speed and ETA on the device, an iOS Live Activity. Who: Plezy (priority column, holding queue), Streamyfin (Live Activity, speed calculator). Size: small-to-medium. The no-notifications rule would need an explicit view on Live Activities.

### F-14.19 Storage management view on the device
Size per show/album, "remove all watched", bulk select, and a way to see and remove files left by another profile or a signed-out account. Size: small. Fixes B-14.14.

### F-14.20 Owner controls and visibility
A dashboard of who holds which downloads on which device, per-profile limits (count or bytes), a per-library "no downloads" switch, "remove this profile's downloads from device X" (the revoke API already supports it, `receipts.go:475-534`). Who: Plex (from memory). Size: small-to-medium; mostly UI over existing data.

### F-14.21 Readable file names and bundle downloads on the web
`Content-Disposition` with "Title (Year).ext" / "Show - S01E02 - Title.ext"; zip of an album or season. Who: Jellyfin (original name), Navidrome (zip with names and cover). Size: small for names; medium for zips.

### F-14.22 Repair and verify
A "check downloads" action that re-verifies files, re-fetches missing extras and clears stuck entries; start-up reconciliation of native tasks with the catalogue. Who: Finamp (repair), Plezy (several repair passes). Size: small-to-medium. The amendments already call for launch reconciliation (`architecture-amendments.md:79`).

### F-14.23 Multi-file and multi-version titles
Download every part of a stacked title; choose which version. Who: Plezy. Size: medium.

### F-14.24 Offline play history
Record offline plays in the server's play history, play counts and the viewer's watch history, with the real watched time. Who: Audiobookshelf (sessions), Finamp (listens). Size: small once B-14.10 is fixed by routing offline observations through the same code as online ones.

### F-14.25 Start the transfer without reopening the app
Without push notifications the realistic options are: an iOS background-refresh task or Android WorkManager job that checks "is my claim ready?" occasionally while claims are pending, or issuing the grant-equivalent up front so the OS downloader can simply retry until the server says ready. Size: medium. Needs a view on the "no periodic chatter" spirit (this is device-to-own-server, only while something is pending).

### F-14.26 Desktop and laptop downloads
Planned desktop companion (`Backlog.md`, "Planned features") should include downloads; laptops are the classic travel device. Size: large (new client).

## Possible bugs and risks in Portico

None of these were reproduced. "Confirmed by reading" means the code path was traced end to end and I am confident of what the source does; it still deserves a runtime check before any fix. None is listed in `Backlog.md` unless stated.

### B-14.1 Downloads delete themselves from the device after the server's retention period
- **What happens:** A claim that is `ready` and has had no grant for `retentionDays` (default 30) is expired and its receipts are revoked (`S/internal/downloads/worker.go:134-160`; `receipts.go:550-578`). Only `IssueGrant` and `OpenGrant` move the clock (`grants.go:104, 201`); renewing a receipt does not (`receipts.go:449-451`). On the app's next connection, renewal answers `refused` with code `revoked` (`receipts.go:405-408`), and the app removes the file (`N/src/bridge/offline-downloads.ts:476`, with `revokesOwnedFile` true for every code except `unknown_receipt`, `:113`).
- **Effect:** A film downloaded on 1 September is deleted from the phone the first time the app is opened online after 1 October, even if the user opened the app daily. The server test `TestRetentionExpiresAnUnusedReadyClaim` asserts the revocation (`downloads_test.go:591-616`), so the server half is intended; the product result contradicts `contract.md` §8 rule 7 and §16.
- **How the comparison surfaced it:** no other product ties a device copy's life to a server-side record.
- **Confidence:** Confirmed by reading. **Severity:** High. **Backlog:** no.
- **Direction:** separate "the server no longer keeps transferable bytes" (stop granting; delete the snapshot) from "the viewer may no longer play their copy" (revoke). Retention should do only the first; or let each successful receipt renewal refresh the claim.

### B-14.2 "Continue" and "Try again" restart from byte zero on both platforms
- **What happens:** JavaScript obtains a new grant for every start (`offline-downloads.ts:412`; a fresh random `operationId` each time, `CC/downloads-service.ts:213`), so the address always changes. iOS keeps resume data only if the stored `grantUrl` equals the new address, otherwise deletes it (`N/Native/PorticoDownloads.m:295-305`). Android deletes the partial file under the same condition (`PorticoDownloadCenter.kt:138-141`), so its `Range` code (`:241-257`) never runs with an offset.
- **Effect:** Any pause, network drop, app kill or Wi-Fi-only suspension throws away what was fetched. The user-facing text says the opposite: "The connection dropped. It will continue when you try again." (`offline-downloads.ts:101`) and the comment "continues from where it stopped" (`:422-423`).
- **Confidence:** Confirmed by reading. **Severity:** High for large files and unreliable links. **Backlog:** no.

### B-14.3 Any pause longer than ten minutes inside the OS downloader fails the transfer
- **What happens:** A grant is refused once more than 10 minutes have passed since issue without a first use, or since first use (`grants.go:150-155`). iOS background sessions pause and resume transfers on their own (connectivity loss, Wi-Fi-only requests waiting for Wi-Fi, system scheduling) and resume by re-requesting the **same URL** with a range. After the window the server answers 401; the app records `failed: grant_expired` and discards the partial (`PorticoDownloads.m:175-181`).
- **Effect:** (a) With Wi-Fi-only on (the default) and the phone on mobile data, a download waits for Wi-Fi inside iOS; if Wi-Fi arrives more than 10 minutes later the download fails instead of starting. (b) A long transfer that loses the network once fails and restarts from zero (B-14.2). (c) A phone moving between the home network and a remote address mid-download holds a URL for the old address.
- **Confidence:** Probable (server side confirmed; the iOS behaviours are documented platform behaviour, not observed here). **Severity:** High. **Backlog:** no.

### B-14.4 Four of the six offered quality rungs can never be produced
- **What happens:** `download-options` offers every ladder rung at or below the source height (`options.go:162-198`) and maps each to `portable-{360,480,720,1080,1440,2160}-v3` (`targets.go:87-92`; recipes in `preparedmedia/types.go:51-56`). The encoder refuses every recipe except 1280×720 at 2500 kbps and 1920×1080 at 5000 kbps, and any recipe whose audio is not 192 kbps (`S/internal/decoder/prepared.go:27-35`). The 360p and 480p recipes use 96 kbps audio; 1440p and 2160p fail the size test. The output validator additionally caps H.264 level at 4.0 (`preparedmedia/worker.go:701`), which 1440p/2160p could not meet anyway, and the client parser rejects prepared versions wider than 1920 (`CC/prepared-media.ts`, `parsePreparedVersion`).
- **Effect:** An owner who picks 480p (the natural phone choice) or 2160p gets a claim that runs and then fails with `optimization_failed`.
- **Confidence:** Confirmed by reading (I did not find another argument builder; `RunPreparedFileEncode` calls `preparedArgs`, `decoder/prepared_file.go:89`). **Severity:** High for the feature, contained (nothing is damaged). **Backlog:** no.

### B-14.5 Original downloads: full copy, double read, per viewer, serial, on the state volume
- **What happens:** see A-14.2. Additional details: the copy phase is not checkpointed — a restart or a pause during it discards the temporary file and starts again (`source_snapshot.go:38-57`), although `downloads.md:96-98` promises "a server restart costs one window". The copy holds `snapshotMu` and runs under `advanceMu` (`worker.go:65-66`), so one large original stalls every other claim, container request and expiry until it finishes. With `maxPreparedBytes` unlimited by default, the only stop is the 2 GiB free-space floor.
- **Confidence:** Confirmed by reading. **Severity:** High as a design risk on small servers (disk exhaustion of the volume that holds the database is exactly what `diskspace.go` was written to prevent). **Backlog:** no.

### B-14.6 A download that finishes while the app is not running online cannot be played offline
- **What happens:** The receipt is requested only after the file is `ready` on the device, and only while JavaScript is running with a server connection (`offline-downloads.ts:262-268`, `:243-247`, `:436-446`). Without a verified receipt the entry says "Connect to your server once to finish setting up this download" (`:511-512`).
- **Effect:** Queue downloads at night, lock the phone, iOS finishes them in the background, board a plane: every title is present and unplayable. The server could issue the receipt as soon as the claim is `ready` — it knows the hash before any byte moves (`receipts.go:289`) — so the app could hold the receipt before the transfer starts.
- **Confidence:** Confirmed by reading for the control flow; whether JavaScript gets a chance to run during an iOS background relaunch was not established. **Severity:** High (it defeats the main use case). **Backlog:** no.

### B-14.7 Season, show, album, playlist and auto-next downloads are listed by an internal id, without artwork
- **What happens:** Only a single-title request stores a display name and artwork URL (`offline-downloads.ts:294`). Container members and auto-next items are stored with `title: ''` (`:338`, `:377`) and `display()` falls back to the file name minus extension (`:301`), which the server sets to the claim id (`targets.go:151`). The web list does the same for every row (`W/screens/downloads/Downloads.tsx:34`) and a browser saves the file under that id.
- **Confidence:** Confirmed by reading. **Severity:** Medium (the feature works but is unusable to navigate). **Backlog:** no; the code comments call it deferred.

### B-14.8 Following a whole-show request is lost when the access token renews or the app restarts
- **What happens:** The list of container requests "started on this device" lives only in memory inside `DownloadsService` (`CC/downloads-service.ts:31-35`). The native provider builds a new service whenever `session.accessToken` changes (`N/src/app/downloads.tsx:19-20, 30`), and `rotate()` publishes a new session object on every renewal (`apps/portico-react-native/packages/client-core/src/index.ts:535-541`); access tokens last 15 minutes (`N/src/app/session.tsx:313`). Members are added to the device's "wanted" list only while the request is being followed (`offline-downloads.ts:328-354`).
- **Effect:** For a large show whose capture and preparation take longer than the time to the next token renewal — or if the app is closed — the remaining episodes are prepared on the server and never fetched; they appear under "On the server" to be tapped one by one.
- **Confidence:** Confirmed by reading for the mechanism; Probable for how often it bites. **Severity:** Medium. **Backlog:** no.

### B-14.9 Every token renewal re-runs the whole offline handshake
- **What happens:** The same effect re-runs `offlineDownloads.connect` (`N/src/app/downloads.tsx:22-31`): fetch receipt keys, renew **every** receipt (a write transaction on the server per 100), re-send progress, re-collect (`offline-downloads.ts:223-249`). Each renewed receipt is stored with `setMeta`, which rewrites the entire manifest (`PorticoDownloads.m:354-369`) and triggers `verifyAll`.
- **Effect:** Roughly every 13–15 minutes of foreground use: N receipt renewals, N full-manifest writes of N entries, N signature operations on the server. With a few hundred downloads that is hundreds of megabytes of flash writes per renewal cycle. It also contradicts the comment on `rotate()` ("no new generation, so nothing viewer-scoped restarts").
- **Confidence:** Probable (mechanism confirmed; cost depends on library size). **Severity:** Medium. **Backlog:** no.

### B-14.10 Offline viewing is recorded differently from online viewing
- **What happens** (`S/internal/downloads/progress.go:201-229` versus `S/internal/catalog/personal_activity.go:60-143`):
  - no row in the permanent play history, no play count, no entry in the viewer's watch history;
  - the viewer's start threshold is ignored: a 3-second peek offline becomes a resume point and a Continue Watching entry;
  - completion uses the app's fixed 92% (`offline-downloads.ts:548`) rather than the viewer's setting or credits markers;
  - on `watched: true` the stored position is left at the end instead of being cleared (the app sends the end position, `:554`);
  - songs get resume positions (online never stores one for a track, `personal_activity.go:126-128`);
  - `progress.playback_id` keeps whatever online session wrote last, so later online logic keyed on it (dismissals, canonical-writer checks) sees stale ownership;
  - explicit-action fences are not consulted; only `progress_activity.updated_at` is compared, so an online *Mark watched/unwatched* that did not touch that row can be overwritten by an older offline observation;
  - a position-only observation writes no `personal_items` row, so `viewer_revisions` is not bumped by the triggers that key on that table (`0001_baseline.sql:4474-4475`); anything memoised by viewer revision may stay stale until another change.
- **Confidence:** Confirmed by reading for each difference; the last two are Probable in effect. **Severity:** Medium. **Backlog:** no. `downloads.md:230-231` says the opposite.

### B-14.11 Audiobook position does not follow the listener to a later part
- **What happens:** Online, the book-level pointer is upserted to the part being played (`S/internal/playback/personal_progress.go:71-75`). Offline sync only runs `UPDATE book_resume SET position=… WHERE profile_id=? AND item_id=?` (`progress.go:216`), which changes nothing unless the pointer already names that part.
- **Effect:** Listen to parts 3–5 on a flight; the server still resumes the book in part 2.
- **Confidence:** Confirmed by reading. **Severity:** Medium for audiobook users. **Backlog:** no.

### B-14.12 "Original" is offered to devices that cannot play it
- **What happens:** see A-14.4. Nothing in `options.go` or the sheet (`N/src/app/downloads.tsx:133-141`) considers the container or codecs; iOS plays downloads with AVPlayer (`N/Native/PorticoPlaybackController.m:14-25`, `PorticoDownloads.m:460`).
- **Effect:** On iPhone and iPad an MKV original (most ripped libraries) downloads, verifies, gets a receipt and then fails to play; files with DTS/TrueHD audio play silently. Android (ExoPlayer) fares better but has its own gaps.
- **Confidence:** Probable (no MKV handling exists in the Apple code; not run). **Severity:** High on Apple devices. **Backlog:** no.

### B-14.13 Pause and Cancel on a working preparation almost always answer "conflict"
- **What happens:** Actions must name the current `revision` (`preparations.go:248-250`). The worker increments `revision` for every 32 MiB hashed (`worker.go:546`) and on every pass over a claim waiting for a conversion (`worker.go:447`); the worker is re-woken by its own commits about once a second (`S/internal/worker/worker.go:104-116`). The client's copy of the row is up to 5 seconds old.
- **Effect:** the documented pause/cancel of running work loses the race nearly every time; each failure returns a row that is stale again within a second.
- **Confidence:** Probable. **Severity:** Medium. **Backlog:** no.

### B-14.14 Files of a signed-out account or another profile stay on the device, invisible and unremovable
- **What happens:** Sign-out and "leave server" only clear the active owner (`N/src/app/session.tsx:1007-1009, 1032`); entries are filtered by owner for display (`offline-downloads.ts:170-177`); the used-space figure and the storage limit count every `ready` entry on the device (`PorticoDownloads.m:371-377`; `offline-downloads.ts:396-400`). There is no screen that lists or deletes entries that are not the current viewer's.
- **Effect:** storage that only deleting the app (or signing back in as that viewer) can reclaim; a second profile can be told "this would go over your download limit" by files it cannot see. Settings (`wifiOnly`, limit, auto-next, delete-watched) are also device-wide, so any profile can change them for all.
- **Confidence:** Confirmed by reading. **Severity:** Medium. **Backlog:** no. Also a contract disagreement (§3 rule 6).

### B-14.15 A ready claim can point at a prepared file that no longer exists
- **What happens:** The owner deleting a prepared version, or the collector retiring one whose source association went away, moves it to `deleting`/`deleted` (`preparedmedia/service.go:468-506`, `artifacts.go:116-125`). Nothing updates download claims that reference it. `OpenDownloadArtifact` then returns `ErrUnavailable` (`downloads_access.go:36-43`), which the transfer route maps through the generic prepared-media failure, while the claim stays `ready`, keeps counting against the ceiling and keeps being offered.
- **Confidence:** Probable. **Severity:** Medium (a dead end for anyone mid-transfer or arriving later). **Backlog:** no.
- **Related, good:** a transfer already open keeps its file through the reader lease (`downloads_access.go:29-49`), so an in-flight download is not cut off.

### B-14.16 Receipt dead ends
- **What happens:** (a) If the server no longer knows a receipt (database restored from an older backup, claim rows lost), renewal answers `unknown_receipt`; the app correctly does not delete, but then never asks for a new receipt because the entry still *has* one (`offline-downloads.ts:243-247` only handles entries without a receipt). After 30 days the title says "needs to check in with your server" forever. (b) If the claim itself is unknown, `obtainReceipt` silently gets `refused` and the entry says "Connect to your server once…" forever (`:441-445`).
- **Confidence:** Confirmed by reading. **Severity:** Low-to-Medium (needs a restore or data loss, but the contract discusses exactly this in CF60). **Backlog:** no.

### B-14.17 A whole-container request fails if anything in the library or the viewer's state changes during capture
- **What happens:** The fence is a digest of the library's revision and the viewer's personal revision for it (`requests.go:218-266`); capture compares it on every 20-member page and fails the request with `selection_changed` on any difference (`requests.go:352-355`). A scan adding one file anywhere in that library, or the same viewer marking something watched or playing something there, changes it.
- **Effect:** large requests against a library that is being scanned or watched fail part-way; already captured members are abandoned (the request never reaches admission).
- **Confidence:** Probable. **Severity:** Medium. **Backlog:** no.

### B-14.18 iOS may lose a completed large download during background verification
- **What happens:** After a background completion the file is moved to `<uuid>.incoming` and hashed on the module's serial queue before the session's completion handler is released (`PorticoDownloads.m:166-222`). iOS allows a relaunched app only a short time to handle these events. If the app is suspended or killed mid-hash, the next launch deletes every `.incoming` file as crash debris (`:101-103`) while the entry stays `downloading` with no task behind it.
- **Confidence:** Speculative — needs a device test with a file of tens of gigabytes. **Severity:** Medium. **Backlog:** no.

### B-14.19 Entries can be stuck in "downloading"
- **What happens:** iOS never reconciles the manifest with the session's live tasks at launch (`loadManifest`, `:99-111`). An entry whose task is gone (B-14.18, or a manifest write that lost a race) offers only Pause, and `pause:` does nothing when no task is found (`:331-337`). Android handles this case (`PorticoDownloadCenter.kt:82`).
- **Confidence:** Probable. **Severity:** Low (Remove still works). **Backlog:** no. The amendments require this reconciliation (`architecture-amendments.md:79`).

### B-14.20 Android transfer lifetime
- **What happens:** transfers die with the process; nothing reschedules them. Wi-Fi-only pauses on a move to cellular and never resumes on return (`PorticoDownloadCenter.kt:86-90`). The foreground service is of type `dataSync` with no `onTimeout` handling (`:350-365`); recent Android versions limit such services to a few hours per day and expect the service to stop when told. Starting it from the background can be refused, which the code tolerates by continuing without it (`:358-363`).
- **Confidence:** Speculative for the platform limits (not tested); Confirmed for no auto-resume. **Severity:** Low-to-Medium. **Backlog:** no.

### B-14.21 Clocks
- **What happens:** playability compares the device's wall clock with the receipt's expiry (`CC/downloads.ts:255-260`); setting the clock back extends a receipt indefinitely, and a device whose clock is far ahead is locked out even after renewal. Offline progress wins or loses by the device's timestamp, with up to 24 hours of future skew accepted (`progress.go:59, 101`), so a device running ahead beats genuinely newer online activity for that long. The amendments ask for monotonic-time protection (`architecture-amendments.md:77`); none exists.
- **Confidence:** Confirmed by reading. **Severity:** Low (this is not DRM by decision). **Backlog:** no.

### B-14.22 A download resumes from the device's own stale position
- **What happens:** see A-14.30. **Confidence:** Confirmed by reading. **Severity:** Low-to-Medium. **Backlog:** no.

### B-14.23 Re-requesting something already held says "Nothing could be added"; a failed claim blocks a fresh one
- **What happens:** An existing live claim (including one in `failed`) is returned as-is with `accepted: 0` and `duplicate: false` (`preparations.go:128-134`); `duplicate` is only set for a replayed `operationId`, which the client never reuses. The sheet then shows "Nothing could be added." (`N/src/app/downloads.tsx:95`). A failed claim is not retried by asking again; the user must find it in the list and press Retry.
- **Confidence:** Confirmed by reading. **Severity:** Low. **Backlog:** no.

### B-14.24 Finished rows are never removed
- **What happens:** nothing deletes `download_preparations` in `cancelled`/`expired`/`unavailable`, revoked `download_receipts`, completed `download_requests` and their items, or `download_progress_marks` (the only deletes are grants and account erasure). The revocation list therefore grows without bound, as does each viewer's preparation list.
- **Confidence:** Confirmed by reading (grep for `DELETE FROM download_`). **Severity:** Low. **Backlog:** no.

### B-14.25 The downloads worker does a write transaction on every wake
- **What happens:** each pass runs `expire` inside a write transaction of class "foreground transfer" and lists the snapshot directory with one query per file (`worker.go:64-75, 122-194`; `source_snapshot.go:158-195`). The loop is woken by any commit anywhere in the server, floored to once per second (`worker.go:36-48`; `S/internal/worker/worker.go:104-116`).
- **Effect:** on a busy server (a scan in progress) the downloads worker takes the single writer about once a second even when nobody has ever downloaded anything.
- **Confidence:** Probable. **Severity:** Low. **Backlog:** no. Easy to gate on "any live claim or request exists".

### B-14.26 Multi-file and multi-version titles are refused with a misleading reason
- **What happens:** `sourceReason` returns `source_unavailable` when a title has any number of asset links other than one (`targets.go:72-76`); the message shown is "The original file is not reachable. Reconnect the source and retry."
- **Effect today:** stacked films cannot be downloaded. **Latent trap:** when the backlog item "a second file of the same title becomes a second version" lands, every title with two versions becomes undownloadable.
- **Confidence:** Confirmed by reading. **Severity:** Low now, Medium later. **Backlog:** the merge is listed; this consequence is not.

### B-14.27 Small ones
- "Remove after watching" fires only on the natural end of the file, not when the viewer stops in the credits after the 92% mark (`offline-downloads.ts:563-567`). Low.
- Removing a download from the device does not release the server claim, so for an original the server keeps its full copy for the rest of the retention period (`offline-downloads.ts:429-434`). Low, but it compounds B-14.5.
- `POST /v1/downloads/receipts/revoke` accepts any identifier-shaped `reason` (`receipts.go:483-488`), but the client's revocation-list parser accepts only the closed reason set and rejects the whole page otherwise (`CC/downloads.ts:295`). No client reads the list today. Low.
- `Retry` from `unavailable` can collide with the one-live-claim unique index if a newer claim for the same title and quality was created meanwhile (`preparations.go:292`; `0001_baseline.sql:3798`). Speculative, Low.
- `HEAD` on the transfer address counts as first use and starts the replay window (`downloads_routes.go:231`; `grants.go:156-161`). Low.
- Android's HTTP client refuses redirects (`PorticoDownloadCenter.kt:63`); a reverse proxy that redirects HTTP to HTTPS turns into `server_refused`. Low.

## Where Portico is stronger

- **Integrity end to end.** A published SHA-256, verified on the device before a file is ever called ready, with atomic promotion from a private partial. Nobody else does this.
- **Owner can really withdraw offline access**, per receipt, per profile or server-wide, and the client honours it — while a mere outage never deletes anything (`offline-downloads.ts:478`).
- **Strict isolation on shared devices.** Entries, receipts, resume points and unsent progress are all filed under the exact viewer; a wrong-profile renewal cannot delete another profile's files (and there are tests for the bug that once did, `N/test/offline-ownership.test.ts`).
- **Permission is re-checked at every step**, and an invisible title is indistinguishable from a missing one.
- **Converted files are made once and shared**, with exact sizes and no repeated CPU cost; conversions cannot be triggered by guests.
- **Server-side expansion of whole shows and playlists** in bounded pages, with no size limit and no client-side library walking — the right shape for very large libraries.
- **Idempotent, fenced commands and per-item outcomes** for batches, receipts and progress; a client always learns exactly what happened to each entry.
- **Honest sizes** before committing, and a storage ceiling the owner controls.
- **Privacy-minded storage:** app-private, excluded from backups, no tokens in file names, grant address removed from the manifest once a download completes, path components sanitised on both platforms (`PorticoDownloads.m:85-97`; `PorticoDownloadCenter.kt:333-339`).
- **A paused history is respected** for offline progress (`progress.go:127-135`).
- **A documented third-party contract** (`S/api/downloads.md`) clear enough that another client could implement it.

## Doc/code disagreements

| # | Document says | Code does |
|---|---|---|
| 1 | `contract.md` §8 rule 7: server retention and device retention are separate. | Retention expiry revokes receipts; the app then deletes the file (B-14.1). `downloads.md:261-263` documents the code's behaviour. |
| 2 | `contract.md` §3 rule 6, §11: sign-out removes the account's downloads. | Files are kept and hidden (`N/src/app/session.tsx:1007-1009`). |
| 3 | `contract.md` §9 rule 2: resume uses validated byte ranges; CF36: renew the grant for the exact artifact. | Partials are discarded whenever the grant changes, which is always (B-14.2). |
| 4 | `contract.md` §10.1: receipt fields `lastVerifiedAt`/`verifyBy`, signed with the server identity key, RFC 8785 canonical JSON, four outcomes (`valid-replacement`, `revoked`, `invalid`, `out-of-scope`), a new `receiptId` on each renewal. | `issuedAt`/`expiresAt`, a separate receipt key table, signed raw payload bytes, outcomes `issued`/`renewed`/`refused` with reason codes, the same receipt id renewed in place (`receipts.go`). |
| 5 | `contract.md` §7: Source may be losslessly remuxed for the device, with disclosure. | No remux; no device input to the options (A-14.4). |
| 6 | `contract.md` §8/§9 state names (`requested`, `preparing`, `transferring-observed`, `retained`, `failed-retryable`, `waiting-for-wifi`, `authorization-unverified`, …). | Server: `queued/running/ready/paused/failed/unavailable/cancelled/expired`. Device: `downloading/paused/failed/ready`. |
| 7 | `contract.md` §4.4, §9 rule 6, amendments line 71: offline outbox with device sequence and base revision; explicit offline actions; "not client timestamps as authority". | One latest observation per title, ordered by device timestamp; no explicit offline actions (decision R09 marks this as future work). |
| 8 | Amendments line 77: monotonic-time protection against clock rollback. | Wall clock only (B-14.21). |
| 9 | Amendments line 79: reconcile OS tasks, partial files and catalogue at launch. | Android partly; iOS not (B-14.19). |
| 10 | `downloads.md:7-9`: "Downloads produce no media of their own." | An original download writes a full copy into the state folder (A-14.2). |
| 11 | `downloads.md:96-98`: pause keeps the work; a restart costs one hashing window. | True for the hashing phase only; the copy phase before it restarts from zero. |
| 12 | `downloads.md:107-116`: the batch endpoint accepts `containerId` up to 100 items. | `Request` has no container field (`types.go:170-176`); containers are only accepted by `/v1/downloads/requests`. The last section of the same file half-corrects this. |
| 13 | `downloads.md:230-231`: offline progress lands in "the same resume position, activity row and watched flag an online client writes. There is no second kind of progress." | A separate, simpler write path (B-14.10). |
| 14 | `downloads.md:273-277`: `allowDownloads` "answers true unless a hosted restriction has revoked the profile; the dedicated per-profile column is owned by the profile-restrictions workstream". | The per-profile restriction is already read (`policy.go:25-29`). |
| 15 | `downloads.md:210-212`: clients poll the revocation list on reconnect. | No first-party client calls it; renewal covers the need. |
| 16 | `offline-downloads.ts:422-423` and the `network` failure text: a continued transfer resumes. | It restarts (B-14.2). |
| 17 | `client-core` `rotate()` comment: a renewed token restarts nothing viewer-scoped. | The downloads provider restarts (B-14.9). |
| 18 | `contract.md` §12: clients distinguish device offline, server offline, permission denied, source missing, storage full, integrity failed, authorization needs verification, update required. | Six device failure codes, several sharing "Try again" (`offline-downloads.ts:96-104`). |

## Open questions for Justin

1. **Should a downloaded file ever disappear because the *server* tidied up?** Today it does after `retentionDays` (B-14.1). If the answer is no, retention should stop transfers and free server bytes without revoking receipts.
2. **Is the 30-day check-in the right length, and should the owner be able to change it** (longer for a family, shorter for guests)? It is a constant today (`types.go:111`).
3. **Sign-out: delete downloads (contract) or keep them hidden (code)?** If kept, should there be a "downloads from other profiles on this device" cleanup?
4. **Original downloads: is the full server-side copy acceptable**, or should originals be served straight from the library with a change check (F-14.1)?
5. **Who may ask for a converted download?** Owner only (today), or profiles the owner allows?
6. **Should the transfer credential live longer than ten minutes** so the phone's own downloader can pause and resume, or should grants be renewable for the same file?
7. **How far should offline go for v1?** Subtitles, artwork and titles with every download (I would call these required); grouped offline list and next-episode; a full offline shell; music and audiobook queues.
8. **Standing rules** ("keep the next three unwatched", "new episodes", "My List"): wanted for v1, later, or never?
9. **Should Play use the downloaded copy when online**, and should title pages show "Downloaded"?
10. **Offline explicit actions** (mark watched, My List, rating): build the contract's outbox, or drop it from the contract?
11. **Conflict rule for offline progress:** keep "newest device timestamp wins" and update the contract, or build the sequence-based model?
12. **Should offline plays count** in play history, play counts and the viewer's history?
13. **Device compatibility:** hide or label options a device cannot play; add a lossless repackaged original?
14. **SD card / external storage on Android, and export to other apps:** in scope?
15. **A profile PIN and offline downloads:** on a cold start with no network the app adopts the last saved viewer and lists its downloads (`N/src/app/session.tsx:310`). If that profile has a PIN, should the PIN be asked first? I could not establish from the code I read whether it is.
16. **Android's required download notification** versus the no-notifications rule: accept as the one exception, or move to a mechanism that needs none?
17. **Default for `maxPreparedBytes`:** keep unlimited, or default to a share of the state volume?

## Evidence boundary

- Everything above is from reading source. No server was started, no app was built or run, no device or simulator was used, no network request was made to any Portico system.
- Platform behaviours that drive several findings — how iOS background sessions pause, retry and wake the app; how long iOS allows for background event handling; Android foreground-service limits and vendor battery killers; whether AVPlayer plays the fragmented MP4 files the encoder writes from local storage with reliable seeking — are stated from platform knowledge and were not observed.
- Performance statements (copy and hash times, manifest rewrite cost, worker wake frequency) are reasoned from the code, not measured.
- I did not verify how the online title page treats a watched item that still has a stored position, or which caches key on `viewer_revisions`; the related parts of B-14.10 are marked Probable.
- Claims about Plex, Infuse and Netflix come from memory of their public behaviour, not from documents fetched during this audit.
- The two download OpenAPI files were not compared field by field with the Go types.
- Comparison projects were read selectively (see Scope); absence of a feature there is "not found in what I read", except where a search covered the tree (Swiftfin).

## Outside my domain

- **Converted files are forced to 30 fps and stereo, and HDR sources are refused** (`S/internal/decoder/prepared.go:36`, `preparedmedia/worker.go:663, 701`). This shapes every smaller download but belongs to the transcoding auditor.
- **The recipe table and the encoder's accepted set disagree** (B-14.4) — also affects the owner's "Optimize" feature, not just downloads.
- **Access-token rotation re-creates services keyed on the token string.** I only traced the downloads provider; other providers keyed the same way would restart too.
- **Capability addresses in logs.** The transfer token is in the URL path; I did not check access-log redaction for `/v1/downloads/artifacts/{grant}`.
