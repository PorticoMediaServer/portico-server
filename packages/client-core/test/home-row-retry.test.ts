import assert from 'node:assert/strict';
import test from 'node:test';
import {HomeService, homeRowRetryDelaysMs} from '../src/home-service.ts';
import type {HomeRow} from '../src/home.ts';

const entry = (id: string) => ({id, libraryId: 'movies', kind: 'movie' as const, title: id, available: true});
const row = (id: string, entries: ReturnType<typeof entry>[], deferred = false): HomeRow => ({id, title: id, kind: 'recent', artworkShape: 'poster', entries, start: 0,
  viewerFence: 'fence', endpoint: '', privacySensitivity: '', policyState: '', priority: 1, required: false, hideable: true, reorderable: true,
  critical: false, cursorCapable: true, total: entries.length, limit: 2, hasMore: false, revision: {catalog: 1, viewer: 1}, ...(deferred ? {deferred: true} : {})});
const document = (rows: HomeRow[]) => ({serverId: 'server', viewerFence: 'fence', rows, spotlight: [], layout: {revision: 1, rowOrder: rows.map(value => value.id), hiddenRowIds: []},
  revision: {catalog: 1, viewer: 1}, generatedAt: '2026-10-22T20:00:00Z'});
const settle = async () => {for (let index = 0; index < 20; index++) await Promise.resolve();};

test('a deferred shelf whose first read fails is retried, so it never stays a skeleton (E-tv-02, E-mobile-08)', async t => {
  t.mock.timers.enable({apis: ['setTimeout']});
  let reads = 0;
  const home = new HomeService({request: async path => {
    if (!path.includes('/rows/')) return JSON.parse(JSON.stringify(document([row('later', [], true)])));
    if (++reads === 1) throw Object.assign(new Error('slow'), {code: 'request_timeout', retryable: true});
    return JSON.parse(JSON.stringify(row('later', [entry('a')])));
  }});
  await home.load(); await settle();
  assert.equal(home.getSnapshot().document!.rows[0].entries.length, 0, 'first read failed; shelf still a skeleton');
  t.mock.timers.tick(homeRowRetryDelaysMs[0]); await settle();
  assert.equal(reads, 2);
  assert.deepEqual(home.getSnapshot().document!.rows[0].entries.map(item => item.id), ['a']);
  home.dispose();
});

test('a shelf that keeps failing is dropped after the last retry instead of staying a skeleton forever', async t => {
  t.mock.timers.enable({apis: ['setTimeout']});
  let reads = 0;
  const home = new HomeService({request: async path => {
    if (!path.includes('/rows/')) return JSON.parse(JSON.stringify(document([row('later', [], true)])));
    reads++; throw Object.assign(new Error('down'), {code: 'request_timeout', retryable: true});
  }});
  await home.load(); await settle();
  for (const delay of homeRowRetryDelaysMs) {t.mock.timers.tick(delay); await settle();}
  assert.equal(reads, homeRowRetryDelaysMs.length + 1);
  assert.equal(home.getSnapshot().document!.rows.length, 0);
  home.dispose();
});

test('a deferred shelf outside the declared demand waits, and loads once it is demanded', async t => {
  t.mock.timers.enable({apis: ['setTimeout']});
  let reads = 0;
  const home = new HomeService({request: async path => {
    if (!path.includes('/rows/')) return JSON.parse(JSON.stringify(document([row('later', [], true)])));
    reads++; return JSON.parse(JSON.stringify(row('later', [entry('a')])));
  }});
  home.setRowDemand([]);
  await home.load(); await settle();
  assert.equal(reads, 0);
  home.setRowDemand(['later']); await settle();
  assert.equal(reads, 1);
  home.dispose();
});
