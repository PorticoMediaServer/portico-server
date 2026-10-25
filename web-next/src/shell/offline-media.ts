/** The launch page's one media surface, owned by the shell.
 *
 * It draws into one visible element through the same `PlaybackMediaOwner` the player
 * screen uses, and runs the local-only runtime (`LocalPlaybackRuntime`) beside the online
 * stack. There is no second player, queue or store: the desktop host supplies its own
 * `createPlayer` factory and the app-owned download broker as the local port. The runtime
 * is terminal once it retires, and the host reports the actual retirement so the next
 * native owner can await it before commanding the device.
 */
import {localPlaybackScope, type LocalPlaybackScope} from '../../../packages/player-core/src/index.ts';
import {LocalPlaybackRuntime, type LocalRuntimeEnvironment} from '../platform/playback/local-runtime.ts';
import type {WebLocalQueueResource} from '../platform/playback/local-queue.ts';
import {PlaybackMediaOwner} from './playback-media-owner.ts';

export type OfflineMediaSnapshot = Readonly<{
  phase: 'idle' | 'loading' | 'playing' | 'paused' | 'ended' | 'error';
  itemId?: string;
}>;

export type OfflineMediaLifetime = Readonly<{signal: AbortSignal; isCurrent(): boolean}>;

const idle: OfflineMediaSnapshot = Object.freeze({phase: 'idle'});

export class OfflineMediaHost {
  private readonly owner = new PlaybackMediaOwner<HTMLVideoElement>();
  private readonly scope: LocalPlaybackScope;
  private readonly listeners = new Set<() => void>();
  private runtime?: LocalPlaybackRuntime;
  private element?: HTMLVideoElement;
  private attachment?: () => void;
  private live = true;
  private retirement?: Promise<void>;

  constructor(private readonly options: Readonly<{
    scope: LocalPlaybackScope;
    /** The platform's own factory and local port, never a launch-only player. */
    environment: LocalRuntimeEnvironment;
    isCurrent: () => boolean;
    reportError: (error: unknown) => void;
    /** The actual retirement task, handed to the next native owner as its barrier. */
    onRetire?: (task: Promise<void>) => void;
  }>) {
    this.scope = localPlaybackScope(options.scope);
  }

  /** The one element this host draws into, while it is attached. */
  element(): HTMLVideoElement | undefined { return this.element; }

  get mounted(): boolean { return !!this.element && !!this.runtime?.mounted; }

  /** Attach (or release) the page's visible element. A release starts the terminal
   * retirement, so nothing here may draw into a later element in this window. */
  attach(element: HTMLVideoElement | null): void {
    if (element === this.element) return;
    if (!element) { this.detach(); void this.retire().catch(error => this.options.reportError(error)); return; }
    if (!this.live || this.retirement) { this.options.reportError(new Error('offline_media_retiring')); return; }
    this.detach();
    const runtime = this.runtime ?? this.create();
    this.element = element;
    try { this.attachment = this.owner.adopt(runtime, element); }
    catch (error) { this.element = undefined; this.options.reportError(error); }
    this.notify();
  }

  /** Play one already-prepared local resource through the local-only runtime. */
  async play(resource: WebLocalQueueResource, admission: OfflineMediaLifetime): Promise<void> {
    const runtime = this.runtime;
    if (!this.live || !this.element || !runtime?.mounted || admission.signal.aborted || !admission.isCurrent()) {
      await resource.release().catch(error => this.options.reportError(error));
      throw new DOMException('Offline playback is not available.', 'AbortError');
    }
    await runtime.play(resource, {signal: admission.signal, isCurrent: () => this.live && !!this.element && admission.isCurrent()});
  }

  snapshot(): OfflineMediaSnapshot {
    const queue = this.runtime?.getSnapshot();
    if (!queue) return idle;
    const item = queue.preparation.items[queue.index], state = queue.observation?.state;
    const phase: OfflineMediaSnapshot['phase'] = queue.phase === 'ready'
      ? (state === 'playing' ? 'playing' : state === 'loading' || state === 'buffering' ? 'loading' : 'paused')
      : queue.phase === 'disposed' ? 'idle' : queue.phase;
    return Object.freeze({phase, ...(item ? {itemId: item.itemId} : {})});
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }

  /** Terminal. Repeated calls share one task; a failed cleanup keeps the runtime's custody
   * and is retried here instead of being reported as finished. */
  retire(): Promise<void> {
    if (this.retirement) return this.retirement;
    this.live = false;
    this.detach();
    const runtime = this.runtime;
    const task = (async () => {
      await runtime?.dispose();
      if (this.runtime === runtime) this.runtime = undefined;
    })();
    const tracked = task.finally(() => { if (this.retirement === tracked) this.retirement = undefined; });
    this.retirement = tracked;
    this.options.onRetire?.(tracked);
    this.notify();
    return tracked;
  }

  private create(): LocalPlaybackRuntime {
    const runtime = new LocalPlaybackRuntime({
      scope: this.scope,
      environment: this.options.environment,
      isCurrent: () => this.live && !!this.element && this.options.isCurrent(),
      // Every resource carries its own release; the runtime needs no fallback owner.
      release: () => Promise.resolve(),
      reportError: error => this.options.reportError(error),
    });
    runtime.subscribe(() => this.notify());
    return runtime;
  }

  private detach(): void {
    const release = this.attachment;
    this.attachment = undefined;
    this.element = undefined;
    try { release?.(); } catch (error) { this.options.reportError(error); }
  }

  private notify(): void { for (const listener of [...this.listeners]) listener(); }
}
