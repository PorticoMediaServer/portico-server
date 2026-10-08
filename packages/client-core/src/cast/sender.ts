/**
 * The Portico Cast sender, one implementation for every platform. The platform finds a Cast
 * device and opens a session (web: the Chrome Cast SDK picker; Apple: `PorticoCast` over a
 * GCKGenericChannel) and hands this controller a `CastTransport` for it. The controller does the
 * rest: pairing through a server-issued code, loading the title with its display metadata,
 * transport commands, track choice, the takeover prompt and being replaced by another sender.
 */
import type {MessageId} from '../../../i18n/src/index.ts';
import {serviceProblem, serviceText} from '../presentation/service-text.ts';
import {CAST_PAIR_TIMEOUT_MS, CAST_PROTOCOL_VERSION, CAST_TAKEOVER_WINDOW_MS, castMetadataFields, parseCastEvent, type CastCommand, type CastEvent, type CastMetadata, type CastTrack} from './protocol.ts';

/** One open session with one receiver, over the `urn:x-cast:tv.getportico.cast` namespace. */
export interface CastTransport {
  /** The device's friendly name ("Living Room TV"). */
  readonly deviceName: string;
  send(message: CastCommand): Promise<void>;
  /** Receiver messages, as the SDK delivers them (JSON string or decoded object). Returns an unsubscribe. */
  onMessage(listener: (raw: unknown) => void): () => void;
  /** The session ended from the device or the SDK side. Returns an unsubscribe. */
  onSessionEnd(listener: () => void): () => void;
  /** Leave the session; `stopReceiver` also closes Portico on the TV. */
  end(stopReceiver: boolean): void;
}

export type CastApi = {request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T>};

export type CastPhase = 'off' | 'connecting' | 'pairing' | 'waiting' | 'casting' | 'error' | 'replaced';

export type CastSnapshot = Readonly<{
  /** The platform can cast from here (set by the platform once its SDK and the server agree). */
  available: boolean;
  phase: CastPhase;
  deviceName?: string;
  itemId?: string;
  title?: string;
  positionSeconds: number;
  durationSeconds: number;
  paused: boolean;
  audioTracks: readonly CastTrack[];
  textTracks: readonly CastTrack[];
  /** A catalogue message in the viewer's language, and its ID. */
  error?: string;
  errorId?: MessageId;
  /** Someone else asked the TV to play; this controller answers (30 s without an answer is no). */
  takeover?: Readonly<{requester: string}>;
}>;

export type CastControllerOptions = Readonly<{
  api: CastApi;
  /** The server origin the TV should redeem its code against. Must be HTTPS: a Cast device loads nothing else. */
  origin: string;
  /** How this sender introduces itself on the TV ("Chrome on Mac", "Sam’s iPhone"). No account details. */
  displayName: string;
  setTimer?: (fn: () => void, ms: number) => unknown;
  clearTimer?: (timer: unknown) => void;
}>;

class CastFailure extends Error {
  readonly messageId: MessageId;
  constructor(id: MessageId) { super(id); this.messageId = id; }
}

const idle: CastSnapshot = Object.freeze({available: false, phase: 'off', positionSeconds: 0, durationSeconds: 0, paused: false, audioTracks: Object.freeze([]), textTracks: Object.freeze([])});

export class CastController {
  private state: CastSnapshot = idle;
  private listeners = new Set<() => void>();
  private api: CastApi;
  private origin: string;
  private displayName: string;
  private transport?: CastTransport;
  private detach: (() => void)[] = [];
  private waiting?: {resolve(): void; reject(e: Error): void};
  private adopting = false;
  private adoptTimer?: unknown;
  private settleAdoption?: (ok: boolean) => void;
  private takeoverTimer?: unknown;
  private setTimer: (fn: () => void, ms: number) => unknown;
  private clearTimer: (timer: unknown) => void;
  private disposed = false;

  constructor(options: CastControllerOptions) {
    if (!/^https:\/\//.test(options.origin)) throw new Error('Cast needs an HTTPS server origin.');
    this.api = options.api;
    this.origin = new URL(options.origin).origin;
    this.displayName = options.displayName.slice(0, 64);
    this.setTimer = options.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
    this.clearTimer = options.clearTimer ?? (t => clearTimeout(t as ReturnType<typeof setTimeout>));
  }

  getSnapshot = (): CastSnapshot => this.state;
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
  private publish(patch: Partial<CastSnapshot>) {
    if (this.disposed) return;
    this.state = Object.freeze({...this.state, ...patch});
    for (const fn of [...this.listeners]) fn();
  }

  /** The platform's verdict on whether Cast can be offered here. */
  setAvailable(available: boolean) { this.publish({available}); }
  /** The platform's device picker is open ("Choose a device…"). */
  connecting(title?: string) { this.publish({phase: 'connecting', title, error: undefined, errorId: undefined, takeover: undefined}); }
  /** The picker closed without a device: not an error. */
  cancelConnecting() { if (this.state.phase === 'connecting') this.publish({phase: 'off'}); }

  /** Pair with the TV on `transport` and start `itemId` there. True once the TV has the title. */
  async start(transport: CastTransport, itemId: string, meta: CastMetadata, startSeconds: number): Promise<boolean> {
    this.release(false);
    this.transport = transport;
    this.detach.push(transport.onMessage(raw => { const e = parseCastEvent(raw); if (e) this.receive(e); }));
    this.detach.push(transport.onSessionEnd(() => this.sessionEnded()));
    this.publish({phase: 'pairing', deviceName: transport.deviceName, title: meta.title, itemId: undefined, error: undefined, errorId: undefined, takeover: undefined, audioTracks: [], textTracks: []});
    try {
      const issued = await this.api.request<{bootstrap?: {code?: unknown}}>('/v1/cast/bootstrap', 'POST', {protocolVersion: CAST_PROTOCOL_VERSION, displayName: transport.deviceName.slice(0, 64)});
      const code = issued?.bootstrap?.code;
      if (typeof code !== 'string' || !code) throw new CastFailure('cast.error.noCode');
      await this.exchange({type: 'pair', code, origin: this.origin, displayName: this.displayName});
      await transport.send({type: 'load', itemId, startSeconds: Math.max(0, startSeconds), ...castMetadataFields(meta)});
      this.publish({phase: 'casting', itemId, positionSeconds: Math.max(0, startSeconds), paused: false});
      return true;
    } catch (e) {
      this.release(true);
      const id: MessageId = e instanceof CastFailure ? e.messageId : 'cast.error.unreachable';
      this.publish({phase: 'error', takeover: undefined, errorId: id, error: e instanceof CastFailure ? serviceText(id) : serviceProblem(e, 'playback', id)});
      return false;
    }
  }

  /**
   * CAST-05: a session the platform resumed on its own (a page reload while the TV plays) is
   * adopted when the TV answers a status request within `waitMs`: the controller shows it as
   * casting and can control and stop it. Otherwise it is left alone (never ended, never paired
   * again). `describe` names the title from its item id. Resolves true once adopted.
   */
  adopt(transport: CastTransport, describe?: (itemId: string) => Promise<string | undefined>, waitMs = 5000): Promise<boolean> {
    if (this.transport || this.disposed) return Promise.resolve(false);
    this.transport = transport;
    return new Promise<boolean>(resolve => {
      const give = (ok: boolean) => { if (!this.adopting) return; this.clearTimer(this.adoptTimer); this.adopting = false; this.settleAdoption = undefined; resolve(ok); };
      this.adopting = true;
      this.settleAdoption = give;
      this.detach.push(transport.onMessage(raw => {
        const e = parseCastEvent(raw);
        if (!e) return;
        if (!this.adopting) { this.receive(e); return; }
        if (e.type === 'status' && e.itemId) {
          this.publish({phase: 'casting', deviceName: transport.deviceName, itemId: e.itemId, title: undefined, error: undefined, errorId: undefined, takeover: undefined, positionSeconds: e.positionSeconds, durationSeconds: e.durationSeconds, paused: e.paused, audioTracks: e.audioTracks, textTracks: e.textTracks});
          give(true);
          void describe?.(e.itemId).then(title => { if (title && this.state.phase === 'casting' && this.state.itemId === e.itemId) this.publish({title}); }, () => {});
        } else if (e.type === 'pair-required' || e.type === 'replaced' || e.type === 'busy') { this.release(false, false); give(false); }
      }));
      this.detach.push(transport.onSessionEnd(() => { if (this.adopting) { this.release(false, false); give(false); } else this.sessionEnded(); }));
      this.adoptTimer = this.setTimer(() => { if (this.transport === transport && this.state.phase !== 'casting') this.release(false, false); give(false); }, waitMs);
      this.send({type: 'status'});
    });
  }

  private exchange(body: CastCommand): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      const timer = this.setTimer(() => { this.waiting = undefined; reject(new CastFailure('cast.error.noAnswer')); }, CAST_PAIR_TIMEOUT_MS);
      this.waiting = {resolve: () => { this.clearTimer(timer); resolve(); }, reject: e => { this.clearTimer(timer); reject(e); }};
      this.transport!.send(body).catch(() => { this.waiting?.reject(new CastFailure('cast.error.noAnswer')); this.waiting = undefined; });
    });
  }

  private receive(e: CastEvent) {
    switch (e.type) {
      case 'paired': this.waiting?.resolve(); this.waiting = undefined; return;
      // The TV is asking the person watching; keep waiting (the pairing timeout allows for it).
      case 'pair-pending': if (this.state.phase === 'pairing') this.publish({phase: 'waiting'}); return;
      case 'pair-failed': this.waiting?.reject(new CastFailure(e.code === 'tv_busy' ? 'cast.error.tvBusy' : 'cast.error.refused')); this.waiting = undefined; return;
      case 'busy': this.fail('cast.error.busy'); return;
      case 'pair-required': this.fail('cast.error.lostPairing'); return;
      case 'load-failed': this.fail('cast.error.loadFailed'); return;
      case 'takeover-requested': {
        this.clearTimer(this.takeoverTimer);
        this.publish({takeover: Object.freeze({requester: e.requester || serviceText('cast.someone')})});
        // The TV treats 30 s without an answer as no; drop the prompt with it.
        this.takeoverTimer = this.setTimer(() => this.publish({takeover: undefined}), CAST_TAKEOVER_WINDOW_MS);
        return;
      }
      case 'replaced':
        this.clearTimer(this.takeoverTimer);
        this.release(false);
        this.publish({phase: 'replaced', takeover: undefined});
        return;
      case 'status':
        if (this.state.phase === 'casting') this.publish({positionSeconds: e.positionSeconds, durationSeconds: e.durationSeconds, paused: e.paused, audioTracks: e.audioTracks, textTracks: e.textTracks});
        return;
    }
  }

  private fail(id: MessageId) { this.publish({phase: 'error', errorId: id, error: serviceText(id)}); }

  private sessionEnded() {
    this.release(false, false);
    if (this.state.phase !== 'error' && this.state.phase !== 'replaced') this.publish({phase: 'off', itemId: undefined, title: undefined, deviceName: undefined, takeover: undefined});
  }

  /** Detach from the transport; `stopReceiver` closes Portico on the TV. */
  private release(stopReceiver: boolean, end = true) {
    for (const off of this.detach.splice(0)) off();
    this.waiting = undefined;
    const t = this.transport;
    this.transport = undefined;
    if (t && end) t.end(stopReceiver);
  }

  private send(message: CastCommand) { void this.transport?.send(message).catch(() => {}); }

  /** The controller's answer to `takeover-requested`. */
  answerTakeover(allow: boolean) {
    this.clearTimer(this.takeoverTimer);
    this.send({type: 'takeover', allow});
    this.publish({takeover: undefined});
  }
  play() { this.send({type: 'play'}); this.publish({paused: false}); }
  pause() { this.send({type: 'pause'}); this.publish({paused: true}); }
  toggle() { if (this.state.paused) this.play(); else this.pause(); }
  seek(positionSeconds: number) { const p = Math.max(0, positionSeconds); this.send({type: 'seek', positionSeconds: p}); this.publish({positionSeconds: p}); }
  setAudioTrack(trackId: number) { if (Number.isSafeInteger(trackId)) this.send({type: 'audio', trackId}); }
  /** `null` turns subtitles off. */
  setSubtitleTrack(trackId: number | null) { this.send({type: 'subtitles', trackIds: trackId === null ? [] : [trackId]}); }
  stop() {
    this.clearTimer(this.takeoverTimer);
    this.send({type: 'stop'});
    this.release(true);
    this.publish({phase: 'off', itemId: undefined, title: undefined, deviceName: undefined, error: undefined, errorId: undefined, takeover: undefined});
  }
  dismiss() { this.publish({phase: 'off', error: undefined, errorId: undefined}); }
  dispose() { this.clearTimer(this.takeoverTimer); this.settleAdoption?.(false); this.release(false); this.disposed = true; this.listeners.clear(); }
}
