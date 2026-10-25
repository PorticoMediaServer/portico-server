import type {DetailProjection} from '@core/detail.ts';
import type {ArtworkSource} from '../../kit/artwork/types';
import type {WatchCollectionNext} from './collection-next';

/** desktop-player f4b: the end of a film. The retained title read supplies the viewer's own
 * rating permission, the rating itself and the server's related row. Nothing is inferred. */
export type WatchEndCard = Readonly<{
  title: string;
  related: readonly Readonly<{id: string; title: string; year?: number; poster?: ArtworkSource}>[];
  rating?: Readonly<{value: number | null; set(value: number | null): Promise<void>}>;
  /** The next member of the collection in its own order, when the server has one. */
  collection?: WatchCollectionNext;
  back: () => void | Promise<void>;
  openRelated: (id: string) => void | Promise<void>;
}>;

type RelatedEntry = Readonly<{id: string; title: string; year?: number; posterUrl?: string}>;

export function watchEndCard(data: DetailProjection | undefined, poster: (entry: RelatedEntry) => ArtworkSource | undefined,
  navigate: (itemId: string) => void | Promise<void>, setRating: (value: number | null) => Promise<void>,
  collection?: WatchCollectionNext): WatchEndCard | undefined {
  if (!data || data.item.kind !== 'movie') return undefined;
  const seen = new Set<string>(), related: {id: string; title: string; year?: number; poster?: ArtworkSource}[] = [];
  for (const entry of data.related?.rows.flatMap(row => row.entries) ?? []) {
    if (seen.has(entry.id)) continue;
    seen.add(entry.id);
    const source = poster(entry);
    related.push({id: entry.id, title: entry.title, ...(entry.year === undefined ? {} : {year: entry.year}), ...(source ? {poster: source} : {})});
    if (related.length === 12) break;
  }
  const canRate = data.actions.some(action => action.enabled && action.id === 'rating');
  return Object.freeze({
    title: data.item.title,
    related: Object.freeze(related.map(entry => Object.freeze(entry))),
    ...(canRate ? {rating: Object.freeze({value: data.personal.rating, set: setRating})} : {}),
    ...(collection ? {collection: Object.freeze({...collection})} : {}),
    back: () => navigate(data.item.id),
    openRelated: (id: string) => navigate(id),
  });
}
