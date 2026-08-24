import type { SettingsGroupsUpdate } from '@porticomediaserver/client-core';

export type WritableSettingsGroup = keyof SettingsGroupsUpdate;

export type ChoiceOption = { value: string; label: string };

export type SettingsFieldDefinition = {
  field: string;
  label: string;
  description: string;
  kind: 'toggle' | 'text' | 'textarea' | 'number' | 'choice' | 'string-list' | 'secret';
  options?: ChoiceOption[];
  placeholder?: string;
  min?: number;
  max?: number;
  step?: number;
  unit?: string;
  defaultValue?: string | boolean | number;
  warningByValue?: Record<string, string>;
  visibleWhen?: SettingsVisibilityCondition;
};

export type SettingsVisibilityCondition = { settingsKey: WritableSettingsGroup; field: string; equals: string | boolean };

export type SettingsFieldGroup = {
  id: string;
  capabilityId: string;
  title: string;
  description?: string;
  settingsKey: WritableSettingsGroup;
  fields: SettingsFieldDefinition[];
  visibleWhen?: SettingsVisibilityCondition;
};

const yesNo = (field: string, label: string, description: string): SettingsFieldDefinition => ({ field, label, description, kind: 'toggle' });
const text = (field: string, label: string, description: string, placeholder?: string): SettingsFieldDefinition => ({ field, label, description, kind: 'text', placeholder });
const number = (field: string, label: string, description: string, unit?: string, min?: number, max?: number, step?: number): SettingsFieldDefinition => ({ field, label, description, kind: 'number', unit, min, max, step });
const choice = (field: string, label: string, description: string, options: ChoiceOption[]): SettingsFieldDefinition => ({ field, label, description, kind: 'choice', options });
const list = (field: string, label: string, description: string, placeholder?: string): SettingsFieldDefinition => ({ field, label, description, kind: 'string-list', placeholder });
const option = (value: string, label = value): ChoiceOption => ({ value, label });

export const serverSettingFieldGroups: Record<string, SettingsFieldGroup[]> = {
  general: [
    {
      id: 'server-identity', capabilityId: 'server', title: 'Server identity', settingsKey: 'server', fields: [
        text('friendlyName', 'Server name', 'Shown in applications, invitations, and connection screens.'),
        { field: 'operatorNote', label: 'Owner note', description: 'Private context for the owner of this server.', kind: 'textarea', placeholder: 'Optional owner note' },
      ],
    },
  ],
  media: [
    {
      id: 'library-scanning', capabilityId: 'library-settings', title: 'Library scanning', settingsKey: 'library', fields: [
        yesNo('scanAutomatically', 'Automatic scans', 'Schedule library scans without requiring a manual request.'),
        yesNo('scanOnFilesystemChanges', 'Check folders for changes', 'Use bounded adaptive checks to detect changes without relying on fragile recursive filesystem watchers.'),
        {
          ...choice('analysisTier', 'Analysis tier', 'Inventory always completes first, and background analysis yields to playback. Basic adds technical facts and representative thumbnails. Complete enables deep whole-file compute such as sonic analysis, loudness, and intro/credit detection. Custom uses the controls below.', [option('file_list_only', 'File List Only'), option('basic', 'Basic (recommended)'), option('complete', 'Complete'), option('custom', 'Custom')]),
          defaultValue: 'basic',
          warningByValue: {
            complete: 'Complete can perform sustained/full-file reads and may require significantly more storage for generated files.',
            custom: 'Custom is advanced. Enabled High disk-I/O operations can perform sustained/full-file reads and may require significantly more network bandwidth and storage for generated files.',
          },
        },
        yesNo('emptyTrashAfterScan', 'Empty trash after scans', 'Remove missing entries after a completed scan instead of retaining them.'),
        yesNo('allowMediaDeletion', 'Allow file deletion', 'Permit the server owner to delete source files through Portico.'),
        number('trashRetentionDays', 'Trash retention', 'Number of days missing media stays recoverable.', 'days', 0, 365),
      ],
    },
    {
      id: 'library-analysis-low-io', capabilityId: 'library-settings', title: 'Low disk I/O', description: 'Directory and small-file reads. Provider requests use network access but do not download media objects.', settingsKey: 'library', visibleWhen: { settingsKey: 'library', field: 'analysisTier', equals: 'custom' }, fields: [
        yesNo('readLocalMetadata', 'Read local NFO/OPF metadata', 'Reads small owner-authored metadata files. Network I/O: none. Generated storage: low.'),
        yesNo('readExternalSubtitlesAndLyrics', 'Read subtitle and lyric sidecars', 'Reads small external subtitle and lyric files. Network I/O: none. Generated storage: low for normalized copies.'),
        yesNo('discoverLocalArtwork', 'Discover local artwork', 'Checks supported local artwork files without reading media content. Network I/O: none. Generated storage: low.'),
        yesNo('fetchDescriptiveMetadata', 'Contact metadata providers', 'Uses filename and inventory evidence after the catalog commits. Network I/O: low to moderate. Generated storage: low. Provider artwork downloads occur only when the selected tier permits them.'),
      ],
    },
    {
      id: 'library-analysis-moderate-io', capabilityId: 'library-settings', title: 'Moderate disk I/O', description: 'Bounded targeted media reads. Remote sources can make range requests; enabled operations never authorize whole-object staging.', settingsKey: 'library', visibleWhen: { settingsKey: 'library', field: 'analysisTier', equals: 'custom' }, fields: [
        yesNo('probeStreams', 'Probe container and streams', 'Required by the other Moderate and High operations. Network I/O: moderate for remote ranges. Generated storage: low.'),
        yesNo('readEmbeddedTags', 'Read embedded tags', 'Reads bounded embedded descriptive tags when supported. Requires stream probing. Network I/O: moderate for remote ranges. Generated storage: none.'),
        yesNo('readEmbeddedIndexes', 'Read embedded indexes', 'Reads chapter, cover, and attachment indexes. Requires stream probing. Network I/O: moderate for remote ranges. Generated storage: low.'),
        yesNo('generateRepresentativeThumbnail', 'Generate one representative thumbnail', 'Creates one representative image. Requires stream probing. Network I/O: moderate for remote ranges. Generated storage: low.'),
      ],
    },
    {
      id: 'library-analysis-high-io', capabilityId: 'library-settings', title: 'High disk I/O', description: 'Sustained or full-file work. Remote sources may stage whole objects. Network I/O and generated storage can be high; every option requires stream probing.', settingsKey: 'library', visibleWhen: { settingsKey: 'library', field: 'analysisTier', equals: 'custom' }, fields: [
        yesNo('generateChapterThumbnails', 'Generate chapter thumbnails', 'Creates chapter stills from sustained reads. Network I/O: high for remote media. Generated storage: moderate to high.'),
        yesNo('generateTrickplay', 'Generate trickplay previews', 'Creates timeline preview tiles from sustained reads. Network I/O: high for remote media. Generated storage: high.'),
        yesNo('analyzeLoudness', 'Analyze loudness', 'Performs a sustained audio pass for normalization facts. Network I/O: high for remote media. Generated storage: low.'),
        yesNo('sonicFingerprinting', 'Sonic fingerprinting', 'Performs a full audio-content pass for similarity and matching. Network I/O: high for remote media. Generated storage: moderate.'),
        yesNo('extractAllEmbeddedAttachments', 'Extract all embedded attachments', 'Extracts supported embedded fonts and attachments. Requires stream probing and embedded indexes. Network I/O: high for remote media. Generated storage: moderate to high.'),
      ],
    },
    {
      id: 'library-generated-navigation', capabilityId: 'library-settings', title: 'Generated navigation scheduling', description: 'Configure schedules and limits for generated preview navigation. Custom analysis permissions above still control whether source-reading work may run.', settingsKey: 'library', fields: [
        text('generateVideoPreview', 'Video preview policy', 'Controls when video preview clips are generated.', 'scheduled'),
        text('chapterThumbnailMode', 'Chapter thumbnails', 'Controls chapter thumbnail generation for video media.', 'chapters'),
        yesNo('trickplayOnScan', 'Generate trickplay on scan', 'Create timeline preview tiles as new video is added.'),
        number('trickplayIntervalSeconds', 'Trickplay interval', 'Seconds between generated timeline preview frames.', 'seconds', 1, 120),
        number('trickplayTileWidth', 'Trickplay tile width', 'Pixel width of each generated timeline frame.', 'px', 80, 640),
        number('trickplayMaxTiles', 'Maximum trickplay tiles', 'Maximum timeline tiles generated for one media item.', 'tiles', 1, 10000),
      ],
    },
    {
      id: 'metadata-providers', capabilityId: 'metadata-agents', title: 'Metadata providers', settingsKey: 'metadataAgents', fields: [
        choice('movies', 'Movies', 'Primary metadata provider for movie libraries.', [option('TMDB'), option('TVDB', 'TheTVDB'), option('None')]),
        choice('moviesFallback', 'Movie fallback', 'Used only when the primary provider returns no confident movie match.', [option('TVDB', 'TheTVDB'), option('TMDB'), option('None')]),
        choice('tv', 'TV', 'Primary metadata provider for television libraries.', [option('TMDB'), option('TVDB', 'TheTVDB'), option('None')]),
        choice('tvFallback', 'TV fallback', 'Used only when the primary provider returns no confident television match.', [option('TVDB', 'TheTVDB'), option('TMDB'), option('None')]),
        choice('anime', 'Anime', 'Primary metadata provider for anime libraries.', [option('AniList'), option('TMDB'), option('None')]),
        choice('music', 'Music', 'Primary metadata provider for music libraries.', [option('MusicBrainz'), option('None')]),
        yesNo('localNFO', 'Read local NFO files', 'Use adjacent NFO files when present.'),
        yesNo('embeddedTags', 'Read embedded tags', 'Use metadata embedded in supported media containers.'),
        yesNo('cacheOriginalArtwork', 'Keep original artwork', 'Retain provider artwork before image processing.'),
        text('metadataLanguage', 'Metadata language', 'Preferred provider language and region code.', 'en-US'),
        number('refreshDays', 'Refresh interval', 'Refresh provider metadata after this many days.', 'days', 1, 365),
      ],
    },
    {
      id: 'metadata-credentials', capabilityId: 'metadata-agents', title: 'Provider credentials', description: 'Existing credentials are never returned by the server.', settingsKey: 'metadataAgents', fields: [
        { field: 'tmdbReadAccessToken', label: 'TMDB read access token override', description: 'Optional advanced override. Portico includes TMDB metadata access by default.', kind: 'secret' },
        { field: 'tmdbAPIKey', label: 'TMDB API key override', description: 'Optional advanced API-key override. Portico includes TMDB metadata access by default.', kind: 'secret' },
        { field: 'tvdbAPIKey', label: 'TheTVDB API key override', description: 'Optional project-key override. Portico includes TheTVDB metadata access by default.', kind: 'secret' },
      ],
    },
  ],
  playback: [
    {
      id: 'playback-languages', capabilityId: 'languages', title: 'Languages', settingsKey: 'languages', fields: [
        text('audio', 'Preferred audio language', 'ISO language code selected when a matching audio track exists.', 'en'),
        text('subtitle', 'Preferred subtitle language', 'ISO language code selected when subtitles are needed.', 'en'),
        choice('subtitleMode', 'Subtitle mode', 'Default subtitle selection policy.', [option('manual', 'Manual'), option('always', 'Always'), option('foreignAudio', 'Foreign audio only')]),
        yesNo('preferForcedSubs', 'Prefer forced subtitles', 'Select forced subtitle tracks before full subtitles.'),
      ],
    },
    {
      id: 'transcoder-core', capabilityId: 'transcoder', title: 'Transcoder', settingsKey: 'transcoder', fields: [
        yesNo('enabled', 'Enable transcoding', 'Allow Portico to convert media for incompatible or bandwidth-limited clients.'),
        text('temporaryDirectory', 'Temporary directory', 'Absolute path used for active transcode segments.'),
        number('maxConcurrentSessions', 'Concurrent sessions', 'Maximum active transcoding sessions.', 'sessions', 1, 64),
        choice('x264Preset', 'H.264 preset', 'Encoding speed and compression trade-off.', ['ultrafast', 'superfast', 'veryfast', 'faster', 'fast', 'medium', 'slow', 'slower'].map((value) => option(value))),
        number('throttleBufferSeconds', 'Throttle buffer', 'Media seconds generated ahead before throttling.', 'seconds', 10, 3600),
        number('playedRetentionSeconds', 'Played segment retention', 'Seconds of already-played segments retained on disk.', 'seconds', 0, 86400),
        yesNo('directStreamRemux', 'Allow direct stream remux', 'Remux compatible streams instead of re-encoding them.'),
      ],
    },
    {
      id: 'transcoder-hardware', capabilityId: 'transcoder', title: 'Hardware acceleration', settingsKey: 'transcoder', fields: [
        yesNo('hardwareAcceleration', 'Hardware decoding', 'Use an available hardware device for supported decode work.'),
        yesNo('hardwareEncoding', 'Hardware encoding', 'Use an available hardware encoder for supported output.'),
        yesNo('hardwareDecodeHEVC', 'Hardware HEVC decoding', 'Use the configured device to decode HEVC sources.'),
        text('hardwareDevice', 'Hardware device', 'Platform device identifier passed to the transcoder.', 'auto'),
        number('maxHardwareSessions', 'Hardware session limit', 'Maximum simultaneous hardware-accelerated sessions.', 'sessions', 0, 64),
        number('maxSoftwareSessions', 'Software session limit', 'Maximum simultaneous software-only sessions.', 'sessions', 0, 64),
        number('maxBackgroundSessions', 'Background session limit', 'Maximum concurrent background conversions.', 'sessions', 0, 32),
        yesNo('hdrToneMapping', 'HDR tone mapping', 'Convert HDR video for clients that require SDR output.'),
        choice('hdrToneMappingAlgorithm', 'Tone mapping algorithm', 'FFmpeg tone mapping algorithm used for HDR conversion.', ['hable', 'mobius', 'reinhard', 'gamma', 'linear', 'clip'].map((value) => option(value))),
      ],
    },
    {
      id: 'optimized-versions', capabilityId: 'optimized-versions', title: 'Optimized versions', settingsKey: 'optimizedVersions', fields: [
        choice('defaultProfile', 'Default template', 'Template used when an optimization request does not specify one.', [
          option('universal-1080p', 'Universal 1080p · H.264'),
          option('universal-720p', 'Universal 720p · H.264'),
          option('universal-480p', 'Universal 480p · H.264'),
          option('efficient-4k', 'Efficient 4K · HEVC'),
          option('efficient-1080p', 'Efficient 1080p · HEVC'),
          option('efficient-720p', 'Efficient 720p · HEVC'),
          option('maximum-compression-source', 'Maximum compression · Source size · AV1'),
          option('maximum-compression-1080p', 'Maximum compression · 1080p · AV1'),
        ]),
        yesNo('preferOptimizedPlayback', 'Prefer optimized versions', 'Use a compatible optimized copy before starting a transcode.'),
        text('storageDirectory', 'Storage directory', 'Absolute path for generated optimized versions.'),
        number('maxConcurrentJobs', 'Concurrent jobs', 'Maximum active optimization jobs.', 'jobs', 1, 4),
        number('maxPerItem', 'Versions per item', 'Maximum optimized versions retained for one item.', 'versions', 1, 16),
        yesNo('autoDelete', 'Automatic cleanup', 'Remove optimized versions using the retention limits below.'),
        number('retentionDays', 'Retention', 'Delete unused optimized versions after this period.', 'days', 1, 3650),
        number('maxStorageMB', 'Storage limit', 'Maximum storage assigned to optimized versions.', 'MB', 0),
      ],
    },
  ],
  live: [
    {
      id: 'dvr-defaults', capabilityId: 'dvr', title: 'Recording defaults', settingsKey: 'dvr', fields: [
        number('defaultStartPaddingMinutes', 'Start padding', 'Minutes added before a scheduled recording.', 'minutes', 0, 120),
        number('defaultEndPaddingMinutes', 'End padding', 'Minutes added after a scheduled recording.', 'minutes', 0, 120),
        number('defaultRetentionDays', 'Retention', 'Days recordings are retained by default.', 'days', 1, 3650),
        text('defaultFolder', 'Default folder', 'Folder label assigned to new recording rules.'),
        number('defaultMaxRecordingsPerSeries', 'Episodes per series', 'Maximum retained recordings per series; zero means unlimited.', 'recordings', 0),
        choice('recordingProfile', 'Recording profile', 'Output profile used for new recordings.', [option('copy', 'Original stream'), option('h264-1080p-8m', 'H.264 1080p · 8 Mbps'), option('h264-720p-4m', 'H.264 720p · 4 Mbps')]),
        text('recordingPathTemplate', 'Path template', 'Folder and filename template for new recordings.', '{folder}/{title}-{start}'),
        yesNo('preserveAllStreams', 'Preserve all streams', 'Keep every source audio and subtitle stream when possible.'),
        yesNo('convertRecordings', 'Convert recordings', 'Apply the selected recording profile after capture.'),
        yesNo('saveNFO', 'Write NFO sidecars', 'Write metadata beside completed recordings.'),
        yesNo('saveImageSidecars', 'Write image sidecars', 'Save downloaded artwork beside completed recordings.'),
      ],
    },
    {
      id: 'guide-defaults', capabilityId: 'dvr', title: 'Guide and rule defaults', settingsKey: 'dvr', fields: [
        number('defaultGuideRefreshIntervalHours', 'Guide refresh interval', 'Hours between automatic guide refreshes.', 'hours', 1, 168),
        yesNo('defaultGuideRequireEpg', 'Require guide listings', 'Exclude channels without matched guide data from new rules.'),
        yesNo('guideChannelAutoMatch', 'Automatic channel matching', 'Assign imported guide programs to matching playable channels.'),
        list('defaultRuleRequiredKeywords', 'Required keywords', 'Default keywords that must appear in matching programs.'),
        list('defaultRuleBlockedKeywords', 'Blocked keywords', 'Default keywords that exclude matching programs.'),
        list('defaultRuleAllowedChannels', 'Allowed channels', 'Channel IDs allowed by default for new recording rules.'),
        list('defaultRuleBlockedChannels', 'Blocked channels', 'Channel IDs excluded by default from new recording rules.'),
      ],
    },
  ],
  connectivity: [
    {
      id: 'network-policy', capabilityId: 'network', title: 'Network policy', settingsKey: 'network', fields: [
        choice('secureConnections', 'Secure connections', 'TLS policy for Portico client connections.', [option('preferred', 'Preferred'), option('required', 'Required'), option('disabled', 'Disabled')]),
        text('lanNetworks', 'LAN networks', 'Comma-separated CIDR networks treated as local.', '192.168.0.0/16, 10.0.0.0/8'),
        text('customAccessUrls', 'Custom access URLs', 'Comma-separated addresses advertised to trusted clients.'),
      ],
    },
    {
      id: 'dlna', capabilityId: 'dlna', title: 'DLNA', settingsKey: 'dlna', fields: [
        yesNo('enabled', 'Enable DLNA', 'Advertise this server to compatible devices on the local network.'),
        text('friendlyName', 'DLNA server name', 'Name shown in compatible media browser applications.'),
        text('advertiseUrl', 'Advertise URL', 'Optional URL advertised to DLNA clients.'),
        list('exposedLibraries', 'Exposed libraries', 'Library IDs available over unauthenticated local DLNA.'),
        yesNo('reportTimeline', 'Report playback timeline', 'Accept timeline reports from supported DLNA clients.'),
      ],
    },
  ],
  people: [
    {
      id: 'device-policy', capabilityId: 'devices', title: 'Device approval', settingsKey: 'devices', fields: [
        yesNo('requireTrustedDevices', 'Require trusted devices', 'Block sign-in from devices the server owner has not approved.'),
        choice('quickConnectApprovalMode', 'Quick Connect approval', 'Who may approve a television or shared-device sign-in.', [option('allUsers', 'All users'), option('ownerOnly', 'Server owner only')]),
      ],
    },
  ],
  maintenance: [
    {
      id: 'maintenance-window', capabilityId: 'scheduled-tasks', title: 'Maintenance window', settingsKey: 'scheduledTasks', fields: [
        yesNo('enabled', 'Enable scheduled maintenance', 'Allow configured background tasks to run automatically.'),
        choice('maintenanceWindow', 'Window', 'When maintenance work may begin.', [option('overnight', 'Overnight'), option('late-night', 'Late night'), option('always', 'Any time'), option('custom', 'Custom')]),
        choice('maintenanceDays', 'Days', 'Days when scheduled maintenance may run.', [option('every-day', 'Every day'), option('weekdays', 'Weekdays'), option('weekends', 'Weekends')]),
        number('startHour', 'Start hour', 'Custom maintenance start hour in server time.', 'hour', 0, 23),
        number('endHour', 'End hour', 'Custom maintenance end hour in server time.', 'hour', 0, 23),
      ],
    },
    {
      id: 'scheduled-library-work', capabilityId: 'scheduled-tasks', title: 'Library work', settingsKey: 'scheduledTasks', fields: [
        yesNo('scanLibraries', 'Scan libraries', 'Run automatic library scans.'),
        choice('libraryScanCadence', 'Library scan cadence', 'How frequently automatic scans are eligible to run.', ['hourly', 'daily', 'weekly', 'monthly', 'custom'].map((value) => option(value))),
        number('libraryScanIntervalHours', 'Custom scan interval', 'Hours between automatic scans when cadence is custom.', 'hours', 1),
        yesNo('refreshMetadata', 'Refresh metadata', 'Refresh stale provider metadata automatically.'),
        choice('metadataRefreshCadence', 'Metadata cadence', 'How frequently metadata refreshes are eligible to run.', ['hourly', 'daily', 'weekly', 'monthly', 'custom'].map((value) => option(value))),
        number('metadataRefreshDays', 'Metadata age', 'Refresh metadata older than this many days.', 'days', 1),
        yesNo('analyzeMedia', 'Analyze media', 'Probe unanalyzed media during maintenance.'),
        choice('analysisCadence', 'Analysis cadence', 'How frequently analysis work is eligible to run.', ['hourly', 'daily', 'weekly', 'monthly', 'custom'].map((value) => option(value))),
      ],
    },
    {
      id: 'scheduled-storage-work', capabilityId: 'scheduled-tasks', title: 'Storage care', settingsKey: 'scheduledTasks', fields: [
        yesNo('backupDatabase', 'Back up database', 'Create automatic configuration and library database backups.'),
        choice('backupCadence', 'Backup cadence', 'How frequently backups are eligible to run.', ['hourly', 'daily', 'weekly', 'monthly', 'custom'].map((value) => option(value))),
        number('backupRetentionDays', 'Backup retention', 'Days automatic backups are retained.', 'days', 1),
        yesNo('emptyTrash', 'Empty library trash', 'Remove expired missing-media entries during maintenance.'),
        number('trashRetentionDays', 'Trash retention', 'Days missing media remains recoverable.', 'days', 0),
        number('trickplayRetentionDays', 'Trickplay retention', 'Days unused trickplay images are retained.', 'days', 0),
        number('trickplayMaxStorageMB', 'Trickplay storage limit', 'Maximum storage assigned to trickplay images.', 'MB', 0),
        number('trickplayIntervalSeconds', 'Trickplay interval', 'Seconds between generated trickplay frames.', 'seconds', 1),
        number('trickplayTileWidth', 'Trickplay tile width', 'Pixel width of generated trickplay frames.', 'px', 80),
        number('trickplayMaxTiles', 'Maximum trickplay tiles', 'Maximum tiles retained per media item.', 'tiles', 1),
      ],
    },
    {
      id: 'notifications', capabilityId: 'notifications', title: 'Operational alerts', settingsKey: 'notifications', fields: [
        yesNo('enabled', 'Enable operational alerts', 'Surface server alerts to the server owner.'),
        choice('minAlertLevel', 'Minimum alert level', 'Lowest severity included in notifications.', [option('info', 'Information'), option('warn', 'Warning'), option('error', 'Error')]),
      ],
    },
  ],
  diagnostics: [
    {
      id: 'history-retention', capabilityId: 'retention', title: 'History & retention', description: 'This data stays on your Portico server. Set a value to 0 to retain that category indefinitely.', settingsKey: 'retention', fields: [
        number('playbackDetailDays', 'Detailed playback sessions', 'Retain per-session playback, device, route, and performance detail. 0 keeps it indefinitely.', 'days', 0, 36500),
        number('playbackHistoryDays', 'Playback history', 'Retain the long-term playback history and statistics derived from completed sessions. 0 keeps it indefinitely.', 'days', 0, 36500),
        number('auditHistoryDays', 'Security audit history', 'Retain tamper-evident administrative and security evidence. Default is 90 days; 0 keeps it indefinitely.', 'days', 0, 36500),
        number('diagnosticHistoryDays', 'Server diagnostics', 'Retain persisted server diagnostics and console history. Default is 30 days; 0 keeps it indefinitely.', 'days', 0, 36500),
        number('clientDiagnosticHistoryDays', 'Client diagnostics', 'Retain accepted client-upload diagnostics in their isolated lane. Default is 30 days; 0 keeps them indefinitely.', 'days', 0, 36500),
        number('jobHistoryDays', 'Job history', 'Retain completed, failed, cancelled, and interrupted jobs. Default is 30 days; 0 keeps them indefinitely.', 'days', 0, 36500),
        number('authRequestDays', 'Sign-in request history', 'Retain completed or expired Quick Connect, TV setup, and Portico sign-in request records. Default is 14 days; 0 keeps them indefinitely.', 'days', 0, 36500),
        number('deviceIPDays', 'Device and playback addresses', 'Retain raw IP addresses recorded for inactive devices and completed playback sessions. Default is 30 days; device identity and use history are retained separately.', 'days', 0, 36500),
      ],
    },
    {
      id: 'diagnostic-policy', capabilityId: 'troubleshooting', title: 'Diagnostic policy', settingsKey: 'troubleshooting', fields: [
        choice('logLevel', 'Log level', 'Controls how much detail is written to server logs.', [option('info', 'Normal'), option('warn', 'Warnings only'), option('debug', 'Debug')]),
        number('debugDurationMinutes', 'Debug expiry', 'Debug logging automatically returns to Normal after this interval.', 'minutes', 5, 1440),
        yesNo('clientLogUploads', 'Accept client diagnostics', 'Allow signed-in Portico applications to upload redacted diagnostic events.'),
      ],
    },
  ],
};
