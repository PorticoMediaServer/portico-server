import type {
  APIKey,
  APIKeyCreateResponse,
  BackupInfo,
  Device,
  DeviceUpdateRequest,
  DLNAStatus,
  Job,
  Library,
  AdminLibraryChannelListResponse,
  LibraryChannelAggregate,
  LibraryChannelConfigurationRequest,
  LibraryChannelGeneration,
  LibraryChannelRestoreDefaultsRequest,
  LibraryChannelRestoreDefaultsResponse,
  LibraryChannelTemplatesResponse,
  LiveTVSource,
  LiveTVSourceRequest,
  ListResponse,
  LogEvent,
  PlaybackSession,
  PorticoInvite,
  RemoteAccessSettingsPatch,
  RemoteAccessStatus,
  ScheduledTask,
  ScheduledTaskRunResponse,
  ScheduledTaskUpdateRequest,
  SettingsDocument,
  SettingsSummaryResponse,
  SettingsUpdateRequest,
  SystemStorageCleanupResponse,
  SystemStorageReport,
  TranscodeCapacityReport,
  User,
  UserPreferences,
  UserCreateRequest,
  UserPatchRequest,
} from '@portico/client-core';
import type {
  AccountIdentitySnapshot,
  AccountSignedInDevice,
  AccountMFAEnableResult,
  AccountMFASetup,
  AccountMFAStatus,
  AccountOrigin,
  DVRStatusSnapshot,
  LibraryMutationInput,
  LibraryScanOperationsResponse,
  LibraryScanReviewResponse,
  LibraryStorageSource,
  IdentityReconciliationReview,
  SettingsDataSource,
  SettingsOperationalSnapshot,
  SettingsStatusSnapshot,
  RestoreWorkflowResponse,
} from './settingsTypes';

const now = () => new Date().toISOString();
const ago = (minutes: number) => new Date(Date.now() - minutes * 60_000).toISOString();
const asFixture = <T,>(value: unknown): T => value as T;

const permissionCatalog = [
  'read', 'playMedia', 'downloadMedia', 'editMetadata', 'manageLibraries', 'manageUsers',
  'watchWithFriends', 'viewLiveTV', 'playLiveTV', 'viewDVR', 'scheduleDVR', 'manageDVR',
  'deleteDVRRecordings', 'transcode', 'manageServer',
];

const defaultPreferences: UserPreferences = {
  locale: 'en-CA',
  timeZone: 'America/Halifax',
  dateFormat: 'medium',
  hourCycle: 'auto',
  audioLanguage: 'en',
  subtitleLanguage: 'en',
  sidebarOrder: ['home', 'library:fixture-tv', 'library:fixture-music', 'library:fixture-movies', 'live-tv', 'saved'],
  playbackProgress: { startedThresholdPercent: 5, playedThresholdPercent: 95 },
  musicPlayback: { autoplayDefault: true, crossfadeSeconds: 0, gapless: true, normalizationMode: 'attenuate', repeatDefault: 'none', shuffleDefault: false },
  privacy: { includeInWatchWithFriends: true, pauseWatchHistory: false, showActivityToMembers: true },
};

function settingGroups() {
  return {
    server: { friendlyName: 'EhlerFlix Test', operatorNote: 'Primary Portico review server.' },
    library: {
      scanAutomatically: true, scanOnFilesystemChanges: true, analyzeOnScan: true, emptyTrashAfterScan: false,
      allowMediaDeletion: false, trashRetentionDays: 30, generateVideoPreview: 'scheduled', chapterThumbnailMode: 'chapters',
      trickplayOnScan: true, trickplayIntervalSeconds: 10, trickplayTileWidth: 240, trickplayMaxTiles: 1000,
    },
    metadataAgents: {
      movies: 'TMDB', tv: 'TMDB', anime: 'AniList', music: 'MusicBrainz', localNFO: true, embeddedTags: true,
      cacheOriginalArtwork: true, metadataLanguage: 'en-CA', refreshDays: 30,
      tmdbReadAccessToken: { present: true }, tmdbAPIKey: { present: false },
    },
    languages: { audio: 'en', subtitle: 'en', subtitleMode: 'foreignAudio', preferForcedSubs: true },
    transcoder: {
      enabled: true, planningPolicy: 'maximum_fidelity', temporaryDirectory: '/var/lib/portico/transcode', maxConcurrentSessions: 3,
      x264Preset: 'veryfast', throttleBufferSeconds: 60, playedRetentionSeconds: 90, directStreamRemux: true,
      hardwareAcceleration: true, hardwareEncoding: true, hardwareDecodeHEVC: true, hardwareDevice: 'auto',
      maxHardwareSessions: 2, maxSoftwareSessions: 2, maxBackgroundSessions: 1,
      hdrToneMapping: true, hdrToneMappingAlgorithm: 'hable',
    },
    optimizedVersions: {
      defaultProfile: 'universal-720p', preferOptimizedPlayback: true, storageDirectory: '/var/lib/portico/optimized',
      templates: [
        { id: 'preset-universal-1080p', name: 'Universal 1080p', profile: 'universal-1080p', enabled: true },
        { id: 'preset-universal-720p', name: 'Universal 720p', profile: 'universal-720p', enabled: true },
        { id: 'preset-universal-480p', name: 'Universal 480p', profile: 'universal-480p', enabled: true },
		{ id: 'preset-efficient-4k', name: 'Efficient 4K', profile: 'efficient-4k', enabled: true },
		{ id: 'preset-efficient-1080p', name: 'Efficient 1080p', profile: 'efficient-1080p', enabled: true },
		{ id: 'preset-efficient-720p', name: 'Efficient 720p', profile: 'efficient-720p', enabled: true },
		{ id: 'preset-maximum-compression-source', name: 'Maximum Compression Source Size', profile: 'maximum-compression-source', enabled: true },
		{ id: 'preset-maximum-compression-1080p', name: 'Maximum Compression 1080p', profile: 'maximum-compression-1080p', enabled: true },
      ],
      maxConcurrentJobs: 1, maxPerItem: 3, autoDelete: true, retentionDays: 90, maxStorageMB: 256000,
    },
    dvr: {
      defaultStartPaddingMinutes: 2, defaultEndPaddingMinutes: 5, defaultRetentionDays: 30, defaultFolder: 'Recorded TV',
      defaultMaxRecordingsPerSeries: 10, recordingProfile: 'copy', recordingPathTemplate: '{folder}/{title}-{start}',
      preserveAllStreams: true, convertRecordings: false, saveNFO: true, saveImageSidecars: true,
      defaultGuideRefreshIntervalHours: 12, defaultGuideRequireEpg: false, guideChannelAutoMatch: true,
      defaultRuleRequiredKeywords: [], defaultRuleBlockedKeywords: [], defaultRuleAllowedChannels: [], defaultRuleBlockedChannels: [],
    },
    network: { secureConnections: 'preferred', lanNetworks: '192.168.0.0/16, 10.0.0.0/8', customAccessUrls: '' },
    dlna: { enabled: false, friendlyName: 'EhlerFlix Test', advertiseUrl: '', exposedLibraries: [], reportTimeline: true },
    devices: { requireTrustedDevices: false, quickConnectApprovalMode: 'allUsers' },
    scheduledTasks: {
      enabled: true, maintenanceWindow: 'overnight', maintenanceDays: 'every-day', startHour: 2, endHour: 5,
      scanLibraries: true, libraryScanCadence: 'daily', libraryScanIntervalHours: 24, refreshMetadata: true,
      metadataRefreshCadence: 'weekly', metadataRefreshDays: 30, analyzeMedia: true, analysisCadence: 'daily',
      backupDatabase: true, backupCadence: 'daily', backupRetentionDays: 14, emptyTrash: true, trashRetentionDays: 30,
      trickplayRetentionDays: 90, trickplayMaxStorageMB: 51200, trickplayIntervalSeconds: 10, trickplayTileWidth: 240, trickplayMaxTiles: 1000,
    },
    notifications: { enabled: true, minAlertLevel: 'warn' },
    retention: { playbackDetailDays: 0, playbackHistoryDays: 0, auditHistoryDays: 90, diagnosticHistoryDays: 30, clientDiagnosticHistoryDays: 30, jobHistoryDays: 30, authRequestDays: 14, deviceIPDays: 30 },
    troubleshooting: { logLevel: 'info', debugDurationMinutes: 60, clientLogUploads: true },
  };
}

function summaryGroups(): SettingsSummaryResponse['groups'] {
  const definitions: Array<[string, string, string]> = [
    ['server', 'Server identity', 'Name and operator details'], ['updates', 'Updates', 'This feature is not yet available.'],
    ['libraries', 'Libraries', 'Media roots and scans'], ['library-settings', 'Library settings', 'Scanning and preview policy'],
    ['metadata-agents', 'Metadata agents', 'Providers and matching'],
    ['transcoder', 'Transcoder', 'Playback conversion policy'], ['languages', 'Languages', 'Default tracks and subtitles'],
    ['optimized-versions', 'Optimized versions', 'Managed compatible copies'], ['live-tv', 'Live TV', 'Sources and guide data'],
    ['dvr', 'DVR', 'Recording policy'], ['library-channels', 'Library Channels', 'Owner-built channels from library media'], ['remote-access', 'Remote Access', 'Free direct server access'],
    ['network', 'Network', 'Local and secure connection policy'], ['dlna', 'DLNA', 'Local network discovery'],
    ['users', 'Users', 'Profiles and permissions'], ['devices', 'Devices', 'Trusted application installations'],
    ['api-keys', 'API keys', 'Integration credentials'], ['scheduled-tasks', 'Scheduled tasks', 'Maintenance automation'],
    ['storage', 'Storage', 'Managed server data'], ['backups', 'Backups', 'Verified database backups'],
    ['notifications', 'Notifications', 'Administrator alerts'], ['retention', 'History & Retention', 'Owner-controlled local data retention'], ['troubleshooting', 'Troubleshooting', 'Diagnostic policy'],
    ['console', 'Server console', 'Recent redacted events'],
  ];
  return definitions.map(([id, label, summary]) => ({
    id, label, summary, category: id === 'console' || id === 'storage' ? 'support' : id === 'libraries' || id === 'users' || id === 'backups' ? 'operate' : 'configure',
    implemented: id !== 'updates', readOnly: id === 'updates', configured: id !== 'updates', dangerous: false, requiresAdmin: true,
    requiresPorticoClaim: id === 'remote-access', requiresRuntimeDependency: id === 'transcoder', status: id === 'updates' ? 'unavailable' : 'ready',
  })) as SettingsSummaryResponse['groups'];
}

function remoteStatus(): RemoteAccessStatus {
  return asFixture<RemoteAccessStatus>({
    generatedAt: now(),
    serverPublicKey: 'fixture-public-key',
    serverPublicKeyFingerprint: 'SHA256:fixture',
    localRoutes: [{ type: 'lan', source: 'interface', url: 'https://ehlerflix.local:32500' }],
    localTlsAddress: 'https://ehlerflix.local:32500', localTlsPort: 32500, localTlsPortMatchesPublic: true,
    porticoConnected: true, porticoMembers: [],
    publicEndpoint: { scheme: 'https', host: 'ehlerflix.portico.direct', port: 443, url: 'https://ehlerflix.portico.direct' },
    connectivity: { hostedServicesStatus: 'reachable', publicRouteStatus: 'reachable', troubleshootingStatus: 'ok', lastCheckedAt: ago(2), troubleshootingHint: 'Hosted Services can reach this server\'s public route.' },
    policySync: { memberCount: 1, note: 'Current', stale: false, status: 'current', lastSyncedAt: ago(2) },
    settings: {
      allowManualLocalAuthRemoteLogin: false, assignedHostname: 'ehlerflix.portico.direct', certificateStatus: 'valid',
      certificateExpiresAt: new Date(Date.now() + 42 * 86_400_000).toISOString(), claimStatus: 'claimed', customCertificateEnabled: false,
      enabled: true, hostedBaseUrl: 'https://api.getportico.tv', lanDiscoveryEnabled: true, lastHeartbeatAt: ago(2),
      lastHostedRemoteAccessState: 'enabled', lastReachabilityCheckAt: ago(2), lastReachabilityResult: 'reachable',
      lastRouterMappingAt: ago(8), manualPublicPort: 0, preferredRemoteAuthMode: 'portico', publicPortMode: 'automatic',
      remoteBitrateLimitMbps: 0, routerAutomationEnabled: true, routerMappingStatus: 'mapped', serverId: 'fixture-server',
    },
  });
}

function liveTVSources(): LiveTVSource[] {
  return [asFixture<LiveTVSource>({
    id: 'fixture-live-source',
    name: 'HDHomeRun Living Room',
    type: 'hdhomerun',
    enabled: true,
    hdhomerunBaseUrl: 'http://192.168.1.50',
    hasM3uText: false,
    hasEpgText: false,
    hasXtreamPassword: false,
    streamBufferSeconds: 18,
    maxRetrySeconds: 45,
    refreshIntervalHours: 12,
    tunerCount: 2,
    discoveredTunerCount: 2,
    tunerCountMode: 'discovered',
    userAgent: 'Portico',
    sortOrder: 0,
    createdAt: ago(1440),
    updatedAt: ago(18),
    filterCategories: [],
    filterCountries: [],
    filterRequireEpg: false,
    keywordAllow: [],
    keywordDeny: [],
    channelCount: 81,
    programCount: 1294,
    logoCount: 76,
    hiddenChannelCount: 4,
    favoriteChannelCount: 8,
    lastRefreshedAt: ago(18),
    actions: ['live.source.edit', 'live.source.refresh', 'live.source.delete'],
  })];
}

function dvrStatus(): DVRStatusSnapshot {
  return {
    configured: true,
    available: true,
    capabilities: {
      canScheduleRecordings: true,
      canManageRecordingRules: true,
      canCreateOwnRules: true,
      canEditOwnRules: true,
      canDeleteOwnRules: true,
      canManageAllRules: true,
      actions: ['dvr.record', 'dvr.record-series', 'dvr.edit', 'dvr.delete'],
    },
    guide: { state: 'current', lastRefreshedAt: ago(18) },
    conflicts: [],
    tuners: [
      { id: 'fixture-tuner-1', name: 'HDHomeRun tuner 1', state: 'recording', channelId: 'fixture-channel-4', recordingId: 'fixture-recording-1' },
      { id: 'fixture-tuner-2', name: 'HDHomeRun tuner 2', state: 'idle' },
    ],
    storage: { usedBytes: 536870912000, availableBytes: 1649267441664, forecastDays: 46, state: 'healthy' },
    generatedAt: now(),
  };
}

function transcodeCapacity(): TranscodeCapacityReport {
  return asFixture<TranscodeCapacityReport>({
    enabled: true,
    maxConcurrentSessions: 3,
    activeSessions: 1,
    availableSlots: 2,
    temporaryDirectory: '/var/lib/portico/transcode',
    temporaryDirectoryReady: true,
    x264Preset: 'veryfast',
    throttleBufferSeconds: 60,
    hardwareAcceleration: true,
    hardwareEncoding: true,
    hardwareDevice: 'auto',
    hardwareDecodeValue: 'hevc,h264',
    hardwareEncoder: 'videotoolbox',
    hardwareEncoderAvailable: true,
    hardwareSupportLevel: 'available',
    hardwareProbes: [],
    hdrToneMapping: true,
    hdrToneMappingAvailable: true,
    hdrToneMappingStatus: 'available',
    hdrToneMappingDetail: 'VideoToolbox and FFmpeg filters are available.',
    directStreamRemux: true,
    ffmpeg: { name: 'FFmpeg', configuredPath: 'ffmpeg', resolvedPath: '/opt/homebrew/bin/ffmpeg', available: true, versionLine: 'ffmpeg 7.1' },
    ffprobe: { name: 'FFprobe', configuredPath: 'ffprobe', resolvedPath: '/opt/homebrew/bin/ffprobe', available: true, versionLine: 'ffprobe 7.1' },
    presets: [],
    warnings: [],
    generatedAt: now(),
  });
}

function job(id: string, message: string, status: Job['status'], progress: number, minutes: number): Job {
  return { id, type: id, message, status, progress, createdAt: ago(minutes + 2), updatedAt: ago(minutes) };
}

function library(id: string, name: string, type: Library['type'], count: number, path: string): Library {
  return { id, name, type, count, path, paths: [path], settings: {}, sortOrder: 0, scanSummary: asFixture({ status: 'completed' }) };
}

function user(id: string, displayName: string, role: User['role'], libraryIds: string[], authOrigin: User['authOrigin']): User {
  return asFixture<User>({
    id, profileId: id, profileIdentityId: `${id}-identity`, username: displayName.toLocaleLowerCase().replaceAll(' ', ''),
    displayName, email: `${displayName.toLocaleLowerCase().replaceAll(' ', '.')}@portico.local`, role, authOrigin,
    authProvider: authOrigin, hasLocalPassword: authOrigin === 'local', libraryIds, permissions: Object.fromEntries(permissionCatalog.map((key) => [key, role !== 'user' || !key.startsWith('manage')])),
    preferences: structuredClone(defaultPreferences),
  });
}

function operationalSnapshot(): SettingsOperationalSnapshot {
  const libraries = [
    library('fixture-tv', 'TV Shows', 'show', 128, '/media/tv'),
    library('fixture-movies', 'Movies', 'movie', 342, '/media/movies'),
    library('fixture-music', 'Music', 'music', 2814, '/media/music'),
  ];
  return asFixture<SettingsOperationalSnapshot>({
    libraries,
    users: [user('fixture-owner', 'Portico Review', 'owner', libraries.map((item) => item.id), 'local'), user('fixture-member', 'Living Room', 'user', ['fixture-tv', 'fixture-movies'], 'portico')],
    devices: [
      { id: 'device-browser', name: 'Work Mac', autoName: 'Chrome on macOS', app: 'Portico Web', platform: 'macOS', user: 'Portico Review', userId: 'fixture-owner', createdAt: ago(43200), lastSeenAt: now(), sessionCount: 1, trusted: true, options: {} },
      { id: 'device-tv', name: 'Living Room TV', autoName: 'Apple TV', app: 'Portico TV', platform: 'tvOS', user: 'Living Room', userId: 'fixture-member', createdAt: ago(20000), lastSeenAt: ago(18), sessionCount: 1, trusted: true, options: {} },
    ],
    apiKeys: [{ id: 'key-home', userId: 'fixture-owner', name: 'Home automation', lastFour: '8Q2M', scopes: ['read'], createdAt: ago(10080), lastUsedAt: ago(22) }],
    tasks: [
      { id: 'library_scan', category: 'library', title: 'Library scan', description: 'Find new and changed media', enabled: true, running: true, schedule: 'Daily at 2:00 AM', jobType: 'library_scan', trigger: { enabled: true, intervalHours: 24 }, lastJob: job('scan-tv', 'Scanning TV Shows', 'running', 62, 1) },
      { id: 'metadata_refresh', category: 'metadata', title: 'Metadata refresh', description: 'Refresh stale metadata and artwork', enabled: true, running: false, schedule: 'Weekly', jobType: 'metadata_refresh', trigger: { enabled: true, intervalHours: 168 }, lastJob: job('metadata-last', 'Metadata refresh completed', 'complete', 100, 680) },
      { id: 'database_backup', category: 'maintenance', title: 'Database backup', description: 'Create a verified SQLite backup', enabled: true, running: false, schedule: 'Daily at 3:00 AM', jobType: 'database_backup', trigger: { enabled: true, intervalHours: 24 }, lastJob: job('backup-last', 'Database backup completed', 'complete', 100, 310) },
    ],
    backups: [{ name: 'portico-2026-07-11T03-00-00.db', createdAt: ago(310), integrity: 'ok', manifestPresent: true, restoreReady: true, sizeBytes: 34865152 }],
    sessions: [
      { id: 'session-current', app: 'Portico Web', authProvider: 'local', canRevoke: false, current: true, deviceName: 'Work Mac', platform: 'Chrome · macOS', trusted: true, createdAt: ago(240), lastSeenAt: now(), expiresAt: new Date(Date.now() + 86_400_000).toISOString() },
      { id: 'session-tv', app: 'Portico TV', authProvider: 'portico', canRevoke: true, current: false, deviceName: 'Living Room TV', platform: 'tvOS', trusted: true, createdAt: ago(10080), lastSeenAt: ago(18), expiresAt: new Date(Date.now() + 20 * 86_400_000).toISOString() },
    ],
    release: { version: '0.0.0-development', apiVersion: 'v1', generatedAt: now(), goos: 'darwin', goarch: 'arm64', installMethod: 'bundled', migrationStatus: 'current', updateStatus: 'unavailable', appDataReady: true, databaseReady: true, webDistReady: true },
    diagnostics: {
      addr: '127.0.0.1:32500', appDataReady: true, databaseReady: true, generatedAt: now(), goarch: 'arm64', goos: 'darwin', version: '0.0.0-development', webDistReady: true,
      mediaToolchain: { source: 'bundled', status: 'available', reasonCode: 'verified_manifest', target: 'darwin-arm64', buildId: 'fixture', ffmpegVersion: '7.1', licenseMode: 'gpl', manifestPresent: true, verified: true, features: [] },
      dependencies: [
        { name: 'FFmpeg', available: true, configuredPath: 'ffmpeg', resolvedPath: '/opt/homebrew/bin/ffmpeg', versionLine: 'ffmpeg 7.1' },
        { name: 'FFprobe', available: true, configuredPath: 'ffprobe', resolvedPath: '/opt/homebrew/bin/ffprobe', versionLine: 'ffprobe 7.1' },
      ],
      resources: { status: 'normal', activePlaybackSessions: 2, activeTranscodeSessions: 1, availableTranscodeSlots: 2, maxTranscodeSessions: 3, runningBackgroundJobs: 1, queuedBackgroundJobs: 0, deferredMaintenanceJobs: 0, failedMaintenanceJobs: 0, backgroundJobsDeferred: false, degradationActions: [], signals: [], saturatedJobLanes: [], saturatedWorkloadLanes: [], sqliteInUseConnections: 2, sqliteMaxOpenConnections: 12 },
      sqlite: { journalMode: 'WAL', databaseBytes: 34865152, openConnections: 3, idleConnections: 1, inUseConnections: 2, maxOpenConnections: 12, lockRetries: 0, lockRetryWaitMillis: 0, readErrors: 0, readOperations: 18204, shmBytes: 32768, slowestReadMillis: 4, slowestWriteMillis: 7, waitCount: 0, waitDurationMillis: 0, walAutoCheckpointPages: 1000, walBytes: 524288, writeAttempts: 831, writeOperations: 831, readLatency: [], writeLatency: [] },
      sqliteHealth: { status: 'healthy', consecutiveFailures: 0, consecutiveSuccesses: 428, evidenceCaptures: 0, lastProbeDurationMillis: 2, lastSuccessfulProbeAt: ago(1), recycleAttempts: 0, recycleSuccesses: 0 },
      runtime: { startedAt: ago(7200), uptimeSeconds: 432000, goroutines: 48, heapAllocBytes: 314572800, heapIdleBytes: 104857600, heapReleasedBytes: 52428800, heapSysBytes: 524288000, stackInUseBytes: 8388608, lastGcPauseMillis: 1, totalGcPauseMillis: 42, nextGcBytes: 629145600, numGc: 184, ioPressure: {} },
      startup: { startedAt: ago(7200), status: 'ready', httpReady: true, httpReadyAt: ago(7199), nonCriticalWorkReady: true, phases: [] },
      jobLanes: [], workloadLanes: [],
    },
    capabilities: { version: '0.0.0-development', apiVersion: 'v1', generatedAt: now(), features: { remoteAccess: true, liveTV: true, dvr: true, watchWithFriends: true }, permissionCatalog, permissions: Object.fromEntries(permissionCatalog.map((key) => [key, true])), markerTypes: ['intro', 'recap', 'credits', 'commercial', 'chapter', 'preview'], extraTypes: ['trailer', 'featurette', 'deleted_scene', 'behind_the_scenes', 'interview', 'scene', 'short', 'other'] },
    storage: { generatedAt: now(), totalBytes: 39728447488, categories: [
      { key: 'database', label: 'Database and backups', sizeBytes: 268435456, fileCount: 18, available: true, writable: true, cleanupSupported: false },
      { key: 'transcode', label: 'Transcode cache', sizeBytes: 25769803776, fileCount: 642, available: true, writable: true, cleanupSupported: true },
      { key: 'optimized', label: 'Optimized versions', sizeBytes: 13690208256, fileCount: 28, available: true, writable: true, cleanupSupported: true },
    ] },
  });
}

export class FixtureSettingsDataSource implements SettingsDataSource {
  private document: SettingsDocument = asFixture({ revision: 'fixture-settings-1', updatedAt: now(), groups: settingGroups(), restartRequired: false, restartRequiredFields: [], applyImpact: { changedFields: [], restartRequired: false, restartRequiredFields: [] } });
  private preferencesValue = structuredClone(defaultPreferences);
  private operations = operationalSnapshot();
  private remote = remoteStatus();
  private liveTVSourceValue = liveTVSources();
  private dvrStatusValue = dvrStatus();
  private accountMFAValue: AccountMFAStatus = { enabled: false, setupStarted: false, recoveryCodesSupported: true, recoveryCodesRemaining: 0 };
  private porticoDevices: AccountSignedInDevice[] = [
    { id: 'portico-device-web', authority: 'portico', name: 'Work Mac', platform: 'macOS', current: false, canRevoke: true, lastSeenAt: now() },
    { id: 'portico-device-tv', authority: 'portico', name: 'Living Room TV', platform: 'tvOS', current: false, canRevoke: true, lastSeenAt: ago(18) },
  ];
  private streams: PlaybackSession[] = [asFixture<PlaybackSession>({
    id: 'fixture-stream-fargo', userId: 'fixture-owner', user: 'Portico Review', device: 'Work Mac', app: 'Chrome', location: 'Local', clientIp: '192.168.1.42',
    state: 'playing', isLive: false, startedAt: ago(31), lastSeenAt: now(), positionSeconds: 1884, progress: 65.4, bandwidthMbps: 38.2,
    decision: 'Direct play', videoSource: 'HEVC · 4K HDR', videoDecision: 'Direct play', videoTarget: 'Original', audioSource: 'EAC3 · 5.1', audioDecision: 'Direct play', audioTarget: 'English',
    command: {}, media: { id: 'fargo', type: 'episode', title: 'The Castle', durationSeconds: 2880, images: { poster: 'https://image.tmdb.org/t/p/w780/a3VW6khsyUVKrG0GBCWFG3NzWPX.jpg', backdrop: 'https://image.tmdb.org/t/p/w1280/4jrSbRpLqpvYJtLKncaxZVC47EW.jpg', thumb: 'https://image.tmdb.org/t/p/w1280/4jrSbRpLqpvYJtLKncaxZVC47EW.jpg' } },
  })];

  settings(): Promise<SettingsDocument> { return Promise.resolve(structuredClone(this.document)); }
  settingsSummary(): Promise<SettingsSummaryResponse> { return Promise.resolve({ generatedAt: now(), groups: summaryGroups(), statusCards: [] }); }
  updateSettings(input: SettingsUpdateRequest): Promise<SettingsDocument> {
    this.document = asFixture({ ...this.document, revision: `fixture-settings-${Date.now()}`, updatedAt: now(), groups: { ...this.document.groups, ...input.groups }, applyImpact: { changedFields: Object.entries(input.groups).flatMap(([group, fields]) => Object.keys(fields ?? {}).map((field) => `${group}.${field}`)), restartRequired: false, restartRequiredFields: [] } });
    return Promise.resolve(structuredClone(this.document));
  }
  settingsStatus(): Promise<SettingsStatusSnapshot> {
    const currentJobs = this.operations.tasks.flatMap((task) => task.lastJob ? [task.lastJob] : []);
    return Promise.resolve(asFixture({
      generatedAt: now(), remoteAccess: structuredClone(this.remote), storage: structuredClone(this.operations.storage), jobs: currentJobs,
      activity: { serverName: 'EhlerFlix Test', generatedAt: now(), cpuPercent: 28, cpuStatus: { status: 'ok' }, memoryUsedBytes: 6227702579, memoryTotalBytes: 25769803776, memoryFreeBytes: 19542101197, memoryStatus: { status: 'ok' }, bandwidthMbps: 46.6, activeStreams: this.streams.length, activeTranscodes: 0, refreshAfterMs: 5000 },
      dashboard: { generatedAt: now(), mode: 'live', period: '1h', nowPlaying: structuredClone(this.streams), jobs: currentJobs, transcodes: [], alerts: [{ id: 'guide-warning', level: 'warn', title: 'One recording rule needs review', message: 'No future airings currently match the Fargo rule', time: ago(45) }], metrics: [], bandwidth: [], conversions: [], libraries: [], playHistory: [], topPlayed: [], topUsers: [], watchedToday: { audiobookSeconds: 0, durationSeconds: 1884, liveTvSeconds: 0, moviesSeconds: 0, musicSeconds: 0, sessions: 1, tvSeconds: 1884, users: 1 }, system: { cpu: [], ram: [], gpu: [], diskIo: [], gpuInfo: { available: false, device: 'No GPU telemetry collector detected', provider: 'Unknown', note: 'The fixture does not provide a GPU telemetry collector.' } } },
    }));
  }
  runConnectivityCheck(): Promise<RemoteAccessStatus> { this.remote = { ...this.remote, generatedAt: now(), settings: { ...this.remote.settings, lastReachabilityCheckAt: now(), lastReachabilityResult: 'reachable' } }; return Promise.resolve(structuredClone(this.remote)); }
  stopPlayback(sessionId: string): Promise<void> { this.streams = this.streams.filter((stream) => stream.id !== sessionId); return Promise.resolve(); }
  remoteAccess(): Promise<RemoteAccessStatus> { return Promise.resolve(structuredClone(this.remote)); }
  updateRemoteAccess(input: RemoteAccessSettingsPatch): Promise<RemoteAccessStatus> { this.remote = { ...this.remote, generatedAt: now(), settings: { ...this.remote.settings, ...input } }; return Promise.resolve(structuredClone(this.remote)); }
  startRemoteAccessClaim(): Promise<RemoteAccessStatus> { this.remote = asFixture({ ...this.remote, claim: { claimId: 'fixture-claim', claimUrl: 'https://app.getportico.tv/claim/fixture', expiresAt: new Date(Date.now() + 900000).toISOString(), hostedReady: true, startedAt: now(), status: 'pending' } }); return Promise.resolve(structuredClone(this.remote)); }
  cancelRemoteAccessClaim(): Promise<RemoteAccessStatus> { const { claim: _claim, ...rest } = this.remote; this.remote = rest as RemoteAccessStatus; return Promise.resolve(structuredClone(this.remote)); }
  unclaimRemoteAccess(): Promise<RemoteAccessStatus> { this.remote = { ...this.remote, porticoConnected: false, settings: { ...this.remote.settings, claimStatus: 'unclaimed', preferredRemoteAuthMode: 'local' } }; return Promise.resolve(structuredClone(this.remote)); }
  renewRemoteAccessCertificate(): Promise<RemoteAccessStatus> { this.remote = { ...this.remote, settings: { ...this.remote.settings, certificateStatus: 'valid', certificateExpiresAt: new Date(Date.now() + 90 * 86400000).toISOString() } }; return Promise.resolve(structuredClone(this.remote)); }
  settingsOperations(): Promise<SettingsOperationalSnapshot> { return Promise.resolve(structuredClone(this.operations)); }
  liveTVSources(): Promise<LiveTVSource[]> { return Promise.resolve(structuredClone(this.liveTVSourceValue)); }
  createLiveTVSource(input: LiveTVSourceRequest, testBeforeSaving: boolean): Promise<LiveTVSource> {
    const created = asFixture<LiveTVSource>({
      ...input,
      id: `fixture-live-source-${Date.now()}`,
      hasM3uText: Boolean(input.m3uText),
      hasEpgText: Boolean(input.epgText),
      hasXtreamPassword: Boolean(input.xtreamPassword),
      streamBufferSeconds: input.streamBufferSeconds || 18,
      maxRetrySeconds: input.maxRetrySeconds || 45,
      refreshIntervalHours: input.refreshIntervalHours || 12,
      filterCategories: input.filterCategories ?? [],
      filterCountries: input.filterCountries ?? [],
      filterRequireEpg: input.filterRequireEpg ?? false,
      keywordAllow: input.keywordAllow ?? [],
      keywordDeny: input.keywordDeny ?? [],
      channelCount: testBeforeSaving ? 42 : 0,
      programCount: testBeforeSaving ? 640 : 0,
      logoCount: testBeforeSaving ? 37 : 0,
      hiddenChannelCount: 0,
      favoriteChannelCount: 0,
      lastRefreshedAt: testBeforeSaving ? now() : undefined,
      actions: ['live.source.edit', 'live.source.refresh', 'live.source.delete'],
    });
    this.liveTVSourceValue.push(created);
    return Promise.resolve(structuredClone(created));
  }
  updateLiveTVSource(id: string, input: LiveTVSourceRequest): Promise<LiveTVSource> {
    const index = this.liveTVSourceValue.findIndex((source) => source.id === id);
    if (index < 0) return Promise.reject(new Error('Live TV source not found.'));
    const current = this.liveTVSourceValue[index];
    const updated = asFixture<LiveTVSource>({
      ...current,
      ...input,
      id,
      hasM3uText: Boolean(input.m3uText) || (input.preserveM3uText === true && current.hasM3uText),
      hasEpgText: Boolean(input.epgText) || (input.preserveEpgText === true && current.hasEpgText),
      hasXtreamPassword: Boolean(input.xtreamPassword) || (input.preserveXtreamPassword === true && current.hasXtreamPassword),
    });
    this.liveTVSourceValue[index] = updated;
    return Promise.resolve(structuredClone(updated));
  }
  refreshLiveTVSource(id: string): Promise<LiveTVSource> {
    const index = this.liveTVSourceValue.findIndex((source) => source.id === id);
    if (index < 0) return Promise.reject(new Error('Live TV source not found.'));
    const updated = { ...this.liveTVSourceValue[index], lastRefreshedAt: now(), lastError: undefined };
    this.liveTVSourceValue[index] = updated;
    this.dvrStatusValue = { ...this.dvrStatusValue, guide: { state: 'current', lastRefreshedAt: updated.lastRefreshedAt }, generatedAt: now() };
    return Promise.resolve(structuredClone(updated));
  }
  deleteLiveTVSource(id: string): Promise<void> { this.liveTVSourceValue = this.liveTVSourceValue.filter((source) => source.id !== id); return Promise.resolve(); }
  dvrStatus(): Promise<DVRStatusSnapshot> { return Promise.resolve(structuredClone(this.dvrStatusValue)); }
  libraryChannels(): Promise<AdminLibraryChannelListResponse> {
    return Promise.resolve(asFixture({ sourceType: 'library-channel', items: [{ id: 'fixture-library-channel', sourceType: 'library-channel', name: 'Saturday Morning', description: 'Cartoons and family adventures from the library.', enabled: true, sortOrder: 0, timezone: 'America/Halifax', defaultRuleId: 'fixture-library-rule', qualityProfile: 'auto', logo: { source: 'built_in', ref: 'saturday-morning', url: '/api/library-channels/logos/builtin-saturday-morning', mimeType: 'image/svg+xml', bugEnabled: false, bugOverheadAccepted: false, bugCorner: 'top_right', bugWidthPercent: 9, bugInsetPercent: 2, bugTreatment: 'color' }, configRevision: 1, healthState: 'healthy', generatedThrough: new Date(Date.now() + 7 * 86_400_000).toISOString(), createdAt: now(), updatedAt: now() }], pageInfo: { hasMore: false, nextCursor: null, total: 1 } }));
  }
  async libraryChannel(id: string): Promise<LibraryChannelAggregate> {
    const channel = (await this.libraryChannels()).items.find((item) => item.id === id);
    if (!channel) throw new Error('Library Channel not found.');
    return asFixture({ channel, rules: [{ id: 'fixture-library-rule', name: 'Family animation', enabled: true, sortOrder: 0, query: { genres: ['Animation'] }, selectionMode: 'shuffle_bag', episodeMode: 'in_order', exhaustionMode: 'loop', dedupeWindow: 12, maxConsecutive: 4, config: {} }], blocks: [] });
  }
  createLibraryChannel(_input: LibraryChannelConfigurationRequest): Promise<LibraryChannelAggregate> { return Promise.reject(new Error('Fixture channel creation is unavailable.')); }
  async updateLibraryChannel(id: string, _input: LibraryChannelConfigurationRequest): Promise<LibraryChannelAggregate> { return this.libraryChannel(id); }
  deleteLibraryChannel(): Promise<void> { return Promise.resolve(); }
  libraryChannelTemplates(): Promise<LibraryChannelTemplatesResponse> { return Promise.resolve(asFixture({ templates: [], blockPresets: [] })); }
  restoreLibraryChannelDefaults(_input: LibraryChannelRestoreDefaultsRequest): Promise<LibraryChannelRestoreDefaultsResponse> { return Promise.resolve({ items: [], createdCount: 0, existingCount: 1, skippedCount: 0 }); }
  regenerateLibraryChannel(id: string): Promise<LibraryChannelGeneration> { const horizonStart = now(); const horizonEnd = new Date(Date.now() + 7 * 86_400_000).toISOString(); return Promise.resolve(asFixture({ channelId: id, generationId: `fixture-generation-${id}`, configRevision: 1, horizonStart, horizonEnd, generatedThrough: horizonEnd, entryCount: 168, warnings: [] })); }
  uploadLibraryChannelLogo(_file: File): Promise<{ id: string; source: 'custom'; mimeType: 'image/png'; url: string; width: number; height: number; bytes: number; sha256: string }> { return Promise.resolve({ id: 'fixture-logo', source: 'custom', mimeType: 'image/png', url: '/brand/portico-app-icon-192.png', width: 192, height: 192, bytes: 1, sha256: '0'.repeat(64) }); }
  transcodeCapacity(): Promise<TranscodeCapacityReport> { return Promise.resolve(structuredClone(transcodeCapacity())); }
  systemStorage(): Promise<SystemStorageReport> { return Promise.resolve(structuredClone(this.operations.storage)); }
  cleanupStorage(): Promise<SystemStorageCleanupResponse> {
    const removed: Record<string, number> = {};
    this.operations.storage = {
      ...this.operations.storage,
      generatedAt: now(),
      categories: this.operations.storage.categories.map((category) => {
        if (!category.cleanupSupported) return category;
        const removedFiles = Math.min(category.fileCount, Math.max(1, Math.round(category.fileCount * 0.08)));
        removed[category.key] = removedFiles;
        return { ...category, fileCount: category.fileCount - removedFiles, sizeBytes: Math.round(category.sizeBytes * 0.92) };
      }),
    };
    this.operations.storage.totalBytes = this.operations.storage.categories.reduce((total, category) => total + category.sizeBytes, 0);
    return Promise.resolve({ ok: true, removed, storage: structuredClone(this.operations.storage), completedAt: now() });
  }
  dlnaStatus(): Promise<DLNAStatus> {
    const group = this.document.groups.dlna;
    const exposed = new Set(group?.exposedLibraries ?? []);
    return Promise.resolve(asFixture<DLNAStatus>({
      enabled: group?.enabled ?? false,
      friendlyName: group?.friendlyName ?? 'EhlerFlix Test',
      advertiseUrl: group?.advertiseUrl ?? '',
      deviceDescriptionUrl: 'http://192.168.1.20:32500/dlna/device.xml',
      contentDirectoryUrl: 'http://192.168.1.20:32500/dlna/content-directory',
      mediaServerUrn: 'urn:schemas-upnp-org:device:MediaServer:1',
      ssdpDiscovery: group?.enabled ? 'active when UDP port 1900 can be bound on this host' : 'disabled',
      exposedLibraries: this.operations.libraries.map((library) => ({ id: library.id, name: library.name, type: library.type, count: library.count, exposed: exposed.has(library.id) })),
      unauthenticatedLanAccess: group?.enabled ?? false,
      byteRangeStreamingSupported: true,
      note: 'DLNA is available only to devices that can reach this server on the local network.',
    }));
  }
  createLibrary(input: LibraryMutationInput): Promise<Library> {
    const primaryPath = input.paths[0] ?? input.path ?? '/media';
    const paths = input.paths.length > 0 ? [...input.paths] : [primaryPath];
    const created = { ...library(`fixture-library-${Date.now()}`, input.name, input.type, 0, primaryPath), path: primaryPath, paths, settings: input.settings ?? {} };
    this.operations.libraries.push(created);
    return Promise.resolve(structuredClone(created));
  }
  updateLibrary(id: string, input: LibraryMutationInput): Promise<Library> { const index = this.operations.libraries.findIndex((item) => item.id === id); const updated = { ...this.operations.libraries[index], ...input } as Library; this.operations.libraries[index] = updated; return Promise.resolve(structuredClone(updated)); }
  deleteLibrary(id: string): Promise<void> { this.operations.libraries = this.operations.libraries.filter((item) => item.id !== id); return Promise.resolve(); }
  libraryScanOperations(id: string): Promise<LibraryScanOperationsResponse> {
    const libraryItem = this.operations.libraries.find((item) => item.id === id);
    const summary = libraryItem?.scanSummary;
    const detail = summary as typeof summary & { mode?: 'targeted' | 'quick' | 'reconcile' | 'force_full' | 'remove_missing'; phase?: string; absenceAuthoritative?: boolean; cleanupAllowed?: boolean; confirmedMissingCount?: number; counts?: { processed?: number; added?: number; updated?: number; missing?: number; metadataPending?: number; analysisPending?: number }; warnings?: string[]; roots?: Array<{ id?: string; path?: string; status?: string; error?: string; errorClass?: string }> };
    const terminal = summary && summary.status !== 'queued' && summary.status !== 'running';
    return Promise.resolve({
      libraryId: id,
      operation: summary && (summary.status === 'queued' || summary.status === 'running') ? {
        jobId: summary.jobId ?? `scan-${id}`, status: summary.status, mode: detail.mode ?? 'reconcile', trigger: 'fixture', progress: summary.progress ?? 0,
        phase: { code: detail.phase ?? summary.status, label: detail.phase ?? (summary.status === 'running' ? 'Scanning' : 'Queued'), state: summary.status === 'running' ? 'active' : 'pending' },
        message: summary.message ?? '', attemptCount: 0, createdAt: summary.createdAt ?? now(), updatedAt: summary.updatedAt ?? now(),
      } : undefined,
      lastRun: terminal ? {
        id: `run-${id}`, jobId: summary.jobId, mode: detail.mode ?? 'reconcile', status: summary.status,
        phase: { code: detail.phase ?? summary.status, label: detail.phase ?? summary.status, state: summary.status === 'completed' ? 'complete' : 'terminal' },
        filesIndexed: Math.max(0, (detail.counts?.processed ?? detail.counts?.added ?? 0) - (detail.counts?.updated ?? 0)), filesUnchanged: detail.counts?.updated ?? 0, filesSkipped: 0,
        missingMarked: detail.confirmedMissingCount ?? detail.counts?.missing ?? 0, metadataQueued: detail.counts?.metadataPending ?? 0, analysisQueued: detail.counts?.analysisPending ?? 0,
        absenceAuthoritative: detail.absenceAuthoritative ?? false, cleanupAllowed: detail.cleanupAllowed ?? false,
        warnings: (detail.warnings ?? []).map((message) => ({ code: 'fixture_warning', severity: 'warning', message })),
        roots: (detail.roots ?? []).map((root, index) => ({ sourceId: root.id ?? `source-${index}`, configuredPath: root.path ?? '', status: root.status ?? 'healthy', errorClass: root.errorClass, errorMessage: root.error, directoriesSeen: 0, filesSeen: 0 })),
        startedAt: summary.createdAt ?? now(), completedAt: summary.updatedAt ?? now(), updatedAt: summary.updatedAt ?? now(),
      } : undefined,
      recentRuns: [], sources: [], actions: { canQuick: true, canTarget: true, canReconcile: true, canForceFull: true, canRemoveMissing: Boolean(detail?.cleanupAllowed), canCancel: summary?.status === 'running', canRetry: summary?.status === 'failed' || summary?.status === 'cancelled' },
      scheduleEnabled: true, lastRunAt: summary?.updatedAt, generatedAt: now(),
    });
  }
  libraryScanReview(id: string): Promise<LibraryScanReviewResponse> { return Promise.resolve({ libraryId: id, canConfirmRemoval: false, missingItems: [], missingTotal: 0, identityReviews: [], openIdentityTotal: 0, limit: 50, hasMore: false, generatedAt: now() }); }
  updateLibraryStorageClassification(libraryId: string, sourceId: string, classification: 'local' | 'network' | 'fuse' | 'unknown'): Promise<LibraryStorageSource> { return Promise.resolve({ id: sourceId, configuredPath: '/media', classification, classificationSource: 'owner', health: 'healthy', circuitState: 'closed', latencyMs: 1, consecutiveFailures: 0, updatedAt: now() }); }
  resolveIdentityReconciliationReview(reviewId: string, resolution: 'keep_separate' | 'merge_into_candidate', selectedCandidateId?: string): Promise<IdentityReconciliationReview> { return Promise.resolve({ id: reviewId, domain: 'media', libraryOrSourceId: 'fixture-movies', subjectId: 'fixture-subject', candidateLocator: 'Fixture candidate', evidenceKind: 'fixture', evidenceValue: 'fixture', candidateIds: selectedCandidateId ? [selectedCandidateId] : [], status: 'resolved', createdAt: now(), resolvedAt: now(), resolution, selectedCandidateId }); }
  scanLibrary(id: string, _signal?: AbortSignal, mode = 'reconcile'): Promise<Job> { const libraryItem = this.operations.libraries.find((item) => item.id === id); return Promise.resolve({ ...job(`scan-${id}`, `Scanning ${libraryItem?.name ?? 'library'}`, 'queued', 0, 0), metadata: { mode } }); }
  cancelLibraryScan(libraryId: string): Promise<Job> { return Promise.resolve(job(`scan-${libraryId}`, 'Library scan cancelled', 'cancelled', 0, 0)); }
  retryLibraryScan(libraryId: string): Promise<Job> { return Promise.resolve(job(`scan-${libraryId}`, 'Library scan retry queued', 'queued', 0, 0)); }
  createUser(input: UserCreateRequest): Promise<User> { const created = user(`fixture-user-${Date.now()}`, input.displayName || input.username || 'New member', 'user', input.libraryIds, 'local'); this.operations.users.push({ ...created, ...input } as User); return Promise.resolve(structuredClone(created)); }
  createPorticoMemberInvite(): Promise<{ inviteUrl?: string }> { return Promise.resolve({ inviteUrl: 'https://web.getportico.tv/invite?code=fixture' }); }
  resendPorticoMemberInvite(inviteId: string): Promise<PorticoInvite> {
    const existing = this.operations.porticoInvites?.find((invite) => invite.id === inviteId);
    const resent = { ...(existing ?? {}), id: inviteId, serverId: 'fixture-server', invitedEmail: existing?.invitedEmail ?? 'fixture@example.test', deliveryMode: 'email', role: 'user', status: 'pending', emailDeliveryStatus: 'queued', permissionTemplate: existing?.permissionTemplate ?? { libraryIds: [], permissions: {} }, resourceLimits: existing?.resourceLimits ?? {}, allowSubordinateProfiles: true, createdByUserId: 'fixture-owner', createdAt: existing?.createdAt ?? now(), expiresAt: existing?.expiresAt ?? now() } as PorticoInvite;
    if (existing) Object.assign(existing, resent);
    return Promise.resolve(structuredClone(resent));
  }
  updateUser(user: User, input: UserPatchRequest): Promise<User> { const index = this.operations.users.findIndex((item) => item.id === user.id); const updated = { ...this.operations.users[index], ...input } as User; this.operations.users[index] = updated; return Promise.resolve(structuredClone(updated)); }
  deleteUser(user: User): Promise<void> { this.operations.users = this.operations.users.filter((item) => item.id !== user.id); return Promise.resolve(); }
  updateDevice(id: string, input: DeviceUpdateRequest): Promise<Device> { const index = this.operations.devices.findIndex((item) => item.id === id); const updated = { ...this.operations.devices[index], ...input } as Device; this.operations.devices[index] = updated; return Promise.resolve(structuredClone(updated)); }
  revokeDevice(id: string): Promise<void> { this.operations.devices = this.operations.devices.filter((item) => item.id !== id); return Promise.resolve(); }
  createAPIKey(input: { name: string; scopes: string[] }): Promise<APIKeyCreateResponse> { const key = asFixture<APIKey>({ id: `fixture-key-${Date.now()}`, userId: 'fixture-owner', name: input.name, lastFour: 'N3XT', scopes: input.scopes, createdAt: now() }); this.operations.apiKeys.push(key); return Promise.resolve({ key, token: 'portico_fixture_token_shown_once' }); }
  revokeAPIKey(id: string): Promise<void> { this.operations.apiKeys = this.operations.apiKeys.filter((item) => item.id !== id); return Promise.resolve(); }
  updateScheduledTask(id: string, input: ScheduledTaskUpdateRequest): Promise<ScheduledTask> { const index = this.operations.tasks.findIndex((item) => item.id === id); const updated = { ...this.operations.tasks[index], ...input } as ScheduledTask; this.operations.tasks[index] = updated; return Promise.resolve(structuredClone(updated)); }
  runScheduledTask(id: string): Promise<ScheduledTaskRunResponse> { const queued = job(`fixture-${id}-${Date.now()}`, `Queued ${id.replaceAll('_', ' ')}`, 'queued', 0, 0); return Promise.resolve({ taskId: id, jobs: [queued] }); }
  createBackup(): Promise<BackupInfo> { const backup = asFixture<BackupInfo>({ name: `portico-${now().replaceAll(':', '-')}.db`, createdAt: now(), integrity: 'ok', manifestPresent: true, restoreReady: true, sizeBytes: 34865152 }); this.operations.backups.unshift(backup); return Promise.resolve(structuredClone(backup)); }
  restoreBackup(name: string, _password: string, _confirmation: string): Promise<RestoreWorkflowResponse> { return Promise.resolve(asFixture<RestoreWorkflowResponse>({ ok: true, name, operationId: `fixture-restore-${Date.now()}`, state: 'staged', phase: 'staged', progress: 25, instruction: 'Backup is staged for supervised restore; it is not restored yet.', statusToken: `fixture-status-${Date.now()}` })); }
  restoreUploadedDatabase(file: File, _password: string, _confirmation: string): Promise<RestoreWorkflowResponse> { return Promise.resolve(asFixture<RestoreWorkflowResponse>({ ok: true, name: file.name, operationId: `fixture-restore-${Date.now()}`, state: 'staged', phase: 'staged', progress: 25, sourceKind: 'raw-import', manifestVerified: false, instruction: 'Database import is staged for supervised restore; it is not restored yet.', statusToken: `fixture-status-${Date.now()}` })); }
  restoreStatus(operationId: string, _statusToken: string): Promise<RestoreWorkflowResponse> { return Promise.resolve(asFixture<RestoreWorkflowResponse>({ ok: true, name: 'fixture-backup', operationId, state: 'complete', phase: 'complete', progress: 100, instruction: 'Restore completed and health checks passed.' })); }
  logs(input: { limit?: number }): Promise<ListResponse<LogEvent>> { const items = [
    asFixture<LogEvent>({ id: 'log-1', time: ago(1), level: 'info', message: 'Library scan progress updated', fields: { library: 'TV Shows', progress: '62%' } }),
    asFixture<LogEvent>({ id: 'log-2', time: ago(2), level: 'info', message: 'Remote reachability check completed', fields: { result: 'reachable' } }),
    asFixture<LogEvent>({ id: 'log-3', time: ago(8), level: 'warn', message: 'Recording rule has no upcoming matches', fields: { rule: 'Fargo' } }),
  ].slice(0, input.limit ?? 200); return Promise.resolve(asFixture({ items, total: items.length, limit: input.limit ?? 200, offset: 0, hasMore: false })); }
  preferences(): Promise<UserPreferences> { return Promise.resolve(structuredClone(this.preferencesValue)); }
  updatePreferences(input: UserPreferences): Promise<User> { this.preferencesValue = structuredClone(input); const owner = this.operations.users[0]; this.operations.users[0] = { ...owner, preferences: structuredClone(input) }; return Promise.resolve(structuredClone(this.operations.users[0])); }
  signedInDevices(origin: AccountOrigin): Promise<AccountSignedInDevice[]> {
    if (origin === 'portico') {
      return Promise.resolve(structuredClone(this.porticoDevices));
    }
    return Promise.resolve(this.operations.sessions.map((session) => ({
      id: session.id,
      authority: 'local' as const,
      name: session.deviceName || session.platform || 'Signed-in device',
      app: session.app,
      platform: session.platform,
      current: session.current,
      canRevoke: session.canRevoke,
      trusted: session.trusted,
      createdAt: session.createdAt,
      lastSeenAt: session.lastSeenAt,
      expiresAt: session.expiresAt,
      clientIp: session.clientIp,
    })));
  }
  updateAccountIdentity(_origin: AccountOrigin, input: { displayName: string; email: string }): Promise<AccountIdentitySnapshot> {
    this.operations.users[0] = { ...this.operations.users[0], ...input };
    return Promise.resolve({ displayName: input.displayName, email: input.email, profileImageUrl: this.operations.users[0].profileImageUrl });
  }
  uploadAccountImage(_origin: AccountOrigin, _file: File): Promise<AccountIdentitySnapshot> {
    const profileImageUrl = `/brand/portico-app-icon-192.png?v=${Date.now()}`;
    this.operations.users[0] = { ...this.operations.users[0], profileImageUrl };
    return Promise.resolve({ displayName: this.operations.users[0].displayName, email: this.operations.users[0].email, profileImageUrl });
  }
  deleteAccountImage(_origin: AccountOrigin): Promise<AccountIdentitySnapshot> {
    this.operations.users[0] = { ...this.operations.users[0], profileImageUrl: undefined };
    return Promise.resolve({ displayName: this.operations.users[0].displayName, email: this.operations.users[0].email });
  }
  accountImageUrl(value: string): string { return value; }
  changeLocalPassword(): Promise<void> { return Promise.resolve(); }
  changePorticoPassword(): Promise<void> { return Promise.resolve(); }
  deletePorticoAccount(): Promise<void> { return Promise.resolve(); }
  porticoMFAStatus(): Promise<AccountMFAStatus> { return Promise.resolve(structuredClone(this.accountMFAValue)); }
  startPorticoMFA(): Promise<AccountMFASetup> {
    this.accountMFAValue = { ...this.accountMFAValue, enabled: false, setupStarted: true };
    return Promise.resolve({ enrollmentToken: 'fixture-enrollment-token', secret: 'PORTICOSETUPKEY42', otpauthUrl: 'otpauth://totp/Portico:review%40portico.local?secret=PORTICOSETUPKEY42&issuer=Portico' });
  }
  enablePorticoMFA(): Promise<AccountMFAEnableResult> {
    const recoveryCodes = ['PORTICO-7V9K-2N4Q', 'PORTICO-5M8R-3T6W', 'PORTICO-4X2P-9J7H'];
    this.accountMFAValue = { enabled: true, setupStarted: false, recoveryCodesSupported: true, recoveryCodesRemaining: recoveryCodes.length };
    return Promise.resolve({ enabled: true, recoveryCodes });
  }
  disablePorticoMFA(): Promise<void> {
    this.accountMFAValue = { enabled: false, setupStarted: false, recoveryCodesSupported: true, recoveryCodesRemaining: 0 };
    return Promise.resolve();
  }
  revokeSignedInDevice(origin: AccountOrigin, id: string): Promise<void> {
    if (origin === 'portico') this.porticoDevices = this.porticoDevices.filter((item) => item.id !== id);
    else this.operations.sessions = this.operations.sessions.filter((item) => item.id !== id);
    return Promise.resolve();
  }
  clearWatchHistory(): Promise<void> { return Promise.resolve(); }
  signOut(): Promise<void> { return Promise.resolve(); }
}
