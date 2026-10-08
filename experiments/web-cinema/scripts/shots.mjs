// Screenshots every screen and state in the screen index at the four review sizes.
//
//   npm run shots                      all screens, all sizes (builds and serves the app on 5412 if needed)
//   npm run shots -- movie home        only screens whose id contains one of these words
//   npm run shots -- --sizes=390x844   only these sizes (comma-separated)
//   BASE=http://127.0.0.1:5402 npm run shots   use a running dev server instead
//
// Output: shots/<width>x<height>/<group>--<id>.png (the folder is replaced on each full run).
// Exits non-zero if any page logs a console error or throws.
import {chromium} from 'playwright';
import {spawn, execSync} from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import {fileURLToPath} from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const args = process.argv.slice(2);
const sizesArg = args.find(a => a.startsWith('--sizes='));
const SIZES = (sizesArg ? sizesArg.slice(8).split(',') : ['1440x900', '1920x1080', '1024x768', '390x844']).map(s => s.split('x').map(Number));
const only = args.filter(a => !a.startsWith('--'));
const BASE = process.env.BASE ?? 'http://127.0.0.1:5412';
const groups = JSON.parse(fs.readFileSync(path.join(root, 'src/screens/dev/screens.json'), 'utf8'));
const slug = s => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
const screens = groups.flatMap(g => g.items.map(i => ({...i, group: slug(g.group)}))).filter(s => !only.length || only.some(o => s.id.includes(o)));

async function up(url) {
  try { const r = await fetch(url); return r.ok; } catch { return false; }
}

let server;
if (!(await up(BASE))) {
  if (!process.env.BASE) {
    if (!fs.existsSync(path.join(root, 'dist/index.html'))) execSync('npm run build', {cwd: root, stdio: 'inherit'});
    server = spawn('npx', ['vite', 'preview', '--host', '127.0.0.1', '--port', '5412', '--strictPort'], {cwd: root, stdio: 'ignore'});
    for (let i = 0; i < 60 && !(await up(BASE)); i++) await new Promise(r => setTimeout(r, 250));
  }
  if (!(await up(BASE))) { console.error(`Nothing is serving ${BASE}.`); process.exit(2); }
}

const outRoot = path.join(root, 'shots');
if (!only.length && !sizesArg) fs.rmSync(outRoot, {recursive: true, force: true});
const browser = await chromium.launch();
const failures = [];
let count = 0;
try {
  for (const [w, h] of SIZES) {
    const dir = path.join(outRoot, `${w}x${h}`);
    fs.mkdirSync(dir, {recursive: true});
    const context = await browser.newContext({viewport: {width: w, height: h}, deviceScaleFactor: 1, hasTouch: w < 768, isMobile: w < 768, reducedMotion: 'no-preference'});
    const queue = [...screens];
    const worker = async () => {
      while (queue.length) {
        const s = queue.shift();
        // A fresh page per screen: sessionStorage (review switches like ?profile=) must not leak.
        const page = await context.newPage();
        const errors = [];
        const onConsole = m => { if (m.type() === 'error') errors.push(m.text()); };
        const onError = e => errors.push(String(e));
        page.on('console', onConsole);
        page.on('pageerror', onError);
        const url = new URL(s.path, BASE);
        if (!url.searchParams.has('latency')) url.searchParams.set('latency', '0');
        try {
          await page.goto(url.href, {waitUntil: 'networkidle', timeout: 30000});
          await page.waitForTimeout(s.path.includes('demo=') ? 1600 : s.path.includes('latency=4') ? 300 : 700);
          await page.evaluate(() => document.fonts.ready);
          await page.screenshot({path: path.join(dir, `${s.group}--${s.id}.png`)});
          count++;
        } catch (e) {
          errors.push(`navigation: ${e.message.split('\n')[0]}`);
        }
        await page.close();
        const real = errors.filter(e => !/Failed to load resource: the server responded with a status of 404/.test(e) || !/favicon/.test(e));
        if (real.length) failures.push({screen: s.id, size: `${w}x${h}`, errors: real});
      }
    };
    await Promise.all([worker(), worker(), worker(), worker()]);
    await context.close();
    console.log(`${w}x${h}: done`);
  }
} finally {
  await browser.close();
  server?.kill();
}
console.log(`${count} screenshots in ${path.relative(process.cwd(), outRoot) || outRoot}`);
if (failures.length) {
  console.error(`\n${failures.length} pages logged errors:`);
  for (const f of failures) console.error(`  ${f.screen} @ ${f.size}\n    ${f.errors.join('\n    ')}`);
  process.exit(1);
}
