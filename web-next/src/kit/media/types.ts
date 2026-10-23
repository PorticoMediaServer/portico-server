/** Presentation references from Music's canonical formatter. No server reason or notice text. */
export type SignalText = Readonly<{key: `player.signal.${string}` | 'action.done' | 'next.states.close' | 'player.queueNowPlaying'; values?: Readonly<Record<string, string | number>>}>;
export type SignalTranslate = (message: SignalText) => string;
export type SignalStep = Readonly<{
  id: 'file' | 'server' | 'leveling' | 'eq' | 'volume' | 'output';
  label: SignalText | Readonly<{deviceName: string}>;
  value: SignalText;
  note?: SignalText;
  changed: boolean;
}>;
/** Render-only model. Music owns rule priority and source/output interpretation in client-core. */
export type SignalPathModel = Readonly<{
  source?: SignalText;
  shortSource?: SignalText;
  state?: SignalText;
  /** DSD names itself within the state; this exact formatted source alone is emphasized. */
  stateEmphasis?: string;
  a11y: SignalText;
  pill: SignalText;
  pillHealthy: boolean;
  greenGlyph: boolean;
  steps: readonly SignalStep[];
}>;
export type MediaLayout = 'desktop' | 'phone' | 'tv';
