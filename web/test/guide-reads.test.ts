import test from 'node:test';
import assert from 'node:assert/strict';
import {dedupedGuideApi, forgetGuideReads} from '../src/app/guide-reads.ts';

function counting() {
  const calls: string[] = [];
  return {calls, api: {request: async <T,>(path: string) => { calls.push(path); return {path} as T; }}};
}

test('identical guide reads within the window share one request', async () => {
  let t = 0;
  const {calls, api} = counting();
  const guide = dedupedGuideApi(api, () => t);
  await Promise.all([guide.request('/v1/guide?a=1'), guide.request('/v1/guide?a=1'), guide.request('/v1/guide?a=2')]);
  assert.deepEqual(calls, ['/v1/guide?a=1', '/v1/guide?a=2']);
  t = 5_000; await guide.request('/v1/guide?a=1');
  assert.equal(calls.length, 2);
  t = 20_000; await guide.request('/v1/guide?a=1');
  assert.equal(calls.length, 3);
});

test('writes, other paths and a forgotten cache always go to the server', async () => {
  const {calls, api} = counting();
  const guide = dedupedGuideApi(api, () => 0);
  await guide.request('/v1/guide?a=1'); await guide.request('/v1/items/x'); await guide.request('/v1/items/x');
  await guide.request('/v1/guide/x', 'POST', {});
  forgetGuideReads();
  await guide.request('/v1/guide?a=1');
  assert.deepEqual(calls, ['/v1/guide?a=1', '/v1/items/x', '/v1/items/x', '/v1/guide/x', '/v1/guide?a=1']);
});

test('one caller aborting does not cancel the shared read for the others', async () => {
  const {api} = counting();
  const guide = dedupedGuideApi(api, () => 0);
  forgetGuideReads();
  const controller = new AbortController();
  const first = guide.request('/v1/guide?b=1', 'GET', undefined, controller.signal);
  const second = guide.request('/v1/guide?b=1');
  controller.abort(new Error('left'));
  await assert.rejects(first);
  assert.deepEqual(await second, {path: '/v1/guide?b=1'});
});
