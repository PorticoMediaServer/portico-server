import {useEffect, useRef, useState} from 'react';
import type {LibraryContentApi} from '@core/library-content.ts';
import {randomId} from '@core/random-id.ts';
import {Button} from '../../../kit/controls';
import {LibraryLyricsFetch} from './lyrics-fetch';

/** Receipt acknowledges a scheduled job only; it never represents completed lyrics. */
export function LibraryLyricsAction({api, libraryId, isCurrent, disabled, label, onError}: Readonly<{
  api: LibraryContentApi; libraryId: string; isCurrent: () => boolean; disabled: boolean; label: string; onError: (error: unknown) => void;
}>) {
  const [busy, setBusy] = useState(false);
  const active = useRef<{command: LibraryLyricsFetch; api: LibraryContentApi; libraryId: string; isCurrent: () => boolean} | undefined>(undefined);
  useEffect(() => {
    const capture = {command: new LibraryLyricsFetch({api, libraryId, isCurrent, randomId}), api, libraryId, isCurrent};
    active.current = capture; setBusy(false);
    return () => {if (active.current === capture) active.current = undefined;capture.command.dispose();};
  }, [api, libraryId, isCurrent]);
  const run = async () => {
    const capture = active.current;
    if (disabled || !capture || capture.api !== api || capture.libraryId !== libraryId || capture.isCurrent !== isCurrent || !isCurrent()) return;
    setBusy(true);
    try {await capture.command.run();}
    catch (error) {if (active.current === capture && isCurrent()) onError(error);}
    finally {if (active.current === capture && isCurrent()) setBusy(false);}
  };
  return <Button variant="glass" size="small" disabled={disabled || busy} loading={busy} onClick={() => {void run();}}>{label}</Button>;
}
