import {useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore, type ReactNode} from 'react';
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
import {createReadBindings} from './bindings';
import {createReadingCollection, createReadingContinueCollection, type ReadingContinueItem, type ReadingLibraryItem} from './library-collection';
import {readDestination} from './navigation';
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

function LibraryRows({resources, name, comic, onMore, onRetry}: {resources: Resources; name: string; comic: boolean; onMore?: (item: ReadingLibraryItem, anchor: HTMLButtonElement) => void; onRetry: () => void}) {
  const grid = useRef<HTMLDivElement>(null), width = useShellWidth(), reducedMotion = useReducedMotion(), [height, setHeight] = useState(600);
  const state = useSyncExternalStore(resources.books.collection.subscribe, resources.books.collection.getSnapshot, resources.books.collection.getSnapshot), failed = state.status === 'error';
  const reading = useSyncExternalStore(resources.resume.collection.subscribe, resources.resume.collection.getSnapshot, resources.resume.collection.getSnapshot);
  useLayoutEffect(() => {
    const element = grid.current; if (!element) return;
    const read = () => setHeight(Math.max(280, window.innerHeight - element.getBoundingClientRect().top));
    const observer = new ResizeObserver(read); observer.observe(element); window.addEventListener('resize', read); read();
    return () => {observer.disconnect(); window.removeEventListener('resize', read);};
  }, [failed, reading.status]);
  useEffect(() => {resources.resume.collection.ensureRange(0, 11);}, [resources]);
  const cardWidth = width < 600 ? 108 : 190;
  /** The same rows at phone width take the handheld scale and gutters (phone-read 1). */
  const platform = width < 600 ? 'phone-web' as const : 'desktop' as const;
  if (state.status === 'error') return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: onRetry}}/>;
  const resume = reading.status === 'error' ? [] : resources.resume.collection.residentItems();
  if (state.total === 0 && resume.length === 0) return <StateView title={t('library.emptyLibrary')} body={t('library.emptyLibraryBody')}/>;
  const rows: readonly ReadDiscoveryRow[] = resume.length > 0 ? [{id: 'continue', kind: 'resume', heading: t('next.cards.read'), items: resumeRows(resources, resume)}] : [];
  return <div className={styles.library}>
    <ReadDiscovery rows={rows} platform={platform} reducedMotion={reducedMotion}
      onVisibleRange={(_row, range) => resources.resume.collection.ensureRange(0, Math.max(0, range.last))}>
      {state.total === 0 ? null : <div ref={grid} className={styles.books} data-view="grid">
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
 * component supplies real bounded Read rows, not an audiobook projection. */
export function ReadLibraryEntry({libraryId, header, onMore}: {libraryId: string; header: ReactNode; onMore?: (item: ReadingLibraryItem, anchor: HTMLButtonElement) => void}) {
  const captured = useProtectedWebScreenContext();
  return captured ? <BoundReadLibrary key={libraryId} captured={captured} libraryId={libraryId} header={header} onMore={onMore}/> : null;
}
function BoundReadLibrary({captured, libraryId, header, onMore}: {captured: Captured; libraryId: string; header: ReactNode; onMore?: (item: ReadingLibraryItem, anchor: HTMLButtonElement) => void}) {
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
    const off = bindings.authority.subscribe(() => setRetired(true));
    setResources({books, resume, bindings, reportError: error => {if (bindings.authority.isCurrent()) errorSink.current(error);}});
    return () => {off();books.dispose();resume.dispose(); void bindings.dispose().catch(() => {});};
  }, [captured.context, libraryId, isReading, retry]);
  if (!isReading || !library) return <StateView title={t('next.states.notFound')} body={t('next.states.notFoundBody')}/>;
  const again = () => setRetry(value => value + 1);
  return <>{header}{retired ? <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: again}}/>
    : resources ? <LibraryRows resources={resources} name={library.name} comic={library.kind === 'comic'} onMore={onMore} onRetry={again}/> : <Skeleton height={320}/>}</>;
}
