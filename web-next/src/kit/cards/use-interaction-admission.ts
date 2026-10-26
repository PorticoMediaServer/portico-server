import {useLayoutEffect, useMemo, useRef, useState, type RefObject} from 'react';
import {createCardInteractionBinding, type CardInteractionAdmission} from './interaction-admission';
import type {CardSelection} from './types';

/** Commit-bound physical row. Descriptor/callback publication does not renew it.
 * Every commit checks the actual ref node; layout retirement is synchronous. */
export function useCardInteractionAdmission(selection: CardSelection | undefined, root: RefObject<HTMLElement | null>, disabled = false): CardSelection | undefined {
  const source = selection?.interaction;
  const binding = useRef<ReturnType<typeof createCardInteractionBinding> | null>(null);
  if (!binding.current) binding.current = createCardInteractionBinding();
  const published = useRef<CardInteractionAdmission | undefined>(undefined);
  const [admission, setAdmission] = useState<CardInteractionAdmission>();
  useLayoutEffect(() => {
    const next = binding.current!.commit(source, root.current, disabled);
    if (published.current !== next) {published.current = next;setAdmission(next);}
  });
  useLayoutEffect(() => () => {
    published.current = undefined;binding.current!.release();
  }, []);
  useLayoutEffect(() => admission?.onRetire(() => {
    if (published.current === admission) {published.current = undefined;setAdmission(undefined);}
  }), [admission]);
  // Never expose a prior row's receipt beneath a replacement source. The live
  // predicate is used by handlers, not during render (it may retire/publish).
  const bound = !disabled && admission?.live && admission.source === source ? admission : undefined;
  return useMemo(() => !selection || !source ? selection : {...selection, admission: bound, onPreview: bound?.preview}, [selection, source, bound]);
}
