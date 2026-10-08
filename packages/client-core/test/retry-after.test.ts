import {test} from 'node:test';
import assert from 'node:assert/strict';
import {retryAfterSecondsFrom} from '../src/index.ts';

test('Retry-After accepts delay-seconds', () => {
  assert.equal(retryAfterSecondsFrom('30'), 30);
  assert.equal(retryAfterSecondsFrom('0'), undefined);
  assert.equal(retryAfterSecondsFrom(null), undefined);
  assert.equal(retryAfterSecondsFrom('soon'), undefined);
});

test('Retry-After accepts an HTTP-date', () => {
  const now = Date.parse('Wed, 24 Sep 2026 01:00:00 GMT');
  assert.equal(retryAfterSecondsFrom('Wed, 24 Sep 2026 01:00:45 GMT', now), 45);
  assert.equal(retryAfterSecondsFrom('Wed, 24 Sep 2026 00:59:00 GMT', now), undefined);
});
