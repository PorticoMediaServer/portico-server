import test from 'node:test';
import assert from 'node:assert/strict';
import {REMEMBERED_PEOPLE, personCredits, rememberedPerson} from '../src/screens/people/person-memory.ts';

const grid = () => { const g = {disposed: false, dispose() { g.disposed = true; }}; return g as unknown as Parameters<typeof personCredits>[2] extends () => infer C ? C & {disposed: boolean} : never; };

test('Back to a person finds the page, its role and its grid as they were; old visits and other viewers let go', () => {
  const api = {}, scope = {serverId: 's', viewerId: 'v'};
  const first = rememberedPerson(api, scope, 'p1');
  first.role = 'cast';
  const credits = personCredits(first, 'cast', grid);
  assert.equal(rememberedPerson(api, scope, 'p1'), first);
  assert.equal(personCredits(rememberedPerson(api, scope, 'p1'), 'cast', grid), credits, 'the same grid, with its loaded pages');
  for (let i = 0; i < REMEMBERED_PEOPLE; i++) rememberedPerson(api, scope, 'other' + i);
  assert.equal((credits as unknown as {disposed: boolean}).disposed, true, 'pushed out: released');
  const again = rememberedPerson(api, scope, 'p1');
  assert.equal(again.role, 'all');
  const mine = personCredits(again, 'all', grid);
  rememberedPerson(api, {serverId: 's', viewerId: 'someone else'}, 'p1');
  assert.equal((mine as unknown as {disposed: boolean}).disposed, true, 'another viewer never sees this one’s grid');
});
