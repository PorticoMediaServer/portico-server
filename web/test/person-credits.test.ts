/**
 * PERF-S17: person credits page through a cursor-chained windowed source, so a
 * prolific filmography costs O(visible) requests and O(window) memory.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {createWindowedCollection} from '@core/collections/index.ts';
import {PERSON_CREDIT_PAGE, PERSON_CREDIT_RESIDENT_PAGES, personCreditsSource} from '../src/screens/people/person-credits.ts';
import type {PersonCredit} from '@core/people.ts';

const scope = {serverId: 'server-one', viewerId: 'local:one:profile'};

/** A fake person endpoint: `total` credits in server order, `limit` per page, cursor = offset. */
function fakePerson(total: number, failAt?: number) {
  const seen: string[] = [];
  let failures = 0;
  const entry = (i: number) => ({id: `title-${i}`, kind: 'movie', title: `Title ${i}`});
  const api = {
    async request<T>(path: string): Promise<T> {
      seen.push(path);
      const url = new URL(path, 'https://server.invalid');
      const limit = Number(url.searchParams.get('limit') ?? '40');
      const cursor = url.searchParams.get('cursor');
      const offset = cursor ? Number(cursor) : 0;
      if (failAt !== undefined && offset === failAt && failures++ === 0) throw new Error('busy server');
      const credits = Array.from({length: Math.min(limit, total - offset)}, (_, k) => {
        const i = offset + k;
        return {role: 'Actor', character: `Character ${i}`, department: '', creditKind: 'cast', media: entry(i)};
      });
      const next = offset + credits.length < total ? String(offset + credits.length) : '';
      return {
        serverId: scope.serverId, viewerFence: 'fence-one', revision: {catalog: 1, viewer: 1},
        person: {id: 'person-one', name: 'Prolific Actor', sortName: 'Actor, Prolific', biography: '', birthDate: '', deathDate: '', portraitUrl: '', roles: ['Actor'], knownFor: [entry(0)], providerIds: {}, revision: 1},
        credits, pageInfo: {total, nextCursor: next},
      } as T;
    },
  };
  return {api, seen};
}

const tick = () => new Promise(done => setTimeout(done, 0));
async function drainFor(total: () => number | undefined, want: number, tries = 500) {
  for (let i = 0; i < tries && total() === undefined; i++) await tick();
  assert.equal(total(), want);
}

test('PERF-S17: deep pages chain one cursor at a time, in order', async () => {
  const {api, seen} = fakePerson(150);
  const source = personCreditsSource(api as never, scope, 'person-one', 'all');
  const page0 = await source(0, PERSON_CREDIT_PAGE, AbortSignal.timeout(1000));
  assert.equal(page0.total, 150);
  assert.equal(page0.items.length, PERSON_CREDIT_PAGE);
  assert.equal((page0.items[0] as PersonCredit).media.id, 'title-0');
  const page2 = await source(2 * PERSON_CREDIT_PAGE, PERSON_CREDIT_PAGE, AbortSignal.timeout(1000));
  assert.equal((page2.items[0] as PersonCredit).media.id, 'title-120');
  const cursors = seen.map(p => new URL(p, 'https://server.invalid').searchParams.get('cursor'));
  assert.deepEqual(cursors, [null, '60', '120'], 'page 2 walks page 1 first; each cursor is fetched once');
});

test('PERF-S17: a failed page is refetched on retry, not cached as failed', async () => {
  const {api, seen} = fakePerson(150, 60);
  const source = personCreditsSource(api as never, scope, 'person-one', 'all');
  await assert.rejects(source(PERSON_CREDIT_PAGE, PERSON_CREDIT_PAGE, AbortSignal.timeout(1000)));
  const page1 = await source(PERSON_CREDIT_PAGE, PERSON_CREDIT_PAGE, AbortSignal.timeout(1000));
  assert.equal((page1.items[0] as PersonCredit).media.id, 'title-60');
  assert.equal(seen.filter(p => p.includes('cursor=60')).length, 2, 'the failed cursor is requested again');
});

test('PERF-S17: a 2,000-credit filmography holds at most ~300 items at any depth', async () => {
  assert.equal(PERSON_CREDIT_PAGE * PERSON_CREDIT_RESIDENT_PAGES, 300);
  const {api} = fakePerson(2000);
  const collection = createWindowedCollection<PersonCredit>({
    fetchPage: personCreditsSource(api as never, scope, 'person-one', 'all'),
    pageSize: PERSON_CREDIT_PAGE, maxResidentPages: PERSON_CREDIT_RESIDENT_PAGES,
    keyOf: c => `${c.media.id}:${c.role}:${c.character}`,
  });
  try {
    collection.ensureRange(0, 59);
    await drainFor(() => collection.getSnapshot().total, 2000);
    collection.ensureRange(1500, 1559);
    for (let i = 0; i < 500 && collection.itemAt(1559) === undefined; i++) await tick();
    assert.ok(collection.itemAt(1500), 'the deep window loads');
    assert.ok(collection.resident().items <= 300, `at most a window stays resident (${collection.resident().items})`);
    assert.equal(collection.getSnapshot().total, 2000);
  } finally {
    collection.dispose();
  }
});
