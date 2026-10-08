/**
 * Edit library › Metadata source: picking the other source asks first (with the copy for that
 * direction), saves on the revision it was read at, and a conflict reloads instead of retrying.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule, hooks} from './helpers/component-harness.mjs';
import * as agentModule from '../src/admin/metadata-agent.ts';

function nodes(tree: any): any[] { if (!tree || typeof tree !== 'object') return []; if (Array.isArray(tree)) return tree.flatMap(nodes); return [tree, ...nodes(tree.props?.children)]; }
const answer = (agent: string, revision: number) => ({libraryId: 'lib', libraryKind: 'movie', revision, agent, agents: [{id: 'online', name: 'Portico online metadata', description: 'Online.', providers: ['tmdb']}, {id: 'local', name: 'Local metadata only', description: 'Local.', providers: []}]});

async function mount(put: (body: unknown) => Promise<unknown>) {
  const h = hooks();
  let reloads = 0, saved = 0;
  const app = await componentModule(new URL('../src/screens/server/MetadataSource.tsx', import.meta.url), {
    react: h.react,
    '../../admin/metadata-agent': agentModule,
    '../../admin/console': {problem: (e: any) => `problem:${e?.code}`, unconfigured: (p: any) => p, useRead: () => ({})},
    '../../admin/metadata-screen': {lookupStatus: () => 'on', saveLookup: async () => {}},
    '../../app/session': {useSession: () => ({api: {request: async (_p: string, _m: string, body: unknown) => put(body)}})},
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

test('a conflict closes the question, reloads and says the setting changed; nothing is retried', async () => {
  let puts = 0;
  const {render, counts} = await mount(async () => { puts++; throw Object.assign(new Error('changed'), {status: 409, code: 'metadata_conflict'}); });
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
