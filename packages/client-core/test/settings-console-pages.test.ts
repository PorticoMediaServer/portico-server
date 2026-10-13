import assert from 'node:assert/strict';
import test from 'node:test';
import {createI18n} from '../../i18n/src/index.ts';
import {searchSettings, settingsStructure, type SettingsPage, type SettingsRow} from '../src/presentation/index.ts';
import {connectionLine, ownCertificateLine} from '../src/server-admin/status-words.ts';
import {withListEntry} from '../src/server-admin/server-forms.ts';

const i18n = createI18n();
const t = i18n.t;
const server = (platform: 'web' | 'phone' | 'tv') => settingsStructure({owner: true, platform, capabilities: {}}).headings.find(h => h.id === 'server')!.pages;
const page = (pages: readonly SettingsPage[], id: string) => pages.find(p => p.id === id);
const rowIds = (p: SettingsPage) => p.sections.flatMap(s => s.rows.map((r: SettingsRow) => (r.kind === 'server-setting' ? `${r.form}.${r.key}` : r.kind === 'custom' ? r.id : '')));

test('the web has one page for keys, members\' tokens, webhooks and assistants', () => {
  const integrations = page(server('web'), 'integrations')!;
  assert.deepEqual(integrations.tabs?.map(x => x.id), ['keys', 'tokens', 'webhooks', 'assistants']);
  assert.deepEqual(rowIds(integrations), ['server.apiKeys', 'runtime.apiRequestsPerMinute', 'server.memberTokens', 'runtime.blockMemberAccessTokens', 'server.webhooks', 'server.assistants']);
  // People keeps accounts and invitations; keys moved out.
  assert.deepEqual(page(server('web'), 'people')!.tabs?.map(x => x.id), ['accounts', 'invitations']);
});

test('the phone keeps its keys list under People and has no page it cannot draw', () => {
  const pages = server('phone');
  assert.equal(page(pages, 'integrations'), undefined);
  assert.equal(page(pages, 'metadata'), undefined);
  assert.deepEqual(page(pages, 'people')!.tabs?.map(x => x.id), ['accounts', 'invitations', 'keys']);
});

test('remote access carries the home networks, the address to give out, proxies, the owner\'s certificate and the connection limit', () => {
  const remote = page(server('web'), 'remote')!;
  const ids = rowIds(remote);
  for (const id of ['server.connection', 'connectivity.lanNetworks', 'connectivity.advertiseUrl', 'connectivity.accessUrls', 'connectivity.trustedProxies', 'server.ownCertificateStatus', 'connectivity.customCertificateDomain', 'connectivity.customCertificatePath', 'connectivity.customCertificateKeyPath', 'connectivity.connectionLimit']) assert.ok(ids.includes(id), id);
  // Sub-path hosting is not offered, and the two retired choices are gone.
  for (const id of ids) assert.doesNotMatch(id, /subPath|basePath|secureConnectionsPolicy|treatWanAsLan/);
  const limit = remote.sections.flatMap(s => s.rows).find(r => r.kind === 'server-setting' && r.key === 'connectionLimit');
  assert.ok(limit && limit.kind === 'server-setting' && limit.control.type === 'number' && limit.control.empty?.stores === 0);
});

test('background work speed is one named choice beside background priority', () => {
  const rows = page(server('web'), 'schedule')!.sections.flatMap(s => s.rows);
  const speed = rows.find(r => r.kind === 'server-setting' && r.key === 'backgroundSpeed');
  assert.ok(speed && speed.kind === 'server-setting' && speed.control.type === 'choice');
  assert.deepEqual(speed.control.choices.map(c => c.value), ['gentle', 'normal', 'fast']);
});

test('personal access tokens appear in a member\'s account only when the client says the member makes them', () => {
  const sections = (accessTokens: boolean, platform: 'web' | 'phone' = 'web') => settingsStructure({owner: false, platform, capabilities: {accessTokens}}).headings[0]!.pages.find(p => p.id === 'profile')!.sections.map(s => s.id);
  assert.ok(sections(true).includes('tokens'));
  assert.ok(!sections(false).includes('tokens'));
  assert.ok(!sections(true, 'phone').includes('tokens'));
});

test('the new pages are found by the settings search', () => {
  const structure = settingsStructure({owner: true, platform: 'web', capabilities: {}});
  assert.ok(searchSettings(structure, 'webhook', t).some(m => m.address === 'server-integrations'));
  assert.ok(searchSettings(structure, 'home networks', t).some(m => m.address === 'server-remote'));
  assert.ok(searchSettings(structure, 'metadata', t).some(m => m.address === 'server-metadata'));
});

test('every word the structure names exists in the catalogue', () => {
  for (const platform of ['web', 'phone', 'tv'] as const) {
    for (const p of server(platform)) {
      assert.ok(i18n.has(p.title), p.title);
      for (const tab of p.tabs ?? []) assert.ok(i18n.has(tab.title), tab.title);
      for (const section of p.sections) {
        if (section.title) assert.ok(i18n.has(section.title), section.title);
        if (section.description) assert.ok(i18n.has(section.description), section.description);
        for (const row of section.rows) {
          if ('label' in row && row.label) assert.ok(i18n.has(row.label), row.label);
          if ('help' in row && row.help) assert.ok(i18n.has(row.help), row.help);
        }
      }
    }
  }
});

test('the connection line says where the owner is and which side of the list that is', () => {
  const connection = {secure: false, remote: false, plainHttpOutsideHome: false, clientAddress: '192.168.2.14', accessUrls: []};
  assert.equal(connectionLine(t, connection)?.text, 'You are connected from 192.168.2.14, which counts as home.');
  assert.equal(connectionLine(t, {...connection, remote: true, clientAddress: '203.0.113.9'})?.text, 'You are connected from 203.0.113.9, which counts as away from home.');
  assert.equal(connectionLine(t, {...connection, clientAddress: ''}), undefined);
  assert.equal(connectionLine(t, undefined), undefined);
});

test('the owner\'s certificate says in use, or what is wrong and whether the last good one still serves', () => {
  const date = () => 'Jan 4, 2027';
  assert.equal(ownCertificateLine(t, undefined, date), undefined);
  assert.equal(ownCertificateLine(t, {serving: false, state: 'none'}, date), undefined);
  assert.deepEqual(ownCertificateLine(t, {serving: true, state: 'ready', notAfter: '2027-01-04T00:00:00Z'}, date), {tone: 'healthy', text: 'In use, valid until Jan 4, 2027.'});
  assert.deepEqual(ownCertificateLine(t, {serving: false, state: 'error', errorCode: 'custom_certificate_name_mismatch'}, date), {tone: 'danger', text: 'The certificate doesn’t cover this domain.'});
  const kept = ownCertificateLine(t, {serving: true, state: 'error', errorCode: 'custom_certificate_unreadable'}, date)!;
  assert.equal(kept.tone, 'warning');
  assert.match(kept.text, /can’t be read.*still in use\.$/);
  assert.equal(ownCertificateLine(t, {serving: false, state: 'error', errorCode: 'something_new'}, date)?.text, 'The certificate files can’t be used.');
});

test('a list setting takes an entry once, trimmed, and never an empty one', () => {
  assert.deepEqual(withListEntry(['10.0.0.0/8'], ' 100.64.0.0/10 '), ['10.0.0.0/8', '100.64.0.0/10']);
  assert.deepEqual(withListEntry(['10.0.0.0/8'], '10.0.0.0/8'), ['10.0.0.0/8']);
  assert.deepEqual(withListEntry(['10.0.0.0/8'], '   '), ['10.0.0.0/8']);
});
