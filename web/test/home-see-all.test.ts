import test from 'node:test';
import assert from 'node:assert/strict';
import {homeRowSeeAll} from '../src/app/see-all.ts';

const row = (more: Record<string, unknown> = {}) => ({id: 'row_1', kind: 'shelf', libraryId: 'movies', cursorCapable: true, ...more}) as Parameters<typeof homeRowSeeAll>[0];

test('a row that names its own browse opens the library on exactly that pivot, sort and conditions', () => {
  const destination = homeRowSeeAll(row({seeAll: {type: 'library', libraryId: 'movies', pivot: 'movies', sort: [{field: 'added', direction: 'desc'}], query: {field: 'genre', operator: 'in', value: ['Drama']}}}), 'movie');
  assert.deepEqual(destination, {to: 'browse', libraryId: 'movies', tab: 'browse', sort: {field: 'added', direction: 'desc'}, filters: [{field: 'genre', operator: 'in', value: ['Drama']}]});
  assert.deepEqual(homeRowSeeAll(row({id: 'top_albums', libraryId: 'music', seeAll: {type: 'library', libraryId: 'music', pivot: 'albums', sort: [{field: 'rating', direction: 'desc'}]}}), 'music'), {to: 'browse', libraryId: 'music', tab: 'releases', sort: {field: 'rating', direction: 'desc'}, filters: []});
});

test('nothing is read out of a row’s id: a row that names no browse opens as its own page', () => {
  assert.deepEqual(homeRowSeeAll(row(), 'movie'), {to: 'row', rowId: 'row_1'});
  assert.deepEqual(homeRowSeeAll(row({id: 'recently_added_by_friends'}), 'movie'), {to: 'row', rowId: 'recently_added_by_friends'}, 'an id that merely sounds recent means nothing');
  assert.deepEqual(homeRowSeeAll(row({id: 'top_rated_movies'}), 'movie'), {to: 'row', rowId: 'top_rated_movies'});
});

test('a browse this app cannot open (an unknown pivot, a query too rich for the filter bar) falls back to the row’s own page', () => {
  assert.deepEqual(homeRowSeeAll(row({seeAll: {type: 'library', libraryId: 'movies', pivot: 'holograms', sort: [{field: 'added', direction: 'desc'}]}}), 'movie'), {to: 'row', rowId: 'row_1'});
  assert.deepEqual(homeRowSeeAll(row({seeAll: {type: 'library', libraryId: 'movies', pivot: 'movies', sort: [{field: 'added', direction: 'desc'}], query: {any: [{field: 'genre', operator: 'in', value: ['Drama']}]}}}), 'movie'), {to: 'row', rowId: 'row_1'});
  // The library is not loaded yet, so its kind (and so its tabs) is unknown.
  assert.deepEqual(homeRowSeeAll(row({seeAll: {type: 'library', libraryId: 'movies', pivot: 'movies', sort: [{field: 'added', direction: 'desc'}]}}), undefined), {to: 'row', rowId: 'row_1'});
});

test('a row that can be neither browsed nor paged opens its library, and a row with nothing has no See all', () => {
  assert.deepEqual(homeRowSeeAll(row({cursorCapable: false}), 'movie'), {to: 'library', libraryId: 'movies'});
  assert.equal(homeRowSeeAll(row({cursorCapable: false, libraryId: undefined}), undefined), undefined);
});

test('the server’s own recent kind opens the library newest first until the row names its browse', () => {
  assert.deepEqual(homeRowSeeAll(row({id: 'recent_movies', kind: 'recent'}), 'movie'), {to: 'browse', libraryId: 'movies', tab: 'browse', sort: {field: 'added', direction: 'desc'}, filters: []});
  assert.deepEqual(homeRowSeeAll(row({id: 'recent_movies', kind: 'recent', seeAll: {type: 'library', libraryId: 'movies', pivot: 'collections', sort: [{field: 'title', direction: 'asc'}]}}), 'movie'), {to: 'browse', libraryId: 'movies', tab: 'collections', sort: {field: 'title', direction: 'asc'}, filters: []}, 'what the row names wins');
});

 test('explicit row and saved-view destinations override row identifiers and library fallbacks', () => {
  assert.deepEqual(homeRowSeeAll(row({seeAll: {type: 'row', rowId: 'another_row'}}), 'movie'), {to: 'row', rowId: 'another_row'});
  assert.deepEqual(homeRowSeeAll(row({seeAll: {type: 'saved_view', resourceId: 'my_view'}}), 'movie'), {to: 'saved_view', resourceId: 'my_view'});
 });

test('collection See all names the existing collection content route without a library fallback',()=>{
  assert.deepEqual(homeRowSeeAll(row({libraryId:undefined,seeAll:{type:'collection',resourceId:'selected-collection',libraryId:'selected-library'}}),undefined),{
    to:'/library/$libraryId/$view/$entityId',params:{libraryId:'selected-library',view:'collection',entityId:'selected-collection'},search:{},
  });
});

test('guide See all retains its actual source and group in existing channel routing',()=>{
  assert.deepEqual(homeRowSeeAll(row({libraryId:undefined,seeAll:{type:'guide',sourceId:'actual-source',group:'Sports'}}),undefined),{
    to:'/channels/$entry',params:{entry:'actual-source'},search:{view:'guide',group:'Sports'},
  });
  assert.deepEqual(homeRowSeeAll(row({seeAll:{type:'guide'}}),undefined),{
    to:'/channels/$entry',params:{entry:'all'},search:{view:'guide'},
  });
});
