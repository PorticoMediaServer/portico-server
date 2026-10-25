// Same-size, same-state capture of desktop Detail frames 2, 3 and 6. Run on the external runner with --browser and the
// exact design secondary source. It mounts the genuine DEV specimen and the
// frame's own markup/CSS; it commits no frame, artwork or private input.
// Diagnostic only: a plain side-by-side pair for the lane report, never a
// fidelity verdict and never a second comparator.
import {createRequire} from 'node:module';
import {readFile, mkdir} from 'node:fs/promises';
import {existsSync} from 'node:fs';
import {createServer} from 'vite';
import path from 'node:path';

const output = process.env.PORTICO_RUNNER_OUTPUT, design = process.env.PORTICO_RUNNER_SECONDARY_SOURCE;
const executablePath = process.env.PORTICO_CHROMIUM ?? ['/usr/bin/chromium', '/snap/bin/chromium', '/usr/bin/google-chrome'].find(existsSync);
if (!executablePath) throw new Error('No Chromium executable on this host.');
if (!output || !design) throw new Error('Run with PORTICO_RUNNER_OUTPUT and the exact design PORTICO_RUNNER_SECONDARY_SOURCE.');
const root = process.env.PORTICO_RUNNER_SITE ?? path.resolve('.');
const designRoot = path.join(design, 'design/ui-v1');
const require = createRequire(import.meta.url);
const {chromium} = require('playwright-core');
await mkdir(output, {recursive: true});

const mime = {'.jpg':'image/jpeg','.jpeg':'image/jpeg','.png':'image/png','.svg':'image/svg+xml','.webp':'image/webp'};
const server = await createServer({root, configFile:path.join(root,'vite.config.ts'), cacheDir:path.join(process.env.TMPDIR ?? output,'detail-vite'), plugins:[{name:'detail-reference-assets',configureServer(context){context.middlewares.use(async(req,res,next)=>{const pathname=decodeURIComponent(new URL(req.url,'http://localhost').pathname);const prefix=pathname.startsWith('/__design-assets__/assets/')?'/__design-assets__/assets/':pathname.startsWith('/assets/')?'/assets/':null;if(!prefix)return next();const relative=pathname.slice(prefix.length);if(relative.includes('..')||path.isAbsolute(relative))return next();try{res.setHeader('Content-Type',mime[path.extname(relative)]??'application/octet-stream');res.end(await readFile(path.join(designRoot,'assets',relative)));}catch{res.statusCode=404;res.end();}});}}],server:{host:'127.0.0.1',port:0}});
const states = process.argv.slice(2).length ? process.argv.slice(2) : ['6'];
if (states.some(state => !['2', '3', '6'].includes(state))) throw new Error('Expected desktop Detail frame 2, 3 or 6.');
let browser;
let watchdog;
try {
  await server.listen();
  const origin = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await chromium.launch({executablePath, headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage']});
  watchdog = setTimeout(async () => {await browser?.close();await server.close();process.exit(1);}, 240000);
  const page = await browser.newPage({viewport: {width: 1440, height: 900}, deviceScaleFactor: 1});
  page.setDefaultTimeout(20000);
  const kit = await readFile(path.join(designRoot, 'kit.css'), 'utf8');
  const source = (await readFile(path.join(designRoot, 'desktop-detail.html'), 'utf8'))
    .replace('<head>', `<head><base href="${origin}/">`)
    .replace('<link rel="stylesheet" href="kit.css">', `<link rel="stylesheet" href="${origin}/src/kit/fonts.css"><style>${kit}</style>`)
    .replace(/<link[^>]+https:\/\/fonts\.[^>]+>/g, '');
  await page.route('**/__detail_reference__*', route => route.fulfill({contentType: 'text/html', body: source}));
  const settle = async () => {
    await page.evaluate(() => document.fonts.ready);
    await page.waitForFunction(() => [...document.images].every(image => image.complete));
    await page.waitForTimeout(300);
  };
  for (const state of states) {
    const prefix = `detail${state}`;
    await page.setViewportSize({width: 1440, height: 900});
    await page.goto(`${origin}/src/dev/entries/${state === '6' ? 'show' : 'detail'}.html?frame=${state}`);
    await page.waitForSelector(state === '6' ? '[data-show-frame6-ready]' : '[data-detail-ready]');
    if (state === '3') {
      const tab = page.getByRole('tab', {name: 'Details', exact: true});
      if (await tab.getAttribute('aria-selected') !== 'true') await tab.click();
    }
    if (state === '3' || state === '6') {
      await page.locator('[class*=tabAnchor]').evaluate(anchor => {
        let scroller = anchor.parentElement;
        while (scroller && scroller !== document.documentElement && !(/auto|scroll/.test(getComputedStyle(scroller).overflowY) && scroller.scrollHeight > scroller.clientHeight)) scroller = scroller.parentElement;
        scroller ??= document.scrollingElement;
        scroller.scrollTop += anchor.getBoundingClientRect().top - 74;
      });
      if (state === '6') await page.getByRole('option').filter({hasText: 'Season 4'}).focus();
    }
    await settle();
    await page.screenshot({path: `${output}/${prefix}-build.jpg`, type: 'jpeg', quality: 90});
    await page.goto(`${origin}/__detail_reference__?only=${state}`);
    await settle();
    await page.locator(`#f${state}`).screenshot({path: `${output}/${prefix}-frame.jpg`, type: 'jpeg', quality: 90});
    const frame = await readFile(`${output}/${prefix}-frame.jpg`), build = await readFile(`${output}/${prefix}-build.jpg`);
    await page.setViewportSize({width: 2880, height: 900});
    await page.setContent(`<body style="margin:0;display:flex;background:#071018"><img width="1440" src="data:image/jpeg;base64,${frame.toString('base64')}"><img width="1440" src="data:image/jpeg;base64,${build.toString('base64')}"></body>`);
    await page.screenshot({path: `${output}/${prefix}-pair.jpg`, type: 'jpeg', quality: 90, fullPage: true});
    if (state === '6') {
      // Exercise the actual anchored kit menu and Show wiring after the pair.
      // The specimen retains canonical asc; server/client tests cover desc pages.
      await page.setViewportSize({width: 1440, height: 900});
      await page.goto(`${origin}/src/dev/entries/show.html?frame=6`);
      await page.waitForSelector('[data-show-frame6-ready]');
      await page.keyboard.press('Escape');
      const trigger = page.getByRole('button', {name: 'Episode order', exact: true});
      await trigger.click();
      const choices = page.getByRole('menuitemradio');
      if (await choices.count() !== 2 || (await choices.allTextContents()).join('|') !== 'Episode order|Reverse episode order') throw new Error('Episode order menu differs from Root §13');
      if (await choices.nth(0).getAttribute('aria-checked') !== 'true' || await choices.nth(1).getAttribute('aria-checked') !== 'false') throw new Error('Default episode order is not checked');
      await choices.nth(0).focus();
      await page.keyboard.press('ArrowDown');
      if (!await choices.nth(1).evaluate(row => row === document.activeElement)) throw new Error('Episode order keyboard navigation failed');
      await page.keyboard.press('Escape');
      await page.waitForFunction(() => !document.querySelector('[role="menuitemradio"]'));
      await page.waitForFunction(button => button === document.activeElement, await trigger.elementHandle(), {timeout: 2000});
      await trigger.click();
      await page.getByRole('menuitemradio', {name: 'Episode order', exact: true}).click();
      await page.waitForFunction(() => !document.querySelector('[role="menuitemradio"]'));
      console.log('Episode order exact rows, checked default, keyboard, Escape, focus return and select-close passed');
    }
    console.log(`Detail frame ${state} pair captured`);
  }
} finally {
  clearTimeout(watchdog);
  await browser?.close();
  await server.close();
}
