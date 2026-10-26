import {useLayoutEffect, useRef, type RefObject} from 'react';
import {easings, motion, reducedMotion} from '@design';
import {signalDockMotion} from './geometry-event';
import {useDockReducedMotion} from './use-motion';

/** The live preference at this element, read synchronously so the first paint is correct. */
function reducedNow(element: HTMLElement): boolean {
  return window.matchMedia?.('(prefers-reduced-motion: reduce)').matches === true
    || !!element.closest('[data-reduce-motion="true"]')
    || document.documentElement.dataset.reduceMotion === 'true'
    || document.body.dataset.reduceMotion === 'true';
}
/** A completed fill is released into settled inline values so it cannot mask a later paint. */
function settle(element: HTMLElement, animation: Animation, transform: boolean) {
  const apply = () => {if (transform) element.style.transform = 'none'; element.style.opacity = '1'; animation.cancel();};
  void animation.finished.then(apply, () => {});
}

/**
 * The authored entrance of the desktop collapsed bar (motion.html §3.6): the bar rises
 * 92 px and fades in over slow 320 on the `out` curve while its words follow 300 ms in.
 * Reduced motion is a 160 ms linear opacity paint with no rise and no delay.
 *
 * One owner per property: this hook owns the bar's transform/opacity and, during the
 * first entrance only, the words' opacity. After entrance, or when the preference changes
 * mid-paint, the existing handover owns the words and this hook retargets only the bar.
 * Paint only — no player timer, media node or loading clock.
 */
export function useDockEntry(bar: RefObject<HTMLElement | null>, identity: string): void {
  const reduced = useDockReducedMotion(bar);
  const entered = useRef(false), paintedIdentity = useRef<string | undefined>(undefined), wordsOwned = useRef(false), paintedReduced = useRef<boolean | undefined>(undefined);
  const barAnimation = useRef<Animation | null>(null), wordsAnimation = useRef<Animation | null>(null);
  const stop = () => {
    barAnimation.current?.cancel(); barAnimation.current = null;
    wordsAnimation.current?.cancel(); wordsAnimation.current = null; wordsOwned.current = false;
  };
  useLayoutEffect(() => {
    const element = bar.current;
    if (!element) return;
    const live = reducedNow(element);
    // A new occupant: stop the previous paint and settle; the handover owns the words.
    if (paintedIdentity.current !== undefined && paintedIdentity.current !== identity) {
      stop();
      element.style.transform = 'none'; element.style.opacity = '1';
      paintedIdentity.current = identity;
      return;
    }
    paintedIdentity.current = identity;
    if (!entered.current) {
      entered.current = true; paintedReduced.current = live;
      const words = element.querySelector<HTMLElement>('[data-dock-identity]');
      const duration = live ? reducedMotion.durationMs : motion.slow, easing = live ? reducedMotion.easing : easings.out;
      const rise = live ? 0 : 92;
      const bar = element.animate(live
        ? [{opacity: 0}, {opacity: 1}]
        : [{transform: `translateY(${rise}px)`, opacity: 0}, {transform: 'none', opacity: 1}], {duration, easing, fill: 'both'});
      barAnimation.current = bar; settle(element, bar, true);
      if (!live && words) {
        const ink = words.animate([{opacity: 0}, {opacity: 1}], {duration, delay: 300, easing, fill: 'both'});
        wordsAnimation.current = ink; wordsOwned.current = true;
        settle(words, ink, false);
      }
      signalDockMotion(element, {kind: 'release', offsets: live ? [0, 0] : [rise, 0], opacity: [0, 1], duration});
      return;
    }
    // Preference changed after the entrance began: keep the painted opacity, drop travel
    // now, and retarget a 160 ms linear fade without replaying any delay.
    if (wordsOwned.current) {wordsAnimation.current?.cancel(); wordsAnimation.current = null; wordsOwned.current = false;}
    if (barAnimation.current && live !== paintedReduced.current) {
      paintedReduced.current = live;
      const painted = getComputedStyle(element).opacity;
      barAnimation.current.cancel(); barAnimation.current = null;
      element.style.transform = 'none';
      const bar = element.animate([{opacity: painted}, {opacity: 1}], {duration: reducedMotion.durationMs, easing: reducedMotion.easing, fill: 'both'});
      barAnimation.current = bar; settle(element, bar, false);
    }
  }, [bar, identity, reduced]);
  useLayoutEffect(() => () => stop(), []);
}

