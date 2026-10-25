import {strict as assert} from 'node:assert';
import {test} from 'node:test';
import type {RepairData} from '../../packages/client-core/src/metadata-repair.ts';
import {metadataDraftValue, patchMetadataDraft} from '../src/screens/metadata/draft-value.ts';

test('lock-only list edits preserve the displayed saved values', () => {
  const data = {schema: [{field: 'genres', type: 'list'}, {field: 'title', type: 'text'}],
    snapshot: {fields: {genres: {value: '', values: ['Drama', 'Science fiction']}, title: {value: 'Dune'}}}} as unknown as RepairData;
  assert.equal(metadataDraftValue(data, 'genres'), 'Drama, Science fiction');
  assert.equal(metadataDraftValue(data, 'title'), 'Dune');
  assert.equal(metadataDraftValue(data, 'missing'), '');
  assert.equal(metadataDraftValue(null, 'genres'), '');
});

test('editing locks the value and explicit unlock retains that edited value', () => {
  const data = {schema: [{field: 'title', type: 'text'}], snapshot: {fields: {title: {value: 'Dune', locked: false}}}} as unknown as RepairData;
  const edited = patchMetadataDraft(data, 'title', undefined, {value: 'Dune: Part One'});
  assert.deepEqual(edited, {value: 'Dune: Part One', locked: true});
  assert.deepEqual(patchMetadataDraft(data, 'title', edited, {locked: false}), {value: 'Dune: Part One', locked: false});
});

test('a list lock toggle retains saved list values', () => {
  const data = {schema: [{field: 'tags', type: 'list'}], snapshot: {fields: {tags: {value: '', values: ['Space', 'Desert'], locked: false}}}} as unknown as RepairData;
  assert.deepEqual(patchMetadataDraft(data, 'tags', undefined, {locked: true}), {value: 'Space, Desert', locked: true});
});
