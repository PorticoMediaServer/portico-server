import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {componentModule} from './helpers/component-harness.mjs';

const src = (path: string) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');
const connection = () => componentModule(new URL('../src/shell/Connection.tsx', import.meta.url), {react: {default: {}}}) as Promise<typeof import('../src/shell/Connection.tsx')>;

test('a page the person had edited is given back as it was after an outage; an untouched page starts afresh', async () => {
  const {pageAfterOutage} = await connection();
  assert.equal(pageAfterOutage({edited: true}), 'keep');
  assert.equal(pageAfterOutage({edited: false}), 'reload');
});

test('the page stays mounted, hidden and inert, under the reconnecting status', () => {
  const source = src('shell/Connection.tsx');
  const body = source.slice(source.indexOf('export function ConnectionOverlay'));
  assert.match(body, /<div className=\{shown \? s\.connectionHeld : undefined\} inert=\{shown\}/, 'the page is held, not replaced');
  assert.match(body, /onInputCapture=\{\(\) => \{ edited\.current = true; \}\}/, 'typing marks the page edited');
  assert.match(body, /if \(unreachable\) return status;/, 'a server never reached at launch has no page to keep');
  const css = src('shell/Shell.module.css');
  assert.match(css, /\.connectionHeld \{ visibility: hidden; \}/);
  assert.match(css, /\.connectionCover \{ position: sticky;/);
});
