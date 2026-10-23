import {createWindowedCollection, type WindowedCollection} from '@core/collections/index.ts';
import {readPerson, type PersonPage, type PersonCredit, type PersonRole} from '@core/people.ts';
import {screenLifetime, type ScreenContext} from './context';

type PersonSnapshot = Readonly<{phase: 'idle' | 'loading' | 'ready' | 'error'; page?: PersonPage; role: PersonRole; error?: unknown}>;
/** People and credits are server identities. Paging keeps at most five resident pages per group. */
export function createPersonBindings(context: ScreenContext) {
  const life = screenLifetime(context), listeners = new Set<() => void>();
  let generation = 0, personId = '', state: PersonSnapshot = {phase: 'idle', role: 'all'};
  let read: AbortController | undefined;
  const collections = new Map<string, WindowedCollection<PersonCredit>>();
  const publish = (next: PersonSnapshot) => { state = Object.freeze(next); for (const listener of listeners) listener(); };
  const clear = () => { for (const collection of collections.values()) collection.dispose(); collections.clear(); };
  const select = async (id: string, role: PersonRole = 'all') => {
    life.assert(); const request = ++generation; read?.abort(); clear(); personId = id;
    const controller = new AbortController(); read = controller;
    publish({phase: 'loading', role});
    try {
      const page = await readPerson(life.api, context.scope, id, {role, limit: 1, signal: life.signal(controller.signal)});
      if (request !== generation || !life.current()) return;
      publish({phase: 'ready', role, page});
    } catch (error) { if (request === generation && life.current() && !controller.signal.aborted) publish({phase: 'error', role, error}); }
  };
  const credits = (page: PersonPage, kind?: string) => {
    life.assert(); const header = state.page, role = state.role, request = generation, id = personId;
    if (!header || page !== header || state.phase !== 'ready') throw new Error('Person required');
    const key = JSON.stringify([request, kind, role]);
    const known = collections.get(key); if (known) return known;
    // Cursor addresses are retained; media pages belong only to the sparse collection.
    const cursors = new Map<number, string | null>([[0, null]]), pageSize = 40;
    let total = kind ? header.groups.find(group => group.kind === kind)?.total ?? header.pageInfo.total : header.pageInfo.total;
    const pending = new Map<number, Promise<{items: readonly PersonCredit[]; total: number}>>();
    const fetch = async (page: number, signal: AbortSignal): Promise<{items: readonly PersonCredit[]; total: number}> => {
      const existing = pending.get(page); if (existing) return existing;
      const work = (async () => {
        if (!cursors.has(page)) await fetch(page - 1, signal);
        if (cursors.get(page) === '') return {items: [], total};
        const value = await readPerson(life.api, context.scope, id, {...(kind ? {kind} : {role}), limit: pageSize, cursor: cursors.get(page), signal: life.signal(signal)});
        life.assert();
        if (request !== generation || state.page !== header || value.viewerFence !== header.viewerFence || value.revision.catalog !== header.revision.catalog || value.revision.viewer !== header.revision.viewer) throw new DOMException('Person credits changed', 'AbortError');
        if (value.pageInfo.nextCursor && [...cursors.values()].includes(value.pageInfo.nextCursor)) throw new Error('Repeated person cursor');
        total = value.pageInfo.total; cursors.set(page + 1, value.pageInfo.nextCursor);
        return {items: value.credits, total: value.pageInfo.total};
      })();
      pending.set(page, work);
      try { return await work; } finally { if (pending.get(page) === work) pending.delete(page); }
    };
    const collection = createWindowedCollection<PersonCredit>({pageSize, maxResidentPages: 5, keyOf: credit => JSON.stringify([credit.media.libraryId, credit.media.id]), fetchPage: (start, _count, signal) => fetch(Math.floor(start / pageSize), signal)});
    collections.set(key, collection); return collection;
  };
  return {
    context, select, credits,
    getSnapshot: () => state,
    subscribe: (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; },
    open(page: PersonPage, credit: PersonCredit) {
      life.assert();
      if (state.page !== page || ![...collections.values()].some(collection => collection.residentItems().includes(credit))) throw new DOMException('Person title changed', 'AbortError');
      context.effects.openEntry(credit.media);
    },
    retry: () => select(personId, state.role),
    dispose() { generation++; life.dispose(); read?.abort(); clear(); personId = ''; publish({phase: 'idle', role: 'all'}); listeners.clear(); },
  };
}
export type PersonBindings = ReturnType<typeof createPersonBindings>;
