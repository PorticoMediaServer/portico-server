import {useEffect, useRef, useState, type RefObject} from 'react';
import {defaultI18n} from '@i18n';
import {Button, TextField} from '../controls';
import {Dialog} from '../overlays/Dialog';
import styles from './settings.module.css';

const t = defaultI18n.t;
export type BackupPassphraseDialogProps = Readonly<{
  open: boolean; mode: 'protect' | 'change' | 'restore'; onClose: () => void;
  /** The real facade adapter maps only its authenticated wrong-passphrase outcome. */
  submit: (passphrase: string, signal: AbortSignal) => Promise<'done' | 'wrong-passphrase'>;
  onDone: () => void;
  /** No raw error or secret leaves this password surface. The parent shows its approved failure state. */
  onFailure: () => void;
  returnFocusRef?: RefObject<HTMLElement | null>;
}>;

/** Ruling §17's controlled password step; no persistence, logging or optimistic switch mutation. */
export function BackupPassphraseDialog({open, mode, onClose, submit, onDone, onFailure, returnFocusRef}: BackupPassphraseDialogProps) {
  const [first, setFirst] = useState(''), [second, setSecond] = useState(''), [error, setError] = useState<'mismatch' | 'incorrect'>(), [busy, setBusy] = useState(false);
  const input = useRef<HTMLInputElement>(null), request = useRef<AbortController | undefined>(undefined);
  const erase = () => {setFirst('');setSecond('');setError(undefined);};
  const close = () => {request.current?.abort();request.current = undefined;erase();setBusy(false);onClose();};
  useEffect(() => {
    if (!open) {request.current?.abort();request.current = undefined;erase();setBusy(false);}
    return () => {request.current?.abort();request.current = undefined;};
  }, [open, mode]);
  const bytes = new TextEncoder().encode(first).byteLength;
  const ready = bytes > 0 && bytes <= 1024 && (mode === 'restore' || bytes >= 12 && second.length > 0);
  const save = async () => {
    if (!open || !ready || request.current) return;
    if (mode !== 'restore' && first !== second) {setError('mismatch');return;}
    const controller = request.current = new AbortController(), passphrase = first;
    erase();setBusy(true);
    try {
      const result = await submit(passphrase, controller.signal);
      if (controller.signal.aborted || request.current !== controller) return;
      if (result === 'wrong-passphrase' && mode === 'restore') setError('incorrect');
      else if (result === 'done') {onDone();onClose();}
      else {onClose();onFailure();}
    } catch {
      if (!controller.signal.aborted && request.current === controller) {onClose();onFailure();}
    } finally {
      if (request.current === controller) {request.current = undefined;setBusy(false);}
    }
  };
  return <Dialog open={open} onOpenChange={value => {if (!value) close();}} initialFocusRef={input} returnFocusRef={returnFocusRef}
    title={t(mode === 'protect' ? 'next.settings.backup.protectTitle' : mode === 'change' ? 'next.settings.backup.changeTitle' : 'next.settings.backup.restoreTitle')}
    footer={<><Button size="small" onClick={close}>{t('next.settings.cancel')}</Button><Button size="small" variant="primary" loading={busy} disabled={!ready} onClick={() => {void save();}}>{t(mode === 'protect' ? 'next.settings.backup.protect' : mode === 'change' ? 'action.save' : 'next.settings.backup.restore')}</Button></>}>
    <form className={styles.passphraseFields} onSubmit={event => {event.preventDefault();void save();}}>
      <TextField ref={input} label={t('next.settings.backup.passphrase')} type="password" autoComplete="off" value={first} disabled={busy}
        onChange={event => {setFirst(event.currentTarget.value);setError(undefined);}} error={error === 'incorrect' ? t('next.settings.backup.incorrect') : undefined} />
      {mode !== 'restore' && <TextField label={t('next.settings.backup.repeat')} type="password" autoComplete="off" value={second} disabled={busy}
        onChange={event => {setSecond(event.currentTarget.value);setError(undefined);}} error={error === 'mismatch' ? t('next.settings.backup.mismatch') : undefined} />}
    </form>
  </Dialog>;
}
