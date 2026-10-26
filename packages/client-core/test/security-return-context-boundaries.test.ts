import assert from 'node:assert/strict';
import test from 'node:test';
import {createHash, generateKeyPairSync, sign, verify} from 'node:crypto';
import {routeBase64, type RouteCrypto} from '../src/route-identity.ts';
import {parseSecurityReturnCode, verifySecurityConfirmation, verifySecurityContext} from '../src/security-return.ts';

// ACC-LEAF-20: signed Security context and confirmation boundaries on the owner
// SDK parent. The crypto is real Ed25519 through Node, wired into the same
// RouteCrypto seam the registered-sign-in tests use — never a verify-always-true
// stub — and the payloads are producer-shaped canonical bytes.

const keys = generateKeyPairSync('ed25519');
const rawKey = keys.publicKey.export({format: 'der', type: 'spki'}).subarray(-32);
const publicKey = rawKey.toString('base64url');
const fingerprint = createHash('sha256').update(rawKey).digest('base64url');
const serverId = 'srv_' + routeBase64(new Uint8Array(createHash('sha256').update('portico.server.identity.v1\0').update(rawKey).digest()));
const pin = {serverId, publicKey, fingerprint};
const spki = (raw: Uint8Array) => Buffer.concat([Buffer.from('302a300506032b6570032100', 'hex'), Buffer.from(raw)]);
const crypto: RouteCrypto = {
  sha256: async bytes => new Uint8Array(createHash('sha256').update(bytes).digest()),
  verify: async (key, signature, message) => verify(null, message, {key: spki(key), format: 'der', type: 'spki'}, signature),
};
const bytes32 = (fill: number) => Buffer.from(new Uint8Array(32).fill(fill)).toString('base64url');
const now = Date.parse('2026-10-26T12:00:00Z');
const stamp = (offset: number) => new Date(now + offset).toISOString().replace(/\.\d{3}Z$/, 'Z');

const prepare = {
  purpose: 'password_set_or_change' as const,
  target: '',
  origin: 'https://media.example.test',
  openingId: 'opening-00000001',
  taskId: 'task-00000000001',
  operationId: 'operation-000001',
  providerIdentityId: 'provider_identity_1',
  providerRevision: '7',
  nonce: bytes32(3),
  expiresAt: stamp(120000),
};
const expected = {prepare, hostedOrigin: 'https://accounts.example.test', accountId: 'acct_A1'};
const registrationId = 'ret_' + bytes32(5);
const payload = (patch: Record<string, unknown> = {}, order?: readonly string[]) => {
  const value: Record<string, unknown> = {
    settingsScope: 'account', settingsSection: 'profile', purpose: prepare.purpose, target: prepare.target,
    kind: 'portico.security-return-prepare.v1', serverId, publicKey, registrationId,
    origin: prepare.origin, hostedOrigin: expected.hostedOrigin, claimOperationId: 'claim-000000001', accountId: expected.accountId,
    actorBinding: bytes32(4), openingId: prepare.openingId, taskId: prepare.taskId, operationId: prepare.operationId,
    providerIdentityId: prepare.providerIdentityId, providerRevision: prepare.providerRevision, nonce: prepare.nonce,
    issuedAt: stamp(0), expiresAt: prepare.expiresAt, ...patch,
  };
  return order ? Object.fromEntries(order.map(key => [key, value[key]])) : value;
};
const signed = (value: unknown) => {
  const raw = Buffer.from(JSON.stringify(value));
  return {payload: raw.toString('base64url'), signature: sign(null, raw, keys.privateKey).toString('base64url')};
};
const context = signed(payload());

test('the full canonical Account Settings tuple verifies with real Ed25519', async () => {
  const facts = await verifySecurityContext(context, pin, expected, {now, crypto});
  assert.equal(facts.settingsScope, 'account');
  assert.equal(facts.settingsSection, 'profile');
  assert.equal(facts.purpose, 'password_set_or_change');
  assert.equal(facts.openingId, prepare.openingId);
});

test('server scope, server section and a non-credential purpose are refused', async () => {
  for (const patch of [
    {settingsScope: 'server'},
    {settingsSection: 'provider'},
    {purpose: 'playback', target: ''},
  ]) {
    const variant = {...expected, prepare: {...prepare, purpose: (patch.purpose as typeof prepare.purpose) ?? prepare.purpose}};
    await assert.rejects(verifySecurityContext(signed(payload(patch)), pin, variant, {now, crypto}), JSON.stringify(patch));
  }
});

test('a changed original, a wrong kind, a tampered signature and noncanonical bytes are refused', async () => {
  await assert.rejects(verifySecurityContext(context, pin, {...expected, prepare: {...prepare, openingId: 'opening-00000002'}}, {now, crypto}));
  await assert.rejects(verifySecurityContext(signed(payload({kind: 'portico.security-return-prepare.v2'})), pin, expected, {now, crypto}));
  await assert.rejects(verifySecurityContext({...context, signature: 'A'.repeat(86)}, pin, expected, {now, crypto}));
  await assert.rejects(verifySecurityContext({payload: Buffer.from(JSON.stringify(payload(), undefined, 2)).toString('base64url'), signature: context.signature}, pin, expected, {now, crypto}));
  await assert.rejects(verifySecurityContext(signed(payload({extra: 'x'})), pin, expected, {now, crypto}));
  await assert.rejects(verifySecurityContext(signed(payload({}, ['kind', 'settingsScope', 'settingsSection', 'purpose', 'target', 'serverId', 'publicKey', 'registrationId', 'origin', 'hostedOrigin', 'claimOperationId', 'accountId', 'actorBinding', 'openingId', 'taskId', 'operationId', 'providerIdentityId', 'providerRevision', 'nonce', 'issuedAt', 'expiresAt'])), pin, expected, {now, crypto}));
});

test('expired, overlong or future-issued contexts are refused', async () => {
  for (const [issuedAt, expiresAt] of [[stamp(-1000), stamp(-1)], [stamp(0), stamp(300001)], [stamp(61000), stamp(120000)]]) {
    const variant = signed(payload({issuedAt, expiresAt}));
    await assert.rejects(verifySecurityContext(variant, pin, {...expected, prepare: {...prepare, expiresAt}}, {now, crypto}), issuedAt + ' ' + expiresAt);
  }
});

const returned = parseSecurityReturnCode(
  'https://accounts.example.test/auth/portico-return#security=scr_' + 'C'.repeat(43) + '&state=' + bytes32(6) + '&transaction=oid_' + 'T'.repeat(43),
  {origin: 'https://accounts.example.test', state: bytes32(6)},
);
const digest = async (value: Uint8Array) => routeBase64(await crypto.sha256(value));
const confirmation = (patch: Record<string, unknown> = {}) => signed({
  kind: 'portico.security-return-confirm.v1',
  contextDigest: routeBase64(new Uint8Array(createHash('sha256').update(Buffer.from(context.payload, 'base64url')).digest())),
  transactionId: returned.transactionId,
  codeDigest: createHash('sha256').update(returned.code).digest('base64url'),
  issuedAt: stamp(1000), expiresAt: stamp(60000), ...patch,
});

test('a fresh confirmation over the exact context, transaction and code verifies', async () => {
  const confirmed = await verifySecurityConfirmation(confirmation(), pin, context, returned, {now, crypto});
  assert.equal(confirmed.payload, confirmation().payload);
});

test('a wrong transaction, digest, kind, expiry or bytes are refused', async () => {
  for (const variant of [
    confirmation({transactionId: 'oid_' + 'X'.repeat(43)}),
    confirmation({contextDigest: routeBase64(new Uint8Array(32))}),
    confirmation({codeDigest: routeBase64(new Uint8Array(32))}),
    confirmation({kind: 'portico.security-return-confirm.v2'}),
    confirmation({issuedAt: stamp(0), expiresAt: stamp(60001)}),
    confirmation({issuedAt: stamp(-1000), expiresAt: stamp(-1)}),
    confirmation({extra: 'x'}),
    {payload: Buffer.from(JSON.stringify(confirmation(), undefined, 2)).toString('base64url'), signature: confirmation().signature},
    {...confirmation(), signature: 'A'.repeat(86)},
  ]) {
    await assert.rejects(verifySecurityConfirmation(variant, pin, context, returned, {now, crypto}), variant.payload);
  }
});
