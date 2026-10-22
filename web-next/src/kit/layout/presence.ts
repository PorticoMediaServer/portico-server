import {useLayoutEffect, useRef, useState} from 'react';
import {easings, reducedMotion} from '@design';

/** Bounded shelf/section presence only. Sparse grids never enumerate or retain their collection here. */
export function useLayoutPresence<T>(items: readonly T[], getKey: (item: T) => string, reduced: boolean, exitMs: number, moveMs: number) {
  const root = useRef<HTMLDivElement>(null), [present, setPresent] = useState(items), [leaving, setLeaving] = useState<ReadonlySet<string>>(new Set());
  const current = useRef(present), latest = useRef(items), keyOf = useRef(getKey), deadlines = useRef(new Map<string, number>());
  current.current = present; latest.current = items; keyOf.current = getKey;
  const previousRects = useRef(new Map<string, DOMRect>()), animations = useRef(new Map<Element, Animation>());
  const capture = () => {
    previousRects.current.clear();
    for (const element of root.current?.children ?? []) {
      const key = (element as HTMLElement).dataset.layoutItem;
      if (key) previousRects.current.set(key, element.getBoundingClientRect());
    }
  };
  useLayoutEffect(() => {
    let timer: ReturnType<typeof setTimeout> | undefined;
    const reconcile = () => {
      capture();
      const now = performance.now(), incoming = latest.current, ids = new Set(incoming.map(keyOf.current)), previous = current.current;
      for (const key of deadlines.current.keys()) if (ids.has(key)) deadlines.current.delete(key);
      const retained = previous.filter(item => {
        const key = keyOf.current(item); if (ids.has(key)) return false;
        if (!deadlines.current.has(key)) deadlines.current.set(key, now + (reduced ? reducedMotion.durationMs : exitMs));
        return deadlines.current.get(key)! > now;
      });
      const merged = [...incoming];
      for (const item of retained) merged.splice(Math.min(previous.indexOf(item), merged.length), 0, item);
      setLeaving(new Set(retained.map(keyOf.current))); setPresent(merged);
      if (retained.length) timer = setTimeout(reconcile, Math.max(1, Math.min(...retained.map(item => deadlines.current.get(keyOf.current(item))! - now))));
      else deadlines.current.clear();
    };
    reconcile();
    return () => { if (timer !== undefined) clearTimeout(timer); };
  }, [items, getKey, reduced, exitMs]);
  useLayoutEffect(() => {
    for (const element of root.current?.children ?? []) {
      const node = element as HTMLElement, key = node.dataset.layoutItem;
      animations.current.get(element)?.cancel();
      if (!key || leaving.has(key) || reduced) continue;
      const before = previousRects.current.get(key), after = element.getBoundingClientRect();
      if (!before || Math.abs(before.left - after.left) < .1 && Math.abs(before.top - after.top) < .1) continue;
      const animation = element.animate([{transform: `translate(${before.left - after.left}px,${before.top - after.top}px)`}, {transform: 'translate(0,0)'}], {duration: moveMs, easing: easings.inOut});
      animations.current.set(element, animation);
      void animation.finished.then(() => { if (animations.current.get(element) === animation) animations.current.delete(element); }, () => {});
    }
  }, [present, leaving, reduced, moveMs]);
  useLayoutEffect(() => () => { for (const animation of animations.current.values()) animation.cancel(); animations.current.clear(); }, []);
  return {root, present, leaving};
}
