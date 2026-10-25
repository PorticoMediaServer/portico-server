/** Series rule editor wiring and debounced preview, through the real DVR page. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {defaultRecordingOptions} from '@core/dvr.ts';
import {dvrPaddings, dvrLimits} from '../src/screens/live/dvr-groups.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree, textOf} from './helpers/interaction-harness.mjs';
const t = (key: any, values?: any) => defaultI18n.t(key, values);
const rule = {id: 'a'.repeat(64), revision: 7, config: {name: 'Series', sourceId: 'b'.repeat(48), seriesId: 'series', enabled: true,
  episodes: 'new', allowedChannels: [], blockedChannels: [], keywords: [], blockedKeywords: [], options: {...defaultRecordingOptions, priority: 3}}};
const page = () => ({recordings: [], rules: [rule], nextCursor: '', revision: 1, captureAvailable: true, deletionAvailable: true, usage: {bytes: 0, pendingDeleteBytes: 0, recordings: 0}});
const storage = () => ({});
const detail = () => ({});
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
    preview: async (...args: any[]) => { calls.push({name: 'preview', args}); return {matches: 4}; },
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
    './dvr-groups': {groupRecordings: () => null, isExceptionBadge: () => false, dvrPaddings, dvrLimits},
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
async function settle(f: any) { for (let i = 0; i < 8; i++) { await Promise.resolve(); f.flat(); } }
async function openRule(context: any, options: Parameters<typeof mount>[0] = {}) {
  context.mock.timers.enable({apis: ['setTimeout']});
  const f = await mount(options); await settle(f);
  one(f.flat(), 'Segmented').props.onChange('rules'); await settle(f);
  one(f.flat(), 'ListRow').props.onClick(); f.flat(); return f;
}

test('rule edits save shown values with the original identity and revision', async context => {
  const f = await openRule(context);
  const original = structuredClone(rule.config);
  all(f.flat(), 'Segmented').find(node => node.props.label === t('web.dvr.record')).props.onChange('all');
  for (const [label, value] of [['web.dvr.keepLabel', '5'], ['web.dvr.startEarly', '60'], ['web.dvr.endLate', '120']]) {
    all(f.flat(), 'Select').find(node => node.props.label === t(label)).props.onChange({target: {value}});
  }
  button(f, t('action.save')).props.onClick(); await settle(f);
  const call = f.calls.find(call => call.name === 'saveRule'); assert.ok(call);
  assert.deepEqual(call.args, [rule.id, rule.revision, {...original, episodes: 'all', options: {...original.options, episodeLimit: 5, beforeSeconds: 60, afterSeconds: 120}}]);
  assert.deepEqual(rule.config, original, 'editing does not mutate the received rule');
  assert.equal(all(f.flat(), 'Dialog').length, 0);
});

test('Cancel discards edits without saving a rule', async context => {
  const f = await openRule(context);
  all(f.flat(), 'Select')[0].props.onChange({target: {value: '5'}});
  button(f, t('action.cancel')).props.onClick();
  assert.equal(all(f.flat(), 'Dialog').length, 0);
  assert.equal(f.calls.some(call => call.name === 'saveRule'), false);
  assert.equal(rule.config.options.episodeLimit, 0);
});

test('a stale save remains open with changed wording and retries only on click', async context => {
  let failed = true;
  const f = await openRule(context, {mutationFailure: () => failed});
  button(f, t('action.save')).props.onClick(); await settle(f);
  assert.equal(all(f.flat(), 'Dialog').length, 1);
  assert.equal(textOf(one(f.flat(), 'Notice')), t('web.dvr.error.changed'));
  assert.equal(f.calls.filter(call => call.name === 'saveRule').length, 1);
  failed = false; button(f, t('action.save')).props.onClick(); await settle(f);
  assert.equal(f.calls.filter(call => call.name === 'saveRule').length, 2);
  assert.equal(all(f.flat(), 'Dialog').length, 0);
});

test('preview waits 400ms after the latest edit and shows the returned count', async context => {
  const f = await openRule(context);
  context.mock.timers.tick(399); await settle(f);
  assert.equal(f.calls.filter(call => call.name === 'preview').length, 0);
  all(f.flat(), 'Select')[0].props.onChange({target: {value: '5'}}); f.flat();
  context.mock.timers.tick(399); await settle(f);
  assert.equal(f.calls.filter(call => call.name === 'preview').length, 0);
  context.mock.timers.tick(1); await settle(f);
  const previews = f.calls.filter(call => call.name === 'preview');
  assert.equal(previews.length, 1);
  assert.equal(previews[0].args[0].options.episodeLimit, 5);
  assert.equal(one(f.flat(), 'Dialog').props.description, t('web.dvr.matches', {count: 4}));
});
