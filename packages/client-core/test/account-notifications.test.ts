import {test} from 'node:test';
import assert from 'node:assert/strict';
import {AccountNotificationsClient, parseAccountNotificationPage, parseNotificationPreferences, runAccountExport} from '../src/account-notifications.ts';

// The exact shapes BE-hosted sends (be/hosted da0a9db: postgres.Notification / NotificationFeed /
// NotificationPreferences / ExportJob; detail per kind as documented in notifications.go).
const feed = {
  items: [
    {id: 'signin_fam1', kind: 'new_device', occurredAt: '2026-09-23T10:00:00Z', read: false, detail: {deviceId: 'device_fam1', name: 'Living room', platform: 'tvos', app: 'Portico'}},
    {id: 'sec_1', kind: 'security_event', occurredAt: '2026-09-23T09:00:00Z', read: false, detail: {event: 'two_step_enabled'}},
    {id: 'off_1', kind: 'server_offline', occurredAt: '2026-09-22T09:00:00Z', read: true, server: {id: 'srv', name: 'Den'}, detail: {}},
    {id: 'export_x', kind: 'account_export_ready', occurredAt: '2026-09-22T08:00:00Z', read: true, detail: {exportId: 'export_x'}},
    {id: 'keyexp_1', kind: 'signing_key_expiring', occurredAt: '2026-09-21T08:00:00Z', read: true, detail: {keyId: 'k1', expiresAt: '2026-10-01T00:00:00Z'}},
    {id: 'future_1', kind: 'something_new', occurredAt: '2026-09-20T08:00:00Z', read: true, detail: null},
  ],
  unread: 2,
  nextCursor: 'cur_2',
};

test('the feed parses every documented kind and keeps an unknown kind as "other"', () => {
  const page = parseAccountNotificationPage(feed);
  assert.equal(page.unread, 2);
  assert.equal(page.nextCursor, 'cur_2');
  assert.deepEqual(page.items.map(i => i.kind), ['new_device', 'security_event', 'server_offline', 'account_export_ready', 'signing_key_expiring', 'other']);
  assert.equal(page.items[0]!.detail.name, 'Living room');
  assert.equal(page.items[2]!.server?.name, 'Den');
  assert.equal(page.items[5]!.rawKind, 'something_new');
  assert.equal(parseAccountNotificationPage({items: [], unread: 0}).nextCursor, undefined);
  assert.throws(() => parseAccountNotificationPage({items: [], unread: -1}));
  assert.throws(() => parseAccountNotificationPage({items: [{...feed.items[0], read: 'no'}], unread: 0}));
});

test('requests go to the documented routes with the documented bodies', async () => {
  const calls: [string, string | undefined, unknown][] = [];
  const replies: Record<string, unknown> = {
    'GET /v1/account/notifications?cursor=cur_2': {items: [], unread: 0},
    'GET /v1/account/notification-preferences': {serverOffline: true, storageNearlyFull: true, invitationAccepted: true},
    'PUT /v1/account/notification-preferences': {serverOffline: false, storageNearlyFull: true, invitationAccepted: true},
  };
  const client = new AccountNotificationsClient(async <T,>(path: string, method?: string, body?: unknown) => { calls.push([path, method, body]); return replies[`${method} ${path}`] as T; });
  await client.page('cur_2');
  await client.markRead(['a', 'b']);
  await client.markRead('all');
  await client.markRead([]);
  assert.deepEqual((await client.preferences()).serverOffline, true);
  assert.deepEqual((await client.savePreferences({serverOffline: false, storageNearlyFull: true, invitationAccepted: true})).serverOffline, false);
  assert.deepEqual(calls.slice(1, 3), [['/v1/account/notifications/read', 'POST', {ids: ['a', 'b']}], ['/v1/account/notifications/read', 'POST', {all: true}]]);
  assert.equal(calls.length, 5);
  assert.throws(() => parseNotificationPreferences({serverOffline: true}));
});

test('the data export: request, wait until ready, download (no email, no token link)', async () => {
  const calls: [string, string | undefined, unknown][] = [];
  let polls = 0;
  const client = new AccountNotificationsClient(async <T,>(path: string, method?: string, body?: unknown) => {
    calls.push([path, method, body]);
    if (path === '/v1/account/export') return {id: 'export_x', state: 'pending', expiresAt: '2026-09-24T00:00:00Z'} as T;
    if (path === '/v1/account/exports/export_x') return {id: 'export_x', state: ++polls >= 2 ? 'ready' : 'pending', expiresAt: '2026-09-24T00:00:00Z'} as T;
    if (path === '/v1/account/exports/export_x/download') return {account: {id: 'acct'}} as T;
    throw new Error('unexpected ' + path);
  });
  const op = 'op-0123456789abcdef';
  const out = await runAccountExport(client, op, {sleep: async () => {}});
  assert.equal(out.state, 'ready');
  assert.deepEqual(out.state === 'ready' && out.data, {account: {id: 'acct'}});
  assert.deepEqual(calls[0], ['/v1/account/export', 'POST', {operationId: op}]);
  assert.deepEqual(calls.at(-1), ['/v1/account/exports/export_x/download', 'POST', {}]);
  const slow = new AccountNotificationsClient(async <T,>(path: string) => ({id: 'export_y', state: 'pending', expiresAt: '2026-09-24T00:00:00Z'} as T));
  assert.equal((await runAccountExport(slow, 'op-fedcba9876543210', {sleep: async () => {}, maxWaitMs: 5000})).state, 'pending');
  await assert.rejects(runAccountExport(slow, 'short', {sleep: async () => {}}));
});
