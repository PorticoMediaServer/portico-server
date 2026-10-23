import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {transformTsx} from './transform-tsx.mjs';
import {dispatchWatch, watchClock, watchIsPortrait, watchSeek, watchSeekBounds} from '../src/screens/watch/model.ts';
import {readWatchFileInfo, readWatchFiles, watchFileGroups} from '../src/screens/watch/file-info.ts';
import {parseSession} from '@core/playback-v1/types.ts';
import type {FileInfoProjection} from '@core/detail.ts';
import type {PlaybackOptions, MediaVersion} from '@core/playback-v1/types.ts';
import type {WebPlaybackAction, WebPlaybackRuntimeSnapshot} from '../src/platform/playback/types.ts';
import type {QueuePlaybackFence} from '../../packages/player-core/src/index.ts';
import {PlaybackDock} from '../../packages/player-core/src/index.ts';
import {parseTrickplay, trickplayFrameAt} from '@core/trickplay.ts';
import {createArtworkRuntime} from '../src/kit/artwork/source.ts';
import {scrubImageStyle, watchPreviewFrame} from '../src/screens/watch/preview.ts';

function previewSet() {
  const id = 'a'.repeat(64);
  return parseTrickplay({scope: {serverId: 'server', libraryId: 'library', itemId: 'item', viewerFence: 'viewer', sessionId: '', sessionGeneration: 0}, sets: [{
    id, revision: 'b'.repeat(64), sourceId: 'source', sourceRevision: 'revision', width: 640, height: 360, tileWidth: 320, tileHeight: 180,
    columns: 2, rows: 2, intervalMs: 10000, durationMs: 60000, sourceOffsetMs: 0, tileCount: 2, frameCount: 6, stale: false,
    tilesUrl: `/v1/items/item/trickplay/${id}/tiles`, thumbnailsUrl: `/v1/items/item/trickplay/${id}/thumbnails.vtt`,
  }]}, 'server', {libraryId: 'library', itemId: 'item'}).sets[0]!;
}

const authority = Object.freeze({serverId: 'server', authority: 'local' as const, accountId: 'account', profileId: 'profile', epoch: 1, viewerFence: 'viewer-fence'});
const fence: QueuePlaybackFence = {kind: 'queue', playback: {scope: {serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile', controllerId: 'controller', controllerEpoch: 'epoch', commandLaneId: 'lane'}, run: 1}, sessionId: 'session', itemId: 'item', sourceId: 'source', entryId: 'entry', generation: 1};
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
    {fence: {...fence, playback: {...fence.playback, scope: {...fence.playback.scope, profileId: 'other'}}}},
  ];
  for (const replacement of cases) {
    let count = 0;
    await assert.rejects(dispatchWatch({getSnapshot: () => snapshot(replacement), onAction: async () => { count++; }}, shown, {kind: 'seek', seconds: 50}), /stale_watch_command/);
    assert.equal(count, 0);
  }
});
test('a retained local callback cannot act after an actual confirmed remote dock takeover', async () => {
  const shown = snapshot(), dock = new PlaybackDock(fence.playback.scope);
  const playback = {kind: 'video' as const, fence, queue: null, positionSeconds: 30, playing: true, expanded: true};
  dock.start(playback);
  dock.beginRemote({id: 'cast', kind: 'cast', phase: 'connecting', sequence: '1', playback, positionSeconds: 0}, 0);
  assert.equal(dock.getSnapshot().local?.playing, true);
  let effects = 0;
  await dispatchWatch({getSnapshot: () => snapshot({dock: dock.getSnapshot()}), onAction: async () => { effects++; }}, shown, {kind: 'pause'});
  assert.equal(effects, 1);
  dock.remote({id: 'cast', kind: 'cast', phase: 'playing', sequence: '2', playback, positionSeconds: 35}, 1000);
  assert.equal(dock.getSnapshot().local?.playing, false);
  await assert.rejects(dispatchWatch({getSnapshot: () => snapshot({dock: dock.getSnapshot()}), onAction: async () => { effects++; }}, shown, {kind: 'seek', seconds: 60}), /stale_watch_command/);
  assert.equal(effects, 1);
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

test('preview selection never requests an obsolete or retired resource', () => {
  const set = previewSet(), frame = trickplayFrameAt(set, 35)!;
  let calls = 0, current = true;
  const artwork = createArtworkRuntime({scopeKey: 'scope', load: async () => { throw new Error('preview_selection_must_not_load'); }, classify: () => ({retryable: false})});
  const source = artwork.source({url: frame.tileUrl, cacheKey: 'tile'});
  const port = {fence, isCurrent: () => current, frameAt: (seconds: number) => { calls++; const frame = trickplayFrameAt(set, seconds); return frame ? {frame, source} : null; }};
  assert.equal(watchPreviewFrame(snapshot(), port, 35)?.frame.index, 3);
  assert.equal(calls, 1);
  for (const shown of [snapshot({disposed: true}), snapshot({fence: {...fence, sourceId: 'other'}}), snapshot({fence: {...fence, generation: 2}})]) assert.equal(watchPreviewFrame(shown, port, 35), null);
  for (const seconds of [NaN, Infinity, -1, 7201, null]) assert.equal(watchPreviewFrame(snapshot(), port, seconds), null);
  current = false;
  assert.equal(watchPreviewFrame(snapshot(), port, 35), null);
  assert.equal(calls, 1);
  artwork.dispose();
});

test('preview crops use canonical sprite cells and reject missing or inconsistent image dimensions', () => {
  const set = previewSet(), frame = trickplayFrameAt(set, 35)!;
  assert.deepEqual(scrubImageStyle(frame, {width: 640, height: 360}, false), {position: 'absolute', width: 512, height: 288, left: -256, top: -144});
  const phone = scrubImageStyle(frame, {width: 640, height: 360}, true)!;
  assert.equal(phone.width, 356);
  assert.equal(phone.left, -178);
  for (const image of [{}, {width: 320, height: 180}, {width: NaN, height: 360}]) assert.equal(scrubImageStyle(frame, image, false), null);
});

test('File info uses its real bounded endpoint and discards a read after a source change', async () => {
  const requests: string[] = [], signal = new AbortController().signal, shown = snapshot();
  let live = shown;
  let release: (value: unknown) => void = () => {};
  const wait = new Promise(resolve => { release = resolve; });
  const source = {scope: {serverId: 'server', viewerId: 'viewer'}, authority, getSnapshot: () => live, isCurrent: () => true,
    api: {request: async <T>(path: string, _method?: string, _body?: unknown, requestedSignal?: AbortSignal): Promise<T> => {
      requests.push(path); assert.equal(requestedSignal, signal); return await wait as T;
    }}};
  const pending = readWatchFileInfo(source, fence, {libraryId: 'library', itemId: 'item'}, signal);
  live = snapshot({fence: {...fence, sourceId: 'replacement'}});
  release({scope: {serverId: 'server', libraryId: 'library', itemId: 'item', viewerFence: 'viewer-fence'}, revision: {catalog: 1, viewer: 1}, kind: 'movie', files: []});
  assert.equal(await pending, null);
  assert.deepEqual(requests, ['/v1/items/item/file-info?limit=50']);
  assert.equal(await readWatchFileInfo(source, fence, {libraryId: 'library', itemId: 'other-title'}, signal), null);
  assert.equal(requests.length, 1);
});

test('File info joins only actual server groups, with no raw-file count or inferred member', () => {
  const file = (id: string, available: boolean) => ({id, available, container: 'mkv', duration: 100, size: 1, audio: [], subtitles: []});
  const files: FileInfoProjection = {scope: {serverId: 'server', libraryId: 'library', itemId: 'item', viewerFence: 'viewer'}, revision: {catalog: 1, viewer: 1}, kind: 'movie', files: [file('first-part', true), file('second-part', true), file('unavailable', false)], nextCursor: 'next-page'};
  const version = (id: string, parts: string[]): MediaVersion => ({id, label: '', durationMs: 200000, parts: parts.map((id, index) => ({id, index, durationMs: 100000})), video: [], audio: [], subtitles: []});
  const options: PlaybackOptions = {itemId: 'item', kind: 'movie', versions: [version('first-part', ['first-part', 'second-part']), version('other-version', ['other-version'])], chapters: [], markers: [], qualities: []};
  const grouped = watchFileGroups(files, options);
  assert.equal(grouped.availableVersionCount, 2);
  assert.equal(grouped.groups[0]!.members[1]!.file, files.files[1]);
  assert.equal(grouped.groups[1]!.members[0]!.file, undefined);
  assert.deepEqual(grouped.ungroupedFiles, [files.files[2]]);
  assert.throws(() => watchFileGroups(files, {...options, itemId: 'another-title'}), /target_changed/);
});

test('paged File info retains current offered groups and session version without picking a physical member', async () => {
  const options: PlaybackOptions = {itemId: 'item', kind: 'movie', versions: [{id: 'group', label: '', durationMs: 200000, parts: [{id: 'one', index: 0, durationMs: 100000}, {id: 'two', index: 1, durationMs: 100000}], video: [], audio: [], subtitles: []}], chapters: [], markers: [], qualities: []};
  const session = parseSession({id: 'session', revision: '1', kind: 'vod', role: 'local', state: 'playing', itemId: 'item', versionId: 'group', presentation: {generation: 1, mode: 'direct', engine: 'native', url: '/v1/playback/sessions/session/media'}});
  let live = snapshot({options, session});
  const paths: string[] = [], controller = new AbortController();
  const projection = {scope: {serverId: 'server', libraryId: 'library', itemId: 'item', viewerFence: 'viewer'}, revision: {catalog: 1, viewer: 1}, kind: 'movie', files: [{id: 'two', available: true, container: 'mkv', duration: 100, size: 1, audio: [], subtitles: []}]};
  let respond = async () => projection;
  const source = {scope: {serverId: 'server', viewerId: 'viewer'}, authority, getSnapshot: () => live, isCurrent: () => true,
    api: {request: async <T>(path: string): Promise<T> => { paths.push(path); return await respond() as T; }}};
  const page = await readWatchFiles(source, fence, {libraryId: 'library', itemId: 'item'}, controller.signal, {cursor: 'second page&next'});
  assert.deepEqual(paths, ['/v1/items/item/file-info?cursor=second+page%26next&limit=50']);
  assert.equal(page?.selectedVersionId, 'group');
  assert.equal(page?.availableVersionCount, 1);
  assert.equal(page?.groups[0]?.members[0]?.file, undefined);
  assert.equal(page?.groups[0]?.members[1]?.file?.id, 'two');
  assert.equal(Object.hasOwn(page!, 'pickedFileId'), false);
  respond = async () => { live = snapshot({options: {...options}, session}); return projection; };
  assert.equal(await readWatchFiles(source, fence, {libraryId: 'library', itemId: 'item'}, controller.signal), null);
});

test('File info suppresses a late failed read only after its captured source has retired', async () => {
  const controller = new AbortController(), shown = snapshot();
  let fail = async () => { throw new Error('actual_transport_failure'); };
  const source = {scope: {serverId: 'server', viewerId: 'viewer'}, authority, getSnapshot: () => shown, isCurrent: () => true,
    api: {request: async <T>(): Promise<T> => fail()}};
  await assert.rejects(readWatchFileInfo(source, fence, {libraryId: 'library', itemId: 'item'}, controller.signal), /actual_transport_failure/);
  fail = async () => { controller.abort(); throw new Error('actual_transport_failure'); };
  assert.equal(await readWatchFileInfo(source, fence, {libraryId: 'library', itemId: 'item'}, controller.signal), null);
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
  const dock = new PlaybackDock(fence.playback.scope), playback = {kind: 'video' as const, fence, queue: null, positionSeconds: 30, playing: true, expanded: true};
  dock.start(playback);
  dock.beginRemote({id: 'cast', kind: 'cast', phase: 'connecting', sequence: '1', playback, positionSeconds: 0}, 0);
  const pending = snapshot({dock: dock.getSnapshot()});
  const render = (value: WebPlaybackRuntimeSnapshot) => renderToStaticMarkup(createElement(WatchScreen, {
    binding: {runtime: {getSnapshot: () => value} as never, artwork: {} as never, environment: {t: (key: string) => key} as never, skips: {back: 10, forward: 30}, reportError: error => { throw error; }},
    snapshot: value, expanded: true, surfaceSlotRef: {current: null}, onCollapse() {},
  }));
  assert.doesNotMatch(render(pending), /class="watch" hidden/);
  dock.remote({id: 'cast', kind: 'cast', phase: 'playing', sequence: '2', playback, positionSeconds: 35}, 1000);
  assert.match(render(snapshot({dock: dock.getSnapshot()})), /class="watch" hidden/);
});
