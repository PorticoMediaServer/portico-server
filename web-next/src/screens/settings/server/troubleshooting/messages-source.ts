import type {ReportTransport} from '@core/client-reports.ts';
import {parseLogPage, parseLogEvent, parseLogSettings, logLevels, type LogRecord, type LogLevel, type LogCategory, type LogSettings} from '@core/server-administration.ts';
import {readEventStream} from '@core/sse.ts';
import {randomId} from '@core/random-id.ts';
import {routePath} from '../../../../../../packages/contracts/src/routes.ts';

export type MessageConnection = Readonly<{open: (headers: Record<string, string>, signal: AbortSignal) => Promise<Response>}>;
export type MessagesSnapshot = Readonly<{phase: 'idle'|'loading'|'ready'|'error'; lines: readonly LogRecord[]; level: LogLevel|''; category: LogCategory|''; live: boolean; reconnecting: boolean; readAt?: number; settings?: LogSettings; changing: boolean}>;
const empty: MessagesSnapshot = Object.freeze({phase: 'idle', lines: [], level: '', category: '', live: false, reconnecting: false, changing: false});
export function appendMessage(lines: readonly LogRecord[], record: LogRecord): readonly LogRecord[] {
  return lines.some(line => line.sequence === record.sequence) ? lines : [...lines.slice(-499), record];
}
export function messageRetryMs(attempt: number, retryAfter: string|null, now = Date.now()): number {
  if (retryAfter !== null) {
    const seconds = Number(retryAfter), date = Date.parse(retryAfter);
    if (retryAfter.trim() && Number.isFinite(seconds) && seconds >= 0) return Math.min(seconds * 1000, 2147483647);
    if (Number.isFinite(date)) return Math.min(Math.max(0, date - now), 2147483647);
  }
  return 1000 * 2 ** Math.min(Math.max(attempt, 1), 6);
}
/** One captured server/viewer lifetime, bounded to 500 redacted server records. */
export class SettingsMessagesSource {
  private state: MessagesSnapshot = empty;
  private readonly listeners = new Set<() => void>();
  private readonly lifetime = new AbortController();
  private read?: AbortController;
  private tail?: AbortController;
  private timer?: ReturnType<typeof setTimeout>;
  private lastId = '';
  constructor(private readonly transport: ReportTransport, private readonly connection: MessageConnection|undefined, private readonly isCurrent: () => boolean) {}
  private current() {return !this.lifetime.signal.aborted && this.isCurrent();}
  getSnapshot = (): MessagesSnapshot => this.current() ? this.state : empty;
  subscribe = (listener: () => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private publish(patch: Partial<MessagesSnapshot>) {if (this.current()) {this.state = Object.freeze({...this.state, ...patch});for (const listener of this.listeners) listener();}}
  load = async (): Promise<void> => {
    if (!this.current()) return;
    this.read?.abort(); const read = new AbortController();this.read = read;
    this.publish({phase: 'loading'});
    const {level, category} = this.state;
    try {
      const query = new URLSearchParams({limit: '200', ...(level ? {level} : {}), ...(category ? {category} : {})});
      const [page, settings] = await Promise.all([
        this.transport.request<unknown>(routePath('get_admin_logs') + '?' + query, 'GET', undefined, read.signal).then(parseLogPage),
        this.transport.request<unknown>(routePath('get_admin_logs_settings'), 'GET', undefined, read.signal).then(parseLogSettings),
      ]);
      if (read.signal.aborted || !this.current()) return;
      this.publish({phase: 'ready', lines: [...page.items].reverse(), settings, readAt: Date.now()});
    } catch {if (!read.signal.aborted) this.publish({phase: 'error'});}
  };
  select = async (level: LogLevel|'', category: LogCategory|'') => {
    const live = this.state.live;this.stopTail();this.lastId = '';
    this.publish({level, category, lines: []});await this.load();
    if (live && this.current()) this.startTail();
  };
  setLive = (live: boolean) => {this.stopTail();this.publish({live, reconnecting: false});if (live) this.startTail();};
  private stopTail() {this.tail?.abort();this.tail = undefined;clearTimeout(this.timer);}
  private startTail() {
    if (!this.current() || !this.connection || !this.state.live) return;
    const controller = new AbortController();this.tail = controller;let attempt = 0;
    const active = () => this.current() && !controller.signal.aborted && this.state.live;
    const connect = async () => {
      let retryAfter: string|null = null;
      try {
        const response = await this.connection!.open({Accept: 'text/event-stream', ...(this.lastId ? {'Last-Event-ID': this.lastId} : {})}, controller.signal);
        if (!active()) {await response.body?.cancel();return;}
        if (!response.ok) {
          if (response.status === 429) retryAfter = response.headers.get('Retry-After');
          await response.body?.cancel();
          // Authentication/permission failures need a fresh user action, never an endless retry.
          if (response.status === 401 || response.status === 403) {this.publish({live: false, reconnecting: false, phase: 'error'});return;}
          throw new Error('Stream refused.');
        }
        this.publish({reconnecting: false});attempt = 0;
        await readEventStream(response, event => {
          if (!active()) return;
          if (event.id) this.lastId = event.id;
          if (event.event === 'reset') {this.publish({lines: []});return;}
          if (event.event !== 'log' && event.event !== 'message') return;
          let record: LogRecord;try {record = parseLogEvent(event.data);} catch {return;}
          const {level, category} = this.state;
          if (category && record.category !== category || level && logLevels.indexOf(record.level) > logLevels.indexOf(level)) return;
          this.publish({lines: appendMessage(this.state.lines, record)});
        }, controller.signal);
      } catch {if (!active()) return;}
      if (!active()) return;
      this.publish({reconnecting: true});attempt++;
      this.timer = setTimeout(() => {void connect();}, messageRetryMs(attempt, retryAfter));
    };
    void connect();
  }
  detail = async (on: boolean): Promise<void> => {
    if (!this.current() || this.state.changing) return;
    this.publish({changing: true});
    try {
      await this.transport.request(routePath('post_admin_logs_debug_window'), 'POST', {operationId: randomId(), minutes: on ? 30 : 0}, this.lifetime.signal);
      if (this.current()) await this.load();
    } finally {this.publish({changing: false});}
  };
  pause = () => {this.stopTail();this.read?.abort();this.publish({live: false, reconnecting: false});};
  dispose = () => {this.pause();this.lifetime.abort();this.state = empty;this.listeners.clear();};
}
