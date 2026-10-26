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
export function applyTitleArtworkClaim(art: HTMLElement | null | undefined, claim: SourceArtClaim | undefined, reduce = reducePreferred()): () => void {
  if (!art || !claim || !claim.name.startsWith(SOURCE_ART_PREFIX) || reduce) return () => {};
  const bounds = art.getBoundingClientRect();
  const current = computedName(art);
  if (current && current !== claim.name) return () => {};
  if (!art.isConnected || !Number.isFinite(bounds.width) || !Number.isFinite(bounds.height) || !(bounds.width > 0) || !(bounds.height > 0) || !realArtwork(art)) return () => {};
  art.style.viewTransitionName = claim.name;
  return () => {if (art.style.viewTransitionName === claim.name) art.style.viewTransitionName = '';};
}

export function useTitleArtworkClaim(art: RefObject<HTMLElement | null>, claim: SourceArtClaim | undefined, titleKey: string, reduce: boolean): void {
  useLayoutEffect(() => {
    const node = art.current;
    if (!node) return;
    const clear = applyTitleArtworkClaim(node, claim, reduce);
    const parent = node.parentNode;
    const replaced = parent ? new MutationObserver(() => {if (!node.isConnected) clear();}) : undefined;
    if (parent && replaced) replaced.observe(parent, {childList: true});
    return () => {replaced?.disconnect(); clear();};
  }, [art, claim?.name, titleKey, reduce]);
}

function painted(element: HTMLElement): Readonly<{opacity: number; transform: string}> {
  const css = typeof getComputedStyle === 'function' ? getComputedStyle(element) : undefined;
  const opacity = css ? Number.parseFloat(css.opacity) : 1;
  return {opacity: Number.isFinite(opacity) ? opacity : 1, transform: css?.transform ?? 'none'};
}

/** One authored entrance per committed title presentation, owned by the real copy
 * node. Replacement and reduce changes retarget from the painted pose before the
 * old animation is cancelled; reduce drops travel and stagger. */
export function useTitleCopyEntrance(copy: RefObject<HTMLElement | null>, titleKey: string, reduce: boolean): void {
  const running = useRef(new Map<HTMLElement, Animation>());
  const latest = useRef<() => void>(() => {});
  latest.current = () => {
    const root = copy.current;
    if (!root) return;
    const timing = titleCopyTiming(reduce);
    const children = Array.from(root.children) as HTMLElement[];
    // Detached or replaced nodes lose their animation; nothing else is touched.
    for (const [element, animation] of running.current) {
      if (!children.includes(element)) {animation.cancel(); running.current.delete(element);}
    }
    const owned = new Set(running.current.keys());
    const frames = children.map(element => ({element, from: owned.has(element) ? painted(element) : {opacity: 0, transform: 'none'}}));
    for (const animation of running.current.values()) animation.cancel();
    running.current.clear();
    for (const [index, frame] of frames.entries()) {
      if (!frame.element.animate) continue;
      const from = timing.risePx > 0
        ? {opacity: frame.from.opacity, transform: frame.from.transform === 'none' ? `translateY(${timing.risePx}px)` : frame.from.transform}
        : {opacity: frame.from.opacity};
      const to = timing.risePx > 0 ? {opacity: 1, transform: 'none'} : {opacity: 1};
      running.current.set(frame.element, frame.element.animate([from, to], {
        duration: timing.duration, delay: timing.risePx > 0 ? timing.staggerMs * index : 0, easing: timing.easing, fill: 'backwards'
      }));
    }
  };
  // One entrance per committed title presentation, plus live reduction retarget.
  useLayoutEffect(() => { latest.current(); }, [copy, titleKey, reduce]);
  useLayoutEffect(() => {
    const root = copy.current;
    // Ordinary metadata adding a child never replays the existing entrance; an
    // actual child replacement (a removal) retargets from the painted pose.
    const replaced = new MutationObserver(records => {if (records.some(record => record.removedNodes.length > 0)) latest.current();});
    if (root) replaced.observe(root, {childList: true});
    return () => {
      replaced.disconnect();
      for (const animation of running.current.values()) animation.cancel();
      running.current.clear();
    };
  }, [copy]);
}
