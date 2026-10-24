import {useReducer, useRef, type ReactNode} from 'react';
import {defaultI18n} from '@i18n';
import {Button, Chip, Segmented, Select, Switch, Tabs} from '../../kit/controls';
import {SidePanel} from '../../kit/overlays';
import {Icon} from '../../kit/Icon';
import {RecordingsLayout, RecordingStorage} from '../../kit/dvr/RecordingsLayout';
import {RuleCard, RuleDecoration, RuleField, RuleGap} from '../../kit/dvr/RuleCard';
import {createRulesState, matchingAirings, rulesReducer, type SeriesRulesView, type RuleSettings} from './series-rules-model';
import styles from '../../kit/dvr/rules.module.css';
const t = defaultI18n.t;
const keepLabel = (value: RuleSettings['keep']) => value === 'all' ? t('recordings.keepAll') : value === '1' ? t('next.live.keepLatest') : t('next.live.keepLatestCount', {count: Number(value)});
const minutes = (value: number) => value ? t('next.live.minutes', {count: value}) : t('recordings.none');
/** Entirely local, typed presentation boundary. No server or platform services. */
export function SeriesRules({view, navigation}: {view: SeriesRulesView; navigation?: ReactNode}) {
  const [state, dispatch] = useReducer(rulesReducer, view, createRulesState);
  const opener = useRef<HTMLButtonElement | null>(null);
  const selected = state.rules.find(rule => rule.id === state.selectedId), draft = state.draft;
  const airings = selected && draft ? matchingAirings(selected, draft) : [];
  const patch = (patch: Partial<RuleSettings>) => dispatch({type: 'change', patch});
  const close = () => {opener.current?.focus(); dispatch({type: 'cancel'});};
  return <RecordingsLayout title={t('next.home.shell.live')} sourceLabel={t('channels.source')} allChannels={t('channels.all')}
    sourceName={view.sourceName} channels={t('player.info.channels', {count: view.channelCount})} through={t('next.live.guideThrough', {day: view.guideThrough})} navigation={navigation}>
    <Tabs className={styles.tabs} listClassName={styles.tabList} value="recordings" aria-label={t('next.home.shell.live')} items={[
      {value: 'guide', label: t('channels.viewGuide')},
      {value: 'recordings', label: <>{t('recordings.title')}<span className={styles.conflictNote}>{t('next.live.conflicts', {count: view.conflictCount})}</span></>},
    ]}/>
    <div className={styles.recordingBar}><div className={styles.chips} aria-label={t('recordings.title')}>
      {([{id: 'upcoming', label: t('recordings.upcoming'), count: view.upcomingCount}, {id: 'recorded', label: t('recordings.recorded'), count: view.recordedCount},
        {id: 'rules', label: t('recordings.rules'), count: state.rules.length}, {id: 'history', label: t('saved.tab.history')}, {id: 'storage', label: t('recordings.storage')}]).map(item => <Chip className={styles.chip} key={item.id} selected={item.id === 'rules'} count={item.count}>{item.label}</Chip>)}
    </div><RecordingStorage used={t('next.live.storageUsed', {amount: view.storage.used})} free={t('next.live.storageFree', {amount: view.storage.free})}
      remaining={t('next.live.storageLeft', {hours: view.storage.hoursLeft})} label={t('next.live.storageLabel', view.storage)} fraction={view.storage.used / (view.storage.used + view.storage.free)}/></div>
    <div className={styles.body}><div className={styles.rules}>{state.rules.map(rule => <RuleCard key={rule.id} title={rule.title} glyph={rule.glyph} tone={rule.tone}
      selected={selected?.id === rule.id} paused={!rule.settings.enabled} options={[t(rule.settings.episodes === 'all' ? 'guide.allEpisodes' : 'guide.newOnly'), rule.settings.channel === 'any' ? t('guide.anyChannel') : rule.channelName, keepLabel(rule.settings.keep)]}
      next={rule.settings.enabled ? rule.next : t('next.live.pausedMatch', {count: rule.airings.filter(airing => airing.isNew).length})} conflict={rule.conflict}
      state={t(rule.settings.enabled ? 'recordings.on' : 'recordings.paused')} upcoming={rule.settings.enabled ? t('next.live.upcomingCount', {count: matchingAirings(rule).length}) : undefined}
      actionsLabel={t('next.live.actionsFor', {title: rule.title})} buttonRef={selected?.id === rule.id ? opener : undefined} onOpen={() => dispatch({type: 'select', id: rule.id})}/> )}</div>
      <p className={styles.hint}>{t('next.live.addRuleHint')}</p></div>
    <SidePanel open={!!selected} onOpenChange={open => {if (!open) close();}} width={600} topInset={76} className={styles.panel} returnFocusRef={opener}
      accessibilityLabel={selected ? t('next.live.editRule', {title: selected.title}) : undefined}
      title={<><span className={styles.kind}><Icon name="i-series" size={18}/>{t('next.live.seriesRule')}</span><span className={styles.title}>{selected?.title}</span></>}
      description={<>{t('next.live.matchCount', {count: airings.length})}<RuleGap/>{t('next.live.recordedCount', {count: selected?.recorded ?? 0})}</>}
      footer={<><Button className={styles.footerButton} variant="primary" onClick={() => dispatch({type: 'save'})}>{t('action.save')}</Button><Button className={styles.footerButton} onClick={close}>{t('action.cancel')}</Button><Button className={`${styles.footerButton} ${styles.delete}`} variant="ghost" onClick={() => dispatch({type: 'delete'})}>{t('web.dvr.deleteRule')}</Button></>}>
      {selected && draft && <><RuleDecoration glyph={selected.glyph} tone={selected.tone}/>
        <div className={styles.toggleRow}><span><label htmlFor="series-rule-enabled">{t(draft.enabled ? 'next.live.ruleOn' : 'next.live.ruleOff')}</label><small id="series-rule-help">{t('next.live.pauseHelp')}</small></span><Switch id="series-rule-enabled" aria-describedby="series-rule-help" checked={draft.enabled} onCheckedChange={enabled => patch({enabled})}/></div>
        <div className={styles.form}><RuleField label={t('guide.record')}><Segmented className={styles.segmented} aria-label={t('guide.record')} value={draft.episodes} onValueChange={value => {if (value === 'all' || value === 'new') patch({episodes: value});}} items={[{value: 'new', label: t('web.dvr.newEpisodes')}, {value: 'all', label: t('guide.allEpisodes')}]}/></RuleField>
          <RuleField label={t('guide.channelColumn')}><Segmented className={styles.segmented} aria-label={t('guide.channelColumn')} value={draft.channel} onValueChange={value => {if (value === 'this' || value === 'any') patch({channel: value});}} items={[{value: 'this', label: t('guide.thisChannel')}, {value: 'any', label: t('guide.anyChannel')}]}/></RuleField>
          <RuleField label={t('recordings.keepLabel')}><Select className={styles.select} aria-label={t('recordings.keepLabel')} value={draft.keep} options={(['all', '1', '3'] as const).map(value => ({value, label: keepLabel(value)}))} onValueChange={value => {if (value === 'all' || value === '1' || value === '3') patch({keep: value});}}/></RuleField>
          {(['startEarly', 'endLate'] as const).map(field => <RuleField key={field} label={t(field === 'startEarly' ? 'recordings.startEarly' : 'recordings.endLate')}><Select className={styles.select} aria-label={t(field === 'startEarly' ? 'recordings.startEarly' : 'recordings.endLate')} value={String(draft[field])} options={[0, 2].map(value => ({value: String(value), label: minutes(value)}))} onValueChange={value => patch({[field]: Number(value)})}/></RuleField>)}
        </div><h3 className={styles.comingUp}>{t('next.live.comingUp')}</h3><div>{airings.map(airing => <div className={styles.airing} key={airing.id}><span className={styles.at}>{airing.time}</span><div><b>{airing.episode}<RuleGap/>{airing.title}</b><small>{airing.isNew && <span className={styles.newFlag}>{t('guide.new')}</span>}{airing.day}<RuleGap/>{airing.channel}</small></div><span className={styles.recordDot} role="img" aria-label={t('dvr.state.scheduled')}/></div>)}</div>
      </>}
    </SidePanel>
  </RecordingsLayout>;
}
