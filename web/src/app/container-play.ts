import type {QueueAnchor, Selector} from '@core/playback-v1/index.ts';

/**
 * Container plays on Playback v1 (Client Playback Migration): a show, a season, an album, an
 * artist, a disc, a collection, a playlist or a book plays or shuffles as one server request
 * (`playSelector`) that the server snapshots, however large, instead of the page of entries the
 * screen has loaded. Pure, so node tests cover the choice (test/container-play.test.ts); the
 * player calls `containerPlay` through `playSequence`.
 */

/** Containers the v1 selector names directly. */
export type ContainerKind = 'show' | 'season' | 'album' | 'artist' | 'disc' | 'collection' | 'playlist' | 'book';
const KINDS: ReadonlySet<string> = new Set<ContainerKind>(['show', 'season', 'album', 'artist', 'disc', 'collection', 'playlist', 'book']);

/** The container a sequence comes from; `shuffle` plays it in the server's shuffle. */
export type SequenceContainer = Readonly<{kind: ContainerKind; id: string; shuffle?: boolean}>;

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

/**
 * What the player sends for a play action: one `playSelector` with a container, else null (the
 * entry list). Play starts at the pressed entry (`anchor`); Shuffle lets the server choose the
 * first entry.
 */
export function containerPlay(container: SequenceContainer | undefined, firstItemId: string, startSeconds: number): {selector: Selector; options: {shuffle?: boolean; anchor?: QueueAnchor; startSeconds?: number}} | null {
  if (!container) return null;
  return {
    selector: {container: {kind: container.kind, id: container.id}},
    options: container.shuffle ? {shuffle: true} : {anchor: {itemId: firstItemId}, ...(startSeconds > 0 ? {startSeconds} : {})},
  };
}
