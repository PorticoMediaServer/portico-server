import {useState} from 'react';
import type {BrowseCapabilities, BrowseSortSelection} from '@core/browse.ts';
import type {Predicate} from '../app/browse';
import {BrowseBar} from '../screens/library/BrowseFilters';
import {Inset, Page, PageHeader} from '../ui';

/** Development only: the library browse bar and filter panel against canned capabilities. */
const field = (id: string, type: string, controlHint: string, extra: Record<string, unknown> = {}) => ({id, labelKey: `browse.field.${id}`, type, operators: type === 'number' || type === 'date' ? ['between', 'at-least', 'at-most'] : type === 'boolean' ? ['equals'] : ['in', 'contains-any', 'equals'], controlHint, complexity: 'standard', cost: 'cheap', applicableKinds: ['movie'], ...extra});
const capabilities = {
  library: {id: 'lib', name: 'Movies', kind: 'movie', defaultView: 'grid', pinned: false},
  pivots: [], resolvedPivot: {id: 'titles', labelKey: 'x', entityKinds: ['movie'], defaultSort: [{field: 'title', direction: 'asc'}], supportedViews: ['grid'], browsable: true},
  fields: [
    field('studio', 'string', 'facet-multi-select'), field('year', 'number', 'number-range'), field('genre', 'string', 'facet-multi-select', {allowedValues: ['Action', 'Comedy', 'Drama', 'Horror', 'Romance', 'Thriller']}),
    field('playState', 'enum', 'select', {allowedValues: ['unwatched', 'in-progress', 'watched']}), field('resolution', 'enum', 'select', {allowedValues: ['sd', 'hd', 'uhd']}), field('contentRating', 'string', 'facet-multi-select', {allowedValues: ['G', 'PG', 'PG-13', 'R', 'NC-17']}),
    field('favorite', 'boolean', 'toggle'), field('watchlisted', 'boolean', 'toggle'), field('collection', 'string', 'facet-multi-select'), field('director', 'string', 'text'), field('actor', 'string', 'text'), field('releaseDate', 'date', 'date-range'), field('audioLanguage', 'string', 'facet-multi-select'), field('tag', 'string', 'text'),
  ],
  sorts: [{id: 'title', labelKey: 'sort.title', directions: ['asc', 'desc'], defaultDirection: 'asc', expensive: false, applicableKinds: []}, {id: 'added', labelKey: 'sort.added', directions: ['asc', 'desc'], defaultDirection: 'desc', expensive: false, applicableKinds: []}],
  quickFilters: [
    {id: 'unwatched', labelKey: 'filter.unwatched', query: {field: 'playState', operator: 'equals', value: 'unwatched'}},
    {id: 'inProgress', labelKey: 'filter.inProgress', query: {field: 'playState', operator: 'equals', value: 'in-progress'}},
    {id: 'recent', labelKey: 'filter.recentlyAdded', query: {field: 'added', operator: 'at-least', value: '-P30D'}},
    {id: 'favorites', labelKey: 'filter.favorites', query: {field: 'favorite', operator: 'equals', value: true}},
    {id: 'watchlist', labelKey: 'filter.watchlist', query: {field: 'watchlisted', operator: 'equals', value: true}},
    {id: 'hd', labelKey: 'filter.hd', query: {field: 'resolution', operator: 'equals', value: 'hd'}},
    {id: 'uhd', labelKey: 'filter.uhd', query: {field: 'resolution', operator: 'equals', value: 'uhd'}},
  ],
  queryLimits: {maximumDepth: 4, maximumClauses: 20, maximumBytes: 8192, maximumSorts: 2, defaultLimit: 60, maximumLimit: 200, cursorTtlSeconds: 600},
} as unknown as BrowseCapabilities;

export function BrowseLab() {
  const [predicates, setPredicates] = useState<readonly Predicate[]>([]);
  const [sort, setSort] = useState<BrowseSortSelection>({field: 'title', direction: 'asc'});
  return (
    <Page>
      <PageHeader title="Browse bar" />
      <Inset>
        <BrowseBar libraryId="" capabilities={capabilities} predicates={predicates} onChange={setPredicates} sort={sort} onSort={setSort} total={1234} onSave={() => {}} libraryKind="movie" pivot="titles" />
        <pre style={{marginTop: 24, fontSize: 12}}>{JSON.stringify(predicates)}</pre>
      </Inset>
    </Page>
  );
}
