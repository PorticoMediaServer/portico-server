import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';

// P6: the For you sort (one direction, no letters) and a personal section's own See all.
// The sort rule is client-core's (`nextToolbarSort`): one rule for the web's chip and Apple's menu.
import {nextToolbarSort} from '../../packages/client-core/src/presentation/browse-toolbar.ts';
const filters = {nextSort: nextToolbarSort};
import {seeAllPredicates} from '../src/app/see-all.ts';
const browse = {seeAllPredicates};

const capabilities: any = {sorts: [
  {id: 'title', labelKey: 'sort.title', directions: ['asc', 'desc'], defaultDirection: 'asc'},
  {id: 'forYou', labelKey: 'sort.forYou', directions: ['desc'], defaultDirection: 'desc'},
]};

test('For you is labelled from the catalogue and keeps its one direction', () => {
  assert.equal(defaultI18n.t('sort.forYou'), 'For you');
  assert.deepEqual(filters.nextSort(capabilities, {field: 'title', direction: 'asc'}, 'forYou'), {field: 'forYou', direction: 'desc'});
  assert.deepEqual(filters.nextSort(capabilities, {field: 'forYou', direction: 'desc'}, 'forYou'), {field: 'forYou', direction: 'desc'}, 'no flip to ascending');
  assert.deepEqual(filters.nextSort(capabilities, {field: 'forYou', direction: 'desc'}, '@asc'), {field: 'forYou', direction: 'desc'});
  assert.deepEqual(filters.nextSort(capabilities, {field: 'title', direction: 'asc'}, 'title'), {field: 'title', direction: 'desc'});
});

test('a personal section’s See all becomes the browse filters, when the bar can hold them', () => {
  const genre = {field: 'genre', operator: 'contains', value: 'Science Fiction'};
  assert.deepEqual(browse.seeAllPredicates(undefined), []);
  assert.deepEqual(browse.seeAllPredicates(genre), [genre]);
  assert.deepEqual(browse.seeAllPredicates({all: [genre, {field: 'year', operator: 'equals', value: 1999}]}), [genre, {field: 'year', operator: 'equals', value: 1999}]);
  assert.equal(browse.seeAllPredicates({any: [genre]}), undefined);
});
