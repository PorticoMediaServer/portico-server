import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {mkdtemp, rm} from 'node:fs/promises';
import {tmpdir} from 'node:os';
import {join} from 'node:path';
import {measureScreen} from './measure-screen.mjs';
import {observeRenderedFonts} from './resolved-fonts.mjs';
import {fontFamilyMetadata} from './font-family-metadata.mjs';

// Linux Chrome keeps its process-singleton socket at
// <TMPDIR>/com.google.Chrome.XXXXXX/SingletonSocket and aborts with
// "Socket path too long" once that passes the kernel's 107-byte sun_path
// limit. An owned runner scratch is usually deeper than that, so the child
// gets its own short temp directory while its retention profile stays in the
// caller's scratch.
const singletonSocketLimit = 107;
const chromeTempSuffix = join('fidelity-chrome-XXXXXX', 'com.google.Chrome.XXXXXX', 'SingletonSocket').length + 1;
function chromeTempBase(scratch) {
  const base = [scratch, tmpdir(), '/tmp'].find(value => typeof value === 'string' && Buffer.byteLength(value) + chromeTempSuffix <= singletonSocketLimit);
  if (!base) throw new Error('A Chromium temp directory within the process-singleton socket limit is required');
  return base;
}

/** An owned, isolated measurement browser. No cookies, auth state, or screenshots. */
export async function withMeasurementBrowser({scratch, executable, viewport, allowedOrigins}, run) {
  if (!scratch || !executable) throw new Error('Explicit owned scratch and Chromium executable required');
  const allowed = new Set(allowedOrigins.map(value => new URL(value).origin));
  let chromeTemp;
  const profile = await mkdtemp(join(scratch, 'fidelity-browser-'));
  try {
    chromeTemp = await mkdtemp(join(chromeTempBase(scratch), 'fidelity-chrome-'));
  } catch (error) {
    await rm(profile, {recursive: true, force: true});
    throw error;
  }
  const chrome = spawn(executable, ['--headless=new', '--no-sandbox', '--disable-dev-shm-usage', '--disable-gpu',
    '--renderer-process-limit=1', '--no-first-run', '--no-default-browser-check', '--remote-debugging-address=127.0.0.1',
    '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe'], detached: true,
    env: {...process.env, TMPDIR: chromeTemp}});
  let socket, session, sequence = 0;
  const pending = new Map(), blocked = [], pageErrors = [];
  const fontResources = new Map(), fontRegistrations = new Map(); let excessiveFonts = false;
  function call(method, params = {}, sessionId = session) {
    const id = ++sequence;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {pending.delete(id); reject(new Error(`CDP timeout: ${method}`));}, 15_000);
      pending.set(id, {resolve, reject, timer});
      socket.send(JSON.stringify({id, method, params, ...(sessionId ? {sessionId} : {})}));
    });
  }
  async function evaluate(expression) {
    const result = await call('Runtime.evaluate', {expression, awaitPromise: true, returnByValue: true});
    if (result.exceptionDetails) throw new Error(result.exceptionDetails.text || 'Measurement evaluation failed');
    return result.result.value;
  }
  try {
    let stderr = '';
    const endpoint = await new Promise((resolve, reject) => {
      const detail = () => stderr.trim() ? `: ${stderr.trim().slice(-4000)}` : '';
      const timer = setTimeout(() => reject(new Error(`Measurement Chromium startup timeout${detail()}`)), 15_000);
      chrome.once('error', error => {clearTimeout(timer); reject(error);});
      chrome.once('exit', (code, signal) => {clearTimeout(timer); reject(new Error(`Measurement Chromium exited ${code ?? signal}${detail()}`));});
      chrome.stderr.on('data', bytes => {
        stderr = (stderr + bytes.toString()).slice(-8192);
        const match = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/);
        if (match) {clearTimeout(timer); resolve(match[1]);}
      });
    });
    socket = new WebSocket(endpoint); await once(socket, 'open');
    socket.addEventListener('message', async event => {
      const message = JSON.parse(event.data);
      if (message.sessionId === session && message.method === 'CSS.fontsUpdated' && message.params.font) {
        const {fontFamily, platformFontFamily} = message.params.font;
        if (typeof platformFontFamily === 'string' && platformFontFamily.length <= 200 && !/[\r\n/\\]|(?:https?|file):|url\s*\(/i.test(platformFontFamily)) {
          const families = fontRegistrations.get(platformFontFamily) ?? new Set();
          families.add(typeof fontFamily === 'string' && fontFamily.length <= 200 && !/[\r\n/\\]|(?:https?|file):|url\s*\(/i.test(fontFamily) ? fontFamily : null);
          fontRegistrations.set(platformFontFamily, families);
        }
      }
      if (message.sessionId === session && message.method === 'Network.responseReceived' && message.params.type === 'Font') {
        const {requestId, response} = message.params;
        const url = new URL(response.url);
        if ((allowed.has(url.origin) || url.protocol === 'data:') && response.status >= 200 && response.status < 300) {
          if (fontResources.size >= 32) excessiveFonts = true;
          else fontResources.set(requestId, {complete: false});
        }
      }
      if (message.sessionId === session && message.method === 'Network.loadingFinished' && fontResources.has(message.params.requestId)) {
        const resource = fontResources.get(message.params.requestId);
        resource.complete = true; resource.size = message.params.encodedDataLength;
      }
      if (message.sessionId === session && message.method === 'Network.loadingFailed') fontResources.delete(message.params.requestId);
      if (message.method === 'Runtime.exceptionThrown') pageErrors.push({kind: 'page-exception'});
      if (message.method === 'Fetch.requestPaused') {
        const {requestId, request} = message.params, url = new URL(request.url);
        const permit = (url.protocol === 'data:' || allowed.has(url.origin)) && !url.pathname.startsWith('/v1/');
        if (!permit) blocked.push({kind: url.pathname.startsWith('/v1/') ? 'api' : 'outside-origin', method: request.method});
        try {await call(permit ? 'Fetch.continueRequest' : 'Fetch.failRequest', permit ? {requestId} : {requestId, errorReason: 'BlockedByClient'}, message.sessionId);}
        catch {pageErrors.push({kind: 'request-interception-error'});}
      }
      const task = pending.get(message.id);
      if (!task) return;
      pending.delete(message.id); clearTimeout(task.timer);
      if (message.error) task.reject(new Error(`${message.error.code}: ${message.error.message}`)); else task.resolve(message.result);
    });
    const {targetId} = await call('Target.createTarget', {url: 'about:blank'}, null);
    session = (await call('Target.attachToTarget', {targetId, flatten: true}, null)).sessionId;
    await call('Page.enable'); await call('Runtime.enable');
    await call('DOM.enable'); await call('CSS.enable');
    await call('Network.enable', {maxTotalBufferSize: 32 * 1024 * 1024, maxResourceBufferSize: 8 * 1024 * 1024});
    await call('Fetch.enable', {patterns: [{urlPattern: '*'}]});
    await call('Emulation.setDeviceMetricsOverride', {...viewport, mobile: false});
    return await run({
      async navigate(url, selector) {
        if (!allowed.has(new URL(url).origin)) throw new Error('Measurement navigation origin is not allowed');
        fontResources.clear(); fontRegistrations.clear(); excessiveFonts = false;
        const response = await call('Page.navigate', {url});
        if (response.errorText) throw new Error('Measurement navigation failed');
        await evaluate(`new Promise((resolve,reject)=>{const end=performance.now()+15000;const check=()=>{if(document.querySelector(${JSON.stringify(selector)}))document.fonts.ready.then(resolve);else if(performance.now()>end)reject(new Error('Screen root did not mount'));else requestAnimationFrame(check)};check()})`);
      },
      measure: (selector, anchors = {}, repeats = []) => evaluate(`(${measureScreen.toString()})(${JSON.stringify({selector, expectedViewport: viewport, anchors, repeats})})`),
      observeRenderedFonts: (screen, selector) => observeRenderedFonts(call, screen, selector, async () => {
        if (excessiveFonts) return {complete: false, families: []};
        const families = [];
        for (const [requestId, resource] of fontResources) {
          if (!resource.complete || resource.size > 8 * 1024 * 1024) return {complete: false, families: []};
          if (resource.metadata === undefined) {
            try {
              const response = await call('Network.getResponseBody', {requestId});
              if (!response.base64Encoded || response.body.length > 12 * 1024 * 1024) throw new Error('Unbounded font response');
              resource.metadata = fontFamilyMetadata(Buffer.from(response.body, 'base64'));
            } catch {resource.metadata = null;}
          }
          if (!resource.metadata) return {complete: false, families: []};
          families.push(resource.metadata);
        }
        return {complete: true, families, registrations: [...fontRegistrations].flatMap(([platformFamily, aliases]) => [...aliases].map(family => ({platformFamily, family})))};
      }),
      evaluate,
      call,
      blocked,
      pageErrors,
    });
  } finally {
    for (const task of pending.values()) {clearTimeout(task.timer); task.reject(new Error('Measurement browser closed'));}
    pending.clear(); socket?.close();
    if (chrome.pid && chrome.exitCode === null && chrome.signalCode === null) {
      try {process.kill(-chrome.pid, 'SIGTERM');} catch {}
      await Promise.race([once(chrome, 'close').catch(() => {}), new Promise(resolve => setTimeout(resolve, 3000))]);
      if (chrome.exitCode === null && chrome.signalCode === null) try {process.kill(-chrome.pid, 'SIGKILL');} catch {}
    }
    try {await rm(profile, {recursive: true, force: true});}
    finally {await rm(chromeTemp, {recursive: true, force: true});}
  }
}
