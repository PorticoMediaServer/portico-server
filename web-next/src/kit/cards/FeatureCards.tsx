import type {ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import type {ChannelMarkModel, FactPart} from '@portico/client-core/kit-model';
import {ChannelMark, type ArtworkSource} from '../artwork';
import {Icon} from '../Icon';
import {Button} from '../controls';
import {CardFrame, Facts, Progress, cx, tintStyle} from './CardFrame';
import {BoundedCovers, CardArtwork, CardAmbient} from './MediaCards';
import {boundedProgress} from './interaction';
import type {CardProps, MediaCardProps} from './types';
import styles from './cards.module.css';

export type LiveCardProps = Omit<CardProps, 'model'> & Readonly<{
  channel: Readonly<{sourceId: string; id: string; name: string; number?: string; mark: ChannelMarkModel}>;
  programme?: Readonly<{title: string; facts: readonly FactPart[]; progress?: number; live?: boolean; isNew?: boolean; premiere?: boolean; recording?: boolean}>;
  logo?: ArtworkSource;
  tint?: string;
}>;
export function LiveCard({channel, programme, logo, tint, ...props}: LiveCardProps) {
  return <CardFrame {...props} model={{title: programme?.title ?? channel.name, caption: []}} kind="live" style={{...tintStyle(tint), ...props.style}}>
    <span className={styles.channel}><ChannelMark mark={channel.mark} source={logo} tint={tint} size={34} visible={props.visible} /><span className={styles.channelName}>{channel.number && <span className={styles.channelNumber}>{channel.number}</span>}{channel.name}</span>
      {programme?.live && <span className={styles.liveFlag}>{defaultI18n.t('next.cards.live')}</span>}
    </span>
    <span className={styles.programme}>{programme?.title ?? channel.name}</span><span className={styles.programmeFlags}>{programme?.isNew && <span>{defaultI18n.t('next.cards.new')}</span>}{programme?.premiere && <span>{defaultI18n.t('next.cards.premiere')}</span>}{programme?.recording && <span className={styles.recording}><Icon name="i-rec" size={12} />{defaultI18n.t('next.cards.recording')}</span>}</span><Facts parts={programme?.facts} className={styles.liveWhen} /><Progress value={programme?.progress} />
  </CardFrame>;
}

type NavigationCardProps = Omit<MediaCardProps, 'onPlay' | 'playLabel'>;
export type CollectionCardProps = NavigationCardProps & Readonly<{
  covers: readonly ArtworkSource[];
  kindLabel?: string;
  description?: string;
  actionLabel?: string;
  onAction?: () => void;
  actionVariant?: 'primary' | 'glass';
}>;
function Collection({variant, covers, kindLabel, description, actionLabel, onAction, actionVariant = 'primary', ...props}: CollectionCardProps & {variant: 'collectionBanner' | 'collectionTile'}) {
  return <CardFrame {...props} onPlay={undefined} kind={variant} footer={onAction && <span className={styles.collectionAction}><Button variant={actionVariant} size="compact" icon={actionVariant === 'primary' ? 'i-play' : undefined} onClick={onAction}>{actionLabel ?? defaultI18n.t('next.cards.playAll')}</Button></span>}>
    <CardAmbient {...props} /><span className={styles.collectionBackdrop}><CardArtwork {...props} /></span><span className={styles.collectionCopy}>{kindLabel && <span className={styles.kindLabel}>{kindLabel}</span>}{variant === 'collectionTile' && <BoundedCovers covers={covers} visible={props.visible} arrivalDurationMs={props.arrivalDurationMs} limit={4} className={styles.collectionMinis} />}<span className={styles.collectionTitle}>{props.model.title}</span>{description && <span className={styles.collectionDescription}>{description}</span>}{variant === 'collectionBanner' ? <BoundedCovers covers={covers} visible={props.visible} arrivalDurationMs={props.arrivalDurationMs} limit={4} className={styles.collectionMinis} /> : <span className={styles.collectionMeta}><Facts parts={props.model.caption} className={styles.collectionFacts} /><Facts parts={props.model.detail} className={styles.collectionDetail} /></span>}</span>{variant === 'collectionTile' && <Progress value={props.model.progress} edge />}
  </CardFrame>;
}
export function CollectionBanner(props: CollectionCardProps) { return <Collection {...props} variant="collectionBanner" />; }
export function CollectionTile(props: CollectionCardProps) { return <Collection {...props} variant="collectionTile" />; }

export type GenreTileProps = NavigationCardProps & Readonly<{variant: 'video' | 'music'; covers?: readonly ArtworkSource[]; tint?: string}>;
export function GenreTile({variant, covers = [], tint, ...props}: GenreTileProps) {
  return <CardFrame {...props} onPlay={undefined} kind={variant === 'music' ? 'musicGenre' : 'videoGenre'} style={{...tintStyle(tint), ...props.style}}>{variant === 'video' ? <span className={styles.genreBackdrop}><CardArtwork {...props} /></span> : <BoundedCovers covers={covers} shape="square" visible={props.visible} arrivalDurationMs={props.arrivalDurationMs} className={styles.genreFan} />}<span className={styles.genreCopy}><span className={styles.genreTitle}>{props.model.title}</span><Facts parts={props.model.caption} className={styles.genreFacts} /></span></CardFrame>;
}

export type LibraryCardProps = NavigationCardProps & Readonly<{covers: readonly ArtworkSource[]; shape: 'poster' | 'square' | 'periodical'}>;
function Library({row, covers, shape, ...props}: LibraryCardProps & {row: boolean}) {
  return <CardFrame {...props} onPlay={undefined} kind={row ? 'libraryRow' : 'libraryCard'}><CardAmbient {...props} treatment="library" /><span className={styles.libraryCopy}><span className={styles.libraryTitle}>{props.model.title}</span><Facts parts={props.model.caption} className={styles.libraryFacts} /></span><BoundedCovers covers={covers} shape={shape} visible={props.visible} arrivalDurationMs={props.arrivalDurationMs} className={styles.libraryFan} /></CardFrame>;
}
export function LibraryCard(props: LibraryCardProps) { return <Library {...props} row={false} />; }
export function LibraryRow(props: LibraryCardProps) { return <Library {...props} row />; }

export type ResumeCardProps = MediaCardProps & Readonly<{location?: FactPart; remaining?: FactPart; kind?: 'book' | 'magazine' | 'newspaper'; generated?: ReactNode}>;
function Resume({read, location, remaining, kind = 'book', generated, ...props}: ResumeCardProps & {read: boolean}) {
  return <CardFrame {...props} kind={read ? 'readResume' : 'resume'} playIcon={read ? 'i-book' : 'i-play'} className={cx(read && kind === 'book' && !generated && styles.resumeSpine, kind !== 'book' && styles.resumePeriodical, props.className)}><CardAmbient {...props} /><span className={styles.resumeCover}><CardArtwork {...props} fallback={generated ?? props.fallback} position={kind === 'newspaper' ? 'top' : props.position} /></span><span className={styles.resumeCopy}><span className={styles.resumeTitle}>{props.model.title}</span><Facts parts={props.model.caption} className={styles.resumeByline} /><span className={styles.resumeWhere}>{location && <span>{location.text}</span>}{remaining && <span>{remaining.text}</span>}</span><Progress value={props.model.progress} /></span></CardFrame>;
}
export function ResumeCard(props: ResumeCardProps) { return <Resume {...props} read={false} />; }
export function ReadResumeCard(props: ResumeCardProps) { return <Resume {...props} read />; }

export type SeriesSegment = Readonly<{id: string; state: 'read' | 'in-progress' | 'unread'; progress?: number}>;
export type SeriesTileProps = NavigationCardProps & Readonly<{covers: readonly ArtworkSource[]; segments: readonly SeriesSegment[]; tint?: string}>;
export function SeriesTile({covers, segments, tint, ...props}: SeriesTileProps) {
  return <CardFrame {...props} onPlay={undefined} kind="series" style={{...tintStyle(tint), ...props.style}}><BoundedCovers covers={covers} visible={props.visible} arrivalDurationMs={props.arrivalDurationMs} className={styles.seriesFan} /><span className={styles.seriesCopy}><span className={styles.seriesTitle}>{props.model.title}</span><Facts parts={props.model.caption} className={styles.seriesFacts} /><span className={styles.segments} aria-hidden="true">{segments.map(segment => <span key={segment.id}><span style={{width: `${(segment.state === 'read' ? 1 : segment.state === 'in-progress' ? boundedProgress(segment.progress) ?? 0 : 0) * 100}%`}} /></span>)}</span></span></CardFrame>;
}

export type QuickTileProps = MediaCardProps & Readonly<{playing?: boolean; onPause?: () => void}>;
export function QuickTile({playing, onPause, ...props}: QuickTileProps) {
  return <CardFrame {...props} onPlay={playing ? onPause : props.onPlay} kind="quick" className={cx(playing && styles.playing, props.className)} playIcon={playing ? 'i-pause' : 'i-play'}><span className={styles.quickCover}><CardArtwork {...props} /></span><span className={styles.quickCopy}><span className={styles.quickTitle}>{props.model.title}</span><span className={styles.quickSubtitle}>{playing && <span className={styles.levelMeter} aria-hidden="true"><i /><i /><i /><i /></span>}<Facts parts={props.model.caption} /></span></span></CardFrame>;
}

export type SessionChannel = Readonly<{mark: ChannelMarkModel; logo?: ArtworkSource; tint?: string; facts: readonly FactPart[]}>;
export type SessionCardProps = NavigationCardProps & Readonly<{
  viewer: Readonly<{name: string; device: string; avatar?: ReactNode}>;
  facts: readonly FactPart[];
  paused?: boolean;
  onStop?: () => void;
}> & (Readonly<{media?: 'video' | 'audio'; channel?: never}> | Readonly<{media: 'channel'; channel: SessionChannel}>);
export function SessionCard({viewer, facts, paused, media = 'video', channel, onStop, ...props}: SessionCardProps) {
  return <CardFrame {...props} onPlay={undefined} onMore={undefined} kind="session" className={cx(paused && styles.paused, props.className)} overlay={<>
    {paused && <span className={styles.sessionState}><Icon name="i-pause" size={12} />{defaultI18n.t('next.cards.paused')}</span>}
    {(onStop || props.onMore) && <span className={styles.sessionActions}>{onStop && <Button variant="glass" size="xs" icon="i-stop" onClick={onStop}>{defaultI18n.t('next.cards.stop')}</Button>}{props.onMore && <Button variant="glass" size="xs" icon="i-more" iconOnly aria-label={defaultI18n.t('next.cards.more')} onClick={props.onMore} />}</span>}
  </>}>
    <span className={cx(styles.sessionArt, media === 'audio' && styles.sessionAudio, media === 'channel' && styles.sessionChannel)} style={tintStyle(channel?.tint)}>{channel ? <><ChannelMark mark={channel.mark} source={channel.logo} tint={channel.tint} size={32} visible={props.visible} /><Facts parts={channel.facts} /></> : <>{media === 'audio' && <CardAmbient {...props} />}<span className={styles.sessionForeground}><CardArtwork {...props} /></span></>}</span><span className={styles.sessionCopy}><span className={styles.sessionTitle}>{props.model.title}</span><Facts parts={props.model.caption} className={styles.sessionSubtitle} /><span className={styles.viewer}>{viewer.avatar}<strong>{viewer.name}</strong><span>{viewer.device}</span></span><Facts parts={facts} className={styles.sessionFacts} /></span><Progress value={props.model.progress} edge />
  </CardFrame>;
}
