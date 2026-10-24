import {useState} from 'react';
import {defaultI18n} from '@i18n';
import {Button} from '../../../kit/controls';
import {LibraryManagementService} from '@core/library-management.ts';
import {libraryScanAction} from './model';

const t = defaultI18n.t;

/** Runs one library scan through the bounded management service, which owns the
 * current library selection, the command and its reconciliation. */
export function LibraryScanAction({rowId, label, libraryId, service, onError}: Readonly<{
  rowId: string; label: string; libraryId: string; service: LibraryManagementService; onError: (error: unknown) => void;
}>) {
  const [busy, setBusy] = useState(false);
  const mode = libraryScanAction(rowId);
  if (!mode) return null;
  const run = async () => {
    setBusy(true);
    try {
      await service.selectLibrary(libraryId);
      if (mode === 'full') await service.scan('full');
      else await service.scan();
    } catch (error) { onError(error); }
    finally { setBusy(false); }
  };
  return <Button variant="glass" size="small" loading={busy} disabled={busy} onClick={() => { void run(); }}>{label}</Button>;
}
