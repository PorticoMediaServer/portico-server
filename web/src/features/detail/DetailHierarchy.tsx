import { BookOpen, CheckCircle2, FolderHeart, ListMusic, Music2, Radio, RefreshCw, Tv } from '#portico-icons';
import { productMessage, type ProductMessageId } from '@portico/client-core';
import { useEffect, useMemo, useRef, useState } from 'react';
import { Link, useSearchParams } from 'react-router-dom';
import { SecondaryButton } from '../../components/controls/Buttons';
import { SelectMenu } from '../../components/controls/SelectMenu';
import { ProductLanguageIcon, productLanguageProblem } from '../../components/states/ProductLanguageState';
import { usePorticoDataSource } from '../../data/DataProvider';
import type { MediaChapter, MediaChildrenPage, MediaItem, PlaybackStartOptions } from '../../data/models';
import { MediaActionMenu, MediaArtwork, SectionHeading, SelectableMediaGrid, mediaDetailPath } from '../catalog/CatalogSurface';
import { targetFromMedia } from '../media/contextTarget';
import { actionPresentation, MediaActionIcon, useMediaActionPresentations } from '../media/MediaActionPresentation';
import { usePlaybackSession } from '../player/PlayerSurface';
import { playbackOptionsForItems, watchNavigationState } from '../player/watchNavigation';
import { detailArtworkShape, detailChildTitle, detailEmptyChildCopy, detailKind, detailPlaybackContext, formatResumeTime, orderedDetailItems } from './detailModel';

function childDetail(item: MediaItem, count: number) {
  const kind = detailKind(item);
  const messages: Record<string, [ProductMessageId, ProductMessageId]> = {
    show: ['media.season-count-single', 'media.season-count'],
    season: ['media.episode-count-single', 'media.episode-count'],
    artist: ['media.release-count-single', 'media.release-count'],
    album: ['media.track-count-single', 'media.track-count'],
    author: ['media.audiobook-count-single', 'media.audiobook-count'],
    book: ['media.chapter-count-single', 'media.chapter-count'],
    series: ['media.audiobook-count-single', 'media.audiobook-count'],
    collection: ['media.item-count-single', 'media.item-count'],
    playlist: ['media.item-count-single', 'media.item-count'],
    channel: ['media.program-count-single', 'media.program-count'],
  };
  const ids = messages[kind] ?? ['media.item-count-single', 'media.item-count'];
  return productMessage(count === 1 ? ids[0] : ids[1], { count }).text ?? '';
}

function HierarchyEmpty({ item }: { item: MediaItem }) {
  const kind = detailKind(item);
  const Icon = kind === 'album' || kind === 'artist' ? Music2
    : kind === 'author' || kind === 'book' || kind === 'series' ? BookOpen
      : kind === 'playlist' ? ListMusic
        : kind === 'collection' ? FolderHeart
          : kind === 'channel' ? Radio
            : Tv;
  return <div className="portico-detail-inline-state empty" role="status">
    <Icon />
    <span><strong>{productMessage('media.children-unavailable-title', { section: detailChildTitle(item) }).text}</strong><small>{detailEmptyChildCopy(item)}</small></span>
  </div>;
}

function episodeNumberLabel(item: MediaItem) {
  if (item.seasonNumber != null && item.episodeNumber != null) return productMessage('media.episode-code', { seasonNumber: item.seasonNumber, episodeNumber: item.episodeNumber }).text;
  if (item.episodeNumber != null) return productMessage('media.episode-number', { episodeNumber: item.episodeNumber }).text;
  return productMessage('media.episode-label').text;
}

function EpisodeRow({ item, selected, onSelect, playbackOptions }: { item: MediaItem; selected: boolean; onSelect: () => void; playbackOptions: PlaybackStartOptions }) {
  const progress = Math.max(0, Math.min(100, Math.round(item.progress ?? 0)));
  const resumeTime = item.progressSeconds && item.progressSeconds > 0 ? formatResumeTime(item.progressSeconds) : '';
  const presentedActions = useMediaActionPresentations(item.actions ?? []);
  const playAction = actionPresentation(presentedActions, 'play');
  const runtimeUnavailable = productMessage('media.runtime-unavailable').text;
  const watched = productMessage('media.watched').text;
  const resumeFrom = productMessage('media.resume-from', { time: resumeTime }).text;
  const playLabel = productMessage(progress ? 'action.resume-item' : 'action.play-item', { title: item.title }).text;
  return <article className={`show-episode-row ${selected ? 'selected' : ''}`} data-episode-id={item.id}>
    <button type="button" className="show-episode-main" onClick={onSelect} aria-pressed={selected}>
      <span className="show-episode-art"><MediaArtwork item={{ ...item, poster: item.backdrop || item.poster }} />{progress > 0 && !item.watched && <span className="show-episode-progress"><span style={{ width: `${progress}%` }} /></span>}</span>
      <span className="show-episode-copy">
        <span className="show-episode-heading"><span>{episodeNumberLabel(item)}</span><strong>{item.title}</strong></span>
        {item.summary && <span className="show-episode-summary">{item.summary}</span>}
        <span className="show-episode-meta">{item.length || runtimeUnavailable}{item.watched ? <><i aria-hidden="true" /><CheckCircle2 /> {watched}</> : resumeTime ? <><i aria-hidden="true" />{resumeFrom}</> : null}</span>
      </span>
    </button>
    <div className="show-episode-actions">
      {playAction && <Link className="show-episode-play" to={`/watch/${encodeURIComponent(item.id)}`} state={watchNavigationState(playbackOptions)} aria-label={playLabel}><MediaActionIcon action={playAction} /></Link>}
      <MediaActionMenu target={targetFromMedia(item)} playbackOptions={playbackOptions} />
    </div>
  </article>;
}

function trackNumber(item: MediaItem, index: number) {
  const value = item.typedMetadata?.trackNumber ?? item.typedMetadata?.track ?? item.typedMetadata?.trackIndex;
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? parsed : index + 1;
}

function trackArtist(item: MediaItem, album: MediaItem) {
  return item.typedMetadata?.trackArtist
    || item.typedMetadata?.artist
    || item.grandparentTitle
    || album.typedMetadata?.albumArtist
    || album.typedMetadata?.artist
    || album.parentTitle
    || productMessage('media.unknown-artist').text;
}

function AlbumTrackRow({ track, album, index, playbackOptions }: { track: MediaItem; album: MediaItem; index: number; playbackOptions: PlaybackStartOptions }) {
  const playback = usePlaybackSession();
  const presentedActions = useMediaActionPresentations(track.actions ?? []);
  const playAction = actionPresentation(presentedActions, 'play');
  const detailPath = mediaDetailPath(track);
  return <article className="album-track-row" role="row">
    <span className="album-track-number" role="cell">{trackNumber(track, index)}</span>
    <span className="album-track-name" role="cell">
      {detailPath ? <Link to={detailPath}>{track.title}</Link> : <strong>{track.title}</strong>}
    </span>
    <span className="album-track-artist" role="cell">{trackArtist(track, album)}</span>
    <span className="album-track-duration" role="cell">{track.length}</span>
    <span className="album-track-actions" role="cell">
      {playAction && <button type="button" className="album-track-play" onClick={() => void playback.start(track.id, playbackOptions)} aria-label={productMessage('action.play-item', { title: track.title }).text}><MediaActionIcon action={playAction} /></button>}
      <MediaActionMenu target={targetFromMedia(track)} playbackOptions={playbackOptions} />
    </span>
  </article>;
}

function AlbumTrackList({ album, tracks }: { album: MediaItem; tracks: MediaItem[] }) {
  const ordered = orderedDetailItems(tracks);
  const playbackOptions = playbackOptionsForItems(ordered, detailPlaybackContext(album));
  return <div className="album-track-list" role="table" aria-label={productMessage('media.track-list-label', { title: album.title }).text}>
    <div className="album-track-header" role="row">
      <span className="album-track-number-header" role="columnheader" aria-label={productMessage('media.track-number-label').text}>#</span>
      <span className="album-track-name-header" role="columnheader">{productMessage('media.column-name').text}</span>
      <span className="album-track-artist-header" role="columnheader">{productMessage('media.column-artist').text}</span>
      <span className="album-track-duration" role="columnheader">{productMessage('media.column-time').text}</span>
      <span className="album-track-actions-header" role="columnheader" aria-label={productMessage('media.column-actions').text} />
    </div>
    {ordered.map((track, index) => <AlbumTrackRow key={track.id} track={track} album={album} index={index} playbackOptions={playbackOptions} />)}
  </div>;
}

function AudiobookChapterRow({ book, chapter, index }: { book: MediaItem; chapter: MediaChapter; index: number }) {
  const presentedActions = useMediaActionPresentations(book.actions ?? []);
  const playAction = actionPresentation(presentedActions, 'play');
  const label = productMessage('action.play-chapter', { chapter: chapter.title }).text;
  const content = <>
    <span className="album-track-number">{index + 1}</span>
    <span className="album-track-name"><strong>{chapter.title}</strong></span>
    <span className="album-track-duration">{formatResumeTime(chapter.startSeconds)}</span>
    <span className="album-track-actions">{playAction && <MediaActionIcon action={playAction} />}</span>
  </>;
  return playAction
    ? <div role="listitem"><Link className="album-track-row audiobook-chapter-row" to={`/watch/${encodeURIComponent(book.id)}`} state={watchNavigationState({ startSeconds: chapter.startSeconds })} aria-label={label}>{content}</Link></div>
    : <div className="album-track-row audiobook-chapter-row" role="listitem" aria-label={chapter.title}>{content}</div>;
}

function AudiobookChapterList({ book, chapters }: { book: MediaItem; chapters: MediaChapter[] }) {
  const label = productMessage('media.chapters-title').text ?? '';
  if (!chapters.length) return <HierarchyEmpty item={book} />;
  return <div className="album-track-list audiobook-chapter-list" role="list" aria-label={productMessage('media.chapter-list-label', { title: book.title }).text}>
    <div className="album-track-header" aria-hidden="true">
      <span className="album-track-number-header">#</span>
      <span className="album-track-name-header">{label}</span>
      <span className="album-track-duration">{productMessage('media.chapter-start-label').text}</span>
      <span className="album-track-actions-header" />
    </div>
    {chapters.map((chapter, index) => <AudiobookChapterRow key={chapter.id} book={book} chapter={chapter} index={index} />)}
  </div>;
}

function hydrateChildren(items: MediaItem[], existing: MediaItem[] | undefined) {
  const details = new Map((existing ?? []).map((item) => [item.id, item]));
  return items.map((item) => {
    const detail = details.get(item.id);
    return detail ? { ...item, ...detail } : item;
  });
}

function appendChildren(current: MediaItem[], next: MediaItem[]) {
  const known = new Set(current.map((item) => item.id));
  return [...current, ...next.filter((item) => !known.has(item.id))];
}

function EpisodeList({ season, episodes, selectedEpisodeID, onSelectEpisode }: { season: MediaItem; episodes: MediaItem[] | undefined; selectedEpisodeID?: string; onSelectEpisode: (id: string) => void }) {
  if (episodes === undefined) {
    const failure = productMessage('media.detail-unavailable');
    return <div className="portico-detail-inline-state error" role="status">
      <ProductLanguageIcon presentation={failure} />
      <span><strong>{failure.title}</strong><small>{failure.body}</small></span>
    </div>;
  }
  if (!episodes.length) return <HierarchyEmpty item={season} />;
  const ordered = orderedDetailItems(episodes);
  const playbackOptions = playbackOptionsForItems(ordered, detailPlaybackContext(season));
  return <div className="show-episode-list">{ordered.map((episode) => <EpisodeRow key={episode.id} item={episode} selected={episode.id === selectedEpisodeID} onSelect={() => onSelectEpisode(episode.id)} playbackOptions={playbackOptions} />)}</div>;
}

function RemoteEpisodePanel({ season, selectedEpisodeID, onSelectEpisode }: { season: MediaItem; selectedEpisodeID?: string; onSelectEpisode: (id: string) => void }) {
  const [reloadKey, setReloadKey] = useState(0);
  const [query, setQuery] = useState<{ status: 'loading' } | { status: 'success'; data: MediaChildrenPage } | { status: 'error'; error: Error }>({ status: 'loading' });
  const [loadingMore, setLoadingMore] = useState(false);
  const [pageError, setPageError] = useState(false);
  const generation = useRef(0);
  const continuation = useRef<AbortController | undefined>(undefined);
  const source = usePorticoDataSource();
  useEffect(() => {
    generation.current += 1;
    continuation.current?.abort();
    const controller = new AbortController();
    setQuery({ status: 'loading' });
    setLoadingMore(false);
    setPageError(false);
    source.mediaChildren(season.id, controller.signal, undefined, 200).then(
      (data) => !controller.signal.aborted && setQuery({ status: 'success', data: { ...data, items: hydrateChildren(data.items, season.children) } }),
      (reason: unknown) => !controller.signal.aborted && setQuery({ status: 'error', error: reason instanceof Error ? reason : new Error(productMessage('media.detail-unavailable').body) }),
    );
    return () => {
      controller.abort();
      continuation.current?.abort();
    };
  }, [reloadKey, season.id, source]);
  useEffect(() => {
    if (!selectedEpisodeID || query.status !== 'success' || query.data.items.some((item) => item.id === selectedEpisodeID) || !query.data.hasMore || !query.data.nextCursor) return;
    continuation.current?.abort();
    const controller = new AbortController();
    continuation.current = controller;
    const activeGeneration = generation.current;
    const startingPage = query.data;
    setLoadingMore(true);
    setPageError(false);
    void (async () => {
      let page = startingPage;
      let items = startingPage.items;
      try {
        while (page.hasMore && page.nextCursor && !items.some((item) => item.id === selectedEpisodeID)) {
          page = await source.mediaChildren(season.id, controller.signal, page.nextCursor, 200);
          if (controller.signal.aborted || activeGeneration !== generation.current) return;
          items = appendChildren(items, page.items);
        }
        setQuery((current) => current.status === 'success'
          ? { status: 'success', data: { ...page, items } }
          : current);
      } catch {
        if (controller.signal.aborted || activeGeneration !== generation.current) return;
        setPageError(true);
      } finally {
        if (!controller.signal.aborted && activeGeneration === generation.current) setLoadingMore(false);
      }
    })();
    return () => controller.abort();
  }, [query, season.id, selectedEpisodeID, source]);
  const loadingMessage = productMessage('media.detail-loading');
  if (query.status === 'loading') return <div className="portico-detail-inline-state loading" aria-live="polite" aria-busy="true"><ProductLanguageIcon presentation={loadingMessage} /> {loadingMessage.title}</div>;
  if (query.status === 'error') {
    const failure = productLanguageProblem(query.error, 'media.detail-unavailable');
    return <div className="portico-detail-inline-state error" role="alert">
      <ProductLanguageIcon presentation={failure} />
      <span><strong>{failure.title}</strong><small>{failure.body}</small></span>
      <SecondaryButton onClick={() => setReloadKey((value) => value + 1)}><RefreshCw /> {failure.actions[0]?.label}</SecondaryButton>
    </div>;
  }
  const loadMore = async () => {
    if (!query.data.nextCursor || loadingMore) return;
    continuation.current?.abort();
    const controller = new AbortController();
    continuation.current = controller;
    const activeGeneration = generation.current;
    setLoadingMore(true);
    setPageError(false);
    try {
      const next = await source.mediaChildren(season.id, controller.signal, query.data.nextCursor, 200);
      if (controller.signal.aborted || activeGeneration !== generation.current) return;
      setQuery((current) => {
        if (current.status !== 'success') return current;
        return { status: 'success', data: { ...next, items: appendChildren(current.data.items, next.items) } };
      });
    } catch {
      if (controller.signal.aborted || activeGeneration !== generation.current) return;
      setPageError(true);
    } finally {
      if (activeGeneration === generation.current) setLoadingMore(false);
    }
  };
  return <><EpisodeList season={season} episodes={query.data.items} selectedEpisodeID={selectedEpisodeID} onSelectEpisode={onSelectEpisode} />
    {pageError && <div className="portico-detail-inline-state error" role="alert"><ProductLanguageIcon presentation={productMessage('problem.request-failed')} /><span><strong>{productMessage('problem.request-failed').title}</strong><small>{productMessage('problem.request-failed').body}</small></span><SecondaryButton onClick={() => void loadMore()}><RefreshCw /> {productMessage('problem.request-failed').actions[0]?.label}</SecondaryButton></div>}
    {query.data.hasMore && query.data.nextCursor && <SecondaryButton disabled={loadingMore} onClick={() => void loadMore()}>{loadingMore ? <RefreshCw className="state-spinner" /> : null} {loadingMore ? productMessage('state.loading-more').title : productMessage('action.load-more-group', { group: productMessage('media.episodes-title').text?.toLocaleLowerCase() }).text}</SecondaryButton>}</>;
}

function EpisodePanel({ season, selectedEpisodeID, onSelectEpisode }: { season: MediaItem; selectedEpisodeID?: string; onSelectEpisode: (id: string) => void }) {
  if (season.children !== undefined && !season.childrenTruncated) return <EpisodeList season={season} episodes={season.children} selectedEpisodeID={selectedEpisodeID} onSelectEpisode={onSelectEpisode} />;
  return <RemoteEpisodePanel season={season} selectedEpisodeID={selectedEpisodeID} onSelectEpisode={onSelectEpisode} />;
}

function ShowHierarchy({ item }: { item: MediaItem }) {
  const [parameters, setParameters] = useSearchParams();
  const children = item.children;
  const seasons = useMemo(() => orderedDetailItems((children ?? []).filter((child) => detailKind(child) === 'season')), [children]);
  const directEpisodes = useMemo(() => orderedDetailItems((children ?? []).filter((child) => detailKind(child) === 'episode')), [children]);
  const requestedSeason = parameters.get('season');
  const requestedEpisode = parameters.get('episode');
  const resumeOrNextEpisode = seasons.flatMap((season) => season.children ?? []).find((episode) => (episode.progress ?? 0) > 0 && !episode.watched)
    ?? seasons.flatMap((season) => season.children ?? []).find((episode) => !episode.watched);
  const episodeSeason = requestedEpisode ? seasons.find((season) => season.children?.some((episode) => episode.id === requestedEpisode)) : undefined;
  const preferredSeason = resumeOrNextEpisode ? seasons.find((season) => season.children?.some((episode) => episode.id === resumeOrNextEpisode.id)) : undefined;
  const selected = seasons.find((season) => season.id === requestedSeason) ?? episodeSeason ?? preferredSeason ?? seasons[0];

  if (children === undefined) return null;
  if (!children.length) return <section className="portico-detail-section"><SectionHeading title={productMessage('media.seasons-title').text ?? ''} detail={productMessage('media.season-count', { count: 0 }).text ?? ''} /><HierarchyEmpty item={item} /></section>;
  if (directEpisodes.length) return <section className="portico-detail-section">
    <SectionHeading title={productMessage('media.episodes-title').text ?? ''} detail={childDetail({ ...item, entityKind: 'season' }, directEpisodes.length)} />
    <EpisodeList season={item} episodes={directEpisodes} selectedEpisodeID={requestedEpisode ?? undefined} onSelectEpisode={(id) => {
      const next = new URLSearchParams(parameters);
      next.set('episode', id);
      setParameters(next, { replace: true });
    }} />
  </section>;
  if (!seasons.length) return <section className="portico-detail-section">
    <SectionHeading title={productMessage('media.included-title').text ?? ''} detail={childDetail(item, children.length)} />
    <SelectableMediaGrid items={children} playbackContext={detailPlaybackContext(item)} />
  </section>;

  const selectSeason = (id: string) => {
    const next = new URLSearchParams(parameters);
    next.set('season', id);
    next.delete('episode');
    setParameters(next, { replace: true });
  };

  const selectEpisode = (id: string) => {
    const next = new URLSearchParams(parameters);
    next.set('season', selected.id);
    next.set('episode', id);
    setParameters(next, { replace: true });
  };

  return <section className="portico-detail-section portico-show-hierarchy">
    <SectionHeading title={productMessage('media.episodes-title').text ?? ''} detail={childDetail(item, seasons.length)} controls={<SelectMenu label={productMessage('media.season-selector-label').text ?? ''} value={selected.id} options={seasons.map((season) => ({ id: season.id, label: season.title }))} onChange={selectSeason} />} />
    <div id="selected-season-episodes">{selected && <EpisodePanel season={selected} selectedEpisodeID={requestedEpisode ?? (selected.children?.some((episode) => episode.id === resumeOrNextEpisode?.id) ? resumeOrNextEpisode?.id : undefined)} onSelectEpisode={selectEpisode} />}</div>
  </section>;
}

function ChildGrid({ item, children }: { item: MediaItem; children: MediaItem[] }) {
  const kind = detailKind(item);
  const view = ['season', 'album', 'book', 'playlist', 'channel'].includes(kind) ? 'list' : 'grid';
  const shape = kind === 'artist' ? 'square'
    : ['author', 'series'].includes(kind) ? 'poster'
      : kind === 'collection' || kind === 'category' ? undefined
        : detailArtworkShape(item);
  return <SelectableMediaGrid items={orderedDetailItems(children)} view={view} shape={shape} playbackContext={detailPlaybackContext(item)} className={view === 'list' ? 'portico-detail-ordered-list' : kind === 'artist' ? 'portico-music-grid' : ''} />;
}

function ResolvedDetailHierarchy({ item }: { item: MediaItem }) {
  if (detailKind(item) === 'show') return <ShowHierarchy item={item} />;
  if (detailKind(item) === 'book' && item.chapters !== undefined) return <section className="portico-detail-section">
    <SectionHeading title={productMessage('media.chapters-title').text ?? ''} detail={childDetail(item, item.chapters.length)} />
    <AudiobookChapterList book={item} chapters={item.chapters} />
  </section>;
  if (item.children === undefined) return null;
  const children = item.children;
  return <section className="portico-detail-section">
    <SectionHeading title={detailChildTitle(item)} detail={childDetail(item, children.length)} />
    {children.length ? detailKind(item) === 'album'
      ? <AlbumTrackList album={item} tracks={children} />
      : <ChildGrid item={item} children={children} />
      : <HierarchyEmpty item={item} />}
  </section>;
}

function PaginatedDetailHierarchy({ item }: { item: MediaItem }) {
  const source = usePorticoDataSource();
  const [reloadKey, setReloadKey] = useState(0);
  const [query, setQuery] = useState<{ status: 'loading' } | { status: 'success'; data: MediaChildrenPage } | { status: 'error'; error: Error }>({ status: 'loading' });
  const [loadingMore, setLoadingMore] = useState(false);
  const [pageError, setPageError] = useState(false);
  const generation = useRef(0);
  const continuation = useRef<AbortController | undefined>(undefined);
  useEffect(() => {
    generation.current += 1;
    continuation.current?.abort();
    const controller = new AbortController();
    setQuery({ status: 'loading' });
    setPageError(false);
    source.mediaChildren(item.id, controller.signal, undefined, 200).then(
      (data) => !controller.signal.aborted && setQuery({ status: 'success', data: { ...data, items: hydrateChildren(data.items, item.children) } }),
      (reason: unknown) => !controller.signal.aborted && setQuery({ status: 'error', error: reason instanceof Error ? reason : new Error(productMessage('media.detail-unavailable').body) }),
    );
    return () => {
      controller.abort();
      continuation.current?.abort();
    };
  }, [item.id, reloadKey, source]);

  const partialItem = { ...item, childrenTruncated: false };
  const loadingMessage = productMessage('media.detail-loading');
  if (query.status === 'loading') return <>
    <ResolvedDetailHierarchy item={partialItem} />
    <div className="portico-detail-inline-state loading" aria-live="polite" aria-busy="true"><ProductLanguageIcon presentation={loadingMessage} /> {loadingMessage.title}</div>
  </>;
  if (query.status === 'error') {
    const failure = productLanguageProblem(query.error, 'media.detail-unavailable');
    return <>
      <ResolvedDetailHierarchy item={partialItem} />
      <div className="portico-detail-inline-state error" role="alert"><ProductLanguageIcon presentation={failure} /><span><strong>{failure.title}</strong><small>{failure.body}</small></span><SecondaryButton onClick={() => setReloadKey((value) => value + 1)}><RefreshCw /> {failure.actions[0]?.label}</SecondaryButton></div>
    </>;
  }
  const loadMore = async () => {
    if (!query.data.nextCursor || loadingMore) return;
    continuation.current?.abort();
    const controller = new AbortController();
    continuation.current = controller;
    const activeGeneration = generation.current;
    setLoadingMore(true);
    setPageError(false);
    try {
      const next = await source.mediaChildren(item.id, controller.signal, query.data.nextCursor, 200);
      if (controller.signal.aborted || activeGeneration !== generation.current) return;
      setQuery((current) => {
        if (current.status !== 'success') return current;
        const known = new Set(current.data.items.map((child) => child.id));
        return { status: 'success', data: { ...next, items: [...current.data.items, ...next.items.filter((child) => !known.has(child.id))] } };
      });
    } catch {
      if (controller.signal.aborted || activeGeneration !== generation.current) return;
      setPageError(true);
    } finally {
      if (activeGeneration === generation.current) setLoadingMore(false);
    }
  };
  return <>
    <ResolvedDetailHierarchy item={{ ...item, children: query.data.items, childrenTruncated: false }} />
    {pageError && <div className="portico-detail-inline-state error" role="alert"><ProductLanguageIcon presentation={productMessage('problem.request-failed')} /><span><strong>{productMessage('problem.request-failed').title}</strong><small>{productMessage('problem.request-failed').body}</small></span><SecondaryButton onClick={() => void loadMore()}><RefreshCw /> {productMessage('problem.request-failed').actions[0]?.label}</SecondaryButton></div>}
    {query.data.hasMore && query.data.nextCursor && <SecondaryButton disabled={loadingMore} onClick={() => void loadMore()}>{loadingMore ? <RefreshCw className="state-spinner" /> : null} {loadingMore ? productMessage('state.loading-more').title : productMessage('action.load-more-group', { group: detailChildTitle(item).toLocaleLowerCase() }).text}</SecondaryButton>}
  </>;
}

export function DetailHierarchy({ item }: { item: MediaItem }) {
  return item.childrenTruncated ? <PaginatedDetailHierarchy item={item} /> : <ResolvedDetailHierarchy item={item} />;
}
