import {readingSearchStepIndex} from './search-step';
import {useCallback, useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {ReadingPreferences} from '@portico/client-core';
import type {ReadingController, ReadingPublication, ReaderErrorCode, ReaderLocation, ReaderContentsTarget, ReaderSearchResult, ReaderTocItem} from '@portico/reader-core';
import {useShellWidth} from '../../shell/viewport';
import {router} from '../../router';
import {Skeleton, StateView} from '../../kit/states';
import {ReaderContents, ReaderPanel, type ReaderContentsEntry} from '../../kit/reader/ReaderPanel';
import {ReaderDisplayMark} from '../../kit/reader/ReaderDisplayMark';
import {ReaderSearch, type ReaderSearchHit} from '../../kit/reader/ReaderSearch';
import {ReadReader} from './ReadReader';
import {ReadingDisplay} from './ReadingDisplay';
import {ReaderFirstRunHint} from './ReaderFirstRunHint';
import {FixedPageMenus, FixedPageStrip, FixedPagesPanel, fixedPageZoomLabel} from './fixed-page';
import {readingDisplayCopy} from './display-copy';
import {readerBookPresentation, type ReaderEnvironment} from './native/tablet-presentation';
import type {ReaderMountAuthority} from './mount';
import type {ReaderChapterMark} from './ReaderChrome';
import fixedStyles from './fixed-page.module.css';
import {ReaderMatchNavigation} from './ReaderMatchNavigation';
import {ReaderBrowserHint} from './ReaderBrowserHint';
const {t} = defaultI18n;
const directionIcon = (rtl: boolean): 'i-rtl' | 'i-ltr' => rtl ? 'i-rtl' : 'i-ltr';
/** Real outline boundaries, including nested chapters; coincident starts draw one tick. */
export function readerChapterMarks(items: readonly ReaderTocItem[], location?: ReaderLocation): readonly ReaderChapterMark[] {
  const outline: ReaderTocItem[] = [];
  const visit = (entries: readonly ReaderTocItem[]) => {for (const entry of entries) {
    const start = entry.location.progression;
    if (Number.isFinite(start) && start >= 0 && start <= 1) outline.push(entry);
    if (entry.children) visit(entry.children);
  }};
  visit(items);
  outline.sort((left, right) => left.location.progression - right.location.progression);
  const boundaries = outline.filter((entry, index) => index === 0 || entry.location.progression !== outline[index - 1].location.progression);
  return boundaries.map((item, index) => {
    const start = item.location.progression, end = boundaries[index + 1]?.location.progression ?? 1;
    return {id: item.id, start, end, current: location && location.format === item.location.format
      ? location.progression >= start && location.progression < end : undefined};
  });
}
/** Fixed page ordinals/counts are actual renderer facts, never inferred EPUB pages. */
export function readerPagePosition(location: ReaderLocation | null, count: number | undefined): string | undefined {
  if (!location) return;
  if (location.format !== 'epub' && count !== undefined && Number.isSafeInteger(count) && count > 0) {
    return t('next.reader.pageOf', {page: location.pageLabel ?? defaultI18n.number(location.page + 1), count: defaultI18n.number(count)});
  }
  return location.pageLabel ? t('next.reader.pageLabel', {page: location.pageLabel}) : undefined;
}
/** Use actual ordered outline positions and the live location, without inventing read/completion facts. */
export function readerCurrentContentsId(items: readonly ReaderTocItem[], location: ReaderLocation | null): string | undefined {
  if (!location || !Number.isFinite(location.progression)) return;
  const flat: ReaderTocItem[] = [];
  const visit = (entries: readonly ReaderTocItem[]) => {for (const entry of entries) {flat.push(entry); if (entry.children) visit(entry.children);}};
  visit(items);
  let current: ReaderTocItem | undefined;
  for (const item of flat) {
    if (item.location.format !== location.format || !Number.isFinite(item.location.progression)) return;
    if (item.location.progression <= location.progression && (!current || item.location.progression > current.location.progression)) current = item;
  }
  // Equal starts are ambiguous (for example a parent and its first child).
  if (!current) return;
  const start = current.location.progression;
  if (flat.filter(item => item.location.progression === start).length === 1) return current.id;
}
export type ReaderBookState = Readonly<{publication: ReadingPublication; controller: ReadingController; authority: ReaderMountAuthority; location?: ReaderLocation; contentsTarget?: ReaderContentsTarget}>;

export function ReaderBook({book, active, reportError, onBack, onOpenSettings, environment = 'web'}: {environment?: ReaderEnvironment; book: ReaderBookState; active: boolean; reportError: (error: unknown) => void; onBack: () => void; onOpenSettings: () => void}) {
  const {controller, publication, authority} = book;
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot), width = useShellWidth();
  const [panel, setPanel] = useState<'display' | 'contents' | 'search'>(), [query, setQuery] = useState('');
  const [fixedMenu, setFixedMenu] = useState<'zoom' | 'direction'>();
  const [fixedPane, setFixedPane] = useState<'contents' | 'pages'>('contents');
  const zoomAnchor = useRef<HTMLButtonElement>(null), directionAnchor = useRef<HTMLButtonElement>(null);
  const fixed = publication.format !== 'epub';
  const [hits, setHits] = useState<readonly ReaderSearchResult[]>([]), [searching, setSearching] = useState(false), [searchComplete, setSearchComplete] = useState(false), [currentHit, setCurrentHit] = useState<string>();
  // phone-read 14: taught once per browser, never re-shown after Got it.
  const hintKey = 'portico.reader.tapZonesHint.v1';
  const [hint, setHint] = useState(() => {try {return !globalThis.localStorage?.getItem(hintKey);} catch {return true;}});
  const dismissHint = useCallback(() => {setHint(false); try {globalThis.localStorage?.setItem(hintKey, '1');} catch { /* Disabled storage simply keeps the hint dismissed for this mount. */ }}, []);
  const [hitsQuery, setHitsQuery] = useState('');
  const queryRef = useRef(query); queryRef.current = query;
  const visibleHits = hitsQuery === query ? hits : [];
  const anchor = useRef<HTMLButtonElement>(null);
  const contentsError = useRef(reportError); contentsError.current = reportError;
  useEffect(() => {
    const target = book.contentsTarget;
    if (!target) return;
    const operation = new AbortController(); let applied = false;
    const apply = () => {
      if (applied || operation.signal.aborted || controller.getSnapshot().status !== 'ready' || !authority.isCurrent()) return;
      applied = true;
      try {
        const current = publication.contentsTarget(target.index);
        if (current.sourceId !== target.sourceId || current.sourceRevision !== target.sourceRevision) throw new Error('source_changed');
        void controller.navigateContents(current, operation.signal).catch(error => {
          if (!operation.signal.aborted && authority.isCurrent()) contentsError.current(error);
        });
      } catch (error) {if (authority.isCurrent()) contentsError.current(error);}
    };
    const off = controller.subscribe(apply); apply();
    return () => {operation.abort(); off();};
  }, [book.contentsTarget, publication, authority, controller]);
  const engineError = useCallback((code: ReaderErrorCode) => reportError({code}), [reportError]);
  const layout = readerBookPresentation(width, environment), {presentation} = layout;
  useEffect(() => {if (!active) {setPanel(undefined); setFixedMenu(undefined);}}, [active]);
  const navigate = (location: ReaderLocation) => {
    const current = controller.getSnapshot();
    if (!active || !authority.isCurrent() || publication.signal.aborted || current.status !== 'ready'
      || current.sourceId !== publication.source.id || current.sourceRevision !== publication.source.revision) return;
    void controller.navigate({kind: 'location', location}).catch(reportError);
    if (presentation === 'phone') setPanel(undefined);
  };
  useEffect(() => {
    setCurrentHit(undefined); setSearchComplete(false); setHitsQuery(query);
    if (!active || !query.trim()) {setHits([]); setSearching(false); return;}
    const life = new AbortController(); let queryCurrent = true;
    setSearching(true); setHits([]);
    void (async () => {
      const found: ReaderSearchResult[] = [];
      for await (const hit of controller.search(query, {signal: life.signal, limit: 100})) {
        if (!queryCurrent || queryRef.current !== query || !authority.isCurrent()) return;
        found.push(hit); setHits([...found]);
      }
      // A rejected or retired iterator never establishes a complete result count.
      // At the core's limit, exhaustion is bounded rather than a book-wide total.
      if (queryCurrent && queryRef.current === query && authority.isCurrent() && !life.signal.aborted && found.length < 100) setSearchComplete(true);
    })().catch(error => {if (queryCurrent && authority.isCurrent()) reportError(error);}).finally(() => {if (queryCurrent) setSearching(false);});
    return () => {queryCurrent = false; life.abort();};
  }, [controller, authority, active, query, reportError]);
  const stepSearch = (direction: 'previous' | 'next') => {
    if (queryRef.current !== query) return;
    const index = readingSearchStepIndex(visibleHits.length, visibleHits.findIndex(hit => hit.id === currentHit), direction);
    const next = index === undefined ? undefined : visibleHits[index];
    if (next) {setCurrentHit(next.id); navigate(next.location);}
  };
  const currentContentsId = snapshot.status === 'ready' && snapshot.sourceId === publication.source.id && snapshot.sourceRevision === publication.source.revision
    ? readerCurrentContentsId(snapshot.toc, snapshot.location) : undefined;
  const contents = (items: readonly ReaderTocItem[]): readonly ReaderContentsEntry[] => items.map(item => ({id: item.id, label: item.label,
    page: item.location.pageLabel, current: item.id === currentContentsId,
    remaining: item.id === currentContentsId && snapshot.location?.pagesLeftInSection !== undefined
      ? t('next.reader.sectionRemaining', {count: snapshot.location.pagesLeftInSection}) : undefined, onNavigate: () => navigate(item.location), children: item.children ? contents(item.children) : undefined}));
  const searchHits: ReaderSearchHit[] = visibleHits.map(hit => {
    const start = hit.excerpt.toLocaleLowerCase().indexOf(query.toLocaleLowerCase());
    const context = start < 0 ? [{text: hit.excerpt}] : [{text: hit.excerpt.slice(0, start)}, {text: hit.excerpt.slice(start, start + query.length), match: true}, {text: hit.excerpt.slice(start + query.length)}];
    return {id: hit.id, chapter: hit.chapter, page: hit.location.pageLabel, context, onNavigate: () => {if (queryRef.current !== query) return; setCurrentHit(hit.id); navigate(hit.location);}};
  });
  const panelContent = !active ? undefined : panel === 'contents' ? (fixed
    ? <ReaderPanel label={t('next.reader.pagesStrip')} tabs={[...(snapshot.toc.length > 0 ? [{id: 'contents', label: t('next.reader.contents')}] : []), {id: 'pages', label: t('next.reader.pagesStrip')}]}
      selected={snapshot.toc.length > 0 ? fixedPane : 'pages'} onSelect={selected => setFixedPane(selected === 'pages' ? 'pages' : 'contents')} onClose={() => setPanel(undefined)}>
      {snapshot.toc.length > 0 && fixedPane === 'contents' ? <ReaderContents entries={contents(snapshot.toc)}/>
        : <FixedPagesPanel controller={controller} authority={authority} format={publication.format} reportError={code => reportError({code})}/>}
    </ReaderPanel>
    : <ReaderPanel label={t('next.reader.contents')} tabs={[{id: 'contents', label: t('next.reader.contents')}]}
      selected="contents" onSelect={() => {}} onClose={() => setPanel(undefined)}><ReaderContents entries={contents(snapshot.toc)}/></ReaderPanel>)
    : panel === 'search' ? <ReaderPanel label={t('next.reader.search')} tabs={[{id: 'search', label: t('next.reader.search')}]}
      selected="search" onSelect={() => {}} onClose={() => setPanel(undefined)}><ReaderSearch query={query} onQuery={setQuery} hits={searchHits} current={currentHit} loading={searching}
        resultSummary={hitsQuery === query && query.trim() && !searching
          ? searchComplete ? t('next.reader.matches', {count: visibleHits.length})
            : visibleHits.length > 0 ? t('next.reader.matchesShown', {count: visibleHits.length}) : undefined
          : undefined}
        onSearchAll={query.trim() ? () => {if (!authority.isCurrent() || !query.trim()) return; void router.navigate({to: '/search', search: {q: query.trim()}}).catch(reportError);} : undefined}
        onStep={stepSearch}
        copy={{search: t('next.reader.search'), clear: t('next.reader.clear'), move: t('next.reader.move'), wholeWords: t('next.reader.wholeWords'), searchAll: t('next.reader.searchAll')}}/></ReaderPanel>
      : panel === 'display' ? <ReadingDisplay controller={controller} authority={authority} copy={readingDisplayCopy(defaultI18n, presentation, 7)}
        scale={{min: 1, max: 7, step: 1, ticks: [1, 2, 3, 4, 5, 6, 7]}} presentation={presentation} popover={layout.displayPopover} label={t('next.reader.display')}
        open onOpenChange={open => {if (!open) setPanel(undefined);}} anchorRef={anchor} reportError={code => reportError({code})}
        onOpenSettings={() => {setPanel(undefined); onOpenSettings();}}/> : undefined;
  return <><ReadReader<ReadingPreferences> controller={controller} source={publication.source} format={publication.format} initialLocation={book.location}
    metadata={publication.metadata} signal={publication.signal} authority={authority} reportError={engineError} onBack={onBack}
    state={state => state.status === 'error' ? <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}/> : <Skeleton height="100%"/>}
    presentation={value => ({copy: {back: t('next.reader.back'), previous: t('next.reader.previous'), next: t('next.reader.next'), progress: t('next.reader.position')},
      holdOpen: fixedMenu !== undefined,
      direction: publication.format === 'cbz' && value.preferences.comicDirection !== 'file' ? value.preferences.comicDirection === 'rtl' ? 'rtl' : 'ltr'
        : value.metadata?.direction === 'rtl' ? 'rtl' : 'ltr', tapTurnEnabled: value.preferences.tapToTurn, showPageArrows: layout.showPageArrows,
      chapters: readerChapterMarks(value.toc, value.location ?? undefined),
      chapter: value.location?.chapter, pages: readerPagePosition(value.location, value.metadata?.pageCount),
      remainingInSection: value.location?.pagesLeftInSection === undefined ? undefined : t('next.reader.sectionRemaining', {count: value.location.pagesLeftInSection}),
      hint: presentation === 'phone' && hint && !panel && value.status === 'ready' && value.preferences.tapToTurn && value.selection === null ? <ReaderFirstRunHint onDone={dismissHint}/> : undefined,
      percent: value.location ? defaultI18n.percent(value.location.progression) : undefined,
      searchNavigation: active && value.status === 'ready' && value.sourceId === publication.source.id && value.sourceRevision === publication.source.revision && presentation === 'phone' && currentHit && visibleHits.some(hit => hit.id === currentHit)
        ? <ReaderMatchNavigation query={query} onStep={stepSearch} onClose={() => setQuery('')}/> : undefined, panel: panelContent, panelReflows: layout.panelReflows(panel),
      fixedPageStrip: fixed ? <FixedPageStrip controller={controller} authority={authority} format={publication.format} reportError={code => reportError({code})}/> : undefined,
      onPanelDismiss: () => setPanel(undefined), actions: [
        {id: 'contents', label: t('next.reader.contents'), icon: 'i-chapters', expanded: panel === 'contents', onPress: () => setPanel(panel === 'contents' ? undefined : 'contents')},
        ...(publication.format !== 'cbz' ? [{id: 'search', label: t('next.reader.search'), icon: 'i-search' as const, expanded: panel === 'search', onPress: () => setPanel(panel === 'search' ? undefined : 'search')}] : []),
        ...(publication.format === 'epub' ? [{id: 'display', label: t('next.reader.displayAction'), text: <ReaderDisplayMark sample={t('next.reader.sample')}/>, expanded: panel === 'display', onPress: (element: HTMLButtonElement) => {anchor.current = element; setPanel(panel === 'display' ? undefined : 'display');}}] : []),
        ...(fixed ? [{id: 'zoom', label: t('next.reader.zoom'), text: <span className={fixedStyles.zoomLabel}>{fixedPageZoomLabel(value.zoom)}</span>,
          expanded: fixedMenu === 'zoom', onPress: (element: HTMLButtonElement) => {zoomAnchor.current = element; setFixedMenu(fixedMenu === 'zoom' ? undefined : 'zoom');}},
          ...(publication.format === 'cbz' ? [{id: 'direction', label: t('next.reader.direction'), icon: directionIcon(value.preferences.comicDirection === 'rtl' || value.preferences.comicDirection === 'file' && publication.metadata.direction === 'rtl'),
            expanded: fixedMenu === 'direction', onPress: (element: HTMLButtonElement) => {directionAnchor.current = element; setFixedMenu(fixedMenu === 'direction' ? undefined : 'direction');}}] : [])] : []),
        ...(layout.fullscreen && document.fullscreenEnabled ? [{id: 'fullscreen', label: t('next.reader.fullscreen'), icon: 'i-fs' as const, separatorBefore: true,
          onPress: (element: HTMLButtonElement) => {const root = element.closest<HTMLElement>('[data-reader-root]'); void (document.fullscreenElement ? document.exitFullscreen() : root?.requestFullscreen())?.catch(reportError);}}] : []),
      ]})}/>
    {active && environment === 'web' && snapshot.status === 'ready' && <ReaderBrowserHint visible={presentation === 'phone' && !panel && !fixedMenu && !currentHit && snapshot.selection === null && !hint}/>}
    {active && fixed && <FixedPageMenus controller={controller} authority={authority} format={publication.format} reportError={code => reportError({code})}
      open={fixedMenu} onOpenChange={setFixedMenu} anchorRef={zoomAnchor} directionRef={directionAnchor}/>}
  </>;
}

