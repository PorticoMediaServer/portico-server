import test from 'node:test';
import assert from 'node:assert/strict';
import {loadConfigFromFile} from 'vite';
import {fileURLToPath} from 'node:url';

const root = fileURLToPath(new URL('..', import.meta.url));
const html = '<html><body><script src="/browser-check.js"></script><script type="module" src="/src/main.tsx"></script></body></html>';
test('the actual production Vite config installs exactly one guarded Services entry selector', async () => {
  const names = ['VITE_HOSTED_WEB', 'VITE_ACCOUNT_WORKSPACE'] as const;
  const previous = names.map(name => process.env[name]);
  try {
    for (const both of [false, true]) {
      process.env.VITE_HOSTED_WEB = 'true';process.env.VITE_ACCOUNT_WORKSPACE = String(both);
      const loaded = await loadConfigFromFile({command: 'build', mode: 'production'}, root + '/vite.config.ts', root);
      assert.ok(loaded);
      const plugins = loaded.config.plugins!.flat(Infinity) as {name?: string; transformIndexHtml?: {handler?: (html: string) => string}}[];
      const selectors = plugins.filter(value => value?.name === 'portico-services-entry');
      assert.equal(selectors.length, 1);
      assert.ok(plugins.some(value => value?.name === 'portico-browser-check'));
      const handler = selectors[0].transformIndexHtml!.handler! as (markup: string, context: {path: string}) => string;
      const selected = handler(html, {path: '/index.html'});
      assert.equal(selected, both ? html.replace('/src/main.tsx', '/src/services/main.tsx') : html);
    }
  } finally {
    names.forEach((name, index) => {if (previous[index] === undefined) delete process.env[name];else process.env[name] = previous[index];});
  }
});
