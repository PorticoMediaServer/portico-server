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
  /** Retires the app-owned preparation when this runtime lets go of it. */
  release: () => Promise<void>;
  reportError: (error: unknown) => void;
}>;

export class LocalPlaybackRuntime {
  readonly slot = new PlayerSlot();
  private player?: PlaybackMediaPort;
  private video?: VideoLike;
  private queue?: WebLocalQueue;
  private run = 0;
  private disposed = false;
  private readonly scope: LocalPlaybackScope;

  constructor(private readonly options: LocalRuntimeOptions) {
    this.scope = localPlaybackScope(options.scope);
  }

  /** The one element this runtime plays into. A second element is refused, not shared. */
  mount(video: VideoLike): () => void {
    if (this.disposed || !this.options.isCurrent()) throw new Error('local_playback_unavailable');
    if (this.video && this.video !== video) throw new Error('playback-element-replaced');
    const create = this.options.environment.createPlayer;
    if (!create) throw new Error('local_playback_unavailable');
    this.video = video;
    this.player = create(video, {loadHls: this.options.environment.loadHls, nativeHls: this.options.environment.nativeHls, timers: this.options.environment.playerTimers});
    this.slot.mount(this.player);
    return () => { if (this.video !== video) return; this.video = undefined; this.slot.unmount(); this.player = undefined; };
  }

  get mounted(): boolean { return !!this.video && !this.disposed; }

  /** Start one receipt-verified preparation. The runtime takes custody of `resource`. */
  async play(resource: WebLocalQueueResource, lifetime: Readonly<{signal: AbortSignal; isCurrent(): boolean}>): Promise<void> {
    if (this.disposed || !this.mounted || !this.options.isCurrent()) { await resource.release().catch(() => {}); throw new Error('local_playback_unavailable'); }
    if (lifetime.signal.aborted || !lifetime.isCurrent()) { await resource.release().catch(() => {}); throw new DOMException('Local selection retired', 'AbortError'); }
    if (!sameLocalScope(resource.preparation.scope, this.scope) || !this.slot.canLoadLocal) {
      await resource.release().catch(() => {});
      throw new Error('local_playback_unavailable');
    }
    const preparation: WebLocalQueuePreparation = captureLocalQueue(resource.preparation, this.scope);
    const current = () => !this.disposed && this.options.isCurrent() && lifetime.isCurrent() && !lifetime.signal.aborted;
    const queue = new WebLocalQueue({
      scope: this.scope, run: ++this.run, port: this.slot, preparation,
      isCurrent: current,
      release: async () => { await resource.release().catch(() => {}); },
      reportError: error => { if (current()) this.options.reportError(error); },
    });
    this.queue = queue;
    try {
      await queue.start({signal: lifetime.signal, isCurrent: current}, {autoplay: true});
      if (!current()) throw new DOMException('Local selection retired', 'AbortError');
    } catch (error) {
      if (this.queue === queue) this.queue = undefined;
      throw error;
    }
  }

  getSnapshot(): WebLocalQueueSnapshot | undefined { return this.queue?.getSnapshot(); }

  subscribe(listener: () => void): () => void { return this.queue?.subscribe(listener) ?? (() => {}); }

  /** Stop local sound before anything else may own the element. */
  async dispose(): Promise<void> {
    if (this.disposed) return;
    this.disposed = true;
    const queue = this.queue;
    this.queue = undefined;
    try { queue?.dispose(); } catch { /* already retired */ }
    try { this.slot.unmount(); } catch { /* no mounted player */ }
    this.video = undefined;
    this.player = undefined;
    await this.options.release().catch(() => {});
  }

  /** Pause/play/seek/stop are local engine commands only. */
  play(): void { this.slot.play(); }
  pause(): void { this.slot.pause(); }
  seek(positionMs: number): void { this.slot.seek(positionMs); }
  stop(): Promise<void> { return this.dispose(); }
}
