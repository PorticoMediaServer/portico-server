import assert from 'node:assert/strict';
import test from 'node:test';
import {emptyMissingTitles, listMissingTitles, mergeTitles, parseMissingEmptied, parseMissingTitlesPage, removeLibrary} from '../src/title-identity.ts';
import {formValue, withFormValue} from '../src/server-admin/server-forms.ts';

const title = {id: 'item-1', libraryId: 'films', kind: 'movie', title: 'Ran', year: 1985, missingSince: '2026-10-01T10:00:00Z', removedAt: '2026-10-31T10:00:00Z'};

test('a missing titles page carries the rule and when each title goes', () => {
  const page = parseMissingTitlesPage({items: [title, {...title, id: 'item-2', kind: 'episode', title: 'Episode 3', parentTitle: 'Kingdom', removedAt: undefined, year: undefined}], nextCursor: '1.2', total: 12, rule: 'after_days', days: 30});
  assert.equal(page.items.length, 2);
  assert.equal(page.items[0]!.removedAt, '2026-10-31T10:00:00Z');
  assert.equal(page.items[1]!.parentTitle, 'Kingdom');
  assert.equal(page.items[1]!.removedAt, undefined);
  assert.equal(page.nextCursor, '1.2');
  assert.equal(page.rule, 'after_days');
  assert.throws(() => parseMissingTitlesPage({items: [title], total: 1, rule: 'soon', days: 30}));
  assert.throws(() => parseMissingTitlesPage({items: [{...title, missingSince: ''}], total: 1, rule: 'never', days: 30}));
  assert.throws(() => parseMissingTitlesPage({items: [{...title, id: ''}], total: 1, rule: 'never', days: 30}));
});

test('listing sends only the filters that were given', async () => {
  const paths: string[] = [];
  const api = {request: async <T>(path: string): Promise<T> => { paths.push(path); return {items: [], total: 0, rule: 'never', days: 30} as T; }};
  await listMissingTitles(api);
  await listMissingTitles(api, {libraryId: 'films', cursor: '5.9', limit: 100});
  assert.deepEqual(paths, ['/v1/admin/missing-titles', '/v1/admin/missing-titles?libraryId=films&cursor=5.9&limit=100']);
});

test('emptying repeats while the server says there is more and adds the totals up', async () => {
  const answers = [{titles: 24, files: 3, more: true}, {titles: 24, files: 0, more: true}, {titles: 2, files: 0, more: false}];
  const bodies: unknown[] = [];
  const api = {request: async <T>(_path: string, _method?: string, body?: unknown): Promise<T> => { bodies.push(body); return answers.shift() as T; }};
  const out = await emptyMissingTitles(api, 'films');
  assert.deepEqual(out, {titles: 50, files: 3, more: false});
  assert.deepEqual(bodies, [{libraryId: 'films'}, {libraryId: 'films'}, {libraryId: 'films'}]);
  assert.throws(() => parseMissingEmptied({titles: 1, files: 0}));
});

test('merging and removing a library send what the owner confirmed', async () => {
  const calls: unknown[] = [];
  const api = {request: async <T>(path: string, method?: string, body?: unknown): Promise<T> => { calls.push([path, method, body]); return (path.endsWith('/merge') ? {itemId: 'b'} : {libraryId: 'films', removing: true}) as T; }};
  assert.equal(await mergeTitles(api, 'a', 'b'), 'b');
  await removeLibrary(api, 'films', 'Films', 7);
  assert.deepEqual(calls, [['/v1/admin/titles/merge', 'POST', {fromId: 'a', intoId: 'b'}], ['/v1/admin/libraries/films', 'DELETE', {confirmation: 'Films', expectedRevision: 7}]]);
  await assert.rejects(mergeTitles(api, 'a', 'a'));
});

test('a setting whose own name has a dot is one key, not a path', () => {
  const draft = {name: 'Portico', 'library.missingTitles': 'after_days', nested: {inner: 1}};
  assert.equal(formValue(draft, 'library.missingTitles'), 'after_days');
  assert.equal(formValue(draft, 'nested.inner'), 1);
  assert.deepEqual(withFormValue(draft, 'library.missingTitles', 'never'), {...draft, 'library.missingTitles': 'never'});
  assert.deepEqual(withFormValue(draft, 'nested.inner', 2), {...draft, nested: {inner: 2}});
});
