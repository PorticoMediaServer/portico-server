/**
 * What a person last chose in each library: the tab they were on, and the sort and filters they
 * used on each tab. A library reopens the way they left it. The memory belongs to one profile on
 * one device: it is kept in the device's own storage under that profile's key, never sent to the
 * server, and another profile on the same device has its own.
 *
 * Only choices the person makes on the library screen are remembered (picking a tab, a sort, a
 * filter). Arriving through a link that names a tab or a sort (a row's "See all", a shared
 * address) shows what the link says and leaves the memory alone.
 *
 * Everything read back from storage is treated as untrusted: it is bounded, and any part that
 * cannot be read is dropped while the rest is kept.
 */
export type RememberedSort = Readonly<{field: string; direction: 'asc' | 'desc'}>;
export type RememberedFilter = Readonly<{field: string; operator: string; value?: unknown}>;
/** One tab's last sort and filters. An empty filter list means the person cleared them. */
export type RememberedTab = Readonly<{sort?: RememberedSort; filters?: readonly RememberedFilter[]}>;
export type RememberedLibrary = Readonly<{tab?: string; tabs: Readonly<Record<string, RememberedTab>>}>;
export type LibraryMemoryScope = Readonly<{serverId: string; viewerId: string}>;

/**
 * The device's storage. Web storage answers at once; a native store may answer later, in which
 * case `LibraryMemory.ready` settles when the saved choices have been read.
 */
export interface LibraryMemoryStorage {
  read(key: string): string | null | undefined | Promise<string | null | undefined>;
  write(key: string, text: string): void | Promise<void>;
  remove(key: string): void | Promise<void>;
}

export const MAX_REMEMBERED_LIBRARIES = 64;
const MAX_TABS = 16;
const MAX_FILTERS = 16;
const MAX_FILTER_TEXT = 2048;
const MAX_STORED_TEXT = 262144;

const record = (v: unknown): v is Record<string, unknown> => typeof v === 'object' && v !== null && !Array.isArray(v);
const name = (v: unknown, max = 128): v is string => typeof v === 'string' && v.length > 0 && v.length <= max && !/[\x00-\x1f\x7f]/.test(v);

/** The storage key for one profile on one server. */
export function libraryMemoryKey(scope: LibraryMemoryScope): string {
  return 'portico.library-memory.v1:' + encodeURIComponent(scope.serverId) + ':' + encodeURIComponent(scope.viewerId);
}

function readSort(v: unknown): RememberedSort | undefined {
  return record(v) && name(v.field) && (v.direction === 'asc' || v.direction === 'desc') ? Object.freeze({field: v.field, direction: v.direction}) : undefined;
}

/** Filters as remembered: each one readable, the whole list bounded. Undefined when there is nothing usable to keep. */
function readFilters(v: unknown): readonly RememberedFilter[] | undefined {
  if (!Array.isArray(v) || v.length > MAX_FILTERS) return undefined;
  const filters: RememberedFilter[] = [];
  for (const row of v) {
    if (!record(row) || !name(row.field) || !name(row.operator, 64)) continue;
    filters.push(Object.freeze(row.value === undefined ? {field: row.field, operator: row.operator} : {field: row.field, operator: row.operator, value: row.value}));
  }
  let text: string;
  try { text = JSON.stringify(filters); } catch { return undefined; }
  return text.length <= MAX_FILTER_TEXT ? Object.freeze(filters) : undefined;
}

function readTab(v: unknown): RememberedTab | undefined {
  if (!record(v)) return undefined;
  const sort = readSort(v.sort);
  const filters = v.filters === undefined ? undefined : readFilters(v.filters);
  if (!sort && !filters) return undefined;
  return Object.freeze({...(sort ? {sort} : {}), ...(filters ? {filters} : {})});
}

function readLibrary(v: unknown): RememberedLibrary | undefined {
  if (!record(v)) return undefined;
  const tabs: Record<string, RememberedTab> = {};
  if (record(v.tabs)) {
    for (const [id, raw] of Object.entries(v.tabs).slice(0, MAX_TABS)) {
      const tab = name(id, 64) ? readTab(raw) : undefined;
      if (tab) tabs[id] = tab;
    }
  }
  const tab = name(v.tab, 64) ? v.tab : undefined;
  if (!tab && !Object.keys(tabs).length) return undefined;
  return Object.freeze({...(tab ? {tab} : {}), tabs: Object.freeze(tabs)});
}

/** Saved text as a map of libraries, most recently used last. Anything unreadable is left out. */
export function parseLibraryMemory(text: string | null | undefined): Map<string, RememberedLibrary> {
  const libraries = new Map<string, RememberedLibrary>();
  if (typeof text !== 'string' || !text || text.length > MAX_STORED_TEXT) return libraries;
  let raw: unknown;
  try { raw = JSON.parse(text); } catch { return libraries; }
  if (!record(raw) || raw.version !== 1 || !Array.isArray(raw.libraries)) return libraries;
  for (const row of raw.libraries.slice(-MAX_REMEMBERED_LIBRARIES)) {
    if (!record(row) || !name(row.id, 256)) continue;
    const library = readLibrary(row);
    if (library) libraries.set(row.id, library);
  }
  return libraries;
}

export function serializeLibraryMemory(libraries: ReadonlyMap<string, RememberedLibrary>): string {
  return JSON.stringify({version: 1, libraries: [...libraries].map(([id, library]) => ({id, ...library}))});
}

/** One profile's remembered library choices on this device. */
export class LibraryMemory {
  /** Settles once the saved choices have been read. Already settled for storage that answers at once. */
  readonly ready: Promise<void>;
  private libraries = new Map<string, RememberedLibrary>();
  private loaded = false;
  private disposed = false;
  private readonly key: string;
  private readonly storage: LibraryMemoryStorage;
  private readonly listeners = new Set<() => void>();

  constructor(storage: LibraryMemoryStorage, scope: LibraryMemoryScope) {
    this.storage = storage;
    this.key = libraryMemoryKey(scope);
    let pending: ReturnType<LibraryMemoryStorage['read']>;
    try { pending = storage.read(this.key); } catch { pending = undefined; }
    if (pending && typeof (pending as Promise<unknown>).then === 'function') {
      this.ready = (pending as Promise<string | null | undefined>).then(text => this.adopt(text), () => this.adopt(undefined));
    } else {
      this.adopt(pending as string | null | undefined);
      this.ready = Promise.resolve();
    }
  }

  /** True once the saved choices have been read; before that `recall` knows only what was chosen in this run. */
  get isReady(): boolean { return this.loaded; }

  /** What the person last chose in this library, or undefined when they have chosen nothing. */
  recall(libraryId: string): RememberedLibrary | undefined {
    return this.libraries.get(libraryId);
  }

  /** The person opened this tab. */
  rememberTab(libraryId: string, tab: string): void {
    if (!name(libraryId, 256) || !name(tab, 64)) return;
    const current = this.libraries.get(libraryId);
    if (current?.tab === tab) { this.touch(libraryId); return; }
    this.put(libraryId, {tab, tabs: current?.tabs ?? {}});
  }

  /** The person chose a sort on this tab; undefined puts the tab back on its own default. */
  rememberSort(libraryId: string, tab: string, sort: RememberedSort | undefined): void {
    const kept = sort === undefined ? undefined : readSort(sort);
    this.patchTab(libraryId, tab, current => ({...(kept ? {sort: kept} : {}), ...(current?.filters ? {filters: current.filters} : {})}));
  }

  /** The person changed this tab's filters; an empty list remembers that they cleared them. */
  rememberFilters(libraryId: string, tab: string, filters: readonly RememberedFilter[]): void {
    const kept = readFilters(filters);
    // Filters too large to keep are not half-remembered: the tab reopens unfiltered.
    this.patchTab(libraryId, tab, current => ({...(current?.sort ? {sort: current.sort} : {}), ...(kept && kept.length ? {filters: kept} : {})}));
  }

  /** Forgets one library's choices, or every library's. */
  forget(libraryId?: string): void {
    if (libraryId === undefined) this.libraries.clear(); else this.libraries.delete(libraryId);
    this.persist();
  }

  subscribe(listener: () => void): () => void {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }

  dispose(): void {
    this.disposed = true;
    this.listeners.clear();
  }

  private adopt(text: string | null | undefined): void {
    // Choices made before a slow store answered are newer than what it held: they win.
    const saved = parseLibraryMemory(text);
    for (const [id, library] of this.libraries) { saved.delete(id); saved.set(id, library); }
    this.libraries = saved;
    this.trim();
    this.loaded = true;
    this.announce();
  }

  private patchTab(libraryId: string, tab: string, change: (current: RememberedTab | undefined) => RememberedTab): void {
    if (!name(libraryId, 256) || !name(tab, 64)) return;
    const current = this.libraries.get(libraryId);
    const tabs: Record<string, RememberedTab> = {...(current?.tabs ?? {})};
    const next = change(tabs[tab]);
    if (next.sort || next.filters) tabs[tab] = Object.freeze(next); else delete tabs[tab];
    const ids = Object.keys(tabs);
    // A library has a handful of tabs; if a store ever holds more, the oldest are let go.
    for (const id of ids.slice(0, Math.max(0, ids.length - MAX_TABS))) delete tabs[id];
    this.put(libraryId, {...(current?.tab ? {tab: current.tab} : {}), tabs});
  }

  private put(libraryId: string, library: RememberedLibrary): void {
    this.libraries.delete(libraryId);
    if (library.tab || Object.keys(library.tabs).length) this.libraries.set(libraryId, Object.freeze({...library, tabs: Object.freeze({...library.tabs})}));
    this.trim();
    this.persist();
  }

  private touch(libraryId: string): void {
    const current = this.libraries.get(libraryId);
    if (!current || [...this.libraries.keys()].at(-1) === libraryId) return;
    this.libraries.delete(libraryId);
    this.libraries.set(libraryId, current);
    this.persist();
  }

  private trim(): void {
    while (this.libraries.size > MAX_REMEMBERED_LIBRARIES) this.libraries.delete(this.libraries.keys().next().value as string);
  }

  private persist(): void {
    this.announce();
    if (this.disposed) return;
    // Remembering is a convenience: storage that is full or switched off never breaks the screen.
    try {
      const done = this.libraries.size ? this.storage.write(this.key, serializeLibraryMemory(this.libraries)) : this.storage.remove(this.key);
      if (done && typeof (done as Promise<void>).then === 'function') (done as Promise<void>).then(undefined, () => {});
    } catch { /* kept for this run only */ }
  }

  private announce(): void {
    for (const listener of [...this.listeners]) { try { listener(); } catch { /* a listener never breaks remembering */ } }
  }
}

/** What an address names for a library: nothing at all (a plain visit), or any of a tab, a sort and filters. */
export type LibraryAddress = Readonly<{tab?: string; sort?: RememberedSort; filters?: readonly RememberedFilter[]; /** The address names something else that decides the page (a category, a row, a search). */ other?: boolean}>;
export type LibraryOpening = Readonly<{tab?: string; sort?: RememberedSort; filters?: readonly RememberedFilter[]; /** The opening came from memory, so the address should be brought in line with it. */ remembered: boolean}>;

/**
 * How a library opens. An address that names anything is followed exactly. A plain visit opens
 * the tab the person last used, with that tab's last sort and filters. `tabs` are the tabs the
 * library has for this person now: a remembered tab that no longer exists is ignored.
 */
export function libraryOpening(address: LibraryAddress, remembered: RememberedLibrary | undefined, tabs: readonly string[]): LibraryOpening {
  const named = address.tab !== undefined || address.sort !== undefined || address.filters !== undefined || !!address.other;
  if (named || !remembered) return {tab: address.tab, sort: address.sort, filters: address.filters, remembered: false};
  const tab = remembered.tab && tabs.includes(remembered.tab) ? remembered.tab : undefined;
  const choices = rememberedTab(remembered, tab ?? tabs[0]);
  if (!tab && !choices) return {remembered: false};
  return {tab, sort: choices?.sort, filters: choices?.filters, remembered: true};
}

/** The sort and filters last used on one tab (what choosing that tab restores). */
export function rememberedTab(remembered: RememberedLibrary | undefined, tab: string | undefined): RememberedTab | undefined {
  return remembered && tab ? remembered.tabs[tab] : undefined;
}
