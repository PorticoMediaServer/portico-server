import {defaultI18n} from '@i18n';
import type {SourceAdvanced} from '@core/remote-sources.ts';
import {RichSelect, Switch} from '../../../kit/controls';
import styles from './remote-sources.module.css';

const t = defaultI18n.t;

/** The drawn Advanced groups (remote-sources.html#4b, #4c), over the values the
 * server actually applies: the folders' check interval, the start and watch
 * passes, the full re-check, the mount's real read cache, and the whole-file
 * analysis stages. Rows the server cannot honour are not drawn. */
export function SourceAdvancedRows({values, cacheSupported, cacheApplied, cacheError, observedInterval, disabled, onChange}: Readonly<{
  values: SourceAdvanced; cacheSupported: boolean; cacheApplied: boolean; cacheError?: string;
  observedInterval: number; disabled: boolean; onChange: (values: SourceAdvanced) => void;
}>) {
  const set = <K extends keyof SourceAdvanced,>(key: K, value: SourceAdvanced[K]) => onChange({...values, [key]: value});
  const intervals = [
    {value: '900', label: 'next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.0.control.options.0.label'},
    {value: '3600', label: 'next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.0.control.options.1.label'},
    {value: '21600', label: 'next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.0.control.options.2.label'},
    {value: '86400', label: 'next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.0.control.options.3.label'},
    {value: '604800', label: 'next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.0.control.options.4.label'},
    {value: '0', label: 'next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.0.control.options.5.label'},
  ] as const;
  const fullRechecks = [
    {value: '7', label: 'next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.3.control.options.0.label'},
    {value: '14', label: 'next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.3.control.options.1.label'},
    {value: '30', label: 'next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.3.control.options.2.label'},
    {value: '0', label: 'next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.3.control.options.3.label'},
  ] as const;
  const cacheSizes = ['0', '2', '4', '8', '16', '32', '64'] as const;
  const floors = ['1', '2', '4', '8', '16'] as const;
  const gb = (value: string) => Number(value) * 1024 * 1024 * 1024;
  const offered = intervals.some(option => option.value === String(values.scanIntervalSeconds));
  const observedUnknown = observedInterval > 0 && !intervals.some(option => option.value === String(observedInterval));
  const intervalValue = observedUnknown && !offered ? String(observedInterval) : String(values.scanIntervalSeconds);
  const intervalOptions = [
    ...(observedUnknown ? [{value: String(observedInterval), label: t('next.remoteSources.advObserved', {minutes: Math.round(observedInterval / 60)}), disabled: true}] : []),
    ...intervals.map(option => ({value: option.value, label: t(option.label), description: t('next.remoteSources.advIntervalHint')})),
  ];
  return <>
    <p className={styles.advTitle}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.title')}</p>
    <RichSelect disabled={disabled} aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.0.label')}
      value={intervalValue}
      options={intervalOptions}
      onValueChange={value => set('scanIntervalSeconds', Number(value))}/>
    {observedUnknown && <p className={styles.note}>{t('next.remoteSources.advObservedHelp')}</p>}
    <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.1.label')}<small className={styles.note}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.1.help')}</small></span>
      <Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.1.label')} checked={values.watchForChanges} disabled={disabled} onCheckedChange={value => set('watchForChanges', value)}/></label>
    <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.2.label')}<small className={styles.note}>{t('next.remoteSources.advStartHint')}</small></span>
      <Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.2.label')} checked={values.scanOnStart} disabled={disabled} onCheckedChange={value => set('scanOnStart', value)}/></label>
    <RichSelect disabled={disabled} aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.3.label')}
      value={String(values.fullRecheckDays)}
      options={fullRechecks.map(option => ({value: option.value, label: t(option.label), description: t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.0.rows.3.help')}))}
      onValueChange={value => set('fullRecheckDays', Number(value))}/>

    <p className={styles.advTitle}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.title')}</p>
    <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.2.control.items.0.label')}<small className={styles.note}>{t('next.remoteSources.advWholeFileHint')}</small></span>
      <Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.2.control.items.0.label')} checked={values.seekPreviews} disabled={disabled} onCheckedChange={value => set('seekPreviews', value)}/></label>
    <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.2.control.items.1.label')}</span>
      <Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.2.control.items.1.label')} checked={values.chapterImages} disabled={disabled} onCheckedChange={value => set('chapterImages', value)}/></label>
    <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.2.control.items.2.label')}</span>
      <Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.2.control.items.2.label')} checked={values.waveforms} disabled={disabled} onCheckedChange={value => set('waveforms', value)}/></label>
    <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.2.control.items.3.label')}</span>
      <Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.2.control.items.3.label')} checked={values.loudness} disabled={disabled} onCheckedChange={value => set('loudness', value)}/></label>
    <label className={styles.switchRow}><span>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.2.control.items.4.label')}</span>
      <Switch aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.1.rows.2.control.items.4.label')} checked={values.checksums} disabled={disabled} onCheckedChange={value => set('checksums', value)}/></label>

    {cacheSupported && <>
      <p className={styles.advTitle}>{t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.2.title')}</p>
      <RichSelect disabled={disabled} aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.1.label')}
        value={String(values.cacheBytes / (1024 * 1024 * 1024))}
        options={cacheSizes.map((size, index) => ({value: size, label: t(`next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.1.control.options.${index}.label` as never)}))}
        onValueChange={value => set('cacheBytes', gb(value))}/>
      <RichSelect disabled={disabled} aria-label={t('next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.2.rows.0.label')}
        value={String(values.cacheFloorBytes / (1024 * 1024 * 1024))}
        options={floors.map((size, index) => ({value: size, label: t(`next.settingsSpec.server.remote-sources.panels.source.tabs.3.groups.2.rows.0.control.options.${index}.label` as never)}))}
        onValueChange={value => set('cacheFloorBytes', gb(value))}/>
      {!cacheApplied && <p className={styles.note} role="status">{t('next.remoteSources.advCachePending')}</p>}
      {cacheError && <p className={styles.error} role="alert">{t('next.remoteSources.advCacheFailed', {reason: cacheError})}</p>}
    </>}
  </>;
}
