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
  /** The page has no list of its own for this group (one library's part of a list that spans several): it is always the browse engine's. */
  always?: boolean;
}>;

/** The surface the server advertises for a group's kind of membership (`browse-capabilities` → `surfaces`). */
const SURFACES: Readonly<Record<string, string>> = {collection: 'collection-titles', memberOf: 'container-titles', person: 'person-titles', list: 'list-titles', search: 'search-results'};

/**
 * Whether a library's published vocabulary can express a group: every condition that keeps the
 * list to the group names a field and an operator the pivot offers, and, where the server lists
 * its surfaces, the surface for that kind of membership is among them. A group it cannot express
 * gets no toolbar (its page keeps its own list) rather than a request the server would refuse.
 */
export function groupOffered(spec: GroupSpec, capabilities: Readonly<{fields: readonly Readonly<{id: string; operators: readonly string[]}>[]; surfaces?: readonly Readonly<{id: string; membershipField?: string}>[]}>): boolean {
  for (const node of spec.base) {
    if (!('field' in node)) continue;
    if (!capabilities.fields.some(field => field.id === node.field && field.operators.includes(node.operator))) return false;
    const surface = SURFACES[node.field];
    if (surface && capabilities.surfaces?.length && !capabilities.surfaces.some(s => s.id === surface && s.membershipField === node.field)) return false;
  }
  return true;
}

/** One library's part of a list that spans libraries (a person's titles, a playlist, My List), with how many it holds. */
export type GroupLibrary = Readonly<{id: string; name: string; kind: string; pivot: string; total: number}>;

/**
 * Sort and filters are a library's: its vocabulary, its values. A list that spans libraries is
 * therefore sorted and filtered one library at a time.
 * - Held by one library only: that library's toolbar sits on the page's own list.
 * - Held by several: the viewer picks a library (`chosen`) and gets its toolbar over that library's
 *   part; with none picked the page's own list stands as it is, with no toolbar.
 */
export function spanningGroup(libraries: readonly GroupLibrary[], chosen: string | undefined, base: readonly BrowseNode[], own: GroupOrder): Readonly<{spec?: GroupSpec; /** Offer the libraries to pick from. */ choose: boolean; chosen?: string}> {
  if (libraries.length === 1) { const only = libraries[0]!; return {spec: {libraryId: only.id, libraryKind: only.kind, pivot: only.pivot, base, own}, choose: false}; }
  const picked = libraries.find(library => library.id === chosen);
  if (!picked) return {choose: libraries.length > 1};
  return {spec: {libraryId: picked.id, libraryKind: picked.kind, pivot: picked.pivot, base, always: true}, choose: true, chosen: picked.id};
}

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
