import test from 'node:test';
import assert from 'node:assert/strict';
import {bucketIndexFor} from '../src/browse-buckets.ts';
import type {BrowsePositionAnchor} from '../src/browse.ts';

function anchors(rows: ReadonlyArray<readonly [string, number]>): BrowsePositionAnchor[] {
  return rows.map(([key, index]) => ({key, index}));
}

test('W1: bucket match finds the year anchor', () => {
  const index = anchors([['1990', 0], ['1995', 480], ['1998', 540]]);
  assert.equal(bucketIndexFor(index, 'year', 1995), 480);
  assert.equal(bucketIndexFor(index, 'year', '1995'), 480);
  assert.equal(bucketIndexFor(index, 'year', 1996), undefined);
});

test('W1: bucket match finds the decade anchor when years collapse', () => {
  const index = anchors([['1980s', 0], ['1990s', 400], ['2000s', 700]]);
  assert.equal(bucketIndexFor(index, 'year', 1995), 400);
  assert.equal(bucketIndexFor(index, 'year', '1990s'), 400);
  assert.equal(bucketIndexFor(index, 'year', 2003), 700);
  assert.equal(bucketIndexFor(index, 'year', 1975), undefined);
});

test('W1: bucket match finds the rating anchor', () => {
  const index = anchors([['6', 0], ['8', 300], ['9', 500]]);
  assert.equal(bucketIndexFor(index, 'communityRating', 8.3), 300);
  assert.equal(bucketIndexFor(index, 'communityRating', '8'), 300);
  assert.equal(bucketIndexFor(index, 'personalRating', 9), 500);
  assert.equal(bucketIndexFor(index, 'rating', 7), undefined);
});

test('W1: bucket match finds the duration anchor', () => {
  const index = anchors([['30', 0], ['90', 400], ['120', 700]]);
  assert.equal(bucketIndexFor(index, 'duration', 90), 400);
  assert.equal(bucketIndexFor(index, 'duration', 5400), 400);
  assert.equal(bucketIndexFor(index, 'duration', 5700), 400);
  assert.equal(bucketIndexFor(index, 'duration', 45), 0);
  assert.equal(bucketIndexFor(index, 'duration', 75), undefined);
});

test('W1: no match returns undefined so the caller clamps', () => {
  const index = anchors([['1995', 480]]);
  assert.equal(bucketIndexFor([], 'year', 1995), undefined);
  assert.equal(bucketIndexFor(index, 'year', undefined), undefined);
  assert.equal(bucketIndexFor(index, 'nonsense', '1995'), 480);
  assert.equal(bucketIndexFor(index, 'nonsense', 'other'), undefined);
});
