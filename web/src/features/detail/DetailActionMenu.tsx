import { StatusSuccessIcon, ActionDownloadIcon, ActionMoreIcon, MediaMovieIcon, PlaybackQualityIcon, ActionRefreshIcon, NavigationSearchIcon, ActionCustomizeIcon, ActionCloseIcon } from '#portico-icons';
import { useCallback, useEffect, useRef, useState } from 'react';
import { productMessage } from '@porticomediaserver/client-core';
import { useNavigate } from 'react-router-dom';
import { IconButton, PrimaryButton, SecondaryButton } from '../../components/controls/Buttons';
import { AnchoredOverlay, ModalOverlay } from '../../components/overlay/OverlayPortal';
import { ProductLanguageIcon } from '../../components/product/ProductLanguageIcon';
import { useMediaMutations, useMediaOperations } from '../../data/DataProvider';
import type { MediaAnalysisMode, MediaDownloadOptions, MediaItem, MediaJob } from '../../data/models';
import { productLanguageProblem } from '../../components/states/ProductLanguageState';
import { MediaMetadataEditor, SavedTargetDialog } from '../media/MediaActionDialogs';
import { MediaDeleteDialog } from '../media/MediaDeleteDialog';
import { MediaRatingDialog } from '../media/MediaRatingDialog';
import { actionPresentation, MediaActionIcon, useMediaActionPresentations } from '../media/MediaActionPresentation';
import { useOptionalPlaybackSession } from '../player/PlayerSurface';
import { formatDetailBytes } from './detailModel';
import { FeedbackDialog } from '../feedback/FeedbackDialog';
import { SemanticProductIcon, productProblem, productProblemText, productText } from '../../components/ProductLanguage';
import './detail.css';

export type DetailOperationNotice = {
  tone: 'pending' | 'success' | 'error';
  title: string;
  detail?: string;
  job?: MediaJob;
};

function mediaVersionLabel(version: NonNullable<MediaItem['mediaFiles']>[number]) {
  return version.versionLabel || version.resolution || version.quality || productText('action.play-version');
}

function mediaVersionDetail(version: NonNullable<MediaItem['mediaFiles']>[number]) {
  return [
    version.resolution,
    version.dynamicRange,
    version.videoCodec?.toLocaleUpperCase(),
    version.audioCodec?.toLocaleUpperCase(),
    version.container?.toLocaleUpperCase(),
    version.sizeBytes ? formatDetailBytes(version.sizeBytes) : undefined,
  ].filter(Boolean).join(' · ');
}

function PlayVersionDialog({ item, onDismiss, onPlay }: {
  item: MediaItem;
  onDismiss: () => void;
  onPlay: (versionId: string) => void;
}) {
  const headingId = 'detail-play-version-title';
  const versions = (item.mediaFiles ?? []).filter((version) => version.available);
  return <ModalOverlay labelledBy={headingId} className="detail-operation-dialog detail-analysis-dialog" onDismiss={onDismiss}>
    <header><div><p>{productText('playback.version-description')}</p><h2 id={headingId}>{productText('playback.version-title')}</h2></div><IconButton label={`${productText('action.close')} ${productText('playback.version-title')}`} onClick={onDismiss}><ActionCloseIcon /></IconButton></header>
    <div className="detail-analysis-options">
      {versions.map((version) => <button type="button" key={version.id} onClick={() => onPlay(version.id)}>
        <MediaMovieIcon />
        <span><strong>{mediaVersionLabel(version)}{version.selected ? ` · ${productText('media.default-version')}` : ''}</strong><small>{mediaVersionDetail(version)}</small></span>
      </button>)}
    </div>
    <footer><SecondaryButton onClick={onDismiss}>{productText('action.cancel')}</SecondaryButton></footer>
  </ModalOverlay>;
}

type MediaOperationTarget = Pick<MediaItem, 'id' | 'title' | 'entityKind' | 'actions'>;

function errorMessage(reason: unknown) {
  const presentation = productLanguageProblem(reason);
  return presentation.body ?? presentation.title ?? presentation.text ?? productMessage('problem.request-failed').body ?? '';
}

function jobStatus(job: MediaJob) {
  const status = job.status.trim().replaceAll('_', ' ').replace(/^\w/, (letter) => letter.toLocaleUpperCase());
  return job.progress > 0 ? `${status} · ${job.progress}%` : status;
}

export function AnalyzeMediaDialog({ item, onDismiss, onQueue }: { item: Pick<MediaItem, 'id' | 'title'>; onDismiss: () => void; onQueue: (mode: MediaAnalysisMode) => void }) {
  const headingId = 'detail-analyze-media-title';
  const analyzeLabel = productMessage('action.analyze-media').text;
  return <ModalOverlay labelledBy={headingId} className="detail-operation-dialog detail-analysis-dialog" onDismiss={onDismiss}>
    <header><div><p>{analyzeLabel}</p><h2 id={headingId}>{analyzeLabel} · {item.title}</h2></div><IconButton label={productMessage('action.close-analysis-options').text ?? ''} onClick={onDismiss}><ActionCloseIcon /></IconButton></header>
    <div className="detail-analysis-options">
      <button type="button" onClick={() => onQueue('probe')}><NavigationSearchIcon /><span><strong>{productMessage('action.stream-inspection').text}</strong><small>{productMessage('media.stream-inspection-body').text}</small></span></button>
      <button type="button" onClick={() => onQueue('full')}><PlaybackQualityIcon /><span><strong>{productMessage('action.full-media-analysis').text}</strong><small>{productMessage('media.full-analysis-body').text}</small></span></button>
    </div>
    <footer><SecondaryButton onClick={onDismiss}>{productMessage('action.cancel').text}</SecondaryButton></footer>
  </ModalOverlay>;
}

type OptionsState =
  | { status: 'loading' }
  | { status: 'success'; data: MediaDownloadOptions }
  | { status: 'error'; error: Error };

export function MediaVersionsDialog({
  item,
  mode,
  onDismiss,
  onNotice,
  onChanged,
}: {
  item: MediaOperationTarget;
  mode: 'download' | 'optimize';
  onDismiss: () => void;
  onNotice: (notice: DetailOperationNotice) => void;
  onChanged: () => void;
}) {
  const operations = useMediaOperations();
  const presentedActions = useMediaActionPresentations(item.actions ?? []);
  const [state, setState] = useState<OptionsState>({ status: 'loading' });
  const [busy, setBusy] = useState('');
  const [error, setError] = useState('');
  const headingId = `detail-${mode}-title`;

  const load = useCallback(async () => {
    setState({ status: 'loading' });
    try {
      const data = await operations.downloadOptions(item.id);
      setState({ status: 'success', data });
    } catch (reason) {
      if (mode === 'download') {
        setState({ status: 'error', error: new Error(productMessage('download.options-failed').body ?? '') });
      } else {
        setState({ status: 'error', error: new Error(errorMessage(reason)) });
      }
    }
  }, [item.id, mode, operations]);

  useEffect(() => { void load(); }, [load]);

  const handToBrowser = async (profile: string) => {
    const url = await operations.createDownloadURL(item.id, profile);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = '';
    anchor.rel = 'noreferrer';
    anchor.referrerPolicy = 'no-referrer';
    anchor.hidden = true;
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
    const copy = productMessage('download.browser-started');
    onNotice({ tone: 'success', title: copy.title ?? '', detail: copy.body });
  };

  const createVersion = async (profile: string, label: string) => {
    setBusy(profile);
    setError('');
    const pending = productMessage('media.action-pending', { action: label });
    onNotice({ tone: 'pending', title: pending.title ?? '', detail: pending.body });
    try {
      const job = await operations.createOptimizedVersion(item.id, profile);
      const queued = productMessage('media.action-queued', { action: label });
      onNotice({ tone: 'success', title: queued.title ?? '', detail: queued.body, job });
      onChanged();
      await load();
    } catch (reason) {
      const message = productProblemText(reason, 'catalog.operation-queue-failed', { operationName: 'Optimized version' });
      setError(message);
      const failed = productMessage('media.action-failed', { action: label });
      onNotice({ tone: 'error', title: failed.title ?? '', detail: message || failed.body });
    } finally {
      setBusy('');
    }
  };

  const downloadVersion = async (profile: string) => {
    setBusy(profile);
    setError('');
    try {
      await handToBrowser(profile);
    } catch (reason) {
      const message = productProblemText(reason, 'download.failed');
      setError(message);
      const copy = productMessage('download.failed');
      onNotice({ tone: 'error', title: copy.title ?? '', detail: message || copy.body });
    } finally {
      setBusy('');
    }
  };

  const canOptimize = Boolean(actionPresentation(presentedActions, 'media.optimize'));
  const shownOptions = state.status === 'success'
    ? state.data.options.filter((option) => mode === 'download' ? option.available : option.kind === 'optimized')
    : [];

  return <ModalOverlay labelledBy={headingId} className="detail-operation-dialog detail-versions-dialog" onDismiss={onDismiss}>
    <header><div><p>{item.title}</p><h2 id={headingId}>{mode === 'download' ? productMessage('download.dialog-title').text : 'Optimized versions'}</h2></div><IconButton label={mode === 'download' ? productMessage('action.close-download-options').text ?? '' : `Close ${mode} options`} onClick={onDismiss}>{mode === 'download' ? <ProductLanguageIcon id="action.close" /> : <ActionCloseIcon />}</IconButton></header>
    {state.status === 'loading' && <div className="detail-operation-state" aria-live="polite" aria-busy="true">{mode === 'download' ? <ProductLanguageIcon id="status.loading" className="state-spinner" /> : <ActionRefreshIcon className="state-spinner" />}<strong>{mode === 'download' ? productMessage('download.options-loading').title : 'Loading media versions…'}</strong></div>}
    {state.status === 'error' && <div className="detail-operation-state error" role="alert"><strong>{mode === 'download' ? productMessage('download.options-failed').title : productMessage('media.load-failed', { featureName: 'Media versions' }).title}</strong><p>{mode === 'download' ? productProblemText(state.error, 'download.options-failed') : productProblemText(state.error, 'media.load-failed', { featureName: 'Media versions' })}</p><SecondaryButton onClick={() => void load()}>{mode === 'download' ? <ProductLanguageIcon id="action.retry" /> : <ActionRefreshIcon />} {productMessage('action.retry').text}</SecondaryButton></div>}
    {state.status === 'success' && <div className="detail-version-options">
      {!shownOptions.length && <div className="detail-operation-state"><strong>{mode === 'download' ? productMessage('download.options-empty').title : 'No versions are available for this item.'}</strong>{mode === 'download' && <p>{productMessage('download.options-empty').body}</p>}</div>}
      {shownOptions.map((option) => {
        const profile = option.profile || option.id;
        const profileInfo = state.data.profiles.find((candidate) => candidate.id === profile);
        const technical = [option.container?.toLocaleUpperCase(), option.videoCodec?.toLocaleUpperCase(), option.audioCodec?.toLocaleUpperCase(), option.sizeBytes ? formatDetailBytes(option.sizeBytes) : undefined].filter(Boolean).join(' · ');
        return <article key={option.id} className={option.available ? 'available' : option.job ? 'pending' : 'unavailable'}>
          <span className="detail-version-mark">{option.kind === 'source' ? mode === 'download' ? <ProductLanguageIcon id="action.download" /> : <ActionDownloadIcon /> : mode === 'download' ? <ProductLanguageIcon id={option.available ? 'status.success' : 'status.preparation'} /> : option.available ? <StatusSuccessIcon /> : <ActionCustomizeIcon />}</span>
          <div><strong>{option.label}{profile === state.data.defaultProfile && <small>{mode === 'download' ? productMessage('download.option-default').text : 'Default'}</small>}</strong><p>{option.description}</p>{technical && <span>{technical}</span>}{profileInfo && !technical && <span>{mode === 'download' ? productMessage('download.profile-video-kbps', { height: profileInfo.height, bitrate: profileInfo.videoKbps.toLocaleString() }).text : `${profileInfo.height}p · ${profileInfo.videoKbps.toLocaleString()} Kbps video`}</span>}{option.job && <span className="detail-job-state">{jobStatus(option.job)}</span>}</div>
          <div className="detail-version-command">
            {mode === 'download' && state.data.canDownload && <PrimaryButton disabled={Boolean(busy)} onClick={() => void downloadVersion(profile)}>{busy === profile ? <ProductLanguageIcon id="status.loading" className="state-spinner" /> : <ProductLanguageIcon id="action.download" />} {productMessage('action.download').text}</PrimaryButton>}
            {!option.available && !option.job && option.kind === 'optimized' && canOptimize && <SecondaryButton disabled={Boolean(busy)} onClick={() => void createVersion(profile, option.label)}>{busy === profile ? <ActionRefreshIcon className="state-spinner" /> : <ActionCustomizeIcon />} Create</SecondaryButton>}
            {option.job && <span>{jobStatus(option.job)}</span>}
            {!option.available && !option.job && option.kind === 'source' && <span>{mode === 'download' ? productMessage('download.option-unavailable').text : 'Unavailable'}</span>}
            {option.available && mode === 'optimize' && <span>Ready</span>}
          </div>
        </article>;
      })}
      {!state.data.canDownload && mode === 'download' && <p className="detail-operation-permission">{productMessage('download.forbidden').body}</p>}
    </div>}
    {error && <p className="context-action-error" role="alert">{error}</p>}
    <footer><SecondaryButton onClick={onDismiss}>{mode === 'download' ? productMessage('action.close').text : 'Close'}</SecondaryButton></footer>
  </ModalOverlay>;
}

export function DetailActionMenu({
  item,
  allowWatchWithFriends,
  onPlayVersion,
  onMetadataChange,
  onNotice,
}: {
  item: MediaItem;
  allowWatchWithFriends: boolean;
  onPlayVersion: (versionId: string) => void;
  onMetadataChange: () => void;
  onNotice: (notice: DetailOperationNotice) => void;
}) {
  const navigate = useNavigate();
  const playback = useOptionalPlaybackSession();
  const operations = useMediaOperations();
  const mediaMutations = useMediaMutations();
  const [open, setOpen] = useState(false);
  const [savedTarget, setSavedTarget] = useState<'playlist' | 'collection'>();
  const [editingMetadata, setEditingMetadata] = useState(false);
  const [analysisOpen, setAnalysisOpen] = useState(false);
  const [playVersionOpen, setPlayVersionOpen] = useState(false);
  const [versionsMode, setVersionsMode] = useState<'download' | 'optimize'>();
  const [deleting, setDeleting] = useState(false);
  const [ratingOpen, setRatingOpen] = useState(false);
  const [feedbackKind, setFeedbackKind] = useState<'media' | 'quality'>();
  const [reaction, setReaction] = useState(item.reaction ?? '');
  const [reactionPending, setReactionPending] = useState(false);
  const [userRating, setUserRating] = useState(item.userRating ?? 0);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const presentedActions = useMediaActionPresentations(item.actions ?? []);
  const actions = new Set(presentedActions.map((action) => action.id));
  const action = (...ids: string[]) => actionPresentation(presentedActions, ...ids);
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
  const canReportProblem = actions.has('feedback.report-problem');
  const canRequestHigherQuality = actions.has('feedback.request-higher-quality');
  const canPlayVersion = (item.mediaFiles ?? []).filter((version) => version.available).length > 1;
  const hasActions = canPlayVersion || canAddCollection || canAddPlaylist || canQueue || canEditMetadata || canRefresh || canAnalyze || canOptimize || canDownload || canDelete || canReact || canRate || allowWatchWithFriends || canReportProblem || canRequestHigherQuality;
  const closeMenu = () => setOpen(false);

  const updatePlaybackQueue = async (placement: 'append' | 'next') => {
    if (!playback) return;
    closeMenu();
    const label = placement === 'next' ? productMessage('action.play-next').text ?? '' : queueAction?.label ?? productMessage('action.add-queue').text ?? '';
    try {
      if (placement === 'next') await playback.playNext([item.id]);
      else await playback.appendQueue([item.id]);
      const updated = productMessage('media.queue-updated', { mediaTitle: item.title });
      onNotice({ tone: 'success', title: updated.title ?? '', detail: updated.body });
    } catch (reason) {
      const failed = productMessage('media.action-failed', { action: label });
      onNotice({ tone: 'error', title: failed.title ?? '', detail: errorMessage(reason) || failed.body });
    }
  };

  useEffect(() => {
    setReaction(item.reaction ?? '');
    setUserRating(item.userRating ?? 0);
  }, [item.id, item.reaction, item.userRating]);

  const updateReaction = async (value: 'like' | 'dislike') => {
    if (reactionPending) return;
    const previous = reaction;
    const next = reaction === value ? '' : value;
    setReactionPending(true);
    setReaction(next);
    try {
      const updated = await mediaMutations.setReaction(item.id, next);
      setReaction(updated.reaction ?? next);
      onMetadataChange();
    } catch (reason) {
      setReaction(previous);
      onNotice({ tone: 'error', title: 'Reaction was not changed', detail: productProblemText(reason, 'catalog.action-failed', { actionName: 'save your reaction' }) });
    } finally {
      setReactionPending(false);
    }
  };

  const queue = async (type: 'metadata_refresh' | 'media_analyze', options: { analysisMode?: MediaAnalysisMode } = {}) => {
    const label = type === 'metadata_refresh'
      ? action('metadata.refresh')?.label ?? productMessage('action.refresh-metadata').text ?? ''
      : productMessage(options.analysisMode === 'probe' ? 'action.stream-inspection' : 'action.full-media-analysis').text ?? '';
    const pending = productMessage('media.action-pending', { action: label.toLocaleLowerCase() });
    onNotice({ tone: 'pending', title: pending.title ?? '', detail: pending.body });
    try {
      const job = await operations.queueJob(item.id, type, options);
      const queued = productMessage('media.action-queued', { action: label });
      onNotice({ tone: 'success', title: queued.title ?? '', detail: queued.body, job });
      onMetadataChange();
    } catch (reason) {
      const failure = productProblem(reason, 'catalog.operation-queue-failed', { operationName: label });
      onNotice({ tone: 'error', title: failure.title ?? `${label} was not queued`, detail: failure.body });
    }
  };

  if (!hasActions) return null;
  return <>
    <div className="more-actions" onPointerDown={(event) => event.stopPropagation()} onClick={(event) => event.stopPropagation()}>
      <IconButton ref={triggerRef} label={productMessage('action.more-for', { title: item.title }).text ?? ''} className={open ? 'selected' : ''} onClick={() => setOpen((value) => !value)}><ActionMoreIcon /></IconButton>
      {open && <AnchoredOverlay anchorRef={triggerRef} placement="bottom-end" className="context-menu detail-context-menu" role="menu" onDismiss={closeMenu}>
        <div className="context-title">{item.poster ? <img src={item.poster} alt="" /> : <span className="context-artwork-fallback"><MediaMovieIcon /></span>}<span><strong>{item.title}</strong><small>{item.subtitle}</small></span></div>
        <div className="context-section">
          {canPlayVersion && <button type="button" onClick={() => { closeMenu(); setPlayVersionOpen(true); }}><MediaMovieIcon /> {productText('action.play-version')}</button>}
          {allowWatchWithFriends && action('watch-with-friends.start') && <button type="button" onClick={() => { closeMenu(); navigate(`/watch-with-friends?media=${encodeURIComponent(item.id)}`); }}><MediaActionIcon action={action('watch-with-friends.start')!} /> {action('watch-with-friends.start')!.label}</button>}
          {canQueue && queueAction && <button type="button" onClick={() => void updatePlaybackQueue('append')}><MediaActionIcon action={queueAction} /> {queueAction.label}</button>}
          {canQueue && queueAction && <button type="button" onClick={() => void updatePlaybackQueue('next')}><MediaActionIcon action={queueAction} /> {productMessage('action.play-next').text}</button>}
          {canAddPlaylist && action('playlist.add') && <button type="button" onClick={() => { closeMenu(); setSavedTarget('playlist'); }}><MediaActionIcon action={action('playlist.add')!} /> {action('playlist.add')!.label}</button>}
          {canAddCollection && action('collection.add') && <button type="button" onClick={() => { closeMenu(); setSavedTarget('collection'); }}><MediaActionIcon action={action('collection.add')!} /> {action('collection.add')!.label}</button>}
        </div>
        {(canReact || canRate) && <div className="context-section feedback-actions">
          {canReact && action('reaction.set') && <button type="button" className={reaction === 'like' ? 'selected' : ''} aria-pressed={reaction === 'like'} aria-busy={reactionPending} disabled={reactionPending} onClick={() => void updateReaction('like')}><MediaActionIcon action={action('reaction.set')!} /> {productMessage(reaction === 'like' ? 'action.remove-like' : 'action.like').text}</button>}
          {canReact && action('reaction.set') && <button type="button" className={reaction === 'dislike' ? 'selected' : ''} aria-pressed={reaction === 'dislike'} aria-busy={reactionPending} disabled={reactionPending} onClick={() => void updateReaction('dislike')}><MediaActionIcon action={action('reaction.set')!} /> {productMessage(reaction === 'dislike' ? 'action.remove-dislike' : 'action.dislike').text}</button>}
          {canRate && action('rating.set') && <button type="button" onClick={() => { closeMenu(); setRatingOpen(true); }}><MediaActionIcon action={action('rating.set')!} /> {action('rating.set')!.label}</button>}
        </div>}
        {(canReportProblem || canRequestHigherQuality) && <div className="context-section feedback-actions">
          {canReportProblem && <button type="button" onClick={() => { closeMenu(); setFeedbackKind('media'); }}><SemanticProductIcon id="action.report" /> {productText('action.report-problem')}</button>}
          {canRequestHigherQuality && <button type="button" onClick={() => { closeMenu(); setFeedbackKind('quality'); }}><SemanticProductIcon id="action.quality" /> {productText('action.request-higher-quality')}</button>}
        </div>}
        {(canEditMetadata || canRefresh || canAnalyze) && <div className="context-section owner-actions">
          {canEditMetadata && action('metadata.edit') && <button type="button" onClick={() => { closeMenu(); setEditingMetadata(true); }}><MediaActionIcon action={action('metadata.edit')!} /> {action('metadata.edit')!.label}</button>}
          {canRefresh && action('metadata.refresh') && <button type="button" onClick={() => { closeMenu(); void queue('metadata_refresh'); }}><MediaActionIcon action={action('metadata.refresh')!} /> {action('metadata.refresh')!.label}</button>}
          {canAnalyze && action('media.analyze') && <button type="button" onClick={() => { closeMenu(); setAnalysisOpen(true); }}><MediaActionIcon action={action('media.analyze')!} /> {action('media.analyze')!.label}</button>}
        </div>}
        {(canOptimize || canDownload) && <div className="context-section owner-actions">
          {canOptimize && action('media.optimize') && <button type="button" onClick={() => { closeMenu(); setVersionsMode('optimize'); }}><MediaActionIcon action={action('media.optimize')!} /> {action('media.optimize')!.label}</button>}
          {canDownload && action('download') && <button type="button" onClick={() => { closeMenu(); setVersionsMode('download'); }}><MediaActionIcon action={action('download')!} /> {action('download')!.label}</button>}
        </div>}
        {canDelete && action('media.delete') && <div className="context-section owner-actions destructive"><button type="button" onClick={() => { closeMenu(); setDeleting(true); }}><MediaActionIcon action={action('media.delete')!} /> {action('media.delete')!.label}</button></div>}
      </AnchoredOverlay>}
    </div>
    {savedTarget && <SavedTargetDialog kind={savedTarget} mediaIds={[item.id]} onDismiss={() => setSavedTarget(undefined)} />}
    {editingMetadata && <MediaMetadataEditor mediaIds={[item.id]} onDismiss={() => setEditingMetadata(false)} onSaved={onMetadataChange} />}
    {analysisOpen && <AnalyzeMediaDialog item={item} onDismiss={() => setAnalysisOpen(false)} onQueue={(mode) => { setAnalysisOpen(false); void queue('media_analyze', { analysisMode: mode }); }} />}
    {playVersionOpen && <PlayVersionDialog item={item} onDismiss={() => setPlayVersionOpen(false)} onPlay={(versionId) => { setPlayVersionOpen(false); onPlayVersion(versionId); }} />}
    {versionsMode && <MediaVersionsDialog item={item} mode={versionsMode} onDismiss={() => setVersionsMode(undefined)} onNotice={onNotice} onChanged={onMetadataChange} />}
    {deleting && <MediaDeleteDialog
      items={[item]}
      onDismiss={() => setDeleting(false)}
      onDelete={(id, input) => mediaMutations.deleteMedia(id, input)}
      onComplete={(result) => {
        if (result.failedIds.length) {
          const failed = productMessage('media.removal-incomplete', { removed: 0, failed: result.failedIds.length });
          onNotice({ tone: 'error', title: failed.title ?? '', detail: failed.body });
          return;
        }
        const removed = productMessage('media.removed');
        onNotice({ tone: 'success', title: removed.title ?? '', detail: removed.body });
        navigate(item.libraryId ? `/library/${item.libraryId}` : '/', { replace: true });
      }}
    />}
    {ratingOpen && <MediaRatingDialog title={item.title} value={userRating} onDismiss={() => setRatingOpen(false)} onSave={async (rating) => {
      const updated = await mediaMutations.setRating(item.id, rating);
      setUserRating(updated.userRating ?? rating);
      onMetadataChange();
    }} />}
    {feedbackKind && <FeedbackDialog kind={feedbackKind} mediaId={item.id} title={productText(feedbackKind === 'quality' ? 'feedback.heading.quality-media' : 'feedback.heading.report-media', { mediaTitle: item.title })} onDismiss={() => setFeedbackKind(undefined)} />}
  </>;
}
