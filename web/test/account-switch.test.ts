import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {belongsToAnotherAccount, listIsCurrent} from '../src/app/account-switch.ts';
import {cachedServerList, lastServer, rememberLastServer, storeServerList} from '../src/app/server-list-cache.ts';

const src = (p: string) => readFileSync(new URL('../src/' + p, import.meta.url), 'utf8');

test('switching Portico Accounts ends the previous account’s server sign-in; a password sign-in is never touched', () => {
  assert.equal(belongsToAnotherAccount('acct_carol', 'acct_bob'), true, 'Carol’s member sign-in while Bob is signed in: ended');
  assert.equal(belongsToAnotherAccount('acct_bob', 'acct_bob'), false);
  assert.equal(belongsToAnotherAccount(undefined, 'acct_bob'), false, 'a password sign-in belongs to no Portico Account');
  assert.equal(belongsToAnotherAccount('acct_carol', undefined), false, 'no Portico Account signed in (or Hosted away): nothing is signed out');
  const session = src('app/session.tsx');
  assert.match(session, /const owner = local\?\.hostedAccountId \?\? \(await browserConnectionEnvironment\(\)\.storage\.read\(\)[^;]*\)\?\.hosted\?\.accountId;/);
  assert.match(session, /if \(belongsToAnotherAccount\(owner, signedInAccount\)\) disconnectServer\(\);/);
});

test('switching Portico Accounts never shows the previous account’s servers or last-used server', () => {
  const store = new Map<string, string>();
  (globalThis as any).localStorage = {getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => store.set(k, v), removeItem: (k: string) => store.delete(k)};
  storeServerList('acct_carol', [{id: 'srv_carol', name: 'Carol’s'} as any]);
  rememberLastServer('acct_carol', 'srv_carol');
  assert.equal(cachedServerList('acct_bob'), undefined, 'lists are kept per account');
  assert.equal(lastServer('acct_bob'), undefined, 'the last-used server is per account');
  assert.equal(listIsCurrent('acct_carol', 'acct_bob'), false, 'a list answered for Carol after the switch is not shown to Bob');
  assert.equal(listIsCurrent('acct_bob', 'acct_bob'), true);
  assert.match(src('app/session.tsx'), /storeServerList\(owner, list\);\n\s*\/\/[^\n]*\n\s*if \(!listIsCurrent\(owner, central\.service\.getSnapshot\(\)\.session\?\.account\.id\)\) return;\n\s*setServers\(list\);/);
  delete (globalThis as any).localStorage;
});

test('a Portico Account member’s server sign-in is presented as the Portico Account’s, not as a password account', () => {
  const panels = src('screens/settings/AccountPanels.tsx');
  assert.match(panels, /const member = session\.session\?\.viewer\.authority === 'local' && !!session\.local\?\.hostedAccountId;/);
  assert.match(panels, /member \? t\('web\.account\.porticoMemberTitle'\) : t\('account\.manageHosted'\)/);
  // Settings hides the password rows from a member through the same fact.
  assert.match(src('screens/settings/Settings.tsx'), /porticoMember: local && !!session\.local\?\.hostedAccountId/);
});

test('every sign-in is offered Switch profile and Switch server from the profile menu', () => {
  const shell = src('shell/Shell.tsx');
  assert.match(shell, /\{id: 'switch', label: t\('web\.shell\.switchProfile'\)/);
  assert.match(shell, /\{id: 'switchServer', label: t\('web\.shell\.switchServer'\)/);
  assert.match(shell, /title=\{t\('web\.account\.switchProfileOrServer'\)\}/, 'a Portico Account session chooses among its servers and profiles');
});
