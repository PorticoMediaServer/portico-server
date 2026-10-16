import test from 'node:test';
import assert from 'node:assert/strict';
import type {HttpLocalApi} from '../src/index.ts';
import {startAppSetup} from '../src/setup-onboarding.ts';
import {safeApprovalUrl} from '../src/claim-web.ts';

function api(answer: unknown, serverId = 'srv_1') {
  const calls: {path: string; method: string}[] = [];
  const fake = {
    getRouteConnection: () => ({pin: {serverId, publicKey: 'pk', fingerprint: 'fp'}}),
    request: async (path: string, method: string) => { calls.push({path, method}); return answer; },
  } as unknown as HttpLocalApi;
  return {fake, calls};
}

test('first run from an app asks the app route for the setup secret of the pinned server', async () => {
  const {fake, calls} = api({serverId: 'srv_1', setupToken: 't'.repeat(43)});
  assert.deepEqual(await startAppSetup(fake), {serverId: 'srv_1', setupToken: 't'.repeat(43)});
  assert.deepEqual(calls, [{path: '/v1/setup/app', method: 'POST'}]);
});

test('a setup answer for another server, or a malformed one, is refused', async () => {
  await assert.rejects(startAppSetup(api({serverId: 'srv_2', setupToken: 'x'}).fake));
  await assert.rejects(startAppSetup(api({serverId: 'srv_1'}).fake));
  await assert.rejects(startAppSetup(api(null).fake));
});

test('the claim approval page opened from an app is only ever the Portico Account site', () => {
  const hosted = 'https://web.getportico.tv';
  assert.equal(safeApprovalUrl(`${hosted}/claim#code=ABC&server=Den`, hosted), `${hosted}/claim#code=ABC&server=Den`);
  assert.equal(safeApprovalUrl('https://evil.example/claim#code=ABC', hosted), undefined);
  assert.equal(safeApprovalUrl(`${hosted}/other#code=ABC`, hosted), undefined);
});
