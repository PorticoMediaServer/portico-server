import {useLayoutEffect, useRef, type RefObject} from 'react';
import {easings, motion, reducedMotion} from '@design';
import {SOURCE_ART_PREFIX, type SourceArtClaim} from '../../kit/cards/source-art-transition';

/** The destination half of motion 3.3. The one claim (and its exact minted name)
 * comes from the original navigation owner; this module never mints a name or
 * keeps a render-time registry of its own. */
export const TITLE_ARTWORK_MORPH_MS = 420;
export const TITLE_COPY_RISE_PX = 10;
export const TITLE_COPY_GROUPS_MS = 60;

export function titleArtworkMorph(reduce: boolean): Readonly<{duration: number; easing: string}> {
  return reduce ? {duration: reducedMotion.durationMs, easing: reducedMotion.easing} : {duration: TITLE_ARTWORK_MORPH_MS, easing: easings.out};
}

/** Title words rise 10 px over standard 180, one 60 ms group after another; reduce
 * is the 160 ms opacity policy with no rise and no stagger. */
export function titleCopyTiming(reduce: boolean): Readonly<{risePx: number; duration: number; staggerMs: number; easing: string}> {
  return reduce
    ? {risePx: 0, duration: reducedMotion.durationMs, staggerMs: 0, easing: reducedMotion.easing}
    : {risePx: TITLE_COPY_RISE_PX, duration: motion.standard, staggerMs: TITLE_COPY_GROUPS_MS, easing: easings.out};
}

function reducePreferred(): boolean {
  if (typeof document === 'undefined') return true;
  return document.documentElement?.dataset.reduceMotion === 'true' ||
    document.body?.dataset.reduceMotion === 'true' ||
    (typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches);
}

/** A wrapper box is not proof of artwork: the destination must carry a current,
 * decoded image, be attached and laid out, and carry no other writer's name. */
function realArtwork(art: HTMLElement): boolean {
  const image = art.querySelector('img');
  if (!image) return false;
  const source = image.currentSrc || image.getAttribute('src') || '';
  return source.length > 0 && image.complete && image.naturalWidth > 0;
}

function computedName(art: HTMLElement): string {
  if (typeof getComputedStyle !== 'function') return '';
  const value = getComputedStyle(art).viewTransitionName;
  return value === 'none' ? '' : value;
}

/** Applies the owner's claim to the real destination artwork once. Another
 * writer's computed name is refused, and reduce keeps no geometry name. */
export function applyTitleArtworkClaim(art: HTMLElement | null | undefined, claim: SourceArtClaim | undefined): () => void {
  if (!art || !claim || !claim.name.startsWith(SOURCE_ART_PREFIX) || reducePreferred()) return () => {};
  const bounds = art.getBoundingClientRect();
  const current = computedName(art);
  if (current && current !== claim.name) return () => {};
  if (!art.isConnected || !(bounds.width > 0) || !(bounds.height > 0) || !realArtwork(art)) return () => {};
  art.style.viewTransitionName = claim.name;
  return () => {if (art.style.viewTransitionName === claim.name) art.style.viewTransitionName = '';};
}

export function useTitleArtworkClaim(art: RefObject<HTMLElement | null>, claim: SourceArtClaim | undefined, titleKey: string): void {
  useLayoutEffect(() => applyTitleArtworkClaim(art.current, claim), [art, claim?.name, titleKey]);
}

function painted(element: HTMLElement): Readonly<{opacity: number; transform: string}> {
  const css = typeof getComputedStyle === 'function' ? getComputedStyle(element) : undefined;
  const opacity = css ? Number.parseFloat(css.opacity) : 1;
  return {opacity: Number.isFinite(opacity) ? opacity : 1, transform: css?.transform ?? 'none'};
}

/** One authored entrance per committed title presentation, owned by the real copy
 * node. Replacement and reduce changes retarget from the painted pose before the
 * old animation is cancelled; reduce drops travel and stagger. */
export function useTitleCopyEntrance(copy: RefObject<HTMLElement | null>, titleKey: string): void {
  const running = useRef<Animation[]>([]);
  const latest = useRef<() => void>(() => {});
  latest.current = () => {
    const root = copy.current;
    if (!root) return;
    const timing = titleCopyTiming(reducePreferred());
    const frames = Array.from(root.children, child => {
      const element = child as HTMLElement;
      return {element, ...painted(element)};
    });
    for (const animation of running.current) animation.cancel();
    running.current = [];
    for (const [index, frame] of frames.entries()) {
      if (!frame.element.animate) continue;
      const from = timing.risePx > 0
        ? {opacity: frame.opacity, transform: frame.transform === 'none' ? `translateY(${timing.risePx}px)` : frame.transform}
        : {opacity: frame.opacity};
      const to = timing.risePx > 0 ? {opacity: 1, transform: 'none'} : {opacity: 1};
      running.current.push(frame.element.animate([from, to], {
        duration: timing.duration, delay: timing.risePx > 0 ? timing.staggerMs * index : 0, easing: timing.easing, fill: 'backwards'
      }));
    }
  };
  useLayoutEffect(() => { latest.current(); }, [copy, titleKey]);
  useLayoutEffect(() => {
    const root = copy.current;
    const replaced = new MutationObserver(() => latest.current());
    if (root) replaced.observe(root, {childList: true});
    const reduced = typeof matchMedia === 'function' ? matchMedia('(prefers-reduced-motion: reduce)') : undefined;
    const onReduce = () => latest.current();
    reduced?.addEventListener('change', onReduce);
    const attributes = new MutationObserver(onReduce);
    for (const element of [document.documentElement, document.body]) {
      if (element) attributes.observe(element, {attributes: true, attributeFilter: ['data-reduce-motion']});
    }
    return () => {
      replaced.disconnect(); attributes.disconnect(); reduced?.removeEventListener('change', onReduce);
      for (const animation of running.current) animation.cancel();
      running.current = [];
    };
  }, [copy]);
}
