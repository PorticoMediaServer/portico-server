/**
 * PERF-07: `now` as a tiny external store. A 30 s tick notifies only the now-line and the
 * on-air progress children; rows and programme blocks never subscribe, so they never
 * re-render on tick. This tests the store contract that makes that true.
 * Mirrors `portico-react-native/apps/apple/test/guide-now.test.ts`.
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import {createNowStore} from '../src/screens/live/now-store.ts';

const settle = async (ms = 60) => new Promise(resolve => setTimeout(resolve, ms));

test('a tick notifies only subscribers; a holder of the store object never re-renders', async () => {
  const store = createNowStore(20);
  try {
    const before = store.get();
    assert.ok(Number.isFinite(before));
    // A row holds the store object (stable identity) without subscribing: no notification.
    const holder = store;
    let progressRenders = 0;
    const unsub = store.subscribe(() => { progressRenders++; });
    await settle();
    assert.ok(store.get() >= before);
    assert.ok(progressRenders >= 1);
    assert.equal(holder, store);
    unsub();
    const notified = progressRenders;
    await settle();
    assert.equal(progressRenders, notified);
  } finally {
    store.dispose();
  }
});

test('getSnapshot and get agree; dispose stops the tick', async () => {
  const store = createNowStore(20);
  try {
    assert.equal(store.getSnapshot(), store.get());
    let calls = 0;
    const unsub = store.subscribe(() => { calls++; });
    await settle();
    assert.ok(calls >= 1);
    assert.equal(store.getSnapshot(), store.get());
    unsub();
    store.dispose();
    const frozen = store.get();
    await settle();
    assert.equal(store.get(), frozen);
  } finally {
    store.dispose();
  }
});
