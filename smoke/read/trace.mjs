// Caught-exception discriminator for one Reader open, over the Chrome DevTools Protocol.
//
// Prepared only: it creates a reading session, so it must not run until Release says the demo is
// ready. No app source, guard or preference is changed; this observes and resumes.
//
//   SMOKE_CREDENTIALS=/path/creds.json SMOKE_OUT=/path/out \
//   node smoke/read/trace.mjs --title 1905 --library Comics
//
// Bounds: pause on caught exceptions only, around one Reader open. Every Debugger.paused event is
// resumed at once and at most 20 summaries are kept. Each summary is exception name, an enum-shaped
// code when one exists, and a public script basename with line/column and a plain function name.
// A cause is inspected ONLY to match the anchored message
// "This server does not publish the preference <key>." and then only the key is recorded.
// No raw description, stack, object dump, response, body, grant or token is written anywhere.
import {readFileSync, mkdirSync, writeFileSync} from 'node:fs';
import {join} from 'node:path';
import {loadPlaywright, launch, Evidence, settle} from '../lib/page.mjs';

const origin = 'https://demo.getportico.tv';
const out = process.env.SMOKE_OUT || join(process.cwd(), 'read-trace');
const args = process.argv.slice(2);
const title = args[args.indexOf('--title') + 1] || '1905';
const library = args[args.indexOf('--library') + 1] || 'Comics';
const MAX_SUMMARIES = 20;
mkdirSync(out, {recursive: true});
const creds = JSON.parse(readFileSync(process.env.SMOKE_CREDENTIALS, 'utf8'));

const ENUM_CODE = /^[A-Za-z0-9_.:-]{1,60}$/;
const PLAIN_NAME = /^[A-Za-z_$][A-Za-z0-9_$]{0,60}$/;
const ANCHORED_CAUSE = /^This server does not publish the preference ([A-Za-z0-9_.-]{1,80})\.$/;

const playwright = await loadPlaywright();
const {browser, context, page} = await launch(playwright, {viewport: {width: 1440, height: 900}});
const evidence = new Evidence(out);
const cdp = await context.newCDPSession(page);
const scripts = new Map();
const summaries = [];

await cdp.send('Debugger.enable').catch(() => {});
cdp.on('Debugger.scriptParsed', event => {
  try { scripts.set(event.scriptId, new URL(event.url).pathname.split('/').pop()); } catch { scripts.set(event.scriptId, ''); }
});
cdp.on('Debugger.paused', async event => {
  // Always resume, whatever happens while summarising.
  try {
    if (summaries.length < MAX_SUMMARIES && event.data) {
      const details = event.data;
      const topFrame = details.callFrames && details.callFrames[0] ? details.callFrames[0] : null;
      const location = typeof details.scriptId === 'string'
        ? {scriptId: details.scriptId, line: details.lineNumber, column: details.columnNumber}
        : (topFrame ? {scriptId: topFrame.location.scriptId, line: topFrame.location.lineNumber, column: topFrame.location.columnNumber} : {});
      const summary = {
        at: new Date().toISOString(),
        name: details.exception && details.exception.className ? String(details.exception.className).slice(0, 40) : null,
        script: scripts.get(location.scriptId) || null,
        line: typeof location.line === 'number' ? location.line + 1 : null,
        column: typeof location.column === 'number' ? location.column + 1 : null,
        function: topFrame && typeof topFrame.functionName === 'string' && PLAIN_NAME.test(topFrame.functionName) ? topFrame.functionName : null,
      };
      if (details.exception && details.exception.objectId) {
        // Bounded property read: only an enum-shaped code, and only the anchored cause key.
        const probe = `
          function () {
            var result = {code: null, preferenceKey: null};
            try {
              if (typeof this.code === 'string' && ${ENUM_CODE.toString()}.test(this.code)) result.code = this.code;
            } catch (e) {}
            try {
              var message = this.cause && this.cause.message;
              if (typeof message === 'string') {
                var match = message.match(${ANCHORED_CAUSE.toString()});
                if (match) result.preferenceKey = match[1];
              }
            } catch (e) {}
            return result;
          }`;
        const answer = await cdp.send('Runtime.callFunctionOn', {
          objectId: details.exception.objectId, functionDeclaration: probe, returnByValue: true, silent: true,
        }).catch(() => null);
        const value = answer && answer.result && answer.result.value;
        if (value) { summary.code = value.code; summary.preferenceKey = value.preferenceKey; }
      }
      summaries.push(summary);
    }
  } finally {
    await cdp.send('Debugger.resume').catch(() => {});
  }
});

try {
  await page.goto(origin + '/sign-in', {waitUntil: 'domcontentloaded', timeout: 45000});
  await page.locator('input[name=username]').first().waitFor({timeout: 45000});
  await page.locator('input[name=username]').fill(creds.username);
  await page.locator('input[type=password]').fill(creds.password);
  await page.getByRole('button', {name: 'Sign in', exact: true}).click();
  await page.waitForTimeout(3500);
  if (/\/profiles/.test(page.url())) {
    await page.locator('button').filter({hasText: /Main profile/}).first().click({timeout: 8000});
    await page.waitForTimeout(3500);
  }
  const nav = page.locator('nav a[href]').filter({hasText: new RegExp(`^${library}$`, 'i')}).first();
  await nav.click({timeout: 10000});
  await settle(page, {quiet: 2500});

  // Only now, and only for this one open.
  await cdp.send('Debugger.setPauseOnExceptions', {state: 'caught'}).catch(() => {});
  const openedAt = new Date().toISOString();
  const card = page.locator('article[data-card-kind]').filter({hasText: title}).first();
  const read = card.getByRole('button', {name: /^Read$/}).first();
  if (await read.count()) await read.click({timeout: 10000});
  else { await card.locator('button[aria-label]').first().focus(); await page.keyboard.press('Enter'); }
  await page.waitForTimeout(12000);
  await cdp.send('Debugger.setPauseOnExceptions', {state: 'none'}).catch(() => {});

  const surface = await page.evaluate(() => ({
    urlPath: location.pathname,
    readerRoot: !!document.querySelector('[data-reader-root]'),
    errorPage: /Something didn't load|ran into a problem/i.test(document.body.innerText || ''),
    textLength: (document.body.innerText || '').trim().length,
    drawnTextHead: (document.body.innerText || '').replace(/\n{2,}/g, ' | ').trim().slice(0, 200),
  }));
  await page.screenshot({path: `${out}/trace.png`});

  const report = {title, library, openedAt, surface, caughtExceptions: summaries.slice(0, MAX_SUMMARIES), totalCaught: summaries.length};
  writeFileSync(`${out}/read-trace.json`, JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report, null, 2));
} finally {
  await cdp.send('Debugger.setPauseOnExceptions', {state: 'none'}).catch(() => {});
  await cdp.send('Debugger.disable').catch(() => {});
  await context.close();
  await browser.close();
}
