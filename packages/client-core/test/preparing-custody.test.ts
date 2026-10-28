import assert from 'node:assert/strict';
import test from 'node:test';
import {PreparingStartCustody, type PreparingStop} from '../src/playback-v1/preparing-custody.ts';

const resource = {id: 'session-1', generation: 3};
const stops = () => {
  const calls: string[] = [];
  const stop: PreparingStop = async (_signal, uninstalled, claim) => {if (uninstalled() && claim()) calls.push('stop');};
  return {calls, stop};
};

test('a bound resource is cleaned up at most once, and only that exact resource', async () => {
  const custody = new PreparingStartCustody(), registration = custody.attach();
  const flight = registration.reserve('key-1', () => true);
  flight.seed();
  const {calls, stop} = stops();
  const cleanup = flight.acquire(resource, () => true, stop);
  assert.ok(cleanup);
  assert.equal(flight.acquire({...resource}, () => true, stop), cleanup, 'the same exact resource returns the same receipt');
  assert.equal(flight.acquire({id: 'session-1', generation: 4}, () => true, stop), undefined);
  assert.equal(flight.acquire({id: 'other', generation: 3}, () => true, stop), undefined);
  await cleanup(); await cleanup();
  assert.deepEqual(calls, ['stop']);
});

test('an installation seen during the flight denies cleanup even after forget', async () => {
  const custody = new PreparingStartCustody(), registration = custody.attach();
  const flight = registration.reserve('key-1', () => true);
  const {calls, stop} = stops();
  const cleanup = flight.acquire(resource, () => true, stop)!;
  assert.equal(registration.remember('session-1'), true);
  registration.forget('session-1');
  await cleanup();
  assert.deepEqual(calls, []);
});

test('a claimed stop refuses a later installation of the same id', async () => {
  const custody = new PreparingStartCustody(), registration = custody.attach();
  const flight = registration.reserve('key-1', () => true);
  let installAnswer: boolean | undefined;
  const cleanup = flight.acquire(resource, () => true, async (_signal, uninstalled, claim) => {
    assert.ok(uninstalled() && claim());
    installAnswer = registration.remember('session-1');
  })!;
  await cleanup();
  assert.equal(installAnswer, false);
});

test('keys are reserved once, invalid resources never bind, and the history fails closed', async () => {
  const custody = new PreparingStartCustody(), registration = custody.attach();
  const flight = registration.reserve('key-1', () => true);
  assert.throws(() => registration.reserve('key-1', () => true), {name: 'AbortError'});
  assert.throws(() => registration.reserve('', () => true), {name: 'AbortError'});
  assert.equal(flight.acquire({id: '', generation: 1}, () => true, async () => {}), undefined);
  assert.equal(flight.acquire({id: 'x', generation: 0}, () => true, async () => {}), undefined);
  for (let i = 0; i < 256; i++) registration.remember('installed-' + i);
  const {calls, stop} = stops();
  const cleanup = flight.acquire(resource, () => true, stop)!;
  registration.remember('one-too-many');
  await cleanup();
  assert.deepEqual(calls, [], 'past 256 installs, cleanup is denied');
});

test('owner or registry authority loss and finished flights refuse cleanup', async () => {
  let owner = true;
  const custody = new PreparingStartCustody(() => owner), registration = custody.attach();
  const flight = registration.reserve('key-1', () => true);
  const {calls, stop} = stops();
  const cleanup = flight.acquire(resource, () => true, stop)!;
  owner = false;
  await cleanup();
  assert.deepEqual(calls, []);
  owner = true;
  const later = registration.reserve('key-2', () => true);
  later.finish();
  assert.equal(later.acquire(resource, () => true, stop), undefined);
  assert.equal(later.held(), false);
  custody.dispose();
  const disposed = registration.reserve('key-3', () => true);
  assert.equal(disposed.current(), false, 'a disposed registry admits no continuation');
  disposed.finish();
});
