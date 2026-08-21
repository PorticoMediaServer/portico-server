import { describe, expect, it } from 'vitest';
import { SearchRequestPool } from './searchRequestPool';

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((done) => { resolve = done; });
  return { promise, resolve };
}

async function flushScheduler() {
  for (let index = 0; index < 8; index += 1) await Promise.resolve();
}

describe('SearchRequestPool', () => {
  it('runs deterministic FIFO work with at most two active requests', async () => {
    const pool = new SearchRequestPool(2);
    const gates = [deferred(), deferred(), deferred(), deferred()];
    const started: number[] = [];
    let active = 0;
    let peak = 0;
    gates.forEach((gate, index) => pool.enqueue(`group-${index}`, async () => {
      started.push(index);
      active += 1;
      peak = Math.max(peak, active);
      await gate.promise;
      active -= 1;
    }));

    await flushScheduler();
    expect(started).toEqual([0, 1]);
    expect(peak).toBe(2);
    gates[0].resolve();
    await gates[0].promise;
    await flushScheduler();
    expect(started).toEqual([0, 1, 2]);
    gates[1].resolve();
    gates[2].resolve();
    await Promise.all([gates[1].promise, gates[2].promise]);
    await flushScheduler();
    expect(started).toEqual([0, 1, 2, 3]);
    expect(peak).toBe(2);
    gates[3].resolve();
  });

  it('drops queued work when a newer search generation replaces it', async () => {
    const pool = new SearchRequestPool(1);
    const active = deferred();
    const started: string[] = [];
    pool.enqueue('old-active', async () => { started.push('old-active'); await active.promise; });
    pool.enqueue('old-queued', async () => { started.push('old-queued'); });
    await flushScheduler();
    pool.clear();
    pool.enqueue('new', async () => { started.push('new'); });
    active.resolve();
    await active.promise;
    await flushScheduler();
    expect(started).toEqual(['old-active', 'new']);
  });

  it('keeps retries in the same FIFO budget and releases rejected tasks', async () => {
    const pool = new SearchRequestPool(2);
    const first = deferred();
    const second = deferred();
    const started: string[] = [];
    pool.enqueue('initial-a', async () => { started.push('initial-a'); await first.promise; });
    pool.enqueue('initial-b', async () => { started.push('initial-b'); await second.promise; });
    pool.enqueue('initial-c', async () => { started.push('initial-c'); throw new Error('group failed'); });
    pool.enqueue('retry-a', async () => { started.push('retry-a'); });

    await flushScheduler();
    expect(started).toEqual(['initial-a', 'initial-b']);
    first.resolve();
    await first.promise;
    await flushScheduler();
    expect(started).toEqual(['initial-a', 'initial-b', 'initial-c', 'retry-a']);
    second.resolve();
  });
});
