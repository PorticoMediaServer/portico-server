import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
import {join, resolve} from 'node:path';
import {readFile} from 'node:fs/promises';
import {createServer} from 'vite';

const version = (await readFile('.node-version', 'utf8')).trim();
assert.equal(process.version, `v${version}`, 'use the repo-declared Node version');
const require = createRequire(join(process.env.PORTICO_BROWSER_TOOLS_DIR, 'loader.cjs'));
const {chromium} = require('playwright-core');
process.env.PORTICO_DEV_SERVER_URL = 'http://127.0.0.1:1';
const server = await createServer({root: resolve('web-next'), configFile: resolve('web-next/vite.config.ts'), cacheDir: join(process.env.TMPDIR, 'saved-vite'), server: {host: '127.0.0.1', port: 0, strictPort: false}});
let browser;
try {
  await server.listen();
  const address = server.httpServer.address();
  const origin = `http://127.0.0.1:${address.port}`;
  browser = await chromium.launch({executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH, headless: true, args: ['--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage', '--renderer-process-limit=1']});
  const page = await browser.newPage({viewport: {width: 1440, height: 900}, deviceScaleFactor: 1});
  const responses = [];
  page.on('response', response => { if (response.url().includes('/v1/downloads/')) responses.push({status: response.status(), url: new URL(response.url()).pathname}); });
  await page.goto(`${origin}/test/saved-downloads-component.html`, {waitUntil: 'networkidle'});
  await page.mouse.click(10, 10);
  await page.evaluate(() => window.savedDownloadsComponentProof.mount());
  const dialog = page.getByRole('dialog', {name: 'Couldn’t load Downloads'});
  await dialog.waitFor({state: 'visible'});
  await page.evaluate(async () => {
    await document.fonts.ready;
    await Promise.all(document.getAnimations().map(animation => animation.finished.catch(() => {})));
  });
  // The reference is a pointer state. Preserve the real keyboard focus ring,
  // and enter pointer modality through an actual non-action dialog click.
  await dialog.getByRole('heading', {name: 'Couldn’t load Downloads'}).click();
  assert.ok(responses.some(response => response.status >= 500), 'the real failed read must reach the service');
  const observations = await dialog.evaluate(element => {
    const bounds = element.getBoundingClientRect();
    return {bounds: {width: bounds.width, height: bounds.height}, text: element.textContent,
      title: (() => { const title = element.querySelector('h2'); const style = getComputedStyle(title); return {font: style.font, spacing: style.letterSpacing}; })(),
      body: getComputedStyle(element.querySelector('p')).font,
      buttons: [...element.querySelectorAll('button')].map(button => ({text: button.textContent, label: button.getAttribute('aria-label'), width: button.getBoundingClientRect().width, height: button.getBoundingClientRect().height, font: getComputedStyle(button).font, padding: getComputedStyle(button).padding}))};
  });
  await dialog.screenshot({path: join(process.env.PORTICO_RUNNER_OUTPUT, 'downloads-load-failure-dialog.jpg'), type: 'jpeg', quality: 95});
  await page.screenshot({path: join(process.env.PORTICO_RUNNER_OUTPUT, 'downloads-load-failure-panel.jpg'), type: 'jpeg', quality: 95, clip: {x: 510, y: 270, width: 420, height: 360}});
  const before = responses.length;
  await dialog.getByRole('button', {name: 'Try again', exact: true}).click();
  await page.waitForFunction(count => performance.getEntriesByType('resource').filter(entry => entry.name.includes('/v1/downloads/')).length > count, before);
  await dialog.waitFor({state: 'visible'});
  await dialog.getByRole('button', {name: 'Close', exact: true}).last().click();
  await dialog.waitFor({state: 'hidden'});
  await page.evaluate(() => window.savedDownloadsComponentProof.dispose());
  console.log(JSON.stringify({kind: 'COMPONENT ONLY; no application/native acceptance', node: process.version, actualTransportResponses: responses, observations, retryAndClose: true}));
} finally { await browser?.close(); await server.close(); }
