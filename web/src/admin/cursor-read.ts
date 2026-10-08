import {useCallback, useState} from 'react';
import {useRead, type Read} from './console';

/**
 * PERF-S15: a console list read one page at a time by the server's cursor, with Previous and Next,
 * so every item is reachable and the page holds one server page (never the whole list). The cursors
 * of the pages walked are kept (bounded) so Previous goes back without re-walking.
 */
export type CursorRead<T> = Read<T> & {page: number; canPrevious: boolean; canNext: boolean; next: () => void; previous: () => void; first: () => void};

export function useCursorRead<T extends {nextCursor?: string}>(fetch: (cursor: string) => Promise<T>, deps: readonly unknown[]): CursorRead<T> {
  const [stack, setStack] = useState<readonly string[]>(['']);
  const cursor = stack[stack.length - 1] ?? '';
  const read = useRead(() => fetch(cursor), [...deps, cursor]);
  const nextCursor = read.data?.nextCursor ?? '';
  const next = useCallback(() => { if (nextCursor) setStack(s => [...s, nextCursor].slice(-1000)); }, [nextCursor]);
  const previous = useCallback(() => setStack(s => (s.length > 1 ? s.slice(0, -1) : s)), []);
  const first = useCallback(() => setStack(['']), []);
  return {...read, page: stack.length, canPrevious: stack.length > 1, canNext: !!nextCursor, next, previous, first};
}

/** Appends a cursor to a list path (`?limit=…` already present or not). */
export const withCursor = (path: string, cursor: string) => (cursor ? `${path}${path.includes('?') ? '&' : '?'}cursor=${encodeURIComponent(cursor)}` : path);

/** The same over a list whose page is `{items, nextCursor}`: `data` is the page's items. */
export function useCursorList<T>(fetch: (cursor: string) => Promise<{items: T[]; nextCursor: string}>, deps: readonly unknown[]): CursorRead<T[]> {
  const pages = useCursorRead(fetch, deps);
  return {...pages, data: pages.data?.items} as CursorRead<T[]>;
}

/** A server page's `nextCursor`, or '' (a malformed one ends the list rather than looping). */
export const nextCursorOf = (raw: unknown): string => {
  const c = (raw as {nextCursor?: unknown} | null)?.nextCursor;
  return typeof c === 'string' && c.length <= 2048 ? c : '';
};
