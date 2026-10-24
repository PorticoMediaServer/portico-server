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
const {MobileServerChooser, AttemptLock, waitingStep, mobileChooserRows} = await import('../src/screens/accounts/Servers.route');

const seenAt = new Date(2026, 9, 12, 12, 0, 0).toISOString();
const rows: readonly MobileChooserRow[] = [
  {id: 'portico-demo', label: 'Portico demo', signIn: 'portico', selected: false, status: {kind: 'online'}, owner: true},
  {id: 'the-cottage', label: 'The cottage', signIn: 'portico', selected: false, status: {kind: 'online'}},
  {id: 'dads-server', label: 'Dad’s server', signIn: 'portico', selected: false, status: {kind: 'seen', at: seenAt}},
  {id: 'lake-house', label: 'Lake house', signIn: 'portico', selected: false, status: {kind: 'never'}},
];
const noop = () => {};
const frameSeen = (iso: string) => new Intl.DateTimeFormat('en-GB', {day: 'numeric', month: 'short', timeZone: 'UTC'}).format(new Date(iso));
function props(overrides: Partial<MobileChooserProps> = {}): MobileChooserProps {
  return {rows, initial: 'N', formatSeen: frameSeen, onChoose: noop, onRefresh: noop, onLocal: noop, onDifferentAccount: noop, onCheckNow: noop, onOther: noop, ...overrides};
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

test('a saved row without account presence keeps its name and invents neither presence nor a sign-in-kind substitute', () => {
  const html = render({rows: [{id: 'local', label: 'Studio', signIn: 'local', selected: false}], onDifferentAccount: undefined});
  assert.match(html, /Studio/);
  assert.doesNotMatch(html, /Signed in with a local account|Signed in with your Portico Account|Online|Last seen|Never seen|Owner|Use a different Portico Account/);
});

test('frame 6b is the real waiting state with a countdown ring and the two recovery actions', () => {
  const html = attempt({kind: 'waiting', row: rows[1]!, seconds: 20, total: 20});
  assert.match(html, /Waiting for The cottage/);
  assert.match(html, /Portico will connect automatically when it’s back\./);
  assert.match(html, /Checking again in 20 seconds/);
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

test('genuine account-directory facts supply the drawn presence and owner; status/ownership unknowns grant nothing', () => {
  const choices = [{id: 'a', label: 'Saved A', signIn: 'portico', selected: false}, {id: 'b', label: 'Saved B', signIn: 'portico', selected: false}, {id: 'c', label: 'Saved C', signIn: 'local', selected: false}] as const;
  const listed = mobileChooserRows(choices, [
    {id: 'a', name: 'Portico demo', status: 'active', presence: {lastSeenAt: null}, ownedByMe: true},
    {id: 'b', name: 'The cottage', status: 'quiet', presence: {lastSeenAt: seenAt}, ownedByMe: false},
    {id: 'c', name: 'Lake house', status: 'dormant', presence: {lastSeenAt: null}, ownedByMe: false},
  ] as const);
  assert.deepEqual(listed[0], {...choices[0], label: 'Portico demo', status: {kind: 'online'}, owner: true});
  assert.deepEqual(listed[1], {...choices[1], label: 'The cottage', status: {kind: 'seen', at: seenAt}});
  assert.deepEqual(listed[2], {...choices[2], label: 'Lake house', status: {kind: 'never'}});
  assert.equal(listed[1]!.owner, undefined); assert.equal(listed[2]!.owner, undefined);
});

test('a saved server absent from the account directory keeps only its saved facts', () => {
  const choices = [{id: 'x', label: 'Studio', signIn: 'local', selected: false}] as const;
  assert.deepEqual(mobileChooserRows(choices, []), choices);
  assert.deepEqual(mobileChooserRows(choices, [{id: 'y', name: 'Elsewhere', status: 'active', presence: {lastSeenAt: null}, ownedByMe: true}]), choices);
});

test('a cancelled attempt cannot let its late settle clear a successor or leave the chooser locked', () => {
  const lock = new AttemptLock();
  const first = lock.begin();
  assert.equal(lock.running, true);
  lock.cancel();
  assert.equal(lock.running, false);
  const second = lock.begin();
  assert.equal(lock.settle(first), false);
  assert.equal(lock.running, true);
  assert.equal(lock.settle(second), true);
  assert.equal(lock.running, false);
  const third = lock.begin(); lock.cancel();
  assert.equal(lock.settle(third), false); assert.equal(lock.running, false);
});

test('the waiting countdown holds hidden, re-arms a positive countdown on visibility, and retries only at zero', () => {
  const waiting = {kind: 'waiting', row: rows[1]!, seconds: 20, total: 20} as const;
  assert.deepEqual(waitingStep(waiting, 'timer', false), {kind: 'hold'});
  assert.deepEqual(waitingStep(waiting, 'timer', true), {kind: 'tick', seconds: 19});
  assert.deepEqual(waitingStep({...waiting, seconds: 0}, 'timer', false), {kind: 'hold'});
  assert.deepEqual(waitingStep({...waiting, seconds: 0}, 'timer', true), {kind: 'retry'});
  assert.deepEqual(waitingStep(waiting, 'visibility', true), {kind: 'rearm'});
  assert.deepEqual(waitingStep({...waiting, seconds: 0}, 'visibility', true), {kind: 'retry'});
  assert.deepEqual(waitingStep(undefined, 'visibility', true), {kind: 'hold'});
  assert.deepEqual(waitingStep({kind: 'unreachable', row: rows[2]!}, 'visibility', true), {kind: 'hold'});
});
