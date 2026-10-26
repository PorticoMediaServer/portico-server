import {useLayoutEffect, useRef, useState} from 'react';
import {easings, reducedMotion} from '@design';
import {retainLayout, type LayoutRetention, type LayoutRetentionPolicy} from './retention';

/** Bounded shelf/section presence only. Sparse grids never enumerate or retain their collection here. */
export function useLayoutPresence<T>(items: readonly T[], getKey: (item: T) => string, reduced: boolean, exitMs: number, moveMs: number, policy?: LayoutRetentionPolicy) {
  const root = useRef<HTMLDivElement>(null);
  const [state, setState] = useState<LayoutRetention<T>>({live: items, present: items, exits: new Map(), scope: policy?.scope});
  const current = useRef(state), owner = useRef<object | null>(null);
  const previousRects = useRef(new Map<string, DOMRect>()), animations = useRef(new Map<Element, Animation>());
  // A replacement original scope can never render the previous scope's paint.
  const present = state.scope === policy?.scope ? state.present : items;
  const incoming = new Set(items.map(getKey));
  // An owner with a captured before-inert focus context performs that handoff
  // in layout before the withdrawal commit. Its original callbacks separately
  // refuse removed data; no outgoing control remains active for a painted frame.
  const leaving = policy?.beforeLeave ? new Set(state.scope === policy.scope ? state.exits.keys() : [])
    : new Set(present.flatMap(item => incoming.has(getKey(item)) ? [] : [getKey(item)]));
  const max = policy?.maxRetained, stagger = policy?.staggerMs, visibleOnly = policy?.visibleOnly, scope = policy?.scope, beforeLeave = policy?.beforeLeave;
  useLayoutEffect(() => {current.current = state;}, [state]);
  useLayoutEffect(() => {
    const identity = {};owner.current = identity;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const reconcile = () => {
      if (owner.current !== identity) return;
      previousRects.current.clear();
      const visible = new Set<string>();
      if (current.current.scope !== scope) {
        for (const animation of animations.current.values()) animation.cancel();
        animations.current.clear();
      } else for (const element of root.current?.children ?? []) {
        const node = element as HTMLElement, key = node.dataset.layoutItem;
        if (!key) continue;
        const bounds = node.getBoundingClientRect();previousRects.current.set(key, bounds);
        if (visibleOnly) {
          const view = node.ownerDocument.defaultView;
          if (view && bounds.width > 0 && bounds.height > 0 && bounds.right > 0 && bounds.bottom > 0 && bounds.left < view.innerWidth && bounds.top < view.innerHeight) visible.add(key);
        }
      }
      const now = performance.now();
      const next = retainLayout(current.current, items, getKey, now, reduced, exitMs, reducedMotion.durationMs, {maxRetained: max, staggerMs: stagger, visibleOnly, scope}, visible);
      const remaining = new Set(items.map(getKey));
      if (beforeLeave && current.current.scope === scope && current.current.live.some(item => !remaining.has(getKey(item)))) beforeLeave(remaining);
      if (owner.current !== identity) return;
      current.current = next;setState(next);
      if (next.exits.size) timer = setTimeout(reconcile, Math.max(1, Math.min(...[...next.exits.values()].map(exit => exit.retireAt - now))));
    };
    reconcile();
    return () => {if (owner.current === identity) owner.current = null;if (timer !== undefined) clearTimeout(timer);};
  }, [items, getKey, reduced, exitMs, max, stagger, visibleOnly, scope, beforeLeave]);
  useLayoutEffect(() => {
    const attached = new Set(Array.from(root.current?.children ?? []));
    for (const [element, animation] of animations.current) if (!attached.has(element)) {animations.current.delete(element);animation.cancel();}
    for (const element of root.current?.children ?? []) {
      const node = element as HTMLElement, key = node.dataset.layoutItem;
      const previous = animations.current.get(element);
      animations.current.delete(element);
      previous?.cancel();
      if (!key || leaving.has(key) || reduced) continue;
      const before = previousRects.current.get(key), after = element.getBoundingClientRect();
      if (!before || Math.abs(before.left - after.left) < .1 && Math.abs(before.top - after.top) < .1) continue;
      const animation = element.animate([{transform: `translate(${before.left - after.left}px,${before.top - after.top}px)`}, {transform: 'translate(0,0)'}], {duration: moveMs, easing: easings.inOut});
      animations.current.set(element, animation);
      const release = () => { if (animations.current.get(element) === animation) animations.current.delete(element); };
      void animation.finished.then(release, release);
    }
  }, [state, reduced, moveMs]);
  useLayoutEffect(() => () => { for (const animation of animations.current.values()) animation.cancel(); animations.current.clear(); }, []);
  const exits = state.scope === scope ? state.exits : new Map<string, never>();
  return {root, present, leaving, exits, delays: new Map([...exits].map(([key, exit]) => [key, exit.delay]))};
}
