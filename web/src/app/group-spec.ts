import type {BrowseNode, BrowseSortSelection} from '@core/browse.ts';
import {acrossLibraries} from '@core/browse.ts';
import {acrossCondition, acrossOwnOrder, acrossPivot, groupSurfaces, type AcrossGroup} from '@core/presentation/browse-groups.ts';

/**
 * A group of titles the library's sort and filter controls apply to: a library, the pivot that
 * holds the group's members, and the condition that keeps the list to them. Pure: which groups a
 * page has is decided here, apart from the screens that draw them.
 */
export type GroupOrder = 'playlist' | 'collection' | 'season' | 'row' | 'history' | 'parts' | 'default';
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
const SURFACES = groupSurfaces;

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

/**
 * A group that spans the viewer's libraries (a person's titles, a playlist, a personal collection, a
 * Home row, My List, Favorites, the history), sorted and filtered as one list across them. `own`
 * names the order the page shows until the viewer picks another; with no order of its own on the
 * server the page's own list stands until a sort or filter is chosen.
 */
export function acrossGroup(group: AcrossGroup, value: string | undefined, own: GroupOrder = 'default'): GroupSpec {
  return {libraryId: acrossLibraries, libraryKind: '', pivot: acrossPivot(group), base: [acrossCondition(group, value)], own: acrossOwnOrder(group) || own !== 'default' ? own : 'default'};
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
 * condition that keeps the list to them.
 */
export function entityGroups(libraryId: string, libraryKind: string, entity: Readonly<{kind: string; id: string}> | undefined): Readonly<{titles?: GroupSpec; songs?: GroupSpec}> {
  if (!entity || !libraryId) return {};
  const of = (pivot: string | undefined, base: BrowseNode, rest: Partial<GroupSpec> = {}): GroupSpec | undefined => (pivot ? {libraryId, libraryKind, pivot, base: [base], ...rest} : undefined);
  switch (entity.kind) {
    case 'collection': return {titles: of(titlePivot(libraryKind), {field: 'collection', operator: 'equals', value: entity.id}, {own: 'collection'})};
    case 'artist': return libraryKind === 'music' ? {titles: of('albums', member(entity.id), {own: 'default'}), songs: of('songs', member(entity.id), {own: 'default'})} : {};
    case 'album': return libraryKind === 'music' ? {songs: of('songs', member(entity.id), {opening: {field: 'trackNumber', direction: 'asc'}})} : {};
    case 'author': return libraryKind === 'audiobook' ? {titles: of('books', member(entity.id), {own: 'default'})} : {};
    // A book's parts, read as the library's parts pivot within the book.
    case 'book': return libraryKind === 'audiobook' ? {titles: of('parts', member(entity.id), {own: 'parts'})} : {};
    default: return {};
  }
}
