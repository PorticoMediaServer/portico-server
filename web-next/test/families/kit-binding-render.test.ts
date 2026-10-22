import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {transformTsx} from '../transform-tsx.mjs';
import type {ActivityPanelView} from '../../src/kit/shell/Activity';

registerHooks({load(url, context, next) {
  if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
  if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
  return next(url, context);
}});
const {ActivityPanel, TopBarActivity} = await import('../../src/kit/shell/Activity.tsx');
const {CardFrame} = await import('../../src/kit/cards/CardFrame.tsx');
const {SongRow} = await import('../../src/kit/browse/TrackTable.tsx');
const idle = (): ActivityPanelView => ({serverName: 'Actual server', phase: 'loading', streams: [], scans: [], tasks: [], onRetry() {}, onDashboard() {}});

test('cold Activity has no guessed health, stream count, quiet state or update dot', () => {
  for (const phase of ['loading', 'error'] as const) {
    const view = {...idle(), phase, health: 'normal' as const, streamCount: 0, taskCount: 0, update: {available: true, onOpen() {}}};
    const html = renderToStaticMarkup(createElement(ActivityPanel, {viewerRole: 'owner', view}));
    assert.doesNotMatch(html, /Running normally|No streams|0 streams|Nothing playing or scanning|Server update available/);
    const trigger = renderToStaticMarkup(createElement(TopBarActivity, {viewerRole: 'owner', view, open: false, onOpenChange() {}}));
    assert.match(trigger, /aria-label="Server activity"/);
    assert.doesNotMatch(trigger, /streamCount|updateDot|No streams|0 streams|data-busy/);
  }
});

test('known ready zero remains a real zero; missing ready facts remain absent', () => {
  const known = {...idle(), phase: 'ready' as const, health: 'normal' as const, streamCount: 0, taskCount: 0};
  const html = renderToStaticMarkup(createElement(ActivityPanel, {viewerRole: 'owner', view: known}));
  assert.match(html, /Running normally/); assert.match(html, /No streams/); assert.match(html, /Nothing playing or scanning/);
  const unknown = renderToStaticMarkup(createElement(ActivityPanel, {viewerRole: 'owner', view: {...idle(), phase: 'ready'}}));
  assert.doesNotMatch(unknown, /Running normally|No streams|Nothing playing or scanning/);
  assert.equal(renderToStaticMarkup(createElement(TopBarActivity, {viewerRole: 'member', view: known, open: false, onOpenChange() {}})), '');
});

test('a card has one real More button with the supplied expanded state', () => {
  const html = renderToStaticMarkup(createElement(CardFrame, {model: {title: 'Actual title', caption: []}, kind: 'poster', onNavigate() {}, onMore() {}, moreExpanded: true, children: createElement('span', {}, 'Actual artwork')}));
  assert.equal((html.match(/aria-haspopup="menu"/g) ?? []).length, 1);
  assert.match(html, /aria-expanded="true"/);
  assert.equal((html.match(/<button/g) ?? []).length, 2);
});

test('the search SongRow owns its search geometry and omits the ordinal', () => {
  const row = {id: 'song', number: 42, text: {id: 'song', title: 'Actual song', caption: [], detail: []}, artwork: createElement('img', {alt: '', src: '/actual-art'})};
  const search = renderToStaticMarkup(createElement(SongRow, {row, context: 'search', onPlay() {}}));
  assert.match(search, /data-context="search"/);
  assert.match(search, /songArtworkPlay/);
  assert.doesNotMatch(search, /songNumber|>42</);
  const list = renderToStaticMarkup(createElement(SongRow, {row, onPlay() {}}));
  assert.match(list, /songNumber/); assert.match(list, />42</);
});

test('the card owns one named selection control around a decorative circle', () => {
  const html = renderToStaticMarkup(createElement(CardFrame, {model: {title: 'Actual title', caption: []}, kind: 'poster', onNavigate() {}, onMore() {}, selection: {active: true, checked: true, control: createElement('span', {'aria-hidden': true}, 'circle'), onToggle() {}, onExit() {}}, children: createElement('span', {}, 'Actual artwork')}));
  assert.equal((html.match(/<button/g) ?? []).length, 3);
  assert.match(html, /class="selection"[^>]*aria-label="[^"]*Actual title/);
  assert.match(html, /class="selection"[^>]*aria-pressed="true"/);
  assert.doesNotMatch(html, /<button[^>]*>(?:(?!<\/button>).)*<button/s);
});
