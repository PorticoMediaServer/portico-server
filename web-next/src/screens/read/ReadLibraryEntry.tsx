import {useEffect, useLayoutEffect, useRef, useState, useSyncExternalStore, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {generatedCoverTint} from '@design';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {useShellWidth} from '../../shell/viewport';
import {WindowedGrid} from '../../kit/layout';
import {CoverCard} from '../../kit/cards';
import {Skeleton, StateView} from '../../kit/states';
import {createReadBindings} from './bindings';
import {createReadingCollection, type ReadingLibraryItem} from './library-collection';
import {readDestination} from './navigation';
import styles from '../browse/browse.module.css';

type Captured = NonNullable<ReturnType<typeof useProtectedWebScreenContext>>;
type Resources = ReturnType<typeof createReadingCollection> & {bindings: ReturnType<typeof createReadBindings>; reportError: (error: unknown) => void};
const {t} = defaultI18n;
function LibraryRows({resources, name, comic, onMore, onRetry}: {resources: Resources; name: string; comic: boolean; onMore?: (item: ReadingLibraryItem, anchor: HTMLButtonElement) => void; onRetry: () => void}) {
  const root = useRef<HTMLDivElement>(null), width = useShellWidth(), [height, setHeight] = useState(600);
  const state = useSyncExternalStore(resources.collection.subscribe, resources.collection.getSnapshot, resources.collection.getSnapshot), failed = state.status === 'error';
  useLayoutEffect(() => {
    const element = root.current; if (!element) return;
    const read = () => setHeight(Math.max(280, window.innerHeight - element.getBoundingClientRect().top));
    const observer = new ResizeObserver(read); observer.observe(element); window.addEventListener('resize', read); read();
    return () => {observer.disconnect(); window.removeEventListener('resize', read);};
  }, [failed]);
  const cardWidth = width < 600 ? 108 : 190;
  if (state.status === 'error') return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: onRetry}}/>;
  return <div ref={root} className={styles.results} data-view="grid">
    {state.total === 0 ? <StateView title={t('library.emptyLibrary')} body={t('library.emptyLibraryBody')}/> : <WindowedGrid collection={resources.collection} label={name}
      height={height} cardWidth={cardWidth} rowHeight={cardWidth * 1.5 + 58} gap={20} rowGap={30} renderPlaceholder={() => <Skeleton width={cardWidth} height={cardWidth * 1.5}/>}
      renderItem={item => <CoverCard kind={comic ? 'comic' : 'book'} model={{title: item.title, caption: []}} context="grid" density="grid" visible
        generated={{title: item.title, glyph: Array.from(item.title)[0] ?? '', tint: generatedCoverTint(item.title)}} disabled={!item.inventoryAvailable}
        onNavigate={() => {if (resources.bindings.authority.isCurrent()) void router.navigate({to: '/item/$itemId', params: {itemId: item.id}}).catch(resources.reportError);}}
        onPlay={item.inventoryAvailable ? () => {if (resources.bindings.authority.isCurrent()) void router.navigate(readDestination(item.id, item.sourceId)).catch(resources.reportError);} : undefined}
        playLabel={t('next.reader.read')} onMore={onMore ? anchor => {if (resources.bindings.authority.isCurrent()) onMore(item, anchor);} : undefined}/>} />}
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
  const reading = library?.kind === 'book' || library?.kind === 'comic';
  const [resources, setResources] = useState<Resources>();
  const [retry, setRetry] = useState(0), [retired, setRetired] = useState(false);
  useEffect(() => {
    if (!reading) return;
    const bindings = createReadBindings(captured.context), rows = createReadingCollection(bindings.client, libraryId);
    setRetired(false);
    const off = bindings.authority.subscribe(() => setRetired(true));
    setResources({...rows, bindings, reportError: error => {if (bindings.authority.isCurrent()) errorSink.current(error);}});
    return () => {off(); rows.dispose(); void bindings.dispose().catch(() => {});};
  }, [captured.context, libraryId, reading, retry]);
  if (!reading || !library) return <StateView title={t('next.states.notFound')} body={t('next.states.notFoundBody')}/>;
  const again = () => setRetry(value => value + 1);
  return <>{header}{retired ? <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: again}}/>
    : resources ? <LibraryRows resources={resources} name={library.name} comic={library.kind === 'comic'} onMore={onMore} onRetry={again}/> : <Skeleton height={320}/>}</>;
}
