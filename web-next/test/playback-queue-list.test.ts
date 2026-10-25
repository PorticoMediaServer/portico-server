import test from 'node:test';
import assert from 'node:assert/strict';
import type {QueuePlaybackView} from '../../packages/client-core/src/queue-controller.ts';
import type {QueueSnapshot, QueueState} from '../../packages/client-core/src/queues.ts';
import type {V1QueueEditIntent} from '../../packages/client-core/src/playback-v1/v1-queue-player.ts';
import {playbackFence, queueScope} from '../../packages/player-core/src/index.ts';
import {createWebQueueListPort} from '../src/platform/playback/queue-list.ts';

function fixture(hidden = false) {
  const scope = queueScope({serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile',
    controllerId: 'controller', controllerEpoch: 'epoch', commandLaneId: 'lane'});
  const queue: QueueSnapshot = {id: 'queue', scope, revision: '1', highestSeenSequence: '0', replayWindow: {results: 256, bodies: 32},
    repeat: 'off', shuffled: false, currentEntryId: 'current', currentPlaybackId: 'session', entries: ['current', 'first', 'second'].map<QueueSnapshot['entries'][number]>((id, index) =>
      hidden && index === 2 ? {id, removed: false, hidden: true} : {id, removed: false, hidden: false, itemId: 'item_' + id,
        editionId: null, partId: null, sourceContext: {kind: 'item', id: 'item_' + id, revision: null, entryId: null}})};
  let view: QueuePlaybackView = {queue, highestSequence: '0', current: {id: 'session', generation: 1, state: 'playing'},
    next: {entryId: 'first', available: true, reason: 'ready'}, items: queue.entries.flatMap(entry => entry.hidden ? [] :
      [{entryId: entry.id, itemId: entry.itemId, title: entry.id, kind: 'movie', libraryId: 'library'}]), remainingCount: 2,
    postPlay: {nextEntryId: 'first', available: true, reason: 'ready', countdownSeconds: 0, autoplay: false,
      passoutCheckDue: false, automaticAdvances: 0, startDuringCredits: false}};
  let workspace: QueueState = {phase: 'ready', error: null, queue};
  const edits: {revision: string; intent: V1QueueEditIntent; signal: AbortSignal}[] = [], life = new AbortController();
  const port = createWebQueueListPort({queueId: queue.id, isCurrent: () => true,
    fence: playbackFence({kind: 'queue', playback: {scope, run: 1}, sessionId: 'session', generation: 1,
      itemId: 'item_current', entryId: 'current', sourceId: 'source'}),
    read: () => ({view, workspace}), subscribe: () => () => {}, play: async () => {},
    edit: async (revision, intent, lifetime) => {edits.push({revision, intent, signal: lifetime.signal});}});
  return {port, edits, life, lifetime: {signal: life.signal, isCurrent: () => !life.signal.aborted},
    replace() {view = {...view};workspace = {...workspace};}};
}

test('Up Next move passes only the drawn adjacent upcoming block and original revision/lifetime', async () => {
  const f = fixture(), snapshot = f.port.getSnapshot();
  await f.port.move(snapshot, 'second', -1, f.lifetime);
  assert.deepEqual(f.edits.map(edit => ({revision: edit.revision, intent: edit.intent})),
    [{revision: '1', intent: {action: 'reorder', entryIds: ['second', 'first']}}]);
  assert.equal(f.edits[0]!.signal, f.life.signal);
  assert.equal(snapshot.entries[1]!.canMoveUp, false, 'the current occurrence is never a move target');
});

test('Up Next refuses retired panel gestures and replacement snapshots before I/O', async () => {
  const retired = fixture(), snapshot = retired.port.getSnapshot();retired.life.abort();
  await assert.rejects(retired.port.remove(snapshot, 'first', retired.lifetime));
  assert.equal(retired.edits.length, 0);
  const replaced = fixture(), old = replaced.port.getSnapshot();replaced.replace();
  await assert.rejects(replaced.port.remove(old, 'first', replaced.lifetime));
  assert.equal(replaced.edits.length, 0);
});

test('Up Next never moves across an unavailable neighbor or removes a hidden row', async () => {
  const f = fixture(true), snapshot = f.port.getSnapshot();
  assert.equal(snapshot.entries[1]!.canMoveDown, false);
  assert.equal(snapshot.entries[2]!.canRemove, false);
  await assert.rejects(f.port.move(snapshot, 'first', 1, f.lifetime));
  await assert.rejects(f.port.remove(snapshot, 'second', f.lifetime));
  assert.equal(f.edits.length, 0);
});
