import React, {createContext, useCallback, useContext, useMemo, useState} from 'react';
import type {Entry} from '@/data/types';

type Sel = {
  active: boolean;
  selected: Map<string, Entry>;
  start: (e?: Entry) => void;
  toggle: (e: Entry) => void;
  selectAll: (list: Entry[]) => void;
  clear: () => void;
  has: (id: string) => boolean;
};
const Ctx = createContext<Sel | null>(null);

/**
 * Multi-select. Starts from a card's menu (Select) or with Shift-click; while
 * active, clicking a card toggles it and the selection bar offers bulk actions.
 */
export function SelectionProvider({children}: {children: React.ReactNode}) {
  const [active, setActive] = useState(false);
  const [selected, setSelected] = useState<Map<string, Entry>>(new Map());
  const start = useCallback((e?: Entry) => { setActive(true); setSelected(e ? new Map([[e.id, e]]) : new Map()); }, []);
  const toggle = useCallback((e: Entry) => { setActive(true); setSelected(prev => { const n = new Map(prev); if (n.has(e.id)) n.delete(e.id); else n.set(e.id, e); return n; }); }, []);
  const selectAll = useCallback((list: Entry[]) => setSelected(prev => new Map([...prev, ...list.map(e => [e.id, e] as const)])), []);
  const clear = useCallback(() => { setActive(false); setSelected(new Map()); }, []);
  const value = useMemo<Sel>(() => ({active, selected, start, toggle, selectAll, clear, has: id => selected.has(id)}), [active, selected, start, toggle, selectAll, clear]);
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}
export const useSelection = () => useContext(Ctx)!;
