import {useLayoutEffect, useState, type PointerEvent, type FocusEvent, type KeyboardEvent as ReactKeyboardEvent} from 'react';
import type {CardSelection} from './types';
import {useRetainedLayoutPaint} from '../layout/retained-paint';

/** Paint intent only; the selection owner checks its actual anchor and members. */
export function useSelectionRangePreview(selection?: CardSelection, disabled = false) {
  const retained = useRetainedLayoutPaint();disabled = disabled || retained;
  const [hover, setHover] = useState(false), paint = selection?.onPreview, active = selection?.active;
  useLayoutEffect(() => {
    if (!hover || !active || !paint || disabled) return;
    let live = true;
    const key = (event: KeyboardEvent) => {if (live && event.key === 'Shift') paint(event.type === 'keydown');};
    window.addEventListener('keydown', key);window.addEventListener('keyup', key);
    return () => {live = false;window.removeEventListener('keydown', key);window.removeEventListener('keyup', key);};
  }, [hover, active, paint, disabled]);
  const leave = () => {setHover(false);paint?.(false);};
  return {
    onPointerEnter: (event: PointerEvent<HTMLElement>) => {if (!paint) return;setHover(event.pointerType === 'mouse' && !disabled);paint(!!active && event.pointerType === 'mouse' && !disabled && event.shiftKey);},
    onPointerMove: (event: PointerEvent<HTMLElement>) => paint?.(!!active && event.pointerType === 'mouse' && !disabled && event.shiftKey),
    onPointerLeave: leave, onPointerCancel: leave,
    onKeyDown: (event: ReactKeyboardEvent<HTMLElement>) => {if (event.key === 'Shift' && !event.defaultPrevented) paint?.(!!active && !disabled);},
    onKeyUp: (event: ReactKeyboardEvent<HTMLElement>) => {if (event.key === 'Shift') paint?.(false);},
    onBlur: (event: FocusEvent<HTMLElement>) => {if (!event.currentTarget.contains(event.relatedTarget)) paint?.(false);},
  };
}
