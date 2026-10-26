import {useCallback, useEffect, useLayoutEffect, useState, useSyncExternalStore} from 'react';
import {createRootRouteWithContext, createRoute, createRouter, Outlet, RouterProvider} from '@tanstack/react-router';
import type {JoinLink} from '@core/join-link.ts';
import {IconSprite} from '../kit/Icon';
import {StateView} from '../kit/states';
import {ServicesAuthStage, ServicesActions, ServicesHeading, ServicesLead, ServicesNotice, ServicesStateIcon} from '../kit/accounts/ServicesAuth';
import {Button} from '../kit/controls';
import {servicesAccount, type ServicesAccount} from './session';
import {ServicesBootstrap} from './bootstrap';
import {useAccountResource} from './use-account-resource';
import {servicesLinkStore, type LinkCapture} from './intake';
import {servicesI18n} from './i18n';
import {ServicesAuthForm} from './AuthForm';
import {DevicePage} from './DevicePage';
import {ResetPage, VerifyPage} from './LinkPages';
import {AccountPage} from './AccountPage';
import {SignedOutRecoveryCodes} from './SecurityPanel';
import {JoinPage, ShortJoinPage} from './JoinPage';
import {ServicesHomePage} from './HomePage';
import {RegisteredSignInPage} from './SignInPage';
import type {SignInCapture} from './sign-in';
import type {HostedOwnerClaimCapture} from './owner-claim';
import {OwnerApprovalPage} from './OwnerApprovalPage';
import {SetupApprovalPage} from './SetupApprovalPage';
import {HostedSetupApproval} from '../screens/setup/HostedSetupApproval';

type Context = {account: ServicesAccount; bootstrap: ServicesBootstrap; invalidContinuation: boolean; join?: JoinLink; plainShortJoin?: boolean; identityTransaction?: string};
const root = createRootRouteWithContext<Context>()({component: ServicesRoot, notFoundComponent: InvalidLink});
const home = createRoute({getParentRoute: () => root, path: '/', component: HomeRoute});
const workspace = createRoute({getParentRoute: () => root, path: '/account', component: AccountRoute,
  validateSearch: (value: Record<string, unknown>) => {
    const transaction = typeof value.identityTransaction === 'string' && /^[A-Za-z0-9_-]{1,160}$/.test(value.identityTransaction) ? value.identityTransaction : undefined;
    return {tab: transaction ? 'security' : value.tab === 'security' || value.tab === 'servers' || value.tab === 'notifications' ? value.tab : 'devices', ...(transaction ? {identityTransaction: transaction} : {})};
  }});
const device = createRoute({getParentRoute: () => root, path: '/device', component: DeviceRoute});
const reset = createRoute({getParentRoute: () => root, path: '/reset', component: ResetRoute});
const verify = createRoute({getParentRoute: () => root, path: '/verify-email', component: VerifyRoute});
const invitation = createRoute({getParentRoute: () => root, path: '/join', component: JoinRoute});
// Owner continuation stays separate from the ordinary signed sign-in root.
const claim = createRoute({getParentRoute: () => root, path: '/claim', component: ClaimStart});
type ServicesRouter = ReturnType<typeof makeRouter>;
function makeRouter(context: Context) {
  return createRouter({routeTree: root.addChildren([home, workspace, device, reset, verify, claim, invitation]), context});
}
// This router's types stay local. The server app remains the only global TanStack Register.
export function ServicesApp({capture, invalidContinuation, join, plainShortJoin, signIn, owner, identityTransaction}: {capture?: LinkCapture; invalidContinuation?: boolean; join?: JoinLink; plainShortJoin?: boolean; signIn?: SignInCapture; owner?: HostedOwnerClaimCapture; identityTransaction?: string} = {}) {
  // main.tsx captures and scrubs intake once, before React. A discarded render
  // must not reread a scrubbed URL or allocate a terminal bootstrap controller.
  const create = useCallback(() => {
    const account = servicesAccount(), bootstrap = new ServicesBootstrap(account, servicesLinkStore, capture, {origin: location.origin, capture: signIn},
      location.pathname === '/claim' ? {origin: location.origin, capture: owner} : undefined);
    return {account, bootstrap, router: makeRouter({account, bootstrap, invalidContinuation: invalidContinuation === true, join, plainShortJoin, identityTransaction}), dispose: () => bootstrap.dispose()};
  }, [capture, invalidContinuation, join, plainShortJoin, signIn, owner, identityTransaction]);
  const bound = useAccountResource(create);
  return <><IconSprite />{bound ? <MountedServicesRouter key={bound.id} value={bound.value} /> : <ServicesAuthStage><></></ServicesAuthStage>}</>;
}
function MountedServicesRouter({value}: {value: {bootstrap: ServicesBootstrap; router: ServicesRouter}}) {
  useLayoutEffect(() => {void value.bootstrap.start();}, [value]);
  return <RouterProvider router={value.router} />;
}
function ServicesRoot() {
  const {account, bootstrap, invalidContinuation} = root.useRouteContext<ServicesRouter>();
  const state = useSyncExternalStore(bootstrap.subscribe, bootstrap.getSnapshot);
  const {t} = servicesI18n();
  if (state.phase === 'restoring') return <ServicesAuthStage><></></ServicesAuthStage>;
  if (state.phase === 'error') return <StateView title={t('next.states.pageFailed')} body={t('next.states.pageFailedBody')}
    primary={{id: 'retry', label: t('next.states.tryAgain'), onPress: () => { void bootstrap.start(); }}} />;
  if (invalidContinuation || state.invalidSignIn || state.invalidOwner) return <InvalidLink />;
  if (state.signIn && location.pathname === '/') return <RegisteredSignInPage account={account} pending={state.signIn} />;
  return <Outlet />;
}
function useAccount() {
  const {account} = root.useRouteContext<ServicesRouter>();
  const state = useSyncExternalStore(account.service.subscribe, account.service.getSnapshot);
  return {account, session: state.session};
}
function HomeRoute() {
  const {account, session} = useAccount();
  const [authenticating, setAuthenticating] = useState(() => !session);
  useEffect(() => { if (!session) setAuthenticating(true); }, [session]);
  // Explicit recovery/create links do not resume a different transaction from the cookie.
  const mode = new URLSearchParams(location.search).get('mode');
  if (!session || authenticating) return <ServicesAuthForm account={account} initialMode={mode === 'forgot' || mode === 'create' ? mode : 'signin'} onSignedIn={() => setAuthenticating(false)} />;
  return <ServicesHomePage account={account} session={session} />;
}
function AccountRoute() {
  const {identityTransaction} = root.useRouteContext<ServicesRouter>();
  const {account, session} = useAccount(), {tab} = workspace.useSearch<ServicesRouter>();
  const [authenticating, setAuthenticating] = useState(() => !session), [deleted, setDeleted] = useState(false);
  const {t} = servicesI18n();
  const [securityEnded, setSecurityEnded] = useState<{codes?: readonly string[]}>();
  useEffect(() => {if (session) {setDeleted(false);setSecurityEnded(undefined);}}, [session]);
  useEffect(() => { if (!session) setAuthenticating(true); }, [session]);
  if (securityEnded?.codes?.length && !session) return <SignedOutRecoveryCodes codes={securityEnded.codes} onDone={() => setSecurityEnded({})}/>;
  if (deleted && !session) return <ServicesAuthStage><ServicesStateIcon/><ServicesHeading>{t('web.deleteAccount.doneTitle')}</ServicesHeading>
    <ServicesLead>{t('web.deleteAccount.doneBody')}</ServicesLead><ServicesActions><Button variant="glass" onClick={() => {setDeleted(false);location.assign('/');}}>{t('web.link.backToPortico')}</Button></ServicesActions></ServicesAuthStage>;
  if (!session || authenticating) return <ServicesAuthForm account={account} before={securityEnded ? <ServicesNotice>{t('services.signin.ended')}</ServicesNotice> : undefined} onSignedIn={() => {setSecurityEnded(undefined);setAuthenticating(false);}} />;
  return <AccountPage identityTransaction={identityTransaction} account={account} session={session} section={tab as 'devices' | 'security' | 'servers' | 'notifications'} onDeleted={() => setDeleted(true)} onSecurityEnded={codes => setSecurityEnded({...(codes ? {codes: Object.freeze([...codes])} : {})})} />;
}
function DeviceRoute() {
  const {account, bootstrap} = root.useRouteContext<ServicesRouter>(), link = bootstrap.getSnapshot().link;
  return <DevicePage account={account} initialCode={link?.kind === 'device' ? link.value : undefined} />;
}
function ResetRoute() {
  const {account, bootstrap} = root.useRouteContext<ServicesRouter>(), link = bootstrap.getSnapshot().link;
  return <ResetPage account={account} token={link?.kind === 'reset' ? link.value : undefined} />;
}
function VerifyRoute() {
  const {account, bootstrap} = root.useRouteContext<ServicesRouter>(), link = bootstrap.getSnapshot().link;
  return <VerifyPage account={account} token={link?.kind === 'verify-email' ? link.value : undefined} />;
}
function JoinRoute() {
  const {account, join, plainShortJoin} = root.useRouteContext<ServicesRouter>();
  return join ? <JoinPage account={account} link={join} onBack={() => location.assign('/')} />
    : plainShortJoin ? <ShortJoinPage account={account} onBack={() => location.assign('/')} /> : <InvalidLink />;
}
function InvalidLink() {
  const {t} = servicesI18n();
  return <ServicesAuthStage variant="reset"><ServicesHeading>{t('auth.welcomeHome')}</ServicesHeading><ServicesNotice>{t('web.link.invalid')}</ServicesNotice>
    <ServicesActions><Button onClick={() => location.assign('/')}>{t('web.link.backToPortico')}</Button></ServicesActions></ServicesAuthStage>;
}
export {InvalidLink as ServicesInvalidLink};
function ClaimStart() {
  const {account, bootstrap} = root.useRouteContext<ServicesRouter>(), {session} = useAccount();
  const [authenticating, setAuthenticating] = useState(() => !session);
  useEffect(() => {if (!session) setAuthenticating(true);}, [session]);
  const {t} = servicesI18n();
  if (bootstrap.getSnapshot().ownerPending) {
    if (!session || authenticating) return <ServicesAuthForm account={account} heading={t('auth.welcomeHome')} lead={t('web.claim.signInFirst')}
      onSignedIn={() => setAuthenticating(false)} />;
    return <ClaimApprovalRoute account={account} bootstrap={bootstrap} />;
  }
  return <ServicesAuthStage variant="reset"><ServicesHeading>{t('web.claim.titleGeneric')}</ServicesHeading><ServicesNotice tone="info">{t('web.claim.openFromServer')}</ServicesNotice>
    <ServicesActions><Button onClick={() => location.assign('/')}>{t('web.claim.backToPortico')}</Button></ServicesActions></ServicesAuthStage>;
}

/** The scrubbed code is an intake locator only. An authenticated server lookup
 * chooses the drawn setup/switch view; its purpose-closed controller then
 * independently rechecks every decision and return against the current family. */
function ClaimApprovalRoute({account,bootstrap}:{account:ServicesAccount;bootstrap:ServicesBootstrap}) {
 const session=useSyncExternalStore(account.service.subscribe,account.service.getSnapshot).session;
 const scope=session?.familyId?JSON.stringify([session.account.id,session.familyId]):'';
 const [resolved,setResolved]=useState<{scope:string;purpose?:'setup'|'switch';error?:unknown}>();
 useEffect(()=>{
  if(!scope)return;
  const abort=new AbortController();
  void bootstrap.approvalPurpose(account,abort.signal).then(purpose=>{if(!abort.signal.aborted)setResolved({scope,purpose});},error=>{if(!abort.signal.aborted)setResolved({scope,error});});
  return()=>abort.abort();
 },[account,bootstrap,scope]);
 const active=resolved?.scope===scope?resolved:undefined;
 if(active?.error)return <InvalidLink/>;
 if(!active?.purpose)return <ServicesAuthStage><></></ServicesAuthStage>;
 return active.purpose==='setup'?<SetupApprovalPage account={account} bootstrap={bootstrap} Review={HostedSetupApproval}/>:<OwnerApprovalPage account={account} bootstrap={bootstrap}/>;
}
