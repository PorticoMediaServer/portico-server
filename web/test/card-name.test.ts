import test from 'node:test';
import assert from 'node:assert/strict';
import {defaultI18n} from '@i18n';
import {componentModule} from './helpers/component-harness.mjs';

type Node = {type: unknown; props: Record<string, any> & {children?: unknown}};
const react = {memo: (f: unknown) => f, useRef: (v: unknown) => ({current: v}), createElement: (type: unknown, props: object, ...children: unknown[]) => ({type, props: {...props, children}})};
const card = () => componentModule(new URL('../src/ui/Card.tsx', import.meta.url), {
  react: {default: react, ...react},
  './i18n': {uiI18n: () => defaultI18n},
  './cx': {cx: (...parts: unknown[]) => parts.filter(Boolean).join(' ')},
  './Artwork': {Artwork: 'artwork'}, './Icon': {Icon: 'icon'}, './Menu': {anchorAt: () => ({}), anchorOf: () => ({})},
  './Card.module.css': {default: new Proxy({}, {get: (_, key) => String(key)})},
}) as Promise<typeof import('../src/ui/Card.tsx')>;
const find = (node: unknown, match: (n: Node) => boolean): Node | undefined => {
  if (Array.isArray(node)) { for (const child of node) { const hit = find(child, match); if (hit) return hit; } return undefined; }
  if (!node || typeof node !== 'object') return undefined;
  const n = node as Node;
  return match(n) ? n : find(n.props?.children, match);
};
const focusTarget = (tree: unknown) => find(tree, n => n.props?.className === 'artButton')!;

test('a card’s name carries what its art shows: watched and how far in', async () => {
  const {cardName} = await card();
  assert.equal(cardName('Dune', {}), 'Dune');
  assert.equal(cardName('Dune', {progress: 0}), 'Dune');
  assert.equal(cardName('Dune', {watched: true}), 'Dune, Watched');
  assert.equal(cardName('Severance', {progress: 0.4}), 'Severance, 40% played');
  assert.equal(cardName('Severance', {watched: true, progress: 0.256}), 'Severance, Watched, 26% played', 'a rewatch shows both the tick and the bar');
  assert.equal(cardName('Severance', {progress: 7}), 'Severance, 100% played');
});

test('the focused card is named with its state, and the tick is not a second, mouse-only source', async () => {
  const {Card} = await card();
  const render = (props: object) => (Card as unknown as (p: object) => unknown)({title: 'Dune', href: '/media/1', onOpen: () => {}, ...props});
  assert.equal(focusTarget(render({})).props['aria-label'], 'Dune');
  const watched = render({watched: true, progress: 0.5});
  assert.equal(focusTarget(watched).props['aria-label'], 'Dune, Watched, 50% played');
  assert.equal(find(watched, n => n.props?.className === 'watched')!.props['aria-hidden'], true);
  // The name still starts with the visible title, so voice control and "find by name" keep working.
  assert.ok(String(focusTarget(watched).props['aria-label']).startsWith('Dune'));
});
