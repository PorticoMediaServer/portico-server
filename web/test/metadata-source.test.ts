/**
 * Edit library › Metadata source: picking the other source asks first (with the copy for that
 * direction) and saves on the revision it was read at. When only the library's revision moved in
 * between, the choice is sent again by itself; when the source itself was changed elsewhere, the
 * question closes and the new setting is shown.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule, hooks} from './helpers/component-harness.mjs';
import * as agentModule from '../src/admin/metadata-agent.ts';
import {ALREADY_DONE, sendWithFreshRecord} from '../../packages/client-core/src/freshness-actions.ts';

function nodes(tree: any): any[] { if (!tree || typeof tree !== 'object') return []; if (Array.isArray(tree)) return tree.flatMap(nodes); return [tree, ...nodes(tree.props?.children)]; }
const answer = (agent: string, revision: number) => ({libraryId: 'lib', libraryKind: 'movie', revision, agent, agents: [{id: 'online', name: 'Portico online metadata', description: 'Online.', providers: ['tmdb']}, {id: 'local', name: 'Local metadata only', description: 'Local.', providers: []}]});

async function mount(put: (body: unknown, method?: string) => Promise<unknown>) {
  const h = hooks();
  let reloads = 0, saved = 0;
  const app = await componentModule(new URL('../src/screens/server/MetadataSource.tsx', import.meta.url), {
    react: h.react,
    '../../admin/metadata-agent': agentModule,
    '../../admin/console': {problem: (e: any) => `problem:${e?.code}`, unconfigured: (p: any) => p, useRead: () => ({}), sendFresh: async (record: any, send: any, reread: any, applies: any) => { const out = await sendWithFreshRecord(record, send, {reread, applies}); return out === ALREADY_DONE ? undefined : out; }},
    '../../admin/metadata-screen': {lookupStatus: () => 'on', saveLookup: async () => {}},
    '../../app/session': {useSession: () => ({api: {request: async (_p: string, method: string, body: unknown) => put(body, method)}})},
    '../../app/i18n': {useI18n: () => defaultI18n},
    '../../ui': {Checkbox: 'Checkbox', ConfirmDialog: 'ConfirmDialog', Notice: 'Notice', SettingsGroup: 'SettingsGroup', Text: 'Text'},
  });
  const read = {data: agentModule.parseMetadataAgent(answer('online', 5)), loading: false, reload: () => { reloads++; }};
  const render = () => h.render(() => app.MetadataSourceGroup({libraryId: 'lib', agent: read, screen: null, onSaved: () => { saved++; }}));
  return {render, counts: () => ({reloads, saved})};
}

test('choosing local asks with the local copy, then saves on the read revision', async () => {
  const bodies: unknown[] = [];
  const {render, counts} = await mount(async body => { bodies.push(body); return answer('local', 6); });
  let tree = render();
  const [online, local] = nodes(tree).filter(n => n.type === 'Checkbox');
  assert.equal(online.props.checked, true);
  assert.equal(local.props.label, 'Local metadata only');
  local.props.onCheckedChange(true);
  tree = render();
  const dialog = nodes(tree).find(n => n.type === 'ConfirmDialog');
  assert.equal(dialog.props.open, true);
  assert.equal(dialog.props.title, 'Stop looking this library up online?');
  assert.equal(dialog.props.body, 'Titles and artwork already found stay until you refresh them. New titles will use only your files.');
  await dialog.props.onConfirm();
  assert.deepEqual(bodies, [{expectedRevision: 5, agent: 'local'}]);
  assert.deepEqual(counts(), {reloads: 1, saved: 1});
  assert.equal(nodes(render()).find(n => n.type === 'ConfirmDialog').props.open, false);
});

test('when only the library moved, the choice is sent again against it without a word', async () => {
  const bodies: unknown[] = [];
  const {render, counts} = await mount(async (body, method) => {
    if (!method || method === 'GET') return answer('online', 7);
    bodies.push(body);
    if ((body as {expectedRevision: number}).expectedRevision !== 7) throw Object.assign(new Error('changed'), {status: 409, code: 'metadata_conflict'});
    return answer('local', 8);
  });
  render();
  nodes(render()).filter(n => n.type === 'Checkbox')[1].props.onCheckedChange(true);
  await nodes(render()).find(n => n.type === 'ConfirmDialog').props.onConfirm();
  const tree = render();
  assert.deepEqual(bodies, [{expectedRevision: 5, agent: 'local'}, {expectedRevision: 7, agent: 'local'}]);
  assert.deepEqual(counts(), {reloads: 1, saved: 1});
  assert.equal(nodes(tree).find(n => n.type === 'ConfirmDialog').props.open, false);
  assert.ok(!nodes(tree).some(n => n.type === 'Notice' && String(n.props.children).includes('metadata settings changed')));
});

test('when the source itself was changed elsewhere, the question closes, reloads and says so; nothing more is sent', async () => {
  let puts = 0;
  // Someone else moved this library to another source in the meantime (a third one, so it is neither what was shown nor what was asked for).
  const {render, counts} = await mount(async (_body, method) => { if (!method || method === 'GET') return {...answer('online', 7), agent: 'custom', agents: [...answer('online', 7).agents, {id: 'custom', name: 'Another source', description: 'Other.', providers: []}]}; puts++; throw Object.assign(new Error('changed'), {status: 409, code: 'metadata_conflict'}); });
  render();
  nodes(render()).filter(n => n.type === 'Checkbox')[1].props.onCheckedChange(true);
  await nodes(render()).find(n => n.type === 'ConfirmDialog').props.onConfirm();
  const tree = render();
  assert.equal(puts, 1);
  assert.equal(counts().reloads, 1);
  assert.equal(nodes(tree).find(n => n.type === 'ConfirmDialog').props.open, false);
  assert.ok(nodes(tree).some(n => n.type === 'Notice' && String(n.props.children).includes('metadata settings changed')));
});

test('an invalid source keeps the question open with the error', async () => {
  const {render} = await mount(async () => { throw Object.assign(new Error('bad'), {status: 400, code: 'invalid_metadata_source'}); });
  render();
  nodes(render()).filter(n => n.type === 'Checkbox')[1].props.onCheckedChange(true);
  await nodes(render()).find(n => n.type === 'ConfirmDialog').props.onConfirm();
  const dialog = nodes(render()).find(n => n.type === 'ConfirmDialog');
  assert.equal(dialog.props.open, true);
  assert.equal(dialog.props.error, 'problem:invalid_metadata_source');
});
