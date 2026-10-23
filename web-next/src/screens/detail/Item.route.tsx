import {useEffect, useMemo} from 'react';
import {useParams} from '@tanstack/react-router';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {createDetailBindings} from './bindings';
import {DetailScreen} from './Detail';

function BoundItem({captured}: {captured: ProtectedLibraryScreen}) {
  const context = useLibraryScreenContext(captured);
  const binding = useMemo(() => createDetailBindings(context), [context]);
  const {itemId} = useParams({from: '/item/$itemId'});
  useEffect(() => () => binding.dispose(), [binding]);
  return <DetailScreen binding={binding} itemId={itemId}/>;
}
function ItemRouteScreen() {
  const captured = useProtectedWebScreenContext();
  return captured ? <BoundItem captured={captured}/> : null;
}
export default defineWebScreen({id: 'item', component: ItemRouteScreen});
