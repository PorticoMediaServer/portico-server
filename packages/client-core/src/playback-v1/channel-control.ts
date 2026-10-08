/**
 * Live TV and Library Channels on a Playback v1 session (spec §11, §18.6; Plan — Client Playback
 * Migration §10, B6). The same surface as the /v2 `ChannelControl` (open, pause, resume, seek, go
 * live, retry, observe, close), so `PlaybackService` and the players don't change: each read of the
 * v1 session is presented as the channel projection the service already applies.
 *
 * - Start: `POST /v1/playback/sessions {channelId, channelGeneration, state}`. A start with every
 *   tuner in use is `409 no_tuner_available`, said as such.
 * - Join: once the stream exists, a go-live seek (`PATCH seek {live: true}`) that the server
 *   resolves to a position in the window: the engine attaches there.
 * - Controls: `PATCH` with the session's revision (`state`; `seek {positionMs}` or `{live: true}`);
 *   play on a failed source is the server's retry.
 * - Reports: the timeline (§6) is the lease and the observation. A report after the engine reached
 *   a seek acknowledges it (the server compares the position with the target). One report at least
 *   every 10 s playing and 30 s paused keeps the lease.
 * - Stop: `DELETE`. A new channel waits for the previous stop, so a one-tuner source isn't refused.
 */
import type {ChannelDesired, ChannelProjection, ChannelReference} from '../playback/linear-protocol.ts';
import {channelMessage} from '../playback/linear-protocol.ts';
import {PlaybackApiError, call, enc, idempotencyKey, type V1Http} from './http.ts';
import {serviceText} from '../presentation/service-text.ts';

export type V1ChannelOptions = Readonly<{http: V1Http; key?: () => string; now?: () => number; pollMs?: number}>;

/** What the service applies: the playback id, whether the start can't proceed, and the projection. */
export type ChannelView = Readonly<{playbackId: string; status?: string; linear: ChannelProjection}>;
export type ChannelViewSink = {state: (value: ChannelView) => void; error: (message: string, terminal: boolean) => void};

/** The service's channel control, whichever protocol (the /v2 `ChannelControl` or this). */
export interface ChannelPlayback {
  open(channel: ChannelReference, sink: ChannelViewSink): Promise<void>;
  recover(): Promise<void>;
  pause(): void;
  resume(): void;
  seek(seconds: number): void;
  goLive(): void;
  retry(): void;
  observe(seconds: number, seekId?: string | null, errorCode?: string | null): Promise<boolean>;
  close(): void;
}

/** Whether the server plays channels as v1 sessions (`features.playback_v1_channels`, spec §18.6). */
export function serverSupportsV1Channels(capabilities: unknown): boolean {
  const features = obj(capabilities) && obj(capabilities.features) ? capabilities.features : undefined;
  return features?.playback_v1_channels === 'enabled';
}

/** A v1 channelId: `live:<sourceId>:<channelId>` or `library:<channelId>` (spec §18.6). */
export function v1ChannelId(ref: ChannelReference): string {
  return ref.kind === 'live-source' ? `live:${ref.sourceId}:${ref.channelId}` : `library:${ref.channelId}`;
}

type Programme = Readonly<{id: string; itemId?: string; title: string; startMs: number; endMs: number}>;
type Linear = Readonly<{
  name: string; programme?: Programme; next?: Programme; originMs: number; windowStartMs: number; windowEndMs: number; liveEdgeMs: number;
  state: string; errorCode?: string; seek?: Readonly<{id: string; live?: boolean; positionMs?: number; acknowledged: boolean; error?: string}>; confirmedPositionMs?: number;
}>;
type Session = Readonly<{id: string; revision: string; state: string; presentation: Readonly<{generation: number; url: string; linear?: Linear}>; end?: Readonly<{reason: string; message?: string}>}>;

const obj = (v: unknown): v is Record<string, any> => !!v && typeof v === 'object' && !Array.isArray(v);
const int = (v: unknown): v is number => typeof v === 'number' && Number.isSafeInteger(v);
const us = (ms: number) => String(Math.round(ms) * 1000);

function parseProgramme(v: unknown): Programme | undefined {
  if (v === undefined) return undefined;
  if (!obj(v) || typeof v.id !== 'string' || typeof v.title !== 'string' || !int(v.startMs) || !int(v.endMs)) throw new Error('Unreadable channel programme.');
  return v as Programme;
}

/** Reads a v1 channel session (the fields this control uses). */
export function parseChannelSession(v: unknown): Session {
  if (!obj(v) || typeof v.id !== 'string' || typeof v.revision !== 'string' || typeof v.state !== 'string' || !obj(v.presentation) || !int(v.presentation.generation) || typeof v.presentation.url !== 'string') throw new Error('Unreadable channel session.');
  const l = v.presentation.linear;
  if (l !== undefined) {
    if (!obj(l) || typeof l.name !== 'string' || !int(l.originMs) || !int(l.windowStartMs) || !int(l.windowEndMs) || !int(l.liveEdgeMs) || typeof l.state !== 'string') throw new Error('Unreadable channel clock.');
    parseProgramme(l.programme); parseProgramme(l.next);
    if (l.seek !== undefined && (!obj(l.seek) || typeof l.seek.id !== 'string' || typeof l.seek.acknowledged !== 'boolean' || l.seek.positionMs !== undefined && !int(l.seek.positionMs))) throw new Error('Unreadable channel seek.');
    if (l.confirmedPositionMs !== undefined && !int(l.confirmedPositionMs)) throw new Error('Unreadable channel position.');
  }
  return v as Session;
}

/** The v1 session as the channel projection `PlaybackService.applyChannel` reads. */
export function channelView(s: Session, channel: ChannelReference): ChannelView {
  const l = s.presentation.linear;
  const generation = s.presentation.generation > 0 ? String(s.presentation.generation) : '';
  const programme = (p?: Programme) => (p ? Object.freeze({id: p.id, itemId: p.itemId ?? '', title: p.title, startMs: p.startMs, endMs: p.endMs}) : null);
  const seek = l?.seek && l.seek.positionMs !== undefined ? Object.freeze({seekCommandId: l.seek.id, bufferGeneration: generation, kind: 'position' as const, positionUs: us(l.seek.positionMs)}) : null;
  const desired: ChannelDesired = Object.freeze({state: s.state === 'paused' ? 'paused' : 'playing', seek, retryRequestId: null});
  const linear: ChannelProjection = Object.freeze({
    channel, name: l?.name ?? '', programme: programme(l?.programme), next: programme(l?.next), desired,
    media: Object.freeze({streamUrl: s.presentation.url, bufferGeneration: generation, presentationGeneration: generation, originMs: l?.originMs ?? 0, windowRevision: '1',
      windowStartUs: us(l?.windowStartMs ?? 0), windowEndUs: us(l?.windowEndMs ?? 0), liveEdgeUs: us(l?.liveEdgeMs ?? 0), gaps: [], state: l?.state ?? 'preparing', errorCode: l?.errorCode ?? '', logoUrl: ''}),
    overlay: {enabled: false, corner: 'top-right', sizePercent: 8, insetPercent: 3, treatment: 'original'},
    confirmedPositionUs: l?.confirmedPositionMs !== undefined ? us(l.confirmedPositionMs) : null,
    acknowledgedSeekCommandId: l?.seek?.acknowledged ? l.seek.id : null,
    seekError: l?.seek?.error ?? '',
  });
  return Object.freeze({playbackId: s.id, status: l?.state === 'recoverable' ? 'recoverable' : undefined, linear});
}

type Active = {serial: number; channel: ChannelReference; id?: string; session?: Session; joined: boolean; seq: number; lastReport: number; timer?: ReturnType<typeof setTimeout>; tail: Promise<unknown>; closed: boolean};

const message = (e: unknown): string => {
  if (e instanceof PlaybackApiError) {
    if (e.code === 'no_tuner_available') return channelMessage('tuner_busy');
    if (e.code === 'not_found') return 'This channel isn’t available.';
    if (e.code === 'seek_unavailable') return channelMessage('window_expired');
    if (e.code === 'stream_limit_reached') return 'This account is already playing as many streams as it is allowed.';
    if (e.code === 'channel_unavailable') return 'This channel can’t be played right now. Try again in a moment.';
    if (e.code === 'session_ended') return 'This channel playback has ended. Select the channel again.';
  }
  return 'Channel playback could not connect. Check the server and retry.';
};
/** Why a channel session ended, in words: an administrator's message, a profile without Live TV, or the generic end. */
export const channelEndMessage = (end: Session['end']): string => {
  if (end?.reason === 'terminated' && end.message) return end.message;
  if (end?.reason === 'feature_restricted') return serviceText('live.offForProfile');
  return 'This channel playback has ended. Select the channel again.';
};
const terminal = (e: unknown) => e instanceof PlaybackApiError && [401, 403, 404, 410].includes(e.status) || e instanceof PlaybackApiError && e.code === 'no_tuner_available';

export class V1ChannelControl implements ChannelPlayback {
  private readonly http: V1Http;
  private readonly key: () => string;
  private readonly now: () => number;
  private readonly pollMs: number;
  private serial = 0;
  private active?: Active;
  private sink?: ChannelViewSink;
  private wanted: 'playing' | 'paused' = 'playing';
  private stopping: Promise<unknown> = Promise.resolve();

  constructor(options: V1ChannelOptions) {
    this.http = options.http;
    this.key = options.key ?? (() => idempotencyKey());
    this.now = options.now ?? Date.now;
    this.pollMs = options.pollMs ?? 1000;
  }

  /** Nothing to recover: a v1 session a crash left behind lapses with its lease. */
  recover(): Promise<void> { return Promise.resolve(); }

  private current(a: Active) { return this.active === a && !a.closed; }
  private enqueue<T>(a: Active, work: () => Promise<T>): Promise<T> {
    const result = a.tail.then(work);
    a.tail = result.catch(() => {});
    return result;
  }

  async open(channel: ChannelReference, sink: ChannelViewSink): Promise<void> {
    this.close();
    this.sink = sink;
    this.wanted = 'playing';
    const a: Active = {serial: ++this.serial, channel, joined: false, seq: 0, lastReport: 0, tail: Promise.resolve(), closed: false};
    this.active = a;
    try {
      await this.stopping.catch(() => {});
      if (!this.current(a)) return;
      const r = await call(this.http, {method: 'POST', path: '/v1/playback/sessions', headers: {'Idempotency-Key': this.key()}, body: {channelId: v1ChannelId(channel), channelGeneration: channel.generation, state: 'playing'}});
      const s = parseChannelSession(r.body);
      if (!this.current(a)) { void this.stop(s.id); return; }
      a.id = s.id;
      this.accept(a, s);
      this.schedule(a);
    } catch (e) {
      if (this.current(a)) { this.active = undefined; sink.error(message(e), true); }
    }
  }

  private accept(a: Active, s: Session) {
    if (!this.current(a)) return;
    if (s.state === 'ended' || s.end) {
      this.active = undefined;
      this.sink?.error(channelEndMessage(s.end), true);
      return;
    }
    a.session = s;
    // The first join is a go-live seek the server resolves to a position in the window.
    if (!a.joined && s.presentation.url && s.presentation.generation > 0) {
      a.joined = true;
      void this.patch(a, {state: this.wanted, seek: {id: this.key(), positionMs: 0, live: true}});
      return;
    }
    this.sink?.state(channelView(s, a.channel));
  }

  private async read(a: Active) {
    if (!a.id || !this.current(a)) return;
    const r = await call(this.http, {method: 'GET', path: `/v1/playback/sessions/${enc(a.id)}`});
    this.accept(a, parseChannelSession(r.body));
  }

  private schedule(a: Active) {
    if (!this.current(a)) return;
    a.timer = setTimeout(() => void this.tick(a), this.pollMs);
    (a.timer as {unref?: () => void} | undefined)?.unref?.();
  }

  private async tick(a: Active) {
    if (!this.current(a)) return;
    await this.enqueue(a, async () => {
      try {
        await this.read(a);
        // The lease: report even when the engine is quiet (paused, preparing).
        const every = a.session?.state === 'paused' ? 30_000 : 10_000;
        if (this.now() - a.lastReport >= every) await this.report(a, undefined);
      } catch (e) {
        if (this.current(a)) { this.sink?.error(message(e), terminal(e)); if (terminal(e)) this.active = undefined; }
      }
    });
    this.schedule(a);
  }

  private async patch(a: Active, body: Record<string, unknown>) {
    return this.enqueue(a, async () => {
      if (!a.id || !a.session || !this.current(a)) return;
      try {
        const r = await call(this.http, {method: 'PATCH', path: `/v1/playback/sessions/${enc(a.id)}`, headers: {'If-Match': `"${a.session.revision}"`}, body});
        this.accept(a, parseChannelSession(r.body));
      } catch (e) {
        if (!this.current(a)) return;
        if (e instanceof PlaybackApiError && e.status === 412) {
          // Another change landed first: take the current session and apply ours on it.
          const current = e.current ? parseChannelSession(e.current) : undefined;
          if (current) {
            a.session = current;
            const r = await call(this.http, {method: 'PATCH', path: `/v1/playback/sessions/${enc(a.id)}`, headers: {'If-Match': `"${current.revision}"`}, body});
            this.accept(a, parseChannelSession(r.body));
            return;
          }
        }
        try { await this.read(a); } catch { /* the next tick reads again */ }
        this.sink?.error(message(e), terminal(e));
      }
    });
  }

  private async report(a: Active, positionSeconds: number | undefined): Promise<boolean> {
    if (!a.id || !a.session || !this.current(a)) return false;
    const s = a.session;
    const position = positionSeconds ?? (s.presentation.linear?.confirmedPositionMs ?? 0) / 1000;
    a.lastReport = this.now();
    await call(this.http, {method: 'POST', path: `/v1/playback/sessions/${enc(a.id)}/timeline`, body: {seq: ++a.seq, generation: s.presentation.generation, state: this.wanted, positionMs: Math.max(0, Math.round(position * 1000))}});
    return true;
  }

  pause() { this.wanted = 'paused'; const a = this.active; if (a) void this.patch(a, {state: 'paused'}); }
  resume() { this.wanted = 'playing'; const a = this.active; if (a) void this.patch(a, {state: 'playing'}); }
  seek(seconds: number) {
    const a = this.active;
    if (!a || !Number.isFinite(seconds)) return;
    void this.patch(a, {state: this.wanted, seek: {id: this.key(), positionMs: Math.max(0, Math.round(seconds * 1000))}});
  }
  goLive() {
    this.wanted = 'playing';
    const a = this.active;
    if (a) void this.patch(a, {state: 'playing', seek: {id: this.key(), positionMs: 0, live: true}});
  }
  /** Play on a failed source is the server's retry. */
  retry() { const a = this.active; if (a) void this.patch(a, {state: 'playing'}); }

  /** Where the engine is (a seek it reached, or ordinary progress); coalesced to one a second. */
  observe(seconds: number, seekId: string | null = null, _errorCode: string | null = null): Promise<boolean> {
    const a = this.active;
    if (!a?.id || !a.session || !this.current(a) || !Number.isFinite(seconds)) return Promise.resolve(false);
    if (!seekId && this.now() - a.lastReport < 1000) return Promise.resolve(false);
    return this.enqueue(a, async () => {
      if (!this.current(a) || seekId && a.session?.presentation.linear?.seek?.id !== seekId) return false;
      try {
        const ok = await this.report(a, seconds);
        if (seekId) await this.read(a);
        return ok;
      } catch (e) {
        if (this.current(a) && seekId) this.sink?.error(message(e), terminal(e));
        return false;
      }
    });
  }

  private stop(id: string): Promise<unknown> {
    const work = call(this.http, {method: 'DELETE', path: `/v1/playback/sessions/${enc(id)}`}, [200, 204, 404, 410]).catch(() => {});
    this.stopping = Promise.all([this.stopping.catch(() => {}), work]);
    return work;
  }

  close() {
    ++this.serial;
    const a = this.active;
    this.active = undefined;
    if (!a) return;
    a.closed = true;
    if (a.timer) clearTimeout(a.timer);
    if (a.id) void this.stop(a.id);
  }
}
