import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {enUS} from '@i18n';
import {componentModule, hooks} from './helpers/component-harness.mjs';

const src = (p: string) => readFileSync(new URL('../src/' + p, import.meta.url), 'utf8');

// ── Gate 1: Link a TV ─────────────────────────────────────────────────────────────────────

test('gate 1: Settings › Link a TV opens /device, as the TV says; the unreachable Quick Connect is gone', () => {
  // The row is authored in client-core's settings structure; the web opens /device from it.
  assert.match(src('screens/settings/SettingsRows.tsx'), /row\.id === 'linkTV'\)[^\n]*navigate\(\{to: '\/device'\}\)/);
  assert.match(readFileSync(new URL('../../packages/client-core/src/presentation/settings-structure.ts', import.meta.url), 'utf8'), /id: 'linkTV', label: 'settings\.linkTV'/);
  assert.match(enUS['quickConnect.lede'], /Settings › Link a TV/);
  assert.match(enUS['quickConnect.lede'], /\{host\}\/device/, 'the TV points a computer at /device');
  assert.equal(enUS['web.device.title'], enUS['settings.linkTV'], 'the page is named what the TV calls it');
  assert.doesNotMatch(src('bridge/setup-codes.ts'), /browserQuickConnect|quick-connect/);
});

test('gate 1: /device reviews a TV code with the Portico Account and approves it', async () => {
  const h = hooks();
  const calls: string[] = [];
  const preview = {deviceName: 'Living room', platform: 'tvos', userCode: 'ABCD-EFGH'};
  class SetupCodeApprover {
    constructor(_api: unknown, authority: string) { calls.push('approver:' + authority); }
    async review(code: string) { calls.push('review:' + code); return preview; }
    async decide(p: unknown, decision: string) { calls.push('decide:' + decision + ':' + (p === preview)); }
  }
  const live = {accessToken: 'tok', account: {id: 'acc', displayName: 'Alice', username: 'alice'}, familyId: 'fam'};
  const central = {api: {origin: 'https://accounts.example'}, service: {subscribe: () => () => {}, getSnapshot: () => ({session: live}), accessSession: async () => live, synchronize: async () => { calls.push('sync'); }}};
  const m = await componentModule(new URL('../src/screens/auth/DeviceApproval.tsx', import.meta.url), {
    react: h.react, '@core/index.ts': {HttpLocalApi: class {}}, '@core/setup-code.ts': {SetupCodeApprover},
    '../../bridge/account': {browserAccount: () => central}, '../../app/session': {useSession: () => ({session: undefined, owner: false})},
    '../../app/i18n': {useI18n: () => ({t: (id: string) => id})}, '../../app/link-fragment': {useFragmentSecrets: () => ({code: 'ABCD-EFGH'})},
    './AccountForm': {AccountForm: 'AccountForm'}, '../../ui': {Button: 'Button', Input: 'Input', KeyValue: 'KeyValue', Notice: 'Notice', QR: 'QR', Spinner: 'Spinner', Text: 'Text'},
    './AuthFrame': {AuthFrame: 'AuthFrame'}, './Auth.module.css': {default: {}}, '../../app/errors': {errorText: () => ''},
  }) as any;
  const render = () => h.render(() => m.DeviceApprovalScreen());
  const find = (node: any, match: (n: any) => boolean): any => {
    if (!node || typeof node !== 'object') return undefined;
    if (Array.isArray(node)) { for (const c of node) { const f = find(c, match); if (f) return f; } return undefined; }
    if (match(node)) return node;
    return find(node.props?.children, match);
  };
  const settle = () => new Promise(r => setTimeout(r, 0));
  let view = render();
  assert.equal(view.props.title, 'web.device.title');
  find(view, n => n.type === 'form').props.onSubmit({preventDefault() {}});
  await settle(); await settle();
  view = render();
  assert.ok(find(view, n => n.type === 'KeyValue'), 'the TV’s details are shown for review');
  find(view, n => n.type === 'Button' && n.props.label === 'web.device.approveDevice').props.onClick();
  await settle(); await settle();
  view = render();
  assert.deepEqual(calls, ['approver:hosted', 'review:ABCD-EFGH', 'sync', 'decide:approve:true']);
  assert.match(String(find(view, n => n.type === 'Notice').props.children), /Approved/);
});
