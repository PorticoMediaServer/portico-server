import {useLayoutEffect, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {RemoteSource, RemoteSourceService} from '@core/remote-sources.ts';
import {router} from '../../../router';
import {Button, Switch} from '../../../kit/controls';
import {Dialog} from '../../../kit/overlays';
import {SettingNumber} from '../../../kit/settings';
import {useOwnedResource} from './resources';
import {OfferedReadingChoices} from './OfferedReadingChoices';
import {ReadingProfileOpening} from './reading-profile-opening';
import styles from './remote-sources.module.css';

const t = defaultI18n.t;
type Props = Readonly<{service: RemoteSourceService; source: RemoteSource; isCurrent: () => boolean;
  reportError: (error: unknown) => void; onClose: () => void;
  errorText: (error: unknown) => string; isAuthFailure: (error: unknown) => boolean}>;

/** This is the existing source Reading dialog, not a second source/consent
 * surface. Each committed source/route opening owns its paired reads and draft. */
export function SourceReadingDialog({service, source, isCurrent, reportError, onClose, errorText, isAuthFailure}: Props) {
  const owned = useOwnedResource(() => {
    const location = router.state.location;
    const reader = new ReadingProfileOpening(service, source, () => isCurrent() && router.state.location === location &&
      service.getSnapshot().sources.some(row => row.id === source.id && row.generation === source.generation),
    () => !service.getSnapshot().busy && !service.getSnapshot().ambiguous);
    const unsubscribe = router.subscribe('onBeforeNavigate', () => reader.retire());
    return {reader, service, onClose, reportError, errorText, isAuthFailure, retire: () => {reader.retire();unsubscribe();}};
  }, entry => entry.retire(), [service, source, isCurrent], entry => {void entry.reader.load();});
  return owned ? <ReadingBody key={owned.id} {...owned.value}/> : null;
}

function ReadingBody({reader, service, onClose, reportError, errorText, isAuthFailure}: {
  reader: ReadingProfileOpening; service: RemoteSourceService; onClose: () => void; reportError: Props['reportError'];
  errorText: Props['errorText']; isAuthFailure: Props['isAuthFailure'];
}) {
  const state = useSyncExternalStore(reader.subscribe, reader.getSnapshot, reader.getSnapshot);
  const sourceState = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  const close = () => {if (reader.current()) {reader.retire();onClose();}};
  useLayoutEffect(() => {if (state.kind === 'saved') close();}, [state, reader]);
  useLayoutEffect(() => {
    if (state.kind === 'failed' && reader.current() && isAuthFailure(state.error)) {
      reader.retire();reportError(state.error);
    }
  }, [state, reader, reportError, isAuthFailure]);
  if (!reader.current() || state.kind === 'retired' || state.kind === 'saved') return null;
  const draft = 'draft' in state ? state.draft : undefined;
  const editable = state.kind === 'ready' || state.kind === 'failed';
  const disabled = !editable || !draft || sourceState.busy || sourceState.ambiguous;
  const choices = draft ? reader.offers(draft) : undefined;
  const error = state.kind === 'failed' ? errorText(state.error) : undefined;
  return <Dialog open title={reader.source.name} description={t('next.remoteSources.reading')}
    onOpenChange={open => {if (!open) close();}}
    footer={<>
      <Button size="compact" variant="glass" onClick={close}>{t(state.kind === 'accepted-unavailable' || state.kind === 'saved-unavailable'
        ? 'action.done' : state.kind === 'unknown' || state.kind === 'saving' ? 'action.close' : 'action.cancel')}</Button>
      {state.kind === 'unknown'
        ? <Button size="compact" variant="primary" onClick={() => {void reader.recover(state);}}>{t('next.remoteSources.checkResult')}</Button>
        : state.kind !== 'accepted-unavailable' && state.kind !== 'saved-unavailable' && <Button size="compact" variant="primary" disabled={disabled}
          onClick={() => {if (draft) void reader.save(draft);}}>{t('next.remoteSources.save')}</Button>}
    </>}>
    {state.kind === 'loading' && <p className={styles.muted} role="status">{t('next.remoteSources.loading')}</p>}
    {error && <p className={styles.error} role="alert">{error}</p>}
    {state.kind === 'unknown' && <p className={styles.note} role="status">{t('next.remoteSources.unconfirmed')}</p>}
    {(state.kind === 'accepted-unavailable' || state.kind === 'saved-unavailable') && <p className={styles.note} role="status">{t('next.remoteSources.error.refreshFailed')}</p>}
    {draft && <fieldset disabled={disabled} className={styles.readingFields}>
      {choices && <OfferedReadingChoices definitions={choices} selected={draft.choice} disabled={disabled || draft.consentDirty}
        onSelect={choice => reader.select(draft, choice)}/>}
      {draft.choice && <p className={styles.note}>{t('next.remoteSources.changeBody')}</p>}
      <div className={styles.limits}>
        <SettingNumber label={t('next.remoteSources.connections')} value={draft.values.connections} min={1} max={32}
          unit={t('next.remoteSources.atOnce')} disabled={disabled || !!draft.choice}
          onCommit={value => {if (value !== null) reader.edit(draft, {connections: value});}}/>
        <SettingNumber label={t('next.remoteSources.requestsPerSecond')} value={draft.values.requestsPerSecond} min={1} max={50}
          unit={t('next.remoteSources.aSecond')} disabled={disabled || !!draft.choice}
          onCommit={value => {if (value !== null) reader.edit(draft, {requestsPerSecond: value});}}/>
        <SettingNumber label={t('next.remoteSources.downloadSpeed')} value={draft.values.bytesPerSecond ? draft.values.bytesPerSecond / 1_000_000 : null}
          min={1} max={1000} optional unit={t('next.remoteSources.megabytesPerSecond')} disabled={disabled || !!draft.choice}
          onCommit={value => reader.edit(draft, {bytesPerSecond: value === null ? 0 : Math.round(value) * 1_000_000})}/>
      </div>
      {draft.consent !== undefined && <label className={styles.switchRow}>
        <span>{t('next.remoteSources.allowAdvancedMediaAnalysis')}<small className={styles.note}>{t('next.remoteSources.allowAdvancedMediaAnalysisWarning')}</small></span>
        <Switch aria-label={t('next.remoteSources.allowAdvancedMediaAnalysis')} checked={draft.consent}
          disabled={disabled || !!draft.choice} onCheckedChange={value => reader.changeConsent(draft, value)}/>
      </label>}
    </fieldset>}
  </Dialog>;
}
