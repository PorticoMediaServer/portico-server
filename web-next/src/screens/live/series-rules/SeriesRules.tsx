import {useId, useReducer, useRef, type CSSProperties} from 'react';
import {defaultI18n} from '@i18n';
import {Icon} from '../../../kit/Icon';
import {Button, Chip, Segmented, Select, Switch, Tabs} from '../../../kit/controls';
import {SidePanel} from '../../../kit/overlays';
import {
  airDayLabel, episodeLabel, initialSeriesRulesState, ruleNextLabel, ruleOptionLabels, ruleStatusLabel, ruleWithDraft,
  seriesRulesReducer, storageFraction, visibleAirings,
  type Airing, type KeepCount, type PanelSnapshot, type SeriesRule, type SeriesRulesAction, type SeriesRulesModel,
} from './model';
import styles from './series-rules.module.css';

const t = defaultI18n.t;
export type RulesView = 'upcoming' | 'recorded' | 'rules' | 'history' | 'storage';
export type RulesTab = 'guide' | 'recordings';

export type SeriesRulesScreenProps = Readonly<{
  model: SeriesRulesModel;
  /** The rule the frame opens. Omit to start with the list and no editor. */
  initialOpenRuleId?: string;
  onSaveRule?: (rule: SeriesRule) => void;
  onDeleteRule?: (ruleId: string) => void;
  onTabChange?: (tab: RulesTab) => void;
  onViewChange?: (view: RulesView) => void;
  onOpenStorage?: () => void;
}>;

/** The frame's whole state, driven by a plain view model. Drafts and the open editor are local;
 *  a server-backed screen would lift the same reducer to its bindings and pass the callbacks. */
export function SeriesRulesScreen({model, initialOpenRuleId, onSaveRule, onDeleteRule, onTabChange, onViewChange, onOpenStorage}: SeriesRulesScreenProps) {
  const [state, dispatch] = useReducer(seriesRulesReducer, initialSeriesRulesState(model, initialOpenRuleId));
  const focusAnchor = useRef<HTMLSpanElement>(null);
  const panel = state.panel;
  const airings = panel ? visibleAirings(panel) : [];
  const used = defaultI18n.bytes(model.storage.usedBytes), free = defaultI18n.bytes(model.storage.freeBytes);
  const usedLabel = t('next.live.rules.used', {size: used}), freeLabel = t('next.live.rules.free', {size: free});
  const save = () => {
    if (panel) {
      const rule = state.rules.find(value => value.id === panel.ruleId);
      if (rule) onSaveRule?.(ruleWithDraft(rule, panel.draft));
    }
    dispatch({type: 'save'});
  };
  const remove = () => {
    if (panel) onDeleteRule?.(panel.ruleId);
    dispatch({type: 'delete'});
  };
  const views: readonly {id: RulesView; label: string; count?: number}[] = [
    {id: 'upcoming', label: t('web.dvr.upcoming'), count: model.recordings.upcomingCount},
    {id: 'recorded', label: t('web.dvr.recorded'), count: model.recordings.recordedCount},
    {id: 'rules', label: t('web.dvr.rules'), count: state.rules.length},
    {id: 'history', label: t('web.dvr.history')},
    {id: 'storage', label: t('web.dvr.storage')},
  ];
  return <section className={styles.screen} aria-label={t('next.home.shell.live')} style={{
    '--ambient-one': model.ambient[0], '--ambient-two': model.ambient[1], '--ambient-three': model.ambient[2],
  } as CSSProperties}>
    <div className={styles.ambient} aria-hidden="true" />
    <header className={styles.header}>
      <h1 className={styles.title}>{t('next.home.shell.live')}</h1>
      <Select className={styles.switcher} aria-label={t('channels.source')} value="all" options={[{value: 'all', label: t('channels.all')}]} />
      <p className={styles.source}><strong>{model.source.name}</strong><span className={styles.gap}/>{t('player.info.channels', {count: model.source.channelCount})}<span className={styles.gap}/>{t('next.live.guideThrough', {day: model.source.guideThrough})}</p>
    </header>
    <div className={styles.tabBar}>
      <Tabs value="recordings" onValueChange={value => onTabChange?.(value === 'guide' ? 'guide' : 'recordings')} aria-label={t('next.live.rules.tabs')} listClassName={styles.tabList}
        items={[
          {value: 'guide', label: t('channels.viewGuide')},
          {value: 'recordings', label: <>{t('recordings.title')}{model.recordings.conflictCount > 0 && <span className={styles.tabNote}>{t('next.live.rules.conflicts', {count: model.recordings.conflictCount})}</span>}</>},
        ]} />
    </div>
    <div className={styles.recBar}>
      <div className={styles.subTabs} role="group" aria-label={t('recordings.title')}>
        {views.map(view => <Chip key={view.id} className={styles.subChip} selected={view.id === 'rules'} count={view.count} onClick={() => onViewChange?.(view.id)}>{view.label}</Chip>)}
      </div>
      <button type="button" className={styles.store} aria-label={t('next.live.rules.storageLabel', {used: usedLabel, free: freeLabel})} onClick={onOpenStorage}>
        <span className={styles.storeTitle}><b>{usedLabel}</b><span className={styles.gap}/>{freeLabel}</span>
        <span className={styles.meter} aria-hidden="true"><i style={{width: `${storageFraction(model.storage.usedBytes, model.storage.freeBytes)}%`}}/></span>
        <span className={styles.storeSub}>{t('web.dvr.hoursLeft', {count: model.storage.hoursLeft})}</span>
      </button>
    </div>
    <div className={styles.recBody}>
      <ul className={styles.rules} aria-label={t('web.dvr.rules')}>
        {state.rules.map(rule => <li key={rule.id} className={styles.rule} data-selected={rule.id === state.selectedRuleId || undefined} data-paused={rule.status === 'paused' || undefined}>
          <button type="button" className={styles.ruleButton} aria-pressed={rule.id === state.selectedRuleId} onClick={() => dispatch({type: 'open', ruleId: rule.id})}>
            <span className={styles.art}><span className={styles.artTint} style={{'--tint': rule.art.tint} as CSSProperties} aria-hidden="true"><span className={styles.artGlyph}>{rule.art.glyph}</span></span></span>
            <span className={styles.copy}>
              <b className={styles.ruleTitle}>{rule.title}</b>
              <small className={styles.opts}>{ruleOptionLabels(rule, t).map(label => <span key={label}>{label}</span>)}</small>
              <span className={styles.next} data-warn={rule.next?.conflict || undefined}>{ruleNextLabel(rule, t)}</span>
            </span>
            <span className={styles.state}>
              <span className={styles.pill} data-status={rule.status}>{ruleStatusLabel(rule, t)}</span>
              {rule.status === 'on' && <small>{t('next.live.rules.upcoming', {count: rule.airings.length})}</small>}
            </span>
          </button>
          <Button className={styles.ruleActions} variant="icon" iconOnly icon="i-more" aria-label={t('web.dvr.actionsFor', {title: rule.title})} onClick={() => dispatch({type: 'open', ruleId: rule.id})} />
        </li>)}
      </ul>
      <p className={styles.hint}>{t('next.live.rules.hint')}</p>
    </div>
    <SidePanel open={state.selectedRuleId !== undefined && panel !== undefined} onOpenChange={open => {if (!open) dispatch({type: 'close'});}}
      width={600} topInset={76} className={styles.sheet} initialFocusRef={focusAnchor} accessibilityLabel={panel ? t('next.live.rules.panelLabel', {title: panel.title}) : undefined}
      title={panel ? <>
        <span ref={focusAnchor} className={styles.focusAnchor} tabIndex={-1} role="group" aria-label={t('next.live.rules.panelLabel', {title: panel.title})}/>
        <span className={styles.shKind}><Icon name="i-series" size={18}/>{t('next.live.rules.kind')}</span>
        <span className={styles.shName}>{panel.title}</span>
        <span className={styles.shGlow} aria-hidden="true" style={{'--tint': panel.art.tint} as CSSProperties}/>
        <span className={styles.shGlyph} aria-hidden="true">{panel.art.glyph}</span>
      </> : undefined}
      description={panel ? <><span>{t('web.dvr.matches', {count: airings.length})}</span><span>{t('next.live.rules.recorded', {count: panel.recordedCount})}</span></> : undefined}
      footer={panel ? <>
        <Button className={styles.footerButton} variant="primary" onClick={save}>{t('action.save')}</Button>
        <Button className={styles.footerButton} variant="glass" onClick={() => dispatch({type: 'close'})}>{t('action.cancel')}</Button>
        <Button className={`${styles.footerButton} ${styles.danger}`} variant="ghost" onClick={remove}>{t('web.dvr.deleteRule')}</Button>
      </> : undefined}>
      {panel && <RuleEditor panel={panel} airings={airings} dispatch={dispatch} />}
    </SidePanel>
  </section>;
}

function RuleEditor({panel, airings, dispatch}: {panel: PanelSnapshot; airings: readonly Airing[]; dispatch: (action: SeriesRulesAction) => void}) {
  const ids = {toggle: useId(), record: useId(), channel: useId(), keep: useId(), start: useId(), end: useId()};
  const draft = panel.draft;
  const minutes = [0, 1, 2, 5, 10].map(value => ({value: String(value), label: value === 0 ? t('web.dvr.none') : t('web.dvr.minutes', {count: value})}));
  return <>
    <div className={styles.toggle}>
      <span id={ids.toggle} className={styles.toggleLabel}>{t(draft.on ? 'next.live.rules.on' : 'next.live.rules.off')}<small className={styles.help}>{t('next.live.rules.pauseHelp')}</small></span>
      <Switch checked={draft.on} aria-labelledby={ids.toggle} onCheckedChange={value => dispatch({type: 'set-on', value})} />
    </div>
    <div className={styles.form}>
      <div className={styles.row}><span id={ids.record}>{t('web.dvr.record')}</span>
        <Segmented className={styles.seg} aria-labelledby={ids.record} value={draft.record} onValueChange={value => {
          if (value === 'new' || value === 'all') dispatch({type: 'set-record', value});
        }} items={[{value: 'new', label: t('web.dvr.newEpisodes')}, {value: 'all', label: t('web.dvr.allEpisodes')}]} />
      </div>
      <div className={styles.row}><span id={ids.channel}>{t('guide.channelColumn')}</span>
        <Segmented className={styles.seg} aria-labelledby={ids.channel} value={draft.channel} onValueChange={value => {
          if (value === 'this' || value === 'any') dispatch({type: 'set-channel', value});
        }} items={[{value: 'this', label: t('guide.thisChannel')}, {value: 'any', label: t('guide.anyChannel')}]} />
      </div>
      <div className={styles.row}><span id={ids.keep}>{t('web.dvr.keepLabel')}</span>
        <Select className={styles.select} aria-labelledby={ids.keep} value={keepValue(draft.keep)} onValueChange={value => dispatch({type: 'set-keep', value: parseKeep(value)})}
          options={[{value: 'all', label: t('web.dvr.keepAll')}, {value: '1', label: t('web.dvr.keepLatest', {count: 1})}, {value: '3', label: t('web.dvr.keepLatest', {count: 3})}]} />
      </div>
      <div className={styles.row}><span id={ids.start}>{t('web.dvr.startEarly')}</span>
        <Select className={styles.select} aria-labelledby={ids.start} value={String(draft.startEarlyMinutes)} onValueChange={value => dispatch({type: 'set-start-early', value: Number(value)})} options={minutes} />
      </div>
      <div className={styles.row}><span id={ids.end}>{t('web.dvr.endLate')}</span>
        <Select className={styles.select} aria-labelledby={ids.end} value={String(draft.endLateMinutes)} onValueChange={value => dispatch({type: 'set-end-late', value: Number(value)})} options={minutes} />
      </div>
    </div>
    <h3 className={styles.comingUp}>{t('next.live.rules.comingUp')}</h3>
    {airings.length > 0 ? airings.map(airing => {
      const episode = episodeLabel(airing, t);
      return <div key={airing.id} className={styles.airRow}>
        <span className={styles.airTime}>{airing.time}</span>
        <div className={styles.airBody}>
          <b className={styles.airTitle}>{episode && <>{episode}<span className={styles.gap}/></>}{airing.title}</b>
          <small className={styles.airMeta}>{airing.isNew && <span className={styles.newBadge}>{t('guide.new')}</span>}{airDayLabel(airing.day, t)}<span className={styles.gap}/>{airing.channel}</small>
        </div>
        <span className={styles.recDot} role="img" aria-label={t('dvr.state.scheduled')} />
      </div>;
    }) : <p className={styles.noAirings}>{t('web.dvr.matches', {count: 0})}</p>}
  </>;
}

function keepValue(keep: KeepCount): string {
  return keep === 'all' ? 'all' : String(keep);
}

function parseKeep(value: string): KeepCount {
  return value === '3' ? 3 : value === '1' ? 1 : 'all';
}
