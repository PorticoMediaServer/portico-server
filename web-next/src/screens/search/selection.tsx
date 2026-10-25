import type {SearchService, SearchGroupResult} from '@core/search.ts';
import type {BrowseSelectionScope} from '@core/selection-model/index.ts';
import type {WebShellPlaybackRuntime} from '../../platform/playback/runtime';
import type {ScreenContext} from '../detail/context';
import {createCatalogueDocumentSelection, type CatalogueDocumentSource} from '../browse/document-selection';
const searchableSelectionKinds = new Set(['movie', 'show', 'season', 'episode', 'album', 'song']);

/** Search is explicit; its own fence/revision never becomes Browse query authority. */
export function createSearchSelection(context: ScreenContext, service: SearchService, principal: Omit<BrowseSelectionScope, 'viewId'>,
  metadataFence: () => string | undefined, runtime?: WebShellPlaybackRuntime) {
  const source: CatalogueDocumentSource = {subscribe: service.subscribe, getSnapshot: () => {
    const snapshot = service.getSnapshot(), data = snapshot.data, query = snapshot.query;
    if (!context.isCurrent() || !data || !query || !['ready', 'loading'].includes(snapshot.phase)
      || query.q !== data.query.q || (query.group ?? '') !== data.query.group) return null;
    return {key: JSON.stringify([data.scope.serverId, data.scope.viewerFence, data.revision, query.q, query.group, query.groups, query.libraryIds, query.sort, query.direction]),
      revision: JSON.stringify([data.scope.viewerFence, data.revision]), entries: snapshot.groups.flatMap(group => group.items.flatMap(entry => 'libraryId' in entry && searchableSelectionKinds.has(entry.kind) ? [entry] : []))};
  }};
  const selection = createCatalogueDocumentSelection(context, source, principal, metadataFence, runtime);
  const members = (group: SearchGroupResult) => {
    if (!service.getSnapshot().groups.includes(group) || !group.totalCountExact || group.hasMore || group.totalCount !== group.items.length) return;
    const entries = group.items.flatMap(entry => 'libraryId' in entry && searchableSelectionKinds.has(entry.kind) ? [entry] : []);
    return entries.length === group.items.length && selection.canSelectEntries(entries) ? entries : undefined;
  };
  return Object.assign(selection, {
    canSelectGroup: (group: SearchGroupResult) => !!members(group),
    selectGroup: (group: SearchGroupResult) => {const entries = members(group);if (!entries) throw new DOMException('Search group changed', 'AbortError');selection.selectEntries(entries);},
  });
}
export type SearchSelection = ReturnType<typeof createSearchSelection>;
