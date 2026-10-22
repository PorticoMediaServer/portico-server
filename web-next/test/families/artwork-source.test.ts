import test from 'node:test';
import assert from 'node:assert/strict';
import {ArtworkScheduler} from '../../../packages/client-core/src/artwork/index.ts';
import {ambientArtworkSource} from '../../../packages/client-core/src/artwork/ambient.ts';
import {routePath} from '../../../packages/contracts/src/routes.ts';
import {createArtworkRuntime, observeArtwork, scheduledAmbientSource, type ArtworkRead} from '../../src/kit/artwork/source.ts';
import type {ArtworkImage} from '../../src/kit/artwork/types.ts';

const flush = async () => { for (let index = 0; index < 12; index++) await Promise.resolve(); };
test('a removed queued image withdraws interest before its authenticated loader starts', async () => {
  let loads = 0;
  const runtime = createArtworkRuntime({scopeKey: 'server/account/profile/authority', load: async () => { loads++; return {src: 'blob:real-loader-output'}; }, classify: () => ({retryable: false})});
  const states: ArtworkRead[] = [];
  const stop = observeArtwork(runtime.source({url: '/selected-art', cacheKey: 'digest'}), value => states.push(value));
  stop(); await flush();
  assert.equal(loads, 0); assert.deepEqual(states, [{status: 'loading'}]);
  assert.equal(runtime.scheduler.stats().queued, 0); runtime.dispose();
});

test('scoped source identity deduplicates consumers while preserving real loader inputs', async () => {
  const requests: unknown[] = [];
  const runtime = createArtworkRuntime({scopeKey: 'server/account/profile/authority', load: async input => { requests.push(input); return {src: 'blob:decoded'}; }, classify: () => ({retryable: false})});
  const source = runtime.source({url: '/selected-art?v=digest', cacheKey: 'versioned-art'});
  const a: ArtworkRead[] = [], b: ArtworkRead[] = [];
  const stopA = observeArtwork(source, value => a.push(value)), stopB = observeArtwork(source, value => b.push(value));
  await flush();
  assert.deepEqual(requests, [{url: '/selected-art?v=digest', cacheKey: 'versioned-art', kind: 'foreground'}]);
  assert.equal(a.at(-1)?.status, 'ready'); assert.equal(b.at(-1)?.image?.src, 'blob:decoded');
  stopA(); stopB(); runtime.dispose(); assert.equal(runtime.scheduler.peek(source.key), undefined);
  assert.throws(() => runtime.source({url: '/selected-art', cacheKey: 'digest'}));
});

test('real Retry-After controls a retry without occupying a scheduler slot', async () => {
  let attempts = 0, serial = 0;
  const timers = new Map<number, {run: () => void; ms: number}>();
  const runtime = createArtworkRuntime({scopeKey: 'authority', load: async () => { if (++attempts === 1) throw new Error('busy'); return {src: 'blob:ready'}; }, classify: () => ({retryable: true, retryAfterSeconds: 2}),
    policy: {backoffMs: [10], setTimer: (run, ms) => { const id = ++serial; timers.set(id, {run, ms}); return id; }, clearTimer: timer => { timers.delete(timer as number); }}});
  const states: ArtworkRead[] = [], stop = observeArtwork(runtime.source({url: '/selected-art', cacheKey: 'digest'}), value => states.push(value));
  await flush(); assert.equal(attempts, 1); assert.equal(runtime.scheduler.stats().active, 0);
  const retry = [...timers].find(([, timer]) => timer.ms === 2000); assert.ok(retry); timers.delete(retry[0]); retry[1].run();
  await flush(); assert.equal(attempts, 2); assert.equal(states.at(-1)?.status, 'ready');
  stop(); runtime.dispose();
});

test('a final loader failure exposes a fallback state without exposing transport error text', async () => {
  const runtime = createArtworkRuntime({scopeKey: 'authority', load: async () => { throw new Error('private transport facts'); }, classify: () => ({retryable: false})});
  const states: ArtworkRead[] = [], stop = observeArtwork(runtime.source({url: '/selected-art', cacheKey: 'digest'}), value => states.push(value));
  await flush(); assert.deepEqual(states.at(-1), {status: 'error'}); assert.equal(JSON.stringify(states).includes('private'), false);
  stop(); runtime.dispose();
});

test('mounted resources remain pinned across the byte budget then become evictable', async () => {
  const released: string[] = [];
  const runtime = createArtworkRuntime({scopeKey: 'authority', load: async input => ({src: input.url, bytes: 8}), classify: () => ({retryable: false}), release: image => released.push(image.src), policy: {maxBytes: 1}});
  const source = runtime.source({url: '/visible-art', cacheKey: 'visible'}), stop = observeArtwork(source, () => {});
  await flush(); assert.equal(runtime.scheduler.peek(source.key)?.src, '/visible-art'); assert.deepEqual(released, []);
  stop(); await runtime.scheduler.load(runtime.source({url: '/next-art', cacheKey: 'next'}).key);
  assert.ok(released.includes('/visible-art')); runtime.dispose();
});

test('disposing an authority releases an in-flight result and never delivers it to the old view', async () => {
  let finish: ((image: ArtworkImage) => void) | undefined;
  const released: string[] = [];
  const runtime = createArtworkRuntime({scopeKey: 'old-authority', load: () => new Promise(resolve => { finish = resolve; }), classify: () => ({retryable: false}), release: image => released.push(image.src)});
  const states: ArtworkRead[] = [], stop = observeArtwork(runtime.source({url: '/selected-art', cacheKey: 'digest'}), value => states.push(value));
  await flush(); stop(); runtime.dispose(); finish!({src: 'blob:old-authority'}); await flush();
  assert.deepEqual(released, ['blob:old-authority']); assert.equal(states.some(value => value.image), false);
});

test('accepted ambient helper provenance and foreground identity remain separate', async () => {
  const url = routePath('get_items_id_art_kind', {id: 'selected', kind: 'poster'}) + '?v=' + 'a'.repeat(64) + '&w=800';
  const ambient = ambientArtworkSource(url, {serverId: 'server', viewerId: 'profile', authorityKey: 'authority'}); assert.ok(ambient);
  const scheduler = new ArtworkScheduler<ArtworkImage>({load: async () => ({src: 'blob:ambient'})});
  const scheduled = scheduledAmbientSource(scheduler, ambient);
  assert.equal(scheduled.key, ambient.cacheKey); assert.equal(scheduled.maxEdge, 64); assert.equal(scheduled.variant, 'ambient-v1');
  const requests: unknown[] = [], runtime = createArtworkRuntime({scopeKey: 'authority', load: async input => { requests.push(input); return {src: 'blob:result'}; }, classify: () => ({retryable: false})});
  const foreground = runtime.source({url, cacheKey: 'sharp'}), wash = runtime.ambient(ambient);
  await runtime.scheduler.load(foreground.key); await runtime.scheduler.load(wash.key);
  assert.deepEqual(requests, [{url, cacheKey: 'sharp', kind: 'foreground'}, {url: ambient.url, cacheKey: ambient.cacheKey, kind: 'ambient'}]);
  assert.notEqual(foreground.key, wash.key); runtime.dispose(); scheduler.dispose();
});
