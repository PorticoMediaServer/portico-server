import type { AuthMeResponse, HostedProfileSelectionEnvelope, HostedRouteDocument, HostedRouteEntry, HostedServer, PorticoDocumentSigningKeySet } from "./types.js";
import type { CredentialAdapter, HostedServicesClient, LocalServerSession, PorticoClient, SessionStore } from "./client.js";
import { ApiError, parseRetryAfter, PorticoTransportError } from "./client.js";
import { productMessage, resolveProductProblem } from "./productLanguage.js";
import { positiveFullJitterDelay } from "./retryScheduling.js";
import {
  assertViewerIdentity,
  assertViewerScopeMatchesCredentials,
  viewerScopeFromNativeCredentials
} from "./viewerScope.js";
import {
  createHostedConnectionRuntime,
  HostedRuntimeCapabilityError,
  type HostedConnectionRuntimeAdapters,
  type ResolvedHostedConnectionRuntimeAdapters
} from "./hostedConnectionRuntime.js";
import {
  isValidPorticoServerPublicKeyFingerprint,
  porticoRouteTransport,
  validatePorticoUrl
} from "./urlPolicy.js";

export type HostedRoutePreference = "lan-first" | "public-first" | "public-only" | "lan-only";

export class NearbyRouteAvailableError extends Error {
  constructor(options?: ErrorOptions) {
    super("The server's public routes are unavailable, but a nearby route can be tried with the user's permission.", options);
    this.name = "NearbyRouteAvailableError";
  }
}

export class LocalNetworkRouteUnavailableError extends Error {
  constructor(options?: ErrorOptions) {
    super("The browser could not reach the server on the local network after every public route failed.", options);
    this.name = "LocalNetworkRouteUnavailableError";
  }
}

export interface HostedServerConnectorOptions {
  hostedClient: HostedServicesClient;
  localClient: PorticoClient;
  sessionStore: Required<Pick<SessionStore, "set" | "clear">> & Partial<Pick<SessionStore, "get">>;
  credentialAdapter?: Required<Pick<CredentialAdapter, "save" | "clear">>;
  runtime?: HostedConnectionRuntimeAdapters;
  /** Optional transport override used only for unauthenticated route-health probes. */
  routeProbeFetch?: typeof fetch;
  retryDelaysMs?: number[];
  retryDelay?: (milliseconds: number) => Promise<void>;
  /** Persisted installation identifier used only to spread route retries. */
  retryCohort?: string;
  /** Absolute Unix-millisecond deadline for the complete Hosted discovery. */
  discoveryDeadlineAt?: number;
  routeProbeTimeoutMs?: number;
  maxParallelRouteProbes?: number;
  /** Clients default to LAN first and fall back to the verified public route without changing account or viewer authority. */
  routePreference?: HostedRoutePreference;
  rememberLastConnectedServer?: (serverId: string) => void;
  localRouteCandidates?: (server: HostedServer, document: HostedRouteDocument, signal?: AbortSignal) => HostedRouteEntry[] | Promise<HostedRouteEntry[]>;
  trustedHostedDocumentKeys: Record<string, string>;
  /** Explicit, PIN-verified profile selection bound to this server and Hosted device session. */
  selectionEnvelope: HostedProfileSelectionEnvelope;
  /** Exchanges the Hosted bootstrap token for server-local profile credentials. */
  clientIdentity: Omit<import("./client.js").PorticoServerSessionExchangeRequest, "accessToken" | "selectionEnvelope">;
  /** A read-only route discovery result selected before credential minting. */
  discoveredConnection?: HostedServerRouteDiscovery;
  now?: () => Date;
  /** Cancels stale server/profile selections before credential publication. */
  signal?: AbortSignal;
}

export interface HostedServerRouteDiscovery {
  routeDocument: HostedRouteDocument;
  route: HostedRouteEntry;
}

export interface HostedRouteHealthEvidence {
  serverId: string;
  serverPublicKeyFingerprint: string;
  remoteAccessEnabled: boolean;
}

export type HostedServerRouteDiscoveryOptions = Pick<HostedServerConnectorOptions,
  "hostedClient" | "runtime" | "routeProbeFetch" | "retryDelaysMs" | "retryDelay" |
  "routeProbeTimeoutMs" | "maxParallelRouteProbes" | "routePreference" |
  "localRouteCandidates" | "trustedHostedDocumentKeys" | "now" | "signal" |
  "retryCohort" | "discoveryDeadlineAt">;

async function validateSelectedHostedDiscovery(
  server: HostedServer,
  discovery: HostedServerRouteDiscovery,
  options: HostedServerRouteDiscoveryOptions
): Promise<void> {
  const runtime = createHostedConnectionRuntime(options.runtime);
  await verifyHostedRouteDocument(
    discovery.routeDocument,
    server.id,
    options.trustedHostedDocumentKeys,
    options.now?.() ?? runtime.now(),
    runtime
  );
  if (discovery.routeDocument.serverPublicKey !== server.serverPublicKey ||
      discovery.routeDocument.serverPublicKeyFingerprint !== server.serverPublicKeyFingerprint) {
    throw new Error("The selected Hosted route does not match the server identity.");
  }
  const route = discovery.route;
  const purpose = isLANRoute(route) ? "lan-server-route" : "trusted-server-route";
  const normalizedRoute = validatePorticoUrl(route.url, purpose);
	if (!routeIsUsableCandidate(route)) throw new Error("The selected route is not currently usable.");
	if (!isLANRoute(route) && !discovery.routeDocument.routes.some((candidate) =>
		candidate.type === route.type &&
		candidate.quality === route.quality &&
		routeIsUsableCandidate(candidate) &&
		validateRouteURL(candidate, false) === normalizedRoute
	)) {
    throw new Error("The selected public route was not issued by the signed route document.");
  }
}

function validateRouteURL(route: HostedRouteEntry, lan: boolean): string | undefined {
  try {
    return validatePorticoUrl(route.url, lan ? "lan-server-route" : "trusted-server-route");
  } catch {
    return undefined;
  }
}

const defaultHostedConnectionRetryDelaysMs = [2500, 5000];
const defaultHostedRouteDiscoveryTimeoutMs = 30_000;
const hostedRouteFallbackDelayMs = 150;
const maxPendingRouteFailureReports = 8;
const routeFailureReportCooldownMs = 30_000;
const maxRememberedRouteFailureReports = 128;
let pendingRouteFailureReports = 0;
const recentRouteFailureReports = new Map<string, number>();

export async function connectHostedServer(server: HostedServer, options: HostedServerConnectorOptions): Promise<AuthMeResponse> {
  throwIfConnectionAborted(options.signal);
  if (server.preferredAuthMode === "local") {
    throw new Error("This server uses This Server sign-in and cannot be opened from a hosted Portico client.");
  }
  if (options.selectionEnvelope.serverId !== server.id ||
      !options.selectionEnvelope.accountId?.trim() ||
      !options.selectionEnvelope.profileId?.trim()) {
    throw new Error("The selected profile is not bound to this server. Choose the profile again.");
  }
  const discoveredConnection = options.discoveredConnection;
  if (discoveredConnection) await validateSelectedHostedDiscovery(server, discoveredConnection, options);
  const { routeDocument, route } = discoveredConnection ?? await discoverHostedServerRoute(server, {
    ...options,
    retryCohort: options.retryCohort?.trim() || options.clientIdentity.installationId,
  });
  throwIfConnectionAborted(options.signal);
  const session: LocalServerSession = {
    serverId: server.id,
    serverName: server.name,
		apiBaseUrl: route.url.replace(/\/+$/, ""),
		serverPublicKey: routeDocument.serverPublicKey,
		serverPublicKeyFingerprint: routeDocument.serverPublicKeyFingerprint,
    routeType: route.type,
    routeAddress: route.address
  };
  const previousSession = options.sessionStore.get?.();
  let candidateInstallationAttempted = false;
  let issuedCredentials: Awaited<ReturnType<PorticoClient["acceptPorticoSessionCredentials"]>> | undefined;
  try {
    candidateInstallationAttempted = true;
    options.sessionStore.set(session);
    throwIfConnectionAborted(options.signal);
    // The public system revision is checked before authenticated bootstrap so a
    // server/client mismatch is not misreported as an account or invite error.
    await options.localClient.checkServerCompatibility({signal: options.signal});
    throwIfConnectionAborted(options.signal);
		const attachment = await preparePorticoAttachmentHandshake(options.localClient, routeDocument, route, options);
		throwIfConnectionAborted(options.signal);
		let porticoSession;
		try {
			porticoSession = await options.hostedClient.porticoSession(
				server.id,
				{selectionEnvelope: options.selectionEnvelope},
				{signal: options.signal}
			);
		} catch (error) {
			throwIfConnectionAborted(options.signal);
			if (error instanceof ApiError) throw error;
			throw new Error(porticoSessionErrorMessage(error), { cause: error });
		}
		throwIfConnectionAborted(options.signal);
		issuedCredentials = await exchangePorticoAttachment(options.localClient, attachment, {
      ...options.clientIdentity,
      accessToken: porticoSession.accessToken,
      selectionEnvelope: options.selectionEnvelope
    }, options.signal);
    const expectedIdentity = {
      authority: "hosted" as const,
      accountId: options.selectionEnvelope.accountId,
      serverId: server.id,
      profileId: options.selectionEnvelope.profileId
    };
    assertViewerIdentity(viewerScopeFromNativeCredentials(issuedCredentials), expectedIdentity);
    throwIfConnectionAborted(options.signal);
    const localAuth = await options.localClient.me({signal: options.signal});
    if (!localAuth.authenticated || !localAuth.user) {
      throw new Error("The server was reached, but this Portico Account does not have access on this server yet. Ask the owner to verify the invite or refresh server memberships.");
    }
    assertViewerIdentity(assertViewerScopeMatchesCredentials(localAuth, issuedCredentials), expectedIdentity);
    throwIfConnectionAborted(options.signal);
    // Product Contract is authenticated. Check it only after access is proven,
    // preserving the existing auth lifecycle and its actionable error.
    await options.localClient.checkCompatibility({signal: options.signal});
    throwIfConnectionAborted(options.signal);
    const storedSession = options.sessionStore.get?.();
    const bootstrappedSession: LocalServerSession = {
      ...(storedSession ?? session),
      serverId: session.serverId,
      serverName: session.serverName,
      apiBaseUrl: session.apiBaseUrl,
      serverPublicKey: session.serverPublicKey,
      serverPublicKeyFingerprint: session.serverPublicKeyFingerprint,
      routeType: session.routeType,
      routeAddress: session.routeAddress,
      bootstrapAccessToken: undefined,
      ...(storedSession ? {
        accessToken: issuedCredentials.accessToken,
        refreshToken: issuedCredentials.refreshToken,
        expiresAt: issuedCredentials.accessExpiresAt,
        refreshExpiresAt: issuedCredentials.refreshExpiresAt,
        authority: issuedCredentials.authority,
        accountId: issuedCredentials.accountId,
        profileId: issuedCredentials.profileId,
        authorizationRevision: issuedCredentials.authorizationRevision
      } : {})
    };
    options.sessionStore.set?.(bootstrappedSession);
    assertPublishedServerSession(bootstrappedSession, session, routeDocument, route, options.selectionEnvelope);
		// The Cloud grant remains function-local and is never installed in a
		// SessionStore. Only the encrypted, server-local credential family is saved.
    await options.credentialAdapter?.save(bootstrappedSession);
    throwIfConnectionAborted(options.signal);
    try {
      options.rememberLastConnectedServer?.(server.id);
    } catch {
      // Last-connected is only a convenience hint and is outside credential
      // publication and durability.
    }
    return localAuth;
  } catch (error) {
    // Start the remote cleanup while the candidate is still available so the
    // request can capture its authorization header, but never await a server
    // that ignores cancellation. Local rollback is the security boundary and
    // must run immediately; late revocation settlement is always consumed.
    if (issuedCredentials?.refreshToken) {
      try {
        void options.localClient.revokeNativeSession(issuedCredentials.refreshToken).catch(() => undefined);
      } catch {
        // Synchronous producer failure is equivalent to an unreachable server.
      }
    }
    if (candidateInstallationAttempted) {
      const rollbackFailures = await restoreConnectorSession(previousSession, options);
      if (rollbackFailures.length) {
        throw new AggregateError([error, ...rollbackFailures], "Portico could not restore or safely clear the previous server credentials.");
      }
    }
    throw error;
  }
}

interface PreparedPorticoAttachment {
  runtime: ResolvedHostedConnectionRuntimeAdapters;
  agreement: Awaited<ReturnType<ResolvedHostedConnectionRuntimeAdapters["createAttachmentKeyAgreement"]>>;
  handshake: Awaited<ReturnType<PorticoClient["createPorticoAttachmentHandshake"]>>;
  transcript: Uint8Array;
  serverEphemeralPublicKey: Uint8Array;
}

function assertPublishedServerSession(
  session: LocalServerSession,
  expectedSession: LocalServerSession,
  routeDocument: HostedRouteDocument,
  route: HostedRouteEntry,
  selectionEnvelope: HostedProfileSelectionEnvelope
): void {
  const purpose = isLANRoute(route) ? "lan-server-route" : "trusted-server-route";
  const expectedURL = validatePorticoUrl(expectedSession.apiBaseUrl ?? route.url, purpose);
  const actualURL = validatePorticoUrl(session.apiBaseUrl ?? "", purpose);
  const expectedTransport = porticoRouteTransport(expectedURL, purpose);
  const actualTransport = porticoRouteTransport(actualURL, purpose);
  if (session.serverId !== expectedSession.serverId || actualURL !== expectedURL || expectedTransport !== actualTransport ||
      session.serverPublicKey !== routeDocument.serverPublicKey ||
      session.serverPublicKeyFingerprint !== routeDocument.serverPublicKeyFingerprint ||
      session.routeType && session.routeType !== route.type ||
      session.bootstrapAccessToken?.trim()) {
    throw new Error("The selected server changed while its connection was being verified.");
  }
  const hasCredentialMetadata = session.accessToken !== undefined || session.refreshToken !== undefined ||
    session.authority !== undefined || session.accountId !== undefined || session.profileId !== undefined;
  if (hasCredentialMetadata && (session.authority !== "hosted" || session.accountId !== selectionEnvelope.accountId ||
      session.serverId !== selectionEnvelope.serverId || session.profileId !== selectionEnvelope.profileId ||
      !session.accessToken?.trim() || !session.refreshToken?.trim())) {
    throw new Error("The server credential is not bound to the selected Hosted account, server, and profile.");
  }
}

async function preparePorticoAttachmentHandshake(
  localClient: PorticoClient,
  routeDocument: HostedRouteDocument,
  route: HostedRouteEntry,
  options: HostedServerConnectorOptions
): Promise<PreparedPorticoAttachment> {
  const runtime = createHostedConnectionRuntime(options.runtime);
  const agreement = await runtime.createAttachmentKeyAgreement();
  const clientNonce = runtime.secureRandom(32);
  if (agreement.publicKey.length !== 65 || clientNonce.length !== 32) {
    throw new HostedRuntimeCapabilityError("p256-aes-gcm", "The platform attachment adapter returned invalid key material.");
  }
  if (agreement.publicKey[0] !== 0x04) {
    throw new HostedRuntimeCapabilityError("p256-aes-gcm", "The platform attachment adapter returned an invalid P-256 public key.");
  }
  const clientPublicKey = runtime.encodeBase64(agreement.publicKey);
  const encodedClientNonce = runtime.encodeBase64(clientNonce);
  const handshake = await localClient.createPorticoAttachmentHandshake({
    version: 1,
    clientPublicKey,
    clientNonce: encodedClientNonce
  }, {signal: options.signal});
  throwIfConnectionAborted(options.signal);
  const routePurpose = isLANRoute(route) ? "lan-server-route" : "trusted-server-route";
  const routeURL = validatePorticoUrl(route.url, routePurpose);
  const expectedAudience = new URL(routeURL).origin;
  if (handshake.version !== 1 || handshake.handshakeId.trim() === "" || handshake.serverId !== routeDocument.serverId ||
      handshake.serverPublicKeyFingerprint !== routeDocument.serverPublicKeyFingerprint ||
      handshake.clientPublicKey !== clientPublicKey || handshake.clientNonce !== encodedClientNonce ||
      handshake.audience !== expectedAudience || handshake.signatureAlgorithm !== "ed25519") {
    throw new Error("The server attachment handshake is not bound to the selected route and server identity.");
  }
  const issuedAt = Date.parse(handshake.issuedAt);
  const expiresAt = Date.parse(handshake.expiresAt);
  const now = (options.now?.() ?? runtime.now()).getTime();
  if (!Number.isFinite(issuedAt) || !Number.isFinite(expiresAt) || issuedAt > now + 60_000 || expiresAt <= issuedAt || expiresAt - issuedAt > 60_000 || now > expiresAt) {
    throw new Error("The server attachment handshake validity window is invalid.");
  }
  const serverIdentityKey = decodeBase64(handshake.serverPublicKey, true, runtime);
  const signedRouteKey = decodeBase64(routeDocument.serverPublicKey, false, runtime);
  if (serverIdentityKey.length !== 32 || signedRouteKey.length !== 32 || !equalBytes(serverIdentityKey, signedRouteKey)) {
    throw new Error("The attachment handshake server key does not match the signed Hosted route.");
  }
  const fingerprint = `sha256:${runtime.encodeBase64(await runtime.sha256(serverIdentityKey))}`;
  if (fingerprint !== handshake.serverPublicKeyFingerprint) {
    throw new Error("The attachment handshake server key fingerprint is invalid.");
  }
  const transcript = runtime.encodeText(porticoAttachmentTranscript(handshake));
  const signature = decodeBase64(handshake.signature, true, runtime);
  if (signature.length !== 64 || !await runtime.verifyEd25519({publicKey: serverIdentityKey, signature, message: transcript})) {
    throw new Error("The server did not prove possession of the signed attachment identity.");
  }
  const serverEphemeralPublicKey = decodeBase64(handshake.serverEphemeralPublicKey, true, runtime);
  if (serverEphemeralPublicKey.length !== 65 || serverEphemeralPublicKey[0] !== 0x04) {
    throw new Error("The server attachment key agreement is invalid.");
  }
  return {runtime, agreement, handshake, transcript, serverEphemeralPublicKey};
}

async function exchangePorticoAttachment(
  localClient: PorticoClient,
  prepared: PreparedPorticoAttachment,
  request: import("./client.js").PorticoServerSessionExchangeRequest,
  signal?: AbortSignal
): Promise<Awaited<ReturnType<PorticoClient["acceptPorticoSessionCredentials"]>>> {
  throwIfConnectionAborted(signal);
  const {runtime, agreement, handshake, transcript, serverEphemeralPublicKey} = prepared;
  const requestNonce = await porticoAttachmentDerivedBytes(runtime, "request-nonce", handshake, 12);
  const requestAAD = runtime.encodeText(porticoAttachmentAAD("request", handshake));
  const plaintext = runtime.encodeText(JSON.stringify(request));
  const ciphertext = await agreement.seal({
    peerPublicKey: serverEphemeralPublicKey,
    transcript,
    nonce: requestNonce,
    additionalData: requestAAD,
    payload: plaintext
  });
  throwIfConnectionAborted(signal);
  const exchanged = await localClient.exchangeEncryptedPorticoSession({
    version: 1,
    handshakeId: handshake.handshakeId,
    ciphertext: runtime.encodeBase64(ciphertext)
  }, {signal});
  throwIfConnectionAborted(signal);
  if (!isRecord(exchanged) || !isRecord(exchanged.payload) || exchanged.payload.version !== 1 ||
      exchanged.payload.handshakeId !== handshake.handshakeId || typeof exchanged.payload.ciphertext !== "string" ||
      exchanged.payload.ciphertext.length > 512 * 1024) {
    throw new Error("The encrypted server attachment response is invalid.");
  }
  const responseCiphertext = decodeBase64(exchanged.payload.ciphertext, true, runtime);
  const responseNonce = await porticoAttachmentDerivedBytes(runtime, "response-nonce", handshake, 12);
  const responseAAD = runtime.encodeText(porticoAttachmentAAD("response", handshake));
  let decoded: unknown;
  try {
    const responsePlaintext = await agreement.open({
      peerPublicKey: serverEphemeralPublicKey,
      transcript,
      nonce: responseNonce,
      additionalData: responseAAD,
      payload: responseCiphertext
    });
    decoded = JSON.parse(runtime.decodeText(responsePlaintext));
  } catch (error) {
    throw new Error("The encrypted server attachment response could not be authenticated.", {cause: error});
  }
  if (!decoded || typeof decoded !== "object" || Array.isArray(decoded)) {
    throw new Error("The encrypted server attachment response did not contain a protected result.");
  }
  const protectedResult = decoded as Record<string, unknown>;
  const status = protectedResult.status;
  const body = protectedResult.body;
  const retryAfter = typeof protectedResult.retryAfter === "string" ? protectedResult.retryAfter : undefined;
  if (!Number.isInteger(status) || (status as number) < 100 || (status as number) > 599 || !body || typeof body !== "object" || Array.isArray(body)) {
    throw new Error("The encrypted server attachment response did not contain a valid protected result.");
  }
  if ((status as number) >= 400) {
    const problem = body as Record<string, unknown>;
    const code = safeProblemCode(problem.code) ?? "attachment_failed";
    throw new ApiError(status as number, code, "The Portico Account could not be attached to this server.", undefined, retryAfter ? {retryAfter: retryAfter.slice(0, 128)} : undefined);
  }
  return localClient.acceptPorticoSessionCredentials(body as Parameters<PorticoClient["acceptPorticoSessionCredentials"]>[0]);
}

function porticoAttachmentTranscript(handshake: PreparedPorticoAttachment["handshake"]): string {
  return [
    "portico-attachment-handshake-v1",
    `handshakeId=${handshake.handshakeId}`,
    `serverId=${handshake.serverId}`,
    `serverPublicKey=${handshake.serverPublicKey}`,
    `serverPublicKeyFingerprint=${handshake.serverPublicKeyFingerprint}`,
    `clientPublicKey=${handshake.clientPublicKey}`,
    `clientNonce=${handshake.clientNonce}`,
    `serverEphemeralPublicKey=${handshake.serverEphemeralPublicKey}`,
    `audience=${handshake.audience}`,
    `issuedAt=${handshake.issuedAt}`,
    `expiresAt=${handshake.expiresAt}`
  ].join("\n");
}

async function porticoAttachmentDerivedBytes(
  runtime: ResolvedHostedConnectionRuntimeAdapters,
  label: string,
  handshake: PreparedPorticoAttachment["handshake"],
  length: number
): Promise<Uint8Array> {
  const digest = await runtime.sha256(runtime.encodeText([
    "portico-attachment-v1", label, handshake.handshakeId, handshake.serverId,
    handshake.serverPublicKeyFingerprint
  ].join("\n")));
  return digest.slice(0, length);
}

function porticoAttachmentAAD(direction: "request" | "response", handshake: PreparedPorticoAttachment["handshake"]): string {
  return [
    "portico-attachment-aead-v1", direction, handshake.handshakeId, handshake.serverId,
    handshake.serverPublicKeyFingerprint, "/api/auth/portico/sessions"
  ].join("\n");
}

function equalBytes(left: Uint8Array, right: Uint8Array): boolean {
  if (left.length !== right.length) return false;
  let difference = 0;
  for (let index = 0; index < left.length; index++) difference |= left[index]! ^ right[index]!;
  return difference === 0;
}

async function restoreConnectorSession(
  previousSession: LocalServerSession | undefined,
  options: Pick<HostedServerConnectorOptions, "sessionStore" | "credentialAdapter">
): Promise<unknown[]> {
  const restoreOperations: (() => void | Promise<void>)[] = [previousSession
    ? () => options.sessionStore.set(previousSession)
    : () => options.sessionStore.clear()];
  if (options.credentialAdapter) {
    restoreOperations.push(previousSession
      ? () => options.credentialAdapter!.save(previousSession)
      : () => options.credentialAdapter!.clear());
  }
  const restoreResults = await Promise.allSettled(
    restoreOperations.map(operation => Promise.resolve().then(operation))
  );
  const failures = restoreResults.flatMap(result => result.status === "rejected" ? [result.reason] : []);
  if (failures.length === 0) return [];

  // A partial restore is not trustworthy. Attempt every fail-closed deletion so
  // a fresh provider cannot resurrect either the candidate or an unknown mix of
  // old and new credentials.
  const clearOperations: (() => void | Promise<void>)[] = [() => options.sessionStore.clear()];
  if (options.credentialAdapter) clearOperations.push(() => options.credentialAdapter!.clear());
  const clearResults = await Promise.allSettled(
    clearOperations.map(operation => Promise.resolve().then(operation))
  );
  return [
    ...failures,
    ...clearResults.flatMap(result => result.status === "rejected" ? [result.reason] : [])
  ];
}

export class HostedConnectionAbortedError extends Error {
  constructor() {
    super("The server or profile selection was replaced by a newer choice.");
    this.name = "AbortError";
  }
}

export class HostedRouteDiscoveryTimeoutError extends ApiError {
  constructor() {
    super(0, "route_discovery_timeout", "The direct server route could not be discovered before the connection deadline.", undefined, {
      retryable: true,
    });
    this.name = "HostedRouteDiscoveryTimeoutError";
  }
}

export class HostedRouteRetryLaterError extends ApiError {
  constructor(cause: ApiError) {
    super(cause.status, "route_discovery_retry_later", "The connection target is temporarily busy. Try this connection again later.", undefined, {
      retryable: true,
      retryAfter: cause.retryAfter,
      retryAt: cause.retryAt,
      retryAfterMs: cause.retryAfterMs,
    });
    this.name = "HostedRouteRetryLaterError";
  }
}

/** Hosted accepted the attachment but has not published a usable endpoint generation yet. */
export class HostedRoutePublicationPendingError extends ApiError {
  constructor(cause?: ApiError) {
    super(cause?.status ?? 425, "route_publication_pending", "Portico is finishing this server's secure route.", undefined, {
      retryable: true,
      retryAfter: cause?.retryAfter,
      retryAt: cause?.retryAt,
      retryAfterMs: cause?.retryAfterMs,
    });
    this.name = "HostedRoutePublicationPendingError";
  }
}

export function throwIfConnectionAborted(signal?: AbortSignal): void {
  if (!signal?.aborted) return;
  if (signal.reason instanceof Error) throw signal.reason;
  throw new HostedConnectionAbortedError();
}

/**
 * Performs only Hosted document loading and pinned route verification. It is
 * safe to race against a cached server connection because it does not mint or
 * persist any account or server credential.
 */
export async function discoverHostedServerRoute(
  server: HostedServer,
  options: HostedServerRouteDiscoveryOptions
): Promise<HostedServerRouteDiscovery> {
  throwIfConnectionAborted(options.signal);
  if (server.preferredAuthMode === "local") {
    throw new Error("This server uses This Server sign-in and cannot be opened from a hosted Portico client.");
  }
  const runtime = createHostedConnectionRuntime(options.runtime);
  const startedAt = Date.now();
  const maximumDeadlineAt = startedAt + defaultHostedRouteDiscoveryTimeoutMs;
  const requestedDeadlineAt = options.discoveryDeadlineAt ?? maximumDeadlineAt;
  if (!Number.isFinite(requestedDeadlineAt) || requestedDeadlineAt <= startedAt) {
    throw new HostedRouteDiscoveryTimeoutError();
  }
  // A caller may shorten the interactive discovery budget, but it cannot turn
  // this foreground operation into an unbounded background retry loop.
  const discoveryDeadlineAt = Math.min(requestedDeadlineAt, maximumDeadlineAt);
  const deadlineController = runtime.createAbortController();
  const abortForCaller = () => deadlineController.abort(
    options.signal?.reason instanceof Error ? options.signal.reason : new HostedConnectionAbortedError()
  );
  options.signal?.addEventListener("abort", abortForCaller, {once: true});
  const deadlineTimer = runtime.setTimeout(
    () => deadlineController.abort(new HostedRouteDiscoveryTimeoutError()),
    Math.max(1, discoveryDeadlineAt - startedAt)
  );
  const discoveryOptions: HostedServerRouteDiscoveryOptions = {
    ...options,
    signal: deadlineController.signal,
    discoveryDeadlineAt,
  };
  // The outer discovery loop is the sole retry owner and is always bounded to
  // two waits / three route requests, even if an older caller supplies more.
  const retryDelays = (options.retryDelaysMs ?? defaultHostedConnectionRetryDelaysMs)
    .filter((milliseconds) => Number.isFinite(milliseconds) && milliseconds >= 0)
    .map((milliseconds) => Math.floor(milliseconds))
    .slice(0, 2);
  const retryDelay = options.retryDelay ?? ((milliseconds: number) => delay(milliseconds, runtime, deadlineController.signal));
  const retryCohort = options.retryCohort?.trim() || "";
  let lastRouteError: Error | undefined;
  const waitForRetry = async (error: unknown, attempt: number): Promise<void> => {
    throwIfConnectionAborted(deadlineController.signal);
    lastRouteError = error instanceof Error ? error : new Error(String(error));
    if (!hostedRouteFetchErrorIsRetryable(error) || attempt === retryDelays.length) throw lastRouteError;
    const retryAfterMs = error instanceof ApiError ? Math.max(0, error.retryAfterMs ?? 0) : 0;
    const configuredCap = retryDelays[attempt] ?? 0;
    const remaining = discoveryDeadlineAt - Date.now();
    if (retryAfterMs > 0 && retryAfterMs >= remaining) throw new HostedRouteRetryLaterError(error as ApiError);
    // Retry-After is a strict lower bound. Fleet spreading is still full jitter
    // across the complete configured cap; never collapse a cold fleet into a
    // one-second band after the server asks it to back off.
    const jitter = routeDiscoveryJitter(runtime, configuredCap, retryCohort, server.id, attempt);
    const waitMilliseconds = retryAfterMs + jitter;
    if (waitMilliseconds >= remaining) {
      if (retryAfterMs > 0) throw new HostedRouteRetryLaterError(error as ApiError);
      throw new HostedRouteDiscoveryTimeoutError();
    }
    await abortableRetryDelay(retryDelay, waitMilliseconds, deadlineController.signal);
    throwIfConnectionAborted(deadlineController.signal);
  };
  try {
    await options.hostedClient.checkCompatibility?.({
      signal: deadlineController.signal,
      retryBudget: 0,
      deadlineAt: discoveryDeadlineAt,
    });
    throwIfConnectionAborted(deadlineController.signal);
    for (let attempt = 0; attempt <= retryDelays.length; attempt++) {
      let routeDocument: HostedRouteDocument;
      try {
        throwIfConnectionAborted(deadlineController.signal);
        routeDocument = await options.hostedClient.routes(server.id, {
          signal: deadlineController.signal,
          retryBudget: 0,
          deadlineAt: discoveryDeadlineAt,
        });
      } catch (error) {
        await waitForRetry(hostedRoutePublicationPendingResponse(error) ?? error, attempt);
        continue;
      }
      try {
        throwIfConnectionAborted(deadlineController.signal);
        await verifyHostedRouteDocument(routeDocument, server.id, options.trustedHostedDocumentKeys, options.now?.() ?? runtime.now(), runtime, true);
			  if (routeDocument.serverPublicKey !== server.serverPublicKey || routeDocument.serverPublicKeyFingerprint !== server.serverPublicKeyFingerprint) {
				  throw new Error("The signed route document does not match the selected Hosted server identity.");
			  }
        assertRoutePublicationReady(routeDocument);
        const routes = routesForConnection(routeDocument, options.routePreference);
        const route = await selectVerifiedRoute(
          routes,
          routeDocument,
          discoveryOptions,
          options.routePreference === "public-only" || !options.localRouteCandidates
            ? undefined
            : signal => localRoutesForConnection(server, routeDocument, { ...discoveryOptions, signal })
        );
        throwIfConnectionAborted(deadlineController.signal);
        if (route) return { routeDocument, route };
        throw lastRouteError ?? new Error("Unable to verify a route for this server.");
      } catch (error) {
        await waitForRetry(error, attempt);
      }
    }
  } finally {
    runtime.clearTimeout(deadlineTimer);
    options.signal?.removeEventListener("abort", abortForCaller);
  }
  throw lastRouteError ?? new Error("Unable to verify a route for this server.");
}

function routeDiscoveryJitter(
  runtime: ResolvedHostedConnectionRuntimeAdapters,
  capMilliseconds: number,
  cohort: string | undefined,
  serverId: string,
  attempt: number,
): number {
  const random = () => {
    const bytes = runtime.secureRandom(4);
    return (
      ((bytes[0] ?? 0) * 0x1000000 + (bytes[1] ?? 0) * 0x10000 + (bytes[2] ?? 0) * 0x100 + (bytes[3] ?? 0)) /
      0x100000000
    );
  };
  const stableCohort = cohort?.trim() ? `${cohort.trim()}:${serverId}` : "";
  return positiveFullJitterDelay(Math.max(1, capMilliseconds), stableCohort, attempt, random);
}

function abortableRetryDelay(
  retryDelay: (milliseconds: number) => Promise<void>,
  milliseconds: number,
  signal: AbortSignal,
): Promise<void> {
  if (signal.aborted) {
    return Promise.reject(signal.reason instanceof Error ? signal.reason : new HostedConnectionAbortedError());
  }
  return new Promise((resolve, reject) => {
    let settled = false;
    const finish = (error?: unknown) => {
      if (settled) return;
      settled = true;
      signal.removeEventListener("abort", abort);
      if (error === undefined) resolve();
      else reject(error);
    };
    const abort = () => finish(signal.reason instanceof Error ? signal.reason : new HostedConnectionAbortedError());
    signal.addEventListener("abort", abort, {once: true});
    Promise.resolve()
      .then(() => {
        if (settled || signal.aborted) {
          throw signal.reason instanceof Error ? signal.reason : new HostedConnectionAbortedError();
        }
        return retryDelay(milliseconds);
      })
      .then(() => finish(), (error) => finish(error));
  });
}

function hostedRouteFetchErrorIsRetryable(error: unknown): boolean {
  if (error instanceof HostedRoutePublicationPendingError) return true;
  if (error instanceof PorticoTransportError) return true;
  if (!(error instanceof ApiError)) return false;
  return error.status === 408 || error.status === 425 || error.status === 429 || (error.status >= 500 && error.status < 600);
}

function hostedRoutePublicationPendingResponse(error: unknown): HostedRoutePublicationPendingError | undefined {
  if (!(error instanceof ApiError)) return undefined;
  // These statuses are scoped to the Hosted route-read operation here.
  return error.status === 409 || error.status === 425
    ? new HostedRoutePublicationPendingError(error)
    : undefined;
}

function assertRoutePublicationReady(document: HostedRouteDocument): void {
  if (!Number.isSafeInteger(document.endpointGeneration) || (document.endpointGeneration ?? 0) <= 0) {
    throw new HostedRoutePublicationPendingError();
  }
  const certificateStatus = document.certificate?.status?.trim().toLowerCase();
  if (document.routes.length === 0 && certificateStatus !== "valid" && certificateStatus !== "active") {
    throw new HostedRoutePublicationPendingError();
  }
}

export async function verifyHostedRouteDocument(
  document: HostedRouteDocument,
  expectedServerId: string,
  trustedKeys: Record<string, string>,
  now = new Date(),
  runtimeAdapters: HostedConnectionRuntimeAdapters | ResolvedHostedConnectionRuntimeAdapters = {},
  allowPendingGeneration = false,
): Promise<void> {
  const runtime = createHostedConnectionRuntime(runtimeAdapters);
  assertRouteDocumentEnvelope(document, allowPendingGeneration);
  if (document.audience !== "portico-media-server") throw new Error("The route document was issued for a different product.");
  if (document.serverId !== expectedServerId) throw new Error("The route document identifies a different server.");
	if (!document.serverPublicKey?.trim() || !isValidPorticoServerPublicKeyFingerprint(document.serverPublicKeyFingerprint)) throw new Error("The route document omits the server identity key.");
	if (!document.signatureKeyId?.trim() || document.signatureKeyId.length > 256) throw new Error("The route document signing key ID is invalid.");
  if (document.signatureAlgorithm !== "ed25519") throw new Error("The route document signature algorithm is not supported.");
  const encodedKey = Object.prototype.hasOwnProperty.call(trustedKeys, document.signatureKeyId)
    ? trustedKeys[document.signatureKeyId]
    : undefined;
  if (!encodedKey) throw new Error("The route document signing key is not trusted.");
  const issuedAt = Date.parse(document.issuedAt);
  const expiresAt = Date.parse(document.expiresAt);
  const nowMs = now.getTime();
  if (!Number.isFinite(issuedAt) || !Number.isFinite(expiresAt) || expiresAt <= issuedAt || expiresAt - issuedAt > 10 * 60_000) {
    throw new Error("The route document validity window is invalid.");
  }
  if (issuedAt > nowMs + 60_000) throw new Error("The route document is not valid yet.");
  if (nowMs > expiresAt + 60_000) throw new Error("The route document has expired.");
	const serverIdentityKey = decodeBase64(document.serverPublicKey, false, runtime);
	if (serverIdentityKey.length !== 32) throw new Error("The route document server identity key is invalid.");
	const serverFingerprint = `sha256:${runtime.encodeBase64(await runtime.sha256(serverIdentityKey))}`;
	if (serverFingerprint !== document.serverPublicKeyFingerprint) throw new Error("The route document server identity fingerprint is invalid.");
  let publicKey: Uint8Array;
  try {
    publicKey = decodeBase64(encodedKey, false, runtime);
  } catch (error) {
    if (error instanceof HostedRuntimeCapabilityError) throw error;
    throw new Error("The trusted Hosted route key is invalid.");
  }
  const signature = decodeBase64(document.signature, true, runtime);
  if (signature.length !== 64) throw new Error("The route document signature is invalid.");
  const payload = runtime.encodeText(`portico-signed-document:route-document:v1\n${canonicalJSONString(document)}`);
  let valid: boolean;
  try {
    valid = await runtime.verifyEd25519({ publicKey, signature, message: payload });
  } catch (error) {
    if (error instanceof HostedRuntimeCapabilityError) throw error;
    throw new Error(`Hosted route signature verification failed: ${error instanceof Error ? error.message : String(error)}`);
  }
  if (!valid) {
    throw new Error("The route document signature is invalid.");
  }
}

export function trustedHostedDocumentKeysFromKeySet(
  keySet: PorticoDocumentSigningKeySet,
  runtimeAdapters: HostedConnectionRuntimeAdapters | ResolvedHostedConnectionRuntimeAdapters = {}
): Record<string, string> {
  const runtime = createHostedConnectionRuntime(runtimeAdapters);
  if (keySet.schemaVersion !== 1 || !keySet.activeKeyId || !Array.isArray(keySet.keys)) {
    throw new Error("The Hosted document signing key set version is not supported.");
  }
  const trusted: Record<string, string> = {};
  let activeKeyFound = false;
  for (const key of keySet.keys) {
    if (!key.keyId || key.algorithm !== "ed25519" || (key.state !== "active" && key.state !== "verification")) {
      throw new Error("The Hosted document signing key set contains an invalid key.");
    }
    if (trusted[key.keyId]) throw new Error("The Hosted document signing key set contains a duplicate key ID.");
    const decoded = decodeBase64(key.publicKeyB64, false, runtime);
    if (decoded.byteLength !== 32) throw new Error("The Hosted document signing key set contains an invalid Ed25519 public key.");
    trusted[key.keyId] = key.publicKeyB64;
    if (key.keyId === keySet.activeKeyId && key.state === "active") activeKeyFound = true;
  }
  if (!activeKeyFound) throw new Error("The Hosted document signing key set does not identify its active key.");
  return trusted;
}

function canonicalJSONString(document: HostedRouteDocument): string {
  const unsigned = { ...document } as Record<string, unknown>;
  delete unsigned.signature;
  // Match Go encoding/json with SetEscapeHTML(false): preserve UTF-8 and
  // HTML-sensitive characters, but escape the two JavaScript line separators.
  return JSON.stringify(sortJSONValue(unsigned))
    .replace(/\u2028/g, "\\u2028")
    .replace(/\u2029/g, "\\u2029");
}

function sortJSONValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortJSONValue);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(Object.entries(value as Record<string, unknown>)
    // Hosted Services canonicalizes JSON object keys by raw lexical byte
    // order. localeCompare is environment-sensitive and orders some adjacent
	// ASCII keys differently (for example host/quality), which
    // invalidates otherwise-correct signed route documents.
    .sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0)
    .map(([key, nested]) => [key, sortJSONValue(nested)]));
}

function decodeBase64(value: string, urlSafe: boolean, runtime: ResolvedHostedConnectionRuntimeAdapters): Uint8Array {
  let normalized = value.trim();
  if (normalized !== value || !normalized || !/^[A-Za-z0-9+/_-]+={0,2}$/u.test(normalized) ||
      normalized.includes("=") && !/={1,2}$/u.test(normalized)) {
    throw new Error("The route document signature encoding is invalid.");
  }
  const unpaddedLength = normalized.replace(/=+$/u, "").length;
  if (unpaddedLength % 4 === 1 || normalized.includes("=") && normalized.length % 4 !== 0) {
    throw new Error("The route document signature encoding is invalid.");
  }
  if (urlSafe || normalized.includes("-") || normalized.includes("_")) normalized = normalized.replace(/-/g, "+").replace(/_/g, "/");
  normalized += "=".repeat((4 - normalized.length % 4) % 4);
  if (normalized.length % 4 !== 0) throw new Error("The route document signature encoding is invalid.");
  try {
    return runtime.decodeBase64(normalized);
  } catch (error) {
    if (error instanceof HostedRuntimeCapabilityError) throw error;
    throw new Error("The route document signature encoding is invalid.");
  }
}

function safeProblemCode(value: unknown): string | undefined {
  if (typeof value !== "string") return undefined;
  const normalized = value.trim();
  return /^[A-Za-z0-9._:-]{1,128}$/u.test(normalized) ? normalized : undefined;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

async function localRoutesForConnection(server: HostedServer, document: HostedRouteDocument, options: HostedServerRouteDiscoveryOptions): Promise<HostedRouteEntry[]> {
  if (!options.localRouteCandidates) return [];
  throwIfConnectionAborted(options.signal);
  const candidates = await options.localRouteCandidates(server, document, options.signal);
  throwIfConnectionAborted(options.signal);
  return candidates
    .filter((route) => isLANRoute(route) && route.url && routeIsUsableCandidate(route));
}

async function selectVerifiedRoute(
  routes: HostedRouteEntry[],
  document: HostedRouteDocument,
  options: HostedServerRouteDiscoveryOptions,
  discoverLocalRoutes?: (signal: AbortSignal) => Promise<HostedRouteEntry[]>
): Promise<HostedRouteEntry> {
  type RouteProbeResult = { route?: HostedRouteEntry; error?: Error };
  const unique = new Map<string, HostedRouteEntry>();
  for (const route of routes) {
    const key = `${route.type}\n${route.url}`;
    if (!unique.has(key)) unique.set(key, route);
  }
  const candidates = [...unique.values()];
  const lan = candidates.filter(isLANRoute);
  const publicRoutes = candidates.filter((route) => !isLANRoute(route));
  const runtime = createHostedConnectionRuntime(options.runtime);
  const controller = runtime.createAbortController();
  const abortForCaller = () => controller.abort(options.signal?.reason);
  options.signal?.addEventListener("abort", abortForCaller, {once: true});
  const scopedOptions = { ...options, signal: controller.signal };
  const tasks: Promise<RouteProbeResult>[] = [];
  const lanFirst = options.routePreference !== "public-first" && options.routePreference !== "public-only";
  const allowLAN = options.routePreference !== "public-only";
  const allowPublic = options.routePreference !== "lan-only";
  if (allowLAN && lan.length > 0) {
    tasks.push((async () => {
      if (!lanFirst) await delay(hostedRouteFallbackDelayMs, runtime, controller.signal);
      return probeRouteGroup(lan, document, scopedOptions);
    })());
  }
  if (allowLAN && discoverLocalRoutes) {
    tasks.push((async () => {
      const discovered = await discoverLocalRoutes(controller.signal);
      throwIfConnectionAborted(controller.signal);
      if (!lanFirst) await delay(hostedRouteFallbackDelayMs, runtime, controller.signal);
      return discovered.length > 0
        ? probeRouteGroup(discovered, document, scopedOptions)
        : { error: new Error("No nearby route was discovered for this server.") } satisfies RouteProbeResult;
    })().catch(error => ({ error: error instanceof Error ? error : new Error(String(error)) } satisfies RouteProbeResult)));
  }
  if (allowPublic && publicRoutes.length > 0) {
    tasks.push((async () => {
      // Preference is a small head start, never a cumulative LAN/WAN timeout.
      if (lanFirst && (lan.length > 0 || discoverLocalRoutes)) {
        await delay(hostedRouteFallbackDelayMs, runtime, controller.signal);
      }
      return probeRouteGroup(publicRoutes, document, scopedOptions);
    })());
  }
  let lastError: Error | undefined;
  let hardError: Error | undefined;
  const pending = new Map<number, Promise<{ index: number; result: RouteProbeResult }>>(tasks.map((task, index) => [index, task.then(
    result => ({ index, result }),
    error => ({ index, result: { error: error instanceof Error ? error : new Error(String(error)) } })
  )]));
  try {
    while (pending.size > 0) {
      const settled = await Promise.race(pending.values());
      throwIfConnectionAborted(options.signal);
      pending.delete(settled.index);
      if (settled.result.route) return settled.result.route;
      if (settled.result.error) {
        lastError = settled.result.error;
        if (!hostedConnectionErrorIsRetryable(settled.result.error) && !hardError) {
          hardError = settled.result.error;
        }
      }
    }
  } finally {
    if (!controller.signal.aborted) controller.abort();
    options.signal?.removeEventListener("abort", abortForCaller);
  }
  if (options.routePreference === "public-only" && lan.length > 0) {
    throw new NearbyRouteAvailableError(lastError === undefined ? undefined : { cause: lastError });
  }
  throw hardError ?? lastError ?? new Error("Unable to verify a route for this server.");
}

async function probeRouteGroup(routes: HostedRouteEntry[], document: HostedRouteDocument, options: HostedServerRouteDiscoveryOptions): Promise<{ route?: HostedRouteEntry; error?: Error }> {
  const parallelism = Math.max(1, Math.min(4, options.maxParallelRouteProbes ?? 3));
  let lastError: Error | undefined;
  let hardError: Error | undefined;
  for (let offset = 0; offset < routes.length; offset += parallelism) {
    throwIfConnectionAborted(options.signal);
    const batch = routes.slice(offset, offset + parallelism);
    const pending = new Map<number, Promise<{ index: number; route?: HostedRouteEntry; error?: Error }>>(batch.map((route, index) => [index, verifyRoute(route, document, options).then(
      () => ({ index, route } as { index: number; route?: HostedRouteEntry; error?: Error }),
      error => ({ index, error: error instanceof Error ? error : new Error(String(error)) } as { index: number; route?: HostedRouteEntry; error?: Error })
    )]));
    while (pending.size > 0) {
      const result = await Promise.race(pending.values());
      throwIfConnectionAborted(options.signal);
      pending.delete(result.index);
      if (result.route) return { route: result.route };
      if (result.error) {
        lastError = result.error;
        if (!hostedConnectionErrorIsRetryable(result.error) && !hardError) hardError = result.error;
      }
    }
  }
  return { error: hardError ?? lastError };
}

function delay(milliseconds: number, runtime: ResolvedHostedConnectionRuntimeAdapters, signal?: AbortSignal): Promise<void> {
  if (signal?.aborted) return Promise.reject(signal.reason instanceof Error ? signal.reason : new HostedConnectionAbortedError());
  return new Promise((resolve, reject) => {
    const timeout = runtime.setTimeout(() => {
      signal?.removeEventListener("abort", abort);
      resolve();
    }, milliseconds);
    const abort = () => {
      runtime.clearTimeout(timeout);
      signal?.removeEventListener("abort", abort);
      reject(signal?.reason instanceof Error ? signal.reason : new HostedConnectionAbortedError());
    };
    signal?.addEventListener("abort", abort, {once: true});
  });
}

export function hostedRouteErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return presentationText(resolveProductProblem(error));
  }
  return presentationText(productMessage("problem.connection-failed"));
}

export function porticoSessionErrorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return presentationText(resolveProductProblem(error));
  }
  return presentationText(productMessage("problem.request-failed"));
}

function presentationText(presentation: { title?: string; body?: string; text?: string }): string {
  return presentation.body ?? presentation.title ?? presentation.text ?? "Portico couldn't complete this request.";
}

export function preferredRoute(document: HostedRouteDocument): HostedRouteEntry | undefined {
  assertRouteDocumentEnvelope(document);
  try {
    return routesForConnection(document)[0];
  } catch {
    return undefined;
  }
}

export function routesForConnection(document: HostedRouteDocument, preference: HostedRoutePreference = "lan-first"): HostedRouteEntry[] {
  assertRouteDocumentEnvelope(document);
  const authModes = Array.isArray(document.authModes) ? document.authModes : ["portico"];
  if (authModes.includes("local") || !authModes.includes("portico")) {
    throw new Error("This server uses This Server sign-in and cannot be opened from a hosted Portico client.");
  }
	const lanRoutes = orderedPublishedRoutes(document.routes, isLANRoute);
	const publicRoutes = orderedPublishedRoutes(document.routes, (route) => isIPEncodedPublicRoute(route) || isPublicDirectRoute(route));
  // Restricted modes retain the untried group as metadata so selection can
  // offer an explicit recovery without probing it.
	const routesByTypePreference = preference === "public-first" || preference === "public-only"
		? [...publicRoutes, ...lanRoutes]
		: [...lanRoutes, ...publicRoutes];
	const routes = routesByTypePreference.sort((left, right) =>
		(left.quality === "reachable" ? 0 : 1) - (right.quality === "reachable" ? 0 : 1),
	);
  const seen = new Set<string>();
  const unique = routes.filter((route) => {
    const key = `${route.type}\n${route.url}`;
    if (seen.has(key)) return false;
    seen.add(key);
    return true;
  });
	if (unique.length === 0) throw new Error("No usable signed route is published for this server yet.");
	return unique;
}

function orderedPublishedRoutes(
	routes: HostedRouteEntry[],
	matchesType: (route: HostedRouteEntry) => boolean,
): HostedRouteEntry[] {
	const ordered: HostedRouteEntry[] = [];
	for (const quality of ["reachable", "probe_required"] as const) {
		ordered.push(
			...routes.filter((route) => matchesType(route) && isIPEncodedDirectRoute(route) && route.quality === quality && routeIsSecureHTTPS(route)),
			...routes.filter((route) => matchesType(route) && !isIPEncodedDirectRoute(route) && route.quality === quality && routeIsSecureHTTPS(route)),
		);
	}
	return ordered;
}

function assertRouteDocumentEnvelope(document: HostedRouteDocument, allowPendingGeneration = false): asserts document is HostedRouteDocument & {endpointGeneration: number} {
  if (document.kind !== "route-document") throw new Error("The route document kind is invalid.");
  if (document.documentVersion !== 1) throw new Error("The route document version is not supported.");
  if (allowPendingGeneration && (document.endpointGeneration === undefined || document.endpointGeneration === 0)) return;
  if (!Number.isSafeInteger(document.endpointGeneration) || (document.endpointGeneration ?? 0) <= 0) {
    throw new Error("The route document endpoint generation is invalid.");
  }
}

function hostedConnectionErrorIsRetryable(error: unknown): boolean {
  // A browser Local Network denial requires a user decision; repeating the
  // private-address probe cannot repair it and can produce repeated prompts.
  if (error instanceof LocalNetworkRouteUnavailableError) return false;
  if (error instanceof ApiError) {
    return error.status === 429 || error.status >= 500 || error.status === 0;
  }
  const message = error instanceof Error ? error.message : String(error);
  const normalized = message.toLowerCase();
  if (normalized.includes("wrong server identity")
    || normalized.includes("fingerprint")
    || normalized.includes("this server sign-in")
    || normalized.includes("route document")
    || normalized.includes("document signing key")
    || normalized.includes("signature")
    || normalized.includes("ed25519")
    || normalized.includes("tls verification")
    || normalized.includes("certificate status")) {
    return false;
  }
  return true;
}

export function isIPEncodedDirectRoute(route: HostedRouteEntry): boolean {
  return route.type === "public_direct_ip_encoded" || route.type === "lan_ip_encoded";
}

function isLANRoute(route: HostedRouteEntry): boolean {
  return route.type === "lan" || route.type === "lan_ip_encoded" || route.type === "lan_discovered";
}

function isPublicDirectRoute(route: HostedRouteEntry): boolean {
  return route.type === "public_direct" || route.type === "public_console_origin";
}

function isIPEncodedPublicRoute(route: HostedRouteEntry): boolean {
  return route.type === "public_direct_ip_encoded";
}

export function routeIsUsableCandidate(route: HostedRouteEntry): boolean {
	return route.quality === "reachable" || route.quality === "probe_required";
}

function routeIsSecureHTTPS(route: HostedRouteEntry): boolean {
  try {
    validatePorticoUrl(route.url, isLANRoute(route) ? "lan-server-route" : "trusted-server-route");
    return true;
  } catch {
    return false;
  }
}

async function verifyRoute(route: HostedRouteEntry, document: HostedRouteDocument, options: HostedServerRouteDiscoveryOptions): Promise<void> {
  throwIfConnectionAborted(options.signal);
  const base = validatePorticoUrl(route.url, isLANRoute(route) ? "lan-server-route" : "trusted-server-route");
  const runtime = createHostedConnectionRuntime(options.runtime);
  const requestFetch = options.routeProbeFetch ?? runtime.fetch;
  let response: Response;
  const controller = runtime.createAbortController();
  const cancelForNewerChoice = () => controller.abort();
  options.signal?.addEventListener("abort", cancelForNewerChoice, {once: true});
  const timeout = runtime.setTimeout(() => controller.abort(), Math.max(500, options.routeProbeTimeoutMs ?? 3500));
  try {
    response = await requestFetch(`${base}/api/remote-access/health`, {
      method: "GET",
      signal: controller.signal,
      redirect: "error",
      credentials: "omit",
      cache: "no-store"
    });
  } catch (error) {
    throwIfConnectionAborted(options.signal);
    const detail = controller.signal.aborted
      ? "The route health check timed out."
      : error instanceof TypeError ? "The client could not establish a secure connection." : "The health request failed.";
    void reportRouteFailure(route, document, options, "transport_failed");
    if ((options.routePreference === "public-first" || options.routePreference === "lan-only") && isLANRoute(route) && error instanceof TypeError) {
      throw new LocalNetworkRouteUnavailableError({ cause: error });
    }
    throw new PorticoTransportError(
      "route_probe_transport_failed",
      `${detail} Check that the server is online and Remote Access is enabled, then try again.`,
      error,
      {method: "GET", phase: controller.signal.aborted ? "timeout" : "request", retryable: true},
    );
  } finally {
    runtime.clearTimeout(timeout);
    options.signal?.removeEventListener("abort", cancelForNewerChoice);
  }
  throwIfConnectionAborted(options.signal);
  try {
    assertRouteHealthResponseNotRedirected(response, base);
  } catch (error) {
    await cancelRouteProbeResponse(response);
    throw error;
  }
  if (response.status === 401 || response.status === 403) {
    await cancelRouteProbeResponse(response);
    void reportRouteFailure(route, document, options, "http_failed");
    throw new Error("This route candidate did not authorize its health check.");
  }
  if (!response.ok) {
    if (response.status === 408 || response.status === 425 || response.status === 429 ||
        (response.status >= 500 && response.status < 600)) {
      const retryMetadata = parseRetryAfter(response.headers.get("Retry-After"));
      await cancelRouteProbeResponse(response);
      throw new ApiError(
        response.status,
        "route_probe_retryable",
        `The media server route is temporarily unavailable (HTTP ${response.status}).`,
        undefined,
        {retryable: true, ...retryMetadata},
      );
    }
    await cancelRouteProbeResponse(response);
    void reportRouteFailure(route, document, options, "http_failed");
    throw new Error(`Remote Access health verification failed with HTTP ${response.status}. Check the local server Remote Access status.`);
  }
  let health: HostedRouteHealthEvidence;
  try {
    health = decodeRouteHealth(await response.json());
  } catch {
    void reportRouteFailure(route, document, options, "invalid_health");
    throw new Error("The selected route returned invalid health evidence.");
  }
  if (health.serverId !== document.serverId) {
    void reportRouteFailure(route, document, options, "identity_mismatch");
    throw new Error("The selected route returned the wrong server identity. Do not continue until DNS points to the expected Portico server.");
  }
  if (health.serverPublicKeyFingerprint !== document.serverPublicKeyFingerprint) {
    void reportRouteFailure(route, document, options, "identity_mismatch");
    throw new Error("The selected route did not match the expected server key fingerprint. Do not continue until the owner verifies the server identity.");
  }
  if (health.remoteAccessEnabled === false) {
    void reportRouteFailure(route, document, options, "remote_access_disabled");
    throw new Error("Remote Access is disabled on the selected server route. Enable Remote Access on the local server before connecting.");
  }
}

async function cancelRouteProbeResponse(response: Response): Promise<void> {
  try {
    await response.body?.cancel();
  } catch {
    // Preserve the classified connection failure if the runtime has already
    // locked or canceled the body. The retry owner must still make progress.
  }
}

export function decodeRouteHealth(value: unknown): HostedRouteHealthEvidence {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError("The route health response is invalid.");
  const health = value as Record<string, unknown>;
  if (typeof health.serverId !== "string" || !health.serverId.trim() ||
      !isValidPorticoServerPublicKeyFingerprint(health.serverPublicKeyFingerprint) ||
      typeof health.remoteAccessEnabled !== "boolean") {
    throw new TypeError("The route health response is missing required identity evidence.");
  }
  return {serverId: health.serverId, serverPublicKeyFingerprint: health.serverPublicKeyFingerprint, remoteAccessEnabled: health.remoteAccessEnabled};
}

export function assertRouteHealthResponseNotRedirected(response: Response, expectedURL: string): void {
  if (response.redirected || response.type === "opaqueredirect") {
    throw new Error("The route health check followed an unexpected redirect.");
  }
  if (!response.url) return;
  try {
    if (new URL(response.url).origin !== new URL(expectedURL).origin) {
      throw new Error("The route health check returned from an unexpected origin.");
    }
  } catch (error) {
    if (error instanceof Error && error.message.includes("unexpected origin")) throw error;
    throw new Error("The route health check returned an invalid response URL.");
  }
}

type RouteFailureCategory = "transport_failed" | "http_failed" | "invalid_health" | "identity_mismatch" | "remote_access_disabled";

async function reportRouteFailure(route: HostedRouteEntry, document: HostedRouteDocument, options: HostedServerRouteDiscoveryOptions, category: RouteFailureCategory): Promise<void> {
  if (!isPublicDirectRoute(route) && !isIPEncodedPublicRoute(route)) return;
  if (options.signal?.aborted) return;
  if (pendingRouteFailureReports >= maxPendingRouteFailureReports) return;
  const endpointGeneration = document.endpointGeneration;
  if (!Number.isSafeInteger(endpointGeneration) || (endpointGeneration ?? 0) <= 0) return;
  const runtime = createHostedConnectionRuntime(options.runtime);
  const now = runtime.now().getTime();
  const reportKey = `${document.serverId}\n${endpointGeneration}\n${route.type}`;
  const lastReportedAt = recentRouteFailureReports.get(reportKey);
  if (lastReportedAt !== undefined && now-lastReportedAt < routeFailureReportCooldownMs) return;
  recentRouteFailureReports.set(reportKey, now);
  if (recentRouteFailureReports.size > maxRememberedRouteFailureReports) {
    const oldest = [...recentRouteFailureReports.entries()].sort((left, right) => left[1]-right[1])[0];
    if (oldest) recentRouteFailureReports.delete(oldest[0]);
  }
  pendingRouteFailureReports += 1;
  try {
    await options.hostedClient.reportRouteFailure(document.serverId, {
      routeType: route.type,
      endpointGeneration: endpointGeneration as number,
      category
    });
  } catch {
    // Failure reporting should never hide the original connection problem.
  } finally {
    pendingRouteFailureReports -= 1;
  }
}
