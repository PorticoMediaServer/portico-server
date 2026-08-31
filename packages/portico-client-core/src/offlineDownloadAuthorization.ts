import {
  createHostedConnectionRuntime,
  type HostedConnectionRuntimeAdapters,
  type ResolvedHostedConnectionRuntimeAdapters
} from "./hostedConnectionRuntime.js";
import { normalizeViewerScope, type ViewerScope } from "./viewerScope.js";

const THIRTY_DAYS_MILLISECONDS = 2_592_000_000;
const SHA256_PATTERN = /^[0-9a-f]{64}$/u;
const FINGERPRINT_PATTERN = /^sha256:[A-Za-z0-9_-]{42}[AEIMQUYcgkosw048]$/u;
const SIGNATURE_PATTERN = /^[A-Za-z0-9_-]{85}[AQgw]$/u;
const SERVER_PUBLIC_KEY_PATTERN = /^[A-Za-z0-9+/]{42}[AEIMQUYcgkosw048]$/u;
const RFC3339_UTC_PATTERN = /^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,9})?Z$/u;

export type OfflineDownloadAuthorizationReceipt = Readonly<{
  version: 1;
  purpose: "offline-download-authorization";
  receiptId: string;
  viewerScope: Readonly<{
    scopeKind: "server-bound";
    authority: "hosted" | "local";
    accountId: string;
    profileId: string;
    serverId: string;
    authorizationRevision: string;
  }>;
  issuer: Readonly<{serverId: string; signingKeyFingerprint: string}>;
  preparation: Readonly<{
    preparationId: string;
    mediaId: string;
    mediaVersionId: string;
    qualityId: string;
  }>;
  artifact: Readonly<{sha256: string; sizeBytes: number}>;
  lastVerifiedAt: string;
  verifyBy: string;
  signature: string;
}>;

export type OfflineDownloadAuthorizationBinding = Readonly<{
  storedViewerScope: ViewerScope;
  originatingServerId: string;
  preparationId: string;
  mediaId: string;
  mediaVersionId: string;
  qualityId: string;
  artifactSha256: string;
  artifactSizeBytes: number;
}>;

export type PinnedServerIdentity = Readonly<{
  serverId: string;
  publicKeyFingerprint: string;
  publicKey: Uint8Array;
}>;

export type DurableServerIdentitySource = Readonly<{
  serverId: string;
  serverPublicKey: string;
  serverPublicKeyFingerprint: string;
}>;

/**
 * Turns a connection/session identity into the byte-exact pin used by offline
 * receipt validation. The raw key is never inferred from its fingerprint.
 */
export async function pinnedServerIdentityFromDurableSource(
  source: DurableServerIdentitySource,
  runtimeAdapters?: HostedConnectionRuntimeAdapters | ResolvedHostedConnectionRuntimeAdapters
): Promise<PinnedServerIdentity> {
  const runtime = createHostedConnectionRuntime(runtimeAdapters);
  const serverId = source.serverId?.trim();
  const encodedKey = source.serverPublicKey?.trim();
  const fingerprint = source.serverPublicKeyFingerprint?.trim();
  if (!serverId || !SERVER_PUBLIC_KEY_PATTERN.test(encodedKey) || !FINGERPRINT_PATTERN.test(fingerprint)) {
    throw new TypeError("The durable Server identity is incomplete.");
  }
  const publicKey = runtime.decodeBase64(encodedKey);
  if (publicKey.byteLength !== 32) throw new TypeError("The durable Server identity key is invalid.");
  const digest = await runtime.sha256(publicKey);
  if (digest.byteLength !== 32 || `sha256:${runtime.encodeBase64(digest)}` !== fingerprint) {
    throw new TypeError("The durable Server identity key does not match its fingerprint.");
  }
  return {serverId, publicKeyFingerprint: fingerprint, publicKey};
}

export type OfflineDownloadAuthorizationValidation =
  | Readonly<{state: "valid"; receipt: OfflineDownloadAuthorizationReceipt}>
  | Readonly<{state: "authorization-unverified"; receipt: OfflineDownloadAuthorizationReceipt}>
  | Readonly<{state: "out-of-scope"; deleteProtectedArtifact: false}>
  | Readonly<{state: "invalid"; deleteProtectedArtifact: true}>;

export type OfflineDownloadAuthorizationValidationContext = Readonly<{
  binding: OfflineDownloadAuthorizationBinding;
  activeViewerScope: ViewerScope;
  pinnedIdentity: PinnedServerIdentity;
  runtimeAdapters?: HostedConnectionRuntimeAdapters | ResolvedHostedConnectionRuntimeAdapters;
  now?: number;
}>;

export type OfflineDownloadAuthorizationRevalidationRequest = Readonly<{
  receipt: OfflineDownloadAuthorizationReceipt;
}>;

export type OfflineDownloadAuthorizationRevalidationResponse =
  | Readonly<{outcome: "valid-replacement"; receipt: OfflineDownloadAuthorizationReceipt}>
  | Readonly<{outcome: "revoked" | "invalid" | "out-of-scope"}>;

export type OfflineDownloadAuthorizationRevalidationAPI = {
  revalidateOfflineDownloadAuthorization(
    body: OfflineDownloadAuthorizationRevalidationRequest
  ): Promise<unknown>;
};

export type OfflineDownloadAuthorizationRevalidationDecision =
  | Readonly<{action: "replace"; receipt: OfflineDownloadAuthorizationReceipt}>
  | Readonly<{action: "delete"; outcome: "revoked" | "invalid"}>
  | Readonly<{action: "preserve"; outcome: "out-of-scope" | "transport-failure"}>;

export async function validateOfflineDownloadAuthorizationReceipt(
  candidate: unknown,
  context: OfflineDownloadAuthorizationValidationContext
): Promise<OfflineDownloadAuthorizationValidation> {
  const runtime = createHostedConnectionRuntime(context.runtimeAdapters);
  const {binding, pinnedIdentity} = context;
  const now = context.now ?? Date.now();
  let storedScope: ViewerScope;
  let activeScope: ViewerScope;
  try {
    storedScope = normalizeViewerScope(binding.storedViewerScope);
    activeScope = normalizeViewerScope(context.activeViewerScope);
  } catch {
    return {state: "out-of-scope", deleteProtectedArtifact: false};
  }
  if (!sameViewerIdentity(activeScope, storedScope)) {
    // Active profile selection is an access boundary, not evidence that the
    // protected artifact or its immutable stored binding is corrupt.
    return {state: "out-of-scope", deleteProtectedArtifact: false};
  }
  let receipt: OfflineDownloadAuthorizationReceipt;
  try {
    if (!Number.isFinite(now)) return {state: "invalid", deleteProtectedArtifact: true};
    receipt = parseOfflineDownloadAuthorizationReceipt(candidate);
    if (receipt.viewerScope.authority !== storedScope.authority || receipt.viewerScope.accountId !== storedScope.accountId ||
        receipt.viewerScope.profileId !== storedScope.profileId || receipt.viewerScope.serverId !== storedScope.serverId ||
        receipt.viewerScope.authorizationRevision !== storedScope.authorizationRevision ||
        receipt.issuer.serverId !== binding.originatingServerId || receipt.issuer.serverId !== pinnedIdentity.serverId ||
        receipt.preparation.preparationId !== binding.preparationId || receipt.preparation.mediaId !== binding.mediaId ||
        receipt.preparation.mediaVersionId !== binding.mediaVersionId || receipt.preparation.qualityId !== binding.qualityId ||
        receipt.artifact.sha256 !== binding.artifactSha256 || receipt.artifact.sizeBytes !== binding.artifactSizeBytes) {
      return {state: "invalid", deleteProtectedArtifact: true};
    }
    const publicKey = Uint8Array.from(pinnedIdentity.publicKey);
    if (publicKey.byteLength !== 32 || receipt.issuer.signingKeyFingerprint !== pinnedIdentity.publicKeyFingerprint) {
      return {state: "invalid", deleteProtectedArtifact: true};
    }
    const fingerprintDigest = await runtime.sha256(publicKey);
    const computedFingerprint = `sha256:${runtime.encodeBase64(fingerprintDigest)}`;
    if (fingerprintDigest.byteLength !== 32 || computedFingerprint !== pinnedIdentity.publicKeyFingerprint ||
        decodeCanonicalFingerprint(receipt.issuer.signingKeyFingerprint, runtime).byteLength !== 32) {
      return {state: "invalid", deleteProtectedArtifact: true};
    }
    const signature = decodeCanonicalBase64URL(receipt.signature, 64, runtime);
    const payload = runtime.encodeText(canonicalOfflineDownloadAuthorizationPayload(receipt));
    if (!await runtime.verifyEd25519({publicKey, signature, message: payload})) {
      return {state: "invalid", deleteProtectedArtifact: true};
    }
    if (!Number.isSafeInteger(binding.artifactSizeBytes) || binding.artifactSizeBytes < 1 ||
        !SHA256_PATTERN.test(binding.artifactSha256)) {
      return {state: "invalid", deleteProtectedArtifact: true};
    }
    const verifyBy = Date.parse(receipt.verifyBy);
    return now < verifyBy ? {state: "valid", receipt} : {state: "authorization-unverified", receipt};
  } catch {
    return {state: "invalid", deleteProtectedArtifact: true};
  }
}

export async function revalidateOfflineDownloadAuthorization(
  api: OfflineDownloadAuthorizationRevalidationAPI,
  receipt: OfflineDownloadAuthorizationReceipt,
  context: OfflineDownloadAuthorizationValidationContext
): Promise<OfflineDownloadAuthorizationRevalidationDecision> {
  let scope: ViewerScope;
  try {
    scope = normalizeViewerScope(context.activeViewerScope);
    if (!sameViewerIdentity(scope, normalizeViewerScope(context.binding.storedViewerScope))) {
      return {action: "preserve", outcome: "out-of-scope"};
    }
  } catch {
    return {action: "preserve", outcome: "out-of-scope"};
  }
  const currentValidation = await validateOfflineDownloadAuthorizationReceipt(receipt, context);
  if (currentValidation.state === "out-of-scope") return {action: "preserve", outcome: "out-of-scope"};
  if (currentValidation.state === "invalid") return {action: "delete", outcome: "invalid"};
  let raw: unknown;
  try {
    raw = await api.revalidateOfflineDownloadAuthorization({receipt});
  } catch {
    return {action: "preserve", outcome: "transport-failure"};
  }
  let response: OfflineDownloadAuthorizationRevalidationResponse;
  try {
    response = parseOfflineDownloadAuthorizationRevalidationResponse(raw);
  } catch {
    return {action: "preserve", outcome: "transport-failure"};
  }
  if (response.outcome === "valid-replacement") {
    if (response.receipt.receiptId === receipt.receiptId) return {action: "preserve", outcome: "transport-failure"};
    const validation = await validateOfflineDownloadAuthorizationReceipt(response.receipt, context);
    if (validation.state !== "valid") return {action: "preserve", outcome: "transport-failure"};
    return {action: "replace", receipt: validation.receipt};
  }
  if (response.outcome === "out-of-scope") return {action: "preserve", outcome: response.outcome};
  return {action: "delete", outcome: response.outcome};
}

export function parseOfflineDownloadAuthorizationReceipt(candidate: unknown): OfflineDownloadAuthorizationReceipt {
  const root = exactObject(candidate, [
    "version", "purpose", "receiptId", "viewerScope", "issuer", "preparation", "artifact",
    "lastVerifiedAt", "verifyBy", "signature"
  ]);
  if (root.version !== 1 || root.purpose !== "offline-download-authorization") invalidReceipt();
  const viewerScope = exactObject(root.viewerScope, ["scopeKind", "authority", "accountId", "profileId", "serverId", "authorizationRevision"]);
  if (viewerScope.scopeKind !== "server-bound" || (viewerScope.authority !== "hosted" && viewerScope.authority !== "local")) invalidReceipt();
  const issuer = exactObject(root.issuer, ["serverId", "signingKeyFingerprint"]);
  const preparation = exactObject(root.preparation, ["preparationId", "mediaId", "mediaVersionId", "qualityId"]);
  const artifact = exactObject(root.artifact, ["sha256", "sizeBytes"]);
  for (const value of [root.receiptId, viewerScope.accountId, viewerScope.profileId, viewerScope.serverId,
    viewerScope.authorizationRevision, issuer.serverId, preparation.preparationId, preparation.mediaId,
    preparation.mediaVersionId, preparation.qualityId]) opaqueId(value);
  if (issuer.serverId !== viewerScope.serverId || typeof issuer.signingKeyFingerprint !== "string" ||
      !FINGERPRINT_PATTERN.test(issuer.signingKeyFingerprint)) invalidReceipt();
  if (typeof artifact.sha256 !== "string" || !SHA256_PATTERN.test(artifact.sha256) ||
      typeof artifact.sizeBytes !== "number" || !Number.isSafeInteger(artifact.sizeBytes) || artifact.sizeBytes < 1) invalidReceipt();
  const lastVerifiedAt = canonicalTimestamp(root.lastVerifiedAt);
  const verifyBy = canonicalTimestamp(root.verifyBy);
  if (Date.parse(verifyBy) - Date.parse(lastVerifiedAt) !== THIRTY_DAYS_MILLISECONDS) invalidReceipt();
  if (typeof root.signature !== "string" || !SIGNATURE_PATTERN.test(root.signature)) invalidReceipt();
  return Object.freeze({
    version: 1,
    purpose: "offline-download-authorization",
    receiptId: root.receiptId as string,
    viewerScope: Object.freeze({
      scopeKind: "server-bound",
      authority: viewerScope.authority as "hosted" | "local",
      accountId: viewerScope.accountId as string,
      profileId: viewerScope.profileId as string,
      serverId: viewerScope.serverId as string,
      authorizationRevision: viewerScope.authorizationRevision as string
    }),
    issuer: Object.freeze({serverId: issuer.serverId as string, signingKeyFingerprint: issuer.signingKeyFingerprint}),
    preparation: Object.freeze({
      preparationId: preparation.preparationId as string,
      mediaId: preparation.mediaId as string,
      mediaVersionId: preparation.mediaVersionId as string,
      qualityId: preparation.qualityId as string
    }),
    artifact: Object.freeze({sha256: artifact.sha256, sizeBytes: artifact.sizeBytes}),
    lastVerifiedAt,
    verifyBy,
    signature: root.signature
  });
}

export function canonicalOfflineDownloadAuthorizationPayload(receipt: OfflineDownloadAuthorizationReceipt): string {
  const unsigned = {...receipt} as Record<string, unknown>;
  delete unsigned.signature;
  return JSON.stringify(sortCanonicalJSONValue(unsigned));
}

function parseOfflineDownloadAuthorizationRevalidationResponse(candidate: unknown): OfflineDownloadAuthorizationRevalidationResponse {
  if (!candidate || typeof candidate !== "object" || Array.isArray(candidate)) throw new TypeError("Offline authorization response is invalid.");
  const object = candidate as Record<string, unknown>;
  if (object.outcome === "valid-replacement") {
    const exact = exactObject(object, ["outcome", "receipt"]);
    return {outcome: "valid-replacement", receipt: parseOfflineDownloadAuthorizationReceipt(exact.receipt)};
  }
  const exact = exactObject(object, ["outcome"]);
  if (exact.outcome !== "revoked" && exact.outcome !== "invalid" && exact.outcome !== "out-of-scope") {
    throw new TypeError("Offline authorization response is invalid.");
  }
  return {outcome: exact.outcome};
}

function exactObject(value: unknown, keys: readonly string[]): Record<string, any> {
  if (!value || typeof value !== "object" || Array.isArray(value) || Object.getPrototypeOf(value) !== Object.prototype) invalidReceipt();
  const actual = Object.keys(value as object).sort();
  const expected = [...keys].sort();
  if (actual.length !== expected.length || actual.some((key, index) => key !== expected[index])) invalidReceipt();
  return value as Record<string, any>;
}

function opaqueId(value: unknown): asserts value is string {
  if (typeof value !== "string" || Array.from(value).length < 1 || Array.from(value).length > 128 || !isUnicodeScalarString(value)) invalidReceipt();
}

function canonicalTimestamp(value: unknown): string {
  if (typeof value !== "string" || !RFC3339_UTC_PATTERN.test(value) || !Number.isFinite(Date.parse(value))) invalidReceipt();
  return value;
}

function decodeCanonicalFingerprint(value: string, runtime: ResolvedHostedConnectionRuntimeAdapters): Uint8Array {
  if (!FINGERPRINT_PATTERN.test(value)) invalidReceipt();
  return decodeCanonicalBase64URL(value.slice("sha256:".length), 32, runtime);
}

function isUnicodeScalarString(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const code = value.charCodeAt(index);
    if (code < 0xd800 || code > 0xdfff) continue;
    if (code > 0xdbff || index + 1 >= value.length) return false;
    const low = value.charCodeAt(index + 1);
    if (low < 0xdc00 || low > 0xdfff) return false;
    index += 1;
  }
  return true;
}

function decodeCanonicalBase64URL(value: string, expectedBytes: number, runtime: ResolvedHostedConnectionRuntimeAdapters): Uint8Array {
  if (!/^[A-Za-z0-9_-]+$/u.test(value) || value.includes("=")) invalidReceipt();
  const standard = value.replace(/-/g, "+").replace(/_/g, "/") + "=".repeat((4 - value.length % 4) % 4);
  const decoded = runtime.decodeBase64(standard);
  if (decoded.byteLength !== expectedBytes || runtime.encodeBase64(decoded) !== value) invalidReceipt();
  return decoded;
}

function sortCanonicalJSONValue(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sortCanonicalJSONValue);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(Object.entries(value as Record<string, unknown>)
    .sort(([left], [right]) => left < right ? -1 : left > right ? 1 : 0)
    .map(([key, nested]) => [key, sortCanonicalJSONValue(nested)]));
}

function invalidReceipt(): never {
  throw new TypeError("Offline download authorization receipt is invalid.");
}

function sameViewerIdentity(left: ViewerScope, right: ViewerScope): boolean {
  return left.authority === right.authority && left.accountId === right.accountId &&
    left.profileId === right.profileId && left.serverId === right.serverId;
}
