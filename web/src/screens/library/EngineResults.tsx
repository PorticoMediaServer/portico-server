import React, {useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import type {WindowedCollection} from '@core/collections/index.ts';
import type {BrowseSortCapability, BrowseSortSelection} from '@core/browse.ts';
import {iconFor} from '@core/presentation/index.ts';
import {recallListWindow, saveListWindow} from '../../app/list-position';
import {useSelectionActions, useSelectionActive, useSelectionSection} from '../../app/selection';
import {ErrorNotice, ErrorState} from '../../app/errors';
import {useI18n} from '../../app/i18n';
import {useEmptyLibraryCopy} from '../shared/NoLibraries';
import {EntryCard} from '../shared/Sections';
import {Inset, Skeleton, StateView, WindowedGrid, createStaticCollection, gridColumnMin, type WindowedGridHandle} from '../../ui';
import {AlphabetRail} from './BrowseFilters';
import {TitlesTable} from './LibraryViews';
import type {SequenceContainer} from '../../app/container-play';
import b from './BrowseFilters.module.css';

const EMPTY: readonly ContentEntry[] = [];

/**
 * Browse results over a windowed collection: only the rows on screen (plus a
 * little overscan) are in the DOM and in memory, whatever the library's size.
 * The position rail jumps with the server's `positionIndex` (any sort that has
 * one; the rail stays hidden where the server sent no anchors).
 *
 * PERF-S04: Back restores from the position memory (`listKey`) with zero
 * requests before paint — the remembered window renders from memory at its
 * scroll offset while the live collection revalidates behind it.
 */
export function EngineResults({collection, onRefresh, libraryId, libraryKind, list, tracks, unmatched, sort, sorts, onSort, sortField, square, filtered, onClearFilters, pivot, listKey, scanning, card, empty, sequence}: {/** The list as the server's query, for a row of the list view to play it on from there. */ sequence?: SequenceContainer; tracks?: boolean; /** The owner's Unmatched tab: empty is good news. */ unmatched?: boolean; sort: BrowseSortSelection; sorts: readonly BrowseSortCapability[]; /** Choosing a column header's order in the list view. */ onSort?: (next: BrowseSortSelection) => void; collection: WindowedCollection<ContentEntry>; onRefresh: () => void; libraryId: string; libraryKind: string; list: boolean; sortField: string; square: boolean; filtered: boolean; onClearFilters?: () => void; pivot?: string; listKey: string; scanning?: boolean; /** Cards that draw themselves (a genre's covers). They are not selectable. */ card?: (entry: ContentEntry) => React.ReactNode; /** What an empty list says when no filter is doing the excluding; the library's own words otherwise. */ empty?: React.ReactNode}) {
  const {t} = useI18n();
  const emptyCopy = useEmptyLibraryCopy();
  const remembered = useMemo(() => recallListWindow<ContentEntry>(listKey), [listKey]);
  const staticStore = useMemo(() => (remembered ? createStaticCollection<ContentEntry>({keyOf: e => e.id, base: remembered.first}) : null), [remembered]);
  useEffect(() => { staticStore?.setItems(remembered?.items ?? [], remembered?.total); }, [staticStore, remembered]);
  useEffect(() => () => staticStore?.collection.dispose(), [staticStore]);
  const [showLive, setShowLive] = useState(!remembered);
  const active = showLive || !staticStore ? collection : staticStore.collection;
  const snapshot = useSyncExternalStore(active.subscribe, active === collection ? collection.getSnapshot : staticStore!.collection.getSnapshot);
  // The live collection revalidates behind the remembered window; its changes
  // re-render so the swap below fires (the screen subscribes to `active` only).
  const liveSnap = useSyncExternalStore(collection.subscribe, collection.getSnapshot);
  const grid = useRef<WindowedGridHandle>(null);
  const actions = useSelectionActions();
  const selecting = useSelectionActive();
  const selectable = !!actions && !!selecting && !card;
  const [range, setRange] = useState({first: remembered?.first ?? 0, count: 0});
  const rangeRef = useRef(range);
  rangeRef.current = range;
  const [current, setCurrent] = useState<string>();
  // A different list in the same mount starts from its own memory, not the old screen.
  const lastKey = useRef(listKey);
  if (lastKey.current !== listKey) {
    lastKey.current = listKey;
    setShowLive(!remembered);
    setRange({first: remembered?.first ?? 0, count: 0});
    setCurrent(undefined);
  }
  const anchors = useMemo(() => Object.entries(snapshot.positionIndex ?? remembered?.positionIndex ?? {}).map(([key, index]) => ({key, index})).sort((a, b) => a.index - b.index), [snapshot.positionIndex, remembered]);
  const onFirstVisible = useCallback((index: number) => {
    let letter: string | undefined;
    for (const a of anchors) { if (a.index <= index) letter = a.key; else break; }
    setCurrent(letter);
    setRange(prev => (prev.first === index ? prev : {first: index, count: 60}));
    // While memory serves the screen, the live collection pages behind it.
    if (!showLive) collection.ensureRange(Math.max(0, index - 60), index + 120);
  }, [anchors, showLive, collection]);
  // The live collection starts at the remembered position, not the top.
  useEffect(() => {
    if (remembered && !showLive) collection.ensureRange(Math.max(0, remembered.first - 60), remembered.first + 120);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [collection, showLive]);
  // Swap to the live collection once it holds the remembered position.
  useEffect(() => {
    if (showLive || !remembered) return;
    // Also when the live list turned out shorter than the remembered position,
    // or failed: the server's current answer replaces memory either way.
    if (liveSnap.total !== undefined && (collection.itemAt(remembered.first) !== undefined || liveSnap.total <= remembered.first) || liveSnap.error !== undefined) {
      setShowLive(true);
      staticStore?.collection.dispose();
    }
  }, [showLive, remembered, liveSnap, collection, staticStore]);
  // Back lands at the remembered scroll offset (the static grid is sized from
  // the remembered total, so the offset maps to the same rows).
  useLayoutEffect(() => {
    if (remembered) window.scrollTo(0, remembered.scrollY);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  // Where the page was last scrolled to by the person. Read at the moment of leaving, the window's
  // own offset may already be the next page's (the router resets it), so it is followed as it changes.
  const scrolledTo = useRef(typeof window === 'undefined' ? 0 : window.scrollY);
  useEffect(() => {
    const follow = () => { scrolledTo.current = window.scrollY; };
    window.addEventListener('scroll', follow, {passive: true});
    return () => window.removeEventListener('scroll', follow);
  }, []);
  // Leaving the list remembers its loaded window and scroll offset.
  useEffect(() => () => {
    const snap = collection.getSnapshot();
    // Remember from the start of the page (60): the restored list then has no
    // partial page above the first visible item.
    const visibleFirst = rangeRef.current.first;
    const aligned = visibleFirst - (visibleFirst % 60);
    const first = collection.itemAt(aligned) !== undefined ? aligned : visibleFirst;
    const items: ContentEntry[] = [];
    for (let i = first; i < first + 180 && i < (snap.total ?? Number.MAX_SAFE_INTEGER); i++) {
      const item = collection.itemAt(i);
      if (!item) break;
      items.push(item);
    }
    saveListWindow(listKey, {first, items, total: snap.total ?? first + items.length, positionIndex: snap.positionIndex, scrollY: scrolledTo.current});
  }, [collection, listKey]);
  // Select all covers what's on screen (a million-item library can't be held).
  const visible = useMemo(() => {
    const out: ContentEntry[] = [];
    for (let i = range.first; i < range.first + range.count; i++) { const item = active.itemAt(i); if (item) out.push(item); }
    return out.length ? out : EMPTY;
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, range, snapshot]);
  useSelectionSection('browse', visible);
  const siblings = useCallback(() => visible, [visible]);
  const density = square ? 'square' : 'poster';
  // Before the first page the grid itself shows placeholders (and asks for that page): never gate it on `total`.
  if (snapshot.status === 'error' && !snapshot.total) return <ErrorState error={snapshot.error} context="library" retry={() => active.retry()} refresh={onRefresh} />;
  // WEB-LIB-02: "no match" only when a filter is doing the excluding.
  if (snapshot.total === 0) return filtered
    ? <StateView icon="filter" title={t('library.noMatch')} body={t('library.noMatchBody')} action={onClearFilters ? {label: t('library.clearFilters'), onClick: onClearFilters} : undefined} />
    : scanning
      ? <StateView icon={iconFor(libraryKind)} title={t('web.library.scanningTitle')} body={t('web.library.scanningBody')} />
      : empty !== undefined
        ? <>{empty}</>
      : pivot === 'collections'
        ? <StateView icon="collection" title={t('library.emptyCollections')} body={t('library.emptyCollectionsBody')} />
        : unmatched
          ? <StateView icon="check" title={t('library.emptyUnmatched')} body={t('library.emptyUnmatchedBody')} />
        : pivot === 'series'
          ? <StateView icon="collection" title={t('library.emptySeries')} body={t('library.emptySeriesBody')} />
          : <StateView icon={iconFor(libraryKind)} title={t('library.emptyLibrary')} body={emptyCopy.body} action={emptyCopy.action} />;
  const shape = square ? 'square' : 'poster';
  // M20: the rail follows anchor presence for default browsing (behind
  // presence for older servers), not the sort. An ad hoc query (a filter or
  // search) has no anchors and never shows the rail.
  // For you ranks by taste: it has no letters to jump to.
  // The letter rail is for a title order only: other orders have nothing to jump to by letter.
  const rail = anchors.length >= 4 && !filtered && sortField === 'title';
  return (
    <div className={b.layout} style={{paddingRight: rail ? 0 : undefined}}>
      <div>
        {snapshot.status === 'error' ? <Inset><ErrorNotice error={snapshot.error} context="library" retry={() => active.retry()} refresh={onRefresh} /></Inset> : null}
        {list ? (
          <TitlesTable ref={grid} collection={active} libraryId={libraryId} tracks={tracks} sort={sort} sorts={sorts} onSort={onSort} onFirstVisible={onFirstVisible} sequence={sequence} />
        ) : (
          <WindowedGrid
            ref={grid}
            collection={active}
            columnMin={gridColumnMin(density)}
            estimateRowHeight={square ? 240 : 300}
            label={t('web.library.titlesLabel')}
            onFirstVisible={onFirstVisible}
            renderItem={entry => (card ? card(entry) : <EntryCard entry={entry} siblings={siblings} libraryId={libraryId} selectable={selectable} />)}
            renderPlaceholder={() => <div aria-hidden><Skeleton style={{aspectRatio: shape === 'square' ? '1 / 1' : '2 / 3', width: '100%'}} /><Skeleton height={14} width="70%" style={{marginTop: 8}} /></div>}
          />
        )}
      </div>
      {rail ? <AlphabetRail index={anchors} current={current} sortField={sortField} onJump={key => { const index = active.indexOfLetter(key) ?? anchors.find(a => a.key === key)?.index; if (index !== undefined) grid.current?.scrollToIndex(index); }} /> : null}
    </div>
  );
}
