import type {WindowedCollection} from '@core/collections/index.ts';
import type {PersonCredit, PersonPage, PersonRole} from '@core/people.ts';

/**
 * The last few people visited, kept so Back to a person shows the page at once: the header, the
 * role chosen, and the credits grid with the pages it had loaded (so the window can scroll back to
 * where it was). The open page reads itself again in place as before. Kept per viewer on one
 * server; another viewer's visit forgets all of it, and a person pushed out releases their grid.
 */
export const REMEMBERED_PEOPLE = 5;

type Kept = {page?: PersonPage; role: PersonRole; credits: Map<PersonRole, WindowedCollection<PersonCredit>>};
const people = new Map<string, Kept>();
let currentScope = '';
let currentApi: unknown;

function release(kept: Kept) { for (const credits of kept.credits.values()) credits.dispose(); }

/** The kept page of `personId` for this viewer, made (empty) on a first visit. */
export function rememberedPerson(api: unknown, scope: {serverId: string; viewerId: string}, personId: string): Kept {
  const viewer = scope.serverId + '\n' + scope.viewerId;
  // Another viewer, or a new connection to the server: the kept grids read through the old one.
  if (viewer !== currentScope || api !== currentApi) { for (const kept of people.values()) release(kept); people.clear(); currentScope = viewer; currentApi = api; }
  let kept = people.get(personId);
  if (kept) people.delete(personId); else kept = {role: 'all', credits: new Map()};
  people.set(personId, kept);
  while (people.size > REMEMBERED_PEOPLE) { const [oldest, gone] = people.entries().next().value!; people.delete(oldest); release(gone); }
  return kept;
}

/** The person's credits for `role`: the kept grid, or a new one (one per role, at most three). */
export function personCredits(kept: Kept, role: PersonRole, make: () => WindowedCollection<PersonCredit>): WindowedCollection<PersonCredit> {
  let credits = kept.credits.get(role);
  if (!credits) { credits = make(); kept.credits.set(role, credits); }
  return credits;
}
