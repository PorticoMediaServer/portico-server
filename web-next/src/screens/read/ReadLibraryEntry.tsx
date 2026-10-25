import {useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {generatedCoverTint} from '@design';
import type {FactPart} from '@portico/client-core/kit-model';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {useShellWidth} from '../../shell/viewport';
import {useReducedMotion} from '../../kit/overlays/shared';
import {WindowedGrid} from '../../kit/layout';
import {CoverCard} from '../../kit/cards';
import {GeneratedCover} from '../../kit/artwork';
import {Skeleton, StateView} from '../../kit/states';
import {Menu, MenuItem} from '../../kit/overlays';
import {useLibraryScreenContext} from '../browse/screen-context';
import {ReadMetadata} from './ReadMetadata';
import {createReadBindings} from './bindings';
import {createReadingCollection, createReadingContinueCollection, type ReadingContinueItem, type ReadingLibraryItem} from './library-collection';
import {readDestination} from './navigation';
import {recentCards, type RecentReadingItem} from './recent-cards';
import {ReadDiscovery, type ReadDiscoveryRow, type ReadResumeItem} from './ReadLibrary';
import styles from './entry.module.css';

type Captured = NonNullable<ReturnType<typeof useProtectedWebScreenContext>>;
type Resources = Readonly<{
  books: ReturnType<typeof createReadingCollection>;
  resume: ReturnType<typeof createReadingContinueCollection>;
  bindings: ReturnType<typeof createReadBindings>;
  reportError: (error: unknown) => void;
}>;
const {t} = defaultI18n;

/** Real facts only: the accepted author, never a length or a guess the DTO doesn't carry. */
const authorFacts = (author: Readonly<{value: string; source: string}> | undefined): readonly FactPart[] =>
  author ? [{id: 'author', text: author.value, role: 'identity'}] : [];

/** Generated art is the drawn fallback for every item without artwork; it never invents a cover. */
const generated = (item: Readonly<{title: string; author?: Readonly<{value: string}>}>) =>
  <GeneratedCover title={item.title} author={item.author?.value} glyph={Array.from(item.title)[0] ?? ''} tint={generatedCoverTint(item.title)} />;

/** The accepted Continue index already names the actual progressed version. */
function resumeRows(resources: Resources, items: readonly ReadingContinueItem[]): readonly ReadResumeItem[] {
  return items.map(row => ({id: row.item.id, card: {
    model: {title: row.item.title, caption: authorFacts(row.item.author), progress: row.progress.location.progression},
    fallback: generated(row.item), disabled: !row.item.inventoryAvailable, visible: true, playLabel: t('next.reader.read'),
    onNavigate: () => {if (resources.bindings.authority.isCurrent()) void router.navigate({to: '/item/$itemId', params: {itemId: row.item.id}}).catch(resources.reportError);},
    onPlay: row.item.inventoryAvailable ? () => {if (resources.bindings.authority.isCurrent()) void router.navigate(readDestination(row.item.id, row.item.sourceId)).catch(resources.reportError);} : undefined,
  }}));
}

function LibraryRows({resources, libraryId, name, comic, view, onMore, onRetry}: {resources: Resources; libraryId: string; name: string; comic: boolean; view: 'discover' | 'books'; onMore?: (item: ReadingLibraryItem, anchor: HTMLButtonElement) => void; onRetry: () => void}) {
  const grid = useRef<HTMLDivElement>(null), width = useShellWidth(), reducedMotion = useReducedMotion(), [height, setHeight] = useState(600);
  const state = useSyncExternalStore(resources.books.collection.subscribe, resources.books.collection.getSnapshot, resources.books.collection.getSnapshot), failed = state.status === 'error';
  const reading = useSyncExternalStore(resources.resume.collection.subscribe, resources.resume.collection.getSnapshot, resources.resume.collection.getSnapshot);
  const [recent, setRecent] = useState<readonly RecentReadingItem[]>();
  useLayoutEffect(() => {
    const element = grid.current; if (!element) return;
    const read = () => setHeight(Math.max(280, window.innerHeight - element.getBoundingClientRect().top));
    const observer = new ResizeObserver(read); observer.observe(element); window.addEventListener('resize', read); read();
    return () => {observer.disconnect(); window.removeEventListener('resize', read);};
    // The grid mounts only on Books (or when Discover has no rows); re-measure and observe it then.
  }, [failed, reading.status, view]);
  useEffect(() => {resources.resume.collection.ensureRange(0, 11);}, [resources]);
  /** phone-read 1: the server's own recently-added row. An empty row draws nothing; a failed
   *  request is reported through the screen's error channel rather than pretending the library
   *  has no recently-added values (Root §3), and the Reading list stays reachable. */
  useEffect(() => {
    const operation = new AbortController();
    resources.bindings.client.discover({libraryId, limit: 20}, operation.signal)
      .then(response => {if (!operation.signal.aborted) setRecent(response.rows[0]?.items ?? []);})
      .catch(error => {
        if (operation.signal.aborted) return;
        setRecent([]);
        if (resources.bindings.authority.isCurrent()) resources.reportError(error);
      });
    return () => operation.abort();
  }, [resources, libraryId]);
  const cardWidth = width < 600 ? 108 : 190;
  /** The same rows at phone width take the handheld scale and gutters (phone-read 1). */
  const platform = width < 600 ? 'phone-web' as const : 'desktop' as const;
  if (state.status === 'error') return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: onRetry}}/>;
  const resume = reading.status === 'error' ? [] : resources.resume.collection.residentItems();
  if (state.total === 0 && resume.length === 0) return <StateView title={t('library.emptyLibrary')} body={t('library.emptyLibraryBody')}/>;
  const rows: readonly ReadDiscoveryRow[] = [
    ...(resume.length > 0 ? [{id: 'continue', kind: 'resume' as const, heading: t('next.cards.read'), items: resumeRows(resources, resume)}] : []),
    ...(recent && recent.length > 0 ? [{
      id: 'recent', kind: 'covers' as const, heading: t('next.layout.recentIn', {library: name}),
      items: recentCards(recent, {read: t('next.reader.read'),
        open: entry => {if (resources.bindings.authority.isCurrent()) void router.navigate({to: '/item/$itemId', params: {itemId: entry.id}}).catch(resources.reportError);},
        play: entry => {if (resources.bindings.authority.isCurrent()) void router.navigate(readDestination(entry.id, entry.sourceId)).catch(resources.reportError);}}),
    }] : []),
  ];
  /** Root §3: Discover draws the server's rows; Books draws the bounded Reading list. When no
   *  authorized row exists there is nothing to draw on Discover, so the list stays reachable. */
  const showGrid = view === 'books' || rows.length === 0;
  return <div className={styles.library}>
    <ReadDiscovery rows={view === 'books' ? [] : rows} platform={platform} reducedMotion={reducedMotion}
      onVisibleRange={(_row, range) => resources.resume.collection.ensureRange(0, Math.max(0, range.last))}>
      {state.total === 0 || !showGrid ? null : <div ref={grid} className={styles.books} data-view="grid">
        <WindowedGrid collection={resources.books.collection} label={name}
          height={height} cardWidth={cardWidth} rowHeight={cardWidth * 1.5 + 58} gap={20} rowGap={30} renderPlaceholder={() => <Skeleton width={cardWidth} height={cardWidth * 1.5}/>}
          renderItem={item => <CoverCard kind={comic ? 'comic' : 'book'} model={{title: item.title, caption: authorFacts(item.author)}} context="grid" density="grid" visible
            generated={{title: item.title, glyph: Array.from(item.title)[0] ?? '', tint: generatedCoverTint(item.title)}} disabled={!item.inventoryAvailable}
            onNavigate={() => {if (resources.bindings.authority.isCurrent()) void router.navigate({to: '/item/$itemId', params: {itemId: item.id}}).catch(resources.reportError);}}
            onPlay={item.inventoryAvailable ? () => {if (resources.bindings.authority.isCurrent()) void router.navigate(readDestination(item.id, item.sourceId)).catch(resources.reportError);} : undefined}
            playLabel={t('next.reader.read')} onMore={onMore ? anchor => {if (resources.bindings.authority.isCurrent()) onMore(item, anchor);} : undefined}/>} />
      </div>}
    </ReadDiscovery>
  </div>;
}

/** Primary Library registration supplies its drawn header/navigation. This
 * component supplies real bounded Read rows, not an audiobook projection.
 * `view` is the selected library tab: Discover draws the server's rows, Books
 * draws the bounded Reading list (Root §3). */
export function ReadLibraryEntry({libraryId, header, onMore, view = 'discover'}: {libraryId: string; header: ReactNode; onMore?: (item: ReadingLibraryItem, anchor: HTMLButtonElement) => void; view?: 'discover' | 'books'}) {
  const captured = useProtectedWebScreenContext();
  return captured ? <BoundReadLibrary key={libraryId} captured={captured} libraryId={libraryId} header={header} onMore={onMore} view={view}/> : null;
}
function BoundReadLibrary({captured, libraryId, header, onMore, view}: {captured: Captured; libraryId: string; header: ReactNode; onMore?: (item: ReadingLibraryItem, anchor: HTMLButtonElement) => void; view: 'discover' | 'books'}) {
  const context = useLibraryScreenContext(captured);
  const [menu, setMenu] = useState<Readonly<{item: ReadingLibraryItem; anchor: HTMLButtonElement}>>(), [editor, setEditor] = useState<Readonly<{item: ReadingLibraryItem; anchor: HTMLButtonElement}>>();
  const errorSink = useRef(captured.binding.home.effects.reportError); errorSink.current = captured.binding.home.effects.reportError;
  const library = captured.context.facts.identity.libraries.find(library => library.id === libraryId);
  const isReading = library?.kind === 'book' || library?.kind === 'comic';
  const [resources, setResources] = useState<Resources>();
  const [retry, setRetry] = useState(0), [retired, setRetired] = useState(false);
  useEffect(() => {
    if (!isReading) return;
    const bindings = createReadBindings(captured.context), books = createReadingCollection(bindings.client, libraryId),
      resume = createReadingContinueCollection(bindings.client, libraryId);
    setRetired(false);
    setMenu(undefined); setEditor(undefined);
    const off = bindings.authority.subscribe(() => {setRetired(true); setMenu(undefined); setEditor(undefined);});
    setResources({books, resume, bindings, reportError: error => {if (bindings.authority.isCurrent()) errorSink.current(error);}});
    return () => {off();books.dispose();resume.dispose(); void bindings.dispose().catch(() => {});};
  }, [captured.context, libraryId, isReading, retry]);
  const viewer = context.identity?.viewer, canEdit = viewer?.authority === 'local' && viewer.role === 'owner';
  const editorContext = useMemo(() => ({...context, isCurrent: () => context.isCurrent() && !!editor && resources !== undefined && resources.bindings.authority.isCurrent()
    && editor.item.libraryId === libraryId && resources.books.collection.residentItems().includes(editor.item)}), [context, editor, resources, libraryId]);
  const openMore = onMore ?? (canEdit ? (item: ReadingLibraryItem, anchor: HTMLButtonElement) => {
    if (context.isCurrent() && resources?.bindings.authority.isCurrent() && item.libraryId === libraryId && resources.books.collection.residentItems().includes(item)) setMenu({item, anchor});
  } : undefined);
  if (!isReading || !library) return <StateView title={t('next.states.notFound')} body={t('next.states.notFoundBody')}/>;
  const again = () => setRetry(value => value + 1);
  return <>{header}{retired ? <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: again}}/>
    : resources ? <LibraryRows resources={resources} libraryId={libraryId} name={library.name} comic={library.kind === 'comic'} view={view} onMore={openMore} onRetry={again}/> : <Skeleton height={320}/>}
    {menu && canEdit && <Menu open title={menu.item.title} anchorRef={{current: menu.anchor}} onOpenChange={open => {if (!open) setMenu(undefined);}}>
      <MenuItem onSelect={() => {if (context.isCurrent() && resources?.bindings.authority.isCurrent() && resources.books.collection.residentItems().includes(menu.item)) setEditor(menu);}}>{t('entry.editMetadata')}</MenuItem>
    </Menu>}
    {editor && canEdit && <ReadMetadata context={editorContext} itemId={editor.item.id} libraryId={libraryId} anchor={{current: editor.anchor}} onClose={() => {setEditor(undefined); setMenu(undefined); again();}}/>}
  </>;
}
