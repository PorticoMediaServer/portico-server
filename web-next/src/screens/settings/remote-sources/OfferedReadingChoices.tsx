import {defaultI18n, type MessageId} from '@i18n';
import {sourceChoiceNames, type SourceChoiceDefinition} from '@core/remote-source-choice.ts';
import styles from './remote-sources.module.css';

const t = defaultI18n.t;
const presetLabels = {
  list: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.0.label',
  once: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.1.label',
  everything: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.2.label',
  ask: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.3.label',
} as const satisfies Record<(typeof sourceChoiceNames)[number], MessageId>;
const presetHints = {
  list: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.0.hint',
  once: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.1.hint',
  everything: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.2.hint',
  ask: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.3.hint',
} as const satisfies Record<(typeof sourceChoiceNames)[number], MessageId>;

/** The four canonical choice definitions the service actually published, drawn as the
 * authored cards. Selection is the exact definition object this render drew; the
 * callback closes that object and never reconstructs a preset. Consent and the
 * retired whole-file flag are not drawn, and scan depth is never presented as a
 * claim that heavy analysis will run. */
export function OfferedReadingChoices({definitions, selected, disabled, onSelect}: Readonly<{
  definitions: readonly SourceChoiceDefinition[];
  selected?: SourceChoiceDefinition;
  disabled: boolean;
  onSelect: (definition: SourceChoiceDefinition) => void;
}>) {
  const offered = new Map(definitions.map(definition => [definition.name, definition] as const));
  return <div className={styles.choices} role="radiogroup" aria-label={t('next.remoteSources.readingMode')}>
    {sourceChoiceNames.map(name => {
      const definition = offered.get(name);
      if (!definition) return null;
      const checked = selected === definition;
      // Only the static authored descriptions that stay true for this definition:
      // the whole-file description is not used, because that work needs its own
      // recorded consent and is not a scan-depth fact.
      const analysis = definition.reading.scanDepth === 'file_list_only' ? t('next.remoteSources.factNone')
        : name === 'everything' ? undefined : t('next.remoteSources.factNewFilesOnce');
      return <button key={name} type="button" role="radio" aria-checked={checked} disabled={disabled}
        className={styles.choice} data-picked={checked || undefined}
        onClick={() => {if (!disabled) onSelect(definition);}}>
        <span className={styles.choiceRadio} aria-hidden="true"/>
        <span className={styles.choiceBody}>
          <span className={styles.choiceTitle}>{t(presetLabels[name])}
            {name === 'once' && <span className={styles.choiceTag}>{t('next.remoteSources.choiceRecommended')}</span>}
          </span>
          <span className={styles.choiceHint}>{t(presetHints[name])}</span>
          <span className={styles.choiceFacts}>
            {analysis !== undefined && <span><em>{t('next.remoteSources.factAnalysis')}</em>{analysis}</span>}
            <span><em>{t('next.remoteSources.factConnections')}</em>{definition.reading.connections} · {definition.reading.requestsPerSecond}{' '}
              {t('next.remoteSources.aSecond')}</span>
          </span>
        </span>
      </button>;
    })}
  </div>;
}
