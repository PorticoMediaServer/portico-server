import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {readFile, stat, rm} from 'node:fs/promises';
import {fileURLToPath, pathToFileURL} from 'node:url';
import {join, resolve} from 'node:path';
import {build} from 'vite';

// Runs the real production main/App/router against an empty browser vault. The
// local origin has NO accounts or credentials; API failures must still paint the
// routed shell/entry instead of leaving only the persistent media host mounted.
const root = fileURLToPath(new URL('..', import.meta.url));
const outDir = join(process.env.TMPDIR, 'assembly-startup-dist');
const {chromium} = await import(pathToFileURL(process.env.PORTICO_PLAYWRIGHT_MODULE).href);
let server, browser;
const mime = {'.js': 'text/javascript', '.css': 'text/css', '.woff2': 'font/woff2', '.svg': 'image/svg+xml', '.png': 'image/png'};
try {
  await build({root, configFile: join(root, 'vite.config.ts'), build: {outDir, emptyOutDir: true}});
  server = createServer(async (request, response) => {
    try {
      const pathname = new URL(request.url, 'http://localhost').pathname;
      if (pathname.startsWith('/v1/')) {
        response.writeHead(503, {'Content-Type': 'application/json'});
        response.end(JSON.stringify({error: {code: 'unavailable', retryable: true}}));return;
      }
      let file = resolve(outDir, '.' + pathname);
      if (!file.startsWith(outDir + '/') && file !== outDir) {response.writeHead(404);response.end();return;}
      try {if (!(await stat(file)).isFile()) file = join(outDir, 'index.html');} catch {file = join(outDir, 'index.html');}
      const extension = file.slice(file.lastIndexOf('.'));
      response.writeHead(200, {'Content-Type': extension === '.html' ? 'text/html' : mime[extension] ?? 'application/octet-stream', 'Cache-Control': 'no-store'});
      response.end(await readFile(file));
    } catch {response.writeHead(500);response.end();}
  });
  await new Promise(resolve => server.listen(0, '127.0.0.1', resolve));
  const origin = `http://127.0.0.1:${server.address().port}`;
  browser = await chromium.launch({executablePath: process.env.PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH ?? '/usr/bin/chromium', headless: true,
    args: ['--no-sandbox', '--disable-dev-shm-usage']});
  for (const path of ['/', '/sign-in']) {
    const context = await browser.newContext(), page = await context.newPage(), errors = [];
    page.on('pageerror', error => {errors.push(error.message);});
    await page.goto(origin + path, {waitUntil: 'networkidle'});
    let mounted = true;
    try {await page.locator('#portico-content').waitFor({timeout: 6000});} catch {mounted = false;}
    const observed = await page.evaluate(() => {
      const root = document.getElementById('root'), key = root && Object.keys(root).find(key => key.startsWith('__reactContainer'));
      const start = key && root[key];let router;
      const visited = new Set(), pending = start ? [start.current ?? start] : [];
      while (pending.length && visited.size < 1000) {
        const fiber = pending.pop();if (!fiber || visited.has(fiber)) continue;visited.add(fiber);
        const candidate = fiber.memoizedProps?.router;
        if (candidate?.stores && candidate?.routesById) {router = candidate;break;}
        pending.push(fiber.child, fiber.sibling, fiber.alternate);
      }
      return {path: location.pathname, content: !!document.getElementById('portico-content'), forms: document.forms.length,
        rootChildren: [...(root?.children ?? [])].map(element => element.tagName),
        router: router && {isServer: router.isServer, status: router.state.status, isLoading: router.state.isLoading,
          matches: router.state.matches.map(value => ({routeId: value.routeId, status: value.status})),
          pending: router.state.pendingMatches?.length ?? 0, transaction: !!router._tx}};
    });
    console.log(JSON.stringify({case: path, mounted, observed, errors}));
    await context.close();
    assert.equal(mounted, true, 'production App must mount routed content before any account/API result');
    assert.deepEqual(errors, []);
  }
} finally {
  await browser?.close();
  if (server) await new Promise(resolve => server.close(resolve));
  await rm(outDir, {recursive: true, force: true});
}
