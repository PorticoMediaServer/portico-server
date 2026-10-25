import {useEffect, useRef, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {generatedCoverTint} from '@design';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {CoverCard} from '../../kit/cards';
import {WindowedGrid} from '../../kit/layout';
import {Skeleton, StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import {createReadBindings} from './bindings';
import {createReadingSeriesCollection} from './library-collection';
import {readDestination} from './navigation';
import styles from './entry.module.css';

const {t} = defaultI18n;
type Captured = NonNullable<ReturnType<typeof useProtectedWebScreenContext>>;
type Resources = Readonly<{comic: boolean; bindings: ReturnType<typeof createReadBindings>; rows: ReturnType<typeof createReadingSeriesCollection>; reportError: (error: unknown) => void}>;

/** The canonical series cursor owns complete membership and ordering, including unnumbered volumes. */
export function ReadSeriesEntry({libraryId, seriesId}: {libraryId: string; seriesId: string}) {
  const captured = useProtectedWebScreenContext();
  return captured ? <BoundSeries key={JSON.stringify([libraryId, seriesId])} captured={captured} libraryId={libraryId} seriesId={seriesId}/> : null;
}
function BoundSeries({captured, libraryId, seriesId}: {captured: Captured; libraryId: string; seriesId: string}) {
  const [resources, setResources] = useState<Resources>(), [retry, setRetry] = useState(0), [retired, setRetired] = useState(false);
  useEffect(() => {
    const bindings = createReadBindings(captured.context), rows = createReadingSeriesCollection(bindings.client, libraryId, seriesId);
    setRetired(false);
    const off = bindings.authority.subscribe(() => {setRetired(true); setResources(undefined);});
    setResources({bindings, rows, comic: captured.context.facts.identity.libraries.find(library => library.id === libraryId)?.kind === 'comic', reportError: error => {if (bindings.authority.isCurrent()) captured.binding.home.effects.reportError(error);}});
    return () => {off(); rows.dispose(); void bindings.dispose().catch(() => {});};
  }, [captured.context, libraryId, seriesId, retry]);
  if (retired) return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => setRetry(value => value + 1)}}/>;
  return resources ? <SeriesRows resources={resources} retry={() => setRetry(value => value + 1)}/> : <Skeleton height={320}/>;
}
function SeriesRows({resources, retry}: {resources: Resources; retry: () => void}) {
  const state = useSyncExternalStore(resources.rows.collection.subscribe, resources.rows.collection.getSnapshot, resources.rows.collection.getSnapshot);
  const width = useShellWidth(), grid = useRef<HTMLDivElement>(null), [height, setHeight] = useState(600);
  useEffect(() => {
    const element = grid.current; if (!element) return;
    const measure = () => setHeight(Math.max(280, window.innerHeight - element.getBoundingClientRect().top));
    const observer = new ResizeObserver(measure); observer.observe(element); window.addEventListener('resize', measure); measure();
    return () => {observer.disconnect(); window.removeEventListener('resize', measure);};
  }, [state.status]);
  const title = resources.rows.collection.residentItems().find(item => item.series)?.series?.name;
  const report = (error: unknown) => {resources.reportError(error);};
  const cardWidth = width < 600 ? 108 : 190;
  if (state.status === 'error') return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: retry}}/>;
  return <section className={styles.library}>
    {title && <h2>{title}</h2>}
    <div ref={grid} className={styles.books}><WindowedGrid collection={resources.rows.collection} label={title ?? t('library.tab.series')}
      height={height} cardWidth={cardWidth} rowHeight={cardWidth * 1.5 + 58} gap={20} rowGap={30}
      renderPlaceholder={() => <Skeleton width={cardWidth} height={cardWidth * 1.5}/>}
      renderItem={item => <CoverCard kind={item.format === 'cbz' ? 'comic' : 'book'} context="grid" density="grid" visible
        model={{title: item.title, caption: [...(item.series?.number !== undefined ? [{id: 'number', text: t(resources.comic ? 'next.reader.volumeNumber' : 'next.reader.bookNumber', {number: defaultI18n.number(item.series.number, {maximumFractionDigits: 20})}), role: 'identity' as const}] : []), ...(item.author ? [{id: 'author', text: item.author.value, role: 'identity' as const}] : [])]}}
        generated={{title: item.title, glyph: Array.from(item.title)[0] ?? '', tint: generatedCoverTint(item.title)}} disabled={!item.inventoryAvailable}
        onNavigate={() => {if (resources.bindings.authority.isCurrent()) void router.navigate({to: '/item/$itemId', params: {itemId: item.id}}).catch(report);}}
        onPlay={item.inventoryAvailable ? () => {if (resources.bindings.authority.isCurrent()) void router.navigate(readDestination(item.id, item.sourceId)).catch(report);} : undefined}
        playLabel={t('next.reader.read')}/>} /></div>
  </section>;
}
