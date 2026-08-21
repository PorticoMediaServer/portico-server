/** Profile-safe cache identity and runtime transition orchestration. */

import type { AuthMeResponse, NativeSessionCredentials } from "./types.js";

export type ViewerScope = {
  authority: "hosted" | "local";
  accountId: string;
  serverId: string;
  profileId: string;
  /** Changes whenever membership, profile policy, PIN trust, or revocation state changes. */
  authorizationRevision: string;
};

export type ViewerIdentityScope = Omit<ViewerScope, "authorizationRevision">;

export type ViewerResourceScope = ViewerScope & {
  contractRevision: string;
  resource: string;
  parameters?: Record<string, unknown>;
};

export type ProfileTransitionReason = "profile-switch" | "server-switch" | "sign-out" | "profile-revoked" | "authorization-changed";

export type ViewerRuntimeAdapter = {
  /** Installs a synchronous generation/write fence before any asynchronous teardown begins. */
  beginTransition(scope: ViewerScope, reason: ProfileTransitionReason): void | Promise<void>;
  cancelRequests(scope: ViewerScope, reason: ProfileTransitionReason): void | Promise<void>;
  stopPlayback(scope: ViewerScope, reason: ProfileTransitionReason): void | Promise<void>;
  closeRealtime(scope: ViewerScope, reason: ProfileTransitionReason): void | Promise<void>;
  clearOptimisticMutations(scope: ViewerScope): void | Promise<void>;
  clearQueryCaches(scope: ViewerScope): void | Promise<void>;
  clearArtworkState(scope: ViewerScope): void | Promise<void>;
  closeOverlays(scope: ViewerScope): void | Promise<void>;
  clearFocusRestoration(scope: ViewerScope): void | Promise<void>;
  clearProfileLocalState(scope: ViewerScope): void | Promise<void>;
  activateProfile?(scope: ViewerScope): void | Promise<void>;
};

export class ViewerRuntimeTeardownError extends Error {
  readonly failures: { operation: string; reason: unknown }[];

  constructor(failures: { operation: string; reason: unknown }[]) {
    super("Portico could not safely clear the previous viewing profile");
    this.name = "ViewerRuntimeTeardownError";
    this.failures = failures;
  }
}

function boundedIdentity(value: string, name: string): string {
  const normalized = value.trim();
  if (!normalized || normalized.length > 128) throw new TypeError(`${name} is invalid`);
  return normalized;
}

export function normalizeViewerScope(scope: ViewerScope): ViewerScope {
  if (scope.authority !== "hosted" && scope.authority !== "local") throw new TypeError("authority is invalid");
  return {
    authority: scope.authority,
    accountId: boundedIdentity(scope.accountId, "accountId"),
    serverId: boundedIdentity(scope.serverId, "serverId"),
    profileId: boundedIdentity(scope.profileId, "profileId"),
    authorizationRevision: boundedIdentity(scope.authorizationRevision, "authorizationRevision")
  };
}

/**
 * Establishes the current viewing scope from the final server-authenticated
 * identity response. Callers must not substitute user IDs or profile-identity
 * records when any canonical scope field is absent.
 */
export function viewerScopeFromAuthMe(identity: AuthMeResponse): ViewerScope {
  if (!identity.authenticated) throw new TypeError("authenticated viewer identity is required");
  return normalizeViewerScope({
    authority: identity.authority as ViewerScope["authority"],
    accountId: identity.accountId ?? "",
    serverId: identity.serverId ?? "",
    profileId: identity.profileId ?? "",
    authorizationRevision: identity.authorizationRevision ?? ""
  });
}

/** Immediate scope assertion carried by a newly issued native credential family. */
export function viewerScopeFromNativeCredentials(credentials: NativeSessionCredentials): ViewerScope {
  return normalizeViewerScope({
    authority: credentials.authority,
    accountId: credentials.accountId,
    serverId: credentials.serverId,
    profileId: credentials.profileId,
    authorizationRevision: credentials.authorizationRevision
  });
}

/**
 * Fails closed if credential issuance and the final /api/auth/me identity do
 * not describe the same viewer. The /me result remains authoritative because
 * its authorization revision may change after credential issuance.
 */
export function assertViewerScopeMatchesCredentials(
  identity: AuthMeResponse,
  credentials: NativeSessionCredentials
): ViewerScope {
  const scope = viewerScopeFromAuthMe(identity);
  if (!sameViewerIdentity(scope, viewerScopeFromNativeCredentials(credentials))) {
    throw new TypeError("native credentials and authenticated viewer scope do not match");
  }
  return scope;
}

/**
 * Asserts the immutable identity dimensions of a candidate transition. The
 * final server `/api/auth/me` response owns `authorizationRevision`: policy or
 * revocation state may advance after the credential family was issued.
 */
export function assertViewerIdentity(
  actual: ViewerScope,
  expected: ViewerIdentityScope
): ViewerScope {
  const normalized = normalizeViewerScope(actual);
  const expectedIdentity: ViewerIdentityScope = {
    authority: expected.authority,
    accountId: boundedIdentity(expected.accountId, "accountId"),
    serverId: boundedIdentity(expected.serverId, "serverId"),
    profileId: boundedIdentity(expected.profileId, "profileId")
  };
  if (!sameViewerIdentity(normalized, expectedIdentity)) {
    throw new TypeError("authenticated viewer scope does not match the expected authority, account, server, and profile");
  }
  return normalized;
}

function canonicalValue(value: unknown, ancestors = new WeakSet<object>()): unknown {
  if (value === null || typeof value === "string" || typeof value === "boolean") return value;
  if (typeof value === "number") {
    if (!Number.isFinite(value)) throw new TypeError("cache key numbers must be finite");
    return Object.is(value, -0) ? 0 : value;
  }
  if (value === undefined || typeof value === "function" || typeof value === "symbol" || typeof value === "bigint") {
    throw new TypeError("cache key parameters must be JSON-compatible");
  }
  if (typeof value !== "object") throw new TypeError("cache key parameters must be JSON-compatible");
  if (ancestors.has(value)) throw new TypeError("cache key parameters must not contain cycles");
  ancestors.add(value);
  try {
    if (Array.isArray(value)) return value.map(child => canonicalValue(child, ancestors));
    const prototype = Object.getPrototypeOf(value);
    if (prototype !== Object.prototype && prototype !== null) throw new TypeError("cache key objects must be plain JSON objects");
    return Object.fromEntries(
      Object.entries(value as Record<string, unknown>)
        .filter(([, child]) => child !== undefined)
        .sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0)
        .map(([key, child]) => [key, canonicalValue(child, ancestors)])
    );
  } finally {
    ancestors.delete(value);
  }
}

export function viewerCacheKey(input: ViewerResourceScope): string {
  const scope = normalizeViewerScope(input);
  const contractRevision = boundedIdentity(input.contractRevision, "contractRevision");
  const resource = boundedIdentity(input.resource, "resource");
  const parameters = canonicalValue(input.parameters ?? {});
  return JSON.stringify([
    "portico", "v1", scope.authority, scope.accountId, scope.serverId, scope.profileId,
    scope.authorizationRevision, contractRevision, resource, parameters
  ]);
}

export function sameViewerScope(left: ViewerScope, right: ViewerScope): boolean {
  const a = normalizeViewerScope(left);
  const b = normalizeViewerScope(right);
  return a.authority === b.authority
    && a.accountId === b.accountId
    && a.serverId === b.serverId
    && a.profileId === b.profileId
    && a.authorizationRevision === b.authorizationRevision;
}

export function sameViewerIdentity(
  left: ViewerIdentityScope,
  right: ViewerIdentityScope
): boolean {
  return left.authority === right.authority
    && left.accountId === right.accountId
    && left.serverId === right.serverId
    && left.profileId === right.profileId;
}

async function settleOperations(
  operations: readonly [string, () => void | Promise<void>][]
): Promise<{ operation: string; reason: unknown }[]> {
  const results = await Promise.allSettled(operations.map(([, operation]) => operation()));
  return results.flatMap((result, index) => result.status === "rejected"
    ? [{ operation: operations[index][0], reason: result.reason }]
    : []);
}

/**
 * Fences old-profile writes, drains producers, clears retained state, then
 * activates the next profile. A revoked profile is torn down even when its
 * identity strings are unchanged.
 */
export async function transitionViewerRuntime(
  adapter: ViewerRuntimeAdapter,
  from: ViewerScope,
  to: ViewerScope | undefined,
  reason: ProfileTransitionReason = "profile-switch"
): Promise<void> {
  const previous = normalizeViewerScope(from);
  const next = to ? normalizeViewerScope(to) : undefined;
  if (next && sameViewerScope(previous, next) && reason === "profile-switch") return;

  const failures: { operation: string; reason: unknown }[] = [];
  try {
    await adapter.beginTransition(previous, reason);
  } catch (error) {
    failures.push({ operation: "beginTransition", reason: error });
  }

  failures.push(...await settleOperations([
    ["cancelRequests", () => adapter.cancelRequests(previous, reason)],
    ["stopPlayback", () => adapter.stopPlayback(previous, reason)],
    ["closeRealtime", () => adapter.closeRealtime(previous, reason)]
  ]));

  failures.push(...await settleOperations([
    ["clearOptimisticMutations", () => adapter.clearOptimisticMutations(previous)],
    ["clearQueryCaches", () => adapter.clearQueryCaches(previous)],
    ["clearArtworkState", () => adapter.clearArtworkState(previous)],
    ["closeOverlays", () => adapter.closeOverlays(previous)],
    ["clearFocusRestoration", () => adapter.clearFocusRestoration(previous)],
    ["clearProfileLocalState", () => adapter.clearProfileLocalState(previous)]
  ]));

  if (failures.length) throw new ViewerRuntimeTeardownError(failures);
  const activationTarget = reason === "profile-revoked" || reason === "sign-out" ? undefined : next;
  if (activationTarget && adapter.activateProfile) await adapter.activateProfile(activationTarget);
}
