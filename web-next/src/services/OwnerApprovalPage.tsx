import {useEffect, useState, useSyncExternalStore, type ReactNode} from 'react';
import {ApiError} from '@core/index.ts';
import {HostedError, ServicesActions, ServicesAuthStage, ServicesFacts, ServicesHeading, ServicesLead, ServicesQuiet} from '../kit/accounts/ServicesAuth';
import {Button} from '../kit/controls';
import type {ServicesAccount} from './session';
import type {ServicesBootstrap} from './bootstrap';
import {OwnerApprovalFlow, type OwnerApprovalDisplay, type OwnerApprovalSnapshot} from './owner-approval-flow';
import {servicesI18n} from './i18n';
import {servicesRelativeTime} from './time';

/** auth24e2. Display facts come from authenticated lookup; the optional notice
 * slot never derives a count from the URL or an absent producer field. */
export function OwnerApprovalReview({display, requested, approved = false, busy = false, notice, onApprove, onDeny, onReturn}: {
  display: OwnerApprovalDisplay; requested: string; approved?: boolean; busy?: boolean; notice?: ReactNode;
  onApprove: () => void; onDeny: () => void; onReturn: () => void;
}) {
  const {t} = servicesI18n();
  return <ServicesAuthStage variant="ownerReview">
    <ServicesHeading>{t('services.claim.switchTitle', {server: display.name})}</ServicesHeading>
    <ServicesLead>{t('services.claim.switchLead')}</ServicesLead>
    <ServicesFacts items={[
      {id: 'server', label: t('web.claim.server'), value: display.name},
      {id: 'address', label: t('web.claim.address'), value: new URL(display.address).host},
      {id: 'account', label: t('web.claim.account'), value: display.accountName},
      {id: 'requested', label: t('web.claim.requested'), value: requested},
    ]} />
    {notice}
    <ServicesActions>{approved
      ? <Button variant="primary" autoFocus loading={busy} disabled={busy} onClick={onReturn}>{t('web.claim.returnTo', {server: display.name})}</Button>
      : <><Button variant="primary" autoFocus loading={busy} disabled={busy} onClick={onApprove}>{t('web.claim.approve')}</Button>
        <Button disabled={busy} onClick={onDeny}>{t('web.claim.deny')}</Button></>}
    </ServicesActions>
    <ServicesQuiet>{t('services.claim.returnHint', {server: display.name})}</ServicesQuiet>
  </ServicesAuthStage>;
}

const empty: OwnerApprovalSnapshot = Object.freeze({phase: 'loading', busy: false});
const emptySnapshot = () => empty;
const noSubscription = () => () => {};
type Instance = {scope: string; flow?: OwnerApprovalFlow; error?: unknown};

/** Account/session restoration belongs to ServicesRoot. This effect owns only
 * its scoped review instance; changing family never renders its predecessor. */
export function OwnerApprovalPage({account, bootstrap}: {account: ServicesAccount; bootstrap: ServicesBootstrap}) {
  const {t} = servicesI18n(), session = useSyncExternalStore(account.service.subscribe, account.service.getSnapshot).session;
  const scope = session?.familyId ? JSON.stringify([session.account.id, session.familyId]) : '';
  const [instance, setInstance] = useState<Instance>();
  const active = instance?.scope === scope ? instance : undefined, flow = active?.flow;
  const state = useSyncExternalStore(flow?.subscribe ?? noSubscription, flow?.getSnapshot ?? emptySnapshot);
  useEffect(() => {
    if (!scope) return;
    let created: OwnerApprovalFlow | undefined;
    try {
      const controller = bootstrap.ownerApproval(account);
      if (!controller) throw new ApiError(409, 'claim_stale', 'claim_stale', false);
      created = new OwnerApprovalFlow(account, controller, location => window.location.assign(location));
      setInstance({scope, flow: created}); void created.start();
    } catch (error) {setInstance({scope, error});}
    return () => created?.dispose();
  }, [account, bootstrap, scope]);
  useEffect(() => {
    if (!flow || !state.display) return;
    const timer = setTimeout(() => flow.expire(), Math.max(0, Date.parse(state.display.expiresAt) - Date.now()));
    return () => clearTimeout(timer);
  }, [flow, state.display]);
  const error = active?.error ?? state.error;
  if (error || state.phase === 'expired') return <ServicesAuthStage variant="ownerReview">
    <ServicesHeading>{t('web.claim.titleGeneric')}</ServicesHeading>
    {state.phase === 'expired' ? <ServicesLead>{t('web.claim.expired')}</ServicesLead> : <HostedError error={error} />}
    <ServicesActions>{!flow || state.phase === 'expired'
      ? <Button onClick={() => window.location.assign('/')}>{t('web.claim.backToPortico')}</Button>
      : <Button onClick={() => {void flow.start();}}>{t('next.states.tryAgain')}</Button>}</ServicesActions>
  </ServicesAuthStage>;
  if (!state.display || state.phase === 'loading') return <ServicesAuthStage variant="ownerReview"><></></ServicesAuthStage>;
  const requested = Date.now() - Date.parse(state.display.requestedAt) < 45000 ? t('services.claim.justNow') : servicesRelativeTime(servicesI18n(), state.display.requestedAt);
  return <OwnerApprovalReview display={state.display} requested={requested} approved={state.phase !== 'review'} busy={state.busy || state.phase === 'returning'}
    onApprove={() => {void flow?.decide('approve');}} onDeny={() => {void flow?.decide('deny');}} onReturn={() => {void flow?.complete();}} />;
}
