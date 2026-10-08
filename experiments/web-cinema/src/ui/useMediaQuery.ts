import {useSyncExternalStore} from 'react';

export function useMediaQuery(query: string): boolean {
  return useSyncExternalStore(
    cb => {
      const m = window.matchMedia(query);
      m.addEventListener('change', cb);
      return () => m.removeEventListener('change', cb);
    },
    () => window.matchMedia(query).matches,
    () => false,
  );
}

export const useCompact = () => useMediaQuery('(max-width: 767px)');
export const useHover = () => useMediaQuery('(hover: hover) and (pointer: fine)');
