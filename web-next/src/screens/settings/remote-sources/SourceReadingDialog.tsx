import {useLayoutEffect, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {RemoteSource, RemoteSourceService} from '@core/remote-sources.ts';
import {router} from '../../../router';
import {Button, Switch} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {usePhone} from '../../../kit/overlays/shared';
import {useOwnedResource} from './resources';
import {OfferedReadingChoices} from './OfferedReadingChoices';
import {ReadingProfileOpening, type ReadingOpeningState} from './reading-profile-opening';
import {SourceChoiceSummary} from './SourceChoiceSummary';
import {SourceChoiceConnectionRows} from './SourceChoiceConnectionRows';
import {SourceAdvancedRows} from './SourceAdvancedRows';
import {sourceChoiceValue} from './source-choice-value';
import {Icon} from '../../../kit/Icon';
import styles from './remote-sources.module.css';

const t = defaultI18n.t;
const choiceLabels = {
  list: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.0.label',
  once: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.1.label',
  everything: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.2.label',
  ask: 'next.settingsSpec.server.remote-sources.panels.source.tabs.1.groups.0.rows.0.control.options.3.label',
} as const;
type Props = Readonly<{embedded?: boolean; service: RemoteSourceService; source: RemoteSource; isCurrent: () => boolean;
  reportError: (error: unknown) => void; onClose: () => void;
  errorText: (error: unknown) => string; isAuthFailure: (error: unknown) => boolean}>;

/** This is the existing source Reading dialog, not a second source/consent
 * surface. Each committed source/route opening owns its paired reads and draft. */
export function SourceReadingDialog({embedded = false, service, source, isCurrent, reportError, onClose, errorText, isAuthFailure}: Props) {
  const owned = useOwnedResource(() => {
    const location = router.state.location;
    const reader = new ReadingProfileOpening(service, source, () => isCurrent() && router.state.location === location &&
      service.getSnapshot().sources.some(row => row.id === source.id && row.generation === source.generation),
    () => !service.getSnapshot().busy && !service.getSnapshot().ambiguous);
    const unsubscribe = router.subscribe('onBeforeNavigate', () => reader.retire());
    return {reader, service, embedded, onClose, reportError, errorText, isAuthFailure, retire: () => {reader.retire();unsubscribe();}};
  }, entry => entry.retire(), [service, source, isCurrent, embedded], entry => {void entry.reader.load();});
  return owned ? <ReadingBody key={owned.id} {...owned.value}/> : null;
}

function ReadingBody({reader, service, embedded, onClose, reportError, errorText, isAuthFailure}: {
  reader: ReadingProfileOpening; service: RemoteSourceService; embedded: boolean; onClose: () => void; reportError: Props['reportError'];
  errorText: Props['errorText']; isAuthFailure: Props['isAuthFailure'];
}) {
  const state = useSyncExternalStore(reader.subscribe, reader.getSnapshot, reader.getSnapshot);
  const sourceState = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  const phone = usePhone();
  const close = () => {if (reader.current()) {reader.retire();onClose();}};
  useLayoutEffect(() => {if (state.kind === 'saved' && !embedded) close();}, [state, reader, embedded]);
  useLayoutEffect(() => {
    if (state.kind === 'failed' && reader.current() && isAuthFailure(state.error)) {
      reader.retire();reportError(state.error);
    }
  }, [state, reader, reportError, isAuthFailure]);
  if (!reader.current() || state.kind === 'retired') return null;
  if (state.kind === 'saved') return embedded ? <section className={styles.group}>
    <p role="status">{t('next.remoteSources.saved')}</p><Button size="compact" onClick={close}>{t('action.done')}</Button>
  </section> : null;
  const draft = 'draft' in state ? state.draft : undefined;
  const editable = state.kind === 'ready' || state.kind === 'failed';
  const disabled = !editable || !draft || sourceState.busy || sourceState.ambiguous;
  const error = state.kind === 'failed' ? errorText(state.error) : undefined;
  const footer = <>
      <Button size="compact" variant="glass" onClick={close}>{t(state.kind === 'accepted-unavailable' || state.kind === 'saved-unavailable'
        ? 'action.done' : state.kind === 'unknown' || state.kind === 'saving' ? 'action.close' : 'action.cancel')}</Button>
      {state.kind === 'unknown'
        ? <Button size="compact" variant="primary" onClick={() => {void reader.recover(state);}}>{t('next.remoteSources.checkResult')}</Button>
        : state.kind !== 'accepted-unavailable' && state.kind !== 'saved-unavailable' && <Button size="compact" variant="primary" disabled={disabled}
          onClick={() => {if (draft) void (draft.advancedDirty ? reader.saveAdvanced(draft, state) : reader.save(draft, state));}}>{t('next.remoteSources.save')}</Button>}
    </>;
  const body = <>
    {state.kind === 'loading' && <p className={styles.muted} role="status">{t('next.remoteSources.loading')}</p>}
    {error && <p className={styles.error} role="alert">{error}</p>}
    {state.kind === 'unknown' && <p className={styles.note} role="status">{t('next.remoteSources.unconfirmed')}</p>}
    {(state.kind === 'accepted-unavailable' || state.kind === 'saved-unavailable') && <p className={styles.note} role="status">{t('next.remoteSources.error.refreshFailed')}</p>}
    <SourceReadingFields reader={reader} state={state} disabled={disabled} phone={phone}/>

  </>;
  return embedded ? <section className={styles.group}><h3>{t('next.remoteSources.reading')}</h3>{body}<div className={styles.actions}>{footer}</div></section>
    : <Dialog open title={reader.source.name} description={t('next.remoteSources.reading')}
      onOpenChange={open => {if (!open) close();}} footer={footer}>{body}</Dialog>;
}

/** The same fields in the source Profile, existing Reading dialog and wizard.
 * Every command callback carries the owner's original draft/render state. */
export function SourceReadingFields({reader, state, disabled, phone, advancedOpen = true, onAdvancedChange}: {
  reader: ReadingProfileOpening; state: ReadingOpeningState; disabled: boolean; phone: boolean;
  advancedOpen?: boolean; onAdvancedChange?: () => void;
}) {
  const draft = 'draft' in state ? state.draft : undefined;
  if (!draft) return null;
  const choices = draft ? reader.offers(draft) : undefined;
  const recorded = draft && state.kind === 'ready' ? reader.recorded(draft) : undefined;
  const custom = recorded && recorded.comparison.changedFields.length > 0 ? recorded : undefined;
  const selected = draft.choice ?? (recorded?.comparison.changedFields.length === 0 ? recorded.definition : undefined);
  const folded = !!onAdvancedChange && advancedOpen && (!!custom || !!selected);
  const advancedChanges = recorded?.comparison.changedFields.filter(field => !['scanDepth', 'connections', 'requestsPerSecond', 'bytesPerSecond'].includes(field)).length;
  const applicable = draft?.profile.choiceApplicableFields;
  const markers = applicable ? {applicable, changed: recorded?.comparison.changedFields ?? [],
    value: (field: Parameters<typeof sourceChoiceValue>[0]) => recorded ? sourceChoiceValue(field, recorded.definition) : undefined,
    ...(draft && recorded && !draft.consentDirty ? {onReset: (field: Parameters<typeof sourceChoiceValue>[0]) => reader.resetField(draft, state, field)} : {})} : undefined;
  const advancedValues = draft ? reader.previewAdvanced(draft) : undefined;
  const readingDirty = !!draft && (draft.values.scanDepth !== draft.profile.values.scanDepth || draft.values.connections !== draft.profile.values.connections ||
    draft.values.requestsPerSecond !== draft.profile.values.requestsPerSecond || draft.values.bytesPerSecond !== draft.profile.values.bytesPerSecond);
  return <fieldset disabled={disabled} className={styles.readingFields}>
      {custom && <div className={styles.recordedChoice}>
        <SourceChoiceSummary comparison={custom.comparison} definition={custom.definition} phone={phone} disabled={disabled || draft.consentDirty}
          onChooseStart={folded ? onAdvancedChange : undefined}
          onResetAll={() => {if (reader.getSnapshot() === state) reader.select(draft, custom.definition);}}/>
      </div>}
      {custom && advancedOpen && <p className={styles.note}>{t('next.remoteSources.changedRowsHint')}</p>}
      {folded && selected && !custom && <div className={styles.readingSummary}><b>{t(choiceLabels[selected.name])}</b>
        <Button size="xs" variant="glass" disabled={disabled} onClick={onAdvancedChange}>{t('next.remoteSources.change')}</Button></div>}
      {!folded && choices && <OfferedReadingChoices definitions={choices} selected={selected} disabled={disabled || draft.consentDirty}
        onSelect={choice => reader.select(draft, choice)}/>}
      {draft.choice && <p className={styles.note}>{t('next.remoteSources.changeBody')}</p>}
      <div className={styles.limits}>
        <SourceChoiceConnectionRows values={draft.values} disabled={disabled || !!draft.choice || !!draft.advancedDirty}
          markers={markers} onChange={patch => reader.edit(draft, patch)}/>
      </div>
      {draft.consent !== undefined && <label className={styles.switchRow}>
        <span>{t('next.remoteSources.allowAdvancedMediaAnalysis')}<small className={styles.note}>{t('next.remoteSources.allowAdvancedMediaAnalysisWarning')}</small></span>
        <Switch aria-label={t('next.remoteSources.allowAdvancedMediaAnalysis')} checked={draft.consent}
          disabled={disabled || !!draft.choice || !!draft.advancedDirty} onCheckedChange={value => reader.changeConsent(draft, value)}/>
      </label>}
      {onAdvancedChange && <button type="button" className={styles.advRow} aria-expanded={advancedOpen} disabled={disabled} onClick={onAdvancedChange}>
        <span><b>{t('next.remoteSources.advanced')}</b><small>{advancedChanges !== undefined && advancedChanges > 0
          ? t('next.remoteSources.advancedChangeCount', {count: advancedChanges}) : t('next.remoteSources.advancedHelp')}</small></span><Icon name="i-down" size={18}/>
      </button>}
      {advancedOpen && draft.advanced && advancedValues && <section>
        <h3>{t('next.remoteSources.advanced')}</h3>
        <SourceAdvancedRows values={advancedValues} cacheSupported={draft.advanced.cacheSupported} transportSupported={!!reader.source.mountId}
          cacheApplied={draft.advanced.cacheApplied} cacheError={draft.advanced.cacheError} cacheUsedBytes={draft.advanced.cacheUsedBytes}
          observedInterval={draft.advanced.scanIntervalObserved} disabled={disabled || !!draft.choice || draft.consentDirty || readingDirty}
          markers={markers} onChange={values => reader.editAdvanced(draft, values)}/>
      </section>}
    </fieldset>;
}
