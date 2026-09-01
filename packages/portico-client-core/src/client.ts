import type {
  AccountPasswordChangeRequest,
  AccountSession,
  APIKey,
  APIKeyCreateResponse,
  AppEvent,
  AuditEvent,
  AuthCapabilitiesResponse,
  AuthMeResponse,
  BackupInfo,
  BrowseLibraryRequest,
  BrowseLibraryResponse,
  BrandingInfo,
  ClientLogUploadRequest,
  ClientLogUploadResponse,
  PorticoInvite,
  PorticoPermissionTemplate,
  DashboardOverviewUsageResponse,
  DashboardResponse,
  ServerActivityResponse,
  Device,
  DeviceUpdateRequest,
  DisplayPreference,
  DisplayPreferenceRequest,
  DLNAStatus,
  DownloadOptionsResponse,
  DownloadPreparation,
  DownloadPreparationBatchCreateRequest,
  DownloadPreparationBatchResponse,
  DownloadPreparationNextEpisodeRequest,
  DownloadPreparationSingleCreateRequest,
  DownloadPreparationUpdateRequest,
  MediaDownloadGrantResponse,
  DVRRecording,
  DVRRecordingGroup,
  DVRPlaybackSessionCreateRequest,
  DVRConsumerStatus,
  DVROperationalStatus,
  DVRRecordingRule,
  FilesystemCreateDirectoryRequest,
  FilesystemBrowseResponse,
  HDHomeRunDiscoveryCandidate,
  HomeRow,
  HomeResponse,
  Job,
  JobCancelResponse,
  Library,
  LibraryBrowseCapabilities,
  LibraryCategory,
  LibraryScanOperationsResponse,
  LibraryScanRequest,
  LibraryScanRetryRequest,
  LibraryScanReviewResponse,
  LibraryScanRunListResponse,
  LibrarySourceGroup,
  ProductContract,
  ProductLanguageCatalog,
  CursorListResponse,
  ListResponse,
  LiveTVChannel,
  LiveTVChannelBrowseParams,
  LiveTVChannelPageResponse,
  LiveTVChannelStateRequest,
  LiveTVGuideParams,
  LiveTVGuideResponse,
  LiveTVSource,
  LiveTVSourceSummary,
  LiveTVSourceRequest,
  LogEvent,
  LyricSearchCandidate,
  ManualMediaMatchRequest,
  MediaImage,
  MediaGrant,
  MediaItem,
  MediaMatchSearchResponse,
  MediaSegmentRequest,
  MediaTrickplaySet,
  MetadataHealthActionResponse,
  MetadataHealthResponse,
  MetadataRepairActionResponse,
  MetadataRepairResponse,
  NetworkConnectionInfo,
  LocalProfileAccountAuthenticationRequest,
  ProfileAccountAuthenticationResponse,
  LocalProfileSelectionRequest,
  ProfileSelectionResponse,
  BrowserProfileSessionRequest,
  NativeProfileSessionRequest,
  PorticoSessionAttachPayload,
  PorticoAttachmentHandshakeRequest,
  PorticoAttachmentHandshakeResponse,
  PorticoAttachmentEncryptedRequest,
  PorticoAttachmentEncryptedResponse,
  NativeSessionCreateRequest,
  NativeSessionCredentials,
  ServerAutomaticProfileTrustRequest,
  ServerAutomaticProfileTrust,
  ServerProfileErasureResponse,
  ServerManagedProfileCreateRequest,
  ServerManagedProfileDirectory,
  ServerManagedProfileUpdateRequest,
  ServerNotificationReadAllResult,
  ServerNotificationReceiptMutation,
  ServerNotificationReceiptResult,
  ServerOwnerFeedbackPage,
  ServerOwnerFeedbackRecord,
  ServerOwnerFeedbackUpdateRequest,
  ServerOwnerNotificationRecipientDirectory,
  ServerOwnerNoticeRequest,
  ServerPatchedPreferenceDocument,
  ServerProfileAdministrationProofRequest,
  ServerProfileAdministrationProofResponse,
  ServerViewerFeedbackCapabilities,
  ServerViewerFeedbackReceipt,
  ServerViewerFeedbackSubmission,
  ServerViewerNotification,
  ServerViewerNotificationPage,
  ServerViewerPreferenceBundle,
  ServerViewerPreferencePatch,
  ServerViewerProfileActivationRequest,
  ServerAccountInstallationPreferenceDocument,
  OptimizedVersionListResponse,
  OptimizedVersionRequest,
  PlaybackClientProfile,
  CastBootstrapRequest,
  CastBootstrapResponse,
  CastRedeemRequest,
  CastReceiverSessionResponse,
  CastReceiverSessionState,
  CastReconnectRequest,
  CastControlRequest,
  CastProgressRequest,
  CastRenewRequest,
  CastRenewResponse,
  CastStopRequest,
  CastStopResponse,
  CastAdvanceRequest,
  CastAdvanceCancelRequest,
  CastAdvanceResponse,
  CastTransferStatusRequest,
  CastTransferStatusResponse,
  CastSegmentSkipRequest,
  CastSegmentSkipResponse,
  LiveTvStreamCloseRequest,
  PlaybackIntent,
  PlaybackCommand,
  PlaybackHandoffInput,
  PlaybackHandoffRequest,
  PlaybackReplacementRequest,
  PlaybackRenegotiationRequest,
  PlaybackReceiver,
  PlaybackReceiverRequest,
  PlaybackReceiverHeartbeatRequest,
  PlaybackReceiverHeartbeatResponse,
  PlaybackReceiverHandoffRequest,
  PlaybackReceiverHandoffCommitRequest,
  PlaybackReceiverHandoffStatusResponse,
  ReceiverAuthorizationRequest,
  ReceiverControllerGrant,
  ReceiverAuthorizationRecord,
  PlaybackPreparedResponse,
  PlaybackPrepareNextRequest,
  PlaybackProgressAcknowledgement,
  PlaybackProgressEvent,
  PlaybackProgressInput,
  PlaybackSessionStopInput,
  PlaybackSessionStopRequest,
  PlaybackSessionTerminalAcknowledgement,
  PlaybackTerminalEvent,
  PlaybackTerminalInput,
  PlaybackContinuationCredential,
  PlaybackContinuationState,
  PlaybackContinuationRotateRequest,
  PlaybackRepeatMode,
  PlaybackRestoreResponse,
  PlaybackResponse,
  PlaybackSession,
  PlaybackSessionQueueReplaceRequest,
  PlaybackSessionQueueRequest,
  PlaybackSessionQueueResponse,
  PlaybackSourceContext,
  PlaybackTarget,
  Collection,
  CollectionCreateRequest,
  CollectionMembershipBatchRequest,
  CollectionMembershipBatchResponse,
  CollectionPage,
  CollectionUpdateRequest,
  PlaylistCreateRequest,
  PlaylistEntryPage,
  PlaylistItemsBatchRequest,
  PlaylistItemsBatchResponse,
  PlaylistPage,
  PlaylistUpdateRequest,
  SavedMediaPage,
  SavedPlaylist,
  SavedShareCandidatePage,
  SavedView,
  SavedViewBrowseRequest,
  SavedViewCreateRequest,
  SavedViewPage,
  SavedViewUpdateRequest,
  QuickConnectRequest,
  QuickConnectStartResponse,
  QuickConnectStatusResponse,
  RemoteAccessHealthResponse,
  RemoteAccessSettingsPatch,
  RemoteAccessStatus,
  RemoteStorageSource,
  RemoteStorageAnalysisModeRequest,
  RemoteStorageSourceListResponse,
  RemoteStorageSourceRequest,
  RestoreBackupRequest,
  RestoreBackupResponse,
  RestoreUploadedDatabaseInput,
  ScheduledTask,
  ScheduledTaskRunResponse,
  ScheduledTaskUpdateRequest,
  SearchRequest,
  SearchResponse,
  SearchHistoryResponse,
  PersonDetailResponse,
  MediaChildrenResponse,
  SettingsDocument,
  SettingsRegistryResponse,
  SettingsSummaryResponse,
  SettingsUpdateRequest,
  ServerCapabilitiesResponse,
  SuggestionsResponse,
  WatchWithFriendsCreateRequest,
  WatchWithFriendsGroup,
  WatchWithFriendsMemberStateRequest,
  WatchWithFriendsQueueOrderRequest,
  WatchWithFriendsQueueRequest,
  WatchWithFriendsSettingsRequest,
  WatchWithFriendsStateRequest,
  SystemDiagnostics,
  SystemIdentity,
  SystemReleaseInfo,
  SystemStatusResponse,
  SystemStorageCleanupResponse,
  StartupDiagnostics,
  StoragePathsResponse,
  SuccessResponse,
  SystemStorageReport,
  SystemTimeSync,
  TranscodeCapacityReport,
  UpdateMediaRequest,
  User,
  UserCreateRequest,
  UserPatchRequest,
  UserPreferences,
} from "./types.js";
import type {
  OfflineDownloadAuthorizationRevalidationRequest,
  OfflineDownloadAuthorizationRevalidationResponse
} from "./offlineDownloadAuthorization.js";
import type {
  ApiOperationJSONBody,
  ApiOperationPath,
  ApiOperationQuery,
} from "./apiTypes.js";
import { genericPlaybackClientProfile } from "./playbackProfiles.js";
import { normalizePlaybackResponse } from "./playback.js";
import {
  parseAutomaticProfileTrust,
  parsePorticoProfile,
  parseProfileDirectory,
  parseProfileSelectionGrant,
} from "./profiles.js";
import {
  normalizeNotificationReceiptMutation,
  normalizeViewerFeedbackAdminUpdate,
  normalizeViewerFeedbackSubmission,
  parseNotificationPage,
  parseNotificationInvalidation,
  parseNotificationReadAllResult,
  parseNotificationReceiptResult,
  parseOwnerNotificationRecipientDirectory,
  parseViewerFeedbackCapabilities,
  parseViewerFeedbackPage,
  parseViewerFeedbackReceipt,
  parseViewerFeedbackRecord,
  parseViewerNotification,
} from "./notifications.js";
import type {
  NotificationInvalidation,
  NotificationRecipient,
} from "./notifications.js";
import type {
  LibraryChannelAggregate,
  AdminLibraryChannelListResponse,
  LibraryChannelApplicabilityResponse,
  LibraryChannelConfigurationRequest,
  LibraryChannelGeneration,
  LibraryChannelGuide,
  LibraryChannelsGuide,
  LibraryChannelGuideParams,
  LibraryChannelHealthResponse,
  LibraryChannelListResponse,
  LibraryChannelListParams,
  LibraryChannelLogoAsset,
  LibraryChannelReorderRequest,
  LibraryChannelRestoreDefaultsRequest,
  LibraryChannelRestoreDefaultsResponse,
  LibraryChannelTemplatesResponse,
  LibraryChannelTuneRequest,
  LibraryChannelTuneResponse,
} from "./libraryChannels.js";
import {
  assertProductContractCompatibility,
  assertServerAPICompatibility,
  assertHostedServicesCompatibility,
  evaluatePorticoCompatibility,
} from "./compatibility.js";
import {
  PorticoEventProtocolError,
  PorticoEventSubscriptionCoordinator,
  runPorticoEventSubscription,
  type PorticoAbortSignal,
  type PorticoEventSubscriptionOptions,
  type PorticoEventSubscriptionRuntime,
  type PorticoLongPollRequest,
} from "./eventSubscriptions.js";
import {
  PorticoSSEDecoder,
  PorticoSSEProtocolError,
  assertPorticoEventStreamContentType,
  dispatchPorticoJSONEvent,
} from "./internal/sseDecoder.js";
import { decodeHighRiskResponse } from "./internal/highRiskResponses.js";
import { getOperationPolicy, type OperationClass } from "./operationPolicy.js";
import { positiveFullJitterDelay } from "./retryScheduling.js";

export type DataTag = string;
export type ValueProvider<T> = T | (() => T);

export type ApiRequestInit = Omit<RequestInit, "body" | "credentials"> & {
  body?: unknown;
  /** Maximum time this logical request may occupy the transport. */
  timeoutMs?: number;
  /** Optional absolute Unix-millisecond deadline for this logical request. */
  deadlineAt?: number;
  /** Maximum automatic retries permitted by the selected operation policy. */
  retryBudget?: number;
  /** Optional semantic transport class; otherwise the HTTP method selects one. */
  operationClass?: OperationClass;
  authorization?:
    | { mode: "viewer" }
    | { mode: "playback-continuation"; token: string; origin: string }
    | { mode: "cast-receiver"; token: string; origin: string }
    | { mode: "download-grant"; token: string }
    | { mode: "profile-admin-proof"; proof: string }
    | { mode: "anonymous" };
};

export interface ApiErrorMetadata {
  type?: string;
  title?: string;
  detail?: string;
  messageId?: string;
  requestId?: string;
  responseHeaders?: Readonly<Record<string, string>>;
  retryAfter?: string;
  retryAt?: string;
  retryAfterMs?: number;
  /** The operation may be retried by a caller without changing its meaning. */
  retryable?: boolean;
  /** The request may have reached the server without a known response. */
  ambiguous?: boolean;
}

export class ApiError extends Error {
  readonly code: string;
  readonly status: number;
  readonly type?: string;
  readonly title?: string;
  readonly detail: string;
  readonly messageId?: string;
  readonly details?: Record<string, unknown>;
  readonly requestId?: string;
  readonly responseHeaders: Readonly<Record<string, string>>;
  readonly retryAfter?: string;
  readonly retryAt?: string;
  readonly retryAfterMs?: number;
  readonly retryable: boolean;
  readonly ambiguous: boolean;

  constructor(
    status: number,
    code: string,
    message: string,
    details?: Record<string, unknown>,
    metadata: ApiErrorMetadata = {},
  ) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
    this.details = details;
    this.type = metadata.type;
    this.title = metadata.title;
    this.detail = metadata.detail ?? message;
    this.messageId = metadata.messageId;
    this.requestId = metadata.requestId;
    this.responseHeaders = metadata.responseHeaders ?? {};
    this.retryAfter = metadata.retryAfter;
    this.retryAt = metadata.retryAt;
    this.retryAfterMs = metadata.retryAfterMs;
    this.retryable = metadata.retryable ?? false;
    this.ambiguous = metadata.ambiguous ?? false;
  }
}

/**
 * A transport failure is deliberately distinct from an HTTP problem. A
 * response was not available, so a non-idempotent operation may already have
 * committed and must be reconciled rather than blindly replayed.
 */
export class PorticoTransportError extends ApiError {
  readonly cause: unknown;
  readonly method: string;
  readonly phase: "request" | "response" | "timeout";

  constructor(
    code: string,
    message: string,
    cause: unknown,
    metadata: ApiErrorMetadata & {
      method?: string;
      phase?: "request" | "response" | "timeout";
    } = {},
  ) {
    super(0, code, message, undefined, metadata);
    this.name = "PorticoTransportError";
    this.cause = cause;
    this.method = metadata.method ?? "GET";
    this.phase = metadata.phase ?? "request";
  }
}

export interface PendingHostedTerminalMutation {
  version: "v1";
  idempotencyKey: string;
  method: string;
  path: string;
  createdAt: string;
}

export interface HostedTerminalMutationDurabilityAdapter {
  load?(): Promise<readonly PendingHostedTerminalMutation[]>;
  save(record: PendingHostedTerminalMutation): Promise<void>;
  remove(idempotencyKey: string): Promise<void>;
}

/** A lost terminal-mutation response was reconciled as committed. */
export class HostedTerminalMutationCommittedError extends ApiError {
  readonly idempotencyKey: string;
  readonly receipt: Readonly<Record<string, unknown>>;
  readonly committed = true;

  constructor(
    idempotencyKey: string,
    receipt: Readonly<Record<string, unknown>>,
  ) {
    super(
      200,
      "terminal_mutation_committed",
      "Portico confirmed that your change was saved.",
      { receipt },
    );
    this.name = "HostedTerminalMutationCommittedError";
    this.idempotencyKey = idempotencyKey;
    this.receipt = receipt;
  }
}

/** The mutation may have committed; retain the key and reconcile, never replay blindly. */
export class HostedTerminalMutationUncertainError extends ApiError {
  readonly cause: unknown;
  readonly idempotencyKey: string;
  readonly method: string;
  readonly path: string;

  constructor(
    cause: unknown,
    record: PendingHostedTerminalMutation,
  ) {
    super(
      cause instanceof ApiError ? cause.status : 0,
      "terminal_mutation_outcome_unknown",
      "Portico is still confirming this change.",
      undefined,
      {
        ambiguous: true,
        retryable: false,
        requestId: cause instanceof ApiError ? cause.requestId : undefined,
        responseHeaders:
          cause instanceof ApiError ? cause.responseHeaders : undefined,
        retryAfter: cause instanceof ApiError ? cause.retryAfter : undefined,
        retryAt: cause instanceof ApiError ? cause.retryAt : undefined,
        retryAfterMs:
          cause instanceof ApiError ? cause.retryAfterMs : undefined,
      },
    );
    this.name = "HostedTerminalMutationUncertainError";
    this.cause = cause;
    this.idempotencyKey = record.idempotencyKey;
    this.method = record.method;
    this.path = record.path;
  }
}

export function isRetryablePorticoError(error: unknown): boolean {
  if (error instanceof ApiError) return error.retryable;
  return Boolean(
    error &&
    typeof error === "object" &&
    (error as { retryable?: unknown }).retryable === true,
  );
}

export function isAmbiguousPorticoError(error: unknown): boolean {
  if (error instanceof ApiError) return error.ambiguous;
  return Boolean(
    error &&
    typeof error === "object" &&
    (error as { ambiguous?: unknown }).ambiguous === true,
  );
}

function terminalMutationRejectionIsDefinitive(error: unknown): boolean {
  return (
    error instanceof ApiError &&
    !error.ambiguous &&
    !error.retryable &&
    error.status !== 401 &&
    error.status !== 404 &&
    error.code !== "playback_terminal_request_conflict" &&
    error.code !== "playback_stopping" &&
    error.code !== "handoff_in_progress" &&
    error.code !== "prepared_handoff_in_progress" &&
    error.status >= 400 &&
    error.status < 500 &&
    error.status !== 408
  );
}

/**
 * Raised when credential publication fails and one or more compensating
 * credential deletions also fail. Callers must treat this as a security latch:
 * a credential copy may remain usable or restorable until platform cleanup is
 * independently verified.
 */
export class CredentialCleanupUncertainError extends ApiError {
  readonly cause: unknown;
  readonly rollbackFailures: readonly unknown[];
  readonly failClosed = true;

  constructor(
    message: string,
    cause: unknown,
    rollbackFailures: readonly unknown[],
  ) {
    super(0, "credential_cleanup_uncertain", message, {
      cause: cause instanceof Error ? cause.message : String(cause),
      cleanupFailureCount: rollbackFailures.length,
    });
    this.name = "CredentialCleanupUncertainError";
    this.cause = cause;
    this.rollbackFailures = rollbackFailures;
  }
}

export interface LocalServerSession {
  serverId?: string;
  serverName?: string;
  apiBaseUrl?: string;
  bootstrapAccessToken?: string;
  refreshToken?: string;
  accessToken?: string;
  /** Canonical unpadded standard-base64 raw Ed25519 Server identity key. */
  serverPublicKey?: string;
  serverPublicKeyFingerprint?: string;
  expiresAt?: string;
  refreshExpiresAt?: string;
  routeType?: string;
  routeAddress?: string;
  authority?: "local" | "hosted";
  accountId?: string;
  profileId?: string;
  installationId?: string;
  authorizationRevision?: string;
}

export type PorticoServerSessionExchangeRequest = PorticoSessionAttachPayload;

export interface SessionStore {
  get(): LocalServerSession | undefined;
  set?(session: LocalServerSession): void;
  clear?(): void;
}

/**
 * Platform-owned durable credential persistence.
 *
 * Client Core deliberately does not assume Keychain, Keystore, localStorage,
 * or any other platform storage. Native and web shells can provide the
 * appropriate implementation here. Calls are awaited so refresh-token
 * rotation is durable before a retried request is released.
 */
export interface CredentialAdapter {
  load?(): Promise<LocalServerSession | undefined>;
  save(session: LocalServerSession): Promise<void>;
  clear(): Promise<void>;
  loadPendingRotation?(): Promise<PendingCredentialRotation | undefined>;
  savePendingRotation?(pending: PendingCredentialRotation): Promise<void>;
  clearPendingRotation?(): Promise<void>;
}

export interface PendingCredentialRotation {
  version: "v1";
  oldRefreshToken: string;
  rotationKey: string;
  createdAt: string;
}

export interface FetchTransport {
  fetch(input: string | URL, init?: RequestInit): Promise<Response>;
}

export type EventStreamChunk = string | Uint8Array;

/**
 * Platform stream bridge. Browser fetch uses ReadableStream and TextDecoder by
 * default; React Native shells can inject an async chunk source and UTF-8
 * decoder without installing browser globals.
 */
export interface EventStreamAdapter {
  read(
    response: Response,
    signal: AbortSignal,
  ): AsyncIterable<EventStreamChunk>;
  decode?(chunk: Uint8Array, options: { stream: boolean }): string;
  flush?(): string;
}

export interface DurablePlaybackEventRecord {
  version: "v1";
  kind?: "progress";
  /** Opaque principal/server/session/generation identity created by Client Core. */
  key: string;
  /** Exact uncertain event followed by at most one coalesced successor. */
  events: readonly PlaybackProgressEvent[];
  updatedAt: string;
}

/**
 * Complete durable playback authority partition: server, authority, account,
 * profile, authorization revision.
 */
export type PlaybackAuthorityScope = readonly [
  string,
  string,
  string,
  string,
  string,
];

export interface CommittedPlaybackReplacementOutcome {
  outcome: "committed-restore-required";
  sourceSessionId: string;
  replacementSessionId: string;
}

export type DurablePlaybackTerminalRecord =
  | {
      version: "v2";
      kind: "terminal";
      key: string;
      scope: PlaybackAuthorityScope;
      sessionId: string;
      operation: "stop";
      request: PlaybackSessionStopRequest;
      updatedAt: string;
    }
  | {
      version: "v2";
      kind: "terminal";
      key: string;
      scope: PlaybackAuthorityScope;
      sessionId: string;
      operation: "handoff";
      request: PlaybackHandoffRequest;
      updatedAt: string;
    }
  | {
      version: "v2";
      kind: "terminal";
      key: string;
      scope: PlaybackAuthorityScope;
      sessionId: string;
      operation: "replacement";
      request: PlaybackReplacementRequest;
      target: DurablePlaybackReplacementTarget;
      /** Durable server-committed identity retained until exact active restore. */
      committedOutcome?: CommittedPlaybackReplacementOutcome;
      updatedAt: string;
    }
  | {
      version: "v2";
      kind: "terminal";
      key: string;
      scope: PlaybackAuthorityScope;
      sessionId: string;
      operation: "live-tv-close";
      channelId: string;
      request: LiveTvStreamCloseRequest;
      updatedAt: string;
    };

export interface DurableCastTransferRecord {
  version: "v1";
  kind: "cast-transfer";
  key: string;
  scope: PlaybackAuthorityScope;
  /** Exact immutable bootstrap target and optional source terminal envelope. */
  request: CastBootstrapRequest;
  updatedAt: string;
}

export type DurablePlaybackProgressRecord =
  | DurablePlaybackEventRecord
  | DurablePlaybackTerminalRecord
  | DurableCastTransferRecord;

export type PendingPlaybackTerminalMutation =
  | { operation: "stop"; request: PlaybackSessionStopRequest }
  | { operation: "handoff"; request: PlaybackHandoffRequest }
  | {
      operation: "live-tv-close";
      channelId: string;
      request: LiveTvStreamCloseRequest;
    }
  | {
      operation: "replacement";
      request: PlaybackReplacementRequest;
      target: DurablePlaybackReplacementTarget;
      committedOutcome?: CommittedPlaybackReplacementOutcome;
    };

export type PendingPlaybackTerminalMutationRecord =
  PendingPlaybackTerminalMutation & { sessionId: string };

export type PlaybackReplacementInput =
  | (Omit<
      PlaybackReplacementRequest,
      "requestId" | "previousTerminal"
    > & {
      /** Core allocates the immutable request identity and terminal ordering. */
      requestId?: never;
      previousTerminal: PlaybackTerminalInput;
    })
  | PlaybackReplacementRequest;

export interface MediaPlaybackStartOptions {
  intent?: PlaybackIntent;
  versionId?: string;
  skipPreroll?: boolean;
  burnInSubtitleId?: string;
  subtitleStreamId?: string;
  audioStreamId?: string;
  startSeconds?: number;
  queueMediaIds?: string[];
  repeatMode?: PlaybackRepeatMode;
  sourceContext?: PlaybackSourceContext;
}

export interface LiveTvPlaybackStartOptions {
  clientProfile?: PlaybackClientProfile;
  intent?: PlaybackIntent;
}

export type DvrPlaybackStartOptions = Omit<
  DVRPlaybackSessionCreateRequest,
  "intent" | "replacement"
> & {
  intent?: PlaybackIntent;
};

export type LibraryChannelPlaybackStartOptions = Omit<
  LibraryChannelTuneRequest,
  "clientInstanceId" | "intent" | "replacement"
> & {
  clientInstanceId?: string;
  intent?: PlaybackIntent;
};

export type PlaybackReplacementTarget =
  | {
      kind: "media";
      mediaId: string;
      playbackOptions?: MediaPlaybackStartOptions;
    }
  | {
      kind: "live-tv";
      channelId: string;
      playbackOptions?: LiveTvPlaybackStartOptions;
    }
  | {
      kind: "live-tv-stream";
      channelId: string;
      playbackOptions?: LiveTvPlaybackStartOptions;
    }
  | {
      kind: "dvr";
      recordingId: string;
      playbackOptions?: DvrPlaybackStartOptions;
    }
  | {
      kind: "library-channel";
      channelId: string;
      playbackOptions?: LibraryChannelPlaybackStartOptions;
    };

export type PlaybackReplacementTargetResponse<
  Target extends PlaybackReplacementTarget,
> = Target extends { kind: "library-channel" }
  ? LibraryChannelTuneResponse
  : PlaybackResponse;

export interface PlaybackReplacementRejection {
  status: number;
  code: string;
  detail: string;
}

export type PlaybackReplacementOutcome<Value> =
  | { outcome: "accepted"; value: Value }
  | {
      outcome: "source-inactive";
      sourceSessionId: string;
      rejection: PlaybackReplacementRejection;
    }
  | {
      outcome: "source-retained";
      sourceSessionId: string;
      rejection: PlaybackReplacementRejection;
    }
  | {
      outcome: "source-closed";
      sourceSessionId: string;
      rejection: PlaybackReplacementRejection;
      terminal: PlaybackSessionTerminalAcknowledgement;
    }
  | CommittedPlaybackReplacementOutcome;

export type PendingPlaybackTerminalRetryValue =
  | PlaybackSessionTerminalAcknowledgement
  | PlaybackResponse
  | LibraryChannelTuneResponse;

export type PendingPlaybackTerminalRetryOutcome =
  PlaybackReplacementOutcome<PendingPlaybackTerminalRetryValue>;

export interface CastReceiverCredential {
  token: string;
  origin: string;
}

export type CastBootstrapInput = Omit<
  CastBootstrapRequest,
  "clientInstanceId" | "requestId" | "replacement"
> & {
  clientInstanceId?: string;
  /** Optional caller-owned identity; Core allocates one when omitted. */
  requestId?: string;
};

export type CastTransferOutcome =
  | {
      outcome: "pending";
      value: CastBootstrapResponse | CastTransferStatusResponse;
    }
  | { outcome: "accepted"; value: CastTransferStatusResponse }
  | {
      outcome: "source-inactive";
      sourceSessionId: string;
      rejection: PlaybackReplacementRejection;
    }
  | {
      outcome: "source-retained";
      sourceSessionId: string;
      value?: CastTransferStatusResponse;
      rejection?: PlaybackReplacementRejection;
    }
  | { outcome: "not-committed"; value: CastTransferStatusResponse };

export interface PendingCastTransfer {
  requestId: string;
  sourceSessionId?: string;
  replacementSessionId?: string;
}

export interface DurablePlaybackReplacementTarget {
  kind: PlaybackReplacementTarget["kind"];
  /** Exact opaque route resource ID; omitted only for the media start route. */
  resourceId?: string;
  /** Exact immutable target request body, including the replacement envelope. */
  body: Readonly<Record<string, unknown>>;
}

export interface PlaybackProgressDurabilityAdapter {
  load(): Promise<readonly DurablePlaybackProgressRecord[]>;
  save(record: DurablePlaybackProgressRecord): Promise<void>;
  remove(key: string): Promise<void>;
}

export interface PorticoClientOptions {
  apiBaseUrl?: ValueProvider<string>;
  /** Long-lived Server API key. Mutually exclusive with refreshable session storage. */
  apiKey?: ValueProvider<string>;
  /** Hosted Services origin used only to rotate server-scoped Portico credentials. */
  hostedApiBaseUrl?: ValueProvider<string>;
  baseHref?: ValueProvider<string>;
  sessionStore?: SessionStore;
  credentialAdapter?: CredentialAdapter;
  /** Durable outbox for exact ordered terminal/progress recovery after process death. */
  playbackProgressDurabilityAdapter?: PlaybackProgressDurabilityAdapter;
  transport?: FetchTransport;
  csrfToken?: ValueProvider<string>;
  playbackClientProfile?: () => PlaybackClientProfile;
  playbackClientInstanceId?: () => string;
  onMutation?: (tags: DataTag[], path: string) => void;
  now?: () => number;
  requestId?: () => string;
  /** Default deadline for control-plane requests, including response reads. */
  requestTimeoutMs?: number;
  /** Default retry count for safe Hosted reads. Mutations are never retried. */
  retryBudget?: number;
  /** Optional delays for safe Hosted-read retries. */
  retryDelaysMs?: readonly number[];
  /** Persisted installation cohort used only to spread control-plane retries. */
  retryCohort?: ValueProvider<string>;
  eventStream?: EventStreamAdapter;
  /** Optional deterministic scheduling hooks for event transport runtimes and tests. */
  eventSubscriptionRuntime?: Partial<PorticoEventSubscriptionRuntime>;
}

type PlaybackHistoryFilterParams = {
  userId?: string;
  libraryId?: string;
  type?: string;
  query?: string;
  period?: string;
};

type PlaybackHistoryParams = PlaybackHistoryFilterParams & {
  limit?: number;
  cursor?: string;
  count?: "none" | "exact";
};

export interface HostedServicesClientOptions {
  hostedApiBaseUrl?: ValueProvider<string>;
  baseHref?: ValueProvider<string>;
  transport?: FetchTransport;
  csrfToken?: ValueProvider<string>;
  /** Receives API-origin CSRF bootstrap tokens exposed only to trusted origins. */
  onCSRFToken?: (token: string) => void;
  accessToken?: ValueProvider<string>;
  onMutation?: (tags: DataTag[], path: string) => void;
  requestId?: () => string;
  /** Default deadline for control-plane requests, including response reads. */
  requestTimeoutMs?: number;
  /** Default retry count for safe Hosted reads. Mutations are never retried. */
  retryBudget?: number;
  /** Optional delays for safe Hosted-read retries. */
  retryDelaysMs?: readonly number[];
  /** Persisted installation cohort used only to spread control-plane retries. */
  retryCohort?: ValueProvider<string>;
  /** Durable key ledger for terminal changes whose HTTP response may be lost. */
  terminalMutationDurabilityAdapter?: HostedTerminalMutationDurabilityAdapter;
}

export interface BitrateTestResult {
  bytes: number;
  durationMs: number;
  mbps: number;
}

export interface RequestSignal {
  signal?: AbortSignal | null;
  keepalive?: boolean;
  timeoutMs?: number;
  deadlineAt?: number;
  retryBudget?: number;
  operationClass?: OperationClass;
}

type TransportConfig = Pick<
  PorticoClientOptions,
  "requestTimeoutMs" | "retryBudget" | "retryDelaysMs" | "retryCohort"
>;
type TransportRequestOptions = RequestSignal & { method?: string };

function requestTransportOptions(options: ApiRequestInit): RequestSignal {
  return {
    signal: options.signal ?? undefined,
    timeoutMs: options.timeoutMs,
    deadlineAt: options.deadlineAt,
    retryBudget: options.retryBudget,
    operationClass: options.operationClass,
  };
}

const MAX_REQUEST_TIMEOUT_MS = 5 * 60_000;
const DEFAULT_READ_RETRY_DELAYS_MS = [150, 500] as const;

interface TransportRequestContext {
  readonly controller: AbortController;
  readonly signal: AbortSignal;
  readonly method: string;
  readonly deadlineAt?: number;
  readonly operationClass: OperationClass;
  dispatched: boolean;
  timedOut: boolean;
  timer?: ReturnType<typeof setTimeout>;
  upstreamSignal?: AbortSignal;
  upstreamAbort?: () => void;
}

function isSafeTransportMethod(method: string): boolean {
  return ["GET", "HEAD", "OPTIONS"].includes(method.toUpperCase());
}

function operationClassFor(
  method: string,
  requested?: OperationClass,
): OperationClass {
  // Only the HTTP method has a safe generic default. Polling, discovery, and
  // stream semantics are caller-selected because a route name cannot prove
  // its retry or timeout contract.
  return (
    requested ??
    (isSafeTransportMethod(method)
      ? "interactive read"
      : "interactive mutation")
  );
}

function operationPolicyFor(
  options: TransportRequestOptions,
): ReturnType<typeof getOperationPolicy> {
  return getOperationPolicy(
    operationClassFor(options.method ?? "GET", options.operationClass),
  );
}

function boundedTransportTimeout(
  value: number | undefined,
  fallback: number,
): number {
  const timeout = value ?? fallback;
  if (
    !Number.isSafeInteger(timeout) ||
    timeout < 1 ||
    timeout > MAX_REQUEST_TIMEOUT_MS
  ) {
    throw new TypeError(
      `Portico request timeout must be an integer between 1 and ${MAX_REQUEST_TIMEOUT_MS} milliseconds.`,
    );
  }
  return timeout;
}

function boundedRetryBudget(
  value: number | undefined,
  policy: ReturnType<typeof getOperationPolicy>,
): number {
  if (!policy.retry.eligible) return 0;
  const budget = value ?? policy.retry.defaultBudget;
  if (
    !Number.isSafeInteger(budget) ||
    budget < 0 ||
    budget > policy.retry.ceiling
  ) {
    throw new TypeError(
      `Portico ${policy.operationClass} retry budget must be an integer between 0 and ${policy.retry.ceiling}.`,
    );
  }
  return budget;
}

function retryDelays(
  config: TransportConfig,
  budget: number,
  policy: ReturnType<typeof getOperationPolicy>,
): readonly number[] {
  if (!policy.retry.eligible || budget === 0) return [];
  const delays = config.retryDelaysMs ?? DEFAULT_READ_RETRY_DELAYS_MS;
  if (delays.length > policy.retry.ceiling)
    throw new TypeError("Portico retry delay budget is too large.");
  for (const delay of delays) {
    if (
      !Number.isSafeInteger(delay) ||
      delay < 0 ||
      delay > MAX_REQUEST_TIMEOUT_MS
    ) {
      throw new TypeError(
        "Portico retry delays must be bounded non-negative integers.",
      );
    }
  }
  return delays.slice(0, budget);
}

function transportDeadlineAt(
  config: TransportConfig,
  options: TransportRequestOptions,
): number | undefined {
  const policy = operationPolicyFor(options);
  const configuredTimeout = options.timeoutMs ?? config.requestTimeoutMs;
  if (configuredTimeout === undefined && policy.defaultDeadlineMs === null) {
    if (options.deadlineAt === undefined) return undefined;
    if (!Number.isFinite(options.deadlineAt))
      throw new TypeError("Portico request deadline must be finite.");
    return options.deadlineAt;
  }
  const timeoutMs = boundedTransportTimeout(
    configuredTimeout,
    policy.defaultDeadlineMs ?? MAX_REQUEST_TIMEOUT_MS,
  );
  const calculated = Date.now() + timeoutMs;
  if (options.deadlineAt === undefined) return calculated;
  if (!Number.isFinite(options.deadlineAt))
    throw new TypeError("Portico request deadline must be finite.");
  return Math.min(calculated, options.deadlineAt);
}

function createTransportRequestContext(
  config: TransportConfig,
  options: TransportRequestOptions,
  method: string,
  upstreamSignal?: AbortSignal,
  deadlineAt = transportDeadlineAt(config, options),
): TransportRequestContext {
  const controller = new AbortController();
  const preserveUpstreamSignal =
    Boolean(upstreamSignal) && deadlineAt === undefined;
  const context: TransportRequestContext = {
    controller,
    signal: preserveUpstreamSignal ? upstreamSignal! : controller.signal,
    method: method.toUpperCase(),
    operationClass: operationClassFor(method, options.operationClass),
    deadlineAt,
    dispatched: false,
    timedOut: false,
  };
  const abortFromUpstream = () => controller.abort();
  if (upstreamSignal) {
    context.upstreamSignal = upstreamSignal;
    context.upstreamAbort = abortFromUpstream;
    if (upstreamSignal.aborted) controller.abort();
    else
      upstreamSignal.addEventListener("abort", abortFromUpstream, {
        once: true,
      });
  }
  if (deadlineAt !== undefined) {
    const remainingMs = Math.max(0, deadlineAt - Date.now());
    context.timer = setTimeout(() => {
      context.timedOut = true;
      controller.abort();
    }, remainingMs);
  }
  return context;
}

function disposeTransportRequestContext(
  context: TransportRequestContext,
): void {
  if (context.timer !== undefined) clearTimeout(context.timer);
  if (context.upstreamSignal && context.upstreamAbort) {
    context.upstreamSignal.removeEventListener("abort", context.upstreamAbort);
  }
}

function transportTimeoutError(
  context: TransportRequestContext,
): PorticoTransportError {
  const policy = getOperationPolicy(context.operationClass);
  const ambiguous =
    context.dispatched &&
    policy.idempotencyRequirement === "reconcile-before-retry";
  return new PorticoTransportError(
    policy.timeout.problemCode,
    ambiguous
      ? "The request exceeded its deadline and its server-side outcome is unknown."
      : "The request exceeded its deadline.",
    undefined,
    {
      method: context.method,
      phase: "timeout",
      retryable: !ambiguous,
      ambiguous,
      messageId: policy.timeout.messageId,
    },
  );
}

function transportFailureError(
  error: unknown,
  context: TransportRequestContext,
): PorticoTransportError {
  const policy = getOperationPolicy(context.operationClass);
  const ambiguous =
    context.dispatched &&
    policy.idempotencyRequirement === "reconcile-before-retry";
  const detail = error instanceof Error ? error.message : String(error);
  const safeDetail = detail.replace(/[\u0000-\u001F\u007F]/g, "").slice(0, 500);
  return new PorticoTransportError(
    ambiguous ? "transport_ambiguous" : "transport_unavailable",
    safeDetail
      ? `Portico transport failed: ${safeDetail}`
      : "Portico transport failed.",
    error,
    {
      method: context.method,
      phase: "request",
      retryable: !ambiguous,
      ambiguous,
    },
  );
}

async function runTransportOperation<T>(
  config: TransportConfig,
  options: TransportRequestOptions,
  method: string,
  operation: (context: TransportRequestContext) => Promise<T>,
  upstreamSignal?: AbortSignal,
  deadlineAt = transportDeadlineAt(config, options),
): Promise<T> {
  const context = createTransportRequestContext(
    config,
    options,
    method,
    upstreamSignal ?? options.signal ?? undefined,
    deadlineAt,
  );
  return new Promise<T>((resolve, reject) => {
    let settled = false;
    const abortSignals =
      context.signal === context.controller.signal
        ? [context.signal]
        : [context.signal, context.controller.signal];
    const finish = (callback: () => void) => {
      if (settled) return;
      settled = true;
      for (const signal of abortSignals)
        signal.removeEventListener("abort", onAbort);
      disposeTransportRequestContext(context);
      callback();
    };
    const onAbort = () =>
      finish(() =>
        reject(
          context.timedOut ? transportTimeoutError(context) : abortError(),
        ),
      );
    for (const signal of abortSignals)
      signal.addEventListener("abort", onAbort, { once: true });
    if (abortSignals.some((signal) => signal.aborted)) {
      onAbort();
      return;
    }
    let operationResult: Promise<T>;
    try {
      operationResult = operation(context);
    } catch (error) {
      operationResult = Promise.reject(error);
    }
    Promise.resolve(operationResult).then(
      (value) => finish(() => resolve(value)),
      (error) =>
        finish(() => {
          if (context.timedOut) reject(transportTimeoutError(context));
          else if (
            isAbortFailure(error) &&
            (context.signal.aborted || context.controller.signal.aborted)
          )
            reject(abortError());
          else if (error instanceof ApiError) reject(error);
          else reject(transportFailureError(error, context));
        }),
    );
  });
}

function transportFetch(
  transport: FetchTransport | undefined,
  url: string,
  init: RequestInit,
  context: TransportRequestContext,
): Promise<Response> {
  context.dispatched = true;
  return fetcher(transport)(url, { ...init, signal: context.signal });
}

function transportBudgetKey(
  config: TransportConfig,
  options: TransportRequestOptions,
): string {
  if (options.deadlineAt !== undefined) return "no-coalesce-deadline";
  const policy = operationPolicyFor(options);
  const timeout =
    options.timeoutMs ??
    config.requestTimeoutMs ??
    policy.defaultDeadlineMs ??
    "none";
  const budget =
    options.retryBudget ?? config.retryBudget ?? policy.retry.defaultBudget;
  return `${timeout}:${policy.operationClass}:${budget}`;
}

function awaitWithAbort<T>(
  value: PromiseLike<T>,
  signal?: AbortSignal,
): Promise<T> {
  if (!signal) return Promise.resolve(value);
  if (signal.aborted) return Promise.reject(abortError());
  return new Promise<T>((resolve, reject) => {
    let settled = false;
    const cleanup = () => signal.removeEventListener("abort", onAbort);
    const onAbort = () => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(abortError());
    };
    signal.addEventListener("abort", onAbort, { once: true });
    Promise.resolve(value).then(
      (result) => {
        if (settled) return;
        settled = true;
        cleanup();
        resolve(result);
      },
      (error) => {
        if (settled) return;
        settled = true;
        cleanup();
        reject(error);
      },
    );
  });
}

function hostedCursorQuery(params: {
  limit?: number;
  cursor?: string;
  count?: "none" | "exact";
}): string {
  const search = new URLSearchParams();
  if (params.limit) search.set("limit", String(params.limit));
  if (params.cursor) search.set("cursor", params.cursor);
  if (params.count) search.set("count", params.count);
  const query = search.toString();
  return query ? `?${query}` : "";
}

function apiQuery(
  params: Readonly<Record<string, string | number | boolean | undefined>>,
): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(params)) {
    if (value !== undefined) search.set(key, String(value));
  }
  const query = search.toString();
  return query ? `?${query}` : "";
}

function normalizeManagedProfileDirectory(
  response: ServerManagedProfileDirectory,
): ServerManagedProfileDirectory {
  if (typeof response.canManage !== "boolean")
    throw new TypeError("managed profile directory canManage is invalid");
  const directory = parseProfileDirectory({
    authority: response.authority,
    accountId: response.accountId,
    serverId: response.serverId,
    profilesAllowed: response.profilesAllowed,
    profiles: response.profiles,
  });
  return { ...directory, canManage: response.canManage };
}

export interface SavedMediaBrowseParams {
  limit?: number;
  cursor?: string;
  filter?: "all" | "unwatched" | "inProgress";
  sort?: "updated" | "title" | "year" | "duration" | "progress";
  order?: "asc" | "desc";
}

export interface SavedResourceBrowseParams {
  cursor?: string;
  limit?: number;
  libraryId?: string;
}

export interface SavedResourceItemsParams {
  cursor?: string;
  limit?: number;
}

export interface SavedShareCandidateParams {
  query?: string;
  limit?: number;
}

interface InFlightJSONRequest {
  controller: AbortController;
  consumers: number;
  completed: boolean;
  promise: Promise<unknown>;
}

interface PlaybackProgressDelivery {
  event: PlaybackProgressEvent;
  send: (
    event: PlaybackProgressEvent,
  ) => Promise<PlaybackProgressAcknowledgement>;
  waiters: Array<{
    resolve: (acknowledgement: PlaybackProgressAcknowledgement) => void;
    reject: (reason?: unknown) => void;
  }>;
}

interface PlaybackProgressMailbox {
  inFlight: PlaybackProgressDelivery;
  successor?: PlaybackProgressDelivery;
  delivering: boolean;
  touchedAt: number;
}

const MAX_PLAYBACK_PROGRESS_MAILBOXES = 128;
const MAX_PLAYBACK_PROGRESS_SEQUENCES = 256;
const MAX_PLAYBACK_TERMINAL_REQUESTS = 128;

function deepFreezeJSON<T>(value: T): T {
  if (value && typeof value === "object" && !Object.isFrozen(value)) {
    for (const child of Object.values(value as Record<string, unknown>)) {
      deepFreezeJSON(child);
    }
    Object.freeze(value);
  }
  return value;
}

function immutableJSONSnapshot<T>(value: T): T {
  return deepFreezeJSON(JSON.parse(JSON.stringify(value)) as T);
}

function normalizedPlaybackProgressInput(
  event: PlaybackProgressInput,
): PlaybackProgressInput {
  if ("completed" in event) {
    throw new TypeError(
      "Playback completion requires the atomic stop operation.",
    );
  }
  const normalized = { ...event };
  if (normalized.progressSeconds !== undefined) {
    if (!Number.isFinite(normalized.progressSeconds))
      throw new TypeError("Playback progress must be finite.");
    normalized.progressSeconds = Math.max(
      0,
      Math.round(normalized.progressSeconds),
    );
  }
  if (normalized.durationSeconds !== undefined) {
    if (!Number.isFinite(normalized.durationSeconds))
      throw new TypeError("Playback duration must be finite.");
    normalized.durationSeconds = Math.max(
      0,
      Math.round(normalized.durationSeconds),
    );
  }
  if (normalized.positionSeconds !== undefined) {
    if (!Number.isFinite(normalized.positionSeconds))
      throw new TypeError("Playback position must be finite.");
    normalized.positionSeconds = Math.max(0, normalized.positionSeconds);
  }
  if (normalized.eventSequence !== undefined) {
    if (!Number.isFinite(normalized.eventSequence))
      throw new TypeError("Playback event sequence must be finite.");
    normalized.eventSequence = Math.max(
      0,
      Math.trunc(normalized.eventSequence),
    );
  }
  return normalized;
}

function assertPlaybackTerminalEvent(
  body: PlaybackTerminalInput | PlaybackTerminalEvent,
): void {
  if (!body || typeof body !== "object") {
    throw new TypeError("Playback terminal event must be an object.");
  }
  if (body.disposition !== "stopped" && body.disposition !== "completed") {
    throw new TypeError("Playback terminal disposition is invalid.");
  }
  if (!Number.isFinite(body.positionSeconds) || body.positionSeconds < 0) {
    throw new TypeError("Playback terminal position must be a finite non-negative number.");
  }
  if (
    !Number.isFinite(body.durationSeconds) ||
    body.durationSeconds < 0 ||
    (body.disposition === "completed" && body.durationSeconds <= 0)
  ) {
    throw new TypeError("Playback terminal duration is invalid for its disposition.");
  }
  if (
    body.disposition === "completed" &&
    body.positionSeconds !== body.durationSeconds
  ) {
    throw new TypeError(
      "Completed playback terminal position must equal its duration.",
    );
  }
  const orderedAuthorityFields = [
    "generation" in body,
    "eventSequence" in body,
    "recordedAt" in body,
  ];
  const orderedAuthorityFieldCount = orderedAuthorityFields.filter(Boolean).length;
  if (
    orderedAuthorityFieldCount !== 0 &&
    orderedAuthorityFieldCount !== orderedAuthorityFields.length
  ) {
    throw new TypeError(
      "Playback terminal ordering authority must be complete.",
    );
  }
  if (
    "generation" in body &&
    "eventSequence" in body &&
    "recordedAt" in body
  ) {
    if (!Number.isSafeInteger(body.generation) || body.generation <= 0) {
      throw new TypeError("Playback terminal generation must be a positive integer.");
    }
    if (!Number.isSafeInteger(body.eventSequence) || body.eventSequence <= 0) {
      throw new TypeError("Playback terminal event sequence must be a positive integer.");
    }
    if (
      typeof body.recordedAt !== "string" ||
      !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(
        body.recordedAt,
      ) ||
      !Number.isFinite(Date.parse(body.recordedAt))
    ) {
      throw new TypeError("Playback terminal observation time must be RFC 3339.");
    }
  }
}

function isCompletePlaybackTerminalEvent(
  body: PlaybackTerminalEvent | PlaybackTerminalInput,
): body is PlaybackTerminalEvent {
  return (
    "generation" in body &&
    "eventSequence" in body &&
    "recordedAt" in body
  );
}

function assertPlaybackSessionStopRequest(
  body: PlaybackSessionStopRequest,
): void {
  if (!body || typeof body !== "object") {
    throw new TypeError("Playback stop request must be an object.");
  }
  assertPlaybackTerminalRequestId(body.requestId);
  assertPlaybackTerminalEvent(body.terminal);
}

function assertLiveTvStreamCloseRequest(
  body: LiveTvStreamCloseRequest,
): void {
  assertPlaybackSessionStopRequest(body);
  if (
    typeof body.sessionId !== "string" ||
    body.sessionId.length < 1 ||
    body.sessionId.length > 512
  ) {
    throw new TypeError("Live TV close session ID is invalid.");
  }
}

function isPlaybackSessionStopRequest(
  body: PlaybackSessionStopInput,
): body is PlaybackSessionStopRequest {
  return "requestId" in body || "terminal" in body;
}

function assertPlaybackHandoffRequest(body: PlaybackHandoffRequest): void {
  if (!body || typeof body !== "object") {
    throw new TypeError("Playback handoff request must be an object.");
  }
  assertPlaybackTerminalRequestId(body.requestId);
  if (
    typeof body.entryId !== "string" ||
    body.entryId.length < 1 ||
    body.entryId.length > 512
  ) {
    throw new TypeError("Playback handoff entry ID is invalid.");
  }
  assertPlaybackTerminalEvent(body.previousTerminal);
  if (
    body.startSeconds !== undefined &&
    (!Number.isSafeInteger(body.startSeconds) || body.startSeconds < 0)
  ) {
    throw new TypeError(
      "Playback handoff start position must be a non-negative integer.",
    );
  }
  for (const revision of [
    body.expectedQueueRevision,
    body.expectedPlaybackRevision,
  ]) {
    if (
      revision !== undefined &&
      (!Number.isSafeInteger(revision) || revision < 0)
    ) {
      throw new TypeError("Playback handoff revision is invalid.");
    }
  }
}

function assertPlaybackReplacementRequest(
  body: PlaybackReplacementRequest | PlaybackReplacementInput,
): void {
  if (!body || typeof body !== "object") {
    throw new TypeError("Playback replacement request must be an object.");
  }
  if (
    typeof body.sourceSessionId !== "string" ||
    body.sourceSessionId.length < 1 ||
    body.sourceSessionId.length > 512
  ) {
    throw new TypeError("Playback replacement source session ID is invalid.");
  }
  assertPlaybackTerminalEvent(body.previousTerminal);
  const completeTerminal = isCompletePlaybackTerminalEvent(
    body.previousTerminal,
  );
  const hasRequestId = typeof body.requestId === "string";
  if (completeTerminal !== hasRequestId) {
    throw new TypeError(
      "A native playback replacement must provide both its complete terminal event and request identity.",
    );
  }
  if (hasRequestId) assertPlaybackTerminalRequestId(body.requestId!);
  for (const revision of [
    body.expectedQueueRevision,
    body.expectedPlaybackRevision,
  ]) {
    if (
      revision !== undefined &&
      (!Number.isSafeInteger(revision) || revision < 0)
    ) {
      throw new TypeError("Playback replacement revision is invalid.");
    }
  }
}

function assertCastBootstrapRequest(body: CastBootstrapRequest): void {
  if (!isRecord(body)) {
    throw new TypeError("Cast bootstrap request must be an object.");
  }
  assertPlaybackTerminalRequestId(body.requestId);
  if (
    !new Set(["media", "live", "dvr", "library-channel"]).has(
      body.sourceKind,
    ) ||
    typeof body.sourceId !== "string" ||
    body.sourceId.length < 1 ||
    body.sourceId.length > 2_048 ||
    typeof body.clientInstanceId !== "string" ||
    body.clientInstanceId.length < 1 ||
    body.clientInstanceId.length > 512 ||
    !isRecord(body.clientProfile) ||
    typeof body.receiverId !== "string" ||
    body.receiverId.length < 1 ||
    body.receiverId.length > 160 ||
    typeof body.receiverOrigin !== "string" ||
    body.receiverOrigin.length < 1 ||
    body.receiverOrigin.length > 2_048 ||
    typeof body.receiverPublicKey !== "string" ||
    body.receiverPublicKey.length < 16 ||
    body.receiverPublicKey.length > 256 ||
    typeof body.receiverChallenge !== "string" ||
    body.receiverChallenge.length < 16 ||
    body.receiverChallenge.length > 256
  ) {
    throw new TypeError("Cast bootstrap request is invalid.");
  }
  if (body.replacement) {
    assertPlaybackReplacementRequest(body.replacement);
    if (body.replacement.requestId !== body.requestId) {
      throw new TypeError(
        "Cast bootstrap and replacement request identities must match.",
      );
    }
  }
  if (JSON.stringify(body).length > 256 * 1024) {
    throw new TypeError("Cast bootstrap request is too large.");
  }
}

function assertDurablePlaybackReplacementTarget(
  target: DurablePlaybackReplacementTarget,
  request: PlaybackReplacementRequest,
): void {
  if (!target || typeof target !== "object") {
    throw new TypeError("Playback replacement target must be an object.");
  }
  if (
    !new Set<PlaybackReplacementTarget["kind"]>([
      "media",
      "live-tv",
      "live-tv-stream",
      "dvr",
      "library-channel",
    ]).has(target.kind)
  ) {
    throw new TypeError("Playback replacement target kind is invalid.");
  }
  const needsResourceId = target.kind !== "media" && target.kind !== "live-tv";
  if (
    needsResourceId !== Boolean(target.resourceId) ||
    (target.resourceId !== undefined &&
      (typeof target.resourceId !== "string" ||
        target.resourceId.length > 2_048))
  ) {
    throw new TypeError("Playback replacement target identity is invalid.");
  }
  if (!isRecord(target.body)) {
    throw new TypeError("Playback replacement target body is invalid.");
  }
  const serialized = JSON.stringify(target.body);
  if (serialized.length > 256 * 1024) {
    throw new TypeError("Playback replacement target body is too large.");
  }
  if (JSON.stringify(target.body.replacement) !== JSON.stringify(request)) {
    throw new TypeError(
      "Playback replacement target does not contain its exact authority envelope.",
    );
  }
}

function assertPlaybackTerminalRequestId(requestId: string): void {
  if (
    typeof requestId !== "string" ||
    requestId.length < 8 ||
    requestId.length > 128 ||
    !/^[A-Za-z0-9._:-]+$/.test(requestId)
  ) {
    throw new TypeError("Playback terminal request ID is invalid.");
  }
}

function assertPlaybackTerminalAcknowledgement(
  acknowledgement: PlaybackSessionTerminalAcknowledgement,
  sessionId: string,
  request: PlaybackSessionStopRequest,
): void {
  const terminal = acknowledgement.terminal;
  if (
    acknowledgement.accepted !== true ||
    typeof acknowledgement.duplicate !== "boolean" ||
    acknowledgement.requestId !== request.requestId ||
    acknowledgement.sessionId !== sessionId ||
    terminal.disposition !== request.terminal.disposition ||
    terminal.generation !== request.terminal.generation ||
    terminal.eventSequence !== request.terminal.eventSequence ||
    terminal.recordedAt !== request.terminal.recordedAt ||
    terminal.positionSeconds !== request.terminal.positionSeconds ||
    terminal.durationSeconds !== request.terminal.durationSeconds
  ) {
    throw new TypeError(
      "Playback terminal acknowledgement does not match the requested event.",
    );
  }
}

function orderedDurablePlaybackProgressEvent(
  event: PlaybackProgressEvent,
): PlaybackProgressEvent {
  const normalized = normalizedPlaybackProgressInput(event);
  if (
    !Number.isSafeInteger(normalized.eventSequence) ||
    Number(normalized.eventSequence) < 0
  )
    throw new TypeError("Durable playback event sequence is invalid.");
  if (
    typeof normalized.recordedAt !== "string" ||
    !Number.isFinite(Date.parse(normalized.recordedAt))
  )
    throw new TypeError("Durable playback event time is invalid.");
  return normalized as PlaybackProgressEvent;
}

export class PlaybackProgressSequenceCoordinator {
  readonly #sequences = new Map<string, number>();
  constructor(readonly maximumEntries = MAX_PLAYBACK_PROGRESS_SEQUENCES) {
    if (
      !Number.isSafeInteger(maximumEntries) ||
      maximumEntries < 1 ||
      maximumEntries > 4_096
    )
      throw new TypeError("Playback progress sequence capacity is invalid.");
  }
  ordered(
    identity: string,
    event: PlaybackProgressInput,
    now = Date.now(),
  ): PlaybackProgressEvent {
    if (!identity.trim() || identity.length > 4_096)
      throw new TypeError("Playback progress identity is required.");
    if (!Number.isFinite(now))
      throw new TypeError("Playback event time must be finite.");
    const normalized = normalizedPlaybackProgressInput(event);
    const current = this.#sequences.get(identity) ?? 0;
    const eventSequence = normalized.eventSequence ?? current + 1;
    this.#sequences.delete(identity);
    this.#sequences.set(identity, Math.max(current, eventSequence));
    while (this.#sequences.size > this.maximumEntries)
      this.#sequences.delete(this.#sequences.keys().next().value!);
    return {
      ...normalized,
      eventSequence,
      recordedAt: event.recordedAt ?? new Date(now).toISOString(),
    };
  }
  forget(identity: string): void {
    this.#sequences.delete(identity);
  }
  seed(identity: string, nextEventSequence: number): void {
    if (!identity || !Number.isFinite(nextEventSequence)) return;
    const current = this.#sequences.get(identity) ?? 0;
    this.#sequences.delete(identity);
    this.#sequences.set(
      identity,
      Math.max(current, Math.max(0, Math.trunc(nextEventSequence) - 1)),
    );
    while (this.#sequences.size > this.maximumEntries)
      this.#sequences.delete(this.#sequences.keys().next().value!);
  }
}

export function createPlaybackProgressSequenceCoordinator(
  maximumEntries?: number,
): PlaybackProgressSequenceCoordinator {
  return new PlaybackProgressSequenceCoordinator(maximumEntries);
}

export function createMemorySessionStore(
  initial?: LocalServerSession,
): SessionStore {
  let current = initial;
  return {
    get: () => current,
    set: (session) => {
      current = session;
    },
    clear: () => {
      current = undefined;
    },
  };
}

export function createPorticoClient(options: PorticoClientOptions = {}) {
  validateAPIKeyClientOptions(options);
  const credentials = createCredentialLifecycle(options);
  const eventSubscriptions = new PorticoEventSubscriptionCoordinator();
  const inFlightJSONRequests = new Map<string, InFlightJSONRequest>();
  const playbackProgressMailboxes = new Map<string, PlaybackProgressMailbox>();
  const playbackProgressSequences = new Map<string, number>();
  const playbackProgressPersistence = new Map<string, Promise<void>>();
  const supersededPlaybackProgressKeys = new Set<string>();
  const playbackSessionGenerations = new Map<string, number>();
  const playbackSessionNextEventSequences = new Map<string, number>();
  const playbackStoppingSessions = new Set<string>();
  const playbackTerminalRequestsInFlight = new Set<string>();
  const playbackTerminalRequests = new Map<
    string,
    PlaybackSessionStopRequest
  >();
  const playbackHandoffRequests = new Map<string, PlaybackHandoffRequest>();
  const playbackLiveTvCloseRequests = new Map<
    string,
    { channelId: string; request: LiveTvStreamCloseRequest }
  >();
  const playbackReplacementRequests = new Map<
    string,
    {
      request: PlaybackReplacementRequest;
      target: DurablePlaybackReplacementTarget;
      committedOutcome?: CommittedPlaybackReplacementOutcome;
    }
  >();
  const playbackTerminalSessionIds = new Map<string, string>();
  const playbackTerminalScopes = new Map<string, PlaybackAuthorityScope>();
  const castTransferRequests = new Map<string, CastBootstrapRequest>();
  const castTransferScopes = new Map<string, PlaybackAuthorityScope>();
  const durablePlaybackProgress = new Map<
    string,
    DurablePlaybackEventRecord
  >();
  let durablePlaybackProgressLoaded = false;
  let durablePlaybackProgressLoad: Promise<void> | undefined;
  const request = <T>(path: string, init: ApiRequestInit = {}) =>
    localRequest<T>(path, init, options, credentials, inFlightJSONRequests);
  const requestWithoutRefresh = <T>(path: string, init: ApiRequestInit = {}) =>
    localRequest<T>(
      path,
      init,
      options,
      credentials,
      inFlightJSONRequests,
      false,
    );
  const formRequest = <T>(
    path: string,
    form: FormData,
    method = "POST",
    init?: RequestSignal,
  ) => localFormRequest<T>(path, form, method, options, credentials, init);
  const apiUrl = (path: string) => buildApiUrl(path, options);
  const resourceUrl = (path: string) => buildResourceUrl(path, options);
  const encryptedAttachmentRequest = async (
    body: PorticoAttachmentEncryptedRequest,
    requestOptions: RequestSignal = {},
  ) => {
    const transportOptions: TransportRequestOptions = {
      ...requestOptions,
      method: "POST",
    };
    const requestDeadline = transportDeadlineAt(options, transportOptions);
    const session = await runTransportOperation(
      options,
      transportOptions,
      "POST",
      async () => {
        const sessionResult = credentials.current();
        return sessionResult instanceof Promise
          ? await sessionResult
          : sessionResult;
      },
      undefined,
      requestDeadline,
    );
    const path = "/api/auth/portico/sessions";
    const url = buildApiUrl(path, options, session);
    return runTransportOperation(
      options,
      transportOptions,
      "POST",
      async (context) => {
        const response = await transportFetch(
          options.transport,
          url,
          {
            method: "POST",
            credentials: "omit",
            headers: withRequestId(
              { "Content-Type": "application/json" },
              options.requestId,
            ),
            body: JSON.stringify(body),
          },
          context,
        );
        const text = await boundedResponseText(
          response,
          256 * 1024,
          context.signal,
        );
        let payload: PorticoAttachmentEncryptedResponse | undefined;
        try {
          payload = text
            ? (JSON.parse(text) as PorticoAttachmentEncryptedResponse)
            : undefined;
        } catch {
          throw new ApiError(
            response.status,
            "invalid_response",
            "Portico received an invalid encrypted attachment response.",
          );
        }
        if (
          !payload ||
          payload.version !== 1 ||
          payload.handshakeId !== body.handshakeId ||
          !payload.ciphertext
        ) {
          if (!response.ok) {
            let problem: Record<string, unknown> | undefined;
            try {
              const decoded = text ? (JSON.parse(text) as unknown) : undefined;
              if (isRecord(decoded)) problem = decoded;
            } catch {
              // Preserve the HTTP boundary even when a pre-handshake failure is not JSON.
            }
            throw new ApiError(
              response.status,
              stringField(problem, "code") || "attachment_handshake_failed",
              stringField(problem, "message") ||
                "The encrypted attachment request failed.",
            );
          }
          throw new ApiError(
            response.status,
            "invalid_response",
            "Portico received an invalid encrypted attachment response.",
          );
        }
        return {
          status: response.status,
          ok: response.ok,
          payload,
          retryAfter: response.headers.get("Retry-After") ?? undefined,
        };
      },
      undefined,
      requestDeadline,
    );
  };

  const playbackAuthorityScope = (
    session = credentials.peek(),
    serverOrigin = "",
  ): PlaybackAuthorityScope => [
      session?.serverId ||
        trimTrailingSlash(
          serverOrigin ||
            session?.apiBaseUrl ||
            resolveValue(options.apiBaseUrl, ""),
        ),
      session?.authority ?? "local",
      session?.accountId ?? "",
      session?.profileId ?? "",
      session?.authorizationRevision ?? "",
    ];
  const playbackAuthorityScopesEqual = (
    left: PlaybackAuthorityScope,
    right: PlaybackAuthorityScope,
  ) => left.every((part, index) => part === right[index]);
  const validPlaybackAuthorityScope = (
    scope: unknown,
  ): scope is PlaybackAuthorityScope =>
    Array.isArray(scope) &&
    scope.length === 5 &&
    scope.every(
      (part) =>
        typeof part === "string" && part.length <= 2_048,
    );
  const playbackProgressKey = (
    sessionId: string,
    generation: number,
    session = credentials.peek(),
    serverOrigin = "",
  ) =>
    JSON.stringify([
      ...playbackAuthorityScope(session, serverOrigin),
      sessionId,
      Math.max(0, Math.trunc(generation)),
    ]);
  const playbackTerminalKey = (
    operation: "stop" | "handoff" | "replacement" | "live-tv-close",
    sessionId: string,
    requestId: string,
    scope: PlaybackAuthorityScope,
  ) => JSON.stringify([...scope, "terminal", operation, sessionId, requestId]);
  const castTransferKey = (
    requestId: string,
    scope: PlaybackAuthorityScope,
  ) => JSON.stringify([...scope, "cast-transfer", requestId]);
  const playbackSessionFenceKey = (
    sessionId: string,
    scope: PlaybackAuthorityScope,
  ) => JSON.stringify([...scope, "terminal-session", sessionId]);
  const loadDurablePlaybackProgress = async (): Promise<void> => {
    if (
      durablePlaybackProgressLoaded ||
      !options.playbackProgressDurabilityAdapter
    )
      return;
    durablePlaybackProgressLoad ??= options.playbackProgressDurabilityAdapter
      .load()
      .then(async (records) => {
        if (records.length > MAX_PLAYBACK_PROGRESS_MAILBOXES)
          throw new TypeError(
            "The durable playback progress outbox is too large.",
          );
        for (const record of records) {
          if (record.kind === "cast-transfer") {
            if (
              record.version !== "v1" ||
              !record.key ||
              record.key.length > 4_096 ||
              !validPlaybackAuthorityScope(record.scope)
            ) {
              await options.playbackProgressDurabilityAdapter!.remove(record.key);
              continue;
            }
            assertCastBootstrapRequest(record.request);
            if (
              record.key !==
              castTransferKey(record.request.requestId, record.scope)
            ) {
              throw new TypeError(
                "A durable Cast transfer identity is invalid.",
              );
            }
            castTransferRequests.set(
              record.key,
              immutableJSONSnapshot(record.request),
            );
            castTransferScopes.set(record.key, record.scope);
            const sourceSessionId = record.request.replacement?.sourceSessionId;
            if (sourceSessionId) {
              playbackStoppingSessions.add(
                playbackSessionFenceKey(sourceSessionId, record.scope),
              );
            }
            continue;
          }
          if (record.kind === "terminal") {
            if (
              record.version !== "v2" ||
              !record.key ||
              record.key.length > 4_096 ||
              !validPlaybackAuthorityScope(record.scope) ||
              !record.sessionId ||
              record.sessionId.length > 512
            ) {
              // Pre-release v1 terminal records lacked a principal/server
              // partition and cannot be safely attributed after restart.
              await options.playbackProgressDurabilityAdapter!.remove(record.key);
              continue;
            }
            if (record.operation === "stop") {
              assertPlaybackSessionStopRequest(record.request);
              if (
                record.key !==
                playbackTerminalKey(
                  "stop",
                  record.sessionId,
                  record.request.requestId,
                  record.scope,
                )
              ) {
                throw new TypeError(
                  "A durable playback stop identity is invalid.",
                );
              }
              playbackTerminalRequests.set(
                record.key,
                immutableJSONSnapshot(record.request),
              );
            } else if (record.operation === "handoff") {
              assertPlaybackHandoffRequest(record.request);
              if (
                record.key !==
                playbackTerminalKey(
                  "handoff",
                  record.sessionId,
                  record.request.requestId,
                  record.scope,
                )
              ) {
                throw new TypeError(
                  "A durable playback handoff identity is invalid.",
                );
              }
              playbackHandoffRequests.set(
                record.key,
                immutableJSONSnapshot(record.request),
              );
            } else if (record.operation === "live-tv-close") {
              assertLiveTvStreamCloseRequest(record.request);
              if (
                typeof record.channelId !== "string" ||
                record.channelId.length < 1 ||
                record.channelId.length > 2_048 ||
                record.key !==
                  playbackTerminalKey(
                    "live-tv-close",
                    record.sessionId,
                    record.request.requestId,
                    record.scope,
                  ) ||
                record.sessionId !== record.request.sessionId
              ) {
                throw new TypeError(
                  "A durable Live TV close identity is invalid.",
                );
              }
              playbackLiveTvCloseRequests.set(record.key, {
                channelId: record.channelId,
                request: immutableJSONSnapshot(record.request),
              });
            } else {
              assertPlaybackReplacementRequest(record.request);
              assertDurablePlaybackReplacementTarget(
                record.target,
                record.request,
              );
              if (
                record.committedOutcome !== undefined &&
                (record.committedOutcome.outcome !==
                  "committed-restore-required" ||
                  record.committedOutcome.sourceSessionId !== record.sessionId ||
                  typeof record.committedOutcome.replacementSessionId !==
                    "string" ||
                  record.committedOutcome.replacementSessionId.length < 1 ||
                  record.committedOutcome.replacementSessionId.length > 512)
              ) {
                throw new TypeError(
                  "A durable committed playback replacement identity is invalid.",
                );
              }
              if (
                record.key !==
                playbackTerminalKey(
                  "replacement",
                  record.sessionId,
                  record.request.requestId,
                  record.scope,
                ) ||
                record.sessionId !== record.request.sourceSessionId
              ) {
                throw new TypeError(
                  "A durable playback replacement identity is invalid.",
                );
              }
              playbackReplacementRequests.set(
                record.key,
                immutableJSONSnapshot({
                  request: record.request,
                  target: record.target,
                  ...(record.committedOutcome
                    ? { committedOutcome: record.committedOutcome }
                    : {}),
                }),
              );
            }
            playbackTerminalSessionIds.set(record.key, record.sessionId);
            playbackTerminalScopes.set(record.key, record.scope);
            playbackStoppingSessions.add(
              playbackSessionFenceKey(record.sessionId, record.scope),
            );
            continue;
          }
          if (
            record.version !== "v1" ||
            !record.key ||
            record.key.length > 4_096 ||
            record.events.length < 1 ||
            record.events.length > 2
          ) {
            throw new TypeError(
              "A durable playback progress record is invalid.",
            );
          }
          const identity = JSON.parse(record.key) as unknown;
          if (
            Array.isArray(identity) &&
            identity.length === 6 &&
            identity.slice(0, 4).every((part) => typeof part === "string")
          ) {
            // Pre-release progress identities omitted authorizationRevision.
            // Their viewer authority cannot be attributed after restart.
            await options.playbackProgressDurabilityAdapter!.remove(record.key);
            continue;
          }
          if (
            !Array.isArray(identity) ||
            identity.length !== 7 ||
            identity.some((part, index) =>
              index === 6
                ? !Number.isSafeInteger(part) || Number(part) < 0
                : typeof part !== "string" || part.length > 2_048,
            )
          ) {
            throw new TypeError(
              "A durable playback progress identity is invalid.",
            );
          }
          if (record.events.some((event) => "completed" in event)) {
            await options.playbackProgressDurabilityAdapter!.remove(record.key);
            continue;
          }
          const events = record.events.map((event) =>
            orderedDurablePlaybackProgressEvent(event),
          );
          durablePlaybackProgress.set(record.key, { ...record, events });
        }
        durablePlaybackProgressLoaded = true;
      })
      .finally(() => {
        durablePlaybackProgressLoad = undefined;
      });
    return durablePlaybackProgressLoad;
  };
  const persistPlaybackProgressMailbox = async (
    key: string,
    mailbox: PlaybackProgressMailbox,
  ): Promise<void> => {
    if (!options.playbackProgressDurabilityAdapter) return;
    const record: DurablePlaybackEventRecord = {
      version: "v1",
      kind: "progress",
      key,
      events: [
        mailbox.inFlight.event,
        ...(mailbox.successor ? [mailbox.successor.event] : []),
      ],
      updatedAt: new Date(options.now?.() ?? Date.now()).toISOString(),
    };
    if (supersededPlaybackProgressKeys.has(key)) {
      await options.playbackProgressDurabilityAdapter.remove(key);
      durablePlaybackProgress.delete(key);
      return;
    }
    const previousPersistence = playbackProgressPersistence.get(key);
    const persistence = (previousPersistence ?? Promise.resolve())
      .catch(() => undefined)
      .then(async () => {
        if (supersededPlaybackProgressKeys.has(key)) {
          await options.playbackProgressDurabilityAdapter!.remove(key);
          durablePlaybackProgress.delete(key);
          return;
        }
        await options.playbackProgressDurabilityAdapter!.save(record);
        if (!supersededPlaybackProgressKeys.has(key)) {
          durablePlaybackProgress.set(key, record);
        }
      });
    playbackProgressPersistence.set(key, persistence);
    try {
      await persistence;
    } finally {
      if (playbackProgressPersistence.get(key) === persistence) {
        playbackProgressPersistence.delete(key);
      }
      if (supersededPlaybackProgressKeys.has(key)) {
        await options.playbackProgressDurabilityAdapter.remove(key);
        durablePlaybackProgress.delete(key);
        if (!playbackProgressPersistence.has(key)) {
          supersededPlaybackProgressKeys.delete(key);
        }
      }
    }
  };
  const removeDurablePlaybackProgress = async (key: string): Promise<void> => {
    if (options.playbackProgressDurabilityAdapter)
      await options.playbackProgressDurabilityAdapter.remove(key);
    durablePlaybackProgress.delete(key);
  };
  const persistPlaybackTerminalRequest = async (
    record: DurablePlaybackTerminalRecord | DurableCastTransferRecord,
  ): Promise<void> => {
    await options.playbackProgressDurabilityAdapter?.save(record);
  };
  const removeDurablePlaybackTerminalRequest = async (
    key: string,
  ): Promise<void> => {
    await options.playbackProgressDurabilityAdapter?.remove(key);
  };
  const assertPlaybackSessionMutationAllowed = async (
    sessionId: string,
  ): Promise<void> => {
    await loadDurablePlaybackProgress();
    const scope = playbackAuthorityScope(await credentials.current());
    if (playbackStoppingSessions.has(playbackSessionFenceKey(sessionId, scope))) {
      throw new ApiError(
        409,
        "playback_stopping",
        "Playback has an unresolved terminal mutation and cannot be changed.",
      );
    }
  };
  const restoreDurablePlaybackProgress = (
    key: string,
    send: (
      event: PlaybackProgressEvent,
    ) => Promise<PlaybackProgressAcknowledgement>,
  ): void => {
    if (playbackProgressMailboxes.has(key)) return;
    const record = durablePlaybackProgress.get(key);
    if (!record) return;
    const [inFlight, successor] = record.events;
    playbackProgressMailboxes.set(key, {
      inFlight: { event: inFlight!, send, waiters: [] },
      ...(successor
        ? { successor: { event: successor, send, waiters: [] } }
        : {}),
      delivering: false,
      touchedAt:
        Date.parse(record.updatedAt) || (options.now?.() ?? Date.now()),
    });
  };
  const trimPlaybackProgressState = () => {
    while (playbackProgressMailboxes.size > MAX_PLAYBACK_PROGRESS_MAILBOXES) {
      const evictable = [...playbackProgressMailboxes.entries()]
        .filter(([, mailbox]) => !mailbox.delivering)
        .sort((left, right) => left[1].touchedAt - right[1].touchedAt)[0];
      if (!evictable) break;
      const [key, mailbox] = evictable;
      const error = new ApiError(
        0,
        "playback_progress_mailbox_evicted",
        "Playback progress delivery was superseded by newer sessions.",
      );
      for (const waiter of [
        ...mailbox.inFlight.waiters,
        ...(mailbox.successor?.waiters ?? []),
      ])
        waiter.reject(error);
      playbackProgressMailboxes.delete(key);
    }
    let attempts = playbackProgressSequences.size;
    while (
      playbackProgressSequences.size > MAX_PLAYBACK_PROGRESS_SEQUENCES &&
      attempts-- > 0
    ) {
      const key = playbackProgressSequences.keys().next().value as
        string | undefined;
      if (!key) break;
      if (playbackProgressMailboxes.has(key)) {
        const sequence = playbackProgressSequences.get(key)!;
        playbackProgressSequences.delete(key);
        playbackProgressSequences.set(key, sequence);
      } else {
        playbackProgressSequences.delete(key);
      }
    }
  };
  const seedClientPlaybackProgressSequence = (
    key: string,
    nextEventSequence: number,
  ) => {
    if (!Number.isFinite(nextEventSequence)) return;
    const sequence = Math.max(0, Math.trunc(nextEventSequence) - 1);
    const current = playbackProgressSequences.get(key) ?? 0;
    playbackProgressSequences.delete(key);
    playbackProgressSequences.set(key, Math.max(current, sequence));
    trimPlaybackProgressState();
  };
  const orderedClientPlaybackProgressEvent = (
    key: string,
    event: PlaybackProgressInput,
  ): PlaybackProgressEvent => {
    const normalized = normalizedPlaybackProgressInput(event);
    const current = playbackProgressSequences.get(key) ?? 0;
    const requested =
      normalized.eventSequence === undefined
        ? current + 1
        : normalized.eventSequence;
    const eventSequence = Math.max(current + 1, requested);
    playbackProgressSequences.delete(key);
    playbackProgressSequences.set(key, eventSequence);
    trimPlaybackProgressState();
    return {
      ...normalized,
      eventSequence,
      recordedAt:
        event.recordedAt ??
        new Date(options.now?.() ?? Date.now()).toISOString(),
    };
  };
  const acknowledgementIsDurable = (ack: PlaybackProgressAcknowledgement) =>
    ack.accepted || ack.duplicate || ack.stale;
  const rejectPlaybackProgressWaiters = (
    mailbox: PlaybackProgressMailbox,
    error: unknown,
  ) => {
    for (const delivery of [mailbox.inFlight, mailbox.successor]) {
      if (!delivery) continue;
      for (const waiter of delivery.waiters.splice(0)) waiter.reject(error);
    }
  };
  const deliverPlaybackProgress = (key: string): void => {
    const mailbox = playbackProgressMailboxes.get(key);
    if (!mailbox || mailbox.delivering) return;
    mailbox.delivering = true;
    const delivery = mailbox.inFlight;
    void persistPlaybackProgressMailbox(key, mailbox)
      .then(() => {
        const current = playbackProgressMailboxes.get(key);
        if (!current || current.inFlight !== delivery) return undefined;
        return delivery.send(delivery.event);
      })
      .then(async (ack) => {
        if (!ack) return;
        const current = playbackProgressMailboxes.get(key);
        if (!current || current.inFlight !== delivery) return;
        current.touchedAt = options.now?.() ?? Date.now();
        if (!acknowledgementIsDurable(ack)) {
          current.delivering = false;
          rejectPlaybackProgressWaiters(
            current,
            new ApiError(
              0,
              "playback_progress_unacknowledged",
              "The server did not durably acknowledge playback progress.",
            ),
          );
          return;
        }
        seedClientPlaybackProgressSequence(key, ack.highestEventSequence + 1);
        for (const waiter of delivery.waiters.splice(0)) waiter.resolve(ack);
        if (current.successor) {
          if (
            current.successor.event.eventSequence <= ack.highestEventSequence
          ) {
            const eventSequence = ack.highestEventSequence + 1;
            current.successor.event = {
              ...current.successor.event,
              eventSequence,
            };
            const allocatedSequence = playbackProgressSequences.get(key) ?? 0;
            playbackProgressSequences.delete(key);
            playbackProgressSequences.set(
              key,
              Math.max(allocatedSequence, eventSequence),
            );
          }
          current.inFlight = current.successor;
          current.successor = undefined;
          await persistPlaybackProgressMailbox(key, current);
          current.delivering = false;
          deliverPlaybackProgress(key);
          return;
        }
        await removeDurablePlaybackProgress(key);
        playbackProgressMailboxes.delete(key);
        trimPlaybackProgressState();
      })
      .catch((error) => {
        const current = playbackProgressMailboxes.get(key);
        if (!current || !current.delivering) return;
        current.delivering = false;
        current.touchedAt = options.now?.() ?? Date.now();
        rejectPlaybackProgressWaiters(current, error);
        // Preserve the exact uncertain event and newest coalesced successor.
        // A later call must retry that event before a successor reaches the wire.
      });
  };
  const enqueuePlaybackProgress = (
    key: string,
    body: PlaybackProgressInput,
    send: (
      event: PlaybackProgressEvent,
    ) => Promise<PlaybackProgressAcknowledgement>,
  ): Promise<PlaybackProgressAcknowledgement> =>
    loadDurablePlaybackProgress().then(() => {
      restoreDurablePlaybackProgress(key, send);
      return new Promise((resolve, reject) => {
        const now = options.now?.() ?? Date.now();
        const waiter = { resolve, reject };
        const mailbox = playbackProgressMailboxes.get(key);
        if (!mailbox) {
          playbackProgressMailboxes.set(key, {
            inFlight: {
              event: orderedClientPlaybackProgressEvent(key, body),
              send,
              waiters: [waiter],
            },
            delivering: false,
            touchedAt: now,
          });
          trimPlaybackProgressState();
          deliverPlaybackProgress(key);
          return;
        }
        mailbox.touchedAt = now;
        mailbox.successor = {
          event: orderedClientPlaybackProgressEvent(key, body),
          send,
          waiters: [...(mailbox.successor?.waiters ?? []), waiter],
        };
        if (!mailbox.delivering) {
          // Keep the progress event immutable, but use the newest live request
          // context so an aborted signal or rotated continuation token cannot
          // poison every retry.
          mailbox.inFlight.send = send;
          deliverPlaybackProgress(key);
        }
      });
    });
  const forgetClientPlaybackProgress = async (
    sessionId: string,
    scope: PlaybackAuthorityScope,
  ): Promise<void> => {
    playbackSessionGenerations.delete(sessionId);
    playbackSessionNextEventSequences.delete(sessionId);
    const supersededKeys = new Set<string>();
    for (const key of durablePlaybackProgress.keys()) {
      const identity = JSON.parse(key) as unknown[];
      if (
        identity[5] === sessionId &&
        playbackAuthorityScopesEqual(
          identity.slice(0, 5) as unknown as PlaybackAuthorityScope,
          scope,
        )
      ) supersededKeys.add(key);
    }
    for (const [key, mailbox] of playbackProgressMailboxes) {
      const identity = JSON.parse(key) as unknown[];
      if (
        identity[5] !== sessionId ||
        !playbackAuthorityScopesEqual(
          identity.slice(0, 5) as unknown as PlaybackAuthorityScope,
          scope,
        )
      ) continue;
      supersededKeys.add(key);
      rejectPlaybackProgressWaiters(
        mailbox,
        new ApiError(
          0,
          "playback_progress_stopped",
          "Playback stopped before queued progress could be delivered.",
        ),
      );
      playbackProgressMailboxes.delete(key);
      playbackProgressSequences.delete(key);
    }
    for (const key of supersededKeys) {
      supersededPlaybackProgressKeys.add(key);
    }
    await Promise.allSettled(
      [...supersededKeys]
        .map((key) => playbackProgressPersistence.get(key))
        .filter((persistence): persistence is Promise<void> => Boolean(persistence)),
    );
    const removals = await Promise.allSettled(
      [...supersededKeys].map((key) => removeDurablePlaybackProgress(key)),
    );
    for (const key of supersededKeys) {
      if (!playbackProgressPersistence.has(key)) {
        supersededPlaybackProgressKeys.delete(key);
      }
    }
    const failed = removals.find(
      (result): result is PromiseRejectedResult => result.status === "rejected",
    );
    if (failed) throw failed.reason;
  };
  const waitForPendingPlaybackProgress = (
    sessionId: string,
    scope: PlaybackAuthorityScope,
  ): Promise<void> => {
    const waits: Promise<unknown>[] = [];
    for (const [key, mailbox] of playbackProgressMailboxes) {
      const identity = JSON.parse(key) as unknown[];
      if (
        identity[5] !== sessionId ||
        !playbackAuthorityScopesEqual(
          identity.slice(0, 5) as unknown as PlaybackAuthorityScope,
          scope,
        )
      ) continue;
      waits.push(
        new Promise<PlaybackProgressAcknowledgement>((resolve, reject) => {
          (mailbox.successor ?? mailbox.inFlight).waiters.push({
            resolve,
            reject,
          });
          if (!mailbox.delivering) deliverPlaybackProgress(key);
        }),
      );
    }
    return Promise.all(waits).then(() => undefined);
  };
  const drainPendingPlaybackProgressBeforeStop = async (
    sessionId: string,
    scope: PlaybackAuthorityScope,
  ): Promise<void> => {
    const pending = waitForPendingPlaybackProgress(sessionId, scope);
    let timer: ReturnType<typeof setTimeout> | undefined;
    await Promise.race([
      pending.catch(() => undefined),
      new Promise<void>((resolve) => {
        timer = setTimeout(resolve, 500);
      }),
    ]).finally(() => {
      if (timer !== undefined) clearTimeout(timer);
    });
  };
  const terminalInputMatches = (
    terminal: PlaybackTerminalEvent,
    input: PlaybackTerminalInput,
  ) =>
    terminal.disposition === input.disposition &&
    terminal.positionSeconds === input.positionSeconds &&
    terminal.durationSeconds === input.durationSeconds;
  const terminalEventMatches = (
    left: PlaybackTerminalEvent,
    right: PlaybackTerminalEvent,
  ) =>
    terminalInputMatches(left, right) &&
    left.generation === right.generation &&
    left.eventSequence === right.eventSequence &&
    left.recordedAt === right.recordedAt;
  const handoffInputMatches = (
    cached: PlaybackHandoffRequest,
    input: PlaybackHandoffInput,
  ) => {
    const terminalMatches = isCompletePlaybackTerminalEvent(
      input.previousTerminal,
    )
      ? terminalEventMatches(cached.previousTerminal, input.previousTerminal)
      : terminalInputMatches(cached.previousTerminal, input.previousTerminal);
    return (
      terminalMatches &&
      cached.requestId === input.requestId &&
      cached.entryId === input.entryId &&
      cached.preparedSessionId === input.preparedSessionId &&
      cached.expectedQueueRevision === input.expectedQueueRevision &&
      cached.expectedPlaybackRevision === input.expectedPlaybackRevision &&
      cached.startSeconds === input.startSeconds &&
      JSON.stringify(cached.intent) === JSON.stringify(input.intent) &&
      JSON.stringify(cached.sourceContext) === JSON.stringify(input.sourceContext) &&
      (input.clientProfile === undefined ||
        JSON.stringify(cached.clientProfile) ===
          JSON.stringify(input.clientProfile))
    );
  };
  const cachePlaybackTerminalRequest = (
    key: string,
    sessionId: string,
    scope: PlaybackAuthorityScope,
    terminalRequest: PlaybackSessionStopRequest,
  ) => {
    if (
      !playbackTerminalRequests.has(key) &&
      playbackTerminalRequests.size +
          playbackHandoffRequests.size +
          playbackReplacementRequests.size +
          playbackLiveTvCloseRequests.size >=
        MAX_PLAYBACK_TERMINAL_REQUESTS
    ) {
      throw new ApiError(
        503,
        "playback_terminal_outbox_full",
        "Playback terminal recovery must finish before another session can close.",
      );
    }
    const snapshot = immutableJSONSnapshot(terminalRequest);
    playbackTerminalRequests.delete(key);
    playbackTerminalRequests.set(key, snapshot);
    playbackTerminalSessionIds.set(key, sessionId);
    playbackTerminalScopes.set(key, scope);
    return snapshot;
  };
  const cachePlaybackHandoffRequest = (
    key: string,
    sessionId: string,
    scope: PlaybackAuthorityScope,
    handoffRequest: PlaybackHandoffRequest,
  ) => {
    if (
      !playbackHandoffRequests.has(key) &&
      playbackTerminalRequests.size +
          playbackHandoffRequests.size +
          playbackReplacementRequests.size +
          playbackLiveTvCloseRequests.size >=
        MAX_PLAYBACK_TERMINAL_REQUESTS
    ) {
      throw new ApiError(
        503,
        "playback_terminal_outbox_full",
        "Playback terminal recovery must finish before another handoff can begin.",
      );
    }
    const snapshot = immutableJSONSnapshot(handoffRequest);
    playbackHandoffRequests.set(key, snapshot);
    playbackTerminalSessionIds.set(key, sessionId);
    playbackTerminalScopes.set(key, scope);
    return snapshot;
  };
  const cachePlaybackReplacementRequest = (
    key: string,
    sessionId: string,
    scope: PlaybackAuthorityScope,
    replacement: {
      request: PlaybackReplacementRequest;
      target: DurablePlaybackReplacementTarget;
    },
  ) => {
    if (
      !playbackReplacementRequests.has(key) &&
      playbackTerminalRequests.size +
          playbackHandoffRequests.size +
          playbackReplacementRequests.size +
          playbackLiveTvCloseRequests.size >=
        MAX_PLAYBACK_TERMINAL_REQUESTS
    ) {
      throw new ApiError(
        503,
        "playback_terminal_outbox_full",
        "Playback terminal recovery must finish before another replacement can begin.",
      );
    }
    const snapshot = immutableJSONSnapshot(replacement);
    playbackReplacementRequests.set(key, snapshot);
    playbackTerminalSessionIds.set(key, sessionId);
    playbackTerminalScopes.set(key, scope);
    return snapshot;
  };
  const allocatePlaybackTerminalEvent = async (
    sessionId: string,
    body: PlaybackTerminalInput,
    session: LocalServerSession | undefined,
    scope: PlaybackAuthorityScope,
    init?: RequestSignal,
  ): Promise<PlaybackTerminalEvent> => {
    assertPlaybackTerminalEvent(body);
    await loadDurablePlaybackProgress();
    for (const key of durablePlaybackProgress.keys()) {
      const identity = JSON.parse(key) as unknown[];
      if (
        identity[5] !== sessionId ||
        !playbackAuthorityScopesEqual(
          identity.slice(0, 5) as unknown as PlaybackAuthorityScope,
          scope,
        )
      ) continue;
      restoreDurablePlaybackProgress(key, (event) =>
        request<PlaybackProgressAcknowledgement>(
          `/api/playback-sessions/${encodeURIComponent(sessionId)}`,
          { ...init, method: "PATCH", body: event },
        ),
      );
    }
    await drainPendingPlaybackProgressBeforeStop(sessionId, scope);
    const generation = playbackSessionGenerations.get(sessionId) ?? 0;
    if (!Number.isSafeInteger(generation) || generation <= 0) {
      throw new ApiError(
        409,
        "playback_session_authority_unavailable",
        "Playback session authority is unavailable.",
      );
    }
    const key = playbackProgressKey(sessionId, generation, session);
    seedClientPlaybackProgressSequence(
      key,
      playbackSessionNextEventSequences.get(sessionId) ?? 1,
    );
    const ordered = orderedClientPlaybackProgressEvent(key, {
      positionSeconds: body.positionSeconds,
      durationSeconds: body.durationSeconds,
      state: "paused",
    });
    const terminal: PlaybackTerminalEvent = {
      disposition: body.disposition,
      generation,
      eventSequence: ordered.eventSequence,
      recordedAt: ordered.recordedAt,
      positionSeconds: body.positionSeconds,
      durationSeconds: body.durationSeconds,
    };
    assertPlaybackTerminalEvent(terminal);
    return terminal;
  };
  const normalizeSessionPlayback = (response: PlaybackResponse) => {
    const generation = Number.isFinite(response.generation)
      ? Math.max(0, Math.trunc(response.generation))
      : 0;
    playbackSessionGenerations.set(response.sessionId, generation);
    playbackSessionNextEventSequences.set(
      response.sessionId,
      response.nextEventSequence,
    );
    seedClientPlaybackProgressSequence(
      playbackProgressKey(response.sessionId, generation),
      response.nextEventSequence,
    );
    return normalizePlaybackResponse(response);
  };
  const playbackHistoryQuery = (params?: PlaybackHistoryParams) => {
    const search = new URLSearchParams();
    if (params?.userId && params.userId !== "all")
      search.set("userId", params.userId);
    if (params?.libraryId && params.libraryId !== "all")
      search.set("libraryId", params.libraryId);
    if (params?.type && params.type !== "all") search.set("type", params.type);
    if (params?.query?.trim()) search.set("query", params.query.trim());
    if (params?.period) search.set("period", params.period);
    if (params?.limit) search.set("limit", String(params.limit));
    if (params?.cursor) search.set("cursor", params.cursor);
    if (params?.count) search.set("count", params.count);
    return search.toString();
  };
  const imageResourceUrl = (
    path: string,
    imageOptions: { width?: number; height?: number } = {},
  ) => {
    if (!path || !path.startsWith("/api/")) return "";
    const url = new URL(resourceUrl(path), baseHref(options.baseHref));
    if (imageOptions.width)
      url.searchParams.set("width", String(imageOptions.width));
    if (imageOptions.height)
      url.searchParams.set("height", String(imageOptions.height));
    return url.toString();
  };
  const profile = () =>
    options.playbackClientProfile?.() ?? genericPlaybackClientProfile();
  const clientInstanceId = () =>
    normalizeClientInstanceId(options.playbackClientInstanceId?.() ?? "");

  const playbackReplacementTargetPath = (
    target: DurablePlaybackReplacementTarget,
  ): string => {
    switch (target.kind) {
      case "media":
        return "/api/playback-sessions";
      case "live-tv":
        return "/api/live-tv/play";
      case "live-tv-stream":
        return `/api/live-tv/streams/${encodeURIComponent(target.resourceId!)}/open`;
      case "dvr":
        return `/api/dvr/recordings/${encodeURIComponent(target.resourceId!)}/playback`;
      case "library-channel":
        return `/api/library-channels/${encodeURIComponent(target.resourceId!)}/tune`;
    }
  };
  const requiredPlaybackReplacementResourceId = (
    value: string,
    kind: PlaybackReplacementTarget["kind"],
  ) => {
    if (typeof value !== "string" || value.length < 1 || value.length > 2_048) {
      throw new TypeError(`Playback replacement ${kind} identity is invalid.`);
    }
    return value;
  };
  const durablePlaybackReplacementTarget = (
    target: PlaybackReplacementTarget,
    replacement: PlaybackReplacementRequest,
  ): DurablePlaybackReplacementTarget => {
    let durableTarget: DurablePlaybackReplacementTarget;
    switch (target.kind) {
      case "media": {
        const playbackOptions = target.playbackOptions ?? {};
        durableTarget = {
          kind: target.kind,
          body: {
            mediaId: requiredPlaybackReplacementResourceId(
              target.mediaId,
              target.kind,
            ),
            clientInstanceId: clientInstanceId(),
            clientProfile: profile(),
            intent: playbackOptions.intent ?? {
              quality: { mode: "automatic" },
            },
            versionId: playbackOptions.versionId,
            skipPreroll: playbackOptions.skipPreroll,
            burnInSubtitleId: playbackOptions.burnInSubtitleId,
            subtitleStreamId: playbackOptions.subtitleStreamId,
            audioStreamId: playbackOptions.audioStreamId,
            startSeconds: playbackOptions.startSeconds,
            queueMediaIds: playbackOptions.queueMediaIds,
            repeatMode: playbackOptions.repeatMode,
            sourceContext: playbackOptions.sourceContext,
            replacement,
          },
        };
        break;
      }
      case "live-tv":
      case "live-tv-stream": {
        const playbackOptions = target.playbackOptions ?? {};
        const channelId = requiredPlaybackReplacementResourceId(
          target.channelId,
          target.kind,
        );
        durableTarget = {
          kind: target.kind,
          ...(target.kind === "live-tv-stream"
            ? { resourceId: channelId }
            : {}),
          body: {
            channelId,
            clientInstanceId: clientInstanceId(),
            clientProfile: playbackOptions.clientProfile ?? profile(),
            intent: playbackOptions.intent ?? {
              quality: { mode: "automatic" },
            },
            replacement,
          },
        };
        break;
      }
      case "dvr": {
        const playbackOptions = target.playbackOptions ?? {};
        const recordingId = requiredPlaybackReplacementResourceId(
          target.recordingId,
          target.kind,
        );
        durableTarget = {
          kind: target.kind,
          resourceId: recordingId,
          body: {
            ...playbackOptions,
            intent: playbackOptions.intent ?? {
              quality: { mode: "automatic" },
            },
            clientInstanceId:
              playbackOptions.clientInstanceId ?? clientInstanceId(),
            clientProfile: playbackOptions.clientProfile ?? profile(),
            replacement,
          },
        };
        break;
      }
      case "library-channel": {
        const playbackOptions = target.playbackOptions ?? {};
        const channelId = requiredPlaybackReplacementResourceId(
          target.channelId,
          target.kind,
        );
        durableTarget = {
          kind: target.kind,
          resourceId: channelId,
          body: {
            ...playbackOptions,
            intent: playbackOptions.intent ?? {
              quality: { mode: "automatic" },
            },
            clientInstanceId:
              playbackOptions.clientInstanceId ?? clientInstanceId(),
            clientProfile: playbackOptions.clientProfile ?? profile(),
            replacement,
          },
        };
        break;
      }
    }
    const snapshot = immutableJSONSnapshot(durableTarget);
    assertDurablePlaybackReplacementTarget(snapshot, replacement);
    return snapshot;
  };
  const normalizePlaybackReplacementTargetResponse = (
    kind: PlaybackReplacementTarget["kind"],
    response: PlaybackResponse | LibraryChannelTuneResponse,
  ): PlaybackResponse | LibraryChannelTuneResponse => {
    if (kind === "library-channel") {
      const tuneResponse = response as LibraryChannelTuneResponse;
      return {
        ...tuneResponse,
        playback: tuneResponse.playback
          ? normalizeSessionPlayback(tuneResponse.playback)
          : tuneResponse.playback,
      };
    }
    return normalizeSessionPlayback(response as PlaybackResponse);
  };
  const playbackReplacementRejection = (
    error: ApiError,
  ): PlaybackReplacementRejection => ({
    status: error.status,
    code: error.code,
    detail: error.detail,
  });
  const isDefinitiveInactivePlaybackReplacement = (
    error: unknown,
  ): error is ApiError =>
    error instanceof ApiError &&
    error.code === "replacement_source_inactive" &&
    !error.ambiguous &&
    !error.retryable;
  const stopPlaybackSession = async (
    sessionId: string,
    body: PlaybackSessionStopInput,
    init?: RequestSignal,
  ): Promise<PlaybackSessionTerminalAcknowledgement> => {
    if (isPlaybackSessionStopRequest(body)) {
      assertPlaybackSessionStopRequest(body);
    } else {
      assertPlaybackTerminalEvent(body);
    }
    await loadDurablePlaybackProgress();
    const session = await credentials.current();
    const scope = playbackAuthorityScope(session);
    const fenceKey = playbackSessionFenceKey(sessionId, scope);
    const requestedRequestId = isPlaybackSessionStopRequest(body)
      ? body.requestId
      : "core-owned";
    const key = playbackTerminalKey(
      "stop",
      sessionId,
      requestedRequestId,
      scope,
    );
    const cachedStopKey = isPlaybackSessionStopRequest(body)
      ? key
      : [...playbackTerminalRequests.keys()].find(
          (candidate) =>
            playbackTerminalSessionIds.get(candidate) === sessionId &&
            playbackAuthorityScopesEqual(
              playbackTerminalScopes.get(candidate) ?? ["", "", "", "", ""],
              scope,
            ),
        ) ?? key;
    if (playbackTerminalRequestsInFlight.has(fenceKey)) {
      throw new ApiError(
        409,
        "playback_stopping",
        "Playback already has a terminal mutation in flight.",
      );
    }
    const cached = playbackTerminalRequests.get(cachedStopKey);
    if (playbackStoppingSessions.has(fenceKey) && !cached) {
      throw new ApiError(
        409,
        "playback_terminal_request_conflict",
        "This playback session already has a different uncertain terminal request.",
      );
    }
    playbackStoppingSessions.add(fenceKey);
    playbackTerminalRequestsInFlight.add(fenceKey);
    let requestDispatched = false;
    let requestKey = cached ? cachedStopKey : key;
    try {
      let requestBody = cached;
      if (requestBody) {
        const matches = isPlaybackSessionStopRequest(body)
          ? requestBody.requestId === body.requestId &&
            terminalEventMatches(requestBody.terminal, body.terminal)
          : terminalInputMatches(requestBody.terminal, body);
        if (!matches) {
          throw new ApiError(
            409,
            "playback_terminal_request_conflict",
            "This playback session already has an uncertain terminal request.",
          );
        }
      } else if (isPlaybackSessionStopRequest(body)) {
        requestBody = cachePlaybackTerminalRequest(
          requestKey,
          sessionId,
          scope,
          body,
        );
        await persistPlaybackTerminalRequest({
          version: "v2",
          kind: "terminal",
          key: requestKey,
          scope,
          sessionId,
          operation: "stop",
          request: requestBody,
          updatedAt: new Date(options.now?.() ?? Date.now()).toISOString(),
        });
      } else {
        const terminal = await allocatePlaybackTerminalEvent(
          sessionId,
          body,
          session,
          scope,
          init,
        );
        const requestId = createRequestId(options.requestId).slice(0, 128);
        assertPlaybackTerminalRequestId(requestId);
        requestBody = {
          requestId,
          terminal,
        };
        requestKey = playbackTerminalKey(
          "stop",
          sessionId,
          requestId,
          scope,
        );
        requestBody = cachePlaybackTerminalRequest(
          requestKey,
          sessionId,
          scope,
          requestBody,
        );
        await persistPlaybackTerminalRequest({
          version: "v2",
          kind: "terminal",
          key: requestKey,
          scope,
          sessionId,
          operation: "stop",
          request: requestBody,
          updatedAt: new Date(options.now?.() ?? Date.now()).toISOString(),
        });
      }
      assertPlaybackSessionStopRequest(requestBody);
      requestDispatched = true;
      const response = await request<PlaybackSessionTerminalAcknowledgement>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}`,
        {
          ...init,
          method: "DELETE",
          body: requestBody,
        },
      );
      assertPlaybackTerminalAcknowledgement(response, sessionId, requestBody);
      await forgetClientPlaybackProgress(sessionId, scope);
      await removeDurablePlaybackTerminalRequest(requestKey);
      playbackTerminalRequests.delete(requestKey);
      playbackTerminalSessionIds.delete(requestKey);
      playbackTerminalScopes.delete(requestKey);
      playbackStoppingSessions.delete(fenceKey);
      return response;
    } catch (error) {
      if (
        (!requestDispatched && !cached) ||
        terminalMutationRejectionIsDefinitive(error)
      ) {
        await removeDurablePlaybackTerminalRequest(requestKey);
        playbackTerminalRequests.delete(requestKey);
        playbackTerminalSessionIds.delete(requestKey);
        playbackTerminalScopes.delete(requestKey);
        playbackStoppingSessions.delete(fenceKey);
      }
      throw error;
    } finally {
      playbackTerminalRequestsInFlight.delete(fenceKey);
    }
  };
  const closeLiveTvStream = async (
    channelId: string,
    sessionId: string,
    body: PlaybackSessionStopInput,
    init?: RequestSignal,
  ): Promise<PlaybackSessionTerminalAcknowledgement> => {
    requiredPlaybackReplacementResourceId(channelId, "live-tv-stream");
    if (isPlaybackSessionStopRequest(body)) {
      assertPlaybackSessionStopRequest(body);
    } else {
      assertPlaybackTerminalEvent(body);
    }
    await loadDurablePlaybackProgress();
    const session = await credentials.current();
    const scope = playbackAuthorityScope(session);
    const fenceKey = playbackSessionFenceKey(sessionId, scope);
    const cachedEntry = [...playbackLiveTvCloseRequests.entries()].find(
      ([key, value]) =>
        value.request.sessionId === sessionId &&
        playbackAuthorityScopesEqual(
          playbackTerminalScopes.get(key) ?? ["", "", "", "", ""],
          scope,
        ),
    );
    if (playbackStoppingSessions.has(fenceKey) && !cachedEntry) {
      throw new ApiError(
        409,
        "playback_terminal_request_conflict",
        "This Live TV session already has a different terminal request.",
      );
    }
    if (playbackTerminalRequestsInFlight.has(fenceKey)) {
      throw new ApiError(
        409,
        "playback_stopping",
        "Playback already has a terminal mutation in flight.",
      );
    }
    playbackStoppingSessions.add(fenceKey);
    playbackTerminalRequestsInFlight.add(fenceKey);
    let key = cachedEntry?.[0];
    let requestBody = cachedEntry?.[1].request;
    let dispatched = false;
    try {
      if (requestBody) {
        const matches =
          cachedEntry![1].channelId === channelId &&
          (isPlaybackSessionStopRequest(body)
            ? requestBody.requestId === body.requestId &&
              terminalEventMatches(requestBody.terminal, body.terminal)
            : terminalInputMatches(requestBody.terminal, body));
        if (!matches) {
          throw new ApiError(
            409,
            "playback_terminal_request_conflict",
            "This Live TV session already has a different terminal request.",
          );
        }
      } else {
        if (
          playbackTerminalRequests.size +
            playbackHandoffRequests.size +
            playbackReplacementRequests.size +
            playbackLiveTvCloseRequests.size >=
          MAX_PLAYBACK_TERMINAL_REQUESTS
        ) {
          throw new ApiError(
            503,
            "playback_terminal_outbox_full",
            "Playback terminal recovery must finish before another Live TV session can close.",
          );
        }
        const terminal = isPlaybackSessionStopRequest(body)
          ? body.terminal
          : await allocatePlaybackTerminalEvent(
              sessionId,
              body,
              session,
              scope,
              init,
            );
        const requestId = isPlaybackSessionStopRequest(body)
          ? body.requestId
          : createRequestId(options.requestId).slice(0, 128);
        requestBody = immutableJSONSnapshot({
          sessionId,
          requestId,
          terminal,
        });
        assertLiveTvStreamCloseRequest(requestBody);
        key = playbackTerminalKey(
          "live-tv-close",
          sessionId,
          requestId,
          scope,
        );
        playbackLiveTvCloseRequests.set(key, { channelId, request: requestBody });
        playbackTerminalSessionIds.set(key, sessionId);
        playbackTerminalScopes.set(key, scope);
        await persistPlaybackTerminalRequest({
          version: "v2",
          kind: "terminal",
          key,
          scope,
          sessionId,
          operation: "live-tv-close",
          channelId,
          request: requestBody,
          updatedAt: new Date(options.now?.() ?? Date.now()).toISOString(),
        });
      }
      dispatched = true;
      const response = await request<PlaybackSessionTerminalAcknowledgement>(
        `/api/live-tv/streams/${encodeURIComponent(channelId)}/close`,
        { ...init, method: "POST", body: requestBody },
      );
      assertPlaybackTerminalAcknowledgement(response, sessionId, requestBody);
      await forgetClientPlaybackProgress(sessionId, scope);
      await removeDurablePlaybackTerminalRequest(key!);
      playbackLiveTvCloseRequests.delete(key!);
      playbackTerminalSessionIds.delete(key!);
      playbackTerminalScopes.delete(key!);
      playbackStoppingSessions.delete(fenceKey);
      return response;
    } catch (error) {
      if ((!dispatched && !cachedEntry) || terminalMutationRejectionIsDefinitive(error)) {
        if (key) {
          await removeDurablePlaybackTerminalRequest(key);
          playbackLiveTvCloseRequests.delete(key);
          playbackTerminalSessionIds.delete(key);
          playbackTerminalScopes.delete(key);
        }
        playbackStoppingSessions.delete(fenceKey);
      }
      throw error;
    } finally {
      playbackTerminalRequestsInFlight.delete(fenceKey);
    }
  };
  const clearPlaybackReplacementRequest = async (
    key: string,
    fenceKey: string,
  ): Promise<void> => {
    await removeDurablePlaybackTerminalRequest(key);
    playbackReplacementRequests.delete(key);
    playbackTerminalSessionIds.delete(key);
    playbackTerminalScopes.delete(key);
    playbackStoppingSessions.delete(fenceKey);
  };
  const executePlaybackReplacement = async (
    key: string,
    replacement: {
      request: PlaybackReplacementRequest;
      target: DurablePlaybackReplacementTarget;
      committedOutcome?: CommittedPlaybackReplacementOutcome;
    },
    scope: PlaybackAuthorityScope,
    init?: RequestSignal,
    lockAcquired = false,
  ): Promise<
    PlaybackReplacementOutcome<PlaybackResponse | LibraryChannelTuneResponse>
  > => {
    const sourceSessionId = replacement.request.sourceSessionId;
    const fenceKey = playbackSessionFenceKey(sourceSessionId, scope);
    if (!lockAcquired) {
      if (playbackTerminalRequestsInFlight.has(fenceKey)) {
        throw new ApiError(
          409,
          "playback_stopping",
          "Playback already has a terminal mutation in flight.",
        );
      }
      playbackStoppingSessions.add(fenceKey);
      playbackTerminalRequestsInFlight.add(fenceKey);
    }
    let completedFallback:
      | {
          rejection: PlaybackReplacementRejection;
          terminal: PlaybackTerminalEvent;
        }
      | undefined;
    try {
      if (replacement.committedOutcome) {
        return replacement.committedOutcome;
      }
      const response = await request<
        PlaybackResponse | LibraryChannelTuneResponse
      >(playbackReplacementTargetPath(replacement.target), {
        ...init,
        method: "POST",
        body: replacement.target.body,
      });
      const normalized = normalizePlaybackReplacementTargetResponse(
        replacement.target.kind,
        response,
      );
      await forgetClientPlaybackProgress(sourceSessionId, scope);
      await clearPlaybackReplacementRequest(key, fenceKey);
      return { outcome: "accepted", value: normalized };
    } catch (error) {
      if (
        error instanceof ApiError &&
        error.code === "playback_replacement_committed_restore_required"
      ) {
        const replacementSessionId = error.details?.replacementSessionId;
        if (
          typeof replacementSessionId !== "string" ||
          replacementSessionId.length < 1 ||
          replacementSessionId.length > 512
        ) {
          // The server claims commit but did not provide the only safe public
          // replacement identity. Preserve the exact request for reconciliation.
          throw error;
        }
        const committedOutcome: CommittedPlaybackReplacementOutcome = {
          outcome: "committed-restore-required",
          sourceSessionId,
          replacementSessionId,
        };
        await persistPlaybackTerminalRequest({
          version: "v2",
          kind: "terminal",
          key,
          scope,
          sessionId: sourceSessionId,
          operation: "replacement",
          request: replacement.request,
          target: replacement.target,
          committedOutcome,
          updatedAt: new Date(options.now?.() ?? Date.now()).toISOString(),
        });
        playbackReplacementRequests.set(
          key,
          immutableJSONSnapshot({ ...replacement, committedOutcome }),
        );
        await forgetClientPlaybackProgress(sourceSessionId, scope);
        return committedOutcome;
      }
      if (isDefinitiveInactivePlaybackReplacement(error)) {
        const rejection = playbackReplacementRejection(error);
        await forgetClientPlaybackProgress(sourceSessionId, scope);
        await clearPlaybackReplacementRequest(key, fenceKey);
        return {
          outcome: "source-inactive",
          sourceSessionId,
          rejection,
        };
      }
      if (!terminalMutationRejectionIsDefinitive(error)) throw error;
      const rejection = playbackReplacementRejection(error as ApiError);
      await clearPlaybackReplacementRequest(key, fenceKey);
      if (replacement.request.previousTerminal.disposition === "stopped") {
        return {
          outcome: "source-retained",
          sourceSessionId,
          rejection,
        };
      }
      completedFallback = {
        rejection,
        terminal: replacement.request.previousTerminal,
      };
    } finally {
      playbackTerminalRequestsInFlight.delete(fenceKey);
    }
    const stopRequestId = createRequestId(options.requestId).slice(0, 128);
    assertPlaybackTerminalRequestId(stopRequestId);
    const terminal = await stopPlaybackSession(
      sourceSessionId,
      {
        requestId: stopRequestId,
        terminal: completedFallback!.terminal,
      },
      init,
    );
    return {
      outcome: "source-closed",
      sourceSessionId,
      rejection: completedFallback!.rejection,
      terminal,
    };
  };
  const replacementInputMatches = (
    cached: PlaybackReplacementRequest,
    input: PlaybackReplacementInput,
  ): boolean => {
    const terminalMatches = isCompletePlaybackTerminalEvent(
      input.previousTerminal,
    )
      ? terminalEventMatches(cached.previousTerminal, input.previousTerminal)
      : terminalInputMatches(cached.previousTerminal, input.previousTerminal);
    return (
      terminalMatches &&
      cached.sourceSessionId === input.sourceSessionId &&
      (input.requestId === undefined || cached.requestId === input.requestId) &&
      cached.expectedQueueRevision === input.expectedQueueRevision &&
      cached.expectedPlaybackRevision === input.expectedPlaybackRevision
    );
  };
  const replacePlaybackTarget = async <
    Target extends PlaybackReplacementTarget,
  >(
    target: Target,
    input: PlaybackReplacementInput,
    init?: RequestSignal,
  ): Promise<
    PlaybackReplacementOutcome<PlaybackReplacementTargetResponse<Target>>
  > => {
    assertPlaybackReplacementRequest(input);
    await loadDurablePlaybackProgress();
    const session = await credentials.current();
    const scope = playbackAuthorityScope(session);
    const fenceKey = playbackSessionFenceKey(input.sourceSessionId, scope);
    const cachedEntry = [...playbackReplacementRequests.entries()].find(
      ([key, replacement]) =>
        playbackTerminalSessionIds.get(key) === input.sourceSessionId &&
        playbackAuthorityScopesEqual(
          playbackTerminalScopes.get(key) ?? ["", "", "", "", ""],
          scope,
        ) &&
        (input.requestId === undefined ||
          replacement.request.requestId === input.requestId),
    );
    if (playbackStoppingSessions.has(fenceKey) && !cachedEntry) {
      throw new ApiError(
        409,
        "playback_terminal_request_conflict",
        "This playback session already has a different uncertain terminal request.",
      );
    }
    if (playbackTerminalRequestsInFlight.has(fenceKey)) {
      throw new ApiError(
        409,
        "playback_stopping",
        "Playback already has a terminal mutation in flight.",
      );
    }
    const fenceWasAlreadySet = playbackStoppingSessions.has(fenceKey);
    playbackStoppingSessions.add(fenceKey);
    playbackTerminalRequestsInFlight.add(fenceKey);
    let key: string | undefined;
    let replacement: {
      request: PlaybackReplacementRequest;
      target: DurablePlaybackReplacementTarget;
      committedOutcome?: CommittedPlaybackReplacementOutcome;
    };
    let executionStarted = false;
    try {
      if (cachedEntry) {
        [key, replacement] = cachedEntry;
        if (!replacementInputMatches(replacement.request, input)) {
          throw new ApiError(
            409,
            "playback_terminal_request_conflict",
            "This playback replacement identity already owns a different request body.",
          );
        }
        const requestedTarget = durablePlaybackReplacementTarget(
          target,
          replacement.request,
        );
        if (JSON.stringify(requestedTarget) !== JSON.stringify(replacement.target)) {
          throw new ApiError(
            409,
            "playback_terminal_request_conflict",
            "This playback replacement identity already owns a different target.",
          );
        }
      } else {
        const previousTerminal = isCompletePlaybackTerminalEvent(
          input.previousTerminal,
        )
          ? input.previousTerminal
          : await allocatePlaybackTerminalEvent(
              input.sourceSessionId,
              input.previousTerminal,
              session,
              scope,
              init,
            );
        const requestId =
          input.requestId ?? createRequestId(options.requestId).slice(0, 128);
        const requestBody: PlaybackReplacementRequest = {
          sourceSessionId: input.sourceSessionId,
          requestId,
          previousTerminal,
          ...(input.expectedQueueRevision !== undefined
            ? { expectedQueueRevision: input.expectedQueueRevision }
            : {}),
          ...(input.expectedPlaybackRevision !== undefined
            ? { expectedPlaybackRevision: input.expectedPlaybackRevision }
            : {}),
        };
        assertPlaybackReplacementRequest(requestBody);
        key = playbackTerminalKey(
          "replacement",
          input.sourceSessionId,
          requestId,
          scope,
        );
        replacement = cachePlaybackReplacementRequest(
          key,
          input.sourceSessionId,
          scope,
          {
            request: requestBody,
            target: durablePlaybackReplacementTarget(target, requestBody),
          },
        );
        await persistPlaybackTerminalRequest({
          version: "v2",
          kind: "terminal",
          key,
          scope,
          sessionId: input.sourceSessionId,
          operation: "replacement",
          request: replacement.request,
          target: replacement.target,
          updatedAt: new Date(options.now?.() ?? Date.now()).toISOString(),
        });
      }
      if (!key || !replacement) {
        throw new ApiError(
          409,
          "playback_terminal_outbox_conflict",
          "The playback replacement could not be resolved exactly.",
        );
      }
      executionStarted = true;
      const outcome = await executePlaybackReplacement(
        key,
        replacement,
        scope,
        init,
        true,
      );
      return outcome as PlaybackReplacementOutcome<
        PlaybackReplacementTargetResponse<Target>
      >;
    } catch (error) {
      if (!executionStarted) {
        playbackTerminalRequestsInFlight.delete(fenceKey);
        if (!cachedEntry) {
          if (key) {
            await removeDurablePlaybackTerminalRequest(key);
            playbackReplacementRequests.delete(key);
            playbackTerminalSessionIds.delete(key);
            playbackTerminalScopes.delete(key);
          }
          if (!fenceWasAlreadySet) playbackStoppingSessions.delete(fenceKey);
        }
      }
      throw error;
    }
  };

  const castBootstrapInputMatches = (
    cached: CastBootstrapRequest,
    body: CastBootstrapInput,
    replacement?: PlaybackReplacementInput,
  ): boolean => {
    const requested = {
      ...body,
      clientInstanceId: clientInstanceId() || body.clientInstanceId,
    };
    delete requested.requestId;
    const existing = { ...cached } as Record<string, unknown>;
    delete existing.requestId;
    delete existing.replacement;
    if (JSON.stringify(existing) !== JSON.stringify(requested)) return false;
    if (!replacement) return cached.replacement === undefined;
    return (
      cached.replacement !== undefined &&
      replacementInputMatches(cached.replacement, replacement)
    );
  };
  const castTransferRequestForScope = (
    requestId: string,
    scope: PlaybackAuthorityScope,
  ): [string, CastBootstrapRequest] | undefined =>
    [...castTransferRequests.entries()].find(
      ([key, requestBody]) =>
        requestBody.requestId === requestId &&
        playbackAuthorityScopesEqual(
          castTransferScopes.get(key) ?? ["", "", "", "", ""],
          scope,
        ),
    );
  const clearCastTransferRequest = async (
    key: string,
    requestBody: CastBootstrapRequest,
    scope: PlaybackAuthorityScope,
  ): Promise<void> => {
    await removeDurablePlaybackTerminalRequest(key);
    castTransferRequests.delete(key);
    castTransferScopes.delete(key);
    const sourceSessionId = requestBody.replacement?.sourceSessionId;
    if (sourceSessionId) {
      playbackStoppingSessions.delete(
        playbackSessionFenceKey(sourceSessionId, scope),
      );
    }
  };
  const assertCastTransferStatusResponse = (
    response: CastTransferStatusResponse,
    requestBody: CastBootstrapRequest,
  ): void => {
    if (
      !response ||
      response.version !== "v1" ||
      response.requestId !== requestBody.requestId ||
      typeof response.replacementSessionId !== "string" ||
      response.replacementSessionId.length < 1 ||
      typeof response.requestFingerprint !== "string" ||
      response.requestFingerprint.length < 1 ||
      !new Set(["pending", "committed", "expired", "failed"]).has(
        response.status,
      )
    ) {
      throw new ApiError(
        0,
        "invalid_cast_transfer_status",
        "The Cast transfer status did not match its durable request.",
      );
    }
    const replacement = requestBody.replacement;
    if (!replacement) {
      if (response.sourceSessionId || response.previousTerminal) {
        throw new ApiError(
          0,
          "invalid_cast_transfer_status",
          "A fresh Cast transfer returned replacement authority evidence.",
        );
      }
      return;
    }
    if (
      response.sourceSessionId !== replacement.sourceSessionId ||
      !response.previousTerminal ||
      !terminalEventMatches(
        response.previousTerminal,
        replacement.previousTerminal,
      )
    ) {
      throw new ApiError(
        0,
        "invalid_cast_transfer_status",
        "The Cast transfer status did not prove the exact source terminal.",
      );
    }
  };
  const reconcileStoredCastTransfer = async (
    key: string,
    requestBody: CastBootstrapRequest,
    scope: PlaybackAuthorityScope,
    init?: RequestSignal,
  ): Promise<CastTransferOutcome> => {
    const sourceSessionId = requestBody.replacement?.sourceSessionId;
    let response: CastTransferStatusResponse;
    try {
      response = await request<CastTransferStatusResponse>(
        "/api/playback/cast/transfer-status",
        {
          ...init,
          method: "POST",
          body: {
            clientInstanceId: requestBody.clientInstanceId,
            requestId: requestBody.requestId,
            ...(sourceSessionId ? { sourceSessionId } : {}),
          } satisfies CastTransferStatusRequest,
        },
      );
    } catch (error) {
      if (sourceSessionId && isDefinitiveInactivePlaybackReplacement(error)) {
        await forgetClientPlaybackProgress(sourceSessionId, scope);
        await clearCastTransferRequest(key, requestBody, scope);
        return {
          outcome: "source-inactive",
          sourceSessionId,
          rejection: playbackReplacementRejection(error),
        };
      }
      throw error;
    }
    assertCastTransferStatusResponse(response, requestBody);
    if (response.status === "pending") {
      return { outcome: "pending", value: response };
    }
    if (response.status === "committed") {
      if (sourceSessionId) {
        await forgetClientPlaybackProgress(sourceSessionId, scope);
      }
      await clearCastTransferRequest(key, requestBody, scope);
      return { outcome: "accepted", value: response };
    }
    await clearCastTransferRequest(key, requestBody, scope);
    if (sourceSessionId) {
      return {
        outcome: "source-retained",
        sourceSessionId,
        value: response,
      };
    }
    return { outcome: "not-committed", value: response };
  };
  const castBootstrapErrorIsAmbiguous = (error: unknown): boolean =>
    !(error instanceof ApiError) ||
    error.status === 0 ||
    error.status === 408 ||
    error.status === 429 ||
    error.status >= 500 ||
    [
      "cast_bootstrap_pending",
      "handoff_in_progress",
      "prepared_handoff_in_progress",
    ].includes(error.code);
  const executeStoredCastBootstrap = async (
    key: string,
    requestBody: CastBootstrapRequest,
    scope: PlaybackAuthorityScope,
    init?: RequestSignal,
  ): Promise<CastTransferOutcome> => {
    try {
      const response = await request<CastBootstrapResponse>(
        "/api/playback/cast/bootstrap",
        { ...init, method: "POST", body: requestBody },
      );
      if (
        response.transferStatus !== "pending" ||
        response.requestId !== requestBody.requestId ||
        typeof response.replacementSessionId !== "string" ||
        response.replacementSessionId.length < 1 ||
        (requestBody.replacement
          ? response.sourceSessionId !==
            requestBody.replacement.sourceSessionId
          : Boolean(response.sourceSessionId))
      ) {
        throw new ApiError(
          0,
          "invalid_cast_bootstrap_response",
          "The Cast bootstrap response did not match its durable request.",
        );
      }
      return { outcome: "pending", value: response };
    } catch (error) {
      if (
        error instanceof ApiError &&
        [
          "cast_transfer_committed",
          "cast_transfer_expired",
          "cast_transfer_failed",
        ].includes(error.code)
      ) {
        return reconcileStoredCastTransfer(key, requestBody, scope, init);
      }
      if (castBootstrapErrorIsAmbiguous(error)) throw error;
      const sourceSessionId = requestBody.replacement?.sourceSessionId;
      if (sourceSessionId && isDefinitiveInactivePlaybackReplacement(error)) {
        await forgetClientPlaybackProgress(sourceSessionId, scope);
        await clearCastTransferRequest(key, requestBody, scope);
        return {
          outcome: "source-inactive",
          sourceSessionId,
          rejection: playbackReplacementRejection(error),
        };
      }
      await clearCastTransferRequest(key, requestBody, scope);
      if (sourceSessionId && error instanceof ApiError) {
        return {
          outcome: "source-retained",
          sourceSessionId,
          rejection: playbackReplacementRejection(error),
        };
      }
      throw error;
    }
  };
  const prepareCastBootstrap = async (
    body: CastBootstrapInput,
    replacementInput?: PlaybackReplacementInput,
    init?: RequestSignal,
  ): Promise<CastTransferOutcome> => {
    if (!body || typeof body !== "object") {
      throw new TypeError("Cast bootstrap input must be an object.");
    }
    if ("replacement" in body) {
      throw new TypeError(
        "Cast replacement authority must be supplied through Client Core's replacement input.",
      );
    }
    if (replacementInput) assertPlaybackReplacementRequest(replacementInput);
    await loadDurablePlaybackProgress();
    const session = await credentials.current();
    const scope = playbackAuthorityScope(session);
    const configuredClientInstanceId =
      clientInstanceId() || body.clientInstanceId || "";
    if (!configuredClientInstanceId) {
      throw new TypeError("Cast bootstrap requires a stable client instance ID.");
    }
    const cachedEntry = [...castTransferRequests.entries()].find(
      ([key, requestBody]) =>
        playbackAuthorityScopesEqual(
          castTransferScopes.get(key) ?? ["", "", "", "", ""],
          scope,
        ) &&
        requestBody.clientInstanceId === configuredClientInstanceId &&
        (body.requestId === undefined || requestBody.requestId === body.requestId) &&
        (replacementInput === undefined ||
          requestBody.replacement?.sourceSessionId ===
            replacementInput.sourceSessionId),
    );
    if (cachedEntry) {
      if (!castBootstrapInputMatches(cachedEntry[1], body, replacementInput)) {
        throw new ApiError(
          409,
          "cast_transfer_request_conflict",
          "This client instance already has a different pending Cast transfer.",
        );
      }
      return executeStoredCastBootstrap(
        cachedEntry[0],
        cachedEntry[1],
        scope,
        init,
      );
    }
    const sourceSessionId = replacementInput?.sourceSessionId;
    const fenceKey = sourceSessionId
      ? playbackSessionFenceKey(sourceSessionId, scope)
      : undefined;
    if (fenceKey && playbackStoppingSessions.has(fenceKey)) {
      throw new ApiError(
        409,
        "playback_terminal_request_conflict",
        "This playback session already has a different uncertain terminal request.",
      );
    }
    if (castTransferRequests.size >= MAX_PLAYBACK_TERMINAL_REQUESTS) {
      throw new ApiError(
        503,
        "cast_transfer_outbox_full",
        "Cast transfer recovery must finish before another transfer can begin.",
      );
    }
    if (fenceKey) playbackStoppingSessions.add(fenceKey);
    let key: string | undefined;
    let persistenceCompleted = false;
    try {
      if (
        body.requestId &&
        replacementInput?.requestId &&
        body.requestId !== replacementInput.requestId
      ) {
        throw new TypeError(
          "Cast bootstrap and native replacement request identities must match.",
        );
      }
      const requestedId = replacementInput?.requestId ?? body.requestId;
      const requestId =
        requestedId ?? createRequestId(options.requestId).slice(0, 128);
      assertPlaybackTerminalRequestId(requestId);
      let replacement: PlaybackReplacementRequest | undefined;
      if (replacementInput) {
        const previousTerminal = isCompletePlaybackTerminalEvent(
          replacementInput.previousTerminal,
        )
          ? replacementInput.previousTerminal
          : await allocatePlaybackTerminalEvent(
              replacementInput.sourceSessionId,
              replacementInput.previousTerminal,
              session,
              scope,
              init,
            );
        replacement = {
          sourceSessionId: replacementInput.sourceSessionId,
          requestId,
          previousTerminal,
          ...(replacementInput.expectedQueueRevision !== undefined
            ? {
                expectedQueueRevision:
                  replacementInput.expectedQueueRevision,
              }
            : {}),
          ...(replacementInput.expectedPlaybackRevision !== undefined
            ? {
                expectedPlaybackRevision:
                  replacementInput.expectedPlaybackRevision,
              }
            : {}),
        };
      }
      const requestBody: CastBootstrapRequest = {
        ...body,
        clientInstanceId: configuredClientInstanceId,
        requestId,
        ...(replacement ? { replacement } : {}),
      };
      assertCastBootstrapRequest(requestBody);
      key = castTransferKey(requestId, scope);
      const snapshot = immutableJSONSnapshot(requestBody);
      castTransferRequests.set(key, snapshot);
      castTransferScopes.set(key, scope);
      await persistPlaybackTerminalRequest({
        version: "v1",
        kind: "cast-transfer",
        key,
        scope,
        request: snapshot,
        updatedAt: new Date(options.now?.() ?? Date.now()).toISOString(),
      });
      persistenceCompleted = true;
      return executeStoredCastBootstrap(key, snapshot, scope, init);
    } catch (error) {
      if (key && persistenceCompleted && castTransferRequests.has(key)) {
        // A persisted or dispatched exact request owns its recovery state.
        throw error;
      }
      if (key) {
        await removeDurablePlaybackTerminalRequest(key).catch(() => undefined);
        castTransferRequests.delete(key);
        castTransferScopes.delete(key);
      }
      if (fenceKey) playbackStoppingSessions.delete(fenceKey);
      throw error;
    }
  };
  const castTransferStatus = async (
    requestId: string,
    init?: RequestSignal,
  ): Promise<CastTransferOutcome> => {
    assertPlaybackTerminalRequestId(requestId);
    await loadDurablePlaybackProgress();
    const scope = playbackAuthorityScope(await credentials.current());
    const stored = castTransferRequestForScope(requestId, scope);
    if (!stored) {
      throw new ApiError(
        404,
        "cast_transfer_request_not_pending",
        "Cast has no pending transfer with this request identity.",
      );
    }
    return reconcileStoredCastTransfer(stored[0], stored[1], scope, init);
  };
  const retryPendingCastTransfer = async (
    requestId: string,
    init?: RequestSignal,
  ): Promise<CastTransferOutcome> => {
    const status = await castTransferStatus(requestId, init);
    if (status.outcome !== "pending") return status;
    const scope = playbackAuthorityScope(await credentials.current());
    const stored = castTransferRequestForScope(requestId, scope);
    if (!stored) return status;
    return executeStoredCastBootstrap(stored[0], stored[1], scope, init);
  };
  const pendingCastTransfers = async (): Promise<readonly PendingCastTransfer[]> => {
    await loadDurablePlaybackProgress();
    const scope = playbackAuthorityScope(await credentials.current());
    return immutableJSONSnapshot(
      [...castTransferRequests.entries()]
        .filter(([key]) =>
          playbackAuthorityScopesEqual(
            castTransferScopes.get(key) ?? ["", "", "", "", ""],
            scope,
          ),
        )
        .map(([, requestBody]) => ({
          requestId: requestBody.requestId,
          ...(requestBody.replacement
            ? { sourceSessionId: requestBody.replacement.sourceSessionId }
            : {}),
        }))
        .sort((left, right) => left.requestId.localeCompare(right.requestId)),
    );
  };
  const createCastBootstrap = (
    body: CastBootstrapInput,
    init?: RequestSignal,
  ): Promise<CastTransferOutcome> =>
    prepareCastBootstrap(body, undefined, init);
  const replacePlaybackWithCast = (
    body: CastBootstrapInput,
    replacement: PlaybackReplacementInput,
    init?: RequestSignal,
  ): Promise<CastTransferOutcome> =>
    prepareCastBootstrap(body, replacement, init);

  const pendingPlaybackTerminalMutationRecords =
    (scope: PlaybackAuthorityScope): readonly PendingPlaybackTerminalMutationRecord[] => {
      const records: PendingPlaybackTerminalMutationRecord[] = [];
      for (const [key, request] of playbackTerminalRequests) {
        const recordScope = playbackTerminalScopes.get(key);
        if (!recordScope || !playbackAuthorityScopesEqual(recordScope, scope)) continue;
        const sessionId = playbackTerminalSessionIds.get(key);
        if (!sessionId) {
          throw new ApiError(
            409,
            "playback_terminal_outbox_conflict",
            "A pending playback stop has no source session identity.",
          );
        }
        records.push({ sessionId, operation: "stop", request });
      }
      for (const [key, request] of playbackHandoffRequests) {
        const recordScope = playbackTerminalScopes.get(key);
        if (!recordScope || !playbackAuthorityScopesEqual(recordScope, scope)) continue;
        const sessionId = playbackTerminalSessionIds.get(key);
        if (!sessionId) {
          throw new ApiError(
            409,
            "playback_terminal_outbox_conflict",
            "A pending playback handoff has no source session identity.",
          );
        }
        records.push({ sessionId, operation: "handoff", request });
      }
      for (const [key, close] of playbackLiveTvCloseRequests) {
        const recordScope = playbackTerminalScopes.get(key);
        if (!recordScope || !playbackAuthorityScopesEqual(recordScope, scope)) continue;
        const sessionId = playbackTerminalSessionIds.get(key);
        if (!sessionId) {
          throw new ApiError(
            409,
            "playback_terminal_outbox_conflict",
            "A pending Live TV close has no source session identity.",
          );
        }
        records.push({
          sessionId,
          operation: "live-tv-close",
          channelId: close.channelId,
          request: close.request,
        });
      }
      for (const [key, replacement] of playbackReplacementRequests) {
        const recordScope = playbackTerminalScopes.get(key);
        if (!recordScope || !playbackAuthorityScopesEqual(recordScope, scope)) continue;
        const sessionId = playbackTerminalSessionIds.get(key);
        if (!sessionId) {
          throw new ApiError(
            409,
            "playback_terminal_outbox_conflict",
            "A pending playback replacement has no source session identity.",
          );
        }
        records.push({
          sessionId,
          operation: "replacement",
          request: replacement.request,
          target: replacement.target,
          ...(replacement.committedOutcome
            ? { committedOutcome: replacement.committedOutcome }
            : {}),
        });
      }
      records.sort((left, right) =>
        left.sessionId.localeCompare(right.sessionId) ||
        left.operation.localeCompare(right.operation) ||
        left.request.requestId.localeCompare(right.request.requestId),
      );
      const sessions = new Set<string>();
      for (const record of records) {
        if (sessions.has(record.sessionId)) {
          throw new ApiError(
            409,
            "playback_terminal_outbox_conflict",
            "Playback has more than one pending terminal mutation.",
          );
        }
        sessions.add(record.sessionId);
      }
      return immutableJSONSnapshot(records);
    };

  const restoreActivePlayback = (
    intent?: PlaybackIntent,
    init?: RequestSignal,
  ) =>
    request<PlaybackRestoreResponse>("/api/playback/active", {
      ...init,
      method: "POST",
      body: {
        clientInstanceId: clientInstanceId(),
        clientProfile: profile(),
        intent,
      },
    }).then((response) => ({
      ...response,
      playback: response.playback
        ? normalizeSessionPlayback(response.playback)
        : undefined,
    }));
  const restoreCommittedPlaybackReplacement = async (
    outcome: Extract<
      PlaybackReplacementOutcome<unknown>,
      { outcome: "committed-restore-required" }
    >,
    intent?: PlaybackIntent,
    init?: RequestSignal,
  ): Promise<PlaybackResponse> => {
    await loadDurablePlaybackProgress();
    const scope = playbackAuthorityScope(await credentials.current());
    const durableCommittedEntry = [...playbackReplacementRequests.entries()].find(
      ([key, replacement]) =>
        replacement.committedOutcome?.sourceSessionId ===
          outcome.sourceSessionId &&
        replacement.committedOutcome.replacementSessionId ===
          outcome.replacementSessionId &&
        playbackAuthorityScopesEqual(
          playbackTerminalScopes.get(key) ?? ["", "", "", "", ""],
          scope,
        ),
    );
    const restored = await restoreActivePlayback(intent, init);
    if (restored.playback?.sessionId !== outcome.replacementSessionId) {
      throw new ApiError(
        409,
        "playback_replacement_restore_mismatch",
        "The active playback restore did not match the committed replacement.",
        { replacementSessionId: outcome.replacementSessionId },
      );
    }
    if (durableCommittedEntry) {
      const [key] = durableCommittedEntry;
      await clearPlaybackReplacementRequest(
        key,
        playbackSessionFenceKey(outcome.sourceSessionId, scope),
      );
    }
    return restored.playback;
  };

  const retryPendingPlaybackTerminalMutation = async (
    sessionId: string,
    init?: RequestSignal,
  ): Promise<PendingPlaybackTerminalRetryOutcome> => {
    await loadDurablePlaybackProgress();
    const scope = playbackAuthorityScope(await credentials.current());
    const record = pendingPlaybackTerminalMutationRecords(scope).find(
      (candidate) => candidate.sessionId === sessionId,
    );
    if (!record) {
      throw new ApiError(
        404,
        "playback_terminal_request_not_pending",
        "Playback has no pending terminal mutation to retry.",
      );
    }
    if (record.operation === "stop") {
      return {
        outcome: "accepted",
        value: await stopPlaybackSession(sessionId, record.request, init),
      };
    }
    if (record.operation === "live-tv-close") {
      return {
        outcome: "accepted",
        value: await closeLiveTvStream(
          record.channelId,
          sessionId,
          record.request,
          init,
        ),
      };
    }
    const fenceKey = playbackSessionFenceKey(sessionId, scope);
    const key = playbackTerminalKey(
      record.operation,
      sessionId,
      record.request.requestId,
      scope,
    );
    if (record.operation === "replacement") {
      const cached = playbackReplacementRequests.get(key);
      if (!cached) {
        throw new ApiError(
          409,
          "playback_terminal_outbox_conflict",
          "The pending playback replacement could not be resolved exactly.",
        );
      }
      return executePlaybackReplacement(key, cached, scope, init);
    }
    const cached = playbackHandoffRequests.get(key);
    if (!cached) {
      throw new ApiError(
        409,
        "playback_terminal_outbox_conflict",
        "The pending playback handoff could not be resolved exactly.",
      );
    }
    if (playbackTerminalRequestsInFlight.has(fenceKey)) {
      throw new ApiError(
        409,
        "playback_stopping",
        "Playback already has a terminal mutation in flight.",
      );
    }
    playbackTerminalRequestsInFlight.add(fenceKey);
    let requestDispatched = false;
    try {
      requestDispatched = true;
      const response = normalizeSessionPlayback(
        await request<PlaybackResponse>(
          `/api/playback-sessions/${encodeURIComponent(sessionId)}/handoff`,
          { ...init, method: "POST", body: cached },
        ),
      );
      await forgetClientPlaybackProgress(sessionId, scope);
      await removeDurablePlaybackTerminalRequest(key);
      playbackHandoffRequests.delete(key);
      playbackTerminalSessionIds.delete(key);
      playbackTerminalScopes.delete(key);
      playbackStoppingSessions.delete(fenceKey);
      return { outcome: "accepted", value: response };
    } catch (error) {
      if (
        (!requestDispatched && !cached) ||
        terminalMutationRejectionIsDefinitive(error)
      ) {
        await removeDurablePlaybackTerminalRequest(key);
        playbackHandoffRequests.delete(key);
        playbackTerminalSessionIds.delete(key);
        playbackTerminalScopes.delete(key);
        playbackStoppingSessions.delete(fenceKey);
      }
      throw error;
    } finally {
      playbackTerminalRequestsInFlight.delete(fenceKey);
    }
  };

  return {
    request,
    formRequest,
    // Adapters that use a route-specific request shape can still publish the
    // authoritative session generation/sequence into Client Core before they
    // enqueue durable progress.
    acceptPlaybackSession: normalizeSessionPlayback,
    replacePlaybackTarget,
    retryPendingPlaybackTerminalMutation,
    pendingPlaybackTerminalMutation: async (
      sessionId: string,
    ): Promise<PendingPlaybackTerminalMutation | undefined> => {
      await loadDurablePlaybackProgress();
      const scope = playbackAuthorityScope(await credentials.current());
      const record = pendingPlaybackTerminalMutationRecords(scope).find(
        (candidate) => candidate.sessionId === sessionId,
      );
      if (!record) return undefined;
      return record.operation === "stop"
        ? immutableJSONSnapshot({ operation: "stop", request: record.request })
        : record.operation === "handoff"
          ? immutableJSONSnapshot({
            operation: "handoff",
            request: record.request,
          })
          : record.operation === "live-tv-close"
            ? immutableJSONSnapshot({
                operation: "live-tv-close",
                channelId: record.channelId,
                request: record.request,
              })
            : immutableJSONSnapshot({
                operation: "replacement",
                request: record.request,
                target: record.target,
                ...(record.committedOutcome
                  ? { committedOutcome: record.committedOutcome }
                  : {}),
              });
    },
    pendingPlaybackTerminalMutations: async (): Promise<
      readonly PendingPlaybackTerminalMutationRecord[]
    > => {
      await loadDurablePlaybackProgress();
      const scope = playbackAuthorityScope(await credentials.current());
      return pendingPlaybackTerminalMutationRecords(scope);
    },
    apiUrl,
    resourceUrl,
    imageResourceUrl,
    streamAppEvents: (
      signal: AbortSignal,
      onEvent: (event: AppEvent) => void,
    ) => streamAppEvents(options, credentials, signal, onEvent),
    subscribeAppEvents: (
      subscription: PorticoEventSubscriptionOptions<AppEvent>,
    ) =>
      eventSubscriptions.run("app-events", subscription.signal, (signal) =>
        subscribeAppEventTransport(options, credentials, {
          ...subscription,
          signal,
        }),
      ),
    system: () => request<SystemStatusResponse>("/api/system"),
    checkServerCompatibility: async (init?: Pick<RequestInit, "signal">) =>
      assertServerAPICompatibility(
        await request<SystemStatusResponse>("/api/system", init),
      ),
    branding: () => request<BrandingInfo>("/api/branding"),
    authCapabilities: () =>
      request<AuthCapabilitiesResponse>("/api/auth/capabilities"),
    me: (init?: Pick<RequestInit, "signal">) =>
      request<AuthMeResponse>("/api/auth/me", init),
    setup: (body: {
      username: string;
      email: string;
      displayName: string;
      password: string;
      setupMode?: "local_only";
      localOnlyAcknowledged?: boolean;
    }) => request<AuthMeResponse>("/api/auth/setup", { method: "POST", body }),
    startPorticoSetupClaim: () =>
      request<{ remoteAccess: RemoteAccessStatus }>(
        "/api/auth/portico-setup/claim/start",
        { method: "POST" },
      ),
    porticoSetupStatus: () =>
      request<{ setupRequired: boolean; remoteAccess: RemoteAccessStatus }>(
        "/api/auth/portico-setup/status",
      ),
    login: (body: { login: string; password: string }) =>
      request<AuthMeResponse>("/api/auth/login", { method: "POST", body }),
    authenticateLocalProfileAccount: async (
      body: LocalProfileAccountAuthenticationRequest,
      init?: Pick<RequestInit, "signal">,
    ) => {
      const response = await request<ProfileAccountAuthenticationResponse>(
        "/api/auth/profile-authentications/local",
        { ...init, method: "POST", body },
      );
      return {
        ...response,
        directory: parseProfileDirectory(response.directory),
      };
    },
    selectLocalProfile: async (
      body: LocalProfileSelectionRequest,
      init?: Pick<RequestInit, "signal">,
    ) =>
      parseProfileSelectionGrant(
        await request<ProfileSelectionResponse>(
          "/api/auth/profile-selections/local",
          { ...init, method: "POST", body },
        ),
      ),
    selectActiveLocalProfile: async (
      body: import("./types.js").ActiveLocalProfileSelectionRequest,
      init?: Pick<RequestInit, "signal">,
    ) =>
      parseProfileSelectionGrant(
        await request<ProfileSelectionResponse>(
          "/api/auth/profile-selections/session",
          { ...init, method: "POST", body },
        ),
      ),
    createBrowserProfileSession: (
      body: BrowserProfileSessionRequest,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<AuthMeResponse>("/api/auth/profile-sessions/browser", {
        ...init,
        method: "POST",
        body,
      }),
    createNativeProfileSession: async (body: NativeProfileSessionRequest) => {
      const created = await request<NativeSessionCredentials>(
        "/api/auth/profile-sessions/native",
        { method: "POST", body },
      );
      await credentials.accept(created);
      return created;
    },
    createNativeSession: async (body: NativeSessionCreateRequest) => {
      const created = await request<NativeSessionCredentials>(
        "/api/auth/sessions",
        { method: "POST", body },
      );
      await credentials.accept(created);
      return created;
    },
    createPorticoAttachmentHandshake: (
      body: PorticoAttachmentHandshakeRequest,
      init?: RequestSignal,
    ) =>
      requestWithoutRefresh<PorticoAttachmentHandshakeResponse>(
        "/api/auth/portico/handshakes",
        { ...init, method: "POST", body, authorization: { mode: "anonymous" } },
      ),
    exchangeEncryptedPorticoSession: (
      body: PorticoAttachmentEncryptedRequest,
      init?: RequestSignal,
    ) => encryptedAttachmentRequest(body, init ?? {}),
    acceptPorticoSessionCredentials: async (
      created: NativeSessionCredentials,
    ) => {
      await credentials.accept(created);
      return created;
    },
    refreshNativeSession: (refreshToken: string, init: RequestSignal = {}) => {
      const transportOptions: TransportRequestOptions = {
        ...init,
        method: "POST",
      };
      const requestDeadline = transportDeadlineAt(options, transportOptions);
      return runTransportOperation(
        options,
        transportOptions,
        "POST",
        (context) =>
          credentials.refreshExplicit(refreshToken, context.signal, () => {
            context.dispatched = true;
          }),
        undefined,
        requestDeadline,
      );
    },
    revokeNativeSession: async (refreshToken: string) => {
      const response = await requestWithoutRefresh<{ ok: boolean }>(
        "/api/auth/sessions/revoke",
        { method: "POST", body: { refreshToken } },
      );
      await credentials.clearIfOwned(refreshToken);
      return response;
    },
    startQuickConnect: (body: {
      installationId?: string;
      deviceName?: string;
      app?: string;
      platform?: string;
    }) =>
      request<QuickConnectStartResponse>("/api/auth/quick-connect/start", {
        method: "POST",
        body,
      }),
    quickConnectStatus: (secret: string) =>
      request<QuickConnectStatusResponse>("/api/auth/quick-connect/status", {
        method: "POST",
        body: { secret },
      }),
    exchangeQuickConnect: async (secret: string) => {
      const created = await request<NativeSessionCredentials>(
        "/api/auth/quick-connect/exchange",
        { method: "POST", body: { secret } },
      );
      await credentials.accept(created);
      return created;
    },
    pendingQuickConnect: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<QuickConnectRequest>>(
        "/api/auth/quick-connect/pending",
        init,
      ),
    authorizeQuickConnect: (code: string) =>
      request<QuickConnectRequest>("/api/auth/quick-connect/authorize", {
        method: "POST",
        body: { code },
      }),
    denyQuickConnect: (code: string) =>
      request<QuickConnectRequest>("/api/auth/quick-connect/deny", {
        method: "POST",
        body: { code },
      }),
    logout: () =>
      request<{ ok: boolean }>("/api/auth/logout", { method: "POST" }),
    apiKeys: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<APIKey>>("/api/auth/api-keys", init),
    createAPIKey: (body: { name: string; scopes: string[] }) =>
      request<APIKeyCreateResponse>("/api/auth/api-keys", {
        method: "POST",
        body,
      }),
    revokeAPIKey: (id: string) =>
      request<APIKey>(`/api/auth/api-keys/${encodeURIComponent(id)}`, {
        method: "DELETE",
      }),
    updateProfile: (body: {
      displayName: string;
      email: string;
      profileImageUrl?: string;
      preferences?: UserPreferences;
    }) => request<User>("/api/account/profile", { method: "PATCH", body }),
    changeAccountPassword: (
      body: AccountPasswordChangeRequest,
      init?: RequestSignal,
    ) =>
      request<SuccessResponse>("/api/account/password", {
        ...init,
        method: "POST",
        body,
      }),
    viewerPreferenceBundle: (
      params: ApiOperationQuery<"getViewerPreferenceBundle">,
      init?: RequestSignal,
    ) =>
      request<ServerViewerPreferenceBundle>(
        `/api/preferences${apiQuery(params)}`,
        init,
      ),
    patchViewerPreferenceDocument: (
      scopeType: ApiOperationPath<"patchViewerPreferenceDocument">["scopeType"],
      params: ApiOperationQuery<"patchViewerPreferenceDocument">,
      body: ServerViewerPreferencePatch,
      init?: RequestSignal,
    ) => {
      if (
        scopeType === "account-server-installation" &&
        Object.prototype.hasOwnProperty.call(body.changes, "lastProfileId")
      ) {
        throw new TypeError(
          "lastProfileId is updated only after authoritative profile activation",
        );
      }
      return request<ServerPatchedPreferenceDocument>(
        `/api/preferences/${encodeURIComponent(scopeType)}${apiQuery(params)}`,
        { ...init, method: "PATCH", body },
      );
    },
    recordViewerProfileActivation: (
      body: ServerViewerProfileActivationRequest,
      init?: RequestSignal,
    ) =>
      request<ServerAccountInstallationPreferenceDocument>(
        "/api/preferences/profile-activation",
        { ...init, method: "POST", body },
      ),
    accountProfiles: async (init?: RequestSignal) =>
      normalizeManagedProfileDirectory(
        await request<ServerManagedProfileDirectory>(
          "/api/account/profiles",
          init,
        ),
      ),
    createProfileAdministrationProof: (
      body: ServerProfileAdministrationProofRequest,
      init?: RequestSignal,
    ) =>
      request<ServerProfileAdministrationProofResponse>(
        "/api/account/profile-admin-proofs",
        { ...init, method: "POST", body },
      ),
    createAccountProfile: async (
      body: ServerManagedProfileCreateRequest,
      proof: string,
      init?: RequestSignal,
    ) =>
      parsePorticoProfile(
        await request<import("./types.js").SelectableProfile>(
          "/api/account/profiles",
          {
            ...init,
            method: "POST",
            authorization: { mode: "profile-admin-proof", proof },
            body,
          },
        ),
      ),
    updateAccountProfile: async (
      profileId: string,
      body: ServerManagedProfileUpdateRequest,
      proof: string,
      init?: RequestSignal,
    ) =>
      parsePorticoProfile(
        await request<import("./types.js").SelectableProfile>(
          `/api/account/profiles/${encodeURIComponent(profileId)}`,
          {
            ...init,
            method: "PATCH",
            authorization: { mode: "profile-admin-proof", proof },
            body,
          },
        ),
      ),
    deleteAccountProfile: (
      profileId: string,
      proof: string,
      init?: RequestSignal,
    ) =>
      request<ServerProfileErasureResponse>(
        `/api/account/profiles/${encodeURIComponent(profileId)}`,
        {
          ...init,
          method: "DELETE",
          authorization: { mode: "profile-admin-proof", proof },
        },
      ),
    reorderAccountProfiles: async (
      body: ApiOperationJSONBody<"reorderAccountProfiles">,
      proof: string,
      init?: RequestSignal,
    ) =>
      normalizeManagedProfileDirectory(
        await request<ServerManagedProfileDirectory>(
          "/api/account/profiles/order",
          {
            ...init,
            method: "PUT",
            authorization: { mode: "profile-admin-proof", proof },
            body,
          },
        ),
      ),
    setAccountProfilePIN: (
      profileId: string,
      body: ApiOperationJSONBody<"setAccountProfilePin">,
      proof: string,
      init?: RequestSignal,
    ) =>
      request<void>(
        `/api/account/profiles/${encodeURIComponent(profileId)}/pin`,
        {
          ...init,
          method: "PUT",
          authorization: { mode: "profile-admin-proof", proof },
          body,
        },
      ),
    clearAccountProfilePIN: (
      profileId: string,
      body: ApiOperationJSONBody<"clearAccountProfilePin">,
      proof: string,
      init?: RequestSignal,
    ) =>
      request<void>(
        `/api/account/profiles/${encodeURIComponent(profileId)}/pin`,
        {
          ...init,
          method: "DELETE",
          authorization: { mode: "profile-admin-proof", proof },
          body,
        },
      ),
    createAutomaticProfileTrust: async (
      body: ServerAutomaticProfileTrustRequest,
      init?: RequestSignal,
    ) =>
      parseAutomaticProfileTrust(
        await request<ServerAutomaticProfileTrust>(
          "/api/account/profile-trusts",
          { ...init, method: "POST", body },
        ),
      ),
    redeemAutomaticProfileTrust: (
      body: ApiOperationJSONBody<"redeemAutomaticProfileTrust">,
      init?: RequestSignal,
    ) =>
      request<void>("/api/account/profile-trusts/redeem", {
        ...init,
        method: "POST",
        body,
      }),
    revokeAutomaticProfileTrusts: (
      body: ServerAutomaticProfileTrustRequest,
      init?: RequestSignal,
    ) =>
      request<void>("/api/account/profile-trusts", {
        ...init,
        method: "DELETE",
        body,
      }),
    viewerNotifications: async (
      params: ApiOperationQuery<"listViewerNotifications"> = {},
      init?: RequestSignal,
    ) =>
      parseNotificationPage(
        await request<ServerViewerNotificationPage>(
          `/api/notifications${apiQuery(params)}`,
          init,
        ),
      ),
    streamViewerNotificationInvalidations: (
      params: ApiOperationQuery<"streamViewerNotificationInvalidations">,
      signal: AbortSignal,
      onInvalidation: (event: NotificationInvalidation) => void,
    ) =>
      streamViewerNotificationInvalidations(
        options,
        credentials,
        params,
        signal,
        onInvalidation,
      ),
    subscribeViewerNotificationInvalidations: (
      params: ApiOperationQuery<"streamViewerNotificationInvalidations">,
      subscription: PorticoEventSubscriptionOptions<NotificationInvalidation>,
    ) =>
      eventSubscriptions.run(
        `notification-events:${String(params.audience ?? "profile")}`,
        subscription.signal,
        (signal) =>
          subscribeNotificationEventTransport(options, credentials, params, {
            ...subscription,
            signal,
          }),
      ),
    updateViewerNotificationReceipts: async (
      body: ServerNotificationReceiptMutation,
      params: ApiOperationQuery<"updateViewerNotificationReceipts"> = {},
      init?: RequestSignal,
    ) => {
      const normalized = normalizeNotificationReceiptMutation(body);
      return parseNotificationReceiptResult(
        await request<ServerNotificationReceiptResult>(
          `/api/notifications/receipts${apiQuery(params)}`,
          { ...init, method: "POST", body: normalized },
        ),
        normalized.recipient,
      );
    },
    markAllViewerNotificationsRead: async (
      params: ApiOperationQuery<"markAllViewerNotificationsRead"> = {},
      init?: RequestSignal,
    ) =>
      parseNotificationReadAllResult(
        await request<ServerNotificationReadAllResult>(
          `/api/notifications/read-all${apiQuery(params)}`,
          { ...init, method: "POST" },
        ),
      ),
    viewerFeedbackCapabilities: async (init?: RequestSignal) =>
      parseViewerFeedbackCapabilities(
        await request<ServerViewerFeedbackCapabilities>(
          "/api/feedback/capabilities",
          init,
        ),
      ),
    submitViewerFeedback: async (
      body: ServerViewerFeedbackSubmission,
      init?: RequestSignal,
    ) =>
      parseViewerFeedbackReceipt(
        await request<ServerViewerFeedbackReceipt>("/api/feedback", {
          ...init,
          method: "POST",
          body: normalizeViewerFeedbackSubmission(body),
        }),
      ),
    ownerViewerFeedback: async (
      params: ApiOperationQuery<"listOwnerViewerFeedback"> = {},
      init?: RequestSignal,
    ) =>
      parseViewerFeedbackPage(
        await request<ServerOwnerFeedbackPage>(
          `/api/admin/viewer-feedback${apiQuery(params)}`,
          init,
        ),
      ),
    ownerViewerNotificationRecipients: async (init?: RequestSignal) =>
      parseOwnerNotificationRecipientDirectory(
        await request<ServerOwnerNotificationRecipientDirectory>(
          "/api/admin/viewer-notification-recipients",
          init,
        ),
      ),
    updateOwnerViewerFeedback: async (
      feedbackId: string,
      body: ServerOwnerFeedbackUpdateRequest,
      init?: RequestSignal,
    ) =>
      parseViewerFeedbackRecord(
        await request<ServerOwnerFeedbackRecord>(
          `/api/admin/viewer-feedback/${encodeURIComponent(feedbackId)}`,
          {
            ...init,
            method: "PATCH",
            body: normalizeViewerFeedbackAdminUpdate(body),
          },
        ),
      ),
    createOwnerViewerNotice: async (
      body: ServerOwnerNoticeRequest,
      init?: RequestSignal,
    ) =>
      parseViewerNotification(
        await request<ServerViewerNotification>(
          "/api/admin/viewer-notifications",
          { ...init, method: "POST", body },
        ),
      ),
    accountSessions: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<AccountSession>>("/api/account/sessions", init),
    revokeAccountSession: (id: string) =>
      request<{ ok: boolean }>(
        `/api/account/sessions/${encodeURIComponent(id)}`,
        { method: "DELETE" },
      ),
    clearWatchHistory: () =>
      request<{ ok: boolean; clearedAt: string }>(
        "/api/account/watch-history",
        { method: "DELETE" },
      ),
    displayPreferences: (
      client: string,
      view: string,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<DisplayPreference>(
        `/api/display-preferences/${encodeURIComponent(client)}/${encodeURIComponent(view)}`,
        init,
      ),
    updateDisplayPreferences: (
      client: string,
      view: string,
      body: DisplayPreferenceRequest,
    ) =>
      request<DisplayPreference>(
        `/api/display-preferences/${encodeURIComponent(client)}/${encodeURIComponent(view)}`,
        { method: "PATCH", body },
      ),
    uploadProfileImage: (form: FormData, init?: RequestSignal) =>
      formRequest<User>("/api/account/image", form, "POST", init),
    deleteProfileImage: () =>
      request<User>("/api/account/image", { method: "DELETE" }),
    systemIdentity: () => request<SystemIdentity>("/api/system/identity"),
    resetSystemIdentity: () =>
      request<SystemIdentity>("/api/system/identity/reset", { method: "POST" }),
    serverCapabilities: () =>
      request<ServerCapabilitiesResponse>("/api/system/capabilities"),
    systemDiagnostics: (init?: Pick<RequestInit, "signal">) =>
      request<SystemDiagnostics>("/api/system/diagnostics", init),
    systemStartup: () => request<StartupDiagnostics>("/api/system/startup"),
    systemTime: () => request<SystemTimeSync>("/api/system/time"),
    systemRelease: (init?: Pick<RequestInit, "signal">) =>
      request<SystemReleaseInfo>("/api/system/release", init),
    systemStorage: (
      params?: { refresh?: boolean },
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<SystemStorageReport>(
        `/api/system/storage${params?.refresh ? "?refresh=true" : ""}`,
        init,
      ),
    storagePaths: (init?: Pick<RequestInit, "signal">) =>
      request<StoragePathsResponse>("/api/system/storage/paths", init),
    updateStoragePaths: (body: {
      databasePath: string;
      backupDirectory: string;
      copyDatabase: boolean;
    }) =>
      request<StoragePathsResponse>("/api/system/storage/paths", {
        method: "PATCH",
        body,
      }),
    cleanupStorage: () =>
      request<SystemStorageCleanupResponse>("/api/system/storage/cleanup", {
        method: "POST",
      }),
    transcodeCapacity: (init?: Pick<RequestInit, "signal">) =>
      request<TranscodeCapacityReport>("/api/transcode/capacity", init),
    networkConnectionInfo: (init?: Pick<RequestInit, "signal">) =>
      request<NetworkConnectionInfo>("/api/network/connection-info", init),
    remoteAccessHealth: () =>
      request<RemoteAccessHealthResponse>("/api/remote-access/health"),
    remoteAccessStatus: (init?: Pick<RequestInit, "signal">) =>
      request<RemoteAccessStatus>("/api/remote-access/status", init),
    updateRemoteAccessSettings: (body: RemoteAccessSettingsPatch) =>
      request<RemoteAccessStatus>("/api/remote-access/settings", {
        method: "PATCH",
        body,
      }),
    startRemoteAccessClaim: () =>
      request<RemoteAccessStatus>("/api/remote-access/claim/start", {
        method: "POST",
      }),
    cancelRemoteAccessClaim: () =>
      request<RemoteAccessStatus>("/api/remote-access/claim/cancel", {
        method: "POST",
      }),
    unclaimRemoteAccess: () =>
      request<RemoteAccessStatus>("/api/remote-access/unclaim", {
        method: "POST",
      }),
    testRemoteAccessDirect: () =>
      request<RemoteAccessStatus>("/api/remote-access/test-direct", {
        method: "POST",
      }),
    renewRemoteAccessCertificate: () =>
      request<RemoteAccessStatus>("/api/remote-access/certificates/renew", {
        method: "POST",
      }),
    mapRemoteAccessMember: (id: string, body: { localUserId: string }) =>
      request<RemoteAccessStatus>(
        `/api/remote-access/members/${encodeURIComponent(id)}`,
        { method: "PATCH", body },
      ),
    dlnaStatus: (init?: Pick<RequestInit, "signal">) =>
      request<DLNAStatus>("/api/dlna/status", init),
    productContract: (init?: Pick<RequestInit, "signal">) =>
      request<ProductContract>("/api/product-contract", init),
    productLanguage: (locale = "en-US", init?: Pick<RequestInit, "signal">) =>
      request<ProductLanguageCatalog>(
        `/api/product-language/${encodeURIComponent(locale)}`,
        init,
      ),
    checkProductContractCompatibility: async (
      init?: Pick<RequestInit, "signal">,
    ) =>
      assertProductContractCompatibility(
        await request<ProductContract>("/api/product-contract", init),
      ),
    checkCompatibility: async (init?: Pick<RequestInit, "signal">) => {
      const status = await request<SystemStatusResponse>("/api/system", init);
      const contract = await request<ProductContract>(
        "/api/product-contract",
        init,
      );
      return evaluatePorticoCompatibility(status, contract);
    },
    browseFilesystem: (path?: string) =>
      request<FilesystemBrowseResponse>(
        `/api/filesystem/browse${path ? `?path=${encodeURIComponent(path)}` : ""}`,
      ),
    createFilesystemDirectory: (body: FilesystemCreateDirectoryRequest) =>
      request<FilesystemBrowseResponse>("/api/filesystem/directories", {
        method: "POST",
        body,
      }),
    libraries: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<Library>>("/api/libraries", init),
    libraryNavigation: (init?: Pick<RequestInit, "signal">) =>
      request<import("./types.js").LibraryNavigationPreferences>(
        "/api/account/library-navigation",
        init,
      ),
    updateLibraryNavigation: (
      body: import("./types.js").LibraryNavigationPreferencesRequest,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<import("./types.js").LibraryNavigationPreferences>(
        "/api/account/library-navigation",
        { ...init, method: "PATCH", body },
      ),
    createLibrary: (body: {
      name: string;
      type: string;
      path?: string;
      paths: string[];
      settings?: Record<string, unknown>;
    }) => request<Library>("/api/libraries", { method: "POST", body }),
    library: (id: string) =>
      request<Library>(`/api/libraries/${encodeURIComponent(id)}`),
    updateLibrary: (
      id: string,
      body: {
        name: string;
        type: string;
        path?: string;
        paths: string[];
        settings?: Record<string, unknown>;
      },
    ) =>
      request<Library>(`/api/libraries/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body,
      }),
    deleteLibrary: (id: string) =>
      request<{ ok: boolean }>(`/api/libraries/${encodeURIComponent(id)}`, {
        method: "DELETE",
      }),
    remoteStorageSources: (id: string, init?: Pick<RequestInit, "signal">) =>
      request<RemoteStorageSourceListResponse>(
        `/api/libraries/${encodeURIComponent(id)}/remote-storage-sources`,
        init,
      ),
    createRemoteStorageSource: (
      id: string,
      body: RemoteStorageSourceRequest,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<RemoteStorageSource>(
        `/api/libraries/${encodeURIComponent(id)}/remote-storage-sources`,
        { ...init, method: "POST", body },
      ),
    deleteRemoteStorageSource: (
      id: string,
      sourceId: string,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<void>(
        `/api/libraries/${encodeURIComponent(id)}/remote-storage-sources/${encodeURIComponent(sourceId)}`,
        { ...init, method: "DELETE" },
      ),
    updateRemoteStorageSourceAnalysisMode: (
      id: string,
      sourceId: string,
      body: RemoteStorageAnalysisModeRequest,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<RemoteStorageSource>(
        `/api/libraries/${encodeURIComponent(id)}/remote-storage-sources/${encodeURIComponent(sourceId)}`,
        { ...init, method: "PATCH", body },
      ),
    inventoryRemoteStorageSource: (
      id: string,
      sourceId: string,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<Job>(
        `/api/libraries/${encodeURIComponent(id)}/remote-storage-sources/${encodeURIComponent(sourceId)}/inventory`,
        { ...init, method: "POST" },
      ),
    libraryBrowseCapabilities: (
      id: string,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<LibraryBrowseCapabilities>(
        `/api/libraries/${encodeURIComponent(id)}/browse-capabilities`,
        init,
      ),
    libraryPivotBrowseCapabilities: (
      id: string,
      pivot: string,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<LibraryBrowseCapabilities>(
        `/api/libraries/${encodeURIComponent(id)}/browse-capabilities?pivot=${encodeURIComponent(pivot)}`,
        init,
      ),
    browseLibrary: (
      id: string,
      body: BrowseLibraryRequest,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<BrowseLibraryResponse>(
        `/api/libraries/${encodeURIComponent(id)}/browse`,
        { ...init, method: "POST", body },
      ),
    libraryDiscover: (
      id: string,
      params: { limit?: number } = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      const query = search.toString();
      return request<SuggestionsResponse>(
        `/api/libraries/${encodeURIComponent(id)}/discover${query ? `?${query}` : ""}`,
        init,
      );
    },
    libraryCategories: (id: string, init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<LibraryCategory>>(
        `/api/libraries/${encodeURIComponent(id)}/categories`,
        init,
      ),
    libraryAuthors: (
      id: string,
      params: { limit?: number; cursor?: string } = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      if (params.cursor) search.set("cursor", params.cursor);
      const query = search.toString();
      return request<
        CursorListResponse<import("./types.js").LibraryFacetValue>
      >(
        `/api/libraries/${encodeURIComponent(id)}/authors${query ? `?${query}` : ""}`,
        init,
      );
    },
    librarySeries: (
      id: string,
      params: { limit?: number; cursor?: string } = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      if (params.cursor) search.set("cursor", params.cursor);
      const query = search.toString();
      return request<
        CursorListResponse<import("./types.js").LibraryFacetValue>
      >(
        `/api/libraries/${encodeURIComponent(id)}/series${query ? `?${query}` : ""}`,
        init,
      );
    },
    librarySources: (
      id: string,
      params: { limit?: number; cursor?: string } = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      if (params.cursor) search.set("cursor", params.cursor);
      const query = search.toString();
      return request<CursorListResponse<LibrarySourceGroup>>(
        `/api/libraries/${encodeURIComponent(id)}/sources${query ? `?${query}` : ""}`,
        init,
      );
    },
    scanLibrary: (id: string, body: LibraryScanRequest) =>
      request<Job>(`/api/libraries/${encodeURIComponent(id)}/scan`, {
        method: "POST",
        body,
      }),
    libraryScanOperations: (
      id: string,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<LibraryScanOperationsResponse>(
        `/api/libraries/${encodeURIComponent(id)}/scan-operations`,
        init,
      ),
    libraryScanReview: (
      id: string,
      params: { limit?: number; cursor?: string } = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      if (params.cursor) search.set("cursor", params.cursor);
      const query = search.toString();
      return request<LibraryScanReviewResponse>(
        `/api/libraries/${encodeURIComponent(id)}/scan-review${query ? `?${query}` : ""}`,
        init,
      );
    },
    libraryScanRuns: (
      id: string,
      params: { limit?: number } = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      const query = search.toString();
      return request<LibraryScanRunListResponse>(
        `/api/libraries/${encodeURIComponent(id)}/scan-runs${query ? `?${query}` : ""}`,
        init,
      );
    },
    cancelLibraryScan: (id: string) =>
      request<Job>(`/api/libraries/${encodeURIComponent(id)}/scan/cancel`, {
        method: "POST",
      }),
    retryLibraryScan: (id: string, body: LibraryScanRetryRequest = {}) =>
      request<Job>(`/api/libraries/${encodeURIComponent(id)}/scan/retry`, {
        method: "POST",
        body,
      }),
    fetchMissingLyrics: (id: string) =>
      request<Job>(`/api/libraries/${encodeURIComponent(id)}/lyrics`, {
        method: "POST",
      }),
    emptyLibraryTrash: () =>
      request<{ ok: boolean; queued: boolean; existing?: boolean; job?: Job }>(
        "/api/libraries/trash/empty",
        { method: "POST" },
      ),
    home: (init?: Pick<RequestInit, "signal">) =>
      request<HomeResponse>("/api/home", init),
    homeRow: (
      id: string,
      params?: { limit?: number; cursor?: string },
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params?.limit) search.set("limit", String(params.limit));
      if (params?.cursor) search.set("cursor", params.cursor);
      const query = search.toString();
      return request<HomeRow>(
        `/api/home/rows/${encodeURIComponent(id)}${query ? `?${query}` : ""}`,
        init,
      );
    },
    suggestions: (
      params?: { limit?: number; libraryId?: string },
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params?.limit) search.set("limit", String(params.limit));
      if (params?.libraryId) search.set("libraryId", params.libraryId);
      const query = search.toString();
      return request<SuggestionsResponse>(
        `/api/suggestions${query ? `?${query}` : ""}`,
        init,
      );
    },
    watchlist: (params?: SavedMediaBrowseParams, init?: RequestSignal) => {
      const search = new URLSearchParams();
      if (params?.limit) search.set("limit", String(params.limit));
      if (params?.cursor) search.set("cursor", params.cursor);
      if (params?.filter) search.set("filter", params.filter);
      if (params?.sort) search.set("sort", params.sort);
      if (params?.order) search.set("order", params.order);
      const query = search.toString();
      return request<CursorListResponse<MediaItem>>(
        `/api/watchlist${query ? `?${query}` : ""}`,
        init,
      );
    },
    favorites: (params?: SavedMediaBrowseParams, init?: RequestSignal) => {
      const search = new URLSearchParams();
      if (params?.limit) search.set("limit", String(params.limit));
      if (params?.cursor) search.set("cursor", params.cursor);
      if (params?.filter && params.filter !== "all")
        search.set("filter", params.filter);
      if (params?.sort) search.set("sort", params.sort);
      if (params?.order) search.set("order", params.order);
      const query = search.toString();
      return request<CursorListResponse<MediaItem>>(
        `/api/favorites${query ? `?${query}` : ""}`,
        init,
      );
    },
    /** Viewer-safe detail only. Management evidence is never selected by client query input. */
    media: (
      id: string,
      params?: { includeRecommendations?: boolean },
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params?.includeRecommendations)
        search.set("includeRecommendations", "true");
      const query = search.toString();
      return request<MediaItem>(
        `/api/media/${encodeURIComponent(id)}${query ? `?${query}` : ""}`,
        init,
      );
    },
    mediaChildren: (
      id: string,
      params: { limit?: number; cursor?: string } = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      if (params.cursor) search.set("cursor", params.cursor);
      const query = search.toString();
      return request<MediaChildrenResponse>(
        `/api/media/${encodeURIComponent(id)}/children${query ? `?${query}` : ""}`,
        init,
      );
    },
    mediaRecommendations: (id: string, init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<HomeRow>>(
        `/api/media/${encodeURIComponent(id)}/recommendations`,
        init,
      ),
    artworkUrl: (
      id: string,
      kind: string,
      imageOptions: { rendition?: "small" | "large" } = {},
    ) => {
      const url = new URL(
        resourceUrl(
          `/api/artwork/${encodeURIComponent(id)}/${encodeURIComponent(kind)}`,
        ),
        baseHref(options.baseHref),
      );
      if (imageOptions.rendition)
        url.searchParams.set("rendition", imageOptions.rendition);
      return url.toString();
    },
    mediaStreamUrl: (id: string) =>
      resourceUrl(`/api/media/${encodeURIComponent(id)}/stream`),
    mediaAttachmentUrl: (id: string, attachmentId: string) =>
      resourceUrl(
        `/api/media/${encodeURIComponent(id)}/attachments/${encodeURIComponent(attachmentId)}`,
      ),
    uploadSubtitle: (id: string, form: FormData, init?: RequestSignal) =>
      formRequest<MediaItem>(
        `/api/media/${encodeURIComponent(id)}/subtitles`,
        form,
        "POST",
        init,
      ),
    updateSubtitle: (
      id: string,
      streamId: string,
      body: { offsetMs: number },
      init?: RequestSignal,
    ) =>
      request<MediaItem>(
        `/api/media/${encodeURIComponent(id)}/subtitles/${encodeURIComponent(streamId)}`,
        { ...init, method: "PATCH", body },
      ),
    deleteSubtitle: (id: string, streamId: string, init?: RequestSignal) =>
      request<{ ok: boolean }>(
        `/api/media/${encodeURIComponent(id)}/subtitles/${encodeURIComponent(streamId)}`,
        { ...init, method: "DELETE" },
      ),
    uploadLyrics: (id: string, form: FormData, init?: RequestSignal) =>
      formRequest<MediaItem>(
        `/api/media/${encodeURIComponent(id)}/lyrics`,
        form,
        "POST",
        init,
      ),
    fetchLyrics: (id: string, init?: RequestSignal) =>
      request<MediaItem>(`/api/media/${encodeURIComponent(id)}/lyrics/fetch`, {
        ...init,
        method: "POST",
      }),
    searchLyrics: (id: string, query: string, init?: RequestSignal) => {
      const search = new URLSearchParams();
      if (query.trim()) search.set("query", query.trim());
      return request<ListResponse<LyricSearchCandidate>>(
        `/api/media/${encodeURIComponent(id)}/lyrics/search?${search}`,
        init,
      );
    },
    applyLyrics: (
      id: string,
      body: { provider: string; externalId: string },
      init?: RequestSignal,
    ) =>
      request<MediaItem>(`/api/media/${encodeURIComponent(id)}/lyrics/apply`, {
        ...init,
        method: "POST",
        body,
      }),
    deleteLyrics: (id: string, lyricId: string, init?: RequestSignal) =>
      request<{ ok: boolean }>(
        `/api/media/${encodeURIComponent(id)}/lyrics/${encodeURIComponent(lyricId)}`,
        { ...init, method: "DELETE" },
      ),
    addMediaSegment: (id: string, body: MediaSegmentRequest) =>
      request<MediaItem>(`/api/media/${encodeURIComponent(id)}/segments`, {
        method: "POST",
        body,
      }),
    deleteMediaSegment: (id: string, segmentId: string) =>
      request<MediaItem>(
        `/api/media/${encodeURIComponent(id)}/segments/${encodeURIComponent(segmentId)}`,
        { method: "DELETE" },
      ),
    mediaTrickplay: (id: string, init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<MediaTrickplaySet>>(
        `/api/media/${encodeURIComponent(id)}/trickplay`,
        init,
      ),
    mediaTrickplayPlaylistUrl: (id: string, setId: string) =>
      resourceUrl(
        `/api/media/${encodeURIComponent(id)}/trickplay/${encodeURIComponent(setId)}/tiles.m3u8`,
      ),
    mediaTrickplayTileUrl: (id: string, setId: string, tileIndex: number) =>
      resourceUrl(
        `/api/media/${encodeURIComponent(id)}/trickplay/${encodeURIComponent(setId)}/tiles/${encodeURIComponent(String(tileIndex))}.jpg`,
      ),
    uploadMediaImage: (id: string, form: FormData, expectedRevision: number, init?: RequestSignal) => {
      form.set("expectedRevision", String(expectedRevision));
      return formRequest<MediaItem>(
        `/api/media/${encodeURIComponent(id)}/images`, form, "POST", init,
      );
    },
    mediaImageInfo: (id: string, imageId: string) =>
      request<MediaImage>(
        `/api/media/${encodeURIComponent(id)}/images/${encodeURIComponent(imageId)}`,
      ),
    deleteMediaImage: (id: string, imageId: string, expectedRevision: number, init?: RequestSignal) =>
      request<{ ok: boolean }>(
        `/api/media/${encodeURIComponent(id)}/images/${encodeURIComponent(imageId)}?expectedRevision=${encodeURIComponent(String(expectedRevision))}`,
        { ...init, method: "DELETE" },
      ),
    setPreferredMediaImage: (
      id: string,
      imageId: string,
      expectedRevision: number,
      init?: RequestSignal,
    ) =>
      request<MediaItem>(
        `/api/media/${encodeURIComponent(id)}/images/${encodeURIComponent(imageId)}/preferred`,
        { ...init, method: "POST", body: { expectedRevision } },
      ),
    reorderMediaImages: (
      id: string,
      imageIds: string[],
      expectedRevision: number,
      init?: RequestSignal,
    ) =>
      request<MediaItem>(`/api/media/${encodeURIComponent(id)}/images/order`, {
        ...init,
        method: "POST",
        body: { imageIds, expectedRevision },
      }),
    updateMedia: (id: string, body: UpdateMediaRequest) =>
      request<MediaItem>(`/api/media/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body,
      }),
    searchMediaMatches: (
      id: string,
      query: string,
      searchOptions: { year?: number; language?: string } = {},
    ) => {
      const search = new URLSearchParams();
      if (query.trim()) search.set("query", query.trim());
      if (searchOptions.year && Number.isFinite(searchOptions.year))
        search.set("year", String(searchOptions.year));
      if (searchOptions.language?.trim())
        search.set("language", searchOptions.language.trim());
      return request<MediaMatchSearchResponse>(
        `/api/media/${encodeURIComponent(id)}/match-candidates?${search}`,
      );
    },
    applyMediaMatch: (id: string, body: ManualMediaMatchRequest) =>
      request<MediaItem>(`/api/media/${encodeURIComponent(id)}/match`, {
        method: "POST",
        body,
      }),
    metadataRepair: (params: { limit?: number; staleDays?: number } = {}) => {
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      if (params.staleDays) search.set("staleDays", String(params.staleDays));
      return request<MetadataRepairResponse>(
        `/api/metadata/repair${search.toString() ? `?${search}` : ""}`,
      );
    },
    metadataHealth: (
      params: { limit?: number; category?: string; libraryId?: string } = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      if (params.category) search.set("category", params.category);
      if (params.libraryId) search.set("libraryId", params.libraryId);
      return request<MetadataHealthResponse>(
        `/api/metadata/health${search.toString() ? `?${search}` : ""}`,
        init,
      );
    },
    metadataHealthAction: (body: {
      category: "missing_artwork";
      limit?: number;
      libraryId?: string;
    }) =>
      request<MetadataHealthActionResponse>("/api/metadata/health", {
        method: "POST",
        body,
      }),
    forceMetadataRematch: (mediaId: string, clearProviderIds = true) =>
      request<MetadataRepairActionResponse>("/api/metadata/repair", {
        method: "POST",
        body: { mediaId, clearProviderIds },
      }),
    deleteMedia: (
      id: string,
      body: { deleteFiles?: boolean; confirmation?: string } = {},
    ) =>
      request<{ ok: boolean; deletedItems: number; trashedFiles: number }>(
        `/api/media/${encodeURIComponent(id)}`,
        { method: "DELETE", body },
      ),
    setWatchlist: (id: string, watchlisted: boolean, init?: RequestSignal) =>
      request<MediaItem>(`/api/media/${encodeURIComponent(id)}/watchlist`, {
        ...init,
        method: "POST",
        body: { watchlisted },
      }),
    setFavorite: (id: string, favorite: boolean, init?: RequestSignal) =>
      request<MediaItem>(`/api/media/${encodeURIComponent(id)}/favorite`, {
        ...init,
        method: "POST",
        body: { favorite },
      }),
    setReaction: (
      id: string,
      reaction: "like" | "dislike" | "",
      init?: RequestSignal,
    ) =>
      request<MediaItem>(`/api/media/${encodeURIComponent(id)}/reaction`, {
        ...init,
        method: "POST",
        body: { reaction },
      }),
    setWatched: (id: string, watched: boolean, init?: RequestSignal) =>
      request<MediaItem>(`/api/media/${encodeURIComponent(id)}/watched`, {
        ...init,
        method: "POST",
        body: { watched },
      }),
    setRating: (id: string, rating: number, init?: RequestSignal) =>
      request<MediaItem>(`/api/media/${encodeURIComponent(id)}/rating`, {
        ...init,
        method: "POST",
        body: { rating },
      }),
    mediaJob: (
      id: string,
      type: string,
      jobOptions: { profile?: string; analysisMode?: "full" | "probe" } = {},
    ) =>
      request<Job>(`/api/media/${encodeURIComponent(id)}/jobs`, {
        method: "POST",
        body: { type, ...jobOptions },
      }),
    bulkMediaState: (
      mediaIds: string[],
      state: { watchlisted?: boolean; favorite?: boolean; watched?: boolean },
    ) =>
      request<ListResponse<MediaItem>>("/api/media/bulk/state", {
        method: "POST",
        body: { mediaIds, ...state },
      }),
    bulkMediaMetadata: (mediaIds: string[], patch: UpdateMediaRequest) =>
      request<ListResponse<MediaItem>>("/api/media/bulk/metadata", {
        method: "POST",
        body: { mediaIds, patch },
      }),
    bulkMediaJobs: (
      mediaIds: string[],
      type: string,
      jobOptions: { profile?: string; analysisMode?: "full" | "probe" } = {},
    ) =>
      request<ListResponse<Job>>("/api/media/bulk/jobs", {
        method: "POST",
        body: { mediaIds, type, ...jobOptions },
      }),
    downloadOptions: (id: string, init?: RequestSignal) =>
      request<DownloadOptionsResponse>(
        `/api/media/${encodeURIComponent(id)}/download-options`,
        init,
      ),
    downloadPreparations: (init?: RequestSignal) =>
      request<ListResponse<DownloadPreparation>>(
        "/api/download-preparations",
        init,
      ),
    downloadPreparation: (id: string, init?: RequestSignal) =>
      request<DownloadPreparation>(
        `/api/download-preparations/${encodeURIComponent(id)}`,
        init,
      ),
    createDownloadPreparation: (
      body: DownloadPreparationSingleCreateRequest,
      init?: RequestSignal,
    ) =>
      request<DownloadPreparation>("/api/download-preparations", {
        ...init,
        method: "POST",
        body,
      }),
    createDownloadPreparationBatch: (
      body: DownloadPreparationBatchCreateRequest,
      init?: RequestSignal,
    ) =>
      request<DownloadPreparationBatchResponse>("/api/download-preparations", {
        ...init,
        method: "POST",
        body,
      }),
    createNextEpisodeDownloadPreparation: (
      body: DownloadPreparationNextEpisodeRequest,
      init?: RequestSignal,
    ) =>
      request<DownloadPreparation>("/api/download-preparations", {
        ...init,
        method: "POST",
        body,
      }),
    updateDownloadPreparation: (
      id: string,
      body: DownloadPreparationUpdateRequest,
      init?: RequestSignal,
    ) =>
      request<DownloadPreparation>(
        `/api/download-preparations/${encodeURIComponent(id)}`,
        { ...init, method: "PATCH", body },
      ),
    removeDownloadPreparation: (id: string, init?: RequestSignal) =>
      request<DownloadPreparation>(
        `/api/download-preparations/${encodeURIComponent(id)}`,
        { ...init, method: "DELETE" },
      ),
    createDownloadPreparationGrant: (
      id: string,
      body: { delivery: "browser" | "native" },
      init?: RequestSignal,
    ) =>
      request<MediaDownloadGrantResponse>(
        `/api/download-preparations/${encodeURIComponent(id)}/grant`,
        { ...init, method: "POST", body },
      ),
    revalidateOfflineDownloadAuthorization: (
      body: OfflineDownloadAuthorizationRevalidationRequest,
      init?: RequestSignal,
    ) =>
      request<OfflineDownloadAuthorizationRevalidationResponse>(
        "/api/offline-download-authorizations/revalidate",
        {...init, method: "POST", body},
      ),
    optimizedVersions: (id: string) =>
      request<OptimizedVersionListResponse>(
        `/api/media/${encodeURIComponent(id)}/optimized`,
      ),
    createOptimizedVersion: (id: string, body: OptimizedVersionRequest) =>
      request<Job>(`/api/media/${encodeURIComponent(id)}/optimized`, {
        method: "POST",
        body,
      }),
    deleteOptimizedVersion: (
      id: string,
      profileId: string,
      init?: RequestSignal,
    ) =>
      request<{ ok: boolean }>(
        `/api/media/${encodeURIComponent(id)}/optimized/${encodeURIComponent(profileId)}`,
        { ...init, method: "DELETE" },
      ),
    startPlayback: (
      mediaId: string,
      playbackOptions: MediaPlaybackStartOptions = {},
      init?: RequestSignal,
    ) =>
      request<PlaybackResponse>("/api/playback-sessions", {
        ...init,
        method: "POST",
        body: {
          mediaId,
          clientInstanceId: clientInstanceId(),
          clientProfile: profile(),
          intent: playbackOptions.intent ?? { quality: { mode: "automatic" } },
          versionId: playbackOptions.versionId,
          skipPreroll: playbackOptions.skipPreroll,
          burnInSubtitleId: playbackOptions.burnInSubtitleId,
          subtitleStreamId: playbackOptions.subtitleStreamId,
          audioStreamId: playbackOptions.audioStreamId,
          startSeconds: playbackOptions.startSeconds,
          queueMediaIds: playbackOptions.queueMediaIds,
          repeatMode: playbackOptions.repeatMode,
          sourceContext: playbackOptions.sourceContext,
        },
      }).then(normalizeSessionPlayback),
    createCastBootstrap,
    replacePlaybackWithCast,
    castTransferStatus,
    retryPendingCastTransfer,
    pendingCastTransfers,
    redeemCastBootstrap: (body: CastRedeemRequest, init?: RequestSignal) =>
      request<CastReceiverSessionResponse>("/api/playback/cast/redeem", {
        ...init,
        method: "POST",
        body,
      }).then((response) => ({
        ...response,
        playback: normalizeSessionPlayback(response.playback),
      })),
    reconnectCast: (body: CastReconnectRequest, init?: RequestSignal) =>
      request<CastReceiverSessionState>("/api/playback/cast/reconnect", {
        ...init,
        method: "POST",
        body,
      }),
    castReceiverState: (
      receiverSessionId: string,
      credential: CastReceiverCredential,
      init?: RequestSignal,
    ) =>
      request<CastReceiverSessionState>(
        `/api/playback/cast/sessions/${encodeURIComponent(receiverSessionId)}/state`,
        {
          ...init,
          authorization: {
            mode: "cast-receiver",
            token: credential.token,
            origin: credential.origin,
          },
        },
      ),
    controlCastReceiver: (
      receiverSessionId: string,
      credential: CastReceiverCredential,
      body: CastControlRequest,
      init?: RequestSignal,
    ) =>
      request<PlaybackCommand>(
        `/api/playback/cast/sessions/${encodeURIComponent(receiverSessionId)}/control`,
        {
          ...init,
          method: "POST",
          authorization: {
            mode: "cast-receiver",
            token: credential.token,
            origin: credential.origin,
          },
          body,
        },
      ),
    reportCastReceiverProgress: (
      receiverSessionId: string,
      credential: CastReceiverCredential,
      body: CastProgressRequest,
      init?: RequestSignal,
    ) =>
      request<PlaybackProgressAcknowledgement>(
        `/api/playback/cast/sessions/${encodeURIComponent(receiverSessionId)}/progress`,
        {
          ...init,
          method: "POST",
          authorization: {
            mode: "cast-receiver",
            token: credential.token,
            origin: credential.origin,
          },
          body,
        },
      ),
    renewCastReceiver: (
      receiverSessionId: string,
      credential: CastReceiverCredential,
      body: CastRenewRequest,
      init?: RequestSignal,
    ) =>
      request<CastRenewResponse>(
        `/api/playback/cast/sessions/${encodeURIComponent(receiverSessionId)}/renew`,
        {
          ...init,
          method: "POST",
          authorization: {
            mode: "cast-receiver",
            token: credential.token,
            origin: credential.origin,
          },
          body,
        },
      ),
    stopCastReceiver: (
      receiverSessionId: string,
      credential: CastReceiverCredential,
      body: CastStopRequest,
      init?: RequestSignal,
    ) =>
      request<CastStopResponse>(
        `/api/playback/cast/sessions/${encodeURIComponent(receiverSessionId)}/stop`,
        {
          ...init,
          method: "POST",
          authorization: {
            mode: "cast-receiver",
            token: credential.token,
            origin: credential.origin,
          },
          body,
        },
      ),
    advanceCastReceiver: (
      receiverSessionId: string,
      credential: CastReceiverCredential,
      body: CastAdvanceRequest,
      init?: RequestSignal,
    ) =>
      request<CastAdvanceResponse>(
        `/api/playback/cast/sessions/${encodeURIComponent(receiverSessionId)}/advance`,
        {
          ...init,
          method: "POST",
          authorization: {
            mode: "cast-receiver",
            token: credential.token,
            origin: credential.origin,
          },
          body,
        },
      ).then((response) => ({
        ...response,
        playback: response.playback
          ? normalizeSessionPlayback(response.playback)
          : undefined,
      })),
    cancelCastReceiverAdvance: (
      receiverSessionId: string,
      credential: CastReceiverCredential,
      body: CastAdvanceCancelRequest,
      init?: RequestSignal,
    ) =>
      request<CastAdvanceResponse>(
        `/api/playback/cast/sessions/${encodeURIComponent(receiverSessionId)}/advance-cancel`,
        {
          ...init,
          method: "POST",
          authorization: {
            mode: "cast-receiver",
            token: credential.token,
            origin: credential.origin,
          },
          body,
        },
      ),
    skipCastReceiverSegment: (
      receiverSessionId: string,
      credential: CastReceiverCredential,
      body: CastSegmentSkipRequest,
      init?: RequestSignal,
    ) =>
      request<CastSegmentSkipResponse>(
        `/api/playback/cast/sessions/${encodeURIComponent(receiverSessionId)}/segment-skip`,
        {
          ...init,
          method: "POST",
          authorization: {
            mode: "cast-receiver",
            token: credential.token,
            origin: credential.origin,
          },
          body,
        },
      ),
    playbackSessionQueue: (sessionId: string) =>
      request<PlaybackSessionQueueResponse>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/queue`,
      ),
    updatePlaybackSessionQueue: async (
      sessionId: string,
      body: PlaybackSessionQueueReplaceRequest,
      init?: RequestSignal,
    ) => {
      await assertPlaybackSessionMutationAllowed(sessionId);
      return request<PlaybackSessionQueueResponse>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/queue`,
        { ...init, method: "PUT", body },
      );
    },
    mutatePlaybackSessionQueue: async (
      sessionId: string,
      body: PlaybackSessionQueueRequest,
      init?: RequestSignal,
    ) => {
      await assertPlaybackSessionMutationAllowed(sessionId);
      return request<PlaybackSessionQueueResponse>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/queue`,
        { ...init, method: "PATCH", body },
      );
    },
    prepareNextPlayback: async (
      sessionId: string,
      body: PlaybackPrepareNextRequest,
      init?: RequestSignal,
    ) => {
      await assertPlaybackSessionMutationAllowed(sessionId);
      const response = await request<PlaybackPreparedResponse>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/prepare-next`,
        {
          ...init,
          method: "POST",
          body: { ...body, clientProfile: body.clientProfile ?? profile() },
        },
      );
      return {
        ...response,
        playback: normalizeSessionPlayback(response.playback),
      };
    },
    handoffPlayback: async (
      sessionId: string,
      body: PlaybackHandoffInput,
      init?: RequestSignal,
    ) => {
      assertPlaybackTerminalRequestId(body.requestId);
      assertPlaybackTerminalEvent(body.previousTerminal);
      if (
        typeof body.entryId !== "string" ||
        body.entryId.length < 1 ||
        body.entryId.length > 512
      ) {
        throw new TypeError("Playback handoff entry ID is invalid.");
      }
      if (
        body.startSeconds !== undefined &&
        (!Number.isSafeInteger(body.startSeconds) || body.startSeconds < 0)
      ) {
        throw new TypeError(
          "Playback handoff start position must be a non-negative integer.",
        );
      }
      await loadDurablePlaybackProgress();
      const session = await credentials.current();
      const scope = playbackAuthorityScope(session);
      const fenceKey = playbackSessionFenceKey(sessionId, scope);
      const key = playbackTerminalKey(
        "handoff",
        sessionId,
        body.requestId,
        scope,
      );
      if (playbackTerminalRequestsInFlight.has(fenceKey)) {
        throw new ApiError(
          409,
          "playback_stopping",
          "Playback already has a terminal mutation in flight.",
        );
      }
      const cached = playbackHandoffRequests.get(key);
      if (playbackStoppingSessions.has(fenceKey) && !cached) {
        throw new ApiError(
          409,
          "playback_terminal_request_conflict",
          "This playback session already has a different uncertain terminal request.",
        );
      }
      playbackStoppingSessions.add(fenceKey);
      playbackTerminalRequestsInFlight.add(fenceKey);
      let requestDispatched = false;
      try {
        let requestBody = cached;
        if (requestBody) {
          if (!handoffInputMatches(requestBody, body)) {
            throw new ApiError(
              409,
              "playback_terminal_request_conflict",
              "This playback handoff identity already owns a different request body.",
            );
          }
        } else {
          const previousTerminal = isCompletePlaybackTerminalEvent(
            body.previousTerminal,
          )
            ? body.previousTerminal
            : await allocatePlaybackTerminalEvent(
                sessionId,
                body.previousTerminal,
                session,
                scope,
                init,
              );
          requestBody = {
            ...body,
            previousTerminal,
            clientProfile: body.clientProfile ?? profile(),
          };
          assertPlaybackHandoffRequest(requestBody);
          requestBody = cachePlaybackHandoffRequest(
            key,
            sessionId,
            scope,
            requestBody,
          );
          await persistPlaybackTerminalRequest({
            version: "v2",
            kind: "terminal",
            key,
            scope,
            sessionId,
            operation: "handoff",
            request: requestBody,
            updatedAt: new Date(options.now?.() ?? Date.now()).toISOString(),
          });
        }
        requestDispatched = true;
        const response = await request<PlaybackResponse>(
          `/api/playback-sessions/${encodeURIComponent(sessionId)}/handoff`,
          {
            ...init,
            method: "POST",
            body: requestBody,
          },
        );
        const normalized = normalizeSessionPlayback(response);
        await forgetClientPlaybackProgress(sessionId, scope);
        await removeDurablePlaybackTerminalRequest(key);
        playbackHandoffRequests.delete(key);
        playbackTerminalSessionIds.delete(key);
        playbackTerminalScopes.delete(key);
        playbackStoppingSessions.delete(fenceKey);
        return normalized;
      } catch (error) {
        if (
          (!requestDispatched && !cached) ||
          terminalMutationRejectionIsDefinitive(error)
        ) {
          await removeDurablePlaybackTerminalRequest(key);
          playbackHandoffRequests.delete(key);
          playbackTerminalSessionIds.delete(key);
          playbackTerminalScopes.delete(key);
          playbackStoppingSessions.delete(fenceKey);
        }
        throw error;
      } finally {
        playbackTerminalRequestsInFlight.delete(fenceKey);
      }
    },
    renegotiatePlayback: async (
      sessionId: string,
      body: PlaybackRenegotiationRequest,
      init?: RequestSignal,
    ) => {
      await assertPlaybackSessionMutationAllowed(sessionId);
      return request<PlaybackResponse>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/renegotiate`,
        {
          ...init,
          method: "POST",
          body: { ...body, clientProfile: body.clientProfile ?? profile() },
        },
      ).then(normalizeSessionPlayback);
    },
    restoreActivePlayback,
    restoreCommittedPlaybackReplacement,
    touchPlayback: async (
      sessionId: string,
      body: PlaybackProgressInput,
      init?: RequestSignal,
    ) => {
      await loadDurablePlaybackProgress();
      const session = await credentials.current();
      const scope = playbackAuthorityScope(session);
      if (playbackStoppingSessions.has(playbackSessionFenceKey(sessionId, scope))) {
        throw new ApiError(
          409,
          "playback_stopping",
          "Playback is stopping and cannot accept more progress updates.",
        );
      }
      const generation = playbackSessionGenerations.get(sessionId) ?? 0;
      const key = playbackProgressKey(sessionId, generation, session);
      seedClientPlaybackProgressSequence(
        key,
        playbackSessionNextEventSequences.get(sessionId) ?? 1,
      );
      return enqueuePlaybackProgress(key, body, (event) =>
        request<PlaybackProgressAcknowledgement>(
          `/api/playback-sessions/${encodeURIComponent(sessionId)}`,
          { ...init, method: "PATCH", body: event },
        ),
      );
    },
    getPlaybackContinuationState: (
      sessionId: string,
      credential: PlaybackContinuationCredential,
      init?: RequestSignal,
    ) =>
      request<PlaybackContinuationState>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/continuation`,
        {
          ...init,
          authorization: {
            mode: "playback-continuation",
            token: credential.token,
            origin: credential.origin,
          },
        },
      ),
    touchPlaybackContinuation: async (
      sessionId: string,
      credential: PlaybackContinuationCredential,
      body: PlaybackProgressInput,
      init?: RequestSignal,
    ) => {
      await loadDurablePlaybackProgress();
      const session = await credentials.current();
      const scope = playbackAuthorityScope(session, credential.origin);
      if (playbackStoppingSessions.has(playbackSessionFenceKey(sessionId, scope))) {
        throw new ApiError(
          409,
          "playback_stopping",
          "Playback is stopping and cannot accept more progress updates.",
        );
      }
      const key = playbackProgressKey(
        sessionId,
        credential.generation,
        session,
        credential.origin,
      );
      return enqueuePlaybackProgress(key, body, (event) =>
        request<PlaybackProgressAcknowledgement>(
          `/api/playback-sessions/${encodeURIComponent(sessionId)}/continuation`,
          {
            ...init,
            method: "PATCH",
            authorization: {
              mode: "playback-continuation",
              token: credential.token,
              origin: credential.origin,
            },
            body: event,
          },
        ),
      );
    },
    rotatePlaybackContinuation: (
      sessionId: string,
      credential: PlaybackContinuationCredential,
      body: PlaybackContinuationRotateRequest,
      init?: RequestSignal,
    ) =>
      request<PlaybackContinuationCredential>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/continuation`,
        {
          ...init,
          method: "POST",
          authorization: {
            mode: "playback-continuation",
            token: credential.token,
            origin: credential.origin,
          },
          body,
        },
      ),
    revokePlaybackContinuation: (
      sessionId: string,
      credential: PlaybackContinuationCredential,
      body: PlaybackSessionStopRequest,
      init?: RequestSignal,
    ) => {
      assertPlaybackSessionStopRequest(body);
      return request<PlaybackSessionTerminalAcknowledgement>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/continuation`,
        {
          ...init,
          method: "DELETE",
          authorization: {
            mode: "playback-continuation",
            token: credential.token,
            origin: credential.origin,
          },
          body,
        },
      ).then((acknowledgement) => {
        assertPlaybackTerminalAcknowledgement(
          acknowledgement,
          sessionId,
          body,
        );
        return acknowledgement;
      });
    },
    renewPlaybackMediaGrant: (sessionId: string, init?: RequestSignal) =>
      request<MediaGrant>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/media-grant`,
        { ...init, method: "POST" },
      ),
    stopPlayback: stopPlaybackSession,
    playbackCommand: (sessionId: string) =>
      request<PlaybackCommand>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/command`,
      ),
    playbackCommandEventsUrl: (sessionId: string) =>
      resourceUrl(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/command/events`,
      ),
    subscribePlaybackCommandEvents: (
      sessionId: string,
      subscription: PorticoEventSubscriptionOptions<PlaybackCommand>,
    ) => {
      const normalizedSessionId = requiredEventResourceId(
        sessionId,
        "playback session",
      );
      return eventSubscriptions.run(
        `playback-command-events:${normalizedSessionId}`,
        subscription.signal,
        (signal) =>
          subscribePlaybackCommandEventTransport(
            options,
            credentials,
            normalizedSessionId,
            { ...subscription, signal },
          ),
      );
    },
    issuePlaybackCommand: async (
      sessionId: string,
      body: {
        action: "play" | "pause" | "seek" | "stop" | "load";
        mediaId?: string;
        positionSeconds?: number;
        message?: string;
      },
    ) => {
      await assertPlaybackSessionMutationAllowed(sessionId);
      return request<PlaybackCommand>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/command`,
        { method: "POST", body },
      );
    },
    playbackTargets: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<PlaybackTarget>>(
        `/api/playback/targets?clientInstanceId=${encodeURIComponent(clientInstanceId())}`,
        init,
      ),
    playbackReceivers: () =>
      request<ListResponse<PlaybackReceiver>>("/api/playback/receivers"),
    registerPlaybackReceiver: (body: Omit<PlaybackReceiverRequest, "clientInstanceId">) =>
      request<PlaybackReceiver>("/api/playback/receivers", {
        method: "POST",
        body: { ...body, clientInstanceId: clientInstanceId() },
      }),
    heartbeatPlaybackReceiver: (
      id: string,
      body: PlaybackReceiverHeartbeatRequest,
    ) =>
      request<PlaybackReceiverHeartbeatResponse>(
        `/api/playback/receivers/${encodeURIComponent(id)}`,
        { method: "PATCH", body },
      ),
    playbackReceiverAuthorizations: (
      id: string,
      receiverPublicKeyFingerprint: string,
    ) =>
      request<ListResponse<ReceiverAuthorizationRecord>>(
        `/api/playback/receivers/${encodeURIComponent(id)}/authorizations?receiverPublicKeyFingerprint=${encodeURIComponent(receiverPublicKeyFingerprint)}`,
      ),
    authorizePlaybackReceiver: (
      id: string,
      body: Omit<ReceiverAuthorizationRequest, "clientInstanceId">,
    ) =>
      request<ReceiverControllerGrant>(
        `/api/playback/receivers/${encodeURIComponent(id)}/authorizations`,
        { method: "POST", body: { ...body, clientInstanceId: clientInstanceId() } },
      ),
    handoffPlaybackToReceiver: (
      id: string,
      body: PlaybackReceiverHandoffRequest,
    ) =>
      request<PlaybackResponse>(
        `/api/playback/receivers/${encodeURIComponent(id)}/handoff`,
        { method: "POST", body },
      ),
    commitPlaybackReceiverHandoff: (
      id: string,
      requestId: string,
      body: PlaybackReceiverHandoffCommitRequest,
    ) =>
      request<PlaybackResponse>(
        `/api/playback/receivers/${encodeURIComponent(id)}/handoffs/${encodeURIComponent(requestId)}/commit`,
        { method: "POST", body },
      ),
    playbackReceiverHandoffStatus: (
      id: string,
      input: {
        authorizationId: string;
        receiverPublicKeyFingerprint: string;
        requestId: string;
        sourceSessionId: string;
      },
    ) =>
      request<PlaybackReceiverHandoffStatusResponse>(
        `/api/playback/receivers/${encodeURIComponent(id)}/handoffs/${encodeURIComponent(input.requestId)}?authorizationId=${encodeURIComponent(input.authorizationId)}&receiverPublicKeyFingerprint=${encodeURIComponent(input.receiverPublicKeyFingerprint)}&sourceSessionId=${encodeURIComponent(input.sourceSessionId)}`,
      ),
    revokePlaybackReceiverAuthorization: (id: string, authorizationId: string) =>
      request<void>(
        `/api/playback/receivers/${encodeURIComponent(id)}/authorizations/${encodeURIComponent(authorizationId)}`,
        { method: "DELETE" },
      ),
    watchWithFriendsGroups: (init?: RequestSignal) =>
      request<ListResponse<WatchWithFriendsGroup>>(
        "/api/watch-with-friends/groups",
        init,
      ),
    createWatchWithFriendsGroup: (
      body: WatchWithFriendsCreateRequest,
      init?: RequestSignal,
    ) =>
      request<WatchWithFriendsGroup>("/api/watch-with-friends/groups", {
        ...init,
        method: "POST",
        body,
      }),
    watchWithFriendsGroup: (id: string, init?: RequestSignal) =>
      request<WatchWithFriendsGroup>(
        `/api/watch-with-friends/groups/${encodeURIComponent(id)}`,
        init,
      ),
    joinWatchWithFriendsGroup: (id: string, init?: RequestSignal) =>
      request<WatchWithFriendsGroup>(
        `/api/watch-with-friends/groups/${encodeURIComponent(id)}/join`,
        { ...init, method: "POST" },
      ),
    leaveWatchWithFriendsGroup: (id: string, init?: RequestSignal) =>
      request<WatchWithFriendsGroup>(
        `/api/watch-with-friends/groups/${encodeURIComponent(id)}/leave`,
        { ...init, method: "POST" },
      ),
    updateWatchWithFriendsState: (
      id: string,
      body: WatchWithFriendsStateRequest,
      init?: RequestSignal,
    ) =>
      request<WatchWithFriendsGroup>(
        `/api/watch-with-friends/groups/${encodeURIComponent(id)}/state`,
        { ...init, method: "PATCH", body },
      ),
    updateWatchWithFriendsSettings: (
      id: string,
      body: WatchWithFriendsSettingsRequest,
      init?: RequestSignal,
    ) =>
      request<WatchWithFriendsGroup>(
        `/api/watch-with-friends/groups/${encodeURIComponent(id)}/settings`,
        { ...init, method: "PATCH", body },
      ),
    updateWatchWithFriendsMemberState: (
      id: string,
      body: WatchWithFriendsMemberStateRequest,
      init?: RequestSignal,
    ) =>
      request<WatchWithFriendsGroup>(
        `/api/watch-with-friends/groups/${encodeURIComponent(id)}/member/state`,
        { ...init, method: "PATCH", body },
      ),
    addWatchWithFriendsQueueItem: (
      id: string,
      body: WatchWithFriendsQueueRequest,
      init?: RequestSignal,
    ) =>
      request<WatchWithFriendsGroup>(
        `/api/watch-with-friends/groups/${encodeURIComponent(id)}/queue`,
        { ...init, method: "POST", body },
      ),
    reorderWatchWithFriendsQueue: (
      id: string,
      body: WatchWithFriendsQueueOrderRequest,
      init?: RequestSignal,
    ) =>
      request<WatchWithFriendsGroup>(
        `/api/watch-with-friends/groups/${encodeURIComponent(id)}/queue`,
        { ...init, method: "PATCH", body },
      ),
    removeWatchWithFriendsQueueItem: (
      id: string,
      entryId: string,
      expectedRevision: number,
      idempotencyKey: string,
      init?: RequestSignal,
    ) =>
      request<WatchWithFriendsGroup>(
        `/api/watch-with-friends/groups/${encodeURIComponent(id)}/queue/${encodeURIComponent(entryId)}?expectedRevision=${encodeURIComponent(String(expectedRevision))}&idempotencyKey=${encodeURIComponent(idempotencyKey)}`,
        { ...init, method: "DELETE" },
      ),
    endWatchWithFriendsGroup: (
      id: string,
      expectedRevision: number,
      idempotencyKey: string,
      init?: RequestSignal,
    ) =>
      request<WatchWithFriendsGroup>(
        `/api/watch-with-friends/groups/${encodeURIComponent(id)}?expectedRevision=${encodeURIComponent(String(expectedRevision))}&idempotencyKey=${encodeURIComponent(idempotencyKey)}`,
        { ...init, method: "DELETE" },
      ),
    watchWithFriendsGroupEventsUrl: (groupId: string) =>
      resourceUrl(
        `/api/watch-with-friends/groups/${encodeURIComponent(groupId)}/events`,
      ),
    streamWatchWithFriendsGroupEvents: (
      groupId: string,
      signal: AbortSignal,
      onGroup: (group: WatchWithFriendsGroup) => void,
    ) =>
      streamWatchWithFriendsGroupEvents(
        options,
        credentials,
        groupId,
        signal,
        onGroup,
      ),
    subscribeWatchWithFriendsGroupEvents: (
      groupId: string,
      subscription: PorticoEventSubscriptionOptions<WatchWithFriendsGroup>,
    ) => {
      const normalizedGroupId = requiredEventResourceId(
        groupId,
        "Watch With Friends group",
      );
      return eventSubscriptions.run(
        `watch-with-friends-events:${normalizedGroupId}`,
        subscription.signal,
        (signal) =>
          subscribeWatchWithFriendsEventTransport(
            options,
            credentials,
            normalizedGroupId,
            { ...subscription, signal },
          ),
      );
    },
    cancelAllEventSubscriptions: () => eventSubscriptions.cancelAll(),
    bitrateTest: async (
      bytes = 8 * 1024 * 1024,
      requestOptions: RequestSignal = {},
    ): Promise<BitrateTestResult> => {
      if (
        !Number.isSafeInteger(bytes) ||
        bytes < 1 ||
        bytes > 64 * 1024 * 1024
      ) {
        throw new TypeError(
          "Bitrate test size must be an integer between 1 byte and 64 MiB.",
        );
      }
      const transportOptions: TransportRequestOptions = {
        ...requestOptions,
        operationClass:
          requestOptions.operationClass ?? "media/stream transfer",
        method: "GET",
      };
      const requestDeadline = transportDeadlineAt(options, transportOptions);
      const session = await runTransportOperation(
        options,
        transportOptions,
        "GET",
        async () => {
          const sessionResult = credentials.current();
          return sessionResult instanceof Promise
            ? await sessionResult
            : sessionResult;
        },
        undefined,
        requestDeadline,
      );
      const started = now(options);
      const url = buildApiUrl(
        `/api/playback/bitrate-test?bytes=${encodeURIComponent(String(bytes))}`,
        options,
        session,
      );
      const principalIdentity = requestPrincipalIdentity(
        session,
        urlOrigin(url, options.baseHref),
      );
      const requestInit: RequestInit = {
        credentials: "include",
        headers: withRequestId(
          {
            ...authHeader(session),
          },
          options.requestId,
        ),
      };
      const response = await runTransportOperation(
        options,
        transportOptions,
        "GET",
        async (context) => {
          let response = await transportFetch(
            options.transport,
            url,
            requestInit,
            context,
          );
          if (
            response.status === 401 &&
            canAutomaticallyRefresh("/api/playback/bitrate-test", session)
          ) {
            const beforeRefresh = await awaitWithAbort(
              Promise.resolve(credentials.current()),
              context.signal,
            );
            const beforeRefreshURL = buildApiUrl(
              `/api/playback/bitrate-test?bytes=${encodeURIComponent(String(bytes))}`,
              options,
              beforeRefresh,
            );
            if (
              beforeRefreshURL !== url ||
              requestPrincipalIdentity(
                beforeRefresh,
                urlOrigin(beforeRefreshURL, options.baseHref),
              ) !== principalIdentity
            ) {
              throw new ApiError(
                409,
                "request_superseded",
                "The active Portico server or viewer changed during the bitrate test.",
              );
            }
            await awaitWithAbort(
              credentials.refreshAfterUnauthorized(
                currentAccessTokenFromSession(session),
                context.signal,
                () => {
                  context.dispatched = true;
                },
              ),
              context.signal,
            );
            const refreshed = await awaitWithAbort(
              Promise.resolve(credentials.current()),
              context.signal,
            );
            const refreshedURL = buildApiUrl(
              `/api/playback/bitrate-test?bytes=${encodeURIComponent(String(bytes))}`,
              options,
              refreshed,
            );
            if (
              refreshedURL !== url ||
              requestPrincipalIdentity(
                refreshed,
                urlOrigin(refreshedURL, options.baseHref),
              ) !== principalIdentity
            ) {
              throw new ApiError(
                409,
                "request_superseded",
                "The active Portico server or viewer changed during the bitrate test.",
              );
            }
            response = await transportFetch(
              options.transport,
              url,
              {
                ...requestInit,
                headers: withRequestId(
                  {
                    ...normalizeHeaders(requestInit.headers),
                    ...authHeader(refreshed),
                  },
                  options.requestId,
                ),
              },
              context,
            );
          }
          if (!response.ok)
            await throwApiError(
              response,
              "bitrate_test_failed",
              "Bitrate test failed.",
              context.signal,
            );
          return response;
        },
        undefined,
        requestDeadline,
      );
      const payload = await runTransportOperation(
        options,
        transportOptions,
        "GET",
        (context) => awaitWithAbort(response.arrayBuffer(), context.signal),
        undefined,
        requestDeadline,
      );
      const durationMs = Math.max(1, now(options) - started);
      return {
        bytes: payload.byteLength,
        durationMs,
        mbps: (payload.byteLength * 8) / (durationMs / 1000) / 1_000_000,
      };
    },
    search: (body: SearchRequest, init?: Pick<RequestInit, "signal">) =>
      request<SearchResponse>("/api/search", { ...init, method: "POST", body }),
    searchHistory: (init?: Pick<RequestInit, "signal">) =>
      request<SearchHistoryResponse>("/api/search/history", init),
    clearSearchHistory: (query?: string, init?: Pick<RequestInit, "signal">) =>
      request<void>(
        `/api/search/history${query?.trim() ? `?query=${encodeURIComponent(query.trim())}` : ""}`,
        { ...init, method: "DELETE" },
      ),
    person: (
      personId: string,
      params: { limit?: number; cursor?: string } = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      if (params.cursor) search.set("cursor", params.cursor);
      const query = search.toString();
      return request<PersonDetailResponse>(
        `/api/people/${encodeURIComponent(personId)}${query ? `?${query}` : ""}`,
        init,
      );
    },
    personMedia: (
      name: string,
      limit = 50,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<ListResponse<MediaItem>>(
        `/api/people/media?name=${encodeURIComponent(name)}&limit=${encodeURIComponent(String(limit))}`,
        init,
      ),
    instantMix: (id: string, limit = 50, init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<MediaItem>>(
        `/api/instant-mix/${encodeURIComponent(id)}?limit=${encodeURIComponent(String(limit))}`,
        init,
      ),
    settings: (init?: RequestSignal) =>
      request<SettingsDocument>("/api/settings", init),
    settingsSummary: (init?: RequestSignal) =>
      request<SettingsSummaryResponse>("/api/settings/summary", init),
    settingsRegistry: (init?: RequestSignal) =>
      request<SettingsRegistryResponse>("/api/settings/registry", init),
    updateSettings: (body: SettingsUpdateRequest, init?: RequestSignal) =>
      request<SettingsDocument>("/api/settings", {
        ...init,
        method: "PATCH",
        body,
      }),
    logs: (
      options:
        | number
        | {
            limit?: number;
            cursor?: string;
            init?: Pick<RequestInit, "signal">;
          } = 200,
      init?: Pick<RequestInit, "signal">,
    ) => {
      const params = typeof options === "number" ? { limit: options } : options;
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      if (params.cursor) search.set("cursor", params.cursor);
      const query = search.toString();
      return request<ListResponse<LogEvent>>(
        `/api/logs${query ? `?${query}` : ""}`,
        typeof options === "number" ? init : options.init,
      );
    },
    logsStreamUrl: () => resourceUrl("/api/logs/stream"),
    uploadClientLogs: (body: ClientLogUploadRequest, init?: RequestSignal) =>
      request<ClientLogUploadResponse>("/api/client-logs", {
        ...init,
        method: "POST",
        body,
      }),
    auditEvents: (
      options:
        | number
        | {
            limit?: number;
            cursor?: string;
            init?: Pick<RequestInit, "signal">;
          } = 100,
    ) => {
      const params = typeof options === "number" ? { limit: options } : options;
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      if (params.cursor) search.set("cursor", params.cursor);
      const query = search.toString();
      return request<ListResponse<AuditEvent>>(
        `/api/audit-events${query ? `?${query}` : ""}`,
        typeof options === "number" ? undefined : options.init,
      );
    },
    devices: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<Device>>("/api/devices", init),
    updateDevice: (id: string, body: DeviceUpdateRequest) =>
      request<Device>(`/api/devices/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body,
      }),
    revokeDevice: (id: string) =>
      request<{ ok: boolean }>(`/api/devices/${encodeURIComponent(id)}`, {
        method: "DELETE",
      }),
    backups: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<BackupInfo>>("/api/backups", init),
    createBackup: () => request<BackupInfo>("/api/backups", { method: "POST" }),
    restoreAuthorizationContext: (init?: RequestSignal) =>
      request<import("./types.js").RestoreAuthorizationContext>(
        "/api/backups/restore-authorization-context",
        init,
      ),
    restoreBackup: (
      name: string,
      body: RestoreBackupRequest,
      init?: RequestSignal,
    ) =>
      request<RestoreBackupResponse>(
        `/api/backups/${encodeURIComponent(name)}/restore`,
        { ...init, method: "POST", body },
      ),
    restoreUploadedDatabase: (
      input: RestoreUploadedDatabaseInput,
      init?: RequestSignal,
    ) => {
      const form = new FormData();
      if (input.password !== undefined) form.append("password", input.password);
      if (input.hostedAuthorization !== undefined) {
        form.append(
          "hostedAuthorization",
          JSON.stringify(input.hostedAuthorization),
        );
      }
      form.append("confirmation", input.confirmation);
      form.append("databaseBytes", String(input.file.size));
      if (input.manifest !== undefined) {
        if (typeof input.manifest === "string")
          form.append("manifest", input.manifest);
        else form.append("manifest", input.manifest, "backup.manifest.json");
      }
      const filename =
        typeof File !== "undefined" &&
        input.file instanceof File &&
        input.file.name
          ? input.file.name
          : "portico-import.db";
      form.append("database", input.file, filename);
      return formRequest<RestoreBackupResponse>(
        "/api/backups/restore/upload",
        form,
        "POST",
        init,
      );
    },
    restoreStatus: (
      operationId: string,
      statusToken: string,
      init?: RequestSignal,
    ) =>
      request<RestoreBackupResponse>(
        `/api/backups/restore/${encodeURIComponent(operationId)}`,
        {
          ...init,
          headers: { "X-Portico-Restore-Status": statusToken },
        },
      ),
    savedShareCandidates: (
      params: SavedShareCandidateParams = {},
      init?: RequestSignal,
    ) => {
      const search = new URLSearchParams();
      if (params.query) search.set("q", params.query);
      if (params.limit) search.set("limit", String(params.limit));
      const query = search.toString();
      return request<SavedShareCandidatePage>(
        `/api/saved/share-candidates${query ? `?${query}` : ""}`,
        init,
      );
    },
    playlists: (
      params: SavedResourceBrowseParams = {},
      init?: RequestSignal,
    ) => {
      const search = new URLSearchParams();
      if (params.cursor) search.set("cursor", params.cursor);
      if (params.limit) search.set("limit", String(params.limit));
      if (params.libraryId) search.set("libraryId", params.libraryId);
      const query = search.toString();
      return request<PlaylistPage>(
        `/api/playlists${query ? `?${query}` : ""}`,
        init,
      );
    },
    collections: (
      params: SavedResourceBrowseParams = {},
      init?: RequestSignal,
    ) => {
      const search = new URLSearchParams();
      if (params.cursor) search.set("cursor", params.cursor);
      if (params.limit) search.set("limit", String(params.limit));
      if (params.libraryId) search.set("libraryId", params.libraryId);
      const query = search.toString();
      return request<CollectionPage>(
        `/api/collections${query ? `?${query}` : ""}`,
        init,
      );
    },
    playlist: (id: string, init?: RequestSignal) =>
      request<SavedPlaylist>(`/api/playlists/${encodeURIComponent(id)}`, init),
    collection: (id: string, init?: RequestSignal) =>
      request<Collection>(`/api/collections/${encodeURIComponent(id)}`, init),
    playlistItems: (
      id: string,
      params: SavedResourceItemsParams = {},
      init?: RequestSignal,
    ) => {
      const search = new URLSearchParams();
      if (params.cursor) search.set("cursor", params.cursor);
      if (params.limit) search.set("limit", String(params.limit));
      const query = search.toString();
      return request<PlaylistEntryPage>(
        `/api/playlists/${encodeURIComponent(id)}/items${query ? `?${query}` : ""}`,
        init,
      );
    },
    collectionItems: (
      id: string,
      params: SavedResourceItemsParams = {},
      init?: RequestSignal,
    ) => {
      const search = new URLSearchParams();
      if (params.cursor) search.set("cursor", params.cursor);
      if (params.limit) search.set("limit", String(params.limit));
      const query = search.toString();
      return request<SavedMediaPage>(
        `/api/collections/${encodeURIComponent(id)}/items${query ? `?${query}` : ""}`,
        init,
      );
    },
    createPlaylist: (body: PlaylistCreateRequest, init?: RequestSignal) =>
      request<SavedPlaylist>("/api/playlists", {
        ...init,
        method: "POST",
        body,
      }),
    createCollection: (body: CollectionCreateRequest, init?: RequestSignal) =>
      request<Collection>("/api/collections", {
        ...init,
        method: "POST",
        body,
      }),
    updatePlaylist: (
      id: string,
      body: PlaylistUpdateRequest,
      init?: RequestSignal,
    ) =>
      request<SavedPlaylist>(`/api/playlists/${encodeURIComponent(id)}`, {
        ...init,
        method: "PATCH",
        body,
      }),
    updateCollection: (
      id: string,
      body: CollectionUpdateRequest,
      init?: RequestSignal,
    ) =>
      request<Collection>(`/api/collections/${encodeURIComponent(id)}`, {
        ...init,
        method: "PATCH",
        body,
      }),
    deletePlaylist: (id: string, init?: RequestSignal) =>
      request<{ ok: boolean }>(`/api/playlists/${encodeURIComponent(id)}`, {
        ...init,
        method: "DELETE",
      }),
    deleteCollection: (id: string, init?: RequestSignal) =>
      request<{ ok: boolean }>(`/api/collections/${encodeURIComponent(id)}`, {
        ...init,
        method: "DELETE",
      }),
    mutatePlaylistItems: (
      id: string,
      body: PlaylistItemsBatchRequest,
      init?: RequestSignal,
    ) =>
      request<PlaylistItemsBatchResponse>(
        `/api/playlists/${encodeURIComponent(id)}/items:batch`,
        { ...init, method: "POST", body },
      ),
    mutateCollectionMemberships: (
      id: string,
      body: CollectionMembershipBatchRequest,
      init?: RequestSignal,
    ) =>
      request<CollectionMembershipBatchResponse>(
        `/api/collections/${encodeURIComponent(id)}/memberships:batch`,
        { ...init, method: "POST", body },
      ),
    savedViews: (
      params: SavedResourceBrowseParams = {},
      init?: RequestSignal,
    ) => {
      const search = new URLSearchParams();
      if (params.cursor) search.set("cursor", params.cursor);
      if (params.limit) search.set("limit", String(params.limit));
      if (params.libraryId) search.set("libraryId", params.libraryId);
      const query = search.toString();
      return request<SavedViewPage>(
        `/api/saved-views${query ? `?${query}` : ""}`,
        init,
      );
    },
    savedView: (id: string, init?: RequestSignal) =>
      request<SavedView>(`/api/saved-views/${encodeURIComponent(id)}`, init),
    createSavedView: (body: SavedViewCreateRequest, init?: RequestSignal) =>
      request<SavedView>("/api/saved-views", { ...init, method: "POST", body }),
    updateSavedView: (
      id: string,
      body: SavedViewUpdateRequest,
      init?: RequestSignal,
    ) =>
      request<SavedView>(`/api/saved-views/${encodeURIComponent(id)}`, {
        ...init,
        method: "PATCH",
        body,
      }),
    deleteSavedView: (id: string, init?: RequestSignal) =>
      request<{ ok: boolean }>(`/api/saved-views/${encodeURIComponent(id)}`, {
        ...init,
        method: "DELETE",
      }),
    browseSavedView: (
      id: string,
      body: SavedViewBrowseRequest = {},
      init?: RequestSignal,
    ) =>
      request<BrowseLibraryResponse>(
        `/api/saved-views/${encodeURIComponent(id)}/browse`,
        { ...init, method: "POST", body },
      ),
    dvrRules: (
      params?: { limit?: number; cursor?: string; count?: "none" | "exact" },
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params?.limit) search.set("limit", String(params.limit));
      if (params?.cursor) search.set("cursor", params.cursor);
      if (params?.count) search.set("count", params.count);
      const query = search.toString();
      return request<CursorListResponse<DVRRecordingRule>>(
        `/api/dvr/rules${query ? `?${query}` : ""}`,
        init,
      );
    },
    createDvrRule: (
      body: Partial<DVRRecordingRule> & { sourceId: string; title: string },
    ) => request<DVRRecordingRule>("/api/dvr/rules", { method: "POST", body }),
    dvrRule: (id: string) =>
      request<DVRRecordingRule>(`/api/dvr/rules/${encodeURIComponent(id)}`),
    updateDvrRule: (
      id: string,
      body: Partial<DVRRecordingRule> & { sourceId: string; title: string },
    ) =>
      request<DVRRecordingRule>(`/api/dvr/rules/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body,
      }),
    deleteDvrRule: (id: string) =>
      request<{ ok: boolean }>(`/api/dvr/rules/${encodeURIComponent(id)}`, {
        method: "DELETE",
      }),
    dvrRecordingGroups: (
      params?: { limit?: number; cursor?: string; count?: "none" | "exact" },
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params?.limit) search.set("limit", String(params.limit));
      if (params?.cursor) search.set("cursor", params.cursor);
      if (params?.count) search.set("count", params.count);
      const query = search.toString();
      return request<CursorListResponse<DVRRecordingGroup>>(
        `/api/dvr/recording-groups${query ? `?${query}` : ""}`,
        init,
      );
    },
    dvrRecordings: (
      params?: { limit?: number; cursor?: string; count?: "none" | "exact" },
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params?.limit) search.set("limit", String(params.limit));
      if (params?.cursor) search.set("cursor", params.cursor);
      if (params?.count) search.set("count", params.count);
      const query = search.toString();
      return request<CursorListResponse<DVRRecording>>(
        `/api/dvr/recordings${query ? `?${query}` : ""}`,
        init,
      );
    },
    dvrSchedule: (
      params?: { limit?: number; cursor?: string },
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params?.limit) search.set("limit", String(params.limit));
      if (params?.cursor) search.set("cursor", params.cursor);
      const query = search.toString();
      return request<CursorListResponse<DVRRecording>>(
        `/api/dvr/schedule${query ? `?${query}` : ""}`,
        init,
      );
    },
    createDvrRecording: (
      body: Partial<DVRRecording> & {
        sourceId: string;
        title: string;
        startsAt: string;
        endsAt: string;
      },
      init?: RequestSignal,
    ) =>
      request<DVRRecording>("/api/dvr/recordings", {
        ...init,
        method: "POST",
        body,
      }),
    dvrRecording: (id: string) =>
      request<DVRRecording>(`/api/dvr/recordings/${encodeURIComponent(id)}`),
    playDvrRecording: (
      id: string,
      playbackOptions: DvrPlaybackStartOptions = {},
      init?: RequestSignal,
    ) =>
      request<PlaybackResponse>(
        `/api/dvr/recordings/${encodeURIComponent(id)}/playback`,
        {
          ...init,
          method: "POST",
          body: {
            ...playbackOptions,
            intent: playbackOptions.intent ?? { quality: { mode: "automatic" } },
            clientInstanceId:
              playbackOptions.clientInstanceId ?? clientInstanceId(),
            clientProfile: playbackOptions.clientProfile ?? profile(),
          },
        },
      ).then(normalizeSessionPlayback),
    dvrRecordingStreamUrl: (id: string) =>
      resourceUrl(`/api/dvr/recordings/${encodeURIComponent(id)}/stream`),
    inspectDvrRecordingStreamUrl: (id: string, init?: RequestSignal) =>
      request<void>(`/api/dvr/recordings/${encodeURIComponent(id)}/stream`, {
        ...init,
        method: "HEAD",
      }),
    dvrRecordingHlsUrl: (id: string, resource: string) =>
      resourceUrl(
        `/api/dvr/recordings/${encodeURIComponent(id)}/hls/${encodeURIComponent(resource)}`,
      ),
    inspectDvrRecordingHlsUrl: (
      id: string,
      resource: string,
      init?: RequestSignal,
    ) =>
      request<void>(
        `/api/dvr/recordings/${encodeURIComponent(id)}/hls/${encodeURIComponent(resource)}`,
        { ...init, method: "HEAD" },
      ),
    updateDvrRecording: (
      id: string,
      body: Partial<DVRRecording> & { sourceId?: string; title?: string },
    ) =>
      request<DVRRecording>(`/api/dvr/recordings/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body,
      }),
    deleteDvrRecording: (id: string) =>
      request<{ ok: boolean }>(
        `/api/dvr/recordings/${encodeURIComponent(id)}`,
        { method: "DELETE" },
      ),
    dvrStatus: (sourceId?: string, init?: RequestSignal) => {
      const search = new URLSearchParams();
      if (sourceId) search.set("sourceId", sourceId);
      const query = search.toString();
      return request<DVRConsumerStatus>(
        `/api/dvr/status${query ? `?${query}` : ""}`,
        init,
      );
    },
    adminDvrOperationalStatus: (sourceId?: string, init?: RequestSignal) => {
      const search = new URLSearchParams();
      if (sourceId) search.set("sourceId", sourceId);
      const query = search.toString();
      return request<DVROperationalStatus>(
        `/api/admin/dvr/status${query ? `?${query}` : ""}`,
        init,
      );
    },
    users: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<User>>("/api/users", init),
    createUser: (body: UserCreateRequest) =>
      request<User>("/api/users", { method: "POST", body }),
    updateUser: (id: string, body: UserPatchRequest) =>
      request<User>(`/api/users/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body,
      }),
    deleteUser: (id: string) =>
      request<{ ok: boolean }>(`/api/users/${encodeURIComponent(id)}`, {
        method: "DELETE",
      }),
    activity: (
      options: { limit?: number; cursor?: string } = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (options.limit) search.set("limit", String(options.limit));
      if (options.cursor) search.set("cursor", options.cursor);
      const query = search.toString();
      return request<ListResponse<Job>>(
        `/api/activity${query ? `?${query}` : ""}`,
        init,
      );
    },
    cancelJob: (id: string) =>
      request<Job>(`/api/activity/${encodeURIComponent(id)}/cancel`, {
        method: "POST",
      }),
    cancelJobs: (body: { type: string }) =>
      request<JobCancelResponse>("/api/activity/cancel", {
        method: "POST",
        body,
      }),
    scheduledTasks: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<ScheduledTask>>("/api/tasks", init),
    updateScheduledTask: (id: string, body: ScheduledTaskUpdateRequest) =>
      request<ScheduledTask>(`/api/tasks/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body,
      }),
    runScheduledTask: (id: string) =>
      request<ScheduledTaskRunResponse>(
        `/api/tasks/${encodeURIComponent(id)}/run`,
        { method: "POST" },
      ),
    dashboard: (
      params?: {
        mode?: string;
        period?: string;
        userId?: string;
        libraryId?: string;
        type?: string;
        sections?: string[];
      },
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params?.mode) search.set("mode", params.mode);
      if (params?.period) search.set("period", params.period);
      if (params?.userId && params.userId !== "all")
        search.set("userId", params.userId);
      if (params?.libraryId && params.libraryId !== "all")
        search.set("libraryId", params.libraryId);
      if (params?.type && params.type !== "all")
        search.set("type", params.type);
      if (params?.sections?.length)
        search.set("sections", params.sections.join(","));
      const query = search.toString();
      return request<DashboardResponse>(
        `/api/dashboard${query ? `?${query}` : ""}`,
        init,
      );
    },
    dashboardActivity: (init?: Pick<RequestInit, "signal">) =>
      request<ServerActivityResponse>("/api/dashboard/activity", init),
    dashboardOverviewUsage: (
      params?: {
        topUsersPeriod?: string;
        topUsersLibraryId?: string;
        topUsersType?: string;
        usageHistoryUserId?: string;
        usageHistoryLibraryId?: string;
        usageHistoryType?: string;
        usageHistoryPeriod?: string;
        topPlayedUserId?: string;
        topPlayedLibraryId?: string;
        topPlayedType?: string;
        topPlayedPeriod?: string;
      },
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params?.topUsersPeriod)
        search.set("topUsersPeriod", params.topUsersPeriod);
      if (params?.topUsersLibraryId && params.topUsersLibraryId !== "all")
        search.set("topUsersLibraryId", params.topUsersLibraryId);
      if (params?.topUsersType && params.topUsersType !== "all")
        search.set("topUsersType", params.topUsersType);
      if (params?.usageHistoryUserId && params.usageHistoryUserId !== "all")
        search.set("usageHistoryUserId", params.usageHistoryUserId);
      if (
        params?.usageHistoryLibraryId &&
        params.usageHistoryLibraryId !== "all"
      )
        search.set("usageHistoryLibraryId", params.usageHistoryLibraryId);
      if (params?.usageHistoryType && params.usageHistoryType !== "all")
        search.set("usageHistoryType", params.usageHistoryType);
      if (params?.usageHistoryPeriod)
        search.set("usageHistoryPeriod", params.usageHistoryPeriod);
      if (params?.topPlayedUserId && params.topPlayedUserId !== "all")
        search.set("topPlayedUserId", params.topPlayedUserId);
      if (params?.topPlayedLibraryId && params.topPlayedLibraryId !== "all")
        search.set("topPlayedLibraryId", params.topPlayedLibraryId);
      if (params?.topPlayedType && params.topPlayedType !== "all")
        search.set("topPlayedType", params.topPlayedType);
      if (params?.topPlayedPeriod)
        search.set("topPlayedPeriod", params.topPlayedPeriod);
      const query = search.toString();
      return request<DashboardOverviewUsageResponse>(
        `/api/dashboard/overview-usage${query ? `?${query}` : ""}`,
        init,
      );
    },
    playbackHistory: (
      params?: PlaybackHistoryParams,
      init?: Pick<RequestInit, "signal">,
    ) => {
      const query = playbackHistoryQuery(params);
      return request<CursorListResponse<PlaybackSession>>(
        `/api/playback/history${query ? `?${query}` : ""}`,
        init,
      );
    },
    playbackHistoryExportUrl: (params?: PlaybackHistoryFilterParams) => {
      const query = playbackHistoryQuery(params);
      return resourceUrl(
        `/api/playback/history/export.csv${query ? `?${query}` : ""}`,
      );
    },
    liveTv: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<LiveTVSourceSummary>>("/api/live-tv", init),
    libraryChannels: (
      params: LibraryChannelListParams = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.cursor) search.set("cursor", params.cursor);
      if (params.limit) search.set("limit", String(params.limit));
      const query = search.toString();
      return request<LibraryChannelListResponse>(
        `/api/library-channels${query ? `?${query}` : ""}`,
        init,
      );
    },
    libraryChannelGuide: (
      channelId: string,
      params: LibraryChannelGuideParams = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.from) search.set("from", params.from);
      if (params.to) search.set("to", params.to);
      if (params.cursor) search.set("cursor", params.cursor);
      if (params.limit) search.set("limit", String(params.limit));
      const query = search.toString();
      return request<LibraryChannelGuide>(
        `/api/library-channels/${encodeURIComponent(channelId)}/guide${query ? `?${query}` : ""}`,
        init,
      );
    },
    libraryChannelsGuide: (
      params: LibraryChannelGuideParams = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.from) search.set("from", params.from);
      if (params.to) search.set("to", params.to);
      if (params.cursor) search.set("cursor", params.cursor);
      if (params.limit) search.set("limit", String(params.limit));
      const query = search.toString();
      return request<LibraryChannelsGuide>(
        `/api/library-channels/guide${query ? `?${query}` : ""}`,
        init,
      );
    },
    tuneLibraryChannel: (
      channelId: string,
      body: LibraryChannelPlaybackStartOptions = {},
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<LibraryChannelTuneResponse>(
        `/api/library-channels/${encodeURIComponent(channelId)}/tune`,
        {
          ...init,
          method: "POST",
          body: {
            ...body,
            intent: body.intent ?? { quality: { mode: "automatic" } },
            clientInstanceId: body.clientInstanceId ?? clientInstanceId(),
            clientProfile: body.clientProfile ?? profile(),
          },
        },
      ).then((response) => ({
        ...response,
        playback: response.playback
          ? normalizeSessionPlayback(response.playback)
          : response.playback,
      })),
    libraryChannelHlsUrl: (
      channelId: string,
      resource: "playlist.m3u8" | "segment",
    ) =>
      resourceUrl(
        `/api/library-channels/${encodeURIComponent(channelId)}/hls/${encodeURIComponent(resource)}`,
      ),
    inspectLibraryChannelHlsUrl: (
      channelId: string,
      resource: "playlist.m3u8" | "segment",
      init?: RequestSignal,
    ) =>
      request<void>(
        `/api/library-channels/${encodeURIComponent(channelId)}/hls/${encodeURIComponent(resource)}`,
        { ...init, method: "HEAD" },
      ),
    libraryChannelLogoUrl: (assetId: string) =>
      resourceUrl(`/api/library-channels/logos/${encodeURIComponent(assetId)}`),
    adminLibraryChannels: (
      params: LibraryChannelListParams = {},
      init?: Pick<RequestInit, "signal">,
    ) => {
      const search = new URLSearchParams();
      if (params.cursor) search.set("cursor", params.cursor);
      if (params.limit) search.set("limit", String(params.limit));
      const query = search.toString();
      return request<AdminLibraryChannelListResponse>(
        `/api/admin/library-channels${query ? `?${query}` : ""}`,
        init,
      );
    },
    createAdminLibraryChannel: (body: LibraryChannelConfigurationRequest) =>
      request<LibraryChannelAggregate>("/api/admin/library-channels", {
        method: "POST",
        body,
      }),
    adminLibraryChannel: (
      channelId: string,
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<LibraryChannelAggregate>(
        `/api/admin/library-channels/${encodeURIComponent(channelId)}`,
        init,
      ),
    replaceAdminLibraryChannel: (
      channelId: string,
      body: LibraryChannelConfigurationRequest,
    ) =>
      request<LibraryChannelAggregate>(
        `/api/admin/library-channels/${encodeURIComponent(channelId)}`,
        { method: "PUT", body },
      ),
    deleteAdminLibraryChannel: (channelId: string, expectedRevision: number) =>
      request<SuccessResponse>(
        `/api/admin/library-channels/${encodeURIComponent(channelId)}?expectedRevision=${encodeURIComponent(String(expectedRevision))}`,
        { method: "DELETE" },
      ),
    reorderAdminLibraryChannels: (body: LibraryChannelReorderRequest) =>
      request<AdminLibraryChannelListResponse>(
        "/api/admin/library-channels/reorder",
        { method: "POST", body },
      ),
    adminLibraryChannelTemplates: (init?: Pick<RequestInit, "signal">) =>
      request<LibraryChannelTemplatesResponse>(
        "/api/admin/library-channels/templates",
        init,
      ),
    adminLibraryChannelTemplateApplicability: (
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<LibraryChannelApplicabilityResponse>(
        "/api/admin/library-channels/templates/applicability",
        init,
      ),
    restoreAdminLibraryChannelDefaults: (
      body: LibraryChannelRestoreDefaultsRequest,
    ) =>
      request<LibraryChannelRestoreDefaultsResponse>(
        "/api/admin/library-channels/restore-defaults",
        { method: "POST", body },
      ),
    adminLibraryChannelHealth: (init?: Pick<RequestInit, "signal">) =>
      request<LibraryChannelHealthResponse>(
        "/api/admin/library-channels/health",
        init,
      ),
    regenerateAdminLibraryChannel: (channelId: string) =>
      request<LibraryChannelGeneration>(
        `/api/admin/library-channels/${encodeURIComponent(channelId)}/regenerate`,
        { method: "POST" },
      ),
    uploadAdminLibraryChannelLogo: (form: FormData, init?: RequestSignal) =>
      formRequest<LibraryChannelLogoAsset>(
        "/api/admin/library-channels/logos",
        form,
        "POST",
        init,
      ),
    deleteAdminLibraryChannelLogo: (assetId: string) =>
      request<SuccessResponse>(
        `/api/admin/library-channels/logos/${encodeURIComponent(assetId)}`,
        { method: "DELETE" },
      ),
    adminLiveTvSources: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<LiveTVSource>>("/api/live-tv/sources", init),
    adminLiveTvSource: (id: string, init?: Pick<RequestInit, "signal">) =>
      request<LiveTVSource>(
        `/api/live-tv/sources/${encodeURIComponent(id)}`,
        init,
      ),
    createLiveTvSource: (body: LiveTVSourceRequest) =>
      request<LiveTVSource>("/api/live-tv/sources", { method: "POST", body }),
    testAddLiveTvSource: (body: LiveTVSourceRequest) =>
      request<LiveTVSource>("/api/live-tv/sources/test-add", {
        method: "POST",
        body,
      }),
    updateLiveTvSource: (id: string, body: LiveTVSourceRequest) =>
      request<LiveTVSource>(`/api/live-tv/sources/${encodeURIComponent(id)}`, {
        method: "PATCH",
        body,
      }),
    deleteLiveTvSource: (id: string) =>
      request<{ ok: boolean }>(
        `/api/live-tv/sources/${encodeURIComponent(id)}`,
        { method: "DELETE" },
      ),
    refreshLiveTvSource: (id: string) =>
      request<LiveTVSource>(
        `/api/live-tv/sources/${encodeURIComponent(id)}/refresh`,
        { method: "POST" },
      ),
    discoverHDHomeRunSources: () =>
      request<ListResponse<HDHomeRunDiscoveryCandidate>>(
        "/api/live-tv/sources/hdhomerun/discover",
        { method: "POST" },
      ),
    liveTvGuide: (
      sourceId: string,
      params: LiveTVGuideParams = {},
      init?: RequestSignal,
    ) => {
      const search = new URLSearchParams();
      if (params.from) search.set("from", params.from);
      if (params.hours) search.set("hours", String(params.hours));
      if (params.limit) search.set("limit", String(params.limit));
      if (params.cursor) search.set("cursor", params.cursor);
      if (params.count) search.set("count", params.count);
      if (params.query) search.set("query", params.query);
      if (params.filter) search.set("filter", params.filter);
      if (params.sort) search.set("sort", params.sort);
      if (params.order) search.set("order", params.order);
      if (params.group) search.set("group", params.group);
      const query = search.toString();
      return request<LiveTVGuideResponse>(
        `/api/live-tv/sources/${encodeURIComponent(sourceId)}/guide${query ? `?${query}` : ""}`,
        init,
      );
    },
    liveTvChannels: (
      sourceId: string,
      params: LiveTVChannelBrowseParams = {},
      init?: RequestSignal,
    ) => {
      const search = new URLSearchParams();
      if (params.limit) search.set("limit", String(params.limit));
      if (params.cursor) search.set("cursor", params.cursor);
      if (params.count) search.set("count", params.count);
      if (params.query) search.set("query", params.query);
      if (params.favoritesOnly) search.set("favoritesOnly", "true");
      if (params.group) search.set("group", params.group);
      const query = search.toString();
      return request<LiveTVChannelPageResponse>(
        `/api/live-tv/sources/${encodeURIComponent(sourceId)}/channels${query ? `?${query}` : ""}`,
        init,
      );
    },
    liveTvStreams: () =>
      request<ListResponse<PlaybackSession>>("/api/live-tv/streams"),
    liveTvHlsPlaylistUrl: (channelId: string) =>
      resourceUrl(
        `/api/live-tv/hls/${encodeURIComponent(channelId)}/playlist.m3u8`,
      ),
    liveTvHlsItemUrl: (channelId: string) =>
      resourceUrl(`/api/live-tv/hls/${encodeURIComponent(channelId)}/item`),
    liveTvHlsSegmentUrl: (channelId: string) =>
      resourceUrl(`/api/live-tv/hls/${encodeURIComponent(channelId)}/segment`),
    liveTvLogoUrl: (channelId: string) =>
      resourceUrl(`/api/live-tv/logos/${encodeURIComponent(channelId)}`),
    liveTvStreamUrl: (channelId: string) =>
      resourceUrl(`/api/live-tv/streams/${encodeURIComponent(channelId)}`),
    liveTvChannel: (channelId: string, init?: RequestSignal) =>
      request<LiveTVChannel>(
        `/api/live-tv/channels/${encodeURIComponent(channelId)}`,
        init,
      ),
    updateLiveTvChannel: (channelId: string, body: LiveTVChannelStateRequest) =>
      request<LiveTVChannel>(
        `/api/live-tv/channels/${encodeURIComponent(channelId)}`,
        { method: "PATCH", body },
      ),
    startLiveTvPlayback: (
      channelId: string,
      playbackOptions: LiveTvPlaybackStartOptions = {},
      init?: RequestSignal,
    ) =>
      request<PlaybackResponse>("/api/live-tv/play", {
        ...init,
        method: "POST",
        body: {
          channelId,
          clientInstanceId: clientInstanceId(),
          clientProfile: playbackOptions.clientProfile ?? profile(),
          intent: playbackOptions.intent ?? { quality: { mode: "automatic" } },
        },
      }).then(normalizeSessionPlayback),
    openLiveTvStream: (
      channelId: string,
      playbackOptions: LiveTvPlaybackStartOptions = {},
      init?: RequestSignal,
    ) =>
      request<PlaybackResponse>(
        `/api/live-tv/streams/${encodeURIComponent(channelId)}/open`,
        {
          ...init,
          method: "POST",
          body: {
            channelId,
            clientInstanceId: clientInstanceId(),
            clientProfile: playbackOptions.clientProfile ?? profile(),
            intent: playbackOptions.intent ?? { quality: { mode: "automatic" } },
          },
        },
      ).then(normalizeSessionPlayback),
    closeLiveTvStream,
  };
}

export type PorticoClient = ReturnType<typeof createPorticoClient>;

function isHostedTerminalMutation(path: string, method: string): boolean {
  const pathname = path.split("?", 1)[0] ?? path;
  const verb = method.toUpperCase();
  if (
    verb === "POST" &&
    new Set([
      "/api/auth/logout",
      "/api/auth/sessions/refresh",
      "/api/auth/sessions/revoke",
      "/api/auth/password-reset/complete",
      "/api/auth/mfa/setup",
      "/api/auth/mfa/enable",
      "/api/auth/mfa/recovery-codes/rotate",
      "/api/auth/mfa/disable",
      "/api/account/me/image",
      "/api/account/me/password",
      "/api/operator/users/actions",
      "/api/operator/users/mfa-reset",
    ]).has(pathname)
  )
    return true;
  if (verb === "DELETE" && pathname === "/api/account/me") return true;
  if (
    verb === "DELETE" &&
    new Set([
      "/api/account/me/image",
    ]).has(pathname)
  )
    return true;
  if (
    verb === "DELETE" &&
    /^\/api\/account\/(?:profiles|devices)\/[^/]+(?:\/pin)?$/.test(pathname)
  )
    return true;
  if (
    verb === "PUT" &&
    /^\/api\/account\/profiles\/[^/]+\/pin$/.test(pathname)
  )
    return true;
  if (
    (verb === "PATCH" || verb === "DELETE") &&
    /^\/api\/(?:account\/)?servers\/[^/]+(?:\/membership|\/members\/[^/]+|\/invites\/[^/]+)?$/.test(
      pathname,
    )
  )
    return true;
  return false;
}

export function createHostedServicesClient(
  options: HostedServicesClientOptions = {},
) {
  const inFlightJSONRequests = new Map<string, InFlightJSONRequest>();
  let apiOwnedCSRFToken = "";
  const hostedOptions: HostedServicesClientOptions = {
    ...options,
    csrfToken: () =>
      apiOwnedCSRFToken || String(resolveValue(options.csrfToken, "")).trim(),
    onCSRFToken: (token) => {
      apiOwnedCSRFToken = token;
      options.onCSRFToken?.(token);
    },
  };
  const clearCSRFToken = () => {
    apiOwnedCSRFToken = "";
    options.onCSRFToken?.("");
  };
  const rawRequest = <T>(path: string, init: ApiRequestInit = {}) =>
    hostedRequest<T>(path, init, hostedOptions, inFlightJSONRequests);
  const pendingTerminalMutations = new Map<
    string,
    PendingHostedTerminalMutation
  >();
  const persistTerminalMutation = async (
    record: PendingHostedTerminalMutation,
  ) => {
    await options.terminalMutationDurabilityAdapter?.save(record);
    pendingTerminalMutations.set(record.idempotencyKey, record);
  };
  const removeTerminalMutation = async (idempotencyKey: string) => {
    await options.terminalMutationDurabilityAdapter?.remove(idempotencyKey);
    pendingTerminalMutations.delete(idempotencyKey);
  };
  const reconcileTerminalMutationByKey = (
    idempotencyKey: string,
    init: ApiRequestInit = {},
  ) =>
    rawRequest<{
      outcome: "committed";
      receipt: Readonly<Record<string, unknown>>;
    }>("/api/account/terminal-mutations", {
      ...init,
      method: "GET",
      headers: { "Idempotency-Key": idempotencyKey },
      operationClass: "interactive read",
    });
  const terminalFormRequest = async <T>(
    path: string,
    form: FormData,
    init?: RequestSignal,
  ): Promise<T> => {
    const idempotencyKey = createRequestId(options.requestId);
    const record: PendingHostedTerminalMutation = {
      version: "v1",
      idempotencyKey,
      method: "POST",
      path: canonicalAPIPath(path),
      createdAt: new Date().toISOString(),
    };
    await persistTerminalMutation(record);
    try {
      const result = await hostedFormRequest<T>(
        path,
        form,
        "POST",
        { ...hostedOptions, requestId: () => idempotencyKey },
        init,
      );
      await removeTerminalMutation(idempotencyKey);
      return result;
    } catch (error) {
      if (!isAmbiguousPorticoError(error)) {
        await removeTerminalMutation(idempotencyKey);
        throw error;
      }
      try {
        const reconciled = await reconcileTerminalMutationByKey(
          idempotencyKey,
          init,
        );
        if (reconciled.outcome === "committed") {
          await removeTerminalMutation(idempotencyKey);
          throw new HostedTerminalMutationCommittedError(
            idempotencyKey,
            reconciled.receipt,
          );
        }
      } catch (reconciliationError) {
        if (reconciliationError instanceof HostedTerminalMutationCommittedError)
          throw reconciliationError;
      }
      throw new HostedTerminalMutationUncertainError(error, record);
    }
  };
  const withMutationIdempotency = (
    init: ApiRequestInit = {},
  ): ApiRequestInit => {
    const method = String(init.method ?? "GET").toUpperCase();
    if (method === "GET" || method === "HEAD" || method === "OPTIONS")
      return init;
    const headers = normalizeHeaders(init.headers);
    if (!headerValue(headers, "idempotency-key"))
      headers["Idempotency-Key"] = createRequestId(options.requestId);
    return { ...init, headers };
  };
  const publicSystemRequest = (init: ApiRequestInit = {}) =>
    hostedRequest<import("./types.js").HostedSystemInfo>(
      "/api/system",
      init,
      {
        ...hostedOptions,
        accessToken: undefined,
        csrfToken: undefined,
      },
      inFlightJSONRequests,
    );
  let compatibilityCheck:
    | Promise<import("./compatibility.js").HostedServicesCompatibility>
    | undefined;
  let documentSigningKeysCheck:
    | Promise<import("./types.js").PorticoDocumentSigningKeySet>
    | undefined;
  const checkCompatibility = (init: ApiRequestInit = {}) => {
    compatibilityCheck ??= publicSystemRequest(init)
      .then(assertHostedServicesCompatibility)
      .catch((error) => {
        compatibilityCheck = undefined;
        throw error;
      });
    return compatibilityCheck;
  };
  const request = async <T>(path: string, init: ApiRequestInit = {}) => {
    if (path === "/api/system") return publicSystemRequest(init) as Promise<T>;
    await awaitWithAbort(
      checkCompatibility(requestTransportOptions(init)),
      init.signal ?? undefined,
    );
    const prepared = withMutationIdempotency(init);
    const method = String(prepared.method ?? "GET").toUpperCase();
    if (!isHostedTerminalMutation(path, method))
      return rawRequest<T>(path, prepared);
    const idempotencyKey = headerValue(
      normalizeHeaders(prepared.headers),
      "idempotency-key",
    );
    const record: PendingHostedTerminalMutation = {
      version: "v1",
      idempotencyKey,
      method,
      path: canonicalAPIPath(path),
      createdAt: new Date().toISOString(),
    };
    // When a durable adapter is configured, failure to persist must stop the
    // request before dispatch. Otherwise a process death could lose the only
    // safe reconciliation identity.
    await persistTerminalMutation(record);
    try {
      const result = await rawRequest<T>(path, prepared);
      await removeTerminalMutation(idempotencyKey);
      return result;
    } catch (error) {
      if (!isAmbiguousPorticoError(error)) {
        await removeTerminalMutation(idempotencyKey);
        throw error;
      }
      try {
        const reconciled = await reconcileTerminalMutationByKey(
          idempotencyKey,
          requestTransportOptions(prepared),
        );
        if (reconciled.outcome === "committed") {
          await removeTerminalMutation(idempotencyKey);
          throw new HostedTerminalMutationCommittedError(
            idempotencyKey,
            reconciled.receipt,
          );
        }
      } catch (reconciliationError) {
        if (reconciliationError instanceof HostedTerminalMutationCommittedError)
          throw reconciliationError;
        // A missing or temporarily unavailable receipt does not prove rollback;
        // retain the durable key for a later authenticated reconciliation.
      }
      throw new HostedTerminalMutationUncertainError(error, record);
    }
  };
  const recheckAfterAuthentication = async <T>(
    response: T,
    init: ApiRequestInit = {},
  ): Promise<T> => {
    compatibilityCheck = undefined;
    await checkCompatibility(init);
    return response;
  };
  const normalizeHostedProfileDirectory = (
    directory: import("./types.js").HostedAccountProfileDirectory,
  ) => ({
    ...directory,
    profiles: directory.profiles.map((profile) => parsePorticoProfile(profile)),
  });
  const normalizeHostedProfileMutation = (
    response: import("./types.js").HostedAccountProfileMutationResponse,
  ) => ({
    ...response,
    profile: parsePorticoProfile(response.profile),
  });
  const profileAdministrationHeaders = (
    proof: import("./types.js").HostedProfileAdministrationProof,
  ): Record<string, string> => {
    const token = String(proof?.token ?? "").trim();
    if (!token) throw new TypeError("profile administration proof is required");
    return { "X-Portico-Profile-Admin": token };
  };
  const loadPendingTerminalMutations = async () => {
    const durable =
      (await options.terminalMutationDurabilityAdapter?.load?.()) ?? [];
    for (const record of durable) {
      if (
        record?.version !== "v1" ||
        typeof record.idempotencyKey !== "string" ||
        record.idempotencyKey.length < 8 ||
        record.idempotencyKey.length > 128 ||
        !/^[A-Za-z0-9._:-]+$/.test(record.idempotencyKey) ||
        typeof record.method !== "string" ||
        typeof record.path !== "string"
      )
        continue;
      pendingTerminalMutations.set(record.idempotencyKey, { ...record });
    }
    return [...pendingTerminalMutations.values()];
  };
  return {
    request,
    hostedApiUrl: (path: string) =>
      `${resolveBase(options.hostedApiBaseUrl, "https://api.getportico.tv")}${path}`,
    system: publicSystemRequest,
    checkCompatibility,
    pendingAccountTerminalMutations: loadPendingTerminalMutations,
    reconcilePendingAccountTerminalMutations: async (init?: RequestSignal) => {
      await awaitWithAbort(
        checkCompatibility(requestTransportOptions(init ?? {})),
        init?.signal ?? undefined,
      );
      const records = await loadPendingTerminalMutations();
      const outcomes: Array<{
        record: PendingHostedTerminalMutation;
        outcome: "committed" | "pending";
        receipt?: Readonly<Record<string, unknown>>;
      }> = [];
      for (const record of records) {
        try {
          const reconciled = await reconcileTerminalMutationByKey(
            record.idempotencyKey,
            init,
          );
          if (reconciled.outcome === "committed") {
            await removeTerminalMutation(record.idempotencyKey);
            outcomes.push({
              record,
              outcome: "committed",
              receipt: reconciled.receipt,
            });
            continue;
          }
        } catch {
          // A missing/temporarily unavailable receipt remains pending. The
          // stable key is retained for the next authenticated recovery pass.
        }
        outcomes.push({ record, outcome: "pending" });
      }
      return outcomes;
    },
    documentSigningKeys: (init?: RequestSignal) => {
      documentSigningKeysCheck ??= request<import("./types.js").PorticoDocumentSigningKeySet>(
        "/api/signing-keys",
      ).catch((error) => {
        documentSigningKeysCheck = undefined;
        throw error;
      });
      return awaitWithAbort(documentSigningKeysCheck, init?.signal ?? undefined);
    },
    me: async (init?: RequestSignal) => {
      const response = await request<
        import("./types.js").PorticoAccountAuthResponse
      >("/api/auth/me", init);
      if (!response.authenticated) clearCSRFToken();
      return response;
    },
    account: () =>
      request<import("./types.js").PorticoAccountResponse>("/api/account/me"),
    reconcileAccountTerminalMutation: (
      idempotencyKey: string,
      init?: RequestSignal,
    ) => {
      const key = String(idempotencyKey ?? "").trim();
      if (key.length < 8 || key.length > 128 || !/^[A-Za-z0-9._:-]+$/.test(key))
        throw new TypeError("A valid terminal mutation idempotency key is required.");
      return request<{
        outcome: "committed";
        receipt: {
          receiptId: string;
          auditEventId: string;
          action: string;
          targetType: string;
          targetId: string;
          actorType: string;
          actorId: string;
          createdAt: string;
        };
      }>("/api/account/terminal-mutations", {
        ...init,
        headers: { "Idempotency-Key": key },
      });
    },
    updateAccount: (body: {
      username: string;
      email?: string;
      preferences?: import("./types.js").UserPreferences;
    }) =>
      request<{
        user: import("./types.js").PorticoAccountAuthResponse["user"];
      }>("/api/account/me", { method: "PATCH", body }),
    uploadAccountImage: (form: FormData, init?: RequestSignal) =>
      terminalFormRequest<{
        user: import("./types.js").PorticoAccountAuthResponse["user"];
      }>("/api/account/me/image", form, init),
    deleteAccountImage: (init?: RequestSignal) =>
      request<{
        user: import("./types.js").PorticoAccountAuthResponse["user"];
      }>("/api/account/me/image", { ...init, method: "DELETE" }),
    changePassword: (body: { currentPassword: string; newPassword: string }) =>
      request<{ ok: boolean }>("/api/account/me/password", {
        method: "POST",
        body,
      }),
    register: async (
      body: { email: string; username: string; password: string },
      init?: RequestSignal,
    ) =>
      recheckAfterAuthentication(
        await request<{
          user: import("./types.js").PorticoAccountAuthResponse["user"];
        }>("/api/auth/register", { ...init, method: "POST", body }),
        init,
      ),
    login: async (
      body: {
        login: string;
        password: string;
        installationId: string;
        mfaCode?: string;
        recoveryCode?: string;
        deviceName?: string;
        devicePlatform?: string;
      },
      init?: RequestSignal,
    ) =>
      recheckAfterAuthentication(
        await request<import("./types.js").PorticoAccountAuthResponse>(
          "/api/auth/login",
          { ...init, method: "POST", body },
        ),
        init,
      ),
    createNativeSession: async (
      body: {
        login: string;
        password: string;
        installationId: string;
        mfaCode?: string;
        recoveryCode?: string;
        deviceName: string;
        devicePlatform: string;
      },
      init?: RequestSignal,
    ) =>
      recheckAfterAuthentication(
        await request<import("./types.js").PorticoNativeSessionResponse>(
          "/api/auth/sessions",
          { ...init, method: "POST", body },
        ),
        init,
      ),
    refreshNativeSession: (body: {
      refreshToken: string;
      rotationKey: string;
      installationId?: string;
    }) =>
      request<import("./types.js").PorticoNativeSessionResponse>(
        "/api/auth/sessions/refresh",
        { method: "POST", body },
      ),
    revokeNativeSession: (refreshToken: string) =>
      request<{ ok: boolean }>("/api/auth/sessions/revoke", {
        method: "POST",
        body: { refreshToken },
      }),
    logout: async () => {
      const response = await request<{ ok: boolean }>("/api/auth/logout", {
        method: "POST",
      });
      clearCSRFToken();
      return response;
    },
    deleteAccount: (body: { password: string }, init?: RequestSignal) =>
      request<{ ok: boolean; deletedAt: string }>("/api/account/me", {
        ...init,
        method: "DELETE",
        body,
      }),
    profiles: async (init?: RequestSignal) =>
      normalizeHostedProfileDirectory(
        await request<import("./types.js").HostedAccountProfileDirectory>(
          "/api/account/profiles",
          init,
        ),
      ),
    profile: async (profileId: string, init?: RequestSignal) =>
      parsePorticoProfile(
        await request<import("./types.js").HostedAccountProfile>(
          `/api/account/profiles/${encodeURIComponent(profileId)}`,
          init,
        ),
      ),
    createProfileAdministrationSession: (
      body: import("./types.js").HostedProfileAdministrationSessionRequest,
      init?: RequestSignal,
    ) =>
      request<import("./types.js").HostedProfileAdministrationSessionResponse>(
        "/api/account/profile-administration/sessions",
        { ...init, method: "POST", body },
      ),
    createProfile: async (
      body: import("./types.js").HostedAccountProfileCreateRequest,
      administration: import("./types.js").HostedProfileAdministrationProof,
      init?: RequestSignal,
    ) =>
      normalizeHostedProfileMutation(
        await request<
          import("./types.js").HostedAccountProfileMutationResponse
        >("/api/account/profiles", {
          ...init,
          method: "POST",
          headers: profileAdministrationHeaders(administration),
          body,
        }),
      ),
    updateProfile: async (
      profileId: string,
      body: import("./types.js").HostedAccountProfileUpdateRequest,
      administration: import("./types.js").HostedProfileAdministrationProof,
      init?: RequestSignal,
    ) =>
      normalizeHostedProfileMutation(
        await request<
          import("./types.js").HostedAccountProfileMutationResponse
        >(`/api/account/profiles/${encodeURIComponent(profileId)}`, {
          ...init,
          method: "PUT",
          headers: profileAdministrationHeaders(administration),
          body,
        }),
      ),
    reorderProfiles: async (
      body: import("./types.js").HostedAccountProfileReorderRequest,
      administration: import("./types.js").HostedProfileAdministrationProof,
      init?: RequestSignal,
    ) =>
      normalizeHostedProfileDirectory(
        await request<import("./types.js").HostedAccountProfileDirectory>(
          "/api/account/profiles/reorder",
          {
            ...init,
            method: "POST",
            headers: profileAdministrationHeaders(administration),
            body,
          },
        ),
      ),
    deleteProfile: (
      profileId: string,
      administration: import("./types.js").HostedProfileAdministrationProof,
      init?: RequestSignal,
    ) =>
      request<import("./types.js").HostedAccountProfileDeleteResponse>(
        `/api/account/profiles/${encodeURIComponent(profileId)}`,
        {
          ...init,
          method: "DELETE",
          headers: profileAdministrationHeaders(administration),
        },
      ),
    setProfilePIN: (
      profileId: string,
      body: import("./types.js").HostedProfilePINSetRequest,
      administration?: import("./types.js").HostedProfileAdministrationProof,
      init?: RequestSignal,
    ) =>
      request<import("./types.js").HostedProfilePINChangeResponse>(
        `/api/account/profiles/${encodeURIComponent(profileId)}/pin`,
        {
          ...init,
          method: "PUT",
          headers: administration
            ? profileAdministrationHeaders(administration)
            : undefined,
          body,
        },
      ),
    clearProfilePIN: (
      profileId: string,
      body: import("./types.js").HostedProfilePINClearRequest,
      administration: import("./types.js").HostedProfileAdministrationProof,
      init?: RequestSignal,
    ) =>
      request<{ ok: boolean }>(
        `/api/account/profiles/${encodeURIComponent(profileId)}/pin`,
        {
          ...init,
          method: "DELETE",
          headers: profileAdministrationHeaders(administration),
          body,
        },
      ),
    createProfileSelectionEnvelope: (
      profileId: string,
      body: import("./types.js").HostedProfileSelectionAssertionRequest,
      init?: RequestSignal,
    ) =>
      request<import("./types.js").HostedProfileSelectionEnvelope>(
        `/api/account/profiles/${encodeURIComponent(profileId)}/selection-assertions`,
        { ...init, method: "POST", body },
      ),
    requestPasswordReset: (body: { email: string }, init?: RequestSignal) =>
      request<{ ok: boolean; resetToken?: string }>(
        "/api/auth/password-reset/start",
        { ...init, method: "POST", body },
      ),
    completePasswordReset: (
      body: { token: string; password: string },
      init?: RequestSignal,
    ) =>
      request<{ ok: boolean }>("/api/auth/password-reset/complete", {
        ...init,
        method: "POST",
        body,
      }),
    createTVSetupSession: (
      body: import("./types.js").HostedTVSetupSessionRequest,
    ) =>
      request<import("./types.js").HostedTVSetupSession>(
        "/api/tv-setup/sessions",
        { method: "POST", body },
      ),
    tvSetupSession: (setupSessionId: string, pollSecret: string) =>
      request<import("./types.js").HostedTVSetupSession>(
        `/api/tv-setup/sessions/${encodeURIComponent(setupSessionId)}`,
        {
          headers: { "X-Portico-TV-Setup-Poll-Secret": pollSecret },
        },
      ),
    previewTVSetupSession: (
      body: import("./types.js").HostedTVSetupPreviewRequest,
      init?: RequestSignal,
    ) =>
      request<import("./types.js").HostedTVSetupPreviewResponse>(
        "/api/tv-setup/preview",
        { ...init, method: "POST", body },
      ),
    authorizeTVSetupGrant: (
      body: import("./types.js").HostedTVSetupGrantRequest,
      init?: RequestSignal,
    ) =>
      request<import("./types.js").HostedTVSetupGrantResponse>(
        "/api/tv-setup/grants",
        { ...init, method: "POST", body },
      ),
    redeemTVSetupSession: (setupSessionId: string, pollSecret: string) =>
      request<{ ok: boolean }>(
        `/api/tv-setup/sessions/${encodeURIComponent(setupSessionId)}/redeem`,
        {
          method: "POST",
          headers: { "X-Portico-TV-Setup-Poll-Secret": pollSecret },
        },
      ),
    createDeviceAuthorizationSession: (
      body: import("./types.js").HostedDeviceAuthorizationSessionRequest,
    ) =>
      request<import("./types.js").HostedDeviceAuthorizationSession>(
        "/api/device-authorization/sessions",
        { method: "POST", body },
      ),
    pollDeviceAuthorizationSession: (
      authorizationSessionId: string,
      deviceCode: string,
    ) =>
      request<import("./types.js").HostedDeviceAuthorizationStatus>(
        `/api/device-authorization/sessions/${encodeURIComponent(authorizationSessionId)}`,
        {
          headers: { "X-Portico-Device-Code": deviceCode },
        },
      ),
    previewDeviceAuthorization: (
      body: import("./types.js").HostedDeviceAuthorizationPreviewRequest,
      init?: RequestSignal,
    ) =>
      request<import("./types.js").HostedDeviceAuthorizationPreviewResponse>(
        "/api/device-authorizations/preview",
        { ...init, method: "POST", body },
      ),
    decideDeviceAuthorization: (
      body: import("./types.js").HostedDeviceAuthorizationDecisionRequest,
      init?: RequestSignal,
    ) =>
      request<import("./types.js").HostedDeviceAuthorizationDecisionResponse>(
        "/api/device-authorizations",
        { ...init, method: "POST", body },
      ),
    redeemDeviceAuthorizationSession: (
      authorizationSessionId: string,
      deviceCode: string,
    ) =>
      request<import("./types.js").HostedDeviceAuthorizationRedeemResponse>(
        `/api/device-authorization/sessions/${encodeURIComponent(authorizationSessionId)}/redeem`,
        {
          method: "POST",
          headers: { "X-Portico-Device-Code": deviceCode },
        },
      ),
    mfaSetup: (body: { password: string }) =>
      request<{ secret: string; otpauthUrl: string; enrollmentToken: string }>(
        "/api/auth/mfa/setup",
        { method: "POST", body },
      ),
    mfaStatus: (init?: RequestSignal) =>
      request<import("./types.js").PorticoMFAStatus>(
        "/api/auth/mfa/status",
        init,
      ),
    mfaEnable: (
      body: { code: string; enrollmentToken: string },
      init?: RequestSignal,
    ) =>
      request<{ enabled: boolean; recoveryCodes: string[] }>(
        "/api/auth/mfa/enable",
        { ...init, method: "POST", body },
      ),
    rotateMFARecoveryCodes: (body: { code: string }, init?: RequestSignal) =>
      request<{ enabled: boolean; recoveryCodes: string[] }>(
        "/api/auth/mfa/recovery-codes/rotate",
        { ...init, method: "POST", body },
      ),
    mfaDisable: (body: { password: string; code: string }) =>
      request<{ ok: boolean }>("/api/auth/mfa/disable", {
        method: "POST",
        body,
      }),
    devices: (
      params: {
        limit?: number;
        cursor?: string;
        count?: "none" | "exact";
      } = {},
      init?: RequestSignal,
    ) =>
      request<CursorListResponse<import("./types.js").PorticoDevice>>(
        `/api/account/devices${hostedCursorQuery(params)}`,
        init,
      ),
    revokeDevice: (id: string, init?: RequestSignal) =>
      request<{ ok: boolean }>(
        `/api/account/devices/${encodeURIComponent(id)}`,
        { ...init, method: "DELETE" },
      ),
    servers: (
      params: {
        limit?: number;
        cursor?: string;
        count?: "none" | "exact";
      } = {},
      init?: RequestSignal,
    ) =>
      request<CursorListResponse<import("./types.js").HostedServer>>(
        `/api/account/servers${hostedCursorQuery(params)}`,
        init,
      ),
    server: (serverId: string) =>
      request<import("./types.js").HostedServer>(
        `/api/account/servers/${encodeURIComponent(serverId)}`,
      ),
    updateServer: (
      serverId: string,
      body: Partial<
        Pick<
          import("./types.js").HostedServer,
          "name" | "remoteAccessEnabled" | "preferredAuthMode"
        >
      >,
    ) =>
      request<import("./types.js").HostedServer>(
        `/api/account/servers/${encodeURIComponent(serverId)}`,
        { method: "PATCH", body },
      ),
    createServerDeletionProof: (
      serverId: string,
      body: import("./types.js").HostedServerDeletionProofRequest,
      init?: RequestSignal,
    ) =>
      request<import("./types.js").HostedServerDeletionProofResponse>(
        `/api/account/servers/${encodeURIComponent(serverId)}/deletion-proofs`,
        { ...init, method: "POST", body },
      ),
    createServerRestoreAuthorization: (
      serverId: string,
      body: import("./types.js").HostedServerRestoreAuthorizationRequest,
      init?: RequestSignal,
    ) =>
      request<import("./types.js").HostedServerRestoreAuthorization>(
        `/api/account/servers/${encodeURIComponent(serverId)}/restore-authorizations`,
        { ...init, method: "POST", body },
      ),
    deleteServer: (
      serverId: string,
      body: import("./types.js").HostedServerDeleteRequest,
      init?: RequestSignal,
    ) =>
      request<{ ok: boolean }>(
        `/api/account/servers/${encodeURIComponent(serverId)}`,
        { ...init, method: "DELETE", body },
      ),
    leaveServer: (serverId: string, init?: RequestSignal) =>
      request<{ ok: boolean }>(
        `/api/account/servers/${encodeURIComponent(serverId)}/membership`,
        { ...init, method: "DELETE" },
      ),
    serverAuditEvents: (serverId: string, limit = 50) =>
      request<ListResponse<AuditEvent>>(
        `/api/account/servers/${encodeURIComponent(serverId)}/audit-events?limit=${limit}`,
      ),
    routes: (serverId: string, init?: RequestSignal) =>
      request<import("./types.js").HostedRouteDocument>(
        `/api/account/servers/${encodeURIComponent(serverId)}/routes`,
        init,
      ),
    reportRouteFailure: (
      serverId: string,
      body: {
        routeType: string;
        endpointGeneration: number;
        category: "transport_failed" | "http_failed" | "invalid_health" | "identity_mismatch" | "remote_access_disabled";
      },
    ) =>
      request<{ ok: boolean; matched: boolean }>(
        `/api/account/servers/${encodeURIComponent(serverId)}/route-failures`,
        { method: "POST", body },
      ),
    porticoSession: (
      serverId: string,
      body: {
        selectionEnvelope: import("./types.js").HostedProfileSelectionEnvelope;
      },
      init?: RequestSignal,
    ) =>
      request<import("./types.js").PorticoSessionBootstrapResponse>(
        `/api/account/servers/${encodeURIComponent(serverId)}/sessions`,
        { ...init, method: "POST", body },
      ),
    authorizeLocalLogin: (
      serverId: string,
      body: {
        callbackUrl: string;
        localOrigin: string;
        state: string;
        serverPublicKeyFingerprint: string;
        publicConsoleOriginGeneration: number;
        profileId: string;
        pin?: string;
        installationId?: string;
      },
      init?: RequestSignal,
    ) =>
      request<import("./types.js").PorticoLocalLoginAuthorizeResponse>(
        `/api/account/servers/${encodeURIComponent(serverId)}/local-login/authorize`,
        { ...init, method: "POST", body },
      ),
    completeClaim: (body: { claimCode: string }, init?: RequestSignal) =>
      request<{
        server: import("./types.js").HostedServer;
        serverCredential: string;
      }>("/api/account/server-claims/by-code/complete", {
        ...init,
        method: "POST",
        body,
      }),
    acceptInvite: (inviteId: string, init?: RequestSignal) =>
      request<import("./types.js").PorticoMembership>(
        `/api/invites/${encodeURIComponent(inviteId)}/accept`,
        { ...init, method: "POST" },
      ),
    members: (
      serverId: string,
      params: {
        limit?: number;
        cursor?: string;
        count?: "none" | "exact";
      } = {},
    ) =>
      request<CursorListResponse<import("./types.js").PorticoMemberProfile>>(
        `/api/account/servers/${encodeURIComponent(serverId)}/members${hostedCursorQuery(params)}`,
      ),
    updateMember: (
      serverId: string,
      memberId: string,
      body: {
        permissionTemplate: import("./types.js").PorticoPermissionTemplate;
      },
    ) =>
      request<import("./types.js").PorticoMembership>(
        `/api/account/servers/${encodeURIComponent(serverId)}/members/${encodeURIComponent(memberId)}`,
        { method: "PATCH", body },
      ),
    revokeMember: (serverId: string, memberId: string) =>
      request<{ ok: boolean }>(
        `/api/account/servers/${encodeURIComponent(serverId)}/members/${encodeURIComponent(memberId)}`,
        { method: "DELETE" },
      ),
    invites: (
      serverId: string,
      params: {
        limit?: number;
        cursor?: string;
        count?: "none" | "exact";
      } = {},
    ) =>
      request<CursorListResponse<PorticoInvite>>(
        `/api/account/servers/${encodeURIComponent(serverId)}/invites${hostedCursorQuery(params)}`,
      ),
    createInvite: (
      serverId: string,
      body: {
        recipient: string;
        email?: string;
        role: string;
        permissionTemplate: PorticoPermissionTemplate;
        deliveryMode: "email" | "link";
      },
    ) =>
      request<PorticoInvite>(
        `/api/account/servers/${encodeURIComponent(serverId)}/invites`,
        { method: "POST", body },
      ),
    resendInvite: (serverId: string, inviteId: string) =>
      request<PorticoInvite>(
        `/api/account/servers/${encodeURIComponent(serverId)}/invites/${encodeURIComponent(inviteId)}/resend`,
        { method: "POST" },
      ),
    revokeInvite: (serverId: string, inviteId: string) =>
      request<{ ok: boolean }>(
        `/api/account/servers/${encodeURIComponent(serverId)}/invites/${encodeURIComponent(inviteId)}`,
        { method: "DELETE" },
      ),
  };
}

export type HostedServicesClient = ReturnType<
  typeof createHostedServicesClient
>;

export function dataTagsForMutation(path: string): string[] {
  const pathname = path.split("?")[0] ?? path;
  if (
    pathname.startsWith("/api/account/profiles") ||
    pathname.startsWith("/api/account/profile-admin-proofs") ||
    pathname.startsWith("/api/account/profile-trusts")
  )
    return ["account", "auth", "profiles"];
  if (pathname.startsWith("/api/preferences"))
    return ["preferences", "home", "playback"];
  if (pathname.startsWith("/api/notifications")) return ["notifications"];
  if (pathname.startsWith("/api/admin/viewer-feedback"))
    return ["feedback", "notifications"];
  if (pathname.startsWith("/api/admin/viewer-notifications"))
    return ["notifications"];
  if (pathname.startsWith("/api/feedback")) return ["feedback"];
  if (pathname.startsWith("/api/display-preferences/"))
    return ["display-preferences"];
  if (pathname === "/api/search/history") return ["search-history"];
  if (pathname === "/api/account/watch-history")
    return [
      "account",
      "auth",
      "media-state",
      "playback-progress",
      "dashboard:history",
      "home",
      "playback",
    ];
  if (pathname.startsWith("/api/account/")) return ["account", "auth"];
  if (
    pathname.startsWith("/api/libraries/trash") ||
    pathname.startsWith("/api/libraries/")
  )
    return ["libraries", "library-items", "home", "jobs"];
  if (pathname === "/api/libraries") return ["libraries", "home"];
  if (pathname === "/api/media/bulk/state")
    return ["media-state", "library-items"];
  if (pathname === "/api/media/bulk/jobs") return ["jobs", "dashboard:jobs"];
  if (pathname === "/api/media/bulk/metadata")
    return ["media", "library-items"];
  if (
    /^\/api\/media\/[^/]+\/(watchlist|favorite|reaction|watched|rating)$/.test(
      pathname,
    )
  )
    return ["media-state", "library-items"];
  if (
    pathname.startsWith("/api/media/") ||
    pathname.startsWith("/api/metadata/")
  )
    return ["media", "library-items"];
  if (pathname.startsWith("/api/download-preparations"))
    return ["downloads", "media"];
  if (
    pathname.startsWith("/api/playback-sessions/") ||
    pathname.startsWith("/api/playback/") ||
    pathname.startsWith("/api/watch-with-friends/")
  )
    return ["playback"];
  if (pathname.startsWith("/api/settings"))
    return ["settings", "dlna", "remote-access"];
  if (pathname.startsWith("/api/devices")) return ["devices", "settings"];
  if (pathname.startsWith("/api/users")) return ["users", "settings"];
  if (pathname.startsWith("/api/auth/api-keys"))
    return ["api-keys", "settings"];
  if (pathname.startsWith("/api/backups")) return ["backups"];
  if (pathname.startsWith("/api/tasks")) return ["tasks", "jobs"];
  if (pathname.startsWith("/api/remote-access"))
    return ["remote-access", "settings"];
  if (pathname.startsWith("/api/live-tv")) return ["live-tv"];
  if (pathname.startsWith("/api/dvr")) return ["dvr"];
  if (
    pathname.startsWith("/api/admin/library-channels") ||
    pathname.startsWith("/api/library-channels")
  )
    return ["library-channels", "library-channel-guide"];
  if (pathname.startsWith("/api/playlists")) return ["playlists"];
  if (pathname.startsWith("/api/collections")) return ["collections"];
  return [];
}

interface CredentialLifecycle {
  current():
    LocalServerSession | undefined | Promise<LocalServerSession | undefined>;
  peek(): LocalServerSession | undefined;
  refreshAfterUnauthorized(
    staleAccessToken: string,
    signal?: AbortSignal,
    onDispatch?: () => void,
  ): Promise<LocalServerSession>;
  refreshExplicit(
    refreshToken: string,
    signal?: AbortSignal,
    onDispatch?: () => void,
  ): Promise<NativeSessionCredentials>;
  accept(
    credentials: NativeSessionCredentials,
    source?: LocalServerSession,
  ): Promise<LocalServerSession>;
  clearIfOwned(refreshToken: string): Promise<void>;
}

type SessionCredentialRotation = Pick<
  NativeSessionCredentials,
  | "tokenType"
  | "accessToken"
  | "accessExpiresAt"
  | "refreshToken"
  | "refreshExpiresAt"
> &
  Partial<
    Pick<
      NativeSessionCredentials,
      | "authority"
      | "accountId"
      | "serverId"
      | "profileId"
      | "authorizationRevision"
      | "serverPublicKey"
      | "serverPublicKeyFingerprint"
    >
  >;

function credentialPersistenceError(
  message: string,
  cause: unknown,
  cleanupFailures: unknown[],
): ApiError {
  if (cleanupFailures.length > 0) {
    return new CredentialCleanupUncertainError(message, cause, cleanupFailures);
  }
  return new ApiError(0, "credential_persistence_failed", message, {
    cause: cause instanceof Error ? cause.message : String(cause),
    cleanupFailureCount: cleanupFailures.length,
  });
}

function createCredentialLifecycle(
  config: PorticoClientOptions,
): CredentialLifecycle {
  let cached = config.sessionStore?.get();
  let adapterLoaded = false;
  let adapterLoad: Promise<LocalServerSession | undefined> | undefined;
  let refreshInFlight: Promise<LocalServerSession> | undefined;
  let pendingRotation: PendingCredentialRotation | undefined;

  const apiKeySession = (): LocalServerSession | undefined => {
    if (config.apiKey === undefined) return undefined;
    const apiKey = String(resolveValue(config.apiKey, "")).trim();
    if (!/^ptc_api_[A-Za-z0-9_-]{43}$/u.test(apiKey))
      throw new TypeError("PorticoClientOptions.apiKey must be a valid ptc_api_ Server API key.");
    return {
      apiBaseUrl: trimTrailingSlash(resolveValue(config.apiBaseUrl, "")),
      accessToken: apiKey,
      authority: "local",
    };
  };
  const peek = () => apiKeySession() ?? config.sessionStore?.get() ?? cached;
  const current = ():
    | LocalServerSession
    | undefined
    | Promise<LocalServerSession | undefined> => {
    const keySession = apiKeySession();
    if (keySession) return keySession;
    const stored = config.sessionStore?.get();
    if (stored) {
      cached = stored;
      return stored;
    }
    if (cached) return cached;
    if (!config.credentialAdapter?.load || adapterLoaded) return undefined;
    adapterLoad ??= config.credentialAdapter
      .load()
      .then(async (loaded) => {
        adapterLoaded = true;
        if (!loaded) {
          cached = undefined;
          return undefined;
        }
        try {
          config.sessionStore?.set?.(loaded);
          cached = loaded;
          return loaded;
        } catch (error) {
          cached = undefined;
          const cleanupFailures = await clearCredentialCopies();
          throw credentialPersistenceError(
            "Restored credentials could not be published.",
            error,
            cleanupFailures,
          );
        }
      })
      .finally(() => {
        adapterLoad = undefined;
      });
    return adapterLoad;
  };

  const clearCredentialCopies = async (): Promise<unknown[]> => {
    cached = undefined;
    adapterLoaded = true;
    const operations: (() => void | Promise<void>)[] = [];
    if (config.sessionStore?.clear)
      operations.push(() => config.sessionStore!.clear!());
    if (config.credentialAdapter?.clear)
      operations.push(() => config.credentialAdapter!.clear());
    if (config.credentialAdapter?.clearPendingRotation)
      operations.push(() => config.credentialAdapter!.clearPendingRotation!());
    const results = await Promise.allSettled(
      operations.map((operation) => Promise.resolve().then(operation)),
    );
    return results.flatMap((result) =>
      result.status === "rejected" ? [result.reason] : [],
    );
  };

  const prepareRotation = async (
    refreshToken: string,
  ): Promise<PendingCredentialRotation> => {
    let pending = pendingRotation;
    if (!pending && config.credentialAdapter?.loadPendingRotation) {
      pending = await config.credentialAdapter.loadPendingRotation();
    }
    if (
      pending?.version === "v1" &&
      pending.oldRefreshToken === refreshToken &&
      validRotationKey(pending.rotationKey)
    ) {
      pendingRotation = pending;
      return pending;
    }
    if (pending && config.credentialAdapter?.clearPendingRotation)
      await config.credentialAdapter.clearPendingRotation();
    const created: PendingCredentialRotation = {
      version: "v1",
      oldRefreshToken: refreshToken,
      rotationKey: createSecureRotationKey(),
      createdAt: new Date().toISOString(),
    };
    await config.credentialAdapter?.savePendingRotation?.(created);
    pendingRotation = created;
    return created;
  };

  const completeRotation = async () => {
    pendingRotation = undefined;
    await config.credentialAdapter?.clearPendingRotation?.();
  };

  const clear = async () => {
    const failures = await clearCredentialCopies();
    if (failures.length)
      throw new AggregateError(
        failures,
        "Portico could not clear every credential copy.",
      );
  };

  const persist = async (session: LocalServerSession) => {
    try {
      await config.credentialAdapter?.save(session);
      config.sessionStore?.set?.(session);
      cached = session;
      adapterLoaded = true;
    } catch (error) {
      const cleanupFailures = await clearCredentialCopies();
      throw credentialPersistenceError(
        "Refreshed credentials could not be persisted.",
        error,
        cleanupFailures,
      );
    }
  };

  const refreshRequest = async (
    refreshToken: string,
    rotationKey: string,
    source?: LocalServerSession,
    signal?: AbortSignal,
    onDispatch?: () => void,
  ): Promise<SessionCredentialRotation> => {
    const base = trimTrailingSlash(
      source?.apiBaseUrl || resolveValue(config.apiBaseUrl, ""),
    );
    const path = "/api/auth/sessions/refresh";
    onDispatch?.();
    const response = await fetcher(config.transport)(`${base}${path}`, {
      method: "POST",
      credentials: "include",
      signal,
      headers: withRequestId(
        {
          "Content-Type": "application/json",
          "X-Portico-CSRF": resolveValue(config.csrfToken, "1"),
        },
        config.requestId,
      ),
      body: JSON.stringify({ refreshToken, rotationKey }),
    });
    try {
      return await handleResponse<SessionCredentialRotation>(
        response,
        "POST",
        path,
        undefined,
        signal,
      );
    } catch (error) {
      // A syntactically successful refresh with an unreadable body is a
      // protocol failure, not proof that the refresh family is invalid. Keep
      // the durable rotation receipt so the exact token/key pair can safely be
      // retried after a lost or malformed response.
      const normalizedError = response.ok
        ? new ApiError(
            401,
            "invalid_refresh_response",
            "The server returned an invalid refresh response.",
          )
        : error;
      if (isTerminalRefreshFailure(normalizedError)) await clear();
      throw normalizedError;
    }
  };

  const mergeCredentials = (
    credentials: SessionCredentialRotation,
    source: LocalServerSession | undefined,
  ): LocalServerSession => {
    if (!credentials.accessToken || !credentials.refreshToken) {
      throw new ApiError(
        401,
        "invalid_refresh_response",
        "The server returned incomplete refreshed credentials.",
      );
    }
    const serverPublicKey = credentials.serverPublicKey?.trim();
    const serverPublicKeyFingerprint = credentials.serverPublicKeyFingerprint?.trim();
    if (!serverPublicKey || !serverPublicKeyFingerprint) {
      throw new ApiError(
        401,
        "invalid_refresh_response",
        "The server returned credentials without its durable identity key.",
      );
    }
    if ((source?.serverPublicKey && source.serverPublicKey !== serverPublicKey) ||
        (source?.serverPublicKeyFingerprint && source.serverPublicKeyFingerprint !== serverPublicKeyFingerprint)) {
      throw new ApiError(
        401,
        "server_identity_changed",
        "The server identity changed while credentials were being refreshed.",
      );
    }
    return {
      ...source,
      apiBaseUrl:
        source?.apiBaseUrl ||
        trimTrailingSlash(resolveValue(config.apiBaseUrl, "")),
      accessToken: credentials.accessToken,
      bootstrapAccessToken: undefined,
      refreshToken: credentials.refreshToken,
      expiresAt: credentials.accessExpiresAt,
      refreshExpiresAt: credentials.refreshExpiresAt,
      serverPublicKey,
      serverPublicKeyFingerprint,
      ...(credentials.authority ? { authority: credentials.authority } : {}),
      ...(credentials.accountId ? { accountId: credentials.accountId } : {}),
      ...(credentials.serverId ? { serverId: credentials.serverId } : {}),
      ...(credentials.profileId ? { profileId: credentials.profileId } : {}),
      ...(credentials.authorizationRevision
        ? { authorizationRevision: credentials.authorizationRevision }
        : {}),
    };
  };

  const rotate = async (
    refreshToken: string,
    source?: LocalServerSession,
    signal?: AbortSignal,
    onDispatch?: () => void,
  ) => {
    let credentials: SessionCredentialRotation;
    try {
      const pending = await prepareRotation(refreshToken);
      credentials = await refreshRequest(
        refreshToken,
        pending.rotationKey,
        source,
        signal,
        onDispatch,
      );
      const next = mergeCredentials(credentials, source);
      try {
        await config.credentialAdapter?.save(next);
        config.sessionStore?.set?.(next);
        cached = next;
        adapterLoaded = true;
      } catch (error) {
        throw new ApiError(
          0,
          "credential_persistence_failed",
          "Refreshed credentials could not be persisted. Portico can safely retry the committed rotation.",
          {
            cause: error instanceof Error ? error.message : String(error),
          },
        );
      }
      await completeRotation().catch(() => undefined);
      return { credentials, session: next };
    } catch (error) {
      throw error;
    }
  };

  return {
    current,
    peek,
    async refreshAfterUnauthorized(
      staleAccessToken: string,
      signal?: AbortSignal,
      onDispatch?: () => void,
    ) {
      const latest = await current();
      const latestAccessToken = currentAccessTokenFromSession(latest);
      if (latestAccessToken && latestAccessToken !== staleAccessToken)
        return latest as LocalServerSession;
      const refreshToken = currentRefreshToken(latest);
      if (!refreshToken)
        throw new ApiError(
          401,
          "session_expired",
          "The session has expired and cannot be refreshed.",
        );
      // A refresh is shared by every request using this credential generation.
      // Individual callers may stop waiting, but their AbortSignal must never
      // cancel the credential rotation for the remaining callers.
      refreshInFlight ??= rotate(refreshToken, latest, undefined, onDispatch)
        .then(({ session }) => session)
        .finally(() => {
          refreshInFlight = undefined;
        });
      return refreshInFlight;
    },
    async refreshExplicit(
      refreshToken: string,
      signal?: AbortSignal,
      onDispatch?: () => void,
    ) {
      const source = await current();
      const { credentials } = await rotate(
        refreshToken,
        source,
        signal,
        onDispatch,
      );
      return credentials as NativeSessionCredentials;
    },
    async accept(
      credentials: NativeSessionCredentials,
      source?: LocalServerSession,
    ) {
      const active = source ?? (await current());
      const next = mergeCredentials(credentials, active);
      await persist(next);
      return next;
    },
    async clearIfOwned(refreshToken: string) {
      const session = await current();
      if (currentRefreshToken(session) === refreshToken) await clear();
    },
  };
}

function validateAPIKeyClientOptions(config: PorticoClientOptions): void {
  if (config.apiKey === undefined) return;
  if (config.sessionStore || config.credentialAdapter)
    throw new TypeError("PorticoClientOptions.apiKey cannot be combined with refreshable session storage.");
  const value = String(resolveValue(config.apiKey, "")).trim();
  if (!/^ptc_api_[A-Za-z0-9_-]{43}$/u.test(value))
    throw new TypeError("PorticoClientOptions.apiKey must be a valid ptc_api_ Server API key.");
}

function currentAccessTokenFromSession(session?: LocalServerSession): string {
  return session?.bootstrapAccessToken || session?.accessToken || "";
}

function currentRefreshToken(session?: LocalServerSession): string {
  return session?.refreshToken || "";
}

function isTerminalRefreshFailure(error: unknown): boolean {
  return (
    isTerminalServerAuthorizationFailure(error) ||
    (error instanceof ApiError &&
      ["invalid_refresh_token", "refresh_token_reuse"].includes(error.code))
  );
}

/**
 * Only an explicit server-scoped revocation may erase a remembered server
 * credential. A timeout, offline server, Hosted outage, generic 401, expired
 * access token, or 5xx response is not proof that the account lost access.
 */
export function isTerminalServerAuthorizationFailure(error: unknown): boolean {
  if (!(error instanceof ApiError)) return false;
  return [
    "credential_revoked",
    "refresh_reused",
    "account_deleted",
    "profile_deleted",
    "membership_removed",
    "membership_inactive",
    "server_not_found",
    "server_session_revoked",
    "account_disabled",
    "device_not_allowed",
  ].includes(error.code);
}

function canAutomaticallyRefresh(
  path: string,
  session?: LocalServerSession,
): boolean {
  if (!currentAccessTokenFromSession(session) || !currentRefreshToken(session))
    return false;
  return (
    path !== "/api/auth/sessions/refresh" &&
    path !== "/api/auth/sessions/revoke"
  );
}

type RequestAuthorization = NonNullable<ApiRequestInit["authorization"]>;

const RESERVED_CALLER_HEADERS = new Set([
  "authorization",
  "cookie",
  "set-cookie",
  "host",
  "content-length",
  "x-portico-csrf",
  "x-portico-profile-admin-proof",
  "x-portico-request-signature",
  "digest",
]);

function canonicalAPIPath(path: string): string {
  if (
    typeof path !== "string" ||
    !path.startsWith("/api/") ||
    path.startsWith("//") ||
    path.includes("\\")
  ) {
    throw new TypeError(
      "Authenticated Portico requests require a canonical relative /api/ path.",
    );
  }
  let decoded: string;
  try {
    decoded = decodeURIComponent(path.split("?", 1)[0]);
  } catch {
    throw new TypeError(
      "Authenticated Portico request path has invalid encoding.",
    );
  }
  if (
    decoded.includes("\\") ||
    decoded.startsWith("//") ||
    decoded.startsWith("/api//") ||
    decoded.includes("://")
  ) {
    throw new TypeError("Authenticated Portico request path is not canonical.");
  }
  return path;
}

function callerHeaders(
  headers: HeadersInit | undefined,
): Record<string, string> {
  const normalized = normalizeHeaders(headers);
  for (const name of Object.keys(normalized)) {
    if (RESERVED_CALLER_HEADERS.has(name.toLowerCase())) {
      throw new TypeError(
        `Header ${name} is controlled by Portico Client Core.`,
      );
    }
  }
  return normalized;
}

function boundedAuthorizationToken(token: string, label: string): string {
  const normalized = String(token ?? "").trim();
  if (
    !normalized ||
    normalized.length > 4096 ||
    /[\r\n\u0000]/.test(normalized)
  ) {
    throw new TypeError(`${label} token is invalid.`);
  }
  return normalized;
}

function requestAuthorizationHeaders(
  mode: RequestAuthorization,
  session?: LocalServerSession,
): Record<string, string> {
  switch (mode.mode) {
    case "viewer":
      return authHeader(session);
    case "playback-continuation":
      return {
        Authorization: `PorticoPlayback ${boundedAuthorizationToken(mode.token, "Playback continuation")}`,
      };
    case "cast-receiver":
      return {
        Authorization: `PorticoReceiver ${boundedAuthorizationToken(mode.token, "Cast receiver")}`,
      };
    case "download-grant":
      return {
        Authorization: `PorticoDownload ${boundedAuthorizationToken(mode.token, "Download grant")}`,
      };
    case "profile-admin-proof":
      return {
        ...authHeader(session),
        "X-Portico-Profile-Admin-Proof": boundedAuthorizationToken(
          mode.proof,
          "Profile administration proof",
        ),
      };
    case "anonymous":
      return {};
  }
}

function requestPrincipalIdentity(
  session: LocalServerSession | undefined,
  origin: string,
): string {
  return JSON.stringify([
    trimTrailingSlash(origin),
    session?.serverId ?? "",
    session?.serverPublicKeyFingerprint ?? "",
    session?.authority ?? "local",
    session?.accountId ?? "",
    session?.profileId ?? "",
    session?.installationId ?? "",
  ]);
}

async function localRequest<T>(
  path: string,
  options: ApiRequestInit,
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  inFlightJSONRequests: Map<string, InFlightJSONRequest>,
  allowRefresh = true,
): Promise<T> {
  path = canonicalAPIPath(path);
  const {
    body,
    authorization = { mode: "viewer" },
    timeoutMs,
    deadlineAt,
    retryBudget,
    operationClass,
    credentials: _callerCredentials,
    ...rest
  } = options as ApiRequestInit & { credentials?: RequestCredentials };
  const method = String(options.method ?? "GET").toUpperCase();
  const transportOptions: TransportRequestOptions = {
    signal: rest.signal,
    timeoutMs,
    deadlineAt,
    retryBudget,
    operationClass,
    method,
  };
  const requestDeadline = transportDeadlineAt(config, transportOptions);
  const sessionResult = credentials.current();
  const session =
    sessionResult instanceof Promise
      ? await runTransportOperation(
          config,
          transportOptions,
          method,
          () => sessionResult,
          undefined,
          requestDeadline,
        )
      : sessionResult;
  const url =
    authorization.mode === "playback-continuation" ||
    authorization.mode === "cast-receiver"
      ? trustedAPIURL(authorization.origin, path, config.baseHref)
      : buildApiUrl(path, config, session);
  const principalIdentity = requestPrincipalIdentity(
    session,
    urlOrigin(url, config.baseHref),
  );
  const requestHeaders = {
    ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
    ...requestAuthorizationHeaders(authorization, session),
    ...(method === "GET" ||
    method === "HEAD" ||
    authorization.mode === "cast-receiver"
      ? {}
      : { "X-Portico-CSRF": resolveValue(config.csrfToken, "1") }),
    ...callerHeaders(options.headers),
  };
  const headers = withRequestId(requestHeaders, config.requestId);
  const init: RequestInit = {
    ...rest,
    method,
    headers,
    credentials:
      authorization.mode === "anonymous" || authorization.mode === "cast-receiver"
        ? "omit"
        : "include",
  };
  if (body !== undefined) init.body = JSON.stringify(body);
  const coalesceKey = jsonRequestCoalesceKey(
    method,
    url,
    requestHeaders,
    body,
    transportBudgetKey(config, transportOptions),
  );
  const execute = async (context: TransportRequestContext) => {
    let response = await transportFetch(config.transport, url, init, context);
    const viewerRefreshable =
      authorization.mode === "viewer" ||
      authorization.mode === "profile-admin-proof";
    if (
      response.status === 401 &&
      allowRefresh &&
      viewerRefreshable &&
      canAutomaticallyRefresh(path, session)
    ) {
      const beforeRefresh = await awaitWithAbort(
        Promise.resolve(credentials.current()),
        context.signal,
      );
      const beforeRefreshURL = buildApiUrl(path, config, beforeRefresh);
      if (
        beforeRefreshURL !== url ||
        requestPrincipalIdentity(
          beforeRefresh,
          urlOrigin(beforeRefreshURL, config.baseHref),
        ) !== principalIdentity
      ) {
        throw new ApiError(
          409,
          "request_superseded",
          "The active Portico server or viewer changed while this request was in progress.",
        );
      }
      await awaitWithAbort(
        credentials.refreshAfterUnauthorized(
          currentAccessTokenFromSession(session),
          context.signal,
          () => {
            context.dispatched = true;
          },
        ),
        context.signal,
      );
      const refreshed = await awaitWithAbort(
        Promise.resolve(credentials.current()),
        context.signal,
      );
      const refreshedURL = buildApiUrl(path, config, refreshed);
      if (
        refreshedURL !== url ||
        requestPrincipalIdentity(
          refreshed,
          urlOrigin(refreshedURL, config.baseHref),
        ) !== principalIdentity
      ) {
        throw new ApiError(
          409,
          "request_superseded",
          "The active Portico server or viewer changed while this request was in progress.",
        );
      }
      response = await transportFetch(
        config.transport,
        url,
        {
          ...init,
          headers: {
            ...normalizeHeaders(init.headers),
            ...requestAuthorizationHeaders(authorization, refreshed),
          },
        },
        context,
      );
    }
    return handleResponse<T>(
      response,
      method,
      path,
      config.onMutation,
      context.signal,
    );
  };
  if (coalesceKey) {
    return coalescedJSONRequest<T>(
      inFlightJSONRequests,
      coalesceKey,
      rest.signal,
      (signal) =>
        runTransportOperation(
          config,
          { ...transportOptions, signal: undefined },
          method,
          execute,
          signal,
          requestDeadline,
        ),
    );
  }
  return runTransportOperation(
    config,
    transportOptions,
    method,
    execute,
    undefined,
    requestDeadline,
  );
}

async function hostedRequest<T>(
  path: string,
  options: ApiRequestInit,
  config: HostedServicesClientOptions,
  inFlightJSONRequests: Map<string, InFlightJSONRequest>,
): Promise<T> {
  path = canonicalAPIPath(path);
  const {
    body,
    authorization: _authorization,
    timeoutMs,
    deadlineAt,
    retryBudget,
    operationClass,
    credentials: _callerCredentials,
    ...rest
  } = options as ApiRequestInit & { credentials?: RequestCredentials };
  const method = String(options.method ?? "GET").toUpperCase();
  const transportOptions: TransportRequestOptions = {
    signal: rest.signal,
    timeoutMs,
    deadlineAt,
    retryBudget,
    operationClass,
    method,
  };
  const requestDeadline = transportDeadlineAt(config, transportOptions);
  const csrf =
    method === "GET" || method === "HEAD"
      ? ""
      : String(resolveValue(config.csrfToken, "")).trim();
  const bearer = String(resolveValue(config.accessToken, "")).trim();
  const requestHeaders = {
    ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
    ...(csrf ? { "X-Portico-CSRF": csrf } : {}),
    ...(bearer ? { Authorization: `Bearer ${bearer}` } : {}),
    ...callerHeaders(options.headers),
  };
  const init: RequestInit = {
    ...rest,
    method,
    headers: withRequestId(requestHeaders, config.requestId),
    credentials: "include",
  };
  if (body !== undefined) init.body = JSON.stringify(body);
  const url = trustedAPIURL(
    resolveBase(config.hostedApiBaseUrl, "https://api.getportico.tv"),
    path,
    config.baseHref,
  );
  const coalesceKey = jsonRequestCoalesceKey(
    method,
    url,
    requestHeaders,
    body,
    transportBudgetKey(config, transportOptions),
  );
  const execute = async (context: TransportRequestContext) => {
    let response = await hostedFetchWithRetry(
      config,
      url,
      init,
      method,
      context,
      retryBudget,
      operationClass,
    );
    captureHostedCSRFToken(response, config);
    if (
      method !== "GET" &&
      method !== "HEAD" &&
      (await responseHasProblemCode(response, "csrf_failed", context.signal))
    ) {
      await refreshHostedRequestVerification(config, context.signal, () => {
        context.dispatched = true;
      });
      const refreshedCSRF = String(resolveValue(config.csrfToken, "")).trim();
      response = await transportFetch(
        config.transport,
        url,
        {
          ...init,
          headers: {
            ...normalizeHeaders(init.headers),
            ...(refreshedCSRF ? { "X-Portico-CSRF": refreshedCSRF } : {}),
          },
        },
        context,
      );
      captureHostedCSRFToken(response, config);
    }
    return handleResponse<T>(
      response,
      method,
      path,
      config.onMutation,
      context.signal,
      "hosted",
    );
  };
  if (coalesceKey) {
    return coalescedJSONRequest<T>(
      inFlightJSONRequests,
      coalesceKey,
      rest.signal,
      (signal) =>
        runTransportOperation(
          config,
          { ...transportOptions, signal: undefined },
          method,
          execute,
          signal,
          requestDeadline,
        ),
    );
  }
  return runTransportOperation(
    config,
    transportOptions,
    method,
    execute,
    undefined,
    requestDeadline,
  );
}

const hostedTransientStatuses = new Set([408, 425, 429, 500, 502, 503, 504]);

async function hostedFetchWithRetry(
  config: HostedServicesClientOptions,
  url: string,
  init: RequestInit,
  method: string,
  context: TransportRequestContext,
  requestedRetryBudget?: number,
  requestedOperationClass?: OperationClass,
): Promise<Response> {
  const policy = getOperationPolicy(
    operationClassFor(method, requestedOperationClass),
  );
  const retryableMethod =
    isSafeTransportMethod(method) &&
    policy.retry.eligible &&
    policy.retry.mode === "request";
  const requestedBudget = boundedRetryBudget(
    requestedRetryBudget ?? config.retryBudget,
    policy,
  );
  const delays = retryDelays(config, requestedBudget, policy);
  const budget = Math.min(requestedBudget, delays.length);
  for (let attempt = 0; ; attempt += 1) {
    let retryDelay: number | undefined;
    try {
      const response = await transportFetch(
        config.transport,
        url,
        init,
        context,
      );
      if (
        !retryableMethod ||
        attempt >= budget ||
        !hostedTransientStatuses.has(response.status)
      )
        return response;
      const configuredDelay = delays[attempt];
      if (configuredDelay === undefined)
        throw new Error("Hosted read retry budget has no delay slot.");
      const retryAfterMs = parseRetryAfter(
        response.headers.get("Retry-After"),
      ).retryAfterMs;
      const retryCohort = String(resolveValue(config.retryCohort, "")).trim();
      const spread = retryCohort
        ? positiveFullJitterDelay(configuredDelay, retryCohort, attempt)
        : configuredDelay;
      // The local backoff is a jitter budget, not an additional mandatory
      // wait. A provider Retry-After is different: it is an exact lower bound,
      // so spread the fleet only after that deadline.
      retryDelay = retryAfterMs === undefined ? spread : retryAfterMs + spread;
      if (
        context.deadlineAt !== undefined &&
        Date.now() + retryDelay >= context.deadlineAt
      )
        return response;
      await Promise.resolve(response.body?.cancel?.()).catch(() => undefined);
    } catch (error) {
      if (
        !retryableMethod ||
        attempt >= budget ||
        isAbortFailure(error) ||
        context.signal.aborted
      )
        throw error;
      const retryFloor = delays[attempt];
      const retryCohort = String(resolveValue(config.retryCohort, "")).trim();
      retryDelay = retryCohort
        ? positiveFullJitterDelay(retryFloor, retryCohort, attempt)
        : retryFloor;
    }
    if (retryDelay === undefined)
      throw new Error("Hosted read retry budget has no delay slot.");
    await abortableRetryDelay(retryDelay, context.signal);
  }
}

function isAbortFailure(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError";
}

function abortableRetryDelay(
  delayMs: number,
  signal?: AbortSignal | null,
): Promise<void> {
  if (signal?.aborted) return Promise.reject(abortError());
  return new Promise((resolve, reject) => {
    const timeout = setTimeout(() => {
      signal?.removeEventListener("abort", abort);
      resolve();
    }, delayMs);
    const abort = () => {
      clearTimeout(timeout);
      signal?.removeEventListener("abort", abort);
      reject(abortError());
    };
    signal?.addEventListener("abort", abort, { once: true });
  });
}

async function localFormRequest<T>(
  path: string,
  form: FormData,
  method: string,
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  options: RequestSignal = {},
): Promise<T> {
  path = canonicalAPIPath(path);
  const normalizedMethod = method.toUpperCase();
  const transportOptions: TransportRequestOptions = {
    ...options,
    operationClass: options.operationClass ?? "form/upload",
    method: normalizedMethod,
  };
  const requestDeadline = transportDeadlineAt(config, transportOptions);
  const sessionResult = credentials.current();
  const session =
    sessionResult instanceof Promise
      ? await runTransportOperation(
          config,
          transportOptions,
          normalizedMethod,
          () => sessionResult,
          undefined,
          requestDeadline,
        )
      : sessionResult;
  const url = buildApiUrl(path, config, session);
  const principalIdentity = requestPrincipalIdentity(
    session,
    urlOrigin(url, config.baseHref),
  );
  const init: RequestInit = {
    method: normalizedMethod,
    credentials: "include",
    headers: withRequestId(
      {
        ...authHeader(session),
        ...(normalizedMethod === "GET" || normalizedMethod === "HEAD"
          ? {}
          : { "X-Portico-CSRF": resolveValue(config.csrfToken, "1") }),
      },
      config.requestId,
    ),
    body: form,
  };
  const execute = async (context: TransportRequestContext) => {
    let response = await transportFetch(config.transport, url, init, context);
    if (response.status === 401 && canAutomaticallyRefresh(path, session)) {
      const beforeRefresh = await awaitWithAbort(
        Promise.resolve(credentials.current()),
        context.signal,
      );
      const beforeRefreshURL = buildApiUrl(path, config, beforeRefresh);
      if (
        beforeRefreshURL !== url ||
        requestPrincipalIdentity(
          beforeRefresh,
          urlOrigin(beforeRefreshURL, config.baseHref),
        ) !== principalIdentity
      ) {
        throw new ApiError(
          409,
          "request_superseded",
          "The active Portico server or viewer changed while this request was in progress.",
        );
      }
      await awaitWithAbort(
        credentials.refreshAfterUnauthorized(
          currentAccessTokenFromSession(session),
          context.signal,
          () => {
            context.dispatched = true;
          },
        ),
        context.signal,
      );
      const refreshed = await awaitWithAbort(
        Promise.resolve(credentials.current()),
        context.signal,
      );
      const refreshedURL = buildApiUrl(path, config, refreshed);
      if (
        refreshedURL !== url ||
        requestPrincipalIdentity(
          refreshed,
          urlOrigin(refreshedURL, config.baseHref),
        ) !== principalIdentity
      ) {
        throw new ApiError(
          409,
          "request_superseded",
          "The active Portico server or viewer changed while this request was in progress.",
        );
      }
      response = await transportFetch(
        config.transport,
        url,
        {
          ...init,
          headers: {
            ...normalizeHeaders(init.headers),
            ...authHeader(refreshed),
          },
        },
        context,
      );
    }
    return handleResponse<T>(
      response,
      normalizedMethod,
      path,
      config.onMutation,
      context.signal,
    );
  };
  return runTransportOperation(
    config,
    transportOptions,
    normalizedMethod,
    execute,
    undefined,
    requestDeadline,
  );
}

async function hostedFormRequest<T>(
  path: string,
  form: FormData,
  method: string,
  config: HostedServicesClientOptions,
  options: RequestSignal = {},
): Promise<T> {
  path = canonicalAPIPath(path);
  const normalizedMethod = method.toUpperCase();
  const transportOptions: TransportRequestOptions = {
    ...options,
    operationClass: options.operationClass ?? "form/upload",
    method: normalizedMethod,
  };
  const requestDeadline = transportDeadlineAt(config, transportOptions);
  const csrf =
    normalizedMethod === "GET" || normalizedMethod === "HEAD"
      ? ""
      : String(resolveValue(config.csrfToken, "")).trim();
  const bearer = String(resolveValue(config.accessToken, "")).trim();
  const url = trustedAPIURL(
    resolveBase(config.hostedApiBaseUrl, "https://api.getportico.tv"),
    path,
    config.baseHref,
  );
  const init: RequestInit = {
    method: normalizedMethod,
    credentials: "include",
    headers: withRequestId(
      {
        ...(csrf ? { "X-Portico-CSRF": csrf } : {}),
        ...(bearer ? { Authorization: `Bearer ${bearer}` } : {}),
        ...(normalizedMethod !== "GET" && normalizedMethod !== "HEAD"
          ? { "Idempotency-Key": createRequestId(config.requestId) }
          : {}),
      },
      config.requestId,
    ),
    body: form,
  };
  const execute = async (context: TransportRequestContext) => {
    let response = await transportFetch(config.transport, url, init, context);
    captureHostedCSRFToken(response, config);
    if (
      normalizedMethod !== "GET" &&
      normalizedMethod !== "HEAD" &&
      (await responseHasProblemCode(response, "csrf_failed", context.signal))
    ) {
      await refreshHostedRequestVerification(config, context.signal, () => {
        context.dispatched = true;
      });
      const refreshedCSRF = String(resolveValue(config.csrfToken, "")).trim();
      response = await transportFetch(
        config.transport,
        url,
        {
          ...init,
          headers: {
            ...normalizeHeaders(init.headers),
            ...(refreshedCSRF ? { "X-Portico-CSRF": refreshedCSRF } : {}),
          },
        },
        context,
      );
      captureHostedCSRFToken(response, config);
    }
    return handleResponse<T>(
      response,
      normalizedMethod,
      path,
      config.onMutation,
      context.signal,
      "hosted",
    );
  };
  return runTransportOperation(
    config,
    transportOptions,
    normalizedMethod,
    execute,
    undefined,
    requestDeadline,
  );
}

async function responseHasProblemCode(
  response: Response,
  code: string,
  signal?: AbortSignal,
): Promise<boolean> {
  if (response.ok) return false;
  try {
    const body = JSON.parse(
      await boundedResponseText(response.clone(), 256 * 1024, signal),
    ) as { code?: unknown };
    return body?.code === code;
  } catch {
    return false;
  }
}

async function refreshHostedRequestVerification(
  config: HostedServicesClientOptions,
  signal?: AbortSignal,
  onDispatch?: () => void,
): Promise<void> {
  const bearer = String(resolveValue(config.accessToken, "")).trim();
  onDispatch?.();
  const response = await fetcher(config.transport)(
    trustedAPIURL(
      resolveBase(config.hostedApiBaseUrl, "https://api.getportico.tv"),
      "/api/auth/me",
      config.baseHref,
    ),
    {
      method: "GET",
      credentials: "include",
      signal,
      headers: withRequestId(
        {
          ...(bearer ? { Authorization: `Bearer ${bearer}` } : {}),
        },
        config.requestId,
      ),
    },
  );
  if (!response.ok)
    await throwApiError(
      response,
      "csrf_refresh_failed",
      "Portico could not refresh request verification.",
      signal,
    );
  captureHostedCSRFToken(response, config);
}

function captureHostedCSRFToken(
  response: Response,
  config: HostedServicesClientOptions,
): void {
  const token = response.headers?.get?.("X-Portico-CSRF")?.trim() ?? "";
  if (token) config.onCSRFToken?.(token);
}

async function handleResponse<T>(
  response: Response,
  method: string,
  path: string,
  onMutation?: (tags: DataTag[], path: string) => void,
  signal?: AbortSignal,
  api: "server" | "hosted" = "server",
): Promise<T> {
  if (!response.ok) await throwApiError(response, undefined, undefined, signal);
  if (response.status === 204) return undefined as T;
  const text = await boundedResponseText(response, 4 * 1024 * 1024, signal);
  if (!text) return undefined as T;
  try {
    const parsed = decodeHighRiskResponse(path, method, JSON.parse(text), api) as T;
    if (method !== "GET" && method !== "HEAD" && onMutation) {
      try {
        onMutation(dataTagsForMutation(path), path);
      } catch {
        // The server mutation is already committed. Cache/diagnostic observer
        // failures must never turn transport success into a retryable failure.
      }
    }
    return parsed;
  } catch {
    const responseHeaders = responseHeaderRecord(response.headers);
    const requestId =
      response.headers?.get?.("X-Request-ID")?.trim() || undefined;
    throw new ApiError(
      response.status,
      "invalid_response",
      "Portico received an invalid response from the service.",
      {
        method,
        path,
      },
      {
        messageId: "problem.request-failed",
        requestId,
        responseHeaders,
      },
    );
  }
}

async function throwApiError(
  response: Response,
  fallbackCode = "request_failed",
  fallbackDetail = "Request failed.",
  signal?: AbortSignal,
): Promise<never> {
  let payload: Record<string, unknown> | undefined;
  try {
    const parsed = JSON.parse(
      await boundedResponseText(response, 256 * 1024, signal),
    ) as unknown;
    if (isRecord(parsed)) payload = parsed;
  } catch {
    // A non-JSON failure still carries stable HTTP and retry metadata.
  }

  const responseHeaders = responseHeaderRecord(response.headers);
  const retry = parseRetryAfter(
    response.headers?.get?.("Retry-After"),
    Date.now(),
  );
  const type = stringField(payload, "type");
  const title = stringField(payload, "title");
  const code = stringField(payload, "code") || fallbackCode;
  const messageId = stringField(payload, "messageId") || undefined;
  const detail = userSafeDetail(
    stringField(payload, "detail") ||
      stringField(payload, "message") ||
      title ||
      response.statusText ||
      fallbackDetail,
    fallbackDetail,
  );
  const requestId =
    stringField(payload, "requestId") ||
    response.headers?.get?.("X-Request-ID")?.trim() ||
    undefined;
  const explicitDetails = payload?.details;
  const details = isRecord(explicitDetails)
    ? explicitDetails
    : problemExtensionDetails(payload);

  throw new ApiError(response.status, code, detail, details, {
    type: type || undefined,
    title: title || undefined,
    detail,
    messageId,
    requestId,
    responseHeaders,
    ambiguous:
      responseHeaders["x-portico-terminal-outcome"] === "outcome_unknown",
    ...retry,
  });
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function stringField(
  value: Record<string, unknown> | undefined,
  key: string,
): string {
  const field = value?.[key];
  return typeof field === "string" ? field.trim() : "";
}

function problemExtensionDetails(
  payload: Record<string, unknown> | undefined,
): Record<string, unknown> | undefined {
  if (!payload) return undefined;
  const reserved = new Set([
    "type",
    "title",
    "status",
    "code",
    "detail",
    "message",
    "messageId",
    "details",
    "requestId",
  ]);
  const extensions = Object.fromEntries(
    Object.entries(payload).filter(([key]) => !reserved.has(key)),
  );
  return Object.keys(extensions).length ? extensions : undefined;
}

function userSafeDetail(value: string, fallback: string): string {
  const normalized = value
    .replace(/[\u0000-\u0008\u000B\u000C\u000E-\u001F\u007F]/g, "")
    .trim();
  return (normalized || fallback).slice(0, 2_000);
}

function responseHeaderRecord(
  headers: Headers | undefined,
): Readonly<Record<string, string>> {
  const result: Record<string, string> = {};
  const allowed = new Set([
    "content-type",
    "retry-after",
    "x-request-id",
    "x-portico-terminal-outcome",
    "x-portico-terminal-receipt",
    "x-ratelimit-limit",
    "x-ratelimit-remaining",
    "x-ratelimit-reset",
  ]);
  let total = 0;
  headers?.forEach?.((value, key) => {
    const normalizedKey = key.toLowerCase();
    if (
      !allowed.has(normalizedKey) ||
      normalizedKey.length > 64 ||
      total >= 4096
    )
      return;
    const normalizedValue = String(value).slice(
      0,
      Math.min(1024, 4096 - total),
    );
    total += normalizedKey.length + normalizedValue.length;
    result[normalizedKey] = normalizedValue;
  });
  return Object.freeze(result);
}

async function boundedResponseText(
  response: Response,
  maximumBytes: number,
  signal?: AbortSignal,
): Promise<string> {
  const contentLengthHeader = response.headers?.get?.("Content-Length")?.trim();
  const contentLength =
    contentLengthHeader && /^\d+$/.test(contentLengthHeader)
      ? Number(contentLengthHeader)
      : Number.NaN;
  if (Number.isFinite(contentLength) && contentLength > maximumBytes) {
    response.body?.cancel?.().catch?.(() => undefined);
    throw new ApiError(
      response.status || 0,
      "response_too_large",
      "Portico rejected an oversized service response.",
    );
  }
  const reader = response.body?.getReader?.();
  if (reader) {
    const chunks: Uint8Array[] = [];
    let total = 0;
    try {
      while (true) {
        const result = await awaitWithAbort(reader.read(), signal);
        if (result.done) break;
        const chunk = result.value;
        total += chunk.byteLength;
        if (total > maximumBytes) {
          await reader.cancel().catch(() => undefined);
          throw new ApiError(
            response.status || 0,
            "response_too_large",
            "Portico rejected an oversized service response.",
          );
        }
        chunks.push(chunk);
      }
    } finally {
      reader.releaseLock();
    }
    const bytes = new Uint8Array(total);
    let offset = 0;
    for (const chunk of chunks) {
      bytes.set(chunk, offset);
      offset += chunk.byteLength;
    }
    try {
      return new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    } catch {
      throw new ApiError(
        response.status || 0,
        "invalid_response_encoding",
        "Portico rejected a response that was not valid UTF-8.",
      );
    }
  }
  // React Native's fetch implementation buffers response bodies internally but
  // does not expose ReadableStream#getReader. HTTP/1.1 responses larger than
  // Go's automatic Content-Length threshold are therefore delivered as
  // chunked responses with neither a readable stream nor a length header. Read
  // the already-buffered native body and enforce the same byte ceiling after
  // decoding so legitimate chunked API responses remain usable on native.
  const text = await awaitWithAbort(response.text(), signal);
  if (utf8ByteLength(text) > maximumBytes)
    throw new ApiError(
      response.status || 0,
      "response_too_large",
      "Portico rejected an oversized service response.",
    );
  return text;
}

function utf8ByteLength(value: string): number {
  let bytes = 0;
  for (let index = 0; index < value.length; index++) {
    const code = value.charCodeAt(index);
    if (code < 0x80) bytes += 1;
    else if (code < 0x800) bytes += 2;
    else if (
      code >= 0xd800 &&
      code <= 0xdbff &&
      index + 1 < value.length &&
      value.charCodeAt(index + 1) >= 0xdc00 &&
      value.charCodeAt(index + 1) <= 0xdfff
    ) {
      bytes += 4;
      index++;
    } else bytes += 3;
  }
  return bytes;
}

export function parseRetryAfter(
  value: string | null | undefined,
  nowMs = Date.now(),
): Pick<ApiErrorMetadata, "retryAfter" | "retryAt" | "retryAfterMs"> {
  const retryAfter = value?.trim();
  if (!retryAfter) return {};
  if (/^\d+(?:\.\d+)?$/.test(retryAfter)) {
    const numericSeconds = Number(retryAfter);
    if (!Number.isFinite(numericSeconds)) return { retryAfter };
    const retryAfterMs = Math.min(
      24 * 60 * 60 * 1_000,
      Math.max(0, Math.ceil(numericSeconds * 1_000)),
    );
    return {
      retryAfter,
      retryAfterMs,
      retryAt: new Date(nowMs + retryAfterMs).toISOString(),
    };
  }
  const retryTimestamp = Date.parse(retryAfter);
  if (!Number.isFinite(retryTimestamp) || Math.abs(retryTimestamp) > 8.64e15)
    return { retryAfter };
  const retryAfterMs = Math.min(
    24 * 60 * 60 * 1_000,
    Math.max(0, retryTimestamp - nowMs),
  );
  return {
    retryAfter,
    retryAfterMs,
    retryAt: new Date(nowMs + retryAfterMs).toISOString(),
  };
}

function buildApiUrl(
  path: string,
  config: PorticoClientOptions,
  session?: LocalServerSession,
): string {
  path = canonicalAPIPath(path);
  const sessionBase =
    session?.apiBaseUrl ?? config.sessionStore?.get()?.apiBaseUrl;
  const base = trimTrailingSlash(
    sessionBase || resolveValue(config.apiBaseUrl, ""),
  );
  return trustedAPIURL(base, path, config.baseHref);
}

function buildResourceUrl(path: string, config: PorticoClientOptions): string {
  if (!path || !path.startsWith("/api/")) return path;
  const raw = buildApiUrl(path, config);
  const url = new URL(raw, baseHref(config.baseHref));
  return url.toString();
}

function urlOrigin(
  value: string,
  baseHrefProvider?: ValueProvider<string>,
): string {
  const parsed = new URL(value, baseHref(baseHrefProvider));
  return parsed.origin;
}

function trustedAPIURL(
  base: string,
  path: string,
  baseHrefProvider?: ValueProvider<string>,
): string {
  path = canonicalAPIPath(path);
  if (!base) return path;
  const parsedBase = new URL(base, baseHref(baseHrefProvider));
  if (
    (parsedBase.protocol !== "http:" && parsedBase.protocol !== "https:") ||
    parsedBase.username ||
    parsedBase.password ||
    parsedBase.search ||
    parsedBase.hash
  ) {
    throw new TypeError("Portico API origin is invalid.");
  }
  const resolved = new URL(
    path,
    `${trimTrailingSlash(parsedBase.toString())}/`,
  );
  if (resolved.origin !== parsedBase.origin) {
    throw new TypeError("Portico API path escaped its trusted origin.");
  }
  return `${trimTrailingSlash(base)}${path}`;
}

function resolveBase(
  value: ValueProvider<string> | undefined,
  fallback: string,
): string {
  return trimTrailingSlash(resolveValue(value, fallback));
}

function resolveValue<T>(value: ValueProvider<T> | undefined, fallback: T): T {
  return typeof value === "function"
    ? (value as () => T)()
    : (value ?? fallback);
}

function baseHref(value?: ValueProvider<string>): string {
  return resolveValue(value, "http://portico.local");
}

function trimTrailingSlash(value: string): string {
  return String(value || "").replace(/\/+$/, "");
}

function authHeader(session?: LocalServerSession): Record<string, string> {
  const token = session?.bootstrapAccessToken || session?.accessToken;
  return token ? { Authorization: `Bearer ${token}` } : {};
}

function normalizeClientInstanceId(value: string): string {
  const normalized = value.trim().slice(0, 120);
  return /^[A-Za-z0-9._-]+$/.test(normalized) ? normalized : "";
}

function normalizeHeaders(
  headers: HeadersInit | undefined,
): Record<string, string> {
  if (!headers) return {};
  if (typeof Headers !== "undefined" && headers instanceof Headers)
    return Object.fromEntries(headers.entries());
  if (Array.isArray(headers)) return Object.fromEntries(headers);
  return { ...(headers as Record<string, string>) };
}

function withRequestId(
  headers: HeadersInit | undefined,
  provider?: () => string,
): Record<string, string> {
  const normalized = normalizeHeaders(headers);
  if (!headerValue(normalized, "x-request-id"))
    normalized["X-Request-ID"] = createRequestId(provider);
  return normalized;
}

function createRequestId(provider?: () => string): string {
  const provided = provider?.().trim();
  if (provided) return provided.slice(0, 200);
  const cryptoObject = globalThis.crypto;
  if (typeof cryptoObject?.randomUUID === "function")
    return cryptoObject.randomUUID();
  return `req_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 14)}`;
}

function createSecureRotationKey(): string {
  const cryptoObject = globalThis.crypto;
  if (typeof cryptoObject?.getRandomValues !== "function") {
    throw new ApiError(
      0,
      "secure_random_unavailable",
      "This runtime cannot safely rotate the session credential.",
    );
  }
  const bytes = cryptoObject.getRandomValues(new Uint8Array(32));
  const alphabet =
    "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_";
  let output = "";
  let buffer = 0;
  let bits = 0;
  for (const byte of bytes) {
    buffer = (buffer << 8) | byte;
    bits += 8;
    while (bits >= 6) {
      bits -= 6;
      output += alphabet[(buffer >>> bits) & 63];
    }
  }
  if (bits > 0) output += alphabet[(buffer << (6 - bits)) & 63];
  return output;
}

function validRotationKey(value: string): boolean {
  return (
    value.length >= 43 && value.length <= 128 && /^[A-Za-z0-9_-]+$/.test(value)
  );
}

function headerValue(headers: Record<string, string>, name: string): string {
  const match = Object.entries(headers).find(
    ([key]) => key.toLowerCase() === name.toLowerCase(),
  );
  return match?.[1] ?? "";
}

function fetcher(transport?: FetchTransport): FetchTransport["fetch"] {
  const candidate = transport?.fetch ?? globalThis.fetch;
  if (!candidate)
    throw new Error(
      "No fetch implementation is available. Provide a transport adapter.",
    );
  return candidate.bind(transport ?? globalThis);
}

function jsonRequestCoalesceKey(
  method: string,
  url: string,
  headers: HeadersInit | undefined,
  body: unknown,
  budgetKey = "",
): string {
  if ((method !== "GET" && method !== "HEAD") || body !== undefined) return "";
  return `${method} ${url} ${stableHeadersKey(headers)} ${budgetKey}`;
}

function stableHeadersKey(headers: HeadersInit | undefined): string {
  const normalized = normalizeHeaders(headers);
  return JSON.stringify(
    Object.keys(normalized)
      .sort()
      .map((key) => [key.toLowerCase(), normalized[key]]),
  );
}

function coalescedJSONRequest<T>(
  inFlightJSONRequests: Map<string, InFlightJSONRequest>,
  key: string,
  signal: AbortSignal | null | undefined,
  start: (signal: AbortSignal) => Promise<T>,
): Promise<T> {
  if (signal?.aborted) return Promise.reject(abortError());
  let entry = inFlightJSONRequests.get(key);
  if (entry?.controller.signal.aborted) {
    if (inFlightJSONRequests.get(key) === entry)
      inFlightJSONRequests.delete(key);
    entry = undefined;
  }
  if (!entry) {
    const controller = new AbortController();
    const created: InFlightJSONRequest = {
      controller,
      consumers: 0,
      completed: false,
      promise: Promise.resolve(),
    };
    try {
      created.promise = start(controller.signal).finally(() => {
        created.completed = true;
        if (inFlightJSONRequests.get(key) === created)
          inFlightJSONRequests.delete(key);
      });
    } catch (error) {
      created.promise = Promise.reject(error).finally(() => {
        created.completed = true;
        if (inFlightJSONRequests.get(key) === created)
          inFlightJSONRequests.delete(key);
      });
    }
    inFlightJSONRequests.set(key, created);
    entry = created;
  }
  entry.consumers++;
  return new Promise<T>((resolve, reject) => {
    let settled = false;
    const cleanup = () => {
      signal?.removeEventListener("abort", onAbort);
      entry!.consumers = Math.max(0, entry!.consumers - 1);
      if (entry!.consumers === 0 && !entry!.completed) {
        if (inFlightJSONRequests.get(key) === entry)
          inFlightJSONRequests.delete(key);
        entry!.controller.abort();
      }
    };
    const onAbort = () => {
      if (settled) return;
      settled = true;
      cleanup();
      reject(abortError());
    };
    signal?.addEventListener("abort", onAbort, { once: true });
    entry.promise.then(
      (value) => {
        if (settled) return;
        settled = true;
        cleanup();
        resolve(value as T);
      },
      (error) => {
        if (settled) return;
        settled = true;
        cleanup();
        reject(error);
      },
    );
  });
}

function abortError(): Error {
  if (typeof DOMException !== "undefined")
    return new DOMException("The operation was aborted.", "AbortError");
  const error = new Error("The operation was aborted.");
  error.name = "AbortError";
  return error;
}

function now(config: PorticoClientOptions): number {
  return config.now?.() ?? performanceNow();
}

function performanceNow(): number {
  return typeof performance !== "undefined" &&
    typeof performance.now === "function"
    ? performance.now()
    : Date.now();
}

function requiredEventResourceId(value: string, name: string): string {
  const normalized = value.trim();
  if (!normalized || normalized.length > 256)
    throw new TypeError(`A valid ${name} is required.`);
  return normalized;
}

function runtimeAbortSignal(signal: PorticoAbortSignal): AbortSignal {
  // Client-created subscriptions are always coordinated through a native
  // AbortController; this cast stays private so the package declaration
  // surface remains portable for runtimes without DOM type libraries.
  return signal as AbortSignal;
}

function eventPollPath(path: string, request: PorticoLongPollRequest): string {
  const queryAt = path.indexOf("?");
  const pathname = queryAt < 0 ? path : path.slice(0, queryAt);
  const search = new URLSearchParams(
    queryAt < 0 ? "" : path.slice(queryAt + 1),
  );
  if (request.cursor) search.set("cursor", request.cursor);
  search.set("waitSeconds", String(request.waitSeconds));
  return `${pathname.replace(/\/$/, "")}/poll?${search.toString()}`;
}

async function authorizedEventResponse(
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  path: string,
  signal: AbortSignal,
  accept: "application/json" | "text/event-stream",
  fallbackCode: string,
  fallbackMessage: string,
  operationClass: OperationClass = "realtime stream",
): Promise<Response> {
  path = canonicalAPIPath(path);
  const transportOptions: TransportRequestOptions = {
    signal,
    operationClass,
    method: "GET",
  };
  const requestDeadline = transportDeadlineAt(config, transportOptions);
  return runTransportOperation(
    config,
    transportOptions,
    "GET",
    async (context) => {
      let session = await awaitWithAbort(
        Promise.resolve(credentials.current()),
        context.signal,
      );
      const url = buildApiUrl(path, config, session);
      const principalIdentity = requestPrincipalIdentity(
        session,
        urlOrigin(url, config.baseHref),
      );
      const init: RequestInit = {
        method: "GET",
        credentials: "include",
        headers: withRequestId(
          { Accept: accept, ...authHeader(session) },
          config.requestId,
        ),
      };
      let response = await transportFetch(config.transport, url, init, context);
      if (response.status === 401 && canAutomaticallyRefresh(path, session)) {
        await discardEventResponse(response);
        await awaitWithAbort(
          credentials.refreshAfterUnauthorized(
            currentAccessTokenFromSession(session),
            context.signal,
            () => {
              context.dispatched = true;
            },
          ),
          context.signal,
        );
        session = await awaitWithAbort(
          Promise.resolve(credentials.current()),
          context.signal,
        );
        const refreshedURL = buildApiUrl(path, config, session);
        if (
          refreshedURL !== url ||
          requestPrincipalIdentity(
            session,
            urlOrigin(refreshedURL, config.baseHref),
          ) !== principalIdentity
        ) {
          throw new ApiError(
            409,
            "request_superseded",
            "The active Portico server or viewer changed while opening an event stream.",
          );
        }
        response = await transportFetch(
          config.transport,
          url,
          {
            ...init,
            headers: {
              ...normalizeHeaders(init.headers),
              ...authHeader(session),
            },
          },
          context,
        );
      }
      if (!response.ok)
        await throwApiError(
          response,
          fallbackCode,
          fallbackMessage,
          context.signal,
        );
      return response;
    },
    undefined,
    requestDeadline,
  );
}

async function discardEventResponse(response: Response): Promise<void> {
  try {
    await response.body?.cancel?.();
  } catch {
    // The credential refresh remains authoritative even when a runtime cannot
    // explicitly cancel an already-finished unauthorized response body.
  }
}

async function pollEventEnvelope(
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  path: string,
  request: PorticoLongPollRequest,
  signal: AbortSignal,
  fallbackCode: string,
  fallbackMessage: string,
): Promise<unknown> {
  const response = await authorizedEventResponse(
    config,
    credentials,
    eventPollPath(path, request),
    signal,
    "application/json",
    fallbackCode,
    fallbackMessage,
    "long poll",
  );
  const text = await boundedResponseText(response, 4 * 1024 * 1024, signal);
  return text ? JSON.parse(text) : undefined;
}

function eventRecord(value: unknown, name: string): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new TypeError(`${name} is invalid`);
  return value as Record<string, unknown>;
}

function eventString(value: unknown, name: string, allowEmpty = false): string {
  if (
    typeof value !== "string" ||
    (!allowEmpty && !value.trim()) ||
    value.length > 4_096
  )
    throw new TypeError(`${name} is invalid`);
  return value;
}

function eventTimestamp(value: unknown, name: string): string {
  const timestamp = eventString(value, name);
  if (!Number.isFinite(Date.parse(timestamp)))
    throw new TypeError(`${name} is invalid`);
  return timestamp;
}

function parseAppEvent(value: unknown): AppEvent {
  const event = eventRecord(value, "application event");
  if (!Number.isSafeInteger(event.id) || Number(event.id) < 0)
    throw new TypeError("application event id is invalid");
  eventString(event.type, "application event type");
  eventTimestamp(event.createdAt, "application event timestamp");
  if (
    !Array.isArray(event.tags) ||
    event.tags.length > 128 ||
    event.tags.some((tag) => typeof tag !== "string" || !tag.trim())
  ) {
    throw new TypeError("application event tags are invalid");
  }
  for (const [field, maximum] of [
    ["resource", 128],
    ["resourceId", 512],
  ] as const) {
    const candidate = event[field];
    if (
      candidate !== undefined &&
      (typeof candidate !== "string" || candidate.length > maximum)
    ) {
      throw new TypeError(`application event ${field} is invalid`);
    }
  }
  if (event.fields !== undefined) {
    const fields = event.fields;
    if (
      !fields ||
      typeof fields !== "object" ||
      Array.isArray(fields) ||
      Object.keys(fields).length > 64 ||
      Object.entries(fields).some(
        ([key, value]) =>
          !key ||
          key.length > 128 ||
          typeof value !== "string" ||
          value.length > 4_096,
      )
    ) {
      throw new TypeError("application event fields are invalid");
    }
  }
  return event as unknown as AppEvent;
}

function parsePlaybackCommandEvent(
  value: unknown,
  allowEmptyId = false,
): PlaybackCommand {
  const command = eventRecord(value, "playback command event");
  eventString(command.id, "playback command id", allowEmptyId);
  const action = eventString(
    command.action,
    "playback command action",
    allowEmptyId,
  );
  if (
    action &&
    !["play", "pause", "seek", "stop", "load", "next", "previous"].includes(
      action,
    )
  ) {
    throw new TypeError("playback command action is invalid");
  }
  if (command.issuedAt !== undefined && command.issuedAt !== "")
    eventTimestamp(command.issuedAt, "playback command timestamp");
  return command as PlaybackCommand;
}

function parseWatchWithFriendsGroupEvent(
  value: unknown,
): WatchWithFriendsGroup {
  const group = eventRecord(value, "Watch With Friends event");
  eventString(group.id, "Watch With Friends group id");
  if (!Number.isSafeInteger(group.revision) || Number(group.revision) < 0)
    throw new TypeError("Watch With Friends revision is invalid");
  if (
    !Number.isSafeInteger(group.reconnectGeneration) ||
    Number(group.reconnectGeneration) < 0
  )
    throw new TypeError("Watch With Friends reconnect generation is invalid");
  eventTimestamp(group.serverTime, "Watch With Friends server time");
  return group as unknown as WatchWithFriendsGroup;
}

async function streamTypedJSONEvents<TEvent>(
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  path: string,
  signal: AbortSignal,
  onEvent: (event: TEvent) => void,
  parseEvent: (value: unknown) => TEvent,
  fallbackCode: string,
  fallbackMessage: string,
  onConnected?: () => void | Promise<void>,
): Promise<void> {
  const response = await authorizedEventResponse(
    config,
    credentials,
    path,
    signal,
    "text/event-stream",
    fallbackCode,
    fallbackMessage,
  );
  assertPorticoEventStreamContentType(response.headers?.get?.("Content-Type"));
  await onConnected?.();
  if (signal.aborted) return;
  await consumePorticoSSE(config, response, signal, parseEvent, onEvent);
}

async function consumePorticoSSE<TEvent>(
  config: PorticoClientOptions,
  response: Response,
  signal: AbortSignal,
  parseEvent: (value: unknown) => TEvent,
  onEvent: (event: TEvent) => void,
  options: { ignoreInvalidPayloads?: boolean } = {},
): Promise<void> {
  const adapter = config.eventStream ?? browserEventStreamAdapter();
  const decoder = new PorticoSSEDecoder();
  const emit = (payloads: string[]) => {
    for (const payload of payloads) {
      try {
        dispatchPorticoJSONEvent(payload, parseEvent, onEvent);
      } catch (error) {
        if (
          options.ignoreInvalidPayloads &&
          error instanceof PorticoSSEProtocolError &&
          error.code === "invalid_event_payload"
        ) {
          continue;
        }
        if (
          error instanceof Error &&
          error.name === "PorticoSSEProtocolError"
        ) {
          throw new PorticoEventProtocolError(
            "Portico event stream did not match the advertised contract",
            error,
          );
        }
        throw error;
      }
    }
  };
  for await (const chunk of adapter.read(response, signal)) {
    if (signal.aborted) break;
    const decodedChunk =
      typeof chunk === "string" || !config.eventStream?.decode
        ? chunk
        : config.eventStream.decode(chunk, { stream: true });
    emit(decoder.push(decodedChunk, signal));
  }
  if (config.eventStream?.flush) {
    const tail = config.eventStream.flush();
    if (tail) emit(decoder.push(tail, signal));
  }
  emit(decoder.finish(signal));
}

function subscribeAppEventTransport(
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  subscription: PorticoEventSubscriptionOptions<AppEvent>,
): Promise<void> {
  const path = "/api/events";
  return runPorticoEventSubscription(
    subscription,
    {
      stream: (signal, onEvent, onConnected) =>
        streamAppEvents(
          config,
          credentials,
          runtimeAbortSignal(signal),
          onEvent,
          onConnected,
        ),
      poll: (request, signal) =>
        pollEventEnvelope(
          config,
          credentials,
          path,
          request,
          runtimeAbortSignal(signal),
          "event_poll_failed",
          "Unable to poll application events.",
        ),
      parseEvent: parseAppEvent,
      eventIdentity: (event) => event.id,
    },
    config.eventSubscriptionRuntime,
  );
}

function subscribeNotificationEventTransport(
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  params: ApiOperationQuery<"streamViewerNotificationInvalidations">,
  subscription: PorticoEventSubscriptionOptions<NotificationInvalidation>,
): Promise<void> {
  const path = `/api/notifications/events${apiQuery(params)}`;
  return runPorticoEventSubscription(
    subscription,
    {
      stream: (signal, onEvent, onConnected) =>
        streamViewerNotificationInvalidations(
          config,
          credentials,
          params,
          runtimeAbortSignal(signal),
          onEvent,
          onConnected,
        ),
      poll: (request, signal) =>
        pollEventEnvelope(
          config,
          credentials,
          path,
          request,
          runtimeAbortSignal(signal),
          "notification_poll_failed",
          "Unable to poll notification updates.",
        ),
      parseEvent: parseNotificationInvalidation,
      eventIdentity: (event) => event.occurredAt,
    },
    config.eventSubscriptionRuntime,
  );
}

function subscribePlaybackCommandEventTransport(
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  sessionId: string,
  subscription: PorticoEventSubscriptionOptions<PlaybackCommand>,
): Promise<void> {
  const path = `/api/playback-sessions/${encodeURIComponent(sessionId)}/command/events`;
  return runPorticoEventSubscription(
    subscription,
    {
      stream: (signal, onEvent, onConnected) =>
        streamTypedJSONEvents(
          config,
          credentials,
          path,
          runtimeAbortSignal(signal),
          onEvent,
          parsePlaybackCommandEvent,
          "playback_command_stream_failed",
          "Unable to open playback command updates.",
          onConnected,
        ),
      poll: (request, signal) =>
        pollEventEnvelope(
          config,
          credentials,
          path,
          request,
          runtimeAbortSignal(signal),
          "playback_command_poll_failed",
          "Unable to poll playback command updates.",
        ),
      parseEvent: parsePlaybackCommandEvent,
      eventIdentity: (event) => event.id || undefined,
    },
    config.eventSubscriptionRuntime,
  );
}

function subscribeWatchWithFriendsEventTransport(
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  groupId: string,
  subscription: PorticoEventSubscriptionOptions<WatchWithFriendsGroup>,
): Promise<void> {
  const path = `/api/watch-with-friends/groups/${encodeURIComponent(groupId)}/events`;
  return runPorticoEventSubscription(
    subscription,
    {
      stream: (signal, onEvent, onConnected) =>
        streamWatchWithFriendsGroupEvents(
          config,
          credentials,
          groupId,
          runtimeAbortSignal(signal),
          onEvent,
          onConnected,
        ),
      poll: (request, signal) =>
        pollEventEnvelope(
          config,
          credentials,
          path,
          request,
          runtimeAbortSignal(signal),
          "watch_with_friends_poll_failed",
          "Unable to poll Watch With Friends updates.",
        ),
      parseEvent: parseWatchWithFriendsGroupEvent,
      eventIdentity: (event) =>
        `${event.id}:${event.revision}:${event.reconnectGeneration}`,
    },
    config.eventSubscriptionRuntime,
  );
}

async function streamAppEvents(
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  signal: AbortSignal,
  onEvent: (event: AppEvent) => void,
  onConnected?: () => void | Promise<void>,
): Promise<void> {
  const response = await authorizedEventResponse(
    config,
    credentials,
    "/api/events",
    signal,
    "text/event-stream",
    "event_stream_failed",
    "Unable to open event stream.",
  );
  assertPorticoEventStreamContentType(response.headers?.get?.("Content-Type"));
  await onConnected?.();
  if (signal.aborted) return;
  await consumePorticoSSE(config, response, signal, parseAppEvent, onEvent);
}

async function streamViewerNotificationInvalidations(
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  params: ApiOperationQuery<"streamViewerNotificationInvalidations">,
  signal: AbortSignal,
  onInvalidation: (event: NotificationInvalidation) => void,
  onConnected?: () => void | Promise<void>,
): Promise<void> {
  const path = `/api/notifications/events${apiQuery(params)}`;
  const response = await authorizedEventResponse(
    config,
    credentials,
    path,
    signal,
    "text/event-stream",
    "notification_stream_failed",
    "Unable to open notification updates.",
  );
  assertPorticoEventStreamContentType(response.headers?.get?.("Content-Type"));
  await onConnected?.();
  if (signal.aborted) return;
  await consumePorticoSSE(
    config,
    response,
    signal,
    parseNotificationInvalidation,
    onInvalidation,
  );
}

async function streamWatchWithFriendsGroupEvents(
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  groupId: string,
  signal: AbortSignal,
  onGroup: (group: WatchWithFriendsGroup) => void,
  onConnected?: () => void | Promise<void>,
): Promise<void> {
  const normalizedGroupId = groupId.trim();
  if (!normalizedGroupId)
    throw new Error("A Watch With Friends group is required.");
  const path = `/api/watch-with-friends/groups/${encodeURIComponent(normalizedGroupId)}/events`;
  const response = await authorizedEventResponse(
    config,
    credentials,
    path,
    signal,
    "text/event-stream",
    "watch_with_friends_stream_failed",
    "Watch With Friends disconnected.",
  );
  assertPorticoEventStreamContentType(response.headers?.get?.("Content-Type"));
  await onConnected?.();
  if (signal.aborted) return;
  await consumePorticoSSE(
    config,
    response,
    signal,
    parseWatchWithFriendsGroupEvent,
    onGroup,
  );
}

function browserEventStreamAdapter(): EventStreamAdapter {
  let decoder: TextDecoder | undefined;
  return {
    async *read(response, signal) {
      const reader = response.body?.getReader?.();
      if (!reader) {
        throw new Error(
          "The fetch response does not expose a readable event-stream body. Provide PorticoClientOptions.eventStream for this platform.",
        );
      }
      try {
        while (!signal.aborted) {
          const { value, done } = await reader.read();
          if (done) break;
          if (value) yield value;
        }
      } finally {
        if (signal.aborted) await reader.cancel().catch(() => undefined);
        reader.releaseLock();
      }
    },
    decode(chunk, options) {
      if (!decoder) {
        if (typeof globalThis.TextDecoder !== "function") {
          throw new Error(
            "No UTF-8 event-stream decoder is available. Provide PorticoClientOptions.eventStream.decode or yield decoded strings.",
          );
        }
        decoder = new globalThis.TextDecoder("utf-8");
      }
      return decoder.decode(chunk, options);
    },
    flush() {
      return decoder?.decode() ?? "";
    },
  };
}
