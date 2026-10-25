import test from 'node:test';
import assert from 'node:assert/strict';
import {QueueSelectionRegistrations, retireQueuedSelectionRegistration} from '../src/playback-v1/selection-registration.ts';
import {V1PlaybackApi} from '../src/playback-v1/legacy-bridge.ts';
import {FakePlaybackServer, fakeHttp} from '../src/playback-v1/testing/fake-server.ts';
import type {Session} from '../src/playback-v1/types.ts';

// Identity and generation deliberately stay unchanged across registrations.
const session = {id: 'occurrence', presentation: {generation: 7}} as Session;
function fixture() {
  const registrations = new QueueSelectionRegistrations();
  let present = false, forgotten = 0;
  const receipt = () => registrations.capture(session.id, () => !present, () => {forgotten++; registrations.changed(session.id); return true;});
  return {registrations, receipt, register: () => {present = true; registrations.changed(session.id);}, get forgotten() {return forgotten;}};
}

test('known absence is exact: same-id registration and an absent/registered/absent cycle retire the original receipt', async () => {
  const f = fixture(), original = f.receipt(); let stops = 0;
  f.register();
  assert.equal(original.current(), false);
  assert.equal(await retireQueuedSelectionRegistration(session, original, () => {stops++; return true;}, () => true), false);
  assert.equal(stops, 0); assert.equal(f.forgotten, 0);
  const ledger = new QueueSelectionRegistrations();
  const receipt = ledger.capture(session.id, () => true, () => true);
  ledger.changed(session.id); ledger.changed(session.id);
  assert.equal(receipt.current(), false, 'an identical id/generation cannot revive a receipt');
});

test('only explicit true retirement under unchanged registration and ownership forgets the original', async () => {
  for (const result of [false, undefined, 'throw', true] as const) {
    const f = fixture(), receipt = f.receipt();
    const retire = async () => {if (result === 'throw') throw new Error('principal refused'); return result as boolean;};
    assert.equal(await retireQueuedSelectionRegistration(session, receipt, retire, () => true), result === true);
    assert.equal(f.forgotten, result === true ? 1 : 0);
  }
  const f = fixture();
  assert.equal(await retireQueuedSelectionRegistration(session, f.receipt(), undefined, () => true), false);
  assert.equal(await retireQueuedSelectionRegistration(session, undefined, () => true, () => true), false);
  assert.equal(f.forgotten, 0);
});

test('a delayed original-principal answer cannot forget re-registration or later accepted ownership', async () => {
  for (const replacement of ['registration', 'ownership'] as const) {
    const f = fixture(), receipt = f.receipt(); let owned = false, finish!: (retired: boolean) => void;
    const pending = retireQueuedSelectionRegistration(session, receipt, (_session, safe) => {
      assert.equal(safe(), true); return new Promise<boolean>(resolve => {finish = resolve;});
    }, () => !owned);
    if (replacement === 'registration') f.register(); else owned = true;
    finish(true);
    assert.equal(await pending, false); assert.equal(f.forgotten, 0);
  }
});

test('receipt eviction closes old ownership without id ABA revival', () => {
  const ledger = new QueueSelectionRegistrations();
  const original = ledger.capture(session.id, () => true, () => true);
  for (let i = 0; i < 129; i++) ledger.changed(`other-${i}`);
  ledger.capture(session.id, () => true, () => true);
  assert.equal(original.current(), false);
});


test('bridge receipt belongs to the returned adoption, never a later registration of the same object', async () => {
  const api = new V1PlaybackApi(fakeHttp(new FakePlaybackServer()), {timers: {setTimer: () => 0, clearTimer: () => {}}});
  const {session, playback} = await api.start({itemId: 'movie'}, {state: 'playing'}, 'registration-source-000000');
  const first = api.selectionRegistration(session.id, playback)!;
  assert.equal(first.current(), true);
  assert.equal(api.selectionRegistration(session.id), undefined, 'id possession cannot acquire an existing registration');
  const next = api.adopt(session, 120), second = api.selectionRegistration(session.id, next)!;
  assert.equal(first.current(), false); assert.equal(second.current(), true);
  assert.equal(first.forget(() => true), false); assert.equal(second.current(), true);
  api.forget(session.id);
});
