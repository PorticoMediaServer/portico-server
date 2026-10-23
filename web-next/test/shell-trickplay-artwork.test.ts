import test from 'node:test';
import assert from 'node:assert/strict';
import {parseTrickplay} from '../../packages/client-core/src/trickplay.ts';
import type {QueuePlaybackFence} from '../../packages/player-core/src/index.ts';
import {createTrickplayArtwork} from '../src/shell/trickplay-artwork.ts';

const fence: QueuePlaybackFence = {kind: 'queue', playback: {scope: {serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile', controllerId: 'v1', controllerEpoch: 'v1', commandLaneId: 'v1'}, run: 1}, sessionId: 'session', itemId: 'item', sourceId: 'source', entryId: 'entry', generation: 1};
const setId = 'a'.repeat(64), revision = 'b'.repeat(64);
const set = parseTrickplay({scope: {serverId: 'server', libraryId: 'library', itemId: 'item', viewerFence: 'viewer-fence', sessionId: '', sessionGeneration: 0}, sets: [{id: setId, revision, sourceId: 'source', sourceRevision: 'actual-source-revision', tileWidth: 160, tileHeight: 90, columns: 2, rows: 2, width: 320, height: 180, frameCount: 8, tileCount: 2, intervalMs: 1000, durationMs: 8000, sourceOffsetMs: 0, stale: false, tilesUrl: `/v1/items/item/trickplay/${setId}/tiles`, thumbnailsUrl: `/v1/items/item/trickplay/${setId}/thumbnails.vtt`}]}, 'server', {libraryId: 'library', itemId: 'item'}).sets[0];
const selection = {set, fence, viewerFence: 'viewer-fence', sourceRevision: 'actual-source-revision', isCurrent: () => true};
const auth = {token: async (value: string) => value, recover: async () => undefined};
const options = {scopeKey: 'current-authority', isCurrent: () => true, currentToken: () => 'private-test-token', auth, api: {baseUrl: 'https://server.invalid', routeFetch: async () => new Response(new Uint8Array([1, 2, 3]), {headers: {'Content-Type': 'image/jpeg'}})}};

test('parsed current Trickplay gives the exact frame crop and only a protected selected tile source', async () => {
  const original = globalThis.createImageBitmap;globalThis.createImageBitmap = async () => ({width: 320, height: 180, close() {}} as ImageBitmap);
  const requests: string[] = [];
  const source = createTrickplayArtwork({...options, api: {...options.api, routeFetch: async (url, init) => {requests.push(String(url));assert.equal(init?.credentials, 'omit');assert.equal(init?.redirect, 'error');return options.api.routeFetch();}}}, selection, () => () => {});
  try {
    const frame = source.frameAt(5.2)!;
    assert.deepEqual([frame.frame.tileIndex, frame.frame.x, frame.frame.y], [1, 160, 0]);
    assert.equal(frame.frame.tileUrl, `/v1/items/item/trickplay/${setId}/tiles/1.jpg`);
    assert.equal((await frame.source.scheduler.load(frame.source.key)).bytes, 3);
    assert.deepEqual(requests, ['https://server.invalid' + frame.frame.tileUrl]);
    assert.equal(source.frameAt(-1), null);assert.equal(source.frameAt(8), null);
  } finally {source.dispose();globalThis.createImageBitmap = original;}
});
test('another source revision, stale/local sheets or an arbitrary tile base are refused before image I/O', () => {
  for (const next of [{...selection, sourceRevision: 'different'}, {...selection, set: {...set, sourceId: 'other'}}, {...selection, set: {...set, stale: true}}, {...selection, set: {...set, tileUrls: ['file:///private']}}, {...selection, set: {...set, tilesUrl: '/v1/other'}}]) {
    assert.throws(() => createTrickplayArtwork(options, next, () => () => {}), /invalid_trickplay_artwork_selection/);
  }
});
test('decoded sheet geometry must match the parsed descriptor and mismatches never enter cache', async () => {
  const original = globalThis.createImageBitmap;globalThis.createImageBitmap = async () => ({width: 160, height: 90, close() {}} as ImageBitmap);
  const source = createTrickplayArtwork(options, selection, () => () => {});
  try {const frame = source.frameAt(1)!;await assert.rejects(frame.source.scheduler.load(frame.source.key), {status: 422});assert.equal(frame.source.scheduler.peek(frame.source.key), undefined);}
  finally {source.dispose();globalThis.createImageBitmap = original;}
});
test('source retirement aborts the image request and a late response cannot restore its private cache', async () => {
  let current = true, changed!: () => void, respond!: (response: Response) => void, signal: AbortSignal | undefined;
  const original = globalThis.createImageBitmap;globalThis.createImageBitmap = async () => ({width: 320, height: 180, close() {}} as ImageBitmap);
  const source = createTrickplayArtwork({...options, api: {...options.api, routeFetch: (_url, init) => {signal = init?.signal ?? undefined;return new Promise(resolve => {respond = resolve;});}}}, {...selection, isCurrent: () => current}, listener => {changed = listener;return () => {};});
  try {
    const frame = source.frameAt(1)!, pending = frame.source.scheduler.load(frame.source.key);await Promise.resolve();await Promise.resolve();
    current = false;changed();assert.equal(signal?.aborted, true);
    respond(new Response(new Uint8Array([1]), {headers: {'Content-Type': 'image/jpeg'}}));
    await assert.rejects(pending);assert.equal(frame.source.scheduler.peek(frame.source.key), undefined);assert.equal(source.frameAt(1), null);
  } finally {source.dispose();globalThis.createImageBitmap = original;}
});
test('identical sheets never share an image cache across playback generations or viewer fences', () => {
  const first = createTrickplayArtwork(options, selection, () => () => {});
  const generation = createTrickplayArtwork(options, {...selection, fence: {...fence, generation: 2}}, () => () => {});
  const viewer = createTrickplayArtwork(options, {...selection, viewerFence: 'changed-viewer'}, () => () => {});
  try {
    assert.notEqual(first.frameAt(1)!.source.key, generation.frameAt(1)!.source.key);
    assert.notEqual(first.frameAt(1)!.source.key, viewer.frameAt(1)!.source.key);
  } finally {first.dispose();generation.dispose();viewer.dispose();}
});
test('a synchronous retirement notification releases its subscription and offers no frame', () => {
  let released = 0;
  const source = createTrickplayArtwork(options, {...selection, isCurrent: () => false}, listener => {listener();return () => {released++;};});
  assert.equal(released, 1);assert.equal(source.frameAt(1), null);assert.equal(source.isCurrent(), false);
  source.dispose();assert.equal(released, 1);
});
test('an invalid image type or excessive declared tile size is rejected before decoding', async () => {
  for (const response of [() => new Response(new Uint8Array([1]), {headers: {'Content-Type': 'image/svg+xml'}}), () => new Response(new Uint8Array([1]), {headers: {'Content-Type': 'image/jpeg', 'Content-Length': String(12 * 1024 * 1024 + 1)}})]) {
    const source = createTrickplayArtwork({...options, api: {...options.api, routeFetch: async () => response()}}, selection, () => () => {});
    try {const frame = source.frameAt(1)!;await assert.rejects(frame.source.scheduler.load(frame.source.key));assert.equal(frame.source.scheduler.peek(frame.source.key), undefined);}
    finally {source.dispose();}
  }
});
