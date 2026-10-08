import { settingsSeed } from '../../../data/fixtures/server';
import { useAdminSection } from '../../../services/queries';
import { api } from '../../../services/client';
import { ErrorState, Skel, Switch } from '../../../components/ui';
import { toast } from '../../../components/overlay';
import { Panel, SettingRow } from '../ui';

type S = typeof settingsSeed;

export function ServerSettings() {
  const q = useAdminSection<S>('settings');
  if (q.isError) return <ErrorState error={q.error} retry={() => q.refetch()} />;
  if (!q.data) return <Skel h={400} />;
  const d = q.data;
  const save = (patch: Partial<S>) => api.updateAdminSection('settings', patch).then(() => (q.refetch(), toast('Server settings saved')));
  return (
    <div className="cstack">
      <div className="cgrid two">
        <Panel title="General">
          <SettingRow title="Server name" sub="Shown on every device and in invitations">
            <input className="input sm" style={{ width: 200 }} defaultValue={d.friendlyName} onBlur={(e) => e.target.value !== d.friendlyName && save({ friendlyName: e.target.value })} />
          </SettingRow>
          <SettingRow title="Language">
            <span className="t-sub">{d.language}</span>
          </SettingRow>
          <SettingRow title="Time zone" sub="Used for schedules, the guide and DVR">
            <span className="t-mono">{d.timezone}</span>
          </SettingRow>
          <SettingRow title="DLNA server" sub="Lets smart TVs and consoles browse Harbor without an app">
            <Switch size="sm" accent checked={d.dlna} onChange={(v) => save({ dlna: v })} label="DLNA" />
          </SettingRow>
        </Panel>
        <Panel title="Updates">
          <SettingRow title="Channel">
            <select className="input sm" value={d.updateChannel} onChange={(e) => save({ updateChannel: e.target.value })}>
              <option>Stable</option>
              <option>Beta</option>
            </select>
          </SettingRow>
          <SettingRow title="Install updates">
            <select className="input sm" value={d.autoUpdate} onChange={(e) => save({ autoUpdate: e.target.value })}>
              <option>Notify only</option>
              <option>Automatically at 04:00</option>
            </select>
          </SettingRow>
          <SettingRow title="Anonymous usage statistics" sub="Off by default. Nothing about your media ever leaves Harbor.">
            <Switch size="sm" accent checked={d.telemetry} onChange={(v) => save({ telemetry: v })} label="Telemetry" />
          </SettingRow>
          <SettingRow title="Send crash reports">
            <Switch size="sm" accent checked={d.crashReports} onChange={(v) => save({ crashReports: v })} label="Crash reports" />
          </SettingRow>
        </Panel>
      </div>
      <Panel title="Sign-in & security">
        <SettingRow title="Direct sign-in" sub="Accounts live on Harbor. Works with no internet at all.">
          <Switch size="sm" accent checked={d.directSignIn} onChange={(v) => save({ directSignIn: v })} label="Direct sign-in" />
        </SettingRow>
        <SettingRow title="Portico cloud account" sub="Optional: lets people find Harbor from anywhere by signing in once">
          <Switch size="sm" accent checked={d.cloudAccount} onChange={(v) => save({ cloudAccount: v })} label="Cloud account" />
        </SettingRow>
        <SettingRow title="Require sign-in on the local network">
          <Switch size="sm" accent checked={d.requireAuthOnLan} onChange={(v) => save({ requireAuthOnLan: v })} label="Require sign-in on LAN" />
        </SettingRow>
        <SettingRow title="Guest access" sub="Visitors on your Wi-Fi can watch Movies without an account">
          <Switch size="sm" accent checked={d.guestAccess} onChange={(v) => save({ guestAccess: v })} label="Guest access" />
        </SettingRow>
        <SettingRow title="Session length">
          <select className="input sm" value={d.sessionLength} onChange={(e) => save({ sessionLength: e.target.value })}>
            <option>30 days</option>
            <option>90 days</option>
            <option>1 year</option>
          </select>
        </SettingRow>
        <SettingRow title="Sign-in rate limit">
          <span className="t-sub">{d.loginRateLimit}</span>
        </SettingRow>
      </Panel>
    </div>
  );
}
