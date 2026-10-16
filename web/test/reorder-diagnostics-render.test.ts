/**
 * Render checks for two new owner screens, through the component harness (hook storage, not a
 * browser): a long library collection's reorder list, and the record of parts this app left out.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {componentModule, hooks} from './helpers/component-harness.mjs';
import {noteSkipped, resetSkippedParts} from '../../packages/client-core/src/tolerant.ts';

const t = (key: string, values?: Record<string, unknown>) => (values ? `${key} ${JSON.stringify(values)}` : key);
type Node = {type: unknown; props: Record<string, any>};
const find = (node: unknown, match: (n: Node) => boolean): Node[] => {
  if (!node || typeof node !== 'object') return [];
  if (Array.isArray(node)) return node.flatMap(child => find(child, match));
  const n = node as Node;
  return [...(match(n) ? [n] : []), ...find(n.props?.children, match)];
};

test('a long collection is arranged in a windowed list: a move sends the title and its new neighbour, and shows at once', async () => {
  const h = hooks();
  const mod = await componentModule(new URL('../src/screens/shared/ManualOrder.tsx', import.meta.url), {
    react: h.react, '../../app/i18n': {useI18n: () => ({t})}, '../../ui': {WindowedGrid: 'WindowedGrid', Button: 'Button', Icon: 'Icon', ListRow: 'ListRow', Menu: 'Menu', cx: () => ''},
    '../../app/content': {cardCaptionOf: () => ''}, './ManualOrder.module.css': {default: {}},
  }) as any;
  // 5,000 titles, of which only a window around the top and the moved one are loaded.
  const total = 5000, loaded = new Set([0, 1, 2, 3, 4, 3000]);
  const item = (i: number) => ({id: `t${i}`, title: `Title ${i}`, kind: 'movie'});
  const collection = {subscribe: () => () => {}, getSnapshot: () => ({generation: 1, total, totalIsLowerBound: false}), itemAt: (i: number) => (loaded.has(i) ? item(i) : undefined)};
  const sent: unknown[] = [];
  const onMove = (request: unknown) => { sent.push(request); return Promise.resolve(); };
  const draw = () => h.render(() => mod.WindowedOrderList({collection, label: 'Collection', onMove}));
  let tree = draw();
  const grid = find(tree, n => n.type === 'WindowedGrid')[0]!;
  assert.equal(grid.props.collection, collection, 'the list draws from the collection itself, a window at a time');
  // "To the bottom" is offered only once the last title has been read.
  const row = (index: number) => grid.props.renderItem(item(index), index) as Node;
  // The row component drawn out: its list row carries the menu of moves as its actions.
  const menuOf = (index: number) => { const element = row(index), drawn = (element.type as (p: unknown) => Node)(element.props); return find(drawn, n => n.type === 'ListRow')[0]!.props.actions.props.items.map((x: {id: string}) => x.id); };
  assert.deepEqual(menuOf(3000), ['top', 'up', 'down']);
  // Move to top from deep in the list.
  row(3000).props.onMove(3000, 0);
  assert.deepEqual(sent, [{entryId: 't3000', afterEntryId: ''}]);
  tree = draw();
  const after = find(tree, n => n.type === 'WindowedGrid')[0]!;
  const shown = (index: number) => (after.props.renderItem(item(index), index) as Node).props.media.id;
  assert.deepEqual([shown(0), shown(1), shown(2)], ['t3000', 't0', 't1'], 'the moved title shows in its new place before the list is read again');
});

test('the record of parts this app left out is listed newest first, and is empty normally', async () => {
  const h = hooks();
  const mod = await componentModule(new URL('../src/screens/server/Diagnostics.tsx', import.meta.url), {
    react: h.react, '../../app/i18n': {useI18n: () => ({t})}, '../../ui': {SettingsGroup: 'SettingsGroup', SettingsRow: 'SettingsRow', Button: 'Button', Text: 'Text'},
  }) as any;
  resetSkippedParts();
  let tree = h.render(() => mod.SkippedPartsGroup());
  assert.deepEqual(find(tree, n => n.type === 'SettingsRow').map(r => r.props.label), ['settings.server.skippedPartsNone']);
  assert.equal(tree.props.action, undefined, 'nothing to clear');
  noteSkipped('saved', 'entries', 3);
  noteSkipped('home', 'rows');
  tree = h.render(() => mod.SkippedPartsGroup());
  const rows = find(tree, n => n.type === 'SettingsRow');
  assert.deepEqual(rows.map(r => [r.props.label, r.props.help]), [['home', 'rows'], ['saved', 'entries']]);
  assert.equal(find(rows[1]!.props.meta, n => n.type === 'Text')[0]!.props.children[0], 'settings.server.skippedPartsCount {"count":3}');
  tree.props.action.props.onClick();
  tree = h.render(() => mod.SkippedPartsGroup());
  assert.deepEqual(find(tree, n => n.type === 'SettingsRow').map(r => r.props.label), ['settings.server.skippedPartsNone']);
});
