import {filterValuesField, presetApplication} from '@core/presentation/index.ts';
import {savedViewBrowse, type PersonalResource} from '@core/personal-saved.ts';
import type {BrowseCapabilities} from '@core/browse.ts';
import {encodePredicates, type Predicate} from '../../app/browse';

/**
 * Where applying a library's preset goes on the web. A view that is only filters that must all
 * match and one sort the tab offers opens as the tab's own controls, its filters in the address;
 * anything richer opens as the saved view itself, which runs its whole query. The address is
 * bounded (`encodePredicates`), so filters too long for it open the saved view too: never the
 * library with no filter in place of the view's. None for a view that needs review.
 */
export type PresetDestination =
  | Readonly<{to: 'library'; search: Readonly<{view: string; sort: string; direction: 'asc' | 'desc'; filters?: string}>}>
  | Readonly<{to: 'saved'; resourceId: string; title: string}>;

export function presetDestination(kind: string, row: PersonalResource, capabilities: Pick<BrowseCapabilities, 'fields'> | undefined): PresetDestination | undefined {
  const stored = savedViewBrowse(row);
  if (!stored) return undefined;
  const offered = new Set((capabilities?.fields ?? []).flatMap(f => [f.id, filterValuesField(f).field]));
  const how = presetApplication(kind, stored, offered);
  if (how.kind === 'controls') {
    const filters = how.predicates.length ? encodePredicates(how.predicates as readonly Predicate[]) : undefined;
    if (!how.predicates.length || filters !== undefined) return {to: 'library', search: {view: how.tab, sort: how.sort.field, direction: how.sort.direction, ...(filters ? {filters} : {})}};
  }
  return {to: 'saved', resourceId: row.id, title: row.name};
}
