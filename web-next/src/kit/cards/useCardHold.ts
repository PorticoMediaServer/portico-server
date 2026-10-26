import {useCallback, useEffect, useLayoutEffect, useRef, useState, type MouseEvent as ReactMouseEvent, type PointerEvent as ReactPointerEvent} from 'react';
import type {CardSelection} from './types';
import {useRetainedLayoutPaint} from '../layout/retained-paint';

export type CardHold = Readonly<{x: number; y: number; progress: number}>;

/** One committed admission of the hold gesture for an exact selection/disabled/
 * row tuple. Only the admission that is still this hook's committed one may
 * start, move, cancel or finish a gesture. */
type Admission = {readonly selection?: CardSelection; readonly disabled: boolean; readonly row: unknown; /** retired in place */ live: boolean};
type Gesture = Readonly<{admission: Admission; selection: CardSelection}>;

/** The drawn hold gesture, exactly as the card has always run it: it begins for
 * touch and pen (a mouse keeps its hover and click), shows progress, and after
 * 500 ms sends the card's own long-press intent.
 *
 * A fresh admission is minted per selection/disabled/row tuple and retired in
 * the same layout pass, together with any scheduled work it owns. Suppression of
 * the click that follows a hold belongs to the physical row, so it survives the
 * same row's selection change but a retired row's callback cannot consume a
 * successor's suppression. */
export function useCardHold({selection, disabled = false, row}: {selection?: CardSelection; disabled?: boolean; row: unknown}) {
  const retained = useRetainedLayoutPaint(), inactive = disabled || retained;
  const [hold, setHold] = useState<CardHold>();
  const committed = useRef<Admission | null>(null);
  const [admission, setAdmission] = useState<Admission | null>(null);
  const gesture = useRef<Gesture | null>(null);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const animation = useRef<number | undefined>(undefined);
  const origin = useRef<{x: number; y: number} | undefined>(undefined);
  /** The physical row whose next click belongs to a hold that already fired. */
  const suppressed = useRef<unknown | null>(null);
  const stop = useCallback(() => {
    if (timer.current !== undefined) clearTimeout(timer.current);
    if (animation.current !== undefined) cancelAnimationFrame(animation.current);
    timer.current = undefined; animation.current = undefined; origin.current = undefined; gesture.current = null;
    setHold(undefined);
  }, []);
  useLayoutEffect(() => {
    // A hold's suppression never crosses to another physical row; within this
    // row it survives selection/menu publication until the click consumes it.
    if (suppressed.current !== null && suppressed.current !== row) suppressed.current = null;
    const entry: Admission = {selection, disabled: inactive, row, live: true};
    committed.current = entry;
    setAdmission(entry);
    return () => {
      // Retire this exact admission, and cancel the work it scheduled, in place.
      entry.live = false;
      if (gesture.current?.admission === entry) stop();
      if (committed.current === entry) committed.current = null;
    };
  }, [selection, inactive, row, stop]);
  const active = admission && admission.live && committed.current === admission
    && admission.selection === selection && admission.disabled === inactive && admission.row === row ? admission : null;
  const owns = (entry: Admission | null) => !!entry && entry.live && committed.current === entry;
  useLayoutEffect(() => () => { stop(); suppressed.current = null; }, [stop]);
  useEffect(() => {
    const interrupt = () => { stop(); suppressed.current = null; };
    window.addEventListener('blur', interrupt);
    window.addEventListener('scroll', interrupt, true);
    document.addEventListener('visibilitychange', interrupt);
    return () => {
      window.removeEventListener('blur', interrupt);
      window.removeEventListener('scroll', interrupt, true);
      document.removeEventListener('visibilitychange', interrupt);
    };
  }, [stop]);
  /** Consumes the click only for this exact committed admission and its row: a
   * retired handler of the same row cannot spend a successor's suppression. */
  const consumeClick = useCallback(() => {
    // The rendered tuple, not just the admission: new props before layout still
    // hold the old admission state and must not spend a successor's suppression.
    if (!owns(active) || suppressed.current !== row) return false;
    suppressed.current = null;
    return true;
  }, [active, row]);
  const onPointerDown = (event: ReactPointerEvent<HTMLButtonElement>) => {
    const held0 = active;
    if (!owns(held0) || !held0) return;
    if (held0.disabled || !event.isPrimary || event.button !== 0) return;
    // A new admitted press on this row starts fresh, exactly as before.
    suppressed.current = null;
    stop();
    // A mouse keeps the drawn hover and click; the hold belongs to touch and pen.
    if (event.pointerType === 'mouse' || !held0.selection || held0.selection.active) return;
    const capture: Gesture = {admission: held0, selection: held0.selection};
    gesture.current = capture;
    const anchor = event.currentTarget, bounds = anchor.parentElement!.getBoundingClientRect();
    const x = event.clientX - bounds.left, y = event.clientY - bounds.top, started = performance.now();
    origin.current = {x: event.clientX, y: event.clientY};
    const tick = () => {
      if (!owns(capture.admission) || gesture.current !== capture) return;
      setHold({x, y, progress: Math.min(1, (performance.now() - started) / 500)});
      animation.current = requestAnimationFrame(tick);
    };
    tick();
    timer.current = setTimeout(() => {
      if (!owns(capture.admission) || gesture.current !== capture) return;
      const firedRow = capture.admission.row;
      stop();
      suppressed.current = firedRow;
      capture.selection.onToggle({range: false, source: 'long-press', anchor});
    }, 500);
  };
  const onPointerMove = (event: ReactPointerEvent<HTMLButtonElement>) => {
    const held0 = active;
    if (!owns(held0) || !held0) return;
    const start = origin.current;
    if (start && Math.hypot(event.clientX - start.x, event.clientY - start.y) > 10) stop();
  };
  const onCancel = () => { if (!owns(active)) return; stop(); };
  const handlers = active ? {
    onPointerDown, onPointerMove,
    onPointerUp: onCancel,
    onPointerCancel: () => { if (!owns(active)) return; stop(); if (suppressed.current === row) suppressed.current = null; },
    onPointerLeave: onCancel,
    onLostPointerCapture: onCancel,
    onContextMenu: (event: ReactMouseEvent<HTMLButtonElement>) => {
      const held0 = active;
      if (!owns(held0) || !held0) return;
      if (held0.selection && suppressed.current === held0.row) event.preventDefault();
    },
  } : {};
  return {hold, handlers, cancel: onCancel, consumeClick};
}
