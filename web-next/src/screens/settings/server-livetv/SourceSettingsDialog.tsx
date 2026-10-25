import {useEffect, useRef, useSyncExternalStore} from 'react';
import {defaultI18n, type MessageId} from '@i18n';
import type {LiveSourceKind} from '@core/administration.ts';
import {guideOnlyPatch} from '@core/server-admin/panel-logic.ts';
import {errorMessage} from '@core/presentation/errors.ts';
import {Button, Segmented, Select, Switch, TextField} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {SettingsRow} from '../../../kit/shell/SettingsSurfaces';
import {dialogAction} from '../../live/owner/dialog-lifetime';
import type {SourceSettingsController} from './source-settings-controller';
import type {SourceChoicesController} from './source-choices-controller';
import {SourceChoicePicker} from './SourceChoicePicker';
import styles from './source-settings.module.css';
const t = defaultI18n.t;
const kinds: Record<LiveSourceKind, MessageId> = {playlist: 'server.live.sourceKind.playlist', 'xmltv-guide': 'server.live.sourceKind.guide', hdhomerun: 'server.live.sourceKind.tuner'};
const filters = {categories: 'server.live.filter.categories', countries: 'server.live.filter.countries', keywords: 'server.live.filter.keywords'} as const;
export function SourceSettingsDialog({controller, choices, name, guides, acquisition, close, changed}: {controller: SourceSettingsController; choices: {groups: SourceChoicesController; channels: SourceChoicesController}; name: string; guides: readonly {id: string; name: string}[]; acquisition?: 'upload' | 'remote'; close: () => void; changed: () => void}) {
  const owner = useRef(controller);owner.current = controller;
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot), {draft, document: doc} = state;
  useEffect(() => {void controller.load();return () => {controller.dispose();choices.groups.dispose();choices.channels.dispose();};}, [controller, choices]);
  const act = (callback: () => void) => dialogAction(owner, controller, callback);
  const busy = state.saving || state.loading || !!state.logoBusy, dirty = !!draft && !!doc && JSON.stringify(draft) !== JSON.stringify(doc.settings), guideOnly = draft?.kind === 'xmltv-guide';
  const closeCurrent = act(() => {controller.dispose();if (owner.current === controller) close();});
  const number = (label: MessageId, value: number | null, set: (value: number | null) => void, scale = 1, max = 10000000, placeholder?: string) => <TextField label={t(label)} type="number" min={0} max={max} step={1} disabled={busy} value={value === null ? '' : value / scale} placeholder={placeholder}
    onChange={event => {const value = event.currentTarget.value === '' ? null : event.currentTarget.valueAsNumber;if (value === null || Number.isSafeInteger(value) && value >= 0 && value <= max) {if (owner.current === controller && controller.isActive()) set(value === null ? null : value * scale);}}}/>;
  return <Dialog open title={name} description={t('web.liveSource.settingsLede')} width={620} onOpenChange={open => {if (!open) closeCurrent();}}
    footer={<><Button size="small" variant="text" onClick={closeCurrent}>{t('action.close')}</Button><Button size="small" variant="primary" loading={state.saving} disabled={!dirty || busy}
      onClick={act(() => void controller.save().then(saved => {if (saved && owner.current === controller && controller.isActive()) changed();}))}>{t('action.save')}</Button></>}>
    <div className={styles.fields}>
      {state.saved && <p role="status">{t(acquisition === 'upload' ? 'server.live.changesAfterUpload' : acquisition === 'remote' ? 'server.live.settingsSaved' : 'settings.saved')}</p>}
      {!!state.error && <p role="alert" className={styles.error}>{errorMessage(state.error)}</p>}
      {!draft && state.loading && <p role="status">{t('web.librarySettings.loading')}</p>}
      {draft && doc && <>
        <Select label={t('web.liveSource.provides')} value={draft.kind} disabled={busy} options={doc.availableKinds.map(value => ({value, label: t(kinds[value])}))}
          onValueChange={kind => controller.edit(guideOnlyPatch(kind) as Partial<typeof draft>)}/>
        {guideOnly ? <p>{t('web.liveSource.guideOnlyNote')}</p> : <>
          <Select label={t('web.liveSource.guideFrom')} description={t('web.liveSource.guideFromHelp')} value={draft.guideSourceId || 'own'} disabled={busy}
            options={[{value: 'own', label: t('web.liveSource.ownGuide')}, ...guides.filter(source => source.id !== controller.sourceId).map(source => ({value: source.id, label: source.name}))]}
            onValueChange={id => controller.edit({guideSourceId: id === 'own' ? '' : id})}/>
          <SourceChoicePicker controller={choices.groups} value={draft.enabledGroups} change={enabledGroups => controller.edit({enabledGroups})} disabled={busy}/>
          <SourceChoicePicker controller={choices.channels} value={draft.enabledChannels} change={enabledChannels => controller.edit({enabledChannels})} disabled={busy}/>
          <section><h3>{t('web.liveSource.keepChannels')}</h3>{(Object.keys(filters) as (keyof typeof filters)[]).map(key => <div key={key} className={styles.filter}>
            <TextField label={t(filters[key])} value={draft.filters[key].values.join(', ')} disabled={busy} onChange={event => controller.edit({filters: {...draft.filters, [key]: {...draft.filters[key], values: event.currentTarget.value.split(',').map(value => value.trim()).filter(Boolean)}}})}/>
            <Segmented aria-label={t('web.liveSource.filterMode', {filter: t(filters[key])})} value={draft.filters[key].mode} disabled={busy}
              items={[{value: 'include', label: t('web.liveSource.onlyThese')}, {value: 'exclude', label: t('web.liveSource.allButThese')}]}
              onValueChange={mode => controller.edit({filters: {...draft.filters, [key]: {...draft.filters[key], mode: mode as 'include' | 'exclude'}}})}/>
          </div>)}</section>
          <section><h3>{t('web.liveSource.channelNumbers')}</h3><Segmented aria-label={t('web.liveSource.numbering')} value={draft.channelNumbering.mode} disabled={busy}
            items={[{value: 'source', label: t('web.liveSource.asSent')}, {value: 'sequential', label: t('web.liveSource.inOrder')}]}
            onValueChange={mode => controller.edit({channelNumbering: {...draft.channelNumbering, mode: mode as 'source' | 'sequential'}})}/>
            {draft.channelNumbering.mode === 'sequential' && <div className={styles.pair}>
              {number('web.liveSource.startAt', draft.channelNumbering.startAt, value => {if (value && value >= 1) controller.edit({channelNumbering: {...draft.channelNumbering, startAt: value}});})}
              {number('web.liveSource.countBy', draft.channelNumbering.step, value => {if (value && value >= 1) controller.edit({channelNumbering: {...draft.channelNumbering, step: value}});})}
            </div>}</section>
          <section><h3>{t('next.liveOwner.sourcePictures')}</h3>
            <SettingsRow label={t('next.liveOwner.bringPictures')}><Switch checked={draft.logoImport.enabled} disabled={busy} aria-label={t('next.liveOwner.bringPictures')} onCheckedChange={enabled => controller.edit({logoImport: {...draft.logoImport, enabled}})}/></SettingsRow>
            <SettingsRow label={t('web.liveSource.replaceLogos')}><Switch checked={draft.logoImport.overwriteExisting} disabled={busy || !draft.logoImport.enabled} aria-label={t('web.liveSource.replaceLogos')} onCheckedChange={overwriteExisting => controller.edit({logoImport: {...draft.logoImport, overwriteExisting}})}/></SettingsRow>
            <Button size="small" disabled={busy || dirty || !doc.settings.logoImport.enabled} loading={state.logoBusy} onClick={act(() => void controller.importLogos())}>{t('web.liveSource.importLogos')}</Button>
            {state.logoResult && <p role="status">{t('web.liveSource.logoResult', state.logoResult)}</p>}
          </section>
          <div className={styles.pair}>
            {number('web.liveSource.streamBuffer', draft.streamBufferMs, streamBufferMs => controller.edit({streamBufferMs}), 1000, 120, t('web.liveSource.serverDefault', {value: doc.effective.streamBufferMs / 1000}))}
            {number('web.liveSource.retryFor', draft.retryWindowMs, retryWindowMs => controller.edit({retryWindowMs}), 1000, 600, t('web.liveSource.serverDefault', {value: doc.effective.retryWindowMs / 1000}))}
          </div>
        </>}
        <section><h3>{t('server.liveSource.limits')}</h3><p>{t('server.liveSource.limitsHelp')}</p><div className={styles.limits}>
          {(['channels', 'programmes', 'bytes'] as const).filter(field => !guideOnly || field !== 'channels').map(field => {
            const scale = field === 'bytes' ? 1048576 : 1, max = doc.effective.importLimits[field] / scale;
            return <div key={field}>{number(field === 'channels' ? 'server.live.limitChannels' : field === 'programmes' ? 'server.live.limitProgrammes' : 'server.live.limitSize', draft.importLimits?.[field] || null,
              value => {const limits = {...(draft.importLimits ?? {channels: 0, programmes: 0, bytes: 0}), [field]: value ?? 0};controller.edit({importLimits: Object.values(limits).some(Boolean) ? limits : null});}, scale, max,
              t('server.liveSource.limitDefault', {value: max}))}</div>;
          })}
        </div></section>
      </>}
    </div>
  </Dialog>;
}
