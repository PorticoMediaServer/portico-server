import { CheckCircle2, Globe, RefreshCw, XCircle } from 'lucide-react';
import { networkSeed } from '../../../data/fixtures/server';
import { useAdminSection } from '../../../services/queries';
import { api } from '../../../services/client';
import { Button, ErrorState, Skel, Switch } from '../../../components/ui';
import { toast } from '../../../components/overlay';
import { ago } from '../../../lib/hooks';
import { Panel, SettingRow } from '../ui';

type N = typeof networkSeed;

export function NetworkSection() {
  const q = useAdminSection<N>('network');
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data) return <Skel h={400} />;
  const d = q.data;
  const save = (patch: Partial<N>) => api.updateAdminSection('network', patch).then(() => (q.refetch(), toast('Network settings saved')));
  return (
    <div className="cstack">
      <div className="remote-hero">
        <Globe />
        <div>
          <div className="t-headline">
            Remote access: <span className="c-ok">{d.remoteAccess}</span>
          </div>
          <div className="t-sub tnum">
            {d.customDomain} → {`73.186.41.207:${d.publicPort}`} · {d.method} · TLS by {d.certificate.issuer}, renews {ago(d.certificate.expires)}
          </div>
        </div>
        <Button size="sm" variant="plain" icon={<RefreshCw />} onClick={() => toast('Reachable from Virginia, Frankfurt and Singapore · 38 ms', <CheckCircle2 />)}>
          Test Now
        </Button>
      </div>
      <Panel title="Connectivity checks">
        <ul className="checks">
          {d.checks.map((c) => (
            <li key={c.name}>
              {c.ok ? <CheckCircle2 className="c-ok" /> : <XCircle className="c-err" />}
              <span>
                <b>{c.name}</b>
                <span className="t-sub">{c.detail}</span>
              </span>
            </li>
          ))}
        </ul>
      </Panel>
      <div className="cgrid two">
        <Panel title="Remote access">
          <SettingRow title="Allow remote connections">
            <Switch size="sm" accent checked={d.remoteAccess !== 'Disabled'} onChange={(v) => save({ remoteAccess: v ? 'Connected' : 'Disabled' })} label="Remote access" />
          </SettingRow>
          <SettingRow title="Public port" sub="Forwarded on your router to 192.168.1.20">
            <input className="input sm mono" style={{ width: 100 }} defaultValue={d.publicPort} />
          </SettingRow>
          <SettingRow title="Configure router with UPnP">
            <Switch size="sm" accent checked={d.upnp} onChange={(v) => save({ upnp: v })} label="UPnP" />
          </SettingRow>
          <SettingRow title="Custom domain">
            <input className="input sm mono" style={{ width: 230 }} defaultValue={d.customDomain} />
          </SettingRow>
          <SettingRow title="Secure connections">
            <select className="input sm" value={d.secureConnections} onChange={(e) => save({ secureConnections: e.target.value })}>
              <option>Required</option>
              <option>Preferred</option>
            </select>
          </SettingRow>
          <SettingRow title="IPv6">
            <Switch size="sm" accent checked={d.ipv6} onChange={(v) => save({ ipv6: v })} label="IPv6" />
          </SettingRow>
        </Panel>
        <Panel title="Bandwidth">
          <SettingRow title="Measured upload">
            <span className="tnum">{d.uploadMbps} Mbps</span>
          </SettingRow>
          <SettingRow title="Total remote limit" sub="Shared across all remote viewers">
            <select className="input sm" value={d.remoteLimitMbps} onChange={(e) => save({ remoteLimitMbps: Number(e.target.value) })}>
              {[10, 20, 40, 80, 150, 0].map((n) => (
                <option key={n} value={n}>
                  {n ? `${n} Mbps` : 'Unlimited'}
                </option>
              ))}
            </select>
          </SettingRow>
          <SettingRow title="Per-stream limit (remote)">
            <select className="input sm" value={d.perStreamLimitMbps} onChange={(e) => save({ perStreamLimitMbps: Number(e.target.value) })}>
              {[4, 8, 12, 20, 0].map((n) => (
                <option key={n} value={n}>
                  {n ? `${n} Mbps` : 'Original'}
                </option>
              ))}
            </select>
          </SettingRow>
          <SettingRow title="Local network">
            <span className="t-sub">{d.lanLimit}</span>
          </SettingRow>
          <SettingRow title="LAN networks" sub="Treated as local: no bandwidth limits">
            <span className="t-mono">{d.lanNetworks.join(', ')}</span>
          </SettingRow>
          <SettingRow title="Networks allowed without sign-in" sub="Leave empty unless you know you need it">
            <span className="t-sub">{d.allowWithoutAuth.length ? d.allowWithoutAuth.join(', ') : 'None'}</span>
          </SettingRow>
        </Panel>
      </div>
    </div>
  );
}
