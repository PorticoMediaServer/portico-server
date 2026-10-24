import test from 'node:test';
import assert from 'node:assert/strict';
import {categoryDestination, libraryTab, libraryTabForPivot, libraryTabSort, libraryTabs} from '../src/presentation/library-tabs.ts';

test('Recently aired opens latest episode first and leaves the order to the viewer', () => {
  const aired = libraryTab('tv', 'aired');
  assert.deepEqual(aired.defaultSort, {field: 'latestAired', direction: 'desc'});
  // The server offers the sort: the tab opens in its own order.
  assert.deepEqual(libraryTabSort(aired, [{id: 'title'}, {id: 'latestAired'}], {field: 'title', direction: 'asc'}), {field: 'latestAired', direction: 'desc'});
  // A server that does not offer it: the pivot's default, never a sort the server would refuse.
  assert.deepEqual(libraryTabSort(aired, [{id: 'title'}], {field: 'title', direction: 'asc'}), {field: 'title', direction: 'asc'});
  // Before the vocabulary has arrived the tab's own order is assumed.
  assert.deepEqual(libraryTabSort(aired, undefined, undefined), {field: 'latestAired', direction: 'desc'});
});

test('a tab without its own order opens in the pivot default, then title', () => {
  const shows = libraryTab('tv', 'browse');
  assert.deepEqual(libraryTabSort(shows, [{id: 'title'}, {id: 'added'}], {field: 'added', direction: 'desc'}), {field: 'added', direction: 'desc'});
  assert.deepEqual(libraryTabSort(shows, [], undefined), {field: 'title', direction: 'asc'});
});

test('no tab hides the sort: none declares an order the viewer cannot change', () => {
  for (const kind of ['movie', 'tv', 'anime', 'music', 'audiobook']) {
    for (const tab of libraryTabs(kind, {owner: true, unmatched: true})) assert.equal('fixedSort' in tab, false, `${kind}/${tab.id}`);
  }
});

test('a row that names a pivot opens the plain tab for it, not Recently aired', () => {
  assert.equal(libraryTabForPivot('tv', 'shows')?.id, 'browse');
  assert.equal(libraryTabForPivot('music', 'albums')?.id, 'releases');
});

test('a category link is the tab that lists its titles with one ordinary filter', () => {
  assert.deepEqual(categoryDestination('movie', 'genre:Drama'), {tab: 'browse', predicate: {field: 'genre', operator: 'contains-any', value: ['Drama']}});
  assert.deepEqual(categoryDestination('tv', 'decade:1990'), {tab: 'browse', predicate: {field: 'decade', operator: 'in', value: [1990]}});
  assert.deepEqual(categoryDestination('movie', 'studio:A24: Films'), {tab: 'browse', predicate: {field: 'studio', operator: 'in', value: ['A24: Films']}});
  // Music genres open albums, as the Genres tab does; an audiobook series opens books.
  assert.deepEqual(categoryDestination('music', 'genre:Jazz'), {tab: 'releases', predicate: {field: 'genre', operator: 'contains-any', value: ['Jazz']}});
  assert.deepEqual(categoryDestination('audiobook', 'series:Dune'), {tab: 'browse', predicate: {field: 'series', operator: 'contains-any', value: ['Dune']}});
});

test('a category this app does not know is left to the server', () => {
  assert.equal(categoryDestination('movie', 'mood:Cosy'), undefined);
  assert.equal(categoryDestination('movie', 'decade:199x'), undefined);
  assert.equal(categoryDestination('movie', 'genre:'), undefined);
  assert.equal(categoryDestination('movie', 'nothing'), undefined);
});

test('a reading library offers Discover and the bounded Books list, never empty facet tabs', () => {
  for (const kind of ['book', 'comic']) {
    assert.deepEqual(libraryTabs(kind).map(tab => tab.id), ['discover', 'browse']);
    assert.equal(libraryTab(kind, 'browse').label, 'library.tab.books');
    // Books reads the bounded Reading list, not the generic browse pivots.
    assert.equal('pivot' in libraryTab(kind, 'browse'), false);
    // Magazines, Newspapers, Authors and Series stay absent until their facts and routes exist.
    assert.deepEqual(libraryTabs(kind).map(tab => tab.id), ['discover', 'browse']);
  }
});
