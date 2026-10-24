/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the drawn account notifications controls do what they say.
 *
 * The Hosted account's notification feed: its loading, empty and failed states, each item's own
 * sentence, Mark all read and the unread count it reports, opening a sign-in alert (to Devices) or
 * a ready export (downloads the file and says so), Older by the server's cursor, and the one
 * server-reachability switch with its rollback on failure. The account client is stubbed.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import * as notificationsCore from '@core/account-notifications.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, textOf, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);
const tick = () => new Promise(resolve => setTimeout(resolve, 0));

const notification = (patch: any = {}) => ({id: 'n1', kind: 'new_device', read: false, occurredAt: new Date().toISOString(), detail: {name: 'Living room TV'}, ...patch});
const page = (patch: any = {}) => ({items: [], nextCursor: '', unread: 0, ...patch});

async function mount(options: {pages?: any[]; prefs?: any; savePrefs?: (next: any) => Promise<any>; exportFails?: boolean; component?: 'notifications' | 'export'; exportRun?: () => Promise<any>} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const saved: any[] = [];
  const calls: {name: string; args: any[]}[] = [];
  const pages = options.pages ?? [page()];
  let pageIndex = 0;
  const client = {
    page: (cursor?: string) => {
      calls.push({name: 'page', args: [cursor]});
      const answer = pages[Math.min(pageIndex++, pages.length - 1)];
      if (answer instanceof Error) return Promise.reject(answer);
      return Promise.resolve(answer);
    },
    markRead: (ids: any) => { calls.push({name: 'markRead', args: [ids]}); return Promise.resolve(); },
    downloadExport: (id: string) => {
      calls.push({name: 'downloadExport', args: [id]});
      return options.exportFails ? Promise.reject({message: 'export missing'}) : Promise.resolve({account: 'json'});
    },
    preferences: () => Promise.resolve(options.prefs ?? {serverOffline: true}),
    savePreferences: (next: any) => {
      calls.push({name: 'savePreferences', args: [next]});
      return options.savePrefs ? options.savePrefs(next) : Promise.resolve(next);
    },
  };
  // One API object and one counter callback per mount: the load callback must not change on render.
  const api = {request: () => Promise.resolve({})};
  const unread: number[] = [];
  const onUnread = (count: number) => unread.push(count);
  const app = await componentModule(new URL('../src/screens/account/Notifications.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); }},
    './account-hooks': {useAccountApi: () => api},
    '../../app/account-notifications': {saveJSON: (data: any) => saved.push(data)},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', errorText: (_e: any, _c: string, _o: string) => 'That did not work.'},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '@core/account-notifications.ts': {
      ...notificationsCore,
      AccountNotificationsClient: class { constructor() { Object.assign(this, client); } },
      runAccountExport: () => (options.exportRun ? options.exportRun() : Promise.resolve({state: 'ready', data: {account: 'json'}})),
    },
    './Notifications.module.css': {default: new Proxy({}, {get: (_target, key) => String(key)})},
    '../../ui': {Button: 'Button', Icon: 'Icon', Loading: 'Loading', Notice: 'Notice', SettingsGroup: 'SettingsGroup', SettingsRow: 'SettingsRow', StateView: 'StateView'},
  });
  const draw = () => (options.component === 'export' ? app.DataExport({scope: {}}) : app.Notifications({scope: {}, onUnread}));
  h.render(draw);
  await tick();
  const flat = () => tree(h.render(draw));
  return {h, flat, navigated, saved, calls, unread, client};
}

const buttons = (rendered: any[]) => all(rendered, 'button');
const byText = (rendered: any[], text: string) => buttons(rendered).find(button => textOf(button) === text)!;

test('the empty feed says so, and a failed read offers one retry', async () => {
  let screen = await mount();
  assert.equal(one(screen.flat(), 'StateView').props.title, t('web.account.notif.empty'));
  const failed = await mount({pages: [new Error('boom'), page()]});
  one(failed.flat(), 'ErrorNotice').props.retry();
  await tick();
  assert.equal(failed.calls.filter(call => call.name === 'page').length, 2);
  assert.equal(all(failed.flat(), 'ErrorNotice').length, 0);
});

test('each notification is drawn with its own sentence, and the unread count is reported', async () => {
  const items = [
    notification({id: 'n1', kind: 'new_device', detail: {name: 'Living room TV'}}),
    notification({id: 'n2', kind: 'security_event', detail: {event: 'password_changed'}, read: true}),
    notification({id: 'n3', kind: 'server_offline', server: {name: 'Attic'}}),
  ];
  const screen = await mount({pages: [page({items, unread: 2})]});
  const rendered = screen.flat();
  const feed = all(rendered, 'section').find(node => node.props['aria-label'] === t('web.account.notifications'))!;
  assert.ok(feed);
  assert.ok(textOf(feed).includes(t('web.account.notif.newDevice', {name: 'Living room TV'})));
  assert.ok(textOf(feed).includes(t('web.account.notif.event.password_changed')));
  assert.ok(textOf(feed).includes(t('web.account.notif.serverOffline', {server: 'Attic'})));
  assert.equal(screen.unread.at(-1), 2, 'the tab counts what the server said');
});

test('Mark all read is offered only with something unread, and clears the count', async () => {
  const items = [notification({id: 'n1'}), notification({id: 'n2'})];
  const screen = await mount({pages: [page({items, unread: 2})]});
  const markAll = byText(screen.flat(), t('web.account.notif.markAll'));
  assert.equal(markAll.props.disabled, false);
  markAll.props.onClick();
  await tick();
  assert.deepEqual(screen.calls.filter(call => call.name === 'markRead').map(call => call.args[0]), ['all']);
  assert.equal(screen.unread.at(-1), 0);
  assert.equal(byText(screen.flat(), t('web.account.notif.markAll')).props.disabled, true);
  // Nothing unread: the control is there but cannot be pressed.
  const quiet = await mount({pages: [page({items: [notification({read: true})], unread: 0})]});
  assert.equal(byText(quiet.flat(), t('web.account.notif.markAll')).props.disabled, true);
});

test('opening a sign-in alert reads it and goes to this account’s devices', async () => {
  const screen = await mount({pages: [page({items: [notification({id: 'n1', kind: 'new_device'})], unread: 1})]});
  byText(screen.flat(), t('web.account.notif.reviewDevices')).props.onClick();
  await tick();
  assert.deepEqual(screen.calls.filter(call => call.name === 'markRead').map(call => call.args[0]), [['n1']]);
  assert.deepEqual(screen.navigated, [{to: '/account', search: {tab: 'sessions'}, replace: true}]);
});

test('a ready export downloads from its own button and says it did, or says it failed', async () => {
  const item = notification({id: 'n1', kind: 'account_export_ready', detail: {exportId: 'x1'}});
  let screen = await mount({pages: [page({items: [item]})]});
  byText(screen.flat(), t('web.account.export.download')).props.onClick();
  await tick();
  assert.deepEqual(screen.calls.filter(call => call.name === 'downloadExport').map(call => call.args[0]), ['x1']);
  assert.deepEqual(screen.saved, [{account: 'json'}]);
  assert.ok(textOf(screen.flat()).includes(t('web.account.export.downloaded')));
  screen = await mount({pages: [page({items: [item]})], exportFails: true});
  byText(screen.flat(), t('web.account.export.download')).props.onClick();
  await tick();
  assert.equal(screen.saved.length, 0);
  assert.ok(textOf(screen.flat()).includes('That did not work.'));
});

test('Older asks for the next page by the server’s cursor', async () => {
  const first = page({items: [notification({id: 'n1'})], nextCursor: 'cursor-2', unread: 1});
  const second = page({items: [notification({id: 'n2'})], nextCursor: '', unread: 1});
  const screen = await mount({pages: [first, second]});
  byText(screen.flat(), t('web.account.notif.older')).props.onClick();
  await tick();
  assert.deepEqual(screen.calls.filter(call => call.name === 'page').map(call => call.args[0]), [undefined, 'cursor-2']);
  const feed = all(screen.flat(), 'section').find(node => node.props['aria-label'] === t('web.account.notifications'))!;
  assert.ok(textOf(feed).includes(t('web.account.notif.newDevice', {name: 'Living room TV'})));
  assert.equal(all(screen.flat(), 'button').filter(button => textOf(button) === t('web.account.notif.older')).length, 0, 'no Older on the last page');
});

test('the server-reachability switch saves its new value and rolls back if saving fails', async () => {
  let screen = await mount();
  let toggle = all(screen.flat(), 'button').find(button => button.props.role === 'switch')!;
  assert.equal(toggle.props['aria-checked'], true);
  toggle.props.onClick();
  await tick();
  assert.deepEqual(screen.calls.filter(call => call.name === 'savePreferences').map(call => call.args[0]), [{serverOffline: false}]);
  assert.equal(all(screen.flat(), 'button').find(button => button.props.role === 'switch').props['aria-checked'], false);
  // A refused save puts the switch back and says so.
  screen = await mount({savePrefs: () => Promise.reject({message: 'nope'})});
  toggle = all(screen.flat(), 'button').find(button => button.props.role === 'switch')!;
  toggle.props.onClick();
  await tick();
  assert.equal(all(screen.flat(), 'button').find(button => button.props.role === 'switch').props['aria-checked'], true);
  assert.ok(textOf(screen.flat()).includes('That did not work.'));
});

test('Download your data saves the ready export, waits while it prepares, and says when it fails', async () => {
  const download = (rendered: any[]) => all(rendered, 'Button').find(button => button.props.label === t('web.account.export.download'))!;
  // Ready: the file is saved and the row says so.
  let screen = await mount({component: 'export'});
  download(screen.flat()).props.onClick();
  await tick();
  assert.deepEqual(screen.saved, [{account: 'json'}]);
  assert.ok(all(screen.flat(), 'Notice').some(notice => textOf(notice).includes(t('web.account.export.downloaded'))));
  // Still preparing: the row says so and can be asked again.
  screen = await mount({component: 'export', exportRun: () => Promise.resolve({state: 'pending'})});
  download(screen.flat()).props.onClick();
  await tick();
  assert.equal(screen.saved.length, 0);
  assert.ok(all(screen.flat(), 'Notice').some(notice => textOf(notice).includes(t('web.account.export.preparing'))));
  assert.equal(download(screen.flat()).props.disabled, false);
  // Failed: the row says so and stays usable.
  screen = await mount({component: 'export', exportRun: () => Promise.reject({message: 'nope'})});
  download(screen.flat()).props.onClick();
  await tick();
  assert.equal(screen.saved.length, 0);
  assert.ok(all(screen.flat(), 'Notice').some(notice => textOf(notice).includes('That did not work.')));
  assert.equal(download(screen.flat()).props.disabled, false);
});
