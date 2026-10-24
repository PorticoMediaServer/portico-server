import test from 'node:test';
import assert from 'node:assert/strict';
import {decode} from '../src/hosted/runtime.ts';
import {decodeServerClaimLookupResponse, decodeServerClaimPendingResponse} from '../src/hosted/decoders.ts';

const pending = {purpose: 'switch', code: 'claim_fixture', serverId: 'server_fixture', name: 'Server', address: 'https://server.test',
  requestedAt: '2030-01-02T03:04:05Z', expiresAt: '2030-01-02T03:05:05Z', status: 'pending', ownerTransactionId: 'transaction_fixture'};

test('generated lookup preserves absent and exact bounded counts including zero', () => {
  assert.equal('otherPeopleCount' in decodeServerClaimLookupResponse(pending), false);
  for (const count of [0, 1, 10000]) assert.equal(decodeServerClaimLookupResponse({...pending, otherPeopleCount: count}).otherPeopleCount, count);
  for (const count of [null, -1, 10001, 1.5, '0', true]) assert.throws(() => decodeServerClaimLookupResponse({...pending, otherPeopleCount: count}));
});

test('pending registration response never publishes a count', () => {
  assert.equal('otherPeopleCount' in decodeServerClaimPendingResponse(pending), false);
  // Existing response projection discards unknown presentation fields. It
  // must not expose a count even if one appears on an untyped wire object.
  assert.equal('otherPeopleCount' in decodeServerClaimPendingResponse({...pending, otherPeopleCount: 0}), false);
});

test('generated pending count is a full nonnull envelope only on owner starts', () => {
  const proof = {payload: 'fixture', signature: 'fixture'};
  for (const body of [proof, {...proof, ownerStart: proof}, {...proof, ownerStart: proof, ownerSwitchCount: proof}]) assert.doesNotThrow(() => decode('ServerClaimPendingRequest', body));
  for (const body of [{...proof, ownerSwitchCount: proof}, {...proof, ownerStart: proof, ownerSwitchCount: null},
    {...proof, ownerStart: proof, ownerSwitchCount: {}}, {...proof, ownerStart: proof, ownerSwitchCount: {...proof, unknown: true}},
    {...proof, ownerStart: proof, ownerSwitchCount: {payload: 'fixture'}}]) assert.throws(() => decode('ServerClaimPendingRequest', body));
});
