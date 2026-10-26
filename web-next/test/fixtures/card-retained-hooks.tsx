import {useLayoutEffect, useMemo, useRef, useState} from 'react';
import {createRoot} from 'react-dom/client';
import {flushSync} from 'react-dom';
import {RetainedLayoutPaint} from '../../src/kit/layout/retained-paint';
import {useCardHold} from '../../src/kit/cards/useCardHold';
import {useSelectionRangePreview} from '../../src/kit/cards/selection-range';
import type {CardSelection} from '../../src/kit/cards/types';

type Snapshot = {down: () => void; enter: () => void; move: () => void; key: () => void; leave: () => void; consume: () => boolean};
type Harness = {setRetained: (value: boolean) => void; setDisabled: (value: boolean) => void; capture: () => void; old: () => Snapshot; current: () => Snapshot; observations: () => {toggles: number; preview: boolean[]}};
declare global {interface Window {cardRetainedHarness?: Harness;}}

/** Uses real React layout effects and DOM nodes; captured handlers model queued old callbacks. */
export function mountCardRetainedHarness() {
  const node = document.createElement('div'); document.body.append(node); const root = createRoot(node);
  flushSync(() => root.render(<Fixture/>));
  return () => {flushSync(() => root.unmount()); node.remove(); delete window.cardRetainedHarness;};
}
function Fixture() {
  const [retained, setRetained] = useState(false), [disabled, setDisabled] = useState(false);
  const observed = useRef<{toggles: number; preview: boolean[]}>({toggles: 0, preview: []});
  const holdSelection = useMemo<CardSelection>(() => ({active: false, checked: false, onToggle: () => observed.current.toggles++, onExit: () => {}}), []);
  const previewSelection = useMemo<CardSelection>(() => ({active: true, checked: false, onToggle: () => {}, onExit: () => {}, onPreview: value => observed.current.preview.push(value)}), []);
  return <RetainedLayoutPaint.Provider value={retained}><Hooks disabled={disabled} holdSelection={holdSelection} previewSelection={previewSelection} observed={observed} setRetained={setRetained} setDisabled={setDisabled}/></RetainedLayoutPaint.Provider>;
}
function Hooks({disabled, holdSelection, previewSelection, observed, setRetained, setDisabled}: {disabled: boolean; holdSelection: CardSelection; previewSelection: CardSelection; observed: React.RefObject<{toggles: number; preview: boolean[]}>; setRetained: (v: boolean) => void; setDisabled: (v: boolean) => void}) {
  const button = useRef<HTMLButtonElement>(null), old = useRef<Snapshot | undefined>(undefined);
  const hold = useCardHold({selection: holdSelection, disabled, row: 'same-row'}), preview = useSelectionRangePreview(previewSelection, disabled);
  const snapshot = (): Snapshot => {
    const node = button.current!;
    const pointer = {currentTarget: node, pointerType: 'touch', isPrimary: true, button: 0, clientX: 1, clientY: 1} as React.PointerEvent<HTMLButtonElement>;
    const mouse = {currentTarget: node, pointerType: 'mouse', shiftKey: true} as React.PointerEvent<HTMLElement>;
    const key = {key: 'Shift', defaultPrevented: false} as React.KeyboardEvent<HTMLElement>;
    return {down: () => hold.handlers.onPointerDown?.(pointer), enter: () => preview.onPointerEnter(mouse), move: () => preview.onPointerMove(mouse), key: () => preview.onKeyDown(key), leave: () => preview.onPointerLeave(), consume: hold.consumeClick};
  };
  useLayoutEffect(() => {window.cardRetainedHarness = {setRetained: value => flushSync(() => setRetained(value)), setDisabled: value => flushSync(() => setDisabled(value)), capture: () => {old.current = snapshot();}, old: () => old.current!, current: snapshot, observations: () => ({toggles: observed.current!.toggles, preview: [...observed.current!.preview]})};});
  return <div><button ref={button} type="button">Card</button></div>;
}
