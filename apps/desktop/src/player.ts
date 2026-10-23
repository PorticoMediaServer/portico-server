import type {PlayerEvent, PlayerObservation, PlayerSource} from '../../../packages/client-core/src/playback-v1/port.ts';
import type {PlaybackMediaPort, VideoLike, WebMediaPlayerOptions, WebPlayerNotice} from '../../../web-next/src/platform/playback/index.ts';
import type {Bridge, NativeCommand} from './bridge.ts';
import {NativeHole} from './surface.ts';

let nextNativeGeneration = 0;

/** The exact existing web MediaPlayer seam. Transport/session/queue/UI remain with their owners. */
export class DesktopMediaPlayer implements PlaybackMediaPort {
  private source?: PlayerSource;
  private reading: PlayerObservation = {generation: 0, state: 'idle', positionMs: 0, rate: 1};
  private listeners = new Set<(event: PlayerEvent) => void>();
  private notices = new Set<(event: WebPlayerNotice) => void>();
  private serial: Promise<void> = Promise.resolve();
  private disposed = false;
  private epoch = 0;
  private nativeGeneration = 0;
  private polling = false;
  private surfaceActive = false;
  private readonly hole?: NativeHole;
  private seeking = false;
  private ticker?: ReturnType<typeof setInterval>;
  private readonly volumeChanged = () => this.send({action: 'volume', volume: this.video.volume, muted: this.video.muted});
  private readonly video: VideoLike;
  private readonly options: WebMediaPlayerOptions;
  private readonly bridge: Bridge;

  constructor(video: VideoLike, options: WebMediaPlayerOptions, bridge: Bridge, poll = true) {
    this.video = video; this.options = options; this.bridge = bridge;
    if (typeof HTMLVideoElement !== 'undefined' && video instanceof HTMLVideoElement) this.hole = new NativeHole(video, bounds => this.bridge.surface?.(bounds, this.nativeGeneration) ?? Promise.reject(new Error('unsupported')), () => this.surfaceActive);
    video.addEventListener('volumechange', this.volumeChanged);
    // The original element remains the shell's geometry/volume owner. It never opens a decoder.
    video.pause(); video.removeAttribute('src'); video.load();
    if (poll) this.ticker = setInterval(() => { void this.tick(); }, 200);
  }
  get loaded() { return this.source?.generation ?? 0; }
  get nativeSurfaceActive() { return this.surfaceActive; }
  get settling() { return this.seeking || this.reading.state === 'loading'; }
  private emit(event: PlayerEvent) { for (const listener of [...this.listeners]) listener(event); }
  private fail(epoch: number) {
    if (this.disposed || epoch !== this.epoch || !this.source) return;
    this.reading = {...this.reading, state: 'error'};
    this.surfaceActive = false;
    this.emit({type: 'error', observation: this.reading, code: 'engine_unavailable'});
  }
  private enqueue(command: NativeCommand, epoch: number) {
    this.serial = this.serial.then(() => this.bridge.command(command)).catch(() => this.fail(epoch));
  }
  private send(command: NativeCommand) {
    if (!this.source || this.disposed) return;
    this.enqueue({...command, generation: this.nativeGeneration}, this.epoch);
  }
  load(source: PlayerSource) {
    if (this.disposed) return;
    const epoch = ++this.epoch; this.source = source; this.seeking = false;
    this.surfaceActive = false;
    this.nativeGeneration = ++nextNativeGeneration;
    this.reading = {generation: source.generation, state: 'loading', positionMs: source.startPositionMs, rate: this.reading.rate};
    this.emit({type: 'state', observation: this.reading});
    this.enqueue({action: 'load', source: {url: this.options.resolveUrl?.(source.url) ?? source.url, generation: this.nativeGeneration, mode: source.mode, startPositionMs: source.startPositionMs, autoplay: source.autoplay}}, epoch);
  }
  play() { this.send({action: 'play'}); }
  pause() { this.send({action: 'pause'}); }
  seek(positionMs: number) { if (Number.isFinite(positionMs) && positionMs >= 0) { this.seeking = true; this.send({action: 'seek', milliseconds: positionMs}); } }
  setRate(rate: number) { if (Number.isFinite(rate) && rate >= .25 && rate <= 4) { this.reading = {...this.reading, rate}; this.send({action: 'rate', rate}); } }
  selectSidecarSubtitle(trackId: string | null) {
    // Embedded libass/bitmap rendering is native. Extracted sidecars require a constrained broker;
    // do not tell the controller an unavailable sidecar is visible.
    this.reading = {...this.reading, subtitleTrackId: trackId, subtitlesCarried: trackId === null};
  }
  observe() { return this.reading; }
  onEvent(listener: (event: PlayerEvent) => void) { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; }
  onNotice(listener: (event: WebPlayerNotice) => void) { this.notices.add(listener); return () => { this.notices.delete(listener); }; }
  async tick() {
    if (this.disposed || !this.source || this.polling) return;
    this.polling = true; const epoch = this.epoch;
    try {
      await this.serial;
      const next = await this.bridge.observe();
      if (this.disposed || epoch !== this.epoch || next.generation !== this.nativeGeneration || !this.source) return;
      const active = next.nativeSurfaceActive && !['idle','loading','ended','error'].includes(next.state);
      const changed = this.reading.state !== next.state || this.surfaceActive !== active;
      this.surfaceActive = active;
      const {nativeSurfaceActive: _, ...projected} = next;
      this.reading = {...projected, generation: this.source.generation, subtitleTrackId: this.reading.subtitleTrackId, subtitlesCarried: this.reading.subtitlesCarried};
      if (changed) this.emit({type: 'state', observation: this.reading});
      if (this.seeking && !['loading','buffering'].includes(next.state)) { this.seeking = false; this.emit({type: 'seeked', observation: this.reading}); }
      this.emit({type: 'time', observation: this.reading});
    } catch { this.fail(epoch); } finally { this.polling = false; }
  }
  unload() {
    if (this.source) this.send({action: 'stop'});
    ++this.epoch; this.source = undefined; this.seeking = false; this.surfaceActive = false;
    this.reading = {generation: 0, state: 'idle', positionMs: 0, rate: this.reading.rate};
  }
  suspend() { this.pause(); }
  resume() { /* A route recovery loads a fresh server capability; never reuse an expired URL. */ }
  dispose() {
    if (this.disposed) return;
    this.unload(); this.disposed = true;
    if (this.ticker) clearInterval(this.ticker);
    this.video.removeEventListener('volumechange', this.volumeChanged);
    this.hole?.dispose();
    this.listeners.clear(); this.notices.clear();
  }
}
