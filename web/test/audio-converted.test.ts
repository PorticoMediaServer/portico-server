import test from 'node:test';
import assert from 'node:assert/strict';
import {ConvertedSource} from '../src/bridge/audio/converted.ts';
import {encodeFlac} from './helpers/flac-writer.ts';

const rate = 44100, total = rate * 3;
const sig = (n: number) => 0.5 * Math.sin(2 * Math.PI * 441 * n / rate);
/** The server's converted route (§18.1): FLAC from `fromFrame`, trim {0, 0}, cut at `budget` bytes for frame 0. */
function server(budget?: number) {
  const requests: string[] = [];
  const fetch = async (url: string) => {
    requests.push(url);
    const from = Number(new URL(url, 'http://x').searchParams.get('fromFrame') ?? 0);
    const l = new Float32Array(total - from), r = new Float32Array(total - from);
    for (let i = 0; i < l.length; i++) l[i] = r[i] = sig(from + i);
    let bytes = encodeFlac(l, r, rate);
    if (budget !== undefined && from === 0) bytes = bytes.slice(0, budget);
    return new Response(new ReadableStream({start(c) { for (let o = 0; o < bytes.length; o += 7000) c.enqueue(bytes.slice(o, o + 7000)); c.close(); }}), {headers: {'X-Audio-Trim-Start': '0', 'X-Audio-Trim-End': '0'}});
  };
  return {requests, fetch};
}
const plan = {id: 'm1', url: '/v1/media/g/audio-converted', container: 'flac', codec: 'flac', sampleRate: rate, channels: 2, durationFrames: total, trim: {startFrames: 0, endFrames: 0}};

async function drain(source: ConvertedSource) {
  let n = 0, worst = 0;
  for (;;) { const b = await source.read(4096); if (!b) break; for (let i = 0; i < b.frames; i++, n++) worst = Math.max(worst, Math.abs(b.channels[0]![i]! - sig(n))); }
  return {n, worst};
}

test('(g) a converted FLAC stream plays exactly durationFrames', async () => {
  const s = server();
  const {n, worst} = await drain(new ConvertedSource(plan, 0, {locked: false, fetch: s.fetch}));
  assert.equal(n, total);
  assert.ok(worst < 1e-4);
  assert.deepEqual(s.requests, ['/v1/media/g/audio-converted']);
});

test('(g) a prepared stream cut at its budget waits for the commit, then continues from exactly the next frame', async () => {
  const s = server(60_000);
  const source = new ConvertedSource(plan, 0, {locked: true, fetch: s.fetch});
  let n = 0;
  // Before commit: only what the budget allowed.
  for (;;) {
    const read = source.read(4096);
    const r = await Promise.race([read, new Promise<'waiting'>(res => setTimeout(() => res('waiting'), 50))]);
    if (r === 'waiting') { source.unlock(); const b = await read; if (b) n += b.frames; break; }
    if (!r) break;
    n += r.frames;
  }
  assert.equal(s.requests.length, 2, 'one reopen after the commit');
  assert.match(s.requests[1]!, /\?fromFrame=\d+$/);
  const resumedAt = Number(s.requests[1]!.split('=')[1]);
  assert.ok(resumedAt > 0 && resumedAt < total);
  const rest = await drain(source);
  assert.equal(n + rest.n, total, 'every frame once');
});

test('(g) a seek opens the converted stream at ?fromFrame', async () => {
  const s = server();
  const source = new ConvertedSource(plan, 50_000, {locked: false, fetch: s.fetch});
  const b = await source.read(10);
  assert.ok(Math.abs(b!.channels[0]![0]! - sig(50_000)) < 1e-4);
  assert.deepEqual(s.requests, ['/v1/media/g/audio-converted?fromFrame=50000']);
  source.close();
});
