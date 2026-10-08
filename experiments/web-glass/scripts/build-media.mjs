// Builds the playable fixture media under public/media:
//  - two real films (Blender Studio, CC BY) as H.264/AAC MP4 with faststart
//  - scrub-preview sprite sheets + a JSON index for each
//  - four synthesized ambient music beds (ffmpeg aevalsrc), used by the music player
//  - narrated openings of each public-domain audiobook (macOS `say`)
//
// Inputs are downloaded into .scratch/video by the first step when missing.
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import fs from 'node:fs/promises';
import { existsSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const run = promisify(execFile);
const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const SRC = path.join(ROOT, '.scratch/video');
const OUT = path.join(ROOT, 'public/media');
await fs.mkdir(SRC, { recursive: true });
await fs.mkdir(path.join(OUT, 'books'), { recursive: true });
await fs.mkdir(path.join(OUT, 'music'), { recursive: true });
const log = (...a) => console.log('[media]', ...a);
const ff = (args) => run('ffmpeg', ['-y', '-hide_banner', '-loglevel', 'error', ...args], { maxBuffer: 1 << 26 });

async function download(url, dest) {
  if (existsSync(dest)) return;
  const res = await fetch(url, { headers: { 'User-Agent': 'PorticoFixtureBuilder/1.0 (offline fixture media build)' } });
  if (!res.ok) throw new Error(`${url}: ${res.status}`);
  await fs.writeFile(dest, Buffer.from(await res.arrayBuffer()));
}

// ---- films ----
const films = [
  {
    id: 'coffee-run',
    url: 'https://upload.wikimedia.org/wikipedia/commons/transcoded/3/3f/Coffee_Run_-_Blender_Open_Movie-full_movie.webm/Coffee_Run_-_Blender_Open_Movie-full_movie.webm.1080p.vp9.webm',
    src: 'coffee-run-1080.webm',
    width: 1152,
    crf: 27,
  },
  { id: 'sintel-trailer', url: 'https://download.blender.org/durian/trailer/sintel_trailer-720p.mp4', src: 'sintel-trailer-720p.mp4', width: 1280, crf: 25 },
];

for (const f of films) {
  const src = path.join(SRC, f.src);
  await download(f.url, src);
  const mp4 = path.join(OUT, `${f.id}.mp4`);
  if (!existsSync(mp4)) {
    await ff(['-i', src, '-vf', `scale=${f.width}:-2`, '-c:v', 'libx264', '-preset', 'slower', '-crf', String(f.crf), '-profile:v', 'high', '-pix_fmt', 'yuv420p', '-c:a', 'aac', '-b:a', '96k', '-movflags', '+faststart', mp4]);
  }
  // scrub sprites: one frame every 2 s, 10 columns
  const { stdout } = await run('ffprobe', ['-v', 'error', '-show_entries', 'format=duration', '-of', 'csv=p=0', mp4]);
  const duration = parseFloat(stdout);
  const every = 2;
  const count = Math.ceil(duration / every);
  const cols = 10;
  const rows = Math.ceil(count / cols);
  const sprite = path.join(OUT, `${f.id}-scrub.jpg`);
  const { stdout: dims } = await run('ffprobe', ['-v', 'error', '-select_streams', 'v:0', '-show_entries', 'stream=width,height', '-of', 'csv=p=0', mp4]);
  const [vw, vh] = dims.trim().split(',').map(Number);
  const tw = 240;
  const th = Math.round((vh / vw) * tw / 2) * 2;
  if (!existsSync(sprite)) {
    await ff(['-i', mp4, '-vf', `fps=1/${every},scale=${tw}:${th},tile=${cols}x${rows}`, '-frames:v', '1', '-q:v', '5', sprite]);
  }
  await fs.writeFile(path.join(OUT, `${f.id}-scrub.json`), JSON.stringify({ every, cols, rows, count, width: tw, height: th, duration }) + '\n');
  log(f.id, duration.toFixed(1), 's', `${vw}x${vh}`);
}

// ---- ambient music beds ----
// Each bed crossfades between two voicings every 16 s, with a slow shimmer and a soft echo.
const hz = (n) => 440 * Math.pow(2, (n - 69) / 12);
const beds = {
  'ambient-a': [[57, 64, 69, 72, 76], [53, 60, 65, 69, 72]], // Am9 → Fmaj7
  'ambient-b': [[50, 57, 62, 66, 69], [55, 62, 67, 71, 74]], // D → G
  'ambient-c': [[52, 59, 64, 67, 71], [48, 55, 60, 64, 67]], // Em7 → C
  'ambient-d': [[45, 52, 57, 61, 64], [50, 57, 62, 65, 69]], // A → Dm
};
for (const [id, [a, b]] of Object.entries(beds)) {
  const out = path.join(OUT, 'music', `${id}.m4a`);
  if (existsSync(out)) continue;
  const voice = (notes, env, phase) =>
    notes
      .map((n, i) => `${(0.11 / (1 + i * 0.35)).toFixed(3)}*sin(2*PI*${hz(n).toFixed(3)}*t+${i})*(0.75+0.25*sin(2*PI*${(0.07 + i * 0.013).toFixed(3)}*t+${phase + i}))`)
      .map((x) => `${x}*${env}`)
      .join('+');
  const envA = '(0.5+0.5*cos(2*PI*t/32))';
  const envB = '(0.5-0.5*cos(2*PI*t/32))';
  const shimmer = `0.018*sin(2*PI*${hz(a[4] + 12).toFixed(2)}*t)*(0.5+0.5*sin(2*PI*0.21*t))`;
  const expr = `${voice(a, envA, 0)}+${voice(b, envB, 2)}+${shimmer}`;
  await ff([
    '-f', 'lavfi', '-i', `aevalsrc=${expr}|${expr}:s=44100:d=128`,
    '-af', 'aecho=0.8:0.6:180|340:0.35|0.22,lowpass=f=5200,afade=t=in:d=3,afade=t=out:st=124:d=4,volume=1.6',
    '-c:a', 'aac', '-b:a', '112k', out,
  ]);
  log('bed', id);
}

// ---- audiobook openings ----
const { audiobookDefs } = await import(pathToFileURL(path.join(ROOT, 'src/data/fixtures/audiobooks.ts')).href);
const VOICES = { Kate: 'Shelley (English (UK))', Daniel: 'Daniel', Alex: 'Reed (English (US))', Samantha: 'Samantha (English (US))' };
for (const b of audiobookDefs) {
  const out = path.join(OUT, 'books', `${b.id}.m4a`);
  if (existsSync(out)) continue;
  const aiff = path.join(SRC, `${b.id}.aiff`);
  await run('say', ['-v', VOICES[b.voice] ?? 'Daniel', '-r', '168', '-o', aiff, b.opening]);
  await ff(['-i', aiff, '-af', 'highpass=f=70,acompressor=threshold=-18dB:ratio=2.5,loudnorm=I=-18', '-ac', '1', '-c:a', 'aac', '-b:a', '48k', out]);
  log('book', b.id);
}
