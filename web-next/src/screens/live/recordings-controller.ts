import {DVRClient, DVRModel, dvrMessage, isAccessError,
  type DVRPage, type DVRSummary, type Recording, type RecordingDetail, type RecordingGroups, type RecordingStorage} from '@core/dvr.ts';
import type {ChannelApi} from '@core/channel-guide.ts';

/** The four Recording destinations this door reads. Series rules are a different
 * generation and stay outside it. */
export const recordingsViews = ['upcoming', 'recorded', 'history', 'storage'] as const;
export type RecordingsView = typeof recordingsViews[number];
export type RecordingsSort = 'recent' | 'title' | 'oldest';
/** The exact resident query. A cursor is the server's opaque token, never rebuilt here. */
export type RecordingsQuery = Readonly<{view: RecordingsView; cursor?: string; recordingId?: string}>;
export type RecordingsPhase = 'idle' | 'loading' | 'ready' | 'error' | 'retired';
/** What this viewer actually holds: the server's own recording page, detail and
 * storage facts, plus its own genuine summary/group aggregates. Nothing is
 * derived from the loaded page. */
export type RecordingsSnapshot = Readonly<{
  query: RecordingsQuery;
  phase: RecordingsPhase;
  page: DVRPage | null;
  detail: RecordingDetail | null;
  storage: RecordingStorage | null;
  summary: DVRSummary | null;
  groups: RecordingGroups | null;
  /** The order the resident groups were read with; `groups.nextCursor` is the server's. */
  groupsSort: RecordingsSort;
  accessDenied: boolean;
  error: string;
}>;
export type RecordingsControllerOptions = Readonly<{
  api: ChannelApi;
  serverId: string;
  /** The existing DVR receipt generator; 48 lowercase hex, never a UUID alias. */
  requestId: () => Promise<string>;
  /** The owning route/principal lease. */
  signal: AbortSignal;
  /** The borrowed family/profile/role/navigation epoch that owns this screen. */
  isCurrent: () => boolean;
}>;

const sorts: readonly RecordingsSort[] = ['recent', 'title', 'oldest'];
const receiptText = /^[a-f0-9]{48}$/;
const recordingText = /^[a-f0-9]{64}$/;
const cancelled = () => new DOMException('recordings-scope-closed', 'AbortError');
const invalid = () => Object.assign(new Error('recordings-query-invalid'), {code: 'recordings-query-invalid'});
const isCancelled = (failure: unknown) => failure instanceof DOMException && failure.name === 'AbortError';

/** Headless, captured-parent adapter for the Recordings door. Construction performs
 * no read and mints nothing; only a mounted caller starts a bounded request. The
 * same DVRClient/DVRModel wire is used, and every read is fenced by the borrowed
 * signal and authority epoch before and after it settles. */
export function createRecordingsController(input: RecordingsControllerOptions) {
  // Captured once: a later caller cannot swap the API, the server or the receipt source.
  const {api: sourceApi, serverId, signal, isCurrent} = input;
  const requestId = input.requestId;
  const life = new AbortController();
  const listeners = new Set<() => void>();
  const api: ChannelApi = {
    async request<T>(path: string, method?: string, body?: unknown, requestSignal?: AbortSignal): Promise<T> {
      if (!current()) throw cancelled();
      const active = requestSignal ? AbortSignal.any([life.signal, requestSignal]) : life.signal;
      const value = await sourceApi.request<T>(path, method, body, active);
      if (!current()) throw cancelled();
      return value;
    },
  };
  const model = new DVRModel(new DVRClient(api, serverId, requestId));
  let closed = false, started = false, stopModel: (() => void) | undefined;
  /** Bumped by a new page/detail/storage load and by disposal, so an older load cannot land. */
  let loadSerial = 0;
  /** Bumped by a lease change (navigation/authority) and disposal only. Summary,
   * group and receipt reads are profile-wide and may finish across a local
   * destination change on the same lease; this fences them across actual retirement. */
  let epoch = 0, summarySerial = 0, groupsSerial = 0;
  let loading = false, accessDenied = false, errorText = '';
  let query: RecordingsQuery = Object.freeze({view: 'upcoming'});
  let page: DVRPage | null = null, detail: RecordingDetail | null = null, storage: RecordingStorage | null = null;
  let summary: DVRSummary | null = null, groups: RecordingGroups | null = null, groupsSort: RecordingsSort = 'recent';
  let state: RecordingsSnapshot = Object.freeze({query, phase: 'idle', page: null, detail: null, storage: null, summary: null, groups: null,
    groupsSort: 'recent', accessDenied: false, error: ''});

  function notify() { for (const listener of [...listeners]) listener(); }
  function current() {
    if (closed) return false;
    if (signal.aborted || !isCurrent()) { retire(); return false; }
    return true;
  }
  function derive(): RecordingsSnapshot {
    const snapshot = model.getSnapshot();
    const error = closed ? '' : errorText || (snapshot.error ?? '');
    const phase: RecordingsPhase = closed ? 'retired' : loading ? 'loading' : error ? 'error'
      : page || detail || storage || summary || groups ? 'ready' : 'idle';
    return Object.freeze({query, phase, page, detail, storage, summary, groups, groupsSort,
      accessDenied: closed ? false : accessDenied || snapshot.accessDenied, error});
  }
  function publish() {
    if (closed) return;
    state = derive();
    notify();
  }
  /** The resident page/detail/storage objects of one destination generation. */
  function dropResident() {
    page = detail = storage = null;
  }
  const sameDestination = (left: RecordingsQuery, right: RecordingsQuery) => left.view === right.view
    && (left.cursor ?? '') === (right.cursor ?? '') && (left.recordingId ?? '') === (right.recordingId ?? '');
  /** One authority change erases every private fact — page, detail, storage and
   * both aggregates — and fences anything already in flight. */
  function clearFacts(denied: boolean) {
    if (closed) return;
    epoch++;summarySerial++;groupsSerial++;
    dropResident();summary = groups = null;groupsSort = 'recent';
    accessDenied = denied;errorText = '';
    publish();
  }
  function recordFailure(failure: unknown) {
    if (isCancelled(failure)) return;
    if (isAccessError(failure)) { clearFacts(true); return; }
    errorText = dvrMessage(failure);
    publish();
  }
  /** Retires this captured viewer once. A silent retirement is for a snapshot read
   * inside a render; it still erases and unsubscribes. */
  function retire(silent = false) {
    if (closed) return;
    closed = true;life.abort();
    signal.removeEventListener('abort', onAbort);stopModel?.();stopModel = undefined;
    loadSerial++;epoch++;summarySerial++;groupsSerial++;
    model.dispose();
    page = detail = storage = null;summary = groups = null;groupsSort = 'recent';
    loading = false;accessDenied = false;errorText = '';
    state = Object.freeze({query, phase: 'retired', page: null, detail: null, storage: null, summary: null, groups: null,
      groupsSort: 'recent', accessDenied: false, error: ''});
    try { if (!silent) notify(); } finally { listeners.clear(); }
  }
  function onAbort() { retire(); }
  function dispose() { retire(); }
  function start() {
    if (started || closed) return;
    started = true;
    if (!current()) return;
    signal.addEventListener('abort', onAbort, {once: true});
    stopModel = model.subscribe(sync);
  }
  /** A borrowed access change (401/403) arrives through the model's own client. */
  function sync() {
    if (closed) return;
    if (signal.aborted || !isCurrent()) { retire(); return; }
    if (model.getSnapshot().accessDenied && !accessDenied) { clearFacts(true); return; }
    publish();
  }
  function checked(value: RecordingsQuery): RecordingsQuery {
    if (!value || typeof value !== 'object') throw invalid();
    const view = value.view;
    if (!recordingsViews.includes(view)) throw invalid();
    const cursor = value.cursor ?? '', recordingId = value.recordingId ?? '';
    if (typeof cursor !== 'string' || cursor.length > 4096) throw invalid();
    if (typeof recordingId !== 'string' || recordingId !== '' && !recordingText.test(recordingId)) throw invalid();
    return Object.freeze({view, ...(cursor ? {cursor} : {}), ...(recordingId ? {recordingId} : {})});
  }
  function checkedGroups(sort: RecordingsSort, cursor: string) {
    if (!sorts.includes(sort) || typeof cursor !== 'string' || cursor.length > 4096) throw invalid();
  }
  return {
    /** Only the exact resident objects of a settled destination generation pass.
     * A replacement, a refresh in flight or a failed destination refuses them. */
    contains(recording: Recording): boolean {
      if (!current()) return false;
      if (derive().phase !== 'ready') return false;
      return (page?.recordings.includes(recording) ?? false) || detail?.recording === recording;
    },
    getSnapshot(): RecordingsSnapshot {
      if (!closed && (signal.aborted || !isCurrent())) retire(true);
      return state;
    },
    subscribe(listener: () => void) {
      if (closed) return () => {};
      listeners.add(listener);
      return () => { listeners.delete(listener); };
    },
    /** Reads one exact destination. A cursor is passed through untouched, and its
     * reply's own `nextCursor` is the only next token. */
    async load(next: RecordingsQuery = query): Promise<void> {
      if (!current()) throw cancelled();
      const captured = checked(next);
      start();
      const serial = ++loadSerial;
      if (!sameDestination(captured, query)) {
        // A replacement is a new destination generation: the previous page,
        // detail and storage objects stop being resident before the new query is
        // published, and a retained error page can never revive them. Profile-wide
        // summary/group facts and the receipt lease are deliberately untouched.
        dropResident();
      }
      query = captured;loading = true;errorText = '';
      publish();
      try {
        await model.load(captured);
        if (!current()) throw cancelled();
        if (serial !== loadSerial) throw cancelled();
        const snapshot = model.getSnapshot();
        page = snapshot.page;detail = snapshot.detail;storage = snapshot.storage;
        accessDenied = snapshot.accessDenied;errorText = snapshot.error ?? '';
        if (accessDenied) {
          summary = null;groups = null;groupsSort = 'recent';
          epoch++;summarySerial++;groupsSerial++;
        }
        loading = false;
        publish();
      } catch (failure) {
        if (!current()) throw cancelled();
        if (serial === loadSerial) {
          loading = false;
          if (isAccessError(failure)) clearFacts(true);
          else if (!isCancelled(failure)) { errorText = dvrMessage(failure);publish(); }
        }
        throw failure;
      }
    },
    /** The server's own totals. A loaded page is never counted instead. */
    async readSummary(): Promise<DVRSummary> {
      if (!current()) throw cancelled();
      start();
      const at = epoch, serial = ++summarySerial;
      try {
        const value = await model.client.summary();
        if (!current()) throw cancelled();
        if (at !== epoch || serial !== summarySerial) throw cancelled();
        summary = value;accessDenied = false;errorText = '';
        publish();
        return value;
      } catch (failure) {
        if (!current()) throw cancelled();
        if (at === epoch && serial === summarySerial) recordFailure(failure);
        throw failure;
      }
    },
    /** The server's genuine ordered groups, with its exact counts and next cursor. */
    async readGroups(sort: RecordingsSort = 'recent', cursor = ''): Promise<RecordingGroups> {
      if (!current()) throw cancelled();
      checkedGroups(sort, cursor);
      start();
      const at = epoch, serial = ++groupsSerial, restated = sort !== groupsSort;
      groupsSort = sort;
      if (restated) groups = null;
      publish();
      try {
        const value = await model.client.groups(sort, cursor);
        if (!current()) throw cancelled();
        if (at !== epoch || serial !== groupsSerial || groupsSort !== sort) throw cancelled();
        groups = value;accessDenied = false;errorText = '';
        publish();
        return value;
      } catch (failure) {
        if (!current()) throw cancelled();
        if (at === epoch && serial === groupsSerial) { if (restated) groups = null;recordFailure(failure); }
        throw failure;
      }
    },
    /** The same genuine 48-hex DVR operation receipt later command integration
     * awaits; mints nothing itself and is fenced like every other read. */
    async requestReceipt(): Promise<string> {
      if (!current()) throw cancelled();
      start();
      const at = epoch;
      const value = await requestId();
      if (!current()) throw cancelled();
      if (at !== epoch) throw cancelled();
      if (typeof value !== 'string' || !receiptText.test(value)) throw invalid();
      return value;
    },
    dispose,
  };
}
export type RecordingsController = ReturnType<typeof createRecordingsController>;
