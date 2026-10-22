import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {transformTsx} from '../transform-tsx.mjs';
registerHooks({load(url, context, next) {
  if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
  if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
  return next(url, context);
}});
const {SongRow, TrackTable} = await import('../../src/kit/browse/TrackTable.tsx');
const {FacetChips, MatchMode} = await import('../../src/kit/browse/FilterBuilder.tsx');
const row = {id: 'actual-song', number: 1, text: {id: 'actual-song', title: 'Actual song', caption: [], detail: []}};
test('a search selection control is independent of play, even without artwork', () => {
  for (const artwork of [undefined, createElement('img', {alt: '', src: '/actual-image'})]) {
    const html = renderToStaticMarkup(createElement(SongRow, {row: {...row, artwork}, context: 'search', selection: createElement('button', {'aria-label': 'Select actual song'}, 'Select'), onPlay() {}}));
    assert.match(html, /songSelection/);
    assert.doesNotMatch(html, /<button[^>]*>(?:(?!<\/button>).)*<button/s);
    assert.equal((html.match(/<button/g) ?? []).length, 2);
  }
});
test('exclusion has a distinct accessible state and known zero facet counts remain visible', () => {
  const html = renderToStaticMarkup(createElement(FacetChips, {values: [{value: 'a', label: 'Actual genre', count: 0}], included: [], excluded: ['a'], onValueChange() {}}));
  assert.match(html, /role="checkbox" aria-checked="mixed"/);
  assert.match(html, /<small>0<\/small>/);
  assert.match(html, /aria-label="Clear Actual genre"/);
});
test('unsupported match modes are omitted and sparse facts stay absent', () => {
  const match = renderToStaticMarkup(createElement(MatchMode, {value: 'any', allowAll: false, allowNone: false, onValueChange() {}}));
  assert.doesNotMatch(match, /All of these|None of these/);
  const html = renderToStaticMarkup(createElement(TrackTable, {rows: [row], variant: 'album', label: 'Actual album', onPlay() {}, onMore() {}}));
  assert.doesNotMatch(html, /Unknown|1:00|Invalid Date/);
  assert.match(html, /aria-rowcount="2"/);
  assert.throws(() => renderToStaticMarkup(createElement(TrackTable, {rows: [row], variant: 'songs', label: 'Actual songs', onPlay() {}, onMore() {}})), /Invalid controlled track window/);
});

test('range presets are exclusive controlled toggles rather than independent checkboxes', async () => {
  const {FilterBuilder} = await import('../../src/kit/browse/FilterBuilder.tsx');
  const html = renderToStaticMarkup(createElement(FilterBuilder, {
    model: {groups: [{id: 'file', fields: [{id: 'durationMs', labelKey: 'browse.field.durationSeconds', type: 'number', operators: ['at-most'], controlHint: 'number-range', complexity: 'standard', cost: 'indexed-join', applicableKinds: ['movie']}]}]},
    labels: {field: () => 'Duration'}, fields: {durationMs: {kind: 'range', value: 'short', options: [{value: 'short', label: 'Under 30 min'}]}}, openFields: ['durationMs'], onFieldOpenChange() {}, onFieldChange() {}, onSearchChange() {},
  }));
  assert.match(html, /aria-pressed="true"[^>]*>Under 30 min/);
  assert.doesNotMatch(html, /role="checkbox"/);
});
