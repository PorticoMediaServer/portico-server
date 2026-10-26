import type {GetItemsIdResponse} from '../../../../packages/contracts/src/types.ts';

/** The one related catalogue entity a Cast display may read for a cover when the
 * playing item's own projection has none. A song points at its album; an
 * audiobook file (`audiobook_file`) points at its book container (`book`), the
 * same expected kind the receiver's `relatedCover` uses. */
export type CastCoverTarget = Readonly<{id: string; kind: 'album' | 'book'}>;

/** A bounded catalogue id, with at least one alphanumeric so a dot-only id cannot
 * normalise the item route. The caller still encodes it. */
const COVER_ID = /^(?=[A-Za-z0-9._:-]*[A-Za-z0-9])[A-Za-z0-9._:-]{1,128}$/;
/** The receiver's own single-slash catalogue-path rule: same origin, no scheme,
 * no whitespace and no backslash. */
const CATALOGUE_PATH = /^\/(?!\/)[^\s\\]{1,500}$/;

function coverTarget(id: unknown, kind: CastCoverTarget['kind']): CastCoverTarget | undefined {
  return typeof id === 'string' && COVER_ID.test(id) ? Object.freeze({id, kind}) : undefined;
}

/**
 * The bounded one-hop cover target for an item with no poster and no backdrop of
 * its own. Presence of the projected relation is the fact; no title, filename,
 * current item or other item is guessed. Returns undefined when the item already
 * has display art or has no exactly typed related id.
 */
export function castCoverTarget(item: GetItemsIdResponse): CastCoverTarget | undefined {
  if (item.posterUrl || item.backdropUrl) return undefined;
  const album = coverTarget(item.song?.albumId, 'album');
  if (album) return album;
  return coverTarget(item.bookFile?.bookId, 'book');
}

/**
 * The related item's own poster, selected only when the caller's read is the
 * exact expected catalogue entity and the path is a safe same-origin catalogue
 * path. Never falls back to a backdrop, another id or a guessed URL. The caller
 * owns the read, its currentness and the guard around caching/publication; this
 * helper performs no IO and holds no state.
 */
export function castCoverPath(expected: CastCoverTarget | undefined, related: GetItemsIdResponse | undefined): string | undefined {
  if (!expected || !related || related.id !== expected.id || related.missing === true || related.kind !== expected.kind) return undefined;
  const poster = related.posterUrl;
  return typeof poster === 'string' && CATALOGUE_PATH.test(poster) ? poster : undefined;
}
