import {useSyncExternalStore} from 'react';

const subscribe = (listener: () => void) => {
  window.addEventListener('resize', listener);
  return () => window.removeEventListener('resize', listener);
};
export function useShellWidth(): number { return useSyncExternalStore(subscribe, () => window.innerWidth, () => 1440); }
const subscribeScroll = (listener: () => void) => {
  window.addEventListener('scroll', listener, {passive: true});
  return () => window.removeEventListener('scroll', listener);
};
export function useShellScrolled(): boolean { return useSyncExternalStore(subscribeScroll, () => window.scrollY > 16, () => false); }
