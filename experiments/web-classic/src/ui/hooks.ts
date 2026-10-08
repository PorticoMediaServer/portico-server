import {useEffect, useState, useSyncExternalStore} from 'react';

/** Subscribes to a CSS media query. */
export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    fn => { const m = window.matchMedia(query); m.addEventListener('change', fn); return () => m.removeEventListener('change', fn); },
    () => window.matchMedia(query).matches,
    () => false,
  );
}
export const usePhone = () => useMediaQuery('(max-width: 767px)');
export const useNarrow = () => useMediaQuery('(max-width: 1279px)');

/** Re-renders every `ms` (clocks, live progress). */
export function useNow(ms = 30000): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => { const t = setInterval(() => setNow(Date.now()), ms); return () => clearInterval(t); }, [ms]);
  return now;
}

/** Sets the document title while mounted. */
export function useTitle(title: string | undefined) {
  useEffect(() => { if (title) document.title = `${title} · Portico`; }, [title]);
}

/** Tints the atmosphere glow toward a colour (the featured title's), or clears it. */
export function useGlowTint(color: string | undefined) {
  useEffect(() => {
    const root = document.documentElement;
    if (color) { root.style.setProperty('--glow-tint', `${color}`); root.style.setProperty('--glow-tint-opacity', '0.55'); }
    else root.style.setProperty('--glow-tint-opacity', '0');
    return () => { root.style.setProperty('--glow-tint-opacity', '0'); };
  }, [color]);
}
