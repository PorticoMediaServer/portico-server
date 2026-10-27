import test from 'node:test';
import assert from 'node:assert/strict';
import {PlayerChaptersService} from '@core/player-chapters.ts';
import {LyricsService} from '@core/lyrics.ts';
import type {QueuePlaylistSave} from '@core/playback-v1/queue.ts';
import type {V1PlaybackTarget, V1QueuePolicyIntent} from '@core/playback-v1/v1-queue-player.ts';
import type {PlaybackFence} from '../../packages/player-core/src/index.ts';
import {createWebListeningPort, type WebListeningInput} from '../src/platform/playback/listening-port.ts';
import type {WebQueueListPort, WebQueueListSnapshot} from '../src/platform/playback/queue-list.ts';

const fence = {kind: 'queue', sessionId: 'session', generation: 2, itemId: 'item', entryId: 'entry', queueId: 'queue'} as unknown as PlaybackFence;
const target = {libraryId: 'library', itemId: 'item', sessionId: 'session', sessionGeneration: 2, versionId: 'version', playbackRun: 4, queueId: 'queue', entryId: 'entry'};
const scope = {serverId: 'server', viewerId: 'local-owner-profile', libraryId: 'library'};
const chaptersTarget = {libraryId: 'library', itemId: 'item', sessionId: 'session', sessionGeneration: 2};

function chaptersData() {
  return {scope: {serverId: 'server', libraryId: 'library', itemId: 'item', sessionId: 'session', generation: 2, sourceId: 'source', viewerFence: 'fence'},
    revision: 'revision', status: 'available', reason: null, durationMs: 60000,
    chapters: [{id: 'source:1', index: 1, title: 'One', startMs: 0, endMs: 30000, action: {id: 'seek', positionMs: 0}},
      {id: 'source:2', index: 2, title: 'Two', startMs: 30000, endMs: 60000, action: {id: 'seek', positionMs: 30000}}],
    totalCount: 2, nextCursor: ''};
}

function chaptersService(payload: unknown = chaptersData()) {
  return new PlayerChaptersService({scope, pageSize: 100, timeoutMs: 1000, api: {request: async <T>() => payload as T}});
}

function lyricsService() {
  return new LyricsService({request: async <T>() => ({} as T)}, scope, {libraryId: 'library', itemId: 'item', sessionId: 'session', sessionGeneration: 2});
}

function queueSnapshot(revision = 'queue-revision'): WebQueueListSnapshot {
  return {current: true, queueId: 'queue', revision, currentEntryId: 'entry', entries: [], more: null, change: 'idle'};
}

function harness(options: Readonly<{lyricsEnabled?: boolean; queue?: boolean; lifetimeCurrent?: boolean}> = {}) {
  const seeks: number[] = [];
  const policies: {target: V1PlaybackTarget; revision: string; intent: V1QueuePolicyIntent}[] = [];
  const saves: {queueId: string; name: string; signal: AbortSignal}[] = [];
  let current = true, lifetimeCurrent = options.lifetimeCurrent !== false;
  const lifetime = {signal: new AbortController().signal, isCurrent: () => lifetimeCurrent};
  const queue = options.queue === false ? undefined : {
    getSnapshot: () => queueSnapshot(), subscribe: () => () => {}, isCurrent: () => current,
    play: async () => {}, move: async () => {}, remove: async () => {},
  } as WebQueueListPort;
  const input: WebListeningInput = {
    fence, lifetime, target, current: () => current, chapters: chaptersService(), chaptersTarget,
    lyrics: lyricsService(), lyricsEnabled: () => options.lyricsEnabled ?? true, ...(queue ? {queue} : {}),
    policy: async (expected, revision, intent) => {policies.push({target: expected, revision, intent});},
    save: async (queueId, name, signal, _guard, onProgress) => {
      saves.push({queueId, name, signal});
      const receipt: QueuePlaylistSave = {playlistId: 'playlist', revision: '1', entries: 3, state: 'saved'};
      onProgress?.(receipt);
      return receipt;
    },
    seek: async seconds => {seeks.push(seconds);},
  };
  const port = createWebListeningPort(input);
  return {port, lifetime, seeks, policies, saves,
    retire: () => {current = false;}, retireLifetime: () => {lifetimeCurrent = false;}};
}

test('the captured record loads and seeks only its own original target and drawn row', async () => {
  const h = harness();
  await h.port.chapters.load();
  const snapshot = h.port.chapters.getSnapshot();
  assert.equal(snapshot.target?.sessionId, 'session');
  assert.equal(snapshot.target?.libraryId, 'library');
  const row = snapshot.data!.chapters[1];
  await h.port.chapters.seek(snapshot, row, h.lifetime);
  assert.deepEqual(h.seeks, [30]);
  await assert.rejects(() => h.port.chapters.seek(snapshot, {...row, action: {id: 'seek', positionSeconds: 45}}, h.lifetime), /listening_command_unavailable/);
  await assert.rejects(() => h.port.chapters.seek({...snapshot}, row, h.lifetime), /listening_command_unavailable/);
});

test('queue policy and save carry the rendered revision and original queue key, never a raw mutation', async () => {
  const h = harness();
  const shown = h.port.queue!.list.getSnapshot();
  await h.port.queue!.policy(shown, {action: 'repeat', repeat: 'all'}, h.lifetime);
  assert.equal(h.policies.length, 1);
  assert.equal(h.policies[0].revision, 'queue-revision');
  assert.equal(h.policies[0].target.queueId, 'queue');
  assert.equal(h.policies[0].target.versionId, 'version');
  assert.equal(h.policies[0].target.playbackRun, 4);
  await assert.rejects(() => h.port.queue!.policy({...shown}, {action: 'shuffle', shuffled: true}, h.lifetime), /listening_command_unavailable/);
  const receipt = await h.port.queue!.save(shown, '  Night drive  ', h.lifetime);
  assert.equal(receipt.state, 'saved');
  assert.deepEqual(h.saves.map(s => [s.queueId, s.name]), [['queue', 'Night drive']]);
});

test('a retired record refuses every command and suppresses late progress', async () => {
  const h = harness();
  const shown = h.port.queue!.list.getSnapshot();
  h.port.dispose();
  assert.equal(h.port.isCurrent(), false);
  await assert.rejects(() => h.port.chapters.load(), /stale_listening_port/);
  await assert.rejects(() => h.port.queue!.policy(shown, {action: 'repeat', repeat: 'off'}, h.lifetime), /stale_listening_port/);
  h.port.dispose();
});

test('a no-longer-current caller lifetime or runtime record refuses before touching a service', async () => {
  const h = harness({lifetimeCurrent: false});
  await assert.rejects(() => h.port.chapters.load(), /stale_listening_port/);
  const h2 = harness();
  h2.retire();
  await assert.rejects(() => h2.port.chapters.load(), /stale_listening_port/);
});

test('lyrics stays withdrawn when the canonical preference is not true', async () => {
  const h = harness({lyricsEnabled: false});
  await assert.rejects(() => h.port.lyrics.refresh(), /listening_command_unavailable/);
  await assert.rejects(() => h.port.lyrics.choose(h.port.lyrics.getSnapshot(), null, h.lifetime), /listening_command_unavailable/);
  await assert.rejects(() => h.port.lyrics.seek(h.port.lyrics.getSnapshot(), 0, h.lifetime), /listening_command_unavailable/);
});

test('a record without an admitted queue exposes no queue facade at all', () => {
  const h = harness({queue: false});
  assert.equal(h.port.queue, undefined);
});
