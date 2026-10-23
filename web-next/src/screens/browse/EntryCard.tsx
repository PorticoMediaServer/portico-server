import type {CSSProperties} from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import {shapeFor} from '@core/presentation/card-shape.ts';
import {homeCardModel} from '../home/model';
import type {TextEnvironment} from '@portico/client-core/kit-model';
import {GeneratedArt} from '../../kit/artwork';
import {CircleCard, CollectionTile, PosterCard, SquareCard, type CardSelection} from '../../kit/cards';
import type {ShellArtwork} from '../../shell/artwork';

export type BrowseItemActions = Readonly<{
  onOpen: () => void;
  onPlay?: () => void;
  onMore?: (anchor: HTMLButtonElement) => void;
  moreExpanded?: boolean;
  onNotInterested?: () => void;
  selection?: CardSelection;
}>;
export type BrowseEntryCardProps = Readonly<{entry: ContentEntry; actions: BrowseItemActions; artwork: ShellArtwork; env: TextEnvironment;
  width: number; order: number; group: string; visible: boolean; backdrops: boolean}>;
/** Display fields never stand in for a navigation target or a permission offer. */
export function BrowseEntryCard({entry, actions, artwork, env, width, order, group, visible, backdrops}: BrowseEntryCardProps) {
  const shape = shapeFor(entry.kind, 'grid');
  const props = {model: homeCardModel(entry, false, env), context: 'grid' as const, visible, watched: entry.watched, isNew: entry.new,
    onNavigate: actions.onOpen, onPlay: actions.onPlay, onMore: actions.onMore, moreExpanded: actions.moreExpanded,
    onNotInterested: actions.onNotInterested, selection: actions.selection, style: {'--cards-width': `${width}px`} as CSSProperties,
    fallback: <GeneratedArt glyph={Array.from(entry.title.trim())[0] ?? ''}/>};
  const source = artwork.foreground(entry.posterUrl, width * devicePixelRatio, shape === 'poster' ? 2 / 3 : 1, {group, order});
  if (entry.kind === 'collection') return <CollectionTile {...props} artwork={backdrops ? artwork.foreground(entry.backdropUrl, width * devicePixelRatio, 16 / 9, {group, order}) : undefined}
    ambient={backdrops ? artwork.ambient(entry.backdropUrl, {group, order}) : undefined}
    covers={(entry.artworkPaths ?? []).slice(0, 4).flatMap((path, index) => {const cover = artwork.foreground(path, 90 * devicePixelRatio, 2 / 3, {group, order: order * 4 + index});return cover ? [cover] : [];})}/>;
  if (entry.kind === 'artist' || shape === 'circle') return <CircleCard {...props} covers={(entry.artworkPaths ?? []).slice(0, 4).flatMap(path => {const cover = artwork.foreground(path, width * devicePixelRatio, 1, {group, order});return cover ? [cover] : [];})} initials={entry.title.trim().split(/\s+/).slice(0, 2).map(word => Array.from(word)[0] ?? '').join('')} variant={entry.kind === 'artist' ? 'artist' : 'person'}/>;
  return shape === 'square' ? <SquareCard {...props} artwork={source} ambient={backdrops ? artwork.ambient(entry.posterUrl, {group, order}) : undefined} variant={entry.kind === 'book' || entry.kind === 'audiobook_file' ? 'audiobook' : 'music'}/> : <PosterCard {...props} artwork={source}/>;
}
