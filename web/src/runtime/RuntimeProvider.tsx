import {
  connectResilientHostedServer,
  connectTrustedServerRecord,
  browserPlaybackClientProfile,
  CredentialCleanupUncertainError,
  createHostedServicesClient,
  createMemorySessionStore,
  createPorticoClient,
  decideProfileSelection,
  defaultAccountServerInstallationPreferences,
  HostedRoutePublicationPendingError,
  isTerminalServerAuthorizationFailure,
  LocalNetworkRouteUnavailableError,
  NearbyRouteAvailableError,
  knownProductMessageId,
  productMessageIdForProblemCode,
  refreshTrustedServerRoute,
  sameViewerScope,
  trustedHostedDocumentKeysFromKeySet,
  TrustedServerCredentialPublicationError,
  TrustedServerCandidateActivationError,
  TrustedServerDurabilityUncertainError,
  TrustedServerPublicationBlockedError,
  ViewerRuntimeTeardownError,
  viewerScopeFromAuthMe,
  type HostedServer,
  type HostedProfileSelectionEnvelope,
  type HostedRouteDocument,
  type HostedRouteEntry,
  type HostedRoutePreference,
  type AuthMeResponse,
  type LocalServerSession,
  type ProductMessageId,
  type PreparedTrustedServerConnection,
  type TrustedServerCandidatePublication,
  type SessionStore,
  type TrustedServerConnectionRecord,
  type TrustedServerRemovalTombstone,
  type ViewerScope,
  ApiError,
} from "@porticomediaserver/client-core";
import {
  type ReactNode,
  useCallback,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
} from "react";
import { HttpPorticoDataSource } from "../data/httpSource";
import type { PorticoDataSource, Viewer } from "../data/models";
import { WebViewerRuntime } from "../data/viewerRuntime";
import {
  classifyRuntimeFailure,
  initialRuntimeState,
  mergeRuntimeEnvironment,
  resolveRuntimeConfig,
  runtimeReducer,
  type HostedServerSummary,
  type RuntimeConfig,
} from "./runtimeMachine";
import { RuntimeContext, type RuntimeContextValue } from "./RuntimeContext";
import {
  automaticHostedAvailabilityRetry,
  createHostedAvailabilityRetryCohort,
} from "./hostedAvailability";
import {
  hostedCSRFToken,
  rememberHostedCSRFToken,
} from "./hostedBrowserSecurity";
import { browserHostedTerminalMutationDurability } from "./hostedTerminalMutationDurability";
import {
  broadcastAccountFence,
  subscribeAccountFences,
  withAccountPublicationLock,
  withAccountPublicationLocks,
} from "./accountPublicationFence";
import {
  createBrowserHostedConnectionVault,
  HOSTED_BROWSER_SESSION_TTL_MS,
  type HostedAccountSnapshot,
  type HostedConnectionVault,
} from "./hostedConnectionVault";
import { browserPlaybackProgressDurability } from "./playbackProgressDurability";
import {
  accountFenceFromTombstoneEvent,
  clearAccountAfterVerifiedCleanup,
  GLOBAL_SIGN_OUT_FENCE_ID,
  markAccountSignedOut,
  protectHostedConnectionVault,
  signedOutAccountQuarantine,
  signedOutAccountRestoreStatus,
  SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX,
  SignedOutAccountRestoreBlockedError,
} from "./signedOutAccountLedger";
import { ambientCookieRestoreStatus } from "./ambientCookieQuarantine";
import {
  localHTTPHostname,
  validServerSetupReturnUrl,
} from "./serverSetupReturnUrl";
import { routePublicationRetryPlan } from "./routePublicationRetry";

function profileSelectionMessageId(reason: unknown): ProductMessageId {
  const candidate = reason as
    { messageId?: unknown; code?: unknown } | undefined;
  const explicit = knownProductMessageId(
    typeof candidate?.messageId === "string" ? candidate.messageId : undefined,
  );
  const permitted = new Set<ProductMessageId>([
    "auth.profile-not-available",
    "auth.profile-not-found",
    "auth.profile-pin-invalid",
    "auth.profile-pin-retry-later",
    "auth.profile-pin-required",
    "auth.profile-selection-failed",
    "auth.profile-temporarily-locked",
  ]);
  if (explicit && permitted.has(explicit)) return explicit;
  const fromCode = productMessageIdForProblemCode(
    typeof candidate?.code === "string" ? candidate.code : undefined,
  );
  return fromCode && permitted.has(fromCode)
    ? fromCode
    : "auth.profile-selection-failed";
}

class HostedContinuationError extends Error {
  constructor(
    readonly phase:
      | "profile-directory"
      | "profile-selection"
      | "local-login-authorization"
      | "membership-mutation",
    readonly reason: unknown,
  ) {
    super(
      phase === "profile-directory"
        ? "Portico could not load the account profile directory."
        : phase === "profile-selection"
          ? "Portico could not verify the selected account profile."
          : phase === "local-login-authorization"
            ? "Portico could not complete local server authorization."
            : "Portico could not confirm the pending account action.",
    );
    this.name = "HostedContinuationError";
  }
}

function classifyHostedSessionCheckFailure(
  reason: unknown,
): "session-expired" | "hosted-session" {
  if (!(reason instanceof ApiError)) return "hosted-session";
  return [
    "invalid_portico_session",
    "invalid_refresh_token",
    "refresh_token_reuse",
    "session_expired",
  ].includes(reason.code)
    ? "session-expired"
    : "hosted-session";
}

function hostedAvailabilityRetryFields(
  reason: unknown,
  idempotentMutation = false,
): {
  automaticAvailabilityRetry?: boolean;
  availabilityRetryAfterMs?: number;
  availabilityRetryAt?: string;
} {
  const status = reason instanceof ApiError ? reason.status : undefined;
  const idempotentTransient =
    idempotentMutation &&
    (reason instanceof TypeError ||
      (reason instanceof DOMException && reason.name === "TimeoutError") ||
      status === 408 ||
      status === 425 ||
      status === 429 ||
      (status !== undefined && status >= 500));
  if (!automaticHostedAvailabilityRetry(reason) && !idempotentTransient)
    return {};
  const candidate = reason as { retryAfterMs?: unknown; retryAt?: unknown };
  return {
    automaticAvailabilityRetry: true,
    ...(typeof candidate.retryAfterMs === "number" &&
    Number.isFinite(candidate.retryAfterMs)
      ? { availabilityRetryAfterMs: candidate.retryAfterMs }
      : {}),
    ...(typeof candidate.retryAt === "string"
      ? { availabilityRetryAt: candidate.retryAt }
      : {}),
  };
}

export type HostedLocalLoginIntent = {
  serverId: string;
  serverName: string;
  callbackUrl: string;
  localOrigin: string;
  state: string;
  serverPublicKeyFingerprint: string;
  publicConsoleOriginGeneration: number;
  installationId?: string;
};

type HostedBootstrapIntent = {
  inviteId?: string;
  claimCode?: string;
  claimServerName?: string;
  claimReturnUrl?: string;
  resetToken?: string;
  ssoOnboardingToken?: string;
  deviceAuthorizationRequested?: boolean;
  deviceAuthorizationCode?: string;
  genericDeviceAuthorizationRequested?: boolean;
  genericDeviceAuthorizationCode?: string;
  genericDeviceAuthorizationProvider?: "google" | "apple";
  genericDeviceAuthorizationNativeReturn?: boolean;
  localLogin?: HostedLocalLoginIntent;
  localLoginRecoveryFailure?: "expired" | "unavailable" | "lost";
};

const LOCAL_LOGIN_HANDOFF_STORAGE_KEY = "portico.hosted.local-login-handoff.v1";
const LOCAL_LOGIN_HANDOFF_RECOVERY_TTL_MS = 5 * 60 * 1000;
const SERVER_CLAIM_HANDOFF_STORAGE_KEY =
  "portico.hosted.server-claim-handoff.v1";
const SERVER_CLAIM_HANDOFF_RECOVERY_TTL_MS = 10 * 60 * 1000;
const DEVICE_AUTHORIZATION_HANDOFF_STORAGE_KEY =
  "portico.hosted.device-authorization-handoff.v1";
const DEVICE_AUTHORIZATION_HANDOFF_RECOVERY_TTL_MS = 10 * 60 * 1000;
const GENERIC_DEVICE_AUTHORIZATION_HANDOFF_STORAGE_KEY =
  "portico.hosted.generic-device-authorization-handoff.v1";
const SSO_ONBOARDING_HANDOFF_STORAGE_KEY =
  "portico.hosted.sso-onboarding-handoff.v1";
const SSO_ONBOARDING_HANDOFF_RECOVERY_TTL_MS = 10 * 60 * 1000;

type StoredLocalLoginHandoff = {
  version: 1;
  expiresAt: number;
  intent: HostedLocalLoginIntent;
};

type StoredServerClaimHandoff = {
  version: 1;
  expiresAt: number;
  claimCode: string;
  serverName?: string;
  returnUrl?: string;
};

type HostedAccountIdentity = {
  authenticated: boolean;
  user?: { id: string; username: string; email: string };
};

type ActiveSelection = {
  generation: number;
  controller: AbortController;
  settled: Promise<void>;
};

function withRuntimeDeadline<T>(
  request: Promise<T>,
  timeoutMs: number,
  message: string,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const timeout = window.setTimeout(
      () => reject(new TypeError(message)),
      timeoutMs,
    );
    request.then(
      (value) => {
        window.clearTimeout(timeout);
        resolve(value);
      },
      (reason) => {
        window.clearTimeout(timeout);
        reject(reason);
      },
    );
  });
}

function withCancellableRuntimeDeadline<T>(
  timeoutMs: number,
  message: string,
  operation: (signal: AbortSignal) => Promise<T>,
): Promise<T> {
  const controller = new AbortController();
  const timeout = window.setTimeout(
    () => controller.abort(new DOMException(message, "TimeoutError")),
    timeoutMs,
  );
  return settleAgainstAbort(
    controller.signal,
    operation(controller.signal),
  ).finally(() => window.clearTimeout(timeout));
}

async function assertAccountPublicationAllowed(
  vault: HostedConnectionVault,
  accountId: string,
): Promise<void> {
  if (!vault.assertPublicationAllowed) return;
  try {
    await withRuntimeDeadline(
      vault.assertPublicationAllowed(accountId),
      4_000,
      "Portico could not verify the browser account authorization fence in time.",
    );
  } catch (reason) {
    if (reason instanceof TrustedServerPublicationBlockedError) throw reason;
    throw new TrustedServerPublicationBlockedError(reason);
  }
}

/** Adds the cross-tab linearization boundary to every durable vault publication. */
export function createPublicationLockedHostedConnectionVault(
  rawVault: HostedConnectionVault,
  currentAccountGeneration: () => number = () => 0,
): HostedConnectionVault {
  const guarded = protectHostedConnectionVault(rawVault);
  const publish = <T,>(accountId: string, operation: () => Promise<T>) => {
    const generation = currentAccountGeneration();
    // Start the durable-fence read before queueing. If cleanup is already in
    // progress, its barrier remains authoritative even when the publication
    // waits until the cleanup lock has released.
    const preflight = assertAccountPublicationAllowed(guarded, accountId);
    void preflight.catch(() => undefined);
    return withAccountPublicationLocks(
      [GLOBAL_SIGN_OUT_FENCE_ID, accountId],
      async () => {
        await preflight;
        if (generation !== currentAccountGeneration()) {
          throw new TrustedServerPublicationBlockedError(
            new Error(
              "The Portico Account changed before browser credential publication.",
            ),
          );
        }
        return operation();
      },
    );
  };
  return {
    ...guarded,
    // A write that began before a tombstone must settle and pass its post-write
    // guard before cleanup may verify absence and release that tombstone.
    save: (record: TrustedServerConnectionRecord) =>
      publish(record.accountId, () => guarded.save(record)),
    rememberAccount: (account) =>
      publish(account.accountId, () => guarded.rememberAccount(account)),
    removeWithTombstone: (tombstone) =>
      publish(tombstone.accountId, () =>
        guarded.removeWithTombstone(tombstone),
      ),
  };
}

function abortError(
  message = "The server or profile selection was replaced by a newer choice.",
) {
  return new DOMException(message, "AbortError");
}

function isCleanSelectionAbort(reason: unknown): boolean {
  if (
    reason instanceof TrustedServerCandidateActivationError &&
    !(reason.cause instanceof AggregateError)
  ) {
    return isCleanSelectionAbort(reason.cause);
  }
  return (
    (reason instanceof DOMException && reason.name === "AbortError") ||
    (reason instanceof Error &&
      reason.name === "AbortError" &&
      !(reason instanceof AggregateError))
  );
}

function securityCriticalConnectionFailure(
  reason: unknown,
  seen = new Set<unknown>(),
): boolean {
  if (!reason || seen.has(reason)) return false;
  seen.add(reason);
  if (
    reason instanceof CredentialCleanupUncertainError ||
    reason instanceof TrustedServerDurabilityUncertainError ||
    reason instanceof TrustedServerPublicationBlockedError ||
    reason instanceof SignedOutAccountRestoreBlockedError ||
    reason instanceof ViewerRuntimeTeardownError
  )
    return true;
  if (reason instanceof TrustedServerCredentialPublicationError)
    return reason.failClosed || reason.rollbackFailures.length > 0;
  if (reason instanceof TrustedServerCandidateActivationError) {
    // Core uses an AggregateError cause when publication and/or compensating
    // rollback could not both be proven safe. An ordinary vault AggregateError
    // outside this activation wrapper remains a recoverable durability miss.
    return (
      reason.cause instanceof AggregateError ||
      securityCriticalConnectionFailure(reason.cause, seen)
    );
  }
  if (typeof reason !== "object") return false;
  const candidate = reason as {
    cause?: unknown;
    failClosed?: unknown;
    name?: unknown;
    rollbackFailures?: unknown;
  };
  if (
    candidate.name === "CredentialCleanupUncertainError" ||
    candidate.name === "TrustedServerDurabilityUncertainError" ||
    candidate.name === "TrustedServerPublicationBlockedError" ||
    candidate.name === "SignedOutAccountRestoreBlockedError" ||
    candidate.name === "ViewerRuntimeTeardownError"
  )
    return true;
  if (candidate.name === "TrustedServerCredentialPublicationError") {
    return (
      candidate.failClosed === true ||
      (Array.isArray(candidate.rollbackFailures) &&
        candidate.rollbackFailures.length > 0)
    );
  }
  if (
    candidate.name === "TrustedServerCandidateActivationError" &&
    candidate.cause instanceof AggregateError
  )
    return true;
  return (
    candidate.cause !== reason &&
    securityCriticalConnectionFailure(candidate.cause, seen)
  );
}

function throwIfSelectionStale(
  generation: number,
  currentGeneration: number,
  signal: AbortSignal,
): void {
  if (signal.aborted || generation !== currentGeneration)
    throw signal.reason instanceof Error ? signal.reason : abortError();
}

async function withAbortableDeadline<T>(
  parentSignal: AbortSignal,
  timeoutMs: number,
  message: string,
  operation: (signal: AbortSignal) => Promise<T>,
  mustSettleAfterAbort: () => boolean = () => false,
): Promise<T> {
  const controller = new AbortController();
  const abortForParent = () => controller.abort(parentSignal.reason);
  if (parentSignal.aborted) abortForParent();
  else parentSignal.addEventListener("abort", abortForParent, { once: true });
  const timeout = window.setTimeout(
    () => controller.abort(new TypeError(message)),
    timeoutMs,
  );
  try {
    if (controller.signal.aborted)
      throw controller.signal.reason instanceof Error
        ? controller.signal.reason
        : abortError();
    return await settleAgainstAbort(
      controller.signal,
      operation(controller.signal),
      mustSettleAfterAbort,
    );
  } finally {
    window.clearTimeout(timeout);
    parentSignal.removeEventListener("abort", abortForParent);
  }
}

/** Rejects immediately on cancellation while still consuming a producer's late settlement. */
function settleAgainstAbort<T>(
  signal: AbortSignal,
  producer: Promise<T>,
  mustSettleAfterAbort: () => boolean = () => false,
): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    let settled = false;
    const finish = (
      callback: (value: T | unknown) => void,
      value: T | unknown,
    ) => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", aborted);
      callback(value);
    };
    const aborted = () => {
      // Verification producers are isolated and can be abandoned when they
      // ignore abort. Once stageCandidate begins fencing shared runtime state,
      // however, the Core transaction must reach a proven commit or rollback
      // before a newer choice may touch global credentials or runtime state.
      if (!mustSettleAfterAbort())
        finish(
          reject,
          signal.reason instanceof Error ? signal.reason : abortError(),
        );
    };
    if (signal.aborted) aborted();
    else signal.addEventListener("abort", aborted, { once: true });
    producer.then(
      (value) => finish(resolve as (value: T | unknown) => void, value),
      (reason) => finish(reject, reason),
    );
  });
}

function serverSummary(server: HostedServer): HostedServerSummary {
  return {
    id: server.id,
    name: server.name,
    assignedHostname: server.assignedHostname,
    remoteAccessEnabled: server.remoteAccessEnabled,
    preferredAuthMode: server.preferredAuthMode,
    lastHeartbeatAt: server.lastHeartbeatAt,
  };
}

function viewerFromAuth(
  auth: AuthMeResponse,
  fallbackServerName: string,
): Viewer {
  const role = auth.user?.role === "owner" ? "owner" : "user";
  const viewerScope = auth.authenticated
    ? viewerScopeFromAuthMe(auth)
    : undefined;
  return {
    authenticated: auth.authenticated,
    setupRequired: auth.setupRequired,
    serverName: auth.serverFriendlyName || fallbackServerName,
    viewerScope,
    user: auth.user
      ? {
          id: auth.user.id,
          displayName: auth.user.displayName,
          email: auth.user.email,
          role,
          profileImageUrl: auth.user.profileImageUrl,
          authOrigin: auth.user.authOrigin,
          authProvider: auth.authProvider ?? auth.user.authProvider,
          hasLocalPassword: auth.user.hasLocalPassword,
        }
      : undefined,
    authCapabilities: {
      setupRequired: auth.setupRequired,
      localCredentialsEnabled: false,
      porticoAccountAuthEnabled: true,
      serverFriendlyName: auth.serverFriendlyName || fallbackServerName,
      publicUserPickerEnabled: false,
      visibleUsers: [],
    },
  };
}

function serverFromSummary(
  server: HostedServerSummary,
  candidates: HostedServer[],
): HostedServer | undefined {
  const match = candidates.find((candidate) => candidate.id === server.id);
  return match;
}

function summaryFromTrustedRecord(
  record: TrustedServerConnectionRecord,
): HostedServerSummary {
  return {
    id: record.serverId,
    name: record.serverName,
    assignedHostname: "",
    remoteAccessEnabled: true,
    preferredAuthMode: "portico",
  };
}

function mergeServerSummaries(
  live: HostedServer[],
  remembered: TrustedServerConnectionRecord[],
): HostedServerSummary[] {
  const summaries = new Map(
    remembered.map((record) => [
      record.serverId,
      summaryFromTrustedRecord(record),
    ]),
  );
  live.forEach((server) => summaries.set(server.id, serverSummary(server)));
  return [...summaries.values()];
}

export async function browserSafeProbeFetch(
  input: string | URL | Request,
  init?: RequestInit,
) {
  const value = input instanceof Request ? input.url : String(input);
  const url = new URL(value);
  if (url.protocol !== "https:")
    throw new TypeError("The direct route is not available over secure HTTPS.");
  return fetch(input, init);
}

export function browserSafeLocalCandidates(
  _server: HostedServer,
  document: HostedRouteDocument,
): HostedRouteEntry[] {
  return document.routes.filter((route) => {
    if (
      ![
        "lan",
        "lan_ip_encoded",
        "lan_discovered",
        "direct_ip_encoded",
        "public_direct_ip_encoded",
      ].includes(route.type)
    )
      return false;
    if (
      [
        "stale",
        "failed",
        "http_failed",
        "tls_failed",
        "identity_mismatch",
        "repairing",
        "repair_requested",
      ].includes(route.quality)
    )
      return false;
    try {
      const candidate = new URL(route.url);
      const hostname = candidate.hostname.replace(/^\[|\]$/g, "");
      const ipLiteral =
        /^\d{1,3}(?:\.\d{1,3}){3}$/.test(hostname) || hostname.includes(":");
      return candidate.protocol === "https:" && !ipLiteral;
    } catch {
      return false;
    }
  });
}

export function extractHostedBootstrapIntent(value: string): {
  intent: HostedBootstrapIntent;
  safeUrl: string;
} {
  const url = new URL(value, "https://web.getportico.tv");
  const ssoOnboardingToken =
    url.pathname === "/auth/sso/onboarding"
      ? url.searchParams.get("token")?.trim()
      : undefined;
	const queryClaimCode =
		url.pathname === "/claim" ? url.searchParams.get("code") : null;
  const rawClaimServerName = queryClaimCode
    ? url.searchParams.get("serverName")?.trim().replace(/\s+/g, " ")
    : undefined;
  const claimServerName =
    rawClaimServerName && Array.from(rawClaimServerName).length <= 120
      ? rawClaimServerName
      : undefined;
  const rawClaimReturnUrl = queryClaimCode
    ? url.searchParams.get("returnUrl")?.trim()
    : undefined;
  const claimReturnUrl = validServerSetupReturnUrl(rawClaimReturnUrl)
    ? rawClaimReturnUrl
    : undefined;
  const intent: HostedBootstrapIntent = {
		inviteId: url.pathname.match(/^\/invites\/([^/]+)$/)?.[1],
		claimCode: queryClaimCode ?? undefined,
    ...(claimServerName ? { claimServerName } : {}),
    ...(claimReturnUrl ? { claimReturnUrl } : {}),
		resetToken: url.pathname.match(/^\/account\/password-reset\/([^/]+)$/)?.[1],
    ...(ssoOnboardingToken ? { ssoOnboardingToken } : {}),
  };
  const fragment = url.hash.startsWith("#") ? url.hash.slice(1) : url.hash;
  if (fragment) {
    if (url.pathname === "/device" || url.pathname === "/authorize-device") {
      const rawCode =
        new URLSearchParams(fragment).get("code")?.trim().toUpperCase() ?? "";
      const compactCode = rawCode.replace(/-/g, "");
      if (/^[A-HJ-KM-NP-Z2-9]{8}$/.test(compactCode)) {
        const formatted = `${compactCode.slice(0, 4)}-${compactCode.slice(4)}`;
        if (url.pathname === "/device")
          intent.deviceAuthorizationCode = formatted;
        else intent.genericDeviceAuthorizationCode = formatted;
      }
      if (url.pathname === "/authorize-device") {
        const provider = new URLSearchParams(fragment).get("provider");
        if (provider === "google" || provider === "apple")
          intent.genericDeviceAuthorizationProvider = provider;
        if (new URLSearchParams(fragment).get("nativeReturn") === "1")
          intent.genericDeviceAuthorizationNativeReturn = true;
      }
      url.hash = "";
    }
    const handoff = new URL(
      fragment.startsWith("/") ? fragment : `/${fragment}`,
      url.origin,
    );
    if (handoff.pathname === "/local-login") {
      const serverId = handoff.searchParams.get("serverId")?.trim() ?? "";
      const callbackUrl = handoff.searchParams.get("callbackUrl")?.trim() ?? "";
      const localOrigin = handoff.searchParams.get("localOrigin")?.trim() ?? "";
      const state = handoff.searchParams.get("state")?.trim() ?? "";
      const serverPublicKeyFingerprint =
        handoff.searchParams.get("serverPublicKeyFingerprint")?.trim() ?? "";
      const rawOriginGeneration =
        handoff.searchParams.get("publicConsoleOriginGeneration")?.trim() ?? "";
      const publicConsoleOriginGeneration = Number(rawOriginGeneration);
      if (
        serverId &&
        callbackUrl &&
        localOrigin &&
        state &&
        serverPublicKeyFingerprint &&
        /^[1-9][0-9]*$/.test(rawOriginGeneration) &&
        Number.isSafeInteger(publicConsoleOriginGeneration)
      ) {
        intent.localLogin = {
          serverId,
          serverName:
            handoff.searchParams.get("serverName")?.trim() || "this server",
          callbackUrl,
          localOrigin,
          state,
          serverPublicKeyFingerprint,
          publicConsoleOriginGeneration,
          ...(handoff.searchParams.get("installationId")?.trim()
            ? {
                installationId: handoff.searchParams
                  .get("installationId")!
                  .trim(),
              }
            : {}),
        };
      }
      url.hash = "";
    }
  }
  if (url.pathname === "/device") intent.deviceAuthorizationRequested = true;
  if (url.pathname === "/authorize-device")
    intent.genericDeviceAuthorizationRequested = true;
  if (ssoOnboardingToken) url.searchParams.delete("token");
  url.searchParams.delete("localLoginResume");
  if (intent.claimCode && url.pathname === "/claim") {
    url.searchParams.delete("code");
    url.searchParams.delete("serverName");
    url.searchParams.delete("returnUrl");
  }
  if (
    intent.inviteId ||
    intent.claimCode ||
    intent.resetToken ||
    intent.ssoOnboardingToken
  )
    url.pathname = "/";
  return { intent, safeUrl: `${url.pathname}${url.search}${url.hash}` };
}

function saveRecoverableDeviceAuthorization(code: string): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.setItem(
      DEVICE_AUTHORIZATION_HANDOFF_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        expiresAt: Date.now() + DEVICE_AUTHORIZATION_HANDOFF_RECOVERY_TTL_MS,
        code,
      }),
    );
  } catch {
    /* Same-tab SSO recovery is best effort. */
  }
}

function loadRecoverableDeviceAuthorization(): string | undefined {
  if (typeof window === "undefined") return undefined;
  try {
    const record = JSON.parse(
      window.sessionStorage.getItem(DEVICE_AUTHORIZATION_HANDOFF_STORAGE_KEY) ??
        "null",
    ) as { version?: unknown; expiresAt?: unknown; code?: unknown } | null;
    if (
      !record ||
      record.version !== 1 ||
      typeof record.expiresAt !== "number" ||
      record.expiresAt <= Date.now() ||
      typeof record.code !== "string"
    ) {
      window.sessionStorage.removeItem(
        DEVICE_AUTHORIZATION_HANDOFF_STORAGE_KEY,
      );
      return undefined;
    }
    return record.code;
  } catch {
    try {
      window.sessionStorage.removeItem(
        DEVICE_AUTHORIZATION_HANDOFF_STORAGE_KEY,
      );
    } catch {
      /* Storage remains unavailable. */
    }
    return undefined;
  }
}

function clearRecoverableDeviceAuthorization(): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.removeItem(DEVICE_AUTHORIZATION_HANDOFF_STORAGE_KEY);
  } catch {
    /* Storage remains unavailable. */
  }
}

function recoverGenericDeviceAuthorization(
  intent: HostedBootstrapIntent,
): void {
  if (
    typeof window === "undefined" ||
    !intent.genericDeviceAuthorizationRequested
  )
    return;
  try {
    if (intent.genericDeviceAuthorizationCode) {
      window.sessionStorage.setItem(
        GENERIC_DEVICE_AUTHORIZATION_HANDOFF_STORAGE_KEY,
        JSON.stringify({
          version: 1,
          expiresAt: Date.now() + DEVICE_AUTHORIZATION_HANDOFF_RECOVERY_TTL_MS,
          code: intent.genericDeviceAuthorizationCode,
          provider: intent.genericDeviceAuthorizationProvider,
          nativeReturn: intent.genericDeviceAuthorizationNativeReturn === true,
        }),
      );
      return;
    }
    const record = JSON.parse(
      window.sessionStorage.getItem(
        GENERIC_DEVICE_AUTHORIZATION_HANDOFF_STORAGE_KEY,
      ) ?? "null",
    ) as {
      version?: unknown;
      expiresAt?: unknown;
      code?: unknown;
      provider?: unknown;
      nativeReturn?: unknown;
    } | null;
    if (
      record?.version === 1 &&
      typeof record.expiresAt === "number" &&
      record.expiresAt > Date.now() &&
      typeof record.code === "string"
    ) {
      intent.genericDeviceAuthorizationCode = record.code;
      if (record.provider === "google" || record.provider === "apple")
        intent.genericDeviceAuthorizationProvider = record.provider;
      if (record.nativeReturn === true)
        intent.genericDeviceAuthorizationNativeReturn = true;
    } else
      window.sessionStorage.removeItem(
        GENERIC_DEVICE_AUTHORIZATION_HANDOFF_STORAGE_KEY,
      );
  } catch {
    /* Same-tab SSO recovery is best effort. */
  }
}

function clearRecoverableGenericDeviceAuthorization(): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.removeItem(
      GENERIC_DEVICE_AUTHORIZATION_HANDOFF_STORAGE_KEY,
    );
  } catch {
    /* Storage remains unavailable. */
  }
}

export function extractPorticoLoginResult(value: string): {
  result?: "success" | "error";
  messageId?: ProductMessageId;
  safeUrl: string;
} {
  const url = new URL(value, "http://localhost");
  const raw = url.searchParams.get("porticoLogin");
  const result = raw === "success" || raw === "error" ? raw : undefined;
  const messageId = knownProductMessageId(
    url.searchParams.get("porticoLoginMessageId") ?? undefined,
  );
  // The callback detail is deliberately not retained or rendered. Product
  // Language supplies stable client copy, while the URL is made safe for
  // reload, sharing, browser history, and diagnostics immediately.
  url.searchParams.delete("porticoLogin");
  url.searchParams.delete("porticoLoginMessageId");
  return {
    result,
    ...(messageId ? { messageId } : {}),
    safeUrl: `${url.pathname}${url.search}${url.hash}`,
  };
}

function isLocalLoginFragment(value: string): boolean {
  try {
    const url = new URL(value, "https://web.getportico.tv");
    const fragment = url.hash.startsWith("#") ? url.hash.slice(1) : url.hash;
    if (!fragment) return false;
    return (
      new URL(fragment.startsWith("/") ? fragment : `/${fragment}`, url.origin)
        .pathname === "/local-login"
    );
  } catch {
    return false;
  }
}

function validRecoverableLocalLoginIntent(
  intent: HostedLocalLoginIntent,
): boolean {
  if (
    !intent.serverId.trim() ||
    intent.serverId.length > 512 ||
    !intent.serverName.trim() ||
    intent.serverName.length > 200 ||
    !intent.state.trim() ||
    intent.state.length > 4096 ||
    !intent.serverPublicKeyFingerprint.trim() ||
    intent.serverPublicKeyFingerprint.length > 1024 ||
    (intent.installationId?.length ?? 0) > 512
    || !Number.isSafeInteger(intent.publicConsoleOriginGeneration)
    || intent.publicConsoleOriginGeneration <= 0
  )
    return false;
  try {
    const callback = new URL(intent.callbackUrl);
    const localOrigin = new URL(intent.localOrigin);
    const permittedProtocol =
      callback.protocol === "https:" ||
      (callback.protocol === "http:" && localHTTPHostname(callback.hostname));
    return (
      permittedProtocol &&
      callback.origin === localOrigin.origin &&
      callback.pathname === "/api/auth/portico/callback"
    );
  } catch {
    return false;
  }
}

function clearRecoverableLocalLoginIntent(): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.removeItem(LOCAL_LOGIN_HANDOFF_STORAGE_KEY);
  } catch {
    /* Recovery storage is optional. */
  }
}

function saveRecoverableLocalLoginIntent(intent: HostedLocalLoginIntent): void {
  if (
    typeof window === "undefined" ||
    !validRecoverableLocalLoginIntent(intent)
  )
    return;
  const record: StoredLocalLoginHandoff = {
    version: 1,
    expiresAt: Date.now() + LOCAL_LOGIN_HANDOFF_RECOVERY_TTL_MS,
    intent,
  };
  try {
    window.sessionStorage.setItem(
      LOCAL_LOGIN_HANDOFF_STORAGE_KEY,
      JSON.stringify(record),
    );
  } catch {
    /* Continue without reload recovery. */
  }
}

function loadRecoverableLocalLoginIntent(): { intent?: HostedLocalLoginIntent; failure?: "expired" | "unavailable" | "lost" } {
  if (typeof window === "undefined") return { failure: "lost" };
  try {
    const raw = window.sessionStorage.getItem(LOCAL_LOGIN_HANDOFF_STORAGE_KEY);
    if (!raw) return { failure: "lost" };
    const record = JSON.parse(raw) as Partial<StoredLocalLoginHandoff>;
    if (
      record.version !== 1 ||
      typeof record.expiresAt !== "number" ||
      record.expiresAt <= Date.now() ||
      !record.intent ||
      !validRecoverableLocalLoginIntent(record.intent)
    ) {
      clearRecoverableLocalLoginIntent();
      return { failure: record.expiresAt !== undefined && typeof record.expiresAt === "number" && record.expiresAt <= Date.now() ? "expired" : "lost" };
    }
    return { intent: record.intent };
  } catch {
    clearRecoverableLocalLoginIntent();
    return { failure: "unavailable" };
  }
}

function clearRecoverableServerClaim(): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.removeItem(SERVER_CLAIM_HANDOFF_STORAGE_KEY);
  } catch {
    /* Recovery storage is optional. */
  }
}

function clearRecoverableSSOOnboarding(): void {
  if (typeof window === "undefined") return;
  try {
    window.sessionStorage.removeItem(SSO_ONBOARDING_HANDOFF_STORAGE_KEY);
  } catch {
    /* Recovery storage is optional. */
  }
}

function saveRecoverableSSOOnboarding(token: string): void {
  if (typeof window === "undefined" || !token.trim() || token.length > 256)
    return;
  try {
    window.sessionStorage.setItem(
      SSO_ONBOARDING_HANDOFF_STORAGE_KEY,
      JSON.stringify({
        version: 1,
        expiresAt: Date.now() + SSO_ONBOARDING_HANDOFF_RECOVERY_TTL_MS,
        token,
      }),
    );
  } catch {
    /* Continue without reload recovery. */
  }
}

function loadRecoverableSSOOnboarding(): string | undefined {
  if (typeof window === "undefined") return undefined;
  try {
    const raw = window.sessionStorage.getItem(
      SSO_ONBOARDING_HANDOFF_STORAGE_KEY,
    );
    if (!raw) return undefined;
    const record = JSON.parse(raw) as {
      version?: unknown;
      expiresAt?: unknown;
      token?: unknown;
    };
    if (
      record.version !== 1 ||
      typeof record.expiresAt !== "number" ||
      record.expiresAt <= Date.now() ||
      typeof record.token !== "string" ||
      !record.token.trim() ||
      record.token.length > 256
    ) {
      clearRecoverableSSOOnboarding();
      return undefined;
    }
    return record.token;
  } catch {
    clearRecoverableSSOOnboarding();
    return undefined;
  }
}

function saveRecoverableServerClaim(intent: HostedBootstrapIntent): void {
  if (typeof window === "undefined" || !intent.claimCode?.trim()) return;
  const record: StoredServerClaimHandoff = {
    version: 1,
    expiresAt: Date.now() + SERVER_CLAIM_HANDOFF_RECOVERY_TTL_MS,
    claimCode: intent.claimCode,
    ...(intent.claimServerName ? { serverName: intent.claimServerName } : {}),
    ...(validServerSetupReturnUrl(intent.claimReturnUrl)
      ? { returnUrl: intent.claimReturnUrl }
      : {}),
  };
  try {
    window.sessionStorage.setItem(
      SERVER_CLAIM_HANDOFF_STORAGE_KEY,
      JSON.stringify(record),
    );
  } catch {
    /* Continue without reload recovery. */
  }
}

function loadRecoverableServerClaim():
  | Pick<
      HostedBootstrapIntent,
      "claimCode" | "claimServerName" | "claimReturnUrl"
    >
  | undefined {
  if (typeof window === "undefined") return undefined;
  try {
    const raw = window.sessionStorage.getItem(SERVER_CLAIM_HANDOFF_STORAGE_KEY);
    if (!raw) return undefined;
    const record = JSON.parse(raw) as Partial<StoredServerClaimHandoff>;
    if (
      record.version !== 1 ||
      typeof record.expiresAt !== "number" ||
      record.expiresAt <= Date.now() ||
      typeof record.claimCode !== "string" ||
      !record.claimCode.trim()
    ) {
      clearRecoverableServerClaim();
      return undefined;
    }
    return {
      claimCode: record.claimCode,
      ...(typeof record.serverName === "string" && record.serverName.trim()
        ? { claimServerName: record.serverName }
        : {}),
      ...(validServerSetupReturnUrl(record.returnUrl)
        ? { claimReturnUrl: record.returnUrl }
        : {}),
    };
  } catch {
    clearRecoverableServerClaim();
    return undefined;
  }
}

function recoverHostedBootstrapIntent(value: string): {
  intent: HostedBootstrapIntent;
  safeUrl: string;
} {
  const extracted = extractHostedBootstrapIntent(value);
  const inputURL = new URL(value, "https://web.getportico.tv");
  if (extracted.intent.ssoOnboardingToken)
    saveRecoverableSSOOnboarding(extracted.intent.ssoOnboardingToken);
  else if (inputURL.pathname === "/auth/sso/onboarding")
    clearRecoverableSSOOnboarding();
  else if (inputURL.pathname === "/")
    extracted.intent.ssoOnboardingToken = loadRecoverableSSOOnboarding();
  if (extracted.intent.deviceAuthorizationRequested) {
    if (extracted.intent.deviceAuthorizationCode)
      saveRecoverableDeviceAuthorization(
        extracted.intent.deviceAuthorizationCode,
      );
    else
      extracted.intent.deviceAuthorizationCode =
        loadRecoverableDeviceAuthorization();
  }
  recoverGenericDeviceAuthorization(extracted.intent);
  if (extracted.intent.claimCode) saveRecoverableServerClaim(extracted.intent);
  else if (inputURL.pathname === "/claim") clearRecoverableServerClaim();
  else Object.assign(extracted.intent, loadRecoverableServerClaim());
  if (extracted.intent.localLogin) {
    if (validRecoverableLocalLoginIntent(extracted.intent.localLogin))
      saveRecoverableLocalLoginIntent(extracted.intent.localLogin);
    else {
      clearRecoverableLocalLoginIntent();
      delete extracted.intent.localLogin;
    }
    return extracted;
  }
  // A malformed new handoff must never resurrect an older request. Normal
  // reloads have no fragment and may resume the short-lived same-tab intent.
  if (isLocalLoginFragment(value)) {
    clearRecoverableLocalLoginIntent();
    return extracted;
  }
  const recovered = loadRecoverableLocalLoginIntent();
  if (recovered.intent) extracted.intent.localLogin = recovered.intent;
  else if (inputURL.searchParams.get("localLoginResume") === "1")
    extracted.intent.localLoginRecoveryFailure = recovered.failure ?? "lost";
  return extracted;
}

export function verifiedLocalLoginRedirect(
  intent: HostedLocalLoginIntent,
  value: unknown,
): string {
  if (typeof value !== "string" || !value.trim())
    throw new Error("Portico did not return a local server sign-in address.");
  let callback: URL;
  let localOrigin: URL;
  let redirect: URL;
  try {
    callback = new URL(intent.callbackUrl);
    localOrigin = new URL(intent.localOrigin);
    redirect = new URL(value);
  } catch {
    throw new Error("The local server sign-in address is invalid.");
  }
  const permittedProtocol =
    callback.protocol === "https:" ||
    (callback.protocol === "http:" && localHTTPHostname(callback.hostname));
  if (!permittedProtocol || callback.origin !== localOrigin.origin)
    throw new Error("The local server sign-in request is not secure.");
  if (
    redirect.origin !== callback.origin ||
    redirect.pathname !== callback.pathname
  )
    throw new Error(
      "Portico returned the sign-in result to an unexpected server address.",
    );
  if (
    !redirect.searchParams.get("code") ||
    redirect.searchParams.get("state") !== intent.state
  )
    throw new Error("The local server sign-in result could not be verified.");
  return redirect.toString();
}

export function RuntimeProvider({
  children,
  config: configured,
  environment,
  fixtureSourceLoader,
  hostedConnectionVault,
  navigate = (target) => window.location.assign(target),
}: {
  children: ReactNode;
  config?: RuntimeConfig;
  environment?: Record<string, string | boolean | undefined>;
  fixtureSourceLoader?: () => Promise<PorticoDataSource>;
  hostedConnectionVault?: HostedConnectionVault;
  navigate?: (target: string) => void;
}) {
  const resolution = useMemo(() => {
    try {
      const baseEnvironment =
        environment ??
        (import.meta.env as Record<string, string | boolean | undefined>);
      const bootstrap =
        environment == null && typeof window !== "undefined"
          ? window.__PORTICO_CONFIG__
          : undefined;
      return {
        config:
          configured ??
          resolveRuntimeConfig(
            mergeRuntimeEnvironment(baseEnvironment, bootstrap),
          ),
      };
    } catch (reason) {
      return {
        error:
          reason instanceof Error
            ? reason
            : new Error("Portico runtime configuration is invalid."),
      };
    }
  }, [configured, environment]);
  const config = resolution.config ?? {
    mode: "bundled" as const,
    hostedApiBaseUrl: "https://api.getportico.tv",
    routeProbeTimeoutMs: 3500,
    buildId: "configuration-error",
  };
  const [state, dispatch] = useReducer(runtimeReducer, undefined, () =>
    initialRuntimeState(),
  );
  const [source, setSource] = useState<PorticoDataSource>();
  const [initialViewer, setInitialViewer] = useState<Viewer>();
  const [restoredPresentation, setRestoredPresentation] = useState<{
    accountId: string;
    displayName: string;
  }>();
  const [expectedViewerScope, setExpectedViewerScope] = useState<ViewerScope>();
  const [busy, setBusy] = useState(false);
  const [accountSettingsOpen, setAccountSettingsOpen] = useState(false);
  const [mfaRequired, setMfaRequired] = useState(false);
  const [revision, setRevision] = useState(0);
  const [hostedServers, setHostedServers] = useState<HostedServer[]>([]);
  const [trustedConnections, setTrustedConnections] = useState<
    TrustedServerConnectionRecord[]
  >([]);
  const [connectionWarning, setConnectionWarning] =
    useState<ProductMessageId>();
  const viewerRuntime = useMemo(() => new WebViewerRuntime(), []);
  const selectionGeneration = useRef(0);
  const activeSelection = useRef<ActiveSelection | undefined>(undefined);
  const selectionSecurityFailure = useRef<Error | undefined>(undefined);
  const hostedAccountGeneration = useRef(0);
  const activeHostedAccount = useRef<HostedAccountSnapshot | undefined>(
    undefined,
  );
  const activeServerConnection = useRef<
    { accountId: string; serverId: string } | undefined
  >(undefined);
  const [publishedServerIdentity, setPublishedServerIdentity] = useState<
    { accountId: string; serverId: string } | undefined
  >(undefined);
  const remoteUnclaimedConnections = useRef(new Set<string>());
  const membershipRefresh = useRef<(() => Promise<void>) | undefined>(
    undefined,
  );
  const membershipRefreshInFlight = useRef<Promise<void> | undefined>(
    undefined,
  );
  const ssoOnboardingCompletionInFlight = useRef<Promise<void> | undefined>(
    undefined,
  );
  const initialBootstrapIntent = useMemo(
    () =>
      config.mode === "hosted" && typeof window !== "undefined"
        ? recoverHostedBootstrapIntent(window.location.href)
        : { intent: {}, safeUrl: "" },
    [config.mode],
  );
  const initialLocalLoginResult = useMemo(
    () =>
      typeof window !== "undefined"
        ? extractPorticoLoginResult(window.location.href)
        : { safeUrl: "" },
    [],
  );
  const bootstrapIntent = useRef<HostedBootstrapIntent>(
    initialBootstrapIntent.intent,
  );
  const sessionStore = useMemo(() => createMemorySessionStore(), []);
  const activeRouteRecovery = useRef<(() => Promise<void>) | undefined>(
    undefined,
  );
  const routeRecoveryInFlight = useRef<Promise<void> | undefined>(undefined);
  const routePublicationRetryAttempt = useRef<
    { key: string; attempt: number; startedAt: number } | undefined
  >(undefined);
  const rawConnectionVault = useMemo(
    () => hostedConnectionVault ?? createBrowserHostedConnectionVault(),
    [hostedConnectionVault],
  );
  const connectionVault = useMemo(
    () =>
      createPublicationLockedHostedConnectionVault(
        rawConnectionVault,
        () => hostedAccountGeneration.current,
      ),
    [rawConnectionVault],
  );
  const [hostedRetryCohort, setHostedRetryCohort] = useState(
    createHostedAvailabilityRetryCohort,
  );
  const hostedRetryCohortRef = useRef(hostedRetryCohort);
  const hostedAvailabilityFailureStartedAtRef = useRef<number | undefined>(
    undefined,
  );
  if (
    state.id === "runtime-recovery" &&
    state.automaticAvailabilityRetry === true
  ) {
    hostedAvailabilityFailureStartedAtRef.current ??= state.startedAt;
  } else if (
    [
      "hosted-sign-in",
      "sso-onboarding",
      "device-authorization",
      "no-memberships",
      "server-selection",
      "profile-selection",
      "server-ready",
    ].includes(state.id)
  ) {
    hostedAvailabilityFailureStartedAtRef.current = undefined;
  }
  useEffect(() => {
    let active = true;
    void connectionVault.installationId().then((value) => {
      if (active && value) {
        hostedRetryCohortRef.current = value;
        setHostedRetryCohort(value);
      }
    }).catch(() => undefined);
    return () => {
      active = false;
    };
  }, [connectionVault]);
  const hostedClient = useMemo(
    () =>
      createHostedServicesClient({
        hostedApiBaseUrl: config.hostedApiBaseUrl,
        csrfToken: hostedCSRFToken,
        onCSRFToken: rememberHostedCSRFToken,
        // Runtime recovery already owns a visible, cohort-spread retry loop.
        // Retrying invisibly inside each bootstrap request both multiplies
        // Hosted load and delays the first useful offline/direct decision.
        retryBudget: 0,
        retryCohort: () => hostedRetryCohortRef.current,
        terminalMutationDurabilityAdapter:
          browserHostedTerminalMutationDurability,
      }),
    [config.hostedApiBaseUrl],
  );
  useEffect(() => {
    if (config.mode !== "hosted") return;
    // This is a read-only reconciliation pass. Missing or temporarily
    // unavailable receipts remain in durable storage for the next launch.
    void hostedClient.reconcilePendingAccountTerminalMutations().catch(
      () => undefined,
    );
  }, [config.mode, hostedClient]);
  const routeAwareTransport = useMemo(
    () => ({
      fetch: async (input: RequestInfo | URL, init?: RequestInit) => {
        const method = String(
          init?.method ?? (input instanceof Request ? input.method : "GET"),
        ).toUpperCase();
        let response: Response | undefined;
        let initialFailure: unknown;
        try {
          response = await fetch(input, init);
        } catch (reason) {
          initialFailure = reason;
        }
        const signal =
          init?.signal ?? (input instanceof Request ? input.signal : undefined);
        const recover = activeRouteRecovery.current;
        const routeStatus =
          response && [421, 502, 503, 504].includes(response.status);
        if (
          (method !== "GET" && method !== "HEAD") ||
          signal?.aborted ||
          !recover ||
          !activeServerConnection.current ||
          (!initialFailure && !routeStatus)
        ) {
          if (initialFailure) throw initialFailure;
          return response as Response;
        }
        try {
          let recovery = routeRecoveryInFlight.current;
          if (!recovery) {
            recovery = recover().finally(() => {
              if (routeRecoveryInFlight.current === recovery)
                routeRecoveryInFlight.current = undefined;
            });
            routeRecoveryInFlight.current = recovery;
          }
          await recovery;
          if (signal?.aborted)
            throw (
              signal.reason ??
              initialFailure ??
              new Error("The request was cancelled.")
            );
          const nextBase = sessionStore.get()?.apiBaseUrl;
          if (!nextBase) {
            if (initialFailure) throw initialFailure;
            return response as Response;
          }
          const original = new URL(
            input instanceof Request ? input.url : String(input),
          );
          const rebased = new URL(
            `${original.pathname}${original.search}${original.hash}`,
            `${nextBase.replace(/\/+$/, "")}/`,
          ).toString();
          const retryInput =
            input instanceof Request ? new Request(rebased, input) : rebased;
          return fetch(retryInput, init);
        } catch (recoveryFailure) {
          if (initialFailure) throw initialFailure;
          if (response) return response;
          throw recoveryFailure;
        }
      },
    }),
    [sessionStore],
  );
  const localClient = useMemo(
    () =>
      createPorticoClient({
        apiBaseUrl: () => sessionStore.get()?.apiBaseUrl ?? "",
        sessionStore,
        transport: routeAwareTransport,
        playbackClientProfile: browserPlaybackClientProfile,
        playbackProgressDurabilityAdapter: browserPlaybackProgressDurability,
        credentialAdapter: {
          load: async () => {
            const selected = activeServerConnection.current;
            return selected
              ? (
                  await connectionVault.load(
                    selected.accountId,
                    selected.serverId,
                  )
                )?.session
              : undefined;
          },
          save: async (session: LocalServerSession) => {
            const selected = activeServerConnection.current;
            if (!selected)
              throw new Error(
                "Portico cannot persist a server session before a server is selected.",
              );
            const record = await connectionVault.load(
              selected.accountId,
              selected.serverId,
            );
            if (!record)
              throw new Error(
                "The trusted server connection is no longer available.",
              );
            await connectionVault.save({ ...record, session });
          },
          clear: async () => {
            const selected = activeServerConnection.current;
            if (selected)
              await connectionVault.remove(
                selected.accountId,
                selected.serverId,
              );
          },
        },
      }),
    [connectionVault, routeAwareTransport, sessionStore],
  );

  const failClosedForExternalAccountFence = useCallback(
    (fencedAccountId: string, reason?: unknown) => {
      const activeAccountId =
        activeHostedAccount.current?.accountId ??
        activeServerConnection.current?.accountId ??
        viewerRuntime.activeScope()?.accountId;
      if (
        fencedAccountId !== GLOBAL_SIGN_OUT_FENCE_ID &&
        activeAccountId !== fencedAccountId
      )
        return;
      const unsafe =
        reason instanceof Error
          ? reason
          : new TrustedServerPublicationBlockedError(
              new Error(
                "This Portico Account was signed out in another browser context.",
              ),
            );
      selectionSecurityFailure.current = unsafe;
      selectionGeneration.current += 1;
      activeSelection.current?.controller.abort(
        abortError(
          "This Portico Account was signed out in another browser context.",
        ),
      );
      hostedAccountGeneration.current += 1;
      viewerRuntime.failClosed();
      try {
        sessionStore.clear?.();
      } catch {
        /* Runtime and UI remain fenced. */
      }
      activeHostedAccount.current = undefined;
      setRestoredPresentation(undefined);
      activeServerConnection.current = undefined;
      setPublishedServerIdentity(undefined);
      setSource(undefined);
      setInitialViewer(undefined);
      setExpectedViewerScope(undefined);
      setHostedServers([]);
      setTrustedConnections([]);
      setBusy(false);
      setConnectionWarning("auth.sign-out-storage-warning");
      dispatch({
        type: "HOSTED_SIGN_IN_REQUIRED",
        messageId: "auth.sign-out-storage-warning",
      });
    },
    [sessionStore, viewerRuntime],
  );

  useEffect(() => {
    if (config.mode !== "hosted") return;
    viewerRuntime.setAuthorizationGate(async (scope, start) => {
      if (scope.authority !== "hosted") return start();
      const started = await withAccountPublicationLocks(
        [GLOBAL_SIGN_OUT_FENCE_ID, scope.accountId],
        async () => {
          try {
            await assertAccountPublicationAllowed(
              connectionVault,
              scope.accountId,
            );
          } catch (reason) {
            failClosedForExternalAccountFence(scope.accountId, reason);
            throw reason;
          }
          // Start the request while holding the account lock, but do not keep the
          // lock across network I/O. Sign-out is now totally ordered before or
          // after the request's side-effect boundary.
          return { producer: start() };
        },
      );
      return started.producer;
    });
    const storageListener = (event: StorageEvent) => {
      const carriedFence = accountFenceFromTombstoneEvent(
        event.key,
        event.newValue,
      );
      if (carriedFence) {
        failClosedForExternalAccountFence(carriedFence);
        return;
      }
      if (
        event.key !== null &&
        !event.key.startsWith(SIGNED_OUT_ACCOUNT_TOMBSTONE_PREFIX)
      )
        return;
      const accountId =
        activeHostedAccount.current?.accountId ??
        activeServerConnection.current?.accountId ??
        viewerRuntime.activeScope()?.accountId;
      if (!accountId) return;
      const status = signedOutAccountRestoreStatus(accountId);
      if (!status.trustedForRestore || status.quarantined)
        failClosedForExternalAccountFence(accountId);
    };
    window.addEventListener("storage", storageListener);
    const unsubscribe = subscribeAccountFences((accountId) =>
      failClosedForExternalAccountFence(accountId),
    );
    return () => {
      viewerRuntime.setAuthorizationGate(undefined);
      window.removeEventListener("storage", storageListener);
      unsubscribe();
    };
  }, [
    config.mode,
    connectionVault,
    failClosedForExternalAccountFence,
    viewerRuntime,
  ]);

  useEffect(() => {
    if (config.mode !== "hosted" || typeof window === "undefined") return;
    if (
      initialBootstrapIntent.safeUrl !==
      `${window.location.pathname}${window.location.search}${window.location.hash}`
    )
      window.history.replaceState(null, "", initialBootstrapIntent.safeUrl);
  }, [config.mode, initialBootstrapIntent.safeUrl]);

  useEffect(() => {
    if (typeof window === "undefined") return;
    if (!initialLocalLoginResult.result) return;
    const current = `${window.location.pathname}${window.location.search}${window.location.hash}`;
    if (
      initialLocalLoginResult.safeUrl &&
      initialLocalLoginResult.safeUrl !== current
    ) {
      window.history.replaceState(null, "", initialLocalLoginResult.safeUrl);
    }
    if (initialLocalLoginResult.result === "error")
      setConnectionWarning(
        initialLocalLoginResult.messageId ?? "auth.sign-in-failed",
      );
  }, [
    initialLocalLoginResult.messageId,
    initialLocalLoginResult.result,
    initialLocalLoginResult.safeUrl,
  ]);

  const becomeReady = (
    nextSource: PorticoDataSource,
    mode: RuntimeConfig["mode"],
    serverName: string,
    viewer: Viewer,
    expectedScope = viewer.viewerScope,
  ) => {
    setSource(nextSource);
    setInitialViewer(viewer);
    setExpectedViewerScope(expectedScope);
    if (expectedScope?.authority === "hosted" && viewer.user?.displayName) {
      setRestoredPresentation({
        accountId: expectedScope.accountId,
        displayName: viewer.user.displayName,
      });
    }
    dispatch({ type: "READY", mode, serverName });
  };

  const createConnectionClient = (store: SessionStore) =>
    createPorticoClient({
      apiBaseUrl: () => store.get()?.apiBaseUrl ?? "",
      sessionStore: store,
      playbackClientProfile: browserPlaybackClientProfile,
      playbackProgressDurabilityAdapter: browserPlaybackProgressDurability,
    });

  const runLatestSelection = <T,>(
    operation: (selection: {
      generation: number;
      signal: AbortSignal;
    }) => Promise<T>,
    outerSignal?: AbortSignal,
  ): Promise<T> => {
    const previous = activeSelection.current;
    const generation = ++selectionGeneration.current;
    previous?.controller.abort(abortError());
    const controller = new AbortController();
    const abortFromCaller = () =>
      controller.abort(
        outerSignal?.reason ??
          abortError("The server selection was cancelled."),
      );
    if (outerSignal?.aborted) abortFromCaller();
    else
      outerSignal?.addEventListener("abort", abortFromCaller, { once: true });
    const run = (async () => {
      if (previous) await previous.settled;
      if (selectionSecurityFailure.current)
        throw selectionSecurityFailure.current;
      throwIfSelectionStale(
        generation,
        selectionGeneration.current,
        controller.signal,
      );
      return operation({ generation, signal: controller.signal });
    })();
    const settled = run.then(
      () => undefined,
      () => undefined,
    );
    const active = { generation, controller, settled };
    activeSelection.current = active;
    void settled.then(() => {
      outerSignal?.removeEventListener("abort", abortFromCaller);
      if (activeSelection.current === active)
        activeSelection.current = undefined;
    });
    return run;
  };

  const cancelActiveSelection = async () => {
    selectionGeneration.current += 1;
    const active = activeSelection.current;
    active?.controller.abort(abortError("The server selection was cancelled."));
    await active?.settled;
  };

  const reconcileProfileLaunchPreference = async (
    accountId: string,
    serverId: string,
    profileId: string,
    installationId: string,
    session: LocalServerSession,
  ) => {
    const scope = {
      authority: "hosted" as const,
      accountId,
      serverId,
      profileId,
      deviceClass: "web" as const,
      installationId,
    };
    let preference = {
      ...defaultAccountServerInstallationPreferences("web"),
      lastProfileId: profileId,
    };
    try {
      const candidateClient = createConnectionClient({ get: () => session });
      let bundle = await candidateClient.viewerPreferenceBundle({
        deviceClass: "web",
        installationId,
      });
      if (bundle.accountServerInstallation.values.lastProfileId !== profileId) {
        const updated = await candidateClient.recordViewerProfileActivation({
          version: "v1",
          expectedRevision: bundle.accountServerInstallation.revision,
        });
        bundle = { ...bundle, accountServerInstallation: updated };
      }
      preference = {
        ...bundle.accountServerInstallation.values,
        lastProfileId: profileId,
      };
    } catch {
      // The verified server session is already authoritative. Preference sync
      // is retryable and must not roll back a successfully activated viewer.
    }
    await connectionVault
      .saveProfileLaunchPreference(scope, preference)
      .catch(() => undefined);
  };

  const connectServer = async (
    summary: HostedServerSummary,
    selection: { generation: number; signal: AbortSignal },
    allServers = hostedServers,
    remembered = trustedConnections,
    suppliedSelectionEnvelope?: HostedProfileSelectionEnvelope,
    expectedProfileId?: string,
    routePreference: HostedRoutePreference = "lan-first",
    automaticRoutePublicationRecovery = false,
  ) => {
    const summaries = mergeServerSummaries(allServers, remembered);
    const previousSource = source;
    const previousViewer = initialViewer;
    const previousScope = viewerRuntime.activeScope();
    const previousConnection = activeServerConnection.current;
    const previousWarning = connectionWarning;
    const previousServerName =
      state.id === "server-ready" ? state.serverName : undefined;
    const account = activeHostedAccount.current;
    let transactionStarted = false;
    setBusy(true);
    try {
      throwIfSelectionStale(
        selection.generation,
        selectionGeneration.current,
        selection.signal,
      );
      const server = serverFromSummary(summary, allServers);
      if (!account)
        throw new Error(
          "Choose or sign in to a Portico Account before opening a server.",
        );
      const record =
        remembered.find((candidate) => candidate.serverId === summary.id) ??
        (await withAbortableDeadline(
          selection.signal,
          4_000,
          "Portico could not read the remembered server connection in time.",
          () => connectionVault.load(account.accountId, summary.id),
        ));
      throwIfSelectionStale(
        selection.generation,
        selectionGeneration.current,
        selection.signal,
      );
      let requiredAutomaticTrustToken: string | undefined;
      let useRememberedProfileSession = false;
      const common = {
        accountId: account.accountId,
        connectionAdapter: connectionVault,
        sessionStore: sessionStore as Required<
          Pick<SessionStore, "set" | "clear">
        > &
          Pick<SessionStore, "get">,
        createLocalClient: createConnectionClient,
        verifyFetch: browserSafeProbeFetch,
        routeProbeTimeoutMs: config.routeProbeTimeoutMs,
        routePreference,
        retryCohort: hostedRetryCohortRef.current,
        signal: selection.signal,
        stageCandidate: async (candidate: PreparedTrustedServerConnection) => {
          throwIfSelectionStale(
            selection.generation,
            selectionGeneration.current,
            selection.signal,
          );
          if (
            candidate.scope.authority !== "hosted" ||
            candidate.scope.accountId !== account.accountId ||
            candidate.scope.serverId !== summary.id ||
            (expectedProfileId !== undefined &&
              candidate.scope.profileId !== expectedProfileId)
          ) {
            throw new Error(
              "The selected server returned a different Portico viewing scope.",
            );
          }
          if (requiredAutomaticTrustToken) {
            const candidateStore: SessionStore = {
              get: () => candidate.session,
            };
            await withAbortableDeadline(
              selection.signal,
              12_000,
              "Portico could not verify automatic profile access in time.",
              () =>
                createConnectionClient(
                  candidateStore,
                ).redeemAutomaticProfileTrust({
                  token: requiredAutomaticTrustToken!,
                }),
            );
            throwIfSelectionStale(
              selection.generation,
              selectionGeneration.current,
              selection.signal,
            );
          }
          transactionStarted = true;
          const staged = await viewerRuntime.stage(
            candidate.scope,
            "server-switch",
          );
          let rollbackFenced = false;
          let rollbackFenceMode: "restore-previous" | "fail-closed" =
            "restore-previous";
          return {
            publish: async (
              publication?: TrustedServerCandidatePublication,
            ) => {
              if (!publication)
                throw new Error(
                  "Client Core did not supply final connection durability before publication.",
                );
              await withAccountPublicationLocks(
                [GLOBAL_SIGN_OUT_FENCE_ID, account.accountId],
                async () => {
                  throwIfSelectionStale(
                    selection.generation,
                    selectionGeneration.current,
                    selection.signal,
                  );
                  await assertAccountPublicationAllowed(
                    connectionVault,
                    account.accountId,
                  );
                  throwIfSelectionStale(
                    selection.generation,
                    selectionGeneration.current,
                    selection.signal,
                  );
                  activeServerConnection.current = {
                    accountId: account.accountId,
                    serverId: summary.id,
                  };
                  setPublishedServerIdentity({
                    accountId: account.accountId,
                    serverId: summary.id,
                  });
                  await staged.publish();
                  throwIfSelectionStale(
                    selection.generation,
                    selectionGeneration.current,
                    selection.signal,
                  );
                  const durabilityWarning =
                    publication.durability === "memory-only" ||
                    connectionVault.durability() === "memory-only"
                      ? ("connection.not-saved" as const)
                      : undefined;
                  setConnectionWarning(durabilityWarning);
                  becomeReady(
                    new HttpPorticoDataSource(localClient, {
                      hostedClient,
                      hostedServerId: summary.id,
                      connectionVault,
                      switchHostedProfile: async (profileId, pin, signal) => {
                        const envelope =
                          await hostedClient.createProfileSelectionEnvelope(
                            profileId,
                            {
                              serverId: summary.id,
                              ...(pin?.trim() ? { pin: pin.trim() } : {}),
                            },
                            { signal },
                          );
                        await runLatestSelection(
                          async (nextSelection) =>
                            connectServer(
                              summary,
                              nextSelection,
                              hostedServers,
                              await connectionVault.list(account.accountId),
                              envelope,
                              profileId,
                            ),
                          signal,
                        );
                        const response =
                          await localClient.request<AuthMeResponse>(
                            "/api/auth/me",
                            { signal },
                          );
                        return viewerFromAuth(response, summary.name);
                      },
                    }),
                    "hosted",
                    candidate.identity.serverFriendlyName || summary.name,
                    viewerFromAuth(candidate.identity, summary.name),
                    candidate.scope,
                  );
                },
              );
            },
            fenceRollback: (
              mode: "restore-previous" | "fail-closed" = "restore-previous",
            ) => {
              if (mode === "fail-closed") rollbackFenceMode = "fail-closed";
              // This callback is deliberately synchronous and lock-free. The
              // account lock may be held by another tab indefinitely; B must be
              // unable to execute before Client Core restores any A credential.
              activeServerConnection.current = undefined;
              if (rollbackFenceMode === "fail-closed") {
                setPublishedServerIdentity(undefined);
                setSource(undefined);
                setInitialViewer(undefined);
                setExpectedViewerScope(undefined);
                setConnectionWarning("auth.sign-out-storage-warning");
              }
              staged.fenceRollback(rollbackFenceMode);
              rollbackFenced = true;
            },
            rollback: async (
              mode: "restore-previous" | "fail-closed" = "restore-previous",
            ) => {
              if (!rollbackFenced)
                throw new Error(
                  "Candidate rollback completion requires the synchronous Web publication fence.",
                );
              try {
                await withAccountPublicationLocks(
                  [GLOBAL_SIGN_OUT_FENCE_ID, account.accountId],
                  async () => {
                    let effectiveMode =
                      mode === "fail-closed" ||
                      rollbackFenceMode === "fail-closed"
                        ? ("fail-closed" as const)
                        : ("restore-previous" as const);
                    if (effectiveMode === "restore-previous") {
                      try {
                        await assertAccountPublicationAllowed(
                          connectionVault,
                          account.accountId,
                        );
                      } catch {
                        effectiveMode = "fail-closed";
                      }
                    }
                    if (effectiveMode === "fail-closed")
                      staged.fenceRollback("fail-closed");
                    staged.rollback(effectiveMode);
                    if (effectiveMode === "restore-previous") {
                      activeServerConnection.current = previousConnection;
                      setConnectionWarning(previousWarning);
                      if (
                        previousSource &&
                        previousViewer &&
                        previousServerName
                      ) {
                        becomeReady(
                          previousSource,
                          "hosted",
                          previousServerName,
                          previousViewer,
                          previousScope,
                        );
                      } else {
                        setSource(undefined);
                        setInitialViewer(undefined);
                        setExpectedViewerScope(undefined);
                        dispatch({
                          type: "MEMBERSHIPS_READY",
                          servers: summaries,
                        });
                      }
                    } else {
                      activeServerConnection.current = undefined;
                      setPublishedServerIdentity(undefined);
                      setSource(undefined);
                      setInitialViewer(undefined);
                      setExpectedViewerScope(undefined);
                      setConnectionWarning("auth.sign-out-storage-warning");
                    }
                  },
                );
              } catch (reason) {
                activeServerConnection.current = undefined;
                setPublishedServerIdentity(undefined);
                throw reason;
              }
            },
          };
        },
      };
      const installationId = await withAbortableDeadline(
        selection.signal,
        4_000,
        "Portico could not read the browser installation identity in time.",
        () => connectionVault.installationId(),
      );
      throwIfSelectionStale(
        selection.generation,
        selectionGeneration.current,
        selection.signal,
      );
      let selectionEnvelope = suppliedSelectionEnvelope;
      if (server && !selectionEnvelope) {
        let profileDirectory;
        try {
          profileDirectory = await withAbortableDeadline(
            selection.signal,
            12_000,
            "Portico profiles did not answer in time.",
            (signal) => hostedClient.profiles({ signal }),
          );
        } catch (reason) {
          throw new HostedContinuationError("profile-directory", reason);
        }
        throwIfSelectionStale(
          selection.generation,
          selectionGeneration.current,
          selection.signal,
        );
        if (profileDirectory.accountId !== account.accountId) {
          throw new HostedContinuationError(
            "profile-directory",
            new Error(
              "Hosted Services returned profiles for a different Portico Account.",
            ),
          );
        }
        const launchScope = {
          authority: "hosted" as const,
          accountId: account.accountId,
          serverId: summary.id,
          profileId:
            record?.profileId ??
            profileDirectory.profiles[0]?.id ??
            account.accountId,
          deviceClass: "web" as const,
          installationId,
        };
        const cached =
          await connectionVault.profileLaunchPreference(launchScope);
        const preference = cached ?? {
          ...defaultAccountServerInstallationPreferences("web"),
          ...(record?.profileId ? { lastProfileId: record.profileId } : {}),
        };
        const trust = preference.lastProfileId
          ? await connectionVault.automaticProfileTrust(
              launchScope,
              preference.lastProfileId,
            )
          : undefined;
        const decision = decideProfileSelection(
          {
            authority: "hosted",
            accountId: profileDirectory.accountId,
            serverId: summary.id,
            profilesAllowed: true,
            profiles: profileDirectory.profiles,
          },
          preference,
          trust,
          { serverId: summary.id, installationId },
        );
        if (decision.kind !== "open") {
          dispatch({
            type: "PROFILE_SELECTION_REQUIRED",
            server: summary,
            servers: summaries,
            profiles: profileDirectory.profiles,
          });
          return;
        }
        const selectedProfile = decision.profile;
        if (selectedProfile.hasPIN) {
          // A locked profile may reopen only through its still-current,
          // server-issued installation trust and the matching remembered
          // profile-scoped server session. The trust is never sent to Local
          // Auth selection and never substitutes for a Cloud assertion.
          if (!record || record.profileId !== selectedProfile.id || !trust) {
            dispatch({
              type: "PROFILE_SELECTION_REQUIRED",
              server: summary,
              servers: summaries,
              profiles: profileDirectory.profiles,
            });
            return;
          }
          expectedProfileId = selectedProfile.id;
          requiredAutomaticTrustToken = trust.token;
          useRememberedProfileSession = true;
        } else {
          expectedProfileId = selectedProfile.id;
          try {
            selectionEnvelope = await withAbortableDeadline(
              selection.signal,
              12_000,
              "Portico profile verification did not answer in time.",
              (signal) =>
                hostedClient.createProfileSelectionEnvelope(
                  selectedProfile.id,
                  { serverId: summary.id },
                  { signal },
                ),
            );
          } catch (reason) {
            throw new HostedContinuationError("profile-selection", reason);
          }
          throwIfSelectionStale(
            selection.generation,
            selectionGeneration.current,
            selection.signal,
          );
        }
      }
      if (
        server &&
        selectionEnvelope &&
        (selectionEnvelope.accountId !== account.accountId ||
          selectionEnvelope.serverId !== summary.id ||
          (expectedProfileId !== undefined &&
            selectionEnvelope.profileId !== expectedProfileId))
      )
        throw new Error(
          "Hosted Services returned a profile selection for a different account, server, or profile.",
        );
      dispatch({ type: "SELECT_SERVER", server: summary, servers: summaries });
      const result = await (server && !useRememberedProfileSession
        ? withAbortableDeadline(
            selection.signal,
            Math.max(20_000, config.routeProbeTimeoutMs * 5 + 5_000),
            `The direct connection to ${summary.name} timed out.`,
            (signal) =>
              connectResilientHostedServer(server, {
                ...common,
                signal,
                hostedClient,
                loadTrustedHostedDocumentKeys: async () =>
                  trustedHostedDocumentKeysFromKeySet(
                    await hostedClient.documentSigningKeys(),
                  ),
                clientIdentity: {
                  installationId,
                  deviceName: "Web browser",
                  app: "portico-web",
                  platform: navigator.platform || "web",
                },
                selectionEnvelope: selectionEnvelope!,
                localRouteCandidates: browserSafeLocalCandidates,
                maxParallelRouteProbes: 3,
                retryDelaysMs: [1000, 2200],
              }),
            () => transactionStarted,
          )
        : record
          ? withAbortableDeadline(
              selection.signal,
              Math.max(12_000, config.routeProbeTimeoutMs * 3 + 3_000),
              `The remembered connection to ${summary.name} timed out.`,
              (signal) =>
                connectTrustedServerRecord(record, { ...common, signal }),
              () => transactionStarted,
            )
          : Promise.reject(
              new Error(
                "This server is not available to the active Portico Account.",
              ),
            ));
      throwIfSelectionStale(
        selection.generation,
        selectionGeneration.current,
        selection.signal,
      );
      if (result.identity.profileId) {
        void reconcileProfileLaunchPreference(
          account.accountId,
          summary.id,
          result.identity.profileId,
          installationId,
          result.session,
        );
      }
      void Promise.all([
        connectionVault.list(account.accountId),
        connectionVault.rememberAccount(account),
      ]).then(
        ([nextRemembered]) => {
          if (
            selection.generation === selectionGeneration.current &&
            activeServerConnection.current?.accountId === account.accountId &&
            activeServerConnection.current.serverId === summary.id
          ) {
            setTrustedConnections(nextRemembered);
          }
        },
        (reason) => {
          if (
            selection.generation === selectionGeneration.current &&
            activeServerConnection.current?.accountId === account.accountId &&
            activeServerConnection.current.serverId === summary.id
          ) {
            if (securityCriticalConnectionFailure(reason))
              failClosedForExternalAccountFence(account.accountId, reason);
            else setConnectionWarning("connection.not-saved");
          }
        },
      );
    } catch (reason) {
      if (
        selection.signal.aborted ||
        selection.generation !== selectionGeneration.current
      ) {
        if (isCleanSelectionAbort(reason)) return;
        const unsafe =
          reason instanceof Error
            ? reason
            : new Error(
                "A stale server connection could not be rolled back safely.",
              );
        selectionSecurityFailure.current = unsafe;
        viewerRuntime.failClosed();
        try {
          sessionStore.clear?.();
        } catch {
          /* Runtime and UI remain fenced even if memory cleanup also fails. */
        }
        activeServerConnection.current = undefined;
        setPublishedServerIdentity(undefined);
        setSource(undefined);
        setInitialViewer(undefined);
        setExpectedViewerScope(undefined);
        setBusy(false);
        dispatch({ type: "FAILURE", classification: "unknown", hosted: true });
        throw unsafe;
      }
      if (securityCriticalConnectionFailure(reason)) {
        const unsafe =
          reason instanceof Error
            ? reason
            : new Error("The server connection could not be proven safe.");
        selectionSecurityFailure.current = unsafe;
        viewerRuntime.failClosed();
        try {
          sessionStore.clear?.();
        } catch {
          /* UI and runtime remain fenced. */
        }
        activeServerConnection.current = undefined;
        setPublishedServerIdentity(undefined);
        setSource(undefined);
        setInitialViewer(undefined);
        setExpectedViewerScope(undefined);
        setConnectionWarning("auth.sign-out-storage-warning");
        setBusy(false);
        dispatch({
          type: "HOSTED_SIGN_IN_REQUIRED",
          messageId: "auth.sign-out-storage-warning",
        });
        if (account) {
          activeHostedAccount.current = undefined;
          setRestoredPresentation(undefined);
          setHostedServers([]);
          setTrustedConnections([]);
          markAccountSignedOut(account.accountId);
          await Promise.allSettled([
            withRuntimeDeadline(
              rawConnectionVault.markCleanupPending(account.accountId),
              4_000,
              "Portico could not publish the browser cleanup barrier in time.",
            ),
            withRuntimeDeadline(
              rawConnectionVault.forgetAccount(account.accountId),
              8_000,
              "Portico could not finish browser credential cleanup in time.",
            ),
          ]);
          // Keep both authorization fences until an explicit, freshly
          // authenticated account flow verifies cleanup. A still-valid Hosted
          // cookie must not silently remint after an uncertain transaction.
        }
        return;
      }
      const terminal = isTerminalServerAuthorizationFailure(reason);
      if (terminal) {
        const accountId = activeHostedAccount.current?.accountId;
        if (accountId)
          await connectionVault
            .remove(accountId, summary.id)
            .catch(() => undefined);
        setTrustedConnections((current) =>
          current.filter((record) => record.serverId !== summary.id),
        );
      }
      const continuationClassification =
        reason instanceof HostedContinuationError
          ? classifyRuntimeFailure(reason.reason, "profile-directory")
          : undefined;
      const classified =
        reason instanceof HostedContinuationError &&
        reason.phase !== "local-login-authorization"
          ? continuationClassification === "session-expired"
            ? "session-expired"
            : "profile-directory"
          : classifyRuntimeFailure(reason, "no-route");
      const routePublicationPending =
        automaticRoutePublicationRecovery &&
        reason instanceof HostedRoutePublicationPendingError;
      const restoredScope = viewerRuntime.activeScope();
      const restoredPrevious =
        previousSource &&
        previousViewer &&
        previousScope &&
        restoredScope &&
        !viewerRuntime.isTransitioning() &&
        sameViewerScope(previousScope, restoredScope);
      if (restoredPrevious) {
        setSource((current) => current ?? previousSource);
        setInitialViewer((current) => current ?? previousViewer);
        setExpectedViewerScope(restoredScope);
        setConnectionWarning("problem.connection-failed");
        dispatch({
          type: "READY",
          mode: "hosted",
          serverName: previousViewer.serverName,
        });
      } else {
        viewerRuntime.failClosed();
        sessionStore.clear?.();
        activeServerConnection.current = undefined;
        if (terminal) setPublishedServerIdentity(undefined);
        setSource(undefined);
        setInitialViewer(undefined);
        setExpectedViewerScope(undefined);
        dispatch({
          type: "FAILURE",
          classification: terminal
            ? "permission-removed"
            : classified === "session-expired" &&
                !(reason instanceof HostedContinuationError)
              ? "server-offline"
              : classified,
          ...(reason instanceof HostedContinuationError &&
          classified !== "session-expired"
            ? {
                messageId:
                  reason.phase === "profile-directory"
                    ? ("problem.profile-request-failed" as const)
                    : profileSelectionMessageId(reason.reason),
              }
            : reason instanceof NearbyRouteAvailableError
              ? { messageId: "connection.nearby-available" as const }
              : reason instanceof LocalNetworkRouteUnavailableError
                ? { messageId: "connection.local-network-unavailable" as const }
                : {}),
          ...(reason instanceof HostedContinuationError &&
          reason.phase === "profile-directory"
            ? hostedAvailabilityRetryFields(reason.reason)
            : {}),
          ...(routePublicationPending
            ? { automaticRoutePublicationRetry: true }
            : {}),
          serverName: summary.name,
          selectedServer: summary,
          servers: summaries,
          hosted: true,
          nearbyAvailable:
            reason instanceof NearbyRouteAvailableError ||
            reason instanceof LocalNetworkRouteUnavailableError,
        });
      }
    } finally {
      if (selection.generation === selectionGeneration.current) setBusy(false);
    }
  };

  useEffect(() => {
    if (
      state.id !== "runtime-recovery" ||
      state.automaticRoutePublicationRetry !== true ||
      !state.selectedServer
    ) {
      // Preserve the attempt across the route-discovery state entered by this
      // retry owner. Every settled destination clears it.
      if (state.id !== "route-discovery")
        routePublicationRetryAttempt.current = undefined;
      return;
    }
    const accountId = activeHostedAccount.current?.accountId;
    if (!accountId) return;
    const selectedServer = state.selectedServer;
    const key = `${accountId}\u0000${selectedServer.id}`;
    const previous = routePublicationRetryAttempt.current;
    const attempt = previous?.key === key ? previous.attempt : 0;
    const startedAt = previous?.key === key ? previous.startedAt : Date.now();
    const plan = routePublicationRetryPlan(
      attempt,
      Date.now() - startedAt,
      hostedRetryCohortRef.current,
      selectedServer.id,
    );
    if (!plan) {
      routePublicationRetryAttempt.current = undefined;
      return;
    }
    routePublicationRetryAttempt.current = { key, attempt: attempt + 1, startedAt };
    const timer = window.setTimeout(() => {
      if (document.visibilityState === "hidden" || !navigator.onLine) return;
      void runLatestSelection((selection) =>
        connectServer(
          selectedServer,
          selection,
          hostedServers,
          trustedConnections,
          undefined,
          undefined,
          "lan-first",
          true,
        ),
      ).catch(() => undefined);
    }, plan.delayMs);
    return () => window.clearTimeout(timer);
  }, [state, hostedServers, trustedConnections]);

  useEffect(() => {
    const recover = async () => {
      const selected = activeServerConnection.current;
      const account = activeHostedAccount.current;
      const activeScope = viewerRuntime.activeScope();
      if (
        !selected ||
        !account ||
        !activeScope ||
        selected.accountId !== account.accountId
      ) {
        throw new Error(
          "No active Hosted server route is available to recover.",
        );
      }
      const record = await connectionVault.load(
        selected.accountId,
        selected.serverId,
      );
      if (!record)
        throw new Error(
          "The trusted server connection is no longer available.",
        );
      const commonOptions = {
        accountId: selected.accountId,
        connectionAdapter: connectionVault,
        sessionStore: sessionStore as Required<
          Pick<SessionStore, "set" | "clear">
        > &
          Pick<SessionStore, "get">,
        createLocalClient: createConnectionClient,
        verifyFetch: browserSafeProbeFetch,
        routeProbeTimeoutMs: config.routeProbeTimeoutMs,
        routePreference: "lan-first",
        retryCohort: hostedRetryCohortRef.current,
        stageCandidate: async (candidate: PreparedTrustedServerConnection) => {
          if (!sameViewerScope(candidate.scope, activeScope)) {
            throw new Error(
              "Route recovery returned a different viewing profile.",
            );
          }
          return {
            publish: () => undefined,
            fenceRollback: () => undefined,
            rollback: () => undefined,
          };
        },
      } as const;
      let result;
      try {
        // A remembered public route is already pinned to this server identity.
        // Verify current/previous routes locally before depending on Hosted
        // Services so an unrelated Cloud outage cannot interrupt buffered or
        // cached playback from an otherwise reachable server.
        result = await connectTrustedServerRecord(record, commonOptions);
      } catch (cachedFailure) {
        if (isTerminalServerAuthorizationFailure(cachedFailure))
          throw cachedFailure;
        const server = hostedServers.find(
          (candidate) => candidate.id === selected.serverId,
        );
        if (!server) throw cachedFailure;
        result = await refreshTrustedServerRoute(record, server, {
          ...commonOptions,
          hostedClient,
          loadTrustedHostedDocumentKeys: async () =>
            trustedHostedDocumentKeysFromKeySet(
              await hostedClient.documentSigningKeys(),
            ),
          localRouteCandidates: browserSafeLocalCandidates,
          retryDelaysMs: [250, 750],
        });
      }
      if (!sameViewerScope(result.scope, activeScope))
        throw new Error("Route recovery changed the active viewing profile.");
      setTrustedConnections((current) =>
        current.map((candidate) =>
          candidate.serverId === result.record.serverId
            ? result.record
            : candidate,
        ),
      );
    };
    activeRouteRecovery.current = recover;
    return () => {
      if (activeRouteRecovery.current === recover)
        activeRouteRecovery.current = undefined;
    };
  }, [
    config.routeProbeTimeoutMs,
    connectionVault,
    hostedClient,
    hostedServers,
    sessionStore,
    viewerRuntime,
  ]);

  const openRememberedConnections = async (
    account: HostedAccountSnapshot,
    generation = hostedAccountGeneration.current,
  ): Promise<boolean> => {
    const records = (await connectionVault.list(account.accountId)).filter(
      (record) =>
        !remoteUnclaimedConnections.current.has(
          `${record.accountId}\u0000${record.serverId}`,
        ),
    );
    if (generation !== hostedAccountGeneration.current || records.length === 0)
      return false;
    activeHostedAccount.current = account;
    setTrustedConnections(records);
    setHostedServers([]);
    const summaries = records.map(summaryFromTrustedRecord);
    if (records.length === 1)
      await runLatestSelection((selection) =>
        connectServer(summaries[0], selection, [], records),
      );
    else dispatch({ type: "MEMBERSHIPS_READY", servers: summaries });
    return true;
  };

  const loadMemberships = async (
    generation = hostedAccountGeneration.current,
    account = activeHostedAccount.current,
  ) => {
    const preserveExplicitChooser = state.id === "server-selection";
    const activeAtStart = activeServerConnection.current;
    const retainingActiveRuntime = Boolean(
      account && activeAtStart?.accountId === account.accountId,
    );
    if (!retainingActiveRuntime) dispatch({ type: "LOAD_MEMBERSHIPS" });
    setBusy(true);
    let servers: HostedServer[];
    try {
      let response = await withCancellableRuntimeDeadline(
        12_000,
        "Portico could not load your servers in time.",
        (signal) => hostedClient.servers({ limit: 50 }, { signal }),
      );
      servers = [...response.items];
      const cursors = new Set<string>();
      while (response.pageInfo.hasMore) {
        const cursor = response.pageInfo.nextCursor;
        if (!cursor)
          throw new Error("Portico returned an incomplete server-list page.");
        if (cursors.has(cursor))
          throw new Error("Portico returned a repeated server-list cursor.");
        cursors.add(cursor);
        response = await withCancellableRuntimeDeadline(
          12_000,
          "Portico could not finish loading your servers in time.",
          (signal) => hostedClient.servers({ limit: 50, cursor }, { signal }),
        );
        servers.push(...response.items);
      }
    } catch (reason) {
      if (generation !== hostedAccountGeneration.current) throw reason;
      if (retainingActiveRuntime)
        setConnectionWarning("problem.cloud-unavailable");
      else
        dispatch({
          type: "FAILURE",
          classification: classifyRuntimeFailure(reason, "membership"),
          hosted: true,
          ...hostedAvailabilityRetryFields(reason),
        });
      throw reason;
    } finally {
      if (generation === hostedAccountGeneration.current) setBusy(false);
    }
    if (generation !== hostedAccountGeneration.current) return;
    setHostedServers(servers);
    let remembered: TrustedServerConnectionRecord[] = [];
    let activeServerWasUnclaimed = false;
    let activeServerId: string | undefined;
    if (account) {
      const authorized = new Set(servers.map((server) => server.id));
      for (const server of servers)
        remoteUnclaimedConnections.current.delete(
          `${account.accountId}\u0000${server.id}`,
        );
      activeServerWasUnclaimed = Boolean(
        activeAtStart &&
        activeAtStart.accountId === account.accountId &&
        !authorized.has(activeAtStart.serverId),
      );
      activeServerId = activeAtStart?.serverId;
      if (activeServerWasUnclaimed) {
        selectionGeneration.current += 1;
        activeSelection.current?.controller.abort(
          abortError(
            "This server is no longer assigned to the active Portico Account.",
          ),
        );
        activeSelection.current = undefined;
        viewerRuntime.failClosed();
        try {
          sessionStore.clear?.();
        } catch {
          /* The generation fence remains authoritative. */
        }
        activeServerConnection.current = undefined;
        setPublishedServerIdentity(undefined);
        setSource(undefined);
        setInitialViewer(undefined);
        setExpectedViewerScope(undefined);
        setConnectionWarning("problem.forbidden");
      }
      if (generation !== hostedAccountGeneration.current) return;
      const existing = await connectionVault.list(account.accountId);
      const unclaimed = existing.filter(
        (record) => !authorized.has(record.serverId),
      );
      for (const record of unclaimed) {
        remoteUnclaimedConnections.current.add(
          `${account.accountId}\u0000${record.serverId}`,
        );
        if (remoteUnclaimedConnections.current.size > 256) {
          const oldest = remoteUnclaimedConnections.current
            .values()
            .next().value;
          if (typeof oldest === "string")
            remoteUnclaimedConnections.current.delete(oldest);
        }
        const tombstone: TrustedServerRemovalTombstone = {
          schemaVersion: 1,
          accountId: record.accountId,
          serverId: record.serverId,
          mutationVersion: Math.max(record.mutationVersion ?? 0, 0) + 1,
          removedAt: new Date().toISOString(),
        };
        try {
          await connectionVault.removeWithTombstone(tombstone);
        } catch {
          // Keep the in-memory quarantine even if browser storage is full or
          // unavailable. The next authoritative membership response can clear
          // it after the server is assigned again.
          if (!activeServerWasUnclaimed)
            setConnectionWarning("auth.sign-out-storage-warning");
        }
      }
      remembered = (await connectionVault.list(account.accountId)).filter(
        (record) =>
          authorized.has(record.serverId) &&
          !remoteUnclaimedConnections.current.has(
            `${record.accountId}\u0000${record.serverId}`,
          ),
      );
      if (generation !== hostedAccountGeneration.current) return;
      setTrustedConnections(remembered);
    }
    const summaries = mergeServerSummaries(servers, remembered);
    if (activeServerWasUnclaimed) {
      dispatch({
        type: "FAILURE",
        classification: "permission-removed",
        serverName: activeServerId,
        selectedServer: activeServerId
          ? summaries.find((server) => server.id === activeServerId)
          : undefined,
        servers: summaries,
        hosted: true,
      });
    } else if (
      retainingActiveRuntime &&
      activeAtStart &&
      summaries.some((server) => server.id === activeAtStart.serverId)
    ) {
      // The active server remains authoritative. Refresh membership metadata
      // without reconnecting or blanking its already verified viewer scope.
      return;
    } else if (
      !preserveExplicitChooser &&
      summaries.length === 1 &&
      summaries[0].remoteAccessEnabled &&
      summaries[0].preferredAuthMode === "portico"
    ) {
      await runLatestSelection((selection) =>
        connectServer(
          summaries[0],
          selection,
          servers,
          remembered,
          undefined,
          undefined,
          "lan-first",
          true,
        ),
      );
    } else dispatch({ type: "MEMBERSHIPS_READY", servers: summaries });
  };

  useEffect(() => {
    if (config.mode !== "hosted" || typeof window === "undefined") return;
    const reconcile = () => {
      if (
        document.visibilityState === "hidden" ||
        membershipRefreshInFlight.current ||
        !activeHostedAccount.current
      )
        return;
      const refresh = membershipRefresh.current;
      if (!refresh) return;
      void refresh().catch(() => undefined);
    };
    window.addEventListener("focus", reconcile);
    window.addEventListener("online", reconcile);
    document.addEventListener("visibilitychange", reconcile);
    return () => {
      window.removeEventListener("focus", reconcile);
      window.removeEventListener("online", reconcile);
      document.removeEventListener("visibilitychange", reconcile);
    };
  }, [config.mode]);

  const applyPendingMembershipIntent = async (): Promise<boolean> => {
    try {
      const intent = bootstrapIntent.current;
      if (intent.inviteId) {
        await withCancellableRuntimeDeadline(
          12_000,
          "Portico could not accept this invitation in time.",
          (signal) => hostedClient.acceptInvite(intent.inviteId!, { signal }),
        );
        delete bootstrapIntent.current.inviteId;
      }
      if (intent.claimCode) {
        await withCancellableRuntimeDeadline(
          12_000,
          "Portico could not claim this server in time.",
          (signal) =>
            hostedClient.completeClaim(
              { claimCode: intent.claimCode! },
              { signal },
            ),
        );
        const returnUrl = validServerSetupReturnUrl(intent.claimReturnUrl)
          ? intent.claimReturnUrl
          : undefined;
        delete bootstrapIntent.current.claimCode;
        delete bootstrapIntent.current.claimServerName;
        delete bootstrapIntent.current.claimReturnUrl;
        clearRecoverableServerClaim();
        if (returnUrl) {
          navigate(returnUrl);
          return true;
        }
      }
      return false;
    } catch (reason) {
      // Hosted invitation acceptance and server claiming are exact,
      // same-account idempotent operations. Preserve the intent so automatic
      // availability recovery can reconcile a response lost after commit.
      throw new HostedContinuationError("membership-mutation", reason);
    }
  };

  const openPendingDeviceAuthorization = async (): Promise<boolean> => {
    const intent = bootstrapIntent.current;
    if (intent.genericDeviceAuthorizationRequested) {
      dispatch({
        type: "DEVICE_AUTHORIZATION",
        mode: "generic",
        initialCode: intent.genericDeviceAuthorizationCode,
        nativeReturn: intent.genericDeviceAuthorizationNativeReturn,
        servers: [],
      });
      return true;
    }
    if (!intent.deviceAuthorizationRequested) return false;
    let response = await withCancellableRuntimeDeadline(
      12_000,
      "Portico could not load your servers in time.",
      (signal) => hostedClient.servers({ limit: 50 }, { signal }),
    );
    const servers = [...response.items];
    const cursors = new Set<string>();
    while (response.pageInfo.hasMore) {
      const cursor = response.pageInfo.nextCursor;
      if (!cursor || cursors.has(cursor))
        throw new Error("Portico returned an incomplete server list.");
      cursors.add(cursor);
      response = await withCancellableRuntimeDeadline(
        12_000,
        "Portico could not finish loading your servers in time.",
        (signal) => hostedClient.servers({ limit: 50, cursor }, { signal }),
      );
      servers.push(...response.items);
    }
    dispatch({
      type: "DEVICE_AUTHORIZATION",
      mode: "tv",
      initialCode: intent.deviceAuthorizationCode,
      servers: mergeServerSummaries(servers, []),
    });
    return true;
  };

  const completePendingLocalLogin = async (
    selectedProfileId?: string,
    pin?: string,
  ): Promise<boolean> => {
    const intent = bootstrapIntent.current.localLogin;
    if (!intent) return false;
    let profileId = selectedProfileId?.trim() ?? "";
    if (!profileId) {
      const account = activeHostedAccount.current;
      if (!account)
        throw new Error(
          "Sign in to your Portico Account before choosing a profile.",
        );
      let directory;
      try {
        directory = await withRuntimeDeadline(
          hostedClient.profiles(),
          12_000,
          "Portico profiles did not answer in time.",
        );
      } catch (reason) {
        throw new HostedContinuationError("profile-directory", reason);
      }
      if (directory.accountId !== account.accountId) {
        throw new HostedContinuationError(
          "profile-directory",
          new Error(
            "Hosted Services returned profiles for a different Portico Account.",
          ),
        );
      }
      const onlyProfile =
        directory.profiles.length === 1 ? directory.profiles[0] : undefined;
      if (!onlyProfile || onlyProfile.hasPIN) {
        const server: HostedServerSummary = {
          id: intent.serverId,
          name: intent.serverName,
          assignedHostname: "",
          remoteAccessEnabled: true,
          preferredAuthMode: "portico",
        };
        dispatch({
          type: "PROFILE_SELECTION_REQUIRED",
          server,
          servers: [server],
          profiles: directory.profiles,
        });
        return true;
      }
      profileId = onlyProfile.id;
    }
    let target: string;
    try {
      const raw = await withCancellableRuntimeDeadline(
        12_000,
        `Portico could not authorize sign-in to ${intent.serverName} in time.`,
        (signal) =>
          hostedClient.authorizeLocalLogin(
            intent.serverId,
            {
              callbackUrl: intent.callbackUrl,
              localOrigin: intent.localOrigin,
              state: intent.state,
              serverPublicKeyFingerprint: intent.serverPublicKeyFingerprint,
              publicConsoleOriginGeneration:
                intent.publicConsoleOriginGeneration,
              profileId,
              ...(pin?.trim() ? { pin: pin.trim() } : {}),
              ...(intent.installationId
                ? { installationId: intent.installationId }
                : {}),
            },
            { signal },
          ),
      );
      const redirectUrl = (raw as unknown as { redirectUrl?: unknown })
        .redirectUrl;
      const returnedLocalOrigin = (raw as unknown as { localOrigin?: unknown })
        .localOrigin;
      if (
        typeof returnedLocalOrigin !== "string" ||
        new URL(returnedLocalOrigin).origin !== new URL(intent.localOrigin).origin
      )
        throw new Error(
          "Portico returned a different local server origin than the one requested.",
        );
      const returnedOriginGeneration = (
        raw as unknown as { publicConsoleOriginGeneration?: unknown }
      ).publicConsoleOriginGeneration;
      if (
        returnedOriginGeneration !== undefined &&
        returnedOriginGeneration !== intent.publicConsoleOriginGeneration
      )
        throw new Error(
          "Portico returned a different public server address generation than the one requested.",
        );
      target = verifiedLocalLoginRedirect(intent, redirectUrl);
    } catch (reason) {
      throw new HostedContinuationError("local-login-authorization", reason);
    }
    // Keep the short-lived same-tab handoff until its TTL elapses. Top-level
    // navigation to localhost can be denied by browser Local Network Access policy,
    // and location assignment cannot report that failure to JavaScript. If the
    // user returns to this tab, the preserved proof-bound intent can authorize
    // a fresh one-time callback without asking for account credentials again.
    navigate(target);
    return true;
  };

  const handleHostedContinuationFailure = (reason: unknown): void => {
    if (reason instanceof HostedContinuationError) {
      const classification = classifyRuntimeFailure(
        reason.reason,
        "hosted-session",
      );
      const localLogin = bootstrapIntent.current.localLogin;
      if (
        reason.phase === "local-login-authorization" &&
        reason.reason instanceof ApiError &&
        [
          "invalid_callback_url",
          "public_console_origin_unverified",
          "public_console_origin_generation_mismatch",
        ].includes(reason.reason.code)
      ) {
        dispatch({
          type: "LOCAL_LOGIN_RECOVERY_REQUIRED",
          reason: "callback-policy",
        });
        return;
      }
      if (classification === "session-expired") {
        dispatch({
          type: "HOSTED_SIGN_IN_REQUIRED",
          messageId: "auth.session-expired",
        });
        return;
      }
      if (reason.phase === "membership-mutation") {
        dispatch({
          type: "FAILURE",
          classification: classifyRuntimeFailure(reason.reason, "membership"),
          hosted: true,
          continueAccount: true,
          ...hostedAvailabilityRetryFields(reason.reason, true),
        });
        return;
      }
      const profileMessage =
        reason.phase === "local-login-authorization"
          ? profileSelectionMessageId(reason.reason)
          : "auth.profile-selection-failed";
      const isProfileFailure =
        reason.phase === "profile-directory" ||
        profileMessage !== "auth.profile-selection-failed" ||
        (reason.reason instanceof ApiError &&
          [
            "invalid_profile",
            "profile_not_found",
            "profile_not_available",
          ].includes(reason.reason.code));
      dispatch({
        type: "FAILURE",
        classification: isProfileFailure ? "profile-directory" : classification,
        messageId: isProfileFailure
          ? reason.phase === "profile-directory"
            ? "problem.profile-request-failed"
            : profileMessage
          : undefined,
        serverName: localLogin?.serverName,
        hosted: true,
        continueAccount: true,
        ...(reason.phase === "profile-directory"
          ? hostedAvailabilityRetryFields(reason.reason)
          : {}),
      });
      return;
    }
    dispatch({
      type: "FAILURE",
      classification: classifyRuntimeFailure(reason, "membership"),
      hosted: true,
      ...hostedAvailabilityRetryFields(reason),
    });
  };

  const continueWithHostedAccount = async (): Promise<void> => {
    delete bootstrapIntent.current.localLogin;
    clearRecoverableLocalLoginIntent();
    await loadMemberships(
      hostedAccountGeneration.current,
      activeHostedAccount.current,
    ).catch(() => undefined);
  };

  const rememberHostedAccount = async (
    account: HostedAccountIdentity,
    clearExplicitSignOut = false,
  ): Promise<HostedAccountSnapshot> => {
    if (!account.authenticated || !account.user)
      throw new Error("Portico Account sign-in could not be completed.");
    const accountUser = account.user;
    const accountOperationGeneration = hostedAccountGeneration.current;
    if (clearExplicitSignOut) {
      let initialQuarantine = signedOutAccountQuarantine();
      if (!initialQuarantine.trustedForRestore)
        throw new SignedOutAccountRestoreBlockedError();
      let initialCleanupPending: string[];
      try {
        initialCleanupPending = await withRuntimeDeadline(
          rawConnectionVault.cleanupPendingAccounts(),
          4_000,
          "Portico could not enumerate saved sign-out barriers in time.",
        );
      } catch (reason) {
        throw new SignedOutAccountRestoreBlockedError(reason);
      }
      const cleanupRequested =
        initialQuarantine.accountIds.has(GLOBAL_SIGN_OUT_FENCE_ID) ||
        initialQuarantine.accountIds.has(accountUser.id) ||
        initialCleanupPending.includes(GLOBAL_SIGN_OUT_FENCE_ID) ||
        initialCleanupPending.includes(accountUser.id);
      if (cleanupRequested) {
        try {
          // The global lock is acquired before discovering old account IDs.
          // Every normal vault publication also acquires it first, so the
          // discovered account set is stable while we subsequently acquire all
          // account locks in lexical order and perform exhaustive cleanup.
          await withAccountPublicationLock(
            GLOBAL_SIGN_OUT_FENCE_ID,
            async () => {
              initialQuarantine = signedOutAccountQuarantine();
              if (!initialQuarantine.trustedForRestore)
                throw new SignedOutAccountRestoreBlockedError();
              const cleanupPending = await withRuntimeDeadline(
                rawConnectionVault.cleanupPendingAccounts(),
                4_000,
                "Portico could not enumerate saved sign-out barriers in time.",
              );
              const globalFencePresent =
                initialQuarantine.accountIds.has(GLOBAL_SIGN_OUT_FENCE_ID) ||
                cleanupPending.includes(GLOBAL_SIGN_OUT_FENCE_ID);
              if (
                !globalFencePresent &&
                !initialQuarantine.accountIds.has(accountUser.id) &&
                !cleanupPending.includes(accountUser.id)
              )
                return;

              const cleanupAccountIds = new Set<string>([accountUser.id]);
              if (globalFencePresent) {
                for (const accountId of initialQuarantine.accountIds) {
                  if (accountId !== GLOBAL_SIGN_OUT_FENCE_ID)
                    cleanupAccountIds.add(accountId);
                }
                for (const accountId of cleanupPending) {
                  if (accountId !== GLOBAL_SIGN_OUT_FENCE_ID)
                    cleanupAccountIds.add(accountId);
                }
              }

              // Enumeration failure must not suppress cleanup attempts for every
              // account already known from the independent ledgers.
              const knownAccountsResult = globalFencePresent
                ? await Promise.allSettled([
                    withRuntimeDeadline(
                      rawConnectionVault.knownAccountIds(),
                      4_000,
                      "Portico could not enumerate every saved browser account in time.",
                    ),
                  ])
                : [];
              if (knownAccountsResult[0]?.status === "fulfilled") {
                for (const accountId of knownAccountsResult[0].value) {
                  if (accountId !== GLOBAL_SIGN_OUT_FENCE_ID)
                    cleanupAccountIds.add(accountId);
                }
              }

              await withAccountPublicationLocks(cleanupAccountIds, async () => {
                const barrierIds = [
                  ...cleanupAccountIds,
                  ...(globalFencePresent ? [GLOBAL_SIGN_OUT_FENCE_ID] : []),
                ];
                const barrierResults = await Promise.allSettled(
                  barrierIds.map((accountId) =>
                    withRuntimeDeadline(
                      rawConnectionVault.markCleanupPending(accountId),
                      4_000,
                      "Portico could not publish a browser cleanup barrier in time.",
                    ),
                  ),
                );
                const accountIds = [...cleanupAccountIds];
                const recordsResults = await Promise.allSettled(
                  accountIds.map((accountId) =>
                    withRuntimeDeadline(
                      rawConnectionVault.list(accountId),
                      4_000,
                      "Portico could not enumerate a saved server credential family in time.",
                    ),
                  ),
                );
                const sessions = new Map<string, LocalServerSession>();
                for (const result of recordsResults) {
                  if (result.status !== "fulfilled") continue;
                  for (const record of result.value) {
                    if (record.session.refreshToken)
                      sessions.set(record.session.refreshToken, record.session);
                  }
                }
                const revocations = [...sessions.entries()].map(
                  ([refreshToken, session]) =>
                    withRuntimeDeadline(
                      createConnectionClient(
                        createMemorySessionStore(session),
                      ).revokeNativeSession(refreshToken),
                      8_000,
                      "Portico could not revoke a saved server session in time.",
                    ),
                );
                const accountDeletions = accountIds.map((accountId) =>
                  withRuntimeDeadline(
                    rawConnectionVault.forgetAccount(accountId),
                    8_000,
                    "Portico could not remove a saved browser account in time.",
                  ),
                );
                const cleanupResults = await Promise.allSettled([
                  ...revocations,
                  ...accountDeletions,
                ]);
                const finalEnumerationResult = globalFencePresent
                  ? await Promise.allSettled([
                      withRuntimeDeadline(
                        rawConnectionVault.knownAccountIds(),
                        4_000,
                        "Portico could not verify removal of every saved browser account in time.",
                      ),
                    ])
                  : [];
                const cleanupFailed =
                  knownAccountsResult.some(
                    (result) => result.status === "rejected",
                  ) ||
                  barrierResults.some(
                    (result) => result.status === "rejected",
                  ) ||
                  recordsResults.some(
                    (result) => result.status === "rejected",
                  ) ||
                  cleanupResults.some(
                    (result) => result.status === "rejected",
                  ) ||
                  finalEnumerationResult.some(
                    (result) => result.status === "rejected",
                  ) ||
                  (finalEnumerationResult[0]?.status === "fulfilled" &&
                    finalEnumerationResult[0].value.length > 0);
                if (cleanupFailed)
                  throw new SignedOutAccountRestoreBlockedError();

                // Release account fences first. The global tombstone/barrier is
                // always last, so every partial release remains fail-closed.
                const releaseAccountIds = new Set<string>(accountIds);
                if (globalFencePresent) {
                  for (const accountId of initialQuarantine.accountIds) {
                    if (accountId !== GLOBAL_SIGN_OUT_FENCE_ID)
                      releaseAccountIds.add(accountId);
                  }
                  for (const accountId of cleanupPending) {
                    if (accountId !== GLOBAL_SIGN_OUT_FENCE_ID)
                      releaseAccountIds.add(accountId);
                  }
                }
                for (const accountId of [...releaseAccountIds].sort()) {
                  await rawConnectionVault.clearCleanupPending(accountId);
                  if (!clearAccountAfterVerifiedCleanup(accountId))
                    throw new SignedOutAccountRestoreBlockedError();
                }
                if (globalFencePresent) {
                  await rawConnectionVault.clearCleanupPending(
                    GLOBAL_SIGN_OUT_FENCE_ID,
                  );
                  if (
                    !clearAccountAfterVerifiedCleanup(GLOBAL_SIGN_OUT_FENCE_ID)
                  )
                    throw new SignedOutAccountRestoreBlockedError();
                }
              });
            },
          );
        } catch (reason) {
          if (reason instanceof SignedOutAccountRestoreBlockedError)
            throw reason;
          throw new SignedOutAccountRestoreBlockedError(reason);
        }
      }
    }
    const timestamp = new Date();
    const fallback: HostedAccountSnapshot = {
      accountId: account.user.id,
      displayName: account.user.username,
      email: account.user.email,
      lastUsedAt: timestamp.toISOString(),
      expiresAt: new Date(
        timestamp.getTime() + HOSTED_BROWSER_SESSION_TTL_MS,
      ).toISOString(),
    };
    try {
      await connectionVault.rememberAccount(fallback);
    } catch (reason) {
      // Only an ordinary metadata persistence miss may fall back to tab memory.
      // A cross-tab tombstone or cleanup barrier is an authorization fence.
      try {
        await assertAccountPublicationAllowed(
          connectionVault,
          fallback.accountId,
        );
      } catch (guardFailure) {
        throw new SignedOutAccountRestoreBlockedError(guardFailure);
      }
      if (
        reason instanceof SignedOutAccountRestoreBlockedError ||
        reason instanceof TrustedServerPublicationBlockedError
      ) {
        throw new SignedOutAccountRestoreBlockedError(reason);
      }
      setConnectionWarning("connection.not-saved");
    }
    try {
      return await withAccountPublicationLocks(
        [GLOBAL_SIGN_OUT_FENCE_ID, fallback.accountId],
        async () => {
          if (accountOperationGeneration !== hostedAccountGeneration.current) {
            throw new TrustedServerPublicationBlockedError(
              new Error(
                "The Portico Account changed before account publication.",
              ),
            );
          }
          await assertAccountPublicationAllowed(
            connectionVault,
            fallback.accountId,
          );
          let remembered: HostedAccountSnapshot | undefined;
          try {
            remembered = await connectionVault.activeAccount();
          } catch (reason) {
            if (
              reason instanceof SignedOutAccountRestoreBlockedError ||
              reason instanceof TrustedServerPublicationBlockedError
            )
              throw reason;
          }
          await assertAccountPublicationAllowed(
            connectionVault,
            fallback.accountId,
          );
          if (accountOperationGeneration !== hostedAccountGeneration.current) {
            throw new TrustedServerPublicationBlockedError(
              new Error(
                "The Portico Account changed before account publication.",
              ),
            );
          }
          if (remembered && remembered.accountId !== fallback.accountId) {
            throw new TrustedServerPublicationBlockedError(
              new Error(
                "Browser account storage returned a different active account.",
              ),
            );
          }
          remembered ??= fallback;
          activeHostedAccount.current = remembered;
          setRestoredPresentation({
            accountId: remembered.accountId,
            displayName: remembered.displayName,
          });
          if (clearExplicitSignOut)
            selectionSecurityFailure.current = undefined;
          return remembered;
        },
      );
    } catch (reason) {
      if (reason instanceof SignedOutAccountRestoreBlockedError) throw reason;
      throw new SignedOutAccountRestoreBlockedError(reason);
    }
  };

  const disconnectServer = async () => {
    await cancelActiveSelection();
    setBusy(false);
    const retainedScope = viewerRuntime.activeScope();
    if (source && initialViewer && retainedScope) {
      setExpectedViewerScope(retainedScope);
    } else if (source || initialViewer || activeServerConnection.current) {
      viewerRuntime.failClosed();
      sessionStore.clear?.();
      activeServerConnection.current = undefined;
      setPublishedServerIdentity(undefined);
      setSource(undefined);
      setInitialViewer(undefined);
      setExpectedViewerScope(undefined);
    }
    dispatch({
      type: "MEMBERSHIPS_READY",
      servers: mergeServerSummaries(hostedServers, trustedConnections),
    });
  };

  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    const bootstrap = async () => {
      const hostedGeneration = hostedAccountGeneration.current;
      setSource(undefined);
      setInitialViewer(undefined);
      setExpectedViewerScope(undefined);
      if (resolution.error) {
        dispatch({
          type: "FAILURE",
          classification: "configuration",
          hosted: config.mode === "hosted",
        });
        return;
      }
      if (config.mode === "fixtures") {
        if (!fixtureSourceLoader) {
          dispatch({
            type: "FAILURE",
            classification: "configuration",
            hosted: false,
          });
          return;
        }
        try {
          const fixtureSource = await withRuntimeDeadline(
            fixtureSourceLoader(),
            8_000,
            "Fixture runtime setup timed out.",
          );
          const [viewer, capabilities] = await withRuntimeDeadline(
            Promise.all([
              fixtureSource.viewer(controller.signal),
              fixtureSource.authCapabilities(controller.signal),
            ]),
            8_000,
            "Fixture runtime did not become ready in time.",
          );
          if (active)
            becomeReady(fixtureSource, "fixtures", viewer.serverName, {
              ...viewer,
              authCapabilities: capabilities,
            });
        } catch {
          if (active)
            dispatch({
              type: "FAILURE",
              classification: "configuration",
              hosted: false,
            });
        }
        return;
      }
      if (config.mode === "bundled") {
        dispatch({ type: "CHECK_LOCAL" });
        const bundledSource = new HttpPorticoDataSource(undefined, () =>
          rawConnectionVault.installationId(),
        );
        try {
          await withRuntimeDeadline(
            bundledSource
              .porticoClient()
              .checkServerCompatibility({ signal: controller.signal }),
            12_000,
            "The local Portico compatibility identity did not answer in time.",
          );
          const capabilities = await withRuntimeDeadline(
            bundledSource.authCapabilities(controller.signal),
            12_000,
            "The local Portico server did not answer in time.",
          );
          // Bundled Web has one ambient HttpOnly server cookie, regardless of
          // whether Hosted projection or Local Auth established it. Any
          // in-flight/uncertain mutation marker blocks bootstrap authority.
          const ambientRestore = ambientCookieRestoreStatus();
          if (!ambientRestore.trustedForRestore || ambientRestore.quarantined) {
            setConnectionWarning("auth.sign-out-storage-warning");
            try {
              await withAbortableDeadline(
                controller.signal,
                12_000,
                "The quarantined local browser session cleanup did not answer in time.",
                (signal) => bundledSource.logout(signal),
              );
            } catch {
              /* The durable barrier remains authoritative until explicit verified re-authentication. */
            }
            if (active)
              becomeReady(
                bundledSource,
                "bundled",
                capabilities.serverFriendlyName,
                {
                  authenticated: false,
                  setupRequired: capabilities.setupRequired,
                  serverName: capabilities.serverFriendlyName,
                  authCapabilities: capabilities,
                },
              );
            return;
          }
          const viewer = capabilities.setupRequired
            ? {
                authenticated: false,
                setupRequired: true,
                serverName: capabilities.serverFriendlyName,
              }
            : await withRuntimeDeadline(
                bundledSource.viewer(controller.signal),
                12_000,
                "The local Portico session did not answer in time.",
              );
          if (viewer.authenticated) {
            await withRuntimeDeadline(
              bundledSource
                .porticoClient()
                .checkCompatibility({ signal: controller.signal }),
              12_000,
              "The authenticated Portico compatibility identity did not answer in time.",
            );
          }
          if (active)
            becomeReady(
              bundledSource,
              "bundled",
              capabilities.serverFriendlyName,
              {
                ...viewer,
                serverName: capabilities.serverFriendlyName,
                authCapabilities: capabilities,
              },
            );
        } catch (reason) {
          if (!active || controller.signal.aborted) return;
          dispatch({
            type: "FAILURE",
            classification: classifyRuntimeFailure(reason, "server-offline"),
            hosted: false,
          });
        }
        return;
      }

      if (bootstrapIntent.current.ssoOnboardingToken) {
        dispatch({ type: "SSO_ONBOARDING" });
        return;
      }
      if (bootstrapIntent.current.localLoginRecoveryFailure) {
        dispatch({
          type: "LOCAL_LOGIN_RECOVERY_REQUIRED",
          reason: bootstrapIntent.current.localLoginRecoveryFailure,
        });
        return;
      }
      dispatch({ type: "CHECK_HOSTED_SESSION" });
      const quarantine = signedOutAccountQuarantine();
      if (
        !quarantine.trustedForRestore ||
        quarantine.accountIds.has(GLOBAL_SIGN_OUT_FENCE_ID)
      ) {
        dispatch({
          type: "HOSTED_SIGN_IN_REQUIRED",
          messageId: "auth.sign-out-storage-warning",
        });
        void hostedClient.logout().catch(() => undefined);
        return;
      }
      let cleanupPending: string[];
      try {
        cleanupPending = await rawConnectionVault.cleanupPendingAccounts();
      } catch {
        dispatch({
          type: "HOSTED_SIGN_IN_REQUIRED",
          messageId: "auth.sign-out-storage-warning",
        });
        void hostedClient.logout().catch(() => undefined);
        return;
      }
      let rememberedAccount: HostedAccountSnapshot | undefined;
      let rememberedRestoreBlocked = false;
      try {
        rememberedAccount = await connectionVault.activeAccount();
        if (rememberedAccount) {
          await assertAccountPublicationAllowed(
            connectionVault,
            rememberedAccount.accountId,
          );
          setRestoredPresentation({
            accountId: rememberedAccount.accountId,
            displayName: rememberedAccount.displayName,
          });
        }
      } catch (reason) {
        rememberedRestoreBlocked = true;
        if (!(reason instanceof SignedOutAccountRestoreBlockedError)) {
          dispatch({
            type: "HOSTED_SIGN_IN_REQUIRED",
            messageId: "auth.sign-out-storage-warning",
          });
          void hostedClient.logout().catch(() => undefined);
          return;
        }
      }
      // A Web restart cannot restore credentials from durable storage by
      // design. During an already-running tab's bootstrap/reconciliation,
      // however, an active direct session is process-memory authority: retry
      // it before Hosted /me and preserve the established viewer if the cloud
      // control plane is unavailable. Explicit login/claim/device intents
      // remain Hosted-first.
      if (
        rememberedAccount &&
        activeServerConnection.current?.accountId === rememberedAccount.accountId &&
        activeHostedAccount.current?.accountId === rememberedAccount.accountId &&
        !bootstrapIntent.current.inviteId &&
        !bootstrapIntent.current.claimCode &&
        !bootstrapIntent.current.localLogin &&
        !bootstrapIntent.current.deviceAuthorizationRequested &&
        !bootstrapIntent.current.genericDeviceAuthorizationRequested &&
        (await openRememberedConnections(rememberedAccount, hostedGeneration).catch(() => false))
      )
        return;
      try {
        const account = await withAbortableDeadline(
          controller.signal,
          rememberedAccount ? 3500 : 12_000,
          "Portico Account sign-in did not answer in time.",
          (signal) => hostedClient.me({ signal }),
        );
        if (!active || hostedGeneration !== hostedAccountGeneration.current)
          return;
        if (!account.authenticated || !account.user) {
          if (
            rememberedAccount &&
            !bootstrapIntent.current.inviteId &&
            !bootstrapIntent.current.claimCode &&
            !bootstrapIntent.current.localLogin &&
            !bootstrapIntent.current.deviceAuthorizationRequested &&
            !bootstrapIntent.current.genericDeviceAuthorizationRequested &&
            (await openRememberedConnections(
              rememberedAccount,
              hostedGeneration,
            ))
          )
            return;
          dispatch({
            type: "HOSTED_SIGN_IN_REQUIRED",
            messageId:
              rememberedRestoreBlocked || cleanupPending.length > 0
                ? "auth.sign-out-storage-warning"
                : undefined,
          });
          return;
        }
        const currentAccountStatus = signedOutAccountRestoreStatus(
          account.user.id,
        );
        if (
          !currentAccountStatus.trustedForRestore ||
          currentAccountStatus.quarantined ||
          cleanupPending.includes(GLOBAL_SIGN_OUT_FENCE_ID) ||
          cleanupPending.includes(account.user.id)
        ) {
          dispatch({
            type: "HOSTED_SIGN_IN_REQUIRED",
            messageId: "auth.sign-out-storage-warning",
          });
          void hostedClient.logout().catch(() => undefined);
          return;
        }
        const activeAccount = await rememberHostedAccount(account);
        try {
          if (await openPendingDeviceAuthorization()) return;
          if (await completePendingLocalLogin()) return;
          if (await applyPendingMembershipIntent()) return;
          await loadMemberships(hostedGeneration, activeAccount);
        } catch (reason) {
          if (!active || hostedGeneration !== hostedAccountGeneration.current)
            return;
          if (reason instanceof HostedContinuationError)
            handleHostedContinuationFailure(reason);
          else throw reason;
        }
      } catch (reason) {
        if (!active || hostedGeneration !== hostedAccountGeneration.current)
          return;
        if (
          rememberedAccount &&
          !bootstrapIntent.current.inviteId &&
          !bootstrapIntent.current.claimCode &&
          !bootstrapIntent.current.localLogin &&
          !bootstrapIntent.current.deviceAuthorizationRequested &&
          !bootstrapIntent.current.genericDeviceAuthorizationRequested &&
          (await openRememberedConnections(
            rememberedAccount,
            hostedGeneration,
          ).catch(() => false))
        )
          return;
        if (
          rememberedRestoreBlocked ||
          cleanupPending.length > 0 ||
          reason instanceof SignedOutAccountRestoreBlockedError
        ) {
          dispatch({
            type: "HOSTED_SIGN_IN_REQUIRED",
            messageId: "auth.sign-out-storage-warning",
          });
          return;
        }
        const classification = classifyHostedSessionCheckFailure(reason);
        if (classification === "session-expired")
          dispatch({
            type: "HOSTED_SIGN_IN_REQUIRED",
            messageId: "auth.session-expired",
          });
        else
          dispatch({
            type: "FAILURE",
            classification,
            hosted: true,
            ...hostedAvailabilityRetryFields(reason),
          });
      }
    };
    void bootstrap();
    return () => {
      active = false;
      controller.abort();
    };
  }, [
    config.mode,
    fixtureSourceLoader,
    hostedClient,
    rawConnectionVault,
    resolution.error,
    revision,
  ]);

  const refreshHostedMemberships = async (): Promise<void> => {
    const generation = hostedAccountGeneration.current;
    let account = activeHostedAccount.current;
    if (!account) {
      const identity = await withCancellableRuntimeDeadline(
        12_000,
        "Portico Account sign-in did not answer in time.",
        (signal) => hostedClient.me({ signal }),
      );
      if (!identity.authenticated || !identity.user)
        throw new Error("The Portico Account session is no longer active.");
      account = await rememberHostedAccount(identity);
    }
    if (generation !== hostedAccountGeneration.current) return;
    await loadMemberships(generation, account);
  };
  const refreshHostedMembershipDirectory = (): Promise<void> => {
    const active = membershipRefreshInFlight.current;
    if (active) return active;
    const pending = refreshHostedMemberships().finally(() => {
      if (membershipRefreshInFlight.current === pending)
        membershipRefreshInFlight.current = undefined;
    });
    membershipRefreshInFlight.current = pending;
    return pending;
  };
  // Directory refresh is the sole recovery owner for a remembered chooser.
  // It first republishes the authenticated account, then replaces the visible
  // directory. A displayed server can therefore never remain an enabled but
  // inert remembered-only button after Hosted Services recovers.
  membershipRefresh.current = refreshHostedMembershipDirectory;

  const selectedHostedServerId: unknown =
    publishedServerIdentity &&
    publishedServerIdentity.accountId === activeHostedAccount.current?.accountId
      ? publishedServerIdentity.serverId
      : undefined;
  const value: RuntimeContextValue = {
    config,
    state,
    source,
    initialViewer,
    restoredPresentation,
    selectedHostedServerId:
      typeof selectedHostedServerId === "string" ? selectedHostedServerId : undefined,
    accountSettingsOpen,
    openAccountSettings: () => setAccountSettingsOpen(true),
    closeAccountSettings: () => setAccountSettingsOpen(false),
    expectedViewerScope,
    viewerRuntime,
    connectionWarning,
    hostedRetryCohort,
    hostedAvailabilityFailureStartedAt:
      hostedAvailabilityFailureStartedAtRef.current,
    dismissConnectionWarning: () => setConnectionWarning(undefined),
    busy,
    mfaRequired,
    hasPasswordResetIntent: Boolean(bootstrapIntent.current.resetToken),
    ssoOnboardingToken: bootstrapIntent.current.ssoOnboardingToken,
    hasServerClaimIntent: Boolean(bootstrapIntent.current.claimCode),
    hasDeviceAuthorizationIntent: Boolean(
      bootstrapIntent.current.deviceAuthorizationRequested ||
      bootstrapIntent.current.genericDeviceAuthorizationRequested,
    ),
    deviceAuthorizationProvider:
      bootstrapIntent.current.genericDeviceAuthorizationProvider,
    deviceAuthorizationCode:
      bootstrapIntent.current.genericDeviceAuthorizationCode,
    nativeDeviceAuthorizationReturn:
      bootstrapIntent.current.genericDeviceAuthorizationNativeReturn === true,
    serverClaimName: bootstrapIntent.current.claimServerName,
    localLoginServerName: bootstrapIntent.current.localLogin?.serverName,
    hasLocalLoginIntent: Boolean(bootstrapIntent.current.localLogin),
    retry: () => {
      void cancelActiveSelection().then(() =>
        setRevision((current) => current + 1),
      );
    },
    tryNearbyConnection: async () => {
      if (state.id !== "runtime-recovery" || !state.selectedServer) return;
      await runLatestSelection((selection) =>
        connectServer(
          state.selectedServer!,
          selection,
          hostedServers,
          trustedConnections,
          undefined,
          undefined,
          "lan-only",
        ),
      );
    },
    recoverActiveRoute: async () => {
      const recover = activeRouteRecovery.current;
      if (!recover)
        throw new Error("No active server route is available to recover.");
      let recovery = routeRecoveryInFlight.current;
      if (!recovery) {
        recovery = recover().finally(() => {
          if (routeRecoveryInFlight.current === recovery)
            routeRecoveryInFlight.current = undefined;
        });
        routeRecoveryInFlight.current = recovery;
      }
      await recovery;
    },
    continueWithHostedAccount,
    selectServer: async (server) => {
      const account = activeHostedAccount.current;
      if (!account) return;
      // The server object comes from the currently published Hosted directory.
      // Do not re-authorize it through separately scheduled React state arrays:
      // Hosted profile/envelope issuance remains the membership authority.
      // The Hosted membership selection is the one durable owner of this
      // identity. Server transport publication may follow or fail, but
      // account-only operations must retain the selected, account-fenced ID.
      setPublishedServerIdentity({
        accountId: account.accountId,
        serverId: server.id,
      });
      await runLatestSelection((selection) =>
        connectServer(server, selection),
      );
    },
    selectProfile: async (profileId, pin) => {
      if (state.id !== "profile-selection") return;
      const selectionState = state;
      const profile = selectionState.profiles.find(
        (candidate) => candidate.id === profileId,
      );
      if (!profile) return;
      const account = activeHostedAccount.current;
      if (!account) return;
      await runLatestSelection(async (selection) => {
        setBusy(true);
        try {
          if (bootstrapIntent.current.localLogin) {
            await completePendingLocalLogin(profile.id, pin);
            return;
          }
          const envelope = await withAbortableDeadline(
            selection.signal,
            12_000,
            "Portico profile verification did not answer in time.",
            (signal) =>
              hostedClient.createProfileSelectionEnvelope(
                profile.id,
                {
                  serverId: selectionState.selectedServer.id,
                  ...(pin?.trim() ? { pin: pin.trim() } : {}),
                },
                { signal },
              ),
          );
          throwIfSelectionStale(
            selection.generation,
            selectionGeneration.current,
            selection.signal,
          );
          await connectServer(
            selectionState.selectedServer,
            selection,
            hostedServers,
            trustedConnections,
            envelope,
            profile.id,
          );
        } catch (reason) {
          if (
            selection.signal.aborted ||
            selection.generation !== selectionGeneration.current
          )
            return;
          dispatch({
            type: "PROFILE_SELECTION_REQUIRED",
            server: selectionState.selectedServer,
            servers: selectionState.servers,
            profiles: selectionState.profiles,
            messageId: profileSelectionMessageId(reason),
          });
        } finally {
          if (selection.generation === selectionGeneration.current)
            setBusy(false);
        }
      }).catch(() => undefined);
    },
    beginProfileSelection: async () => {
      if (config.mode !== "hosted") return;
      const connected = activeServerConnection.current;
      const account = activeHostedAccount.current;
      if (!connected || !account) return;
      const summary = mergeServerSummaries(
        hostedServers,
        trustedConnections,
      ).find((candidate) => candidate.id === connected.serverId);
      if (!summary) return;
      await cancelActiveSelection();
      setBusy(true);
      try {
        const directory = await withRuntimeDeadline(
          hostedClient.profiles(),
          12_000,
          "Portico profiles did not answer in time.",
        );
        if (directory.accountId !== account.accountId) {
          throw new Error(
            "Hosted Services returned profiles for a different Portico Account.",
          );
        }
        // Profile selection is an account-authenticated state outside the
        // server application. Fail the old viewer scope closed before the
        // selector is published so no prior profile data can remain visible.
        viewerRuntime.failClosed();
        sessionStore.clear?.();
        activeServerConnection.current = undefined;
        setSource(undefined);
        setInitialViewer(undefined);
        setExpectedViewerScope(undefined);
        dispatch({
          type: "PROFILE_SELECTION_REQUIRED",
          server: summary,
          servers: mergeServerSummaries(hostedServers, trustedConnections),
          profiles: directory.profiles,
        });
      } catch (reason) {
        dispatch({
          type: "FAILURE",
          classification: "profile-directory",
          selectedServer: summary,
          serverName: summary.name,
          messageId: profileSelectionMessageId(reason),
          hosted: true,
          ...hostedAvailabilityRetryFields(reason),
        });
      } finally {
        setBusy(false);
      }
    },
    reselectServer: disconnectServer,
    disconnectServer,
    hostedLogin: async (credentials) => {
      setBusy(true);
      let activeAccount: HostedAccountSnapshot;
      try {
        const installationId = await withRuntimeDeadline(
          rawConnectionVault.installationId(),
          4_000,
          "Portico could not read this browser identity in time.",
        );
        const account = await withCancellableRuntimeDeadline(
          12_000,
          "Portico Account sign-in timed out.",
          (signal) =>
            hostedClient.login(
              {
                ...credentials,
                deviceName: "Portico Web",
                devicePlatform: "web",
                installationId,
              },
              { signal },
            ),
        );
        if (!account.authenticated || !account.user) {
          throw new Error("Portico Account sign-in could not be completed.");
        }
        activeAccount = await rememberHostedAccount(account, true);
        setMfaRequired(false);
      } catch (reason) {
        if (reason instanceof ApiError && reason.code === "mfa_required") {
          setMfaRequired(true);
          setBusy(false);
          return;
        }
        if (reason instanceof SignedOutAccountRestoreBlockedError) {
          dispatch({
            type: "HOSTED_SIGN_IN_REQUIRED",
            messageId: "auth.sign-out-storage-warning",
          });
          void hostedClient.logout().catch(() => undefined);
          setBusy(false);
          return;
        }
        setBusy(false);
        throw reason;
      }
      try {
        if (await openPendingDeviceAuthorization()) return;
        if (await completePendingLocalLogin()) return;
        if (await applyPendingMembershipIntent()) return;
        await loadMemberships(hostedAccountGeneration.current, activeAccount);
      } catch (reason) {
        handleHostedContinuationFailure(reason);
      } finally {
        setBusy(false);
      }
    },
    hostedRegister: async (details) => {
      setBusy(true);
      let activeAccount: HostedAccountSnapshot;
      try {
        const installationId = await withRuntimeDeadline(
          rawConnectionVault.installationId(),
          4_000,
          "Portico could not read this browser identity in time.",
        );
        let registrationFailure: unknown;
        try {
          await withCancellableRuntimeDeadline(
            12_000,
            "Portico Account registration timed out.",
            (signal) => hostedClient.register(details, { signal }),
          );
        } catch (reason) {
          // A disconnected POST may have committed before the browser observed
          // the abort. One exact-credential login reconciles that ambiguity
          // without replaying account creation.
          registrationFailure = reason;
        }
        let account: HostedAccountIdentity;
        try {
          account = await withCancellableRuntimeDeadline(
            12_000,
            "Portico could not open the new account session in time.",
            (signal) =>
              hostedClient.login(
                {
                  login: details.username,
                  password: details.password,
                  deviceName: "Portico Web",
                  devicePlatform: "web",
                  installationId,
                },
                { signal },
              ),
          );
        } catch (loginFailure) {
          throw registrationFailure ?? loginFailure;
        }
        if (!account.authenticated || !account.user) {
          throw new Error(
            "Portico created the account but could not open its session.",
          );
        }
        activeAccount = await rememberHostedAccount(account, true);
      } catch (reason) {
        if (reason instanceof SignedOutAccountRestoreBlockedError) {
          dispatch({
            type: "HOSTED_SIGN_IN_REQUIRED",
            messageId: "auth.sign-out-storage-warning",
          });
          void hostedClient.logout().catch(() => undefined);
          setBusy(false);
          return;
        }
        setBusy(false);
        throw reason;
      }
      try {
        if (await openPendingDeviceAuthorization()) return;
        if (await completePendingLocalLogin()) return;
        if (await applyPendingMembershipIntent()) return;
        await loadMemberships(hostedAccountGeneration.current, activeAccount);
      } catch (reason) {
        handleHostedContinuationFailure(reason);
      } finally {
        setBusy(false);
      }
    },
    previewSSOOnboarding: async (onboardingToken, signal) => {
      const raw = await withCancellableRuntimeDeadline(
        12_000,
        "Portico could not load account setup in time.",
        (deadlineSignal) =>
          hostedClient.request<unknown>("/api/auth/sso/onboarding/preview", {
            method: "POST",
            body: { onboardingToken },
            signal: signal ?? deadlineSignal,
          }),
      );
      if (!raw || typeof raw !== "object" || Array.isArray(raw))
        throw new Error("Portico returned an invalid account setup response.");
      const preview = raw as Record<string, unknown>;
      if (
        (preview.provider !== "google" && preview.provider !== "apple") ||
        (preview.privateEmail !== undefined &&
          typeof preview.privateEmail !== "boolean") ||
        (preview.verifiedContactEmailRequired !== undefined &&
          typeof preview.verifiedContactEmailRequired !== "boolean") ||
        (preview.providerEmail !== undefined &&
          typeof preview.providerEmail !== "string") ||
        (preview.suggestedUsername !== undefined &&
          typeof preview.suggestedUsername !== "string")
      ) {
        throw new Error("Portico returned an invalid account setup response.");
      }
      return {
        provider: preview.provider,
        ...(preview.providerEmail
          ? { providerEmail: preview.providerEmail }
          : {}),
        privateEmail: preview.privateEmail === true,
        verifiedContactEmailRequired:
          preview.verifiedContactEmailRequired === true,
        ...(preview.suggestedUsername
          ? { suggestedUsername: preview.suggestedUsername }
          : {}),
      };
    },
    completeSSOOnboarding: async (details) => {
      const existing = ssoOnboardingCompletionInFlight.current;
      if (existing) return existing;
      const completionRequest = (async () => {
        setBusy(true);
        try {
          const raw = await withCancellableRuntimeDeadline(
            12_000,
            "Portico could not finish account setup in time.",
            (signal) =>
              hostedClient.request<unknown>("/api/auth/sso/onboarding/complete", {
                method: "POST",
                body: details,
                signal,
              }),
          );
          if (!raw || typeof raw !== "object" || Array.isArray(raw))
            throw new Error(
              "Portico returned an invalid account setup response.",
            );
          const completion = raw as Record<string, unknown>;
          if (
            completion.usernameUnavailable === true &&
            completion.onboardingRequired === true
          ) {
            if (
              typeof completion.onboardingToken !== "string" ||
              !completion.onboardingToken.trim()
            ) {
              throw new Error(
                "Portico could not safely retry account setup. Start again with Google or Apple.",
              );
            }
            saveRecoverableSSOOnboarding(completion.onboardingToken.trim());
            throw Object.assign(new Error("That username is already in use."), {
              code: "username_unavailable",
              usernameUnavailable: true,
              onboardingToken: completion.onboardingToken.trim(),
            });
          }
          if (completion.authenticated !== true)
            throw new Error(
              "Portico did not finish account setup. Start again with Google or Apple.",
            );
          delete bootstrapIntent.current.ssoOnboardingToken;
          clearRecoverableSSOOnboarding();
          setRevision((current) => current + 1);
        } finally {
          setBusy(false);
        }
      })();
      ssoOnboardingCompletionInFlight.current = completionRequest;
      try {
        await completionRequest;
      } finally {
        if (ssoOnboardingCompletionInFlight.current === completionRequest) {
          ssoOnboardingCompletionInFlight.current = undefined;
        }
      }
    },
    hostedLogout: async () => {
      const signedOutAccountId =
        activeHostedAccount.current?.accountId ??
        activeServerConnection.current?.accountId ??
        viewerRuntime.activeScope()?.accountId ??
        GLOBAL_SIGN_OUT_FENCE_ID;
      // Publish both independent barriers synchronously/before any teardown or
      // lock wait. A hung pre-existing vault publication may delay cleanup, but
      // it can never delay the immediate restart and active-tab authorization
      // fence.
      const immediateLedgerPublished = markAccountSignedOut(signedOutAccountId);
      const immediateBarrierPublication = withRuntimeDeadline(
        rawConnectionVault.markCleanupPending(signedOutAccountId),
        4_000,
        "Portico could not save the sign-out cleanup barrier in time.",
      ).then(
        () => true,
        () => false,
      );
      broadcastAccountFence(signedOutAccountId);
      let activeSessionAtSignOut: LocalServerSession | undefined;
      let activeSessionReadFailed = false;
      try {
        activeSessionAtSignOut = sessionStore.get();
      } catch {
        activeSessionReadFailed = true;
      }
      hostedAccountGeneration.current += 1;
      viewerRuntime.fence();
      const selectionCleanup = cancelActiveSelection();
      activeHostedAccount.current = undefined;
      setRestoredPresentation(undefined);
      activeServerConnection.current = undefined;
      setPublishedServerIdentity(undefined);
      setMfaRequired(false);
      setSource(undefined);
      setInitialViewer(undefined);
      setExpectedViewerScope(undefined);
      setHostedServers([]);
      setTrustedConnections([]);
      setBusy(false);
      setConnectionWarning(undefined);
      dispatch({ type: "HOSTED_SIGN_IN_REQUIRED" });

      // Drain publications that linearized before sign-out, re-publish the
      // fences after that drain, and discover every account when identity was
      // not yet known. All normal vault publishers acquire the global lock
      // first, so the nested account lock set is stable.
      const publicationDrain = await withRuntimeDeadline(
        withAccountPublicationLock(GLOBAL_SIGN_OUT_FENCE_ID, async () => {
          const knownAccountsResult =
            signedOutAccountId === GLOBAL_SIGN_OUT_FENCE_ID
              ? await Promise.allSettled([
                  withRuntimeDeadline(
                    rawConnectionVault.knownAccountIds(),
                    4_000,
                    "Portico could not enumerate every saved browser account in time.",
                  ),
                ])
              : [];
          const accountIds = new Set<string>();
          if (signedOutAccountId !== GLOBAL_SIGN_OUT_FENCE_ID)
            accountIds.add(signedOutAccountId);
          if (knownAccountsResult[0]?.status === "fulfilled") {
            for (const accountId of knownAccountsResult[0].value) {
              if (accountId !== GLOBAL_SIGN_OUT_FENCE_ID)
                accountIds.add(accountId);
            }
          }
          return withAccountPublicationLocks(accountIds, async () => {
            const ledgerPublished = markAccountSignedOut(signedOutAccountId);
            const barrierIds = [
              ...new Set([signedOutAccountId, ...accountIds]),
            ];
            const barrierResults = await Promise.allSettled(
              barrierIds.map((accountId) =>
                withRuntimeDeadline(
                  rawConnectionVault.markCleanupPending(accountId),
                  4_000,
                  "Portico could not save a sign-out cleanup barrier in time.",
                ),
              ),
            );
            broadcastAccountFence(signedOutAccountId);
            return {
              accountIds: [...accountIds],
              barrierPublished: barrierResults.every(
                (result) => result.status === "fulfilled",
              ),
              enumerationTrusted: knownAccountsResult.every(
                (result) => result.status === "fulfilled",
              ),
              ledgerPublished,
            };
          });
        }),
        12_000,
        "Portico could not drain browser credential publication in time.",
      ).then(
        (result) => ({ ...result, acquired: true as const }),
        () => ({
          accountIds:
            signedOutAccountId === GLOBAL_SIGN_OUT_FENCE_ID
              ? []
              : [signedOutAccountId],
          acquired: false as const,
          barrierPublished: false,
          enumerationTrusted: false,
          ledgerPublished: false,
        }),
      );
      const immediateBarrierPublished = await immediateBarrierPublication;
      const rememberedResults = await Promise.allSettled(
        publicationDrain.accountIds.map((accountId) =>
          withRuntimeDeadline(
            rawConnectionVault.list(accountId),
            4_000,
            "Portico could not enumerate saved server sessions in time.",
          ),
        ),
      );
      const sessionsToRevoke = new Map<string, LocalServerSession>();
      if (activeSessionAtSignOut?.refreshToken)
        sessionsToRevoke.set(
          activeSessionAtSignOut.refreshToken,
          activeSessionAtSignOut,
        );
      for (const result of rememberedResults) {
        if (result.status !== "fulfilled") continue;
        for (const record of result.value) {
          if (record.session.refreshToken)
            sessionsToRevoke.set(record.session.refreshToken, record.session);
        }
      }
      const remoteRevocations = [...sessionsToRevoke.entries()].map(
        ([refreshToken, session]) => {
          const revokeStore = createMemorySessionStore(session);
          return withRuntimeDeadline(
            createConnectionClient(revokeStore).revokeNativeSession(
              refreshToken,
            ),
            8_000,
            "Portico could not revoke a server session in time.",
          );
        },
      );

      const runtimeCleanup = (async () => {
        await selectionCleanup;
        let failure: unknown;
        try {
          if (viewerRuntime.activeScope())
            await viewerRuntime.transition(undefined, "sign-out");
        } catch (reason) {
          failure = reason;
        } finally {
          viewerRuntime.failClosed();
        }
        if (failure !== undefined) throw failure;
      })();
      const cleanupResults = await Promise.allSettled([
        withRuntimeDeadline(
          runtimeCleanup,
          8_000,
          "Portico could not finish closing the active server in time.",
        ),
        Promise.resolve().then(() => sessionStore.clear?.()),
        ...publicationDrain.accountIds.map((accountId) =>
          withRuntimeDeadline(
            rawConnectionVault.forgetAccount(accountId),
            8_000,
            "Portico could not remove a signed-out browser account in time.",
          ),
        ),
        withRuntimeDeadline(
          hostedClient.logout(),
          8_000,
          "Portico Account sign-out timed out.",
        ),
        ...remoteRevocations,
      ]);
      viewerRuntime.failClosed();

      const primaryCleanupSucceeded =
        !activeSessionReadFailed &&
        publicationDrain.acquired &&
        publicationDrain.enumerationTrusted &&
        immediateLedgerPublished &&
        immediateBarrierPublished &&
        publicationDrain.ledgerPublished &&
        publicationDrain.barrierPublished &&
        rememberedResults.every((result) => result.status === "fulfilled") &&
        cleanupResults.every((result) => result.status === "fulfilled");
      let fenceReleaseSucceeded = false;
      if (primaryCleanupSucceeded) {
        try {
          await withRuntimeDeadline(
            withAccountPublicationLock(GLOBAL_SIGN_OUT_FENCE_ID, () =>
              withAccountPublicationLocks(
                publicationDrain.accountIds,
                async () => {
                  if (signedOutAccountId === GLOBAL_SIGN_OUT_FENCE_ID) {
                    const remaining =
                      await rawConnectionVault.knownAccountIds();
                    if (remaining.length > 0)
                      throw new SignedOutAccountRestoreBlockedError();
                  }
                  for (const accountId of [
                    ...publicationDrain.accountIds,
                  ].sort()) {
                    await rawConnectionVault.clearCleanupPending(accountId);
                    if (!clearAccountAfterVerifiedCleanup(accountId))
                      throw new SignedOutAccountRestoreBlockedError();
                  }
                  if (signedOutAccountId === GLOBAL_SIGN_OUT_FENCE_ID) {
                    await rawConnectionVault.clearCleanupPending(
                      GLOBAL_SIGN_OUT_FENCE_ID,
                    );
                    if (
                      !clearAccountAfterVerifiedCleanup(
                        GLOBAL_SIGN_OUT_FENCE_ID,
                      )
                    )
                      throw new SignedOutAccountRestoreBlockedError();
                  }
                },
              ),
            ),
            8_000,
            "Portico could not verify final browser sign-out cleanup in time.",
          );
          fenceReleaseSucceeded = true;
        } catch {
          /* Every remaining marker stays fail-closed. */
        }
      }
      const localCleanupFailed =
        !primaryCleanupSucceeded || !fenceReleaseSucceeded;
      if (!localCleanupFailed) selectionSecurityFailure.current = undefined;
      if (localCleanupFailed) {
        dispatch({
          type: "HOSTED_SIGN_IN_REQUIRED",
          messageId: "auth.sign-out-storage-warning",
        });
      }
    },
    refreshMemberships: refreshHostedMembershipDirectory,
    canSelectHostedServer: Boolean(activeHostedAccount.current),
    claimServer: async (claimCode) => {
      setBusy(true);
      try {
        await withCancellableRuntimeDeadline(
          12_000,
          "Portico could not claim this server in time.",
          (signal) =>
            hostedClient.completeClaim(
              { claimCode: claimCode.trim() },
              { signal },
            ),
        );
      } finally {
        setBusy(false);
      }
      await loadMemberships();
    },
    acceptInvite: async (inviteId) => {
      setBusy(true);
      try {
        await withCancellableRuntimeDeadline(
          12_000,
          "Portico could not accept this invitation in time.",
          (signal) => hostedClient.acceptInvite(inviteId.trim(), { signal }),
        );
      } finally {
        setBusy(false);
      }
      await loadMemberships();
    },
    previewTVSetup: async (code, signal) =>
      withCancellableRuntimeDeadline(
        12_000,
        "Portico could not check this TV setup request in time.",
        (deadlineSignal) =>
          hostedClient.previewTVSetupSession(
            { code: code.trim() },
            { signal: signal ?? deadlineSignal },
          ),
      ),
    authorizeTVSetup: async (preview, serverId, signal) => {
      const result = await withCancellableRuntimeDeadline(
        12_000,
        "Portico could not authorize this TV in time.",
        (deadlineSignal) =>
          hostedClient.authorizeTVSetupGrant(
            {
              code: preview.code,
              setupSessionId: preview.setupSessionId,
              serverId,
            },
            { signal: signal ?? deadlineSignal },
          ),
      );
      delete bootstrapIntent.current.deviceAuthorizationCode;
      delete bootstrapIntent.current.deviceAuthorizationRequested;
      clearRecoverableDeviceAuthorization();
      return result;
    },
    previewGenericDeviceAuthorization: async (code, signal) =>
      hostedClient.previewDeviceAuthorization(
        { userCode: code.trim() },
        { signal },
      ),
    decideGenericDeviceAuthorization: async (code, decision, signal) => {
      const result = await hostedClient.decideDeviceAuthorization(
        { userCode: code.trim(), decision },
        { signal },
      );
      delete bootstrapIntent.current.genericDeviceAuthorizationCode;
      delete bootstrapIntent.current.genericDeviceAuthorizationRequested;
      clearRecoverableGenericDeviceAuthorization();
      return result;
    },
    requestPasswordReset: async (email) => {
      setBusy(true);
      try {
        await withCancellableRuntimeDeadline(
          12_000,
          "Portico Account recovery timed out.",
          (signal) =>
            hostedClient.requestPasswordReset(
              { email: email.trim() },
              { signal },
            ),
        );
      } finally {
        setBusy(false);
      }
    },
    completePasswordReset: async (password) => {
      const token = bootstrapIntent.current.resetToken;
      if (!token)
        throw new Error(
          "This recovery link has expired or is incomplete. Request a new recovery email.",
        );
      setBusy(true);
      try {
        await withCancellableRuntimeDeadline(
          12_000,
          "Portico could not update this password in time.",
          (signal) =>
            hostedClient.completePasswordReset({ token, password }, { signal }),
        );
        delete bootstrapIntent.current.resetToken;
        dispatch({ type: "HOSTED_SIGN_IN_REQUIRED" });
      } finally {
        setBusy(false);
      }
    },
  };

  return (
    <RuntimeContext.Provider value={value}>{children}</RuntimeContext.Provider>
  );
}
