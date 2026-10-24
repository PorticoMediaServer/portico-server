import {ConsoleClient, type FeedbackReport, type FeedbackStatus} from '@core/console.ts';
import type {ReportTransport} from '@core/client-reports.ts';

export type FeedbackSnapshot = Readonly<{
  phase: 'idle' | 'loading' | 'ready' | 'error';
  reports: readonly FeedbackReport[];
  /** The server's own cursor; present only while it holds more reports. */
  next?: string;
}>;

const empty: FeedbackSnapshot = Object.freeze({phase: 'idle', reports: Object.freeze([])});

/**
 * Owner-scoped viewer feedback. The list, one report and the status/reply write all
 * go through the same console client and captured viewer fence as the rest of Settings.
 */
export class SettingsFeedbackSource {
  private state: FeedbackSnapshot = empty;
  private readonly listeners = new Set<() => void>();
  private readonly lifetime = new AbortController();
  private reading?: Promise<void>;
  private readonly console: ConsoleClient;
  constructor(transport: ReportTransport, serverId: string, viewerId: string, private readonly isCurrent: () => boolean) {
    this.console = new ConsoleClient(transport, {serverId, viewerId});
  }
  private current() {return !this.lifetime.signal.aborted && this.isCurrent();}
  getSnapshot = (): FeedbackSnapshot => this.current() ? this.state : empty;
  subscribe = (listener: () => void): (() => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private publish(state: FeedbackSnapshot) {if (this.current()) {this.state = Object.freeze(state);for (const listener of [...this.listeners]) listener();}}
  load = (): Promise<void> => {
    if (!this.current() || this.reading) return this.reading ?? Promise.resolve();
    const task = Promise.resolve().then(async () => {
      this.publish({...this.state, phase: 'loading'});
      try {
        const page = await this.console.reports(true);
        if (!this.current()) return;
        this.publish({phase: 'ready', reports: page.items, ...(page.nextCursor ? {next: page.nextCursor} : {})});
      } catch {if (this.current() && !this.lifetime.signal.aborted) this.publish({...this.state, phase: 'error'});}
      finally {this.reading = undefined;}
    });
    this.reading = task;return task;
  };
  /** The next page the server named, appended in its order. */
  more = async (): Promise<void> => {
    const cursor = this.state.next;
    if (!this.current() || this.reading || !cursor) return;
    this.publish({...this.state, phase: 'loading'});
    try {
      const page = await this.console.reports(true, cursor);
      if (!this.current()) return;
      const known = new Set(this.state.reports.map(report => report.id));
      this.publish({...this.state, phase: 'ready', reports: [...this.state.reports, ...page.items.filter(report => !known.has(report.id))], ...(page.nextCursor ? {next: page.nextCursor} : {next: undefined})});
    } catch {if (this.current() && !this.lifetime.signal.aborted) this.publish({...this.state, phase: 'error'});}
  };
  /** One report exactly as the server keeps it, for the dialog that is open now. */
  async read(id: string, signal: AbortSignal): Promise<FeedbackReport> {
    if (!this.current() || !this.state.reports.some(report => report.id === id)) throw new Error('The selected report is no longer listed.');
    return this.console.report(id, true);
  }
  /**
   * Save status and reply on the report the dialog captured. If the report moved while the
   * dialog was open, the owner's edit is carried over to the newer revision once (ruling: the
   * change is silent), and the answer is the row the list then shows.
   */
  async triage(report: FeedbackReport, status: FeedbackStatus, reply: string, signal: AbortSignal): Promise<FeedbackReport> {
    if (!this.current()) throw new DOMException('The feedback scope ended.', 'AbortError');
    let current = report;
    for (let attempt = 0; attempt < 2; attempt++) {
      try {
        const saved = await this.console.triage(current.id, current.revision, status, reply);
        if (!this.current()) throw new DOMException('The feedback scope ended.', 'AbortError');
        this.publish({...this.state, reports: this.state.reports.map(row => row.id === saved.id ? saved : row)});
        return saved;
      } catch (error) {
        const code = (error as {code?: string}).code;
        if (attempt === 0 && (code === 'console_conflict' || code === 'revision_mismatch')) {
          signal.throwIfAborted();
          current = await this.console.report(current.id, true);
          signal.throwIfAborted();
          if (!this.current()) throw new DOMException('The feedback scope ended.', 'AbortError');
          continue;
        }
        throw error;
      }
    }
    throw new Error('feedback_conflict');
  }
  retry = (): Promise<void> => {this.state = empty;return this.load();};
  dispose(): void {if (!this.lifetime.signal.aborted) {this.lifetime.abort();this.listeners.clear();this.console.dispose();this.state = empty;}}
}
