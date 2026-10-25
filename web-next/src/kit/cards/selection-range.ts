import {useEffect, useState, type PointerEvent, type FocusEvent, type KeyboardEvent as ReactKeyboardEvent} from 'react';
import type {CardSelection} from './types';

/** Paint intent only; the selection owner checks its actual anchor and members. */
export function useSelectionRangePreview(selection?: CardSelection, disabled = false) {
  const [hover, setHover] = useState(false), paint = selection?.onPreview, active = selection?.active;
  useEffect(() => {
    if (!hover || !active || !paint) return;
    const key = (event: KeyboardEvent) => {if (event.key === 'Shift') paint(event.type === 'keydown');};
    window.addEventListener('keydown', key);window.addEventListener('keyup', key);
    return () => {window.removeEventListener('keydown', key);window.removeEventListener('keyup', key);};
  }, [hover, active, paint]);
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
