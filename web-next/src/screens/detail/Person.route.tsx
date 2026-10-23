import {useEffect, useMemo} from 'react';
import {useParams} from '@tanstack/react-router';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {createPersonBindings} from './person-bindings';
import {PersonScreen} from './Person';

function BoundPerson({captured}: {captured: ProtectedLibraryScreen}) {
  const context = useLibraryScreenContext(captured);
  const binding = useMemo(() => createPersonBindings(context), [context]);
  const {personId} = useParams({from: '/person/$personId'});
  useEffect(() => () => binding.dispose(), [binding]);
  return <PersonScreen binding={binding} personId={personId}/>;
}
function PersonRouteScreen() {
  const captured = useProtectedWebScreenContext();
  return captured ? <BoundPerson captured={captured}/> : null;
}
export default defineWebScreen({id: 'person', component: PersonRouteScreen});
