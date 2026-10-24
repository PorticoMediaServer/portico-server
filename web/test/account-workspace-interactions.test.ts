/**
 * Behaviour QA (P2-BEHAVIOUR-QA): the Hosted account workspace's sections behave as drawn.
 *
 * The account page (M0: create, sign in, manage): signed out it offers sign-in in place; signed in
 * it draws the four sections with the current one marked, switches by address, lands a returning
 * identity check on Security, and labels Notifications with the feed's unread count. The account
 * client and the section bodies are stubbed; the shell's own frame is a fixture.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import * as notificationsCore from '@core/account-notifications.ts';
import {componentModule} from './helpers/component-harness.mjs';
import {all, createRenderer, one, tree} from './helpers/interaction-harness.mjs';

const t = (key: any, values?: any) => defaultI18n.t(key, values);
const tick = () => new Promise(resolve => setTimeout(resolve, 0));

async function mount(options: {search?: any; hosted?: any; unread?: number} = {}) {
  const h = createRenderer();
  const navigated: any[] = [];
  const search = {...(options.search ?? {tab: 'servers'})};
  const hosted = options.hosted === undefined ? {account: {id: 'a1', username: 'justin'}, familyId: 'f1'} : options.hosted;
  const api = {request: () => Promise.resolve({})};
  const app = await componentModule(new URL('../src/screens/account/Account.tsx', import.meta.url), {
    react: h.react,
    '@tanstack/react-router': {
      Link: 'Link',
      useNavigate: () => (to: any) => { navigated.push(to); return Promise.resolve(); },
      useSearch: () => search,
    },
    '../../app/i18n': {useI18n: () => defaultI18n},
    '@core/account-sessions': {},
    '../../bridge/account-api': {accountApi: () => api},
    './account-hooks': {useAccountApi: () => api},
    '../../app/session': {useSession: () => ({hosted, accountSignedIn: () => {}}), sessionIdentity: () => 'identity'},
    '@core/account-notifications.ts': {
      ...notificationsCore,
      AccountNotificationsClient: class { page() { return Promise.resolve({items: [], nextCursor: '', unread: options.unread ?? 0}); } },
    },
    './Notifications': {DataExport: 'DataExport', Notifications: 'Notifications'},
    './Servers': {Servers: 'Servers'},
    './Security': {AccountSecurity: 'AccountSecurity'},
    './AccountShell': {
      // The shell is presentational: a nav of its sections and the body it was given.
      AccountShell: ({sections, section, onSection, children, username}: any) => ({
        type: 'AccountShell',
        props: {username, section, onSection, children, sections},
      }),
    },
    '../auth/AccountForm': {AccountForm: 'AccountForm'},
    '../auth/AuthFrame': {AuthFrame: 'AuthFrame'},
    '../../app/errors': {ErrorNotice: 'ErrorNotice', errorText: () => 'failed'},
    '../../ui': {Badge: 'Badge', Button: 'Button', ConfirmDialog: 'ConfirmDialog', Dialog: 'Dialog', Input: 'Input', Loading: 'Loading', Notice: 'Notice', SettingsGroup: 'SettingsGroup', SettingsPage: 'SettingsPage', SettingsRow: 'SettingsRow', StateView: 'StateView', Text: 'Text', relative: () => ''},
  });
  const draw = () => app.AccountWorkspace();
  h.render(draw);
  await tick();
  const flat = () => tree(h.render(draw));
  return {h, flat, navigated, search, setSearch: (patch: any) => Object.assign(search, patch)};
}

test('signed out, the account page offers sign-in in place', async () => {
  const screen = await mount({hosted: null});
  const frame = one(screen.flat(), 'AuthFrame');
  assert.equal(frame.props.title, t('web.hostedAccount.signInTitle'));
  assert.equal(all(screen.flat(), 'AccountForm').length, 1);
  assert.equal(all(screen.flat(), 'AccountShell').length, 0);
});

test('signed in, the four sections are drawn with the current one marked', async () => {
  const screen = await mount({search: {tab: 'servers'}});
  const shell = one(screen.flat(), 'AccountShell');
  assert.deepEqual(shell.props.sections.map((section: any) => section.id), ['sessions', 'security', 'servers', 'notifications']);
  assert.deepEqual(shell.props.sections.map((section: any) => section.label), [t('devices.title'), t('account.security'), t('web.servers.title'), t('web.account.notifications')]);
  assert.equal(shell.props.section, 'servers');
  assert.equal(shell.props.username, 'justin');
  assert.equal(all(screen.flat(), 'Servers').length, 1);
  assert.equal(all(screen.flat(), 'AccountSecurity').length, 0);
});

test('choosing a section writes the address, and the address chooses the body', async () => {
  const screen = await mount({search: {tab: 'servers'}});
  one(screen.flat(), 'AccountShell').props.onSection('security');
  assert.deepEqual(screen.navigated.at(-1), {to: '/account', search: {tab: 'security'}, replace: true});
  screen.setSearch({tab: 'security'});
  const shell = one(screen.flat(), 'AccountShell');
  assert.equal(shell.props.section, 'security');
  assert.equal(all(screen.flat(), 'AccountSecurity').length, 1);
  assert.equal(all(screen.flat(), 'DataExport').length, 1, 'Security carries the export row');
  assert.equal(all(screen.flat(), 'Servers').length, 0);
});

test('a returning identity check lands on Security and clears from the address after it answers', async () => {
  const screen = await mount({search: {tab: 'servers', identityTransaction: 'tx-1'}});
  const shell = one(screen.flat(), 'AccountShell');
  assert.equal(shell.props.section, 'security');
  const security = one(screen.flat(), 'AccountSecurity');
  assert.equal(security.props.returning, 'tx-1');
  security.props.onReturned();
  assert.deepEqual(screen.navigated.at(-1), {to: '/account', search: {tab: 'security'}, replace: true});
});

test('the Notifications section is labelled with the feed’s unread count', async () => {
  let screen = await mount({unread: 3});
  let shell = one(screen.flat(), 'AccountShell');
  assert.equal(shell.props.sections.find((section: any) => section.id === 'notifications').badge, '3');
  // Nothing unread: no count on the label.
  screen = await mount({unread: 0});
  shell = one(screen.flat(), 'AccountShell');
  assert.equal(shell.props.sections.find((section: any) => section.id === 'notifications').badge, undefined);
});
