import test from 'node:test';
import assert from 'node:assert/strict';
import {fontReadiness} from './readiness.mjs';

test('used font family and weight must actually load; computed family alone is insufficient', () => {
  const screen = {strings: [{style: {fontFamily: 'Example, sans-serif', fontWeight: '700'}}], fontFaces: []};
  assert.equal(fontReadiness(screen, 'reference').length, 1);
  screen.fontFaces = [{family: 'Example', weight: '400 800', status: 'loaded'}];
  assert.deepEqual(fontReadiness(screen, 'reference'), []);
  screen.fontFaces[0].status = 'error';
  assert.equal(fontReadiness(screen, 'reference').length, 1);
});
