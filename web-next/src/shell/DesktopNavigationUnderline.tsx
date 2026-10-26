import {useEffect, useRef, type RefObject} from 'react';
import {easings, motion, reducedMotion} from '@design';
import styles from './shell.module.css';

type Geometry = Readonly<{left: number; width: number; bottom: number}>;

/** One shell-owned underline for the real selected DesktopNavigation destination.
 * Pure decoration: it measures the selected link or More trigger in the owning
 * `<ul>`, never calls routes or reads session/library data. */
export function DesktopNavigationUnderline({list}: {list: RefObject<HTMLUListElement | null>}) {
  const underline = useRef<HTMLSpanElement>(null);
  useEffect(() => {
    const bar = underline.current, destinations = list.current;
    if (!bar || !destinations) return;
    let placed: Geometry | undefined, animation: Animation | undefined, disposed = false;
    const reduced = () => document.documentElement.dataset.reduceMotion === 'true' || matchMedia('(prefers-reduced-motion: reduce)').matches;
    const selected = () => destinations.querySelector<HTMLElement>('[aria-current="page"], button[data-active="true"]');
    const geometry = (): Geometry | undefined => {
      const at = selected();
      if (!at || !at.isConnected || !destinations.isConnected) return undefined;
      const box = destinations.getBoundingClientRect(), target = at.getBoundingClientRect();
      const value = {left: target.left - box.left, width: target.width, bottom: box.bottom - target.bottom};
      return [value.left, value.width, value.bottom].every(Number.isFinite) ? value : undefined;
    };
    const show = (value: Geometry) => {
      bar.style.left = `${value.left}px`; bar.style.width = `${value.width}px`; bar.style.bottom = `${value.bottom}px`; bar.style.opacity = '1';
    };
    const hide = () => {animation?.cancel(); animation = undefined; bar.style.opacity = '0'; placed = undefined;};
    const move = (immediate: boolean) => {
      if (disposed) return;
      const next = geometry();
      if (!next) {hide(); return;}
      const from = placed;
      placed = next;
      animation?.cancel(); animation = undefined;
      if (!from || immediate || !bar.animate) {show(next); return;}
      if (from.left === next.left && from.width === next.width && from.bottom === next.bottom) {show(next); return;}
      if (reduced()) {
        show(next);
        animation = bar.animate([{opacity: 0}, {opacity: 1}], {duration: reducedMotion.durationMs, easing: reducedMotion.easing});
        return;
      }
      animation = bar.animate([{left: `${from.left}px`, width: `${from.width}px`, bottom: `${from.bottom}px`},
        {left: `${next.left}px`, width: `${next.width}px`, bottom: `${next.bottom}px`}], {duration: motion.slow, easing: easings.inOut});
      show(next);
    };
    const resize = new ResizeObserver(() => move(true));
    resize.observe(destinations);
    const mutation = new MutationObserver(() => move(false));
    mutation.observe(destinations, {attributes: true, attributeFilter: ['aria-current', 'data-active'], childList: true, subtree: true});
    const media = matchMedia('(prefers-reduced-motion: reduce)');
    const onMedia = () => move(true);
    media.addEventListener('change', onMedia);
    void document.fonts?.ready.then(() => move(true)).catch(() => {});
    move(true);
    return () => {disposed = true; resize.disconnect(); mutation.disconnect(); media.removeEventListener('change', onMedia); animation?.cancel();};
  }, [list]);
  return <li className={styles.underlineAnchor} aria-hidden="true"><span ref={underline} className={styles.navUnderline} /></li>;
}
