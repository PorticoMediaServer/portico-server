import {useEffect, useState, useSyncExternalStore} from 'react';
import {useProtectedWebScreenContext} from '../../route-registry-view';
import {ViewerFeedback} from './feedback-controller';
import {installFeedback} from './feedback';
import {FeedbackDialog} from '../../kit/accounts/FeedbackDialog';

export function FeedbackHost() {
  const captured = useProtectedWebScreenContext();
  const [controller, setController] = useState<ViewerFeedback>();
  const context = captured?.context;
  useEffect(() => {
    if (!context?.isCurrent()) return;
    const session = context.session(), viewer = session.viewer;
    let active = true;
    const current = () => active && context.isCurrent() && context.session().sessionFamilyId === session.sessionFamilyId &&
      (['serverId', 'authority', 'accountId', 'profileId'] as const).every(key => context.session().viewer[key] === viewer[key]);
    const owned = new ViewerFeedback(context.api, viewer, current);
    const off = installFeedback(owned); setController(owned); owned.initialize();
    return () => {active = false;owned.dispose();off();};
  }, [context]);
  return controller && context?.isCurrent() ? <BoundFeedback controller={controller} /> : null;
}
function BoundFeedback({controller}: {controller: ViewerFeedback}) {
  const state = useSyncExternalStore(controller.subscribe, controller.getSnapshot);
  return state.open ? <FeedbackDialog snapshot={state} title={state.target?.item.title} hasPlayback={!!state.target?.playback} problem={controller.problem()} onChange={patch => controller.change(patch)} onSubmit={() => {void controller.submit();}} onClose={controller.close} /> : null;
}
