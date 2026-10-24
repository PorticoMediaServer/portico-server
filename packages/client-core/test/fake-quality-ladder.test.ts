import test from 'node:test';
import assert from 'node:assert/strict';
import {SessionsClient} from '../src/playback-v1/sessions.ts';
import {PlaybackOptionsClient} from '../src/playback-v1/options.ts';
import {qualityLadder} from '../src/playback-v1/quality.ts';
import {FakePlaybackServer, fakeHttp, type FakeRequest} from '../src/playback-v1/testing/fake-server.ts';
import {routePath} from '../../contracts/src/routes.ts';

const canonicalIds = ['2160p', '1080p-high', '1080p-low', '720p-high', '720p-low', '480p', '240p'];

function fixture() {
  const server = new FakePlaybackServer();
  const requests: FakeRequest[] = [];
  const http = fakeHttp(server);
  const sessions = new SessionsClient({send(request) { requests.push(request); return http.send(request); }});
  return {server, requests, sessions};
}

test('fake automatic quality retains every actually offered canonical id and the exact request', async () => {
  assert.deepEqual(qualityLadder.map(rung => rung.id), canonicalIds);
  const {server, requests, sessions} = fixture();
  const offered = await new PlaybackOptionsClient(fakeHttp(server)).options('movie-1');
  assert.deepEqual(offered.qualities.filter(option => option.mode === 'fixed').map(option => option.id), canonicalIds);
  let session = await sessions.start({itemId: 'movie-1'}, {quality: {mode: 'automatic'}}, 'canonical-quality-request');
  const originalId = session.id;
  for (const rung of qualityLadder) {
    const quality = {mode: 'automatic' as const, id: rung.id};
    session = await sessions.patch(session.id, session.revision, {quality});
    assert.deepEqual(requests.at(-1)?.body, {quality}, 'the producer receives the exact canonical choice');
    assert.equal(session.id, originalId, 'quality changes replan the same session');
    assert.deepEqual(session.presentation.quality, quality);
    assert.deepEqual(server.session(session.id)?.quality, quality, 'stored producer state retains the requested id');
  }
});

test('fake automatic down and up follow the complete ladder and clamp at both ends', async () => {
  const {sessions} = fixture();
  let session = await sessions.start({itemId: 'movie-1'}, {quality: {mode: 'automatic'}}, 'canonical-quality-steps');
  const originalId = session.id;
  for (const id of canonicalIds) {
    session = await sessions.patch(session.id, session.revision, {quality: {mode: 'automatic', step: 'down'}});
    assert.deepEqual(session.presentation.quality, {mode: 'automatic', id});
    assert.equal(session.id, originalId);
  }
  session = await sessions.patch(session.id, session.revision, {quality: {mode: 'automatic', step: 'down'}});
  assert.deepEqual(session.presentation.quality, {mode: 'automatic', id: '240p'}, 'down never crosses the lowest offered ceiling');
  for (const id of [...canonicalIds.slice(0, -1).reverse(), undefined]) {
    session = await sessions.patch(session.id, session.revision, {quality: {mode: 'automatic', step: 'up'}});
    assert.deepEqual(session.presentation.quality, {mode: 'automatic', ...(id ? {id} : {})});
  }
  session = await sessions.patch(session.id, session.revision, {quality: {mode: 'automatic', step: 'up'}});
  assert.deepEqual(session.presentation.quality, {mode: 'automatic'}, 'up never crosses Original');
});

test('fake automatic Original and unknown aliases preserve the existing original fallback', async () => {
  const {server, sessions} = fixture();
  let session = await sessions.start({itemId: 'movie-1'}, {quality: {mode: 'automatic'}}, 'canonical-quality-original');
  session = await sessions.patch(session.id, session.revision, {quality: {mode: 'automatic', id: '720p-low'}});
  session = await sessions.patch(session.id, session.revision, {quality: {mode: 'automatic', id: 'original'}});
  assert.deepEqual(session.presentation.quality, {mode: 'automatic'});
  const path = routePath('change_playback_session', {id: session.id});
  for (const id of ['720p', '1080p', 'not-a-quality']) {
    const current = await server.handle({method: 'GET', path});
    const result = await server.handle({method: 'PATCH', path, headers: {'If-Match': current.headers.ETag!}, body: {quality: {mode: 'automatic', id}}});
    assert.equal(result.status, 200);
    assert.deepEqual(server.session(session.id)?.quality, {mode: 'automatic'}, 'unknown ids are never invented or accepted as aliases');
  }
});
