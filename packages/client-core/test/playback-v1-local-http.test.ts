import test from 'node:test';
import assert from 'node:assert/strict';
import {HttpLocalApi} from '../src/index.ts';
import {PlaybackApiError, SessionsClient, localV1Http} from '../src/playback-v1/index.ts';
import {FakePlaybackServer} from '../src/playback-v1/testing/fake-server.ts';

/** A fetch that serves a FakePlaybackServer, recording what HttpLocalApi actually sent. */
function served(server: FakePlaybackServer, options: {expireFirst?: boolean} = {}) {
  const sent: {path: string; method: string; headers: Record<string, string>}[] = [];
  let expired = options.expireFirst ?? false;
  const fetcher: typeof fetch = async (input, init = {}) => {
    const url = new URL(String(input));
    const headers: Record<string, string> = {};
    new Headers(init.headers).forEach((v, k) => { headers[k] = v; });
    sent.push({path: url.pathname + url.search, method: String(init.method ?? 'GET'), headers});
    if (expired && headers.authorization === 'Bearer old') { expired = false; return new Response(JSON.stringify({error: {code: 'unauthorized'}}), {status: 401}); }
    const r = await server.handle({method: String(init.method ?? 'GET'), path: url.pathname + url.search, headers: Object.fromEntries(Object.entries(headers).map(([k, v]) => [k.replace(/(^|-)\w/g, m => m.toUpperCase()), v])), body: init.body ? JSON.parse(String(init.body)) : undefined});
    return new Response(r.body === undefined ? null : JSON.stringify(r.body), {status: r.status, headers: {...r.headers, ...(r.body === undefined ? {} : {'Content-Type': 'application/json'})}});
  };
  return {fetcher, sent};
}

test('localV1Http: statuses, headers and bodies pass through HttpLocalApi unchanged', async () => {
  const server = new FakePlaybackServer();
  const {fetcher, sent} = served(server);
  const api = new HttpLocalApi('https://portico.test', 'token', fetcher);
  const http = localV1Http(api);
  const client = new SessionsClient(http);
  const s = await client.start({itemId: 'movie-1'}, {}, 'key_local_http_0001');
  assert.equal(sent[0]!.headers['idempotency-key'], 'key_local_http_0001');
  assert.equal(sent[0]!.headers.authorization, 'Bearer token');
  // A timeline answer is 204 with Report-Every-Ms.
  const every = await client.timeline(s.id, {seq: 1, generation: 1, state: 'playing', positionMs: 1000, rate: 1});
  assert.equal(every.reportEveryMs, 10_000);
  // A stale If-Match is a 412 whose `current` survives the transport.
  await client.patch(s.id, s.revision, {state: 'paused'});
  await assert.rejects(client.patch(s.id, s.revision, {state: 'playing'}), (e: unknown) => e instanceof PlaybackApiError && e.status === 412 && (e.current as {revision?: string} | undefined)?.revision === '2');
  const patch = sent.find(r => r.method === 'PATCH')!;
  assert.equal(patch.headers['content-type'], 'application/merge-patch+json');
  assert.equal(patch.headers['if-match'], s.revision);
  // A request cannot replace the bearer.
  await http.send({method: 'GET', path: `/v1/playback/sessions/${s.id}`, headers: {Authorization: 'Bearer forged'}});
  assert.equal(sent.at(-1)!.headers.authorization, 'Bearer token');
});

test('localV1Http: a 401 renews the token once and resends', async () => {
  const server = new FakePlaybackServer();
  const {fetcher, sent} = served(server, {expireFirst: true});
  const api = new HttpLocalApi('https://portico.test', 'old', fetcher);
  let recovered = 0;
  api.setAuthRecovery({token: async t => t, recover: async () => { recovered++; api.setAccessToken('new'); return 'new'; }});
  const s = await new SessionsClient(localV1Http(api)).start({itemId: 'movie-1'}, {}, 'key_local_http_0002');
  assert.ok(s.id);
  assert.equal(recovered, 1);
  assert.deepEqual(sent.map(r => r.headers.authorization), ['Bearer old', 'Bearer new']);
  assert.equal(sent[1]!.headers['idempotency-key'], 'key_local_http_0002', 'the same key on the resend');
});
