import {useLayoutEffect, type RefObject} from 'react';
import {motion, reducedMotion} from '@design';
export function routeFadeTiming(reduce: boolean): {duration: number; easing: string} {
  return {duration: reduce ? reducedMotion.durationMs : motion.standard, easing: 'linear'};
}
/** View Transitions own supported browsers; the browser-floor fallback is an opacity fade. */
export function useRouteFade(target: RefObject<HTMLElement | null>, routeKey: string): void {
  useLayoutEffect(() => {
    if (typeof document.startViewTransition === 'function' || !target.current?.animate) return;
    const reduce = document.documentElement.dataset.reduceMotion === 'true' || matchMedia('(prefers-reduced-motion: reduce)').matches;
    const animation = target.current.animate([{opacity: 0}, {opacity: 1}], routeFadeTiming(reduce));
    return () => animation.cancel();
  }, [target, routeKey]);
}
