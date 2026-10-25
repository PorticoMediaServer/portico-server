import {useCallback, useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {ReadingPreferences} from '@portico/client-core';
import type {ReadingController, ReadingPublication, ReaderErrorCode, ReaderLocation, ReaderSearchResult, ReaderTocItem} from '@portico/reader-core';
import {useShellWidth} from '../../shell/viewport';
import {Skeleton, StateView} from '../../kit/states';
import {ReaderContents, ReaderPanel, type ReaderContentsEntry} from '../../kit/reader/ReaderPanel';
import {ReaderDisplayMark} from '../../kit/reader/ReaderDisplayMark';
import {ReaderSearch, type ReaderSearchHit} from '../../kit/reader/ReaderSearch';
import {ReadReader} from './ReadReader';
import {ReadingDisplay} from './ReadingDisplay';
import {readingDisplayCopy} from './display-copy';
import {readerBookPresentation, type ReaderEnvironment} from './native/tablet-presentation';
import type {ReaderMountAuthority} from './mount';
const {t} = defaultI18n;
export type ReaderBookState = Readonly<{publication: ReadingPublication; controller: ReadingController; authority: ReaderMountAuthority; location?: ReaderLocation}>;

export function ReaderBook({book, active, reportError, onBack, onOpenSettings, environment = 'web'}: {environment?: ReaderEnvironment; book: ReaderBookState; active: boolean; reportError: (error: unknown) => void; onBack: () => void; onOpenSettings: () => void}) {
  const {controller, publication, authority} = book;
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot), width = useShellWidth();
  const [panel, setPanel] = useState<'display' | 'contents' | 'search'>(), [query, setQuery] = useState('');
  const [hits, setHits] = useState<readonly ReaderSearchResult[]>([]), [searching, setSearching] = useState(false), [currentHit, setCurrentHit] = useState<string>();
  const anchor = useRef<HTMLButtonElement>(null);
  const engineError = useCallback((code: ReaderErrorCode) => reportError({code}), [reportError]);
  const layout = readerBookPresentation(width, environment), {presentation} = layout;
  useEffect(() => {if (!active) setPanel(undefined);}, [active]);
  const navigate = (location: ReaderLocation) => {
    if (!authority.isCurrent()) return;
    void controller.navigate({kind: 'location', location}).catch(reportError);
    if (presentation === 'phone') setPanel(undefined);
  };
  useEffect(() => {
    if (!active || panel !== 'search' || !query.trim()) {setHits([]); setSearching(false); return;}
    const life = new AbortController(); let queryCurrent = true;
    setSearching(true); setHits([]);
    void (async () => {
      const found: ReaderSearchResult[] = [];
      for await (const hit of controller.search(query, {signal: life.signal, limit: 100})) {
        if (!queryCurrent || !authority.isCurrent()) return;
        found.push(hit); setHits([...found]);
      }
    })().catch(error => {if (queryCurrent && authority.isCurrent()) reportError(error);}).finally(() => {if (queryCurrent) setSearching(false);});
    return () => {queryCurrent = false; life.abort();};
  }, [controller, authority, active, panel, query, reportError]);
  const contents = (items: readonly ReaderTocItem[]): readonly ReaderContentsEntry[] => items.map(item => ({id: item.id, label: item.label,
    page: item.location.pageLabel, onNavigate: () => navigate(item.location), children: item.children ? contents(item.children) : undefined}));
  const searchHits: ReaderSearchHit[] = hits.map(hit => {
    const start = hit.excerpt.toLocaleLowerCase().indexOf(query.toLocaleLowerCase());
    const context = start < 0 ? [{text: hit.excerpt}] : [{text: hit.excerpt.slice(0, start)}, {text: hit.excerpt.slice(start, start + query.length), match: true}, {text: hit.excerpt.slice(start + query.length)}];
    return {id: hit.id, chapter: hit.chapter, page: hit.location.pageLabel, context, onNavigate: () => {setCurrentHit(hit.id); navigate(hit.location);}};
  });
  const panelContent = !active ? undefined : panel === 'contents' ? <ReaderPanel label={t('next.reader.contents')} tabs={[{id: 'contents', label: t('next.reader.contents')}]}
    selected="contents" onSelect={() => {}} onClose={() => setPanel(undefined)}><ReaderContents entries={contents(snapshot.toc)}/></ReaderPanel>
    : panel === 'search' ? <ReaderPanel label={t('next.reader.search')} tabs={[{id: 'search', label: t('next.reader.search')}]}
      selected="search" onSelect={() => {}} onClose={() => setPanel(undefined)}><ReaderSearch query={query} onQuery={setQuery} hits={searchHits} current={currentHit} loading={searching}
        resultSummary={!searching && query.trim() && hits.length < 100 ? t('next.reader.matches', {count: hits.length}) : undefined}
        onStep={direction => {const index = hits.findIndex(hit => hit.id === currentHit), next = hits[(index + (direction === 'next' ? 1 : -1) + hits.length) % hits.length]; if (next) {setCurrentHit(next.id); navigate(next.location);}}}
        copy={{search: t('next.reader.search'), clear: t('next.reader.clear'), move: t('next.reader.move'), wholeWords: t('next.reader.wholeWords'), searchAll: t('next.reader.searchAll')}}/></ReaderPanel>
      : panel === 'display' ? <ReadingDisplay controller={controller} authority={authority} copy={readingDisplayCopy(defaultI18n, presentation, 7)}
        scale={{min: 1, max: 7, step: 1, ticks: [1, 2, 3, 4, 5, 6, 7]}} presentation={presentation} popover={layout.displayPopover} label={t('next.reader.display')}
        open onOpenChange={open => {if (!open) setPanel(undefined);}} anchorRef={anchor} reportError={code => reportError({code})}
        onOpenSettings={() => {setPanel(undefined); onOpenSettings();}}/> : undefined;
  return <ReadReader<ReadingPreferences> controller={controller} source={publication.source} format={publication.format} initialLocation={book.location}
    metadata={publication.metadata} signal={publication.signal} authority={authority} reportError={engineError} onBack={onBack}
    state={state => state.status === 'error' ? <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}/> : <Skeleton height="100%"/>}
    presentation={value => ({copy: {back: t('next.reader.back'), previous: t('next.reader.previous'), next: t('next.reader.next'), progress: t('next.reader.position')},
      direction: publication.format === 'cbz' && value.preferences.comicDirection !== 'file' ? value.preferences.comicDirection === 'rtl' ? 'rtl' : 'ltr'
        : value.metadata?.direction === 'rtl' ? 'rtl' : 'ltr', tapTurnEnabled: value.preferences.tapToTurn, showPageArrows: layout.showPageArrows,
      chapter: value.location?.chapter, pages: value.location?.pageLabel ? t('next.reader.pageLabel', {page: value.location.pageLabel}) : undefined,
      remainingInSection: value.location?.pagesLeftInSection === undefined ? undefined : t('next.reader.sectionRemaining', {count: value.location.pagesLeftInSection}),
      percent: value.location ? defaultI18n.percent(value.location.progression) : undefined, panel: panelContent, panelReflows: layout.panelReflows(panel),
      onPanelDismiss: () => setPanel(undefined), actions: [
        {id: 'contents', label: t('next.reader.contents'), icon: 'i-chapters', expanded: panel === 'contents', onPress: () => setPanel(panel === 'contents' ? undefined : 'contents')},
        {id: 'search', label: t('next.reader.search'), icon: 'i-search', expanded: panel === 'search', onPress: () => setPanel(panel === 'search' ? undefined : 'search')},
        ...(publication.format === 'epub' ? [{id: 'display', label: t('next.reader.displayAction'), text: <ReaderDisplayMark sample={t('next.reader.sample')}/>, expanded: panel === 'display', onPress: (element: HTMLButtonElement) => {anchor.current = element; setPanel(panel === 'display' ? undefined : 'display');}}] : []),
        ...(layout.fullscreen && document.fullscreenEnabled ? [{id: 'fullscreen', label: t('next.reader.fullscreen'), icon: 'i-fs' as const, separatorBefore: true,
          onPress: (element: HTMLButtonElement) => {const root = element.closest<HTMLElement>('[data-reader-root]'); void (document.fullscreenElement ? document.exitFullscreen() : root?.requestFullscreen())?.catch(reportError);}}] : []),
      ]})}/>;
}

