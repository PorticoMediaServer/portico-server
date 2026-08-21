import {
  ApiError,
  createMemorySessionStore,
  isTerminalServerAuthorizationFailure,
  type LocalServerSession,
  type PorticoClient,
  type PorticoServerSessionExchangeRequest,
  type SessionStore
} from "./client.js";
import {
  connectHostedServer,
  assertRouteHealthResponseNotRedirected,
  decodeRouteHealth,
  discoverHostedServerRoute,
  LocalNetworkRouteUnavailableError,
  NearbyRouteAvailableError,
  throwIfConnectionAborted,
  type HostedServerRouteDiscovery,
  type HostedServerConnectorOptions,
  type HostedServerRouteDiscoveryOptions
} from "./hostedServerConnection.js";
import { createHostedConnectionRuntime, type HostedConnectionRuntimeAdapters } from "./hostedConnectionRuntime.js";
import type { AuthMeResponse, HostedServer } from "./types.js";
import { isValidPorticoServerPublicKeyFingerprint, validatePorticoUrl } from "./urlPolicy.js";
import { viewerScopeFromAuthMe, type ViewerScope } from "./viewerScope.js";

export interface VerifiedServerRouteHint {
  url: string;
  type: string;
  address?: string;
  verifiedAt: string;
}

/**
 * Durable, account-scoped knowledge of one trusted Portico Server.
 *
 * Hosted account credentials never belong here. `session` contains only the
 * server-native credential family minted after the Hosted bootstrap exchange.
 */
export interface TrustedServerConnectionRecord {
  schemaVersion: 2;
  accountId: string;
  serverId: string;
  profileId: string;
  serverName: string;
  serverPublicKeyFingerprint: string;
  currentRoute: VerifiedServerRouteHint;
  previousRoute?: VerifiedServerRouteHint;
  session: LocalServerSession;
  lastSuccessfulConnectionAt: string;
  /** Monotonic adapter-owned revision for CAS-capable persistence. */
  mutationVersion?: number;
}

export interface TrustedServerRemovalTombstone {
  schemaVersion: 1;
  accountId: string;
  serverId: string;
  mutationVersion: number;
  removedAt: string;
}

export type TrustedServerPersistencePolicy = "saved-session" | "reauthorize-on-start";

export interface TrustedServerConnectionAdapter {
  /** Resolves only after durable state has been hydrated and is safe to read. */
  ready?(): Promise<void>;
  /**
   * Current persistence health. A healthy adapter reports `durable` even when
   * its explicit restart policy requires reauthorization; `memory-only` is
   * reserved for an unexpected inability to save restart metadata.
   */
  durability?(): "durable" | "memory-only";
  /** Whether a fresh process may restore a saved server session or must remint it. */
  readonly persistencePolicy?: TrustedServerPersistencePolicy;
  list(accountId: string): Promise<TrustedServerConnectionRecord[]>;
  load(accountId: string, serverId: string): Promise<TrustedServerConnectionRecord | undefined>;
  /**
   * Must be atomic. Throw `TrustedServerDurabilityUncertainError` if a partial
   * write or failed compensating rollback means the prior value is uncertain.
   */
  save(record: TrustedServerConnectionRecord): Promise<void>;
  remove(accountId: string, serverId: string): Promise<void>;
  clearAccount(accountId: string): Promise<void>;
  /** Required durable CAS contract. A mismatch must return false without writing. */
  compareAndSwap(expectedVersion: number, record: TrustedServerConnectionRecord): Promise<boolean>;
  /** Durable sign-out fence. Implementations must commit the tombstone atomically with removal. */
  removeWithTombstone(tombstone: TrustedServerRemovalTombstone): Promise<void>;
  loadRemovalTombstone(accountId: string, serverId: string): Promise<TrustedServerRemovalTombstone | undefined>;
}

/**
 * Adds a per-adapter/key serialization boundary. Platform adapters should also
 * implement the durable CAS/tombstone methods for cross-process coordination.
 */
export function createSerializedTrustedServerConnectionAdapter(delegate: TrustedServerConnectionAdapter): TrustedServerConnectionAdapter {
  const queues = new Map<string, Promise<unknown>>();
  const serial = <T>(key: string, operation: () => Promise<T>): Promise<T> => {
    const previous = queues.get(key) ?? Promise.resolve();
    const next = previous.catch(() => undefined).then(operation);
    queues.set(key, next);
    void next.finally(() => { if (queues.get(key) === next) queues.delete(key); }).catch(() => undefined);
    return next;
  };
  return {
    persistencePolicy: delegate.persistencePolicy,
    durability: delegate.durability?.bind(delegate),
    ready: delegate.ready?.bind(delegate),
    list: accountId => serial(`${accountId}\n*`, async () => {
      const records = await delegate.list(accountId);
      const visible: TrustedServerConnectionRecord[] = [];
      for (const record of records) {
        const tombstone = await delegate.loadRemovalTombstone(accountId, record.serverId);
        if (!tombstone || tombstone.mutationVersion < (record.mutationVersion ?? 0)) visible.push(record);
      }
      return visible;
    }),
    load: (accountId, serverId) => serial(`${accountId}\n*`, async () => {
      const [record, tombstone] = await Promise.all([delegate.load(accountId, serverId), delegate.loadRemovalTombstone(accountId, serverId)]);
      return tombstone && (!record?.mutationVersion || tombstone.mutationVersion >= record.mutationVersion) ? undefined : record;
    }),
    save: record => serial(`${record.accountId}\n*`, async () => {
      const current = await delegate.load(record.accountId, record.serverId);
      const tombstone = await delegate.loadRemovalTombstone(record.accountId, record.serverId);
      if (tombstone && (record.mutationVersion === undefined || tombstone.mutationVersion >= record.mutationVersion)) throw new TrustedServerPublicationBlockedError(tombstone);
      const stale = current !== undefined && (record.mutationVersion === undefined || (current.mutationVersion ?? 0) >= record.mutationVersion);
      if (stale) throw new TrustedServerPublicationBlockedError(new Error("The trusted server record is stale."));
      const next = {
        ...(current ?? {}), ...record,
        mutationVersion: Math.max(current?.mutationVersion ?? 0, tombstone?.mutationVersion ?? 0, record.mutationVersion ?? 0) + 1
      } as TrustedServerConnectionRecord;
      const saved = await delegate.compareAndSwap(current?.mutationVersion ?? 0, next);
      if (!saved) throw new TrustedServerPublicationBlockedError(new Error("The trusted server record changed concurrently."));
    }),
    remove: (accountId, serverId) => serial(`${accountId}\n*`, async () => {
      const current = await delegate.load(accountId, serverId);
      const previousTombstone = await delegate.loadRemovalTombstone(accountId, serverId);
      const tombstone: TrustedServerRemovalTombstone = {
        schemaVersion: 1,
        accountId,
        serverId,
        mutationVersion: Math.max(current?.mutationVersion ?? 0, previousTombstone?.mutationVersion ?? 0) + 1,
        removedAt: new Date().toISOString()
      };
      await delegate.removeWithTombstone(tombstone);
    }),
    clearAccount: accountId => serial(`${accountId}\n*`, async () => {
      const records = await delegate.list(accountId);
      for (const record of records) {
        const previousTombstone = await delegate.loadRemovalTombstone(accountId, record.serverId);
        await delegate.removeWithTombstone({
          schemaVersion: 1,
          accountId,
          serverId: record.serverId,
          mutationVersion: Math.max(record.mutationVersion ?? 0, previousTombstone?.mutationVersion ?? 0) + 1,
          removedAt: new Date().toISOString()
        });
      }
    }),
    compareAndSwap: (expectedVersion, record) => serial(`${record.accountId}\n*`, async () => {
      const tombstone = await delegate.loadRemovalTombstone(record.accountId, record.serverId);
      if (tombstone && tombstone.mutationVersion >= (record.mutationVersion ?? 0)) {
        throw new TrustedServerPublicationBlockedError(tombstone);
      }
      return delegate.compareAndSwap(expectedVersion, record);
    }),
    removeWithTombstone: tombstone => serial(`${tombstone.accountId}\n*`, () =>
      delegate.removeWithTombstone(tombstone)),
    loadRemovalTombstone: (accountId, serverId) => serial(`${accountId}\n*`, () =>
      delegate.loadRemovalTombstone(accountId, serverId))
  };
}

export interface TrustedServerConnectorOptions {
  accountId: string;
  connectionAdapter: TrustedServerConnectionAdapter;
  sessionStore: Required<Pick<SessionStore, "set" | "clear">> & Partial<Pick<SessionStore, "get">>;
  createLocalClient(sessionStore: SessionStore): PorticoClient;
  runtime?: HostedConnectionRuntimeAdapters;
  routeProbeFetch?: typeof fetch;
  routeProbeTimeoutMs?: number;
  routePreference?: HostedServerConnectorOptions["routePreference"];
  now?: () => Date;
  /** Cancels a stale server/profile choice before any active publication. */
  signal?: AbortSignal;
  /**
   * Fences and drains the old runtime while the verified candidate remains
   * isolated. `publish` activates the new runtime only after active credential
   * publication and durable-save resolution. `rollback` restores the previous
   * runtime or leaves it fail-closed according to the supplied mode.
   * If staging itself rejects after changing runtime state, the implementation
   * must restore that state or fail-closed before rejecting because no rollback
   * handle has been returned to Client Core yet.
   */
  stageCandidate(candidate: PreparedTrustedServerConnection): Promise<StagedTrustedServerCandidate>;
}

export type CandidateRollbackMode = "restore-previous" | "fail-closed";

export interface StagedTrustedServerCandidate {
  publish(publication?: TrustedServerCandidatePublication): void | Promise<void>;
  /**
   * Synchronously retracts/fences the candidate runtime and every UI binding.
   *
   * Client Core invokes this before the first credential or durable-record
   * compensation await. Implementations must not perform asynchronous work,
   * and must leave both the candidate and previous runtime unable to issue
   * work until `rollback` completes. Repeated calls and escalation from
   * `restore-previous` to `fail-closed` must be safe.
   */
  fenceRollback(mode?: CandidateRollbackMode): void;
  /**
   * Completes a rollback whose synchronous fence has already been installed.
   * In restore mode this is the only callback allowed to republish the
   * previous runtime/UI, after Client Core has restored credentials and any
   * durable record. In fail-closed mode it leaves authenticated UI absent.
   */
  rollback(mode?: CandidateRollbackMode): void | Promise<void>;
}

export interface ResilientHostedServerConnectorOptions extends TrustedServerConnectorOptions {
  hostedClient: HostedServerConnectorOptions["hostedClient"];
  loadTrustedHostedDocumentKeys(): Promise<Record<string, string>>;
  selectionEnvelope: HostedServerConnectorOptions["selectionEnvelope"];
  clientIdentity: Omit<PorticoServerSessionExchangeRequest, "accessToken" | "selectionEnvelope">;
  retryDelaysMs?: number[];
  retryDelay?: (milliseconds: number) => Promise<void>;
  maxParallelRouteProbes?: number;
  routePreference?: HostedServerConnectorOptions["routePreference"];
  rememberLastConnectedServer?: (serverId: string) => void;
  localRouteCandidates?: HostedServerConnectorOptions["localRouteCandidates"];
  /**
   * Ignores remembered route hints for this attempt and verifies a newly
   * signed Hosted route document. The existing trusted record remains the
   * rollback/scope authority until the fresh candidate is fully published.
   */
  forceFreshRouteDiscovery?: boolean;
}

export interface TrustedServerRouteRefreshOptions extends TrustedServerConnectorOptions,
  Omit<HostedServerRouteDiscoveryOptions, "trustedHostedDocumentKeys"> {
  loadTrustedHostedDocumentKeys(): Promise<Record<string, string>>;
}

export interface PreparedTrustedServerConnection {
  identity: AuthMeResponse;
  scope: ViewerScope;
  session: LocalServerSession;
  record: TrustedServerConnectionRecord;
  source: "hosted" | "cached";
}

export interface TrustedServerConnectionResult extends PreparedTrustedServerConnection {
  durability: "durable" | "memory-only";
  persistencePolicy: TrustedServerPersistencePolicy;
  durabilityError?: unknown;
}

/** Final durability facts supplied while the staged runtime still owns rollback. */
export type TrustedServerCandidatePublication = Pick<
  TrustedServerConnectionResult,
  "durability" | "persistencePolicy" | "durabilityError"
>;

/**
 * Raised when the platform-owned viewer runtime rejects a verified candidate.
 * Client Core restores the previous transaction or requests fail-closed
 * rollback before surfacing this error.
 */
export class TrustedServerCandidateActivationError extends Error {
  readonly cause: unknown;

  constructor(cause: unknown) {
    super(cause instanceof Error ? cause.message : "The verified server connection could not be activated.");
    this.name = "TrustedServerCandidateActivationError";
    this.cause = cause;
  }
}

export class TrustedServerCredentialPublicationError extends Error {
  readonly cause: unknown;
  readonly rollbackFailures: readonly unknown[];
  readonly failClosed: boolean;

  constructor(cause: unknown, rollbackFailures: readonly unknown[], failClosed: boolean) {
    super("The verified server credentials could not be published safely.");
    this.name = "TrustedServerCredentialPublicationError";
    this.cause = cause;
    this.rollbackFailures = rollbackFailures;
    this.failClosed = failClosed;
  }
}

/**
 * An adapter must throw this when a durable write may have partially succeeded
 * or when its own compensating rollback did not complete. Ordinary save errors
 * promise that the previous durable value is still intact and may therefore
 * produce a live memory-only connection; this error never may.
 */
export class TrustedServerDurabilityUncertainError extends Error {
  readonly cause: unknown;
  readonly rollbackFailures: readonly unknown[];

  constructor(cause: unknown, rollbackFailures: readonly unknown[] = []) {
    super("The trusted server credential write could not be proven atomic.");
    this.name = "TrustedServerDurabilityUncertainError";
    this.cause = cause;
    this.rollbackFailures = rollbackFailures;
  }
}

/**
 * Raised by a platform adapter when an authorization fence changes while a
 * candidate is being committed (for example, another browser tab signs the
 * account out). This is not degraded persistence health: neither the candidate
 * nor the previous authenticated runtime may remain published.
 */
export class TrustedServerPublicationBlockedError extends Error {
  readonly cause: unknown;

  constructor(cause: unknown) {
    super("Trusted server publication was blocked by the platform security policy.");
    this.name = "TrustedServerPublicationBlockedError";
    this.cause = cause;
  }
}

function isCandidateTransactionFailure(error: unknown): boolean {
  return error instanceof TrustedServerCandidateActivationError
    || error instanceof TrustedServerCredentialPublicationError;
}

interface CandidateConnection {
  identity: AuthMeResponse;
  session: LocalServerSession;
  route: VerifiedServerRouteHint;
  fingerprint: string;
  source: "hosted" | "cached";
}

/**
 * Connects without contacting Hosted Services. Route hints are not authority:
 * each is probed over HTTPS and must return the pinned server ID and public-key
 * fingerprint before the remembered server credential is used.
 */
export async function connectTrustedServerRecord(
  record: TrustedServerConnectionRecord,
  options: Omit<TrustedServerConnectorOptions, "accountId"> & { accountId?: string }
): Promise<TrustedServerConnectionResult> {
  await options.connectionAdapter.ready?.();
  throwIfConnectionAborted(options.signal);
  const accountId = options.accountId ?? record.accountId;
  assertTrustedRecord(record, accountId, record.serverId);
  try {
    const routeScope = options.routePreference === "public-only"
      ? "public"
      : options.routePreference === "lan-only"
        ? "lan"
        : "all";
    const candidate = await cachedCandidate(record, options, routeScope);
    return commitCandidate(record, candidate, { ...options, accountId });
  } catch (error) {
    if (!isCandidateTransactionFailure(error) && isTerminalServerAuthorizationFailure(error)) {
      await removeTrustedServerRecord(options.connectionAdapter, accountId, record.serverId);
    }
    throw error;
  }
}

/**
 * Re-resolves a signed route for an already-authenticated viewer without
 * minting or rotating credentials. The old server session remains live until
 * the freshly verified route proves the same pinned server and viewer scope,
 * then normal staged publication atomically swaps the route.
 */
export async function refreshTrustedServerRoute(
  record: TrustedServerConnectionRecord,
  server: HostedServer,
  options: TrustedServerRouteRefreshOptions
): Promise<TrustedServerConnectionResult> {
  await options.connectionAdapter.ready?.();
  throwIfConnectionAborted(options.signal);
  assertTrustedRecord(record, options.accountId, server.id);
  const trustedHostedDocumentKeys = await options.loadTrustedHostedDocumentKeys();
  throwIfConnectionAborted(options.signal);
  const discovery = await discoverHostedServerRoute(server, {
    hostedClient: options.hostedClient,
    runtime: options.runtime,
    routeProbeFetch: options.routeProbeFetch,
    retryDelaysMs: options.retryDelaysMs,
    retryDelay: options.retryDelay,
    routeProbeTimeoutMs: options.routeProbeTimeoutMs,
    maxParallelRouteProbes: options.maxParallelRouteProbes,
    routePreference: options.routePreference,
    localRouteCandidates: options.localRouteCandidates,
    trustedHostedDocumentKeys,
    now: options.now,
    signal: options.signal
  });
  if (discovery.routeDocument.serverPublicKeyFingerprint !== record.serverPublicKeyFingerprint) {
    throw new TypeError("The refreshed route returned a different pinned server identity.");
  }
  const session: LocalServerSession = {
    ...record.session,
    serverId: record.serverId,
    serverName: record.serverName,
    serverPublicKeyFingerprint: record.serverPublicKeyFingerprint,
    apiBaseUrl: discovery.route.url.replace(/\/+$/, ""),
    routeType: discovery.route.type,
    routeAddress: discovery.route.address
  };
  const temporaryStore = createMemorySessionStore(session);
  const localClient = options.createLocalClient(temporaryStore);
  await localClient.checkServerCompatibility({signal: options.signal});
  throwIfConnectionAborted(options.signal);
  const identity = await localClient.me({signal: options.signal});
  throwIfConnectionAborted(options.signal);
  if (!identity.authenticated || !identity.user) {
    throw new ApiError(401, "server_session_unavailable", "The remembered server session could not be authenticated on the refreshed route.");
  }
  await localClient.checkCompatibility({signal: options.signal});
  throwIfConnectionAborted(options.signal);
  const candidate: CandidateConnection = {
    identity,
    session: temporaryStore.get() ?? session,
    route: {
      url: discovery.route.url.replace(/\/+$/, ""),
      type: discovery.route.type,
      address: discovery.route.address,
      verifiedAt: (options.now?.() ?? new Date()).toISOString()
    },
    fingerprint: record.serverPublicKeyFingerprint,
    source: "cached"
  };
  return commitCandidate(record, candidate, options, server);
}

/**
 * Starts cached direct/LAN authentication and read-only Hosted route discovery
 * together. A cached winner returns without minting a new credential; a live
 * winner mints only after discovery is selected. Failure of either path never
 * erases durable state unless the selected server explicitly reports a scoped
 * revocation.
 */
export async function connectResilientHostedServer(
  server: HostedServer,
  options: ResilientHostedServerConnectorOptions
): Promise<TrustedServerConnectionResult> {
  await options.connectionAdapter.ready?.();
  throwIfConnectionAborted(options.signal);
  const existing = await options.connectionAdapter.load(options.accountId, server.id);
  throwIfConnectionAborted(options.signal);
  if (existing) assertTrustedRecord(existing, options.accountId, server.id);

  const finish = async (candidate: CandidateConnection) => {
    throwIfConnectionAborted(options.signal);
    const result = await commitCandidate(existing, candidate, options, server);
    if (!options.signal?.aborted) {
      try {
        options.rememberLastConnectedServer?.(server.id);
      } catch {
        // Last-connected is a convenience hint, never part of the credential
        // transaction. A preference-store failure cannot demote a live viewer.
      }
    }
    return result;
  };
  const fail = async (error: unknown): Promise<never> => {
    if (isTerminalServerAuthorizationFailure(error)) {
      await removeTrustedServerRecord(options.connectionAdapter, options.accountId, server.id);
    }
    throw error;
  };

  // A remembered server credential is profile-scoped. Never attempt to
  // activate profile A's family while the authoritative Hosted selection is
  // opening profile B; mint and atomically replace it instead. Treating that
  // expected mismatch as a candidate-transaction failure previously made
  // profile switching fail before the live path could run.
  const requestedProfileId = options.selectionEnvelope?.profileId;
  if (existing && requestedProfileId && existing.profileId !== requestedProfileId) {
    try {
      return await finish(await liveCandidate(server, options));
    } catch (error) {
      if (isCandidateTransactionFailure(error)) throw error;
      throwIfConnectionAborted(options.signal);
      return fail(error);
    }
  }

  if (!existing) {
    try {
      return await finish(await liveCandidate(server, options));
    } catch (error) {
      if (isCandidateTransactionFailure(error)) throw error;
      throwIfConnectionAborted(options.signal);
      return fail(error);
    }
  }

  if (options.forceFreshRouteDiscovery) {
    try {
      return await refreshTrustedServerRoute(existing, server, options);
    } catch (error) {
      if (isCandidateTransactionFailure(error)) throw error;
      throwIfConnectionAborted(options.signal);
      return fail(error);
    }
  }

  // Restricted browser modes are deliberately sequential. Automatic Web
  // startup uses public-only; lan-only is reserved for an explicit nearby
  // connection action that may trigger browser Local Network permission.
  if (options.routePreference === "public-only" || options.routePreference === "lan-only") {
    const scope = options.routePreference === "public-only" ? "public" : "lan";
    let cachedError: unknown;
    try {
      return await finish(await cachedCandidate(existing, options, scope));
    } catch (error) {
      if (isCandidateTransactionFailure(error)) throw error;
      throwIfConnectionAborted(options.signal);
      cachedError = error;
    }
    try {
      return await finish(await liveCandidate(server, options));
    } catch (error) {
      if (isCandidateTransactionFailure(error)) throw error;
      throwIfConnectionAborted(options.signal);
      if (options.routePreference === "public-only") {
        const hasRememberedLAN = [existing.currentRoute, existing.previousRoute]
          .some((route) => route !== undefined && cachedRouteIsLAN(route));
        if (hasRememberedLAN && !isTerminalServerAuthorizationFailure(error)) {
          return fail(error instanceof NearbyRouteAvailableError
            ? error
            : new NearbyRouteAvailableError({ cause: error }));
        }
      }
      if (error instanceof NearbyRouteAvailableError || error instanceof LocalNetworkRouteUnavailableError) return fail(error);
      return fail(cachedError ?? error);
    }
  }

  // Public-first remains available to callers that consciously accept
  // automatic LAN fallback after public routes fail.
  if (options.routePreference === "public-first") {
    try {
      return await finish(await cachedCandidate(existing, options, "public"));
    } catch (error) {
      if (isCandidateTransactionFailure(error)) throw error;
      throwIfConnectionAborted(options.signal);
    }
    try {
      // A previously identity-verified LAN route is still useful when Hosted
      // Services is unavailable. Probe every durable rollback candidate before
      // making fresh signed discovery a requirement; public remains first.
      return await finish(await cachedCandidate(existing, options, "lan"));
    } catch (error) {
      if (isCandidateTransactionFailure(error)) throw error;
      throwIfConnectionAborted(options.signal);
    }
    try {
      return await finish(await liveCandidate(server, options));
    } catch (error) {
      if (isCandidateTransactionFailure(error)) throw error;
      throwIfConnectionAborted(options.signal);
      return fail(error);
    }
  }

  // Race cached authentication only against read-only Hosted discovery. A
  // fresh Hosted credential is minted only if discovery wins the race (or the
  // cached path fails), so a successful cached startup cannot leave an unused
  // 180-day credential family behind.
  const cached = cachedCandidate(existing, options).then(
    candidate => ({ kind: "cached" as const, candidate }),
    error => ({ kind: "cached-error" as const, error })
  );
  const discovered = discoverLiveRoute(server, options).then(
    discovery => ({ kind: "discovered" as const, discovery }),
    error => ({ kind: "discovery-error" as const, error })
  );
  const first = await Promise.race([cached, discovered]);
  throwIfConnectionAborted(options.signal);

  if (first.kind === "cached") {
    try {
      return await finish(first.candidate);
    } catch (error) {
      if (isCandidateTransactionFailure(error)) throw error;
      throwIfConnectionAborted(options.signal);
      const live = await discovered;
      throwIfConnectionAborted(options.signal);
      if (live.kind === "discovered") {
        try {
          return await finish(await liveCandidate(server, options, live.discovery));
        } catch (liveError) {
          if (isCandidateTransactionFailure(liveError)) throw liveError;
          throwIfConnectionAborted(options.signal);
          return fail(liveError);
        }
      }
      return fail(error);
    }
  }

  if (first.kind === "discovered") {
    try {
      return await finish(await liveCandidate(server, options, first.discovery));
    } catch (liveError) {
      if (isCandidateTransactionFailure(liveError)) throw liveError;
      throwIfConnectionAborted(options.signal);
      const remembered = await cached;
      throwIfConnectionAborted(options.signal);
      if (remembered.kind === "cached") {
        try {
          return await finish(remembered.candidate);
        } catch (cachedError) {
          if (isCandidateTransactionFailure(cachedError)) throw cachedError;
          throwIfConnectionAborted(options.signal);
          return fail(cachedError);
        }
      }
      return fail(liveError);
    }
  }

  if (first.kind === "cached-error") {
    const live = await discovered;
    throwIfConnectionAborted(options.signal);
    if (live.kind === "discovered") {
      try {
        return await finish(await liveCandidate(server, options, live.discovery));
      } catch (liveError) {
        if (isCandidateTransactionFailure(liveError)) throw liveError;
        throwIfConnectionAborted(options.signal);
        return fail(liveError);
      }
    }
    return fail(first.error);
  }

  const remembered = await cached;
  throwIfConnectionAborted(options.signal);
  if (remembered.kind === "cached") {
    try {
      return await finish(remembered.candidate);
    } catch (error) {
      if (isCandidateTransactionFailure(error)) throw error;
      throwIfConnectionAborted(options.signal);
      return fail(error);
    }
  }
  return fail(first.error);
}

export function createTrustedServerCredentialAdapter(
  accountId: string,
  serverId: string,
  connections: TrustedServerConnectionAdapter
) {
  return {
    load: async () => { await connections.ready?.(); return (await connections.load(accountId, serverId))?.session; },
    save: async (session: LocalServerSession) => {
      await connections.ready?.();
      const current = await connections.load(accountId, serverId);
      if (!current) throw new Error("The trusted server record no longer exists.");
      await connections.save({ ...current, session, mutationVersion: (current.mutationVersion ?? 0) + 1 });
    },
    clear: async () => { await connections.ready?.(); return connections.remove(accountId, serverId); }
  };
}

async function discoverLiveRoute(
  server: HostedServer,
  options: ResilientHostedServerConnectorOptions
): Promise<HostedServerRouteDiscovery> {
  throwIfConnectionAborted(options.signal);
  const trustedHostedDocumentKeys = await options.loadTrustedHostedDocumentKeys();
  throwIfConnectionAborted(options.signal);
  return discoverHostedServerRoute(server, {
    hostedClient: options.hostedClient,
    runtime: options.runtime,
    routeProbeFetch: options.routeProbeFetch,
    retryDelaysMs: options.retryDelaysMs,
    retryDelay: options.retryDelay,
    routeProbeTimeoutMs: options.routeProbeTimeoutMs,
    maxParallelRouteProbes: options.maxParallelRouteProbes,
    routePreference: options.routePreference,
    localRouteCandidates: options.localRouteCandidates,
    trustedHostedDocumentKeys,
    now: options.now,
    signal: options.signal
  });
}

async function liveCandidate(
  server: HostedServer,
  options: ResilientHostedServerConnectorOptions,
  discoveredConnection?: HostedServerRouteDiscovery
): Promise<CandidateConnection> {
  throwIfConnectionAborted(options.signal);
  const temporaryStore = createMemorySessionStore();
  const localClient = options.createLocalClient(temporaryStore);
  // Supplied discovery is still re-verified by connectHostedServer; retain the
  // pinned signing keys for that second verification instead of treating the
  // caller-provided route as self-authenticating.
  const trustedHostedDocumentKeys = await options.loadTrustedHostedDocumentKeys();
  throwIfConnectionAborted(options.signal);
  const identity = await connectHostedServer(server, {
    hostedClient: options.hostedClient,
    localClient,
    sessionStore: temporaryStore as Required<Pick<SessionStore, "set" | "clear">> & Pick<SessionStore, "get">,
    runtime: options.runtime,
    routeProbeFetch: options.routeProbeFetch,
    retryDelaysMs: options.retryDelaysMs,
    retryDelay: options.retryDelay,
    routeProbeTimeoutMs: options.routeProbeTimeoutMs,
    maxParallelRouteProbes: options.maxParallelRouteProbes,
    routePreference: options.routePreference,
    localRouteCandidates: options.localRouteCandidates,
    trustedHostedDocumentKeys,
    discoveredConnection,
    now: options.now,
    selectionEnvelope: options.selectionEnvelope,
    clientIdentity: options.clientIdentity,
    signal: options.signal
  });
  throwIfConnectionAborted(options.signal);
  const session = temporaryStore.get();
  if (!session?.apiBaseUrl || !session.serverPublicKeyFingerprint) {
    throw new Error("The server connection did not create a durable local session.");
  }
  return {
    identity,
    session,
    fingerprint: session.serverPublicKeyFingerprint,
    route: routeHintFromSession(session, options.now?.() ?? new Date()),
    source: "hosted"
  };
}

async function cachedCandidate(
  record: TrustedServerConnectionRecord,
  options: Pick<TrustedServerConnectorOptions, "createLocalClient" | "runtime" | "routeProbeFetch" | "routeProbeTimeoutMs" | "routePreference" | "now" | "signal">,
  routeScope: "all" | "public" | "lan" = "all"
): Promise<CandidateConnection> {
  throwIfConnectionAborted(options.signal);
  const route = await selectCachedVerifiedRoute(record, options, routeScope);
  throwIfConnectionAborted(options.signal);
  const session: LocalServerSession = {
    ...record.session,
    serverId: record.serverId,
    serverName: record.serverName,
    serverPublicKeyFingerprint: record.serverPublicKeyFingerprint,
    apiBaseUrl: route.url.replace(/\/+$/, ""),
    routeType: route.type,
    routeAddress: route.address
  };
  const temporaryStore = createMemorySessionStore(session);
  const localClient = options.createLocalClient(temporaryStore);
  await localClient.checkServerCompatibility({signal: options.signal});
  throwIfConnectionAborted(options.signal);
  const identity = await localClient.me({signal: options.signal});
  throwIfConnectionAborted(options.signal);
  if (!identity.authenticated || !identity.user) {
    throw new ApiError(401, "server_session_unavailable", "The remembered server session could not be authenticated.");
  }
  await localClient.checkCompatibility({signal: options.signal});
  throwIfConnectionAborted(options.signal);
  const current = temporaryStore.get() ?? session;
  return { identity, session: current, route, fingerprint: record.serverPublicKeyFingerprint, source: "cached" };
}

async function selectCachedVerifiedRoute(
  record: TrustedServerConnectionRecord,
  options: Pick<TrustedServerConnectorOptions, "runtime" | "routeProbeFetch" | "routeProbeTimeoutMs" | "routePreference" | "signal">,
  routeScope: "all" | "public" | "lan" = "all"
): Promise<VerifiedServerRouteHint> {
  throwIfConnectionAborted(options.signal);
  const candidates = [record.currentRoute, record.previousRoute]
    .filter((route): route is VerifiedServerRouteHint => Boolean(route))
    .filter((route) => routeScope === "all" || (routeScope === "lan") === cachedRouteIsLAN(route));
  if (candidates.length === 0) throw new Error(`No remembered ${routeScope === "all" ? "" : `${routeScope} `}route could reach this server.`);
  if ((options.routePreference === "public-first" || options.routePreference === "lan-first") && routeScope === "all") {
    const lan = candidates.filter(cachedRouteIsLAN);
    const publicRoutes = candidates.filter((route) => !cachedRouteIsLAN(route));
    const groups = options.routePreference === "lan-first" ? [lan, publicRoutes] : [publicRoutes, lan];
    const errors: unknown[] = [];
    for (const group of groups) {
      if (group.length === 0) continue;
      const results = await Promise.all(group.map(async (route) => {
        try {
          await verifyCachedRoute(route, record, options);
          return { route };
        } catch (error) {
          return { error };
        }
      }));
      throwIfConnectionAborted(options.signal);
      const success = results.find((result) => result.route);
      if (success?.route) return success.route;
      errors.push(...results.flatMap((result) => result.error === undefined ? [] : [result.error]));
    }
    const identityError = errors.find(error => errorMessage(error).includes("identity"));
    throw identityError ?? errors.at(-1) ?? new Error("No remembered route could reach this server.");
  }
  const pending = new Map<number, Promise<{ index: number; route?: VerifiedServerRouteHint; error?: unknown }>>();
  candidates.forEach((route, index) => {
    pending.set(index, verifyCachedRoute(route, record, options).then(
      () => ({ index, route }),
      error => ({ index, error })
    ));
  });
  const errors: unknown[] = [];
  while (pending.size > 0) {
    const result = await Promise.race(pending.values());
    throwIfConnectionAborted(options.signal);
    pending.delete(result.index);
    if (result.route) return result.route;
    errors.push(result.error);
  }
  const identityError = errors.find(error => errorMessage(error).includes("identity"));
  throw identityError ?? errors.at(-1) ?? new Error("No remembered route could reach this server.");
}

function cachedRouteIsLAN(route: VerifiedServerRouteHint): boolean {
  return route.type === "lan" || route.type === "lan_ip_encoded" || route.type === "lan_discovered";
}

async function verifyCachedRoute(
  route: VerifiedServerRouteHint,
  record: TrustedServerConnectionRecord,
  options: Pick<TrustedServerConnectorOptions, "runtime" | "routeProbeFetch" | "routeProbeTimeoutMs" | "signal">
): Promise<void> {
  throwIfConnectionAborted(options.signal);
  const base = validatePorticoUrl(route.url, cachedRouteIsLAN(route) ? "lan-server-route" : "trusted-server-route");
  const runtime = createHostedConnectionRuntime(options.runtime);
  const controller = runtime.createAbortController();
  const cancelForNewerChoice = () => controller.abort();
  options.signal?.addEventListener("abort", cancelForNewerChoice, {once: true});
  const timeout = runtime.setTimeout(() => controller.abort(), Math.max(500, options.routeProbeTimeoutMs ?? 3500));
  let response: Response;
  try {
    response = await (options.routeProbeFetch ?? runtime.fetch)(`${base}/api/remote-access/health`, {
      method: "GET",
      signal: controller.signal,
      redirect: "error",
      credentials: "omit",
      cache: "no-store"
    });
  } finally {
    runtime.clearTimeout(timeout);
    options.signal?.removeEventListener("abort", cancelForNewerChoice);
  }
  throwIfConnectionAborted(options.signal);
  assertRouteHealthResponseNotRedirected(response, base);
  if (!response.ok) throw new Error(`The remembered route health check failed with HTTP ${response.status}.`);
  let health: ReturnType<typeof decodeRouteHealth>;
  try {
    health = decodeRouteHealth(await response.json());
  } catch {
    throw new Error("The remembered route health response is invalid.");
  }
  if (health.serverId !== record.serverId || health.serverPublicKeyFingerprint !== record.serverPublicKeyFingerprint) {
    throw new Error("The remembered route returned a different server identity. The pinned server identity was not changed.");
  }
  if (health.remoteAccessEnabled === false) throw new Error("Remote Access is disabled on this server.");
}

async function commitCandidate(
  existing: TrustedServerConnectionRecord | undefined,
  candidate: CandidateConnection,
  options: TrustedServerConnectorOptions,
  server?: HostedServer
): Promise<TrustedServerConnectionResult> {
  throwIfConnectionAborted(options.signal);
  const now = options.now?.() ?? new Date();
  let scope: ViewerScope;
  try {
    scope = assertCandidateViewerScope(existing, candidate, options, server);
  } catch (error) {
    if (candidate.source === "hosted") await discardUncommittedHostedCandidate(candidate, options);
    throw new TrustedServerCandidateActivationError(error);
  }
  // The credential exchange and the final authenticated /me request are two
  // distinct points in the connection transaction. Policy can legitimately
  // advance between them (for example while Hosted profile state is being
  // reconciled). The final /me response is the verified runtime authority, so
  // publish and persist the credential family with that exact viewer fence.
  // The access and refresh tokens are unchanged; only their client-side scope
  // metadata is brought forward to the identity the server just authenticated.
  candidate = {
    ...candidate,
    session: {
      ...candidate.session,
      authority: scope.authority,
      accountId: scope.accountId,
      serverId: scope.serverId,
      profileId: scope.profileId,
      authorizationRevision: scope.authorizationRevision
    }
  };
  const record = updateRecord(existing, candidate, scope, options.accountId, server, now);
  const prepared: PreparedTrustedServerConnection = {
    identity: candidate.identity,
    scope,
    session: candidate.session,
    record,
    source: candidate.source
  };
  let previousSession: LocalServerSession | undefined;
  try {
    previousSession = options.sessionStore.get?.();
  } catch (error) {
    if (candidate.source === "hosted") await discardUncommittedHostedCandidate(candidate, options);
    throw new TrustedServerCredentialPublicationError(error, [], true);
  }

  let staged: StagedTrustedServerCandidate;
  try {
    staged = await options.stageCandidate(prepared);
  } catch (error) {
    if (candidate.source === "hosted") await discardUncommittedHostedCandidate(candidate, options);
    throw new TrustedServerCandidateActivationError(error);
  }

  try {
    throwIfConnectionAborted(options.signal);
  } catch (error) {
    const rollback = await rollbackCandidateTransaction({
      staged,
      previousSession,
      existing,
      candidate,
      durableRecord: record,
      options,
      activePublicationAttempted: false,
      durableCommitted: false
    });
    if (rollback.failures.length) {
      throw new TrustedServerCredentialPublicationError(
        error,
        rollback.failures,
        rollback.mode === "fail-closed"
      );
    }
    throw error;
  }

  try {
    options.sessionStore.set(candidate.session);
  } catch (error) {
    const rollback = await rollbackCandidateTransaction({
      staged,
      previousSession,
      existing,
      candidate,
      durableRecord: record,
      options,
      activePublicationAttempted: true,
      durableCommitted: false
    });
    throw new TrustedServerCredentialPublicationError(error, rollback.failures, rollback.mode === "fail-closed");
  }

  let durability: TrustedServerConnectionResult["durability"] = "durable";
  const persistencePolicy = options.connectionAdapter.persistencePolicy ?? "saved-session";
  let durabilityError: unknown;
  let durableCommitted = false;
  try {
    await options.connectionAdapter.save(record);
    durableCommitted = true;
    if (options.connectionAdapter.durability?.() === "memory-only") {
      durability = "memory-only";
    }
  } catch (error) {
    if (error instanceof TrustedServerPublicationBlockedError) {
      const rollback = await rollbackCandidateTransaction({
        staged,
        previousSession,
        existing,
        candidate,
        durableRecord: record,
        options,
        activePublicationAttempted: true,
        durableCommitted: false,
        forceFailClosed: true
      });
      throw new TrustedServerCredentialPublicationError(error, rollback.failures, true);
    }
    if (error instanceof TrustedServerDurabilityUncertainError || error instanceof AggregateError) {
      const rollback = await rollbackCandidateTransaction({
        staged,
        previousSession,
        existing,
        candidate,
        durableRecord: record,
        options,
        activePublicationAttempted: true,
        // An uncertain write must be treated as potentially committed so the
        // previous durable value is restored or every candidate copy removed.
        durableCommitted: true
      });
      const adapterFailures = error instanceof TrustedServerDurabilityUncertainError
        ? error.rollbackFailures
        : [];
      throw new TrustedServerCredentialPublicationError(
        error,
        [...adapterFailures, ...rollback.failures],
        rollback.mode === "fail-closed"
      );
    }
    // TrustedServerConnectionAdapter.save is required to be atomic. Its failure
    // is the sole condition that may keep the verified candidate live only in
    // memory; active credential publication failures are always fatal.
    durability = "memory-only";
    durabilityError = error;
  }

  try {
    throwIfConnectionAborted(options.signal);
  } catch (error) {
    const rollback = await rollbackCandidateTransaction({
      staged,
      previousSession,
      existing,
      candidate,
      durableRecord: record,
      options,
      activePublicationAttempted: true,
      durableCommitted
    });
    if (rollback.failures.length) {
      throw new TrustedServerCredentialPublicationError(
        error,
        rollback.failures,
        rollback.mode === "fail-closed"
      );
    }
    throw error;
  }

  try {
    await staged.publish({durability, persistencePolicy, durabilityError});
    throwIfConnectionAborted(options.signal);
  } catch (error) {
    if (error instanceof TrustedServerPublicationBlockedError) {
      const rollback = await rollbackCandidateTransaction({
        staged,
        previousSession,
        existing,
        candidate,
        durableRecord: record,
        options,
        activePublicationAttempted: true,
        durableCommitted,
        forceFailClosed: true
      });
      throw new TrustedServerCredentialPublicationError(error, rollback.failures, true);
    }
    const rollback = await rollbackCandidateTransaction({
      staged,
      previousSession,
      existing,
      candidate,
      durableRecord: record,
      options,
      activePublicationAttempted: true,
      durableCommitted
    });
    const cause = rollback.failures.length
      ? new AggregateError([error, ...rollback.failures], "Candidate publication and rollback failed.")
      : error;
    throw new TrustedServerCandidateActivationError(cause);
  }

  return durability === "durable"
    ? { ...prepared, durability, persistencePolicy }
    : { ...prepared, durability, persistencePolicy, durabilityError };
}

type CandidateTransactionRollback = {
  staged: StagedTrustedServerCandidate;
  previousSession: LocalServerSession | undefined;
  existing: TrustedServerConnectionRecord | undefined;
  candidate: CandidateConnection;
  durableRecord: TrustedServerConnectionRecord;
  options: TrustedServerConnectorOptions;
  activePublicationAttempted: boolean;
  durableCommitted: boolean;
  forceFailClosed?: boolean;
};

async function rollbackCandidateTransaction(input: CandidateTransactionRollback): Promise<{
  mode: CandidateRollbackMode;
  failures: unknown[];
}> {
  const failures: unknown[] = [];
  let mode: CandidateRollbackMode = input.forceFailClosed ? "fail-closed" : "restore-previous";

  // This is intentionally the first rollback operation and is synchronous.
  // Never restore A's credentials while B's runtime/UI can still execute: a
  // platform adapter must first retract B and keep both runtimes fenced until
  // the completion callback below.
  try {
    input.staged.fenceRollback(mode);
  } catch (error) {
    failures.push(error);
    mode = "fail-closed";
    try {
      input.staged.fenceRollback("fail-closed");
    } catch (failClosedFenceError) {
      failures.push(failClosedFenceError);
    }
  }

  if (input.activePublicationAttempted) {
    if (mode === "fail-closed") {
      const failClosed = await Promise.all([
        settleOperation(() => input.options.sessionStore.clear()),
        settleOperation(() => removeTrustedServerRecord(
          input.options.connectionAdapter,
          input.options.accountId,
          input.candidate.session.serverId ?? input.durableRecord.serverId
        ))
      ]);
      failures.push(...failClosed.flatMap(failure => failure === undefined ? [] : [failure]));
    } else {
      const activeRestore = await settleOperation(input.previousSession
        ? () => input.options.sessionStore.set(input.previousSession!)
        : () => input.options.sessionStore.clear());
      if (activeRestore !== undefined) failures.push(activeRestore);

      if (input.durableCommitted) {
        const durableRestore = await settleOperation(input.existing
          ? () => restoreTrustedServerRecord(input)
          : () => removeTrustedServerRecord(
            input.options.connectionAdapter,
            input.options.accountId,
            input.candidate.session.serverId ?? input.durableRecord.serverId
          ));
        if (durableRestore !== undefined) failures.push(durableRestore);
      }

      if (failures.length) {
        mode = "fail-closed";
        try {
          input.staged.fenceRollback("fail-closed");
        } catch (error) {
          failures.push(error);
        }
        const failClosed = await Promise.all([
          settleOperation(() => input.options.sessionStore.clear()),
          settleOperation(() => removeTrustedServerRecord(
            input.options.connectionAdapter,
            input.options.accountId,
            input.candidate.session.serverId ?? input.durableRecord.serverId
          ))
        ]);
        failures.push(...failClosed.flatMap(failure => failure === undefined ? [] : [failure]));
      }
    }
  } else if (mode === "fail-closed") {
    // A synchronous platform-fence failure is itself security-critical even
    // when B's credentials were never published. Remove the still-active A
    // family and its restart record rather than leaving credentials usable by
    // a runtime whose state could not be proven fenced.
    const failClosed = await Promise.all([
      settleOperation(() => input.options.sessionStore.clear()),
      settleOperation(() => removeTrustedServerRecord(
        input.options.connectionAdapter,
        input.options.accountId,
        input.candidate.session.serverId ?? input.existing?.serverId ?? input.durableRecord.serverId
      ))
    ]);
    failures.push(...failClosed.flatMap(failure => failure === undefined ? [] : [failure]));
  }

  const runtimeRollback = await settleOperation(() => input.staged.rollback(mode));
  if (runtimeRollback !== undefined) {
    failures.push(runtimeRollback);
    if (mode === "restore-previous") {
      mode = "fail-closed";
      try {
        input.staged.fenceRollback("fail-closed");
      } catch (error) {
        failures.push(error);
      }
      const failClosed = await Promise.all([
        settleOperation(() => input.options.sessionStore.clear()),
        settleOperation(() => removeTrustedServerRecord(
          input.options.connectionAdapter,
          input.options.accountId,
          input.candidate.session.serverId ?? input.durableRecord.serverId
        ))
      ]);
      failures.push(...failClosed.flatMap(failure => failure === undefined ? [] : [failure]));
      const failClosedRuntime = await settleOperation(() => input.staged.rollback("fail-closed"));
      if (failClosedRuntime !== undefined) failures.push(failClosedRuntime);
    }
  }
  if (input.candidate.source === "hosted") await discardUncommittedHostedCandidate(input.candidate, input.options);
  return {mode, failures};
}

async function settleOperation(operation: () => void | Promise<void>): Promise<unknown | undefined> {
  try {
    await operation();
    return undefined;
  } catch (error) {
    return error;
  }
}

async function removeTrustedServerRecord(
  adapter: TrustedServerConnectionAdapter,
  accountId: string,
  serverId: string
): Promise<void> {
  const current = await adapter.load(accountId, serverId);
  const previousTombstone = typeof adapter.loadRemovalTombstone === "function"
    ? await adapter.loadRemovalTombstone(accountId, serverId)
    : undefined;
  const tombstone: TrustedServerRemovalTombstone = {
    schemaVersion: 1,
    accountId,
    serverId,
    mutationVersion: Math.max(current?.mutationVersion ?? 0, previousTombstone?.mutationVersion ?? 0) + 1,
    removedAt: new Date().toISOString()
  };
  if (typeof adapter.removeWithTombstone === "function") {
    await adapter.removeWithTombstone(tombstone);
    return;
  }
  // Older in-memory test/platform adapters can still be cleared, but durable
  // adapters must implement removeWithTombstone to satisfy the interface.
  await adapter.remove(accountId, serverId);
}

async function restoreTrustedServerRecord(input: CandidateTransactionRollback): Promise<void> {
  const serverId = input.durableRecord.serverId;
  const current = await input.options.connectionAdapter.load(input.options.accountId, serverId);
  if (!current || !samePublishedTrustedRecord(current, input.durableRecord)) {
    throw new TrustedServerPublicationBlockedError(new Error("The trusted server record changed while rollback was in progress."));
  }
  if (typeof input.options.connectionAdapter.compareAndSwap === "function") {
    const restored = await input.options.connectionAdapter.compareAndSwap(current.mutationVersion ?? 0, input.existing!);
    if (!restored) throw new TrustedServerPublicationBlockedError(new Error("The trusted server record changed while rollback was in progress."));
    return;
  }
  // Compatibility adapters without CAS can only restore after the full
  // published-value comparison above. Preserve the exact prior record,
  // including an absent mutationVersion.
  await input.options.connectionAdapter.save(input.existing!);
}

function samePublishedTrustedRecord(left: TrustedServerConnectionRecord, right: TrustedServerConnectionRecord): boolean {
  return left.accountId === right.accountId && left.serverId === right.serverId &&
    left.profileId === right.profileId && left.serverPublicKeyFingerprint === right.serverPublicKeyFingerprint &&
    left.currentRoute.url === right.currentRoute.url &&
    left.session.accessToken === right.session.accessToken &&
    left.session.refreshToken === right.session.refreshToken;
}

async function discardUncommittedHostedCandidate(
  candidate: CandidateConnection,
  options: Pick<TrustedServerConnectorOptions, "createLocalClient">
): Promise<void> {
  if (!candidate.session.refreshToken) return;
  const temporaryStore = createMemorySessionStore(candidate.session);
  try {
    // Start revocation, but never let an abort-ignoring server hold the
    // selection queue. localRequest captures the authorization header before
    // yielding, so the temporary credential can be erased immediately while
    // the best-effort network operation settles in the background.
    const revocation = options.createLocalClient(temporaryStore).revokeNativeSession(candidate.session.refreshToken);
    temporaryStore.clear?.();
    void revocation.catch(() => undefined);
  } catch {
    // The activation error is authoritative. Revocation is best effort because
    // the candidate was never published and its access credential is short lived.
    try { temporaryStore.clear?.(); } catch { /* The temporary store is process-local and discarded here. */ }
  }
}

function updateRecord(
  existing: TrustedServerConnectionRecord | undefined,
  candidate: CandidateConnection,
  scope: ViewerScope,
  accountId: string,
  server: HostedServer | undefined,
  now: Date
): TrustedServerConnectionRecord {
  const sameRoute = existing?.currentRoute.url === candidate.route.url;
  const identityChanged = Boolean(existing && existing.serverPublicKeyFingerprint !== candidate.fingerprint);
  return {
    schemaVersion: 2,
    accountId,
    serverId: candidate.session.serverId ?? existing?.serverId ?? server?.id ?? "",
    profileId: scope.profileId,
    serverName: candidate.session.serverName ?? existing?.serverName ?? server?.name ?? "Portico Server",
    serverPublicKeyFingerprint: candidate.fingerprint,
    currentRoute: { ...candidate.route, verifiedAt: now.toISOString() },
    previousRoute: identityChanged ? undefined : sameRoute ? existing?.previousRoute : existing?.currentRoute,
    session: candidate.session,
    lastSuccessfulConnectionAt: now.toISOString()
    ,mutationVersion: (existing?.mutationVersion ?? 0) + 1
  };
}

function assertCandidateViewerScope(
  existing: TrustedServerConnectionRecord | undefined,
  candidate: CandidateConnection,
  options: TrustedServerConnectorOptions,
  server?: HostedServer
): ViewerScope {
  const scope = viewerScopeFromAuthMe(candidate.identity);
  const expectedServerId = server?.id ?? existing?.serverId ?? candidate.session.serverId;
  const expectedProfileId = "selectionEnvelope" in options
    ? (options as ResilientHostedServerConnectorOptions).selectionEnvelope.profileId
    : existing?.profileId;
  if (
    scope.authority !== "hosted" ||
    scope.accountId !== options.accountId ||
    !expectedServerId ||
    scope.serverId !== expectedServerId ||
    !expectedProfileId ||
    scope.profileId !== expectedProfileId
  ) {
    throw new TypeError("The authenticated viewer scope does not match the selected Hosted account, server, and profile.");
  }
  if (candidate.session.serverId && candidate.session.serverId !== scope.serverId) {
    throw new TypeError("The server credential and authenticated viewer scope do not match.");
  }
  return scope;
}

function routeHintFromSession(session: LocalServerSession, now: Date): VerifiedServerRouteHint {
  return {
    url: session.apiBaseUrl!,
    type: session.routeType ?? "direct",
    address: session.routeAddress,
    verifiedAt: now.toISOString()
  };
}

function assertTrustedRecord(record: TrustedServerConnectionRecord, accountId: string, serverId: string): void {
  if (record.schemaVersion !== 2 || record.accountId !== accountId || record.serverId !== serverId) {
    throw new Error("The remembered server connection does not belong to this account and server.");
  }
  if (!record.accountId.trim() || !record.serverId.trim() || !record.profileId.trim() ||
      !isValidPorticoServerPublicKeyFingerprint(record.serverPublicKeyFingerprint) ||
      !record.currentRoute?.url || !record.session) {
    throw new Error("The remembered server connection is incomplete.");
  }
  assertTrustedRouteHint(record.currentRoute, record.serverPublicKeyFingerprint);
  if (record.previousRoute) assertTrustedRouteHint(record.previousRoute, record.serverPublicKeyFingerprint);
  if (record.session.authority !== "hosted" || record.session.accountId !== record.accountId ||
      record.session.serverId !== record.serverId || record.session.profileId !== record.profileId ||
      record.session.serverPublicKeyFingerprint !== record.serverPublicKeyFingerprint ||
      !record.session.accessToken?.trim() || !record.session.refreshToken?.trim() || record.session.bootstrapAccessToken?.trim()) {
    throw new Error("The remembered server credential is not bound to its trusted server record.");
  }
  const sessionPurpose = cachedRouteIsLAN(record.currentRoute) ? "lan-server-route" : "trusted-server-route";
  const sessionURL = validatePorticoUrl(record.session.apiBaseUrl ?? "", sessionPurpose);
  if (sessionURL !== validatePorticoUrl(record.currentRoute.url, sessionPurpose)) {
    throw new Error("The remembered server credential is not bound to its current trusted route.");
  }
}

function assertTrustedRouteHint(route: VerifiedServerRouteHint, fingerprint: string): void {
  if (!route.type?.trim() || !route.verifiedAt || !Number.isFinite(Date.parse(route.verifiedAt))) {
    throw new Error("The remembered server route hint is incomplete.");
  }
  const purpose = cachedRouteIsLAN(route) ? "lan-server-route" : "trusted-server-route";
  validatePorticoUrl(route.url, purpose);
  if (!isValidPorticoServerPublicKeyFingerprint(fingerprint)) {
    throw new Error("The remembered server fingerprint is invalid.");
  }
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message.toLowerCase() : String(error).toLowerCase();
}
