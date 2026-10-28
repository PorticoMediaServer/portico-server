import {useShellFacts} from '../../shell/fact-hooks';
import {forwardFacts} from '../../shell/fact-source';
import {useEffect, useState} from 'react';
import {defineWebScreen} from '../../route-registry';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {router} from '../../router';
import {useScreenAppearance} from '../browse/screen-preferences';
import {isReadingLibrary} from '../browse/reading-library';
import {HomeScreen} from './Home';
import {FirstLibraryState} from './first-library-state';
import {MemberEmptyState} from './member-empty-state';
import {createReadingContinueBindings, type ReadingContinueBindings} from './reading-continue';
import {useMusicMixes} from '../browse/music-mixes-hook';
import {MusicDiscoverMore} from '../browse/music-discover-navigation';

function HomeRouteScreen() {
  const captured = useProtectedWebScreenContext();
  useShellFacts(captured?.context);
  const appearance = useScreenAppearance(captured?.context);
  const context = captured?.context, supported = !!context?.facts.identity.libraries.some(library => isReadingLibrary(library.kind));
  const musicMixes = useMusicMixes(captured, !!context?.facts.identity.libraries.some(library => library.kind === 'music'));
  const [reading, setReading] = useState<Readonly<{context: NonNullable<typeof context>; binding: ReadingContinueBindings}>>();
  useEffect(() => {
    if (!context || !supported) return;
    const current = () => context.isCurrent() && router.state.location.pathname === '/';
    const binding = createReadingContinueBindings({context: forwardFacts(context, {isCurrent: current}), open: destination => {
      if (!current()) throw new DOMException('Home reading scope changed', 'AbortError');
      return router.navigate(destination);
    }});
    setReading({context, binding});
    return () => {void binding.dispose().catch(() => {});};
  }, [context, supported]);
  const identity = context?.facts.identity;
  if (captured && context?.isCurrent() && context.session().viewer.role === 'owner' && identity?.primaryProfile === true
    && identity.capabilities?.manageServer === true && identity.libraries.length === 0 && !identity.liveAvailable) return <FirstLibraryState captured={captured}/>;
  if (captured && context?.isCurrent() && context.session().viewer.role !== 'owner'
    && identity?.libraries.length === 0 && !identity.liveAvailable) return <MemberEmptyState captured={captured}/>;
  return captured ? <HomeScreen {...captured.binding.home} appearance={appearance} musicMixes={musicMixes} musicMixesMore={<MusicDiscoverMore captured={captured}/>}
    readingContinue={reading && reading.context === context && supported ? reading.binding : undefined}/> : null;
}

export default defineWebScreen({id: 'home', component: HomeRouteScreen});
