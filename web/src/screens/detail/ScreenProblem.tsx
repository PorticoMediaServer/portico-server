import {presentError} from '@core/presentation/errors.ts';
import {serviceI18n} from '@core/presentation/service-text.ts';
import {StateLayout} from '../../kit/states';
import type {ScreenContext} from './context';
import {t} from './text';

/** A denied title carries no cached artwork. Names are genuine current-scope facts only. */
export function ScreenProblem({context, error, subject, retry}: {context: ScreenContext; error: unknown; subject?: string; retry: () => void}) {
  const problem = presentError(error, 'detail', {i18n: serviceI18n()}), identity = context.identity;
  if (problem.silent || !context.isCurrent()) return null;
  const home = context.effects.openHome, back = context.effects.back, chooseProfile = context.effects.chooseProfile;
  const guarded = (action: () => void) => () => { if (context.isCurrent()) action(); };
  if (problem.category === 'not-found') return <StateLayout title={identity && subject ? t('next.states.removed', {title: subject, server: identity.serverName}) : t('next.states.notFound')}
    body={t(identity && subject ? 'next.states.removedBody' : 'next.states.notFoundBody')} icon="i-info"
    primary={home ? {id: 'home', label: t('next.states.goHome'), onPress: guarded(home)} : undefined}
    secondary={back ? {id: 'back', label: t('next.states.back'), onPress: guarded(back)} : undefined} />;
  if (problem.category === 'permission' && identity) return <StateLayout title={t('next.states.restricted', {profile: identity.profileName})} body={t('next.states.restrictedBody')} icon="i-lock"
    primary={chooseProfile ? {id: 'profile', label: t('next.states.switchProfile'), onPress: guarded(chooseProfile)} : undefined}
    secondary={back ? {id: 'back', label: t('next.states.back'), onPress: guarded(back)} : undefined} />;
  return <StateLayout title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')} icon="i-warn"
    primary={problem.action === 'none' ? undefined : {id: 'retry', label: t('next.states.tryAgain'), onPress: guarded(retry)}}
    secondary={home ? {id: 'home', label: t('next.states.goHome'), onPress: guarded(home)} : undefined} />;
}
