// Controlled production components vs untouched approved auth20b/20c columns.
// This is presentation evidence, not a reset grant, server fixture or flow verdict.
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {createServer as createHTTP} from 'node:http';
import {readFile, writeFile, mkdir, mkdtemp, rm} from 'node:fs/promises';
import path from 'node:path';
import {createServer as createVite} from 'vite';

const root = process.cwd(), output = process.env.PORTICO_RUNNER_OUTPUT;
const referenceRoot = path.join(process.env.PORTICO_RUNNER_SECONDARY_SOURCE ?? '', 'design/ui-v1');
assert.ok(output && process.env.PORTICO_RUNNER_SECONDARY_SOURCE, 'Exact paired source and runner output are required');
const scratch = await mkdtemp(path.join(process.env.TMPDIR, 'owner-reset-'));
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
let vite, reference, chrome, page, browser, chromeLog = '';
const exceptions = [];
async function connect(url) {
  const socket = new WebSocket(url);await new Promise((resolve, reject) => {socket.addEventListener('open', resolve, {once: true});socket.addEventListener('error', reject, {once: true});});
  let sequence = 0;const pending = new Map();
  socket.addEventListener('message', event => {
    const message = JSON.parse(event.data), reply = pending.get(message.id);
    if (reply) {pending.delete(message.id);clearTimeout(reply.timer);message.error ? reply.reject(Error(JSON.stringify(message.error))) : reply.resolve(message.result);}
    if (message.method === 'Runtime.exceptionThrown') exceptions.push(message.params.exceptionDetails);
  });
  return {send(method, params = {}) {return new Promise((resolve, reject) => {const id = ++sequence, timer = setTimeout(() => {pending.delete(id);reject(Error(`CDP timeout: ${method}`));}, 15000);pending.set(id, {resolve, reject, timer});socket.send(JSON.stringify({id, method, params}));});}, close() {socket.close();}};
}
async function evaluate(expression) {
  const result = await page.send('Runtime.evaluate', {expression, awaitPromise: true, returnByValue: true});
  if (result.exceptionDetails) throw Error(JSON.stringify(result.exceptionDetails));
  return result.result.value;
}
async function waitFor(expression) {
  const until = Date.now() + 15000;
  do {if (await evaluate(expression)) return;await pause(60);} while (Date.now() < until);
  throw Error(`Missing browser state: ${expression}`);
}
async function navigate(url, selector) {
  await page.send('Page.navigate', {url});await waitFor(`location.href===${JSON.stringify(url)} && !!document.querySelector(${JSON.stringify(selector)})`);
  await evaluate('document.fonts.ready.then(()=>true)');
}
async function screenshot(clip) {return (await page.send('Page.captureScreenshot', {format: 'jpeg', quality: 95, captureBeyondViewport: true, clip: {...clip, scale: 1}})).data;}
const metrics = (selector, parent) => evaluate(`(() => {const node=document.querySelector(${JSON.stringify(selector)}), r=node.getBoundingClientRect(), p=document.querySelector(${JSON.stringify(parent)}).getBoundingClientRect(), s=getComputedStyle(node);return {text:node.textContent,x:r.x-p.x,y:r.y-p.y,width:r.width,height:r.height,font:s.fontFamily,size:s.fontSize,weight:s.fontWeight,lineHeight:s.lineHeight};})()`);
try {
  vite = await createVite({root: path.join(root, 'web-next'), configFile: path.join(root, 'web-next/vite.config.ts'), server: {host: '127.0.0.1', port: 0, strictPort: false}});
  await vite.listen();const origin = `http://127.0.0.1:${vite.httpServer.address().port}`;
  const mime = {'.html': 'text/html', '.css': 'text/css', '.js': 'text/javascript', '.svg': 'image/svg+xml', '.woff2': 'font/woff2', '.png': 'image/png', '.jpg': 'image/jpeg'};
  reference = createHTTP(async (req, res) => {
    try {const pathname = decodeURIComponent(new URL(req.url, 'http://127.0.0.1').pathname), file = path.resolve(referenceRoot, '.' + pathname);if (!file.startsWith(referenceRoot + path.sep)) throw Error('Invalid reference path');const bytes = await readFile(file);res.setHeader('Content-Type', mime[path.extname(file)] ?? 'application/octet-stream');res.end(bytes);} catch {res.writeHead(404);res.end();}
  });
  await new Promise(resolve => reference.listen(0, '127.0.0.1', resolve));const referenceOrigin = `http://127.0.0.1:${reference.address().port}`;
  const profile = path.join(scratch, 'profile');await mkdir(profile);
  chrome = spawn(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ?? '/usr/bin/chromium', ['--headless=new', '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage', '--disable-background-networking', '--no-first-run', '--hide-scrollbars', '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank'], {stdio: ['ignore', 'ignore', 'pipe']});
  chrome.stderr.on('data', data => {chromeLog = (chromeLog + data).slice(-16000);});
  let port;for (let n = 0; n < 180 && chrome.exitCode === null; n++) {try {port = (await readFile(path.join(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0];break;} catch {await pause(50);}}
  assert.ok(port, chromeLog);
  const version = await (await fetch(`http://127.0.0.1:${port}/json/version`)).json();browser = await connect(version.webSocketDebuggerUrl);
  const target = await (await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, {method: 'PUT'})).json();page = await connect(target.webSocketDebuggerUrl);
  await page.send('Page.enable');await page.send('Runtime.enable');await page.send('Page.bringToFront');
  await page.send('Emulation.setDeviceMetricsOverride', {width: 1440, height: 900, deviceScaleFactor: 1, mobile: false});
  console.log(JSON.stringify({browser: version.Browser, scope: 'Controlled desktop auth columns, reference left/build right,720x900 each; no backend/reset grant/full-screen acceptance'}));
  for (const frame of ['20b', '20c']) {
    const ref = `.frame[data-f="${frame}"] .screen`;
    await navigate(`${referenceOrigin}/auth.html?frame=${frame}`, ref);
    await evaluate(`document.querySelector(${JSON.stringify(ref)}).scrollIntoView({block:'start',inline:'start'})`);
    await waitFor(`document.querySelector(${JSON.stringify(ref)}).getBoundingClientRect().width===1440`);
    const bounds = await evaluate(`(() => {const r=document.querySelector(${JSON.stringify(ref)}).getBoundingClientRect();return {x:r.x+scrollX,y:r.y+scrollY};})()`);
    const referenceMetrics = await metrics(`${ref} .au h1`, ref);
    const left = await screenshot({...bounds, width: 720, height: 900});
    await navigate(`${origin}/src/dev/entries/accounts.html?frame=${frame}`, '[data-account-specimen] h1');
    if (frame === '20c') {
      // Thirteen real input characters match the frame's bullet count. The
      // canonical strength result remains real, even if that differs from Strong.
      for (const selector of ['input[name="new-password"]', 'input[name="confirm-password"]']) {
        await evaluate(`document.querySelector(${JSON.stringify(selector)}).focus()`);await page.send('Input.insertText', {text: 'drawAB123!xyz'});
      }
      await evaluate('document.querySelector(\'input[name="new-password"]\').focus()');
    }
    await pause(100);
    const buildMetrics = await metrics('[data-auth-content] h1', '[data-account-specimen]');
    const strength = frame === '20c' ? await evaluate('({bars:document.querySelector("[data-level]")?.dataset.level,label:document.querySelector("[data-level]")?.textContent,inputCharacters:document.querySelector(\'input[name="new-password"]\').value.length})') : undefined;
    const right = await screenshot({x: 0, y: 0, width: 720, height: 900});
    const pair = `<!doctype html><html><head><style>*{box-sizing:border-box}html,body{margin:0;width:1440px;height:900px;overflow:hidden}body{display:flex}img{display:block;width:720px;height:900px;flex:none}</style></head><body><img src="data:image/jpeg;base64,${left}"><img src="data:image/jpeg;base64,${right}"></body></html>`;
    await evaluate(`document.open();document.write(${JSON.stringify(pair)});document.close()`);
    await waitFor('[...document.images].every(image=>image.complete && image.naturalWidth===720)');
    await writeFile(path.join(output, `owner-${frame}-pair.jpg`), Buffer.from(await screenshot({x: 0, y: 0, width: 1440, height: 900}), 'base64'));
    console.log(JSON.stringify({frame, reference: referenceMetrics, build: buildMetrics, strength}));
  }
  assert.equal(exceptions.length, 0, JSON.stringify(exceptions));
} finally {
  try {await browser?.send('Browser.close');} catch {}
  page?.close();browser?.close();
  if (chrome && chrome.exitCode === null) {const exited = new Promise(resolve => chrome.once('exit', resolve));chrome.kill('SIGTERM');await Promise.race([exited, pause(3000)]);if (chrome.exitCode === null) chrome.kill('SIGKILL');}
  await vite?.close();if (reference) await new Promise(resolve => reference.close(resolve));await rm(scratch, {recursive: true, force: true});
}
