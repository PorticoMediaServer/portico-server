import type {
  DownloadOption,
  DownloadOptionsResponse,
  DownloadPreparation,
  DownloadPreparationSingleCreateRequest,
  MediaDownloadGrantResponse
} from "./types.js";
import type {HostedConnectionRuntimeAdapters, ResolvedHostedConnectionRuntimeAdapters} from "./hostedConnectionRuntime.js";
import {
  parseOfflineDownloadAuthorizationReceipt,
  revalidateOfflineDownloadAuthorization,
  validateOfflineDownloadAuthorizationReceipt,
  type OfflineDownloadAuthorizationReceipt,
  type OfflineDownloadAuthorizationRevalidationAPI,
  type PinnedServerIdentity
} from "./offlineDownloadAuthorization.js";
import type {ViewerScope} from "./viewerScope.js";
import { validatePorticoUrl } from "./urlPolicy.js";
export { validatePorticoUrl, trustedRouteOrigin, type PorticoUrlPurpose, type PorticoUrlPolicyOptions } from "./urlPolicy.js";

export type NativeDownloadNetwork = "offline" | "wifi" | "cellular" | "unknown";
export type NativeDownloadState = "preparing" | "queued" | "waiting-network" | "waiting-storage" | "waiting-grant" | "transferring" | "paused" | "failed" | "cancelled" | "verifying" | "available-offline" | "authorization-unverified" | "deleting" | "removed";

export type NativeDownloadScope = { serverId: string; accountId: string; profileId: string; installationId: string };

export type NativeDownloadOfflineMetadata = {
  title: string;
  mediaType: string;
  durationSeconds?: number;
  localAssetId?: string;
  artworkAssetIds: readonly string[];
};

export type NativeDownloadQueueItem = {
  id: string;
  /** Immutable identity of this logical attempt. Retry creates a new value. */
  attemptId: string;
  /** Monotonic generation used to reject callbacks from superseded OS work. */
  generation: number;
  preparationId: string;
  mediaId: string;
  scope: NativeDownloadScope;
  state: NativeDownloadState;
  variant?: NativeDownloadVariant;
  preparationProgress?: number;
  sizeKind?: "unknown" | "estimated" | "exact";
  expectedBytes?: number;
  /** Explicit conservative reservation for a preparation whose size is not yet known. */
  reservedBytes?: number;
  transferredBytes: number;
  transferGrantExpiresAt?: string;
  authorizationReceipt?: OfflineDownloadAuthorizationReceipt;
  authorizationViewerScope?: ViewerScope;
  offline?: NativeDownloadOfflineMetadata;
  /** Server-side prepared artifact lifetime; it does not expire a completed local copy. */
  preparationArtifactExpiresAt?: string;
  failureMessageId?: "download.failed" | "download.storage-full";
  watched?: boolean;
};

export type NativeDownloadQueueEvent =
  | { type: "pause"; attemptId: string; generation: number }
  | { type: "resume"; attemptId: string; generation: number }
  | { type: "cancel"; attemptId: string; generation: number }
  | { type: "retry"; attemptId: string; nextAttemptId: string; generation: number }
  | { type: "remove"; attemptId: string; generation: number }
  | { type: "grant"; expiresAt: string; receipt: OfflineDownloadAuthorizationReceipt; viewerScope: ViewerScope; attemptId: string; generation: number }
  | { type: "complete"; finalBytes: number; attemptId: string; generation: number }
  | { type: "watched"; watched: boolean; attemptId: string; generation: number }
  | { type: "fail"; messageId?: "download.failed" | "download.storage-full"; attemptId: string; generation: number };

export type NativeDownloadPlatformTask = {
  attemptId: string;
  generation: number;
  state: "queued" | "running" | "paused" | "cancelled" | "complete";
};

export type NativeDownloadReconciliation = {
  queue: NativeDownloadQueueItem[];
  /** Platform work that must be cancelled before its tombstone may be collected. */
  cancelPlatformTasks: NativeDownloadPlatformTask[];
  /** Tombstones whose matching platform task is confirmed absent. */
  confirmedTombstones: NativeDownloadTombstone[];
};

export type NativeDownloadTombstone = {
  id: string;
  attemptId: string;
  generation: number;
  scope: NativeDownloadScope;
  removedAt: string;
};

export type NativeDownloadGrantBinding = NativeDownloadScope & {
  trustedRouteUrl: string;
  preparationId: string;
  attemptId: string;
  generation: number;
  viewerScope: ViewerScope;
  pinnedServerIdentity: PinnedServerIdentity;
};

export type NativeDownloadSchedulerPolicy = {
  wifiOnly: boolean;
  storageLimitBytes?: number;
  usedStorageBytes: number;
  /**
   * Optional conservative reservation for unknown-size jobs. Without either
   * this value or an item reservation, an unknown-size transfer waits.
   */
  unknownSizeReservationBytes?: number;
  deleteWatched: boolean;
};

export type NativeDownloadVariant = {
  optionId: string;
  kind: "source" | "optimized";
  qualityProfile: string;
  label: string;
  description?: string;
  expectedBytes?: number;
  container?: string;
  videoCodec?: string;
  audioCodec?: string;
};

export type NativeDownloadPreparationResult = {
  variant: NativeDownloadVariant;
  preparation: DownloadPreparation;
};

export type NativeDownloadTransferPlan = {
  /** Clean server-issued URL. Native adapters must not persist or log the plan. */
  downloadUrl: string;
  authorization: string;
  grantExpiresAt: string;
  preparationId: string;
  mediaId: string;
  qualityProfile: string;
  expectedBytes?: number;
  sizeKind: "unknown" | "estimated" | "exact";
  authorizationReceipt: OfflineDownloadAuthorizationReceipt;
  authorizationViewerScope: ViewerScope;
  binding: Readonly<NativeDownloadGrantBinding>;
};

export type NativeDownloadPreparationAPI = {
  downloadOptions(mediaId: string): Promise<DownloadOptionsResponse>;
  createDownloadPreparation(body: DownloadPreparationSingleCreateRequest): Promise<DownloadPreparation>;
  downloadPreparation(id: string): Promise<DownloadPreparation>;
  createDownloadPreparationGrant(id: string, body: {delivery: "native"}): Promise<MediaDownloadGrantResponse>;
};

export type NativeDownloadPublicationEvidence = Readonly<{
  viewerScope: ViewerScope;
  pinnedServerIdentity: PinnedServerIdentity;
  artifactSha256: string;
  artifactSizeBytes: number;
  offline: NativeDownloadOfflineMetadata;
}>;

export class NativeDownloadWorkflowError extends Error {
  readonly messageId: "download.failed" | "download.storage-full";
  constructor(readonly code: "option_unavailable" | "preparation_not_ready" | "grant_invalid", messageId: "download.failed" | "download.storage-full" = "download.failed") {
    super(messageId);
    this.name = "NativeDownloadWorkflowError";
    this.messageId = messageId;
  }
}

/** Returns every available server option, including optimized/transcoded variants. */
export function nativeDownloadVariants(response: DownloadOptionsResponse): NativeDownloadVariant[] {
  if (!response.canDownload) return [];
  return response.options.flatMap((option) => {
    // Optimized profiles are first-class choices even before their artifact
    // exists. The preparation endpoint owns that asynchronous materialization.
    if (!option.available && !(option.kind === "optimized" && option.requiresOptimizedVersion)) return [];
    const qualityProfile = nativeDownloadQualityProfile(option);
    if (!qualityProfile) return [];
    return [{
      optionId: option.id,
      kind: option.kind,
      qualityProfile,
      label: option.label,
      description: option.description,
      expectedBytes: positiveBytes(option.sizeBytes),
      container: option.container,
      videoCodec: option.videoCodec,
      audioCodec: option.audioCodec
    }];
  });
}

/** Fetches server options and starts its asynchronous preparation contract. */
export async function beginNativeDownloadPreparation(
  api: NativeDownloadPreparationAPI,
  mediaId: string,
  optionId: string
): Promise<NativeDownloadPreparationResult> {
  const options = await api.downloadOptions(mediaId);
  const variant = nativeDownloadVariants(options).find((candidate) => candidate.optionId === optionId);
  if (!variant) throw new NativeDownloadWorkflowError("option_unavailable");
  const preparation = await api.createDownloadPreparation({ mediaId, qualityProfile: variant.qualityProfile });
  return { variant, preparation };
}

export async function refreshNativeDownloadPreparation(
  api: NativeDownloadPreparationAPI,
  result: NativeDownloadPreparationResult
): Promise<NativeDownloadPreparationResult> {
  return { ...result, preparation: await api.downloadPreparation(result.preparation.id) };
}

/** Obtains an ephemeral transfer grant only after the prepared artifact is ready. */
export async function authorizeNativeDownloadTransfer(
  api: NativeDownloadPreparationAPI,
  result: NativeDownloadPreparationResult,
  now = Date.now(),
  binding: NativeDownloadGrantBinding,
  runtimeAdapters: HostedConnectionRuntimeAdapters | ResolvedHostedConnectionRuntimeAdapters = {}
): Promise<NativeDownloadTransferPlan> {
  const { preparation, variant } = result;
  if (preparation.state !== "ready" || preparation.qualityProfile !== variant.qualityProfile) {
    throw new NativeDownloadWorkflowError("preparation_not_ready");
  }
  const grant = await api.createDownloadPreparationGrant(preparation.id, {delivery: "native"});
  const expiry = Date.parse(grant.expiresAt);
  const rawReceipt = (grant as MediaDownloadGrantResponse & {authorizationReceipt?: unknown}).authorizationReceipt;
  let receipt: OfflineDownloadAuthorizationReceipt;
  try {
    receipt = parseOfflineDownloadAuthorizationReceipt(rawReceipt);
  } catch {
    throw new NativeDownloadWorkflowError("grant_invalid");
  }
  if (!grant.downloadUrl || !grant.grantToken || !Number.isFinite(expiry) || expiry <= now || grant.profile !== variant.qualityProfile) {
    throw new NativeDownloadWorkflowError("grant_invalid");
  }
  let downloadUrl: string;
  try {
    const validated = validatePorticoUrl(grant.downloadUrl, "download-grant", {trustedOrigin: binding.trustedRouteUrl});
    downloadUrl = new URL(validated, binding.trustedRouteUrl).href;
  } catch {
    throw new NativeDownloadWorkflowError("grant_invalid");
  }
  if (binding.preparationId !== preparation.id || !validScope(binding) || binding.attemptId.trim() === "" || !Number.isSafeInteger(binding.generation) || binding.generation < 0 ||
      binding.viewerScope.serverId !== binding.serverId || binding.viewerScope.accountId !== binding.accountId ||
      binding.viewerScope.profileId !== binding.profileId || binding.pinnedServerIdentity.serverId !== binding.serverId) {
    throw new NativeDownloadWorkflowError("grant_invalid");
  }
  const validation = await validateOfflineDownloadAuthorizationReceipt(receipt, {
    binding: {
      storedViewerScope: binding.viewerScope,
      originatingServerId: binding.serverId,
      preparationId: preparation.id,
      mediaId: preparation.mediaId,
      mediaVersionId: receipt.preparation.mediaVersionId,
      qualityId: preparation.qualityProfile,
      artifactSha256: receipt.artifact.sha256,
      artifactSizeBytes: receipt.artifact.sizeBytes
    },
    activeViewerScope: binding.viewerScope,
    pinnedIdentity: binding.pinnedServerIdentity,
    runtimeAdapters,
    now
  });
  if (validation.state !== "valid") throw new NativeDownloadWorkflowError("grant_invalid");
  return {
    downloadUrl,
    authorization: `PorticoDownload ${grant.grantToken}`,
    grantExpiresAt: grant.expiresAt,
    preparationId: preparation.id,
    mediaId: preparation.mediaId,
    qualityProfile: preparation.qualityProfile,
    expectedBytes: positiveBytes(preparation.sizeBytes) ?? variant.expectedBytes,
    sizeKind: preparation.sizeKind,
    authorizationReceipt: receipt,
    authorizationViewerScope: binding.viewerScope,
    binding: Object.freeze({...binding})
  };
}

export function nativeDownloadQueueItemFromPreparation(
  id: string,
  result: NativeDownloadPreparationResult,
  scope: NativeDownloadScope,
  offline?: NativeDownloadQueueItem["offline"]
): NativeDownloadQueueItem {
  const expectedBytes = positiveBytes(result.preparation.sizeBytes) ?? result.variant.expectedBytes;
  return {
    id,
    attemptId: id,
    generation: 0,
    preparationId: result.preparation.id,
    mediaId: result.preparation.mediaId,
    scope,
    state: nativeStateForPreparation(result.preparation),
    variant: result.variant,
    preparationProgress: result.preparation.progress,
    sizeKind: result.preparation.sizeKind,
    expectedBytes,
    reservedBytes: expectedBytes,
    transferredBytes: 0,
    offline,
    preparationArtifactExpiresAt: result.preparation.artifactExpiresAt,
    failureMessageId: result.preparation.failureMessageId
  };
}

/** Applies authoritative server preparation progress to a durable queue row. */
export function applyNativeDownloadPreparation(item: NativeDownloadQueueItem, preparation: DownloadPreparation, fence: {attemptId: string; generation: number}): NativeDownloadQueueItem {
  if (isNativeDownloadTerminal(item.state) || !matchesNativeDownloadAttempt(item, fence)) return item;
  if (preparation.id !== item.preparationId || preparation.mediaId !== item.mediaId) return item;
  const expectedBytes = positiveBytes(preparation.sizeBytes) ?? item.expectedBytes;
  return {
    ...item,
    preparationProgress: preparation.progress,
    sizeKind: preparation.sizeKind,
    expectedBytes,
    reservedBytes: expectedBytes ?? item.reservedBytes,
    preparationArtifactExpiresAt: preparation.artifactExpiresAt,
    failureMessageId: preparation.failureMessageId,
    state: preparation.state === "ready" && ["waiting-network", "waiting-storage", "waiting-grant", "transferring", "verifying", "available-offline", "authorization-unverified", "deleting"].includes(item.state)
      ? item.state
      : nativeStateForPreparation(preparation)
  };
}

export function sameNativeDownloadScope(left: NativeDownloadScope, right: NativeDownloadScope): boolean {
  return left.serverId === right.serverId && left.accountId === right.accountId && left.profileId === right.profileId && left.installationId === right.installationId;
}

export function nativeDownloadSchedule(
  queue: readonly NativeDownloadQueueItem[], activeScope: NativeDownloadScope, network: NativeDownloadNetwork,
  policy: NativeDownloadSchedulerPolicy, now = Date.now()
): NativeDownloadQueueItem[] {
  let reserved = policy.usedStorageBytes;
  return queue.map((item) => {
    if (!sameNativeDownloadScope(item.scope, activeScope)) return item;
    if (item.state === "available-offline") {
      if (!item.authorizationReceipt) return {...item, state: "deleting"};
      const verifyBy = Date.parse(item.authorizationReceipt?.verifyBy ?? "");
      return Number.isFinite(verifyBy) && now < verifyBy ? item : {...item, state: "authorization-unverified"};
    }
    if (["preparing", "verifying", "authorization-unverified", "deleting", "paused", "cancelled", "removed", "failed"].includes(item.state)) return item;
    if (network === "offline" || (policy.wifiOnly && network !== "wifi")) return { ...item, state: "waiting-network" };
    const boundedBytes = positiveBytes(item.expectedBytes) ?? positiveBytes(item.reservedBytes) ?? positiveBytes(policy.unknownSizeReservationBytes);
    if (boundedBytes === undefined) return { ...item, state: "waiting-storage" };
    const remaining = Math.max(0, boundedBytes - item.transferredBytes);
    if (policy.storageLimitBytes !== undefined && reserved + remaining > policy.storageLimitBytes) return { ...item, state: "waiting-storage" };
    reserved += remaining;
    const expiry = Date.parse(item.transferGrantExpiresAt ?? "");
    if (!Number.isFinite(expiry) || expiry <= now) return { ...item, state: "waiting-grant" };
    return { ...item, state: "transferring" };
  });
}

function nativeDownloadQualityProfile(option: DownloadOption): string | undefined {
  const explicit = option.profile?.trim();
  if (explicit) return explicit;
  if (option.kind === "source") return "source";
  return option.id.trim() || undefined;
}

function nativeStateForPreparation(preparation: DownloadPreparation): NativeDownloadState {
  switch (preparation.state) {
    case "ready": return "queued";
    case "queued":
    case "running": return "preparing";
    case "paused": return "paused";
    case "cancelled": return "cancelled";
    case "failed":
    case "unavailable": return "failed";
  }
}

function positiveBytes(value: number | undefined): number | undefined {
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : undefined;
}

export function nativeDownloadsToDeleteAfterWatch(queue: readonly NativeDownloadQueueItem[], scope: NativeDownloadScope, deleteWatched: boolean): string[] {
  if (!deleteWatched) return [];
  return queue.filter((item) => item.state === "available-offline" && item.watched && sameNativeDownloadScope(item.scope, scope)).map((item) => item.id);
}

export function applyNativeDownloadProgress(item: NativeDownloadQueueItem, transferredBytes: number, totalBytes: number | undefined, fence: { attemptId: string; generation: number }): NativeDownloadQueueItem {
  if (isNativeDownloadTerminal(item.state) || !matchesNativeDownloadAttempt(item, fence) || !transitionAllowed(item.state, "transferring")) return item;
  const authoritativeTotal = positiveBytes(totalBytes);
  const expectedBytes = authoritativeTotal ?? item.expectedBytes;
  const transferred = Math.max(item.transferredBytes, Math.max(0, transferredBytes));
  const bytesComplete = expectedBytes !== undefined && transferred >= expectedBytes && (authoritativeTotal !== undefined || item.sizeKind === "exact");
  return { ...item, sizeKind: authoritativeTotal !== undefined ? "exact" : item.sizeKind, expectedBytes, transferredBytes: expectedBytes ? Math.min(transferred, expectedBytes) : transferred, state: bytesComplete ? "verifying" : "transferring", failureMessageId: undefined };
}

/**
 * Publishes local bytes only after the signed receipt, current viewer scope,
 * pinned Server key, and the locally measured artifact all agree.
 */
export async function publishNativeDownloadArtifact(
  item: NativeDownloadQueueItem,
  evidence: NativeDownloadPublicationEvidence,
  fence: {attemptId: string; generation: number},
  runtimeAdapters: HostedConnectionRuntimeAdapters | ResolvedHostedConnectionRuntimeAdapters = {},
  now = Date.now()
): Promise<NativeDownloadQueueItem> {
  if (!matchesNativeDownloadAttempt(item, fence) || item.state !== "verifying") return item;
  if (!item.authorizationReceipt || !item.authorizationViewerScope) return {...item, state: "deleting"};
  if (!sameNativeDownloadViewerIdentity(item.scope, evidence.viewerScope)) return item;
  const receipt = item.authorizationReceipt;
  const validation = await validateOfflineDownloadAuthorizationReceipt(receipt, {
    binding: {
      storedViewerScope: item.authorizationViewerScope,
      originatingServerId: item.scope.serverId,
      preparationId: item.preparationId,
      mediaId: item.mediaId,
      mediaVersionId: receipt.preparation.mediaVersionId,
      qualityId: item.variant?.qualityProfile ?? receipt.preparation.qualityId,
      artifactSha256: evidence.artifactSha256,
      artifactSizeBytes: evidence.artifactSizeBytes
    },
    activeViewerScope: evidence.viewerScope,
    pinnedIdentity: evidence.pinnedServerIdentity,
    runtimeAdapters,
    now
  });
  if (validation.state === "out-of-scope") return item;
  if (validation.state === "invalid") return {...item, state: "deleting"};
  const nextState = validation.state === "valid" ? "available-offline" : "authorization-unverified";
  return {
    ...item,
    state: nextState,
    preparationProgress: 100,
    sizeKind: "exact",
    expectedBytes: evidence.artifactSizeBytes,
    reservedBytes: evidence.artifactSizeBytes,
    transferredBytes: evidence.artifactSizeBytes,
    transferGrantExpiresAt: undefined,
    authorizationReceipt: validation.receipt,
    offline: evidence.offline,
    failureMessageId: undefined
  };
}

export async function revalidateNativeDownloadArtifact(
  item: NativeDownloadQueueItem,
  api: OfflineDownloadAuthorizationRevalidationAPI,
  evidence: NativeDownloadPublicationEvidence,
  runtimeAdapters: HostedConnectionRuntimeAdapters | ResolvedHostedConnectionRuntimeAdapters = {},
  now = Date.now()
): Promise<NativeDownloadQueueItem> {
  if (item.state !== "authorization-unverified") return item;
  if (!item.authorizationReceipt || !item.authorizationViewerScope) return {...item, state: "deleting"};
  if (!sameNativeDownloadViewerIdentity(item.scope, evidence.viewerScope)) return item;
  const validationContext = {
    binding: {
      storedViewerScope: item.authorizationViewerScope,
      originatingServerId: item.scope.serverId,
      preparationId: item.preparationId,
      mediaId: item.mediaId,
      mediaVersionId: item.authorizationReceipt.preparation.mediaVersionId,
      qualityId: item.variant?.qualityProfile ?? item.authorizationReceipt.preparation.qualityId,
      artifactSha256: evidence.artifactSha256,
      artifactSizeBytes: evidence.artifactSizeBytes
    },
    activeViewerScope: evidence.viewerScope,
    pinnedIdentity: evidence.pinnedServerIdentity,
    runtimeAdapters,
    now
  } as const;
  const decision = await revalidateOfflineDownloadAuthorization(api, item.authorizationReceipt, validationContext);
  if (decision.action === "preserve") return item;
  if (decision.action === "delete") return {...item, state: "deleting"};
  // The shared revalidation owner has already required a new receipt ID and
  // fully verified the replacement against the pinned identity and binding.
  return {...item, state: "available-offline", authorizationReceipt: decision.receipt};
}

/** Pure queue transition for durable native stores. OS transfer adapters apply the resulting state and effects. */
export function reduceNativeDownloadQueueItem(item: NativeDownloadQueueItem, event: NativeDownloadQueueEvent): NativeDownloadQueueItem {
  if (!matchesNativeDownloadAttempt(item, event)) return item;
  const target = eventTargetState(item, event);
  if (target && !transitionAllowed(item.state, target)) return item;
  switch (event.type) {
    case "pause":
      return { ...item, state: "paused" };
    case "resume":
      return item.state !== "paused" ? item : { ...item, state: "queued", failureMessageId: undefined };
    case "cancel":
      return { ...item, state: "cancelled", transferGrantExpiresAt: undefined };
    case "retry":
      if (item.state !== "failed" && item.state !== "cancelled") return item;
      if (!event.nextAttemptId.trim() || event.nextAttemptId === item.attemptId) return item;
      return { ...item, attemptId: event.nextAttemptId, generation: item.generation + 1, state: "queued", transferredBytes: 0, transferGrantExpiresAt: undefined, authorizationReceipt: undefined, authorizationViewerScope: undefined, offline: undefined, failureMessageId: undefined };
    case "remove":
      return { ...item, state: "removed", transferGrantExpiresAt: undefined, authorizationReceipt: undefined, authorizationViewerScope: undefined, offline: item.offline ? { ...item.offline, localAssetId: undefined, artworkAssetIds: [] } : undefined };
    case "grant":
      return isNativeDownloadTerminal(item.state) ? item : { ...item, transferGrantExpiresAt: event.expiresAt, authorizationReceipt: event.receipt, authorizationViewerScope: event.viewerScope, state: item.state === "waiting-grant" ? "queued" : item.state };
    case "complete": {
      if (["cancelled", "removed", "available-offline", "authorization-unverified", "deleting"].includes(item.state)) return item;
      const finalBytes = Math.max(item.transferredBytes, Math.max(0, event.finalBytes));
      return {
        ...item,
        state: "verifying",
        preparationProgress: 100,
        sizeKind: "exact",
        expectedBytes: finalBytes,
        reservedBytes: finalBytes,
        transferredBytes: finalBytes,
        transferGrantExpiresAt: undefined,
        failureMessageId: undefined
      };
    }
    case "watched":
      return { ...item, watched: event.watched };
    case "fail":
      return isNativeDownloadTerminal(item.state) ? item : { ...item, state: "failed", failureMessageId: event.messageId ?? "download.failed" };
  }
}

export function nativeDownloadTombstone(item: NativeDownloadQueueItem, removedAt = new Date()): NativeDownloadTombstone {
  if (!Number.isFinite(removedAt.getTime())) throw new TypeError("Download removal time is invalid.");
  return {id: item.id, attemptId: item.attemptId, generation: item.generation, scope: item.scope, removedAt: removedAt.toISOString()};
}

/** Reconciles durable state against the platform registry before callbacks are accepted. */
export function reconcileNativeDownloads(queue: readonly NativeDownloadQueueItem[], tombstones: readonly NativeDownloadTombstone[], platformTasks: readonly NativeDownloadPlatformTask[]): NativeDownloadReconciliation {
  const removed = new Map(tombstones.map(value => [`${value.id}\n${value.scope.serverId}\n${value.scope.accountId}\n${value.scope.profileId}\n${value.scope.installationId}`, value]));
  const tasks = new Map(platformTasks.map(task => [`${task.attemptId}\n${task.generation}`, task]));
  const reconciled = queue.flatMap(item => {
    const tombstone = removed.get(`${item.id}\n${item.scope.serverId}\n${item.scope.accountId}\n${item.scope.profileId}\n${item.scope.installationId}`);
    if (tombstone && tombstone.generation >= item.generation) return [];
    const task = tasks.get(`${item.attemptId}\n${item.generation}`);
    if (!task && item.state === "transferring") return [{...item, state: "queued" as const, transferGrantExpiresAt: undefined}];
    return [item];
  });
  const cancelPlatformTasks = platformTasks.filter(task => tombstones.some(value => value.attemptId === task.attemptId && value.generation >= task.generation));
  const confirmedTombstones = tombstones.filter(value => !platformTasks.some(task => task.attemptId === value.attemptId && task.generation <= value.generation));
  return {queue: reconciled, cancelPlatformTasks, confirmedTombstones};
}

export function matchesNativeDownloadAttempt(item: NativeDownloadQueueItem, fence: {attemptId: string; generation: number}): boolean {
  return fence.attemptId === item.attemptId && fence.generation === item.generation;
}

export const NATIVE_DOWNLOAD_TRANSITIONS: Readonly<Record<NativeDownloadState, readonly NativeDownloadState[]>> = Object.freeze({
  preparing: ["preparing", "queued", "paused", "failed", "cancelled", "removed"],
  queued: ["queued", "waiting-network", "waiting-storage", "waiting-grant", "transferring", "paused", "failed", "cancelled", "removed"],
  "waiting-network": ["queued", "waiting-network", "waiting-storage", "waiting-grant", "transferring", "paused", "failed", "cancelled", "removed"],
  "waiting-storage": ["queued", "waiting-network", "waiting-storage", "waiting-grant", "transferring", "paused", "failed", "cancelled", "removed"],
  "waiting-grant": ["queued", "waiting-network", "waiting-storage", "waiting-grant", "transferring", "paused", "failed", "cancelled", "removed"],
  transferring: ["transferring", "paused", "failed", "cancelled", "verifying", "removed"],
  paused: ["queued", "paused", "cancelled", "removed"],
  failed: ["queued", "failed", "removed"],
  cancelled: ["queued", "cancelled", "removed"],
  verifying: ["verifying", "available-offline", "authorization-unverified", "deleting", "failed", "removed"],
  "available-offline": ["available-offline", "authorization-unverified", "deleting", "removed"],
  "authorization-unverified": ["authorization-unverified", "available-offline", "deleting", "removed"],
  deleting: ["deleting", "removed"],
  removed: ["removed"]
});

function transitionAllowed(from: NativeDownloadState, to: NativeDownloadState): boolean {
  return NATIVE_DOWNLOAD_TRANSITIONS[from].includes(to);
}

function eventTargetState(item: NativeDownloadQueueItem, event: NativeDownloadQueueEvent): NativeDownloadState | undefined {
  switch (event.type) {
    case "pause": return "paused";
    case "resume": return "queued";
    case "cancel": return "cancelled";
    case "retry": return "queued";
    case "remove": return "removed";
    case "grant": return item.state === "waiting-grant" ? "queued" : item.state;
    case "complete": return "verifying";
    case "watched": return item.state;
    case "fail": return "failed";
  }
}

function validScope(scope: NativeDownloadScope): boolean {
  return [scope.serverId, scope.accountId, scope.profileId, scope.installationId].every(value => typeof value === "string" && value.trim() !== "");
}

function sameNativeDownloadViewerIdentity(scope: NativeDownloadScope, viewerScope: ViewerScope): boolean {
  return scope.serverId === viewerScope.serverId && scope.accountId === viewerScope.accountId && scope.profileId === viewerScope.profileId;
}

function isNativeDownloadTerminal(state: NativeDownloadState): boolean {
  return state === "cancelled" || state === "removed" || state === "verifying" || state === "available-offline" || state === "authorization-unverified" || state === "deleting";
}
