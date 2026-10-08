/**
 * Not interested (Recommendations P5), from a recommendation card's actions menu. The title leaves
 * every recommendation and similar titles rank a little lower; client-core `setNotInterested`
 * sends it on the personal batch route, and Undo sends `false`.
 *
 * Like Remove from Continue Watching (./continue-watching.ts) it is optimistic: the menu updates
 * one viewer-scoped store, recommendation rows hide the card at once, and the shell offers one
 * Undo. A refused request puts the card back with the error.
 */
import {notInterestedTarget, setNotInterested} from '@core/recommendation-feedback.ts';

type Api = {request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>};

export const RECOMMENDATIONS_RESET = 'portico:recommendations-reset';
export type NotInterestedNotice = {key: string; title: string; operation: string; phase: 'pending' | 'done' | 'failed'; error?: 'mark' | 'undo'};
type Target = {operation: string; hidden: boolean; mark?: Promise<(() => Promise<void>) | undefined>};
const targets = new Map<string, Map<string, Target>>();
const notices = new Map<string, NotInterestedNotice>();
const listeners = new Set<() => void>();
let version = 0;
const emit = () => { version++; for (const listener of listeners) listener(); };
export const subscribeNotInterested = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };
export const notInterestedVersion = () => version;
export const recommendationViewer = (scope: {viewerId: string; serverId: string}) => JSON.stringify([scope.serverId, scope.viewerId]);
export const hiddenNotInterested = (viewer: string): ReadonlySet<string> => new Set([...targets.get(viewer) ?? []].filter(([, value]) => value.hidden).map(([key]) => key));
export const currentNotInterestedNotice = (viewer: string) => notices.get(viewer);
const current = (viewer: string, key: string, operation: string) => targets.get(viewer)?.get(key)?.operation === operation;
const announce = (viewer: string, notice: NotInterestedNotice) => { notices.set(viewer, notice); emit(); };

const operationId = () => crypto.randomUUID();

/** Marks the card's title Not interested; the returned undo takes that back. */
export async function markNotInterested(api: Api, serverId: string, entry: {id: string; kind?: string; navigation?: {entityId?: string}}): Promise<{undo: () => Promise<void>}> {
  const itemId = notInterestedTarget(entry);
  await setNotInterested(api, {itemId, notInterested: true, serverId, operationId: operationId()});
  return {undo: async () => { await setNotInterested(api, {itemId, notInterested: false, serverId, operationId: operationId()}); }};
}

/** One ordered local decision per viewer and target. Undo waits for its mark before writing false. */
export function startNotInterested(api: Api, viewer: string, serverId: string, entry: {id: string; title: string; kind?: string; navigation?: {entityId?: string}}): string {
  const key = notInterestedTarget(entry), operation = operationId();
  const items = targets.get(viewer) ?? new Map<string, Target>();
  targets.set(viewer, items);
  const target: Target = {operation, hidden: true};
  items.set(key, target);
  announce(viewer, {key, title: entry.title, operation, phase: 'pending'});
  target.mark = markNotInterested(api, serverId, entry).then(done => {
    if (current(viewer, key, operation)) announce(viewer, {key, title: entry.title, operation, phase: 'done'});
    return done.undo;
  }, () => {
    if (!current(viewer, key, operation)) return;
    target.hidden = false;
    announce(viewer, {key, title: entry.title, operation, phase: 'failed', error: 'mark'});
    return undefined;
  });
  return operation;
}

export async function undoNotInterested(viewer: string, notice: NotInterestedNotice): Promise<void> {
  const target = targets.get(viewer)?.get(notice.key);
  if (!target || target.operation !== notice.operation || !target.hidden) return;
  const operation = operationId();
  target.operation = operation;
  target.hidden = false;
  notices.delete(viewer);
  emit();
  const undo = await target.mark;
  if (!current(viewer, notice.key, operation)) return;
  if (!undo) return;
  try {
    await undo();
  } catch {
    if (!current(viewer, notice.key, operation)) return;
    target.hidden = true;
    announce(viewer, {...notice, operation, phase: 'failed', error: 'undo'});
  }
}

export function dismissNotInterested(viewer: string, operation: string) {
  if (notices.get(viewer)?.operation === operation) { notices.delete(viewer); emit(); }
}

/** A successful reset clears only this viewer and tells mounted recommendation readers to reload. */
export function clearNotInterested(viewer: string) {
  targets.delete(viewer);
  notices.delete(viewer);
  emit();
  if (typeof window !== 'undefined') window.dispatchEvent(new CustomEvent(RECOMMENDATIONS_RESET, {detail: {viewer}}));
}

/** A row's entries without the hidden cards (the same row when none of them is). */
export function withoutHidden<R extends {entries: readonly {id: string; kind?: string; navigation?: {entityId?: string}}[]}>(row: R, hidden: ReadonlySet<string>): R {
  if (!hidden.size || !row.entries.some(e => hidden.has(notInterestedTarget(e)))) return row;
  return {...row, entries: row.entries.filter(e => !hidden.has(notInterestedTarget(e)))};
}
