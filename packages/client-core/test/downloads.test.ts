import test from 'node:test';
import assert from 'node:assert/strict';
import {
  parseDownloadPreparation, parseDownloadBatch, parseDownloadPage, parseDownloadGrant, parseDownloadOptions,
  decodeDownloadReceipt, parseReceiptOutcomes, parseReceiptKeys, parseRevocationPage, parseDeferredProgressReceipt,
  parseDownloadUsage, parseDownloadSettings, validDeferredProgressEntry, downloadActionOffered, receiptUsable,
  downloadReasonMessage, downloadBytes,
} from '../src/downloads.ts';

const sha = 'a'.repeat(64);
const encode = (v: unknown) => Buffer.from(JSON.stringify(v), 'utf8').toString('base64url');
const signature = 'A'.repeat(86);
const publicKey = 'B'.repeat(43);

function preparation(): any {
  return {
    id: 'prep', itemId: 'item', libraryId: 'lib', profileId: 'profile', quality: '1080p', origin: 'item',
    batchId: 'batch', state: 'ready', reason: '',
    artifact: {kind: 'prepared', ref: 'version', sha256: sha, bytes: 4096, estimated: false, container: 'mp4', contentType: 'video/mp4', fileName: 'prep.mp4'},
    progress: {bytesDone: 4096, bytesTotal: 4096, bytesTotalEstimated: false, percent: 100, etaSeconds: null},
    actions: ['remove'], revision: 3,
    createdAt: '2026-09-16T10:00:00Z', updatedAt: '2026-09-16T10:05:00Z', readyAt: '2026-09-16T10:05:00Z', expiresAt: '2026-10-16T10:05:00Z',
  };
}

function options(): any {
  return {
    itemId: 'item', kind: 'movie',
    source: {container: 'mp4', videoCodec: 'h264', audioCodec: 'aac', height: 1080, durationSeconds: 5400, bytes: 8_000_000_000, available: true},
    options: [
      {quality: 'original', label: 'Original', kind: 'source', targetDisplayHeight: 1080, estimatedBytes: 8_000_000_000, estimated: false, available: true, reason: '', requiresPreparation: false},
      {quality: '720p', label: '720p', kind: 'optimized', targetDisplayHeight: 720, maxVideoBitrateBps: 4_000_000, maxAudioBitrateBps: 128_000, estimatedBytes: 2_800_000_000, estimated: true, available: false, reason: 'optimized_version_unavailable', requiresPreparation: false, preparedProfileId: 'portable-720-v3'},
    ],
    policy: {allowDownloads: true, reason: ''},
    storage: {maxPreparedBytes: 0, committedBytes: 12, remainingBytes: null, retentionDays: 30},
  };
}

function claims(): any {
  return {
    kind: 'portico.download-receipt', version: '1', receiptId: 'receipt', keyId: 'key',
    viewer: {authority: 'local', accountId: 'account', profileId: 'profile', serverId: 'server'},
    itemId: 'item', preparationId: 'prep', quality: '1080p', qualityLabel: '1080p',
    artifact: {sha256: sha, bytes: 4096}, issuedAt: '2026-09-16T10:00:00Z', expiresAt: '2026-10-16T10:00:00Z',
  };
}
const envelope = (c = claims()) => ({receiptId: c.receiptId, algorithm: 'ed25519', keyId: c.keyId, payload: encode(c), signature, claims: c, revision: 1});

test('a preparation is read only when every published invariant holds', () => {
  const parsed = parseDownloadPreparation(preparation());
  assert.equal(parsed.artifact.sha256, sha);
  assert.deepEqual([...parsed.actions], ['remove']);
  assert(Object.isFrozen(parsed.actions));
  assert.equal(downloadActionOffered(parsed, 'remove'), true);
  assert.equal(downloadActionOffered(parsed, 'retry'), false);
  for (const change of [
    (v: any) => v.state = 'finished',
    (v: any) => v.quality = '4320p',
    (v: any) => v.reason = 'made_up_code',
    (v: any) => v.origin = 'guess',
    (v: any) => v.actions = ['pause', 'pause'],
    (v: any) => v.actions = ['detonate'],
    (v: any) => v.revision = 0,
    (v: any) => v.progress.bytesDone = v.progress.bytesTotal + 1,
    (v: any) => v.progress.percent = 101,
    (v: any) => v.artifact.sha256 = 'not-a-digest',
    // A ready claim without a verification hash is the dangerous case: it would
    // invite a client to trust bytes it cannot check.
    (v: any) => v.artifact.sha256 = '',
    (v: any) => v.artifact.estimated = true,
    (v: any) => v.createdAt = 'yesterday',
  ]) {
    const v = preparation();
    change(v);
    assert.throws(() => parseDownloadPreparation(v), `accepted ${JSON.stringify(v)}`);
  }
});

test('a queued preparation carries an estimate and no hash', () => {
  const v = preparation();
  Object.assign(v, {state: 'queued', actions: ['pause', 'cancel'], readyAt: '', expiresAt: ''});
  v.artifact = {kind: '', ref: '', sha256: '', bytes: 2_800_000_000, estimated: true, container: '', contentType: '', fileName: ''};
  v.progress = {bytesDone: 0, bytesTotal: 2_800_000_000, bytesTotalEstimated: true, percent: 0, etaSeconds: null};
  const parsed = parseDownloadPreparation(v);
  assert.equal(parsed.artifact.estimated, true);
  assert.equal(parsed.expiresAt, '');
});

test('CD-26: estimated totals may be overtaken; exact totals may not', () => {
  const estimated = preparation();
  estimated.progress = {bytesDone: 3_000_000_000, bytesTotal: 2_800_000_000, bytesTotalEstimated: true, percent: 100, etaSeconds: null};
  assert.equal(parseDownloadPreparation(estimated).progress.bytesDone, 3_000_000_000);
  const exact = preparation();
  exact.progress = {bytesDone: 4097, bytesTotal: 4096, bytesTotalEstimated: false, percent: 100, etaSeconds: null};
  assert.throws(() => parseDownloadPreparation(exact));
  // Ready artifact digest/size requirements remain enforced.
  const noHash = preparation();
  noHash.artifact = {...noHash.artifact, sha256: ''};
  assert.throws(() => parseDownloadPreparation(noHash));
});

test('a batch bounds its targets and cannot claim more accepted than it returned', () => {
  const batch = parseDownloadBatch({batchId: 'batch', items: [preparation()], rejected: [{itemId: 'other', code: 'source_unavailable'}], accepted: 1, duplicate: false});
  assert.equal(batch.items.length, 1);
  assert.equal(batch.rejected[0].code, 'source_unavailable');
  assert.throws(() => parseDownloadBatch({batchId: 'batch', items: [preparation()], rejected: [], accepted: 2, duplicate: false}));
  assert.throws(() => parseDownloadBatch({batchId: 'batch', items: [preparation(), preparation()], rejected: [], accepted: 2, duplicate: false}));
  assert.throws(() => parseDownloadBatch({batchId: 'batch', items: [], rejected: [{itemId: 'other', code: ''}], accepted: 0, duplicate: false}));
  const page = parseDownloadPage({items: [preparation()], nextCursor: ''});
  assert.equal(page.nextCursor, '');
});

test('a grant URL is a server path and never an absolute redirect', () => {
  const good = {preparationId: 'prep', itemId: 'item', url: '/v1/downloads/artifacts/tok-1', token: 'tok-1', issuedAt: '2026-09-16T10:00:00Z', expiresAt: '2026-09-16T10:10:00Z', replayWindowSeconds: 600, artifact: preparation().artifact};
  assert.equal(parseDownloadGrant(good).replayWindowSeconds, 600);
  for (const url of ['https://evil.example/v1/downloads/artifacts/tok', '/v1/downloads/artifacts/../../etc', '/v1/media/tok', '']) {
    assert.throws(() => parseDownloadGrant({...good, url}), `accepted ${url}`);
  }
  assert.throws(() => parseDownloadGrant({...good, expiresAt: good.issuedAt}));
  assert.throws(() => parseDownloadGrant({...good, artifact: {...good.artifact, sha256: ''}}));
});

test('download options publish the original first and explain every unavailable rung', () => {
  const view = parseDownloadOptions(options(), 'item');
  assert.equal(view.options[0].quality, 'original');
  assert.equal(view.options[1].reason, 'optimized_version_unavailable');
  assert.equal(view.storage.remainingBytes, null);
  assert.throws(() => parseDownloadOptions(options(), 'other'));
  for (const change of [
    // An available option with a reason, or an unavailable one without, would
    // leave a client with nothing to say.
    (v: any) => v.options[1].available = true,
    (v: any) => v.options[0].reason = 'storage_full',
    (v: any) => v.options.reverse(),
    (v: any) => v.options.push(v.options[0]),
    (v: any) => v.policy = {allowDownloads: false, reason: ''},
    (v: any) => v.storage.remainingBytes = 10,
    (v: any) => v.storage = {maxPreparedBytes: 100, committedBytes: 0, remainingBytes: null, retentionDays: 30},
  ]) {
    const v = options();
    change(v);
    assert.throws(() => parseDownloadOptions(v, 'item'));
  }
});

test('a receipt envelope may not disagree with the document it signed', () => {
  const {receipt, signedBytes, signatureBytes} = decodeDownloadReceipt(envelope());
  assert.equal(receipt.claims.artifact.sha256, sha);
  assert.equal(signatureBytes.length, 64);
  assert.equal(new TextDecoder().decode(signedBytes), JSON.stringify(claims()));
  // The signature covers `payload`; a client that read `claims` instead would
  // be reading an unsigned copy, so a disagreement is refused outright.
  assert.throws(() => decodeDownloadReceipt({...envelope(), claims: {...claims(), itemId: 'other'}}));
  assert.throws(() => decodeDownloadReceipt({...envelope(), receiptId: 'different'}));
  assert.throws(() => decodeDownloadReceipt({...envelope(), keyId: 'different'}));
  assert.throws(() => decodeDownloadReceipt({...envelope(), algorithm: 'none'}));
  assert.throws(() => decodeDownloadReceipt({...envelope(), signature: 'AAAA'}));
  const backwards = claims();
  backwards.expiresAt = backwards.issuedAt;
  assert.throws(() => decodeDownloadReceipt(envelope(backwards)));
});

test('a receipt is usable only for its own viewer, in date, and not revoked', () => {
  const {receipt} = decodeDownloadReceipt(envelope());
  const viewer = {authority: 'local', accountId: 'account', profileId: 'profile', serverId: 'server'};
  assert.equal(receiptUsable(receipt.claims, viewer, new Date('2026-09-20T00:00:00Z')), true);
  assert.equal(receiptUsable(receipt.claims, viewer, new Date('2026-11-20T00:00:00Z')), false);
  assert.equal(receiptUsable(receipt.claims, {...viewer, profileId: 'other'}, new Date('2026-09-20T00:00:00Z')), false);
  assert.equal(receiptUsable(receipt.claims, {...viewer, serverId: 'other'}, new Date('2026-09-20T00:00:00Z')), false);
  assert.equal(receiptUsable(receipt.claims, viewer, new Date('2026-09-20T00:00:00Z'), new Set(['receipt'])), false);
});

test('receipt outcomes pair a receipt with issue and renewal, and a reason with refusal', () => {
  const results = parseReceiptOutcomes({results: [
    {preparationId: 'prep', receiptId: 'receipt', outcome: 'issued', receipt: envelope()},
    {receiptId: 'gone', outcome: 'refused', code: 'item_deleted'},
  ]});
  assert.equal(results[0].receipt?.receiptId, 'receipt');
  assert.equal(results[1].receipt, null);
  assert.throws(() => parseReceiptOutcomes({results: [{receiptId: 'gone', outcome: 'refused', code: 'item_deleted', receipt: envelope()}]}));
  assert.throws(() => parseReceiptOutcomes({results: [{receiptId: 'gone', outcome: 'refused', code: ''}]}));
  assert.throws(() => parseReceiptOutcomes({results: [{receiptId: 'a', outcome: 'renewed'}]}));
});

test('verification keys are 32 bytes and unique', () => {
  const keys = parseReceiptKeys({keys: [{id: 'key', algorithm: 'ed25519', publicKey, createdAt: '2026-09-01T00:00:00Z', retiredAt: ''}]});
  assert.equal(keys[0].id, 'key');
  assert.throws(() => parseReceiptKeys({keys: [{id: 'key', algorithm: 'ed25519', publicKey: 'AAAA', createdAt: '2026-09-01T00:00:00Z'}]}));
  assert.throws(() => parseReceiptKeys({keys: [{id: 'key', algorithm: 'hmac', publicKey, createdAt: '2026-09-01T00:00:00Z'}]}));
});

test('a revocation page must be ordered, because its cursor is a sequence', () => {
  const page = parseRevocationPage({items: [
    {receiptId: 'a', itemId: 'item', profileId: 'profile', reason: 'removed', revokedAt: '2026-09-16T10:00:00Z', sequence: 4},
    {receiptId: 'b', itemId: 'item', profileId: 'profile', reason: 'retention_expired', revokedAt: '2026-09-16T11:00:00Z', sequence: 9},
  ], nextCursor: '9', asOf: '2026-09-16T12:00:00Z'});
  assert.equal(page.items.length, 2);
  assert.equal(page.nextCursor, '9');
  assert.throws(() => parseRevocationPage({items: [
    {receiptId: 'b', itemId: 'item', profileId: 'profile', reason: 'removed', revokedAt: '2026-09-16T11:00:00Z', sequence: 9},
    {receiptId: 'a', itemId: 'item', profileId: 'profile', reason: 'removed', revokedAt: '2026-09-16T10:00:00Z', sequence: 4},
  ], nextCursor: '4', asOf: '2026-09-16T12:00:00Z'}));
  assert.throws(() => parseRevocationPage({items: [
    {receiptId: 'a', itemId: 'item', profileId: 'profile', reason: 'removed', revokedAt: '2026-09-16T10:00:00Z', sequence: 4},
  ], nextCursor: '99', asOf: '2026-09-16T12:00:00Z'}));
});

test('deferred progress reports every entry and its counts agree', () => {
  const receipt = parseDeferredProgressReceipt({applied: 1, conflicts: 1, entries: [
    {itemId: 'item', outcome: 'applied', positionSeconds: 600, watched: false, observedAt: '2026-09-14T09:12:00Z'},
    {itemId: 'other', outcome: 'stale_observation', positionSeconds: 42, watched: true, observedAt: '2026-09-13T09:12:00Z'},
  ]});
  assert.equal(receipt.entries.length, 2);
  assert.throws(() => parseDeferredProgressReceipt({applied: 2, conflicts: 0, entries: [
    {itemId: 'item', outcome: 'applied', positionSeconds: 1, watched: false},
  ]}));
  assert.throws(() => parseDeferredProgressReceipt({applied: 1, conflicts: 0, entries: [
    {itemId: 'item', outcome: 'no_such_outcome', positionSeconds: 1, watched: false},
  ]}));
  assert.equal(validDeferredProgressEntry({itemId: 'item', positionSeconds: 12, observedAt: '2026-09-14T09:12:00Z'}), true);
  assert.equal(validDeferredProgressEntry({itemId: 'item', positionSeconds: -1, observedAt: '2026-09-14T09:12:00Z'}), false);
  assert.equal(validDeferredProgressEntry({itemId: 'item', positionSeconds: Number.NaN, observedAt: '2026-09-14T09:12:00Z'}), false);
  assert.equal(validDeferredProgressEntry({itemId: 'item', positionSeconds: 1, observedAt: 'soon'}), false);
});

test('usage and settings enforce the unlimited-ceiling contract', () => {
  const usage = parseDownloadUsage({profileId: 'profile', profileBytes: 10, profileCount: 1, serverBytes: 20, serverCount: 2, distinctArtifactBytes: 20, maxPreparedBytes: 100, remainingBytes: 80, retentionDays: 30, profiles: [{profileId: 'profile', bytes: 10, count: 1}]});
  assert.equal(usage.remainingBytes, 80);
  // A profile cannot hold more than the server it is part of.
  assert.throws(() => parseDownloadUsage({profileId: 'profile', profileBytes: 30, profileCount: 1, serverBytes: 20, serverCount: 2, distinctArtifactBytes: 20, maxPreparedBytes: 0, remainingBytes: null, retentionDays: 30, profiles: []}));
  assert.throws(() => parseDownloadUsage({profileId: 'profile', profileBytes: 1, profileCount: 1, serverBytes: 20, serverCount: 2, distinctArtifactBytes: 20, maxPreparedBytes: 0, remainingBytes: 5, retentionDays: 30, profiles: []}));
  assert.deepEqual(parseDownloadSettings({maxPreparedBytes: 0, retentionDays: 30, revision: 1}), {maxPreparedBytes: 0, retentionDays: 30, revision: 1});
  assert.throws(() => parseDownloadSettings({maxPreparedBytes: 0, retentionDays: 0, revision: 1}));
  assert.throws(() => parseDownloadSettings({maxPreparedBytes: 0, retentionDays: 400, revision: 1}));
});

test('reason codes render a sentence and sizes render a unit', () => {
  assert.match(downloadReasonMessage('storage_full'), /no room/);
  assert.match(downloadReasonMessage('optimized_version_unavailable'), /original/);
  assert.equal(downloadReasonMessage(''), '');
  assert.equal(downloadReasonMessage('brand_new_code'), 'brand new code');
  assert.equal(downloadBytes(2048), '2 KiB');
  assert.equal(downloadBytes(5_368_709_120), '5.00 GiB');
});
