import {useEffect, useMemo} from 'react';
import {LibraryMemory, type LibraryMemoryStorage} from '@core/library-memory.ts';
import {useViewerScope} from './viewer-scope';

/** The browser's own storage. A browser with storage switched off simply remembers for this visit only. */
const browserStorage: LibraryMemoryStorage = {
  read: key => { try { return localStorage.getItem(key); } catch { return undefined; } },
  write: (key, text) => { try { localStorage.setItem(key, text); } catch { /* full or blocked */ } },
  remove: key => { try { localStorage.removeItem(key); } catch { /* blocked */ } },
};

/**
 * The signed-in profile's remembered library choices in this browser (the tab, sort and filters
 * each library was last left on). Another profile in the same browser has its own.
 */
export function useLibraryMemory(): LibraryMemory {
  const scope = useViewerScope();
  const memory = useMemo(() => new LibraryMemory(browserStorage, scope), [scope]);
  useEffect(() => () => memory.dispose(), [memory]);
  return memory;
}
