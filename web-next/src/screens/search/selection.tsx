import type {SearchService} from '@core/search.ts';
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
  return createCatalogueDocumentSelection(context, source, principal, metadataFence, runtime);
}
export type SearchSelection = ReturnType<typeof createSearchSelection>;
