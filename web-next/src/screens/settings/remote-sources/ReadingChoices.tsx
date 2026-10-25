import {defaultI18n, type MessageId} from '@i18n';
import {sourceReadPreset, sourceReadPresetName, type SourceReadPreset, type SourceReadValues} from '@core/remote-sources.ts';
import {Icon} from '../../../kit/Icon';
import styles from './remote-sources.module.css';

const t = defaultI18n.t;
const presetKeys: readonly SourceReadPreset[] = ['list', 'once', 'everything', 'ask'];
const presetLabels = {
  list: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.0.label',
  once: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.1.label',
  everything: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.2.label',
  ask: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.3.label',
} as const;
const presetHints = {
  list: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.0.hint',
  once: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.1.hint',
  everything: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.2.hint',
  ask: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.3.hint',
} as const;

/** The drawn one-line description of a choice, for the mobile-web summary row. */
export const presetHint = (name: SourceReadPreset): MessageId => presetHints[name];

/** Whether the address is one the owner's own network answers: the drawn rule
 * for when reading whole files is cheap, with no bandwidth bill. */
export function sourceOnHomeNetwork(origin: string | undefined): boolean {
  if (!origin) return false;
  try {
    const host = new URL(origin).hostname.replace(/^\[|\]$/g, '').toLowerCase();
    if (!host || host === 'localhost' || !host.includes('.') || host.endsWith('.local') || host.endsWith('.lan') || host.endsWith('.home')) return true;
    if (/^127\./.test(host) || /^10\./.test(host) || /^192\.168\./.test(host) || /^169\.254\./.test(host)) return true;
    if (/^172\.(1[6-9]|2\d|3[01])\./.test(host)) return true;
    return /^(f[cd]|fe80)/.test(host);
  } catch { return false; }
}

/** The four choices as the frames draw them (remote-sources.html#4, #4d): plain
 * radio cards with the real facts of each choice. Whole-file choices say what
 * they cost on the storage that is actually connected, and a storage that cannot
 * skip ahead never offers one. Per-source cache figures are not drawn because
 * the server keeps no per-source cache setting; the row is left off, not invented. */
export function ReadingChoices({values, origin, rangeSupport, disabled, onChange}: Readonly<{
  values: SourceReadValues; origin?: string; rangeSupport: string; disabled: boolean;
  onChange: (values: SourceReadValues) => void;
}>) {
  const chosen = sourceReadPresetName(values);
  const home = sourceOnHomeNetwork(origin);
  const seekable = rangeSupport !== 'unsupported';
  return <div className={styles.choices} role="radiogroup" aria-label={t('next.remoteSources.readingMode')}>
    {presetKeys.map(name => {
      const preset = sourceReadPreset(name);
      const unavailable = name === 'everything' && !seekable;
      const picked = chosen === name;
      const whole = preset.wholeFileReads;
      return <button key={name} type="button" role="radio" aria-checked={picked} disabled={disabled || unavailable}
        className={styles.choice} data-picked={picked || undefined} data-unavailable={unavailable || undefined}
        onClick={() => { if (!unavailable) onChange(preset); }}>
        <span className={styles.choiceRadio} aria-hidden="true"/>
        <span className={styles.choiceBody}>
          <span className={styles.choiceTitle}>{t(presetLabels[name])}
            {name === 'once' && <span className={styles.choiceTag}>{t('next.remoteSources.choiceRecommended')}</span>}
            {unavailable && <span className={styles.choiceTag} data-tone="warning">{t('next.remoteSources.choiceNotAvailable')}</span>}
            {name === 'everything' && !unavailable && home && <span className={styles.choiceTag} data-tone="good">{t('next.remoteSources.choiceSuits')}</span>}
          </span>
          <span className={styles.choiceHint}>{t(presetHints[name])}</span>
          <span className={styles.choiceFacts}>
            <span><em>{t('next.remoteSources.factAnalysis')}</em>{whole ? t('next.remoteSources.factWholeFiles') : preset.scanDepth === 'file_list_only' ? t('next.remoteSources.factNone') : t('next.remoteSources.factNewFilesOnce')}</span>
            <span><em>{t('next.remoteSources.factConnections')}</em>{preset.connections} · {preset.requestsPerSecond}{' '}
              {t('next.remoteSources.aSecond')}</span>
          </span>
          {unavailable && <span className={styles.choiceCost} data-tone="warning"><Icon name="i-warn" size={18}/>
            {t('next.remoteSources.costNoRange')}</span>}
          {whole && !unavailable && <span className={styles.choiceCost} data-tone={home ? 'info' : 'warning'}>
            <Icon name={home ? 'i-info' : 'i-warn'} size={18}/>
            {t(home ? 'next.remoteSources.costHome' : 'next.remoteSources.costInternet')}</span>}
        </span>
      </button>;
    })}
  </div>;
}
