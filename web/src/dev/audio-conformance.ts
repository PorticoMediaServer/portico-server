/**
 * Dev-only §18.8 conformance run (not part of the app build): decodes every fixture in this
 * browser, through WebCodecs and through decodeAudioData, and reports how many frames each path
 * produced and how the output lines up with the known signal. Open /audio-conformance.html with
 * `vite --config vite.conformance.config.ts`; the report is POSTed to /__audio-report.
 */
import {openDemuxer} from '../bridge/audio/demux';
import {createDecoder} from '../bridge/audio/decoder';
import {TrackStream} from '../bridge/audio/track';
import {Deck, linkDecks, type PcmSource} from '../bridge/audio/engine';
import type {ByteReader, TrackConfig} from '../bridge/audio/types';

type Sidecar = {file: string; container: string; codec: string; sampleRate: number; channels: number; durationFrames: number; rawFrames: number; trim: {startFrames: number; endFrames: number}; signal: {amplitude: number; frequency: number; offset: number}; tolerance: number; edgeFrames: number; edgeTolerance: number};

const sidecars = import.meta.glob('../../../fixtures/audio-gapless/*.json', {eager: true, import: 'default'}) as Record<string, Sidecar | {tracks: unknown}>;
const files = import.meta.glob('../../../fixtures/audio-gapless/*.{mp3,m4a,flac,opus,wav}', {eager: true, query: '?url', import: 'default'}) as Record<string, string>;
const urlOf = (file: string) => Object.entries(files).find(([k]) => k.endsWith('/' + file))?.[1];

export const memoryReader = (bytes: Uint8Array): ByteReader => ({size: bytes.length, read: async (o, n) => bytes.slice(o, Math.min(bytes.length, o + n))});

export function browserName(): string {
  const ua = navigator.userAgent;
  const version = (re: RegExp) => ua.match(re)?.[1] ?? '';
  if (/Edg\//.test(ua)) return 'edge-' + version(/Edg\/(\d+)/);
  if (/Chrome\//.test(ua)) return 'chrome-' + version(/Chrome\/(\d+)/);
  if (/Firefox\//.test(ua)) return 'firefox-' + version(/Firefox\/(\d+)/);
  if (/Safari\//.test(ua)) return 'safari-' + version(/Version\/(\d+(?:\.\d+)?)/);
  return 'unknown';
}

/** Max abs error of `out` against the signal at trimmed index (i + lag), over [from, to). */
function error(out: Float32Array, s: Sidecar, lag: number, from: number, to: number): number {
  let worst = 0;
  const {amplitude: a, frequency: f, offset} = s.signal, rate = s.sampleRate;
  for (let i = Math.max(0, from); i < Math.min(out.length, to); i++) {
    const n = i + lag;
    const want = n < 0 || n >= s.durationFrames ? 0 : a * Math.sin(2 * Math.PI * f * (n + offset) / rate);
    worst = Math.max(worst, Math.abs(out[i]! - want));
  }
  return worst;
}

/** The lag (within one period) that best fits the middle of the output. */
function bestLag(out: Float32Array, s: Sidecar): number {
  const period = Math.ceil(s.sampleRate / s.signal.frequency);
  const mid = Math.floor(out.length / 2);
  let best = 0, bestErr = Infinity;
  for (let lag = 0; lag < period; lag++) {
    const e = error(out, s, lag, mid, mid + 512);
    if (e < bestErr) { bestErr = e; best = lag; }
  }
  return best;
}

function concat(blocks: readonly {channels: readonly Float32Array[]; frames: number}[]): Float32Array {
  const total = blocks.reduce((n, b) => n + b.frames, 0), out = new Float32Array(total);
  let o = 0;
  for (const b of blocks) { out.set(b.channels[0]!.subarray(0, b.frames), o); o += b.frames; }
  return out;
}

async function viaWebCodecs(bytes: Uint8Array, s: Sidecar, variant?: (c: TrackConfig) => TrackConfig) {
  const opened = await openDemuxer(s.container, memoryReader(bytes));
  const demuxer = variant ? {config: variant(opened.config), packets: opened.packets.bind(opened)} : opened;
  const decoder = await createDecoder(demuxer.config);
  if (!decoder) return {supported: false, codec: demuxer.config.codec};
  const blocks = [];
  try {
    for await (const p of demuxer.packets()) { decoder.decode(p); blocks.push(...await decoder.drain(8)); }
    blocks.push(...await decoder.flush());
  } catch (e) { return {supported: true, codec: demuxer.config.codec, failed: String(e)}; } finally { decoder.close(); }
  return {supported: true, codec: demuxer.config.codec, ...measure(concat(blocks), s)};
}

async function viaDecodeAudioData(bytes: Uint8Array, s: Sidecar) {
  try {
    const context = new OfflineAudioContext(s.channels, 1, s.sampleRate);
    const buffer = await context.decodeAudioData(bytes.slice().buffer);
    return {supported: true, rate: buffer.sampleRate, ...measure(buffer.getChannelData(0), s)};
  } catch (e) { return {supported: false, failed: String(e)}; }
}

/** How the output lines up: `dropped` is raw frames the path did not output, and `startOffset`
 * where the track begins in the output when the path dropped only leading frames. */
function measure(out: Float32Array, s: Sidecar) {
  const frames = out.length, dropped = s.rawFrames - frames;
  // Which leading drop (frames the path skipped by itself) makes the track line up.
  const candidates = [...new Set([dropped, 0, 529, 576, 1105, 1024, 2112, 312, s.trim.startFrames, s.rawFrames - s.durationFrames])];
  const skips = candidates.filter(skip => { const r = fit(out, s, skip); return r.pass; });
  return {...fit(out, s, skips[0] ?? dropped), skips};
}

function fit(out: Float32Array, s: Sidecar, skip: number) {
  const frames = out.length, dropped = s.rawFrames - frames;
  const startOffset = s.trim.startFrames - skip;
  const period = s.sampleRate / s.signal.frequency;
  const lag = bestLag(out, s); // out[i] ≈ signal(i + lag) mod period
  const expected = ((-startOffset % period) + period) % period;
  const fits = Math.min(Math.abs(lag - expected), period - Math.abs(lag - expected)) <= 1;
  // With the start-only hypothesis, the trimmed track and its error.
  const track = startOffset >= 0 && startOffset + s.durationFrames <= frames ? out.subarray(startOffset, startOffset + s.durationFrames) : undefined;
  const interior = track ? error(track, s, 0, s.edgeFrames, s.durationFrames - s.edgeFrames) : undefined;
  const edges = track ? Math.max(error(track, s, 0, 0, s.edgeFrames), error(track, s, 0, s.durationFrames - s.edgeFrames, s.durationFrames)) : undefined;
  return {frames, dropped, skip, startOffset, lag, expectedLag: Math.round(expected), fits, interior, edges, pass: !!track && fits && interior! <= s.tolerance && edges! <= s.edgeTolerance};
}

async function throughTrack(bytes: Uint8Array, s: Sidecar, from: number) {
  try {
    const stream = await TrackStream.open({...s, id: s.file}, memoryReader(bytes), from);
    if (!stream) return {supported: false};
    let n = from, interior = 0, edges = 0;
    const {amplitude: a, frequency: f, offset} = s.signal;
    for (;;) {
      const block = await stream.read(8192);
      if (!block) break;
      for (let i = 0; i < block.frames; i++, n++) {
        const e = Math.abs(block.channels[0]![i]! - a * Math.sin(2 * Math.PI * f * (n + offset) / s.sampleRate));
        if (n < s.edgeFrames || n >= s.durationFrames - s.edgeFrames || n < from + 4096) edges = Math.max(edges, e); else interior = Math.max(interior, e);
      }
    }
    stream.close();
    // After a seek the first frames are the decoder settling (sample accuracy matters at boundaries, not seeks).
    return {supported: true, via: stream.via, frames: n, exact: n === s.durationFrames, interior, edges, pass: n === s.durationFrames && interior <= s.tolerance && (from > 0 || edges <= s.edgeTolerance)};
  } catch (e) { return {supported: true, failed: String(e)}; }
}

/** (b) Each album, rendered through the scheduler with gapless links, is one continuous signal. */
async function albumJoins(codec: string) {
  const names = (Object.values(sidecars).find(x => 'tracks' in x) as {tracks: Record<string, string[]>}).tracks[codec]!;
  const tracks = names.map(n => Object.values(sidecars).find((x): x is Sidecar => 'file' in x && x.file.startsWith(n + '.'))!);
  const rate = tracks[0]!.sampleRate, total = tracks.reduce((n, t) => n + t.durationFrames, 0);
  const context = new OfflineAudioContext(2, total + rate, rate);
  const decks: Deck[] = [];
  for (const t of tracks) {
    const bytes = new Uint8Array(await (await fetch(urlOf(t.file)!)).arrayBuffer());
    const stream = await TrackStream.open({...t, id: t.file}, memoryReader(bytes), 0);
    if (!stream) return {supported: false};
    const deck = new Deck(context, context.destination, stream, {frames: t.durationFrames, gain: 1, label: t.file});
    await deck.pump(Infinity);
    decks.push(deck);
  }
  decks[0]!.start(0);
  for (let i = 1; i < decks.length; i++) linkDecks(context, decks[i - 1]!, decks[i]!, {crossfadeSeconds: 6, sameAlbum: true});
  const out = (await context.startRendering()).getChannelData(0);
  const t0 = tracks[0]!, a = t0.signal.amplitude, f = t0.signal.frequency;
  const want = (n: number) => (n < total ? a * Math.sin(2 * Math.PI * f * n / rate) : 0);
  let interior = 0, edges = 0;
  const joins: number[] = [];
  for (let k = 0, at = 0; k < tracks.length; at += tracks[k]!.durationFrames, k++) joins.push(at);
  const nearJoin = (n: number) => joins.some(j => Math.abs(n - j) < t0.edgeFrames) || n >= total - t0.edgeFrames;
  for (let n = 0; n < total; n++) { const e = Math.abs(out[n]! - want(n)); if (nearJoin(n)) edges = Math.max(edges, e); else interior = Math.max(interior, e); }
  // The boundary error in frames: the lag that best fits the audio just after each join.
  const lags = joins.slice(1).map(j => {
    let best = 0, bestErr = Infinity;
    for (let lag = -3; lag <= 3; lag++) { let e = 0; for (let n = j + 64; n < j + 64 + 2048; n++) e += (out[n]! - want(n + lag)) ** 2; if (e < bestErr) { bestErr = e; best = lag; } }
    return best;
  });
  return {supported: true, via: '', total, rendered: out.length, interior, edges, lags, pass: interior <= t0.tolerance && edges <= t0.edgeTolerance && lags.every(l => Math.abs(l) <= 1)};
}

const constant = (frames: number, rate: number, value: number): PcmSource => {
  let at = 0;
  return {sampleRate: rate, channels: 2, async read(max) { if (at >= frames) return undefined; const n = Math.min(max, frames - at); at += n; const c = new Float32Array(n).fill(value); return {channels: [c, c], frames: n}; }, close() {}};
};

/** (c) A 6 s crossfade starts 6 s before the end (± 50 ms), equal-power; (d) a deck's gain applies. */
async function crossfade() {
  const rate = 48000, len = 20 * rate;
  const context = new OfflineAudioContext(2, 41 * rate, rate);
  const a = new Deck(context, context.destination, constant(len, rate, 0.5), {frames: len, gain: 1});
  const b = new Deck(context, context.destination, constant(len, rate, 0), {frames: len, gain: 0.5});
  await a.pump(Infinity); await b.pump(Infinity);
  a.start(0);
  const edge = linkDecks(context, a, b, {crossfadeSeconds: 6, sameAlbum: false});
  const out = (await context.startRendering()).getChannelData(0);
  const fadeStart = out.findIndex(v => v < 0.5 - 1e-6) / rate;
  const mid = out[Math.round(17 * rate)]!; // halfway: 0.5 × cos(π/4)
  const gained = new OfflineAudioContext(2, rate, rate);
  const g = new Deck(gained, gained.destination, constant(rate, rate, 0.5), {frames: rate, gain: 0.5});
  await g.pump(Infinity); g.start(0);
  const level = (await gained.startRendering()).getChannelData(0)[1000]!;
  return {edgeSeconds: edge.start / rate, fadeStart, mid, level, pass: Math.abs(fadeStart - 14) <= 0.05 && Math.abs(mid - 0.5 * Math.SQRT1_2) < 0.01 && Math.abs(level - 0.25) < 1e-6};
}

async function run() {
  const report: Record<string, unknown> = {browser: browserName(), userAgent: navigator.userAgent, webCodecs: 'AudioDecoder' in globalThis, results: {}};
  const out = document.getElementById('out')!;
  const singles = Object.values(sidecars).filter((s): s is Sidecar => 'file' in s);
  singles.sort((a, b) => a.file.localeCompare(b.file));
  for (const s of singles) {
    const url = urlOf(s.file);
    if (!url) continue;
    const bytes = new Uint8Array(await (await fetch(url)).arrayBuffer());
    const row: Record<string, unknown> = {webcodecs: await viaWebCodecs(bytes, s), decodeAudioData: await viaDecodeAudioData(bytes, s)};
    if (s.codec === 'flac' && s.file.startsWith('single-')) {
      // FLAC descriptions differ by browser: STREAMINFO alone, or with its block header.
      row.flacStreamInfoOnly = await viaWebCodecs(bytes, s, c => ({...c, description: c.description!.subarray(8)}));
      row.flacBlockNoMarker = await viaWebCodecs(bytes, s, c => ({...c, description: c.description!.subarray(4)}));
      row.flacNoDescription = await viaWebCodecs(bytes, s, c => ({...c, description: undefined}));
      row.flacDfLa = await viaWebCodecs(bytes, s, c => { const d = new Uint8Array(4 + 38); d.set(c.description!.subarray(4), 4); return {...c, description: d}; });
    }
    (report.results as Record<string, unknown>)[s.file] = row;
    out.textContent = JSON.stringify(report, null, 1);
  }
  // Exit test (a) through the engine's own path: exactly durationFrames, within tolerance, from
  // the start and after a seek. (b): each album joins sample-exactly with the next.
  const tracks: Record<string, unknown> = {};
  for (const s of singles) {
    const url = urlOf(s.file);
    if (!url) continue;
    const bytes = new Uint8Array(await (await fetch(url)).arrayBuffer());
    tracks[s.file] = {start: await throughTrack(bytes, s, 0), seek: await throughTrack(bytes, s, 12345)};
    report.tracks = tracks;
    out.textContent = JSON.stringify(report, null, 1);
  }
  report.engine = {albumFlac: await albumJoins('flac'), albumMp3: await albumJoins('mp3'), albumAac: await albumJoins('aac'), crossfade: await crossfade()};
  report.done = true;
  out.textContent = JSON.stringify(report, null, 1);
  await fetch('/__audio-report', {method: 'POST', body: JSON.stringify(report)}).catch(() => {});
}
const token = fetch('/__audio-run', {cache: 'no-store'}).then(r => r.text()).catch(() => '');
void run().finally(() => {
  setInterval(() => { void fetch('/__audio-run', {cache: 'no-store'}).then(r => r.text()).then(async now => { if (now !== await token) location.reload(); }).catch(() => {}); }, 2000);
});
