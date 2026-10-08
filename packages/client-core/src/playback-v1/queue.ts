/**
 * Queues (spec §8; Server Media ARCH-MEDIA-03/04/05). A queue is server-owned: selector segments
 * frozen into snapshots at Play, addressed by stable entry ids, read in windows of at most 200
 * entries. The client never holds the whole queue: `QueueView` puts the entries behind the
 * windowed collection, so a 10-million-entry queue costs what is on screen.
 *
 * Play and Shuffle are one request (`create(..., {startPlayback})` returns the first session).
 * Commands are constant-size, carry `If-Match` and an `Idempotency-Key`, and return the new
 * header; a 412 carries the current header so the caller can decide again.
 */
import {createWindowedCollection, type WindowedCollection} from '../collections/windowed.ts';
import {PlaybackApiError, call, enc, idempotencyKey, type V1Http} from './http.ts';
import type {QualityRequest} from './quality.ts';
import {ContractError, parseEntry, parsePage, parsePreparedNext, parseQueue, parseSession, obj, type PreparedNext, type QueueEntry, type QueueHeader, type Selector, type Session} from './types.ts';

/** Published window maximum (spec §8, invariant 8). */
export const QUEUE_WINDOW_MAX = 200;

export type QueueOrder = Readonly<{mode: 'source'} | {mode: 'shuffle'; seed?: string}>;
export type QueueAnchor = Readonly<{itemId: string} | {position: number}> | 'resume';
export type SegmentInput = Readonly<{source: Selector; order?: QueueOrder; anchor?: QueueAnchor}>;
export type Placement = 'next' | 'end' | Readonly<{after: string}>;

export type CreateResult = Readonly<{queue: QueueHeader; window: readonly QueueEntry[]; session?: Session}>;

export class QueueClient {
  private http: V1Http;
  private key: () => string;
  constructor(http: V1Http, key: () => string = () => idempotencyKey()) { this.http = http; this.key = key; }

  /** Play (or Shuffle, or Resume a show): replaces this device's current queue. */
  async create(segments: readonly SegmentInput[], options: {startPlayback?: Readonly<{state?: 'playing' | 'paused'; quality?: QualityRequest}>; signal?: AbortSignal} = {}): Promise<CreateResult> {
    const r = await call(this.http, {method: 'POST', path: '/v1/queues', headers: {'Idempotency-Key': this.key()}, body: {owner: 'device', segments, ...(options.startPlayback ? {startPlayback: options.startPlayback} : {})}, signal: options.signal});
    const b = r.body as {queue?: unknown; window?: unknown; session?: unknown};
    return Object.freeze({queue: parseQueue(b.queue), window: Object.freeze(Array.isArray(b.window) ? b.window.map(parseEntry) : []), session: b.session !== undefined ? parseSession(b.session) : undefined});
  }

  async header(id: string, signal?: AbortSignal): Promise<QueueHeader> {
    return parseQueue((await call(this.http, {method: 'GET', path: `/v1/queues/${enc(id)}`, signal})).body);
  }

  /**
   * A very large selection can't always pick its first entry in the create request (a shuffle
   * whose size wasn't known in time): the create answers with segments `building`, no `current`
   * and no session, and the server chooses the first entry when the snapshot is built (then a
   * `queue.updated` event). This resolves with the header once `current` is set; the caller then
   * starts playback of that entry (`SessionsClient.start({queue: {queueId, entryId}})`). Polls the
   * header every `intervalMs`; `wake()` on the returned handle checks at once (a queue event).
   */
  awaitCurrent(id: string, options: {signal?: AbortSignal; intervalMs?: number} = {}): Readonly<{done: Promise<QueueHeader>; wake: () => void}> {
    let wake = () => {};
    const done = (async () => {
      for (;;) {
        if (options.signal?.aborted) throw Object.assign(new Error('Aborted'), {name: 'AbortError'}); // Hermes has no DOMException
        const header = await this.header(id, options.signal);
        if (header.current) return header;
        if (!header.segments.some(s => s.state === 'building')) throw new PlaybackApiError(409, 'queue_ended', 'never');
        await new Promise<void>(resolve => {
          const timer = setTimeout(resolve, Math.max(50, options.intervalMs ?? 1000));
          wake = () => { clearTimeout(timer); resolve(); };
          options.signal?.addEventListener('abort', () => { clearTimeout(timer); resolve(); }, {once: true});
        });
      }
    })();
    return Object.freeze({done, wake: () => wake()});
  }

  /** True when a create answered before its start could be chosen (see `awaitCurrent`). */
  static startPending(result: CreateResult): boolean {
    return !result.session && !result.queue.current && result.queue.segments.some(s => s.state === 'building');
  }

  /** Entries around the current one (the Up Next panel). */
  async window(id: string, before = 20, after = 50, signal?: AbortSignal): Promise<readonly QueueEntry[]> {
    const b = Math.max(0, Math.min(before, QUEUE_WINDOW_MAX)), a = Math.max(0, Math.min(after, QUEUE_WINDOW_MAX - b));
    return parsePage((await call(this.http, {method: 'GET', path: `/v1/queues/${enc(id)}/window?around=current&before=${b}&after=${a}`, signal})).body, parseEntry).items;
  }

  /** A page of entries from a position (never more than 200). */
  async entries(id: string, from: number, limit: number, signal?: AbortSignal) {
    const n = Math.max(1, Math.min(limit, QUEUE_WINDOW_MAX));
    return parsePage((await call(this.http, {method: 'GET', path: `/v1/queues/${enc(id)}/entries?from=${Math.max(0, from)}&limit=${n}`, signal})).body, parseEntry);
  }

  private async command(id: string, revision: string, method: 'POST' | 'PATCH' | 'DELETE', path: string, body?: unknown): Promise<Readonly<{queue: QueueHeader; session?: Session}>> {
    try {
      const r = await call(this.http, {method, path, headers: {'If-Match': revision, 'Idempotency-Key': this.key(), ...(method === 'PATCH' ? {'Content-Type': 'application/merge-patch+json'} : {})}, body});
      const b = r.body as {queue?: unknown; session?: unknown} | undefined;
      const header = obj(b) && b.queue !== undefined ? b.queue : b;
      return Object.freeze({queue: parseQueue(header), session: obj(b) && b.session !== undefined ? parseSession(b.session) : undefined});
    } catch (e) {
      if (e instanceof PlaybackApiError && e.status === 412 && e.current !== undefined) {
        let current: QueueHeader | undefined;
        try { current = parseQueue(e.current); } catch { current = undefined; }
        throw new PlaybackApiError(412, e.code, 'after_refresh', current);
      }
      throw e;
    }
  }

  /** Play next / Add to queue / insert after an entry. */
  addSegment(id: string, revision: string, placement: Placement, segment: SegmentInput) {
    return this.command(id, revision, 'POST', `/v1/queues/${enc(id)}/segments`, {placement, ...segment});
  }
  remove(id: string, revision: string, entryId: string) {
    return this.command(id, revision, 'DELETE', `/v1/queues/${enc(id)}/entries/${enc(entryId)}`);
  }
  move(id: string, revision: string, entryId: string, to: Readonly<{before: string} | {after: string}>) {
    return this.command(id, revision, 'POST', `/v1/queues/${enc(id)}/entries/${enc(entryId)}:move`, to);
  }
  /** Repeat and shuffle. Shuffle on makes the current entry position 0 of a new seeded order. */
  update(id: string, revision: string, change: Readonly<{repeat?: 'off' | 'one' | 'all'; shuffle?: Readonly<{on: boolean; seed?: string}>}>) {
    return this.command(id, revision, 'PATCH', `/v1/queues/${enc(id)}`, change);
  }
  /** Completion, Next, Previous or jump to an entry. Returns the next session when one starts. */
  advance(id: string, revision: string, reason: 'completion' | 'next' | 'previous' | 'entry', entryId?: string) {
    return this.command(id, revision, 'POST', `/v1/queues/${enc(id)}:advance`, {reason, ...(entryId ? {entryId} : {})});
  }
  /** Prepares the next entry's audio before the current one ends (spec §18.2): private, frame 0
   * only, expiring after 60 s, canceled by any other queue or session command. */
  async prepareNext(id: string, revision: string, current: Readonly<{sessionId: string; sessionGeneration: number}>, signal?: AbortSignal): Promise<PreparedNext> {
    const r = await call(this.http, {method: 'POST', path: `/v1/queues/${enc(id)}:prepare-next`, headers: {'If-Match': revision, 'Idempotency-Key': this.key()}, body: current, signal});
    return parsePreparedNext(r.body);
  }
  /** Commits the prepared next entry at the audio boundary (spec §18.3): the queue moves and a new
   * session starts, inheriting state, rate, quality and track. Idempotent by token. */
  async commitNext(id: string, token: string, signal?: AbortSignal): Promise<Readonly<{queue: QueueHeader; session: Session}>> {
    const r = await call(this.http, {method: 'POST', path: `/v1/queues/${enc(id)}:commit-next`, body: {token}, signal});
    const b = r.body as {queue?: unknown; session?: unknown} | undefined;
    if (!obj(b) || b.session === undefined) throw new ContractError('committed next');
    return Object.freeze({queue: parseQueue(b.queue), session: parseSession(b.session)});
  }
  /** One save request. A long queue is copied in the background: replaying the same `key` reports
   * progress (NEW-37). `saveQueueAsPlaylist` does the replaying. */
  async saveAsPlaylist(id: string, name: string, key: string = this.key(), signal?: AbortSignal): Promise<QueuePlaylistSave> {
    return parseQueuePlaylistSave((await call(this.http, {method: 'POST', path: `/v1/queues/${enc(id)}:save-as-playlist`, headers: {'Idempotency-Key': key}, body: {name}, signal})).body);
  }
  async discard(id: string): Promise<void> {
    await call(this.http, {method: 'DELETE', path: `/v1/queues/${enc(id)}`}, [204, 200, 404]);
  }
}

/**
 * One queue on screen: the header and a windowed collection of its entries. `queue.updated`
 * events (or a command's result) swap in the new header; entries refetch as a new generation
 * while the old ones stay on screen.
 */
export class QueueView {
  readonly entries: WindowedCollection<QueueEntry>;
  private client: QueueClient;
  private current: QueueHeader;
  private listeners = new Set<() => void>();

  constructor(client: QueueClient, header: QueueHeader, options: {pageSize?: number; maxResidentPages?: number} = {}) {
    this.client = client;
    this.current = header;
    const pageSize = Math.min(options.pageSize ?? 100, QUEUE_WINDOW_MAX);
    this.entries = createWindowedCollection<QueueEntry>({
      pageSize, maxResidentPages: options.maxResidentPages ?? 8, keyOf: e => e.entryId,
      fetchPage: (start, count, signal) => this.fetch(start, count, signal),
    });
  }

  private async fetch(start: number, count: number, signal: AbortSignal) {
    const page = await this.client.entries(this.current.id, start, count, signal);
    return {items: page.items, total: page.total ?? this.current.total};
  }

  get header(): QueueHeader { return this.current; }
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };

  /** Adopt a newer header (a command result or a `queue.updated` re-read). Older revisions are ignored. */
  adopt(header: QueueHeader, entriesChanged = true): void {
    if (header.id !== this.current.id || header.revision === this.current.revision) return;
    this.current = header;
    if (entriesChanged) this.entries.invalidate();
    for (const l of [...this.listeners]) l();
  }

  /** A `queue.updated` event for this queue: re-read the header if the revision moved. */
  async updated(revision?: string, signal?: AbortSignal): Promise<void> {
    if (revision !== undefined && revision === this.current.revision) return;
    this.adopt(await this.client.header(this.current.id, signal));
  }

  /**
   * Run a command with the current revision; on 412 adopt the server's header and let the caller
   * decide again (the returned promise rejects with the 412 so a UI can say "the queue changed").
   */
  async run<T extends {queue: QueueHeader}>(command: (id: string, revision: string) => Promise<T>): Promise<T> {
    try {
      const result = await command(this.current.id, this.current.revision);
      this.adopt(result.queue);
      return result;
    } catch (e) {
      if (e instanceof PlaybackApiError && e.status === 412 && e.current) this.adopt(e.current as QueueHeader);
      throw e;
    }
  }

  dispose() { this.entries.dispose(); this.listeners.clear(); }
}

/** NEW-37: a queue saved as a playlist. `saving` while a long queue is still being copied (`entries`
 * of `total` so far); `failed` with `errorCode` (`queue_changed`: the queue's order changed meanwhile). */
export type QueuePlaylistSave = Readonly<{playlistId: string; revision: string; entries: number; total?: number; state: 'saving' | 'saved' | 'failed'; errorCode?: string}>;
export function parseQueuePlaylistSave(v: unknown): QueuePlaylistSave {
  if (!obj(v) || typeof v.playlistId !== 'string' || !v.playlistId) throw new ContractError('saved playlist');
  const n = (x: unknown) => (typeof x === 'number' && Number.isSafeInteger(x) && x >= 0 ? x : undefined);
  // An unknown state is read as done: the playlist exists and nothing more can be learned by waiting.
  const state = v.state === 'saving' || v.state === 'failed' ? v.state : 'saved';
  return Object.freeze({playlistId: v.playlistId, revision: typeof v.revision === 'string' ? v.revision : '', entries: n(v.entries) ?? 0, total: n(v.total), state, errorCode: typeof v.errorCode === 'string' && v.errorCode ? v.errorCode : undefined});
}

/**
 * Saves a queue as a playlist and follows a long save to the end by replaying the same
 * Idempotency-Key (never a second save). Waits 1 s, growing to 5 s, between replays; each answer
 * is passed to `onProgress`. Resolves with the final answer (`saved` or `failed`).
 */
export async function saveQueueAsPlaylist(client: Pick<QueueClient, 'saveAsPlaylist'>, queueId: string, name: string, options: Readonly<{key?: string; onProgress?: (save: QueuePlaylistSave) => void; signal?: AbortSignal; waitMs?: (attempt: number) => number}> = {}): Promise<QueuePlaylistSave> {
  const key = options.key ?? idempotencyKey();
  const wait = options.waitMs ?? ((attempt: number) => Math.min(5000, 1000 * 1.5 ** attempt));
  for (let attempt = 0; ; attempt++) {
    const save = await client.saveAsPlaylist(queueId, name, key, options.signal);
    options.onProgress?.(save);
    if (save.state !== 'saving') return save;
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(done, wait(attempt));
      function done() { clearTimeout(timer); options.signal?.removeEventListener('abort', done); options.signal?.aborted ? reject(new DOMException('Aborted', 'AbortError')) : resolve(); }
      options.signal?.addEventListener('abort', done, {once: true});
    });
  }
}

/** The catalogue message for a save's current state (NEW-37): progress while saving, the result after. */
export function queueSaveMessage(save: QueuePlaylistSave): Readonly<{id: 'player.queueSaving' | 'player.queueSavingProgress' | 'player.queueSaved' | 'player.queueSaveChanged' | 'player.queueSaveFailed'; values?: Readonly<{done: number; total: number}>}> {
  if (save.state === 'saving') return save.total ? {id: 'player.queueSavingProgress', values: {done: Math.min(save.entries, save.total), total: save.total}} : {id: 'player.queueSaving'};
  if (save.state === 'failed') return {id: save.errorCode === 'queue_changed' ? 'player.queueSaveChanged' : 'player.queueSaveFailed'};
  return {id: 'player.queueSaved'};
}
