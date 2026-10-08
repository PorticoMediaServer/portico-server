import React, {createContext, useCallback, useEffect, useMemo, useRef, useState} from 'react';
import {isTV} from '../theme/layout';

type Gate = {gated: boolean; claim: () => void; release: () => void};
export const FocusGateContext = createContext<Gate | null>(null);

/**
 * Initial focus inside a modal (TV). React Native's preferred-focus request
 * needs the app's root view, which a modal does not have, so tvOS would pick
 * the top-left control instead. When a control marked `hasTVPreferredFocus`
 * appears as the modal opens, it closes the gate: until it takes focus (or a
 * moment passes) it is the only control that can. Modals without such a
 * control are left to tvOS.
 */
export function FocusGate({children}: {children: React.ReactNode}) {
  const [gated, setGated] = useState(false);
  const opened = useRef(Date.now());
  const done = useRef(false);
  const claim = useCallback(() => {
    if (!isTV || done.current || Date.now() - opened.current > 600) return;
    setGated(true);
  }, []);
  const release = useCallback(() => { done.current = true; setGated(false); }, []);
  useEffect(() => {
    if (!gated) return;
    const t = setTimeout(release, 900);
    return () => clearTimeout(t);
  }, [gated, release]);
  const value = useMemo(() => ({gated, claim, release}), [gated, claim, release]);
  return <FocusGateContext.Provider value={value}>{children}</FocusGateContext.Provider>;
}
