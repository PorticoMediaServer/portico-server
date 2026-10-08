import {unreadableServerResponse} from './server-messages.ts';
/**
 * Wire shapes for offline downloads (`server/api/downloads.openapi.yaml`).
 *
 * Parsers only. A download decides whether a client may play bytes with no
 * server in reach, so nothing here is trusted by shape alone: every field is
 * checked, every collection is bounded, and a response that does not match the
 * published contract is rejected rather than half-read.
 */

export type DownloadState = 'queued'|'running'|'ready'|'paused'|'failed'|'unavailable'|'cancelled'|'expired';
export type DownloadAction = 'pause'|'resume'|'cancel'|'retry'|'remove';
export type DownloadOrigin = 'item'|'items'|'container'|'next';
export type DownloadArtifactKind = ''|'source'|'prepared';

export const downloadStates: readonly DownloadState[] = Object.freeze(['queued','running','ready','paused','failed','unavailable','cancelled','expired'] as const);
export const downloadActions: readonly DownloadAction[] = Object.freeze(['pause','resume','cancel','retry','remove'] as const);
/** The closed reason set the server publishes. An unknown code is a contract break. */
export const downloadReasons: readonly string[] = Object.freeze([
  '','downloads_not_allowed','item_deleted','source_unavailable','source_changed','optimized_version_unavailable',
  'optimization_failed','storage_full','artifact_changed','verification_failed','retention_expired','cancelled',
  'removed','preparation_failed','account_disabled','revoked','unknown_receipt','transcoding_disabled',
]);
/** Quality is `original` or a published quality ladder rung id. */
export const downloadQualities: readonly string[] = Object.freeze(['original','2160p','1440p','1080p','720p','480p','360p']);

export type DownloadArtifact = Readonly<{kind: DownloadArtifactKind; ref: string; sha256: string; bytes: number; estimated: boolean; container: string; contentType: string; fileName: string}>;
export type DownloadProgress = Readonly<{bytesDone: number; bytesTotal: number; bytesTotalEstimated: boolean; percent: number; etaSeconds: number|null}>;
export type DownloadPreparation = Readonly<{
  id: string; itemId: string; libraryId: string; profileId: string; quality: string; origin: DownloadOrigin;
  batchId: string; state: DownloadState; reason: string; artifact: DownloadArtifact; progress: DownloadProgress;
  actions: readonly DownloadAction[]; revision: number; createdAt: string; updatedAt: string; readyAt: string; expiresAt: string;
}>;
export type DownloadRejection = Readonly<{itemId: string; code: string}>;
export type DownloadBatch = Readonly<{batchId: string; items: readonly DownloadPreparation[]; rejected: readonly DownloadRejection[]; accepted: number; duplicate: boolean}>;
export type DownloadPage = Readonly<{items: readonly DownloadPreparation[]; nextCursor: string}>;
export type DownloadGrant = Readonly<{preparationId: string; itemId: string; url: string; token: string; issuedAt: string; expiresAt: string; replayWindowSeconds: number; artifact: DownloadArtifact}>;
export type DownloadQualityOption = Readonly<{
  quality: string; label: string; kind: 'source'|'optimized'; targetDisplayHeight: number;
  maxVideoBitrateBps: number; maxAudioBitrateBps: number; estimatedBytes: number; estimated: boolean;
  available: boolean; reason: string; requiresPreparation: boolean;
  preparedProfileId: string; preparedVersionId: string; preparationId: string; preparationState: string;
}>;
export type DownloadSource = Readonly<{container: string; videoCodec: string; audioCodec: string; height: number; durationSeconds: number; bytes: number; available: boolean}>;
export type DownloadStorage = Readonly<{maxPreparedBytes: number; committedBytes: number; remainingBytes: number|null; retentionDays: number}>;
export type DownloadOptionsView = Readonly<{
  itemId: string; kind: string; source: DownloadSource; options: readonly DownloadQualityOption[];
  policy: Readonly<{allowDownloads: boolean; reason: string}>; storage: DownloadStorage;
}>;
export type ReceiptClaims = Readonly<{
  kind: 'portico.download-receipt'; version: '1'; receiptId: string; keyId: string;
  viewer: Readonly<{authority: string; accountId: string; profileId: string; serverId: string}>;
  itemId: string; preparationId: string; quality: string; qualityLabel: string;
  artifact: Readonly<{sha256: string; bytes: number}>; issuedAt: string; expiresAt: string;
}>;
export type DownloadReceipt = Readonly<{receiptId: string; algorithm: 'ed25519'; keyId: string; payload: string; signature: string; claims: ReceiptClaims; revision: number}>;
export type ReceiptOutcome = Readonly<{preparationId: string; receiptId: string; outcome: 'issued'|'renewed'|'refused'; code: string; receipt: DownloadReceipt|null}>;
export type ReceiptKey = Readonly<{id: string; algorithm: 'ed25519'; publicKey: string; createdAt: string; retiredAt: string}>;
export type DownloadRevocation = Readonly<{receiptId: string; itemId: string; profileId: string; reason: string; revokedAt: string; sequence: number}>;
export type DownloadRevocationPage = Readonly<{items: readonly DownloadRevocation[]; nextCursor: string; asOf: string}>;
export type DeferredProgressEntry = Readonly<{itemId: string; positionSeconds: number; watched?: boolean; observedAt: string}>;
export type DeferredProgressOutcome = 'applied'|'stale_observation'|'superseded_online'|'item_deleted'|'invalid_entry'|'observation_in_future';
export type DeferredProgressResult = Readonly<{itemId: string; outcome: DeferredProgressOutcome; positionSeconds: number; watched: boolean; observedAt: string}>;
export type DeferredProgressReceipt = Readonly<{applied: number; conflicts: number; entries: readonly DeferredProgressResult[]}>;
export type DownloadUsage = Readonly<{
  profileId: string; profileBytes: number; profileCount: number; serverBytes: number; serverCount: number;
  distinctArtifactBytes: number; maxPreparedBytes: number; remainingBytes: number|null; retentionDays: number;
  profiles: readonly Readonly<{profileId: string; bytes: number; count: number}>[];
}>;
export type DownloadSettings = Readonly<{maxPreparedBytes: number; retentionDays: number; revision: number}>;

const object = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);
const id = (v: unknown): v is string => typeof v === 'string' && /^[A-Za-z0-9_-]{1,160}$/.test(v);
const text = (v: unknown, max = 512): v is string => typeof v === 'string' && v.length <= max && !/[\x00-\x1f\x7f]/.test(v);
const count = (v: unknown): v is number => Number.isSafeInteger(v) && Number(v) >= 0;
const digest = (v: unknown): v is string => typeof v === 'string' && /^[0-9a-f]{64}$/.test(v);
const base64url = (v: unknown, max: number): v is string => typeof v === 'string' && v.length > 0 && v.length <= max && /^[A-Za-z0-9_-]+$/.test(v);
/** An empty instant means "not yet" and is published as an absent field. */
const instant = (v: unknown, optional = false): v is string => typeof v === 'string' && (optional && v === '' || /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})$/.test(v) && Number.isFinite(Date.parse(v)));

function invalid(): never { throw new Error(unreadableServerResponse); }
function array(v: unknown, max: number): unknown[] { if (!Array.isArray(v) || v.length > max) invalid(); return v; }
function nullableCount(v: unknown): number|null { if (v === null || v === undefined) return null; if (!count(v)) invalid(); return v; }

function artifact(v: unknown): DownloadArtifact {
  if (!object(v) || !['','source','prepared'].includes(String(v.kind ?? '')) || !count(v.bytes) || typeof v.estimated !== 'boolean') invalid();
  const sha = v.sha256 ?? '';
  if (sha !== '' && !digest(sha)) invalid();
  for (const field of ['ref','container','contentType','fileName'] as const) if (!text(v[field] ?? '', 256)) invalid();
  // An exact size is only credible once the bytes exist; an artifact with a
  // digest and no size, or a size it calls estimated, is not a ready artifact.
  if (sha !== '' && (v.estimated || v.bytes < 1)) invalid();
  return Object.freeze({kind: String(v.kind ?? '') as DownloadArtifactKind, ref: String(v.ref ?? ''), sha256: String(sha), bytes: v.bytes, estimated: v.estimated, container: String(v.container ?? ''), contentType: String(v.contentType ?? ''), fileName: String(v.fileName ?? '')});
}

function progress(v: unknown): DownloadProgress {
  if (!object(v) || !count(v.bytesDone) || !count(v.bytesTotal) || typeof v.bytesTotalEstimated !== 'boolean') invalid();
  if (typeof v.percent !== 'number' || !Number.isFinite(v.percent) || v.percent < 0 || v.percent > 100) invalid();
  // CD-26: an estimate can be overtaken; reject overshoot only for exact totals.
  if (v.bytesTotal > 0 && v.bytesDone > v.bytesTotal && v.bytesTotalEstimated === false) invalid();
  const eta = nullableCount(v.etaSeconds);
  return Object.freeze({bytesDone: v.bytesDone, bytesTotal: v.bytesTotal, bytesTotalEstimated: v.bytesTotalEstimated, percent: v.percent, etaSeconds: eta});
}

export function parseDownloadPreparation(raw: unknown): DownloadPreparation {
  if (!object(raw) || !id(raw.id) || !id(raw.itemId) || !id(raw.libraryId) || !id(raw.profileId)) invalid();
  if (!downloadQualities.includes(String(raw.quality))) invalid();
  if (!['item','items','container','next'].includes(String(raw.origin))) invalid();
  if (!downloadStates.includes(String(raw.state) as DownloadState)) invalid();
  if (!downloadReasons.includes(String(raw.reason ?? ''))) invalid();
  if (!count(raw.revision) || raw.revision < 1) invalid();
  if (!instant(raw.createdAt) || !instant(raw.updatedAt) || !instant(raw.readyAt ?? '', true) || !instant(raw.expiresAt ?? '', true)) invalid();
  if (raw.batchId !== undefined && !text(raw.batchId, 128)) invalid();
  const actions = array(raw.actions, 5).map(v => { if (!downloadActions.includes(String(v) as DownloadAction)) invalid(); return String(v) as DownloadAction; });
  if (new Set(actions).size !== actions.length) invalid();
  const state = String(raw.state) as DownloadState;
  const bytes = artifact(raw.artifact);
  // A ready claim must name bytes a client can verify; anything else must not
  // pretend to, so a client cannot be talked into trusting an unhashed file.
  if (state === 'ready' ? bytes.sha256 === '' || bytes.kind === '' : false) invalid();
  return Object.freeze({
    id: raw.id, itemId: raw.itemId, libraryId: raw.libraryId, profileId: raw.profileId,
    quality: String(raw.quality), origin: String(raw.origin) as DownloadOrigin, batchId: String(raw.batchId ?? ''),
    state, reason: String(raw.reason ?? ''), artifact: bytes, progress: progress(raw.progress),
    actions: Object.freeze(actions), revision: raw.revision,
    createdAt: raw.createdAt, updatedAt: raw.updatedAt, readyAt: String(raw.readyAt ?? ''), expiresAt: String(raw.expiresAt ?? ''),
  });
}

export function parseDownloadBatch(raw: unknown): DownloadBatch {
  if (!object(raw) || !text(raw.batchId, 128) || !count(raw.accepted) || typeof raw.duplicate !== 'boolean') invalid();
  const items = array(raw.items, 100).map(parseDownloadPreparation);
  if (new Set(items.map(v => v.id)).size !== items.length) invalid();
  const rejected = array(raw.rejected, 100).map(v => {
    if (!object(v) || !id(v.itemId) || !downloadReasons.includes(String(v.code)) || v.code === '') invalid();
    return Object.freeze({itemId: v.itemId, code: String(v.code)});
  });
  if (raw.accepted > items.length) invalid();
  return Object.freeze({batchId: raw.batchId, items: Object.freeze(items), rejected: Object.freeze(rejected), accepted: raw.accepted, duplicate: raw.duplicate});
}

export function parseDownloadPage(raw: unknown): DownloadPage {
  if (!object(raw) || !text(raw.nextCursor ?? '', 512)) invalid();
  const items = array(raw.items, 200).map(parseDownloadPreparation);
  if (new Set(items.map(v => v.id)).size !== items.length) invalid();
  return Object.freeze({items: Object.freeze(items), nextCursor: String(raw.nextCursor ?? '')});
}

export function parseDownloadGrant(raw: unknown): DownloadGrant {
  if (!object(raw) || !id(raw.preparationId) || !id(raw.itemId) || !base64url(raw.token, 200)) invalid();
  // The URL is a path this server published. A client must never construct one,
  // and an absolute URL here would be an attempt to redirect the transfer.
  if (typeof raw.url !== 'string' || !raw.url.startsWith('/v1/downloads/artifacts/') || raw.url.includes('..')) invalid();
  if (!instant(raw.issuedAt) || !instant(raw.expiresAt) || !count(raw.replayWindowSeconds) || raw.replayWindowSeconds < 1) invalid();
  if (Date.parse(raw.expiresAt) <= Date.parse(raw.issuedAt)) invalid();
  const bytes = artifact(raw.artifact);
  if (bytes.sha256 === '') invalid();
  return Object.freeze({preparationId: raw.preparationId, itemId: raw.itemId, url: raw.url, token: raw.token, issuedAt: raw.issuedAt, expiresAt: raw.expiresAt, replayWindowSeconds: raw.replayWindowSeconds, artifact: bytes});
}

function qualityOption(raw: unknown): DownloadQualityOption {
  if (!object(raw) || !downloadQualities.includes(String(raw.quality)) || !text(raw.label, 128)) invalid();
  if (!['source','optimized'].includes(String(raw.kind))) invalid();
  if (!count(raw.estimatedBytes) || typeof raw.estimated !== 'boolean' || typeof raw.available !== 'boolean' || typeof raw.requiresPreparation !== 'boolean') invalid();
  if (!downloadReasons.includes(String(raw.reason ?? ''))) invalid();
  for (const field of ['targetDisplayHeight','maxVideoBitrateBps','maxAudioBitrateBps'] as const) if (raw[field] !== undefined && !count(raw[field])) invalid();
  for (const field of ['preparedProfileId','preparedVersionId','preparationId'] as const) if (raw[field] !== undefined && raw[field] !== '' && !id(raw[field])) invalid();
  if (raw.preparationState !== undefined && raw.preparationState !== '' && !downloadStates.includes(String(raw.preparationState) as DownloadState)) invalid();
  // An unavailable option owes the client a reason; an available one has none.
  if (raw.available === (String(raw.reason ?? '') !== '')) invalid();
  return Object.freeze({
    quality: String(raw.quality), label: raw.label, kind: String(raw.kind) as 'source'|'optimized',
    targetDisplayHeight: Number(raw.targetDisplayHeight ?? 0), maxVideoBitrateBps: Number(raw.maxVideoBitrateBps ?? 0), maxAudioBitrateBps: Number(raw.maxAudioBitrateBps ?? 0),
    estimatedBytes: raw.estimatedBytes, estimated: raw.estimated, available: raw.available, reason: String(raw.reason ?? ''),
    requiresPreparation: raw.requiresPreparation, preparedProfileId: String(raw.preparedProfileId ?? ''), preparedVersionId: String(raw.preparedVersionId ?? ''),
    preparationId: String(raw.preparationId ?? ''), preparationState: String(raw.preparationState ?? ''),
  });
}

function storage(raw: unknown): DownloadStorage {
  if (!object(raw) || !count(raw.maxPreparedBytes) || !count(raw.committedBytes) || !count(raw.retentionDays) || raw.retentionDays < 1) invalid();
  const remaining = nullableCount(raw.remainingBytes);
  // 0 means unlimited, and an unlimited ceiling has no remaining figure.
  if (raw.maxPreparedBytes === 0 ? remaining !== null : remaining === null) invalid();
  return Object.freeze({maxPreparedBytes: raw.maxPreparedBytes, committedBytes: raw.committedBytes, remainingBytes: remaining, retentionDays: raw.retentionDays});
}

export function parseDownloadOptions(raw: unknown, itemId: string): DownloadOptionsView {
  if (!object(raw) || raw.itemId !== itemId || !text(raw.kind, 64)) invalid();
  const source = raw.source;
  if (!object(source) || !text(source.container ?? '', 64) || !count(source.bytes) || typeof source.available !== 'boolean') invalid();
  if (typeof source.durationSeconds !== 'number' || !Number.isFinite(source.durationSeconds) || source.durationSeconds < 0) invalid();
  for (const field of ['videoCodec','audioCodec'] as const) if (source[field] !== undefined && !text(source[field], 64)) invalid();
  if (source.height !== undefined && !count(source.height)) invalid();
  const policy = raw.policy;
  if (!object(policy) || typeof policy.allowDownloads !== 'boolean' || !downloadReasons.includes(String(policy.reason ?? ''))) invalid();
  if (policy.allowDownloads === (String(policy.reason ?? '') !== '')) invalid();
  const options = array(raw.options, 16).map(qualityOption);
  if (new Set(options.map(v => v.quality)).size !== options.length) invalid();
  if (!options.length || options[0].quality !== 'original') invalid();
  return Object.freeze({
    itemId, kind: raw.kind,
    source: Object.freeze({container: String(source.container ?? ''), videoCodec: String(source.videoCodec ?? ''), audioCodec: String(source.audioCodec ?? ''), height: Number(source.height ?? 0), durationSeconds: source.durationSeconds, bytes: source.bytes, available: source.available}),
    options: Object.freeze(options),
    policy: Object.freeze({allowDownloads: policy.allowDownloads, reason: String(policy.reason ?? '')}),
    storage: storage(raw.storage),
  });
}

/**
 * Decodes a receipt envelope and returns the claims plus the exact bytes that
 * were signed. Verification itself is a platform Ed25519 operation on
 * `signedBytes` against the key whose id is `claims.keyId`; this function does
 * the parsing and the shape checks, and refuses anything inconsistent.
 */
export function decodeDownloadReceipt(raw: unknown): {receipt: DownloadReceipt; signedBytes: Uint8Array; signatureBytes: Uint8Array} {
  if (!object(raw) || raw.algorithm !== 'ed25519' || !id(raw.receiptId) || !id(raw.keyId)) invalid();
  if (!base64url(raw.payload, 16384) || !base64url(raw.signature, 128)) invalid();
  if (!count(raw.revision) || raw.revision < 1) invalid();
  const signedBytes = decodeBase64url(raw.payload);
  const signatureBytes = decodeBase64url(raw.signature);
  if (signatureBytes.length !== 64) invalid();
  let decoded: unknown;
  try { decoded = JSON.parse(new TextDecoder().decode(signedBytes)); } catch { invalid(); }
  const claims = parseReceiptClaims(decoded);
  // The envelope may not disagree with the signed document about anything.
  if (claims.receiptId !== raw.receiptId || claims.keyId !== raw.keyId) invalid();
  const parsed = object(raw.claims) ? parseReceiptClaims(raw.claims) : claims;
  if (JSON.stringify(parsed) !== JSON.stringify(claims)) invalid();
  return {receipt: Object.freeze({receiptId: raw.receiptId, algorithm: 'ed25519', keyId: raw.keyId, payload: raw.payload, signature: raw.signature, claims, revision: raw.revision}), signedBytes, signatureBytes};
}

export function parseReceiptClaims(raw: unknown): ReceiptClaims {
  if (!object(raw) || raw.kind !== 'portico.download-receipt' || raw.version !== '1') invalid();
  if (!id(raw.receiptId) || !id(raw.keyId) || !id(raw.itemId) || !id(raw.preparationId)) invalid();
  if (!downloadQualities.includes(String(raw.quality)) || !text(raw.qualityLabel ?? '', 128)) invalid();
  if (!instant(raw.issuedAt) || !instant(raw.expiresAt) || Date.parse(raw.expiresAt) <= Date.parse(raw.issuedAt)) invalid();
  const viewer = raw.viewer;
  if (!object(viewer) || !['local','hosted'].includes(String(viewer.authority)) || !id(viewer.accountId) || !id(viewer.profileId) || !id(viewer.serverId)) invalid();
  const bytes = raw.artifact;
  if (!object(bytes) || !digest(bytes.sha256) || !count(bytes.bytes) || bytes.bytes < 1) invalid();
  return Object.freeze({
    kind: 'portico.download-receipt', version: '1', receiptId: raw.receiptId, keyId: raw.keyId,
    viewer: Object.freeze({authority: String(viewer.authority), accountId: viewer.accountId, profileId: viewer.profileId, serverId: viewer.serverId}),
    itemId: raw.itemId, preparationId: raw.preparationId, quality: String(raw.quality), qualityLabel: String(raw.qualityLabel ?? ''),
    artifact: Object.freeze({sha256: bytes.sha256, bytes: bytes.bytes}), issuedAt: raw.issuedAt, expiresAt: raw.expiresAt,
  });
}

/**
 * Whether a receipt still authorizes offline playback for this viewer at `now`.
 * A signature check is required as well; this is the half a client can get
 * wrong by forgetting, so it is one function.
 */
export function receiptUsable(claims: ReceiptClaims, viewer: {authority: string; accountId: string; profileId: string; serverId: string}, now: Date, revoked: ReadonlySet<string> = new Set()): boolean {
  if (revoked.has(claims.receiptId)) return false;
  if (claims.viewer.authority !== viewer.authority || claims.viewer.accountId !== viewer.accountId) return false;
  if (claims.viewer.profileId !== viewer.profileId || claims.viewer.serverId !== viewer.serverId) return false;
  return now.getTime() < Date.parse(claims.expiresAt);
}

export function parseReceiptOutcomes(raw: unknown): readonly ReceiptOutcome[] {
  if (!object(raw)) invalid();
  const results = array(raw.results, 100).map(v => {
    if (!object(v) || !['issued','renewed','refused'].includes(String(v.outcome))) invalid();
    if (!downloadReasons.includes(String(v.code ?? ''))) invalid();
    if (v.preparationId !== undefined && v.preparationId !== '' && !id(v.preparationId)) invalid();
    if (v.receiptId !== undefined && v.receiptId !== '' && !id(v.receiptId)) invalid();
    const refused = v.outcome === 'refused';
    if (refused ? v.receipt != null || !v.code : v.receipt == null) invalid();
    return Object.freeze({
      preparationId: String(v.preparationId ?? ''), receiptId: String(v.receiptId ?? ''),
      outcome: String(v.outcome) as 'issued'|'renewed'|'refused', code: String(v.code ?? ''),
      receipt: refused ? null : decodeDownloadReceipt(v.receipt).receipt,
    });
  });
  return Object.freeze(results);
}

export function parseReceiptKeys(raw: unknown): readonly ReceiptKey[] {
  if (!object(raw)) invalid();
  const keys = array(raw.keys, 32).map(v => {
    if (!object(v) || !id(v.id) || v.algorithm !== 'ed25519' || !base64url(v.publicKey, 64) || !instant(v.createdAt) || !instant(v.retiredAt ?? '', true)) invalid();
    if (decodeBase64url(v.publicKey).length !== 32) invalid();
    return Object.freeze({id: v.id, algorithm: 'ed25519' as const, publicKey: v.publicKey, createdAt: v.createdAt, retiredAt: String(v.retiredAt ?? '')});
  });
  if (new Set(keys.map(v => v.id)).size !== keys.length) invalid();
  return Object.freeze(keys);
}

export function parseRevocationPage(raw: unknown): DownloadRevocationPage {
  if (!object(raw) || !text(raw.nextCursor ?? '', 64) || !instant(raw.asOf)) invalid();
  const items = array(raw.items, 500).map(v => {
    if (!object(v) || !id(v.receiptId) || !id(v.itemId) || !id(v.profileId) || !instant(v.revokedAt)) invalid();
    if (!downloadReasons.includes(String(v.reason)) || v.reason === '') invalid();
    if (!count(v.sequence) || v.sequence < 1) invalid();
    return Object.freeze({receiptId: v.receiptId, itemId: v.itemId, profileId: v.profileId, reason: String(v.reason), revokedAt: v.revokedAt, sequence: v.sequence});
  });
  // The cursor is a sequence, so the page must be ordered for it to mean
  // anything; an out-of-order page would silently lose revocations.
  for (let i = 1; i < items.length; i++) if (items[i].sequence <= items[i-1].sequence) invalid();
  if (String(raw.nextCursor ?? '') !== '' && items.length && String(raw.nextCursor) !== String(items[items.length-1].sequence)) invalid();
  return Object.freeze({items: Object.freeze(items), nextCursor: String(raw.nextCursor ?? ''), asOf: raw.asOf});
}

export function parseDeferredProgressReceipt(raw: unknown): DeferredProgressReceipt {
  if (!object(raw) || !count(raw.applied) || !count(raw.conflicts)) invalid();
  const outcomes: readonly string[] = ['applied','stale_observation','superseded_online','item_deleted','invalid_entry','observation_in_future'];
  const entries = array(raw.entries, 100).map(v => {
    if (!object(v) || !id(v.itemId) && v.itemId !== '' || !outcomes.includes(String(v.outcome)) || typeof v.watched !== 'boolean') invalid();
    if (typeof v.positionSeconds !== 'number' || !Number.isFinite(v.positionSeconds) || v.positionSeconds < 0) invalid();
    if (v.observedAt !== undefined && !instant(v.observedAt, true)) invalid();
    return Object.freeze({itemId: String(v.itemId), outcome: String(v.outcome) as DeferredProgressOutcome, positionSeconds: v.positionSeconds, watched: v.watched, observedAt: String(v.observedAt ?? '')});
  });
  if (raw.applied + raw.conflicts !== entries.length) invalid();
  if (entries.filter(v => v.outcome === 'applied').length !== raw.applied) invalid();
  return Object.freeze({applied: raw.applied, conflicts: raw.conflicts, entries: Object.freeze(entries)});
}

export function parseDownloadUsage(raw: unknown): DownloadUsage {
  if (!object(raw) || !id(raw.profileId)) invalid();
  if (!count(raw.profileBytes) || !count(raw.profileCount) || !count(raw.serverBytes) || !count(raw.serverCount) || !count(raw.distinctArtifactBytes) || !count(raw.maxPreparedBytes) || !count(raw.retentionDays)) invalid();
  if (raw.retentionDays < 1) invalid();
  if (raw.profileBytes > raw.serverBytes || raw.profileCount > raw.serverCount) invalid();
  const remaining = nullableCount(raw.remainingBytes);
  if (raw.maxPreparedBytes === 0 ? remaining !== null : remaining === null) invalid();
  const profiles = array(raw.profiles, 256).map(v => {
    if (!object(v) || !id(v.profileId) || !count(v.bytes) || !count(v.count)) invalid();
    return Object.freeze({profileId: v.profileId, bytes: v.bytes, count: v.count});
  });
  if (new Set(profiles.map(v => v.profileId)).size !== profiles.length) invalid();
  return Object.freeze({
    profileId: raw.profileId, profileBytes: raw.profileBytes, profileCount: raw.profileCount,
    serverBytes: raw.serverBytes, serverCount: raw.serverCount, distinctArtifactBytes: raw.distinctArtifactBytes,
    maxPreparedBytes: raw.maxPreparedBytes, remainingBytes: remaining, retentionDays: raw.retentionDays, profiles: Object.freeze(profiles),
  });
}

export function parseDownloadSettings(raw: unknown): DownloadSettings {
  if (!object(raw) || !count(raw.maxPreparedBytes) || !count(raw.retentionDays) || !count(raw.revision)) invalid();
  if (raw.retentionDays < 1 || raw.retentionDays > 365 || raw.revision < 1) invalid();
  return Object.freeze({maxPreparedBytes: raw.maxPreparedBytes, retentionDays: raw.retentionDays, revision: raw.revision});
}

/** Validates one deferred observation before it is queued for a later send. */
export function validDeferredProgressEntry(v: DeferredProgressEntry): boolean {
  if (!id(v.itemId) || !instant(v.observedAt)) return false;
  if (typeof v.positionSeconds !== 'number' || !Number.isFinite(v.positionSeconds) || v.positionSeconds < 0) return false;
  return v.watched === undefined || typeof v.watched === 'boolean';
}

/** Whether an action the client is about to send is one the server published. */
export const downloadActionOffered = (preparation: DownloadPreparation, action: DownloadAction): boolean => preparation.actions.includes(action);

export function downloadReasonMessage(reason: string): string {
  const reasons: Record<string, string> = {
    downloads_not_allowed: 'Downloads are turned off for this profile.',
    item_deleted: 'This title is no longer in the library.',
    source_unavailable: 'The original file is not reachable. Reconnect the source and retry.',
    source_changed: 'The original file changed. Prepare the download again.',
    optimized_version_unavailable: 'No optimized copy at this quality exists yet. Ask the server owner to prepare one, or download the original.',
    optimization_failed: 'The server could not prepare this quality. Check Logs & diagnostics.',
    storage_full: 'The server has no room left for prepared downloads.',
    artifact_changed: 'The prepared file changed. Download it again.',
    verification_failed: 'The downloaded file did not match its verification hash.',
    retention_expired: 'This download expired and was removed from the server.',
    cancelled: 'Cancelled.',
    removed: 'Removed.',
    preparation_failed: 'The download could not be prepared. Retry.',
    account_disabled: 'This account is no longer active.',
    revoked: 'Offline access to this download was withdrawn.',
    unknown_receipt: 'This server does not recognise that download.',
    transcoding_disabled: 'The server owner has turned conversions off, so only the original can be downloaded.',
  };
  return reasons[reason] ?? (reason ? reason.replace(/_/g, ' ') : '');
}

export const downloadBytes = (n: number): string => n < 1048576 ? `${Math.round(n/1024)} KiB` : n < 1073741824 ? `${(n/1048576).toFixed(1)} MiB` : `${(n/1073741824).toFixed(2)} GiB`;

function decodeBase64url(v: string): Uint8Array {
  const padded = v.replace(/-/g, '+').replace(/_/g, '/') + '='.repeat((4 - v.length % 4) % 4);
  const binary = atob(padded);
  const out = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) out[i] = binary.charCodeAt(i);
  return out;
}
