/** Offline two-user rehearsal of the real observation/reliable-command wire. Never a capacity run. */
import assert from 'node:assert/strict';
import {setTimeout as delay} from 'node:timers/promises';
import {SessionsClient} from '../../packages/client-core/src/playback-v1/sessions.ts';
import {ObservationReporter} from '../../packages/client-core/src/playback-v1/observation.ts';
import {personalBatchBody} from '../../packages/client-core/src/personal-saved.ts';
import {options} from './load.ts';

export async function selfCheck(o) {
  const sent = [], reporters = [], clients = [];
  const original = o.execute;
  assert.equal(original, false);
  for (let i = 0; i < 2; i++) {
    const http = {async send(r) {
      sent.push({i, ...r});
      if (r.method === 'POST') return {status: 200, headers: {}, body: {epoch: 'abcdefghijklmnopqrstuv', everyMs: 1000}};
      if (r.method === 'PUT') return {status: 200, headers: {}, body: {acceptedSeq: r.body.seq}};
      return {status: 204, headers: {}};
    }};
    const c = new SessionsClient(http); clients.push(c);
    const reporter = new ObservationReporter({everyMs: 1000,
      claim: b => c.claimObserver('session' + i, b), send: b => c.observe('session' + i, b)});
    reporter.setGeneration(1);
    reporter.observe({generation: 1, state: 'playing', positionMs: 0, rate: 1});
    reporters.push(reporter);
  }
  await delay(3200);
  reporters.forEach(r => r.stop());
  await Promise.all(clients.map((c, i) => c.stop('session' + i, 3000)));
  const observations = sent.filter(r => r.method === 'PUT');
  assert.equal(sent.filter(r => r.method === 'POST').length, 2);
  assert.equal(sent.filter(r => r.method === 'DELETE').length, 2);
  assert.ok(observations.length >= 6);
  for (const r of observations) {
    assert.equal(r.singleAttempt, true); assert.equal(r.deadlineMs, 1500);
    assert.ok(r.body.monotonicMs >= r.body.playClockMs);
    assert.ok(r.body.seq >= 1);
  }
  const body = personalBatchBody('offline-selfcheck', [{itemId: 'known-item', watchlisted: true}]);
  assert.equal(body.items[0].watchlisted, true);
  assert.throws(() => personalBatchBody('x', []));
  const before = sent.length; await delay(800); assert.equal(sent.length, before);
  assert.throws(() => options(['--execute', '--self-check']));
  return {kind: 'offline-request-boundary-self-check', users: 2, seconds: 4, passed: true,
    observationRequests: observations.length, cleanupCommands: 2, networkRequests: 0,
    scope: 'real client-core observer and reliable Stop, Saved validation; full workload and capacity remain unrun'};
}
