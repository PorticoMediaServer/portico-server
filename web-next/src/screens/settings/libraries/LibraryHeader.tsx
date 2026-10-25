import {useEffect, useSyncExternalStore} from 'react';
import {defaultI18n} from '@i18n';
import type {LibraryManagementService} from '@core/library-management.ts';
import type {ShellArtwork} from '../../../shell/artwork';
import {Artwork} from '../../../kit/artwork/Artwork';
import {LibraryScanAction} from './LibraryActions';
import styles from './library-header.module.css';

const pivots: Readonly<Record<string, string>> = {movie: 'movies', show: 'shows', artist: 'artists', album: 'albums', song: 'songs', book: 'books'};
/** Only the selected server library's bounded summary supplies counts and art. */
export function LibraryHeader({libraryId, name, service, artwork, onError}: Readonly<{
  libraryId: string; name: string; service: LibraryManagementService; artwork: ShellArtwork; onError: (error: unknown) => void;
}>) {
  const snapshot = useSyncExternalStore(service.subscribe, service.getSnapshot, service.getSnapshot);
  useEffect(() => {
    if (service.getSnapshot().selectedLibraryId !== libraryId) void service.selectLibrary(libraryId).catch(onError);
  }, [service, libraryId, onError]);
  const library = snapshot.selected.data?.library.id === libraryId ? snapshot.selected.data.library : snapshot.directory.data?.items.find(item => item.id === libraryId);
  const square = library?.kind === 'music' || library?.kind === 'audiobook';
  const scan = library?.lastScan;
  const active = scan && ['queued', 'running', 'paused'].includes(scan.status);
  return <header className={styles.hero}>
    <div className={styles.strip} data-square={square || undefined} aria-hidden="true">
      {library?.summary?.coverUrls.map((path, order) => {
        const source = artwork.foreground(path, 240, square ? 1 : 2 / 3, {group: 'settings-library', order});
        return source && <span key={path}><Artwork source={source} alt="" visible /></span>;
      })}
    </div>
    <div className={styles.content}>
      <h1>{name}</h1>
      <div className={styles.facts}>
        {library?.summary?.counts.map(count => <span key={count.kind}>{defaultI18n.t(count.atLeast ? 'lib.countAtLeast' : 'lib.count', {count: count.count, pivot: pivots[count.kind] ?? count.kind})}</span>)}
        {active ? <span role="status">{defaultI18n.t('next.settings.libraries.scanning')}</span> : scan?.finishedAt && Number.isFinite(Date.parse(scan.finishedAt)) && ['complete', 'complete_with_warnings'].includes(scan.status)
          ? <span>{defaultI18n.t('server.libraries.scanned', {when: defaultI18n.dateTime(scan.finishedAt)})}</span> : null}
      </div>
      {library?.actions.includes('scan') && <div className={styles.actions}><LibraryScanAction rowId="libraries.scanNow" label={defaultI18n.t('next.settingsSpec.server.libraries.panels.library.tabs.3.groups.0.rows.0.control.label')} libraryId={libraryId} service={service} onError={onError} /></div>}
    </div>
  </header>;
}
