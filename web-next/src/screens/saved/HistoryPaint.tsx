import {useLayoutEffect, useRef, type ReactNode} from 'react';
import type {LayoutExit} from '../../kit/layout/retention';

/** Own only opacity; the outer roster owns gap transforms. Each admitted exit
 * keeps its delay/deadline when another cohort is removed. */
export function HistoryPaint({exit, reduced, scope, children}: {exit?: LayoutExit<unknown>; reduced: boolean; scope: object; children: ReactNode}) {
  const node = useRef<HTMLDivElement>(null);
  const owned = useRef<{node: HTMLDivElement; animation: Animation; scope: object} | undefined>(undefined);
  useLayoutEffect(() => {
    const element = node.current, old = owned.current;
    if (!element) return;
    // Read partial paint before cancelling. A new original scope never inherits
    // the old scope's pose or lets its settled completion touch this node.
    const opacity = old?.scope === scope ? Number(getComputedStyle(element).opacity) : 1;
    old?.animation.cancel();owned.current = undefined;
    if (!exit && (!old || old.scope !== scope)) {element.style.opacity = '';return;}
    const now = performance.now(), delay = exit && !reduced ? Math.max(0, exit.admittedAt + exit.delay - now) : 0;
    const duration = exit ? Math.max(0, Math.min(exit.retireAt - now - delay, reduced ? 160 : Infinity)) : reduced ? 160 : 180;
    if (!duration) {element.style.opacity = exit ? '0' : '';return;}
    const animation = element.animate([{opacity}, {opacity: exit ? 0 : 1}], {delay, duration, easing: 'linear', fill: 'both'});
    const job = {node: element, animation, scope};owned.current = job;
    const finish = () => {
      if (owned.current !== job) return;
      element.style.opacity = exit ? '0' : '';owned.current = undefined;animation.cancel();
    };
    void animation.finished.then(finish, () => {if (owned.current === job) owned.current = undefined;});
    // Replacement reads the painted opacity before cancellation in its setup.
  }, [exit?.token, exit?.reduced, reduced, scope]);
  useLayoutEffect(() => () => {const job = owned.current;owned.current = undefined;job?.animation.cancel();}, []);
  return <div ref={node}>{children}</div>;
}
