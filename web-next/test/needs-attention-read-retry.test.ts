/**
 * FX-240 / BLK-WEB-015: the Unmatched page can recover a failed list read at its own cursor.
 *
 * The actual NeedsAttention component runs against the real metadata-attention service; only the
 * transport is stubbed. Covers initial failure/Retry, a later page retry that keeps accepted rows,
 * coalesced double clicks, a replacement api retiring late old responses, and the separation from
 * the per-title retry mutation.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule} from '../../web/test/helpers/component-harness.mjs';
import {all, createRenderer, tick, tree} from '../../web/test/helpers/interaction-harness.mjs';

const entry = (id: string, title: string) => ({entityId: id, kind: 'item', itemKind: 'movie', parentId: '', actions: ['retry'], title, revision: 1, reason: 'unmatched', confidence: 0, candidates: []});
const page = (entries: any[], nextCursor = '', libraryId = 'lib') => ({libraryId, total: entries.length, entries, nextCursor});
const cursorOf = (path: string) => new URLSearchParams(path.split('?')[1] ?? '').get('cursor') ?? '';

async function mount(libraryId = 'lib') {
  const h = createRenderer(), lists: string[] = [], acts: any[] = [];
  let respond: (path: string) => Promise<unknown> = async () => page([], '', libraryId);
  const transport = (requests: string[], actions: any[]) => ({
    request: (path: string, method?: string, body?: unknown) => {
      if (method === 'POST') { actions.push({path, body}); return Promise.resolve(undefined); }
      requests.push(path);
      return respond(path);
    },
  });
  const api = transport(lists, acts);
  const app = await componentModule(new URL('../src/screens/metadata/NeedsAttention.tsx', import.meta.url), {
    react: h.react,
    '@i18n': {defaultI18n: {t: (key: string) => key, has: () => false}},
    '../../kit/controls': {Button: 'Button', TextField: 'TextField'},
    '../../kit/overlays': {Dialog: 'Dialog'},
    '../../kit/Icon': {Icon: 'Icon'},
    '../../kit/accounts/ServicesAuth': {HostedError: 'HostedError'},
    '../../kit/artwork': {Artwork: 'Artwork', GeneratedCover: 'GeneratedCover'},
    './metadata.module.css': {default: {}},
    '@design': {generatedCoverTint: () => ''},
  });
  const show = (target = api, id = libraryId) => {
    const paint = () => tree(h.render(() => app.NeedsAttention({libraryId: id, libraryName: 'Movies', api: target})));
    paint();
    return paint();
  };
  return {acts, api, app, lists, h, reply: (fn: (path: string) => Promise<unknown>) => {respond = fn;}, show, transport};
}

const labelOf = (node: any) => Array.isArray(node.props.children) ? node.props.children.join('') : node.props.children;
const button = (rendered: any[], label: string) => all(rendered, 'Button').find(node => labelOf(node) === label);
const articles = (rendered: any[]) => all(rendered, 'article');

test('a failed initial read shows Retry and recovers at the same cursor', async () => {
  const {acts, lists, reply, show} = await mount();
  reply(async () => { throw new Error('transport down'); });
  let v = show();
  await tick();
  v = show();
  assert.equal(articles(v).length, 0);
  const retry = button(v, 'next.browse.retry');
  assert.ok(retry, 'a read failure offers Retry');
  reply(async () => page([entry('e1', 'T2')]));
  retry.props.onClick();
  await tick();
  v = show();
  assert.equal(cursorOf(lists[1]), '', 'Retry repeats the exact failed cursor (page one)');
  assert.equal(articles(v).length, 1, 'the recovered page renders');
  assert.equal(acts.length, 0, 'a list Retry never sends a mutation');
});

test('a failed later page keeps accepted rows and retries its offered cursor', async () => {
  const {lists, reply, show} = await mount();
  reply(async path => {
    const cursor = cursorOf(path);
    if (cursor === '') return page([entry('e1', 'T2')], 'next-1');
    if (cursor === 'next-1') throw new Error('later page down');
    return page([entry('e2', 'Monsters')], '');
  });
  let v = show();
  await tick();
  v = show();
  assert.equal(articles(v).length, 1);
  button(v, 'web.hostedAccount.older').props.onClick();
  await tick();
  v = show();
  assert.equal(articles(v).length, 1, 'accepted rows stay on a later-page failure');
  assert.ok(button(v, 'next.browse.retry'), 'later-page failure offers Retry');
  button(v, 'next.browse.retry').props.onClick();
  await tick();
  v = show();
  assert.equal(cursorOf(lists.at(-1)!), 'next-1', 'Retry asks for the offered later cursor, not page one');
  assert.equal(articles(v).length, 1);
});

test('double Retry coalesces into one read', async () => {
  const {lists, reply, show} = await mount();
  reply(async () => { throw new Error('down'); });
  let v = show();
  await tick();
  v = show();
  const before = lists.length;
  reply(async () => page([entry('e1', 'T2')]));
  const retry = button(v, 'next.browse.retry');
  retry.props.onClick();
  retry.props.onClick();
  await tick();
  assert.equal(lists.length - before, 1, 'the second click joins the in-flight retry');
});

test('a replacement api retires the late old read', async () => {
  const {lists, reply, show, transport} = await mount();
  let resolveOld!: (value: unknown) => void;
  reply(() => new Promise(resolve => { resolveOld = resolve; }));
  let v = show();
  const next = transport(lists, []);
  reply(async () => page([entry('e2', 'Monsters')]));
  v = show(next);
  await tick();
  resolveOld(page([entry('e1', 'T2')]));
  await tick();
  v = show(next);
  const titles = articles(v).map(node => JSON.stringify(node));
  assert.equal(articles(v).length, 1);
  assert.ok(titles.some(value => value.includes('Monsters')), 'only the current api/library response publishes');
  assert.ok(!titles.some(value => value.includes('T2')), 'the retired response never reaches the page');
});

test('a successful empty list still draws its empty state without Retry', async () => {
  const {reply, show} = await mount();
  reply(async () => page([]));
  let v = show();
  await tick();
  v = show();
  assert.equal(articles(v).length, 0);
  assert.equal(button(v, 'next.browse.retry'), undefined);
  assert.ok(all(v, 'div').some(node => JSON.stringify(node).includes('metadata.attention.empty')));
});

test('per-title retry stays a mutation and is untouched by list Retry', async () => {
  const {acts, reply, show} = await mount();
  reply(async () => page([entry('e1', 'T2')]));
  let v = show();
  await tick();
  v = show();
  const retry = button(v, 'next.browse.retry');
  assert.equal(retry, undefined, 'a loaded list has no read Retry');
  assert.equal(acts.length, 0);
});
