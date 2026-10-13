import test from 'node:test';
import assert from 'node:assert/strict';
import {ALREADY_DONE, arrivals, isStaleAnswer, mergeRevalidatedList, retryWhenStale} from '../src/freshness-actions.ts';

const stale = () => Object.assign(new Error('moved'), {code: 'personal_state_conflict', status: 409});
const row = (id: string, text = '') => ({id, text});
const ids = (list: readonly {id: string}[]) => list.map(e => e.id).join(',');

test('an out-of-date answer is met with a re-read and another attempt, silently', async () => {
  let revision = 1, rereads = 0;
  const sent: number[] = [];
  const result = await retryWhenStale(async () => { sent.push(revision); if (revision < 3) throw stale(); return 'saved'; }, {reread: async () => { rereads++; revision++; }});
  assert.equal(result, 'saved');
  assert.deepEqual(sent, [1, 2, 3]);
  assert.equal(rereads, 2);
});

test('only a real conflict reaches the person: it keeps losing, no longer applies, or is another kind of failure', async () => {
  let attempts = 0;
  await assert.rejects(retryWhenStale(async () => { attempts++; throw stale(); }, {reread: async () => {}}), {code: 'personal_state_conflict'});
  assert.equal(attempts, 3, 'two silent retries, then the answer is shown');
  attempts = 0;
  await assert.rejects(retryWhenStale(async () => { attempts++; throw stale(); }, {reread: async () => {}, stillApplies: () => false}), {code: 'personal_state_conflict'});
  assert.equal(attempts, 1, 'the thing they acted on is gone: nothing is sent again');
  attempts = 0;
  await assert.rejects(retryWhenStale(async () => { attempts++; throw Object.assign(new Error('no'), {status: 403, code: 'forbidden'}); }, {reread: async () => { throw new Error('never called'); }}), {code: 'forbidden'});
  assert.equal(attempts, 1);
  // A re-read that fails leaves the first answer; the action is not sent blind.
  await assert.rejects(retryWhenStale(async () => { throw stale(); }, {reread: async () => { throw new Error('offline'); }}), {code: 'personal_state_conflict'});
});

test('an action another device already took is simply done', async () => {
  let attempts = 0;
  const result = await retryWhenStale(async () => { attempts++; throw stale(); }, {reread: async () => {}, stillApplies: () => 'done'});
  assert.equal(result, ALREADY_DONE);
  assert.equal(attempts, 1);
});

test('what counts as out of date', () => {
  for (const code of ['stale_continuation', 'invalid_cursor', 'personal_state_conflict', 'playlist_conflict', 'console_conflict', 'revision_conflict']) assert.equal(isStaleAnswer({code}), true, code);
  assert.equal(isStaleAnswer({status: 412}), true);
  assert.equal(isStaleAnswer({retry: 'after_refresh'}), true);
  for (const error of [{code: 'forbidden', status: 403}, {code: 'dvr_conflict', status: 409}, {status: 500}, new TypeError('Failed to fetch'), null, 'text']) assert.equal(isStaleAnswer(error), false);
});

test('a fresh first page is merged into a list that was paged further: nothing below the person disappears', () => {
  const shown = Array.from({length: 8}, (_, i) => row('a' + (i + 1)));
  // A new title at the top pushes the first page down by one; the tail stays.
  assert.equal(ids(mergeRevalidatedList(shown, [row('new'), shown[0]!, shown[1]!, shown[2]!], e => e.id)), 'new,a1,a2,a3,a4,a5,a6,a7,a8');
  // A title inside the first page's reach was removed: it goes, and the page's new last entry is not repeated in the tail.
  assert.equal(ids(mergeRevalidatedList(shown, [shown[0]!, shown[2]!, shown[3]!, shown[4]!], e => e.id)), 'a1,a3,a4,a5,a6,a7,a8');
  // A changed entry is replaced in place; an unchanged one is the same object.
  const changed = mergeRevalidatedList(shown, [shown[0]!, row('a2', 'watched'), shown[2]!], e => e.id, {same: (a, b) => a.text === b.text});
  assert.equal(changed[0], shown[0]);
  assert.equal(changed[1]!.text, 'watched');
  assert.equal(changed.length, 8);
  // Nothing changed at all: the very same array, so nothing re-renders.
  assert.equal(mergeRevalidatedList(shown, shown.slice(0, 3).map(e => ({...e})), e => e.id, {same: (a, b) => a.text === b.text}), shown);
  // The fresh page is the whole list now: nothing beyond it is kept.
  assert.equal(ids(mergeRevalidatedList(shown, [shown[0]!, shown[1]!], e => e.id, {complete: true})), 'a1,a2');
  // A different list altogether (another sort): the fresh page replaces everything.
  assert.equal(ids(mergeRevalidatedList(shown, [row('x'), row('y')], e => e.id)), 'x,y');
  assert.equal(ids(mergeRevalidatedList([], [row('x')], e => e.id)), 'x');
});

test('only a few new cards fade in; a first load or a new sort animates nothing', () => {
  const before = [row('a'), row('b')];
  assert.deepEqual([...arrivals(before, [row('n'), row('a'), row('b')], e => e.id)], ['n']);
  assert.equal(arrivals(undefined, [row('a')], e => e.id).size, 0);
  assert.equal(arrivals([], [row('a')], e => e.id).size, 0);
  assert.equal(arrivals(before, Array.from({length: 40}, (_, i) => row('z' + i)), e => e.id).size, 0);
});
