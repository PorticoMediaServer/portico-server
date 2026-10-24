/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn live DVR controls do what they say.
 *
 * The DVR page inside Live TV: its five views and the read each one asks for, the empty wording
 * per view with Open guide, the capture-off and failure notices with Try again, a recording row's
 * Play/Keep/Cancel/Delete (the two destructive ones ask first), a series rule's Pause and Delete
 * (with what happens to what it already scheduled), and the cursor pager. The DVR client is
 * stubbed, so nothing leaves the process.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, textOf, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);
const tick = () => new Promise(resolve => setTimeout(resolve, 0));

const recording = (patch: any = {}) => ({
  id: 'r1', itemId: 'm1', libraryId: 'library', state: 'completed', start: '2026-10-24T19:00:00Z', end: '2026-10-24T20:00:00Z',
  bytes: 1000, keep: false, watched: null, reason: '', revision: 1,
  channel: {id: 'c1', name: 'BBC One', sourceId: 'source'}, programme: {id: 'p1', title: 'Dune', seriesId: '', episode: null}, ...patch,
});
const rule = (patch: any = {}) => ({id: 'rule1', revision: 2, config: {name: 'Dune', enabled: true, episodes: 'all', keywords: [], options: {episodeLimit: 0}}, ...patch});
const page = (patch: any = {}) => ({recordings: [], rules: [], nextCursor: '', revision: 1, captureAvailable: true, captureUnavailableReason: '', deletionAvailable: true, usage: {bytes: 0, pendingDeleteBytes: 0, recordings: 0}, ...patch});

async function mount(options: {pages?: Record<string, any>; fail?: boolean; preview?: any} = {}) {
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
    storage: () => new Promise(() => {}),
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
    '../../app/session': {useSession: () => ({owner: false})},
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

test('the five views are drawn, and choosing one reads that view', async () => {
  const screen = await mount();
  let rendered = screen.flat();
  const views = one(rendered, 'Segmented');
  assert.deepEqual(views.props.options.map((option: any) => option.id), ['upcoming', 'recorded', 'rules', 'history', 'storage']);
  assert.equal(views.props.value, 'upcoming');
  views.props.onChange('recorded');
  rendered = screen.flat();
  await tick();
  assert.deepEqual(screen.lists.at(-1), ['recorded', '']);
  assert.equal(one(screen.flat(), 'Segmented').props.value, 'recorded');
});

test('each empty view says what belongs there, and Upcoming offers the guide', async () => {
  let screen = await mount();
  await tick();
  let state = one(screen.flat(), 'StateView');
  assert.equal(state.props.title, t('web.dvr.empty.upcoming'));
  state.props.action.onClick();
  assert.deepEqual(screen.navigated.at(-1), {to: '/channels'});
  screen = await mount();
  await tick();
  one(screen.flat(), 'Segmented').props.onChange('rules');
  screen.flat();
  await tick();
  assert.equal(one(screen.flat(), 'StateView').props.title, t('web.dvr.empty.rules'));
});

test('a server that cannot capture says so, and a failed read offers Try again', async () => {
  let screen = await mount({pages: {upcoming: page({captureAvailable: false})}});
  await tick();
  assert.equal(textOf(one(screen.flat(), 'Notice')), t('web.dvr.captureOff'));
  screen = await mount({fail: true});
  await tick();
  const notice = one(screen.flat(), 'Notice');
  assert.equal(notice.props.tone, 'error');
  notice.props.action.onClick();
  await tick();
  assert.equal(screen.lists.length, 2, 'Try again reads the view again');
});

test('a finished recording opens from its row, and its menu plays or keeps it', async () => {
  const done = recording();
  const screen = await mount({pages: {upcoming: page({recordings: [done], usage: {bytes: 1000, pendingDeleteBytes: 0, recordings: 1}})}});
  await tick();
  let rendered = screen.flat();
  const row = one(rendered, 'ListRow');
  row.props.onClick();
  assert.deepEqual(screen.played, [['m1', 0]]);
  let menu = all(rendered, 'Menu')[0];
  assert.deepEqual(menu.props.items.map((item: any) => item.id), ['play', 'keep', 'delete']);
  menu.props.onSelect('play');
  menu.props.onSelect('keep');
  await tick();
  assert.deepEqual(screen.played, [['m1', 0], ['m1', 0]]);
  assert.deepEqual(screen.calls.filter(call => call.name === 'keep').map(call => call.args[0].id), ['r1']);
});

test('stopping and deleting ask first; the confirmation does the work', async () => {
  const live = recording({id: 'r1', state: 'recording', itemId: '', libraryId: ''});
  const done = recording({id: 'r2'});
  const screen = await mount({pages: {upcoming: page({recordings: [live, done]})}});
  await tick();
  let menus = all(screen.flat(), 'Menu');
  assert.deepEqual(menus.map(menu => menu.props.items.map((item: any) => item.id)), [['cancel'], ['play', 'keep', 'delete']]);
  menus[0].props.onSelect('cancel');
  let dialog = one(screen.flat(), 'ConfirmDialog');
  assert.equal(dialog.props.open, true);
  assert.equal(dialog.props.title, t('web.dvr.cancelTitle', {title: 'Dune'}));
  assert.deepEqual(screen.calls.filter(call => call.name === 'cancel'), [], 'nothing stops before the confirmation');
  dialog.props.onConfirm();
  await tick();
  assert.deepEqual(screen.calls.filter(call => call.name === 'cancel').map(call => call.args[0].id), ['r1']);
  // Deleting reads the preview first, then asks with its size and what happens to the copy.
  menus = all(screen.flat(), 'Menu').filter(menu => menu.props.items.some((item: any) => item.id === 'delete'));
  menus[0].props.onSelect('delete');
  await tick();
  dialog = one(screen.flat(), 'ConfirmDialog');
  assert.equal(dialog.props.title, t('web.dvr.deleteTitle', {title: 'Dune'}));
  dialog.props.onConfirm();
  await tick();
  assert.deepEqual(screen.calls.filter(call => call.name === 'delete').map(call => call.args[0].id), ['r2']);
});

test('a series rule pauses at once, and its delete asks what happens to what it scheduled', async () => {
  const series = rule();
  const screen = await mount({pages: {rules: page({rules: [series]})}});
  await tick();
  one(screen.flat(), 'Segmented').props.onChange('rules');
  screen.flat();
  await tick();
  const menu = all(screen.flat(), 'Menu')[0];
  assert.deepEqual(menu.props.items.map((item: any) => item.id), ['edit', 'toggle', 'delete']);
  menu.props.onSelect('toggle');
  await tick();
  const saved = screen.calls.find(call => call.name === 'saveRule')!;
  assert.deepEqual(saved.args.slice(0, 2), ['rule1', 2]);
  assert.equal(saved.args[2].enabled, false);
  // Delete: the dialog asks whether upcoming recordings stay or go.
  menu.props.onSelect('delete');
  const dialog = one(screen.flat(), 'Dialog');
  assert.equal(dialog.props.title, t('web.dvr.deleteRuleTitle', {name: 'Dune'}));
  const choice = all(tree(dialog), 'Segmented').find(node => node.props.label === t('web.dvr.upcomingChoice'))!;
  choice.props.onChange('cancel');
  // The dialog redraws with the chosen future before its button is pressed.
  const redrawn = one(screen.flat(), 'Dialog');
  const confirm = all(tree(redrawn), 'Button').find(node => node.props.label === t('web.dvr.deleteRuleAction'))!;
  confirm.props.onClick();
  await tick();
  const removed = screen.calls.find(call => call.name === 'deleteRule')!;
  assert.deepEqual([removed.args[0].id, removed.args[1]], ['rule1', 'cancel']);
});

test('the pager asks for the next page by the server’s cursor', async () => {
  const screen = await mount({pages: {upcoming: page({recordings: [recording()], nextCursor: 'cursor-2'})}});
  await tick();
  const pager = one(screen.flat(), 'ListPager');
  assert.equal(pager.props.pages.canNext, true);
  assert.equal(pager.props.pages.canPrevious, false);
  pager.props.pages.next();
  screen.flat();
  await tick();
  assert.deepEqual(screen.lists.at(-1), ['upcoming', 'cursor-2']);
  assert.equal(one(screen.flat(), 'ListPager').props.pages.canPrevious, true);
});
