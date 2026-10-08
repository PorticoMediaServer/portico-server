/**
 * Now Playing administration (spec §14): the active sessions with their delivery decisions, a
 * terminate command that shows the viewer a message. Playback history is the console's
 * `/v1/admin/playback/history` (console.ts). The live list refreshes
 * on `admin.sessions` events (coalesced), never on a timer.
 */
import {call, enc, idempotencyKey, type V1Http} from './http.ts';
import {parseAdminSession, parsePage, type AdminSession} from './types.ts';

export class AdminSessionsClient {
  private http: V1Http;
  private key: () => string;
  constructor(http: V1Http, key: () => string = () => idempotencyKey()) { this.http = http; this.key = key; }

  async list(cursor?: string, limit = 50, signal?: AbortSignal) {
    const q = new URLSearchParams({limit: String(Math.max(1, Math.min(limit, 200)))});
    if (cursor) q.set('cursor', cursor);
    return parsePage((await call(this.http, {method: 'GET', path: `/v1/admin/sessions?${q}`, signal})).body, parseAdminSession);
  }

  /** End a viewer's session; the optional `message` is shown to them. */
  async terminate(sessionId: string, message?: string, signal?: AbortSignal): Promise<void> {
    const text = message?.trim();
    await call(this.http, {method: 'POST', path: `/v1/admin/sessions/${enc(sessionId)}:terminate`, headers: {'Idempotency-Key': this.key()}, body: text ? {message: text.slice(0, 500)} : {}, signal}, [200, 202, 204, 404]);
  }

}

export type NowPlayingSnapshot = Readonly<{sessions: readonly AdminSession[]; loading: boolean; error?: unknown; updatedAt?: number}>;

/**
 * The live Now Playing list for the admin console. Active sessions are few (bounded by stream
 * caps), so the store reads all pages; it refreshes on `admin.sessions` events, coalescing bursts.
 */
export class NowPlayingStore {
  private client: AdminSessionsClient;
  private state: NowPlayingSnapshot = Object.freeze({sessions: Object.freeze([]), loading: false});
  private listeners = new Set<() => void>();
  private inFlight?: Promise<void>;
  private again = false;
  private now: () => number;
  private maxPages: number;

  constructor(client: AdminSessionsClient, options: {now?: () => number; maxPages?: number} = {}) {
    this.client = client;
    this.now = options.now ?? Date.now;
    this.maxPages = options.maxPages ?? 10;
  }

  getSnapshot = (): NowPlayingSnapshot => this.state;
  subscribe = (fn: () => void) => { this.listeners.add(fn); return () => { this.listeners.delete(fn); }; };
  private publish(p: Partial<NowPlayingSnapshot>) { this.state = Object.freeze({...this.state, ...p}); for (const l of [...this.listeners]) l(); }

  /** Re-read now; calls during a read coalesce into one more read afterwards. */
  refresh(): Promise<void> {
    if (this.inFlight) { this.again = true; return this.inFlight; }
    this.inFlight = (async () => {
      do {
        this.again = false;
        this.publish({loading: true});
        try {
          const all: AdminSession[] = [];
          let cursor: string | undefined;
          for (let i = 0; i < this.maxPages; i++) {
            const page = await this.client.list(cursor);
            all.push(...page.items);
            if (!page.nextCursor) break;
            cursor = page.nextCursor;
          }
          this.publish({sessions: Object.freeze(all), loading: false, error: undefined, updatedAt: this.now()});
        } catch (e) {
          this.publish({loading: false, error: e});
        }
      } while (this.again);
    })().finally(() => { this.inFlight = undefined; });
    return this.inFlight;
  }

  async terminate(sessionId: string, message?: string): Promise<void> {
    await this.client.terminate(sessionId, message);
    this.publish({sessions: Object.freeze(this.state.sessions.filter(s => s.id !== sessionId))});
    void this.refresh();
  }
}
