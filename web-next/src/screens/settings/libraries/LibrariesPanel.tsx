import {useEffect, useState, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryContentApi} from '@core/library-content.ts';
import {LibraryManagementService, type ManagedLibrary} from '@core/library-management.ts';
import {reorderLibraries} from '@core/server-admin/libraries.ts';
import {metadataAgentPath, parseMetadataAgent} from '@core/server-admin/metadata-agent.ts';
import {Button} from '../../../kit/controls';
import {Menu} from '../../../kit/overlays/Popover';
import {Icon} from '../../../kit/Icon';
import {router} from '../../../router';
import {StateView} from '../../../kit/states';
import styles from './libraries.module.css';
import {libraryRows, movedIds} from './model';
import {AddLibrary} from './AddLibrary';
import {RemoveLibrary} from './RemoveLibrary';

const t = defaultI18n.t;

/** The owner's Libraries settings list. Server facts only: each library's name,
 * its folder and whether a scan is running. A scan is started here, from the
 * row the owner chose, and the page never shows scan progress or counts.
 *
 * Add library and the folder explorer are the drawn frame's own later job
 * (owner-libraries.html#8–#12, P1-SETTINGS-WEB-LIBRARIES-01); this list is the
 * M0 flow's server Libraries page with its scan. */
export function LibrariesPanel({api, service, onEdit, onError}: {
  api: LibraryContentApi; service: LibraryManagementService; onEdit: (libraryId: string) => void; onError: (error: unknown) => void;
}) {
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  const [busy, setBusy] = useState<string>(), [menu, setMenu] = useState<string>(), [adding, setAdding] = useState(false), [removing, setRemoving] = useState<string>();
  const directory = snapshot.directory;
  const libraries = directory.data?.items ?? [];
  const menuLibrary = libraries.find(library => library.id === menu);
  const [agentFact, setAgentFact] = useState<Readonly<{api: LibraryContentApi; library: ManagedLibrary; online: boolean}>>();
  useEffect(() => {
    if (!menuLibrary) return;
    let live = true;
    void api.request<unknown>(metadataAgentPath(menuLibrary.id), 'GET').then(raw => {
      if (!live) return;
      const agent = parseMetadataAgent(raw);
      setAgentFact({api, library: menuLibrary, online: agent.libraryId === menuLibrary.id && agent.libraryKind === menuLibrary.kind && agent.agent === 'online'});
    }).catch(() => {if (live) setAgentFact({api, library: menuLibrary, online: false});});
    return () => {live = false;};
  }, [api, menuLibrary]);
  const onlineMenu = agentFact?.api === api && agentFact.library === menuLibrary && agentFact.online;
  useEffect(() => { void service.loadLibraries().catch(onError); }, [service, onError]);
  const scan = async (library: ManagedLibrary) => {
    setBusy(library.id);
    try { await service.selectLibrary(library.id); await service.scan(); await service.loadLibraries(); }
    catch (error) { onError(error); }
    finally { setBusy(undefined); }
  };
  const move = async (library: ManagedLibrary, offset: -1 | 1) => {
    const next = movedIds(libraries, library.id, offset);
    if (!next) return;
    setBusy(library.id);
    try { await reorderLibraries(api, next); await service.loadLibraries(); }
    catch (error) { onError(error); }
    finally { setBusy(undefined); }
  };
  if (directory.phase === 'error' || directory.phase === 'refresh-required') {
    return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
      primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => { void service.loadLibraries().catch(onError); }}} />;
  }
  if (!directory.data) return <p role="status" className={styles.loading}>{t('next.settings.loading')}</p>;
  const rows = libraryRows(libraries);
  return <>
    <div className={styles.listHeader}>
      <Button variant="primary" size="small" icon="i-plus" onClick={() => setAdding(true)}>{t('next.settingsSpec.server.libraries.groups.0.rows.0.control.addLabel')}</Button>
    </div>
    {!libraries.length && <p className={styles.empty}>{t('next.settings.libraries.empty')}</p>}
    <ol className={styles.list} aria-label={t('next.settingsSpec.server.libraries.title')}>
    {rows.map(row => <li key={row.id} className={styles.row} data-library-row={row.id}>
      <span className={styles.identity}>
        <b className={styles.name}>{row.name}</b>
        <small className={styles.folder}>{row.folder}</small>
      </span>
      {row.scanning
        ? <span className={styles.scanning} role="status"><Icon name="i-refresh" size={16} />{t('next.settings.libraries.scanning')}</span>
        : <Button variant="glass" size="small" disabled={!row.canScan || busy === row.id} loading={busy === row.id}
            onClick={() => { const library = libraries.find(value => value.id === row.id); if (library) void scan(library); }}>
            {t('next.settingsSpec.server.libraries.groups.0.rows.0.control.itemActions.0.label')}
          </Button>}
      <Menu open={menu === row.id} onOpenChange={open => {setAgentFact(undefined); setMenu(open ? row.id : undefined);}} title={row.name} items={[
        {id: 'edit', label: t('next.settingsSpec.server.libraries.groups.0.rows.0.control.itemActions.1.label'), onSelect: () => onEdit(row.id)},
        ...(row.canScan && !row.scanning ? [{id: 'scan', label: t('next.settingsSpec.server.libraries.groups.0.rows.0.control.itemActions.0.label'), onSelect: () => {
          const library = libraries.find(value => value.id === row.id); if (library) void scan(library);
        }}] : []),
        ...(onlineMenu && menu === row.id ? [{id: 'unmatched', label: t('next.settings.libraries.unmatched'), onSelect: () => {
          void router.navigate({to: '/library/$libraryId', params: {libraryId: row.id}, search: {unmatched: true}}).catch(onError);
        }}] : []),
        {id: 'up', label: t('next.settings.moveUp', {name: row.name}), disabled: !row.canMoveUp, onSelect: () => {
          const library = libraries.find(value => value.id === row.id); if (library) void move(library, -1);
        }},
        {id: 'down', label: t('next.settings.moveDown', {name: row.name}), disabled: !row.canMoveDown, onSelect: () => {
          const library = libraries.find(value => value.id === row.id); if (library) void move(library, 1);
        }},
        {id: 'remove', label: t('next.settingsSpec.server.libraries.panels.library.tabs.0.groups.2.rows.0.control.label'), destructive: true,
          onSelect: () => setRemoving(row.id)},
      ]} trigger={<Button variant="icon" icon="i-more" iconOnly aria-label={t('next.settings.actions')} disabled={busy === row.id} />} />
    </li>)}
    </ol>
    <AddLibrary open={adding} onOpenChange={setAdding} api={api} service={service} onError={onError} />
    <RemoveLibrary open={!!removing} onOpenChange={open => { if (!open) setRemoving(undefined); }} libraryId={removing ?? ''} service={service} api={api}
      onRemoved={() => { setRemoving(undefined); void service.loadLibraries().catch(onError); }} onError={onError} />
  </>;
}
