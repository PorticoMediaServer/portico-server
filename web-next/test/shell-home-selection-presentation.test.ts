import test from 'node:test';
import assert from 'node:assert/strict';
import type {BulkReceipt} from '@core/metadata-bulk.ts';
import type {BulkMetadataPreview, HomeSelectionMember} from '@core/home-selection/index.ts';
import {editorCoreDraft, editorResultView, editorUiDraft, editorValues, selectionMemberDisplay} from '../src/shell/selection-presentation.ts';
import {homeFixtures as f} from '../../packages/i18n/src/catalog/en-US/next/dev/home-fixtures.ts';
import {runSelectionCommand} from '../src/shell/selection-actions.ts';

test('a menu command consumes its current canonical target before the overlay close callback', async () => {
  let menuOpen = true, called = 0;
  const errors: unknown[] = [];
  runSelectionCommand(() => { assert.equal(menuOpen, true); called++; return Promise.resolve(); }, error => errors.push(error));
  menuOpen = false;
  assert.equal(called, 1);
  await Promise.resolve();
  assert.deepEqual(errors, []);
});
test('command feedback catches synchronous authority failures and asynchronous transport rejection', async () => {
  const sync = new Error('retired'), asyncError = new Error('busy'), errors: unknown[] = [];
  runSelectionCommand(() => { throw sync; }, error => errors.push(error));
  runSelectionCommand(() => Promise.reject(asyncError), error => errors.push(error));
  await Promise.resolve();
  assert.deepEqual(errors, [sync, asyncError]);
});

test('editing sends only changed scalar fields and keeps genre and tag add/remove deltas distinct', () => {
  const draft = {fields: {year: '2026'}, lists: {genres: {add: [f.genre], remove: []}, tags: {add: [], remove: [f.genre]}}, lockEdited: true};
  const body = editorCoreDraft(draft);
  assert.deepEqual(body, {fields: {year: {value: '2026'}}, lists: {tags: {add: [], remove: [f.genre]}}, genres: {add: [f.genre], remove: []}, lockEdited: true});
  assert.deepEqual(editorUiDraft(body), draft);
  assert.ok(!Object.hasOwn(body.fields!, 'title'));
});
test('metadata preview preserves the exact distinct count and truncation instead of treating 64 values as the full distribution', () => {
  const values = Array.from({length: 64}, (_, index) => ({value: String(1900 + index), count: 1}));
  const preview = {schema: [{field: 'year', type: 'integer', bulk: true}], fields: {year: {state: 'mixed', distinctCount: 100, truncated: true, values, entries: [], locked: 0}}} as unknown as BulkMetadataPreview;
  const shown = editorValues(preview).year;
  assert.equal(shown.state, 'mixed'); assert.equal(shown.distinctCount, 100); assert.equal(shown.truncated, true);
  assert.equal(shown.values, values);
  assert.equal(editorValues(preview), preview.fields);
});
test('an actual shared empty string survives the editor display and unchanged draft omits it', () => {
  const preview = {schema: [{field: 'studio', type: 'text', bulk: true}], fields: {studio: {state: 'shared', distinctCount: 1, truncated: false, value: '', values: [{value: '', count: 4}], entries: [], locked: 0}}} as unknown as BulkMetadataPreview;
  const shown = editorValues(preview).studio;
  assert.equal(shown.state, 'shared');
  assert.deepEqual(shown.values, [{value: '', count: 4}]);
  assert.deepEqual(editorCoreDraft(editorUiDraft({lockEdited: true})).fields, {});
});
test('dialog display keeps real container and source-local channel names without a renderer identity cache', () => {
  const collection = {item: {kind: 'collection', id: 'collection', libraryId: 'movies'}, collection: {collection: {id: 'collection', name: f.movie}, members: []}} as unknown as HomeSelectionMember;
  assert.deepEqual(selectionMemberDisplay(collection), {id: 'collection', title: f.movie, posterUrl: undefined});
  const channel = {item: {kind: 'channel', sourceId: 'source', channelId: 'channel'}, channel: {channel: {sourceId: 'source', id: 'channel', name: f.channel}}} as unknown as HomeSelectionMember;
  assert.equal(selectionMemberDisplay(channel).id, JSON.stringify(['source', 'channel']));
  assert.equal(selectionMemberDisplay(channel).title, f.channel);
  assert.throws(() => selectionMemberDisplay({item: {kind: 'movie', id: 'private-id'}} as HomeSelectionMember), /display-required/);
});
test('actual retry receipts replace the prior failed status without counting a successful title twice', () => {
  const receipts: BulkReceipt[] = [
    {operationId: 'first', updated: 1, failed: 1, results: [{kind: 'item', id: 'one', ok: true}, {kind: 'item', id: 'two', ok: false, code: 'revision_mismatch'}]},
    {operationId: 'retry', updated: 1, failed: 0, results: [{kind: 'item', id: 'two', ok: true}]},
  ];
  const result = editorResultView(receipts)!;
  assert.equal(result.updated, 2); assert.equal(result.failed, 0); assert.equal(result.results.length, 2);
  assert.equal(result.results[1], receipts[1].results[0]);
  assert.equal('operationId' in result, false);
  assert.equal(receipts[0].results[1].ok, false);
  assert.equal(editorResultView([]), undefined);
});
