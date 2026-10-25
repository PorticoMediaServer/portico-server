import {useEffect, useState, useSyncExternalStore, type ReactNode} from 'react';
import {ApiError} from '@core/index.ts';
import type {MessageId} from '@i18n';
import {HostedError, ServicesActions, ServicesAuthStage, ServicesFacts, ServicesHeading, ServicesLead, ServicesNotice, ServicesQuiet} from '../kit/accounts/ServicesAuth';
import {Button} from '../kit/controls';
import type {ServicesAccount} from './session';
import type {ServicesBootstrap} from './bootstrap';
import {OwnerApprovalFlow, type OwnerApprovalDisplay, type OwnerApprovalSnapshot} from './owner-approval-flow';
import {servicesI18n} from './i18n';
import {servicesRelativeTime} from './time';

/** The four drawn reasons, each with its own title and consequence sentence. The
 * reason is the signed claim's own fact; this only chooses the drawn words. */
export function claimApprovalCopy(reason: 'switch' | 'change-owner' | 'start-over'): {title: MessageId; lead: MessageId} {
  return reason === 'start-over'
    ? {title: 'services.claim.startOverTitle', lead: 'services.claim.startOverLead'}
    : reason === 'change-owner'
      ? {title: 'services.claim.changeOwnerTitle', lead: 'services.claim.changeOwnerLead'}
      : {title: 'services.claim.switchTitle', lead: 'services.claim.switchLead'};
}

/** auth24e2. Facts and the other-people sentence come from the authenticated
 * lookup; an absent count is an absent sentence, never zero or a URL hint. */
export function OwnerApprovalReview({display, requested, approved = false, busy = false, notice, onApprove, onDeny, onReturn}: {
  display: OwnerApprovalDisplay; requested: string; approved?: boolean; busy?: boolean; notice?: ReactNode;
  onApprove: () => void; onDeny: () => void; onReturn: () => void;
}) {
  const {t} = servicesI18n(), copy = claimApprovalCopy(display.reason);
  const previous = display.previousOwnerRole === 'admin' ? 'services.claim.previousAdmin'
    : display.previousOwnerRole === 'member' ? 'services.claim.previousMember'
      : display.previousOwnerRole === 'remove' ? 'services.claim.previousRemoved' : undefined;
  return <ServicesAuthStage variant="ownerReview">
    <ServicesHeading>{t(copy.title, {server: display.name})}</ServicesHeading>
    <ServicesLead>{t(copy.lead, {server: display.name, account: display.accountName})}</ServicesLead>
    <ServicesFacts items={[
      {id: 'server', label: t('web.claim.server'), value: display.name},
      {id: 'address', label: t('web.claim.address'), value: new URL(display.address).host},
      {id: 'account', label: t('web.claim.account'), value: display.accountName},
      {id: 'requested', label: t('web.claim.requested'), value: requested},
    ]} />
    {previous ? <ServicesNotice tone="info">{t(previous)}</ServicesNotice> : null}
    {display.otherPeopleCount === undefined ? null : <ServicesNotice tone="info">
      <b>{t('services.claim.becomesOwner', {account: display.accountName})}</b>{' '}{t('services.claim.otherPeople', {count: display.otherPeopleCount})}
    </ServicesNotice>}
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
