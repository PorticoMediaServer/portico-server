import {useLayoutEffect, useRef, useState, type PointerEvent, type FocusEvent, type KeyboardEvent as ReactKeyboardEvent} from 'react';
import type {CardSelection} from './types';
import {useRetainedLayoutPaint} from '../layout/retained-paint';

type Admission = {readonly selection?: CardSelection; readonly disabled: boolean; live: boolean; preview: boolean};

/** Paint intent only; the selection owner checks its actual anchor and members. */
export function useSelectionRangePreview(selection?: CardSelection, disabled = false) {
  const retained = useRetainedLayoutPaint(), inactive = disabled || retained;
  const [hover, setHover] = useState(false), [admission, setAdmission] = useState<Admission | null>(null);
  const committed = useRef<Admission | null>(null);
  useLayoutEffect(() => {
    const entry: Admission = {selection, disabled: inactive, live: true, preview: false};
    committed.current = entry; setAdmission(entry);
    if (inactive) setHover(false);
    return () => {
      entry.live = false;
      if (committed.current === entry) {
        committed.current = null;
        if (entry.preview) entry.selection?.onPreview?.(false);
      }
    };
  }, [selection, inactive]);
  const current = admission && admission.live && committed.current === admission && admission.selection === selection
    && admission.disabled === inactive && !inactive ? admission : null;
  const owns = (entry: Admission | null) => !!entry && entry.live && committed.current === entry && !entry.disabled;
  const paint = (entry: Admission | null, value: boolean) => {
    if (!owns(entry) || !entry) return;
    entry.preview = value; entry.selection?.onPreview?.(value);
  };
  useLayoutEffect(() => {
    const entry = current;
    if (!entry || !hover || !entry.selection?.active || !entry.selection.onPreview) return;
    const key = (event: KeyboardEvent) => {if (owns(entry) && event.key === 'Shift') paint(entry, event.type === 'keydown');};
    window.addEventListener('keydown', key); window.addEventListener('keyup', key);
    return () => {window.removeEventListener('keydown', key); window.removeEventListener('keyup', key);};
  }, [current, hover]);
  const leave = () => {if (!owns(current)) return; setHover(false); paint(current, false);};
  return {
    onPointerEnter: (event: PointerEvent<HTMLElement>) => {if (!owns(current) || !current?.selection?.onPreview) return; setHover(event.pointerType === 'mouse'); paint(current, !!current.selection.active && event.pointerType === 'mouse' && event.shiftKey);},
    onPointerMove: (event: PointerEvent<HTMLElement>) => {if (!owns(current)) return; if (event.pointerType === 'mouse') setHover(true); paint(current, !!current?.selection?.active && event.pointerType === 'mouse' && event.shiftKey);},
    onPointerLeave: leave, onPointerCancel: leave,
    onKeyDown: (event: ReactKeyboardEvent<HTMLElement>) => {if (event.key === 'Shift' && !event.defaultPrevented) paint(current, !!current?.selection?.active);},
    onKeyUp: (event: ReactKeyboardEvent<HTMLElement>) => {if (event.key === 'Shift') paint(current, false);},
    onBlur: (event: FocusEvent<HTMLElement>) => {if (!event.currentTarget.contains(event.relatedTarget)) paint(current, false);},
  };
}
