import type {MessageId} from '../../../i18n/src/index.ts';

/**
 * The tabs of a library, authored (Justin, 2 Oct 2026). Discover is first and the default on
 * every library; there is no Categories tab (genres, decades and studios are filters; Music's
 * Genres tab is genres only). Every client renders these in this order with these names: the
 * server's pivots are vocabulary, not layout.
 */
export type LibraryTabId = 'discover' | 'browse' | 'aired' | 'collections' | 'releases' | 'songs' | 'genres' | 'playlists' | 'authors' | 'series' | 'unmatched';

export type LibraryTabSort = Readonly<{field: string; direction: 'asc' | 'desc'}>;
export type LibraryTabPredicate = Readonly<{field: string; operator: string; value?: unknown}>;

export type LibraryTab = Readonly<{
  id: LibraryTabId;
  label: MessageId;
  /** The browse pivot the tab reads; Discover and Playlists have none. */
  pivot?: string;
  /** How the tab is drawn: Discover's rows, a browsable grid (or the list table), the songs table, genre cards, or the viewer's playlists. */
  shows: 'rows' | 'titles' | 'tracks' | 'genres' | 'playlists';
  /** The tab's own order, which the viewer does not change (Recently aired). No Sort control and no letter rail. */
  fixedSort?: LibraryTabSort;
  /** Conditions the tab always applies (Unmatched). */
  fixedFilter?: readonly LibraryTabPredicate[];
  /** An aggregate pivot: its entries are groups (genres, series) that open another tab filtered. */
  opens?: Readonly<{tab: LibraryTabId; field: string}>;
}>;

const discover: LibraryTab = {id: 'discover', label: 'library.tab.discover', shows: 'rows'};
const collections: LibraryTab = {id: 'collections', label: 'library.tab.collections', pivot: 'collections', shows: 'titles'};

const TABS: Readonly<Record<string, readonly LibraryTab[]>> = {
  movie: [discover, {id: 'browse', label: 'library.tab.movies', pivot: 'movies', shows: 'titles'}, collections],
  tv: [discover, {id: 'browse', label: 'library.tab.shows', pivot: 'shows', shows: 'titles'}, {id: 'aired', label: 'library.tab.aired', pivot: 'shows', shows: 'titles', fixedSort: {field: 'latestAired', direction: 'desc'}}, collections],
  music: [
    discover,
    {id: 'browse', label: 'library.tab.artists', pivot: 'artists', shows: 'titles'},
    {id: 'releases', label: 'library.tab.albums', pivot: 'albums', shows: 'titles'},
    {id: 'songs', label: 'library.tab.songs', pivot: 'songs', shows: 'tracks'},
    {id: 'genres', label: 'library.tab.genres', pivot: 'genres', shows: 'genres', opens: {tab: 'releases', field: 'genre'}},
    {id: 'playlists', label: 'library.tab.playlists', shows: 'playlists'},
  ],
  audiobook: [
    discover,
    {id: 'browse', label: 'library.tab.books', pivot: 'books', shows: 'titles'},
    {id: 'authors', label: 'library.tab.authors', pivot: 'authors', shows: 'titles'},
    {id: 'series', label: 'library.tab.series', pivot: 'series', shows: 'titles'},
  ],
};

/** Owner-only: albums made from files without tags (the scanner files them under "Unknown artist"), so the owner can find and fix them. */
const unmatched: LibraryTab = {id: 'unmatched', label: 'library.tab.unmatched', pivot: 'albums', shows: 'titles', fixedFilter: [{field: 'match', operator: 'equals', value: 'unmatched'}]};

/** The tabs of a library of this kind, in order. Anime has television's. */
export function libraryTabs(kind: string, options: Readonly<{owner?: boolean; /** The server offers the `match` field on albums. */ unmatched?: boolean}> = {}): readonly LibraryTab[] {
  const tabs = TABS[kind === 'anime' ? 'tv' : kind] ?? [discover];
  return kind === 'music' && options.owner && options.unmatched ? [...tabs, unmatched] : tabs;
}

/** The tab an address names, or Discover. */
export function libraryTab(kind: string, id: string | undefined, options?: Parameters<typeof libraryTabs>[1]): LibraryTab {
  const tabs = libraryTabs(kind, options);
  return tabs.find(tab => tab.id === id) ?? tabs[0]!;
}

/** The tab that browses a pivot in a library of this kind (a row's "See all" names a pivot). */
export function libraryTabForPivot(kind: string, pivot: string): LibraryTab | undefined {
  return libraryTabs(kind).find(tab => tab.pivot === pivot && !tab.fixedSort && !tab.fixedFilter);
}

/** An artist's releases by type, in the order the page shows them. A type with nothing is left out. */
export type ReleaseGroup<T> = Readonly<{id: 'albums' | 'singles' | 'compilations' | 'appearances'; label: MessageId; entries: readonly T[]}>;
export function artistReleaseGroups<T extends {role?: string}>(entries: readonly T[]): readonly ReleaseGroup<T>[] {
  const groups: ReleaseGroup<T>[] = [
    {id: 'albums', label: 'title.albums', entries: entries.filter(e => e.role === 'album' || e.role === undefined)},
    {id: 'singles', label: 'title.singlesAndEps', entries: entries.filter(e => e.role === 'single' || e.role === 'ep')},
    {id: 'compilations', label: 'title.compilations', entries: entries.filter(e => e.role === 'compilation')},
    {id: 'appearances', label: 'title.appearsOn', entries: entries.filter(e => e.role === 'appearance')},
  ];
  return groups.filter(g => g.entries.length);
}
