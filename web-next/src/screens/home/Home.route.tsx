import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {useScreenAppearance} from '../browse/screen-preferences';
import {HomeScreen} from './Home';
import {useEffect, useMemo} from 'react';
import {useLibraryScreenContext, type ProtectedLibraryScreen} from '../browse/screen-context';
import {createSavedViewCapabilitiesReader} from '../saved/view-capabilities';

function HomeRouteScreen() {
  const captured = useProtectedWebScreenContext();
  return captured ? <CapturedHome captured={captured}/> : null;
}
function CapturedHome({captured}: {captured: ProtectedLibraryScreen}) {
  const appearance = useScreenAppearance(captured.context), context = useLibraryScreenContext(captured);
  const reader = useMemo(() => createSavedViewCapabilitiesReader(context.api, context.isCurrent), [context]);
  useEffect(() => () => reader.dispose(), [reader]);
  const customization = captured.binding.home.customization;
  return <HomeScreen {...captured.binding.home} appearance={appearance} customization={customization && {...customization, readViewCapabilities: reader.read}}/>;
}

export default defineWebScreen({id: 'home', component: HomeRouteScreen});
