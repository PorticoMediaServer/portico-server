import {randomId} from '../random-id.ts';
/**
 * The HTTP seam for playback v1. Modules speak in `V1Request`/`V1Response` so the same code runs
 * against the real server (an adapter over `HttpLocalApi`/`fetch`, phase 2 wiring) and
 * `FakePlaybackServer`. Non-2xx responses become `PlaybackApiError` with the style guide's error
 * fields (§4.6): `code`, `retry` class, `current` (on 412) and `Retry-After`.
 */
export type V1Request = Readonly<{method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'; path: string; headers?: Readonly<Record<string, string>>; body?: unknown; signal?: AbortSignal}>;
export type V1Response = Readonly<{status: number; headers: Readonly<Record<string, string>>; body?: unknown}>;
export interface V1Http {
  /** Resolves with any HTTP response; rejects only when there is no response (offline, aborted). */
  send(request: V1Request): Promise<V1Response>;
}

export type RetryClass = 'never' | 'same_request' | 'after_refresh' | 'after_reauth';

export class PlaybackApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly retry: RetryClass;
  readonly retryAfterMs?: number;
  /** The resource's current representation, when the server sends it (412, some 409s). */
  readonly current?: unknown;
  constructor(status: number, code: string, retry: RetryClass, current?: unknown, retryAfterMs?: number) {
    super(code);
    this.name = 'PlaybackApiError';
    this.status = status;
    this.code = code;
    this.retry = retry;
    this.current = current;
    this.retryAfterMs = retryAfterMs;
  }
}

/** Case-insensitive header read. */
export function header(r: V1Response, name: string): string | undefined {
  const want = name.toLowerCase();
  for (const [k, v] of Object.entries(r.headers)) if (k.toLowerCase() === want) return v;
  return undefined;
}

function retryAfter(r: V1Response, body: {retryAfterSeconds?: unknown} | undefined): number | undefined {
  const h = header(r, 'Retry-After');
  if (h && /^\d+$/.test(h)) return Number(h) * 1000;
  return typeof body?.retryAfterSeconds === 'number' ? body.retryAfterSeconds * 1000 : undefined;
}

const defaultRetry = (status: number): RetryClass => (status === 412 || status === 409 ? 'after_refresh' : status === 401 ? 'after_reauth' : status >= 500 || status === 429 ? 'same_request' : 'never');

/** Send and require one of `ok` statuses (default 2xx); anything else throws `PlaybackApiError`. */
export async function call(http: V1Http, request: V1Request, ok?: readonly number[]): Promise<V1Response> {
  const r = await http.send(request);
  const good = ok ? ok.includes(r.status) : r.status >= 200 && r.status < 300;
  if (good) return r;
  const e = (r.body as {error?: {code?: unknown; retry?: unknown; current?: unknown; retryAfterSeconds?: unknown}} | undefined)?.error;
  const code = typeof e?.code === 'string' ? e.code : r.status === 404 ? 'not_found' : 'request_failed';
  const retry = (['never', 'same_request', 'after_refresh', 'after_reauth'] as const).includes(e?.retry as RetryClass) ? e!.retry as RetryClass : defaultRetry(r.status);
  throw new PlaybackApiError(r.status, code, retry, e?.current, retryAfter(r, e));
}

/** A fresh `Idempotency-Key` (16–128 chars, `[A-Za-z0-9_-]`, style guide §4.7.3). */
export function idempotencyKey(random: () => string = () => randomId()): string {
  return random().replace(/[^A-Za-z0-9_-]/g, '').slice(0, 128).padEnd(16, '0');
}

export const enc = encodeURIComponent;
