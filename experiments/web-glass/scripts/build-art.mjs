// Builds every fixture artwork file under public/art and the palette index at
// src/data/generated/palettes.json.
//
// Sources: Unsplash photography via picsum.photos (seeded), Blender Studio open
// movie posters/stills (CC BY) and public-domain film posters from Wikimedia
// Commons (downloaded once into .scratch/commons by scripts/fetch-commons.py).
// Posters, logos, album and audiobook covers are typeset here with Playwright.
//
// Usage: node scripts/build-art.mjs [--only=posters,logos,...] [--force]
import { chromium } from 'playwright';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import fs from 'node:fs/promises';
import { existsSync, readdirSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const run = promisify(execFile);
const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const OUT = path.join(ROOT, 'public/art');
const CACHE = path.join(ROOT, '.scratch/src/picsum');
const COMMONS = path.join(ROOT, '.scratch/commons');
const WORK = path.join(ROOT, '.scratch/render');
const args = process.argv.slice(2);
const FORCE = args.includes('--force');
const ONLY = (args.find((a) => a.startsWith('--only=')) ?? '').replace('--only=', '').split(',').filter(Boolean);
const want = (k) => ONLY.length === 0 || ONLY.includes(k);

const imp = (p) => import(pathToFileURL(path.join(ROOT, p)).href);
const { movieDefs } = await imp('src/data/fixtures/movies.ts');
const { showDefs } = await imp('src/data/fixtures/shows.ts');
const { albumDefs, artistDefs } = await imp('src/data/fixtures/music.ts');
const { audiobookDefs } = await imp('src/data/fixtures/audiobooks.ts');
const { photoAlbumDefs, homeVideoDefs } = await imp('src/data/fixtures/photos.ts');
const { programSeeds } = await imp('src/data/fixtures/live.ts');

for (const d of ['poster', 'backdrop', 'logo', 'still', 'cover', 'artist', 'photo', 'photo-thumb', 'program']) {
  await fs.mkdir(path.join(OUT, d), { recursive: true });
}
await fs.mkdir(CACHE, { recursive: true });
await fs.mkdir(WORK, { recursive: true });

// ---------- helpers ----------
async function pool(items, n, fn) {
  const q = [...items];
  const workers = Array.from({ length: n }, async () => {
    while (q.length) {
      const it = q.shift();
      await fn(it);
    }
  });
  await Promise.all(workers);
}

async function picsum(seed, w, h) {
  const f = path.join(CACHE, `${seed}-${w}x${h}.jpg`);
  if (existsSync(f)) return f;
  for (let attempt = 0; attempt < 5; attempt++) {
    try {
      const res = await fetch(`https://picsum.photos/seed/pt${seed}/${w}/${h}`);
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
      await fs.writeFile(f, Buffer.from(await res.arrayBuffer()));
      return f;
    } catch (e) {
      await new Promise((r) => setTimeout(r, 800 * (attempt + 1)));
      if (attempt === 4) throw new Error(`picsum ${seed} ${w}x${h}: ${e.message}`);
    }
  }
}

const commonsFile = (key) => {
  const f = readdirSync(COMMONS).find((x) => x.replace(/\.[a-z]+$/, '') === key);
  return f ? path.join(COMMONS, f) : null;
};

async function magick(argv) {
  await run('magick', argv, { maxBuffer: 64 * 1024 * 1024 });
}

const exists = (f) => !FORCE && existsSync(f);

async function toWebp(src, dest, { w, h, q = 78, gravity = 'center', extra = [] }) {
  if (exists(dest)) return;
  const geo = h ? [`-resize`, `${w}x${h}^`, '-gravity', gravity, '-extent', `${w}x${h}`] : ['-resize', `${w}x`];
  await magick([src, '-auto-orient', '-colorspace', 'sRGB', ...geo, ...extra, '-strip', '-quality', String(q), dest]);
}

// ---------- palettes ----------
const palettes = {};
async function paletteOf(file, key) {
  const { stdout } = await run('magick', [file, '-resize', '24x24!', '-depth', '8', 'rgb:-'], { encoding: 'buffer', maxBuffer: 1 << 20 });
  const px = [];
  for (let i = 0; i + 2 < stdout.length; i += 3) px.push([stdout[i], stdout[i + 1], stdout[i + 2]]);
  const lum = ([r, g, b]) => (0.2126 * r + 0.7152 * g + 0.0722 * b) / 255;
  const sat = ([r, g, b]) => {
    const mx = Math.max(r, g, b), mn = Math.min(r, g, b);
    return mx === 0 ? 0 : (mx - mn) / mx;
  };
  const avg = px.reduce((a, p) => [a[0] + p[0], a[1] + p[1], a[2] + p[2]], [0, 0, 0]).map((v) => v / px.length);
  const avgL = lum(avg);
  // base: the average, pushed dark enough to sit behind white text
  const k = Math.min(1, 0.26 / Math.max(avgL, 0.01));
  const base = avg.map((v) => Math.round(v * k));
  // accent: most vivid mid-luminance pixel cluster
  let best = null, bestScore = -1;
  for (const p of px) {
    const L = lum(p);
    const s = sat(p) * (1 - Math.abs(L - 0.55) * 1.4);
    if (s > bestScore) { bestScore = s; best = p; }
  }
  let accent = best ?? avg;
  const aL = lum(accent);
  if (aL < 0.45) accent = accent.map((v) => Math.min(255, Math.round(v * (0.55 / Math.max(aL, 0.05)))));
  const hex = (c) => '#' + c.map((v) => Math.max(0, Math.min(255, Math.round(v))).toString(16).padStart(2, '0')).join('');
  palettes[key] = { base: hex(base), accent: hex(accent), ...(avgL > 0.62 ? { light: true } : {}) };
}

// ---------- poster templates ----------
const credits = (d) => {
  const names = (d.cast ?? []).slice(0, 3).map((c) => c[0].toUpperCase());
  return names;
};

function billing(d) {
  const dir = (d.director ?? (d.creators ?? [])[0] ?? '').toUpperCase();
  const cast = (d.cast ?? []).slice(0, 4).map((c) => c[0].toUpperCase()).join('  ');
  const studio = (d.studio ?? d.network ?? '').toUpperCase();
  return `${studio} PRESENTS  A ${dir.split(' ').pop()} FILM  ${cast}  MUSIC BY ${['ELSA', 'JONAS', 'MARTA', 'KENJI'][dir.length % 4]} ${['HOLM', 'REYES', 'VANCE', 'ODA'][dir.length % 4]}  DIRECTOR OF PHOTOGRAPHY ${['LENA', 'OSCAR', 'PAULO'][dir.length % 3]} ${['BRANDT', 'MOREAU', 'SILVA'][dir.length % 3]}  WRITTEN AND DIRECTED BY ${dir}`;
}

function posterHTML(d, img) {
  const t = d.title;
  const style = d.style ?? 'minimal';
  const top = credits(d);
  const tagline = d.tagline ?? '';
  const year = d.year;
  const bill = billing(d);
  const jp = d.originalTitle ?? '';
  const tint = d.tint ?? null;
  const long = t.length > 18;
  const common = `
    <div class="bill">${bill}</div>`;
  switch (style) {
    case 'didot':
      return `<div class="p didot"><img src="${img}"><div class="shade"></div>
        <div class="names">${top.join('<span>·</span>')}</div>
        <div class="foot">${tagline ? `<div class="tag">${tagline}</div>` : ''}<div class="title ${long ? 'long' : ''}">${t}</div>${common}</div></div>`;
    case 'condensed':
      return `<div class="p condensed"><img src="${img}"><div class="shade"></div>
        <div class="names">${top.join('   ')}</div>
        <div class="foot">${tagline ? `<div class="tag">${tagline}</div>` : ''}<div class="title">${t}</div>${common}</div></div>`;
    case 'minimal':
      return `<div class="p minimal"><img src="${img}"><div class="shade"></div>
        <div class="mid"><div class="title">${t}</div>${tagline ? `<div class="tag">${tagline}</div>` : ''}</div><div class="foot">${common}</div></div>`;
    case 'futura': {
      const bands = ['#ffd640', '#ff8a5c', '#f4f1ea', '#7fe0d0', '#b9a5ff', '#9fd3f0'];
      const band = tint ?? bands[[...d.id].reduce((a, c) => a + c.charCodeAt(0), 0) % bands.length];
      return `<div class="p futura"><img src="${img}"><div class="shade"></div>
        <div class="band" style="background:${band}"><div class="title">${t}</div></div>
        <div class="foot">${tagline ? `<div class="tag">${tagline}</div>` : ''}<div class="names">${top.join(' · ')}</div>${common}</div></div>`;
    }
    case 'thriller':
      return `<div class="p thriller"><img src="${img}"><div class="shade"></div>
        <div class="names">${top.join('   ')}</div>
        <div class="foot">${tagline ? `<div class="tag">${tagline}</div>` : ''}<div class="rule"></div><div class="title">${t}</div>${common}</div></div>`;
    case 'kids':
      return `<div class="p kids"><img src="${img}"><div class="shade"></div>
        <div class="foot">${tagline ? `<div class="tag">${tagline}</div>` : ''}<div class="title">${t}</div>${common}</div></div>`;
    case 'doc':
      return `<div class="p doc"><div class="frame"><img src="${img}"></div>
        <div class="foot"><div class="kicker">A FILM BY ${(d.director ?? d.creators?.[0] ?? '').toUpperCase()}</div><div class="title">${t}</div><div class="laurels"><span>OFFICIAL SELECTION</span><span>${year}</span></div></div></div>`;
    case 'romance':
      return `<div class="p romance"><img src="${img}"><div class="shade"></div>
        <div class="names">${top.join(' &amp; ')}</div>
        <div class="foot">${tagline ? `<div class="tag">${tagline}</div>` : ''}<div class="title">${t}</div>${common}</div></div>`;
    case 'scifi':
      return `<div class="p scifi"><img src="${img}"><div class="shade"></div>
        <div class="tagtop">${tagline}</div>
        <div class="foot"><div class="title">${t}</div><div class="names">${top.join('   ')}</div>${common}</div></div>`;
    case 'western':
      return `<div class="p western"><img src="${img}"><div class="shade"></div>
        <div class="names">${top.join(' · ')}</div>
        <div class="foot">${tagline ? `<div class="tag">${tagline}</div>` : ''}<div class="title">${t}</div>${common}</div></div>`;
    case 'anime':
      return `<div class="p anime"><img src="${img}"><div class="shade"></div>
        <div class="jp">${jp}</div>
        <div class="foot"><div class="title">${t}</div><div class="studio">${(d.network ?? '').toUpperCase()}</div></div></div>`;
    default:
      return `<div class="p minimal"><img src="${img}"><div class="shade"></div><div class="mid"><div class="title">${t}</div></div></div>`;
  }
}

const POSTER_CSS = `
*{box-sizing:border-box;margin:0;padding:0}
html,body{background:transparent}
body{width:480px;height:720px;overflow:hidden;-webkit-font-smoothing:antialiased}
.p{position:relative;width:480px;height:720px;overflow:hidden;background:#0b1520;color:#fff}
.p img{position:absolute;inset:0;width:100%;height:100%;object-fit:cover}
.shade{position:absolute;inset:0}
.names{position:absolute;top:30px;left:0;right:0;text-align:center;font:600 11px/1 "Avenir Next Condensed";letter-spacing:.28em;opacity:.92}
.names span{margin:0 .7em;opacity:.6}
.foot{position:absolute;left:28px;right:28px;bottom:26px;text-align:center}
.bill{margin-top:16px;font:500 7.2px/1.25 "Avenir Next Condensed";letter-spacing:.06em;opacity:.55;text-align:center;text-transform:uppercase}
.tag{font:italic 400 15px/1.3 "Baskerville";opacity:.9;margin-bottom:12px}

.didot .shade{background:linear-gradient(180deg,rgba(0,0,0,.45) 0%,rgba(0,0,0,0) 22%,rgba(0,0,0,0) 48%,rgba(4,10,18,.88) 82%,rgba(4,10,18,.96) 100%)}
.didot .title{font:400 60px/0.98 "Didot";letter-spacing:-.01em}
.didot .title.long{font-size:44px}
.didot img{filter:saturate(.85) contrast(1.05)}

.condensed .shade{background:linear-gradient(180deg,rgba(0,0,0,.5) 0%,rgba(0,0,0,0) 20%,rgba(0,0,0,0) 50%,rgba(0,0,0,.9) 100%)}
.condensed .title{font:800 74px/.86 "Avenir Next Condensed";text-transform:uppercase;letter-spacing:-.01em;text-align:left}
.condensed .foot{text-align:left}
.condensed .tag{font:600 12px/1.2 "Avenir Next Condensed";letter-spacing:.24em;text-transform:uppercase;font-style:normal;color:#9fd3f0}
.condensed .bill{text-align:left}
.condensed img{filter:contrast(1.12) saturate(.9)}

.minimal .shade{background:linear-gradient(180deg,rgba(0,0,0,.2),rgba(0,0,0,.05) 40%,rgba(0,0,0,.55))}
.minimal .mid{position:absolute;left:30px;right:30px;top:62%;text-align:center}
.minimal .title{font:500 25px/1.2 "Futura";letter-spacing:.42em;text-transform:uppercase;padding-left:.42em}
.minimal .tag{margin-top:14px;font:400 13px/1.3 "Avenir Next";letter-spacing:.08em;font-style:normal;opacity:.85}

.futura .shade{background:linear-gradient(180deg,rgba(0,0,0,0) 40%,rgba(0,0,0,.75))}
.futura .band{position:absolute;left:0;right:0;top:56px;padding:18px 28px;background:rgba(255,214,64,.94);color:#141414}
.futura .title{font:700 44px/1 "Futura";letter-spacing:-.01em;text-transform:none}
.futura .names{position:static;margin-top:8px;font-size:10.5px}
.futura .tag{font:500 16px/1.3 "Futura";font-style:normal}

.thriller img{filter:grayscale(.55) contrast(1.25) brightness(.8)}
.thriller .shade{background:linear-gradient(180deg,rgba(0,0,0,.6),rgba(0,0,0,.1) 30%,rgba(0,0,0,.2) 55%,rgba(0,0,0,.95))}
.thriller .title{font:700 46px/1 "Helvetica Neue";letter-spacing:.14em;text-transform:uppercase;padding-left:.14em}
.thriller .rule{width:54px;height:3px;background:#e2393f;margin:0 auto 18px}
.thriller .tag{font:500 13px/1.3 "Helvetica Neue";letter-spacing:.2em;text-transform:uppercase;font-style:normal;opacity:.85}

.kids img{filter:saturate(1.25) brightness(1.05)}
.kids .shade{background:linear-gradient(180deg,rgba(0,0,0,0) 45%,rgba(10,30,80,.7))}
.kids .title{font:800 52px/.95 "Avenir Next";letter-spacing:-.02em;color:#fff;text-shadow:0 3px 0 #f6a623,0 6px 0 rgba(0,0,0,.25)}
.kids .tag{font:700 14px/1.3 "Avenir Next";font-style:normal;background:#ffd23f;color:#1a1a1a;display:inline-block;padding:6px 12px;border-radius:999px}

.doc{background:#efe9dd!important;color:#1b1b1b!important}
.doc .frame{position:absolute;left:26px;right:26px;top:26px;height:500px;overflow:hidden}
.doc .frame img{filter:saturate(.9)}
.doc .foot{bottom:30px;color:#1b1b1b}
.doc .kicker{font:600 10px/1 "Gill Sans";letter-spacing:.3em;opacity:.7;margin-bottom:10px}
.doc .title{font:400 38px/1 "Gill Sans";letter-spacing:.02em}
.doc .laurels{display:flex;justify-content:space-between;margin-top:14px;font:600 9px/1 "Gill Sans";letter-spacing:.3em;opacity:.55}

.romance img{filter:sepia(.25) saturate(1.1) brightness(1.02)}
.romance .shade{background:linear-gradient(180deg,rgba(40,10,20,.35),rgba(0,0,0,0) 35%,rgba(60,20,20,.1) 55%,rgba(30,8,12,.85))}
.romance .title{font:italic 400 54px/1 "Bodoni 72"}
.romance .tag{font:400 13px/1.3 "Avenir Next";letter-spacing:.24em;text-transform:uppercase;font-style:normal}

.scifi img{filter:saturate(1.1) contrast(1.05)}
.scifi .shade{background:linear-gradient(180deg,rgba(2,6,20,.8) 0%,rgba(2,6,20,0) 24%,rgba(2,6,20,0) 55%,rgba(2,6,20,.94) 100%)}
.scifi .tagtop{position:absolute;top:34px;left:0;right:0;text-align:center;font:400 13px/1.3 "Avenir Next";letter-spacing:.34em;text-transform:uppercase;opacity:.9}
.scifi .title{font:300 48px/1 "Futura";letter-spacing:.3em;text-transform:uppercase;padding-left:.3em;text-shadow:0 0 30px rgba(92,179,226,.7)}
.scifi .names{position:static;margin-top:16px;font-size:10px}

.western img{filter:sepia(.55) contrast(1.1) saturate(1.2)}
.western .shade{background:linear-gradient(180deg,rgba(40,20,5,.5),rgba(0,0,0,0) 30%,rgba(30,12,2,.1) 55%,rgba(30,12,2,.9))}
.western .title{font:700 44px/1 "Copperplate";letter-spacing:.06em;text-transform:uppercase;color:#f6e4c3}
.western .tag{font:italic 400 15px/1.3 "Big Caslon"}

.anime img{filter:saturate(1.45) contrast(1.08) brightness(1.04) hue-rotate(-6deg)}
.anime .shade{background:linear-gradient(180deg,rgba(255,120,180,.12),rgba(80,120,255,.08) 45%,rgba(8,12,40,.86))}
.anime .jp{position:absolute;top:34px;right:30px;writing-mode:vertical-rl;font:600 50px/1.05 "Hiragino Mincho ProN";letter-spacing:.08em;text-shadow:0 2px 18px rgba(0,0,0,.45)}
.anime .foot{text-align:left}
.anime .title{font:700 34px/1.02 "Avenir Next";letter-spacing:-.01em}
.anime .studio{margin-top:10px;font:600 10px/1 "Avenir Next";letter-spacing:.3em;opacity:.7}
`;

// Logos: title treatments on transparent backgrounds, used over backdrops.
function logoHTML(d) {
  const style = d.style ?? 'minimal';
  const t = d.title;
  const map = {
    didot: `font:400 96px/1 'Didot'`,
    condensed: `font:800 104px/.9 'Avenir Next Condensed';text-transform:uppercase`,
    minimal: `font:500 54px/1.1 'Futura';letter-spacing:.4em;text-transform:uppercase;padding-left:.4em`,
    futura: `font:700 92px/1 'Futura'`,
    thriller: `font:700 70px/1 'Helvetica Neue';letter-spacing:.16em;text-transform:uppercase;padding-left:.16em`,
    kids: `font:800 92px/.95 'Avenir Next';letter-spacing:-.02em;text-shadow:0 5px 0 #f6a623`,
    doc: `font:400 80px/1 'Gill Sans';letter-spacing:.02em`,
    romance: `font:italic 400 96px/1 'Bodoni 72'`,
    scifi: `font:300 76px/1 'Futura';letter-spacing:.32em;text-transform:uppercase;padding-left:.32em;text-shadow:0 0 40px rgba(92,179,226,.8)`,
    western: `font:700 78px/1 'Copperplate';letter-spacing:.06em;text-transform:uppercase;color:#f6e4c3`,
    anime: `font:700 78px/1 'Avenir Next';letter-spacing:-.01em`,
  };
  const jp = d.originalTitle ? `<div style="font:600 30px/1 'Hiragino Mincho ProN';letter-spacing:.3em;margin-bottom:14px;opacity:.9">${d.originalTitle}</div>` : '';
  return `<div id="logo" style="display:inline-block;max-width:1100px;padding:24px 28px;color:#fff;text-align:left">${jp}<div style="${map[style]};text-wrap:balance">${t}</div></div>`;
}

// ---------- album + book covers ----------
function albumHTML(a, artist, img) {
  const s = a.coverStyle;
  const T = a.title;
  const A = artist.name;
  const base = `<div class="c ${s}"><img src="${img}"><div class="shade"></div>`;
  if (s === 'none') return `${base}</div>`;
  if (s === 'type-top') return `${base}<div class="tl"><div class="ar">${A}</div><div class="ti">${T}</div></div></div>`;
  if (s === 'type-bottom') return `${base}<div class="bl"><div class="ar">${A}</div><div class="ti">${T}</div></div></div>`;
  if (s === 'type-center') return `${base}<div class="cc"><div class="ti">${T}</div><div class="ar">${A}</div></div></div>`;
  if (s === 'classical') {
    const [composer, work] = T.split(': ');
    return `<div class="c classical"><div class="band"><div class="co">${composer.toUpperCase()}</div><div class="wk">${work}</div><div class="ar">${A.toUpperCase()}</div></div><div class="ph"><img src="${img}"></div></div>`;
  }
  if (s === 'kids') return `${base}<div class="kd"><div class="ti">${T}</div><div class="ar">${A}</div></div></div>`;
  return `${base}</div>`;
}

const COVER_CSS = `
*{box-sizing:border-box;margin:0;padding:0}
body{width:600px;height:600px;overflow:hidden;-webkit-font-smoothing:antialiased;background:transparent}
.c{position:relative;width:600px;height:600px;overflow:hidden;color:#fff;background:#111}
.c img{position:absolute;inset:0;width:100%;height:100%;object-fit:cover}
.shade{position:absolute;inset:0}
.type-top .shade{background:linear-gradient(180deg,rgba(0,0,0,.45),rgba(0,0,0,0) 40%)}
.type-bottom .shade{background:linear-gradient(0deg,rgba(0,0,0,.55),rgba(0,0,0,0) 45%)}
.type-center .shade{background:radial-gradient(circle at 50% 50%,rgba(0,0,0,.35),rgba(0,0,0,0) 70%)}
.tl{position:absolute;top:34px;left:36px;right:36px}
.bl{position:absolute;bottom:34px;left:36px;right:36px}
.ar{font:600 15px/1.2 "Helvetica Neue";letter-spacing:.24em;text-transform:uppercase;opacity:.92}
.ti{font:300 42px/1.05 "Helvetica Neue";margin-top:8px;letter-spacing:-.01em}
.cc{position:absolute;inset:0;display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;padding:40px}
.cc .ti{font:500 34px/1.1 "Futura";letter-spacing:.46em;text-transform:uppercase;padding-left:.46em;margin:0}
.cc .ar{margin-top:18px;font:500 13px/1 "Futura";letter-spacing:.5em;opacity:.8}
.classical{background:#f2eee6;color:#1a1a1a}
.classical .band{position:absolute;top:0;left:0;right:0;height:210px;padding:34px 36px;border-bottom:1px solid rgba(0,0,0,.15)}
.classical .co{font:600 30px/1 "Baskerville";letter-spacing:.14em}
.classical .wk{font:italic 400 22px/1.25 "Baskerville";margin-top:12px;color:#5a4a2a}
.classical .ar{position:absolute;bottom:26px;left:36px;font:600 12px/1 "Gill Sans";letter-spacing:.3em;color:#333}
.classical .ph{position:absolute;left:0;right:0;bottom:0;top:210px;overflow:hidden}
.classical .ph img{filter:grayscale(.3) contrast(1.05)}
.kids img{filter:saturate(1.3)}
.kids .shade{background:linear-gradient(0deg,rgba(40,20,90,.55),rgba(0,0,0,0) 55%)}
.kd{position:absolute;left:30px;right:30px;bottom:30px;text-align:center}
.kd .ti{font:800 44px/1 "Avenir Next";text-shadow:0 4px 0 #ff7aa8;margin:0}
.kd .ar{margin-top:14px;font:700 16px/1 "Avenir Next";letter-spacing:.06em;text-transform:none;background:#ffd23f;color:#222;display:inline-block;padding:8px 14px;border-radius:999px}
`;

function bookHTML(b, img) {
  const s = b.coverStyle;
  const T = b.title;
  const au = b.author;
  if (s === 'classic')
    return `<div class="b classic"><div class="ph"><img src="${img}"></div><div class="tx"><div class="orn">✦</div><div class="ti">${T}</div><div class="au">${au.toUpperCase()}</div></div><div class="badge">READ BY ${b.narrator.toUpperCase()}</div></div>`;
  if (s === 'modern')
    return `<div class="b modern"><img src="${img}"><div class="panel"><div class="au">${au.toUpperCase()}</div><div class="ti">${T}</div></div></div>`;
  if (s === 'gothic')
    return `<div class="b gothic"><img src="${img}"><div class="shade"></div><div class="tx"><div class="au">${au.toUpperCase()}</div><div class="rule"></div><div class="ti">${T}</div></div></div>`;
  return `<div class="b kids"><img src="${img}"><div class="shade"></div><div class="tx"><div class="ti">${T}</div><div class="au">${au}</div></div></div>`;
}

const BOOK_CSS = `
*{box-sizing:border-box;margin:0;padding:0}
body{width:600px;height:600px;overflow:hidden;-webkit-font-smoothing:antialiased;background:transparent}
.b{position:relative;width:600px;height:600px;overflow:hidden;color:#fff}
.b img{position:absolute;inset:0;width:100%;height:100%;object-fit:cover}
.classic{background:#16324a}
.classic .ph{position:absolute;top:36px;left:36px;right:36px;height:300px;overflow:hidden;border:1px solid rgba(230,210,160,.5)}
.classic .ph img{filter:sepia(.35) saturate(.8)}
.classic .tx{position:absolute;left:36px;right:36px;top:350px;text-align:center;color:#ecd9a8}
.classic .orn{font:400 16px/1 "Hoefler Text";opacity:.8}
.classic .ti{font:400 40px/1.05 "Hoefler Text";margin-top:10px}
.classic .au{margin-top:14px;font:600 13px/1 "Gill Sans";letter-spacing:.32em;opacity:.85}
.classic .badge{position:absolute;bottom:26px;left:0;right:0;text-align:center;font:600 9px/1 "Gill Sans";letter-spacing:.3em;color:#ecd9a8;opacity:.6}
.modern .panel{position:absolute;left:0;right:0;bottom:0;height:210px;background:#f4f1ea;color:#161616;padding:30px 34px}
.modern .au{font:600 13px/1 "Futura";letter-spacing:.3em;opacity:.7}
.modern .ti{font:700 44px/1.02 "Futura";margin-top:12px;letter-spacing:-.01em}
.gothic img{filter:grayscale(.6) contrast(1.2) brightness(.7)}
.gothic .shade{position:absolute;inset:0;background:linear-gradient(180deg,rgba(0,0,0,.2),rgba(0,0,0,.85))}
.gothic .tx{position:absolute;left:36px;right:36px;bottom:40px;text-align:center}
.gothic .au{font:500 13px/1 "Didot";letter-spacing:.4em;opacity:.85}
.gothic .rule{width:60px;height:2px;background:#b3121c;margin:16px auto}
.gothic .ti{font:400 52px/1 "Didot"}
.kids img{filter:saturate(1.3) brightness(1.05)}
.kids .shade{position:absolute;inset:0;background:linear-gradient(0deg,rgba(10,40,20,.65),rgba(0,0,0,0) 55%)}
.kids .tx{position:absolute;left:30px;right:30px;bottom:34px;text-align:center}
.kids .ti{font:800 42px/1.02 "Avenir Next";text-shadow:0 3px 0 rgba(0,0,0,.25)}
.kids .au{margin-top:12px;font:600 15px/1 "Avenir Next";opacity:.9}
`;

// ---------- run ----------
const browser = await chromium.launch();
const ctx = await browser.newContext({ deviceScaleFactor: 1 });

async function renderMany(css, w, h, jobs, { transparent = false, selector = null } = {}) {
  if (!jobs.length) return;
  const tpl = path.join(WORK, `tpl-${w}x${h}-${transparent ? 't' : 'o'}.html`);
  await fs.writeFile(tpl, `<!doctype html><html><head><meta charset="utf-8"><style>${css}</style></head><body><div id="root"></div></body></html>`);
  const pages = await Promise.all(Array.from({ length: 4 }, async () => {
    const p = await ctx.newPage();
    await p.setViewportSize({ width: w, height: h });
    await p.goto(pathToFileURL(tpl).href);
    return p;
  }));
  const queue = [...jobs];
  await Promise.all(pages.map(async (page) => {
    while (queue.length) {
      const job = queue.shift();
      await page.evaluate((html) => { document.getElementById('root').innerHTML = html; }, job.html);
      await page.evaluate(() => Promise.all([...document.images].map((i) => (i.complete ? null : new Promise((r) => { i.onload = r; i.onerror = r; })))));
      await page.evaluate(() => document.fonts.ready);
      const png = job.out.replace(/\.webp$/, '.png').replace(OUT, WORK);
      await fs.mkdir(path.dirname(png), { recursive: true });
      if (selector) await page.locator(selector).screenshot({ path: png, omitBackground: true });
      else await page.screenshot({ path: png, omitBackground: transparent, clip: { x: 0, y: 0, width: w, height: h } });
      await magick([png, '-strip', '-quality', transparent ? '90' : '82', ...(job.post ?? []), job.out]);
    }
  }));
  await Promise.all(pages.map((p) => p.close()));
}

const fileUrl = (f) => pathToFileURL(f).href;
const log = (...a) => console.log('[art]', ...a);

// Movies
const posterJobs = [];
const logoJobs = [];
const posterFrom = [];

for (const d of movieDefs) {
  const poster = path.join(OUT, 'poster', `${d.id}.webp`);
  const backdrop = path.join(OUT, 'backdrop', `${d.id}.webp`);
  const logo = path.join(OUT, 'logo', `${d.id}.webp`);
  const cp = d.commons?.poster && commonsFile(d.commons.poster);
  const cb = d.commons?.backdrop && commonsFile(d.commons.backdrop);
  const cl = d.commons?.logo && commonsFile(d.commons.logo);
  // backdrop
  if (want('backdrops')) {
    if (cb) await toWebp(cb, backdrop, { w: 1600, h: 900, q: 76 });
    else if (d.back != null) await toWebp(await picsum(d.back, 1920, 1080), backdrop, { w: 1600, h: 900, q: 74 });
    else if (cp) {
      // No still exists: build a wide backdrop from the poster itself (blurred fill + centered key art).
      if (!exists(backdrop))
        await magick([cp, '-resize', '1600x900^', '-gravity', 'center', '-extent', '1600x900', '-blur', '0x30', '-modulate', '70', '(', cp, '-resize', 'x900', ')', '-gravity', 'east', '-geometry', '+120+0', '-composite', '-strip', '-quality', '76', backdrop]);
    }
  }
  // poster
  if (want('posters')) {
    const fit = d.commons?.posterFit && commonsFile(d.commons.posterFit);
    if (cp) await toWebp(cp, poster, { w: 480, h: 720, q: 82, gravity: 'north' });
    else if (fit) {
      if (!exists(poster))
        await magick([fit, '-resize', '480x720^', '-gravity', 'center', '-extent', '480x720', '-blur', '0x24', '-modulate', '75', '(', fit, '-resize', '480x', ')', '-gravity', 'center', '-composite', '-strip', '-quality', '82', poster]);
    } else {
      const src = d.seed != null ? await picsum(d.seed, 720, 1080) : cb;
      if (src && !exists(poster)) posterJobs.push({ html: posterHTML(d, fileUrl(src)), out: poster });
    }
  }
  // logo
  if (want('logos')) {
    if (cl) await toWebp(cl, logo, { w: 900, q: 90 });
    else if (!d.commons && !exists(logo)) logoJobs.push({ html: logoHTML(d), out: logo, post: ['-trim', '+repage', '-resize', '900x300>'] });
  }
  posterFrom.push(poster);
}

// Shows + anime
const stillJobs = [];
for (const d of showDefs) {
  const poster = path.join(OUT, 'poster', `${d.id}.webp`);
  const backdrop = path.join(OUT, 'backdrop', `${d.id}.webp`);
  const logo = path.join(OUT, 'logo', `${d.id}.webp`);
  const anime = d.style === 'anime';
  const grade = anime ? ['-modulate', '104,145,98', '-sigmoidal-contrast', '3x45%'] : [];
  if (want('backdrops')) await toWebp(await picsum(d.back, 1920, 1080), backdrop, { w: 1600, h: 900, q: 74, extra: grade });
  if (want('posters') && !exists(poster)) posterJobs.push({ html: posterHTML({ ...d, director: d.creators[0] }, fileUrl(await picsum(d.seed, 720, 1080))), out: poster });
  if (want('logos') && !exists(logo)) logoJobs.push({ html: logoHTML(d), out: logo, post: ['-trim', '+repage', '-resize', '900x300>'] });
  if (want('stills')) {
    let k = 0;
    d.seasons.forEach((s, si) => {
      s.episodes.forEach((_, ei) => {
        const id = `${d.id}-s${si + 1}e${ei + 1}`;
        const seed = d.stills[k % d.stills.length];
        k++;
        stillJobs.push({ id, seed, grade });
      });
    });
  }
}
if (want('stills')) {
  await pool(stillJobs, 10, async (j) => {
    const dest = path.join(OUT, 'still', `${j.id}.webp`);
    if (exists(dest)) return;
    await toWebp(await picsum(j.seed, 800, 450), dest, { w: 640, h: 360, q: 72, extra: j.grade });
  });
  log('stills', stillJobs.length);
}

await renderMany(POSTER_CSS, 480, 720, posterJobs);
log('posters', posterJobs.length);
await renderMany('*{margin:0;padding:0}body{background:transparent;-webkit-font-smoothing:antialiased;width:1400px}', 1400, 500, logoJobs, { transparent: true, selector: '#logo' });
log('logos', logoJobs.length);

// Albums
if (want('covers')) {
  const jobs = [];
  for (const a of albumDefs) {
    const out = path.join(OUT, 'cover', `${a.id}.webp`);
    if (exists(out)) continue;
    const artist = artistDefs.find((x) => x.id === a.artistId);
    jobs.push({ html: albumHTML(a, artist, fileUrl(await picsum(a.cover, 900, 900))), out });
  }
  await renderMany(COVER_CSS, 600, 600, jobs);
  const bj = [];
  for (const b of audiobookDefs) {
    const out = path.join(OUT, 'cover', `${b.id}.webp`);
    if (exists(out)) continue;
    bj.push({ html: bookHTML(b, fileUrl(await picsum(b.cover, 900, b.coverStyle === 'classic' ? 500 : 900))), out });
  }
  await renderMany(BOOK_CSS, 600, 600, bj);
  log('covers', jobs.length + bj.length);
}

// Artists
if (want('artists')) {
  await pool(artistDefs, 8, async (a) => toWebp(await picsum(a.photo, 900, 900), path.join(OUT, 'artist', `${a.id}.webp`), { w: 640, h: 640, q: 78 }));
  log('artists', artistDefs.length);
}

// Photos
if (want('photos')) {
  const jobs = photoAlbumDefs.flatMap((a) => a.seeds.map((seed) => ({ seed, portrait: a.portrait?.includes(seed) })));
  await pool(jobs, 10, async ({ seed, portrait }) => {
    const [w, h] = portrait ? [1067, 1600] : [1600, 1067];
    const src = await picsum(seed, w, h);
    await toWebp(src, path.join(OUT, 'photo', `ph-${seed}.webp`), { w, h, q: 80 });
    await toWebp(src, path.join(OUT, 'photo-thumb', `ph-${seed}.webp`), { w: portrait ? 320 : 480, h: portrait ? 480 : 320, q: 72 });
  });
  log('photos', jobs.length);
  await pool(homeVideoDefs, 8, async (v) => toWebp(await picsum(v.seed, 800, 450), path.join(OUT, 'still', `${v.id}.webp`), { w: 640, h: 360, q: 74 }));
}

// Live TV program art
if (want('programs')) {
  await pool(programSeeds, 10, async (seed) => toWebp(await picsum(seed, 640, 360), path.join(OUT, 'program', `s${seed}.webp`), { w: 480, h: 270, q: 72 }));
  log('programs', programSeeds.length);
}

await browser.close();

// Palettes for everything we tint by.
const palTargets = [];
for (const dir of ['poster', 'cover', 'still', 'artist', 'photo-thumb', 'backdrop']) {
  for (const f of readdirSync(path.join(OUT, dir))) if (f.endsWith('.webp')) palTargets.push([path.join(OUT, dir, f), `/art/${dir}/${f}`]);
}
await pool(palTargets, 12, ([f, key]) => paletteOf(f, key));
const sorted = Object.fromEntries(Object.entries(palettes).sort(([a], [b]) => a.localeCompare(b)));
await fs.writeFile(path.join(ROOT, 'src/data/generated/palettes.json'), JSON.stringify(sorted, null, 0).replace(/},/g, '},\n') + '\n');
log('palettes', palTargets.length);
