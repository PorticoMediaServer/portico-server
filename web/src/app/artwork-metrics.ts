import {useEffect} from 'react';
import {useLocation} from '@tanstack/react-router';
import type {ArtworkScreenStats} from '@core/artwork/index.ts';
import type {ArtworkStore} from '../ui/artwork-store';

/**
 * Artwork timing per screen (lead's performance plan, item c): time to first
 * image and time until every requested image has settled, logged to the
 * console. On in dev builds; in a production build set
 * `localStorage['portico.debug.artwork'] = '1'` and reload. The last 50
 * screens are kept on `window.__porticoArtwork` for scripted runs.
 */
export function artworkMetricsEnabled(): boolean {
  if (import.meta.env?.DEV) return true;
  try { return localStorage.getItem('portico.debug.artwork') === '1'; } catch { return false; }
}

/** Route label with ids folded, so runs of the same screen compare. */
export function screenLabel(pathname: string, search = ''): string {
  const view = /(?:^|[?&])view=([a-z]+)/.exec(search)?.[1];
  return pathname.replace(/[A-Za-z0-9_-]{16,}/g, ':id') + (view ? `?view=${view}` : '');
}

export function logScreen(stats: ArtworkScreenStats) {
  const w = window as Window & {__porticoArtwork?: ArtworkScreenStats[]};
  (w.__porticoArtwork ??= []).push(stats);
  if (w.__porticoArtwork.length > 50) w.__porticoArtwork.shift();
  const ms = (v?: number) => (v === undefined ? '—' : `${Math.round(v)} ms`);
  console.info(`[artwork] ${stats.label}: first image ${ms(stats.firstImageMs)}, all settled ${ms(stats.allVisibleMs)} (${stats.requested} requested, ${stats.cached} cached, ${stats.failed} failed, ${stats.pending} pending)`);
}

/** Starts a measurement on every navigation. */
export function useArtworkScreens(store: ArtworkStore, enabled: boolean) {
  const location = useLocation();
  const label = screenLabel(location.pathname, location.searchStr);
  useEffect(() => { if (enabled) store.beginScreen(label); }, [store, label, enabled]);
}
