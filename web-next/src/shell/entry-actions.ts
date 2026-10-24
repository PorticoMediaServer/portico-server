import type {ContentEntryKind} from '@core/library-content.ts';

/** One title a screen can act on; facts only, never a projection. */
export type EntryActionTarget = Readonly<{libraryId: string; itemId: string; kind: ContentEntryKind; title: string}>;
type Api = Readonly<{request: <T>(path: string, method?: string, body?: unknown, signal?: AbortSignal) => Promise<T>}>;
export type EntryActionPorts = Readonly<{api: Api; serverId: string; viewerId: string; isCurrent: () => boolean; requestId: () => Promise<string>}>;
type OpenDelete = Readonly<{kind: 'delete'; target: EntryActionTarget; anchor: HTMLElement | null; ports: EntryActionPorts}>;
type Open = OpenDelete;

/** One shell-owned surface at a time; the effects open it with the captured authority and the
 * host owns the preview, the typed confirmation and the actual removal. */
export class EntryActionStore {
  private state: Open | null = null;
  private listeners = new Set<() => void>();
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => {this.listeners.delete(listener);}; };
  getSnapshot = (): Open | null => this.state;
  openDelete(target: EntryActionTarget, anchor: HTMLElement | null, ports: EntryActionPorts) { this.state = {kind: 'delete', target, anchor, ports}; this.emit(); }
  close() { if (!this.state) return; this.state = null; this.emit(); }
  private emit() { for (const listener of [...this.listeners]) listener(); }
}
export const entryActions = new EntryActionStore();

/** A screen's optional effect. It opens the shared dialog and resolves once it is open. */
export function deleteTitleEffect(ports: EntryActionPorts) {
  return async (target: EntryActionTarget, anchor: HTMLElement): Promise<void> => {
    if (!ports.isCurrent()) throw new DOMException('Screen authority changed', 'AbortError');
    entryActions.openDelete(target, anchor, ports);
  };
}
