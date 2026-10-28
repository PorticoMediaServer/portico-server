import test from 'node:test';
import assert from 'node:assert/strict';
import {MemberProfilesClient, type MemberProfilesApi} from '../src/member-profiles.ts';

const restrictions = (profileId: string, patch: Record<string, unknown> = {}) => ({profileId, ratingSystem: '', maximumAgeRating: '', maximumAge: -1, allowUnrated: true,
  blockedLabels: [], allowDownloads: true, allowLiveTv: true, allowDvr: true, allowWatchTogether: true, revision: 1, ...patch});
const profile = (id: string, patch: Record<string, unknown> = {}) => ({id, name: id === 'main' ? 'Nana' : 'Movie night', art: 'coral', primary: id === 'main', position: id === 'main' ? 0 : 1,
  revision: 3, pinRequired: id !== 'main', pinRevision: 1, allowedLibraries: [], restrictions: restrictions(id), ...patch});
function client(answer: (path: string, method?: string, body?: unknown) => unknown) {
  const calls: {path: string; method?: string; body?: unknown}[] = [];
  const api: MemberProfilesApi = {request: (async (path: string, method?: string, body?: unknown) => {calls.push({path, method, body}); return answer(path, method, body);}) as MemberProfilesApi['request']};
  return {calls, profiles: new MemberProfilesClient(api, 'nana')};
}

test('lists exactly the named person\'s profiles and refuses another account\'s answer', async () => {
  const f = client(() => ({accountId: 'nana', items: [profile('main'), profile('movies')]}));
  const items = await f.profiles.list();
  assert.equal(f.calls[0].path, '/v1/admin/access/members/nana/profiles');
  assert.deepEqual(items.map(item => [item.id, item.primary, item.pinRequired]), [['main', true, false], ['movies', false, true]]);
  for (const raw of [{accountId: 'someone-else', items: []}, {accountId: 'nana', items: [profile('main'), profile('main')]}, {accountId: 'nana', items: [profile('main'), profile('movies', {primary: true})]},
    {accountId: 'nana', items: [profile('movies', {restrictions: restrictions('main')})]}, {accountId: 'nana', items: [profile('movies', {art: 'plaid'})]}]) {
    await assert.rejects(client(() => raw).profiles.list(), JSON.stringify(raw).slice(0, 80));
  }
});

test('edits are fenced on the revision read and must come back as the same profile', async () => {
  const f = client((path, method) => method === 'PATCH' ? profile('movies', {name: 'Film club', revision: 4}) : method === 'PUT' ? profile('movies', {restrictions: restrictions('movies', {maximumAgeRating: 'PG', ratingSystem: 'mpaa', maximumAge: 8, revision: 2})}) : profile('movies', {pinRequired: false, revision: 4}));
  const movies = (await client(() => ({accountId: 'nana', items: [profile('movies')]})).profiles.list())[0];
  const renamed = await f.profiles.rename(movies, {name: 'Film club'});
  assert.deepEqual(f.calls[0], {path: '/v1/admin/access/members/nana/profiles/movies', method: 'PATCH', body: {expectedRevision: 3, name: 'Film club'}});
  assert.equal(renamed.name, 'Film club');
  const limited = await f.profiles.restrict(movies, {ratingSystem: 'mpaa', maximumAgeRating: 'PG'});
  assert.equal(f.calls[1].path, '/v1/admin/access/members/nana/profiles/movies/restrictions');
  assert.equal((f.calls[1].body as {expectedRevision: number}).expectedRevision, 1);
  assert.equal(limited.restrictions.maximumAgeRating, 'PG');
  const cleared = await f.profiles.resetPIN(movies);
  assert.deepEqual(f.calls[2], {path: '/v1/admin/access/members/nana/profiles/movies/pin-reset', method: 'POST', body: {expectedRevision: 3}});
  assert.equal(cleared.pinRequired, false);
  // An answer for a different profile, or a reset that left the PIN, is not a confirmation.
  await assert.rejects(client(() => profile('main')).profiles.rename(movies, {name: 'x'}));
  await assert.rejects(client(() => profile('movies')).profiles.resetPIN(movies));
});
