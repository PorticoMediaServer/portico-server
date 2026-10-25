import {useEffect, useRef, useState} from 'react';
import {Button} from '../../../kit/controls';
import {LibraryManagementService} from '@core/library-management.ts';
import {libraryScanAction} from './model';

/** Runs one library scan through the bounded management service, which owns the
 * current library selection, the command and its reconciliation. */
export function LibraryScanAction({rowId, label, libraryId, service, onError}: Readonly<{
  rowId: string; label: string; libraryId: string; service: LibraryManagementService; onError: (error: unknown) => void;
}>) {
  const [busy, setBusy] = useState(false);
  const lifetime = useRef<{libraryId: string; running: boolean} | undefined>(undefined);
  useEffect(() => {
    const capture = {libraryId, running: false}; lifetime.current = capture; setBusy(false);
    return () => {if (lifetime.current === capture) lifetime.current = undefined;};
  }, [libraryId, service]);
  const mode = libraryScanAction(rowId);
  if (!mode) return null;
  const run = async () => {
    const capture = lifetime.current;
    if (!capture || capture.libraryId !== libraryId || capture.running) return;
    capture.running = true;
    const current = () => lifetime.current === capture;
    setBusy(true);
    try {
      await service.selectLibrary(libraryId);
      if (!current()) return;
      const selected = service.getSnapshot().selected;
      if (selected.phase !== 'ready' || selected.data?.library.id !== libraryId) throw new Error('library_changed');
      if (mode === 'full') await service.scan('full');
      else await service.scan();
      if (current() && service.getSnapshot().mutation.phase !== 'complete') throw new Error('library_scan_failed');
    } catch (error) {if (current()) onError(error);}
    finally {if (current()) {capture.running = false;setBusy(false);}}
  };
  return <Button variant="glass" size="small" loading={busy} disabled={busy} onClick={() => { void run(); }}>{label}</Button>;
}
