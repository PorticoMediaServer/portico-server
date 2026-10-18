import {useState} from 'react';
import {Button, Dialog, Input, Notice, Text} from '../../ui';
import {errorText} from '../../app/errors';
import {useI18n} from '../../app/i18n';

/** Names the current filters and sort as a saved view the server reruns on open. */
export function SaveViewDialog({open, onClose, onSave, held, onRetry, onDiscard, describe}: {open: boolean; onClose: () => void; onSave: (name: string) => Promise<void>; /** A save sent with no answer: offered again under the name it was sent with, until it is confirmed or set aside. */ held?: string; /** Sends the held save again exactly as it was sent (its own filters, sort and presentation, never the toolbar's now). */ onRetry?: () => Promise<void>; /** Closing sets the unconfirmed save aside. */ onDiscard?: () => void; /** The words for a failure (the library's: an unconfirmed save in the way, or the server's answer). */ describe?: (e: unknown) => string}) {
  const {t} = useI18n();
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  // The button's loading lock cannot cover Enter, and each submission builds a
  // fresh service and mutation, so the guard belongs on the handler itself.
  const save = async () => {
    const sending = held ?? name.trim();
    if (busy || !sending) return;
    setBusy(true); setError(undefined);
    try { await (held !== undefined && onRetry ? onRetry() : onSave(sending)); setName(''); onClose(); }
    catch (e) { setError(describe ? describe(e) : errorText(e, 'library', 'save')); }
    finally { setBusy(false); }
  };
  const close = () => { if (held !== undefined) onDiscard?.(); setError(undefined); onClose(); };
  return (
    <Dialog open={open} onOpenChange={o => !o && close()} title={t('lib.saveViewTitle')} description={t('lib.saveViewLede')} width={420} actions={<><Button variant="ghost" label={t('action.cancel')} onClick={close} disabled={busy} /><Button variant="primary" label={t('lib.saveView')} loading={busy} disabled={!(held ?? name.trim())} onClick={() => void save()} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error">{error}</Notice> : held !== undefined ? <Notice tone="warning">{t('lib.savedViewUnconfirmed')}</Notice> : null}
        <Input label={t('lib.saveViewName')} value={held ?? name} readOnly={held !== undefined} onChange={e => setName(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && (held ?? name.trim())) { e.preventDefault(); void save(); } }} autoFocus placeholder={t('lib.saveViewPlaceholder')} />
        <Text variant="caption" tone="tertiary">{t('lib.saveViewHelp')}</Text>
      </div>
    </Dialog>
  );
}
