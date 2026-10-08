import type {HttpLocalApi} from '@core/index.ts';
import {ArtworkScheduler, type ArtworkScreenStats} from '@core/artwork/index.ts';
import {ArtworkReadError, protectedArtwork} from '../bridge/protectedArtwork';

/**
 * Authorized artwork for the web: images need a bearer token, which <img>
 * cannot send, so bytes are fetched once per path and handed to the DOM as an
 * object URL (revoked when evicted or disposed).
 *
 * The policy is the shared `ArtworkScheduler` (packages/client-core/src/artwork,
 * WEB-SYS-01): Retry-After or 5 s / 15 s / 45 s backoff without holding a slot,
 * priority-only slots for the hero, queued work nobody waits for dropped, a
 * failed cooldown and a bounded LRU cache. This file only plugs in the web
 * loader. Viewport interest is the caller's `signal` (see `useArtworkUrl`).
 */
/** `order`: screen position (row × 1000 + column; lower starts first). `group`: the row, which fills left to right once started. */
export type ArtworkLoadOptions = {signal?: AbortSignal; priority?: 'high' | 'normal'; order?: number; group?: string; /** PERF-22: backdrops and other full-size images live in the 4-entry large pool. */ large?: boolean};
export type ArtworkStoreOptions = {
  now?: () => number;
  setTimer?: (fn: () => void, ms: number) => ReturnType<typeof setTimeout>;
  clearTimer?: (t: ReturnType<typeof setTimeout>) => void;
  /** Per-request timeout; thumbnails are small, so a slow one is better retried than waited on. */
  timeoutMs?: number;
  /** Parallel requests; default follows the page's HTTP version (see `defaultConcurrency`). */
  concurrency?: number;
  /** Byte budget override (tests); default follows `deviceMemory` (see `ARTWORK_MAX_BYTES`). */
  maxBytes?: number;
  onScreenSettled?: (stats: ArtworkScreenStats) => void;
};

/**
 * HTTP/1.1 gives a host six connections shared by every API call and image,
 * so artwork takes four and leaves room for the rest; HTTP/2 and HTTP/3
 * multiplex, so more images can be in flight without starving API calls.
 */
export function defaultConcurrency(protocol = pageProtocol()): number {
  return protocol === 'h2' || protocol === 'h3' ? 10 : 4;
}
function pageProtocol(): string {
  try { return (performance.getEntriesByType('navigation')[0] as PerformanceNavigationTiming | undefined)?.nextHopProtocol ?? ''; } catch { return ''; }
}

// Object URLs pin encoded bytes in memory; a low-memory device keeps fewer.
const deviceMemory = typeof navigator === 'undefined' ? undefined : (navigator as Navigator & {deviceMemory?: number}).deviceMemory;
const MAX = deviceMemory !== undefined && deviceMemory <= 2 ? 150 : deviceMemory !== undefined && deviceMemory <= 4 ? 300 : 600;
/** PERF-22: the normal pool is capped by bytes, counting each blob's `size`: 16 MB when `deviceMemory ≤ 2`, else 48 MB. */
export const ARTWORK_MAX_BYTES = deviceMemory !== undefined && deviceMemory <= 2 ? 16 * 1024 * 1024 : 48 * 1024 * 1024;
/** PERF-22: large images (backdrops, Now Playing art, trickplay sheets) live in a separate LRU of 4, outside the byte budget. */
export const ARTWORK_MAX_LARGE = 4;
export const ARTWORK_BACKOFF_MS = [5000, 15000, 45000] as const;

/**
 * PERF-22: which pool a path belongs to. Sized thumbnails (`w=`/`size=`) are
 * the normal pool; unsized server artwork (the full/display variant) and
 * trickplay sheets are large. Callers with full-size images (the backdrop)
 * pass `large: true` explicitly; the heuristic covers the call sites M2 does
 * not own (player trickplay tiles, channel logos).
 */
export function isLargeArtworkPath(path: string): boolean {
  if (path.includes('/trickplay/')) return true;
  let url: URL;
  try { url = new URL(path, 'https://artwork.invalid'); } catch { return false; }
  if (!/^\/v1\//.test(url.pathname) || url.searchParams.has('candidate')) return false;
  if (url.searchParams.has('w') || url.searchParams.has('size')) return false;
  return true;
}

/** Encoded bytes per live object URL (dropped when the URL is revoked). */
const artworkSizes = new Map<string, number>();
function trackArtworkSize(url: string, size: number) { artworkSizes.set(url, Math.max(0, size)); }
function readArtworkSize(url: string): number { return artworkSizes.get(url) ?? 0; }
function forgetArtworkSize(url: string) { artworkSizes.delete(url); }

/**
 * PERF-22: an evicted object URL is revoked only once nothing on screen shows it; a mounted
 * image keeps its bytes (and a remount of it never breaks) until it unmounts.
 */
const mounted = new Map<string, number>();
const evicted = new Set<string>();
function revokeArtworkUrl(url: string) {
  URL.revokeObjectURL(url);
  forgetArtworkSize(url);
}
export function releaseArtworkUrl(url: string) {
  if (mounted.has(url)) evicted.add(url);
  else revokeArtworkUrl(url);
}
/** Marks `url` as shown; the returned function un-marks it (and revokes it if it was evicted meanwhile). */
export function retainArtworkUrl(url: string): () => void {
  mounted.set(url, (mounted.get(url) ?? 0) + 1);
  let done = false;
  return () => {
    if (done) return;
    done = true;
    const left = (mounted.get(url) ?? 1) - 1;
    if (left > 0) { mounted.set(url, left); return; }
    mounted.delete(url);
    if (evicted.delete(url)) revokeArtworkUrl(url);
  };
}

export class ArtworkStore {
  private scheduler: ArtworkScheduler<string>;
  /** `token` may be a getter, so a rotated access token is used without replacing the store. */
  constructor(api: HttpLocalApi, token: string | (() => string), options: ArtworkStoreOptions = {}) {
    const bearer = typeof token === 'function' ? token : () => token;
    this.scheduler = new ArtworkScheduler<string>({
      // The scheduler owns the timeout (its abort is classified as a retryable timeout).
      // PERF-22: the blob's `size` is counted toward the byte budget.
      load: (path, signal) => protectedArtwork(api, path, bearer(), signal, 0).then(blob => {
        const url = URL.createObjectURL(blob);
        trackArtworkSize(url, blob.size);
        return url;
      }),
      classify: e => (e instanceof ArtworkReadError ? {retryable: e.retryable, retryAfterSeconds: e.retryAfter} : {retryable: (e as {name?: string} | null)?.name !== 'AbortError'}),
      release: url => releaseArtworkUrl(url),
      maxEntries: MAX,
      maxBytes: options.maxBytes ?? ARTWORK_MAX_BYTES,
      sizeOf: readArtworkSize,
      // PERF-22: a mounted URL survives eviction pressure; it is evicted later when its count drops to 0.
      isPinned: url => mounted.has(url),
      isLargeKey: isLargeArtworkPath,
      maxLargeEntries: ARTWORK_MAX_LARGE,
      concurrency: options.concurrency ?? defaultConcurrency(),
      onScreenSettled: options.onScreenSettled,
      backoffMs: ARTWORK_BACKOFF_MS,
      timeoutMs: options.timeoutMs ?? 8000,
      now: options.now,
      setTimer: options.setTimer as ((fn: () => void, ms: number) => unknown) | undefined,
      clearTimer: options.clearTimer as ((t: unknown) => void) | undefined,
    });
  }
  peek(path: string): string | undefined { return this.scheduler.peek(path); }
  subscribe(listener: () => void): () => void { return this.scheduler.subscribe(listener); }
  load(path: string, options: ArtworkLoadOptions = {}): Promise<string> { return this.scheduler.load(path, options); }
  /** The viewport moved: a waiting request takes its new screen position. */
  reorder(path: string, order: number, group?: string) { this.scheduler.reorder(path, order, group); }
  /** Start measuring a screen load (time to first image, time to all visible). */
  beginScreen(label: string) { this.scheduler.beginScreen(label); }
  stats() { return this.scheduler.stats(); }
  /** Terminal: aborts work, clears timers, revokes every URL, rejects waiters. */
  dispose() { this.scheduler.dispose(); }
}

/**
 * PERF-22: one shared IntersectionObserver owned by the store serves every
 * card — a 600 px lookahead plus on-screen ordering from a single observer,
 * not one observer per card.
 */
/** Screen position for the artwork queue: row × 1000 + column, so work starts top-left first and a row fills left to right. */
export type ArtworkPlace = {order: number; group: string};
/** Rows are bucketed by their top edge; cards in one shelf or grid row share it. */
const ROW_PX = 24;
export function placeOf(el: Element): ArtworkPlace {
  const r = el.getBoundingClientRect();
  const row = Math.max(0, Math.round(r.top / ROW_PX));
  return {order: row * 1000 + Math.max(0, Math.min(999, Math.round(r.left / 8))), group: `row:${row}`};
}
/** Offscreen lookahead waits behind everything on screen. */
export const ARTWORK_LOOKAHEAD = 1_000_000;
export const artworkInViewport = (e: IntersectionObserverEntry) => e.boundingClientRect.bottom > 0 && e.boundingClientRect.top < (typeof window === 'undefined' ? 0 : window.innerHeight);

type ArtworkWatch = (entry: IntersectionObserverEntry) => void;
const watches = new WeakMap<Element, ArtworkWatch>();
let sharedObserver: IntersectionObserver | undefined;
function artworkObserver(): IntersectionObserver {
  return (sharedObserver ??= new IntersectionObserver(
    entries => { for (const e of entries) watches.get(e.target)?.(e); },
    // Threshold crossings re-report position as a card scrolls from the
    // lookahead into view, so its queue order follows it with one observer.
    {rootMargin: '600px', threshold: [0, 0.5, 1]},
  ));
}
/** Watch one frame with the shared observer; the returned function un-watches it. */
export function observeArtwork(el: Element, watch: ArtworkWatch): () => void {
  const o = artworkObserver();
  watches.set(el, watch);
  o.observe(el);
  return () => { o.unobserve(el); if (watches.get(el) === watch) watches.delete(el); };
}
