import {useCallback, useLayoutEffect, useRef, useState, useSyncExternalStore, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {timelineFacts, type TextEnvironment} from '@portico/client-core/kit-model';
import {durationText} from '@core/presentation/kit-model/text.ts';
import type {BrowseSortSelection} from '@core/browse.ts';
import type {ContentEntry} from '@core/library-content.ts';
import {AlphabetRail, WindowedGrid, type WindowedGridHandle} from '../../kit/layout';
import {gridColumns} from '../../kit/layout/geometry';
import {Skeleton, StateView} from '../../kit/states';
import {Artwork, GeneratedArt} from '../../kit/artwork';
import {Button} from '../../kit/controls';
import {Icon} from '../../kit/Icon';
import {SelectionCircle} from '../../kit/selection';
import type {ShellArtwork} from '../../shell/artwork';
import {BrowseEntryCard, type BrowseItemActions} from './EntryCard';
import type {BrowseController, BrowseMember} from './controller';
import styles from './browse.module.css';

export type BrowseResultsProps = Readonly<{
  controller: BrowseController; label: string; view: 'grid' | 'list'; cardWidth: number; shape: 'poster' | 'square' | 'collection';
  artwork: ShellArtwork; env: TextEnvironment; reducedMotion: boolean; backdrops: boolean;
  actions: (member: BrowseMember, index: number) => BrowseItemActions;
  /** The actual selected viewer's library list supplies family for mixed rows. */
  familyForEntry?: (entry: ContentEntry) => string | undefined;
  onVisibleRange?: (first: number, last: number) => void;
  onSort: (sort: BrowseSortSelection) => void;
  empty?: ReactNode;
}>;
const t = defaultI18n.t;
// Frames #grid/#big draw the poster grid as `repeat(7, minmax(0, 1fr))`: the card fills its
// column, so the painted width is the measured column at the reported Medium size, never a
// fixed width that leaves the row short of the gutter.
const gridGap = 20, gridRowGap = 30;
// The grid measures one pitch for every row, so it must cover the tallest drawn card: 10px
// caption offset, two 14px/1.3 title lines, the 2px year offset and one 13px line at the
// font's 1.4 normal leading. Frames #grid (49px, one line) and #big (66.6px, two lines) agree.
const gridCaption = 66.6;
const columns = [
  {id: 'title', label: 'library.column.title'}, {id: 'year', label: 'library.column.year'},
  {id: 'duration', label: 'library.column.runtime'}, {id: 'communityRating', label: 'library.column.rating'},
  {id: 'resolution', label: 'library.column.resolution'}, {id: 'added', label: 'library.column.added'},
] as const;
export function BrowseResults(props: BrowseResultsProps) {
  const {controller, view, shape, cardWidth, onVisibleRange} = props;
  const root = useRef<HTMLDivElement>(null), grid = useRef<WindowedGridHandle>(null);
  const [size, setSize] = useState({width: 0, grid: 0, height: 600}), [visible, setVisible] = useState({first: -1, last: -1});
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const snapshot = useSyncExternalStore(controller.collection.subscribe, controller.collection.getSnapshot, controller.collection.getSnapshot);
  const failed = snapshot.status === 'error';
  useLayoutEffect(() => {
    const element = root.current; if (!element) return;
    // The grid owns its scrollbar, so its content width — not the outer box — decides the fill.
    const grids = new Set<Element>();
    let observer: ResizeObserver;
    const read = () => {
      const bounds = element.getBoundingClientRect(), node = element.querySelector<HTMLElement>('[role="list"]');
      if (node && !grids.has(node)) {grids.add(node);observer.observe(node);}
      setSize({width: element.clientWidth, grid: node?.clientWidth ?? 0, height: Math.max(280, window.innerHeight - bounds.top)});
    };
    observer = new ResizeObserver(read);
    observer.observe(element); window.addEventListener('resize', read);read();
    return () => {observer.disconnect();window.removeEventListener('resize', read);};
  }, [failed]);
  const range = useCallback((first: number, last: number) => {setVisible(before => before.first === first && before.last === last ? before : {first, last});onVisibleRange?.(first, last);}, [onVisibleRange]);
  const anchors = Object.entries(snapshot.positionIndex ?? {}).map(([key, index]) => ({key, index}));
  const list = view === 'list', ratio = shape === 'poster' ? 2 / 3 : shape === 'collection' ? 1.9 : 1;
  const musicGenres = state.capabilities?.library.kind === 'music' && controller.source.pivot === 'genres';
  const painted = size.width > 0 ? gridColumns(size.width, cardWidth, gridGap) : 1;
  // Fill the grid's own content box so every row reaches the gutter. The hundredth of a pixel
  // keeps the canonical column count exactly `painted` when the grid re-derives it from this width.
  const width = list ? Math.max(1, size.width) : size.grid > 0 ? (size.grid - (painted - 1) * gridGap) / painted - .01 : cardWidth;
  const height = musicGenres ? 152 : width / ratio + (shape === 'collection' ? 0 : gridCaption);
  if (failed) return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: controller.retry}}/>;
  return <div className={styles.results} ref={root} data-view={view}>
    {list && <div className={styles.listHead} role="row">{columns.map(column => {
      const offered = state.capabilities?.sorts.find(sort => sort.id === column.id);
      return <div key={column.id} role="columnheader" aria-sort={state.sort?.field === column.id ? state.sort.direction === 'asc' ? 'ascending' : 'descending' : 'none'}>
        {offered ? <button type="button" onClick={() => props.onSort({field: offered.id, direction: state.sort?.field === offered.id && offered.directions.length > 1 ? state.sort.direction === 'asc' ? 'desc' : 'asc' : offered.defaultDirection})}>{t(column.label)}{state.sort?.field === column.id && <Icon name={state.sort.direction === 'asc' ? 'i-up' : 'i-down'} size={14}/>}</button> : t(column.label)}
      </div>;
    })}<span aria-label={t('next.cards.watched')} role="columnheader"/><span/></div>}
    {snapshot.total === 0 ? props.empty : <WindowedGrid collection={controller.collection} ref={grid} label={props.label} height={size.height - (list ? 44 : 0)}
      cardWidth={width} rowHeight={list ? 70 : height} gap={list ? 0 : gridGap} rowGap={list ? 0 : gridRowGap} reducedMotion={props.reducedMotion}
      onVisibleRange={range} renderPlaceholder={() => <Skeleton height={list ? 54 : musicGenres ? 152 : width / ratio} width={list ? '100%' : width}/>}
      renderItem={(member, context) => list ? <BrowseListRow {...props} member={member} index={context.index}/> : <BrowseEntryCard entry={member.entry} actions={props.actions(member, context.index)} artwork={props.artwork} env={props.env}
        width={width} family={props.familyForEntry?.(member.entry) ?? state.capabilities?.library.kind} mixed={controller.source.libraryId === '*'} visible order={context.index} group={controller.source.libraryId} backdrops={props.backdrops}/>} />}
    {state.sort?.field === 'title' && anchors.length > 0 && <AlphabetRail anchors={anchors} visibleRange={visible} onJump={anchor => grid.current?.scrollToIndex(anchor.index)} className={styles.rail}/>}
  </div>;
}
function BrowseListRow({member, index, artwork, env, actions}: BrowseResultsProps & {member: BrowseMember; index: number}) {
  const source = useRef<'pointer' | 'keyboard'>('pointer');
  const {entry} = member, title = entry.displayTitle ?? entry.title, action = actions(member, index), progress = timelineFacts(entry.duration, entry.progressSeconds).progress;
  const art = artwork.foreground(entry.posterUrl, 72 * devicePixelRatio, 2 / 3, {group: entry.libraryId, order: index});
  const added = entry.addedAt && Number.isFinite(Date.parse(entry.addedAt)) ? new Intl.DateTimeFormat(env.locale ?? 'en-US', {month: 'short', day: 'numeric', year: 'numeric'}).format(new Date(entry.addedAt)) : undefined;
  const resolution = entry.resolution === '4k' ? t('quality.4k') : entry.resolution === '1080p' ? t('downloads.quality.1080p') : entry.resolution === '720p' ? t('downloads.quality.720p') : entry.resolution === 'sd' ? t('quality.sd') : undefined;
  return <div className={styles.listRow} data-browse-entry={entry.id} onKeyDown={event => {if (event.key === 'Escape' && action.selection?.active) {event.preventDefault();action.selection.onExit();}}}>
    <div className={styles.listTitle}>{action.selection && <SelectionCircle selected={action.selection.checked} selecting={action.selection.active} onSelectedChange={(_next, intent) => action.selection?.onToggle({range: intent.intent === 'range' || intent.shiftKey === true, source: source.current})} onKeyDownCapture={() => {source.current = 'keyboard';}} onPointerDownCapture={() => {source.current = 'pointer';}} accessibilityLabel={t('next.selection.select')} variant="row"/>}
      <button type="button" data-layout-primary onClick={action.onOpen}><span className={styles.listArt}><Artwork source={art} visible alt="" fallback={<GeneratedArt glyph={Array.from(title)[0] ?? ''}/>}/></span><span>{title}</span></button></div>
    <span>{entry.year}</span><span>{durationText(entry.duration, env.t)}</span><span>{entry.rating !== undefined && <><Icon name="i-star" size={13}/>{defaultI18n.number(entry.rating, {maximumFractionDigits: 1})}</>}</span>
    <span>{resolution && <span className={styles.resolution}>{resolution}</span>}</span><span>{added}</span>
    <span>{entry.watched ? <Icon name="i-check" size={18}/> : progress !== undefined && <span className={styles.miniProgress}><span style={{width: `${progress * 100}%`}}/></span>}</span>
    <span className={styles.listActions}>{action.onPlay && <Button variant="primary" size="small" icon="i-play" iconOnly onClick={action.onPlay} aria-label={t('next.browse.play', {title})}/>}{action.onMore && <Button variant="icon" size="small" icon="i-more" iconOnly onClick={event => action.onMore?.(event.currentTarget)} aria-label={t('next.browse.moreFor', {title})}/>}</span>
  </div>;
}
