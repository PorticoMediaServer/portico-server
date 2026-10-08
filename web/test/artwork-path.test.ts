import test from 'node:test';
import assert from 'node:assert/strict';
import {thumbnailArtworkPath} from '../src/ui/artwork-path.ts';
test('cards request bounded artwork without losing the selection revision or portrait subject', () => {
  assert.equal(thumbnailArtworkPath('/v1/items/movie/art/poster'), '/v1/items/movie/art/poster?size=thumbnail');
  assert.equal(thumbnailArtworkPath('/v1/metadata/show/show/art/portrait?subject=tmdb%3A123&v=abc'), '/v1/metadata/show/show/art/portrait?subject=tmdb%3A123&v=abc&size=thumbnail');
  assert.equal(thumbnailArtworkPath('/v1/items/a/art/poster?size=thumbnail'), '/v1/items/a/art/poster?size=thumbnail');
  assert.equal(thumbnailArtworkPath('/v1/metadata/item/a/art/poster?candidate=b'), '/v1/metadata/item/a/art/poster?candidate=b');
  assert.equal(thumbnailArtworkPath('/assets/cover.png'), '/assets/cover.png');
  assert.equal(thumbnailArtworkPath(undefined), undefined);
});
