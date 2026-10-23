import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {createPrivateKey, sign} from 'node:crypto';
import {verifyOwnerClaimStart, type OwnerStartExpectation} from '../src/owner-claim-start.ts';
import {transportJSON} from '../src/index.ts';
import {routeBase64, routeBytes, webRouteCrypto} from '../src/route-identity.ts';

// Consume the immutable canonical Go fixture in MAIN, not a second copied
// success fixture. Its public interoperability key is explicitly not a secret.
const vector = JSON.parse(readFileSync(new URL('../../../server/internal/claimweb/testdata/owner_start.json', import.meta.url), 'utf8'));
const claim = JSON.parse(vector.claimPayload), start = JSON.parse(vector.startPayload), now = Date.parse('2030-01-02T03:04:07Z');
const key = createPrivateKey({key: Buffer.concat([Buffer.from('302e020100300506032b657004220420', 'hex'), Buffer.alloc(32, 0x42)]), format: 'der', type: 'pkcs8'});
function signed(value: unknown) {
  const raw = Buffer.from(transportJSON(value));
  return {payload: raw.toString('base64url'), signature: sign(null, raw, key).toString('base64url')};
}
async function expectation(): Promise<OwnerStartExpectation> {
  return {pin: {serverId: claim.serverId, publicKey: claim.publicKey, fingerprint: routeBase64(await webRouteCrypto.sha256(routeBytes(claim.publicKey, 32)))},
    origin: claim.address, hostedOrigin: claim.audience, operationId: claim.operationId, localGeneration: claim.localGeneration,
    state: start.state, codeChallenge: start.codeChallenge, installationId: start.installationId};
}
test('actual canonical Go full claim/start vector verifies unchanged under the pinned Server key', async () => {
  const out = await verifyOwnerClaimStart(vector.claim, vector.start, await expectation(), now);
  assert.deepEqual(out.claim, claim); assert.deepEqual(out.start, start);
  assert.deepEqual(signed(claim), vector.claim); assert.deepEqual(signed(start), vector.start);
});
for (const [field, value] of Object.entries({claimHash: 'A'.repeat(43), operationId: 'another-operation', audience: 'https://other.test', serverId: 'srv_other', publicKey: 'A'.repeat(43), localGeneration: '8', address: 'https://another.test', purpose: 'sign-in', state: 'A'.repeat(43), codeChallenge: 'A'.repeat(43), installationId: 'another-installation-'.padEnd(43, '_'), serverChallenge: 'short', issuedAt: '2030-01-02T03:04:04Z', expiresAt: '2030-01-02T03:09:06.000000001Z'})) {
  test('a genuinely re-signed start cannot substitute captured ' + field, async () => {
    await assert.rejects(verifyOwnerClaimStart(vector.claim, signed({...start, [field]: value}), await expectation(), now));
  });
}
test('expiry, noncanonical UTC and original proof/hash are independently bounded', async () => {
  const expected = await expectation();
  await assert.rejects(verifyOwnerClaimStart(vector.claim, vector.start, expected, Date.parse(start.expiresAt)));
  await assert.rejects(verifyOwnerClaimStart(vector.claim, signed({...start, issuedAt: '2030-01-02T03:04:06.0Z'}), expected, now));
  await assert.rejects(verifyOwnerClaimStart({...vector.claim, payload: vector.claim.payload + '\n'}, vector.start, expected, now));
  await assert.rejects(verifyOwnerClaimStart(vector.claim, {...vector.start, signature: vector.start.signature + '='}, expected, now));
  await assert.rejects(verifyOwnerClaimStart(signed({...claim, name: 'Changed immutable original'}), vector.start, expected, now));
});
test('literal localhost callback remains separate from registered HTTPS and network HTTP is refused', async () => {
  for (const address of ['http://localhost:32500', 'http://127.0.0.1:32500', 'http://server.example.test', 'http://localhost.attacker.test']) {
    const original = signed({...claim, address});
    const changed = signed({...start, address, claimHash: routeBase64(await webRouteCrypto.sha256(routeBytes(original.payload)))});
    const expected = {...await expectation(), origin: address};
    if (address === 'http://localhost:32500') assert.equal((await verifyOwnerClaimStart(original, changed, expected, now)).start.address, address);
    else await assert.rejects(verifyOwnerClaimStart(original, changed, expected, now));
  }
});
test('canonical JSON refuses duplicate/order/unknown fields before signature verification', async () => {
  for (const raw of [vector.startPayload.replace('{', '{"kind":"portico.owner-start.v1",'), transportJSON({...start, unknown: true}), transportJSON(Object.fromEntries(Object.entries(start).reverse()))]) {
    let verified = 0;
    const crypto = {...webRouteCrypto, verify: async (...args: Parameters<typeof webRouteCrypto.verify>) => {verified++; return webRouteCrypto.verify(...args);}};
    await assert.rejects(verifyOwnerClaimStart(vector.claim, {payload: Buffer.from(raw).toString('base64url'), signature: sign(null, Buffer.from(raw), key).toString('base64url')}, await expectation(), now, crypto));
    assert.equal(verified, 0);
  }
});
