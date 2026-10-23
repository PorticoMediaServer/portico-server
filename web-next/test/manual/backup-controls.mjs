// Controlled UI regression: actual components/parsers, no server files or authentication.
// Run from the repository root in an isolated remote job after the declared install.
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {mkdir, mkdtemp, readFile, rm} from 'node:fs/promises';
import path from 'node:path';
import {createServer} from 'vite';

const root = process.cwd(), scratch = await mkdtemp(path.join(root, '.backup-controls-'));
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
let server, child, browser, page, chromeLog = '';
const results = [], exceptions = [];
async function connect(url) {
  const socket = new WebSocket(url);
  await new Promise((resolve, reject) => {socket.addEventListener('open', resolve, {once: true});socket.addEventListener('error', reject, {once: true});});
  let sequence = 0;const pending = new Map();
  socket.addEventListener('message', event => {
    const message = JSON.parse(event.data), reply = pending.get(message.id);
    if (reply) {pending.delete(message.id);clearTimeout(reply.timer);message.error ? reply.reject(Error(JSON.stringify(message.error))) : reply.resolve(message.result);}
    if (message.method === 'Runtime.exceptionThrown') exceptions.push(message.params.exceptionDetails);
  });
  return {send(method, params = {}) {return new Promise((resolve, reject) => {const id = ++sequence, timer = setTimeout(() => {pending.delete(id);reject(Error(`CDP timeout: ${method}`));}, 10000);pending.set(id, {resolve, reject, timer});socket.send(JSON.stringify({id, method, params}));});}, close() {socket.close();}};
}
async function evaluate(expression) {
  const result = await page.send('Runtime.evaluate', {expression, awaitPromise: true, returnByValue: true});
  if (result.exceptionDetails) throw Error(JSON.stringify(result.exceptionDetails));
  return result.result.value;
}
async function waitFor(expression) {
  const until = Date.now() + 8000;
  do {if (await evaluate(expression)) return;await pause(25);} while (Date.now() < until);
  throw Error(`Missing state: ${expression}`);
}
const click = (text, scope = 'document') => evaluate(`(() => {const button=[...${scope}.querySelectorAll('button')].find(button=>button.textContent.trim()===${JSON.stringify(text)});if(!button||button.disabled)throw Error('Missing enabled button');button.click();})()`);
const proof = () => evaluate('structuredClone(window.backupPickerProof)');
async function check(name, run) {await run();results.push({name, pass: true});}
async function fill(value, index = 0) {await evaluate(`(() => {const input=document.querySelectorAll('[role=dialog] input')[${index}];Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set.call(input,${JSON.stringify(value)});input.dispatchEvent(new Event('input',{bubbles:true}));})()`);await pause(20);}
try {
  server = await createServer({root: path.join(root, 'web-next'), configFile: path.join(root, 'web-next/vite.config.ts'), cacheDir: path.join(scratch, 'vite'), server: {host: '127.0.0.1', port: 0, strictPort: false, watch: {ignored: ['**/.backup-controls-*/**']}}});
  await server.listen();const origin = `http://127.0.0.1:${server.httpServer.address().port}`;
  const profile = path.join(scratch, 'profile');await mkdir(profile);
  child = spawn(process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ?? '/usr/bin/chromium', ['--headless=new', '--no-sandbox', '--disable-gpu', '--disable-dev-shm-usage', '--disable-background-networking', '--no-first-run', '--hide-scrollbars', '--remote-debugging-port=0', `--user-data-dir=${profile}`, `--disk-cache-dir=${path.join(scratch, 'cache')}`, 'about:blank'], {env: {...process.env, TMPDIR: scratch}, stdio: ['ignore', 'ignore', 'pipe']});
  child.stderr.on('data', bytes => {chromeLog = (chromeLog + bytes).slice(-16000);});
  let port;
  for (let attempt = 0; attempt < 200 && child.exitCode === null; attempt++) {try {port = (await readFile(path.join(profile, 'DevToolsActivePort'), 'utf8')).split('\n')[0];break;} catch {await pause(50);}}
  assert.ok(port, `Chromium started; exit=${child.exitCode}; stderr=${chromeLog}`);
  const version = await (await fetch(`http://127.0.0.1:${port}/json/version`)).json();browser = await connect(version.webSocketDebuggerUrl);
  const target = await (await fetch(`http://127.0.0.1:${port}/json/new?about:blank`, {method: 'PUT'})).json();page = await connect(target.webSocketDebuggerUrl);
  await page.send('Page.enable');await page.send('Runtime.enable');await page.send('Emulation.setDeviceMetricsOverride', {width: 1440, height: 1000, deviceScaleFactor: 1, mobile: false});
  const navigate = async query => {await page.send('Page.navigate', {url: `${origin}/test/backup-picker.html${query}`});await waitFor('!!window.backupPickerProof');};
  await navigate('');await waitFor("!!document.querySelector('[aria-pressed]')");
  await check('bounded actual filesystem request and offered file eligibility', async () => {
    const state = await evaluate("({requests:backupPickerProof.requests,labels:[...document.querySelectorAll('[aria-pressed]')].map(b=>({text:b.textContent,disabled:b.disabled})),body:document.querySelector('[role=dialog]').textContent})");
    assert.ok(state.requests.every(r => new URL(r.path, locationFallback()).searchParams.get('includeFiles') === '1'));
    assert.equal(state.body.includes('unrelated.txt'), false);assert.equal(state.body.includes('unknown.db'), false);
    assert.ok(state.labels.find(r => r.text.startsWith('link.db')).disabled);assert.ok(state.labels.find(r => r.text.startsWith('unreadable.db')).disabled);
  });
  function locationFallback() {return 'http://127.0.0.1';}
  await check('explicit encrypted archive selection remains a path, never a protection inference', async () => {
    await evaluate("[...document.querySelectorAll('[aria-pressed]')].find(b=>b.textContent.startsWith('protected.pba')).click()");await click('Use this file or folder');await waitFor("!document.querySelector('[role=dialog]')");
    assert.deepEqual((await proof()).choices, [['/mnt/media/Backups/protected.pba']]);
  });
  await check('folder navigation and single current-folder selection', async () => {
    await click('Reopen fixture');await waitFor("!![...document.querySelectorAll('[role=dialog] button')].find(b=>b.textContent==='old')");await click('old', "document.querySelector('[role=dialog]')");await waitFor("document.querySelector('[role=dialog]').textContent.includes('older.PBA')");await click('Use this file or folder');await waitFor("!document.querySelector('[role=dialog]')");assert.deepEqual((await proof()).choices.at(-1), ['/mnt/media/Backups/old']);
  });
  await check('late continuation cannot publish or choose after cancel', async () => {
    await click('Reopen fixture');await waitFor("!![...document.querySelectorAll('button')].find(b=>b.textContent==='Show more')");await click('Show more');await waitFor("backupPickerProof.requests.at(-1).path.includes('cursor=')");
    assert.equal(await evaluate("[...document.querySelectorAll('button')].find(b=>b.textContent==='Use this file or folder').disabled"), true);await click('Cancel');await evaluate('releaseBackupPage()');await waitFor("!document.querySelector('[role=dialog]')");assert.equal((await proof()).requests.at(-1).aborted, true);assert.equal((await proof()).choices.length, 2);
  });
  await navigate('?purpose=folder');await waitFor("!![...document.querySelectorAll('[role=dialog] button')].find(b=>b.textContent==='old')");
  await check('default explorer stays folders-only', async () => {const state = await evaluate("({text:document.querySelector('[role=dialog]').textContent,requests:backupPickerProof.requests})");assert.equal(state.text.includes('plain.db'), false);assert.ok(state.requests.every(r => !r.path.includes('includeFiles=')));});
  await navigate('?list=1');await waitFor("!![...document.querySelectorAll('button')].find(b=>b.textContent==='Restore this backup')");
  await check('list omits retired private paths and columns and delegates actual target', async () => {const text = await evaluate('document.body.textContent');assert.equal(text.includes('/private-not-drawn'), false);assert.equal(text.includes('Schema 41'), false);await click('Restore this backup');assert.deepEqual((await proof()).restoreOffers, ['newest']);});
  await check('retired authority synchronously erases list facts', async () => {await evaluate('retireBackupAuthority()');await waitFor("!document.body.textContent.includes('Scheduled')");assert.equal((await proof()).restoreOffers.length, 1);});
  await navigate('?confirm=delete');await waitFor("!!document.querySelector('[role=dialog] input')");
  await check('delete requires exact word and duplicate submit is refused while pending', async () => {
    assert.equal(await evaluate("[...document.querySelectorAll('button')].find(b=>b.textContent==='Delete backup').disabled"), true);await fill('DELETE');assert.equal(await evaluate("[...document.querySelectorAll('button')].find(b=>b.textContent==='Delete backup').disabled"), true);await fill('delete');await waitFor("![...document.querySelectorAll('button')].find(b=>b.textContent==='Delete backup').disabled");
    await evaluate("(()=>{const b=[...document.querySelectorAll('button')].find(b=>b.textContent==='Delete backup');b.click();b.click();})()");assert.equal((await proof()).confirmations.length, 1);assert.equal(await evaluate("document.querySelector('[role=dialog] input').value"), '');assert.equal(await evaluate("!!document.querySelector('[role=dialog]')"), true);
    await evaluate('completeBackupConfirmation()');await waitFor("!document.querySelector('[role=dialog]')");assert.equal((await proof()).failures, 0);
  });
  await navigate('?confirm=restore');await waitFor("!![...document.querySelectorAll('button')].find(b=>b.textContent==='Restore backup')");
  await check('restore confirmation retains original command until completion and aborts on retirement', async () => {
    assert.equal(await evaluate('document.activeElement.textContent'), 'Cancel');await click('Restore backup');assert.equal((await proof()).confirmations.length, 1);await evaluate('retireBackupAuthority()');await waitFor("!document.querySelector('[role=dialog]')");assert.equal((await proof()).confirmations[0].aborted, true);await evaluate('completeBackupConfirmation()');assert.equal((await proof()).closes, 0);
  });
  await navigate('?passphrase=protect');await waitFor("document.querySelectorAll('[role=dialog] input').length===2");
  await check('passphrase fields clear before submit and authority retirement aborts without late callbacks', async () => {
    await fill('controlled test phrase');await fill('controlled test phrase', 1);await click('Protect backups');
    assert.equal((await proof()).passphrases.length, 1);assert.deepEqual(await evaluate("[...document.querySelectorAll('[role=dialog] input')].map(input=>input.value)"), ['', '']);
    await evaluate('retireBackupAuthority()');await waitFor("!document.querySelector('[role=dialog]')");assert.equal((await proof()).passphrases[0].aborted, true);
    await evaluate("completeBackupPassphrase('done')");await pause(30);const state = await proof();assert.equal(state.completed, 0);assert.equal(state.closes, 0);assert.equal(state.failures, 0);
  });
  await navigate('?passphrase=restore');await waitFor("document.querySelectorAll('[role=dialog] input').length===1");
  await check('wrong passphrase stays empty for retry and cancellation retires the next request', async () => {
    await fill('controlled test phrase');await click('Restore');await evaluate("completeBackupPassphrase('wrong-passphrase')");
    await waitFor("document.querySelector('[role=dialog]').textContent.includes('That passphrase doesn’t open this backup.')");assert.equal(await evaluate("document.querySelector('[role=dialog] input').value"), '');
    await fill('another test phrase');await click('Restore');assert.equal((await proof()).passphrases.length, 2);await click('Cancel');await waitFor("!document.querySelector('[role=dialog]')");assert.equal((await proof()).passphrases[1].aborted, true);
    await evaluate("completeBackupPassphrase('done')");await pause(30);assert.equal((await proof()).completed, 0);
  });
  assert.deepEqual(exceptions, []);console.log(JSON.stringify({browser: version.Browser, kind: 'Controlled component/parser behavior only; no live backup or UI/frame acceptance', results}, null, 2));
} finally {
  page?.close();try {await browser?.send('Browser.close');} catch {}browser?.close();if (child?.exitCode === null) child.kill('SIGTERM');await server?.close();await rm(scratch, {recursive: true, force: true});
}
