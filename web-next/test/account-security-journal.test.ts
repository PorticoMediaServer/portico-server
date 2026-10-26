import assert from 'node:assert/strict';
import test from 'node:test';
import {AccountSecurityJournal} from '../src/platform/account-security-journal.ts';
import {clone, hostedFixture, iso} from './support/account-security-journal-fixture.ts';

// ACC-LEAF-26: AccountSecurityJournal predecessor, immutable-original and CAS
// boundaries, authored UNRUN. Records are real parser-valid DTOs; caller inputs are
// mutable JSON clones, while records the class returns are frozen.

function harness(initial?: unknown) {
  let value: unknown = initial === undefined ? undefined : clone(initial);
  let release: (() => void) | undefined;
  let held = false;
  const writes: number[] = [];
  const store = {
    read: async () => value,
    change: async (update: (current: unknown) => unknown, beforeCommit?: () => void) => {
      const next = update(value);
      if (held) await new Promise<void>(resolve => {release = resolve;});
      beforeCommit?.();
      value = next; writes.push(1);
    },
    assertAvailable: () => {},
  };
  return {journal: new AccountSecurityJournal(store as never), get value() {return value;}, get writes() {return writes.length;},
    hold: () => {held = true;}, release: () => {release?.();}};
}

test('retireAccount removes the identical original and never successor B', async () => {
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
  await h.journal.clear(f.pending);
  assert.deepEqual(h.value, {original: f.successor});
  await assert.rejects(h.journal.replace(f.pending, {original: {...f.original, transactionId: 'oid_' + 'U'.repeat(43)}}, () => {}));
  assert.deepEqual(h.value, {original: f.successor});
});

test('same-original enrichment is accepted and returns a frozen record', async () => {
  const f = hostedFixture();
  const h = harness(f.pending);
  const out = await h.journal.replace(clone(f.pending), clone(f.enriched), () => {});
  assert.equal(Object.isFrozen(out), true);
  assert.deepEqual(h.value, f.enriched);
});

test('same-opening mutation of purpose, target, provider or operation is refused', async () => {
  const f = hostedFixture();
  for (const patch of [
    {purpose: 'email_change'}, {target: 'changed@example.test'},
    {operationId: 'operation_C_0000000000'},
  ]) {
    const h = harness(f.pending);
    const next = clone(f.enriched);
    next.original.intent = {...next.original.intent, ...patch};
    await assert.rejects(h.journal.replace(clone(f.pending), next, () => {}), JSON.stringify(patch));
    assert.deepEqual(h.value, f.pending);
  }
  const h = harness(f.pending);
  const provider = clone(f.enriched);
  provider.original.provider = {...provider.original.provider, id: 'identity_other'};
  await assert.rejects(h.journal.replace(clone(f.pending), provider, () => {}));
});

test('a caller clone mutated while the store is held cannot clear or replace resident B', async () => {
  const f = hostedFixture();
  const h = harness({original: f.successor});
  const supplied = clone(f.pending);
  h.hold();
  const pendingClear = h.journal.clear(supplied);
  await Promise.resolve();
  // The supplied clone is now moved toward B while the lock/read is held.
  supplied.original = clone(f.successor);
  h.release();
  await pendingClear;
  assert.deepEqual(h.value, {original: f.successor});
});

test('expired A may be cleaned up exactly while an ordinary read refuses it', async () => {
  const f = hostedFixture();
  const expired = clone(f.pending);
  expired.original.expiresAt = iso(-1000);
  const h = harness(expired);
  await assert.rejects(h.journal.read());
  await h.journal.clear(clone(expired));
  assert.equal(h.value, undefined);
});
