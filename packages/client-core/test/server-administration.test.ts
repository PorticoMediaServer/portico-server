import {test} from 'node:test';
import assert from 'node:assert/strict';
import {
 parseAccessMember,parseAccessMemberPage,parseLimitsDocument,parseMemberLimits,parseInvitation,parseInvitationPage,
 parseAccessDevicePage,parseAccessAPIKey,parseLogPage,parseLogSettings,parseLogEvent,parseClientLogUploadPage,
 parseConnectivityPolicy,parseConnectivityStatus,grantsTier,managesTier,isAPIKeySecret,apiKeyPrefix,
} from '../src/server-administration.ts';

const limits={maxStreams:1,remoteBitrateKbps:4000,maxContentRating:'PG-13',allowUnrated:true,
 schedule:{timezone:'UTC',windows:[{days:[1,3],startMinute:600,endMinute:780}]},
 channelPolicy:{mode:'deny',channels:['news']},tagPolicy:{deniedLabels:['Spoilers']}};

test('member limits round-trip and reject a rating this client does not know', () => {
 const parsed = parseMemberLimits(limits);
 assert.equal(parsed.maxContentRating, 'PG-13');
 // The retired maxSessions field is never sent: it is absent from the parsed
 // document, so a limits save cannot carry it back to a server that rejects it.
 assert.equal('maxSessions' in parsed, false);
  assert.equal(parsed.channelPolicy.mode, 'deny');
  assert.deepEqual([...parsed.tagPolicy.deniedLabels], ['Spoilers']);
  assert.deepEqual([...parsed.schedule.windows[0].days], [1, 3]);
  assert.throws(() => parseMemberLimits({...limits, maxContentRating: 'XX'}));
  assert.throws(() => parseMemberLimits({...limits, maxStreams: 100000}));
  assert.throws(() => parseMemberLimits({...limits, schedule: {timezone: 'UTC', windows: [{days: [], startMinute: 0, endMinute: 10}]}}));
  // An absent envelope is an unrestricted one, not a parse failure.
  const empty = parseMemberLimits({});
  assert.equal(empty.maxContentRating, '');
  assert.equal(empty.allowUnrated, true);
  assert.deepEqual([...empty.tagPolicy.deniedLabels], []);
});

test('a limits document is pinned to the account it was read for', () => {
  const document = parseLimitsDocument({accountId: 'member', revision: 3, updatedAt: '2026-09-16T12:00:00Z', limits}, 'member');
  assert.equal(document.revision, 3);
  assert.equal(document.updatedAt, '2026-09-16T12:00:00Z');
  assert.throws(() => parseLimitsDocument({accountId: 'other', revision: 1, limits}, 'member'));
  assert.throws(() => parseLimitsDocument({accountId: 'member', revision: 0, limits}));
});

test('a member carries its tier and both revisions', () => {
  const member = parseAccessMember({accountId: 'a', username: 'ada', profileId: 'p', role: 'admin', disabled: false,
    revision: 2, allowedLibraries: ['lib'], limitsRevision: 1, limits});
  assert.equal(member.role, 'admin');
  assert.equal(member.limitsRevision, 1);
  assert.throws(() => parseAccessMember({accountId: 'a', username: 'ada', profileId: 'p', role: 'root', disabled: false, revision: 1, allowedLibraries: [], limitsRevision: 1, limits}));
  const page = parseAccessMemberPage({limit: 25, nextCursor: 'cursor', items: [{accountId: 'a', username: 'ada', profileId: 'p', role: 'member', disabled: false, revision: 1, allowedLibraries: [], limitsRevision: 1, limits}]});
  assert.equal(page.items.length, 1);
  assert.equal(page.nextCursor, 'cursor');
  // A page that returns more rows than it promised is a server disagreeing with
  // itself, which the client refuses rather than renders.
  assert.throws(() => parseAccessMemberPage({limit: 0, items: []}));
});

test('an invitation code is accepted only where the server issues one', () => {
  const created = parseInvitation({id: 'i1', email: 'guest@example.com', role: 'member', allowedLibraries: [],
    state: 'pending', createdAt: '2026-09-16T12:00:00Z', expiresAt: '2026-09-23T12:00:00Z', revision: 1, code: 'secret-code'});
  assert.equal(created.code, 'secret-code');
  const listed = parseInvitation({id: 'i1', email: 'guest@example.com', role: 'member', allowedLibraries: [],
    state: 'accepted', createdAt: '2026-09-16T12:00:00Z', expiresAt: '2026-09-23T12:00:00Z', acceptedAt: '2026-09-17T12:00:00Z', accountId: 'acc', revision: 2});
  assert.equal(listed.code, undefined);
  assert.equal(listed.accountId, 'acc');
  // Link-and-code invitations need no email (Hosted sends none): an empty one is read, not refused.
  assert.equal(parseInvitation({id: 'i2', email: '', role: 'member', allowedLibraries: [], state: 'pending', createdAt: '2026-09-16T12:00:00Z', expiresAt: '2026-09-23T12:00:00Z', revision: 1}).email, '');
  assert.throws(() => parseInvitation({id: 'i1', email: 'guest@example.com', role: 'member', allowedLibraries: [], state: 'unknown', createdAt: '2026-09-16T12:00:00Z', expiresAt: '2026-09-23T12:00:00Z', revision: 1}));
  assert.throws(() => parseInvitation({id: 'i1', email: 'guest@example.com', role: 'member', allowedLibraries: [], state: 'pending', createdAt: 'not-a-date', expiresAt: '2026-09-23T12:00:00Z', revision: 1}));
  const page = parseInvitationPage({limit: 2, items: []});
  assert.deepEqual([...page.items], []);
});

test('a device page carries the approval policy that governs it', () => {
  const page = parseAccessDevicePage({limit: 25, approvalRequired: true, items: [{id: 'living-room', accountId: 'a',
    profileId: 'p', name: 'Living Room', platform: 'tvos', trust: 'pending', firstSeenAt: '2026-09-16T12:00:00Z', lastSeenAt: '2026-09-16T12:30:00Z', revision: 1}]});
  assert.equal(page.approvalRequired, true);
  assert.equal(page.items[0].trust, 'pending');
  assert.throws(() => parseAccessDevicePage({limit: 25, approvalRequired: true, items: [{id: 'x', accountId: 'a', trust: 'trusted', firstSeenAt: '2026-09-16T12:00:00Z', lastSeenAt: '2026-09-16T12:00:00Z', revision: 1}]}));
});

test('an API key secret must carry the key prefix, and a listing must not carry one', () => {
  const created = parseAccessAPIKey({id: 'k', name: 'Automation', scope: 'playback', accountId: 'a', hint: 'abc123',
    createdAt: '2026-09-16T12:00:00Z', revoked: false, revision: 1, secret: apiKeyPrefix + 'xyz'});
  assert.equal(created.secret, apiKeyPrefix + 'xyz');
  assert.ok(isAPIKeySecret(created.secret!));
  assert.ok(!isAPIKeySecret('session-token'));
  // A "secret" that is not a key is a response this client will not trust.
  assert.throws(() => parseAccessAPIKey({id: 'k', name: 'n', scope: 'full', accountId: 'a', hint: 'h', createdAt: '2026-09-16T12:00:00Z', revoked: false, revision: 1, secret: 'plain'}));
  assert.throws(() => parseAccessAPIKey({id: 'k', name: 'n', scope: 'god-mode', accountId: 'a', hint: 'h', createdAt: '2026-09-16T12:00:00Z', revoked: false, revision: 1}));
  const listed = parseAccessAPIKey({id: 'k', name: 'n', scope: 'read-only', accountId: 'a', hint: 'h',
    createdAt: '2026-09-16T12:00:00Z', lastUsedAt: '2026-09-16T13:00:00Z', revoked: true, revision: 2});
  assert.equal(listed.secret, undefined);
  assert.equal(listed.lastUsedAt, '2026-09-16T13:00:00Z');
});

test('log records keep their sequence as a string so a long-running server does not lose precision', () => {
  const page = parseLogPage({level: 'warn', effectiveLevel: 'debug', limit: 100, nextCursor: '9007199254740993',
    items: [{sequence: '9007199254740993', at: '2026-09-16T12:00:00Z', level: 'error', category: 'playback', message: 'stalled'}]});
  assert.equal(page.items[0].sequence, '9007199254740993');
  assert.equal(page.effectiveLevel, 'debug');
  assert.throws(() => parseLogPage({level: 'warn', effectiveLevel: 'warn', limit: 10, items: [{sequence: 'abc', at: '2026-09-16T12:00:00Z', level: 'error', category: 'server', message: 'x'}]}));
  assert.throws(() => parseLogPage({level: 'chatty', effectiveLevel: 'warn', limit: 10, items: []}));
  const record = parseLogEvent('{"sequence":"4","at":"2026-09-16T12:00:00Z","level":"info","category":"scan","message":"walking"}');
  assert.equal(record.category, 'scan');
  assert.throws(() => parseLogEvent('not json'));
});

test('log settings publish the level in force and any open debug window', () => {
  const settings = parseLogSettings({revision: 4, logLevel: 'info', effectiveLevel: 'debug', debugWindowUntil: '2026-09-16T12:05:00Z',
    retention: [{category: 'scan', days: 2}], levels: ['error', 'warn', 'info', 'debug'], categories: ['server', 'scan']});
  assert.equal(settings.effectiveLevel, 'debug');
  assert.equal(settings.retention[0].days, 2);
  assert.equal(settings.debugWindowUntil, '2026-09-16T12:05:00Z');
  // No window open: the field is simply absent.
  const quiet = parseLogSettings({revision: 4, logLevel: 'info', effectiveLevel: 'info', retention: [], levels: [], categories: []});
  assert.equal(quiet.debugWindowUntil, undefined);
  assert.throws(() => parseLogSettings({revision: 4, logLevel: 'info', effectiveLevel: 'info', retention: [{category: 'nope', days: 1}], levels: [], categories: []}));
  assert.throws(() => parseLogSettings({revision: 4, logLevel: 'info', effectiveLevel: 'info', retention: [{category: 'scan', days: 0}], levels: [], categories: []}));
});

test('a client log listing carries metadata without bodies', () => {
  const page = parseClientLogUploadPage({limit: 25, items: [{id: 'u1', accountId: 'a', profileId: 'p', deviceId: 'd',
    platform: 'tvos', appVersion: '1.0', receivedAt: '2026-09-16T12:00:00Z', bytes: 42}]});
  assert.equal(page.items[0].body, undefined);
  assert.equal(page.items[0].bytes, 42);
});

test('connectivity policy and status parse the whole report', () => {
  const policy = {revision: 7, remoteSignInPolicy: 'owner-only', remoteBitrateLimitKbps: 8000,
    secureConnectionsPolicy: 'required', lanNetworks: ['192.168.1.0/24'], accessUrls: ['https://media.example.com'], lanDiscoveryEnabled: false};
  const parsed = parseConnectivityPolicy(policy);
  assert.equal(parsed.remoteSignInPolicy, 'owner-only');
  assert.equal(parsed.lanDiscoveryEnabled, false);
  assert.throws(() => parseConnectivityPolicy({...policy, secureConnectionsPolicy: 'whatever'}));
  assert.throws(() => parseConnectivityPolicy({...policy, remoteBitrateLimitKbps: -1}));
  const status = parseConnectivityStatus({observedAt: '2026-09-16T12:00:00Z', policy,
    tls: {configured: true, state: 'active', tlsReady: true, publiclyTrusted: false, listenerBound: true, listenPort: 8443, notAfter: '2026-12-01T00:00:00Z'},
    discovery: {enabled: false, supported: true, state: 'stopped', serviceType: '_portico._tcp.local.'},
    addresses: [{address: '192.168.1.20', family: 'ipv4', class: 'lan', source: 'interface'}],
    interfaces: [{name: 'en0', up: true, loopback: false, addresses: ['192.168.1.20/24']}],
    warnings: ['secure_connections_required_without_tls']});
  assert.equal(status.tls.listenPort, 8443);
  assert.equal(status.discovery.state, 'stopped');
  assert.equal(status.addresses[0].class, 'lan');
  assert.equal(status.warnings.length, 1);
  assert.throws(() => parseConnectivityStatus({observedAt: '2026-09-16T12:00:00Z', policy,
    tls: {configured: true, tlsReady: true, publiclyTrusted: false, listenerBound: true, listenPort: 8443},
    discovery: {enabled: true, supported: true, state: 'humming'}, addresses: [], interfaces: [], warnings: []}));
});

test('CD-39: a host-sized report renders bounded slices with exact omitted counts', () => {
  const policy = {revision: 7, remoteSignInPolicy: 'allow', remoteBitrateLimitKbps: 0,
    secureConnectionsPolicy: 'preferred', lanNetworks: [], accessUrls: [], lanDiscoveryEnabled: false};
  const base = {observedAt: '2026-09-16T12:00:00Z', policy,
    tls: {configured: false, tlsReady: false, publiclyTrusted: false, listenerBound: false, listenPort: 0},
    discovery: {enabled: false, supported: false, state: 'unavailable'}, warnings: []};
  const address = (n: number) => ({address: `10.0.0.${n % 256}`, family: 'ipv4', class: 'lan', source: 'interface'});
  const iface = (n: number) => ({name: `veth${n}`, up: true, loopback: false, addresses: [`10.0.0.${n % 256}/24`]});
  const status = parseConnectivityStatus({...base,
    addresses: Array.from({length: 257}, (_, n) => address(n)),
    interfaces: Array.from({length: 129}, (_, n) => iface(n))});
  assert.equal(status.addresses.length, 256);
  assert.equal(status.interfaces.length, 128);
  assert.equal(status.omittedAddresses, 1);
  assert.equal(status.omittedInterfaces, 1);
  // Retained entries are still validated.
  assert.throws(() => parseConnectivityStatus({...base, addresses: [{...address(0), family: 'carrier-pigeon'}], interfaces: []}));
  // The policy enums stay strict.
  assert.throws(() => parseConnectivityStatus({...base, policy: {...policy, remoteSignInPolicy: 'sometimes'}, addresses: [], interfaces: []}));
});

test('the tier helpers mirror the server predicate', () => {
  assert.ok(grantsTier('owner', 'admin'));
  assert.ok(grantsTier('admin', 'admin'));
  assert.ok(!grantsTier('member', 'admin'));
  assert.ok(!grantsTier('admin', 'owner'));
  assert.ok(!grantsTier('root', 'member'));
  assert.ok(managesTier('owner', 'admin'));
  assert.ok(managesTier('admin', 'member'));
  assert.ok(!managesTier('admin', 'admin'));
  assert.ok(!managesTier('owner', 'owner'));
  assert.ok(!managesTier('member', 'member'));
});

test('connectivity policy: network parity rows parse, with shipped defaults for an older server', () => {
  const base = {revision: 4, remoteSignInPolicy: 'allow', remoteBitrateLimitKbps: 0, secureConnectionsPolicy: 'preferred', lanNetworks: [], accessUrls: ['https://media.example.com'], lanDiscoveryEnabled: true};
  const older = parseConnectivityPolicy(base);
  assert.deepEqual([older.trustedProxies, older.treatWanAsLan, older.uploadCapacityKbps, older.pausedSessionTimeoutMinutes, older.advertisedInterface, older.customCertificateDomain], [[], true, 0, 0, '', '']);
  const newer = parseConnectivityPolicy({...base, trustedProxies: ['127.0.0.1/32'], treatWanAsLan: false, uploadCapacityKbps: 20000, pausedSessionTimeoutMinutes: 60, advertisedInterface: 'br0',
    customCertificatePath: '/certs/full.pem', customCertificateKeyPath: '/certs/key.pem', customCertificateDomain: 'media.example.com'});
  assert.equal(newer.treatWanAsLan, false);
  assert.equal(newer.uploadCapacityKbps, 20000);
  assert.equal(newer.advertisedInterface, 'br0');
  assert.equal(newer.customCertificateKeyPath, '/certs/key.pem');
  assert.throws(() => parseConnectivityPolicy({...base, pausedSessionTimeoutMinutes: 5000}));
});
