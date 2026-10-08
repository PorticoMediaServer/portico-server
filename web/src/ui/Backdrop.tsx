import React, {useEffect, useState} from 'react';
import {useArtworkUrl} from './Artwork';
import {retainArtworkUrl} from './artwork-store';
import {cx} from './cx';
import s from './Backdrop.module.css';

/**
 * Media-led page backdrop. The atmosphere stays underneath; the image
 * crossfades in after decode and a continuous scrim carries it into the
 * page. Never a hard edge, never a black void on failure.
 *
 * X-01: no backdrop art when the viewer turned `appearance.showBackdrops` off.
 * The shell publishes the choice as `data-backdrops="off"` on the document
 * root; the atmosphere (glow + scrim) stays so the page never goes void.
 */
function useBackdropsOff(): boolean {
  const [off, setOff] = useState(() => typeof document !== 'undefined' && document.documentElement.dataset.backdrops === 'off');
  useEffect(() => {
    const root = document.documentElement;
    const obs = new MutationObserver(() => setOff(root.dataset.backdrops === 'off'));
    obs.observe(root, {attributes: true, attributeFilter: ['data-backdrops']});
    return () => obs.disconnect();
  }, []);
  return off;
}

export function Backdrop({path, height, opacity, position, className}: {path?: string; height?: string; opacity?: number; position?: string; className?: string}) {
  const backdropsOff = useBackdropsOff();
  // The hero is the first thing on screen: it never waits behind a row of posters.
  // PERF-28: phones take the 800 px variant rather than the 1920 px one (≈ 8 MB decoded per layer).
  // PERF-22: backdrops live in the store's separate 4-entry large pool, outside the thumbnail byte budget.
  const [narrow] = useState(() => typeof window !== 'undefined' && window.innerWidth < 700);
  const {url} = useArtworkUrl(narrow ? narrowVariant(path) : path, 'high', undefined, {large: true});
  const [layers, setLayers] = useState<{a?: string; b?: string; front: 'a' | 'b'}>({front: 'a'});
  useEffect(() => (layers.a ? retainArtworkUrl(layers.a) : undefined), [layers.a]);
  useEffect(() => (layers.b ? retainArtworkUrl(layers.b) : undefined), [layers.b]);
  useEffect(() => {
    if (!url) return;
    const img = new Image();
    img.src = url;
    let active = true;
    (img.decode ? img.decode().catch(() => {}) : Promise.resolve()).then(() => {
      if (!active) return;
      setLayers(prev => (prev[prev.front] === url ? prev : prev.front === 'a' ? {...prev, b: url, front: 'b'} : {...prev, a: url, front: 'a'}));
    });
    return () => {
      active = false;
    };
  }, [url]);
  return (
    <div className={cx(s.backdrop, className)} aria-hidden style={{'--backdrop-height': height, '--backdrop-opacity': opacity, '--backdrop-position': position} as React.CSSProperties}>
      {!backdropsOff && layers.a ? <div className={cx(s.layer, layers.front === 'a' && s.shown)} style={{backgroundImage: `url("${layers.a}")`}} /> : null}
      {!backdropsOff && layers.b ? <div className={cx(s.layer, layers.front === 'b' && s.shown)} style={{backgroundImage: `url("${layers.b}")`}} /> : null}
      <div className={s.glow} />
      <div className={s.scrim} />
    </div>
  );
}

/** The 800 px variant (`w=800`, lane B). Unlike cards it keeps the full image on servers without
 *  variants, which ignore `w`, rather than falling back to their small thumbnail. */
function narrowVariant(path?: string): string | undefined {
  if (!path) return path;
  const url = new URL(path, 'https://artwork.invalid');
  if (!/^\/v1\//.test(url.pathname) || url.searchParams.has('candidate')) return path;
  url.searchParams.set('w', '800');
  return path.startsWith('/') ? url.pathname + url.search + url.hash : url.href;
}
