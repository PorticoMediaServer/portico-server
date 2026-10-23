import {useCallback, useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {ReadingPreferences, ReadingSession} from '@portico/client-core';
import {createComicAdapter, createEpubAdapter, createPdfAdapter, createReadingPublication,
  type ReadingController, type ReadingPublication, type ReaderErrorCode, type ReaderLocation, type ReaderSearchResult, type ReaderTocItem} from '@portico/reader-core';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {useShellWidth} from '../../shell/viewport';
import {Skeleton, StateView} from '../../kit/states';
import {ReaderContents, ReaderPanel, type ReaderContentsEntry} from '../../kit/reader/ReaderPanel';
import {ReaderSearch, type ReaderSearchHit} from '../../kit/reader/ReaderSearch';
import {createReadBindings} from './bindings';
import {bindReadPreferences} from './preferences';
import {bindReadingProgress} from './progress';
import {loadReadFonts, type ReadFonts} from './fonts';
import {readerPdfWorkerUrl} from './pdf-worker';
import {ReadReader} from './ReadReader';
import {ReadingDisplay} from './ReadingDisplay';
import {readingDisplayCopy} from './display-copy';
import type {ReaderMountAuthority} from './mount';
import styles from './entry.module.css';

const {t} = defaultI18n;
type Captured = NonNullable<ReturnType<typeof useProtectedWebScreenContext>>;
type OpenBook = Readonly<{session: ReadingSession; publication: ReadingPublication; controller: ReadingController;
  authority: ReaderMountAuthority; location?: ReaderLocation}>;

function ReaderBook({book, reportError, onBack}: {book: OpenBook; reportError: (error: unknown) => void; onBack: () => void}) {
  const {controller, publication, authority} = book;
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot), width = useShellWidth();
  const [panel, setPanel] = useState<'display' | 'contents' | 'search'>(), [query, setQuery] = useState('');
  const [hits, setHits] = useState<readonly ReaderSearchResult[]>([]), [searching, setSearching] = useState(false), [currentHit, setCurrentHit] = useState<string>();
  const anchor = useRef<HTMLButtonElement>(null);
  const engineError = useCallback((code: ReaderErrorCode) => reportError({code}), [reportError]);
  const presentation = width < 600 ? 'phone' : width < 1024 ? 'tablet' : 'desktop';
  const navigate = (location: ReaderLocation) => {
    if (!authority.isCurrent()) return;
    void controller.navigate({kind: 'location', location}).catch(reportError);
    if (presentation === 'phone') setPanel(undefined);
  };
  useEffect(() => {
    if (panel !== 'search' || !query.trim()) {setHits([]); setSearching(false); return;}
    const life = new AbortController(); let active = true;
    setSearching(true); setHits([]);
    void (async () => {
      const found: ReaderSearchResult[] = [];
      for await (const hit of controller.search(query, {signal: life.signal, limit: 100})) {
        if (!active || !authority.isCurrent()) return;
        found.push(hit); setHits([...found]);
      }
    })().catch(error => {if (active && authority.isCurrent()) reportError(error);}).finally(() => {if (active) setSearching(false);});
    return () => {active = false; life.abort();};
  }, [controller, authority, panel, query, reportError]);
  const contents = (items: readonly ReaderTocItem[]): readonly ReaderContentsEntry[] => items.map(item => ({id: item.id, label: item.label,
    page: item.location.pageLabel, onNavigate: () => navigate(item.location), children: item.children ? contents(item.children) : undefined}));
  const searchHits: ReaderSearchHit[] = hits.map(hit => {
    const start = hit.excerpt.toLocaleLowerCase().indexOf(query.toLocaleLowerCase());
    const context = start < 0 ? [{text: hit.excerpt}] : [{text: hit.excerpt.slice(0, start)}, {text: hit.excerpt.slice(start, start + query.length), match: true}, {text: hit.excerpt.slice(start + query.length)}];
    return {id: hit.id, chapter: hit.chapter, page: hit.location.pageLabel, context, onNavigate: () => {setCurrentHit(hit.id); navigate(hit.location);}};
  });
  const panelContent = panel === 'contents' ? <ReaderPanel label={t('next.reader.contents')} tabs={[{id: 'contents', label: t('next.reader.contents')}]}
    selected="contents" onSelect={() => {}} onClose={() => setPanel(undefined)}><ReaderContents entries={contents(snapshot.toc)}/></ReaderPanel>
    : panel === 'search' ? <ReaderPanel label={t('next.reader.search')} tabs={[{id: 'search', label: t('next.reader.search')}]}
      selected="search" onSelect={() => {}} onClose={() => setPanel(undefined)}><ReaderSearch query={query} onQuery={setQuery} hits={searchHits} current={currentHit} loading={searching}
        resultSummary={!searching && query.trim() && hits.length < 100 ? t('next.reader.matches', {count: hits.length}) : undefined}
        onStep={direction => {const index = hits.findIndex(hit => hit.id === currentHit), next = hits[(index + (direction === 'next' ? 1 : -1) + hits.length) % hits.length]; if (next) {setCurrentHit(next.id); navigate(next.location);}}}
        copy={{search: t('next.reader.search'), clear: t('next.reader.clear'), move: t('next.reader.move'), wholeWords: t('next.reader.wholeWords'), searchAll: t('next.reader.searchAll')}}/></ReaderPanel>
      : panel === 'display' ? <ReadingDisplay controller={controller} authority={authority} copy={readingDisplayCopy(defaultI18n, presentation, 7)}
        scale={{min: 1, max: 7, step: 1, ticks: [1, 2, 3, 4, 5, 6, 7]}} presentation={presentation} label={t('next.reader.display')}
        open onOpenChange={open => {if (!open) setPanel(undefined);}} anchorRef={anchor} reportError={code => reportError({code})}
        onOpenSettings={() => {void router.navigate({to: '/settings', search: {scope: 'account', section: 'reading'}}).catch(reportError);}}/> : undefined;
  return <ReadReader<ReadingPreferences> controller={controller} source={publication.source} format={publication.format} initialLocation={book.location}
    metadata={publication.metadata} signal={publication.signal} authority={authority} reportError={engineError} onBack={onBack}
    state={state => state.status === 'error' ? <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}/> : <Skeleton height="100%"/>}
    presentation={value => ({copy: {back: t('next.reader.back'), previous: t('next.reader.previous'), next: t('next.reader.next'), progress: t('next.reader.position')},
      direction: publication.format === 'cbz' && value.preferences.comicDirection !== 'file' ? value.preferences.comicDirection === 'rtl' ? 'rtl' : 'ltr'
        : value.metadata?.direction === 'rtl' ? 'rtl' : 'ltr', tapTurnEnabled: value.preferences.tapToTurn, showPageArrows: width >= 1024,
      chapter: value.location?.chapter, pages: value.location?.pageLabel ? t('next.reader.pageLabel', {page: value.location.pageLabel}) : undefined,
      remainingInSection: value.location?.pagesLeftInSection === undefined ? undefined : t('next.reader.sectionRemaining', {count: value.location.pagesLeftInSection}),
      percent: value.location ? defaultI18n.percent(value.location.progression) : undefined, panel: panelContent, panelReflows: !!panel && presentation !== 'phone',
      onPanelDismiss: () => setPanel(undefined), actions: [
        {id: 'contents', label: t('next.reader.contents'), icon: 'i-chapters', expanded: panel === 'contents', onPress: () => setPanel(panel === 'contents' ? undefined : 'contents')},
        {id: 'search', label: t('next.reader.search'), icon: 'i-search', expanded: panel === 'search', onPress: () => setPanel(panel === 'search' ? undefined : 'search')},
        ...(publication.format === 'epub' ? [{id: 'display', label: t('next.reader.displayAction'), text: <span className={styles.aa}>{Array.from(t('next.reader.sample'))[0]}<small>{Array.from(t('next.reader.sample')).slice(1).join('')}</small></span>, expanded: panel === 'display', onPress: (element: HTMLButtonElement) => {anchor.current = element; setPanel(panel === 'display' ? undefined : 'display');}}] : []),
        ...(presentation === 'desktop' && document.fullscreenEnabled ? [{id: 'fullscreen', label: t('next.reader.fullscreen'), icon: 'i-fs' as const, separatorBefore: true,
          onPress: (element: HTMLButtonElement) => {const root = element.closest<HTMLElement>('[data-reader-root]'); void (document.fullscreenElement ? document.exitFullscreen() : root?.requestFullscreen())?.catch(reportError);}}] : []),
      ]})}/>;
}

/** Real publication session; services start only inside the captured route lifetime. */
export function ReadEntry({captured, itemId, sourceId}: {captured: Captured; itemId: string; sourceId?: string}) {
  const [state, setState] = useState<Readonly<{book?: OpenBook; failed?: boolean}>>({});
  const [retry, setRetry] = useState(0);
  const reportRef = useRef(captured.binding.home.effects.reportError); reportRef.current = captured.binding.home.effects.reportError;
  const reportError = useRef((error: unknown) => {if (captured.context.isCurrent()) reportRef.current(error);}).current;
  useEffect(() => {
    const life = new AbortController(), bindings = createReadBindings(captured.context);
    let alive = true, fonts: ReadFonts | undefined, publication: ReadingPublication | undefined;
    let reader: ReturnType<typeof bindReadPreferences> | undefined, progress: ReturnType<typeof bindReadingProgress> | undefined;
    const offRetired = bindings.authority.subscribe(() => {if (alive) setState({failed: true});});
    let disposal: Promise<void> | undefined;
    const dispose = () => {
      if (disposal) return disposal;
      alive = false; offRetired(); life.abort();
      disposal = (async () => {try {await reader?.dispose(); await progress?.stop();} finally {publication?.dispose(); fonts?.dispose(); await bindings.dispose();}})();
      return disposal;
    };
    setState({});
    void (async () => {
      const versions = (await bindings.client.versions(itemId, life.signal)).versions.filter(version => version.inventoryAvailable);
      if (!alive) return;
      const selected = sourceId ? versions.find(version => version.sourceId === sourceId) : versions.length === 1 ? versions[0] : undefined;
      if (!selected) {
        if (versions.length > 1 && !sourceId) await router.navigate({to: '/item/$itemId', params: {itemId}, replace: true});
        else setState({failed: true});
        return;
      }
      const session = await bindings.open({itemId, sourceId: selected.sourceId});
      publication = await createReadingPublication(session, life.signal);
      const saved = await session.progress(life.signal), location = publication.resumeLocation(saved) ?? undefined;
      if (publication.format === 'epub') fonts = await loadReadFonts({document, signal: life.signal});
      if (!alive) return;
      const adapters = publication.format === 'epub' ? [createEpubAdapter({fonts: fonts!.fonts})]
        : publication.format === 'pdf' ? [createPdfAdapter({workerUrl: readerPdfWorkerUrl})] : [createComicAdapter({pages: publication.comicPages!})];
      reader = bindReadPreferences({adapters, preferences: bindings.preferences, authority: bindings.authority, reportError: code => reportError({code})});
      progress = bindReadingProgress({controller: reader.controller, session, initial: saved, authority: bindings.authority, reportError});
      setState({book: {session, publication, controller: reader.controller, authority: bindings.authority, location}});
    })().catch(error => {if (alive && bindings.authority.isCurrent()) {reportError(error); setState({failed: true});} void dispose().catch(() => {});});
    return () => {void dispose().catch(() => {});};
  }, [captured.context, itemId, sourceId, reportError, retry]);
  if (state.book) return <ReaderBook key={state.book.session.manifest.sessionId} book={state.book} reportError={reportError} onBack={() => router.history.back()}/>;
  if (state.failed) return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => setRetry(value => value + 1)}}/>;
  return <div className={styles.loading}><Skeleton height="100dvh"/></div>;
}
