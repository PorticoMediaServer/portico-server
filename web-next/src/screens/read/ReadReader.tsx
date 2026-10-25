import {useCallback, useSyncExternalStore, type ReactNode} from 'react';
import {readerErrorCode, type ReaderController, type ReaderSnapshot} from '@portico/reader-core';
import {ReaderChrome, type ReaderChromeProps} from './ReaderChrome';
import {ReaderSurface, type ReaderSurfaceProps} from './ReaderSurface';
import styles from '../../kit/reader/surface.module.css';

export type ReaderChromePresentation = Omit<ReaderChromeProps,
  'title' | 'subtitle' | 'progression' | 'pageTurnsEnabled' | 'onBack' | 'onTurn' | 'onSeek' | 'subscribeSurfaceInput' | 'children' | 'fixedPage' | 'textSelectionActive'>;
export type ReadReaderProps<P extends object> = ReaderSurfaceProps<P> & Readonly<{
  /** Source/profile data is formatted by the real localized adapter, never a fixture here. */
  presentation: (snapshot: ReaderSnapshot<P>) => ReaderChromePresentation;
  /** Approved shared loading/error states; raw engine messages are never displayed. */
  state?: (snapshot: ReaderSnapshot<P>) => ReactNode;
  onBack: () => void;
}>;

/** The checked main controller is the single state and navigation owner for all formats. */
export function ReadReader<P extends object>({controller, source, format, initialLocation, metadata, signal, authority, reportError, presentation, state, onBack}: ReadReaderProps<P>) {
  const snapshot = useSyncExternalStore(controller.subscribe, controller.getSnapshot, controller.getSnapshot);
  const chrome = presentation(snapshot);
  const navigate = useCallback((target: Parameters<ReaderController<P>['navigate']>[0]) => {
    const active = controller.getSnapshot();
    if (!authority.isCurrent() || signal?.aborted || active.status !== 'ready' || active.sourceId !== source.id || active.sourceRevision !== source.revision) return;
    void controller.navigate(target).catch(error => { const active = controller.getSnapshot();if (authority.isCurrent() && !signal?.aborted && active.sourceId === source.id && active.sourceRevision === source.revision) reportError(readerErrorCode(error)); });
  }, [authority, controller, reportError, signal, source.id, source.revision]);
  const turn = useCallback((kind: 'next' | 'previous') => {
    if (controller.getSnapshot().canTurnAtFit) navigate({kind});
  }, [controller, navigate]);
  const seek = useCallback((value: number) => navigate({kind: 'progression', value}), [navigate]);
  const currentPublication = !signal?.aborted && snapshot.sourceId === source.id && snapshot.sourceRevision === source.revision;
  const publicationReady = currentPublication && snapshot.status === 'ready';
  return <ReaderChrome {...chrome} title={metadata ? metadata.title : snapshot.metadata?.title} subtitle={metadata ? metadata.author : snapshot.metadata?.author}
    progression={snapshot.location?.progression} pageTurnsEnabled={publicationReady && snapshot.canTurnAtFit}
    fixedPage={format !== 'epub'} holdOpen={chrome.holdOpen || !publicationReady || snapshot.selection !== null}
    textSelectionActive={snapshot.selection !== null}
    subscribeSurfaceInput={controller.subscribeSurfaceInput} onTurn={turn} onSeek={seek} onBack={onBack}>
    <ReaderSurface controller={controller} source={source} format={format} initialLocation={initialLocation} metadata={metadata} signal={signal} authority={authority} reportError={reportError} />
    {snapshot.status !== 'ready' && state && <div className={styles.state}>{state(snapshot)}</div>}
  </ReaderChrome>;
}
