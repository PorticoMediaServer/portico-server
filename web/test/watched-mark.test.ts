import test from 'node:test';
import assert from 'node:assert/strict';
import {announceWatchedMark, currentWatchedMark, dismissWatchedMark, markRemaining, resetWatchedMark, subscribeWatchedMark, undoWatchedMark} from '../src/app/watched-mark.ts';

const state = (over: Record<string, unknown> = {}) => ({revision: 7, watched: true, watermark: 'w', watchlisted: false, favorite: false, undoToken: 'undo-1', undoUntil: new Date(Date.now() + 30_000).toISOString(), ...over});
const receipt = {revision: 8, watched: false, watermark: 'w2', watchlisted: false, favorite: false};

test('a mark made from a card menu keeps its own receipt, and Undo sends that receipt to the undo route', async () => {
  resetWatchedMark();
  const calls: {path: string; method?: string; body?: unknown}[] = [];
  const api = {request: async <T,>(path: string, method?: string, body?: unknown) => { calls.push({path, method, body}); return receipt as T; }};
  let told = 0;
  const off = subscribeWatchedMark(() => { told++; });
  announceWatchedMark(api, 'show', 'show-1', 'The Long Show', state());
  assert.deepEqual(currentWatchedMark(), {kind: 'show', id: 'show-1', title: 'The Long Show', watched: true, revision: 7, undoToken: 'undo-1', undoUntil: currentWatchedMark()!.undoUntil});
  assert.ok(markRemaining(currentWatchedMark(), Date.now()) > 25_000);
  assert.equal(await undoWatchedMark(), true);
  off();
  // Undo is the server's own (every episode back as it was), never "mark unwatched".
  assert.equal(calls.length, 1);
  assert.equal(calls[0]!.method, 'POST');
  assert.match(calls[0]!.path, /^\/v1\/containers\/show\/show-1\/personal-state\/undo$/);
  assert.deepEqual(calls[0]!.body, {expectedRevision: 7, undoToken: 'undo-1'});
  assert.equal(currentWatchedMark(), undefined);
  assert.ok(told >= 2);
});

test('a receipt without an Undo offer offers nothing', () => {
  resetWatchedMark();
  const api = {request: async <T,>() => receipt as T};
  announceWatchedMark(api, 'album', 'a1', 'Album', state({undoToken: undefined, undoUntil: undefined}));
  assert.equal(currentWatchedMark(), undefined);
});

test('an Undo the server refuses says so and leaves the mark standing; nothing else is sent', async () => {
  resetWatchedMark();
  let calls = 0;
  const api = {request: async <T,>(): Promise<T> => { calls++; throw Object.assign(new Error('gone'), {code: 'undo_expired'}); }};
  announceWatchedMark(api, 'season', 's1', 'Season 1', state());
  assert.equal(await undoWatchedMark(), false);
  assert.equal(calls, 1);
  assert.equal(currentWatchedMark()?.failed, true);
  dismissWatchedMark(currentWatchedMark());
  assert.equal(currentWatchedMark(), undefined);
});

test('a newer mark replaces the offer for an older one, and an expired offer has no time left', () => {
  resetWatchedMark();
  const api = {request: async <T,>() => receipt as T};
  announceWatchedMark(api, 'show', 'one', 'One', state({undoToken: 'a'}));
  announceWatchedMark(api, 'book', 'two', 'Two', state({undoToken: 'b', watched: false}));
  assert.equal(currentWatchedMark()?.id, 'two');
  assert.equal(currentWatchedMark()?.watched, false);
  assert.equal(markRemaining({undoUntil: new Date(Date.now() - 1000).toISOString()}, Date.now()), 0);
  assert.equal(markRemaining({undoUntil: 'not a date'}, Date.now()), 0);
  assert.equal(markRemaining(undefined, Date.now()), 0);
  resetWatchedMark();
});
