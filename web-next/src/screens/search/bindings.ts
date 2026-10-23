import {SearchService, readSearchHistory, clearSearchHistory, type SearchEntry, type SearchGroup, type SearchQuery} from '@core/search.ts';
import {screenLifetime, type ScreenContext} from '../detail/context';

/** One session is retained by the parent while Search is off screen; never browser storage. */
export function createSearchBindings(context: ScreenContext) {
  const life = screenLifetime(context);
  const service = new SearchService({api: life.api, scope: context.scope});
  let text = '', group: SearchGroup | undefined;
  let resultKey = '';
  const visible = new Set<SearchEntry>();
  const off = service.subscribe(() => {
    const state = service.getSnapshot(), data = state.data;
    if (!data) { visible.clear(); resultKey = ''; return; }
    const key = JSON.stringify([data.query.q, data.query.group, data.scope.viewerFence, data.revision]);
    if (key !== resultKey || !state.pagination.cursor) visible.clear();
    resultKey = key;
    for (const result of state.groups) for (const entry of result.items) visible.add(entry);
  });
  const select = async (q: string, options: Omit<SearchQuery, 'q'> = {}) => {
    life.assert(); text = q; group = options.group; await service.select(q, options);
  };
  return {
    context, service, select,
    session: () => ({text, group}),
    recent: () => readSearchHistory(life.api, context.scope, life.signal()),
    clearRecent: () => { life.assert(); return clearSearchHistory(life.api, life.signal()); },
    open(entry: SearchEntry) {
      life.assert();
      if (service.getSnapshot().phase !== 'ready' || !visible.has(entry)) throw new DOMException('Search result changed', 'AbortError');
      context.effects.openEntry(entry);
    },
    async play(entry: SearchEntry) {
      life.assert();
      if (service.getSnapshot().phase !== 'ready' || !visible.has(entry) || !('libraryId' in entry) || !entry.libraryId || !entry.playback || entry.available !== true) throw new DOMException('Search playback changed', 'AbortError');
      await context.effects.play({libraryId: entry.libraryId, itemId: entry.playback.itemId, startSeconds: entry.playback.startSeconds ?? 0}, life.signal());
    },
    dispose() { text = ''; group = undefined; visible.clear(); off(); life.dispose(); service.cancel(); service.dispose(); },
  };
}
export type SearchBindings = ReturnType<typeof createSearchBindings>;
