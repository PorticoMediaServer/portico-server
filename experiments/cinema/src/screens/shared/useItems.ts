import {useCallback, useMemo} from 'react';
import {useQuery, useServices} from '../../data/hooks';
import type {Item} from '../../data/types';
import type {CardModel} from '../../design/Card';
import {toCard} from './cards';

/** Loads a set of items (plus the shows of any episodes) and returns lookup + card helpers. */
export function useItems(ids: string[] | undefined, key?: string) {
  const s = useServices();
  const list = ids ?? [];
  const k = key ?? list.join(',');
  const q = useQuery(ids ? 'items:' + k : null, async () => {
    const items = await s.catalog.items(list);
    const showIds = [...new Set(items.filter(i => i.kind === 'episode').map(i => (i as {showId: string}).showId))];
    const shows = showIds.length ? await s.catalog.items(showIds) : [];
    return [...items, ...shows];
  });
  const map = useMemo(() => new Map((q.data ?? []).map(i => [i.id, i])), [q.data]);
  const lookup = useCallback((id: string) => map.get(id), [map]);
  const cards = useCallback(
    (idsIn: string[], opts?: {shape?: CardModel['shape']; context?: 'row' | 'showDetail'}) =>
      idsIn.map(id => map.get(id)).filter((i): i is Item => !!i).map(i => toCard(i, s.user.state(i.id), {...opts, lookup})),
    [map, lookup, s],
  );
  return {map, lookup, cards, loading: q.loading, error: q.error, reload: q.reload};
}
