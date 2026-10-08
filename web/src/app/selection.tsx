import React, {createContext, useCallback, useContext, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore} from 'react';
import {useLocation} from '@tanstack/react-router';
import type {ContentEntry} from '@core/library-content.ts';
import type {JobSelector} from '@core/bulk-jobs.ts';

/**
 * PERF-S09: the whole set behind the current screen, when the screen can name
 * it without loading it: a browse query (`{libraryId, pivot, filter, sort}`)
 * or a single-library container. `total` is the whole-set size ("Select all
 * N"); `catalogRevision` is the browse page revision (query) or the decimal
 * catalog revision (container) the one bulk job is fenced with.
 * `resolveRevision` reads the current revision with one tiny browse request
 * when the bar submits (lazy, so a screen that never bulk-acts sends nothing
 * extra); containers the screen already knows the revision of omit it. Never
 * an items list, and never built by iterating the library on the client.
 */
export type SelectionTarget = Readonly<{
  selector: Exclude<JobSelector, {items: unknown}>;
  total: number;
  catalogRevision?: string;
  resolveRevision?: () => Promise<string | undefined>;
}>;

/**
 * Multi-select for cards anywhere in the app: library grids, Home rows,
 * Discover, Saved and search. The provider sits in the shell so one bar
 * serves every screen; navigating away clears the selection. Sections
 * register their visible entries so "Select all" and shift-range know the
 * current page, and screens register a refresh so bulk edits show up.
 *
 * Three contexts rather than one: the actions never change identity, the
 * active flag changes only on enter/exit, and the chosen set changes per
 * toggle. A section subscribes to the first two, a card wrapper to the third,
 * so ticking one card does not re-render every section on the screen; and
 * section registration keeps its own version so it never re-renders cards.
 */
type Actions = {
  exit: () => void;
  toggle: (entry: ContentEntry) => void;
  selectMany: (entries: readonly ContentEntry[]) => void;
  refresh: () => void;
  registerSection: (key: string, entries: readonly ContentEntry[]) => () => void;
  registerRefresh: (run: () => void) => () => void;
  /** Choose the visible entries and mean the whole set behind the screen ("Select all N"). */
  selectWholeSet: (entries: readonly ContentEntry[]) => void;
  /** A browse screen names the whole set behind its grid so Select-all bulk-acts in one job. */
  registerQueryTarget: (key: string, target: SelectionTarget) => () => void;
  /** Every selectable entry currently on screen, in section order, read on demand. */
  visibleNow: () => readonly ContentEntry[];
  /** The chosen entries in selection order, read on demand. */
  entriesNow: () => readonly ContentEntry[];
};
type Value = Actions & {
  active: boolean;
  ids: ReadonlySet<string>;
  entries: readonly ContentEntry[];
  visible: readonly ContentEntry[];
  /** The whole set behind the current screen, when a screen named one. */
  target: SelectionTarget | null;
  /** The viewer chose the whole set ("Select all N"); only then may a bulk action use `target`. */
  wholeSet: boolean;
  isSelected: (id: string) => boolean;
};
const ActionsCtx = createContext<Actions | null>(null);
const ActiveCtx = createContext<boolean>(false);
const IdsCtx = createContext<ReadonlySet<string>>(new Set());
/** The whole set behind the current screen, when a screen named one (rarely changes). */
const TargetCtx = createContext<SelectionTarget | null>(null);
/** Whether the viewer chose the whole set ("Select all N"), not only what is visible. */
const WholeSetCtx = createContext<boolean>(false);
/** PERF-28: the chosen set as a store, so a card re-renders only when its own id flips. */
type Ids = ReadonlySet<string>;
interface IdsStore {
  subscribe(fn: () => void): () => void;
  get(): Ids;
  set(ids: Ids): void;
}
function idsStore(): IdsStore {
  let current: Ids = new Set();
  const listeners = new Set<() => void>();
  return {subscribe: fn => { listeners.add(fn); return () => { listeners.delete(fn); }; }, get: () => current, set: ids => { current = ids; for (const fn of [...listeners]) fn(); }};
}
const IdsStoreCtx = createContext<IdsStore>(idsStore());
const SectionsVersionCtx = createContext(0);

/** Every catalog kind carries personal state and metadata, so all cards can be selected. People and categories are not entries. */
export function selectableKind(_kind: ContentEntry['kind']): boolean {
  return true;
}

export function SelectionProvider({children}: {children: React.ReactNode}) {
  const [chosen, setChosen] = useState<Map<string, ContentEntry>>(() => new Map());
  const sections = useRef(new Map<string, readonly ContentEntry[]>());
  const [sectionsVersion, setSectionsVersion] = useState(0);
  const refreshers = useRef(new Set<() => void>());
  const chosenRef = useRef(chosen);
  chosenRef.current = chosen;
  const active = chosen.size > 0;
  // PERF-S09: whether the viewer chose the whole set behind the screen ("Select all N"), as
  // opposed to every item they can see. Only that explicit choice may act on the whole set; any
  // other change to the selection clears it.
  const [wholeSet, setWholeSet] = useState(false);
  const exit = useCallback(() => { setWholeSet(false); setChosen(prev => (prev.size ? new Map() : prev)); }, []);
  // Selection mode is derived from the set: unticking the last item leaves it,
  // so cards get their play and more buttons back without a separate Done tap.
  const toggle = useCallback((entry: ContentEntry) => {
    setWholeSet(false);
    setChosen(prev => {
      const next = new Map(prev);
      if (next.has(entry.id)) next.delete(entry.id);
      else next.set(entry.id, entry);
      return next;
    });
  }, []);
  const selectMany = useCallback((entries: readonly ContentEntry[]) => {
    setWholeSet(false);
    setChosen(prev => {
      const next = new Map(prev);
      for (const e of entries) if (selectableKind(e.kind)) next.set(e.id, e);
      return next;
    });
  }, []);
  /** "Select all N": the visible entries are chosen and the whole set behind the screen is meant. */
  const selectWholeSet = useCallback((entries: readonly ContentEntry[]) => {
    selectMany(entries);
    setWholeSet(true);
  }, [selectMany]);
  const registerSection = useCallback((key: string, entries: readonly ContentEntry[]) => {
    sections.current.set(key, entries);
    setSectionsVersion(v => v + 1);
    return () => { sections.current.delete(key); setSectionsVersion(v => v + 1); };
  }, []);
  const [targetState, setTargetState] = useState<{key: string; target: SelectionTarget} | null>(null);
  const registerQueryTarget = useCallback((key: string, target: SelectionTarget) => {
    setTargetState({key, target});
    return () => { setTargetState(prev => (prev?.key === key ? null : prev)); };
  }, []);
  const registerRefresh = useCallback((run: () => void) => {
    refreshers.current.add(run);
    return () => { refreshers.current.delete(run); };
  }, []);
  const refresh = useCallback(() => { for (const run of refreshers.current) run(); }, []);
  const visibleNow = useCallback(() => {
    const seen = new Set<string>();
    const out: ContentEntry[] = [];
    for (const list of sections.current.values()) for (const e of list) if (selectableKind(e.kind) && !seen.has(e.id)) { seen.add(e.id); out.push(e); }
    return out;
  }, []);
  const entriesNow = useCallback(() => [...chosenRef.current.values()], []);
  const actions = useMemo<Actions>(() => ({exit, toggle, selectMany, selectWholeSet, refresh, registerSection, registerRefresh, registerQueryTarget, visibleNow, entriesNow}), [exit, toggle, selectMany, selectWholeSet, refresh, registerSection, registerRefresh, registerQueryTarget, visibleNow, entriesNow]);
  const ids = useMemo<ReadonlySet<string>>(() => new Set(chosen.keys()), [chosen]);
  const [store] = useState(idsStore);
  useLayoutEffect(() => { store.set(ids); }, [store, ids]);

  /* Leaving the screen ends the selection; it never follows the viewer around. */
  const {pathname} = useLocation();
  useEffect(() => { exit(); setTargetState(null); }, [pathname, exit]);
  // A new or removed whole-set target ends the whole-set choice (the set it named is gone).
  useEffect(() => { setWholeSet(false); }, [targetState]);

  /* Keyboard: Escape leaves selection mode; Cmd/Ctrl+A selects everything visible. */
  useEffect(() => {
    if (!active) return;
    const key = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement | null;
      if (target?.closest('input,textarea,[contenteditable=true],[role="dialog"]')) return;
      if (e.key === 'Escape') { e.preventDefault(); exit(); }
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'a') { e.preventDefault(); selectMany(visibleNow()); }
    };
    document.addEventListener('keydown', key);
    return () => document.removeEventListener('keydown', key);
  }, [active, exit, selectMany, visibleNow]);

  return (
    <ActionsCtx.Provider value={actions}>
      <ActiveCtx.Provider value={active}>
        <SectionsVersionCtx.Provider value={sectionsVersion}>
          <WholeSetCtx.Provider value={wholeSet}><TargetCtx.Provider value={targetState?.target ?? null}><IdsStoreCtx.Provider value={store}><IdsCtx.Provider value={ids}>{children}</IdsCtx.Provider></IdsStoreCtx.Provider></TargetCtx.Provider></WholeSetCtx.Provider>
        </SectionsVersionCtx.Provider>
      </ActiveCtx.Provider>
    </ActionsCtx.Provider>
  );
}

/** Stable intents; null outside the shell (sign-in, restore), so shared components stay inert there. */
export const useSelectionActions = () => useContext(ActionsCtx);
/** Whether selection mode is on; changes only on enter/exit. */
export const useSelectionActive = () => useContext(ActiveCtx);
/** Whether one entry is chosen; re-renders only when this entry flips. */
export function useIsSelected(id: string): boolean {
  const store = useContext(IdsStoreCtx);
  return useSyncExternalStore(store.subscribe, () => store.get().has(id));
}

/**
 * The full picture for the selection bar: chosen entries, everything visible,
 * and the intents. Re-renders on every toggle and section change, which is
 * right for the one component that shows counts.
 */
export function useSelection(): Value | null {
  const actions = useContext(ActionsCtx);
  const active = useContext(ActiveCtx);
  const ids = useContext(IdsCtx);
  const version = useContext(SectionsVersionCtx);
  const target = useContext(TargetCtx);
  const wholeSet = useContext(WholeSetCtx);
  return useMemo(() => {
    if (!actions) return null;
    return {...actions, active, ids, entries: actions.entriesNow(), visible: actions.visibleNow(), target, wholeSet, isSelected: (id: string) => ids.has(id)};
    // `version` is a dependency on purpose: it is how section registration reaches this view.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [actions, active, ids, version, target, wholeSet]);
}

/** A section announces the entries it shows so Select all and shift-range cover them. */
export function useSelectionSection(key: string, entries: readonly ContentEntry[]) {
  const register = useSelectionActions()?.registerSection;
  useEffect(() => register?.(key, entries), [register, key, entries]);
}

/** A browse screen names the whole set behind its grid so Select-all bulk-acts in one job. */
export function useSelectionQueryTarget(key: string, target: SelectionTarget | undefined) {
  const register = useSelectionActions()?.registerQueryTarget;
  useEffect(() => (target ? register?.(key, target) : undefined), [register, key, target]);
}

/** A screen offers its reload so bulk edits are reflected as soon as they apply. */
export function useSelectionRefresh(run: (() => void) | undefined) {
  const register = useSelectionActions()?.registerRefresh;
  useEffect(() => (run ? register?.(run) : undefined), [register, run]);
}
