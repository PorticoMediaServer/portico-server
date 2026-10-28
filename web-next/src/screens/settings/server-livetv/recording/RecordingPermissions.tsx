import {useEffect, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {recordingOwnerKey, recordingAllowed, type RecordingOwnerChoice} from '@core/server-admin/recording-policy.ts';
import {errorMessage} from '@core/presentation/errors.ts';
import {Button, Switch} from '../../../../kit/controls';
import {SettingsRow} from '../../../../kit/shell/SettingsSurfaces';
import type {RecordingPermissionsController} from './permissions-controller';
import styles from '../../../live/owner/owner.module.css';
const t = defaultI18n.t;
/** Every fetched current profile is a real server candidate. Permission is
 * explicit or inherited; owner status never manufactures recording access. */
export function RecordingPermissions({controller, embedded = false}: {embedded?: boolean; controller: RecordingPermissionsController}) {
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  useEffect(() => {void controller.load();}, [controller]);
  if (!controller.isActive()) return null;
  const rows: RecordingOwnerChoice[] = [...state.owners];
  for (const grant of state.grants) if (!state.cursor && !rows.some(row => recordingOwnerKey(row.owner) === recordingOwnerKey(grant.owner))) rows.push({owner: grant.owner, accountName: t('server.live.permissionGone'), profileName: t('server.live.permissionGone')});
  const grants = state.grants.map(grant => ({...grant, ...(state.edits[recordingOwnerKey(grant.owner)] ?? {})}));
  for (const row of rows) {const key = recordingOwnerKey(row.owner);if (state.edits[key] && !grants.some(grant => recordingOwnerKey(grant.owner) === key)) grants.push({owner: row.owner, ...state.edits[key], revision: 0});}
  const dirty = Object.keys(state.edits).length > 0;
  const groups = new Map<string, {name: string; profiles: RecordingOwnerChoice[]}>();
  for (const row of rows) {const key = JSON.stringify([row.owner.authority, row.owner.accountId]), group = groups.get(key) ?? {name: row.accountName, profiles: []};group.profiles.push(row);groups.set(key, group);}
  return <section className={styles.section}><div className={styles.heading}><h3>{t('web.dvrSettings.whoCanRecord')}</h3>
    <Button variant="icon" icon="i-refresh" iconOnly size="small" aria-label={t('action.refresh')} disabled={state.loading || state.saving || dirty || state.uncertain} onClick={() => void controller.load()}/></div>
    <p className={styles.lede}>{t('web.dvrSettings.whoCanRecordLede')}</p>
    {!!state.error && <p role="alert" className={styles.error}>{errorMessage(state.error)}{dirty && <Button size="small" disabled={state.saving} onClick={() => void controller.save()}>{t('action.tryAgain')}</Button>}</p>}
    {state.loading && !rows.length && <p role="status">{t('status.loading')}</p>}
    {[...groups].map(([key, group]) => <section key={key} className={styles.sources}><h4 className={styles.accountHeading}>{group.name}</h4>{group.profiles.map(row => {
      const key = recordingOwnerKey(row.owner), grant = grants.find(grant => recordingOwnerKey(grant.owner) === key);
      const present = state.owners.some(candidate => recordingOwnerKey(candidate.owner) === key), allowed = recordingAllowed(row.owner, grants), inherited = !grant && allowed;
      const disabled = state.loading || state.saving || !!state.uncertain;
      return <div key={key}>
        <SettingsRow label={row.profileName} help={!present ? t('web.dvrSettings.goneHelp') : t(inherited ? 'next.liveOwner.recordInherited' : allowed ? 'next.liveOwner.canRecord' : 'next.liveOwner.cannotRecord')}>
          <Switch checked={allowed} disabled={disabled || !present && !allowed} aria-label={t('web.dvrSettings.allowRecording') + ': ' + row.profileName} onCheckedChange={enabled => controller.edit(row.owner, {enabled})}/>
        </SettingsRow>
        {grant?.enabled && present && <SettingsRow label={t('web.dvrSettings.inheritRow')} help={t('web.dvrSettings.inheritHelp')} dependent>
          <Switch checked={grant.inheritProfiles} disabled={disabled} aria-label={t('web.dvrSettings.inheritSwitch') + ': ' + row.profileName} onCheckedChange={inheritProfiles => controller.edit(row.owner, {inheritProfiles})}/>
        </SettingsRow>}
      </div>;
    })}</section>)}
    {state.cursor && <Button size="small" loading={state.loading} disabled={state.saving || dirty || state.uncertain} onClick={() => void controller.load(true)}>{t('action.showMore')}</Button>}
    {!embedded && dirty && <div className={styles.save}><Button variant="text" size="small" disabled={state.saving || state.uncertain} onClick={() => controller.discard()}>{t('action.discard')}</Button><Button variant="primary" size="small" loading={state.saving} onClick={() => void controller.save()}>{t('action.save')}</Button></div>}
  </section>;
}
