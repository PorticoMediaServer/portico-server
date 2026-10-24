import {resolve, join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
import {withMeasurementBrowser} from './browser.mjs';
import {comparePixels} from './png.mjs';
import {compareStrings} from './strings.mjs';
import {compareTextMetrics} from './text-metrics.mjs';

const root = resolve(fileURLToPath(new URL('../..', import.meta.url)));

/** Full declared frame versus the actual DEV screen, with measured state and no API. */
export async function compareWebScreen({referenceURL, referenceSelector, entry, actualSelector, viewport, focus, publicFixtureText}) {
  if (!process.env.TMPDIR || !process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH) throw new Error('Owned runner scratch and explicit Chromium required');
  if (!entry.startsWith('/src/dev/entries/') && !entry.startsWith('/test/')) throw new Error('An explicit controlled DEV entry is required');
  const vite = await createServer({root: join(root, 'web-next'), configFile: join(root, 'web-next/vite.config.ts'),
    cacheDir: join(process.env.TMPDIR, 'fidelity-vite'), server: {host: '127.0.0.1', port: 0, strictPort: false}});
  try {
    await vite.listen();
    const actualOrigin = `http://127.0.0.1:${vite.httpServer.address().port}`, actualURL = actualOrigin + entry;
    return await withMeasurementBrowser({scratch: process.env.TMPDIR, executable: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,
      viewport, allowedOrigins: [new URL(referenceURL).origin, actualOrigin]}, async browser => {
      await browser.navigate(referenceURL, referenceSelector);
      const reference = await browser.measure(referenceSelector), expectedPixels = await browser.raster(referenceSelector, {referenceFrame: true});
      await browser.navigate(actualURL, actualSelector);
      if (focus) {
        const point = await browser.evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(focus)});if(!e)throw new Error('Missing fixture control');const r=e.getBoundingClientRect();return{x:r.x+r.width/2,y:r.y+r.height/2}})()`);
        await browser.call('Input.dispatchMouseEvent', {type: 'mousePressed', button: 'left', clickCount: 1, ...point});
        await browser.call('Input.dispatchMouseEvent', {type: 'mouseReleased', button: 'left', clickCount: 1, ...point});
        if (publicFixtureText !== undefined) await browser.call('Input.insertText', {text: publicFixtureText});
      } else if (publicFixtureText !== undefined) throw new Error('Public fixture text requires an explicit focused control');
      const actual = await browser.measure(actualSelector), actualPixels = await browser.raster(actualSelector);
      const raster = comparePixels(expectedPixels, actualPixels), strings = compareStrings(reference, actual), textMetrics = compareTextMetrics(reference, actual);
      return {version: 1, scope: 'screen', viewport,
        reference: {selector: referenceSelector, measurement: reference},
        actual: {entry, selector: actualSelector, measurement: actual},
        state: {focusRequested: Boolean(focus), typedPublicFixtureCharacters: publicFixtureText?.length ?? 0},
        raster, strings, textMetrics, network: {blocked: browser.blocked}, pageErrors: browser.pageErrors,
        verdict: !actual.fullViewport || browser.blocked.length || browser.pageErrors.length ? 'invalid' :
          raster.changedPixels || strings.differences.length ? 'different' : 'exact',
        qualification: 'Controlled full-screen measurement; not an application flow, native or hardware result. No implicit caret mask or tolerance.'};
    });
  } finally {await vite.close();}
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const argv = process.argv.slice(2), args = {};
  for (let index = 0; index < argv.length; index += 2) {
    if (!argv[index].startsWith('--') || argv[index + 1] === undefined) throw new Error('Named argument/value pairs required');
    const name = argv[index].slice(2); if (name in args) throw new Error('Duplicate argument'); args[name] = argv[index + 1];
  }
  for (const key of ['reference-url', 'reference-selector', 'entry', 'actual-selector', 'width', 'height']) if (!args[key]) throw new Error(`Missing --${key}`);
  const width = Number(args.width), height = Number(args.height);
  if (!Number.isInteger(width) || !Number.isInteger(height) || width < 1 || height < 1 || width * height > 16_000_000) throw new Error('Invalid full viewport dimensions');
  const result = await compareWebScreen({referenceURL: args['reference-url'], referenceSelector: args['reference-selector'],
    entry: args.entry, actualSelector: args['actual-selector'], viewport: {width, height, deviceScaleFactor: 1},
    focus: args.focus, publicFixtureText: args['public-fixture-text']});
  console.log('FIDELITY_SCREEN_JSON ' + JSON.stringify(result));
  if (result.verdict === 'invalid') process.exitCode = 2;
}
