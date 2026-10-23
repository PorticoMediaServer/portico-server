import {useEffect, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {BrowseFacetPage, BrowseCount} from '@core/browse.ts';
import {Chip} from '../../kit/controls';
import type {BrowseController} from './controller';

/** Counts and membership stay server-evaluated under the immutable Home row condition. */
export function BrowseScopeChips({controller, reportError}: Readonly<{controller: BrowseController; reportError: (error: unknown) => void}>) {
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const [scope, setScope] = useState<Readonly<{facets: BrowseFacetPage; all: BrowseCount}> | undefined>(undefined);
  const offered = controller.source.libraryId === '*' && state.capabilities?.fields.some(field => field.id === 'library' && field.facetSource);
  useEffect(() => {
    if (!offered) return;
    const stop = new AbortController(), predicates = state.predicates.filter(node => !('field' in node) || node.field !== 'library');
    const timer = setTimeout(() => {
      void Promise.all([controller.facets('library', '', predicates, stop.signal), controller.count(predicates, stop.signal)]).then(([facets, all]) => {
        if (!stop.signal.aborted) setScope({facets, all});
      }).catch(error => {if (!stop.signal.aborted) {setScope(undefined);reportError(error);}});
    }, 180);
    return () => {clearTimeout(timer);stop.abort();};
  }, [controller, offered, state.predicates, reportError]);
  if (!offered || !scope) return null;
  const selected = state.predicates.find(node => 'field' in node && node.field === 'library');
  const values = selected && 'field' in selected ? Array.isArray(selected.value) ? selected.value : [selected.value] : [];
  const count = (total: number, lower: boolean) => lower ? defaultI18n.t('next.browse.countAtLeast', {count: total}) : defaultI18n.number(total);
  const change = (value?: string) => {
    try {controller.setQuery([...state.predicates.filter(node => !('field' in node) || node.field !== 'library'), ...(value ? [controller.parseNode({field: 'library', operator: 'in', value: [value]})] : [])]);}
    catch (error) {reportError(error);}
  };
  return <><Chip variant="quick" selected={values.length === 0} count={count(scope.all.total, scope.all.totalIsLowerBound)} onClick={() => change()}>{defaultI18n.t('next.controls.all')}</Chip>
    {scope.facets.values.map(facet => <Chip key={facet.value} variant="quick" selected={values.includes(facet.value)} count={count(facet.count, facet.countIsLowerBound === true)} onClick={() => change(facet.value)}>{facet.label}</Chip>)}</>;
}
