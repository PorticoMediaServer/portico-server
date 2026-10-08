import React, {useMemo, useState} from 'react';
import {browserClientProfile} from '../../bridge/client-profile';
import {currentI18n} from '../../app/i18n';
import {SessionFixture} from '../../app/session';
import {GroupSessionService} from '@core/index.ts';
import {TogetherFixture} from '../../app/together';
import {TogetherScreen} from '../together/Together';
import {InboxProvider} from '../../app/inbox';
import {NotificationsSection} from '../settings/Notifications';
import {FeedbackDialog} from '../shared/FeedbackDialog';
import {DevicesSection, ProfileRestrictionsDialog, TwoFactorSection} from '../settings/Security';
import {Button, Inset, Page, PageHeader, Section, Segmented} from '../../ui';

/** Development-only: feature screens mounted against canned server responses, so they can be
 * seen and exercised without a server or a sign-in. `failing` makes every request fail, to
 * check that a screen keeps what it has already loaded. Deliberate i18n exception (M12):
 * fixture and sample copy here never ships to users, so its lines carry `lint-strings-allow`
 * instead of catalogue keys. */
const now = Date.now();
const notice = (id: string, over: Record<string, unknown> = {}): Record<string, any> => ({id, audience: 'profile', severity: 'info', source: 'downloads', category: 'download.finished', title: 'Download ready', body: '“The Long Way Home” finished downloading and is ready to watch offline.', arguments: {}, actions: [{kind: 'navigate', label: 'Open settings', target: {view: 'settings'}}], dedupeKey: id, revision: 7, createdAt: now - 12 * 60000, updatedAt: now, expiresAt: now + 9e9, readAt: null, archivedAt: null, read: false, archived: false, cursor: id, ...over}); // lint-strings-allow: developer-only fixture
const notices = [
  notice('n1'),
  notice('n2', {severity: 'warning', source: 'storage', title: 'Storage is almost full', body: 'The Movies drive has 4% free. New recordings may fail.', createdAt: now - 3 * 3600000, actions: []}), // lint-strings-allow: developer-only fixture
  notice('n3', {severity: 'critical', source: 'security', title: 'New device signed in', body: 'An Apple TV signed in to your account from a new location.', createdAt: now - 26 * 3600000, read: true, readAt: now, actions: []}), // lint-strings-allow: developer-only fixture
  notice('n4', {source: 'feedback', title: 'Reply to your report', body: 'Thanks. The file was re-encoded; it should play smoothly now.', createdAt: now - 4 * 86400000, read: true, readAt: now, actions: []}), // lint-strings-allow: developer-only fixture
];
const capabilities = {revision: 'e1.0', canSubmit: true, kinds: [
  {id: 'playback', label: 'Playback', categories: [{id: 'buffering', label: 'Buffering or stuttering', description: 'It keeps pausing, skipping or stuttering.', wantsPlaybackSession: true, wantsItem: true}, {id: 'audio', label: 'Audio problem', description: 'Wrong language, out of sync, or no sound.', wantsPlaybackSession: true, wantsItem: true}]}, // lint-strings-allow: developer-only fixture
  {id: 'metadata', label: 'Title information', categories: [{id: 'wrong-match', label: 'Wrong title or artwork', description: 'This is matched to the wrong film or show.', wantsPlaybackSession: false, wantsItem: true}]}, // lint-strings-allow: developer-only fixture
  {id: 'other', label: 'Something else', categories: [{id: 'general', label: 'General', description: 'Anything that does not fit above.', wantsPlaybackSession: false, wantsItem: false}]}, // lint-strings-allow: developer-only fixture
], maxMessageLength: 2000, minMessageLength: 8, diagnosticsSupported: true, diagnosticsOptional: true, duplicateWindowHours: 24, retentionDays: 180, statuses: ['open', 'in-progress', 'resolved', 'closed'], diagnosticsDecisions: ['attached', 'unavailable', 'declined', 'not-requested'], perProfileHourlyLimit: 10, reporterName: 'Sam', reporterAuthority: 'local'};

const restrictions = {profileId: 'prof_kid', ratingSystem: 'mpaa', maximumAgeRating: 'PG', maximumAge: 8, allowUnrated: false, blockedLabels: ['horror'], allowDownloads: true, allowLiveTv: true, allowDvr: false, allowWatchTogether: false, revision: 1};
const device = (id: string, over: Record<string, unknown>) => ({id, installationId: 'i'.repeat(32) + id, name: '', platform: 'web', app: 'Portico Web', appVersion: '0.1.0', ip: '192.168.2.41', firstSeen: new Date(now - 40 * 86400000).toISOString(), lastSeen: new Date(now - 60000).toISOString(), trusted: true, approvalState: 'approved', lastProfileId: 'prof_kid', rememberAccount: true, current: false, sessions: 1, ...over});
const devices = [device('d1', {name: 'Living room', platform: 'tvos', app: 'Portico', current: false, lastSeen: new Date(now - 2 * 3600000).toISOString()}), device('d2', {name: 'This Mac', current: true}), device('d3', {name: 'Sam’s iPhone', platform: 'ios', app: 'Portico', trusted: false, approvalState: 'pending', sessions: 0, ip: '86.12.4.200', lastSeen: new Date(now - 5 * 60000).toISOString()})];

const member = (id: string, displayName: string, over: Record<string, unknown> = {}) => ({id, displayName, role: 'member', state: 'joined', readiness: 'ready', positionUs: '0', reportedAt: new Date().toISOString(), presence: 'connected', joinedAt: new Date().toISOString(), ...over});
const room = {
  queue: {revision: '2', position: 0, entries: [{entryId: 'ent_1', position: 0, itemId: 'item_1', unavailable: false, addedBy: 'mem_1'}, {entryId: 'ent_2', position: 1, itemId: 'item_2', unavailable: false, addedBy: 'mem_2'}, {entryId: 'ent_3', position: 2, itemId: null, unavailable: true, addedBy: ''}], eligibility: {blockedEntries: 1, currentEntryBlocked: false, members: [{memberId: 'mem_3', displayName: 'Robin', blockedEntryIds: ['ent_2']}]}},
  group: {
    id: 'grp_1', name: 'Movie night', state: 'paused', hostAuthority: 'host-only', hostMemberId: 'mem_1', revision: '7', playbackRevision: '3', queueRevision: '2', reconnectGeneration: '1', lastCommand: 'pause', lastCommandId: 'p', endedReason: '', eventOrdinal: '9',
    createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), permissions: {isHost: true, canControl: true, canManageQueue: true},
    authority: {deviceId: 'dev_1', playbackId: null, state: 'fresh'},
    host: {presence: 'connected', lastSeenAt: new Date().toISOString(), pauseAt: null, endAt: null},
    timeline: {itemId: 'item_1', currentEntryId: 'ent_1', state: 'paused', anchorPositionUs: '754000000', anchorAt: new Date().toISOString(), rate: {numerator: '1', denominator: '1'}, queuePosition: 0},
    settings: {shuffleEnabled: false, repeatMode: 'none'}, sync: {noCorrectionUnderMs: 750, rateCorrectionMinimum: '0.90', rateCorrectionMaximum: '1.10', rateCorrectionMaxMs: 4000, seekAtOrOverMs: 3000},
    readiness: {aggregate: 'buffering', ready: 2, buffering: 1, lagging: 0, stale: 0, memberCount: 3},
    members: [member('mem_1', 'Sam', {role: 'host'}), member('mem_2', 'Alex'), member('mem_3', 'Robin', {readiness: 'buffering'})], queue: null, viewerMemberId: 'mem_1',
  } as Record<string, any>,
};
const roomNow = () => ({protocolVersion: '1.0', serverTime: new Date().toISOString(), group: room.group});
const fixtureTitles: Record<string, string> = {item_1: 'The Long Goodbye', item_2: 'Paper Moon'};

function fixtureApi(failing: () => boolean) {
  let items: Record<string, any>[] = notices.map(n => ({...n}));
  const counts = () => ({unread: items.filter(n => !n.read && !n.archived).length, read: items.filter(n => n.read && !n.archived).length, archived: items.filter(n => n.archived).length, total: items.length});
  return {
    baseUrl: 'http://fixture.invalid',
    item: async (id: string) => { if (!fixtureTitles[id]) throw new Error('Not found'); return {id, libraryId: 'lib', title: fixtureTitles[id], kind: 'movie', duration: 6720, progressSeconds: 0, available: true}; },
    request: async (path: string, method = 'GET', body?: any) => {
      await new Promise(r => setTimeout(r, 250));
      if (failing()) throw new TypeError('Failed to fetch');
      if (path.startsWith('/v1/notifications/unread-count')) return {data: {revision: 7, audience: 'profile', counts: path.includes('account-admin') ? {unread: 1, read: 0, archived: 0, total: 1} : counts(), observedAt: now}};
      if (path.startsWith('/v1/notifications/inbox/actions')) {
        for (const op of body.operations) items = items.map(n => (op.action === 'read-all' || op.ids?.includes(n.id) ? {...n, ...(op.action === 'read' || op.action === 'read-all' ? {read: true, readAt: now} : op.action === 'unread' ? {read: false, readAt: null} : op.action === 'archive' ? {archived: true, archivedAt: now} : {archived: false, archivedAt: null})} : n));
        return {data: {revision: 8, audience: 'profile', applied: 1, receipts: [{action: body.operations[0].action, outcome: 'applied'}], counts: counts()}};
      }
      if (path.startsWith('/v1/notifications/inbox')) {
        const view = new URL('http://x' + path).searchParams.get('state');
        const visible = items.filter(n => (view === 'archived' ? n.archived : view === 'unread' ? !n.archived && !n.read : !n.archived));
        return {data: {revision: 7, audience: 'profile', audiences: ['profile', 'account-admin'], state: view, counts: counts(), items: visible, nextCursor: '', observedAt: now, retentionDays: 180}};
      }
      if (path === '/v1/rating-systems') return {items: [{id: 'mpaa', name: 'MPA', region: 'US', values: [{code: 'G', label: 'G', minimumAge: 0}, {code: 'PG', label: 'PG', minimumAge: 8}, {code: 'PG-13', label: 'PG-13', minimumAge: 13}, {code: 'R', label: 'R', minimumAge: 17}], screens: ['movie']}, {id: 'bbfc', name: 'BBFC', region: 'GB', values: [{code: 'U', label: 'U', minimumAge: 0}, {code: '12', label: '12', minimumAge: 12}, {code: '15', label: '15', minimumAge: 15}, {code: '18', label: '18', minimumAge: 18}], screens: ['movie']}]}; // lint-strings-allow: developer-only fixture
      if (path.endsWith('/restrictions') && method === 'GET') return restrictions;
      if (path.endsWith('/restrictions') && method === 'PUT') return {...restrictions, ...body, revision: 2, maximumAge: 13};
      if (path.startsWith('/v1/devices?')) return {items: devices};
      if (path === '/v1/direct/two-factor' && method === 'GET') return {enabled: false, pendingEnrolment: false, recoveryCodesRemaining: 0};
      if (path === '/v1/direct/two-factor/enrol') return {secret: 'JBSWY3DPEHPK3PXPJBSWY3DP', uri: 'otpauth://totp/Portico:sam?secret=JBSWY3DPEHPK3PXPJBSWY3DP&issuer=Portico', recoveryCodes: ['4f9a-22kd', '81mm-0zqp', 'c7e2-ht5n', 'p0x3-9vva', 'w2bb-61rs', 'j8dn-4key', 'm1ty-73cu', 'z5qo-e8fl']};
      if (path === '/v1/direct/two-factor/verify') return {enabled: true, pendingEnrolment: false, recoveryCodesRemaining: 8};
      if (path === '/v1/feedback/capabilities') return {data: capabilities};
      if (path === '/v1/feedback/reports') return {data: {report: {id: 'r1', cursor: '1', revision: 1, status: 'open', kind: body.kind, category: body.category, message: body.message, createdAt: now, updatedAt: now, expiresAt: now + 9e9, reporter: {name: 'Sam', authority: 'local', role: 'member', self: true}, diagnostics: {decision: body.attachDiagnostics ? 'attached' : 'declined'}, duplicates: 0, thread: []}, duplicate: false, created: true}};
      if (path === '/v1/groups' && method === 'GET') return {protocolVersion: '1.0', serverTime: new Date().toISOString(), groups: [room.group]};
      if (path === '/v1/groups' && method === 'POST') return roomNow();
      if (path === '/v1/groups/join') { if (body.code !== 'ABCDEFGH') throw Object.assign(new Error('That code isn’t right, or it has expired.'), {status: 404, code: 'invite_not_found'}); return roomNow(); }
      if (path.endsWith('/transport')) { const playing = body.command === 'play'; room.group = {...room.group, state: playing ? 'playing' : 'paused', revision: String(Number(room.group.revision) + 1), timeline: {...room.group.timeline, state: playing ? 'playing' : 'paused', anchorAt: new Date().toISOString()}}; return {protocolVersion: '1.0', serverTime: new Date().toISOString(), groupId: 'grp_1', idempotencyKey: body.idempotencyKey, disposition: 'accepted', command: body.command, revision: room.group.revision, queueRevision: '2', recordedAt: new Date().toISOString(), timeline: room.group.timeline, settings: room.group.settings, override: null}; }
      if (path.endsWith('/invites')) return {protocolVersion: '1.0', serverTime: new Date().toISOString(), invite: {id: 'inv_1', groupId: 'grp_1', code: 'ABCDEFGH', expiresAt: new Date(Date.now() + 900000).toISOString(), maxUses: 8, uses: 0, recipientProfileId: ''}};
      if (path.endsWith('/queue')) return {protocolVersion: '1.0', serverTime: new Date().toISOString(), groupId: 'grp_1', queue: room.queue};
      if (path.startsWith('/v1/groups/')) return roomNow();
      throw new Error('No fixture for ' + path);
    },
  };
}

/** The capability document this browser would publish, asked live. */
function DeviceProfile() {
  const [text, setText] = useState('Asking the browser…');
  React.useEffect(() => { void browserClientProfile(true).then(p => setText(JSON.stringify(p, null, 2)), e => setText(String(e))); }, []);
  return <pre style={{fontSize: 12, lineHeight: 1.5, whiteSpace: 'pre-wrap', userSelect: 'text'}}>{text}</pre>;
}

export function FeaturesGallery() {
  const [failing, setFailing] = useState(false);
  const flag = React.useRef(false);
  flag.current = failing;
  const api = useMemo(() => fixtureApi(() => flag.current), []);
  // The stream never answers, as when a proxy swallows it: the room must still work from re-reads.
  const together = useMemo(() => new GroupSessionService({api: api as never, stream: (_p, _l, signal) => new Promise<Response>((_, reject) => signal.addEventListener('abort', () => reject(new Error('aborted'))))}), [api]);
  const [feedback, setFeedback] = useState(false);
  const [limits, setLimits] = useState(false);
  const [surface, setSurface] = useState('inbox');
  return (
    <SessionFixture value={{api: api as never, session: {accessToken: 'fixture'} as never, owner: true}}>
      <InboxProvider>
        <Page>
          <PageHeader title="Feature gallery" eyebrow="Development" /> {/* lint-strings-allow: developer-only screen */}
          <Inset>
            <div style={{display: 'flex', gap: 12, flexWrap: 'wrap', marginBottom: 20}}>
              <Segmented label="Surface" value={surface} onChange={setSurface} options={[{id: 'inbox', label: 'Inbox'}, {id: 'feedback', label: 'Feedback'}, {id: 'security', label: 'Account security'}, {id: 'together', label: 'Watch Together'}, {id: 'profile', label: 'Device profile'}]} /> {/* lint-strings-allow: developer-only screen */}
              <Button variant={failing ? 'danger' : 'outline'} size="sm" label={failing ? currentI18n().t('web.dev.requestsFailing') : currentI18n().t('web.dev.makeRequestsFail')} onClick={() => setFailing(f => !f)} />
            </div>
            {surface === 'together' ? <TogetherFixture value={together}><TogetherScreen /></TogetherFixture> : null}
            {surface === 'profile' ? <Section title="What this browser tells its server it can play"><DeviceProfile /></Section> : null} {/* lint-strings-allow: developer-only screen */}
            {surface === 'inbox' ? <Section title="Notifications"><NotificationsSection /></Section> : null} {/* lint-strings-allow: developer-only screen */}
            {surface === 'security' ? <div style={{display: 'flex', flexDirection: 'column', gap: 20, maxWidth: 720}}><Button variant="secondary" label="Limits for Robin (password: correct)" onClick={() => setLimits(true)} /><TwoFactorSection /><DevicesSection />{limits ? <ProfileRestrictionsDialog profile={{id: 'prof_kid', name: 'Robin'}} onClose={() => setLimits(false)} /> : null}</div> : null} {/* lint-strings-allow: developer-only screen */}
            {surface === 'feedback' ? <Section title="Feedback"><Button variant="primary" label="Report a problem (from the player)" onClick={() => setFeedback(true)} />{feedback ? <FeedbackDialog open onOpenChange={setFeedback} itemId="item-1" playbackSessionId="play-1" title="The Long Way Home" /> : null}</Section> : null} {/* lint-strings-allow: developer-only screen */}
          </Inset>
        </Page>
      </InboxProvider>
    </SessionFixture>
  );
}
