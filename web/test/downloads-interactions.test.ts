/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn Downloads controls do what they say.
 *
 * Downloads lists what the server has prepared for this profile: each row's own actions, the
 * confirmation that stopping or removing asks first, Save (which hands the file to the browser),
 * Go to title, Show more, and the loading, empty, unavailable and failed states. The service is
 * stubbed, so nothing leaves the process.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);
const assigned: string[] = [];
(globalThis as any).window = {location: {assign: (url: string) => assigned.push(url)}, addEventListener: () => {}, removeEventListener: () => {}};

const title = (patch: any = {}) => ({kind: 'movie', title: 'Dune', subtitle: '', year: 2026, parentTitle: '', artist: '', groupNumber: null, number: null, onServer: true, ...patch});
const item = (patch: any = {}) => ({
  id: 'p1', versionId: 'v1', itemId: 'm1', libraryId: 'library', profileId: 'viewer', title: title(),
  quality: 'original', origin: 'manual', batchId: 'b1', state: 'ready', reason: '', artifact: {bytes: 1000, estimated: false},
  progress: {percent: 0, etaSeconds: 0}, actions: ['remove'], revision: 1, createdAt: '2026-10-24T00:00:00Z',
  updatedAt: '2026-10-24T00:00:00Z', readyAt: '2026-10-24T00:00:00Z', expiresAt: '', ...patch,
});
const state = (patch: any = {}) => ({phase: 'ready', items: [], requests: [], usage: null, unavailable: false, error: null, busy: [], loadingMore: false, nextCursor: '', ...patch});

async function mount(options: {state?: any; owner?: boolean} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const calls: {name: string; args: any[]}[] = [];
  const record = (name: string) => (...args: any[]) => { calls.push({name, args}); return Promise.resolve(undefined); };
  const snapshot = options.state ?? state();
  assigned.length = 0;
  const service = {
    start: record('start'), stop: record('stop'), act: record('act'), refresh: record('refresh'),
    revalidate: record('revalidate'), refreshRequests: record('refreshRequests'), loadMore: record('loadMore'),
    grant: (p: any) => { calls.push({name: 'grant', args: [p.id]}); return Promise.resolve({url: '/download/grant-1'}); },
    reason: () => '',
  };
  const app = await componentModule(new URL('../src/screens/downloads/Downloads.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); }},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../app/downloads': {useDownloads: () => ({service, state: snapshot})},
    '../../app/session': {useSession: () => ({api: {mediaUrl: (url: string) => `https://server.example${url}`}, owner: !!options.owner})},
    '../../app/freshness': {useQuietRefresh: () => {}},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', ErrorState: 'ErrorState', errorText: (error: any) => String(error?.message ?? 'failed')},
    '../../ui': {Badge: 'Badge', Button: 'Button', ConfirmDialog: 'ConfirmDialog', Inset: 'Inset', Loading: 'Loading', Notice: 'Notice', Page: 'Page', PageHeader: 'PageHeader', ProgressBar: 'ProgressBar', StateView: 'StateView', Surface: 'Surface', Text: 'Text'},
  });
  h.render(() => app.DownloadsScreen());
  const flat = () => tree(h.render(() => app.DownloadsScreen()));
  return {h, flat, navigated, calls, service, snapshot};
}

test('nothing prepared says so, and an unavailable server says why it cannot be', async () => {
  let screen = await mount();
  assert.equal(one(screen.flat(), 'StateView').props.title, t('web.downloads.emptyTitle'));
  assert.equal(one(screen.flat(), 'StateView').props.body, t('web.downloads.emptyBody'));
  screen = await mount({state: state({unavailable: true})});
  assert.equal(one(screen.flat(), 'StateView').props.title, t('web.downloads.offTitle'));
  assert.equal(one(screen.flat(), 'StateView').props.body, t('web.downloads.offMember'));
  screen = await mount({state: state({unavailable: true}), owner: true});
  assert.equal(one(screen.flat(), 'StateView').props.body, t('web.downloads.offOwner'));
});

test('stopping or removing asks first, and only the confirmation acts', async () => {
  const prepared = item({actions: ['pause', 'remove']});
  const screen = await mount({state: state({items: [prepared]})});
  const remove = all(screen.flat(), 'Button').find(button => button.props.label === t('action.removeShort'))!;
  remove.props.onClick();
  const dialog = one(screen.flat(), 'ConfirmDialog');
  assert.equal(dialog.props.open, true);
  assert.equal(dialog.props.title, t('confirm.removePrepared.title'));
  assert.equal(dialog.props.body, t('confirm.removePrepared.body'));
  assert.ok(!screen.calls.some(call => call.name === 'act'), 'nothing is removed before the confirmation');
  dialog.props.onConfirm();
  assert.deepEqual(screen.calls.filter(call => call.name === 'act').map(call => [call.args[0].id, call.args[1]]), [['p1', 'remove']]);
  // Pause is not destructive: it acts at once, with no dialog.
  all(screen.flat(), 'Button').find(button => button.props.label === t('action.pause'))!.props.onClick();
  assert.deepEqual(screen.calls.filter(call => call.name === 'act').at(-1)!.args[1], 'pause');
  assert.equal(all(screen.flat(), 'ConfirmDialog').filter(dialog => dialog.props.open).length, 0);
});

test('Save hands a ready file to the browser through its short-lived grant', async () => {
  const screen = await mount({state: state({items: [item()]})});
  all(screen.flat(), 'Button').find(button => button.props.label === t('web.downloads.save'))!.props.onClick();
  await new Promise(resolve => setTimeout(resolve, 0));
  assert.deepEqual(screen.calls.filter(call => call.name === 'grant').map(call => call.args), [['p1']]);
  assert.deepEqual(assigned, ['https://server.example/download/grant-1']);
});

test('Go to title opens the title, and Show more asks for the next page', async () => {
  const screen = await mount({state: state({items: [item()], nextCursor: 'cursor-2'})});
  all(screen.flat(), 'Button').find(button => button.props.label === t('web.downloads.goToTitle'))!.props.onClick();
  assert.deepEqual(screen.navigated, [{to: '/media/$itemId', params: {itemId: 'm1'}, search: {}}]);
  all(screen.flat(), 'Button').find(button => button.props.label === t('action.showMore'))!.props.onClick();
  assert.deepEqual(screen.calls.filter(call => call.name === 'loadMore').length, 1);
});

test('a failed list offers its one recovery, and a failed refresh keeps the list', async () => {
  let screen = await mount({state: state({phase: 'error', error: {code: 'request_failed', message: 'Downloads could not load.', retryable: true}})});
  one(screen.flat(), 'ErrorState').props.retry();
  assert.deepEqual(screen.calls.filter(call => call.name === 'refresh').length, 1);
  screen = await mount({state: state({items: [item()], error: {code: 'request_failed', message: 'Downloads could not refresh.', retryable: true}})});
  const notice = one(screen.flat(), 'ErrorNotice');
  notice.props.retry();
  assert.deepEqual(screen.calls.filter(call => call.name === 'refresh').length, 1);
});
