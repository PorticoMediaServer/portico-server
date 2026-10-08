/**
 * Playback v1, phase 2 prep: the web `MediaPlayer` port over `<video>` and hls.js (Plan — Client
 * Playback Migration §2). Not wired to screens yet; `PlaybackSessionController` drives it.
 *
 * - `load` starts a new presentation generation: the previous engine is destroyed, sidecar
 *   `<track>`s are replaced, and every event is tagged with the generation now loaded, so the
 *   core drops a late `timeupdate` from an older URL.
 * - Direct play (`mode: 'direct'`) sets `src`; streams use hls.js, or native HLS (Safari) when hls.js
 *   is unsupported.
 * - Failures: a stream the server refuses (401/403/404/410) is `stream_gone`; network errors retry
 *   twice before `network_error`; media errors recover twice (the second swaps the audio codec)
 *   before `decode_error`. What to do next (a new session, a lower rung) is the core's decision.
 * - Everything DOM- or hls.js-shaped is behind small interfaces so the adapter is unit-tested with fakes.
 */
import type {MediaPlayer, PlayerEvent, PlayerObservation, PlayerSource, PlayerState} from '@core/playback-v1/index.ts';

/** The parts of `HTMLVideoElement` the adapter uses. */
export type VideoLike = {
  src: string;
  currentTime: number;
  playbackRate: number;
  volume: number;
  muted: boolean;
  paused: boolean;
  ended: boolean;
  readonly buffered: {length: number; start(i: number): number; end(i: number): number};
  readonly textTracks: ArrayLike<{id: string; mode: string}>;
  play(): Promise<void> | void;
  pause(): void;
  load(): void;
  removeAttribute(name: string): void;
  canPlayType(type: string): string;
  appendChild(node: unknown): unknown;
  querySelectorAll(selector: string): ArrayLike<{remove(): void}>;
  addEventListener(type: string, listener: () => void): void;
  removeEventListener(type: string, listener: () => void): void;
  getVideoPlaybackQuality?(): {droppedVideoFrames: number};
  ownerDocument: {createElement(tag: 'track'): {kind: string; src: string; id: string; srclang: string; label: string; default: boolean}};
};

/** The parts of an hls.js instance the adapter uses. */
export type HlsLike = {
  attachMedia(media: unknown): void;
  loadSource(url: string): void;
  startLoad(position?: number): void;
  recoverMediaError(): void;
  swapAudioCodec(): void;
  destroy(): void;
  on(event: string, listener: (event: string, data: HlsErrorData) => void): void;
  readonly bandwidthEstimate?: number;
};
export type HlsErrorData = {fatal?: boolean; type?: string; details?: string; response?: {code?: number}};
/** The hls.js constructor surface: `Hls.isSupported()`, `new Hls(config)`, `Hls.Events.ERROR`, `Hls.ErrorTypes`. */
export type HlsModule = {
  isSupported(): boolean;
  new (config: Record<string, unknown>): HlsLike;
  Events: {ERROR: string};
  ErrorTypes: {NETWORK_ERROR: string; MEDIA_ERROR: string};
};

export type WebMediaPlayerOptions = Readonly<{
  /** Loads hls.js on first use (a dynamic import in the app). */
  loadHls?: () => Promise<HlsModule>;
  /** hls.js tuning (buffer sizes, load policies); the adapter adds `startPosition` and `xhrSetup`. */
  hlsConfig?: Record<string, unknown>;
  /** Makes a presentation URL absolute (`/v1/media/…` against the selected server's origin). */
  resolveUrl?: (url: string) => string;
}>;

export class WebMediaPlayer implements MediaPlayer {
  private video: VideoLike;
  private options: WebMediaPlayerOptions;
  private listeners = new Set<(event: PlayerEvent) => void>();
  private generation = 0;
  private state: PlayerState = 'idle';
  private hls?: HlsLike;
  private subtitle: string | null = null;
  private seeking = false;
  private pendingStartMs?: number;
  private autoplay = false;
  private networkRetries = 0;
  private mediaRecoveries = 0;
  private disposed = false;
  private handlers: [string, () => void][] = [];

  constructor(video: VideoLike, options: WebMediaPlayerOptions = {}) {
    this.video = video;
    this.options = options;
    const on = (type: string, fn: () => void) => { video.addEventListener(type, fn); this.handlers.push([type, fn]); };
    on('playing', () => this.setState('playing'));
    on('pause', () => { if (!video.ended) this.setState('paused'); });
    on('waiting', () => this.setState('buffering'));
    on('ended', () => this.setState('ended'));
    on('loadedmetadata', () => {
      // Direct play starts where the session says; hls.js does this through `startPosition`.
      if (this.pendingStartMs !== undefined) { video.currentTime = this.pendingStartMs / 1000; this.pendingStartMs = undefined; }
    });
    on('canplay', () => {
      if (this.state !== 'loading') return;
      if (this.autoplay) void Promise.resolve(video.play()).catch(() => this.setState('paused'));
      else this.setState('paused');
    });
    on('timeupdate', () => this.emit({type: 'time', observation: this.observe()}));
    on('seeked', () => { if (!this.seeking) return; this.seeking = false; this.emit({type: 'seeked', observation: this.observe()}); });
    on('error', () => { if (!this.hls) this.fail('decode_error', 'media element error'); });
  }

  load(source: PlayerSource): void {
    if (this.disposed) return;
    this.teardown();
    this.generation = source.generation;
    this.autoplay = source.autoplay;
    this.networkRetries = 0;
    this.mediaRecoveries = 0;
    this.seeking = false;
    this.subtitle = null;
    this.setState('loading');
    const url = this.resolve(source.url);
    this.addSidecars(source);
    if (source.mode === 'direct') {
      this.pendingStartMs = source.startPositionMs > 0 ? source.startPositionMs : undefined;
      this.video.src = url;
      this.video.load();
      return;
    }
    const generation = source.generation;
    const load = this.options.loadHls;
    if (!load) { this.native(source, url); return; }
    void load().then(Hls => {
      if (this.disposed || generation !== this.generation) return;
      if (!Hls.isSupported()) { this.native(source, url); return; }
      const origin = new URL(url).origin;
      const engine = new Hls({
        ...this.options.hlsConfig,
        startPosition: source.startPositionMs / 1000,
        // Capability URLs never leave their origin and never carry cookies.
        xhrSetup: (xhr: {withCredentials: boolean}, url: string) => {
          if (new URL(url).origin !== origin) throw new Error('Unexpected stream origin');
          xhr.withCredentials = false;
        },
      });
      this.hls = engine;
      engine.on(Hls.Events.ERROR, (_e, data) => this.hlsError(engine, Hls, data));
      engine.attachMedia(this.video);
      engine.loadSource(url);
    }, () => { if (generation === this.generation) this.fail('engine_unavailable', 'hls.js could not load'); });
  }

  /** Safari and iOS play HLS natively. */
  private native(source: PlayerSource, url: string) {
    if (!this.video.canPlayType('application/vnd.apple.mpegurl')) { this.fail('unsupported', 'no HLS support'); return; }
    this.pendingStartMs = source.startPositionMs > 0 ? source.startPositionMs : undefined;
    this.video.src = url;
    this.video.load();
  }

  private hlsError(engine: HlsLike, Hls: HlsModule, data: HlsErrorData) {
    if (engine !== this.hls || !data.fatal) return;
    if (data.type === Hls.ErrorTypes.NETWORK_ERROR) {
      const status = data.response?.code;
      // The server answering "no" is a fact; not answering is weather.
      if (status === 401 || status === 403 || status === 404 || status === 410) { this.fail('stream_gone', `HTTP ${status}`); return; }
      if (this.networkRetries++ < 2) { engine.startLoad(); return; }
      this.fail('network_error', String(data.details ?? '').slice(0, 120));
      return;
    }
    if (data.type === Hls.ErrorTypes.MEDIA_ERROR && this.mediaRecoveries < 2) {
      if (this.mediaRecoveries++ === 1) engine.swapAudioCodec();
      engine.recoverMediaError();
      return;
    }
    this.fail(data.type === Hls.ErrorTypes.MEDIA_ERROR ? 'decode_error' : 'engine_error', String(data.details ?? '').slice(0, 120));
  }

  private addSidecars(source: PlayerSource) {
    for (const s of source.subtitles) {
      const track = this.video.ownerDocument.createElement('track');
      track.kind = 'subtitles';
      track.id = s.trackId;
      track.src = this.resolve(s.url);
      track.srclang = '';
      track.label = s.trackId;
      track.default = false;
      this.video.appendChild(track);
    }
    this.applySubtitle();
  }

  private applySubtitle() {
    const tracks = this.video.textTracks;
    for (let i = 0; i < tracks.length; i++) { const t = tracks[i]!; t.mode = t.id === this.subtitle && this.subtitle !== null ? 'showing' : 'disabled'; }
  }

  private resolve(url: string): string { return this.options.resolveUrl ? this.options.resolveUrl(url) : url; }

  play(): void { void Promise.resolve(this.video.play()).catch(() => {}); }
  pause(): void { this.video.pause(); }
  seek(positionMs: number): void { this.seeking = true; this.video.currentTime = Math.max(0, positionMs) / 1000; }
  setRate(rate: number): void { this.video.playbackRate = rate; }

  selectSidecarSubtitle(trackId: string | null): void {
    this.subtitle = trackId;
    this.applySubtitle();
    this.emit({type: 'state', observation: this.observe()});
  }

  observe(): PlayerObservation {
    const v = this.video;
    let bufferedMs: number | undefined;
    for (let i = 0; i < v.buffered.length; i++) if (v.buffered.start(i) <= v.currentTime && v.currentTime <= v.buffered.end(i)) bufferedMs = Math.round((v.buffered.end(i) - v.currentTime) * 1000);
    const dropped = v.getVideoPlaybackQuality?.().droppedVideoFrames;
    const bandwidth = this.hls?.bandwidthEstimate;
    return {
      generation: this.generation, state: this.state, positionMs: Math.max(0, Math.round(v.currentTime * 1000)), rate: v.playbackRate,
      volume: v.volume, muted: v.muted, subtitleTrackId: this.subtitle,
      ...(bufferedMs !== undefined ? {bufferedMs} : {}),
      ...(dropped !== undefined ? {droppedFrames: dropped} : {}),
      ...(bandwidth !== undefined && Number.isFinite(bandwidth) ? {bandwidthKbps: Math.round(bandwidth / 1000)} : {}),
    };
  }

  onEvent(listener: (event: PlayerEvent) => void): () => void {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }

  dispose(): void {
    if (this.disposed) return;
    this.teardown();
    for (const [type, fn] of this.handlers) this.video.removeEventListener(type, fn);
    this.handlers = [];
    this.listeners.clear();
    this.disposed = true;
    this.state = 'idle';
  }

  private teardown() {
    this.hls?.destroy();
    this.hls = undefined;
    this.pendingStartMs = undefined;
    this.video.pause();
    const old = this.video.querySelectorAll('track');
    for (let i = 0; i < old.length; i++) old[i]!.remove();
    this.video.removeAttribute('src');
    this.video.load();
  }

  private setState(state: PlayerState) {
    if (this.disposed || this.state === state) return;
    this.state = state;
    this.emit({type: 'state', observation: this.observe()});
  }

  private fail(code: string, detail?: string) {
    this.hls?.destroy();
    this.hls = undefined;
    this.state = 'error';
    this.emit({type: 'error', observation: this.observe(), code, ...(detail ? {detail} : {})});
  }

  private emit(event: PlayerEvent) {
    if (this.disposed) return;
    for (const l of [...this.listeners]) l(event);
  }
}
