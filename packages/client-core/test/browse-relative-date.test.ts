import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '../../i18n/src/index.ts';
import {parseBrowseCapabilities, parseBrowseNode, validateBrowseQuery, type BrowseNode, type BrowseSortSelection} from '../src/browse.ts';
import type {SavedDefinition} from '../src/personal-saved.ts';
import {activeRangePreset, appliedFilterLabel, rangePresets} from '../src/presentation/browse-toolbar.ts';
import {savedViewSummary} from '../src/presentation/saved-view-summary.ts';

const field = (id: string, type: string, operators: readonly string[]) => ({id, type, operators, labelKey: `browse.field.${id}`, controlHint: type === 'date' ? 'date-range' : 'select', applicableKinds: ['movie']});
const caps = parseBrowseCapabilities({
  library: {id: 'library', name: 'Movies', kind: 'movie', defaultView: 'browse'},
  pivots: [{id: 'movies', labelKey: 'library.movies', entityKinds: ['movie'], defaultSort: [{field: 'title', direction: 'asc'}], supportedViews: ['grid', 'list'], browsable: true}],
  fields: [
    field('dateAdded', 'date', ['at-least', 'within-last-days']),
    field('releaseDate', 'date', ['at-least', 'within-last-days']),
    field('lastPlayedAt', 'date', ['at-least', 'within-last-days']),
    field('playState', 'enum', ['equals']),
  ],
  quickFilters: [],
  sorts: ['title', 'added', 'lastPlayed'].map(id => ({id, labelKey: `sort.${id}`, directions: ['asc', 'desc'], defaultDirection: id === 'title' ? 'asc' : 'desc', applicableKinds: ['movie']})),
  queryLimits: {maximumDepth: 5, maximumClauses: 40, maximumBytes: 65536, maximumSorts: 3, defaultLimit: 40, maximumLimit: 100, cursorTtlMs: 10000},
});
const definition = (query: BrowseNode | null, sort: string, direction: 'asc' | 'desc'): SavedDefinition => ({libraryId: 'library', pivot: 'movies', query, sort: [{field: sort, direction}], presentation: 'grid'});
const dateAdded = caps.fields.find(value => value.id === 'dateAdded')!;

test('date presets emit the relative operator for every window', () => {
  const presets = rangePresets(dateAdded, new Date('2026-10-23T12:00:00Z'));
  assert.deepEqual(presets.map(preset => preset.id), ['last-7', 'last-30', 'last-90', 'last-365']);
  assert.deepEqual(presets.map(preset => preset.predicates), [
    [{field: 'dateAdded', operator: 'within-last-days', value: 7}],
    [{field: 'dateAdded', operator: 'within-last-days', value: 30}],
    [{field: 'dateAdded', operator: 'within-last-days', value: 90}],
    [{field: 'dateAdded', operator: 'within-last-days', value: 365}],
  ]);
  assert.equal(presets[1]?.label.id, 'filter.preset.lastDays');
  assert.equal(presets[3]?.label.id, 'filter.preset.lastYear');
});

test('activeRangePreset recognises the relative operator and not a frozen date', () => {
  const presets = rangePresets(dateAdded, new Date('2026-10-23T12:00:00Z'));
  assert.equal(activeRangePreset(presets, [{field: 'dateAdded', operator: 'within-last-days', value: 30}]), 'last-30');
  assert.equal(activeRangePreset(presets, [{field: 'dateAdded', operator: 'at-least', value: '2026-09-23'}]), undefined);
});

test('the chip uses the existing preset wording, with last year for 365', () => {
  assert.equal(appliedFilterLabel({field: 'dateAdded', operator: 'within-last-days', value: 30}, () => 'Date added', defaultI18n.t), 'In the last 30 days');
  assert.equal(appliedFilterLabel({field: 'lastPlayedAt', operator: 'within-last-days', value: 7}, () => 'Last played', defaultI18n.t), 'In the last 7 days');
  assert.equal(appliedFilterLabel({field: 'dateAdded', operator: 'within-last-days', value: 365}, () => 'Date added', defaultI18n.t), 'In the last year');
});

test('the summary maps relative date windows for dateAdded and every other date field', () => {
  assert.deepEqual(savedViewSummary(definition({field: 'dateAdded', operator: 'within-last-days', value: 30}, 'added', 'desc'), caps, defaultI18n), {
    line: 'Added in the last 30 days. Newest first.',
    conditions: 'Added in the last 30 days',
    suggestedName: 'Added in the last 30 days',
  });
  assert.equal(savedViewSummary(definition({field: 'lastPlayedAt', operator: 'within-last-days', value: 7}, 'lastPlayed', 'desc'), caps, defaultI18n)?.line, 'Last played in the last 7 days. Last played first.');
  assert.equal(savedViewSummary(definition({field: 'releaseDate', operator: 'within-last-days', value: 90}, 'title', 'asc'), caps, defaultI18n)?.conditions, 'Release date in the last 90 days');
});

test('the client parser and validator take only a whole number of days from 1 to 36500', () => {
  const node = {field: 'dateAdded', operator: 'within-last-days', value: 30} as const;
  assert.deepEqual(parseBrowseNode(node), node);
  assert.doesNotThrow(() => validateBrowseQuery(caps, parseBrowseNode(node)));
  for (const value of [0, 36501, 1.5, '30', null, true]) {
    assert.throws(() => parseBrowseNode({field: 'dateAdded', operator: 'within-last-days', value}), {code: 'invalid_browse'});
    assert.throws(() => validateBrowseQuery(caps, {field: 'dateAdded', operator: 'within-last-days', value} as never), {code: 'invalid_query'});
  }
  assert.throws(() => parseBrowseNode({field: 'dateAdded', operator: 'within-last-days'}), {code: 'invalid_browse'});
  // The operator is date-only, so a number field refuses it.
  assert.throws(() => validateBrowseQuery(caps, {field: 'playState', operator: 'within-last-days', value: 30} as never), {code: 'invalid_query'});
});
