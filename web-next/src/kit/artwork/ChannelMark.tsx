import type {CSSProperties} from 'react';
import type {ChannelMarkModel} from '@portico/client-core/kit-model';
import {Artwork, artClass, artSize} from './Artwork';
import type {ArtworkSource, ArtworkStatus, ArtStyleProps} from './types';
import styles from './artwork.module.css';

export type ChannelMarkProps = ArtStyleProps & Readonly<{
  mark: ChannelMarkModel; source?: ArtworkSource | null; size?: number; tint?: string; alt?: string;
  visible?: boolean; onStatusChange?: (status: ArtworkStatus) => void;
}>;
/** Requires a server-authoritative mark. A page of channels cannot establish the domain. */
export function ChannelMark({mark, source, size = 34, tint, alt = '', visible = true, onStatusChange, className, style, width = size, height = size}: ChannelMarkProps) {
  return <span className={artClass(styles.channelMark, className)} style={artSize({width, height, style: {...(tint ? {'--channel-tint': tint} : {}), ...style} as CSSProperties})} role={alt ? 'img' : undefined} aria-label={alt || undefined} aria-hidden={alt ? undefined : true}>
    <span className={styles.monogram}>{mark.text}</span>
    {source && <Artwork source={source} alt="" fit="contain" visible={visible} className={styles.channelLogo} onStatusChange={onStatusChange} />}
  </span>;
}
