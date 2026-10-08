import test from 'node:test';
import assert from 'node:assert/strict';
import {episodeArt, heroLayout, mosaicPaths} from '../src/app/title-layout.ts';

test('M26: hero without a backdrop is compact, never the tall empty band', () => {
  assert.equal(heroLayout({backdropUrl: '/v1/backdrop'}), 'full');
  assert.equal(heroLayout({}), 'compact');
  assert.equal(heroLayout({backdropUrl: undefined}), 'compact');
  // Loading keeps the skeleton geometry until data lands (no layout jump).
  assert.equal(heroLayout({loading: true}), 'loading');
  assert.equal(heroLayout({loading: true, backdropUrl: '/v1/backdrop'}), 'full');
  // Loaded without a backdrop compacts even if it was loading before.
  assert.equal(heroLayout({loading: false}), 'compact');
});

test('M26: an episode shows its own still, else the placeholder — never the show backdrop', () => {
  assert.deepEqual(episodeArt({stillUrl: '/v1/metadata/item/e1/art/still?v=1'}), {kind: 'still', path: '/v1/metadata/item/e1/art/still?v=1'});
  // The backdrop and poster are the season's or show's art: never used as the episode's still.
  assert.deepEqual(episodeArt({backdropUrl: '/v1/backdrop', posterUrl: '/v1/poster'} as never), {kind: 'placeholder'});
  assert.deepEqual(episodeArt({}), {kind: 'placeholder'});
  assert.deepEqual(episodeArt({stillUrl: undefined}), {kind: 'placeholder'});
});

test('M26: collection mosaic uses custom art when present, else the first 4 item posters', () => {
  // Custom art wins: no mosaic.
  assert.equal(mosaicPaths({posterUrl: '/v1/custom'}), undefined);
  assert.equal(mosaicPaths({backdropUrl: '/v1/custom'}), undefined);
  // The server's own mosaic paths win over the loaded items.
  assert.deepEqual(
    mosaicPaths({artworkPaths: ['/a', '/b']}, [{posterUrl: '/x'}]),
    ['/a', '/b'],
  );
  // Otherwise the first 4 item posters or covers, in order, skipping artless items.
  const items = [{posterUrl: '/1'}, {}, {backdropUrl: '/3'}, {posterUrl: '/4'}, {posterUrl: '/5'}, {posterUrl: '/6'}];
  assert.deepEqual(mosaicPaths({}, items), ['/1', '/3', '/4', '/5']);
  // Nothing with art: empty (the hero renders nothing, not empty tiles).
  assert.deepEqual(mosaicPaths({}, [{}, {}]), []);
  assert.deepEqual(mosaicPaths({}), []);
});
