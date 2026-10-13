import {undoContainerState, type ContainerKind, type ContainerState} from '@core/container-state.ts';

/**
 * A show, season, album or book just marked watched or unwatched from somewhere that has since
 * closed (a card's menu). The mark's own receipt is kept here, for as long as the server will
 * take it back, so Undo can be offered where the person still is. Undo is the server's: it puts
 * every title back as it was, with its place. It is never "mark the other way".
 *
 * One mark at a time: a newer mark replaces the offer for an older one.
 */
type Api = {request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>};
export type WatchedMark = Readonly<{kind: ContainerKind; id: string; title: string; watched: boolean; revision: number; undoToken: string; undoUntil: string; /** Undo is on its way. */ undoing?: boolean; /** Undo was refused: the mark stands. */ failed?: boolean}>;

/** A container's personal state changed outside the screen showing it: `{kind, id}`. Screens holding that state read it again. */
export const CONTAINER_MARKED = 'portico:container-marked';

let current: (WatchedMark & {api: Api}) | undefined;
let shown: WatchedMark | undefined;
const listeners = new Set<() => void>();
const publish = () => { shown = current ? (({api: _api, ...mark}) => mark)(current) : undefined; for (const listener of [...listeners]) listener(); };
const tell = (kind: ContainerKind, id: string) => { if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent(CONTAINER_MARKED, {detail: {kind, id}})); };

export const subscribeWatchedMark = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };
export const currentWatchedMark = (): WatchedMark | undefined => shown;

/** How long the offer still has, in milliseconds. */
export function markRemaining(mark: Pick<WatchedMark, 'undoUntil'> | undefined, now: number): number {
  if (!mark) return 0;
  const until = Date.parse(mark.undoUntil);
  return Number.isFinite(until) ? Math.max(0, until - now) : 0;
}

/** Records a mark the server will take back. A receipt without an Undo offer records nothing. */
export function announceWatchedMark(api: Api, kind: ContainerKind, id: string, title: string, state: ContainerState): void {
  tell(kind, id);
  if (!state.undoToken || !state.undoUntil) { if (current?.kind === kind && current.id === id) { current = undefined; publish(); } return; }
  current = {api, kind, id, title, watched: state.watched, revision: state.revision, undoToken: state.undoToken, undoUntil: state.undoUntil};
  publish();
}

export function dismissWatchedMark(mark?: WatchedMark): void {
  if (!current || (mark && mark.undoToken !== current.undoToken)) return;
  current = undefined;
  publish();
}

/** Takes the mark back. False when the server no longer offers it (too late, or something changed since): the mark stands and the offer goes. */
export async function undoWatchedMark(): Promise<boolean> {
  const mark = current;
  if (!mark || mark.undoing) return false;
  current = {...mark, undoing: true};
  publish();
  try {
    await undoContainerState(mark.api, mark.kind, mark.id, {expectedRevision: mark.revision, undoToken: mark.undoToken});
    if (current?.undoToken === mark.undoToken) { current = undefined; publish(); }
    tell(mark.kind, mark.id);
    return true;
  } catch {
    if (current?.undoToken === mark.undoToken) { current = {...mark, failed: true}; publish(); }
    return false;
  }
}

/** Forgets the offer (the person signed out or switched profile). */
export function resetWatchedMark(): void { current = undefined; publish(); }
