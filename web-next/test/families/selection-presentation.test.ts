import test from 'node:test';
import assert from 'node:assert/strict';
import type {DeletePreview} from '../../../packages/client-core/src/administration.ts';
import type {RepairFieldSpec} from '../../../packages/client-core/src/metadata-repair.ts';
import {addListValue, bulkDraftValid, bulkEditorSchema, bulkSummary, deletePermission, missingPickerIds, toggleListRemoval} from '../../src/kit/selection/presentation.ts';
import type {BulkEditorDraft, BulkFieldValues} from '../../src/kit/selection/types.ts';

const schema: readonly RepairFieldSpec[] = [
  {field: 'year', label: 'Year', group: 'numbers', type: 'integer', min: 1800, max: 2200, bulk: true},
  {field: 'title', label: 'Title', group: 'general', type: 'text', bulk: false},
  {field: 'tags', label: 'Tags', group: 'general', type: 'list', bulk: true},
];
const values: Readonly<Record<string, BulkFieldValues>> = {year: {kind: 'scalar', values: [{value: '2019', count: 1}, {value: '2020', count: 3}]}, tags: {kind: 'list', entries: [{value: 'shared', count: 3}]}};
const empty: BulkEditorDraft = {fields: {}, lists: {}, lockEdited: true};

test('only actual bulk fields can enter a save; untouched Mixed fields never become empty writes', () => {
  assert.equal(bulkDraftValid(schema, values, empty), false);
  assert.equal(bulkDraftValid(schema, values, {...empty, fields: {year: '2020'}}), true);
  assert.equal(bulkDraftValid(schema, values, {...empty, fields: {title: 'changed'}}), false);
  assert.equal(bulkDraftValid(schema, values, {...empty, fields: {unsupported: 'value'}}), false);
  assert.equal(bulkDraftValid(schema, {}, {...empty, fields: {year: '2020'}}), false);
  assert.deepEqual(bulkSummary(schema, values, empty, 4), []);
});

test('schema numeric limits reject invalid values before an operation is offered', () => {
  for (const year of ['not-a-number', 'Infinity', '2020.5', '1799', '2201']) assert.equal(bulkDraftValid(schema, values, {...empty, fields: {year}}), false, year);
  assert.equal(bulkDraftValid(schema, values, {...empty, fields: {year: ''}}), true);
});

test('the summary counts the actual aggregate difference instead of claiming all titles change', () => {
  const summary = bulkSummary(schema, values, {...empty, fields: {year: '2020'}}, 4);
  assert.equal(summary.length, 1);
  assert.equal(summary[0]!.changedCount, 1);
  assert.equal(summary[0]!.text, 'Year becomes 2020 on all 4');
});

test('Genres appears only when the existing separate bulk capability is explicitly supplied', () => {
  assert.deepEqual(bulkEditorSchema(schema, 'movie').map(field => field.field), ['year', 'tags']);
  const allowed = bulkEditorSchema(schema, 'movie', {allowed: true, values: {kind: 'list', entries: []}});
  assert.deepEqual(allowed.map(field => field.field), ['year', 'genres', 'tags']);
  assert.deepEqual(bulkEditorSchema(schema, 'movie', {allowed: false, values: {kind: 'list', entries: []}}).map(field => field.field), ['year', 'tags']);
});

test('list changes preserve shared and partial values and never replace a list', () => {
  const removed = toggleListRemoval(undefined, 'shared');
  assert.deepEqual(removed, {add: [], remove: ['shared']});
  assert.deepEqual(toggleListRemoval(removed, 'shared'), {add: [], remove: []});
  const added = addListValue(removed, '  shared  ');
  assert.deepEqual(added, {add: ['shared'], remove: []});
  assert.deepEqual(addListValue(added!, 'shared'), added);
  assert.deepEqual(toggleListRemoval(added!, 'shared'), {add: [], remove: []});
  assert.equal(addListValue(undefined, '  '), null);
  assert.equal(addListValue(undefined, 'bad\u0000value'), null);
  assert.equal(bulkDraftValid(schema, values, {...empty, lists: {tags: {add: ['shared'], remove: ['shared']}}}), false);
});

test('picker retry sends only unique missing ids in their real result order', () => {
  assert.deepEqual(missingPickerIds([{id: 'failed-2', title: 'second', reason: 'changed'}, {id: 'failed-1', title: 'first', reason: 'changed'}, {id: 'failed-2', title: 'second', reason: 'changed'}]), ['failed-2', 'failed-1']);
  assert.deepEqual(missingPickerIds([]), []);
});

const target = {itemId: 'title-1', title: 'title', kind: 'movie', libraryId: 'library-1', libraryName: 'library', allowMediaDeletion: true, titles: 1, bytes: 10, files: [], dependents: []};
const preview: DeletePreview = {revision: 9, confirmation: 'title', confirmationKind: 'title', totalBytes: 10, totalFiles: 1, totalBytesText: '10 bytes', blocked: [], targets: [target]};

test('deletion requires the exact server confirmation, a real preview, an owner and a matching count', () => {
  assert.equal(deletePermission(true, 1, preview, 'title'), true);
  for (const text of ['', 'TITLE', ' title', 'title ']) assert.equal(deletePermission(true, 1, preview, text), false);
  assert.equal(deletePermission(false, 1, preview, 'title'), false);
  assert.equal(deletePermission(true, 1, null, 'title'), false);
  assert.equal(deletePermission(true, 2, preview, 'title'), false);
  assert.equal(deletePermission(true, 1, preview, 'title', true), false);
  assert.equal(deletePermission(true, 1, {...preview, confirmation: ''}, ''), false);
  assert.equal(deletePermission(true, 2, {...preview, confirmation: '2', confirmationKind: 'count', targets: [target, target]}, '2'), false);
});

test('delete restrictions cannot be bypassed through a typed count or a whole-view selection', () => {
  assert.equal(deletePermission(true, 201, {...preview, confirmation: '201', confirmationKind: 'count'}, '201'), false);
  assert.equal(deletePermission(true, 1, {...preview, blocked: ['library-1']}, 'title'), false);
  assert.equal(deletePermission(true, 1, {...preview, targets: [{...target, allowMediaDeletion: false}]}, 'title'), false);
  const mixed: DeletePreview = {...preview, confirmation: '2', confirmationKind: 'count', targets: [target, {...target, itemId: 'title-2', libraryId: 'library-2'}]};
  assert.equal(deletePermission(true, 2, mixed, '2'), false);
});
