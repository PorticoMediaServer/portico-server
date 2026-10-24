import assert from 'node:assert/strict';
import test from 'node:test';
import {createServer} from 'node:http';
import {once} from 'node:events';
import {readdir} from 'node:fs/promises';
import {withMeasurementBrowser} from './browser.mjs';

// Public synthetic measurement fixture, never a product screen or approved design.
const html = `<!doctype html><html><style>
html,body{margin:0}main{width:800px;height:600px;background:#10212d;color:#edf5fa}
h1{margin:0;font:700 24px/30px sans-serif}p{margin:0}p::after{content:"!"}
button{width:120px;height:40px}input{width:160px;height:30px}.hidden{display:none}
</style><main><h1>Public fixture</h1><p>Exact text</p><button>Public action</button>
<label>Account<input name="username" value="public-user" placeholder="Account name"></label>
<label>Password<input type="password" name="password" value="never-serialize-this"></label>
<svg width="20" height="20"><defs><symbol id="public-shape"><path d="M0 0L10 10"/></symbol></defs><use href="#public-shape"/></svg>
<span class="hidden">Not rendered</span></main></html>`;

test('real browser measures the whole viewport and protects source, secret and network boundaries', async () => {
  assert.ok(process.env.TMPDIR, 'owned runner scratch required');
  const executable = process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH;
  assert.ok(executable, 'explicit provisioned measurement browser required');
  let apiRequests = 0;
  const server = createServer((request, response) => {
    if (request.url.startsWith('/v1/')) apiRequests++;
    response.writeHead(200, {'Content-Type': 'text/html'}).end(html);
  });
  server.listen(0, '127.0.0.1'); await once(server, 'listening');
  const origin = `http://127.0.0.1:${server.address().port}`;
  try {
    const measured = await withMeasurementBrowser({scratch: process.env.TMPDIR, executable,
      viewport: {width: 800, height: 600, deviceScaleFactor: 1}, allowedOrigins: [origin]}, async browser => {
      await browser.navigate(origin, 'main');
      await browser.evaluate('document.querySelector("input[name=password]").focus()');
      const screen = await browser.measure('main');
      assert.deepEqual(screen, await browser.measure('main'), 'same source/state extraction is deterministic');
      assert.equal(screen.scope, 'screen'); assert.equal(screen.fullViewport, true);
      assert.deepEqual(screen.root, {x: 0, y: 0, width: 800, height: 600});
      assert.deepEqual(screen.viewport, {width: 800, height: 600, deviceScaleFactor: 1});
      assert.equal(screen.fontStatus, 'loaded');
      assert.ok(screen.strings.some(value => value.text === 'Exact text'));
      assert.ok(screen.strings.some(value => value.kind === 'placeholder' && value.text === 'Account name'));
      assert.ok(!screen.strings.some(value => value.text === 'Not rendered'));
      const password = screen.elements.find(value => value.input?.name === 'password');
      assert.equal(password.focused, true); assert.equal(password.input.value, null);
      assert.equal(password.input.length, 'never-serialize-this'.length); assert.equal(password.input.redacted, true);
      assert.ok(screen.elements.some(value => value.tag === '::after' && value.content === '"!"'));
      assert.ok(screen.elements.some(value => value.image?.kind === 'icon'));
      const serialized = JSON.stringify(screen);
      assert.ok(!serialized.includes('never-serialize-this')); assert.ok(!serialized.includes('M0 0L10 10'));
      assert.ok(!serialized.includes('<main>')); assert.ok(!serialized.includes('<style>'));
      await browser.evaluate('fetch("/v1/private-test").catch(()=>{})');
      assert.deepEqual(browser.blocked, [{kind: 'api', method: 'GET'}]);
      assert.equal(apiRequests, 0, 'no auth API request reaches even the disposable fixture');
      assert.deepEqual(browser.pageErrors, []);
      await browser.evaluate(`document.querySelector('main').insertAdjacentHTML('beforeend', '<section><div data-auth-content><div><button><span>A</span>First</button><button><span>B</span>Second</button></div><form><button type="submit"><span>Synthetic submit</span></button></form><div><button>Link one</button><button>Link two</button></div></div></section>')`);
      const grouped = await browser.measure('main', {
        first: '[data-auth-content] > div:first-of-type > button:nth-child(1)', second: '[data-auth-content] > div:first-of-type > button:nth-child(2)',
        submit: 'button[type=submit]', label: 'button[type=submit] > span',
      });
      assert.deepEqual(grouped.anchorIssues, []);
      assert.equal(grouped.elements.find(value => value.frame === 'submit').ownText, '', 'do not duplicate descendant label in its container');
      assert.equal(grouped.elements.find(value => value.frame === 'label').ownText, 'Synthetic submit');
      return screen;
    });
    assert.ok(measured.elements.length > 8);
    assert.ok(!(await readdir(process.env.TMPDIR)).some(name => name.startsWith('fidelity-browser-')), 'owned browser profile removed');
  } finally {server.closeAllConnections(); await new Promise(resolve => server.close(resolve));}
});
