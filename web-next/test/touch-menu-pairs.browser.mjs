import {spawn} from 'node:child_process';
import {mkdir, writeFile} from 'node:fs/promises';
import {join} from 'node:path';
import {once} from 'node:events';
import assert from 'node:assert/strict';

// Real Menu/BottomSheet/Popover component pixels, not a production shell or action proof.
// References are served unmodified from an exact checked design pin over an owned tunnel.
const output = process.env.PORTICO_RUNNER_OUTPUT;
const reference = process.env.PORTICO_REFERENCE_ORIGIN;
if (!output || !reference) throw new Error('The runner output and checked reference origin are required.');
await mkdir(output, {recursive: true});
const processes = [];
let browser, target;
const errors = [];
const log = value => process.stdout.write(`${JSON.stringify(value)}\n`);

class CDP {
  constructor(socket) {
    this.socket = socket; this.serial = 0; this.pending = new Map(); this.events = new Map();
    socket.addEventListener('message', event => {
      const value = JSON.parse(event.data);
      if (value.id) {
        const promise = this.pending.get(value.id); this.pending.delete(value.id);
        if (value.error) promise?.reject(new Error(JSON.stringify(value.error))); else promise?.resolve(value.result);
      } else for (const callback of this.events.get(value.method) ?? []) callback(value.params);
    });
  }
  static async open(url) {const socket = new WebSocket(url); await once(socket, 'open'); return new CDP(socket);}
  call(method, params = {}) {return new Promise((resolve, reject) => {const id = ++this.serial; this.pending.set(id, {resolve, reject}); this.socket.send(JSON.stringify({id, method, params}));});}
  on(method, callback) {const list = this.events.get(method) ?? []; list.push(callback); this.events.set(method, list);}
  next(method) {return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`Timed out waiting for ${method}`)), 30_000);
    const callback = value => {clearTimeout(timer); this.events.set(method, (this.events.get(method) ?? []).filter(item => item !== callback)); resolve(value);};
    this.on(method, callback);
  });}
  async evaluate(expression) {
    const result = await this.call('Runtime.evaluate', {expression, awaitPromise: true, returnByValue: true});
    if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails));
    return result.result.value;
  }
  close() {this.socket.close();}
}

const launch = (command, args, marker) => new Promise((resolve, reject) => {
  const child = spawn(command, args, {stdio: ['ignore', 'pipe', 'pipe']}); processes.push(child);
  const timeout = setTimeout(() => reject(new Error(`Startup timed out: ${command}`)), 30_000);
  const observe = chunk => {const text = chunk.toString(); const match = text.match(marker); if (match) {clearTimeout(timeout); resolve({child, match});}};
  child.stdout.on('data', observe); child.stderr.on('data', observe);
  child.on('error', reject); child.on('exit', code => {if (code !== null) {clearTimeout(timeout); reject(new Error(`${command} exited ${code}`));}});
});
const screenshot = async (name, clip) => {
  const result = await target.call('Page.captureScreenshot', {format: 'jpeg', quality: 94, captureBeyondViewport: true, clip});
  await writeFile(join(output, name), Buffer.from(result.data, 'base64'));
  return result.data;
};
const settle = () => target.evaluate(`(async()=>{await document.fonts.ready;await Promise.all([...document.images].map(image=>image.complete?Promise.resolve():new Promise(resolve=>{image.onload=resolve;image.onerror=resolve;})));await new Promise(resolve=>setTimeout(resolve,900));return true})()`);
const navigate = async url => {const loaded = target.next('Page.loadEventFired'); await target.call('Page.navigate', {url}); await loaded; await settle();};
const pause = milliseconds => target.evaluate(`new Promise(resolve=>setTimeout(resolve,${milliseconds}))`);
const primary = () => target.evaluate(`(()=>{const r=document.querySelector('[data-card-kind] > button').getBoundingClientRect();return{x:r.x+r.width/2,y:r.y+r.height/2}})()`);
const start = point => target.call('Input.dispatchTouchEvent', {type: 'touchStart', touchPoints: [{...point, radiusX: 1, radiusY: 1, force: 1}]});
const end = () => target.call('Input.dispatchTouchEvent', {type: 'touchEnd', touchPoints: []});
const state = () => target.evaluate(`(()=>{const e=document.querySelector('[data-component-only]');return{navigations:Number(e.dataset.navigations),menus:Number(e.dataset.menus),toggles:Number(e.dataset.toggles),open:!!document.querySelector('[role=menu],[role=dialog]')}})()`);
const bounds = selector => target.evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(selector)});if(!e)throw new Error('Missing capture element');e.scrollIntoView({block:'center',inline:'center'});const r=e.getBoundingClientRect();return{x:r.x+scrollX,y:r.y+scrollY,width:r.width,height:r.height,scale:1}})()`);
const pair = async (id, referenceImage, buildImage, referenceClip, buildClip) => {
  const width = Math.ceil(Math.max(referenceClip.width, buildClip.width)), height = Math.ceil(Math.max(referenceClip.height, buildClip.height));
  await target.call('Emulation.setDeviceMetricsOverride', {width: width * 2, height, deviceScaleFactor: 1, mobile: false});
  await navigate('about:blank');
  await target.evaluate(`document.body.style.cssText='margin:0;background:#071018;display:flex;align-items:flex-end';document.body.innerHTML=${JSON.stringify(`<img alt="Mock-up" src="data:image/jpeg;base64,${referenceImage}" style="width:${referenceClip.width}px;height:${referenceClip.height}px"><img alt="Build" src="data:image/jpeg;base64,${buildImage}" style="width:${buildClip.width}px;height:${buildClip.height}px">`)};true`);
  await settle(); await screenshot(`${id}-pair.jpg`, {x: 0, y: 0, width: width * 2, height, scale: 1});
};
const measure = selector => target.evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(selector)});const r=e.getBoundingClientRect(),s=getComputedStyle(e);return{rect:{x:r.x,y:r.y,width:r.width,height:r.height},background:s.backgroundColor,padding:s.padding,borderRadius:s.borderRadius,text:e.innerText,rows:[...e.querySelectorAll('button')].map(b=>({text:b.innerText,height:b.getBoundingClientRect().height,font:getComputedStyle(b).font,padding:getComputedStyle(b).padding})),images:[...e.querySelectorAll('img')].map(i=>({complete:i.complete,naturalWidth:i.naturalWidth,rect:{width:i.width,height:i.height}}))}})()`);

try {
  await launch('node', ['node_modules/vite/bin/vite.js', 'web-next', '--config', 'web-next/vite.config.ts', '--host', '127.0.0.1', '--port', '18732', '--strictPort'], /Local:\s+(http:\/\/127\.0\.0\.1:18732\/)/);
  const chrome = await launch(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ?? '/usr/bin/chromium', ['--headless=new', '--no-sandbox', '--disable-dev-shm-usage', '--remote-debugging-address=127.0.0.1', '--remote-debugging-port=0', `--user-data-dir=${join(process.env.TMPDIR, 'q6-chrome')}`, '--no-first-run', 'about:blank'], /DevTools listening on (ws:\/\/[^\s]+)/);
  browser = await CDP.open(chrome.match[1]);
  const created = await browser.call('Target.createTarget', {url: 'about:blank'});
  const port = new URL(chrome.match[1]).port;
  const pages = await (await fetch(`http://127.0.0.1:${port}/json/list`)).json();
  target = await CDP.open(pages.find(page => page.id === created.targetId).webSocketDebuggerUrl);
  target.on('Runtime.exceptionThrown', event => errors.push(event));
  await target.call('Page.enable'); await target.call('Runtime.enable');
  await target.call('Emulation.setFocusEmulationEnabled', {enabled: true});
  await target.call('Emulation.setTouchEmulationEnabled', {enabled: true, maxTouchPoints: 1});
  const cases = [
    {id: 'compact', file: 'select.html', selector: '#s17 .col:first-child .sheet.lp', width: 390, height: 844},
    {id: 'anchored', file: 'tablet.html', selector: '.frame[data-f="9"] .pop', width: 1180, height: 900},
  ];
  const results = [];
  for (const item of cases) {
    await target.call('Emulation.setDeviceMetricsOverride', {width: 2000, height: 1300, deviceScaleFactor: 1, mobile: false});
    await navigate(`${reference}/${item.file}`);
    const referenceFacts = await measure(item.selector), referenceClip = await bounds(item.selector);
    const referenceImage = await screenshot(`${item.id}-reference.jpg`, referenceClip);
    const referencePointerClip = item.id === 'anchored' ? await target.evaluate(`(()=>{const e=document.querySelector(${JSON.stringify(item.selector)}),r=e.getBoundingClientRect(),p=getComputedStyle(e,'::before');return{x:r.x+scrollX-14,y:r.y+scrollY+parseFloat(p.top)+parseFloat(p.height)/2-14,width:28,height:28,scale:1}})()`) : undefined;
    const referencePointer = referencePointerClip && await screenshot('anchored-pointer-reference.jpg', referencePointerClip);
    await target.call('Emulation.setDeviceMetricsOverride', {width: item.width, height: item.height, deviceScaleFactor: 1, mobile: false});
    await navigate(`http://127.0.0.1:18732/test/fixtures/touch-menu.html?case=${item.id}&reference=${encodeURIComponent(reference)}`);
    let point = await primary();
    await start(point); await pause(100); await end(); await settle();
    assert.deepEqual(await state(), {navigations: 1, menus: 0, toggles: 0, open: false});
    point = await primary();
    await start(point); await pause(550); await end();
    await settle();
    assert.deepEqual(await state(), {navigations: 1, menus: 1, toggles: 0, open: true});
    if (item.id === 'compact') {
      const hover = await target.evaluate(`(()=>{const r=[...document.querySelectorAll('[data-menu-item]')].find(e=>e.textContent===${JSON.stringify('Select')}).getBoundingClientRect();return{x:r.x+r.width/2,y:r.y+r.height/2}})()`);
      await target.call('Input.dispatchMouseEvent', {type: 'mouseMoved', ...hover});
    }
    const selector = item.id === 'compact' ? '[role=dialog]' : '[role=menu]';
    const unresolved = await target.evaluate(`(()=>[...document.querySelectorAll(${JSON.stringify(selector)}+' use')].map(e=>e.getAttribute('href')).filter(id=>!document.getElementById(id.slice(1))))()`);
    if (unresolved.length) throw new Error(`Missing document icon symbols: ${unresolved.join(',')}`);
    const buildFacts = await measure(selector), buildClip = await bounds(selector);
    const buildImage = await screenshot(`${item.id}-build.jpg`, buildClip);
    const buildPointerClip = item.id === 'anchored' ? await target.evaluate(`(()=>{const e=document.querySelector('[role=menu]'),r=e.getBoundingClientRect(),p=e.querySelector('[data-touch-menu-pointer]').getBoundingClientRect();return{x:r.x+scrollX-14,y:p.y+scrollY+p.height/2-14,width:28,height:28,scale:1}})()`) : undefined;
    const buildPointer = buildPointerClip && await screenshot('anchored-pointer-build.jpg', buildPointerClip);
    await target.call('Input.dispatchKeyEvent', {type: 'keyDown', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27});
    await target.call('Input.dispatchKeyEvent', {type: 'keyUp', key: 'Escape', code: 'Escape', windowsVirtualKeyCode: 27});
    await settle();
    assert.equal((await state()).open, false);
    assert.equal(await target.evaluate(`document.activeElement===document.querySelector('[data-card-kind] > button')`), true);
    results.push({id: item.id, qualification: 'component-only; exact frame supplied rows, no application/authority/dispatch acceptance', reference: referenceFacts, build: buildFacts, referenceClip, buildClip});
    await pair(item.id, referenceImage, buildImage, referenceClip, buildClip);
    if (referencePointer && buildPointer) await pair('anchored-pointer', referencePointer, buildPointer, referencePointerClip, buildPointerClip);
    await target.call('Emulation.setDeviceMetricsOverride', {width: item.width, height: item.height, deviceScaleFactor: 1, mobile: false});
    const fixture = `http://127.0.0.1:18732/test/fixtures/touch-menu.html?case=${item.id}&reference=${encodeURIComponent(reference)}`;
    await navigate(fixture);
    point = await primary(); await start(point);
    await target.call('Input.dispatchTouchEvent', {type: 'touchMove', touchPoints: [{x: point.x + 6, y: point.y + 8.1, radiusX: 1, radiusY: 1, force: 1}]});
    await pause(550); await end(); await settle();
    assert.deepEqual(await state(), {navigations: 0, menus: 0, toggles: 0, open: false});
    await navigate(`${fixture}&selecting=1`);
    point = await primary(); await start(point); await pause(550); await end(); await settle();
    assert.deepEqual(await state(), {navigations: 0, menus: 0, toggles: 1, open: false});
    log({id: item.id, componentInteractionChecks: ['short tap navigates once', 'hold opens menu without title activation', 'Escape closes and restores genuine opener', 'movement greater than10px cancels', 'selecting hold toggles once without opening menu']});
  }
  log({cases: results, errors});
  if (errors.length) process.exitCode = 1;
} finally {
  target?.close();
  if (browser) {await browser.call('Browser.close').catch(() => {}); browser.close();}
  for (const child of processes.reverse()) if (child.exitCode === null) {child.kill('SIGTERM'); await Promise.race([once(child, 'exit'), new Promise(resolve => setTimeout(resolve, 3000))]); if (child.exitCode === null) child.kill('SIGKILL');}
}
