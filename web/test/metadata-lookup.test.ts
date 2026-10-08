import test from 'node:test';
import assert from 'node:assert/strict';
import {lookupStatus, lookupStatusId, lookupUpdate, lookupsOn, parseScreenLookup, saveLookup} from '../src/admin/metadata-screen.ts';

// GET /v1/libraries/{id}/metadata/screen as the server returns it.
const server = (over: Record<string, unknown> = {}) => ({libraryId: 'lib_1', libraryKind: 'anime', revision: 3, consentRevision: 2, confirmed: true, status: 'enabled', enabled: true, providers: [], availableProviders: ['anilist', 'tmdb', 'tvdb'], language: 'en', region: 'US', refreshMode: 'replace_unlocked', disclosure: 'Portico can send cleaned titles…', disclosureVersion: 'screen-metadata-v1', ...over});

test('the server’s four statuses map directly (consent first)', () => {
  assert.equal(lookupStatus(parseScreenLookup(server())), 'on');
  assert.equal(lookupStatus(parseScreenLookup(server({status: 'disabled', enabled: false}))), 'off');
  assert.equal(lookupStatus(parseScreenLookup(server({status: 'needs_consent', confirmed: false}))), 'needs_consent');
  assert.equal(lookupStatus(parseScreenLookup(server({status: 'declined', confirmed: false}))), 'declined');
  assert.equal(lookupStatus(parseScreenLookup(server({providers: []}))), 'on', 'no providers chosen means the server’s defaults, still on');
  assert.equal(lookupStatusId('needs_consent'), 'web.metadataLookup.status.needsConsent');
  assert.equal(lookupStatusId('declined'), 'web.metadataLookup.status.declined');
  const older = (o: Record<string, unknown>) => { const v: Record<string, unknown> = server(o); delete v.status; return parseScreenLookup(v); };
  assert.equal(lookupStatus(older({confirmed: false})), 'needs_consent', 'a server without status: derived');
  assert.equal(lookupStatus(older({enabled: false})), 'off');
});

test('the switch shows the library’s own choice', () => {
  assert.equal(lookupsOn(parseScreenLookup(server())), true);
  assert.equal(lookupsOn(parseScreenLookup(server({status: 'disabled', enabled: false}))), false);
  assert.equal(lookupsOn(parseScreenLookup(server({status: 'needs_consent', confirmed: false, enabled: true}))), false, 'waiting for consent: on grants it');
  assert.equal(lookupsOn(parseScreenLookup(server({status: 'declined', confirmed: false, enabled: true}))), true, 'withdrawn server-wide: the library keeps its preference');
});

test('the per-library toggle sends its settings back as they are and never touches consent', () => {
  const off = lookupUpdate(parseScreenLookup(server({providers: ['tmdb'], language: 'ja', region: 'JP', refreshMode: 'fill_missing'})), false);
  assert.deepEqual(off, {expectedRevision: 3, expectedConsentRevision: 2, enabled: false, providers: ['tmdb'], language: 'ja', region: 'JP', refreshMode: 'fill_missing', disclosureVersion: 'screen-metadata-v1'});
  const on = lookupUpdate(parseScreenLookup(server({status: 'disabled', enabled: false})), true);
  assert.deepEqual(on.providers, [], 'no client-side defaults: the server stores its per-kind ones');
  assert.equal('confirmRemote' in on, false);
  const declined = lookupUpdate(parseScreenLookup(server({status: 'declined', confirmed: false, enabled: false})), true);
  assert.equal('confirmRemote' in declined, false, 'a server-wide withdrawal is not undone from one library');
  const offWaiting = lookupUpdate(parseScreenLookup(server({status: 'needs_consent', confirmed: false})), false);
  assert.equal('confirmRemote' in offWaiting, false, 'never confirmRemote: false');
});

test('turning on from needs_consent re-grants with the disclosure version', () => {
  const body = lookupUpdate(parseScreenLookup(server({status: 'needs_consent', confirmed: false, enabled: false, disclosureVersion: ''})), true);
  assert.equal(body.confirmRemote, true);
  assert.equal(body.disclosureVersion, 'screen-metadata-v1');
  assert.equal(body.enabled, true);
});

test('a 409 reloads and retries once; anything else fails at once', async () => {
  const calls: {method?: string; body?: unknown}[] = [];
  let conflicts = 1;
  const request = async <T,>(_path: string, method?: string, body?: unknown): Promise<T> => {
    calls.push({method, body});
    if (method === 'GET') return server({revision: 4, status: 'disabled', enabled: false}) as T;
    if (conflicts-- > 0) throw Object.assign(new Error('stale'), {status: 409});
    return undefined as T;
  };
  await saveLookup(request, 'lib_1', parseScreenLookup(server({status: 'disabled', enabled: false})), true);
  assert.deepEqual(calls.map(c => c.method), ['PUT', 'GET', 'PUT']);
  assert.equal((calls[2]!.body as {expectedRevision: number}).expectedRevision, 4);
  conflicts = 2; calls.length = 0;
  await assert.rejects(saveLookup(request, 'lib_1', parseScreenLookup(server()), false));
  assert.deepEqual(calls.map(c => c.method), ['PUT', 'GET', 'PUT'], 'only one retry');
  const failing = async () => { throw Object.assign(new Error('nope'), {status: 403}); };
  await assert.rejects(saveLookup(failing as never, 'lib_1', parseScreenLookup(server()), false), /nope/);
});

test('a reply without revisions or flags is refused', () => {
  assert.throws(() => parseScreenLookup({}));
  assert.throws(() => parseScreenLookup(server({revision: 0})));
  assert.throws(() => parseScreenLookup(server({confirmed: 'yes'})));
  assert.equal(parseScreenLookup(server({region: 'us'})).region, '', 'a malformed region is dropped, not sent back');
});
