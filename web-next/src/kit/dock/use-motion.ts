import {useLayoutEffect, useRef, useState, type RefObject} from 'react';

export function useDockReducedMotion(ref: RefObject<HTMLElement | null>) {
  const [reduced, setReduced] = useState(false);
  useLayoutEffect(() => {
    const media = window.matchMedia('(prefers-reduced-motion: reduce)');
    const update = () => setReduced(media.matches || !!ref.current?.closest('[data-reduce-motion="true"]') || document.documentElement.dataset.reduceMotion === 'true' || document.body.dataset.reduceMotion === 'true');
    const observer = new MutationObserver(update); observer.observe(document.documentElement, {subtree: true, attributes: true, attributeFilter: ['data-reduce-motion']});
    media.addEventListener('change', update); update();
    return () => { observer.disconnect(); media.removeEventListener('change', update); };
  }, [ref]); return reduced;
}

/** Only the identity changes on handover; the owner keeps its video surface mounted. */
export function useDockHandover(ref: RefObject<HTMLElement | null>, identity: string) {
  const reduced = useDockReducedMotion(ref), previous = useRef({identity, reduced}), animation = useRef<Animation | null>(null);
  useLayoutEffect(() => {
    if ((previous.current.identity === identity && previous.current.reduced === reduced) || !ref.current) return;
    previous.current = {identity, reduced};
    const content = ref.current.querySelector<HTMLElement>('[data-dock-identity]'); if (!content) return;
    const current = animation.current?.playState === 'running' ? getComputedStyle(content) : null;
    const opacity = current?.opacity ?? '0', transform = current?.transform ?? 'translateY(6px)';
    animation.current?.cancel();
    animation.current = content.animate(reduced ? [{opacity}, {opacity: 1}] : [{opacity, transform}, {opacity: 1, transform: 'none'}], {duration: reduced ? 160 : 180, easing: 'ease-out'});
  }, [identity, reduced, ref]);
  useLayoutEffect(() => () => animation.current?.cancel(), []);
}
