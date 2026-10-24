import test from 'node:test';
import assert from 'node:assert/strict';
import {routePath} from '../../../../packages/contracts/src/routes.ts';
import {SearchOnNowController} from './on-now.ts';

const serverId = 'server';
const source = {id: 'src1', name: 'Live source', generation: 'g1', publishedAt: '2026-10-24T12:00:00Z',
  availableStart: '', availableEnd: '', provenance: 'live-source', refreshState: 'ready'};
const channel = (id, mark) => ({id, sourceId: 'src1', provenance: 'live-source',
  name: id === 'ch1' ? 'Northwind News' : 'Summit Sports', number: id === 'ch1' ? '101' : '201', group: 'News', generation: 'g1',
  preferenceRevision: 0, programmes: [], tuneAvailable: true, recordAvailable: true, favorite: id === 'ch1', hidden: false,
  ...(mark ? {mark} : {})});
const directory = (channels, limit = 4) => ({protocolVersion: '1.0', serverId,
  directory: {viewerFence: 'fence', revision: 'directory-revision', total: channels.length, offset: 0, limit, sources: [source], channels}});
const guide = (ids, start, end, timezone) => ({protocolVersion: '1.0', serverId,
  guide: {state: 'ready', viewerFence: 'fence', revision: 'guide-revision', start, end, timezone, observedAt: start, nextCursor: '',
    sources: [source], channels: ids.map(id => ({...channel(id), programmes: [{id: 'programme-' + id, channelId: id, title: 'World Tonight', start, end, subtitle: 'Homecoming', flags: {live: true}}]}))}});

function setup(handler, initialZone = 'America/Halifax') {
  let current = true, requests = [], zone = initialZone ?? undefined;
  const parent = new AbortController();
  const controller = new SearchOnNowController({api: {request: async (path, method, body, signal) => {
    assert.equal(method, 'GET');assert.equal(body, undefined);requests.push(path);
    return await handler(path, signal);
  }}, serverId, timeZone: () => zone ?? undefined, isCurrent: () => current, signal: parent.signal, limit: 4});
  return {controller, parent, requests: () => requests, retire: () => {current = false;}, setZone: value => {zone = value;}};
}

test('an empty permitted directory leaves On now absent with one favourite-ordered read', async () => {
  const owner = setup(async () => directory([]));
  try {
    assert.equal(owner.controller.getSnapshot().phase, 'idle');
    await owner.controller.load();
    const state = owner.controller.getSnapshot();
    assert.equal(state.phase, 'ready');assert.deepEqual(state.cards, []);assert.deepEqual(state.sourceNames, []);
    assert.equal(owner.requests().length, 1);
    const query = new URL(owner.requests()[0], 'https://local.invalid').searchParams;
    assert.equal(query.get('sort'), 'favorites');assert.equal(query.get('limit'), '4');assert.equal(query.get('kind'), 'all');
  } finally {owner.controller.dispose();}
});

test('the actual directory order and its exact programmes become the drawn cards', async () => {
  const owner = setup(async path => {
    const url = new URL(path, 'https://local.invalid'), query = url.searchParams;
    if (url.pathname === routePath('get_guide_channels')) return directory([channel('ch1', {text: 'NN', seed: 'src1:ch1'}), channel('ch2')]);
    if (url.pathname === routePath('get_guide_sources')) return {protocolVersion: '1.0', serverId, viewerFence: 'fence', revision: 'sources-revision',
      sources: [{...source, position: 0, recordAvailable: false, guideDays: 7, channelCount: 2, favoriteCount: 1, groups: [], groupCounts: [], unavailableReasons: {}}]};
    assert.equal(url.pathname, routePath('get_guide'));
    return guide((query.get('channels') ?? '').split(','), query.get('start'), query.get('end'), query.get('timezone'));
  });
  try {
    await owner.controller.load();
    const state = owner.controller.getSnapshot();
    assert.equal(state.phase, 'ready');
    assert.deepEqual(state.cards.map(card => card.channel.id), ['ch1', 'ch2']);
    assert.deepEqual(state.sourceNames, ['Live source']);
    assert.equal(state.cards[0].channel.mark?.text, 'NN');
    assert.equal(state.cards[0].programmes[0].title, 'World Tonight');
    assert.equal(state.cards[0].programmes[0].subtitle, 'Homecoming');
    assert.equal(owner.requests().length, 3);
    const guideRead = owner.requests().map(path => new URL(path, 'https://local.invalid')).find(url => url.pathname === routePath('get_guide'));
    assert.equal(guideRead.searchParams.get('channels'), 'ch1,ch2');assert.equal(guideRead.searchParams.get('timezone'), 'America/Halifax');
  } finally {owner.controller.dispose();}
});

test('a refused Live TV read hides the section without exposing another source', async () => {
  const owner = setup(async () => {throw Object.assign(new Error('not permitted'), {status: 403});});
  try {
    await assert.rejects(owner.controller.load(), {message: 'not permitted'});
    const state = owner.controller.getSnapshot();
    assert.equal(state.phase, 'hidden');assert.deepEqual(state.cards, []);
    assert.equal(owner.requests().length, 1);
  } finally {owner.controller.dispose();}
});

test('retiring the captured authority refuses a late guide answer and clears private cards', async () => {
  let resolve;
  const pending = new Promise(done => {resolve = done;});
  const owner = setup(async () => pending);
  try {
    const loading = owner.controller.load();
    owner.retire();
    resolve(directory([]));
    await assert.rejects(loading, {name: 'AbortError'});
    assert.deepEqual(owner.controller.getSnapshot().cards, []);
    assert.equal(owner.controller.getSnapshot().phase, 'idle');
  } finally {owner.controller.dispose();}
});

test('an authority failure on the source names erases a held programme read and never publishes it late', async () => {
  let resolveGuide;
  const held = new Promise(done => {resolveGuide = done;});
  const owner = setup(async path => {
    const url = new URL(path, 'https://local.invalid');
    if (url.pathname === routePath('get_guide_channels')) return directory([channel('ch1')]);
    if (url.pathname === routePath('get_guide')) return held;
    if (url.pathname === routePath('get_guide_sources')) throw Object.assign(new Error('access changed'), {status: 401});
    throw new Error('unexpected ' + url.pathname);
  });
  try {
    await assert.rejects(owner.controller.load(), {message: 'access changed'});
    let state = owner.controller.getSnapshot();
    assert.equal(state.phase, 'hidden');assert.deepEqual(state.cards, []);assert.deepEqual(state.sourceNames, []);
    resolveGuide(guide(['ch1'], new Date().toISOString(), new Date(Date.now() + 3600000).toISOString(), 'America/Halifax'));
    await new Promise(done => setImmediate(done));await new Promise(done => setImmediate(done));
    state = owner.controller.getSnapshot();
    assert.equal(state.phase, 'hidden', 'a refused authority never resurfaces held channels');
    assert.deepEqual(state.cards, []);
  } finally {owner.controller.dispose();}
});

test('a channel row without its actual source never becomes a card with an empty identity', async () => {
  const owner = setup(async path => {
    const url = new URL(path, 'https://local.invalid'), query = url.searchParams;
    if (url.pathname === routePath('get_guide_channels')) return directory([channel('ch1'), {...channel('ch2'), sourceId: ''}]);
    if (url.pathname === routePath('get_guide_sources')) return {protocolVersion: '1.0', serverId, viewerFence: 'fence', revision: 'sources-revision',
      sources: [{...source, position: 0, recordAvailable: false, guideDays: 7, channelCount: 1, favoriteCount: 1, groups: [], groupCounts: [], unavailableReasons: {}}]};
    return guide((query.get('channels') ?? '').split(','), query.get('start'), query.get('end'), query.get('timezone'));
  });
  try {
    await owner.controller.load();
    const state = owner.controller.getSnapshot();
    assert.deepEqual(state.cards.map(card => card.channel.id), ['ch1']);
    assert(state.cards.every(card => card.channel.sourceId === 'src1'), 'no card carries a fabricated empty source');
  } finally {owner.controller.dispose();}
});

test('no confirmed time zone refuses the window instead of reading a guessed one', async () => {
  const owner = setup(async () => {throw new Error('must not read');}, null);
  try {
    await owner.controller.load();
    const state = owner.controller.getSnapshot();
    assert.equal(state.phase, 'hidden');assert.deepEqual(state.cards, []);
    assert.equal(owner.requests().length, 0);
  } finally {owner.controller.dispose();}
});

test('each window uses the current confirmed zone rather than one captured at construction', async () => {
  const zones = [];
  const owner = setup(async path => {
    const url = new URL(path, 'https://local.invalid'), query = url.searchParams;
    if (url.pathname === routePath('get_guide_channels')) return directory([channel('ch1')]);
    if (url.pathname === routePath('get_guide_sources')) return {protocolVersion: '1.0', serverId, viewerFence: 'fence', revision: 'sources-revision',
      sources: [{...source, position: 0, recordAvailable: false, guideDays: 7, channelCount: 1, favoriteCount: 1, groups: [], groupCounts: [], unavailableReasons: {}}]};
    zones.push(query.get('timezone'));
    return guide((query.get('channels') ?? '').split(','), query.get('start'), query.get('end'), query.get('timezone'));
  });
  try {
    await owner.controller.load();
    owner.setZone('Europe/Berlin');
    await owner.controller.load();
    assert.deepEqual(zones, ['America/Halifax', 'Europe/Berlin']);
  } finally {owner.controller.dispose();}
});
