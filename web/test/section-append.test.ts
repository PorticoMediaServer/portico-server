import test from 'node:test';
import assert from 'node:assert/strict';
import {accumulatePage, mergeSectionPages, SECTION_WINDOW_AT} from '../src/screens/shared/section-pages.ts';
import type {ContentSection} from '@core/library-content.ts';

const entry = (id: string) => ({id, kind: 'movie', title: id} as const);
const section = (id: string, ids: readonly string[], nextCursor: string, totalCount = 5000): ContentSection => ({
  id, type: 'grid', heading: {key: id, fallback: id}, entries: ids.map(entry) as unknown as ContentSection['entries'], totalCount, nextCursor,
});

test('PERF-S02: a full projection replaces every section', () => {
  const previous = {routeKey: 'artist', sections: [section('releases', ['a1'], 'c1', 12), section('songs', ['t0'], 's1', 5000)]};
  const incoming = [section('releases', ['a1'], '', 12), section('songs', ['t0'], '', 5000)];
  const merged = mergeSectionPages(previous, 'artist', null, incoming);
  assert.equal(merged.sections.length, 2);
  assert.deepEqual(merged.sections.map(s => s.id), ['releases', 'songs']);
});

test('PERF-S02: three "More" presses append pages 1-4 and keep the other sections', () => {
  const route = 'artist:1';
  let state = mergeSectionPages(null, route, null, [section('releases', ['a1', 'a2'], '', 12), section('songs', ['t0', 't1'], 'c1', 5000)]);
  assert.equal(state.sections.length, 2);
  const pages = [['t2', 't3'], ['t4', 't5'], ['t6', 't7']];
  const cursors = ['c2', 'c3', ''];
  pages.forEach((ids, i) => {
    state = mergeSectionPages(state, route, ['c1', 'c2', 'c3'][i]!, [section('songs', ids, cursors[i]!, 5000)]);
  });
  // The other section is still rendered.
  assert.deepEqual(state.sections.map(s => s.id), ['releases', 'songs']);
  assert.deepEqual(state.sections[0]!.entries.map(e => e.id), ['a1', 'a2']);
  // The section holds pages 1-4 in order.
  assert.deepEqual(state.sections[1]!.entries.map(e => e.id), ['t0', 't1', 't2', 't3', 't4', 't5', 't6', 't7']);
  assert.equal(state.sections[1]!.totalCount, 5000);
  assert.equal(state.sections[1]!.nextCursor, '');
});

test('PERF-S02: re-sent entries do not duplicate, a new route starts over', () => {
  let state = mergeSectionPages(null, 'artist:1', null, [section('songs', ['t0'], 'c1', 3)]);
  state = mergeSectionPages(state, 'artist:1', 'c1', [section('songs', ['t0', 't1'], '', 3)]);
  assert.deepEqual(state.sections[0]!.entries.map(e => e.id), ['t0', 't1']);
  state = mergeSectionPages(state, 'artist:2', 'c1', [section('songs', ['u0'], '', 9)]);
  assert.deepEqual(state.sections[0]!.entries.map(e => e.id), ['u0']);
});

test('PERF-S02: cursor pages accumulate in order and dedupe (saved-list helper)', () => {
  const store = {scope: '', pages: new Map<string, readonly string>()};
  const keyOf = (s: string) => s;
  assert.deepEqual(accumulatePage(store, 'playlist:1', null, ['e0', 'e1'], keyOf), ['e0', 'e1']);
  assert.deepEqual(accumulatePage(store, 'playlist:1', 'c1', ['e1', 'e2'], keyOf), ['e0', 'e1', 'e2']);
  assert.deepEqual(accumulatePage(store, 'playlist:1', 'c2', ['e3'], keyOf).length, 4);
  // Entry 1,000 stays reachable: 25 pages of 40 accumulate.
  let all: readonly string[] = [];
  const big = {scope: '', pages: new Map<string, readonly string>()};
  for (let p = 0; p < 25; p++) {
    const ids = Array.from({length: 40}, (_, i) => `e${p * 40 + i}`);
    all = accumulatePage(big, 'playlist:big', p === 0 ? null : `c${p}`, ids, keyOf);
  }
  assert.equal(all.length, 1000);
  assert.equal(all[999], 'e999');
});

test('PERF-S02: large sections window (threshold is above the 4-page acceptance)', () => {
  // The acceptance walks 4 pages (160 entries); windowing only cuts in past 6 pages.
  assert.ok(SECTION_WINDOW_AT > 160);
});

test('Paging through N entries keys each entry once, and an in-place page still replaces its segment', () => {
  const store = {scope: '', pages: new Map<string, readonly {id: string}[]>()};
  let calls = 0;
  const keyOf = (e: {id: string}) => { calls++; return e.id; };
  const page = (n: number) => Array.from({length: 100}, (_, i) => ({id: `e${n * 100 + i}`}));
  let all = accumulatePage(store, 's', null, page(0), keyOf);
  for (let n = 1; n < 400; n++) all = accumulatePage(store, 's', `c${n}`, page(n), keyOf);
  assert.equal(all.length, 40_000);
  assert.equal(calls, 40_000, 'each entry keyed once, not once per later page');
  // A page that changed in place replaces its own segment (a full rebuild).
  all = accumulatePage(store, 's', 'c1', [{id: 'x'}], keyOf);
  assert.equal(all.length, 39_901);
  assert.ok(all.some(e => e.id === 'x') && !all.some(e => e.id === 'e150'));
});
