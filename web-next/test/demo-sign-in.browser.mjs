// Manual campaign smoke: real sign-in, profile switch and sign-out against a
// deployed server's own web build. Credentials arrive once from a loopback
// broker URL and are never written to the report. Dependency-free CDP so the
// runner needs only its provisioned Chromium.
import assert from 'node:assert/strict';
import {spawn} from 'node:child_process';
import {existsSync} from 'node:fs';
import {mkdir, writeFile} from 'node:fs/promises';
import {once} from 'node:events';
import {resolve} from 'node:path';

const output = resolve(process.env.PORTICO_RUNNER_OUTPUT ?? '.');
const origin = process.env.PORTICO_DEMO_ORIGIN ?? 'https://demo.getportico.tv';
const credentialsURL = process.env.PORTICO_SIGNIN_CREDS_URL;
if (!credentialsURL) throw new Error('PORTICO_SIGNIN_CREDS_URL is required');
const credentials = await (await fetch(credentialsURL, {cache: 'no-store'})).json();
if (typeof credentials.username !== 'string' || typeof credentials.password !== 'string') throw new Error('credential broker returned no sign-in pair');
const executable = process.env.PORTICO_CHROMIUM ?? ['/usr/bin/chromium', '/snap/bin/chromium', '/usr/bin/google-chrome'].find(existsSync);
if (!executable) throw new Error('No Chromium executable on this host');
await mkdir(output, {recursive: true});

const steps = [], errors = [], netFailures = [], cspFindings = [], consoleErrors = [], reloads = [];
const step = (name, ok, detail = '') => {steps.push({name, ok, detail}); if (!ok) errors.push(`${name}: ${detail}`);};
const profile = resolve(process.env.TMPDIR ?? output, 'demo-sign-in-profile');
await mkdir(profile, {recursive: true});
const chrome = spawn(executable, ['--headless=new', '--no-sandbox', '--disable-dev-shm-usage', '--no-first-run', '--no-default-browser-check',
  '--remote-debugging-address=127.0.0.1', '--remote-debugging-port=0', `--user-data-dir=${profile}`, 'about:blank'],
  {stdio: ['ignore', 'ignore', 'pipe'], detached: true});
let socket, sequence = 0, session, stderr = '';
const pending = new Map();
function call(method, params = {}, sessionId = session) {
  const id = ++sequence;
  return new Promise((resolveCall, reject) => {
    const timer = setTimeout(() => {pending.delete(id); reject(new Error(`CDP timeout: ${method}`));}, 20000);
    pending.set(id, {resolve: resolveCall, reject, timer});
    socket.send(JSON.stringify({id, method, params, ...(sessionId ? {sessionId} : {})}));
  });
}
async function evaluate(expression) {
  const result = await call('Runtime.evaluate', {expression, awaitPromise: true, returnByValue: true});
  if (result.exceptionDetails) throw new Error(JSON.stringify(result.exceptionDetails.exception?.description ?? result.exceptionDetails.text));
  return result.result.value;
}
const paint = () => evaluate('new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))');
async function waitFor(expression, label, timeout = 20000) {
  const until = Date.now() + timeout;
  while (Date.now() < until) {
    try {if (await evaluate(expression)) return true;} catch {}
    await new Promise(resolveWait => setTimeout(resolveWait, 150));
  }
  throw new Error(`timeout waiting for ${label}`);
}
async function screenshot(name) {
  await new Promise(resolveDwell => setTimeout(resolveDwell, 700));
  await evaluate('Promise.race([Promise.all(document.getAnimations().map(animation => animation.finished)), new Promise(resolveWait => setTimeout(resolveWait, 2500))])').catch(() => {});
  await paint();
  const result = await call('Page.captureScreenshot', {format: 'png'});
  await writeFile(resolve(output, name), Buffer.from(result.data, 'base64'));
}
async function navigate(url) {
  await call('Page.navigate', {url});
  await waitFor('document.readyState === "complete"', `load ${url}`);
  await paint();
}
async function fill(selector, value) {
  const ok = await evaluate(`(()=>{const el=document.querySelector(${JSON.stringify(selector)});if(!el)return false;
    const setter=Object.getOwnPropertyDescriptor(HTMLInputElement.prototype,'value').set;
    setter.call(el,${JSON.stringify(value)});el.dispatchEvent(new Event('input',{bubbles:true}));return true;})()`);
  if (!ok) throw new Error(`missing input ${selector}`);
}
async function clickMenuItem(text) {
  return evaluate(`(()=>{const items=[...document.querySelectorAll('[role="menuitem"]')];
    const target=items.find(item=>(item.innerText||'').trim().includes(${JSON.stringify(text)}) && !/^Sign out/.test((item.innerText||'').trim()));
    if(!target)return false;target.click();return true;})()`);
}
async function bodyState() {
  return evaluate(`(()=>({url:location.pathname+location.search, heading:[...document.querySelectorAll('h1,h2')].map(h=>h.innerText.trim()).filter(Boolean).slice(0,4),
    buttons:[...document.querySelectorAll('button')].map(b=>(b.innerText||b.getAttribute('aria-label')||'').trim()).filter(Boolean).slice(0,24),
    alerts:[...document.querySelectorAll('[role="alert"]')].map(a=>a.innerText.trim()).filter(Boolean)}))()`);
}
async function signInOnce(label, viewport) {
  await call('Emulation.setDeviceMetricsOverride', {width: viewport.width, height: viewport.height, deviceScaleFactor: 1, mobile: viewport.mobile === true});
  await navigate(`${origin}/sign-in`);
  await waitFor('!!document.querySelector(\'input[name="username"]\') || !!document.querySelector(\'[role="alert"]\')', `${label} sign-in form`);
  const initial = await bodyState();
  step(`${label}: sign-in screen`, initial.url.startsWith('/sign-in'), JSON.stringify(initial).slice(0, 600));
  await screenshot(`${label}-01-sign-in.png`);
  if (!await evaluate('!!document.querySelector(\'input[name="username"]\')')) throw new Error(`${label}: no username field; ${JSON.stringify(initial)}`);
  await fill('input[name="username"]', credentials.username);
  await fill('input[name="password"]', credentials.password);
  await screenshot(`${label}-02-filled.png`);
  const submitted = await evaluate('(()=>{const form=document.querySelector("form");if(!form)return false;form.requestSubmit?form.requestSubmit():form.submit();return true;})()');
  if (!submitted) throw new Error(`${label}: sign-in form not submittable`);
  await waitFor('location.pathname === "/profiles" || location.pathname === "/" || !!document.querySelector(\'[role="alert"]\')', `${label} sign-in result`, 30000);
  const afterSignIn = await bodyState();
  await screenshot(`${label}-03-after-sign-in.png`);
  step(`${label}: credentials accepted`, afterSignIn.url === '/profiles' || afterSignIn.url === '/' || afterSignIn.alerts.length === 0, JSON.stringify(afterSignIn).slice(0, 800));
  if (afterSignIn.alerts.length) throw new Error(`${label}: sign-in alert ${JSON.stringify(afterSignIn.alerts)}`);
  let names = [];
  if (afterSignIn.url === '/profiles') {
    await waitFor('!!document.querySelector(\'button\')', `${label} profile picker`);
    names = await evaluate('[...document.querySelectorAll("button b")].filter(b=>b.offsetParent!==null && b.closest("button") && !b.closest("button").disabled).map(b=>(b.innerText||"").trim()).filter(t=>t && !/Manage profiles|Add profile/.test(t))');
    step(`${label}: profile picker`, names.length >= 1, JSON.stringify(names).slice(0, 400));
    await screenshot(`${label}-03b-profiles.png`);
    const clicked = await evaluate(`(()=>{const b=[...document.querySelectorAll('button b')].find(x=>x.offsetParent!==null && (x.innerText||'').trim()===${JSON.stringify(names[0])} && x.closest('button') && !x.closest('button').disabled);if(!b)return false;b.closest('button').click();return true;})()`);
    if (!clicked) throw new Error(`${label}: no selectable profile`);
    await waitFor('location.pathname === "/" && !!document.querySelector("[data-shell-navigation]")', `${label} home after profile`, 30000);
  } else {
    await waitFor('!!document.querySelector("[data-shell-navigation]")', `${label} home shell`, 30000);
  }
  try {await waitFor('document.body.innerText.trim().length > 300', `${label} home content`, 15000);} catch {}
  await new Promise(resolveDwell => setTimeout(resolveDwell, 1200));
  const populated = await evaluate('document.body.innerText.trim().length');
  const home = await bodyState(), violations = await evaluate('window.__porticoCsp ?? []');
  await screenshot(`${label}-04-home.png`);
  step(`${label}: Home reachable`, home.url === '/', JSON.stringify(home).slice(0, 600));
  step(`${label}: Home content loads`, populated > 300, `text=${populated}`);
  await call('Page.reload');
  for (const delay of [1500, 5000, 15000]) {
    await new Promise(resolveDwell => setTimeout(resolveDwell, delay));
    const probe = await evaluate(`({path: location.pathname, text: document.body.innerText.trim().length,
      root: document.getElementById('root')?.innerHTML.length ?? -1,
      route: !!document.querySelector('[data-route-content]'),
      alert: [...document.querySelectorAll('[role="alert"]')].map(node => node.innerText.trim()).filter(Boolean),
      skeleton: document.querySelectorAll('[class*="skeleton" i]').length})`);
    reloads.push({label, delay, probe});
    if (delay === 1500) await screenshot(`${label}-04c-reloaded.png`);
    if (probe.text > 300) break;
  }
  const restored = reloads.filter(entry => entry.label === label).at(-1)?.probe ?? {path: '', text: 0};
  step(`${label}: session survives reload`, restored.path === '/' && restored.text > 300, JSON.stringify(reloads.filter(entry => entry.label === label)));
  if (label === 'desktop') {
    await navigate(`${origin}/servers`);
    try {await waitFor('document.body.innerText.includes("Portico demo")', 'servers route', 15000);} catch {}
    await screenshot('desktop-04b-servers.png');
    step('desktop: servers route renders', await evaluate('document.body.innerText.includes("Portico demo")'));
    await navigate(`${origin}/`);
    await waitFor('document.body.innerText.trim().length > 300', 'home after servers', 20000);
  }
  return {names, home, violations};
}
let result;
try {
  const endpoint = await new Promise((resolveEndpoint, reject) => {
    const timer = setTimeout(() => reject(new Error(`Chromium startup timeout: ${stderr}`)), 15000);
    chrome.once('error', error => {clearTimeout(timer); reject(error);});
    chrome.stderr.on('data', chunk => {
      stderr = (stderr + chunk.toString()).slice(-8192);
      const match = stderr.match(/DevTools listening on (ws:\/\/[^\s]+)/);
      if (match) {clearTimeout(timer); resolveEndpoint(match[1]);}
    });
    chrome.once('exit', code => {clearTimeout(timer); reject(new Error(`Chromium exited ${code}: ${stderr}`));});
  });
  socket = new WebSocket(endpoint);
  await once(socket, 'open');
  socket.addEventListener('message', event => {
    const message = JSON.parse(event.data);
    if (message.method === 'Runtime.exceptionThrown') errors.push(`exception: ${message.params.exceptionDetails.exception?.description ?? message.params.exceptionDetails.text}`);
    if (message.method === 'Runtime.consoleAPICalled' && ['error', 'warning'].includes(message.params.type)) {
      consoleErrors.push(message.params.args.map(arg => String(arg.value ?? arg.description ?? '')).join(' ').slice(0, 300));
    }
    if (message.method === 'Log.entryAdded' && (message.params.entry.source === 'security' || /Content Security Policy|inline style/i.test(message.params.entry.text))) {
      const entry = message.params.entry;
      cspFindings.push(`${entry.text.slice(0, 400)}${entry.url ? ` [${entry.url}]` : ''}`);
    }
    if (message.method === 'Network.loadingFailed' && !message.params.canceled) netFailures.push(`${message.params.type}: ${message.params.errorText}`);
    const task = pending.get(message.id);
    if (!task) return;
    pending.delete(message.id); clearTimeout(task.timer);
    if (message.error) task.reject(new Error(JSON.stringify(message.error))); else task.resolve(message.result);
  });
  const target = await call('Target.createTarget', {url: 'about:blank'}, null);
  session = (await call('Target.attachToTarget', {targetId: target.targetId, flatten: true}, null)).sessionId;
  for (const domain of ['Page.enable', 'Runtime.enable', 'Network.enable', 'Log.enable']) await call(domain);
  await call('Page.addScriptToEvaluateOnNewDocument', {source: `window.__porticoCsp=[];document.addEventListener('securitypolicyviolation',event=>{
    const target=event.target;
    window.__porticoCsp.push({directive:event.violatedDirective,blocked:event.blockedURI,sample:(event.sample||'').slice(0,400),file:event.sourceFile,line:event.lineNumber,column:event.columnNumber,
      target:target?{tag:target.tagName,cls:String(target.className||'').slice(0,120),html:String(target.outerHTML||'').slice(0,400)}:null});
  });`});

  const desktop = await signInOnce('desktop', {width: 1440, height: 900});
  const other = desktop.names[1];
  step('desktop: second profile present', typeof other === 'string' && other.length > 0, JSON.stringify(desktop.names));
  if (other) {
    const opened = await evaluate('(()=>{const b=[...document.querySelectorAll("button")].find(x=>(x.getAttribute("aria-label")||"").endsWith("account menu"));if(!b)return false;b.click();return true;})()');
    step('desktop: account menu opens', opened);
    if (opened) {
      await waitFor('[role="menu"]', 'account menu');
      const switched = await clickMenuItem(other);
      step('desktop: switch profile control', switched, other);
      if (switched) {
        await waitFor('!!document.querySelector("[data-shell-navigation]")', 'home after profile switch', 30000);
        try {await waitFor(`([...document.querySelectorAll("button")].map(b=>b.getAttribute("aria-label")||"").find(v=>v.endsWith("account menu"))||"").includes(${JSON.stringify(other)})`, 'switched profile label', 20000);} catch {}
        try {await waitFor('document.body.innerText.trim().length > 300', 'home content after switch', 20000);} catch {}
        await paint();
        const label = await evaluate('[...document.querySelectorAll("button")].map(b=>b.getAttribute("aria-label")||"").find(v=>v.endsWith("account menu"))||""');
        step('desktop: switched profile shown', label.includes(other), label);
        await screenshot('desktop-05-switched-profile.png');
      }
    }
    const openedAgain = await evaluate('(()=>{const b=[...document.querySelectorAll("button")].find(x=>(x.getAttribute("aria-label")||"").endsWith("account menu"));if(!b)return false;b.click();return true;})()');
    if (openedAgain) {
      await waitFor('[role="menu"]', 'account menu for sign-out');
      const signedOut = await evaluate(`(()=>{const items=[...document.querySelectorAll('[role="menuitem"]')];
        const item=items.find(x=>/^Sign out( of |$)/.test((x.innerText||'').trim()));if(!item)return false;item.click();return true;})()`);
      step('desktop: sign out control', signedOut);
      if (signedOut) {
        await waitFor('location.pathname === "/sign-in" && !!document.querySelector(\'input[name="username"]\')', 'sign-in after sign out', 30000);
        await screenshot('desktop-06-signed-out.png');
        step('desktop: signed out to sign-in', true);
      }
    }
  }

  await call('Storage.clearDataForOrigin', {origin, storageTypes: 'all'});
  await call('Network.clearBrowserCookies');
  const mobile = await signInOnce('mobile', {width: 390, height: 844, mobile: true});
  result = {status: errors.length ? 'FAIL' : 'PASS', origin, steps, errors, cspFindings, consoleErrors: consoleErrors.slice(0, 20), reloads, netFailures: netFailures.slice(0, 12),
    desktopProfiles: desktop.names, mobileProfiles: mobile.names, home: mobile.home,
    violations: {desktop: desktop.violations, mobile: mobile.violations}};
} catch (error) {
  result = {status: 'FAIL', origin, steps, errors, netFailures: netFailures.slice(0, 12), failure: String(error.stack ?? error)};
} finally {
  try {socket?.close();} catch {}
  try {process.kill(-chrome.pid, 'SIGKILL');} catch {try {chrome.kill('SIGKILL');} catch {}}
}
await writeFile(resolve(output, 'signin-report.log'), JSON.stringify(result, null, 2));
console.log('SIGNIN_DEMO ' + JSON.stringify({status: result.status, steps: result.steps.length, failures: result.errors.slice(0, 6), failure: result.failure?.split('\n')[0]}));
process.exit(result.status === 'PASS' ? 0 : 1);
