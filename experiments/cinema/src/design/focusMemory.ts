import type {View} from 'react-native';

/**
 * Remembers the last focused element on TV so overlays can hand focus back to whatever opened
 * them when they close (the platform focus engine would otherwise pick its own preferred view).
 */
let last: View | null = null;
export const focusMemory = {
  set(v: View | null) {
    if (v) last = v;
  },
  take(): View | null {
    return last;
  },
  restore(v: View | null) {
    if (!v) return;
    setTimeout(() => {
      try {
        (v as unknown as {requestTVFocus?: () => void}).requestTVFocus?.();
      } catch {
        // The element may have unmounted; the focus engine falls back on its own.
      }
    }, 60);
  },
};
