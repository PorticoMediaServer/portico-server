import test from 'node:test';
import assert from 'node:assert/strict';
import {presetApplication} from '../src/presentation/index.ts';

// A library's presets (its saved views) are applied exactly as stored: as the tab's own controls
// when that is all they are, otherwise run whole. Nothing is flattened or dropped.
const browse = (query: any, sort: any[] = [{field: 'title', direction: 'asc'}], pivot = 'shows') => ({libraryId: 'l', pivot, query, sort});
const drama = {field: 'genre', operator: 'in', value: ['Drama']} as const;
const nineties = {field: 'year', operator: 'between', value: [1990, 1999]} as const;

test('presetApplication: filters that must all match and one sort are the tab\'s own controls', () => {
  assert.deepEqual(presetApplication('tv', browse({all: [drama, nineties]})), {kind: 'controls', tab: 'browse', predicates: [drama, nineties], sort: {field: 'title', direction: 'asc'}});
  assert.deepEqual(presetApplication('tv', browse(undefined)), {kind: 'controls', tab: 'browse', predicates: [], sort: {field: 'title', direction: 'asc'}});
  assert.equal(presetApplication('tv', browse(drama)).kind, 'controls');
});

test('presetApplication: anything richer runs exactly as stored', () => {
  for (const [why, b] of [
    ['any', browse({any: [drama, nineties]})],
    ['nested not', browse({all: [drama, {not: nineties}]})],
    ['two sorts', browse(drama, [{field: 'year', direction: 'desc'}, {field: 'title', direction: 'asc'}])],
    ['a shuffle\'s seed', browse(drama, [{field: 'random', direction: 'asc', seed: 7}])],
  ] as const) assert.deepEqual(presetApplication('tv', b as any), {kind: 'exact', tab: 'browse'}, why);
  assert.equal(presetApplication('tv', browse({all: [drama, nineties]}), new Set(['genre'])).kind, 'exact', 'a field the controls do not offer is never dropped');
  assert.deepEqual(presetApplication('tv', browse(drama, undefined, 'nowhere')), {kind: 'exact'}, 'no tab browses that pivot');
});
