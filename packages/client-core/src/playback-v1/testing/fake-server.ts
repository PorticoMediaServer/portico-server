/**
 * A fake Portico playback server built from `Spec — Playback Protocol v1.md` (§3–§9, §14, §15, §16)
 * and the API style guide, for client tests before lane C's server exists. In memory; enforces
 * what a client must live with:
 * - idempotent start (422 idempotency_key_reused on a reused key with a different body; spec §17.1), 202 preparation;
 * - `If-Match` on sessions and queues (412 `revision_mismatch` with `current`; §16.2);
 * - generations bump only for byte changes; timeline ignores lower `seq` or other generations (§16.3);
 * - idempotent `DELETE`; lease expiry (§16.4); `replacesSessionId`;
 * - queues from selectors with lazily generated keys (a 10M-entry queue costs nothing), windows
 *   ≤ 200 (§16.8), seeded shuffle with the current entry first, move/remove as tombstones;
 * - device commands and transfers (commit on the target's first playing report; §16.9);
 * - `/v1/events` long-poll (parks until an event or `waitSeconds`) and a stream opener, with a
 *   bounded ring (`stream.resync` for cursors older than it) and `stream.closed`;
 * - admin Now Playing and terminate.
 * Not for production.
 */
import {ShufflePermutation} from '../shuffle.ts';

export type FakeResponse = Readonly<{status: number; headers: Readonly<Record<string, string>>; body?: unknown}>;
export type FakeRequest = Readonly<{method: string; path: string; headers?: Readonly<Record<string, string>>; body?: unknown; signal?: AbortSignal}>;
export type FakeEvent = {id: string; type: string; at: string; resource?: {kind: string; id: string}; revision?: string; data?: unknown};

type Session = {
  id: string; revision: number; state: 'playing' | 'paused'; kind: 'vod' | 'audio' | 'channel'; itemId: string; versionId: string;
  audio?: unknown; subtitles?: unknown; quality: unknown; generation: number; positionMs: number; deviceId: string;
  lastSeq: number; lastActivity: number; ended: boolean; reports: Record<string, unknown>[];
  queue?: {queueId: string; entryId: string}; transferId?: string; startedAt: number; terminatedMessage?: string; endReason?: string;
};
type Segment = {id: string; kind: 'items' | 'selector'; label: string; count: number; keyAt: (i: number) => string; removals: Set<number>};
type Queue = {id: string; revision: number; deviceId: string; segments: Segment[]; current: number; repeat: 'off' | 'one' | 'all'; shuffle?: {seed: number; lap: number; domain: number; first: number}; sessionId?: string;
  /** §18: the prepared next entry (private until committed), and committed answers by token. */
  prepared?: {token: string; sessionId: string; sessionGeneration: number; nextPosition: number; nextSessionId: string; expiresAt: number; revision: number};
  committed?: Map<string, FakeResponse>};;
type Transfer = {id: string; sourceId: string; targetDeviceId: string; committed: boolean};

export type FakeServerOptions = Readonly<{
  now?: () => number;
  reportEveryMs?: number;
  leaseMs?: number;
  /** The device making requests (the "current" device). */
  deviceId?: string;
  devices?: readonly {id: string; name: string; form: string}[];
  /** Refuse a start (admission, spec §5.1): return an error code, or undefined to allow. */
  admit?: (body: Record<string, unknown>) => string | undefined;
  /** How many 202 responses a start key gets before 201 (preparation). */
  prepareRounds?: (body: Record<string, unknown>) => number;
  /** Entries a container selector expands to (default 12). Keys are generated, never stored. */
  containerSize?: (kind: string, id: string) => number;
  /** Events kept for resume (default 1000). */
  ring?: number;
  setTimer?: (fn: () => void, ms: number) => unknown;
  clearTimer?: (t: unknown) => void;
  /** Items that play as audio sessions with a render plan (§18.1; default: ids starting "song"). */
  audioItem?: (itemId: string) => boolean;
  /** An audio item's kind on the wire (`song`, `audiobook_file`…); default `song` for audio, else `movie`. */
  itemKind?: (itemId: string) => string | undefined;
  /** The viewer the listening endpoints answer for (their `scope`). */
  viewer?: Readonly<{serverId: string; authority: string; accountId: string; profileId: string}>;
  /** The listening preference `autoplayNext` (default true). */
  autoplayNext?: boolean;
  /** An item's file and facts (e.g. a `fixtures/audio-gapless/` file and its sidecar),
   * served with Range at the plan's `url`. Without one, a synthetic three-minute FLAC plan. */
  audioSource?: (itemId: string) => FakeAudioSource | undefined;
  /** Choose the mode from the device's published `audioDecode` (spec §3, §18.1): `direct`
   * for a declared pair within its limits, else `converted` to FLAC when flac/flac is declared,
   * else `unavailable`. Off by default (every plan is `direct`, as before). */
  modeFromCapabilities?: boolean;
}>;

export type FakeAudioSource = Readonly<{
  bytes: Uint8Array; container: string; codec: string; sampleRate: number; channels: number; durationFrames: number;
  trim: Readonly<{startFrames: number; endFrames: number; source: string}>;
  gain?: Readonly<{trackDb?: number; albumDb?: number; trackPeak?: number; albumPeak?: number; source: string}>;
}>;

/** Strong quoted ETag of the revision; `If-Match` accepts either form (spec §17). */
const etag = (revision: number) => `"${revision}"`;
const unquote = (v: string) => v.replace(/^W\//, '').replace(/^"(.*)"$/, '$1');
const BYTES_CHANGING = ['versionId', 'partIndex', 'audio', 'subtitles', 'quality'] as const;
const WINDOW_MAX = 200;

export class FakePlaybackServer {
  readonly capabilities = new Map<string, unknown>();
  /** §18 counters for tests. */
  preparations = 0;
  commits = 0;
  readonly commands: {deviceId: string; command: Record<string, unknown>}[] = [];
  private sessions = new Map<string, Session>();
  private keys = new Map<string, {body: string; status: number; response: unknown; rounds: number}>();
  private queues = new Map<string, Queue>();
  private transfers = new Map<string, Transfer>();
  private events: FakeEvent[] = [];
  private eventSeq = 0;
  private waiters = new Set<() => void>();
  private streams = new Set<{push: (e: FakeEvent) => void; close: (reconnectAfterMs: number) => void; fail: (status: number) => void}>();
  private streamRefusal?: number;
  private failures: (number | 'offline')[] = [];
  private nextId = 0;
  private o: FakeServerOptions & {reportEveryMs: number; leaseMs: number; deviceId: string; ring: number};
  private now: () => number;
  private setTimer: (fn: () => void, ms: number) => unknown;
  private clearTimer: (t: unknown) => void;

  constructor(options: FakeServerOptions = {}) {
    this.o = {reportEveryMs: 10_000, leaseMs: 120_000, deviceId: 'this-device', ring: 1000, ...options};
    this.now = options.now ?? Date.now;
    this.setTimer = options.setTimer ?? ((fn, ms) => setTimeout(fn, ms));
    this.clearTimer = options.clearTimer ?? (t => clearTimeout(t as ReturnType<typeof setTimeout>));
  }

  // ── Test controls ───────────────────────────────────────────────
  failNext(...outcomes: (number | 'offline')[]): void { this.failures.push(...outcomes); }
  session(id: string): Readonly<Session> | undefined { const s = this.sessions.get(id); if (s) this.expire(s); return s; }
  queue(id: string): Readonly<Queue> | undefined { return this.queues.get(id); }
  eventLog(): readonly FakeEvent[] { return this.events; }
  /** As another device (a TV, a phone) for the next requests' `deviceId`. */
  asDevice(deviceId: string): FakePlaybackServer { const view = Object.create(this) as FakePlaybackServer; (view as unknown as {o: FakePlaybackServer['o']}).o = {...this.o, deviceId}; return view; }
  /** Publish an event (tests may also publish their own). */
  publish(type: string, resource?: {kind: string; id: string}, revision?: string, data?: unknown): FakeEvent {
    const e: FakeEvent = {id: String(++this.eventSeq), type, at: new Date(this.now()).toISOString(), ...(resource ? {resource} : {}), ...(revision ? {revision} : {}), ...(data !== undefined ? {data} : {})};
    this.events.push(e);
    if (this.events.length > this.o.ring) this.events.shift();
    for (const w of [...this.waiters]) w();
    for (const s of [...this.streams]) s.push(e);
    return e;
  }
  /** End every open stream with `stream.closed`. */
  closeStreams(reconnectAfterMs = 0): void { for (const s of [...this.streams]) s.close(reconnectAfterMs); }
  /** Refuse (or fail) streams with an HTTP status, e.g. 409 `stream_exists`. `undefined` allows them again. */
  refuseStreams(status: number | undefined): void { this.streamRefusal = status; if (status) for (const s of [...this.streams]) s.fail(status); }

  /** A `StreamOpener` over this fake (what a platform's SSE adapter provides). */
  openStream = (lastEventId: string | undefined, onEvent: (e: {id?: string; event?: string; data: string}) => void, signal: AbortSignal): Promise<void> =>
    new Promise((resolve, reject) => {
      if (this.streamRefusal) { reject(Object.assign(new Error('stream refused'), {status: this.streamRefusal})); return; }
      const send = (e: FakeEvent) => onEvent({id: e.id, event: e.type, data: JSON.stringify(e)});
      for (const e of this.since(lastEventId)) send(e);
      const handle = {
        push: send,
        close: (after: number) => { send({id: String(++this.eventSeq), type: 'stream.closed', at: new Date(this.now()).toISOString(), data: {reconnectAfterMs: after}}); },
        fail: (status: number) => { this.streams.delete(handle); reject(Object.assign(new Error('stream failed'), {status})); },
      };
      this.streams.add(handle);
      signal.addEventListener('abort', () => { this.streams.delete(handle); resolve(); }, {once: true});
    });

  /** Events after a cursor; a cursor older than the ring yields one `stream.resync`. */
  private since(after: string | undefined): FakeEvent[] {
    if (after === undefined) return [];
    const n = Number(after);
    const oldest = this.events[0] ? Number(this.events[0].id) : this.eventSeq + 1;
    if (Number.isFinite(n) && n < oldest - 1) return [{id: String(this.eventSeq), type: 'stream.resync', at: new Date(this.now()).toISOString()}];
    return this.events.filter(e => Number(e.id) > n);
  }

  // ── Router ──────────────────────────────────────────────────────
  async handle(r: FakeRequest): Promise<FakeResponse> {
    const failure = this.failures.shift();
    if (failure === 'offline') throw Object.assign(new Error('network unreachable'), {name: 'TypeError'});
    if (typeof failure === 'number') return this.error(failure, failure >= 500 ? 'internal' : 'request_failed');
    const body = (r.body ?? {}) as Record<string, unknown>;
    const m = r.method.toUpperCase();
    const [pathOnly, qs] = r.path.split('?', 2) as [string, string | undefined];
    const q = new URLSearchParams(qs ?? '');
    const h = (name: string) => r.headers?.[name] ?? r.headers?.[name.toLowerCase()];
    let match: RegExpExecArray | null;

    if ((match = /^\/v1\/media\/grant-([^/]+)\/audio$/.exec(pathOnly)) && (m === 'GET' || m === 'HEAD')) return this.audioBytes(match[1]!, h('Range'), m === 'HEAD');
    if (m === 'PUT' && pathOnly === '/v1/me/devices/current/capabilities') { this.capabilities.set(this.o.deviceId, body); return {status: 204, headers: {}}; }
    if (m === 'GET' && pathOnly === '/v1/events') return this.poll(q, r.signal);
    // Listening (music and books): the viewer's preferences and an item's context.
    if (m === 'GET' && pathOnly === '/v1/listening/preferences') return {status: 200, headers: {}, body: {scope: this.viewerScope(), data: {revision: 1, musicRate: 1, bookRate: 1, autoplayNext: this.o.autoplayNext ?? true, passoutMinutes: 0}}};
    if ((match = /^\/v1\/items\/([^/]+)\/listening$/.exec(pathOnly)) && m === 'GET') {
      const item = decodeURIComponent(match[1]!);
      return {status: 200, headers: {}, body: {scope: this.viewerScope(), data: {itemId: item, libraryId: 'library', kind: this.kindOf(item)}}};
    }
    if (m === 'GET' && pathOnly === '/v1/me/devices') return {status: 200, headers: {}, body: {items: (this.o.devices ?? []).filter(d => d.id !== this.o.deviceId).map(d => ({...d, nowPlaying: this.nowPlaying(d.id)})), page: {limit: 50}}};
    if ((match = /^\/v1\/items\/([^/]+)\/playback-options$/.exec(pathOnly)) && m === 'GET') return {status: 200, headers: {}, body: this.options(decodeURIComponent(match[1]!), q)};
    if (m === 'POST' && pathOnly === '/v1/playback/sessions') return this.start(h('Idempotency-Key'), body);
    if ((match = /^\/v1\/playback\/sessions\/([^/]+)\/transfers$/.exec(pathOnly)) && m === 'POST') return this.offer(decodeURIComponent(match[1]!), body);
    if ((match = /^\/v1\/playback\/sessions\/([^/]+)(\/timeline)?$/.exec(pathOnly))) {
      const s = this.sessions.get(decodeURIComponent(match[1]!));
      if (match[2]) return this.timeline(s, body);
      if (m === 'PATCH') return this.patch(s, h('If-Match'), body);
      if (m === 'DELETE') return this.stop(s, body);
      if (m === 'GET') return s ? {status: 200, headers: {ETag: etag(s.revision)}, body: this.view(s)} : this.error(404, 'session_not_found');
    }
    if ((match = /^\/v1\/devices\/([^/]+)\/commands$/.exec(pathOnly)) && m === 'POST') return this.command(decodeURIComponent(match[1]!), body);
    if (m === 'POST' && pathOnly === '/v1/queues') return this.createQueue(body);
    if ((match = /^\/v1\/queues\/([^/:]+)(.*)$/.exec(pathOnly))) return this.queueRoute(m, decodeURIComponent(match[1]!), match[2]!, q, h('If-Match'), body);
    if (m === 'GET' && pathOnly === '/v1/admin/sessions') return this.adminList(q);
    if ((match = /^\/v1\/admin\/sessions\/([^/:]+):terminate$/.exec(pathOnly)) && m === 'POST') return this.terminate(decodeURIComponent(match[1]!), body);
    return this.error(404, 'not_found');
  }

  private error(status: number, code: string, extra: Record<string, unknown> = {}): FakeResponse {
    return {status, headers: {}, body: {error: {code, message: code, retry: status === 412 || status === 409 ? 'after_refresh' : status >= 500 ? 'same_request' : 'never', requestId: 'req', ...extra}}};
  }

  // ── Options (§4.1) ──────────────────────────────────────────────
  private options(itemId: string, q: URLSearchParams) {
    const limited = q.get('quality') === 'limit';
    const audio = q.get('audioId') ?? 'a1';
    return {
      itemId, kind: 'movie', resume: {positionMs: 120_000, partIndex: 0, updatedAt: new Date(this.now()).toISOString()},
      versions: [{
        id: 'v1', label: '4K HDR · Remux', container: 'mkv', durationMs: 7_200_000, sizeBytes: 60e9, bitrateKbps: 60_000,
        parts: [{id: 'p1', index: 0, durationMs: 7_200_000}],
        video: [{id: 'v0', codec: 'hevc', profile: 'main10', bitDepth: 10, width: 3840, height: 2160, fps: 23.976, hdr: 'hdr10'}],
        audio: [{id: 'a1', codec: 'truehd', channels: 8, layout: '7.1', atmos: true, language: 'en', title: 'English', default: true}, {id: 'a2', codec: 'aac', channels: 2, language: 'en', title: 'Commentary', commentary: true}],
        subtitles: [{id: 's3', format: 'pgs', source: 'embedded', language: 'en', title: 'English SDH', sdh: true}, {id: 's4', format: 'srt', source: 'sidecar', language: 'fr', title: 'Français'}],
        futureField: 'ignored by tolerant readers',
      }],
      chapters: [{startMs: 0, title: 'Opening'}, {startMs: 600_000, title: 'Act II'}],
      markers: [{type: 'intro', startMs: 10_000, endMs: 70_000, confidence: 'detected'}, {type: 'mystery', startMs: 1, endMs: 2, confidence: 'unheard-of'}],
      preferred: {versionId: 'v1', audioId: 'a1', reason: 'profile_language'},
      plan: {versionId: 'v1', mode: 'stream', streams: [{id: 'v0', action: limited ? 'transcode' : 'copy', reasons: limited ? ['bitrate_above_request'] : []}, {id: audio, action: audio === 'a1' ? 'transcode' : 'copy', reasons: audio === 'a1' ? ['audio_codec_unsupported'] : []}], quality: {mode: limited ? 'limit' : 'original'}},
    };
  }

  // ── Sessions (§5) ───────────────────────────────────────────────
  private view(s: Session) {
    const transcoded = (s.quality as {mode?: string} | undefined)?.mode === 'limit';
    return {
      id: s.id, revision: String(s.revision), kind: s.kind, role: s.transferId ? 'receiver' : 'local', state: s.ended ? 'ended' : s.state, itemId: s.itemId, versionId: s.versionId,
      ...(s.queue ? {queue: s.queue} : {}),
      ...(s.ended ? {end: {reason: s.endReason ?? 'stopped', ...(s.terminatedMessage ? {message: s.terminatedMessage} : {})}} : {}),
      lease: {expiresAt: new Date(s.lastActivity + this.o.leaseMs).toISOString(), reportEveryMs: this.o.reportEveryMs},
      presentation: {
        generation: s.generation, mode: 'stream', url: `/v1/media/grant-${s.id}/${s.generation}/master.m3u8`, startPositionMs: s.positionMs,
        subtitles: [{trackId: 's4', format: 'webvtt', url: `/v1/media/grant-${s.id}/${s.generation}/subs/s4.vtt`}],
        decision: {video: {action: transcoded ? 'transcode' : 'copy', reasons: transcoded ? ['bitrate_above_request'] : []}, audio: {action: 'copy', reasons: []}, subtitles: {action: 'sidecar'}},
        ...(s.kind === 'audio' ? {audioRender: this.renderPlan(s.id, s.generation, s.itemId)} : {}),
      },
    };
  }

  private viewerScope() { return this.o.viewer ?? {serverId: 'server', authority: 'local', accountId: 'account', profileId: 'profile'}; }

  /** Bytes served to a prepared (private) presentation, by its session id: capped at prefetchBytes (§18.1). */
  private prefetched = new Map<string, number>();
  /** Version 2 direct audio: the file with Range (206), or the prepared grant's prefetch cap (403 prepared_limit). */
  private audioBytes(sessionId: string, range: string | undefined, head: boolean): FakeResponse {
    const s = this.sessions.get(sessionId);
    const pending = [...this.queues.values()].map(q => q.prepared).find(p => p?.nextSessionId === sessionId);
    const itemId = s?.itemId ?? (pending ? this.entryAt(this.queues.get([...this.queues.entries()].find(([, q]) => q.prepared === pending)![0])!, pending.nextPosition)?.itemId : undefined);
    if ((!s || s.ended) && !pending || !itemId) return this.error(404, 'not_found');
    const src = this.o.audioSource?.(itemId);
    if (!src) return this.error(404, 'not_found');
    const size = src.bytes.length;
    let start = 0, end = size - 1;
    const r = range && /^bytes=(\d*)-(\d*)$/.exec(range.trim());
    if (range && !r) return {status: 416, headers: {'Content-Range': `bytes */${size}`}};
    if (r) {
      if (r[1] === '') { start = Math.max(0, size - Number(r[2])); } else { start = Number(r[1]); if (r[2] !== '') end = Math.min(size - 1, Number(r[2])); }
      if (start > end || start >= size) return {status: 416, headers: {'Content-Range': `bytes */${size}`}};
    }
    if (!s && pending) {
      const served = (this.prefetched.get(sessionId) ?? 0) + (end - start + 1);
      if (served > Math.min(size, 8 << 20)) return {status: 403, headers: {}, body: {error: {code: 'prepared_limit', retry: 'never', requestId: 'req'}}};
      if (!head) this.prefetched.set(sessionId, served);
    }
    const headers: Record<string, string> = {'Accept-Ranges': 'bytes', 'Content-Length': String(end - start + 1), 'Content-Type': 'application/octet-stream'};
    if (r) headers['Content-Range'] = `bytes ${start}-${end}/${size}`;
    return {status: r ? 206 : 200, headers, body: head ? undefined : src.bytes.subarray(start, end + 1)};
  }

  /** A render plan (§18.1 version 2): the item's file (direct, Range) with its exact trim and gains. */
  renderPlan(sessionId: string, generation: number, itemId = '') {
    const src = this.o.audioSource?.(itemId);
    const bytes = src?.bytes.length ?? 180 * 44100 * 2;
    if (this.o.modeFromCapabilities) {
      const container = src?.container ?? 'flac', codec = src?.codec ?? 'flac', sampleRate = src?.sampleRate ?? 44100, channels = src?.channels ?? 2;
      const decode = ((this.capabilities.get(this.sessions.get(sessionId)?.deviceId ?? this.o.deviceId) as {audioDecode?: unknown} | undefined)?.audioDecode ?? []) as {codec?: string; containers?: string[]; maxSampleRate?: number; sampleRates?: number[]; maxChannels?: number}[];
      const fits = (d: (typeof decode)[number], c: string, k: string, rate: number) => d.codec === k && (d.containers ?? []).includes(c) && (d.sampleRates ? d.sampleRates.includes(rate) : rate <= (d.maxSampleRate ?? 0)) && channels <= (d.maxChannels ?? 0);
      const id = `m-${sessionId}-${generation}`;
      if (!decode.some(d => fits(d, container, codec, sampleRate))) {
        if (!decode.some(d => d.codec === 'flac' && (d.containers ?? []).includes('flac')))
          return {version: 2, mode: 'unavailable', id, reason: 'This device can’t decode this audio, and it declares no format Portico could convert it to.'};
        return {version: 2, mode: 'converted', id, reason: `This device can’t decode ${codec.toUpperCase()}, so Portico converts it to FLAC.`, url: `/v1/media/grant-${sessionId}/audio?format=flac`,
          container: 'flac', codec: 'flac', sampleRate, channels, durationFrames: src?.durationFrames ?? 180 * 44100, trim: {startFrames: 0, endFrames: 0, source: 'flac'},
          gain: src?.gain ?? {trackDb: -6, albumDb: -5, trackPeak: 0.9, albumPeak: 0.95, source: 'tags'}};
      }
    }
    return {version: 2, mode: 'direct', id: `m-${sessionId}-${generation}`, url: `/v1/media/grant-${sessionId}/audio`,
      container: src?.container ?? 'flac', codec: src?.codec ?? 'flac', sampleRate: src?.sampleRate ?? 44100, channels: src?.channels ?? 2,
      bytes, prefetchBytes: Math.min(bytes, 8 << 20), durationFrames: src?.durationFrames ?? 180 * 44100,
      trim: src?.trim ?? {startFrames: 0, endFrames: 0, source: 'flac'},
      gain: src?.gain ?? {trackDb: -6, albumDb: -5, trackPeak: 0.9, albumPeak: 0.95, source: 'tags'}};
  }

  /** The next position completion would play (repeat and shuffle laps as `:advance`), or undefined. */
  private completionNext(q: Queue): number | undefined {
    const total = this.total(q);
    if (q.repeat === 'one') return q.current;
    if (q.current + 1 < total) return q.current + 1;
    if (q.repeat === 'all') return 0;
    return undefined;
  }

  /** Any change to the queue or its current session cancels a preparation (§18.2). */
  private cancelPrepared(sessionId?: string) {
    for (const q of this.queues.values()) if (q.prepared && (sessionId === undefined || q.prepared.sessionId === sessionId)) q.prepared = undefined;
  }

  private expire(s: Session) { if (!s.ended && this.now() - s.lastActivity > this.o.leaseMs) this.end(s, 'lease_expired'); }

  /** Ends a session and says why, like the server: the event and a re-read both carry the reason. */
  private end(s: Session, reason = 'stopped') {
    if (s.ended) return;
    s.ended = true;
    s.endReason = reason;
    this.publish('session.updated', {kind: 'session', id: s.id}, String(++s.revision), {state: 'ended', reason, ...(s.terminatedMessage ? {message: s.terminatedMessage} : {})});
    this.publish('admin.sessions');
  }

  private newSession(itemId: string, fields: Partial<Session>): Session {
    const id = fields.id ?? `s${++this.nextId}`; // a committed preparation reserved its id (§18.3)
    const s: Session = {id, revision: 1, state: 'playing', kind: (this.o.audioItem ?? (i => i.startsWith('song')))(itemId) ? 'audio' : 'vod', itemId, versionId: 'v1', quality: {mode: 'original'}, generation: 1, positionMs: 0, deviceId: this.o.deviceId, lastSeq: 0, lastActivity: this.now(), ended: false, reports: [], startedAt: this.now(), ...fields};
    this.sessions.set(id, s);
    this.publish('admin.sessions');
    return s;
  }

  private start(key: string | undefined, body: Record<string, unknown>): FakeResponse {
    if (!key) return this.error(428, 'idempotency_key_required');
    const canonical = JSON.stringify(body);
    const prior = this.keys.get(key);
    if (prior) {
      if (prior.body !== canonical) return this.error(422, 'idempotency_key_reused');
      if (prior.rounds > 0) { prior.rounds--; return {status: 202, headers: {'Retry-After': '1'}, body: {retryAfterMs: 500}}; }
      if (prior.status === 202) return this.start2(key, body, prior);
      return {status: prior.status, headers: {'Idempotent-Replayed': 'true'}, body: prior.response};
    }
    const refused = this.o.admit?.(body);
    if (refused) return this.error(403, refused);
    const rounds = this.o.prepareRounds?.(body) ?? 0;
    const record = {body: canonical, status: 202, response: undefined as unknown, rounds};
    this.keys.set(key, record);
    if (rounds > 0) { record.rounds--; return {status: 202, headers: {'Retry-After': '1'}, body: {retryAfterMs: 500}}; }
    return this.start2(key, body, record);
  }

  private start2(_key: string, body: Record<string, unknown>, record: {status: number; response: unknown}): FakeResponse {
    let s: Session;
    if (typeof body.transferId === 'string') {
      const t = this.transfers.get(body.transferId);
      const src = t && this.sessions.get(t.sourceId);
      if (!t || !src) return this.error(404, 'transfer_not_found');
      s = this.newSession(src.itemId, {versionId: src.versionId, positionMs: src.positionMs, audio: src.audio, subtitles: src.subtitles, quality: src.quality, queue: src.queue, transferId: t.id});
    } else if (body.queue && typeof (body.queue as {queueId?: unknown}).queueId === 'string') {
      const qref = body.queue as {queueId: string; entryId: string};
      const queue = this.queues.get(qref.queueId);
      if (!queue) return this.error(404, 'queue_not_found');
      s = this.newSession(this.itemOfEntry(queue, qref.entryId) ?? 'unknown', {queue: qref, state: body.state === 'paused' ? 'paused' : 'playing', positionMs: typeof body.startPositionMs === 'number' ? body.startPositionMs : 0});
      queue.sessionId = s.id;
    } else if (typeof body.itemId === 'string' || typeof body.channelId === 'string') {
      s = this.newSession((body.itemId ?? body.channelId) as string, {
        ...(typeof body.channelId === 'string' ? {kind: 'channel' as const} : {}), state: body.state === 'paused' ? 'paused' : 'playing',
        versionId: typeof body.versionId === 'string' ? body.versionId : 'v1', audio: body.audio, subtitles: body.subtitles, quality: body.quality ?? {mode: 'original'},
        positionMs: typeof body.startPositionMs === 'number' ? body.startPositionMs : body.startFrom === 'resume' ? 120_000 : 0,
      });
    } else return this.error(400, 'invalid_request');
    const replaced = typeof body.replacesSessionId === 'string' ? this.sessions.get(body.replacesSessionId) : undefined;
    if (replaced && replaced.deviceId === this.o.deviceId) this.end(replaced, 'replaced');
    record.status = 201;
    record.response = this.view(s);
    return {status: 201, headers: {ETag: etag(s.revision)}, body: record.response};
  }

  private patch(s: Session | undefined, ifMatch: string | undefined, body: Record<string, unknown>): FakeResponse {
    if (!s) return this.error(404, 'session_not_found');
    this.cancelPrepared(s.id); // §18.2: a change to the current session cancels its preparation
    this.expire(s);
    if (s.ended) return this.error(410, 'session_ended');
    if (ifMatch === undefined) return this.error(428, 'revision_required');
    if (unquote(ifMatch) !== String(s.revision)) return {status: 412, headers: {ETag: etag(s.revision)}, body: {error: {code: 'revision_mismatch', retry: 'after_refresh', current: this.view(s), requestId: 'req'}}};
    let bytes = false;
    for (const k of BYTES_CHANGING) if (k in body && JSON.stringify(body[k]) !== JSON.stringify((s as Record<string, unknown>)[k])) { (s as Record<string, unknown>)[k] = body[k]; bytes = true; }
    if (body.state === 'playing' || body.state === 'paused') s.state = body.state;
    const seek = body.seek as {positionMs?: unknown} | undefined;
    if (seek && typeof seek.positionMs === 'number') { s.positionMs = seek.positionMs; bytes = true; }
    if (bytes) s.generation++;
    s.revision++;
    s.lastActivity = this.now();
    return {status: 200, headers: {ETag: etag(s.revision)}, body: this.view(s)};
  }

  /** Simulate another client changing a session (a remote PATCH), for 412 tests. */
  bump(sessionId: string, change: Record<string, unknown> = {state: 'paused'}): void {
    const s = this.sessions.get(sessionId)!;
    this.patch(s, String(s.revision), change);
    this.publish('session.updated', {kind: 'session', id: s.id}, String(s.revision));
  }

  private timeline(s: Session | undefined, body: Record<string, unknown>): FakeResponse {
    if (!s) return this.error(404, 'session_not_found');
    this.expire(s);
    if (s.ended) return this.error(410, 'session_ended');
    s.lastActivity = this.now();
    s.reports.push(body);
    if (typeof body.seq === 'number' && body.seq > s.lastSeq && body.generation === s.generation) {
      s.lastSeq = body.seq;
      if (typeof body.positionMs === 'number') s.positionMs = body.positionMs;
      if (body.state === 'playing' || body.state === 'paused') s.state = body.state;
      if (body.state === 'ended') this.end(s);
      // Transfer commit: the target's first playing (or paused) report ends the source (§9.2).
      if (s.transferId && (body.state === 'playing' || body.state === 'paused')) {
        const t = this.transfers.get(s.transferId);
        if (t && !t.committed) { t.committed = true; const src = this.sessions.get(t.sourceId); if (src) this.end(src, 'transferred'); }
      }
    }
    return {status: 204, headers: {'Report-Every-Ms': String(this.o.reportEveryMs)}};
  }

  private stop(s: Session | undefined, body: Record<string, unknown>): FakeResponse {
    if (s) this.cancelPrepared(s.id);
    if (s && !s.ended) {
      if (typeof body.positionMs === 'number') s.positionMs = body.positionMs;
      this.end(s);
    }
    return {status: 204, headers: {}};
  }

  // ── Devices, commands, transfers (§9) ───────────────────────────
  private nowPlaying(deviceId: string) {
    const s = [...this.sessions.values()].find(x => x.deviceId === deviceId && !x.ended);
    return s ? {itemId: s.itemId, state: s.state, positionMs: s.positionMs} : undefined;
  }

  private command(deviceId: string, body: Record<string, unknown>): FakeResponse {
    if (!(this.o.devices ?? []).some(d => d.id === deviceId)) return this.error(404, 'device_not_found');
    if (typeof body.type !== 'string') return this.error(400, 'invalid_request');
    const commandId = `c${++this.nextId}`;
    this.commands.push({deviceId, command: body});
    this.publish('device.command', {kind: 'device', id: deviceId}, undefined, {...body, commandId, fromDeviceId: this.o.deviceId, targetDeviceId: deviceId});
    return {status: 202, headers: {}, body: {commandId}};
  }

  private offer(sessionId: string, body: Record<string, unknown>): FakeResponse {
    const s = this.sessions.get(sessionId);
    if (!s || s.ended) return this.error(404, 'session_not_found');
    if (typeof body.targetDeviceId !== 'string') return this.error(400, 'invalid_request');
    const t: Transfer = {id: `t${++this.nextId}`, sourceId: s.id, targetDeviceId: body.targetDeviceId, committed: false};
    this.transfers.set(t.id, t);
    this.publish('playback.transfer_offered', {kind: 'device', id: t.targetDeviceId}, undefined, {transferId: t.id, fromDeviceId: s.deviceId, itemId: s.itemId});
    return {status: 201, headers: {}, body: {transferId: t.id}};
  }

  // ── Events (§15) ────────────────────────────────────────────────
  private poll(q: URLSearchParams, signal?: AbortSignal): Promise<FakeResponse> | FakeResponse {
    const after = q.get('after') ?? undefined;
    const wait = Math.max(0, Math.min(Number(q.get('waitSeconds') ?? 25), 55)) * 1000;
    const answer = (): FakeResponse => {
      const events = this.since(after ?? String(this.eventSeq));
      return {status: 200, headers: {}, body: {events, nextAfter: events.length ? events[events.length - 1]!.id : after ?? String(this.eventSeq)}};
    };
    if (after !== undefined && this.since(after).length) return answer();
    return new Promise(resolve => {
      let timer: unknown;
      const done = () => { this.waiters.delete(done); this.clearTimer(timer); resolve(answer()); };
      this.waiters.add(done);
      timer = this.setTimer(done, wait);
      signal?.addEventListener('abort', () => { this.waiters.delete(done); this.clearTimer(timer); resolve({status: 499, headers: {}}); }, {once: true});
    });
  }

  // ── Queues (§8) ─────────────────────────────────────────────────
  private segmentFrom(source: Record<string, unknown>): Segment | undefined {
    const id = `g${++this.nextId}`;
    const items = (source.items as {ids?: unknown} | undefined)?.ids;
    if (Array.isArray(items) && items.every(x => typeof x === 'string') && items.length <= 500) {
      const ids = [...items] as string[];
      return {id, kind: 'items', label: `${ids.length} items`, count: ids.length, keyAt: i => ids[i]!, removals: new Set()};
    }
    const c = source.container as {kind?: unknown; id?: unknown} | undefined;
    if (c && typeof c.kind === 'string' && typeof c.id === 'string') {
      const n = this.o.containerSize?.(c.kind, c.id) ?? 12;
      const prefix = `${c.id}-`;
      return {id, kind: 'selector', label: `${c.kind} ${c.id}`, count: n, keyAt: i => prefix + i, removals: new Set()};
    }
    return undefined;
  }

  /** Visible entries in source order: (segment, ordinal) pairs, skipping tombstones. O(removals). */
  private total(q: Queue): number { return q.segments.reduce((n, s) => n + s.count - s.removals.size, 0); }

  private sourceAt(q: Queue, p: number): {seg: Segment; ordinal: number} | undefined {
    for (const seg of q.segments) {
      const live = seg.count - seg.removals.size;
      if (p < live) {
        let ordinal = p;
        for (const r of [...seg.removals].sort((a, b) => a - b)) if (r <= ordinal) ordinal++;
        return {seg, ordinal};
      }
      p -= live;
    }
    return undefined;
  }

  private sourceIndexOf(q: Queue, segId: string, ordinal: number): number | undefined {
    let base = 0;
    for (const seg of q.segments) {
      if (seg.id === segId) { if (seg.removals.has(ordinal) || ordinal >= seg.count) return undefined; return base + ordinal - [...seg.removals].filter(r => r < ordinal).length; }
      base += seg.count - seg.removals.size;
    }
    return undefined;
  }

  /** Play-order position → source position (shuffle keeps the current entry first; later additions are an unshuffled tail). */
  private orderToSource(q: Queue, p: number): number {
    const sh = q.shuffle;
    if (!sh || p >= sh.domain) return p;
    if (p === 0) return sh.first;
    const perm = new ShufflePermutation(sh.domain, sh.seed, sh.lap);
    const c = perm.indexOf(sh.first);
    return perm.at(p - 1 < c ? p - 1 : p);
  }

  private sourceToOrder(q: Queue, s: number): number {
    const sh = q.shuffle;
    if (!sh || s >= sh.domain) return s;
    if (s === sh.first) return 0;
    const perm = new ShufflePermutation(sh.domain, sh.seed, sh.lap);
    const c = perm.indexOf(sh.first), k = perm.indexOf(s);
    return k < c ? k + 1 : k;
  }

  private kindOf(itemId: string): string {
    return this.o.itemKind?.(itemId) ?? ((this.o.audioItem ?? (i => i.startsWith('song')))(itemId) ? 'song' : 'movie');
  }

  private entryAt(q: Queue, position: number) {
    const src = this.sourceAt(q, this.orderToSource(q, position));
    if (!src) return undefined;
    const itemId = src.seg.keyAt(src.ordinal);
    return {entryId: `${src.seg.id}.${src.ordinal}`, position, itemId, kind: this.kindOf(itemId), title: `Title ${itemId}`, durationMs: 180_000, available: true};
  }

  private positionOf(q: Queue, entryId: string): number | undefined {
    const [segId, ord] = entryId.split('.');
    const s = this.sourceIndexOf(q, segId!, Number(ord));
    return s === undefined ? undefined : this.sourceToOrder(q, s);
  }

  private itemOfEntry(q: Queue, entryId: string): string | undefined {
    const [segId, ord] = entryId.split('.');
    return q.segments.find(s => s.id === segId)?.keyAt(Number(ord));
  }

  private header(q: Queue) {
    const current = this.entryAt(q, q.current);
    return {
      id: q.id, revision: String(q.revision), total: this.total(q), repeat: q.repeat,
      ...(current ? {current: {entryId: current.entryId, position: q.current}} : {}),
      ...(q.shuffle ? {shuffle: {seed: String(q.shuffle.seed), lap: q.shuffle.lap}} : {}),
      segments: q.segments.map(s => ({id: s.id, kind: s.kind, label: s.label, count: s.count - s.removals.size, state: 'ready'})),
    };
  }

  private window(q: Queue, from: number, limit: number) {
    const total = this.total(q), n = Math.max(0, Math.min(limit, WINDOW_MAX, total - from));
    const items = Array.from({length: n}, (_, i) => this.entryAt(q, from + i)!).filter(Boolean);
    return {items, page: {limit: Math.min(limit, WINDOW_MAX), total, start: from, revision: String(q.revision), ...(from + n < total ? {nextCursor: `from:${from + n}`} : {})}};
  }

  private createQueue(body: Record<string, unknown>): FakeResponse {
    const segs = Array.isArray(body.segments) ? body.segments as Record<string, unknown>[] : [];
    if (!segs.length) return this.error(400, 'invalid_request');
    const previous = [...this.queues.values()].find(x => x.deviceId === this.o.deviceId);
    if (previous) this.queues.delete(previous.id);
    const q: Queue = {id: `q${++this.nextId}`, revision: 1, deviceId: this.o.deviceId, segments: [], current: 0, repeat: 'off'};
    for (const s of segs) { const seg = this.segmentFrom((s.source ?? {}) as Record<string, unknown>); if (!seg) return this.error(422, 'unsupported_selector'); q.segments.push(seg); }
    const first = segs[0]!;
    const order = first.order as {mode?: string; seed?: string} | undefined;
    const anchor = first.anchor as {itemId?: string; position?: number} | string | undefined;
    const total = this.total(q);
    let start = 0;
    if (anchor && typeof anchor === 'object' && typeof anchor.position === 'number') start = Math.min(anchor.position, total - 1);
    if (anchor && typeof anchor === 'object' && typeof anchor.itemId === 'string') { for (let i = 0; i < Math.min(total, 10_000); i++) if (this.sourceAt(q, i)!.seg.keyAt(this.sourceAt(q, i)!.ordinal) === anchor.itemId) { start = i; break; } }
    if (order?.mode === 'shuffle') {
      const seed = order.seed !== undefined ? Number(order.seed) >>> 0 : (this.nextId * 2654435761) >>> 0;
      // Shuffle-play from the start picks a uniformly random first entry (ARCH-MEDIA-04); here, from the seed.
      const firstSource = anchor ? start : new ShufflePermutation(total, seed, 1).at(0);
      q.shuffle = {seed, lap: 0, domain: total, first: firstSource};
      q.current = 0;
    } else q.current = start;
    this.queues.set(q.id, q);
    let session: Session | undefined;
    const sp = body.startPlayback as {state?: string; quality?: unknown; startPositionMs?: unknown} | undefined;
    if (sp) {
      const cur = this.entryAt(q, q.current)!;
      // A resumed start (a book's saved position): the test says where the session begins.
      const positionMs = typeof sp.startPositionMs === 'number' && sp.startPositionMs >= 0 ? sp.startPositionMs : 0;
      session = this.newSession(cur.itemId, {queue: {queueId: q.id, entryId: cur.entryId}, state: sp.state === 'paused' ? 'paused' : 'playing', quality: sp.quality ?? {mode: 'original'}, positionMs});
      q.sessionId = session.id;
    }
    const w = this.window(q, Math.max(0, q.current - 20), 71);
    return {status: 201, headers: {ETag: etag(q.revision)}, body: {queue: this.header(q), window: w.items, ...(session ? {session: this.view(session)} : {})}};
  }

  private queueRoute(m: string, id: string, rest: string, qs: URLSearchParams, ifMatch: string | undefined, body: Record<string, unknown>): FakeResponse {
    const q = this.queues.get(id);
    if (!q) return this.error(404, 'queue_not_found');
    if (m === 'GET' && rest === '') return {status: 200, headers: {ETag: etag(q.revision)}, body: this.header(q)};
    if (m === 'GET' && rest === '/entries') {
      const limit = Number(qs.get('limit') ?? 50);
      if (!(limit >= 1)) return this.error(400, 'invalid_limit');
      return {status: 200, headers: {}, body: this.window(q, Math.max(0, Number(qs.get('from') ?? 0)), limit)};
    }
    if (m === 'GET' && rest === '/window') {
      const before = Math.min(Number(qs.get('before') ?? 20), WINDOW_MAX), after = Math.min(Number(qs.get('after') ?? 50), WINDOW_MAX - before);
      const from = Math.max(0, q.current - before);
      return {status: 200, headers: {}, body: this.window(q, from, q.current - from + after + 1)};
    }
    if (m === 'DELETE' && rest === '') { this.queues.delete(id); return {status: 204, headers: {}}; }
    if (m === 'POST' && rest === ':commit-next') return this.commitNext(q, body);
    // Commands: If-Match.
    if (ifMatch === undefined) return this.error(428, 'revision_required');
    if (unquote(ifMatch) !== String(q.revision)) return {status: 412, headers: {}, body: {error: {code: 'revision_mismatch', retry: 'after_refresh', current: this.header(q), requestId: 'req'}}};
    let match: RegExpExecArray | null;
    let session: Session | undefined;
    if (m === 'POST' && rest === ':prepare-next') return this.prepareNext(q, body);
    q.prepared = undefined; // any other command cancels a preparation (§18.2)
    if (m === 'POST' && rest === '/segments') {
      const seg = this.segmentFrom((body.source ?? {}) as Record<string, unknown>);
      if (!seg) return this.error(422, 'unsupported_selector');
      const placement = body.placement as string | {after?: string};
      const currentSrc = this.sourceAt(q, this.orderToSource(q, q.current));
      let at = q.segments.length;
      if (placement === 'next' && currentSrc) at = q.segments.indexOf(currentSrc.seg) + 1;
      if (typeof placement === 'object' && placement?.after) { const segId = placement.after.split('.')[0]; const i = q.segments.findIndex(s => s.id === segId); if (i >= 0) at = i + 1; }
      q.segments.splice(at, 0, seg);
    } else if (m === 'DELETE' && (match = /^\/entries\/(.+)$/.exec(rest))) {
      const [segId, ord] = decodeURIComponent(match[1]!).split('.');
      const seg = q.segments.find(s => s.id === segId);
      if (!seg || seg.removals.has(Number(ord))) return this.error(404, 'entry_not_found');
      if (q.shuffle) return this.error(409, 'unsupported_in_fake');
      const pos = this.positionOf(q, decodeURIComponent(match[1]!))!;
      seg.removals.add(Number(ord));
      if (pos < q.current) q.current--;
    } else if (m === 'POST' && (match = /^\/entries\/(.+):move$/.exec(rest))) {
      if (q.shuffle) return this.error(409, 'unsupported_in_fake');
      const entryId = decodeURIComponent(match[1]!);
      const itemId = this.itemOfEntry(q, entryId);
      const [segId, ord] = entryId.split('.');
      const seg = q.segments.find(s => s.id === segId);
      const to = (body.after ?? body.before) as string | undefined;
      const toSeg = to ? q.segments.findIndex(s => s.id === to.split('.')[0]) : -1;
      if (!seg || !itemId || toSeg < 0) return this.error(404, 'entry_not_found');
      seg.removals.add(Number(ord));
      const one: Segment = {id: `g${++this.nextId}`, kind: 'items', label: '1 item', count: 1, keyAt: () => itemId, removals: new Set()};
      q.segments.splice(body.after ? toSeg + 1 : toSeg, 0, one);
    } else if (m === 'PATCH' && rest === '') {
      if (body.repeat === 'off' || body.repeat === 'one' || body.repeat === 'all') q.repeat = body.repeat;
      const sh = body.shuffle as {on?: boolean; seed?: string} | undefined;
      if (sh?.on === true) { const src = this.orderToSource(q, q.current); q.shuffle = {seed: sh.seed !== undefined ? Number(sh.seed) >>> 0 : (this.nextId * 40503) >>> 0, lap: 0, domain: this.total(q), first: src}; q.current = 0; }
      if (sh?.on === false && q.shuffle) { q.current = this.orderToSource(q, q.current); q.shuffle = undefined; }
    } else if (m === 'POST' && rest === ':advance') {
      const total = this.total(q);
      const reason = body.reason;
      if (reason === 'entry' && typeof body.entryId === 'string') { const p = this.positionOf(q, body.entryId); if (p === undefined) return this.error(404, 'entry_not_found'); q.current = p; }
      else if (reason === 'previous') q.current = Math.max(0, q.current - 1);
      else if (q.repeat === 'one' && reason === 'completion') { /* same entry */ }
      else if (q.current + 1 < total) q.current++;
      else if (q.repeat === 'all') { q.current = 0; if (q.shuffle) q.shuffle.lap++; }
      else return this.error(409, 'queue_ended');
      if (q.sessionId) {
        const cur = this.entryAt(q, q.current)!;
        const old = this.sessions.get(q.sessionId);
        // §18.5: the successor inherits state, quality and audio track, even after an `ended` report.
        session = this.newSession(cur.itemId, {queue: {queueId: q.id, entryId: cur.entryId}, ...(old ? {state: old.state, quality: old.quality, audio: old.audio} : {})});
        if (old) this.end(old);
        q.sessionId = session.id;
      }
    } else return this.error(404, 'not_found');
    q.revision++;
    this.publish('queue.updated', {kind: 'queue', id: q.id}, String(q.revision), {current: q.current});
    return {status: 200, headers: {ETag: etag(q.revision)}, body: {queue: this.header(q), ...(session ? {session: this.view(session)} : {})}};
  }

  // ── Queue transitions (§18) ─────────────────────────────────────
  private prepareNext(q: Queue, body: Record<string, unknown>): FakeResponse {
    const current = q.sessionId ? this.sessions.get(q.sessionId) : undefined;
    if (current) this.expire(current);
    if (!current || current.ended || body.sessionId !== current.id) return {status: 409, headers: {}, body: {error: {code: 'prepare_not_allowed', reason: 'session_not_current', retry: 'after_refresh', requestId: 'req'}}};
    if (body.sessionGeneration !== current.generation) return {status: 409, headers: {}, body: {error: {code: 'prepare_not_allowed', reason: 'session_changed', retry: 'after_refresh', requestId: 'req'}}};
    if (current.kind !== 'audio') return {status: 409, headers: {}, body: {error: {code: 'prepare_not_allowed', reason: 'not_rendered', retry: 'after_refresh', requestId: 'req'}}};
    const next = this.completionNext(q);
    if (next === undefined) return this.error(409, 'queue_ended');
    const entry = this.entryAt(q, next)!;
    const nextSessionId = `s${++this.nextId}`, token = `pn_${this.nextId}`, expiresAt = this.now() + 60_000;
    q.prepared = {token, sessionId: current.id, sessionGeneration: current.generation, nextPosition: next, nextSessionId, expiresAt, revision: q.revision};
    this.preparations++;
    return {status: 200, headers: {}, body: {token, expiresAt: new Date(expiresAt).toISOString(), entryId: entry.entryId, itemId: entry.itemId,
      presentation: {generation: 1, mode: 'stream', url: '', startPositionMs: 0, subtitles: [], decision: {audio: {action: 'copy', reasons: []}}, audioRender: this.renderPlan(nextSessionId, 1, entry.itemId)}}};
  }

  private commitNext(q: Queue, body: Record<string, unknown>): FakeResponse {
    const token = typeof body.token === 'string' ? body.token : '';
    const replay = q.committed?.get(token);
    if (replay) return replay;
    const p = q.prepared;
    const canceled = (reason: string): FakeResponse => { q.prepared = undefined; return {status: 409, headers: {}, body: {error: {code: 'prepared_canceled', reason, retry: 'never', requestId: 'req'}}}; };
    if (!p || p.token !== token) return this.error(404, 'not_found');
    if (p.expiresAt <= this.now()) { q.prepared = undefined; return this.error(410, 'prepared_expired'); }
    const old = this.sessions.get(p.sessionId);
    if (old) this.expire(old);
    if (!old || old.ended || old.generation !== p.sessionGeneration || q.sessionId !== old.id) return canceled('session_changed');
    if (q.revision !== p.revision) return canceled('queue_changed');
    q.current = p.nextPosition;
    const entry = this.entryAt(q, q.current)!;
    const session = this.newSession(entry.itemId, {id: p.nextSessionId, queue: {queueId: q.id, entryId: entry.entryId}, state: old.state, quality: old.quality, audio: old.audio});
    this.end(old, 'completed');
    q.sessionId = session.id;
    q.prepared = undefined;
    q.revision++;
    this.commits++;
    this.publish('queue.updated', {kind: 'queue', id: q.id}, String(q.revision), {current: q.current});
    const response: FakeResponse = {status: 200, headers: {ETag: etag(q.revision)}, body: {queue: this.header(q), session: this.view(session)}};
    (q.committed ??= new Map()).set(token, response);
    return response;
  }

  // ── Admin (§14) ─────────────────────────────────────────────────
  private adminList(q: URLSearchParams): FakeResponse {
    const active = [...this.sessions.values()].filter(s => { this.expire(s); return !s.ended; });
    const limit = Math.max(1, Math.min(Number(q.get('limit') ?? 50), 200));
    const start = Number((q.get('cursor') ?? 'at:0').split(':')[1]);
    const page = active.slice(start, start + limit).map(s => ({
      id: s.id, user: {id: 'u1', name: 'demo'}, profile: {id: 'p1', name: 'Admin'}, device: {id: s.deviceId, name: s.deviceId}, item: {id: s.itemId, title: `Title ${s.itemId}`},
      kind: s.kind, state: s.state, positionMs: s.positionMs, decision: this.view(s).presentation.decision, bitrateKbps: 8000, location: 'local', startedAt: new Date(s.startedAt).toISOString(),
    }));
    return {status: 200, headers: {}, body: {items: page, page: {limit, total: active.length, ...(start + limit < active.length ? {nextCursor: `at:${start + limit}`} : {})}}};
  }

  private terminate(id: string, body: Record<string, unknown>): FakeResponse {
    const s = this.sessions.get(id);
    if (!s || s.ended) return this.error(404, 'session_not_found');
    s.terminatedMessage = typeof body.message === 'string' ? body.message : '';
    this.end(s, 'terminated');
    return {status: 204, headers: {}};
  }
}

/** A client-shaped error for a non-2xx fake response (status and code, like ApiError). */
export function fakeResponseError(r: FakeResponse): Error {
  const code = (r.body as {error?: {code?: string}} | undefined)?.error?.code ?? 'request_failed';
  return Object.assign(new Error(code), {status: r.status, code, body: r.body});
}

/** A `V1Http` over the fake (what the platform's HTTP adapter will be in phase 2). */
export function fakeHttp(server: FakePlaybackServer) {
  return {send: (request: FakeRequest) => server.handle(request)};
}
