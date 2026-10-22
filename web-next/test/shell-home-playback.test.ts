import test from 'node:test';
import assert from 'node:assert/strict';
import {PlaybackDock, type DockPlayback, type PlaybackFence, type QueueScope} from '../../packages/player-core/src/index.ts';
import type {WebPlaybackRuntimeSnapshot} from '../src/platform/playback/index.ts';
import type {DockPresentation} from '../src/kit/dock/index.ts';
import {hasDock, PlaybackPresentations} from '../src/shell/playback-model.ts';
import {homeFixtures as f} from '../../packages/i18n/src/catalog/en-US/next/dev/home-fixtures.ts';

const scope: QueueScope = {serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile', controllerId: 'v1', controllerEpoch: 'v1', commandLaneId: 'v1'};
const fence = (id: string, generation = 1): PlaybackFence => ({playback: {scope, run: 1}, sessionId: id, itemId: id, entryId: id, sourceId: id, generation});
const audio: DockPlayback = {fence: fence('song'), kind: 'music', positionSeconds: 107, playing: true, expanded: false, queue: null};
const video: DockPlayback = {fence: fence('movie'), kind: 'video', positionSeconds: 845, playing: true, expanded: true, queue: null};
function presentation(value: DockPlayback): DockPresentation { return {fence: value.fence, identity: {id: value.fence.itemId, kind: value.kind === 'music' ? 'music' : 'video', title: f.movie, subtitle: []}, controls: ['play', 'pause'], skipBackSeconds: 10, skipForwardSeconds: 30}; }
function snapshot(dock: PlaybackDock): WebPlaybackRuntimeSnapshot { return {dock: dock.getSnapshot()} as WebPlaybackRuntimeSnapshot; }

test('presentation retention follows actual parked/bookmarked fences and clears when the source goes away', () => {
  const dock = new PlaybackDock(scope), values = new PlaybackPresentations();
  dock.start(audio); assert.equal(values.update(snapshot(dock), presentation(audio)).length, 1);
  dock.start(video); const parked = values.update(snapshot(dock), presentation(video));
  assert.equal(parked.length, 2); assert.ok(parked.some(value => value.fence === audio.fence));
  dock.reset(scope); assert.deepEqual(values.update(snapshot(dock), undefined), []);
  dock.start(video); values.update(snapshot(dock), presentation(video)); values.clear();
  assert.deepEqual(values.update(snapshot(dock), undefined), []);
});
test('a source generation replacement cannot reuse the old display identity', () => {
  const dock = new PlaybackDock(scope), values = new PlaybackPresentations(); dock.start(video); values.update(snapshot(dock), presentation(video));
  const replaced = {...video, fence: fence('movie', 2)}; dock.start(replaced);
  const current = values.update(snapshot(dock), presentation(replaced));
  assert.equal(current.length, 1); assert.equal(current[0].fence.generation, 2);
});
test('spacing follows accepted dock arbitration, including remote ownership while local video is expanded', () => {
  const dock = new PlaybackDock(scope); assert.equal(hasDock(snapshot(dock)), false);
  dock.start(video); assert.equal(hasDock(snapshot(dock)), false);
  dock.collapse(video.fence); assert.equal(hasDock(snapshot(dock)), true);
  dock.expand(video.fence); dock.beginRemote({id: 'remote', kind: 'play-on', phase: 'connecting', sequence: '1', playback: video, positionSeconds: 845}, 0);
  assert.equal(hasDock(snapshot(dock)), true);
});
