import type {QueueAnchor, QueueBrowseSelector, Selector} from '@core/playback-v1/index.ts';
import type {BrowseNode, BrowseSortSelection} from '@core/browse.ts';

/**
 * Container plays on Playback v1 (Client Playback Migration): a show, a season, an album, an
 * artist, a disc, a collection, a playlist or a book plays or shuffles as one server request
 * (`playSelector`) that the server snapshots, however large, instead of the page of entries the
 * screen has loaded. Pure, so node tests cover the choice (test/container-play.test.ts); the
 * player calls `containerPlay` through `playSequence`.
 *
 * A sorted or filtered list, and a saved view or personal collection, plays the same way: the
 * server's own query (`viewSequence`) or the saved resource (`savedSequence`), so Play, Shuffle
 * and play-from-here cover exactly what the list shows, however long, never the rows loaded.
 */

/** Containers the v1 selector names directly. */
export type ContainerKind = 'show' | 'season' | 'album' | 'artist' | 'disc' | 'collection' | 'playlist' | 'book';
const KINDS: ReadonlySet<string> = new Set<ContainerKind>(['show', 'season', 'album', 'artist', 'disc', 'collection', 'playlist', 'book']);

/**
 * The container a sequence comes from; `shuffle` plays it in the server's shuffle. In a playlist,
 * `entryId` is the pressed occurrence, so the copy pressed plays when a title is in it twice.
 * `query` is a library's browse (a sorted or filtered list); `saved` a saved view or personal
 * collection by its id.
 */
export type SequenceContainer =
  | Readonly<{kind: ContainerKind; id: string; shuffle?: boolean; entryId?: string}>
  | Readonly<{kind: 'query'; query: QueueBrowseSelector; shuffle?: boolean}>
  | Readonly<{kind: 'saved'; id: string; shuffle?: boolean}>;

/**
 * The container a screen passes with Play or Shuffle, or undefined (the loaded entries play).
 * A disc page names its catalog navigation id (`albumId:disc:N`); the v1 disc selector takes
 * `albumId:N`, so the id is mapped (an unmappable disc falls back to the entry list). An
 * episode group that isn't a season (an arc, a range, a year) would need the show with a range,
 * which the v1 selector can't express yet, so it falls back to the entry list. Popular tracks
 * (a top-5 list) always play their own five as an item list, so callers pass no container.
 */
export function sequenceContainer(kind: string | undefined, id: string | undefined, o: {shuffle?: boolean; group?: boolean} = {}): SequenceContainer | undefined {
  if (!kind || !id || !KINDS.has(kind) || o.group) return undefined;
  if (kind === 'disc') {
    const mapped = discSelectorId(id);
    if (!mapped) return undefined;
    return {kind: 'disc', id: mapped, ...(o.shuffle ? {shuffle: true} : {})};
  }
  return {kind: kind as ContainerKind, id, ...(o.shuffle ? {shuffle: true} : {})};
}

/**
 * A disc page id (`albumId:disc:N`, the catalog's navigation id) as the playbackv1 disc
 * container id (`albumId:N`), or undefined when the page id doesn't parse.
 */
export function discSelectorId(id: string): string | undefined {
  const at = id.indexOf(':disc:');
  if (at < 0) return undefined;
  const album = id.slice(0, at);
  const raw = id.slice(at + ':disc:'.length);
  if (!album || !/^\d{1,4}$/.test(raw)) return undefined;
  return `${album}:${Number(raw)}`;
}

/** A playlist played from one of its rows: that occurrence, not the title's first copy. */
export function occurrenceSequence(playlistId: string, entryId: string): SequenceContainer {
  return {kind: 'playlist', id: playlistId, entryId};
}

/** A saved view or personal collection, whole, in its own order (or the server's shuffle). */
export function savedSequence(id: string, o: {shuffle?: boolean} = {}): SequenceContainer {
  return {kind: 'saved', id, ...(o.shuffle ? {shuffle: true} : {})};
}

/**
 * The list on screen as the server's browse query: the library, the pivot, what the list is of
 * (`base`), the viewer's filters and the sort, exactly as the list itself is read. Undefined
 * until the list has a library and a pivot.
 */
export function viewSequence(view: Readonly<{libraryId?: string; pivot?: string; base?: readonly BrowseNode[]; predicates?: readonly BrowseNode[]; sort?: BrowseSortSelection}>, o: {shuffle?: boolean} = {}): SequenceContainer | undefined {
  if (!view.libraryId || !view.pivot) return undefined;
  const all = [...(view.base ?? []), ...(view.predicates ?? [])];
  // A Home row is what Home shows now, which a queue cannot name: its titles play from the list on screen.
  if (all.some(node => 'field' in node && node.field === 'homeRow')) return undefined;
  return {kind: 'query', query: {libraryId: view.libraryId, pivot: view.pivot, ...(all.length ? {query: {all}} : {}), ...(view.sort ? {sort: [view.sort]} : {})}, ...(o.shuffle ? {shuffle: true} : {})};
}

/** The selector a sequence names. */
export function containerSelector(container: SequenceContainer): Selector {
  if (container.kind === 'query') return {query: container.query};
  if (container.kind === 'saved') return {saved: {id: container.id}};
  return {container: {kind: container.kind, id: container.id}};
}

/**
 * What the player sends for a play action: one `playSelector` with a container, else null (the
 * entry list). Play starts at the pressed entry (`anchor`, the pressed playlist occurrence when
 * there is one); Shuffle lets the server choose the first entry.
 */
export function containerPlay(container: SequenceContainer | undefined, firstItemId: string, startSeconds: number): {selector: Selector; options: {shuffle?: boolean; anchor?: QueueAnchor; startSeconds?: number}} | null {
  if (!container) return null;
  const entryId = 'entryId' in container ? container.entryId : undefined;
  return {
    selector: containerSelector(container),
    options: container.shuffle ? {shuffle: true} : {anchor: entryId ? {entryId, itemId: firstItemId} : {itemId: firstItemId}, ...(startSeconds > 0 ? {startSeconds} : {})},
  };
}

/**
 * A group page's sorted or filtered list (`useGroupBrowse`) as the server's query, or undefined
 * while the page shows its own list (its own container plays then).
 */
export function groupSequence(group: Readonly<{active: boolean; spec?: Readonly<{libraryId: string; pivot: string; base: readonly BrowseNode[]}>; predicates: readonly BrowseNode[]; sort: BrowseSortSelection}>, o: {shuffle?: boolean} = {}): SequenceContainer | undefined {
  if (!group.active || !group.spec) return undefined;
  return viewSequence({libraryId: group.spec.libraryId, pivot: group.spec.pivot, base: group.spec.base, predicates: group.predicates, sort: group.sort}, o);
}
