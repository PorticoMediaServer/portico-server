import {useEffect, useLayoutEffect, useRef, type RefObject} from 'react';
import {easings, motion, reducedMotion} from '@design';
import {useReducedMotion} from '../overlays/shared';

/** The collection's own witness for the answer it is actually showing. */
export type AcceptedAnswer = Readonly<{
  generation: number;
  /** Unknown until a page of the shown generation has arrived. */
  total?: number | undefined;
  /** A newer, non-quiet generation is still loading behind this one. */
  refreshing?: boolean;
  /** The collection's own readiness for the shown generation. */
  status?: string;
}>;

/**
 * An answer is accepted only when the collection reports the shown generation ready: its total is
 * known, no newer answer is loading, and its own status is `ready`. A generation number alone is
 * not enough — `invalidate` bumps it while the new generation's pages are still empty — and a known
 * total is not enough either, since the collection publishes one while other visible pages load.
 */
const accepted = (answer: AcceptedAnswer | undefined): answer is AcceptedAnswer & {total: number} =>
  answer !== undefined && answer.status === 'ready' && answer.total !== undefined && !answer.refreshing;

/** App or system reduction keeps the authored short linear fade, read at the moment it is drawn. */
const reducedNow = (node: HTMLElement, prefersReduced: boolean) => prefersReduced
  || window.matchMedia('(prefers-reduced-motion: reduce)').matches
  || !!node.closest('[data-reduce-motion="true"]')
  || document.documentElement.dataset.reduceMotion === 'true';

/** motion §3.8: the answer fades in at standard 180, and reduction keeps a 160 linear fade. */
const fade = (node: HTMLElement, fromOpacity: number, reduced: boolean) => node.animate([{opacity: fromOpacity}, {opacity: 1}],
  {duration: reduced ? reducedMotion.durationMs : motion.standard, easing: reduced ? easings.linear : easings.out, fill: 'both'});

/**
 * motion 3.8: when a filter or sort answer lands, the grid and its count cross-fade at 180; nothing
 * fades while an answer is still pending, and the first accepted answer a surface paints is already
 * there. Only the last accepted generation is remembered, and it is recorded only once the answer
 * is accepted, so a pending, failed, loading or still-empty generation never consumes the next
 * arrival.
 * `source` is the collection the answer belongs to: replacing it cancels the running fade and
 * starts over, since a new collection's first answer is immediate even if its numbers repeat.
 */
export function useAcceptedArrival(element: RefObject<HTMLElement | null>, answer: AcceptedAnswer | undefined, source?: unknown) {
  const prefersReduced = useReducedMotion(element);
  const shown = useRef<{source: unknown; generation: number} | undefined>(undefined);
  const animation = useRef<Animation | null>(null);
  useLayoutEffect(() => {
    const remembered = shown.current;
    if (remembered === undefined || remembered.source !== source) {
      // A different collection owns this element now: the running arrival is not this answer's.
      animation.current?.cancel(); animation.current = null;
      shown.current = accepted(answer) ? {source, generation: answer.generation} : undefined;
      return;
    }
    if (!accepted(answer) || remembered.generation === answer.generation) return;
    shown.current = {source, generation: answer.generation};
    const node = element.current;
    if (!node) return;
    // An interrupted arrival continues from what is painted instead of restarting from nothing.
    const fromOpacity = animation.current ? Number(getComputedStyle(node).opacity) : 0;
    animation.current?.cancel();
    const current = fade(node, fromOpacity, reducedNow(node, prefersReduced));
    animation.current = current;
    current.onfinish = () => {
      if (animation.current !== current) return;
      node.style.opacity = '1'; current.cancel(); animation.current = null;
    };
  }, [element, answer, source, prefersReduced]);
  useEffect(() => () => {animation.current?.cancel(); animation.current = null;}, []);
}

/**
 * The applied chips appear under the toolbar with the same 180 fade. Each chip keeps its own
 * identity key, so a retained chip never replays and a removed one simply leaves; this only
 * animates the element it owns, and a later reduction change does not replay it.
 */
export function useAppearanceFade(element: RefObject<HTMLElement | null>) {
  const prefersReduced = useReducedMotion(element);
  const played = useRef(false);
  useLayoutEffect(() => {
    const node = element.current;
    if (!node || played.current) return;
    played.current = true;
    const current = fade(node, 0, reducedNow(node, prefersReduced));
    current.onfinish = () => { node.style.opacity = '1'; current.cancel(); };
    return () => current.cancel();
  }, [element, prefersReduced]);
}
