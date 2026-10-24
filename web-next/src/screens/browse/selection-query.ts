import type {BrowseWholeSelection} from '@core/browse.ts';
import {selectionCapabilityNames} from '@core/selection-model/capabilities.ts';
import type {SelectionCapabilities, SelectionQuery, WholeSelectionSelector} from '@core/selection-model/index.ts';

/** The server's approved whole-view descriptor as the selection model's query
 * contract. Every fact is copied from the descriptor; nothing is inferred, and
 * `summary.every` is derived exactly as the model recomputes it (a capability is
 * offered for the whole view only when every displayed title has it). */
export function wholeViewQuery(whole: BrowseWholeSelection): SelectionQuery {
  const selector = {
    query: {
      libraryId: whole.libraryId,
      pivot: whole.pivot,
      ...(whole.filter ? {filter: whole.filter} : {}),
      ...(whole.sort.length ? {sort: whole.sort} : {}),
    },
  } as WholeSelectionSelector;
  const types = whole.types.map(type => ({
    kind: type.kind as SelectionQuery['summary']['types'][number]['kind'],
    count: type.count,
    capabilityCounts: type.capabilities,
    libraries: type.libraries,
  }));
  const every = Object.fromEntries(selectionCapabilityNames.map(name => [name,
    types.length > 0 && types.every(type => type.capabilityCounts[name] === type.count)])) as Record<keyof SelectionCapabilities, boolean>;
  return {
    // The revision is what makes two descriptors different views; the pivot keeps it readable.
    id: `whole:${whole.catalogRevision}:${whole.pivot}`,
    selector,
    total: whole.total,
    catalogRevision: whole.catalogRevision,
    summary: {
      types,
      library: types.length ? {kind: 'single' as const, libraryId: whole.libraryId} : {kind: 'none' as const},
      every,
    },
  };
}
