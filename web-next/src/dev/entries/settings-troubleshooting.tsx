import {createRoot} from 'react-dom/client';
import {createI18n, deviceRegion, resolveRegion} from '@i18n';
import {setServiceI18n} from '@core/presentation/service-text.ts';
import {IconSprite} from '../../kit/Icon';
import {ReportsPanel} from '../../screens/settings/server/troubleshooting/Reports';
import {SettingsReportsSource} from '../../screens/settings/server/troubleshooting/reports-source';
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
const setAside = {failures: [
  {domain: 1, title: 'Severance, season 1', reason: 'The provider answered too slowly to finish the match.', attempts: 3, failedAt: '2026-10-22T17:10:00Z'},
  {domain: 1, title: 'Neagley', reason: 'The artwork could not be read as an image.', attempts: 2, failedAt: '2026-10-22T17:10:00Z'},
], more: false};
const request = async (path: string, method = 'GET'): Promise<unknown> => {
  if (path.includes('/console/diagnostics/')) return {scope: {serverId, viewerFence: fence}, data: {items: records, nextCursor: ''}};
  if (path.includes('/catalogue/failures')) return setAside;
  if (method === 'DELETE') return undefined;
  if (path.includes('client-logs/')) return envelope({...report('one', 'iPhone', 'phone', '1.4.2', '2026-10-21T22:44:00Z', 38912), body: 'The app closed while opening a title.'});
  return envelope({items: [
    report('one', 'iPhone', 'phone', '1.4.2', '2026-10-21T22:44:00Z', 38912),
    report('two', 'Android TV', 'tv', '1.4.1', '2026-10-20T21:10:00Z', 114688),
    report('three', 'Chrome', 'desktop', '1.4.2', '2026-10-18T23:33:00Z', 21504),
  ], limit: 25, nextCursor: ''});
};
const transport = {request: <T,>(path: string, method?: string) => request(path, method) as Promise<T>};
const source = new SettingsReportsSource(transport, serverId, 'specimen-viewer', () => true);

createRoot(document.getElementById('root')!).render(<div style={{minHeight: '100vh', background: '#071018', padding: '40px 32px'}}>
  <div style={{maxWidth: 952, margin: '0 auto'}}><IconSprite />
    <ReportsPanel source={source} subscribe={source.subscribe} isCurrent={() => true} downloadBase="" reportError={() => {}} />
  </div>
</div>);
