import {useEffect, useRef, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../kit/controls';
import {Dialog} from '../kit/overlays';
import {DeleteMediaDialog} from '../kit/selection';
import {entryActions} from './entry-actions';
import {deleteSurfaceFor, EntryDeleteProducer} from './entry-delete';

const t = defaultI18n.t;
function errorText(code: string | undefined): string | undefined {
  if (!code) return undefined;
  return t(code === 'delete_not_ready' ? 'next.selection.deleteNotReady'
    : code === 'delete_uncertain' ? 'next.selection.deleteUncertain'
      : 'next.selection.deleteFailed');
}

/** Mounted once by the shell. It renders nothing until a screen effect opens a surface. */
export function EntryActionHost() {
  const open = useSyncExternalStore(entryActions.subscribe, entryActions.getSnapshot, () => null);
  const producer = useRef(new EntryDeleteProducer());
  const state = useSyncExternalStore(producer.current.subscribe, producer.current.getSnapshot, producer.current.getSnapshot);
  const anchor = useRef<HTMLElement | null>(null);
  const stale = !!open && !open.ports.isCurrent();
  useEffect(() => {
    if (stale) { entryActions.close(); return; }
    const current = producer.current;
    if (!open) { current.close(); return; }
    anchor.current = open.anchor;
    current.open(open.target, open.ports);
    return () => current.close();
  }, [open, stale]);
  if (!open || stale) return null;
  const deleteNow = () => { void (state.phase === 'uncertain' ? producer.current.retry() : producer.current.confirm()); };
  const surface = deleteSurfaceFor(state.phase);
  if (surface === 'uncertain') {
    return <Dialog open onOpenChange={value => { if (!value) entryActions.close(); }} title={t('next.selection.delete')}
      returnFocusRef={anchor} footer={<Button variant="primary" onClick={deleteNow}>{t('next.states.tryAgain')}</Button>}>
      <p role="alert">{errorText(state.errorCode)}</p>
    </Dialog>;
  }
  return <DeleteMediaDialog owner open onOpenChange={value => { if (!value && state.phase !== 'deleting' && state.phase !== 'confirming') entryActions.close(); }}
    returnFocusRef={anchor} selectionCount={1} preview={state.preview ?? null}
    confirmation={state.confirmation} onConfirmationChange={value => producer.current.setConfirmation(value)}
    removeFiles={state.removeFiles} onRemoveFilesChange={value => producer.current.setRemoveFiles(value)}
    onDelete={deleteNow} busy={state.phase === 'loading' || state.phase === 'confirming' || state.phase === 'deleting'}
    error={errorText(state.errorCode)} result={state.result}
    ownerTag={<span>{t('next.selection.owner')}</span>} />;
}
