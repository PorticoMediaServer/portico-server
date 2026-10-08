import test from 'node:test';
import assert from 'node:assert/strict';
import {dedupQualityRungs, convertQualityRungs, qualityNetworkFooter, hasQualityFooter} from '../src/presentation/quality-model.ts';
import {noteCastMeta, castMeta} from '../src/presentation/cast-meta.ts';

test('MU4 CON-26: ladder rungs dedupe by id, Convert excludes Automatic/Original', () => {
  const sources = [
    {qualities: [{id: 'auto', kind: 'automatic', label: 'Automatic', enabled: true}, {id: 'o', kind: 'original', label: 'Original', enabled: true}, {id: 'h', kind: 'fixed', label: '1080p', enabled: true}]},
    {qualities: [{id: 'h', kind: 'fixed', label: '1080p', enabled: true}, {id: 'm', kind: 'fixed', label: '720p', enabled: false}]},
  ];
  assert.deepEqual(dedupQualityRungs(sources).map(q => q.id), ['auto', 'o', 'h', 'm']);
  assert.deepEqual(convertQualityRungs(dedupQualityRungs(sources)).map(q => q.id), ['h', 'm']);
  assert.deepEqual(dedupQualityRungs(null), []);
  assert.deepEqual(convertQualityRungs(null), []);
});

test('MU4 CON-26: the same policy gives the same footer on both platforms', () => {
  // "Home network · up to 1080p" / "Away from home · up to 1080p" parts.
  assert.deepEqual(qualityNetworkFooter({networkClass: 'local', maxVideoHeight: 1080}), {network: 'home', height: 1080});
  assert.deepEqual(qualityNetworkFooter({networkClass: 'remote', maxVideoHeight: 1080}), {network: 'away', height: 1080});
  assert.deepEqual(qualityNetworkFooter({networkClass: 'wifi', maxVideoHeight: 720}), {network: 'wifi', height: 720});
  assert.deepEqual(qualityNetworkFooter({networkClass: 'cellular', maxVideoHeight: 480}), {network: 'cellular', height: 480});
  assert.deepEqual(qualityNetworkFooter({networkClass: 'unknown', maxVideoHeight: 1080}), {network: null, height: 1080});
  assert.deepEqual(qualityNetworkFooter(null), {network: null, height: null});
  assert.equal(hasQualityFooter({network: 'home', height: 1080}), true);
  assert.equal(hasQualityFooter({network: null, height: 1080}), false);
  assert.equal(hasQualityFooter({network: 'home', height: null}), false);
});

test('MU4 CAST-03: cast meta is remembered per item and bounded', () => {
  noteCastMeta('item1', {title: 'Jurassic Park', subtitle: 'Adventure', artworkPath: '/v1/items/x/artwork'});
  assert.deepEqual(castMeta('item1'), {title: 'Jurassic Park', subtitle: 'Adventure', artworkPath: '/v1/items/x/artwork'});
  assert.equal(castMeta('unknown'), undefined);
  // Non-/v1/ artwork is dropped, never trusted.
  noteCastMeta('item2', {title: 'X', artworkPath: 'https://evil.example/poster.jpg'});
  assert.deepEqual(castMeta('item2'), {title: 'X'});
  for (let i = 0; i < 30; i++) noteCastMeta(`item-${i}`, {title: `Title ${i}`});
  assert.equal(castMeta('item1'), undefined);
});
