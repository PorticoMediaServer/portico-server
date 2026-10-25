import {strict as assert} from 'node:assert';
import {test} from 'node:test';
import type {RepairData} from '../../packages/client-core/src/metadata-repair.ts';
import {metadataDraftValue} from '../src/screens/metadata/draft-value.ts';

test('lock-only list edits preserve the displayed saved values', () => {
  const data = {schema: [{field: 'genres', type: 'list'}, {field: 'title', type: 'text'}],
    snapshot: {fields: {genres: {value: '', values: ['Drama', 'Science fiction']}, title: {value: 'Dune'}}}} as unknown as RepairData;
  assert.equal(metadataDraftValue(data, 'genres'), 'Drama, Science fiction');
  assert.equal(metadataDraftValue(data, 'title'), 'Dune');
  assert.equal(metadataDraftValue(data, 'missing'), '');
  assert.equal(metadataDraftValue(null, 'genres'), '');
});
