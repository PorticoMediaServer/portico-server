import type {ReadingAnnotation, ReadingAnnotationContent, ReadingAnnotationPort, ReadingAnnotationsPage} from './reading-annotations.ts';

export type ReadingAnnotationState = Readonly<{entries: readonly ReadingAnnotation[]; revision?: number; nextCursor: string; loading: boolean; saving: boolean; error?: unknown}>;
const empty: ReadingAnnotationState = Object.freeze({entries: Object.freeze([]), nextCursor: '', loading: false, saving: false});
/** One captured profile/source session. No background loops, offline replay or re-anchoring. */
export class ReadingAnnotationService {
  private state: ReadingAnnotationState = Object.freeze({...empty, loading: true});
  private listeners = new Set<() => void>();
  private request?: AbortController;
  private disposed = false;
  constructor(readonly port: ReadingAnnotationPort, private readonly isCurrent: () => boolean) {}
  current = () => !this.disposed && !this.port.signal.aborted && this.port.manifest.canAnnotate === true && this.isCurrent();
  getSnapshot = () => this.current() ? this.state : empty;
  subscribe = (listener: () => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private publish(state: ReadingAnnotationState) {if (this.current()) {this.state = Object.freeze(state);this.listeners.forEach(listener => listener());}}
  private owns(request: AbortController) {return this.current() && this.request === request && !request.signal.aborted;}
  private accept(page: ReadingAnnotationsPage, append: boolean) {
    if (append && page.revision !== this.state.revision) throw new Error('annotation_revision_changed');
    const entries = append ? [...this.state.entries, ...page.entries] : [...page.entries];
    if (entries.length > 256 || new Set(entries.map(entry => entry.id)).size !== entries.length) throw new Error('invalid_reading_annotations');
    this.publish({entries: Object.freeze(entries), revision: page.revision, nextCursor: page.nextCursor, loading: false, saving: false});
  }
  load = async (more = false): Promise<void> => {
    if (!this.current() || this.request || more && !this.state.nextCursor) return;
    const request = new AbortController();this.request = request;
    const cursor = more ? this.state.nextCursor : '', revision = more ? this.state.revision : undefined;
    this.publish({...this.state, loading: true, error: undefined});
    try {
      const page = await this.port.annotations(cursor, revision, request.signal);
      if (this.owns(request)) this.accept(page, more);
    } catch (error) {if (this.owns(request)) this.publish({...this.state, revision: undefined, loading: false, saving: false, error});}
    finally {if (this.request === request) this.request = undefined;}
  };
  write = async (content: ReadingAnnotationContent | Readonly<{id: string; remove: true}>): Promise<boolean> => {
    if (!this.current() || this.request || this.state.revision === undefined) return false;
    const expectedRevision = this.state.revision;
    const captured = structuredClone(content);
    const mutation = 'remove' in captured ? {operation: 'delete' as const, expectedRevision, id: captured.id}
      : {...captured, operation: 'upsert' as const, expectedRevision};
    const request = new AbortController();this.request = request;
    this.publish({...this.state, saving: true, error: undefined});
    try {
      const saved = await this.port.mutateAnnotation(mutation, request.signal);
      if (!this.owns(request)) return false;
      const page = await this.port.annotations('', undefined, request.signal);
      if (!this.owns(request)) return false;
      if (page.revision !== saved.revision) throw new Error('annotation_revision_changed');
      this.accept(page, false);return true;
    } catch (error) {
      // A lost response is never replayed. A genuine explicit reload is required.
      if (this.owns(request)) this.publish({...this.state, revision: undefined, loading: false, saving: false, error});
      return false;
    } finally {if (this.request === request) this.request = undefined;}
  };
  dispose() {this.disposed = true;this.request?.abort();this.request = undefined;this.listeners.clear();this.state = empty;}
}
