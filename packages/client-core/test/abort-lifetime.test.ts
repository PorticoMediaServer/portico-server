import test from 'node:test';
import assert from 'node:assert/strict';
import {linkAbortSignals, throwIfAborted} from '../src/abort-lifetime.ts';

function tracked() {
  const controller = new AbortController(), signal = controller.signal, listeners = new Set<unknown>();
  const add = signal.addEventListener.bind(signal), remove = signal.removeEventListener.bind(signal);
  Object.defineProperty(signal, 'addEventListener', {configurable: true, value: (type: string, listener: EventListenerOrEventListenerObject, options?: AddEventListenerOptions) => {
    if (type === 'abort') listeners.add(listener); add(type, listener, options);
  }});
  Object.defineProperty(signal, 'removeEventListener', {configurable: true, value: (type: string, listener: EventListenerOrEventListenerObject, options?: EventListenerOptions) => {
    if (type === 'abort') listeners.delete(listener); remove(type, listener, options);
  }});
  return {controller, signal, listeners};
}

test('native-compatible composition links each supplied signal once and detaches all on abort', () => {
  const a = tracked(), b = tracked(), lifetime = linkAbortSignals([a.signal, undefined, b.signal, a.signal]);
  assert.equal(a.listeners.size, 1); assert.equal(b.listeners.size, 1);
  a.controller.abort();
  assert.equal(lifetime.signal.aborted, true);
  assert.equal(a.listeners.size, 0); assert.equal(b.listeners.size, 0);
  lifetime.dispose(); lifetime.dispose();
});

test('pre-aborted input cancels synchronously and leaves no upstream subscriptions', () => {
  const a = tracked(), b = tracked(); b.controller.abort();
  const lifetime = linkAbortSignals([a.signal, b.signal]);
  assert.equal(lifetime.signal.aborted, true);
  assert.equal(a.listeners.size, 0); assert.equal(b.listeners.size, 0);
});

test('ordinary completion detaches listeners without cancelling unrelated owners', () => {
  const a = tracked(), lifetime = linkAbortSignals([a.signal]);
  lifetime.dispose();
  assert.equal(a.listeners.size, 0); assert.equal(a.signal.aborted, false);
  a.controller.abort(); assert.equal(lifetime.signal.aborted, false);
});

test('the original deadline is owned and cleared on both abort and ordinary completion', () => {
  const originalSet = globalThis.setTimeout, originalClear = globalThis.clearTimeout;
  let fire: (() => void) | undefined, deadline: number | undefined, cleared = 0;
  const handle = {} as ReturnType<typeof setTimeout>;
  globalThis.setTimeout = ((callback: () => void, ms: number) => {fire = callback; deadline = ms; return handle;}) as typeof setTimeout;
  globalThis.clearTimeout = ((value: ReturnType<typeof setTimeout>) => {assert.equal(value, handle); cleared++;}) as typeof clearTimeout;
  try {
    const a = tracked(), lifetime = linkAbortSignals([a.signal], 10000);
    assert.equal(deadline, 10000); fire!();
    assert.equal(lifetime.signal.aborted, true); assert.equal(a.listeners.size, 0); assert.equal(cleared, 1);
    lifetime.dispose(); assert.equal(cleared, 1);
    const done = linkAbortSignals([], 15000); assert.equal(deadline, 15000);
    done.dispose(); assert.equal(cleared, 2); assert.equal(done.signal.aborted, false);
    const source = tracked(), cancelled = linkAbortSignals([source.signal], 10000);
    source.controller.abort(); assert.equal(cleared, 3); assert.equal(cancelled.signal.aborted, true);
  } finally {globalThis.setTimeout = originalSet; globalThis.clearTimeout = originalClear;}
});

test('an abort while installing a listener is observed before any work can dispatch', () => {
  const source = tracked(), add = source.signal.addEventListener.bind(source.signal);
  Object.defineProperty(source.signal, 'addEventListener', {configurable: true, value: (type: string, listener: EventListener, options?: AddEventListenerOptions) => {
    add(type, listener, options); source.controller.abort();
  }});
  const lifetime = linkAbortSignals([source.signal]);
  assert.equal(lifetime.signal.aborted, true); assert.equal(source.listeners.size, 0);
});

test('setup failure releases earlier listeners and invalid deadlines attach nothing', () => {
  const source = tracked(), broken = new AbortController().signal;
  Object.defineProperty(broken, 'addEventListener', {value: () => {throw new Error('listener unavailable');}});
  assert.throws(() => linkAbortSignals([source.signal, broken]), /listener unavailable/);
  assert.equal(source.listeners.size, 0);
  for (const timeout of [-1, 1.5, Infinity, NaN, 2147483648]) assert.throws(() => linkAbortSignals([source.signal], timeout), RangeError);
  assert.equal(source.listeners.size, 0);
});

test('portable cancellation needs only aborted and throws an Error-shaped AbortError', () => {
  const controller = new AbortController();
  throwIfAborted(undefined); throwIfAborted(controller.signal);
  controller.abort();
  assert.throws(() => throwIfAborted(controller.signal), (error: unknown) => error instanceof Error && error.name === 'AbortError');
});
