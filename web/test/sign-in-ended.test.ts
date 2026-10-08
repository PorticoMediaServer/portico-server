import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {signInHintFor} from '../src/app/sign-in-hint.ts';
import {restoreOutcome} from '../src/bridge/restore-policy.ts';
import {SessionRefreshError} from '../../packages/client-core/src/server-connections.ts';

const viewer = (authority: string) => ({viewer: {accountId: 'acc_1', profileId: 'prof_1', serverId: 'srv_1', authority, role: 'member'}});

test('an ended direct sign-in reopens the direct form for the same server and account', () => {
  assert.deepEqual(signInHintFor('https://media.example:32500', viewer('local'), 'justin'), {authority: 'local', serverId: 'srv_1', serverUrl: 'https://media.example:32500', accountId: 'acc_1', username: 'justin'});
  assert.equal('username' in signInHintFor('https://media.example:32500', viewer('local')), false, 'at launch the name comes from the server’s remembered list');
  assert.equal(signInHintFor('https://relay.example', viewer('hosted')).authority, 'hosted', 'a Portico Account goes to account sign-in and its server chooser');
});

test('ended vs offline: a refused renewal ends the sign-in, a network failure keeps it', async () => {
  const saved = {serverUrl: 'https://media.example', serverId: 'srv_1'};
  const system = (async () => new Response(JSON.stringify({id: 'srv_1'}), {status: 200})) as typeof fetch;
  assert.equal(await restoreOutcome(new SessionRefreshError('refresh_refused', 'Your sign-in has ended.', true), saved, system), 'reset');
  assert.equal(await restoreOutcome(new TypeError('Failed to fetch'), saved, system), 'retry');
});

test('the shell never shows a "sign in again" banner; an ended sign-in goes to the sign-in screen', () => {
  const src = (p: string) => readFileSync(new URL('../src/' + p, import.meta.url), 'utf8');
  const connection = src('shell/Connection.tsx');
  assert.doesNotMatch(connection, /web\.connection\.signInAgain|kind === 'reset'/);
  const session = src('app/session.tsx');
  assert.doesNotMatch(session, /kind: 'reset'/);
  assert.match(session, /signInEnded\.current\(record\.serverUrl, record\.session, undefined, portico\)/, 'launch restore: reset → sign-in screen');
  assert.match(session, /refreshEnded\.current = error => \{[\s\S]*signInEnded\.current\(current\.serverUrl, current\.session, portico \? undefined : local\?\.username, portico\)/, 'refresh refused while running → sign-in screen');
});

test('a Portico Account member whose sign-in ended goes to the account’s server chooser, not the password form', () => {
  const hint = signInHintFor('https://home.example', viewer('local'), undefined, true);
  assert.equal(hint.authority, 'hosted');
  assert.equal('username' in hint, false);
});
