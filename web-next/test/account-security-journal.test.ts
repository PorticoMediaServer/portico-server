import assert from 'node:assert/strict';
import test from 'node:test';
import {AccountSecurityJournal} from '../src/platform/account-security-journal.ts';
import {clone, hostedFixture, iso} from './support/account-security-journal-fixture.ts';

// ACC-LEAF-26: predecessor, immutable-original and CAS boundaries, authored UNRUN.
// Each held vector pauses at the real seam it targets (before the updater, before a
// read, or after the updater but before the final commit) and mutates a caller-owned
// clone; records the class returns are frozen.

function harness(initial?: unknown) {
  let value: unknown = initial === undefined ? undefined : clone(initial);
  let writes = 0;
  let gateUpdate: (() => void) | undefined, gateRead: (() => void) | undefined, gateCommit: (() => void) | undefined;
  let holdUpdate = false, holdRead = false, holdCommit = false;
  const store = {
    read: async () => {
      if (holdRead) await new Promise<void>(resolve => {gateRead = resolve;});
      return value;
    },
    change: async (update: (current: unknown) => unknown, beforeCommit?: () => void) => {
      if (holdUpdate) await new Promise<void>(resolve => {gateUpdate = resolve;});
      const next = update(value);
      if (holdCommit) await new Promise<void>(resolve => {gateCommit = resolve;});
      beforeCommit?.();
      value = next; writes++;
    },
    assertAvailable: () => {},
  };
  return {
    journal: new AccountSecurityJournal(store),
    get value() {return value;},
    get writes() {return writes;},
    holdBeforeUpdate: () => {holdUpdate = true;},
    holdBeforeRead: () => {holdRead = true;},
    holdBeforeCommit: () => {holdCommit = true;},
    releaseUpdate: () => {gateUpdate?.();},
    releaseRead: () => {gateRead?.();},
    releaseCommit: () => {gateCommit?.();},
  };
}

test('retireAccount removes only the identical original and never successor B', async () => {
  const f = hostedFixture();
  const h = harness(f.pending);
  await h.journal.retireAccount(f.original);
  assert.equal(h.value, undefined);
  const b = harness({original: f.successor});
  await b.journal.retireAccount(f.original);
  assert.deepEqual(b.value, {original: f.successor});
});

test('a late clear or replace of A preserves resident successor B', async () => {
  const f = hostedFixture();
  const h = harness({original: f.successor});
  await h.journal.clear(clone(f.pending));
  assert.deepEqual(h.value, {original: f.successor});
  await assert.rejects(h.journal.replace(clone(f.pending), clone(f.enriched), () => {}));
  assert.deepEqual(h.value, {original: f.successor});
});

test('same-original enrichment is accepted and returns a frozen record', async () => {
  const f = hostedFixture();
  const h = harness(f.pending);
  const out = await h.journal.replace(clone(f.pending), clone(f.enriched), () => {});
  assert.equal(Object.isFrozen(out), true);
  assert.deepEqual(h.value, f.enriched);
});

test('same-opening purpose, target, provider or family mutation is refused', async () => {
  const f = hostedFixture();
  const email = clone(f.pending);
  email.original.intent = {purpose: 'email_change', target: 'old@example.test', operationId: 'operation_A_0000000000'};
  const purpose = clone(email);
  purpose.original.intent = {purpose: 'mfa_disable', target: '', operationId: 'operation_A_0000000000'};
  const target = clone(email);
  target.original.intent = {...target.original.intent, target: 'new@example.test'};

  const second = {id: 'identity_second', provider: 'apple' as const, revision: '2'};
  const providerBase = clone(f.pending);
  providerBase.original.state.providers = [...providerBase.original.state.providers, second];
  const provider = clone(providerBase);
  provider.original.provider = second;

  // The original predecessor stays untouched; only the successor moves family,
  // through session, state and both coordinator families together.
  const family = clone(f.pending);
  family.original.session.familyId = 'family_other';
  family.original.state.familyId = 'family_other';
  const coordinator = JSON.parse(family.original.coordinator);
  coordinator.active.session.familyId = 'family_other';
  coordinator.active.authority.familyId = 'family_other';
  family.original.coordinator = JSON.stringify(coordinator);

  for (const [previous, next] of [[email, purpose], [email, target], [providerBase, provider], [f.pending, family]] as const) {
    const h = harness(previous);
    await assert.rejects(h.journal.replace(clone(previous), clone(next), () => {}), JSON.stringify(next.original.intent));
    assert.deepEqual(h.value, previous);
  }
});

test('a caller clone mutated while the updater is held cannot clear or replace resident B', async () => {
  const f = hostedFixture();
  for (const operation of ['clear', 'replace'] as const) {
    const h = harness({original: f.successor});
    const supplied = clone(f.pending);
    h.holdBeforeUpdate();
    const running = operation === 'clear' ? h.journal.clear(supplied)
      : h.journal.replace(clone(f.pending), clone(f.enriched), () => {});
    await Promise.resolve();
    supplied.original = clone(f.successor);
    h.releaseUpdate();
    if (operation === 'clear') await running; else await assert.rejects(running);
    assert.deepEqual(h.value, {original: f.successor}, operation);
  }
});

test('a caller clone mutated while the read is held cannot satisfy assertOriginal', async () => {
  const f = hostedFixture();
  const h = harness({original: f.successor});
  const expected = clone(f.pending);
  h.holdBeforeRead();
  const running = h.journal.assertOriginal(expected);
  await Promise.resolve();
  expected.original = clone(f.successor);
  h.releaseRead();
  await assert.rejects(running);
  assert.deepEqual(h.value, {original: f.successor});
});

test('a getter-backed clone is copied before the hold, so reentry cannot retarget it', async () => {
  const f = hostedFixture();
  let backing = f.original.openingId;
  const supplied = clone(f.pending);
  Object.defineProperty(supplied.original as Record<string, unknown>, 'openingId', {get: () => backing, enumerable: true});
  const h = harness({original: f.successor});
  h.holdBeforeUpdate();
  const running = h.journal.clear(supplied as unknown as Parameters<AccountSecurityJournal['clear']>[0]);
  await Promise.resolve();
  backing = f.successor.openingId;
  h.releaseUpdate();
  await running;
  assert.deepEqual(h.value, {original: f.successor});
});

test('retiring the mounted lifetime at the final commit refuses and keeps the prior record', async () => {
  const f = hostedFixture();
  const h = harness(f.pending);
  let live = true;
  h.holdBeforeCommit();
  const running = h.journal.create(clone(f.enriched), () => {if (!live) throw new Error('The original continuation lifetime ended.');});
  await Promise.resolve();
  live = false;
  h.releaseCommit();
  await assert.rejects(running, /lifetime ended/);
  assert.deepEqual(h.value, f.pending);
});

test('expired A may be cleaned up exactly while an ordinary read refuses it', async () => {
  const f = hostedFixture();
  const expired = clone(f.pending);
  expired.original.createdAt = iso(-300000);
  expired.original.expiresAt = iso(-1000);
  const h = harness(expired);
  await assert.rejects(h.journal.read());
  await h.journal.clear(clone(expired));
  assert.equal(h.value, undefined);
});
