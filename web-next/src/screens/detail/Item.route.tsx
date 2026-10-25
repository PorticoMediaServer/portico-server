import {useEffect, useMemo, useState} from 'react';
import {useParams} from '@tanstack/react-router';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {createDetailBindings} from './bindings';
import {DetailScreen} from './Detail';
import {createReadBindings, ReadItemEntry, type ReadItemEligibility} from '../read';
import {resolveItemRead} from './read-resolution';
import {DetailLoading} from './Loading';
import {ScreenProblem} from './ScreenProblem';

function OrdinaryItem({captured, itemId}: {captured: ProtectedLibraryScreen; itemId: string}) {
  const context = useLibraryScreenContext(captured);
  const binding = useMemo(() => createDetailBindings(context), [context]);
  useEffect(() => () => binding.dispose(), [binding]);
  return <DetailScreen binding={binding} itemId={itemId}/>;
}
type ReadResolution = {phase: 'loading'} | {phase: 'ready'; eligibility: ReadItemEligibility | null} | {phase: 'error'; error: unknown};
function BoundItem({captured, itemId}: {captured: ProtectedLibraryScreen; itemId: string}) {
  const context = useLibraryScreenContext(captured), [resolution, setResolution] = useState<ReadResolution>({phase: 'loading'}), [retry, setRetry] = useState(0);
  useEffect(() => {
    const reading = createReadBindings(captured.context), request = new AbortController();
    let active = true;
    const current = () => active && context.isCurrent() && reading.authority.isCurrent();
    const off = reading.authority.subscribe(() => {request.abort();});
    setResolution({phase: 'loading'});
    void resolveItemRead(reading.client, itemId, request.signal, current)
      .then(eligibility => {if (current()) setResolution({phase: 'ready', eligibility});})
      .catch(error => {if (active && context.isCurrent()) setResolution({phase: 'error', error});});
    return () => {active = false;request.abort();off();void reading.dispose().catch(error => {if (context.isCurrent()) context.effects.reportError(error);});};
  }, [captured.context, context, itemId, retry]);
  if (resolution.phase === 'loading') return <DetailLoading/>;
  if (resolution.phase === 'error') return <ScreenProblem context={context} error={resolution.error} retry={() => setRetry(value => value + 1)}/>;
  return resolution.eligibility ? <ReadItemEntry itemId={itemId} eligibility={resolution.eligibility}/>
    : <OrdinaryItem captured={captured} itemId={itemId}/>;
}
function ItemRouteScreen() {
  const captured = useProtectedWebScreenContext();
  const {itemId} = useParams({from: '/item/$itemId'});
  return captured ? <BoundItem key={itemId} captured={captured} itemId={itemId}/> : null;
}
export default defineWebScreen({id: 'item', component: ItemRouteScreen, phoneChrome: 'standalone'});
