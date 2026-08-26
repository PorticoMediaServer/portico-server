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
  MediaDownloadGrantRequest,
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
  LocalizationInfo,
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
  PlaybackIntent,
  PlaybackCommand,
  PlaybackHandoffRequest,
  PlaybackRenegotiationRequest,
  PlaybackReceiver,
  PlaybackNextResponse,
  PlaybackPreparedResponse,
  PlaybackPrepareNextRequest,
  PlaybackProgressAcknowledgement,
  PlaybackProgressEvent,
  PlaybackProgressInput,
  PlaybackContinuationCredential,
  PlaybackContinuationState,
  PlaybackContinuationRotateRequest,
  PlaybackQueueResponse,
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

export interface DurablePlaybackProgressRecord {
  version: "v1";
  /** Opaque principal/server/session/generation identity created by Client Core. */
  key: string;
  /** Exact uncertain event followed by at most one coalesced successor. */
  events: readonly PlaybackProgressEvent[];
  updatedAt: string;
}

export interface PlaybackProgressDurabilityAdapter {
  load(): Promise<readonly DurablePlaybackProgressRecord[]>;
  save(record: DurablePlaybackProgressRecord): Promise<void>;
  remove(key: string): Promise<void>;
}

export interface PorticoClientOptions {
  apiBaseUrl?: ValueProvider<string>;
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

function normalizedPlaybackProgressInput(
  event: PlaybackProgressInput,
): PlaybackProgressInput {
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
  const credentials = createCredentialLifecycle(options);
  const eventSubscriptions = new PorticoEventSubscriptionCoordinator();
  const inFlightJSONRequests = new Map<string, InFlightJSONRequest>();
  const playbackProgressMailboxes = new Map<string, PlaybackProgressMailbox>();
  const playbackProgressSequences = new Map<string, number>();
  const playbackSessionGenerations = new Map<string, number>();
  const playbackSessionNextEventSequences = new Map<string, number>();
  const playbackStoppingSessions = new Set<string>();
  const durablePlaybackProgress = new Map<
    string,
    DurablePlaybackProgressRecord
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

  const playbackProgressKey = (
    sessionId: string,
    generation: number,
    session = credentials.peek(),
    serverOrigin = "",
  ) =>
    JSON.stringify([
      session?.serverId ||
        trimTrailingSlash(
          serverOrigin ||
            session?.apiBaseUrl ||
            resolveValue(options.apiBaseUrl, ""),
        ),
      session?.authority ?? "local",
      session?.accountId ?? "",
      session?.profileId ?? "",
      sessionId,
      Math.max(0, Math.trunc(generation)),
    ]);
  const loadDurablePlaybackProgress = async (): Promise<void> => {
    if (
      durablePlaybackProgressLoaded ||
      !options.playbackProgressDurabilityAdapter
    )
      return;
    durablePlaybackProgressLoad ??= options.playbackProgressDurabilityAdapter
      .load()
      .then((records) => {
        if (records.length > MAX_PLAYBACK_PROGRESS_MAILBOXES)
          throw new TypeError(
            "The durable playback progress outbox is too large.",
          );
        for (const record of records) {
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
            !Array.isArray(identity) ||
            identity.length !== 6 ||
            identity.some((part, index) =>
              index === 5
                ? !Number.isSafeInteger(part) || Number(part) < 0
                : typeof part !== "string" || part.length > 2_048,
            )
          ) {
            throw new TypeError(
              "A durable playback progress identity is invalid.",
            );
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
    const record: DurablePlaybackProgressRecord = {
      version: "v1",
      key,
      events: [
        mailbox.inFlight.event,
        ...(mailbox.successor ? [mailbox.successor.event] : []),
      ],
      updatedAt: new Date(options.now?.() ?? Date.now()).toISOString(),
    };
    await options.playbackProgressDurabilityAdapter.save(record);
    durablePlaybackProgress.set(key, record);
  };
  const removeDurablePlaybackProgress = async (key: string): Promise<void> => {
    if (options.playbackProgressDurabilityAdapter)
      await options.playbackProgressDurabilityAdapter.remove(key);
    durablePlaybackProgress.delete(key);
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
      .then(() => delivery.send(delivery.event))
      .then(async (ack) => {
        const current = playbackProgressMailboxes.get(key);
        if (!current || current.inFlight !== delivery) return;
        current.delivering = false;
        current.touchedAt = options.now?.() ?? Date.now();
        if (!acknowledgementIsDurable(ack)) {
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
        if (current.successor && !delivery.event.completed) {
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
          deliverPlaybackProgress(key);
          return;
        }
        if (current.successor) {
          for (const waiter of current.successor.waiters.splice(0))
            waiter.resolve(ack);
        }
        await removeDurablePlaybackProgress(key);
        playbackProgressMailboxes.delete(key);
        trimPlaybackProgressState();
      })
      .catch((error) => {
        const current = playbackProgressMailboxes.get(key);
        if (!current || current.inFlight !== delivery) return;
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
        if (mailbox.inFlight.event.completed) {
          mailbox.inFlight.waiters.push(waiter);
        } else if (mailbox.successor?.event.completed && !body.completed) {
          mailbox.successor.waiters.push(waiter);
        } else {
          mailbox.successor = {
            event: orderedClientPlaybackProgressEvent(key, body),
            send,
            waiters: [...(mailbox.successor?.waiters ?? []), waiter],
          };
        }
        if (!mailbox.delivering) {
          // Keep the progress event immutable, but use the newest live request
          // context so an aborted signal or rotated continuation token cannot
          // poison every retry.
          mailbox.inFlight.send = send;
          deliverPlaybackProgress(key);
        }
      });
    });
  const forgetClientPlaybackProgress = (sessionId: string) => {
    playbackSessionGenerations.delete(sessionId);
    playbackSessionNextEventSequences.delete(sessionId);
    for (const [key, mailbox] of playbackProgressMailboxes) {
      const identity = JSON.parse(key) as unknown[];
      if (identity[4] !== sessionId) continue;
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
  };
  const waitForPendingPlaybackProgress = (sessionId: string): Promise<void> => {
    const waits: Promise<unknown>[] = [];
    for (const [key, mailbox] of playbackProgressMailboxes) {
      const identity = JSON.parse(key) as unknown[];
      if (identity[4] !== sessionId) continue;
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

  return {
    request,
    formRequest,
    // Adapters that use a route-specific request shape can still publish the
    // authoritative session generation/sequence into Client Core before they
    // enqueue durable progress.
    acceptPlaybackSession: normalizeSessionPlayback,
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
    localization: () => request<LocalizationInfo>("/api/localization"),
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
    preferences: () => request<UserPreferences>("/api/account/preferences"),
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
      imageOptions: { width?: number; height?: number } = {},
    ) =>
      imageResourceUrl(
        `/api/artwork/${encodeURIComponent(id)}/${encodeURIComponent(kind)}`,
        imageOptions,
      ),
    mediaStreamUrl: (id: string) =>
      resourceUrl(`/api/media/${encodeURIComponent(id)}/stream`),
    mediaDownloadUrl: (id: string, profileId?: string) => {
      const search = new URLSearchParams();
      if (profileId) search.set("profile", profileId);
      const query = search.toString();
      return resourceUrl(
        `/api/media/${encodeURIComponent(id)}/download${query ? `?${query}` : ""}`,
      );
    },
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
    createDownloadPreparationGrant: (id: string, init?: RequestSignal) =>
      request<MediaDownloadGrantResponse>(
        `/api/download-preparations/${encodeURIComponent(id)}/grant`,
        { ...init, method: "POST" },
      ),
    createMediaDownloadGrant: (
      id: string,
      body: MediaDownloadGrantRequest = { profile: "source" },
      init?: RequestSignal,
    ) =>
      request<MediaDownloadGrantResponse>(
        `/api/media/${encodeURIComponent(id)}/download-grants`,
        { ...init, method: "POST", body },
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
      playbackOptions: {
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
      } = {},
      init?: RequestSignal,
    ) =>
      request<PlaybackResponse>("/api/playback-sessions", {
        ...init,
        method: "POST",
        body: {
          mediaId,
          clientInstanceId: clientInstanceId(),
          clientProfile: profile(),
          intent: playbackOptions.intent,
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
    createCastBootstrap: (body: CastBootstrapRequest, init?: RequestSignal) =>
      request<CastBootstrapResponse>("/api/playback/cast/bootstrap", {
        ...init,
        method: "POST",
        // The configured installation identity is the same value attached to
        // the source playback session. Never let a UI confuse a playback
        // session id with the stable client instance during Cast handoff.
        body: {
          ...body,
          clientInstanceId: clientInstanceId() || body.clientInstanceId,
        },
      }),
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
    nextPlayable: (mediaId: string, queueMediaIds: string[] = []) =>
      request<PlaybackNextResponse>("/api/playback/next", {
        method: "POST",
        body: { mediaId, queueMediaIds },
      }),
    playbackQueue: (mediaId: string, queueMediaIds: string[] = []) =>
      request<PlaybackQueueResponse>("/api/playback/queue", {
        method: "POST",
        body: { mediaId, queueMediaIds },
      }),
    playbackSessionQueue: (sessionId: string) =>
      request<PlaybackSessionQueueResponse>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/queue`,
      ),
    updatePlaybackSessionQueue: (
      sessionId: string,
      body: PlaybackSessionQueueReplaceRequest,
      init?: RequestSignal,
    ) =>
      request<PlaybackSessionQueueResponse>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/queue`,
        { ...init, method: "PUT", body },
      ),
    mutatePlaybackSessionQueue: (
      sessionId: string,
      body: PlaybackSessionQueueRequest,
      init?: RequestSignal,
    ) =>
      request<PlaybackSessionQueueResponse>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/queue`,
        { ...init, method: "PATCH", body },
      ),
    prepareNextPlayback: (
      sessionId: string,
      body: PlaybackPrepareNextRequest = {},
      init?: RequestSignal,
    ) =>
      request<PlaybackPreparedResponse>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/prepare-next`,
        {
          ...init,
          method: "POST",
          body: { ...body, clientProfile: body.clientProfile ?? profile() },
        },
      ).then((response) => ({
        ...response,
        playback: normalizeSessionPlayback(response.playback),
      })),
    handoffPlayback: (
      sessionId: string,
      body: PlaybackHandoffRequest = {},
      init?: RequestSignal,
    ) =>
      request<PlaybackResponse>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/handoff`,
        {
          ...init,
          method: "POST",
          body: { ...body, clientProfile: body.clientProfile ?? profile() },
        },
      ).then(normalizeSessionPlayback),
    renegotiatePlayback: (
      sessionId: string,
      body: PlaybackRenegotiationRequest,
      init?: RequestSignal,
    ) =>
      request<PlaybackResponse>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/renegotiate`,
        {
          ...init,
          method: "POST",
          body: { ...body, clientProfile: body.clientProfile ?? profile() },
        },
      ).then(normalizeSessionPlayback),
    restoreActivePlayback: (intent?: PlaybackIntent) =>
      request<PlaybackRestoreResponse>("/api/playback/active", {
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
      })),
    touchPlayback: async (
      sessionId: string,
      body: PlaybackProgressInput,
      init?: RequestSignal,
    ) => {
      if (playbackStoppingSessions.has(sessionId)) {
        throw new ApiError(
          409,
          "playback_stopping",
          "Playback is stopping and cannot accept more progress updates.",
        );
      }
      const session = await credentials.current();
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
      if (playbackStoppingSessions.has(sessionId)) {
        throw new ApiError(
          409,
          "playback_stopping",
          "Playback is stopping and cannot accept more progress updates.",
        );
      }
      const session = await credentials.current();
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
      init?: RequestSignal,
    ) =>
      request<{ ok: boolean }>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/continuation`,
        {
          ...init,
          method: "DELETE",
          authorization: {
            mode: "playback-continuation",
            token: credential.token,
            origin: credential.origin,
          },
        },
      ),
    renewPlaybackMediaGrant: (sessionId: string, init?: RequestSignal) =>
      request<MediaGrant>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/media-grant`,
        { ...init, method: "POST" },
      ),
    stopPlayback: async (sessionId: string, init?: RequestSignal) => {
      if (playbackStoppingSessions.has(sessionId)) {
        throw new ApiError(
          409,
          "playback_stopping",
          "Playback is already stopping.",
        );
      }
      playbackStoppingSessions.add(sessionId);
      try {
        await loadDurablePlaybackProgress();
        for (const key of durablePlaybackProgress.keys()) {
          const identity = JSON.parse(key) as unknown[];
          if (identity[4] !== sessionId) continue;
          restoreDurablePlaybackProgress(key, (event) =>
            request<PlaybackProgressAcknowledgement>(
              `/api/playback-sessions/${encodeURIComponent(sessionId)}`,
              { ...init, method: "PATCH", body: event },
            ),
          );
        }
        await waitForPendingPlaybackProgress(sessionId);
        const response = await request<{ ok: boolean }>(
          `/api/playback-sessions/${encodeURIComponent(sessionId)}`,
          { ...init, method: "DELETE" },
        );
        forgetClientPlaybackProgress(sessionId);
        return response;
      } finally {
        playbackStoppingSessions.delete(sessionId);
      }
    },
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
    issuePlaybackCommand: (
      sessionId: string,
      body: {
        action: "play" | "pause" | "seek" | "stop" | "load";
        mediaId?: string;
        positionSeconds?: number;
        message?: string;
      },
    ) =>
      request<PlaybackCommand>(
        `/api/playback-sessions/${encodeURIComponent(sessionId)}/command`,
        { method: "POST", body },
      ),
    playbackTargets: (init?: Pick<RequestInit, "signal">) =>
      request<ListResponse<PlaybackTarget>>(
        `/api/playback/targets?clientInstanceId=${encodeURIComponent(clientInstanceId())}`,
        init,
      ),
    playbackReceivers: () =>
      request<ListResponse<PlaybackReceiver>>("/api/playback/receivers"),
    createPlaybackReceiver: (body: {
      name: string;
      app?: string;
      platform?: string;
      supportedCommands?: string[];
    }) =>
      request<PlaybackReceiver>("/api/playback/receivers", {
        method: "POST",
        body,
      }),
    touchPlaybackReceiver: (id: string) =>
      request<PlaybackReceiver>(
        `/api/playback/receivers/${encodeURIComponent(id)}`,
        { method: "PATCH" },
      ),
    issuePlaybackReceiverCommand: (
      id: string,
      body: { mediaId: string; positionSeconds?: number },
    ) =>
      request<PlaybackCommand>(
        `/api/playback/receivers/${encodeURIComponent(id)}/command`,
        { method: "POST", body: { action: "load", ...body } },
      ),
    playbackReceiverEventsUrl: (receiverId: string) =>
      resourceUrl(
        `/api/playback/receivers/${encodeURIComponent(receiverId)}/events`,
      ),
    subscribePlaybackReceiverEvents: (
      receiverId: string,
      subscription: PorticoEventSubscriptionOptions<PlaybackReceiver>,
    ) => {
      const normalizedReceiverId = requiredEventResourceId(
        receiverId,
        "playback receiver",
      );
      return eventSubscriptions.run(
        `playback-receiver-events:${normalizedReceiverId}`,
        subscription.signal,
        (signal) =>
          subscribePlaybackReceiverEventTransport(
            options,
            credentials,
            normalizedReceiverId,
            { ...subscription, signal },
          ),
      );
    },
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
      mediaId: string,
      expectedRevision: number,
      idempotencyKey: string,
      init?: RequestSignal,
    ) =>
      request<WatchWithFriendsGroup>(
        `/api/watch-with-friends/groups/${encodeURIComponent(id)}/queue/${encodeURIComponent(mediaId)}?expectedRevision=${encodeURIComponent(String(expectedRevision))}&idempotencyKey=${encodeURIComponent(idempotencyKey)}`,
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
      form.append("password", input.password);
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
      playbackOptions: DVRPlaybackSessionCreateRequest = {},
      init?: RequestSignal,
    ) =>
      request<PlaybackResponse>(
        `/api/dvr/recordings/${encodeURIComponent(id)}/playback`,
        {
          ...init,
          method: "POST",
          body: {
            ...playbackOptions,
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
      body: LibraryChannelTuneRequest = {},
      init?: Pick<RequestInit, "signal">,
    ) =>
      request<LibraryChannelTuneResponse>(
        `/api/library-channels/${encodeURIComponent(channelId)}/tune`,
        {
          ...init,
          method: "POST",
          body: {
            ...body,
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
      playbackOptions: {
        clientProfile?: PlaybackClientProfile;
        intent?: PlaybackIntent;
      } = {},
      init?: RequestSignal,
    ) =>
      request<PlaybackResponse>("/api/live-tv/play", {
        ...init,
        method: "POST",
        body: {
          channelId,
          clientInstanceId: clientInstanceId(),
          clientProfile: playbackOptions.clientProfile ?? profile(),
          intent: playbackOptions.intent,
        },
      }).then(normalizePlaybackResponse),
    openLiveTvStream: (
      channelId: string,
      playbackOptions: {
        clientProfile?: PlaybackClientProfile;
        intent?: PlaybackIntent;
      } = {},
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
            intent: playbackOptions.intent,
          },
        },
      ).then(normalizePlaybackResponse),
    closeLiveTvStream: (channelId: string, sessionId: string) =>
      request<{ ok: boolean }>(
        `/api/live-tv/streams/${encodeURIComponent(channelId)}/close`,
        { method: "POST", body: { sessionId } },
      ),
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
    /^\/api\/(?:account\/)?servers\/[^/]+(?:\/members\/[^/]+|\/invites\/[^/]+)?$/.test(
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
    deleteServer: (serverId: string) =>
      request<{ ok: boolean }>(
        `/api/account/servers/${encodeURIComponent(serverId)}`,
        { method: "DELETE" },
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

  const peek = () => config.sessionStore?.get() ?? cached;
  const current = ():
    | LocalServerSession
    | undefined
    | Promise<LocalServerSession | undefined> => {
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
    authorization.mode === "playback-continuation"
      ? trustedAPIURL(authorization.origin, path, config.baseHref)
      : buildApiUrl(path, config, session);
  const principalIdentity = requestPrincipalIdentity(
    session,
    urlOrigin(url, config.baseHref),
  );
  const requestHeaders = {
    ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
    ...requestAuthorizationHeaders(authorization, session),
    ...(method === "GET" || method === "HEAD"
      ? {}
      : { "X-Portico-CSRF": resolveValue(config.csrfToken, "1") }),
    ...callerHeaders(options.headers),
  };
  const headers = withRequestId(requestHeaders, config.requestId);
  const init: RequestInit = {
    ...rest,
    method,
    headers,
    credentials: authorization.mode === "anonymous" ? "omit" : "include",
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
): Promise<T> {
  if (!response.ok) await throwApiError(response, undefined, undefined, signal);
  if (response.status === 204) return undefined as T;
  const text = await boundedResponseText(response, 4 * 1024 * 1024, signal);
  if (!text) return undefined as T;
  try {
    const parsed = decodeHighRiskResponse(path, method, JSON.parse(text)) as T;
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

function parsePlaybackReceiverEvent(value: unknown): PlaybackReceiver {
  const receiver = eventRecord(value, "playback receiver event");
  eventString(receiver.id, "playback receiver id");
  eventString(receiver.name, "playback receiver name");
  eventString(receiver.code, "playback receiver code");
  eventTimestamp(receiver.createdAt, "playback receiver creation time");
  eventTimestamp(receiver.lastSeenAt, "playback receiver heartbeat time");
  if (
    !Array.isArray(receiver.supportedCommands) ||
    receiver.supportedCommands.some((command) => command !== "load")
  ) {
    throw new TypeError("playback receiver commands are invalid");
  }
  parsePlaybackCommandEvent(receiver.command, true);
  return receiver as unknown as PlaybackReceiver;
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

function subscribePlaybackReceiverEventTransport(
  config: PorticoClientOptions,
  credentials: CredentialLifecycle,
  receiverId: string,
  subscription: PorticoEventSubscriptionOptions<PlaybackReceiver>,
): Promise<void> {
  const path = `/api/playback/receivers/${encodeURIComponent(receiverId)}/events`;
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
          parsePlaybackReceiverEvent,
          "playback_receiver_stream_failed",
          "Unable to open playback receiver updates.",
          onConnected,
        ),
      poll: (request, signal) =>
        pollEventEnvelope(
          config,
          credentials,
          path,
          request,
          runtimeAbortSignal(signal),
          "playback_receiver_poll_failed",
          "Unable to poll playback receiver updates.",
        ),
      parseEvent: parsePlaybackReceiverEvent,
      eventIdentity: (event) =>
        `${event.id}:${event.command.id ?? ""}:${event.lastSeenAt}`,
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
  // Notification invalidations are hints rather than authoritative state. Preserve
  // the established forward-compatible behavior: discard an unknown payload and
  // continue with later frames, while retaining strict framing/size enforcement.
  await consumePorticoSSE(
    config,
    response,
    signal,
    parseNotificationInvalidation,
    onInvalidation,
    { ignoreInvalidPayloads: true },
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
