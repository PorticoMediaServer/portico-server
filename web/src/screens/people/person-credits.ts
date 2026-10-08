import type {ContentScope, LibraryContentApi} from '@core/library-content.ts';
import {readPerson, type PersonCredit, type PersonPage, type PersonRole} from '@core/people.ts';
import type {FetchPage} from '@core/collections/index.ts';

/** Credits per windowed page; 5 resident pages keep at most ~300 cards mounted (PERF-S17). */
export const PERSON_CREDIT_PAGE = 60;
export const PERSON_CREDIT_RESIDENT_PAGES = 5;

/**
 * Credits as a windowed collection source. Person paging is cursor-based (no
 * offset seeking), so pages chain: page N waits for page N-1's cursor, and
 * concurrent callers share the chain. A failure drops that page so a retry
 * refetches it. Past the last cursor a page reads empty at the known total.
 */
export function personCreditsSource(api: LibraryContentApi, scope: ContentScope, personId: string, role: PersonRole): FetchPage<PersonCredit> {
  type Chained = {credits: readonly PersonCredit[]; total: number; nextCursor: string};
  const toChained = (page: PersonPage): Chained => ({credits: page.credits, total: page.pageInfo.total, nextCursor: page.pageInfo.nextCursor});
  const pages = new Map<number, Promise<Chained>>();
  const getPage = (page: number): Promise<Chained> => {
    const known = pages.get(page);
    if (known) return known;
    const next = (page === 0
      ? readPerson(api, scope, personId, {role, limit: PERSON_CREDIT_PAGE}).then(toChained)
      : getPage(page - 1).then(prev => {
        if (!prev.nextCursor) return {credits: [], total: prev.total, nextCursor: ''};
        return readPerson(api, scope, personId, {role, limit: PERSON_CREDIT_PAGE, cursor: prev.nextCursor}).then(toChained);
      })
    );
    pages.set(page, next);
    next.catch(() => { if (pages.get(page) === next) pages.delete(page); });
    return next;
  };
  return async (start, _count, _signal) => {
    const chained = await getPage(Math.floor(start / PERSON_CREDIT_PAGE));
    return {items: chained.credits, total: chained.total};
  };
}
