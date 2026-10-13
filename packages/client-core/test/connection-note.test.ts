import test from 'node:test';
import assert from 'node:assert/strict';
import {parseConnectionFacts, plainHttpOutsideHome} from '../src/connection-note.ts';

test('plain HTTP from outside the home networks is worth a note; anything else is not', () => {
  assert.equal(parseConnectionFacts({secure: false, remote: true, plainHttpOutsideHome: true, clientAddress: '203.0.113.9', accessUrls: []}).plainHttpOutsideHome, true);
  assert.equal(parseConnectionFacts({secure: false, remote: false, plainHttpOutsideHome: false}).plainHttpOutsideHome, false);
  assert.equal(parseConnectionFacts({secure: true, remote: true, plainHttpOutsideHome: false}).plainHttpOutsideHome, false);
  // A contradictory answer never produces the note on an encrypted connection.
  assert.equal(parseConnectionFacts({secure: true, remote: true, plainHttpOutsideHome: true}).plainHttpOutsideHome, false);
});

test('an answer that cannot be read says nothing', () => {
  for (const value of [undefined, null, 'yes', [], {plainHttpOutsideHome: 'true'}, {newField: 1}]) {
    assert.deepEqual(parseConnectionFacts(value), {secure: false, remote: false, plainHttpOutsideHome: false});
  }
});

test('a server that does not answer the question shows no note', async () => {
  const paths: string[] = [];
  assert.equal(await plainHttpOutsideHome({request: async (path: string) => { paths.push(path); throw Object.assign(new Error('not found'), {status: 404}); }}), false);
  assert.equal(await plainHttpOutsideHome({request: async <T>() => ({secure: false, remote: true, plainHttpOutsideHome: true}) as T}), true);
  assert.deepEqual(paths, ['/v1/networking/connection']);
});
