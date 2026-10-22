import {useCallback, useState, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {Icon} from '../Icon';
import type {FactPart} from '@portico/client-core/kit-model';
import {Artwork, AmbientArt, CircleArt, PersonCircle, GeneratedCover, CoverMosaic, type ArtworkSource, type ArtworkStatus} from '../artwork';
import {CardBadges, CardCaption, CardFrame, Facts, Progress, cx} from './CardFrame';
import type {MediaCardProps} from './types';
import styles from './cards.module.css';

export function CardArtwork({artwork, fallback, position, visible, arrivalDurationMs}: Pick<MediaCardProps, 'artwork' | 'fallback' | 'position' | 'visible' | 'arrivalDurationMs'>) {
  const [read, setRead] = useState<{key?: string; status: ArtworkStatus}>({status: artwork ? 'loading' : 'empty'});
  const key = artwork?.key;
  const onStatusChange = useCallback((status: ArtworkStatus) => setRead(previous => previous.key === key && previous.status === status ? previous : {key, status}), [key]);
  const loading = !!artwork && (read.key !== key || read.status === 'loading');
  return <Artwork source={artwork} visible={visible} arrivalDurationMs={arrivalDurationMs} alt="" position={position} onStatusChange={onStatusChange} fallback={loading ? <span className={styles.loadingArt} /> : fallback} />;
}

export function PosterCard({titleLines, ...props}: MediaCardProps & {titleLines?: 1 | 2}) {
  titleLines ??= props.context === 'grid' || props.density === 'grid' ? 2 : 1;
  return <CardFrame {...props} kind="poster"><span className={styles.art}><CardArtwork {...props} /><CardBadges watched={props.watched} isNew={props.isNew} progress={props.model.progress} /></span><CardCaption model={props.model} titleLines={titleLines} /></CardFrame>;
}

export function WideCard(props: MediaCardProps) {
  return <CardFrame {...props} kind="wide"><span className={styles.art}><CardArtwork {...props} /><span className={styles.wideCopy}><span className={styles.wideTitle}>{props.model.title}</span><Facts parts={props.model.caption} className={styles.wideSubtitle} /><Progress value={props.model.progress} /></span></span><Facts parts={props.model.detail} className={styles.wideDetail} /></CardFrame>;
}

export function SquareCard({variant = 'music', ...props}: MediaCardProps & {variant?: 'music' | 'audiobook'}) {
  return <CardFrame {...props} kind="square"><CardAmbient {...props} /><span className={styles.art}><CardArtwork {...props} /><CardBadges watched={props.watched} completionLabel={variant === 'audiobook' ? defaultI18n.t('next.cards.finished') : undefined} isNew={props.isNew} progress={props.model.progress} /></span><CardCaption model={props.model} /></CardFrame>;
}

export type CircleCardProps = Omit<MediaCardProps, 'artwork' | 'fallback' | 'ambient' | 'palette' | 'position' | 'arrivalDurationMs'> & {covers: readonly ArtworkSource[]; initials?: string; variant?: 'artist' | 'person'};
export function CircleCard({covers, initials, variant = initials ? 'person' : 'artist', ...props}: CircleCardProps) {
  const initialsSize = props.context === 'search' ? 34 : props.context === 'detail' ? 30 : undefined;
  return <CardFrame {...props} kind="circle"><span className={styles.art}>{variant === 'person' ? <PersonCircle covers={covers} initials={initials ?? ''} alt="" initialsSize={initialsSize} visible={props.visible} /> : <CircleArt covers={covers} initials={initials} alt="" treatment={props.context === 'search' ? 'search' : 'artist'} initialsSize={initialsSize} visible={props.visible} />}</span><CardCaption model={props.model} /></CardFrame>;
}

export type EpisodeTileProps = MediaCardProps & Readonly<{
  artworkKind: 'still' | 'backdrop';
  length?: string;
  status?: readonly FactPart[];
  airDate?: FactPart;
  cropOrigin?: string;
  description?: string;
  expanded?: boolean;
}>;
export function EpisodeTile({artworkKind, length, status, airDate, cropOrigin, description, expanded, ...props}: EpisodeTileProps) {
  return <CardFrame {...props} className={cx(expanded && styles.expanded, props.className)} kind="episode"><span className={styles.art}><span className={cx(styles.episodePicture, artworkKind === 'backdrop' && styles.backdropCrop)} style={{transformOrigin: cropOrigin}}><CardArtwork {...props} /></span>{length && <span className={styles.length}>{length}</span>}<CardBadges watched={props.watched} progress={props.model.progress} /></span>
    <span className={styles.episodeCopy}><span className={styles.episodeTitle}>{props.model.title}</span><span className={styles.episodeMeta}>{status && <Facts parts={status} className={styles.episodeStatus} />}<Facts parts={airDate ? [...props.model.caption, airDate] : props.model.caption} className={styles.episodeLabel} /></span>{description && <span className={styles.description}>{description}</span>}</span>
  </CardFrame>;
}

export type CoverCardProps = MediaCardProps & Readonly<{
  kind?: 'book' | 'comic' | 'magazine' | 'newspaper';
  generated?: Omit<Parameters<typeof GeneratedCover>[0], 'className' | 'style'>;
  titleLines?: 1 | 2;
}>;
export function CoverCard({kind = 'book', generated, titleLines, ...props}: CoverCardProps) {
  const book = kind === 'book' || kind === 'comic';
  return <CardFrame {...props} kind="cover" playIcon="i-book" className={cx(book && !generated && styles.spine, !book && styles.periodical, kind === 'newspaper' && styles.newspaper, props.className)}>
    <span className={styles.art}><CardArtwork {...props} position={kind === 'newspaper' ? 'top' : props.position} fallback={generated ? <GeneratedCover {...generated} /> : props.fallback} /><CardBadges watched={props.watched} isNew={props.isNew} progress={props.model.progress} /></span><CardCaption model={props.model} titleLines={titleLines} />
  </CardFrame>;
}

export function RankedCard({rank, ...props}: MediaCardProps & {rank: number}) {
  return <div className={styles.ranked}><span className={styles.rank} aria-hidden="true">{rank}</span><PosterCard {...props} /></div>;
}

export type PlaylistMosaicProps = Omit<MediaCardProps, 'arrivalDurationMs'> & Readonly<{covers: readonly ArtworkSource[]}>;
export function PlaylistMosaic({covers, ...props}: PlaylistMosaicProps) {
  return <SquareCard {...props} artwork={undefined} fallback={<CoverMosaic covers={covers} shape="square" alt="" visible={props.visible} />} />;
}

export function NewPlaylistCard({model, onCreate, className}: {model: MediaCardProps['model']; onCreate: () => void; className?: string}) {
  return <SquareCard model={model} onNavigate={onCreate} className={cx(styles.newPlaylist, className)} fallback={<span className={styles.createGlyph}><Icon name="i-plus" size={28} /></span>} />;
}

export function BoundedCovers({covers, shape = 'poster', limit = 3, visible, arrivalDurationMs, className}: {covers: readonly ArtworkSource[]; shape?: 'poster' | 'square' | 'periodical'; limit?: 3 | 4; visible?: boolean; arrivalDurationMs?: number; className?: string}) {
  return <span className={cx(styles.coverFan, styles[`fan-${shape}`], className)} aria-hidden="true">{covers.slice(0, limit).map((source, index) => <span key={`${source.key}-${index}`} className={styles.fanItem}><Artwork source={source} alt="" visible={visible} arrivalDurationMs={arrivalDurationMs} /></span>)}</span>;
}

export type CardAmbientProps = Pick<MediaCardProps, 'ambient' | 'palette' | 'visible' | 'arrivalDurationMs'> & {treatment?: 'cover' | 'library'};
export function CardAmbient({ambient, palette, visible, arrivalDurationMs, treatment = 'cover'}: CardAmbientProps) {
  return ambient || palette ? <span className={styles.ambient}><AmbientArt source={ambient} palette={palette} treatment={treatment} visible={visible} arrivalDurationMs={arrivalDurationMs} /></span> : null;
}

export type CardArtSlot = ReactNode;
