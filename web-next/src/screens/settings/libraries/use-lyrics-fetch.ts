import {useEffect, useRef, useState} from 'react';
import type {LibraryContentApi} from '@core/library-content.ts';
import {randomId} from '@core/random-id.ts';
import {LibraryLyricsFetch} from './lyrics-fetch';

/** One library's explicit lyrics command for the current page lifetime. Both the
 * Metadata tab row and the library's ⋯ menu share this fence, so an uncertain
 * result keeps its idempotency key for a manual retry. */
export function useLibraryLyricsFetch({api, libraryId, isCurrent, onError}: Readonly<{
  api?: LibraryContentApi; libraryId: string; isCurrent: () => boolean; onError: (error: unknown) => void;
}>) {
  const [busy, setBusy] = useState(false);
  const active = useRef<{command: LibraryLyricsFetch; api: LibraryContentApi; libraryId: string; isCurrent: () => boolean} | undefined>(undefined);
  useEffect(() => {
    if (!api) {active.current = undefined; setBusy(false); return;}
    const capture = {command: new LibraryLyricsFetch({api, libraryId, isCurrent, randomId}), api, libraryId, isCurrent};
    active.current = capture; setBusy(false);
    return () => {if (active.current === capture) active.current = undefined;capture.command.dispose();};
  }, [api, libraryId, isCurrent]);
  const run = async () => {
    const capture = active.current;
    if (!api || !capture || capture.api !== api || capture.libraryId !== libraryId || capture.isCurrent !== isCurrent || !isCurrent()) return;
    setBusy(true);
    try {await capture.command.run();}
    catch (error) {if (active.current === capture && isCurrent()) onError(error);}
    finally {if (active.current === capture && isCurrent()) setBusy(false);}
  };
  return {busy, run};
}
