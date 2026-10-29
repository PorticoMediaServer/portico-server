import {FileInfoWindowSession, fileInfoRows, type FileInfoRows, type FileInfoWindowFacts} from './file-info-windows.ts';
import {
  FILE_INFO_DEFAULT_LIMIT, fileInfoReadyQueryId, parseFileInfoDiscPage, parseFileInfoGroupsPage, parseFileInfoMembersPage, parseFileInfoPreparing, parseFileInfoTracksPage,
  type FileInfoDiscPage, type FileInfoFileRow, type FileInfoGroupRow, type FileInfoOpening, type FileInfoSurface, type FileInfoTrackRow,
} from './file-info-pages.ts';
import type {PreparationRequestPort, PreparationResponse, PreparationRuntime} from './read-preparation.ts';

/** Provisional paths until the frozen contract's generated route IDs land (P2-29). Every
 * FileInfo request is built here and nowhere else. */
export const fileInfoPaths = Object.freeze({
  groups: (itemId: string) => `/v1/items/${encodeURIComponent(itemId)}/groups`,
  members: (itemId: string) => `/v1/items/${encodeURIComponent(itemId)}/groups/members`,
  tracks: (itemId: string) => `/v1/items/${encodeURIComponent(itemId)}/tracks`,
  disc: (itemId: string) => `/v1/items/${encodeURIComponent(itemId)}/disc-titles`,
  preparation: (queryId: string) => `/v1/file-info-preparations/${encodeURIComponent(queryId)}`,
});

export type FileInfoLane<Row> = Readonly<{
  status: 'loading' | 'preparing' | 'ready' | 'failed';
  rows?: FileInfoRows<Row>;
  /** Total of the whole lane where the server publishes one. */
  total?: number;
  error?: unknown;
}>;
export type FileInfoTrackLane = 'audio' | 'subtitles';
export type FileInfoSnapshot = Readonly<{
  groups: FileInfoLane<FileInfoGroupRow>;
  surface?: FileInfoSurface;
  /** Keyed by `fileInfoTrackKey(occurrence, lane)`. Absent until a renderer asks for it. */
  tracks: ReadonlyMap<string, FileInfoLane<FileInfoTrackRow>>;
  /** Owner only, keyed by group id. */
  members: ReadonlyMap<string, FileInfoLane<FileInfoFileRow>>;
  disc?: FileInfoLane<FileInfoDiscPage['titles'][number]> & Readonly<{occurrence?: string; discKind?: FileInfoDiscPage['discKind']}>;
}>;
export const fileInfoTrackKey = (occurrence: string, lane: FileInfoTrackLane) => `${lane}:${occurrence}`;

export type FileInfoSessionOptions = Readonly<{
  api: PreparationRequestPort;
  opening: FileInfoOpening;
  errorFromResponse(response: PreparationResponse): Error;
  isCurrent(): boolean;
  signal?: AbortSignal;
  runtime?: Partial<PreparationRuntime>;
  limit?: number;
}>;

type Lane<Read, Page, Row> = {
  session: FileInfoWindowSession<Read, Page>;
  read: Read;
  rows(page: Page): readonly Row[];
  total?(page: Page): number | undefined;
};

/** One File info opening: the accepted Detail it was made from, its lanes and their resident
 * rows. Lanes are read only when a renderer asks, one window at a time, and a lane holds its
 * server lease only while it has more to read. Retire it with the panel. */
export class FileInfoSession {
  private readonly options: FileInfoSessionOptions;
  private readonly listeners = new Set<() => void>();
  private snapshot: FileInfoSnapshot = Object.freeze({groups: Object.freeze({status: 'loading'}), tracks: new Map(), members: new Map()});
  private readonly lanes = new Map<string, Lane<unknown, unknown, unknown>>();
  private readonly controller = new AbortController();
  private retired = false;

  constructor(options: FileInfoSessionOptions) {
    this.options = options;
    options.signal?.addEventListener('abort', () => this.retire(), {once: true});
  }
  subscribe = (listener: () => void): (() => void) => { this.listeners.add(listener); return () => this.listeners.delete(listener); };
  getSnapshot = (): FileInfoSnapshot => this.snapshot;
  private live = () => !this.retired && !this.controller.signal.aborted && this.options.isCurrent();
  private publish(next: Partial<FileInfoSnapshot>): void {
    if (!this.live()) return;
    this.snapshot = Object.freeze({...this.snapshot, ...next});
    for (const listener of [...this.listeners]) listener();
  }
  retire(): void {
    if (this.retired) return;
    this.retired = true;
    this.controller.abort();
    for (const lane of this.lanes.values()) lane.session.retire();
    this.lanes.clear();
    this.listeners.clear();
  }
  private limit(): number { return this.options.limit ?? FILE_INFO_DEFAULT_LIMIT; }
  private window<Read extends Readonly<{cursor?: string}>, Page extends Readonly<{window: FileInfoWindowFacts}>>(path: (read: Read, cursor?: string) => string, parse: (body: unknown, response: PreparationResponse) => Page, onPreparing: () => void) {
    return new FileInfoWindowSession<Read, Page>({
      api: this.options.api, path, poll: fileInfoPaths.preparation, cancel: fileInfoPaths.preparation,
      parse: (body, _read, response) => parse(body, response), window: page => page.window,
      parsePreparing: (body, response) => parseFileInfoPreparing(body, this.options.opening, response),
      readyQueryId: fileInfoReadyQueryId, errorFromResponse: this.options.errorFromResponse,
      isCurrent: this.live, signal: this.controller.signal, runtime: this.options.runtime, onPreparing,
    });
  }
  /** Drive one lane: open or continue it, keep its resident rows, and give up its lease when it
   * has nothing more to read. A failure keeps the rows already shown. */
  private async drive<Page extends Readonly<{window: FileInfoWindowFacts}>, Row>(
    lane: Lane<Readonly<{cursor?: string}>, Page, Row>, mode: 'open' | 'next' | 'restart',
    current: () => FileInfoLane<Row> | undefined, store: (value: FileInfoLane<Row>) => void, after?: (page: Page) => void,
  ): Promise<void> {
    const held = current();
    store(Object.freeze({...held, status: 'loading', error: undefined}));
    try {
      const result = mode === 'next' ? await lane.session.next() : await lane.session.open(lane.read);
      if (!this.live()) return;
      const rows = fileInfoRows(mode === 'next' && !result.reopened ? held?.rows : undefined, lane.rows(result.page), result.page.window, mode === 'next' && !result.reopened ? 'append' : 'replace');
      const total = lane.total?.(result.page);
      store(Object.freeze({status: 'ready', rows, ...(total !== undefined ? {total} : {})}));
      after?.(result.page);
      if (!result.page.window.next) lane.session.cancel();
    } catch (error) {
      if (!this.live() || (error as {name?: string})?.name === 'AbortError') return;
      store(Object.freeze({...current(), status: 'failed', error}));
    }
  }
  private lane<Page extends Readonly<{window: FileInfoWindowFacts}>, Row>(key: string, make: () => Lane<Readonly<{cursor?: string}>, Page, Row>): Lane<Readonly<{cursor?: string}>, Page, Row> {
    let lane = this.lanes.get(key) as Lane<Readonly<{cursor?: string}>, Page, Row> | undefined;
    if (!lane) { lane = make(); this.lanes.set(key, lane as Lane<unknown, unknown, unknown>); }
    return lane;
  }

  /** The versions. `more` reads the next window; `restart` reads the lane from its start. */
  groups(mode: 'open' | 'next' | 'restart' = 'open'): Promise<void> {
    const {opening} = this.options, item = opening.target.itemId;
    const lane = this.lane('groups', () => ({
      read: {},
      session: this.window((_read, cursor) => withQuery(fileInfoPaths.groups(item), {limit: this.limit(), cursor}),
        (body, response) => parseFileInfoGroupsPage(body, opening, response), () => this.publish({groups: Object.freeze({...this.snapshot.groups, status: 'preparing'})})),
      rows: page => page.groups,
    }));
    return this.drive(lane, mode, () => this.snapshot.groups, value => this.publish({groups: value}), page => this.publish({surface: page.surface}));
  }
  /** One version's raw audio or subtitle tracks, read from its head file. */
  tracks(occurrence: string, kind: FileInfoTrackLane, mode: 'open' | 'next' | 'restart' = 'open'): Promise<void> {
    const {opening} = this.options, item = opening.target.itemId, key = fileInfoTrackKey(occurrence, kind);
    const set = (value: FileInfoLane<FileInfoTrackRow>) => this.publish({tracks: new Map([...this.snapshot.tracks, [key, value]])});
    const lane = this.lane(key, () => ({
      read: {},
      session: this.window((_read, cursor) => withQuery(fileInfoPaths.tracks(item), {lane: kind, occurrence, limit: this.limit(), cursor}),
        (body, response) => parseFileInfoTracksPage(body, opening, kind, occurrence, response), () => set(Object.freeze({...this.snapshot.tracks.get(key), status: 'preparing'}))),
      rows: page => page.tracks, total: page => page.total,
    }));
    return this.drive(lane, mode, () => this.snapshot.tracks.get(key), set);
  }
  /** Owner only: the files of one version, with their paths. */
  members(groupId: string, mode: 'open' | 'next' | 'restart' = 'open'): Promise<void> {
    const {opening} = this.options, item = opening.target.itemId, key = `members:${groupId}`, owner = !!this.snapshot.surface?.manage;
    const set = (value: FileInfoLane<FileInfoFileRow>) => this.publish({members: new Map([...this.snapshot.members, [groupId, value]])});
    const lane = this.lane(key, () => ({
      read: {},
      session: this.window((_read, cursor) => withQuery(fileInfoPaths.members(item), {group: groupId, limit: this.limit(), cursor}),
        (body, response) => parseFileInfoMembersPage(body, opening, groupId, owner, response), () => set(Object.freeze({...this.snapshot.members.get(groupId), status: 'preparing'}))),
      rows: page => page.members, total: page => page.total,
    }));
    return this.drive(lane, mode, () => this.snapshot.members.get(groupId), set);
  }
  /** Owner only: the whole surface's disc titles. */
  disc(mode: 'open' | 'next' | 'restart' = 'open'): Promise<void> {
    const {opening} = this.options, item = opening.target.itemId;
    type Disc = NonNullable<FileInfoSnapshot['disc']>;
    const set = (value: Disc) => this.publish({disc: Object.freeze({...this.snapshot.disc, ...value})});
    const lane = this.lane('disc', () => ({
      read: {},
      session: this.window((_read, cursor) => withQuery(fileInfoPaths.disc(item), {limit: this.limit(), cursor}),
        (body, response) => parseFileInfoDiscPage(body, opening, response), () => set(Object.freeze({status: 'preparing'}) as Disc)),
      rows: page => page.titles, total: page => page.total,
    }));
    return this.drive(lane, mode, () => this.snapshot.disc, value => set(value as Disc), page => set(Object.freeze({...this.snapshot.disc!, occurrence: page.occurrence, discKind: page.discKind})));
  }
}

function withQuery(path: string, query: Readonly<Record<string, string | number | undefined>>): string {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) if (value !== undefined) search.set(key, String(value));
  const text = search.toString();
  return text ? `${path}?${text}` : path;
}
