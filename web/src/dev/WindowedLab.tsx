import {useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {createWindowedCollection, type PageResult} from '@core/collections/index.ts';
import type {ContentEntry} from '@core/library-content.ts';
import {Card, Skeleton, WindowedGrid, gridColumnMin, type WindowedGridHandle} from '../ui';
import {AlphabetRail} from '../screens/library/BrowseFilters';
import b from '../screens/library/BrowseFilters.module.css';

/**
 * Dev-only scale lab (kept outside screens/: it's a harness, not product UI) (`/gallery/windowed?total=1000000`): the Browse grid over
 * a mocked collection of any size, to measure resident DOM nodes and memory
 * (scale rule: client cost is O(visible)). The mock answers each page after
 * 30 ms with synthetic entries (no artwork requests).
 */
const LETTERS = '#ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('');

export function WindowedLab() {
  const total = Math.max(0, Math.min(10_000_000, Number(new URLSearchParams(location.search).get('total') ?? 1_000_000) || 0));
  const collection = useMemo(() => createWindowedCollection<ContentEntry>({
    pageSize: 60, maxResidentPages: 10, keyOf: e => e.id,
    fetchPage: (start, count, signal) => new Promise<PageResult<ContentEntry>>((resolve, reject) => {
      const t = setTimeout(() => {
        const items: ContentEntry[] = [];
        for (let i = start; i < Math.min(total, start + count); i++) items.push({id: `m${i}`, kind: 'movie', title: `Title ${i.toLocaleString('en-US')}`, subtitle: String(1950 + (i % 75))});
        resolve({items, total, positionIndex: Object.fromEntries(LETTERS.map((l, k) => [l, Math.floor((total / LETTERS.length) * k)]))});
      }, 30);
      signal.addEventListener('abort', () => { clearTimeout(t); reject(Object.assign(new Error('aborted'), {name: 'AbortError'})); }, {once: true});
    }),
  }), [total]);
  useEffect(() => () => collection.dispose(), [collection]);
  const snap = useSyncExternalStore(collection.subscribe, collection.getSnapshot);
  const grid = useRef<WindowedGridHandle>(null);
  const [first, setFirst] = useState(0);
  const anchors = useMemo(() => Object.entries(snap.positionIndex ?? {}).map(([key, index]) => ({key, index})), [snap.positionIndex]);
  const current = [...anchors].reverse().find(a => a.index <= first)?.key;
  // For scripted measurement.
  (window as Window & {__lab?: unknown}).__lab = {collection, grid, resident: () => collection.resident()};
  return (
    <div style={{padding: '24px 0'}}>
      <h1 style={{padding: '0 var(--page-gutter)'}}>Windowed grid lab · {total.toLocaleString('en-US')} items · first visible · {first.toLocaleString('en-US')}</h1>
      <div className={b.layout} style={{paddingRight: 0}}>
        <WindowedGrid
          ref={grid}
          collection={collection}
          columnMin={gridColumnMin('poster')}
          estimateRowHeight={300}
          label="Lab"
          onFirstVisible={setFirst}
          renderItem={entry => <Card title={entry.title} caption={entry.subtitle} shape="poster" icon="film" />}
          renderPlaceholder={() => <div aria-hidden><Skeleton style={{aspectRatio: '2 / 3', width: '100%'}} /><Skeleton height={14} width="70%" style={{marginTop: 8}} /></div>}
        />
        <AlphabetRail index={anchors} current={current} onJump={key => { const i = collection.indexOfLetter(key); if (i !== undefined) grid.current?.scrollToIndex(i); }} />
      </div>
    </div>
  );
}

/**
 * Scripted scale measurement: 200 random jumps, small 1:1 steps, the end, and a
 * letter jump; reports DOM nodes, rendered cells, resident pages/items and JS
 * heap. Scroll events are dispatched by hand so it also runs in a hidden tab.
 * Run from the console: `await window.__labMeasure()`.
 */
async function measure() {
  const sleep = (ms: number) => new Promise(r => setTimeout(r, ms));
  const w = window as Window & {__lab?: {collection: ReturnType<typeof createWindowedCollection<ContentEntry>>}; __labResult?: unknown};
  const lab = w.__lab!;
  const go = async (y: number, wait: number) => { window.scrollTo(0, y); window.dispatchEvent(new Event('scroll')); await sleep(wait); };
  const region = () => document.querySelector('[role=region][aria-label="Lab"]');
  const cells = () => region()?.firstElementChild?.childElementCount ?? -1;
  const heap = () => { const m = (performance as Performance & {memory?: {usedJSHeapSize: number}}).memory; return m ? Math.round(m.usedJSHeapSize / 104857.6) / 10 : null; };
  const shown = () => [...(region()?.firstElementChild?.querySelectorAll('[title]') ?? [])].map(e => Number((e.getAttribute('title') ?? '').replace(/\D/g, '')));
  const first = () => Number((document.querySelector('h1')?.textContent ?? '').split('·').pop()!.replace(/\D/g, ''));
  const nodes = () => document.getElementsByTagName('*').length;
  const start = {nodes: nodes(), cells: cells(), resident: lab.collection.resident(), heapMB: heap(), scrollHeightPx: document.documentElement.scrollHeight};
  const max = {nodes: 0, cells: 0, residentItems: 0, residentPages: 0};
  for (let i = 0; i < 200; i++) {
    await go(Math.random() * (document.documentElement.scrollHeight - innerHeight), i % 10 === 0 ? 250 : 50);
    const r = lab.collection.resident();
    max.nodes = Math.max(max.nodes, nodes()); max.cells = Math.max(max.cells, cells()); max.residentItems = Math.max(max.residentItems, r.items); max.residentPages = Math.max(max.residentPages, r.pages);
  }
  await sleep(1000);
  const s1 = shown();
  const after = {nodes: nodes(), cells: cells(), resident: lab.collection.resident(), heapMB: heap(), firstVisible: first(), shown: s1.length ? [Math.min(...s1), Math.max(...s1)] : null};
  const f0 = first(); await go(scrollY + 300, 300); await go(scrollY + 300, 300);
  const small = {from: f0, to: first()};
  await go(document.documentElement.scrollHeight, 1200);
  const e = shown(); const end = {maxShown: e.length ? Math.max(...e) : null};
  (document.querySelector('nav[aria-label="Jump to letter"] button[title="Jump to M"]') as HTMLButtonElement | null)?.click();
  window.dispatchEvent(new Event('scroll')); await sleep(300); window.dispatchEvent(new Event('scroll')); await sleep(1200);
  const j = shown(); const jump = {indexOfM: lab.collection.indexOfLetter('M'), firstVisible: first(), shownMin: j.length ? Math.min(...j) : null};
  w.__labResult = {start, after, small, end, jump, max};
  return w.__labResult;
}
if (typeof window !== 'undefined') (window as Window & {__labMeasure?: typeof measure}).__labMeasure = measure;
