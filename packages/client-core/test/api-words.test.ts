import assert from 'node:assert/strict';
import test from 'node:test';
import {createI18n} from '../../i18n/src/index.ts';
import type {ApiCredential, Webhook, WebhookDelivery} from '../src/api-access.ts';
import {assistantAddress, chosenScopes, credentialFacts, credentialMember, credentialState, credentialWorks, deliveryProblem, deliveryProgress, deliveryTone, expiryInstant, expiryLabel, scopeTicked, scopesText, webhookAddressOk, webhookEventGroups, webhookEventLabel, webhookTrouble} from '../src/server-admin/api-words.ts';

const i18n = createI18n();
const t = i18n.t;
const has = (id: string) => i18n.has(id);
const key: ApiCredential = {id: 'k1', kind: 'key', name: 'Home Assistant', scopes: ['read', 'playback'], hint: 'a1b2', createdAt: '2026-10-01T10:00:00Z', state: 'active', member: {accountId: 'acc', username: 'justin', profileId: 'p1', profile: 'Justin'}};
const format = {relative: () => '3 hours ago', date: () => 'Jan 4'};

test('"Everything" stands alone, and the rest are sent in the server\'s order', () => {
  assert.deepEqual(chosenScopes(new Set(['library', 'read'])), ['read', 'library']);
  assert.deepEqual(chosenScopes(new Set(['read', 'admin', 'library'])), ['admin']);
  assert.deepEqual(chosenScopes(new Set()), []);
  assert.equal(scopeTicked(new Set(['admin']), 'library'), true);
  assert.equal(scopeTicked(new Set(['read']), 'library'), false);
});

test('what a key may do is said in words, never as scope names', () => {
  assert.equal(scopesText(t, ['playback', 'read']), 'Read, Play and control playback');
  assert.equal(scopesText(t, ['admin']), 'Everything');
});

test('an expiry is a number of days from when the form was sent, or never', () => {
  assert.equal(expiryInstant(0, Date.UTC(2026, 0, 1)), undefined);
  assert.equal(expiryInstant(30, Date.UTC(2026, 0, 1)), '2026-01-31T00:00:00.000Z');
  assert.equal(expiryLabel(t, 0), 'Never, until I revoke it');
  assert.equal(expiryLabel(t, 90), 'In 90 days');
  assert.equal(expiryLabel(t, 365), 'In a year');
});

test('a key\'s line says what it may do, how it ends, when it was used and when it stops', () => {
  assert.equal(credentialFacts(t, key, format), 'Read, Play and control playback · ends in a1b2 · Never used · Does not expire');
  assert.equal(credentialFacts(t, {...key, lastUsedAt: '2026-10-12T10:00:00Z', expiresAt: '2027-01-04T00:00:00Z'}, format), 'Read, Play and control playback · ends in a1b2 · Last used 3 hours ago · Expires Jan 4');
});

test('a token names its member, with the profile when it has another name', () => {
  assert.equal(credentialMember({...key, member: {...key.member, profile: 'Kids'}}), 'justin · Kids');
  assert.equal(credentialMember({...key, member: {...key.member, profile: 'justin'}}), 'justin');
});

test('a key that stopped says why when the server gives a reason it has words for', () => {
  assert.equal(credentialState(t, has, key), undefined);
  assert.equal(credentialWorks(key), true);
  assert.equal(credentialWorks({...key, state: 'awaiting_approval'}), true);
  assert.equal(credentialState(t, has, {...key, state: 'awaiting_approval'})?.tone, 'warning');
  const ended = {...key, state: 'ended' as const, endedReason: 'member_disabled'};
  assert.equal(credentialWorks(ended), false);
  assert.equal(credentialState(t, has, ended)?.text, 'No longer works. It stopped working when this person was disabled.');
  // A reason this app has no words for is not shown as a code.
  assert.equal(credentialState(t, has, {...ended, endedReason: 'quantum_flux'})?.text, 'No longer works');
});

test('webhook events are grouped, and one this app does not know is still offered', () => {
  const groups = webhookEventGroups(['title.added', 'playback.started', 'storage.low', 'server.started', 'title.added', 'lyrics.fetched']);
  assert.deepEqual(groups.map(g => g.id), ['titles', 'playback', 'server', 'other']);
  assert.deepEqual(groups.find(g => g.id === 'server')!.events, ['storage.low', 'server.started']);
  assert.deepEqual(groups.find(g => g.id === 'other')!.events, ['lyrics.fetched']);
  assert.equal(webhookEventLabel(t, has, 'playback.started'), 'Someone starts playing');
  assert.equal(webhookEventLabel(t, has, 'lyrics.fetched'), 'lyrics.fetched');
});

test('a webhook says its most serious trouble first', () => {
  const hook: Webhook = {id: 'w', name: 'n', url: 'https://example.com', events: [], enabled: true, secretHint: 'ab', createdAt: '2026-10-01T10:00:00Z', updatedAt: '2026-10-01T10:00:00Z', waiting: 2, failing: 0, dropped: 0};
  assert.equal(webhookTrouble(t, {...hook, waiting: 0}), undefined);
  assert.equal(webhookTrouble(t, hook)?.tone, 'neutral');
  assert.equal(webhookTrouble(t, {...hook, dropped: 4})?.tone, 'warning');
  assert.equal(webhookTrouble(t, {...hook, dropped: 4, failing: 3})?.tone, 'danger');
});

test('a delivery that has not arrived says why in words', () => {
  const sent: WebhookDelivery = {id: 'd', event: 'title.added', state: 'failed', attempts: 3, createdAt: '2026-10-01T10:00:00Z', status: 0, durationMs: 0, error: 'timed_out'};
  assert.equal(deliveryTone(sent), 'danger');
  assert.equal(deliveryProblem(t, has, sent), 'The other service took too long to answer.');
  assert.equal(deliveryProblem(t, has, {...sent, error: 'something_new'}), 'The message could not be sent.');
  assert.equal(deliveryProblem(t, has, {...sent, error: undefined, status: 500}), 'The other service answered with an error (500).');
  assert.equal(deliveryProblem(t, has, {...sent, state: 'delivered', error: undefined, status: 200}), undefined);
  assert.equal(deliveryProgress(t, {...sent, state: 'pending', nextAttemptAt: '2026-10-01T10:05:00Z'}, () => '10:05'), '3 attempts · trying again at 10:05');
  assert.equal(deliveryProgress(t, {...sent, state: 'pending', attempts: 0}, () => ''), '');
});

test('a webhook address is http or https with no name or password in it', () => {
  assert.equal(webhookAddressOk('https://example.com/hooks/portico'), true);
  assert.equal(webhookAddressOk(' http://192.168.1.20:8123/api/webhook/abc '), true);
  assert.equal(webhookAddressOk('ftp://example.com'), false);
  assert.equal(webhookAddressOk('https://user:secret@example.com'), false);
  assert.equal(webhookAddressOk('example.com/hook'), false);
});

test('the assistant address is the server\'s own address with the one path', () => {
  assert.equal(assistantAddress('https://media.example.com'), 'https://media.example.com/v1/mcp');
  assert.equal(assistantAddress('http://192.168.1.10:32500/'), 'http://192.168.1.10:32500/v1/mcp');
});
