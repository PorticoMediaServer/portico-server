import {useLayoutEffect, type RefObject} from 'react';
import {easings, motion, reducedMotion} from '@design';

export function routeFadeTiming(reduce: boolean): {duration: number; easing: string} {
  return {duration: reduce ? reducedMotion.durationMs : motion.standard, easing: reduce ? reducedMotion.easing : easings.out};
}

/** View Transitions own supported browsers. The older-browser fallback is the new page's
 * fade and 4 px rise (motion.html 3.1: standard 180, ease-out, as Page.module.css); Reduce
 * motion keeps the fade only. */
export function useRouteFade(target: RefObject<HTMLElement | null>, routeKey: string): void {
  useLayoutEffect(() => {
    if (typeof document.startViewTransition === 'function' || !target.current?.animate) return;
    const reduce = document.documentElement.dataset.reduceMotion === 'true' || matchMedia('(prefers-reduced-motion: reduce)').matches;
    const frames = reduce
      ? [{opacity: 0}, {opacity: 1}]
      : [{opacity: 0, transform: 'translateY(4px)'}, {opacity: 1, transform: 'translateY(0)'}];
    const animation = target.current.animate(frames, routeFadeTiming(reduce));
    return () => animation.cancel();
  }, [target, routeKey]);
}
