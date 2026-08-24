import {
  AlertTriangle,
  Check,
  ChevronLeft,
  ChevronRight,
  CircleCheck,
  Ellipsis,
  Film,
  Music2,
  RefreshCw,
  X,
} from '#portico-icons';
import { productMessage, resolveMediaDetailViewModel, resolveMediaViewModel, type ProductContract } from '@porticomediaserver/client-core';
import { type ReactNode, useEffect, useRef, useState, useSyncExternalStore } from 'react';
import { createPortal } from 'react-dom';
import { Link, useNavigate } from 'react-router-dom';
import { IconButton } from '../../components/controls/Buttons';
import { AnchoredOverlay } from '../../components/overlay/OverlayPortal';
import { productLanguageProblem } from '../../components/states/ProductLanguageState';
import { useMediaMutations, useMediaOperations, useProductContract } from '../../data/DataProvider';
import { artworkFailureCacheVersion, artworkFailureExpiresAt, rememberArtworkFailure, subscribeArtworkFailureCache } from '../../data/artworkFailureCache';
import type { MediaItem, PlaybackStartOptions } from '../../data/models';
import { optionalMotionBehavior } from '../../runtime/motion';
import {
  AnalyzeMediaDialog,
  MediaVersionsDialog,
  type DetailOperationNotice,
} from '../detail/DetailActionMenu';
import {
  type PlaybackCollectionContext,
  playbackOptionsForItems,
  watchNavigationState,
} from '../player/watchNavigation';
import { useOptionalPlaybackSession } from '../player/PlayerSurface';
import { MediaMetadataEditor, SavedTargetDialog } from '../media/MediaActionDialogs';
import { actionPresentation, MediaActionIcon, useMediaActionPresentations } from '../media/MediaActionPresentation';
import { MediaDeleteDialog } from '../media/MediaDeleteDialog';
import { MediaRatingDialog } from '../media/MediaRatingDialog';
import { savedAriaLabel } from '../media/actionLabels';
import { targetFromMedia, type ContextTarget } from '../media/contextTarget';
import { mediaPresentation } from './mediaPresentation';
import './catalog.css';

function sharedFailureDetail(reason: unknown) {
  const presentation = productLanguageProblem(reason);
  return presentation.body ?? presentation.title ?? productMessage('problem.request-failed').body ?? '';
}

export type ArtworkShape = 'poster' | 'square' | 'landscape';

function CatalogOperationNotice({ notice, onDismiss }: { notice: DetailOperationNotice; onDismiss: () => void }) {
  const root = document.getElementById('portico-overlays') ?? document.body;
  return createPortal(<div className={`catalog-operation-notice ${notice.tone}`} role={notice.tone === 'error' ? 'alert' : 'status'} aria-live="polite">
    {notice.tone === 'pending' ? <RefreshCw className="state-spinner" /> : notice.tone === 'success' ? <CircleCheck /> : <AlertTriangle />}
    <span><strong>{notice.title}</strong>{notice.detail && <small>{notice.detail}</small>}</span>
    {notice.tone !== 'pending' && <IconButton label={productMessage('action.dismiss-status').text ?? ''} onClick={onDismiss}><X /></IconButton>}
  </div>, root);
}

export function mediaDetailPath(item: MediaItem) {
  const kind = String(item.entityKind || item.kind).replaceAll('_', '-');
  if (kind === 'live-channel') {
    const parameters = new URLSearchParams({ tab: 'channels', channel: item.id, q: item.title });
    if (item.libraryId) parameters.set('source', item.libraryId);
    return `/live?${parameters}`;
  }
  if (kind === 'collection' || kind === 'playlist') return `/saved/${kind}s/${encodeURIComponent(item.id)}`;
  if (kind === 'category') return undefined;
  if (kind === 'person') return `/person/${encodeURIComponent(item.id)}`;
  if (kind === 'season' && item.parentId) {
    return `/media/${encodeURIComponent(item.parentId)}?season=${encodeURIComponent(item.id)}`;
  }
  if (kind === 'episode' && item.grandparentId) {
    const parameters = new URLSearchParams({ season: item.parentId ?? '', episode: item.id });
    if (!item.parentId) parameters.delete('season');
    return `/media/${encodeURIComponent(item.grandparentId)}?${parameters}`;
  }
  return `/media/${encodeURIComponent(item.id)}`;
}

export function resolveWebMediaViewModel(contract: ProductContract, item: MediaItem) {
  return resolveMediaViewModel(contract, {
    id: item.id,
    libraryId: item.libraryId,
    entityKind: item.entityKind || item.kind,
    type: item.type,
    title: item.title,
    subtitle: item.subtitle,
    parentTitle: item.parentTitle,
    summary: item.summary,
    year: item.year,
    durationSeconds: item.durationSeconds,
    artwork: item.artwork,
    images: { poster: item.poster, backdrop: item.backdrop },
    actions: item.actions,
    fields: item.typedMetadata,
    state: {
      watched: item.watched,
      watchlisted: item.watchlisted,
      favorite: item.favorite,
      progressSeconds: item.progressSeconds,
      rating: item.userRating,
      reaction: item.reaction,
    },
    availability: item.availability ? {
      status: item.availability,
      fileCount: item.fileCount,
      missingFileCount: item.missingFileCount,
    } : undefined,
    missing: item.missing,
    fileCount: item.fileCount,
    missingFileCount: item.missingFileCount,
  });
}

/** Adapts the Web transport model once, then delegates detail semantics to Client Core. */
export function resolveWebMediaDetailViewModel(contract: ProductContract, item: MediaItem) {
  const source = {
    id: item.id,
    libraryId: item.libraryId,
    type: item.entityKind || item.kind,
    title: item.title,
    images: { poster: item.poster, backdrop: item.backdrop, thumb: '' },
    artwork: item.artwork ?? {},
    parentTitle: item.parentTitle,
    summary: item.summary,
    year: item.year || undefined,
    durationSeconds: item.durationSeconds,
    actions: (item.actions ?? []) as Parameters<typeof resolveMediaDetailViewModel>[1]['actions'],
    typedMetadata: item.typedMetadata,
    missing: item.missing,
    fileCount: item.fileCount,
    missingFileCount: item.missingFileCount,
    state: {
      watched: item.watched ?? false,
      watchlisted: item.watchlisted ?? false,
      favorite: item.favorite ?? false,
      progressSeconds: item.progressSeconds ?? 0,
      rating: item.userRating ?? 0,
      reaction: item.reaction || undefined,
    },
  };
  return resolveMediaDetailViewModel(contract, source);
}

function useWebMediaViewModel(item: MediaItem) {
  const contract = useProductContract();
  return contract.status === 'success' ? resolveWebMediaViewModel(contract.data, item) : undefined;
}

function artworkShapeFor(item: MediaItem, preferred?: ArtworkShape, aspectRatio?: number, contractKnown = false): ArtworkShape {
  if (preferred) return preferred;
  if (contractKnown && aspectRatio) return Math.abs(aspectRatio - 1) < 0.08 ? 'square' : 'poster';
  return mediaPresentation(item).artworkShape;
}

function displayArtworkVariant(source: string, width: number, height: number) {
  if (!source.includes('/api/')) return source;
  const url = new URL(source, 'http://portico.invalid');
  url.searchParams.set('width', String(width));
  url.searchParams.set('height', String(height));
  return source.startsWith('http://') || source.startsWith('https://') ? url.toString() : `${url.pathname}${url.search}${url.hash}`;
}

export function MediaArtwork({ item, shape, className = '' }: { item: MediaItem; shape?: ArtworkShape; className?: string }) {
  useSyncExternalStore(subscribeArtworkFailureCache, artworkFailureCacheVersion, artworkFailureCacheVersion);
  const viewModel = useWebMediaViewModel(item);
  const resolvedShape = artworkShapeFor(item, shape, viewModel?.artwork.shape.aspectRatio, viewModel?.semantics.known);
  const source = viewModel?.artwork.url || item.poster;
  const failed = artworkFailureExpiresAt(source) > 0;
  if (!source || failed) {
    const Icon = resolvedShape === 'square' ? Music2 : Film;
    return <span className={`catalog-artwork-fallback ${resolvedShape} ${className}`} data-artwork-role={viewModel?.artwork.role} role="img" aria-label={productMessage('media.artwork-unavailable', { title: item.title }).text}><Icon /></span>;
  }
  const width = 320;
  const height = resolvedShape === 'square' ? 320 : resolvedShape === 'landscape' ? 180 : 480;
  return <img
    className={className}
    src={displayArtworkVariant(source, width, height)}
    srcSet={`${displayArtworkVariant(source, Math.round(width / 2), Math.round(height / 2))} 160w, ${displayArtworkVariant(source, width, height)} 320w, ${displayArtworkVariant(source, Math.round(width * 1.5), Math.round(height * 1.5))} 480w`}
    sizes="(max-width: 720px) 36vw, 200px"
    width={width}
    height={height}
    alt=""
    data-artwork-role={viewModel?.artwork.role}
    loading="lazy"
    decoding="async"
    style={{ objectFit: viewModel?.artwork.shape.fit }}
    onError={() => {
      rememberArtworkFailure(source);
    }}
  />;
}

export function MediaActionMenu({
  target,
  card = false,
  playbackOptions,
  onFavoriteChange,
  onMetadataChange,
  onDeleted,
}: {
  target: ContextTarget;
  card?: boolean;
  playbackOptions?: PlaybackStartOptions;
  onFavoriteChange?: (id: string, favorite: boolean) => void;
  onMetadataChange?: () => void;
  onDeleted?: (ids: string[]) => void;
}) {
  const playback = useOptionalPlaybackSession();
  const mediaActions = useMediaMutations();
  const mediaOperations = useMediaOperations();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const [watchlisted, setWatchlisted] = useState(target.watchlisted ?? false);
  const [favorite, setFavorite] = useState(target.favorite ?? false);
  const [watched, setWatched] = useState(target.watched ?? false);
  const [reaction, setReaction] = useState(target.reaction ?? '');
  const [reactionPending, setReactionPending] = useState(false);
  const [userRating, setUserRating] = useState(target.userRating ?? 0);
  const [actionError, setActionError] = useState('');
  const [savedTarget, setSavedTarget] = useState<'playlist' | 'collection'>();
  const [editingMetadata, setEditingMetadata] = useState(false);
  const [analysisOpen, setAnalysisOpen] = useState(false);
  const [versionsMode, setVersionsMode] = useState<'download' | 'optimize'>();
  const [deleting, setDeleting] = useState(false);
  const [ratingOpen, setRatingOpen] = useState(false);
  const [operationNotice, setOperationNotice] = useState<DetailOperationNotice>();
  const [projectedActionIds, setProjectedActionIds] = useState<string[]>(target.actions ?? []);
  const targetActionKey = (target.actions ?? []).join('\u001f');
  const triggerRef = useRef<HTMLButtonElement>(null);
  const presentedActions = useMediaActionPresentations(projectedActionIds);
  const actions = new Set<string>(presentedActions.map((action) => action.id));
  const action = (...ids: string[]) => actionPresentation(presentedActions, ...ids);
  const canPlay = actions.has('play');
  const canRestart = canPlay && actions.has('play.from-beginning');
  const canWatchlist = actions.has('watchlist.add') || actions.has('watchlist.remove');
  const canFavorite = actions.has('favorite.add') || actions.has('favorite.remove');
  const canMarkWatched = watched
    ? actions.has('watched.unmark') || actions.has('watched.set')
    : actions.has('watched.mark') || actions.has('watched.set');
  const canAddCollection = actions.has('collection.add');
  const canAddPlaylist = actions.has('playlist.add');
  const queueAction = action('queue.add');
  const canQueue = Boolean(queueAction && playback?.queue?.canMutate && !playback.queueBusy && !playback.queueNeedsRefresh);
  const canEditMetadata = actions.has('metadata.edit');
  const canRefresh = actions.has('metadata.refresh');
  const canAnalyze = actions.has('media.analyze');
  const canOptimize = actions.has('media.optimize');
  const canDownload = actions.has('download');
  const canDelete = actions.has('media.delete');
  const canReact = actions.has('reaction.set');
  const canRate = actions.has('rating.set');
  const canWatchWithFriends = canPlay && actions.has('watch-with-friends.start');
  const closeMenu = () => setOpen(false);
  useEffect(() => {
    setProjectedActionIds(target.actions ?? []);
  }, [target.id, targetActionKey]);
  const updateWatchlist = async () => {
    const next = !watchlisted;
    setActionError('');
    setWatchlisted(next);
    try {
      const updated = await mediaActions.setWatchlist(target.id, next);
      setWatchlisted(updated.watchlisted ?? next);
      setProjectedActionIds(updated.actions ?? []);
    } catch (reason) {
      setWatchlisted(!next);
      setActionError(sharedFailureDetail(reason));
    }
  };
  const updateFavorite = async () => {
    const next = !favorite;
    setActionError('');
    setFavorite(next);
    try {
      const updated = await mediaActions.setFavorite(target.id, next);
      setFavorite(updated.favorite ?? next);
      setProjectedActionIds(updated.actions ?? []);
      onFavoriteChange?.(target.id, next);
    } catch (reason) {
      setFavorite(!next);
      setActionError(sharedFailureDetail(reason));
    }
  };
  const updateWatched = async () => {
    const next = !watched;
    setActionError('');
    setWatched(next);
    try {
      const updated = await mediaActions.setWatched(target.id, next);
      setWatched(updated.watched ?? next);
      setProjectedActionIds(updated.actions ?? []);
    } catch (reason) {
      setWatched(!next);
      setActionError(sharedFailureDetail(reason));
    }
  };
  const updateReaction = async (value: 'like' | 'dislike') => {
    if (reactionPending) return;
    const previous = reaction;
    const next = reaction === value ? '' : value;
    setActionError('');
    setReactionPending(true);
    setReaction(next);
    try {
      const updated = await mediaActions.setReaction(target.id, next);
      setReaction(updated.reaction ?? next);
      setProjectedActionIds(updated.actions ?? []);
    } catch (reason) {
      setReaction(previous);
      setActionError(sharedFailureDetail(reason));
    } finally {
      setReactionPending(false);
    }
  };

  const queueOperation = async (type: 'metadata_refresh' | 'media_analyze', mode?: 'probe' | 'full') => {
    const label = type === 'metadata_refresh'
      ? action('metadata.refresh')?.label ?? productMessage('action.refresh-metadata').text ?? ''
      : productMessage(mode === 'probe' ? 'action.stream-inspection' : 'action.full-media-analysis').text ?? '';
    const pending = productMessage('media.action-pending', { action: label.toLocaleLowerCase() });
    closeMenu();
    setOperationNotice({ tone: 'pending', title: pending.title ?? '', detail: pending.body });
    try {
      const job = await mediaOperations.queueJob(target.id, type, mode ? { analysisMode: mode } : {});
      const queued = productMessage('media.action-queued', { action: label });
      setOperationNotice({ tone: 'success', title: queued.title ?? '', detail: queued.body, job });
      if (type === 'metadata_refresh') onMetadataChange?.();
    } catch (reason) {
      const failed = productMessage('media.action-failed', { action: label });
      setOperationNotice({ tone: 'error', title: failed.title ?? '', detail: sharedFailureDetail(reason) || failed.body });
    }
  };

  const updatePlaybackQueue = async (placement: 'append' | 'next') => {
    if (!playback) return;
    closeMenu();
    const label = placement === 'next' ? productMessage('action.play-next').text ?? '' : queueAction?.label ?? productMessage('action.add-queue').text ?? '';
    try {
      if (placement === 'next') await playback.playNext([target.id]);
      else await playback.appendQueue([target.id]);
      const updated = productMessage('media.queue-updated', { mediaTitle: target.title });
      setOperationNotice({ tone: 'success', title: updated.title ?? '', detail: updated.body });
    } catch (reason) {
      const failed = productMessage('media.action-failed', { action: label });
      setOperationNotice({ tone: 'error', title: failed.title ?? '', detail: sharedFailureDetail(reason) || failed.body });
    }
  };

  const playTarget = (startSeconds?: number) => {
    closeMenu();
    const options = startSeconds == null ? playbackOptions : { ...(playbackOptions ?? {}), startSeconds };
    if (playback && (target.type === 'music' || ['track', 'album'].includes(target.kind))) {
      void playback.start(target.id, options);
      return;
    }
    navigate(`/watch/${target.id}`, { state: watchNavigationState(options) });
  };

  if (!canPlay && !canWatchlist && !canFavorite && !canMarkWatched && !canAddCollection && !canAddPlaylist && !canQueue && !canEditMetadata && !canRefresh && !canAnalyze && !canOptimize && !canDownload && !canDelete && !canReact && !canRate && !canWatchWithFriends) return null;
  return <>
    <div className={`more-actions ${card ? 'card-more-actions' : ''}`} onPointerDown={(event) => event.stopPropagation()} onClick={(event) => event.stopPropagation()}>
      <IconButton ref={triggerRef} label={productMessage('action.more-for', { title: target.title }).text ?? ''} className={open ? 'selected' : ''} onClick={(event) => { event.preventDefault(); event.stopPropagation(); setOpen((value) => !value); }}><Ellipsis /></IconButton>
      {open && <AnchoredOverlay anchorRef={triggerRef} placement={card ? 'right-start' : 'bottom-end'} className={`context-menu ${card ? 'card-context-menu' : ''}`} role="menu" onDismiss={closeMenu}>
        <div className="context-title">{target.poster ? <img src={target.poster} alt="" /> : <span className="context-artwork-fallback"><Film /></span>}<span><strong>{target.title}</strong><small>{target.subtitle}</small></span></div>
        <div className="context-section">
          {action('play') && <button type="button" onClick={() => playTarget()}><MediaActionIcon action={action('play')!} /> {action('play')!.label}</button>}
          {action('play.from-beginning') && canRestart && <button type="button" onClick={() => playTarget(0)}><MediaActionIcon action={action('play.from-beginning')!} /> {action('play.from-beginning')!.label}</button>}
          {action('watch-with-friends.start') && canWatchWithFriends && <button type="button" onClick={() => { closeMenu(); navigate(`/watch-with-friends?media=${encodeURIComponent(target.id)}`); }}><MediaActionIcon action={action('watch-with-friends.start')!} /> {action('watch-with-friends.start')!.label}</button>}
          {canQueue && queueAction && <button type="button" onClick={() => void updatePlaybackQueue('append')}><MediaActionIcon action={queueAction} /> {queueAction.label}</button>}
          {canQueue && queueAction && <button type="button" onClick={() => void updatePlaybackQueue('next')}><MediaActionIcon action={queueAction} /> {productMessage('action.play-next').text}</button>}
          {canWatchlist && action('watchlist.add', 'watchlist.remove') && <button type="button" onClick={() => void updateWatchlist()}><MediaActionIcon action={action('watchlist.add', 'watchlist.remove')!} /> {action('watchlist.add', 'watchlist.remove')!.label}</button>}
          {canFavorite && action('favorite.add', 'favorite.remove') && <button type="button" onClick={() => void updateFavorite()}><MediaActionIcon action={action('favorite.add', 'favorite.remove')!} /> {action('favorite.add', 'favorite.remove')!.label}</button>}
          {canAddPlaylist && action('playlist.add') && <button type="button" onClick={() => { closeMenu(); setSavedTarget('playlist'); }}><MediaActionIcon action={action('playlist.add')!} /> {action('playlist.add')!.label}</button>}
          {canAddCollection && action('collection.add') && <button type="button" onClick={() => { closeMenu(); setSavedTarget('collection'); }}><MediaActionIcon action={action('collection.add')!} /> {action('collection.add')!.label}</button>}
          {canMarkWatched && action('watched.mark', 'watched.unmark', 'watched.set') && <button type="button" onClick={() => void updateWatched()}><MediaActionIcon action={action('watched.mark', 'watched.unmark', 'watched.set')!} /> {action('watched.mark', 'watched.unmark', 'watched.set')!.label}</button>}
        </div>
        {(canReact || canRate) && <div className="context-section feedback-actions">
          {canReact && action('reaction.set') && <button type="button" className={reaction === 'like' ? 'selected' : ''} aria-pressed={reaction === 'like'} aria-busy={reactionPending} disabled={reactionPending} onClick={() => void updateReaction('like')}><MediaActionIcon action={action('reaction.set')!} /> {productMessage(reaction === 'like' ? 'action.remove-like' : 'action.like').text}</button>}
          {canReact && action('reaction.set') && <button type="button" className={reaction === 'dislike' ? 'selected' : ''} aria-pressed={reaction === 'dislike'} aria-busy={reactionPending} disabled={reactionPending} onClick={() => void updateReaction('dislike')}><MediaActionIcon action={action('reaction.set')!} /> {productMessage(reaction === 'dislike' ? 'action.remove-dislike' : 'action.dislike').text}</button>}
          {canRate && action('rating.set') && <button type="button" onClick={() => { closeMenu(); setRatingOpen(true); }}><MediaActionIcon action={action('rating.set')!} /> {action('rating.set')!.label}</button>}
        </div>}
        {(canEditMetadata || canRefresh || canAnalyze) && <div className="context-section owner-actions">
          {canEditMetadata && action('metadata.edit') && <button type="button" onClick={() => { closeMenu(); setEditingMetadata(true); }}><MediaActionIcon action={action('metadata.edit')!} /> {action('metadata.edit')!.label}</button>}
          {canRefresh && action('metadata.refresh') && <button type="button" onClick={() => void queueOperation('metadata_refresh')}><MediaActionIcon action={action('metadata.refresh')!} /> {action('metadata.refresh')!.label}</button>}
          {canAnalyze && action('media.analyze') && <button type="button" onClick={() => { closeMenu(); setAnalysisOpen(true); }}><MediaActionIcon action={action('media.analyze')!} /> {action('media.analyze')!.label}</button>}
        </div>}
        {(canOptimize || canDownload) && <div className="context-section owner-actions">
          {canOptimize && action('media.optimize') && <button type="button" onClick={() => { closeMenu(); setVersionsMode('optimize'); }}><MediaActionIcon action={action('media.optimize')!} /> {action('media.optimize')!.label}</button>}
          {canDownload && action('download') && <button type="button" onClick={() => { closeMenu(); setVersionsMode('download'); }}><MediaActionIcon action={action('download')!} /> {action('download')!.label}</button>}
        </div>}
        {canDelete && action('media.delete') && <div className="context-section owner-actions destructive"><button type="button" onClick={() => { closeMenu(); setDeleting(true); }}><MediaActionIcon action={action('media.delete')!} /> {action('media.delete')!.label}</button></div>}
        {actionError && <p className="context-action-error" role="alert">{actionError}</p>}
      </AnchoredOverlay>}
    </div>
    {savedTarget && <SavedTargetDialog kind={savedTarget} mediaIds={[target.id]} onDismiss={() => setSavedTarget(undefined)} />}
    {editingMetadata && <MediaMetadataEditor mediaIds={[target.id]} onDismiss={() => setEditingMetadata(false)} onSaved={onMetadataChange} />}
    {analysisOpen && <AnalyzeMediaDialog item={target} onDismiss={() => setAnalysisOpen(false)} onQueue={(mode) => { setAnalysisOpen(false); void queueOperation('media_analyze', mode); }} />}
    {versionsMode && <MediaVersionsDialog item={target} mode={versionsMode} onDismiss={() => setVersionsMode(undefined)} onNotice={setOperationNotice} onChanged={() => onMetadataChange?.()} />}
    {deleting && <MediaDeleteDialog
      items={[target]}
      onDismiss={() => setDeleting(false)}
      onDelete={(id, input) => mediaActions.deleteMedia(id, input)}
      onComplete={(result) => {
        if (result.deletedIds.length) onDeleted?.(result.deletedIds);
        const removal = result.failedIds.length
          ? productMessage('media.removal-incomplete', { removed: result.deletedIds.length, failed: result.failedIds.length })
          : productMessage('media.removed');
        setOperationNotice({ tone: result.failedIds.length ? 'error' : 'success', title: removal.title ?? '', detail: removal.body });
      }}
    />}
    {ratingOpen && <MediaRatingDialog title={target.title} value={userRating} onDismiss={() => setRatingOpen(false)} onSave={async (rating) => {
      const updated = await mediaActions.setRating(target.id, rating);
      setUserRating(updated.userRating ?? rating);
    }} />}
    {operationNotice && <CatalogOperationNotice notice={operationNotice} onDismiss={() => setOperationNotice(undefined)} />}
  </>;
}

function BulkActionBar({ items, onClear, onRetain, onDeleted }: { items: MediaItem[]; onClear: () => void; onRetain: (ids: string[]) => void; onDeleted: (ids: string[]) => void }) {
  const mediaActions = useMediaMutations();
  const mediaOperations = useMediaOperations();
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState(false);
  const [savedTarget, setSavedTarget] = useState<'playlist' | 'collection'>();
  const [editingMetadata, setEditingMetadata] = useState(false);
  const [analysisOpen, setAnalysisOpen] = useState(false);
  const [versionsMode, setVersionsMode] = useState<'download' | 'optimize'>();
  const [deleting, setDeleting] = useState(false);
  const presentedActions = useMediaActionPresentations([...new Set(items.flatMap((item) => item.actions ?? []))]);
  const projected = new Set<string>(presentedActions.map((action) => action.id));
  const action = (...ids: string[]) => actionPresentation(presentedActions, ...ids);
  const supports = (names: string[]) => items.every((item) => names.some((name) => projected.has(name) && item.actions?.includes(name)));
  const canWatchlist = supports(['watchlist.add']);
  const canFavorite = supports(['favorite.add']);
  const canMarkWatched = supports(['watched.mark', 'watched.set']);
  const canMarkUnwatched = supports(['watched.unmark', 'watched.set']);
  const canAddPlaylist = supports(['playlist.add']);
  const canAddCollection = supports(['collection.add']);
  const canEditMetadata = supports(['metadata.edit']) && new Set(items.map((item) => item.kind)).size === 1;
  const canRefresh = supports(['metadata.refresh']);
  const canAnalyze = supports(['media.analyze']);
  const canOptimize = supports(['media.optimize']);
  const canDownload = items.length === 1 && supports(['download']);
  const canDelete = supports(['media.delete']);
  const run = async (actionLabel: string, mutation: (item: MediaItem) => Promise<unknown>) => {
    setBusy(true);
    setNotice('');
    const results = await Promise.allSettled(items.map(mutation));
    const failed = results.flatMap((result, index) => result.status === 'rejected' ? [items[index]] : []);
    if (failed.length) {
      onRetain(failed.map((item) => item.id));
      setNotice(productMessage('media.selection-partial', { updated: items.length - failed.length, failed: failed.length }).text ?? '');
    } else {
      setNotice(productMessage('media.selection-updated', { count: items.length, action: actionLabel }).text ?? '');
    }
    setBusy(false);
  };
  return <>
    <div className="bulk-bar" role="region" aria-label={productMessage('library.selection-label', { count: items.length }).text}>
      <div className="bulk-summary-row">
        <div className="bulk-count"><strong>{items.length}</strong><span>{productMessage('library.selected-label').text}</span><button type="button" onClick={onClear}>{productMessage('action.clear-selection').text}</button></div>
        {notice && <span className="bulk-notice" aria-live="polite">{notice}</span>}
        <IconButton label={productMessage('action.cancel-selection').text ?? ''} onClick={onClear}><X /></IconButton>
      </div>
      {(canWatchlist || canFavorite || canMarkWatched || canMarkUnwatched || canAddPlaylist || canAddCollection || canEditMetadata || canRefresh || canAnalyze || canOptimize || canDownload || canDelete) && <div className="bulk-actions-stack"><div className="bulk-action-row primary-actions">
        {canAddCollection && action('collection.add') && <button type="button" disabled={busy} onClick={() => setSavedTarget('collection')}><MediaActionIcon action={action('collection.add')!} /><span>{action('collection.add')!.label}</span></button>}
        {canAddPlaylist && action('playlist.add') && <button type="button" disabled={busy} onClick={() => setSavedTarget('playlist')}><MediaActionIcon action={action('playlist.add')!} /><span>{action('playlist.add')!.label}</span></button>}
        {canWatchlist && action('watchlist.add') && <button type="button" disabled={busy} onClick={() => void run(action('watchlist.add')!.label, (item) => mediaActions.setWatchlist(item.id, true))}><MediaActionIcon action={action('watchlist.add')!} /><span>{action('watchlist.add')!.label}</span></button>}
        {canFavorite && action('favorite.add') && <button type="button" disabled={busy} onClick={() => void run(action('favorite.add')!.label, (item) => mediaActions.setFavorite(item.id, true))}><MediaActionIcon action={action('favorite.add')!} /><span>{action('favorite.add')!.label}</span></button>}
        {canMarkWatched && action('watched.mark', 'watched.set') && <button type="button" disabled={busy} onClick={() => void run(action('watched.mark', 'watched.set')!.label, (item) => mediaActions.setWatched(item.id, true))}><MediaActionIcon action={action('watched.mark', 'watched.set')!} /><span>{action('watched.mark', 'watched.set')!.label}</span></button>}
        {canMarkUnwatched && action('watched.unmark', 'watched.set') && <button type="button" disabled={busy} onClick={() => void run(action('watched.unmark', 'watched.set')!.label, (item) => mediaActions.setWatched(item.id, false))}><MediaActionIcon action={action('watched.unmark', 'watched.set')!} /><span>{action('watched.unmark', 'watched.set')!.label}</span></button>}
        {canEditMetadata && action('metadata.edit') && <button type="button" disabled={busy} onClick={() => setEditingMetadata(true)}><MediaActionIcon action={action('metadata.edit')!} /><span>{action('metadata.edit')!.label}</span></button>}
      </div>
      {(canRefresh || canAnalyze || canOptimize || canDownload) && <div className="bulk-action-row server-actions">
        {canRefresh && action('metadata.refresh') && <button type="button" disabled={busy} onClick={() => void run(action('metadata.refresh')!.label, (item) => mediaOperations.queueJob(item.id, 'metadata_refresh', {}))}><MediaActionIcon action={action('metadata.refresh')!} /><span>{action('metadata.refresh')!.label}</span></button>}
        {canAnalyze && action('media.analyze') && <button type="button" disabled={busy} onClick={() => items.length === 1 ? setAnalysisOpen(true) : void run(action('media.analyze')!.label, (item) => mediaOperations.queueJob(item.id, 'media_analyze', { analysisMode: 'full' }))}><MediaActionIcon action={action('media.analyze')!} /><span>{action('media.analyze')!.label}</span></button>}
        {canOptimize && action('media.optimize') && <button type="button" disabled={busy} onClick={() => items.length === 1 ? setVersionsMode('optimize') : void run(action('media.optimize')!.label, (item) => mediaOperations.createOptimizedVersion(item.id, 'default'))}><MediaActionIcon action={action('media.optimize')!} /><span>{action('media.optimize')!.label}</span></button>}
        {canDownload && action('download') && <button type="button" disabled={busy} onClick={() => setVersionsMode('download')}><MediaActionIcon action={action('download')!} /><span>{action('download')!.label}</span></button>}
        {canDelete && action('media.delete') && <button type="button" className="bulk-delete" disabled={busy} onClick={() => setDeleting(true)}><MediaActionIcon action={action('media.delete')!} /><span>{action('media.delete')!.label}</span></button>}
      </div>}
      </div>}
    </div>
    {savedTarget && <SavedTargetDialog kind={savedTarget} mediaIds={items.map((item) => item.id)} onDismiss={() => setSavedTarget(undefined)} />}
    {editingMetadata && <MediaMetadataEditor mediaIds={items.map((item) => item.id)} initialItems={items} onDismiss={() => setEditingMetadata(false)} onSaved={() => setNotice(productMessage('media.selection-updated', { count: items.length, action: action('metadata.edit')?.label ?? productMessage('action.edit-metadata').text }).text ?? '')} />}
    {analysisOpen && items[0] && <AnalyzeMediaDialog item={items[0]} onDismiss={() => setAnalysisOpen(false)} onQueue={(mode) => { setAnalysisOpen(false); void run(action('media.analyze')?.label ?? productMessage('action.analyze-media').text ?? '', (item) => mediaOperations.queueJob(item.id, 'media_analyze', { analysisMode: mode })); }} />}
    {versionsMode && items[0] && <MediaVersionsDialog item={items[0]} mode={versionsMode} onDismiss={() => setVersionsMode(undefined)} onNotice={(value) => setNotice(value.detail ? `${value.title}: ${value.detail}` : value.title)} onChanged={() => undefined} />}
    {deleting && <MediaDeleteDialog
      items={items}
      onDismiss={() => setDeleting(false)}
      onDelete={(id, input) => mediaActions.deleteMedia(id, input)}
      onComplete={(result) => {
        onDeleted(result.deletedIds);
        onRetain(result.failedIds);
        const removal = result.failedIds.length
          ? productMessage('media.removal-incomplete', { removed: result.deletedIds.length, failed: result.failedIds.length })
          : productMessage('media.removed');
        setNotice(removal.body ?? removal.title ?? '');
      }}
    />}
  </>;
}

export function MediaCard({
  item,
  shape,
  playbackOptions,
  selected = false,
  onSelect,
  onWatchlistChange,
  onFavoriteChange,
  onDeleted,
}: {
  item: MediaItem;
  shape?: ArtworkShape;
  playbackOptions?: PlaybackStartOptions;
  selected?: boolean;
  onSelect?: () => void;
  onWatchlistChange?: (id: string, watchlisted: boolean) => void;
  onFavoriteChange?: (id: string, favorite: boolean) => void;
  onDeleted?: (ids: string[]) => void;
}) {
  const playback = useOptionalPlaybackSession();
  const mediaActions = useMediaMutations();
  const [saved, setSaved] = useState(item.watchlisted ?? false);
  const [saveError, setSaveError] = useState('');
  const itemActionKey = (item.actions ?? []).join('\u001f');
  const [projectedActionIds, setProjectedActionIds] = useState<string[]>(item.actions ?? []);
  const saveGeneration = useRef(0);
  const viewModel = useWebMediaViewModel(item);
  const resolvedShape = artworkShapeFor(item, shape, viewModel?.artwork.shape.aspectRatio, viewModel?.semantics.known);
  const artworkAspectRatio = shape === 'square' ? 1 : shape === 'poster' ? 2 / 3 : shape === 'landscape' ? 16 / 9 : viewModel?.artwork.shape.aspectRatio;
  const target = targetFromMedia({ ...item, actions: projectedActionIds, watchlisted: saved });
  const presentedActions = useMediaActionPresentations(projectedActionIds);
  const playAction = actionPresentation(presentedActions, 'play');
  const watchlistAction = actionPresentation(presentedActions, 'watchlist.add', 'watchlist.remove');
  const detailPath = mediaDetailPath(item);
  const canPlay = Boolean(playAction);
  const canWatchlist = Boolean(watchlistAction);
  useEffect(() => {
    setSaved(item.watchlisted ?? false);
    setProjectedActionIds(item.actions ?? []);
  }, [item.id, item.watchlisted, itemActionKey]);
  const toggleSaved = async () => {
    const generation = ++saveGeneration.current;
    const next = !saved;
    setSaved(next);
    setSaveError('');
    try {
      const updated = await mediaActions.setWatchlist(item.id, next);
      if (generation !== saveGeneration.current) return;
      const reconciled = updated.watchlisted ?? next;
      setSaved(reconciled);
      setProjectedActionIds(updated.actions ?? []);
      onWatchlistChange?.(item.id, reconciled);
    } catch (reason) {
      if (generation !== saveGeneration.current) return;
      setSaved(!next);
      setSaveError(sharedFailureDetail(reason));
    }
  };
  return <article className={`media-card-shell catalog-card ${resolvedShape} ${selected ? 'selected' : ''}`}>
    <div className="artwork-stage" style={artworkAspectRatio ? { aspectRatio: artworkAspectRatio } : undefined}>
      {detailPath ? <Link to={detailPath} className="artwork-wrap" aria-label={productMessage('action.open-item', { title: item.title }).text}><MediaArtwork item={item} shape={resolvedShape} />{item.progress != null && <span className="progress"><span style={{ width: `${item.progress}%` }} /></span>}</Link> : <div className="artwork-wrap"><MediaArtwork item={item} shape={resolvedShape} /></div>}
      {onSelect && <button type="button" className={`selection-check ${selected ? 'selected' : ''}`} onClick={(event) => { event.preventDefault(); event.stopPropagation(); onSelect(); }} aria-label={productMessage(selected ? 'library.deselect-item' : 'library.select-item', { title: item.title }).text} aria-pressed={selected}>{selected && <Check />}</button>}
      {(canPlay || canWatchlist || target.actions?.length) && <div className="card-command-strip" aria-label={productMessage('media.actions-label', { title: item.title }).text}>
        {canPlay && (playback && (item.type === 'music' || ['track', 'album'].includes(item.kind))
          ? <button type="button" className="card-command" onClick={(event) => { event.stopPropagation(); void playback.start(item.id, playbackOptions); }} aria-label={`${playAction?.label ?? productMessage('action.play').text ?? ''} ${item.title}`}><MediaActionIcon action={playAction!} /></button>
          : <Link className="card-command" to={`/watch/${item.id}`} state={watchNavigationState(playbackOptions)} onClick={(event) => event.stopPropagation()} aria-label={`${playAction?.label ?? productMessage('action.play').text ?? ''} ${item.title}`}><MediaActionIcon action={playAction!} /></Link>)}
        {canWatchlist && <button type="button" className={`card-command ${saved ? 'selected' : ''}`} onClick={(event) => { event.preventDefault(); event.stopPropagation(); void toggleSaved(); }} aria-label={`${watchlistAction?.label ?? savedAriaLabel(item, saved)} ${item.title}`} aria-pressed={saved} title={saveError || undefined}><MediaActionIcon action={watchlistAction!} /></button>}
        <MediaActionMenu target={target} card playbackOptions={playbackOptions} onFavoriteChange={onFavoriteChange} onDeleted={onDeleted} />
      </div>}
    </div>
    {detailPath ? <Link to={detailPath} className="media-card-copy"><span className="media-title">{item.title}</span><span className="media-subtitle">{item.subtitle || [item.year || undefined, item.genre || undefined].filter(Boolean).join(' · ')}</span></Link> : <div className="media-card-copy"><span className="media-title">{item.title}</span><span className="media-subtitle">{item.subtitle}</span></div>}
  </article>;
}

export function SectionHeading({ title, detail, controls }: { title: string; detail?: string; controls?: ReactNode }) {
  return <div className="section-heading"><div><h2>{title}</h2>{detail && <p>{detail}</p>}</div>{controls && <div className="section-controls">{controls}</div>}</div>;
}

export function MediaRail({ title, items, detail, shape, playbackContext, hasMore = false, continuationKey, loadingMore = false, onEndReached }: { title: string; items: MediaItem[]; detail?: string; shape?: ArtworkShape; playbackContext?: PlaybackCollectionContext; hasMore?: boolean; continuationKey?: string; loadingMore?: boolean; onEndReached?: () => void }) {
  const rail = useRef<HTMLDivElement>(null);
  const requestedContinuation = useRef<string | undefined>(undefined);
  const [selected, setSelected] = useState<string[]>([]);
  const [removed, setRemoved] = useState<string[]>([]);
  const [scrollState, setScrollState] = useState({ left: false, right: false });
  const visibleItems = items.filter((item) => !removed.includes(item.id));
  const requestContinuation = () => {
    const requestKey = continuationKey ?? `items:${visibleItems.length}`;
    if (!hasMore || loadingMore || !onEndReached || requestedContinuation.current === requestKey) return;
    requestedContinuation.current = requestKey;
    onEndReached();
  };
  const syncScrollState = () => {
    const element = rail.current;
    if (!element) return;
    const remaining = element.scrollWidth - element.clientWidth - element.scrollLeft;
    setScrollState({ left: element.scrollLeft > 2, right: remaining > 2 || hasMore });
    if (remaining < Math.max(220, element.clientWidth * 0.45)) requestContinuation();
  };
  const move = (direction: number) => {
    const element = rail.current;
    if (!element) return;
    element.scrollBy({ left: direction * Math.max(320, element.clientWidth * 0.7), behavior: optionalMotionBehavior() });
  };
  const toggle = (id: string) => setSelected((ids) => ids.includes(id) ? ids.filter((value) => value !== id) : [...ids, id]);
  const remove = (ids: string[]) => setRemoved((current) => [...new Set([...current, ...ids])]);
  const selectedItems = visibleItems.filter((item) => selected.includes(item.id));
  useEffect(() => setSelected((ids) => ids.filter((id) => items.some((item) => item.id === id) && !removed.includes(id))), [items, removed]);
  const playbackOptions = playbackContext ? playbackOptionsForItems(visibleItems, playbackContext) : undefined;
  useEffect(() => {
    requestedContinuation.current = undefined;
    const frame = window.requestAnimationFrame(syncScrollState);
    const element = rail.current;
    const observer = element && typeof ResizeObserver !== 'undefined' ? new ResizeObserver(syncScrollState) : undefined;
    if (element) observer?.observe(element);
    return () => {
      window.cancelAnimationFrame(frame);
      observer?.disconnect();
    };
  }, [continuationKey, hasMore, items.length]);
  return <section className="media-section">
    <SectionHeading title={title} controls={<><IconButton label={productMessage('action.scroll-left').text ?? ''} disabled={!scrollState.left} onClick={() => move(-1)}><ChevronLeft /></IconButton><IconButton label={productMessage('action.scroll-right').text ?? ''} disabled={!scrollState.right} onClick={() => move(1)}><ChevronRight /></IconButton></>} />
    <div className={`media-rail ${shape ?? ''}`} ref={rail} role="region" aria-label={title} aria-description={detail} tabIndex={0} onScroll={syncScrollState} onKeyDown={(event) => { if (event.key === 'ArrowLeft' || event.key === 'ArrowRight') { event.preventDefault(); move(event.key === 'ArrowLeft' ? -1 : 1); } }}>{visibleItems.map((item) => <MediaCard key={item.id} item={item} shape={shape} playbackOptions={playbackOptions} selected={selected.includes(item.id)} onSelect={item.actions?.length ? () => toggle(item.id) : undefined} onDeleted={remove} />)}{loadingMore && <span className="media-rail-continuation" role="status"><RefreshCw className="state-spinner" /><span className="sr-only">{productMessage('state.loading-more').title}</span></span>}</div>
    {selectedItems.length > 0 && <BulkActionBar items={selectedItems} onClear={() => setSelected([])} onRetain={setSelected} onDeleted={remove} />}
  </section>;
}

export function SelectableMediaGrid({ items, shape, className = '', view = 'grid', playbackContext, onWatchlistChange, onFavoriteChange, onDeleted }: { items: MediaItem[]; shape?: ArtworkShape; className?: string; view?: 'grid' | 'list'; playbackContext?: PlaybackCollectionContext; onWatchlistChange?: (id: string, watchlisted: boolean) => void; onFavoriteChange?: (id: string, favorite: boolean) => void; onDeleted?: (ids: string[]) => void }) {
  const [selected, setSelected] = useState<string[]>([]);
  const [removed, setRemoved] = useState<string[]>([]);
  const visibleItems = items.filter((item) => !removed.includes(item.id));
  const toggle = (id: string) => setSelected((ids) => ids.includes(id) ? ids.filter((value) => value !== id) : [...ids, id]);
  const remove = (ids: string[]) => {
    setRemoved((current) => [...new Set([...current, ...ids])]);
    onDeleted?.(ids);
  };
  const selectedItems = visibleItems.filter((item) => selected.includes(item.id));
  const playbackOptions = playbackContext ? playbackOptionsForItems(visibleItems, playbackContext) : undefined;
  useEffect(() => setSelected((ids) => ids.filter((id) => items.some((item) => item.id === id) && !removed.includes(id))), [items, removed]);
  return <>
    <div className={`${view === 'grid' ? 'poster-grid' : 'media-list'} ${className}`}>
      {visibleItems.map((item) => view === 'grid'
        ? <MediaCard key={item.id} item={item} shape={shape} playbackOptions={playbackOptions} selected={selected.includes(item.id)} onSelect={item.actions?.length ? () => toggle(item.id) : undefined} onWatchlistChange={onWatchlistChange} onFavoriteChange={onFavoriteChange} onDeleted={remove} />
        : <MediaListRow key={item.id} item={item} playbackOptions={playbackOptions} selected={selected.includes(item.id)} onSelect={item.actions?.length ? () => toggle(item.id) : undefined} onDeleted={remove} />)}
    </div>
    {selectedItems.length > 0 && <BulkActionBar items={selectedItems} onClear={() => setSelected([])} onRetain={setSelected} onDeleted={remove} />}
  </>;
}

export function MediaListRow({ item, playbackOptions, selected = false, onSelect, onDeleted }: { item: MediaItem; playbackOptions?: PlaybackStartOptions; selected?: boolean; onSelect?: () => void; onDeleted?: (ids: string[]) => void }) {
  const detailPath = mediaDetailPath(item);
  const target = targetFromMedia(item);
  return <article className={`catalog-list-row ${selected ? 'selected' : ''}`}>
    {onSelect && <button type="button" className={`selection-check row-check ${selected ? 'selected' : ''}`} onClick={onSelect} aria-label={productMessage(selected ? 'library.deselect-item' : 'library.select-item', { title: item.title }).text} aria-pressed={selected}>{selected && <Check />}</button>}
    {detailPath ? <Link className="catalog-list-row-link" to={detailPath}><span className="catalog-list-art"><MediaArtwork item={item} shape={mediaPresentation(item).artworkShape} /></span><span className="list-primary"><strong>{item.title}</strong><small>{item.subtitle || [item.genre, item.year || undefined].filter(Boolean).join(' · ')}</small></span><span>{item.rating}</span><span>{item.length}</span><ChevronRight className="list-detail" aria-hidden="true" /></Link> : <div className="catalog-list-row-link"><span className="catalog-list-art"><MediaArtwork item={item} shape={mediaPresentation(item).artworkShape} /></span><span className="list-primary"><strong>{item.title}</strong><small>{item.subtitle}</small></span></div>}
    <MediaActionMenu target={target} playbackOptions={playbackOptions} onDeleted={onDeleted} />
  </article>;
}
