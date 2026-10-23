// Actual built Services + cmd/web + fresh cmd/hosted integration. No accounts,
// sign-in, mail, provider, owner, frame or physical-device acceptance is implied.
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {readFileSync, mkdtempSync, rmSync} from 'node:fs';
import net from 'node:net';
import path from 'node:path';
import {pathToFileURL} from 'node:url';

const repository = process.env.PORTICO_SERVER_REPO, scratch = process.env.TMPDIR;
assert(repository && path.isAbsolute(repository) && scratch && path.isAbsolute(scratch));
const {ChromeDriver} = await import(pathToFileURL(path.join(repository, 'web/test/integration/chrome-driver.mjs')));
const origin = 'http://127.0.0.1:19495', api = 'http://127.0.0.1:19410';
const browser = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;
assert(browser && path.isAbsolute(browser));
const profile = mkdtempSync(path.join(scratch, 'services-boot-browser-'));
const socket = net.createServer(); await new Promise(resolve => socket.listen(0, '127.0.0.1', resolve));
const debugPort = socket.address().port; await new Promise(resolve => socket.close(resolve));
const rss = pid => {
  try {
    return Number(readFileSync(`/proc/${pid}/status`, 'utf8').match(/^VmRSS:\s+(\d+)/m)?.[1] ?? 0) +
      readFileSync(`/proc/${pid}/task/${pid}/children`, 'utf8').trim().split(/\s+/).filter(Boolean).reduce((sum, child) => sum + rss(child), 0);
  } catch { return 0; }
};
let peakKiB = 0, chrome, driver;
const timer = setInterval(() => { peakKiB = Math.max(peakKiB, rss(process.pid)); }, 200);
try {
  const options = await fetch(api + '/v1/identity/options', {headers: {Origin: origin}});
  assert.equal(options.status, 200);
  assert.equal(options.headers.get('access-control-allow-origin'), origin);
  const facts = await options.json();
  assert.equal(facts.password, true); assert.equal(facts.registration, false); assert.equal(facts.recovery, false);
  assert(facts.providers.every(provider => !provider.enabled));
  const initial = await fetch(origin + '/', {headers: {Accept: 'text/html'}}), html = await initial.text();
  assert.equal(initial.status, 200); assert.equal(initial.headers.get('x-content-type-options'), 'nosniff');
  const scripts = [...html.matchAll(/<script\b([^>]*)>/g)].map(match => match[1]);
  assert(scripts.some(tag => /src=["']\/browser-check\.js["']/.test(tag) && !/\b(?:async|defer|type)\s*(?:=|\s|$)/.test(tag)));
  const assets = [...new Set([...html.matchAll(/(?:src|href)=["'](\/assets\/[^"']+|\/browser-check\.js)["']/g)].map(match => match[1]))];
  assert(assets.some(asset => asset.endsWith('.js')) && assets.includes('/browser-check.js'));
  for (const asset of assets) {
    const response = await fetch(origin + asset);
    assert.equal(response.status, 200); assert.equal(response.headers.get('x-content-type-options'), 'nosniff');
    assert(!response.headers.get('content-type')?.startsWith('text/html'));
  }
  chrome = spawn(browser, ['--headless=new', '--no-sandbox', '--disable-dev-shm-usage', '--disable-gpu', '--no-first-run', '--no-default-browser-check', '--disable-background-networking', '--disable-component-update', '--disable-sync', '--metrics-recording-only', '--password-store=basic', '--remote-debugging-address=127.0.0.1', `--remote-debugging-port=${debugPort}`, `--user-data-dir=${profile}`, 'about:blank'], {stdio: 'ignore'});
  let ready = false;
  for (let attempt = 0; attempt < 100; attempt++) {
    try { if ((await fetch(`http://127.0.0.1:${debugPort}/json/version`)).ok) { ready = true; break; } } catch {}
    if (chrome.exitCode !== null) throw new Error('Disposable Chrome exited before readiness.');
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  assert(ready, 'Disposable Chrome readiness deadline.');
  driver = await ChromeDriver.connect(debugPort);
  await driver.viewport(1440, 900);
  const routes = [
    ['/', true], ['/account', true], ['/device', true], ['/reset', false],
    ['/verify-email', false], ['/claim', false], ['/join', false],
  ];
  for (const [route, credentials] of routes) {
    const reload = await fetch(origin + route, {method: 'HEAD', headers: {Accept: 'text/html'}});
    assert.equal(reload.status, 200); assert.equal(await reload.text(), '');
    await driver.navigate(origin + route);
    const condition = credentials ? 'document.querySelector("input[name=username]") && document.querySelector("input[name=password]")' : 'document.querySelector("h1")';
    await driver.waitFor(condition, 30000);
    await driver.send('Page.reload', {ignoreCache: true});
    await driver.waitFor(condition, 30000);
    assert.equal(await driver.evaluate('location.pathname'), route);
    assert.equal(await driver.evaluate('globalThis.isSecureContext && Boolean(crypto.subtle) && Boolean(navigator.locks)'), true);
    assert.equal(await driver.evaluate('Boolean(document.querySelector("a[href^=\\"/libraries\\"], a[href^=\\"/library\\"]"))'), false);
  }
  const allowed = new Set(['/v1/identity/options', '/v1/identity/current', '/v1/registrations/current']);
  const requests = [...driver.requests.values()].map(value => new URL(value));
  assert(requests.some(url => url.origin === api && url.pathname === '/v1/identity/options'));
  for (const url of requests) {
    assert(['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname) || ['data:', 'blob:', 'about:'].includes(url.protocol), 'Unexpected external application request.');
    if (url.pathname.startsWith('/v1/')) assert(url.origin === api && allowed.has(url.pathname), 'Server bootstrap/private-data request from Services.');
  }
  assert.equal(driver.events.filter(event => event.kind === 'exception' || event.kind === 'external-blocked').length, 0);
  console.log('PASS actual modern build/static host + real signed-out API options; browser-check/assets; seven routes and reloads; protected context; zero Server bootstrap/private-data requests. No account/mail/provider/owner/frame acceptance.');
} finally {
  driver?.ws.close(); clearInterval(timer);
  if (chrome && chrome.exitCode === null) {
    chrome.kill('SIGTERM');
    await new Promise(resolve => { const timeout = setTimeout(resolve, 3000); chrome.once('exit', () => { clearTimeout(timeout); resolve(); }); });
    if (chrome.exitCode === null && chrome.signalCode === null) { chrome.kill('SIGKILL'); await new Promise(resolve => chrome.once('exit', resolve)); }
  }
  rmSync(profile, {recursive: true, force: true});
  console.log('Disposable browser/profile stopped; max observed browser-tree RSS KiB', peakKiB);
}
