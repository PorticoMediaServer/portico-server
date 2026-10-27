import test from 'node:test';
import assert from 'node:assert/strict';
import {PlayerChaptersService} from '@core/player-chapters.ts';
import {LyricsService} from '@core/lyrics.ts';
import type {QueuePlaylistSave} from '@core/playback-v1/queue.ts';
import type {V1PlaybackTarget, V1QueuePolicyIntent} from '@core/playback-v1/v1-queue-player.ts';
import type {PlaybackFence} from '../../packages/player-core/src/index.ts';
import {createWebListeningPort, type WebListeningInput} from '../src/platform/playback/listening-port.ts';
import type {WebQueueListPort, WebQueueListSnapshot} from '../src/platform/playback/queue-list.ts';

const fence = {kind: 'queue', sessionId: 'session', generation: 2, itemId: 'item', entryId: 'entry', sourceId: 'source'} as unknown as PlaybackFence;
const target = {libraryId: 'library', itemId: 'item', sessionId: 'session', sessionGeneration: 2, sourceId: 'source',
  versionId: 'version', playbackRun: 4, queueId: 'queue', entryId: 'entry'};
const scope = {serverId: 'server', viewerId: 'local-owner-profile', libraryId: 'library'};
const chaptersTarget = {libraryId: 'library', itemId: 'item', sessionId: 'session', sessionGeneration: 2, sourceId: 'source'};

function chaptersData() {
  return {scope: {serverId: 'server', libraryId: 'library', itemId: 'item', sessionId: 'session', generation: 2, sourceId: 'source', viewerFence: 'fence'},
    revision: 'revision', status: 'available', reason: null, durationMs: 60000,
    chapters: [{id: 'source:1', index: 1, title: 'One', startMs: 0, endMs: 30000, action: {id: 'seek', positionMs: 0}},
      {id: 'source:2', index: 2, title: 'Two', startMs: 30000, endMs: 60000, action: {id: 'seek', positionMs: 30000}}],
    totalCount: 2, nextCursor: ''};
}

function deferred<T>() {let resolve!: (value: T) => void; const promise = new Promise<T>(r => {resolve = r;}); return {promise, resolve};}

function chaptersService(io?: {promise: Promise<unknown>}) {
  const payload = chaptersData();
  return new PlayerChaptersService({scope, pageSize: 100, timeoutMs: 1000,
    api: {request: async <T>() => (io ? io.promise as T : payload as T)}});
}

function lyricsService() {
  return new LyricsService({request: async <T>() => ({} as T)}, scope, {libraryId: 'library', itemId: 'item', sessionId: 'session', sessionGeneration: 2});
}

/** The published queue port hands out one stable snapshot object, as the real port does. */
function stableQueue() {
  const snapshot: WebQueueListSnapshot = Object.freeze({current: true, queueId: 'queue', revision: 'queue-revision', currentEntryId: 'entry', entries: Object.freeze([]), more: null, change: 'idle'});
  const calls: string[] = [];
  const port = {
    getSnapshot: () => snapshot, subscribe: () => () => {}, isCurrent: () => true,
    play: async () => {calls.push('play');}, move: async () => {calls.push('move');}, remove: async () => {calls.push('remove');},
  } as WebQueueListPort;
  return {port, snapshot, calls};
}

function harness(options: Readonly<{lyricsEnabled?: boolean; queue?: boolean; io?: {promise: Promise<unknown>}}> = {}) {
  const seeks: number[] = [];
  const policies: {target: V1PlaybackTarget; revision: string; intent: V1QueuePolicyIntent}[] = [];
  const saves: {queueId: string; name: string; signal: AbortSignal}[] = [];
  let current = true, lifetimeCurrent = true, reentered = false, port: ReturnType<typeof createWebListeningPort> | undefined;
  const lifetimeController = new AbortController();
  const lifetime = {signal: lifetimeController.signal, isCurrent: () => lifetimeCurrent};
  const queue = options.queue === false ? undefined : stableQueue();
  const input: WebListeningInput = {
    fence, lifetime, target,
    /** A predicate that may synchronously retire the record and still answer true. */
    current: () => {if (reentered && port) {port.dispose(); return true;} return current;},
    chapters: chaptersService(options.io), chaptersTarget,
    lyrics: lyricsService(), lyricsEnabled: () => options.lyricsEnabled ?? true, ...(queue ? {queue: queue.port} : {}),
    policy: async (expected, revision, intent, caller) => {
      if (!caller.isCurrent() || caller.signal.aborted) throw new Error('stale_listening_port');
      policies.push({target: expected, revision, intent});
    },
    save: async (queueId, name, caller) => {
      if (!caller.isCurrent() || caller.signal.aborted) throw new Error('stale_listening_port');
      saves.push({queueId, name, signal: caller.signal});
      const receipt: QueuePlaylistSave = {playlistId: 'playlist', revision: '1', entries: 3, state: 'saved'};
      return receipt;
    },
    seek: async (seconds, caller) => {
      if (!caller.isCurrent() || caller.signal.aborted) throw new Error('stale_listening_port');
      seeks.push(seconds);
    },
  };
  port = createWebListeningPort(input);
  return {port, lifetime, lifetimeController, queue, seeks, policies, saves,
    retire: () => {current = false;}, retireLifetime: () => {lifetimeCurrent = false;},
    armReentry: () => {reentered = true;}};
}

test('the captured record loads and seeks only its own original target, source and drawn row', async () => {
  const h = harness();
  await h.port.chapters.load();
  const snapshot = h.port.chapters.getSnapshot();
  assert.equal(snapshot.target?.sessionId, 'session');
  assert.equal(snapshot.target?.sourceId, 'source');
  await h.port.chapters.seek(snapshot, snapshot.data!.chapters[1], h.lifetime);
  assert.deepEqual(h.seeks, [30]);
  await assert.rejects(() => h.port.chapters.seek(snapshot, {...snapshot.data!.chapters[1], action: {id: 'seek', positionSeconds: 45}}, h.lifetime), /listening_command_unavailable/);
  await assert.rejects(() => h.port.chapters.seek({...snapshot}, snapshot.data!.chapters[1], h.lifetime), /listening_command_unavailable/);
});

test('a late response or publication after retirement never reaches the consumer', async () => {
  const io = deferred<unknown>();
  const h = harness({io});
  let published = 0;
  h.port.chapters.subscribe(() => {published++;});
  const loading = h.port.chapters.load();
  h.port.dispose();
  io.resolve(chaptersData());
  await loading.catch(() => {});
  assert.equal(published, 0);
  assert.equal(h.port.isCurrent(), false);
});

test('source or playback-run drift during awaited IO refuses the command', async () => {
  const io = deferred<unknown>();
  const h = harness({io});
  const loading = h.port.chapters.load();
  h.retire();
  io.resolve(chaptersData());
  await assert.rejects(() => loading, /stale_listening_port/);
});

test('an external predicate that synchronously disposes is re-proved before dispatch', async () => {
  const h = harness();
  const shown = h.port.queue!.list.getSnapshot();
  h.armReentry();
  await assert.rejects(() => h.port.queue!.policy(shown, {action: 'repeat', repeat: 'all'}, h.lifetime), /stale_listening_port/);
  assert.equal(h.policies.length, 0);
  assert.equal(h.port.isCurrent(), false);
});

test('queue policy and save carry the rendered revision and the original queue key', async () => {
  const h = harness();
  const shown = h.port.queue!.list.getSnapshot();
  await h.port.queue!.policy(shown, {action: 'repeat', repeat: 'all'}, h.lifetime);
  assert.equal(h.policies.length, 1);
  assert.equal(h.policies[0].revision, 'queue-revision');
  assert.deepEqual([h.policies[0].target.queueId, h.policies[0].target.versionId, h.policies[0].target.playbackRun], ['queue', 'version', 4]);
  const receipt = await h.port.queue!.save(shown, '  Night drive  ', h.lifetime);
  assert.equal(receipt.state, 'saved');
  assert.deepEqual(h.saves.map(s => [s.queueId, s.name]), [['queue', 'Night drive']]);
});

test('the bound queue list refuses after dispose and cannot command from an old snapshot', async () => {
  const h = harness();
  const shown = h.port.queue!.list.getSnapshot();
  assert.equal(h.port.queue!.list.getSnapshot(), shown);
  h.port.dispose();
  assert.equal(h.port.queue!.list.getSnapshot().current, false);
  await assert.rejects(() => h.port.queue!.list.play(shown, 'entry', h.lifetime), /stale_listening_port/);
  assert.deepEqual(h.queue!.calls, []);
  assert.equal(h.port.queue!.list.isCurrent(), false);
});

test('caller cancellation stops a command and suppresses later save progress', async () => {
  const h = harness();
  const shown = h.port.queue!.list.getSnapshot();
  h.lifetimeController.abort();
  await assert.rejects(() => h.port.queue!.save(shown, 'Night drive', h.lifetime, () => {throw new Error('progress published after cancel');}), /stale_listening_port/);
  assert.equal(h.saves.length, 0);
});

test('lyrics stays withdrawn when the canonical preference is not true', async () => {
  const h = harness({lyricsEnabled: false});
  await assert.rejects(() => h.port.lyrics.refresh(), /listening_command_unavailable/);
  await assert.rejects(() => h.port.lyrics.choose(h.port.lyrics.getSnapshot(), null, h.lifetime), /listening_command_unavailable/);
  await assert.rejects(() => h.port.lyrics.seek(h.port.lyrics.getSnapshot(), 0, h.lifetime), /listening_command_unavailable/);
});

test('a record without an admitted queue exposes no queue facade at all', () => {
  assert.equal(harness({queue: false}).port.queue, undefined);
});

test('a retired record withholds source facts, targets and callbacks, not just future commands', async () => {
  const h = harness();
  await h.port.chapters.load();
  let published = 0;
  h.port.chapters.subscribe(() => {published++;});
  assert.equal(h.port.chapters.getSnapshot().phase, 'ready');
  h.port.dispose();
  const retired = h.port.chapters.getSnapshot();
  assert.equal(retired.target, null);
  assert.equal(retired.data, null);
  assert.equal(retired.phase, 'idle');
  assert.equal(h.port.lyrics.getSnapshot().data, null);
  assert.equal(published, 0);
});

test('a chapter response from a different source is not adopted as a new source', async () => {
  const payload = {...chaptersData(), scope: {...chaptersData().scope, sourceId: 'other'}};
  const service = new PlayerChaptersService({scope, pageSize: 100, timeoutMs: 1000, api: {request: async <T>() => payload as T}});
  const receipt: QueuePlaylistSave = {playlistId: 'playlist', revision: '1', entries: 0, state: 'saved'};
  const input: WebListeningInput = {fence, lifetime: {signal: new AbortController().signal, isCurrent: () => true}, target, current: () => true,
    chapters: service, chaptersTarget: {libraryId: 'library', itemId: 'item', sessionId: 'session', sessionGeneration: 2, sourceId: 'source'},
    lyrics: lyricsService(), lyricsEnabled: () => true,
    policy: async () => {}, save: async () => receipt, seek: async () => {}};
  const port = createWebListeningPort(input);
  await port.chapters.load();
  assert.equal(port.chapters.getSnapshot().data, null);
  assert.notEqual(port.chapters.getSnapshot().phase, 'ready');
  port.dispose();
});
