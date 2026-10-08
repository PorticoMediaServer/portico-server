import React, {createContext, useCallback, useContext, useEffect, useMemo, useState} from 'react';
import {store} from './storage';

/** Browser-local interface preferences (profile preferences live on the server, see Settings). */
export type UiPrefs = {
  sidebar: 'expanded' | 'collapsed';
  cardSize: 'small' | 'medium' | 'large';
  reduceMotion: boolean;
  showBackdrops: boolean;
  /** Per-library view mode, keyed by library id. */
  libraryLayout: Record<string, 'grid' | 'list'>;
  homeHero: boolean;
};
const DEFAULTS: UiPrefs = {sidebar: 'expanded', cardSize: 'medium', reduceMotion: false, showBackdrops: true, libraryLayout: {}, homeHero: true};
type Ctx = {prefs: UiPrefs; update: (patch: Partial<UiPrefs>) => void};
const PrefsContext = createContext<Ctx>({prefs: DEFAULTS, update: () => {}});

export function PrefsProvider({children}: {children: React.ReactNode}) {
  const [prefs, setPrefs] = useState<UiPrefs>(() => ({...DEFAULTS, ...store.get<Partial<UiPrefs>>('ui', {})}));
  const update = useCallback((patch: Partial<UiPrefs>) => setPrefs(p => { const next = {...p, ...patch}; store.set('ui', next); return next; }), []);
  useEffect(() => {
    const root = document.documentElement;
    root.dataset.reduceMotion = String(prefs.reduceMotion);
    root.dataset.cardSize = prefs.cardSize;
    root.dataset.backdrops = prefs.showBackdrops ? 'on' : 'off';
  }, [prefs.reduceMotion, prefs.cardSize, prefs.showBackdrops]);
  const value = useMemo(() => ({prefs, update}), [prefs, update]);
  return <PrefsContext.Provider value={value}>{children}</PrefsContext.Provider>;
}
export const usePrefs = () => useContext(PrefsContext);
