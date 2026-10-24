import {useEffect, useMemo} from 'react';
import {useSearch} from '@tanstack/react-router';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {createSearchBindings} from './bindings';
import {SearchScreen} from './Search';

function BoundSearch({captured}: {captured: ProtectedLibraryScreen}) {
  const context = useLibraryScreenContext(captured);
  const binding = useMemo(() => createSearchBindings(context), [context]);
  useEffect(() => () => binding.dispose(), [binding]);
  const search = useSearch({from: '/search'});
  return <SearchScreen binding={binding} linkedQuery={typeof search.q === 'string' ? search.q : undefined} onOpenLive={sourceId => {
    if (!context.isCurrent()) return;
    void router.navigate({to: '/live', search: sourceId ? {sourceId} : {}}).catch(context.effects.reportError);
  }} onConsumeLinkedQuery={() => {
    void router.navigate({to: '/search', search: {q: ''}, replace: true, resetScroll: false}).catch(context.effects.reportError);
  }}/>;
}
function SearchRouteScreen() {
  const captured = useProtectedWebScreenContext();
  return captured ? <BoundSearch captured={captured}/> : null;
}
export default defineWebScreen({id: 'search', component: SearchRouteScreen});
