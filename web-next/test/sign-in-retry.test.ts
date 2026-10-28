import test from 'node:test';
import assert from 'node:assert/strict';
import {signInRetryClock, signInRetryDeadline, signInRetrySeconds} from '../src/kit/accounts/sign-in-retry.ts';

test('password cooldown comes only from a valid server rate-limit response', () => {
  for (const error of [undefined, null, 'rate_limited', {}, {status: 401, retryAfterSeconds: 32}, {status: 503, retryAfterSeconds: 32},
    ...[undefined, 0, -1, NaN, Infinity, '32'].map(retryAfterSeconds => ({status: 429, retryAfterSeconds}))]) {
    assert.equal(signInRetryDeadline(error, 1000), undefined);
  }
  assert.equal(signInRetryDeadline({status: 429, retryAfterSeconds: 32}, 1000), 33000);
  assert.equal(signInRetryDeadline({code: 'rate_limited', retryAfterSeconds: 32}, 1000), 33000);
  assert.equal(signInRetryDeadline({code: 'too_many_requests', retryAfterSeconds: 1.5}, 1000), 2500);
  assert.equal(signInRetryDeadline({status: 429, retryAfterSeconds: Number.MAX_VALUE}, 1000), undefined);
});

test('countdown uses the original deadline after a throttled timer and never enables early', () => {
  const deadline = signInRetryDeadline({status: 429, retryAfterSeconds: 32}, 1000);
  assert.equal(signInRetrySeconds(deadline, 1000), 32);
  assert.equal(signInRetrySeconds(deadline, 32001), 1);
  assert.equal(signInRetrySeconds(deadline, 32999), 1);
  assert.equal(signInRetrySeconds(deadline, 33000), 0);
  assert.equal(signInRetrySeconds(deadline, 80000), 0);
  assert.equal(signInRetrySeconds(undefined, 1000), 0);
});

test('the displayed countdown keeps two second digits and the active locale', () => {
  assert.equal(signInRetryClock(32, 'en-US'), '0:32');
  assert.equal(signInRetryClock(65, 'en-US'), '1:05');
  assert.equal(signInRetryClock(.1, 'en-US'), '0:01');
  assert.equal(signInRetryClock(0, 'en-US'), '0:00');
  assert.equal(signInRetryClock(65, 'ar-EG'), '١:٠٥');
});
