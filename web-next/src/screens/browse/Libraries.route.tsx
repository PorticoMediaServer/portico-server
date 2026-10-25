import {useEffect, useState} from 'react';
import {defaultI18n} from '@i18n';
import {readLibrarySummaries} from '@core/library-summary.ts';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {GridSkeleton, StateView} from '../../kit/states';
import {useShellWidth} from '../../shell/viewport';
import {useLoadingClock} from '../detail/Loading';
import {LibrariesScreen} from './Libraries';
import styles from './browse.module.css';

function LibrariesRouteScreen() {
  const captured = useProtectedWebScreenContext(), context = captured?.context, width = useShellWidth(), elapsed = useLoadingClock();
  const [libraries, setLibraries] = useState<Awaited<ReturnType<typeof readLibrarySummaries>>>();
  const [failed, setFailed] = useState(false), [attempt, setAttempt] = useState(0);
  useEffect(() => {
    if (!context) return;
    const life = new AbortController();setFailed(false);
    void readLibrarySummaries(context.api, life.signal).then(value => {
      if (!life.signal.aborted && context.isCurrent()) setLibraries(value);
    }).catch(() => {if (!life.signal.aborted && context.isCurrent()) setFailed(true);});
    return () => life.abort();
  }, [context, attempt]);
  if (!captured) return null;
  // The overview's own heading is known before its summaries are; the grid waits in the
  // shared shape instead of leaving the page blank.
  const heading = <h1>{defaultI18n.t('next.browse.libraries')}</h1>;
  if (failed) return <section className={styles.libraries}>{heading}<StateView title={defaultI18n.t('next.states.pageFailed')} body={defaultI18n.t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: defaultI18n.t('next.states.tryAgain'), onPress: () => setAttempt(value => value + 1)}}/></section>;
  return libraries ? <LibrariesScreen libraries={libraries} artwork={captured.context.artwork}
    onOpen={libraryId => {void router.navigate({to: '/library/$libraryId', params: {libraryId}, search: {}}).catch(captured.binding.home.effects.reportError);}}/>
    : <section className={styles.libraries}>{heading}<GridSkeleton elapsedMs={elapsed} window={{width, height: window.innerHeight, scrollTop: window.scrollY}} itemCount={6}/></section>;
}

export default defineWebScreen({id: 'libraries', component: LibrariesRouteScreen});
