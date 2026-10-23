import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {transformTsx} from './transform-tsx.mjs';
import {dispatchWatch, watchClock, watchIsPortrait, watchSeek, watchSeekBounds} from '../src/screens/watch/model.ts';
import type {WebPlaybackAction, WebPlaybackRuntimeSnapshot} from '../src/platform/playback/types.ts';
import type {QueuePlaybackFence} from '../../packages/player-core/src/index.ts';

const authority = Object.freeze({serverId: 'server', viewerId: 'viewer', epoch: 1, viewerFence: 'viewer-fence'});
const fence: QueuePlaybackFence = {kind: 'queue', playback: {scope: {serverId: 'server', viewerId: 'viewer', controllerId: 'controller', controllerEpoch: 'epoch', commandLaneId: 'lane'}, run: 1}, sessionId: 'session', itemId: 'item', sourceId: 'source', entryId: 'entry', generation: 1};
function snapshot(change: Partial<WebPlaybackRuntimeSnapshot> = {}): WebPlaybackRuntimeSnapshot {
  return {authority, sequence: 1, disposed: false, fence,
    presentation: {fence, subject: {kind: 'video', item: {id: 'item', kind: 'movie', title: 'Title'}}, kind: 'video', durationSeconds: 7200, positionSeconds: 30, playing: true, expanded: true, finished: false, controls: ['pause', 'seek'], volume: .7, muted: false, rate: 1, policy: null},
    playback: {} as WebPlaybackRuntimeSnapshot['playback'], dock: {} as WebPlaybackRuntimeSnapshot['dock'], presentations: [], queueView: null, options: null, offers: null, ...change};
}

test('the screen sends the displayed complete fence through the existing runtime command', async () => {
  const shown = snapshot(), calls: WebPlaybackAction[] = [];
  await dispatchWatch({getSnapshot: () => snapshot({sequence: 2}), onAction: async action => { calls.push(action); }}, shown, {kind: 'pause'});
  assert.deepEqual(calls, [{kind: 'pause', fence}]);
});
test('retained screen and keyboard callbacks reject every changed fence and authority before an effect', async () => {
  const shown = snapshot(), cases: Partial<WebPlaybackRuntimeSnapshot>[] = [
    {disposed: true}, {fence: undefined}, {authority: {...authority, viewerFence: 'new-viewer'}},
    {fence: {...fence, sessionId: 'replacement'}}, {fence: {...fence, sourceId: 'replacement'}},
    {fence: {...fence, generation: 2}}, {fence: {...fence, entryId: 'another-occurrence'}},
    {fence: {...fence, playback: {...fence.playback, run: 2}}},
    {fence: {...fence, playback: {...fence.playback, scope: {...fence.playback.scope, viewerId: 'other'}}}},
  ];
  for (const replacement of cases) {
    let count = 0;
    await assert.rejects(dispatchWatch({getSnapshot: () => snapshot(replacement), onAction: async () => { count++; }}, shown, {kind: 'seek', seconds: 50}), /stale_watch_command/);
    assert.equal(count, 0);
  }
});
test('seek bounds respect the actual retained live window and never invent a clock', () => {
  const live = snapshot();
  const withWindow = {...live, presentation: {...live.presentation!, seekWindow: {startSeconds: 1800, endSeconds: 2100}}};
  assert.deepEqual(watchSeekBounds(withWindow), {start: 1800, end: 2100});
  assert.equal(watchSeek(withWindow, 1700), 1800);
  assert.equal(watchSeek(withWindow, 2200), 2100);
  assert.equal(watchSeek(withWindow, NaN), null);
  for (const presentation of [undefined, {...live.presentation!, controls: []}, {...live.presentation!, durationSeconds: NaN}, {...live.presentation!, durationSeconds: 0}]) assert.equal(watchSeekBounds({...live, presentation}), null);
});
test('clock lengths remain exact while window shape changes presentation only', () => {
  assert.equal(watchClock(3186), '53:06');
  assert.equal(watchClock(3601), '1:00:01');
  assert.equal(watchClock(59.9), '0:59');
  assert.equal(watchClock(NaN), '');
  assert.equal(watchIsPortrait(390, 844), true);
  assert.equal(watchIsPortrait(834, 1194), true);
  assert.equal(watchIsPortrait(890, 626), false);
  assert.equal(watchIsPortrait(1440, 810), false);
});

test('the actual screen renders one empty picture target and real identity, without owning media', async context => {
  const loader = registerHooks({load(url, context, next) {
    if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
    if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
    return next(url, context);
  }});
  context.after(() => loader.deregister());
  const {WatchScreen} = await import('../src/screens/watch/WatchScreen.tsx');
  const shown = snapshot();
  const html = renderToStaticMarkup(createElement(WatchScreen, {
    binding: {runtime: {getSnapshot: () => shown, onAction: async () => { throw new Error('render_must_not_command'); }} as never, artwork: {} as never, environment: {t: (key: string) => key} as never, skips: {back: 10, forward: 30}, reportError: error => { throw error; }},
    snapshot: shown, expanded: true, surfaceSlotRef: {current: null}, onCollapse() {},
  }));
  assert.equal((html.match(/data-watch-picture-slot/g) ?? []).length, 1);
  assert.equal((html.match(/data-watch-retained-body/g) ?? []).length, 1);
  assert.doesNotMatch(html, /<video|<audio|Movie night|1080p/);
  assert.match(html, /<h2>Title<\/h2>/);
  assert.match(html, /aria-label="Pause"/);
});
