import test from 'node:test';
import assert from 'node:assert/strict';
import {backoffDelay, foregroundRetry, jittered} from '../src/app/foreground-retry.ts';

function fakeEnv(random = 0) {
  const timers: {fn: () => void; ms: number}[] = [];
  let hidden = false, now = 0; const listeners = new Set<() => void>();
  return {
    timers, advance(ms: number) { now += ms; },
    setHidden(h: boolean) { hidden = h; for (const fn of [...listeners]) fn(); },
    fire() { const t = timers.shift(); t?.fn(); return t?.ms; },
    env: {setTimer: (fn: () => void, ms: number) => { const t = {fn, ms}; timers.push(t); return t; }, clearTimer: (t: unknown) => { const i = timers.indexOf(t as never); if (i >= 0) timers.splice(i, 1); }, hidden: () => hidden, onVisibility: (fn: () => void) => { listeners.add(fn); return () => listeners.delete(fn); }, random: () => random, now: () => now},
  };
}

test('a hidden tab runs no timer; a retry due meanwhile runs shortly after it is shown', () => {
  const f = fakeEnv(); let runs = 0;
  const r = foregroundRetry(() => { runs++; }, {}, f.env);
  r.arm(120_000);
  assert.equal(f.timers.length, 1);
  f.setHidden(true);
  assert.equal(f.timers.length, 0, 'hidden: nothing scheduled');
  f.advance(600_000);
  f.setHidden(false);
  assert.equal(f.timers.length, 1);
  assert.ok(f.timers[0]!.ms >= 1000 && f.timers[0]!.ms <= 1500, 'resumes 1 to 1.5 s after being shown');
  f.fire();
  assert.equal(runs, 1);
  assert.equal(r.pending(), false);
});

test('armed while hidden: nothing runs until the tab is shown; a not-yet-due retry keeps its time', () => {
  const f = fakeEnv(); let runs = 0;
  f.setHidden(true);
  const r = foregroundRetry(() => { runs++; }, {}, f.env);
  r.arm(20_000);
  assert.equal(f.timers.length, 0);
  f.advance(5_000);
  f.setHidden(false);
  assert.equal(f.timers[0]!.ms, 15_000);
  r.stop();
  assert.equal(f.timers.length, 0);
  f.setHidden(true); f.setHidden(false);
  assert.equal(f.timers.length, 0, 'stopped: visibility no longer schedules');
  assert.equal(runs, 0);
});

test('never before the Portico Account circuit allows it', () => {
  const f = fakeEnv(); let runs = 0, blocked = 90_000;
  const r = foregroundRetry(() => { runs++; }, {hostedWait: () => blocked}, f.env);
  r.arm(20_000);
  assert.equal(f.timers[0]!.ms, 90_000, 'the circuit’s wait is the floor');
  blocked = 30_000;
  f.fire();
  assert.equal(runs, 0, 'still blocked when it fired: waits again');
  assert.equal(f.timers[0]!.ms, 30_000);
  blocked = 0;
  f.fire();
  assert.equal(runs, 1);
});

test('backoff doubles to a cap with up to 50% jitter', () => {
  assert.equal(backoffDelay(0, 20_000, 300_000, () => 0), 20_000);
  assert.equal(backoffDelay(2, 20_000, 300_000, () => 0), 80_000);
  assert.equal(backoffDelay(9, 20_000, 300_000, () => 0), 300_000);
  assert.equal(backoffDelay(9, 20_000, 300_000, () => 0.999), 449_850);
  assert.equal(jittered(120_000, () => 0.5), 150_000);
});

test('the reconnect and the chooser use it (no bare timers left)', async () => {
  const {readFileSync} = await import('node:fs');
  const src = (p: string) => readFileSync(new URL('../src/' + p, import.meta.url), 'utf8');
  const chooser = src('screens/auth/HostedChooser.tsx');
  assert.doesNotMatch(chooser, /setInterval/);
  assert.match(chooser, /foregroundRetry\([\s\S]*hostedWait: \(\) => gate\.blockedFor\(\)/);
  const session = src('app/session.tsx');
  assert.doesNotMatch(session, /timer = setTimeout\(\(\) => void run\(\)/);
  assert.match(session, /retry\.arm\(jittered\(wait\)\)/);
});
