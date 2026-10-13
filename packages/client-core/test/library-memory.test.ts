import test from 'node:test';
import assert from 'node:assert/strict';
import {LibraryMemory, MAX_REMEMBERED_LIBRARIES, libraryMemoryKey, libraryOpening, parseLibraryMemory, rememberedTab, type LibraryMemoryStorage} from '../src/library-memory.ts';

function storage(initial: Record<string, string> = {}): LibraryMemoryStorage & {data: Map<string, string>; writes: number} {
  const data = new Map(Object.entries(initial));
  const box = {
    data, writes: 0,
    read: (key: string) => data.get(key),
    write: (key: string, text: string) => { box.writes++; data.set(key, text); },
    remove: (key: string) => { box.writes++; data.delete(key); },
  };
  return box;
}
const scope = {serverId: 'server', viewerId: 'ana'};

test('a library reopens on the tab, sort and filters the person last used', () => {
  const box = storage();
  const memory = new LibraryMemory(box, scope);
  memory.rememberTab('movies', 'browse');
  memory.rememberSort('movies', 'browse', {field: 'year', direction: 'desc'});
  memory.rememberFilters('movies', 'browse', [{field: 'genre', operator: 'in', value: ['Drama']}]);

  // A later visit, in a new run of the app.
  const later = new LibraryMemory(box, scope);
  assert.equal(later.isReady, true);
  const opening = libraryOpening({}, later.recall('movies'), ['discover', 'browse', 'collections']);
  assert.deepEqual(opening, {tab: 'browse', sort: {field: 'year', direction: 'desc'}, filters: [{field: 'genre', operator: 'in', value: ['Drama']}], remembered: true});
});

test('an address that names a tab, a sort, filters or another view is followed and memory is not consulted', () => {
  const memory = new LibraryMemory(storage(), scope);
  memory.rememberTab('movies', 'collections');
  const remembered = memory.recall('movies');
  assert.deepEqual(libraryOpening({tab: 'browse'}, remembered, ['discover', 'browse', 'collections']), {tab: 'browse', sort: undefined, filters: undefined, remembered: false});
  assert.equal(libraryOpening({sort: {field: 'added', direction: 'desc'}}, remembered, ['discover', 'browse', 'collections']).remembered, false);
  assert.equal(libraryOpening({other: true}, remembered, ['discover', 'browse', 'collections']).remembered, false);
});

test('a remembered tab the library no longer has is ignored', () => {
  const memory = new LibraryMemory(storage(), scope);
  memory.rememberTab('music', 'unmatched');
  assert.deepEqual(libraryOpening({}, memory.recall('music'), ['discover', 'browse']), {remembered: false});
});

test('each tab keeps its own sort and filters, and clearing filters is remembered', () => {
  const memory = new LibraryMemory(storage(), scope);
  memory.rememberSort('music', 'releases', {field: 'year', direction: 'asc'});
  memory.rememberFilters('music', 'releases', [{field: 'genre', operator: 'in', value: ['Jazz']}]);
  memory.rememberSort('music', 'songs', {field: 'plays', direction: 'desc'});
  assert.deepEqual(rememberedTab(memory.recall('music'), 'releases'), {sort: {field: 'year', direction: 'asc'}, filters: [{field: 'genre', operator: 'in', value: ['Jazz']}]});
  assert.deepEqual(rememberedTab(memory.recall('music'), 'songs'), {sort: {field: 'plays', direction: 'desc'}});
  memory.rememberFilters('music', 'releases', []);
  assert.deepEqual(rememberedTab(memory.recall('music'), 'releases'), {sort: {field: 'year', direction: 'asc'}});
  memory.rememberSort('music', 'releases', undefined);
  assert.equal(rememberedTab(memory.recall('music'), 'releases'), undefined);
});

test('each profile has its own memory on the device', () => {
  const box = storage();
  new LibraryMemory(box, scope).rememberTab('movies', 'browse');
  assert.equal(new LibraryMemory(box, {serverId: 'server', viewerId: 'ben'}).recall('movies'), undefined);
  assert.equal(new LibraryMemory(box, {serverId: 'other', viewerId: 'ana'}).recall('movies'), undefined);
  assert.notEqual(libraryMemoryKey(scope), libraryMemoryKey({serverId: 'server', viewerId: 'ben'}));
  assert.notEqual(libraryMemoryKey({serverId: 'a:b', viewerId: 'c'}), libraryMemoryKey({serverId: 'a', viewerId: 'b:c'}));
});

test('damaged saved text loses only the part that cannot be read', () => {
  const text = JSON.stringify({version: 1, libraries: [
    {id: 'movies', tab: 'browse', tabs: {browse: {sort: {field: 'year', direction: 'sideways'}, filters: [{field: 'genre', operator: 'in', value: ['Drama']}, {field: 7}]}}},
    {id: '', tab: 'browse', tabs: {}},
    'nonsense',
    {id: 'tv', tab: 'x'.repeat(200), tabs: {aired: {sort: {field: 'latestAired', direction: 'desc'}}}},
  ]});
  const parsed = parseLibraryMemory(text);
  assert.deepEqual([...parsed.keys()], ['movies', 'tv']);
  assert.deepEqual(parsed.get('movies'), {tab: 'browse', tabs: {browse: {filters: [{field: 'genre', operator: 'in', value: ['Drama']}]}}});
  assert.deepEqual(parsed.get('tv'), {tabs: {aired: {sort: {field: 'latestAired', direction: 'desc'}}}});
  assert.equal(parseLibraryMemory('not json').size, 0);
  assert.equal(parseLibraryMemory(JSON.stringify({version: 2, libraries: []})).size, 0);
  assert.equal(parseLibraryMemory('x'.repeat(300000)).size, 0);
});

test('the memory is bounded: the least recently used library is let go, and oversized filters are not kept', () => {
  const memory = new LibraryMemory(storage(), scope);
  for (let i = 0; i < MAX_REMEMBERED_LIBRARIES + 5; i++) memory.rememberTab('library' + i, 'browse');
  assert.equal(memory.recall('library0'), undefined);
  assert.equal(memory.recall('library5')?.tab, 'browse');
  memory.rememberFilters('library5', 'browse', [{field: 'title', operator: 'contains', value: 'x'.repeat(4000)}]);
  assert.equal(rememberedTab(memory.recall('library5'), 'browse'), undefined);
});

test('storage that fails never breaks remembering for the current run', () => {
  const memory = new LibraryMemory({read: () => { throw new Error('off'); }, write: () => { throw new Error('full'); }, remove: () => { throw new Error('full'); }}, scope);
  memory.rememberTab('movies', 'browse');
  assert.equal(memory.recall('movies')?.tab, 'browse');
  memory.forget();
  assert.equal(memory.recall('movies'), undefined);
});

test('a store that answers later keeps choices made while it was reading', async () => {
  let release!: (text: string) => void;
  const saved: string[] = [];
  const memory = new LibraryMemory({read: () => new Promise<string>(resolve => { release = resolve; }), write: async (_key, text) => { saved.push(text); }, remove: async () => {}}, scope);
  assert.equal(memory.isReady, false);
  memory.rememberTab('movies', 'collections');
  let heard = 0;
  memory.subscribe(() => heard++);
  release(JSON.stringify({version: 1, libraries: [{id: 'movies', tab: 'browse', tabs: {}}, {id: 'tv', tab: 'aired', tabs: {}}]}));
  await memory.ready;
  assert.equal(memory.isReady, true);
  assert.equal(memory.recall('movies')?.tab, 'collections');
  assert.equal(memory.recall('tv')?.tab, 'aired');
  assert.equal(heard, 1);
});

test('forgetting everything removes the saved text', () => {
  const box = storage();
  const memory = new LibraryMemory(box, scope);
  memory.rememberTab('movies', 'browse');
  assert.equal(box.data.size, 1);
  memory.forget();
  assert.equal(box.data.size, 0);
});
