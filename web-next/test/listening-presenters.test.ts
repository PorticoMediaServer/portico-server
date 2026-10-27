import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {Children, isValidElement, type ReactNode} from 'react';
import {transformTsx} from './transform-tsx.mjs';
const loader = registerHooks({load(url, context, next) {
  const module = (source: string) => ({format: 'module' as const, source, shortCircuit: true});
  if (url.endsWith('.module.css')) return module('export default new Proxy({}, {get:(_,k)=>String(k)});');
  if (url.endsWith('/kit/controls/index.ts')) return module("export const Button='button',Segmented='segmented',Slider='slider',TextField='input';");
  if (url.endsWith('/kit/overlays/index.ts')) return module("export const Banner='banner',Dialog='dialog';");
  if (url.endsWith('/kit/artwork/index.ts')) return module("export const Artwork='artwork';");
  if (url.endsWith('/kit/states/index.ts')) return module("export const StateView='state';");
  if (url.endsWith('/kit/Icon.tsx')) return module("export const Icon='icon';");
  if (url.endsWith('.tsx')) return module(transformTsx(url));
  return next(url, context);
}});
const {AudiobookListening} = await import('../src/screens/watch/listening/AudiobookListening.tsx');
const {LyricsPanel} = await import('../src/screens/watch/listening/LyricsPanel.tsx');
const {ListeningQueue} = await import('../src/screens/watch/listening/ListeningQueue.tsx');
function elements(tree: ReactNode): any[] {const result: any[] = [];Children.forEach(tree, child => {if (isValidElement<any>(child)) {result.push(child);result.push(...elements(child.props.children));}});return result;}
const noop = () => {};
const chapter = (id: string, start: number, end: number) => ({id, index: start / 10, title: id, startSeconds: start, endSeconds: end, action: {id: 'seek', positionSeconds: start}});
const chapters: any = {generation: 1, phase: 'ready', target: {}, scope: {}, pagination: {cursor: null, canPrevious: true}, error: null,
  data: {status: 'available', totalCount: 101, nextCursor: 'opaque-2', revision: 'original', duration: 100, chapters: [chapter('first', 0, 10), chapter('second', 10, 20)]}};
const playback: any = {disposed: false, dock: {}, presentation: {kind: 'audiobook', expanded: true, playing: true, positionSeconds: 12, durationSeconds: 100, rate: 1.25, volume: .5, muted: false, controls: ['seek', 'pause', 'volume']}};
const book = (patch: any = {}) => ({snapshot: playback, title: 'A real title', subtitle: 'A real author', chapters, speedChoices: [.75, 1, 1.25], sleepChoices: [0, 15, 30], sleepMinutes: 0, skips: {back: 15, forward: 30}, onIntent: noop, onSpeed: noop, onSleep: noop, onChapter: noop, onChapterPage: noop, onChapterRetry: noop, onCollapse: noop, ...patch});

test('audiobook view uses actual part-local chapters and only offered pages/rates with original witnesses', () => {
  const calls: unknown[] = [], props = book({onChapter: (...args: unknown[]) => calls.push(args), onChapterPage: (...args: unknown[]) => calls.push(args), onSpeed: (...args: unknown[]) => calls.push(args)});
  const tree = elements(AudiobookListening(props));
  tree.find(e => e.type === 'button' && e.props['aria-current']).props.onClick();
  assert.deepEqual(calls.shift(), [chapters, chapters.data.chapters[1]]);
  const pages = tree.filter(e => e.type === 'button' && e.props.children === 'Show more');pages[0].props.onClick();
  assert.deepEqual(calls.shift(), [chapters, 'next']);
  const speed = tree.find(e => e.type === 'segmented' && e.props['aria-label'] === 'Speed');speed.props.onValueChange('2');assert.equal(calls.length, 0);
  speed.props.onValueChange('1.25');assert.deepEqual(calls.shift(), [playback, 1.25]);
  assert.equal(elements(AudiobookListening(book({snapshot: {...playback, dock: {remote: {phase: 'playing'}}}}))).length, 0);
});

test('chapter unavailable/none/error remain distinct, and paging never invents totals or requests', () => {
  for (const status of ['none', 'unavailable']) {
    const tree = elements(AudiobookListening(book({chapters: {...chapters, data: {...chapters.data, status, totalCount: 0, nextCursor: '', chapters: []}}})));
    assert.equal(tree.filter(e => e.type === 'state').length, 1);assert.equal(tree.some(e => e.props.children === 'Show more'), false);
  }
  let retried: unknown;
  const failed: any = {...chapters, phase: 'error', data: null};
  elements(AudiobookListening(book({chapters: failed, onChapterRetry: (shown: unknown) => {retried = shown;}}))).find(e => e.type === 'banner').props.action.onAction();
  assert.equal(retried, failed);
});

function lyrics(format: 'lrc' | 'text' = 'lrc'): any {
  const document = {format, text: 'First\nSecond', embeddedOffsetMs: 750, lines: [{atMs: format === 'lrc' ? 10000 : null, text: 'First'}, {atMs: format === 'lrc' ? 12000 : null, text: 'Second'}]};
  const resource = {id: 'resource-original', revision: 7, offsetMs: 500, language: 'en', provenance: {label: 'Supplied'}, document};
  return {busy: false, generation: 1, error: '', data: {selection: {resource}, source: {startSeconds: 10, duration: 20}, resources: [resource]}, warnings: [], candidates: [], preview: null};
}
test('synced lyrics highlight accepted source time once and line callbacks retain the original service snapshot', () => {
  const snapshot = lyrics(), calls: unknown[] = [];
  const tree = elements(LyricsPanel({snapshot, positionSeconds: 2.5, enabled: true, canSeek: true, onRetry: noop, onChoose: (...args: unknown[]) => calls.push(args), onSeek: (...args: unknown[]) => calls.push(args)}));
  const active = tree.find(e => e.props['aria-current']);assert.equal(active.props.children, 'Second');active.props.onClick();assert.deepEqual(calls.shift(), [snapshot, 1]);
  tree.find(e => e.type === 'button' && e.props['aria-pressed']).props.onClick();assert.deepEqual(calls.shift(), [snapshot, snapshot.data.resources[0]]);
});
test('plain/out-of-source lyrics never offer seek; disabled preference renders no lyrics or reads', () => {
  const props = {snapshot: lyrics('text'), positionSeconds: 2.5, enabled: true, canSeek: true, onRetry: noop, onChoose: noop, onSeek: noop};
  assert.equal(elements(LyricsPanel(props)).some(e => e.type === 'button' && e.props.className === 'line'), false);
  props.snapshot = lyrics();props.snapshot.data.source.startSeconds = 30;
  assert.equal(elements(LyricsPanel(props)).some(e => e.type === 'button' && e.props.className === 'line'), false);
  assert.equal(LyricsPanel({...props, enabled: false}), null);
});
test('real empty, loading and failed lyrics do not turn one another into a false no-lyrics answer', () => {
  const props: any = {snapshot: {...lyrics(), data: null}, positionSeconds: 0, enabled: true, canSeek: false, onRetry: noop, onChoose: noop, onSeek: noop};
  assert.equal(elements(LyricsPanel(props)).filter(e => e.type === 'state').length, 1);
  assert.equal(elements(LyricsPanel({...props, snapshot: {...props.snapshot, busy: true}})).some(e => e.props.role === 'status'), true);
  const failed = elements(LyricsPanel({...props, snapshot: {...props.snapshot, error: 'private transport error'}}));
  assert.equal(failed.filter(e => e.type === 'state').length, 0);assert.equal(failed.filter(e => e.type === 'banner').length, 1);
});

test('queue renders occurrence identity and forwards only that original witness; no synthetic More or play grant', () => {
  const snapshot: any = {current: true, queueId: 'queue', revision: '7', change: 'idle', more: 151,
    entries: [{entryId: 'one', title: 'Same title', playing: true}, {entryId: 'two', title: 'Same title', hidden: false, pending: false, canRemove: true, canMoveUp: false, canMoveDown: true}, {entryId: 'hidden', hidden: true}]};
  const calls: unknown[] = [], props: any = {snapshot, policy: {repeat: 'off', shuffled: false}, onEdit: (...args: unknown[]) => calls.push(args), onPolicy: (...args: unknown[]) => calls.push(args), onRefresh: noop};
  const tree = elements(ListeningQueue(props));
  const remove = tree.find(e => e.props['aria-label'] === 'Remove from queue');remove.props.onClick();assert.deepEqual(calls.shift(), [snapshot, 'two', 'remove']);
  assert.equal(tree.some(e => e.props.children === 'Show more'), false);
  assert.equal(tree.some(e => String(e.props['aria-label'] ?? '').startsWith('Play ')), false);
  assert.equal(tree.filter(e => e.props.role === 'listitem').length, 3);
  tree.find(e => e.type === 'button' && e.props.children === 'Shuffle off').props.onClick();assert.deepEqual(calls.shift(), [snapshot, {action: 'shuffle', shuffled: true}]);
});
test('phone audiobook uses the authored one-tap panels and no device volume slider', () => {
  const calls: unknown[] = [], tree = elements(AudiobookListening(book({phone: true, onShowPanel: (...args: unknown[]) => calls.push(args)})));
  assert.equal(tree.some(e => e.type === 'slider' || e.type === 'segmented'), false);
  const options = tree.filter(e => e.type === 'button' && typeof e.props.children !== 'string' && e.props.onClick && elements(e.props.children).some(child => child.type === 'b'));
  options[0].props.onClick();assert.deepEqual(calls.shift(), [playback, 'speed']);
  assert.equal(tree.some(e => e.type === 'aside'), false);
});

test('queue save uses canonical progress and the rendered occurrence/revision witness', () => {
  const snapshot: any = {current: true, entries: [], revision: '7', change: 'idle', more: null}, calls: unknown[] = [];
  const save = {open: true, name: '  Original name  ', progress: null, openDialog: noop, close: noop, changeName: noop,
    confirm: (...args: unknown[]) => calls.push(args)};
  const props: any = {snapshot, onEdit: noop, onPolicy: noop, onRefresh: noop, save};
  const dialog = elements(ListeningQueue(props)).find(e => e.type === 'dialog');
  const confirm = elements(dialog.props.footer).find(e => e.props.variant === 'primary');confirm.props.onClick();
  assert.deepEqual(calls.shift(), [snapshot, 'Original name']);
  const progress: any = {state: 'saving', entries: 50, total: 101, playlistId: 'original', revision: '7'};
  const active = elements(ListeningQueue({...props, save: {...save, progress}}));
  assert.equal(active.find(e => e.props.role === 'status').props.children, 'Saving… 50 of 101');
  const activeDialog = active.find(e => e.type === 'dialog');assert.equal(elements(activeDialog.props.footer).find(e => e.props.variant === 'primary').props.disabled, true);
});
test.after(() => loader.deregister());
