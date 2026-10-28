import assert from 'node:assert/strict';
import {test} from 'node:test';
import {selectServicesEntry, servicesEntryPlugin} from '../services/vite-entry.mjs';

const html = '<html><head></head><body><script src="/browser-check.js"></script><div id="root"></div><script type="module" src="/src/main.tsx"></script></body></html>';
test('the real root HTML selects Services only with both exact existing build switches', () => {
  for (const env of [{}, {VITE_HOSTED_WEB: 'true'}, {VITE_ACCOUNT_WORKSPACE: 'true'}, {VITE_HOSTED_WEB: 'TRUE', VITE_ACCOUNT_WORKSPACE: 'true'}, {VITE_HOSTED_URL: 'https://web.getportico.tv'}])
    assert.equal(selectServicesEntry(html, env), html);
  const result = selectServicesEntry(html, {VITE_HOSTED_WEB: 'true', VITE_ACCOUNT_WORKSPACE: 'true'});
  assert.equal(result, html.replace('/src/main.tsx', '/src/services/main.tsx'));
  assert.ok(result.includes('<script src="/browser-check.js"></script>'));
});
test('enabled Services cannot silently emit a missing, duplicated or non-module server bootstrap', () => {
  const env = {VITE_HOSTED_WEB: 'true', VITE_ACCOUNT_WORKSPACE: 'true'};
  for (const input of [html.replace('/src/main.tsx', '/other.tsx'), html + '<script type="module" src="/src/main.tsx"></script>', html.replace('type="module" ', '')])
    assert.throws(() => selectServicesEntry(input, env));
});
test('the plugin captures one explicit build mode before later environment mutation', () => {
  const env = {VITE_HOSTED_WEB: 'true', VITE_ACCOUNT_WORKSPACE: 'true'};
  const plugin = servicesEntryPlugin(env); env.VITE_ACCOUNT_WORKSPACE = 'false';
  assert.equal(plugin.name, 'portico-services-entry');
  assert.equal(plugin.transformIndexHtml.order, 'pre');
  assert.ok(plugin.transformIndexHtml.handler(html, {path: '/index.html'}).includes('/src/services/main.tsx'));
});
test('Services selection touches only the real root index, leaving owned fixture/deep HTML inputs alone', () => {
  const plugin = servicesEntryPlugin({VITE_HOSTED_WEB: 'true', VITE_ACCOUNT_WORKSPACE: 'true'});
  const fixture = '<html><script type="module" src="/test/fixture.tsx"></script></html>';
  assert.equal(plugin.transformIndexHtml.handler(fixture, {path: '/test/fixture.html'}), fixture);
});
