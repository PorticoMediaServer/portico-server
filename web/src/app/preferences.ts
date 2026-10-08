import React, {createContext, useCallback, useContext, useEffect, useMemo, useState} from 'react';

/** Installation-scoped viewing preferences. Device conveniences, never account state. */
export type Preferences = {
  reduceMotion: boolean;
  posterSize: 'compact' | 'regular' | 'large';
  libraryView: 'grid' | 'list';
  /** WEB-LIB-04: grid or list per library (the global value is the default for a library not yet set). */
  libraryViews: Readonly<Record<string, 'grid' | 'list'>>;
  railExpanded: boolean;
};
const defaults: Preferences = {reduceMotion: false, posterSize: 'regular', libraryView: 'grid', libraryViews: {}, railExpanded: true};
const key = 'portico.preferences.v1';

function read(): Preferences {
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return defaults;
    const r = JSON.parse(raw) as Partial<Preferences>;
    return {
      reduceMotion: typeof r.reduceMotion === 'boolean' ? r.reduceMotion : defaults.reduceMotion,
      posterSize: r.posterSize === 'compact' || r.posterSize === 'large' ? r.posterSize : 'regular',
      libraryView: r.libraryView === 'list' ? 'list' : 'grid',
      libraryViews: r.libraryViews && typeof r.libraryViews === 'object' ? Object.fromEntries(Object.entries(r.libraryViews).filter(([k, v]) => k.length <= 128 && (v === 'grid' || v === 'list')).slice(0, 200)) as Record<string, 'grid' | 'list'> : {},
      railExpanded: typeof r.railExpanded === 'boolean' ? r.railExpanded : true,
    };
  } catch {
    return defaults;
  }
}

type Value = {preferences: Preferences; update: (patch: Partial<Preferences>) => void};
const PreferencesContext = createContext<Value>({preferences: defaults, update: () => {}});

export function PreferencesProvider({children}: {children: React.ReactNode}) {
  const [preferences, setPreferences] = useState(read);
  const update = useCallback((patch: Partial<Preferences>) => {
    setPreferences(prev => {
      // An identical patch (the appearance bridge re-applies server values)
      // must not write storage or re-render the shell.
      if ((Object.keys(patch) as (keyof Preferences)[]).every(k => prev[k] === patch[k])) return prev;
      const next = {...prev, ...patch};
      try {
        localStorage.setItem(key, JSON.stringify(next));
      } catch {}
      return next;
    });
  }, []);
  useEffect(() => {
    document.documentElement.dataset.reduceMotion = preferences.reduceMotion ? 'true' : 'false';
    document.documentElement.dataset.posterSize = preferences.posterSize;
  }, [preferences.reduceMotion, preferences.posterSize]);
  const value = useMemo(() => ({preferences, update}), [preferences, update]);
  return React.createElement(PreferencesContext.Provider, {value}, children);
}
export const usePreferences = () => useContext(PreferencesContext);
