import {createRoot} from 'react-dom/client';
import {createI18n, deviceRegion, resolveRegion} from '@i18n';
import {setServiceI18n} from '@core/presentation/service-text.ts';
import {IconSprite} from '../../kit/Icon';
import {TroubleshootingPage} from '../../screens/settings/server/troubleshooting';
import {SettingsFeedbackSource} from '../../screens/settings/server/troubleshooting/feedback-source';
import {SettingsReportsSource} from '../../screens/settings/server/troubleshooting/reports-source';
import {SettingsMessagesSource} from '../../screens/settings/server/troubleshooting/messages-source';
import {SettingsSupportSource} from '../../screens/settings/server/troubleshooting/support-source';
import '../../kit/tokens.css';
import '../../kit/fonts.css';
import '../../bootstrap.css';

// Drawn demo facts exist only in this DEV specimen, never in runtime adapters.
// It mounts the production Reports screen; every row is owner-server.html frame 5's own state.
if (!import.meta.env.DEV) throw new Error('Settings specimens require development.');

const params = new URLSearchParams(location.search);
if (params.get('clock') === 'fixed') {
  const fixed = Date.parse('2026-10-22T19:00:00Z'), RealDate = Date;
  const Construct = RealDate as unknown as new (...args: unknown[]) => Date;
  const FakeDate = function (...args: unknown[]) { return args.length ? new Construct(...args) : new Construct(fixed); } as unknown as DateConstructor;
  FakeDate.now = () => fixed;
  FakeDate.parse = RealDate.parse.bind(RealDate);
  FakeDate.UTC = RealDate.UTC.bind(RealDate);
  window.Date = FakeDate;
}
if (params.get('capture') === '1') {
  const style = document.createElement('style');
  style.textContent = '*,*::before,*::after{animation:none!important;transition:none!important}';
  document.head.append(style);
}

setServiceI18n(createI18n(resolveRegion(undefined, deviceRegion())));
const serverId = 'specimen-server', fence = 'a'.repeat(64);
const envelope = (result: unknown) => ({protocolVersion: '1.0', serverId, result});
const report = (id: string, platform: string, deviceClass: string, appVersion: string, receivedAt: string, bytes: number) =>
  ({id, accountId: 'account', profileId: 'profile', deviceId: `device-${id}`, platform, deviceClass, appVersion, receivedAt, bytes});
const records = [
  {sequence: 1842, at: '2026-10-22T18:58:02Z', lane: 'runtime', severity: 'info', component: 'backups', code: 'backup_finished', fields: {kept: 7}},
  {sequence: 1841, at: '2026-10-22T19:03:00Z', lane: 'runtime', severity: 'warn', component: 'remote_access', code: 'port_mapping_failed'},
];
const feedback = [
  {id: 'feedback-1', createdAt: '2026-10-22T18:20:00Z', updatedAt: '2026-10-22T18:20:00Z', revision: 1, status: 'open', category: 'playback',
   message: 'Playback stopped twice during the second episode.', itemId: 'item-1', contextAvailable: true, diagnostic: {platform: 'ios', state: 'playing', online: true}},
  {id: 'feedback-2', createdAt: '2026-10-21T23:05:00Z', updatedAt: '2026-10-22T17:00:00Z', revision: 2, status: 'in-progress', category: 'subtitles',
   message: 'The subtitles are about a second ahead of the picture.', contextAvailable: true, diagnostic: {platform: 'tvos', state: 'playing', online: false}},
  {id: 'feedback-3', createdAt: '2026-10-20T20:15:00Z', updatedAt: '2026-10-21T09:00:00Z', revision: 3, status: 'resolved', category: 'metadata',
   message: 'The poster for this film is wrong.', itemId: 'item-2', contextAvailable: false},
  {id: 'feedback-4', createdAt: '2026-10-19T18:40:00Z', updatedAt: '2026-10-20T10:00:00Z', revision: 2, status: 'closed', category: 'other',
   message: 'Thanks for the update.', contextAvailable: false},
];
const setAside = {failures: [
  {domain: 1, title: 'Severance, season 1', reason: 'The provider answered too slowly to finish the match.', attempts: 3, failedAt: '2026-10-22T17:10:00Z'},
  {domain: 1, title: 'Neagley', reason: 'The artwork could not be read as an image.', attempts: 2, failedAt: '2026-10-22T17:10:00Z'},
], more: false};
const capabilities = {items: [
  {capability: 'live_tv', available: true, checkedAt: '2026-10-22T18:00:00Z'},
  {capability: 'recording', available: true, checkedAt: '2026-10-22T18:00:00Z'},
  {capability: 'prepared_media', available: true, checkedAt: '2026-10-22T18:00:00Z'},
  {capability: 'hardware_encoding', available: false, code: 'hardware_unavailable', detail: 'Intel Quick Sync was requested but this processor has no usable encoder.', checkedAt: '2026-10-22T18:00:00Z'},
  {capability: 'decoder_sandbox', available: false, code: 'decoder_confinement_unavailable', checkedAt: '2026-10-22T18:00:00Z'},
  {capability: 'dolby_vision_conversion', available: true, checkedAt: '2026-10-22T18:00:00Z'},
]};
const alerts = [{id: 'permissions', code: 'state-permissions', status: 'open', severity: 'warning', firstAt: '2026-10-22T17:00:00Z', lastAt: '2026-10-22T18:00:00Z', occurrences: 3, revision: 1}];
const request = async (path: string, method = 'GET'): Promise<unknown> => {
  if (path.includes('/console/diagnostics/')) return {scope: {serverId, viewerFence: fence}, data: {items: records, nextCursor: ''}};
  if (path.includes('/catalogue/failures')) return setAside;
  if (method === 'DELETE') return undefined;
  if (path.includes('/logs/settings')) return {revision: 1, logLevel: 'info', effectiveLevel: 'info', retention: [], levels: ['error','warn','info','debug'], categories: ['server','playback','scan','network','client']};
  if (path.includes('/admin/logs')) return {items: [{sequence: '1', at: '2026-10-22T19:58:02Z', level: 'info', category: 'server', message: 'Backup finished. 18 MB written, 7 backups kept.'}], limit: 200, nextCursor: '', level: 'info', effectiveLevel: 'info'};
  if (path.includes('/diagnostics/capabilities')) return capabilities;
  if (path.includes('/console/alerts')) return {scope: {serverId, viewerFence: fence}, data: alerts};
  if (path.includes('/admin/feedback/')) {
    const id = path.split('/').pop();
    return {scope: {serverId, viewerFence: fence}, data: feedback.find(item => item.id === id) ?? feedback[0]};
  }
  if (path.includes('/admin/feedback')) return {scope: {serverId, viewerFence: fence}, data: {items: feedback, nextCursor: ''}};
  if (path.includes('client-logs/')) return envelope({...report('one', 'iPhone', 'phone', '1.4.2', '2026-10-21T22:44:00Z', 38912), body: 'The app closed while opening a title.'});
  return envelope({items: [
    report('one', 'iPhone', 'phone', '1.4.2', '2026-10-21T22:44:00Z', 38912),
    report('two', 'Android TV', 'tv', '1.4.1', '2026-10-20T21:10:00Z', 114688),
    report('three', 'Chrome', 'desktop', '1.4.2', '2026-10-18T23:33:00Z', 21504),
  ], limit: 25, nextCursor: ''});
};
const transport = {request: <T,>(path: string, method?: string) => request(path, method) as Promise<T>};
const messages = new SettingsMessagesSource(transport, undefined, () => true);
const reports = new SettingsReportsSource(transport, serverId, 'specimen-viewer', () => true);
const feedbackSource = new SettingsFeedbackSource(transport, serverId, 'specimen-viewer', () => true);
const supportSource = new SettingsSupportSource(transport, serverId, 'specimen-viewer', () => true);
const subscribe = (listener: () => void) => {const offs = [reports.subscribe(listener), feedbackSource.subscribe(listener), supportSource.subscribe(listener)];return () => {for (const off of offs) off();};};

createRoot(document.getElementById('root')!).render(<div style={{minHeight: '100vh', background: '#071018', padding: '40px 32px'}}>
  <div style={{maxWidth: 952, margin: '0 auto'}}><IconSprite />
    <TroubleshootingPage messages={messages} reports={reports} feedback={feedbackSource} support={supportSource} subscribe={subscribe} isCurrent={() => true} downloadBase="" reportError={() => {}}
      initialView={params.get('view') === 'messages' ? 'messages' : params.get('view') === 'reports' ? 'reports' : params.get('view') === 'support' ? 'support' : 'feedback'} />
  </div>
</div>);
