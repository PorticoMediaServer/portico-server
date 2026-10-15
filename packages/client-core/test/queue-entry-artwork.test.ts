import test from 'node:test';
import assert from 'node:assert/strict';
import {parseEntry} from '../src/playback-v1/types.ts';

test('a queue entry carries its poster and still for the Up next card, only as this server’s addresses', () => {
  const base = {entryId: 'e1', position: 0, itemId: 'i1', kind: 'episode', title: 'Pilot', available: true};
  const entry = parseEntry({...base, posterUrl: '/v1/items/i1/art/poster', stillUrl: '/v1/metadata/item/i1/art/still?v=ab&w=800'});
  assert.equal(entry.posterUrl, '/v1/items/i1/art/poster');
  assert.equal(entry.stillUrl, '/v1/metadata/item/i1/art/still?v=ab&w=800');
  const foreign = parseEntry({...base, posterUrl: 'https://elsewhere.example/p.jpg', stillUrl: 7});
  assert.equal(foreign.posterUrl, undefined);
  assert.equal(foreign.stillUrl, undefined);
});
