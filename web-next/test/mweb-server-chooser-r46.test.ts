import test from 'node:test';
import assert from 'node:assert/strict';
import {registerHooks} from 'node:module';
import {createElement} from 'react';
import {renderToStaticMarkup} from 'react-dom/server';
import {transformTsx} from './transform-tsx.mjs';
import type {MobileChooserAttempt, MobileChooserProps, MobileChooserRow} from '../src/screens/accounts/Servers.route';

registerHooks({load(url, context, next) {
  // The route shell owns navigation; this unit render never mounts the app router.
  if (url.endsWith('/src/router.tsx')) return {format: 'module', source: 'export const router = {navigate: async () => {}};', shortCircuit: true};
  if (url.endsWith('/Servers.route.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
  if (url.endsWith('.module.css')) return {format: 'module', source: 'export default new Proxy({}, {get: (_, key) => String(key)});', shortCircuit: true};
  if (url.endsWith('.svg')) return {format: 'module', source: 'export default "/wordmark.svg";', shortCircuit: true};
  if (url.endsWith('.tsx')) return {format: 'module', source: transformTsx(url), shortCircuit: true};
  return next(url, context);
}});
const {MobileServerChooser} = await import('../src/screens/accounts/Servers.route');

const seenAt = new Date(2026, 9, 12, 12, 0, 0).toISOString();
const rows: readonly MobileChooserRow[] = [
  {id: 'portico-demo', label: 'Portico demo', signIn: 'portico', selected: false, status: {kind: 'online'}, owner: true},
  {id: 'the-cottage', label: 'The cottage', signIn: 'portico', selected: false, status: {kind: 'online'}},
  {id: 'dads-server', label: 'Dad’s server', signIn: 'portico', selected: false, status: {kind: 'seen', at: seenAt}},
  {id: 'lake-house', label: 'Lake house', signIn: 'portico', selected: false, status: {kind: 'never'}},
];
const noop = () => {};
function props(overrides: Partial<MobileChooserProps> = {}): MobileChooserProps {
  return {rows, initial: 'N', onChoose: noop, onRefresh: noop, onLocal: noop, onDifferentAccount: noop, onCheckNow: noop, onOther: noop, ...overrides};
}
function render(overrides: Partial<MobileChooserProps> = {}) {
  return renderToStaticMarkup(createElement(MobileServerChooser, props(overrides)));
}
function attempt(value: MobileChooserAttempt) {return render({attempt: value});}

test('frame 6a renders the authored list from the saved rows, with no invented current marker or credential form', () => {
  const html = render();
  assert.match(html, /<h1>Choose a server<\/h1>/);
  assert.match(html, /Choose the server to watch from\./);
  assert.equal((html.match(/data-row=/g) ?? []).length, 4);
  assert.equal((html.match(/>Online</g) ?? []).length, 2);
  assert.match(html, /Last seen 12 Oct/);
  assert.match(html, /Never seen/);
  assert.equal((html.match(/>Owner</g) ?? []).length, 1);
  assert.match(html, /Refresh servers/);
  assert.match(html, /Use a different Portico Account/);
  assert.match(html, /Use a local server/);
  assert.doesNotMatch(html, /Current server|<input|<form|<a\b/);
  assert.match(html, /data-mweb-server-chooser/);
});

test('a production row without account presence shows its real saved sign-in kind instead of inventing Online', () => {
  const html = render({rows: [{id: 'local', label: 'Studio', signIn: 'local', selected: false}], onDifferentAccount: undefined});
  assert.match(html, /Studio<small>Signed in with a local account<\/small>/);
  assert.doesNotMatch(html, /Online|Last seen|Never seen|Owner|Use a different Portico Account/);
});

test('frame 6b is the real waiting state with a countdown ring and the two recovery actions', () => {
  const html = attempt({kind: 'waiting', row: rows[1]!, seconds: 20, total: 20});
  assert.match(html, /Waiting for The cottage/);
  assert.match(html, /Portico will connect automatically when it’s back\./);
  assert.match(html, /Trying again/);
  assert.match(html, /data-progress="1"/);
  assert.match(html, /Check now/);
  assert.match(html, /Choose another server/);
  assert.doesNotMatch(html, /Refresh servers|Owner|Choose a server/);
});

test('frame 6c says exactly what was last seen and offers Try again, never a password form', () => {
  const html = attempt({kind: 'unreachable', row: rows[2]!});
  assert.match(html, /Portico can’t reach Dad’s server/);
  assert.match(html, /It was last online 12 Oct\./);
  assert.match(html, /Try again/);
  assert.match(html, /Choose another server/);
  assert.doesNotMatch(html, /<input|<form|Password/);
});

test('the unreachable sentence falls back to the saved server name when the runtime has no presence clock', () => {
  const html = attempt({kind: 'unreachable', row: {id: 'cottage', label: 'The cottage', signIn: 'portico', selected: false}});
  assert.match(html, /Portico can’t reach The cottage\. Check that it’s on and connected, then try again\./);
  assert.doesNotMatch(html, /12 Oct|Never seen/);
});

test('opening, loading and list-failed are real transient states, not a second fake chooser', () => {
  const opening = attempt({kind: 'opening', row: rows[1]!});
  assert.match(opening, /Opening The cottage…/);
  assert.match(opening, /Choose another server/);
  assert.doesNotMatch(opening, /Refresh servers|Owner/);
  assert.match(attempt({kind: 'loading'}), /Loading your servers/);
  const failed = render({attempt: {kind: 'loadFailed'}, notice: 'Your saved servers couldn’t be read.'});
  assert.match(failed, /Your servers couldn’t be loaded/);
  assert.match(failed, /Your saved servers couldn’t be read\./);
  assert.match(failed, /Try again/);
});
