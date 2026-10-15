import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {enUS} from '../../packages/i18n/src/catalog/en-US.ts';

const read = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8');

// B-16.12: one declared floor, the same in the build, the check, the documentation and the words shown.
test('the browser floor is declared once and stated the same everywhere', () => {
  const config = read('../vite.config.ts');
  assert.match(config, /const browserFloor = \['chrome111', 'edge111', 'firefox113', 'safari16\.4', 'ios16\.4'\];/);
  assert.match(config, /target: browserFloor, cssTarget: browserFloor/);
  assert.match(read('../browser-check.js'), /Chrome and Edge 111, Firefox 113, Safari 16\.4/);
  assert.match(read('../README.md'), /Chrome and Edge 111, Firefox 113, and Safari 16\.4/);
  assert.match(enUS['web.browser.unsupportedBody'], /Chrome or Edge 111, Firefox 113, Safari 16\.4/);
});

test('an older engine is told before the app starts, and the app then stays out of the way', () => {
  const html = read('../index.html');
  assert.ok(html.indexOf('/browser-check.js') < html.indexOf('/src/main.tsx'), 'the check runs first');
  assert.doesNotMatch(read('../browser-check.js').replace(/\/\*[\s\S]*?\*\//g, ''), /=>|\blet\b|\bconst\b|`/, 'the check is written in the oldest syntax');
  assert.match(read('../src/main.tsx'), /__porticoUnsupportedBrowser\) createRoot/);
});
