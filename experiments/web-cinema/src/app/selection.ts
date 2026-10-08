import {useSyncExternalStore} from 'react';

/** Multi-select in grids: which items are picked, and whether select mode is on. */
type State = {active: boolean; ids: string[]; scope: string | null};
let state: State = {active: false, ids: [], scope: null};
const listeners = new Set<() => void>();
const set = (s: Partial<State>) => { state = {...state, ...s}; listeners.forEach(l => l()); };

export const selection = {
  start: (scope: string, first?: string) => set({active: true, scope, ids: first ? [first] : []}),
  toggle: (id: string) => set({ids: state.ids.includes(id) ? state.ids.filter(x => x !== id) : [...state.ids, id]}),
  setAll: (ids: string[]) => set({ids}),
  clear: () => set({ids: []}),
  end: () => set({active: false, ids: [], scope: null}),
  get: () => state,
};

export function useSelection() {
  return useSyncExternalStore(l => { listeners.add(l); return () => listeners.delete(l); }, () => state);
}
