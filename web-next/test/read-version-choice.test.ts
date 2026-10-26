import test from 'node:test';
import assert from 'node:assert/strict';
import {readBackDestination, readVersionAdmission, readVersionAlternatives, readVersionCanChoose, readVersionPick} from '../src/screens/read/read-version-choice.ts';

const epub = {sourceId: 'epub-source', format: 'epub' as const, size: 900, inventoryAvailable: true};
const pdf = {sourceId: 'pdf-source', format: 'pdf' as const, size: 1200, inventoryAvailable: true};
const stale = {sourceId: 'stale-source', format: 'pdf' as const, size: 800, inventoryAvailable: false};

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

test('the component admits an explicit recovery only from the exact render it was offered on', () => {
  const offered = [epub, pdf];
  const route = {} as never, attempt = {} as never;
  const base = {liveRoute: true, renderedContext: route, currentContext: route, renderedSourceId: 'epub-source', currentSourceId: 'epub-source',
    renderedAttempt: attempt, currentAttempt: attempt, attemptLive: true, candidate: pdf, offered, currentOffered: offered};
  assert.equal(readVersionAdmission(base), true);
  // A retained menu callback after the attempt was replaced must not retire the successor.
  assert.equal(readVersionAdmission({...base, currentAttempt: {} as never}), false);
  // A replaced source or version list refuses the old row.
  assert.equal(readVersionAdmission({...base, currentSourceId: 'pdf-source'}), false);
  assert.equal(readVersionAdmission({...base, offered: [epub, {...pdf, size: 999}], currentOffered: [epub, {...pdf, size: 999}]}), false);
  // A replaced committed list invalidates the callback's own captured list even when it matches itself.
  assert.equal(readVersionAdmission({...base, currentOffered: [epub]}), false);
  assert.equal(readVersionAdmission({...base, offered: [epub, stale]}), false);
  // A stale route/context and a retired original also refuse, while a failed original stays admissible.
  assert.equal(readVersionAdmission({...base, liveRoute: false}), false);
  assert.equal(readVersionAdmission({...base, currentContext: {} as never}), false);
  assert.equal(readVersionAdmission({...base, attemptLive: false}), false);
  assert.equal(readVersionAdmission({...base, renderedAttempt: undefined, currentAttempt: undefined}), false, 'both undefined never admits');
  assert.equal(readVersionAdmission({...base, candidate: epub}), false, 'the current version is not a change');
});

test('Back returns in-app only when the captured route really has a predecessor', () => {
  assert.equal(readBackDestination(true), 'back');
  assert.equal(readBackDestination(false), 'home');
});
