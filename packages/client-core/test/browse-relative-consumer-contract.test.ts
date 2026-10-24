import test from 'node:test';
import assert from 'node:assert/strict';
import {decodeBrowseLibraryResponse} from '../../contracts/src/decoders.ts';
import {schemas} from '../../contracts/src/schemas.ts';
import {BrowseQueryBuilder, BrowseQueryError, parseBrowseNode, parseBrowseResult, validateBrowseQuery, type BrowseCapabilities, type BrowseFieldCapability, type BrowseNode} from '../src/browse.ts';

/**
 * Relative recency (`within-last-days`) crosses four layers, and they do not all say
 * the same thing, on purpose:
 *
 *  1. the generated contract (`BrowseLibraryResponse.applied.query`) admits the
 *     operator only on the three published date fields, for 1–36500 whole days;
 *  2. `parseBrowseNode` is capability-free structure: it re-reads what the server
 *     sent, so it refuses an unknown operator or a bad day count but knows no field
 *     vocabulary;
 *  3. `parseBrowseResult` echoes `applied.query` through that structural parser;
 *  4. `validateBrowseQuery`/`BrowseQueryBuilder` are the send guard: they refuse an
 *     operator the published field capability does not offer.
 *
 * These tests pin the split rather than flattening it. A window on a field the
 * contract cannot express is refused by (1) and (4); (2) and (3) tolerate it
 * because they only ever read server-sourced values, and failing a whole page over
 * one is not their call.
 */

const OPERATOR = 'within-last-days';
const DATE_FIELDS = ['dateAdded', 'releaseDate', 'lastPlayedAt'] as const;

// The published node shape, taken from the generated schema the client decodes.
const nodeSchema: any = (schemas as any).BrowseLibraryResponse.properties.applied.properties.query;
const relative = nodeSchema.oneOf.find((shape: any) => shape.properties?.operator?.enum?.includes(OPERATOR));
const general = nodeSchema.oneOf.find((shape: any) => shape !== relative);

function dateField(id: string): BrowseFieldCapability {
  return {id, type: 'date', labelKey: `browse.field.${id}`, controlHint: 'date-range', complexity: 'standard', cost: 'indexed', operators: ['within-last-days', 'equals', 'at-most', 'at-least', 'between', 'is-present', 'is-missing'], applicableKinds: ['movie']};
}

const yearField: BrowseFieldCapability = {id: 'year', type: 'number', labelKey: 'browse.field.year', controlHint: 'number-range', complexity: 'quick', cost: 'indexed', operators: ['equals', 'at-most', 'at-least', 'between'], applicableKinds: ['movie']};

/** The vocabulary the server publishes today, narrowed to what these cases read. */
function capabilities(maximumDepth = 5): BrowseCapabilities {
  const pivot = {id: 'movies', labelKey: 'browse.pivot.movies', entityKinds: ['movie'], defaultSort: [{field: 'title', direction: 'asc' as const}], supportedViews: ['grid'], browsable: true};
  return {
    library: {id: 'library', name: 'Library', kind: 'movie', defaultView: 'browse', pinned: false},
    pivots: [pivot], resolvedPivot: pivot,
    fields: [dateField('dateAdded'), dateField('releaseDate'), dateField('lastPlayedAt'), yearField],
    sorts: [{id: 'title', labelKey: 'browse.sort.title', directions: ['asc', 'desc'], defaultDirection: 'asc', expensive: false, applicableKinds: ['movie']}],
    quickFilters: [],
    queryLimits: {maximumDepth, maximumClauses: 40, maximumBytes: 8192, maximumSorts: 3, defaultLimit: 50, maximumLimit: 200, cursorTtlMs: 60000},
  };
}

/** A page the generated contract accepts, carrying the applied query under test. */
function page(query: BrowseNode | null): any {
  return {pivot: 'movies', applied: {query, sort: [{field: 'title', direction: 'asc'}]}, entries: [], pageInfo: {start: 0, total: 0, totalIsLowerBound: false, revision: 'revision-1', hasMore: false, nextCursor: '', durationMs: 0, durationIsLowerBound: false}, positionIndex: []};
}

const queryError = (path: string) => (error: unknown) => error instanceof BrowseQueryError && error.code === 'invalid_query' && error.field === path;

test('the published relative window is exactly the three date fields, in whole days from 1 to 36500', () => {
  assert.deepEqual([...relative.properties.field.enum].sort(), [...DATE_FIELDS].sort());
  assert.deepEqual(relative.properties.value, {type: 'integer', minimum: 1, maximum: 36500});
  // The general predicate shape deliberately does not carry the relative operator:
  // it is only ever a date window on one of those fields, never a scalar value.
  assert.equal([...general.properties.operator.enum].includes(OPERATOR), false);
  for (const field of DATE_FIELDS) for (const value of [1, 7, 30, 90, 365, 36500]) {
    const node = {field, operator: OPERATOR, value};
    assert.deepEqual(parseBrowseNode(node), node, `${field} ${value} is read exactly`);
    assert.doesNotThrow(() => validateBrowseQuery(capabilities(), node as BrowseNode), `${field} ${value} is offered`);
    assert.doesNotThrow(() => decodeBrowseLibraryResponse(page(node as BrowseNode)), `${field} ${value} is published`);
    assert.deepEqual(parseBrowseResult(page(node as BrowseNode)).applied.query, node);
  }
});

test('a day count outside 1–36500 is refused by the contract, the structural parser and the send guard', () => {
  for (const value of [0, -1, 36501, 1.5, '30', null, true, [], {}, NaN, Infinity]) {
    const node = {field: 'dateAdded', operator: OPERATOR, value};
    assert.throws(() => parseBrowseNode(node), {code: 'invalid_browse'}, `parse ${String(value)}`);
    assert.throws(() => decodeBrowseLibraryResponse(page(node as never)), /Invalid contract value/, `decode ${String(value)}`);
    assert.throws(() => validateBrowseQuery(capabilities(), node as never), queryError('query.value'), `validate ${String(value)}`);
  }
});

test('a relative window on a non-date field is refused by the contract and the send guard, and only tolerated structurally', () => {
  const node = {field: 'year', operator: OPERATOR, value: 30};
  // 1. The generated contract cannot express it at all: the relative shape enumerates
  //    date fields and the general shape's operator list leaves the relative one out.
  assert.throws(() => decodeBrowseLibraryResponse(page(node as never)), /Invalid contract value/);
  // 2. The structural parser knows no fields, so a server-sourced node is not a parse
  //    failure. This is the exact semantic difference from the published schema.
  assert.deepEqual(parseBrowseNode(node), node);
  // 3. A page's applied query is echoed through that same parser, unchanged.
  assert.deepEqual(parseBrowseResult(page(node)).applied.query, node);
  // 4. Capability validation refuses it at the operator, naming the same path the
  //    server would: the published year filter does not offer the relative operator.
  assert.throws(() => validateBrowseQuery(capabilities(), node), queryError('query.operator'));
  assert.throws(() => validateBrowseQuery(capabilities(), node), /within-last-days is not available for year\./);
  // The executed send path refuses before a request exists.
  assert.throws(() => new BrowseQueryBuilder(capabilities(), 'movies').where('year', OPERATOR, 30), queryError('query.all[0].operator'));
});

test('nested groups keep relative windows exact, refuse a non-date field at its own path, and stay within the published depth', () => {
  const node = {all: [
    {field: 'dateAdded', operator: OPERATOR, value: 30},
    {any: [{field: 'releaseDate', operator: OPERATOR, value: 7}, {not: {field: 'lastPlayedAt', operator: OPERATOR, value: 90}}]},
  ]};
  assert.deepEqual(parseBrowseNode(node), node);
  assert.deepEqual(parseBrowseResult(page(node)).applied.query, node);
  assert.doesNotThrow(() => validateBrowseQuery(capabilities(), node));
  assert.doesNotThrow(() => decodeBrowseLibraryResponse(page(node)));
  // A non-date window is refused wherever it sits, with that node's path.
  assert.throws(() => validateBrowseQuery(capabilities(), {all: [{field: 'dateAdded', operator: OPERATOR, value: 30}, {field: 'year', operator: OPERATOR, value: 30}]}), queryError('query.all[1].operator'));
  // Six levels parse structurally but exceed the published depth of five: the
  // structural bound and the executable bound are separate numbers.
  let deep: BrowseNode = {field: 'dateAdded', operator: OPERATOR, value: 30};
  for (let level = 0; level < 6; level++) deep = {all: [deep]};
  assert.doesNotThrow(() => parseBrowseNode(deep));
  assert.throws(() => validateBrowseQuery(capabilities(), deep), /Nest at most 5 levels\./);
});

test('the applied query is echoed exactly, and no relative operator beyond the published one exists', () => {
  const absolute = {field: 'dateAdded', operator: 'at-least', value: '2026-09-23'};
  const relative30 = {field: 'dateAdded', operator: OPERATOR, value: 30};
  const result = parseBrowseResult(page({all: [relative30, absolute]}));
  assert.deepEqual(result.applied.query, {all: [relative30, absolute]});
  assert.equal(Object.isFrozen(result.applied.query), true);
  assert.equal(parseBrowseResult(page(null)).applied.query, null);
  for (const operator of ['within-last-weeks', 'within-last-months', 'last-days', 'in-the-last-days']) {
    assert.throws(() => parseBrowseNode({field: 'dateAdded', operator, value: 4}), {code: 'invalid_browse'});
  }
});

test('absolute date predicates are unchanged by the relative window work', () => {
  const nodes = [
    {field: 'dateAdded', operator: 'at-least', value: '2026-09-23'},
    {field: 'releaseDate', operator: 'between', value: ['2020-01-01', '2020-12-31']},
    {field: 'lastPlayedAt', operator: 'is-missing', value: null},
  ];
  for (const node of nodes) {
    assert.deepEqual(parseBrowseNode(node), node);
    assert.doesNotThrow(() => validateBrowseQuery(capabilities(), node as BrowseNode));
    assert.deepEqual(parseBrowseResult(page(node as BrowseNode)).applied.query, node);
    // An old absolute predicate and a new relative one live in one group unchanged.
    assert.doesNotThrow(() => decodeBrowseLibraryResponse(page({all: [node as BrowseNode, {field: 'dateAdded', operator: OPERATOR, value: 30}]})));
  }
});

test('every operator the published predicate shape can express is structurally readable', () => {
  const list = new Set(['in', 'not-in', 'between', 'contains-any', 'contains-all']);
  const presence = new Set(['is-present', 'is-missing']);
  for (const operator of general.properties.operator.enum as string[]) {
    const value = presence.has(operator) ? null : list.has(operator) ? (operator === 'between' ? ['a', 'b'] : ['a']) : 'a';
    assert.doesNotThrow(() => parseBrowseNode({field: 'title', operator, value}), `${operator} is not read`);
  }
});
