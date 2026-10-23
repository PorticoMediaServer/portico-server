import {useEffect, useRef, useState, type PointerEvent} from 'react';
import {createTouchHold, type TouchHoldPhase} from './touch-hold';

/** No width test: Q6 assigns the same gesture to every touch window. */
export function useTouchHold(onFire: (anchor: HTMLElement) => void, enabled: boolean) {
  const callback = useRef(onFire), allowed = useRef(enabled), anchor = useRef<HTMLElement | null>(null);
  callback.current = onFire; allowed.current = enabled;
  const [phase, setPhase] = useState<TouchHoldPhase>('idle');
  const [hold] = useState(() => createTouchHold(() => {if (allowed.current && anchor.current) callback.current(anchor.current);}, setPhase, {
    schedule: (run, delay) => setTimeout(run, delay), cancel: ticket => clearTimeout(ticket as ReturnType<typeof setTimeout>),
  }));
  useEffect(() => {
    const interrupt = () => hold.cancel();
    window.addEventListener('blur', interrupt); window.addEventListener('scroll', interrupt, true);
    document.addEventListener('visibilitychange', interrupt);
    return () => {hold.dispose(); window.removeEventListener('blur', interrupt); window.removeEventListener('scroll', interrupt, true); document.removeEventListener('visibilitychange', interrupt);};
  }, [hold]);
  useEffect(() => {if (!enabled) hold.cancel();}, [enabled, hold]);
  return {phase, consumeClick: hold.consumeClick, handlers: {
    onPointerDown(event: PointerEvent<HTMLElement>) {
      if (!event.isPrimary) {hold.cancel(); return;}
      if (!enabled || event.button !== 0 || event.pointerType === 'mouse') return;
      anchor.current = event.currentTarget; hold.start({x: event.clientX, y: event.clientY});
    },
    onPointerMove: (event: PointerEvent<HTMLElement>) => hold.move({x: event.clientX, y: event.clientY}),
    onPointerUp: () => hold.end(), onPointerCancel: () => hold.cancel(), onPointerLeave: () => hold.cancel(), onLostPointerCapture: () => hold.cancel(),
  }};
}
