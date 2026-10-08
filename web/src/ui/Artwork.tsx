import React, {createContext, useContext, useEffect, useRef, useState} from 'react';
import {cx} from './cx';
import {artworkWidthBucket} from '@core/artwork/index.ts';
import {sizedArtworkPath} from './artwork-path';
import {Icon, type IconName} from './Icon';
import {ARTWORK_LOOKAHEAD, artworkInViewport, observeArtwork, placeOf, retainArtworkUrl, type ArtworkPlace, type ArtworkStore} from './artwork-store';
import s from './Artwork.module.css';

export type {ArtworkPlace};

export type ArtworkShape = 'poster' | 'square' | 'landscape' | 'wide' | 'circle' | 'logo';

export const ArtworkContext = createContext<ArtworkStore | null>(null);

/** Resolves an authorized artwork path to a displayable URL; undefined while loading or on failure. */
export function useArtworkUrl(path?: string, priority: 'high' | 'normal' = 'normal', place?: ArtworkPlace, options: {large?: boolean} = {}): {url?: string; failed: boolean} {
  const store = useContext(ArtworkContext);
  const [state, setState] = useState<{path?: string; url?: string; failed: boolean}>({path, url: path ? store?.peek(path) : undefined, failed: false});
  useEffect(() => {
    if (!path || !store) {
      setState({path, failed: false});
      return;
    }
    const cached = store.peek(path);
    if (cached) {
      // A revisit paints from the first render; skip the update when the state already matches.
      setState(prev => (prev.path === path && prev.url === cached && !prev.failed ? prev : {path, url: cached, failed: false}));
      return;
    }
    let active = true;
    // Leaving (scrolled away, unmounted, new path) withdraws interest, so the
    // store can drop queued work nobody will see. Retries happen inside the
    // store; this promise simply resolves later when the server catches up.
    const controller = new AbortController();
    setState({path, failed: false});
    store.load(path, {signal: controller.signal, priority, order: place?.order, group: place?.group, large: options.large}).then(url => active && setState({path, url, failed: false})).catch(() => active && setState({path, failed: true}));
    return () => {
      active = false;
      controller.abort();
    };
  }, [path, store, priority, options.large]);
  return state.path === path ? {url: state.url, failed: state.failed} : {failed: false};
}

/**
 * Artwork with reserved geometry. The frame exists before bytes arrive; the
 * image fades in after decode; failure keeps the frame and shows a quiet
 * placeholder, never a broken glyph.
 */
export function Artwork({path, shape = 'poster', alt = '', icon = 'film', initial, progress, className, fit, sizes}: {path?: string; shape?: ArtworkShape; alt?: string; icon?: IconName; initial?: string; progress?: number; className?: string; fit?: 'cover' | 'contain'; sizes?: string}) {
  // Bytes travel through fetch rather than <img>: the fetch waits until the
  // frame nears the viewport, which is what keeps a long grid from downloading
  // every poster at once. The <img> itself isn't lazy (its bytes are already in
  // memory, and a lazy image in a background tab never decodes).
  const frame = useRef<HTMLDivElement>(null);
  const store = useContext(ArtworkContext);
  // PERF-22: artwork this page already showed paints on the first render, with no placeholder
  // frame and no fade (Back to Home, a revisited shelf).
  const [seen] = useState(() => { const key = path ? lastRequested.get(path) : undefined; return key && store?.peek(key) ? key : undefined; });
  const [near, setNear] = useState(() => !!seen || typeof IntersectionObserver === 'undefined');
  const [place, setPlace] = useState<ArtworkPlace>();
  const [bucket, setBucket] = useState<number>();
  // Interest follows the viewport both ways: a card that scrolls far away
  // withdraws its queued request (the store drops it before it starts), and a
  // settled image comes straight back from the cache when it returns.
  // Its screen position orders the queue: what's on screen, top-left first;
  // the 600 px lookahead after that. One shared observer serves every card.
  useEffect(() => {
    const el = frame.current;
    if (!el || typeof IntersectionObserver === 'undefined') return;
    const width = el.getBoundingClientRect().width;
    setBucket(artworkWidthBucket(width * (window.devicePixelRatio || 1), {aspect: aspectOf(shape)}));
    return observeArtwork(el, entry => {
      if (entry.isIntersecting) {
        const p = placeOf(el);
        setPlace(entry.intersectionRatio > 0 && artworkInViewport(entry) ? p : {order: p.order + ARTWORK_LOOKAHEAD, group: p.group});
      }
      setNear(entry.isIntersecting);
    });
  }, [shape]);
  const measured = near && bucket !== undefined ? sizedArtworkPath(path, bucket) : undefined;
  // Until the frame is measured, keep showing what this card showed last time.
  const requested = measured && (!seen || store?.peek(measured)) ? measured : near ? seen ?? measured : undefined;
  useEffect(() => { if (path && requested) remember(path, requested); }, [path, requested]);
  // A waiting request follows the card when it scrolls into view.
  useEffect(() => { if (requested && place) store?.reorder(requested, place.order, place.group); }, [store, requested, place]);
  const {url} = useArtworkUrl(requested, 'normal', place);
  useEffect(() => (url ? retainArtworkUrl(url) : undefined), [url]);
  // A cached blob can load before passive effects run. Never reset its ready
  // flag after onLoad: associate readiness with the exact displayed URL instead.
  const [decodedUrl, setDecodedUrl] = useState<string>();
  const instant = !!seen && requested === seen;
  const decoded = !!url && (decodedUrl === url || instant);
  const contain = fit === 'contain' || shape === 'logo';
  return (
    <div ref={frame} className={cx(s.frame, s[shape], className)}>
      {/* Crossfades with the image instead of vanishing under it, and never waits on a neighbor. */}
      <div className={cx(s.placeholder, decoded && s.placeholderGone, instant && s.instant)} aria-hidden>
        {initial ? <span className={s.initial}>{initial}</span> : <Icon name={icon} size={shape === 'square' || shape === 'circle' ? 28 : 26} strokeWidth={1.4} />}
      </div>
      {url ? <img src={url} alt={alt} sizes={sizes} className={cx(s.image, contain && s.contain, decoded && s.shown, instant && s.instant)} onLoad={() => setDecodedUrl(url)} onError={() => setDecodedUrl(undefined)} decoding="async" draggable={false} /> : null}
      {progress && progress > 0 ? (
        <div className={s.progress} role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={Math.round(progress * 100)}>
          <i style={{width: `${Math.max(2, Math.min(100, progress * 100))}%`}} />
        </div>
      ) : null}
    </div>
  );
}

/** The sized path each artwork path last requested (bounded, most recent last). */
const lastRequested = new Map<string, string>();
const REMEMBERED = 2000;
function remember(path: string, requested: string) {
  lastRequested.delete(path);
  lastRequested.set(path, requested);
  if (lastRequested.size > REMEMBERED) lastRequested.delete(lastRequested.keys().next().value!);
}

const aspectOf = (shape: ArtworkShape) => (shape === 'poster' ? 2 / 3 : shape === 'landscape' || shape === 'wide' ? 16 / 9 : 1);
