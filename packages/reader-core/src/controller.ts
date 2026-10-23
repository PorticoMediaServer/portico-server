import type {ReaderAdapter, ReaderDocument, ReaderFormat, ReaderLocation, ReaderNavigation, ReaderMetadata,
  ReaderSearchOptions, ReaderSearchResult, ReaderSnapshot, ReaderSource, ReaderView, ReaderViewport,
  ReaderZoom, ReaderSurfaceInput} from './ports.ts';
import {ReaderError, readerErrorCode, throwIfAborted} from './errors.ts';
import {validateSource} from './source.ts';
import {validateViewport} from './layout.ts';

type Session<P extends object> = {abort: AbortController; document?: ReaderDocument<P>; view?: ReaderView<P>; tail: Promise<void>};
const empty = Object.freeze([]);
function copy<P extends object>(value: P): Readonly<P> { return Object.freeze({...value}); }
export function validateLocation(location: ReaderLocation, format: ReaderFormat): void {
  if (location.format !== format || !Number.isFinite(location.progression) || location.progression < 0 || location.progression > 1
    || (location.format === 'epub' ? !/^epubcfi\(.+\)$/.test(location.cfi) : !Number.isSafeInteger(location.page) || location.page < 0))
    throw new ReaderError('invalid_location');
}

/** Session scope owns all overrides, asynchronous work and mounted resources.
 * Incoming defaults remain live even when an individual field is overridden.
 * The controller never receives a persistence port or a credential. */
export class ReaderController<P extends object> {
  private readonly adapters = new Map<ReaderFormat, ReaderAdapter<P>>();
  private readonly surfaceListeners = new Set<(event: ReaderSurfaceInput) => void>();
  private readonly listeners = new Set<() => void>();
  private base: Readonly<P>;
  private overrides: Partial<P> = {};
  private session?: Session<P>;
  private viewport: ReaderViewport = Object.freeze({width: 1, height: 1});
  private snapshot: ReaderSnapshot<P>;
  constructor(options: Readonly<{adapters: readonly ReaderAdapter<P>[]; preferences: Readonly<P>}>) {
    for (const adapter of options.adapters) {
      if (this.adapters.has(adapter.format)) throw new ReaderError('unsupported_format');
      this.adapters.set(adapter.format, adapter);
    }
    this.base = copy(options.preferences);
    this.snapshot = Object.freeze({status: 'closed', zoom: null, canTurnAtFit: true, toc: empty, location: null, selection: null,
      preferences: this.base, overriddenKeys: empty});
  }
  /** Stable until an actual publication; safe for useSyncExternalStore. */
  getSnapshot = (): ReaderSnapshot<P> => this.snapshot;
  subscribe = (listener: () => void): (() => void) => { this.listeners.add(listener); return () => this.listeners.delete(listener); };
  subscribeSurfaceInput = (listener: (event: ReaderSurfaceInput) => void): (() => void) => {
    this.surfaceListeners.add(listener); return () => this.surfaceListeners.delete(listener);
  };
  refreshViewState() {
    const view = this.session?.view;
    this.publish({zoom: view?.getZoom?.() ?? null, canTurnAtFit: view?.canTurnAtFit?.() ?? true});
  }
  private publish(patch: Partial<ReaderSnapshot<P>>) {
    this.snapshot = Object.freeze({...this.snapshot, ...patch});
    for (const listener of this.listeners) listener();
  }
  private current(session: Session<P>) { return this.session === session && !session.abort.signal.aborted; }
  private effective(): Readonly<P> { return copy({...this.base, ...this.overrides}); }
  private publishPreferences() {
    this.publish({preferences: this.effective(), overriddenKeys: Object.freeze(Object.keys(this.overrides) as (keyof P)[])});
  }
  async open(options: Readonly<{source: ReaderSource; format: ReaderFormat; container: HTMLElement; location?: ReaderLocation; viewport?: ReaderViewport; metadata?: ReaderMetadata}>): Promise<void> {
    // Invalidate before waiting for disposal, so simultaneous opens cannot adopt
    // a stale document/view. A late adapter result is disposed in its own turn.
    const previous = this.session;
    this.session = undefined;
    previous?.abort.abort();
    const session: Session<P> = {abort: new AbortController(), tail: Promise.resolve()};
    this.session = session;
    this.overrides = {};
    this.publish({status: 'opening', zoom: null, canTurnAtFit: true, format: options.format, sourceId: options.source.id, sourceRevision: options.source.revision, metadata: undefined,
      toc: empty, location: null, selection: null, error: undefined, preferences: this.base, overriddenKeys: empty});
    try {
      await this.disposeSession(previous);
      if (!this.current(session)) return;
      validateSource(options.source);
      if (options.location) validateLocation(options.location, options.format);
      const adapter = this.adapters.get(options.format);
      if (!adapter) throw new ReaderError('unsupported_format');
      this.viewport = Object.freeze(options.viewport ?? {width: options.container.clientWidth, height: options.container.clientHeight});
      validateViewport(this.viewport);
      const document = await adapter.open({source: options.source, signal: session.abort.signal});
      if (!this.current(session)) { await document.dispose(); return; }
      session.document = document;
      if (document.format !== options.format || document.id !== options.source.id || document.revision !== options.source.revision) throw new ReaderError('invalid_book');
      const view = await document.mount({container: options.container, preferences: this.snapshot.preferences,
        viewport: this.viewport, signal: session.abort.signal,
        onLocation: location => {
          if (!this.current(session)) return;
          validateLocation(location, options.format);
          this.publish({location: copy(location)});
        },
        onSurfaceInput: event => {
          if (this.current(session)) for (const listener of this.surfaceListeners) listener(event);
        },
        onSelection: selection => {
          if (!this.current(session)) return;
          this.publish({selection: selection ? Object.freeze({...selection, rects: Object.freeze(selection.rects.map(copy))}) : null});
        }});
      if (!this.current(session)) { await view.dispose(); return; }
      session.view = view;
      // Defaults may have changed while the adapter was opening/mounting.
      await this.enqueue(async (mounted, active) => {
        await mounted.applyPreferences(this.snapshot.preferences, active);
        if (options.location) await mounted.navigate({kind: 'location', location: options.location}, active);
      });
      if (!this.current(session)) return;
      const location = view.getLocation();
      if (location) validateLocation(location, options.format);
      this.refreshViewState();
      this.publish({status: 'ready', metadata: copy(options.metadata === undefined ? document.metadata : {...document.metadata, ...options.metadata,
        title: options.metadata.title, author: options.metadata.author, language: options.metadata.language}), toc: freezeToc(document.toc),
        location: location ? copy(location) : this.snapshot.location});
    } catch (error) {
      if (!this.current(session)) return;
      session.abort.abort();
      this.session = undefined;
      await this.disposeSession(session);
      this.publish({status: 'error', error: readerErrorCode(error), selection: null});
    }
  }
  private async disposeSession(session?: Session<P>) {
    if (!session) return;
    session.abort.abort();
    // A view error must not prevent its document/ZIP/blob disposal.
    try { await session.view?.dispose(); }
    finally { await session.document?.dispose(); }
  }
  async close(): Promise<void> {
    const session = this.session;
    this.session = undefined;
    session?.abort.abort(); this.overrides = {};
    this.snapshot = Object.freeze({status: 'closed', zoom: null, canTurnAtFit: true, toc: empty, location: null, selection: null,
      preferences: this.base, overriddenKeys: empty});
    for (const listener of this.listeners) listener();
    await this.disposeSession(session);
  }
  private enqueue(operation: (view: ReaderView<P>, signal: AbortSignal) => Promise<void>): Promise<void> {
    const session = this.session;
    if (!session?.view || !this.current(session)) return Promise.reject(new ReaderError('closed'));
    const next = session.tail.then(async () => {
      throwIfAborted(session.abort.signal);
      await operation(session.view!, session.abort.signal);
      if (this.current(session)) this.refreshViewState();
      throwIfAborted(session.abort.signal);
    });
    session.tail = next.catch(error => { if (this.current(session)) this.publish({error: readerErrorCode(error)}); });
    return next;
  }
  navigate(target: ReaderNavigation): Promise<void> {
    if (target.kind === 'location') validateLocation(target.location, this.snapshot.format!);
    if (target.kind === 'progression' && (!Number.isFinite(target.value) || target.value < 0 || target.value > 1))
      return Promise.reject(new ReaderError('invalid_location'));
    return this.enqueue((view, signal) => view.navigate(target, signal));
  }
  updatePreferences(preferences: Readonly<P>): Promise<void> {
    this.base = copy(preferences); this.publishPreferences();
    return this.session?.view ? this.enqueue((view, signal) => view.applyPreferences(this.snapshot.preferences, signal)) : Promise.resolve();
  }
  setOverride(patch: Partial<P>): Promise<void> {
    if (!this.session || this.snapshot.status === 'error') return Promise.reject(new ReaderError('closed'));
    for (const key of Object.keys(patch) as (keyof P)[]) if (!(key in this.base) || patch[key] === undefined)
      return Promise.reject(new ReaderError('invalid_preferences'));
    this.overrides = {...this.overrides, ...patch}; this.publishPreferences();
    return this.session.view ? this.enqueue((view, signal) => view.applyPreferences(this.snapshot.preferences, signal)) : Promise.resolve();
  }
  clearOverrides(keys?: readonly (keyof P)[]): Promise<void> {
    if (keys) { for (const key of keys) delete this.overrides[key]; } else this.overrides = {};
    this.publishPreferences();
    return this.session?.view ? this.enqueue((view, signal) => view.applyPreferences(this.snapshot.preferences, signal)) : Promise.resolve();
  }
  resize(viewport: ReaderViewport): Promise<void> {
    validateViewport(viewport); this.viewport = copy(viewport);
    return this.enqueue((view, signal) => view.resize(this.viewport, signal));
  }
  setZoom(zoom: ReaderZoom): Promise<void> {
    if (zoom.kind === 'scale' && (!Number.isFinite(zoom.value) || zoom.value <= 0 || zoom.value > 8))
      return Promise.reject(new ReaderError('invalid_preferences'));
    return this.enqueue((view, signal) => { if (!view.setZoom) throw new ReaderError('unsupported_operation'); return view.setZoom(zoom, signal); });
  }
  getZoom() { return this.session?.view?.getZoom?.() ?? null; }
  pan(offset: Readonly<{x: number; y: number}>): Promise<void> {
    if (!Number.isFinite(offset.x) || !Number.isFinite(offset.y)) return Promise.reject(new ReaderError('invalid_preferences'));
    return this.enqueue((view, signal) => { if (!view.pan) throw new ReaderError('unsupported_operation'); return view.pan(offset, signal); });
  }
  async *search(query: string, options: ReaderSearchOptions): AsyncIterable<ReaderSearchResult> {
    const session = this.session;
    if (!session?.document || !this.current(session)) throw new ReaderError('closed');
    if (!query.trim() || query.length > 1024 || !Number.isSafeInteger(options.limit) || options.limit < 1 || options.limit > 500)
      throw new ReaderError('limit_exceeded');
    const signal = AbortSignal.any([options.signal, session.abort.signal]);
    let count = 0;
    for await (const result of session.document.search(query, {...options, signal})) {
      throwIfAborted(signal);
      if (!this.current(session)) throw new ReaderError('aborted');
      validateLocation(result.location, session.document.format);
      yield Object.freeze({...result, location: copy(result.location)});
      if (++count >= options.limit) return;
    }
  }
}
function freezeToc(items: readonly import('./ports.ts').ReaderTocItem[]): readonly import('./ports.ts').ReaderTocItem[] {
  return Object.freeze(items.map(item => Object.freeze({...item, location: copy(item.location),
    ...(item.children ? {children: freezeToc(item.children)} : {})})));
}
