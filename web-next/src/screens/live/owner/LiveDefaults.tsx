import {useSyncExternalStore} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import type {liveDefaultsForm} from '@core/server-admin/server-forms.ts';
import {Button, Switch, TextField} from '../../../kit/controls';
import {SettingsRow} from '../../../kit/shell/SettingsSurfaces';
import styles from './owner.module.css';
const t = defaultI18n.t;
type DefaultsForm = ReturnType<typeof liveDefaultsForm>;

/** One explicit Save/Discard owns both defaults and import limits. Unit
 * conversion happens at the field, leaving the server document in ms/bytes. */
export function LiveDefaults({form, isCurrent, failure}: {form: DefaultsForm; isCurrent: () => boolean; failure: (error: unknown) => void}) {
  const state = useSyncExternalStore(form.subscribe, form.getSnapshot, form.getSnapshot), draft = state.draft;
  if (!draft || !isCurrent()) return null;
  const number = (label: MessageId, value: number, min: number, max: number, set: (value: number) => void, unit?: MessageId, help?: MessageId) =>
    <SettingsRow label={t(label)} help={help && t(help)}><TextField type="number" required min={min} max={max} step={1} value={value}
      variant="setting" aria-label={t(label)} disabled={state.saving} onChange={event => {if (isCurrent() && Number.isFinite(event.currentTarget.valueAsNumber)) set(event.currentTarget.valueAsNumber);}}/>{unit && <span className={styles.unit}>{t(unit)}</span>}</SettingsRow>;
  return <form onSubmit={event => {event.preventDefault();if (isCurrent() && event.currentTarget.reportValidity()) void form.save().then(() => {const error = form.getSnapshot().saveError;if (error && isCurrent()) failure(error);});}}>
    <section className={styles.section}><div className={styles.heading}><h3>{t('web.dvrSettings.liveDefaults')}</h3></div><p className={styles.lede}>{t('web.dvrSettings.liveDefaultsLede')}</p><div className={styles.sources}>
      {number('web.dvrSettings.buffer', draft.streamBufferMs / 1000, 0, 120, value => form.set({streamBufferMs: value * 1000}), 'settings.unit.seconds', 'web.dvrSettings.bufferHelp')}
      {number('web.dvrSettings.retryWindow', draft.retryWindowMs / 1000, 5, 600, value => form.set({retryWindowMs: value * 1000}), 'settings.unit.seconds', 'web.dvrSettings.retryWindowHelp')}
      {number('web.dvrSettings.guideDays', draft.guideDays, 1, 21, value => form.set({guideDays: value}), 'settings.unit.days', 'web.dvrSettings.guideDaysHelp')}
      <SettingsRow label={t('web.dvrSettings.userAgent')} help={t('web.dvrSettings.userAgentHelp')}><TextField value={draft.userAgent} maxLength={200} variant="setting" disabled={state.saving}
        aria-label={t('web.dvrSettings.userAgent')} onChange={event => {if (isCurrent()) form.set({userAgent: event.currentTarget.value});}}/></SettingsRow>
      <SettingsRow label={t('web.dvrSettings.logoImport')}><Switch checked={draft.logoImport} disabled={state.saving} aria-label={t('web.dvrSettings.logoImport')}
        onCheckedChange={logoImport => {if (isCurrent()) form.set({logoImport});}}/></SettingsRow>
      <SettingsRow label={t('web.dvrSettings.discovery')} help={t('web.dvrSettings.discoveryHelp')}><Switch checked={draft.discoveryEnabled} disabled={state.saving} aria-label={t('web.dvrSettings.discovery')}
        onCheckedChange={discoveryEnabled => {if (isCurrent()) form.set({discoveryEnabled});}}/></SettingsRow>
    </div></section>
    <section className={styles.section}><div className={styles.heading}><h3>{t('server.live.limits')}</h3></div><p className={styles.lede}>{t('server.live.limitsLede')}</p><div className={styles.sources}>
      {number('server.live.limitChannels', draft.importLimits.channels, 1, 100000, channels => form.set({importLimits: {...draft.importLimits, channels}}))}
      {number('server.live.limitProgrammes', draft.importLimits.programmes, 1, 10000000, programmes => form.set({importLimits: {...draft.importLimits, programmes}}))}
      {number('server.live.limitSize', draft.importLimits.bytes / 1048576, 1, 1024, bytes => form.set({importLimits: {...draft.importLimits, bytes: bytes * 1048576}}), 'settings.unit.mb')}
    </div></section>
    {state.dirty && <div className={styles.save}><Button size="small" variant="text" disabled={state.saving} onClick={() => {if (isCurrent()) form.discard();}}>{t('action.discard')}</Button><Button type="submit" size="small" variant="primary" loading={state.saving}>{t('action.save')}</Button></div>}
  </form>;
}
