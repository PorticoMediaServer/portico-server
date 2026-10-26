import {useLayoutEffect, useRef, type RefObject} from 'react';
import {easings, motion, reducedMotion} from '@design';
import {signalDockMotion} from './geometry-event';
import {useDockReducedMotion} from './use-motion';

/**
 * The authored entrance of the desktop collapsed bar (motion.html §3.6): the bar rises
 * 92 px and fades in over slow 320 on the `out` curve while its words follow 300 ms in.
 * Reduced motion is a 160 ms opacity-only paint with no rise or delay.
 *
 * This owns paint only. It starts once per mounted bar, cancels on unmount or on a
 * source/session/mode replacement (which keeps the existing handover paint), and never
 * creates a player timer, media node or loading clock. The bar's expanded/collapsed host
 * lifetime and the single media node stay with their existing owner.
 */
export function useDockEntry(bar: RefObject<HTMLElement | null>, identity: string): void {
  const reduced = useDockReducedMotion(bar);
  const entered = useRef(false), painted = useRef<string | undefined>(undefined);
  const animations = useRef<readonly Animation[]>([]);
  useLayoutEffect(() => {
    const element = bar.current;
    if (!element) return;
    if (painted.current !== undefined && painted.current !== identity) {
      // A new occupant: stop the old entrance paint only; the handover animation owns the words.
      for (const animation of animations.current) animation.cancel();
      animations.current = [];
      painted.current = identity;
      return;
    }
    painted.current = identity;
    if (entered.current) return;
    entered.current = true;
    const words = element.querySelector<HTMLElement>('[data-dock-identity]');
    const duration = reduced ? reducedMotion.durationMs : motion.slow;
    const easing = reduced ? reducedMotion.easing : easings.out;
    const rise = reduced ? 0 : 92;
    const barPaint = element.animate(reduced
      ? [{opacity: 0}, {opacity: 1}]
      : [{transform: `translateY(${rise}px)`, opacity: 0}, {transform: 'none', opacity: 1}], {duration, easing, fill: 'both'});
    const wordsPaint = words?.animate([{opacity: 0}, {opacity: 1}], {duration, delay: reduced ? 0 : 300, easing, fill: 'both'});
    animations.current = wordsPaint ? [barPaint, wordsPaint] : [barPaint];
    signalDockMotion(element, {kind: 'release', offsets: reduced ? [0, 0] : [rise, 0], opacity: [0, 1], duration});
    return () => {for (const animation of animations.current) animation.cancel(); animations.current = [];};
  }, [bar, identity, reduced]);
}
