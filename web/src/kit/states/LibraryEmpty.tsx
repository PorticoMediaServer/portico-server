import {useRef, useEffect, useState} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryContentApi, ContentScope} from '@core/library-content.ts';
import {LibraryManagementService} from '@core/library-management.ts';
import {StateView} from './State';
const t = defaultI18n.t;
/** Only the owner's actual, complete one-job history establishes a first scan, or the server's own
 * word that this library's image is still being built (`importing`), which anyone may see. Library
 * pages never show scanning progress (Justin, 21 Oct); only the owner is offered View activity. */
export function LibraryEmpty({api, scope, libraryId, libraryName, owner, importing = false, onActivity}: Readonly<{
  api: LibraryContentApi; scope: ContentScope; libraryId: string; libraryName: string; owner: boolean; importing?: boolean; onActivity: () => void;
}>) {
  const [firstScan, setFirstScan] = useState(false);
  // Publish only at commit, then sample once for this setup's original lifetime.
  const setupPorts = useRef({scope});
  useEffect(() => {setupPorts.current = {scope};});
  useEffect(() => {
    const {scope} = setupPorts.current;
    setFirstScan(false);
    if (!owner) return;
    let current = true;
    const service = new LibraryManagementService({api, scope});
    const update = () => {
      if (!current) return;
      const {selected, jobs} = service.getSnapshot(), library = selected.data?.library, history = jobs.data;
      setFirstScan(selected.phase === 'ready' && jobs.phase === 'ready' && library?.id === libraryId &&
        !!library.lastScan && ['queued', 'running'].includes(library.lastScan.status) && !!history &&
        !history.nextCursor && history.items.length === 1 && history.items[0].id === library.lastScan.id);
    };
    const off = service.subscribe(update);
    void service.selectLibrary(libraryId).then(update).catch(() => {if (current) setFirstScan(false);});
    return () => {current = false;off();service.dispose();};
  }, [api, scope.serverId, scope.viewerId, libraryId, owner]);
  return firstScan || importing ? <StateView title={t('next.states.firstScan')} body={t('next.states.firstScanBody', {library: libraryName})}
    {...(owner ? {primary: {id: 'activity', label: t('next.states.viewActivity'), onPress: onActivity}} : {})}/>
    : <StateView title={t('library.emptyLibrary')} body={t('library.emptyLibraryBody')}/>;
}
