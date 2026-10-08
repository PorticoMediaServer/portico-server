import test from 'node:test';
import assert from 'node:assert/strict';
import {decodeVerificationToken, readFragment} from '../src/app/link-fragment.ts';

const encode = (value: unknown) => Buffer.from(JSON.stringify(value)).toString('base64url');

test('readFragment picks only the named secrets from the fragment', () => {
  assert.deepEqual(readFragment(['token'], '#token=abc&other=x'), {token: 'abc'});
  assert.deepEqual(readFragment(['invite'], 'invite=id.secret'), {invite: 'id.secret'});
  assert.deepEqual(readFragment(['code'], ''), {});
});

test('decodeVerificationToken reads the Hosted verification tuple', () => {
  assert.deepEqual(decodeVerificationToken(encode(['1', 'registration', 'reg_123', '482913'])), {kind: 'registration', resource: 'reg_123', code: '482913'});
  assert.deepEqual(decodeVerificationToken(encode(['1', 'email_change', 'ch-9', 'abc'])), {kind: 'email_change', resource: 'ch-9', code: 'abc'});
  assert.equal(decodeVerificationToken(encode(['1', 'oidc_contact', 'c1', 'x']))?.kind, 'oidc_contact');
});

test('decodeVerificationToken rejects anything else', () => {
  assert.equal(decodeVerificationToken(undefined), null);
  assert.equal(decodeVerificationToken('not base64 json'), null);
  assert.equal(decodeVerificationToken(encode(['2', 'registration', 'r', 'c'])), null);
  assert.equal(decodeVerificationToken(encode(['1', 'password', 'r', 'c'])), null);
  assert.equal(decodeVerificationToken(encode(['1', 'registration', '../r', 'c'])), null);
  assert.equal(decodeVerificationToken(encode(['1', 'registration', 'r', ''])), null);
  assert.equal(decodeVerificationToken(encode({kind: 'registration'})), null);
});

test('a Watch Together link code is normalized and taken once', async () => {
  const store = new Map<string, string>();
  (globalThis as {sessionStorage?: unknown}).sessionStorage = {getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => void store.set(k, v), removeItem: (k: string) => void store.delete(k)};
  const {normalizeTogetherCode, setPendingTogetherCode, takePendingTogetherCode} = await import('../src/app/link-fragment.ts');
  assert.equal(normalizeTogetherCode('ab12-cd'), 'AB12CD');
  assert.equal(normalizeTogetherCode('<script>'), undefined);
  setPendingTogetherCode('AB12CD');
  assert.equal(takePendingTogetherCode(), 'AB12CD');
  assert.equal(takePendingTogetherCode(), undefined);
});

test('A33: an email link confirms by token and maps the three statuses', async () => {
  const {confirmEmailLink, emailLinkOutcome, parseEmailLinkResult, EMAIL_LINK_CONFIRM_PATH} = await import('../src/app/email-link.ts');
  const calls: unknown[] = [];
  const request = async (path: string, method: string, body: unknown) => { calls.push([path, method, body]); return {kind: 'registration', status: 'confirmed'}; };
  const r = await confirmEmailLink(request, 'tok_123');
  assert.deepEqual(calls, [[EMAIL_LINK_CONFIRM_PATH, 'POST', {token: 'tok_123'}]]);
  assert.equal(emailLinkOutcome(r), 'confirmed');
  assert.equal(emailLinkOutcome(parseEmailLinkResult({kind: 'email_change', status: 'email_changed'})), 'changed');
  assert.equal(emailLinkOutcome(parseEmailLinkResult({kind: 'oidc_contact', status: 'already_confirmed'})), 'confirmed');
  assert.throws(() => parseEmailLinkResult({kind: 'registration', status: 'pending'}));
  assert.throws(() => parseEmailLinkResult({kind: 'password', status: 'confirmed'}));
  await assert.rejects(confirmEmailLink(request, ''));
});
