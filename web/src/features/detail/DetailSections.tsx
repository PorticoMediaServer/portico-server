import { NavigationDisclosureIcon, MetadataInfoIcon } from '#portico-icons';
import { productMessage, type MediaViewModel } from '@porticomediaserver/client-core';
import { useMemo } from 'react';
import { Link } from 'react-router-dom';
import type { HomeRow, MediaItem } from '../../data/models';
import { StableImage } from '../../components/media/StableImage';
import { MediaRail, SectionHeading } from '../catalog/CatalogSurface';
import {
  displayMetadataLabel,
  formatDetailBytes,
  formatDetailDate,
  friendlyStreamLabel,
  isAudiobookDetail,
  isChannelDetail,
  isMusicDetail,
  isPlayableDetail,
} from './detailModel';

function personInitials(name: string) {
  return name.split(/\s+/).filter(Boolean).slice(0, 2).map((part) => part[0]?.toLocaleUpperCase()).join('') || '?';
}

function PersonPortrait({ name, imageUrl, retryKey }: { name: string; imageUrl?: string; retryKey?: string | number }) {
  const fallback = <span className="portico-person-fallback" aria-hidden="true">{personInitials(name)}</span>;
  return <StableImage src={imageUrl} alt="" width="192" height="192" loading="lazy" decoding="async" fallback={fallback} retryKey={retryKey} />;
}

function personDetailPath(id: string | undefined, name: string) {
  return id ? `/person/${encodeURIComponent(id)}` : `/search?q=${encodeURIComponent(name)}`;
}

function railShape(items: MediaItem[]): 'square' | 'poster' | undefined {
  if (!items.length) return undefined;
  return items.every(isMusicDetail) ? 'square' : undefined;
}

export function PeopleSection({ item }: { item: MediaItem }) {
  const people = useMemo(() => [...(item.people ?? [])].sort((left, right) => (left.sortOrder ?? 999) - (right.sortOrder ?? 999)), [item.people]);
  if (!people.length) return null;
  const title = productMessage(isMusicDetail(item) ? 'media.people-credits' : isAudiobookDetail(item) ? 'media.people-narrators' : 'media.people-cast-crew').text ?? '';
  return <section className="portico-detail-section portico-people-section">
    <SectionHeading title={title} detail={productMessage(people.length === 1 ? 'media.credit-count-single' : 'media.credit-count', { count: people.length }).text} />
    <div className="portico-people-list" aria-label={title}>
      {people.map((person, index) => <Link to={personDetailPath(person.id, person.name)} aria-label={productMessage('action.open-item', { title: person.name }).text} key={`${person.id || person.name}:${person.role}:${index}`}>
        <PersonPortrait name={person.name} imageUrl={person.imageUrl} retryKey={item.metadataEtag ?? item.metadataRevision} />
        <strong>{person.name}</strong>
        <span>{person.character || person.role}</span>
      </Link>)}
    </div>
  </section>;
}

export function musicRecommendationItems(items: MediaItem[]) {
  const order: string[] = [];
  const grouped = new Map<string, MediaItem>();
  items.forEach((candidate) => {
    const kind = String(candidate.entityKind).replaceAll('_', '-').toLocaleLowerCase();
    const isTrack = kind === 'track';
    const albumIdentity = isTrack && (candidate.parentId || candidate.parentTitle)
      ? candidate.parentId || `${candidate.parentTitle}\u0000${candidate.grandparentTitle || candidate.subtitle}`.toLocaleLowerCase()
      : candidate.id;
    const key = `${isTrack ? 'album' : kind}:${albumIdentity}`;
    const item = isTrack && candidate.parentTitle ? {
      ...candidate,
      id: candidate.parentId || key,
      title: candidate.parentTitle,
      subtitle: candidate.grandparentTitle || candidate.typedMetadata?.trackArtist || candidate.typedMetadata?.artist || '',
      entityKind: 'album',
      parentId: candidate.grandparentId,
      parentTitle: candidate.grandparentTitle,
      grandparentId: undefined,
      grandparentTitle: undefined,
      progress: undefined,
      progressSeconds: undefined,
      watched: undefined,
      actions: [],
      children: undefined,
    } : candidate;
    if (!grouped.has(key)) order.push(key);
    if (!grouped.has(key) || !isTrack) grouped.set(key, item);
  });
  return order.map((key) => grouped.get(key)).filter((item): item is MediaItem => Boolean(item));
}

export function musicRecommendationRows(rows: HomeRow[], item: MediaItem) {
  const seen = new Set([item.id, item.parentId].filter((id): id is string => Boolean(id)));
  return rows.map((row) => ({
    ...row,
    items: musicRecommendationItems(row.items).filter((candidate) => {
      if (seen.has(candidate.id)) return false;
      seen.add(candidate.id);
      return true;
    }),
  }));
}

export function ExtrasSections({ item }: { item: MediaItem }) {
  return <>{(item.extras ?? []).map((relationship, index) => relationship.items.length > 0 && <MediaRail
    key={`${relationship.type}:${relationship.label}:${index}`}
    title={relationship.label}
    detail={productMessage(relationship.items.length === 1 ? 'media.item-count-single' : 'media.item-count', { count: relationship.items.length }).text}
    items={relationship.items}
    shape={railShape(relationship.items)}
    playbackContext={{ type: 'queue', id: `${item.id}:extras:${relationship.type}`, title: relationship.label }}
  />)}</>;
}

export function RelatedSections({ rows, item }: { rows: HomeRow[]; item: MediaItem }) {
  const discoveryRows = isMusicDetail(item) ? musicRecommendationRows(rows, item) : rows;
  return <>{discoveryRows.map((row) => {
      const items = row.items;
      return items.length > 0 && <MediaRail
        key={row.id}
        title={row.title}
        detail={row.explanation || row.detail}
        items={items}
        shape={railShape(items)}
        playbackContext={{ type: 'queue', id: `${item.id}:related:${row.id}`, title: row.title }}
      />;
    })}</>;
}

export function DiscoverySections({ rows, item }: { rows: HomeRow[]; item: MediaItem }) {
  return <><ExtrasSections item={item} /><RelatedSections rows={rows} item={item} /></>;
}

const identityMetadata = new Set([
  'albumartist',
  'artist',
  'author',
  'chapter',
  'chapternumber',
  'disc',
  'discnumber',
  'medianumber',
  'narrator',
  'position',
  'series',
  'seriesposition',
  'track',
  'trackartist',
  'trackindex',
  'tracknumber',
]);

function technicalMetadata(item: MediaItem) {
  return Object.entries(item.typedMetadata ?? {}).filter(([key, value]) => {
    const normalized = key.replaceAll('_', '').replaceAll('-', '').toLocaleLowerCase();
    return Boolean(String(value ?? '').trim()) && !identityMetadata.has(normalized);
  });
}

function availabilityLabel(availability: NonNullable<MediaViewModel['availability']>) {
  return productMessage(availability.status === 'available' ? 'media.available' : availability.status === 'partial' ? 'media.partially-available' : 'media.unavailable').text;
}

export function MediaInformation({ item, availability }: { item: MediaItem; availability?: MediaViewModel['availability'] }) {
  const metadata = technicalMetadata(item);
  const showTechnical = isPlayableDetail(item) && !isChannelDetail(item);
  if (!showTechnical) return null;
  const hasInformation = Boolean(
    item.streams?.length
    || item.optimizedVersions?.length
    || metadata.length
    || availability?.fileCount != null
    || availability?.missingFileCount != null
    || item.edition
    || availability,
  );
  if (!hasInformation) return null;
  const streams = [...(item.streams ?? [])].sort((left, right) => ['video', 'audio', 'subtitle'].indexOf(left.kind) - ['video', 'audio', 'subtitle'].indexOf(right.kind));
  const streamGroups = [
    { kind: 'video', title: productMessage('media.stream-video').text },
    { kind: 'audio', title: productMessage('media.stream-audio').text },
    { kind: 'subtitle', title: productMessage('media.stream-subtitles').text },
  ].map((group) => ({ ...group, streams: streams.filter((stream) => stream.kind === group.kind) })).filter((group) => group.streams.length > 0);
  const versions = [...(item.optimizedVersions ?? [])].sort((left, right) => right.updatedAt.localeCompare(left.updatedAt));
  const files = availability?.fileCount != null ? productMessage(availability.fileCount === 1 ? 'media.file-count-single' : 'media.file-count', { count: availability.fileCount }).text : undefined;
  const missing = availability?.missingFileCount ? productMessage('media.missing-count', { count: availability.missingFileCount }).text : undefined;

  return <details className="portico-technical-details">
    <summary><MetadataInfoIcon /> {productMessage('media.information-title').text} <NavigationDisclosureIcon /></summary>
    <div className="portico-technical-content">
      {(item.edition || availability || files || metadata.length > 0) && <dl className="portico-fact-grid">
        {item.edition && <div><dt>{productMessage('media.edition-label').text}</dt><dd>{item.edition}</dd></div>}
        {availability && <div><dt>{productMessage('media.availability-label').text}</dt><dd>{availabilityLabel(availability)}</dd></div>}
        {files && <div><dt>{productMessage('media.source-files-label').text}</dt><dd>{[files, missing].filter(Boolean).join(' · ')}</dd></div>}
        {metadata.map(([label, value]) => <div key={label}><dt>{displayMetadataLabel(label)}</dt><dd>{value}</dd></div>)}
      </dl>}
      {streamGroups.map((group) => <div className="portico-stream-list" key={group.kind}>
        <h3>{group.title}</h3>
        {group.streams.map((stream) => <div key={stream.id}>
          <span>{stream.language || displayMetadataLabel(stream.codec || stream.kind)}</span>
          <strong>{stream.displayTitle}</strong>
          <small>{friendlyStreamLabel(stream)}</small>
        </div>)}
      </div>)}
      {versions.length > 0 && <div className="portico-version-list">
        <h3>{productMessage('media.optimized-versions-title').text}</h3>
        {versions.map((version) => <div key={version.id}>
          <span><strong>{version.profileName || version.profile}</strong><small>{version.profile}</small></span>
          <span>{formatDetailBytes(version.sizeBytes)}<small>{formatDetailDate(version.updatedAt)}</small></span>
        </div>)}
      </div>}
    </div>
  </details>;
}
