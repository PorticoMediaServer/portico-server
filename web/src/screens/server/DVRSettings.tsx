import {useEffect, useState} from 'react';
import {recordingOwnerKey, recordingOwnerLabel} from '../../admin/recording-policy';
import {parseEnvelope, parseTunerView} from '@core/administration.ts';
import {tunerStatus, tunerUseLine} from '@core/server-admin/live-words.ts';
import {QUIET_READ_EVERY_MS, problem, useRead} from '../../admin/console';
import {useInlineForm} from '../settings/ServerForms';
import {useSession} from '../../app/session';
import {useI18n} from '../../app/i18n';
import {Notice, Select, SettingsEmpty, SettingsGroup, SettingsLoading, SettingsNote, SettingsRow, Status, Switch, Text} from '../../ui';

function useServerId() { const {system, session} = useSession(); return system?.id ?? session?.viewer.serverId ?? ''; }

/** Which tuner is doing what, now and soon, and where two recordings will collide. */
export function TunersGroup() {
  const {api} = useSession();
  const i18n = useI18n();
  const {t} = i18n;
  const serverId = useServerId();
  const read = useRead(async () => parseEnvelope(await api.request<unknown>('/v1/admin/dvr/tuners'), serverId, parseTunerView).result, [api, serverId], {every: QUIET_READ_EVERY_MS});
  const view = read.data;
  return (
    <SettingsGroup title={t('web.dvrSettings.tuners')} description={view ? t('web.dvrSettings.tunersLede', {hours: view.upcomingWindowHours}) : undefined}>
      {read.error ? <SettingsNote><Notice tone={view ? 'warning' : 'error'} compact action={{label: t('action.tryAgain'), onClick: read.reload}}>{read.error}</Notice></SettingsNote> : null}
      {!view && read.loading ? <SettingsLoading thing={t('server.thing.tuners')} /> : null}
      {view?.sources.map(src => (
        <SettingsRow key={src.sourceId} icon="live" label={src.sourceName} stack
          state={<Status tone={tunerStatus(t, src).tone}>{tunerStatus(t, src).text}</Status>}
          control={src.active.length || src.upcoming.length ? <div style={{display: 'flex', flexDirection: 'column', gap: 4}}>{[...src.active, ...src.upcoming.slice(0, 6)].map((a, n) => <Text key={a.allocationId || a.recordingId || n} variant="caption" tone="secondary">{tunerUseLine(t, a, iso => i18n.formatDate(iso, {weekday: 'short', hour: 'numeric', minute: '2-digit'}))}</Text>)}</div> : <Text variant="caption" tone="tertiary">{t('web.dvrSettings.nothingScheduled')}</Text>} />
      ))}
      {view && !view.sources.length ? <SettingsEmpty title={t('web.dvrSettings.noTuners')} /> : null}
    </SettingsGroup>
  );
}

/**
 * Explicit profile grants. Server administration alone grants no recording access. Choosing a
 * profile shows its two switches; a change is part of the page's one Save, like every other
 * setting on the page.
 */
export function RecordingPermissionsGroup({owners, grants, onSaved}: {owners: readonly import('../../admin/recording-policy').RecordingOwnerChoice[]; grants: readonly import('../../admin/recording-policy').RecordingGrant[]; onSaved: () => void}) {
  const {api} = useSession();
  const {t} = useI18n();
  const [selected, setSelected] = useState('');
  const [enabled, setEnabled] = useState(false);
  const [inherit, setInherit] = useState(false);
  const choices = [...owners];
  for (const grant of grants) if (!choices.some(c => recordingOwnerKey(c.owner) === recordingOwnerKey(grant.owner))) choices.push({owner: grant.owner, accountName: t('server.live.permissionGone'), profileName: grant.owner.profileId});
  const choice = choices.find(c => recordingOwnerKey(c.owner) === selected);
  const existing = grants.find(g => recordingOwnerKey(g.owner) === selected);
  const currentMember = owners.some(c => recordingOwnerKey(c.owner) === selected);
  const reset = () => { setEnabled(existing?.enabled ?? false); setInherit(existing?.inheritProfiles ?? false); };
  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(reset, [selected, existing?.revision]);
  const changed = !!choice && (enabled !== (existing?.enabled ?? false) || inherit !== (existing?.inheritProfiles ?? false));
  const inline = useInlineForm('recording-permission', {
    dirty: changed && !(enabled && !currentMember),
    discard: reset,
    save: async () => { if (!choice) return; await api.request('/v1/admin/dvr/recording-permissions', 'PUT', {owner: choice.owner, enabled, inheritProfiles: inherit, revision: existing?.revision ?? 0}); onSaved(); },
  });
  return <SettingsGroup title={t('web.dvrSettings.whoCanRecord')} description={t('web.dvrSettings.whoCanRecordLede')}>
    {inline.error ? <SettingsNote><Notice tone="error" compact>{problem(inline.error, 'action')}</Notice></SettingsNote> : null}
    <SettingsRow label={t('web.shell.profile')} control={<Select hideLabel label={t('web.dvrSettings.recordingProfile')} value={selected} disabled={changed} onChange={e => setSelected(e.target.value)} options={[{value: '', label: t('web.dvrSettings.chooseProfile')}, ...choices.map(c => ({value: recordingOwnerKey(c.owner), label: recordingOwnerLabel(c)}))]} />} />
    {choice ? <>
      <SettingsRow label={t('web.dvrSettings.allowRecording')} help={!currentMember ? t('web.dvrSettings.goneHelp') : undefined} control={<Switch label={t('web.dvrSettings.allowRecording')} checked={enabled} disabled={!currentMember && !enabled} onCheckedChange={setEnabled} />} />
      <SettingsRow label={t('web.dvrSettings.inheritRow')} help={t('web.dvrSettings.inheritHelp')} control={<Switch label={t('web.dvrSettings.inheritSwitch')} checked={inherit} disabled={!enabled || !currentMember} onCheckedChange={setInherit} />} />
    </> : null}
  </SettingsGroup>;
}
