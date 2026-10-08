import {useCallback, useSyncExternalStore} from 'react';

/**
 * This browser's display preferences (the profile's playback and language preferences live on the
 * server, in Settings › Preferences). Stored in localStorage; reading never throws.
 */
export type Prefs = {
  railCollapsed: boolean;
  reduceMotion: boolean;
  textSize: 'default' | 'large';
  posterSize: 'compact' | 'regular' | 'large';
  libraryLayout: 'grid' | 'list';
  autoplayPreviews: boolean;
  showBackdrops: boolean;
  volume: number;
  muted: boolean;
};
const defaults: Prefs = {railCollapsed: false, reduceMotion: false, textSize: 'default', posterSize: 'regular', libraryLayout: 'grid', autoplayPreviews: true, showBackdrops: true, volume: 0.8, muted: false};
const KEY = 'portico.prefs';
let prefs: Prefs = (() => {
  try { return {...defaults, ...JSON.parse(localStorage.getItem(KEY) ?? '{}')}; } catch { return defaults; }
})();
const listeners = new Set<() => void>();

function apply() {
  const root = document.documentElement;
  root.dataset.reduceMotion = String(prefs.reduceMotion);
  root.dataset.textSize = prefs.textSize;
  root.dataset.posterSize = prefs.posterSize;
  root.dataset.backdrops = prefs.showBackdrops ? 'on' : 'off';
}
apply();

export function updatePrefs(patch: Partial<Prefs>) {
  prefs = {...prefs, ...patch};
  try { localStorage.setItem(KEY, JSON.stringify(prefs)); } catch { /* ignore */ }
  apply();
  listeners.forEach(l => l());
}
export const getPrefs = () => prefs;

export function usePrefs() {
  const p = useSyncExternalStore(l => { listeners.add(l); return () => listeners.delete(l); }, () => prefs);
  const update = useCallback((patch: Partial<Prefs>) => updatePrefs(patch), []);
  return {prefs: p, update};
}

export const reducedMotion = () => prefs.reduceMotion || window.matchMedia('(prefers-reduced-motion: reduce)').matches;
