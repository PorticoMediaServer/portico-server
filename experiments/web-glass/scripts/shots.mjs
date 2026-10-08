// Screenshots every screen in src/features/screens/screens.ts at four sizes.
// Fails (exit 1) on console errors, uncaught exceptions or sideways scrolling.
//
//   npm run shots                 all screens, all sizes
//   npm run shots -- --only=Movie  screens whose name contains "Movie"
//   BASE_URL=http://127.0.0.1:5414 npm run shots   against the production preview
import { chromium } from 'playwright';
import { spawn } from 'node:child_process';
import fs from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const OUT = path.join(ROOT, 'shots');
const { SCREENS, screenUrl } = await import(pathToFileURL(path.join(ROOT, 'src/features/screens/screens.ts')).href);
const only = (process.argv.find((a) => a.startsWith('--only=')) ?? '').slice(7).toLowerCase();
const SIZES = [
  { name: '1440x900', width: 1440, height: 900 },
  { name: '1920x1080', width: 1920, height: 1080 },
  { name: '390x844', width: 390, height: 844, mobile: true },
  { name: '430x932', width: 430, height: 932, mobile: true },
];

let base = process.env.BASE_URL ?? 'http://127.0.0.1:5404';
let server = null;
async function up(url) {
  try {
    const r = await fetch(url);
    return r.ok;
  } catch {
    return false;
  }
}
if (!(await up(base))) {
  base = 'http://127.0.0.1:5404';
  server = spawn('npx', ['vite', '--host', '127.0.0.1', '--port', '5404', '--strictPort'], { cwd: ROOT, stdio: 'ignore' });
  for (let i = 0; i < 60 && !(await up(base)); i++) await new Promise((r) => setTimeout(r, 500));
}

const slug = (s) => s.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '');
const screens = SCREENS.filter((s) => !only || `${s.group} ${s.name}`.toLowerCase().includes(only));
await fs.mkdir(OUT, { recursive: true });
const browser = await chromium.launch();
const failures = [];
const report = [];

async function shoot(size, s) {
  const ctx = await browser.newContext({
    viewport: { width: size.width, height: size.height },
    deviceScaleFactor: size.mobile ? 2 : 1,
    isMobile: !!size.mobile,
    hasTouch: !!size.mobile,
    reducedMotion: 'reduce',
  });
  const page = await ctx.newPage();
  const errors = [];
  page.on('console', (m) => {
    if (m.type() === 'error') errors.push(m.text());
  });
  page.on('pageerror', (e) => errors.push(`Uncaught: ${e.message}`));
  const url = base + screenUrl(s);
  try {
    await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 30000 });
    await page.waitForTimeout(s.path.includes('latency=slow') ? 3500 : 1600);
    // nudge lazy images into view, then return to the top
    await page.evaluate(async () => {
      const h = document.documentElement.scrollHeight;
      for (let y = 0; y < Math.min(h, 7000); y += 700) {
        window.scrollTo(0, y);
        await new Promise((r) => setTimeout(r, 60));
      }
      window.scrollTo(0, 0);
    });
    await page.waitForTimeout(700);
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    const dir = path.join(OUT, size.name);
    await fs.mkdir(dir, { recursive: true });
    const file = path.join(dir, `${slug(s.group)}--${slug(s.name)}.png`);
    const full = await page.evaluate(() => document.documentElement.scrollHeight);
    await page.screenshot({ path: file, fullPage: full < 6000, animations: 'disabled' });
    const problems = [];
    if (overflow > 1) problems.push(`sideways scroll: ${overflow}px`);
    for (const e of errors) problems.push(`console: ${e}`);
    report.push({ size: size.name, screen: `${s.group} / ${s.name}`, file: path.relative(ROOT, file), problems });
    if (problems.length) failures.push(`${size.name}  ${s.group} / ${s.name}\n    ${problems.join('\n    ')}`);
  } catch (e) {
    failures.push(`${size.name}  ${s.group} / ${s.name}\n    ${e.message}`);
  } finally {
    await ctx.close();
  }
}

const jobs = SIZES.flatMap((size) => screens.map((s) => () => shoot(size, s)));
let done = 0;
await Promise.all(
  Array.from({ length: 4 }, async () => {
    while (jobs.length) {
      await jobs.shift()();
      done++;
      if (done % 20 === 0) process.stdout.write(`  ${done} shots\n`);
    }
  }),
);
await browser.close();
if (server) server.kill();
await fs.writeFile(path.join(OUT, 'report.json'), JSON.stringify(report, null, 2));
console.log(`${report.length} screenshots in ${path.relative(process.cwd(), OUT)}/ (${screens.length} screens × ${SIZES.length} sizes)`);
if (failures.length) {
  console.error(`\n${failures.length} problems:\n${failures.join('\n')}`);
  process.exit(1);
}
console.log('No console errors. No sideways scrolling.');
