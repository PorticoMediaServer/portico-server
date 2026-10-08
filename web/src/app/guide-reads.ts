import type {ChannelApi} from '@core/channel-guide.ts';

/**
 * Identical guide reads within a few seconds share one request: the grid's pages and tiles, the
 * chip catalog and the source probe all read `/v1/guide` windows, and must never refetch an
 * unchanged window (the server rate-limits them). Per API instance, so viewers never share.
 */
const GUIDE_REUSE_MS = 10_000;
let guideReads = new WeakMap<ChannelApi, Map<string, {at: number; result: Promise<unknown>}>>();
/** After a change (a recording, a preference, a new query), the next read goes to the server. */
export function forgetGuideReads() { guideReads = new WeakMap(); }
export function dedupedGuideApi(api: ChannelApi, now: () => number = Date.now): ChannelApi {
  return {
    request<T>(path: string, method?: string, body?: unknown, signal?: AbortSignal): Promise<T> {
      if ((method ?? 'GET') !== 'GET' || body !== undefined || !path.startsWith('/v1/guide')) return api.request<T>(path, method, body, signal);
      let reads = guideReads.get(api);
      if (!reads) guideReads.set(api, (reads = new Map()));
      const at = now();
      for (const [key, read] of reads) if (at - read.at > GUIDE_REUSE_MS) reads.delete(key);
      let read = reads.get(path);
      if (!read) {
        read = {at, result: api.request<T>(path)};
        const mine = read;
        read.result.catch(() => { if (reads!.get(path) === mine) reads!.delete(path); });
        reads.set(path, read);
      }
      const shared = read.result as Promise<T>;
      if (!signal) return shared;
      if (signal.aborted) return Promise.reject(signal.reason);
      return new Promise<T>((resolve, reject) => {
        const abort = () => reject(signal.reason);
        signal.addEventListener('abort', abort, {once: true});
        shared.then(v => { signal.removeEventListener('abort', abort); resolve(v); }, e => { signal.removeEventListener('abort', abort); reject(e); });
      });
    },
  };
}

