import {useCallback, useEffect, useRef, useState} from 'react';
import {useDataVersion, useServices} from '../../data/hooks';
import type {BrowseQuery} from '../../data/services';
import type {Item} from '../../data/types';

const PAGE = 60;

/**
 * Paged access to a browse result of any size: only the pages that come into view are fetched, so
 * the client's cost follows what's on screen, not the size of the library.
 */
export function useBrowse(query: Omit<BrowseQuery, 'cursor' | 'pageSize'>) {
  const services = useServices();
  const version = useDataVersion();
  const key = JSON.stringify(query);
  const [state, setState] = useState<{key: string; total: number | null; pages: Map<number, Item[]>; error: Error | null}>({key, total: null, pages: new Map(), error: null});
  const inflight = useRef(new Set<number>());
  const current = state.key === key ? state : {key, total: null, pages: new Map<number, Item[]>(), error: null};

  const load = useCallback((page: number) => {
    if (inflight.current.has(page)) return;
    inflight.current.add(page);
    services.catalog.browse({...query, cursor: String(page * PAGE), pageSize: PAGE}).then(
      res => setState(s => {
        inflight.current.delete(page);
        const base = s.key === key ? s : {key, total: null, pages: new Map<number, Item[]>(), error: null};
        const pages = new Map(base.pages);
        pages.set(page, res.items);
        return {...base, total: res.total, pages};
      }),
      (error: Error) => { inflight.current.delete(page); setState(s => ({...s, error})); },
    );
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, services]);

  // First page on every query change, and a quiet refresh of loaded pages after mutations.
  useEffect(() => {
    inflight.current.clear();
    load(0);
  }, [key, load]);
  useEffect(() => {
    if (current.total === null) return;
    for (const p of current.pages.keys()) { inflight.current.delete(p); load(p); }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [version]);

  const ensure = useCallback((from: number, to: number) => {
    for (let p = Math.floor(from / PAGE); p <= Math.floor(Math.max(from, to) / PAGE); p++) {
      if (!current.pages.has(p)) load(p);
    }
  }, [current.pages, load]);
  const at = useCallback((i: number): Item | undefined => current.pages.get(Math.floor(i / PAGE))?.[i % PAGE], [current.pages]);
  const loaded = [...current.pages.values()].flat();
  return {total: current.total, at, ensure, error: current.error, loaded, reload: () => load(0)};
}
