import {useEffect, useState} from 'react';
import {defaultI18n} from '@i18n';
import {readLibrarySummaries} from '@core/library-summary.ts';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {StateView} from '../../kit/states';
import {LibrariesScreen} from './Libraries';

function LibrariesRouteScreen() {
  const captured = useProtectedWebScreenContext(), context = captured?.context;
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
  if (failed) return <StateView title={defaultI18n.t('next.states.pageFailed')} body={defaultI18n.t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: defaultI18n.t('next.states.tryAgain'), onPress: () => setAttempt(value => value + 1)}}/>;
  return libraries ? <LibrariesScreen libraries={libraries} artwork={captured.context.artwork}
    onOpen={libraryId => {void router.navigate({to: '/library/$libraryId', params: {libraryId}, search: {}}).catch(captured.binding.home.effects.reportError);}}/> : null;
}

export default defineWebScreen({id: 'libraries', component: LibrariesRouteScreen});
