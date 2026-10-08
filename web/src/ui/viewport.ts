import {useEffect, useState} from 'react';

function watch(query: string) {
  return function useMedia(): boolean {
    const [matches, set] = useState(() => (typeof matchMedia === 'function' ? matchMedia(query).matches : false));
    useEffect(() => {
      const mq = matchMedia(query);
      const on = () => set(mq.matches);
      on();
      mq.addEventListener('change', on);
      return () => mq.removeEventListener('change', on);
    }, []);
    return matches;
  };
}
/** Phone-width shell: bottom tabs, sheets instead of dialogs, stacked layouts. */
export const useCompact = watch('(max-width: 720px)');
/** Tablet and below: the rail collapses to icons. */
export const useNarrow = watch('(max-width: 1080px)');
export const useHoverCapable = watch('(hover: hover)');
export const usePrefersReducedMotion = watch('(prefers-reduced-motion: reduce)');
