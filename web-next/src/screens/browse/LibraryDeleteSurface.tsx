import {useEffect, useMemo, useRef, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../kit/controls';
import {Dialog} from '../../kit/overlays';
import {DeleteMediaDialog} from '../../kit/selection';
import type {EntryActionPorts, EntryActionTarget} from '../../shell/entry-actions';
import type {LibraryEditorSession} from './SelectionEditorSurface';
import {LibraryDeleteProducer} from './selection-delete';

export type LibraryDeleteOptions = EntryActionPorts & Readonly<{signal: AbortSignal}>;
const t = defaultI18n.t;
export function LibraryDeleteSurface({session, options, onClose}: {session: LibraryEditorSession; options: LibraryDeleteOptions; onClose(): void}) {
  const anchor = useRef<HTMLElement | null>(session.anchor);
  const producer = useMemo(() => new LibraryDeleteProducer(), [session]);
  const state = useSyncExternalStore(producer.subscribe, producer.getSnapshot, producer.getSnapshot);
  useEffect(() => {
    const target = session.operation.target;
    if (target.mode !== 'explicit' || !session.current()) return;
    const targets: Pick<EntryActionTarget, 'itemId' | 'libraryId' | 'kind'>[] = [];
    for (const pick of target.items) {
      if (!('libraryId' in pick.item) || !pick.item.capabilities.delete) return;
      targets.push({itemId: pick.item.id, libraryId: pick.item.libraryId, kind: pick.item.kind});
    }
    const signal = AbortSignal.any([session.lifetime.signal, options.signal]);
    const check = () => {if (signal.aborted || !session.current() || !options.isCurrent()) throw new DOMException('Selection retired', 'AbortError');};
    producer.open(targets, {...options, isCurrent: () => !signal.aborted && session.current() && options.isCurrent(), api: {
      request: async<T>(path: string, method?: string, body?: unknown, caller?: AbortSignal): Promise<T> => {
        check(); const value = await options.api.request<T>(path, method, body, caller ? AbortSignal.any([signal, caller]) : signal); check(); return value;
      },
    }});
    return () => producer.close();
  }, [producer, session, options]);
  const close = () => {session.lifetime.abort(); producer.close(); onClose();};
  const run = () => {void (state.phase === 'uncertain' ? producer.retry() : producer.confirm());};
  if (state.phase === 'uncertain' || state.phase === 'error') return <Dialog open onOpenChange={open => {if (!open) close();}}
    title={t('next.selection.delete')} returnFocusRef={anchor}
    footer={<Button onClick={() => {if (state.phase === 'uncertain') run(); else {close();}}}>{t(state.phase === 'uncertain' ? 'next.states.tryAgain' : 'next.selection.done')}</Button>}>
    <p role="alert">{t(state.phase === 'uncertain' ? 'next.selection.deleteUncertain' : 'next.selection.deleteFailed')}</p>
  </Dialog>;
  return <DeleteMediaDialog owner open onOpenChange={open => {if (!open && state.phase !== 'deleting' && state.phase !== 'confirming') close();}}
    returnFocusRef={anchor} selectionCount={session.operation.target.mode === 'explicit' ? session.operation.target.items.length : 0}
    preview={state.preview ?? null} confirmation={state.confirmation} onConfirmationChange={value => producer.setConfirmation(value)}
    removeFiles={state.removeFiles} onRemoveFilesChange={value => producer.setRemoveFiles(value)} onDelete={run}
    busy={state.phase === 'idle' || state.phase === 'loading' || state.phase === 'confirming' || state.phase === 'deleting'}
    error={state.errorCode ? t('next.selection.deleteNotReady') : undefined} result={state.result} ownerTag={<span>{t('next.selection.owner')}</span>}/>;
}
