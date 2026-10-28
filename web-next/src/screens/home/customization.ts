import type {HomeService} from '@core/home-service.ts';
import type {PersonalResource, PersonalSavedService} from '@core/personal-saved.ts';
import {addHomeDraftView, homeDraftIntent, homeLayoutDraft, moveHomeDraftRow, removeHomeDraftView, showHomeDraftRow, type HomeLayoutDraft} from './layout-draft';

export type HomeCustomizationBindings = Readonly<{
  /** A fresh dialog-owned service, captured to this signed-in transport and SavedScope. */
  createSavedViews: () => PersonalSavedService;
  requestId: () => string;
  isCurrent: () => boolean;
}>;
export type HomeCustomizationState = Readonly<{
  phase: 'loading' | 'ready' | 'load-error';
  draft?: HomeLayoutDraft;
  pending?: 'save' | 'reset';
  error?: unknown;
  /** Only an explicit successful layout reload clears a save conflict. */
  conflict: boolean;
}>;
type LayoutService = Pick<HomeService, 'loadLayout' | 'saveLayout' | 'resetLayout'>;

/** One open dialog. The actual HomeService remains the sole layout producer/writer. */
export class HomeCustomization {
  private state: HomeCustomizationState = Object.freeze({phase: 'loading', conflict: false});
  private listeners = new Set<() => void>();
  private life = new AbortController();
  private read?: AbortController;
  private epoch = 0;
  constructor(private service: LayoutService, private bindings: HomeCustomizationBindings) {}
  getSnapshot = () => this.state;
  subscribe = (listener: () => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private current = () => !this.life.signal.aborted && this.bindings.isCurrent();
  private publish(patch: Partial<HomeCustomizationState>) {
    if (!this.current()) return;
    this.state = Object.freeze({...this.state, ...patch});
    for (const listener of this.listeners) listener();
  }
  load = async () => {
    if (!this.current() || this.state.pending) return;
    this.read?.abort();const read = new AbortController(), epoch = ++this.epoch;this.read = read;
    this.publish({phase: 'loading', draft: undefined, error: undefined});
    try {
      const view = await this.service.loadLayout(read.signal);
      if (this.current() && epoch === this.epoch && !read.signal.aborted) this.publish({phase: 'ready', draft: homeLayoutDraft(view), conflict: false});
    } catch (error) {
      if (this.current() && epoch === this.epoch && !read.signal.aborted) this.publish({phase: 'load-error', error});
    } finally {if (this.read === read) this.read = undefined;}
  };
  private edit(change: (draft: HomeLayoutDraft) => HomeLayoutDraft) {
    if (this.current() && this.state.phase === 'ready' && this.state.draft && !this.state.pending) this.publish({draft: change(this.state.draft)});
  }
  move = (id: string, target: number) => this.edit(draft => moveHomeDraftRow(draft, id, target));
  show = (id: string, shown: boolean) => this.edit(draft => showHomeDraftRow(draft, id, shown));
  add = (resource: PersonalResource) => this.edit(draft => addHomeDraftView(draft, resource));
  remove = (id: string) => this.edit(draft => removeHomeDraftView(draft, id));
  restoreDraft = (draft: HomeLayoutDraft) => this.edit(current => current.view === draft.view ? draft : current);
  save = async (): Promise<boolean> => {
    if (!this.current() || this.state.phase !== 'ready' || !this.state.draft || this.state.pending || this.state.conflict) return false;
    const intent = homeDraftIntent(this.state.draft), epoch = this.epoch;
    this.publish({pending: 'save', error: undefined});
    try {
      await this.service.saveLayout(intent.rowOrder, intent.hiddenRowIds, this.bindings.requestId, intent.expectedRevision);
      return this.current() && epoch === this.epoch;
    } catch (error) {
      if (this.current() && epoch === this.epoch) this.publish({error, conflict: (error as {code?: unknown} | null)?.code === 'home_layout_conflict'});
      return false;
    } finally {if (epoch === this.epoch) this.publish({pending: undefined});}
  };
  /** Called only after the drawn Reset confirmation. */
  reset = async (): Promise<boolean> => {
    if (!this.current() || this.state.phase !== 'ready' || !this.state.draft || this.state.pending || this.state.conflict) return false;
    const epoch = this.epoch, expectedRevision = this.state.draft.view.revision;this.publish({pending: 'reset', error: undefined});
    try {await this.service.resetLayout(this.bindings.requestId, expectedRevision);return this.current() && epoch === this.epoch;}
    catch (error) {if (this.current() && epoch === this.epoch) this.publish({error, conflict: (error as {code?: unknown} | null)?.code === 'home_layout_conflict'});return false;}
    finally {if (epoch === this.epoch) this.publish({pending: undefined});}
  };
  dispose() {this.life.abort();this.read?.abort();this.epoch++;this.listeners.clear();}
}
