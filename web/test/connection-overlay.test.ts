import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';

const src = (path: string) => readFileSync(new URL('../src/' + path, import.meta.url), 'utf8');

test('a server that stops answering never covers, empties or restarts the page', () => {
  const source = src('shell/Connection.tsx');
  const body = source.slice(source.indexOf('export function ConnectionOverlay'));
  // Only an address that answers as someone else covers the page.
  assert.match(body, /const unavailable = !serverProblem && !!route && route\.phase === 'identity_mismatch';/);
  assert.doesNotMatch(body, /route\.phase === 'offline'/, 'an offline route is not a reason to cover the page');
  // The page is one stable element: no key that would start it afresh when the server returns.
  assert.match(body, /<div className=\{shown \? s\.connectionHeld : undefined\} inert=\{shown\}>\{children\}<\/div>/);
  assert.doesNotMatch(body, /Fragment key=/);
  assert.match(body, /if \(unreachable\) return status;/, 'a server never reached at launch has no page to keep');
});

test('after a sustained failure one small line says so, and it is a status, not an alert', () => {
  const source = src('shell/Connection.tsx');
  assert.match(source, /const away = useReachability\(\) === 'unreachable' && !serverProblem && !unavailable;/);
  assert.match(source, /away && !shown \? <div className=\{s\.connectionAway\} role="status"><Notice tone="info" compact>\{i18n\.t\('web\.connection\.cantReach', \{server: name\}\)\}<\/Notice><\/div> : null/);
  const css = src('shell/Shell.module.css');
  assert.match(css, /\.connectionAway \{/);
  assert.match(css, /\.connectionHeld \{ visibility: hidden; \}/);
});
