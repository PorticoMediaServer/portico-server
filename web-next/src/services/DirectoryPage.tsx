import {useRef, useLayoutEffect, useMemo, useState, useSyncExternalStore, type ReactNode} from 'react';
import type {HostedSession} from '@core/index.ts';
import {ServicesServerDirectory} from '../kit/accounts/ServerDirectory';
import {ServicesAuthStage, HostedError} from '../kit/accounts/ServicesAuth';
import {LocalServerAddress} from '../kit/accounts/ServerSwitcher';
import {ServicesDirectoryStatus} from '../kit/accounts/DirectoryStatus';
import {ServicesDirectoryFlow} from './directory-flow';
import {DirectoryAccountUnavailable} from './directory-errors';
import {servicesDirectoryConnection, servicesLocalOrigin} from './directory-connection';
import {servicesServerDirectoryCopy} from './server-directory';
import {servicesI18n} from './i18n';
import {servicesRelativeTime} from './time';
import {accountIdentity, sameAccount, type ServicesAccount} from './session';
import type {WorkspaceApi, WorkspaceServers} from './workspace-api';

type Props = {account: ServicesAccount; session: Omit<HostedSession,'refreshToken'>; api: WorkspaceApi; initial: WorkspaceServers; sourceCurrent: () => boolean; header: ReactNode; onEmpty: () => void; onJoin: () => void; onSetup: () => void; onLocal: () => void};
export function ServicesDirectoryPage(props: Props) {
  const {accountId, familyId} = accountIdentity(props.session);
  const source = useMemo(() => ({account: props.account, identity: {accountId, familyId}, api: props.api, initial: props.initial, current: props.sourceCurrent, empty: props.onEmpty, local: props.onLocal}), [props.account, accountId, familyId, props.api, props.initial, props.sourceCurrent, props.onEmpty, props.onLocal]);
  const [instance, setInstance] = useState<{source: typeof source; flow: ServicesDirectoryFlow}>();
  // Each directory setup samples its own committed session, never a successor's token.
  const setupSession = useRef(props.session);
  useLayoutEffect(() => {setupSession.current = props.session;});
  useLayoutEffect(() => {
    const session = setupSession.current;
    let live = true;
    const current = () => {const session = source.account.service.getSnapshot().session; return Boolean(live && source.current() && session?.familyId && sameAccount(accountIdentity(session), source.identity));};
    const flow = new ServicesDirectoryFlow(source.initial, {current, read: source.api.servers, connect: servicesDirectoryConnection(source.account, session),
      navigate: (origin, purpose) => {if (current()) location.assign(origin + (purpose === 'owner-recovery' ? '/sign-in' : '/'));}, localRecovery: () => {if (current()) source.local();}, empty: () => {if (current()) source.empty();}});
    setInstance({source, flow});
    return () => {live = false; flow.dispose();};
  }, [source]);
  return instance?.source === source ? <ReadyDirectory {...props} flow={instance.flow} /> : <ServicesAuthStage><></></ServicesAuthStage>;
}
function ReadyDirectory({flow, ...props}: Props & {flow: ServicesDirectoryFlow}) {
  const state = useSyncExternalStore(flow.subscribe, flow.getSnapshot), i18n = useMemo(servicesI18n, []), {t} = i18n;
  const [clock, setClock] = useState(Date.now);
  useLayoutEffect(() => {const timer = setInterval(() => {if (document.visibilityState === 'visible') setClock(Date.now());},15000); return () => clearInterval(timer);}, []);
  const allOffline = state.items.length > 0 && state.items.every(item => item.status !== 'active');
  const blockedIdentity = state.status === 'identity' || state.status === 'clock';
  useLayoutEffect(() => {
    let live = true, timer: ReturnType<typeof setTimeout> | undefined;
    const items = state.items;
    const reconcile = () => {
      clearTimeout(timer);
      if (!live) return;
      if (document.visibilityState !== 'visible') {flow.cancelAutomatic(); return;}
      flow.automaticInitial();
      if (flow.getSnapshot().phase !== 'idle' || blockedIdentity || (!allOffline && !state.waitingId)) return;
      const deadline = Date.now() + flow.retryDelay() * 1000;
      const tick = () => {
        if (!live || document.visibilityState !== 'visible' || flow.getSnapshot().phase !== 'idle' || flow.getSnapshot().items !== items) return;
        const remaining = Math.max(0,Math.ceil((deadline-Date.now())/1000)); flow.countdown(remaining);
        if (!remaining) flow.automatic(); else timer = setTimeout(tick,Math.min(1000,deadline-Date.now()));
      };
      tick();
    };
    document.addEventListener('visibilitychange',reconcile); reconcile();
    return () => {live = false; clearTimeout(timer); document.removeEventListener('visibilitychange',reconcile);};
  }, [flow,state.phase,state.items,state.waitingId,allOffline,blockedIdentity]);
  const target = state.target;
  if (target && state.showTarget && (state.phase === 'opening' || state.phase === 'leaving' || state.phase === 'recovering' || state.status)) {
    const lastSeen = target.presence.lastSeenAt ? i18n.date(target.presence.lastSeenAt) : undefined;
    return <ServicesDirectoryStatus header={props.header} phase={state.phase === 'opening' || state.phase === 'leaving' && state.status !== 'account-waiting' ? 'opening' : state.status!}
      server={target.name} lastSeen={lastSeen} owner={target.ownedByMe} seconds={state.seconds} busy={state.phase === 'recovering'}
      error={state.phase === 'recovering' || state.error instanceof DirectoryAccountUnavailable || state.status !== 'account-waiting' ? undefined : <HostedError error={state.error}/>}
      onCheck={() => {void flow.open(target, state);}} onChoose={() => flow.cancelOpening(state)} onAccountHelp={() => {void flow.recoverOwner(state);}}/>;
  }
  const checked = clock-state.checkedAt < 45000 ? t('services.directory.justNow') : servicesRelativeTime(i18n,new Date(state.checkedAt).toISOString(),clock);
  return <ServicesServerDirectory header={props.header} signedInAs={props.session.account.displayName || props.session.account.username} checked={checked}
    servers={state.items.map(item => ({id:item.id,name:item.name,online:item.status==='active',presence:servicesServerDirectoryCopy(item,i18n).presence,ownedByMe:item.ownedByMe}))}
    offline={allOffline} seconds={state.seconds} pendingId={state.pendingId} busy={state.phase==='reading'} error={<HostedError error={state.error} />}
    onOpen={id=>{const item=state.items.find(item=>item.id===id);if(item)void flow.open(item,state);}} onRefresh={()=>void flow.refresh(state)}
    onAccount={()=>location.assign('/account')} onJoin={props.onJoin} onSetup={props.onSetup} onLocal={props.onLocal}
    onDifferentAccount={()=>{void props.account.service.logoutIfCurrent(accountIdentity(props.session)).catch(error=>flow.reportError(error,state));}} />;
}

export function ServicesLocalServerPage({current,onBack}: {current: () => boolean; onBack: () => void}) {
  const [instance,setInstance]=useState<{live:boolean;controller?:AbortController}>();
  const [busy,setBusy]=useState(false),[error,setError]=useState<unknown>();
  useLayoutEffect(()=>{const lease={live:true,controller:undefined as AbortController|undefined};setInstance(lease);return()=>{lease.live=false;lease.controller?.abort();};},[current]);
  return <LocalServerAddress busy={busy} error={error} onBack={()=>{if(instance?.live&&current())onBack();}} onContinue={address=>{
    const lease=instance;if(!lease?.live||lease.controller||!current())return;
    const controller=new AbortController();lease.controller=controller;setBusy(true);setError(undefined);
    const live=()=>lease.live&&lease.controller===controller&&!controller.signal.aborted&&current();
    void servicesLocalOrigin(address,controller.signal).then(origin=>{if(live())location.assign(origin+'/');},error=>{if(live())setError(error);}).finally(()=>{if(live()){lease.controller=undefined;setBusy(false);}});
  }} />;
}
