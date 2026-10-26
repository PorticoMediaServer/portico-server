import {useLayoutEffect, useMemo, useRef, useState, type PointerEvent, type KeyboardEvent as ReactKeyboardEvent} from 'react';
import {flushSync} from 'react-dom';
import {createRoot} from 'react-dom/client';
import {RetainedLayoutPaint} from '../../src/kit/layout/retained-paint';
import {createCardInteractionOwner, type CardInteractionAdmission} from '../../src/kit/cards/interaction-admission';
import {useCardInteractionAdmission} from '../../src/kit/cards/use-interaction-admission';
import {useCardHold} from '../../src/kit/cards/useCardHold';
import {useSelectionRangePreview} from '../../src/kit/cards/selection-range';
import type {CardSelection} from '../../src/kit/cards/types';

type Actions = {down(): void; move(): void; key(): void; consume(): boolean};
type Harness = {capture(): void; old(): Actions; current(): Actions; retire(): void; revive(): void; retained(value: boolean): void; missing(value: boolean): void; observations(): {toggles: number; preview: boolean[]; publications: number}; receipt(): CardInteractionAdmission | undefined};
declare global {interface Window {cardBoundHooks?: Harness; disposeCardBoundHooks?: () => void;}}

/** Real producer receipt, node binding and volatile React descriptor publication. */
export function mountCardBoundHooks() {
  const host = document.createElement('div'); document.body.append(host); const root = createRoot(host);
  flushSync(() => root.render(<Fixture/>));
  return () => {flushSync(() => root.unmount()); host.remove(); delete window.cardBoundHooks;};
}
function Fixture() {
  const [retained, setRetained] = useState(false);
  return <RetainedLayoutPaint.Provider value={retained}><FixtureBody retained={retained} setRetained={setRetained}/></RetainedLayoutPaint.Provider>;
}
function FixtureBody({retained, setRetained}: {retained: boolean; setRetained: (value: boolean) => void}) {
  const node = useRef<HTMLDivElement>(null), button = useRef<HTMLButtonElement>(null), old = useRef<Actions | undefined>(undefined);
  const [missing, setMissing] = useState(false), [version, setVersion] = useState(0);
  const events = useRef({toggles: 0, preview: [] as boolean[], publications: 0});
  const owner = useMemo(() => createCardInteractionOwner(), []), row = useMemo(() => ({}), []);
  const source = useMemo(() => owner.source(row, {current: () => true, preview: value => {events.current.preview.push(value); events.current.publications++; setVersion(n => n + 1);}}), [owner, row]);
  // New descriptors and callbacks on every React publication, same original source.
  const descriptor: CardSelection = {interaction: source, active: false, checked: false, onPreview: () => {}, onToggle: () => {events.current.toggles++;}, onExit: () => {}};
  const bound = useCardInteractionAdmission(descriptor, node, retained);
  const selection = missing ? {...descriptor, admission: undefined} : bound;
  const hold = useCardHold({selection, disabled: false, row: {version}});
  const preview = useSelectionRangePreview(selection && {...selection, active: true}, false);
  const snapshot = (): Actions => {
    const element = button.current!;
    const pointer = {currentTarget: element, pointerType: 'touch', isPrimary: true, button: 0, clientX: 1, clientY: 1} as PointerEvent<HTMLButtonElement>;
    const mouse = {currentTarget: element, pointerType: 'mouse', shiftKey: true} as PointerEvent<HTMLElement>;
    const key = {key: 'Shift', defaultPrevented: false} as ReactKeyboardEvent<HTMLElement>;
    return {down: () => hold.handlers.onPointerDown?.(pointer), move: () => preview.onPointerMove(mouse), key: () => preview.onKeyDown(key), consume: hold.consumeClick};
  };
  useLayoutEffect(() => {window.cardBoundHooks = {capture: () => {old.current = snapshot();}, old: () => old.current!, current: snapshot,
    retire: () => bound?.admission?.retire(), revive: () => {flushSync(() => setRetained(true)); flushSync(() => setRetained(false));},
    retained: value => flushSync(() => setRetained(value)), missing: value => flushSync(() => setMissing(value)),
    observations: () => ({toggles: events.current.toggles, preview: [...events.current.preview], publications: events.current.publications}), receipt: () => bound?.admission};});
  return <div ref={node}><button ref={button} type="button">Card</button></div>;
}
