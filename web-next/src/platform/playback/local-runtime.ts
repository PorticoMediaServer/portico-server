import {localPlaybackScope, sameLocalScope, type LocalPlaybackScope} from '../../../../packages/player-core/src/index.ts';
import type {LocalPlayerLoadContext} from '@core/playback-v1/local-port.ts';
import {captureLocalQueue, WebLocalQueue, type WebLocalQueuePreparation, type WebLocalQueueResource, type WebLocalQueueSnapshot} from './local-queue.ts';
import {PlayerSlot, type PlaybackMediaPort} from './web-playback.ts';
import type {HlsModule, VideoLike, WebMediaPlayerOptions} from './web-media-player.ts';

/** A player and queue for downloads alone, beside the online runtime.
 *
 * It reuses the same `PlayerSlot` and `WebLocalQueue` the shell player uses and the
 * platform's own `createPlayer` factory. It has no `WebPlaybackSource`, no V1 queue,
 * session, item-listening or capability route, and it mints no viewer fence: the only
 * authority it knows is the download scope the app already validated. Anything the
 * platform cannot report locally stays unknown here; nothing is borrowed from a server.
 *
 * One element, one active queue. A replacement retires the exact queue and awaits its
 * owned preparation release before the slot is used again; dispose returns the same task
 * until it finishes, and a failed cleanup keeps custody so a retry can finish it.
 */
export type LocalRuntimeEnvironment = Readonly<{
  createPlayer?: (video: VideoLike, options: WebMediaPlayerOptions) => PlaybackMediaPort;
  loadHls?: () => Promise<HlsModule>;
  nativeHls?: 'first' | 'fallback' | 'never';
  playerTimers?: WebMediaPlayerOptions['timers'];
}>;

export type LocalRuntimeOptions = Readonly<{
  scope: LocalPlaybackScope;
  environment: LocalRuntimeEnvironment;
  /** False once the persisted authority is spent or the window is leaving. */
  isCurrent: () => boolean;
  /** Retires the app-owned preparation this runtime let go of. */
  release: () => Promise<void>;
  reportError: (error: unknown) => void;
}>;

type Active = Readonly<{
  token: number;
  queue: WebLocalQueue;
  resource: WebLocalQueueResource;
  /** The actual owned preparation release; the queue's own release path shares it. */
  release: () => Promise<void>;
  off: () => void;
}>;

export class LocalPlaybackRuntime {
  readonly slot = new PlayerSlot();
  private player?: PlaybackMediaPort;
  private video?: VideoLike;
  private attachment?: Readonly<{video: VideoLike; token: number}>;
  private attachmentSerial = 0;
  private active?: Active;
  private serial = 0;
  private run = 0;
  private disposed = false;
  private retirement?: Promise<void>;
  private readonly listeners = new Set<() => void>();
  private readonly scope: LocalPlaybackScope;

  constructor(private readonly options: LocalRuntimeOptions) {
    this.scope = localPlaybackScope(options.scope);
  }

  /** The one element this runtime plays into. Repeating the same element is idempotent;
   * a different element while one is attached is refused, not replaced. */
  mount(video: VideoLike): () => void {
    this.assertAlive();
    if (this.attachment) {
      if (this.attachment.video !== video) throw new Error('playback-element-replaced');
      return () => {};
    }
    const create = this.options.environment.createPlayer;
    if (!create) throw new Error('local_playback_unavailable');
    const token = ++this.attachmentSerial;
    this.video = video;
    this.player = create(video, {loadHls: this.options.environment.loadHls, nativeHls: this.options.environment.nativeHls, timers: this.options.environment.playerTimers});
    this.slot.mount(this.player);
    this.attachment = {video, token};
    return () => {
      if (this.attachment?.token !== token) return;
      this.attachment = undefined;
      this.video = undefined;
      this.player = undefined;
      this.slot.unmount();
    };
  }

  get mounted(): boolean { return !!this.attachment && !this.disposed; }

  /** Start one receipt-verified preparation. Any previous queue is retired first and
   * its preparation release awaited, so two queues never share the slot. */
  async play(resource: WebLocalQueueResource, admission: Readonly<{signal: AbortSignal; isCurrent(): boolean}>): Promise<void> {
    const alive = () => this.alive();
    if (!alive() || !this.mounted || !this.slot.canLoadLocal) { await this.releaseQuiet(resource); throw new Error('local_playback_unavailable'); }
    if (admission.signal.aborted || !admission.isCurrent()) { await this.releaseQuiet(resource); throw new DOMException('Local selection retired', 'AbortError'); }
    if (!sameLocalScope(resource.preparation.scope, this.scope)) { await this.releaseQuiet(resource); throw new Error('local_playback_unavailable'); }
    const token = ++this.serial;
    let preparation: WebLocalQueuePreparation;
    try {
      preparation = captureLocalQueue(resource.preparation, this.scope);
    } catch (error) {
      await this.releaseQuiet(resource);
      throw error;
    }
    // Retire the exact previous queue and wait for its owned release before adopting.
    await this.retireActive();
    if (token !== this.serial || !alive()) { await this.releaseQuiet(resource); throw new Error('local_playback_unavailable'); }
    let queue: WebLocalQueue;
    const owned = once(() => resource.release());
    try {
      queue = new WebLocalQueue({
        scope: this.scope, run: ++this.run, port: this.slot, preparation,
        // The action's lifetime only admits the start; the runtime's own scope life keeps it.
        isCurrent: alive,
        release: owned.call,
        reportError: error => { if (alive()) this.options.reportError(error); },
      });
    } catch (error) {
      await this.releaseQuiet(resource);
      throw error;
    }
    const off = queue.subscribe(() => this.notify());
    const active = {token, queue, resource, release: owned.call, off};
    this.active = active;
    this.notify();
    try {
      await queue.start({signal: admission.signal, isCurrent: () => alive() && admission.isCurrent()}, {autoplay: true});
      if (token !== this.serial || !alive()) throw new Error('local_playback_unavailable');
    } catch (error) {
      if (this.active?.token === token) await this.retireActive().catch(report => this.options.reportError(report));
      else await this.releaseQuiet(resource);
      throw error;
    }
  }

  /** Local engine commands only, fenced by the queue's own snapshot. */
  resume(): void { const active = this.active, fence = active?.queue.getSnapshot().fence; if (active && fence) active.queue.resume(fence); }
  pause(): void { const active = this.active, fence = active?.queue.getSnapshot().fence; if (active && fence) active.queue.pause(fence); }
  seek(seconds: number): void { const active = this.active, fence = active?.queue.getSnapshot().fence; if (active && fence) active.queue.seek(fence, seconds); }

  getSnapshot(): WebLocalQueueSnapshot | undefined { return this.active?.queue.getSnapshot(); }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }

  /** Stop local sound and await the actual preparation release. Repeated calls return the
   * same task; a failed cleanup keeps its custody and may be retried. */
  dispose(): Promise<void> {
    if (this.retirement) return this.retirement;
    this.disposed = true;
    const task = (async () => {
      try {
        await this.retireActive();
        this.attachment = undefined;
        this.video = undefined;
        this.player = undefined;
        try { this.slot.unmount(); } catch { /* already unmounted */ }
      } catch (error) {
        // Keep custody of the unreleased preparation so a later dispose can finish it.
        this.retirement = undefined;
        this.disposed = false;
        this.options.reportError(error);
        throw error;
      }
    })();
    this.retirement = task;
    return task;
  }

  async stop(): Promise<void> { await this.dispose(); }

  private alive(): boolean { return !this.disposed && this.options.isCurrent(); }

  private assertAlive(): void { if (!this.alive()) throw new Error('local_playback_unavailable'); }

  private async retireActive(): Promise<void> {
    const active = this.active;
    if (!active) return;
    this.active = undefined;
    try { active.off(); } catch { /* no longer subscribed */ }
    try { active.queue.dispose(); } catch { /* release below still runs */ }
    try {
      await active.release();
    } catch (error) {
      // Retain the exact custody for a retry rather than pretending it was retired.
      if (!this.active) this.active = active;
      throw error;
    }
    this.notify();
  }

  private async releaseQuiet(resource: WebLocalQueueResource): Promise<void> {
    try { await resource.release(); } catch (error) { this.options.reportError(error); }
  }

  private notify(): void { for (const listener of [...this.listeners]) listener(); }
}

function once(run: () => Promise<void>): {call: () => Promise<void>} {
  let task: Promise<void> | undefined;
  return {call: () => task ??= run()};
}
