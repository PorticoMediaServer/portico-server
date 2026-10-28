import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {selectionUnavailableActions} from '../src/screens/browse/selection-unavailable-actions';
import {wholeViewQuery} from '../src/screens/browse/selection-query';
import {createQuerySelectionEditor} from '../src/screens/browse/selection-query-editor';
import {componentModule} from './helpers/component-harness.mjs';
import {createRenderer, tree} from './helpers/interaction-harness.mjs';

const unavailableActions = {play: 'unsupported-sort', queue: 'unsupported-sort', metadataPreview: 'preview-limit'} as const;
const count = 201, basis = 'b'.repeat(64), revision = `browse_${'a'.repeat(64)}`;
const every = {play: true, queue: true, playlist: true, myList: true, watched: true, collection: true, favorite: true, download: true, edit: true, refresh: true, delete: true};
const descriptor = {libraryId: 'lib', pivot: 'movies', filter: null, sort: [{field: 'title', direction: 'asc'}], catalogRevision: revision, selectionBasis: basis, total: count, unavailableActions,
  types: [{kind: 'movie', count, libraries: [{libraryId: 'lib', count}], capabilities: Object.fromEntries(Object.keys(every).map(key => [key, count]))}]};
test('web query mapping retains optional exclusions, proof and truthful counts', () => {
  const query = wholeViewQuery(descriptor as never);
  assert.deepEqual(query.summary.unavailableActions, unavailableActions);
  assert.equal(query.summary.types[0].capabilityCounts.play, 201);assert.equal(query.summary.every.edit, true);
  assert.equal('query' in query.selector && query.selector.query.selectionBasis, basis);
});
test('web exclusions replace only already offered callbacks with concise reasons', () => {
  const callbacks = {onPlay() {}, onQueue() {}, onEdit() {}, onMyList() {}};
  const actions = selectionUnavailableActions(callbacks, unavailableActions);
  assert.equal(actions.onPlay, undefined);assert.equal(actions.onQueue, undefined);assert.equal(actions.onEdit, undefined);
  assert.equal(actions.playDisabledReason, defaultI18n.t('next.selection.unsupportedSort'));
  assert.equal(actions.queueDisabledReason, actions.playDisabledReason);
  assert.equal(actions.editDisabledReason, defaultI18n.t('next.selection.previewLimit'));
  assert.equal(actions.onMyList, callbacks.onMyList);assert.equal(callbacks.onPlay instanceof Function, true);
  assert.deepEqual(selectionUnavailableActions({}, unavailableActions), {});
  assert.deepEqual(selectionUnavailableActions(callbacks, undefined), callbacks);
});
test('actual desktop and phone controls show unavailable Play/Edit and explain Queue without invoking stale callbacks', async () => {
  const host = createRenderer();Object.assign(host.react, {useId: () => 'exclusion'});
  const {SelectionBar, PhoneActionBar} = await componentModule(new URL('../src/kit/selection/SelectionBar.tsx', import.meta.url), {
    react: host.react, '@i18n': {defaultI18n}, '../controls': {Button: 'Button'}, '../Icon': {Icon: 'Icon'},
    '../overlays': {Menu: 'Menu', MenuGroup: 'MenuGroup', MenuItem: 'MenuItem'}, '../overlays/shared': {usePhone: () => false},
    './copy': {selectionText: (key: string, values: any) => defaultI18n.t(key as never, values)}, './presentation': {selectionCountText: () => '201 movies'}, './selection.module.css': {default: {}},
  });
  let calls = 0;const reason = defaultI18n.t('next.selection.unsupportedSort'), preview = defaultI18n.t('next.selection.previewLimit');
  const actions = {onPlay() {calls++;}, onQueue() {calls++;}, onEdit() {calls++;}, playDisabledReason: reason, queueDisabledReason: reason, editDisabledReason: preview};
  try {
    for (const component of [() => SelectionBar({selection: {count}, actions, onExit() {}}), () => PhoneActionBar({selection: {count}, actions})]) {
      const nodes = tree(host.render(component));
      const play = nodes.find((node: any) => node.type === 'Button' && node.props.title === reason);
      assert(play);assert.equal(play.props.disabled, true);assert.equal(play.props.onClick, undefined);
      const queue = nodes.find((node: any) => node.type === 'Menu' && node.props.items?.[0]?.id === 'next');assert(queue);
      for (const item of queue.props.items) {assert.equal(item.disabled, true);assert.equal(item.description, reason);item.onSelect();}
      const edit = nodes.find((node: any) => node.type === 'Button' && node.props.title === preview);
      const editRow = nodes.flatMap((node: any) => node.type === 'Menu' ? node.props.items ?? [] : []).find((item: any) => item.id === 'edit');
      if (edit) {assert.equal(edit.props.disabled, true);assert.equal(edit.props.onClick, undefined);}
      else {assert(editRow);assert.equal(editRow.disabled, true);assert.equal(editRow.description, preview);editRow.onSelect();}
    }
    assert.equal(calls, 0);
  } finally {host.dispose();}
});
test('metadata preview exclusion refuses before requests; small previews retain the existing proof-bearing query API', async () => {
  const descriptorQuery = wholeViewQuery(descriptor as never);
  const target = {mode: 'query', selector: descriptorQuery.selector, total: count, sourceTotal: count, summary: {count, types: [{kind: 'movie', count}], every, library: {kind: 'single', libraryId: 'lib'}, unavailableActions}, refinement: {kind: null, excludedItems: []}, expected: {catalogRevision: revision}};
  const requests: any[] = [];const stop = new Error('captured-preview-query');
  const producer = createQuerySelectionEditor({api: {request: async (...args: any[]) => {requests.push(args);throw stop;}}, serverId: 's', viewerFence: () => 'f'.repeat(64), requestId: async () => 'request', isCurrent: () => true} as never);
  await assert.rejects(producer.preview({target} as never), /query-editor-unavailable/);assert.equal(requests.length, 0);
  const small = {...target, total: 200, sourceTotal: 200, summary: {...target.summary, count: 200, types: [{kind: 'movie', count: 200}], unavailableActions: undefined}};
  await assert.rejects(producer.preview({target: small} as never), error => error === stop);
  assert.equal(requests.length, 1);assert.deepEqual(requests[0][2], {selector: descriptorQuery.selector, expected: {catalogRevision: revision}});
});
