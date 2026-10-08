// Builds web artwork from the Portico Cinema app's licensed art set (experiments/cinema/assets/art):
// every image becomes a WebP under public/art/<category>/<id>.webp at the size the web displays,
// and src/data/fixtures/art.json records each key's dominant tint (from the Cinema manifest).
// Idempotent: existing outputs are kept unless FORCE=1. Needs `cwebp` (libwebp) on PATH.
import {execFileSync} from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const src = path.resolve(root, '../cinema/assets/art');
const out = path.join(root, 'public/art');
const force = process.env.FORCE === '1';
// Longest edge per category; the source set is already close to display size.
const size = {backdrop: 1600, photo: 1600, poster: 480, book: 480, still: 480, homevideo: 640, album: 500, artist: 500, person: 300, photothumb: 360, logo: 400};
const quality = {backdrop: 80, photo: 80};

const keys = [];
let made = 0;
for (const cat of fs.readdirSync(src)) {
  const dir = path.join(src, cat);
  if (!fs.statSync(dir).isDirectory()) continue;
  fs.mkdirSync(path.join(out, cat), {recursive: true});
  for (const file of fs.readdirSync(dir)) {
    if (!/\.(jpe?g|png)$/i.test(file)) continue;
    const id = file.replace(/\.[^.]+$/, '');
    keys.push(`${cat}/${id}`);
    const dest = path.join(out, cat, `${id}.webp`);
    if (!force && fs.existsSync(dest)) continue;
    const args = ['-quiet', '-q', String(quality[cat] ?? 76), '-m', '6', '-metadata', 'none'];
    const [w, h] = execFileSync('sips', ['-g', 'pixelWidth', '-g', 'pixelHeight', path.join(dir, file)]).toString().match(/\d+$/gm).map(Number);
    const max = size[cat] ?? 800;
    if (Math.max(w, h) > max) args.push('-resize', w >= h ? String(max) : '0', w >= h ? '0' : String(max));
    execFileSync('cwebp', [...args, path.join(dir, file), '-o', dest]);
    made++;
  }
}

// Tints come from the Cinema manifest (computed there from each image's dominant colour).
const manifest = fs.readFileSync(path.resolve(root, '../cinema/src/data/fixtures/art.generated.ts'), 'utf8');
const tintsJson = manifest.match(/export const tints[^=]*=\s*(\{[\s\S]*?\});/);
const tints = tintsJson ? JSON.parse(tintsJson[1]) : {};
fs.writeFileSync(path.join(root, 'src/data/fixtures/art.json'), JSON.stringify({keys: keys.sort(), tints}));
console.log(`art: ${keys.length} images (${made} written), ${Object.keys(tints).length} tints`);
