import {useEffect, useRef, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {DeleteMediaDialog} from '../kit/selection';
import {entryActions} from './entry-actions';
import {EntryDeleteProducer, type EntryDeleteSnapshot} from './entry-delete';

const t = defaultI18n.t;
function errorText(state: EntryDeleteSnapshot): string | undefined {
  if (!state.errorCode) return undefined;
  return t(state.errorCode === 'delete_not_ready' ? 'next.selection.deleteNotReady'
    : state.errorCode === 'delete_uncertain' ? 'next.selection.deleteUncertain'
      : 'next.selection.deleteFailed');
}

/** Mounted once by the shell. It renders nothing until a screen effect opens a surface. */
export function EntryActionHost() {
  const open = useSyncExternalStore(entryActions.subscribe, entryActions.getSnapshot, () => null);
  const producer = useRef<EntryDeleteProducer | null>(null);
  if (!producer.current) producer.current = new EntryDeleteProducer();
  const state = useSyncExternalStore(producer.current.subscribe, producer.current.getSnapshot, producer.current.getSnapshot);
  const anchor = useRef<HTMLElement | null>(null);
  useEffect(() => {
    const current = producer.current;
    if (!open) {current.close(); return;}
    anchor.current = open.anchor;
    current.open(open.target, open.ports);
    return () => current.close();
  }, [open]);
  if (!open) return null;
  const deleteNow = () => { if (!producer.current) return; void (state.phase === 'uncertain' ? producer.current.retry() : producer.current.confirm()); };
  return <DeleteMediaDialog owner open onOpenChange={value => { if (!value && state.phase !== 'deleting') entryActions.close(); }}
    returnFocusRef={anchor} selectionCount={1} preview={state.preview ?? null}
    confirmation={state.confirmation} onConfirmationChange={value => producer.current?.setConfirmation(value)}
    removeFiles={state.removeFiles} onRemoveFilesChange={value => producer.current?.setRemoveFiles(value)}
    onDelete={deleteNow} busy={state.phase === 'loading' || state.phase === 'deleting'}
    error={errorText(state)} result={state.result}
    ownerTag={<span>{t('next.selection.owner')}</span>} />;
}
