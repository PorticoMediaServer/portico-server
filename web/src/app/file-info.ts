import {readFileInfo, type DetailApi, type DetailScope, type DetailTarget, type FileInfoProjection} from '@core/detail.ts';
import {CodedError, unreadableServerResponse} from '@core/server-messages.ts';

export type FileInfoState = Readonly<{phase: 'idle' | 'loading' | 'ready' | 'error'; data?: FileInfoProjection; error?: unknown}>;

/** One lazy file read belongs to one viewer, title and detail fence. It never
 * caches owner locations across selection or retains them after a refused read. */
export class FileInfoReader {
  private state: FileInfoState = {phase: 'idle'};
  private listeners = new Set<() => void>();
  private controller?: AbortController;
  private generation = 0;
  private disposed = false;
  private api: DetailApi;
  private scope: DetailScope;
  private target: DetailTarget;
  private viewerFence: string;
  constructor(api: DetailApi, scope: DetailScope, target: DetailTarget, viewerFence: string) {
    this.api = api; this.scope = scope; this.target = target; this.viewerFence = viewerFence;
  }
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private emit(state: FileInfoState) { this.state = state; for (const listener of this.listeners) listener(); }
  async load(more = false) {
    if (this.disposed || (more && (this.state.phase !== 'ready' || !this.state.data?.nextCursor))) return;
    const previous = more ? this.state.data : undefined;
    const generation = ++this.generation;
    this.controller?.abort();
    const controller = this.controller = new AbortController();
    this.emit({phase: 'loading', ...(previous ? {data: previous} : {})});
    try {
      const page = await readFileInfo(this.api, this.scope, this.target, {limit: 50, ...(previous?.nextCursor ? {cursor: previous.nextCursor} : {})}, controller.signal);
      if (this.disposed || controller.signal.aborted || generation !== this.generation) return;
      if (page.scope.viewerFence !== this.viewerFence || (previous && (
        page.revision.catalog !== previous.revision.catalog || page.revision.viewer !== previous.revision.viewer ||
        page.kind !== previous.kind || JSON.stringify(page.manage) !== JSON.stringify(previous.manage) ||
        page.files.some(file => previous.files.some(old => old.id === file.id)) || page.nextCursor === previous.nextCursor
      ))) throw new CodedError('invalid_response', unreadableServerResponse);
      this.emit({phase: 'ready', data: previous ? {...page, files: [...previous.files, ...page.files]} : page});
    } catch (error) {
      if (!this.disposed && !controller.signal.aborted && generation === this.generation) this.emit({phase: 'error', error});
    } finally { if (this.controller === controller) this.controller = undefined; }
  }
  activate() { this.disposed = false; }
  dispose() {
    this.disposed = true;
    this.generation++;
    this.controller?.abort();
    this.state = {phase: 'idle'};
    this.listeners.clear();
  }
}
