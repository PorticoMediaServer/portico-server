import { StatusWarningIcon, StatusSuccessIcon, PlaybackPlayIcon, ActionRefreshIcon, ActionCloseIcon } from '#portico-icons';
import { contextualMediaPlayAction, productMessage } from '@porticomediaserver/client-core';
import { useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { reviewedProductErrorText } from '../../components/ProductLanguage';
import { useMediaMutations } from '../../data/DataProvider';
import type { MediaItem } from '../../data/models';
import { usePlaybackSession } from '../player/PlayerSurface';
import { actionPresentation, MediaActionIcon, useMediaActionPresentations } from '../media/MediaActionPresentation';
import { playbackOptionsForItems, watchNavigationState } from '../player/watchNavigation';
import { DetailActionMenu, type DetailOperationNotice } from './DetailActionMenu';
import {
  detailKind,
  detailPlaybackContext,
  detailSavedLabel,
  detailWatchedLabel,
  isChannelDetail,
  isMusicDetail,
  orderedDetailItems,
  showEpisodes,
  showPlaybackTarget,
} from './detailModel';

function supports(actions: Set<string>, names: string[]) {
  return names.some((name) => actions.has(name));
}

const primaryActionNames = new Set([
  'play',
  'live.play',
  'dvr.play',
  'watchlist.add',
  'watchlist.remove',
  'favorite.add',
  'favorite.remove',
  'watched.set',
  'watched.mark',
  'watched.unmark',
  'play.from-beginning',
]);

const menuActionNames = new Set(['queue.add', 'collection.add', 'playlist.add', 'metadata.edit', 'metadata.refresh', 'media.analyze', 'media.optimize', 'download', 'media.delete', 'reaction.set', 'rating.set']);

export function DetailActions({ item, onMetadataChange }: { item: MediaItem; onMetadataChange: () => void }) {
  const navigate = useNavigate();
  const playback = usePlaybackSession();
  const mutations = useMediaMutations();
  const [saved, setSaved] = useState(item.watchlisted ?? false);
  const [favorite, setFavorite] = useState(item.favorite ?? false);
  const [watched, setWatched] = useState(item.watched ?? false);
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const [notice, setNotice] = useState<DetailOperationNotice>();
  const showTarget = detailKind(item) === 'show' ? showPlaybackTarget(item) : undefined;
  const showPlaybackActionIds = (showTarget?.actions ?? []).filter((id) => ['play', 'play.from-beginning', 'watch-with-friends.start'].includes(id));
  const projectedActionKey = [...new Set([...(item.actions ?? []), ...showPlaybackActionIds])].join('\u001f');
  const [projectedActionIds, setProjectedActionIds] = useState<string[]>(() => projectedActionKey ? projectedActionKey.split('\u001f') : []);
  const presentedActions = useMediaActionPresentations(projectedActionIds);
  const actions = useMemo(() => new Set(presentedActions.map((action) => action.id)), [presentedActions]);
  const action = (...ids: string[]) => actionPresentation(presentedActions, ...ids);

  useEffect(() => {
    setSaved(item.watchlisted ?? false);
    setFavorite(item.favorite ?? false);
    setWatched(item.watched ?? false);
    setProjectedActionIds(projectedActionKey ? projectedActionKey.split('\u001f') : []);
    setBusy('');
    setError('');
  }, [item.favorite, item.id, item.watchlisted, item.watched, projectedActionKey]);

  useEffect(() => setNotice(undefined), [item.id]);

  const canPlay = supports(actions, ['play', 'live.play', 'dvr.play']);
  const canWatchlist = supports(actions, ['watchlist.add', 'watchlist.remove']);
  const canRestart = actions.has('play.from-beginning') && canPlay && !isChannelDetail(item);
  const canFavorite = supports(actions, ['favorite.add', 'favorite.remove']);
  const canMarkWatched = supports(actions, ['watched.set', 'watched.mark', 'watched.unmark']);
  const remainingActions = presentedActions.map((candidate) => candidate.id).filter((candidate) => !primaryActionNames.has(candidate));
  const hasPlayableVersions = (item.mediaFiles ?? []).filter((version) => version.available).length > 1;
  const hasMenuActions = hasPlayableVersions || remainingActions.some((action) => menuActionNames.has(action));
  const canWatchWithFriends = canPlay && !isChannelDetail(item) && actions.has('watch-with-friends.start');
  const primaryPlayAction = action('play', 'live.play', 'dvr.play');
  const contextualPlayAction = contextualMediaPlayAction(primaryPlayAction, {
    entityKind: item.entityKind,
    progressSeconds: item.progressSeconds,
    seasonNumber: item.seasonNumber,
    episodeNumber: item.episodeNumber,
    playbackTarget: showTarget ? {
      entityKind: showTarget.entityKind,
      progressSeconds: showTarget.progressSeconds,
      seasonNumber: showTarget.seasonNumber,
      episodeNumber: showTarget.episodeNumber,
    } : undefined,
  });

  const run = async (name: string, operation: () => Promise<MediaItem>, apply: (updated: MediaItem) => void, rollback?: () => void) => {
    setBusy(name);
    setError('');
    try {
      const updated = await operation();
      apply(updated);
      setProjectedActionIds([...new Set([...(updated.actions ?? []), ...showPlaybackActionIds])]);
    } catch (reason) {
      rollback?.();
      setError(reviewedProductErrorText(reason, 'catalog.action-failed', { actionName: 'complete that action' }));
    } finally {
      setBusy('');
    }
  };

  const startPlayback = async () => {
    setBusy('play');
    setError('');
    try {
      if (isChannelDetail(item)) {
        const session = await playback.startLive(item.id);
        if (session) navigate(`/watch/${session.media.id}`);
        return;
      }
      if (detailKind(item) === 'recording') {
        const session = await playback.startDVR(item.id);
        if (session) navigate(`/watch/${session.media.id}`);
        return;
      }
      if (detailKind(item) === 'show') {
        const episodes = showEpisodes(item);
        const target = showTarget;
        if (target) {
          const options = playbackOptionsForItems(episodes.length ? episodes : [target], detailPlaybackContext(item));
          navigate(`/watch/${target.id}`, { state: watchNavigationState(options) });
          return;
        }
      }
      const children = item.children ? orderedDetailItems(item.children) : [];
      const options = children.some((child) => child.actions?.includes('play'))
        ? playbackOptionsForItems(children, detailPlaybackContext(item))
        : undefined;
      if (isMusicDetail(item)) {
        await playback.start(item.id, options);
        return;
      }
      navigate(`/watch/${item.id}`, { state: watchNavigationState(options) });
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'playback.failed'));
    } finally {
      setBusy('');
    }
  };

  const startFromBeginning = async () => {
    const target = detailKind(item) === 'show' ? showTarget : item;
    if (!target) return;
    setBusy('restart');
    setError('');
    try {
      const options = { startSeconds: 0, ...(detailKind(item) === 'show' ? playbackOptionsForItems(showEpisodes(item), detailPlaybackContext(item)) : {}) };
      if (isMusicDetail(target)) await playback.start(target.id, options);
      else navigate(`/watch/${target.id}`, { state: watchNavigationState(options) });
    } catch (reason) {
      setError(reviewedProductErrorText(reason, 'playback.failed'));
    } finally {
      setBusy('');
    }
  };

  const toggleSaved = () => {
    const next = !saved;
    setSaved(next);
    return run('saved', () => mutations.setWatchlist(item.id, next), (updated) => setSaved(updated.watchlisted ?? next), () => setSaved(!next));
  };

  const toggleFavorite = () => {
    const next = !favorite;
    setFavorite(next);
    return run('favorite', () => mutations.setFavorite(item.id, next), (updated) => setFavorite(updated.favorite ?? next), () => setFavorite(!next));
  };

  const toggleWatched = () => {
    const next = !watched;
    setWatched(next);
    return run('watched', () => mutations.setWatched(item.id, next), (updated) => setWatched(updated.watched ?? next), () => setWatched(!next));
  };

  if (!canPlay && !canWatchlist && !canFavorite && !canMarkWatched && !hasMenuActions && !canWatchWithFriends) return null;

  return <>
    <div className="action-row portico-detail-actions">
      {canPlay && <PrimaryButton disabled={Boolean(busy)} onClick={() => void startPlayback()}>
        {busy === 'play' ? <ActionRefreshIcon className="state-spinner" /> : contextualPlayAction ? <MediaActionIcon action={contextualPlayAction} /> : <PlaybackPlayIcon fill="currentColor" />}
        {contextualPlayAction?.label}
      </PrimaryButton>}
      {canRestart && action('play.from-beginning') && <SecondaryButton disabled={Boolean(busy)} onClick={() => void startFromBeginning()}>{busy === 'restart' ? <ActionRefreshIcon className="state-spinner" /> : <MediaActionIcon action={action('play.from-beginning')!} />} {action('play.from-beginning')!.label}</SecondaryButton>}
      {canWatchlist && <SecondaryButton disabled={Boolean(busy)} selected={saved} onClick={() => void toggleSaved()}>
        <MediaActionIcon action={action('watchlist.add', 'watchlist.remove')!} /> {detailSavedLabel(item, saved)}
      </SecondaryButton>}
      {canFavorite && <SecondaryButton disabled={Boolean(busy)} selected={favorite} onClick={() => void toggleFavorite()}>
        <MediaActionIcon action={action('favorite.add', 'favorite.remove')!} /> {action('favorite.add', 'favorite.remove')!.label}
      </SecondaryButton>}
      {canMarkWatched && <SecondaryButton disabled={Boolean(busy)} selected={watched} onClick={() => void toggleWatched()}>
        <MediaActionIcon action={action('watched.mark', 'watched.unmark', 'watched.set')!} /> {detailWatchedLabel(item, watched)}
      </SecondaryButton>}
      {(hasMenuActions || canWatchWithFriends) && <DetailActionMenu item={{ ...item, actions: remainingActions, watchlisted: saved, favorite, watched }} allowWatchWithFriends={canWatchWithFriends} onPlayVersion={(versionId) => navigate(`/watch/${item.id}`, { state: watchNavigationState({ versionId }) })} onMetadataChange={onMetadataChange} onNotice={setNotice} />}
    </div>
    {error && <p className="hero-action-error" role="alert">{error}</p>}
    {notice && <div className={`portico-detail-operation-notice ${notice.tone}`} role={notice.tone === 'error' ? 'alert' : 'status'} aria-live="polite">
      {notice.tone === 'pending' ? <ActionRefreshIcon className="state-spinner" /> : notice.tone === 'success' ? <StatusSuccessIcon /> : <StatusWarningIcon />}
      <span><strong>{notice.title}</strong>{notice.detail && <small>{notice.detail}</small>}{notice.job && <small className="portico-detail-job-reference">{notice.job.status === 'queued' ? productMessage('media.job-queued').text : notice.job.status}{notice.job.progress > 0 ? ` · ${notice.job.progress}%` : ''}</small>}</span>
      {notice.tone !== 'pending' && <IconButton label={productMessage('action.dismiss-status').text ?? ''} onClick={() => setNotice(undefined)}><ActionCloseIcon /></IconButton>}
    </div>}
  </>;
}
