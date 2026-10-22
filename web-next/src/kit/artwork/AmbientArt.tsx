import type {CSSProperties} from 'react';
import {Artwork, artClass, artSize} from './Artwork';
import type {AmbientArtSource, ArtworkPalette, ArtworkSource, ArtStyleProps, ArtworkStatus} from './types';
import styles from './artwork.module.css';

export type ColourBackdropProps = ArtStyleProps & Readonly<{shape?: 'side' | 'round' | 'top'; fill?: boolean}>;
/** Public/auth/state backgrounds intentionally have no artwork or glyph input. */
export function ColourBackdrop({shape = 'side', fill = false, className, style, width, height}: ColourBackdropProps) {
  return <div aria-hidden="true" className={artClass(styles.colour, styles[shape], fill && styles.fill, className)} style={artSize({width, height, style})} />;
}

export type AmbientArtProps = ArtStyleProps & Readonly<{
  source?: AmbientArtSource | null; sources?: readonly AmbientArtSource[];
  foreground?: ArtworkSource | null; palette?: ArtworkPalette;
  treatment?: 'hero' | 'title' | 'phone' | 'cover' | 'wash' | 'library' | 'artist' | 'poster';
  visible?: boolean; arrivalDurationMs?: number; foregroundStyle?: CSSProperties; position?: string;
  onReady?: (source: ArtworkSource) => void; onError?: (source: ArtworkSource) => void; onStatusChange?: (status: ArtworkStatus) => void;
}>;

/** One continuous masked surface; sharp art and tiny preblur remain independent requests. */
export function AmbientArt({source, sources, foreground, palette, treatment = 'hero', visible = true, arrivalDurationMs, foregroundStyle, position, onReady, onError, onStatusChange, className, style, width, height}: AmbientArtProps) {
  const ambient = (sources ?? (source ? [source] : [])).slice(0, 3).filter(item => item.variant === 'ambient-v1' && item.maxEdge === 64 && /^[a-f0-9]{64}$/.test(item.version));
  const colourStyle = palette ? {'--art-first': palette.first, '--art-second': palette.second} as CSSProperties : undefined;
  return <div aria-hidden="true" className={artClass(styles.ambient, styles[treatment], className)} style={artSize({width, height, style: {...colourStyle, ...style}})}>
    <div className={styles.washLayer}>
      {palette && <span className={styles.palette} />}
      {ambient.map((item, index) => <Artwork key={item.key} source={item} visible={visible} arrivalDurationMs={arrivalDurationMs} alt="" className={styles.ambientImage} style={{'--art-order': index} as CSSProperties} onStatusChange={!foreground && index === 0 ? onStatusChange : undefined} onReady={!foreground && index === 0 ? onReady : undefined} onError={!foreground && index === 0 ? onError : undefined} />)}
    </div>
    {foreground && <span className={styles.foreground}><span data-foreground-art className={styles.foregroundArt} style={foregroundStyle}><Artwork source={foreground} visible={visible} arrivalDurationMs={arrivalDurationMs} alt="" position={position} onReady={onReady} onError={onError} onStatusChange={onStatusChange} /></span></span>}
  </div>;
}
