import test from 'node:test';
import assert from 'node:assert/strict';
import {homeRowSeeAll} from '../src/app/see-all.ts';

const row = (more: Record<string, unknown> = {}) => ({id: 'recent_movies', libraryId: 'movies', cursorCapable: true, ...more}) as Parameters<typeof homeRowSeeAll>[0];

test('a row that names its own browse opens the library on exactly that pivot, sort and conditions', () => {
  const destination = homeRowSeeAll(row({seeAll: {pivot: 'movies', sort: [{field: 'added', direction: 'desc'}], query: {field: 'genre', operator: 'in', value: ['Drama']}}}), 'movie');
  assert.deepEqual(destination, {to: 'browse', libraryId: 'movies', tab: 'browse', sort: {field: 'added', direction: 'desc'}, filters: [{field: 'genre', operator: 'in', value: ['Drama']}]});
  assert.deepEqual(homeRowSeeAll(row({id: 'top_albums', libraryId: 'music', seeAll: {pivot: 'albums', sort: [{field: 'rating', direction: 'desc'}]}}), 'music'), {to: 'browse', libraryId: 'music', tab: 'releases', sort: {field: 'rating', direction: 'desc'}, filters: []});
});

test('nothing is guessed from a row’s id: a row that names no browse opens as its own page', () => {
  assert.deepEqual(homeRowSeeAll(row(), 'movie'), {to: 'row', rowId: 'recent_movies'});
  assert.deepEqual(homeRowSeeAll(row({id: 'top_rated_movies'}), 'movie'), {to: 'row', rowId: 'top_rated_movies'});
});

test('a browse this app cannot open (an unknown pivot, a query too rich for the filter bar) falls back to the row’s own page', () => {
  assert.deepEqual(homeRowSeeAll(row({seeAll: {pivot: 'holograms', sort: [{field: 'added', direction: 'desc'}]}}), 'movie'), {to: 'row', rowId: 'recent_movies'});
  assert.deepEqual(homeRowSeeAll(row({seeAll: {pivot: 'movies', sort: [{field: 'added', direction: 'desc'}], query: {any: [{field: 'genre', operator: 'in', value: ['Drama']}]}}}), 'movie'), {to: 'row', rowId: 'recent_movies'});
  // The library is not loaded yet, so its kind (and so its tabs) is unknown.
  assert.deepEqual(homeRowSeeAll(row({seeAll: {pivot: 'movies', sort: [{field: 'added', direction: 'desc'}]}}), undefined), {to: 'row', rowId: 'recent_movies'});
});

test('a row that can be neither browsed nor paged opens its library, and a row with nothing has no See all', () => {
  assert.deepEqual(homeRowSeeAll(row({cursorCapable: false}), 'movie'), {to: 'library', libraryId: 'movies'});
  assert.equal(homeRowSeeAll(row({cursorCapable: false, libraryId: undefined}), undefined), undefined);
});
