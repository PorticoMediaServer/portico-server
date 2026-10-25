/** DVR storage view wiring. Isolated read adapters; no server mutations or rendering credit. */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree, tick} from './helpers/interaction-harness.mjs';
const t = (key: any, values?: any) => defaultI18n.t(key, values);
const page = () => ({recordings: [], rules: [], nextCursor: '', revision: 1, captureAvailable: true, deletionAvailable: true, usage: {bytes: 0, pendingDeleteBytes: 0, recordings: 0}});
const storage = () => ({measurement: {usedBytes: 300, freeBytes: 700, writeHealthy: true}, pendingDeleteBytes: 50, forecastHours: 12.9, policy: {retentionDays: 7}});
async function mount(options: {pages?: Record<string, any>; fail?: boolean; preview?: any; storage?: any; owner?: boolean; storageFailure?: () => boolean} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const lists: [string, string][] = [];
  const calls: {name: string; args: any[]}[] = [];
  const played: any[] = [];
  const record = (name: string) => (...args: any[]) => { calls.push({name, args}); return Promise.resolve(undefined); };
  const dvr = {
    list: (view: string, at: string) => {
      lists.push([view, at]);
      if (options.fail && lists.length === 1) return Promise.reject({code: 'request_failed', message: 'DVR could not load.'});
      return Promise.resolve(options.pages?.[view] ?? page());
    },
    storage: () => { calls.push({name: 'storage', args: []}); return options.storageFailure?.() ? Promise.reject({status: 503}) : Promise.resolve(options.storage ?? storage()); },
    deletePreview: () => Promise.resolve(options.preview ?? {recordingId: 'r1', revision: 1, bytes: 1000, keep: false, activeReaders: false, result: ''}),
    delete: record('delete'), cancel: record('cancel'), keep: record('keep'),
    saveRule: record('saveRule'), deleteRule: record('deleteRule'),
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


async function openStorage(options: Parameters<typeof mount>[0] = {}) {
  const f = await mount(options);
  await tick();
  one(f.flat(), 'Segmented').props.onChange('storage');
  f.flat();
  await tick();
  return f;
}

test('Storage reads its endpoint and shows usage, pending deletion and remaining hours', async () => {
  const f = await openStorage();
  const rendered = f.flat();
  assert.equal(f.calls.filter(call => call.name === 'storage').length, 1);
  assert.equal(one(rendered, 'ProgressBar').props.value, 0.3);
  const rows = all(rendered, 'SettingsRow');
  assert.ok(rows.some(row => row.props.label === t('web.dvr.used', {used: defaultI18n.bytes(300), free: defaultI18n.bytes(700)}) && row.props.help === t('web.dvr.hoursLeft', {count: 12})));
  assert.ok(rows.some(row => row.props.label === t('web.dvr.pendingDelete', {size: defaultI18n.bytes(50)})));
  assert.equal(all(rendered, 'ListPager').length, 0);
});

test('unhealthy storage gives owner help and retention only to the owner', async () => {
  const value = storage(); value.measurement.writeHealthy = false;
  for (const owner of [false, true]) {
    const f = await openStorage({storage: value, owner});
    const rows = all(f.flat(), 'SettingsRow');
    const warning = rows.find(row => row.props.label === t('web.dvr.writeUnhealthy'));
    assert.ok(warning);
    assert.equal(warning.props.help, owner ? t('web.dvr.writeUnhealthyOwner') : undefined);
    assert.equal(rows.some(row => row.props.label === t('web.dvr.retention', {count: 7})), owner);
  }
});

test('zero capacity omits the usage bar and empty optional storage facts', async () => {
  const value = storage();
  value.measurement.usedBytes = 0; value.measurement.freeBytes = 0;
  value.pendingDeleteBytes = 0; value.forecastHours = 0; value.policy.retentionDays = 0;
  const f = await openStorage({storage: value, owner: true});
  assert.equal(all(f.flat(), 'ProgressBar').length, 0);
  const rows = all(f.flat(), 'SettingsRow');
  assert.equal(rows.length, 1);
  assert.equal(rows[0].props.help, undefined);
});

test('failed Storage read retries Storage and recovers without reading another list', async () => {
  let failed = true;
  const f = await openStorage({storageFailure: () => failed});
  const before = f.lists.length;
  const notice = one(f.flat(), 'Notice');
  assert.equal(notice.props.tone, 'error');
  failed = false;
  notice.props.action.onClick();
  await tick();
  assert.equal(f.calls.filter(call => call.name === 'storage').length, 2);
  assert.equal(f.lists.length, before);
  assert.equal(all(f.flat(), 'Notice').length, 0);
  assert.equal(one(f.flat(), 'ProgressBar').props.value, 0.3);
});
