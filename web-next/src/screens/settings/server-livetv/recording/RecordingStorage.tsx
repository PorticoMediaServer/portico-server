import {useEffect, useSyncExternalStore} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import {errorMessage} from '@core/presentation/errors.ts';
import {Button, TextField} from '../../../../kit/controls';
import {SettingsRow} from '../../../../kit/shell/SettingsSurfaces';
import {RecordingStorage as StorageMeter} from '../../../../kit/dvr/RecordingsLayout';
import type {RecordingStorageController} from './storage-controller';
import styles from '../../../live/owner/owner.module.css';
const t = defaultI18n.t;
export function RecordingStorage({controller}: {controller: RecordingStorageController}) {
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot), {storage, draft} = state;
  useEffect(() => {void controller.load();}, [controller]);
  if (!controller.isActive()) return null;
  const dirty = !!storage && !!draft && JSON.stringify(storage.policy) !== JSON.stringify(draft);
  const number = (id: MessageId, help: MessageId, value: number, max: number, set: (value: number) => void, unit?: MessageId) => <SettingsRow label={t(id)} help={t(help)}>
    <TextField type="number" variant="setting" value={value} min={0} max={max} step={1} required disabled={state.saving} aria-label={t(id)} onChange={event => {const value = event.currentTarget.valueAsNumber;if (controller.isActive() && Number.isSafeInteger(value) && value >= 0 && value <= max) set(value);}}/>{unit && <span className={styles.unit}>{t(unit)}</span>}</SettingsRow>;
  return <form onSubmit={event => {event.preventDefault();if (controller.isActive() && event.currentTarget.reportValidity()) void controller.save();}}>
    <section className={styles.section}><div className={styles.heading}><h3>{t('channels.recordings')}</h3><div className={styles.actions}>
      {storage?.measurement.measuredAt && <span className={styles.fresh}>{t('freshness.measured', {when: defaultI18n.relativeTime(Date.parse(storage.measurement.measuredAt))})}</span>}
      <Button variant="icon" size="small" icon="i-refresh" iconOnly disabled={state.loading || state.saving || dirty} aria-label={t('action.refresh')} onClick={() => void controller.load()}/>
    </div></div><p className={styles.lede}>{t('web.liveTv.recordingsLede')}</p>
    {!!state.error && <p role="alert" className={styles.error}>{errorMessage(state.error)}</p>}
    {!storage && state.loading && <p role="status">{t('status.loading')}</p>}
    {storage && draft && <>
      <div className={styles.sources}><SettingsRow label={t('web.liveTv.space')}><dl>
        {([
          ['server.live.usedByRecordings', defaultI18n.bytes(storage.measurement.usedBytes)], ['server.live.freeOnVolume', defaultI18n.bytes(storage.measurement.freeBytes)],
          ['web.liveTv.space.reserved', defaultI18n.bytes(storage.measurement.reservedBytes)], ['web.liveTv.space.waitingDelete', defaultI18n.bytes(storage.pendingDeleteBytes)],
          ['web.liveTv.space.forecast', t('web.liveTv.space.forecastOver', {size: defaultI18n.bytes(storage.forecastBytes), hours: storage.forecastHours})],
        ] as const).map(([key, value]) => <div key={key}><dt>{t(key)}</dt><dd>{value}</dd></div>)}
      </dl></SettingsRow>
      <StorageMeter used={t('next.live.storageUsed', {amount: Math.round(storage.measurement.usedBytes / 1e9)})} free={t('next.live.storageFree', {amount: Math.round(storage.measurement.freeBytes / 1e9)})}
        remaining="" label={t('recordings.storage')} fraction={storage.measurement.usedBytes / Math.max(1, storage.policy.capBytes || storage.measurement.usedBytes + storage.measurement.freeBytes)}/>
      {number('web.liveTv.keepRecordings', 'web.liveTv.keepRecordingsHelp', draft.retentionDays, 3650, retentionDays => controller.edit({retentionDays}), 'settings.unit.days')}
      {number('web.liveTv.episodesPerSeries', 'web.liveTv.episodesPerSeriesHelp', draft.episodeLimit, 100000, episodeLimit => controller.edit({episodeLimit}))}
      {number('web.liveTv.leaveFree', 'web.liveTv.leaveFreeHelp', Math.round(draft.floorBytes / 1e9), 9007199, value => controller.edit({floorBytes: value * 1e9}), 'web.liveTv.gbUnit')}
      {number('web.liveTv.capUpTo', 'web.liveTv.capUpToHelp', Math.round(draft.capBytes / 1e9), 9007199, value => controller.edit({capBytes: value * 1e9}), 'web.liveTv.gbUnit')}
      </div>
      {!storage.measurement.writeHealthy && <p role="alert" className={styles.error}>{t('recordings.unavailableBody')}</p>}
    </>}
    </section>
    {dirty && <div className={styles.save}><Button variant="text" size="small" disabled={state.saving} onClick={() => controller.discard()}>{t('action.discard')}</Button><Button variant="primary" size="small" type="submit" loading={state.saving}>{t('action.save')}</Button></div>}
  </form>;
}
