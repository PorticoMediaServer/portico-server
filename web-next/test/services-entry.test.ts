import assert from 'node:assert/strict';
import {test} from 'node:test';
import {captureServicesEntry} from '../src/services/entry.ts';

const origin = 'https://account.example';
function nativePage(fragment: string) {
  const calls: unknown[][] = [];
  const page = {href: origin + '/app/account-return' + fragment, replace: (value: string | URL) => { calls.push(['open', String(value)]); },
    get pathname(): string { throw new Error('generic fragment handling ran'); },
    get search(): string { throw new Error('generic fragment handling ran'); },
    get hash(): string { throw new Error('generic fragment handling ran'); }};
  const history = {state: null, replaceState: (...args: unknown[]) => { calls.push(['scrub', ...args]); }};
  return {page, history, calls};
}
test('Services gives the checked Apps native return exclusive synchronous dispatch before generic capture', () => {
  const code = 'c'.repeat(43), f = nativePage('#tx=transaction_123&code=' + code);
  assert.deepEqual(captureServicesEntry(f.page, f.history, origin), {native: 'forwarded'});
  assert.deepEqual(f.calls.map(call => call[0]), ['scrub', 'open']);
});
test('a refused native return goes only to the caller fallback and never to generic capture', () => {
  const f = nativePage('#tx=transaction_123&code=short');
  assert.deepEqual(captureServicesEntry(f.page, f.history, origin), {native: 'invalid'});
  assert.deepEqual(f.calls.map(call => call[0]), ['scrub']);
});
test('ordinary device intake is scrubbed before Services can start storage or account work', () => {
  const page = {href: origin + '/device#code=7KQM4TXP', pathname: '/device', hash: '#code=7KQM4TXP', search: '', replace() { throw new Error('no app handoff'); }};
  const calls: unknown[][] = [];
  const history = {state: null, replaceState: (...args: unknown[]) => { calls.push(args); page.hash = ''; }};
  const result = captureServicesEntry(page, history, origin);
  assert.equal(result.capture?.link?.value, '7KQM-4TXP');
  assert.equal(result.invalidContinuation, false);
  assert.equal(page.hash, ''); assert.equal(calls.length, 1);
});

test('a canonical invitation is scrubbed and retained only in this document', () => {
  const code = 'a'.repeat(32), href = 'https://web.getportico.tv/join#code=' + code + '&server=https%3A%2F%2Fserver.example';
  const page = {href, pathname: '/join', hash: new URL(href).hash, search: '', replace() { throw new Error('no app handoff'); }};
  const history = {state: null, replaceState: (_state: unknown, _unused: string, path?: string | URL | null) => { page.href = new URL(String(path), page.href).href; page.hash = ''; }};
  const result = captureServicesEntry(page, history, 'https://web.getportico.tv');
  assert.deepEqual(result.join, {code, server: 'https://server.example'});
  assert.equal(result.invalidContinuation, false); assert.equal(page.hash, '');
  assert.equal(captureServicesEntry(page, history, 'https://web.getportico.tv').invalidContinuation, true, 'a document opened after the scrub must require the invitation again');
});
