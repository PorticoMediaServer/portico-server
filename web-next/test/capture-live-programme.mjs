// One remote capture pair for desktop-live.html#live-sheet. No server data or
// credentials: the DEV specimen uses the drawn synthetic fixture, and the frame
// comes from the exact published design source.
import {spawn} from 'node:child_process';
import {readFile, writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
import {pathToFileURL} from 'node:url';

const output = process.env.PORTICO_RUNNER_OUTPUT, reference = process.env.PORTICO_RUNNER_SECONDARY_SOURCE;
if (!output || !reference || !process.env.PORTICO_BROWSER_MODULE) throw new Error('Run on the external runner with --browser and the exact design secondary source.');
const {chromium} = await import(pathToFileURL(process.env.PORTICO_BROWSER_MODULE));
const origin = 'http://127.0.0.1:5188';
const server = spawn(process.execPath, [resolve('node_modules/vite/bin/vite.js'), '--host', '127.0.0.1', '--port', '5188'],
  {cwd: resolve('web-next'), detached: true, stdio: ['ignore', 'pipe', 'pipe']});
let log = '';
server.stdout.on('data', data => {log += data;});server.stderr.on('data', data => {log += data;});
let browser;
try {
  for (let attempt = 0; ; attempt++) {
    try {if ((await fetch(origin)).ok) break;} catch {}
    if (attempt > 100 || server.exitCode !== null) throw new Error(`Vite did not start: ${log}`);
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  browser = await chromium.launch({executablePath: process.env.PORTICO_CHROMIUM ?? '/usr/bin/chromium', headless: true, args: ['--no-sandbox', '--disable-dev-shm-usage']});
  const page = await browser.newPage({viewport: {width: 1440, height: 1230}, deviceScaleFactor: 1});
  await page.goto(`${origin}/src/dev/entries/live-guide.html?panel=1`);
  await page.locator('[data-panel-specimen]').waitFor();
  await page.evaluate(() => document.fonts.ready);
  await page.waitForTimeout(350);
  await page.evaluate(() => document.activeElement?.blur());
  const implementation = await page.screenshot({path: `${output}/implementation.jpg`, type: 'jpeg', quality: 90});
  const build = await page.evaluate(() => {
    const box = (selector) => {const element = document.querySelector(selector);return element ? {text: element.textContent, box: element.getBoundingClientRect().toJSON(), font: getComputedStyle(element).font} : null;};
    return {panel: box('[data-panel-specimen]'), title: box('[data-programme-panel] h2'), channel: box('[data-programme-panel] [class*=programmeChannel]'),
      actions: box('[data-programme-panel] [class*=programmeActions]'), series: box('[data-programme-panel] [class*=seriesBox]'), airing: box('[data-programme-panel] [class*=airingRow]')};
  });
  const source = await readFile(`${reference}/design/ui-v1/desktop-live.html`, 'utf8');
  const kit = await readFile(`${reference}/design/ui-v1/kit.css`, 'utf8');
  const css = source.match(/<style>([\s\S]*?)<\/style>/)[1];
  const frame = source.slice(source.indexOf('<div class="screen" id="live-sheet"'), source.indexOf('<p class="cap"><b>List view'));
  const sprite = [...source.matchAll(/<svg width="0" height="0" style="position:absolute">[\s\S]*?<\/svg>/g)].map(match => match[0]).join('');
  const referenceHtml = `<!doctype html><html><head><meta charset="utf-8"><link rel="stylesheet" href="${origin}/src/kit/fonts.css"><style>${kit}\n${css}\nbody{margin:0;padding:0}</style></head><body class="p">${sprite}${frame.replace('assets/wordmark.svg', origin + '/src/shell/wordmark.svg')}</body></html>`;
  await page.route('**/__live-panel-reference__', route => route.fulfill({contentType: 'text/html', body: referenceHtml}));
  await page.goto(`${origin}/__live-panel-reference__`);
  await page.evaluate(() => document.fonts.ready);
  const frameImage = await page.screenshot({path: `${output}/frame.jpg`, type: 'jpeg', quality: 90});
  const drawn = await page.evaluate(() => {
    const box = (selector) => {const element = document.querySelector(selector);return element ? {text: element.textContent, box: element.getBoundingClientRect().toJSON(), font: getComputedStyle(element).font} : null;};
    return {panel: box('#live-sheet aside.sheet'), title: box('#live-sheet .sh-title'), channel: box('#live-sheet .sh-ch'),
      actions: box('#live-sheet .sh-act'), series: box('#live-sheet .sh-box'), airing: box('#live-sheet .air-row')};
  });
  await page.setViewportSize({width: 2880, height: 1230});
  await page.setContent(`<body style="margin:0;display:flex"><img width="1440" height="1230" src="data:image/jpeg;base64,${implementation.toString('base64')}"><img width="1440" height="1230" src="data:image/jpeg;base64,${frameImage.toString('base64')}"></body>`);
  await page.screenshot({path: `${output}/comparison.jpg`, type: 'jpeg', quality: 90});
  await writeFile(`${output}/browser.log`, JSON.stringify({build, drawn}, null, 2));
  console.log(JSON.stringify({build, drawn}));
} finally {
  await browser?.close();
  try {process.kill(-server.pid, 'SIGTERM');} catch {}
  await new Promise(resolve => {if (server.exitCode !== null) resolve();else server.once('exit', resolve);});
  await writeFile(`${output}/vite.log`, log);
}
