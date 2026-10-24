import type {I18n} from '@i18n';

/** Everything the Series rules screen draws, in display order. */
export type SeriesRulesModel = Readonly<{
  rules: readonly SeriesRule[];
  storage: Readonly<{usedBytes: number; freeBytes: number; hoursLeft: number}>;
  recordings: Readonly<{upcomingCount: number; recordedCount: number; conflictCount: number}>;
  source: Readonly<{name: string; channelCount: number; guideThrough: string}>;
  /** The header glow is drawn from the visible channels' own tints: left, middle, right. */
  ambient: readonly [string, string, string];
}>;

export type SeriesRule = Readonly<{
  id: string;
  title: string;
  art: Readonly<{tint: string; glyph: string}>;
  record: RecordMode;
  channel: ChannelMode;
  /** The channel a "this channel" choice resolves to, and the channel named by the next airing. */
  channelName: string;
  keep: KeepCount;
  startEarlyMinutes: number;
  endLateMinutes: number;
  status: RuleStatus;
  next?: Readonly<{day: RuleDay; time: string; channel?: string; conflict?: boolean}>;
  /** A paused rule has no scheduled airings; this is how many new ones would match. */
  pausedMatch?: number;
  airings: readonly Airing[];
  recordedCount: number;
}>;

export type RuleStatus = 'on' | 'paused';
export type RecordMode = 'new' | 'all';
export type ChannelMode = 'this' | 'any';
export type KeepCount = 'all' | 1 | 3;
export type RuleDay = 'today' | 'tomorrow';
export type Airing = Readonly<{id: string; day: RuleDay; time: string; season?: number; episode?: number; title: string; channel: string; isNew?: boolean}>;

export type RuleDraft = Readonly<{
  on: boolean;
  record: RecordMode;
  channel: ChannelMode;
  keep: KeepCount;
  startEarlyMinutes: number;
  endLateMinutes: number;
}>;

export type PanelSnapshot = Readonly<{
  ruleId: string;
  title: string;
  art: Readonly<{tint: string; glyph: string}>;
  channelName: string;
  recordedCount: number;
  airings: readonly Airing[];
  draft: RuleDraft;
}>;

export type SeriesRulesState = Readonly<{
  rules: readonly SeriesRule[];
  /** The rule whose editor is open. A closed panel keeps its last snapshot for the exit frame. */
  selectedRuleId?: string;
  panel?: PanelSnapshot;
}>;

export type SeriesRulesAction =
  | Readonly<{type: 'open'; ruleId: string}>
  | Readonly<{type: 'close'}>
  | Readonly<{type: 'set-on'; value: boolean}>
  | Readonly<{type: 'set-record'; value: RecordMode}>
  | Readonly<{type: 'set-channel'; value: ChannelMode}>
  | Readonly<{type: 'set-keep'; value: KeepCount}>
  | Readonly<{type: 'set-start-early'; value: number}>
  | Readonly<{type: 'set-end-late'; value: number}>
  | Readonly<{type: 'save'}>
  | Readonly<{type: 'delete'}>;

export function draftFromRule(rule: SeriesRule): RuleDraft {
  return {on: rule.status === 'on', record: rule.record, channel: rule.channel, keep: rule.keep, startEarlyMinutes: rule.startEarlyMinutes, endLateMinutes: rule.endLateMinutes};
}

export function panelFromRule(rule: SeriesRule): PanelSnapshot {
  return {ruleId: rule.id, title: rule.title, art: rule.art, channelName: rule.channelName, recordedCount: rule.recordedCount, airings: rule.airings, draft: draftFromRule(rule)};
}

export function ruleWithDraft(rule: SeriesRule, draft: RuleDraft): SeriesRule {
  return {...rule, status: draft.on ? 'on' : 'paused', record: draft.record, channel: draft.channel, keep: draft.keep,
    startEarlyMinutes: draft.startEarlyMinutes, endLateMinutes: draft.endLateMinutes};
}

/** The panel opens only for a real rule; a stale id never invents one. */
export function initialSeriesRulesState(model: SeriesRulesModel, openRuleId?: string): SeriesRulesState {
  const rule = openRuleId === undefined ? undefined : model.rules.find(value => value.id === openRuleId);
  return rule ? {rules: model.rules, selectedRuleId: rule.id, panel: panelFromRule(rule)} : {rules: model.rules};
}

/** Record and Channel narrow what the rule would capture, so the list and its count follow the draft. */
export function visibleAirings(panel: PanelSnapshot): readonly Airing[] {
  return panel.airings.filter(airing =>
    (panel.draft.record === 'all' || airing.isNew === true) &&
    (panel.draft.channel === 'any' || airing.channel === panel.channelName));
}

export function seriesRulesReducer(state: SeriesRulesState, action: SeriesRulesAction): SeriesRulesState {
  switch (action.type) {
    case 'open': {
      const rule = state.rules.find(value => value.id === action.ruleId);
      if (!rule || rule.id === state.selectedRuleId) return state;
      return {...state, selectedRuleId: rule.id, panel: panelFromRule(rule)};
    }
    case 'close': return state.selectedRuleId === undefined ? state : {...state, selectedRuleId: undefined};
    case 'set-on': return editDraft(state, draft => ({...draft, on: action.value}));
    case 'set-record': return editDraft(state, draft => ({...draft, record: action.value}));
    case 'set-channel': return editDraft(state, draft => ({...draft, channel: action.value}));
    case 'set-keep': return editDraft(state, draft => ({...draft, keep: action.value}));
    case 'set-start-early': return editDraft(state, draft => ({...draft, startEarlyMinutes: action.value}));
    case 'set-end-late': return editDraft(state, draft => ({...draft, endLateMinutes: action.value}));
    case 'save': {
      const panel = state.panel;
      if (state.selectedRuleId === undefined || !panel) return state;
      const rules = state.rules.map(rule => rule.id === panel.ruleId ? ruleWithDraft(rule, panel.draft) : rule);
      return {rules, panel};
    }
    case 'delete': {
      const panel = state.panel;
      if (state.selectedRuleId === undefined || !panel) return state;
      return {rules: state.rules.filter(rule => rule.id !== panel.ruleId), panel};
    }
  }
}

function editDraft(state: SeriesRulesState, change: (draft: RuleDraft) => RuleDraft): SeriesRulesState {
  if (!state.panel) return state;
  return {...state, panel: {...state.panel, draft: change(state.panel.draft)}};
}

/** One decimal is enough for the 6px bar and matches the drawn 13.4%. */
export function storageFraction(usedBytes: number, freeBytes: number): number {
  const total = usedBytes + freeBytes;
  return total > 0 ? Math.round(usedBytes / total * 1000) / 10 : 0;
}

export type Translate = I18n['t'];

/** The card's three option facts, in the frame's order. */
export function ruleOptionLabels(rule: SeriesRule, t: Translate): readonly string[] {
  return [
    t(rule.record === 'new' ? 'guide.newOnly' : 'web.dvr.allEpisodes'),
    rule.channel === 'any' ? t('guide.anyChannel') : rule.channelName || t('guide.thisChannel'),
    rule.keep === 'all' ? t('web.dvr.keepAll') : t('web.dvr.keepLatest', {count: rule.keep}),
  ];
}

/** A paused rule says what would match; every other rule names its next airing. */
export function ruleNextLabel(rule: SeriesRule, t: Translate): string {
  if (rule.status === 'paused' && rule.pausedMatch !== undefined) return t('next.live.rules.pausedMatch', {count: rule.pausedMatch});
  if (!rule.next) return '';
  const when = t('next.live.rules.when', {day: t(rule.next.day === 'today' ? 'next.live.rules.today' : 'next.live.rules.tomorrow'), time: rule.next.time});
  if (rule.next.conflict) return t('next.live.rules.nextConflict', {when});
  if (rule.channel === 'any' && rule.next.channel) return t('next.live.rules.nextOn', {when, channel: rule.next.channel});
  return t('next.live.rules.next', {when});
}

export function ruleStatusLabel(rule: SeriesRule, t: Translate): string {
  return t(rule.status === 'on' ? 'web.dvr.on' : 'web.dvr.paused');
}

export function airDayLabel(day: RuleDay, t: Translate): string {
  return t(day === 'today' ? 'saved.today' : 'guide.tomorrow');
}

export function episodeLabel(airing: Airing, t: Translate): string | undefined {
  if (airing.season !== undefined && airing.episode !== undefined) return t('next.kit.episodeCode', {season: airing.season, episode: airing.episode});
  return airing.episode === undefined ? undefined : t('next.kit.episode', {number: airing.episode});
}
