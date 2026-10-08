/**
 * PERF-S15: console lists (trash, backups, members, invitations, devices, API keys) and the DVR
 * read one server page at a time by cursor, and every page is reachable with Next and Previous.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';

test('Next follows the server cursor to the last page; Previous goes back without re-walking', async () => {
  const h = hooks();
  const requested: string[] = [];
  const pages = 26; // e.g. 2,600 held trash entries at 100 a page
  // useRead as a synchronous stand-in: the page for the cursor it is given.
  const useRead = (fn: () => {items: number[]; nextCursor: string}) => ({data: fn(), loading: false, reload: () => {}});
  const mod = await componentModule(new URL('../src/admin/cursor-read.ts', import.meta.url), {react: h.react, './console': {useRead}});
  const fetch = (cursor: string) => { requested.push(cursor); const n = cursor ? Number(cursor.slice(1)) : 0; return {items: [n], nextCursor: n + 1 < pages ? `p${n + 1}` : ''}; };
  let read = h.render(() => mod.useCursorRead(fetch, []));
  assert.equal(read.page, 1);
  assert.equal(read.canPrevious, false);
  while (read.canNext) { read.next(); read = h.render(() => mod.useCursorRead(fetch, [])); }
  assert.deepEqual(read.data.items, [pages - 1], 'the last page is reachable');
  assert.equal(read.page, pages);
  const walked = requested.length;
  read.previous(); read = h.render(() => mod.useCursorRead(fetch, []));
  assert.deepEqual(read.data.items, [pages - 2]);
  assert.equal(requested.length, walked + 1, 'Previous is one read of the remembered cursor');
  assert.equal(requested.at(-1), `p${pages - 2}`);
});

test('a list page keeps its items as data and its cursor for Next; a malformed cursor ends the list', async () => {
  const h = hooks();
  const useRead = (fn: () => unknown) => ({data: fn(), loading: false, reload: () => {}});
  const mod = await componentModule(new URL('../src/admin/cursor-read.ts', import.meta.url), {react: h.react, './console': {useRead}});
  const list = h.render(() => mod.useCursorList(() => ({items: ['a', 'b'], nextCursor: 'next'}), []));
  assert.deepEqual(list.data, ['a', 'b']);
  assert.equal(list.canNext, true);
  assert.equal(mod.nextCursorOf({nextCursor: 42}), '');
  assert.equal(mod.nextCursorOf({nextCursor: 'x'.repeat(5000)}), '');
  assert.equal(mod.withCursor('/v1/admin/trash?limit=100', 'a b'), '/v1/admin/trash?limit=100&cursor=a%20b');
  assert.equal(mod.withCursor('/v1/admin/backups', ''), '/v1/admin/backups');
});
