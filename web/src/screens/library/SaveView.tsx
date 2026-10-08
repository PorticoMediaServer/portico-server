import {useState} from 'react';
import {currentI18n} from '../../app/i18n';
import {Button, Dialog, Input, Notice, Text} from '../../ui';
import {errorText} from '../../app/errors';

/** Names the current filters and sort as a saved view the server reruns on open. */
export function SaveViewDialog({open, onClose, onSave}: {open: boolean; onClose: () => void; onSave: (name: string) => Promise<void>}) {
  const [name, setName] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string>();
  // The button's loading lock cannot cover Enter, and each submission builds a
  // fresh service and mutation, so the guard belongs on the handler itself.
  const save = async () => {
    if (busy || !name.trim()) return;
    setBusy(true); setError(undefined);
    try { await onSave(name.trim()); setName(''); onClose(); }
    catch (e) { setError(errorText(e, 'library', 'save')); }
    finally { setBusy(false); }
  };
  return (
    <Dialog open={open} onOpenChange={o => !o && onClose()} title={currentI18n().t('lib.saveViewTitle')} description={currentI18n().t('lib.saveViewLede')} width={420} actions={<><Button variant="ghost" label={currentI18n().t('action.cancel')} onClick={onClose} disabled={busy} /><Button variant="primary" label={currentI18n().t('lib.saveView')} loading={busy} disabled={!name.trim()} onClick={() => void save()} /></>}>
      <div style={{display: 'flex', flexDirection: 'column', gap: 12}}>
        {error ? <Notice tone="error">{error}</Notice> : null}
        <Input label={currentI18n().t('lib.saveViewName')} value={name} onChange={e => setName(e.target.value)} onKeyDown={e => { if (e.key === 'Enter' && name.trim()) { e.preventDefault(); void save(); } }} autoFocus placeholder={currentI18n().t('lib.saveViewPlaceholder')} />
        <Text variant="caption" tone="tertiary">{currentI18n().t('lib.saveViewHelp')}</Text>
      </div>
    </Dialog>
  );
}
