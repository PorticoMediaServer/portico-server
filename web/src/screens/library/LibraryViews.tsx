import React, {forwardRef, useEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {useNavigate} from '@tanstack/react-router';
import type {ContentEntry} from '@core/library-content.ts';
import type {WindowedCollection} from '@core/collections/index.ts';
import {parseBrowseResult, type BrowseSortCapability, type BrowseSortSelection} from '@core/browse.ts';
import {cardCaption, formatClock, formatDuration, iconFor, progressFor, shapeFor} from '@core/presentation/index.ts';
import {useSession} from '../../app/session';
import {useI18n, type MessageId} from '../../app/i18n';
import {useOpenEntry} from '../../app/open';
import {usePlayingItemId} from '../../player/engine';
import {usePlayerActions} from '../../player/PlayerContext';
import type {SequenceContainer} from '../../app/container-play';
import {useRead} from '../../admin/console';
import {valueLabel} from './BrowseFilters';
import type {PlaylistCard} from '@core/saved.ts';
import {SavedSortControl, useSaved} from '../saved/Saved';
import {Artwork, Button, Grid, Icon, Inset, ListRow, Notice, Skeleton, StateView, Surface, Text, WindowedGrid, type MenuAnchor, type WindowedGridHandle} from '../../ui';
import s from './LibraryViews.module.css';

type Column = Readonly<{id: string; label: MessageId; /** The server sort this header drives. */ sort?: string; end?: boolean}>;

const TITLE_COLUMNS: readonly Column[] = [
  {id: 'title', label: 'library.column.title', sort: 'title'},
  {id: 'year', label: 'library.column.year', sort: 'year', end: true},
  {id: 'runtime', label: 'library.column.runtime', sort: 'duration', end: true},
  {id: 'rating', label: 'library.column.rating', sort: 'communityRating', end: true},
  {id: 'resolution', label: 'library.column.resolution', end: true},
  {id: 'added', label: 'library.column.added', sort: 'added', end: true},
];
const TRACK_COLUMNS: readonly Column[] = [
  {id: 'title', label: 'library.column.title', sort: 'title'},
  {id: 'artist', label: 'library.column.artist'},
  {id: 'album', label: 'library.column.album'},
  {id: 'runtime', label: 'library.column.duration', sort: 'duration', end: true},
  // How many times this viewer has played the song to the end.
  {id: 'plays', label: 'library.column.plays', end: true},
  {id: 'added', label: 'library.column.added', sort: 'added', end: true},
];

/** Songs queued after the one pressed. */
const QUEUE_AHEAD = 200;


/**
 * A library's list view (and the Songs tab): a table whose headers sort the list on the server,
 * over the same windowed collection as the grid. Only the rows in view are mounted.
 */
export const TitlesTable = forwardRef(function TitlesTable({collection, libraryId, tracks, sort, sorts, onSort, onFirstVisible, sequence}: {collection: WindowedCollection<ContentEntry>; libraryId: string; /** The list as the server's query: a pressed row plays it on from there, however long. */ sequence?: SequenceContainer; /** Songs: title, artist, album, duration, plays, date added. */ tracks?: boolean; sort: BrowseSortSelection; sorts: readonly BrowseSortCapability[]; /** Absent: the order is the tab's own. */ onSort?: (next: BrowseSortSelection) => void; onFirstVisible?: (index: number) => void}, ref: React.Ref<WindowedGridHandle>) {
  const {t} = useI18n();
  const player = usePlayerActions();
  const rowCount = useSyncExternalStore(collection.subscribe, () => collection.getSnapshot().total);
  const columns = tracks ? TRACK_COLUMNS : TITLE_COLUMNS;
  // A song plays on through the list in the order shown, as every music player does: the whole
  // list, from the server's query, starting at the one pressed. Without the query, the songs
  // already loaded after it are the queue (bounded; nothing is fetched to build it).
  const playFrom = (index: number) => {
    const pressed = collection.itemAt(index);
    if (sequence && pressed?.playback) { player.playSequence([pressed], 0, pressed.playback.startSeconds ?? 0, sequence); return; }
    const queue: ContentEntry[] = [];
    for (let i = index; i < index + QUEUE_AHEAD; i++) { const item = collection.itemAt(i); if (!item) break; if (item.playback) queue.push(item); }
    if (queue.length) player.playSequence(queue, 0, 0);
  };
  const press = (column: Column) => {
    const cap = sorts.find(x => x.id === column.sort);
    if (!cap || !onSort) return;
    const flipped = sort.direction === 'asc' ? 'desc' : 'asc';
    onSort(sort.field === cap.id ? (cap.directions.includes(flipped) ? {field: cap.id, direction: flipped} : sort) : {field: cap.id, direction: cap.defaultDirection});
  };
  return (
    <div className={s.table} data-kind={tracks ? 'tracks' : 'titles'} role="table" aria-label={t('web.library.titlesLabel')} aria-rowcount={rowCount === undefined ? -1 : rowCount + 1}>
      <div className={s.head} role="row" aria-rowindex={1}>
        {columns.map(column => {
          const sortable = !!onSort && !!column.sort && sorts.some(x => x.id === column.sort);
          const current = sort.field === column.sort;
          const cls = [s.headCell, s[column.id], column.end ? s.end : ''].filter(Boolean).join(' ');
          const label = t(column.label);
          return sortable
            ? <button key={column.id} type="button" role="columnheader" className={cls} aria-sort={current ? (sort.direction === 'asc' ? 'ascending' : 'descending') : 'none'} title={t('library.column.sortBy', {column: label})} onClick={() => press(column)}>{label}{current ? <Icon name={sort.direction === 'asc' ? 'chevronUp' : 'chevronDown'} size={12} /> : null}</button>
            : <span key={column.id} role="columnheader" className={cls}>{label}</span>;
        })}
        <span role="columnheader" aria-label={tracks ? undefined : t('card.watched')} />
        <span role="columnheader" />
      </div>
      <WindowedGrid
        ref={ref}
        collection={collection}
        estimateRowHeight={64}
        label={t('web.library.titlesLabel')}
        semantics="none"
        onFirstVisible={onFirstVisible}
        renderItem={(entry, index) => <TitleRow entry={entry} rowIndex={index + 2} libraryId={libraryId} tracks={tracks} onPlayTrack={tracks ? () => playFrom(index) : undefined} />}
        renderPlaceholder={() => <div style={{height: 64, padding: '8px 16px'}}><Skeleton height={48} /></div>}
      />
    </div>
  );
});

const TitleRow = React.memo(function TitleRow({entry, rowIndex, libraryId, tracks, onPlayTrack}: {entry: ContentEntry; /** The row's place in the whole table (the header is row 1), since only the rows in view are drawn. */ rowIndex: number; libraryId: string; tracks?: boolean; onPlayTrack?: () => void}) {
  const i18n = useI18n();
  const {t} = i18n;
  const open = useOpenEntry();
  const navigate = useNavigate();
  const playing = usePlayingItemId() === (entry.playback?.itemId ?? entry.id);
  const progress = progressFor(entry);
  const watched = entry.watched === true;
  const shape = shapeFor(entry.kind, 'list');
  const anchor = (el: Element): MenuAnchor => { const r = el.getBoundingClientRect(); return {x: r.left, y: r.top, width: r.width, height: r.height}; };
  // A track plays where it is; everything else opens its page.
  const activate = () => (tracks && entry.playback && onPlayTrack ? onPlayTrack() : open(entry, 'open', libraryId));
  const library = entry.libraryId ?? libraryId;
  const caption = tracks ? undefined : entry.kind === 'movie' || entry.kind === 'show' ? undefined : cardCaption(entry, t, {locale: i18n.locale});
  return (
    <div className={s.row} role="row" aria-rowindex={rowIndex} data-playing={playing || undefined}>
      <button type="button" className={s.main} role="cell" onClick={activate} aria-label={tracks ? t('card.play', {title: entry.title}) : entry.title}>
        <span className={s.art} data-shape={shape}><Artwork path={entry.posterUrl ?? entry.backdropUrl} shape={shape === 'circle' ? 'square' : shape} icon={iconFor(entry.kind)} alt="" /></span>
        <span className={s.text}><span className={s.name}>{entry.title}</span>{caption ? <span className={s.caption}>{caption}</span> : null}</span>
      </button>
      {tracks ? (
        <>
          <span role="cell" className={`${s.cell} ${s.artist}`}>{entry.subtitle ?? ''}</span>
          <span role="cell" className={`${s.cell} ${s.album}`}>{entry.album ? <button type="button" className={s.link} onClick={() => void navigate({to: '/library/$libraryId/$view/$entityId', params: {libraryId: library, view: 'album', entityId: entry.album!.id}})}>{entry.album.name}</button> : ''}</span>
          <span role="cell" className={`${s.cell} ${s.end} ${s.runtime}`}>{entry.duration ? formatClock(entry.duration) : ''}</span>
          <span role="cell" className={`${s.cell} ${s.end} ${s.plays}`}>{entry.plays ?? ''}</span>
          <span role="cell" className={`${s.cell} ${s.end} ${s.added}`}>{entry.addedAt ? i18n.date(entry.addedAt) : ''}</span>
        </>
      ) : (
        <>
          <span role="cell" className={`${s.cell} ${s.end} ${s.year}`}>{entry.year ?? ''}</span>
          <span role="cell" className={`${s.cell} ${s.end} ${s.runtime}`}>{formatDuration(entry.duration)}</span>
          <span role="cell" className={`${s.cell} ${s.end} ${s.rating}`}>{entry.rating ? entry.rating.toFixed(1) : ''}</span>
          <span role="cell" className={`${s.cell} ${s.end} ${s.resolution}`}>{entry.resolution ? valueLabel(entry.resolution) : ''}</span>
          <span role="cell" className={`${s.cell} ${s.end} ${s.added}`}>{entry.addedAt ? i18n.date(entry.addedAt) : ''}</span>
        </>
      )}
      <span role="cell" className={s.state}>{tracks ? null : watched ? <Icon name="check" size={16} aria-label={t('card.watched')} /> : progress ? <Text variant="caption" tone="tertiary">{`${Math.round(progress * 100)}%`}</Text> : null}</span>
      <span role="cell" className={s.actions}>
        {!tracks && entry.playback ? <Button variant="ghost" size="sm" icon="play" aria-label={t('card.play', {title: entry.title})} onClick={() => open(entry, 'play', libraryId)} /> : null}
        <Button variant="ghost" size="sm" icon="more" aria-label={t('card.more', {title: entry.title})} onClick={(e: React.MouseEvent<HTMLElement>) => open(entry, 'more', libraryId, anchor(e.currentTarget))} />
      </span>
    </div>
  );
});

/** Music › Genres: one genre, with covers from it. A genre opens Albums filtered to it. Its covers are read when the card comes into view: the cost is the cards on screen. */
export function GenreCard({libraryId, entry, onOpen}: {libraryId: string; entry: ContentEntry; onOpen: () => void}) {
  const {api} = useSession();
  const {t} = useI18n();
  const node = useRef<HTMLButtonElement>(null);
  const [covers, setCovers] = useState<readonly (string | undefined)[]>(entry.artworkPaths ?? []);
  useEffect(() => {
    const el = node.current;
    if (!el || covers.length || typeof IntersectionObserver === 'undefined') return;
    let live = true;
    const io = new IntersectionObserver(seen => {
      if (!seen.some(x => x.isIntersecting)) return;
      io.disconnect();
      api.request<unknown>(`/v1/libraries/${encodeURIComponent(libraryId)}/browse`, 'POST', {pivot: 'albums', query: {all: [{field: 'genre', operator: 'in', value: [entry.title]}]}, sort: [{field: 'added', direction: 'desc'}], limit: 4})
        .then(raw => { if (live) setCovers(parseBrowseResult(raw).entries.map(e => e.posterUrl)); }, () => {});
    }, {rootMargin: '300px'});
    io.observe(el);
    return () => { live = false; io.disconnect(); };
  }, [api, libraryId, entry.title, covers.length]);
  return (
    <button ref={node} type="button" className={s.genre} onClick={onOpen} aria-label={entry.title}>
      {/* Four covers make a mosaic; fewer, the first one whole. */}
      {covers.filter(Boolean).length >= 4 ? <span className={s.mosaic}>{[0, 1, 2, 3].map(i => <Artwork key={i} path={covers[i]} shape="square" icon="music" alt="" />)}</span> : <Artwork path={covers[0]} shape="square" icon="music" alt="" />}
      <span className={s.genreText}>
        <Text as="span" variant="bodyStrong">{entry.title}</Text>
        {entry.count ? <Text as="span" variant="caption" tone="secondary">{t('title.songCount', {count: entry.count})}</Text> : null}
      </span>
    </button>
  );
}

/** Music › Playlists: the viewer's playlists, as on Saved. */
export function LibraryPlaylists({sort, onSort}: {/** The order chosen on this tab; the list's own when none was. */ sort?: BrowseSortSelection; onSort: (next: BrowseSortSelection) => void}) {
  const {t} = useI18n();
  const navigate = useNavigate();
  const route = useMemo(() => ({view: 'playlists' as const, ...(sort ? {sort: sort.field, direction: sort.direction} : {})}), [sort?.field, sort?.direction]); // eslint-disable-line react-hooks/exhaustive-deps
  const saved = useSaved(route);
  const snapshot = saved.snapshot;
  const cards = (snapshot.projection?.sections ?? []).flatMap(section => section.entries).filter((e): e is PlaylistCard => e.kind === 'playlist');
  if (snapshot.phase === 'error' && !cards.length) return <Inset><Notice tone="error" action={{label: t('action.tryAgain'), onClick: saved.retry}}>{t('library.playlists.loadFailed')}</Notice></Inset>;
  if (snapshot.phase === 'loading' && !cards.length) return <Inset><Skeleton height={64} /></Inset>;
  if (!cards.length) return <StateView icon="queue" title={t('web.saved.empty.playlists')} body={t('web.saved.empty.playlistsBody')} />;
  const projection = snapshot.projection;
  // The same Sort control as every other tab, with the sorts the server offers a list of playlists.
  const sortBar = projection?.sorts?.length ? <Inset><SavedSortControl sorts={projection.sorts} active={sort?.field ?? projection.query.sort ?? projection.sorts[0]?.id} direction={sort?.direction ?? (projection.query.direction === 'desc' ? 'desc' : 'asc')} onChange={(field, direction) => onSort({field, direction})} /></Inset> : null;
  return <>{sortBar}<Inset><Surface padless>{cards.map(card => <ListRow key={card.id} icon="queue" title={card.title} subtitle={card.subtitle} meta={card.count != null ? t('web.saved.items', {count: card.count}) : undefined} trailingIcon="forward" onClick={() => void navigate({to: '/saved/$kind/$resourceId', params: {kind: 'playlist', resourceId: card.navigation.entityId}, search: {title: card.title}})} />)}</Surface></Inset></>;
}
