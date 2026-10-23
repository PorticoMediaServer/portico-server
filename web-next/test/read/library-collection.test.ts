import assert from 'node:assert/strict';
import {test} from 'node:test';
import {HttpLocalApi} from '@portico/client-core';
import {createReadingCollection} from '../../src/screens/read/library-collection.ts';
import {readItemEligibility} from '../../src/screens/read/navigation.ts';

const tick = () => new Promise<void>(resolve => setImmediate(resolve));
async function settle(done: () => boolean) {for (let i = 0; i < 100 && !done(); i++) await tick(); assert.ok(done(), 'bounded unit request settled');}
function fixture(total = 480, wrongLibrary = false) {
  const calls: URL[] = [];
  const api = new HttpLocalApi('http://127.0.0.1:39117', 'unit-token', async input => {
    const url = new URL(String(input)); calls.push(url);
    if (url.pathname.endsWith('/reading/versions')) return url.pathname.includes('/unit-reading/')
      ? new Response(JSON.stringify({versions: [{sourceId: 'unit-source', format: 'epub', size: 500, inventoryAvailable: false}]}), {headers: {'Content-Type': 'application/json'}})
      : new Response('{}', {status: 404, headers: {'Content-Type': 'application/json'}});
    const start = Number(url.searchParams.get('cursor') ?? 0), limit = Number(url.searchParams.get('limit'));
    const items = Array.from({length: Math.max(0, Math.min(limit, total - start))}, (_, offset) => ({id: `unit-${start + offset}`, sourceId: `source-${start + offset}`,
      libraryId: wrongLibrary ? 'another-library' : 'unit-library', title: `Unit title ${start + offset}`, format: 'epub', inventoryAvailable: true}));
    return new Response(JSON.stringify({items, ...(start + items.length < total ? {nextCursor: String(start + items.length)} : {})}), {headers: {'Content-Type': 'application/json'}});
  });
  const client = api.reading({viewer: {accountId: 'unit-account', profileId: 'unit-profile', serverId: 'unit-server', authority: 'local', role: 'member'}, isCurrent: () => true});
  const rows = createReadingCollection(client, 'unit-library');
  return {client, rows, calls, dispose() {rows.dispose(); client.dispose();}};
}

test('actual cursor pages feed the existing sparse collection without fabricated totals or full-library work', async t => {
  const f = fixture(); t.after(() => f.dispose()); assert.equal(f.calls.length, 0);
  const collection = f.rows.collection;
  collection.ensureRange(0, 12); await settle(() => collection.itemAt(0) !== undefined);
  assert.equal(collection.getSnapshot().totalIsLowerBound, true);
  assert.ok(f.calls.length <= 3); assert.equal(f.calls[0].searchParams.get('libraryId'), 'unit-library');
  for (let first = 60; first < 480; first += 60) {
    collection.ensureRange(first, first + 12); await settle(() => collection.itemAt(first) !== undefined);
    assert.ok(collection.resident().pages <= 6); assert.ok(collection.resident().items <= 360);
  }
  await settle(() => collection.getSnapshot().totalIsLowerBound === false);
  assert.equal(collection.getSnapshot().total, 480); assert.equal(collection.itemAt(479)?.id, 'unit-479');
  assert.ok(f.calls.length <= 10);
});

test('an unexpected library row never becomes a visible or selectable Read item', async t => {
  const f = fixture(1, true); t.after(() => f.dispose());
  f.rows.collection.ensureRange(0, 1); await settle(() => f.rows.collection.getSnapshot().status === 'error');
  assert.equal(f.rows.collection.itemAt(0), undefined); assert.equal(f.rows.collection.resident().items, 0);
});

test('only actual versions classify Read; unavailable real versions remain Read and ordinary item 404 stays ordinary', async t => {
  const f = fixture(); t.after(() => f.dispose());
  const reading = await readItemEligibility(f.client, 'unit-reading');
  assert.equal(reading?.itemId, 'unit-reading'); assert.equal(reading?.versions[0].inventoryAvailable, false);
  assert.equal(await readItemEligibility(f.client, 'unit-movie'), null);
});
