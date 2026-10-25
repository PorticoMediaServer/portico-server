import type {BrowseSelectionScope} from '@core/selection-model/index.ts';
import type {WebShellPlaybackRuntime} from '../../platform/playback/runtime';
import {createCatalogueDocumentSelection, type CatalogueDocumentSource} from '../browse/document-selection';
import type {ScreenContext} from './context';
import type {ShowBindings} from './show-bindings';
/** The workspace page owns this episode set. Changing season, filter, ordering,
 * page, viewer or revision retires it rather than selecting an unseen sibling. */
export function createEpisodeSelection(context: ScreenContext, binding: ShowBindings,
  principal: Omit<BrowseSelectionScope, 'viewId'>, metadataFence: () => string | undefined, runtime?: WebShellPlaybackRuntime) {
  const source: CatalogueDocumentSource = {
    subscribe: listener => {const off = binding.service.subscribe(listener), offView = binding.view.subscribe(listener); return () => {off(); offView();};},
    getSnapshot: () => {
      const state = binding.service.getSnapshot(), data = state.data;
      if (!context.isCurrent() || state.phase !== 'ready' || !data || binding.view.getSnapshot().tab !== 'episodes') return null;
      return {key: JSON.stringify([data.scope, data.revision, state.route, data.selected.seasonId, data.selected.group, state.pagination.cursor]),
        revision: JSON.stringify([data.scope.viewerFence, data.revision]),
        entries: data.episodes.sections.flatMap(section => section.entries.filter(entry => entry.kind === 'episode' && entry.libraryId === data.scope.libraryId))};
    },
  };
  return createCatalogueDocumentSelection(context, source, principal, metadataFence, runtime);
}
