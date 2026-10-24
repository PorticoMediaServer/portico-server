import {resolve, join} from 'node:path';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';
import {withMeasurementBrowser} from './browser.mjs';
import {enUS} from '../../packages/i18n/src/catalog/en-US.ts';
import {compareCatalogueStrings} from './strings.mjs';
import {compareTextMetrics} from './text-metrics.mjs';
import {compareScreens} from './compare.mjs';
import {auth4} from './auth4.mjs';
import {colorDifference, parseColor} from './color.mjs';

const root = resolve(fileURLToPath(new URL('../..', import.meta.url)));

/** Full declared frame versus the actual DEV screen, with measured state and no API. */
export async function compareWebScreen({referenceURL, adapter = auth4}) {
  const {referenceSelector, entry, actualSelector, viewport} = adapter;
  if (!process.env.TMPDIR || !process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH) throw new Error('Owned runner scratch and explicit Chromium required');
  if (!entry.startsWith('/src/dev/entries/') && !entry.startsWith('/test/')) throw new Error('An explicit controlled DEV entry is required');
  const vite = await createServer({root: join(root, 'web-next'), configFile: join(root, 'web-next/vite.config.ts'),
    cacheDir: join(process.env.TMPDIR, 'fidelity-vite'), server: {host: '127.0.0.1', port: 0, strictPort: false}});
  try {
    await vite.listen();
    const actualOrigin = `http://127.0.0.1:${vite.httpServer.address().port}`, actualURL = actualOrigin + entry;
    return await withMeasurementBrowser({scratch: process.env.TMPDIR, executable: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH,
      viewport, allowedOrigins: [new URL(referenceURL).origin, actualOrigin, 'https://fonts.googleapis.com', 'https://fonts.gstatic.com']}, async browser => {
      await browser.navigate(referenceURL, referenceSelector);
      const reference = await browser.measure(referenceSelector, adapter.reference);
      const deterministicReference = JSON.stringify(reference) === JSON.stringify(await browser.measure(referenceSelector, adapter.reference));
      await browser.navigate(actualURL, actualSelector);
      const point = await browser.evaluate('(()=>{const e=document.querySelector("input[name=password]");const r=e.getBoundingClientRect();return{x:r.x+r.width/2,y:r.y+r.height/2}})()');
      await browser.call('Input.dispatchMouseEvent', {type: 'mousePressed', button: 'left', clickCount: 1, ...point});
      await browser.call('Input.dispatchMouseEvent', {type: 'mouseReleased', button: 'left', clickCount: 1, ...point});
      // Explicit PUBLIC fixture state: match the frame's ten masked characters.
      // The inert existing DEV recorder cannot send this value to an auth API.
      await browser.call('Input.insertText', {text: '0123456789'});
      const actual = await browser.measure(actualSelector, adapter.actual);
      const deterministicActual = JSON.stringify(actual) === JSON.stringify(await browser.measure(actualSelector, adapter.actual));
      const comparison = compareScreens(reference, actual, adapter), strings = compareCatalogueStrings(reference, actual, enUS, adapter), textMetrics = compareTextMetrics(reference, actual);
      const differences = [...comparison.differences, ...strings.differences];
      const password = actual.elements.find(node => node.frame === 'password-input');
      if (!password?.focused || password.input?.length !== 10) differences.push({kind: 'fixture-state', label: 'Real password focus and ten masked characters', focused: password?.focused, length: password?.input?.length});
      if (!deterministicReference || !deterministicActual) differences.push({kind: 'nondeterministic-extraction', label: 'Same-input determinism', reference: deterministicReference, actual: deterministicActual});
      if (browser.blocked.length || browser.pageErrors.length) differences.push({kind: 'page-readiness', label: 'Blocked or failed runtime resource', blocked: browser.blocked, pageErrors: browser.pageErrors});
      const caretColor = password && colorDifference(password.style.color, password.style.caretColor);
      const nativeCaret = {ruling: 27, focused: Boolean(password?.focused && actual.documentFocused),
        collapsedSelection: password?.input?.selectionStart === 10 && password?.input?.selectionEnd === 10,
        colorFollowsText: Boolean(caretColor && caretColor.deltaE2000 <= 2 && caretColor.alpha <= .02 && parseColor(password.style.caretColor)?.[3] > 0),
        qualification: 'Native caret existence/focus, collapsed insertion point and CSS colour; platform governs x/width/blink. No pixel or blink-timing claim.'};
      if (!nativeCaret.focused || !nativeCaret.collapsedSelection || !nativeCaret.colorFollowsText) differences.push({kind: 'native-caret-state', label: 'Native input caret', ...nativeCaret});
      comparison.diagnostics.push({label: 'Native input caret illustration', scored: false, ruling: 27, properties: ['x', 'width', 'blink'], reason: 'Binding native-input illustration erratum; no fake caret or input metric compensation.'});
      return {version: 1, id: adapter.id, state: adapter.state, scope: 'screen', viewport, reference, actual,
        comparison, strings, textMetrics, nativeCaret, differences, differenceCount: differences.length,
        deterministic: {reference: deterministicReference, actual: deterministicActual},
        network: {blocked: browser.blocked}, pageErrors: browser.pageErrors,
        verdict: differences.length ? 'FAIL' : 'PASS', qualification: 'Controlled genuine full-screen state, not a complete authentication flow. Artwork contents excluded; image slot geometry/style included. No screenshot files or private source URLs.'};
    });
  } finally {await vite.close();}
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const argv = process.argv.slice(2), args = {};
  for (let index = 0; index < argv.length; index += 2) {
    if (!argv[index].startsWith('--') || argv[index + 1] === undefined) throw new Error('Named argument/value pairs required');
    const name = argv[index].slice(2); if (name in args) throw new Error('Duplicate argument'); args[name] = argv[index + 1];
  }
  if (!args['reference-url'] || Object.keys(args).some(key => key !== 'reference-url')) throw new Error('Usage: run-web.mjs --reference-url EXTERNAL_READ_ONLY_REFERENCE');
  const result = await compareWebScreen({referenceURL: args['reference-url']});
  console.log('FIDELITY_SCREEN_JSON ' + JSON.stringify(result));
}
