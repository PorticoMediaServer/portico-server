# Outcome
Implemented one background speed choice, an idempotent owner pause/resume action, a durable one-hour pause, and a plain startup recovery page. Added adjustable subtitle track/file limits, analysis artifact/stage storage limits, typed resource-usage readback, rolling upload deadlines and progress-based provider/artwork/subtitle HTTP transport. The stored-control inventory guard and focused semantic tests pass. Cold-process checks passed for corrupt database and invalid state-folder failures. Windows amd64 and macOS arm64 builds passed. The still-watching preferences are wired through server queues and the shared client; server and client regressions pass. Implementation and final verification are complete within the owned files. Milestone snapshots 001–004 preserve the work; final cumulative snapshot 005 is the integration handoff. Client presentation, native synchronization, active core-file handoffs and physical/load acceptance remain explicitly listed below.

# Decisions I made
- Read the programme and lane briefs in full. Work stays in the named worktree; no commits, push or deployment.
- Context sync could not write its read-only lock; existing local guidance was loaded. Host: ehler-homelab-mini-1 (personal Ubuntu).
- Preserve hostile-input bounds and small database checkpoints; owner limits are distinct from those safety bounds.
- Keep four additional size/count values with the owner storage controls, rather than adding unrelated pace/time switches. Defaults remain useful on modest hardware; choosing larger values is the owner’s choice.
- Background pause is temporary and explicit. Active jobs finish their current quantum; playback, DVR capture, authorization expiry and essential cleanup remain available. Scope-limited active core worker integration is listed below.
- Limits lowered during operation keep existing analysis artifacts/subtitle inventory and do not evict streams. New scans/acquisitions/stages use the chosen limit. Conversion/connection limits retain the existing per-operation/restart application rules.
- Recovery is plain HTTP on the configured listener while database/TLS settings are unavailable. It offers no unauthenticated repair/restart operation, does not open/write the failed database, uses catalogue-backed English recovery copy and exposes a message ID for clients. Error details stay in the service log.
- Local artwork usage never walks storage or waits for the ingestion mutex on an owner request. Its readback explicitly distinguishes a known measurement from unknown/currently updating.
- A stored preference with a nominal “consumer” label is insufficient: this lane’s guard requires an actual runtime reader. That additional check found and led to wiring the still-watching settings.

# Bugs

- B-01.4 — already fixed on this base: device approval has identity/playback consumers; covered by the runtime inventory.
- B-03.8 — already fixed on this base: sessionStorageMiB is adjustable and HLS pruning uses it.
- B-03.10 — already fixed on this base: temporaryDirectory is used at startup and background conversion admission reads maxBackgroundSessions. This lane removed the overlapping priority control.
- B-04.2 — already fixed on this base: scan-time extraction was replaced by immediately selectable lazy resources.
- B-04.3 — already fixed on this base: discovery no longer couples every subtitle extraction to basic/deep analysis success.
- B-04.4 — already fixed on this base: no eight-track scan extraction under a shared 90-second timer; this lane also removes the remaining remote-acquisition wall clock.
- B-04.5 — fixed: useful tracks are retained and the owner can raise the per-file inventory limit (128 default, 4096 maximum).
- B-04.11 — fixed: image-subtitle acquisition/extraction/sidecar readers follow the owner size limit (128 MiB default, 512 MiB maximum); bounded metadata/text/image-geometry checks remain.
- B-04.12 — already fixed on this base: lazy extraction uses a progress-aware input and is detached from a single viewer’s short import request.
- B-04.27 — already fixed on this base: managed discovery queries indexed basename candidates and the helper enumerates subtitle candidates; the 4096 bound is on candidate matches, not the entire flat media folder.
- B-04.32 — already fixed by the base’s snapshot/foreground subtitle changes (listed in the fix log).
- B-05.1 — already fixed by the base’s adaptive scan quantum; final speed/scan policy integration remains with the active scan-loop lane.
- B-05.29 — not changed: local metadata/parser input bounds remain intentional hostile-input bounds; tolerant record handling belongs to naming/scanning.
- B-06.11 — already improved on this base by provider profiles/concurrency; this lane extends the same speed projection to shared batch/process boundaries and removes whole-provider-chain wall clocks. No millions-of-titles throughput claim.
- B-09.1 — already fixed: restore/integrity operations have their own bounded concurrency and no short request wall clock. No remaining timeout in backup/download domain source.
- B-09.9 — already fixed: conversion path applied at startup, requested/effective distinction retained.
- B-09.10 — already fixed: secureConnectionsPolicy and its redundant public choices are removed on this base.
- B-09.28 — fixed: schedule windows/defaults/accepted tasks offer backups only, and the catalogue says so. Other work follows Background work speed or the explicit one-hour pause.
- B-09.29 — fixed: both corrupt/unopenable database and earlier state-folder failures serve a plain recovery page plus failed readiness until the service is restarted or stopped. Cold-process checks passed.
- B-11.5 — already fixed on this base: owner import bytes/channels/programmes limits replace the old 16 MiB/2000/100000 ceilings; chunked/filtered import remains bounded.
- B-11.9 — already fixed on this base: buffer, retry, user agent, source filters, numbering and guide mapping have runtime consumers; this lane adds progress-based live fetch/probe behavior.
- B-14.3 — already fixed on this base: grant replay no longer rejects merely because ten minutes passed since first use. Signed grant expiry/revocation remain security boundaries.
- B-17.4 — already fixed on this base: web fragment load has a large safety ceiling and recovery observes incoming bytes; this lane removes server-side segment/probe wall clocks that could still interrupt progress.
- B-18.4 — not changed: complete-result Genres/Series paging belongs to the active search-sort/client lanes; no page-size knob is substituted for continuation.
- B-18.11 — fixed here for the server/shared inventory habit: 120 stored control leaves plus all 59 preference keys require reviewed runtime witnesses, and the previously inert still-watching pair is wired. Device acceptance is not implied by this static guard.
- B-20.12 — not changed here: native subtitle appearance/system caption style belongs to the subtitle/Apple/Android lanes; their reported wiring is in the intake inventory.
- B-20.17 — not changed here: native default video speed belongs to the Apple/native playback lanes; their report says it is now applied. The web runtime reader is checked here.
- B-21.5 — not changed: unwindowed native History/Saved lists belong to native-app; server pagination remains bounded with cursors.
- B-21.6 — not changed: native preference control reachability belongs to native-app; newly added owner controls have shared structure rows where appropriate.
- B-21.7 — not changed here: the native lane reports corrected device-class wording; see its intake inventory.
- B-21.11 — not changed: Saved Play/Shuffle acting only on a loaded native prefix belongs to native-app/shared queue selection integration.

The P3/P5 cross-domain entries above are distinguished from this lane’s changes; other unrelated bugs in the cited domain documents remain with their domain lanes. The original audit’s D-23 suggestion to remove inert features is superseded by the programme’s wire-them rule. The programme’s explicit one-hour owner pause supersedes the older “never paused” wording in workpolicy commentary.

# API changes
- GET/PUT `/v1/admin/maintenance/settings`: removed public `backgroundTaskPriority`, retain `backgroundSpeed`; response adds `pausedUntilUnixMillis`. Client-core structure/parser updated; authored priority row removed. Internal process priority derives from speed.
- POST `/v1/admin/maintenance/background-pause`: `{paused, operationId}` returns the maintenance document. Owner role and admin key scope required. Same operation ID replays without extending the pause.

- GET `/v1/admin/resource-usage`: typed owner/admin-scope usage for connections, analysis tracked bytes, streams, measured local-artwork usage (unknown while ingestion is measuring), and maximum subtitle tracks per file; client-core service added.
- Queue post-play response fields keep their shape but now carry actual autoplay/countdown/still-watching policy; client-core no longer replaces it with constants. Preference registry revision is p39.CD32.B5.
- Subtitle mutation route body bounds now allow the raised image-subtitle sizes; the toolkit maximum is a 1 GiB safety bound, with each route retaining its own body limit and normal authorization.
- Full runtime settings document adds `subtitleTrackLimit`, `subtitleFileLimitMiB`, `analysisStorageGiB`, `analysisStageStorageMiB`.

# Schema changes
- `maintenance_policy.paused_until_ms`: durable owner pause deadline, default 0.
- `maintenance_window_tasks`: only the supported scheduled backup task is accepted; baseline seed windows contain backup only.
- `queues_v1.automatic_advances`: durable bounded episode-run count (default 0), updated on automatic completion and reset on deliberate queue navigation.
- Baseline edited directly; schema sync refreshed known columns/digests and its persistence check passed. No compatibility migrations or old database paths added.

# Owner or viewer settings added or wired
## Current stored-setting inventory and guard

`server/internal/operations/setting_consumers_test.go` reflects nine settings document types (including nested controls), requires an explicit reviewed source witness and stated effect for every stored control, checks those witnesses through the Go AST, and rejects stale entries. There are 120 runtime leaf controls. Informational projections are explicitly listed separately in the test: revision fences, schedule-editor labels/presets, fixed unsupported library-preview/DVR readback and notification cadence/retention aliases. Unsupported alternative values are rejected rather than saved. This is a structural guard against orphaned controls; focused semantic tests verify this lane's effects. It does not prove every external player applies its preferences.

| Document field | Runtime consumer | Effect |
|---|---|---|
| `administration.DVRDefaults.keepPolicy.keepCount` | `server/internal/administration/dvr_runtime.go` (KeepCount) | Set padding or retained recording count/days |
| `administration.DVRDefaults.keepPolicy.keepDays` | `server/internal/administration/dvr_runtime.go` (KeepDays) | Set padding or retained recording count/days |
| `administration.DVRDefaults.keepPolicy.mode` | `server/internal/administration/dvr_runtime.go` (Mode) | Set padding or retained recording count/days |
| `administration.DVRDefaults.postPaddingSeconds` | `server/internal/administration/dvr_runtime.go` (PostPaddingSeconds) | Set padding or retained recording count/days |
| `administration.DVRDefaults.prePaddingSeconds` | `server/internal/administration/dvr_runtime.go` (PrePaddingSeconds) | Set padding or retained recording count/days |
| `administration.LibrarySettings.allowMediaDeletion` | `server/internal/administration/deletion.go` (AllowMediaDeletion) | Authorize physical media deletion |
| `administration.LibrarySettings.analysis` | `server/internal/administration/library_runtime.go` (Analysis) | Select analysis recipes and trickplay geometry |
| `administration.LibrarySettings.continueWatching.includeSeasonPremieres` | `server/internal/catalog/continue_policy.go` (IncludeSeasonPremieres) | Select continue-watching rows and completion rules |
| `administration.LibrarySettings.continueWatching.maximumItems` | `server/internal/catalog/continue_policy.go` (MaximumItems) | Select continue-watching rows and completion rules |
| `administration.LibrarySettings.continueWatching.videoCompletion` | `server/internal/catalog/continue_policy.go` (VideoCompletion) | Select continue-watching rows and completion rules |
| `administration.LibrarySettings.continueWatching.videoPlayedThreshold` | `server/internal/catalog/continue_policy.go` (VideoPlayedThreshold) | Select continue-watching rows and completion rules |
| `administration.LibrarySettings.continueWatching.weeks` | `server/internal/catalog/continue_policy.go` (Weeks) | Select continue-watching rows and completion rules |
| `administration.LibrarySettings.navigation.chapterThumbnailMode` | `server/internal/administration/library_runtime.go` (ChapterThumbnailMode) | Select analysis recipes and trickplay geometry |
| `administration.LibrarySettings.navigation.trickplayIntervalSeconds` | `server/internal/administration/library_runtime.go` (TrickplayIntervalSeconds) | Select analysis recipes and trickplay geometry |
| `administration.LibrarySettings.navigation.trickplayMaxTiles` | `server/internal/administration/library_runtime.go` (TrickplayMaxTiles) | Select analysis recipes and trickplay geometry |
| `administration.LibrarySettings.navigation.trickplayTileWidth` | `server/internal/administration/library_runtime.go` (TrickplayTileWidth) | Select analysis recipes and trickplay geometry |
| `administration.LibrarySettings.providers.language` | `server/internal/administration/library_runtime.go` (Language) | Apply provider and locale policy to domain tables |
| `administration.LibrarySettings.providers.mediaKind` | `server/internal/administration/library_runtime.go` (MediaKind) | Apply provider and locale policy to domain tables |
| `administration.LibrarySettings.providers.provider` | `server/internal/administration/library_runtime.go` (Provider) | Apply provider and locale policy to domain tables |
| `administration.LibrarySettings.providers.region` | `server/internal/administration/library_runtime.go` (Region) | Apply provider and locale policy to domain tables |
| `administration.MaintenanceSettingsDocument.backgroundSpeed` | `server/internal/administration/background_policy.go` (BackgroundSpeed) | Set shared batch pacing and derived process priority |
| `administration.MaintenanceSettingsDocument.backupKeepCount` | `server/internal/administration/maintenance_runtime.go` (BackupKeepCount) | Keep the owner number of scheduled backups |
| `administration.MaintenanceSettingsDocument.retention` | `server/internal/administration/maintenance.go` (Retention) | Select the age for an owner cleanup action |
| `administration.MaintenanceSettingsDocument.windows.days` | `server/internal/workpolicy/policy.go` (Days) | Select scheduled backup occurrences |
| `administration.MaintenanceSettingsDocument.windows.durationMinutes` | `server/internal/workpolicy/policy.go` (DurationMinutes) | Select scheduled backup occurrences |
| `administration.MaintenanceSettingsDocument.windows.enabled` | `server/internal/workpolicy/policy.go` (Enabled) | Select scheduled backup occurrences |
| `administration.MaintenanceSettingsDocument.windows.id` | `server/internal/workpolicy/policy.go` (ID) | Select scheduled backup occurrences |
| `administration.MaintenanceSettingsDocument.windows.startMinute` | `server/internal/workpolicy/policy.go` (StartMinute) | Select scheduled backup occurrences |
| `administration.MaintenanceSettingsDocument.windows.tasks` | `server/internal/workpolicy/policy.go` (Tasks) | Select scheduled backup occurrences |
| `administration.MaintenanceSettingsDocument.windows.timezone` | `server/internal/workpolicy/policy.go` (Timezone) | Select scheduled backup occurrences |
| `downloads.Settings.maxPreparedBytes` | `server/internal/downloads/service.go` (MaxPreparedBytes) | Admit prepared bytes or expire download claims |
| `downloads.Settings.retentionDays` | `server/internal/downloads/service.go` (RetentionDays) | Admit prepared bytes or expire download claims |
| `livechannels.LiveDefaults.discoveryEnabled` | `server/internal/administration/live_sources.go` (DiscoveryEnabled) | Apply shared guide and live-source defaults |
| `livechannels.LiveDefaults.guideDays` | `server/internal/livechannels/directory.go` (GuideDays) | Apply shared guide and live-source defaults |
| `livechannels.LiveDefaults.importLimits.bytes` | `server/internal/livechannels/model.go` (Bytes) | Bound guide or playlist bytes and row counts |
| `livechannels.LiveDefaults.importLimits.channels` | `server/internal/livechannels/model.go` (Channels) | Bound guide or playlist bytes and row counts |
| `livechannels.LiveDefaults.importLimits.programmes` | `server/internal/livechannels/model.go` (Programmes) | Bound guide or playlist bytes and row counts |
| `livechannels.LiveDefaults.logoImport` | `server/internal/administration/guide_image_worker.go` (LogoImport) | Apply shared guide and live-source defaults |
| `livechannels.LiveDefaults.retryWindowSeconds` | `server/internal/livechannels/configuration.go` (RetryWindowSeconds) | Apply shared guide and live-source defaults |
| `livechannels.LiveDefaults.streamBufferSeconds` | `server/internal/livechannels/configuration.go` (StreamBufferSeconds) | Apply shared guide and live-source defaults |
| `livechannels.LiveDefaults.userAgent` | `server/internal/livechannels/configuration.go` (UserAgent) | Apply shared guide and live-source defaults |
| `livechannels.LiveSourceSettings.channelNumbering.mode` | `server/internal/administration/live_sources.go` (Mode) | Set source kind, guide mapping and channel numbers |
| `livechannels.LiveSourceSettings.channelNumbering.startAt` | `server/internal/administration/live_sources.go` (StartAt) | Set source kind, guide mapping and channel numbers |
| `livechannels.LiveSourceSettings.channelNumbering.step` | `server/internal/administration/live_sources.go` (Step) | Set source kind, guide mapping and channel numbers |
| `livechannels.LiveSourceSettings.enabledChannels` | `server/internal/livechannels/remote.go` (EnabledChannels) | Filter or override the saved live source |
| `livechannels.LiveSourceSettings.enabledGroups` | `server/internal/livechannels/remote.go` (EnabledGroups) | Filter or override the saved live source |
| `livechannels.LiveSourceSettings.filters.categories.mode` | `server/internal/livechannels/configuration.go` (Mode) | Filter or override the saved live source |
| `livechannels.LiveSourceSettings.filters.categories.values` | `server/internal/livechannels/configuration.go` (Values) | Filter or override the saved live source |
| `livechannels.LiveSourceSettings.filters.countries.mode` | `server/internal/livechannels/configuration.go` (Mode) | Filter or override the saved live source |
| `livechannels.LiveSourceSettings.filters.countries.values` | `server/internal/livechannels/configuration.go` (Values) | Filter or override the saved live source |
| `livechannels.LiveSourceSettings.filters.keywords.mode` | `server/internal/livechannels/configuration.go` (Mode) | Filter or override the saved live source |
| `livechannels.LiveSourceSettings.filters.keywords.values` | `server/internal/livechannels/configuration.go` (Values) | Filter or override the saved live source |
| `livechannels.LiveSourceSettings.guideSourceId` | `server/internal/administration/live_sources.go` (GuideSourceID) | Set source kind, guide mapping and channel numbers |
| `livechannels.LiveSourceSettings.kind` | `server/internal/administration/live_sources.go` (Kind) | Set source kind, guide mapping and channel numbers |
| `livechannels.LiveSourceSettings.logoImport.enabled` | `server/internal/livechannels/configuration.go` (Enabled) | Filter or override the saved live source |
| `livechannels.LiveSourceSettings.logoImport.overwriteExisting` | `server/internal/livechannels/store.go` (OverwriteExisting) | Preserve or replace imported logos |
| `livechannels.LiveSourceSettings.retryWindowSeconds` | `server/internal/livechannels/configuration.go` (RetryWindowSeconds) | Filter or override the saved live source |
| `livechannels.LiveSourceSettings.streamBufferSeconds` | `server/internal/livechannels/configuration.go` (StreamBufferSeconds) | Filter or override the saved live source |
| `livechannels.LiveSourceSettings.userAgent` | `server/internal/livechannels/configuration.go` (UserAgent) | Filter or override the saved live source |
| `metadata.MetadataSettingsUpdate.includeAdult` | `server/internal/metadata/runtime_settings.go` (IncludeAdult) | Apply artwork storage, adult matching or provider credentials |
| `metadata.MetadataSettingsUpdate.localArtworkLimitBytes` | `server/cmd/server/main.go` (LocalArtworkLimit) | Apply the owner storage bound to local artwork ingestion |
| `metadata.MetadataSettingsUpdate.tmdbReadToken` | `server/internal/metadata/runtime_settings.go` (TMDBReadToken) | Apply artwork storage, adult matching or provider credentials |
| `metadata.MetadataSettingsUpdate.tvdbProjectKey` | `server/internal/metadata/runtime_settings.go` (TVDBProjectKey) | Apply artwork storage, adult matching or provider credentials |
| `operations.NotificationSettings.certificateWarningDays` | `server/internal/operations/notifications.go` (CertificateWarningDays) | Prune inbox history or emit owner storage/certificate notices |
| `operations.NotificationSettings.retentionDays` | `server/internal/operations/notifications.go` (RetentionDays) | Prune inbox history or emit owner storage/certificate notices |
| `operations.NotificationSettings.storageCriticalPercent` | `server/internal/operations/notifications.go` (StorageCriticalPercent) | Prune inbox history or emit owner storage/certificate notices |
| `operations.NotificationSettings.storageWarningPercent` | `server/internal/operations/notifications.go` (StorageWarningPercent) | Prune inbox history or emit owner storage/certificate notices |
| `operations.Settings.accessUrls` | `server/cmd/server/networking_initializer.go` (AccessURLs) | Publish owner connection candidates |
| `operations.Settings.advertiseUrl` | `server/cmd/server/networking_initializer.go` (AdvertiseURL) | Publish owner connection candidates |
| `operations.Settings.advertisedInterface` | `server/cmd/server/main.go` (AdvertisedInterface) | Configure the running listener or media process |
| `operations.Settings.analysisStageStorageMiB` | `server/internal/mediaanalysis/storage_limits.go` (literal:analysisStageStorageMiB) | Bound analysis artifacts or one stage output |
| `operations.Settings.analysisStorageGiB` | `server/internal/mediaanalysis/storage_limits.go` (literal:analysisStorageGiB) | Bound analysis artifacts or one stage output |
| `operations.Settings.apiRequestsPerMinute` | `server/internal/httpapi/connection_policy.go` (APIRequestsPerMinute) | Rate limit a key or token |
| `operations.Settings.blockMemberAccessTokens` | `server/internal/httpapi/api_credentials_routes.go` (BlockMemberAccessTokens) | Admit or refuse member token creation |
| `operations.Settings.connectionLimit` | `server/cmd/server/main.go` (ConnectionLimit) | Configure the running listener or media process |
| `operations.Settings.customCertificateDomain` | `server/cmd/server/main.go` (CustomCertificateDomain) | Configure the running listener or media process |
| `operations.Settings.customCertificateKeyPath` | `server/cmd/server/main.go` (CustomCertificateKeyPath) | Configure the running listener or media process |
| `operations.Settings.customCertificatePath` | `server/cmd/server/main.go` (CustomCertificatePath) | Configure the running listener or media process |
| `operations.Settings.deviceApprovalRequired` | `server/internal/httpapi/access_admission.go` (DeviceApprovalRequired) | Apply device approval to playback admission |
| `operations.Settings.diagnosticDays` | `server/internal/operations/evidence.go` (DiagnosticDays) | Prune the corresponding history by its retention rule |
| `operations.Settings.directStreamRemux` | `server/internal/httpapi/delivery_settings_adapter.go` (DirectStreamRemux) | Project conversion policy to runtime delivery settings |
| `operations.Settings.ffmpegPath` | `server/cmd/server/main.go` (FFmpegPath) | Configure the running listener or media process |
| `operations.Settings.hardwareBackend` | `server/internal/playback/delivery_plan.go` (HardwareBackend) | Plan media delivery using owner policy |
| `operations.Settings.hardwareDevice` | `server/internal/playback/delivery_plan.go` (HardwareDevice) | Plan media delivery using owner policy |
| `operations.Settings.hdrToneMapping` | `server/internal/httpapi/delivery_settings_adapter.go` (HDRToneMapping) | Project conversion policy to runtime delivery settings |
| `operations.Settings.hdrToneMappingAlgorithm` | `server/internal/httpapi/delivery_settings_adapter.go` (HDRToneMappingAlgorithm) | Project conversion policy to runtime delivery settings |
| `operations.Settings.home.communityActivityEnabled` | `server/internal/httpapi/home_routes.go` (CommunityActivityEnabled) | Include community activity in home content |
| `operations.Settings.jobDays` | `server/internal/operations/evidence.go` (JobDays) | Prune the corresponding history by its retention rule |
| `operations.Settings.lanDiscoveryEnabled` | `server/cmd/server/administration.go` (LANDiscoveryEnabled) | Start or stop LAN advertisement |
| `operations.Settings.lanNetworks` | `server/internal/httpapi/request_locality.go` (LANNetworks) | Classify home connections and forwarded client addresses |
| `operations.Settings.library.missingTitles` | `server/internal/catalog/missing.go` (literal:library.missingTitles) | Keep or remove missing titles by the owner rule |
| `operations.Settings.library.missingTitlesDays` | `server/internal/catalog/missing.go` (literal:library.missingTitlesDays) | Keep or remove missing titles by the owner rule |
| `operations.Settings.logLevel` | `server/cmd/server/administration.go` (LogLevel) | Choose recorded log detail |
| `operations.Settings.logRetention.category` | `server/internal/httpapi/admin_diagnostics.go` (LogRetentionDays) | Prune the message log by category |
| `operations.Settings.logRetention.days` | `server/internal/httpapi/admin_diagnostics.go` (LogRetentionDays) | Prune the message log by category |
| `operations.Settings.maxBackgroundSessions` | `server/internal/httpapi/delivery_settings_adapter.go` (MaxBackgroundSessions) | Project conversion policy to runtime delivery settings |
| `operations.Settings.maxConcurrentSessions` | `server/internal/httpapi/delivery_settings_adapter.go` (MaxConcurrentSessions) | Project conversion policy to runtime delivery settings |
| `operations.Settings.maxHardwareSessions` | `server/internal/httpapi/delivery_settings_adapter.go` (MaxHardwareSessions) | Project conversion policy to runtime delivery settings |
| `operations.Settings.maxSoftwareSessions` | `server/internal/httpapi/delivery_settings_adapter.go` (MaxSoftwareSessions) | Project conversion policy to runtime delivery settings |
| `operations.Settings.name` | `server/internal/identity/service.go` (literal:name) | Name the server in identity documents |
| `operations.Settings.notificationDays` | `server/internal/operations/evidence.go` (NotificationDays) | Prune the corresponding history by its retention rule |
| `operations.Settings.pausedSessionTimeoutMinutes` | `server/internal/httpapi/playback_v1_routes.go` (PausedSessionTimeoutMinutes) | Expire paused playback sessions |
| `operations.Settings.perAccountCap` | `server/internal/playback/v1_session_helpers.go` (literal:per_account_cap) | Admit streams against the owner cap |
| `operations.Settings.planningPolicy` | `server/internal/playback/delivery_plan.go` (PlanningPolicy) | Plan media delivery using owner policy |
| `operations.Settings.playHistoryDays` | `server/internal/operations/evidence.go` (PlayHistoryDays) | Prune the corresponding history by its retention rule |
| `operations.Settings.playedRetentionSeconds` | `server/internal/playback/delivery_settings.go` (PlayedRetentionSeconds) | Pace conversion and retain played segments |
| `operations.Settings.remoteBitrateLimitKbps` | `server/internal/httpapi/delivery_settings_adapter.go` (RemoteBitrateLimitKbps) | Project conversion policy to runtime delivery settings |
| `operations.Settings.remoteSignInPolicy` | `server/internal/httpapi/access_admission.go` (RemoteSignInPolicy) | Admit remote sign-ins under owner policy |
| `operations.Settings.serverCap` | `server/internal/playback/v1_session_helpers.go` (literal:server_cap) | Admit streams against the owner cap |
| `operations.Settings.sessionStorageMiB` | `server/internal/httpapi/delivery_settings_adapter.go` (SessionStorageMiB) | Project conversion policy to runtime delivery settings |
| `operations.Settings.subtitleFileLimitMiB` | `server/internal/httpapi/connection_policy.go` (SubtitleFileLimitMiB) | Apply subtitle discovery and acquisition bounds |
| `operations.Settings.subtitleTrackLimit` | `server/internal/httpapi/connection_policy.go` (SubtitleTrackLimit) | Apply subtitle discovery and acquisition bounds |
| `operations.Settings.temporaryDirectory` | `server/cmd/server/main.go` (TemporaryDirectory) | Configure the running listener or media process |
| `operations.Settings.throttleBufferSeconds` | `server/internal/playback/delivery_settings.go` (ThrottleBufferSeconds) | Pace conversion and retain played segments |
| `operations.Settings.transcodingEnabled` | `server/internal/playback/delivery_plan.go` (TranscodingEnabled) | Plan media delivery using owner policy |
| `operations.Settings.trustedProxies` | `server/internal/httpapi/request_locality.go` (TrustedProxies) | Classify home connections and forwarded client addresses |
| `operations.Settings.updates.feedUrl` | `server/cmd/server/main.go` (UpdateFeedURL) | Check the selected release feed and channel |
| `operations.Settings.updates.releaseChannel` | `server/cmd/server/main.go` (UpdateChannel) | Check the selected release feed and channel |
| `operations.Settings.uploadCapacityKbps` | `server/internal/httpapi/playback_v1_routes.go` (UploadCapacityKbps) | Divide the remote upload ceiling among streams |
| `operations.Settings.x264Preset` | `server/internal/httpapi/delivery_settings_adapter.go` (X264Preset) | Project conversion policy to runtime delivery settings |

## Viewer preference inventory

The guard also walks all 59 published stored preference keys. Each must have a runtime reader call in a reviewed server, shared-client or web consumer. It excludes preference forms, the catalogue and comments. Quality keys share the runtime’s dynamic network-prefix reader. Native physical behavior remains a device acceptance check; the shared client consumes the server’s queue policy.

| Stored preference | Runtime reader |
|---|---|
| `appearance.cardSizePercent` | `web/src/shell/Shell.tsx` |
| `appearance.reduceMotion` | `web/src/shell/Shell.tsx` |
| `appearance.showBackdrops` | `web/src/shell/Shell.tsx` |
| `audiobooks.defaultSpeed` | `server/internal/catalog/listening_preferences.go` |
| `delivery.directPlay` | `server/internal/playback/delivery_policy.go` |
| `delivery.directStream` | `server/internal/playback/delivery_policy.go` |
| `delivery.transcode` | `server/internal/playback/delivery_policy.go` |
| `home.hiddenRowIds` | `server/internal/operations/home_layout.go` |
| `home.rowOrder` | `server/internal/operations/home_layout.go` |
| `music.audioNormalization` | `packages/client-core/src/music-preferences.ts` |
| `music.crossfadeSeconds` | `packages/client-core/src/music-preferences.ts` |
| `music.defaultSpeed` | `server/internal/catalog/listening_preferences.go` |
| `music.gapless` | `packages/client-core/src/music-preferences.ts` |
| `notifications.badges` | `web/src/shell/Shell.tsx` |
| `playback.autoplayNext` | `server/internal/httpapi/playback_v1_routes.go` |
| `playback.creditsSkip` | `web/src/player/Player.tsx` |
| `playback.defaultSpeed` | `web/src/player/Player.tsx` |
| `playback.introSkip` | `web/src/player/Player.tsx` |
| `playback.passoutAfterEpisodes` | `server/internal/httpapi/playback_v1_routes.go` |
| `playback.passoutProtection` | `server/internal/httpapi/playback_v1_routes.go` |
| `playback.playedThresholdPercent` | `server/internal/catalog/personal_activity.go` |
| `playback.preferredAudioLanguages` | `server/internal/playback/delivery_context.go` |
| `playback.preferredSubtitleLanguages` | `server/internal/subtitles/provider.go` |
| `playback.recapSkip` | `web/src/player/Player.tsx` |
| `playback.showSyncedLyrics` | `web/src/player/Player.tsx` |
| `playback.skipBackSeconds` | `web/src/player/Player.tsx` |
| `playback.skipForwardSeconds` | `web/src/player/Player.tsx` |
| `playback.sleepTimerMinutes` | `server/internal/catalog/listening_preferences.go` |
| `playback.startedThresholdPercent` | `server/internal/catalog/personal_activity.go` |
| `playback.subtitleBackground` | `web/src/player/Player.tsx` |
| `playback.subtitleMode` | `server/internal/playback/delivery_context.go` |
| `playback.subtitleSize` | `web/src/player/Player.tsx` |
| `playback.upNextCountdownSeconds` | `server/internal/httpapi/playback_v1_routes.go` |
| `privacy.includeInWatchTogether` | `server/internal/social/privacy.go` |
| `privacy.pauseWatchHistory` | `server/internal/downloads/progress.go` |
| `privacy.showActivityToMembers` | `server/internal/catalog/recommendation_engine.go` |
| `quality.cellular.allowHDR` | `server/internal/playback/delivery_policy.go` |
| `quality.cellular.maxAudioBitrateKbps` | `server/internal/playback/delivery_policy.go` |
| `quality.cellular.maxVideoBitrateMbps` | `server/internal/playback/delivery_policy.go` |
| `quality.cellular.maxVideoHeight` | `server/internal/playback/delivery_policy.go` |
| `quality.cellular.mode` | `server/internal/playback/delivery_policy.go` |
| `quality.local.allowHDR` | `server/internal/playback/delivery_policy.go` |
| `quality.local.maxAudioBitrateKbps` | `server/internal/playback/delivery_policy.go` |
| `quality.local.maxVideoBitrateMbps` | `server/internal/playback/delivery_policy.go` |
| `quality.local.maxVideoHeight` | `server/internal/playback/delivery_policy.go` |
| `quality.local.mode` | `server/internal/playback/delivery_policy.go` |
| `quality.unknown.allowHDR` | `server/internal/playback/delivery_policy.go` |
| `quality.unknown.maxAudioBitrateKbps` | `server/internal/playback/delivery_policy.go` |
| `quality.unknown.maxVideoBitrateMbps` | `server/internal/playback/delivery_policy.go` |
| `quality.unknown.maxVideoHeight` | `server/internal/playback/delivery_policy.go` |
| `quality.unknown.mode` | `server/internal/playback/delivery_policy.go` |
| `quality.wifi.allowHDR` | `server/internal/playback/delivery_policy.go` |
| `quality.wifi.maxAudioBitrateKbps` | `server/internal/playback/delivery_policy.go` |
| `quality.wifi.maxVideoBitrateMbps` | `server/internal/playback/delivery_policy.go` |
| `quality.wifi.maxVideoHeight` | `server/internal/playback/delivery_policy.go` |
| `quality.wifi.mode` | `server/internal/playback/delivery_policy.go` |
| `region.hourCycle` | `web/src/app/i18n.ts` |
| `region.locale` | `web/src/app/i18n.ts` |
| `search.rememberHistory` | `server/internal/catalog/search_history.go` |

## Added and consolidated controls

| Name | Type and default | Effect | Screen |
|---|---|---|---|
| Still watching protection and episode count | Existing boolean true / integer 3 (2–5) | Episode queues track automatic advances; after the configured run a Continue is required; deliberate next/previous/entry starts a new run. Audio queues are unaffected | Existing preference and post-play controls; shared client now reads the server policy |
| Background work speed | Gentle / Normal / Fast; Normal | Shared database batch pacing, metadata provider pace/concurrency, storage process priority; Fast uses normal process priority, other choices lower it | Existing Schedule row; separate priority row removed |
| Pause background work for an hour / Resume | Owner action; no pause by default | Durable deadline, automatic expiry, cancellable waits between cooperative batches and background/scan process admissions; active work finishes its quantum | Client-core action service and strings ready; custom panel action needed |
| Analysis storage | Integer GiB 1–16384; 32 | Analysis artifact admission and publication boundaries; tracked bytes in resource-usage API | Shared Storage & backups row added |
| Output per analysis stage | Integer MiB 1–4096; 256, no larger than total analysis storage | Analysis-stage output budget | Shared Storage & backups row added |
| Subtitle tracks per file | Integer 32–4096; 128 | Inventory retains useful tracks up to the chosen limit; raising it includes more on a new scan | Shared Storage & backups row added |
| Largest subtitle file | Integer MiB 32–512; 128 | Embedded image-subtitle extraction, sidecar reads and acquisition | Shared Storage & backups row added |

Storage controls occupy one shared Storage & backups section; streams and conversions stay together on their existing Playback page. Provider credential and per-source import documents keep their existing authority rather than being copied into the main settings document. The new resource-usage route reports tracked analysis bytes, active leased streams, current/maximum connections and largest stored subtitle track count. Existing conversion-capacity, DVR storage, download storage and metadata reports continue supplying their domain views.

## Inventory of other lanes at intake (their reported state, not a second settings authority)

### accounts-security

| Setting | Type | Default | Where it takes effect | Screen |
|---|---|---|---|---|
| `remoteSignInPolicy` | allow / owner-only / off | allow | Every session created (`identity.IssueTx`) | Exists (Connectivity) |
| `deviceApprovalRequired` | boolean | false | Device registration at sign-in. The same value as `deviceApproval` on `PUT /v1/direct/registration-policy`. | The switch exists; the members' device list exists per person on the People page and now shows real data |
| Member limit `ratingSystem` | one of the published rating systems, or empty | empty | The catalogue predicate | No picker yet; the People page offers the American list only |
| Self-registration `invite-only` | existing value | off | `POST /v1/auth/register` | No first-party screen sets the sign-up policy |

No new owner switches were added. Retention of ended session records (90 days), the hashing-slot wait (3 seconds) and the cross-device PIN count (20 a day) are constants.


### android-mpv

| Setting | Type | Default | Where it takes effect | Screen |
| --- | --- | --- | --- | --- |
| Player (`preferences.playerEngine`, this device) | Automatic, This device's player, mpv | Automatic | Now also on Android, Android TV and Fire TV | Yes (the Apple lane's row and player menu), shown where mpv loads |
| System caption size (Android Accessibility) | system | 1 | Scales subtitles in both Android engines | The system's |
| System "match content frame rate" (Android TV 12+) | system | system | Decides whether the display switches for mpv | The system's |


### api-platform

| Name | Type | Default | Takes effect | Screen |
|---|---|---|---|---|
| `apiRequestsPerMinute` | integer 0–100000 | 0 = 600 | Rate for one key or token, at once | None yet (it is in the settings registry, so it shows wherever the registry is rendered) |
| `blockMemberAccessTokens` | boolean | false | Stops members (not owner or administrators) creating tokens; existing ones keep working | None yet |

Both are phrased so that a settings document saved without them reads as the default.


### apple-mpv-review

No new setting or screen. Existing device Player setting (Automatic default, Native, mpv) now safely publishes and applies mid-title without discarding Up Next or silently resuming a paused title. The existing settings/player controls remain those supplied by the predecessor.


### apple-mpv

| Setting | Type | Default | Where it takes effect | Screen |
| --- | --- | --- | --- | --- |
| Player (`preferences.playerEngine`, this device) | Automatic, This device's player, mpv | Automatic | Engine choice in the player; sent to the server as `engineSetting` | Yes: Settings, Quality page, and the player's menu (Player). Shown only on a build that has mpv. |
| `playback.defaultSpeed`, `audiobooks.defaultSpeed` (existing, did nothing on native) | speed | 1 | Each video and audiobook starts at it | Existing rows |


### audio-engines

No new setting or screen. Existing normalization (off/track/album, default off), gapless (default on), crossfade (default zero), speed, pause intent and owner conversion/bitrate limits remain. Android now amplifies when measured gain and peak permit it. Missing-data behavior is the policy above. Audiobook comfort uses fixed server-matched listening rules; no extra switch.


### console

| Setting | Control | Where |
|---|---|---|
| `apiRequestsPerMinute` | number a minute, empty = 600 | Server › Integrations › API keys › Limits |
| `blockMemberAccessTokens` | "Members may make their own tokens": Yes / No | Server › Integrations › Members' tokens |
| `tmdbReadToken`, `tvdbProjectKey` | password field, replace or remove; shows only whether one is set | Server › Metadata |
| `includeAdult` | switch | Server › Metadata |
| `localArtworkLimitBytes` | number in GB | Server › Metadata |
| `backgroundSpeed` | Gentle / Normal / Fast | Server › Schedule |
| `lanNetworks` | add/remove list | Server › Remote access › Home network |
| `advertiseUrl` | text | Server › Remote access › Access from outside |
| `accessUrls`, `trustedProxies` | add/remove lists | Server › Remote access › Access from outside |
| `customCertificateDomain` / `Path` / `KeyPath` | text, with a status line | Server › Remote access › Your own certificate |
| `connectionLimit` | number, empty = automatic | Server › Remote access › Advanced |
| `library.missingTitles`, `library.missingTitlesDays` | already had a screen (identity lane) | Server › Storage & backups › Missing titles |
| Per-library provider and "local metadata only" | already had a screen | Libraries › a library › Metadata |


### conversion-outputs

| Setting | Type/default | Takes effect | Screen |
|---|---|---|---|
| ffmpegPath | absolute path / empty uses defaults | restart; qualify before use, report rejected override as pending | new field; client lanes must add control |
| sessionStorageMiB | integer / 4096; 64–1048576 | next budget measurement/operation | new field; client lanes must add control |
| temporaryDirectory | absolute directory / empty uses state/hls | restart; dedicated portico-hls child when configured | existing control |
| maxBackgroundSessions | integer / 0 means no added owner cap | future prepared/background media processes; cancellable queued admission | existing control |
| hdrToneMapping | boolean / true | existing delivery planning | existing control; fresh default now agrees with registry |

New setting labels/help are in the string catalogue. Existing throttle/retention, hardware backend/device, tone curve and software preset remain wired. No viewer switch added: playback-core selects the output from the viewer's quality and capability choices.


### downloads

None added. `downloads.maxPreparedBytes` now applies to converted copies only (decision 5). `downloads.retentionDays` now frees only the claim. `GET /v1/downloads/settings` additionally reports the producible qualities (computed, not stored).


### freshness-server

None. Existing personal preferences are now pushed to the profile's devices when saved. Maintenance/coalescing/batch limits are implementation defaults, not user settings.


### hosted

No new owner/viewer switch. Signing rotation uses existing key files/operator workflow. App name and return URI are per-request app inputs, visibly unverified, not account preferences. Existing HTTP acceptance and Hosted TLS route rules are unchanged.


### identity


### livetv
- importLimits: bytes/channels/programmes, default 512 MiB/50,000/5,000,000, hard maxima 1 GiB/100,000/10,000,000. Takes effect during fetch, preview and publication. New owner controls needed.
- enabledGroups/enabledChannels: arrays, empty means all. Available both on initial remote drafts and saved source configuration; an explicit empty saved selection clears an earlier draft selection. Parsed before guide staging; disabled channels have no catalogue/programme staging cost. New selection controls needed.
- streamBufferSeconds: integer 0–120, default 8; per-source nullable override; live readiness and EXT-X-START delay. Existing owner controls need verification.
- retryWindowSeconds: integer, default 60; per-source nullable override; upstream reconnect budget. Existing controls.
- userAgent: bounded string, default Portico/1.0; source fetch, guide fetch and input requests. Existing controls.
- categories/countries/keywords include/exclude filters, channelNumbering and guide mappings: now affect imported rows/current reads; existing controls.
- owner global hidden channels: affect every viewer and tuning, independent of personal hide; existing controls need clear labels.
- guideDays: default seven, affects remote guide ingestion. Existing controls.
- source logo import/overwrite settings affect imported logo values; overwrite off retains the prior channel URL across generations. The existing default logo/discovery worker policy is retained; no new artwork-download pipeline is added here.
- discoveryEnabled: existing setting continues to gate LAN discovery; descriptor enrichment is bounded to 64 devices/total five seconds.


### music-audiobooks

No new setting. Existing LRCLIB enablement, per-library local metadata policy, playback bitrate/capability limits, normalization/gapless/crossfade choices, and source read-policy guards remain effective. Audiobook completion overrides the generic played percentage for book files; there is no new screen for completion or rewind. The global artwork-cap setting did not exist.


### naming

None. No screens were added. Built-in exclusions are default scanner behaviour; ignore files are owner-authored files whose loading remains a scanner integration task.


### native-app

None added. Two changes to what Settings shows: speed, gapless, crossfade and volume levelling are hidden on Vega; quality, playback method and HDR say "All your phones and tablets" or "All your TVs" instead of "This device".


### networking

| Setting | Type/default | Runtime effect | Screen status |
| --- | --- | --- | --- |
| lanNetworks (Home networks) | string list; standard local ranges plus attached subnets | Hot, sole inbound locality authority; [] makes all remote | Existing connectivity screen; client lanes must update help/empty-list behavior |
| trustedProxies | string list; [] | Hot, trusted address/host/scheme headers and all rate/fairness subjects | Existing screen |
| advertiseUrl (External address) | string; empty | Hot, local connection document; HTTPS manual Hosted candidate prioritized | New screen row needed |
| accessUrls | string list; [] | Hot; local clients accept HTTP/HTTPS origins; Hosted filters HTTPS | Existing screen/help update needed |
| connectionLimit | integer; 0 = existing memory-based default; max 65536 | Restart; ordinary connection capacity plus fixed console reserve | New numeric screen row needed |
| customCertificatePath / customCertificateKeyPath / customCertificateDomain | strings; empty, configured together | Hot source, PEM file reload and live/resumed TLS validity | Existing screen; serving state display needed |
| advertisedInterface | string; empty | Explicit interface preference; automatic no longer filters names | Existing screen/help update needed |
| lanDiscoveryEnabled | boolean; true | Hot advertisement lifecycle; home-network changes refresh discovery topology | Existing screen |
| remoteBitrateLimitKbps / uploadCapacityKbps | integers; 0 | Existing caps now use the same owner home decision as sign-in | Existing screens |
| remoteSignInPolicy | enumeration; allow | Existing request locality seam is unified; all-route policy enforcement belongs to accounts-security | Existing screen |

Retired secureConnectionsPolicy and treatWanAsLan. Added catalogue IDs for the HTTP note, external address, connection limit and setup-already-complete wording; no viewer preference was added.


### ops-packaging
- `temporaryDirectory`: existing absolute path, default empty; now used by HLS at startup and compared with the applied value for pending Restart. Existing Playback screen exists; the settings registry revision is now p09.3.
- `PORTICO_CACHE_DIR`: absolute startup directory, default state; transient media consumers wired. No console field added.
- `PORTICO_BACKUP_DIR`: absolute startup directory, default state/backups; regular backup create/list/delete/restore/prune wired. No console field added.
- `updates.feedUrl`: existing hot string; default official latest published manifest. Empty opts out and clears its owner notice. Startup/five-minute maintenance drives the cached check (successful fetch refreshes after six hours); failures leave the previous notice alone. Existing Updates screen exists.
- Scheduled backup keep count: existing setting, default seven; now protects manual copies from scheduled pruning.
- Existing storage/certificate notice thresholds now have the five-minute runtime consumer.


### personal-state
None yet.


### playback-core

| Name | Type | Default | Where it takes effect | Screen |
|---|---|---|---|---|
| `quality.home` (preference registry, device-class scope) | `original` \| `automatic` \| `2160p` \| `1440p` \| `1080p` \| `720p` \| `480p` \| `360p` | `original` | the quality of a start that names none, inside the owner's home networks | Row exists in `settings-structure.ts` (Quality page, "At home"); renders on web through the generic preference row; native lane must check |
| `quality.remote` | same | `original` | same, outside the home networks | same ("Away from home") |
| Player (`engineSetting` in the capability document) | `automatic` \| `native` \| `mpv` | `automatic` | engine choice in planning | Row `playerEngine` declared (storage `device`, shown only when `capabilities.mpvPlayer`); **no screen wiring yet** |
| Force stereo (`overrides.forceStereo`) | switch | off | planning: every title two channels, no passthrough | Row `forceStereo` declared; no wiring yet |
| Don't pass sound through (`overrides.disablePassthrough`) | switch | off | planning: passthrough list ignored | Row `disablePassthrough` declared (shown when `capabilities.audioPassthrough`); no wiring yet |
| Turn off HDR (`overrides.disableHdr`) | switch | off | planning: device treated as SDR only | Row `disableHdr` declared (hidden when `capabilities.hdr === false`); no wiring yet |

Removed viewer settings: the per-network quality rows (`quality.cellular` composite and the lane keys) and the registry-backed `hdr` composite (replaced by the override). Owner settings are unchanged; the owner's remote bitrate limit, audio ceiling, height ceiling, HDR switch, remux switch, conversion switch and planning policy all still apply (the conversion switch no longer blocks copy-only repackaging).

New strings (`packages/i18n/src/catalog/en-US/settings.ts`, `preferences.ts`): `settings.row.playerEngine`, `settings.help.playerEngine`, `settings.playerEngine.automatic|native|mpv`, `settings.section.playbackFixes`, `settings.section.playbackFixesHelp`, `settings.row.forceStereo`, `settings.help.forceStereo`, `settings.row.disablePassthrough`, `settings.help.disablePassthrough`, `settings.row.disableHdr`, `settings.help.disableHdr`, `pref.label.quality.home|remote`, `pref.help.quality.home|remote`; reworded `settings.row.qualityHome`, `settings.help.qualityHome`, `settings.help.qualityAway`, `settings.section.qualityLimits`, `pref.group.quality.description`. Removed: `settings.row.qualityCellular`, `settings.row.hdr`, `settings.help.hdr`, `settings.hdr.*`, `pref.qualityMode.*`, `pref.network.*`, `pref.label.allowHDR`. The server's fallback message for `connection_too_slow` is in the error catalogue; `V1QueuePlayer` has the same sentence hard-coded beside its existing ones.


### providers-matching
- backgroundSpeed: shared maintenance choice gentle/normal/fast, default normal; wired provider pace, title concurrency and foreground pressure yield. Screen needed from client lanes.
- localArtworkLimitBytes: positive integer, default 2147483648; local artwork ingest reads it, max owner input 2^50; screen needed.
- includeAdult: boolean, default false; TMDB matching searches; screen needed.
- tmdbReadToken/tvdbProjectKey: optional owner strings; immediate adapter override and retry of blocked credential jobs, blank restores application value; read side is presence booleans; screen needed.
- Per-library provider and local metadata only remain existing settings; curated registry advertises actual video providers, MusicBrainz for music and local for all kinds. Existing library source/settings screens can render supplied choices.


### review-api-platform

None made. Reviewed `apiRequestsPerMinute` (integer, 0 means 600 by default) and `blockMemberAccessTokens` (boolean, false). Rate reads use current connection settings; token-creation setting exempts administrators and does not revoke existing credentials. Console screens are outside this lane and remain needed.


### review-identity
No changes made. Checked `library.missingTitles` (`immediately`, `after_days`, `never`, default `after_days`) and `library.missingTitlesDays` (1–3650, default 30). Rule/default tests pass; periodic cleanup is backed by the five-minute worker safety tick. Immediate cleanup can be deferred until a later worker pass because the loop throttles sweeps to thirty seconds, consistent with the programme's freshness window. The web missing-title rule/list exists; the other screens in the implementation report remain external follow-up work.


### review-networking

None added by this review. Observed implementation:

| Setting | Observed behavior |
| --- | --- |
| Home networks / lanNetworks | Replacement list; defaults include standard local and attached networks; [] makes all remote. Cache can regress on old receipts (F2). |
| trustedProxies | Empty default; hot shared client/host/scheme policy. F2/F7 need fixing; device IP uses socket peer (F10). |
| advertiseUrl / accessUrls | Local HTTP/HTTPS origins; manual Hosted entries filter HTTPS. Combined publication needs F6/F8, runtime hook F5. |
| connectionLimit | 0 selects memory-based default; owner override <=65536 applied at startup. Restart metadata incorrect (F9); reserve F1/F3. |
| customCertificatePath/key/domain | Validated together, hot source with fallback refresh; content digests and last valid pair implemented. Policy PATCH misses immediate source invalidation (F5). |
| lanDiscoveryEnabled | True default; hot through connectivity PATCH, not full console-settings PATCH (F5). |
| remoteBitrateLimitKbps | Shared locality is used; policy PATCH misses immediate delivery snapshot refresh (F5). |
| remoteSignInPolicy | Networking supplies the shared locality seam; complete sign-in/session enforcement remains accounts-security's scope. |


### scan-loop
Shared backgroundSpeed will pace scanning as well as providers. Per-library monitoring will default on.


### search-sort
No new switches or settings. Existing sort-title/metadata-language facts feed sorting. Existing rating/detail action ranges and duration filter presets use the new units/scale.


### social

- **No new settings.**
- **privacy.includeInWatchTogether** remains a profile-server boolean, default true, with its existing settings screen. False now anonymizes the joined participant and withholds individual position/report time, while keeping occupancy/readiness truthful. Client screens should render the private-member catalogue label.
- Existing group shuffle/repeat controls retain their settings and gain an independent fence. This lane does not add queue progression or redesign shuffle behavior.
- New catalogue messages: `web.together.state.waiting`, `together.privateViewer`, `playOn.controlUnavailable`.


### subtitles-text
- Existing preferred subtitle languages: profile language list, existing default; now also determines provider search language. Existing preference screens can continue editing it.
- Device subtitle rendering choice: API/client-core enum `text|ass|pgs|vobsub|dvb`; omitted choice retains established text/burn behavior. No new screen in this lane. Capability-driven automatic choice is playback-core integration.
- OpenSubtitles credentials: optional owner-held `PORTICO_OPENSUBTITLES_USERNAME` and `PORTICO_OPENSUBTITLES_PASSWORD`, unset by default; used for login/renewal. Existing API key enables anonymous use. `PORTICO_OPENSUBTITLES_TOKEN` is no longer read. No credential screen exists in this lane. The provider's official API contract was consulted: https://stoplight.io/api/v1/projects/opensubtitles/opensubtitles-api/nodes/open_api.json.
- Existing segment-detection setting retains its type/default and now accurately describes chapter/scene suggestions needing review.


### web-app

- `search.rememberHistory` (existing, boolean, default on): now takes effect on web. Its row already exists in Settings › Privacy.
- No setting added. Remembered library choices have no switch.
- Relabelled only: quality limits, playback method, HDR, card size, backdrops and reduce motion carry the device-kind badge.


### web-player

- No new keys. **Default quality** for home and away is the existing registry composite (`quality.home`, `quality.away`, and `quality.cellular` on phones) over `quality.<network>.mode` and `maxVideoHeight`; screen: Settings › Quality & data. Now read by the web player to tick the menu and to turn Automatic on, and reordered Original first.
- Device-local, no screen: fill/fit (`localStorage` `portico.player.fill.v1`), toggled from More and with Z.


### web-subtitles

| Setting | Type | Default | Takes effect | Screen |
|---|---|---|---|---|
| `audiobooks.defaultSpeed` (existing) | choice, now up to 3× | 1 | audiobook playback speed | Settings (structure updated) and the player's Speed menu |
| `playback.subtitleSize` (existing) | choice | medium | now also the overall size of drawn subtitles (0.9 / 1 / 1.12 / 1.25) | existing |
| Subtitle position (device, `portico.player.subtitle-raise.v1`) | −10 to +30 % of picture height, steps of 2 | 0 | drawn subtitles only | player › Subtitles › Position (Up, Down, Reset), shown while a drawn track is on |


# Needed from others

- Client/console lanes: use `pauseBackgroundWork` and `loadResourceUsage` from client-core in the Schedule and Storage/Playback custom panels. Show pause expiry/resume and measured usage beside the corresponding limits; show “not measured yet” when localArtwork.measured is false. Four numeric storage rows are already authored in shared settings structure.
- Native lane/integrator: sync the updated shared packages into the native worktree. Verify the existing post-play dialog uses the now-effective server policy and that “Still watching” sends a deliberate continuation. No native tree was edited from this lane.
- Active scan-loop lane: keep the same shared backgroundSpeed and workpolicy pause authority. The existing cooperative `Yield`/`PaceBackground` and background/scan helper admission now honor this lane’s projection. `ingestion/deep_analysis.go` still has a six-hour outer cap; replace it with the domain’s input/output progress signal when changing that core file.
- Active freshness lane: compactcatalog Run already calls the shared cooperative boundaries; `catalog/visibility_classes.go` does not. Add a cancellable `dbwork.Yield` between visibility batches, outside transactions, so that the explicit owner pause covers that remaining worker too. Preserve its current-content serving policy.
- Active search-sort/personal-state lanes: retain bounded pages and continuation frontiers. This lane did not restructure their candidate/batch core files or change the meaning of those bounds into user-facing truncation controls.
- Networking review/integrator: this lane fixes pending/effective connectionLimit readback and rejects older settings receipts in the connection-policy projection. Merge those localized changes with the networking review’s cache/route refresh fixes.
- Architecture/product contracts: replace blanket “never paused” and all-jobs-in-maintenance-windows text with the explicit temporary owner pause plus backup-only schedule. Restore/watch-state compatibility text must continue following the pre-release programme rules.

# Test notes for other environments

## Verified here

- `GOFLAGS=-p=2 go build ./...` passed after implementation.
- Focused tests passed in operations, httpapi, administration, dbwork, subtitles, mediaanalysis, decoder, playback/vod, progressio and cmd/server. They cover runtime-consumer guards, owner/member route boundaries, raised track/artifact limits, durable idempotent pause/resume, pending connection caps, stale receipts, actual slow/stalled HTTP/upload progress, physical decoder retirement/output bounds, finite-window deadline cancellation and recovery HTML.
- The still-watching server regression sets the episode-run count to the threshold, proves automatic advance is refused, and proves manual Continue resets it. The shared queue-client single-file suite passed 34 tests, including preserving the question on an automatic callback and advancing only after a deliberate Continue. Console client single-file suite passed 25 tests.
- TypeScript `npx tsc --noEmit -p web/tsconfig.json` passed. API generation passed and maintenance contract fixtures were refreshed through their focused record test. Schema sync passed.
- Cold binary on private loopback, temporary isolated state: invalid SQLite bytes and a state path that is a regular file each served HTML 503 and readiness status startup_failed; both stopped cleanly on SIGTERM. The page does not expose the private state path. No real server state or deployment was touched.
- Final Windows amd64 and macOS arm64 command cross-builds passed after the still-watching wiring and recovery copy changes: `GOFLAGS=-p=2 GOOS=windows GOARCH=amd64 go build -o /tmp/portico-limits-settings-final-windows.exe ./cmd/server` and `GOFLAGS=-p=2 GOOS=darwin GOARCH=arm64 go build -o /tmp/portico-limits-settings-final-darwin ./cmd/server`.
- Final targeted `go vet` passed for progressio, medialimits, operations, administration, httpapi, mediaanalysis, playbackv1, decoder, storage and cmd/server. Final TypeScript checking and `git diff --check` passed.
- Cumulative milestone snapshots: 001 (background controls/recovery), 002 (adjustable media limits/progress), 003 (consumer guard/owner boundary checks), 004 (stored playback preferences/resource recovery), and final 005 (finished verification). All are under `_program/snapshots/limits-settings/`; use the final cumulative snapshot for the complete tree. No worktree edit follows the final snapshot.

## Device and saturation acceptance

- On Windows/macOS/Linux service installs: keep a copy of the state folder, use a disposable corrupt database or deny that service account access, start the service, open `/` in a browser, and request `/v1/readiness`. Pass: useful recovery guidance, HTTP 503, no private path/credential leak, service remains reachable; restart after correcting the cause starts normally. Do not use a production database for this check.
- On a slow NAS: use a film with more than 32 subtitle tracks and a PGS sidecar above 32 MiB. Default limits should admit the ordinary file; a deliberately lower owner limit should warn/retain useful tracks; raising and rescanning should include the additional tracks. Verify both extraction and original/burn playback on actual clients.
- Over a shaped slow link: fetch a live segment, provider artwork and a large guide/subtitle input for longer than the old total deadline while bytes arrive. Pass: completion while progress continues, cancellation after a true idle stall, no loss of authorization or output bounds.
- With conversion slots filled and a scan running, set a small connectionLimit and keep ordinary media sockets open. The existing focused reserve test passes over HTTP and TLS. Full combined saturation is still an acceptance check: shell/login, admin session terminate, background pause/resume and restart must answer; another ordinary connection should be bounded/refused honestly. API-key scripts do not consume the human console reserve.
- On phone, TV and web: play consecutive episodes with protection on and count 2 or 3, verify the question occurs at that count, then Continue; with protection off, verify no question. Check autoplay-off and countdown choices separately. Audio queues should follow their existing listening policy.

# Not finished

- UI custom-panel action/usage presentation, native package synchronization and physical-device/platform acceptance are intentionally external work under this lane’s boundaries.
- The active scanning/freshness core-file items listed above (six-hour deep-analysis outer cap and the visibility worker’s missing cooperative pause boundary) are not changed here.
- Full simultaneous conversion + scan + connection saturation, live tuner hardware, slow NAS/network and GPU playback were not exercised. Focused tests/builds do not establish those capacities.
- No push, deployment or direct Git commit. Cumulative snapshots carry all tree changes for integration.

# Merge with the base

- `v1-queue-player.test.ts`: retain both independent tests: server still-watching/manual Continue and live-session subtitle delivery/timing changes. All 35 tests pass.
- `connection_policy.go`: retain serialized, revision-fenced settings projection and owner subtitle limits; include the base’s remote-sign-in normalization and trusted-proxy transport parser. The stale-receipt regression now also fences remote-sign-in policy.
- `route_lanes.go`: retain both owner action/readback routes in the admin lane and the base’s download bundle/artwork/subtitle/request routes; keep MCP/API credentials/webhooks classifications. Both lane completeness and mandatory credential-scope tests pass. This lane’s pause and resource-usage registry routes require `ScopeAdmin`.
- `preferences.go`: follow the base’s two saved qualities (`quality.home`, `quality.remote`) and expanded audiobook speed domain, keep effective still-watching controls, and publish combined registry revision `p40.Q2.B5`. Adapted the consumer guard to moved identity device approval, cached remote-sign-in policy, replacement quality readers and informational computed download choices; obsolete controls were not restored.
- Regenerated contracts/OpenAPI with apigen and baseline-derived persistence files with schema-sync. Linux, Windows amd64 and macOS arm64 `GOFLAGS=-p=2 go build ./...`, targeted operations tests, vet for operations/httpapi, web TypeScript checking and conflict-marker grep passed. Final targeted HTTP regressions (remote sign-in, proxies, credentials, scopes/lanes, pause/usage and still-watching), recovery/console-reserve regressions and focused foundation/maintenance contract refresh all passed. Retained the refreshed bootstrap and capabilities fixtures for the changed registry/API; discarded incidental random fixture-ID churn. The merged consumer inventory covers 120 stored control leaves and 41 current preference keys; the earlier inventory above records the pre-merge keys. Conflict-marker grep and `git diff --check` are clean.
- No staging, commit or additional snapshot; the earlier snapshot freeze was reopened only for this explicitly requested merge job. Git’s unmerged index entries remain for the owner to stage.
