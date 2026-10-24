/** Local editor state; the caller supplies resolved display copy and real airing facts. */
export type RuleSettings = Readonly<{enabled: boolean; episodes: 'new' | 'all'; channel: 'this' | 'any'; keep: 'all' | '1' | '3'; startEarly: number; endLate: number}>;
export type RuleAiring = Readonly<{id: string; time: string; day: string; channelId: string; channel: string; episode: string; title: string; isNew: boolean}>;
export type SeriesRule = Readonly<{id: string; title: string; glyph: string; tone: 'documentary' | 'news' | 'sport'; channelId: string; channelName: string; recorded: number; next: string; conflict?: boolean; settings: RuleSettings; airings: readonly RuleAiring[]}>;
export type SeriesRulesView = Readonly<{sourceName: string; channelCount: number; guideThrough: string; upcomingCount: number; recordedCount: number; conflictCount: number; storage: Readonly<{used: number; free: number; hoursLeft: number}>; rules: readonly SeriesRule[]; selectedRuleId?: string}>;
export type RulesState = Readonly<{rules: readonly SeriesRule[]; selectedId?: string; draft?: RuleSettings}>;
export type RulesAction = Readonly<{type: 'select'; id: string}> | Readonly<{type: 'change'; patch: Partial<RuleSettings>}> | Readonly<{type: 'cancel' | 'save' | 'delete'}>;
export function matchingAirings(rule: SeriesRule, settings: RuleSettings = rule.settings): readonly RuleAiring[] {
  return settings.enabled ? rule.airings.filter(airing => (settings.episodes === 'all' || airing.isNew) && (settings.channel === 'any' || airing.channelId === rule.channelId)) : [];
}
export function createRulesState(view: SeriesRulesView): RulesState {
  const rule = view.rules.find(rule => rule.id === view.selectedRuleId);
  return {rules: view.rules, ...(rule ? {selectedId: rule.id, draft: {...rule.settings}} : {})};
}
export function rulesReducer(state: RulesState, action: RulesAction): RulesState {
  if (action.type === 'select') {
    const rule = state.rules.find(rule => rule.id === action.id);
    return rule ? {...state, selectedId: rule.id, draft: {...rule.settings}} : state;
  }
  if (action.type === 'change') return state.draft ? {...state, draft: {...state.draft, ...action.patch}} : state;
  if (action.type === 'cancel') return {rules: state.rules};
  if (action.type === 'delete') return {rules: state.rules.filter(rule => rule.id !== state.selectedId)};
  return {rules: state.rules.map(rule => rule.id === state.selectedId && state.draft ? {...rule, settings: {...state.draft}} : rule)};
}
