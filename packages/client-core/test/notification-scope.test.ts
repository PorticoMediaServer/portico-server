import assert from 'node:assert/strict';
import test from 'node:test';
import {routePath} from '../../contracts/src/routes.ts';
import {createNotificationScope} from '../src/notification-scope.ts';

// The exact document GET /v1/notifications/capabilities publishes today, copied
// from the canonical notification fixtures; nothing here is invented.
const publishedVocabulary = {
  audiences: ['profile', 'account-admin', 'all'],
  severities: ['info', 'warning', 'critical'],
  states: ['unread', 'read', 'archived', 'all'],
  sources: ['feedback', 'downloads', 'dvr', 'security', 'scan', 'networking', 'storage', 'broadcast'],
  commands: ['retry-job', 'open-download', 'dismiss-conflict', 'review-device', 'open-feedback', 'run-scan'],
  views: ['home', 'library', 'item', 'show', 'season', 'episode', 'person', 'collection', 'downloads', 'recordings',
    'recording', 'live', 'search', 'feedback', 'notifications', 'settings', 'settings-storage', 'settings-network',
    'settings-security', 'settings-jobs', 'admin-jobs', 'admin-feedback', 'admin-devices'],
  maxLimit: 100, heartbeatMs: 20000, maxWaitMs: 60000,
};

const serverId = 'server-1';
const firstFence = 'a'.repeat(64);
const otherFence = 'b'.repeat(64);
const inbox = routePath('get_notifications_inbox');
const unread = routePath('get_notifications_unread_count');
const capabilities = routePath('get_notifications_capabilities');
const actions = routePath('post_notifications_inbox_actions');

type Call = Readonly<{path: string; method: string; body: unknown; signal?: AbortSignal}>;
type Reply = unknown | Promise<unknown>;

function transport(reply: (call: Call) => Reply) {
  const calls: Call[] = [];
  const api = {
    request: async <T>(path: string, method = 'GET', body?: unknown, signal?: AbortSignal): Promise<T> => {
      const call: Call = {path, method, body, signal};
      calls.push(call);
      return await reply(call) as T;
    },
  };
  return {api, calls};
}

const envelope = (fence: string, data: unknown) => ({scope: {serverId, viewerFence: fence}, data});
const lifetime = (over: Partial<{signal: AbortSignal; isCurrent: () => boolean; onRetired: () => void}> = {}) => ({
  serverId, signal: new AbortController().signal, isCurrent: () => true, ...over,
});
const codeOf = (error: unknown) => (error as {code?: string} | null)?.code;

test('the first admitted envelope pins the notification fence for this scope', async () => {
  const {api, calls} = transport(() => envelope(firstFence, {audience: 'profile', items: []}));
  const scope = createNotificationScope(api, lifetime());
  const first = await scope.request<{scope: {viewerFence: string}}>(inbox);
  assert.equal(first.scope.viewerFence, firstFence);
  await scope.request(unread);
  // The route and its request were passed through unchanged, once each.
  assert.deepEqual(calls.map(call => [call.path, call.method]), [[inbox, 'GET'], [unread, 'GET']]);
  assert.equal(calls[0].body, undefined);
});

test('a changed fence or another server retires the scope, aborts work and contains the sink', async () => {
  for (const reply of [envelope(otherFence, {audience: 'profile', items: []}), {scope: {serverId: 'other', viewerFence: firstFence}, data: {audience: 'profile', items: []}}]) {
    let retired = 0;
    const {api, calls} = transport(call => {
      if (calls.length === 1) return envelope(firstFence, {audience: 'profile', items: []});
      if (calls.length === 2) return new Promise((_, reject) => {
        call.signal?.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), {name: 'AbortError'})), {once: true});
      });
      return reply;
    });
    const scope = createNotificationScope(api, lifetime({onRetired: () => { retired++; throw new Error('sink cannot publish'); }}));
    await scope.request(inbox);
    const inFlight = scope.request(unread);
    await assert.rejects(scope.request(actions, 'POST', {operationId: 'op-1', audience: 'profile', operations: []}), error => {
      assert.equal(codeOf(error), 'access_refused');
      return true;
    });
    assert.equal(retired, 1, 'the changed envelope retires the original consumer once');
    assert.equal(calls[1].signal?.aborted, true, 'concurrent work is aborted with the scope');
    await assert.rejects(inFlight, error => (error as {name?: string}).name === 'AbortError');
    // A retired scope refuses further work; the throwing sink was contained.
    await assert.rejects(scope.request(inbox), error => (error as {name?: string}).name === 'AbortError');
  }
});

test('an envelope whose audience differs from the requested one is refused', async () => {
  const {api, calls} = transport(() => envelope(firstFence, {audience: 'account-admin', items: []}));
  const scope = createNotificationScope(api, lifetime());
  await assert.rejects(scope.request(`${inbox}?audience=profile`), error => codeOf(error) === 'invalid_document');
  // The action body's own audience is checked the same way, and nothing was pinned.
  await assert.rejects(scope.request(actions, 'POST', {operationId: 'op-1', audience: 'all', operations: []}), error => codeOf(error) === 'invalid_document');
  assert.equal(calls.length, 2);
});

test('a malformed first payload does not admit its fence', async () => {
  let call = 0;
  const {api} = transport(() => (++call === 1
    ? {scope: {serverId, viewerFence: 'not-a-fence'}, data: {audience: 'profile', items: []}}
    : envelope(firstFence, {audience: 'profile', items: []})));
  const scope = createNotificationScope(api, lifetime());
  await assert.rejects(scope.request(inbox), error => codeOf(error) === 'invalid_document');
  // Nothing was pinned by the refused document: the genuine fence is admitted.
  await scope.request(inbox);
});

test('the captured method and body cannot change while the request is in flight', async () => {
  const body = {operationId: 'op-1', audience: 'profile', operations: [{action: 'read', ids: ['n-1']}]};
  let release: (() => void) | undefined;
  const {api, calls} = transport(() => new Promise(resolve => { release = () => resolve(envelope(firstFence, {audience: 'profile', results: []})); }));
  const scope = createNotificationScope(api, lifetime());
  const call = scope.request(actions, 'POST', body);
  body.operations[0].action = 'archive';
  (body as {operationId: string}).operationId = 'changed';
  await Promise.resolve();
  assert.equal(calls[0].method, 'POST');
  assert.deepEqual(calls[0].body, {operationId: 'op-1', audience: 'profile', operations: [{action: 'read', ids: ['n-1']}]});
  release?.();
  await call;
});

test('a caller or lifetime signal end, and a retired or throwing current check, answer AbortError', async () => {
  const controller = new AbortController();
  const {api} = transport(() => envelope(firstFence, {audience: 'profile', items: []}));
  const scope = createNotificationScope(api, lifetime({signal: controller.signal}));
  await scope.request(inbox);
  controller.abort();
  await assert.rejects(scope.request(inbox), error => (error as {name?: string}).name === 'AbortError');

  const caller = new AbortController();
  const callerScope = createNotificationScope(transport(() => envelope(firstFence, {audience: 'profile', items: []})).api, lifetime());
  caller.abort();
  await assert.rejects(callerScope.request(inbox, 'GET', undefined, caller.signal), error => (error as {name?: string}).name === 'AbortError');

  let current = true;
  const currentScope = createNotificationScope(transport(() => envelope(firstFence, {audience: 'profile', items: []})).api, lifetime({isCurrent: () => current}));
  await currentScope.request(inbox);
  current = false;
  await assert.rejects(currentScope.request(inbox), error => (error as {name?: string}).name === 'AbortError');

  const throwing = createNotificationScope(transport(() => envelope(firstFence, {audience: 'profile', items: []})).api, lifetime({isCurrent: () => { throw new Error('retired'); }}));
  await assert.rejects(throwing.request(inbox), error => (error as {name?: string}).name === 'AbortError');
});

test('an API error keeps its identity, and a refused credential retires the scope', async () => {
  const denied = Object.assign(new Error('Access refused.'), {status: 403});
  const {api} = transport(() => { throw denied; });
  const scope = createNotificationScope(api, lifetime());
  await assert.rejects(scope.request(inbox), error => {
    assert.equal(error, denied, 'the transport error is the same error object');
    return true;
  });
  await assert.rejects(scope.request(inbox), error => (error as {name?: string}).name === 'AbortError');

  const badRequest = Object.assign(new Error('Bad request.'), {status: 400});
  const ordinary = createNotificationScope(transport(() => { throw badRequest; }).api, lifetime());
  await assert.rejects(ordinary.request(inbox), error => error === badRequest);
  await assert.rejects(ordinary.request(inbox), error => error === badRequest); // a 400 does not retire the scope
});

test('only the four notification routes, their methods, and their query keys are accepted', async () => {
  const {api, calls} = transport(() => envelope(firstFence, {audience: 'profile', items: []}));
  const scope = createNotificationScope(api, lifetime());
  for (const bad of [
    () => scope.request('/v1/notifications/unread-count?audience=profile&audience=all'),
    () => scope.request(`${inbox}?unknown=1`),
    () => scope.request(`${inbox}#fragment`),
    () => scope.request(`${unread}?cursor=x`),
    () => scope.request(inbox, 'POST', {audience: 'profile'}),
    () => scope.request(inbox, 'GET', {audience: 'profile'}),
    () => scope.request('/v1/home'),
  ]) {
    await assert.rejects(bad(), error => codeOf(error) === 'invalid_document');
  }
  assert.equal(calls.length, 0, 'a refused route never reaches the transport');
  await scope.request(`${inbox}?audience=profile&state=all&limit=40`);
  assert.equal(calls.length, 1);
});

test('the vocabulary reads the published document through the same scope', async () => {
  const {api, calls} = transport(() => envelope(firstFence, publishedVocabulary));
  const scope = createNotificationScope(api, lifetime());
  const vocabulary = await scope.vocabulary();
  assert.deepEqual(vocabulary.audiences, publishedVocabulary.audiences);
  assert.deepEqual(vocabulary.views, publishedVocabulary.views);
  assert.equal(vocabulary.maxLimit, 100);
  assert.equal(calls[0].path, capabilities);
  assert.equal(calls[0].method, 'GET');
  // A malformed vocabulary is refused, and the fence it arrived with is not used
  // as an admission for anything else.
  const malformed = createNotificationScope(transport(() => envelope(firstFence, {...publishedVocabulary, maxLimit: 0})).api, lifetime());
  await assert.rejects(malformed.vocabulary(), error => codeOf(error) === 'invalid_document');
});
