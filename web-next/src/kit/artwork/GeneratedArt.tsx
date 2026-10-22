import type {CSSProperties} from 'react';
import {artClass, artSize} from './Artwork';
import type {ArtworkPalette, ArtStyleProps, GeneratedCoverTint} from './types';
import styles from './artwork.module.css';

export type GeneratedArtProps = ArtStyleProps & Readonly<{glyph: string; palette?: ArtworkPalette}>;
/** Missing-title art only. ColourBackdrop is used for auth, state and Server bands. */
export function GeneratedArt({glyph, palette, className, style, width, height}: GeneratedArtProps) {
  return <span aria-hidden="true" className={artClass(styles.generatedArt, className)} style={artSize({width, height, style: {...(palette ? {'--art-first': palette.first, '--art-second': palette.second} : {}), ...style} as CSSProperties})}>
    <span className={styles.generatedGlyph}>{glyph}</span>
  </span>;
}

export type GeneratedCoverProps = ArtStyleProps & Readonly<{
  title: string; author?: string; tint: GeneratedCoverTint; glyph: string;
  /** Authored CSS cqw percentage; defaults to 12.6 without title-based inference. */
  fontScale?: number;
}>;
export function GeneratedCover({title, author, tint, glyph, fontScale = 12.6, className, style, width, height}: GeneratedCoverProps) {
  return <span role="img" aria-label={title} className={artClass(styles.generatedCover, className)} style={artSize({width, height, style: {'--art-first': tint.first, '--art-second': tint.second, '--cover-font': `${fontScale}cqw`, ...style} as CSSProperties})}>
    <span className={styles.coverGlyph} aria-hidden="true">{glyph}</span>
    <span className={styles.coverCopy} aria-hidden="true"><b>{title}</b>{author && <small>{author}</small>}</span>
  </span>;
}
