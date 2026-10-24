import test from 'node:test';
import assert from 'node:assert/strict';
import {createSelectionEditor, editorPreviewRequest, editorTargetLawful, SelectionEditorError} from '../src/screens/browse/selection-editor.ts';

const capabilities = {play: true, queue: true, playlist: true, myList: true, watched: true, collection: true, favorite: true, download: false, edit: true, refresh: true, delete: true};
const item = {kind: 'movie', id: 'm1', libraryId: 'a', capabilities};
const explicit = {mode: 'explicit' as const, context: {}, items: [{key: 'k', item}], displayItems: [{key: 'k', item}]};
const query = {mode: 'query' as const, context: {}, queryId: 'q', selector: {query: {libraryId: 'a', pivot: 'movies', filter: {field: 'year', operator: 'at-least', value: 2000}, sort: [{field: 'title', direction: 'asc'}]}},
  total: 48000, sourceTotal: 48000, refinement: {kind: null, excludedItems: []}, summary: {}, expected: {catalogRevision: 'revision-1'}};
const preview = {serverId: 'server', viewerFence: 'a'.repeat(64), contentKind: 'movie', kind: 'item', total: 1, schema: [], singleItemFields: [], excludedFields: [], fields: {},
  targets: [{kind: 'item', id: 'm1', libraryId: 'a', expectedRevision: 'b'.repeat(64)}]};
const queryPreview = {serverId: 'server', viewerFence: 'a'.repeat(64), contentKind: 'movie', kind: 'item', total: 48000, selectionRevision: 'revision-1', schema: [], singleItemFields: [], excludedFields: [], fields: {}, targets: []};

test('explicit picks preview their exact identities and a whole view previews its query', () => {
  assert.deepEqual(editorPreviewRequest(explicit as never), {targets: [{kind: 'item', contentKind: 'movie', id: 'm1', libraryId: 'a'}]});
  const request = editorPreviewRequest(query as never) as {selector: {query: {libraryId: string}}; expected: {catalogRevision: string}};
  assert.equal(request.selector.query.libraryId, 'a');
  assert.equal(request.expected.catalogRevision, 'revision-1');
  assert.equal(editorTargetLawful(explicit as never), true);
  assert.equal(editorTargetLawful(query as never), true);
  assert.equal(editorTargetLawful(null), false);
});

test('a refined query target is refused before any preview or save', () => {
  const refined = {...query, total: 47999};
  assert.throws(() => editorPreviewRequest(refined as never), SelectionEditorError);
  assert.equal(editorTargetLawful(refined as never), false);
});

test('the preview reads through the real endpoint and the editor refuses when not current', async () => {
  const paths: string[] = [], bodies: unknown[] = [];
  const api = {request: async (path: string, _method?: string, body?: unknown) => {paths.push(path); bodies.push(body); return (body as {targets?: unknown}).targets ? preview : queryPreview; }};
  const editor = createSelectionEditor({api: api as never, events: {on: () => () => {}}, requestId: async () => 'op', isCurrent: () => true, serverId: 'server'});
  const loaded = await editor.preview({target: explicit} as never);
  assert.equal(loaded.total, 1);
  assert.match(paths[0] ?? '', /metadata\/bulk\/preview/);
  // A whole view previews its selector and is fenced to the page revision.
  const whole = await editor.preview({target: query} as never);
  assert.equal(whole.total, 48000);
  const body = bodies[1] as {selector: {query: {libraryId: string}}; expected: {catalogRevision: string}};
  assert.equal(body.selector.query.libraryId, 'a');
  assert.equal(body.expected.catalogRevision, 'revision-1');
  const retired = createSelectionEditor({api: api as never, events: {on: () => () => {}}, requestId: async () => 'op', isCurrent: () => false, serverId: 'server'});
  await assert.rejects(retired.preview({target: explicit} as never), (error: unknown) => error instanceof SelectionEditorError && error.code === 'cancelled');
});

test('a save sends one metadata-edit job with only the changed values, fenced to the view revision', async () => {
  const bodies: unknown[] = [];
  const api = {request: async (_path: string, _method?: string, body?: unknown) => {
    bodies.push(body);
    return {jobId: 'job-1', command: 'metadata-edit', state: 'complete', total: 48000, totalKnown: true, done: 48000, failed: 0};
  }};
  const editor = createSelectionEditor({api: api as never, events: {on: () => () => {}}, requestId: async () => 'operation-1', isCurrent: () => true, serverId: 'server'});
  const result = await editor.save({target: query} as never, {fields: {contentRating: 'G'}, lists: {tags: {add: ['pixar'], remove: []}}, lockEdited: true});
  assert.equal(result.updated, 48000);
  const body = bodies[0] as {command: string; selector: {query: {libraryId: string}}; expected: {catalogRevision: string}; args: {fields: Record<string, string>; lists: Record<string, unknown>; lockEdited: boolean}};
  assert.equal(body.command, 'metadata-edit');
  assert.equal(body.selector.query.libraryId, 'a');
  assert.equal(body.expected.catalogRevision, 'revision-1');
  assert.deepEqual(body.args.fields, {contentRating: 'G'});
  assert.deepEqual(body.args.lists, {tags: {add: ['pixar'], remove: []}});
  assert.equal(body.args.lockEdited, true);
});
