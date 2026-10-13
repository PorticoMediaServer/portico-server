import type {BrowseNode, BrowseSortSelection} from '@core/browse.ts';

/**
 * A group of titles the library's sort and filter controls apply to: a library, the pivot that
 * holds the group's members, and the condition that keeps the list to them. Pure: which groups a
 * page has is decided here, apart from the screens that draw them.
 */
export type GroupOrder = 'playlist' | 'collection' | 'season' | 'default';
export type GroupSpec = Readonly<{
  libraryId: string;
  libraryKind: string;
  pivot: string;
  base: readonly BrowseNode[];
  /** The page's own order, when no server sort says the same thing. Offered first in the Sort menu and chosen until the viewer picks another. */
  own?: GroupOrder;
  /** The server sort the page's own list is already in (an album's track numbers): the group opens on it. */
  opening?: BrowseSortSelection;
}>;

/** The pivot that lists a library's titles themselves: what a collection or a person's page is made of. */
export function titlePivot(libraryKind: string): string | undefined {
  switch (libraryKind) {
    case 'movie': return 'movies';
    case 'tv': case 'anime': return 'shows';
    case 'music': return 'albums';
    case 'audiobook': return 'books';
    default: return undefined;
  }
}

/** The pivot that lists what a library plays one at a time: what a playlist is made of. */
export function itemPivot(libraryKind: string): string | undefined {
  switch (libraryKind) {
    case 'movie': return 'movies';
    case 'tv': case 'anime': return 'episodes';
    case 'music': return 'songs';
    case 'audiobook': return 'books';
    default: return undefined;
  }
}

const member = (id: string): BrowseNode => ({field: 'memberOf', operator: 'equals', value: id});

/**
 * The groups an entity page lists, by the entity's kind: which pivot holds its members and the
 * condition that keeps the list to them. A kind with no pivot of its own (a book's parts, a disc)
 * has none and keeps the page's own list.
 */
export function entityGroups(libraryId: string, libraryKind: string, entity: Readonly<{kind: string; id: string}> | undefined): Readonly<{titles?: GroupSpec; songs?: GroupSpec}> {
  if (!entity || !libraryId) return {};
  const of = (pivot: string | undefined, base: BrowseNode, rest: Partial<GroupSpec> = {}): GroupSpec | undefined => (pivot ? {libraryId, libraryKind, pivot, base: [base], ...rest} : undefined);
  switch (entity.kind) {
    case 'collection': return {titles: of(titlePivot(libraryKind), {field: 'collection', operator: 'equals', value: entity.id}, {own: 'collection'})};
    case 'artist': return libraryKind === 'music' ? {titles: of('albums', member(entity.id), {own: 'default'}), songs: of('songs', member(entity.id), {own: 'default'})} : {};
    case 'album': return libraryKind === 'music' ? {songs: of('songs', member(entity.id), {opening: {field: 'trackNumber', direction: 'asc'}})} : {};
    case 'author': return libraryKind === 'audiobook' ? {titles: of('books', member(entity.id), {own: 'default'})} : {};
    default: return {};
  }
}
