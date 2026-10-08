import type {ByteReader} from './types';

/**
 * The compressed-bytes cache and the Range reader over it (§9.6 rule 1): files are fetched in
 * aligned blocks with `Range`, kept in one bounded LRU shared by every track (so a track played
 * again, or the next track fetched ahead, costs no second download), and evicted least recently
 * used first. Memory is bounded by the byte budget, never by library or queue size.
 */
export const BLOCK = 256 * 1024;

export class ByteCache {
  private readonly blocks = new Map<string, Uint8Array>();
  private used = 0;
  readonly budget: number;
  constructor(budget = 64 * 1024 * 1024) { this.budget = budget; }
  get(key: string): Uint8Array | undefined {
    const v = this.blocks.get(key);
    if (v) { this.blocks.delete(key); this.blocks.set(key, v); } // most recent last
    return v;
  }
  put(key: string, value: Uint8Array) {
    const old = this.blocks.get(key);
    if (old) { this.used -= old.length; this.blocks.delete(key); }
    this.blocks.set(key, value);
    this.used += value.length;
    for (const [k, v] of this.blocks) {
      if (this.used <= this.budget) break;
      this.blocks.delete(k); this.used -= v.length;
    }
  }
  get size() { return this.used; }
  /** Drops everything (sign-out, or a switch of profile or server: nothing fetched for one viewer
   * outlives it in this tab, as the native engine's cache does). */
  clear() { this.blocks.clear(); this.used = 0; }
  /** Drops one file's blocks (its media changed). */
  forget(prefix: string) { for (const [k, v] of this.blocks) if (k.startsWith(prefix + ':')) { this.blocks.delete(k); this.used -= v.length; } }
}

/** One compressed-bytes cache for every track in this tab (64 MB). */
export const audioByteCache = new ByteCache(64 * 1024 * 1024);

export class MediaReadError extends Error {
  readonly code: string;
  readonly status?: number;
  constructor(code: string, message: string, status?: number) { super(message); this.code = code; this.status = status; }
}

export type RangeFetch = (url: string, init: RequestInit) => Promise<Response>;

/**
 * Reads one file by `Range`. `limit` caps the bytes this reader may ask for in total (a prepared,
 * private presentation serves at most `prefetchBytes`, §18.1); reads past it wait until
 * `unlock()` (at commit) or fail when the reader is closed.
 */
export class RangeReader implements ByteReader {
  readonly size?: number;
  private readonly url: string;
  private readonly key: string;
  private readonly cache: ByteCache;
  private readonly fetcher: RangeFetch;
  private readonly inflight = new Map<number, Promise<Uint8Array>>();
  private limit: number;
  private requested = 0;
  private waiting: (() => void)[] = [];
  private readonly abort = new AbortController();
  constructor(o: {url: string; key: string; size?: number; cache: ByteCache; fetch?: RangeFetch; limit?: number}) {
    this.url = o.url; this.key = o.key; this.size = o.size; this.cache = o.cache;
    this.fetcher = o.fetch ?? ((u, i) => fetch(u, i));
    this.limit = o.limit ?? Infinity;
  }
  /** At commit: the whole file may now be fetched. */
  unlock() { this.limit = Infinity; const w = this.waiting; this.waiting = []; for (const f of w) f(); }
  close() { this.abort.abort(); const w = this.waiting; this.waiting = []; for (const f of w) f(); }
  get locked() { return this.limit !== Infinity; }

  private async block(index: number, signal?: AbortSignal): Promise<Uint8Array> {
    const key = `${this.key}:${index}`;
    const hit = this.cache.get(key);
    if (hit) return hit;
    const pending = this.inflight.get(index);
    if (pending) return pending;
    const task = (async () => {
      const start = index * BLOCK, end = this.size !== undefined ? Math.min(this.size, start + BLOCK) : start + BLOCK;
      while (this.requested + (end - start) > this.limit && !this.abort.signal.aborted) await new Promise<void>(r => this.waiting.push(r));
      if (this.abort.signal.aborted) throw new MediaReadError('cancelled', 'Reading was canceled.');
      this.requested += end - start;
      const response = await this.fetcher(this.url, {headers: {Range: `bytes=${start}-${end - 1}`}, signal: signal ? anySignal(signal, this.abort.signal) : this.abort.signal, credentials: 'omit', cache: 'no-store', redirect: 'error'});
      if (response.status === 416) return new Uint8Array(0);
      if (response.status === 403) throw new MediaReadError('prepared_limit', 'The prepared track can’t be read further before it is committed.', 403);
      if (!response.ok) throw new MediaReadError('media_unavailable', 'The audio file could not be read.', response.status);
      const bytes = new Uint8Array(await response.arrayBuffer());
      // A server that ignored Range sends the whole file: take this block from it.
      const data = response.status === 200 ? bytes.slice(start, end) : bytes;
      this.cache.put(key, data);
      return data;
    })().finally(() => this.inflight.delete(index));
    this.inflight.set(index, task);
    return task;
  }

  async read(offset: number, length: number, signal?: AbortSignal): Promise<Uint8Array> {
    if (this.size !== undefined) length = Math.max(0, Math.min(length, this.size - offset));
    if (length <= 0) return new Uint8Array(0);
    const first = Math.floor(offset / BLOCK), last = Math.floor((offset + length - 1) / BLOCK);
    const parts = await Promise.all(Array.from({length: last - first + 1}, (_, i) => this.block(first + i, signal)));
    const out = new Uint8Array(length);
    let written = 0;
    for (let i = 0; i < parts.length; i++) {
      const part = parts[i]!, blockStart = (first + i) * BLOCK;
      const from = Math.max(0, offset - blockStart), to = Math.min(part.length, offset + length - blockStart);
      if (to <= from) break;
      out.set(part.subarray(from, to), written);
      written += to - from;
      if (part.length < BLOCK) break; // end of file
    }
    return written === length ? out : out.subarray(0, written);
  }

  /** Fetches the rest of the file into the cache ahead of need (the next track, before its
   * boundary), when it fits the cache's budget. */
  async prefetchAll(signal?: AbortSignal): Promise<void> {
    if (this.size === undefined || this.size > this.cache.budget / 2) return;
    for (let i = 0; i * BLOCK < this.size; i++) { if (signal?.aborted) return; await this.block(i, signal); }
  }
}

function anySignal(a: AbortSignal, b: AbortSignal): AbortSignal {
  const any = (AbortSignal as unknown as {any?: (s: AbortSignal[]) => AbortSignal}).any;
  if (any) return any.call(AbortSignal, [a, b]);
  const c = new AbortController();
  const stop = () => c.abort();
  a.addEventListener('abort', stop, {once: true}); b.addEventListener('abort', stop, {once: true});
  return c.signal;
}
