import React, {useMemo, useSyncExternalStore} from 'react';
import type {ContentEntry} from '@core/library-content.ts';
import type {GroupBrowse} from '../../app/group-browse';
import {listWindowKey} from '../../app/list-position';
import {useViewerScope} from '../../app/viewer-scope';
import {ErrorNotice, ErrorState} from '../../app/errors';
import {useI18n} from '../../app/i18n';
import {groupOrderSort} from '@core/presentation/browse-groups.ts';
import {BrowseBar} from '../library/BrowseFilters';
import {FacetGroupContext} from '../../app/browse';
import {facetGroupOf} from '@core/browse.ts';
import {EngineResults} from '../library/EngineResults';
import {Inset, StateView, WindowedGrid} from '../../ui';

/**
 * The library's toolbar on a group page (a collection, an artist's albums, an album's songs, a
 * season, a person's titles, a playlist): the same Sort, filters and chips, built from what the
 * server offers for the pivot. Where the page has an order of its own, that is the first choice
 * in Sort and the one it opens on. Absent until the library's vocabulary has been read; the page's
 * own list never waits for it.
 */
export function GroupBar({group, total}: {group: GroupBrowse; /** The page's own count, shown while its own list stands. */ total?: number}) {
  const {t} = useI18n();
  const {spec} = group;
  // What the group already is (Favorites, this collection) is not offered again as a filter on it.
  const capabilities = useMemo(() => {
    const full = group.capabilities;
    if (!full || !spec) return undefined;
    const fixed = new Set(spec.base.flatMap(node => ('field' in node ? [node.field] : [])));
    if (!fixed.size) return full;
    return {...full, fields: full.fields.filter(field => !fixed.has(field.id)), quickFilters: full.quickFilters.filter(quick => !('field' in quick.query && fixed.has(quick.query.field)))};
  }, [group.capabilities, spec]);
  // A list across libraries counts its filter choices over its own titles.
  const facetGroup = useMemo(() => (spec ? facetGroupOf(spec.base) : undefined), [spec]);
  if (!spec || !capabilities) return null;
  // The page's own order cannot be kept through a filter (the server orders what it filters), so it is offered only while nothing is filtered.
  const ownOrder = spec.own && !spec.always && (!group.filtered || group.opening.field === groupOrderSort) ? {label: t('web.group.ownOrder', {group: spec.own}), selected: group.ownSelected, onSelect: group.ownOrder} : undefined;
  return <FacetGroupContext.Provider value={facetGroup}><BrowseBar libraryId={spec.libraryId} libraryKind={spec.libraryKind} pivot={spec.pivot} capabilities={capabilities} predicates={group.predicates} onChange={group.setPredicates} sort={group.sort} onSort={group.setSort} defaultSort={group.opening} ownOrder={ownOrder} total={group.total ?? total} totalIsLowerBound={group.total !== undefined && group.totalIsLowerBound} /></FacetGroupContext.Provider>;
}

/** A group's sorted or filtered titles as cards: the library's own windowed grid, kept to the group. */
export function GroupGrid({group, square}: {group: GroupBrowse; square: boolean}) {
  const {t} = useI18n();
  const scope = useViewerScope();
  const {spec, collection} = group;
  if (!spec || !collection) return null;
  return (
    <EngineResults
      collection={collection}
      onRefresh={group.refresh}
      libraryId={spec.libraryId}
      libraryKind={spec.libraryKind}
      list={false}
      sort={group.sort}
      sorts={group.capabilities?.sorts ?? []}
      onSort={group.setSort}
      sortField={group.sort.field}
      square={square}
      filtered={group.filtered}
      onClearFilters={group.clearFilters}
      pivot={spec.pivot}
      listKey={listWindowKey({serverId: scope.serverId, viewerId: scope.viewerId, libraryId: spec.libraryId, pivot: spec.pivot, query: JSON.stringify([spec.base, group.predicates, group.sort])})}
      empty={<StateView inline icon="filter" title={t('web.entity.emptyTitle')} body={t('web.entity.emptyBody')} />}
    />
  );
}

/**
 * A group's sorted or filtered members as the page's own rows (track rows, episode rows): the
 * same row the page draws in its own order, over the browse engine's windowed list.
 */
export function GroupRows({group, label, estimateRowHeight, columnMin, className, context, renderItem, renderPlaceholder, wrap}: {
  group: GroupBrowse;
  label: string;
  estimateRowHeight: number;
  columnMin?: string;
  className?: string;
  /** What the list is, for an error's wording. */
  context: 'library' | 'collection' | 'show' | 'playlist' | 'person' | 'saved';
  renderItem: (entry: ContentEntry, index: number) => React.ReactNode;
  renderPlaceholder: () => React.ReactNode;
  /** Wraps the list (not its empty or error state), for a page that frames its rows. */
  wrap?: (list: React.ReactNode) => React.ReactNode;
}) {
  const {t} = useI18n();
  const collection = group.collection;
  const snapshot = useSyncExternalStore(collection?.subscribe ?? noSubscribe, () => collection?.getSnapshot());
  if (!collection || !snapshot) return null;
  if (snapshot.status === 'error' && !snapshot.total) return <ErrorState error={snapshot.error} context={context} retry={() => collection.retry()} refresh={group.refresh} />;
  if (snapshot.total === 0) {
    return group.filtered
      ? <StateView inline icon="filter" title={t('library.noMatch')} body={t('library.noMatchBody')} action={{label: t('library.clearFilters'), onClick: group.clearFilters}} />
      : <StateView inline icon="filter" title={t('web.entity.emptyTitle')} body={t('web.entity.emptyBody')} />;
  }
  const list = <WindowedGrid collection={collection} estimateRowHeight={estimateRowHeight} columnMin={columnMin} className={className} label={label} renderItem={renderItem} renderPlaceholder={renderPlaceholder} />;
  return (
    <>
      {snapshot.status === 'error' ? <Inset><ErrorNotice error={snapshot.error} context={context} retry={() => collection.retry()} refresh={group.refresh} /></Inset> : null}
      {wrap ? wrap(list) : list}
    </>
  );
}

const noSubscribe = () => () => {};

