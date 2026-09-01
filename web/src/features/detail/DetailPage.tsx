import { NavigationLibraryIcon, NavigationDisclosureIcon, MediaMusicIcon, MediaMovieIcon, LibrarySavedIcon, MediaPlaylistIcon, NavigationChannelsIcon, ActionRefreshIcon, DeviceTvIcon } from '#portico-icons';
import { productMessage, resolveMediaAvailability } from '@porticomediaserver/client-core';
import { type CSSProperties, useRef, useState } from 'react';
import { Link, Navigate, useParams } from 'react-router-dom';
import { SecondaryButton } from '../../components/controls/Buttons';
import { useStableBackdrop } from '../../components/media/StableImage';
import { ProductLanguageIcon, productLanguageProblem } from '../../components/states/ProductLanguageState';
import { useMediaDetail, useProductContract } from '../../data/DataProvider';
import type { MediaItem } from '../../data/models';
import { MediaArtwork, mediaDetailPath, resolveWebMediaDetailViewModel } from '../catalog/CatalogSurface';
import { AvailabilityNotice } from './AvailabilityNotice';
import { DetailActions } from './DetailActions';
import { DetailHierarchy } from './DetailHierarchy';
import { ExtrasSections, MediaInformation, PeopleSection, RelatedSections } from './DetailSections';
import {
  detailArtworkShape,
  detailKind,
  detailKindLabel,
  detailLibraryDestination,
  detailMetaParts,
  isAudiobookDetail,
  isMusicDetail,
} from './detailModel';
import './detail.css';

function DetailBreadcrumbs({ item }: { item: MediaItem }) {
  const library = detailLibraryDestination(item);
  const parents = [
    item.grandparentId && item.grandparentTitle ? { id: item.grandparentId, title: item.grandparentTitle } : undefined,
    item.parentId && item.parentTitle ? { id: item.parentId, title: item.parentTitle } : undefined,
  ].filter((value): value is { id: string; title: string } => Boolean(value));
  const uniqueParents = parents.filter((parent, index) => parent.id !== item.id && parents.findIndex((candidate) => candidate.id === parent.id) === index);
  return <nav className="portico-detail-breadcrumbs" aria-label={productMessage('media.hierarchy-label').text}>
    <Link to={library.path}>{library.label}</Link>
    {uniqueParents.map((parent) => <span key={parent.id}><NavigationDisclosureIcon /><Link to={`/media/${parent.id}`}>{parent.title}</Link></span>)}
    <span><NavigationDisclosureIcon /><strong aria-current="page">{item.title}</strong></span>
  </nav>;
}

function DetailIcon({ item }: { item: MediaItem }) {
  const kind = detailKind(item);
  if (kind === 'collection') return <LibrarySavedIcon />;
  if (kind === 'playlist') return <MediaPlaylistIcon />;
  if (kind === 'live-channel' || kind === 'recording') return <NavigationChannelsIcon />;
  if (isAudiobookDetail(item)) return <NavigationLibraryIcon />;
  if (isMusicDetail(item)) return kind === 'album' || kind === 'track' ? <MediaMusicIcon /> : <MediaMusicIcon />;
  if (kind === 'movie') return <MediaMovieIcon />;
  if (kind === 'category') return <NavigationLibraryIcon />;
  return <DeviceTvIcon />;
}

function DetailLoading() {
  const message = productMessage('media.detail-loading');
  return <div className="portico-detail-page portico-detail-loading" aria-live="polite" aria-busy="true">
    <span className="sr-only">{message.title}. {message.body}</span>
    <section className="portico-detail-hero">
      <div className="portico-detail-skeleton portico-detail-art-skeleton" />
      <div className="portico-detail-copy">
        <span className="portico-detail-skeleton skeleton-breadcrumb" />
        <span className="portico-detail-skeleton skeleton-kind" />
        <span className="portico-detail-skeleton skeleton-title" />
        <span className="portico-detail-skeleton skeleton-meta" />
        <span className="portico-detail-skeleton skeleton-summary" />
        <span className="portico-detail-skeleton skeleton-summary short" />
        <div className="portico-detail-skeleton-actions"><span /><span /><span /></div>
      </div>
    </section>
    <div className="portico-detail-content"><span className="portico-detail-skeleton skeleton-section" /><div className="portico-detail-skeleton-row">{Array.from({ length: 5 }, (_, index) => <span key={index} />)}</div></div>
  </div>;
}

function unavailableError(error: Error) {
  const problem = error as Error & { status?: number; code?: string };
  return problem.status === 404 || problem.code === 'media_not_found';
}

function DetailFailure({ error, onRetry }: { error: Error; onRetry: () => void }) {
  const missing = unavailableError(error);
  const presentation = missing ? productMessage('problem.not-found') : productLanguageProblem(error, 'media.detail-unavailable');
  const retryLabel = productMessage('action.retry').text ?? '';
  return <div className="standard-page portico-detail-failure">
    <div className="library-state error" role="alert">
      <ProductLanguageIcon presentation={presentation} />
      <strong>{presentation.title}</strong>
      <p>{presentation.body}</p>
      <div className="portico-detail-failure-actions">
        {!missing && <SecondaryButton onClick={onRetry}><ActionRefreshIcon /> {retryLabel}</SecondaryButton>}
        <Link className="button secondary" to="/libraries">{productMessage('action.open-libraries').text}</Link>
      </div>
    </div>
  </div>;
}

export function DetailPage() {
  const { id } = useParams();
  const [reloadKey, setReloadKey] = useState(0);
  const previousId = useRef(id);
  const previousItem = useRef<MediaItem | undefined>(undefined);
  if (previousId.current !== id) {
    previousId.current = id;
    previousItem.current = undefined;
  }
  const detail = useMediaDetail(id, reloadKey);
  const productContract = useProductContract();
  if (detail.status === 'success') previousItem.current = detail.data;
  const pendingItem = detail.status === 'success' ? detail.data : previousItem.current;
  const stableBackdrop = useStableBackdrop(pendingItem?.backdrop, true, pendingItem?.metadataEtag ?? pendingItem?.metadataRevision);
  if (detail.status === 'loading' && !previousItem.current) return <DetailLoading />;
  if (detail.status === 'error') return <DetailFailure error={detail.error} onRetry={() => setReloadKey((value) => value + 1)} />;

  const item = pendingItem;
  if (!item) return <DetailLoading />;
  const canonicalPath = mediaDetailPath(item);
  if ((detailKind(item) === 'season' || detailKind(item) === 'episode') && canonicalPath && canonicalPath !== `/media/${encodeURIComponent(item.id)}`) {
    return <Navigate replace to={canonicalPath} />;
  }
  const viewModel = productContract.status === 'success' ? resolveWebMediaDetailViewModel(productContract.data, item) : undefined;
  const availability = viewModel?.availability ?? resolveMediaAvailability({
    availability: item.availability ? {
      status: item.availability,
      fileCount: item.fileCount,
      missingFileCount: item.missingFileCount,
    } : undefined,
  });
  const contractArtwork = viewModel?.semantics.known ? viewModel.artwork : undefined;
  const fallbackShape = detailArtworkShape(item);
  const shape = contractArtwork ? (Math.abs(contractArtwork.shape.aspectRatio - 1) < 0.08 ? 'square' : undefined) : fallbackShape;
  const hasArtwork = Boolean(contractArtwork?.url || item.poster || Object.values(item.artwork ?? {}).some(Boolean));
  const metadataLine = detailMetaParts(item).join(' · ');
  const onMetadataChange = () => setReloadKey((value) => value + 1);

  return <div className={`portico-detail-page ${shape === 'square' ? 'music-detail' : ''}`} style={{ '--detail-backdrop': stableBackdrop } as CSSProperties}>
    <section className="portico-detail-hero">
      <div className={`portico-detail-art ${shape ?? ''}`} style={contractArtwork ? { aspectRatio: contractArtwork.shape.aspectRatio } : undefined}>
        {hasArtwork ? <MediaArtwork item={item} shape={contractArtwork ? undefined : shape} /> : <span className="portico-detail-art-fallback"><DetailIcon item={item} /><strong>{item.title.slice(0, 2).toLocaleUpperCase()}</strong></span>}
      </div>
      <div className="portico-detail-copy">
        <DetailBreadcrumbs item={item} />
        <p className="portico-detail-kind">{detailKindLabel(item)}</p>
        <h1>{item.title}</h1>
        {metadataLine && <p className="portico-detail-meta">{metadataLine}</p>}
        {item.tagline && <p className="portico-detail-tagline">{item.tagline}</p>}
        {item.summary && <p className="portico-detail-summary">{item.summary}</p>}
        <DetailActions item={item} onMetadataChange={onMetadataChange} />
        <AvailabilityNotice item={item} availability={availability} onMetadataChange={onMetadataChange} />
      </div>
    </section>
    <div className="portico-detail-content">
      <MediaInformation item={item} availability={availability} />
      <DetailHierarchy item={item} />
      <PeopleSection item={item} />
      <ExtrasSections item={item} />
      <RelatedSections item={item} rows={item.recommendationRows ?? []} />
    </div>
  </div>;
}
