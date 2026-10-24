import test from 'node:test';
import assert from 'node:assert/strict';
import {createRecordingsController} from '../src/screens/live/recordings-controller.ts';
import type {ChannelApi} from '@core/channel-guide.ts';

type Call = Readonly<{path: string; method: string; body?: unknown; signal?: AbortSignal}>;
type Reply = (call: Call) => unknown;
const hex = (length = 64, character = 'a') => character.repeat(length);
const time = '2026-09-06T12:00:00Z', end = '2026-09-06T13:00:00Z';
/** Padding on the wire is milliseconds; the DVR DTO keeps seconds client-side. */
const wireOptions = {beforeMs: 0, afterMs: 0, priority: 0, retentionDays: 0, episodeLimit: 0};
const recording = (id: string, patch: Record<string, unknown> = {}) => ({id, revision: 1,
  occurrence: {sourceId: hex(48), channelId: hex(64, 'b'), generation: hex(48, 'c'), programmeId: hex(64, 'd')},
  programme: {id: hex(64, 'd'), channelId: hex(64, 'b'), title: 'Airing', start: time, end, lineage: 'provider-id', seriesId: 'series', episodeId: 'episode', newEvidence: 'unknown'},
  options: {...wireOptions}, start: time, end, state: 'scheduled', reason: '', keep: false, itemId: '', libraryId: '',
  coverageStart: '', coverageEnd: '', bytes: 0, ruleId: '', conflicts: [], ...patch});
const pageOf = (recordings: unknown[], nextCursor = '', revision = 1) => ({recordings, rules: [], nextCursor, revision,
  captureAvailable: true, captureUnavailableReason: '', deletionAvailable: true, usage: {bytes: 0, pendingDeleteBytes: 0, recordings: recordings.length}});
const summaryFact = {upcoming: 3, recorded: 12, rules: 2, conflicts: 1, groups: 5, unwatched: 4};
const groupsFact = (order: string, nextCursor: string) => ({revision: 8, nextCursor, groups: [{key: `${order}-one`, title: 'Show', latest: end,
  recordings: 9, unwatched: 2, kept: 1, exceptions: 4, recording: recording(hex(64, '5'))}]});
const storageFact = () => ({policy: {revision: 4, retentionDays: 30, episodeLimit: 10, floorBytes: 1000, capBytes: 2000},
  measurement: {freeBytes: 300, usedBytes: 400, reservedBytes: 50, writeHealthy: true, measuredAt: time},
  pendingDeleteBytes: 5, forecastBytes: 600, forecastHours: 12, forecastDescription: 'Room for a day', captureAvailable: true, warning: ''});
const detailFact = (id: string) => ({recording: recording(id), capacity: {known: true, effective: 2}, overlaps: [], nextOverlap: ''});
const envelope = (result: unknown, serverId = 'server') => ({protocolVersion: '1.0', serverId, result});
const held = <T>() => {
  let resolve!: (value: T) => void, reject!: (failure: unknown) => void;
  const promise = new Promise<T>((done, failed) => { resolve = done; reject = failed; });
  return {promise, resolve, reject};
};
const url = (call: Call) => new URL(call.path, 'http://portico.invalid');
const params = (call: Call) => url(call).searchParams;
const pathname = (call: Call) => url(call).pathname;
const tick = () => new Promise<void>(done => setTimeout(done, 0));

/** The canonical envelope every DVR read has to receive, with a per-test reply. */
function fixtureServer(reply: Reply) {
  const calls: Call[] = [];
  const api: ChannelApi = {
    async request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> {
      const call = Object.freeze({path, method: method ?? 'GET', body, signal});
      calls.push(call);
      const value = await reply(call);
      return (value && typeof value === 'object' && 'protocolVersion' in value ? value : envelope(value)) as T;
    },
  };
  return {api, calls};
}
function controllerFor(api: ChannelApi, overrides: Partial<{requestId: () => Promise<string>; signal: AbortSignal; isCurrent: () => boolean}> = {}) {
  return createRecordingsController({api, serverId: 'server', requestId: async () => hex(48, 'e'),
    signal: new AbortController().signal, isCurrent: () => true, ...overrides});
}

test('construction is inert: no HTTP, no minted receipt and an empty snapshot', () => {
  let receipts = 0;
  const {api, calls} = fixtureServer(() => pageOf([]));
  const controller = createRecordingsController({api, serverId: 'server',
    requestId: async () => { receipts++; return hex(48, 'e'); }, signal: new AbortController().signal, isCurrent: () => true});
  assert.equal(calls.length, 0);
  assert.equal(receipts, 0);
  assert.deepEqual(controller.getSnapshot(), {query: {view: 'upcoming'}, phase: 'idle', page: null, detail: null, storage: null,
    summary: null, groups: null, groupsSort: 'recent', accessDenied: false, error: ''});
  controller.dispose();
  assert.equal(calls.length, 0);
  assert.equal(receipts, 0);
});

test('all four destinations read one canonical envelope with the existing DVR query', async () => {
  const upcoming = pageOf([recording(hex(64, '1'))], 'upcoming-next', 3);
  const recorded = pageOf([recording(hex(64, '2'))], '', 4);
  const history = pageOf([recording(hex(64, '3'))], '', 5);
  const detail = detailFact(hex(64, '4'));
  const {api, calls} = fixtureServer(call => {
    if (pathname(call) === '/v1/dvr/summary') return summaryFact;
    if (pathname(call) === '/v1/dvr/groups') return groupsFact('recent', '');
    if (pathname(call) === '/v1/dvr/storage') return storageFact();
    if (pathname(call).startsWith('/v1/dvr/recordings/')) return detail;
    const state = params(call).get('state');
    return state === 'upcoming' ? upcoming : state === 'recorded' ? recorded : history;
  });
  const controller = controllerFor(api);
  try {
    await controller.load({view: 'upcoming'});
    const first = params(calls.at(-1)!);
    assert.equal(first.get('state'), 'upcoming');
    assert.equal(first.get('limit'), '50');
    assert.equal(first.get('sort'), 'recent');
    assert.equal(first.has('cursor'), false);
    assert.deepEqual(controller.getSnapshot().page?.recordings.map(value => value.id), [hex(64, '1')]);
    assert.equal(controller.getSnapshot().page?.nextCursor, 'upcoming-next');
    assert.equal(controller.getSnapshot().phase, 'ready');

    await controller.load({view: 'recorded'});
    assert.equal(params(calls.at(-1)!).get('state'), 'recorded');
    assert.deepEqual(controller.getSnapshot().page?.recordings.map(value => value.id), [hex(64, '2')]);

    await controller.load({view: 'history'});
    assert.equal(params(calls.at(-1)!).get('state'), 'history');
    assert.deepEqual(controller.getSnapshot().page?.recordings.map(value => value.id), [hex(64, '3')]);

    await controller.load({view: 'storage'});
    assert.equal(pathname(calls.at(-1)!), '/v1/dvr/storage');
    assert.deepEqual(controller.getSnapshot().storage, storageFact());
    assert.equal(controller.getSnapshot().page, null);

    await controller.load({view: 'recorded', recordingId: hex(64, '4')});
    assert.equal(pathname(calls.at(-1)!), '/v1/dvr/recordings/' + hex(64, '4'));
    assert.equal(controller.getSnapshot().detail?.recording.id, hex(64, '4'));
    assert.equal(controller.contains(controller.getSnapshot().detail!.recording), true);
    assert.ok(calls.every(call => call.method === 'GET' && call.body === undefined), 'reads never mutate');
    assert.equal('act' in controller, false);
    assert.equal('mutating' in controller.getSnapshot(), false);
  } finally { controller.dispose(); }
});

test('an opaque cursor is carried forward exactly and the reply nextCursor is the only next token', async () => {
  const pages: Record<string, unknown> = {'': pageOf([recording(hex(64, '1'))], 'opaque-two', 1),
    'opaque-two': pageOf([recording(hex(64, '2'))], 'opaque-three', 2), 'opaque-three': pageOf([recording(hex(64, '3'))], '', 3)};
  const {api, calls} = fixtureServer(call => pages[params(call).get('cursor') ?? '']);
  const controller = controllerFor(api);
  try {
    await controller.load({view: 'recorded'});
    assert.equal(params(calls.at(-1)!).has('cursor'), false);
    assert.equal(controller.getSnapshot().page?.nextCursor, 'opaque-two');
    await controller.load({view: 'recorded', cursor: controller.getSnapshot().page!.nextCursor});
    assert.equal(params(calls.at(-1)!).get('cursor'), 'opaque-two');
    assert.equal(controller.getSnapshot().page?.revision, 2);
    await controller.load({view: 'recorded', cursor: 'opaque-three'});
    assert.equal(params(calls.at(-1)!).get('cursor'), 'opaque-three');
    assert.deepEqual(controller.getSnapshot().page?.recordings.map(value => value.id), [hex(64, '3')]);
    assert.equal(controller.getSnapshot().page?.nextCursor, '');
  } finally { controller.dispose(); }
});

test('genuine aggregates are the server counts, never the loaded page', async () => {
  const {api, calls} = fixtureServer(call => pathname(call) === '/v1/dvr/summary' ? summaryFact
    : pathname(call) === '/v1/dvr/groups' ? groupsFact(params(call).get('sort') ?? 'recent', 'group-next')
    : pageOf([recording(hex(64, '1'))], '', 1));
  const controller = controllerFor(api);
  try {
    await controller.load({view: 'recorded'});
    assert.equal(controller.getSnapshot().page?.recordings.length, 1);
    assert.equal(controller.getSnapshot().summary, null, 'a page is not a summary');
    assert.equal(controller.getSnapshot().groups, null);

    const summary = await controller.readSummary();
    assert.deepEqual(summary, summaryFact);
    assert.deepEqual(controller.getSnapshot().summary, summaryFact);
    assert.notEqual(controller.getSnapshot().summary?.recorded, controller.getSnapshot().page?.recordings.length);

    const groups = await controller.readGroups('title');
    assert.equal(params(calls.at(-1)!).get('sort'), 'title');
    assert.equal(params(calls.at(-1)!).has('cursor'), false);
    assert.equal(groups.groups[0].recordings, 9);
    assert.equal(groups.groups[0].unwatched, 2);
    assert.equal(groups.groups[0].kept, 1);
    assert.equal(groups.groups[0].exceptions, 4);
    assert.equal(groups.revision, 8);
    assert.equal(controller.getSnapshot().groupsSort, 'title');
    assert.equal(controller.getSnapshot().groups?.nextCursor, 'group-next');

    await controller.readGroups('title', 'group-next');
    assert.equal(params(calls.at(-1)!).get('cursor'), 'group-next');
    assert.equal(controller.getSnapshot().groups?.nextCursor, 'group-next');
  } finally { controller.dispose(); }
});

test('a replaced group order erases the old order and a late reply cannot restore it', async () => {
  const first = held<unknown>(), second = held<unknown>();
  let served = 0;
  const {api, calls} = fixtureServer(() => (served++ === 0 ? first.promise : second.promise));
  const controller = controllerFor(api);
  try {
    const recent = controller.readGroups('recent');
    const oldest = controller.readGroups('oldest');
    assert.equal(controller.getSnapshot().groups, null);
    second.resolve(groupsFact('oldest', 'oldest-next'));
    await oldest;
    assert.equal(controller.getSnapshot().groups?.nextCursor, 'oldest-next');
    assert.equal(controller.getSnapshot().groupsSort, 'oldest');
    first.resolve(groupsFact('recent', 'recent-next'));
    await assert.rejects(recent, failure => failure instanceof DOMException && failure.name === 'AbortError');
    assert.equal(controller.getSnapshot().groups?.nextCursor, 'oldest-next');
    assert.equal(controller.getSnapshot().groups?.groups[0].key, 'oldest-one');
    assert.equal(calls.length, 2);
    assert.equal(params(calls[0]).get('sort'), 'recent');
    assert.equal(params(calls[1]).get('sort'), 'oldest');
  } finally { controller.dispose(); }
});

test('a superseded view load settles without republishing its page', async () => {
  const upcoming = held<unknown>(), history = held<unknown>();
  let served = 0;
  const {api} = fixtureServer(() => (served++ === 0 ? upcoming.promise : history.promise));
  const controller = controllerFor(api);
  try {
    const first = controller.load({view: 'upcoming'});
    const second = controller.load({view: 'history'});
    history.resolve(pageOf([recording(hex(64, '9'))], '', 2));
    await second;
    upcoming.resolve(pageOf([recording(hex(64, '8'))], '', 1));
    await assert.rejects(first, failure => failure instanceof DOMException && failure.name === 'AbortError');
    const snapshot = controller.getSnapshot();
    assert.equal(snapshot.query.view, 'history');
    assert.equal(snapshot.page?.revision, 2);
    assert.deepEqual(snapshot.page?.recordings.map(value => value.id), [hex(64, '9')]);
  } finally { controller.dispose(); }
});

test('navigation and authority loss retire every fact and refuse further reads', async () => {
  let live = true;
  const {api, calls} = fixtureServer(call => pathname(call) === '/v1/dvr/summary' ? summaryFact
    : pathname(call) === '/v1/dvr/groups' ? groupsFact('recent', '') : pageOf([recording(hex(64, '1'))], 'next', 1));
  const controller = controllerFor(api, {isCurrent: () => live});
  try {
    await controller.load({view: 'recorded'});
    await controller.readSummary();
    await controller.readGroups();
    let noticed = 0;
    controller.subscribe(() => { noticed++; });
    live = false;
    const snapshot = controller.getSnapshot();
    assert.equal(snapshot.phase, 'retired');
    assert.equal(snapshot.page, null);
    assert.equal(snapshot.detail, null);
    assert.equal(snapshot.storage, null);
    assert.equal(snapshot.summary, null);
    assert.equal(snapshot.groups, null);
    assert.ok(noticed <= 1);
    const before = calls.length;
    await assert.rejects(controller.load({view: 'history'}), failure => failure instanceof DOMException && failure.name === 'AbortError');
    await assert.rejects(controller.readSummary(), failure => failure instanceof DOMException && failure.name === 'AbortError');
    await assert.rejects(controller.requestReceipt(), failure => failure instanceof DOMException && failure.name === 'AbortError');
    assert.equal(calls.length, before, 'a retired viewer starts no request');
    assert.equal(controller.contains(recording(hex(64, '1'))), false);
  } finally { controller.dispose(); }
});

test('dispose aborts the in-flight read and erases every fact', async () => {
  let active: AbortSignal | undefined;
  const {api} = fixtureServer(call => {
    active = call.signal;
    return new Promise<unknown>((_done, failed) => {
      const stop = () => failed(new DOMException('aborted', 'AbortError'));
      if (call.signal?.aborted) stop();else call.signal?.addEventListener('abort', stop, {once: true});
    });
  });
  const controller = controllerFor(api);
  const pending = controller.load({view: 'recorded'});
  await tick();
  assert.equal(active?.aborted, false);
  controller.dispose();
  assert.equal(active?.aborted, true, 'disposal aborts the request signal');
  await assert.rejects(pending, failure => failure instanceof DOMException && failure.name === 'AbortError');
  await tick();
  const snapshot = controller.getSnapshot();
  assert.equal(snapshot.phase, 'retired');
  assert.equal(snapshot.page, null);
  assert.equal(snapshot.detail, null);
  assert.equal(snapshot.storage, null);
  assert.equal(snapshot.summary, null);
  assert.equal(snapshot.groups, null);
  assert.equal(controller.subscribe(() => {}) instanceof Function, true);
});

test('a page 403 clears page, detail, storage and both aggregates', async () => {
  let deny = false;
  const {api} = fixtureServer(call => {
    if (deny) throw {status: 403};
    if (pathname(call) === '/v1/dvr/summary') return summaryFact;
    if (pathname(call) === '/v1/dvr/groups') return groupsFact('recent', '');
    return pageOf([recording(hex(64, '1'))], '', 1);
  });
  const controller = controllerFor(api);
  try {
    await controller.load({view: 'recorded'});
    await controller.readSummary();
    await controller.readGroups();
    deny = true;
    await controller.load({view: 'history'});
    const snapshot = controller.getSnapshot();
    assert.equal(snapshot.accessDenied, true);
    assert.equal(snapshot.page, null);
    assert.equal(snapshot.detail, null);
    assert.equal(snapshot.storage, null);
    assert.equal(snapshot.summary, null);
    assert.equal(snapshot.groups, null);
    assert.equal(controller.contains(recording(hex(64, '1'))), false);

    deny = false;
    await controller.load({view: 'recorded'});
    assert.equal(controller.getSnapshot().accessDenied, false);
    assert.equal(controller.getSnapshot().page?.recordings.length, 1);
  } finally { controller.dispose(); }
});

test('a summary 401 clears every fact and fences the aggregate read', async () => {
  let deny = false;
  const {api} = fixtureServer(call => {
    if (deny && pathname(call) === '/v1/dvr/summary') throw {status: 401};
    if (pathname(call) === '/v1/dvr/summary') return summaryFact;
    if (pathname(call) === '/v1/dvr/groups') return groupsFact('recent', '');
    return pageOf([recording(hex(64, '1'))], '', 1);
  });
  const controller = controllerFor(api);
  try {
    await controller.load({view: 'recorded'});
    await controller.readSummary();
    await controller.readGroups();
    deny = true;
    await assert.rejects(controller.readSummary(), failure => failure !== null && typeof failure === 'object' && (failure as {status?: number}).status === 401);
    const snapshot = controller.getSnapshot();
    assert.equal(snapshot.accessDenied, true);
    assert.equal(snapshot.page, null);
    assert.equal(snapshot.detail, null);
    assert.equal(snapshot.storage, null);
    assert.equal(snapshot.summary, null);
    assert.equal(snapshot.groups, null);
  } finally { controller.dispose(); }
});

test('the receipt is the genuine 48-hex DVR operation id, fenced before and after the await', async () => {
  let minted = 0;
  const {api, calls} = fixtureServer(() => pageOf([]));
  const controller = controllerFor(api, {requestId: async () => { minted++; return hex(48, 'f'); }});
  try {
    assert.equal(await controller.requestReceipt(), hex(48, 'f'));
    assert.equal(minted, 1);
    assert.equal(calls.length, 0);
  } finally { controller.dispose(); }

  const uuid = controllerFor(fixtureServer(() => pageOf([])).api, {requestId: async () => '123e4567-e89b-12d3-a456-426614174000'});
  await assert.rejects(uuid.requestReceipt(), /recordings-query-invalid/);
  uuid.dispose();

  let live = true;
  const pendingId = held<string>();
  const fenced = controllerFor(fixtureServer(() => pageOf([])).api, {requestId: () => pendingId.promise, isCurrent: () => live});
  const receipt = fenced.requestReceipt();
  live = false;
  pendingId.resolve(hex(48, 'a'));
  await assert.rejects(receipt, failure => failure instanceof DOMException && failure.name === 'AbortError');
  fenced.dispose();
});

test('undrawn destinations, cursors and group orders perform no request', async () => {
  const {api, calls} = fixtureServer(() => pageOf([]));
  const controller = controllerFor(api);
  try {
    await assert.rejects(controller.load({view: 'rules' as never}), /recordings-query-invalid/);
    await assert.rejects(controller.load({view: 'recorded', recordingId: 'recording-one'}), /recordings-query-invalid/);
    await assert.rejects(controller.load({view: 'recorded', cursor: 'c'.repeat(4097)}), /recordings-query-invalid/);
    await assert.rejects(controller.readGroups('newest' as never), /recordings-query-invalid/);
    await assert.rejects(controller.readGroups('recent', 'c'.repeat(4097)), /recordings-query-invalid/);
    assert.equal(calls.length, 0);
    assert.equal(controller.getSnapshot().phase, 'idle');
  } finally { controller.dispose(); }
});

test('a foreign envelope is refused and leaves no private facts', async () => {
  const {api, calls} = fixtureServer(() => envelope(pageOf([recording(hex(64, '1'))]), 'another-server'));
  const controller = controllerFor(api);
  try {
    await controller.load({view: 'recorded'});
    const snapshot = controller.getSnapshot();
    assert.equal(calls.length, 1);
    assert.equal(snapshot.page, null);
    assert.equal(snapshot.detail, null);
    assert.equal(snapshot.storage, null);
    assert.equal(snapshot.phase, 'error');
    assert.ok(snapshot.error.length > 0);
    assert.ok(!snapshot.error.includes('another-server'));
  } finally { controller.dispose(); }
});

test('contains accepts only the exact resident objects of this generation', async () => {
  const resident = recording(hex(64, '1')), copy = {...resident};
  const {api} = fixtureServer(call => pathname(call).startsWith('/v1/dvr/recordings/') ? detailFact(hex(64, '2')) : pageOf([resident], '', 1));
  const controller = controllerFor(api);
  try {
    await controller.load({view: 'recorded'});
    const row = controller.getSnapshot().page!.recordings[0];
    assert.equal(row.id, resident.id);
    assert.equal(controller.contains(row), true);
    assert.equal(controller.contains(copy), false);
    await controller.load({view: 'recorded', recordingId: hex(64, '2')});
    assert.equal(controller.contains(row), false, 'a different destination is not resident');
    assert.equal(controller.contains(controller.getSnapshot().detail!.recording), true);
  } finally { controller.dispose(); }
});
