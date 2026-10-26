import {useEffect, useSyncExternalStore} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import type {recordingForm} from '@core/server-admin/server-forms.ts';
import {serverChoices} from '@core/presentation/settings-structure.ts';
import type {DVRDefaults} from '@core/administration.ts';
import {errorMessage} from '@core/presentation/errors.ts';
import {Button, Select, Switch, TextField} from '../../../../kit/controls';
import {SettingsRow} from '../../../../kit/shell/SettingsSurfaces';
import styles from '../../../live/owner/owner.module.css';
import layout from './recording-defaults.module.css';
const t = defaultI18n.t;
export function RecordingDefaults({form, current, reportError, embedded = false}: {embedded?: boolean; form: ReturnType<typeof recordingForm>; current: () => boolean; reportError: (error: unknown) => void}) {
  const state = useSyncExternalStore(form.subscribe, form.getSnapshot, form.getSnapshot), draft = state.draft;
  useEffect(() => {if (current()) void form.load();}, [form, current]);
  if (!current()) return null;
  const set = (patch: Partial<DVRDefaults>) => {if (current() && !state.saving) form.set(patch);};
  const number = (id: MessageId, value: number, min: number, max: number, change: (value: number) => void, help?: MessageId, unit?: MessageId) => <SettingsRow className={layout.numberRow} label={t(id)} help={help && t(help)}>
    <TextField variant="setting" type="number" aria-label={t(id)} required min={min} max={max} step={1} value={value} disabled={state.saving} onChange={event => {const value = event.currentTarget.valueAsNumber;if (current() && Number.isSafeInteger(value) && value >= min && value <= max) change(value);}}/>{unit && <span className={styles.unit}>{t(unit)}</span>}</SettingsRow>;
  return <form onSubmit={event => {event.preventDefault();if (!embedded && current() && event.currentTarget.reportValidity()) void form.save().catch(error => {if (current()) reportError(error);});}}>
    <section className={styles.section}><div className={styles.heading}><h3>{t('web.dvrSettings.defaults')}</h3></div><p className={styles.lede}>{t('web.dvrSettings.defaultsLede')}</p>
      {state.loading && !draft && <p role="status">{t('status.loading')}</p>}
      {!!state.loadError && <p role="alert" className={styles.error}>{errorMessage(state.loadError)}<Button size="small" onClick={() => {if (current()) void form.load();}}>{t('action.tryAgain')}</Button></p>}
      {!!state.saveError && <p role="alert" className={styles.error}>{errorMessage(state.saveError)}</p>}
      {draft && <div className={`${styles.sources} ${layout.rows}`}>
        {number('web.dvr.startEarly', draft.prePaddingMs / 60000, 0, 30, value => set({prePaddingMs: value * 60000}), 'web.dvrSettings.startEarlyHelp', 'settings.unit.minutes')}
        {number('web.dvrSettings.finishLate', draft.postPaddingMs / 60000, 0, 120, value => set({postPaddingMs: value * 60000}), 'web.dvrSettings.finishLateHelp', 'settings.unit.minutes')}
        <SettingsRow className={layout.selectRow} label={t('server.live.tunerConflict')} help={t('server.live.tunerConflictHelp')}><Select className={layout.select} containerClassName={layout.selectField} value={draft.liveTunerConflict} disabled={state.saving} aria-label={t('server.live.tunerConflict')}
          options={[{value: 'keep-watching', label: t('server.live.tunerConflict.keep')}, {value: 'stop-watching', label: t('server.live.tunerConflict.stop')}]}
          onValueChange={value => set({liveTunerConflict: value as DVRDefaults['liveTunerConflict']})}/></SettingsRow>
        <SettingsRow className={layout.selectRow} label={t('web.dvr.keepLabel')}><Select className={layout.select} containerClassName={layout.selectField} value={draft.keepPolicy.mode} disabled={state.saving} aria-label={t('web.dvr.keepLabel')}
          options={[{value: 'keep-all', label: t('web.logs.everything')}, {value: 'keep-count', label: t('web.dvrSettings.keepNewest')}, {value: 'keep-days', label: t('web.dvrSettings.keepDays')}]}
          onValueChange={mode => set({keepPolicy: {...draft.keepPolicy, mode: mode as DVRDefaults['keepPolicy']['mode']}})}/></SettingsRow>
        {draft.keepPolicy.mode === 'keep-count' && number('web.dvrSettings.howMany', draft.keepPolicy.keepCount, 1, 1000, keepCount => set({keepPolicy: {...draft.keepPolicy, keepCount}}))}
        {draft.keepPolicy.mode === 'keep-days' && number('web.dvrSettings.howManyDays', draft.keepPolicy.keepDays, 1, 3650, keepDays => set({keepPolicy: {...draft.keepPolicy, keepDays}}), undefined, 'settings.unit.days')}
        <SettingsRow className={layout.selectRow} label={t('web.dvrSettings.afterRecording')}><Select className={layout.select} containerClassName={layout.selectField} value={draft.conversion.mode} disabled={state.saving} aria-label={t('web.dvrSettings.afterRecording')}
          options={serverChoices('conversionModes', state.status, t, draft.conversion.mode)} onValueChange={mode => set({conversion: {...draft.conversion, mode: mode as DVRDefaults['conversion']['mode'], ...(mode === 'none' ? {deleteOriginal: false} : {})}})}/></SettingsRow>
        {draft.conversion.mode !== 'none' && <SettingsRow label={t('web.dvrSettings.removeOriginal')}><Switch checked={draft.conversion.deleteOriginal} disabled={state.saving} aria-label={t('web.dvrSettings.removeOriginal')}
          onCheckedChange={deleteOriginal => set({conversion: {...draft.conversion, deleteOriginal}})}/></SettingsRow>}
        <SettingsRow label={t('web.dvrSettings.folderShows')}><TextField variant="setting" value={draft.folderTemplate} maxLength={400} disabled={state.saving} aria-label={t('web.dvrSettings.folderShows')} onChange={event => set({folderTemplate: event.currentTarget.value})}/></SettingsRow>
        <SettingsRow label={t('web.dvrSettings.folderMovies')}><TextField variant="setting" value={draft.movieFolderTemplate} maxLength={400} disabled={state.saving} aria-label={t('web.dvrSettings.folderMovies')} onChange={event => set({movieFolderTemplate: event.currentTarget.value})}/></SettingsRow>
      </div>}
      {!!state.status?.folderTokens.length && <p className={styles.lede}>{state.status.folderTokens.map(token => `{${token.token}}`).join(', ')}</p>}
    </section>
    {!embedded && state.dirty && <div className={styles.save}><Button size="small" variant="text" disabled={state.saving} onClick={() => {if (current()) form.discard();}}>{t('action.discard')}</Button><Button type="submit" size="small" variant="primary" loading={state.saving}>{t('action.save')}</Button></div>}
  </form>;
}
