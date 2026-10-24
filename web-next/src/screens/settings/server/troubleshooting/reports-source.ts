import {deleteClientReport, listClientReports, readClientReport, type ClientReport, type ReportTransport} from '@core/client-reports.ts';
import {readCatalogueFailures, type CatalogueFailures} from '@core/catalogue-failures.ts';
import {ConsoleClient, type EvidenceRecord} from '@core/console.ts';

export type RecordLane = 'runtime' | 'client' | 'audit';
export type RecordsSlice = Readonly<{lane: RecordLane; items: readonly EvidenceRecord[]; next?: string}>;
export type ReportsSnapshot = Readonly<{
  phase: 'idle' | 'loading' | 'ready' | 'error';
  reports: readonly ClientReport[];
  /** The server's own cursor; present only while it holds more reports. */
  next?: string;
  /** Titles a catalogue rebuild could not finish; absent until the server answers. */
  setAside?: CatalogueFailures;
  /** What the server noted on the selected lane, a page at a time. */
  records?: RecordsSlice;
}>;

const empty: ReportsSnapshot = Object.freeze({phase: 'idle', reports: Object.freeze([])});

/**
 * Owner-scoped reads for the drawn Client reports panel and its Titles set aside
 * section. One report's body is returned to the caller and never cached here: the
 * log can name titles and devices, so it lives only inside the open dialog.
 */
export class SettingsReportsSource {
  private state: ReportsSnapshot = empty;
  private readonly listeners = new Set<() => void>();
  private readonly lifetime = new AbortController();
  private reading?: Promise<void>;
  private readonly console: ConsoleClient;
  constructor(private readonly transport: ReportTransport, private readonly serverId: string, viewerId: string, private readonly isCurrent: () => boolean) {
    this.console = new ConsoleClient(transport, {serverId, viewerId});
  }
  private current() {return !this.lifetime.signal.aborted && this.isCurrent();}
  getSnapshot = (): ReportsSnapshot => this.current() ? this.state : empty;
  subscribe = (listener: () => void): (() => void) => {this.listeners.add(listener);return () => {this.listeners.delete(listener);};};
  private publish(state: ReportsSnapshot) {if (this.current()) {this.state = Object.freeze(state);for (const listener of [...this.listeners]) listener();}}
  private signal() {return this.lifetime.signal;}
  load = (): Promise<void> => {
    if (!this.current() || this.reading) return this.reading ?? Promise.resolve();
    const task = Promise.resolve().then(async () => {
      this.publish({...this.state, phase: 'loading'});
      try {
        const [page, setAside, records] = await Promise.all([
          listClientReports(this.transport, this.serverId, '', this.signal()),
          readCatalogueFailures(this.transport, this.signal()).catch(() => undefined),
          this.console.records('runtime').catch(() => undefined),
        ]);
        if (!this.current()) return;
        this.publish({phase: 'ready', reports: page.items, ...(page.nextCursor ? {next: page.nextCursor} : {}), ...(setAside ? {setAside} : {}),
          ...(records ? {records: {lane: 'runtime' as const, items: records.items, ...(records.nextCursor ? {next: records.nextCursor} : {})}} : {})});
      } catch {if (this.current() && !this.lifetime.signal.aborted) this.publish({...this.state, phase: 'error'});}
      finally {this.reading = undefined;}
    });
    this.reading = task;return task;
  };
  /** The next page the server named, appended in its order; no page is ever guessed. */
  more = async (): Promise<void> => {
    const cursor = this.state.next;
    if (!this.current() || this.reading || !cursor) return;
    this.publish({...this.state, phase: 'loading'});
    try {
      const page = await listClientReports(this.transport, this.serverId, cursor, this.signal());
      if (!this.current()) return;
      const known = new Set(this.state.reports.map(report => report.id));
      this.publish({...this.state, phase: 'ready', reports: [...this.state.reports, ...page.items.filter(report => !known.has(report.id))], ...(page.nextCursor ? {next: page.nextCursor} : {next: undefined})});
    } catch {if (this.current() && !this.lifetime.signal.aborted) this.publish({...this.state, phase: 'error'});}
  };
  /** One report exactly as the server keeps it, for the dialog that is open now. */
  async read(id: string, signal: AbortSignal): Promise<ClientReport> {
    if (!this.current() || !this.state.reports.some(report => report.id === id)) throw new Error('The selected report is no longer listed.');
    return readClientReport(this.transport, this.serverId, id, signal);
  }
  /** Delete the listed report the owner chose, then reread the list the server answers. */
  async remove(id: string): Promise<boolean> {
    if (!this.current() || !this.state.reports.some(report => report.id === id)) return false;
    this.publish({...this.state, phase: 'loading'});
    try {
      await deleteClientReport(this.transport, id, this.signal());
      if (!this.current()) return false;
      const page = await listClientReports(this.transport, this.serverId, '', this.signal());
      if (!this.current()) return false;
      this.publish({...this.state, phase: 'ready', reports: page.items, ...(page.nextCursor ? {next: page.nextCursor} : {next: undefined})});
      return true;
    } catch {if (this.current() && !this.lifetime.signal.aborted) this.publish({...this.state, phase: 'error'});return false;}
  }
  /** Switch the records lane from its first page; the list keeps only that lane's rows. */
  selectLane = async (lane: RecordLane): Promise<void> => {
    const shown = this.state.records;
    if (!this.current() || (shown?.lane === lane && shown.items.length > 0)) return;
    try {
      const page = await this.console.records(lane);
      if (!this.current()) return;
      this.publish({...this.state, records: {lane, items: page.items, ...(page.nextCursor ? {next: page.nextCursor} : {})}});
    } catch {if (this.current() && !this.lifetime.signal.aborted) this.publish({...this.state, phase: 'error'});}
  };
  /** The next page of the selected lane, only when the server named one. */
  moreRecords = async (): Promise<void> => {
    const current = this.state.records;
    if (!this.current() || !current?.next) return;
    try {
      const page = await this.console.records(current.lane, current.next);
      if (!this.current()) return;
      const known = new Set(current.items.map(item => item.sequence));
      this.publish({...this.state, records: {lane: current.lane, items: [...current.items, ...page.items.filter(item => !known.has(item.sequence))], ...(page.nextCursor ? {next: page.nextCursor} : {})}});
    } catch {if (this.current() && !this.lifetime.signal.aborted) this.publish({...this.state, phase: 'error'});}
  };
  /** Back to the first page of the selected lane. */
  latestRecords = async (): Promise<void> => {
    const current = this.state.records;
    if (!this.current() || !current) return;
    try {
      const page = await this.console.records(current.lane);
      if (!this.current()) return;
      this.publish({...this.state, records: {lane: current.lane, items: page.items, ...(page.nextCursor ? {next: page.nextCursor} : {})}});
    } catch {if (this.current() && !this.lifetime.signal.aborted) this.publish({...this.state, phase: 'error'});}
  };
  /** Re-read the titles a rebuild set aside; the server tries them again by itself. */
  checkSetAside = async (): Promise<void> => {
    if (!this.current()) return;
    try {
      const setAside = await readCatalogueFailures(this.transport, this.signal());
      if (this.current()) this.publish({...this.state, setAside});
    } catch {if (this.current() && !this.lifetime.signal.aborted) this.publish({...this.state, phase: 'error'});}
  };
  retry = (): Promise<void> => {this.state = empty;return this.load();};
  dispose(): void {if (!this.lifetime.signal.aborted) {this.lifetime.abort();this.listeners.clear();this.console.dispose();this.state = empty;}}
}
