/** DVR conflict dialog: reach the real private dialog through its merged recording row. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {defaultRecordingOptions} from '@core/dvr.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree, tick, textOf} from './helpers/interaction-harness.mjs';
const t = (key: any, values?: any) => defaultI18n.t(key, values);
const recording = {id: 'a'.repeat(64), revision: 3, state: 'conflicted', reason: 'tuner-conflict', keep: false, bytes: 0,
  start: '2026-10-24T19:00:00Z', end: '2026-10-24T20:00:00Z', channel: {name: 'One'}, programme: {title: 'Programme'},
  options: {...defaultRecordingOptions, priority: 2}};
const page = () => ({recordings: [recording], rules: [], nextCursor: '', revision: 1, captureAvailable: true, deletionAvailable: true, usage: {bytes: 0, pendingDeleteBytes: 0, recordings: 1}});
const storage = () => ({});
const detail = () => ({capacity: {known: true, effective: 2}, overlaps: [{id: 'b'.repeat(64), title: 'Other programme', priority: 8, start: recording.start, end: recording.end}]});
async function mount(options: {pages?: Record<string, any>; fail?: boolean; preview?: any; storage?: any; owner?: boolean; storageFailure?: () => boolean; detail?: () => Promise<any>; mutationFailure?: () => boolean} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const lists: [string, string][] = [];
  const calls: {name: string; args: any[]}[] = [];
  const played: any[] = [];
  const record = (name: string) => (...args: any[]) => { calls.push({name, args}); return options.mutationFailure?.() ? Promise.reject({status: 409}) : Promise.resolve(undefined); };
  const dvr = {
    list: (view: string, at: string) => {
      lists.push([view, at]);
      if (options.fail && lists.length === 1) return Promise.reject({code: 'request_failed', message: 'DVR could not load.'});
      return Promise.resolve(options.pages?.[view] ?? page());
    },
    storage: () => { calls.push({name: 'storage', args: []}); return options.storageFailure?.() ? Promise.reject({status: 503}) : Promise.resolve(options.storage ?? storage()); },
    deletePreview: () => Promise.resolve(options.preview ?? {recordingId: 'r1', revision: 1, bytes: 1000, keep: false, activeReaders: false, result: ''}),
    delete: record('delete'), cancel: record('cancel'), keep: record('keep'),
    saveRule: record('saveRule'), deleteRule: record('deleteRule'), update: record('update'),
    detail: (...args: any[]) => { calls.push({name: 'detail', args}); return options.detail ? options.detail() : Promise.resolve(detail()); },
  };
  const app = await componentModule(new URL('../src/screens/live/DVR.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); }},
    './guide': {
      useChannelGuide: () => ({}),
      useDVRClient: () => dvr,
      useGuideFormat: () => ({dayLabel: () => 'Today', clock: () => '20:00', range: () => '20:00–21:00', t: defaultI18n.t, zone: 'UTC'}),
      localTimezone: () => 'UTC',
    },
    './dvr-groups': {groupRecordings: () => null, isExceptionBadge: () => false, dvrPaddings: {}, dvrLimits: {}},
    '../server/ListPager': {ListPager: 'ListPager'},
    '../../app/session': {useSession: () => ({owner: options.owner === true})},
    '../../app/errors': {errorText: () => 'failed'},
    '../../app/i18n': {useI18n: () => defaultI18n, currentI18n: () => defaultI18n},
    '../../app/freshness': {useQuietRefresh: () => {}},
    '../../player/PlayerContext': {usePlayerActions: () => ({play: (itemId: string, at: number) => played.push([itemId, at])})},
    '../../ui': {Badge: 'Badge', Button: 'Button', ConfirmDialog: 'ConfirmDialog', Dialog: 'Dialog', Inset: 'Inset', ListRow: 'ListRow', Loading: 'Loading', Menu: 'Menu', Notice: 'Notice', ProgressBar: 'ProgressBar', Segmented: 'Segmented', Select: 'Select', SettingsGroup: 'SettingsGroup', SettingsRow: 'SettingsRow', StateView: 'StateView', Surface: 'Surface', Text: 'Text'},
  });
  h.render(() => app.DVRView());
  const flat = () => tree(h.render(() => app.DVRView()));
  return {h, flat, navigated, lists, calls, played, dvr};
}



const button = (f: any, label: string) => { const found = all(f.flat(), 'Button').find(node => node.props.label === label); assert.ok(found, label); return found; };
async function openConflict(options: Parameters<typeof mount>[0] = {}, menu = false) {
  const f = await mount(options); await tick();
  if (menu) one(f.flat(), 'Menu').props.onSelect('conflict');
  else all(f.flat(), 'ListRow').find(node => node.props.title === 'Programme').props.onClick();
  f.flat(); return f;
}

test('row and menu open the exact recording conflict; Record waits for detail', async () => {
  let resolve!: (value: any) => void;
  const pending = new Promise(resolveValue => { resolve = resolveValue; });
  const f = await openConflict({detail: () => pending}, true);
  assert.deepEqual(f.calls.find(call => call.name === 'detail')?.args, [recording.id]);
  assert.equal(button(f, t('web.dvr.recordThis')).props.disabled, true);
  assert.equal(all(f.flat(), 'Loading').length, 1);
  resolve(detail()); await tick();
  assert.equal(button(f, t('web.dvr.recordThis')).props.disabled, false);
  assert.ok(all(f.flat(), 'ListRow').some(row => row.props.title === 'Other programme'));
  assert.ok(all(f.flat(), 'Text').some(node => textOf(node) === t('web.dvr.conflictBodyTuners', {count: 2})));
});

test('Record this raises above overlap priority with the original recording and options', async () => {
  const f = await openConflict(); await tick();
  button(f, t('web.dvr.recordThis')).props.onClick(); await tick();
  const call = f.calls.find(call => call.name === 'update'); assert.ok(call);
  assert.equal(call.args[0], recording);
  assert.deepEqual(call.args[1], {...recording.options, priority: 9});
  assert.equal(all(f.flat(), 'Dialog').length, 0);
  assert.equal(f.lists.length, 2, 'successful choice refreshes recordings');
});

test('Record this clamps priority at 1000 and unknown capacity uses honest wording', async () => {
  const value = detail(); value.capacity = {known: false, effective: 0}; value.overlaps[0].priority = 1000;
  const f = await openConflict({detail: async () => value}); await tick();
  assert.ok(all(f.flat(), 'Text').some(node => textOf(node) === t('web.dvr.conflictBody')));
  button(f, t('web.dvr.recordThis')).props.onClick(); await tick();
  assert.equal(f.calls.find(call => call.name === 'update')?.args[1].priority, 1000);
});

test('Skip this cancels only the selected recording; Close does not mutate', async () => {
  const f = await openConflict(); await tick();
  button(f, t('web.dvr.skipThis')).props.onClick(); await tick();
  assert.deepEqual(f.calls.find(call => call.name === 'cancel')?.args, [recording]);
  assert.equal(f.calls.some(call => call.name === 'update'), false);
  const closed = await openConflict(); await tick();
  button(closed, t('action.close')).props.onClick();
  assert.equal(all(closed.flat(), 'Dialog').length, 0);
  assert.equal(closed.calls.some(call => ['update', 'cancel'].includes(call.name)), false);
});

test('failed detail leaves Record disabled and reports the failure', async () => {
  const f = await openConflict({detail: async () => { throw {status: 503}; }}); await tick();
  assert.equal(button(f, t('web.dvr.recordThis')).props.disabled, true);
  assert.equal(textOf(one(f.flat(), 'Notice')), 'failed');
  assert.equal(all(f.flat(), 'Loading').length, 0);
});

test('a stale mutation keeps the conflict open and permits explicit retry', async () => {
  let failed = true;
  const f = await openConflict({mutationFailure: () => failed}); await tick();
  button(f, t('web.dvr.recordThis')).props.onClick(); await tick();
  assert.equal(all(f.flat(), 'Dialog').length, 1);
  assert.equal(textOf(one(f.flat(), 'Notice')), t('web.dvr.error.changed'));
  assert.equal(f.lists.length, 1);
  failed = false;
  button(f, t('web.dvr.recordThis')).props.onClick(); await tick();
  assert.equal(f.calls.filter(call => call.name === 'update').length, 2);
  assert.equal(all(f.flat(), 'Dialog').length, 0);
});
