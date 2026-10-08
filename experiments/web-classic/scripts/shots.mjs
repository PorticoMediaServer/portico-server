#!/usr/bin/env node
/**
 * Screenshots every screen in the screen index at four viewport sizes, and
 * fails if any page logs a console error, throws, or scrolls sideways.
 *
 *   npm run shots                         all screens, all sizes (against the preview server)
 *   npm run shots -- --only home,movie    some screens
 *   npm run shots -- --sizes 1440         some sizes (1440, 1920, 1024, 390)
 *   npm run shots -- --base http://127.0.0.1:5401   another server
 *
 * Output: shots/<size>/<screen>.png (the folder is emptied first).
 */
import {chromium} from 'playwright';
import {mkdirSync, rmSync} from 'node:fs';
import {SCREENS} from './screens.mjs';

const args = process.argv.slice(2);
const opt = (name, fallback) => { const i = args.indexOf(`--${name}`); return i >= 0 ? args[i + 1] : fallback; };
const base = opt('base', 'http://127.0.0.1:5411');
const only = opt('only', '')?.split(',').filter(Boolean);
const SIZES = {1440: [1440, 900], 1920: [1920, 1080], 1024: [1024, 768], 390: [390, 844]};
const sizes = (opt('sizes', '') || Object.keys(SIZES).join(',')).split(',');
const full = args.includes('--full');
const out = opt('out', 'shots');

const screens = SCREENS.filter(s => !only?.length || only.includes(s.id));
if (!only?.length) rmSync(out, {recursive: true, force: true});
const browser = await chromium.launch();
const failures = [];
for (const size of sizes) {
  const [width, height] = SIZES[size];
  mkdirSync(`${out}/${size}`, {recursive: true});
  const context = await browser.newContext({viewport: {width, height}, deviceScaleFactor: 1, reducedMotion: 'reduce', hasTouch: size === '390', isMobile: size === '390'});
  const page = await context.newPage();
  let errors = [];
  page.on('console', m => { if (m.type() === 'error') errors.push(m.text()); });
  page.on('pageerror', e => errors.push(String(e)));
  for (const s of screens) {
    errors = [];
    const [pathPart, hash] = s.path.split('#');
    const sep = pathPart.includes('?') ? '&' : '?';
    await page.goto(`${base}${pathPart}${sep}latency=0${hash ? `#${hash}` : ''}`, {waitUntil: 'networkidle'});
    await page.waitForTimeout(s.wait ?? 700);
    if (s.run) { try { await s.run(page); } catch (e) { errors.push(`run: ${e}`); } await page.waitForTimeout(500); }
    await page.screenshot({path: `${out}/${size}/${s.id}.png`, fullPage: full || !!s.full});
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
    if (overflow > 1) errors.push(`page scrolls sideways by ${overflow}px`);
    if (errors.length) failures.push(`${size} ${s.id}: ${errors.join(' | ')}`);
  }
  await context.close();
}
await browser.close();
if (failures.length) { console.error(`Console errors:\n${failures.join('\n')}`); process.exit(1); }
console.log(`OK: ${screens.length} screens × ${sizes.length} sizes in ${out}/`);
