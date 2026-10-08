import test from 'node:test';
import assert from 'node:assert/strict';
import {V1PlaybackApi} from '../src/playback-v1/legacy-bridge.ts';

/** PERF-24, spec §6: the timeline is sent every Report-Every-Ms while playing steadily (10 s), and
 * at once on a state change, a new generation, a seek, or the first report. */
function setup() {
  let now = 0;
  const sent: {state: string; positionMs: number}[] = [];
  const timers: {at: number; fn: () => void; id: number}[] = [];
  let ids = 0;
  const http = {send: async (r: {path: string; body?: unknown}) => {
    if (r.path.endsWith('/timeline')) { const b = r.body as {state: string; positionMs: number}; sent.push({state: b.state, positionMs: b.positionMs}); }
    return {status: 204, headers: {'report-every-ms': '10000'}, body: undefined};
  }};
  const api = new V1PlaybackApi(http as never, {now: () => now, timers: {setTimer: (fn, ms) => { const id = ++ids; timers.push({at: now + ms, fn, id}); return id; }, clearTimer: id => { const i = timers.findIndex(t => t.id === id); if (i >= 0) timers.splice(i, 1); }}});
  const advance = async (ms: number) => {
    const end = now + ms;
    for (;;) {
      timers.sort((a, b) => a.at - b.at);
      const next = timers[0];
      if (!next || next.at > end) break;
      timers.shift(); now = next.at; next.fn();
      await new Promise(r => setImmediate(r));
    }
    now = end;
  };
  return {api, sent, advance, at: () => now};
}

test('a minute of steady playback reports every 10 s, not every 3 s', async () => {
  const {api, sent, advance, at} = setup();
  // The service checkpoints every 3 s of position while playing.
  for (let s = 0; s <= 60; s += 3) {
    await api.progressPlayback('ps_1', {generation: 1, sequence: s, positionSeconds: s, state: 'playing'});
    await advance(3000);
  }
  assert.ok(at() >= 63000);
  assert.ok(sent.length >= 6 && sent.length <= 8, `${sent.length} reports in 63 s`);
  assert.ok(sent.every(r => r.state === 'playing'));
  // The timer carries the latest position.
  assert.ok(sent.at(-1)!.positionMs >= 50000);
});

test('pause, a seek and a new generation are sent at once', async () => {
  const {api, sent, advance} = setup();
  await api.progressPlayback('ps_1', {generation: 1, sequence: 1, positionSeconds: 0, state: 'playing'});
  await advance(3000);
  await api.progressPlayback('ps_1', {generation: 1, sequence: 2, positionSeconds: 3, state: 'playing'});
  assert.equal(sent.length, 1, 'steady: carried by the timer');
  await api.progressPlayback('ps_1', {generation: 1, sequence: 3, positionSeconds: 120, state: 'playing'});
  assert.equal(sent.length, 2, 'a seek is reported at once');
  await api.progressPlayback('ps_1', {generation: 1, sequence: 4, positionSeconds: 121, state: 'paused'});
  assert.equal(sent.at(-1)!.state, 'paused', 'a pause is reported at once');
  await api.progressPlayback('ps_1', {generation: 2, sequence: 5, positionSeconds: 121, state: 'playing'});
  assert.equal(sent.length, 4, 'a new generation is reported at once');
  await api.progressPlayback('ps_1', {generation: 2, sequence: 6, positionSeconds: 124, state: 'ended'});
  assert.equal(sent.at(-1)!.state, 'ended');
});
