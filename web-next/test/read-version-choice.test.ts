import test from 'node:test';
import assert from 'node:assert/strict';
import {readVersionAlternatives, readVersionCanChoose, readVersionPick} from '../src/screens/read/read-version-choice.ts';

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
