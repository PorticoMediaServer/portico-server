import type {
  AccountSession,
  APIKey,
  APIKeyCreateResponse,
  BackupInfo,
  DashboardResponse,
  Device,
  DeviceUpdateRequest,
  DLNAStatus,
  DVROperationalStatus,
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
  Permissions,
  PorticoInvite,
  RemoteAccessSettingsPatch,
  RemoteAccessStatus,
  RestoreBackupResponse,
  ScheduledTask,
  ScheduledTaskRunResponse,
  ScheduledTaskUpdateRequest,
  ServerCapabilitiesResponse,
  ServerActivityResponse,
  SettingsDocument,
  SettingsSummaryResponse,
  SettingsUpdateRequest,
  SystemDiagnostics,
  SystemReleaseInfo,
  SystemStorageCleanupResponse,
  SystemStorageReport,
  TranscodeCapacityReport,
  User,
  UserPreferences,
  UserCreateRequest,
  UserPatchRequest,
} from '@portico/client-core';

export type SettingsViewer = {
  id: string;
  displayName: string;
  email: string;
  role: 'owner' | 'user';
  serverName: string;
  profileImageUrl?: string;
  authOrigin?: 'local' | 'portico';
  authProvider?: 'local' | 'portico' | 'api_key';
  hasLocalPassword?: boolean;
  permissions?: Record<string, boolean>;
};

export type LibraryMutationInput = {
  name: string;
  type: 'movie' | 'show' | 'anime' | 'music' | 'audiobook' | 'recorded-tv';
  path?: string;
  paths: string[];
  settings?: Record<string, unknown>;
};

export type LibraryScanMode = 'targeted' | 'quick' | 'reconcile' | 'force_full' | 'remove_missing';

export type LibraryScanPhase = { code: string; label: string; state: string };
export type LibraryScanWarning = { code: string; severity: string; message: string; sourceId?: string };
export type LibraryScanRootResult = {
  sourceId: string;
  configuredPath: string;
  resolvedPath?: string;
  status: string;
  errorClass?: string;
  errorMessage?: string;
  directoriesSeen: number;
  filesSeen: number;
  lastProgressAt?: string;
};
export type LibraryStorageSource = {
  id: string;
  configuredPath: string;
  resolvedPath?: string;
  classification: string;
  classificationSource: string;
  health: string;
  circuitState: string;
  errorClass?: string;
  errorMessage?: string;
  latencyMs: number;
  consecutiveFailures: number;
  lastProgressAt?: string;
  lastSuccessAt?: string;
  lastFailureAt?: string;
  updatedAt: string;
};
export type LibraryScanRun = {
  id: string;
  jobId?: string;
  mode: LibraryScanMode;
  status: string;
  phase: LibraryScanPhase;
  filesIndexed: number;
  filesUnchanged: number;
  filesSkipped: number;
  missingMarked: number;
  metadataQueued: number;
  analysisQueued: number;
  absenceAuthoritative: boolean;
  cleanupAllowed: boolean;
  warnings: LibraryScanWarning[];
  roots: LibraryScanRootResult[];
  startedAt: string;
  completedAt?: string;
  updatedAt: string;
};
export type LibraryScanOperation = {
  jobId: string;
  status: string;
  mode: LibraryScanMode;
  trigger: string;
  progress: number;
  phase: LibraryScanPhase;
  message: string;
  attemptCount: number;
  nextAttemptAt?: string;
  cancellationRequestedAt?: string;
  createdAt: string;
  updatedAt: string;
};
export type LibraryScanActions = {
  canQuick: boolean;
  canTarget: boolean;
  canReconcile: boolean;
  canForceFull: boolean;
  canRemoveMissing: boolean;
  canCancel: boolean;
  canRetry: boolean;
};
export type LibraryScanOperationsResponse = {
  libraryId: string;
  operation?: LibraryScanOperation;
  lastRun?: LibraryScanRun;
  recentRuns: LibraryScanRun[];
  sources: LibraryStorageSource[];
  actions: LibraryScanActions;
  scheduleEnabled: boolean;
  lastRunAt?: string;
  nextRunAt?: string;
  generatedAt: string;
};
export type LibraryMissingMediaReview = { mediaId: string; fileId: string; title: string; path: string; missingSince: string; sourceId?: string; sourceHealth?: string };
export type IdentityReconciliationReview = {
  id: string; domain: string; libraryOrSourceId: string; subjectId: string; candidateLocator: string; evidenceKind: string; evidenceValue: string;
  candidateIds: string[]; status: string; createdAt: string; resolvedAt?: string; resolution?: string; selectedCandidateId?: string; resolvedByUserId?: string; resolutionNote?: string;
};
export type LibraryScanReviewResponse = {
  libraryId: string; confirmationRunId?: string; canConfirmRemoval: boolean; missingItems: LibraryMissingMediaReview[]; missingTotal: number;
  identityReviews: IdentityReconciliationReview[]; openIdentityTotal: number; limit: number; hasMore: boolean; nextCursor?: string; generatedAt: string;
};

export type SettingsStatusSnapshot = {
  activity?: ServerActivityResponse;
  dashboard?: DashboardResponse;
  storage?: SystemStorageReport;
  remoteAccess?: RemoteAccessStatus;
  jobs?: Job[];
  failures?: Partial<Record<'activity' | 'dashboard' | 'storage' | 'remoteAccess' | 'jobs', string>>;
  generatedAt: string;
};

export type SettingsOperationalSnapshot = {
  libraries: Library[];
  users: User[];
  devices: Device[];
  apiKeys: APIKey[];
  tasks: ScheduledTask[];
  backups: BackupInfo[];
  sessions: AccountSession[];
  release: SystemReleaseInfo;
  diagnostics: SystemDiagnostics;
  capabilities: ServerCapabilitiesResponse;
  storage: SystemStorageReport;
  porticoInvites?: PorticoInvite[];
};

export type SettingsOperationalScope = 'media' | 'people' | 'maintenance' | 'diagnostics' | 'help';

export type RestoreWorkflowResponse = Omit<RestoreBackupResponse, 'state'> & {
  state: 'validating' | 'staged' | 'quiescing' | 'safety-copy' | 'installing' | 'reopening/migrating' | 'health-checking' | 'complete' | 'rolling-back' | 'failed' | 'recovery-required';
  phase?: string;
  progress?: number;
  sourceKind?: 'catalog-backup' | 'raw-import';
  manifestVerified?: boolean;
  maxDatabaseBytes?: number;
  recoveryRequired?: boolean;
  validationCode?: string;
  errorCode?: string;
  errorMessage?: string;
  warningCode?: string;
  warningMessage?: string;
  statusToken?: string;
};

export type DVRStatusSnapshot = DVROperationalStatus;

export type AccountOrigin = 'local' | 'portico';

export type AccountSignedInDevice = {
  id: string;
  authority: AccountOrigin;
  name: string;
  app?: string;
  platform?: string;
  current: boolean;
  canRevoke: boolean;
  trusted?: boolean;
  createdAt?: string;
  lastSeenAt: string;
  expiresAt?: string;
  clientIp?: string;
};

export type PorticoMemberInviteInput = {
  recipient: string;
  email?: string;
  role: 'user';
  permissionTemplate: {
    libraryIds: string[];
    permissions: Permissions;
    maxContentRating?: string;
  };
  deliveryMode: 'email' | 'link';
};

export type PorticoMemberInviteResult = { inviteUrl?: string };

export type AccountIdentitySnapshot = {
  displayName: string;
  email: string;
  profileImageUrl?: string;
  serverSyncWarning?: string;
};

export type AccountMFAStatus = {
  enabled: boolean;
  setupStarted: boolean;
  recoveryCodesSupported: boolean;
  recoveryCodesRemaining?: number;
};

export type AccountMFASetup = {
  enrollmentToken: string;
  secret: string;
  otpauthUrl: string;
};

export type AccountMFAEnableResult = {
  enabled: boolean;
  recoveryCodes: string[];
};

export interface SettingsDataSource {
  settings(signal: AbortSignal): Promise<SettingsDocument>;
  settingsSummary(signal: AbortSignal): Promise<SettingsSummaryResponse>;
  updateSettings(input: SettingsUpdateRequest, signal: AbortSignal): Promise<SettingsDocument>;

  settingsStatus(signal: AbortSignal): Promise<SettingsStatusSnapshot>;
  runConnectivityCheck(signal: AbortSignal): Promise<RemoteAccessStatus>;
  stopPlayback(sessionId: string, signal: AbortSignal): Promise<void>;

  remoteAccess(signal: AbortSignal): Promise<RemoteAccessStatus>;
  updateRemoteAccess(input: RemoteAccessSettingsPatch, signal: AbortSignal): Promise<RemoteAccessStatus>;
  startRemoteAccessClaim(signal: AbortSignal): Promise<RemoteAccessStatus>;
  cancelRemoteAccessClaim(signal: AbortSignal): Promise<RemoteAccessStatus>;
  unclaimRemoteAccess(signal: AbortSignal): Promise<RemoteAccessStatus>;
  renewRemoteAccessCertificate(signal: AbortSignal): Promise<RemoteAccessStatus>;

  settingsOperations(scope: SettingsOperationalScope, signal: AbortSignal): Promise<SettingsOperationalSnapshot>;
  liveTVSources(signal: AbortSignal): Promise<LiveTVSource[]>;
  createLiveTVSource(input: LiveTVSourceRequest, testBeforeSaving: boolean, signal: AbortSignal): Promise<LiveTVSource>;
  updateLiveTVSource(id: string, input: LiveTVSourceRequest, signal: AbortSignal): Promise<LiveTVSource>;
  refreshLiveTVSource(id: string, signal: AbortSignal): Promise<LiveTVSource>;
  deleteLiveTVSource(id: string, signal: AbortSignal): Promise<void>;
  dvrStatus(sourceId: string | undefined, signal: AbortSignal): Promise<DVRStatusSnapshot>;
  libraryChannels(signal: AbortSignal): Promise<AdminLibraryChannelListResponse>;
  libraryChannel(id: string, signal: AbortSignal): Promise<LibraryChannelAggregate>;
  createLibraryChannel(input: LibraryChannelConfigurationRequest, signal: AbortSignal): Promise<LibraryChannelAggregate>;
  updateLibraryChannel(id: string, input: LibraryChannelConfigurationRequest, signal: AbortSignal): Promise<LibraryChannelAggregate>;
  deleteLibraryChannel(id: string, expectedRevision: number, signal: AbortSignal): Promise<void>;
  libraryChannelTemplates(signal: AbortSignal): Promise<LibraryChannelTemplatesResponse>;
  restoreLibraryChannelDefaults(input: LibraryChannelRestoreDefaultsRequest, signal: AbortSignal): Promise<LibraryChannelRestoreDefaultsResponse>;
  regenerateLibraryChannel(id: string, signal: AbortSignal): Promise<LibraryChannelGeneration>;
  uploadLibraryChannelLogo(file: File, signal: AbortSignal): Promise<{ id: string; source: 'custom'; mimeType: 'image/png'; url: string; width: number; height: number; bytes: number; sha256: string }>;

  transcodeCapacity(signal: AbortSignal): Promise<TranscodeCapacityReport>;
  systemStorage(signal: AbortSignal): Promise<SystemStorageReport>;
  cleanupStorage(signal: AbortSignal): Promise<SystemStorageCleanupResponse>;
  dlnaStatus(signal: AbortSignal): Promise<DLNAStatus>;

  createLibrary(input: LibraryMutationInput, signal: AbortSignal): Promise<Library>;
  updateLibrary(id: string, input: LibraryMutationInput, signal: AbortSignal): Promise<Library>;
  deleteLibrary(id: string, signal: AbortSignal): Promise<void>;
  libraryScanOperations(id: string, signal: AbortSignal): Promise<LibraryScanOperationsResponse>;
  libraryScanReview(id: string, cursor: string | undefined, signal: AbortSignal): Promise<LibraryScanReviewResponse>;
  updateLibraryStorageClassification(libraryId: string, sourceId: string, classification: 'local' | 'network' | 'fuse' | 'unknown', signal: AbortSignal): Promise<LibraryStorageSource>;
  resolveIdentityReconciliationReview(reviewId: string, resolution: 'keep_separate' | 'merge_into_candidate', selectedCandidateId: string | undefined, signal: AbortSignal): Promise<IdentityReconciliationReview>;
  scanLibrary(id: string, signal: AbortSignal, mode?: LibraryScanMode, confirmedRunId?: string): Promise<Job>;
  cancelLibraryScan(libraryId: string, signal: AbortSignal): Promise<Job>;
  retryLibraryScan(libraryId: string, runId: string | undefined, signal: AbortSignal): Promise<Job>;

  createUser(input: UserCreateRequest, signal: AbortSignal): Promise<User>;
  createPorticoMemberInvite(input: PorticoMemberInviteInput, signal: AbortSignal): Promise<PorticoMemberInviteResult>;
  resendPorticoMemberInvite(inviteId: string, signal: AbortSignal): Promise<PorticoInvite>;
  updateUser(user: User, input: UserPatchRequest, signal: AbortSignal): Promise<User>;
  deleteUser(user: User, signal: AbortSignal): Promise<void>;
  updateDevice(id: string, input: DeviceUpdateRequest, signal: AbortSignal): Promise<Device>;
  revokeDevice(id: string, signal: AbortSignal): Promise<void>;
  createAPIKey(input: { name: string; scopes: string[] }, signal: AbortSignal): Promise<APIKeyCreateResponse>;
  revokeAPIKey(id: string, signal: AbortSignal): Promise<void>;

  updateScheduledTask(id: string, input: ScheduledTaskUpdateRequest, signal: AbortSignal): Promise<ScheduledTask>;
  runScheduledTask(id: string, signal: AbortSignal): Promise<ScheduledTaskRunResponse>;
  createBackup(signal: AbortSignal): Promise<BackupInfo>;
  restoreBackup(name: string, password: string, confirmation: string, signal: AbortSignal): Promise<RestoreWorkflowResponse>;
  restoreUploadedDatabase(file: File, password: string, confirmation: string, signal: AbortSignal): Promise<RestoreWorkflowResponse>;
  restoreStatus(operationId: string, statusToken: string, signal: AbortSignal): Promise<RestoreWorkflowResponse>;

  logs(input: { limit?: number; cursor?: string }, signal: AbortSignal): Promise<ListResponse<LogEvent>>;
  preferences(signal: AbortSignal): Promise<UserPreferences>;
  updatePreferences(input: UserPreferences, signal: AbortSignal): Promise<User>;
  signedInDevices(origin: AccountOrigin, signal: AbortSignal): Promise<AccountSignedInDevice[]>;
  updateAccountIdentity(origin: AccountOrigin, input: { displayName: string; email: string }, signal: AbortSignal): Promise<AccountIdentitySnapshot>;
  uploadAccountImage(origin: AccountOrigin, file: File, signal: AbortSignal): Promise<AccountIdentitySnapshot>;
  deleteAccountImage(origin: AccountOrigin, signal: AbortSignal): Promise<AccountIdentitySnapshot>;
  accountImageUrl(value: string): string;
  changeLocalPassword(input: { currentPassword: string; newPassword: string }, signal: AbortSignal): Promise<void>;
  changePorticoPassword(input: { currentPassword: string; newPassword: string }, signal: AbortSignal): Promise<void>;
  deletePorticoAccount(input: { password: string; mfaCode?: string; recoveryCode?: string }, signal: AbortSignal): Promise<void>;
  porticoMFAStatus(signal: AbortSignal): Promise<AccountMFAStatus>;
  startPorticoMFA(password: string, signal: AbortSignal): Promise<AccountMFASetup>;
  enablePorticoMFA(input: { code: string; enrollmentToken: string }, signal: AbortSignal): Promise<AccountMFAEnableResult>;
  disablePorticoMFA(input: { password: string; code: string }, signal: AbortSignal): Promise<void>;
  revokeSignedInDevice(origin: AccountOrigin, id: string, signal: AbortSignal): Promise<void>;
  clearWatchHistory(signal: AbortSignal): Promise<void>;
  signOut(signal: AbortSignal): Promise<void>;
}

export type QueryState<T> =
  | { status: 'loading'; data?: undefined; error?: undefined }
  | { status: 'success'; data: T; error?: undefined }
  | { status: 'error'; data?: undefined; error: Error };
