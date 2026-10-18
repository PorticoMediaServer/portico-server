import type {BrowseNode, BrowseSortSelection} from '../browse.ts';
import {libraryTabForPivot, type LibraryTabId} from './library-tabs.ts';

/** A saved view as the browse request it stands for (`savedViewBrowse`). */
export type PresetBrowse = Readonly<{libraryId: string; pivot: string; query?: BrowseNode; sort: readonly BrowseSortSelection[]}>;
type Leaf = Extract<BrowseNode, {field: string}>;

/**
 * How a library applies one of its presets (its saved views), on every client:
 *
 * - `controls`: it is exactly what the tab's own controls hold (filters all of which must match,
 *   one sort with no shuffle seed, every field one the tab's controls offer: `offered`), on the
 *   tab that browses its pivot with no conditions of its own. The tab shows it with its ordinary
 *   filters and sort, so it can be refined from there.
 * - `exact`: anything richer (any/not, more than one sort, a shuffle's seed) runs exactly as stored
 *   (a client that can hold a whole query on the tab does; another opens the saved view itself).
 *   `tab` is the tab that browses its pivot, when there is one.
 *
 * Nothing is flattened or dropped, and no tab's own conditions are added to a preset's query.
 * Applying a preset is not remembering a choice: the library's remembered tab, sort and filters
 * (on this device) stay as they were.
 */
export function presetApplication(kind: string, browse: PresetBrowse, offered?: ReadonlySet<string>): Readonly<{kind: 'controls'; tab: LibraryTabId; predicates: readonly Leaf[]; sort: BrowseSortSelection}> | Readonly<{kind: 'exact'; tab?: LibraryTabId}> {
  const tab = libraryTabForPivot(kind, browse.pivot)?.id;
  const leaves = flatLeaves(browse.query);
  const [sort] = browse.sort;
  if (tab && leaves && (!offered || leaves.every(leaf => offered.has(leaf.field))) && browse.sort.length === 1 && sort && sort.seed === undefined) return {kind: 'controls', tab, predicates: leaves, sort};
  return tab ? {kind: 'exact', tab} : {kind: 'exact'};
}

/** The filters a query is, when it is only filters that must all match (or none); undefined otherwise. */
function flatLeaves(query: BrowseNode | undefined): readonly Leaf[] | undefined {
  if (!query) return [];
  if ('field' in query) return [query];
  if ('all' in query && query.all.every(node => 'field' in node)) return query.all as readonly Leaf[];
  return undefined;
}
