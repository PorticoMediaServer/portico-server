import test from 'node:test';
import assert from 'node:assert/strict';
import {ApiError} from '@core/index.ts';
import {SessionRefreshError} from '@core/server-connections.ts';
import {restoreOutcome, serverReplaced} from '../src/bridge/restore-policy.ts';

const saved = {serverUrl: 'https://server.example/', serverId: 'srv_saved'};
const system = (id: string): typeof fetch => async () => new Response(JSON.stringify({id, name: 'Home', setupRequired: false}), {status: 200, headers: {'Content-Type': 'application/json'}});
const unreachable: typeof fetch = async () => { throw new TypeError('fetch failed'); };

test('a refused renewal (401) is a reset', async () => {
  assert.equal(await restoreOutcome(new SessionRefreshError('refresh_refused', 'Your sign-in has ended.', true), saved, system('srv_saved')), 'reset');
});

test('access_refused (the server refuses this sign-in after a renewal) has ended it', async () => {
  const refused = Object.assign(new Error('This profile no longer has access to this server.'), {name: 'RouteError', code: 'access_refused'});
  assert.equal(await restoreOutcome(refused, saved, system('srv_saved')), 'reset');
  const offline = Object.assign(new Error('No route.'), {name: 'RouteError', code: 'route_unavailable'});
  assert.equal(await restoreOutcome(offline, saved, unreachable), 'retry', 'an unreachable route is not an ended sign-in');
});

test('a 400 keeps the saved sign-in (retry), as does a 404', async () => {
  assert.equal(await restoreOutcome(new ApiError(400, 'invalid_request', 'Bad request'), saved, system('srv_saved')), 'retry');
  assert.equal(await restoreOutcome(new ApiError(404, 'not_found', 'Not found'), saved, system('srv_saved')), 'retry');
  // A renewal that failed without being refused is not a reset either.
  assert.equal(await restoreOutcome(new SessionRefreshError('refresh_failed', 'Try again.', false), saved, system('srv_saved')), 'retry');
});

test('a different server id at the saved address is a reset; the same id is not', async () => {
  assert.equal(await restoreOutcome(new ApiError(404, 'not_found', 'Not found'), saved, system('srv_new')), 'reset');
  assert.equal(await restoreOutcome(new Error('The saved connection no longer matches this tab’s profile.'), saved, system('srv_saved')), 'retry');
  assert.equal(await serverReplaced(saved.serverUrl, saved.serverId, system('srv_new')), true);
});

test('a network error is retried, and an unreachable server is never judged replaced', async () => {
  assert.equal(await restoreOutcome(new TypeError('fetch failed'), saved, unreachable), 'retry');
  assert.equal(await serverReplaced(saved.serverUrl, saved.serverId, unreachable), false);
  assert.equal(await serverReplaced(saved.serverUrl, saved.serverId, async () => new Response('down', {status: 503})), false);
});
