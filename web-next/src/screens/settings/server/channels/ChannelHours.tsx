import {useState} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryChannelBlock, LibraryChannelConfig} from '@core/library-channels.ts';
import {channelRequestID} from '@core/live-source-draft.ts';
import {Button, RichSelect, TextField} from '../../../../kit/controls';
import {Dialog} from '../../../../kit/overlays';
import type {BlockPresetsSource} from './block-presets-source';
import styles from './channels.module.css';
const t = defaultI18n.t;
const minuteText = (minute: number) => `${String(Math.floor(minute / 60)).padStart(2,'0')}:${String(minute % 60).padStart(2,'0')}`;
const minuteValue = (text: string) => /^([01]\d|2[0-3]):[0-5]\d$/.test(text) ? Number(text.slice(0,2))*60+Number(text.slice(3)) : undefined;
const dayLabel = (day: number) => new Intl.DateTimeFormat(undefined,{weekday:'short',timeZone:'UTC'}).format(Date.UTC(2023,0,1+day));
type Draft = {original?: LibraryChannelBlock; block: LibraryChannelBlock; from: string; to: string};
export function ChannelHours({config, locked, openPresets, presets, isCurrent, onChange, onEditing}: Readonly<{config: LibraryChannelConfig; presets: BlockPresetsSource; openPresets: () => void; locked: boolean; isCurrent: () => boolean; onChange: (change: (value: LibraryChannelConfig) => LibraryChannelConfig) => boolean; onEditing: (editing: boolean) => void}>) {
  const [draft, setDraft] = useState<Draft>(), [attempt] = useState(() => ({draft: undefined as Draft | undefined})), [error, setError] = useState(false);
  const current = () => isCurrent() && !locked;
  const open = (original?: LibraryChannelBlock) => {
    if (!current() || attempt.draft || original && !config.blocks.includes(original)) return;
    try {
      const block = original ? structuredClone(original) : {id:'block-'+channelRequestID().slice(0,24),weekdays:[],startMinute:0,endMinute:0,priority:0,ruleId:config.defaultRuleId,fallbackRuleId:config.defaultRuleId,anchor:'channel-cursor' as const,overrun:'finish' as const};
      const next={original,block,from:original ? minuteText(original.startMinute) : '',to:original ? minuteText(original.endMinute) : ''};
      attempt.draft=next;onEditing(true);setDraft(next);setError(false);
    } catch {setError(true);}
  };
  const update = (next: Draft) => {if (!current() || !draft || attempt.draft !== draft) return;attempt.draft=next;setDraft(next);};
  const close = () => {if (!isCurrent() || !draft || attempt.draft !== draft) return;attempt.draft=undefined;onEditing(false);setDraft(undefined);setError(false);};
  const apply = (remove = false) => {
    if (!current() || !draft || attempt.draft !== draft) return;
    const from=minuteValue(draft.from),to=minuteValue(draft.to);
    if (!remove && (from === undefined || to === undefined || !draft.block.weekdays.length || !config.rules.some(rule => rule.id === draft.block.ruleId))) {setError(true);return;}
    const block={...draft.block,startMinute:from ?? draft.block.startMinute,endMinute:to ?? draft.block.endMinute};
    if (onChange(value => ({...value,blocks:remove ? value.blocks.filter(row => row.id !== block.id) : draft.original ? value.blocks.map(row => row.id === block.id ? block : row) : [...value.blocks,block]}))) close();
  };
  const ruleLabel = (id: string) => id === config.defaultRuleId ? t('builder.block.default') : config.rules.find(rule => rule.id === id)?.name || t('error.unavailable');
  return <section className={styles.editorSection}><h3>{t('builder.schedule.hours')}</h3><p className={styles.help}>{t('builder.schedule.help')}</p>
    <div className={styles.choices}><Button size="small" variant={!config.blocks.length ? 'primary' : 'glass'} disabled={locked || !!draft} onClick={() => {if (current()) onChange(value => ({...value,blocks:[]}));}}>{t('builder.schedule.always')}</Button><Button size="small" variant={config.blocks.length ? 'primary' : 'glass'} disabled={locked || !!draft || config.blocks.length >= 64} onClick={() => open()}>{t('builder.schedule.scheduled')}</Button></div>
    <div className={styles.weekGrid}>{[1,2,3,4,5,6,0].map(day => <div key={day}><span>{dayLabel(day)}</span><div className={styles.dayTrack}>{config.blocks.filter(block => block.weekdays.includes(day)).map(block => <button type="button" key={block.id} disabled={locked} onClick={() => open(block)} title={`${ruleLabel(block.ruleId)} · ${minuteText(block.startMinute)} – ${minuteText(block.endMinute)}`}>{minuteText(block.startMinute)} – {minuteText(block.endMinute)}</button>)}</div></div>)}</div>
    {config.blocks.map(block => <Button key={block.id} size="small" variant="glass" disabled={locked || !!draft} onClick={() => open(block)}>{ruleLabel(block.ruleId)} · {block.weekdays.map(dayLabel).join(', ')} · {minuteText(block.startMinute)} – {minuteText(block.endMinute)}</Button>)}
    <p className={styles.help}>{t('builder.schedule.otherwise')}</p><Button size="small" variant="glass" disabled={locked || !!draft || config.blocks.length >= 64} onClick={() => open()}>{t('builder.schedule.add')}</Button>
    <Button size="small" variant="text" disabled={locked} onClick={() => {if (current() && !presets.getSnapshot().busy) openPresets();}}>{t('builder.schedule.presets')}</Button>
    <Dialog open={!!draft && isCurrent()} title={t('builder.block.edit')} onOpenChange={value => {if (!value) close();}} width={540} footer={<><Button size="compact" variant="glass" onClick={close}>{t('builder.cancel')}</Button><Button size="compact" variant="primary" onClick={() => apply()}>{t('action.done')}</Button></>}>
      {draft && <div className={styles.editorFields}><div className={styles.choices}>{[1,2,3,4,5,6,0].map(day => <label key={day}><input type="checkbox" checked={draft.block.weekdays.includes(day)} onChange={event => update({...draft,block:{...draft.block,weekdays:event.target.checked ? [...draft.block.weekdays,day] : draft.block.weekdays.filter(value => value !== day)}})}/>{dayLabel(day)}</label>)}</div>
        <div className={styles.choices}><TextField aria-label={t('builder.preset.starts')} type="time" value={draft.from} onChange={event => update({...draft,from:event.target.value})}/><span>{t('next.kit.filter.between')}</span><TextField aria-label={t('next.kit.filter.between')} type="time" value={draft.to} onChange={event => update({...draft,to:event.target.value})}/></div>
        <label className={styles.control}><span>{t('builder.block.plays')}</span><RichSelect aria-label={t('builder.block.plays')} value={draft.block.ruleId} options={config.rules.map(rule => ({value:rule.id,label:ruleLabel(rule.id)}))} onValueChange={ruleId => {if (config.rules.some(rule => rule.id === ruleId)) update({...draft,block:{...draft.block,ruleId}});}}/></label>
        <label className={styles.control}><span>{t('builder.block.overrun')}</span><RichSelect aria-label={t('builder.block.overrun')} value={draft.block.overrun} options={(['finish','cut','fit'] as const).map(value => ({value,label:t(`builder.block.${value}`)}))} onValueChange={value => {if (value === 'finish' || value === 'cut' || value === 'fit') update({...draft,block:{...draft.block,overrun:value}});}}/></label>
        <label className={styles.control}><span>{t('builder.block.anchor')}</span><RichSelect aria-label={t('builder.block.anchor')} value={draft.block.anchor} options={[{value:'channel-cursor',label:t('builder.block.resume')},{value:'block-start',label:t('builder.block.restart')}]} onValueChange={value => {if (value === 'channel-cursor' || value === 'block-start') update({...draft,block:{...draft.block,anchor:value}});}}/></label>
        <label className={styles.control}><span>{t('builder.block.fallback')}</span><RichSelect aria-label={t('builder.block.fallback')} value={draft.block.fallbackRuleId} options={[{value:config.defaultRuleId,label:t('builder.block.default')},{value:'',label:t('builder.whenDone.slate')},...config.rules.filter(rule => rule.id !== config.defaultRuleId).map(rule => ({value:rule.id,label:rule.name}))]} onValueChange={fallbackRuleId => {if (!fallbackRuleId || config.rules.some(rule => rule.id === fallbackRuleId)) update({...draft,block:{...draft.block,fallbackRuleId}});}}/></label>
        {draft.original && <Button size="small" variant="text" className={styles.danger} onClick={() => apply(true)}>{t('builder.block.remove')}</Button>}
        {error && <p role="alert" className={styles.warning}>{t('builder.error.rejected')}</p>}
      </div>}
    </Dialog>
    {error && !draft && <p role="alert" className={styles.warning}>{t('error.unavailable')}</p>}
  </section>;
}
