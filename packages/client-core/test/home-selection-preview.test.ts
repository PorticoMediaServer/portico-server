import assert from 'node:assert/strict';
import test from 'node:test';
import {parseBulkMetadataPreview} from '../src/home-selection/metadata-preview.ts';

test('Mixed carries the exact distinct count independently of its bounded displayed values', () => {
  const targets = Array.from({length: 70}, (_, index) => ({kind: 'item' as const, contentKind: 'movie', id: String(index), libraryId: 'library'}));
  const raw = {serverId: 'server', viewerFence: 'a'.repeat(64), kind: 'item', contentKind: 'movie', total: 70, schema: [{field: 'year', label: 'Year', group: 'numbers', type: 'integer', bulk: true}], singleItemFields: [{field: 'year', label: 'Year', group: 'numbers', type: 'integer', bulk: true}, {field: 'title', label: 'Title', group: 'general', type: 'text', bulk: false}], excludedFields: [{field: 'title', label: 'Title', group: 'general', type: 'text', bulk: false}], fields: {year: {state: 'mixed', distinctCount: 70, values: Array.from({length: 64}, (_, index) => ({value: String(1900 + index), count: 1})), entries: [], locked: 0, truncated: true}}, targets: targets.map(target => ({...target, expectedRevision: 'b'.repeat(64)}))};
  const parsed = parseBulkMetadataPreview(raw, {targets}, 'server');
  assert.deepEqual(parsed.excludedFields.map(field => [field.field, field.label]), [['title', 'Title']]);
  assert.equal(parsed.fields.year.distinctCount, 70); assert.equal(parsed.fields.year.values.length, 64); assert.equal(parsed.fields.year.value, undefined);
  assert.throws(() => parseBulkMetadataPreview({...raw, fields: {year: {...raw.fields.year, distinctCount: undefined}}}, {targets}, 'server'), /invalid-metadata-preview/);
  assert.throws(() => parseBulkMetadataPreview({...raw, fields: {year: {...raw.fields.year, truncated: false}}}, {targets}, 'server'), /invalid-metadata-preview/);
});
