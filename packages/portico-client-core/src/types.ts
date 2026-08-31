import type { components as HostedComponents } from "./hosted-openapi-types.js";
import type { components as MainComponents } from "./openapi-types.js";

type MainSchema<Name extends keyof MainComponents["schemas"]> = MainComponents["schemas"][Name];
type HostedSchema<Name extends keyof HostedComponents["schemas"]> = HostedComponents["schemas"][Name];

// Hosted Services TV setup is intentionally distinct from the server-local TV
// setup protocol below. Native TV clients can start this flow before a server
// has been selected, then use the returned scoped poll secret while the signed
// Portico Account grant is prepared.
export type HostedTVSetupSessionRequest = HostedSchema<"TVSetupSessionRequest">;
export type HostedTVSetupSession = HostedSchema<"TVSetupSessionResponse">;
export type HostedTVSetupGrantRequest = HostedSchema<"TVSetupGrantRequest">;
export type HostedTVSetupGrantResponse = HostedSchema<"TVSetupGrantResponse">;
export type HostedTVSetupPreviewRequest = HostedSchema<"TVSetupPreviewRequest">;
export type HostedTVSetupPreviewResponse = HostedSchema<"TVSetupPreviewResponse">;

// Generic limited-input device authorization follows RFC 8628 polling and
// terminal-state semantics while redeeming a Hosted Portico Account credential
// family. Server discovery, selection, and server-scoped credential attachment
// are intentionally separate operations after account authorization. The
// returned deviceCode is a secret and must remain in platform-protected client
// storage; userCode is the only value intended for display.
export type HostedDeviceAuthorizationSessionRequest = HostedSchema<"DeviceAuthorizationSessionRequest">;
export type HostedDeviceAuthorizationSession = HostedSchema<"DeviceAuthorizationSessionCreateResponse">;
export type HostedDeviceAuthorizationStatus = HostedSchema<"DeviceAuthorizationSessionStatusResponse">;
export type HostedDeviceAuthorizationPreviewRequest = HostedSchema<"DeviceAuthorizationPreviewRequest">;
export type HostedDeviceAuthorizationPreviewResponse = HostedSchema<"DeviceAuthorizationPreviewResponse">;
export type HostedDeviceAuthorizationDecisionRequest = HostedSchema<"DeviceAuthorizationDecisionRequest">;
export type HostedDeviceAuthorizationDecisionResponse = HostedSchema<"DeviceAuthorizationDecisionResponse">;
export type HostedDeviceAuthorizationRedeemResponse = HostedSchema<"DeviceAuthorizationRedeemResponse">;

// Portico Account profiles are Cloud-owned. These aliases deliberately expose
// the generated wire contract so every client uses the same request and
// response shapes rather than recreating profile transport models per shell.
export type HostedAccountProfile = HostedSchema<"PorticoProfile">;
export type HostedAccountProfileProjection = HostedSchema<"AccountProfileProjection">;
export type HostedAccountProfileDirectory = HostedSchema<"AccountProfileDirectory">;
export type HostedAccountProfileList = HostedAccountProfileDirectory;
export type HostedAccountProfileMutationResponse = HostedSchema<"AccountProfileMutationResponse">;
export type HostedAccountProfileDeleteResponse = HostedSchema<"AccountProfileDeleteResponse">;
export type HostedAccountProfileCreateRequest = HostedSchema<"AccountProfileCreateRequest">;
export type HostedAccountProfileUpdateRequest = HostedSchema<"AccountProfileUpdateRequest">;
export type HostedAccountProfileReorderRequest = HostedSchema<"AccountProfileReorderRequest">;
export type HostedProfileAdministrationSessionRequest = HostedSchema<"ProfileAdministrationSessionRequest">;
export type HostedProfileAdministrationSessionResponse = HostedSchema<"ProfileAdministrationSessionResponse">;
/**
 * Short-lived Cloud proof used only while administering account profiles.
 * The token must remain in memory and is bound server-side to the authenticated
 * account session, device record, refresh family, and primary PIN revision.
 */
export type HostedProfileAdministrationProof = {
  token: string;
};
export type HostedProfilePINSetRequest = HostedSchema<"ProfilePINSetRequest">;
export type HostedProfilePINClearRequest = HostedSchema<"ProfilePINClearRequest">;
export type HostedProfilePINChangeResponse = HostedSchema<"ProfilePINChangeResponse">;
export type HostedProfileSelectionAssertionRequest = HostedSchema<"ProfileSelectionAssertionRequest">;
export type HostedProfileSelectionEnvelope = HostedSchema<"HostedProfileSelectionEnvelope">;
export type HostedProfileDirectorySnapshotRequest = HostedSchema<"HostedProfileDirectorySnapshotRequest">;
export type HostedProfileDirectorySnapshot = HostedSchema<"HostedProfileDirectorySnapshot">;

// Main Server wire shapes. Keep these aliases ergonomic for clients while the
// generated OpenAPI document remains the sole owner of their fields and enums.
export type Permissions = MainSchema<"Permissions">;
export type User = MainSchema<"User">;
export type SignInMethod = MainSchema<"SignInMethod">;
export type UserPreferences = MainSchema<"UserPreferences">;
export type MusicPlaybackPreferences = MainSchema<"MusicPlaybackPreferences">;
export type PlaybackProgressPreferences = MainSchema<"PlaybackProgressPreferences">;
export type UserPrivacyPreferences = MainSchema<"UserPrivacyPreferences">;
export type UserAccessSchedule = MainSchema<"UserAccessSchedule">;
export type UserTagPolicy = MainSchema<"UserTagPolicy">;
export type UserDevicePolicy = MainSchema<"UserDevicePolicy">;
export type UserChannelPolicy = MainSchema<"UserChannelPolicy">;
export type DisplayPreference = MainSchema<"DisplayPreference">;
export type DisplayPreferenceRequest = MainSchema<"DisplayPreferenceRequest">;
export type SuccessResponse = MainSchema<"SuccessResponse">;
export type UserCreateRequest = MainSchema<"UserCreateRequest">;
export type UserPatchRequest = MainSchema<"UserPatchRequest">;
export type AuthMeResponse = MainSchema<"AuthMe">;
export type ProfileDeviceDescriptor = MainSchema<"ProfileDeviceDescriptor">;
export type SelectableProfile = MainSchema<"SelectableProfile">;
export type LocalProfileAccountAuthenticationRequest = MainSchema<"LocalProfileAccountAuthenticationRequest">;
export type ProfileAccountAuthenticationResponse = MainSchema<"ProfileAccountAuthenticationResponse">;
export type LocalProfileSelectionRequest = MainSchema<"LocalProfileSelectionRequest">;
export type ActiveLocalProfileSelectionRequest = MainSchema<"ActiveLocalProfileSelectionRequest">;
export type ProfileSelectionResponse = MainSchema<"ProfileSelectionResponse">;
export type ServerManagedProfileDirectory = MainSchema<"ManagedProfileDirectory">;
export type ServerProfileAdministrationProofRequest = MainSchema<"ProfileAdministrationProofRequest">;
export type ServerProfileAdministrationProofResponse = MainSchema<"ProfileAdministrationProofResponse">;
export type ServerLocalProfilePINSetRequest = MainSchema<"LocalProfilePINSetRequest">;
export type ServerLocalProfilePINClearRequest = MainSchema<"LocalProfilePINClearRequest">;
export type ServerManagedProfileCreateRequest = MainSchema<"CreateManagedProfileRequest">;
export type ServerManagedProfileUpdateRequest = MainSchema<"UpdateManagedProfileRequest">;
export type ServerProfileErasureResponse = MainSchema<"ProfileErasureResponse">;
export type ServerAutomaticProfileTrustRequest = MainSchema<"AutomaticProfileTrustRequest">;
export type ServerAutomaticProfileTrust = MainSchema<"AutomaticProfileTrust">;
export type ServerViewerPreferenceBundle = MainSchema<"ViewerPreferenceBundle">;
export type ServerViewerPreferencePatch = MainSchema<"ViewerPreferencePatch">;
export type ServerViewerProfileActivationRequest = MainSchema<"ViewerProfileActivationRequest">;
export type ServerAccountInstallationPreferenceDocument = MainSchema<"AccountServerInstallationPreferenceDocument">;
export type ServerPatchedPreferenceDocument = MainSchema<"PatchViewerPreferenceDocumentResponse">;
export type ServerViewerNotificationPage = MainSchema<"ViewerNotificationPage">;
export type ServerNotificationReceiptMutation = MainSchema<"NotificationReceiptMutation">;
export type ServerNotificationReceiptResult = MainSchema<"NotificationReceiptResult">;
export type ServerNotificationReadAllResult = MainSchema<"NotificationReceipt">;
export type ServerViewerFeedbackCapabilities = MainSchema<"ViewerFeedbackCapabilities">;
export type ServerViewerFeedbackSubmission = MainSchema<"ViewerFeedbackSubmission">;
export type ServerViewerFeedbackReceipt = MainSchema<"ViewerFeedbackReceipt">;
export type ServerOwnerFeedbackPage = MainSchema<"OwnerFeedbackPage">;
export type ServerOwnerFeedbackUpdateRequest = MainSchema<"OwnerFeedbackUpdateRequest">;
export type ServerOwnerFeedbackRecord = MainSchema<"OwnerFeedbackRecord">;
export type ServerOwnerNotificationRecipientDirectory = MainSchema<"OwnerNotificationRecipientDirectory">;
export type ServerOwnerNoticeRequest = MainSchema<"OwnerNoticeRequest">;
export type ServerViewerNotification = MainSchema<"ViewerNotification">;
export type BrowserProfileSessionRequest = MainSchema<"BrowserProfileSessionRequest">;
export type NativeProfileSessionRequest = MainSchema<"NativeProfileSessionRequest">;
export type PorticoSessionAttachPayload = MainSchema<"PorticoSessionAttachPayload">;
export type PorticoAttachmentHandshakeRequest = MainSchema<"PorticoAttachmentHandshakeRequest">;
export type PorticoAttachmentHandshakeResponse = MainSchema<"PorticoAttachmentHandshakeResponse">;
export type PorticoAttachmentEncryptedRequest = MainSchema<"PorticoAttachmentEncryptedRequest">;
export type PorticoAttachmentEncryptedResponse = MainSchema<"PorticoAttachmentEncryptedResponse">;
export type NativeSessionCreateRequest = MainSchema<"NativeSessionCreateRequest">;
export type NativeSessionRefreshRequest = MainSchema<"NativeSessionRefreshRequest">;
export type NativeSessionCredentials = MainSchema<"NativeSessionCredentials">;
export type AppEvent = MainSchema<"AppEvent">;
export type AuthCapabilitiesResponse = MainSchema<"AuthCapabilities">;
export type ServerCapabilitiesResponse = MainSchema<"ServerCapabilitiesResponse">;
export type PublicLoginUser = MainSchema<"PublicLoginUser">;
export type QuickConnectStartResponse = MainSchema<"QuickConnectStartResponse">;
export type QuickConnectStatusResponse = MainSchema<"QuickConnectStatus">;
export type QuickConnectRequest = MainSchema<"QuickConnectRequest">;
export type SystemIdentity = MainSchema<"SystemIdentity">;
export type SystemStatusResponse = MainSchema<"SystemStatus">;
export type ConnectionUrl = MainSchema<"ConnectionURL">;
export type RemoteAccessInfo = MainSchema<"RemoteAccessInfo">;
export type RemoteAccessSettings = MainSchema<"RemoteAccessSettings">;
export type RemoteAccessRoute = MainSchema<"RemoteAccessRoute">;
export type RemoteAccessEndpoint = MainSchema<"RemoteAccessEndpoint">;
export type RemoteAccessClaim = MainSchema<"RemoteAccessClaim">;
export type RemoteAccessMember = MainSchema<"RemoteAccessMember">;
export type RemotePolicySync = MainSchema<"RemotePolicySync">;
export type RemoteAccessStatus = MainSchema<"RemoteAccessStatus">;
export type RemoteAccessSettingsPatch = MainSchema<"RemoteAccessSettingsPatch">;
export type RemoteAccessHealthResponse = MainSchema<"GetRemoteAccessHealthResponse">;
export type NetworkConnectionInfo = MainSchema<"NetworkConnectionInfo">;
export type DLNALibraryExposure = MainSchema<"DLNALibraryExposure">;
export type DLNAStatus = MainSchema<"DLNAStatus">;
export type LibraryScanSummary = MainSchema<"LibraryScanSummary">;
export type LibraryScanRequest = MainSchema<"LibraryScanRequest">;
export type LibraryScanRetryRequest = MainSchema<"LibraryScanRetryRequest">;
export type LibraryScanOperation = MainSchema<"LibraryScanOperation">;
export type LibraryScanOperationsResponse = MainSchema<"LibraryScanOperationsResponse">;
export type LibraryScanReviewResponse = MainSchema<"LibraryScanReviewResponse">;
export type LibraryScanRun = MainSchema<"LibraryScanRun">;
export type LibraryScanRunListResponse = MainSchema<"LibraryScanRunListResponse">;
export type Library = MainSchema<"Library">;
export type RemoteStorageAnalysisMode = "file_list_only" | "basic" | "complete" | "custom";
export type RemoteStorageAnalysisModeRequest = { analysisMode: RemoteStorageAnalysisMode };
export type RemoteStorageSourceRequest = Omit<MainSchema<"RemoteStorageSourceRequest">, "analysisMode"> & { analysisMode?: RemoteStorageAnalysisMode };
export type RemoteStorageSource = Omit<MainSchema<"RemoteStorageSource">, "analysisMode"> & { analysisMode: RemoteStorageAnalysisMode };
export type RemoteStorageSourceListResponse = Omit<MainSchema<"RemoteStorageSourceListResponse">, "items"> & { items: RemoteStorageSource[] };
export type LibraryNavigationPreferences = MainSchema<"LibraryNavigationPreferences">;
export type LibraryNavigationPreferencesRequest = MainSchema<"LibraryNavigationPreferencesRequest">;
export type ProductContract = MainSchema<"ProductContract">;
export type ProductLanguageCatalog = MainSchema<"ProductLanguageCatalog">;
export type ProductLanguageReference = MainSchema<"ProductLanguageReference">;
export type LibraryBrowseCapabilities = MainSchema<"LibraryBrowseCapabilities">;
export type BrowseLibraryRequest = MainSchema<"BrowseLibraryRequest">;
export type BrowseSeek = MainSchema<"BrowseSeek">;
export type BrowseExpression = MainSchema<"BrowseExpression">;
export type BrowseLibraryResponse = MainSchema<"BrowseLibraryResponse">;
export type MediaCard = MainSchema<"MediaCard">;
export type FilesystemBrowseEntry = MainSchema<"FilesystemBrowseEntry">;
export type FilesystemRoot = MainSchema<"FilesystemRoot">;
export type FilesystemBrowseResponse = MainSchema<"FilesystemBrowseResponse">;
export type FilesystemCreateDirectoryRequest = MainSchema<"FilesystemCreateDirectoryRequest">;
export type StoragePathsResponse = MainSchema<"StoragePathsResponse">;
export type LibraryCategory = MainSchema<"LibraryCategory">;
export type LibraryFacetValue = MainSchema<"LibraryFacetValue">;
export type LibrarySourceGroup = MainSchema<"LibrarySourceGroup">;
export type ImageSet = MainSchema<"ImageSet">;
export type MediaState = MainSchema<"MediaState">;
export type Stream = MainSchema<"Stream">;
export type MediaAttachment = MainSchema<"MediaAttachment">;
export type MediaSegment = MainSchema<"MediaSegment">;
export type AudioNormalization = MainSchema<"AudioNormalization">;
export type MediaSegmentRequest = MainSchema<"MediaSegmentRequest">;
export type MediaTrickplaySet = MainSchema<"MediaTrickplaySet">;
export type OptimizedVersion = MainSchema<"OptimizedVersion">;
export type OptimizedVersionProfile = MainSchema<"OptimizedVersionProfile">;
export type OptimizedVersionListResponse = MainSchema<"OptimizedVersionListResponse">;
export type OptimizedVersionRequest = MainSchema<"OptimizedVersionRequest">;
export type DownloadOption = MainSchema<"DownloadOption">;
export type DownloadOptionsResponse = MainSchema<"DownloadOptionsResponse">;
export type DownloadPreparation = MainSchema<"DownloadPreparation">;
export type DownloadPreparationCreateRequest = MainSchema<"DownloadPreparationCreateRequest">;
export type DownloadPreparationBatchFailure = MainSchema<"DownloadPreparationBatchFailure">;
export type DownloadPreparationBatchResponse = MainSchema<"DownloadPreparationBatchResponse">;
export type DownloadPreparationSingleCreateRequest = { mediaId: string; qualityProfile?: string };
export type DownloadPreparationBatchCreateRequest = ({ mediaIds: string[] } | { containerId: string }) & { qualityProfile?: string };
export type DownloadPreparationNextEpisodeRequest = { nextAfterMediaId: string; qualityProfile?: string };
export type DownloadPreparationUpdateRequest = MainSchema<"DownloadPreparationUpdateRequest">;
export type DownloadPreparationGrantRequest = MainSchema<"DownloadPreparationGrantRequest">;
export type MediaDownloadGrantResponse = MainSchema<"MediaDownloadGrantResponse">;
export type LiveTVSource = MainSchema<"LiveTVSource">;
export type LiveTVSourceSummary = MainSchema<"LiveTVSourceSummary">;
export type LiveTVSourceRequest = MainSchema<"LiveTVSourceRequest">;
export type HDHomeRunDiscoveryCandidate = MainSchema<"HDHomeRunDiscoveryCandidate">;
export type LiveTVChannel = MainSchema<"LiveTVChannel">;
export type LiveTVChannelPageResponse = MainSchema<"LiveTVChannelListResponse">;
export type LiveTVChannelStateRequest = MainSchema<"LiveTVChannelStateRequest">;
export type LiveTVProgram = MainSchema<"LiveTVProgram">;
export type LiveTVGuideResponse = MainSchema<"LiveTVGuide">;
export type LiveTVGuideFilter = "all" | "favorites" | "hd" | "sports" | "news" | "movies";
export type LiveTVGuideSort = "default" | "name" | "now" | "favorites" | "recent";
export interface LiveTVGuideParams {
  from?: string;
  hours?: number;
  limit?: number;
  cursor?: string;
  count?: "none" | "exact";
  query?: string;
  filter?: LiveTVGuideFilter;
  sort?: LiveTVGuideSort;
  order?: "asc" | "desc";
  group?: string;
}
export interface LiveTVChannelBrowseParams {
  limit?: number;
  cursor?: string;
  count?: "none" | "exact";
  query?: string;
  favoritesOnly?: boolean;
  group?: string;
}
export type MediaItem = MainSchema<"MediaItem">;
export type MediaExtraRelationship = MainSchema<"MediaExtraRelationship">;
export type MediaLyric = MainSchema<"MediaLyric">;
export type LyricSearchCandidate = MainSchema<"LyricSearchCandidate">;
export type MediaPerson = MainSchema<"MediaPerson">;
export type PersonSummary = MainSchema<"PersonSummary">;
export type PersonDetailResponse = MainSchema<"PersonDetailResponse">;
export type MediaChildrenResponse = MainSchema<"MediaChildrenResponse">;
export type MediaImage = MainSchema<"MediaImage">;
export type MetadataRepairItem = MainSchema<"MetadataRepairItem">;
export type MetadataRepairResponse = MainSchema<"MetadataRepairResponse">;
export type MetadataHealthIssue = MainSchema<"MetadataHealthIssue">;
export type MetadataHealthSummary = MainSchema<"MetadataHealthSummary">;
export type MetadataHealthResponse = MainSchema<"MetadataHealthResponse">;
export type MetadataHealthActionResponse = MainSchema<"MetadataHealthActionResponse">;
export type MetadataRepairActionResponse = MainSchema<"MetadataRepairActionResponse">;
export type UpdateMediaRequest = MainSchema<"UpdateMediaRequest">;
export type HomeRow = MainSchema<"HomeRow">;
export type HomeResponse = MainSchema<"HomeResponse">;
export type SearchRequest = MainSchema<"SearchRequest">;
export type SearchGroup = MainSchema<"SearchGroup">;
export type SearchResponse = MainSchema<"SearchResponse">;
export type SearchHistoryItem = MainSchema<"SearchHistoryItem">;
export type SearchHistoryResponse = MainSchema<"SearchHistoryResponse">;
export type MediaSuggestion = MainSchema<"MediaSuggestion">;
export type SuggestionsResponse = MainSchema<"SuggestionsResponse">;
export type Device = MainSchema<"Device">;
export type DeviceUpdateRequest = MainSchema<"DeviceUpdateRequest">;
export type DeviceOptions = MainSchema<"DeviceOptions">;
export type AccountSession = MainSchema<"AccountSession">;
export type AccountPasswordChangeRequest = MainSchema<"AccountPasswordChangeRequest">;
export type APIKey = MainSchema<"APIKey">;
export type APIKeyCreateResponse = MainSchema<"APIKeyCreateResponse">;
export type AuditEvent = MainSchema<"AuditEvent">;
export type BackupInfo = MainSchema<"BackupInfo"> & {
  databaseFormatVersion?: number;
  migrationHead?: string;
  migrationLedgerSha256?: string;
  migrationLedgerRows?: number;
  minimumReader?: string;
  validationCode?: string;
  publicationState?: "published" | "degraded";
  warningCode?: string;
  warningMessage?: string;
};
export type SystemDiagnostics = MainSchema<"SystemDiagnostics">;
export type StartupDiagnostics = MainSchema<"StartupDiagnostics">;
export type StartupPhaseDiagnostic = MainSchema<"StartupPhaseDiagnostic">;
export type RuntimeDiagnostics = MainSchema<"RuntimeDiagnostics">;
export type IOPressureDiagnostics = MainSchema<"IOPressureDiagnostics">;
export type SQLiteDiagnostics = MainSchema<"SQLiteDiagnostics">;
export type SQLiteHealthDiagnostic = MainSchema<"SQLiteHealthDiagnostic">;
export type ResourceDiagnostics = MainSchema<"ResourceDiagnostics">;
export type JobLaneDiagnostic = MainSchema<"JobLaneDiagnostic">;
export type WorkloadLaneDiagnostic = MainSchema<"WorkloadLaneDiagnostic">;
export type SystemTimeSync = MainSchema<"SystemTimeSync">;
export type SystemReleaseInfo = MainSchema<"SystemReleaseInfo">;
export type SystemStorageReport = MainSchema<"SystemStorageReport">;
export type SystemStorageCategory = MainSchema<"SystemStorageCategory">;
export type SystemStorageCleanupResponse = MainSchema<"SystemStorageCleanupResponse">;
export type RuntimeDependency = MainSchema<"RuntimeDependency">;
export type BrandingInfo = MainSchema<"BrandingInfo">;
export type RestoreBackupState =
  | "validating"
  | "staged"
  | "quiescing"
  | "safety-copy"
  | "installing"
  | "reopening/migrating"
  | "health-checking"
  | "complete"
  | "rolling-back"
  | "failed"
  | "recovery-required";

/** Destructive confirmation and local-password or signed Hosted step-up. */
export type RestoreBackupRequest = MainSchema<"RestoreBackupRequest">;
export type RestoreAuthorizationContext = MainSchema<"RestoreAuthorizationContext">;

/** Multipart input for a raw database import or a manifest-verified backup import. */
type RestoreUploadedDatabaseBase = {
  file: Blob;
  confirmation: string;
  manifest?: Blob | string;
};
export type RestoreUploadedDatabaseInput = RestoreUploadedDatabaseBase & (
  | { password: string; hostedAuthorization?: never }
  | { password?: never; hostedAuthorization: HostedServerRestoreAuthorization }
);

/** Capability-bearing restore response; statusToken is returned only when enqueueing. */
export interface RestoreBackupResponse {
  ok: boolean;
  name: string;
  operationId: string;
  sourceKind?: "catalog-backup" | "raw-import";
  manifestVerified?: boolean;
  maxDatabaseBytes?: number;
  recoveryRequired: boolean;
  state: RestoreBackupState;
  phase?: Exclude<RestoreBackupState, "recovery-required">;
  progress?: number;
  validationCode?: string;
  errorCode?: string;
  errorMessage?: string;
  warningCode?: string;
  warningMessage?: string;
  instruction: string;
  statusToken?: string;
}
export type SavedResourceShare = MainSchema<"SavedResourceShare">;
export type SavedResourceShareRequest = MainSchema<"SavedResourceShareRequest">;
export type SavedShareCandidate = MainSchema<"SavedShareCandidate">;
export type SavedShareCandidatePage = MainSchema<"SavedShareCandidatePage">;
export type CollectionSummary = MainSchema<"CollectionSummary">;
export type Collection = MainSchema<"Collection">;
export type CollectionPage = MainSchema<"CollectionPage">;
export type CollectionCreateRequest = MainSchema<"CollectionCreateRequest">;
export type CollectionUpdateRequest = MainSchema<"CollectionUpdateRequest">;
export type CollectionMembershipBatchRequest = MainSchema<"CollectionMembershipBatchRequest">;
export type CollectionMembershipBatchResponse = MainSchema<"CollectionMembershipBatchResponse">;
export type PlaylistSummary = MainSchema<"PlaylistSummary">;
export type SavedPlaylist = MainSchema<"SavedPlaylist">;
export type PlaylistPage = MainSchema<"PlaylistPage">;
export type PlaylistCreateRequest = MainSchema<"PlaylistCreateRequest">;
export type PlaylistUpdateRequest = MainSchema<"PlaylistUpdateRequest">;
export type PlaylistEntry = MainSchema<"PlaylistEntry">;
export type PlaylistEntryPage = MainSchema<"PlaylistEntryPage">;
export type PlaylistItemsBatchRequest = MainSchema<"PlaylistItemsBatchRequest">;
export type PlaylistItemsBatchResponse = MainSchema<"PlaylistItemsBatchResponse">;
export type SavedMediaPage = MainSchema<"SavedMediaPage">;
export type SavedView = MainSchema<"SavedView">;
export type SavedViewPage = MainSchema<"SavedViewPage">;
export type SavedViewCreateRequest = MainSchema<"SavedViewCreateRequest">;
export type SavedViewUpdateRequest = MainSchema<"SavedViewUpdateRequest">;
export type SavedViewBrowseRequest = MainSchema<"SavedViewBrowseRequest">;
export type DVRRecordingRule = MainSchema<"DVRRecordingRule">;
export type DVRRecording = MainSchema<"DVRRecording">;
export type DVRRecordingGroup = MainSchema<"DVRRecordingGroup">;
export type DVRPlaybackSessionCreateRequest = MainSchema<"DVRPlaybackSessionCreateRequest">;
export type DVRConflict = MainSchema<"DVRConflict">;
export type DVRTunerAllocation = MainSchema<"DVRTunerAllocation">;
export type DVRGuideOperationalStatus = MainSchema<"DVRGuideOperationalStatus">;
export type DVRStorageOperationalStatus = MainSchema<"DVRStorageOperationalStatus">;
export type DVROperationalStatus = MainSchema<"DVROperationalStatus">;
export type DVRConsumerStatus = MainSchema<"DVRConsumerStatus">;
export type Job = MainSchema<"Job">;
export type JobCancelResponse = MainSchema<"JobCancelResponse">;
export type ScheduledTask = MainSchema<"ScheduledTask">;
export type ScheduledTaskTrigger = MainSchema<"ScheduledTaskTrigger">;
export type ScheduledTaskUpdateRequest = MainSchema<"ScheduledTaskUpdateRequest">;
export type ScheduledTaskRunResponse = MainSchema<"ScheduledTaskRunResponse">;
export type LogEvent = MainSchema<"LogEvent">;
export type ClientLogUploadRequest = MainSchema<"ClientLogUploadRequest">;
export type ClientLogUploadResponse = MainSchema<"ClientLogUploadResponse">;
export type DashboardResponse = MainSchema<"DashboardResponse">;
export type DashboardWatchedToday = MainSchema<"DashboardWatchedToday">;
export type ServerActivityResponse = MainSchema<"ServerActivityResponse">;
export type DashboardOverviewUsageResponse = MainSchema<"DashboardOverviewUsageResponse">;
export type PlaybackSession = MainSchema<"PlaybackSession">;
export type PlaybackQueueEntry = MainSchema<"PlaybackQueueEntry">;
export type PlaybackQueueHistoryEntry = MainSchema<"PlaybackQueueHistoryEntry">;
export type PlaybackRepeatMode = MainSchema<"PlaybackRepeatMode">;
export type PlaybackSessionQueueReplaceRequest = MainSchema<"PlaybackSessionQueueReplaceRequest">;
export type PlaybackSessionQueueRequest = MainSchema<"PlaybackSessionQueueRequest">;
export type PlaybackProgressEvent = MainSchema<"PlaybackProgressEvent">;
export type PlaybackProgressInput = Omit<PlaybackProgressEvent, "eventSequence" | "recordedAt"> & Partial<Pick<PlaybackProgressEvent, "eventSequence" | "recordedAt">>;
export type PlaybackProgressAcknowledgement = MainSchema<"PlaybackProgressAcknowledgement">;
export type PlaybackTerminalEvent = MainSchema<"PlaybackTerminalEvent">;
export type PlaybackSessionStopRequest = MainSchema<"PlaybackSessionStopRequest">;
export type PlaybackTerminalInput = Pick<
  PlaybackTerminalEvent,
  "disposition" | "positionSeconds" | "durationSeconds"
>;
export type PlaybackSessionStopInput =
  | PlaybackTerminalInput
  | PlaybackSessionStopRequest;
export type PlaybackSessionTerminalAcknowledgement = MainSchema<"PlaybackSessionTerminalAcknowledgement">;
export type PlaybackContinuationCredential = MainSchema<"PlaybackContinuationCredential">;
export type PlaybackContinuationState = MainSchema<"PlaybackContinuationState">;
export type PlaybackContinuationRotateRequest = MainSchema<"PlaybackContinuationRotateRequest">;
export type PlaybackSessionQueueResponse = MainSchema<"PlaybackSessionQueueResponse">;
export type PlaybackPrepareNextRequest = MainSchema<"PlaybackPrepareNextRequest">;
export type PlaybackPreparedResponse = MainSchema<"PlaybackPreparedResponse">;
export type PlaybackHandoffRequest = MainSchema<"PlaybackHandoffRequest">;
export type PlaybackHandoffInput = Omit<PlaybackHandoffRequest, "previousTerminal"> & {
  previousTerminal: PlaybackTerminalEvent | PlaybackTerminalInput;
};
export type PlaybackReplacementRequest = MainSchema<"PlaybackReplacementRequest">;
export type PlaybackRenegotiationRequest = MainSchema<"PlaybackRenegotiationRequest">;
export type PlaybackSourceContext = MainSchema<"PlaybackSourceContext">;
export type PlaybackDiagnostics = MainSchema<"PlaybackDiagnostics">;
export type PlaybackCommand = MainSchema<"PlaybackCommand">;
export type PlaybackReceiver = MainSchema<"PlaybackReceiver">;
export type PlaybackReceiverRequest = MainSchema<"PlaybackReceiverRequest">;
export type PlaybackReceiverHeartbeatRequest = MainSchema<"PlaybackReceiverHeartbeatRequest">;
export type PlaybackReceiverHeartbeatResponse = MainSchema<"PlaybackReceiverHeartbeatResponse">;
export type PlaybackReceiverHandoffRequest = MainSchema<"PlaybackReceiverHandoffRequest">;
export type PlaybackReceiverHandoffCommitRequest = MainSchema<"PlaybackReceiverHandoffCommitRequest">;
export type PlaybackReceiverHandoffStatusResponse = MainSchema<"PlaybackReceiverHandoffStatusResponse">;
export type ReceiverAuthorizationRequest = MainSchema<"ReceiverAuthorizationRequest">;
export type ReceiverControllerGrant = MainSchema<"ReceiverControllerGrant">;
export type ReceiverAuthorizationRecord = MainSchema<"ReceiverAuthorizationRecord">;
export type WatchWithFriendsGroup = MainSchema<"WatchWithFriendsGroup">;
export type WatchWithFriendsMember = MainSchema<"WatchWithFriendsMember">;
export type WatchWithFriendsCreateRequest = MainSchema<"WatchWithFriendsCreateRequest">;
export type WatchWithFriendsStateRequest = MainSchema<"WatchWithFriendsStateRequest">;
export type WatchWithFriendsMemberStateRequest = MainSchema<"WatchWithFriendsMemberStateRequest">;
export type WatchWithFriendsSettingsRequest = MainSchema<"WatchWithFriendsSettingsRequest">;
export type WatchWithFriendsQueueItem = MainSchema<"WatchWithFriendsQueueItem">;
export type WatchWithFriendsQueueRequest = MainSchema<"WatchWithFriendsQueueRequest">;
export type WatchWithFriendsQueueOrderRequest = MainSchema<"WatchWithFriendsQueueOrderRequest">;
export type PlaybackTarget = MainSchema<"PlaybackTarget">;
export type DashboardMetric = MainSchema<"DashboardMetric">;
export type DashboardSample = MainSchema<"DashboardSample">;
export type DashboardSystem = MainSchema<"DashboardSystem">;
export type DashboardSystemSample = MainSchema<"DashboardSystemSample">;
export type DashboardGPUSample = MainSchema<"DashboardGPUSample">;
export type DashboardDiskIOSample = MainSchema<"DashboardDiskIOSample">;
export type DashboardGPUInfo = MainSchema<"DashboardGPUInfo">;
export type DashboardTopUser = MainSchema<"DashboardTopUser">;
export type DashboardTopUserLibrary = MainSchema<"DashboardTopUserLibrary">;
export type PlayHistoryPoint = MainSchema<"PlayHistoryPoint">;
export type TopPlayedGroup = MainSchema<"TopPlayedGroup">;
export type TopPlayedItem = MainSchema<"TopPlayedItem">;
export type TranscodeSession = MainSchema<"TranscodeSession">;
export type TranscodeCapacityReport = MainSchema<"TranscodeCapacityReport">;
export type TranscodeProbe = MainSchema<"TranscodeProbe">;
export type TranscodePresetInfo = MainSchema<"TranscodePresetInfo">;
export type DashboardNotice = MainSchema<"DashboardNotice">;
export type ConversionJob = MainSchema<"ConversionJob">;
export type LibraryStat = MainSchema<"LibraryStat">;
export type PlaybackClientProfile = MainSchema<"PlaybackClientProfile">;
export type PlaybackQualitySelection = MainSchema<"PlaybackQualitySelection">;
export type PlaybackQualityOffer = MainSchema<"PlaybackQualityOffer">;
export type PlaybackQualityOfferSet = MainSchema<"PlaybackQualityOfferSet">;
export type PlaybackIntent = MainSchema<"PlaybackIntent">;
export type PlaybackRenegotiationIntent = MainSchema<"PlaybackRenegotiationIntent">;
export type CastBootstrapRequest = MainSchema<"CastBootstrapRequest">;
export type CastBootstrapResponse = MainSchema<"CastBootstrapResponse">;
export type CastRedeemRequest = MainSchema<"CastRedeemRequest">;
export type CastReceiverSessionResponse = MainSchema<"CastReceiverSessionResponse">;
export type CastReceiverSessionState = MainSchema<"CastReceiverSessionState">;
export type CastReconnectRequest = MainSchema<"CastReconnectRequest">;
export type CastControlRequest = MainSchema<"CastControlRequest">;
export type CastProgressRequest = MainSchema<"CastProgressRequest">;
export type CastRenewRequest = MainSchema<"CastRenewRequest">;
export type CastRenewResponse = MainSchema<"CastRenewResponse">;
export type CastStopRequest = MainSchema<"CastStopRequest">;
export type CastStopResponse = MainSchema<"CastStopResponse">;
export type CastAdvanceRequest = MainSchema<"CastAdvanceRequest">;
export type CastAdvanceCancelRequest = MainSchema<"CastAdvanceCancelRequest">;
export type CastAdvanceResponse = MainSchema<"CastAdvanceResponse">;
export type CastTransferStatusRequest = MainSchema<"CastTransferStatusRequest">;
export type CastTransferStatusResponse = MainSchema<"CastTransferStatusResponse">;
export type CastSegmentSkipRequest = MainSchema<"CastSegmentSkipRequest">;
export type CastSegmentSkipResponse = MainSchema<"CastSegmentSkipResponse">;
export type CastOperationResponse = MainSchema<"CastOperationResponse">;
export type LiveTvStreamCloseRequest = PlaybackSessionStopRequest & {
  sessionId: string;
};
export type ResolvedPlaybackPolicy = MainSchema<"ResolvedPlaybackPolicy">;
export type PlaybackDecision = MainSchema<"PlaybackDecision">;
export type PlaybackResource = MainSchema<"PlaybackResource">;
export type PlaybackResponse = MainSchema<"PlaybackResponse">;
export type MediaGrant = MainSchema<"MediaGrant">;
export type PlaybackRestoreResponse = MainSchema<"PlaybackRestoreResponse">;
export type ServerSettings = MainSchema<"ServerSettings">;
export type DeviceSettings = MainSchema<"DeviceSettings">;
export type DLNASettings = MainSchema<"DLNASettings">;
export type DVRSettings = MainSchema<"DVRSettings">;
export type LibrarySettings = MainSchema<"LibrarySettings">;
export type LanguageSettings = MainSchema<"LanguageSettings">;
export type SettingsSecretState = MainSchema<"SettingsSecretState">;
export type SettingsSecretChange = MainSchema<"SettingsSecretChange">;
export type MetadataAgentSettings = MainSchema<"MetadataAgentSettings">;
export type MetadataAgentSettingsUpdate = MainSchema<"MetadataAgentSettingsUpdate">;
export type NetworkSettings = MainSchema<"NetworkSettings">;
export type NotificationSettings = MainSchema<"NotificationSettings">;
export type OptimizedVersionSettings = MainSchema<"OptimizedVersionSettings">;
export type ScheduledTaskTriggerSettings = MainSchema<"ScheduledTaskTriggerSettings">;
export type ScheduledTaskTriggers = MainSchema<"ScheduledTaskTriggers">;
export type ScheduledTaskSettings = MainSchema<"ScheduledTaskSettings">;
export type TranscoderSettings = MainSchema<"TranscoderSettings">;
export type TroubleshootingSettings = MainSchema<"TroubleshootingSettings">;
export type SettingsGroups = MainSchema<"SettingsGroups">;
export type SettingsGroupsUpdate = MainSchema<"SettingsGroupsUpdate">;
export type SettingsApplyImpact = MainSchema<"SettingsApplyImpact">;
export type SettingsDocument = MainSchema<"SettingsDocument">;
export type SettingsGeneration = MainSchema<"SettingsGeneration">;
export type SettingsRegistryField = MainSchema<"SettingsRegistryField">;
export type SettingsRegistryGroup = MainSchema<"SettingsRegistryGroup">;
export type SettingsRegistryResponse = MainSchema<"SettingsRegistryResponse">;
export type SettingsUpdateRequest = MainSchema<"SettingsUpdateRequest">;
export type SettingsSummaryResponse = MainSchema<"SettingsSummaryResponse">;
export type SettingsGroupSummary = MainSchema<"SettingsGroupSummary">;

// The base list schema owns pagination fields; this generic narrows only its
// intentionally untyped item array for ergonomic client wrappers.
export type ListResponse<T> = Omit<MainSchema<"ListResponse">, "items"> & { items: T[] };
export type CursorPageInfo = MainSchema<"CursorPageInfo">;
export type CursorListResponse<T> = Omit<MainSchema<"CursorListResponse">, "items"> & { items: T[] };

// Hosted Services wire shapes use explicit Portico-prefixed public names so
// they cannot be confused with same-named Main Server schemas.
export type PorticoAccountUser = HostedSchema<"User">;
export type HostedSystemInfo = HostedSchema<"HostedSystemInfo">;
export type PorticoAccountAuthResponse = HostedSchema<"HostedAuthState">;
export type PorticoNativeSessionResponse = HostedSchema<"NativeSessionCredentials">;
export type PorticoDocumentSigningKeySet = HostedSchema<"DocumentSigningKeySet">;
export type PorticoMFAStatus = HostedSchema<"MFAStatus">;
export type PorticoAccountResponse = HostedSchema<"AccountResponse">;
export type PorticoDevice = HostedSchema<"Device">;
export type HostedServer = HostedSchema<"Server">;
export type HostedServerDeletionProofRequest = HostedSchema<"ServerDeletionProofRequest">;
export type HostedServerDeletionProofResponse = HostedSchema<"ServerDeletionProofResponse">;
export type HostedServerDeleteRequest = HostedSchema<"ServerDeleteRequest">;
export type HostedServerRestoreAuthorizationRequest = HostedSchema<"ServerRestoreAuthorizationRequest">;
export type HostedServerRestoreAuthorization = HostedSchema<"ServerRestoreAuthorization">;
export type PorticoMembership = HostedSchema<"Membership">;
export type PorticoMemberProfile = HostedSchema<"MemberProfile">;
export type HostedRouteDocument = HostedSchema<"RouteDocument">;
export type HostedRouteEntry = HostedRouteDocument["routes"][number];
export type PorticoPermissionTemplate = NonNullable<PorticoMembership["permissionTemplate"]>;
export type PorticoSessionBootstrapResponse = HostedSchema<"PorticoSessionBootstrap">;
export type PorticoLocalLoginAuthorizeResponse = HostedSchema<"LocalLoginAuthorizeResponse">;
export type PorticoInvite = HostedSchema<"Invitation">;

export type MatchCandidateReason = MainSchema<"MatchCandidateReason">;
export type MatchCandidate = MainSchema<"MatchCandidate">;
export type MediaMatchSearchResponse = MainSchema<"MediaMatchSearchResponse">;
export type ManualMediaMatchRequest = MainSchema<"ManualMediaMatchRequest">;

export type Chapter = MainSchema<"Chapter">;
