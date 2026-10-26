import {useLayoutEffect, type RefObject} from 'react';
import {easings, motion, reducedMotion} from '@design';

/** The destination half of motion 3.3. The claim name itself is supplied by the
 * original navigation owner (one shared namespace); this module never invents or
 * owns a second global view-transition name. */
export type TitleArtworkClaim = Readonly<{name: string}>;

export const TITLE_ARTWORK_MORPH_MS = 420;
export const TITLE_COPY_RISE_PX = 10;
export const TITLE_COPY_GROUPS_MS = 60;

export type TitleArtworkFacts = Readonly<{attached: boolean; width: number; height: number; reduce: boolean; claimed: boolean}>;

/** Real attached laid-out artwork, motion allowed, and a supplied name. */
export function titleArtworkEligible(facts: TitleArtworkFacts): boolean {
  return facts.attached && Number.isFinite(facts.width) && Number.isFinite(facts.height) &&
    facts.width > 0 && facts.height > 0 && !facts.reduce && facts.claimed;
}

/** Poster→backdrop runs 420 out; reduce is 160 opacity only. */
export function titleArtworkMorph(reduce: boolean): Readonly<{duration: number; easing: string}> {
  return reduce ? {duration: reducedMotion.durationMs, easing: reducedMotion.easing} : {duration: TITLE_ARTWORK_MORPH_MS, easing: easings.out};
}

/** Title words rise 10 px over 180, one 60 ms group after another; reduce is 160 opacity only. */
export function titleCopyTiming(reduce: boolean): Readonly<{risePx: number; duration: number; staggerMs: number; easing: string}> {
  return reduce
    ? {risePx: 0, duration: reducedMotion.durationMs, staggerMs: 0, easing: reducedMotion.easing}
    : {risePx: TITLE_COPY_RISE_PX, duration: motion.standard, staggerMs: TITLE_COPY_GROUPS_MS, easing: easings.out};
}

function safeName(name: string): boolean {
  return name.length > 0 && name.length <= 128 && /^[A-Za-z0-9_-]+$/.test(name);
}

function reducePreferred(): boolean {
  if (typeof document === 'undefined') return true;
  return document.documentElement.dataset.reduceMotion === 'true' ||
    (typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches);
}

/** Applies the supplied claim to the real destination artwork once and returns a
 * cleanup that clears only this exact owned name. A name in use by another writer
 * is refused; reduce keeps no geometry name. */
export function applyTitleArtworkClaim(art: HTMLElement | null | undefined, claim: TitleArtworkClaim | undefined): () => void {
  if (!art || !claim || !safeName(claim.name) || reducePreferred()) return () => {};
  const bounds = art.getBoundingClientRect();
  const prior = art.style.viewTransitionName;
  if (!titleArtworkEligible({attached: art.isConnected, width: bounds.width, height: bounds.height, reduce: false, claimed: prev(prior, claim.name)})) return () => {};
  art.style.viewTransitionName = claim.name;
  return () => {if (art.style.viewTransitionName === claim.name) art.style.viewTransitionName = '';};
}

function prev(current: string, name: string): boolean {
  return current === '' || current === name;
}

let offered: TitleArtworkClaim | undefined;

/** The navigation owner offers the claim once, for the destination it is about to draw. */
export function offerTitleArtworkClaim(claim: TitleArtworkClaim | undefined): void {
  offered = claim && safeName(claim.name) ? Object.freeze({name: claim.name}) : undefined;
}

/** The destination takes the offered claim once; later titles never reuse it. */
export function takeTitleArtworkClaim(): TitleArtworkClaim | undefined {
  const claim = offered;
  offered = undefined;
  return claim;
}

export function useTitleArtworkClaim(art: RefObject<HTMLElement | null>, claim: TitleArtworkClaim | undefined, titleKey: string): void {
  useLayoutEffect(() => applyTitleArtworkClaim(art.current, claim), [art, claim?.name, titleKey]);
}

/** One authored entrance per committed title presentation, not per metadata update. */
export function useTitleCopyEntrance(copy: RefObject<HTMLElement | null>, titleKey: string): void {
  useLayoutEffect(() => {
    const root = copy.current;
    if (!root) return;
    const timing = titleCopyTiming(reducePreferred());
    const animations: Animation[] = [];
    Array.from(root.children).forEach((child, index) => {
      const node = child as HTMLElement;
      if (!node.animate) return;
      const frames = timing.risePx > 0
        ? [{opacity: 0, transform: `translateY(${timing.risePx}px)`}, {opacity: 1, transform: 'none'}]
        : [{opacity: 0}, {opacity: 1}];
      animations.push(node.animate(frames, {duration: timing.duration, delay: timing.staggerMs * index, easing: timing.easing, fill: 'backwards'}));
    });
    return () => {for (const animation of animations) animation.cancel();};
  }, [copy, titleKey]);
}
