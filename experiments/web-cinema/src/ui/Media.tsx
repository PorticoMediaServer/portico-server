import React, {useState} from 'react';
import {artUrl} from '../data/art';
import type {ArtRef} from '../data/types';
import {Icon, type IconName} from './Icon';
import {cx} from './cx';

/**
 * Artwork with its dominant colour as the placeholder, a quick fade once decoded, and a graceful
 * typographic fallback when there is no image (never an empty box).
 */
export function Art({src, alt = '', tint, ratio, className, fallback, fallbackIcon = 'film', eager, position, style, sizes}: {src?: ArtRef | null; alt?: string; tint?: string; ratio?: string; className?: string; fallback?: string; fallbackIcon?: IconName; eager?: boolean; position?: string; style?: React.CSSProperties; sizes?: string}) {
  const url = artUrl(src);
  const [loaded, setLoaded] = useState(false);
  const [failed, setFailed] = useState(false);
  return (
    <span className={cx('art', loaded && 'is-loaded', className)} style={{'--art-tint': tint ?? '#1d3848', aspectRatio: ratio, ...style} as React.CSSProperties}>
      {url && !failed ? (
        <img src={url} alt={alt} loading={eager ? 'eager' : 'lazy'} decoding="async" sizes={sizes} onLoad={() => setLoaded(true)} onError={() => setFailed(true)} style={position ? {objectPosition: position} : undefined} ref={el => { if (el?.complete && el.naturalWidth && !loaded) setLoaded(true); }} />
      ) : (
        <span className="art-fallback" aria-hidden={!alt} role={alt ? 'img' : undefined} aria-label={alt || undefined}>
          <Icon name={fallbackIcon} size={26} />
          {fallback ? <span className="art-fallback-text">{fallback}</span> : null}
        </span>
      )}
    </span>
  );
}

export function Avatar({name, color, size = 32, src, className, ring}: {name: string; color?: string; size?: number; src?: ArtRef; className?: string; ring?: boolean}) {
  const url = artUrl(src);
  const initials = name.split(/\s+/).filter(Boolean).slice(0, 2).map(p => p[0]!.toUpperCase()).join('') || '?';
  return (
    <span className={cx('avatar', ring && 'has-ring', className)} style={{width: size, height: size, fontSize: size * 0.4, '--avatar': color ?? 'var(--accent)'} as React.CSSProperties} aria-hidden>
      {url ? <img src={url} alt="" /> : initials.slice(0, size < 30 ? 1 : 2)}
    </span>
  );
}

/** Portico's wordmark and Play-P symbol, the official artwork, unmodified. */
export function Wordmark({height = 18, className}: {height?: number; className?: string}) {
  return <img src="/brand/portico-wordmark-mono-white.svg" alt="Portico" className={cx('brand-wordmark', className)} style={{height, width: 'auto'}} />;
}
export function BrandSymbol({size = 28, className}: {size?: number; className?: string}) {
  return <img src="/brand/portico-symbol-mono-white.svg" alt="Portico" className={cx('brand-symbol', className)} style={{height: size, width: 'auto'}} />;
}
