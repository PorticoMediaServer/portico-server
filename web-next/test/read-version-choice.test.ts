import test from 'node:test';
import assert from 'node:assert/strict';
import {readBackDestination, readRouteEntry, readSameRouteEntry, readVersionAlternatives, readVersionCanChoose,
  readVersionPick, readVersionPostRetire, readVersionPreRetire, readVersionRecovery, type ReadRouteEntry} from '../src/screens/read/read-version-choice.ts';

const epub = {sourceId: 'epub-source', format: 'epub' as const, size: 900, inventoryAvailable: true};
const pdf = {sourceId: 'pdf-source', format: 'pdf' as const, size: 1200, inventoryAvailable: true};
const stale = {sourceId: 'stale-source', format: 'pdf' as const, size: 800, inventoryAvailable: false};
const route = {} as never;
const entry = (key: string, href = `/item/book?entry=${key}`): ReadRouteEntry => ({href, key});
const location = (value: ReadRouteEntry) => ({href: value.href, state: {__TSR_key: value.key}});

test('a rejected default EPUB leaves the available PDF choosable', () => {
  const versions = [epub, pdf];
  assert.equal(readVersionCanChoose(versions), true);
  const picked = readVersionPick(versions, epub.sourceId, pdf.sourceId);
  assert.equal(picked?.sourceId, pdf.sourceId);
  assert.deepEqual(readVersionAlternatives(versions, epub.sourceId).map(v => v.sourceId), [pdf.sourceId]);
});

test('unavailable, duplicate and unchanged versions cannot dispatch', () => {
  assert.equal(readVersionPick([epub, stale], epub.sourceId, stale.sourceId), undefined);
  assert.equal(readVersionPick([epub, pdf], epub.sourceId, epub.sourceId), undefined);
  assert.equal(readVersionPick([epub, pdf], epub.sourceId, undefined), undefined);
  assert.equal(readVersionPick([epub, pdf], epub.sourceId, 'absent-source'), undefined);
  assert.equal(readVersionPick([epub, {...pdf, sourceId: pdf.sourceId}], epub.sourceId, pdf.sourceId), undefined, 'duplicate ids refuse');
});

test('a one-version item shows no chooser', () => {
  assert.equal(readVersionCanChoose([epub]), false);
  assert.equal(readVersionCanChoose([]), false);
  assert.deepEqual(readVersionAlternatives([epub], epub.sourceId), []);
});

test('the actual source sequence admits failed A then dispatches B after retirement', () => {
  const versions = [epub, pdf];
  const opened = entry('entry-a');
  const attemptA = {live: true, aborted: false, sourceId: epub.sourceId};
  const rendered = {context: route, sourceId: epub.sourceId, versions, attempt: attemptA};
  const committed = {context: route, sourceId: epub.sourceId, versions};
  let currentAttempt: unknown = attemptA;
  const facts = {
    liveRoute: true, route: opened, currentRoute: opened, rendered, committed,
    currentAttempt, attemptLive: !attemptA.aborted && attemptA.live, candidate: pdf,
    retire: (value: unknown) => {
      assert.equal(value, attemptA, 'retirement may only target the attempt the row was rendered with');
      attemptA.live = false; attemptA.aborted = true; currentAttempt = undefined;
    },
    afterRetire: () => ({liveRoute: true, currentRoute: opened, successor: currentAttempt, committed}),
  };
  // This is the sequence the component calls; it must reach setSourceId for a failed A.
  assert.equal(readVersionRecovery(facts), true);
  assert.equal(attemptA.live, false, 'the original attempt really was retired');
  assert.equal(currentAttempt, undefined, 'retirement cleared the attempt ref');
  // The post-retire leg must not require the retired attempt to still be live or current.
  assert.equal(readVersionPostRetire({
    liveRoute: true, route: opened, currentRoute: opened, rendered, committed, successor: undefined, candidate: pdf,
  }), true);
});

test('a retained row is refused after a successor, replacement or navigation', () => {
  const versions = [epub, pdf];
  const opened = entry('entry-a'), elsewhere = entry('entry-b');
  const attemptA = {live: true, aborted: false}, attemptB = {live: true, aborted: false};
  const rendered = {context: route, sourceId: epub.sourceId, versions, attempt: attemptA};
  const committed = {context: route, sourceId: epub.sourceId, versions};
  const base = {liveRoute: true, route: opened, currentRoute: opened, rendered, committed, currentAttempt: attemptA, attemptLive: true, candidate: pdf};
  assert.equal(readVersionPreRetire(base), true);
  // A successor attempt exists: the old callback must not retire it.
  assert.equal(readVersionPreRetire({...base, currentAttempt: attemptB}), false);
  // The original was already retired.
  assert.equal(readVersionPreRetire({...base, attemptLive: false}), false);
  // The committed source or list was replaced (even with equal contents).
  assert.equal(readVersionPreRetire({...base, committed: {...committed, sourceId: pdf.sourceId}}), false);
  assert.equal(readVersionPreRetire({...base, committed: {...committed, versions: [...versions]}}), false);
  // Another route entry replaced the original.
  assert.equal(readVersionPreRetire({...base, currentRoute: elsewhere}), false);
  assert.equal(readVersionPreRetire({...base, liveRoute: false}), false);
  // Both attempts undefined never admits.
  assert.equal(readVersionPreRetire({...base, rendered: {...rendered, attempt: undefined}, currentAttempt: undefined}), false);
  // The row itself must still be the unique, available, non-current row.
  assert.equal(readVersionPreRetire({...base, candidate: epub}), false);
  assert.equal(readVersionPreRetire({...base, rendered: {...rendered, versions: [epub, stale]}}), false);
});

test('post-retire still refuses a successor or a replaced witness', () => {
  const versions = [epub, pdf];
  const opened = entry('entry-a'), elsewhere = entry('entry-b');
  const attemptA = {live: false, aborted: true};
  const rendered = {context: route, sourceId: epub.sourceId, versions, attempt: attemptA};
  const committed = {context: route, sourceId: epub.sourceId, versions};
  const base = {liveRoute: true, route: opened, currentRoute: opened, rendered, committed, successor: undefined, candidate: pdf};
  assert.equal(readVersionPostRetire(base), true);
  assert.equal(readVersionPostRetire({...base, successor: {}}), false);
  assert.equal(readVersionPostRetire({...base, currentRoute: elsewhere}), false);
  assert.equal(readVersionPostRetire({...base, committed: {...committed, versions: [...versions]}}), false);
});

test('the route witness is the real __TSR_key entry, not history.location.key', () => {
  assert.deepEqual(readRouteEntry(location(entry('entry-a'))), entry('entry-a'));
  assert.equal(readRouteEntry({href: '/item/book', state: {}}), undefined);
  assert.equal(readRouteEntry({href: '/item/book', state: {__TSR_key: ''}}), undefined);
  assert.equal(readRouteEntry({href: '/item/book', state: {__TSR_key: 7 as never}}), undefined);
  assert.equal(readSameRouteEntry(entry('entry-a'), entry('entry-a')), true);
  assert.equal(readSameRouteEntry(entry('entry-a'), entry('entry-b')), false);
  assert.equal(readSameRouteEntry(entry('entry-a'), {...entry('entry-a'), href: '/other'}), false);
  assert.equal(readSameRouteEntry(undefined, entry('entry-a')), false);
});

test('Back returns in-app only when the captured route really has a predecessor', () => {
  assert.equal(readBackDestination(true), 'back');
  assert.equal(readBackDestination(false), 'home');
});
