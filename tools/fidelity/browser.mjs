import {spawn} from 'node:child_process';
import {once} from 'node:events';
import {mkdtemp, rm} from 'node:fs/promises';
import {join} from 'node:path';
import {measureScreen} from './measure-screen.mjs';
import {decodePNG} from './png.mjs';

/** An owned, isolated measurement browser. No cookies, auth state, or screenshots. */
export async function withMeasurementBrowser({scratch, executable, viewport, allowedOrigins}, run) {
  if (!scratch || !executable) throw new Error('Explicit owned scratch and Chromium executable required');
  const allowed = new Set(allowedOrigins.map(value => new URL(value).origin));
  const profile = await mkdtemp(join(scratch, 'fidelity-browser-'));
  const chrome = spawn(executable, ['--headless=new', '--no-sandbox', '--disable-dev-shm-usage', '--disable-gpu',
    '--renderer-process-limit=1', '--no-first-run', '--no-default-browser-check', '--remote-debugging-address=127.0.0.1',
    '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe'], detached: true});
  let socket, session, sequence = 0;
  const pending = new Map(), blocked = [], pageErrors = [];
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
    const endpoint = await new Promise((resolve, reject) => {
      let stderr = '';
      const timer = setTimeout(() => reject(new Error('Measurement Chromium startup timeout')), 15_000);
      chrome.once('error', error => {clearTimeout(timer); reject(error);});
      chrome.once('exit', code => {clearTimeout(timer); reject(new Error(`Measurement Chromium exited ${code}`));});
      chrome.stderr.on('data', bytes => {
        stderr = (stderr + bytes.toString()).slice(-8192);
        const match = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/);
        if (match) {clearTimeout(timer); resolve(match[1]);}
      });
    });
    socket = new WebSocket(endpoint); await once(socket, 'open');
    socket.addEventListener('message', async event => {
      const message = JSON.parse(event.data);
      if (message.method === 'Runtime.exceptionThrown') pageErrors.push(message.params.exceptionDetails.text);
      if (message.method === 'Fetch.requestPaused') {
        const {requestId, request} = message.params, url = new URL(request.url);
        const permit = (url.protocol === 'data:' || allowed.has(url.origin)) && !url.pathname.startsWith('/v1/');
        if (!permit) blocked.push({origin: url.origin, path: url.pathname, method: request.method});
        try {await call(permit ? 'Fetch.continueRequest' : 'Fetch.failRequest', permit ? {requestId} : {requestId, errorReason: 'BlockedByClient'}, message.sessionId);}
        catch (error) {pageErrors.push(error.message);}
      }
      const task = pending.get(message.id);
      if (!task) return;
      pending.delete(message.id); clearTimeout(task.timer);
      if (message.error) task.reject(new Error(`${message.error.code}: ${message.error.message}`)); else task.resolve(message.result);
    });
    const {targetId} = await call('Target.createTarget', {url: 'about:blank'}, null);
    session = (await call('Target.attachToTarget', {targetId, flatten: true}, null)).sessionId;
    await call('Page.enable'); await call('Runtime.enable');
    await call('Fetch.enable', {patterns: [{urlPattern: '*'}]});
    await call('Emulation.setDeviceMetricsOverride', {...viewport, mobile: false});
    return await run({
      async navigate(url, selector) {
        if (!allowed.has(new URL(url).origin)) throw new Error('Measurement navigation origin is not allowed');
        const response = await call('Page.navigate', {url});
        if (response.errorText) throw new Error('Measurement navigation failed');
        await evaluate(`new Promise((resolve,reject)=>{const end=performance.now()+15000;const check=()=>{if(document.querySelector(${JSON.stringify(selector)}))document.fonts.ready.then(resolve);else if(performance.now()>end)reject(new Error('Screen root did not mount'));else requestAnimationFrame(check)};check()})`);
      },
      measure: selector => evaluate(`(${measureScreen.toString()})(${JSON.stringify({selector, expectedViewport: viewport})})`),
      async raster(selector, {referenceFrame = false} = {}) {
        const rect = await evaluate(`(()=>{const root=document.querySelector(${JSON.stringify(selector)});if(!root)throw new Error('Missing screen');const r=root.getBoundingClientRect();return{x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,viewportX:r.x,viewportY:r.y}})()`);
        if (rect.width !== viewport.width || rect.height !== viewport.height || (!referenceFrame && (rect.viewportX !== 0 || rect.viewportY !== 0))) throw new Error('Only the complete declared screen can be compared');
        const result = await call('Page.captureScreenshot', {format: 'png', captureBeyondViewport: referenceFrame, fromSurface: true,
          clip: {x: rect.x, y: rect.y, width: rect.width, height: rect.height, scale: 1}});
        const pixels = decodePNG(Buffer.from(result.data, 'base64'));
        if (pixels.width !== viewport.width * viewport.deviceScaleFactor || pixels.height !== viewport.height * viewport.deviceScaleFactor) throw new Error('Raster does not match declared viewport and DPR');
        return pixels;
      },
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
    await rm(profile, {recursive: true, force: true});
  }
}
