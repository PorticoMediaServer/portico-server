import {useEffect, useState} from 'react';
import {recordingOwnerKey, recordingOwnerLabel} from '../../admin/recording-policy';
import {parseEnvelope, parseTunerView} from '@core/administration.ts';
import {useAction, useRead} from '../../admin/console';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Button, Notice, Select, SettingsGroup, SettingsRow, Status, Switch, Text} from '../../ui';

function useServerId() { const {system, session} = useSession(); return system?.id ?? session?.viewer.serverId ?? ''; }
const time = (iso: string) => new Date(iso).toLocaleString(undefined, {weekday: 'short', hour: 'numeric', minute: '2-digit'});

/** Which tuner is doing what, now and soon, and where two recordings will collide. */
export function TunersGroup() {
  const {api} = useSession();
  const {t} = useI18n();
  const serverId = useServerId();
  const read = useRead(async () => parseEnvelope(await api.request<unknown>('/v1/admin/dvr/tuners'), serverId, parseTunerView).result, [api, serverId], {every: 60000});
  const view = read.data;
  return (
    <SettingsGroup title={t('web.dvrSettings.tuners')} description={view ? t('web.dvrSettings.tunersLede', {hours: view.upcomingWindowHours}) : undefined}>
      {read.error ? <div style={{padding: 12}}><Notice tone={view ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></div> : null}
      {view?.sources.map(src => (
        <SettingsRow key={src.sourceId} icon="live" label={src.sourceName} stack
          state={<Status tone={src.conflicts ? 'danger' : src.inUse >= src.tunerCount && src.tunerCount > 0 ? 'warning' : 'healthy'}>{src.conflicts ? `${src.conflicts} clash${src.conflicts === 1 ? '' : 'es'}` : `${src.inUse} of ${src.tunerCount} in use`}</Status>}
          control={src.active.length || src.upcoming.length ? <div style={{display: 'flex', flexDirection: 'column', gap: 4}}>{[...src.active, ...src.upcoming.slice(0, 6)].map(a => <Text key={a.allocationId || a.recordingId} variant="caption" tone="secondary">{a.state === 'recording' ? 'Recording now' : time(a.startsAt)} · {a.title}</Text>)}</div> : <Text variant="caption" tone="tertiary">{t('web.dvrSettings.nothingScheduled')}</Text>} />
      ))}
      {view && !view.sources.length ? <div style={{padding: 16}}><Text variant="caption" tone="tertiary">{t('web.dvrSettings.noTuners')}</Text></div> : null}
    </SettingsGroup>
  );
}

/** Explicit profile grants. Server administration alone grants no recording access. */
export function RecordingPermissionsGroup({owners, grants, onSaved}: {owners: readonly import('../../admin/recording-policy').RecordingOwnerChoice[]; grants: readonly import('../../admin/recording-policy').RecordingGrant[]; onSaved: () => void}) {
  const {api} = useSession();
  const {t} = useI18n();
  const action = useAction();
  const [selected, setSelected] = useState('');
  const [enabled, setEnabled] = useState(false);
  const [inherit, setInherit] = useState(false);
  const choices = [...owners];
  for (const grant of grants) if (!choices.some(c => recordingOwnerKey(c.owner) === recordingOwnerKey(grant.owner))) choices.push({owner: grant.owner, accountName: 'Unavailable account', profileName: grant.owner.profileId});
  const choice = choices.find(c => recordingOwnerKey(c.owner) === selected);
  const existing = grants.find(g => recordingOwnerKey(g.owner) === selected);
  const currentMember = owners.some(c => recordingOwnerKey(c.owner) === selected);
  useEffect(() => { setEnabled(existing?.enabled ?? false); setInherit(existing?.inheritProfiles ?? false); }, [selected, existing?.revision]);
  const changed = !!choice && (enabled !== (existing?.enabled ?? false) || inherit !== (existing?.inheritProfiles ?? false) || !existing);
  return <SettingsGroup title={t('web.dvrSettings.whoCanRecord')} description={t('web.dvrSettings.whoCanRecordLede')}>
    {action.error ? <div style={{padding: 12}}><Notice tone="error" compact>{action.error}</Notice></div> : null}
    {action.notice ? <div style={{padding: 12}}><Notice tone="success" compact>{action.notice}</Notice></div> : null}
    <SettingsRow label={t('web.shell.profile')} control={<Select hideLabel label={t('web.dvrSettings.recordingProfile')} value={selected} onChange={e => setSelected(e.target.value)} options={[{value: '', label: t('web.dvrSettings.chooseProfile')}, ...choices.map(c => ({value: recordingOwnerKey(c.owner), label: recordingOwnerLabel(c)}))]} />} />
    {choice ? <>
      <SettingsRow label={t('web.dvrSettings.allowRecording')} help={!currentMember ? t('web.dvrSettings.goneHelp') : undefined} control={<Switch label={t('web.dvrSettings.allowRecording')} checked={enabled} disabled={!currentMember && !enabled} onCheckedChange={setEnabled} />} />
      <SettingsRow label={t('web.dvrSettings.inheritRow')} help={t('web.dvrSettings.inheritHelp')} control={<Switch label={t('web.dvrSettings.inheritSwitch')} checked={inherit} disabled={!enabled || !currentMember} onCheckedChange={setInherit} />} />
      <div style={{padding: '4px 16px 16px'}}><Button size="sm" variant="primary" label={t('web.dvrSettings.savePermission')} loading={action.busy} disabled={!changed || enabled && !currentMember} onClick={() => void action.run(async () => { await api.request('/v1/admin/dvr/recording-permissions', 'PUT', {owner: choice.owner, enabled, inheritProfiles: inherit, revision: existing?.revision ?? 0}); onSaved(); }, 'Recording permission saved.')} /></div>
    </> : null}
  </SettingsGroup>;
}
