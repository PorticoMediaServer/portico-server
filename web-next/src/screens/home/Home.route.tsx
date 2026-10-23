import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {useScreenAppearance} from '../browse/screen-preferences';
import {HomeScreen} from './Home';

function HomeRouteScreen() {
  const captured = useProtectedWebScreenContext();
  const appearance = useScreenAppearance(captured?.context);
  return captured ? <HomeScreen {...captured.binding.home} appearance={appearance}/> : null;
}

export default defineWebScreen({id: 'home', component: HomeRouteScreen});
