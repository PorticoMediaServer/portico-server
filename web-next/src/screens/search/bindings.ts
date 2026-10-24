import {SearchService, readSearchHistory, clearSearchHistory, type SearchEntry, type SearchGroup, type SearchQuery} from '@core/search.ts';
import {screenLifetime, type ScreenContext} from '../detail/context';
import {createDetailBindings} from '../detail/bindings';
import {createShowBindings} from '../detail/show-bindings';
import {SEARCH_CHIPS} from '@core/presentation/search-model.ts';
import type {SearchData, SearchGroupResult} from '@core/search.ts';
import {defaultI18n} from '@i18n';
import {SearchGenresController} from './genres';
import {SearchOnNowController} from './on-now';

/** Group narrowing shares counts only within the same actual base query.
 * Page size and recording do not change membership; library scope/order do. */
const summaryKey = (data: SearchData) => JSON.stringify([data.query.q, data.query.sort, data.query.direction,
  data.query.libraryIds, data.query.searchMode, data.scope.serverId, data.scope.viewerFence, data.revision]);
const requestKey = (query: SearchQuery | null) => query && JSON.stringify([query.q, query.group, query.groups, query.sort, query.direction, query.libraryIds]);

/** One session is retained by the parent while Search is off screen; never browser storage. */
export function createSearchBindings(context: ScreenContext) {
  const life = screenLifetime(context);
  const service = new SearchService({api: life.api, scope: context.scope});
  const genres = new SearchGenresController({api: life.api, signal: life.signal(), isCurrent: life.current, slot: 'backdrop', limit: 8});
  const onNow = new SearchOnNowController({api: life.api, serverId: context.scope.serverId,
    timezone: defaultI18n.region.timeZone ?? Intl.DateTimeFormat().resolvedOptions().timeZone,
    isCurrent: life.current, signal: life.signal(), limit: 4});
  let text = '', group: SearchGroup | undefined;
  let resultKey = '';
  let summary: Readonly<{key: string; groups: readonly Pick<SearchGroupResult, 'id' | 'status' | 'totalCount' | 'totalCountExact'>[]}> | undefined;
  const visible = new Set<SearchEntry>();
  const commandContext: ScreenContext = {...context, effects: {...context.effects, async play(target, signal, callerCurrent = () => true) {
    life.assert();signal.throwIfAborted();
    const captured = service.getSnapshot(), query = requestKey(captured.query), source = resultKey;
    const current = () => {
      const state = service.getSnapshot();
      return life.current() && callerCurrent() && state.phase === 'ready' && state.generation === captured.generation
        && requestKey(state.query) === query && resultKey === source;
    };
    if (service.getSnapshot().phase !== 'ready' || !query || !source || !current()) throw new DOMException('Search playback changed', 'AbortError');
    await context.effects.play(target, signal, current);
    if (!current()) throw new DOMException('Search playback changed', 'AbortError');
  }}};
  const topTitle = createDetailBindings(commandContext), topShow = createShowBindings(commandContext);
  const off = service.subscribe(() => {
    const state = service.getSnapshot(), data = state.data;
    if (!data) { visible.clear(); resultKey = ''; return; }
    const key = JSON.stringify([summaryKey(data), data.query.group, data.query.groups]);
    if (!data.query.group) summary = Object.freeze({key: summaryKey(data), groups: Object.freeze(state.groups.map(({id, status, totalCount, totalCountExact}) => Object.freeze({id, status, totalCount, totalCountExact})))});
    if (key !== resultKey || !state.pagination.cursor) visible.clear();
    resultKey = key;
    for (const result of state.groups) for (const entry of result.items) visible.add(entry);
  });
  const select = async (q: string, options: Omit<SearchQuery, 'q'> = {}) => {
    life.assert(); text = q; group = options.group;
    if (q.trim()) { genres.cancel(); onNow.cancel(); }
    await service.select(q, options.group || options.groups ? options : {...options, groups: SEARCH_CHIPS.flatMap(chip => chip.id === 'all' ? [] : [chip.id])});
  };
  return {
    context: commandContext, service, genres, onNow, select, topTitle, topShow,
    session: () => ({text, group}),
    summary: () => {
      const state = service.getSnapshot(), data = state.data;
      return life.current() && state.phase === 'ready' && data && summary?.key === summaryKey(data) ? summary.groups : [];
    },
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
      await commandContext.effects.play({libraryId: entry.libraryId, itemId: entry.playback.itemId, startSeconds: entry.playback.startSeconds ?? 0}, life.signal(), () => visible.has(entry));
    },
    dispose() { text = ''; group = undefined; summary = undefined; visible.clear(); off(); genres.dispose(); onNow.dispose(); life.dispose(); topTitle.dispose(); topShow.dispose(); service.cancel(); service.dispose(); },
  };
}
export type SearchBindings = ReturnType<typeof createSearchBindings>;
