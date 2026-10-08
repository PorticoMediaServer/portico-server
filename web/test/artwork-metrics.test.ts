import test from 'node:test';
import assert from 'node:assert/strict';
import {screenLabel} from '../src/app/artwork-metrics.ts';

test('long ids fold so runs of the same screen compare', () => {
  assert.equal(screenLabel('/libraries/abcdefghijklmnop'), '/libraries/:id');
  assert.equal(screenLabel('/libraries/short'), '/libraries/short');
  assert.equal(screenLabel('/items/abc123XYZ-_4567890abcdef'), '/items/:id');
  // Short segments stay; only 16+ fold.
  assert.equal(screenLabel('/items/abc123'), '/items/abc123');
});

test('the view query survives; other queries do not', () => {
  assert.equal(screenLabel('/library', '?view=grid'), '/library?view=grid');
  assert.equal(screenLabel('/library', '?view=browse&foo=1'), '/library?view=browse');
  assert.equal(screenLabel('/library', '?foo=1'), '/library');
  assert.equal(screenLabel('/library'), '/library');
});

test('empty and unknown inputs stay readable', () => {
  assert.equal(screenLabel(''), '');
  assert.equal(screenLabel('/search', '?view='), '/search');
});
